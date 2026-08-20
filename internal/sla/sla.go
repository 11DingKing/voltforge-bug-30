package sla

import (
	"fmt"
	"sync"
	"time"

	"voltforge/internal/domain"
)

// Rule lifecycle states for a transit-SLA rule.
const (
	RuleStateDraft      = "draft"
	RuleStateActive     = "active"
	RuleStateDeprecated = "deprecated"
)

// Classification labels describing a session's transit timeliness.
const (
	ClassOnTime       = "on_time"
	ClassOverdue      = "overdue"
	ClassCritical     = "critical"
	ClassUnknownRoute = "unknown_protocol"
	ClassNotInTransit = "not_negotiating"
)

// Rule binds a protocol to the maximum admissible transit duration.
type Rule struct {
	ProtocolID      string
	MaxTransitHours int
	State           string
}

// RuleSet is a concurrency-safe registry of transit-SLA rules. When no
// active rule matches a protocol, the default maximum is used as a fallback.
type RuleSet struct {
	mu         sync.RWMutex
	rules      map[string]*Rule
	defaultMax int
	sm         *domain.StateMachine
}

// NewRuleSet builds an empty rule set. defaultMaxHours is the fallback
// applied when no active rule exists for a protocol.
func NewRuleSet(defaultMaxHours int) *RuleSet {
	rs := &RuleSet{rules: map[string]*Rule{}, defaultMax: defaultMaxHours}
	rs.sm = domain.NewStateMachine("sla_rule", RuleStateDraft,
		domain.StateTransition{From: RuleStateDraft, To: RuleStateActive},
		domain.StateTransition{From: RuleStateActive, To: RuleStateDeprecated},
		domain.StateTransition{From: RuleStateDeprecated, To: RuleStateActive},
	)
	return rs
}

// Add registers a rule in draft state, overwriting any prior rule for the protocol.
func (rs *RuleSet) Add(r *Rule) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	r.State = RuleStateDraft
	rs.rules[r.ProtocolID] = r
}

// transition mutates a rule's lifecycle, rejecting illegal moves with a
// wrapped domain error so callers can distinguish not-found vs bad-transition.
func (rs *RuleSet) transition(protocolID, target string) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	r, ok := rs.rules[protocolID]
	if !ok {
		return fmt.Errorf("%w: sla rule %s", domain.ErrNotFound, protocolID)
	}
	if err := rs.sm.Validate(r.State, target); err != nil {
		return fmt.Errorf("sla rule transition: %w", err)
	}
	r.State = target
	return nil
}

// Activate promotes a draft rule, or re-activates a deprecated one.
func (rs *RuleSet) Activate(protocolID string) error {
	return rs.transition(protocolID, RuleStateActive)
}

// Deprecate retires an active rule so it no longer governs classification.
func (rs *RuleSet) Deprecate(protocolID string) error {
	return rs.transition(protocolID, RuleStateDeprecated)
}

// activeMax returns the transit ceiling for a protocol, or the default.
func (rs *RuleSet) activeMax(protocolID string) int {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	if r, ok := rs.rules[protocolID]; ok && r.State == RuleStateActive {
		return r.MaxTransitHours
	}
	return rs.defaultMax
}

// Classify determines a session's timeliness against its protocol SLA. A session not
// in transit, or with no recorded transit start, yields a non-overdue label.
// Elapsed time is compared with closed lower bounds so a session exactly at the
// SLA ceiling is still on-time, and exactly at twice the ceiling is overdue
// (not critical): the boundary belongs to the more lenient bucket.
func (rs *RuleSet) Classify(m *domain.ChargeSession, now time.Time) string {
	if m == nil || m.State != domain.SessionStateNegotiating {
		return ClassNotInTransit
	}
	if m.InTransitAt.IsZero() {
		return ClassUnknownRoute
	}
	maxH := rs.activeMax(m.ProtocolID)
	if maxH <= 0 {
		return ClassUnknownRoute
	}
	critH := maxH * 2
	elapsed := now.Sub(m.InTransitAt)
	switch {
	case elapsed <= time.Duration(maxH)*time.Hour:
		return ClassOnTime
	case elapsed <= time.Duration(critH)*time.Hour:
		return ClassOverdue
	default:
		return ClassCritical
	}
}

// Summary tallies classifications across a batch of sessions.
func (rs *RuleSet) Summary(sessions []*domain.ChargeSession, now time.Time) map[string]int {
	out := map[string]int{}
	for _, m := range sessions {
		out[rs.Classify(m, now)]++
	}
	return out
}
