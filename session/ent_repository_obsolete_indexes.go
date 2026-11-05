package session

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/tstapler/stapler-squad/log"
)

// obsoleteIndexes are indexes removed from the ent schema. ent's migration is
// additive and never drops indexes, so databases created before the removal
// keep paying their write amplification until dropped here.
var obsoleteIndexes = []string{ //nolint:gochecknoglobals
	"classificationanalytics_session_id",
	"classificationanalytics_decision",
	"classificationanalytics_risk_level",
	"classificationanalytics_rule_id",
}

// dropObsoleteIndexes is idempotent (DROP INDEX IF EXISTS). Freed pages return
// to the OS through the incremental-vacuum maintenance in session/dbmaint.
func dropObsoleteIndexes(ctx context.Context, er *EntRepository) error {
	if er.driver == nil {
		return nil // built from a bare client (tests/seeded); no raw exec path
	}
	for _, name := range obsoleteIndexes {
		var res sql.Result
		if err := er.driver.Exec(ctx, fmt.Sprintf("DROP INDEX IF EXISTS %q", name), []any{}, &res); err != nil {
			return fmt.Errorf("drop obsolete index %s: %w", name, err)
		}
	}
	log.InfoLog().Printf("[Migration] dropped %d obsolete index(es) if present", len(obsoleteIndexes))
	return nil
}
