package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSessionLifecycleAndRoleBoundaries(t *testing.T) {
	now := time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC)
	service := New(time.Hour, func() time.Time { return now })
	session, err := service.Login(context.Background(), "labreviewer", "labreviewer-demo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RequireRole(context.Background(), session.ID, RoleAuditor); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected role denial, got %v", err)
	}
	if _, err := service.RequireRole(context.Background(), session.ID, RoleLabReviewer); err != nil {
		t.Fatalf("expected labreviewer access, got %v", err)
	}
	if err := service.Logout(context.Background(), session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(context.Background(), session.ID); !errors.Is(err, ErrSessionRevoked) {
		t.Fatalf("expected revoked session, got %v", err)
	}
}

func TestExpiredSessionIsRejected(t *testing.T) {
	now := time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC)
	service := New(time.Minute, func() time.Time { return now })
	session, err := service.Login(context.Background(), "vendorengineer", "vendorengineer-demo")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := service.Authenticate(context.Background(), session.ID); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("expected expiry, got %v", err)
	}
}
