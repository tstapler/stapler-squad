package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/headless"
)

// TestClassifyHeadlessCallError_should_BucketErrorsForLogGrepping covers
// classifyHeadlessCallError's decision table — the 2026-07-24 stuck-triage
// incident required manually reconstructing which failure mode occurred from
// raw timing/text; this test guards that the bucketing logic keeps matching
// its own doc comment.
func TestClassifyHeadlessCallError_should_BucketErrorsForLogGrepping(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		elapsed time.Duration
		want    string
	}{
		{"ctx deadline exceeded", context.DeadlineExceeded, 5 * time.Minute, "timeout"},
		{"wrapped ctx deadline exceeded", fmt.Errorf("headless call ended: %w", context.DeadlineExceeded), 5 * time.Minute, "timeout"},
		{"elapsed within budget tail even without deadline error", errors.New("some other error"), 29*time.Minute + 56*time.Second, "timeout"},
		{"ctx canceled (shutdown)", context.Canceled, time.Minute, "shutdown"},
		{"claude binary not found", headless.ErrClaudeNotFound, time.Second, "claude_not_found"},
		{"llm error", headless.ErrLLMError, time.Minute, "process_error"},
		{"usage error", headless.ErrUsageError, time.Second, "process_error"},
		{"interrupted", headless.ErrInterrupted, time.Second, "process_error"},
		{"unrelated error, short elapsed", errors.New("boom"), time.Minute, "other"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, classifyHeadlessCallError(tc.err, tc.elapsed))
		})
	}
}

// TestApplyTriageResultToUpdate_should_OnlySetValidPriorityAndCategory covers the
// decision table for what triage's assessed priority/item_category actually get
// applied vs. left alone: out-of-range priority, invalid/empty category, and the
// happy path all need to behave correctly for auto-spawn's priority ordering to be
// trustworthy (a clobbered or garbage value would be worse than never assigning one).
func TestApplyTriageResultToUpdate_should_OnlySetValidPriorityAndCategory(t *testing.T) {
	tests := []struct {
		name         string
		priority     int
		itemCategory string
		wantPriority *int
		wantCategory *string
	}{
		{"valid priority and category", 1, "bugfix", intPtr(1), strPtr("bugfix")},
		{"zero priority (omitted) leaves it unset", 0, "feature", nil, strPtr("feature")},
		{"negative priority rejected", -1, "", nil, nil},
		{"priority above range rejected", 6, "", nil, nil},
		{"empty category leaves it unset", 3, "", intPtr(3), nil},
		{"invalid category rejected", 2, "not-a-real-category", intPtr(2), nil},
		{"all valid categories accepted", 3, "chore", intPtr(3), strPtr("chore")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := &session.HeadlessTriageResult{Priority: tc.priority, ItemCategory: tc.itemCategory}
			update := &session.BacklogItemUpdate{}
			applyTriageResultToUpdate(result, update)
			if tc.wantPriority == nil {
				assert.Nil(t, update.Priority)
			} else {
				require.NotNil(t, update.Priority)
				assert.Equal(t, *tc.wantPriority, *update.Priority)
			}
			if tc.wantCategory == nil {
				assert.Nil(t, update.Category)
			} else {
				require.NotNil(t, update.Category)
				assert.Equal(t, *tc.wantCategory, *update.Category)
			}
		})
	}
}

func intPtr(v int) *int { return &v }

// initGitRepoForTest initialises a minimal git repository in dir. A smaller,
// dependency-free duplicate of backlog_triage_harness_test.go's initGitRepo,
// which lives behind the "harness" build tag and isn't linked into normal
// `go test` runs.
func initGitRepoForTest(t *testing.T, dir string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, args := range [][]string{
		{"init", dir},
		{"-C", dir, "config", "user.email", "test@example.com"},
		{"-C", dir, "config", "user.name", "Test"},
	} {
		cmd := safeexec.CommandContext(ctx, "git", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (%s)", args, err, out)
		}
	}
}

// TestResolveSessionPath_should_ErrorNotFallBackToRepoPath_When_GitManagedWorktreeCreationFails
// guards BUG-057: a worktree-creation failure on a repo that IS git-managed
// must fail loudly, not fall back to session.ResolveSessionPath(repoPath) —
// which returns repoPath itself unscoped, silently pointing the spawned
// session directly at the live checkout.
func TestResolveSessionPath_should_ErrorNotFallBackToRepoPath_When_GitManagedWorktreeCreationFails(t *testing.T) {
	repoPath := t.TempDir()
	initGitRepoForTest(t, repoPath)

	// Force CreateBacklogWorktree's worktree-directory creation to fail
	// deterministically: os.MkdirAll(worktreesDir, ...) errors when a path
	// component already exists as a regular file instead of a directory.
	testDir := t.TempDir()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", testDir)
	require.NoError(t, os.WriteFile(filepath.Join(testDir, "worktrees"), []byte("not a directory"), 0o644))

	path, useWorktree, err := resolveSessionPath(repoPath, "test-slug")

	require.Error(t, err)
	assert.False(t, useWorktree)
	assert.Empty(t, path, "must not silently fall back to a path when the repo is git-managed")
}

// TestResolveSessionPath_should_FallBackToDirectory_When_RepoIsNotGitManaged verifies
// the legitimate fallback path still works: a plain, never-git-initialized
// directory should still spawn a directory session at that path.
func TestResolveSessionPath_should_FallBackToDirectory_When_RepoIsNotGitManaged(t *testing.T) {
	repoPath := t.TempDir()

	path, useWorktree, err := resolveSessionPath(repoPath, "test-slug")

	require.NoError(t, err)
	assert.False(t, useWorktree)
	resolved, resolveErr := session.ResolveSessionPath(repoPath)
	require.NoError(t, resolveErr)
	assert.Equal(t, resolved, path)
}

// --- Story 2.1.2: rework_cap durable write (notifyReworkCapHit) ---

// TestNotifyReworkCapHit_should_markStuckReworkCapImmediately_When_CapHit
// verifies that hitting the rework cap writes a durable rework_cap stuck row
// (threshold 0 — the cap hit is a discrete, definitive event) with a
// cap-describing context, in addition to the existing notification.
func TestNotifyReworkCapHit_should_markStuckReworkCapImmediately_When_CapHit(t *testing.T) {
	storage := createTestStorage(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Rework cap test item",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	svc.notifyReworkCapHit(ctx, item.ID, item.Title, session.BacklogStatusReview, "after a failed review verdict", 3)

	open, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, domain.StuckReasonReworkCap, open[0].Reason)
	assert.Equal(t, item.ID, open[0].ItemID)
	assert.Contains(t, open[0].Context, "rework cap")
	assert.NotNil(t, open[0].NotifiedAt, "dedup must be pre-set since the notification already fired")
}

// TestNotifyReworkCapHit_should_stillPublishNotification_When_MarkStuckReturnsError
// verifies the durable write is additive, not a gate: when MarkStuck errors
// (forced here via an invalid item ID so the ent UUID parse fails), the
// operator notification must still publish — a storage hiccup must never
// silently suppress the cap-hit signal.
func TestNotifyReworkCapHit_should_stillPublishNotification_When_MarkStuckReturnsError(t *testing.T) {
	storage := createTestStorage(t)
	ctx := context.Background()

	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	bus := events.NewEventBus(4)
	svc.SetEventBus(bus)

	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)

	svc.notifyReworkCapHit(ctx, "not-a-valid-item-uuid", "Broken Item", session.BacklogStatusReview, "after a failed review verdict", 3)

	select {
	case ev := <-ch:
		assert.Equal(t, events.EventNotification, ev.Type)
	case <-time.After(2 * time.Second):
		t.Fatal("expected a notification event even though MarkStuck errored")
	}
}

// TestNotifyReworkCapHit_should_persistRowSurvivingRestart_When_CapHit verifies
// the rework_cap row survives a simulated server restart (DB close/reopen
// from the same file) — the whole point of moving off the in-memory
// notify-once map (root cause #3).
func TestNotifyReworkCapHit_should_persistRowSurvivingRestart_When_CapHit(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "rework-cap-restart.db")

	var itemID string
	func() {
		repo, err := session.NewEntRepository(session.WithDatabasePath(dbPath))
		require.NoError(t, err)
		defer repo.Close()
		storage, err := session.NewStorageWithRepository(repo)
		require.NoError(t, err)

		item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
			Title:  "Restart-surviving rework cap item",
			Status: string(session.BacklogStatusPRPending),
		})
		require.NoError(t, err)
		itemID = item.ID

		svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
		svc.notifyReworkCapHit(context.Background(), itemID, item.Title, session.BacklogStatusPRPending, "while fixing PR #7", 3)
	}()

	repo2, err := session.NewEntRepository(session.WithDatabasePath(dbPath))
	require.NoError(t, err)
	defer repo2.Close()

	open, err := repo2.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, itemID, open[0].ItemID)
	assert.Equal(t, domain.StuckReasonReworkCap, open[0].Reason)
}

// --- BUG-030: AutoReopenAfterFailedReview's swallowed rollback failure ---

// TestNotifySpawnAndRollbackFailed_should_markStuckAndNotify_When_Called is the
// regression test for BUG-030: live incident on backlog item 54e5aa1f
// ("The camera dialog freezes forever on picture capture") — AutoReopenAfterFailedReview
// transitioned the item review->in_progress, its SpawnSessionFromItem call then
// failed, and the scoped rollback to "review" ALSO failed (its precondition no
// longer matched). Before this fix, that double failure was only
// log.ErrorLog.Printf'd — the item was left silently stranded in_progress with
// no work session and no operator-visible signal anywhere, invisible to every
// stuck detector (none of them check "in_progress with zero live sessions").
// notifySpawnAndRollbackFailed is the fix: a durable StuckReasonSpawnFailed row
// plus an operator notification, mirroring notifyReworkCapHit's structure.
func TestNotifySpawnAndRollbackFailed_should_markStuckAndNotify_When_Called(t *testing.T) {
	storage := createTestStorage(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Camera dialog freezes test item",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	bus := events.NewEventBus(4)
	svc.SetEventBus(bus)
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)

	svc.notifySpawnAndRollbackFailed(ctx, item.ID, item.Title,
		fmt.Errorf("failed to spawn session: worktree setup failed"),
		fmt.Errorf("precondition failed: item already updated"))

	open, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, domain.StuckReasonSpawnFailed, open[0].Reason,
		"the item must be durably marked stuck, not left silently stranded in_progress with no session")
	assert.Equal(t, item.ID, open[0].ItemID)
	assert.Contains(t, open[0].Context, "rework session failed to spawn")
	assert.NotNil(t, open[0].NotifiedAt, "dedup must be pre-set since the notification already fired")

	select {
	case ev := <-ch:
		assert.Equal(t, events.EventNotification, ev.Type)
	case <-time.After(2 * time.Second):
		t.Fatal("expected an operator-facing notification when spawn and rollback both fail")
	}
}

// --- Systemic fix: silent status-transition/session-bookkeeping write failures ---
//
// The 2026-07-27 backlog-feature-improvement audit found four more instances
// of the exact BUG-030/040/041/046/048 shape: a status-transition (or
// session-bookkeeping) write fails AFTER its side effects have already
// happened, the failure is only logged, and nothing else ever surfaces the
// resulting reality/status mismatch. notifyTransitionFailed is the shared fix
// (mirrors notifyTriagePersistFailure's notification-only shape), wired into
// spawnSessionAfterGates and TriggerReReview below and their sibling call
// sites (SubmitManualReview, AttachSessionToItem,
// autonomous_orchestration_service.go's onAutonomousDriverComplete,
// session/backlog_lifecycle.go's reconcileBouncingItems/ReconcilePRPending).

// TestNotifyTransitionFailed_should_publishNotification_When_Called is the
// direct unit test for the shared helper, at the same fidelity as
// TestNotifySpawnAndRollbackFailed_should_markStuckAndNotify_When_Called above
// for BUG-030 — notifyTransitionFailed is notification-only (no durable
// BacklogStuckState row, unlike notifyReworkCapHit/notifySpawnAndRollbackFailed:
// there is no single good StuckReason bucket for "a routine write failed").
func TestNotifyTransitionFailed_should_publishNotification_When_Called(t *testing.T) {
	svc := NewBacklogService(nil, nil, nil, nil, nil, nil)
	bus := events.NewEventBus(4)
	svc.SetEventBus(bus)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)

	svc.notifyTransitionFailed("item-123", "Fix the login bug",
		"code was confirmed shipped to main but the item's transition to done failed",
		fmt.Errorf("precondition failed: expected status \"review\", got \"in_progress\""))

	select {
	case ev := <-ch:
		require.Equal(t, events.EventNotification, ev.Type)
		assert.Equal(t, "Status update failed after work completed", ev.NotificationTitle)
		assert.Contains(t, ev.NotificationMessage, "Fix the login bug")
		assert.Contains(t, ev.NotificationMessage, "code was confirmed shipped to main")
		assert.Equal(t, "item-123", ev.NotificationMetadata["item_id"])
	case <-time.After(2 * time.Second):
		t.Fatal("expected an operator-facing notification when a status-transition write fails")
	}
}

// TestNotifyTransitionFailed_should_NoOp_When_NoEventBusWired verifies the
// no-op guard — must never panic when no event bus is configured (e.g. a
// service constructed without one, as in headless/test contexts).
func TestNotifyTransitionFailed_should_NoOp_When_NoEventBusWired(t *testing.T) {
	svc := NewBacklogService(nil, nil, nil, nil, nil, nil)
	assert.NotPanics(t, func() {
		svc.notifyTransitionFailed("item-123", "Some item", "some failure context", errors.New("boom"))
	})
}

// --- Backlog work-item queue: DequeueNextQueuedItems ---

// TestDequeueNextQueuedItems_SpawnsOldestQueuedItemFirst verifies that once a
// WIP slot frees up, the oldest (by QueuedAt) queued item is claimed and
// spawned — a newer queued item must stay queued until its own slot frees.
func TestDequeueNextQueuedItems_SpawnsOldestQueuedItemFirst(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	// Fill the (default, cfg=nil) WIP cap of 2.
	inProgressIDs := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		id := createReadyItemForSpawn(t, svc, repoPath, fmt.Sprintf("in-progress %d", i))
		_, err := svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: id}))
		require.NoError(t, err)
		inProgressIDs = append(inProgressIDs, id)
	}

	// Queue two more items with a small gap so QueuedAt ordering is deterministic.
	olderID := createReadyItemForSpawn(t, svc, repoPath, "older queued")
	resp, err := svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: olderID}))
	require.NoError(t, err)
	require.True(t, resp.Msg.Queued)

	time.Sleep(5 * time.Millisecond)

	newerID := createReadyItemForSpawn(t, svc, repoPath, "newer queued")
	resp, err = svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: newerID}))
	require.NoError(t, err)
	require.True(t, resp.Msg.Queued)

	// Free exactly one slot: end the first in_progress item's work session and
	// transition it out of in_progress (mirrors what onSessionExited does).
	sessions, err := storage.ListItemSessions(t.Context(), inProgressIDs[0])
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.NoError(t, storage.UpdateItemSessionEnded(t.Context(), sessions[0].ID, time.Now()))
	_, err = storage.TransitionBacklogItemStatus(t.Context(), inProgressIDs[0], session.BacklogStatusReview, nil, session.TriggeredBySystem)
	require.NoError(t, err)

	require.NoError(t, svc.DequeueNextQueuedItems(t.Context()))

	olderItem, err := svc.GetBacklogItem(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: olderID}))
	require.NoError(t, err)
	assert.Equal(t, "in_progress", olderItem.Msg.Item.Status, "the older queued item must be dequeued first")

	newerItem, err := svc.GetBacklogItem(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: newerID}))
	require.NoError(t, err)
	assert.Equal(t, "queued", newerItem.Msg.Item.Status, "only one slot freed — the newer item must stay queued")
}

// TestDequeueNextQueuedItems_RollsBackToQueuedOnSpawnFailure verifies that when
// the claim (queued->in_progress) succeeds but the spawn itself fails (here:
// missing repo_path), the item is rolled back to queued rather than stranded
// in_progress with no session.
func TestDequeueNextQueuedItems_RollsBackToQueuedOnSpawnFailure(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:              "no repo path",
		AcceptanceCriteria: []*sessionv1.AcCriterion{{Index: 0, Text: "test", Status: "pending"}},
		SkipTriage:         true,
		SkipPlanning:       true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id
	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId: itemID, TargetStatus: "ready",
	}))
	require.NoError(t, err)

	item, err := storage.GetBacklogItem(t.Context(), itemID)
	require.NoError(t, err)
	_, err = svc.queueBacklogItem(t.Context(), item, false)
	require.NoError(t, err)

	require.NoError(t, svc.DequeueNextQueuedItems(t.Context()))

	getResp, err := svc.GetBacklogItem(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: itemID}))
	require.NoError(t, err)
	assert.Equal(t, "queued", getResp.Msg.Item.Status, "spawn failure must roll the item back to queued, not strand it in_progress")
	assert.Empty(t, creator.calls)
}

// TestDequeueNextQueuedItems_SurvivesRestart_DequeuesOnFreshServiceInstance
// verifies that "queued" status and queued_at are durable ent state, not
// in-memory — a simulated server restart (DB close/reopen from the same file,
// fresh BacklogService instance) still dequeues and spawns the item once a
// slot is free, exactly like the live process would on its first reconcile
// tick after boot.
func TestDequeueNextQueuedItems_SurvivesRestart_DequeuesOnFreshServiceInstance(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "queue-restart.db")
	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	var itemID string
	func() {
		repo, err := session.NewEntRepository(session.WithDatabasePath(dbPath))
		require.NoError(t, err)
		defer repo.Close()
		storage, err := session.NewStorageWithRepository(repo)
		require.NoError(t, err)

		svc := NewBacklogService(storage, &mockSessionCreator{}, nil, nil, nil, nil)
		itemID = createReadyItemForSpawn(t, svc, repoPath, "restart-surviving queued item")
		item, err := storage.GetBacklogItem(t.Context(), itemID)
		require.NoError(t, err)
		_, err = svc.queueBacklogItem(t.Context(), item, false)
		require.NoError(t, err)
	}()

	// Simulate a server restart: fresh repo/storage/service against the same DB file.
	repo2, err := session.NewEntRepository(session.WithDatabasePath(dbPath))
	require.NoError(t, err)
	defer repo2.Close()
	storage2, err := session.NewStorageWithRepository(repo2)
	require.NoError(t, err)

	fetched, err := storage2.GetBacklogItem(context.Background(), itemID)
	require.NoError(t, err)
	assert.Equal(t, "queued", fetched.Status, "queued status must survive a restart")
	require.NotNil(t, fetched.QueuedAt, "queued_at must survive a restart")

	creator2 := &mockSessionCreator{}
	svc2 := NewBacklogService(storage2, creator2, nil, nil, nil, nil)
	require.NoError(t, svc2.DequeueNextQueuedItems(context.Background()))

	final, err := storage2.GetBacklogItem(context.Background(), itemID)
	require.NoError(t, err)
	assert.Equal(t, "in_progress", final.Status, "a fresh service instance must dequeue the restart-surviving item")
	assert.Len(t, creator2.calls, 1)
}

