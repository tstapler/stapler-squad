package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

var errSpawnBoom = errors.New("tmux exploded")

// newReadyItemThatCannotSpawn returns a service plus a ready item whose spawn gate refuses
// (no repo_path), the shape of the live dequeue loop on item 0791f5b4.
func newReadyItemThatCannotSpawn(t *testing.T) (*BacklogService, *session.Storage, string) {
	t.Helper()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, &mockSessionCreator{}, nil, nil, nil, nil)
	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:              "cannot spawn",
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
	return svc, storage, itemID
}

// newServiceWithFailingCreator returns a service whose creator fails every spawn, plus the
// creator so a test can later make spawns succeed.
func newServiceWithFailingCreator(t *testing.T) (*BacklogService, *session.Storage, *mockSessionCreator) {
	t.Helper()
	storage := createTestStorage(t)
	creator := &mockSessionCreator{err: errSpawnBoom}
	return NewBacklogService(storage, creator, nil, nil, nil, nil), storage, creator
}

func newReadyItemWithRepo(t *testing.T, svc *BacklogService, title string) string {
	t.Helper()
	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)
	return createReadyItemForSpawn(t, svc, repoPath, title)
}

func (m *mockSessionCreator) setErr(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

func countEventsWithNote(t *testing.T, storage *session.Storage, itemID, note string) int {
	t.Helper()
	item, err := storage.GetBacklogItem(t.Context(), itemID)
	require.NoError(t, err)
	n := 0
	for _, e := range item.StatusEvents {
		if e.Note != nil && *e.Note == note {
			n++
		}
	}
	return n
}

func openSpawnFailedRow(t *testing.T, storage *session.Storage, itemID string) (session.OpenStuckStateData, bool) {
	t.Helper()
	row, ok, err := storage.OpenStuckState(t.Context(), itemID, domain.StuckReasonSpawnFailed)
	require.NoError(t, err)
	return row, ok
}

// expireBackoff moves the row's deadline into the past, as if the wait elapsed.
func expireBackoff(t *testing.T, storage *session.Storage, itemID string) {
	t.Helper()
	row, ok := openSpawnFailedRow(t, storage, itemID)
	require.True(t, ok)
	past := time.Now().Add(-time.Minute)
	_, err := storage.RecordRemediationAttempt(t.Context(), itemID, domain.StuckReasonSpawnFailed, row.RemediationAttempts, &past)
	require.NoError(t, err)
}

func drainNotificationTitles(ch <-chan *events.Event) []string {
	var titles []string
	for {
		select {
		case ev := <-ch:
			if ev.Type == events.EventNotification {
				titles = append(titles, ev.NotificationTitle)
			}
		default:
			return titles
		}
	}
}

// Regression for the live ready<->in_progress loop: with no backoff every sweep re-claimed
// the item, failed, and rolled back (one flip pair per minute). After the first real
// failure the item must sit out the backoff window, however many sweeps run inside it.
func TestDequeueNextQueuedItems_BacksOffAfterRealSpawnFailure(t *testing.T) {
	t.Parallel()
	svc, storage, _ := newServiceWithFailingCreator(t)
	itemID := newReadyItemWithRepo(t, svc, "real failure")

	for range 10 {
		require.NoError(t, svc.DequeueNextQueuedItems(t.Context()))
	}

	assert.Equal(t, 1, countEventsWithNote(t, storage, itemID, "dequeue spawn failed"),
		"ten sweeps inside the backoff window may roll back only once")
	assert.Equal(t, 1, countEventsWithNote(t, storage, itemID, "dequeued: WIP slot freed"),
		"ten sweeps inside the backoff window may claim only once")
	item, err := storage.GetBacklogItem(t.Context(), itemID)
	require.NoError(t, err)
	assert.Equal(t, "ready", item.Status)

	row, ok := openSpawnFailedRow(t, storage, itemID)
	require.True(t, ok, "the failure must surface as an open spawn_failed stuck row")
	assert.Equal(t, int32(1), row.RemediationAttempts)
	require.NotNil(t, row.NextRemediationAt, "the backoff deadline must be durable in next_remediation_at")
	assert.True(t, domain.IsDequeueSpawnFailedContext(row.Context), "row must be distinguishable from the review-reopen use")
}

// The backoff is durable, so a freshly constructed service (a restart: empty in-memory
// state) must still sit out the window.
func TestDequeueNextQueuedItems_BackoffSurvivesServiceRestart(t *testing.T) {
	t.Parallel()
	svc, storage, creator := newServiceWithFailingCreator(t)
	itemID := newReadyItemWithRepo(t, svc, "restart")
	require.NoError(t, svc.DequeueNextQueuedItems(t.Context()))
	require.Equal(t, 1, countEventsWithNote(t, storage, itemID, "dequeue spawn failed"))

	restarted := NewBacklogService(storage, creator, nil, nil, nil, nil)
	for range 3 {
		require.NoError(t, restarted.DequeueNextQueuedItems(t.Context()))
	}

	assert.Equal(t, 1, countEventsWithNote(t, storage, itemID, "dequeued: WIP slot freed"))
}

// A gate refusal (here: no repo_path) is not a failure of the item: it must not open a
// spawn_failed row or spend attempts, but a short in-memory hold still stops the per-sweep flip.
func TestDequeueNextQueuedItems_GateRefusalIsHeldButNotCounted(t *testing.T) {
	t.Parallel()
	svc, storage, itemID := newReadyItemThatCannotSpawn(t)
	now := time.Unix(1_700_000_000, 0)
	svc.spawnGuard.now = func() time.Time { return now }

	for range 10 {
		require.NoError(t, svc.DequeueNextQueuedItems(t.Context()))
	}
	assert.Equal(t, 1, countEventsWithNote(t, storage, itemID, "dequeue spawn failed"), "hold must stop the flip loop")
	_, ok := openSpawnFailedRow(t, storage, itemID)
	assert.False(t, ok, "a refusal must not open a spawn_failed row")

	now = now.Add(dequeueRefusalHold + time.Second)
	require.NoError(t, svc.DequeueNextQueuedItems(t.Context()))
	assert.Equal(t, 2, countEventsWithNote(t, storage, itemID, "dequeue spawn failed"), "item is retried once the hold expires")
	_, ok = openSpawnFailedRow(t, storage, itemID)
	assert.False(t, ok, "still no spawn_failed row after repeated refusals")
}

func TestClassifySpawnFailure(t *testing.T) {
	t.Parallel()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	cases := []struct {
		name string
		ctx  context.Context
		err  error
		want spawnFailureKind
	}{
		{"missing repo_path or no-op gate", t.Context(), connect.NewError(connect.CodeFailedPrecondition, errors.New("x")), spawnFailureRefusal},
		{"active session", t.Context(), connect.NewError(connect.CodeAlreadyExists, errors.New("x")), spawnFailureRefusal},
		{"creator unwired", t.Context(), connect.NewError(connect.CodeUnimplemented, errors.New("x")), spawnFailureRefusal},
		{"cancelled ctx", canceled, errSpawnBoom, spawnFailureCancelled},
		{"context.Canceled error", t.Context(), context.Canceled, spawnFailureCancelled},
		{"plain failure", t.Context(), errSpawnBoom, spawnFailureReal},
		{"internal connect error", t.Context(), connect.NewError(connect.CodeInternal, errors.New("x")), spawnFailureReal},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, classifySpawnFailure(tc.ctx, tc.err), tc.name)
	}
}

