package dbpool

import (
	"context"
	"database/sql"
	"fmt"

	"modernc.org/sqlite"
)

// RestoreFrom copies the database at srcURI (typically a shared-cache
// in-memory template) into db's single connection using SQLite's online
// backup API (modernc's conn.NewRestore). The copy lands in db's own named
// database, so other connections opened on the same DSN see it too.
func RestoreFrom(db *sql.DB, srcURI string) error {
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
