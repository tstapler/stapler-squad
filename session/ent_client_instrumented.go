package session

import (
	"database/sql"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"github.com/tstapler/stapler-squad/session/dbobs"
	"github.com/tstapler/stapler-squad/session/ent"
)

// newInstrumentedEntClient builds the ent client over db with query timing,
// slow-query logging, spans and pool stats (session/dbobs). On by default.
func newInstrumentedEntClient(db *sql.DB) *ent.Client {
	drv := dbobs.Wrap(entsql.OpenDB(dialect.SQLite, db), db, dbobs.ConfigFromEnv("ent"))
	return ent.NewClient(ent.Driver(drv))
}
