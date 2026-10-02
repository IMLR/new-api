package workbuddy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTaskResultMapsUpstreamStates(t *testing.T) {
	adaptor := &TaskAdaptor{}

	queued, err := adaptor.ParseTaskResult([]byte(`{"code":0,"data":{"id":"t1","status":"queued"}}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusQueued, queued.Status)

	running, err := adaptor.ParseTaskResult([]byte(`{"code":0,"data":{"id":"t1","status":"in_progress"}}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusInProgress, running.Status)

	completed, err := adaptor.ParseTaskResult([]byte(`{"code":0,"data":{"id":"t1","status":"completed",
		"data":[{"url":"https://cdn.example/video.mp4","resolution":"1280x720"}],
		"usage":{"total_tokens":216900,"output_video_tokens":216900,"credit":208.01},"size":"1280x720"}}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, completed.Status)
	assert.Equal(t, "100%", completed.Progress)
	assert.Equal(t, "https://cdn.example/video.mp4", completed.Url)
	assert.Equal(t, 216900, completed.TotalTokens)

	failed, err := adaptor.ParseTaskResult([]byte(`{"code":0,"data":{"id":"t1","status":"failed","error":{"message":"missing content"}}}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusFailure, failed.Status)
	assert.Equal(t, "missing content", failed.Reason)
}

func TestParseTaskResultRejectsUpstreamError(t *testing.T) {
	adaptor := &TaskAdaptor{}
	_, err := adaptor.ParseTaskResult([]byte(`{"code":14407,"msg":"Video model [nope] route config not found"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "14407")
}

func TestDoResponseReadsSubmitAnswer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(
			`{"code":0,"msg":"OK","data":{"id":"2600029973-AigcVideo-abc","status":"queued","created_at":1790941248}}`)),
	}
	info := &relaycommon.RelayInfo{
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task_public_1"},
		OriginModelName: "seedance-2.5",
	}

	adaptor := &TaskAdaptor{}
	taskID, taskData, taskErr := adaptor.DoResponse(c, resp, info)

	require.Nil(t, taskErr)
	assert.Equal(t, "2600029973-AigcVideo-abc", taskID)
	assert.Contains(t, string(taskData), "queued")
	assert.Contains(t, recorder.Body.String(), "task_public_1")
}

func TestDoResponseReportsUpstreamError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(
			`{"code":14407,"msg":"Create video failed with error: Video model [nope] route config not found"}`)),
	}

	adaptor := &TaskAdaptor{}
	_, _, taskErr := adaptor.DoResponse(c, resp, &relaycommon.RelayInfo{})

	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "14407")
}

func TestBuildRequestBodyUsesPromptAndModelMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Model:  "seedance-2.5",
		Prompt: "a red ball on a table",
		Images: []string{"https://cdn.example/first.png"},
	})
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{IsModelMapped: true, UpstreamModelName: "seedance-2.5"},
	}

	adaptor := &TaskAdaptor{}
	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	raw, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"seedance-2.5","prompt":"a red ball on a table","images":["https://cdn.example/first.png"]}`, string(raw))
}

func TestResolveBaseURLFollowsCredentialRealm(t *testing.T) {
	globalKey := `{"accessToken":"at","refreshToken":"rt","realm":"global","domain":"www.workbuddy.ai"}`
	cnKey := `{"accessToken":"at","refreshToken":"rt","realm":"cn"}`

	assert.Equal(t, "https://www.workbuddy.ai", resolveBaseURL("", globalKey))
	assert.Equal(t, "https://www.workbuddy.ai", resolveBaseURL("https://copilot.tencent.com", globalKey))
	assert.Equal(t, "https://copilot.tencent.com", resolveBaseURL("", cnKey))
	assert.Equal(t, "https://proxy.example", resolveBaseURL("https://proxy.example", globalKey))
}

func TestModelListCoversSeedance(t *testing.T) {
	assert.Contains(t, ModelList, "seedance-2.5")
}
