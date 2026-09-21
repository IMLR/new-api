package cline

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultQuotaCooldown applies when a daily cap error carries no reset hint.
	DefaultQuotaCooldown = time.Hour
	MinimumQuotaCooldown = time.Minute
	MaximumQuotaCooldown = 48 * time.Hour
)

var (
	retryAfterClause = regexp.MustCompile(`(?i)try again in\s+([0-9dhms\s]+)`)
	retryAfterUnit   = regexp.MustCompile(`(?i)(\d+)\s*(d|h|m|s)`)
)

// IsDailyFreeLimit reports whether an upstream error is the per-account daily
// cap on one model rather than ordinary throttling.
func IsDailyFreeLimit(code, message string) bool {
	if strings.EqualFold(strings.TrimSpace(code), "INFERENCE_CAP_ERROR") {
		return true
	}
	return strings.Contains(strings.ToLower(message), "daily free limit")
}

// ParseQuotaWindow reads the "Try again in 20h 4m" hint that Cline appends to
// daily cap errors.
func ParseQuotaWindow(message string) (time.Duration, bool) {
	clause := retryAfterClause.FindStringSubmatch(message)
	if clause == nil {
		return 0, false
	}
	var window time.Duration
	for _, unit := range retryAfterUnit.FindAllStringSubmatch(clause[1], -1) {
		amount, err := strconv.Atoi(unit[1])
		if err != nil || amount <= 0 {
			continue
		}
		switch strings.ToLower(unit[2]) {
		case "d":
			window += time.Duration(amount) * 24 * time.Hour
		case "h":
			window += time.Duration(amount) * time.Hour
		case "m":
			window += time.Duration(amount) * time.Minute
		case "s":
			window += time.Duration(amount) * time.Second
		}
	}
	if window <= 0 {
		return 0, false
	}
	return window, true
}

// QuotaWindow picks the cooldown length for a daily cap error, applying the
// default and the sanity limits.
func QuotaWindow(message string) time.Duration {
	window, ok := ParseQuotaWindow(message)
	if !ok {
		window = DefaultQuotaCooldown
	}
	if window < MinimumQuotaCooldown {
		window = MinimumQuotaCooldown
	}
	if window > MaximumQuotaCooldown {
		window = MaximumQuotaCooldown
	}
	return window
}
