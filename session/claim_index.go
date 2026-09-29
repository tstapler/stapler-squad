package session

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/gofrs/flock"

	"github.com/tstapler/stapler-squad/log"
)

// claimIndexFileName is the JSON file (alongside host_registry.json) that
// durably persists every ClaimRecord this instance knows about.
const claimIndexFileName = "claim_index.json"

// claimIndexLockFileName is the flock coordination file for claimIndexFileName.
const claimIndexLockFileName = "claim_index.lock"

// ClaimRecord is a signed fact "host X claimed external URL Y, and its item
// lives at this deep link". It is a sibling of AdvertisementRecord rather than
// a field on it: see
// project_plans/cross-host-claim-dedup/decisions/ADR-001-separate-claim-gossip-channel.md.
type ClaimRecord struct {
	ExternalURL    string            `json:"external_url"`
	ClaimingHostID HostID            `json:"claiming_host_id"`
	ItemDeepLink   string            `json:"item_deep_link"`
	ClaimedAt      time.Time         `json:"claimed_at"`
	PublicKey      ed25519.PublicKey `json:"public_key"`
	Signature      []byte            `json:"signature"`

	// Disputed is this host's own observation that two different hosts claimed
	// ExternalURL. It is local metadata, not a signed fact: it is excluded from
	// signingPayload and ignored on records received from peers.
	Disputed bool `json:"disputed,omitempty"`
}

// signingPayload is the canonical byte form of the signed fields. PublicKey
// and Signature are excluded (see AdvertisementRecord.signingPayload), as is
// the local-only Disputed flag.
func (r ClaimRecord) signingPayload() []byte {
	payload := struct {
		ExternalURL    string    `json:"external_url"`
		ClaimingHostID HostID    `json:"claiming_host_id"`
		ItemDeepLink   string    `json:"item_deep_link"`
		ClaimedAt      time.Time `json:"claimed_at"`
	}{r.ExternalURL, r.ClaimingHostID, r.ItemDeepLink, r.ClaimedAt}
	// Strings, a HostID and a time.Time can never fail to marshal.
	data, _ := json.Marshal(payload)
	return data
}

// Sign fills in PublicKey and Signature from identity. Callers must set the
// signed fields first.
func (r *ClaimRecord) Sign(identity HostIdentity) {
	r.PublicKey = identity.PublicKey
	r.Signature = identity.Sign(r.signingPayload())
}

// Verify reports whether Signature is valid over signingPayload under
// PublicKey. This proves internal consistency only; it does not prove the
// signer is ClaimingHostID. ClaimIndex.RecordClaim adds that TOFU pin check.
func (r ClaimRecord) Verify() bool {
	return VerifyAdvertisement(r.PublicKey, r.signingPayload(), r.Signature)
}

// NewSignedClaimRecord builds a ClaimRecord for identity and signs it.
func NewSignedClaimRecord(identity HostIdentity, externalURL, itemDeepLink string, claimedAt time.Time) ClaimRecord {
	record := ClaimRecord{
		ExternalURL:    externalURL,
		ClaimingHostID: identity.ID,
		ItemDeepLink:   itemDeepLink,
		ClaimedAt:      claimedAt,
	}
	record.Sign(identity)
	return record
}

// ClaimOutcome describes what RecordClaim did with a record.
type ClaimOutcome struct {
	// Accepted is false when the record was silently rejected (invalid
	// signature, failed TOFU pin check, or missing fields). Rejected records
	// are never stored or re-gossiped.
	Accepted bool
	// IsNew is true when the stored record for the URL changed as a result, so
	// callers know to re-gossip it. A duplicate delivery is not new, which
	// bounds gossip to one hop per fact.
	IsNew bool
	// Conflict is true when a different host already held the URL.
	Conflict bool
}

type claimIndexFile struct {
	Entries []ClaimRecord `json:"entries"`
}

// ClaimIndex is the local, durable store of ClaimRecords keyed by ExternalURL.
// It is persisted like HostRegistry (flock + mutex, write-tmp-then-rename) and
// re-read from disk under the lock so two processes sharing a stateDir stay
// consistent.
type ClaimIndex struct {
	stateDir     string
	hostRegistry *HostRegistry
	lockFile     *flock.Flock

	mu      sync.Mutex
	entries map[string]ClaimRecord // keyed by ExternalURL
}

