// Package workbuddy relays CodeBuddy / WorkBuddy accounts. The upstream speaks
// OpenAI chat completions with extra requirements: the request always streams,
// the account headers carry a session family, and the body needs several
// normalizations before it is accepted.
package workbuddy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	workbuddyapi "github.com/QuantumNous/new-api/pkg/workbuddy"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

const ChannelName = "workbuddy"

// Adaptor relays through the shared OpenAI handler, which already understands
// the streaming and non-streaming answer shapes.
type Adaptor struct {
	openai.Adaptor
	credential *workbuddyapi.Credential
	meta       workbuddyapi.ChatMeta
	base       string
}

func (a *Adaptor) GetChannelName() string { return ChannelName }

// GetModelList returns nothing: the catalog comes from the upstream account.
func (a *Adaptor) GetModelList() []string { return nil }

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info == nil {
		return "", fmt.Errorf("missing relay info")
	}
	if err := unsupportedRelayMode(info.RelayMode); err != nil {
		return "", err
	}
	base := a.base
	if base == "" {
		base = strings.TrimRight(info.ChannelBaseUrl, "/")
	}
	if base == "" {
		base = workbuddyapi.ChatBaseCN
	}
	return strings.TrimRight(base, "/") + workbuddyapi.ChatCompletionsPath, nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, header)
	if a.credential == nil {
		return fmt.Errorf("WorkBuddy credential unavailable")
	}
	workbuddyapi.ChatHeaders(*header, a.credential, a.meta)
	return nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (any, error) {
	if err := unsupportedRelayMode(info.RelayMode); err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	credential, err := service.ResolveWorkBuddyCredential(c.Request.Context(), info.ChannelId, "")
	if err != nil {
		return nil, err
	}
	a.credential = credential
	a.meta = chatMeta(c, info)
	a.base = channelBase(info, credential)
	prepared := a.prepareBody(raw, info, credential)
	info.UpstreamRequestBodySize = int64(len(prepared))

	resp, err := a.sendRequest(c, info, prepared)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		refreshed, refreshErr := service.ResolveWorkBuddyCredential(c.Request.Context(), info.ChannelId, credential.AccessToken)
		if refreshErr != nil {
			return nil, refreshErr
		}
		a.credential = refreshed
		a.base = channelBase(info, refreshed)
		resp, err = a.sendRequest(c, info, prepared)
		if err != nil {
			return nil, err
		}
	}
	if resp.StatusCode >= 400 {
		return rewriteError(resp), nil
	}
	if !info.IsStream {
		merged, err := workbuddyapi.AggregateStream(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if !workbuddyapi.HasUsableAnswer(merged) {
			// The upstream answered with an empty turn, which some accounts do
			// when the tool call header frame goes missing. One more attempt
			// hides the flake from the client.
			if retried, retryErr := a.retryAggregate(c, info, prepared); retryErr == nil {
				merged = retried
			} else {
				common.SysError(fmt.Sprintf("workbuddy relay retry failed: %v", retryErr))
			}
		}
		return jsonResponse(resp, merged), nil
	}
	frameErr, prefix, err := peekStreamError(resp.Body)
	if err != nil {
		resp.Body.Close()
		return nil, err
	}
	if frameErr != nil {
		resp.Body.Close()
		return errorResponse(resp, frameErr.status(), frameErr.Error()), nil
	}
	resp.Body = a.streamBody(c, info, prepared, prefix, resp.Body)
	resp.Header.Set("Content-Type", "text/event-stream")
	return resp, nil
}

// streamBody wraps the first upstream body so the relay can ask for one more
// attempt when the stream ends without a usable answer. The frames of a
// discarded attempt never reach the client.
func (a *Adaptor) streamBody(c *gin.Context, info *relaycommon.RelayInfo, prepared, prefix []byte, first io.ReadCloser) *retryBody {
	stream := &retryBody{current: first}
	stream.reader = workbuddyapi.NormalizeStreamWithRetry(
		io.MultiReader(bytes.NewReader(prefix), first),
		func() (io.ReadCloser, error) {
			_ = stream.current.Close()
			next, err := a.openStream(c, info, prepared)
			if err != nil {
				return nil, err
			}
			stream.current = next
			return next, nil
		},
	)
	return stream
}

// openStream sends the prepared request once more and returns the upstream body
// with the frames that the error peek consumed put back in front. It serves the
// retry path, where an upstream error is reported as a failed attempt instead of
// a response of its own.
func (a *Adaptor) openStream(c *gin.Context, info *relaycommon.RelayInfo, prepared []byte) (io.ReadCloser, error) {
	resp, err := a.sendRequest(c, info, prepared)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("workbuddy upstream returned status %d", resp.StatusCode)
	}
	frameErr, prefix, err := peekStreamError(resp.Body)
	if err != nil {
		resp.Body.Close()
		return nil, err
	}
	if frameErr != nil {
		resp.Body.Close()
		return nil, frameErr
	}
	return &prefixedBody{
		Reader: io.MultiReader(bytes.NewReader(prefix), resp.Body),
		Closer: resp.Body,
	}, nil
}

