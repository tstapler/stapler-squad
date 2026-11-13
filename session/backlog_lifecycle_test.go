package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/internal/sessiontest"
)

// TestCreateItemSessionWithVerdict_Atomic verifies that CreateItemSessionWithVerdict
// creates both ItemSession and ReviewVerdict atomically — both records must exist,
// and the verdict is linked to the session.
func TestCreateItemSessionWithVerdict_Atomic(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{
		Title:              "Atomic Verdict Test",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(domain.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	sessionUUID := "headless-review-" + uuid.New().String()
	is, err := storage.CreateItemSessionWithVerdict(ctx, backlogdto.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: SessionRoleReview,
	}, backlogdto.ReviewVerdictData{
		OverallOutcome: domain.ReviewVerdictFail,
		Summary:        "Blocked by security check.",
	})
	require.NoError(t, err)
	assert.Equal(t, sessionUUID, is.SessionUUID)
	require.NotNil(t, is.ReviewVerdict, "ReviewVerdict must be linked to the ItemSession")
	assert.Equal(t, string(domain.ReviewVerdictFail), is.ReviewVerdict.OverallOutcome)
	assert.Equal(t, "Blocked by security check.", is.ReviewVerdict.Summary)

	// Both records must be queryable from the same DB — verifies the commit succeeded.
	sessions, listErr := storage.ListItemSessions(ctx, item.ID)
	require.NoError(t, listErr)
	require.Len(t, sessions, 1)
	assert.Equal(t, sessionUUID, sessions[0].SessionUUID)
	require.NotNil(t, sessions[0].ReviewVerdict, "ReviewVerdict must be linked to the ItemSession")
	assert.Equal(t, string(domain.ReviewVerdictFail), sessions[0].ReviewVerdict.OverallOutcome)
}

// TestFindStuckReviewItems_ReturnsAbandonedItem_ExcludesActiveAndGateless is a
// regression test for a live-data bug found via manual QA: an item whose
// AutoReopenAfterFailedReview spawn attempt failed and rolled the status back
// to "review" already has a review ItemSession on record, so
// FindReviewItemsWithoutGate never re-checks it and the item sits abandoned
// forever. FindStuckReviewItems must catch exactly that case while excluding
// items with an active session (review still running, or rework in progress)
// and items with no review session at all (that's FindReviewItemsWithoutGate's
// job, not this one's).
func TestFindStuckReviewItems_ReturnsAbandonedItem_ExcludesActiveAndGateless(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	abandoned := newStuckReviewTestItem(t, storage, domain.ReviewVerdictUnverifiable, true, false)
	stillReviewing := newStuckReviewTestItem(t, storage, domain.ReviewVerdictUnverifiable, false, false)
	reworkInProgress := newStuckReviewTestItem(t, storage, domain.ReviewVerdictFail, true, true)

	// Gateless item: review status but no review ItemSession at all — belongs
	// to FindReviewItemsWithoutGate, must NOT show up here.
	gateless, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{
		Title:              "Gateless review item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(domain.BacklogStatusReview),
	})
	require.NoError(t, err)

	er := storage.Repo
	stuck, err := er.FindStuckReviewItems(ctx)
	require.NoError(t, err)

	gotIDs := make([]string, 0, len(stuck))
	for _, item := range stuck {
		gotIDs = append(gotIDs, item.ID.String())
	}
	assert.Contains(t, gotIDs, abandoned.ID)
	assert.NotContains(t, gotIDs, stillReviewing.ID, "item with an active (unfinished) review session must be excluded")
	assert.NotContains(t, gotIDs, reworkInProgress.ID, "item with an active work session (rework in progress) must be excluded")
	assert.NotContains(t, gotIDs, gateless.ID, "item with no review session at all belongs to FindReviewItemsWithoutGate, not FindStuckReviewItems")
}

