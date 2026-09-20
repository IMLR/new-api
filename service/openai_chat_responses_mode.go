package service

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service/relayconvert"
	"github.com/QuantumNous/new-api/setting/model_setting"
)

func ShouldChatCompletionsUseResponsesPolicy(policy model_setting.ChatCompletionsToResponsesPolicy, channelID int, channelType int, model string) bool {
	return relayconvert.ShouldChatCompletionsUseResponsesPolicy(policy, channelID, channelType, model)
}

func ShouldChatCompletionsUseResponsesGlobal(channelID int, channelType int, model string) bool {
	return relayconvert.ShouldChatCompletionsUseResponsesGlobal(channelID, channelType, model)
}

// Channel preferences take precedence over the global conversion policy.
func ShouldChannelUseResponses(settings dto.ChannelSettings, channelID, channelType int, model string) bool {
	if endpoint := settings.ModelEndpoints[model]; endpoint != "" {
		return endpoint == string(constant.EndpointTypeOpenAIResponse)
	}
	return ShouldChatCompletionsUseResponsesGlobal(channelID, channelType, model)
}
