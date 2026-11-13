package backlog

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/backlog/internal/blktest"
	"github.com/tstapler/stapler-squad/session/backlog/reviewgate"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// blockingReviewSpawner blocks every SpawnReviewSession call until release is
// closed, signalling entered first so a test can observe overlapping spawns.
type blockingReviewSpawner struct {
	blktest.MockReviewGateSpawner
	entered chan struct{}
	release chan struct{}
}

func newBlockingReviewSpawner() *blockingReviewSpawner {
	return &blockingReviewSpawner{entered: make(chan struct{}, 16), release: make(chan struct{})}
}

func (b *blockingReviewSpawner) SpawnReviewSession(ctx context.Context, item *backlogdto.BacklogItemData, isID, prompt string) (*session.Instance, error) {
	b.entered <- struct{}{}
	<-b.release
	return b.MockReviewGateSpawner.SpawnReviewSession(ctx, item, isID, prompt)
}

// reviewGuardFixture is a listener plus one in-review item with a work session.
type reviewGuardFixture struct {
	l       *BacklogLifecycleListener
	storage *session.Storage
	item    *backlogdto.BacklogItemData
	work    backlogdto.ItemSessionSummary
}

func newReviewGuardFixture(t *testing.T, spawner reviewgate.ReviewGateSpawner, status session.BacklogStatus) *reviewGuardFixture {
	t.Helper()
	storage, cleanup := createTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()

	created, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{
		Title:    "dup review guard",
		Priority: 1,
		Status:   string(status),
		RepoPath: blktest.NewNonEmptyDiffGitRepo(t),
	})
	require.NoError(t, err)
	work, err := storage.CreateItemSession(ctx, backlogdto.ItemSessionData{
		ItemID: created.ID, SessionUUID: uuid.New().String(), SessionRole: session.SessionRoleWork,
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
		if s.Role == session.SessionRoleReview {
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
	t.Parallel()
	spawner := newBlockingReviewSpawner()
	f := newReviewGuardFixture(t, spawner, session.BacklogStatusReview)

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

	assert.Equal(t, 1, spawner.CallCount(), "exactly one reviewer must spawn per item")
	total, open := f.reviewRows(t)
	assert.Equal(t, 1, total, "exactly one review ItemSession row")
	assert.Equal(t, 1, open)
}

func TestReviewGate_ReleasedAfterSpawnError_AllowsRetry(t *testing.T) {
	t.Parallel()
	spawner := &blktest.MockReviewGateSpawner{Err: assert.AnError}
	f := newReviewGuardFixture(t, spawner, session.BacklogStatusReview)

	f.l.spawnReviewGate(builtInReviewGateContext, f.item, f.work)
	assert.False(t, f.l.ReviewSpawnGuard().InFlight(f.item.ID), "reservation must be released after a spawn error")

	spawner.SetErr(nil)
	f.l.spawnReviewGate(builtInReviewGateContext, f.item, f.work)
	assert.Equal(t, 2, spawner.CallCount(), "retry after a failed spawn must be allowed")
}

func TestReviewGate_ReservationHeldUntilItemSessionPersisted(t *testing.T) {
	t.Parallel()
	spawner := newBlockingReviewSpawner()
	f := newReviewGuardFixture(t, spawner, session.BacklogStatusReview)

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
	t.Parallel()
	t.Run("not in review", func(t *testing.T) {
		spawner := &blktest.MockReviewGateSpawner{}
		f := newReviewGuardFixture(t, spawner, session.BacklogStatusInProgress)
		f.l.triggerReviewForSession(f.work.SessionUUID)
		assert.Equal(t, 0, spawner.CallCount())
	})
	t.Run("open review session", func(t *testing.T) {
		spawner := &blktest.MockReviewGateSpawner{}
		f := newReviewGuardFixture(t, spawner, session.BacklogStatusReview)
		_, err := f.storage.CreateItemSession(context.Background(), backlogdto.ItemSessionData{
			ItemID: f.item.ID, SessionUUID: uuid.New().String(), SessionRole: session.SessionRoleReview,
		})
		require.NoError(t, err)
		f.l.triggerReviewForSession(f.work.SessionUUID)
		assert.Equal(t, 0, spawner.CallCount())
	})
	t.Run("in-flight reservation", func(t *testing.T) {
		spawner := &blktest.MockReviewGateSpawner{}
		f := newReviewGuardFixture(t, spawner, session.BacklogStatusReview)
		release, ok := f.l.ReserveReview(context.Background(), f.item.ID)
		require.True(t, ok)
		defer release()
		f.l.triggerReviewForSession(f.work.SessionUUID)
		assert.Equal(t, 0, spawner.CallCount())
	})
}

func TestReviewGate_ReReviewAllowedAfterPriorEnded(t *testing.T) {
	t.Parallel()
	spawner := &blktest.MockReviewGateSpawner{}
	f := newReviewGuardFixture(t, spawner, session.BacklogStatusReview)
	prior, err := f.storage.CreateItemSession(context.Background(), backlogdto.ItemSessionData{
		ItemID: f.item.ID, SessionUUID: uuid.New().String(), SessionRole: session.SessionRoleReview,
	})
	require.NoError(t, err)
	require.NoError(t, f.storage.UpdateItemSessionEnded(context.Background(), prior.ID, time.Now()))

	f.l.triggerReviewForSession(f.work.SessionUUID)
	assert.Equal(t, 1, spawner.CallCount())
}
