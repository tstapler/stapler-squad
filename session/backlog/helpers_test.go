package backlog

import (
	"testing"

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
