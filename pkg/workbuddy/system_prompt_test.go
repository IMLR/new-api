package workbuddy

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareBodyKimiRequiresNonEmptySystem(t *testing.T) {
	for _, tc := range []struct {
		name       string
		messages   string
		configured string
		want       any
	}{
		{"absent", `[{"role":"user","content":"hello"}]`, "", "You are a helpful assistant."},
		{"empty", `[{"role":"system","content":""},{"role":"user","content":"hello"}]`, "", "You are a helpful assistant."},
		{"whitespace", `[{"role":"system","content":" \n "},{"role":"user","content":"hello"}]`, " \t ", "You are a helpful assistant."},
		{"null", `[{"role":"system","content":null},{"role":"user","content":"hello"}]`, "", "You are a helpful assistant."},
		{"missing content", `[{"role":"system"},{"role":"user","content":"hello"}]`, "", "You are a helpful assistant."},
		{"empty blocks", `[{"role":"system","content":[]},{"role":"user","content":"hello"}]`, "", "You are a helpful assistant."},
		{"blank text blocks", `[{"role":"system","content":[{"type":"text","text":" "}]},{"role":"user","content":"hello"}]`, "", "You are a helpful assistant."},
		{"empty developer", `[{"role":"developer","content":""},{"role":"user","content":"hello"}]`, "channel rules", "channel rules"},
		{"channel prompt", `[{"role":"user","content":"hello"}]`, "channel rules", "channel rules"},
		{"caller prompt", `[{"role":"user","content":"hello"},{"role":"system","content":"caller rules"}]`, "channel rules", "caller rules"},
		{"caller text blocks", `[{"role":"system","content":[{"type":"text","text":"caller rules"}]},{"role":"user","content":"hello"}]`, "channel rules", []any{map[string]any{"type": "text", "text": "caller rules"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"model":"kimi-k3","messages":%s}`, tc.messages)
			var parsed map[string]any
			require.NoError(t, common.Unmarshal(PrepareBody([]byte(body), PrepareOptions{Global: true, SystemPrompt: tc.configured}), &parsed))
			messages := parsed["messages"].([]any)
			require.Len(t, messages, 2)
			assert.Equal(t, "system", messages[0].(map[string]any)["role"])
			assert.Equal(t, tc.want, messages[0].(map[string]any)["content"])
			assert.Equal(t, map[string]any{"role": "user", "content": "hello"}, messages[1])
		})
	}
}

func TestPrepareBodyKimiRepairsAllEmptySystemMessages(t *testing.T) {
	body := `{"model":"kimi-k3","messages":[{"role":"system","content":"caller rules"},{"role":"user","content":"hello"},{"role":"developer","content":""}]}`
	var parsed map[string]any
	require.NoError(t, common.Unmarshal(PrepareBody([]byte(body), PrepareOptions{Global: true}), &parsed))
	messages := parsed["messages"].([]any)
	require.Len(t, messages, 3)
	assert.Equal(t, "caller rules", messages[0].(map[string]any)["content"])
	assert.Equal(t, "hello", messages[1].(map[string]any)["content"])
	assert.Equal(t, map[string]any{"role": "system", "content": "You are a helpful assistant."}, messages[2])
}

func TestPrepareBodyGPTKeepsEmptySystemHandling(t *testing.T) {
	for _, messages := range []string{
		`[{"role":"user","content":"hello"}]`,
		`[{"role":"system","content":""},{"role":"user","content":"hello"}]`,
	} {
		body := fmt.Sprintf(`{"model":"gpt-6-luna","messages":%s}`, messages)
		var parsed map[string]any
		require.NoError(t, common.Unmarshal(PrepareBody([]byte(body), PrepareOptions{Global: true}), &parsed))
		assert.Equal(t, "", parsed["messages"].([]any)[0].(map[string]any)["content"])
	}
}
