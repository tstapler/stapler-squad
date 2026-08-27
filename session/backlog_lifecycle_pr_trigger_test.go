package session

import (
	"context"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/git"
)

// newTestRepoWithRemote creates a real (empty) git repository at a fresh temp
// directory with an "origin" remote pointing at remoteURL, so
// github.GetOwnerRepoFromRemote(dir) resolves exactly as it would against a
// real worktree. Each call uses a unique t.TempDir() so github.GetRemoteURL's
// process-wide remoteURLCache (keyed by repoPath) never sees a stale entry
// from a different test.
func newTestRepoWithRemote(t *testing.T, remoteURL string) string {
	t.Helper()
	dir := t.TempDir()
	repo, err := gogit.PlainInit(dir, false)
	require.NoError(t, err)
	_, err = repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{remoteURL}})
	require.NoError(t, err)
	return dir
}

func TestFindPRPendingItemForEvent_should_ReturnItemAndTrue_When_PrNumberAndRepoMatch(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	repoPath := newTestRepoWithRemote(t, "https://github.com/tstapler/stapler-squad.git")
	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "PR pending test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusPRPending),
		RepoPath:           repoPath,
	})
	require.NoError(t, err)
	prURL := "https://github.com/tstapler/stapler-squad/pull/189"
	prNumber := 189
	_, err = storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{PrURL: &prURL, PrNumber: &prNumber}, nil)
	require.NoError(t, err)

	got, found := findPRPendingItemForEvent(ctx, storage.repo, "tstapler/stapler-squad", 189)

	require.True(t, found)
	assert.Equal(t, item.ID, got.ID.String())
}

func TestFindPRPendingItemForEvent_should_ReturnNilFalse_When_PrNumberMatchesButRepoDiffers(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	repoPath := newTestRepoWithRemote(t, "https://github.com/someone-else/unrelated-fork.git")
	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "PR pending test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusPRPending),
		RepoPath:           repoPath,
	})
	require.NoError(t, err)
	prURL := "https://github.com/someone-else/unrelated-fork/pull/189"
	prNumber := 189
	_, err = storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{PrURL: &prURL, PrNumber: &prNumber}, nil)
	require.NoError(t, err)

	_, found := findPRPendingItemForEvent(ctx, storage.repo, "tstapler/stapler-squad", 189)

	assert.False(t, found, "PR-number collision across two tracked repos must not match on number alone")
}

func TestFindPRPendingItemForEvent_should_ReturnNilFalse_When_ZeroPrPendingItemsExist(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	_, found := findPRPendingItemForEvent(context.Background(), storage.repo, "tstapler/stapler-squad", 189)

	assert.False(t, found)
}

func TestTriggerPRFixForEvent_should_ReconcileItemAndReturnMatchedTrue_When_ItemFound(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	repoPath := newTestRepoWithRemote(t, "https://github.com/tstapler/stapler-squad.git")
	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "PR pending test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusPRPending),
		RepoPath:           repoPath,
	})
	require.NoError(t, err)
	prURL := "https://github.com/tstapler/stapler-squad/pull/189"
	prNumber := 189
	_, err = storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{PrURL: &prURL, PrNumber: &prNumber}, nil)
	require.NoError(t, err)

	newTrackedWorkSession(t, storage, item.ID, item.RepoPath, "backlog/trigger-pr-fix-event", "")

	listener := NewBacklogLifecycleListener(storage)
	listener.SetEnabled(true)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{merged: true})
	stubMatchingPRByNumberFinder(listener, "backlog/trigger-pr-fix-event")

	matched, err := listener.TriggerPRFixForEvent(ctx, "tstapler/stapler-squad", 189)

	require.NoError(t, err)
	assert.True(t, matched)

	// Reconciliation runs asynchronously (dispatched onto l.shutdownCtx in a goroutine,
	// see TriggerPRFixForEvent's doc comment) so matched=true only means "queued," not
	// "completed" — poll for the done transition rather than asserting immediately.
	require.Eventually(t, func() bool {
		refreshed, refreshErr := storage.GetBacklogItem(ctx, item.ID)
		return refreshErr == nil && refreshed.Status == string(BacklogStatusDone)
	}, 2*time.Second, 20*time.Millisecond, "reconcilePRPendingItem should have run for the matched item (merged PR -> done)")
}

func TestTriggerPRFixForEvent_should_ReturnFalseNilWithoutQuerying_When_ListenerDisabled(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	repoPath := newTestRepoWithRemote(t, "https://github.com/tstapler/stapler-squad.git")
	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "PR pending test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusPRPending),
		RepoPath:           repoPath,
	})
	require.NoError(t, err)
	prURL := "https://github.com/tstapler/stapler-squad/pull/189"
	prNumber := 189
	_, err = storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{PrURL: &prURL, PrNumber: &prNumber}, nil)
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage) // enabled defaults to false

	matched, err := listener.TriggerPRFixForEvent(ctx, "tstapler/stapler-squad", 189)

	require.NoError(t, err)
	assert.False(t, matched)

	refreshed, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusPRPending), refreshed.Status, "disabled listener must not touch the item")
}

