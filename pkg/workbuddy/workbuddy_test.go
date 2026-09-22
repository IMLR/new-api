package workbuddy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCredentialAcceptsBothStorageShapes(t *testing.T) {
	nested := `{"auth":{"accessToken":"at-1","refreshToken":"rt-1","expiresAt":1790000000,"domain":"www.workbuddy.ai","realm":"global"},"account":{"uid":"user-123456","enterpriseId":"ent-1","nickname":"Alice"},"device_token":"dev-1"}`
	credential, err := ParseCredential(nested)
	require.NoError(t, err)
	assert.Equal(t, "at-1", credential.AccessToken)
	assert.Equal(t, "rt-1", credential.RefreshToken)
	assert.Equal(t, "user-123456", credential.UID)
	assert.Equal(t, "ent-1", credential.EnterpriseID)
	assert.Equal(t, "dev-1", credential.DeviceToken)
	assert.Equal(t, RealmGlobal, credential.EffectiveRealm())
	assert.Equal(t, GlobalBase, credential.ChatBase())
	assert.Equal(t, "****3456", credential.MaskUID())

	flat := `{"accessToken":"at-2","refreshToken":"rt-2","expiresAt":1790000000000,"domain":"copilot.tencent.com","uid":"cn-user"}`
	credential, err = ParseCredential(flat)
	require.NoError(t, err)
	assert.Equal(t, int64(1790000000), credential.ExpiresAt, "milliseconds are normalized to seconds")
	assert.Equal(t, RealmCN, credential.EffectiveRealm())
	assert.Equal(t, ChatBaseCN, credential.ChatBase())
	assert.Equal(t, BillingBaseCN, credential.BillingBase())
}

func TestParseCredentialRejectsIncompleteInput(t *testing.T) {
	_, err := ParseCredential(`{"accessToken":"only"}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refreshToken")

	_, err = ParseCredential(`{"refreshToken":"rt","realm":"moon"}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "realm")

	_, err = ParseCredential("not json")
	require.Error(t, err)
}

func TestCredentialRefreshWindow(t *testing.T) {
	credential := &Credential{AccessToken: "at", RefreshToken: "rt", ExpiresAt: time.Now().Add(time.Hour).Unix()}
	assert.False(t, credential.NeedsRefresh(time.Now(), 5*time.Minute))
	assert.True(t, credential.NeedsRefresh(time.Now(), 2*time.Hour))
	assert.True(t, (&Credential{RefreshToken: "rt"}).NeedsRefresh(time.Now(), time.Minute))
}

func prepare(t *testing.T, request string, opts PrepareOptions) map[string]any {
	t.Helper()
	out := PrepareBody([]byte(request), opts)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(out, &parsed))
	return parsed
}

func TestPrepareBodyNormalizesRequest(t *testing.T) {
	body := `{
		"model":"glm-5.3",
		"max_completion_tokens":4096,
		"tool_choice":{"type":"function","function":{"name":"lookup"}},
		"messages":[
			{"role":"developer","content":"rules"},
			{"role":"user","content":[{"type":"image_url","image_url":"data:image/png;base64,AAA"}]}
		]
	}`
	parsed := prepare(t, body, PrepareOptions{UID: "uid-1", ConversationID: "conv-1"})
	assert.Equal(t, true, parsed["stream"], "the upstream rejects non-streaming requests")
	assert.Equal(t, float64(4096), parsed["max_tokens"])
	assert.NotContains(t, parsed, "max_completion_tokens")
	assert.Equal(t, "lookup", parsed["tool_choice"])
	assert.Equal(t, map[string]any{"include_usage": true}, parsed["stream_options"])
	cacheKey, _ := parsed["prompt_cache_key"].(string)
	assert.True(t, strings.HasPrefix(cacheKey, "wb2a-uid-1-"), "cache key carries the account prefix: %s", cacheKey)
	messages := parsed["messages"].([]any)
	first := messages[0].(map[string]any)
	assert.Equal(t, "system", first["role"])
	second := messages[1].(map[string]any)
	parts := second["content"].([]any)
	assert.Equal(t, map[string]any{"url": "data:image/png;base64,AAA"}, parts[0].(map[string]any)["image_url"])
}

