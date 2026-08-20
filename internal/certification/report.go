package certification

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrNotReady = errors.New("certification evidence is incomplete")

type Evidence struct {
	ProtocolOK     bool
	CableOK        bool
	ThermalOK      bool
	PowerDisplayOK bool
	TelemetryFresh bool
	CheckedAt      time.Time
	ExpiresAt      time.Time
}

type Report struct {
	ProductID string
	Status    string
	Reasons   []string
	CheckedAt time.Time
	ExpiresAt time.Time
}

type RuleSet struct {
	minimumValidity time.Duration
	now             func() time.Time
}

func NewRuleSet(minimumValidity time.Duration, now func() time.Time) (*RuleSet, error) {
	if minimumValidity <= 0 {
		return nil, fmt.Errorf("minimum validity must be positive")
	}
	if now == nil {
		now = time.Now
	}
	return &RuleSet{minimumValidity: minimumValidity, now: now}, nil
}

func (r *RuleSet) Evaluate(ctx context.Context, productID string, evidence Evidence) (Report, error) {
	if err := ctx.Err(); err != nil {
		return Report{}, fmt.Errorf("evaluate certification: %w", err)
	}
	if productID == "" || evidence.CheckedAt.IsZero() {
		return Report{}, ErrNotReady
	}
	now := r.now()
	reasons := make([]string, 0, 5)
	if !evidence.ProtocolOK {
		reasons = append(reasons, "protocol handshake failed")
	}
	if !evidence.CableOK {
		reasons = append(reasons, "cable certificate failed")
	}
	if !evidence.ThermalOK {
		reasons = append(reasons, "thermal control failed")
	}
	if !evidence.PowerDisplayOK {
		reasons = append(reasons, "power display failed")
	}
	if !evidence.TelemetryFresh {
		reasons = append(reasons, "telemetry is stale")
	}
	if evidence.ExpiresAt.Before(now.Add(r.minimumValidity)) {
		reasons = append(reasons, "evidence validity is too short")
	}
	status := "certified"
	if len(reasons) > 0 {
		status = "retest_required"
	}
	return Report{ProductID: productID, Status: status, Reasons: reasons, CheckedAt: now, ExpiresAt: evidence.ExpiresAt}, nil
}
