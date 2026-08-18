package sqlitedsn

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite" // Pure Go SQLite driver — must honor the DSN pragmas this package builds
)

// TestOpen_should_EnforceForeignKeysAndWAL_When_BuiltViaWithForeignKeysAndWithWAL
// opens a real connection through modernc.org/sqlite (the driver stapler-squad
// switched to away from cgo-only mattn/go-sqlite3, see .backlog-context.md)
// and asserts PRAGMA foreign_keys / PRAGMA journal_mode live-report the values
// this package's DSN builder requests, rather than only asserting the DSN
// string looks right (TestWithForeignKeys_should_SetLongForm and
// TestBuild_should_JoinParamsWithQuestionMark_When_PathHasNoExistingQuery do
// that). A cgo-vs-pure-Go driver swap is exactly the kind of change that can
// silently stop honoring a query-string pragma while still producing an
// identical-looking DSN.
func TestOpen_should_EnforceForeignKeysAndWAL_When_BuiltViaWithForeignKeysAndWithWAL(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pragma-check.db")
	dsn := New(dbPath).WithWAL().WithForeignKeys().Build()

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	// journal_mode=WAL must be requested before any write; querying it is
	// enough to trigger the pragma if it hasn't run yet.
	var journalMode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want %q", journalMode, "wal")
	}

	var foreignKeys int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, want 1", foreignKeys)
	}
}
