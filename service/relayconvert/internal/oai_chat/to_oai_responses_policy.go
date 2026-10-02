package oaichat

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service/relayconvert/internal/matcher"
	"github.com/QuantumNous/new-api/setting/model_setting"
)

func ShouldChatCompletionsUseResponsesPolicy(policy model_setting.ChatCompletionsToResponsesPolicy, channelID int, channelType int, model string) bool {
	if !policy.IsChannelEnabled(channelID, channelType) {
		return false
	}
	if !common.ChannelTypeServesOpenAIResponses(channelType) {
		// The upstream cannot read a Responses body, so converting the request
		// only turns a working chat request into an upstream error.
		return false
	}
	return matcher.MatchAnyRegex(policy.ModelPatterns, model)
}

func ShouldChatCompletionsUseResponsesGlobal(channelID int, channelType int, model string) bool {
	return ShouldChatCompletionsUseResponsesPolicy(
		model_setting.GetGlobalSettings().ChatCompletionsToResponsesPolicy,
		channelID,
		channelType,
		model,
	)
}