// TestDequeue_ConcurrentClaimsAreExclusive races two concurrent calls to
// TransitionBacklogItemStatus with the same ExpectedStatus=queued precondition
// against the same item — the SQL-level compare-and-swap (Update().Where(...),
// not a read-then-write check) must let exactly one caller win. Run with -race.
func TestDequeue_ConcurrentClaimsAreExclusive(t *testing.T) {
	storage := createTestStorage(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "concurrent claim item",
		Status: string(session.BacklogStatusQueued),
	})
	require.NoError(t, err)

	precondition := &session.BacklogItemPrecondition{ExpectedStatus: string(session.BacklogStatusQueued)}

	const attempts = 8
	var wg sync.WaitGroup
	results := make(chan error, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := storage.TransitionBacklogItemStatus(ctx, item.ID, session.BacklogStatusInProgress, precondition, session.TriggeredBySystem)
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	successCount := 0
	for err := range results {
		if err == nil {
			successCount++
		}
	}
	assert.Equal(t, 1, successCount, "exactly one concurrent claim must win")

	final, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusInProgress), final.Status)
}

// TestDequeueNextQueuedItems_should_ClaimOnlyOneItem_When_CalledConcurrentlyWithOneFreeSlot
// is the regression test for PR #199 review F2: DequeueNextQueuedItems previously
// had no mutex/singleflight serializing its body. Two concurrent callers (the
// unsynchronized `go l.triggerDequeue(...)` fired from onSessionExited, and the
// periodic ReconcileStuck sweep) could each compute freeSlots from their own
// stale ListBacklogItems snapshot and jointly claim two DIFFERENT queued items
// even though only one WIP slot was actually free — the per-item CAS in
// TransitionBacklogItemStatus only prevents two callers claiming the SAME item,
// not this "jointly overshoot the cap" class of race. This is the exact
// uncontrolled-concurrency-overshoot bug the 2026-07-12 OOM incident (and this
// whole feature) exists to prevent. With dequeueMu now serializing the entire
// method body, only one of the two concurrent calls may observe and claim the
// single free slot. Run with -race.
func TestDequeueNextQueuedItems_should_ClaimOnlyOneItem_When_CalledConcurrentlyWithOneFreeSlot(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	// Fill testWIPCap-1 in_progress slots, leaving exactly 1 free slot.
	for i := 0; i < testWIPCap-1; i++ {
		id := createReadyItemForSpawn(t, svc, repoPath, fmt.Sprintf("in-progress %d", i))
		_, err := svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: id}))
		require.NoError(t, err)
	}
	require.Len(t, creator.calls, testWIPCap-1)

	// Queue 2 items (SkipPlanning so the claim isn't blocked by the planning gate
	// this test isn't exercising).
	queuedIDs := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		id := createReadyItemForSpawn(t, svc, repoPath, fmt.Sprintf("queued %d", i))
		item, err := storage.GetBacklogItem(t.Context(), id)
		require.NoError(t, err)
		_, err = svc.queueBacklogItem(t.Context(), item, false)
		require.NoError(t, err)
		queuedIDs = append(queuedIDs, id)
	}

	// Race two concurrent dequeue sweeps against the single free slot.
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NoError(t, svc.DequeueNextQueuedItems(t.Context()))
		}()
	}
	wg.Wait()

	inProgressCount := 0
	stillQueuedCount := 0
	for _, id := range queuedIDs {
		item, err := storage.GetBacklogItem(t.Context(), id)
		require.NoError(t, err)
		switch item.Status {
		case string(session.BacklogStatusInProgress):
			inProgressCount++
		case string(session.BacklogStatusQueued):
			stillQueuedCount++
		}
	}

	assert.Equal(t, 1, inProgressCount, "exactly one queued item must be dequeued for the single free slot")
	assert.Equal(t, 1, stillQueuedCount, "the other queued item must remain queued — the WIP cap must not be overshot")
	assert.Len(t, creator.calls, testWIPCap, "no more sessions than the WIP cap allows should have been spawned")
}

// TestDequeueNextQueuedItems_should_LeaveQueued_When_ClaimedItemLacksApprovedPlan
// is the defense-in-depth regression test for PR #199 review F3/F4: even if an
// unapproved-plan item somehow ends up "queued" (bypassing
// SpawnSessionFromItem's own planning gate — e.g. a pre-existing row from before
// that gate was reordered ahead of the WIP-cap gate, or a future regression at
// some other call site), the dequeue claim (queued->in_progress) must still be
// rejected by the domain TransitionGuard's ErrPlanRequired check now that the
// claim routes through transitionWithGuard — never silently spawning a work
// session with no planning check at all.
func TestDequeueNextQueuedItems_should_LeaveQueued_When_ClaimedItemLacksApprovedPlan(t *testing.T) {
	storage := createTestStorage(t)
	ctx := context.Background()
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	// Directly construct an item already "queued" with no approved plan and
	// SkipPlanning=false — a state the RPC surface should never produce after
	// the F2 ordering fix, used here to exercise the defense-in-depth layer in
	// isolation.
	now := time.Now()
	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:    "queued without approved plan",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusQueued),
		QueuedAt: &now,
	})
	require.NoError(t, err)
	require.False(t, item.PlanApproved)
	require.False(t, item.SkipPlanning)

	require.NoError(t, svc.DequeueNextQueuedItems(ctx))

	final, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusQueued), final.Status, "an unapproved-plan item must never be claimed off the queue")
	assert.Empty(t, creator.calls, "no session should have been spawned for an unapproved-plan item")
}

// TestDequeueNextQueuedItems_should_AutoSpawnReadyItem_When_SlotFreeAndConfigDefault
// guards the "software factory" default switch: a "ready" item that was never
// explicitly queued (no manual "Spawn Session" click, no AutoSpawnSession flag) must
// still be picked up and spawned by DequeueNextQueuedItems when a WIP slot is free —
// this is what makes auto-implementation the default (config.Config.
// AutoSpawnReadyItemsOrDefault defaults to true with cfg=nil, matching every other
// test in this file).
func TestDequeueNextQueuedItems_should_AutoSpawnReadyItem_When_SlotFreeAndConfigDefault(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	itemID := createReadyItemForSpawn(t, svc, repoPath, "never manually spawned")

	require.NoError(t, svc.DequeueNextQueuedItems(t.Context()))

	item, err := svc.GetBacklogItem(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: itemID}))
	require.NoError(t, err)
	assert.Equal(t, "in_progress", item.Msg.Item.Status, "a ready item must be auto-spawned once a slot is free, with no manual trigger")
	assert.Len(t, creator.calls, 1)
}

// TestDequeueNextQueuedItems_should_NotAutoSpawnReadyItems_When_ConfigDisabled verifies
// the opt-out: explicit AutoSpawnReadyItems=false must leave "ready" items exactly
// where manual-spawn-only behavior left them before this feature existed.
func TestDequeueNextQueuedItems_should_NotAutoSpawnReadyItems_When_ConfigDisabled(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	disabled := false
	cfg := &config.Config{AutoSpawnReadyItems: &disabled}
	svc := NewBacklogService(storage, creator, cfg, nil, nil, nil)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	itemID := createReadyItemForSpawn(t, svc, repoPath, "manual spawn only")

	require.NoError(t, svc.DequeueNextQueuedItems(t.Context()))

	item, err := svc.GetBacklogItem(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: itemID}))
	require.NoError(t, err)
	assert.Equal(t, "ready", item.Msg.Item.Status, "with the feature disabled, a ready item must NOT be auto-spawned")
	assert.Empty(t, creator.calls)
}

// TestDequeueNextQueuedItems_should_SpawnHigherPriorityReadyItemFirst_When_OnlyOneSlotFree
// is the direct regression test for "in priority order": P1 must win over P5 for the
// one free slot, regardless of which was created first.
func TestDequeueNextQueuedItems_should_SpawnHigherPriorityReadyItemFirst_When_OnlyOneSlotFree(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	// Fill the (default, cfg=nil) WIP cap of 2 first, then free exactly one slot —
	// mirrors TestDequeueNextQueuedItems_SpawnsOldestQueuedItemFirst's setup.
	inProgressIDs := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		id := createReadyItemForSpawn(t, svc, repoPath, fmt.Sprintf("in-progress %d", i))
		_, err := svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: id}))
		require.NoError(t, err)
		inProgressIDs = append(inProgressIDs, id)
	}

	// Lower-priority item created first, higher-priority item created second — a
	// pure FIFO/creation-order dequeue would pick the P5 one; priority order must
	// pick the P1 one instead.
	p5ID := createReadyItemWithPriority(t, svc, repoPath, "low priority", 5)
	time.Sleep(5 * time.Millisecond)
	p1ID := createReadyItemWithPriority(t, svc, repoPath, "high priority", 1)

	sessions, err := storage.ListItemSessions(t.Context(), inProgressIDs[0])
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.NoError(t, storage.UpdateItemSessionEnded(t.Context(), sessions[0].ID, time.Now()))
	_, err = storage.TransitionBacklogItemStatus(t.Context(), inProgressIDs[0], session.BacklogStatusReview, nil, session.TriggeredBySystem)
	require.NoError(t, err)

	require.NoError(t, svc.DequeueNextQueuedItems(t.Context()))

	p1Item, err := svc.GetBacklogItem(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: p1ID}))
	require.NoError(t, err)
	assert.Equal(t, "in_progress", p1Item.Msg.Item.Status, "the P1 item must be spawned first, regardless of creation order")

	p5Item, err := svc.GetBacklogItem(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: p5ID}))
	require.NoError(t, err)
	assert.Equal(t, "ready", p5Item.Msg.Item.Status, "the P5 item must stay ready — only one slot was free")
}

// TestDequeueNextQueuedItems_should_RollBackToReady_When_AutoClaimedReadyItemSpawnFails
// mirrors TestDequeueNextQueuedItems_RollsBackToQueuedOnSpawnFailure for the new
// ready-origin claim path: rollback must target "ready" (where the item actually
// came from), not unconditionally "queued" — in_progress->queued isn't even a valid
// transition for an item that was never queued.
func TestDequeueNextQueuedItems_should_RollBackToReady_When_AutoClaimedReadyItemSpawnFails(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:              "ready with no repo path",
		AcceptanceCriteria: []*sessionv1.AcCriterion{{Index: 0, Text: "test", Status: "pending"}},
		SkipTriage:         true,
		SkipPlanning:       true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id
	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId: itemID, TargetStatus: "ready",
	}))
	require.NoError(t, err)

	require.NoError(t, svc.DequeueNextQueuedItems(t.Context()))

	getResp, err := svc.GetBacklogItem(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: itemID}))
	require.NoError(t, err)
	assert.Equal(t, "ready", getResp.Msg.Item.Status, "spawn failure for an auto-claimed ready item must roll back to ready, not queued")
	assert.Empty(t, creator.calls)
}

// --- AutoReopenForPRFix: live-bug regression (ReconcilePRPending churn) ---

// TestAutoReopenForPRFix_ActiveWorkSession_SkipsWithoutStatusChurn is the regression
// test for a live production incident: ReconcilePRPending calls AutoReopenForPRFix on
// every ~60s tick for any pr_pending item with failing CI, with no check for whether a
// fix is already in flight. When a work session was still genuinely active (a real
// multi-hour autonomous session, not dead), the old code transitioned pr_pending->
// in_progress, discovered SpawnSessionFromItem was blocked by that same active session,
// and rolled back to pr_pending — writing two BacklogStatusEvent rows every tick,
// forever, with zero progress. AutoReopenForPRFix must now check for an active work
// session FIRST and return early with no status transition at all.
func TestAutoReopenForPRFix_ActiveWorkSession_SkipsWithoutStatusChurn(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	stopper := &mockSessionStopper{liveUUIDs: map[string]bool{"active-work-uuid": true}}
	svc.SetSessionStopper(stopper)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "PR-pending item with an active fix session",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusPRPending),
		PrNumber: 42,
		PrURL:    "https://github.com/example/repo/pull/42",
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "active-work-uuid",
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	reopenErr := svc.AutoReopenForPRFix(context.Background(), item.ID, "CI failing: tests broke")
	require.NoError(t, reopenErr, "must not error — this is the expected 'already in flight' outcome, not a failure")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusPRPending), fetched.Status, "status must never churn while a fix session is already active")
	assert.Empty(t, creator.calls, "no new session should be spawned while one is already active")
}

// TestAutoReopenForPRFix_ActiveWorkSession_RecordsRespawnBlockedActive is the
// regression test for the audit-trail gap this fix closes: before it, the
// skip branch above only log.InfoLog.Printf'd — no durable
// BacklogStuckState row and no operator notification, unlike every other
// "an automated action was skipped" path in this file (notifyReworkCapHit,
// notifySpawnAndRollbackFailed). Verifies a StuckReasonRespawnBlockedActive
// row is written and a notification is published when the skip fires.
func TestAutoReopenForPRFix_ActiveWorkSession_RecordsRespawnBlockedActive(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	stopper := &mockSessionStopper{liveUUIDs: map[string]bool{"active-work-uuid": true}}
	svc.SetSessionStopper(stopper)
	bus := events.NewEventBus(4)
	svc.SetEventBus(bus)
	subCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "PR-pending item with an active fix session",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusPRPending),
		PrNumber: 42,
		PrURL:    "https://github.com/example/repo/pull/42",
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "active-work-uuid",
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	require.NoError(t, svc.AutoReopenForPRFix(context.Background(), item.ID, "CI failing: tests broke"))

	open, err := storage.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, domain.StuckReasonRespawnBlockedActive, open[0].Reason)
	assert.Equal(t, item.ID, open[0].ItemID)
	assert.Contains(t, open[0].Context, "active-work-uuid")

	select {
	case ev := <-ch:
		assert.Equal(t, events.EventNotification, ev.Type)
		assert.Equal(t, item.ID, ev.NotificationMetadata["item_id"])
	case <-time.After(2 * time.Second):
		t.Fatal("expected an operator-facing notification when auto-respawn is skipped for an active session")
	}
}

// TestAutoReopenForPRFix_NoActiveSession_ResolvesAnyOpenRespawnBlockedActiveRow
// is AutoReopenForPRFix's counterpart to
// TestAutoRespawnAutonomousWork_NoActiveSession_ResolvesAnyOpenRespawnBlockedActiveRow
// — verifies the inline resolve at the top of AutoReopenForPRFix clears a
// pre-existing respawn_blocked_active row once its guard passes. (The
// independent periodic sweep, reconcileRespawnBlockedActiveResolution in
// session/backlog_lifecycle.go, additionally guarantees resolution even when
// this inline path is never reached again — see that function's doc comment
// and TestReconcileRespawnBlockedActiveResolution_should_resolveRow_When_BlockingSessionHasEnded.)
func TestAutoReopenForPRFix_NoActiveSession_ResolvesAnyOpenRespawnBlockedActiveRow(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	svc.SetSessionStopper(&mockSessionStopper{liveUUIDs: map[string]bool{}})

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "PR-pending item whose blocking fix session has since ended",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusPRPending),
		PrNumber: 44,
		PrURL:    "https://github.com/example/repo/pull/44",
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "ended-work-uuid",
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	sessions, err := storage.ListItemSessions(context.Background(), item.ID)
	require.NoError(t, err)
	require.NoError(t, storage.UpdateItemSessionEnded(context.Background(), sessions[0].ID, time.Now()))

	applied, err := storage.MarkStuck(context.Background(), item.ID, domain.StuckReasonRespawnBlockedActive,
		session.BacklogStatusPRPending, "pre-existing open row from a prior blocked attempt")
	require.NoError(t, err)
	require.True(t, applied)

	require.NoError(t, svc.AutoReopenForPRFix(context.Background(), item.ID, "CI failing: tests broke"))
	assert.Len(t, creator.calls, 1, "a fresh fix session must be spawned now that nothing is blocking")

	open, err := storage.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	for _, row := range open {
		assert.NotEqual(t, domain.StuckReasonRespawnBlockedActive, row.Reason,
			"the respawn_blocked_active row must be resolved once the guard passes")
	}
}

// TestAutoReopenForPRFix_DeadWorkSession_TombstonesThenReopens verifies the other half:
// a work session that IS confirmed dead (not live) must be tombstoned automatically so
// the reopen can proceed normally, rather than blocking forever like the bug above.
func TestAutoReopenForPRFix_DeadWorkSession_TombstonesThenReopens(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	stopper := &mockSessionStopper{liveUUIDs: map[string]bool{}} // nothing is live
	svc.SetSessionStopper(stopper)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "PR-pending item with a dead prior session",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusPRPending),
		PrNumber: 43,
		PrURL:    "https://github.com/example/repo/pull/43",
	})
	require.NoError(t, err)
	deadIS, err := storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "dead-work-uuid",
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	reopenErr := svc.AutoReopenForPRFix(context.Background(), item.ID, "CI failing: tests broke")
	require.NoError(t, reopenErr)

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusInProgress), fetched.Status, "reopen must proceed once the dead session is cleared")
	assert.Len(t, creator.calls, 1, "a new fix session must be spawned")

	deadFetched, err := storage.GetItemSession(context.Background(), deadIS.ID)
	require.NoError(t, err)
	assert.NotNil(t, deadFetched.EndedAt, "the dead session must be tombstoned")
}

// --- AutoRespawnAutonomousWork: closes the same audit-trail gap as AutoReopenForPRFix ---

// TestAutoRespawnAutonomousWork_ActiveWorkSession_RecordsRespawnBlockedActive
// is AutoRespawnAutonomousWork's counterpart to
// TestAutoReopenForPRFix_ActiveWorkSession_RecordsRespawnBlockedActive — the
// first of the three call sites this fix covers. Before this fix, an
// in_progress item whose autonomous work session was still active only
// produced a bare log.InfoLog.Printf'd skip with no durable record and no
// operator notification.
func TestAutoRespawnAutonomousWork_ActiveWorkSession_RecordsRespawnBlockedActive(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	stopper := &mockSessionStopper{liveUUIDs: map[string]bool{"active-work-uuid": true}}
	svc.SetSessionStopper(stopper)
	bus := events.NewEventBus(4)
	svc.SetEventBus(bus)
	subCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "In-progress item with an active autonomous work session",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "active-work-uuid",
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	require.NoError(t, svc.AutoRespawnAutonomousWork(context.Background(), item.ID))
	assert.Empty(t, creator.calls, "no new session should be spawned while one is already active")

	open, err := storage.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, domain.StuckReasonRespawnBlockedActive, open[0].Reason)
	assert.Equal(t, item.ID, open[0].ItemID)
	assert.Contains(t, open[0].Context, "active-work-uuid")

	select {
	case ev := <-ch:
		assert.Equal(t, events.EventNotification, ev.Type)
	case <-time.After(2 * time.Second):
		t.Fatal("expected an operator-facing notification when auto-respawn is skipped for an active session")
	}
}

