package services

// Cancelled-class stops (an operator's stop or a server shutdown) must never be
// handled as stalls, in any role arm or on the unlinked path (Story 3.1.3).

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

var cancelledReasons = []domain.StopReason{domain.StopReasonCancelledExternal, domain.StopReasonCancelledShutdown}

func cancelledOutcome(reason domain.StopReason) session.AutonomousDriverOutcome {
	return session.AutonomousDriverOutcome{StopReason: reason, Reason: "cancelled", Turns: 4}
}

func (f *charFixture) setReadoption(on bool) {
	f.svc.autonomousSvc.SetReadoptionEnabled(func() bool { return on })
}

// assertNoRowOrRecovery asserts the item has no open stuck row, its ItemSession is
// still open, the respawner was never called and no notification was published.
func (f *charFixture) assertNoRowOrRecovery(t *testing.T, respawner *signalingRespawner) {
	t.Helper()
	open, err := f.storage.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	assert.Empty(t, open, "a cancelled stop must not write an autonomous_stuck row")
	is, err := f.storage.GetItemSessionBySessionUUID(context.Background(), f.inst.UUID)
	require.NoError(t, err)
	assert.Nil(t, is.EndedAt, "UpdateItemSessionEnded must not run")
	assert.Empty(t, respawner.called)
	assert.Empty(t, notificationTitles(f.drain()))
}

// SVC-4. Only the observable silence is asserted; the nudger and driver starter
// seams do not exist yet, so "0 nudges / 0 driver starts" is covered by the
// absence of any effect (respawner, row, ItemSession end).
func TestOnAutonomousDriverComplete_CancelledClassGuards(t *testing.T) {
	roles := []struct {
		name       string
		role       string
		status     session.BacklogStatus
		wantStatus session.BacklogStatus
	}{
		{"work", session.SessionRoleWork, session.BacklogStatusInProgress, session.BacklogStatusInProgress},
		{"triage", session.SessionRoleTriage, session.BacklogStatusIdea, session.BacklogStatusIdea},
		{"review", session.SessionRoleReview, session.BacklogStatusReview, session.BacklogStatusReview},
		{"unrecognized", "future-pipeline-stage", session.BacklogStatusIdea, session.BacklogStatusIdea},
		{"unlinked", "", "", ""},
	}
	for _, r := range roles {
		for _, reason := range cancelledReasons {
			for _, flagOn := range []bool{false, true} {
				name := r.name + "/" + string(reason)
				if flagOn {
					name += "/readoption_on"
				} else {
					name += "/readoption_off"
				}
				t.Run(name, func(t *testing.T) {
					title := "cancel-guard-" + r.name + "-" + string(reason)
					if flagOn {
						title += "-on"
					}
					var f *charFixture
					if r.role == "" {
						f = newUnlinkedCharFixture(t, title)
					} else {
						f = newCharFixture(t, title, r.role, r.status)
					}
					f.setReadoption(flagOn)
					respawner := newSignalingRespawner()
					f.svc.SetAutonomousStuckRespawner(respawner)

					f.complete(cancelledOutcome(reason))

					snap := f.inst.Snapshot().Autonomous
					assert.Equal(t, reason, snap.AutonomousStopReason)
					switch {
					case reason == domain.StopReasonCancelledExternal:
						assert.Equal(t, "", snap.AutonomousOutcome)
						assert.False(t, snap.AutonomousMode)
					case flagOn:
						assert.True(t, snap.AutonomousMode, "shutdown keeps persisted mode as the re-adoption signal")
					default:
						assert.False(t, snap.AutonomousMode)
						assert.Equal(t, "stuck", snap.AutonomousOutcome)
					}
					if r.role == "" {
						assert.Empty(t, notificationTitles(f.drain()))
						return
					}
					f.assertNoRowOrRecovery(t, respawner)
					item, err := f.storage.GetBacklogItem(context.Background(), f.itemID)
					require.NoError(t, err)
					assert.Equal(t, string(r.wantStatus), item.Status, "no status transition (triage idea -> ready included)")
				})
			}
		}
	}
}

