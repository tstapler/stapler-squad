package session

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blockingReviewSpawner blocks every SpawnReviewSession call until release is
// closed, signalling entered first so a test can observe overlapping spawns.
type blockingReviewSpawner struct {
	mockReviewGateSpawner
	entered chan struct{}
	release chan struct{}
}

func newBlockingReviewSpawner() *blockingReviewSpawner {
	return &blockingReviewSpawner{entered: make(chan struct{}, 16), release: make(chan struct{})}
}

func (b *blockingReviewSpawner) SpawnReviewSession(ctx context.Context, item *BacklogItemData, isID, prompt string) (*Instance, error) {
	b.entered <- struct{}{}
	<-b.release
	return b.mockReviewGateSpawner.SpawnReviewSession(ctx, item, isID, prompt)
}

// reviewGuardFixture is a listener plus one in-review item with a work session.
type reviewGuardFixture struct {
	l       *BacklogLifecycleListener
	storage *Storage
	item    *BacklogItemData
	work    ItemSessionSummary
}

func newReviewGuardFixture(t *testing.T, spawner ReviewGateSpawner, status BacklogStatus) *reviewGuardFixture {
	t.Helper()
	storage, cleanup := createTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()

	created, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:    "dup review guard",
		Priority: 1,
		Status:   string(status),
		RepoPath: newNonEmptyDiffGitRepo(t),
	})
	require.NoError(t, err)
	work, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID: created.ID, SessionUUID: uuid.New().String(), SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	l := NewBacklogLifecycleListener(storage)
	l.SetSessionCreator(spawner)
	t.Cleanup(l.Shutdown)
	return &reviewGuardFixture{l: l, storage: storage, item: created, work: work}
}

func (f *reviewGuardFixture) reviewRows(t *testing.T) (total, open int) {
	t.Helper()
	sessions, err := f.storage.ListItemSessions(context.Background(), f.item.ID)
	require.NoError(t, err)
	for _, s := range sessions {
		if s.Role == SessionRoleReview {
			total++
			if s.EndedAt == nil {
				open++
			}
		}
	}
	return total, open
}

// TestReviewGate_ConcurrentTriggers_SingleSpawn reproduces the duplicate-reviewer
// race: onSessionExited, TriggerReviewForSession and ReconcileStuckItems all
// funnel into spawnReviewGate, and nothing stopped a second caller from
// spawning while the first was still inside SpawnReviewSession.
func TestReviewGate_ConcurrentTriggers_SingleSpawn(t *testing.T) {
	spawner := newBlockingReviewSpawner()
	f := newReviewGuardFixture(t, spawner, BacklogStatusReview)

	const callers = 3
	var returned atomic.Int32
	skipped := make(chan struct{}, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f.l.spawnReviewGate(builtInReviewGateContext, f.item, f.work)
			if returned.Add(1) < callers { // the winner returns last, after release
				skipped <- struct{}{}
			}
		}()
	}

	// Either a second spawn enters the (blocked) spawner — the bug — or the
	// other two callers return without spawning.
	entered := 0
	skips := 0
	for entered < 2 && skips < callers-1 {
		select {
		case <-spawner.entered:
			entered++
		case <-skipped:
			skips++
		case <-time.After(10 * time.Second):
			t.Fatal("timed out waiting for spawn or skip")
		}
	}
	close(spawner.release)
	wg.Wait()

	assert.Equal(t, 1, spawner.getCallCount(), "exactly one reviewer must spawn per item")
	total, open := f.reviewRows(t)
	assert.Equal(t, 1, total, "exactly one review ItemSession row")
	assert.Equal(t, 1, open)
}

