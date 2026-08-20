package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"voltforge/internal/domain"
	"voltforge/internal/storage"
)

func setupTestSessionState(t *testing.T, ctx context.Context, store storage.Store, clock *domain.FakeClock, state string) *domain.ChargeSession {
	t.Helper()
	session := &domain.ChargeSession{
		ID: "session-disp-" + clock.Now().Format("150405"), SessionNo: "MND" + clock.Now().Format("150405"),
		ProtocolID: "R001", AdapterModel: "V001", State: state,
		DeviceModel: "JMS-01", ChargerModel: "JMS-02",
		VendorID: "S", LabID: "R", OwnerID: "test",
		RegisteredAt: clock.Now(), Version: 1,
		ShardID: domain.ShardIDFor("2026-08-19", "R001"), DataVersion: 1,
	}
	require.NoError(t, store.ChargeSessionRepo().Save(ctx, session))
	return session
}

func TestMitigationConflictResolution(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	dispSvc := NewMitigationService(store, clock, bus)
	session := setupTestSessionState(t, ctx, store, clock, domain.SessionStateNegotiating)
	disp1, err := dispSvc.Submit(ctx, SubmitSafetyMitigation{
		SessionID: session.ID, Type: domain.MitigationTypeThermalThrottle, SubmittedBy: "product-op",
	})
	require.NoError(t, err)
	assert.Equal(t, domain.MitigationStatePending, disp1.State)
	disp2, err := dispSvc.Submit(ctx, SubmitSafetyMitigation{
		SessionID: session.ID, Type: domain.MitigationTypeStopCharge, SubmittedBy: "another-op",
	})
	require.NoError(t, err)
	assert.Equal(t, domain.MitigationStateLost, disp2.State)
	assert.NotEmpty(t, disp2.ConflictReason)
}

func TestConcurrentMitigationRace(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	dispSvc := NewMitigationService(store, clock, bus)
	session := setupTestSessionState(t, ctx, store, clock, domain.SessionStateNegotiating)
	var wg sync.WaitGroup
	var pending, lost int64
	const goroutines = 10
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			disp, err := dispSvc.Submit(ctx, SubmitSafetyMitigation{
				SessionID: session.ID, Type: domain.MitigationTypeThermalThrottle,
				SubmittedBy: "concurrent-op",
				RequestNo:   "REQ-" + itoa(idx),
			})
			if err != nil {
				return
			}
			if disp.State == domain.MitigationStatePending {
				atomic.AddInt64(&pending, 1)
			} else if disp.State == domain.MitigationStateLost {
				atomic.AddInt64(&lost, 1)
			}
		}(i)
	}
	wg.Wait()
	assert.Equal(t, int64(1), pending, "exactly one mitigation should be pending")
	assert.Equal(t, int64(goroutines-1), lost, "all others should be lost")
}

func TestAdjudicationWorkflow(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	dispSvc := NewMitigationService(store, clock, bus)
	session := setupTestSessionState(t, ctx, store, clock, domain.SessionStateNegotiating)
	disp, err := dispSvc.Submit(ctx, SubmitSafetyMitigation{
		SessionID: session.ID, Type: domain.MitigationTypeProtocolFallback,
		TargetAddress: "new address", SubmittedBy: "product-op",
	})
	require.NoError(t, err)
	assert.Equal(t, domain.MitigationStatePending, disp.State)
	disp, err = dispSvc.Review(ctx, ReviewRequest{
		MitigationID: disp.ID, Reviewer: "adjudicator", Decision: "approve", Note: "approved",
	})
	require.NoError(t, err)
	assert.Equal(t, domain.MitigationStateIssued, disp.State)
	disp, err = dispSvc.Execute(ctx, disp.ID, "system")
	require.NoError(t, err)
	assert.Equal(t, domain.MitigationStateCompleted, disp.State)
	updated, _ := store.ChargeSessionRepo().Get(ctx, session.ID)
	assert.Equal(t, domain.SessionStateProtocolFallback, updated.State)
	assert.Equal(t, "new address", updated.FirmwareVersion)
}

func TestWithdrawAndRetry(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	dispSvc := NewMitigationService(store, clock, bus)
	session := setupTestSessionState(t, ctx, store, clock, domain.SessionStateNegotiating)
	disp, err := dispSvc.Submit(ctx, SubmitSafetyMitigation{
		SessionID: session.ID, Type: domain.MitigationTypeThermalThrottle, SubmittedBy: "product-op",
	})
	require.NoError(t, err)
	disp, err = dispSvc.Review(ctx, ReviewRequest{
		MitigationID: disp.ID, Reviewer: "adjudicator", Decision: "approve", Note: "ok",
	})
	require.NoError(t, err)
	disp, err = dispSvc.Withdraw(ctx, WithdrawRequest{
		MitigationID: disp.ID, WithdrawnBy: "adjudicator", Reason: "wrong request",
	})
	require.NoError(t, err)
	assert.Equal(t, domain.MitigationStateWithdrawn, disp.State)
	active, err := dispSvc.GetActiveBySession(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, len(active), "no active mitigations after withdrawal")
}

func TestRejectMitigationInvalidDecision(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	dispSvc := NewMitigationService(store, clock, bus)
	session := setupTestSessionState(t, ctx, store, clock, domain.SessionStateNegotiating)
	disp, err := dispSvc.Submit(ctx, SubmitSafetyMitigation{
		SessionID: session.ID, Type: domain.MitigationTypeThermalThrottle, SubmittedBy: "op",
	})
	require.NoError(t, err)
	_, err = dispSvc.Review(ctx, ReviewRequest{
		MitigationID: disp.ID, Reviewer: "adjudicator", Decision: "invalid",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrValidation))
}
