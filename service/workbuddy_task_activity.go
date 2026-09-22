package service

import (
	"context"
	"net/http"

	"github.com/QuantumNous/new-api/model"
	workbuddyapi "github.com/QuantumNous/new-api/pkg/workbuddy"
)

func init() {
	RegisterWorkBuddyTask(WorkBuddyTask{
		ID:          "activity",
		Title:       "Activity report",
		Description: "Reports one chat event so the activity map and streak counter advance.",
		Hours:       []int{10},
		Run: func(ctx context.Context, client *http.Client, channel *model.Channel, credential *workbuddyapi.Credential) (workbuddyapi.TaskOutcome, error) {
			return workbuddyapi.ActivityReportTask(ctx, client, credential.BillingBase(), credential)
		},
	})
}