func TestPrepareBodyKeepsClientCacheKeyAndExistingStreamOptions(t *testing.T) {
	body := `{"model":"glm-5.3","stream_options":{"include_usage":false},"prompt_cache_key":"client-key","messages":[{"role":"user","content":"hi"}]}`
	parsed := prepare(t, body, PrepareOptions{UID: "uid-1"})
	assert.Equal(t, "client-key", parsed["prompt_cache_key"])
	assert.Equal(t, map[string]any{"include_usage": false}, parsed["stream_options"])
}

func TestPrepareBodyInjectsDeepSeekThinking(t *testing.T) {
	body := `{"model":"deepseek-v4.1-flash","messages":[
		{"role":"user","content":"hi"},
		{"role":"assistant","content":"earlier"}
	]}`
	parsed := prepare(t, body, PrepareOptions{
		UID:              "uid-1",
		SupportedEfforts: map[string][]string{"deepseek-v4.1-flash": {"low", "high"}},
		DefaultEfforts:   map[string]string{"deepseek-v4.1-flash": "max"},
	})
	assert.Equal(t, map[string]any{"type": "enabled"}, parsed["thinking"])
	// max is unsupported, so the effort drops to the highest supported level.
	assert.Equal(t, "high", parsed["reasoning_effort"])
	messages := parsed["messages"].([]any)
	assistant := messages[1].(map[string]any)
	assert.Equal(t, " ", assistant["reasoning"], "assistant turns need a non-empty reasoning value")
	assert.Equal(t, "", assistant["reasoning_content"])
}

func TestPrepareBodyRespectsDisabledThinking(t *testing.T) {
	body := `{"model":"deepseek-v4-pro","thinking":{"type":"disabled"},"reasoning_effort":"high","messages":[{"role":"user","content":"hi"}]}`
	parsed := prepare(t, body, PrepareOptions{UID: "uid-1"})
	assert.Equal(t, map[string]any{"type": "disabled"}, parsed["thinking"])
	assert.NotContains(t, parsed, "reasoning_effort")
}

func TestPrepareBodyCleansUnpairedToolCalls(t *testing.T) {
	body := `{"model":"kimi-k3","messages":[
		{"role":"user","content":"hi"},
		{"role":"assistant","content":null,"tool_calls":[
			{"id":"call-1","type":"function","function":{"name":"a","arguments":"{}"}},
			{"id":"call-2","type":"function","function":{"name":"b","arguments":"{}"}}
		]},
		{"role":"tool","tool_call_id":"call-1","content":"result"},
		{"role":"tool","tool_call_id":"orphan","content":"stray"}
	]}`
	parsed := prepare(t, body, PrepareOptions{UID: "uid-1"})
	messages := parsed["messages"].([]any)
	require.Len(t, messages, 3, "the stray tool result is dropped")
	assistant := messages[1].(map[string]any)
	calls := assistant["tool_calls"].([]any)
	require.Len(t, calls, 1)
	assert.Equal(t, "call-1", calls[0].(map[string]any)["id"])
}

func TestPrepareBodySanitizesReviewFingerprints(t *testing.T) {
	body := `{"model":"glm-5.3","messages":[
		{"role":"system","content":"You are Claude Code, Anthropic's official CLI for Claude. Error code 11128."},
		{"role":"user","content":"hi"}
	]}`
	parsed := prepare(t, body, PrepareOptions{UID: "uid-1", Sanitize: true})
	messages := parsed["messages"].([]any)
	system := messages[0].(map[string]any)["content"].(string)
	assert.Contains(t, system, "official CLI tool for Claude")
	assert.Contains(t, system, "11-128")
	assert.NotContains(t, system, "11128")
}

