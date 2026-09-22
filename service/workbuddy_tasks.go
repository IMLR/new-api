package service

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	workbuddyapi "github.com/QuantumNous/new-api/pkg/workbuddy"
)

// WorkBuddyTask is one account task. Tasks are independent units: a new one is
// added by registering it, and removing the registration removes it from the
// scheduler, the channel view and the switch list.
type WorkBuddyTask struct {
	ID          string
	Title       string
	Description string
	// Hours are the local hours (Asia/Shanghai) when the task may run.
	Hours []int
	// Realms limits the task to one deployment; empty serves both.
	Realms []string
	Run    func(ctx context.Context, client *http.Client, channel *model.Channel, credential *workbuddyapi.Credential) (workbuddyapi.TaskOutcome, error)
}

var (
	workBuddyTaskMu       sync.RWMutex
	workBuddyTaskRegistry = map[string]WorkBuddyTask{}
	workBuddyTaskOrder    []string
)

// RegisterWorkBuddyTask adds one task to the scheduler. Registering the same id
// again replaces the definition, which keeps test setups simple.
func RegisterWorkBuddyTask(task WorkBuddyTask) {
	task.ID = strings.TrimSpace(task.ID)
	if task.ID == "" || task.Run == nil {
		return
	}
	workBuddyTaskMu.Lock()
	defer workBuddyTaskMu.Unlock()
	if _, exists := workBuddyTaskRegistry[task.ID]; !exists {
		workBuddyTaskOrder = append(workBuddyTaskOrder, task.ID)
	}
	workBuddyTaskRegistry[task.ID] = task
}

// workBuddyTasks returns the registered tasks in registration order.
func workBuddyTasks() []WorkBuddyTask {
	workBuddyTaskMu.RLock()
	defer workBuddyTaskMu.RUnlock()
	out := make([]WorkBuddyTask, 0, len(workBuddyTaskOrder))
	for _, id := range workBuddyTaskOrder {
		if task, ok := workBuddyTaskRegistry[id]; ok {
			out = append(out, task)
		}
	}
	return out
}

func workBuddyTaskByID(id string) (WorkBuddyTask, bool) {
	workBuddyTaskMu.RLock()
	defer workBuddyTaskMu.RUnlock()
	task, ok := workBuddyTaskRegistry[id]
	return task, ok
}

// workBuddyTaskState is the per channel record of one task: whether the
// operator keeps it enabled, when it last ran and how it ended.
type workBuddyTaskState struct {
	Enabled *bool  `json:"enabled,omitempty"`
	Date    string `json:"date,omitempty"`
	Status  string `json:"status,omitempty"`
	Message string `json:"message,omitempty"`
	At      int64  `json:"at,omitempty"`
}

// Status values shown to the operator. pending means the task has not run in
// the current day yet.
const (
	workBuddyTaskStatusPending = "pending"
	workBuddyTaskStatusDone    = "done"
	workBuddyTaskStatusSkipped = "skipped"
	workBuddyTaskStatusFailed  = "failed"
)

const workBuddyTaskInfoKey = "workbuddy_tasks"

// workBuddyTaskTimeZone is the wall clock the upstream activities use.
var workBuddyTaskTimeZone = time.FixedZone("CST", 8*3600)

func workBuddyTaskStates(ch *model.Channel) map[string]workBuddyTaskState {
	states := map[string]workBuddyTaskState{}
	if ch == nil {
		return states
	}
	raw, ok := ch.GetOtherInfo()[workBuddyTaskInfoKey]
	if !ok {
		return states
	}
	encoded, err := common.Marshal(raw)
	if err != nil {
		return states
	}
	parsed := map[string]workBuddyTaskState{}
	if err := common.Unmarshal(encoded, &parsed); err != nil {
		return states
	}
	return parsed
}

// WorkBuddyTaskSnapshot is one task as the channel page shows it.
type WorkBuddyTaskSnapshot struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Hours       []int  `json:"hours,omitempty"`
	Enabled     bool   `json:"enabled"`
	// Applicable is false when the task never runs for this account, for
	// example the CN-only check-in on a global account.
	Applicable bool   `json:"applicable"`
	Status     string `json:"status"`
	Message    string `json:"message,omitempty"`
	At         int64  `json:"at,omitempty"`
}