// TestAutoRespawnAutonomousWork_NoActiveSession_ResolvesAnyOpenRespawnBlockedActiveRow
// verifies the other half: once the blocking session ends and the guard
// passes, a previously-open respawn_blocked_active row must be resolved
// rather than left open forever (mirrors ResolveReworkBlockedStaleIfRecovered's
// resolve-side responsibility for its own reason).
func TestAutoRespawnAutonomousWork_NoActiveSession_ResolvesAnyOpenRespawnBlockedActiveRow(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	svc.SetSessionStopper(&mockSessionStopper{liveUUIDs: map[string]bool{}})

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "In-progress item whose blocking session has since ended",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)
	// Ended work session — findActiveWorkSession must not treat this as active.
	_, err = storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "ended-work-uuid",
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	sessions, err := storage.ListItemSessions(context.Background(), item.ID)
	require.NoError(t, err)
	require.NoError(t, storage.UpdateItemSessionEnded(context.Background(), sessions[0].ID, time.Now()))

	applied, err := storage.MarkStuck(context.Background(), item.ID, domain.StuckReasonRespawnBlockedActive,
		session.BacklogStatusInProgress, "pre-existing open row from a prior blocked attempt")
	require.NoError(t, err)
	require.True(t, applied)

	require.NoError(t, svc.AutoRespawnAutonomousWork(context.Background(), item.ID))
	assert.Len(t, creator.calls, 1, "a fresh work session must be spawned now that nothing is blocking")

	open, err := storage.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	for _, row := range open {
		assert.NotEqual(t, domain.StuckReasonRespawnBlockedActive, row.Reason,
			"the respawn_blocked_active row must be resolved once the guard passes")
	}
}

// --- AutoRespawnReview: closes the "detected/notified but never respawned" gap ---
//
// Before this, markAbandonedReview (session/backlog_lifecycle.go) only wrote a
// stuck row and notified an operator — nothing ever re-triggered the review gate,
// so real backlog items sat stuck in review for days
// (docs/tasks/backlog-feature-improvement.md, 2026-07-17 update). AutoRespawnReview
// implements session.ReviewRespawner and is the mechanism markAbandonedReview now
// dispatches into.

// --- Repeated-failure circuit breaker (session.IsRepeatedFailure) ---

// TestAutoReopenAfterFailedReview_RepeatedFailure_LeavesInReviewAndNotifies is
// the regression test for a fast-looping non-converging rework cycle (e.g. an
// infrastructure fault like a broken worktree diff, reproduced identically on
// every attempt): once the last two review verdicts fail for the exact same
// reason, AutoReopenAfterFailedReview must stop reopening — ahead of the
// (possibly much larger) rework cap — and park the item via the same durable
// stuck-state/notification path notifyReworkCapHit uses.
func TestAutoReopenAfterFailedReview_RepeatedFailure_LeavesInReviewAndNotifies(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	ctx := context.Background()

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:    "Item that fails the same way every time",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	// Two prior review rounds, both ending in an identical FAIL verdict — the
	// shape left behind by a persistent infrastructure fault that a fresh
	// rework attempt can never fix on its own.
	for i := 0; i < 2; i++ {
		is, isErr := storage.CreateItemSession(ctx, session.ItemSessionData{
			ItemID:      item.ID,
			SessionUUID: "prior-review-" + string(rune('a'+i)),
			SessionRole: session.SessionRoleReview,
		})
		require.NoError(t, isErr)
		require.NoError(t, storage.SaveReviewVerdict(ctx, is.ID, session.ReviewVerdictData{
			ItemSessionID:  is.ID,
			OverallOutcome: session.ReviewOutcomeFail,
			Summary:        "Review blocked: could not compute a diff for this session",
		}))
	}

	reopenErr := svc.AutoReopenAfterFailedReview(ctx, item.ID)
	require.NoError(t, reopenErr, "stopping the loop is an expected outcome, not a failure")

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusReview), fetched.Status, "item must stay in review, not spin on an identical failure")

	open, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, domain.StuckReasonBouncing, open[0].Reason, "reuses the bouncing reason — same non-converging-cycle semantics, tripped immediately instead of waiting for the periodic sweep")
}

// TestAutoReopenAfterFailedReview_ActiveWorkSession_StillTransitionsToInProgress
// is the regression test for backlog item 4c71d3a3-1dd5-4d82-86ec-694a98835d2f
// ("request_review fails permanently when a backlog item is stuck in 'review'
// status with no active reviewer"). Confirmed live 2026-08-03 on backlog item
// 40a243b0: a review session recorded a FAIL verdict and then died as a
// zombie (no clean exit) before handleReviewSessionExited ever processed it.
// The crash-recovery sweep (reconcileUnprocessedReviewVerdicts) correctly
// detected this and dispatched into AutoReopenAfterFailedReview — but this
// function's hasActiveWorkSession guard used to return here without ever
// transitioning the item out of "review", on the theory that the live work
// session would "discover the verdict on its own next poll." That theory had
// no implementation behind it: request_review is the only tool available to
// a work session, and its own precondition hardcodes ExpectedStatus:
// in_progress (server/mcp/tools_backlog.go) — so the live work session could
// never make forward progress, no matter how many times it fixed the noted
// gaps and called request_review again. It failed identically forever with
// "concurrent modification detected: expected status \"in_progress\", got
// \"review\"". This test asserts the item now transitions to in_progress
// even when a work session is still active — it fails against the pre-fix
// code, which left the item's status unchanged ("review") here.
func TestAutoReopenAfterFailedReview_ActiveWorkSession_StillTransitionsToInProgress(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Item whose reviewer zombied out after a FAIL verdict",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	// The work session that originally called request_review and is still
	// alive, fixing the FAIL findings and about to call request_review again.
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "still-alive-work-session",
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	reopenErr := svc.AutoReopenAfterFailedReview(ctx, item.ID)
	require.NoError(t, reopenErr, "reusing an active work session is an expected outcome, not a failure")

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusInProgress), fetched.Status,
		"must transition back to in_progress even when a work session is already active — otherwise that session's request_review precondition can never be satisfied again")

	// No new work session should have been spawned — the point is reusing the
	// live one, not respawning.
	sessions, err := storage.ListItemSessions(ctx, item.ID)
	require.NoError(t, err)
	workCount := 0
	for _, is := range sessions {
		if is.Role == session.SessionRoleWork {
			workCount++
		}
	}
	assert.Equal(t, 1, workCount, "must not spawn a second work session when one is already active")
}

// --- Stale-but-alive blocking work session: closes the "zero operator signal" gap ---
//
// hasActiveWorkSession's guard (above) is purely liveness-based (EndedAt == nil) and
// cannot distinguish a session that's genuinely making progress from one that's alive
// but has produced no output for hours. Confirmed live 2026-07-20 on backlog item
// 9264efe7: the session manager reported the blocking work session Active with a
// current last_activity_at, while review_queue_determiner.go's independently-computed
// staleness detector flagged the SAME session "STALENESS DETECTED ... 6h 35m since
// last meaningful output" on every reconciliation tick — with nothing ever surfaced to
// the operator. These two tests cover notifyIfActiveWorkSessionStale, which closes
// that visibility gap without changing the reopen decision itself (the stale session
// is never stopped, killed, or bypassed — see that function's doc comment for why).

// TestAutoReopenAfterFailedReview_ActiveStaleWorkSession_NotifiesOperator verifies
// that when the active work session blocking a respawn is ALSO independently
// confirmed stale — using the exact same staleness computation and threshold
// review_queue_determiner.go's own detector uses
// (Instance.GetTimeSinceLastMeaningfulOutput vs
// session.DefaultReviewQueuePollerConfig().StalenessThreshold) — an operator
// notification fires, while the "reuse the live session instead of
// respawning" decision itself is unchanged. The item must still transition to
// in_progress regardless (see
// TestAutoReopenAfterFailedReview_ActiveWorkSession_StillTransitionsToInProgress):
// request_review's own precondition requires it, and a stale-but-alive
// session is still deliberately never stopped or bypassed here.
func TestAutoReopenAfterFailedReview_ActiveStaleWorkSession_NotifiesOperator(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	ctx := context.Background()

	stopper := &mockSessionStopper{
		liveUUIDs: map[string]bool{"active-work-uuid": true},
		// Mirrors the live incident's observed staleness (6h 35m), well past
		// the review queue's own 2-minute StalenessThreshold.
		staleFor: map[string]time.Duration{"active-work-uuid": 6*time.Hour + 35*time.Minute},
	}
	svc.SetSessionStopper(stopper)

	bus := events.NewEventBus(4)
	svc.SetEventBus(bus)
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Item with a stale-but-alive blocking work session",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "active-work-uuid",
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	reopenErr := svc.AutoReopenAfterFailedReview(ctx, item.ID)
	require.NoError(t, reopenErr, "an active work session is an expected 'leave it in place' outcome, not a failure")

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusInProgress), fetched.Status,
		"the item must still transition to in_progress so the live (if stale) work session's next request_review call can succeed — a stale session is never stopped, but the transition itself is unconditional")

	select {
	case ev := <-ch:
		assert.Equal(t, events.EventNotification, ev.Type)
	case <-time.After(2 * time.Second):
		t.Fatal("expected an operator notification when the blocking work session is independently stale")
	}

	// Story 2.1.1: the toast is now paired with a durable MarkStuck row so
	// this state survives past the one-shot notification.
	open, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, domain.StuckReasonReworkBlockedStale, open[0].Reason)
	// ItemStatus reflects the item's actual current status at query time (it
	// is not a snapshot taken when MarkStuck ran) — now in_progress, since the
	// fix makes AutoReopenAfterFailedReview transition the item regardless of
	// the active work session.
	assert.Equal(t, session.BacklogStatusInProgress, open[0].ItemStatus)
}

// TestAutoReopenAfterFailedReview_ActiveFreshWorkSession_NoNotification is the
// companion negative case: an active work session that is NOT independently flagged
// stale (idle time well under review_queue_determiner.go's own staleness threshold)
// must not trigger a notification — this closes the "silent skip" gap only for
// genuinely stuck sessions, not every routine active-session skip (which would make
// the notification spam-prone rather than a meaningful signal). This is also the
// exact shape backlog item 4c71d3a3 reported live: the item must still
// transition to in_progress here (see
// TestAutoReopenAfterFailedReview_ActiveWorkSession_StillTransitionsToInProgress
// for the dedicated regression test), even though no notification fires.
func TestAutoReopenAfterFailedReview_ActiveFreshWorkSession_NoNotification(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	ctx := context.Background()

	stopper := &mockSessionStopper{
		liveUUIDs: map[string]bool{"active-work-uuid": true},
		staleFor:  map[string]time.Duration{"active-work-uuid": 5 * time.Second},
	}
	svc.SetSessionStopper(stopper)

	bus := events.NewEventBus(4)
	svc.SetEventBus(bus)
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Item with a genuinely-active blocking work session",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "active-work-uuid",
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	reopenErr := svc.AutoReopenAfterFailedReview(ctx, item.ID)
	require.NoError(t, reopenErr)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusInProgress), fetched.Status,
		"must still transition to in_progress so the live work session's next request_review call succeeds")

	select {
	case ev := <-ch:
		t.Fatalf("expected no notification for a genuinely active (non-stale) session, got %+v", ev)
	case <-time.After(300 * time.Millisecond):
		// expected: no notification fired
	}

	open, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "must not durably mark the item stuck when the threshold isn't exceeded")
}

// TestNotifyIfActiveWorkSessionStale_should_notFire_When_IdleUnder15Min and
// TestNotifyIfActiveWorkSessionStale_should_fire_When_IdleOver15Min pin the
// exact ADR-001 boundary (maxReworkBlockStaleness=15min) — distinct from the
// two tests above, which use values well clear of the boundary on either
// side. These two sit right at the edge to catch an off-by-one in the `idle
// <= threshold` comparison.
func TestNotifyIfActiveWorkSessionStale_should_notFire_When_IdleUnder15Min(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	ctx := context.Background()

	stopper := &mockSessionStopper{
		liveUUIDs: map[string]bool{"active-work-uuid": true},
		staleFor:  map[string]time.Duration{"active-work-uuid": 10 * time.Minute},
	}
	svc.SetSessionStopper(stopper)
	bus := events.NewEventBus(4)
	svc.SetEventBus(bus)

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Item idle 10m — under the 15m threshold",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID: item.ID, SessionUUID: "active-work-uuid", SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	require.NoError(t, svc.AutoReopenAfterFailedReview(ctx, item.ID))

	open, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "10 min idle must not durably mark the item stuck (15 min threshold)")
}

func TestNotifyIfActiveWorkSessionStale_should_fire_When_IdleOver15Min(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	ctx := context.Background()

	stopper := &mockSessionStopper{
		liveUUIDs: map[string]bool{"active-work-uuid": true},
		staleFor:  map[string]time.Duration{"active-work-uuid": 20 * time.Minute},
	}
	svc.SetSessionStopper(stopper)
	bus := events.NewEventBus(4)
	svc.SetEventBus(bus)

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Item idle 20m — over the 15m threshold",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID: item.ID, SessionUUID: "active-work-uuid", SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	require.NoError(t, svc.AutoReopenAfterFailedReview(ctx, item.ID))

	open, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, domain.StuckReasonReworkBlockedStale, open[0].Reason)
}

// TestNotifyIfActiveWorkSessionStale_should_skipGracefully_When_StatusPreconditionMismatched
// confirms MarkStuck's expectedStatus precondition is honored: if the item
// has moved off review by the time notifyIfActiveWorkSessionStale runs (a
// race between the AutoReopenAfterFailedReview read and this write), the
// function must not error or panic — it silently skips the durable mark
// (the notification-publish behavior is unaffected, covered by the two tests
// above).
func TestNotifyIfActiveWorkSessionStale_should_skipGracefully_When_StatusPreconditionMismatched(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	ctx := context.Background()

	stopper := &mockSessionStopper{
		liveUUIDs: map[string]bool{"active-work-uuid": true},
		staleFor:  map[string]time.Duration{"active-work-uuid": 20 * time.Minute},
	}
	svc.SetSessionStopper(stopper)
	bus := events.NewEventBus(4)
	svc.SetEventBus(bus)

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Item that moved off review before the mark call",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID: item.ID, SessionUUID: "active-work-uuid", SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	sessions, err := storage.ListItemSessions(ctx, item.ID)
	require.NoError(t, err)

	// Simulate the race directly: move the item off review, then call the
	// function under test with the (now stale) pre-read state, exactly as
	// AutoReopenAfterFailedReview would if something else touched the item
	// between its own read and this call.
	_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, session.BacklogStatusInProgress, nil, "test")
	require.NoError(t, err)

	require.NotPanics(t, func() {
		svc.notifyIfActiveWorkSessionStale(ctx, item.ID, item.Title, sessions)
	})

	open, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "MarkStuck's expectedStatus precondition must have prevented the write")
}

// TestNotifyIfActiveWorkSessionStale_should_addSecondOpenReason_When_ItemAlreadyHasBouncingRowOpen
// is a coexistence smoke test (plan.md Task 2.1.1d): the multi-row
// BacklogStuckState model already supports multiple simultaneous open
// reasons per item (see notifyReworkCapHit's own tests) — this confirms
// MarkStuck-ing rework_blocked_stale onto an item that already has an open,
// unrelated row does not clobber or conflict with it, catching a regression
// if a future change accidentally assumes one-reason-per-item. Uses
// "bouncing" as the pre-existing unrelated reason (not rework_cap): since
// AutoReopenAfterFailedReview now always transitions the item to
// in_progress when reused with a live work session (see
// TestAutoReopenAfterFailedReview_ActiveWorkSession_StillTransitionsToInProgress),
// it also resolves any open rework_cap/abandoned_review rows as part of that
// transition — using rework_cap here would conflate "did the transition
// correctly resolve a no-longer-applicable row" with the actual thing this
// test checks (do two genuinely unrelated open reasons coexist).
func TestNotifyIfActiveWorkSessionStale_should_addSecondOpenReason_When_ItemAlreadyHasBouncingRowOpen(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	ctx := context.Background()

	stopper := &mockSessionStopper{
		liveUUIDs: map[string]bool{"active-work-uuid": true},
		staleFor:  map[string]time.Duration{"active-work-uuid": 20 * time.Minute},
	}
	svc.SetSessionStopper(stopper)
	bus := events.NewEventBus(4)
	svc.SetEventBus(bus)

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Item already parked on bouncing",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID: item.ID, SessionUUID: "active-work-uuid", SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	applied, err := storage.MarkStuck(ctx, item.ID, domain.StuckReasonBouncing, session.BacklogStatusReview, "bouncing between review and in_progress")
	require.NoError(t, err)
	require.True(t, applied)

	require.NoError(t, svc.AutoReopenAfterFailedReview(ctx, item.ID))

	open, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 2, "both the pre-existing bouncing row and the new rework_blocked_stale row must coexist")
	reasons := map[domain.StuckReason]bool{}
	for _, row := range open {
		reasons[row.Reason] = true
	}
	assert.True(t, reasons[domain.StuckReasonBouncing])
	assert.True(t, reasons[domain.StuckReasonReworkBlockedStale])
}

// TestAutoRespawnReview_ReworkCapHit_LeavesInReviewAndNotifies is the regression
// test for the runaway-loop risk this fix introduces if left unbounded: unlike
// AutoReopenAfterFailedReview/AutoReopenForPRFix, AutoRespawnReview never adds a
// work session, so their work-session-counting cap would never trip here. Without
// its own cap on *review* sessions, an item whose underlying work is genuinely
// incomplete (verdict never PASSes) would re-review forever, once per
// abandoned_review occurrence. This asserts the cap — reusing the same
// maxAutoReworkIterations threshold and notifyReworkCapHit pattern as the other
// two rework loops — actually stops it.
func TestAutoRespawnReview_ReworkCapHit_LeavesInReviewAndNotifies(t *testing.T) {
	storage := createTestStorage(t)
	// Explicit cap (rather than relying on the nil-config default, which is 20 —
	// raised from 3 since real, ultimately-fixable items were routinely tripping
	// the old default before they were actually stuck) so this test's intent
	// (verify the cap-hit behavior itself) stays independent of that default's
	// exact value.
	svc := NewBacklogService(storage, nil, &config.Config{MaxAutoReworkIterations: 3}, nil, nil, nil)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "Repeatedly-failing review item",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	// Seed maxAutoReworkIterations (3) prior, already-ended review sessions —
	// the shape left behind by 3 prior abandoned_review respawns that each ran
	// to completion without a PASS verdict.
	for i := 0; i < 3; i++ {
		is, isErr := storage.CreateItemSession(context.Background(), session.ItemSessionData{
			ItemID:      item.ID,
			SessionUUID: "prior-re-review-" + string(rune('a'+i)),
			SessionRole: session.SessionRoleReview,
		})
		require.NoError(t, isErr)
		require.NoError(t, storage.UpdateItemSessionEnded(context.Background(), is.ID, time.Now()))
	}

	respawnErr := svc.AutoRespawnReview(context.Background(), item.ID)
	require.NoError(t, respawnErr, "hitting the cap is an expected outcome, not a failure")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusReview), fetched.Status, "item must stay in review, not spin")

	open, err := storage.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, domain.StuckReasonReworkCap, open[0].Reason, "cap hit must write the same durable rework_cap row the other two rework loops use")
}

// TestAutoRespawnReview_ReworkCapHit_UsesConfiguredCap_When_MaxAutoReworkIterationsSet
// verifies the rework cap is read from config.Config, not hardcoded — a cap of 1
// must trip after a single prior review session, not the default 3.
func TestAutoRespawnReview_ReworkCapHit_UsesConfiguredCap_When_MaxAutoReworkIterationsSet(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, &config.Config{MaxAutoReworkIterations: 1}, nil, nil, nil)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "Configured low-cap item",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	is, isErr := storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "prior-re-review-only-one",
		SessionRole: session.SessionRoleReview,
	})
	require.NoError(t, isErr)
	require.NoError(t, storage.UpdateItemSessionEnded(context.Background(), is.ID, time.Now()))

	respawnErr := svc.AutoRespawnReview(context.Background(), item.ID)
	require.NoError(t, respawnErr, "hitting the configured cap is an expected outcome, not a failure")

	open, err := storage.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, domain.StuckReasonReworkCap, open[0].Reason)
	assert.Contains(t, open[0].Context, "1-iteration rework cap", "context must reflect the configured cap, not the default")
}

