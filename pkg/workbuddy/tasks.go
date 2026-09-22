package workbuddy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Endpoints of the account tasks. They live on two hosts: billing tasks on the
// billing host, growth tasks on the chat host.
const (
	DailyCheckinPath = "/v2/billing/meter/daily-checkin"
	ReportPath       = "/v2/report"

	TravelStatusPath = "/activity/growth/buddy/travel/status"
	TravelDepartPath = "/activity/growth/buddy/travel/depart"
	TravelClaimPath  = "/activity/growth/buddy/travel/claim"
)

// TaskOutcome is the normalized result of one task run. A task that the
// upstream no longer offers, or that was already completed today, is not a
// failure: the account simply has nothing to do.
type TaskOutcome struct {
	// Status is "done" or "skipped".
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// Task status values used by the outcome and by the stored state.
const (
	TaskStatusDone    = "done"
	TaskStatusSkipped = "skipped"
)

// taskJSON posts to one of the task endpoints and unwraps the envelope. The
// billing host serves the billing paths, the chat host the growth paths.
func taskJSON(ctx context.Context, client *http.Client, method, url string, credential *Credential, body any) (json.RawMessage, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, err
	}
	BillingHeaders(request.Header, credential)
	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	return readEnvelope(resp)
}

// CheckinTask signs the account in for the day. The upstream answers with a
// business code once the day is already collected.
func CheckinTask(ctx context.Context, client *http.Client, base string, credential *Credential) (TaskOutcome, error) {
	if credential == nil {
		return TaskOutcome{}, fmt.Errorf("workbuddy credential unavailable")
	}
	if base == "" {
		base = credential.BillingBase()
	}
	_, err := taskJSON(ctx, client, http.MethodPost, strings.TrimRight(base, "/")+DailyCheckinPath, credential, map[string]any{})
	if err != nil {
		return ClassifyTaskError(err)
	}
	return TaskOutcome{Status: TaskStatusDone, Message: "checked in"}, nil
}

// chatRequestEvent mirrors the client's chat_request_send event. The upstream
// drops the report silently when the account id is missing, so every field the
// client sends is kept.
type chatRequestEvent struct {
	EventCode             string `json:"eventCode"`
	Timestamp             int64  `json:"timestamp"`
	ReportDelay           int    `json:"reportDelay"`
	Mode                  string `json:"mode"`
	ConversationID        string `json:"conversationId"`
	RequestID             string `json:"requestId"`
	InputLength           int    `json:"inputLength"`
	RequestModelID        string `json:"requestModelId"`
	RequestModelName      string `json:"requestModelName"`
	IsPlan                bool   `json:"isPlan"`
	IsAutoExecuteTerminal bool   `json:"isAutoExecuteTerminal"`
	IsAutoModify          bool   `json:"isAutoModify"`
	CodebaseEnable        bool   `json:"codebaseEnable"`
	MaxToken              int    `json:"maxToken"`
	MaxSteps              int    `json:"maxSteps"`
	Temperature           int    `json:"temperature"`
	MaxRetries            int    `json:"maxRetries"`
	MentionContexts       []any  `json:"mentionContexts"`
	KnowledgeID           []any  `json:"knowledgeId"`
	KnowledgeName         []any  `json:"knowledgeName"`
	CodebaseID            string `json:"codebaseId"`
	MentionContextCount   int    `json:"mentionContextCount"`
	Command               string `json:"command"`
	ExpertID              string `json:"expertId"`
	RecommendID           string `json:"recommendId"`
	SkillID               string `json:"skillId"`
	SkillCount            int    `json:"skillCount"`
	TotalCount            int    `json:"totalCount"`
	FileURI               string `json:"fileUri"`
	PresentAt             int64  `json:"presentAt"`
	TraceID               string `json:"traceId"`
	RootRequestID         string `json:"rootRequestId"`
	ParentConversationID  string `json:"parentConversationId"`
	AgentName             string `json:"agentName"`
	AgentType             string `json:"agentType"`
	UserID                string `json:"userId"`
}

