package session

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	entsession "github.com/tstapler/stapler-squad/session/ent/session"
)

var testEntRepoCounter int64

// NewTestEntRepository returns an EntRepository backed by a uniquely-named
// shared-cache in-memory SQLite database, closed automatically via
// t.Cleanup. It replaces the repeated
// NewEntRepository(WithDatabasePath(filepath.Join(dir, "sessions.db")))
// pattern duplicated across this package and server/, server/services/,
// server/mcp/, and server/workflows/ test files — those call sites paid for
// real file creation, WAL journal files, and disk I/O on every test despite
// never needing on-disk durability.
//
// The DSN's name must be unique per repository: cache=shared makes SQLite
// look up the in-memory database by name process-wide, so two repositories
// with the same name would see each other's data.
func NewTestEntRepository(t testing.TB) *EntRepository {
	t.Helper()
	id := atomic.AddInt64(&testEntRepoCounter, 1)
	dsn := fmt.Sprintf("file:testentrepo%d?mode=memory&cache=shared", id)
	repo, err := NewEntRepository(WithDatabasePath(dsn))
	if err != nil {
		t.Fatalf("NewTestEntRepository: %v", err)
	}
	t.Cleanup(func() {
		if err := repo.Close(); err != nil {
			t.Logf("NewTestEntRepository cleanup: repo.Close: %v", err)
		}
	})
	return repo
}

// TestBackdateCreationProgress rewrites a persisted instance's
// creation_progress_updated_at column directly to `when`, simulating elapsed
// wall-clock time without a real sleep. There is no production setter for an
// arbitrary (possibly past) timestamp -- SetCreationProgress always stamps
// "now" -- so callers outside this package (e.g. the Stale-Creation
// Sweeper's tests in server/services) need this to exercise their
// reload-from-storage path without importing session/ent directly, which
// server/services' depguard rule (no_ent_in_services) forbids.
func TestBackdateCreationProgress(t *testing.T, storage *Storage, uuid string, when time.Time) {
	t.Helper()
	client := storage.GetEntClient()
	if client == nil {
		t.Fatalf("TestBackdateCreationProgress: storage has no ent client")
	}
	n, err := client.Session.Update().
		Where(entsession.UUID(uuid)).
		SetCreationProgressUpdatedAt(when).
		Save(context.Background())
	if err != nil {
		t.Fatalf("TestBackdateCreationProgress: %v", err)
	}
	if n != 1 {
		t.Fatalf("TestBackdateCreationProgress: expected to update 1 row, updated %d", n)
	}
}
