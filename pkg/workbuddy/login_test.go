package workbuddy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartAuthorizationReturnsLoginURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, AuthStatePath, r.URL.Path)
		assert.Equal(t, "CLI", r.URL.Query().Get("platform"))
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, LoginUserAgent, r.Header.Get("User-Agent"))
		assert.Equal(t, originCN, r.Header.Get("Origin"))
		_, _ = w.Write([]byte(`{"code":0,"msg":"OK","data":{"state":"state-1","authUrl":"https://copilot.tencent.com/login?platform=CLI&state=state-1"}}`))
	}))
	defer server.Close()

	session, err := StartAuthorization(context.Background(), server.Client(), RealmCN, server.URL)
	require.NoError(t, err)
	assert.Equal(t, "state-1", session.State)
	assert.Contains(t, session.AuthURL, "state-1")
}

func TestStartAuthorizationReportsUpstreamFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":12403,"msg":"unsupported client"}`))
	}))
	defer server.Close()

	_, err := StartAuthorization(context.Background(), server.Client(), RealmCN, server.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "12403")
}

func TestPollAuthorizationReportsPending(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, AuthTokenPath, r.URL.Path)
		assert.Equal(t, "state-1", r.URL.Query().Get("state"))
		_, _ = w.Write([]byte(`{"code":10001,"msg":"login ing"}`))
	}))
	defer server.Close()

	_, _, err := PollAuthorization(context.Background(), server.Client(), RealmCN, server.URL, "state-1")
	require.ErrorIs(t, err, ErrAuthorizationPending)
}

func TestPollAuthorizationReturnsTokensAndAccount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case AuthTokenPath:
			_, _ = w.Write([]byte(`{"code":0,"data":{"accessToken":"at-1","refreshToken":"rt-1","expiresIn":5184000,"domain":"copilot.tencent.com"}}`))
		case LoginAccountPath:
			assert.Equal(t, "Bearer at-1", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(`{"code":0,"data":{"uid":"user-123456","enterpriseId":"ent-1","nickname":"Alice"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	tokens, account, err := PollAuthorization(context.Background(), server.Client(), RealmCN, server.URL, "state-1")
	require.NoError(t, err)
	require.NotNil(t, tokens)
	require.NotNil(t, account)
	assert.Equal(t, "rt-1", tokens.RefreshToken)
	assert.Equal(t, "user-123456", account.UID)

	credential := CredentialFromLogin(tokens, account, RealmCN)
	require.NotNil(t, credential)
	assert.Equal(t, "at-1", credential.AccessToken)
	assert.Equal(t, "rt-1", credential.RefreshToken)
	assert.Equal(t, "user-123456", credential.UID)
	assert.Equal(t, RealmCN, credential.EffectiveRealm())
	assert.True(t, credential.ExpiresAt > time.Now().Add(59*24*time.Hour).Unix())

	// The credential the flow returns must survive a parse round trip.
	encoded, err := json.Marshal(credential)
	require.NoError(t, err)
	parsed, err := ParseCredential(string(encoded))
	require.NoError(t, err)
	assert.Equal(t, credential.UID, parsed.UID)
	assert.Equal(t, credential.Domain, parsed.Domain)
}

func TestPollAuthorizationKeepsWorkingWithoutAccount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == AuthTokenPath {
			_, _ = w.Write([]byte(`{"code":0,"data":{"accessToken":"at-2","refreshToken":"rt-2","expiresIn":3600}}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	tokens, account, err := PollAuthorization(context.Background(), server.Client(), RealmGlobal, server.URL, "state-2")
	require.NoError(t, err)
	require.NotNil(t, tokens)
	assert.Nil(t, account)
	credential := CredentialFromLogin(tokens, account, RealmGlobal)
	require.NotNil(t, credential)
	assert.Equal(t, GlobalBase, credential.ChatBase())
}

func TestLoginBaseFollowsRealm(t *testing.T) {
	assert.Equal(t, ChatBaseCN, LoginBase(RealmCN))
	assert.Equal(t, GlobalBase, LoginBase(RealmGlobal))
}
