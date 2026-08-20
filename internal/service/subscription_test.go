package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"voltforge/internal/domain"
)

func TestSubscriptionReplayAfterReconnect(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	subSvc := NewSubscriptionService(store, clock, bus)
	for i := 0; i < 5; i++ {
		event := &domain.Event{
			Type: domain.EventSessionLoaded, BusinessKey: "session-" + itoa(i),
			ShardID: "2026-08-19_R001", Payload: "{}", CreatedAt: clock.Now(),
		}
		require.NoError(t, bus.Publish(ctx, event))
		clock.Advance(1e9)
	}
	lastID, err := bus.GetLastEventID(ctx)
	require.NoError(t, err)
	assert.True(t, lastID >= 5)
	sub, err := subSvc.EnsureSubscriber(ctx, domain.SubscriptionRequest{
		SubscriberID: "sub-replay", SubscriberType: domain.SubscriberTypeDispatcher,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), sub.LastEventID)
	events, err := bus.ReplayAfter(ctx, 0, 100)
	require.NoError(t, err)
	assert.Equal(t, 5, len(events))
	for i, e := range events {
		expected := "session-" + itoa(i)
		assert.Equal(t, expected, e.BusinessKey, "event %d should have business key %s", i, expected)
	}
}

func TestSubscriptionSlowConsumerEvicted(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	subSvc := NewSubscriptionService(store, clock, bus)
	_, err := subSvc.EnsureSubscriber(ctx, domain.SubscriptionRequest{
		SubscriberID: "sub-slow", SubscriberType: domain.SubscriberTypeDispatcher,
	})
	require.NoError(t, err)
	sc := bus.Register("sub-slow")
	require.NotNil(t, sc)
	require.False(t, bus.SubscriberCount() == 0)
	for i := 0; i < subscriberBufferSize+10; i++ {
		event := &domain.Event{
			Type: domain.EventSessionLoaded, BusinessKey: "session-ev-" + itoa(i),
			ShardID: "2026-08-19_R001", Payload: "{}", CreatedAt: clock.Now(),
		}
		if err := bus.Publish(ctx, event); err != nil {
			break
		}
		if bus.SubscriberCount() == 0 {
			break
		}
	}
	time.Sleep(10 * time.Millisecond)
	assert.Equal(t, 0, bus.SubscriberCount(), "slow consumer should be evicted")
}

func TestSubscriptionPersistCheckpoint(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	subSvc := NewSubscriptionService(store, clock, bus)
	sub, err := subSvc.EnsureSubscriber(ctx, domain.SubscriptionRequest{
		SubscriberID: "sub-persist", SubscriberType: domain.SubscriberTypeDriver,
	})
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		event := &domain.Event{
			Type: domain.EventSessionInTransit, BusinessKey: "session-cp-" + itoa(i),
			ShardID: "2026-08-19_R001", Payload: "{}", CreatedAt: clock.Now(),
		}
		require.NoError(t, bus.Publish(ctx, event))
		clock.Advance(1e9)
	}
	require.NoError(t, store.SubscriberRepo().UpdateCheckpoint(ctx, sub.ID, 3))
	recovered, err := store.SubscriberRepo().Get(ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), recovered.LastEventID)
}

func TestSubscriptionOrderGuarantee(t *testing.T) {
	_, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	for i := 0; i < 10; i++ {
		event := &domain.Event{
			Type: domain.EventSessionLoaded, BusinessKey: "session-ord-" + itoa(i),
			ShardID: "2026-08-19_R001", Payload: "{}", CreatedAt: clock.Now(),
		}
		require.NoError(t, bus.Publish(ctx, event))
		clock.Advance(1e9)
	}
	events, err := bus.ReplayAfter(ctx, 0, 100)
	require.NoError(t, err)
	require.Equal(t, 10, len(events))
	for i := 0; i < len(events)-1; i++ {
		assert.True(t, events[i].ID < events[i+1].ID, "events should be in order")
	}
}
