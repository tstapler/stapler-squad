package session

import (
	"fmt"
	"sync/atomic"
	"testing"
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
	t.Cleanup(func() { repo.Close() })
	return repo
}