// retryAggregate runs one more upstream attempt for a non streaming request
// whose first answer carried nothing the client could use.
func (a *Adaptor) retryAggregate(c *gin.Context, info *relaycommon.RelayInfo, prepared []byte) ([]byte, error) {
	body, err := a.openStream(c, info, prepared)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return workbuddyapi.AggregateStream(body)
}

// The upstream gateway serves HTTP/2, where a half dead stream surfaces as
// "http2: response body closed" in the middle of an answer. The chat path pins
// HTTP/1.1, so one client is cached per proxy address.
var workBuddyClients sync.Map

func upstreamClient(proxy string) (*http.Client, error) {
	if cached, ok := workBuddyClients.Load(proxy); ok {
		return cached.(*http.Client), nil
	}
	client, err := workbuddyapi.NewUpstreamClient(proxy)
	if err != nil {
		return nil, err
	}
	workBuddyClients.Store(proxy, client)
	return client, nil
}

// sendRequest performs one chat request with the pinned transport. It mirrors
// the shared request builder, including the channel header overrides.
func (a *Adaptor) sendRequest(c *gin.Context, info *relaycommon.RelayInfo, body []byte) (*http.Response, error) {
	url, err := a.GetRequestURL(info)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if err := a.SetupRequestHeader(c, &request.Header, info); err != nil {
		return nil, err
	}
	override, err := channel.ResolveHeaderOverride(info, c)
	if err != nil {
		return nil, err
	}
	for key, value := range override {
		request.Header.Set(key, value)
	}
	client, err := upstreamClient(info.ChannelSetting.Proxy)
	if err != nil {
		return nil, err
	}
	return client.Do(request)
}

// unsupportedRelayMode rejects the endpoint families the upstream does not
// serve. Chat, Responses and the Claude Messages route all end up on the chat
// path, and an unset mode is left to the caller of the relay.
func unsupportedRelayMode(mode int) error {
	switch mode {
	case relayconstant.RelayModeUnknown,
		relayconstant.RelayModeChatCompletions,
		relayconstant.RelayModeResponses:
		return nil
	case relayconstant.RelayModeResponsesCompact:
		return fmt.Errorf("WorkBuddy does not support the compact Responses endpoint")
	}
	return fmt.Errorf("WorkBuddy serves chat completions only")
}

// prepareBody runs the upstream request pipeline: the model capable thinking
// levels come from the cached catalog, everything else from the request itself.
func (a *Adaptor) prepareBody(raw []byte, info *relaycommon.RelayInfo, credential *workbuddyapi.Credential) []byte {
	supported, defaults := service.WorkBuddyModelCapabilities(info.ChannelId)
	opts := workbuddyapi.PrepareOptions{
		SupportedEfforts: supported,
		DefaultEfforts:   defaults,
		UID:              credential.UID,
		ConversationID:   a.meta.ConversationID,
		Global:           credential.IsGlobal(),
		Sanitize:         true,
	}
	return workbuddyapi.PrepareBody(raw, opts)
}

// channelBase resolves the upstream host: a channel base URL set by the
// operator wins, otherwise the account realm decides.
func channelBase(info *relaycommon.RelayInfo, credential *workbuddyapi.Credential) string {
	base := ""
	if info != nil {
		base = strings.TrimRight(info.ChannelBaseUrl, "/")
	}
	if base != "" && base != workbuddyapi.ChatBaseCN {
		return base
	}
	if credential != nil {
		return credential.ChatBase()
	}
	if base != "" {
		return base
	}
	return workbuddyapi.ChatBaseCN
}

// chatMeta builds the conversation identity: the client session when it sends
// one, and a stable per caller value otherwise, so consecutive turns of one
// conversation stay on the same upstream route and prefix cache.
func chatMeta(c *gin.Context, info *relaycommon.RelayInfo) workbuddyapi.ChatMeta {
	meta := workbuddyapi.ChatMeta{}
	if info != nil {
		meta.ConversationRequestID = strings.TrimSpace(info.RequestId)
	}
	if c != nil && c.Request != nil {
		for _, name := range []string{"X-Conversation-ID", "X-Session-Id", "session_id", "conversation_id"} {
			if value := strings.TrimSpace(c.Request.Header.Get(name)); value != "" {
				meta.ConversationID = value
				break
			}
		}
	}
	if meta.ConversationID == "" && info != nil {
		meta.ConversationID = fmt.Sprintf("workbuddy-%d-%d-%d", info.ChannelId, info.UserId, info.TokenId)
	}
	return meta
}

// upstreamFrameError is an error delivered inside a started event stream.
type upstreamFrameError struct {
	Code    any    `json:"code"`
	Message string `json:"message"`
}

