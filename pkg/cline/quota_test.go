package cline

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseQuotaWindow(t *testing.T) {
	window, ok := ParseQuotaWindow("Try again in 45m")
	require.True(t, ok)
	require.Equal(t, 45*time.Minute, window)

	window, ok = ParseQuotaWindow("Try again in 45 minutes")
	require.True(t, ok)
	require.Equal(t, 45*time.Minute, window)

	window, ok = ParseQuotaWindow("Try again in 1d 2h 30m")
	require.True(t, ok)
	require.Equal(t, 26*time.Hour+30*time.Minute, window)

	_, ok = ParseQuotaWindow("Daily free limit reached")
	require.False(t, ok)
}

func TestQuotaWindowAppliesDefaultAndLimits(t *testing.T) {
	// Missing hint falls back to the default window.
	require.Equal(t, time.Hour, QuotaWindow("Daily free limit reached"))

	// Windows are clamped at both ends.
	require.Equal(t, MaximumQuotaCooldown, QuotaWindow("Try again in 100h"))
	require.Equal(t, MinimumQuotaCooldown, QuotaWindow("Try again in 1s"))
}

func TestCooldownUntilHonorsRateLimitHints(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 44, 0, 0, time.UTC)
	for _, tc := range []struct {
		name       string
		status     int
		code       string
		message    string
		retryAfter string
		window     time.Duration
	}{
		{"observed Vercel limit", 429, "rate_limit_exceeded", "Rate limit exceeded: this team's limit of 100000000 input tokens per minute was reached. Retry after 55s.", "", 55 * time.Second},
		{"written units", 429, "rate_limit_exceeded", "Please retry in 44 seconds.", "", 44 * time.Second},
		{"header seconds", 429, "", "Too many requests", "20", 20 * time.Second},
		{"header date", 429, "", "Too many requests", now.Add(30 * time.Second).Format(http.TimeFormat), 30 * time.Second},
		{"later hint wins", 429, "", "Retry after 10s.", "25", 25 * time.Second},
		{"daily default", 429, "INFERENCE_CAP_ERROR", "Daily free limit reached", "", time.Hour},
		{"daily reset", 429, "INFERENCE_CAP_ERROR", "Try again in 20h 4m", "", 20*time.Hour + 4*time.Minute},
		{"duration overflow", 429, "", "Retry after 999999999999999999999999 seconds", "", 48 * time.Hour},
		{"header overflow", 429, "", "Too many requests", "999999999999999999999999", 48 * time.Hour},
		{"no reset hint", 429, "rate_limit_exceeded", "Too many requests", "", 0},
		{"non-rate error", 503, "", "Retry after 55s.", "", 0},
		{"expired header", 429, "", "Too many requests", now.Add(-time.Second).Format(http.TimeFormat), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			until, ok := CooldownUntil(tc.status, tc.code, tc.message, tc.retryAfter, now)
			if tc.window == 0 {
				assert.False(t, ok)
				return
			}
			require.True(t, ok)
			assert.Equal(t, now.Add(tc.window), until)
		})
	}
}

func TestIsDailyFreeLimit(t *testing.T) {
	require.True(t, IsDailyFreeLimit("INFERENCE_CAP_ERROR", "Error 429: something else"))
	require.True(t, IsDailyFreeLimit("", "Error 429: Daily free limit reached on model x"))
	require.False(t, IsDailyFreeLimit("rate_limit_exceeded", "Too many requests"))
}
