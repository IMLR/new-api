package opencode

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRelayInfo(model string, mode int, format types.RelayFormat) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		RelayMode:       mode,
		RelayFormat:     format,
		OriginModelName: model,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenCodeGo,
			ChannelBaseUrl:    "https://opencode.ai/zen/go",
			ApiKey:            "sk-test",
			UpstreamModelName: model,
		},
	}
}

func testContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	return c, recorder
}

func TestGetRequestURLFollowsModelWire(t *testing.T) {
	cases := []struct {
		name  string
		model string
		mode  int
		want  string
	}{
		{"chat model", "kimi-k3", relayconstant.RelayModeChatCompletions, "https://opencode.ai/zen/go/v1/chat/completions"},
		{"messages model", "qwen3.8-max", relayconstant.RelayModeChatCompletions, "https://opencode.ai/zen/go/v1/messages"},
		{"responses model", "grok-4.5", relayconstant.RelayModeChatCompletions, "https://opencode.ai/zen/go/v1/responses"},
		{"responses model on responses route", "grok-4.5", relayconstant.RelayModeResponses, "https://opencode.ai/zen/go/v1/responses"},
		{"client prefix", "opencode-go/muse-spark-1.2-contributor", relayconstant.RelayModeResponses, "https://opencode.ai/zen/go/v1/responses"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url, err := (&Adaptor{}).GetRequestURL(testRelayInfo(tc.model, tc.mode, types.RelayFormatOpenAI))
			require.NoError(t, err)
			assert.Equal(t, tc.want, url)
		})
	}
}

func TestGetRequestURLRejectsMismatchedRoute(t *testing.T) {
	adaptor := &Adaptor{}
	_, err := adaptor.GetRequestURL(testRelayInfo("kimi-k3", relayconstant.RelayModeResponses, types.RelayFormatOpenAIResponses))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat completions")

	_, err = adaptor.GetRequestURL(testRelayInfo("grok-4.5", relayconstant.RelayModeResponsesCompact, types.RelayFormatOpenAIResponses))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compact")
}

func TestGetRequestURLRejectsPassThroughOnAnotherWire(t *testing.T) {
	info := testRelayInfo("qwen3.8-max", relayconstant.RelayModeChatCompletions, types.RelayFormatOpenAI)
	info.ChannelSetting.PassThroughBodyEnabled = true
	_, err := (&Adaptor{}).GetRequestURL(info)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Anthropic Messages")
}

func TestGetRequestURLUsesChannelBaseURL(t *testing.T) {
	info := testRelayInfo("kimi-k3", relayconstant.RelayModeChatCompletions, types.RelayFormatOpenAI)
	info.ChannelBaseUrl = "https://gateway.example.com/opencode/go/"
	url, err := (&Adaptor{}).GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://gateway.example.com/opencode/go/v1/chat/completions", url)
}

func TestSetupRequestHeaderMatchesWireAuth(t *testing.T) {
	c, _ := testContext(t)
	adaptor := &Adaptor{}

	messagesHeaders := http.Header{}
	require.NoError(t, adaptor.SetupRequestHeader(c, &messagesHeaders, testRelayInfo("minimax-m3", relayconstant.RelayModeChatCompletions, types.RelayFormatOpenAI)))
	assert.Equal(t, "sk-test", messagesHeaders.Get("x-api-key"))
	assert.Empty(t, messagesHeaders.Get("Authorization"))

	chatHeaders := http.Header{}
	require.NoError(t, adaptor.SetupRequestHeader(c, &chatHeaders, testRelayInfo("kimi-k3", relayconstant.RelayModeChatCompletions, types.RelayFormatOpenAI)))
	assert.Equal(t, "Bearer sk-test", chatHeaders.Get("Authorization"))
	assert.Empty(t, chatHeaders.Get("x-api-key"))
}

