package cline

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	clineapi "github.com/QuantumNous/new-api/pkg/cline"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// Embed the standard OpenAI response handling and request conversion.
type Adaptor struct {
	openai.Adaptor
	credential *clineapi.Credential
}

func (a *Adaptor) GetChannelName() string { return "cline" }
func (a *Adaptor) GetModelList() []string { return nil } // Models are account-dependent.
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.RelayMode != relayconstant.RelayModeChatCompletions {
		return "", fmt.Errorf("Cline supports chat completions only")
	}
	base := strings.TrimRight(info.ChannelBaseUrl, "/")
	if base == "" {
		base = clineapi.BaseURL
	}
	return base + "/api/v1/chat/completions", nil
}
func (a *Adaptor) SetupRequestHeader(c *gin.Context, h *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, h)
	if a.credential == nil {
		return fmt.Errorf("Cline credential unavailable")
	}
	clineapi.SetHeaders(*h)
	h.Set("Authorization", "Bearer "+a.credential.Bearer())
	return nil
}
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (any, error) {
	if info.RelayMode != relayconstant.RelayModeChatCompletions {
		return nil, fmt.Errorf("Cline supports chat completions only")
	}
	var err error
	a.credential, err = service.ResolveClineCredential(c.Request.Context(), info.ChannelId, "")
	if err != nil {
		return nil, err
	}
	// Force SSE upstream: Cline's promotional routes can return empty nonstream responses.
	var request map[string]any
	if err = common.DecodeJson(body, &request); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, fmt.Errorf("Cline request must be an object")
	}
	request["stream"] = true
	request["stream_options"] = map[string]bool{"include_usage": true}
	encoded, err := common.Marshal(request)
	if err != nil {
		return nil, err
	}
	info.UpstreamRequestBodySize = int64(len(encoded))
	result, err := channel.DoApiRequest(a, c, info, bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	resp := result
	if resp.StatusCode == 401 {
		resp.Body.Close()
		a.credential, err = service.ResolveClineCredential(c.Request.Context(), info.ChannelId, a.credential.AccessToken)
		if err != nil {
			return nil, err
		}
		result, err = channel.DoApiRequest(a, c, info, bytes.NewReader(encoded))
		if err != nil {
			return nil, err
		}
		resp = result
	}
	if resp.StatusCode != 200 {
		// Cline reports the exhausted daily free quota either as an SSE error
		// frame inside HTTP 200 or as a plain HTTP 429 payload. The window in
		// the message drives channel selection, so it is read here as well.
		upstreamErr, err := readErrorBody(resp)
		if err != nil {
			resp.Body.Close()
			return nil, err
		}
		setQuotaRetryAfter(resp, upstreamErr)
		return resp, nil
	}
	if !info.IsStream {
		merged, err := collectStream(resp.Body)
		resp.Body.Close()
		if err != nil {
			var upstreamErr *UpstreamError
			if errors.As(err, &upstreamErr) {
				upstreamErr.RetryAfter = resp.Header.Get("Retry-After")
				setQuotaRetryAfter(resp, upstreamErr)
				return upstreamErrorResponse(resp, upstreamErr), nil
			}
			return nil, err
		}
		resp.Body = io.NopCloser(bytes.NewReader(merged))
		resp.ContentLength = int64(len(merged))
		resp.Header.Set("Content-Type", "application/json")
		resp.Header.Del("Content-Length")
		return resp, nil
	}
	// Cline reports exhausted free quota as an error frame in a started
	// stream. While nothing has been written downstream the failure can
	// still become a normal HTTP error, so downstream clients and channel
	// retries see the real cause.
	upstreamErr, prefix, err := peekFirstFrameError(resp.Body)
	if err != nil {
		resp.Body.Close()
		return nil, err
	}
	if upstreamErr != nil {
		resp.Body.Close()
		upstreamErr.RetryAfter = resp.Header.Get("Retry-After")
		setQuotaRetryAfter(resp, upstreamErr)
		return upstreamErrorResponse(resp, upstreamErr), nil
	}
	resp.Body = &prefixedBody{Reader: io.MultiReader(bytes.NewReader(prefix), resp.Body), Closer: resp.Body}
	return resp, nil
}

// setQuotaRetryAfter states Cline's quota window as a Retry-After header. The
// relay records the cooldown from the error message and this header, so daily
// caps without a reset hint still rest for the default window.
func setQuotaRetryAfter(resp *http.Response, upstreamErr *UpstreamError) {
	until, ok := upstreamErr.CooldownUntil(time.Now())
	if !ok {
		return
	}
	resp.Header.Set("Retry-After", strconv.FormatInt(int64(math.Ceil(time.Until(until).Seconds())), 10))
}

// prefixedBody replays the bytes consumed while inspecting the first SSE frame.
type prefixedBody struct {
	io.Reader
	io.Closer
}

// upstreamErrorResponse turns an in-stream Cline error into a regular upstream
// HTTP error response so the shared relay error path reports the real message
// and applies the configured retry rules.
func upstreamErrorResponse(resp *http.Response, upstreamErr *UpstreamError) *http.Response {
	body := upstreamErr.BuildErrorBody()
	resp.StatusCode = upstreamErr.StatusCode
	resp.Status = fmt.Sprintf("%d %s", upstreamErr.StatusCode, http.StatusText(upstreamErr.StatusCode))
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Set("Content-Type", "application/json")
	resp.Header.Del("Content-Length")
	resp.Header.Del("Transfer-Encoding")
	return resp
}
