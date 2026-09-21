package session

// ent_repository_migrations.go — the uniform-shaped subset of NewEntRepository's
// startup migrations (session/ent_repository.go), run via a single loop instead
// of one hand-copied if-err-block per migration.
//
// One migration step does NOT implement Migration and stays as an explicit,
// separately-called exception in NewEntRepository, in this exact order:
//  1. client.Schema.Create() — ent's own schema DDL, not a data migration, and
//     must run before every migration below (several depend on columns it adds).
//  2. workflowEnabledColumnPreexisted(db) + the gated call to
//     runWorkflowEnabledFieldBackfill — the "preexisted" check must be captured
//     from the raw *sql.DB BEFORE Schema.Create() runs (see
//     workflow_enabled_field_migration.go's doc comment), so it cannot be
//     expressed as a post-schema Run(ctx, er) step.
//
// Forcing this exception into the interface for uniformity's sake would need
// widening Migration with a pre-schema hook that every other migration would
// ignore — needless interface bloat for one outlier out of the total.
//
// A former third exception, runStatusRemap, was removed entirely (not just
// excluded from this list): its one-time job — remapping an old 7-value
// session.Status iota to the 5-value iota introduced by the hibernation
// feature — completed long ago, but its "is this legacy data" check
// (status > 4) never accounted for session.Status growing past 5 values
// since (Restoring=5, Crashed=6, PermanentlyFailed=7, Failed=8+ are all
// legitimate, currently-persisted states). Because it ran unconditionally on
// every NewEntRepository call, any session sitting in one of those states at
// restart time got its status value corrupted by a compounding +100
// sentinel shift the migration's CASE expression never mapped back down —
// repeated restarts drove the stored value arbitrarily high (observed
// status=66707 in production), which then failed every subsequent
// transition-to-Active check. See git history for the removed file if the
// original migration logic is ever needed for reference. Removing it
// stopped further corruption but left already-corrupted rows behind
// (confirmed still occurring after the removal: status 207/334507/384507,
// all ≡7 mod 100) — the "status corruption repair" entry below
// (status_corruption_repair.go) is the one-time cleanup for exactly those
// rows.

import "context"

type Migration interface {
	// Name identifies the migration in log output and error wrapping.
	Name() string
	// Run performs the migration. Must be idempotent and safe to call on
	// every process startup — see each concrete migration's own doc comment
	// for its specific idempotency argument.
	Run(ctx context.Context, er *EntRepository) error
}

// funcMigration adapts a plain `func(ctx, *EntRepository) error` to the
// Migration interface. Every migration below is this same shape — a name
// plus a run function — so a hand-written adapter type with its own
// Name()/Run() methods per migration (the pattern every migration file used
// before this) was pure duplication with nothing migration-specific in it.
// Adding a new migration is now: write `func runXBackfill(ctx
// context.Context, er *EntRepository) error` (plus its test), and append
// `funcMigration{"x backfill", runXBackfill}` to startupMigrations below —
// no new type, no boilerplate methods.
type funcMigration struct {
	name string
	fn   func(ctx context.Context, er *EntRepository) error
}

func (m funcMigration) Name() string { return m.name }

func (m funcMigration) Run(ctx context.Context, er *EntRepository) error {
	return m.fn(ctx, er)
}

// startupMigrations lists every Migration NewEntRepository runs, in the
// (here, non-load-bearing — each is independent) order they were added.
// NewEntRepository's body itself never needs to change when adding one —
// see funcMigration's doc comment above for what adding a migration
// actually takes.
var startupMigrations = []Migration{ //nolint:gochecknoglobals
	funcMigration{"backlog item updated_at UTC backfill", runBacklogItemUpdatedAtUTCBackfill},
	funcMigration{"workflow updated_at UTC backfill", runWorkflowUpdatedAtUTCBackfill},
	funcMigration{"github pr url backfill", runGitHubPRURLBackfill},
	funcMigration{"backlog item public id backfill", func(ctx context.Context, er *EntRepository) error {
		return er.BackfillBacklogItemPublicIDs(ctx)
	}},
	funcMigration{"backlog item repo_path canonicalization", runBacklogItemRepoPathCanonicalizationBackfill},
	funcMigration{"status corruption repair", runStatusCorruptionRepair},
}
