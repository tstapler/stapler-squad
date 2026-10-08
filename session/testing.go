package session

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/internal/sqlitedsn"
	entsession "github.com/tstapler/stapler-squad/session/ent/session"
)

var testEntRepoCounter int64

var (
	templateKeeper *sql.Conn // never closed: pins the template in memory for the process lifetime
	templateDBOnce sync.Once
	templateDBURI  string
	templateDBErr  error
)

// migratedTemplateDBURI returns the DSN of a shared-cache in-memory database that
// already has the full schema and every startup migration applied, built once per
// test process and kept open (never closed) so the in-memory database survives for
// every later copy. Schema creation (ent/Atlas) is ~70% of NewEntRepository's CPU
// and runs under a process-wide mutex, so repeating it for every test is the
// dominant cost of storage-backed tests; copying this template skips it.
func migratedTemplateDBURI() (string, error) {
	templateDBOnce.Do(func() {
		dsn := fmt.Sprintf("file:testentrepotemplate%d?mode=memory&cache=shared", atomic.AddInt64(&testEntRepoCounter, 1))
		if _, err := NewEntRepository(WithDatabasePath(dsn)); err != nil {
			templateDBErr = err
			return
		}
		// An in-memory database lives only while a connection to it stays open, and
		// NewEntRepository's pool recycles its connection after an hour
		// (SetConnMaxLifetime). Pin a second connection with no lifetime limit so the
		// template survives a long-running test process.
		keeper, err := sql.Open("sqlite", dsn)
		if err != nil {
			templateDBErr = err
			return
		}
		keeper.SetConnMaxLifetime(0)
		if templateKeeper, err = keeper.Conn(context.Background()); err != nil {
			templateDBErr = err
			return
		}
		templateDBURI = dsn
	})
	return templateDBURI, templateDBErr
}

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
	seedURI, seedErr := migratedTemplateDBURI()
	if seedErr != nil {
		t.Fatalf("NewTestEntRepository: build template database: %v", seedErr)
	}
	repo, err := NewEntRepository(WithDatabasePath(dsn), withSeedURI(seedURI))
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

// TestBackdateItemSessionCreatedAt rewrites a persisted ItemSession's
// created_at column directly to `when`. created_at is Immutable() in the ent
// schema (session/ent/schema/itemsession.go), so there is no generated
// setter for it -- unlike TestBackdateCreationProgress's field, it can't be
// reached through client.ItemSession.Update() at all. Callers that need to
// simulate an old rework round without a real sleep (the Superseded-Session
// Sweeper's tests in server/services, seeding rounds "as if persisted before
// the sweeper ever ran") instead open a second raw connection to the same
// database and issue a plain UPDATE, using the identical DSN-building the
// production connection uses so the write round-trips through the same
// modernc.org/sqlite time-compat quirks (see WithEntTimeCompat's doc
// comment) that later ent reads depend on.
func TestBackdateItemSessionCreatedAt(t *testing.T, storage *Storage, itemSessionID string, when time.Time) error {
	t.Helper()
	if storage.repo == nil {
		return fmt.Errorf("TestBackdateItemSessionCreatedAt: storage has no repository")
	}

	dsn := sqlitedsn.New(storage.repo.dbPath).
		WithWAL().
		WithBusyTimeout(5000 * time.Millisecond).
		WithForeignKeysShort().
		WithEntTimeCompat().
		Build()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("TestBackdateItemSessionCreatedAt: open: %w", err)
	}
	defer db.Close()

	res, err := db.ExecContext(context.Background(), "UPDATE item_sessions SET created_at = ? WHERE id = ?", when, itemSessionID)
	if err != nil {
		return fmt.Errorf("TestBackdateItemSessionCreatedAt: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("TestBackdateItemSessionCreatedAt: RowsAffected: %w", err)
	}
	if n != 1 {
		return fmt.Errorf("TestBackdateItemSessionCreatedAt: expected to update 1 row, updated %d", n)
	}
	return nil
}
