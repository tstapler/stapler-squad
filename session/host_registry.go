package session

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"

	"github.com/tstapler/stapler-squad/log"
)

// hostRegistryFileName is the JSON file (alongside host_identity.json) that
// durably persists every peer HostIdentity this instance has learned about
// via advertisement or re-gossip.
const hostRegistryFileName = "host_registry.json"

// hostRegistryLockFileName is the flock coordination file for
// hostRegistryFileName, mirroring hostIdentityLockFileName.
const hostRegistryLockFileName = "host_registry.lock"

// hostRegistryLockTimeout mirrors hostIdentityLockTimeout.
const hostRegistryLockTimeout = 5 * time.Second

// DefaultHostAdvertisementInterval is how often HostAdvertiser (see
// host_advertiser.go) re-broadcasts this instance's own advertisement to
// every known peer. Defined here (rather than in host_advertiser.go)
// because DefaultHostRegistryTTL is expressed as a multiple of it.
const DefaultHostAdvertisementInterval = 5 * time.Minute

// DefaultHostRegistryTTL is how long a registry entry survives without a
// refreshing advertisement before Prune removes it. Per ADR-002: "an entry
// not re-advertised within N missed cycles is dropped, not immediately, to
// tolerate transient network blips" -- this is several multiples of
// DefaultHostAdvertisementInterval, not one.
const DefaultHostRegistryTTL = 3 * DefaultHostAdvertisementInterval

// Clock abstracts time.Now for injectable-fake-clock testing (TTL/prune
// logic must not depend on wall-clock sleep, per
// .claude/rules/fix-flaky-tests-dont-defer.md). Defined locally rather than
// reusing executor.Clock to avoid a session -> executor dependency for what
// is otherwise just time.Now(); mirrors executor/circuit_breaker.go's Clock
// interface shape.
type Clock interface {
	Now() time.Time
}

// realClock is the default Clock implementation using time.Now.
type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// RegistryEntry is one peer's current state in the local Workspace Host
// Registry: the addresses it last advertised, the public key its identity
// is pinned to (TOFU), and when it was first/last seen.
type RegistryEntry struct {
	HostID            HostID            `json:"host_id"`
	AdvertisedAddress []string          `json:"advertised_address"`
	PublicKey         ed25519.PublicKey `json:"public_key"`
	AdvertisedAt      time.Time         `json:"advertised_at"`
	LastSeenAt        time.Time         `json:"last_seen_at"`
}

// AdvertisementRecord is the wire format a host broadcasts to advertise its
// own identity and reachable address(es) to a peer, per ADR-002.
type AdvertisementRecord struct {
	HostIdentity      HostID            `json:"host_identity"`
	AdvertisedAddress []string          `json:"advertised_address"`
	AdvertisedAt      time.Time         `json:"advertised_at"`
	PublicKey         ed25519.PublicKey `json:"public_key"`
	Signature         []byte            `json:"signature"`
}

// signingPayload returns the canonical byte representation of the fields
// that are actually signed -- HostIdentity, AdvertisedAddress, and
// AdvertisedAt. PublicKey travels alongside the signature (it's what the
// signature is verified against, not part of what's signed) and Signature
// is of course excluded from what it signs.
func (r AdvertisementRecord) signingPayload() []byte {
	payload := struct {
		HostIdentity      HostID    `json:"host_identity"`
		AdvertisedAddress []string  `json:"advertised_address"`
		AdvertisedAt      time.Time `json:"advertised_at"`
	}{r.HostIdentity, r.AdvertisedAddress, r.AdvertisedAt}
	// A struct of only a HostID (custom string-shaped MarshalJSON), a
	// []string, and a time.Time can never fail to marshal.
	data, _ := json.Marshal(payload)
	return data
}

// Sign fills in PublicKey and Signature from identity, over this record's
// signingPayload(). Callers must set HostIdentity, AdvertisedAddress, and
// AdvertisedAt first.
func (r *AdvertisementRecord) Sign(identity HostIdentity) {
	r.PublicKey = identity.PublicKey
	r.Signature = identity.Sign(r.signingPayload())
}

// Verify reports whether Signature is a valid Ed25519 signature over this
// record's signingPayload() under PublicKey.
func (r AdvertisementRecord) Verify() bool {
	return VerifyAdvertisement(r.PublicKey, r.signingPayload(), r.Signature)
}

// BuildAdvertisement constructs and signs this instance's own
// AdvertisementRecord, for the advertisement HTTP handler's response and the
// client-side broadcast loop (session/host_advertiser.go) to send to peers --
// centralizes the field-then-Sign boilerplate in one place.
func BuildAdvertisement(identity HostIdentity, addresses []string, at time.Time) AdvertisementRecord {
	record := AdvertisementRecord{
		HostIdentity:      identity.ID,
		AdvertisedAddress: addresses,
		AdvertisedAt:      at,
	}
	record.Sign(identity)
	return record
}

