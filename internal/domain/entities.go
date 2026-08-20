package domain

import (
	"time"
)

const (
	SessionStateRequested         = "requested"
	SessionStateCapabilityChecked = "capability_checked"
	SessionStateNegotiating       = "negotiating"
	SessionStateCharging          = "charging"
	SessionStateAuthorized        = "authorized"
	SessionStateCompleted         = "completed"
	SessionStateThermalThrottled  = "thermal_throttled"
	SessionStateProtocolFallback  = "protocol_fallback"
	SessionStateRejected          = "retest_required"
)

var sessionStateMachine = NewStateMachine("session", SessionStateRequested,
	StateTransition{SessionStateRequested, SessionStateCapabilityChecked},
	StateTransition{SessionStateCapabilityChecked, SessionStateNegotiating},
	StateTransition{SessionStateNegotiating, SessionStateCharging},
	StateTransition{SessionStateNegotiating, SessionStateThermalThrottled},
	StateTransition{SessionStateNegotiating, SessionStateProtocolFallback},
	StateTransition{SessionStateCharging, SessionStateAuthorized},
	StateTransition{SessionStateProtocolFallback, SessionStateCharging},
	StateTransition{SessionStateAuthorized, SessionStateCompleted},
	StateTransition{SessionStateThermalThrottled, SessionStateRejected},
	StateTransition{SessionStateRejected, SessionStateCompleted},
)

type ChargeSession struct {
	ID              string    `json:"id"`
	SessionNo       string    `json:"session_no"`
	ProtocolID      string    `json:"protocol_id"`
	AdapterModel    string    `json:"adapter_model"`
	State           string    `json:"state"`
	HandshakeID     string    `json:"handshake_id,omitempty"`
	MitigationID    string    `json:"mitigation_id,omitempty"`
	DeviceModel     string    `json:"device_model"`
	ChargerModel    string    `json:"charger_model"`
	VendorID        string    `json:"vendor_id"`
	CableID         string    `json:"cable_id"`
	LabID           string    `json:"lab_id"`
	FirmwareVersion string    `json:"firmware_version"`
	OwnerID         string    `json:"owner_id"`
	RegisteredAt    time.Time `json:"requested_at"`
	LoadedAt        time.Time `json:"capability_checked_at,omitempty"`
	InTransitAt     time.Time `json:"negotiating_at,omitempty"`
	ArrivedAt       time.Time `json:"charging_at,omitempty"`
	SignedAt        time.Time `json:"signed_at,omitempty"`
	CompletedAt     time.Time `json:"completed_at,omitempty"`
	Version         int       `json:"version"`
	ShardID         string    `json:"shard_id"`
	DataVersion     int       `json:"data_version"`
}

func ValidateSessionTransition(current, target string) error {
	return sessionStateMachine.Validate(current, target)
}

const (
	HandshakeStateDraft          = "draft"
	HandshakeStateOutboundSigned = "outbound_signed"
	HandshakeStateArrivalSigned  = "arrival_signed"
	HandshakeStateDualSigned     = "authorized"
	HandshakeStateVoided         = "voided"
)

var handshakeStateMachine = NewStateMachine("handshake", HandshakeStateDraft,
	StateTransition{HandshakeStateDraft, HandshakeStateOutboundSigned},
	StateTransition{HandshakeStateDraft, HandshakeStateArrivalSigned},
	StateTransition{HandshakeStateOutboundSigned, HandshakeStateArrivalSigned},
	StateTransition{HandshakeStateArrivalSigned, HandshakeStateOutboundSigned},
	StateTransition{HandshakeStateOutboundSigned, HandshakeStateDualSigned},
	StateTransition{HandshakeStateArrivalSigned, HandshakeStateDualSigned},
	StateTransition{HandshakeStateOutboundSigned, HandshakeStateVoided},
	StateTransition{HandshakeStateArrivalSigned, HandshakeStateVoided},
	StateTransition{HandshakeStateDraft, HandshakeStateVoided},
)