// WorkBuddyChannelTasks lists the tasks of one channel with their current day
// status.
func WorkBuddyChannelTasks(ch *model.Channel) []WorkBuddyTaskSnapshot {
	states := workBuddyTaskStates(ch)
	today := time.Now().In(workBuddyTaskTimeZone).Format("2006-01-02")
	realm := ""
	if ch != nil {
		if credential, err := workbuddyapi.ParseCredential(ch.Key); err == nil && credential != nil {
			realm = credential.EffectiveRealm()
		}
	}
	out := make([]WorkBuddyTaskSnapshot, 0, len(workBuddyTaskOrder))
	for _, task := range workBuddyTasks() {
		state := states[task.ID]
		snapshot := WorkBuddyTaskSnapshot{
			ID:          task.ID,
			Title:       task.Title,
			Description: task.Description,
			Hours:       task.Hours,
			Enabled:     state.Enabled == nil || *state.Enabled,
			Applicable:  true,
			Status:      workBuddyTaskStatusPending,
		}
		if realm != "" && !workBuddyTaskServesRealm(task, realm) {
			snapshot.Applicable = false
			snapshot.Status = workBuddyTaskStatusSkipped
			snapshot.Message = fmt.Sprintf("task does not run on the %s deployment", realm)
		} else if state.Date == today {
			snapshot.Status = state.Status
			snapshot.Message = state.Message
			snapshot.At = state.At
		}
		out = append(out, snapshot)
	}
	return out
}

// workBuddyTaskServesRealm reports whether one task runs on one deployment.
func workBuddyTaskServesRealm(task WorkBuddyTask, realm string) bool {
	if len(task.Realms) == 0 {
		return true
	}
	for _, candidate := range task.Realms {
		if candidate == realm {
			return true
		}
	}
	return false
}

// SetWorkBuddyChannelTask turns one task on or off for one channel.
func SetWorkBuddyChannelTask(ctx context.Context, channelId int, taskID string, enabled bool) error {
	if _, ok := workBuddyTaskByID(taskID); !ok {
		return fmt.Errorf("unknown WorkBuddy task %q", taskID)
	}
	return model.WithWorkBuddyChannelOtherLock(ctx, channelId, func(ch *model.Channel) (string, error) {
		states := workBuddyTaskStates(ch)
		state := states[taskID]
		value := enabled
		state.Enabled = &value
		states[taskID] = state
		return encodeWorkBuddyTaskStates(ch, states)
	})
}

// encodeWorkBuddyTaskStates writes the task states back into the channel
// other-info document, keeping every other key untouched.
func encodeWorkBuddyTaskStates(ch *model.Channel, states map[string]workBuddyTaskState) (string, error) {
	info := ch.GetOtherInfo()
	info[workBuddyTaskInfoKey] = states
	encoded, err := common.Marshal(info)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// recordWorkBuddyTaskState stores the outcome of one run.
func recordWorkBuddyTaskState(ctx context.Context, channelId int, taskID string, status, message string, at time.Time) {
	err := model.WithWorkBuddyChannelOtherLock(ctx, channelId, func(ch *model.Channel) (string, error) {
		states := workBuddyTaskStates(ch)
		state := states[taskID]
		state.Date = at.In(workBuddyTaskTimeZone).Format("2006-01-02")
		state.Status = status
		state.Message = message
		state.At = at.Unix()
		states[taskID] = state
		return encodeWorkBuddyTaskStates(ch, states)
	})
	if err != nil {
		common.SysError(fmt.Sprintf("WorkBuddy task %s state write failed: %v", taskID, err))
	}
}

var workBuddyTaskSchedulerOnce sync.Once

// StartWorkBuddyTaskScheduler runs the registered account tasks. Each task
// decides by itself whether it is due, so adding or removing a task never
// touches the scheduler.
func StartWorkBuddyTaskScheduler() {
	workBuddyTaskSchedulerOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		go func() {
			runWorkBuddyTasks(context.Background(), time.Now())
			ticker := time.NewTicker(10 * time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				runWorkBuddyTasks(context.Background(), time.Now())
			}
		}()
	})
}

