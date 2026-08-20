package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"voltforge/internal/circuit"
	"voltforge/internal/domain"
	"voltforge/internal/storage"
)

func newDispSvcWithBreaker(t *testing.T) (*MitigationService, *circuit.Breaker, storage.Store, *domain.FakeClock, *EventBus, context.Context) {
	t.Helper()
	store, clock, bus, ctx, _ := setupTestEnv(t)
	breaker := circuit.New(circuit.Config{
		Name: "disp-test", MaxRequests: 1, Timeout: 200 * time.Millisecond,
		FailureThreshold: 3, FailureRatio: 0.5,
	})
	dispSvc := NewMitigationService(store, clock, bus).WithBreaker(breaker)
	return dispSvc, breaker, store, clock, bus, ctx
}

func TestCircuitBreakerOpensOnRepeatedExecuteFailures(t *testing.T) {
	dispSvc, breaker, store, clock, bus, ctx := newDispSvcWithBreaker(t)
	handSvc := NewHandshakeService(store, clock, bus)
	registerTestHandshake(t, ctx, handSvc, "F001", "2026-08-19", "R001")
	sessions, _, err := store.ChargeSessionRepo().List(ctx, storage.SessionFilter{PageSize: 100})
	require.NoError(t, err)
	require.NotEmpty(t, sessions, "need a requested session to attach mitigation")

	disp, err := dispSvc.Submit(ctx, SubmitSafetyMitigation{
		SessionID: sessions[0].ID, Type: domain.MitigationTypeThermalThrottle, SubmittedBy: "op",
	})
	require.NoError(t, err)
	require.Equal(t, domain.MitigationStatePending, disp.State)

	for i := 0; i < 3; i++ {
		_, execErr := dispSvc.Execute(ctx, disp.ID, "op")
		require.Error(t, execErr, "execute attempt %d should fail (pending->executing illegal)", i)
	}
	assert.Equal(t, "open", breaker.State(), "breaker should be open after threshold failures")

	_, err = dispSvc.Execute(ctx, disp.ID, "op")
	assert.ErrorIs(t, err, circuit.ErrCircuitOpen, "open circuit should short-circuit with ErrCircuitOpen")
}

func TestCircuitBreakerRecoversAfterTimeout(t *testing.T) {
	dispSvc, breaker, store, clock, bus, ctx := newDispSvcWithBreaker(t)
	handSvc := NewHandshakeService(store, clock, bus)
	registerTestHandshake(t, ctx, handSvc, "F002", "2026-08-19", "R002")
	sessions, _, _ := store.ChargeSessionRepo().List(ctx, storage.SessionFilter{PageSize: 100})
	require.NotEmpty(t, sessions)
	disp, err := dispSvc.Submit(ctx, SubmitSafetyMitigation{
		SessionID: sessions[0].ID, Type: domain.MitigationTypeThermalThrottle, SubmittedBy: "op",
	})
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		dispSvc.Execute(ctx, disp.ID, "op")
	}
	require.Equal(t, "open", breaker.State())
	time.Sleep(250 * time.Millisecond)
	_, err = dispSvc.Execute(ctx, disp.ID, "op")
	assert.NotErrorIs(t, err, circuit.ErrCircuitOpen, "after timeout breaker should allow half-open trial")
}

func TestCircuitBreakerTransparentWhenNil(t *testing.T) {
	store, _, bus, ctx, _ := setupTestEnv(t)
	dispSvc := NewMitigationService(store, nil, bus)
	_, err := dispSvc.Execute(ctx, "nonexistent", "op")
	assert.Error(t, err, "execute with nil breaker should still return underlying error")
}
