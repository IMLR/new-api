package cline

import (
	"errors"
	"net/http"
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
	retryAfterClause = regexp.MustCompile(`(?i)\b(?:try again in|retry after|retry in)\s+((?:\d+\s*(?:days?|hours?|minutes?|seconds?|[dhms])\s*)+)`)
	retryAfterUnit   = regexp.MustCompile(`(?i)(\d+)\s*(days?|hours?|minutes?|seconds?|[dhms])`)
)

// IsDailyFreeLimit reports whether an upstream error is the per-account daily
// cap on one model rather than ordinary throttling.
func IsDailyFreeLimit(code, message string) bool {
	if strings.EqualFold(strings.TrimSpace(code), "INFERENCE_CAP_ERROR") {
		return true
	}
	return strings.Contains(strings.ToLower(message), "daily free limit")
}

// ParseQuotaWindow reads both daily-cap hints ("Try again in 20h 4m") and
// rate-limit hints ("Retry after 55s") without overflowing a duration.
func ParseQuotaWindow(message string) (time.Duration, bool) {
	clause := retryAfterClause.FindStringSubmatch(message)
	if clause == nil {
		return 0, false
	}
	var window time.Duration
	for _, unit := range retryAfterUnit.FindAllStringSubmatch(clause[1], -1) {
		amount, err := strconv.ParseUint(unit[1], 10, 64)
		if errors.Is(err, strconv.ErrRange) {
			return MaximumQuotaCooldown, true
		}
		if err != nil || amount == 0 {
			continue
		}
		var scale time.Duration
		switch strings.ToLower(unit[2])[:1] {
		case "d":
			scale = 24 * time.Hour
		case "h":
			scale = time.Hour
		case "m":
			scale = time.Minute
		case "s":
			scale = time.Second
		}
		if amount > uint64((MaximumQuotaCooldown-window)/scale) {
			return MaximumQuotaCooldown, true
		}
		window += time.Duration(amount) * scale
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

// CooldownUntil shares the same reset-time rules between live requests and
// quota probes. Rate limits need an explicit hint; only daily caps fall back to
// an hour. Retry-After accepts seconds or an HTTP date (RFC 9110, section 10.2.3).
func CooldownUntil(statusCode int, code, message, retryAfter string, now time.Time) (time.Time, bool) {
	if statusCode != http.StatusTooManyRequests {
		return time.Time{}, false
	}
	window, _ := ParseQuotaWindow(message)
	retryAfter = strings.TrimSpace(retryAfter)
	if retryAfter != "" {
		var headerWindow time.Duration
		if seconds, err := strconv.ParseUint(retryAfter, 10, 64); err == nil {
			if seconds > uint64(MaximumQuotaCooldown/time.Second) {
				headerWindow = MaximumQuotaCooldown
			} else {
				headerWindow = time.Duration(seconds) * time.Second
			}
		} else if errors.Is(err, strconv.ErrRange) {
			headerWindow = MaximumQuotaCooldown
		} else if date, err := http.ParseTime(retryAfter); err == nil {
			headerWindow = date.Sub(now)
			if headerWindow > MaximumQuotaCooldown {
				headerWindow = MaximumQuotaCooldown
			}
		}
		if headerWindow > window {
			window = headerWindow
		}
	}
	if IsDailyFreeLimit(code, message) {
		if window <= 0 {
			window = DefaultQuotaCooldown
		}
		if window < MinimumQuotaCooldown {
			window = MinimumQuotaCooldown
		}
	}
	if window <= 0 {
		return time.Time{}, false
	}
	return now.Add(window), true
}
