package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunBacklogItemRepoPathCanonicalizationBackfill_should_CanonicalizeWorktreePath_When_Called
// is the automatic-migration regression test: an item filed before RepoPath
// canonicalization existed (an agent's own in-progress worktree stored
// verbatim) must have its RepoPath redirected to the main repo root — the
// same effect Storage.CreateBacklogItem now applies at creation time — so it
// converges into the same web UI "group by repository" bucket as items
// filed against the main checkout directly.
func TestRunBacklogItemRepoPathCanonicalizationBackfill_should_CanonicalizeWorktreePath_When_Called(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	mainDir := t.TempDir()
	mainRepo, err := git.PlainInit(mainDir, false)
	require.NoError(t, err)
	wt, err := mainRepo.Worktree()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "f.txt"), []byte("x"), 0644))
	_, err = wt.Add("f.txt")
	require.NoError(t, err)
	_, err = wt.Commit("init", &git.CommitOptions{Author: &object.Signature{Name: "t", Email: "t@example.com"}})
	require.NoError(t, err)

	worktreeDir := filepath.Join(t.TempDir(), "agent-worktree")
	runGitOrFail(t, mainDir, "worktree", "add", "-b", "agent-branch", worktreeDir)

	created, err := repo.client.BacklogItem.Create().
		SetTitle("item filed from an agent's own worktree").
		SetStatus("idea").
		SetRepoPath(worktreeDir).
		Save(ctx)
	require.NoError(t, err)

	require.NoError(t, runBacklogItemRepoPathCanonicalizationBackfill(ctx, repo))

	migrated, err := repo.client.BacklogItem.Get(ctx, created.ID)
	require.NoError(t, err)
	wantMain, err := filepath.EvalSymlinks(mainDir)
	require.NoError(t, err)
	gotRepoPath, err := filepath.EvalSymlinks(migrated.RepoPath)
	require.NoError(t, err)
	assert.Equal(t, wantMain, gotRepoPath, "backfill must canonicalize the worktree path to the main repo root")
}

// TestRunBacklogItemRepoPathCanonicalizationBackfill_should_BeIdempotent_When_RunTwice
// proves a second run is a safe no-op — already-canonical rows (including
// ones the first backfill run just canonicalized, and freshly-created rows
// under the fixed creation-time behavior) are left untouched.
func TestRunBacklogItemRepoPathCanonicalizationBackfill_should_BeIdempotent_When_RunTwice(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	mainDir := t.TempDir()
	_, err := git.PlainInit(mainDir, false)
	require.NoError(t, err)

	created, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "already-canonical item", RepoPath: mainDir})
	require.NoError(t, err)

	require.NoError(t, runBacklogItemRepoPathCanonicalizationBackfill(ctx, repo))
	require.NoError(t, runBacklogItemRepoPathCanonicalizationBackfill(ctx, repo))

	final, err := repo.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.RepoPath, final.RepoPath, "idempotent backfill must not perturb an already-canonical row")
}

// TestRunBacklogItemRepoPathCanonicalizationBackfill_should_SkipEmptyAndRelativeRepoPath
// verifies the same guardrail TriggerTriage's own repo_path validation relies
// on: a relative/bare-slug RepoPath (a caller mistake, not a real path) must
// never be silently resolved via filepath.Abs — left untouched instead, same
// as Storage.CreateBacklogItem's creation-time gate.
func TestRunBacklogItemRepoPathCanonicalizationBackfill_should_SkipEmptyAndRelativeRepoPath(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	emptyItem, err := repo.client.BacklogItem.Create().
		SetTitle("repo-less item").
		SetStatus("idea").
		Save(ctx)
	require.NoError(t, err)

	relativeItem, err := repo.client.BacklogItem.Create().
		SetTitle("bare-slug repo_path item").
		SetStatus("idea").
		SetRepoPath("stapler-squad").
		Save(ctx)
	require.NoError(t, err)

	require.NoError(t, runBacklogItemRepoPathCanonicalizationBackfill(ctx, repo))

	gotEmpty, err := repo.client.BacklogItem.Get(ctx, emptyItem.ID)
	require.NoError(t, err)
	assert.Equal(t, "", gotEmpty.RepoPath)

	gotRelative, err := repo.client.BacklogItem.Get(ctx, relativeItem.ID)
	require.NoError(t, err)
	assert.Equal(t, "stapler-squad", gotRelative.RepoPath, "a relative repo_path must be left untouched, not silently resolved via filepath.Abs")
}

// TestRunBacklogItemRepoPathCanonicalizationBackfill_should_SkipNonExistentPath_WithoutInvokingGit
// verifies the os.Stat pre-filter: a repo_path that no longer exists on disk
// (routine here — worktrees get pruned once their session archives) must be
// skipped without ever invoking ResolveMainRepoRoot's git subprocess call,
// not just left unchanged as a side effect of that call failing.
func TestRunBacklogItemRepoPathCanonicalizationBackfill_should_SkipNonExistentPath_WithoutInvokingGit(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	missingPath := filepath.Join(t.TempDir(), "does-not-exist-anymore")
	created, err := repo.client.BacklogItem.Create().
		SetTitle("item whose worktree was already cleaned up").
		SetStatus("idea").
		SetRepoPath(missingPath).
		Save(ctx)
	require.NoError(t, err)

	require.NoError(t, runBacklogItemRepoPathCanonicalizationBackfill(ctx, repo))

	got, err := repo.client.BacklogItem.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, missingPath, got.RepoPath, "a repo_path that no longer exists on disk must be left untouched")
}
