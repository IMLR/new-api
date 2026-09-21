package cline

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	defaultCapCooldown = time.Hour
	minimumCapCooldown = time.Minute
	maximumCapCooldown = 48 * time.Hour
)

var (
	retryAfterClause = regexp.MustCompile(`(?i)try again in\s+([0-9dhms\s]+)`)
	retryAfterUnit   = regexp.MustCompile(`(?i)(\d+)\s*(d|h|m|s)`)
)

// CooldownUntil turns a Cline daily cap error into the time when the account
// can serve the model again. The second result is false for errors that are not
// bound to a daily free limit, which keeps transient throttling on the normal
// retry path.
func (e *UpstreamError) CooldownUntil(now time.Time) (time.Time, bool) {
	if e == nil || e.StatusCode != http.StatusTooManyRequests || !isDailyFreeLimit(e) {
		return time.Time{}, false
	}
	window := defaultCapCooldown
	if parsed, ok := parseRetryAfter(e.Message); ok {
		window = parsed
	}
	if window < minimumCapCooldown {
		window = minimumCapCooldown
	}
	if window > maximumCapCooldown {
		window = maximumCapCooldown
	}
	return now.Add(window), true
}

func isDailyFreeLimit(e *UpstreamError) bool {
	if strings.EqualFold(strings.TrimSpace(e.Code), "INFERENCE_CAP_ERROR") {
		return true
	}
	return strings.Contains(strings.ToLower(e.Message), "daily free limit")
}

// parseRetryAfter reads the "Try again in 20h 4m" hint that Cline appends to
// quota errors.
func parseRetryAfter(message string) (time.Duration, bool) {
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
