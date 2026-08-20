package service

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"voltforge/internal/domain"
)

func TestConcurrentSessionUpdateRace(t *testing.T) {
	store, clock, _, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	session := setupTestSessionState(t, ctx, store, clock, domain.SessionStateNegotiating)
	current, err := store.ChargeSessionRepo().Get(ctx, session.ID)
	require.NoError(t, err)
	expectedState := current.State
	expectedVersion := current.Version
	var wg sync.WaitGroup
	var success, conflict int64
	const goroutines = 20
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, err := store.BeginTx(ctx)
			if err != nil {
				atomic.AddInt64(&conflict, 1)
				return
			}
			defer tx.Rollback()
			err = store.ChargeSessionRepo().UpdateStateTx(ctx, tx, session.ID,
				expectedState, domain.SessionStateCharging, expectedVersion)
			if err != nil {
				atomic.AddInt64(&conflict, 1)
				return
			}
			if err := tx.Commit(); err != nil {
				atomic.AddInt64(&conflict, 1)
				return
			}
			atomic.AddInt64(&success, 1)
		}()
	}
	wg.Wait()
	assert.Equal(t, int64(1), success, "exactly one update should succeed")
	assert.Equal(t, int64(goroutines-1), conflict, "all others should conflict")
}

func TestParallelHandshakeAttestationRace(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	handSvc := NewHandshakeService(store, clock, bus)
	form := registerTestHandshake(t, ctx, handSvc, "F-CONC", "2026-08-19", "R001")
	form, _ = handSvc.AttestationWithLock(ctx, domain.HandshakeAttestationRequest{
		FormID: form.ID, Party: domain.AttestationPartyOutbound, Signer: "op1", Product: "JMS-01",
	})
	var wg sync.WaitGroup
	var success, fail int64
	const goroutines = 10
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := handSvc.AttestationWithLock(ctx, domain.HandshakeAttestationRequest{
				FormID: form.ID, Party: domain.AttestationPartyArrival, Signer: "arrival-op", Product: "JMS-02",
			})
			if err != nil {
				atomic.AddInt64(&fail, 1)
			} else {
				atomic.AddInt64(&success, 1)
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, int64(1), success, "exactly one arrival attestation should succeed")
	assert.True(t, fail > 0, "others should fail")
}
