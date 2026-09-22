package workbuddy

import "strings"

// repackToolResultBlocks moves messages that were inserted between an assistant
// tool call and its results behind the whole result group. OpenAI compatible
// wires require tool results to follow the assistant message directly, and the
// upstream rejects the request when anything sits between them.
func repackToolResultBlocks(messages []any) ([]any, bool) {
	if len(messages) < 3 {
		return messages, false
	}
	out := make([]any, 0, len(messages))
	changed := false
	index := 0
	for index < len(messages) {
		message, ok := messages[index].(map[string]any)
		if !ok || message["role"] != "assistant" {
			out = append(out, messages[index])
			index++
			continue
		}
		calls, hasCalls := message["tool_calls"].([]any)
		if !hasCalls || len(calls) == 0 {
			out = append(out, messages[index])
			index++
			continue
		}
		expected := map[string]bool{}
		for _, raw := range calls {
			if call, ok := raw.(map[string]any); ok {
				if id, _ := call["id"].(string); id != "" {
					expected[id] = true
				}
			}
		}
		out = append(out, messages[index])
		index++
		var results []any
		var between []any
		sawOther := false
		for index < len(messages) {
			current, ok := messages[index].(map[string]any)
			if !ok {
				break
			}
			role, _ := current["role"].(string)
			if role == "tool" {
				id, _ := current["tool_call_id"].(string)
				if !expected[id] {
					break
				}
				results = append(results, messages[index])
				if sawOther {
					changed = true
				}
				index++
				continue
			}
			if len(results) == 0 {
				break
			}
			// A further assistant tool call starts a new group and must stay a
			// group head, otherwise its own results never get repacked.
			if role == "assistant" {
				if next, _ := current["tool_calls"].([]any); len(next) > 0 {
					break
				}
			}
			between = append(between, messages[index])
			sawOther = true
			index++
		}
		out = append(out, results...)
		out = append(out, between...)
	}
	if !changed {
		return messages, false
	}
	return out, true
}

// cleanupOrphanToolCalls drops tool calls without results and tool results
// without calls. Such a history makes the upstream reject every later message
// of the conversation with HTTP 400, so the relay removes the unpaired parts.
func cleanupOrphanToolCalls(messages []any) ([]any, bool) {
	if len(messages) == 0 {
		return messages, false
	}
	callIDs := map[string]bool{}
	resultIDs := map[string]bool{}
	hasTraffic := false
	for _, raw := range messages {
		message, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		switch message["role"] {
		case "tool":
			if id, ok := message["tool_call_id"].(string); ok && id != "" {
				resultIDs[id] = true
				hasTraffic = true
			}
		case "assistant":
			if calls, ok := message["tool_calls"].([]any); ok {
				for _, rawCall := range calls {
					call, ok := rawCall.(map[string]any)
					if !ok {
						continue
					}
					if id, ok := call["id"].(string); ok && id != "" {
						callIDs[id] = true
						hasTraffic = true
					}
				}
			}
		}
	}
	if !hasTraffic {
		return messages, false
	}
	keep := map[string]bool{}
	for id := range callIDs {
		if resultIDs[id] {
			keep[id] = true
		}
	}
	changed := false
	for _, raw := range messages {
		message, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if role, _ := message["role"].(string); role != "assistant" {
			continue
		}
		calls, ok := message["tool_calls"].([]any)
		if !ok || len(calls) == 0 {
			continue
		}
		kept := make([]any, 0, len(calls))
		for _, rawCall := range calls {
			call, ok := rawCall.(map[string]any)
			if !ok {
				continue
			}
			if id, _ := call["id"].(string); keep[id] {
				kept = append(kept, call)
			}
		}
		if len(kept) == len(calls) {
			continue
		}
		changed = true
		if len(kept) == 0 {
			delete(message, "tool_calls")
			continue
		}
		message["tool_calls"] = kept
	}
	kept := make([]any, 0, len(messages))
	for _, raw := range messages {
		message, ok := raw.(map[string]any)
		if !ok {
			kept = append(kept, raw)
			continue
		}
		if role, _ := message["role"].(string); role == "tool" {
			id, _ := message["tool_call_id"].(string)
			if !keep[id] {
				changed = true
				continue
			}
		}
		kept = append(kept, raw)
	}
	if !changed {
		return messages, false
	}
	return kept, true
}

// defaultDeepSeekEffort is the fallback thinking level for DeepSeek models when
// the model catalog declares none.
const defaultDeepSeekEffort = "high"

func isDeepSeekModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "deepseek")
}

