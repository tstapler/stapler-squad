package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/git"
)

// attachWorkSessionWithCommit creates a work ItemSession for item, records
// sha as its last commit, and registers worktree data (repoPath, branchName)
// so GetBacklogItemShipStatus can resolve both the commit and the branch.
func attachWorkSessionWithCommit(t *testing.T, storage *session.Storage, repo *session.EntRepository, itemID, sessionUUID, repoPath, worktreePath, branchName, sha string) {
	t.Helper()
	is, err := storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      itemID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	require.NoError(t, repo.UpdateItemSessionGitActivity(t.Context(), is.ID, sha, "work commit", time.Now(), 1))

	now := time.Now()
	require.NoError(t, repo.Create(t.Context(), session.InstanceData{
		Title:      sessionUUID,
		UUID:       sessionUUID,
		Path:       worktreePath,
		WorkingDir: worktreePath,
		Branch:     branchName,
		Status:     session.Paused,
		Program:    "claude",
		CreatedAt:  now,
		UpdatedAt:  now,
		Worktree: session.GitWorktreeData{
			RepoPath:      repoPath,
			WorktreePath:  worktreePath,
			SessionName:   sessionUUID,
			BranchName:    branchName,
			BaseCommitSHA: sha,
		},
	}))
}

// TestGetBacklogItemShipStatus_should_ReportShippedDirect_When_CommitOnMainNoPrURL
// covers the "committed directly to main, no PR" case — the shipped_via must read
// "direct", and since the branch is main itself, ahead/behind must both be 0.
func TestGetBacklogItemShipStatus_should_ReportShippedDirect_When_CommitOnMainNoPrURL(t *testing.T) {
	_, repoPath := setupPRFixSyncRepo(t)
	sha := strings.TrimSpace(runGitTestCmd(t, repoPath, "rev-parse", "HEAD"))

	storage, repo := createTestStorageWithRepo(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "direct commit item",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusDone),
	})
	require.NoError(t, err)
	attachWorkSessionWithCommit(t, storage, repo, item.ID, "direct-work", repoPath, repoPath, "main", sha)

	resp, err := svc.GetBacklogItemShipStatus(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemShipStatusRequest{ItemId: item.ID}))
	require.NoError(t, err)
	st := resp.Msg.Status
	assert.Empty(t, st.Error)
	assert.True(t, st.Shipped)
	assert.Equal(t, "direct", st.ShippedVia)
	assert.Equal(t, sha, st.LastCommitSha)
}

// TestGetBacklogItemShipStatus_should_ReportShippedViaPr_When_PrUrlSetAndMerged
// covers the "opened a PR, it got merged" case — shipped_via must read "pr", and the
// branch (now merged into main) must report zero ahead, since main already contains it.
func TestGetBacklogItemShipStatus_should_ReportShippedViaPr_When_PrUrlSetAndMerged(t *testing.T) {
	_, repoPath := setupPRFixSyncRepo(t)

	runGitTestCmd(t, repoPath, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "feature.txt"), []byte("work\n"), 0o644))
	runGitTestCmd(t, repoPath, "add", "feature.txt")
	runGitTestCmd(t, repoPath, "commit", "-m", "feature work")
	sha := strings.TrimSpace(runGitTestCmd(t, repoPath, "rev-parse", "HEAD"))
	runGitTestCmd(t, repoPath, "checkout", "main")
	runGitTestCmd(t, repoPath, "merge", "--no-ff", "--no-edit", "feature") // simulates a merged PR

	storage, repo := createTestStorageWithRepo(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "PR-shipped item",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusDone),
	})
	require.NoError(t, err)
	prURL := "https://github.com/example/repo/pull/42"
	_, err = storage.UpdateBacklogItem(t.Context(), item.ID, session.BacklogItemUpdate{PrURL: &prURL}, nil)
	require.NoError(t, err)
	attachWorkSessionWithCommit(t, storage, repo, item.ID, "pr-work", repoPath, repoPath, "feature", sha)

	resp, err := svc.GetBacklogItemShipStatus(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemShipStatusRequest{ItemId: item.ID}))
	require.NoError(t, err)
	st := resp.Msg.Status
	assert.Empty(t, st.Error)
	assert.True(t, st.Shipped)
	assert.Equal(t, "pr", st.ShippedVia)
	assert.True(t, st.BranchExists, "branch still exists locally in this test repo")
	assert.Equal(t, int32(0), st.AheadOfMain, "main already contains the merged feature branch")
}

