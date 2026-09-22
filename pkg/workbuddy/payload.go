package workbuddy

import (
	"encoding/json"
	"strings"
)

// PrepareOptions carries everything the body pipeline needs beyond the request
// itself: the model capability tables and the identity used for the prompt
// cache key.
type PrepareOptions struct {
	// SupportedEfforts maps a model id to the thinking levels the upstream
	// accepts. An empty table leaves reasoning_effort untouched.
	SupportedEfforts map[string][]string
	// DefaultEfforts maps a model id to the level the client would pick.
	DefaultEfforts map[string]string
	UID            string
	ConversationID string
	Global         bool
	// Sanitize enables the review fingerprint cleanup.
	Sanitize bool
}

// PrepareBody rewrites a client request into the shape the upstream accepts.
// The upstream always streams, so a non-streaming downstream request is turned
// into a stream here and aggregated again after the answer.
func PrepareBody(src []byte, opts PrepareOptions) []byte {
	if len(src) == 0 {
		return src
	}
	var obj map[string]any
	if err := json.Unmarshal(src, &obj); err != nil {
		return src
	}
	obj["stream"] = true
	translateMaxCompletionTokens(obj)
	if _, present := obj["stream_options"]; !present {
		obj["stream_options"] = map[string]any{"include_usage": true}
	}
	normalizeToolChoice(obj)
	normalizeRoles(obj)
	normalizeImageURL(obj)
	if messages, ok := obj["messages"].([]any); ok {
		// Repair tool call pairing before anything else inspects the messages:
		// an unpaired history makes the upstream reject every later request.
		repaired, _ := repackToolResultBlocks(messages)
		repaired, _ = cleanupOrphanToolCalls(repaired)
		obj["messages"] = repaired
	}
	model, _ := obj["model"].(string)
	injectThinking(obj, opts.SupportedEfforts, opts.DefaultEfforts)
	normalizeReasoningEffort(obj, opts.SupportedEfforts)
	backfillReasoningContent(obj)
	if opts.Sanitize {
		if messages, ok := obj["messages"].([]any); ok {
			sanitizeMessages(messages)
		}
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return src
	}
	out = injectPromptCacheKey(out, opts.UID, conversationOf(obj, opts.ConversationID))
	if opts.Global {
		out = ensureGlobalSystem(out)
	}
	_ = model
	return out
}

// conversationOf prefers the conversation id the client already sends, so the
// cache key follows the client's own notion of a conversation.
func conversationOf(obj map[string]any, fallback string) string {
	if value, ok := obj["conversation_id"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if value, ok := obj["conversationId"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

// injectPromptCacheKey adds the per account cache key, which the upstream uses
// to reuse the prefix cache. A client supplied key always wins.
func injectPromptCacheKey(body []byte, uid, conversation string) []byte {
	if len(body) == 0 {
		return body
	}
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return body
	}
	if existing, ok := obj["prompt_cache_key"].(string); ok && existing != "" {
		return body
	}
	obj["prompt_cache_key"] = CacheKey(uid, conversation)
	out, err := json.Marshal(obj)
	if err != nil {
		return body
	}
	return out
}

// ensureGlobalSystem prepends a system message for global accounts when the
// client sends none, which the international deployment requires.
func ensureGlobalSystem(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return body
	}
	messages, ok := obj["messages"].([]any)
	if !ok || len(messages) == 0 {
		return body
	}
	if first, ok := messages[0].(map[string]any); ok {
		if role, _ := first["role"].(string); strings.EqualFold(strings.TrimSpace(role), "system") {
			return body
		}
	}
	obj["messages"] = append([]any{map[string]any{"role": "system", "content": "You are a helpful assistant."}}, messages...)
	out, err := json.Marshal(obj)
	if err != nil {
		return body
	}
	return out
}

// translateMaxCompletionTokens maps the newer OpenAI alias onto max_tokens,
// which is the only field the upstream reads.
func translateMaxCompletionTokens(obj map[string]any) {
	alias, present := obj["max_completion_tokens"]
	delete(obj, "max_completion_tokens")
	if !present {
		return
	}
	if _, explicit := obj["max_tokens"]; explicit {
		return
	}
	switch value := alias.(type) {
	case float64:
		if value > 0 && value == float64(int64(value)) {
			obj["max_tokens"] = int64(value)
		}
	case int64:
		if value > 0 {
			obj["max_tokens"] = value
		}
	case int:
		if value > 0 {
			obj["max_tokens"] = int64(value)
		}
	}
}

// normalizeRoles rewrites the developer role, which the upstream role
// whitelist rejects, into its OpenAI equivalent system.
func normalizeRoles(obj map[string]any) {
	messages, ok := obj["messages"].([]any)
	if !ok {
		return
	}
	for _, raw := range messages {
		message, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		role, ok := message["role"].(string)
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(role), "developer") {
			message["role"] = "system"
		}
	}
}

// normalizeImageURL turns the string form of an image part into the object form
// the upstream accepts.
func normalizeImageURL(obj map[string]any) {
	messages, ok := obj["messages"].([]any)
	if !ok {
		return
	}
	for _, raw := range messages {
		message, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		parts, ok := message["content"].([]any)
		if !ok {
			continue
		}
		for _, rawPart := range parts {
			part, ok := rawPart.(map[string]any)
			if !ok || part["type"] != "image_url" {
				continue
			}
			url, ok := part["image_url"].(string)
			if !ok || url == "" {
				continue
			}
			part["image_url"] = map[string]any{"url": url}
		}
	}
}

// normalizeToolChoice rewrites tool_choice into the string form the upstream
// accepts and drops the tool list when the client asks for none.
func normalizeToolChoice(obj map[string]any) {
	suppress := func() {
		delete(obj, "tools")
		delete(obj, "functions")
	}
	choice, present := obj["tool_choice"]
	if !present {
		return
	}
	switch value := choice.(type) {
	case string:
		if strings.EqualFold(strings.TrimSpace(value), "none") {
			delete(obj, "tool_choice")
			suppress()
		}
	case map[string]any:
		kind, _ := value["type"].(string)
		kind = strings.ToLower(strings.TrimSpace(kind))
		switch kind {
		case "none":
			delete(obj, "tool_choice")
			suppress()
		case "auto", "required":
			obj["tool_choice"] = kind
		case "function":
			name := ""
			if function, ok := value["function"].(map[string]any); ok {
				name, _ = function["name"].(string)
			}
			if name == "" {
				name, _ = value["name"].(string)
			}
			if name = strings.TrimSpace(name); name != "" {
				obj["tool_choice"] = name
			} else {
				obj["tool_choice"] = "auto"
			}
		default:
			delete(obj, "tool_choice")
		}
	default:
		delete(obj, "tool_choice")
	}
}
