package opencode

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// ParseKey normalizes one OpenCode Go API key. A plain key is returned as is;
// an entry copied out of the OpenCode credential file (`auth.json`) or a small
// JSON object is unwrapped so the key can be pasted without editing.
func ParseKey(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("OpenCode Go API key is empty")
	}
	if !strings.HasPrefix(trimmed, "{") {
		return trimmed, nil
	}
	var obj map[string]any
	if common.UnmarshalJsonStr(trimmed, &obj) != nil || obj == nil {
		return "", fmt.Errorf("OpenCode Go key must be an API key or a JSON object")
	}
	if nested, ok := obj["opencode-go"].(map[string]any); ok {
		if key := jsonKey(nested); key != "" {
			return key, nil
		}
	}
	if key := jsonKey(obj); key != "" {
		return key, nil
	}
	if _, isZen := obj["opencode"]; isZen {
		return "", fmt.Errorf("this is an OpenCode Zen credential; OpenCode Go needs its own subscription key")
	}
	return "", fmt.Errorf("OpenCode Go key JSON must contain key or apiKey")
}

func jsonKey(obj map[string]any) string {
	for _, name := range []string{"key", "apiKey", "api_key", "OPENCODE_API_KEY", "opencodeApiKey"} {
		if value, ok := obj[name].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// MaskKey hides all but the last four characters for display in the console.
func MaskKey(key string) string {
	key = strings.TrimSpace(key)
	if len(key) <= 4 {
		return strings.Repeat("*", len(key))
	}
	return strings.Repeat("*", 4) + key[len(key)-4:]
}