func TestPrepareBodyAddsGlobalSystemMessage(t *testing.T) {
	body := `{"model":"glm-5.3","messages":[{"role":"user","content":"hi"}]}`
	parsed := prepare(t, body, PrepareOptions{UID: "uid-1", Global: true})
	messages := parsed["messages"].([]any)
	require.Len(t, messages, 2)
	first := messages[0].(map[string]any)
	assert.Equal(t, "system", first["role"])
	assert.Equal(t, "You are a helpful assistant.", first["content"])

	cn := prepare(t, body, PrepareOptions{UID: "uid-1"})
	assert.Len(t, cn["messages"].([]any), 1)
}

func TestPrepareBodyLeavesBrokenInputUntouched(t *testing.T) {
	assert.Equal(t, "not json", string(PrepareBody([]byte("not json"), PrepareOptions{})))
	assert.Equal(t, "", string(PrepareBody(nil, PrepareOptions{})))
}

func TestChatHeadersCarryProtocolFields(t *testing.T) {
	credential := &Credential{
		AccessToken:  "at-1",
		RefreshToken: "rt-1",
		UID:          "user-1",
		EnterpriseID: "ent-1",
		Domain:       "copilot.tencent.com",
		DeviceToken:  "dev-1",
	}
	const requestID = "0123456789abcdef0123456789abcdef"
	headers := http.Header{}
	ChatHeaders(headers, credential, ChatMeta{ConversationID: "conv-9", ConversationRequestID: requestID})

	assert.Equal(t, "Bearer at-1", headers.Get("Authorization"))
	assert.Equal(t, "1", headers.Get("X-CodeBuddy-Request"))
	assert.Equal(t, "user-1", headers.Get("X-User-Id"))
	assert.Equal(t, "ent-1", headers.Get("X-Enterprise-Id"))
	assert.Equal(t, "zh-CN", headers.Get("Accept-Language"))
	assert.Equal(t, "https://www.codebuddy.cn", headers.Get("Origin"))
	assert.Contains(t, headers.Get("User-Agent"), "WorkBuddy/"+ClientVersion)
	assert.Contains(t, headers.Get("User-Agent"), "CLI/"+CliVersion)
	assert.Equal(t, "dev-1", headers.Get("X-Device-Token"))
	assert.Empty(t, headers.Get("X-Forwarded-For"), "the client address stays with the relay")
	assert.Equal(t, "conv-9", headers.Get("X-Conversation-ID"))
	assert.Equal(t, requestID, headers.Get("X-Conversation-Request-ID"))
	assert.Len(t, headers.Get("X-Conversation-Message-ID"), 32)
	assert.Equal(t, headers.Get("X-Conversation-Message-ID"), headers.Get("X-Request-ID"))
	assert.Equal(t, requestID, headers.Get("X-Root-Request-ID"))
	assert.Equal(t, requestID, headers.Get("X-Trace-ID"))
	assert.Equal(t, requestID, headers.Get("X-B3-TraceId"))
	assert.Equal(t, headers.Get("X-Conversation-Message-ID")[:16], headers.Get("X-B3-SpanId"))
	assert.Equal(t, "1", headers.Get("X-B3-Sampled"))
	assert.Equal(t, deriveAccountStableID("user-1", "machine"), headers.Get("X-Machine-ID"))
	assert.Equal(t, deriveAccountStableID("user-1", "session"), headers.Get("X-Session-ID"))
}

