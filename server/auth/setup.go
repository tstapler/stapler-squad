package auth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/tstapler/stapler-squad/log"
)

const setupTokenTTL = time.Hour

// SetupTokenFile is the well-known filename written by print-qr-codes and
// watched by the running server.
const SetupTokenFile = "setup-token.json"

// setupTokenRecord is the on-disk representation of a setup token.
type setupTokenRecord struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// SetupManager handles the bootstrap flow: a one-time setup token that
// allows the first passkey to be registered without existing auth.
//
// Tokens are stored in-memory and can be refreshed from a file written by
// the print-qr-codes CLI command. The server watches the file via WatchFile.
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

// Init generates a new single-use setup token valid for setupTokenTTL and
// holds it in memory. Used at server startup when no passkeys are registered.
func (s *SetupManager) Init() (string, error) {
	token, err := randomHex(16)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = token
	s.expiresAt = time.Now().Add(setupTokenTTL)
	s.used = false

	log.InfoLog.Printf("auth: setup token generated (expires in 1h)")
	return token, nil
}

// GenerateToFile generates a new setup token, writes it to path, and loads it
// into the manager. Called by the print-qr-codes CLI command.
func (s *SetupManager) GenerateToFile(path string) (string, error) {
	token, err := randomHex(16)
	if err != nil {
		return "", err
	}
	rec := setupTokenRecord{
		Token:     token,
		ExpiresAt: time.Now().Add(setupTokenTTL),
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal setup token: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return "", fmt.Errorf("write setup token file: %w", err)
	}
	s.loadRecord(rec)
	return token, nil
}

// LoadFromFile reads a token from path and loads it into the manager.
func (s *SetupManager) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read setup token file: %w", err)
	}
	var rec setupTokenRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return fmt.Errorf("parse setup token file: %w", err)
	}
	if rec.Token == "" {
		return fmt.Errorf("setup token file has empty token")
	}
	s.loadRecord(rec)
	log.InfoLog.Printf("auth: setup token loaded from file (expires %s)", rec.ExpiresAt.Format(time.RFC3339))
	return nil
}

func (s *SetupManager) loadRecord(rec setupTokenRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = rec.Token
	s.expiresAt = rec.ExpiresAt
	s.used = false
}

// WatchFile watches path for writes and reloads the token on each change.
// Blocks until ctx is cancelled; intended to be run in a goroutine.
func (s *SetupManager) WatchFile(ctx context.Context, path string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.WarningLog.Printf("auth: setup token watcher: %v", err)
		return
	}
	defer watcher.Close()

	// Watch the directory so we catch file creation events (file may not exist yet).
	dir := dirOf(path)
	if err := watcher.Add(dir); err != nil {
		log.WarningLog.Printf("auth: setup token watcher add dir: %v", err)
		return
	}

	// Also add the file itself if it already exists.
	if _, statErr := os.Stat(path); statErr == nil {
		_ = watcher.Add(path)
		// Load any token that's already on disk.
		if loadErr := s.LoadFromFile(path); loadErr != nil {
			log.WarningLog.Printf("auth: initial setup token load: %v", loadErr)
		}
	}

	log.InfoLog.Printf("auth: watching %s for new setup tokens", path)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Name != path {
				continue
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				// Re-add the file on create so we catch future writes.
				_ = watcher.Add(path)
				if loadErr := s.LoadFromFile(path); loadErr != nil {
					log.WarningLog.Printf("auth: reload setup token: %v", loadErr)
				}
			}
			if event.Has(fsnotify.Rename) {
				// Atomic writes (write-to-tmp then rename) remove the inode we
				// were watching. The directory watch will fire a Create event
				// when the new file lands, but we also re-add here in case the
				// file is already present by the time we process the event.
				_ = watcher.Add(path)
				if loadErr := s.LoadFromFile(path); loadErr == nil {
					// File was already in place.
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.WarningLog.Printf("auth: setup token watcher error: %v", err)
		}
	}
}

// IsValid checks whether the candidate token is valid without consuming it.
func (s *SetupManager) IsValid(candidate string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.token == "" || s.used {
		return false
	}
	if time.Now().After(s.expiresAt) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(s.token), []byte(candidate)) == 1
}

// Consume marks the setup token as used. Call after the full ceremony completes.
func (s *SetupManager) Consume(candidate string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.token == "" || s.used {
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

// dirOf returns the directory containing path.
func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	return "."
}
