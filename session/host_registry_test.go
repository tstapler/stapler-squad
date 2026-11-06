package session

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	ssqlog "github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// fakeClock is a controllable Clock for deterministic TTL/prune tests --
// never wall-clock time.Sleep, per the `fix-flaky-tests-dont-defer` skill.
type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func newTestAdvertisement(t *testing.T, identity HostIdentity, addrs []string, at time.Time) AdvertisementRecord {
	t.Helper()
	record := AdvertisementRecord{
		HostIdentity:      identity.ID,
		AdvertisedAddress: addrs,
		AdvertisedAt:      at,
	}
	record.Sign(identity)
	return record
}

func TestHostRegistry_Advertise_should_UpsertEntryAndRefreshLastSeenAt_When_AdvertisementReceived(t *testing.T) {
	stateDir := t.TempDir()
	clock := &fakeClock{now: time.Now()}
	registry, err := NewHostRegistryWithClock(stateDir, DefaultHostRegistryTTL, clock)
	if err != nil {
		t.Fatalf("NewHostRegistryWithClock() error = %v, want nil", err)
	}

	identity, err := LoadOrCreateHostIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity() error = %v, want nil", err)
	}

	first := newTestAdvertisement(t, identity, []string{"peer-a:8444"}, clock.Now())
	isNew, accepted, err := registry.Advertise(first)
	if err != nil {
		t.Fatalf("Advertise() error = %v, want nil", err)
	}
	if !isNew || !accepted {
		t.Fatalf("Advertise() = (isNew=%v, accepted=%v), want (true, true) for first advertisement", isNew, accepted)
	}

	entry, ok := registry.Lookup(identity.ID)
	if !ok {
		t.Fatalf("Lookup() after first Advertise() = not found, want found")
	}
	firstSeenAt := entry.LastSeenAt

	clock.Advance(time.Minute)
	repeat := newTestAdvertisement(t, identity, []string{"peer-a:8444"}, clock.Now())
	isNew, accepted, err = registry.Advertise(repeat)
	if err != nil {
		t.Fatalf("second Advertise() error = %v, want nil", err)
	}
	if isNew {
		t.Fatalf("second Advertise() isNew = true, want false for a repeat advertisement of a known identity")
	}
	if !accepted {
		t.Fatalf("second Advertise() accepted = false, want true for a validly re-signed repeat advertisement")
	}

	entry, ok = registry.Lookup(identity.ID)
	if !ok {
		t.Fatalf("Lookup() after second Advertise() = not found, want found")
	}
	if !entry.LastSeenAt.After(firstSeenAt) {
		t.Fatalf("LastSeenAt not refreshed: first = %v, second = %v", firstSeenAt, entry.LastSeenAt)
	}
}

func TestHostRegistry_Prune_should_RemoveEntry_When_RegistryTTLCyclesElapseWithNoRefresh(t *testing.T) {
	stateDir := t.TempDir()
	ttl := 3 * time.Minute
	clock := &fakeClock{now: time.Now()}
	registry, err := NewHostRegistryWithClock(stateDir, ttl, clock)
	if err != nil {
		t.Fatalf("NewHostRegistryWithClock() error = %v, want nil", err)
	}

	identity, err := LoadOrCreateHostIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity() error = %v, want nil", err)
	}

	record := newTestAdvertisement(t, identity, []string{"peer-a:8444"}, clock.Now())
	if _, _, err := registry.Advertise(record); err != nil {
		t.Fatalf("Advertise() error = %v, want nil", err)
	}

	// Advance past the TTL with no refreshing advertisement.
	clock.Advance(ttl + time.Second)
	if err := registry.Prune(); err != nil {
		t.Fatalf("Prune() error = %v, want nil", err)
	}

	if _, ok := registry.Lookup(identity.ID); ok {
		t.Fatalf("Lookup() after Prune() past TTL = found, want not found (stale entry should be pruned)")
	}
}

