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

func TestProbeModelReadsQuotaWindow(t *testing.T) {
	now := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer workos:a", r.Header.Get("Authorization"))
		var body map[string]any
		require.NoError(t, common.DecodeJson(r.Body, &body))
		assert.Equal(t, "cline-free/kimi-k3", body["model"])
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"code":"INFERENCE_CAP_ERROR","message":"Error 429: Daily free limit reached on model vmc/fireworks-cline-k3-contributor-fallbacks. Try again in 33m"}}`)
	}))
	defer server.Close()

	result, err := ProbeModel(context.Background(), server.Client(), server.URL, &Credential{AccessToken: "a"}, "cline-free/kimi-k3")
	require.NoError(t, err)
	assert.Equal(t, http.StatusTooManyRequests, result.StatusCode)
	assert.False(t, result.Available)
	assert.Equal(t, "INFERENCE_CAP_ERROR", result.Code)
	assert.Contains(t, result.Message, "Daily free limit")

	until, limited := result.CooldownUntil(now)
	require.True(t, limited)
	assert.Equal(t, now.Add(33*time.Minute), until)
}

func TestProbeModelReportsAvailableRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"p\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	result, err := ProbeModel(context.Background(), server.Client(), server.URL, &Credential{AccessToken: "a"}, "z-ai/glm-5.3-flash")
	require.NoError(t, err)
	assert.True(t, result.Available)
	_, limited := result.CooldownUntil(time.Now())
	assert.False(t, limited)
}

func TestProbeModelKeepsPlainTextErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, "upstream exploded")
	}))
	defer server.Close()

	result, err := ProbeModel(context.Background(), server.Client(), server.URL, &Credential{AccessToken: "a"}, "cline-free/kimi-k3")
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadGateway, result.StatusCode)
	assert.False(t, result.Available)
	assert.Equal(t, "upstream exploded", result.Message)
	_, limited := result.CooldownUntil(time.Now())
	assert.False(t, limited)
}

func TestFetchAccountAndPlan(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer workos:a", r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/api/v1/users/me":
			fmt.Fprint(w, `{"success":true,"data":{"id":"usr-1","email":" user@example.com ","displayName":" user "}}`)
		case "/api/v1/users/me/plan":
			fmt.Fprint(w, `{"success":true,"data":{"subscriptionId":"sub","status":"active","currentPeriodEnd":"2026-10-01T00:00:00Z","plan":{"name":"cline-pass"}}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	account, err := FetchAccount(context.Background(), server.Client(), server.URL, &Credential{AccessToken: "a"})
	require.NoError(t, err)
	assert.Equal(t, "user@example.com", account.Email)
	assert.Equal(t, "user", account.DisplayName)

	plan, err := FetchCurrentPlan(context.Background(), server.Client(), server.URL, &Credential{AccessToken: "a"})
	require.NoError(t, err)
	require.NotNil(t, plan)
	assert.True(t, plan.Active(time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)))
}

func TestFetchCurrentPlanWithoutHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"success":false,"data":null,"error":"no plan history found for user"}`)
	}))
	defer server.Close()

	plan, err := FetchCurrentPlan(context.Background(), server.Client(), server.URL, &Credential{AccessToken: "a"})
	require.NoError(t, err)
	assert.Nil(t, plan)
}
