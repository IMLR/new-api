package opencode

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	opencodeapi "github.com/QuantumNous/new-api/pkg/opencode"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

const ChannelName = "opencode_go"

// sessionHeaders carry the conversation id. OpenCode Go uses it to keep a
// conversation on one route and to reuse prompt caches, and answers requests
// without the header with 400 MissingSessionID.
var sessionHeaders = []string{"x-opencode-session", "session_id"}

// defaultUserAgent identifies this relay when the calling client sends none.
// OpenCode Go asks clients to name themselves instead of relying on the HTTP
// library default.
const defaultUserAgent = "new-api"

// Adaptor serves OpenCode Go. One subscription publishes three API families:
// most models answer on chat completions, Qwen and MiniMax answer on Anthropic
// Messages, and Grok, GPT 5.6 Luna and Muse Spark answer on the OpenAI Responses
// API. The adaptor forwards each model to the family that owns it and converts
// the request and the answer when the client speaks another format.
type Adaptor struct {
	chat     openai.Adaptor
	messages claude.Adaptor
}

func (a *Adaptor) GetChannelName() string { return ChannelName }

// GetModelList returns nothing because the catalog is fetched from the upstream.
func (a *Adaptor) GetModelList() []string { return nil }

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	a.chat.Init(info)
	a.messages.Init(info)
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info == nil {
		return "", errors.New("missing relay info")
	}
	wire := wireFor(info)
	base := channelBaseURL(info)
	if info.RelayMode == relayconstant.RelayModeResponses {
		if wire != opencodeapi.WireOpenAIResponses {
			return "", fmt.Errorf("OpenCode Go serves model %q on the %s API, not on the OpenAI Responses API", modelName(info), wire.EndpointName())
		}
		return base + opencodeapi.OpenAIResponsesPath, nil
	}
	if info.RelayMode == relayconstant.RelayModeResponsesCompact {
		return "", errors.New("OpenCode Go does not support the compact Responses endpoint")
	}
	if passThroughBodyEnabled(info) && !formatMatchesWire(info.RelayFormat, wire) {
		return "", fmt.Errorf("OpenCode Go needs the request body in %s format for model %q, so request pass-through cannot be enabled for this channel", wire.EndpointName(), modelName(info))
	}
	return base + wire.Path(), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	var err error
	if wireFor(info) == opencodeapi.WireAnthropicMessages {
		// The Messages API reads x-api-key; a bearer token is answered with
		// "Missing API key." even when it is valid.
		err = a.messages.SetupRequestHeader(c, header, info)
	} else {
		err = a.chat.SetupRequestHeader(c, header, info)
	}
	if err != nil {
		return err
	}
	forwardSessionHeaders(c, header, info)
	setUserAgent(c, header)
	return nil
}

// forwardSessionHeaders writes the conversation id into the upstream headers.
// A client value is forwarded as is; clients that send none, such as batch
// jobs and the channel test, get a value derived from the channel and caller so
// the upstream can still route the request.
func forwardSessionHeaders(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) {
	for _, name := range sessionHeaders {
		if header.Get(name) != "" {
			continue
		}
		if value := clientHeaderValue(c, name); value != "" {
			header.Set(name, value)
		}
	}
	if header.Get("x-opencode-session") != "" {
		return
	}
	// Coding agents such as Codex send their own session header instead.
	if value := clientHeaderValue(c, "session_id"); value != "" {
		header.Set("x-opencode-session", value)
		return
	}
	if value := fallbackSessionID(info); value != "" {
		header.Set("x-opencode-session", value)
	}
}

func clientHeaderValue(c *gin.Context, name string) string {
	if c == nil || c.Request == nil {
		return ""
	}
	return strings.TrimSpace(c.Request.Header.Get(name))
}

// fallbackSessionID keeps requests of one channel, user and token on one
// upstream session, which preserves routing and prompt cache reuse for clients
// that send no conversation id of their own.
func fallbackSessionID(info *relaycommon.RelayInfo) string {
	if info == nil {
		return ""
	}
	return fmt.Sprintf("new-api-%d-%d-%d", info.ChannelId, info.UserId, info.TokenId)
}