func TestHostRegistry_Prune_should_LogHostRegistryEntryExpired_When_EntryDroppedPastTTL(t *testing.T) {
	stateDir := t.TempDir()
	ttl := 3 * time.Minute
	clock := &fakeClock{now: time.Now()}
	registry, err := NewHostRegistryWithClock(stateDir, ttl, clock)
	if err != nil {
		t.Fatalf("NewHostRegistryWithClock() error = %v, want nil", err)
	}

	identity, err := LoadOrCreateHostIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity() error = %v, want nil", err)
	}

	record := newTestAdvertisement(t, identity, []string{"peer-a:8444"}, clock.Now())
	if _, _, err := registry.Advertise(record); err != nil {
		t.Fatalf("Advertise() error = %v, want nil", err)
	}

	clock.Advance(ttl + time.Second)

	var buf bytes.Buffer
	prev := ssqlog.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { ssqlog.SetSlogDefaultForTest(prev) })

	if err := registry.Prune(); err != nil {
		t.Fatalf("Prune() error = %v, want nil", err)
	}

	got := buf.String()
	if !strings.Contains(got, "host_registry.entry_expired") {
		t.Fatalf("expected a host_registry.entry_expired log line, got: %s", got)
	}
	if !strings.Contains(got, identity.ID.String()) {
		t.Fatalf("expected the log line to name the expired host id %s, got: %s", identity.ID.String(), got)
	}
}

func TestHostRegistry_Prune_should_NotLogHostRegistryEntryExpired_When_NoEntryDropped(t *testing.T) {
	stateDir := t.TempDir()
	ttl := 3 * time.Minute
	clock := &fakeClock{now: time.Now()}
	registry, err := NewHostRegistryWithClock(stateDir, ttl, clock)
	if err != nil {
		t.Fatalf("NewHostRegistryWithClock() error = %v, want nil", err)
	}

	identity, err := LoadOrCreateHostIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity() error = %v, want nil", err)
	}
	record := newTestAdvertisement(t, identity, []string{"peer-a:8444"}, clock.Now())
	if _, _, err := registry.Advertise(record); err != nil {
		t.Fatalf("Advertise() error = %v, want nil", err)
	}

	var buf bytes.Buffer
	prev := ssqlog.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { ssqlog.SetSlogDefaultForTest(prev) })

	if err := registry.Prune(); err != nil {
		t.Fatalf("Prune() error = %v, want nil", err)
	}

	if strings.Contains(buf.String(), "host_registry.entry_expired") {
		t.Fatalf("expected no host_registry.entry_expired log line when nothing was pruned, got: %s", buf.String())
	}
}

func TestHostRegistry_Prune_should_KeepEntry_When_RefreshedWithinTTL(t *testing.T) {
	stateDir := t.TempDir()
	ttl := 3 * time.Minute
	clock := &fakeClock{now: time.Now()}
	registry, err := NewHostRegistryWithClock(stateDir, ttl, clock)
	if err != nil {
		t.Fatalf("NewHostRegistryWithClock() error = %v, want nil", err)
	}

	identity, err := LoadOrCreateHostIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity() error = %v, want nil", err)
	}

	record := newTestAdvertisement(t, identity, []string{"peer-a:8444"}, clock.Now())
	if _, _, err := registry.Advertise(record); err != nil {
		t.Fatalf("Advertise() error = %v, want nil", err)
	}

	clock.Advance(ttl - time.Second)
	refresh := newTestAdvertisement(t, identity, []string{"peer-a:8444"}, clock.Now())
	if _, _, err := registry.Advertise(refresh); err != nil {
		t.Fatalf("refresh Advertise() error = %v, want nil", err)
	}

	clock.Advance(ttl - time.Second)
	if err := registry.Prune(); err != nil {
		t.Fatalf("Prune() error = %v, want nil", err)
	}

	if _, ok := registry.Lookup(identity.ID); !ok {
		t.Fatalf("Lookup() after Prune() following a within-TTL refresh = not found, want found")
	}
}

