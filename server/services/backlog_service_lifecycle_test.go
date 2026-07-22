package services

import (
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
)

// ─── pipeline_mode presence gating (Story 1.4.4) ───────────────────────────────

// TestUpdateBacklogItem_should_PreserveExistingPipelineMode_When_FieldOmittedFromRequest
// is the regression test for the proto3-bool-clobbering bug class this field was
// specifically designed to avoid: an UpdateBacklogItem request that omits
// pipeline_mode entirely (req.Msg.PipelineMode == nil) must never clobber the
// item's existing stored mode back to "".
func TestUpdateBacklogItem_should_PreserveExistingPipelineMode_When_FieldOmittedFromRequest(t *testing.T) {
	svc := newBacklogService(t)

	quick := "quick"
	created, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:        "item using quick mode",
		PipelineMode: &quick,
	}))
	require.NoError(t, err)
	require.NotNil(t, created.Msg.Item.PipelineMode)
	require.Equal(t, "quick", *created.Msg.Item.PipelineMode)

	// pipeline_mode is deliberately left unset (nil) on this request.
	updated, err := svc.UpdateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.UpdateBacklogItemRequest{
		ItemId: created.Msg.Item.Id,
		Title:  "renamed item",
	}))
	require.NoError(t, err)
	require.NotNil(t, updated.Msg.Item.PipelineMode)
	assert.Equal(t, "quick", *updated.Msg.Item.PipelineMode, "omitted pipeline_mode must not clobber the item's existing mode")
}

// TestUpdateBacklogItem_should_ResetPipelineModeToEmptyString_When_FieldExplicitlySetToEmptyString
// proves the other half of presence-gating: an explicitly-present empty string
// (req.Msg.PipelineMode != nil && *req.Msg.PipelineMode == "") is a real reset
// request, distinct from "omitted", and must be honored.
func TestUpdateBacklogItem_should_ResetPipelineModeToEmptyString_When_FieldExplicitlySetToEmptyString(t *testing.T) {
	svc := newBacklogService(t)

	quick := "quick"
	created, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:        "item using quick mode",
		PipelineMode: &quick,
	}))
	require.NoError(t, err)

	empty := ""
	updated, err := svc.UpdateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.UpdateBacklogItemRequest{
		ItemId:       created.Msg.Item.Id,
		PipelineMode: &empty,
	}))
	require.NoError(t, err)
	require.NotNil(t, updated.Msg.Item.PipelineMode)
	assert.Equal(t, "", *updated.Msg.Item.PipelineMode, "explicit empty pipeline_mode must reset the item's mode")
}

// TestCreateBacklogItem_should_SetPipelineModeFromRequest_When_FieldPresent verifies
// CreateBacklogItem persists a non-default pipeline_mode supplied on the request
// (Story 1.4.4).
func TestCreateBacklogItem_should_SetPipelineModeFromRequest_When_FieldPresent(t *testing.T) {
	svc := newBacklogService(t)

	quick := "quick"
	resp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:        "item using quick mode",
		PipelineMode: &quick,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Item.PipelineMode)
	assert.Equal(t, "quick", *resp.Msg.Item.PipelineMode)
}

// ─── auto_create_pr policy flag (opt-in "auto-create PR on Complete") ─────────

// TestCreateBacklogItem_should_DefaultAutoCreatePrToFalse_When_FieldOmitted is the
// default-behavior guard for the opt-in AutoCreatePR policy — an item created
// without the flag must not have it silently enabled.
func TestCreateBacklogItem_should_DefaultAutoCreatePrToFalse_When_FieldOmitted(t *testing.T) {
	svc := newBacklogService(t)

	resp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "item without auto-create-pr",
	}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.Item.AutoCreatePr)
}

// TestCreateBacklogItem_should_PersistAutoCreatePr_When_FieldSetTrue verifies
// CreateBacklogItem persists an explicitly-enabled auto_create_pr flag, and
// UpdateBacklogItem round-trips it (unconditional-bool-wrap pattern, same as
// SkipReviewGate/SkipPlanning/AutoSpawnSession).
func TestCreateBacklogItem_should_PersistAutoCreatePr_When_FieldSetTrue(t *testing.T) {
	svc := newBacklogService(t)

	created, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:        "item with auto-create-pr",
		AutoCreatePr: true,
	}))
	require.NoError(t, err)
	assert.True(t, created.Msg.Item.AutoCreatePr)

	updated, err := svc.UpdateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.UpdateBacklogItemRequest{
		ItemId:       created.Msg.Item.Id,
		AutoCreatePr: true,
	}))
	require.NoError(t, err)
	assert.True(t, updated.Msg.Item.AutoCreatePr, "UpdateBacklogItem must persist auto_create_pr")
}

