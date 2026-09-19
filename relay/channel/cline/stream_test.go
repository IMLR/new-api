package cline

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamCollectsToolArgumentsAndUsage(t *testing.T) {
	raw := `data: {"id":"r","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"t","function":{"name":"echo","arguments":"{\"text\":"}}]}}]}

data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"ok\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":12,"completion_tokens":8,"total_tokens":20}}

data: [DONE]
`
	b, err := collectStream(strings.NewReader(raw))
	require.NoError(t, err)
	var result struct {
		Choices []collectedChoice `json:"choices"`
		Usage   struct {
			Total int `json:"total_tokens"`
		} `json:"usage"`
	}
	require.NoError(t, common.Unmarshal(b, &result))
	require.Len(t, result.Choices, 1)
	assert.Equal(t, "tool_calls", result.Choices[0].FinishReason)
	require.Len(t, result.Choices[0].Message.Tools, 1)
	assert.JSONEq(t, `{"text":"ok"}`, result.Choices[0].Message.Tools[0].Function.Arguments)
	assert.Equal(t, 20, result.Usage.Total)
}
func TestStreamRejectsErrorsAndTruncation(t *testing.T) {
	for _, raw := range []string{"data: {\"error\":{\"message\":\"limit\"}}\n", "data: [DONE]\n", "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"}}]}\n", "data: invalid\n"} {
		_, err := collectStream(strings.NewReader(raw))
		assert.Error(t, err)
	}
}