func TestChatHeadersSwitchToGlobalShape(t *testing.T) {
	credential := &Credential{AccessToken: "at", RefreshToken: "rt", UID: "u", Realm: RealmGlobal}
	headers := http.Header{}
	ChatHeaders(headers, credential, ChatMeta{ConversationRequestID: "req"})
	assert.Equal(t, "en-US", headers.Get("Accept-Language"))
	assert.Equal(t, "https://www.workbuddy.ai", headers.Get("Origin"))
	assert.Contains(t, headers.Get("User-Agent"), "WorkBuddy AI/")
	assert.Equal(t, "www.workbuddy.ai", headers.Get("X-Domain"))
	assert.Equal(t, "1", headers.Get("X-No-Enterprise-Id"))
	assert.Empty(t, headers.Get("X-No-Department-Info"))

	billing := http.Header{}
	BillingHeaders(billing, credential)
	assert.Equal(t, "Bearer at", billing.Get("Authorization"))
	assert.Equal(t, "WorkBuddy/"+ClientVersion, billing.Get("User-Agent"))

	refresh := http.Header{}
	RefreshHeaders(refresh, credential)
	assert.Equal(t, "rt", refresh.Get("X-Refresh-Token"))
	assert.Equal(t, "plugin", refresh.Get("X-Auth-Refresh-Source"))
}

func TestConversationHeadersFallBackToGeneratedRequestID(t *testing.T) {
	headers := http.Header{}
	injectConversationHeaders(headers, ChatMeta{})
	assert.Len(t, headers.Get("X-Conversation-Request-ID"), 32)
	assert.Empty(t, headers.Get("X-Conversation-ID"))
	assert.Equal(t, headers.Get("X-Conversation-Request-ID"), headers.Get("X-B3-TraceId"))
}

func TestNormalizeStreamRebuildsFrames(t *testing.T) {
	source := strings.Join([]string{
		"data: {\"id\":\"chunk-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\"},\"finish_reason\":\"\"}],\"noise\":1}",
		"",
		"data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\",\"reasoning_content\":\"think\"},\"finish_reason\":\"\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":1}}",
		"",
		"data: {\"id\":\"chunk-1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}",
		"",
	}, "\n")
	out, err := readAll(NormalizeStream(strings.NewReader(source)))
	require.NoError(t, err)
	assert.Contains(t, out, "data: [DONE]")
	frames := strings.Split(strings.TrimSpace(out), "\n\n")
	require.GreaterOrEqual(t, len(frames), 4)
	var first map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frames[0], "data: ")), &first))
	assert.NotContains(t, first, "noise")
	assert.Equal(t, "chat.completion.chunk", first["object"])
	choices := first["choices"].([]any)
	assert.Equal(t, map[string]any{"role": "assistant"}, choices[0].(map[string]any)["delta"], "empty content is dropped")
	assert.Nil(t, choices[0].(map[string]any)["finish_reason"])
	var second map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frames[1], "data: ")), &second))
	assert.Equal(t, "chunk-1", second["id"], "the stream keeps one message id")
}

func TestNormalizeStreamKeepsErrorFramesAndAddsDone(t *testing.T) {
	source := "data: {\"error\":{\"code\":11128,\"message\":\"blocked\"}}\n\n"
	out, err := readAll(NormalizeStream(strings.NewReader(source)))
	require.NoError(t, err)
	assert.Contains(t, out, "\"code\":11128")
	assert.Contains(t, out, "data: [DONE]")
}

func TestNormalizeStreamKeepsToolNameOnFirstFragmentOnly(t *testing.T) {
	source := strings.Join([]string{
		"data: {\"id\":\"chunk-1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"type\":\"function\",\"function\":{\"name\":\"lookup\",\"arguments\":\"{\"}}]},\"finish_reason\":null}]}",
		"",
		"data: {\"id\":\"chunk-1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"name\":\"lookup\",\"arguments\":\"}\"}}]},\"finish_reason\":null}]}",
		"",
	}, "\n")
	out, err := readAll(NormalizeStream(strings.NewReader(source)))
	require.NoError(t, err)
	frames := strings.Split(strings.TrimSpace(out), "\n\n")
	var first, second map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frames[0], "data: ")), &first))
	require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frames[1], "data: ")), &second))
	firstFunction := first["choices"].([]any)[0].(map[string]any)["delta"].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)
	secondFunction := second["choices"].([]any)[0].(map[string]any)["delta"].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)
	assert.Equal(t, "lookup", firstFunction["name"])
	assert.NotContains(t, secondFunction, "name", "the name is sent once per tool call")
	assert.Equal(t, "}", secondFunction["arguments"])
}

