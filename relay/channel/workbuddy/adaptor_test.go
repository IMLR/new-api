package workbuddy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	workbuddyapi "github.com/QuantumNous/new-api/pkg/workbuddy"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testContext(t *testing.T, headers map[string]string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	for name, value := range headers {
		context.Request.Header.Set(name, value)
	}
	return context
}

func TestChannelBaseFollowsRealmUnlessOperatorOverrides(t *testing.T) {
	cn := &workbuddyapi.Credential{RefreshToken: "rt"}
	global := &workbuddyapi.Credential{RefreshToken: "rt", Realm: workbuddyapi.RealmGlobal}

	// The stored default base URL keeps the account realm routing.
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: workbuddyapi.ChatBaseCN}}
	assert.Equal(t, workbuddyapi.ChatBaseCN, channelBase(info, cn))
	assert.Equal(t, workbuddyapi.GlobalBase, channelBase(info, global))

	custom := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://gateway.example.com"}}
	assert.Equal(t, "https://gateway.example.com", channelBase(custom, global))
}

func TestGetRequestURLUsesChatCompletionsPath(t *testing.T) {
	adaptor := &Adaptor{base: workbuddyapi.ChatBaseCN}
	for _, mode := range []int{
		relayconstant.RelayModeChatCompletions,
		relayconstant.RelayModeResponses,
		relayconstant.RelayModeUnknown,
	} {
		info := &relaycommon.RelayInfo{
			RelayMode:   mode,
			ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeWorkBuddy},
		}
		url, err := adaptor.GetRequestURL(info)
		require.NoError(t, err, "mode %d", mode)
		assert.Equal(t, workbuddyapi.ChatBaseCN+"/v2/chat/completions", url)
	}
}

func TestGetRequestURLRejectsUnsupportedModes(t *testing.T) {
	adaptor := &Adaptor{base: workbuddyapi.ChatBaseCN}
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeEmbeddings,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeWorkBuddy},
	}
	_, err := adaptor.GetRequestURL(info)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat completions")

	info.RelayMode = relayconstant.RelayModeResponsesCompact
	_, err = adaptor.GetRequestURL(info)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compact")
}

func TestChatMetaPrefersClientSession(t *testing.T) {
	context := testContext(t, map[string]string{"X-Session-Id": "sess-42", "X-Forwarded-For": "203.0.113.9, 10.0.0.1"})
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 19},
		RequestId:   "req-1",
		UserId:      7,
		TokenId:     31,
	}
	meta := chatMeta(context, info)
	assert.Equal(t, "sess-42", meta.ConversationID)
	assert.Equal(t, "req-1", meta.ConversationRequestID)

	anonymous := testContext(t, nil)
	meta = chatMeta(anonymous, info)
	assert.Equal(t, "workbuddy-19-7-31", meta.ConversationID, "a stable per caller session keeps routing and caching")
}

func TestPeekStreamErrorDetectsErrorFrame(t *testing.T) {
	body := "data: {\"error\":{\"code\":11128,\"message\":\"blocked\"}}\n\n"
	frameErr, _, err := peekStreamError(strings.NewReader(body))
	require.NoError(t, err)
	require.NotNil(t, frameErr)
	assert.Contains(t, frameErr.Error(), "11128")
	assert.Contains(t, frameErr.Error(), "blocked")
	assert.Equal(t, http.StatusBadRequest, frameErr.status())

	limited := &upstreamFrameError{Code: float64(429), Message: "slow down"}
	assert.Equal(t, http.StatusTooManyRequests, limited.status())
}

func TestPeekStreamErrorKeepsNormalFrames(t *testing.T) {
	body := "data: {\"id\":\"chunk-1\",\"choices\":[]}\n\ndata: [DONE]\n\n"
	frameErr, prefix, err := peekStreamError(strings.NewReader(body))
	require.NoError(t, err)
	assert.Nil(t, frameErr)
	assert.Contains(t, string(prefix), "chunk-1", "consumed bytes are replayed to the client")
}

func TestRewriteErrorTurnsEnvelopeIntoOpenAIError(t *testing.T) {
	response := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(strings.NewReader(`{"code":11102,"msg":"model unavailable"}`)),
		Header:     http.Header{},
	}
	rewritten := rewriteError(response)
	raw, err := io.ReadAll(rewritten.Body)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))
	message := payload["error"].(map[string]any)["message"].(string)
	assert.Equal(t, "code=11102 msg=model unavailable", message)
	assert.Equal(t, http.StatusBadRequest, rewritten.StatusCode)
}

func TestErrorResponseReplacesStatusAndBody(t *testing.T) {
	response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("stream"))}
	rewritten := errorResponse(response, http.StatusTooManyRequests, "slow down")
	assert.Equal(t, http.StatusTooManyRequests, rewritten.StatusCode)
	raw, _ := io.ReadAll(rewritten.Body)
	assert.Contains(t, string(raw), "slow down")
}
