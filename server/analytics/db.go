package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/tstapler/stapler-squad/internal/sqlitedsn"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/dbobs"
	"github.com/tstapler/stapler-squad/session/ent"
	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

// OpenAnalyticsDB opens (or creates) the dedicated analytics.db SQLite database
// inside dataDir, runs auto-migration, and returns the ent client.
//
// The returned client is configured with a single open connection to enforce
// SQLite's single-writer semantics and avoid "database is locked" errors.
// The caller is responsible for calling client.Close() on shutdown.
func OpenAnalyticsDB(ctx context.Context, dataDir string) (*ent.Client, error) {
	client, _, err := OpenAnalyticsDBWithDriver(ctx, dataDir)
	return client, err
}

// OpenAnalyticsDBWithDriver is OpenAnalyticsDB that also returns the
// dbobs-instrumented driver behind the client, for session/dbmaint.
func OpenAnalyticsDBWithDriver(ctx context.Context, dataDir string) (*ent.Client, dialect.ExecQuerier, error) {
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return nil, nil, fmt.Errorf("analytics db: create data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "analytics.db")
	// _timeout must be set explicitly: unlike the previous mattn/go-sqlite3 driver
	// (which defaulted busy_timeout to 5000ms even when the DSN omitted it),
	// modernc.org/sqlite only issues `PRAGMA busy_timeout` when the DSN sets
	// _timeout/_busy_timeout, otherwise leaving SQLite's own default of 0 (immediate
	// SQLITE_BUSY on lock contention) in effect.
	dsn := sqlitedsn.New("file:"+dbPath).
		WithWAL().
		WithSynchronousNormal().
		WithForeignKeys().
		WithBusyTimeout(5000*time.Millisecond).
		WithPragma("wal_autocheckpoint", "1000").
		WithPragma("auto_vacuum", "incremental").
		Build()

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("analytics db: open sqlite: %w", err)
	}

	// SQLite supports only one writer at a time; serialise all access through a
	// single connection to eliminate "database is locked" contention.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	drv := dbobs.Wrap(entsql.OpenDB(dialect.SQLite, db), db, dbobs.ConfigFromEnv("analytics"))
	client := ent.NewClient(ent.Driver(drv))

	// Auto-migrate the AnalyticsEvent table (and any future schema additions).
	// Serialized via session.EntSchemaCreateMu — see its doc comment for why:
	// entgo.io/ent/dialect/sql/schema.(*Atlas) has package-level state that
	// races when multiple goroutines run schema creation concurrently, even
	// across independent *ent.Client instances.
	session.EntSchemaCreateMu.Lock()
	err = client.Schema.Create(ctx)
	session.EntSchemaCreateMu.Unlock()
	if err != nil {
		_ = client.Close()
		return nil, nil, fmt.Errorf("analytics db: schema migration: %w", err)
	}
	// Legacy cleanup, not needed for correctness: a transient lock must not disable analytics.
	if err := dropObsoleteEscapeIndexes(ctx, db); err != nil {
		log.Warn("analytics db: drop obsolete escape indexes failed; retrying next start", "err", err)
	}
	// Bounded compaction for databases created with incremental auto-vacuum.
	// This is a no-op for legacy databases until their next maintenance VACUUM.
	if _, err := db.ExecContext(ctx, "PRAGMA incremental_vacuum(4096)"); err != nil {
		_ = client.Close()
		return nil, nil, fmt.Errorf("analytics db: incremental vacuum: %w", err)
	}

	return client, drv, nil
}