// Wiring: a successful spawn must clear the failure history (counters and row), so the
// next failure starts at step one.
func TestClaimAndSpawnCandidate_SuccessClearsSpawnFailure(t *testing.T) {
	t.Parallel()
	svc, storage, creator := newServiceWithFailingCreator(t)
	itemID := newReadyItemWithRepo(t, svc, "recovers")
	require.NoError(t, svc.DequeueNextQueuedItems(t.Context()))
	_, ok := openSpawnFailedRow(t, storage, itemID)
	require.True(t, ok)

	creator.setErr(nil)
	expireBackoff(t, storage, itemID)
	require.NoError(t, svc.DequeueNextQueuedItems(t.Context()))

	item, err := storage.GetBacklogItem(t.Context(), itemID)
	require.NoError(t, err)
	require.Equal(t, "in_progress", item.Status, "the retry must have spawned")
	_, ok = openSpawnFailedRow(t, storage, itemID)
	assert.False(t, ok, "success must resolve the spawn_failed row")
}

// Running out of fast attempts parks the row and tells the operator once; retries after the
// cold-retry deadline keep the row blocked until the next one.
func TestDequeueNextQueuedItems_ParksAfterFastAttemptsAndNotifiesOnce(t *testing.T) {
	t.Parallel()
	svc, storage, _ := newServiceWithFailingCreator(t)
	bus := events.NewEventBus(100)
	svc.SetEventBus(bus)
	ch, _ := bus.Subscribe(t.Context())
	itemID := newReadyItemWithRepo(t, svc, "parks")

	require.NoError(t, svc.DequeueNextQueuedItems(t.Context()))
	for range session.MaxRemediationAttempts - 1 {
		expireBackoff(t, storage, itemID)
		require.NoError(t, svc.DequeueNextQueuedItems(t.Context()))
	}
	row, ok := openSpawnFailedRow(t, storage, itemID)
	require.True(t, ok)
	require.Equal(t, session.MaxRemediationAttempts, row.RemediationAttempts)
	assert.Equal(t, []string{"Backlog spawn keeps failing"}, drainNotificationTitles(ch), "parking notifies exactly once")

	blocked, err := storage.RemediationBlocked(t.Context(), itemID, domain.StuckReasonSpawnFailed)
	require.NoError(t, err)
	assert.True(t, blocked, "parked until the cold-retry deadline")
}