func TestSetupRequestHeaderForwardsClientSession(t *testing.T) {
	c, _ := testContext(t)
	c.Request.Header.Set("X-Opencode-Session", "sess-42")
	c.Request.Header.Set("session_id", "codex-session")
	adaptor := &Adaptor{}

	chatHeaders := http.Header{}
	require.NoError(t, adaptor.SetupRequestHeader(c, &chatHeaders, testRelayInfo("kimi-k3", relayconstant.RelayModeChatCompletions, types.RelayFormatOpenAI)))
	assert.Equal(t, "sess-42", chatHeaders.Get("x-opencode-session"))
	assert.Equal(t, "codex-session", chatHeaders.Get("session_id"))

	messagesHeaders := http.Header{}
	require.NoError(t, adaptor.SetupRequestHeader(c, &messagesHeaders, testRelayInfo("minimax-m3", relayconstant.RelayModeChatCompletions, types.RelayFormatOpenAI)))
	assert.Equal(t, "sess-42", messagesHeaders.Get("x-opencode-session"))
}

func TestConvertOpenAIRequestFollowsModelWire(t *testing.T) {
	c, _ := testContext(t)
	adaptor := &Adaptor{}

	chatRequest := &dto.GeneralOpenAIRequest{
		Model:         "opencode-go/kimi-k3",
		Stream:        common.GetPointer(true),
		StreamOptions: &dto.StreamOptions{IncludeUsage: true},
	}
	chatValue, err := adaptor.ConvertOpenAIRequest(c, testRelayInfo("opencode-go/kimi-k3", relayconstant.RelayModeChatCompletions, types.RelayFormatOpenAI), chatRequest)
	require.NoError(t, err)
	convertedChat, ok := chatValue.(*dto.GeneralOpenAIRequest)
	require.True(t, ok, "chat wire returned %T", chatValue)
	assert.Equal(t, "kimi-k3", convertedChat.Model)
	require.NotNil(t, convertedChat.StreamOptions, "stream options were dropped")
	assert.True(t, convertedChat.StreamOptions.IncludeUsage)

	messagesRequest := &dto.GeneralOpenAIRequest{
		Model: "qwen3.8-max",
		Messages: []dto.Message{
			{Role: "user", Content: "hi"},
		},
	}
	messagesValue, err := adaptor.ConvertOpenAIRequest(c, testRelayInfo("qwen3.8-max", relayconstant.RelayModeChatCompletions, types.RelayFormatOpenAI), messagesRequest)
	require.NoError(t, err)
	convertedMessages, ok := messagesValue.(*dto.ClaudeRequest)
	require.True(t, ok, "messages wire returned %T", messagesValue)
	assert.Equal(t, "qwen3.8-max", convertedMessages.Model)
	assert.NotEmpty(t, convertedMessages.Messages)

	responsesRequest := &dto.GeneralOpenAIRequest{
		Model: "grok-4.5",
		Messages: []dto.Message{
			{Role: "user", Content: "hi"},
		},
	}
	responsesValue, err := adaptor.ConvertOpenAIRequest(c, testRelayInfo("grok-4.5", relayconstant.RelayModeChatCompletions, types.RelayFormatOpenAI), responsesRequest)
	require.NoError(t, err)
	convertedResponses, ok := responsesValue.(*dto.OpenAIResponsesRequest)
	require.True(t, ok, "responses wire returned %T", responsesValue)
	assert.Equal(t, "grok-4.5", convertedResponses.Model)
}

func TestDoResponseConvertsResponsesAnswerToChat(t *testing.T) {
	c, recorder := testContext(t)
	info := testRelayInfo("grok-4.5", relayconstant.RelayModeChatCompletions, types.RelayFormatOpenAI)
	body := `{"id":"resp_1","object":"response","created_at":1,"model":"grok-4.5","status":"completed",
		"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}],
		"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	usage, apiErr := (&Adaptor{}).DoResponse(c, resp, info)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	chatUsage, ok := usage.(*dto.Usage)
	require.True(t, ok, "usage type = %T", usage)
	assert.Equal(t, 5, chatUsage.TotalTokens)
	assert.Contains(t, c.Writer.Header().Get("Content-Type"), "application/json")
	assert.Contains(t, recorder.Body.String(), "hello")
}
