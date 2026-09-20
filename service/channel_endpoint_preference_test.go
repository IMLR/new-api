package service

import (
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
