package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartCodexOAuthAuthorizationFlowBuildsCurrentCodexRequest(t *testing.T) {
	disableRedisForCodexOAuthTest(t)

	flow, err := StartCodexOAuthAuthorizationFlow(context.Background(), 17, 23, "")
	require.NoError(t, err)
	require.NotEmpty(t, flow.FlowID)
	assert.WithinDuration(t, time.Now().Add(codexOAuthFlowTTL), flow.ExpiresAt, 2*time.Second)

	authorizeURL, err := url.Parse(flow.AuthorizeURL)
	require.NoError(t, err)
	assert.Equal(t, "https", authorizeURL.Scheme)
	assert.Equal(t, "auth.openai.com", authorizeURL.Host)
	assert.Equal(t, "/oauth/authorize", authorizeURL.Path)

	query := authorizeURL.Query()
	assert.Equal(t, "code", query.Get("response_type"))
	assert.Equal(t, codexOAuthClientID, query.Get("client_id"))
	assert.Equal(t, codexOAuthRedirectURI, query.Get("redirect_uri"))
	assert.Equal(t, codexOAuthScope, query.Get("scope"))
	assert.Equal(t, "S256", query.Get("code_challenge_method"))
	assert.Equal(t, "true", query.Get("id_token_add_organizations"))
	assert.Equal(t, "true", query.Get("codex_cli_simplified_flow"))
	assert.Equal(t, codexOAuthOriginator, query.Get("originator"))

	rawRecord, ok := codexOAuthMemoryFlows.Load(flow.FlowID)
	require.True(t, ok)
	record, ok := rawRecord.(codexOAuthFlowRecord)
	require.True(t, ok)
	assert.Equal(t, 17, record.AdminID)
	assert.Equal(t, 23, record.ChannelID)
	assert.Equal(t, record.State, query.Get("state"))
	assert.GreaterOrEqual(t, len(record.Verifier), 86)
	challengeSum := sha256.Sum256([]byte(record.Verifier))
	assert.Equal(
		t,
		base64.RawURLEncoding.EncodeToString(challengeSum[:]),
		query.Get("code_challenge"),
	)
}

func TestCodexOAuthFlowCanOnlyBeConsumedOnce(t *testing.T) {
	disableRedisForCodexOAuthTest(t)

	flow, err := StartCodexOAuthAuthorizationFlow(context.Background(), 8, 0, "")
	require.NoError(t, err)

	record, err := consumeCodexOAuthFlow(context.Background(), flow.FlowID)
	require.NoError(t, err)
	assert.Equal(t, 8, record.AdminID)

	_, err = consumeCodexOAuthFlow(context.Background(), flow.FlowID)
	assert.ErrorIs(t, err, errCodexOAuthFlowExpired)
}

func TestCompleteCodexOAuthAuthorizationFlowRejectsDifferentOwnerAndConsumesFlow(t *testing.T) {
	disableRedisForCodexOAuthTest(t)

	flow, err := StartCodexOAuthAuthorizationFlow(context.Background(), 8, 12, "")
	require.NoError(t, err)

	_, err = CompleteCodexOAuthAuthorizationFlow(
		context.Background(),
		9,
		12,
		flow.FlowID,
		"http://localhost:1455/auth/callback?code=secret-code&state=secret-state",
	)
	require.ErrorContains(t, err, "does not match")

	_, err = consumeCodexOAuthFlow(context.Background(), flow.FlowID)
	assert.ErrorIs(t, err, errCodexOAuthFlowExpired)
}

func TestParseCodexOAuthCallbackURL(t *testing.T) {
	t.Run("accepts exact localhost callback", func(t *testing.T) {
		code, state, err := parseCodexOAuthCallbackURL(
			"http://localhost:1455/auth/callback?code=authorization-code&state=oauth-state",
		)
		require.NoError(t, err)
		assert.Equal(t, "authorization-code", code)
		assert.Equal(t, "oauth-state", state)
	})

	tests := []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "wrong scheme", input: "https://localhost:1455/auth/callback?code=a&state=b"},
		{name: "wrong host", input: "http://example.com:1455/auth/callback?code=a&state=b"},
		{name: "wrong port", input: "http://localhost:1457/auth/callback?code=a&state=b"},
		{name: "wrong path", input: "http://localhost:1455/other?code=a&state=b"},
		{name: "fragment", input: "http://localhost:1455/auth/callback?code=a&state=b#unexpected"},
		{name: "missing code", input: "http://localhost:1455/auth/callback?state=b"},
		{name: "oauth error", input: "http://localhost:1455/auth/callback?error=access_denied&state=b"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := parseCodexOAuthCallbackURL(test.input)
			assert.Error(t, err)
		})
	}
}

func TestExchangeCodexAuthorizationCodeUsesPKCEAndDoesNotExposeErrorBody(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodPost, r.Method)
			require.NoError(t, r.ParseForm())
			assert.Equal(t, "authorization_code", r.Form.Get("grant_type"))
			assert.Equal(t, "test-client", r.Form.Get("client_id"))
			assert.Equal(t, "test-code", r.Form.Get("code"))
			assert.Equal(t, "test-verifier", r.Form.Get("code_verifier"))
			assert.Equal(t, codexOAuthRedirectURI, r.Form.Get("redirect_uri"))
			assert.Equal(t, codexOAuthOriginator, r.Header.Get("originator"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"id_token":"id-token",
				"access_token":"access-token",
				"refresh_token":"refresh-token",
				"expires_in":3600
			}`))
		}))
		defer server.Close()

		result, err := exchangeCodexAuthorizationCode(
			context.Background(),
			server.Client(),
			server.URL,
			"test-client",
			"test-code",
			"test-verifier",
			codexOAuthRedirectURI,
		)
		require.NoError(t, err)
		assert.Equal(t, "id-token", result.IDToken)
		assert.Equal(t, "access-token", result.AccessToken)
		assert.Equal(t, "refresh-token", result.RefreshToken)
		assert.WithinDuration(t, time.Now().Add(time.Hour), result.ExpiresAt, 2*time.Second)
	})

	t.Run("upstream error is redacted", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error_description":"sensitive authorization code"}`))
		}))
		defer server.Close()

		_, err := exchangeCodexAuthorizationCode(
			context.Background(),
			server.Client(),
			server.URL,
			"test-client",
			"secret-code",
			"secret-verifier",
			codexOAuthRedirectURI,
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status=400")
		assert.NotContains(t, err.Error(), "sensitive")
		assert.NotContains(t, err.Error(), "secret-code")
		assert.NotContains(t, err.Error(), "secret-verifier")
	})
}

func disableRedisForCodexOAuthTest(t *testing.T) {
	t.Helper()
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	codexOAuthMemoryFlows.Range(func(key, _ any) bool {
		codexOAuthMemoryFlows.Delete(key)
		return true
	})
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
		codexOAuthMemoryFlows.Range(func(key, _ any) bool {
			codexOAuthMemoryFlows.Delete(key)
			return true
		})
	})
}

func TestCodexOAuthCallbackInputLimit(t *testing.T) {
	_, _, err := parseCodexOAuthCallbackURL(strings.Repeat("a", codexOAuthInputMaxBytes+1))
	assert.Error(t, err)
}
