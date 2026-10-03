package service

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestChannelEndpointPreferenceOverridesGlobalPolicy(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	old := settings.ChatCompletionsToResponsesPolicy
	t.Cleanup(func() { settings.ChatCompletionsToResponsesPolicy = old })
	settings.ChatCompletionsToResponsesPolicy = model_setting.ChatCompletionsToResponsesPolicy{Enabled: true, AllChannels: true, ModelPatterns: []string{".*"}}
	channel := dto.ChannelSettings{ModelEndpoints: map[string]string{"astra": "openai-response", "sol": "openai"}}
	assert.True(t, ShouldChannelUseResponses(channel, 11, 1, "astra"))
	assert.False(t, ShouldChannelUseResponses(channel, 11, 1, "sol"))
	assert.True(t, ShouldChannelUseResponses(channel, 11, 1, "other"))
	settings.ChatCompletionsToResponsesPolicy.Enabled = false
	assert.True(t, ShouldChannelUseResponses(channel, 11, 1, "astra"))
	assert.False(t, ShouldChannelUseResponses(channel, 11, 1, "other"))
}

func TestResponsesPolicySkipsChatOnlyChannelTypes(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	old := settings.ChatCompletionsToResponsesPolicy
	t.Cleanup(func() { settings.ChatCompletionsToResponsesPolicy = old })
	settings.ChatCompletionsToResponsesPolicy = model_setting.ChatCompletionsToResponsesPolicy{
		Enabled:       true,
		AllChannels:   true,
		ModelPatterns: []string{"^gpt-.*$"},
	}

	assert.True(t, ShouldChannelUseResponses(dto.ChannelSettings{}, 4, constant.ChannelTypeCodex, "gpt-6-astra"))
	assert.False(t, ShouldChannelUseResponses(dto.ChannelSettings{}, 20, constant.ChannelTypeWorkBuddy, "gpt-6-astra"))

	// 客户端端点配置不能改变只接受聊天协议的上游。
	override := dto.ChannelSettings{ModelEndpoints: map[string]string{"gpt-6-astra": "openai-response"}}
	assert.False(t, ShouldChannelUseResponses(override, 20, constant.ChannelTypeWorkBuddy, "gpt-6-astra"))
	assert.True(t, ShouldChannelUseResponses(override, 11, constant.ChannelTypeOpenAI, "gpt-6-astra"))
	assert.True(t, ShouldChannelUseResponses(override, 4, constant.ChannelTypeCodex, "gpt-6-astra"))
}