func TestNormalizeStreamKeepsLegacyFunctionCall(t *testing.T) {
	source := "data: {\"id\":\"chunk-1\",\"choices\":[{\"index\":0,\"delta\":{\"function_call\":{\"name\":\"legacy\",\"arguments\":\"{}\"}},\"finish_reason\":null}]}\n\n"
	out, err := readAll(NormalizeStream(strings.NewReader(source)))
	require.NoError(t, err)
	assert.Contains(t, out, "legacy")
}

func TestNormalizeStreamHoldsToolCallUntilNameArrives(t *testing.T) {
	// The upstream sometimes sends the identity first and the function name in a
	// later fragment. A client that registers the call on the first fragment
	// would keep an empty name, so the relay holds the fragments back.
	source := strings.Join([]string{
		"data: {\"id\":\"chunk-1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-7\",\"type\":\"function\",\"function\":{\"name\":\"\",\"arguments\":\"\"}}]},\"finish_reason\":null}]}",
		"",
		"data: {\"id\":\"chunk-1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\\\"path\\\":\"}}]},\"finish_reason\":null}]}",
		"",
		"data: {\"id\":\"chunk-1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"name\":\"read_file\",\"arguments\":\"\\\"a.txt\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}",
		"",
	}, "\n")
	out, err := readAll(NormalizeStream(strings.NewReader(source)))
	require.NoError(t, err)

	var fragments []map[string]any
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "data: ") || strings.Contains(line, "[DONE]") {
			continue
		}
		var frame map[string]any
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &frame))
		choices, ok := frame["choices"].([]any)
		if !ok || len(choices) == 0 {
			continue
		}
		delta, ok := choices[0].(map[string]any)["delta"].(map[string]any)
		if !ok {
			continue
		}
		for _, rawCall := range toAnySlice(delta["tool_calls"]) {
			call, ok := rawCall.(map[string]any)
			if !ok {
				continue
			}
			fragments = append(fragments, call)
		}
	}
	require.Len(t, fragments, 2, "the held fragments arrive with the name, the rest follows")
	released := fragments[0]
	assert.Equal(t, "call-7", released["id"], "the held identity is released with the name")
	releasedFunction := released["function"].(map[string]any)
	assert.Equal(t, "read_file", releasedFunction["name"])
	assert.Equal(t, "{\"path\":", releasedFunction["arguments"])
	rest := fragments[1]["function"].(map[string]any)
	assert.NotContains(t, rest, "name", "the name is sent once")
	assert.Equal(t, "\"a.txt\"}", rest["arguments"])
	assert.Equal(t, "{\"path\":\"a.txt\"}", releasedFunction["arguments"].(string)+rest["arguments"].(string))
	assert.NotContains(t, out, `"name":""`, "an empty name never reaches the client")
}

func TestNormalizeStreamDropsToolCallWithoutName(t *testing.T) {
	source := strings.Join([]string{
		"data: {\"id\":\"chunk-1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-8\",\"type\":\"function\",\"function\":{\"arguments\":\"{}\"}}]},\"finish_reason\":null}]}",
		"",
		"data: {\"id\":\"chunk-1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}",
		"",
	}, "\n")
	out, err := readAll(NormalizeStream(strings.NewReader(source)))
	require.NoError(t, err)
	assert.NotContains(t, out, "call-8", "an unusable call is not forwarded")
	assert.NotContains(t, out, `"name":""`)
}