func TestNotifyDequeueSpawnParked_TextDependsOnWhetherRetriesStop(t *testing.T) {
	t.Parallel()
	svc, storage, _ := newServiceWithFailingCreator(t)
	bus := events.NewEventBus(100)
	svc.SetEventBus(bus)
	ch, _ := bus.Subscribe(t.Context())
	itemID := newReadyItemWithRepo(t, svc, "text")
	item, err := storage.GetBacklogItem(t.Context(), itemID)
	require.NoError(t, err)
	_, err = storage.MarkStuck(t.Context(), itemID, domain.StuckReasonSpawnFailed, session.BacklogStatusReady, "c")
	require.NoError(t, err)

	message := func(attempts int32) string {
		_, err := storage.RecordRemediationAttempt(t.Context(), itemID, domain.StuckReasonSpawnFailed, attempts, nil)
		require.NoError(t, err)
		svc.notifyDequeueSpawnParked(t.Context(), *item, errSpawnBoom)
		return (<-ch).NotificationMessage
	}

	assert.Contains(t, message(session.MaxRemediationAttempts), "a week apart")
	final := message(session.MaxRemediationAttempts + session.MaxRemediationColdRetries)
	assert.Contains(t, final, "have now stopped")
	assert.NotContains(t, final, "a week apart")
}

// Operator fixes must clear the backoff instead of leaving the item parked for up to a week.
func TestDequeueSpawnBackoff_ClearedByOperatorActions(t *testing.T) {
	t.Parallel()
	setup := func(t *testing.T) (*BacklogService, *session.Storage, string) {
		svc, storage, _ := newServiceWithFailingCreator(t)
		itemID := newReadyItemWithRepo(t, svc, "operator")
		require.NoError(t, svc.DequeueNextQueuedItems(t.Context()))
		_, ok := openSpawnFailedRow(t, storage, itemID)
		require.True(t, ok)
		return svc, storage, itemID
	}
	assertCleared := func(t *testing.T, svc *BacklogService, storage *session.Storage, itemID string) {
		t.Helper()
		_, ok := openSpawnFailedRow(t, storage, itemID)
		assert.False(t, ok, "row must be resolved")
		assert.False(t, svc.spawnGuard.held(itemID))
	}

	t.Run("repo_path change", func(t *testing.T) {
		t.Parallel()
		svc, storage, itemID := setup(t)
		newRepo := t.TempDir()
		initGitRepoWithCommit(t, newRepo)
		_, err := svc.UpdateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.UpdateBacklogItemRequest{
			ItemId: itemID, RepoPath: newRepo, SkipPlanning: true, // the RPC overwrites these booleans
		}))
		require.NoError(t, err)
		assertCleared(t, svc, storage, itemID)
		require.NoError(t, svc.DequeueNextQueuedItems(t.Context()))
		assert.Equal(t, 2, countEventsWithNote(t, storage, itemID, "dequeued: WIP slot freed"), "the fixed item is retried at once")
	})
	t.Run("same repo_path is not a fix", func(t *testing.T) {
		t.Parallel()
		svc, storage, itemID := setup(t)
		item, err := storage.GetBacklogItem(t.Context(), itemID)
		require.NoError(t, err)
		_, err = svc.UpdateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.UpdateBacklogItemRequest{
			ItemId: itemID, RepoPath: item.RepoPath,
		}))
		require.NoError(t, err)
		_, ok := openSpawnFailedRow(t, storage, itemID)
		assert.True(t, ok)
	})
	t.Run("manual transition", func(t *testing.T) {
		t.Parallel()
		svc, storage, itemID := setup(t)
		_, err := svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
			ItemId: itemID, TargetStatus: "idea",
		}))
		require.NoError(t, err)
		assertCleared(t, svc, storage, itemID)
	})
	t.Run("manual spawn", func(t *testing.T) {
		t.Parallel()
		svc, storage, itemID := setup(t)
		_, err := svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID}))
		require.Error(t, err, "the creator still fails; only the clearing is under test")
		assertCleared(t, svc, storage, itemID)
	})
}

