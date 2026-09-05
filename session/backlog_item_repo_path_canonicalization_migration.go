package session

// backlog_item_repo_path_canonicalization_migration.go — idempotent migration
// that canonicalizes every BacklogItem's RepoPath to its main repo root, the
// same redirect Storage.CreateBacklogItem (session/storage.go) now applies at
// creation time via ResolveMainRepoRoot.
//
// Nothing upstream ever guaranteed a stored RepoPath was the main checkout
// rather than a worktree — an agent filing an item routinely passed its own
// in-progress worktree as repo_path — so items filed before that creation-time
// fix are stuck with a fragmented RepoPath: different items targeting the
// same repo end up with different literal paths (one per worktree the filing
// agent happened to be in), which (a) fragments the web UI's "group by
// repository" view into one bucket per worktree instead of one per repo, and
// (b) previously caused TriggerTriage's isolated worktree to fork from that
// stale worktree's branch tip instead of the repo's default branch.
//
// This must run automatically, not as a manually-invoked CLI command: this
// project is used by people (e.g. a TPM organizing their own work) who are
// not going to run a database-path-aware maintenance command by hand. Mirrors
// this file's sibling backfills' (see backlog_item_updated_at_utc_migration.go)
// unconditional-on-every-startup pattern rather than
// workflow_enabled_field_migration.go's one-time-only column-existence gate:
// unlike that field's non-self-limiting correction condition, an
// already-canonical RepoPath (including a freshly created item's) is a
// stable steady state that ResolveMainRepoRoot will keep resolving to itself.
//
// Unlike this file's sibling backfills, "already canonical" isn't a free
// in-memory check — ResolveMainRepoRoot shells out to git. An os.Stat guard
// (below) skips that subprocess call entirely for the common case of a
// repo_path that no longer exists on disk (routine here: worktrees get
// pruned once their session archives), so this stays cheap in practice, but
// it is not literally free the way e.g. the updated_at-UTC backfill's
// time.Time.Location() comparison is.

import (
	"context"
	"os"
	"path/filepath"

	"github.com/tstapler/stapler-squad/log"
)

// runBacklogItemRepoPathCanonicalizationBackfill re-resolves every
// BacklogItem's RepoPath via ResolveMainRepoRoot, updating rows whose stored
// path is a linked worktree (or otherwise resolves to a different main repo
// root) in place. Idempotent: an empty, relative, no-longer-existing, or
// already-canonical RepoPath is left untouched — a no-op call costs one query
// plus, for each row with a repo_path that still exists on disk, one
// best-effort git subprocess call. Best-effort per row: a single row's
// resolve or save failure is logged and does not abort the rest, mirroring
// this file's sibling backfills' discipline.
func runBacklogItemRepoPathCanonicalizationBackfill(ctx context.Context, er *EntRepository) error {
	//nolint:entfullscan idempotent startup backfill canonicalizing every BacklogItem's repo_path; safe to re-run on every startup (see file doc comment).
	items, err := er.client.BacklogItem.Query().All(ctx)
	if err != nil {
		// Table may not exist yet (fresh DB before schema.Create) — ignore,
		// mirroring this file's sibling migrations' defensive posture.
		return nil //nolint:nilerr
	}

	var migrated int
	for _, item := range items {
		if item.RepoPath == "" || !filepath.IsAbs(item.RepoPath) {
			continue
		}
		// Cheap pre-filter before the git subprocess call below: a path that
		// no longer exists can never resolve to anything (ResolveMainRepoRoot
		// would just fail and fall through unchanged), so skip it with a
		// plain stat() rather than paying for git rev-parse + its timeout.
		if _, statErr := os.Stat(item.RepoPath); statErr != nil {
			continue
		}
		resolved, resolveErr := ResolveMainRepoRoot(item.RepoPath)
		if resolveErr != nil || resolved == item.RepoPath {
			continue
		}
		if _, saveErr := er.client.BacklogItem.UpdateOneID(item.ID).
			SetRepoPath(resolved).
			Save(ctx); saveErr != nil {
			log.WarningLog().Printf("[Migration] backlog item repo_path canonicalization: item=%s: %v", item.ID, saveErr)
			continue
		}
		migrated++
	}
	if migrated > 0 {
		log.InfoLog().Printf("[Migration] backlog item repo_path canonicalization: canonicalized %d row(s)", migrated)
	}
	return nil
}

// backlogItemRepoPathCanonicalizationMigration adapts
// runBacklogItemRepoPathCanonicalizationBackfill to the Migration interface
// (session/ent_repository_migrations.go).
type backlogItemRepoPathCanonicalizationMigration struct{}

func (backlogItemRepoPathCanonicalizationMigration) Name() string {
	return "backlog item repo_path canonicalization"
}

func (backlogItemRepoPathCanonicalizationMigration) Run(ctx context.Context, er *EntRepository) error {
	return runBacklogItemRepoPathCanonicalizationBackfill(ctx, er)
}
