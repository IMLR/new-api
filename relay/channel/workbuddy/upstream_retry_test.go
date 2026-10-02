package workbuddy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	workbuddyapi "github.com/QuantumNous/new-api/pkg/workbuddy"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const usableStream = "data: {\"id\":\"answer\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"

func retryTestAdaptor(server *httptest.Server) (*Adaptor, *relaycommon.RelayInfo) {
	return &Adaptor{
			base:       server.URL,
			credential: &workbuddyapi.Credential{AccessToken: "token", UID: "uid"},
			meta:       workbuddyapi.ChatMeta{ConversationID: "conversation", ConversationRequestID: "turn"},
		}, &relaycommon.RelayInfo{
			RelayMode:   relayconstant.RelayModeChatCompletions,
			ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeWorkBuddy, ChannelId: 31},
		}
}

func TestRequestChatRecoversFromParameterRejection(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"HTTP rejection", http.StatusBadRequest, `{"code":11133,"msg":"Invalid request parameters"}`},
		{"SSE error", http.StatusOK, "data: {\"error\":{\"code\":\"11133\",\"message\":\"Invalid request parameters\"}}\n\n"},
		{"SSE envelope", http.StatusOK, "data: {\"code\":11133,\"msg\":\"Invalid request parameters\"}\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var bodies []string
			var authorization, conversations, turns []string
			var records sync.Mutex
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				records.Lock()
				bodies = append(bodies, string(raw))
				authorization = append(authorization, r.Header.Get("Authorization"))
				conversations = append(conversations, r.Header.Get("X-Conversation-ID"))
				turns = append(turns, r.Header.Get("X-Conversation-Request-ID"))
				request := len(bodies)
				records.Unlock()
				if request == 1 {
					w.WriteHeader(tc.status)
					_, _ = io.WriteString(w, tc.body)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, usableStream)
			}))
			defer server.Close()
			adaptor, info := retryTestAdaptor(server)
			prepared := []byte(`{"model":"deepseek-v4.1-flash","messages":[{"role":"user","content":"hello"}],"stream":true}`)

			resp, err := adaptor.requestChat(testContext(t, nil), info, prepared)
			require.NoError(t, err)
			defer resp.Body.Close()
			out, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, usableStream, string(out), "only the successful attempt reaches the caller")
			records.Lock()
			defer records.Unlock()
			assert.Equal(t, []string{string(prepared), string(prepared)}, bodies)
			assert.Equal(t, []string{"Bearer token", "Bearer token"}, authorization)
			assert.Equal(t, []string{"conversation", "conversation"}, conversations)
			assert.Equal(t, []string{"turn", "turn"}, turns)
		})
	}
}

func TestRequestChatKeepsPersistentRejection(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"code":11133,"msg":"Invalid request parameters"}`)
	}))
	defer server.Close()
	adaptor, info := retryTestAdaptor(server)

	resp, err := adaptor.requestChat(testContext(t, nil), info, []byte("{}"))
	require.NoError(t, err)
	defer resp.Body.Close()
	var payload struct {
		Error upstreamFrameError `json:"error"`
	}
	require.NoError(t, common.DecodeJson(resp.Body, &payload))
	assert.EqualValues(t, 3, requests.Load(), "a permanent rejection stops after two retries")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.EqualValues(t, 11133, payload.Error.Code)
	assert.Equal(t, "code=11133 msg=Invalid request parameters", payload.Error.Message)
}

func TestRequestChatDoesNotRetryOtherErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"other parameter error", http.StatusBadRequest, `{"code":11101,"msg":"invalid model"}`},
		{"error text is not a code", http.StatusBadRequest, `{"code":11101,"msg":"11133"}`},
		{"rate limit", http.StatusTooManyRequests, `{"code":11133,"msg":"too many requests"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			adaptor, info := retryTestAdaptor(server)

			resp, err := adaptor.requestChat(testContext(t, nil), info, []byte("{}"))
			require.NoError(t, err)
			defer resp.Body.Close()
			out, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.EqualValues(t, 1, requests.Load())
			assert.Equal(t, tc.status, resp.StatusCode)
			assert.Contains(t, string(out), "workbuddy_upstream_error")
		})
	}
}

func TestParameterRetryBudgetSurvivesEmptyAnswerRetry(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 3 {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"code":11133,"msg":"Invalid request parameters"}`)
	}))
	defer server.Close()
	adaptor, info := retryTestAdaptor(server)
	c := testContext(t, nil)
	resp, err := adaptor.requestChat(c, info, []byte("{}"))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	stream := adaptor.streamBody(c, info, []byte("{}"), resp.Body)
	defer stream.Close()
	out, err := io.ReadAll(stream)
	require.NoError(t, err)
	assert.EqualValues(t, 4, requests.Load(), "the empty-answer retry must not start another parameter-retry budget")
	assert.Contains(t, string(out), "upstream stream ended without an answer")
}

func TestRequestChatDoesNotReplayPartialAnswer(t *testing.T) {
	var requests atomic.Int32
	partial := "data: {\"id\":\"answer\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"}}]}\n\ndata: {\"error\":{\"code\":11133,\"message\":\"Invalid request parameters\"}}\n\ndata: [DONE]\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, partial)
	}))
	defer server.Close()
	adaptor, info := retryTestAdaptor(server)
	c := testContext(t, nil)
	resp, err := adaptor.requestChat(c, info, []byte("{}"))
	require.NoError(t, err)
	stream := adaptor.streamBody(c, info, []byte("{}"), resp.Body)
	defer stream.Close()
	out, err := io.ReadAll(stream)
	require.NoError(t, err)
	assert.EqualValues(t, 1, requests.Load(), "a retry must not duplicate text or tool calls already received")
	assert.Contains(t, string(out), `"content":"ok"`)
	assert.Contains(t, string(out), "11133")
}

func TestRequestChatHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"code":11133,"msg":"Invalid request parameters"}`)
		w.(http.Flusher).Flush()
		cancel()
	}))
	defer server.Close()
	adaptor, info := retryTestAdaptor(server)
	c := testContext(t, nil)
	c.Request = c.Request.WithContext(ctx)

	resp, err := adaptor.requestChat(c, info, []byte("{}"))
	if resp != nil {
		resp.Body.Close()
	}
	require.ErrorIs(t, err, context.Canceled)
	assert.EqualValues(t, 1, requests.Load())
}
