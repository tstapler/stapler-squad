package session

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session/ent"
)

// blockingTriggerFirer is a TriggerFirer whose FireTriggerChained call signals
// `started` and then blocks on `unblock` — used to prove ordering (AC9: the
// caller must not be blocked waiting for this call) rather than just eventual
// completion. Panics if called more than once (each test using it expects
// exactly one call — the double-fire prevention test below uses
// countingTriggerFirer instead, which tolerates concurrent calls).
type blockingTriggerFirer struct {
	started chan struct{}
	unblock chan struct{}
}

func (f *blockingTriggerFirer) FireTriggerChained(ctx context.Context, wf *ent.Workflow, priorItemSummary string, chainDepth int32) (string, error) {
	close(f.started)
	<-f.unblock
	return "blocking-fake-session-id", nil
}

// countingTriggerFirer is a thread-safe TriggerFirer fake that records every
// call (safe to invoke concurrently — used by the double-fire prevention
// test), so tests can assert exactly how many times FireTriggerChained was
// actually invoked.
type countingTriggerFirer struct {
	mu         sync.Mutex
	calls      int
	lastWf     *ent.Workflow
	lastDepth  int32
	lastPrompt string
}

func (f *countingTriggerFirer) FireTriggerChained(ctx context.Context, wf *ent.Workflow, priorItemSummary string, chainDepth int32) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastWf = wf
	f.lastDepth = chainDepth
	f.lastPrompt = priorItemSummary
	return fmt.Sprintf("fake-session-%d", f.calls), nil
}

func (f *countingTriggerFirer) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// chainTestFixture bundles the collaborators every ChainFirer test needs:
// a real (sqlite-backed) EntRepository plus the WorkflowRepository/
// TriggerFireEventRepository built around the exact same ent.Client, mirroring
// how server/dependencies.go wires ChainFirer in production
// (Storage.WireChainFirer).
type chainTestFixture struct {
	repo       *EntRepository
	workflows  WorkflowRepository
	fireEvents TriggerFireEventRepository
}

func newChainTestFixture(t *testing.T) (*chainTestFixture, func()) {
	t.Helper()
	repo, cleanup := createTestEntRepository(t)
	client := repo.GetEntClient()
	return &chainTestFixture{
		repo:       repo,
		workflows:  NewEntWorkflowRepository(client),
		fireEvents: NewEntTriggerFireEventRepository(client),
	}, cleanup
}

func (f *chainTestFixture) createWorkflow(t *testing.T, slug string) *ent.Workflow {
	t.Helper()
	wf, err := f.workflows.Create(context.Background(), WorkflowCreateInput{
		Slug:            slug,
		Name:            slug,
		Command:         "do the next thing",
		TargetDirectory: "/tmp/chain-fire-test",
	})
	require.NoError(t, err)
	return wf
}

func enabledCfg() *config.Config {
	return &config.Config{FeatureFlags: map[string]bool{"webhook_triggers": true}}
}

