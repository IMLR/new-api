package cline

import (
	"io"
	"net/http"
	"strings"
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

func TestParseErrorBodyReadsHTTPErrorPayload(t *testing.T) {
	now := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)

	// Cline has served the same daily cap message as a plain HTTP error body.
	upstreamErr := parseErrorBody([]byte(`{"error":{"message":"Error 429: Daily free limit reached on model vmc/fireworks-cline-k3-contributor-fallbacks. Try again in 18h 53m","code":"INFERENCE_CAP_ERROR"}}`))
	require.NotNil(t, upstreamErr)
	require.Equal(t, http.StatusTooManyRequests, upstreamErr.StatusCode)
	until, ok := upstreamErr.CooldownUntil(now)
	require.True(t, ok)
	require.Equal(t, now.Add(18*time.Hour+53*time.Minute), until)

	// A string error keeps its message and maps onto the credential status.
	upstreamErr = parseErrorBody([]byte(`{"error":"Unauthorized: re-authenticate your Cline account."}`))
	require.NotNil(t, upstreamErr)
	require.Equal(t, http.StatusUnauthorized, upstreamErr.StatusCode)
	_, ok = upstreamErr.CooldownUntil(now)
	require.False(t, ok)

	// Plain text still becomes an error, without inventing a quota window.
	upstreamErr = parseErrorBody([]byte("Daily free limit reached. Try again in 2h"))
	require.NotNil(t, upstreamErr)
	require.Equal(t, http.StatusTooManyRequests, upstreamErr.StatusCode)
	until, ok = upstreamErr.CooldownUntil(now)
	require.True(t, ok)
	require.Equal(t, now.Add(2*time.Hour), until)

	require.Nil(t, parseErrorBody([]byte("   ")))
}

func TestReadErrorBodyKeepsResponseReadable(t *testing.T) {
	body := `{"error":{"message":"Error 429: Daily free limit reached. Try again in 3h","code":"INFERENCE_CAP_ERROR"}}`
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Content-Type": {"application/json"}, "Content-Length": {"110"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	upstreamErr, err := readErrorBody(resp)
	require.NoError(t, err)
	require.Equal(t, http.StatusTooManyRequests, upstreamErr.StatusCode)

	restored, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, body, string(restored))
	require.Equal(t, int64(len(body)), resp.ContentLength)
	require.Empty(t, resp.Header.Get("Content-Length"))
}
