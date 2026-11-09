package backlogsvc

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
)

// blockerHarness is a done item with one work session whose worktree row points at a
// temp path. removal is recorded, never performed, so "the worktree is retained" is an
// assertion on removed.
type blockerHarness struct {
	t       *testing.T
	storage *session.Storage
	repo    *session.EntRepository
	svc     *BacklogService
	stopper *svctest.MockSessionStopper
	itemID  string
	uuid    string
	path    string
	removed []string
}

func newBlockerHarness(t *testing.T, status session.BacklogStatus) *blockerHarness {
	t.Helper()
	storage, repo := createTestStorageWithRepo(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	h := &blockerHarness{t: t, storage: storage, repo: repo, svc: svc, stopper: &svctest.MockSessionStopper{LiveUUIDs: map[string]bool{}}, uuid: "work-" + t.Name()}
	svc.SetSessionStopper(h.stopper)
	svc.cleanup.removeFn = func(wt session.GitWorktreeData) error {
		h.removed = append(h.removed, wt.WorktreePath)
		return nil
	}
	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{Title: "blocker item", Status: string(status)})
	require.NoError(t, err)
	h.itemID = item.ID
	h.path = filepath.Join(t.TempDir(), "wt")
	now := time.Now()
	require.NoError(t, repo.Create(t.Context(), session.InstanceData{
		Title: h.uuid, UUID: h.uuid, Path: h.path, Status: session.Paused, Program: "claude", CreatedAt: now, UpdatedAt: now,
		Worktree: session.GitWorktreeData{RepoPath: filepath.Dir(h.path), WorktreePath: h.path, SessionName: h.uuid, BranchName: "backlog/x", BaseCommitSHA: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"},
	}))
	_, err = storage.CreateItemSession(t.Context(), session.ItemSessionData{ItemID: item.ID, SessionUUID: h.uuid, SessionRole: session.SessionRoleWork})
	require.NoError(t, err)
	return h
}

func (h *blockerHarness) sessions() []session.ItemSessionSummary {
	h.t.Helper()
	s, err := h.storage.ListItemSessions(h.t.Context(), h.itemID)
	require.NoError(h.t, err)
	return s
}

func (h *blockerHarness) link(owner session.PROwner, n int, state session.PRState) {
	h.t.Helper()
	ref, err := session.NewPRRef("github.com", "acme", "app", n)
	require.NoError(h.t, err)
	_, err = h.repo.UpsertSessionPR(h.t.Context(), owner, ref, session.PRLink{
		Role: session.PRRolePrimary, Source: session.AttachSourceManual, Initial: session.PRInitial{State: state, Terminal: state.Terminal()},
	})
	require.NoError(h.t, err)
}

func uniquePath(h *blockerHarness, label string) context.Context {
	return session.WithCleanupPath(h.t.Context(), label)
}

// Each removal path respects the gate with one open PR, with the live clause on and off.
func TestCleanupGate_ShouldRetainWorktreeOnEveryPath_WhenOnePROpen(t *testing.T) {
	t.Parallel()
	for _, scope := range []bool{false, true} {
		for _, path := range []string{session.CleanupPathArchiveRPC, session.CleanupPathTransition, session.CleanupPathHook, session.CleanupPathSweep, session.CleanupPathRework, session.CleanupPathTombstone} {
			scope, path := scope, path
			name := path + map[bool]string{false: "/scope off", true: "/scope on"}[scope]
			t.Run(name, func(t *testing.T) {
				h := newBlockerHarness(t, session.BacklogStatusReview)
				h.svc.cleanup.liveEnabled = func() bool { return scope }
				h.link(session.SessionPROwner(h.uuid), 21, session.PRStateMerged)
				h.link(session.SessionPROwner(h.uuid), 22, session.PRStateOpen)
				before := session.CleanupBlockedCount(path)
				ctx := uniquePath(h, path)

				switch path {
				case session.CleanupPathArchiveRPC:
					_, err := h.svc.ArchiveBacklogItem(ctx, connect.NewRequest(&sessionv1.ArchiveBacklogItemRequest{ItemId: h.itemID}))
					require.NoError(t, err)
				case session.CleanupPathTransition:
					_, err := h.svc.TransitionBacklogItemStatus(ctx, connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{ItemId: h.itemID, TargetStatus: "done", OverrideReason: "test"}))
					require.NoError(t, err)
				case session.CleanupPathRework:
					out := h.svc.cleanupWorktrees(ctx, h.sessions(), "", session.GateOptions{IgnoreLive: true})
					assert.Equal(t, session.CleanupBlocked, out.Status)
				case session.CleanupPathTombstone:
					assert.Equal(t, session.CleanupBlocked, h.svc.cleanupItemWorktrees(ctx, h.sessions()).Status)
				default:
					out := h.svc.CleanupTerminalItem(ctx, h.itemID)
					assert.Equal(t, session.CleanupBlocked, out.Status)
					assert.False(t, out.LiveOnly)
					out = h.svc.CleanupTerminalItem(ctx, h.itemID) // second tick
					assert.Equal(t, session.CleanupBlocked, out.Status)
				}

				assert.Empty(t, h.removed, "the worktree must still exist")
				assert.Equal(t, before+1, session.CleanupBlockedCount(path), "counted once for this path; a second attempt leaves it unchanged")
				assert.GreaterOrEqual(t, session.CleanupBlockedItemsGauge(path), 1)
				if path == session.CleanupPathTransition || path == session.CleanupPathHook || path == session.CleanupPathSweep {
					assert.Contains(t, h.stopper.ArchivedUUIDs, h.uuid, "archival is not gated by open PRs; only the worktree removal is withheld")
				}
			})
		}
	}
}

