package cline

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCredentialImports(t *testing.T) {
	for _, raw := range []string{`{"refreshToken":"r","accessToken":"a","expiresAt":1800000000000}`, `{"refresh_token":"r","access_token":"a","expires_at":1800000000}`, `{"providers":{"cline":{"settings":{"auth":{"refreshToken":"r","accessToken":"a","expiresAt":1800000000000}}}}}`} {
		c, err := ParseCredential(raw)
		require.NoError(t, err)
		assert.Equal(t, "r", c.RefreshToken)
		assert.Equal(t, int64(1800000000000), c.ExpiresAt)
		assert.Equal(t, "workos:a", c.Bearer())
	}
	for _, raw := range []string{`[]`, `null`, `{"accessToken":"a"}`, `{"refreshToken":12}`, `{"providers":{}}`} {
		_, err := ParseCredential(raw)
		assert.Error(t, err)
	}
	c, err := ParseCredential(`{"refreshToken":"r"}`)
	require.NoError(t, err)
	assert.True(t, c.NeedsRefresh(time.Now()))
}
func TestModelsRespectSubscription(t *testing.T) {
	future := time.Now().Add(time.Hour).Format(time.RFC3339)
	past := time.Now().Add(-time.Hour).Format(time.RFC3339)
	for _, tt := range []struct {
		name      string
		status    int
		plan      string
		want      []string
		wantError bool
	}{
		{"no plan", 404, `{"success":false,"data":null,"error":"no plan history found for user"}`, []string{"cline-free/kimi-k3"}, false},
		{"active", 200, fmt.Sprintf(`{"success":true,"data":{"subscriptionId":"s","currentPeriodEnd":%q,"plan":{"displayName":"ClinePass Monthly"}}}`, future), []string{"cline-free/kimi-k3", "cline-pass/kimi-k3"}, false},
		{"expired", 200, fmt.Sprintf(`{"success":true,"data":{"subscriptionId":"s","currentPeriodEnd":%q,"plan":{"name":"cline-pass"}}}`, past), []string{"cline-free/kimi-k3"}, false},
		{"canceled", 200, `{"success":true,"data":{"subscriptionId":"s","status":"canceled","plan":{"name":"cline-pass"}}}`, []string{"cline-free/kimi-k3"}, false},
		{"other plan", 200, `{"success":true,"data":{"subscriptionId":"s","plan":{"name":"enterprise"}}}`, []string{"cline-free/kimi-k3"}, false},
		{"unauthorized", 401, `{"success":false}`, nil, true},
		{"server error", 503, `{"success":false}`, nil, true},
		{"unknown 404", 404, `{"error":"not found"}`, nil, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "cline-desktop", r.Header.Get("X-CLIENT-TYPE"))
				assert.Equal(t, "Bearer workos:a", r.Header.Get("Authorization"))
				if r.URL.Path == "/api/v1/users/me/plan" {
					w.WriteHeader(tt.status)
					fmt.Fprint(w, tt.plan)
					return
				}
				fmt.Fprint(w, `{"free":[{"id":"cline-free/kimi-k3"},{"id":"cline-free/kimi-k3"}],"clinePass":[{"id":"cline-pass/kimi-k3"}],"recommended":[{"id":"paid-model"}]}`)
			}))
			defer server.Close()
			got, err := Models(context.Background(), server.Client(), server.URL, &Credential{AccessToken: "a"})
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
func TestRefreshRotatesCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		require.NoError(t, common.DecodeJson(r.Body, &body))
		assert.Equal(t, "old-refresh", body["refreshToken"])
		assert.Equal(t, "refresh_token", body["grantType"])
		fmt.Fprintf(w, `{"success":true,"data":{"accessToken":"new-access","refreshToken":"new-refresh","expiresAt":%q}}`, time.Now().Add(time.Hour).Format(time.RFC3339))
	}))
	defer server.Close()
	c, err := Refresh(context.Background(), server.Client(), server.URL, &Credential{RefreshToken: "old-refresh"})
	require.NoError(t, err)
	assert.Equal(t, "new-refresh", c.RefreshToken)
	assert.Equal(t, "new-access", c.AccessToken)
	assert.False(t, c.NeedsRefresh(time.Now()))
}
