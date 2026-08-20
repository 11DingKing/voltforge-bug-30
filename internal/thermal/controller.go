package thermal

import (
	"context"
	"errors"
	"fmt"
)

var ErrEmergencyShutdown = errors.New("thermal emergency shutdown")

type Sample struct {
	BatteryC float64
	AdapterC float64
	AmbientC float64
}

type Policy struct {
	WarnBatteryC     float64
	ThrottleBatteryC float64
	ShutdownBatteryC float64
	WarnAdapterC     float64
	ThrottleAdapterC float64
	ShutdownAdapterC float64
}

type Decision struct {
	Multiplier float64
	Status     string
	Reason     string
}

type Controller struct {
	policy Policy
}

func NewController(policy Policy) (*Controller, error) {
	if policy.WarnBatteryC <= 0 || policy.WarnBatteryC >= policy.ThrottleBatteryC || policy.ThrottleBatteryC >= policy.ShutdownBatteryC {
		return nil, fmt.Errorf("invalid battery thermal thresholds")
	}
	if policy.WarnAdapterC <= 0 || policy.WarnAdapterC >= policy.ThrottleAdapterC || policy.ThrottleAdapterC >= policy.ShutdownAdapterC {
		return nil, fmt.Errorf("invalid adapter thermal thresholds")
	}
	return &Controller{policy: policy}, nil
}

func (c *Controller) Decide(ctx context.Context, sample Sample) (Decision, error) {
	if err := ctx.Err(); err != nil {
		return Decision{}, fmt.Errorf("thermal decision: %w", err)
	}
	if sample.BatteryC >= c.policy.ShutdownBatteryC || sample.AdapterC >= c.policy.ShutdownAdapterC {
		return Decision{Multiplier: 0, Status: "shutdown", Reason: "temperature exceeded shutdown threshold"}, ErrEmergencyShutdown
	}
	if sample.BatteryC >= c.policy.ThrottleBatteryC || sample.AdapterC >= c.policy.ThrottleAdapterC {
		return Decision{Multiplier: 0.5, Status: "throttled", Reason: "temperature exceeded throttle threshold"}, nil
	}
	if sample.BatteryC >= c.policy.WarnBatteryC || sample.AdapterC >= c.policy.WarnAdapterC {
		return Decision{Multiplier: 0.8, Status: "warming", Reason: "temperature exceeded warning threshold"}, nil
	}
	return Decision{Multiplier: 1, Status: "normal", Reason: "temperature within limits"}, nil
}