// TestTransitionBacklogItemStatus_should_BlockDone_When_PrURLSetButCommitNotOnMain is
// the regression test for the bug Tyler reported: an item could reach "done" once a
// PrURL existed at all, regardless of whether that PR was ever actually merged — an
// open, unmerged PR still has PrURL set, so it silently satisfied the old guard.
func TestTransitionBacklogItemStatus_should_BlockDone_When_PrURLSetButCommitNotOnMain(t *testing.T) {
	_, repoPath := setupPRFixSyncRepo(t)

	// A commit that exists only on a feature branch — never merged anywhere —
	// mirroring a PR that was opened but never actually merged.
	runGitTestCmd(t, repoPath, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "feature.txt"), []byte("unshipped work\n"), 0o644))
	runGitTestCmd(t, repoPath, "add", "feature.txt")
	runGitTestCmd(t, repoPath, "commit", "-m", "work that never actually merged")
	unshippedSHA := strings.TrimSpace(runGitTestCmd(t, repoPath, "rev-parse", "HEAD"))

	storage, repo := createTestStorageWithRepo(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "item with an open, unmerged PR",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusReview),
		PrURL:    "https://github.com/example/repo/pull/999", // set, but the PR was never merged
	})
	require.NoError(t, err)

	// A PASS review verdict, so the failure below is unambiguously the code-on-main
	// gate (ErrCodeNotOnMain), not the separate, already-covered verdict gate.
	_, err = storage.CreateItemSessionWithVerdict(t.Context(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "review-session",
		SessionRole: session.SessionRoleReview,
	}, session.ReviewVerdictData{
		OverallOutcome: session.ReviewVerdictPass,
	})
	require.NoError(t, err)

	is, err := storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "unmerged-work-session",
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	require.NoError(t, repo.UpdateItemSessionGitActivity(t.Context(), is.ID, unshippedSHA, "work that never actually merged", time.Now(), 1))

	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       item.ID,
		TargetStatus: "done",
	}))
	require.Error(t, err, "an item whose only PR was never merged must not reach done just because PrURL is set")
	assert.Contains(t, err.Error(), "must actually be on main")

	fetched, err := storage.GetBacklogItem(t.Context(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusReview), fetched.Status, "the item must stay in review, not silently reach done")

	// Now actually merge the commit to main — the item must be allowed to reach
	// done once the code is verifiably shipped.
	runGitTestCmd(t, repoPath, "checkout", "main")
	runGitTestCmd(t, repoPath, "merge", "--no-edit", "feature")

	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       item.ID,
		TargetStatus: "done",
	}))
	require.NoError(t, err, "once the commit is actually merged to main, done must be allowed")

	fetched, err = storage.GetBacklogItem(t.Context(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusDone), fetched.Status)
}