// TestAutoRespawnReview_ReworkCapOverride_AllowsMoreRoundsThanGlobalDefault is the
// regression test for the per-item rework-cap override: an item whose
// ReworkCapOverride is set higher than the global default (3) must keep
// auto-respawning past that default, using its own cap instead.
func TestAutoRespawnReview_ReworkCapOverride_AllowsMoreRoundsThanGlobalDefault(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, &config.Config{MaxAutoReworkIterations: 3}, nil, nil, nil)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	override := 5
	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:             "Item with a raised rework cap",
		RepoPath:          repoPath,
		Status:            string(session.BacklogStatusReview),
		ReworkCapOverride: &override,
	})
	require.NoError(t, err)

	// 4 prior review sessions: past the global default (3) but under this
	// item's override (5) — the automatic respawn must still proceed.
	for i := 0; i < 4; i++ {
		is, isErr := storage.CreateItemSession(context.Background(), session.ItemSessionData{
			ItemID:      item.ID,
			SessionUUID: "prior-re-review-" + string(rune('a'+i)),
			SessionRole: session.SessionRoleReview,
		})
		require.NoError(t, isErr)
		require.NoError(t, storage.UpdateItemSessionEnded(context.Background(), is.ID, time.Now()))
	}

	respawnErr := svc.AutoRespawnReview(context.Background(), item.ID)
	require.NoError(t, respawnErr)

	open, err := storage.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	assert.Empty(t, open, "an item under its own raised cap must not be parked as rework_cap")
}

// TestAutoRespawnReview_ReworkCapOverride_ZeroMeansUnlimited verifies the 0
// sentinel disables the cap entirely for that item, even with many prior
// review sessions well past the global default.
func TestAutoRespawnReview_ReworkCapOverride_ZeroMeansUnlimited(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, &config.Config{MaxAutoReworkIterations: 3}, nil, nil, nil)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	unlimited := 0
	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:             "Item with an unlimited rework cap",
		RepoPath:          repoPath,
		Status:            string(session.BacklogStatusReview),
		ReworkCapOverride: &unlimited,
	})
	require.NoError(t, err)

	for i := 0; i < 10; i++ {
		is, isErr := storage.CreateItemSession(context.Background(), session.ItemSessionData{
			ItemID:      item.ID,
			SessionUUID: "prior-re-review-" + string(rune('a'+i)),
			SessionRole: session.SessionRoleReview,
		})
		require.NoError(t, isErr)
		require.NoError(t, storage.UpdateItemSessionEnded(context.Background(), is.ID, time.Now()))
	}

	respawnErr := svc.AutoRespawnReview(context.Background(), item.ID)
	require.NoError(t, respawnErr)

	open, err := storage.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	assert.Empty(t, open, "override=0 must mean unlimited retries, never hitting rework_cap")
}

// TestAutoRespawnReview_ActiveReviewSession_SkipsWithoutDoubleSpawn verifies
// AutoRespawnReview does not spawn a second, concurrent re-review pass when one
// is already running — the headless re-review path only records its ItemSession
// row after the LLM call completes, so a naive implementation could otherwise
// double-dispatch across two reconcile ticks landing close together.
func TestAutoRespawnReview_ActiveReviewSession_SkipsWithoutDoubleSpawn(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	// A headless pool being wired but never called proves no second attempt fired.
	pool := &fakeHeadlessPool{response: `{"overall":"PASS","summary":"ok","verdicts":[]}`}
	svc.SetHeadlessPool(pool)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "Review item with an active re-review already running",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusReview),
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "active-review-uuid",
		SessionRole: session.SessionRoleReview,
	})
	require.NoError(t, err)

	respawnErr := svc.AutoRespawnReview(context.Background(), item.ID)
	require.NoError(t, respawnErr)
	assert.Empty(t, pool.calls, "must not start a second headless review call while one is already active")
}

// TestAutoRespawnReview_ActiveReviewSession_RecordsRespawnBlockedActive is
// AutoRespawnReview's counterpart to
// TestAutoReopenForPRFix_ActiveWorkSession_RecordsRespawnBlockedActive above
// — same audit-trail gap, third of the three call sites this fix covers.
func TestAutoRespawnReview_ActiveReviewSession_RecordsRespawnBlockedActive(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	pool := &fakeHeadlessPool{response: `{"overall":"PASS","summary":"ok","verdicts":[]}`}
	svc.SetHeadlessPool(pool)
	bus := events.NewEventBus(4)
	svc.SetEventBus(bus)
	subCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "Review item with an active re-review already running",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusReview),
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "active-review-uuid",
		SessionRole: session.SessionRoleReview,
	})
	require.NoError(t, err)

	require.NoError(t, svc.AutoRespawnReview(context.Background(), item.ID))

	open, err := storage.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, domain.StuckReasonRespawnBlockedActive, open[0].Reason)
	assert.Equal(t, item.ID, open[0].ItemID)
	assert.Contains(t, open[0].Context, "active-review-uuid")

	select {
	case ev := <-ch:
		assert.Equal(t, events.EventNotification, ev.Type)
	case <-time.After(2 * time.Second):
		t.Fatal("expected an operator-facing notification when auto-respawn is skipped for an active review session")
	}
}

// TestAutoRespawnReview_NoActiveSession_ResolvesAnyOpenRespawnBlockedActiveRow
// is AutoRespawnReview's counterpart to
// TestAutoRespawnAutonomousWork_NoActiveSession_ResolvesAnyOpenRespawnBlockedActiveRow
// — verifies the inline resolve at the top of AutoRespawnReview clears a
// pre-existing respawn_blocked_active row once its guard passes. This is the
// less critical of the two resolution paths for AutoRespawnReview: its only
// real caller, markAbandonedReview, is backoff-gated and can eventually stop
// re-invoking this function entirely, at which point only the independent
// periodic sweep (reconcileRespawnBlockedActiveResolution,
// session/backlog_lifecycle.go) still guarantees resolution — see
// TestReconcileRespawnBlockedActiveResolution_should_resolveRow_When_BlockingSessionHasEnded
// for that scenario, exercised with no call to AutoRespawnReview at all.
func TestAutoRespawnReview_NoActiveSession_ResolvesAnyOpenRespawnBlockedActiveRow(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	pool := &fakeHeadlessPool{response: `{"overall":"PASS","summary":"ok","verdicts":[]}`}
	svc.SetHeadlessPool(pool)
	svc.SetCapabilityCheck(headless.NewPassedCapabilitySelfCheckForTesting())

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "Review item whose blocking session has since ended",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusReview),
	})
	require.NoError(t, err)
	is, err := storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "ended-review-uuid",
		SessionRole: session.SessionRoleReview,
	})
	require.NoError(t, err)
	require.NoError(t, storage.UpdateItemSessionEnded(context.Background(), is.ID, time.Now()))

	applied, err := storage.MarkStuck(context.Background(), item.ID, domain.StuckReasonRespawnBlockedActive,
		session.BacklogStatusReview, "pre-existing open row from a prior blocked attempt")
	require.NoError(t, err)
	require.True(t, applied)

	require.NoError(t, svc.AutoRespawnReview(context.Background(), item.ID))
	assert.NotEmpty(t, pool.calls, "must actually invoke the headless review call now that nothing is blocking")

	open, err := storage.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	for _, row := range open {
		assert.NotEqual(t, domain.StuckReasonRespawnBlockedActive, row.Reason,
			"the respawn_blocked_active row must be resolved once the guard passes")
	}
}

// TestAutoRespawnReview_DeadWorkSession_TombstonedThenRespawns verifies the
// counterpart to TestAutoReopenForPRFix_DeadWorkSession_TombstonesThenReopens
// for the review-respawn path: a work session that looks open (EndedAt nil)
// but is confirmed dead (no live tmux/CLI process) must be tombstoned so the
// respawn can proceed, rather than blocking forever like the pr_pending live
// bug this mirrors (docs/tasks/backlog-feature-improvement.md).
//
// This fixture's "dead" work session never had an Instance/Worktree row recorded
// for it at all (matching a session that died before its worktree was ever
// persisted) — i.e. exactly the "no real per-item worktree" shape BUG-045 fixed
// resolveCodebaseWorkDir to refuse rather than silently fall back to repoPath (the
// shared main checkout). So "respawns" here means the tombstone unblocks
// AutoRespawnReview from skipping via hasActiveWorkSession and it actually drives
// a re-review attempt through to a recorded verdict — not that the headless pool
// gets a real evidence-backed call, which BUG-045 correctly refuses to spend here.
func TestAutoRespawnReview_DeadWorkSession_TombstonedThenRespawns(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	stopper := &mockSessionStopper{liveUUIDs: map[string]bool{}} // nothing is live
	svc.SetSessionStopper(stopper)
	// This response would (falsely) confirm a BUG-045 regression if the pool were
	// ever actually invoked against repoPath for a dead session with no resolvable
	// worktree — asserting callCount()==0 below proves it is not.
	pool := &fakeHeadlessPool{response: `{"overall":"UNVERIFIABLE","summary":"no evidence","verdicts":[]}`}
	svc.SetHeadlessPool(pool)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "Review item with a dead prior work session",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusReview),
	})
	require.NoError(t, err)
	deadIS, err := storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "dead-work-uuid",
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	respawnErr := svc.AutoRespawnReview(context.Background(), item.ID)
	require.NoError(t, respawnErr, "respawn must proceed (not error/hang) once the dead work session is cleared")
	assert.Equal(t, 0, pool.callCount(), "must not spend a headless call against repoPath when the dead session has no resolvable worktree (BUG-045)")

	deadFetched, err := storage.GetItemSession(context.Background(), deadIS.ID)
	require.NoError(t, err)
	assert.NotNil(t, deadFetched.EndedAt, "the dead work session must be tombstoned")

	outcome, err := storage.GetMostRecentReviewVerdictForItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, session.ReviewVerdictUnverifiable, outcome,
		"must record an explicit UNVERIFIABLE verdict rather than silently reviewing the shared checkout")
}

// TestAutoRespawnReview_NoActiveSession_TriggersReReview verifies the success
// path: an abandoned review item with no active session gets a fresh review pass,
// and (mirroring TestTriggerReReview_HeadlessPassAutoTransitionsToDone) a PASS
// verdict from that respawned review carries the item all the way to done —
// proving the respawn is not just "detected," it actually unsticks the item.
func TestAutoRespawnReview_NoActiveSession_TriggersReReview(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	repoDir := t.TempDir()
	require.NoError(t, os.WriteFile(repoDir+"/README.md", []byte("hello\n"), 0o644))
	pool := &fakeHeadlessPool{response: `{"overall":"PASS","summary":"looks good","verdicts":[{"criterion_index":0,"outcome":"PASS","evidence":"verified"}],"tool_reads":["README.md"]}`}
	svc.SetHeadlessPool(pool)
	svc.SetCapabilityCheck(headless.NewPassedCapabilitySelfCheckForTesting())

	createResp, err := svc.CreateBacklogItem(context.Background(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:    "Abandoned review item",
		RepoPath: repoDir,
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "pending"},
		},
		// SkipTriage prevents CreateBacklogItem's auto-triage goroutine from racing
		// this test's own explicit idea->ready->in_progress->review transitions below
		// (both would otherwise try to move idea->ready concurrently).
		SkipTriage:   true,
		SkipPlanning: true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	for _, status := range []string{
		string(session.BacklogStatusReady),
		string(session.BacklogStatusInProgress),
		string(session.BacklogStatusReview),
	} {
		_, err = svc.TransitionBacklogItemStatus(context.Background(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
			ItemId:       itemID,
			TargetStatus: status,
		}))
		require.NoError(t, err)
	}

	respawnErr := svc.AutoRespawnReview(context.Background(), itemID)
	require.NoError(t, respawnErr)
	assert.NotEmpty(t, pool.calls, "must actually invoke the headless review call, not just detect the item")

	updated, err := svc.GetBacklogItem(context.Background(), connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: itemID}))
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusDone), updated.Msg.Item.Status,
		"a PASS verdict from the respawned review should carry the item to done, same as a manual TriggerReReview call")
}

// --- AutoRespawnTriage: session.TriageRespawner implementation (orphaned_triage
// remediation) — closes the gap where StuckReasonOrphanedTriage was detected and
// notified but never automatically retried (docs/tasks/backlog-feature-improvement.md,
// 2026-07-27 update: items 4f03de7b and 505fb733 sat in "idea" for 2 days). ---

// TestAutoRespawnTriage_should_retriggerTriage_When_ItemStillIdea verifies the happy
// path: an idea-status item (the only status reconcileOrphanedTriageRemediation ever
// calls this for) gets triage re-triggered via the same TriggerTriage entry point a
// manual re-trigger would use.
func TestAutoRespawnTriage_should_retriggerTriage_When_ItemStillIdea(t *testing.T) {
	storage := createTestStorage(t)
	pool := &fakeHeadlessPool{response: validTriageJSON()}
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	svc.SetHeadlessPool(pool)

	repoPath := t.TempDir()
	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "Orphaned triage respawn item",
		Status:   string(session.BacklogStatusIdea),
		Priority: 3,
		RepoPath: repoPath,
	})
	require.NoError(t, err)

	respawnErr := svc.AutoRespawnTriage(t.Context(), item.ID)
	require.NoError(t, respawnErr)

	require.Eventually(t, func() bool {
		return pool.callCount() >= 1
	}, 5*time.Second, 50*time.Millisecond, "must actually invoke the headless triage call, not just detect the item")

	require.Eventually(t, func() bool {
		updated, loadErr := storage.GetBacklogItem(t.Context(), item.ID)
		return loadErr == nil && updated.Status == string(session.BacklogStatusReady)
	}, 5*time.Second, 50*time.Millisecond, "item should transition to ready after the re-triggered headless triage completes")
}

// TestAutoRespawnTriage_should_resetQueuedToIdeaAndRetrigger_When_ItemQueued is the
// regression test for the 2026-08-03 generalization (docs/tasks/backlog-feature-improvement.md,
// item be676dab): a queued item gated on plan approval with no usable triage result must
// have AutoRespawnTriage reset it queued->idea (TriggerTriage only ever accepts idea/ready)
// before re-triggering triage — mirroring the manual "Return to Triage" recovery already
// performed for be676dab, now automated.
func TestAutoRespawnTriage_should_resetQueuedToIdeaAndRetrigger_When_ItemQueued(t *testing.T) {
	storage := createTestStorage(t)
	pool := &fakeHeadlessPool{response: validTriageJSON()}
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	svc.SetHeadlessPool(pool)

	repoPath := t.TempDir()
	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:        "Queued item with no usable plan",
		Status:       string(session.BacklogStatusQueued),
		Priority:     3,
		RepoPath:     repoPath,
		SkipPlanning: false,
		PlanApproved: false,
	})
	require.NoError(t, err)

	respawnErr := svc.AutoRespawnTriage(t.Context(), item.ID)
	require.NoError(t, respawnErr)

	require.Eventually(t, func() bool {
		return pool.callCount() >= 1
	}, 5*time.Second, 50*time.Millisecond, "must actually invoke the headless triage call after resetting to idea")

	require.Eventually(t, func() bool {
		updated, loadErr := storage.GetBacklogItem(t.Context(), item.ID)
		return loadErr == nil && updated.Status == string(session.BacklogStatusReady)
	}, 5*time.Second, 50*time.Millisecond, "item should transition queued->idea->ready after the re-triggered headless triage completes")
}

// TestAutoRespawnTriage_should_noop_When_ItemNoLongerIdea verifies the staleness guard:
// an item that moved off "idea" between the caller's stuck-row query and this async
// call running (e.g. a human already re-triggered triage manually) must not be acted
// on again — mirrors AutoRespawnReview's identical guard for StuckReasonAbandonedReview.
func TestAutoRespawnTriage_should_noop_When_ItemNoLongerIdea(t *testing.T) {
	storage := createTestStorage(t)
	pool := &fakeHeadlessPool{response: validTriageJSON()}
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	svc.SetHeadlessPool(pool)

	repoPath := t.TempDir()
	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "Already-progressed item",
		Status:   string(session.BacklogStatusIdea),
		Priority: 3,
		RepoPath: repoPath,
	})
	require.NoError(t, err)

	_, err = storage.TransitionBacklogItemStatus(t.Context(), item.ID, session.BacklogStatusReady, nil, session.TriggeredBySystem)
	require.NoError(t, err)

	respawnErr := svc.AutoRespawnTriage(t.Context(), item.ID)
	require.NoError(t, respawnErr)
	assert.Empty(t, pool.calls, "an item that already moved off idea must not have triage re-triggered")
}

// --- AutoReopenForPRFix: proactive branch sync with main (Task 2.1.6d) ---
//
// Before AutoReopenForPRFix respawns a fix session, it now merges main into the
// worktree behind the currently open, failing PR — preventive sync rather than
// reactive (the PR #157 pattern: a branch drifted from main with nobody proactively
// resyncing it until it hit a hard conflict). These three tests cover the merge, the
// conflict, and the no-op cases via syncPRBranchWithMain.

// chmodRecursive sets mode on every entry under root (root included). A plain
// os.Chmod(root, mode) only affects root itself, which isn't enough to make a git
// remote's push targets (.git/objects/**, .git/refs/**) unwritable — git creates new
// files a few directories deep, and those subdirectories keep their own (writable)
// permissions unless changed individually.
func chmodRecursive(t *testing.T, root string, mode os.FileMode) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chmod(path, mode)
	})
	require.NoError(t, err)
}

// runGitTestCmd runs a git command in dir and fails the test on error.
func runGitTestCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...) //nolint:norawexec // test helper, blocking CombinedOutput, no zombie risk
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s failed: %s", strings.Join(args, " "), out)
	return string(out)
}

// setupPRFixSyncRepo builds an "origin" repo and a clone of it (repoPath) with an
// "origin" remote already configured — mirroring production, where item.RepoPath is a
// checkout whose worktrees (git worktree add) share the same .git and inherit that
// remote. It returns the origin dir and the clone (repoPath), both with a branch
// explicitly named "main" (prFixMainBranch's target).
func setupPRFixSyncRepo(t *testing.T) (originDir, repoPath string) {
	t.Helper()
	originDir = t.TempDir()
	initGitRepoWithCommit(t, originDir)
	runGitTestCmd(t, originDir, "branch", "-M", "main")

	repoPath = filepath.Join(t.TempDir(), "clone")
	cloneCmd := exec.Command("git", "clone", originDir, repoPath) //nolint:norawexec // test helper
	out, err := cloneCmd.CombinedOutput()
	require.NoError(t, err, "git clone failed: %s", out)
	runGitTestCmd(t, repoPath, "config", "user.email", "test@example.com")
	runGitTestCmd(t, repoPath, "config", "user.name", "Test")
	return originDir, repoPath
}

