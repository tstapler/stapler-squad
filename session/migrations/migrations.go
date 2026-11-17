// Package migrations holds the ent-only startup data migrations that
// session.NewEntRepository runs. Adding one is a new file here plus one entry
// in Default; session is not edited.
package migrations

import (
	"context"

	"entgo.io/ent/dialect"

	"github.com/tstapler/stapler-squad/session/ent"
)

// Env is what a migration may touch. The concrete ent types are deliberate:
// this package is ent-bound infrastructure.
type Env struct {
	Client *ent.Client
	// Driver is nil when the repository was built from a bare client.
	Driver dialect.Driver
	// BackfillPublicIDs needs session-side backlog helpers, so session supplies it.
	BackfillPublicIDs func(ctx context.Context) error
}

// Migration is idempotent and safe to run on every startup.
type Migration struct {
	Name string
	Run  func(ctx context.Context, env Env) error
}

// PRURLFunc builds a PR URL for a session row, reporting false when the
// host cannot be recovered from path.
type PRURLFunc func(path, owner, repo string, prNumber int) (string, bool)

// Hooks are the session-owned behaviours some migrations need.
type Hooks struct {
	PRURL               PRURLFunc
	ResolveMainRepoRoot func(repoPath string) (string, error)
}

// Default lists every startup migration in run order. The order is not
// load-bearing between entries (each is independent), but it is pinned by
// session's golden test.
func Default(h Hooks) []Migration {
	return []Migration{
		{"drop obsolete indexes", DropObsoleteIndexes}, // first: cheap, and later migrations shell out to git per item
		{"backlog item updated_at UTC backfill", BacklogItemUpdatedAtUTC},
		{"workflow updated_at UTC backfill", WorkflowUpdatedAtUTC},
		{"github pr url backfill", func(ctx context.Context, env Env) error { return PRURLBackfill(ctx, env, h.PRURL) }},
		{"backlog item public id backfill", func(ctx context.Context, env Env) error { return env.BackfillPublicIDs(ctx) }},
		{"backlog item repo_path canonicalization", func(ctx context.Context, env Env) error {
			return BacklogItemRepoPathCanonicalization(ctx, env, h.ResolveMainRepoRoot)
		}},
		{"status corruption repair", StatusCorruptionRepair},
		{"backlog item legacy import external backfill", BacklogItemLegacyImportExternal},
	}
}