// TestTransitionBacklogItemStatus_should_BlockDone_When_PrPendingWithConflictedPR
// is the regression test for the live incident this guard was extended to catch:
// TransitionGuard previously only matched from==review, so a pr_pending item
// (already past review, sitting on an open, unmerged, conflicted PR) could be
// marked done via a bare TransitionBacklogItemStatus("done") call — e.g. a
// human clicking "Approve" without noticing the PR had a merge conflict — with
// no verdict/shipped-code check at all. Once done, the item became permanently
// invisible to ReconcilePRPending (which only polls pr_pending-status items),
// orphaning the real GitHub PR from any further conflict/CI monitoring. The
// fix widens the guard to `to == done` regardless of from, reusing the exact
// git-ancestry fixture pattern from
// TestTransitionBacklogItemStatus_should_BlockDone_When_PrURLSetButCommitNotOnMain.
func TestTransitionBacklogItemStatus_should_BlockDone_When_PrPendingWithConflictedPR(t *testing.T) {
	_, repoPath := setupPRFixSyncRepo(t)

	// A commit that exists only on a feature branch — mirroring a PR that's
	// still open (and, in the real incident, conflicted) rather than merged.
	runGitTestCmd(t, repoPath, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "feature.txt"), []byte("unshipped work\n"), 0o644))
	runGitTestCmd(t, repoPath, "add", "feature.txt")
	runGitTestCmd(t, repoPath, "commit", "-m", "work behind an open, conflicted PR")
	unshippedSHA := strings.TrimSpace(runGitTestCmd(t, repoPath, "rev-parse", "HEAD"))

	storage, repo := createTestStorageWithRepo(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "item with an open, conflicted PR",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusPRPending),
		PrURL:    "https://github.com/example/repo/pull/172",
		PrNumber: 172,
	})
	require.NoError(t, err)

	_, err = storage.CreateItemSessionWithVerdict(t.Context(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "review-session",
		SessionRole: session.SessionRoleReview,
	}, session.ReviewVerdictData{
		OverallOutcome: session.ReviewVerdictPass,
	})
	require.NoError(t, err)

	is, err := storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "unmerged-work-session",
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	require.NoError(t, repo.UpdateItemSessionGitActivity(t.Context(), is.ID, unshippedSHA, "work behind an open, conflicted PR", time.Now(), 1))

	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       item.ID,
		TargetStatus: "done",
	}))
	require.Error(t, err, "a pr_pending item whose PR was never merged must not reach done")
	assert.Contains(t, err.Error(), "must actually be on main")

	fetched, err := storage.GetBacklogItem(t.Context(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusPRPending), fetched.Status,
		"the item must stay in pr_pending — where ReconcilePRPending can still see and fix it — not silently reach done")
}

// ─── work-session archival on terminal transition (backlog session accumulation bug) ──

// TestTransitionBacklogItemStatus_should_ArchiveWorkSessions_When_ItemReachesDone is
// the regression test for the bug where backlog work sessions accumulated forever in
// the session list even after their item finished — the session archival mechanism
// (see docs/tasks/workflow-history-and-archiving.md) was built exclusively for the
// Workflow feature and was never wired to backlog work sessions. A transition into
// "done" must now archive every work-role session for the item via the injected
// SessionStopper (mirrors cleanupItemWorktrees, which already runs at this call site).
func TestTransitionBacklogItemStatus_should_ArchiveWorkSessions_When_ItemReachesDone(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	stopper := &mockSessionStopper{liveUUIDs: map[string]bool{}}
	svc.SetSessionStopper(stopper)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:  "item reaching done",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	_, err = storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "work-session-to-archive",
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:         item.ID,
		TargetStatus:   "done",
		OverrideReason: "test: bypass verdict gate",
	}))
	require.NoError(t, err)

	assert.Contains(t, stopper.archivedUUIDs, "work-session-to-archive",
		"reaching done must archive the item's work session so it stops accumulating in the session list")
}

// TestTransitionBacklogItemStatus_should_ArchiveWorkSessions_When_ItemArchived mirrors
// the done case above for the "archived" terminal status.
func TestTransitionBacklogItemStatus_should_ArchiveWorkSessions_When_ItemArchived(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	stopper := &mockSessionStopper{liveUUIDs: map[string]bool{}}
	svc.SetSessionStopper(stopper)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:  "item being archived",
		Status: string(session.BacklogStatusIdea),
	})
	require.NoError(t, err)

	_, err = storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "idea-work-session-to-archive",
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       item.ID,
		TargetStatus: "archived",
	}))
	require.NoError(t, err)

	assert.Contains(t, stopper.archivedUUIDs, "idea-work-session-to-archive",
		"archiving an item must archive its work session(s) too")
}

// TestTransitionBacklogItemStatus_should_NotArchiveWorkSessions_When_TransitionIsNotTerminal
// guards against over-eager archival: a non-terminal transition (e.g. ready->in_progress)
// must not touch any work session.
func TestTransitionBacklogItemStatus_should_NotArchiveWorkSessions_When_TransitionIsNotTerminal(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	stopper := &mockSessionStopper{liveUUIDs: map[string]bool{}}
	svc.SetSessionStopper(stopper)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:        "item mid-flight",
		Status:       string(session.BacklogStatusReady),
		SkipPlanning: true,
	})
	require.NoError(t, err)

	_, err = storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "in-flight-work-session",
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       item.ID,
		TargetStatus: "in_progress",
	}))
	require.NoError(t, err)

	assert.Empty(t, stopper.archivedUUIDs, "a non-terminal transition must not archive any work session")
}

