package service

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/model"
	workbuddyapi "github.com/QuantumNous/new-api/pkg/workbuddy"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testChannelWithBase(base string) *model.Channel {
	channel := &model.Channel{}
	if base != "" {
		value := base
		channel.BaseURL = &value
	}
	return channel
}

func TestWorkBuddyBaseFollowsAccountRealm(t *testing.T) {
	cn := &workbuddyapi.Credential{RefreshToken: "rt"}
	global := &workbuddyapi.Credential{RefreshToken: "rt", Realm: workbuddyapi.RealmGlobal}

	// A channel that keeps the built-in CN chat address still routes a global
	// account to the international host, which is what the chat relay does too.
	defaultBase := testChannelWithBase(workbuddyapi.ChatBaseCN)
	assert.Equal(t, workbuddyapi.ChatBaseCN, workBuddyBase(defaultBase, cn, false))
	assert.Equal(t, workbuddyapi.GlobalBase, workBuddyBase(defaultBase, global, false))
	assert.Equal(t, workbuddyapi.BillingBaseCN, workBuddyBase(defaultBase, cn, true))
	assert.Equal(t, workbuddyapi.GlobalBase, workBuddyBase(defaultBase, global, true))

	// An address set by the operator wins for every call.
	custom := testChannelWithBase("https://mirror.example.com")
	assert.Equal(t, "https://mirror.example.com", workBuddyBase(custom, cn, false))
	assert.Equal(t, "https://mirror.example.com", workBuddyBase(custom, global, true))

	// An empty address falls back to the realm of the credential.
	assert.Equal(t, workbuddyapi.GlobalBase, workBuddyBase(testChannelWithBase(""), global, false))
	assert.Equal(t, workbuddyapi.ChatBaseCN, workBuddyBase(nil, nil, false))
}

func TestWorkBuddyCredentialErrorExplainsDeadSession(t *testing.T) {
	expired := &workbuddyapi.HTTPError{
		Status: 401,
		Body:   `{"code":12153,"msg":"12153:refresh token failed:400 Bad Request: invalid_grant: Invalid refresh token"}`,
	}
	wrapped := workBuddyCredentialError(expired)
	require.Error(t, wrapped)
	assert.Contains(t, wrapped.Error(), "sign in again")
	assert.Contains(t, wrapped.Error(), "invalid_grant", "the upstream text stays visible")

	plain := errors.New("network down")
	assert.Equal(t, plain, workBuddyCredentialError(plain))
}

func TestIsSessionExpiredDetectsUpstreamCodes(t *testing.T) {
	assert.True(t, workbuddyapi.IsSessionExpired(&workbuddyapi.HTTPError{Status: 401, Body: `{"code":12153,"msg":"refresh token failed"}`}))
	assert.True(t, workbuddyapi.IsSessionExpired(&workbuddyapi.HTTPError{Status: 400, Body: `invalid_grant`}))
	assert.False(t, workbuddyapi.IsSessionExpired(&workbuddyapi.HTTPError{Status: 429, Body: `{"code":6004,"msg":"rate limited"}`}))
	assert.False(t, workbuddyapi.IsSessionExpired(errors.New("12153")))
}
