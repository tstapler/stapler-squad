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

// backfillEnabledField is a one-time migration step for the dedicated Enabled field
// (webhook-triggers verify follow-ups, AC0-3).
//
// Existing Workflow rows predate the enabled field. ent's additive auto-migration
// backfills the new column with its schema default (true) for EVERY existing row,
// uniformly, regardless of what cron_enabled held — because field.Bool("enabled").
// Default(true) applies at ALTER TABLE time. Before this field existed, cron_enabled
// was the ONLY way to express "disabled" for any trigger type (see
// validateTriggerTypeFieldConsistency's doc comment). Left uncorrected, a pre-existing
// row that an operator had explicitly disabled via cron_enabled=false would silently
// come back enabled the moment the webhook handlers switch from gating on CronEnabled
// to gating on Enabled.
//
// This scans every Workflow row and corrects enabled to false wherever it's still at
// the migration-artifact default (true) but cron_enabled is false — i.e. a row that
// predates enabled entirely and was, under the old overloaded semantics, disabled. It
// deliberately does NOT touch a row already at enabled=false (already correct, possibly
// already backfilled by a prior run) or one with cron_enabled=true (nothing to correct
// — true is also the right value there). Mirrors backfillTriggerTypes's shape and
// carries the same accepted, narrow limitation: a row an operator explicitly sets to
// enabled=true while leaving cron_enabled=false, between this migration landing and the
// next server restart, would be flipped back to disabled by that later restart. This
// case cannot be distinguished from an unmigrated row using only these two fields, and
// this codebase's existing migration (backfillTriggerTypes) accepts the same class of
// risk for the identical reason.
func backfillEnabledField(ctx context.Context, repo session.WorkflowRepository) {
	if repo == nil {
		return
	}

	wfs, err := repo.ListAll(ctx)
	if err != nil {
		log.Error("[WorkflowScheduler] enabled backfill: failed to list workflows", "err", err)
		return
	}

	backfilled := 0
	for _, wf := range wfs {
		if !wf.Enabled || wf.CronEnabled {
			continue
		}
		enabled := false
		if _, updateErr := repo.Update(ctx, wf.ID, session.WorkflowUpdateInput{Enabled: &enabled}); updateErr != nil {
			log.Warn("[WorkflowScheduler] enabled backfill: failed to update workflow",
				"slug", wf.Slug, "err", updateErr)
			continue
		}
		backfilled++
	}
	if backfilled > 0 {
		log.Info("[WorkflowScheduler] enabled backfill complete", "count", backfilled)
	}
}
