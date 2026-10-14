package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkBuddyCompatibilityErrorsAllowFallback(t *testing.T) {
	for _, tc := range []struct {
		name        string
		channelType int
		code        any
		want        bool
	}{
		{"missing system", constant.ChannelTypeWorkBuddy, 11128, true},
		{"invalid parameters", constant.ChannelTypeWorkBuddy, 11133, true},
		{"empty content", constant.ChannelTypeWorkBuddy, 11151, true},
		{"string code", constant.ChannelTypeWorkBuddy, "11151", true},
		{"other provider", constant.ChannelTypeMoonshot, 11151, false},
		{"other bad request", constant.ChannelTypeWorkBuddy, 11101, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			common.SetContextKey(c, constant.ContextKeyChannelType, tc.channelType)
			err := types.WithOpenAIError(types.OpenAIError{Code: tc.code, Message: "WorkBuddy rejection"}, http.StatusBadRequest)
			assert.Equal(t, tc.want, shouldRetry(c, err))
		})
	}
}

func TestWorkBuddyFallbackRespectsRetryGuards(t *testing.T) {
	for _, scenario := range []string{"cancelled", "output", "specific-channel", "skip-retry"} {
		t.Run(scenario, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeWorkBuddy)
			var options []types.NewAPIErrorOptions
			switch scenario {
			case "cancelled":
				ctx, cancel := context.WithCancel(c.Request.Context())
				cancel()
				c.Request = c.Request.WithContext(ctx)
			case "output":
				_, err := c.Writer.WriteString("data: started\n\n")
				require.NoError(t, err)
			case "specific-channel":
				c.Set("specific_channel_id", "1")
			case "skip-retry":
				options = append(options, types.ErrOptionWithSkipRetry())
			}
			err := types.WithOpenAIError(types.OpenAIError{Code: 11151, Message: "empty content"}, http.StatusBadRequest, options...)
			assert.False(t, shouldRetry(c, err))
		})
	}
}

func TestRelayWorkBuddyErrorsFallBackToMoonshot(t *testing.T) {
	for _, rejection := range []struct {
		name   string
		status int
		body   string
	}{
		{"HTTP numeric", http.StatusBadRequest, `{"code":11151,"msg":"a message has empty content"}`},
		{"HTTP string", http.StatusBadRequest, `{"code":"11151","msg":"a message has empty content"}`},
		{"SSE envelope", http.StatusOK, "data: {\"code\":11151,\"msg\":\"a message has empty content\"}\n\n"},
		{"SSE nested", http.StatusOK, "data: {\"error\":{\"code\":\"11151\",\"message\":\"a message has empty content\"}}\n\n"},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", rejection.name, stream), func(t *testing.T) {
				db, router := setupRelayRetryTest(t)
				var mu sync.Mutex
				var calls []string
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					account := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
					mu.Lock()
					calls = append(calls, account)
					mu.Unlock()
					var request struct {
						Model    string           `json:"model"`
						Stream   bool             `json:"stream"`
						Messages []map[string]any `json:"messages"`
					}
					if !assert.NoError(t, common.DecodeJson(r.Body, &request)) {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					assert.Equal(t, retryTestModel, request.Model)
					if account != "fallback" {
						assert.Equal(t, "/v2/chat/completions", r.URL.Path)
						assert.True(t, request.Stream)
						assert.Len(t, request.Messages, 2)
						if rejection.status == http.StatusOK {
							w.Header().Set("Content-Type", "text/event-stream")
						} else {
							w.Header().Set("Content-Type", "application/json")
						}
						w.WriteHeader(rejection.status)
						fmt.Fprint(w, rejection.body)
						return
					}
					assert.Equal(t, "/v1/chat/completions", r.URL.Path)
					assert.Equal(t, stream, request.Stream)
					assert.Equal(t, []map[string]any{{"role": "user", "content": "hello"}}, request.Messages)
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						fmt.Fprint(w, "data: {\"id\":\"ok\",\"object\":\"chat.completion.chunk\",\"model\":\"retry-contract\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"ok\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"ok\",\"object\":\"chat.completion.chunk\",\"model\":\"retry-contract\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\ndata: [DONE]\n\n")
						return
					}
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"id":"ok","object":"chat.completion","model":"retry-contract","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
				}))
				t.Cleanup(server.Close)
				createRetryChannel(t, db, server.URL, 0, constant.ChannelTypeWorkBuddy)
				createRetryChannel(t, db, server.URL, 1, constant.ChannelTypeWorkBuddy)
				createRetryChannel(t, db, server.URL, 2, constant.ChannelTypeMoonshot)

				body := fmt.Sprintf(`{"model":"retry-contract","messages":[{"role":"user","content":"hello"}],"stream":%t}`, stream)
				req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, req)
				require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
				assert.Contains(t, recorder.Body.String(), `"content":"ok"`)
				assert.NotContains(t, recorder.Body.String(), "11151")
				mu.Lock()
				assert.Equal(t, []string{"account-0", "account-1", "fallback"}, calls)
				mu.Unlock()
			})
		}
	}
}

func TestRelayWorkBuddyExhaustionPreservesRejection(t *testing.T) {
	db, router := setupRelayRetryTest(t)
	var mu sync.Mutex
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Header.Get("Authorization"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"code":11151,"msg":"a message has empty content"}`)
	}))
	t.Cleanup(server.Close)
	createRetryChannel(t, db, server.URL, 0, constant.ChannelTypeWorkBuddy)
	createRetryChannel(t, db, server.URL, 1, constant.ChannelTypeWorkBuddy)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"retry-contract","messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
	var result struct {
		Error types.OpenAIError `json:"error"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
	assert.Equal(t, float64(11151), result.Error.Code)
	assert.Equal(t, "workbuddy_upstream_error", result.Error.Type)
	assert.Contains(t, result.Error.Message, "a message has empty content")
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"Bearer account-0", "Bearer account-1"}, calls)
}
