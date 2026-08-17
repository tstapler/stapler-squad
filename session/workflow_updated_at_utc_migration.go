package session

// workflow_updated_at_utc_migration.go — one-time idempotent migration that
// normalizes Workflow.updated_at to UTC for rows written before the fix in
// session/ent/schema/workflow.go (Default/UpdateDefault changed from bare
// time.Now, which returns Local, to time.Now().UTC()).
//
// Mirrors backlog_item_updated_at_utc_migration.go exactly — see that file's
// doc comment for the full mechanism. This one exists because
// UpdateWorkflowRequest.expected_updated_at (webhook-triggers verify
// follow-ups AC9) is the same class of protobuf-Timestamp-derived CAS
// precondition: a Local-formatted stored value could never byte-match a
// UTC-zoned precondition, making the CAS unconditionally fail for any row
// that predates this fix.

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// runWorkflowUpdatedAtUTCBackfill re-saves every Workflow whose stored
// updated_at isn't already UTC, normalizing it in place. Idempotent: rows
// already in UTC (including freshly-created ones, and rows already migrated
// by a prior run) are skipped. Safe to call on a fresh/empty database.
// Best-effort per row: a single row's save failure is logged and does not
// abort the rest.
func runWorkflowUpdatedAtUTCBackfill(ctx context.Context, er *EntRepository) error {
	wfs, err := er.client.Workflow.Query().All(ctx)
	if err != nil {
		// Table may not exist yet (fresh DB before schema.Create) — ignore,
		// mirroring runBacklogItemUpdatedAtUTCBackfill's same defensive posture.
		return nil //nolint:nilerr
	}

	var migrated int
	for _, wf := range wfs {
		if wf.UpdatedAt.Location() == time.UTC {
			continue
		}
		if _, saveErr := er.client.Workflow.UpdateOneID(wf.ID).
			SetUpdatedAt(wf.UpdatedAt.UTC()).
			Save(ctx); saveErr != nil {
			log.WarningLog.Printf("[Migration] workflow updated_at UTC backfill: workflow=%s: %v", wf.ID, saveErr)
			continue
		}
		migrated++
	}
	if migrated > 0 {
		log.InfoLog.Printf("[Migration] workflow updated_at UTC backfill: normalized %d row(s)", migrated)
	}
	return nil
}
