package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupDiffHashTestRepo and the addCommitForDiffHashTest helper are self-contained
// test fixtures (not a shared helper import from session/git's test file, which is
// package-private to session/git) — this test only needs a minimal on-disk repo,
// not the fuller clone/branch fixtures ops_test.go uses.
//
// Uses go-git directly rather than shelling out — see
// .claude/rules/prefer-go-git-over-subshells.md.
func addCommitForDiffHashTest(t *testing.T, repo *gogit.Repository, path, message string) string {
	t.Helper()
	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add(path)
	require.NoError(t, err)
	hash, err := wt.Commit(message, &gogit.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@example.com", When: time.Now()},
	})
	require.NoError(t, err)
	return hash.String()
}

func setupDiffHashTestRepo(t *testing.T) (repoPath string, baseSHA string) {
	t.Helper()
	dir := t.TempDir()
	repo, err := gogit.PlainInit(dir, false)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("init\n"), 0o644))
	sha := addCommitForDiffHashTest(t, repo, "README.md", "init")
	return dir, sha
}

// TestComputeCurrentDiffHash_should_returnNonEmptyHash_When_CompletedWorkSessionHasValidCommitRange
// is the AC3 write-site round trip: a completed work session with a real base..head
// commit range must produce a non-empty DiffHash, computed via go-git
// (git.DiffHashBetween), not a git subshell.
func TestComputeCurrentDiffHash_should_returnNonEmptyHash_When_CompletedWorkSessionHasValidCommitRange(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)
	ctx := context.Background()

	repoPath, baseSHA := setupDiffHashTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "foo.go"), []byte("package foo\n"), 0o644))
	gitRepo, err := gogit.PlainOpen(repoPath)
	require.NoError(t, err)
	headSHA := addCommitForDiffHashTest(t, gitRepo, "foo.go", "add foo.go")

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{Title: "diffhash item", RepoPath: repoPath})
	require.NoError(t, err)

	is, err := storage.CreateItemSession(ctx, ItemSessionData{ItemID: item.ID, SessionUUID: "work-1", SessionRole: SessionRoleWork})
	require.NoError(t, err)
	require.NoError(t, storage.SetItemSessionBaseCommit(ctx, is.ID, baseSHA))
	require.NoError(t, storage.UpdateItemSessionGitActivity(ctx, is.ID, headSHA, "add foo.go", time.Now(), 1))
	require.NoError(t, storage.UpdateItemSessionEnded(ctx, is.ID, time.Now()))

	hash := storage.ComputeCurrentDiffHash(ctx, item.ID)
	assert.NotEmpty(t, hash, "a completed work session with a real commit range must produce a non-empty hash")
}

// TestComputeCurrentDiffHash_should_returnEmptyString_When_NoCompletedWorkSession verifies
// the best-effort fallback: no completed work session at all must return "" rather than
// erroring or panicking, so it never blocks the review-verdict write it's attached to.
func TestComputeCurrentDiffHash_should_returnEmptyString_When_NoCompletedWorkSession(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)
	ctx := context.Background()

	repoPath, _ := setupDiffHashTestRepo(t)
	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{Title: "no session item", RepoPath: repoPath})
	require.NoError(t, err)

	hash := storage.ComputeCurrentDiffHash(ctx, item.ID)
	assert.Empty(t, hash, "no completed work session means there is no diff to hash")
}

// TestGetRecentReviewVerdictSummaries_should_roundTripDiffHash_When_VerdictSaved is
// AC4: DiffHash must not be silently dropped between SaveReviewVerdict and
// GetRecentReviewVerdictSummaries — IsFlakyVerdictFlipFlop reads it straight off this
// query's result with no separate lookup.
func TestGetRecentReviewVerdictSummaries_should_roundTripDiffHash_When_VerdictSaved(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{Title: "round trip item"})
	require.NoError(t, err)

	is, err := storage.CreateItemSession(ctx, ItemSessionData{ItemID: item.ID, SessionUUID: "review-1", SessionRole: SessionRoleReview})
	require.NoError(t, err)

	require.NoError(t, storage.SaveReviewVerdict(ctx, is.ID, ReviewVerdictData{
		OverallOutcome: ReviewOutcomeFail,
		Summary:        "diff hash round trip",
		DiffHash:       "deadbeef",
	}))

	recent, err := storage.GetRecentReviewVerdictSummaries(ctx, item.ID, 2)
	require.NoError(t, err)
	require.Len(t, recent, 1)
	assert.Equal(t, "deadbeef", recent[0].DiffHash, "DiffHash must round-trip through GetRecentReviewVerdictSummaries")
}
