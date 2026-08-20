package domain

import (
	"time"
)

const (
	MitigationTypeProtocolFallback = "protocol_fallback"
	MitigationTypeThermalThrottle  = "thermal_throttle"
	MitigationTypeStopCharge       = "stop_charge"
)

const (
	MitigationStatePending     = "pending"
	MitigationStateUnderReview = "under_review"
	MitigationStateIssued      = "issued"
	MitigationStateExecuting   = "executing"
	MitigationStateCompleted   = "completed"
	MitigationStateRejected    = "retest_required"
	MitigationStateWithdrawn   = "withdrawn"
	MitigationStateLost        = "lost"
)

var mitigationStateMachine = NewStateMachine("mitigation", MitigationStatePending,
	StateTransition{MitigationStatePending, MitigationStateUnderReview},
	StateTransition{MitigationStatePending, MitigationStateIssued},
	StateTransition{MitigationStatePending, MitigationStateRejected},
	StateTransition{MitigationStateUnderReview, MitigationStateIssued},
	StateTransition{MitigationStateUnderReview, MitigationStateRejected},
	StateTransition{MitigationStateIssued, MitigationStateExecuting},
	StateTransition{MitigationStateExecuting, MitigationStateCompleted},
	StateTransition{MitigationStateIssued, MitigationStateWithdrawn},
	StateTransition{MitigationStateExecuting, MitigationStateWithdrawn},
	StateTransition{MitigationStatePending, MitigationStateWithdrawn},
	StateTransition{MitigationStatePending, MitigationStateLost},
	StateTransition{MitigationStateUnderReview, MitigationStateLost},
	StateTransition{MitigationStateWithdrawn, MitigationStatePending},
)

type SafetyMitigation struct {
	ID              string    `json:"id"`
	RequestNo       string    `json:"request_no"`
	SessionID       string    `json:"session_id"`
	SessionNo       string    `json:"session_no"`
	Type            string    `json:"type"`
	TargetAddress   string    `json:"target_address,omitempty"`
	State           string    `json:"state"`
	SubmittedBy     string    `json:"submitted_by"`
	SubmittedAt     time.Time `json:"submitted_at"`
	ReviewedBy      string    `json:"reviewed_by,omitempty"`
	ReviewedAt      time.Time `json:"reviewed_at,omitempty"`
	ReviewNote      string    `json:"review_note,omitempty"`
	IssuedBy        string    `json:"issued_by,omitempty"`
	IssuedAt        time.Time `json:"issued_at,omitempty"`
	ExecutedAt      time.Time `json:"executed_at,omitempty"`
	CompletedAt     time.Time `json:"completed_at,omitempty"`
	WithdrawnBy     string    `json:"withdrawn_by,omitempty"`
	WithdrawnAt     time.Time `json:"withdrawn_at,omitempty"`
	WithdrawnReason string    `json:"withdrawn_reason,omitempty"`
	ConflictReason  string    `json:"conflict_reason,omitempty"`
	LostAt          time.Time `json:"lost_at,omitempty"`
	Version         int       `json:"version"`
	ShardID         string    `json:"shard_id"`
	DataVersion     int       `json:"data_version"`
}

const (
	AdjudicationActionSubmit   = "submit"
	AdjudicationActionReview   = "review"
	AdjudicationActionExecute  = "execute"
	AdjudicationActionComplete = "complete"
	AdjudicationActionWithdraw = "withdraw"
	AdjudicationActionLost     = "lost"
)

func ValidateMitigationTransition(current, target string) error {
	return mitigationStateMachine.Validate(current, target)
}

func IsMitigationActive(state string) bool {
	switch state {
	case MitigationStatePending, MitigationStateUnderReview,
		MitigationStateIssued, MitigationStateExecuting:
		return true
	default:
		return false
	}
}
