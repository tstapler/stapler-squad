// Package escapedb owns db/escape_events.db: the terminal escape-event batch
// writer, its retention and eviction, and the one-time copy of rows from the
// legacy analytics.db table. It is the only package that selects the EscapeEvent
// entity (enforced by tools/lint/nowrongdbentity).
package escapedb

import (
	"context"
	"fmt"
	"path/filepath"

	"entgo.io/ent/dialect/sql/schema"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/dbpool"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/migrate"
)

// Path is the escape-events database file for a config directory.
func Path(configDir string) string {
	return filepath.Join(configDir, "db", "escape_events.db")
}

// Open opens db/escape_events.db through the dbpool registry (own single writer,
// WAL, synchronous=NORMAL) and creates only the escape_events table. Closing the
// returned client closes the database.
func Open(ctx context.Context, configDir string) (*ent.Client, *dbpool.Database, error) {
	h, err := dbpool.OpenEscapeEvents(Path(configDir))
	if err != nil {
		return nil, nil, fmt.Errorf("escape events db: %w", err)
	}
	client := ent.NewClient(ent.Driver(h.Driver()))

	// Serialized like every ent schema create: the Atlas migrator has package-level state.
	session.EntSchemaCreateMu.Lock()
	err = migrate.Create(ctx, migrate.NewSchema(h.Driver()), []*schema.Table{migrate.EscapeEventsTable})
	session.EntSchemaCreateMu.Unlock()
	if err != nil {
		_ = client.Close()
		return nil, nil, fmt.Errorf("escape events db: schema migration: %w", err)
	}
	return client, h.Database, nil
}