func TestReviewGate_ReleasedAfterSpawnError_AllowsRetry(t *testing.T) {
	spawner := &mockReviewGateSpawner{err: assert.AnError}
	f := newReviewGuardFixture(t, spawner, BacklogStatusReview)

	f.l.spawnReviewGate(builtInReviewGateContext, f.item, f.work)
	assert.False(t, f.l.ReviewSpawnGuard().InFlight(f.item.ID), "reservation must be released after a spawn error")

	spawner.mu.Lock()
	spawner.err = nil
	spawner.mu.Unlock()
	f.l.spawnReviewGate(builtInReviewGateContext, f.item, f.work)
	assert.Equal(t, 2, spawner.getCallCount(), "retry after a failed spawn must be allowed")
}

func TestReviewSpawnGuard_ReleasedOnPanicAndIdempotent(t *testing.T) {
	g := NewReviewSpawnGuard()
	func() {
		defer func() { _ = recover() }()
		release, ok := g.TryReserve("a", nil)
		require.True(t, ok)
		defer release()
		panic("boom")
	}()
	assert.False(t, g.InFlight("a"))

	release, ok := g.TryReserve("a", nil)
	require.True(t, ok)
	_, dup := g.TryReserve("a", nil)
	assert.False(t, dup)
	release()
	release()
	_, again := g.TryReserve("a", nil)
	assert.True(t, again)
}

func TestReviewGate_ReservationHeldUntilItemSessionPersisted(t *testing.T) {
	spawner := newBlockingReviewSpawner()
	f := newReviewGuardFixture(t, spawner, BacklogStatusReview)

	done := make(chan struct{})
	go func() {
		defer close(done)
		f.l.spawnReviewGate(builtInReviewGateContext, f.item, f.work)
	}()
	<-spawner.entered
	assert.True(t, f.l.ReviewSpawnGuard().InFlight(f.item.ID))
	close(spawner.release)
	<-done

	total, _ := f.reviewRows(t)
	assert.Equal(t, 1, total, "row persisted before reservation released")
	assert.False(t, f.l.ReviewSpawnGuard().InFlight(f.item.ID))
}

func TestTriggerReviewForSession_NoopWhenReviewOpenOrNotInReview(t *testing.T) {
	t.Run("not in review", func(t *testing.T) {
		spawner := &mockReviewGateSpawner{}
		f := newReviewGuardFixture(t, spawner, BacklogStatusInProgress)
		f.l.triggerReviewForSession(f.work.SessionUUID)
		assert.Equal(t, 0, spawner.getCallCount())
	})
	t.Run("open review session", func(t *testing.T) {
		spawner := &mockReviewGateSpawner{}
		f := newReviewGuardFixture(t, spawner, BacklogStatusReview)
		_, err := f.storage.CreateItemSession(context.Background(), ItemSessionData{
			ItemID: f.item.ID, SessionUUID: uuid.New().String(), SessionRole: SessionRoleReview,
		})
		require.NoError(t, err)
		f.l.triggerReviewForSession(f.work.SessionUUID)
		assert.Equal(t, 0, spawner.getCallCount())
	})
	t.Run("in-flight reservation", func(t *testing.T) {
		spawner := &mockReviewGateSpawner{}
		f := newReviewGuardFixture(t, spawner, BacklogStatusReview)
		release, ok := f.l.ReserveReview(context.Background(), f.item.ID)
		require.True(t, ok)
		defer release()
		f.l.triggerReviewForSession(f.work.SessionUUID)
		assert.Equal(t, 0, spawner.getCallCount())
	})
}

func TestReviewGate_ReReviewAllowedAfterPriorEnded(t *testing.T) {
	spawner := &mockReviewGateSpawner{}
	f := newReviewGuardFixture(t, spawner, BacklogStatusReview)
	prior, err := f.storage.CreateItemSession(context.Background(), ItemSessionData{
		ItemID: f.item.ID, SessionUUID: uuid.New().String(), SessionRole: SessionRoleReview,
	})
	require.NoError(t, err)
	require.NoError(t, f.storage.UpdateItemSessionEnded(context.Background(), prior.ID, time.Now()))

	f.l.triggerReviewForSession(f.work.SessionUUID)
	assert.Equal(t, 1, spawner.getCallCount())
}