// TOFU-pinning cases (plan.md Story 3.2 Task 6, adversarial review):
//   - first advertisement for a new HostIdentity is accepted and pins its key
//   - a later advertisement from the same identity with a mismatched PublicKey
//     is rejected and does not overwrite the existing entry
//   - a later advertisement with an invalid Signature is rejected
//   - a valid re-advertisement signed by the pinned key is accepted (covered
//     by TestHostRegistry_Advertise_should_UpsertEntryAndRefreshLastSeenAt_When_AdvertisementReceived above)

func TestHostRegistry_Advertise_should_RejectAndNotOverwrite_When_PublicKeyMismatchesPinnedIdentity(t *testing.T) {
	stateDir := t.TempDir()
	clock := &fakeClock{now: time.Now()}
	registry, err := NewHostRegistryWithClock(stateDir, DefaultHostRegistryTTL, clock)
	if err != nil {
		t.Fatalf("NewHostRegistryWithClock() error = %v, want nil", err)
	}

	genuine, err := LoadOrCreateHostIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity(genuine) error = %v, want nil", err)
	}
	impostor, err := LoadOrCreateHostIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity(impostor) error = %v, want nil", err)
	}

	original := newTestAdvertisement(t, genuine, []string{"peer-a:8444"}, clock.Now())
	if _, accepted, err := registry.Advertise(original); err != nil || !accepted {
		t.Fatalf("original Advertise() = (accepted=%v, err=%v), want (true, nil)", accepted, err)
	}

	// Forge a record claiming genuine's HostIdentity but signed by impostor's
	// key -- an attempt to overwrite the pinned trust relationship.
	forged := AdvertisementRecord{
		HostIdentity:      genuine.ID,
		AdvertisedAddress: []string{"evil:1234"},
		AdvertisedAt:      clock.Now(),
	}
	forged.Sign(impostor)

	isNew, accepted, err := registry.Advertise(forged)
	if err != nil {
		t.Fatalf("forged Advertise() error = %v, want nil (rejection is not an error)", err)
	}
	if isNew || accepted {
		t.Fatalf("forged Advertise() = (isNew=%v, accepted=%v), want (false, false)", isNew, accepted)
	}

	entry, ok := registry.Lookup(genuine.ID)
	if !ok {
		t.Fatalf("Lookup() after rejected forgery = not found, want the original entry preserved")
	}
	if len(entry.AdvertisedAddress) != 1 || entry.AdvertisedAddress[0] != "peer-a:8444" {
		t.Fatalf("entry.AdvertisedAddress = %v, want original [peer-a:8444] to be preserved (not overwritten)", entry.AdvertisedAddress)
	}
}

func TestHostRegistry_Advertise_should_Reject_When_SignatureInvalid(t *testing.T) {
	stateDir := t.TempDir()
	clock := &fakeClock{now: time.Now()}
	registry, err := NewHostRegistryWithClock(stateDir, DefaultHostRegistryTTL, clock)
	if err != nil {
		t.Fatalf("NewHostRegistryWithClock() error = %v, want nil", err)
	}

	identity, err := LoadOrCreateHostIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity() error = %v, want nil", err)
	}

	record := newTestAdvertisement(t, identity, []string{"peer-a:8444"}, clock.Now())
	// Tamper with the signature after signing.
	record.Signature = append([]byte{}, record.Signature...)
	record.Signature[0] ^= 0xFF

	isNew, accepted, err := registry.Advertise(record)
	if err != nil {
		t.Fatalf("Advertise() error = %v, want nil", err)
	}
	if isNew || accepted {
		t.Fatalf("Advertise() = (isNew=%v, accepted=%v), want (false, false) for an invalid signature", isNew, accepted)
	}
}