// attachPRFixWorkSession records a completed work ItemSession for item plus the
// GitWorktreeData needed for syncPRBranchWithMain's GetWorktreeDataBySessionUUID
// lookup to find worktreePath — the worktree behind the currently open PR.
//
// Persists straight through repo.Create (the same low-level path
// session/ent_repository_test.go's TestEntRepository_Worktree uses) rather than going
// through session.FromInstanceData/storage.AddInstance: constructing a live *Instance
// pulls in gitManager/state-machine machinery that isn't needed here and, for a
// worktree directory git created directly (not through the app's own worktree
// registry), triggered an unrelated existence-check cleanup that deleted the very
// worktree this helper is trying to register.
func attachPRFixWorkSession(t *testing.T, storage *session.Storage, repo *session.EntRepository, item *session.BacklogItemData, sessionUUID, repoPath, worktreePath, branchName string) {
	t.Helper()
	is, err := storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	require.NoError(t, storage.UpdateItemSessionEnded(context.Background(), is.ID, time.Now().Add(-time.Hour)))

	baseCommitSHA := strings.TrimSpace(runGitTestCmd(t, worktreePath, "rev-parse", "HEAD"))
	now := time.Now()
	require.NoError(t, repo.Create(context.Background(), session.InstanceData{
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
			BaseCommitSHA: baseCommitSHA,
		},
	}))
}

// createTestStorageWithRepo is createTestStorage but also returns the underlying
// *session.EntRepository, needed by attachPRFixWorkSession to persist worktree data
// via the low-level repo.Create path.
func createTestStorageWithRepo(t *testing.T) (*session.Storage, *session.EntRepository) {
	t.Helper()
	testDir := t.TempDir()
	repo, err := session.NewEntRepository(session.WithDatabasePath(testDir + "/sessions.db"))
	require.NoError(t, err)
	t.Cleanup(func() { repo.Close() })
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)
	return storage, repo
}

// fakeRepoWatchRemover is a test double implementing services.RepoWatchRemover,
// recording every call so a test can assert on which repo paths were told to
// stop being watched.
type fakeRepoWatchRemover struct {
	removed []string
}

func (f *fakeRepoWatchRemover) RemoveRepo(repoPath string) {
	f.removed = append(f.removed, repoPath)
}

// TestCleanupItemWorktreesExcept_should_tellScannerToStopWatching_When_WorktreeCleanupSucceeds
// is the regression test for BUG-034: cleaning up a completed backlog rework
// round's worktree (the highest-volume real path removing worktrees from
// disk — every reopen/rework cycle calls this) must also tell the
// unfinished-changes scanner to stop watching that path, so it doesn't keep
// rescanning a directory that no longer exists forever.
func TestCleanupItemWorktreesExcept_should_tellScannerToStopWatching_When_WorktreeCleanupSucceeds(t *testing.T) {
	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	const workBranch = "backlog/cleanup-test"
	workWT := filepath.Join(t.TempDir(), "work-wt")
	runGitTestCmd(t, repoPath, "worktree", "add", "-b", workBranch, workWT)

	storage, repo := createTestStorageWithRepo(t)
	svc := NewBacklogService(storage, &mockSessionCreator{}, nil, nil, nil, nil)
	remover := &fakeRepoWatchRemover{}
	svc.SetRepoWatchRemover(remover)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "Cleanup test item",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)
	attachPRFixWorkSession(t, storage, repo, item, "cleanup-work-uuid", repoPath, workWT, workBranch)

	sessions, err := storage.ListItemSessions(context.Background(), item.ID)
	require.NoError(t, err)

	svc.cleanupItemWorktreesExcept(context.Background(), sessions, "")

	require.Len(t, remover.removed, 1, "RemoveRepo must be called exactly once after the worktree is actually cleaned up")
	assert.Equal(t, workWT, remover.removed[0])
	_, statErr := os.Stat(workWT)
	assert.True(t, os.IsNotExist(statErr), "sanity: the worktree directory must actually be gone")
}

// TestCleanupItemWorktreesExcept_should_notTellScannerToStopWatching_When_PathIsExempted
// verifies the exceptPath skip (a reopen/rework spawn reusing the same
// worktree across revisions) also skips telling the scanner to stop watching
// it — that worktree is still in active use, not actually gone.
func TestCleanupItemWorktreesExcept_should_notTellScannerToStopWatching_When_PathIsExempted(t *testing.T) {
	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	const workBranch = "backlog/cleanup-except-test"
	workWT := filepath.Join(t.TempDir(), "work-wt")
	runGitTestCmd(t, repoPath, "worktree", "add", "-b", workBranch, workWT)

	storage, repo := createTestStorageWithRepo(t)
	svc := NewBacklogService(storage, &mockSessionCreator{}, nil, nil, nil, nil)
	remover := &fakeRepoWatchRemover{}
	svc.SetRepoWatchRemover(remover)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "Cleanup except-path test item",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)
	attachPRFixWorkSession(t, storage, repo, item, "cleanup-except-work-uuid", repoPath, workWT, workBranch)

	sessions, err := storage.ListItemSessions(context.Background(), item.ID)
	require.NoError(t, err)

	svc.cleanupItemWorktreesExcept(context.Background(), sessions, workWT)

	assert.Empty(t, remover.removed, "the exempted, still-in-use worktree must not be reported as removed")
	_, statErr := os.Stat(workWT)
	assert.NoError(t, statErr, "sanity: the exempted worktree directory must still exist")
}

// TestAutoReopenForPRFix_should_MergeAndPushMain_When_BranchIsStaleButMergesCleanly
// verifies the preventive-sync path: a fix landed on main after the PR's branch was
// created (drift unrelated to the PR's own diff). AutoReopenForPRFix must merge main
// into the PR's branch, push the merge back to origin, and tell the spawned session it
// did so.
func TestAutoReopenForPRFix_should_MergeAndPushMain_When_BranchIsStaleButMergesCleanly(t *testing.T) {
	originDir, repoPath := setupPRFixSyncRepo(t)

	const workBranch = "backlog/pr-fix-clean-merge"
	workWT := filepath.Join(t.TempDir(), "work-wt")
	runGitTestCmd(t, repoPath, "worktree", "add", "-b", workBranch, workWT)
	runGitTestCmd(t, workWT, "config", "user.email", "test@example.com")
	runGitTestCmd(t, workWT, "config", "user.name", "Test")

	// The PR's own work: an unrelated new file, committed on the PR branch.
	require.NoError(t, os.WriteFile(filepath.Join(workWT, "pr-work.txt"), []byte("pr work\n"), 0o644))
	runGitTestCmd(t, workWT, "add", "pr-work.txt")
	runGitTestCmd(t, workWT, "commit", "-m", "PR work")

	// A fix lands on main after the branch was created — the drift this sync is meant
	// to catch preventively.
	require.NoError(t, os.WriteFile(filepath.Join(originDir, "main-fix.txt"), []byte("fix on main\n"), 0o644))
	runGitTestCmd(t, originDir, "add", "main-fix.txt")
	runGitTestCmd(t, originDir, "commit", "-m", "fix landed on main")

	storage, repo := createTestStorageWithRepo(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "PR-pending item that drifted from main",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusPRPending),
		PrNumber: 201,
		PrURL:    "https://github.com/example/repo/pull/201",
	})
	require.NoError(t, err)
	attachPRFixWorkSession(t, storage, repo, item, "clean-merge-work-uuid", repoPath, workWT, workBranch)

	reopenErr := svc.AutoReopenForPRFix(context.Background(), item.ID, "CI failing: flaky test")
	require.NoError(t, reopenErr)

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusInProgress), fetched.Status)
	require.Len(t, creator.calls, 1, "a new fix session must be spawned")
	assert.Contains(t, creator.calls[0].prompt, "[Branch sync]",
		"the spawned session's prompt must mention the branch sync outcome")
	assert.Contains(t, creator.calls[0].prompt, "pushed it",
		"a clean merge must be pushed, not just merged locally")

	// The push must have landed on origin: it must now have the PR branch, containing
	// both the PR's own work and the fix that had landed on main. (workWT itself is
	// gone by this point — SpawnSessionFromItem's reopen path cleans up the prior work
	// session's worktree once the new fix session is safely persisted; the merge's
	// staying power comes from having been pushed, not from the local worktree.)
	_, refErr := exec.Command("git", "-C", originDir, "rev-parse", "refs/heads/"+workBranch).CombinedOutput() //nolint:norawexec // test assertion
	require.NoError(t, refErr, "the merge must be pushed to the PR's branch on origin")
	_, prWorkErr := exec.Command("git", "-C", originDir, "cat-file", "-e", workBranch+":pr-work.txt").CombinedOutput() //nolint:norawexec // test assertion
	assert.NoError(t, prWorkErr, "pushed branch must still contain the PR's own work")
	_, mainFixErr := exec.Command("git", "-C", originDir, "cat-file", "-e", workBranch+":main-fix.txt").CombinedOutput() //nolint:norawexec // test assertion
	assert.NoError(t, mainFixErr, "pushed branch must contain the fix that had landed on main")
}

// TestAutoReopenForPRFix_should_IncludeConflictsInFixContext_When_MergingMainConflicts
// verifies the conflict path: when main and the PR's branch touch the same lines, the
// merge must be aborted (leaving the worktree clean and nothing pushed) and the
// conflicting file paths must be folded into the fix context handed to the spawned
// session, so resolving them against main becomes part of the fix.
func TestAutoReopenForPRFix_should_IncludeConflictsInFixContext_When_MergingMainConflicts(t *testing.T) {
	originDir, repoPath := setupPRFixSyncRepo(t)

	const workBranch = "backlog/pr-fix-conflict"
	workWT := filepath.Join(t.TempDir(), "work-wt")
	runGitTestCmd(t, repoPath, "worktree", "add", "-b", workBranch, workWT)
	runGitTestCmd(t, workWT, "config", "user.email", "test@example.com")
	runGitTestCmd(t, workWT, "config", "user.name", "Test")

	// The PR branch edits README.md (created by initGitRepoWithCommit).
	require.NoError(t, os.WriteFile(filepath.Join(workWT, "README.md"), []byte("# PR Edit\n"), 0o644))
	runGitTestCmd(t, workWT, "add", "README.md")
	runGitTestCmd(t, workWT, "commit", "-m", "PR edits README")

	// Main edits the same line differently.
	require.NoError(t, os.WriteFile(filepath.Join(originDir, "README.md"), []byte("# Main Edit\n"), 0o644))
	runGitTestCmd(t, originDir, "add", "README.md")
	runGitTestCmd(t, originDir, "commit", "-m", "main edits README")

	storage, repo := createTestStorageWithRepo(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "PR-pending item that conflicts with main",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusPRPending),
		PrNumber: 202,
		PrURL:    "https://github.com/example/repo/pull/202",
	})
	require.NoError(t, err)
	attachPRFixWorkSession(t, storage, repo, item, "conflict-work-uuid", repoPath, workWT, workBranch)

	reopenErr := svc.AutoReopenForPRFix(context.Background(), item.ID, "CI failing: merge conflict risk")
	require.NoError(t, reopenErr)

	require.Len(t, creator.calls, 1, "a new fix session must be spawned even when the sync conflicts")
	assert.Contains(t, creator.calls[0].prompt, "conflicts", "the fix context must mention the merge conflict")
	assert.Contains(t, creator.calls[0].prompt, "README.md", "the fix context must name the conflicting file")

	// Nothing must have been pushed. (The worktree's own clean-abort behavior is
	// covered directly by session/git's TestMergeMainIntoWorktree_should_ReportConflictedAndAbort_*;
	// workWT itself is gone by this point — see the comment at the end of the clean-merge
	// test above for why.)
	_, refErr := exec.Command("git", "-C", originDir, "rev-parse", "refs/heads/"+workBranch).CombinedOutput() //nolint:norawexec // test assertion
	assert.Error(t, refErr, "a conflicted merge must never be pushed to origin")
}

// TestAutoReopenForPRFix_should_SkipSyncNote_When_BranchAlreadyUpToDateWithMain verifies
// the no-op case: when the PR's branch already contains everything on main, the sync
// must do nothing observable — no push, no "[Branch sync]" note cluttering the fix
// context handed to the spawned session.
func TestAutoReopenForPRFix_should_SkipSyncNote_When_BranchAlreadyUpToDateWithMain(t *testing.T) {
	_, repoPath := setupPRFixSyncRepo(t)

	const workBranch = "backlog/pr-fix-up-to-date"
	workWT := filepath.Join(t.TempDir(), "work-wt")
	runGitTestCmd(t, repoPath, "worktree", "add", "-b", workBranch, workWT)
	runGitTestCmd(t, workWT, "config", "user.email", "test@example.com")
	runGitTestCmd(t, workWT, "config", "user.name", "Test")

	// No further commits anywhere — the branch already contains main's tip.

	storage, repo := createTestStorageWithRepo(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "PR-pending item already in sync with main",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusPRPending),
		PrNumber: 203,
		PrURL:    "https://github.com/example/repo/pull/203",
	})
	require.NoError(t, err)
	attachPRFixWorkSession(t, storage, repo, item, "up-to-date-work-uuid", repoPath, workWT, workBranch)

	reopenErr := svc.AutoReopenForPRFix(context.Background(), item.ID, "CI failing: flaky test")
	require.NoError(t, reopenErr)

	require.Len(t, creator.calls, 1)
	assert.NotContains(t, creator.calls[0].prompt, "[Branch sync]",
		"an already-synced branch must not add sync noise to the fix context")
}

// TestAutoReopenForPRFix_should_ReportUnpushedMerge_When_PushFails verifies the
// merge-succeeds-but-push-fails path: the merge must still happen locally, and the fix
// context must say the merge could not be pushed and give an explicit, actionable
// command against the shared repo checkout (not the worktree, which SpawnSessionFromItem
// deletes once the new fix session is spawned) — see syncPRBranchWithMain's push-error
// branch.
func TestAutoReopenForPRFix_should_ReportUnpushedMerge_When_PushFails(t *testing.T) {
	originDir, repoPath := setupPRFixSyncRepo(t)

	const workBranch = "backlog/pr-fix-push-fails"
	workWT := filepath.Join(t.TempDir(), "work-wt")
	runGitTestCmd(t, repoPath, "worktree", "add", "-b", workBranch, workWT)
	runGitTestCmd(t, workWT, "config", "user.email", "test@example.com")
	runGitTestCmd(t, workWT, "config", "user.name", "Test")

	// A fix lands on main so there's something to merge (and therefore push).
	require.NoError(t, os.WriteFile(filepath.Join(originDir, "main-fix.txt"), []byte("fix on main\n"), 0o644))
	runGitTestCmd(t, originDir, "add", "main-fix.txt")
	runGitTestCmd(t, originDir, "commit", "-m", "fix landed on main")

	// Make origin's .git tree unwritable (recursively — a top-level chmod alone leaves
	// .git/objects and .git/refs writable) so the fetch+merge (read-only) still
	// succeeds but the subsequent push fails. Restored before t.TempDir() cleanup runs.
	originGitDir := filepath.Join(originDir, ".git")
	chmodRecursive(t, originGitDir, 0o555)
	t.Cleanup(func() { chmodRecursive(t, originGitDir, 0o755) })

	storage, repo := createTestStorageWithRepo(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "PR-pending item whose merge can't be pushed",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusPRPending),
		PrNumber: 204,
		PrURL:    "https://github.com/example/repo/pull/204",
	})
	require.NoError(t, err)
	attachPRFixWorkSession(t, storage, repo, item, "push-fails-work-uuid", repoPath, workWT, workBranch)

	reopenErr := svc.AutoReopenForPRFix(context.Background(), item.ID, "CI failing: flaky test")
	require.NoError(t, reopenErr, "a push failure during sync must not block the fix spawn")

	require.Len(t, creator.calls, 1, "a new fix session must be spawned even when the sync's push fails")
	prompt := creator.calls[0].prompt
	assert.Contains(t, prompt, "could not push", "the fix context must say the merge could not be pushed")
	assert.Contains(t, prompt, workBranch, "the fix context must name the affected branch")
	assert.Contains(t, prompt, "git -C "+repoPath, "the fix context must give an actionable command against the shared repo checkout")
}

// TestAutoReopenForPRFix_should_SpawnNormally_When_SyncFetchFails verifies that a sync
// failure unrelated to the merge outcome (here: origin unreachable, so the fetch itself
// errors) is swallowed exactly like syncPRBranchWithMain's other best-effort failure
// paths — no "[Branch sync]" note, and the fix session is spawned normally rather than
// being blocked by a sync-layer problem.
func TestAutoReopenForPRFix_should_SpawnNormally_When_SyncFetchFails(t *testing.T) {
	_, repoPath := setupPRFixSyncRepo(t)

	const workBranch = "backlog/pr-fix-fetch-fails"
	workWT := filepath.Join(t.TempDir(), "work-wt")
	runGitTestCmd(t, repoPath, "worktree", "add", "-b", workBranch, workWT)
	runGitTestCmd(t, workWT, "config", "user.email", "test@example.com")
	runGitTestCmd(t, workWT, "config", "user.name", "Test")

	// Break the "origin" remote so MergeMainIntoWorktree's fetch fails outright.
	runGitTestCmd(t, workWT, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "does-not-exist"))

	storage, repo := createTestStorageWithRepo(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "PR-pending item whose sync fetch fails",
		RepoPath: repoPath,
		Status:   string(session.BacklogStatusPRPending),
		PrNumber: 205,
		PrURL:    "https://github.com/example/repo/pull/205",
	})
	require.NoError(t, err)
	attachPRFixWorkSession(t, storage, repo, item, "fetch-fails-work-uuid", repoPath, workWT, workBranch)

	reopenErr := svc.AutoReopenForPRFix(context.Background(), item.ID, "CI failing: flaky test")
	require.NoError(t, reopenErr, "a sync fetch failure must not block the fix spawn")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusInProgress), fetched.Status)
	require.Len(t, creator.calls, 1, "a new fix session must still be spawned when the sync's fetch fails")
	assert.NotContains(t, creator.calls[0].prompt, "[Branch sync]",
		"a swallowed sync error must not add a sync note to the fix context")
}

// --- Epic 1.6: ItemSessionSummary.PipelineModeSnapshot/SnapshotHash ---

// TestSpawnSessionFromItem_should_SnapshotResolvedModeSlugAndContentHash_When_SessionFirstStarts
// verifies that spawning a work session for an item with a non-default PipelineMode
// records BOTH the resolved mode slug and its content hash (as computed by
// PipelineEngine.ContentHashFor at that moment) onto the new ItemSession row — Story
// 1.6.2's core acceptance criterion. NOTE: svc.pipelineEngine is wired directly here
// (this package's test can reach the unexported field) since NewBacklogService's
// constructor does not yet accept a PipelineEngine parameter — that wiring is Epic
// 1.5's job. See the field's doc comment on BacklogService for the full rationale.
func TestSpawnSessionFromItem_should_SnapshotResolvedModeSlugAndContentHash_When_SessionFirstStarts(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	ctx := t.Context()

	pmRepo := session.NewEntPipelineModeRepository(storage.GetEntClient())
	_, err := pmRepo.Create(ctx, session.PipelineModeCreateInput{
		Slug:                 "quick",
		Name:                 "Quick Fix",
		Enabled:              true,
		TriagePromptTemplate: "quick-mode triage prompt",
	})
	require.NoError(t, err)

	engine, err := session.NewPipelineEngine(pmRepo)
	require.NoError(t, err)
	svc.pipelineEngine = engine

	wantHash, ok := engine.ContentHashFor(session.PipelineMode("quick"))
	require.True(t, ok, "setup: 'quick' must resolve to a content hash")
	require.NotEmpty(t, wantHash)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	createResp, err := svc.CreateBacklogItem(ctx, connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:        "quick-mode item",
		RepoPath:     repoPath,
		PipelineMode: strPtr("quick"),
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "pending"},
		},
		SkipTriage:   true,
		SkipPlanning: true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	_, err = svc.TransitionBacklogItemStatus(ctx, connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       itemID,
		TargetStatus: "ready",
	}))
	require.NoError(t, err)

	_, err = svc.SpawnSessionFromItem(ctx, connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID}))
	require.NoError(t, err)

	sessions, err := storage.ListItemSessions(ctx, itemID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, "quick", sessions[0].PipelineModeSnapshot)
	assert.Equal(t, wantHash, sessions[0].PipelineModeSnapshotHash)
}

