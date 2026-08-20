package service

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"voltforge/internal/domain"
)

func TestDualPartyAttestation(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	handSvc := NewHandshakeService(store, clock, bus)
	form := registerTestHandshake(t, ctx, handSvc, "F001", "2026-08-19", "R001")
	assert.Equal(t, domain.HandshakeStateDraft, form.State)
	form, err := handSvc.Attestation(ctx, domain.HandshakeAttestationRequest{
		FormID: form.ID, Party: domain.AttestationPartyOutbound, Signer: "outbound-op", Product: "JMS-01",
	})
	require.NoError(t, err)
	assert.Equal(t, domain.HandshakeStateOutboundSigned, form.State)
	form, err = handSvc.Attestation(ctx, domain.HandshakeAttestationRequest{
		FormID: form.ID, Party: domain.AttestationPartyArrival, Signer: "arrival-op", Product: "JMS-02",
	})
	require.NoError(t, err)
	assert.Equal(t, domain.HandshakeStateDualSigned, form.State)
}

func TestSinglePartyAttestationIncomplete(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	handSvc := NewHandshakeService(store, clock, bus)
	form := registerTestHandshake(t, ctx, handSvc, "F002", "2026-08-19", "R001")
	form, err := handSvc.Attestation(ctx, domain.HandshakeAttestationRequest{
		FormID: form.ID, Party: domain.AttestationPartyOutbound, Signer: "outbound-op", Product: "JMS-01",
	})
	require.NoError(t, err)
	assert.False(t, domain.IsHandshakeComplete(form.State))
	assert.Equal(t, domain.HandshakeStateOutboundSigned, form.State)
}

func TestHandshakeIdempotentRegister(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	handSvc := NewHandshakeService(store, clock, bus)
	form1 := registerTestHandshake(t, ctx, handSvc, "F003", "2026-08-19", "R001")
	assert.NotEmpty(t, form1.ID)
	_, err := handSvc.Register(ctx, RegisterHandshakeRequest{
		FormNo: "F003", Date: "2026-08-19", ProtocolID: "R001",
		ChargeSessions: []RegisterChargeSession{{SessionNo: "F003-M1"}},
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrAlreadyExists))
}

func TestHandshakeRejectDoubleAttestation(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	handSvc := NewHandshakeService(store, clock, bus)
	form := registerTestHandshake(t, ctx, handSvc, "F004", "2026-08-19", "R001")
	form, _ = handSvc.Attestation(ctx, domain.HandshakeAttestationRequest{
		FormID: form.ID, Party: domain.AttestationPartyOutbound, Signer: "op1", Product: "JMS-01",
	})
	form, _ = handSvc.Attestation(ctx, domain.HandshakeAttestationRequest{
		FormID: form.ID, Party: domain.AttestationPartyArrival, Signer: "op2", Product: "JMS-02",
	})
	_, err := handSvc.Attestation(ctx, domain.HandshakeAttestationRequest{
		FormID: form.ID, Party: domain.AttestationPartyOutbound, Signer: "op3", Product: "JMS-01",
	})
	require.Error(t, err)
}

func TestHandshakeVoidedRejectsAttestation(t *testing.T) {
	store, clock, bus, ctx, cleanup := setupTestEnv(t)
	defer cleanup()
	handSvc := NewHandshakeService(store, clock, bus)
	form := registerTestHandshake(t, ctx, handSvc, "F005", "2026-08-19", "R001")
	form.State = domain.HandshakeStateVoided
	form.Version++
	require.NoError(t, store.HandshakeRepo().Save(ctx, form))
	_, err := handSvc.Attestation(ctx, domain.HandshakeAttestationRequest{
		FormID: form.ID, Party: domain.AttestationPartyOutbound, Signer: "op1", Product: "JMS-01",
	})
	require.Error(t, err)
}
