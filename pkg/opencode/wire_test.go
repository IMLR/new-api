package opencode

import "testing"

func TestWireForModel(t *testing.T) {
	cases := map[string]Wire{
		"kimi-k3":                    WireChatCompletions,
		"glm-5.3":                    WireChatCompletions,
		"deepseek-v4-flash":          WireChatCompletions,
		"mimo-v2.5-pro":              WireChatCompletions,
		"hy3":                        WireChatCompletions,
		"qwen3.5-plus":               WireChatCompletions,
		"qwen3.7-plus":               WireChatCompletions,
		"qwen3.8-max":                WireChatCompletions,
		"qwen3.8-flash":              WireAnthropicMessages,
		"minimax-m3":                 WireAnthropicMessages,
		"grok-4.6":                   WireOpenAIResponses,
		"gpt-5.6-luna":               WireOpenAIResponses,
		"muse-spark-1.3-contributor": WireOpenAIResponses,
		"opencode-go/kimi-k3":        WireChatCompletions,
		"opencode-go/grok-4.7":       WireOpenAIResponses,
		"OPencode-go/QWEN3.8-Flash":  WireAnthropicMessages,
		"OPencode-go/QWEN3.8-Max":    WireChatCompletions,
		"":                           WireChatCompletions,
	}
	for model, want := range cases {
		if got := WireForModel(model); got != want {
			t.Errorf("WireForModel(%q) = %q, want %q", model, got, want)
		}
	}
}

func TestWirePath(t *testing.T) {
	cases := map[Wire]string{
		WireChatCompletions:   "/v1/chat/completions",
		WireAnthropicMessages: "/v1/messages",
		WireOpenAIResponses:   "/v1/responses",
	}
	for wire, want := range cases {
		if got := wire.Path(); got != want {
			t.Errorf("%q path = %q, want %q", wire, got, want)
		}
	}
}