type ProtocolHandshake struct {
	ID                 string    `json:"id"`
	FormNo             string    `json:"form_no"`
	Date               string    `json:"date"`
	ProtocolID         string    `json:"protocol_id"`
	AdapterModel       string    `json:"adapter_model"`
	State              string    `json:"state"`
	OutboundProduct    string    `json:"outbound_product"`
	OutboundSigner     string    `json:"outbound_signer"`
	OutboundSignedAt   time.Time `json:"outbound_signed_at,omitempty"`
	ArrivalProduct     string    `json:"arrival_product"`
	ArrivalSigner      string    `json:"arrival_signer"`
	ArrivalSignedAt    time.Time `json:"arrival_signed_at,omitempty"`
	ChargeSessionCount int       `json:"session_item_count"`
	OwnerID            string    `json:"owner_id"`
	RegisteredAt       time.Time `json:"requested_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	Version            int       `json:"version"`
	ShardID            string    `json:"shard_id"`
	DataVersion        int       `json:"data_version"`
}

type HandshakeAttestationRequest struct {
	FormID  string `json:"form_id"`
	Party   string `json:"party"`
	Signer  string `json:"signer"`
	Product string `json:"product"`
}

const (
	AttestationPartyOutbound = "outbound"
	AttestationPartyArrival  = "arrival"
)

func ValidateHandshakeTransition(current, target string) error {
	return handshakeStateMachine.Validate(current, target)
}

func IsHandshakeComplete(state string) bool {
	return state == HandshakeStateDualSigned
}

func HandshakeNextStateAfterAttestation(current, party string) string {
	switch {
	case current == HandshakeStateDraft && party == AttestationPartyOutbound:
		return HandshakeStateOutboundSigned
	case current == HandshakeStateDraft && party == AttestationPartyArrival:
		return HandshakeStateArrivalSigned
	case current == HandshakeStateOutboundSigned && party == AttestationPartyArrival:
		return HandshakeStateDualSigned
	case current == HandshakeStateArrivalSigned && party == AttestationPartyOutbound:
		return HandshakeStateDualSigned
	default:
		return current
	}
}

const (
	BatchStatePending    = "pending"
	BatchStateProcessing = "processing"
	BatchStateSucceeded  = "succeeded"
	BatchStateRolledBack = "rolled_back"
	BatchStateCompleted  = "completed"
)

var batchStateMachine = NewStateMachine("batch", BatchStatePending,
	StateTransition{BatchStatePending, BatchStateProcessing},
	StateTransition{BatchStateProcessing, BatchStateSucceeded},
	StateTransition{BatchStateProcessing, BatchStateRolledBack},
	StateTransition{BatchStateRolledBack, BatchStateProcessing},
	StateTransition{BatchStateSucceeded, BatchStateCompleted},
)

const (
	BatchItemStatePending    = "pending"
	BatchItemStateSucceeded  = "succeeded"
	BatchItemStateFailed     = "failed"
	BatchItemStateRolledBack = "rolled_back"
)

type BatchRecord struct {
	ID             string    `json:"id"`
	AdapterModel   string    `json:"adapter_model"`
	Date           string    `json:"date"`
	ProtocolID     string    `json:"protocol_id"`
	State          string    `json:"state"`
	TotalCount     int       `json:"total_count"`
	SucceededCount int       `json:"succeeded_count"`
	FailedCount    int       `json:"failed_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Version        int       `json:"version"`
	ShardID        string    `json:"shard_id"`
	DataVersion    int       `json:"data_version"`
}

type BatchItem struct {
	ID        string    `json:"id"`
	BatchID   string    `json:"batch_id"`
	SessionID string    `json:"session_id"`
	SessionNo string    `json:"session_no"`
	State     string    `json:"state"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func ValidateBatchTransition(current, target string) error {
	return batchStateMachine.Validate(current, target)
}