// ActivityReportTask reports one chat request so the upstream lights up the
// activity map and the streak counter. One report per account and day is what
// the client does, so the task sends exactly one.
func ActivityReportTask(ctx context.Context, client *http.Client, base string, credential *Credential) (TaskOutcome, error) {
	if credential == nil {
		return TaskOutcome{}, fmt.Errorf("workbuddy credential unavailable")
	}
	if base == "" {
		base = credential.BillingBase()
	}
	now := time.Now()
	conversationID := fmt.Sprintf("new-api-%d-%d", now.UnixMilli(), now.Nanosecond())
	event := chatRequestEvent{
		EventCode:            "chat_request_send",
		Timestamp:            now.UnixMilli(),
		Mode:                 "craft",
		ConversationID:       conversationID,
		RequestID:            conversationID,
		InputLength:          12,
		RequestModelID:       "deepseek-v4-flash",
		RequestModelName:     "DeepSeek V4 Flash",
		MentionContexts:      []any{},
		KnowledgeID:          []any{},
		KnowledgeName:        []any{},
		PresentAt:            now.UnixMilli(),
		RootRequestID:        conversationID,
		ParentConversationID: conversationID,
		AgentName:            "default",
		AgentType:            "conversation",
		UserID:               credential.UID,
	}
	payload, err := json.Marshal([]chatRequestEvent{event})
	if err != nil {
		return TaskOutcome{}, err
	}
	_, err = taskJSON(ctx, client, http.MethodPost, strings.TrimRight(base, "/")+ReportPath, credential, json.RawMessage(payload))
	if err != nil {
		return ClassifyTaskError(err)
	}
	return TaskOutcome{Status: TaskStatusDone, Message: "activity reported"}, nil
}

// TravelState is the state of the account's pet travel.
type TravelState struct {
	State             string `json:"state"`
	DailyLimitReached bool   `json:"daily_limit_reached"`
	RecordID          int64  `json:"record_id"`
	RewardCredit      int64  `json:"reward_credit"`
}

// TravelTask advances the pet travel by one step: it claims a finished trip,
// departs a new one when the daily limit allows it, and does nothing while the
// pet is still on the road.
func TravelTask(ctx context.Context, client *http.Client, base string, credential *Credential) (TaskOutcome, error) {
	if credential == nil {
		return TaskOutcome{}, fmt.Errorf("workbuddy credential unavailable")
	}
	if base == "" {
		base = credential.ChatBase()
	}
	host := strings.TrimRight(base, "/")
	data, err := taskJSON(ctx, client, http.MethodGet, host+TravelStatusPath, credential, nil)
	if err != nil {
		return ClassifyTaskError(err)
	}
	var state TravelState
	if err := json.Unmarshal(data, &state); err != nil {
		return TaskOutcome{}, fmt.Errorf("workbuddy travel status parse failed: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(state.State)) {
	case "arrived":
		if state.RecordID == 0 {
			return TaskOutcome{Status: TaskStatusSkipped, Message: "travel reward has no record id"}, nil
		}
		if _, err := taskJSON(ctx, client, http.MethodPost, host+TravelClaimPath, credential, map[string]any{"record_id": state.RecordID}); err != nil {
			outcome, failure := ClassifyTaskError(err)
			if failure != nil {
				return TaskOutcome{}, failure
			}
			return outcome, nil
		}
		return TaskOutcome{Status: TaskStatusDone, Message: fmt.Sprintf("claimed %d credits", state.RewardCredit)}, nil
	case "traveling":
		return TaskOutcome{Status: TaskStatusSkipped, Message: "pet is still traveling"}, nil
	}
	if state.DailyLimitReached {
		return TaskOutcome{Status: TaskStatusSkipped, Message: "daily travel limit reached"}, nil
	}
	if _, err := taskJSON(ctx, client, http.MethodPost, host+TravelDepartPath, credential, map[string]any{"location_id": 1}); err != nil {
		outcome, failure := ClassifyTaskError(err)
		if failure != nil {
			return TaskOutcome{}, failure
		}
		return outcome, nil
	}
	return TaskOutcome{Status: TaskStatusDone, Message: "pet departed"}, nil
}

// ClassifyTaskError turns an upstream answer into a task outcome. The first
// return value carries the outcome for a business state, the second one the
// outcome when the call itself failed.
//
// An activity that the upstream no longer runs, and a reward that was already
// collected, are normal states: they must not look like failures, otherwise the
// operator would disable a task that is working as intended.
func ClassifyTaskError(err error) (TaskOutcome, error) {
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		return TaskOutcome{}, err
	}
	body := strings.ToLower(httpErr.Body)
	if httpErr.Status == http.StatusNotFound {
		return TaskOutcome{Status: TaskStatusSkipped, Message: "activity is not available for this account"}, nil
	}
	for _, marker := range []string{"已签到", "已领取", "已经领取", "重复", "already", "duplicate"} {
		if strings.Contains(body, strings.ToLower(marker)) {
			return TaskOutcome{Status: TaskStatusSkipped, Message: "already completed today"}, nil
		}
	}
	for _, marker := range []string{"未开启", "未开始", "已下线", "不存在", "not found", "not open", "expired"} {
		if strings.Contains(body, strings.ToLower(marker)) {
			return TaskOutcome{Status: TaskStatusSkipped, Message: "activity is not running"}, nil
		}
	}
	return TaskOutcome{}, err
}