// TestTransitionBacklogItemStatus_should_returnBeforeChainFireCreateSessionBegins_When_DoneWithNextWorkflowSet
// is the AC9 verification the plan explicitly calls for (Task 6.2.1a/webhook-
// triggers requirements.md AC9): TransitionBacklogItemStatus's own DB write
// must return to its caller well before the chain-fire's CreateSession-
// equivalent call (TriggerFirer.FireTriggerChained) begins running to
// completion — the chain-fire must be dispatched off the transition's own
// call stack (go + semaphore), never awaited inside it. A blocking fake
// firer proves this: it never returns until the test explicitly unblocks it,
// so if TransitionBacklogItemStatus were (incorrectly) waiting on it
// synchronously, this test would hang past its own timeout.
func TestTransitionBacklogItemStatus_should_returnBeforeChainFireCreateSessionBegins_When_DoneWithNextWorkflowSet(t *testing.T) {
	t.Parallel()
	fx, cleanup := newChainTestFixture(t)
	defer cleanup()
	ctx := context.Background()

	wf := fx.createWorkflow(t, "chain-next-ac9")

	started := make(chan struct{})
	unblock := make(chan struct{})
	firer := &blockingTriggerFirer{started: started, unblock: unblock}

	cf := NewChainFirer(fx.repo, fx.workflows, fx.fireEvents, firer, enabledCfg())
	fx.repo.SetChainFirer(cf)

	item, err := fx.repo.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "chained item ac9",
		Status: string(BacklogStatusReview),
	})
	require.NoError(t, err)

	wfID := wf.ID
	_, err = fx.repo.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{NextWorkflowID: &wfID}, nil)
	require.NoError(t, err)

	transitionStart := time.Now()
	_, err = fx.repo.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusDone,
		&BacklogItemPrecondition{ExpectedStatus: string(BacklogStatusReview)}, TriggeredBySystem)
	transitionElapsed := time.Since(transitionStart)
	require.NoError(t, err)

	// The transition must return quickly — well under the time the blocking
	// FireTriggerChained call would take (it cannot return until `unblock` is
	// closed, which has NOT happened yet at this point in the test). A
	// synchronous, buggy implementation would make this assertion fail by
	// taking however long the test's own timeout below allows.
	assert.Less(t, transitionElapsed, 500*time.Millisecond,
		"TransitionBacklogItemStatus must not block on the chain-fire's CreateSession-equivalent call (AC9)")

	// Confirm the chain-fire really was dispatched (not silently dropped) —
	// it should be in-flight right now, blocked on `unblock`.
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("chain-fire was never dispatched from TransitionBacklogItemStatus")
	}

	close(unblock)

	require.Eventually(t, func() bool {
		latest, getErr := fx.repo.GetBacklogItem(ctx, item.ID)
		return getErr == nil && latest.ChainFired
	}, 2*time.Second, 20*time.Millisecond, "ChainFired must eventually be persisted once the async fire completes")
}

// TestChainFirer_Fire_should_rejectAndMarkChainFired_When_DepthAtOrAboveMax
// covers Epic 6.3's chain-depth cap: an item already at maxChainDepth must be
// rejected before ever calling FireTriggerChained, with ChainFired left true
// (so the reconciler never retries it) and a fired_failed audit row.
func TestChainFirer_Fire_should_rejectAndMarkChainFired_When_DepthAtOrAboveMax(t *testing.T) {
	t.Parallel()
	fx, cleanup := newChainTestFixture(t)
	defer cleanup()
	ctx := context.Background()

	wf := fx.createWorkflow(t, "chain-depth-cap")
	firer := &countingTriggerFirer{}
	cf := NewChainFirer(fx.repo, fx.workflows, fx.fireEvents, firer, enabledCfg())

	item, err := fx.repo.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "already deep chain",
		Status: string(BacklogStatusDone),
	})
	require.NoError(t, err)

	wfID := wf.ID
	depth := maxChainDepth
	_, err = fx.repo.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{
		NextWorkflowID:        &wfID,
		TriggeredByChainDepth: &depth,
	}, nil)
	require.NoError(t, err)

	latest, err := fx.repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)

	fired, err := cf.Fire(ctx, latest)
	require.NoError(t, err)
	assert.False(t, fired)
	assert.Equal(t, 0, firer.callCount(), "must never call FireTriggerChained once the depth cap is reached")

	final, err := fx.repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.True(t, final.ChainFired, "ChainFired must be set so TriggerChainReconciler never retries a depth-capped chain")

	events, err := fx.fireEvents.ListByWorkflow(ctx, wf.ID, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "fired_failed", events[0].Outcome)
	assert.Contains(t, events[0].ErrorMessage, "chain depth exceeded")
}

