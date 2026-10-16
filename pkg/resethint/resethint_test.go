package resethint

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseReadsProviderResetHints(t *testing.T) {
	now := time.Date(2026, 10, 16, 14, 33, 25, 0, time.UTC)
	for _, tc := range []struct {
		name       string
		message    string
		retryAfter string
		want       time.Time
	}{
		{"WorkBuddy absolute reset", "bad response status code 429, message: code=6004 msg=usage exceeds frequency limit, but don't worry, your usage will reset at 2026-10-17 18:09:40 UTC+8, alternatively, you can switch to the other models to continue using it.", "", time.Date(2026, 10, 17, 10, 9, 40, 0, time.UTC)},
		{"ISO reset", `{"error":{"message":"Quota exhausted, resets at 2026-10-17T10:09:40Z"}}`, "", time.Date(2026, 10, 17, 10, 9, 40, 0, time.UTC)},
		{"numeric offset", "available after 2026/10/17 18:09 +0800", "", time.Date(2026, 10, 17, 10, 9, 0, 0, time.UTC)},
		{"Chinese absolute uses server zone", "额度已用完，将于 2026-10-17 18:09:40 重置", "", time.Date(2026, 10, 17, 18, 9, 40, 0, time.Local)},
		{"Cline daily cap", "Error 429: Daily free limit reached on model x. Try again in 20h 4m", "", now.Add(20*time.Hour + 4*time.Minute)},
		{"Vercel rate limit", "Rate limit exceeded: this team's limit of 100000000 input tokens per minute was reached. Retry after 55s.", "", now.Add(55 * time.Second)},
		{"written units", "Please retry in 1 day and 2 hours", "", now.Add(26 * time.Hour)},
		{"compact decimal units", "Rate limit reached for gpt-4o. Please try again in 1m30.5s.", "", now.Add(90*time.Second + 500*time.Millisecond)},
		{"milliseconds", "Please try again in 20ms.", "", now.Add(20 * time.Millisecond)},
		{"Chinese relative", "请求过于频繁，请 1小时20分钟后重试", "", now.Add(80 * time.Minute)},
		{"header seconds", "Too many requests", "20", now.Add(20 * time.Second)},
		{"header date", "Too many requests", now.Add(30 * time.Second).Format(http.TimeFormat), now.Add(30 * time.Second)},
		{"later hint wins", "Retry after 10s.", "25", now.Add(25 * time.Second)},
		{"overflow is capped", "Retry after 999999999999999999999999 seconds", "999999999999999999999999", now.Add(Maximum)},
		{"far reset is capped", "reset at 2099-01-01 00:00:00 UTC", "", now.Add(Maximum)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			until, ok := Parse(tc.message, tc.retryAfter, now)
			assert.True(t, ok)
			assert.True(t, tc.want.Equal(until), "want %s, got %s", tc.want, until)
		})
	}
}

func TestParseIgnoresMessagesWithoutResetHint(t *testing.T) {
	now := time.Date(2026, 10, 16, 14, 33, 25, 0, time.UTC)
	for _, tc := range []struct {
		name       string
		message    string
		retryAfter string
	}{
		{"no time", "code=429 msg=too many requests, try again in a few minutes", ""},
		{"unit runs into a word", "retry in 3 models", ""},
		{"unrelated timestamp", "request 2026-10-17 18:09:40 failed: invalid parameters", ""},
		{"reset already passed", "your usage will reset at 2026-10-16 22:00:00 UTC+8", ""},
		{"invalid offset", "reset at 2026-10-17 18:09:40 UTC+25", ""},
		{"expired header date", "Too many requests", now.Add(-time.Second).Format(http.TimeFormat)},
		{"negative header", "Too many requests", "-5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := Parse(tc.message, tc.retryAfter, now)
			assert.False(t, ok)
		})
	}
}
