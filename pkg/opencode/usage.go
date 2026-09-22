package opencode

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// The subscription reports three windows: a rolling five hour window, a weekly
// window and a monthly window. Window lengths are not part of the payload, they
// come from the OpenCode Go documentation.
type UsageWindow struct {
	Name           string  `json:"name"`
	UsedPercent    float64 `json:"used_percent"`
	ResetInSeconds int64   `json:"reset_in_seconds"`
}

// Usage is the parsed /v1/usage payload. Windows the upstream omits stay nil so
// the channel view can tell "no window" apart from "zero percent".
type Usage struct {
	Rolling *UsageWindow    `json:"rolling,omitempty"`
	Weekly  *UsageWindow    `json:"weekly,omitempty"`
	Monthly *UsageWindow    `json:"monthly,omitempty"`
	Raw     json.RawMessage `json:"raw,omitempty"`
}

// UsageWindows returns the windows in display order.
func (u *Usage) UsageWindows() []UsageWindow {
	if u == nil {
		return nil
	}
	windows := make([]UsageWindow, 0, 3)
	for _, window := range []*UsageWindow{u.Rolling, u.Weekly, u.Monthly} {
		if window != nil {
			windows = append(windows, *window)
		}
	}
	return windows
}

// ResetAt converts the reset countdown into an absolute time. The second result
// is false when the payload carries no countdown.
func (u *Usage) ResetAt(name string, now time.Time) (time.Time, bool) {
	if u == nil {
		return time.Time{}, false
	}
	for _, window := range u.UsageWindows() {
		if window.Name != name || window.ResetInSeconds <= 0 {
			continue
		}
		return now.Add(time.Duration(window.ResetInSeconds) * time.Second), true
	}
	return time.Time{}, false
}

var usageWindows = []struct {
	name string
	keys []string
}{
	{"rolling", []string{"rolling", "rollingUsage", "rolling_usage", "rollingWindow", "rolling_window", "fiveHour", "five_hour"}},
	{"weekly", []string{"weekly", "weeklyUsage", "weekly_usage", "weeklyWindow", "weekly_window"}},
	{"monthly", []string{"monthly", "monthlyUsage", "monthly_usage", "monthlyWindow", "monthly_window"}},
}

var usageEnvelopes = []string{"usage", "data", "result", "billing", "payload"}

var usagePercentKeys = []string{
	"usedPercent", "usagePercent", "percentUsed", "percent",
	"used_percent", "usage_percent", "utilizationPercent", "utilization_percent",
	"utilization", "usage",
}

var usageUsedKeys = []string{"used", "usage", "consumed", "count", "usedTokens", "used_tokens"}
var usageLimitKeys = []string{"limit", "total", "quota", "max", "cap", "tokenLimit", "token_limit"}

var usageResetSecondKeys = []string{
	"resetInSec", "resetInSeconds", "resetSeconds", "resetIn",
	"reset_sec", "reset_in_sec", "reset_in_seconds", "resetsInSec", "resetsInSeconds",
}

var usageResetAtKeys = []string{
	"resetAt", "resetsAt", "reset_at", "resets_at", "nextReset", "next_reset", "renewAt", "renew_at",
}

// ParseUsage reads the /v1/usage payload. The published payload shape is
// `{"usage":{"rolling":{"percent":3,"resetInSec":18100},...}}`; the parser also
// accepts the windows at the top level and the envelope keys the console page
// has been observed to add, because no official schema is published.
func ParseUsage(body []byte) (*Usage, error) {
	var payload map[string]any
	if common.Unmarshal(body, &payload) != nil || payload == nil {
		return nil, fmt.Errorf("Invalid OpenCode Go usage payload")
	}
	usage := &Usage{Raw: json.RawMessage(append([]byte(nil), body...))}
	scope := locateUsageScope(payload)
	for _, spec := range usageWindows {
		window := parseUsageWindow(spec.name, firstObject(scope, spec.keys))
		if window == nil {
			continue
		}
		switch spec.name {
		case "rolling":
			usage.Rolling = window
		case "weekly":
			usage.Weekly = window
		case "monthly":
			usage.Monthly = window
		}
	}
	if len(usage.UsageWindows()) == 0 {
		return nil, fmt.Errorf("OpenCode Go usage payload carries no rolling, weekly or monthly window")
	}
	return usage, nil
}

// locateUsageScope finds the object that holds the windows. An inner "usage"
// object wins over the document root.
func locateUsageScope(payload map[string]any) map[string]any {
	for _, key := range usageEnvelopes {
		if inner, ok := payload[key].(map[string]any); ok {
			if hasUsageWindow(inner) {
				return inner
			}
			if nested := locateUsageScope(inner); nested != nil {
				return nested
			}
		}
	}
	return payload
}

func hasUsageWindow(payload map[string]any) bool {
	for _, spec := range usageWindows {
		if firstObject(payload, spec.keys) != nil {
			return true
		}
	}
	return false
}

func parseUsageWindow(name string, window map[string]any) *UsageWindow {
	if window == nil {
		return nil
	}
	percent, ok := windowPercent(window)
	if !ok {
		return nil
	}
	switch {
	case percent < 0:
		percent = 0
	case percent > 100:
		percent = 100
	}
	return &UsageWindow{
		Name:           name,
		UsedPercent:    math.Round(percent*100) / 100,
		ResetInSeconds: windowResetSeconds(window),
	}
}

// windowPercent reads the consumed share of one window. A stated percent is
// already a percentage, so no fraction rescaling is applied; a payload that
// only carries used and limit is divided into a percentage here.
func windowPercent(window map[string]any) (float64, bool) {
	for _, key := range usagePercentKeys {
		if value, ok := number(window[key]); ok {
			return value, true
		}
	}
	used, usedOK := firstNumber(window, usageUsedKeys)
	limit, limitOK := firstNumber(window, usageLimitKeys)
	if usedOK && limitOK && limit > 0 {
		return used / limit * 100, true
	}
	return 0, false
}

// windowResetSeconds reads the reset either as a countdown or as an instant.
func windowResetSeconds(window map[string]any) int64 {
	if seconds, ok := firstNumber(window, usageResetSecondKeys); ok && seconds > 0 {
		return int64(seconds)
	}
	for _, key := range usageResetAtKeys {
		instant, ok := timeValue(window[key])
		if !ok {
			continue
		}
		if seconds := int64(time.Until(instant).Seconds()); seconds > 0 {
			return seconds
		}
	}
	return 0
}

func firstObject(payload map[string]any, keys []string) map[string]any {
	for _, key := range keys {
		if value, ok := payload[key].(map[string]any); ok {
			return value
		}
	}
	return nil
}

func firstNumber(payload map[string]any, keys []string) (float64, bool) {
	for _, key := range keys {
		if value, ok := number(payload[key]); ok {
			return value, true
		}
	}
	return 0, false
}

func number(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return 0, false
		}
		return typed, true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return 0, false
		}
		return parsed, true
	}
	return 0, false
}

func timeValue(value any) (time.Time, bool) {
	if seconds, ok := number(value); ok {
		if seconds > 1e12 {
			return time.UnixMilli(int64(seconds)), true
		}
		if seconds > 1e9 {
			return time.Unix(int64(seconds), 0), true
		}
		return time.Time{}, false
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(text))
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}