// TestChainFirer_Fire_should_rejectAndMarkChainFired_When_WaitExceedsMaxChainWaitDuration
// covers the pre-mortem P2 #5 retry ceiling: a chain that's been eligible
// (ChainedAt) for longer than maxChainWaitDuration gives up rather than
// letting TriggerChainReconciler retry it forever behind a saturated WIP gate.
func TestChainFirer_Fire_should_rejectAndMarkChainFired_When_WaitExceedsMaxChainWaitDuration(t *testing.T) {
	t.Parallel()
	fx, cleanup := newChainTestFixture(t)
	defer cleanup()
	ctx := context.Background()

	wf := fx.createWorkflow(t, "chain-expired-wait")
	firer := &countingTriggerFirer{}
	cf := NewChainFirer(fx.repo, fx.workflows, fx.fireEvents, firer, enabledCfg())

	item, err := fx.repo.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "long-waiting chain",
		Status: string(BacklogStatusDone),
	})
	require.NoError(t, err)

	wfID := wf.ID
	longAgo := time.Now().Add(-(maxChainWaitDuration + time.Hour))
	_, err = fx.repo.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{
		NextWorkflowID: &wfID,
		ChainedAt:      &longAgo,
	}, nil)
	require.NoError(t, err)

	latest, err := fx.repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.NotNil(t, latest.ChainedAt)

	fired, err := cf.Fire(ctx, latest)
	require.NoError(t, err)
	assert.False(t, fired)
	assert.Equal(t, 0, firer.callCount())

	final, err := fx.repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.True(t, final.ChainFired)

	events, err := fx.fireEvents.ListByWorkflow(ctx, wf.ID, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "fired_failed", events[0].Outcome)
	assert.Contains(t, events[0].ErrorMessage, "expired waiting for WIP capacity")
}

// TestChainFirer_Fire_should_createSessionAndMarkChainFired_When_ValidChainConfigured
// is the happy path: FireTriggerChained is called once with depth+1, and
// ChainFired lands true afterward.
func TestChainFirer_Fire_should_createSessionAndMarkChainFired_When_ValidChainConfigured(t *testing.T) {
	t.Parallel()
	fx, cleanup := newChainTestFixture(t)
	defer cleanup()
	ctx := context.Background()

	wf := fx.createWorkflow(t, "chain-happy-path")
	firer := &countingTriggerFirer{}
	cf := NewChainFirer(fx.repo, fx.workflows, fx.fireEvents, firer, enabledCfg())

	item, err := fx.repo.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "valid chain item",
		Status: string(BacklogStatusDone),
	})
	require.NoError(t, err)

	wfID := wf.ID
	depth := 2
	_, err = fx.repo.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{
		NextWorkflowID:        &wfID,
		TriggeredByChainDepth: &depth,
	}, nil)
	require.NoError(t, err)

	latest, err := fx.repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)

	fired, err := cf.Fire(ctx, latest)
	require.NoError(t, err)
	assert.True(t, fired)
	assert.Equal(t, 1, firer.callCount())
	assert.Equal(t, int32(3), firer.lastDepth, "chained session's depth must be item.TriggeredByChainDepth + 1")
	require.NotNil(t, firer.lastWf)
	assert.Equal(t, wf.ID, firer.lastWf.ID)

	final, err := fx.repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.True(t, final.ChainFired)
}

