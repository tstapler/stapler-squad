package session

import (
	"database/sql"
	"os"
	"strconv"
	"strings"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"github.com/tstapler/stapler-squad/internal/sqlitedsn"
	"github.com/tstapler/stapler-squad/session/dbobs"
	"github.com/tstapler/stapler-squad/session/dbsplit"
	"github.com/tstapler/stapler-squad/session/ent"
)

// ReadPoolEnv sets the read-pool size; 0 disables the reader/writer split.
const ReadPoolEnv = "STAPLER_SQUAD_DB_READ_CONNS"

const defaultReadPoolSize = 4

// tuneEntDSN applies synchronous=NORMAL, which is safe with WAL (a power loss
// may drop the last commits, never corrupts) and removes an fsync from every
// commit; the modernc default is FULL. cache_size/mmap_size/temp_store showed no
// measurable gain on this workload, so they are left at SQLite defaults.
func tuneEntDSN(b *sqlitedsn.Builder) *sqlitedsn.Builder {
	return b.WithSynchronousNormal()
}

func readPoolSize() int {
	n, err := strconv.Atoi(os.Getenv(ReadPoolEnv))
	if err != nil || n < 0 {
		return defaultReadPoolSize
	}
	return n
}

// isOnDiskDatabase reports whether path names a real file. In-memory databases
// (":memory:", file: URIs) are per-connection or shared-cache and cannot be
// reopened by a second pool.
func isOnDiskDatabase(path string) bool {
	return path != "" && path != ":memory:" && !strings.HasPrefix(path, "file:") && !strings.Contains(path, "mode=memory")
}

// newInstrumentedEntClient builds the ent client over db with query timing,
// slow-query logging, spans and pool stats (session/dbobs). On by default.
// A non-nil readDB routes plain SELECTs to it (session/dbsplit); stats for
// that pool are reported as "ent-read". The driver is also returned so
// background maintenance (session/dbmaint) runs through the same instrumentation.
func newInstrumentedEntClient(db, readDB *sql.DB) (*ent.Client, dialect.Driver) {
	drv := dbobs.Wrap(entsql.OpenDB(dialect.SQLite, db), db, dbobs.ConfigFromEnv("ent"))
	if readDB != nil {
		reader := dbobs.Wrap(entsql.OpenDB(dialect.SQLite, readDB), readDB, dbobs.ConfigFromEnv("ent-read"))
		drv = dbsplit.New(drv, reader)
	}
	return ent.NewClient(ent.Driver(drv)), drv
}