// ─── SubmitManualReview PASS→done guard (2026-07-18 finding) ──────────────────
//
// docs/tasks/backlog-feature-improvement.md's 2026-07-18 update: SubmitManualReview
// transitions review->done via the storage layer directly
// (s.storage.TransitionBacklogItemStatus), bypassing the guarded
// TransitionBacklogItemStatus RPC handler's ErrPRRequired check entirely. These
// tests cover both the pre-existing intended behavior (nothing to ship — PASS
// still marks done) and the guard itself (unshipped code — stays in review),
// via the same isCodeShippedToMain check the RPC handler uses (see
// TestTransitionBacklogItemStatus_should_BlockDone_When_PrURLSetButCommitNotOnMain
// above for the underlying git-ancestry fixture pattern reused here).

// TestSubmitManualReview_PassNoUnshippedCode_TransitionsToDone is the happy-path
// regression test: an item with no work-session commits (nothing to ship) must
// still auto-transition straight to done on a PASS manual review, matching the
// pre-existing behavior this guard must not regress.
func TestSubmitManualReview_PassNoUnshippedCode_TransitionsToDone(t *testing.T) {
	svc := newBacklogService(t)

	created, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:        "item with nothing to ship",
		SkipPlanning: true,
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "pending"},
		},
	}))
	require.NoError(t, err)
	itemID := created.Msg.Item.Id

	for _, status := range []string{
		string(session.BacklogStatusReady),
		string(session.BacklogStatusInProgress),
		string(session.BacklogStatusReview),
	} {
		_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
			ItemId:       itemID,
			TargetStatus: status,
		}))
		require.NoError(t, err)
	}

	resp, err := svc.SubmitManualReview(t.Context(), connect.NewRequest(&sessionv1.SubmitManualReviewRequest{
		ItemId:         itemID,
		OverallOutcome: "PASS",
		Summary:        "looks good",
	}))
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusDone), resp.Msg.Item.Status,
		"PASS manual review with nothing to ship should still auto-transition to done")
}

// TestSubmitManualReview_PassWithUnshippedCode_StaysInReviewForShipPR is the
// regression test for the guard itself: a PASS manual review must not mark an
// item done while its work session's commit was never actually merged to main —
// mirrors TestTransitionBacklogItemStatus_should_BlockDone_When_PrURLSetButCommitNotOnMain's
// fixture (a real feature-branch commit, verified via isCodeShippedToMain), but
// through SubmitManualReview's own storage-layer transition instead of the RPC
// handler, to prove this call site is wired to the same guard.
func TestSubmitManualReview_PassWithUnshippedCode_StaysInReviewForShipPR(t *testing.T) {
	_, repoPath := setupPRFixSyncRepo(t)

	runGitTestCmd(t, repoPath, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "feature.txt"), []byte("unshipped work\n"), 0o644))
	runGitTestCmd(t, repoPath, "add", "feature.txt")
	runGitTestCmd(t, repoPath, "commit", "-m", "work that never actually merged")
	unshippedSHA := strings.TrimSpace(runGitTestCmd(t, repoPath, "rev-parse", "HEAD"))

	storage, repo := createTestStorageWithRepo(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "item with unshipped code",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	workIS, err := storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "work-session-uuid",
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	require.NoError(t, repo.UpdateItemSessionGitActivity(t.Context(), workIS.ID, unshippedSHA, "work that never actually merged", time.Now(), 1))

	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       item.ID,
		TargetStatus: string(session.BacklogStatusReview),
	}))
	require.NoError(t, err)

	resp, err := svc.SubmitManualReview(t.Context(), connect.NewRequest(&sessionv1.SubmitManualReviewRequest{
		ItemId:         item.ID,
		OverallOutcome: "PASS",
		Summary:        "looks good",
	}))
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusReview), resp.Msg.Item.Status,
		"PASS manual review whose work-session commit was never merged to main must stay in review for the Ship PR action")
}