// hostRegistryFile is the on-disk shape of host_registry.json.
type hostRegistryFile struct {
	Entries []RegistryEntry `json:"entries"`
}

// HostRegistry is each instance's local Workspace Host Registry: a
// TOFU-pinned, TTL-pruned table of every peer HostIdentity this instance has
// learned about via direct or re-gossiped advertisement. Persisted the same
// flock+mutex-guarded way as HostIdentity.
type HostRegistry struct {
	stateDir string
	ttl      time.Duration
	clock    Clock
	lockFile *flock.Flock

	// mu provides real intra-process mutual exclusion, in addition to (not
	// instead of) the flock -- mirrors SuspendedProcessStore's mu.
	mu      sync.Mutex
	entries map[string]RegistryEntry // keyed by HostID.String()
}

// NewHostRegistry creates a HostRegistry rooted at stateDir, using the real
// wall clock and ttl for pruning. Existing persisted entries (if any) are
// loaded immediately.
func NewHostRegistry(stateDir string, ttl time.Duration) (*HostRegistry, error) {
	return NewHostRegistryWithClock(stateDir, ttl, realClock{})
}

// NewHostRegistryWithClock is like NewHostRegistry but with an injectable
// Clock, for deterministic TTL/prune testing without wall-clock sleep.
func NewHostRegistryWithClock(stateDir string, ttl time.Duration, clock Clock) (*HostRegistry, error) {
	r := &HostRegistry{
		stateDir: stateDir,
		ttl:      ttl,
		clock:    clock,
		lockFile: flock.New(filepath.Join(stateDir, hostRegistryLockFileName)),
		entries:  make(map[string]RegistryEntry),
	}
	if err := r.withReadLock(func() error {
		file, err := r.readWithoutLocking()
		if err != nil {
			return err
		}
		for _, entry := range file.Entries {
			r.entries[entry.HostID.String()] = entry
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return r, nil
}

// Advertise validates and upserts record into the registry.
//
// isNew reports whether HostIdentity had no prior entry -- callers (the
// advertisement HTTP handler) use this to decide whether to re-gossip the
// record to other known peers, bounding propagation to once per new fact
// per node.
//
// accepted reports whether record passed verification: first-seen
// identities are accepted per TOFU (and their PublicKey pinned for every
// future advertisement claiming that HostIdentity); a later advertisement
// is accepted only if its PublicKey matches the pinned key AND its
// Signature verifies. A rejected record is not an error -- it's the
// expected outcome for a misbehaving or buggy peer -- and is not upserted.
func (r *HostRegistry) Advertise(record AdvertisementRecord) (isNew bool, accepted bool, err error) {
	if !record.HostIdentity.IsValid() || len(record.AdvertisedAddress) == 0 {
		return false, false, fmt.Errorf("advertisement record missing host identity or advertised address")
	}
	for _, addr := range record.AdvertisedAddress {
		if !isPlausiblePeerAddress(addr) {
			log.Warn("host_registry.advertisement_rejected",
				"host_id", record.HostIdentity.String(),
				"reason", "implausible_advertised_address",
				"address", addr)
			return false, false, nil
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := record.HostIdentity.String()
	existing, hadEntry := r.entries[key]

	if hadEntry && !bytes.Equal(existing.PublicKey, record.PublicKey) {
		// TOFU pin mismatch: this identity was first seen under a different
		// public key. Reject -- a peer cannot impersonate a HostIdentity it
		// doesn't hold the private key for.
		return false, false, nil
	}
	if !record.Verify() {
		return false, false, nil
	}

	now := r.clock.Now()
	r.entries[key] = RegistryEntry{
		HostID:            record.HostIdentity,
		AdvertisedAddress: record.AdvertisedAddress,
		PublicKey:         record.PublicKey,
		AdvertisedAt:      record.AdvertisedAt,
		LastSeenAt:        now,
	}
	if err := r.persistLocked(); err != nil {
		return false, false, err
	}
	return !hadEntry, true, nil
}

// isPlausiblePeerAddress rejects an advertised address that names a
// loopback, link-local, or unspecified endpoint (e.g. "127.0.0.1:8543",
// "169.254.169.254:80", "localhost:1", "[::1]:80") -- the addresses a
// misbehaving or compromised peer would advertise to redirect this
// instance's own outbound liveness check (registryHostResolver.checkLiveness
// in server/services/deep_link_resolver.go) at itself or at a cloud
// metadata endpoint reachable only from this instance's network position.
// A TOFU-pinned identity is still trusted to say who it is; this only
// bounds where "who it is" is allowed to claim to be reachable at. Address
// content is otherwise unrestricted -- see ADR-002's accepted same-LAN
// threat model -- this blocks the specific classes of address that turn a
// trust decision about identity into an SSRF primitive.
func isPlausiblePeerAddress(addr string) bool {
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	if strings.EqualFold(host, "localhost") {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil {
		// Not an IP literal (a real DNS hostname) -- can't be resolved
		// safely here without risking a DNS-rebinding TOCTOU against
		// whatever later dials this address, so it's allowed through
		// unchanged; the bounded-timeout liveness check remains the
		// backstop against a genuinely unreachable/malicious target.
		return true
	}
	return !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsUnspecified()
}

// Prune removes every entry whose LastSeenAt is more than the registry's
// ttl behind the current clock time -- an entry survives several missed
// advertisement cycles (per ADR-002) rather than being dropped after just
// one, to tolerate transient network blips.
func (r *HostRegistry) Prune() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	cutoff := r.clock.Now().Add(-r.ttl)
	changed := false
	for key, entry := range r.entries {
		if entry.LastSeenAt.Before(cutoff) {
			log.Info("host_registry.entry_expired",
				"host_id", entry.HostID.String(),
				"last_seen_at", entry.LastSeenAt,
				"ttl", r.ttl)
			delete(r.entries, key)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return r.persistLocked()
}

// Lookup returns the RegistryEntry for id, if known.
func (r *HostRegistry) Lookup(id HostID) (RegistryEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.entries[id.String()]
	return entry, ok
}

// LookupByHostname returns the RegistryEntry whose AdvertisedAddress
// contains a "host:port" (or bare host) entry whose host part matches
// hostname, per ADR-002's "resolution looks up hostname (matched against a
// host's advertised addresses...) in this local registry."
func (r *HostRegistry) LookupByHostname(hostname string) (HostID, RegistryEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, entry := range r.entries {
		for _, addr := range entry.AdvertisedAddress {
			h := addr
			if host, _, err := net.SplitHostPort(addr); err == nil {
				h = host
			}
			if h == hostname {
				return entry.HostID, entry, true
			}
		}
	}
	return HostID{}, RegistryEntry{}, false
}

// Snapshot returns every currently-registered peer entry. Used by the
// advertiser's broadcast loop, the re-gossip fan-out, and the
// --list-known-hosts CLI command.
func (r *HostRegistry) Snapshot() []RegistryEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]RegistryEntry, 0, len(r.entries))
	for _, entry := range r.entries {
		out = append(out, entry)
	}
	return out
}

func (r *HostRegistry) path() string {
	return filepath.Join(r.stateDir, hostRegistryFileName)
}

func (r *HostRegistry) withReadLock(fn func() error) error {
	ctx, cancel := context.WithTimeout(context.Background(), hostRegistryLockTimeout)
	defer cancel()
	locked, err := r.lockFile.TryRLockContext(ctx, 100*time.Millisecond)
	if err != nil {
		return fmt.Errorf("failed to acquire host registry read lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("could not acquire host registry read lock within timeout")
	}
	defer func() { _ = r.lockFile.Unlock() }()
	return fn()
}

func (r *HostRegistry) withWriteLock(fn func() error) error {
	ctx, cancel := context.WithTimeout(context.Background(), hostRegistryLockTimeout)
	defer cancel()
	locked, err := r.lockFile.TryLockContext(ctx, 100*time.Millisecond)
	if err != nil {
		return fmt.Errorf("failed to acquire host registry write lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("could not acquire host registry write lock within timeout")
	}
	defer func() { _ = r.lockFile.Unlock() }()
	return fn()
}

func (r *HostRegistry) readWithoutLocking() (hostRegistryFile, error) {
	data, err := os.ReadFile(r.path())
	if err != nil {
		if os.IsNotExist(err) {
			return hostRegistryFile{}, nil
		}
		return hostRegistryFile{}, fmt.Errorf("failed to read host registry file: %w", err)
	}
	var file hostRegistryFile
	if err := json.Unmarshal(data, &file); err != nil {
		return hostRegistryFile{}, fmt.Errorf("host registry file %s is corrupted: %w", r.path(), err)
	}
	return file, nil
}

// persistLocked writes the current in-memory entries to disk. Callers must
// hold r.mu. Acquires the flock (cross-process) internally.
func (r *HostRegistry) persistLocked() error {
	return r.withWriteLock(func() error {
		file := hostRegistryFile{Entries: make([]RegistryEntry, 0, len(r.entries))}
		for _, entry := range r.entries {
			file.Entries = append(file.Entries, entry)
		}
		if err := os.MkdirAll(r.stateDir, 0755); err != nil {
			return fmt.Errorf("failed to create state directory: %w", err)
		}
		data, err := json.MarshalIndent(file, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal host registry: %w", err)
		}
		tmpPath := r.path() + ".tmp"
		if err := os.WriteFile(tmpPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write temporary host registry file: %w", err)
		}
		if err := os.Rename(tmpPath, r.path()); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("failed to atomically write host registry file: %w", err)
		}
		return nil
	})
}
