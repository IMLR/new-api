package relay

import (
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type chatViaResponsesTestAdaptor struct {
	convertedResponsesRequest dto.OpenAIResponsesRequest
	convertInfoIsStream       bool
	doRequestInfoIsStream     bool
	responseContentType       string
}

func (a *chatViaResponsesTestAdaptor) Init(*relaycommon.RelayInfo) {}

func (a *chatViaResponsesTestAdaptor) GetRequestURL(*relaycommon.RelayInfo) (string, error) {
	return "", nil
}

func (a *chatViaResponsesTestAdaptor) SetupRequestHeader(*gin.Context, *http.Header, *relaycommon.RelayInfo) error {
	return nil
}

func (a *chatViaResponsesTestAdaptor) ConvertOpenAIRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeneralOpenAIRequest) (any, error) {
	return nil, errors.New("unused")
}

func (a *chatViaResponsesTestAdaptor) ConvertRerankRequest(*gin.Context, int, dto.RerankRequest) (any, error) {
	return nil, errors.New("unused")
}

func (a *chatViaResponsesTestAdaptor) ConvertEmbeddingRequest(*gin.Context, *relaycommon.RelayInfo, dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("unused")
}

func (a *chatViaResponsesTestAdaptor) ConvertAudioRequest(*gin.Context, *relaycommon.RelayInfo, dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("unused")
}

func (a *chatViaResponsesTestAdaptor) ConvertImageRequest(*gin.Context, *relaycommon.RelayInfo, dto.ImageRequest) (any, error) {
	return nil, errors.New("unused")
}

func (a *chatViaResponsesTestAdaptor) ConvertOpenAIResponsesRequest(_ *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	a.convertedResponsesRequest = request
	a.convertInfoIsStream = info.IsStream
	return request, nil
}

func (a *chatViaResponsesTestAdaptor) DoRequest(_ *gin.Context, info *relaycommon.RelayInfo, _ io.Reader) (any, error) {
	a.doRequestInfoIsStream = info.IsStream
	contentType := a.responseContentType
	if contentType == "" {
		contentType = "application/json"
	}
	body := `{"id":"resp_1","model":"gpt-test","created_at":1710000000,"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`
	if a.convertedResponsesRequest.Stream != nil && *a.convertedResponsesRequest.Stream {
		body = strings.Join([]string{
			`data: {"type":"response.output_text.delta","delta":"ok"}`,
			`data: {"type":"response.done","response":{"model":"gpt-test","status":"completed","usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`,
			`data: [DONE]`,
			``,
		}, "\n")
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

func (a *chatViaResponsesTestAdaptor) DoResponse(*gin.Context, *http.Response, *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	return nil, nil
}

func (a *chatViaResponsesTestAdaptor) GetModelList() []string {
	return nil
}

func (a *chatViaResponsesTestAdaptor) GetChannelName() string {
	return "test"
}

func (a *chatViaResponsesTestAdaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("unused")
}

func (a *chatViaResponsesTestAdaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("unused")
}

func newChatViaResponsesTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(common.RequestIdKey, "chat-via-responses-test")
	return c, recorder
}

func TestIsResponsesEventStreamContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		{name: "plain", contentType: "text/event-stream", want: true},
		{name: "mixed case with charset", contentType: "Text/Event-Stream; charset=utf-8", want: true},
		{name: "json", contentType: "application/json", want: false},
		{name: "empty", contentType: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isResponsesEventStreamContentType(tt.contentType))
		})
	}
}

func TestRecalcQuotaFromRatiosIgnoresInvalidMultipliers(t *testing.T) {
	info := &relaycommon.RelayInfo{
		PriceData: types.PriceData{
			Quota: 100,
		},
	}
	info.PriceData.AddOtherRatio("duration", 2)

	quota, ok := recalcQuotaFromRatios(info, map[string]float64{
		"duration": 3,
		"zero":     0,
		"negative": -1,
		"nan":      math.NaN(),
		"inf":      math.Inf(1),
	})

	require.True(t, ok)
	assert.Equal(t, 150, quota)
	assert.True(t, info.PriceData.HasOtherRatio("duration"))
}

