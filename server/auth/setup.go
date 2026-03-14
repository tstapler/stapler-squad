package auth

import (
	"crypto/subtle"
	"sync"
	"time"

	"claude-squad/log"
)

const setupTokenTTL = 24 * time.Hour

// SetupManager handles the bootstrap flow: a one-time setup token that
// allows the first passkey to be registered without existing auth.
//
// The setup token is printed to the server console on startup (when no
// passkeys are registered) and is valid for setupTokenTTL.
type SetupManager struct {
	mu        sync.Mutex
	token     string
	expiresAt time.Time
	used      bool
}

// NewSetupManager creates a SetupManager. Call Init() to generate a token.
func NewSetupManager() *SetupManager {
	return &SetupManager{}
}

// Init generates a new single-use setup token and logs it.
// Called when the server starts with no registered credentials.
func (s *SetupManager) Init() (string, error) {
	token, err := randomHex(16) // 32 hex chars is plenty
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = token
	s.expiresAt = time.Now().Add(setupTokenTTL)
	s.used = false

	log.InfoLog.Printf("auth: setup token generated (expires in 24h)")
	return token, nil
}

// Validate checks the provided token against the stored token.
// Returns true if valid; consumes the token on success (single-use).
func (s *SetupManager) Validate(candidate string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.token == "" || s.used {
		return false
	}
	if time.Now().After(s.expiresAt) {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(s.token), []byte(candidate)) != 1 {
		return false
	}
	s.used = true
	return true
}

// IsActive returns true if a valid (unused, non-expired) setup token exists.
func (s *SetupManager) IsActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token != "" && !s.used && time.Now().Before(s.expiresAt)
}
