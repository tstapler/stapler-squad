package dbpool

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/internal/sqlitedsn"
)

// OpenForeignReadOnly opens another tool's SQLite file (opencode, agy, ...) for
// reading. It sets query_only and never requests a journal mode, so opening
// leaves the owner's mode alone (query_only does not stop a caller from running
// PRAGMA journal_mode itself). busy is how long to retry on a locked file; 0
// fails fast.
func OpenForeignReadOnly(path string, busy time.Duration) (*sql.DB, error) {
	db, err := sql.Open("sqlite", sqlitedsn.New(path).WithBusyTimeout(busy).WithQueryOnly().Build())
	if err != nil {
		return nil, fmt.Errorf("dbpool: open foreign db %q: %w", path, err)
	}
	return db, nil
}