// TestGetBacklogItemShipStatus_should_ReportNotShipped_When_BranchNeverMerged covers
// the exact regression case: a PR URL is set but the branch was never actually merged.
func TestGetBacklogItemShipStatus_should_ReportNotShipped_When_BranchNeverMerged(t *testing.T) {
	_, repoPath := setupPRFixSyncRepo(t)

	runGitTestCmd(t, repoPath, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "feature.txt"), []byte("work\n"), 0o644))
	runGitTestCmd(t, repoPath, "add", "feature.txt")
	runGitTestCmd(t, repoPath, "commit", "-m", "feature work, never merged")
	sha := strings.TrimSpace(runGitTestCmd(t, repoPath, "rev-parse", "HEAD"))

	storage, repo := createTestStorageWithRepo(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "item with an open, unmerged PR",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusReview),
	})
	require.NoError(t, err)
	prURL := "https://github.com/example/repo/pull/999"
	_, err = storage.UpdateBacklogItem(t.Context(), item.ID, session.BacklogItemUpdate{PrURL: &prURL}, nil)
	require.NoError(t, err)
	attachWorkSessionWithCommit(t, storage, repo, item.ID, "unmerged-work", repoPath, repoPath, "feature", sha)

	resp, err := svc.GetBacklogItemShipStatus(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemShipStatusRequest{ItemId: item.ID}))
	require.NoError(t, err)
	st := resp.Msg.Status
	assert.Empty(t, st.Error)
	assert.False(t, st.Shipped)
	assert.Empty(t, st.ShippedVia)
	assert.True(t, st.BranchExists)
	assert.Equal(t, int32(1), st.AheadOfMain, "the unmerged feature commit must count as ahead of main")
}

// TestGetBacklogItemShipStatus_should_ReturnErrorField_When_NoWorkSessionEverCommitted
// verifies the no-code case reports a descriptive error rather than a false "shipped".
func TestGetBacklogItemShipStatus_should_ReturnErrorField_When_NoWorkSessionEverCommitted(t *testing.T) {
	_, repoPath := setupPRFixSyncRepo(t)
	storage, _ := createTestStorageWithRepo(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "item with no code",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusDone),
	})
	require.NoError(t, err)

	resp, err := svc.GetBacklogItemShipStatus(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemShipStatusRequest{ItemId: item.ID}))
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Msg.Status.Error)
	assert.False(t, resp.Msg.Status.Shipped)
}