// spawnReadyItemWithActiveWorkSession is shared setup for the two
// TestSpawnSessionFromItem_should_..._When_BlockedByActiveWorkSession tests
// below: creates a ready item, spawns its first work session, and returns
// that session's UUID so the caller can wire a mockSessionStopper's
// liveUUIDs/staleFor maps before attempting the blocked second spawn.
func spawnReadyItemWithActiveWorkSession(t *testing.T, svc *BacklogService, storage *session.Storage, ctx context.Context) (itemID, activeSessionUUID string) {
	t.Helper()
	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	createResp, err := svc.CreateBacklogItem(ctx, connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:        "blocked-spawn test item",
		RepoPath:     repoPath,
		SkipTriage:   true,
		SkipPlanning: true,
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "pending"},
		},
	}))
	require.NoError(t, err)
	itemID = createResp.Msg.Item.Id

	_, err = svc.TransitionBacklogItemStatus(ctx, connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       itemID,
		TargetStatus: "ready",
	}))
	require.NoError(t, err)

	spawnResp, err := svc.SpawnSessionFromItem(ctx, connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID}))
	require.NoError(t, err)
	require.NotEmpty(t, spawnResp.Msg.SessionUuid)

	return itemID, spawnResp.Msg.SessionUuid
}

// TestSpawnSessionFromItem_should_ReportStalled_When_BlockedByActiveWorkSession
// is the regression test for the 2026-07-31 finding in
// docs/tasks/backlog-feature-improvement.md: before this fix, a blocked
// second spawn returned a bare "already active... wait or kill it" error
// with no signal about whether the blocking session was actually making
// progress — a caller (human or agent) had to reconstruct that answer by
// hand via get_session's timestamps, a full diff pull, and a live tmux
// check. This asserts the blocked-spawn error itself now carries the same
// progress signal notifyIfActiveWorkSessionStale already computes for the
// analogous review-reopen path (TimeSinceLastMeaningfulOutput vs.
// maxReworkBlockStaleness), so a caller can tell "stalled" from "still
// working" from this one RPC response.
func TestSpawnSessionFromItem_should_ReportStalled_When_BlockedByActiveWorkSession(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	stopper := &mockSessionStopper{
		liveUUIDs: map[string]bool{},
		staleFor:  map[string]time.Duration{},
	}
	svc.SetSessionStopper(stopper)
	ctx := t.Context()

	itemID, activeUUID := spawnReadyItemWithActiveWorkSession(t, svc, storage, ctx)
	stopper.liveUUIDs[activeUUID] = true
	stopper.staleFor[activeUUID] = 20 * time.Minute // > maxReworkBlockStaleness (15min)

	_, err := svc.SpawnSessionFromItem(ctx, connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeAlreadyExists, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "likely stalled")
	assert.Contains(t, err.Error(), "20m0s")
}

// TestSpawnSessionFromItem_should_ReportStillActive_When_BlockedByActiveWorkSession
// is the healthy-case counterpart above: a blocking session that has produced
// output recently (well within maxReworkBlockStaleness) must be reported as
// active, not stalled — the enrichment must not cry wolf on a genuinely
// healthy in-progress session.
func TestSpawnSessionFromItem_should_ReportStillActive_When_BlockedByActiveWorkSession(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	stopper := &mockSessionStopper{
		liveUUIDs: map[string]bool{},
		staleFor:  map[string]time.Duration{},
	}
	svc.SetSessionStopper(stopper)
	ctx := t.Context()

	itemID, activeUUID := spawnReadyItemWithActiveWorkSession(t, svc, storage, ctx)
	stopper.liveUUIDs[activeUUID] = true
	stopper.staleFor[activeUUID] = 30 * time.Second // well within maxReworkBlockStaleness

	_, err := svc.SpawnSessionFromItem(ctx, connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeAlreadyExists, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "still active")
	assert.NotContains(t, err.Error(), "likely stalled")
}

// TestSpawnSessionFromItem_should_ReportSessionStopperNotWired_When_BlockedByActiveWorkSession
// covers the third of activeWorkSessionBlockedError's four branches: no
// sessionStopper was ever wired via SetSessionStopper (NewBacklogService's
// zero value). The blocked-spawn error must say so explicitly rather than
// silently falling back to a bare "already active" message with no
// indication that the progress signal couldn't even be attempted.
func TestSpawnSessionFromItem_should_ReportSessionStopperNotWired_When_BlockedByActiveWorkSession(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	// Deliberately no svc.SetSessionStopper call.
	ctx := t.Context()

	itemID, _ := spawnReadyItemWithActiveWorkSession(t, svc, storage, ctx)

	_, err := svc.SpawnSessionFromItem(ctx, connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeAlreadyExists, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "progress signal unavailable: sessionStopper not wired")
}

// TestSpawnSessionFromItem_should_ReportNotTrackedLive_When_BlockedByActiveWorkSession
// covers the fourth branch: a sessionStopper is wired and the blocking
// session is live (IsSessionLive true, so spawnSessionAfterGates' 8a orphan
// sweep does NOT tombstone it), but TimeSinceLastMeaningfulOutput itself
// reports live=false — the disagreement mockSessionStopper's
// tslmoOverrideNotLive models (see its doc comment). The blocked-spawn error
// must say the progress signal is unavailable for that reason, not silently
// mislabel it as fresh or stale.
func TestSpawnSessionFromItem_should_ReportNotTrackedLive_When_BlockedByActiveWorkSession(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	stopper := &mockSessionStopper{
		liveUUIDs:            map[string]bool{},
		staleFor:             map[string]time.Duration{},
		tslmoOverrideNotLive: map[string]bool{},
	}
	svc.SetSessionStopper(stopper)
	ctx := t.Context()

	itemID, activeUUID := spawnReadyItemWithActiveWorkSession(t, svc, storage, ctx)
	stopper.liveUUIDs[activeUUID] = true            // IsSessionLive → true: not tombstoned as orphan.
	stopper.tslmoOverrideNotLive[activeUUID] = true // TimeSinceLastMeaningfulOutput → live=false.

	_, err := svc.SpawnSessionFromItem(ctx, connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeAlreadyExists, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "progress signal unavailable: session not currently tracked live")
}

// TestSpawnSessionFromItem_should_SnapshotEmptyHash_When_PipelineModeIsDefaultOrUnresolved
// covers both zero-hash edge cases from Story 1.6.2's acceptance criteria: the default
// mode ("") short-circuits ContentHashFor without touching the cache, and an unresolved
// slug (absent from the cache) falls back via ContentHashFor's ok=false path — both
// must produce PipelineModeSnapshotHash == "", ignoring the ok bool per spec.
func TestSpawnSessionFromItem_should_SnapshotEmptyHash_When_PipelineModeIsDefaultOrUnresolved(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	ctx := t.Context()

	pmRepo := session.NewEntPipelineModeRepository(storage.GetEntClient())
	_, err := pmRepo.Create(ctx, session.PipelineModeCreateInput{
		Slug:                 "quick",
		Name:                 "Quick Fix",
		Enabled:              true,
		TriagePromptTemplate: "quick-mode triage prompt",
	})
	require.NoError(t, err)

	engine, err := session.NewPipelineEngine(pmRepo)
	require.NoError(t, err)
	svc.pipelineEngine = engine

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	testCases := []struct {
		name         string
		pipelineMode *string // nil == omitted (default mode)
	}{
		{name: "default mode omitted", pipelineMode: nil},
		{name: "unresolved slug", pipelineMode: strPtr("missing-mode")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			createResp, err := svc.CreateBacklogItem(ctx, connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
				Title:        "item: " + tc.name,
				RepoPath:     repoPath,
				PipelineMode: tc.pipelineMode,
				AcceptanceCriteria: []*sessionv1.AcCriterion{
					{Index: 0, Text: "test", Status: "pending"},
				},
				SkipTriage:   true,
				SkipPlanning: true,
			}))
			require.NoError(t, err)
			itemID := createResp.Msg.Item.Id

			_, err = svc.TransitionBacklogItemStatus(ctx, connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
				ItemId:       itemID,
				TargetStatus: "ready",
			}))
			require.NoError(t, err)

			_, err = svc.SpawnSessionFromItem(ctx, connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID}))
			require.NoError(t, err)

			sessions, err := storage.ListItemSessions(ctx, itemID)
			require.NoError(t, err)
			require.Len(t, sessions, 1)
			assert.Empty(t, sessions[0].PipelineModeSnapshotHash)
		})
	}
}

// --- Epic 1.5: PipelineEngine wired into the 4 call sites ---

// readCommandFiles reads every file under worktreePath/.claude/commands/backlog/ into
// a name->content map, for comparing two independently-written slash-command sets.
func readCommandFiles(t *testing.T, worktreePath string) map[string]string {
	t.Helper()
	dir := filepath.Join(worktreePath, ".claude", "commands", "backlog")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	files := make(map[string]string, len(entries))
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		require.NoError(t, err)
		files[e.Name()] = string(data)
	}
	return files
}

// TestSpawnAndAttachSessionFromItem_should_ProduceIdenticalCommandFiles_When_SameItemAndModeUsedByBothCallers
// (Story 1.5.2) is the direct regression test for "2 independent WriteSlashCommands
// callers must not drift" (research/pitfalls.md §5 point 1): SpawnSessionFromItem and
// AttachSessionToItem both write .claude/commands/backlog/*.md for the same item, and
// both must go through the SAME shared PipelineEngine so a non-default PipelineMode's
// rendered content is identical regardless of which caller wrote it.
func TestSpawnAndAttachSessionFromItem_should_ProduceIdenticalCommandFiles_When_SameItemAndModeUsedByBothCallers(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	ctx := t.Context()

	pmRepo := session.NewEntPipelineModeRepository(storage.GetEntClient())
	_, err := pmRepo.Create(ctx, session.PipelineModeCreateInput{
		Slug:                  "quick",
		Name:                  "Quick Fix",
		Enabled:               true,
		StatusCommandTemplate: "status for {{item_title}} ({{item_id}})",
		DoneCommandTemplate:   "done {{criteria_index}}: {{criteria_text}}",
		FailCommandTemplate:   "fail {{criteria_index}}: {{criteria_text}}",
		ReviewCommandTemplate: "review {{item_title}}",
		ShipCommandTemplate:   "ship {{item_title}}",
		HelpCommandTemplate:   "help {{item_title}}",
	})
	require.NoError(t, err)

	engine, err := session.NewPipelineEngine(pmRepo)
	require.NoError(t, err)
	svc.pipelineEngine = engine

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	createResp, err := svc.CreateBacklogItem(ctx, connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:        "quick-mode parity item",
		RepoPath:     repoPath,
		PipelineMode: strPtr("quick"),
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "pending"},
		},
		SkipTriage:   true,
		SkipPlanning: true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	_, err = svc.TransitionBacklogItemStatus(ctx, connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       itemID,
		TargetStatus: "ready",
	}))
	require.NoError(t, err)

	// Caller 1: SpawnSessionFromItem writes into the worktree the mock creator reports.
	_, err = svc.SpawnSessionFromItem(ctx, connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID}))
	require.NoError(t, err)
	require.Len(t, creator.calls, 1)
	spawnPath := creator.calls[0].path

	// Caller 2: AttachSessionToItem writes into a second, independent directory.
	attachPath := t.TempDir()
	const attachUUID = "attach-parity-uuid"
	require.NoError(t, storage.AddInstance(&session.Instance{
		Title: "attach-target",
		UUID:  attachUUID,
		Path:  attachPath,
		// Paused (not Active) so LoadInstances doesn't attempt a real cold-restore
		// tmux/claude process start — see the identical comment in
		// TestAttachSessionToItem_WritesContextFileWithPlanArtifactsAndPriorSessions
		// (backlog_service_test.go).
		Status:    session.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}))
	_, err = svc.AttachSessionToItem(ctx, connect.NewRequest(&sessionv1.AttachSessionToItemRequest{
		ItemId:      itemID,
		SessionUuid: attachUUID,
	}))
	require.NoError(t, err)

	spawnFiles := readCommandFiles(t, spawnPath)
	attachFiles := readCommandFiles(t, attachPath)
	assert.Equal(t, spawnFiles, attachFiles,
		"SpawnSessionFromItem and AttachSessionToItem must write byte-identical slash-command file sets for the same item+mode")
	// Sanity: the mode-specific content actually rendered (not the default set).
	assert.Contains(t, spawnFiles["status.md"], "status for quick-mode parity item")
}

// TestTriggerTriage_should_UseModeSpecificTriagePrompt_When_ItemHasNonDefaultPipelineModeAndFirstTriageBranch
// (Story 1.5.3) verifies the FIRST-triage (non-retriage) branch routes through
// PipelineEngine.TriagePromptFor: the LLM call receives the mode's rendered
// TriagePromptTemplate, not BuildHeadlessTriagePrompt's default boilerplate.
// TestTriggerTriage_NeverPublishesUntaggedNotification_OnHeadlessPoolFailureOrSuccess
// (Story 2.4) is the negative-proof regression test for TriggerTriage's headless-pool
// goroutine: it has no events.NewNotificationEvent/eventBus.Publish call at all on its
// success, LLM-call-error, or parse-failure paths — every one of those paths only logs
// and, on failure, ends the ItemSession (server/services/backlog_service_triage.go,
// TriggerTriage's async goroutine). The ONLY notification this goroutine can ever
// publish is notifyTriagePersistFailure (backlog_service_triage.go L224-246), which is
// already unconditionally item_id-tagged. This guards that invariant against
// regression: an untagged notification slipping onto any of these paths would defeat
// the item_id-metadata contract Epic 2 establishes.
func TestTriggerTriage_NeverPublishesUntaggedNotification_OnHeadlessPoolFailureOrSuccess(t *testing.T) {
	waitForTriageSessionEnded := func(t *testing.T, storage *session.Storage, itemID string) {
		t.Helper()
		require.Eventually(t, func() bool {
			sessions, listErr := storage.ListItemSessions(context.Background(), itemID)
			if listErr != nil {
				return false
			}
			for _, is := range sessions {
				if is.Role == session.SessionRoleTriage && is.EndedAt != nil {
					return true
				}
			}
			return false
		}, 5*time.Second, 50*time.Millisecond, "expected the triage ItemSession to be marked ended")
	}

	t.Run("LLMCallError_PublishesNoEvents", func(t *testing.T) {
		storage := createTestStorage(t)
		pool := &fakeHeadlessPool{err: errors.New("simulated LLM failure")}
		svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
		svc.SetHeadlessPool(pool)
		bus := events.NewEventBus(8)
		svc.SetEventBus(bus)

		subCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		eventCh, _ := bus.Subscribe(subCtx)

		item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
			Title:    "triage-llm-failure item",
			Status:   string(session.BacklogStatusIdea),
			Priority: 3,
			RepoPath: t.TempDir(),
		})
		require.NoError(t, err)

		_, trigErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
			ItemId: item.ID,
		}))
		require.NoError(t, trigErr)

		waitForTriageSessionEnded(t, storage, item.ID)

		select {
		case ev := <-eventCh:
			t.Fatalf("expected zero events published on the LLM-call-error path, got type=%s", ev.Type)
		case <-time.After(300 * time.Millisecond):
			// Correct — no notification published.
		}
	})

	t.Run("MalformedResponse_PublishesNoEvents", func(t *testing.T) {
		storage := createTestStorage(t)
		pool := &fakeHeadlessPool{response: "this is not valid JSON at all"}
		svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
		svc.SetHeadlessPool(pool)
		bus := events.NewEventBus(8)
		svc.SetEventBus(bus)

		subCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		eventCh, _ := bus.Subscribe(subCtx)

		item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
			Title:    "triage-malformed-response item",
			Status:   string(session.BacklogStatusIdea),
			Priority: 3,
			RepoPath: t.TempDir(),
		})
		require.NoError(t, err)

		_, trigErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
			ItemId: item.ID,
		}))
		require.NoError(t, trigErr)

		waitForTriageSessionEnded(t, storage, item.ID)

		select {
		case ev := <-eventCh:
			t.Fatalf("expected zero events published on the parse-failure path, got type=%s", ev.Type)
		case <-time.After(300 * time.Millisecond):
			// Correct — no notification published.
		}
	})

	// Sub-case 3 ("valid triage result but storage fake configured to fail
	// UpdateBacklogItem, forcing notifyTriagePersistFailure to fire — assert
	// exactly one event arrives with item_id metadata") is documented via code
	// inspection instead of exercised directly.
	//
	// Verification via code inspection: forcing only s.storage.UpdateBacklogItem
	// (backlog_service_triage.go ~L1900) to fail while
	// UpdateItemSessionTriageResult/TransitionBacklogItemStatus/UpdateItemSessionEnded
	// keep succeeding is not constructible with this package's existing test
	// fixtures. BacklogService.storage is a concrete *session.Storage, not an
	// interface — it cannot be swapped for a test double. *session.Storage's
	// ItemSession-specific methods (CreateItemSession, ListItemSessions,
	// UpdateItemSessionTriageResult, UpdateItemSessionEnded — session/storage.go
	// ~L953-1104) each hard type-assert their internal repo field to
	// *session.EntRepository and fail closed otherwise (`er, ok :=
	// s.repo.(*EntRepository); if !ok { return ... }`). Wrapping that repo in a
	// decorator that overrides only UpdateBacklogItem (which is instead a plain
	// passthrough to the session.Repository interface, session/storage.go:721-723)
	// would make the decorator's dynamic type no longer *EntRepository, breaking
	// every ItemSession call this same goroutine depends on — including the one
	// this test needs to detect completion (UpdateItemSessionEnded). The only
	// other lever — closing the real ent DB connection between TriggerTriage
	// returning and its goroutine's persistence step running — races the
	// fakeHeadlessPool's near-instant response, would fail ALL three persistence
	// calls at once rather than isolating UpdateBacklogItem, and would itself
	// break the EndedAt-based completion signal (UpdateItemSessionEnded would
	// fail too), leaving no reliable way to know the goroutine finished. Per this
	// task's explicit guidance, this sub-case is documented rather than forced
	// with a fragile/contrived fixture.
	//
	// By inspection: notifyTriagePersistFailure (backlog_service_triage.go
	// L224-246) is invoked exactly once, at L1915, guarded by `if
	// len(persistFailures) > 0` — i.e. whenever ANY of
	// UpdateItemSessionTriageResult (L1892), UpdateBacklogItem (L1900), or
	// TransitionBacklogItemStatus (L1907) fails. Its body unconditionally builds
	// `map[string]string{"item_id": itemID}` (L244) as the notification metadata
	// — there is no branch inside it that omits item_id. Since this is also the
	// ONLY events.NewNotificationEvent call reachable from anywhere in
	// TriggerTriage's async goroutine (confirmed by reading the full function
	// body, L1805-1937 — no other Publish/NewNotificationEvent call exists on
	// the success, LLM-error, or parse-error paths, matching the two subtests
	// above), the "exactly one item_id-tagged event on persist failure"
	// invariant is structurally guaranteed rather than left unverified.
}