// TestBackfillMissingPRNumbers_ParsesNumberFromURL is a regression test for a
// live-data bug found via manual QA: a pr_pending item can have pr_url set but
// pr_number left at 0 (e.g. from a pre-migration row), making it permanently
// invisible to FindPRPendingItems' PrNumberGT(0) filter. Verifies the number is
// parsed out of the URL and persisted.
func TestBackfillMissingPRNumbers_ParsesNumberFromURL(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{
		Title:              "Missing pr_number test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(domain.BacklogStatusPRPending),
		RepoPath:           "/tmp/fake-repo",
	})
	require.NoError(t, err)

	prURL := "https://github.com/tstapler/stapler-squad/pull/148"
	_, err = storage.UpdateBacklogItem(ctx, item.ID, backlogdto.BacklogItemUpdate{PrURL: &prURL}, nil)
	require.NoError(t, err)

	er := storage.Repo

	// Before backfill: invisible to FindPRPendingItems despite being pr_pending with a URL.
	before, err := er.FindPRPendingItems(ctx)
	require.NoError(t, err)
	assert.Empty(t, before, "item with pr_number=0 must not be returned before backfill")

	n, err := er.BackfillMissingPRNumbers(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, 148, fetched.PrNumber)

	// After backfill: now visible to FindPRPendingItems.
	after, err := er.FindPRPendingItems(ctx)
	require.NoError(t, err)
	require.Len(t, after, 1)
	assert.Equal(t, item.ID, after[0].ID.String())
}

// ─── RecordPRCreatedOutOfBand ──────────────────────────────────────────────────
//
// Regression coverage for the status-desync bug behind PR #157's linked backlog
// item getting stuck at "review"/BOUNCING instead of "pr_pending": the Review
// Queue's manual "Create PR" button drives SessionService.RunOneShot, a path
// that creates a real PR but — unlike pushAndCreatePR — never touched the
// backlog item at all. See docs/tasks/backlog-feature-improvement.md's
// "second, compounding root cause" note and RecordPRCreatedOutOfBand's doc
// comment above for the full trace.

// ─── PR-lifecycle status drift (self-heal + immediate recovery) ───────────────
//
// Live 2026-07-20 repro: two backlog items (c2ad7bf3-91bf-4d47-8654-
// 0f2f20869080, PR #251; 6700a3f2-8c0d-4a98-8bbd-39515d5391b1, PR #172) sat at
// status="review" with a real, open, cached PR reference that ReconcilePRPending
// could never see, because it anchors purely on status=="pr_pending". The tests
// below cover the two-part fix: (1) pushAndCreatePR/shipViaAgentOrFallback/
// RecordPRCreatedOutOfBand now attempt an immediate recovery when their own
// resolveToPRPending CAS loses a race after PR fields were already persisted,
// and (2) reconcileDriftedPRItems is the periodic self-heal backstop.

// TestFindDriftedPRItems_ExcludesPRPendingAndTerminalStatuses verifies the
// query's own status filter directly: items already in pr_pending (nothing to
// recover), done, or archived must never be returned, even though they may
// still carry PR fields.
func TestFindDriftedPRItems_ExcludesPRPendingAndTerminalStatuses(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.Repo

	for _, status := range []domain.BacklogStatus{domain.BacklogStatusPRPending, domain.BacklogStatusDone, domain.BacklogStatusArchived} {
		prURL := "https://github.com/tstapler/stapler-squad/pull/900"
		prNumber := 900
		item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{
			Title:              "Terminal/pr_pending item " + string(status),
			AcceptanceCriteria: `[]`,
			Priority:           1,
			Status:             string(status),
			RepoPath:           "/tmp/fake-repo",
		})
		require.NoError(t, err)
		_, err = storage.UpdateBacklogItem(ctx, item.ID, backlogdto.BacklogItemUpdate{
			PrURL:    &prURL,
			PrNumber: &prNumber,
		}, nil)
		require.NoError(t, err)
	}

	items, err := er.FindDriftedPRItems(ctx)
	require.NoError(t, err)
	assert.Empty(t, items, "pr_pending/done/archived items must never be treated as drifted")
}

