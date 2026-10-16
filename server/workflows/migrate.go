package workflows

import (
	"context"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// backfillTriggerTypes is a one-time migration step (webhook-triggers plan Task 1.1.1d).
//
// Existing Workflow rows predate the trigger_type field. ent's additive auto-migration
// backfills the new column with its schema default ("manual") for EVERY existing row —
// regardless of whether cron_enabled is set — because field.String("trigger_type").
// Default("manual") applies uniformly at ALTER TABLE time (confirmed via the generated
// Default: "manual" entry in session/ent/migrate/schema.go). Left uncorrected, a
// pre-existing cron-enabled Workflow would silently stop firing the moment the
// cron-registration gate is tightened to also require trigger_type == "cron"
// (Task 1.1.1e / scheduler.go's addCronEntry), since ent's own migration would have
// already stamped it "manual", not "cron".
//
// This scans every Workflow row and corrects trigger_type to "cron" wherever
// cron_enabled is true and trigger_type is still at its migration-artifact default
// value ("" or ent's schema default "manual") — i.e. a row that predates trigger_type
// entirely. It deliberately does NOT touch a row whose trigger_type is already some
// other explicit, non-default value (e.g. "webhook", "github_push") even if
// cron_enabled also happens to be true on that row: that combination is a genuine
// save-time mismatch (Task 1.1.1e), not a migration artifact, and silently
// "correcting" it here would mask the mismatch rather than surface it — the tightened
// cron-registration gate (addCronEntry) is what's supposed to refuse to register that
// row, not this backfill.
func backfillTriggerTypes(ctx context.Context, repo session.WorkflowRepository) {
	if repo == nil {
		return
	}

	wfs, err := repo.ListAll(ctx)
	if err != nil {
		log.Error("[WorkflowScheduler] trigger_type backfill: failed to list workflows", "err", err)
		return
	}

	backfilled := 0
	for _, wf := range wfs {
		if !wf.CronEnabled || wf.TriggerType == "cron" {
			continue
		}
		if wf.TriggerType != "" && wf.TriggerType != "manual" {
			// Explicit non-default trigger_type conflicting with cron_enabled=true —
			// a real mismatch, not a migration artifact. Leave it for the
			// cron-registration gate to refuse, not this backfill to paper over.
			continue
		}
		triggerType := "cron"
		if _, updateErr := repo.Update(ctx, wf.ID, session.WorkflowUpdateInput{TriggerType: &triggerType}); updateErr != nil {
			log.Warn("[WorkflowScheduler] trigger_type backfill: failed to update workflow",
				"slug", wf.Slug, "err", updateErr)
			continue
		}
		backfilled++
	}
	if backfilled > 0 {
		log.Info("[WorkflowScheduler] trigger_type backfill complete", "count", backfilled)
	}
}