func TestCleanupGate_ShouldNameTheItem_WhenOpenPRIsItemDirectOnly(t *testing.T) {
	t.Parallel()
	h := newBlockerHarness(t, session.BacklogStatusDone)
	h.link(session.ItemPROwner(h.itemID), 22, session.PRStateOpen)

	out := h.svc.cleanupItemWorktrees(uniquePath(h, session.CleanupPathArchiveRPC), h.sessions())

	assert.Equal(t, session.CleanupBlocked, out.Status)
	assert.Contains(t, out.Reason, "item "+h.itemID)
	assert.Empty(t, h.removed)
}

func TestCleanupGate_ShouldFailClosed_WhenItemPrNumberHasNoLink(t *testing.T) {
	t.Parallel()
	h := newBlockerHarness(t, session.BacklogStatusDone)
	number, url := 5, ""
	_, err := h.storage.UpdateBacklogItem(t.Context(), h.itemID, session.BacklogItemUpdate{PrNumber: &number, PrURL: &url}, nil)
	require.NoError(t, err)

	out := h.svc.cleanupItemWorktrees(uniquePath(h, session.CleanupPathSweep), h.sessions())

	assert.Equal(t, session.CleanupBlocked, out.Status)
	assert.Contains(t, out.Reason, "item "+h.itemID)
	assert.Empty(t, h.removed)
	blocked := h.svc.BlockedCleanups()
	require.Len(t, blocked, 1, "the stuck report's cleanup-blocked list shows it")
	assert.Equal(t, h.itemID, blocked[0].ItemID)
}

func TestCleanupGate_ShouldRemoveWorktree_WhenEveryPRTerminalAndRecoverAfterLastPRCloses(t *testing.T) {
	t.Parallel()
	h := newBlockerHarness(t, session.BacklogStatusDone)
	h.link(session.SessionPROwner(h.uuid), 22, session.PRStateOpen)
	ctx := uniquePath(h, session.CleanupPathSweep)
	require.Equal(t, session.CleanupBlocked, h.svc.cleanupItemWorktrees(ctx, h.sessions()).Status)

	ref, err := session.NewPRRef("github.com", "acme", "app", 22)
	require.NoError(t, err)
	_, err = h.repo.DeleteSessionPR(t.Context(), session.SessionPROwner(h.uuid), ref)
	require.NoError(t, err)

	out := h.svc.cleanupItemWorktrees(ctx, h.sessions())
	assert.Equal(t, session.CleanupDone, out.Status)
	assert.Equal(t, []string{h.path}, h.removed)
	assert.Empty(t, h.svc.BlockedCleanups(), "released items leave the blocked set")
}

