package cline

import (
	"testing"
	"time"

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

func TestIsDailyFreeLimit(t *testing.T) {
	require.True(t, IsDailyFreeLimit("INFERENCE_CAP_ERROR", "Error 429: something else"))
	require.True(t, IsDailyFreeLimit("", "Error 429: Daily free limit reached on model x"))
	require.False(t, IsDailyFreeLimit("rate_limit_exceeded", "Too many requests"))
}
