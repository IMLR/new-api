package service

import (
	"context"
	"net/http"

	"github.com/QuantumNous/new-api/model"
	workbuddyapi "github.com/QuantumNous/new-api/pkg/workbuddy"
)

func init() {
	RegisterWorkBuddyTask(WorkBuddyTask{
		ID:          "checkin",
		Title:       "Daily check-in",
		Description: "Collects the daily sign-in credits of the account.",
		Hours:       []int{9, 21},
		// The international deployment has no measured check-in endpoint.
		Realms: []string{workbuddyapi.RealmCN},
		Run: func(ctx context.Context, client *http.Client, channel *model.Channel, credential *workbuddyapi.Credential) (workbuddyapi.TaskOutcome, error) {
			return workbuddyapi.CheckinTask(ctx, client, credential.BillingBase(), credential)
		},
	})
}
