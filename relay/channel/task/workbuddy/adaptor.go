// Package workbuddy relays WorkBuddy (CodeBuddy) media generation tasks.
//
// The gateway publishes the media models in the same catalog as the chat
// models, but serves them on a separate JSON API:
//
//	POST /v2/videos/generations  {model, prompt}   -> {data:{id, status}}
//	POST /v2/videos/tasks        {task_id}         -> {data:{status, data:[{url}]}}
//
// The chat endpoint answers those model ids with "Backend [mps] is not
// supported", so video requests only work through this task adaptor.
package workbuddy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	workbuddyapi "github.com/QuantumNous/new-api/pkg/workbuddy"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

const (
	ChannelName = "workbuddy"

	videoSubmitPath = "/v2/videos/generations"
	videoQueryPath  = "/v2/videos/tasks"
)

// ModelList holds the media models the gateway serves for video generation.
// The gateway validates the model name on submit, so the list stays short.
var ModelList = []string{
	"seedance-2.5",
}

// ============================
// Request / response structures
// ============================

type requestPayload struct {
	Model  string   `json:"model"`
	Prompt string   `json:"prompt"`
	Images []string `json:"images,omitempty"`
}

type submitResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		CreatedAt int64  `json:"created_at"`
	} `json:"data"`
}

type taskResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		ID          string `json:"id"`
		Status      string `json:"status"`
		CreatedAt   int64  `json:"created_at"`
		CompletedAt int64  `json:"completed_at"`
		// The finished artifacts live in a nested array named data.
		Data []struct {
			URL        string `json:"url"`
			Resolution string `json:"resolution"`
		} `json:"data"`
		Size  string `json:"size"`
		Usage struct {
			TotalTokens int     `json:"total_tokens"`
			VideoTokens int     `json:"output_video_tokens"`
			Credit      float64 `json:"credit"`
			Coefficient int     `json:"output_video_coefficient"`
		} `json:"usage"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	} `json:"data"`
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	baseURL     string
	apiKey      string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.apiKey = info.ApiKey
	a.baseURL = resolveBaseURL(info.ChannelBaseUrl, info.ApiKey)
}

// ValidateRequestAndSetAction parses body, validates fields and sets default action.
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}

// BuildRequestURL constructs the upstream URL.
func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	if a.baseURL == "" {
		return "", fmt.Errorf("WorkBuddy base url is empty")
	}
	return a.baseURL + videoSubmitPath, nil
}

// BuildRequestHeader sets the WorkBuddy account headers and renews the token
// when it is about to expire.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	credential, err := currentCredential(c, info, a.apiKey)
	if err != nil {
		return err
	}
	workbuddyapi.MediaHeaders(req.Header, credential)
	return nil
}

// BuildRequestBody converts the client request into the media API shape.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}
	body := requestPayload{
		Model:  req.Model,
		Prompt: req.Prompt,
		Images: req.Images,
	}
	if info.IsModelMapped {
		body.Model = info.UpstreamModelName
	} else {
		info.UpstreamModelName = body.Model
	}
	data, err := common.Marshal(body)
	if err != nil {
		return nil, errors.Wrap(err, "marshal request body failed")
	}
	return bytes.NewReader(data), nil
}

// DoRequest delegates to the shared task request helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse reads the submit answer and reports the upstream task id.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	var parsed submitResponse
	if err := common.Unmarshal(responseBody, &parsed); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}
	if parsed.Code != 0 {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("code=%d msg=%s", parsed.Code, parsed.Msg), "upstream_error", http.StatusBadRequest)
		return "", responseBody, taskErr
	}
	if parsed.Data.ID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName

	c.JSON(http.StatusOK, ov)
	return parsed.Data.ID, responseBody, nil
}

// FetchTask reads one upstream task state. The polling loop passes the channel
// base url, which is the CN default for accounts that did not configure one, so
// the realm of the credential decides the host again.
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	credential, err := workbuddyapi.ParseCredential(key)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(baseUrl) == "" || baseUrl == workbuddyapi.ChatBaseCN {
		baseUrl = credential.ChatBase()
	}
	payload, err := common.Marshal(map[string]any{"task_id": taskID})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseUrl, "/")+videoQueryPath, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	workbuddyapi.MediaHeaders(req.Header, credential)
	client, err := workbuddyapi.NewUpstreamClient(proxy)
	if err != nil {
		return nil, fmt.Errorf("new upstream client failed: %w", err)
	}
	return client.Do(req)
}

// ParseTaskResult maps the upstream task state onto the internal one.
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var parsed taskResponse
	if err := common.Unmarshal(respBody, &parsed); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}
	if parsed.Code != 0 {
		return nil, fmt.Errorf("code=%d msg=%s", parsed.Code, parsed.Msg)
	}

	result := &relaycommon.TaskInfo{
		Code: 0,
	}
	switch parsed.Data.Status {
	case "queued":
		result.Status = model.TaskStatusQueued
		result.Progress = "10%"
	case "in_progress":
		result.Status = model.TaskStatusInProgress
		result.Progress = "50%"
	case "completed":
		result.Status = model.TaskStatusSuccess
		result.Progress = "100%"
		result.Url = firstVideoURL(parsed)
		result.TotalTokens = parsed.Data.Usage.TotalTokens
		result.CompletionTokens = parsed.Data.Usage.VideoTokens
		if result.Url == "" {
			logger.LogError(context.Background(), fmt.Sprintf("workbuddy video task %s completed without a url: %s", parsed.Data.ID, common.LocalLogPreview(string(respBody))))
		}
	case "failed":
		result.Status = model.TaskStatusFailure
		result.Progress = "100%"
		result.Reason = parsed.Data.Error.Message
	default:
		result.Status = model.TaskStatusInProgress
		result.Progress = "30%"
	}
	return result, nil
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

// ConvertToOpenAIVideo renders the stored task as an OpenAI video object.
func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var parsed taskResponse
	if err := common.Unmarshal(originTask.Data, &parsed); err != nil {
		return nil, errors.Wrap(err, "unmarshal workbuddy task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.TaskID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.SetMetadata("url", firstVideoURL(parsed))
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt
	openAIVideo.Model = originTask.Properties.OriginModelName
	if parsed.Data.Status == "failed" {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: parsed.Data.Error.Message,
		}
	}
	return common.Marshal(openAIVideo)
}

// currentCredential resolves the stored credential, renewing the access token
// through the channel lock when it is about to expire.
func currentCredential(c *gin.Context, info *relaycommon.RelayInfo, rawKey string) (*workbuddyapi.Credential, error) {
	if c != nil && c.Request != nil && info != nil && info.ChannelId > 0 {
		if credential, err := service.ResolveWorkBuddyCredential(c.Request.Context(), info.ChannelId, ""); err == nil && credential != nil {
			return credential, nil
		} else if err != nil {
			return nil, err
		}
	}
	credential, err := workbuddyapi.ParseCredential(rawKey)
	if err != nil {
		return nil, err
	}
	return credential, nil
}

// resolveBaseURL picks the upstream host: an operator configured base url wins,
// the CN default counts as unset because it is the value the channel page saves
// when the field is left empty.
func resolveBaseURL(configured, rawKey string) string {
	configured = strings.TrimRight(strings.TrimSpace(configured), "/")
	if configured != "" && configured != workbuddyapi.ChatBaseCN {
		return configured
	}
	credential, err := workbuddyapi.ParseCredential(rawKey)
	if err != nil {
		return configured
	}
	return credential.ChatBase()
}

// firstVideoURL reads the artifact url of a finished task.
func firstVideoURL(parsed taskResponse) string {
	for _, artifact := range parsed.Data.Data {
		if strings.TrimSpace(artifact.URL) != "" {
			return artifact.URL
		}
	}
	return ""
}
