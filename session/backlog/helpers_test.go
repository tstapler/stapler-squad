package backlog

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
)

// createTestStorage returns a Storage over a fresh in-memory test repository; the cleanup func is a no-op kept so
// call sites mirror session's own helper of the same name (NewTestEntRepository registers its own t.Cleanup).
func createTestStorage(t *testing.T) (*session.Storage, func()) {
	t.Helper()
	storage, err := session.NewStorageWithRepository(session.NewTestEntRepository(t))
	require.NoError(t, err)
	return storage, func() {}
}

// newTestInstance returns a minimal Paused Instance ready for Storage.AddInstance.
func newTestInstance(title string) *session.Instance { return session.NewPausedTestInstance(title) }

var prTestEpoch = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

type fakeMarkerClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeMarkerClock() *fakeMarkerClock {
	return &fakeMarkerClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (c *fakeMarkerClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeMarkerClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}
