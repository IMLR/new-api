package cline

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

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
	if resp.StatusCode == 200 && !info.IsStream {
		merged, err := collectStream(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		resp.Body = io.NopCloser(bytes.NewReader(merged))
		resp.ContentLength = int64(len(merged))
		resp.Header.Set("Content-Type", "application/json")
		resp.Header.Del("Content-Length")
	}
	return resp, nil
}
