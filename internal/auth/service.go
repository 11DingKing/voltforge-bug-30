package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrSessionRevoked     = errors.New("session revoked")
	ErrSessionExpired     = errors.New("session expired")
	ErrForbidden          = errors.New("forbidden")
)

type Role string

const (
	RoleAuditor        Role = "auditor"
	RoleLabReviewer    Role = "labreviewer"
	RoleVendorEngineer Role = "vendorengineer"
	RoleTestEngineer   Role = "testengineer"
)

type User struct {
	ID           string
	Username     string
	PasswordHash string
	Role         Role
}

type Session struct {
	ID        string
	UserID    string
	Role      Role
	ExpiresAt time.Time
	RevokedAt *time.Time
}

type Service struct {
	mu         sync.RWMutex
	users      map[string]User
	sessions   map[string]Session
	now        func() time.Time
	ttl        time.Duration
	repository Repository
}

type Repository interface {
	FindUser(ctx context.Context, username string) (User, error)
	SaveSession(ctx context.Context, session Session) error
	FindSession(ctx context.Context, token string) (Session, error)
	RevokeSession(ctx context.Context, token string, revokedAt time.Time) error
}

func New(ttl time.Duration, now func() time.Time, repositories ...Repository) *Service {
	if ttl <= 0 {
		ttl = 8 * time.Hour
	}
	if now == nil {
		now = time.Now
	}
	service := &Service{
		users: map[string]User{
			"auditor":        {ID: "u-auditor", Username: "auditor", PasswordHash: hashPassword("auditor-demo"), Role: RoleAuditor},
			"labreviewer":    {ID: "u-labreviewer", Username: "labreviewer", PasswordHash: hashPassword("labreviewer-demo"), Role: RoleLabReviewer},
			"vendorengineer": {ID: "u-vendorengineer", Username: "vendorengineer", PasswordHash: hashPassword("vendorengineer-demo"), Role: RoleVendorEngineer},
			"testengineer":   {ID: "u-testengineer", Username: "testengineer", PasswordHash: hashPassword("testengineer-demo"), Role: RoleTestEngineer},
		},
		sessions: make(map[string]Session), now: now, ttl: ttl,
	}
	if len(repositories) > 0 {
		service.repository = repositories[0]
	}
	return service
}

func (s *Service) Login(ctx context.Context, username, password string) (Session, error) {
	if err := ctx.Err(); err != nil {
		return Session{}, fmt.Errorf("login: %w", err)
	}
	user, ok := s.users[username]
	var err error
	if s.repository != nil {
		user, err = s.repository.FindUser(ctx, username)
		ok = err == nil
	}
	if !ok || user.PasswordHash != hashPassword(password) {
		return Session{}, ErrInvalidCredentials
	}
	token, err := randomToken()
	if err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}
	now := s.now()
	session := Session{ID: token, UserID: user.ID, Role: user.Role, ExpiresAt: now.Add(s.ttl)}
	s.mu.Lock()
	s.sessions[token] = session
	s.mu.Unlock()
	if s.repository != nil {
		if err := s.repository.SaveSession(ctx, session); err != nil {
			return Session{}, fmt.Errorf("persist session: %w", err)
		}
	}
	return session, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (Session, error) {
	if err := ctx.Err(); err != nil {
		return Session{}, fmt.Errorf("authenticate: %w", err)
	}
	s.mu.RLock()
	session, ok := s.sessions[token]
	s.mu.RUnlock()
	if !ok && s.repository != nil {
		var err error
		session, err = s.repository.FindSession(ctx, token)
		ok = err == nil
	}
	if !ok {
		return Session{}, ErrSessionRevoked
	}
	now := s.now()
	if session.RevokedAt != nil {
		return Session{}, ErrSessionRevoked
	}
	if !now.Before(session.ExpiresAt) {
		return Session{}, ErrSessionExpired
	}
	return session, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	session, err := s.Authenticate(ctx, token)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if s.repository != nil {
		if err := s.repository.RevokeSession(ctx, token, now); err != nil {
			return ErrSessionRevoked
		}
	}
	session.RevokedAt = &now
	s.sessions[token] = session
	return nil
}

func hashPassword(password string) string {
	digest := sha256.Sum256([]byte(password))
	return hex.EncodeToString(digest[:])
}

func (s *Service) RequireRole(ctx context.Context, token string, allowed ...Role) (Session, error) {
	session, err := s.Authenticate(ctx, token)
	if err != nil {
		return Session{}, err
	}
	for _, role := range allowed {
		if session.Role == role {
			return session, nil
		}
	}
	return Session{}, ErrForbidden
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	digest := sha256.Sum256(buf)
	return hex.EncodeToString(digest[:]), nil
}
