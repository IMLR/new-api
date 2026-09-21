package cline

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamCollectsToolArgumentsAndUsage(t *testing.T) {
	raw := `data: {"id":"r","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"t","function":{"name":"echo","arguments":"{\"text\":"}}]}}]}

data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"ok\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":12,"completion_tokens":8,"total_tokens":20}}

data: [DONE]
`
	b, err := collectStream(strings.NewReader(raw))
	require.NoError(t, err)
	var result struct {
		Choices []collectedChoice `json:"choices"`
		Usage   struct {
			Total int `json:"total_tokens"`
		} `json:"usage"`
	}
	require.NoError(t, common.Unmarshal(b, &result))
	require.Len(t, result.Choices, 1)
	assert.Equal(t, "tool_calls", result.Choices[0].FinishReason)
	require.Len(t, result.Choices[0].Message.Tools, 1)
	assert.JSONEq(t, `{"text":"ok"}`, result.Choices[0].Message.Tools[0].Function.Arguments)
	assert.Equal(t, 20, result.Usage.Total)
}
func TestStreamRejectsErrorsAndTruncation(t *testing.T) {
	for _, raw := range []string{"data: {\"error\":{\"message\":\"limit\"}}\n", "data: [DONE]\n", "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"}}]}\n", "data: invalid\n"} {
		_, err := collectStream(strings.NewReader(raw))
		assert.Error(t, err)
	}
}

func TestStreamKeepsUpstreamErrorMessage(t *testing.T) {
	raw := `data: {"error":{"code":"INFERENCE_CAP_ERROR","message":"Error 429: Daily free limit reached on model vmc/fireworks-cline-k3-contributor-fallbacks. Try again in 7h 30m"}}

data: [DONE]
`
	_, err := collectStream(strings.NewReader(raw))
	require.Error(t, err)
	var upstreamErr *UpstreamError
	require.ErrorAs(t, err, &upstreamErr)
	assert.Equal(t, http.StatusTooManyRequests, upstreamErr.StatusCode)
	assert.Equal(t, "INFERENCE_CAP_ERROR", upstreamErr.Code)
	assert.Contains(t, upstreamErr.Error(), "Daily free limit reached")
	assert.NotContains(t, upstreamErr.Error(), "Cline upstream stream failed")
}

func TestUpstreamErrorStatusMapping(t *testing.T) {
	cases := []struct {
		code    string
		message string
		want    int
	}{
		{"INFERENCE_CAP_ERROR", "Error 429: Daily free limit reached", http.StatusTooManyRequests},
		{"", "Rate limit exceeded, retry later", http.StatusTooManyRequests},
		{"UNAUTHORIZED", "invalid token", http.StatusUnauthorized},
		{"", "Cline credential revoked", http.StatusUnauthorized},
		{"UPSTREAM_FAILURE", "provider returned no content", http.StatusBadGateway},
	}
	for _, item := range cases {
		assert.Equal(t, item.want, upstreamErrorStatus(item.code, item.message), item.message)
	}
}

func TestPeekFirstFrameErrorOnlyInspectsFirstEvent(t *testing.T) {
	errorFrame := `data: {"error":{"code":"INFERENCE_CAP_ERROR","message":"Error 429: Daily free limit reached"}}

`
	upstreamErr, _, err := peekFirstFrameError(strings.NewReader(errorFrame))
	require.NoError(t, err)
	require.NotNil(t, upstreamErr)
	assert.Equal(t, http.StatusTooManyRequests, upstreamErr.StatusCode)

	normal := `data: {"choices":[{"index":0,"delta":{"content":"hi"}}]}

data: {"error":{"message":"late failure"}}

`
	upstreamErr, prefix, err := peekFirstFrameError(strings.NewReader(normal))
	require.NoError(t, err)
	assert.Nil(t, upstreamErr)
	assert.Equal(t, normal, string(prefix))
}

func TestUpstreamErrorResponseBecomesHTTPError(t *testing.T) {
	resp := &http.Response{
		StatusCode:    http.StatusOK,
		Status:        "200 OK",
		Header:        http.Header{"Content-Type": {"text/event-stream"}},
		Body:          io.NopCloser(strings.NewReader("data: [DONE]\n\n")),
		ContentLength: 14,
	}
	converted := upstreamErrorResponse(resp, &UpstreamError{
		StatusCode: http.StatusTooManyRequests,
		Code:       "INFERENCE_CAP_ERROR",
		Message:    "Error 429: Daily free limit reached",
	})
	assert.Equal(t, http.StatusTooManyRequests, converted.StatusCode)
	assert.Equal(t, "application/json", converted.Header.Get("Content-Type"))
	body, err := io.ReadAll(converted.Body)
	require.NoError(t, err)
	assert.Equal(t, int64(len(body)), converted.ContentLength)
	assert.True(t, bytes.HasPrefix(body, []byte(`{"error"`)), string(body))
	assert.Contains(t, string(body), "Daily free limit reached")
	assert.Contains(t, string(body), "INFERENCE_CAP_ERROR")
}
