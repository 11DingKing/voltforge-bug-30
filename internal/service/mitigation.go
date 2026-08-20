package service

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"voltforge/internal/circuit"
	"voltforge/internal/domain"
	"voltforge/internal/storage"
)

type MitigationService struct {
	store    storage.Store
	clock    domain.Clock
	eventBus *EventBus
	breaker  *circuit.Breaker
}

func NewMitigationService(store storage.Store, clock domain.Clock, bus *EventBus) *MitigationService {
	return &MitigationService{store: store, clock: clock, eventBus: bus}
}

func (s *MitigationService) WithBreaker(b *circuit.Breaker) *MitigationService {
	s.breaker = b
	return s
}

type SubmitSafetyMitigation struct {
	RequestNo     string `json:"request_no"`
	SessionID     string `json:"session_id"`
	Type          string `json:"type"`
	TargetAddress string `json:"target_address,omitempty"`
	SubmittedBy   string `json:"submitted_by"`
}

func (s *MitigationService) Submit(ctx context.Context, req SubmitSafetyMitigation) (*domain.SafetyMitigation, error) {
	if req.SessionID == "" || req.Type == "" || req.SubmittedBy == "" {
		return nil, domain.ValidationError{Field: "session_id/type/submitted_by", Message: "required fields missing"}
	}
	session, err := s.store.ChargeSessionRepo().Get(ctx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	if req.RequestNo == "" {
		req.RequestNo = uuid.NewString()
	}
	shardID := session.ShardID
	now := s.clock.Now()
	disp := &domain.SafetyMitigation{
		ID: uuid.NewString(), RequestNo: req.RequestNo, SessionID: req.SessionID, SessionNo: session.SessionNo,
		Type: req.Type, TargetAddress: req.TargetAddress, State: domain.MitigationStatePending,
		SubmittedBy: req.SubmittedBy, SubmittedAt: now, Version: 1, ShardID: shardID, DataVersion: 1,
	}
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	activeCount, err := s.store.MitigationRepo().CountActiveBySessionTx(ctx, tx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("count active: %w", err)
	}
	if activeCount > 0 {
		disp.State = domain.MitigationStateLost
		disp.ConflictReason = fmt.Sprintf("another active mitigation already exists for session %s", session.SessionNo)
		disp.LostAt = now
		if err := s.store.MitigationRepo().SaveTx(ctx, tx, disp); err != nil {
			return nil, fmt.Errorf("save lost mitigation: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit: %w", err)
		}
		sharedRecordTelemetry(ctx, s.store, s.clock, shardID, "", disp.SessionNo, req.SubmittedBy,
			domain.TelemetryEntryTypeMitigation, domain.MitigationStatePending, disp.State)
		sharedAppendAudit(ctx, s.store, s.clock, req.SubmittedBy, domain.AdjudicationActionLost, domain.EntityTypeMitigation, disp.ID, shardID, "", disp.State, disp.ConflictReason)
		sharedPublishEvent(ctx, s.eventBus, s.clock, domain.EventMitigationLost, disp.ID, shardID, disp)
		return disp, nil
	}
	if err := s.store.MitigationRepo().SaveTx(ctx, tx, disp); err != nil {
		return nil, fmt.Errorf("save mitigation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	sharedRecordTelemetry(ctx, s.store, s.clock, shardID, "", disp.SessionNo, req.SubmittedBy,
		domain.TelemetryEntryTypeMitigation, "", disp.State)
	sharedAppendAudit(ctx, s.store, s.clock, req.SubmittedBy, domain.AdjudicationActionSubmit, domain.EntityTypeMitigation, disp.ID, shardID, "", disp.State, "submitted")
	sharedPublishEvent(ctx, s.eventBus, s.clock, domain.EventMitigationSubmitted, disp.ID, shardID, disp)
	return disp, nil
}

type ReviewRequest struct {
	MitigationID string `json:"mitigation_id"`
	Reviewer     string `json:"reviewer"`
	Decision     string `json:"decision"`
	Note         string `json:"note"`
}

func (s *MitigationService) Review(ctx context.Context, req ReviewRequest) (*domain.SafetyMitigation, error) {
	disp, err := s.store.MitigationRepo().Get(ctx, req.MitigationID)
	if err != nil {
		return nil, fmt.Errorf("get mitigation: %w", err)
	}
	var targetState string
	switch req.Decision {
	case "approve":
		targetState = domain.MitigationStateIssued
	case "reject":
		targetState = domain.MitigationStateRejected
	default:
		return nil, domain.ValidationError{Field: "decision", Message: "must be approve or reject"}
	}
	if err := domain.ValidateMitigationTransition(disp.State, targetState); err != nil {
		return nil, fmt.Errorf("validate transition: %w", err)
	}
	prevState := disp.State
	disp.State = targetState
	disp.ReviewedBy = req.Reviewer
	disp.ReviewedAt = s.clock.Now()
	disp.ReviewNote = req.Note
	disp.Version++
	if targetState == domain.MitigationStateIssued {
		disp.IssuedBy = req.Reviewer
		disp.IssuedAt = s.clock.Now()
	}
	if err := s.store.MitigationRepo().Save(ctx, disp); err != nil {
		return nil, fmt.Errorf("save mitigation: %w", err)
	}
	sharedRecordTelemetry(ctx, s.store, s.clock, disp.ShardID, "", disp.SessionNo, req.Reviewer,
		domain.TelemetryEntryTypeMitigation, prevState, disp.State)
	sharedAppendAudit(ctx, s.store, s.clock, req.Reviewer, domain.AdjudicationActionReview, domain.EntityTypeMitigation, disp.ID, disp.ShardID, prevState, disp.State, req.Note)
	sharedPublishEvent(ctx, s.eventBus, s.clock, domain.EventMitigationReviewed, disp.ID, disp.ShardID, disp)
	return disp, nil
}

type WithdrawRequest struct {
	MitigationID string `json:"mitigation_id"`
	WithdrawnBy  string `json:"withdrawn_by"`
	Reason       string `json:"reason"`
}

func (s *MitigationService) Withdraw(ctx context.Context, req WithdrawRequest) (*domain.SafetyMitigation, error) {
	disp, err := s.store.MitigationRepo().Get(ctx, req.MitigationID)
	if err != nil {
		return nil, fmt.Errorf("get mitigation: %w", err)
	}
	if err := domain.ValidateMitigationTransition(disp.State, domain.MitigationStateWithdrawn); err != nil {
		return nil, fmt.Errorf("validate transition: %w", err)
	}
	prevState := disp.State
	disp.State = domain.MitigationStateWithdrawn
	disp.WithdrawnBy = req.WithdrawnBy
	disp.WithdrawnAt = s.clock.Now()
	disp.WithdrawnReason = req.Reason
	disp.Version++
	if err := s.store.MitigationRepo().Save(ctx, disp); err != nil {
		return nil, fmt.Errorf("save mitigation: %w", err)
	}
	sharedRecordTelemetry(ctx, s.store, s.clock, disp.ShardID, "", disp.SessionNo, req.WithdrawnBy,
		domain.TelemetryEntryTypeWithdrawal, prevState, disp.State)
	sharedAppendAudit(ctx, s.store, s.clock, req.WithdrawnBy, domain.AdjudicationActionWithdraw, domain.EntityTypeMitigation, disp.ID, disp.ShardID, prevState, disp.State, req.Reason)
	sharedPublishEvent(ctx, s.eventBus, s.clock, domain.EventMitigationWithdrawn, disp.ID, disp.ShardID, disp)
	return disp, nil
}

func (s *MitigationService) Execute(ctx context.Context, dispID, actor string) (*domain.SafetyMitigation, error) {
	if s.breaker != nil {
		var result *domain.SafetyMitigation
		err := s.breaker.Execute(func() error {
			r, e := s.executeInternal(ctx, dispID, actor)
			result = r
			return e
		})
		return result, err
	}
	return s.executeInternal(ctx, dispID, actor)
}

func (s *MitigationService) executeInternal(ctx context.Context, dispID, actor string) (*domain.SafetyMitigation, error) {
	disp, err := s.store.MitigationRepo().Get(ctx, dispID)
	if err != nil {
		return nil, fmt.Errorf("get mitigation: %w", err)
	}
	if err := domain.ValidateMitigationTransition(disp.State, domain.MitigationStateExecuting); err != nil {
		return nil, fmt.Errorf("validate transition: %w", err)
	}
	prevState := disp.State
	disp.State = domain.MitigationStateExecuting
	disp.ExecutedAt = s.clock.Now()
	disp.Version++
	if err := s.store.MitigationRepo().Save(ctx, disp); err != nil {
		return nil, fmt.Errorf("save mitigation: %w", err)
	}
	session, err := s.store.ChargeSessionRepo().Get(ctx, disp.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	sessionPrevState := session.State
	switch disp.Type {
	case domain.MitigationTypeThermalThrottle:
		session.State = domain.SessionStateThermalThrottled
	case domain.MitigationTypeProtocolFallback:
		session.State = domain.SessionStateProtocolFallback
		if disp.TargetAddress != "" {
			session.FirmwareVersion = disp.TargetAddress
		}
	case domain.MitigationTypeStopCharge:
		session.State = domain.SessionStateRejected
	}
	session.MitigationID = disp.ID
	session.Version++
	if err := s.store.ChargeSessionRepo().Save(ctx, session); err != nil {
		return nil, fmt.Errorf("save session: %w", err)
	}
	disp.State = domain.MitigationStateCompleted
	disp.CompletedAt = s.clock.Now()
	disp.Version++
	if err := s.store.MitigationRepo().Save(ctx, disp); err != nil {
		return nil, fmt.Errorf("complete mitigation: %w", err)
	}
	sharedRecordTelemetry(ctx, s.store, s.clock, disp.ShardID, "", disp.SessionNo, actor,
		domain.TelemetryEntryTypeMitigation, prevState, disp.State)
	sharedAppendAudit(ctx, s.store, s.clock, actor, domain.AdjudicationActionExecute, domain.EntityTypeMitigation, disp.ID, disp.ShardID, prevState, disp.State, "")
	sharedAppendAudit(ctx, s.store, s.clock, actor, domain.AdjudicationActionComplete, domain.EntityTypeMitigation, disp.ID, disp.ShardID, sessionPrevState, session.State, "")
	sharedPublishEvent(ctx, s.eventBus, s.clock, domain.EventMitigationCompleted, disp.ID, disp.ShardID, disp)
	return disp, nil
}

func (s *MitigationService) Get(ctx context.Context, id string) (*domain.SafetyMitigation, error) {
	return s.store.MitigationRepo().Get(ctx, id)
}

func (s *MitigationService) List(ctx context.Context, filter storage.MitigationFilter) ([]*domain.SafetyMitigation, int, error) {
	return s.store.MitigationRepo().List(ctx, filter)
}

func (s *MitigationService) GetActiveBySession(ctx context.Context, sessionID string) ([]*domain.SafetyMitigation, error) {
	return s.store.MitigationRepo().GetActiveBySession(ctx, sessionID)
}