func TestHostRegistry_Advertise_should_ReturnError_When_AdvertisedAddressEmpty(t *testing.T) {
	stateDir := t.TempDir()
	registry, err := NewHostRegistry(stateDir, DefaultHostRegistryTTL)
	if err != nil {
		t.Fatalf("NewHostRegistry() error = %v, want nil", err)
	}

	identity, err := LoadOrCreateHostIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity() error = %v, want nil", err)
	}

	record := newTestAdvertisement(t, identity, nil, time.Now())
	if _, _, err := registry.Advertise(record); err == nil {
		t.Fatalf("Advertise() error = nil, want an error for a record with no advertised address")
	}
}

func TestHostRegistry_Advertise_should_RejectAndNotUpsert_When_AdvertisedAddressIsLoopbackOrLinkLocal(t *testing.T) {
	implausible := []string{
		"127.0.0.1:8543",
		"localhost:8543",
		"169.254.169.254:80", // cloud metadata endpoint
		"[::1]:8543",
		"0.0.0.0:8543",
	}
	for _, addr := range implausible {
		t.Run(addr, func(t *testing.T) {
			stateDir := t.TempDir()
			registry, err := NewHostRegistry(stateDir, DefaultHostRegistryTTL)
			if err != nil {
				t.Fatalf("NewHostRegistry() error = %v, want nil", err)
			}
			identity, err := LoadOrCreateHostIdentity(t.TempDir())
			if err != nil {
				t.Fatalf("LoadOrCreateHostIdentity() error = %v, want nil", err)
			}

			record := newTestAdvertisement(t, identity, []string{addr}, time.Now())
			isNew, accepted, err := registry.Advertise(record)
			if err != nil {
				t.Fatalf("Advertise(%q) error = %v, want nil", addr, err)
			}
			if isNew || accepted {
				t.Fatalf("Advertise(%q) = (isNew=%v, accepted=%v), want (false, false)", addr, isNew, accepted)
			}
			if _, ok := registry.Lookup(identity.ID); ok {
				t.Fatalf("Lookup() found an entry for %q, want none -- implausible address must not be upserted", addr)
			}
		})
	}
}

func TestHostRegistry_Advertise_should_Accept_When_AdvertisedAddressIsOrdinaryLANHost(t *testing.T) {
	stateDir := t.TempDir()
	registry, err := NewHostRegistry(stateDir, DefaultHostRegistryTTL)
	if err != nil {
		t.Fatalf("NewHostRegistry() error = %v, want nil", err)
	}
	identity, err := LoadOrCreateHostIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity() error = %v, want nil", err)
	}

	record := newTestAdvertisement(t, identity, []string{"192.168.1.42:8543"}, time.Now())
	_, accepted, err := registry.Advertise(record)
	if err != nil {
		t.Fatalf("Advertise() error = %v, want nil", err)
	}
	if !accepted {
		t.Fatalf("Advertise() accepted = false, want true for an ordinary LAN address")
	}
}

func TestHostRegistry_LookupByHostname_should_ReturnEntry_When_HostnameMatchesAdvertisedAddress(t *testing.T) {
	stateDir := t.TempDir()
	registry, err := NewHostRegistry(stateDir, DefaultHostRegistryTTL)
	if err != nil {
		t.Fatalf("NewHostRegistry() error = %v, want nil", err)
	}

	identity, err := LoadOrCreateHostIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity() error = %v, want nil", err)
	}

	record := newTestAdvertisement(t, identity, []string{"otherhost:8444"}, time.Now())
	if _, _, err := registry.Advertise(record); err != nil {
		t.Fatalf("Advertise() error = %v, want nil", err)
	}

	id, entry, ok := registry.LookupByHostname("otherhost")
	if !ok {
		t.Fatalf("LookupByHostname(otherhost) = not found, want found")
	}
	if id.String() != identity.ID.String() {
		t.Fatalf("LookupByHostname(otherhost) returned HostID %s, want %s", id, identity.ID)
	}
	if len(entry.AdvertisedAddress) == 0 {
		t.Fatalf("LookupByHostname(otherhost) returned an entry with no advertised address")
	}
}

