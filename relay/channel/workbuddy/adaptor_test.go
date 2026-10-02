package workbuddy

import (
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
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

func TestGetRequestURLUsesMediaPathForImages(t *testing.T) {
	adaptor := &Adaptor{base: workbuddyapi.GlobalBase}
	for _, mode := range []int{relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits} {
		info := &relaycommon.RelayInfo{
			RelayMode:   mode,
			ChannelMeta: &relaycommon.ChannelMeta{},
		}
		url, err := adaptor.GetRequestURL(info)
		require.NoError(t, err, "mode %d", mode)
		assert.Equal(t, workbuddyapi.GlobalBase+"/v2/images/generations", url)
	}
}

func TestConvertImageRequestBuildsMediaPayload(t *testing.T) {
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	converted, err := adaptor.ConvertImageRequest(nil, info, dto.ImageRequest{
		Model:   "gpt-image-2.5-sunburst",
		Prompt:  "a red ball",
		Size:    "1024x1024",
		Quality: "high",
		Image:   json.RawMessage(`["https://cdn.example/first.png","https://cdn.example/second.png"]`),
	})
	require.NoError(t, err)
	payload, ok := converted.(workBuddyImageRequest)
	require.True(t, ok)
	assert.Equal(t, "gpt-image-2.5-sunburst", payload.Model)
	assert.Equal(t, "a red ball", payload.Prompt)
	assert.Equal(t, "1024x1024", payload.Size)
	assert.Equal(t, "high", payload.Quality)
	assert.Equal(t, "https://cdn.example/first.png", payload.Image)
	assert.Equal(t, "gpt-image-2.5-sunburst", info.UpstreamModelName)
}

func TestConvertImageRequestRejectsUploadedEdit(t *testing.T) {
	context := testContext(t, map[string]string{"Content-Type": "multipart/form-data; boundary=x"})
	context.Request.MultipartForm = &multipart.Form{
		File: map[string][]*multipart.FileHeader{"image": {{}}},
	}
	adaptor := &Adaptor{}
	_, err := adaptor.ConvertImageRequest(context, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}, dto.ImageRequest{
		Model:  "gpt-image-2.5-sunburst",
		Prompt: "edit this",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file uploads are not supported")
}

func TestTranslateImageResponseBuildsOpenAIShape(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body: io.NopCloser(strings.NewReader(`{"code":0,"msg":"OK","data":{
			"created":1790945010,"size":"1312x1199","quality":"low","output_format":"png",
			"data":[{"url":"https://cdn.example/image.png"}],
			"usage":{"total_tokens":4366,"credit":0.6}}}`)),
	}
	translated, apiErr := translateImageResponse(resp)
	require.Nil(t, apiErr)
	raw, err := io.ReadAll(translated.Body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"created":1790945010,"data":[{"url":"https://cdn.example/image.png"}],
		"usage":{"output_tokens":4366,"total_tokens":4366}}`, string(raw))
}

func TestTranslateImageResponseReportsUpstreamError(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body: io.NopCloser(strings.NewReader(
			`{"code":14401,"msg":"Create image failed with error: Image model [nope] route config not found"}`)),
	}
	_, apiErr := translateImageResponse(resp)
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "14401")
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
	rewritten, err := rewriteError(response)
	require.NoError(t, err)
	raw, err := io.ReadAll(rewritten.Body)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(raw, &payload))
	message := payload["error"].(map[string]any)["message"].(string)
	assert.Equal(t, "code=11102 msg=model unavailable", message)
	assert.EqualValues(t, 11102, payload["error"].(map[string]any)["code"])
	assert.Equal(t, http.StatusBadRequest, rewritten.StatusCode)
}

func TestRewriteErrorNamesTheMediaEndpoint(t *testing.T) {
	cases := map[string]string{
		`{"code":11103,"msg":"Backend [mps] is not supported"}`:  "/v1/video/generations",
		`{"code":11103,"msg":"Backend [maas] is not supported"}`: "/v1/images/generations",
	}
	for body, want := range cases {
		response := &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     http.Header{},
		}
		rewritten, err := rewriteError(response)
		require.NoError(t, err)
		raw, err := io.ReadAll(rewritten.Body)
		require.NoError(t, err)
		assert.Contains(t, string(raw), want)
	}
}

func TestErrorResponseReplacesStatusAndBody(t *testing.T) {
	response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("stream"))}
	rewritten := errorResponse(response, http.StatusTooManyRequests, "slow down", nil)
	assert.Equal(t, http.StatusTooManyRequests, rewritten.StatusCode)
	raw, _ := io.ReadAll(rewritten.Body)
	assert.Contains(t, string(raw), "slow down")
}

func TestStreamBodyRetriesEmptyAttempt(t *testing.T) {
	// The upstream sometimes streams a tool call without its header frame. That
	// attempt renders nothing, so the adaptor asks for one more and only the
	// surviving answer reaches the client.
	broken := strings.Join([]string{
		"data: {\"id\":\"chunk-1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\"}}]},\"finish_reason\":\"tool_calls\"}]}",
		"",
		"data: [DONE]",
		"",
	}, "\n")
	good := strings.Join([]string{
		"data: {\"id\":\"chunk-2\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}",
		"",
		"data: [DONE]",
		"",
	}, "\n")
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, good)
	}))
	defer server.Close()

	adaptor := &Adaptor{
		base:       server.URL,
		credential: &workbuddyapi.Credential{AccessToken: "token", UID: "uid"},
	}
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeWorkBuddy},
	}
	stream := adaptor.streamBody(testContext(t, nil), info, []byte("{}"),
		io.NopCloser(strings.NewReader(broken)))
	defer stream.Close()

	out, err := io.ReadAll(stream)
	require.NoError(t, err)
	assert.Equal(t, 1, requests, "the empty attempt is retried once")
	assert.Contains(t, string(out), "\"content\":\"ok\"")
	assert.Contains(t, string(out), "chunk-2")
	assert.NotContains(t, string(out), "chunk-1", "the discarded attempt leaves no frames behind")
	assert.NotContains(t, string(out), "without an answer")
}

func TestOpenStreamKeepsFramesOfTheRetriedAttempt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"chunk-3\",\"choices\":[]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	adaptor := &Adaptor{
		base:       server.URL,
		credential: &workbuddyapi.Credential{AccessToken: "token", UID: "uid"},
	}
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeWorkBuddy},
	}
	body, err := adaptor.openStream(testContext(t, nil), info, []byte("{}"))
	require.NoError(t, err)
	defer body.Close()

	out, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Contains(t, string(out), "chunk-3", "the peeked frame is replayed")
	assert.Contains(t, string(out), "[DONE]")
}

func TestPeekStreamErrorKeepsFramesReadAhead(t *testing.T) {
	// The peek stops at the first data frame, but the buffered reader may have
	// pulled later frames off the wire already. Those bytes belong to the
	// client and must survive the peek.
	body := strings.Join([]string{
		"data: {\"id\":\"chunk-1\",\"choices\":[]}",
		"",
		"data: {\"id\":\"chunk-2\",\"choices\":[]}",
		"",
		"data: [DONE]",
		"",
	}, "\n")
	frameErr, prefix, err := peekStreamError(strings.NewReader(body))
	require.NoError(t, err)
	assert.Nil(t, frameErr)
	assert.Contains(t, string(prefix), "chunk-1")
	assert.Contains(t, string(prefix), "chunk-2", "frames read ahead are replayed")
	assert.Contains(t, string(prefix), "[DONE]")
}
