package thermal

import (
	"context"
	"errors"
	"testing"
)

func controller(t *testing.T) *Controller {
	t.Helper()
	c, err := NewController(Policy{WarnBatteryC: 38, ThrottleBatteryC: 42, ShutdownBatteryC: 48, WarnAdapterC: 40, ThrottleAdapterC: 48, ShutdownAdapterC: 55})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestControllerReturnsProgressiveDerating(t *testing.T) {
	c := controller(t)
	cases := []struct {
		name       string
		sample     Sample
		status     string
		multiplier float64
	}{
		{"normal", Sample{BatteryC: 30, AdapterC: 35}, "normal", 1},
		{"warning", Sample{BatteryC: 39, AdapterC: 35}, "warming", 0.8},
		{"throttle", Sample{BatteryC: 43, AdapterC: 35}, "throttled", 0.5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := c.Decide(context.Background(), tc.sample)
			if err != nil || got.Status != tc.status || got.Multiplier != tc.multiplier {
				t.Fatalf("got=%+v err=%v", got, err)
			}
		})
	}
}

func TestControllerShutsDownAtEitherCriticalSensor(t *testing.T) {
	c := controller(t)
	got, err := c.Decide(context.Background(), Sample{BatteryC: 30, AdapterC: 55})
	if !errors.Is(err, ErrEmergencyShutdown) || got.Status != "shutdown" || got.Multiplier != 0 {
		t.Fatalf("unexpected emergency decision: %+v %v", got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Decide(ctx, Sample{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}
