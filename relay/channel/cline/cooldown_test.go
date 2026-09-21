package cline

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCooldownUntilParsesDailyCapWindow(t *testing.T) {
	now := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	upstreamErr := &UpstreamError{
		StatusCode: http.StatusTooManyRequests,
		Code:       "INFERENCE_CAP_ERROR",
		Message:    "Error 429: Daily free limit reached on model vmc/fireworks-cline-k3-contributor-fallbacks. Try again in 20h 4m",
	}
	until, ok := upstreamErr.CooldownUntil(now)
	require.True(t, ok)
	require.Equal(t, now.Add(20*time.Hour+4*time.Minute), until)
}

func TestCooldownUntilFallbacksAndClamps(t *testing.T) {
	now := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)

	// Daily cap without a parseable reset time falls back to one hour.
	until, ok := (&UpstreamError{
		StatusCode: http.StatusTooManyRequests,
		Code:       "INFERENCE_CAP_ERROR",
		Message:    "Error 429: Daily free limit reached",
	}).CooldownUntil(now)
	require.True(t, ok)
	require.Equal(t, now.Add(time.Hour), until)

	// Unknown windows are capped at 48 hours.
	until, ok = (&UpstreamError{
		StatusCode: http.StatusTooManyRequests,
		Code:       "INFERENCE_CAP_ERROR",
		Message:    "Daily free limit reached. Try again in 100h",
	}).CooldownUntil(now)
	require.True(t, ok)
	require.Equal(t, now.Add(48*time.Hour), until)

	// Transient throttling without a daily cap stays on the retry path.
	_, ok = (&UpstreamError{
		StatusCode: http.StatusTooManyRequests,
		Code:       "rate_limit_exceeded",
		Message:    "Too many requests",
	}).CooldownUntil(now)
	require.False(t, ok)

	// Non-429 errors never start a quota window.
	_, ok = (&UpstreamError{
		StatusCode: http.StatusForbidden,
		Message:    "Daily free limit reached. Try again in 5h",
	}).CooldownUntil(now)
	require.False(t, ok)
}

func TestParseRetryAfter(t *testing.T) {
	window, ok := parseRetryAfter("Try again in 45m")
	require.True(t, ok)
	require.Equal(t, 45*time.Minute, window)

	window, ok = parseRetryAfter("Try again in 45 minutes")
	require.True(t, ok)
	require.Equal(t, 45*time.Minute, window)

	window, ok = parseRetryAfter("Try again in 1d 2h 30m")
	require.True(t, ok)
	require.Equal(t, 26*time.Hour+30*time.Minute, window)

	_, ok = parseRetryAfter("Daily free limit reached")
	require.False(t, ok)
}
