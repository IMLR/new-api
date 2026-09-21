package cline

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// probeBodyLimit caps how much of a failed probe response is read. Cline quota
// errors are a few hundred bytes.
const probeBodyLimit = 64 << 10

// ProbeResult reports what one minimal completion request learned about a model
// route on one account.
type ProbeResult struct {
	StatusCode int           `json:"status_code"`
	Available  bool          `json:"available"`
	Code       string        `json:"code,omitempty"`
	Message    string        `json:"message,omitempty"`
	Window     time.Duration `json:"-"`
	Latency    time.Duration `json:"-"`
}

// CooldownUntil returns the time when the probed model can serve requests
// again. The second result is false when the account is not out of quota.
func (r *ProbeResult) CooldownUntil(now time.Time) (time.Time, bool) {
	if r == nil || r.Available || r.StatusCode != http.StatusTooManyRequests {
		return time.Time{}, false
	}
	if !IsDailyFreeLimit(r.Code, r.Message) {
		return time.Time{}, false
	}
	return now.Add(QuotaWindow(r.Message)), true
}

// ProbeModel sends one minimal streaming completion. Cline reports the daily
// cap as HTTP 429 with a reset hint, so the status and the message are enough to
// tell whether the free quota is still available and when it comes back.
func ProbeModel(
	ctx context.Context,
	client *http.Client,
	base string,
	credential *Credential,
	model string,
) (*ProbeResult, error) {
	if credential == nil {
		return nil, &HTTPError{Status: http.StatusUnauthorized, Reauth: true}
	}
	payload, err := common.Marshal(map[string]any{
		"model":      model,
		"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		"max_tokens": 1,
		"stream":     true,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(base, "/")+"/api/v1/chat/completions",
		bytes.NewReader(payload),
	)
	if err != nil {
		return nil, err
	}
	SetHeaders(req.Header)
	req.Header.Set("Authorization", "Bearer "+credential.Bearer())

	started := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	result := &ProbeResult{StatusCode: resp.StatusCode}
	// A success needs no body: the status already proves the route has quota.
	if resp.StatusCode == http.StatusOK {
		_, _ = io.CopyN(io.Discard, resp.Body, 1<<10)
		result.Available = true
		result.Latency = time.Since(started)
		return result, nil
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, probeBodyLimit))
	result.Latency = time.Since(started)
	if readErr != nil {
		return nil, readErr
	}
	result.Code, result.Message = parseErrorPayload(body)
	return result, nil
}

// parseErrorPayload reads the code and message out of a Cline error body.
func parseErrorPayload(body []byte) (string, string) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return "", ""
	}
	var payload struct {
		Error   any    `json:"error"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if common.UnmarshalJsonStr(string(trimmed), &payload) == nil {
		code, message := payload.Code, strings.TrimSpace(payload.Message)
		switch value := payload.Error.(type) {
		case string:
			if message == "" {
				message = strings.TrimSpace(value)
			}
		case map[string]any:
			if text, ok := value["code"].(string); ok && code == "" {
				code = text
			}
			if text, ok := value["message"].(string); ok && message == "" {
				message = strings.TrimSpace(text)
			}
		}
		if code != "" || message != "" {
			return code, message
		}
	}
	return "", string(trimmed)
}
