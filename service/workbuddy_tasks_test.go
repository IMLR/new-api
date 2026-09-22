package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	workbuddyapi "github.com/QuantumNous/new-api/pkg/workbuddy"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisteredTasksAreVisibleWithoutState(t *testing.T) {
	// Every registered task shows up in the channel view, even before it ran or
	// was touched by an operator.
	tasks := WorkBuddyChannelTasks(nil)
	require.NotEmpty(t, tasks)
	byID := map[string]WorkBuddyTaskSnapshot{}
	for _, task := range tasks {
		byID[task.ID] = task
		assert.True(t, task.Enabled, "tasks start enabled")
		assert.Equal(t, workBuddyTaskStatusPending, task.Status)
	}
	for _, id := range []string{"checkin", "activity", "travel", "keepalive"} {
		assert.Contains(t, byID, id)
	}
}

func TestWorkBuddyTaskDueFollowsHoursAndCooldown(t *testing.T) {
	task := WorkBuddyTask{ID: "checkin", Hours: []int{9, 21}}
	nine := time.Date(2026, 9, 22, 9, 5, 0, 0, workBuddyTaskTimeZone)

	assert.True(t, workBuddyTaskDue(task, workBuddyTaskState{}, nine), "the configured hour is due")
	assert.False(t, workBuddyTaskDue(task, workBuddyTaskState{}, nine.Add(2*time.Hour)), "another hour is not due")

	ran := workBuddyTaskState{At: nine.Add(-30 * time.Minute).Unix()}
	assert.False(t, workBuddyTaskDue(task, ran, nine), "a recent run is not repeated")

	yesterday := workBuddyTaskState{At: nine.Add(-24 * time.Hour).Unix()}
	assert.True(t, workBuddyTaskDue(task, yesterday, nine), "a new day runs again")

	retry := workBuddyTaskState{At: nine.Add(-2 * time.Hour).Unix()}
	assert.True(t, workBuddyTaskDue(task, retry, nine), "a failed run of an earlier slot is retried")
}

func TestWorkBuddyTaskStatusMapping(t *testing.T) {
	assert.Equal(t, workBuddyTaskStatusDone, workBuddyTaskStatusFor(workbuddyapi.TaskOutcome{Status: workbuddyapi.TaskStatusDone}))
	assert.Equal(t, workBuddyTaskStatusSkipped, workBuddyTaskStatusFor(workbuddyapi.TaskOutcome{Status: workbuddyapi.TaskStatusSkipped}))
}

func TestTaskStateRoundTripKeepsOtherKeys(t *testing.T) {
	channel := &model.Channel{OtherInfo: `{"status_reason":"ok"}`}
	enabled := false
	states := map[string]workBuddyTaskState{
		"checkin": {Enabled: &enabled, Date: "2026-09-22", Status: workBuddyTaskStatusFailed, Message: "boom", At: 1790000000},
	}
	encoded, err := encodeWorkBuddyTaskStates(channel, states)
	require.NoError(t, err)
	channel.OtherInfo = encoded

	restored := workBuddyTaskStates(channel)
	require.Contains(t, restored, "checkin")
	assert.False(t, *restored["checkin"].Enabled)
	assert.Equal(t, workBuddyTaskStatusFailed, restored["checkin"].Status)
	assert.Contains(t, channel.OtherInfo, "status_reason", "unrelated other-info keys survive")
}

func TestSetWorkBuddyChannelTaskRejectsUnknownTask(t *testing.T) {
	err := SetWorkBuddyChannelTask(context.Background(), 1, "does-not-exist", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown WorkBuddy task")
}

func TestRegisterWorkBuddyTaskReplacesDefinition(t *testing.T) {
	calls := 0
	run := func(context.Context, *http.Client, *model.Channel, *workbuddyapi.Credential) (workbuddyapi.TaskOutcome, error) {
		calls++
		return workbuddyapi.TaskOutcome{Status: workbuddyapi.TaskStatusDone}, nil
	}
	RegisterWorkBuddyTask(WorkBuddyTask{ID: "test-task", Title: "Test", Hours: []int{1}, Run: run})
	RegisterWorkBuddyTask(WorkBuddyTask{ID: "test-task", Title: "Test again", Hours: []int{1}, Run: run})

	task, ok := workBuddyTaskByID("test-task")
	require.True(t, ok)
	assert.Equal(t, "Test again", task.Title)
	assert.Equal(t, 1, countTaskID("test-task"), "the task is registered once")
}

func countTaskID(id string) int {
	count := 0
	for _, task := range workBuddyTasks() {
		if task.ID == id {
			count++
		}
	}
	return count
}

func TestWorkBuddyChannelTasksMarkForeignRealmTasksUnavailable(t *testing.T) {
	// The global deployment has no check-in and no travel, so those tasks must
	// not look runnable on a global account.
	global := &model.Channel{
		Key: `{"accessToken":"a","refreshToken":"r","expiresAt":1,"domain":"www.workbuddy.ai","realm":"global","uid":"u"}`,
	}
	tasks := WorkBuddyChannelTasks(global)
	byID := map[string]WorkBuddyTaskSnapshot{}
	for _, task := range tasks {
		byID[task.ID] = task
	}
	assert.False(t, byID["checkin"].Applicable, "check-in is CN only")
	assert.False(t, byID["travel"].Applicable, "travel is CN only")
	assert.True(t, byID["activity"].Applicable, "activity report runs on both deployments")
	assert.True(t, byID["keepalive"].Applicable, "token renewal runs on both deployments")
	assert.Equal(t, workBuddyTaskStatusSkipped, byID["checkin"].Status)
	assert.NotEmpty(t, byID["checkin"].Message)
}
