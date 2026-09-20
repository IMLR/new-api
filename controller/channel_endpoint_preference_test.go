package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestChannelTestUsesSavedModelEndpoint(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Setting: common.GetPointer(`{"model_endpoints":{"gpt-6-astra":"openai-response"}}`)}
	assert.Equal(t, "openai-response", normalizeChannelTestEndpoint(channel, "gpt-6-astra", ""))
	assert.Equal(t, "openai", normalizeChannelTestEndpoint(channel, "gpt-6-astra", "openai"))
	assert.Empty(t, normalizeChannelTestEndpoint(channel, "other", ""))
}
