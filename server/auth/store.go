package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"claude-squad/config"
	"claude-squad/log"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/gofrs/flock"
)

const credentialFileName = "passkeys.json"

// CredentialStore persists WebAuthn credentials to disk as JSON.
// Thread-safe via a mutex for in-process coordination and a file lock for
// multi-process safety (though claude-squad runs as a single process).
type CredentialStore struct {
	mu       sync.RWMutex
	filePath string
	data     credentialData
}

type credentialData struct {
	Credentials []storedCredential `json:"credentials"`
}

// storedCredential is a JSON-serialisable wrapper around webauthn.Credential.
type storedCredential struct {
	ID              []byte                `json:"id"`
	PublicKey       []byte                `json:"public_key"`
	AttestationType string                `json:"attestation_type"`
	Authenticator   webauthn.Authenticator `json:"authenticator"`
}

// NewCredentialStore creates or loads the credential store from the workspace
// config directory.
func NewCredentialStore() (*CredentialStore, error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("get config dir: %w", err)
	}

	filePath := filepath.Join(configDir, credentialFileName)
	cs := &CredentialStore{filePath: filePath}

	if err := cs.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load credential store: %w", err)
	}

	return cs, nil
}

// HasCredentials reports whether any passkeys are registered.
func (cs *CredentialStore) HasCredentials() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return len(cs.data.Credentials) > 0
}

// GetCredentials returns a copy of all stored credentials.
func (cs *CredentialStore) GetCredentials() []webauthn.Credential {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	creds := make([]webauthn.Credential, 0, len(cs.data.Credentials))
	for _, sc := range cs.data.Credentials {
		creds = append(creds, webauthn.Credential{
			ID:              sc.ID,
			PublicKey:       sc.PublicKey,
			AttestationType: sc.AttestationType,
			Authenticator:   sc.Authenticator,
		})
	}
	return creds
}

// AddCredential persists a new credential atomically.
func (cs *CredentialStore) AddCredential(cred webauthn.Credential) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	cs.data.Credentials = append(cs.data.Credentials, storedCredential{
		ID:              cred.ID,
		PublicKey:       cred.PublicKey,
		AttestationType: cred.AttestationType,
		Authenticator:   cred.Authenticator,
	})

	return cs.save()
}

// UpdateCredential updates the sign count of an existing credential.
func (cs *CredentialStore) UpdateCredential(cred webauthn.Credential) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	for i, sc := range cs.data.Credentials {
		if bytesEqual(sc.ID, cred.ID) {
			cs.data.Credentials[i].Authenticator = cred.Authenticator
			return cs.save()
		}
	}
	return fmt.Errorf("credential %x not found", cred.ID)
}

// RemoveCredential removes a credential by ID.
func (cs *CredentialStore) RemoveCredential(credID []byte) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	for i, sc := range cs.data.Credentials {
		if bytesEqual(sc.ID, credID) {
			cs.data.Credentials = append(cs.data.Credentials[:i], cs.data.Credentials[i+1:]...)
			return cs.save()
		}
	}
	return fmt.Errorf("credential %x not found", credID)
}

// CredentialCount returns the number of registered passkeys.
func (cs *CredentialStore) CredentialCount() int {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return len(cs.data.Credentials)
}

func (cs *CredentialStore) load() error {
	lock := flock.New(cs.filePath + ".lock")
	if err := lock.RLock(); err != nil {
		return fmt.Errorf("acquire read lock: %w", err)
	}
	defer lock.Unlock() //nolint:errcheck

	data, err := os.ReadFile(cs.filePath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &cs.data)
}

func (cs *CredentialStore) save() error {
	if err := os.MkdirAll(filepath.Dir(cs.filePath), 0700); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	lock := flock.New(cs.filePath + ".lock")
	if err := lock.Lock(); err != nil {
		return fmt.Errorf("acquire write lock: %w", err)
	}
	defer lock.Unlock() //nolint:errcheck

	data, err := json.MarshalIndent(cs.data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}

	tmp := cs.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := os.Rename(tmp, cs.filePath); err != nil {
		return fmt.Errorf("rename to final path: %w", err)
	}

	log.InfoLog.Printf("auth: saved %d passkey credential(s)", len(cs.data.Credentials))
	return nil
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