func TestTriggerTriage_should_UseModeSpecificTriagePrompt_When_ItemHasNonDefaultPipelineModeAndFirstTriageBranch(t *testing.T) {
	storage := createTestStorage(t)
	pool := &fakeHeadlessPool{response: validTriageJSON()}
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	svc.SetHeadlessPool(pool)

	pmRepo := session.NewEntPipelineModeRepository(storage.GetEntClient())
	_, err := pmRepo.Create(t.Context(), session.PipelineModeCreateInput{
		Slug:                 "quick",
		Name:                 "Quick Fix",
		Enabled:              true,
		TriagePromptTemplate: "QUICK MODE TRIAGE: {{item_title}}",
	})
	require.NoError(t, err)
	engine, err := session.NewPipelineEngine(pmRepo)
	require.NoError(t, err)
	svc.pipelineEngine = engine

	repoPath := t.TempDir()
	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:        "quick-mode triage item",
		Status:       string(session.BacklogStatusIdea),
		Priority:     3,
		RepoPath:     repoPath,
		PipelineMode: "quick",
	})
	require.NoError(t, err)

	_, trigErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId: item.ID,
	}))
	require.NoError(t, trigErr)

	require.Eventually(t, func() bool {
		return pool.callCount() == 1
	}, 5*time.Second, 50*time.Millisecond, "expected exactly one headless triage call")

	gotPrompt := pool.firstCall().userPrompt
	assert.Contains(t, gotPrompt, "QUICK MODE TRIAGE: quick-mode triage item",
		"expected the mode-specific rendered triage prompt, got: %s", gotPrompt)
	assert.NotContains(t, gotPrompt, "Perform pre-implementation triage",
		"sanity: the default BuildHeadlessTriagePrompt's boilerplate must not appear when a non-default mode is wired")
}

// TestTriggerTriage_should_UseUnmodifiedRetriagePrompt_When_RetriagingRegardlessOfPipelineMode
// (Story 1.5.3) proves the retriage (feedback-driven refine) branch stays on
// BuildHeadlessRetriagePrompt directly, even when item.PipelineMode is non-default with a
// custom TriagePromptTemplate — "refine the existing plan" is mode-independent
// (research/architecture.md §3), and this seam must not have accidentally routed it
// through PipelineEngine too.
func TestTriggerTriage_should_UseUnmodifiedRetriagePrompt_When_RetriagingRegardlessOfPipelineMode(t *testing.T) {
	storage := createTestStorage(t)
	secondResponse := `{"summary":"revised summary","suggestions":[],"tasks":[{"text":"revised task","estimate":"3h","category":"backend"}]}`
	pool := &fakeHeadlessPool{responses: []string{validTriageJSON(), secondResponse}}
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	svc.SetHeadlessPool(pool)

	pmRepo := session.NewEntPipelineModeRepository(storage.GetEntClient())
	_, err := pmRepo.Create(t.Context(), session.PipelineModeCreateInput{
		Slug:                 "quick",
		Name:                 "Quick Fix",
		Enabled:              true,
		TriagePromptTemplate: "QUICK MODE TRIAGE: {{item_title}}",
	})
	require.NoError(t, err)
	engine, err := session.NewPipelineEngine(pmRepo)
	require.NoError(t, err)
	svc.pipelineEngine = engine

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:        "quick-mode refine item",
		Status:       string(session.BacklogStatusIdea),
		Priority:     3,
		RepoPath:     t.TempDir(),
		PipelineMode: "quick",
	})
	require.NoError(t, err)

	// Initial (first-triage) call — mode-specific, per the companion test above.
	_, trigErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId: item.ID,
	}))
	require.NoError(t, trigErr)
	require.Eventually(t, func() bool {
		updated, loadErr := storage.GetBacklogItem(t.Context(), item.ID)
		return loadErr == nil && updated.Status == string(session.BacklogStatusReady)
	}, 5*time.Second, 50*time.Millisecond, "initial triage should mark item ready")

	// Refine with feedback — the retriage branch under test.
	_, refineErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId:   item.ID,
		Feedback: "This missed the mobile case entirely.",
	}))
	require.NoError(t, refineErr)
	require.Eventually(t, func() bool {
		return pool.callCount() == 2
	}, 5*time.Second, 50*time.Millisecond, "refine should make a second headless call")

	retriagePrompt := pool.callAt(1).userPrompt
	assert.Contains(t, retriagePrompt, "## Prior triage result",
		"expected the default BuildHeadlessRetriagePrompt output, got: %s", retriagePrompt)
	assert.NotContains(t, retriagePrompt, "QUICK MODE TRIAGE:",
		"the retriage branch must NOT be routed through PipelineEngine even when PipelineMode is non-default")
}

// TestSpawnSessionFromItem_should_UseModeSpecificInitialPrompt_When_AutoSpawnSessionAndNonDefaultPipelineMode
// (Story 1.5.5) verifies inst.Prompt (and therefore NewAutonomousDriver's goal) contains
// the mode's rendered InitialPromptTemplate, not BuildTokenBudgetedPrompt's default
// output — proving this seam is not cosmetic for autonomous-mode sessions.
func TestSpawnSessionFromItem_should_UseModeSpecificInitialPrompt_When_AutoSpawnSessionAndNonDefaultPipelineMode(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	ctx := t.Context()

	pmRepo := session.NewEntPipelineModeRepository(storage.GetEntClient())
	_, err := pmRepo.Create(ctx, session.PipelineModeCreateInput{
		Slug:                  "quick",
		Name:                  "Quick Fix",
		Enabled:               true,
		InitialPromptTemplate: "QUICK MODE INITIAL PROMPT: {{item_title}}",
	})
	require.NoError(t, err)
	engine, err := session.NewPipelineEngine(pmRepo)
	require.NoError(t, err)
	svc.pipelineEngine = engine

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	createResp, err := svc.CreateBacklogItem(ctx, connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:        "quick-mode autonomous item",
		RepoPath:     repoPath,
		PipelineMode: strPtr("quick"),
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "pending"},
		},
		SkipTriage:   true,
		SkipPlanning: true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	_, err = svc.TransitionBacklogItemStatus(ctx, connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       itemID,
		TargetStatus: "ready",
	}))
	require.NoError(t, err)

	_, err = svc.SpawnSessionFromItem(ctx, connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{
		ItemId:     itemID,
		Autonomous: true,
	}))
	require.NoError(t, err)

	require.Len(t, creator.calls, 1)
	gotPrompt := creator.calls[0].prompt
	assert.Contains(t, gotPrompt, "QUICK MODE INITIAL PROMPT: quick-mode autonomous item",
		"expected the mode-specific rendered initial prompt, got: %s", gotPrompt)
}

// TestSpawnSessionFromItem_should_UseDefaultInitialPrompt_When_PipelineModeIsDefault
// (Story 1.5.5) is the zero-regression companion: default mode still produces
// BuildTokenBudgetedPrompt's unmodified output.
func TestSpawnSessionFromItem_should_UseDefaultInitialPrompt_When_PipelineModeIsDefault(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	ctx := t.Context()

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	createResp, err := svc.CreateBacklogItem(ctx, connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:    "default-mode item",
		RepoPath: repoPath,
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "pending"},
		},
		SkipTriage:   true,
		SkipPlanning: true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	_, err = svc.TransitionBacklogItemStatus(ctx, connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       itemID,
		TargetStatus: "ready",
	}))
	require.NoError(t, err)

	// Snapshot the item's pre-spawn state (status "ready") — SpawnSessionFromItem
	// transitions it to "in_progress" as its LAST step, after the prompt has already
	// been built, so re-fetching AFTER the call would reconstruct a different
	// (post-transition) BuildTokenBudgetedPrompt rendering than what was actually sent.
	preSpawn, err := storage.GetBacklogItem(ctx, itemID)
	require.NoError(t, err)

	_, err = svc.SpawnSessionFromItem(ctx, connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID}))
	require.NoError(t, err)

	require.Len(t, creator.calls, 1)

	wantPrompt := session.BuildTokenBudgetedPrompt(preSpawn, nil)
	assert.Equal(t, wantPrompt, creator.calls[0].prompt,
		"default PipelineMode must still produce BuildTokenBudgetedPrompt's unmodified output")
}

// ─── getWorkSessionDiff / resolveCodebaseWorkDir: worktree-gone false-FAIL regression ──
//
// Root cause (confirmed live on the "Backlog History feature Broken" item, PR #173,
// branch backlog/stapler-squad-fix-backlog-status-audit-trail-r3, 2026-07-20): the
// review gate's own diff-read path (ReviewGateRunner.Run, session/review_gate.go) is
// hardened against a gone worktree — it falls back to GetGitDiffRef/RecoverBaseCommitSHA
// and, failing that, blocks the review with a synthetic FAIL and an operator
// notification rather than proceeding. TriggerReReview's re-review path
// (getWorkSessionDiff/resolveCodebaseWorkDir, this file) had no equivalent hardening:
// getWorkSessionDiff logged a warning and silently returned "", and
// resolveCodebaseWorkDir hands the reviewer the DB-recorded worktree path without ever
// checking it still exists on disk. The reviewer is then granted Read/Grep/Glob access
// scoped to a directory that isn't there, finds no evidence for any criterion, and
// FAILs/UNVERIFIABLEs everything — even though the real work is sitting on a pushed
// branch. The two tests below cover both halves of the fix.

// TestGetWorkSessionDiff_should_RecoverViaMergeBase_When_WorktreeGoneAndBaseShaCorrupted
// mirrors TestReviewGateRunner_DiffComputationFailure_AutoRepairsFromDivergentBranch
// (session/review_gate_test.go) for the TriggerReReview path: the session's worktree
// directory is gone AND its recorded base_commit_sha is a well-formed but nonexistent
// SHA (the same corruption shape as backlog item ae1e2070), but the work branch is real
// and reachable from repoPath's own object store. getWorkSessionDiff must recover the
// diff via RecoverBaseCommitSHA's merge-base repair instead of giving up empty the
// moment the naive branch-ref fallback also fails.
func TestGetWorkSessionDiff_should_RecoverViaMergeBase_When_WorktreeGoneAndBaseShaCorrupted(t *testing.T) {
	repoDir := t.TempDir()
	initGitRepoWithCommit(t, repoDir)
	runGitTestCmd(t, repoDir, "branch", "-M", "main")

	const workBranch = "backlog/recover-diff-merge-base"
	runGitTestCmd(t, repoDir, "branch", workBranch)
	runGitTestCmd(t, repoDir, "checkout", workBranch)
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "feature.txt"), []byte("real work\n"), 0o644))
	runGitTestCmd(t, repoDir, "add", "feature.txt")
	runGitTestCmd(t, repoDir, "commit", "-m", "real fix")
	runGitTestCmd(t, repoDir, "checkout", "main")

	storage, repo := createTestStorageWithRepo(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:    "Diff auto-repair via TriggerReReview",
		RepoPath: repoDir,
		Status:   string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	const workSessionUUID = "diff-repair-triage-uuid"
	workIS, err := storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: workSessionUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	require.NoError(t, storage.UpdateItemSessionEnded(context.Background(), workIS.ID, time.Now()))

	// The worktree directory itself no longer exists (simulating cleanup deleting it),
	// and base_commit_sha is corrupted — both GetGitDiff(worktree) and the naive
	// GetGitDiffRef(repo fallback) must fail before the merge-base repair kicks in.
	goneWT := filepath.Join(t.TempDir(), "worktree-that-is-gone")
	now := time.Now()
	require.NoError(t, repo.Create(context.Background(), session.InstanceData{
		Title:      workSessionUUID,
		UUID:       workSessionUUID,
		Path:       goneWT,
		WorkingDir: goneWT,
		Branch:     workBranch,
		Status:     session.Paused,
		Program:    "claude",
		CreatedAt:  now,
		UpdatedAt:  now,
		Worktree: session.GitWorktreeData{
			RepoPath:      repoDir,
			WorktreePath:  goneWT,
			SessionName:   workSessionUUID,
			BranchName:    workBranch,
			BaseCommitSHA: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		},
	}))

	diff := svc.getWorkSessionDiff(context.Background(), repoDir, &session.ItemSessionSummary{SessionUUID: workSessionUUID})
	assert.Contains(t, diff, "feature.txt",
		"must recover the real diff via merge-base auto-repair instead of giving up empty")
}

