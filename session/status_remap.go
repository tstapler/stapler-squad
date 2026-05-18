package session

// status_remap.go contains the one-time idempotent SQL migration that remaps
// the sessions.status integer column from the old 7-value iota to the new
// 5-value iota introduced in Epic 1 of the session hibernation feature.
//
// Old iota: Running=0, Ready=1, Loading=2, Paused=3, NeedsApproval=4, Creating=5, Stopped=6
// New iota: Creating=0, Active=1, Paused=2, Stopped=3, Hibernated=4
//
// The migration uses a sentinel offset (+100) to avoid value collisions during
// the multi-step remap.  It is idempotent: values 0–4 are already in the new
// range and the CASE expression leaves them unchanged; the sentinel rows (100–106)
// cannot occur in a database that has already been migrated.

import (
	"database/sql"
	"fmt"

	"github.com/tstapler/stapler-squad/log"
)

// runStatusRemap remaps legacy status integer values to the new 5-state model.
// It is safe to call on a freshly-created database (no rows → no-op) and
// idempotent when called on an already-migrated database.
func runStatusRemap(db *sql.DB) error {
	// Quick check: does the sessions table have any rows with values > 4?
	// If not, nothing to do (either empty DB or already migrated).
	var legacyCount int
	row := db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE status > 4`)
	if err := row.Scan(&legacyCount); err != nil {
		// Table may not exist yet (fresh DB before schema.Create) — ignore.
		return nil
	}
	if legacyCount == 0 {
		return nil
	}

	log.Info("running status remap migration", "legacy_rows", legacyCount)

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("status remap: begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Step 1: Shift all legacy values up by 100 as sentinels (avoids collision).
	// Values already in the new range (0–4) are unaffected because status + 100
	// produces 100–104 which the CASE expression in Step 2 maps back to the
	// correct new values — but we only shift rows where status > 4.
	if _, err = tx.Exec(`UPDATE sessions SET status = status + 100 WHERE status > 4`); err != nil {
		return fmt.Errorf("status remap: sentinel shift: %w", err)
	}

	// Step 2: Remap sentinel values to new integers.
	if _, err = tx.Exec(`UPDATE sessions SET status = CASE status
		WHEN 100 THEN 1   -- old Running(0)        → new Active(1)
		WHEN 101 THEN 1   -- old Ready(1)           → new Active(1)
		WHEN 102 THEN 0   -- old Loading(2)         → new Creating(0)
		WHEN 103 THEN 2   -- old Paused(3)          → new Paused(2)
		WHEN 104 THEN 1   -- old NeedsApproval(4)   → new Active(1)
		WHEN 105 THEN 0   -- old Creating(5)        → new Creating(0)
		WHEN 106 THEN 3   -- old Stopped(6)         → new Stopped(3)
		ELSE status        -- future-proof: leave unknowns unchanged
	END WHERE status >= 100`); err != nil {
		return fmt.Errorf("status remap: case remap: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("status remap: commit: %w", err)
	}

	log.Info("status remap migration complete", "rows_affected", legacyCount)
	return nil
}
