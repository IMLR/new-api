package cline

import (
	"time"

	clineapi "github.com/QuantumNous/new-api/pkg/cline"
)

// CooldownUntil turns daily-cap and rate-limit hints into a per-model reset
// time. A rate limit without a reset hint does not invent a cooldown window.
func (e *UpstreamError) CooldownUntil(now time.Time) (time.Time, bool) {
	if e == nil {
		return time.Time{}, false
	}
	return clineapi.CooldownUntil(e.StatusCode, e.Code, e.Message, e.RetryAfter, now)
}