// When the spawn path itself is down, failures across distinct items must not walk every
// item up the backoff ladder; the operator hears about it once.
func TestDequeueNextQueuedItems_SystemicOutageDoesNotSpendPerItemBudget(t *testing.T) {
	t.Parallel()
	svc, storage, creator := newServiceWithFailingCreator(t)
	bus := events.NewEventBus(100)
	svc.SetEventBus(bus)
	ch, _ := bus.Subscribe(t.Context())
	now := time.Unix(1_700_000_000, 0)
	svc.spawnGuard.now = func() time.Time { return now }
	ids := make([]string, dequeueSystemicItems)
	for i := range ids {
		ids[i] = newReadyItemWithRepo(t, svc, "outage")
	}

	require.NoError(t, svc.DequeueNextQueuedItems(t.Context()))

	counted := 0
	for _, id := range ids {
		if row, ok := openSpawnFailedRow(t, storage, id); ok {
			counted++
			assert.Equal(t, int32(1), row.RemediationAttempts)
		}
	}
	assert.Equal(t, dequeueSystemicItems-1, counted, "the failure that reveals the outage is not charged to its item")
	assert.Equal(t, []string{"Backlog spawns failing across items"}, drainNotificationTitles(ch))

	now = now.Add(dequeueSystemicHold + time.Second)
	require.NoError(t, svc.DequeueNextQueuedItems(t.Context()))
	assert.Empty(t, drainNotificationTitles(ch), "one alert per streak")

	creator.setErr(nil)
	now = now.Add(dequeueSystemicHold + time.Second)
	require.NoError(t, svc.DequeueNextQueuedItems(t.Context()))
	svc.spawnGuard.mu.Lock()
	defer svc.spawnGuard.mu.Unlock()
	assert.Empty(t, svc.spawnGuard.streak, "a success ends the streak")
}

func TestDequeueSpawnGuard_HoldsExpireOnInjectedClock(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_000, 0)
	g := &dequeueSpawnGuard{now: func() time.Time { return now }}

	assert.False(t, g.held("a"))
	g.hold("a", time.Minute)
	assert.True(t, g.held("a"))
	now = now.Add(time.Minute)
	assert.False(t, g.held("a"), "expired at the deadline")

	g.hold("a", time.Hour)
	g.forget("a")
	assert.False(t, g.held("a"))
}

func TestDequeueSpawnGuard_StreakNeedsDistinctItemsAndAlertsOncePerStreak(t *testing.T) {
	t.Parallel()
	var g dequeueSpawnGuard
	for range 5 {
		systemic, _ := g.noteRealFailure("same")
		assert.False(t, systemic, "one item failing repeatedly is that item's problem")
	}
	g.noteRealFailure("b")
	systemic, first := g.noteRealFailure("c")
	assert.True(t, systemic)
	assert.True(t, first)
	systemic, first = g.noteRealFailure("d")
	assert.True(t, systemic)
	assert.False(t, first, "alert once per streak")

	g.noteSuccess("x")
	systemic, _ = g.noteRealFailure("a")
	assert.False(t, systemic, "a success resets the streak")
}

// MarkStuck applies only while the item is still in the expected status; when it is not,
// the failure is logged, no row is opened, and no hold is set for an item that moved on.
func TestRecordDequeueSpawnFailure_UnappliedMarkStuckOpensNoRowAndNoHold(t *testing.T) {
	t.Parallel()
	svc, storage, itemID := newReadyItemThatCannotSpawn(t)
	item, err := storage.GetBacklogItem(t.Context(), itemID)
	require.NoError(t, err)

	svc.recordDequeueSpawnFailure(t.Context(), *item, session.BacklogStatusQueued /* item is ready */, errSpawnBoom)

	_, ok := openSpawnFailedRow(t, storage, itemID)
	assert.False(t, ok)
	svc.spawnGuard.mu.Lock()
	defer svc.spawnGuard.mu.Unlock()
	assert.NotContains(t, svc.spawnGuard.holds, itemID)
}