func TestHostRegistry_LookupByHostname_should_ReturnNotFound_When_NoEntryMatchesHostname(t *testing.T) {
	stateDir := t.TempDir()
	registry, err := NewHostRegistry(stateDir, DefaultHostRegistryTTL)
	if err != nil {
		t.Fatalf("NewHostRegistry() error = %v, want nil", err)
	}

	if _, _, ok := registry.LookupByHostname("nowhere"); ok {
		t.Fatalf("LookupByHostname(nowhere) = found, want not found against an empty registry")
	}
}

func TestHostRegistry_should_PersistAndReload_When_ReopenedAgainstSameStateDir(t *testing.T) {
	stateDir := t.TempDir()
	first, err := NewHostRegistry(stateDir, DefaultHostRegistryTTL)
	if err != nil {
		t.Fatalf("NewHostRegistry() error = %v, want nil", err)
	}

	identity, err := LoadOrCreateHostIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity() error = %v, want nil", err)
	}
	record := newTestAdvertisement(t, identity, []string{"peer-a:8444"}, time.Now())
	if _, _, err := first.Advertise(record); err != nil {
		t.Fatalf("Advertise() error = %v, want nil", err)
	}

	second, err := NewHostRegistry(stateDir, DefaultHostRegistryTTL)
	if err != nil {
		t.Fatalf("second NewHostRegistry() error = %v, want nil", err)
	}
	if _, ok := second.Lookup(identity.ID); !ok {
		t.Fatalf("Lookup() on reloaded registry = not found, want the persisted entry to survive a reload")
	}
}

// TestAdvertisementRecord_Verify_should_RoundTrip is a focused unit check on
// the signing/verification helpers themselves, independent of the registry.
func TestAdvertisementRecord_Verify_should_RoundTrip_When_SignedByOwnIdentity(t *testing.T) {
	identity, err := LoadOrCreateHostIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity() error = %v, want nil", err)
	}
	record := newTestAdvertisement(t, identity, []string{"peer-a:8444"}, time.Now())
	if !record.Verify() {
		t.Fatalf("record.Verify() = false, want true for a record signed by its own claimed identity")
	}
}

func TestAdvertisementRecord_Verify_should_ReturnFalse_When_PublicKeyMissing(t *testing.T) {
	record := AdvertisementRecord{
		HostIdentity:      HostID{},
		AdvertisedAddress: []string{"peer-a:8444"},
		AdvertisedAt:      time.Now(),
		PublicKey:         ed25519.PublicKey{},
		Signature:         []byte("not a real signature"),
	}
	if record.Verify() {
		t.Fatalf("record.Verify() = true, want false for a record with no public key")
	}
}

