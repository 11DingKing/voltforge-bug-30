package domain

import (
	"errors"
)

var (
	ErrNotFound              = errors.New("entity not found")
	ErrAlreadyExists         = errors.New("entity already exists")
	ErrInvalidTransition     = errors.New("invalid state transition")
	ErrConflict              = errors.New("concurrent write conflict")
	ErrDuplicateRequest      = errors.New("duplicate request")
	ErrBatchPartialFail      = errors.New("batch partially failed")
	ErrAttestationIncomplete = errors.New("dual-party attestation incomplete")
	ErrMitigationActive      = errors.New("an active mitigation already exists for this session")
	ErrShardCorrupted        = errors.New("shard file corrupted")
	ErrSlowConsumer          = errors.New("subscriber too slow, evicted")
	ErrStaleCheckpoint       = errors.New("subscriber checkpoint is stale, resync required")
	ErrPermanentFailure      = errors.New("task permanently failed")
	ErrValidation            = errors.New("validation error")
	ErrNotReady              = errors.New("service not ready")
)

type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

func (e ValidationError) Is(target error) bool {
	return target == ErrValidation
}

type ConflictError struct {
	EntityID  string
	Current   string
	Attempted string
	Reason    string
}

func (e ConflictError) Error() string {
	return "conflict on " + e.EntityID + ": current=" + e.Current + ", attempted=" + e.Attempted + ", reason=" + e.Reason
}

func (e ConflictError) Is(target error) bool {
	return target == ErrConflict
}

type TransitionError struct {
	EntityID string
	From     string
	To       string
	Allowed  []string
}

func (e TransitionError) Error() string {
	return "illegal transition on " + e.EntityID + ": " + e.From + " -> " + e.To
}

func (e TransitionError) Is(target error) bool {
	return target == ErrInvalidTransition
}

type StateTransition struct {
	From string
	To   string
}

type StateMachine struct {
	name        string
	initial     string
	transitions map[StateTransition]bool
}

func NewStateMachine(name, initial string, pairs ...StateTransition) *StateMachine {
	sm := &StateMachine{
		name:        name,
		initial:     initial,
		transitions: make(map[StateTransition]bool, len(pairs)),
	}
	for _, p := range pairs {
		sm.transitions[p] = true
	}
	return sm
}

func (sm *StateMachine) Initial() string { return sm.initial }

func (sm *StateMachine) CanTransition(from, to string) bool {
	return sm.transitions[StateTransition{From: from, To: to}]
}

func (sm *StateMachine) Validate(current, target string) error {
	if current == target {
		return nil
	}
	if sm.CanTransition(current, target) {
		return nil
	}
	allowed := sm.allowedTargets(current)
	return TransitionError{
		EntityID: sm.name,
		From:     current,
		To:       target,
		Allowed:  allowed,
	}
}

func (sm *StateMachine) allowedTargets(from string) []string {
	var result []string
	for t := range sm.transitions {
		if t.From == from {
			result = append(result, t.To)
		}
	}
	return result
}