// TestGetBacklogItemShipStatus_should_ListShippedCommits_When_MultipleCommitsInRange
// covers Tyler's ask: identifying which commits actually shipped, like a PR's
// commits tab, so newest-first ordering and content must both be right.
func TestGetBacklogItemShipStatus_should_ListShippedCommits_When_MultipleCommitsInRange(t *testing.T) {
	_, repoPath := setupPRFixSyncRepo(t)
	baseSHA := strings.TrimSpace(runGitTestCmd(t, repoPath, "rev-parse", "HEAD"))

	runGitTestCmd(t, repoPath, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "first.txt"), []byte("first\n"), 0o644))
	runGitTestCmd(t, repoPath, "add", "first.txt")
	runGitTestCmd(t, repoPath, "commit", "-m", "first commit")
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "second.txt"), []byte("second\n"), 0o644))
	runGitTestCmd(t, repoPath, "add", "second.txt")
	runGitTestCmd(t, repoPath, "commit", "-m", "second commit")
	headSHA := strings.TrimSpace(runGitTestCmd(t, repoPath, "rev-parse", "HEAD"))

	storage, repo := createTestStorageWithRepo(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "item with two shipped commits",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusDone),
	})
	require.NoError(t, err)
	attachWorkSessionWithRange(t, storage, repo, item.ID, "two-commit-work", repoPath, repoPath, "feature", baseSHA, headSHA)

	resp, err := svc.GetBacklogItemShipStatus(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemShipStatusRequest{ItemId: item.ID}))
	require.NoError(t, err)
	st := resp.Msg.Status
	require.Len(t, st.Commits, 2)
	assert.Equal(t, headSHA, st.Commits[0].Sha, "newest commit must come first")
	assert.Equal(t, "second commit", st.Commits[0].Summary)
	assert.Equal(t, "first commit", st.Commits[1].Summary)
}

// snapshotBacklogItemUpdate builds a BacklogItemUpdate populating all 6 durable
// ship-snapshot fields, mirroring what CaptureShipSnapshot (Epic 3.3, implemented
// separately) writes via a single UpdateBacklogItem call.
func snapshotBacklogItemUpdate(checkConclusion string, approvedCount, changesReqCount int, snapshotAt time.Time, fileStatsJSON string, captureFailed bool) session.BacklogItemUpdate {
	return session.BacklogItemUpdate{
		ShippedCheckConclusion:       &checkConclusion,
		ShippedApprovedCount:         &approvedCount,
		ShippedChangesReqCount:       &changesReqCount,
		ShippedSnapshotAt:            &snapshotAt,
		ShippedFileStats:             &fileStatsJSON,
		ShippedSnapshotCaptureFailed: &captureFailed,
	}
}

// TestGetBacklogItemShipStatus_ShouldPopulateSnapshotFields_WhenShippedSnapshotAtNonNil
// covers Story 3.4.1's happy path: a fully populated durable snapshot on the
// BacklogItem's 6 new columns must flow straight into the response, with the
// JSON-encoded file stats decoded into ShippedFileStat entries.
func TestGetBacklogItemShipStatus_ShouldPopulateSnapshotFields_WhenShippedSnapshotAtNonNil(t *testing.T) {
	_, repoPath := setupPRFixSyncRepo(t)
	sha := strings.TrimSpace(runGitTestCmd(t, repoPath, "rev-parse", "HEAD"))

	storage, repo := createTestStorageWithRepo(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "item with a durable ship snapshot",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusDone),
	})
	require.NoError(t, err)
	attachWorkSessionWithCommit(t, storage, repo, item.ID, "snapshot-work", repoPath, repoPath, "main", sha)

	fileStats := []git.FileStat{
		{Path: "a.go", Status: "modified", Additions: 5, Deletions: 1},
		{Path: "b.go", Status: "added", Additions: 10, Deletions: 0},
		{Path: "c.go", Status: "deleted", Additions: 0, Deletions: 8},
	}
	encoded, err := json.Marshal(fileStats)
	require.NoError(t, err)

	snapshotAt := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	_, err = storage.UpdateBacklogItem(t.Context(), item.ID, snapshotBacklogItemUpdate("success", 2, 0, snapshotAt, string(encoded), false), nil)
	require.NoError(t, err)

	resp, err := svc.GetBacklogItemShipStatus(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemShipStatusRequest{ItemId: item.ID}))
	require.NoError(t, err)
	st := resp.Msg.Status
	assert.Empty(t, st.Error)
	assert.Equal(t, "success", st.ShippedCheckConclusion)
	assert.Equal(t, int32(2), st.ShippedApprovedCount)
	assert.Equal(t, int32(0), st.ShippedChangesReqCount)
	require.NotNil(t, st.SnapshotAt)
	assert.True(t, snapshotAt.Equal(st.SnapshotAt.AsTime()), "expected %s, got %s", snapshotAt, st.SnapshotAt.AsTime())
	assert.False(t, st.SnapshotCaptureFailed)
	require.Len(t, st.FileStats, 3)
	assert.Equal(t, "a.go", st.FileStats[0].Path)
	assert.Equal(t, sessionv1.FileStatus_FILE_STATUS_MODIFIED, st.FileStats[0].Status)
	assert.Equal(t, int32(5), st.FileStats[0].Additions)
	assert.Equal(t, int32(1), st.FileStats[0].Deletions)
	assert.Equal(t, "b.go", st.FileStats[1].Path)
	assert.Equal(t, sessionv1.FileStatus_FILE_STATUS_ADDED, st.FileStats[1].Status)
	assert.Equal(t, "c.go", st.FileStats[2].Path)
	assert.Equal(t, sessionv1.FileStatus_FILE_STATUS_DELETED, st.FileStats[2].Status)
}

