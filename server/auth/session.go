package auth

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

const (
	// sessionTokenLength is the number of random bytes for auth tokens and
	// WebAuthn ceremony session IDs.
	sessionTokenLength = 32

	// authTokenTTL is how long an authenticated session lasts.
	authTokenTTL = 30 * 24 * time.Hour

	// ceremonySessionTTL is how long a WebAuthn ceremony (register/login) is
	// valid. Must be long enough for the user to complete the browser flow.
	ceremonySessionTTL = 5 * time.Minute

	// AuthCookieName is the name of the HTTP session cookie.
	AuthCookieName = "cs_auth"
)

// SessionManager manages two distinct token spaces:
//  1. WebAuthn ceremony sessions (short-lived, indexed by a random key stored
//     in the browser session storage during the ceremony).
//  2. Authenticated sessions (long-lived, stored in an httpOnly cookie after
//     successful login or registration).
//
// All state is in-memory; it is intentionally not persisted across server
// restarts to force re-authentication.
type SessionManager struct {
	mu          sync.Mutex
	ceremonies  map[string]*ceremony  // key → ceremony session
	authSessions map[string]*authSession // token → auth session
}

type ceremony struct {
	data      webauthn.SessionData
	expiresAt time.Time
	kind      ceremonyKind
}

type ceremonyKind int

const (
	ceremonyRegister ceremonyKind = iota
	ceremonyLogin
)

type authSession struct {
	token     string
	createdAt time.Time
	expiresAt time.Time
}

// NewSessionManager creates an empty SessionManager and starts a background
// goroutine to purge expired entries.
func NewSessionManager() *SessionManager {
	sm := &SessionManager{
		ceremonies:   make(map[string]*ceremony),
		authSessions: make(map[string]*authSession),
	}
	go sm.cleanup()
	return sm
}

// StoreCeremony stores the WebAuthn session data for an in-progress ceremony
// and returns a random key the client must echo back.
func (sm *SessionManager) StoreCeremony(kind ceremonyKind, data webauthn.SessionData) (string, error) {
	key, err := randomHex(sessionTokenLength)
	if err != nil {
		return "", err
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.ceremonies[key] = &ceremony{
		data:      data,
		expiresAt: time.Now().Add(ceremonySessionTTL),
		kind:      kind,
	}
	return key, nil
}

// GetCeremony retrieves and removes the ceremony session data for the given key.
// Returns false if not found or expired.
func (sm *SessionManager) GetCeremony(key string) (webauthn.SessionData, bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	c, ok := sm.ceremonies[key]
	if !ok || time.Now().After(c.expiresAt) {
		delete(sm.ceremonies, key)
		return webauthn.SessionData{}, false
	}
	delete(sm.ceremonies, key)
	return c.data, true
}

// CreateAuthSession issues a new authenticated session token.
func (sm *SessionManager) CreateAuthSession() (string, error) {
	token, err := randomHex(sessionTokenLength)
	if err != nil {
		return "", err
	}

	now := time.Now()
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.authSessions[token] = &authSession{
		token:     token,
		createdAt: now,
		expiresAt: now.Add(authTokenTTL),
	}
	return token, nil
}

// ValidateAuthSession returns true if the token is valid and not expired.
func (sm *SessionManager) ValidateAuthSession(token string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	s, ok := sm.authSessions[token]
	if !ok {
		return false
	}
	if time.Now().After(s.expiresAt) {
		delete(sm.authSessions, token)
		return false
	}
	return true
}

// RevokeAuthSession invalidates a specific session token (logout).
func (sm *SessionManager) RevokeAuthSession(token string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.authSessions, token)
}

// RevokeAllSessions invalidates all authenticated sessions (force re-auth).
func (sm *SessionManager) RevokeAllSessions() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.authSessions = make(map[string]*authSession)
}

// AuthTokenTTL returns the authentication token TTL for use in cookie Max-Age.
func AuthTokenTTL() time.Duration {
	return authTokenTTL
}

// cleanup periodically removes expired entries.
func (sm *SessionManager) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		sm.mu.Lock()
		for k, c := range sm.ceremonies {
			if now.After(c.expiresAt) {
				delete(sm.ceremonies, k)
			}
		}
		for k, s := range sm.authSessions {
			if now.After(s.expiresAt) {
				delete(sm.authSessions, k)
			}
		}
		sm.mu.Unlock()
	}
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