// injectThinking turns thinking on for DeepSeek models. The upstream answers
// without a chain of thought unless the request carries thinking.type=enabled
// together with a reasoning effort.
func injectThinking(obj map[string]any, supported map[string][]string, defaults map[string]string) {
	model, _ := obj["model"].(string)
	if !isDeepSeekModel(model) {
		return
	}
	thinking, ok := obj["thinking"].(map[string]any)
	kind := ""
	if ok {
		kind, _ = thinking["type"].(string)
		kind = strings.TrimSpace(kind)
	}
	if kind != "" {
		if strings.EqualFold(kind, "disabled") {
			delete(obj, "reasoning_effort")
			delete(obj, "reasoningEffort")
			return
		}
		ensureDeepSeekEffort(obj, defaults[model])
		return
	}
	if !ok {
		obj["thinking"] = map[string]any{"type": "enabled"}
	} else {
		thinking["type"] = "enabled"
	}
	ensureDeepSeekEffort(obj, defaults[model])
}

func ensureDeepSeekEffort(obj map[string]any, defaultEffort string) {
	if _, ok := obj["reasoning_effort"]; ok {
		return
	}
	if _, ok := obj["reasoningEffort"]; ok {
		return
	}
	if defaultEffort == "" {
		defaultEffort = defaultDeepSeekEffort
	}
	obj["reasoning_effort"] = defaultEffort
}

// backfillReasoningContent keeps assistant messages consistent for DeepSeek:
// every assistant message carries a reasoning_content string once thinking is
// on, otherwise the upstream rejects the replay of a conversation.
func backfillReasoningContent(obj map[string]any) {
	model, _ := obj["model"].(string)
	if !isDeepSeekModel(model) {
		return
	}
	messages, ok := obj["messages"].([]any)
	if !ok || len(messages) == 0 {
		return
	}
	thinkingEnabled := false
	if thinking, ok := obj["thinking"].(map[string]any); ok {
		if kind, _ := thinking["type"].(string); strings.EqualFold(strings.TrimSpace(kind), "enabled") {
			thinkingEnabled = true
		}
	}
	hasTrace := false
	for _, raw := range messages {
		message, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if reasoning, ok := message["reasoning"].(string); ok && reasoning != "" {
			hasTrace = true
			break
		}
		if _, ok := message["reasoning_content"]; ok {
			hasTrace = true
			break
		}
	}
	if !thinkingEnabled && !hasTrace {
		return
	}
	for _, raw := range messages {
		message, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if role, _ := message["role"].(string); role != "assistant" {
			continue
		}
		reasoning, hasReasoning := message["reasoning_content"].(string)
		if !hasReasoning {
			if value, ok := message["reasoning"].(string); ok {
				reasoning = value
				message["reasoning_content"] = reasoning
			} else {
				reasoning = ""
				message["reasoning_content"] = reasoning
			}
		}
		if current, ok := message["reasoning"].(string); ok && current != "" {
			continue
		}
		if reasoning != "" {
			message["reasoning"] = reasoning
		} else {
			// The upstream checks the field is present and non-empty.
			message["reasoning"] = " "
		}
	}
}

// effortRank orders the thinking levels the upstream understands.
var effortRank = map[string]int{"off": 0, "minimal": 1, "low": 2, "medium": 3, "high": 4, "xhigh": 5, "max": 6}

// normalizeReasoningEffort lowers a requested effort to the highest level the
// model supports, so the upstream never sees an unsupported value.
func normalizeReasoningEffort(obj map[string]any, supported map[string][]string) {
	if len(supported) == 0 {
		return
	}
	model, _ := obj["model"].(string)
	if model == "" {
		return
	}
	levels, ok := supported[model]
	if !ok || len(levels) == 0 {
		return
	}
	key := ""
	if _, present := obj["reasoning_effort"]; present {
		key = "reasoning_effort"
	} else if _, present := obj["reasoningEffort"]; present {
		key = "reasoningEffort"
	} else {
		return
	}
	requested, ok := obj[key].(string)
	if !ok {
		return
	}
	requested = strings.TrimSpace(strings.ToLower(requested))
	requestedIndex, known := effortRank[requested]
	if !known {
		return
	}
	best, bestIndex := "", -1
	for _, level := range levels {
		index, known := effortRank[strings.TrimSpace(strings.ToLower(level))]
		if known && index <= requestedIndex && index > bestIndex {
			best, bestIndex = level, index
		}
	}
	if best != "" {
		if !strings.EqualFold(best, requested) {
			obj[key] = best
		}
		return
	}
	lowest, lowestIndex := "", 1<<30
	for _, level := range levels {
		index, known := effortRank[strings.TrimSpace(strings.ToLower(level))]
		if known && index < lowestIndex {
			lowest, lowestIndex = level, index
		}
	}
	if lowest != "" {
		obj[key] = lowest
	}
}
