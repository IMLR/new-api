package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	opencodeapi "github.com/QuantumNous/new-api/pkg/opencode"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestValidateOpenCodeGoChannelKey(t *testing.T) {
	plain := &model.Channel{Type: constant.ChannelTypeOpenCodeGo, Key: "sk-plain"}
	require.NoError(t, validateChannel(plain, true))
	assert.Equal(t, "sk-plain", plain.Key)

	imported := &model.Channel{
		Type: constant.ChannelTypeOpenCodeGo,
		Key:  "{\n\"opencode-go\": {\"key\": \"sk-imported\"}\n}",
	}
	require.NoError(t, validateChannel(imported, true))
	assert.Equal(t, "sk-imported", imported.Key)

	zen := &model.Channel{Type: constant.ChannelTypeOpenCodeGo, Key: `{"opencode":{"key":"sk-zen"}}`}
	assert.Error(t, validateChannel(zen, true))

	multiKey := &model.Channel{Type: constant.ChannelTypeOpenCodeGo, Key: "sk-one\nsk-two"}
	multiKey.ChannelInfo.IsMultiKey = true
	assert.Error(t, validateChannel(multiKey, false))
}

func TestNormalizeChannelTestEndpointForOpenCodeGo(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeOpenCodeGo, Models: "kimi-k3,qwen3.8-max,grok-4.5,aliased"}
	mapping := `{"aliased":"minimax-m2.5"}`
	channel.ModelMapping = &mapping

	cases := map[string]string{
		"kimi-k3":     "",
		"qwen3.8-max": string(constant.EndpointTypeAnthropic),
		"grok-4.5":    string(constant.EndpointTypeOpenAIResponse),
		"aliased":     string(constant.EndpointTypeAnthropic),
	}
	for modelName, want := range cases {
		assert.Equal(t, want, normalizeChannelTestEndpoint(channel, modelName, ""), modelName)
	}
	assert.Equal(t, string(constant.EndpointTypeOpenAI), normalizeChannelTestEndpoint(channel, "kimi-k3", string(constant.EndpointTypeOpenAI)))
	assert.True(t, shouldUseStreamForAutomaticChannelTest(channel))
}

func TestOpenCodeGoQuotaWindows(t *testing.T) {
	usage, err := opencodeapi.ParseUsage([]byte(`{"usage":{
		"rolling":{"percent":12.5,"resetInSec":3600},
		"weekly":{"percent":1,"resetInSec":86400}
	}}`))
	require.NoError(t, err)

	now := time.Unix(1_700_000_000, 0)
	windows := openCodeGoQuotaWindows(usage, now)
	require.Len(t, windows, 2)
	assert.Equal(t, "rolling", windows[0].Name)
	assert.Equal(t, 12.5, windows[0].UsedPercent)
	assert.Equal(t, 87.5, windows[0].RemainingPercent)
	assert.Equal(t, int64(3600), windows[0].ResetInSeconds)
	assert.Equal(t, now.Add(time.Hour).Format(time.RFC3339), windows[0].ResetAt)
	assert.Equal(t, "weekly", windows[1].Name)
	assert.Equal(t, int64(86400), windows[1].ResetInSeconds)
}

func TestGetOpenCodeGoChannelQuotaReportsWindowsAndModels(t *testing.T) {
	service.InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, opencodeapi.UsagePath, r.URL.Path)
		assert.Equal(t, "Bearer sk-quota", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"usage":{"rolling":{"percent":5,"resetInSec":7200}}}`))
	}))
	defer server.Close()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	oldDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = oldDB; _ = sqlDB.Close() })

	mapping := `{"aliased":"minimax-m2.5"}`
	base := server.URL
	channel := model.Channel{
		Type:         constant.ChannelTypeOpenCodeGo,
		Name:         "go",
		Key:          "sk-quota",
		BaseURL:      &base,
		Models:       "kimi-k3,aliased,grok-4.5",
		ModelMapping: &mapping,
	}
	require.NoError(t, db.Create(&channel).Error)

	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/channel/1/opencode-go/quota", nil)
	c.Params = gin.Params{{Key: "id", Value: "1"}}

	GetOpenCodeGoChannelQuota(c)

	var payload struct {
		Success bool                   `json:"success"`
		Message string                 `json:"message"`
		Data    openCodeGoQuotaPayload `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success, payload.Message)
	assert.Equal(t, channel.Id, payload.Data.ChannelId)
	assert.Equal(t, "****uota", payload.Data.KeyMasked)
	require.Len(t, payload.Data.Windows, 1)
	assert.Equal(t, 95.0, payload.Data.Windows[0].RemainingPercent)
	assert.NotEmpty(t, payload.Data.Windows[0].ResetAt)
	assert.Equal(t, []openCodeGoQuotaModel{
		{Model: "kimi-k3", Endpoint: opencodeapi.ChatCompletionsPath},
		{Model: "aliased", Endpoint: opencodeapi.AnthropicMessagesPath},
		{Model: "grok-4.5", Endpoint: opencodeapi.OpenAIResponsesPath},
	}, payload.Data.Models)
}

// openCodeGoQuotaPayload mirrors the handler response for decoding.
type openCodeGoQuotaPayload struct {
	ChannelId int                     `json:"channel_id"`
	KeyMasked string                  `json:"key_masked"`
	Windows   []openCodeGoQuotaWindow `json:"windows"`
	Models    []openCodeGoQuotaModel  `json:"models"`
}
