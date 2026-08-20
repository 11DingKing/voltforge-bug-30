package cable

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrCertificateExpired = errors.New("cable certificate expired")
	ErrCertificateRevoked = errors.New("cable certificate revoked")
	ErrInsufficientRating = errors.New("cable rating is below negotiated power")
)

type Certificate struct {
	ID        string
	OwnerID   string
	Protocols []string
	MaxWatts  int
	ExpiresAt time.Time
	RevokedAt *time.Time
	Issuer    string
}

type Verifier struct {
	now func() time.Time
}

func NewVerifier(now func() time.Time) *Verifier {
	if now == nil {
		now = time.Now
	}
	return &Verifier{now: now}
}

func (v *Verifier) Verify(ctx context.Context, cert Certificate, protocolName string, watts int) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("certify cable: %w", err)
	}
	if cert.ID == "" || cert.Issuer == "" || cert.MaxWatts <= 0 || strings.TrimSpace(protocolName) == "" {
		return fmt.Errorf("invalid cable certificate")
	}
	if cert.RevokedAt != nil && !cert.RevokedAt.After(v.now()) {
		return ErrCertificateRevoked
	}
	if !cert.ExpiresAt.After(v.now()) {
		return ErrCertificateExpired
	}
	if watts <= 0 || watts > cert.MaxWatts {
		return ErrInsufficientRating
	}
	for _, supported := range cert.Protocols {
		if supported == protocolName {
			return nil
		}
	}
	return fmt.Errorf("protocol %s is not certified", protocolName)
}
