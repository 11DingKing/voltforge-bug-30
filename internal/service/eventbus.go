package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"voltforge/internal/domain"
	"voltforge/internal/storage"
)

const subscriberBufferSize = 256

type SubscriberChannel struct {
	ID   string
	Ch   chan *domain.Event
	Done chan struct{}
}

type EventBus struct {
	mu          sync.RWMutex
	subscribers map[string]*SubscriberChannel
	eventRepo   storage.EventRepository
}

func NewEventBus(eventRepo storage.EventRepository) *EventBus {
	return &EventBus{
		subscribers: make(map[string]*SubscriberChannel),
		eventRepo:   eventRepo,
	}
}

func (bus *EventBus) Register(id string) *SubscriberChannel {
	bus.mu.Lock()
	defer bus.mu.Unlock()
	if existing, ok := bus.subscribers[id]; ok {
		close(existing.Done)
		delete(bus.subscribers, id)
	}
	sc := &SubscriberChannel{
		ID:   id,
		Ch:   make(chan *domain.Event, subscriberBufferSize),
		Done: make(chan struct{}),
	}
	bus.subscribers[id] = sc
	return sc
}

func (bus *EventBus) Unregister(id string) {
	bus.mu.Lock()
	defer bus.mu.Unlock()
	if sc, ok := bus.subscribers[id]; ok {
		delete(bus.subscribers, id)
		close(sc.Done)
	}
}

func (bus *EventBus) Publish(ctx context.Context, event *domain.Event) error {
	id, err := bus.eventRepo.Append(ctx, event)
	if err != nil {
		return fmt.Errorf("persist event: %w", err)
	}
	event.ID = id
	bus.mu.RLock()
	var toEvict []string
	for subID, sc := range bus.subscribers {
		select {
		case sc.Ch <- event:
		default:
			toEvict = append(toEvict, subID)
		}
	}
	bus.mu.RUnlock()
	if len(toEvict) > 0 {
		bus.mu.Lock()
		for _, subID := range toEvict {
			if sc, ok := bus.subscribers[subID]; ok {
				delete(bus.subscribers, subID)
				close(sc.Done)
			}
		}
		bus.mu.Unlock()
	}
	return nil
}

func (bus *EventBus) SubscriberCount() int {
	bus.mu.RLock()
	defer bus.mu.RUnlock()
	return len(bus.subscribers)
}

func (bus *EventBus) ReplayAfter(ctx context.Context, afterID int64, limit int) ([]*domain.Event, error) {
	return bus.eventRepo.ListAfter(ctx, afterID, limit)
}

func (bus *EventBus) GetLastEventID(ctx context.Context) (int64, error) {
	return bus.eventRepo.GetLastID(ctx)
}

func sharedRecordTelemetry(ctx context.Context, store storage.Store, clock domain.Clock, shardID, formNo, sessionNo, owner_id, entryType, prevState, nextState string) {
	date, protocolID := domain.SplitShardID(shardID)
	entry := &domain.TelemetryEntry{
		ID: uuid.NewString(), Date: date, ProtocolID: protocolID, VolumeNo: date + "_" + protocolID,
		FormNo: formNo, SessionNo: sessionNo, OwnerID: owner_id, EntryType: entryType,
		Description: entryType + ": " + prevState + " -> " + nextState,
		PrevState:   prevState, NextState: nextState, CreatedAt: clock.Now(), ShardID: shardID, DataVersion: 1,
	}
	store.TelemetryRepo().Save(ctx, entry)
}

func sharedAppendAudit(ctx context.Context, store storage.Store, clock domain.Clock, actor, action, entityType, entityID, shardID, beforeState, afterState, detail string) {
	rec := &domain.AuditRecord{
		Actor: actor, Action: action, EntityType: entityType, EntityID: entityID,
		ShardID: shardID, BeforeState: beforeState, AfterState: afterState,
		Detail: detail, Timestamp: clock.Now(),
	}
	store.AuditRepo().Append(ctx, rec)
}

func sharedPublishEvent(ctx context.Context, bus *EventBus, clock domain.Clock, eventType, businessKey, shardID string, payload any) {
	payloadBytes, _ := json.Marshal(payload)
	event := &domain.Event{
		Type: eventType, BusinessKey: businessKey, ShardID: shardID,
		Payload: string(payloadBytes), CreatedAt: clock.Now(),
	}
	bus.Publish(ctx, event)
}
