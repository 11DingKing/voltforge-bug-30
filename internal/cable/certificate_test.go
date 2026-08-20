package cable

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestVerifierChecksExpiryRevocationProtocolAndRating(t *testing.T) {
	now := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	v := NewVerifier(func() time.Time { return now })
	cert := Certificate{ID: "c-1", Issuer: "lab", Protocols: []string{"PPS", "QC"}, MaxWatts: 100, ExpiresAt: now.Add(time.Hour)}
	if err := v.Verify(context.Background(), cert, "PPS", 80); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(v.Verify(context.Background(), cert, "PPS", 120), ErrInsufficientRating) {
		t.Fatal("rating should be enforced")
	}
	cert.ExpiresAt = now
	if !errors.Is(v.Verify(context.Background(), cert, "PPS", 80), ErrCertificateExpired) {
		t.Fatal("expiry should be enforced")
	}
	cert.ExpiresAt = now.Add(time.Hour)
	revoked := now
	cert.RevokedAt = &revoked
	if !errors.Is(v.Verify(context.Background(), cert, "PPS", 80), ErrCertificateRevoked) {
		t.Fatal("revocation should be enforced")
	}
}

func TestVerifierPropagatesCancellation(t *testing.T) {
	v := NewVerifier(time.Now)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := v.Verify(ctx, Certificate{ID: "c", Issuer: "lab", MaxWatts: 1, ExpiresAt: time.Now().Add(time.Hour)}, "PPS", 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}
