package session

import (
	"database/sql"
	"fmt"

	"github.com/tstapler/stapler-squad/session/dbpool"
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
	if err := dbpool.RestoreFrom(db, repo.seedURI); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to load seed database: %w", err)
	}
	repo.Client, repo.driver = newInstrumentedEntClient(db, nil)
	repo.watchGuidanceWrites(repo.Client)
	return repo, nil
}