// TestGetBacklogItemShipStatus_ShouldDegradeGracefully_WhenShippedFileStatsJsonCorrupt
// is the architecture-review Concern fix: a corrupt/truncated ShippedFileStats blob
// must not fail the whole RPC — it should log a Warning with the item ID and leave
// FileStats empty while every other snapshot field still populates normally.
func TestGetBacklogItemShipStatus_ShouldDegradeGracefully_WhenShippedFileStatsJsonCorrupt(t *testing.T) {
	_, repoPath := setupPRFixSyncRepo(t)
	sha := strings.TrimSpace(runGitTestCmd(t, repoPath, "rev-parse", "HEAD"))

	storage, repo := createTestStorageWithRepo(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "item with a corrupt ship snapshot",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusDone),
	})
	require.NoError(t, err)
	attachWorkSessionWithCommit(t, storage, repo, item.ID, "corrupt-snapshot-work", repoPath, repoPath, "main", sha)

	snapshotAt := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	_, err = storage.UpdateBacklogItem(t.Context(), item.ID, snapshotBacklogItemUpdate("success", 1, 0, snapshotAt, "{not valid json", false), nil)
	require.NoError(t, err)

	buf := swapWarningLog(t)

	resp, err := svc.GetBacklogItemShipStatus(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemShipStatusRequest{ItemId: item.ID}))
	require.NoError(t, err, "a corrupt file-stats blob must not fail the RPC")
	st := resp.Msg.Status
	assert.Empty(t, st.Error)
	assert.Empty(t, st.FileStats)
	assert.Equal(t, "success", st.ShippedCheckConclusion, "other snapshot fields must still populate")
	assert.Equal(t, int32(1), st.ShippedApprovedCount)
	require.NotNil(t, st.SnapshotAt)
	assert.True(t, snapshotAt.Equal(st.SnapshotAt.AsTime()))
	assert.Contains(t, buf.String(), item.ID, "warning log must contain the backlog item ID")
	assert.Contains(t, buf.String(), "failed to decode ShippedFileStats")
}

