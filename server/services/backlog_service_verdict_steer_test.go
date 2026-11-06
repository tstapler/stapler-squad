package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

const verdictSteerSessionUUID = "verdict-steer-work-uuid"

func newVerdictSteerFixture(t *testing.T, withWorkSession bool) (*BacklogService, *mockSessionSteerer, *events.EventBus, *session.BacklogItemData) {
	t.Helper()
	svc, steerer, bus := newTestBacklogServiceForSteerIntegration(t, verdictSteerSessionUUID, "claude")
	svc.verdictSteerPollInterval = 5 * time.Millisecond
	svc.verdictSteerReadyTimeout = 2 * time.Second
	t.Cleanup(svc.Shutdown)

	item, err := svc.storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:  "Awaiting review",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)
	if withWorkSession {
		_, err = svc.storage.CreateItemSession(context.Background(), session.ItemSessionData{
			ItemID:      item.ID,
			SessionUUID: verdictSteerSessionUUID,
			SessionRole: session.SessionRoleWork,
		})
		require.NoError(t, err)
	}
	return svc, steerer, bus, item
}

func publishVerdict(bus *events.EventBus, item *session.BacklogItemData, outcome session.ReviewOutcome, summary string) {
	bus.Publish(events.NewBacklogItemChangedEvent(&events.BacklogItemEventPayload{
		Kind:    events.BacklogChangeVerdictRecorded,
		Item:    item,
		Verdict: &session.ReviewVerdictData{OverallOutcome: outcome, Summary: summary},
	}))
}

// A verdict landing while the work session sits idle after request_review
// re-steers that session — no polling needed.
func TestVerdictSteering_VerdictRecorded_SteersIdleWorkSession(t *testing.T) {
	svc, steerer, bus, item := newVerdictSteerFixture(t, true)
	svc.StartVerdictSteering()

	publishVerdict(bus, item, session.ReviewOutcomePass, "all criteria met")

	require.Eventually(t, func() bool { return len(steerer.calls()) == 1 }, 3*time.Second, 10*time.Millisecond)
	call := steerer.calls()[0]
	assert.Equal(t, verdictSteerSessionUUID, call.uuid)
	assert.Contains(t, call.message, "PASS")
	assert.Contains(t, call.message, "all criteria met")
	assert.Contains(t, call.message, "/backlog/ship")
}

func TestVerdictSteering_FailVerdict_AsksForFixAndReReview(t *testing.T) {
	svc, steerer, bus, item := newVerdictSteerFixture(t, true)
	svc.StartVerdictSteering()

	publishVerdict(bus, item, session.ReviewOutcomeFail, "AC 2 not met")

	require.Eventually(t, func() bool { return len(steerer.calls()) == 1 }, 3*time.Second, 10*time.Millisecond)
	msg := steerer.calls()[0].message
	assert.Contains(t, msg, "FAIL")
	assert.Contains(t, msg, "request_review again")
}

// The session may still be finishing its turn when the verdict lands: wait for
// idle instead of dropping the steer.
func TestVerdictSteering_WaitsUntilSessionIdle(t *testing.T) {
	svc, steerer, bus, item := newVerdictSteerFixture(t, true)
	steerer.notReady = map[string]bool{verdictSteerSessionUUID: true}
	svc.StartVerdictSteering()

	publishVerdict(bus, item, session.ReviewOutcomePass, "ok")
	time.Sleep(50 * time.Millisecond) //nolint:notimesleeptest negative assertion: nothing is emitted on the steerer so there is no completion signal to await
	require.Empty(t, steerer.calls(), "must not write into a busy pane")

	steerer.mu.Lock()
	steerer.notReady[verdictSteerSessionUUID] = false
	steerer.mu.Unlock()
	require.Eventually(t, func() bool { return len(steerer.calls()) == 1 }, 3*time.Second, 10*time.Millisecond)
}