// runWorkBuddyTasks runs every due task of every enabled WorkBuddy channel.
func runWorkBuddyTasks(ctx context.Context, now time.Time) {
	local := now.In(workBuddyTaskTimeZone)
	lastID := 0
	for {
		var channels []model.Channel
		if err := model.DB.Where("type = ? AND status = ? AND id > ?", constant.ChannelTypeWorkBuddy, common.ChannelStatusEnabled, lastID).
			Order("id").Limit(100).Find(&channels).Error; err != nil {
			common.SysError("WorkBuddy task scan failed")
			return
		}
		if len(channels) == 0 {
			return
		}
		for index := range channels {
			lastID = channels[index].Id
			runWorkBuddyChannelTasks(ctx, &channels[index], local)
		}
	}
}

func runWorkBuddyChannelTasks(ctx context.Context, channel *model.Channel, local time.Time) {
	states := workBuddyTaskStates(channel)
	credential, err := workbuddyapi.ParseCredential(channel.Key)
	if err != nil {
		return
	}
	for _, task := range workBuddyTasks() {
		state := states[task.ID]
		if state.Enabled != nil && !*state.Enabled {
			continue
		}
		if !workBuddyTaskDue(task, state, local) {
			continue
		}
		runWorkBuddyTask(ctx, channel, task, credential, local)
	}
}

// workBuddyTaskDue reports whether a task should run in this time slot. A task
// runs at most once per slot, and a failed run is retried on the next slot
// rather than every few minutes.
func workBuddyTaskDue(task WorkBuddyTask, state workBuddyTaskState, local time.Time) bool {
	due := false
	for _, hour := range task.Hours {
		if hour == local.Hour() {
			due = true
			break
		}
	}
	if !due {
		return false
	}
	if state.At == 0 {
		return true
	}
	last := time.Unix(state.At, 0).In(workBuddyTaskTimeZone)
	if last.Format("2006-01-02") != local.Format("2006-01-02") {
		return true
	}
	return local.Sub(last) >= 90*time.Minute
}

func runWorkBuddyTask(ctx context.Context, channel *model.Channel, task WorkBuddyTask, credential *workbuddyapi.Credential, local time.Time) {
	if !workBuddyTaskServesRealm(task, credential.EffectiveRealm()) {
		recordWorkBuddyTaskState(ctx, channel.Id, task.ID, workBuddyTaskStatusSkipped,
			fmt.Sprintf("task does not run on the %s deployment", credential.EffectiveRealm()), local)
		return
	}
	client, err := workbuddyapi.NewUpstreamClient(channel.GetSetting().Proxy)
	if err != nil {
		recordWorkBuddyTaskState(ctx, channel.Id, task.ID, workBuddyTaskStatusFailed, "invalid channel proxy", local)
		return
	}
	taskCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	outcome, err := task.Run(taskCtx, client, channel, credential)
	if err != nil {
		common.SysError(fmt.Sprintf("WorkBuddy task %s failed on channel %d: %v", task.ID, channel.Id, err))
		recordWorkBuddyTaskState(ctx, channel.Id, task.ID, workBuddyTaskStatusFailed, err.Error(), local)
		return
	}
	recordWorkBuddyTaskState(ctx, channel.Id, task.ID, workBuddyTaskStatusFor(outcome), outcome.Message, local)
}

// workBuddyTaskStatusFor maps a task outcome to the stored status.
func workBuddyTaskStatusFor(outcome workbuddyapi.TaskOutcome) string {
	if outcome.Status == workbuddyapi.TaskStatusSkipped {
		return workBuddyTaskStatusSkipped
	}
	return workBuddyTaskStatusDone
}

// WorkBuddyTaskIDs lists the registered task ids, used by tests and by the
// channel page to explain what exists.
func WorkBuddyTaskIDs() []string {
	ids := make([]string, 0, len(workBuddyTaskOrder))
	for _, task := range workBuddyTasks() {
		ids = append(ids, task.ID)
	}
	sort.Strings(ids)
	return ids
}
