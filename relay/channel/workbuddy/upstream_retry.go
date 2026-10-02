package workbuddy

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
)

// CodeBuddy also reports transient upstream validation failures as 400/11133.
// Invalid client parameters can produce the same code, so the two retries are
// bounded and shared with the empty-answer retry of this relay. A persistent
// rejection keeps its original status and business code.
var parameterRetryDelays = [...]time.Duration{time.Second, 4 * time.Second}

// requestChat checks errors before any response reaches the caller. A successful
// stream replays every byte consumed by the first-frame check.
func (a *Adaptor) requestChat(c *gin.Context, info *relaycommon.RelayInfo, prepared []byte) (*http.Response, error) {
	for {
		resp, err := a.sendRequest(c, info, prepared)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 400 {
			resp, err = rewriteError(resp)
			if err != nil {
				return nil, err
			}
		} else {
			frameErr, prefix, err := peekStreamError(resp.Body)
			if err != nil {
				resp.Body.Close()
				return nil, err
			}
			if frameErr == nil {
				resp.Body = &prefixedBody{
					Reader: io.MultiReader(bytes.NewReader(prefix), resp.Body),
					Closer: resp.Body,
				}
				return resp, nil
			}
			resp.Body.Close()
			resp = errorResponse(resp, frameErr.status(), frameErr.Error(), frameErr.Code)
		}

		if resp.StatusCode != http.StatusBadRequest || a.parameterRetries >= len(parameterRetryDelays) {
			return resp, nil
		}
		raw, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(raw))
		var rejection struct {
			Error upstreamFrameError `json:"error"`
		}
		if err != nil || common.Unmarshal(raw, &rejection) != nil || fmt.Sprint(rejection.Error.Code) != "11133" {
			return resp, nil
		}

		if a.parameterRetries == 0 {
			// Record the rejected request's shape without conversation content,
			// tool schemas, images, account headers or cache identifiers.
			var shape struct {
				Model           string     `json:"model"`
				Messages        []struct{} `json:"messages"`
				Tools           []struct{} `json:"tools"`
				MaxTokens       *int       `json:"max_tokens"`
				ReasoningEffort string     `json:"reasoning_effort"`
			}
			if common.Unmarshal(prepared, &shape) == nil {
				maxTokens := "unset"
				if shape.MaxTokens != nil {
					maxTokens = fmt.Sprint(*shape.MaxTokens)
				}
				logger.LogWarn(c, fmt.Sprintf("workbuddy code=11133 request: channel #%d, model=%q, messages=%d, tools=%d, max_tokens=%s, reasoning_effort=%q, request_bytes=%d", info.ChannelId, shape.Model, len(shape.Messages), len(shape.Tools), maxTokens, shape.ReasoningEffort, len(prepared)))
			}
		}

		delay := parameterRetryDelays[a.parameterRetries]
		a.parameterRetries++
		logger.LogWarn(c, fmt.Sprintf("workbuddy code=11133: channel #%d, retry=%d/%d, delay=%s", info.ChannelId, a.parameterRetries, len(parameterRetryDelays), delay))
		timer := time.NewTimer(delay)
		select {
		case <-c.Request.Context().Done():
			timer.Stop()
			return nil, c.Request.Context().Err()
		case <-timer.C:
		}
	}
}