func TestNormalizeStreamKeepsMessageFormFrames(t *testing.T) {
	// Some upstream frames carry the whole message instead of a delta. Dropping
	// them leaves the client with a stream that never contained an answer.
	source := strings.Join([]string{
		"data: {\"id\":\"chunk-1\",\"choices\":[{\"index\":0,\"message\":{\"role\":\"assistant\",\"content\":\"done\"},\"finish_reason\":\"stop\"}]}",
		"",
	}, "\n")
	out, err := readAll(NormalizeStream(strings.NewReader(source)))
	require.NoError(t, err)
	assert.Contains(t, out, "\"content\":\"done\"")
	assert.NotContains(t, out, "without an answer")
}

func TestNormalizeStreamMatchesToolCallFragmentsByID(t *testing.T) {
	// Fragments without an index must not collapse into one call: the second
	// call would lose its name and strict clients would reject it.
	source := strings.Join([]string{
		"data: {\"id\":\"chunk-1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"id\":\"call-a\",\"type\":\"function\",\"function\":{\"name\":\"read_file\",\"arguments\":\"{\"}}]}}]}",
		"",
		"data: {\"id\":\"chunk-1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"id\":\"call-b\",\"type\":\"function\",\"function\":{\"name\":\"write_file\",\"arguments\":\"{}\"}}]}}]}",
		"",
	}, "\n")
	out, err := readAll(NormalizeStream(strings.NewReader(source)))
	require.NoError(t, err)
	assert.Contains(t, out, "\"name\":\"read_file\"")
	assert.Contains(t, out, "\"name\":\"write_file\"", "both calls keep their name")
	assert.Contains(t, out, "\"index\":0")
	assert.Contains(t, out, "\"index\":1")
}

func TestNormalizeStreamReportsStreamWithoutAnswer(t *testing.T) {
	source := strings.Join([]string{
		"data: {\"id\":\"chunk-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}",
		"",
	}, "\n")
	out, err := readAll(NormalizeStream(strings.NewReader(source)))
	require.NoError(t, err)
	assert.Contains(t, out, "upstream stream ended without an answer")
	assert.Contains(t, out, "data: [DONE]")
}

func TestNormalizeStreamKeepsReasoningOnlyStream(t *testing.T) {
	// A reasoning-only frame carries no answer, but the stream itself is valid:
	// the relay still reports it to the client instead of failing the turn here.
	source := strings.Join([]string{
		"data: {\"id\":\"chunk-1\",\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":\"thinking\"},\"finish_reason\":\"stop\"}]}",
		"",
	}, "\n")
	out, err := readAll(NormalizeStream(strings.NewReader(source)))
	require.NoError(t, err)
	assert.Contains(t, out, "thinking")
}

func toAnySlice(value any) []any {
	slice, _ := value.([]any)
	return slice
}

func TestAggregateStreamBuildsChatCompletion(t *testing.T) {
	source := strings.Join([]string{
		"data: {\"id\":\"chunk-1\",\"model\":\"glm-5.3\",\"created\":1790000000,\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hel\"},\"finish_reason\":null}]}",
		"",
		"data: {\"id\":\"chunk-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"lo\",\"reasoning_content\":\"why\"},\"finish_reason\":null}]}",
		"",
		"data: {\"id\":\"chunk-1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"type\":\"function\",\"function\":{\"name\":\"lookup\",\"arguments\":\"{\\\"a\\\"\"}}]},\"finish_reason\":null}]}",
		"",
		"data: {\"id\":\"chunk-1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\":1}\"}}]},\"finish_reason\":\"tool_calls\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":4}}",
		"",
		"data: [DONE]",
		"",
	}, "\n")
	raw, err := AggregateStream(strings.NewReader(source))
	require.NoError(t, err)
	var answer map[string]any
	require.NoError(t, json.Unmarshal(raw, &answer))
	assert.Equal(t, "chat.completion", answer["object"])
	assert.Equal(t, "chunk-1", answer["id"])
	assert.Equal(t, "glm-5.3", answer["model"])
	choice := answer["choices"].([]any)[0].(map[string]any)
	message := choice["message"].(map[string]any)
	assert.Equal(t, "hello", message["content"])
	assert.Equal(t, "why", message["reasoning_content"])
	calls := message["tool_calls"].([]any)
	require.Len(t, calls, 1)
	call := calls[0].(map[string]any)
	assert.Equal(t, "call-1", call["id"])
	assert.Equal(t, map[string]any{"name": "lookup", "arguments": "{\"a\":1}"}, call["function"])
	assert.Equal(t, "tool_calls", choice["finish_reason"])
	usage := answer["usage"].(map[string]any)
	assert.Equal(t, float64(14), usage["total_tokens"], "total tokens are filled in")
}

