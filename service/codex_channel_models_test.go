package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchCodexChannelModelsReturnsUpstreamModelVariants(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/backend-api/codex/models", r.URL.Path)
		assert.Equal(t, "1.2.3", r.URL.Query().Get("client_version"))
		assert.Equal(t, "Bearer access-token", r.Header.Get("Authorization"))
		assert.Equal(t, "account-id", r.Header.Get("ChatGPT-Account-Id"))
		assert.Equal(t, "codex-cli/1.2.3", r.Header.Get("User-Agent"))
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{
			"models": [
				{"slug": "gpt-5.6-sol"},
				{"slug": "codex-auto-review"},
				{"slug": "gpt-5.6-sol"},
				{"slug": " "}
			]
		}`))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	baseURL := server.URL
	channel := &model.Channel{
		Type:    constant.ChannelTypeCodex,
		Key:     `{"access_token":"access-token","account_id":"account-id"}`,
		BaseURL: &baseURL,
	}

	models, err := fetchCodexChannelModels(
		context.Background(),
		channel,
		server.URL,
		server.Client(),
		"1.2.3",
	)

	require.NoError(t, err)
	assert.Equal(t, []string{
		"gpt-5.6-sol",
		"codex-auto-review",
		"gpt-5.6-sol-openai-compact",
	}, models)
}
