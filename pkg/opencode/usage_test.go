package opencode

import (
	"testing"
	"time"
)

func TestParseUsageDocumentedShape(t *testing.T) {
	body := []byte(`{"usage":{
		"rolling":{"percent":3,"resetInSec":18100},
		"weekly":{"percent":1,"resetInSec":266500},
		"monthly":{"percent":0,"resetInSec":1539100}
	}}`)
	usage, err := ParseUsage(body)
	if err != nil {
		t.Fatalf("ParseUsage failed: %v", err)
	}
	windows := usage.UsageWindows()
	if len(windows) != 3 {
		t.Fatalf("got %d windows, want 3", len(windows))
	}
	if windows[0].Name != "rolling" || windows[0].UsedPercent != 3 || windows[0].ResetInSeconds != 18100 {
		t.Fatalf("unexpected rolling window: %+v", windows[0])
	}
	if windows[1].UsedPercent != 1 || windows[2].UsedPercent != 0 {
		t.Fatalf("unexpected weekly/monthly windows: %+v", windows)
	}
	if len(usage.Raw) == 0 {
		t.Fatal("raw payload was not kept")
	}
}

func TestParseUsageTopLevelAndRootEnvelope(t *testing.T) {
	body := []byte(`{"data":{"rollingUsage":{"usagePercent":"25.5","resetInSeconds":60}}}`)
	usage, err := ParseUsage(body)
	if err != nil {
		t.Fatalf("ParseUsage failed: %v", err)
	}
	if usage.Rolling == nil || usage.Rolling.UsedPercent != 25.5 || usage.Rolling.ResetInSeconds != 60 {
		t.Fatalf("unexpected rolling window: %+v", usage.Rolling)
	}
	if usage.Weekly != nil || usage.Monthly != nil {
		t.Fatalf("absent windows must stay nil: %+v", usage)
	}
}

func TestParseUsageUsedOverLimit(t *testing.T) {
	body := []byte(`{"rolling":{"used":3,"limit":12},"weekly":{"used":0,"total":30}}`)
	usage, err := ParseUsage(body)
	if err != nil {
		t.Fatalf("ParseUsage failed: %v", err)
	}
	if usage.Rolling.UsedPercent != 25 {
		t.Fatalf("rolling percent = %v, want 25", usage.Rolling.UsedPercent)
	}
	if usage.Weekly.UsedPercent != 0 {
		t.Fatalf("weekly percent = %v, want 0", usage.Weekly.UsedPercent)
	}
}

func TestParseUsageResetInstant(t *testing.T) {
	reset := time.Now().Add(90 * time.Minute).UTC().Format(time.RFC3339)
	body := []byte(`{"rolling":{"percent":50,"resetsAt":"` + reset + `"}}`)
	usage, err := ParseUsage(body)
	if err != nil {
		t.Fatalf("ParseUsage failed: %v", err)
	}
	if usage.Rolling.ResetInSeconds < 5300 || usage.Rolling.ResetInSeconds > 5500 {
		t.Fatalf("reset countdown = %d, want about 5400", usage.Rolling.ResetInSeconds)
	}
	at, ok := usage.ResetAt("rolling", time.Now())
	if !ok || at.Before(time.Now()) {
		t.Fatalf("ResetAt = %v, %v", at, ok)
	}
}

func TestParseUsageClampsPercent(t *testing.T) {
	usage, err := ParseUsage([]byte(`{"monthly":{"percent":140}}`))
	if err != nil {
		t.Fatalf("ParseUsage failed: %v", err)
	}
	if usage.Monthly.UsedPercent != 100 {
		t.Fatalf("percent = %v, want 100", usage.Monthly.UsedPercent)
	}
	if _, ok := usage.ResetAt("monthly", time.Now()); ok {
		t.Fatal("ResetAt reported a reset without a countdown")
	}
}

func TestParseUsageRejectsUnknownPayload(t *testing.T) {
	if _, err := ParseUsage([]byte(`{"status":"ok"}`)); err == nil {
		t.Fatal("expected an error for a payload without windows")
	}
	if _, err := ParseUsage([]byte(`not json`)); err == nil {
		t.Fatal("expected an error for a non JSON payload")
	}
}
