package services

// Characterization tests for onAutonomousDriverComplete's work-role stall arm.
// They were written green against the pre-extraction code (Task 3.1.1a) and pin
// the branches the older tests do not reach, so later stories that move the arm
// into handleWorkStall cannot change them silently.

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// charFixture is one backlog-linked autonomous session with a subscribed event channel.
type charFixture struct {
	svc     *SessionService
	storage *session.Storage
	inst    *session.Instance
	title   string
	itemID  string
	itemSes string
	ch      <-chan *events.Event
}

func newCharFixture(t *testing.T, title string, role string, itemStatus session.BacklogStatus) *charFixture {
	t.Helper()
	storage := createTestStorage(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	bus := events.NewEventBus(100)
	svc := NewSessionService(storage, bus)
	t.Cleanup(func() { svc.Shutdown() })
	svc.SetLifecycleContext(ctx)

	inst := &session.Instance{
		Title: title, UUID: title + "-uuid", Path: "/tmp/test",
		Status: session.Paused, Program: "claude", AutonomousMode: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))
	svc.autonomousSvc.SetInstanceFinder(func(_ string) *session.Instance { return inst })

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: title + " item", Status: string(itemStatus)})
	require.NoError(t, err)
	is, err := storage.CreateItemSession(ctx, session.ItemSessionData{ItemID: item.ID, SessionUUID: inst.UUID, SessionRole: role})
	require.NoError(t, err)

	ch, _ := bus.Subscribe(ctx)
	return &charFixture{svc: svc, storage: storage, inst: inst, title: title, itemID: item.ID, itemSes: is.ID, ch: ch}
}

func (f *charFixture) complete(outcome session.AutonomousDriverOutcome) {
	f.svc.autonomousSvc.onAutonomousDriverComplete(f.title, outcome)
}

// drain returns every event already published; the callback publishes synchronously.
func (f *charFixture) drain() []*events.Event {
	var out []*events.Event
	for {
		select {
		case ev := <-f.ch:
			out = append(out, ev)
		default:
			return out
		}
	}
}

func notificationTitles(evs []*events.Event) []string {
	var titles []string
	for _, ev := range evs {
		if ev.Type == events.EventNotification {
			titles = append(titles, ev.NotificationTitle)
		}
	}
	return titles
}

// seedAutonomousStuckRow opens the item's autonomous_stuck row with the given
// remediation bookkeeping, as a previous stall would have left it.
func (f *charFixture) seedAutonomousStuckRow(t *testing.T, attempts int32, nextAt *time.Time) {
	t.Helper()
	ctx := context.Background()
	applied, err := f.storage.MarkStuck(ctx, f.itemID, domain.StuckReasonAutonomousStuck, session.BacklogStatusInProgress, "seed")
	require.NoError(t, err)
	require.True(t, applied)
	_, err = f.storage.RecordRemediationAttempt(ctx, f.itemID, domain.StuckReasonAutonomousStuck, attempts, nextAt)
	require.NoError(t, err)
}

func (f *charFixture) openStuckRow(t *testing.T) session.OpenStuckStateData {
	t.Helper()
	open, err := f.storage.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	require.Len(t, open, 1)
	return open[0]
}

var stallOutcome = session.AutonomousDriverOutcome{Done: false, Reason: "no DONE signal", Turns: 20, Stuck: true}

// signalingRespawner reports each respawn on a buffered channel.
type signalingRespawner struct{ called chan string }

func newSignalingRespawner() *signalingRespawner {
	return &signalingRespawner{called: make(chan string, 4)}
}

func (r *signalingRespawner) AutoRespawnAutonomousWork(_ context.Context, itemID string) error {
	r.called <- itemID
	return nil
}

func TestOnAutonomousDriverComplete_Characterization_RemediationNotDue_SkipsRespawn(t *testing.T) {
	// Not parallel: captureLogs swaps the shared slog seam.
	f := newCharFixture(t, "char-not-due", session.SessionRoleWork, session.BacklogStatusInProgress)
	respawner := newSignalingRespawner()
	f.svc.SetAutonomousStuckRespawner(respawner)
	future := time.Now().Add(time.Hour)
	f.seedAutonomousStuckRow(t, 1, &future)
	buf := captureLogs(t)

	f.complete(stallOutcome)

	assert.Contains(t, buf.String(), "remediation backoff not yet due, skipping respawn")
	assert.Empty(t, respawner.called, "a not-due gate must not dispatch a respawn")
	row := f.openStuckRow(t)
	assert.Equal(t, int32(1), row.RemediationAttempts, "a skipped occurrence must not consume an attempt")
	is, err := f.storage.GetItemSessionBySessionUUID(context.Background(), f.inst.UUID)
	require.NoError(t, err)
	assert.NotNil(t, is.EndedAt, "the stalled ItemSession is ended before the gate, even when the respawn is skipped")
	assert.Contains(t, notificationTitles(f.drain()), "Autonomous fix stuck")
}

