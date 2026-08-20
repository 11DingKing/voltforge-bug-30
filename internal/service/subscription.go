package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"net/http"
	"voltforge/internal/domain"
	"voltforge/internal/ratelimit"
	"voltforge/internal/storage"
)

type SubscriptionService struct {
	store    storage.Store
	clock    domain.Clock
	eventBus *EventBus
	limiter  *ratelimit.SubscriberLimiter
}

func NewSubscriptionService(store storage.Store, clock domain.Clock, bus *EventBus) *SubscriptionService {
	return &SubscriptionService{store: store, clock: clock, eventBus: bus}
}

func (s *SubscriptionService) WithLimiter(l *ratelimit.SubscriberLimiter) *SubscriptionService {
	s.limiter = l
	return s
}

func (s *SubscriptionService) EnsureSubscriber(ctx context.Context, req domain.SubscriptionRequest) (*domain.Subscriber, error) {
	sub, err := s.store.SubscriberRepo().Get(ctx, req.SubscriberID)
	if err == nil {
		return sub, nil
	}
	now := s.clock.Now()
	sub = &domain.Subscriber{
		ID: req.SubscriberID, Type: req.SubscriberType, Name: req.Name,
		LastEventID: req.LastEventID, LastActiveAt: now, CreatedAt: now,
	}
	if sub.ID == "" {
		sub.ID = uuid.NewString()
	}
	if err := s.store.SubscriberRepo().Save(ctx, sub); err != nil {
		return nil, fmt.Errorf("save subscriber: %w", err)
	}
	return sub, nil
}

func (s *SubscriptionService) HandleSSE(w http.ResponseWriter, r *http.Request, subscriberID string) {
	if s.limiter != nil {
		if err := s.limiter.Acquire(subscriberID); err != nil {
			status := http.StatusTooManyRequests
			if err == ratelimit.ErrTooManyConnections {
				status = http.StatusServiceUnavailable
			}
			http.Error(w, err.Error(), status)
			return
		}
		defer s.limiter.Release(subscriberID)
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	ctx := r.Context()
	sub, err := s.store.SubscriberRepo().Get(ctx, subscriberID)
	if err != nil {
		http.Error(w, "subscriber not found", http.StatusNotFound)
		return
	}
	sc := s.eventBus.Register(subscriberID)
	defer s.eventBus.Unregister(subscriberID)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	afterID := sub.LastEventID
	if lastEventID := r.Header.Get("Last-Event-ID"); lastEventID != "" {
		var parsed int64
		if _, err := fmt.Sscanf(lastEventID, "%d", &parsed); err == nil && parsed > afterID {
			afterID = parsed
		}
	}
	if err := s.replayEvents(ctx, w, flusher, afterID); err != nil {
		return
	}
	lastSentID := afterID
	for {
		select {
		case <-ctx.Done():
			s.store.SubscriberRepo().UpdateCheckpoint(ctx, subscriberID, lastSentID)
			return
		case <-sc.Done:
			s.writeSSEEvent(w, flusher, &domain.Event{
				Type: "subscriber_evicted", BusinessKey: subscriberID,
				Payload: `{"reason":"slow consumer"}`, CreatedAt: s.clock.Now(),
			}, 0)
			return
		case event := <-sc.Ch:
			if event.ID > lastSentID {
				s.writeSSEEvent(w, flusher, event, event.ID)
				lastSentID = event.ID
			}
		}
	}
}

func (s *SubscriptionService) replayEvents(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, afterID int64) error {
	events, err := s.eventBus.ReplayAfter(ctx, afterID, 500)
	if err != nil {
		return err
	}
	for _, event := range events {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		s.writeSSEEvent(w, flusher, event, event.ID)
	}
	return nil
}

func (s *SubscriptionService) writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, event *domain.Event, id int64) {
	data, _ := json.Marshal(event)
	if id > 0 {
		fmt.Fprintf(w, "id: %d\n", id)
	}
	fmt.Fprintf(w, "event: %s\n", event.Type)
	fmt.Fprintf(w, "data: %s\n\n", data)
	if flusher != nil {
		flusher.Flush()
	}
}

func (s *SubscriptionService) GetLastEventID(ctx context.Context) (int64, error) {
	return s.eventBus.GetLastEventID(ctx)
}

func (s *SubscriptionService) PublishEvent(ctx context.Context, eventType, businessKey, shardID string, payload any) error {
	payloadBytes, _ := json.Marshal(payload)
	event := &domain.Event{
		Type: eventType, BusinessKey: businessKey, ShardID: shardID,
		Payload: string(payloadBytes), CreatedAt: s.clock.Now(),
	}
	return s.eventBus.Publish(ctx, event)
}
