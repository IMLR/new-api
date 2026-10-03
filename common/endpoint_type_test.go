package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
)

func TestWorkBuddyResponsesEndpointsAreScopedToGPTChat(t *testing.T) {
	for _, tc := range []struct {
		model     string
		responses bool
	}{
		{"gpt-6-luna", true},
		{"gpt-5.3-codex", true},
		{"gpt-image-2.5-sunburst", false},
		{"claude-opus-4.6", false},
		{"fast-model", false},
		{"seedance-2.5", false},
	} {
		t.Run(tc.model, func(t *testing.T) {
			endpoints := GetEndpointTypesByChannelType(constant.ChannelTypeWorkBuddy, tc.model)
			if tc.responses {
				assert.Contains(t, endpoints, constant.EndpointTypeOpenAIResponse)
			} else {
				assert.NotContains(t, endpoints, constant.EndpointTypeOpenAIResponse)
			}
		})
	}
	assert.Equal(t, []constant.EndpointType{constant.EndpointTypeOpenAI}, GetEndpointTypesByChannelType(constant.ChannelTypeOpenAI, "gpt-6-luna"))
	assert.Contains(t, GetEndpointTypesByChannelType(constant.ChannelTypeCodex, "gpt-6-luna"), constant.EndpointTypeOpenAIResponse)
}
