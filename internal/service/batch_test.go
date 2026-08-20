package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"voltforge/internal/domain"
)

func TestBatchAllSucceedCommit(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	batchSvc := NewBatchService(store, clock, bus)
	var sessionIDs []string
	for i := 0; i < 3; i++ {
		session := setupTestSessionState(t, ctx, store, clock, domain.SessionStateNegotiating)
		sessionIDs = append(sessionIDs, session.ID)
		clock.Advance(1e9)
	}
	batch, err := batchSvc.CreateBatch(ctx, CreateBatchRequest{
		AdapterModel: "V001", Date: "2026-08-19", ProtocolID: "R001",
		SessionIDs: sessionIDs,
	})
	require.NoError(t, err)
	assert.Equal(t, domain.BatchStatePending, batch.State)
	batch, err = batchSvc.ProcessBatch(ctx, batch.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BatchStateCompleted, batch.State)
	assert.Equal(t, 3, batch.SucceededCount)
}

func TestBatchRollbackCompensation(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	batchSvc := NewBatchService(store, clock, bus)
	session1 := setupTestSessionState(t, ctx, store, clock, domain.SessionStateNegotiating)
	clock.Advance(1e9)
	session2 := setupTestSessionState(t, ctx, store, clock, domain.SessionStateRequested)
	clock.Advance(1e9)
	session3 := setupTestSessionState(t, ctx, store, clock, domain.SessionStateNegotiating)
	clock.Advance(1e9)
	batch, err := batchSvc.CreateBatch(ctx, CreateBatchRequest{
		AdapterModel: "V001", Date: "2026-08-19", ProtocolID: "R001",
		SessionIDs: []string{session1.ID, session2.ID, session3.ID},
	})
	require.NoError(t, err)
	batch, err = batchSvc.ProcessBatch(ctx, batch.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BatchStateRolledBack, batch.State)
	assert.True(t, batch.FailedCount > 0)
	compensated1, _ := store.ChargeSessionRepo().Get(ctx, session1.ID)
	assert.Equal(t, domain.SessionStateNegotiating, compensated1.State, "compensated back to negotiating")
	compensated3, _ := store.ChargeSessionRepo().Get(ctx, session3.ID)
	assert.Equal(t, domain.SessionStateNegotiating, compensated3.State, "compensated back to negotiating")
}

func TestBatchRetryIdempotentNotReapplied(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	batchSvc := NewBatchService(store, clock, bus)
	session1 := setupTestSessionState(t, ctx, store, clock, domain.SessionStateNegotiating)
	clock.Advance(1e9)
	session2 := setupTestSessionState(t, ctx, store, clock, domain.SessionStateRequested)
	clock.Advance(1e9)
	batch, err := batchSvc.CreateBatch(ctx, CreateBatchRequest{
		AdapterModel: "V001", Date: "2026-08-19", ProtocolID: "R001",
		SessionIDs: []string{session1.ID, session2.ID},
	})
	require.NoError(t, err)
	batch, err = batchSvc.ProcessBatch(ctx, batch.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BatchStateRolledBack, batch.State)
	session2.State = domain.SessionStateNegotiating
	session2.Version++
	require.NoError(t, store.ChargeSessionRepo().Save(ctx, session2))
	batch, err = batchSvc.ProcessBatch(ctx, batch.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BatchStateCompleted, batch.State)
}

func TestBatchCompensationIdempotentDuplicate(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	batchSvc := NewBatchService(store, clock, bus)
	session1 := setupTestSessionState(t, ctx, store, clock, domain.SessionStateNegotiating)
	clock.Advance(1e9)
	session2 := setupTestSessionState(t, ctx, store, clock, domain.SessionStateRequested)
	clock.Advance(1e9)
	batch, err := batchSvc.CreateBatch(ctx, CreateBatchRequest{
		AdapterModel: "V001", Date: "2026-08-19", ProtocolID: "R001",
		SessionIDs: []string{session1.ID, session2.ID},
	})
	require.NoError(t, err)
	batch, err = batchSvc.ProcessBatch(ctx, batch.ID)
	require.NoError(t, err)
	require.Equal(t, domain.BatchStateRolledBack, batch.State)
	items, _ := batchSvc.ListItems(ctx, batch.ID)
	var rolledBackItem *domain.BatchItem
	for _, item := range items {
		if item.State == domain.BatchItemStateRolledBack {
			rolledBackItem = item
			break
		}
	}
	require.NotNil(t, rolledBackItem)
	batchSvc.CompensateItem(ctx, rolledBackItem)
	sessionAfter1, _ := store.ChargeSessionRepo().Get(ctx, session1.ID)
	assert.Equal(t, domain.SessionStateNegotiating, sessionAfter1.State)
	batchSvc.CompensateItem(ctx, rolledBackItem)
	sessionAfter2, _ := store.ChargeSessionRepo().Get(ctx, session1.ID)
	assert.Equal(t, domain.SessionStateNegotiating, sessionAfter2.State, "second compensation is idempotent")
}
