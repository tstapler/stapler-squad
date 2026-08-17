package session

// workflow_enabled_field_migration.go — one-time migration step for the
// dedicated Workflow.enabled field (webhook-triggers verify follow-ups,
// AC0-3).
//
// Existing Workflow rows predate the enabled field. ent's additive
// auto-migration backfills the new column with its schema default (true) for
// EVERY existing row, uniformly, regardless of what cron_enabled held —
// because field.Bool("enabled").Default(true) applies at ALTER TABLE time.
// Before this field existed, cron_enabled was the ONLY way to express
// "disabled" for any trigger type. Left uncorrected, a pre-existing row that
// an operator had explicitly disabled via cron_enabled=false would silently
// come back enabled the moment the webhook handlers switch from gating on
// CronEnabled to gating on Enabled.
//
// UNLIKE backfillTriggerTypes (server/workflows/migrate.go) — whose analogous
// correction condition ("trigger_type is still at its migration-artifact
// default") is naturally exhausted after the first correct write, since a
// corrected row's trigger_type becomes "cron", never again matching the
// artifact-default check — this field has no such self-limiting condition.
// "enabled=true, cron_enabled=false" is BOTH the migration-artifact state
// AND the permanent, correct steady state for any legitimately-enabled
// webhook/github_push trigger created *after* this field ships. A backfill
// keyed only on current field values would therefore re-disable every such
// trigger on every subsequent server restart, forever — confirmed as a real
// bug during sdd:6-verify's architecture review, not a hypothetical.
//
// The fix: gate the backfill on whether the enabled column *itself* already
// existed before this process's own schema.Create() call — a signal that is
// true exactly once per database (the run where ent's auto-migration first
// adds the column) and never again, regardless of what values later land in
// the column. workflowEnabledColumnPreexisted must be called with the raw
// *sql.DB BEFORE client.Schema.Create() runs (session/ent_repository.go),
// since that call is what adds the column this check needs to still be
// absent.
//
// Known residual limitation (accepted, not fixed): if the process crashes
// between schema.Create() adding the column and this backfill completing,
// the retry's workflowEnabledColumnPreexisted check will see the column as
// already present and skip the backfill, leaving any not-yet-corrected
// legacy rows uncorrected. Given this only matters for a narrow crash
// window on a single-writer personal-scale database, this is judged an
// acceptable tradeoff against the alternative (a persisted
// migration-completion marker) for the risk/complexity this repo operates
// at — revisit if that assumption changes.

import (
	"context"
	"database/sql"

	"github.com/tstapler/stapler-squad/log"
)

// workflowEnabledColumnPreexisted reports whether the workflows table already
// has an "enabled" column, checked via PRAGMA table_info directly against db
// — must be called before client.Schema.Create() runs. Returns false (the
// safe default: skip the backfill) if the table doesn't exist yet (fresh DB)
// or the check itself fails for any reason.
func workflowEnabledColumnPreexisted(db *sql.DB) bool {
	rows, err := db.Query(`PRAGMA table_info(workflows)`)
	if err != nil {
		return false
	}
	defer rows.Close()

	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false
		}
		if name == "enabled" {
			return true
		}
	}
	return false
}

// runWorkflowEnabledFieldBackfill scans every Workflow row and corrects
// enabled to false wherever it's still at the migration-artifact default
// (true) but cron_enabled is false — i.e. a row that predates enabled
// entirely and was, under the old overloaded semantics, disabled. Only call
// when enabledColumnPreexisted is false (see workflowEnabledColumnPreexisted)
// — the caller is responsible for that gating, this function does not
// re-check it. Deliberately does NOT touch a row already at enabled=false or
// one with cron_enabled=true. Best-effort per row: a single row's save
// failure is logged and does not abort the rest.
func runWorkflowEnabledFieldBackfill(ctx context.Context, er *EntRepository) {
	wfs, err := er.client.Workflow.Query().All(ctx)
	if err != nil {
		// Table may not exist yet (fresh DB before schema.Create) — ignore,
		// mirroring this file's sibling migrations' defensive posture.
		return
	}

	var migrated int
	for _, wf := range wfs {
		if !wf.Enabled || wf.CronEnabled {
			continue
		}
		if _, saveErr := er.client.Workflow.UpdateOneID(wf.ID).
			SetEnabled(false).
			Save(ctx); saveErr != nil {
			log.WarningLog.Printf("[Migration] workflow enabled backfill: workflow=%s: %v", wf.ID, saveErr)
			continue
		}
		migrated++
	}
	if migrated > 0 {
		log.InfoLog.Printf("[Migration] workflow enabled backfill: corrected %d row(s)", migrated)
	}
}
