// Package entdbtest gives leaf store packages a migrated in-memory *ent.Client
// for tests without importing package session (which would be an import cycle
// once session embeds the leaf). It builds the ent schema once per process
// into a shared-cache template and restores a private copy per test; startup
// data migrations are not applied, so tests seed their own rows.
package entdbtest

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"github.com/tstapler/stapler-squad/internal/sqlitedsn"
	"github.com/tstapler/stapler-squad/session/dbpool"
	"github.com/tstapler/stapler-squad/session/ent"

	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

var (
	dbCounter      atomic.Int64
	templateOnce   sync.Once
	templateURI    string
	templateErr    error
	templateKeeper *sql.Conn // never closed: pins the template in memory for the process lifetime
)

// Warm builds the process-wide template database. Its pool goroutines live for
// the whole process, so a goleak.IgnoreCurrent() baseline taken before the
// first NewClient call flags them as leaks; call Warm from TestMain first.
func Warm() error {
	_, err := template()
	return err
}

func template() (string, error) {
	templateOnce.Do(func() {
		uri := fmt.Sprintf("file:entdbtesttemplate%d?mode=memory&cache=shared", dbCounter.Add(1))
		db, err := open(uri)
		if err != nil {
			templateErr = err
			return
		}
		client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
		dbpool.EntSchemaCreateMu.Lock()
		err = client.Schema.Create(context.Background())
		dbpool.EntSchemaCreateMu.Unlock()
		if err != nil {
			_ = client.Close()
			templateErr = fmt.Errorf("create template schema: %w", err)
			return
		}
		// An in-memory database lives only while a connection stays open and the
		// pool above recycles its connection; pin a second one with no lifetime.
		keeper, err := sql.Open("sqlite", uri) //nolint:norawdb in-memory test fixture helper
		if err != nil {
			templateErr = err
			return
		}
		keeper.SetConnMaxLifetime(0)
		if templateKeeper, err = keeper.Conn(context.Background()); err != nil {
			templateErr = err
			return
		}
		_ = templateKeeper // held, not read: the open connection is what keeps the database alive
		templateURI = uri
	})
	return templateURI, templateErr
}

func open(uri string) (*sql.DB, error) {
	dsn := sqlitedsn.New(uri).
		WithBusyTimeout(5000 * time.Millisecond).
		WithForeignKeysShort().
		WithEntTimeCompat().
		Build()
	db, err := sql.Open("sqlite", dsn) //nolint:norawdb in-memory test fixture helper
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)
	return db, nil
}

// NewClient returns an *ent.Client over a private in-memory database holding
// the full ent schema and no rows, closed via t.Cleanup.
func NewClient(t testing.TB) *ent.Client {
	t.Helper()
	seed, err := template()
	if err != nil {
		t.Fatalf("entdbtest: build template database: %v", err)
	}
	uri := fmt.Sprintf("file:entdbtest%d?mode=memory&cache=shared", dbCounter.Add(1))
	db, err := open(uri)
	if err != nil {
		t.Fatalf("entdbtest: open database: %v", err)
	}
	if err := dbpool.RestoreFrom(db, seed); err != nil {
		_ = db.Close()
		t.Fatalf("entdbtest: restore template: %v", err)
	}
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Logf("entdbtest: close client: %v", err)
		}
	})
	return client
}
