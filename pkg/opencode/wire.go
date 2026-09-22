package opencode

import "strings"

// Wire is the upstream API family that serves one model on OpenCode Go.
//
// The upstream client catalog assigns every Go model to one family: most models
// answer on the OpenAI-compatible chat completions API, MiniMax and Qwen3.8
// Flash on Anthropic Messages, and Grok, GPT 5.6 Luna and Muse Spark on the
// OpenAI Responses API.
type Wire string

const (
	WireChatCompletions   Wire = "chat_completions"
	WireAnthropicMessages Wire = "anthropic_messages"
	WireOpenAIResponses   Wire = "openai_responses"
)

const (
	ChatCompletionsPath   = "/v1/chat/completions"
	AnthropicMessagesPath = "/v1/messages"
	OpenAIResponsesPath   = "/v1/responses"
	// ModelsPath lists every Go model, including the models served on another wire.
	ModelsPath = "/v1/models"
	// UsagePath reports the subscription windows of one API key.
	UsagePath = "/v1/usage"

	ClientPrefix = "opencode-go/"
)

// NormalizeModel removes the client prefix OpenCode configs use, so
// `opencode-go/kimi-k3` and `kimi-k3` reach the gateway as the same id.
func NormalizeModel(model string) string {
	id := strings.TrimSpace(model)
	if trimmed, ok := cutPrefixFold(id, ClientPrefix); ok && trimmed != "" {
		id = trimmed
	}
	return id
}

// cutPrefixFold removes a prefix regardless of letter case.
func cutPrefixFold(text, prefix string) (string, bool) {
	if len(text) < len(prefix) || !strings.EqualFold(text[:len(prefix)], prefix) {
		return text, false
	}
	return text[len(prefix):], true
}

// WireForModel returns the API family of one model ID. Unknown IDs default to
// chat completions, which serves the largest group of Go models. A channel
// model endpoint setting overrides this choice per model.
func WireForModel(model string) Wire {
	id := strings.ToLower(NormalizeModel(model))
	switch {
	case strings.HasPrefix(id, "minimax"), id == "qwen3.8-flash":
		return WireAnthropicMessages
	case strings.HasPrefix(id, "grok"), strings.HasPrefix(id, "muse-spark"), id == "gpt-5.6-luna":
		return WireOpenAIResponses
	}
	return WireChatCompletions
}

// Path returns the upstream path of the wire.
func (w Wire) Path() string {
	switch w {
	case WireAnthropicMessages:
		return AnthropicMessagesPath
	case WireOpenAIResponses:
		return OpenAIResponsesPath
	}
	return ChatCompletionsPath
}

// EndpointName is the label shown in the channel view and in error messages.
func (w Wire) EndpointName() string {
	switch w {
	case WireAnthropicMessages:
		return "Anthropic Messages"
	case WireOpenAIResponses:
		return "OpenAI Responses"
	}
	return "chat completions"
}
