package workbuddy

import (
	"io"
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
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesRequestPreservesInstructionsToolsAndParameters(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeResponses, RelayFormat: types.RelayFormatOpenAIResponses,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeWorkBuddy, UpstreamModelName: "gpt-6-luna"},
	}
	var request dto.OpenAIResponsesRequest
	require.NoError(t, common.UnmarshalJsonStr(`{
		"model":"gpt-6-luna","instructions":"Only output JSON.","stream":true,
		"input":[
			{"role":"user","content":[{"type":"input_text","text":"Find a film."}]},
			{"type":"function_call","call_id":"call_1","name":"search","arguments":"{\"title\":\"Arrival\"}"},
			{"type":"function_call_output","call_id":"call_1","output":"Found."}
		],
		"tools":[{"type":"function","name":"search","parameters":{"type":"object","properties":{}}}],
		"temperature":0,"top_p":0,"max_output_tokens":64,"parallel_tool_calls":false,
		"reasoning":{"effort":"high"},"text":{"format":{"type":"json_object"}}
	}`, &request))
	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertOpenAIResponsesRequest(testContext(t, nil), info, request)
	require.NoError(t, err)
	raw, err := common.Marshal(converted)
	require.NoError(t, err)
	// The outbound stage must not convert an already converted request twice.
	raw, err = adaptor.chatRequestBody(testContext(t, nil), info, raw)
	require.NoError(t, err)
	prepared := adaptor.prepareBody(raw, info, &workbuddyapi.Credential{Realm: workbuddyapi.RealmGlobal})
	var payload map[string]any
	require.NoError(t, common.Unmarshal(prepared, &payload))
	assert.NotContains(t, payload, "instructions")
	assert.NotContains(t, payload, "input")
	assert.Equal(t, float64(0), payload["temperature"])
	assert.Equal(t, float64(0), payload["top_p"])
	assert.Equal(t, false, payload["parallel_tool_calls"])
	assert.Equal(t, float64(64), payload["max_tokens"])
	assert.Equal(t, "high", payload["reasoning_effort"])
	assert.Equal(t, map[string]any{"type": "json_object"}, payload["response_format"])
	messages := payload["messages"].([]any)
	require.Len(t, messages, 4)
	assert.Equal(t, map[string]any{"role": "system", "content": "Only output JSON."}, messages[0])
	assert.Equal(t, "Find a film.", messages[1].(map[string]any)["content"])
	call := messages[2].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)
	assert.Equal(t, "call_1", call["id"])
	assert.Equal(t, map[string]any{"name": "search", "arguments": `{"title":"Arrival"}`}, call["function"])
	assert.Equal(t, "call_1", messages[3].(map[string]any)["tool_call_id"])
	assert.Equal(t, "Found.", messages[3].(map[string]any)["content"])
	tool := payload["tools"].([]any)[0].(map[string]any)
	assert.Equal(t, "search", tool["function"].(map[string]any)["name"])
}

func TestPassThroughResponsesStillConvertToChatOnRetry(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeResponses, RelayFormat: types.RelayFormatOpenAIResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeWorkBuddy, UpstreamModelName: "gpt-6-luna",
			ChannelSetting: dto.ChannelSettings{PassThroughBodyEnabled: true},
		},
	}
	adaptor := &Adaptor{}
	// A later channel attempt reuses metadata but receives the original
	// pass-through body. Both attempts must preserve its instructions.
	for attempt := 0; attempt < 2; attempt++ {
		converted, err := adaptor.chatRequestBody(testContext(t, nil), info, []byte(`{"model":"gpt-6-luna","instructions":"Caller rule.","input":"hello"}`))
		require.NoError(t, err)
		prepared := adaptor.prepareBody(converted, info, &workbuddyapi.Credential{Realm: workbuddyapi.RealmGlobal})
		var payload map[string]any
		require.NoError(t, common.Unmarshal(prepared, &payload))
		messages := payload["messages"].([]any)
		require.Len(t, messages, 2)
		assert.Equal(t, map[string]any{"role": "system", "content": "Caller rule."}, messages[0])
		assert.Equal(t, "hello", messages[1].(map[string]any)["content"])
		assert.Equal(t, types.RelayFormatOpenAI, info.GetFinalRequestRelayFormat())
	}
}