func TestPruneTickerLoop_should_RemoveStaleHostRegistryEntry_When_TTLElapsesWithNoRefresh(t *testing.T) {
	stateDir := t.TempDir()
	clock := &fakeClock{now: time.Now()}
	registry, err := NewHostRegistryWithClock(stateDir, DefaultHostRegistryTTL, clock)
	if err != nil {
		t.Fatalf("NewHostRegistryWithClock() error = %v, want nil", err)
	}
	identity, err := LoadOrCreateHostIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity() error = %v, want nil", err)
	}
	if _, _, err := registry.Advertise(newTestAdvertisement(t, identity, []string{"peer-a:8444"}, clock.Now())); err != nil {
		t.Fatalf("Advertise() error = %v, want nil", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ticks := make(chan time.Time)
	done := make(chan struct{})
	go func() {
		registry.RunPruneLoop(ctx, ticks)
		close(done)
	}()

	clock.Advance(DefaultHostRegistryTTL + time.Second)
	ticks <- clock.Now()
	// The loop only receives this second tick after finishing the first
	// Prune, so it synchronizes without a sleep.
	ticks <- clock.Now()
	cancel()
	<-done

	if _, ok := registry.Lookup(identity.ID); ok {
		t.Fatalf("Lookup() after prune tick past TTL = found, want pruned")
	}
	reloaded, err := NewHostRegistryWithClock(stateDir, DefaultHostRegistryTTL, clock)
	if err != nil {
		t.Fatalf("reopen registry: %v", err)
	}
	if _, ok := reloaded.Lookup(identity.ID); ok {
		t.Fatalf("pruned entry still present in host_registry.json after reload")
	}
}

func TestPruneTickerLoop_should_ContinueRunning_When_PruneReturnsError(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(stateDir, 0750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	clock := &fakeClock{now: time.Now()}
	registry, err := NewHostRegistryWithClock(stateDir, DefaultHostRegistryTTL, clock)
	if err != nil {
		t.Fatalf("NewHostRegistryWithClock() error = %v, want nil", err)
	}
	identity, err := LoadOrCreateHostIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity() error = %v, want nil", err)
	}
	if _, _, err := registry.Advertise(newTestAdvertisement(t, identity, []string{"peer-a:8444"}, clock.Now())); err != nil {
		t.Fatalf("Advertise() error = %v, want nil", err)
	}
	clock.Advance(DefaultHostRegistryTTL + time.Second)

	var buf lockedBuffer
	prev := ssqlog.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { ssqlog.SetSlogDefaultForTest(prev) })

	// Removing the state dir makes the flock acquire inside Prune's persist fail.
	if err := os.RemoveAll(stateDir); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ticks := make(chan time.Time)
	done := make(chan struct{})
	go func() {
		registry.RunPruneLoop(ctx, ticks)
		close(done)
	}()

	ticks <- clock.Now() // fails
	// The send returns on receipt, not on Prune finishing; wait for the failure log
	// before restoring the dir or the first Prune could succeed.
	wait.RequireEventually(t, func() bool { return strings.Contains(buf.String(), "host_registry.prune_failed") },
		5*time.Second, time.Millisecond, "first Prune never logged host_registry.prune_failed")
	if err := os.MkdirAll(stateDir, 0750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	ticks <- clock.Now() // must be received: loop survived the error
	ticks <- clock.Now() // synchronizes on the second Prune finishing
	cancel()
	<-done

	if !strings.Contains(buf.String(), "host_registry.prune_failed") {
		t.Fatalf("expected host_registry.prune_failed log line, got: %s", buf.String())
	}
	if _, ok := registry.Lookup(identity.ID); ok {
		t.Fatalf("entry still present after a successful retry tick, want pruned")
	}
}

var (
	adr002Mention   = regexp.MustCompile(`ADR-002`)
	adr002FullPaths = regexp.MustCompile(`docs/adr/ADR-002-workspace-host-registry-gossip\.md|project_plans/backlog-deep-linking/decisions/ADR-002-gossip-based-host-registry\.md`)
)

func TestADR002References_should_PointToCanonicalPath_When_CodeCommentsGrepped(t *testing.T) {
	for _, file := range []string{"host_registry.go", "host_advertiser.go"} {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		bare := adr002FullPaths.ReplaceAll(data, nil)
		if adr002Mention.Match(bare) {
			t.Errorf("%s has a bare ADR-002 reference that does not name a full path", file)
		}
		if !adr002FullPaths.Match(data) {
			t.Errorf("%s has no ADR-002 path reference at all", file)
		}
	}
}

func TestADR002PointerFile_should_ExistAtDocsAdrPath_When_RepoChecked(t *testing.T) {
	pointer := filepath.Join("..", "docs", "adr", "ADR-002-workspace-host-registry-gossip.md")
	data, err := os.ReadFile(pointer)
	if err != nil {
		t.Fatalf("pointer file missing: %v", err)
	}
	canonical := "project_plans/backlog-deep-linking/decisions/ADR-002-gossip-based-host-registry.md"
	if !strings.Contains(string(data), canonical) {
		t.Errorf("pointer file does not link to %s", canonical)
	}
	if _, err := os.Stat(filepath.Join("..", canonical)); err != nil {
		t.Errorf("canonical decision record missing: %v", err)
	}
}
