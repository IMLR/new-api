package cline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCooldownUntilAppliesDailyCapRules(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 44, 0, 0, time.UTC)
	for _, tc := range []struct {
		name       string
		code       string
		message    string
		retryAfter string
		window     time.Duration
	}{
		{"daily default", "INFERENCE_CAP_ERROR", "Daily free limit reached", "", time.Hour},
		{"daily reset", "INFERENCE_CAP_ERROR", "Try again in 20h 4m", "", 20*time.Hour + 4*time.Minute},
		{"daily minimum", "", "Daily free limit reached. Try again in 1s", "", time.Minute},
		{"rate limit hint", "rate_limit_exceeded", "Retry after 10s.", "25", 25 * time.Second},
		{"no reset hint", "rate_limit_exceeded", "Too many requests", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			until, ok := CooldownUntil(tc.code, tc.message, tc.retryAfter, now)
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
