package charging

import (
	"errors"
	"time"
)

var (
	ErrNotFound                = errors.New("charging record not found")
	ErrInvalidState            = errors.New("invalid charging state transition")
	ErrAlreadyClosed           = errors.New("issue already closed")
	ErrMissingFirmwareEvidence = errors.New("charge_test firmware_evidence is required")
)

type ProductStatus string

const (
	ProductActive    ProductStatus = "active"
	ProductSuspended ProductStatus = "suspended"
)

type IssueState string

const (
	IssueOpen           IssueState = "open"
	IssueAssigned       IssueState = "assigned"
	IssueMitigated      IssueState = "mitigated"
	IssueCertified      IssueState = "certified"
	IssueRetestRequired IssueState = "retest_required"
)

type Product struct {
	ID                  string        `json:"id"`
	Name                string        `json:"name"`
	Market              string        `json:"market"`
	VendorID            string        `json:"vendor_id"`
	Status              ProductStatus `json:"status"`
	SupportedProtocols  string        `json:"supported_protocols"`
	MaxPowerWatts       int           `json:"max_power_watts"`
	GaN                 bool          `json:"gan"`
	PortCount           int           `json:"port_count"`
	ThermalLimitC       float64       `json:"thermal_limit_c"`
	BatteryArchitecture string        `json:"battery_architecture"`
	CellCount           int           `json:"cell_count"`
	CreatedAt           time.Time     `json:"created_at"`
	UpdatedAt           time.Time     `json:"updated_at"`
	Version             int           `json:"version"`
}

type Issue struct {
	ID               string     `json:"id"`
	ProductID        string     `json:"product_id"`
	Kind             string     `json:"kind"`
	Severity         string     `json:"severity"`
	Description      string     `json:"description"`
	State            IssueState `json:"state"`
	SubmittedBy      string     `json:"submitted_by"`
	VendorAssigneeID string     `json:"vendor_assignee_id,omitempty"`
	ReviewDueAt      time.Time  `json:"review_due_at"`
	MitigatedAt      *time.Time `json:"mitigated_at,omitempty"`
	CertifiedAt      *time.Time `json:"certified_at,omitempty"`
	FirmwareEvidence string     `json:"firmware_evidence,omitempty"`
	Version          int        `json:"version"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type ChargeTest struct {
	ID                     string    `json:"id"`
	ProductID              string    `json:"product_id"`
	LabEngineerID          string    `json:"lab_engineer_id"`
	CheckedAt              time.Time `json:"checked_at"`
	ProtocolHandshakeOK    bool      `json:"protocol_handshake_ok"`
	CableCertificateExpiry time.Time `json:"cable_certificate_expiry"`
	ThermalControlOK       bool      `json:"thermal_control_ok"`
	PowerDisplayOK         bool      `json:"power_display_ok"`
	Notes                  string    `json:"notes"`
}

func (h Issue) CanTransition(next IssueState) bool {
	if h.State == next {
		return true
	}
	switch h.State {
	case IssueOpen:
		return next == IssueAssigned
	case IssueAssigned:
		return next == IssueMitigated
	case IssueMitigated:
		return next == IssueCertified || next == IssueRetestRequired
	case IssueRetestRequired:
		return next == IssueAssigned
	default:
		return false
	}
}