// setUserAgent forwards the calling client's own identification, so the
// upstream sees a coding agent name rather than a generic HTTP client.
func setUserAgent(c *gin.Context, header *http.Header) {
	userAgent := clientHeaderValue(c, "User-Agent")
	if userAgent == "" {
		userAgent = defaultUserAgent
	}
	header.Set("User-Agent", userAgent)
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	normalizeRequestModel(info, &request.Model)
	switch wireFor(info) {
	case opencodeapi.WireAnthropicMessages:
		result, err := service.ConvertRequest(c, info, types.RelayFormatClaude, request)
		if err != nil {
			return nil, err
		}
		return result.Value, nil
	case opencodeapi.WireOpenAIResponses:
		result, err := service.ConvertRequest(c, info, types.RelayFormatOpenAIResponses, request)
		if err != nil {
			return nil, err
		}
		return result.Value, nil
	}
	streamOptions := request.StreamOptions
	result, err := a.chat.ConvertOpenAIRequest(c, info, request)
	if err != nil {
		return nil, err
	}
	// The OpenAI adaptor drops stream_options for third-party channel types.
	// OpenCode Go answers on OpenAI-compatible endpoints, where the usage flag
	// in the stream is accepted and keeps token accounting exact.
	if streamOptions != nil {
		switch converted := result.(type) {
		case *dto.GeneralOpenAIRequest:
			converted.StreamOptions = streamOptions
		case dto.GeneralOpenAIRequest:
			converted.StreamOptions = streamOptions
		}
	}
	return result, nil
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	normalizeRequestModel(info, &request.Model)
	if wireFor(info) == opencodeapi.WireAnthropicMessages {
		return a.messages.ConvertClaudeRequest(c, info, request)
	}
	return a.chat.ConvertClaudeRequest(c, info, request)
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	normalizeRequestModel(info, &request.Model)
	if wire := wireFor(info); wire != opencodeapi.WireOpenAIResponses {
		return nil, fmt.Errorf("OpenCode Go serves model %q on the %s API, not on the OpenAI Responses API", modelName(info), wire.EndpointName())
	}
	return a.chat.ConvertOpenAIResponsesRequest(c, info, request)
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return a.chat.ConvertGeminiRequest(c, info, request)
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return a.chat.ConvertRerankRequest(c, relayMode, request)
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return a.chat.ConvertEmbeddingRequest(c, info, request)
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return a.chat.ConvertAudioRequest(c, info, request)
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return a.chat.ConvertImageRequest(c, info, request)
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	wire := wireFor(info)
	if wire == opencodeapi.WireAnthropicMessages {
		return a.messages.DoResponse(c, resp, info)
	}
	if wire == opencodeapi.WireOpenAIResponses && info.RelayMode == relayconstant.RelayModeChatCompletions {
		return responsesToChat(c, info, resp)
	}
	return a.chat.DoResponse(c, resp, info)
}

// wireFor resolves the upstream API family from the model the request is routed
// to, so that a channel model mapping decides the endpoint as well.
func wireFor(info *relaycommon.RelayInfo) opencodeapi.Wire {
	return opencodeapi.WireForModel(modelName(info))
}

func modelName(info *relaycommon.RelayInfo) string {
	if info == nil {
		return ""
	}
	if name := strings.TrimSpace(info.UpstreamModelName); name != "" {
		return name
	}
	return strings.TrimSpace(info.OriginModelName)
}

// normalizeRequestModel keeps the model id sent upstream free of the
// `opencode-go/` prefix OpenCode configs use.
func normalizeRequestModel(info *relaycommon.RelayInfo, model *string) {
	if model != nil {
		*model = opencodeapi.NormalizeModel(*model)
	}
	if info != nil {
		info.UpstreamModelName = opencodeapi.NormalizeModel(info.UpstreamModelName)
	}
}

func channelBaseURL(info *relaycommon.RelayInfo) string {
	base := ""
	if info != nil {
		base = strings.TrimSpace(info.ChannelBaseUrl)
	}
	if base == "" {
		base = opencodeapi.BaseURL
	}
	return strings.TrimRight(base, "/")
}

func passThroughBodyEnabled(info *relaycommon.RelayInfo) bool {
	if info == nil {
		return false
	}
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled {
		return true
	}
	return info.ChannelSetting.PassThroughBodyEnabled
}

// formatMatchesWire reports whether the body of the client request is already
// the format the upstream wire expects.
func formatMatchesWire(format types.RelayFormat, wire opencodeapi.Wire) bool {
	switch wire {
	case opencodeapi.WireAnthropicMessages:
		return format == types.RelayFormatClaude
	case opencodeapi.WireOpenAIResponses:
		return format == types.RelayFormatOpenAIResponses
	default:
		return format == types.RelayFormatOpenAI
	}
}

// responsesToChat converts an answer of the Responses API into a chat
// completion. The upstream chooses the encoding: a non-stream request is
// answered with JSON and a stream request with server-sent events.
func responsesToChat(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	upstreamStream := isEventStream(resp.Header.Get("Content-Type"))
	clientStream := info.IsStream
	info.IsStream = clientStream || upstreamStream
	switch {
	case upstreamStream && clientStream:
		return openai.OaiResponsesToChatStreamHandler(c, info, resp)
	case upstreamStream:
		info.IsStream = false
		return openai.OaiResponsesToChatBufferedStreamHandler(c, info, resp)
	default:
		return openai.OaiResponsesToChatHandler(c, info, resp)
	}
}

func isEventStream(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/event-stream")
}