// NewClaimIndex opens (loading any persisted claims) the ClaimIndex rooted at
// stateDir. hostRegistry supplies the TOFU-pinned public keys RecordClaim
// checks incoming records against, and must not be nil.
func NewClaimIndex(stateDir string, hostRegistry *HostRegistry) (*ClaimIndex, error) {
	if hostRegistry == nil {
		return nil, fmt.Errorf("claim index requires a host registry for TOFU key pinning")
	}
	c := &ClaimIndex{
		stateDir:     stateDir,
		hostRegistry: hostRegistry,
		lockFile:     flock.New(filepath.Join(stateDir, claimIndexLockFileName)),
		entries:      make(map[string]ClaimRecord),
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.withLock(false, c.reloadLocked); err != nil {
		return nil, err
	}
	return c, nil
}

// RecordClaim validates record and merges it into the index.
//
// A record with an invalid signature, or whose PublicKey differs from the key
// HostRegistry pinned for ClaimingHostID, is rejected silently (Accepted ==
// false, nil error), like HostRegistry.Advertise. A host with no pinned key yet
// is accepted on first sight.
//
// When a different host already holds the URL the earlier ClaimedAt wins (ties
// go to the lexically smaller HostID) and the surviving entry is marked
// Disputed for a human to look at.
func (c *ClaimIndex) RecordClaim(record ClaimRecord) (ClaimOutcome, error) {
	if record.ExternalURL == "" || !record.ClaimingHostID.IsValid() || !record.Verify() {
		return ClaimOutcome{}, nil
	}
	if pinned, ok := c.hostRegistry.Lookup(record.ClaimingHostID); ok && !bytes.Equal(pinned.PublicKey, record.PublicKey) {
		log.Warn("claim_index.claim_rejected",
			"host_id", record.ClaimingHostID.String(),
			"external_url", record.ExternalURL,
			"reason", "public_key_mismatch")
		return ClaimOutcome{}, nil
	}
	record.Disputed = false // peers cannot assert a local observation

	c.mu.Lock()
	defer c.mu.Unlock()

	outcome := ClaimOutcome{Accepted: true}
	err := c.withLock(true, func() error {
		if err := c.reloadLocked(); err != nil {
			return err
		}
		existing, had := c.entries[record.ExternalURL]
		next, changed, conflict := mergeClaim(existing, had, record)
		outcome.IsNew = changed && (!had || !sameClaimFact(existing, next))
		outcome.Conflict = conflict
		if conflict {
			log.Warn("claim_index.conflict_detected",
				"host_a", existing.ClaimingHostID.String(),
				"host_b", record.ClaimingHostID.String(),
				"external_url", record.ExternalURL)
		}
		if !changed {
			return nil
		}
		c.entries[record.ExternalURL] = next
		return c.writeLocked()
	})
	if err != nil {
		return ClaimOutcome{}, err
	}
	return outcome, nil
}

// mergeClaim decides the stored entry after incoming meets existing. changed
// reports whether the stored entry differs (including only its Disputed flag).
func mergeClaim(existing ClaimRecord, had bool, incoming ClaimRecord) (next ClaimRecord, changed, conflict bool) {
	if !had {
		return incoming, true, false
	}
	if existing.ClaimingHostID.String() == incoming.ClaimingHostID.String() {
		if incoming.ClaimedAt.Before(existing.ClaimedAt) || incoming.ClaimedAt.Equal(existing.ClaimedAt) {
			return existing, false, false
		}
		incoming.Disputed = existing.Disputed
		return incoming, true, false
	}
	winner := existing
	if incoming.ClaimedAt.Before(existing.ClaimedAt) ||
		(incoming.ClaimedAt.Equal(existing.ClaimedAt) && incoming.ClaimingHostID.String() < existing.ClaimingHostID.String()) {
		winner = incoming
	}
	winner.Disputed = true
	changed = winner.ClaimingHostID.String() != existing.ClaimingHostID.String() || !existing.Disputed
	return winner, changed, true
}

// sameClaimFact reports whether a and b carry the same signed fact (claimant
// and time), ignoring the local-only Disputed flag.
func sameClaimFact(a, b ClaimRecord) bool {
	return a.ClaimingHostID.String() == b.ClaimingHostID.String() && a.ClaimedAt.Equal(b.ClaimedAt)
}

// CheckClaim returns the claim held for externalURL, if any. Disputed is
// returned as stored. It re-reads the file, so a claim recorded by another
// process is visible; a read failure is logged and reported as not found.
func (c *ClaimIndex) CheckClaim(externalURL string) (ClaimRecord, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.withLock(false, c.reloadLocked); err != nil {
		log.Warn("claim_index.read_failed", "err", err)
	}
	record, ok := c.entries[externalURL]
	return record, ok
}

// ForeignClaim returns the claim for externalURL when it is held by a host
// other than self. A claim held by self, or no claim at all, reports false: only
// a foreign claim can make this host's own import or dequeue a duplicate.
func (c *ClaimIndex) ForeignClaim(externalURL string, self HostID) (ClaimRecord, bool) {
	record, ok := c.CheckClaim(externalURL)
	if !ok || record.ClaimingHostID.String() == self.String() {
		return ClaimRecord{}, false
	}
	return record, true
}

// Snapshot returns every stored claim, ordered by ExternalURL for stable output.
func (c *ClaimIndex) Snapshot() []ClaimRecord {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.withLock(false, c.reloadLocked); err != nil {
		log.Warn("claim_index.read_failed", "err", err)
	}
	out := make([]ClaimRecord, 0, len(c.entries))
	for _, record := range c.entries {
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ExternalURL < out[j].ExternalURL })
	return out
}

