package service

import (
	"context"
	"net/http"

	"github.com/QuantumNous/new-api/model"
	workbuddyapi "github.com/QuantumNous/new-api/pkg/workbuddy"
)

func init() {
	RegisterWorkBuddyTask(WorkBuddyTask{
		ID:          "travel",
		Title:       "Pet travel",
		Description: "Claims a finished trip and sends the pet out again when the daily limit allows it.",
		Hours:       []int{9, 21},
		// The growth domain is measured on the CN deployment only.
		Realms: []string{workbuddyapi.RealmCN},
		Run: func(ctx context.Context, client *http.Client, channel *model.Channel, credential *workbuddyapi.Credential) (workbuddyapi.TaskOutcome, error) {
			return workbuddyapi.TravelTask(ctx, client, credential.ChatBase(), credential)
		},
	})
}
