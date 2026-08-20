package service

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"voltforge/internal/domain"
	"voltforge/internal/storage"
)

type BatchService struct {
	store    storage.Store
	clock    domain.Clock
	eventBus *EventBus
}

func NewBatchService(store storage.Store, clock domain.Clock, bus *EventBus) *BatchService {
	return &BatchService{store: store, clock: clock, eventBus: bus}
}

type CreateBatchRequest struct {
	AdapterModel string   `json:"adapter_model"`
	Date         string   `json:"date"`
	ProtocolID   string   `json:"protocol_id"`
	SessionIDs   []string `json:"session_ids"`
	Mitigations  []struct {
		SessionID     string `json:"session_id"`
		Type          string `json:"type"`
		TargetAddress string `json:"target_address"`
	} `json:"mitigations"`
}

func (s *BatchService) CreateBatch(ctx context.Context, req CreateBatchRequest) (*domain.BatchRecord, error) {
	if req.AdapterModel == "" || req.Date == "" || req.ProtocolID == "" {
		return nil, domain.ValidationError{Field: "adapter_model/date/protocol_id", Message: "required"}
	}
	shardID := domain.ShardIDFor(req.Date, req.ProtocolID)
	now := s.clock.Now()
	itemCount := len(req.Mitigations)
	if itemCount == 0 {
		itemCount = len(req.SessionIDs)
	}
	batch := &domain.BatchRecord{
		ID: uuid.NewString(), AdapterModel: req.AdapterModel, Date: req.Date, ProtocolID: req.ProtocolID,
		State: domain.BatchStatePending, TotalCount: itemCount,
		CreatedAt: now, UpdatedAt: now, Version: 1, ShardID: shardID, DataVersion: 1,
	}
	if err := s.store.BatchRepo().Save(ctx, batch); err != nil {
		return nil, fmt.Errorf("save batch: %w", err)
	}
	sessionIDs := req.SessionIDs
	if len(req.Mitigations) > 0 {
		sessionIDs = make([]string, 0, len(req.Mitigations))
		for _, d := range req.Mitigations {
			sessionIDs = append(sessionIDs, d.SessionID)
		}
	}
	for _, sessionID := range sessionIDs {
		session, err := s.store.ChargeSessionRepo().Get(ctx, sessionID)
		if err != nil {
			return nil, fmt.Errorf("get session %s: %w", sessionID, err)
		}
		item := &domain.BatchItem{
			ID: uuid.NewString(), BatchID: batch.ID, SessionID: sessionID, SessionNo: session.SessionNo,
			State: domain.BatchItemStatePending, CreatedAt: now, UpdatedAt: now,
		}
		if err := s.store.BatchRepo().SaveItem(ctx, item); err != nil {
			return nil, fmt.Errorf("save batch item: %w", err)
		}
	}
	return batch, nil
}

func (s *BatchService) ProcessBatch(ctx context.Context, batchID string) (*domain.BatchRecord, error) {
	batch, err := s.store.BatchRepo().Get(ctx, batchID)
	if err != nil {
		return nil, fmt.Errorf("get batch: %w", err)
	}
	if err := domain.ValidateBatchTransition(batch.State, domain.BatchStateProcessing); err != nil {
		return nil, fmt.Errorf("validate batch transition: %w", err)
	}
	batch.State = domain.BatchStateProcessing
	batch.UpdatedAt = s.clock.Now()
	batch.Version++
	if err := s.store.BatchRepo().Save(ctx, batch); err != nil {
		return nil, fmt.Errorf("save batch processing: %w", err)
	}
	items, err := s.store.BatchRepo().ListItems(ctx, batchID)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	processedThisAttempt := make(map[string]bool)
	allSucceeded := true
	for _, item := range items {
		if item.State == domain.BatchItemStateSucceeded {
			continue
		}
		if err := s.processItem(ctx, item); err != nil {
			item.State = domain.BatchItemStateFailed
			item.Error = err.Error()
			item.UpdatedAt = s.clock.Now()
			s.store.BatchRepo().SaveItem(ctx, item)
			allSucceeded = false
			continue
		}
		item.State = domain.BatchItemStateSucceeded
		item.UpdatedAt = s.clock.Now()
		s.store.BatchRepo().SaveItem(ctx, item)
		processedThisAttempt[item.ID] = true
	}
	if allSucceeded {
		batch.State = domain.BatchStateSucceeded
		batch.SucceededCount = len(items)
		batch.UpdatedAt = s.clock.Now()
		batch.Version++
		s.store.BatchRepo().Save(ctx, batch)
		batch.State = domain.BatchStateCompleted
		batch.Version++
		s.store.BatchRepo().Save(ctx, batch)
		sharedPublishEvent(ctx, s.eventBus, s.clock, domain.EventBatchProcessed, batch.ID, batch.ShardID, batch)
		return batch, nil
	}
	for _, item := range items {
		if processedThisAttempt[item.ID] {
			s.compensateItem(ctx, item)
			dbItem, _ := s.store.BatchRepo().GetItem(ctx, item.ID)
			dbItem.State = domain.BatchItemStateRolledBack
			dbItem.UpdatedAt = s.clock.Now()
			s.store.BatchRepo().SaveItem(ctx, dbItem)
		}
	}
	succeeded := 0
	failed := 0
	for _, item := range items {
		switch item.State {
		case domain.BatchItemStateSucceeded:
			succeeded++
		default:
			failed++
		}
	}
	batch.State = domain.BatchStateRolledBack
	batch.SucceededCount = succeeded
	batch.FailedCount = failed
	batch.UpdatedAt = s.clock.Now()
	batch.Version++
	s.store.BatchRepo().Save(ctx, batch)
	sharedPublishEvent(ctx, s.eventBus, s.clock, domain.EventBatchRolledBack, batch.ID, batch.ShardID, batch)
	return batch, nil
}

func (s *BatchService) processItem(ctx context.Context, item *domain.BatchItem) error {
	session, err := s.store.ChargeSessionRepo().Get(ctx, item.SessionID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}
	if session.State != domain.SessionStateNegotiating && session.State != domain.SessionStateCharging {
		return fmt.Errorf("session %s in state %s, cannot process", session.SessionNo, session.State)
	}
	session.State = domain.SessionStateCharging
	session.Version++
	return s.store.ChargeSessionRepo().Save(ctx, session)
}

func (s *BatchService) compensateItem(ctx context.Context, item *domain.BatchItem) {
	session, err := s.store.ChargeSessionRepo().Get(ctx, item.SessionID)
	if err != nil {
		return
	}
	if session.State == domain.SessionStateCharging {
		session.State = domain.SessionStateNegotiating
		session.Version++
		s.store.ChargeSessionRepo().Save(ctx, session)
	}
	sharedAppendAudit(ctx, s.store, s.clock, "system", "compensate", domain.EntityTypeBatch, item.BatchID, session.ShardID,
		item.State, domain.BatchItemStateRolledBack, "batch compensation")
}

func (s *BatchService) CompensateItem(ctx context.Context, item *domain.BatchItem) {
	s.compensateItem(ctx, item)
}

func (s *BatchService) GetBatch(ctx context.Context, id string) (*domain.BatchRecord, error) {
	return s.store.BatchRepo().Get(ctx, id)
}

func (s *BatchService) ListItems(ctx context.Context, batchID string) ([]*domain.BatchItem, error) {
	return s.store.BatchRepo().ListItems(ctx, batchID)
}

func (s *BatchService) ListPending(ctx context.Context) ([]*domain.BatchRecord, error) {
	return s.store.BatchRepo().ListPending(ctx)
}
