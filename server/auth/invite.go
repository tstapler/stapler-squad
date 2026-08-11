package auth

import (
	"crypto/subtle"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

const (
	inviteTTL      = 15 * time.Minute
	inviteMaxSlots = 5
)

type inviteEntry struct {
	token     string
	label     string
	expiresAt time.Time
}

// InviteManager issues short-lived one-time tokens that allow an unauthenticated
// device to register a passkey. Unlike SetupManager (bootstrap-only, file-backed),
// InviteManager is in-memory and requires an authenticated caller to generate tokens.
type InviteManager struct {
	mu      sync.Mutex
	entries []inviteEntry
}

// NewInviteManager creates an InviteManager.
func NewInviteManager() *InviteManager {
	return &InviteManager{}
}

// Generate creates a new invite token with the given label, evicting the oldest
// entry if the slot limit is reached. Returns the token and its expiry time.
func (m *InviteManager) Generate(label string) (token string, expiresAt time.Time, err error) {
	token, err = randomHex(16)
	if err != nil {
		return "", time.Time{}, err
	}

	expiresAt = time.Now().Add(inviteTTL)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.evictExpiredLocked()

	if len(m.entries) >= inviteMaxSlots {
		m.entries = m.entries[1:]
	}

	m.entries = append(m.entries, inviteEntry{
		token:     token,
		label:     label,
		expiresAt: expiresAt,
	})

	log.Info("auth: invite token generated", "label", label, "expires_in", "15m")
	return token, expiresAt, nil
}

// IsValid checks whether the candidate token is valid without consuming it.
func (m *InviteManager) IsValid(candidate string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for _, e := range m.entries {
		if now.Before(e.expiresAt) && subtle.ConstantTimeCompare([]byte(e.token), []byte(candidate)) == 1 {
			return true
		}
	}
	return false
}

// Consume validates and removes the token atomically. Returns the associated
// label if successful, empty string if not found or expired.
func (m *InviteManager) Consume(candidate string) (label string, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for i, e := range m.entries {
		if now.Before(e.expiresAt) && subtle.ConstantTimeCompare([]byte(e.token), []byte(candidate)) == 1 {
			m.entries = append(m.entries[:i], m.entries[i+1:]...)
			log.Info("auth: invite token consumed", "label", e.label)
			return e.label, true
		}
	}
	return "", false
}

func (m *InviteManager) evictExpiredLocked() {
	now := time.Now()
	// Reuse the backing array to avoid an allocation. Safe because `range`
	// copies each element into `e` before `kept` can overwrite it.
	kept := m.entries[:0]
	for _, e := range m.entries {
		if now.Before(e.expiresAt) {
			kept = append(kept, e)
		}
	}
	m.entries = kept
}