// ResolveDispute clears the Disputed flag for externalURL once a human has
// looked at it. It is a no-op for an unknown or undisputed URL. Nothing clears
// the flag automatically.
func (c *ClaimIndex) ResolveDispute(externalURL string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.withLock(true, func() error {
		if err := c.reloadLocked(); err != nil {
			return err
		}
		record, ok := c.entries[externalURL]
		if !ok || !record.Disputed {
			return nil
		}
		record.Disputed = false
		c.entries[externalURL] = record
		return c.writeLocked()
	})
}

func (c *ClaimIndex) path() string {
	return filepath.Join(c.stateDir, claimIndexFileName)
}

// withLock runs fn holding the cross-process flock (exclusive when write).
// Callers must hold c.mu.
func (c *ClaimIndex) withLock(write bool, fn func() error) error {
	ctx, cancel := context.WithTimeout(context.Background(), hostRegistryLockTimeout)
	defer cancel()
	var locked bool
	var err error
	if write {
		if mkErr := os.MkdirAll(c.stateDir, 0750); mkErr != nil {
			return fmt.Errorf("failed to create state directory: %w", mkErr)
		}
		locked, err = c.lockFile.TryLockContext(ctx, 100*time.Millisecond)
	} else {
		locked, err = c.lockFile.TryRLockContext(ctx, 100*time.Millisecond)
	}
	if err != nil {
		return fmt.Errorf("failed to acquire claim index lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("could not acquire claim index lock within timeout")
	}
	defer func() { _ = c.lockFile.Unlock() }()
	return fn()
}

// reloadLocked replaces c.entries with the on-disk contents. A missing file
// means an empty index.
func (c *ClaimIndex) reloadLocked() error {
	data, err := os.ReadFile(c.path())
	if err != nil {
		if os.IsNotExist(err) {
			c.entries = make(map[string]ClaimRecord)
			return nil
		}
		return fmt.Errorf("failed to read claim index file: %w", err)
	}
	var file claimIndexFile
	if err := json.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("claim index file %s is corrupted: %w", c.path(), err)
	}
	c.entries = make(map[string]ClaimRecord, len(file.Entries))
	for _, record := range file.Entries {
		c.entries[record.ExternalURL] = record
	}
	return nil
}

// writeLocked atomically persists c.entries. Callers must hold c.mu and the
// write flock.
func (c *ClaimIndex) writeLocked() error {
	file := claimIndexFile{Entries: make([]ClaimRecord, 0, len(c.entries))}
	for _, record := range c.entries {
		file.Entries = append(file.Entries, record)
	}
	sort.Slice(file.Entries, func(i, j int) bool { return file.Entries[i].ExternalURL < file.Entries[j].ExternalURL })
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal claim index: %w", err)
	}
	tmpPath := c.path() + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write temporary claim index file: %w", err)
	}
	if err := os.Rename(tmpPath, c.path()); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to atomically write claim index file: %w", err)
	}
	return nil
}
