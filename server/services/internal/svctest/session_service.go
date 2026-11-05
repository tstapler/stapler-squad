package svctest

import (
	"testing"

	"github.com/tstapler/stapler-squad/session"
)

// createTestStorage creates a test storage backed by an in-memory Ent
// repository via session.NewTestEntRepository — see that function's doc
// comment for why this uses a named, shared-cache in-memory DSN rather than
// a bare ":memory:" literal.
//
// This package's fixtures are called 300+ times across its test files; each
// call still pays the full Ent schema-creation + backfill-migration cost,
// but backing it with an in-memory SQLite database (rather than a
// t.TempDir()-backed file) removes the disk I/O and WAL fsync overhead.
func CreateTestStorage(t *testing.T) *session.Storage {
	t.Helper()

	repo := session.NewTestEntRepository(t)

	storage, err := session.NewStorageWithRepository(repo)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	return storage
}
