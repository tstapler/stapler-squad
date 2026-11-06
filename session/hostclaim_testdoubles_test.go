package session

// Copies of the host-claim test helpers that package session's own tests use. They live in session/hostclaim's
// _test.go files, which no other package can import.

import (
	"bytes"
	"log/slog"
	"testing"
	"time"

	ssqlog "github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/hostclaim"
)

const testIssueURL = "https://github.com/o/r/issues/1"

// fakeClock is a controllable hostclaim.Clock for deterministic TTL tests.
type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func newTestIdentity(t *testing.T) hostclaim.HostIdentity {
	t.Helper()
	identity, err := hostclaim.LoadOrCreateHostIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity() error = %v, want nil", err)
	}
	return identity
}

func newTestAdvertisement(t *testing.T, identity hostclaim.HostIdentity, addrs []string, at time.Time) hostclaim.AdvertisementRecord {
	t.Helper()
	record := hostclaim.AdvertisementRecord{
		HostIdentity:      identity.ID,
		AdvertisedAddress: addrs,
		AdvertisedAt:      at,
	}
	record.Sign(identity)
	return record
}

// newTestClaimIndex returns a ClaimIndex whose registry has pinned every identity in pinned, sharing stateDir.
func newTestClaimIndex(t *testing.T, stateDir string, clock hostclaim.Clock, pinned ...hostclaim.HostIdentity) *hostclaim.ClaimIndex {
	t.Helper()
	registry, err := hostclaim.NewHostRegistryWithClock(t.TempDir(), hostclaim.DefaultHostRegistryTTL, clock)
	if err != nil {
		t.Fatalf("NewHostRegistryWithClock() error = %v, want nil", err)
	}
	for _, identity := range pinned {
		if _, accepted, err := registry.Advertise(newTestAdvertisement(t, identity, []string{"peer:8444"}, clock.Now())); err != nil || !accepted {
			t.Fatalf("Advertise(pin) = accepted %v, err %v", accepted, err)
		}
	}
	index, err := hostclaim.NewClaimIndex(stateDir, registry)
	if err != nil {
		t.Fatalf("NewClaimIndex() error = %v, want nil", err)
	}
	return index
}

// captureLogInfo redirects the log package's slog seam to a text handler writing into the returned buffer.
func captureLogInfo(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := ssqlog.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() {
		ssqlog.SetSlogDefaultForTest(prev)
	})
	return &buf
}
