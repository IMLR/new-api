package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	opencodeapi "github.com/QuantumNous/new-api/pkg/opencode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveOpenCodeGoKey(t *testing.T) {
	plain := &model.Channel{Type: constant.ChannelTypeOpenCodeGo, Key: "sk-opencode-go"}
	key, err := ResolveOpenCodeGoKey(plain)
	require.NoError(t, err)
	assert.Equal(t, "sk-opencode-go", key)

	imported := &model.Channel{
		Type: constant.ChannelTypeOpenCodeGo,
		Key:  `{"opencode-go":{"type":"api","key":"sk-imported"}}`,
	}
	key, err = ResolveOpenCodeGoKey(imported)
	require.NoError(t, err)
	assert.Equal(t, "sk-imported", key)

	zen := &model.Channel{Type: constant.ChannelTypeOpenCodeGo, Key: `{"opencode":{"type":"api","key":"sk-zen"}}`}
	_, err = ResolveOpenCodeGoKey(zen)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Zen")

	otherType := &model.Channel{Type: constant.ChannelTypeCline, Key: "sk-opencode-go"}
	_, err = ResolveOpenCodeGoKey(otherType)
	require.Error(t, err)

	multiKey := &model.Channel{Type: constant.ChannelTypeOpenCodeGo, Key: "sk-one\nsk-two"}
	multiKey.ChannelInfo.IsMultiKey = true
	_, err = ResolveOpenCodeGoKey(multiKey)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "single-key")
}

func TestOpenCodeGoBaseURL(t *testing.T) {
	assert.Equal(t, opencodeapi.BaseURL, OpenCodeGoBaseURL(&model.Channel{Type: constant.ChannelTypeOpenCodeGo}))

	custom := "https://gateway.example.com/opencode/go"
	ch := &model.Channel{Type: constant.ChannelTypeOpenCodeGo, BaseURL: &custom}
	assert.Equal(t, custom, OpenCodeGoBaseURL(ch))
}

func TestFetchOpenCodeGoChannelData(t *testing.T) {
	InitHttpClient()
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		assert.Equal(t, "Bearer sk-test", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case opencodeapi.ModelsPath:
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"kimi-k3"},{"id":"qwen3.8-max"}]}`))
		case opencodeapi.UsagePath:
			_, _ = w.Write([]byte(`{"usage":{"rolling":{"percent":12.5,"resetInSec":3600},"weekly":{"percent":1,"resetInSec":86400}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	base := server.URL
	ch := &model.Channel{Type: constant.ChannelTypeOpenCodeGo, Key: "sk-test", BaseURL: &base}

	models, err := FetchOpenCodeGoChannelModels(ch)
	require.NoError(t, err)
	assert.Equal(t, []string{"kimi-k3", "qwen3.8-max"}, models)

	usage, err := FetchOpenCodeGoChannelUsage(ch)
	require.NoError(t, err)
	require.NotNil(t, usage.Rolling)
	assert.Equal(t, 12.5, usage.Rolling.UsedPercent)
	require.NotNil(t, usage.Weekly)
	assert.Nil(t, usage.Monthly)

	assert.Equal(t, []string{opencodeapi.ModelsPath, opencodeapi.UsagePath}, paths)
}

func TestFetchOpenCodeGoChannelUsageReportsUpstreamError(t *testing.T) {
	InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"AuthError","message":"Unauthorized"}}`))
	}))
	defer server.Close()

	base := server.URL
	ch := &model.Channel{Type: constant.ChannelTypeOpenCodeGo, Key: "sk-bad", BaseURL: &base}
	_, err := FetchOpenCodeGoChannelUsage(ch)
	require.Error(t, err)
	var httpErr *opencodeapi.HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusUnauthorized, httpErr.Status)
	assert.Equal(t, fmt.Sprintf("OpenCode Go AuthError: %s", "Unauthorized"), httpErr.Error())
}