// TestTriggerChainReconciler_should_completeInterruptedChain_When_DoneItemHasUnfiredChain
// simulates the AC5 restart-recovery scenario: a done item with
// NextWorkflowID set and ChainFired still false (as if the process crashed
// between the done write and the async dispatch ever running) is picked up
// by ReconcileChains and fired.
func TestTriggerChainReconciler_should_completeInterruptedChain_When_DoneItemHasUnfiredChain(t *testing.T) {
	t.Parallel()
	fx, cleanup := newChainTestFixture(t)
	defer cleanup()
	ctx := context.Background()

	wf := fx.createWorkflow(t, "chain-crash-recovery")
	firer := &countingTriggerFirer{}
	cf := NewChainFirer(fx.repo, fx.workflows, fx.fireEvents, firer, enabledCfg())
	reconciler := NewTriggerChainReconciler(cf)

	item, err := fx.repo.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "crashed before fire",
		Status: string(BacklogStatusDone),
	})
	require.NoError(t, err)
	wfID := wf.ID
	_, err = fx.repo.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{NextWorkflowID: &wfID}, nil)
	require.NoError(t, err)

	// Note: no ChainFirer was ever wired to fx.repo (SetChainFirer not
	// called) — mirroring a real crash where the async dispatch from
	// TransitionBacklogItemStatus simply never ran (or the process died
	// before it could). ReconcileChains must recover independently of that
	// happy path.
	reconciler.ReconcileChains(ctx, fx.repo)

	require.Eventually(t, func() bool {
		return firer.callCount() == 1
	}, 2*time.Second, 20*time.Millisecond, "TriggerChainReconciler must fire the interrupted chain")

	require.Eventually(t, func() bool {
		latest, getErr := fx.repo.GetBacklogItem(ctx, item.ID)
		return getErr == nil && latest.ChainFired
	}, 2*time.Second, 20*time.Millisecond)

	// A second reconcile tick must be a no-op — the chain already fired.
	reconciler.ReconcileChains(ctx, fx.repo)
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 1, firer.callCount(), "an already-fired chain must never be re-fired by a later reconcile tick")
}

// TestTriggerChainReconciler_should_findPendingChain_When_MoreThan1000DoneItemsExist
// covers the sdd:6-verify finding: ReconcileChains used to call
// ListBacklogItems(BacklogItemFilter{Statuses: []string{"done"}}) with no
// ChainFired/NextWorkflowIDSet filter pushed into SQL, relying on
// ListBacklogItems's default 1000-row safety cap plus a Go-side post-filter —
// past 1000 "done" items, a pending unfired chain outside that window was
// silently never reconciled. This seeds 1000 "done" noise items (bulk
// inserted, no chain) with updated_at strictly AFTER the one item that DOES
// have a pending chain, so under the default sort (priority asc, updated_at
// desc) all 1000 noise items rank ahead of it — exactly the shape that would
// have pushed it past the old unfiltered 1000-row cutoff. With the fix
// (ChainFired/NextWorkflowIDSet pushed into the SQL WHERE clause, no reliance
// on row position), it must still be found and fired.
func TestTriggerChainReconciler_should_findPendingChain_When_MoreThan1000DoneItemsExist(t *testing.T) {
	t.Parallel()
	fx, cleanup := newChainTestFixture(t)
	defer cleanup()
	ctx := context.Background()

	wf := fx.createWorkflow(t, "beyond-1000-cap")
	firer := &countingTriggerFirer{}
	cf := NewChainFirer(fx.repo, fx.workflows, fx.fireEvents, firer, enabledCfg())
	reconciler := NewTriggerChainReconciler(cf)

	// The one item with a pending, unfired chain — created (and its
	// NextWorkflowID set) BEFORE the noise batch below, so its updated_at is
	// the oldest of the bunch.
	item, err := fx.repo.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "the pending chain item",
		Status: string(BacklogStatusDone),
	})
	require.NoError(t, err)
	wfID := wf.ID
	_, err = fx.repo.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{NextWorkflowID: &wfID}, nil)
	require.NoError(t, err)

	// 1000 "done" items with no pending chain, bulk-inserted (for test speed)
	// AFTER the item above so they all sort ahead of it under the default
	// (priority asc, updated_at desc) ordering — same priority, strictly more
	// recent updated_at.
	const noise = 1000
	client := fx.repo.GetEntClient()
	builders := make([]*ent.BacklogItemCreate, 0, noise)
	for i := 0; i < noise; i++ {
		builders = append(builders, client.BacklogItem.Create().
			SetTitle(fmt.Sprintf("noise done item %d", i)).
			SetStatus(string(BacklogStatusDone)).
			SetPriority(3))
	}
	_, err = client.BacklogItem.CreateBulk(builders...).Save(ctx)
	require.NoError(t, err)

	reconciler.ReconcileChains(ctx, fx.repo)

	require.Eventually(t, func() bool {
		return firer.callCount() == 1
	}, 5*time.Second, 20*time.Millisecond, "the pending chain item must still be found and fired despite 1000 more-recently-updated done items ranking ahead of it")

	final, err := fx.repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.True(t, final.ChainFired)
}

