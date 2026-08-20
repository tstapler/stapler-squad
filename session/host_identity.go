package session

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"
	"github.com/oklog/ulid/v2"

	"github.com/tstapler/stapler-squad/log"
)

// HostIDPrefix is prepended to every generated HostIdentity ID (e.g.
// "host_01J..."), mirroring BacklogItemIDPrefix's "bl_" convention so a
// HostID is recognizable at a glance and distinguishable from other
// prefixed IDs in the system.
const HostIDPrefix = "host_"

// hostIdentityFileName is the JSON file (alongside config.StateFileName and
// config.InstancesFileName) that durably persists this instance's
// HostIdentity. Per ADR-002, this identity and its Ed25519 keypair are
// minted once on first run and are immutable for the life of the install --
// there is deliberately no "refresh" or "rotate" path here.
const hostIdentityFileName = "host_identity.json"

// hostIdentityLockFileName is the flock coordination file for
// hostIdentityFileName, mirroring config.LockFileName.
const hostIdentityLockFileName = "host_identity.lock"

// hostIdentityLockTimeout mirrors config.DefaultLockTimeout.
const hostIdentityLockTimeout = 5 * time.Second

// hostIDMu and hostIDEntropy are the shared, mutex-guarded monotonic entropy
// source for HostID generation. Mirrors backlogItemIDMu/backlogItemIDEntropy
// in session/backlog_item_id.go -- see that file's doc comment for why
// ulid.Monotonic requires external synchronization.
var (
	hostIDMu      sync.Mutex
	hostIDEntropy = ulid.Monotonic(rand.Reader, 0)
)

// HostID is a type-safe wrapper around a ULID used as the durable public
// identifier for a stapler-squad instance ("host_<26-char ULID>"). The zero
// value is not a valid ID -- construct one via NewHostID or ParseHostID.
type HostID struct {
	value ulid.ULID
	valid bool
}

// NewHostID generates a new HostID from the current time and the shared
// monotonic entropy source. Safe for concurrent use from any number of
// goroutines.
func NewHostID() (HostID, error) {
	hostIDMu.Lock()
	defer hostIDMu.Unlock()

	id, err := ulid.New(ulid.Timestamp(time.Now()), hostIDEntropy)
	if err != nil {
		return HostID{}, fmt.Errorf("failed to generate host id: %w", err)
	}
	return HostID{value: id, valid: true}, nil
}

// ParseHostID parses a string of the form "host_<ULID>" into a HostID,
// returning a descriptive error if the prefix is missing or the remainder
// is not a well-formed ULID.
func ParseHostID(s string) (HostID, error) {
	if !strings.HasPrefix(s, HostIDPrefix) {
		return HostID{}, fmt.Errorf("host id %q: missing required %q prefix", s, HostIDPrefix)
	}
	rest := strings.TrimPrefix(s, HostIDPrefix)
	parsed, err := ulid.ParseStrict(rest)
	if err != nil {
		return HostID{}, fmt.Errorf("host id %q: invalid ULID after %q prefix: %w", s, HostIDPrefix, err)
	}
	return HostID{value: parsed, valid: true}, nil
}

// IsHostIDShape reports whether s looks like a HostID (i.e. starts with
// HostIDPrefix) without fully validating the ULID portion.
func IsHostIDShape(s string) bool {
	return strings.HasPrefix(s, HostIDPrefix)
}

// String returns the prefixed string form, e.g. "host_01HXYZABCDEFGHJKMNPQRSTVWX".
// Returns "" for the zero value.
func (id HostID) String() string {
	if !id.valid {
		return ""
	}
	return HostIDPrefix + id.value.String()
}

// IsValid reports whether id was constructed via NewHostID or ParseHostID.
// The zero value is not valid.
func (id HostID) IsValid() bool { return id.valid }

// MarshalJSON encodes id as its prefixed string form (or "" for the zero
// value), so HostID can be embedded directly in persisted/wire structs
// without a separate string field.
func (id HostID) MarshalJSON() ([]byte, error) {
	return json.Marshal(id.String())
}

// UnmarshalJSON decodes id from its prefixed string form. An empty string
// decodes to the zero value (not valid); anything else must parse via
// ParseHostID.
func (id *HostID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	if s == "" {
		*id = HostID{}
		return nil
	}
	parsed, err := ParseHostID(s)
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}