func (e *upstreamFrameError) Error() string {
	if e.Code != nil {
		return fmt.Sprintf("code=%v msg=%s", e.Code, e.Message)
	}
	return e.Message
}

func (e *upstreamFrameError) status() int {
	if code, ok := e.Code.(float64); ok {
		switch int(code) {
		case 429:
			return http.StatusTooManyRequests
		case 401:
			return http.StatusUnauthorized
		}
	}
	return http.StatusBadRequest
}

// peekStreamError reads up to the first data frame and reports whether it
// carries an error, together with the bytes already consumed.
func peekStreamError(body io.Reader) (*upstreamFrameError, []byte, error) {
	reader := newFramePeeker(body)
	for {
		payload, raw, done, err := reader.next()
		if err != nil {
			return nil, reader.read(), err
		}
		if done {
			return nil, reader.read(), nil
		}
		if payload == "" {
			continue
		}
		var frame struct {
			Error *upstreamFrameError `json:"error"`
		}
		if common.Unmarshal([]byte(payload), &frame) == nil && frame.Error != nil {
			if frame.Error.Message == "" {
				frame.Error.Message = payload
			}
			if frame.Error.Code == nil {
				var envelope struct {
					Code any `json:"code"`
				}
				if common.Unmarshal([]byte(payload), &envelope) == nil {
					frame.Error.Code = envelope.Code
				}
			}
			return frame.Error, raw, nil
		}
		return nil, reader.read(), nil
	}
}

// rewriteError converts an upstream error envelope into the OpenAI error shape
// the relay reports to the caller.
func rewriteError(resp *http.Response) *http.Response {
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if err != nil {
		return resp
	}
	message := string(raw)
	var envelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if json.Unmarshal(raw, &envelope) == nil && envelope.Msg != "" {
		message = fmt.Sprintf("code=%d msg=%s", envelope.Code, envelope.Msg)
	}
	return errorResponse(resp, resp.StatusCode, message)
}

// errorResponse replaces a response with a JSON error body.
func errorResponse(resp *http.Response, status int, message string) *http.Response {
	body, _ := json.Marshal(map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    "workbuddy_upstream_error",
		},
	})
	resp.StatusCode = status
	resp.Status = fmt.Sprintf("%d %s", status, http.StatusText(status))
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Set("Content-Type", "application/json")
	resp.Header.Del("Content-Length")
	resp.Header.Del("Transfer-Encoding")
	return resp
}

// jsonResponse replaces a streaming upstream body with an aggregated answer.
func jsonResponse(resp *http.Response, body []byte) *http.Response {
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Set("Content-Type", "application/json")
	resp.Header.Del("Content-Length")
	resp.Header.Del("Transfer-Encoding")
	return resp
}

// prefixedBody replays the bytes consumed while inspecting the first frame.
type prefixedBody struct {
	io.Reader
	io.Closer
}

// retryBody keeps the upstream body that the current attempt reads from, so a
// retry can replace it while the client still closes one body.
type retryBody struct {
	reader  io.Reader
	current io.ReadCloser
}

func (b *retryBody) Read(p []byte) (int, error) { return b.reader.Read(p) }

func (b *retryBody) Close() error { return b.current.Close() }

// framePeeker reads server sent event frames and keeps everything it consumed.
type framePeeker struct {
	reader *bufio.Reader
	buffer bytes.Buffer
}

func newFramePeeker(source io.Reader) *framePeeker {
	return &framePeeker{reader: bufio.NewReaderSize(source, 64*1024)}
}

func (p *framePeeker) read() []byte {
	p.drain()
	return p.buffer.Bytes()
}

// drain moves the bytes the buffered reader pulled in but did not return yet
// into the replayed prefix. Without it a frame that arrived in the same read as
// the first one would be lost when the peek stops.
func (p *framePeeker) drain() {
	buffered := p.reader.Buffered()
	if buffered == 0 {
		return
	}
	extra := make([]byte, buffered)
	if _, err := io.ReadFull(p.reader, extra); err == nil {
		p.buffer.Write(extra)
	}
}

// next returns the payload of the next frame. done reports the end of the
// stream, raw returns everything read so far.
func (p *framePeeker) next() (payload string, raw []byte, done bool, err error) {
	for {
		line, readErr := p.reader.ReadString('\n')
		if line != "" {
			p.buffer.WriteString(line)
		}
		if readErr != nil && line == "" {
			if readErr == io.EOF {
				return "", p.buffer.Bytes(), true, nil
			}
			return "", p.buffer.Bytes(), false, readErr
		}
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r\n"))
		if strings.HasPrefix(trimmed, "data:") {
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
			if value == "[DONE]" {
				return "", p.buffer.Bytes(), true, nil
			}
			return value, p.buffer.Bytes(), false, nil
		}
		if readErr == io.EOF {
			return "", p.buffer.Bytes(), true, nil
		}
		if readErr != nil {
			return "", p.buffer.Bytes(), false, readErr
		}
	}
}