func TestVerdictSteering_NeverIdle_PublishesWarningInsteadOfSteering(t *testing.T) {
	svc, steerer, bus, item := newVerdictSteerFixture(t, true)
	svc.verdictSteerReadyTimeout = 50 * time.Millisecond
	steerer.notReady = map[string]bool{verdictSteerSessionUUID: true}
	ch, subID := bus.Subscribe(context.Background())
	defer bus.Unsubscribe(subID)
	svc.StartVerdictSteering()

	publishVerdict(bus, item, session.ReviewOutcomePass, "ok")

	require.Eventually(t, func() bool {
		select {
		case evt := <-ch:
			return evt.Type == events.EventNotification
		default:
			return false
		}
	}, 3*time.Second, 10*time.Millisecond)
	assert.Empty(t, steerer.calls())
}

func TestVerdictSteering_NoLiveWorkSession_NoSteer(t *testing.T) {
	svc, steerer, bus, item := newVerdictSteerFixture(t, false)
	svc.StartVerdictSteering()

	publishVerdict(bus, item, session.ReviewOutcomePass, "ok")
	time.Sleep(100 * time.Millisecond) //nolint:notimesleeptest negative assertion: nothing is emitted on the steerer so there is no completion signal to await
	assert.Empty(t, steerer.calls())
}

func TestVerdictSteering_IgnoresNonVerdictEvents(t *testing.T) {
	svc, steerer, bus, item := newVerdictSteerFixture(t, true)
	svc.StartVerdictSteering()

	bus.Publish(events.NewBacklogItemChangedEvent(&events.BacklogItemEventPayload{
		Kind: events.BacklogChangeStatusTransition, Item: item, NewStatus: "review",
	}))
	time.Sleep(100 * time.Millisecond) //nolint:notimesleeptest negative assertion: nothing is emitted on the steerer so there is no completion signal to await
	assert.Empty(t, steerer.calls())
}

// The same verdict event delivered twice must steer once.
func TestVerdictSteering_DuplicateEvent_SteersOnce(t *testing.T) {
	svc, steerer, bus, item := newVerdictSteerFixture(t, true)
	svc.StartVerdictSteering()

	publishVerdict(bus, item, session.ReviewOutcomePass, "ok")
	publishVerdict(bus, item, session.ReviewOutcomePass, "ok")

	require.Eventually(t, func() bool { return len(steerer.calls()) >= 1 }, 3*time.Second, 10*time.Millisecond)
	time.Sleep(100 * time.Millisecond) //nolint:notimesleeptest negative assertion: nothing is emitted on the steerer so there is no completion signal to await
	assert.Len(t, steerer.calls(), 1)
}

// A newer verdict landing while an older one waits for idle must replace it.
func TestVerdictSteering_NewerVerdictSupersedesWaitingOne(t *testing.T) {
	svc, steerer, bus, item := newVerdictSteerFixture(t, true)
	steerer.notReady = map[string]bool{verdictSteerSessionUUID: true}
	svc.StartVerdictSteering()

	publishVerdict(bus, item, session.ReviewOutcomeFail, "first")
	publishVerdict(bus, item, session.ReviewOutcomePass, "second")

	// Events are consumed asynchronously; release the session only once the
	// second verdict has superseded the first, or the first is legitimately
	// delivered when the session goes idle before the dispatcher sees event 2.
	require.Eventually(t, func() bool {
		latest, _ := svc.verdictSteerLatest.Load(item.ID)
		key, _ := latest.(string)
		return strings.Contains(key, "|"+string(session.ReviewOutcomePass)+"|")
	}, 3*time.Second, 5*time.Millisecond)

	steerer.mu.Lock()
	steerer.notReady[verdictSteerSessionUUID] = false
	steerer.mu.Unlock()
	require.Eventually(t, func() bool { return len(steerer.calls()) >= 1 }, 3*time.Second, 10*time.Millisecond)
	time.Sleep(100 * time.Millisecond) //nolint:notimesleeptest negative assertion: nothing is emitted on the steerer so there is no completion signal to await
	calls := steerer.calls()
	require.Len(t, calls, 1)
	assert.Contains(t, calls[0].message, "second")
}