// TestTriggerReReview_should_BlockInsteadOfFalseFail_When_WorktreeGoneAndDiffUnrecoverable
// reproduces the live bug end to end: no diff is recoverable (worktree gone, branch
// itself unresolvable — simulating a fully torn-down session, the case where even the
// merge-base repair above cannot help) and resolveCodebaseWorkDir's fallback directory
// does not exist on disk either. TriggerReReview must block before ever spending a
// headless call — proven here by a fake pool that would confidently return FAIL if it
// were ever actually invoked — and must record an explicit UNVERIFIABLE verdict plus an
// operator notification, never a false FAIL synthesized from reading nothing.
func TestTriggerReReview_should_BlockInsteadOfFalseFail_When_WorktreeGoneAndDiffUnrecoverable(t *testing.T) {
	storage, repo := createTestStorageWithRepo(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	bus := events.NewEventBus(4)
	svc.SetEventBus(bus)
	subCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)

	// This response would (falsely) confirm the original bug if the pool were ever
	// actually called — asserting callCount()==0 below proves the fix short-circuits
	// before spending a headless call, not merely that some downstream heuristic
	// happens to catch a bad result afterward.
	//
	// The pool is wired AFTER setupItemInReview: CreateBacklogItem triggers automatic
	// triage when a headless pool is already set, which would otherwise consume this
	// pool's single scripted response before the re-review call under test ever runs
	// (same ordering setupItemInReview's other callers rely on).
	repoDir := t.TempDir()
	initGitRepoWithCommit(t, repoDir)

	itemID := setupItemInReview(t, svc, repoDir)

	pool := &fakeHeadlessPool{response: `{"overall":"FAIL","summary":"no diff exists; codebase shows none of the claimed work","tool_reads":[],"verdicts":[]}`}
	svc.SetHeadlessPool(pool)
	svc.SetCapabilityCheck(headless.NewPassedCapabilitySelfCheckForTesting())

	const workSessionUUID = "gone-worktree-unrecoverable-uuid"
	workIS, err := storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID:      itemID,
		SessionUUID: workSessionUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	require.NoError(t, storage.UpdateItemSessionEnded(context.Background(), workIS.ID, time.Now()))

	goneWT := filepath.Join(t.TempDir(), "worktree-that-is-gone")
	now := time.Now()
	require.NoError(t, repo.Create(context.Background(), session.InstanceData{
		Title:      workSessionUUID,
		UUID:       workSessionUUID,
		Path:       goneWT,
		WorkingDir: goneWT,
		Branch:     "backlog/gone-branch-nowhere",
		Status:     session.Paused,
		Program:    "claude",
		CreatedAt:  now,
		UpdatedAt:  now,
		Worktree: session.GitWorktreeData{
			RepoPath:     repoDir,
			WorktreePath: goneWT,
			SessionName:  workSessionUUID,
			// BranchName was never created in repoDir, so it is unresolvable — neither
			// GetGitDiffRef nor RecoverBaseCommitSHA's merge-base lookup can recover
			// anything, forcing the empty-diff codebase-read path.
			BranchName:    "backlog/gone-branch-nowhere",
			BaseCommitSHA: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		},
	}))

	resp, err := svc.TriggerReReview(t.Context(), connect.NewRequest(&sessionv1.TriggerReReviewRequest{ItemId: itemID}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.ItemSession)

	assert.Equal(t, 0, pool.callCount(), "must block before ever calling the headless pool")

	outcome, err := storage.GetMostRecentReviewVerdictForItem(t.Context(), itemID)
	require.NoError(t, err)
	assert.Equal(t, session.ReviewVerdictUnverifiable, outcome,
		"must not record a false FAIL when the codebase-read directory does not exist")

	select {
	case ev := <-ch:
		assert.Equal(t, events.EventNotification, ev.Type)
	case <-time.After(2 * time.Second):
		t.Fatal("expected an operator notification when the review is blocked")
	}
}

// TestResolveCodebaseWorkDir_should_RefuseFallback_When_WorkSessionWorktreeUnresolvable
// is a direct unit regression for BUG-045's root cause: resolveCodebaseWorkDir used to
// fall back to repoPath and report exists=true whenever a work session's worktree data
// could not be resolved at all (the underlying session/worktree row itself reaped, not
// merely its directory) — silently treating the shared main checkout as a stand-in for
// the item's own state. This asserts the fallback is now refused (exists=false) in that
// exact scenario, even though repoPath itself is a perfectly real, existing directory.
func TestResolveCodebaseWorkDir_should_RefuseFallback_When_WorkSessionWorktreeUnresolvable(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	// repoPath is a real, existing directory — simulating the shared main checkout,
	// which always exists. The bug was conflating "repoPath exists" with "repoPath is
	// safe to use as this session's own state."
	repoPath := t.TempDir()

	const reapedSessionUUID = "reaped-session-no-worktree-row"
	// Deliberately do NOT create any Instance/Worktree row for reapedSessionUUID —
	// GetWorktreeDataBySessionUUID will return an empty GitWorktreeData with a nil
	// error (ent.IsNotFound path), exactly reproducing "the worktree has been reaped."
	dir, exists := svc.resolveCodebaseWorkDir(t.Context(), repoPath, &session.ItemSessionSummary{SessionUUID: reapedSessionUUID})
	assert.False(t, exists, "must refuse the codebase-read fallback when a work session's worktree cannot be resolved, even though repoPath itself exists on disk")
	assert.Equal(t, repoPath, dir, "returned dir is still repoPath for logging purposes, but exists must be false")
}

// TestResolveCodebaseWorkDir_should_AllowFallback_When_NoWorkSessionAtAll guards the
// companion, still-legitimate case this fix must not regress: an item with no work
// session at all (e.g. reviewed before any session ever ran) has nothing item-specific
// to fall back from, so repoPath remains the only — and correct — directory to use.
func TestResolveCodebaseWorkDir_should_AllowFallback_When_NoWorkSessionAtAll(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	repoPath := t.TempDir()

	dir, exists := svc.resolveCodebaseWorkDir(t.Context(), repoPath, nil)
	assert.True(t, exists, "must still allow the repoPath fallback when there was never a work session to begin with")
	assert.Equal(t, repoPath, dir)
}

// TestTriggerReReview_should_BlockInsteadOfReviewingSharedCheckout_When_WorktreeRowReaped
// reproduces BUG-045 end to end at TriggerReReview's actual live entry point: item
// 693c2700's review fell into the codebase-read fallback because its dedicated
// worktree had been reaped, and resolveCodebaseWorkDir silently granted the reviewer
// live Read/Grep/Glob access to the shared main checkout instead — which happened to
// contain unrelated, uncommitted work at that moment, producing a plausible-sounding
// but completely wrong FAIL verdict describing that unrelated work.
//
// Here, repoPath is a real directory containing content that would prove the bug if
// ever handed to the reviewer (a marker file + a scripted pool response naming it) —
// the work session's worktree cannot be resolved (row reaped) and no primary diff
// exists either. TriggerReReview must refuse the fallback and record an explicit
// UNVERIFIABLE verdict before ever spending a headless call, never silently reviewing
// repoPath's arbitrary contents and calling the result a real verdict on this item.
func TestTriggerReReview_should_BlockInsteadOfReviewingSharedCheckout_When_WorktreeRowReaped(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	bus := events.NewEventBus(4)
	svc.SetEventBus(bus)
	subCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)

	// repoPath stands in for the shared main checkout: it's a real git repo, and it
	// contains content totally unrelated to this item — exactly the shape of the live
	// incident (an operator's unrelated, uncommitted work sitting in the checkout).
	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "unrelated-in-progress-work.txt"), []byte("someone else's uncommitted work\n"), 0o644))

	itemID := setupItemInReview(t, svc, repoPath)

	// This response would (falsely) confirm the original bug if the pool were ever
	// actually invoked against repoPath — asserting callCount()==0 below proves the fix
	// short-circuits before a headless call is ever spent reading the wrong directory.
	pool := &fakeHeadlessPool{response: `{"overall":"FAIL","summary":"found unrelated-in-progress-work.txt, not the item's own work","tool_reads":["unrelated-in-progress-work.txt"],"verdicts":[]}`}
	svc.SetHeadlessPool(pool)
	svc.SetCapabilityCheck(headless.NewPassedCapabilitySelfCheckForTesting())

	const reapedSessionUUID = "reaped-worktree-693c2700-shape"
	workIS, err := storage.CreateItemSession(context.Background(), session.ItemSessionData{
		ItemID:      itemID,
		SessionUUID: reapedSessionUUID,
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	require.NoError(t, storage.UpdateItemSessionEnded(context.Background(), workIS.ID, time.Now()))
	// Deliberately no corresponding Instance/Worktree row is ever created for
	// reapedSessionUUID — simulating the item's dedicated worktree (and its DB row)
	// having been reaped by the time this review runs, exactly as confirmed live on
	// item 693c2700.

	resp, err := svc.TriggerReReview(t.Context(), connect.NewRequest(&sessionv1.TriggerReReviewRequest{ItemId: itemID}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.ItemSession)

	assert.Equal(t, 0, pool.callCount(), "must block before ever calling the headless pool against the shared checkout")

	outcome, err := storage.GetMostRecentReviewVerdictForItem(t.Context(), itemID)
	require.NoError(t, err)
	assert.Equal(t, session.ReviewVerdictUnverifiable, outcome,
		"must not silently review the shared main checkout's arbitrary contents and record a verdict on this item")

	select {
	case ev := <-ch:
		assert.Equal(t, events.EventNotification, ev.Type)
	case <-time.After(2 * time.Second):
		t.Fatal("expected an operator notification when the review is blocked")
	}
}

// TestTriggerReReview_should_BlockOnBranchDriftInsteadOfMisleadingFailVerdict is the
// regression guard for BUG-044's actual live entry point: AutoRespawnReview (the
// abandoned-review self-heal path — see BUG-043) re-drives review via TriggerReReview,
// not the fresh work-session-exit path. Backlog item 693c2700 drifted 289 commits
// behind main across repeated abandoned-review cycles through exactly this call before
// ever being caught, and its review then failed with a misleading "no code related to
// the feature at all" verdict driven by drift noise, not the item's actual (complete)
// work. This reproduces the drifted-and-conflicting shape and verifies TriggerReReview
// blocks with an explicit UNVERIFIABLE verdict — naming the real cause — before ever
// spending a headless call on a diff that would have been dominated by upstream drift.
func TestTriggerReReview_should_BlockOnBranchDriftInsteadOfMisleadingFailVerdict(t *testing.T) {
	storage, repo := createTestStorageWithRepo(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	bus := events.NewEventBus(4)
	svc.SetEventBus(bus)
	subCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)

	// origin/work mirror production: item.RepoPath (origin) is the shared checkout,
	// work is the item's dedicated worktree clone with a real "origin" remote — the
	// same shape setupPRFixSyncRepo already builds for syncPRBranchWithMain's tests.
	origin, work := setupPRFixSyncRepo(t)

	itemID := setupItemInReview(t, svc, origin)

	// This response would (falsely) confirm the original bug if the pool were ever
	// actually invoked with the drift-inflated diff — asserting callCount()==0 below
	// proves the fix short-circuits before spending a headless call.
	pool := &fakeHeadlessPool{response: `{"overall":"FAIL","summary":"the diff contains no code related to the feature at all","tool_reads":[],"verdicts":[]}`}
	svc.SetHeadlessPool(pool)
	svc.SetCapabilityCheck(headless.NewPassedCapabilitySelfCheckForTesting())

	runGitTestCmd(t, work, "checkout", "-b", "feature")

	// The feature branch made its own real, complete edit.
	require.NoError(t, os.WriteFile(filepath.Join(work, "README.md"), []byte("# Feature Edit\n"), 0o644))
	runGitTestCmd(t, work, "add", "README.md")
	runGitTestCmd(t, work, "commit", "-m", "feature: complete the work")

	// Main diverges on the same line AND drifts well past the default threshold — the
	// exact shape that made 693c2700's diff unreviewable.
	require.NoError(t, os.WriteFile(filepath.Join(origin, "README.md"), []byte("# Main Edit\n"), 0o644))
	runGitTestCmd(t, origin, "add", "README.md")
	runGitTestCmd(t, origin, "commit", "-m", "main: unrelated edit")
	for i := 0; i < 55; i++ {
		fname := fmt.Sprintf("upstream-%d.txt", i)
		require.NoError(t, os.WriteFile(filepath.Join(origin, fname), []byte("content\n"), 0o644))
		runGitTestCmd(t, origin, "add", fname)
		runGitTestCmd(t, origin, "commit", "-m", fmt.Sprintf("upstream commit %d", i))
	}

	attachPRFixWorkSession(t, storage, repo, &session.BacklogItemData{ID: itemID},
		"branch-drift-rereview-uuid", origin, work, "feature")

	resp, err := svc.TriggerReReview(t.Context(), connect.NewRequest(&sessionv1.TriggerReReviewRequest{ItemId: itemID}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.ItemSession)

	assert.Equal(t, 0, pool.callCount(), "must block before ever calling the headless pool with a drift-inflated diff")

	outcome, err := storage.GetMostRecentReviewVerdictForItem(t.Context(), itemID)
	require.NoError(t, err)
	assert.Equal(t, session.ReviewVerdictUnverifiable, outcome,
		"must record an explicit blocked verdict, not the misleading FAIL the headless pool was primed to return")

	select {
	case ev := <-ch:
		assert.Equal(t, events.EventNotification, ev.Type)
	case <-time.After(2 * time.Second):
		t.Fatal("expected an operator notification when the review is blocked")
	}
}

// --- git-drift-check steering hook scoping (BUG-044 follow-up) ---
//
// HookGitDriftCheck must be injected into a spawned session's worktree ONLY when
// SpawnSessionFromItem's autonomous flag is true (the AutonomousDriver-run, no-human-
// attached path) — never for a manual spawn, and never left behind on a worktree
// reused by a later manual respawn. These three tests are the scoping guard called
// for in the hook's own design: "assert the hook is NOT injected for a
// non-autonomous/manually-created session."

// driftHookURLFragment matches the fragment InjectHooksConfig/RemoveHooksConfig write
// into the curl command for HookGitDriftCheck — kept in sync with hookEndpoints'
// "/api/hooks/post-tool-use-drift-check" suffix.
const driftHookURLFragment = "/api/hooks/post-tool-use-drift-check"

// settingsHasDriftHook reports whether <worktreePath>/.claude/settings.local.json
// exists and its PostToolUse hooks contain the git-drift-check command. A missing
// settings file is treated as "hook absent" (false, no error) since a purely-manual
// spawn may never create one at all.
func settingsHasDriftHook(t *testing.T, worktreePath string) bool {
	t.Helper()
	path := filepath.Join(worktreePath, ".claude", "settings.local.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		t.Fatalf("read settings.local.json: %v", err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatalf("parse settings.local.json: %v", err)
	}
	hooksRaw, ok := top["hooks"]
	if !ok {
		return false
	}
	var hooksMap map[string]json.RawMessage
	if err := json.Unmarshal(hooksRaw, &hooksMap); err != nil {
		t.Fatalf("parse hooks map: %v", err)
	}
	postToolRaw, ok := hooksMap["PostToolUse"]
	if !ok {
		return false
	}
	var groups []hookMatcherGroup
	if err := json.Unmarshal(postToolRaw, &groups); err != nil {
		t.Fatalf("parse PostToolUse groups: %v", err)
	}
	for _, g := range groups {
		for _, hk := range g.Hooks {
			if strings.Contains(hk.Command, driftHookURLFragment) {
				return true
			}
		}
	}
	return false
}

// spawnReadyItemForDriftHookTest creates a backlog item against a real git repo and
// transitions it to ready, returning the item ID — shared setup for the three tests
// below, mirroring TestSpawnSessionFromItem_should_SnapshotResolvedModeSlugAndContentHash's
// existing real-repo pattern.
func spawnReadyItemForDriftHookTest(t *testing.T, svc *BacklogService, title string) string {
	t.Helper()
	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:    title,
		RepoPath: repoPath,
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "pending"},
		},
		SkipTriage:   true,
		SkipPlanning: true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       itemID,
		TargetStatus: "ready",
	}))
	require.NoError(t, err)
	return itemID
}

// TestSpawnSessionFromItem_should_InjectGitDriftCheckHook_When_Autonomous is the
// positive scoping case: Autonomous:true (the flag AutoReopenAfterFailedReview,
// AutoReopenForPRFix, AutoRespawnReview, and the "Run Autonomously" button all pass)
// must get the steering hook wired into its worktree.
func TestSpawnSessionFromItem_should_InjectGitDriftCheckHook_When_Autonomous(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	itemID := spawnReadyItemForDriftHookTest(t, svc, "autonomous drift hook item")

	_, err := svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{
		ItemId:     itemID,
		Autonomous: true,
	}))
	require.NoError(t, err)
	require.Len(t, creator.calls, 1)

	worktreePath := creator.calls[0].path
	assert.True(t, settingsHasDriftHook(t, worktreePath),
		"expected git-drift-check hook to be injected into an autonomous spawn's worktree")
}

// TestSpawnSessionFromItem_should_NotInjectGitDriftCheckHook_When_NotAutonomous is the
// negative scoping case: a plain manual spawn (the default "Start Session" button and
// the manual "Reopen for Revision" flow both omit autonomous, defaulting to false)
// must NEVER get the steering hook wired in.
func TestSpawnSessionFromItem_should_NotInjectGitDriftCheckHook_When_NotAutonomous(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	itemID := spawnReadyItemForDriftHookTest(t, svc, "manual drift hook item")

	_, err := svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{
		ItemId: itemID,
		// Autonomous intentionally omitted — proto default false, matching every
		// manual call site (spawn_session, handleGateReopen).
	}))
	require.NoError(t, err)
	require.Len(t, creator.calls, 1)

	worktreePath := creator.calls[0].path
	assert.False(t, settingsHasDriftHook(t, worktreePath),
		"git-drift-check hook must never be injected into a non-autonomous (manually-created) session")
}

// TestSpawnSessionFromItem_should_RemoveGitDriftCheckHook_When_ReopenedManuallyOnReusedWorktree
// covers the worktree-reuse gap the hard scoping requirement calls out explicitly: a
// backlog item's worktree/branch is reused across reopen cycles (same
// "backlog/<item>" slug every revision — see spawnSessionAfterGates step 10). Without
// an active removal step, a worktree first spawned autonomously would keep the
// steering hook wired into every later session on that same worktree forever, even a
// subsequent manual respawn a human is actively watching. This spawns autonomously
// once (hook injected), then force-respawns the SAME item non-autonomously on the
// SAME worktree, and asserts the hook is gone afterward.
func TestSpawnSessionFromItem_should_RemoveGitDriftCheckHook_When_ReopenedManuallyOnReusedWorktree(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	itemID := spawnReadyItemForDriftHookTest(t, svc, "reused worktree drift hook item")

	_, err := svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{
		ItemId:     itemID,
		Autonomous: true,
	}))
	require.NoError(t, err)
	require.Len(t, creator.calls, 1)
	firstWorktreePath := creator.calls[0].path
	require.True(t, settingsHasDriftHook(t, firstWorktreePath), "setup: hook must be present after the autonomous spawn")

	// Force-respawn the same item non-autonomously. Force ends the still-open first
	// work session so the guard against duplicate active sessions doesn't reject it;
	// the branch slug (derived from repoName + baseTitle, unaffected by prior work
	// sessions) resolves to the SAME worktree path both times.
	_, err = svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{
		ItemId: itemID,
		Force:  true,
		// Autonomous omitted: false — a human forcing a restart is driving this one.
	}))
	require.NoError(t, err)
	require.Len(t, creator.calls, 2)
	secondWorktreePath := creator.calls[1].path
	require.Equal(t, firstWorktreePath, secondWorktreePath, "setup: expected the reopen to reuse the same worktree path")

	assert.False(t, settingsHasDriftHook(t, secondWorktreePath),
		"git-drift-check hook must be actively removed when a worktree that was previously spawned "+
			"autonomously is later respawned non-autonomously (manual reopen)")
}

// --- Story 2.1.2: ResolveReworkBlockedStaleIfRecovered ---

// TestResolveReworkBlockedStaleIfRecovered_should_resolveStuckRow_When_SessionRecovered
// verifies the resolve pass clears an open rework_blocked_stale row once the
// blocking session's idle time drops back under maxReworkBlockStaleness.
func TestResolveReworkBlockedStaleIfRecovered_should_resolveStuckRow_When_SessionRecovered(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Item whose blocking session recovered",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID: item.ID, SessionUUID: "active-work-uuid", SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	applied, err := storage.MarkStuck(ctx, item.ID, domain.StuckReasonReworkBlockedStale, session.BacklogStatusReview, "was stale")
	require.NoError(t, err)
	require.True(t, applied)

	stopper := &mockSessionStopper{
		liveUUIDs: map[string]bool{"active-work-uuid": true},
		staleFor:  map[string]time.Duration{"active-work-uuid": 2 * time.Minute}, // recovered — well under 15min
	}
	svc.SetSessionStopper(stopper)

	require.NoError(t, svc.ResolveReworkBlockedStaleIfRecovered(ctx, item.ID))

	open, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "row must be resolved once the session recovers")
}

// TestResolveReworkBlockedStaleIfRecovered_should_leaveRowOpen_When_StillStale
// is the negative case: a session still idle past the threshold must leave
// the row open (no ResolveStuck call).
func TestResolveReworkBlockedStaleIfRecovered_should_leaveRowOpen_When_StillStale(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Item whose blocking session is still stale",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID: item.ID, SessionUUID: "active-work-uuid", SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	applied, err := storage.MarkStuck(ctx, item.ID, domain.StuckReasonReworkBlockedStale, session.BacklogStatusReview, "still stale")
	require.NoError(t, err)
	require.True(t, applied)

	stopper := &mockSessionStopper{
		liveUUIDs: map[string]bool{"active-work-uuid": true},
		staleFor:  map[string]time.Duration{"active-work-uuid": 25 * time.Minute}, // still stale
	}
	svc.SetSessionStopper(stopper)

	require.NoError(t, svc.ResolveReworkBlockedStaleIfRecovered(ctx, item.ID))

	open, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1, "row must remain open while still stale")
	assert.Equal(t, domain.StuckReasonReworkBlockedStale, open[0].Reason)
}

// TestResolveReworkBlockedStaleIfRecovered_should_beNoOp_When_NoActiveWorkSession
// is the belt-and-suspenders case: if the blocking work session has since
// ended (EndedAt set), the row must resolve even without re-checking
// liveness — there's nothing left to be stale.
func TestResolveReworkBlockedStaleIfRecovered_should_beNoOp_When_NoActiveWorkSession(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Item whose blocking session already ended",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)
	is, err := storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID: item.ID, SessionUUID: "active-work-uuid", SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	require.NoError(t, storage.UpdateItemSessionEnded(ctx, is.ID, time.Now()))

	applied, err := storage.MarkStuck(ctx, item.ID, domain.StuckReasonReworkBlockedStale, session.BacklogStatusReview, "session ended")
	require.NoError(t, err)
	require.True(t, applied)

	svc.SetSessionStopper(&mockSessionStopper{}) // nothing live — must not be consulted

	require.NoError(t, svc.ResolveReworkBlockedStaleIfRecovered(ctx, item.ID))

	open, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "row must resolve once the blocking work session has ended, without needing a liveness check")
}

// --- reconcileReworkBlockedStaleResolution (session package orchestration) ---
// See session/backlog_lifecycle_test.go for tests exercising the
// BacklogLifecycleListener orchestration function itself; the tests above
// cover the ReworkBlockStaleResolver implementation these tests delegate to.

// TestReworkBlockedStale_should_markAndLaterResolve_When_SessionStallsThenRecovers_Integration
// is the full round-trip validation.md calls for: MarkStuck via the real
// notifyIfActiveWorkSessionStale call path -> appears in FindOpenStuckStates
// -> the blocking session's staleness clears -> a real
// session.BacklogLifecycleListener's periodic ReconcileStuck tick (wired to
// the real BacklogService via SetReworkBlockStaleResolver, exactly as
// server/dependencies.go wires it in production) resolves the row — no fakes
// on either side, unlike the unit tests above and in
// session/backlog_lifecycle_stuck_test.go, which each fake out the other
// side of this interface boundary.
func TestReworkBlockedStale_should_markAndLaterResolve_When_SessionStallsThenRecovers_Integration(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	ctx := context.Background()

	stopper := &mockSessionStopper{
		liveUUIDs: map[string]bool{"active-work-uuid": true},
		staleFor:  map[string]time.Duration{"active-work-uuid": 20 * time.Minute}, // stalled
	}
	svc.SetSessionStopper(stopper)
	svc.SetEventBus(events.NewEventBus(4))

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Item whose session stalls then recovers",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID: item.ID, SessionUUID: "active-work-uuid", SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)

	// Mark side: the real production call path (AutoReopenAfterFailedReview ->
	// notifyIfActiveWorkSessionStale -> MarkStuck), not a direct storage.MarkStuck call.
	require.NoError(t, svc.AutoReopenAfterFailedReview(ctx, item.ID))

	open, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1, "setup: the mark side must have opened a row before the resolve side is exercised")
	assert.Equal(t, domain.StuckReasonReworkBlockedStale, open[0].Reason)

	// Session recovers.
	stopper.staleFor["active-work-uuid"] = 2 * time.Minute

	// Resolve side: a real session.BacklogLifecycleListener's periodic tick,
	// wired to the real BacklogService exactly as production does it.
	listener := session.NewBacklogLifecycleListener(storage)
	listener.SetEnabled(true)
	listener.SetReworkBlockStaleResolver(svc)
	listener.ReconcileStuck(ctx)

	open, err = storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "the periodic tick must resolve the row once the session recovers")
}
