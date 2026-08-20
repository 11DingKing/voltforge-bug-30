package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvalidSessionTransition(t *testing.T) {
	err := ValidateSessionTransition(SessionStateRequested, SessionStateCompleted)
	require.Error(t, err)
	assert.True(t, errIs(err, ErrInvalidTransition))
}

func TestIllegalHandshakeTransition(t *testing.T) {
	err := ValidateHandshakeTransition(HandshakeStateVoided, HandshakeStateDualSigned)
	require.Error(t, err)
	var te TransitionError
	assert.True(t, errAs(err, &te))
	assert.Equal(t, HandshakeStateVoided, te.From)
	assert.Equal(t, HandshakeStateDualSigned, te.To)
}

func TestRejectMitigationTransition(t *testing.T) {
	err := ValidateMitigationTransition(MitigationStateCompleted, MitigationStatePending)
	require.Error(t, err)
	assert.True(t, errIs(err, ErrInvalidTransition))
}

func TestValidSessionTransitions(t *testing.T) {
	cases := []struct{ from, to string }{
		{SessionStateRequested, SessionStateCapabilityChecked},
		{SessionStateCapabilityChecked, SessionStateNegotiating},
		{SessionStateNegotiating, SessionStateCharging},
		{SessionStateCharging, SessionStateAuthorized},
		{SessionStateAuthorized, SessionStateCompleted},
		{SessionStateNegotiating, SessionStateThermalThrottled},
		{SessionStateThermalThrottled, SessionStateRejected},
	}
	for _, c := range cases {
		err := ValidateSessionTransition(c.from, c.to)
		require.NoError(t, err, "from=%s to=%s", c.from, c.to)
	}
}

func TestValidHandshakeTransitions(t *testing.T) {
	cases := []struct{ from, to string }{
		{HandshakeStateDraft, HandshakeStateOutboundSigned},
		{HandshakeStateOutboundSigned, HandshakeStateDualSigned},
		{HandshakeStateDraft, HandshakeStateArrivalSigned},
		{HandshakeStateArrivalSigned, HandshakeStateDualSigned},
	}
	for _, c := range cases {
		err := ValidateHandshakeTransition(c.from, c.to)
		require.NoError(t, err, "from=%s to=%s", c.from, c.to)
	}
}

func TestValidMitigationTransitions(t *testing.T) {
	cases := []struct{ from, to string }{
		{MitigationStatePending, MitigationStateUnderReview},
		{MitigationStateUnderReview, MitigationStateIssued},
		{MitigationStateIssued, MitigationStateExecuting},
		{MitigationStateExecuting, MitigationStateCompleted},
		{MitigationStateIssued, MitigationStateWithdrawn},
	}
	for _, c := range cases {
		err := ValidateMitigationTransition(c.from, c.to)
		require.NoError(t, err, "from=%s to=%s", c.from, c.to)
	}
}

func TestHandshakeNextStateAfterAttestation(t *testing.T) {
	assert.Equal(t, HandshakeStateOutboundSigned,
		HandshakeNextStateAfterAttestation(HandshakeStateDraft, AttestationPartyOutbound))
	assert.Equal(t, HandshakeStateDualSigned,
		HandshakeNextStateAfterAttestation(HandshakeStateOutboundSigned, AttestationPartyArrival))
	assert.Equal(t, HandshakeStateDualSigned,
		HandshakeNextStateAfterAttestation(HandshakeStateArrivalSigned, AttestationPartyOutbound))
	assert.Equal(t, HandshakeStateOutboundSigned,
		HandshakeNextStateAfterAttestation(HandshakeStateOutboundSigned, AttestationPartyOutbound))
}

func TestBatchTransitions(t *testing.T) {
	require.NoError(t, ValidateBatchTransition(BatchStatePending, BatchStateProcessing))
	require.NoError(t, ValidateBatchTransition(BatchStateProcessing, BatchStateSucceeded))
	require.NoError(t, ValidateBatchTransition(BatchStateProcessing, BatchStateRolledBack))
	require.NoError(t, ValidateBatchTransition(BatchStateRolledBack, BatchStateProcessing))
	require.Error(t, ValidateBatchTransition(BatchStateCompleted, BatchStateProcessing))
}

func TestIsMitigationActive(t *testing.T) {
	assert.True(t, IsMitigationActive(MitigationStatePending))
	assert.True(t, IsMitigationActive(MitigationStateIssued))
	assert.False(t, IsMitigationActive(MitigationStateCompleted))
	assert.False(t, IsMitigationActive(MitigationStateRejected))
}

func errIs(err, target error) bool {
	return errorIs(err, target)
}

func errAs(err error, target any) bool {
	return errorAs(err, target)
}