// A verdict steer that times out waiting for the session to go idle must
// leave a durable StuckReasonVerdictUndelivered row behind, not just the
// one-shot notification — the notification alone ages out of
// NotificationHistoryStore's 7-day/500-row cap with no trace (confirmed
// live 2026-10-17 on backlog items 4666bc80/1cfae441/6c24cbab).
func TestVerdictSteering_NeverIdle_MarksVerdictUndeliveredStuck(t *testing.T) {
	svc, steerer, bus, item := newVerdictSteerFixture(t, true)
	svc.verdictSteerReadyTimeout = 50 * time.Millisecond
	steerer.notReady = map[string]bool{verdictSteerSessionUUID: true}
	svc.StartVerdictSteering()

	publishVerdict(bus, item, session.ReviewOutcomePass, "ok")

	require.Eventually(t, func() bool {
		return openStuckReasons(t, svc)[domain.StuckReasonVerdictUndelivered]
	}, 3*time.Second, 10*time.Millisecond)
	assert.Empty(t, steerer.calls())
}

// A steer that actually reaches SteerActiveSession but errors (e.g. the pane
// went away between the readiness check and the write) must also mark the
// row — not just the "never went idle" timeout path.
func TestVerdictSteering_DeliveryError_MarksVerdictUndeliveredStuck(t *testing.T) {
	svc, steerer, bus, item := newVerdictSteerFixture(t, true)
	steerer.mu.Lock()
	steerer.steerErr[verdictSteerSessionUUID] = errors.New("steer delivery boom")
	steerer.mu.Unlock()
	svc.StartVerdictSteering()

	publishVerdict(bus, item, session.ReviewOutcomePass, "ok")

	require.Eventually(t, func() bool {
		return openStuckReasons(t, svc)[domain.StuckReasonVerdictUndelivered]
	}, 3*time.Second, 10*time.Millisecond)
}

// A successful steer must resolve a previously-open StuckReasonVerdictUndelivered
// row for the item — the reconciler's "retry via a later bounce cycle" path and
// selfHealStuck's status-anchor backstop both depend on the point-of-success
// resolve firing so a since-recovered item doesn't sit flagged forever.
func TestVerdictSteering_SuccessfulSteer_ResolvesVerdictUndeliveredRow(t *testing.T) {
	svc, _, bus, item := newVerdictSteerFixture(t, true)
	_, err := svc.storage.MarkStuck(context.Background(), item.ID, domain.StuckReasonVerdictUndelivered, session.BacklogStatusReview, "a prior attempt never went idle")
	require.NoError(t, err)
	require.True(t, openStuckReasons(t, svc)[domain.StuckReasonVerdictUndelivered], "fixture setup: row must start open")

	svc.StartVerdictSteering()
	publishVerdict(bus, item, session.ReviewOutcomePass, "ok")

	require.Eventually(t, func() bool {
		return !openStuckReasons(t, svc)[domain.StuckReasonVerdictUndelivered]
	}, 3*time.Second, 10*time.Millisecond)
}

// Two consecutive failed steer attempts for the same item must leave exactly
// one open row, not two — MarkStuck's (item_id, reason) upsert is what
// guarantees this, but this test pins the behavior at the call-site level
// too, matching the spirit of the other dedup test above
// (TestVerdictSteering_DuplicateEvent_SteersOnce).
func TestVerdictSteering_RepeatedFailures_DoNotDuplicateStuckRow(t *testing.T) {
	svc, steerer, bus, item := newVerdictSteerFixture(t, true)
	svc.verdictSteerReadyTimeout = 50 * time.Millisecond
	steerer.notReady = map[string]bool{verdictSteerSessionUUID: true}
	svc.StartVerdictSteering()

	publishVerdict(bus, item, session.ReviewOutcomeFail, "first")
	require.Eventually(t, func() bool {
		return openStuckReasons(t, svc)[domain.StuckReasonVerdictUndelivered]
	}, 3*time.Second, 10*time.Millisecond)

	publishVerdict(bus, item, session.ReviewOutcomeFail, "second")
	require.Eventually(t, func() bool {
		open, err := svc.storage.FindOpenStuckStates(context.Background())
		require.NoError(t, err)
		count := 0
		for _, row := range open {
			if row.ItemID == item.ID && row.Reason == domain.StuckReasonVerdictUndelivered {
				count++
			}
		}
		return count == 1
	}, 3*time.Second, 10*time.Millisecond)
}