// Streak membership decays: failures spread over more than the window are not "together".
func TestDequeueSpawnGuard_StreakEntriesExpire(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_000, 0)
	g := &dequeueSpawnGuard{now: func() time.Time { return now }}
	g.noteRealFailure("a")
	g.noteRealFailure("b")
	now = now.Add(dequeueStreakWindow + time.Second)
	systemic, _ := g.noteRealFailure("c")
	assert.False(t, systemic, "stale failures must not combine with a new one into an outage")
}

// An item inside a streak is exempted from the durable budget only a bounded number of times.
func TestDequeueSpawnGuard_SystemicExemptionIsBoundedPerItem(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_000, 0)
	g := &dequeueSpawnGuard{now: func() time.Time { return now }}
	g.noteRealFailure("b")
	g.noteRealFailure("c")
	for range dequeueSystemicMaxHolds {
		systemic, _ := g.noteRealFailure("a")
		assert.True(t, systemic)
		now = now.Add(dequeueSystemicHold + time.Second)
	}
	systemic, _ := g.noteRealFailure("a")
	assert.False(t, systemic, "after the bounded exemptions the failure is charged to the item")
	g.noteSuccess("zzz")
	systemic, _ = g.noteRealFailure("a")
	assert.False(t, systemic, "another item's success does not refund a's exemptions")
}

// Regression: three persistently broken items used to sit in the systemic streak forever,
// each re-claimed every 30m with no durable attempt, never parked, never notified.
func TestDequeueNextQueuedItems_PersistentlyFailingItemsStillPark(t *testing.T) {
	t.Parallel()
	svc, storage, _ := newServiceWithFailingCreator(t)
	bus := events.NewEventBus(100)
	svc.SetEventBus(bus)
	ch, _ := bus.Subscribe(t.Context())
	now := time.Unix(1_700_000_000, 0)
	svc.spawnGuard.now = func() time.Time { return now }
	ids := make([]string, dequeueSystemicItems)
	for i := range ids {
		ids[i] = newReadyItemWithRepo(t, svc, "broken")
	}

	titles := make([]string, 0, 8)
	for range 40 {
		require.NoError(t, svc.DequeueNextQueuedItems(t.Context()))
		titles = append(titles, drainNotificationTitles(ch)...)
		for _, id := range ids {
			if _, ok := openSpawnFailedRow(t, storage, id); ok {
				expireBackoff(t, storage, id)
			}
		}
		now = now.Add(dequeueSystemicHold + time.Second)
	}

	assert.Contains(t, titles, "Backlog spawn keeps failing")
	for _, id := range ids {
		row, ok := openSpawnFailedRow(t, storage, id)
		require.True(t, ok)
		assert.Greater(t, row.RemediationAttempts, int32(2), "item %s advanced up the ladder", id)
	}
}

// A genuine brief outage (several items fail once, then a spawn succeeds) charges nothing
// to the item whose failure revealed it and leaves no exemption debt behind.
func TestDequeueSpawnGuard_BriefOutageChargesNothingAndResets(t *testing.T) {
	t.Parallel()
	var g dequeueSpawnGuard
	g.noteRealFailure("a")
	g.noteRealFailure("b")
	systemic, _ := g.noteRealFailure("c")
	assert.True(t, systemic)
	g.noteSuccess("a")
	g.mu.Lock()
	defer g.mu.Unlock()
	assert.Empty(t, g.streak)
}

// A refusal that never resolves must not flip ready<->in_progress forever in silence: after
// dequeueRefusalEscalateAfter consecutive refusals it takes the durable path.
func TestDequeueNextQueuedItems_PersistentRefusalEscalatesToDurableRow(t *testing.T) {
	t.Parallel()
	svc, storage, itemID := newReadyItemThatCannotSpawn(t)
	now := time.Unix(1_700_000_000, 0)
	svc.spawnGuard.now = func() time.Time { return now }

	for i := 1; i < dequeueRefusalEscalateAfter; i++ {
		require.NoError(t, svc.DequeueNextQueuedItems(t.Context()))
		_, ok := openSpawnFailedRow(t, storage, itemID)
		assert.False(t, ok, "refusal %d is only held in memory", i)
		now = now.Add(dequeueRefusalHold + time.Second)
	}
	require.NoError(t, svc.DequeueNextQueuedItems(t.Context()))
	row, ok := openSpawnFailedRow(t, storage, itemID)
	require.True(t, ok, "the persistent refusal opens a durable row")
	assert.Equal(t, int32(1), row.RemediationAttempts)
}