// ─── handleReviewSessionExited ────────────────────────────────────────────────
//
// Review now always happens in a real, hidden session.Instance rather than a
// synchronous in-process headless LLM call (see ReviewGateRunner.Run and
// server/services/session_service.go's SpawnReviewSession). The verdict — if
// any — is submitted via the submit_review_verdict MCP tool while that session
// runs, and handleReviewSessionExited is what processes it once the review
// session exits.

// ─── SetSessionCreator / getSessionCreator wiring ─────────────────────────────

// --- Story 3.3.1: CaptureShipSnapshot ---

// TestCreateBacklogItem_Labels_RoundTripsThroughGetBacklogItem is the
// Epic 0.1 (Story 0.1.2) round-trip test: an item created with Labels/
// ExternalURL set must read back identically via GetBacklogItem, confirming
// the ent create+read mapping for both new fields end-to-end through Storage.
func TestCreateBacklogItem_Labels_RoundTripsThroughGetBacklogItem(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	created, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{
		Title:       "item with labels and external URL",
		Labels:      []string{"bug", "p1"},
		ExternalURL: "https://github.com/tstapler/stapler-squad/issues/42",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"bug", "p1"}, created.Labels)
	assert.Equal(t, "https://github.com/tstapler/stapler-squad/issues/42", created.ExternalURL)

	fetched, err := storage.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"bug", "p1"}, fetched.Labels)
	assert.Equal(t, "https://github.com/tstapler/stapler-squad/issues/42", fetched.ExternalURL)
}

// TestCreateBacklogItem_BaseBranch_RoundTripsThroughGetBacklogItem verifies
// the explicit base_branch override (the opt-in escape hatch from the
// default-branch-only behavior) persists end to end through Storage, and
// that UpdateBacklogItem can change it afterward.
func TestCreateBacklogItem_BaseBranch_RoundTripsThroughGetBacklogItem(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	created, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{
		Title:      "item with an explicit base_branch override",
		BaseBranch: "release-2.0",
	})
	require.NoError(t, err)
	assert.Equal(t, "release-2.0", created.BaseBranch)

	fetched, err := storage.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "release-2.0", fetched.BaseBranch)

	newBranch := "release-2.1"
	updated, err := storage.UpdateBacklogItem(ctx, created.ID, backlogdto.BacklogItemUpdate{BaseBranch: &newBranch}, nil)
	require.NoError(t, err)
	assert.Equal(t, "release-2.1", updated.BaseBranch)
}

// TestCreateBacklogItem_CanonicalizesRepoPathToMainRepo verifies
// Storage.CreateBacklogItem redirects a linked-worktree RepoPath to its main
// repo root at creation time. Every creation path (create_backlog_item,
// import_github_issue, the web UI's RPCs) funnels through this one function,
// so this is the single point that keeps two items targeting the same repo —
// one filed against the main checkout, another by an agent that passed its
// own in-progress worktree as repo_path — from ending up with two different
// RepoPath strings and fragmenting the web UI's "group by repository" view.
func TestCreateBacklogItem_CanonicalizesRepoPathToMainRepo(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	mainDir := t.TempDir()
	repo, err := gogit.PlainInit(mainDir, false)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mainDir, "f.txt"), []byte("x"), 0644))
	_, err = wt.Add("f.txt")
	require.NoError(t, err)
	_, err = wt.Commit("init", &gogit.CommitOptions{Author: &object.Signature{Name: "t", Email: "t@example.com"}})
	require.NoError(t, err)

	worktreeDir := filepath.Join(t.TempDir(), "agent-worktree")
	sessiontest.RunGitOrFail(t, mainDir, "worktree", "add", "-b", "agent-branch", worktreeDir)

	created, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{
		Title:    "item filed from an agent's own worktree",
		RepoPath: worktreeDir,
	})
	require.NoError(t, err)

	wantMain, err := filepath.EvalSymlinks(mainDir)
	require.NoError(t, err)
	gotRepoPath, err := filepath.EvalSymlinks(created.RepoPath)
	require.NoError(t, err)
	assert.Equal(t, wantMain, gotRepoPath, "RepoPath must be canonicalized to the main repo, not stored as the filing agent's worktree")
}