func TestCleanupGate_ShouldHoldWorktreeAndRulePane_WhileCurrentRoundSessionLive(t *testing.T) {
	t.Parallel()
	h := newBlockerHarness(t, session.BacklogStatusDone)
	h.svc.cleanup.liveEnabled = func() bool { return true }
	probe := &stubProbe{live: map[string]bool{h.uuid: true}}
	h.svc.SetLiveWorkProbe(probe)
	h.link(session.SessionPROwner(h.uuid), 21, session.PRStateMerged)

	out := h.svc.CleanupTerminalItem(uniquePath(h, session.CleanupPathHook), h.itemID)

	assert.Equal(t, session.CleanupBlocked, out.Status)
	assert.True(t, out.LiveOnly, "only the live clause holds: never settled")
	assert.Empty(t, h.removed)
	assert.NotContains(t, h.stopper.ArchivedUUIDs, h.uuid, "RULE L: a live session is not archived")
	assert.NotContains(t, h.stopper.KilledPaneUUIDs, h.uuid, "RULE L: and its pane is not killed")

	probe.live[h.uuid] = false
	out = h.svc.CleanupTerminalItem(uniquePath(h, session.CleanupPathHook), h.itemID)
	assert.Equal(t, session.CleanupDone, out.Status)
	assert.Equal(t, []string{h.path}, h.removed)
	assert.Contains(t, h.stopper.ArchivedUUIDs, h.uuid)
	assert.Contains(t, h.stopper.KilledPaneUUIDs, h.uuid)
}

func TestCleanupGate_ShouldIgnoreLiveSessionOnReworkPath_OnlyPRClausesBlock(t *testing.T) {
	t.Parallel()
	h := newBlockerHarness(t, session.BacklogStatusInProgress)
	h.svc.cleanup.liveEnabled = func() bool { return true }
	h.svc.SetLiveWorkProbe(&stubProbe{live: map[string]bool{h.uuid: true}})

	out := h.svc.cleanupWorktrees(uniquePath(h, session.CleanupPathRework), h.sessions(), "", session.GateOptions{IgnoreLive: true})

	assert.Equal(t, session.CleanupDone, out.Status, "a live session does not block the prior rounds' cleanup by itself")
	assert.Equal(t, []string{h.path}, h.removed)
}

func TestCleanupGate_ShouldRecheckBeforeEachRemoval_WhenPRAttachesMidCleanup(t *testing.T) {
	t.Parallel()
	h := newBlockerHarness(t, session.BacklogStatusDone)
	second := "work2-" + t.Name()
	now := time.Now()
	path2 := filepath.Join(t.TempDir(), "wt2")
	require.NoError(t, h.repo.Create(t.Context(), session.InstanceData{
		Title: second, UUID: second, Path: path2, Status: session.Paused, Program: "claude", CreatedAt: now, UpdatedAt: now,
		Worktree: session.GitWorktreeData{RepoPath: filepath.Dir(path2), WorktreePath: path2, SessionName: second, BranchName: "backlog/y", BaseCommitSHA: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"},
	}))
	_, err := h.storage.CreateItemSession(t.Context(), session.ItemSessionData{ItemID: h.itemID, SessionUUID: second, SessionRole: session.SessionRoleWork})
	require.NoError(t, err)
	h.svc.cleanup.removeFn = func(wt session.GitWorktreeData) error {
		h.removed = append(h.removed, wt.WorktreePath)
		h.link(session.ItemPROwner(h.itemID), 77, session.PRStateOpen) // lands after the first removal
		return nil
	}

	out := h.svc.cleanupItemWorktrees(uniquePath(h, session.CleanupPathSweep), h.sessions())

	assert.Equal(t, session.CleanupBlocked, out.Status)
	assert.Len(t, h.removed, 1, "the second worktree is kept: the gate is re-checked per removal")
}

type stubProbe struct{ live map[string]bool }

func (s *stubProbe) IsLive(_ context.Context, id string, _ time.Time) (bool, error) {
	return s.live[id], nil
}

// Row 14 of the path inventory: force-reset stops a live session even with an open PR,
// but the stop is not silent.
func TestStopLiveWorkAndReviewSessions_ShouldStopSession_AndNameItsOpenPR(t *testing.T) {
	t.Parallel()
	h := newBlockerHarness(t, session.BacklogStatusInProgress)
	h.link(session.SessionPROwner(h.uuid), 12, session.PRStateOpen)

	warnings := h.svc.stopLiveWorkAndReviewSessions(t.Context(), h.itemID)

	assert.Equal(t, []string{h.uuid}, h.stopper.StoppedUUIDs, "the operator action still stops the session")
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "acme/app#12")
	assert.Contains(t, warnings[0], h.uuid)
}

func TestStopLiveWorkAndReviewSessions_ShouldWarnNothing_WhenEveryPRTerminal(t *testing.T) {
	t.Parallel()
	h := newBlockerHarness(t, session.BacklogStatusInProgress)
	h.link(session.SessionPROwner(h.uuid), 12, session.PRStateMerged)

	assert.Empty(t, h.svc.stopLiveWorkAndReviewSessions(t.Context(), h.itemID))
}
