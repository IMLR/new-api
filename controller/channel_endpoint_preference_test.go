package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestChannelTestUsesSavedModelEndpoint(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Setting: common.GetPointer(`{"model_endpoints":{"gpt-6-astra":"openai-response"}}`)}
	assert.Equal(t, "openai-response", normalizeChannelTestEndpoint(channel, "gpt-6-astra", ""))
	assert.Equal(t, "openai", normalizeChannelTestEndpoint(channel, "gpt-6-astra", "openai"))
	assert.Empty(t, normalizeChannelTestEndpoint(channel, "other", ""))
}

func TestChannelTestEndpointSkipsResponsesPolicyForChatOnlyChannels(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	old := settings.ChatCompletionsToResponsesPolicy
	t.Cleanup(func() { settings.ChatCompletionsToResponsesPolicy = old })
	settings.ChatCompletionsToResponsesPolicy = model_setting.ChatCompletionsToResponsesPolicy{
		Enabled:       true,
		AllChannels:   true,
		ModelPatterns: []string{"^gpt-.*$"},
	}

	// WorkBuddy 的上游只接受 chat completions 请求体，测试必须走 chat 端点。
	workbuddy := &model.Channel{Type: constant.ChannelTypeWorkBuddy}
	assert.Empty(t, normalizeChannelTestEndpoint(workbuddy, "gpt-6-astra", ""))
	assert.Empty(t, normalizeChannelTestEndpoint(workbuddy, "gpt-5.3-codex", ""))

	// 支持 Responses 端点的渠道类型仍然跟随策略。
	codex := &model.Channel{Type: constant.ChannelTypeCodex}
	assert.Equal(t, "openai-response", normalizeChannelTestEndpoint(codex, "gpt-6-astra", ""))
}
