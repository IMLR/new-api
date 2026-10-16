package cline

import (
	"strings"
	"time"

	"github.com/QuantumNous/new-api/pkg/resethint"
)

const (
	// DefaultQuotaCooldown applies when a daily cap error carries no reset hint.
	DefaultQuotaCooldown = time.Hour
	MinimumQuotaCooldown = time.Minute
)

// IsDailyFreeLimit reports whether an upstream error is the per-account daily
// cap on one model rather than ordinary throttling.
func IsDailyFreeLimit(code, message string) bool {
	if strings.EqualFold(strings.TrimSpace(code), "INFERENCE_CAP_ERROR") {
		return true
	}
	return strings.Contains(strings.ToLower(message), "daily free limit")
}

// CooldownUntil applies Cline's daily-cap rules on top of the shared reset
// hints: a daily cap rests for at least a minute, and for an hour when it
// states no reset time. Other errors need an explicit hint.
func CooldownUntil(code, message, retryAfter string, now time.Time) (time.Time, bool) {
	until, ok := resethint.Parse(message, retryAfter, now)
	if !IsDailyFreeLimit(code, message) {
		return until, ok
	}
	if !ok {
		until = now.Add(DefaultQuotaCooldown)
	}
	if until.Before(now.Add(MinimumQuotaCooldown)) {
		until = now.Add(MinimumQuotaCooldown)
	}
	return until, true
}
