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
	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"github.com/tstapler/stapler-squad/session/ent"
)

// OpenAnalyticsDB opens (or creates) the dedicated analytics.db SQLite database
// inside dataDir, runs auto-migration, and returns the ent client.
//
// The returned client is configured with a single open connection to enforce
// SQLite's single-writer semantics and avoid "database is locked" errors.
// The caller is responsible for calling client.Close() on shutdown.
func OpenAnalyticsDB(ctx context.Context, dataDir string) (*ent.Client, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("analytics db: create data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "analytics.db")
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_synchronous=NORMAL&_foreign_keys=on", dbPath)

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("analytics db: open sqlite: %w", err)
	}

	// SQLite supports only one writer at a time; serialise all access through a
	// single connection to eliminate "database is locked" contention.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := ent.NewClient(ent.Driver(drv))

	// Auto-migrate the AnalyticsEvent table (and any future schema additions).
	if err := client.Schema.Create(ctx); err != nil {
		client.Close()
		return nil, fmt.Errorf("analytics db: schema migration: %w", err)
	}

	return client, nil
}