// SVC-5.
func TestOnAutonomousDriverComplete_CancelledExternal_PreexistingOpenRowStaysOpen(t *testing.T) {
	f := newCharFixture(t, "cancel-open-row", session.SessionRoleWork, session.BacklogStatusInProgress)
	respawner := newSignalingRespawner()
	f.svc.SetAutonomousStuckRespawner(respawner)
	f.seedAutonomousStuckRow(t, 2, nil)

	f.complete(cancelledOutcome(domain.StopReasonCancelledExternal))

	row := f.openStuckRow(t)
	assert.Equal(t, domain.StuckReasonAutonomousStuck, row.Reason)
	assert.Equal(t, int32(2), row.RemediationAttempts)
	assert.Empty(t, respawner.called)
	is, err := f.storage.GetItemSessionBySessionUUID(context.Background(), f.inst.UUID)
	require.NoError(t, err)
	assert.Nil(t, is.EndedAt)
}

// SVC-6.
func TestOnAutonomousDriverComplete_ReviewRole_CancelledAfterItemLeftReview_ResolvesRow(t *testing.T) {
	f := newCharFixture(t, "cancel-review-left", session.SessionRoleReview, session.BacklogStatusInProgress)
	f.seedAutonomousStuckRow(t, 0, nil)

	f.complete(cancelledOutcome(domain.StopReasonCancelledExternal))

	open, err := f.storage.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	assert.Empty(t, open, "the existing status != review case resolves the row")
	is, err := f.storage.GetItemSessionBySessionUUID(context.Background(), f.inst.UUID)
	require.NoError(t, err)
	assert.Nil(t, is.EndedAt, "UpdateItemSessionEnded must not run for a cancelled review")
}

// SVC-7.
func TestCancelledShutdown_FlagOn_KeepsModeAndRoundTrips(t *testing.T) {
	f := newCharFixture(t, "cancel-shutdown-on", session.SessionRoleWork, session.BacklogStatusInProgress)
	f.setReadoption(true)
	respawner := newSignalingRespawner()
	f.svc.SetAutonomousStuckRespawner(respawner)

	f.complete(cancelledOutcome(domain.StopReasonCancelledShutdown))

	snap := f.inst.Snapshot().Autonomous
	assert.True(t, snap.AutonomousMode)
	assert.Equal(t, domain.StopReasonCancelledShutdown, snap.AutonomousStopReason)
	f.assertNoRowOrRecovery(t, respawner)

	data := f.inst.ToInstanceData()
	assert.True(t, data.AutonomousMode)
	restored, err := session.FromInstanceDataDeferred(data)
	require.NoError(t, err)
	assert.True(t, restored.Snapshot().Autonomous.AutonomousMode)
}

// SVC-8.
func TestCancelledShutdown_FlagOff_ClearsModeOutcomeStuckOnly(t *testing.T) {
	f := newCharFixture(t, "cancel-shutdown-off", session.SessionRoleWork, session.BacklogStatusInProgress)
	respawner := newSignalingRespawner()
	f.svc.SetAutonomousStuckRespawner(respawner)

	f.complete(cancelledOutcome(domain.StopReasonCancelledShutdown))

	snap := f.inst.Snapshot().Autonomous
	assert.False(t, snap.AutonomousMode)
	assert.Equal(t, "stuck", snap.AutonomousOutcome)
	assert.Equal(t, domain.StopReasonCancelledShutdown, snap.AutonomousStopReason)
	f.assertNoRowOrRecovery(t, respawner)
}

// SVC-9: the final notification switch is three-way; the update event still fires.
func TestOnAutonomousDriverComplete_UnlinkedCancelled_NoFinalNotification(t *testing.T) {
	f := newUnlinkedCharFixture(t, "cancel-unlinked")

	f.complete(cancelledOutcome(domain.StopReasonCancelledExternal))

	evs := f.drain()
	assert.Empty(t, notificationTitles(evs))
	var updated bool
	for _, ev := range evs {
		updated = updated || ev.Type == events.EventSessionUpdated
	}
	assert.True(t, updated, "the badge update event still fires")
}

// SVC-10: an unlinked stall keeps today's handling.
func TestOnAutonomousDriverComplete_StallOutcomeUnlinked_TodayHandling(t *testing.T) {
	f := newUnlinkedCharFixture(t, "stall-unlinked")

	f.complete(session.AutonomousDriverOutcome{StopReason: domain.StopReasonOrchestratorError, Turns: 2})

	snap := f.inst.Snapshot().Autonomous
	assert.False(t, snap.AutonomousMode)
	assert.Equal(t, "stuck", snap.AutonomousOutcome)
	assert.Equal(t, []string{"Autonomous fix stuck"}, notificationTitles(f.drain()))
}
