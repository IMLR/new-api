package cline

import (
	"net/http"
	"time"

	clineapi "github.com/QuantumNous/new-api/pkg/cline"
)

// CooldownUntil turns a Cline daily cap error into the time when the account
// can serve the model again. The second result is false for errors that are not
// bound to a daily free limit, which keeps transient throttling on the normal
// retry path.
func (e *UpstreamError) CooldownUntil(now time.Time) (time.Time, bool) {
	if e == nil || e.StatusCode != http.StatusTooManyRequests || !clineapi.IsDailyFreeLimit(e.Code, e.Message) {
		return time.Time{}, false
	}
	return now.Add(clineapi.QuotaWindow(e.Message)), true
}
