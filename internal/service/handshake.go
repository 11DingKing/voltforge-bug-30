package service

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"sync"
	"voltforge/internal/domain"
	"voltforge/internal/storage"
)

type HandshakeService struct {
	store    storage.Store
	clock    domain.Clock
	eventBus *EventBus
}

func NewHandshakeService(store storage.Store, clock domain.Clock, bus *EventBus) *HandshakeService {
	return &HandshakeService{store: store, clock: clock, eventBus: bus}
}

type RegisterHandshakeRequest struct {
	FormNo          string                  `json:"form_no"`
	Date            string                  `json:"date"`
	ProtocolID      string                  `json:"protocol_id"`
	AdapterModel    string                  `json:"adapter_model"`
	OutboundProduct string                  `json:"outbound_product"`
	ArrivalProduct  string                  `json:"arrival_product"`
	OwnerID         string                  `json:"owner_id"`
	ChargeSessions  []RegisterChargeSession `json:"session_items"`
}

type RegisterChargeSession struct {
	SessionNo       string `json:"session_no"`
	VendorID        string `json:"vendor_id"`
	CableID         string `json:"cable_id"`
	LabID           string `json:"lab_id"`
	FirmwareVersion string `json:"firmware_version"`
}

func (s *HandshakeService) Register(ctx context.Context, req RegisterHandshakeRequest) (*domain.ProtocolHandshake, error) {
	if req.FormNo == "" || req.Date == "" || req.ProtocolID == "" {
		return nil, domain.ValidationError{Field: "form_no/date/protocol_id", Message: "required fields missing"}
	}
	if _, err := s.store.HandshakeRepo().GetByFormNo(ctx, req.FormNo); err == nil {
		return nil, fmt.Errorf("%w: form_no %s", domain.ErrAlreadyExists, req.FormNo)
	}
	shardID := domain.ShardIDFor(req.Date, req.ProtocolID)
	now := s.clock.Now()
	form := &domain.ProtocolHandshake{
		ID: uuid.NewString(), FormNo: req.FormNo, Date: req.Date, ProtocolID: req.ProtocolID,
		AdapterModel: req.AdapterModel, State: domain.HandshakeStateDraft,
		OutboundProduct: req.OutboundProduct, ArrivalProduct: req.ArrivalProduct,
		ChargeSessionCount: len(req.ChargeSessions), OwnerID: req.OwnerID,
		RegisteredAt: now, UpdatedAt: now, Version: 1, ShardID: shardID, DataVersion: 1,
	}
	if err := s.store.HandshakeRepo().Save(ctx, form); err != nil {
		return nil, fmt.Errorf("save handshake: %w", err)
	}
	for _, mi := range req.ChargeSessions {
		session := &domain.ChargeSession{
			ID: uuid.NewString(), SessionNo: mi.SessionNo, ProtocolID: req.ProtocolID, AdapterModel: req.AdapterModel,
			State: domain.SessionStateRequested, HandshakeID: form.ID,
			DeviceModel: req.OutboundProduct, ChargerModel: req.ArrivalProduct,
			VendorID: mi.VendorID, CableID: mi.CableID,
			LabID: mi.LabID, FirmwareVersion: mi.FirmwareVersion,
			OwnerID: req.OwnerID, RegisteredAt: now, Version: 1,
			ShardID: shardID, DataVersion: 1,
		}
		if err := s.store.ChargeSessionRepo().Save(ctx, session); err != nil {
			return nil, fmt.Errorf("save session %s: %w", mi.SessionNo, err)
		}
		sharedRecordTelemetry(ctx, s.store, s.clock, shardID, req.FormNo, mi.SessionNo,
			req.OwnerID, domain.TelemetryEntryTypeRegistration, "", domain.SessionStateRequested)
	}
	sharedAppendAudit(ctx, s.store, s.clock, req.OwnerID, "register", domain.EntityTypeHandshake, form.ID, shardID, "", form.State,
		fmt.Sprintf("form %s with %d sessions", req.FormNo, len(req.ChargeSessions)))
	sharedPublishEvent(ctx, s.eventBus, s.clock, domain.EventHandshakeRegistered, form.ID, shardID, form)
	return form, nil
}

func (s *HandshakeService) Attestation(ctx context.Context, req domain.HandshakeAttestationRequest) (*domain.ProtocolHandshake, error) {
	form, err := s.store.HandshakeRepo().Get(ctx, req.FormID)
	if err != nil {
		return nil, fmt.Errorf("get handshake: %w", err)
	}
	if form.State == domain.HandshakeStateDualSigned {
		return nil, fmt.Errorf("%w: already dual-signed", domain.ErrAttestationIncomplete)
	}
	if form.State == domain.HandshakeStateVoided {
		return nil, fmt.Errorf("%w: form voided", domain.ErrInvalidTransition)
	}
	nextState := domain.HandshakeNextStateAfterAttestation(form.State, req.Party)
	if nextState == form.State {
		return nil, fmt.Errorf("%w: party %s cannot sign in state %s", domain.ErrInvalidTransition, req.Party, form.State)
	}
	if err := domain.ValidateHandshakeTransition(form.State, nextState); err != nil {
		return nil, fmt.Errorf("validate transition: %w", err)
	}
	prevState := form.State
	form.State = nextState
	form.Version++
	form.UpdatedAt = s.clock.Now()
	if req.Party == domain.AttestationPartyOutbound {
		form.OutboundSigner = req.Signer
		form.OutboundSignedAt = s.clock.Now()
	} else {
		form.ArrivalSigner = req.Signer
		form.ArrivalSignedAt = s.clock.Now()
	}
	if err := s.store.HandshakeRepo().Save(ctx, form); err != nil {
		return nil, fmt.Errorf("save handshake after attestation: %w", err)
	}
	sharedRecordTelemetry(ctx, s.store, s.clock, form.ShardID, form.FormNo, "",
		req.Signer, domain.TelemetryEntryTypeAttestation, prevState, nextState)
	sharedAppendAudit(ctx, s.store, s.clock, req.Signer, "attestation_"+req.Party, domain.EntityTypeHandshake, form.ID, form.ShardID, prevState, nextState, "")
	sharedPublishEvent(ctx, s.eventBus, s.clock, domain.EventHandshakeSigned, form.ID, form.ShardID, form)
	return form, nil
}

func (s *HandshakeService) Get(ctx context.Context, id string) (*domain.ProtocolHandshake, error) {
	return s.store.HandshakeRepo().Get(ctx, id)
}

func (s *HandshakeService) List(ctx context.Context, filter storage.HandshakeFilter) ([]*domain.ProtocolHandshake, int, error) {
	return s.store.HandshakeRepo().List(ctx, filter)
}

func (s *HandshakeService) ModifySave(ctx context.Context, form *domain.ProtocolHandshake) error {
	return s.store.HandshakeRepo().Save(ctx, form)
}

var attestationMu sync.Mutex

func (s *HandshakeService) AttestationWithLock(ctx context.Context, req domain.HandshakeAttestationRequest) (*domain.ProtocolHandshake, error) {
	attestationMu.Lock()
	defer attestationMu.Unlock()
	return s.Attestation(ctx, req)
}