func TestChatCompletionsViaResponsesForcesCodexUpstreamStreamOnly(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	streamFalse := false
	streamTrue := true

	tests := []struct {
		name                   string
		channelType            int
		clientStream           bool
		requestStream          *bool
		wantUpstreamStream     *bool
		upstreamContentType    string
		wantDownstreamSSE      bool
		wantInfoIsStreamDuring bool
	}{
		{
			name:                   "codex non-stream explicit false forces upstream stream",
			channelType:            constant.ChannelTypeCodex,
			clientStream:           false,
			requestStream:          &streamFalse,
			wantUpstreamStream:     &streamTrue,
			upstreamContentType:    "application/json",
			wantDownstreamSSE:      false,
			wantInfoIsStreamDuring: false,
		},
		{
			name:                   "codex non-stream omitted stream forces upstream stream",
			channelType:            constant.ChannelTypeCodex,
			clientStream:           false,
			requestStream:          nil,
			wantUpstreamStream:     &streamTrue,
			upstreamContentType:    "application/json",
			wantDownstreamSSE:      false,
			wantInfoIsStreamDuring: false,
		},
		{
			name:                   "codex stream keeps streaming",
			channelType:            constant.ChannelTypeCodex,
			clientStream:           true,
			requestStream:          &streamTrue,
			wantUpstreamStream:     &streamTrue,
			upstreamContentType:    "text/event-stream",
			wantDownstreamSSE:      true,
			wantInfoIsStreamDuring: true,
		},
		{
			name:                   "non-codex non-stream keeps original upstream stream",
			channelType:            constant.ChannelTypeOpenAI,
			clientStream:           false,
			requestStream:          &streamFalse,
			wantUpstreamStream:     &streamFalse,
			upstreamContentType:    "application/json",
			wantDownstreamSSE:      false,
			wantInfoIsStreamDuring: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, recorder := newChatViaResponsesTestContext(t)
			info := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelType:       tt.channelType,
					UpstreamModelName: "gpt-test",
				},
				IsStream:           tt.clientStream,
				RelayFormat:        types.RelayFormatOpenAI,
				ShouldIncludeUsage: true,
				DisablePing:        true,
			}
			adaptor := &chatViaResponsesTestAdaptor{responseContentType: tt.upstreamContentType}
			request := &dto.GeneralOpenAIRequest{
				Model:  "gpt-test",
				Stream: tt.requestStream,
				Messages: []dto.Message{
					{Role: "user", Content: "reply with ok"},
				},
			}

			usage, newAPIError := chatCompletionsViaResponses(c, info, adaptor, request)

			require.Nil(t, newAPIError)
			require.NotNil(t, usage)
			require.Equal(t, 2, usage.TotalTokens)
			assert.Equal(t, tt.wantInfoIsStreamDuring, adaptor.convertInfoIsStream)
			assert.Equal(t, tt.wantInfoIsStreamDuring, adaptor.doRequestInfoIsStream)
			if tt.wantUpstreamStream == nil {
				assert.Nil(t, adaptor.convertedResponsesRequest.Stream)
			} else {
				require.NotNil(t, adaptor.convertedResponsesRequest.Stream)
				assert.Equal(t, *tt.wantUpstreamStream, *adaptor.convertedResponsesRequest.Stream)
			}
			if tt.wantDownstreamSSE {
				assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
				assert.Contains(t, recorder.Body.String(), "data:")
				assert.Contains(t, recorder.Body.String(), `data: [DONE]`)
			} else {
				assert.NotEqual(t, "text/event-stream", recorder.Header().Get("Content-Type"))
				assert.NotContains(t, recorder.Body.String(), "data:")
				assert.Contains(t, recorder.Body.String(), `"object":"chat.completion"`)
				assert.Contains(t, recorder.Body.String(), `"content":"ok"`)
			}
		})
	}
}

func TestRecalcQuotaFromRatiosRejectsAllInvalidAdjustedRatios(t *testing.T) {
	info := &relaycommon.RelayInfo{
		PriceData: types.PriceData{
			Quota: 100,
		},
	}
	info.PriceData.AddOtherRatio("duration", 2)

	quota, ok := recalcQuotaFromRatios(info, map[string]float64{
		"zero":     0,
		"negative": -1,
		"nan":      math.NaN(),
		"inf":      math.Inf(1),
	})

	require.False(t, ok)
	assert.Equal(t, 0, quota)
	assert.True(t, info.PriceData.HasOtherRatio("duration"))
}
