package analytics

import (
	"context"
	"database/sql"
	"fmt"
)

// obsoleteEscapeIndexes are escape_events indexes removed from the ent schema.
// ent's additive migration never drops indexes, so existing databases keep them
// (~190 MB at 894k rows) until dropped here.
var obsoleteEscapeIndexes = []string{
	"escapeevent_session_id",
	"escapeevent_session_id_stage",
	"escapeevent_mangled",
	"escapeevent_sequence_type",
	"escapeevent_project_path",
}

// dropObsoleteEscapeIndexes is idempotent; freed pages are reclaimed by incremental vacuum.
func dropObsoleteEscapeIndexes(ctx context.Context, db *sql.DB) error {
	for _, name := range obsoleteEscapeIndexes {
		if _, err := db.ExecContext(ctx, fmt.Sprintf("DROP INDEX IF EXISTS %q", name)); err != nil {
			return fmt.Errorf("drop index %s: %w", name, err)
		}
	}
	return nil
}