func TestResponsesRejectsNonGPTWorkBuddyModels(t *testing.T) {
	for _, model := range []string{"claude-opus-4.6", "fast-model", "gpt-image-2.5-sunburst"} {
		t.Run(model, func(t *testing.T) {
			info := &relaycommon.RelayInfo{
				RelayMode: relayconstant.RelayModeResponses, RelayFormat: types.RelayFormatOpenAIResponses,
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelType: constant.ChannelTypeWorkBuddy, UpstreamModelName: model,
					ChannelSetting: dto.ChannelSettings{PassThroughBodyEnabled: true},
				},
			}
			adaptor := &Adaptor{}
			_, err := adaptor.ConvertOpenAIResponsesRequest(testContext(t, nil), info, dto.OpenAIResponsesRequest{Model: model})
			require.ErrorContains(t, err, "only supports GPT chat models")
			raw, err := common.Marshal(dto.OpenAIResponsesRequest{Model: model})
			require.NoError(t, err)
			_, err = adaptor.chatRequestBody(testContext(t, nil), info, raw)
			require.ErrorContains(t, err, "only supports GPT chat models")
		})
	}
}

func TestResponsesReplyConvertsTextToolsAndUsage(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "JSON", true: "SSE"}[stream], func(t *testing.T) {
			body := "data: {\"id\":\"chat_1\",\"model\":\"gpt-6-luna\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Found.\"},\"finish_reason\":null}]}\n\n" +
				"data: {\"id\":\"chat_1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"search\",\"arguments\":\"{\\\"q\\\":\\\"film\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\n" +
				"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":4,\"total_tokens\":16}}\n\ndata: [DONE]\n\n"
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Set(common.RequestIdKey, "responses-workbuddy-test")
			info := &relaycommon.RelayInfo{
				RelayMode: relayconstant.RelayModeResponses, RelayFormat: types.RelayFormatOpenAIResponses,
				IsStream: stream, DisablePing: true,
				ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeWorkBuddy, UpstreamModelName: "gpt-6-luna"},
			}
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}
			if !stream {
				merged, err := workbuddyapi.AggregateStream(resp.Body)
				require.NoError(t, err)
				resp = jsonResponse(resp, merged)
			}
			usage, apiErr := (&Adaptor{}).DoResponse(c, resp, info)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, 16, usage.(*dto.Usage).TotalTokens)
			var response dto.OpenAIResponsesResponse
			if stream {
				assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
				assert.Contains(t, recorder.Body.String(), "event: response.output_text.delta")
				assert.Contains(t, recorder.Body.String(), "event: response.function_call_arguments.delta")
				for _, line := range strings.Split(recorder.Body.String(), "\n") {
					if !strings.HasPrefix(line, "data:") {
						continue
					}
					var event dto.ResponsesStreamResponse
					require.NoError(t, common.UnmarshalJsonStr(strings.TrimSpace(strings.TrimPrefix(line, "data:")), &event))
					if event.Type == "response.completed" {
						require.NotNil(t, event.Response)
						response = *event.Response
					}
				}
			} else {
				require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
				assert.Equal(t, "response", response.Object)
			}
			require.Len(t, response.Output, 2)
			assert.Equal(t, "Found.", response.Output[0].Content[0].Text)
			assert.Equal(t, "function_call", response.Output[1].Type)
			assert.Equal(t, "call_1", response.Output[1].CallId)
			assert.Equal(t, "search", response.Output[1].Name)
			require.NotNil(t, response.Usage)
			assert.Equal(t, 12, response.Usage.InputTokens)
			assert.Equal(t, 4, response.Usage.OutputTokens)
		})
	}
}
