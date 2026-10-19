package wakes

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// diskHarness boots successive dispatcher "processes" over one on-disk SQLite
// file, with the fakes (clock, pane, owners) surviving across boots like the
// real world around a restart does.
type diskHarness struct {
	t        *testing.T
	path     string
	clock    *FakeClock
	steerer  *fakeSteerer
	owners   *fakeOwners
	notifier *recordingNotifier
	backlog  *fakeBacklog
}

func newDiskHarness(t *testing.T) *diskHarness {
	t.Helper()
	h := &diskHarness{
		t: t, path: filepath.Join(t.TempDir(), "wakes.db"), clock: NewFakeClock(t0),
		steerer: newFakeSteerer(), owners: newFakeOwners(), notifier: &recordingNotifier{},
		backlog: &fakeBacklog{items: map[string]ItemState{}},
	}
	h.steerer.setReady("owner-1", true)
	return h
}

// process is one booted server: a repository, a dispatcher, and a way to kill it.
type process struct {
	repo *session.EntRepository
	d    *Dispatcher
	once sync.Once
}

// boot opens the database file and builds (but does not start) a dispatcher.
func (h *diskHarness) boot(mutate ...func(*Config)) *process {
	h.t.Helper()
	repo, err := session.NewEntRepository(session.WithDatabasePath(h.path))
	require.NoError(h.t, err)
	cfg := Config{
		Store: repo, Clock: h.clock, Steerer: h.steerer, Admission: &fixedAdmission{}, Owners: h.owners,
		Notifier: h.notifier, Events: events.NewEventBus(64), Backlog: h.backlog,
	}
	for _, m := range mutate {
		m(&cfg)
	}
	d := NewDispatcher(cfg)
	repo.SetWakeChangeHook(d.ChangeHook())
	p := &process{repo: repo, d: d}
	h.t.Cleanup(p.kill)
	return p
}

func (h *diskHarness) start(p *process) {
	h.t.Helper()
	require.NoError(h.t, p.d.Start(context.Background()))
}

// kill stops the process and closes its database, as exit does. In-memory
// state (the SteerDelivery cache, the index) is gone; only the file remains.
func (p *process) kill() {
	p.once.Do(func() {
		_ = p.d.Stop(context.Background())
		_ = p.repo.Close()
	})
}

func (p *process) status(t *testing.T, id string) domain.WakeStatus { return statusOf(t, p.repo, id) }

// Success metric "wakes survive restart" (Story 4.8): three pending triggers, one due
// during downtime, one whose condition became true while down, one claimed
// by the dead process; the restarted server delivers each once.
func TestRestart_should_deliverEachOfThreePendingTriggersOnceAndMatchReconcileCounters_When_serverRestartsOnSameDB(t *testing.T) {
	t.Parallel()
	h := newDiskHarness(t)
	ctx := context.Background()
	h.steerer.setReady("owner-2", true)
	h.steerer.setReady("owner-3", true)

	first := h.boot()
	h.start(first)
	due := t0.Add(10 * time.Minute)
	expires := t0.Add(6 * time.Hour)
	dueRow := newRow(t, first.repo, rowOpts{owner: "owner-1", payload: "due during downtime", spec: domain.AtSpec{At: due}, nextFire: &due, expiresAt: &expires})
	eventRow := backlogRow(t, &dispatcherRig{repo: first.repo}, itemX, "verdict_recorded", rowOpts{owner: "owner-2", payload: "condition true while down", expiresAt: &expires})
	claimedAt := t0.Add(-5 * time.Minute)
	staleRow := claimedRow(t, first.repo, claimedAt, rowOpts{owner: "owner-3", payload: "claimed by the dead process", expiresAt: &expires})
	first.kill()

	h.clock.Advance(20 * time.Minute) // downtime
	h.backlog.items[itemX] = ItemState{Exists: true, HasVerdict: true}
	second := h.boot()
	h.start(second)

	m := second.d.Metrics()
	for _, id := range []string{dueRow.ID, eventRow.ID, staleRow.ID} {
		assert.Equal(t, domain.WakeStatusDelivered, second.status(t, id), id)
	}
	assert.Len(t, h.steerer.pane.written(), 3, "each trigger is written exactly once")
	assert.EqualValues(t, 1, m.Value(MetricReconcile, ReconcileRequeuedClaimed))
	assert.EqualValues(t, 1, m.Value(MetricReconcile, ReconcileFired))
	assert.EqualValues(t, 2, m.Value(MetricTrigger, "at", "delivered"), "the due row and the re-queued claim are at-rows")
	assert.EqualValues(t, 1, m.Value(MetricTrigger, "backlog_event", "delivered"))
	assert.EqualValues(t, 0, m.Value(MetricDuplicate))
	second.kill()

	third := h.boot()
	h.start(third)
	h.clock.Advance(time.Hour)
	third.d.Reconcile(ctx)
	assert.Len(t, h.steerer.pane.written(), 3, "a further restart re-sends nothing")
	for _, id := range []string{dueRow.ID, eventRow.ID, staleRow.ID} {
		assert.Equal(t, domain.WakeStatusDelivered, third.status(t, id))
	}
}

// Rows past expires_at at restart become missed with one operator notification
// naming them, never a silent drop (Story 4.8, plan line "reconcile completes").
func TestRestart_should_markExpiredRowMissedAndNotifyOnce_When_expiryPassedDuringDowntime(t *testing.T) {
	t.Parallel()
	h := newDiskHarness(t)
	first := h.boot()
	due := t0.Add(10 * time.Minute)
	expires := t0.Add(30 * time.Minute)
	row := newRow(t, first.repo, rowOpts{spec: domain.AtSpec{At: due}, nextFire: &due, expiresAt: &expires})
	first.kill()

	h.clock.Advance(2 * time.Hour)
	second := h.boot()
	h.start(second)

	assert.Equal(t, domain.WakeStatusMissed, second.status(t, row.ID))
	assert.Empty(t, h.steerer.pane.written())
	require.Len(t, h.notifier.all(), 1)
	assert.Equal(t, NotifyFailure, h.notifier.all()[0].Kind)
}