// TestGetBacklogItemShipStatus_ShouldReturnDurableSnapshot_WhenCalledAgainstRealEntStorage
// is the integration case: a row written via UpdateBacklogItem (the same call
// CaptureShipSnapshot, implemented separately in Epic 3.3, would make) against a real
// ent-backed Storage, then read through the actual connect RPC handler — confirming the
// full storage-to-RPC mapping path, not just handler logic against an in-memory value.
func TestGetBacklogItemShipStatus_ShouldReturnDurableSnapshot_WhenCalledAgainstRealEntStorage(t *testing.T) {
	_, repoPath := setupPRFixSyncRepo(t)
	sha := strings.TrimSpace(runGitTestCmd(t, repoPath, "rev-parse", "HEAD"))

	storage, repo := createTestStorageWithRepo(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "item with a snapshot captured by the real reconciler path",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusDone),
	})
	require.NoError(t, err)
	attachWorkSessionWithCommit(t, storage, repo, item.ID, "ent-storage-snapshot-work", repoPath, repoPath, "main", sha)

	fileStats := []git.FileStat{{Path: "only.go", Status: "modified", Additions: 3, Deletions: 2}}
	encoded, err := json.Marshal(fileStats)
	require.NoError(t, err)
	snapshotAt := time.Date(2026, 7, 17, 12, 30, 0, 0, time.UTC)

	updated, err := storage.UpdateBacklogItem(t.Context(), item.ID, snapshotBacklogItemUpdate("failure", 1, 1, snapshotAt, string(encoded), false), nil)
	require.NoError(t, err)
	require.NotNil(t, updated.ShippedSnapshotAt, "write must have persisted to the real ent-backed row")

	// Re-fetch through storage.GetBacklogItem directly to confirm the write actually
	// round-tripped through ent before exercising the RPC handler on top of it.
	persisted, err := storage.GetBacklogItem(t.Context(), item.ID)
	require.NoError(t, err)
	require.NotNil(t, persisted.ShippedSnapshotAt)
	assert.Equal(t, "failure", persisted.ShippedCheckConclusion)

	resp, err := svc.GetBacklogItemShipStatus(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemShipStatusRequest{ItemId: item.ID}))
	require.NoError(t, err)
	st := resp.Msg.Status
	assert.Empty(t, st.Error)
	assert.Equal(t, "failure", st.ShippedCheckConclusion)
	assert.Equal(t, int32(1), st.ShippedApprovedCount)
	assert.Equal(t, int32(1), st.ShippedChangesReqCount)
	require.NotNil(t, st.SnapshotAt)
	assert.True(t, snapshotAt.Equal(st.SnapshotAt.AsTime()))
	require.Len(t, st.FileStats, 1)
	assert.Equal(t, "only.go", st.FileStats[0].Path)
}

// TestGetBacklogItemShipStatus_should_NotReportShipped_When_OnlySpawnTimeBaseShaIsRecorded
// is the RPC-side half of BUG-047's regression coverage (the reconciler side
// lives in session/backlog_lifecycle_superseded_test.go).
//
// ItemSession.LastCommitSha used to be seeded once at spawn with the worktree's
// pre-work base SHA and never refreshed. A base SHA is by construction already
// an ancestor of main, so this RPC — which backs the item detail page's Ship PR
// button and its "shipped" badge — reported *every* item as already shipped,
// however much unmerged work its session actually had. This test records only
// the base SHA (no work commit recorded on top) and asserts the RPC refuses to
// claim the item shipped.
func TestGetBacklogItemShipStatus_should_NotReportShipped_When_OnlySpawnTimeBaseShaIsRecorded(t *testing.T) {
	_, repoPath := setupPRFixSyncRepo(t)
	baseSHA := strings.TrimSpace(runGitTestCmd(t, repoPath, "rev-parse", "HEAD"))

	storage, repo := createTestStorageWithRepo(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "item whose session only ever recorded its base SHA",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusPRPending),
	})
	require.NoError(t, err)

	is, err := storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "base-only-work",
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	// Exactly what spawn seeding leaves behind, in both the new field and the
	// legacy one, so pre-fix rows already in production are covered too.
	require.NoError(t, repo.SetItemSessionBaseCommit(t.Context(), is.ID, baseSHA))
	require.NoError(t, repo.UpdateItemSessionGitActivity(t.Context(), is.ID, baseSHA, "", time.Now(), 0))

	resp, err := svc.GetBacklogItemShipStatus(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemShipStatusRequest{ItemId: item.ID}))
	require.NoError(t, err)
	st := resp.Msg.Status
	assert.False(t, st.Shipped,
		"the session's own base commit is always on main; it must never be read as proof the item's work shipped")
	assert.Empty(t, st.ShippedVia)
	assert.NotEmpty(t, st.Error, "the RPC should say it has no committed work to judge, not silently claim success")
}