func TestTriggerPRFixForEvent_should_ReturnFalseNil_When_NoPrPendingItemMatches(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	listener := NewBacklogLifecycleListener(storage)
	listener.SetEnabled(true)

	matched, err := listener.TriggerPRFixForEvent(context.Background(), "tstapler/stapler-squad", 189)

	require.NoError(t, err)
	assert.False(t, matched)
}

// TestTriggerPRFixForEvent_should_TagFixAttemptLogAsWebhookTriggered and
// TestReconcilePRPending_should_TagFixAttemptLogAsPollerTriggered are the regression
// tests for the review-verdict gap on AC8 ("% of PR-fix reconciliations triggered by
// webhook vs. poller ... derivable from TriggerFireEvent outcomes and the
// first-successful-delivery log line"): since PRStatusPoller never writes to
// TriggerFireEvent (by design, AC3), the poller side of that split is only knowable
// via remediatePRFixWithBackoffGate's trigger_source-tagged log line. These two tests
// confirm the same fix-attempt funnel logs the correct source for each of
// TriggerPRFixForEvent's two callers.
func TestTriggerPRFixForEvent_should_TagFixAttemptLogAsWebhookTriggered(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	repoPath := newTestRepoWithRemote(t, "https://github.com/tstapler/stapler-squad.git")
	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "PR pending test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusPRPending),
		RepoPath:           repoPath,
	})
	require.NoError(t, err)
	prURL := "https://github.com/tstapler/stapler-squad/pull/189"
	prNumber := 189
	_, err = storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{PrURL: &prURL, PrNumber: &prNumber}, nil)
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	listener.SetEnabled(true)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{
		status: &git.PRStatus{CIFailing: true, FeedbackText: "CI failed"},
	})
	listener.SetPRFixSpawner(&fakePRFixSpawner{})

	buf := redirectInfoLog(t)
	matched, err := listener.TriggerPRFixForEvent(ctx, "tstapler/stapler-squad", 189)

	require.NoError(t, err)
	assert.True(t, matched)
	// Reconciliation runs asynchronously — poll for the log line rather than asserting
	// immediately (see TriggerPRFixForEvent's doc comment).
	require.Eventually(t, func() bool {
		return strings.Contains(buf.String(), "attempting fix (trigger_source=webhook)")
	}, 2*time.Second, 20*time.Millisecond, "webhook-triggered reconciliation must tag its fix-attempt log line as trigger_source=webhook; got: %s", buf.String())
}

// TestReconcilePRPending_should_SteerOnEveryTick_When_ActiveSessionPresentEvenWithinBackoffWindow
// is the session-package-level regression test for pre-mortem.md's P1
// finding (Story 4.2.2): remediatePRFixWithBackoffGate must bypass
// RemediationDue's 30m->72h backoff entirely whenever the item has an active
// work session, so the steer path's own tighter dedup/debounce throttle
// (Epic 2.2/2.3) actually gets to run at the ~60s reconcile-tick cadence it
// was designed for — without this bypass, a changed failure reason (e.g.
// conflict resolved, CI now failing — the same example requirements.md uses
// for Success Metric #2) would not reach AutoReopenForPRFix again for up to
// 72h after the first tick. Distinct from the server/services-level tests
// (Epic 5.3) that cover steerActiveSessionForPRFix's own dedup/debounce
// logic in isolation — this test only proves the gate bypass at the
// session-package boundary, via fakePRFixSpawner.callCount.
func TestReconcilePRPending_should_SteerOnEveryTick_When_ActiveSessionPresentEvenWithinBackoffWindow(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	newPRPendingTestItem(t, storage, 9101)

	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{
		status: &git.PRStatus{HasConflicts: true, FeedbackText: "## Merge conflict\nconflict details\n"},
	})
	fakeSpawner := &fakePRFixSpawner{hasActiveWorkSession: true}
	listener.SetPRFixSpawner(fakeSpawner)

	er := storage.repo
	listener.ReconcilePRPending(ctx, er)
	require.Equal(t, 1, fakeSpawner.callCount, "first tick must call AutoReopenForPRFix (steer path)")

	// Change the underlying failure reason before the second tick — conflict
	// resolved, CI now failing instead — without advancing time or
	// backdating next_remediation_at, so this tick still falls well within
	// RemediationDue's 30-minute floor. If the backoff gate weren't bypassed
	// for the active-session case, this second call would be skipped.
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{
		status: &git.PRStatus{CIFailing: true, FeedbackText: "## Failing CI checks\n- build FAILED\n"},
	})

	listener.ReconcilePRPending(ctx, er)
	assert.Equal(t, 2, fakeSpawner.callCount, "an active-session item must be steered on every tick regardless of RemediationDue's backoff window — bypass path (pre-mortem.md P1)")
}

func TestReconcilePRPending_should_TagFixAttemptLogAsPollerTriggered(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	newPRPendingTestItem(t, storage, 9099)

	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{
		status: &git.PRStatus{CIFailing: true, FeedbackText: "CI failed"},
	})
	listener.SetPRFixSpawner(&fakePRFixSpawner{})

	buf := redirectInfoLog(t)
	listener.ReconcilePRPending(ctx, storage.repo)

	assert.True(t, strings.Contains(buf.String(), "attempting fix (trigger_source=poller)"), "poller-tick reconciliation must tag its fix-attempt log line as trigger_source=poller; got: %s", buf.String())
}
