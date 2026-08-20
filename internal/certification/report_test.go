package certification

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRuleSetCertifiesCompleteFreshEvidence(t *testing.T) {
	now := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	rules, err := NewRuleSet(24*time.Hour, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	report, err := rules.Evaluate(context.Background(), "phone-x", Evidence{ProtocolOK: true, CableOK: true, ThermalOK: true, PowerDisplayOK: true, TelemetryFresh: true, CheckedAt: now, ExpiresAt: now.Add(72 * time.Hour)})
	if err != nil || report.Status != "certified" || len(report.Reasons) != 0 {
		t.Fatalf("report=%+v err=%v", report, err)
	}
}

func TestRuleSetExplainsEveryMissingEvidence(t *testing.T) {
	now := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	rules, _ := NewRuleSet(time.Hour, func() time.Time { return now })
	report, err := rules.Evaluate(context.Background(), "phone-x", Evidence{CheckedAt: now, ExpiresAt: now.Add(10 * time.Minute)})
	if err != nil || report.Status != "retest_required" || len(report.Reasons) != 6 {
		t.Fatalf("report=%+v err=%v", report, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := rules.Evaluate(ctx, "phone-x", Evidence{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}
