package session

// ent_repository_migrations.go — the uniform-shaped subset of NewEntRepository's
// startup migrations (session/ent_repository.go), run via a single loop instead
// of one hand-copied if-err-block per migration.
//
// Two migration steps do NOT implement Migration and stay as explicit,
// separately-called exceptions in NewEntRepository, in this exact order:
//  1. client.Schema.Create() — ent's own schema DDL, not a data migration, and
//     must run before every migration below (several depend on columns it adds).
//  2. workflowEnabledColumnPreexisted(db) + the gated call to
//     runWorkflowEnabledFieldBackfill — the "preexisted" check must be captured
//     from the raw *sql.DB BEFORE Schema.Create() runs (see
//     workflow_enabled_field_migration.go's doc comment), so it cannot be
//     expressed as a post-schema Run(ctx, er) step. runStatusRemap also needs
//     the raw *sql.DB rather than the ent client EntRepository wraps, for the
//     same reason it's excluded.
//
// Forcing either exception into this interface for uniformity's sake would
// need widening Migration with a pre-schema hook or a raw-*sql.DB parameter
// that every other migration would ignore — needless interface bloat for two
// outliers out of seven total migrations.

import "context"

type Migration interface {
	// Name identifies the migration in log output and error wrapping.
	Name() string
	// Run performs the migration. Must be idempotent and safe to call on
	// every process startup — see each concrete migration's own doc comment
	// for its specific idempotency argument.
	Run(ctx context.Context, er *EntRepository) error
}

// startupMigrations lists every Migration NewEntRepository runs, in the
// (here, non-load-bearing — each is independent) order they were added.
// Adding a new uniform-shaped migration means writing its file exactly like
// today (a small `run...Backfill(ctx, er) error` function plus tests) and
// appending one adapter value here — NewEntRepository's body itself never
// needs to change.
var startupMigrations = []Migration{ //nolint:gochecknoglobals
	backlogItemUpdatedAtUTCMigration{},
	workflowUpdatedAtUTCMigration{},
	gitHubPRURLBackfillMigration{},
	backlogItemPublicIDMigration{},
	backlogItemRepoPathCanonicalizationMigration{},
}