// TestChainFirer_should_fireExactlyOnce_When_DispatchAndReconcilerRaceOnSameItem
// is the concurrency-sensitive regression test the plan calls for (Task
// 6.2.1d): the happy-path async dispatch and TriggerChainReconciler's
// periodic sweep can both observe the same item with ChainFired==false and
// attempt to fire it concurrently (e.g. a chain still pending when a
// reconcile tick lands). Exactly one of them may ever reach
// TriggerFirer.FireTriggerChained — the claim-before-fire CAS in
// ChainFirer.Fire (EntRepository.ClaimChainFire) is what guarantees this.
// Run with -race to additionally prove no data race in the shared semaphore/
// counting fake.
func TestChainFirer_should_fireExactlyOnce_When_DispatchAndReconcilerRaceOnSameItem(t *testing.T) {
	t.Parallel()
	fx, cleanup := newChainTestFixture(t)
	defer cleanup()
	ctx := context.Background()

	wf := fx.createWorkflow(t, "chain-race")
	firer := &countingTriggerFirer{}
	cf := NewChainFirer(fx.repo, fx.workflows, fx.fireEvents, firer, enabledCfg())

	item, err := fx.repo.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "racing chain item",
		Status: string(BacklogStatusDone),
	})
	require.NoError(t, err)
	wfID := wf.ID
	_, err = fx.repo.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{NextWorkflowID: &wfID}, nil)
	require.NoError(t, err)

	latest, err := fx.repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)

	// Simulate several concurrent "attempts" on the exact same item snapshot
	// — the shape of the happy-path dispatch racing one (or several) reconcile
	// ticks, all of which read the item before any of their writes landed.
	const attempts = 5
	var wg sync.WaitGroup
	wg.Add(attempts)
	for i := 0; i < attempts; i++ {
		itemCopy := *latest
		go func() {
			defer wg.Done()
			cf.Dispatch(&itemCopy)
		}()
	}
	wg.Wait()

	require.Eventually(t, func() bool {
		return firer.callCount() >= 1
	}, 2*time.Second, 10*time.Millisecond)

	// Give any losing goroutine a chance to (incorrectly) also fire, if the
	// claim-before-fire CAS weren't actually closing the race.
	time.Sleep(300 * time.Millisecond)

	assert.Equal(t, 1, firer.callCount(), "exactly one concurrent Dispatch may ever reach FireTriggerChained for a given item")

	final, err := fx.repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.True(t, final.ChainFired)
}

// TestChainFirer_Dispatch_should_noOp_When_FeatureFlagDisabled covers Task
// 8.2.1b's defense-in-depth gate: Dispatch must not fire (or even reserve a
// semaphore slot) when webhook_triggers is off.
func TestChainFirer_Dispatch_should_noOp_When_FeatureFlagDisabled(t *testing.T) {
	t.Parallel()
	fx, cleanup := newChainTestFixture(t)
	defer cleanup()
	ctx := context.Background()

	wf := fx.createWorkflow(t, "chain-flag-off")
	firer := &countingTriggerFirer{}
	cf := NewChainFirer(fx.repo, fx.workflows, fx.fireEvents, firer, &config.Config{FeatureFlags: map[string]bool{"webhook_triggers": false}})

	item, err := fx.repo.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "flag off item",
		Status: string(BacklogStatusDone),
	})
	require.NoError(t, err)
	wfID := wf.ID
	_, err = fx.repo.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{NextWorkflowID: &wfID}, nil)
	require.NoError(t, err)

	latest, err := fx.repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)

	cf.Dispatch(latest)
	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, 0, firer.callCount())

	final, err := fx.repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.False(t, final.ChainFired)
}