func TestAggregateStreamRejectsEmptyAndErrorStreams(t *testing.T) {
	_, err := AggregateStream(strings.NewReader("data: [DONE]\n\n"))
	require.ErrorIs(t, err, ErrEmptyStream)

	_, err = AggregateStream(strings.NewReader("data: {\"error\":{\"code\":6004,\"message\":\"limited\"}}\n\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "6004")
}

func TestAggregateStreamDropsToolCallWithoutName(t *testing.T) {
	source := strings.Join([]string{
		"data: {\"id\":\"chunk-1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-9\",\"type\":\"function\",\"function\":{\"arguments\":\"{}\"}}]},\"finish_reason\":null}]}",
		"",
		"data: {\"id\":\"chunk-1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}",
		"",
	}, "\n")
	raw, err := AggregateStream(strings.NewReader(source))
	require.NoError(t, err)
	var answer map[string]any
	require.NoError(t, json.Unmarshal(raw, &answer))
	message := answer["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
	assert.NotContains(t, message, "tool_calls")
}

func TestModelsMergesBothCatalogs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case ConfigPath:
			require.Equal(t, "Bearer at-1", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(`{"code":0,"data":{"models":[
				{"id":"glm-5.3","name":"GLM 5.3","maxInputTokens":200000,"maxOutputTokens":8000},
				{"id":"nes-embed","maxOutputTokens":1000},
				{"id":"deepseek-v4-flash","maxInputTokens":1000000,"reasoning":{"supportedEfforts":["high"],"defaultEffort":"high"}}
			]}}`))
		case EnterpriseModelsCN:
			_, _ = w.Write([]byte(`{"code":0,"data":{"models":[
				{"id":"glm-5.3","name":"GLM 5.3 legacy","maxInputTokens":190000},
				{"id":"kimi-k3","maxInputTokens":1048576,"tags":["badge:free"]},
				{"id":"completion-stub","maxOutputTokens":512},
				{"id":"image-gen","tags":["text-to-image"]},
				{"id":"disabled-model","disabled":true,"maxOutputTokens":4096}
			],"agents":[{"name":"cli","models":["glm-5.3","kimi-k3","completion-stub","image-gen","disabled-model"]}]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	credential := &Credential{AccessToken: "at-1", RefreshToken: "rt-1"}
	infos, err := Models(context.Background(), server.Client(), server.URL, credential)
	require.NoError(t, err)
	ids := make([]string, 0, len(infos))
	for _, info := range infos {
		ids = append(ids, info.ID)
	}
	assert.Equal(t, []string{"glm-5.3", "deepseek-v4-flash", "kimi-k3"}, ids)
	assert.Equal(t, int64(200000), infos[0].ContextWindow, "the configuration catalog wins")
	assert.Equal(t, []string{"high"}, infos[1].Efforts)
}

func TestFetchCreditsAggregatesPackages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, BillingMeterPath, r.URL.Path)
		require.Equal(t, "Bearer at-1", r.Header.Get("Authorization"))
		require.Equal(t, "WorkBuddy/"+ClientVersion, r.Header.Get("User-Agent"))
		_, _ = w.Write([]byte(`{"code":0,"data":{"Response":{"Data":{"Accounts":[
			{"PackageName":"monthly","CycleCapacitySize":1000,"CycleCapacityRemain":250,"CycleCapacityUsed":750,"CycleEndTime":"2026-10-01 00:00:00"},
			{"PackageName":"bonus","CapacitySize":100,"CapacityRemain":100,"CapacityUsed":0}
		]}}}}`))
	}))
	defer server.Close()

	credential := &Credential{AccessToken: "at-1", RefreshToken: "rt-1"}
	credits, err := FetchCredits(context.Background(), server.Client(), server.URL, credential)
	require.NoError(t, err)
	assert.Equal(t, int64(350), credits.Remain)
	assert.Equal(t, int64(750), credits.Used)
	assert.Equal(t, int64(1100), credits.Size)
	require.Len(t, credits.Packages, 2)
	assert.Equal(t, "2026-10-01 00:00:00", credits.Packages[0].EndTime)
}

func TestRefreshRotatesTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, RefreshPath, r.URL.Path)
		assert.Equal(t, "rt-old", r.Header.Get("X-Refresh-Token"))
		assert.Equal(t, "plugin", r.Header.Get("X-Auth-Refresh-Source"))
		_, _ = w.Write([]byte(`{"code":0,"data":{"accessToken":"at-new","refreshToken":"rt-new","expiresIn":5184000,"domain":"www.workbuddy.ai"}}`))
	}))
	defer server.Close()

	credential := &Credential{AccessToken: "at-old", RefreshToken: "rt-old", UID: "u-1"}
	refreshed, err := Refresh(context.Background(), server.Client(), server.URL, credential)
	require.NoError(t, err)
	assert.Equal(t, "at-new", refreshed.AccessToken)
	assert.Equal(t, "rt-new", refreshed.RefreshToken)
	assert.Equal(t, "www.workbuddy.ai", refreshed.Domain)
	assert.True(t, refreshed.ExpiresAt > time.Now().Add(59*24*time.Hour).Unix())
	assert.Equal(t, "rt-old", credential.RefreshToken, "the caller owns persistence")
}

func TestRefreshRejectsMissingToken(t *testing.T) {
	_, err := Refresh(context.Background(), http.DefaultClient, "", &Credential{AccessToken: "at"})
	require.Error(t, err)
}

func TestCacheKeyIsolatesAccounts(t *testing.T) {
	first := CacheKey("uid-aaaaaaaa", "conv-1")
	second := CacheKey("uid-bbbbbbbb", "conv-1")
	assert.NotEqual(t, first, second)
	assert.Equal(t, first, CacheKey("uid-aaaaaaaa", "conv-1"))
	assert.True(t, strings.HasPrefix(first, "wb2a-uid-aaaa-"))
}

func readAll(reader interface{ Read([]byte) (int, error) }) (string, error) {
	buf := make([]byte, 0, 4096)
	chunk := make([]byte, 512)
	for {
		n, err := reader.Read(chunk)
		buf = append(buf, chunk[:n]...)
		if err != nil {
			if err.Error() == "EOF" {
				return string(buf), nil
			}
			return string(buf), err
		}
	}
}

func TestNormalizeStreamReportsDroppedToolCallAsFailure(t *testing.T) {
	// A stream whose only answer is a tool call without a name is unusable: the
	// client would end up with an empty turn, so the relay reports it instead.
	source := strings.Join([]string{
		"data: {\"id\":\"chunk-1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-9\",\"type\":\"function\",\"function\":{\"arguments\":\"{\\\"cmd\\\":\\\"ls\\\"}\"}}]},\"finish_reason\":null}]}",
		"",
		"data: {\"id\":\"chunk-1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}",
		"",
	}, "\n")
	out, err := readAll(NormalizeStream(strings.NewReader(source)))
	require.NoError(t, err)
	assert.Contains(t, out, "upstream stream ended without an answer")
	assert.NotContains(t, out, "call-9")
}