func TestOnAutonomousDriverComplete_Characterization_JustParked_NotifiesAndStillRespawns(t *testing.T) {
	f := newCharFixture(t, "char-just-parked", session.SessionRoleWork, session.BacklogStatusInProgress)
	respawner := newSignalingRespawner()
	f.svc.SetAutonomousStuckRespawner(respawner)
	past := time.Now().Add(-time.Minute)
	f.seedAutonomousStuckRow(t, session.MaxRemediationAttempts-1, &past)

	f.complete(stallOutcome)

	titles := notificationTitles(f.drain())
	assert.Contains(t, titles, "Automated retry paused")
	assert.Contains(t, titles, "Autonomous fix stuck")
	select {
	case got := <-respawner.called:
		assert.Equal(t, f.itemID, got)
	case <-time.After(10 * time.Second): // hang guard, not a latency assertion
		t.Fatal("the parking attempt is still granted and must dispatch its respawn")
	}
	assert.Equal(t, session.MaxRemediationAttempts, f.openStuckRow(t).RemediationAttempts)
}

func TestOnAutonomousDriverComplete_Characterization_UpdateItemSessionEndedFails_NotifiesBookkeepingFailed(t *testing.T) {
	f := newCharFixture(t, "char-end-fails", session.SessionRoleWork, session.BacklogStatusInProgress)
	respawner := newSignalingRespawner()
	f.svc.SetAutonomousStuckRespawner(respawner)
	session.TestFailItemSessionUpdates(t, f.storage)

	f.complete(stallOutcome)

	titles := notificationTitles(f.drain())
	assert.Contains(t, titles, "Stuck-work bookkeeping failed")
	assert.Contains(t, titles, "Autonomous fix stuck", "the generic notification still fires after a bookkeeping failure")
	select {
	case <-respawner.called:
	case <-time.After(10 * time.Second): // hang guard
		t.Fatal("a failed end-session write must not suppress the respawn dispatch")
	}
}

func TestOnAutonomousDriverComplete_Characterization_NilRespawner_LeavesItemInProgress(t *testing.T) {
	f := newCharFixture(t, "char-nil-respawner", session.SessionRoleWork, session.BacklogStatusInProgress)
	f.svc.SetAutonomousStuckRespawner(nil)

	f.complete(stallOutcome)

	row := f.openStuckRow(t)
	assert.Equal(t, domain.StuckReasonAutonomousStuck, row.Reason)
	assert.Equal(t, int32(0), row.RemediationAttempts, "no respawner means the gate is never consulted")
	is, err := f.storage.GetItemSessionBySessionUUID(context.Background(), f.inst.UUID)
	require.NoError(t, err)
	assert.NotNil(t, is.EndedAt)
	item, err := f.storage.GetBacklogItem(context.Background(), f.itemID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusInProgress), item.Status)
	assert.Equal(t, []string{"Autonomous fix stuck"}, notificationTitles(f.drain()))
}

// SVC-11: the SessionUpdatedEvent carries the values the write left on the instance.
func TestOnAutonomousDriverComplete_Characterization_EventPayloadEqualsPostWriteSnapshot(t *testing.T) {
	cases := []struct {
		name        string
		outcome     session.AutonomousDriverOutcome
		wantOutcome string
	}{
		{"done", session.AutonomousDriverOutcome{Done: true, Reason: "done", Turns: 3}, "done"},
		{"stall", stallOutcome, "stuck"},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newCharFixture(t, "char-event-payload-"+string(rune('a'+i)), session.SessionRoleWork, session.BacklogStatusInProgress)

			f.complete(tc.outcome)

			var updated *events.Event
			for _, ev := range f.drain() {
				if ev.Type == events.EventSessionUpdated && assert.ObjectsAreEqual([]string{"autonomous_mode", "autonomous_outcome"}, ev.UpdatedFields) {
					updated = ev
				}
			}
			require.NotNil(t, updated, "expected the autonomous_mode/autonomous_outcome update event")
			require.Same(t, f.inst, updated.Session)
			assert.False(t, updated.Session.AutonomousMode)
			assert.Equal(t, tc.wantOutcome, updated.Session.AutonomousOutcome)
			assert.Equal(t, f.inst.AutonomousOutcome, updated.Session.AutonomousOutcome)
		})
	}
}

// SVC-12: exactly one MarkStuck per stall for every role, none for a DONE run.
func TestOnAutonomousDriverComplete_Characterization_MarkStuckOncePerRolePerStall(t *testing.T) {
	roles := []struct {
		name   string
		role   string
		status session.BacklogStatus
	}{
		{"work", session.SessionRoleWork, session.BacklogStatusInProgress},
		{"triage", session.SessionRoleTriage, session.BacklogStatusIdea},
		{"review", session.SessionRoleReview, session.BacklogStatusReview},
		{"unrecognized", "future-pipeline-stage", session.BacklogStatusIdea},
	}
	for _, r := range roles {
		for _, done := range []bool{false, true} {
			name := r.name + "/stall"
			want := int64(1)
			outcome := stallOutcome
			if done {
				name = r.name + "/done"
				want = 0
				outcome = session.AutonomousDriverOutcome{Done: true, Reason: "done", Turns: 3}
			}
			t.Run(name, func(t *testing.T) {
				f := newCharFixture(t, "char-markstuck-"+r.name+"-"+name[len(r.name)+1:], r.role, r.status)
				f.svc.SetAutonomousStuckRespawner(nil)
				writes := session.TestCountMarkStuckWrites(t, f.storage)

				f.complete(outcome)

				assert.Equal(t, want, writes())
			})
		}
	}
}
