package service

import (
	"context"
	"net/http"

	"github.com/QuantumNous/new-api/model"
	workbuddyapi "github.com/QuantumNous/new-api/pkg/workbuddy"
)

func init() {
	RegisterWorkBuddyTask(WorkBuddyTask{
		ID:          "keepalive",
		Title:       "Credential renewal",
		Description: "Renews the access token of the account once a day.",
		Hours:       []int{22},
		Run: func(ctx context.Context, client *http.Client, channel *model.Channel, credential *workbuddyapi.Credential) (workbuddyapi.TaskOutcome, error) {
			// A rejected-token refresh forces the rotation instead of waiting
			// for the expiry window.
			if _, err := ResolveWorkBuddyCredential(ctx, channel.Id, credential.AccessToken); err != nil {
				return workbuddyapi.TaskOutcome{}, err
			}
			return workbuddyapi.TaskOutcome{Status: workbuddyapi.TaskStatusDone, Message: "token renewed"}, nil
		},
	})
}
