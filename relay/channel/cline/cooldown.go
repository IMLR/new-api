package cline

import (
	"time"

	clineapi "github.com/QuantumNous/new-api/pkg/cline"
)

// CooldownUntil turns daily-cap and reset hints into a per-model reset time.
// An error without a reset hint does not invent a cooldown window, except the
// daily cap.
func (e *UpstreamError) CooldownUntil(now time.Time) (time.Time, bool) {
	if e == nil {
		return time.Time{}, false
	}
	return clineapi.CooldownUntil(e.Code, e.Message, e.RetryAfter, now)
}
