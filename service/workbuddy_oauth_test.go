package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	workbuddyapi "github.com/QuantumNous/new-api/pkg/workbuddy"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkBuddyAuthorizationFlow walks the sign-in: the administrator starts a
// flow, the browser sign-in stays pending for a while and then yields the
// credential the channel stores.
func TestWorkBuddyAuthorizationFlow(t *testing.T) {
	tokenReady := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case workbuddyapi.AuthStatePath:
			_, _ = w.Write([]byte(`{"code":0,"data":{"state":"state-9","authUrl":"https://example.com/login?state=state-9"}}`))
		case workbuddyapi.AuthTokenPath:
			if !tokenReady {
				_, _ = w.Write([]byte(`{"code":10001,"msg":"login ing"}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"accessToken":"at-9","refreshToken":"rt-9","expiresIn":5184000,"domain":"copilot.tencent.com"}}`))
		case workbuddyapi.LoginAccountPath:
			_, _ = w.Write([]byte(`{"code":0,"data":{"uid":"user-9","enterpriseId":"ent-9","nickname":"Bob"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	previous := workBuddyLoginBase
	workBuddyLoginBase = func(string) string { return server.URL }
	t.Cleanup(func() { workBuddyLoginBase = previous })

	flow, err := StartWorkBuddyAuthorizationFlow(context.Background(), 1, workbuddyapi.RealmCN, "")
	require.NoError(t, err)
	require.NotEmpty(t, flow.FlowID)
	assert.Contains(t, flow.AuthorizeURL, "state-9")

	pending, err := PollWorkBuddyAuthorizationFlow(context.Background(), 1, flow.FlowID)
	require.NoError(t, err)
	assert.Equal(t, "pending", pending.Status)

	// The flow survives a pending confirmation, so the administrator can
	// confirm again after finishing the browser sign-in.
	tokenReady = true
	ready, err := PollWorkBuddyAuthorizationFlow(context.Background(), 1, flow.FlowID)
	require.NoError(t, err)
	require.Equal(t, "ready", ready.Status)
	credential, err := workbuddyapi.ParseCredential(ready.Credential)
	require.NoError(t, err)
	assert.Equal(t, "at-9", credential.AccessToken)
	assert.Equal(t, "user-9", credential.UID)
	assert.Equal(t, workbuddyapi.RealmCN, credential.EffectiveRealm())

	// Once consumed the flow is gone.
	_, err = PollWorkBuddyAuthorizationFlow(context.Background(), 1, flow.FlowID)
	require.ErrorIs(t, err, errWorkBuddyOAuthFlowExpired)
}

func TestWorkBuddyAuthorizationFlowRejectsForeignAdministrator(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"state":"state-1","authUrl":"https://example.com/login"}}`))
	}))
	defer server.Close()

	previous := workBuddyLoginBase
	workBuddyLoginBase = func(string) string { return server.URL }
	t.Cleanup(func() { workBuddyLoginBase = previous })

	flow, err := StartWorkBuddyAuthorizationFlow(context.Background(), 1, workbuddyapi.RealmCN, "")
	require.NoError(t, err)

	_, err = PollWorkBuddyAuthorizationFlow(context.Background(), 2, flow.FlowID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "administrator")

	_, err = PollWorkBuddyAuthorizationFlow(context.Background(), 1, "missing-flow")
	require.ErrorIs(t, err, errWorkBuddyOAuthFlowExpired)
}

func TestWorkBuddyAuthorizationFlowRejectsUnknownProxy(t *testing.T) {
	_, err := StartWorkBuddyAuthorizationFlow(context.Background(), 1, workbuddyapi.RealmCN, "not-a-proxy")
	require.Error(t, err)
}
