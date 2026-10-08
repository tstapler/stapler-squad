package session

import (
	"context"
	"database/sql"
	"fmt"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"modernc.org/sqlite"

	"github.com/tstapler/stapler-squad/session/ent"
)

// withSeedURI makes NewEntRepository copy the database at uri (a fully
// schema-migrated shared-cache in-memory database with startup migrations
// applied) into its own instead of running schema creation and startup
// migrations. Test-only: production databases must always migrate.
func withSeedURI(uri string) RepositoryOption {
	return func(r interface{}) error {
		if entRepo, ok := r.(*EntRepository); ok {
			entRepo.seedURI = uri
		}
		return nil
	}
}

// newSeededEntRepository finishes NewEntRepository for a seeded database: copy the
// template into db with SQLite's backup API, then build the ent client. The copy
// lands in db's own named shared-cache database, so other connections opened on
// the same DSN (some test helpers do this) see it too — unlike
// sqlite3_deserialize, which detaches the connection from the shared cache. The
// template already holds the schema and the result of every startup migration, so
// neither is re-run (and EntSchemaCreateMu, which only serializes Atlas, is not
// needed).
func newSeededEntRepository(db *sql.DB, repo *EntRepository) (*EntRepository, error) {
	if err := restoreFrom(db, repo.seedURI); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to load seed database: %w", err)
	}
	repo.client = ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	return repo, nil
}

// restoreFrom copies the database at srcURI into db's single connection using
// SQLite's online backup API (modernc's conn.NewRestore).
func restoreFrom(db *sql.DB, srcURI string) error {
	conn, err := db.Conn(context.Background())
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Raw(func(dc any) error {
		restorer, ok := dc.(interface {
			NewRestore(srcURI string) (*sqlite.Backup, error)
		})
		if !ok {
			return fmt.Errorf("sqlite driver connection %T does not support NewRestore", dc)
		}
		backup, err := restorer.NewRestore(srcURI)
		if err != nil {
			return err
		}
		// A negative page count copies everything in one step.
		if _, err := backup.Step(-1); err != nil {
			_ = backup.Finish()
			return err
		}
		return backup.Finish()
	})
}