// HostIdentity is the durable identity a stapler-squad instance presents to
// other hosts. It is minted once per install (LoadOrCreateHostIdentity) and
// persisted to host_identity.json; the private key never leaves the
// instance that generated it. See ADR-002 for the full design rationale.
type HostIdentity struct {
	ID         HostID             `json:"id"`
	PublicKey  ed25519.PublicKey  `json:"public_key"`
	PrivateKey ed25519.PrivateKey `json:"private_key"`
}

// Sign produces an Ed25519 signature over payload using this identity's
// private key. Callers use this to sign an advertisement record's
// canonical byte representation (see AdvertisementRecord.signingPayload).
func (h HostIdentity) Sign(payload []byte) []byte {
	return ed25519.Sign(h.PrivateKey, payload)
}

// VerifyAdvertisement reports whether sig is a valid Ed25519 signature over
// payload under pubKey. Returns false (never panics) for a malformed or
// wrong-length public key, so callers can treat any failure uniformly as
// "reject this advertisement."
func VerifyAdvertisement(pubKey ed25519.PublicKey, payload, sig []byte) bool {
	if len(pubKey) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(pubKey, payload, sig)
}

// hostIdentityMu provides real intra-process mutual exclusion around
// host_identity.json, in addition to (not instead of) the flock below --
// mirrors SuspendedProcessStore's mu, since a single *flock.Flock value
// gives no protection against two goroutines in this process racing on the
// same file.
var hostIdentityMu sync.Mutex

// LoadOrCreateHostIdentity loads the persisted HostIdentity from
// <stateDir>/host_identity.json, minting and persisting a new one (a fresh
// HostID plus an Ed25519 keypair) if the file does not yet exist. The
// returned identity is stable across restarts: once minted, the same
// HostIdentity is returned on every subsequent call against the same
// stateDir, per ADR-002's "immutable for the life of the install."
func LoadOrCreateHostIdentity(stateDir string) (HostIdentity, error) {
	hostIdentityMu.Lock()
	defer hostIdentityMu.Unlock()

	lockPath := filepath.Join(stateDir, hostIdentityLockFileName)
	lockFile := flock.New(lockPath)

	ctx, cancel := context.WithTimeout(context.Background(), hostIdentityLockTimeout)
	defer cancel()
	locked, err := lockFile.TryLockContext(ctx, 100*time.Millisecond)
	if err != nil {
		return HostIdentity{}, fmt.Errorf("failed to acquire host identity lock: %w", err)
	}
	if !locked {
		return HostIdentity{}, fmt.Errorf("could not acquire host identity lock within timeout")
	}
	defer func() { _ = lockFile.Unlock() }()

	path := filepath.Join(stateDir, hostIdentityFileName)
	data, err := os.ReadFile(path)
	if err == nil {
		var identity HostIdentity
		if unmarshalErr := json.Unmarshal(data, &identity); unmarshalErr != nil {
			return HostIdentity{}, fmt.Errorf("host identity file %s is corrupted: %w", path, unmarshalErr)
		}
		if !identity.ID.IsValid() || len(identity.PublicKey) != ed25519.PublicKeySize || len(identity.PrivateKey) != ed25519.PrivateKeySize {
			return HostIdentity{}, fmt.Errorf("host identity file %s is corrupted: incomplete identity", path)
		}
		return identity, nil
	}
	if !os.IsNotExist(err) {
		return HostIdentity{}, fmt.Errorf("failed to read host identity file: %w", err)
	}

	id, err := NewHostID()
	if err != nil {
		return HostIdentity{}, fmt.Errorf("failed to mint host id: %w", err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return HostIdentity{}, fmt.Errorf("failed to generate host identity keypair: %w", err)
	}
	identity := HostIdentity{ID: id, PublicKey: pub, PrivateKey: priv}

	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return HostIdentity{}, fmt.Errorf("failed to create state directory: %w", err)
	}
	out, err := json.MarshalIndent(identity, "", "  ")
	if err != nil {
		return HostIdentity{}, fmt.Errorf("failed to marshal host identity: %w", err)
	}
	tmpPath := path + ".tmp"
	// 0600: this file holds the instance's Ed25519 private key.
	if err := os.WriteFile(tmpPath, out, 0600); err != nil {
		return HostIdentity{}, fmt.Errorf("failed to write temporary host identity file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return HostIdentity{}, fmt.Errorf("failed to atomically write host identity file: %w", err)
	}
	log.Info("host_identity.generated", "host_id", identity.ID.String(), "state_dir", stateDir)
	return identity, nil
}
