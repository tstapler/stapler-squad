package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

func newApplyStopFixture(t *testing.T) (*AutonomousOrchestrationService, *session.Instance, *session.LiveInstance, <-chan *events.Event) {
	t.Helper()
	bus := events.NewEventBus(100)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ch, _ := bus.Subscribe(ctx)
	inst := &session.Instance{Title: "apply-stop", UUID: "apply-stop-uuid", Status: session.Paused, Program: "claude"}
	li := session.NewLiveInstance(inst)
	t.Cleanup(li.Stop)
	inst.SetAutonomousMode(true, "")
	inst.SetAutonomousTurn(3, 20)
	return NewAutonomousOrchestrationService(nil, bus), inst, li, ch
}

func TestApplyInstanceStop_WireOutcomeSetClosed(t *testing.T) {
	t.Parallel()
	reasons := append([]domain.StopReason{""}, domain.AllStopReasons...)
	actions := []domain.StallAction{domain.StallActionRespawn, domain.StallActionNone, domain.StallActionNudge, domain.StallActionResupervise}
	for _, reason := range reasons {
		for _, action := range actions {
			t.Run(string(reason)+"/"+string(action), func(t *testing.T) {
				t.Parallel()
				svc, inst, _, ch := newApplyStopFixture(t)

				require.NoError(t, svc.applyInstanceStop(inst, action, session.AutonomousDriverOutcome{StopReason: reason}))

				var ev *events.Event
				select {
				case ev = <-ch:
				default:
					t.Fatal("no SessionUpdatedEvent published after a successful write")
				}
				assert.Equal(t, events.EventSessionUpdated, ev.Type)
				assert.Equal(t, []string{"autonomous_mode", "autonomous_outcome"}, ev.UpdatedFields)
				got := ev.Session.Snapshot().Autonomous
				assert.Contains(t, []string{"", "done", "stuck"}, got.AutonomousOutcome)
				assert.Equal(t, reason, got.AutonomousStopReason)
				assert.Zero(t, got.AutonomousTurn)
				inPlace := action == domain.StallActionNudge || action == domain.StallActionResupervise
				assert.Equal(t, inPlace, got.AutonomousMode, "only in-place recoveries keep autonomy on")
			})
		}
	}
}

func TestApplyInstanceStop_WriteFailed_NoEventAndWarns(t *testing.T) {
	// Not parallel: captureLogs swaps the shared slog seam.
	svc, inst, li, ch := newApplyStopFixture(t)
	li.Stop() // actor ctx done: the instance is going away
	buf := captureLogs(t)

	err := svc.applyInstanceStop(inst, domain.StallActionNone, session.AutonomousDriverOutcome{StopReason: domain.StopReasonOrchestratorError})

	require.ErrorIs(t, err, context.Canceled)
	select {
	case ev := <-ch:
		t.Fatalf("unexpected event after a failed write: %v", ev.Type)
	default:
	}
	assert.Contains(t, buf.String(), "level=WARN")
	assert.Contains(t, buf.String(), "stop_reason=orchestrator_error")
	assert.Contains(t, buf.String(), "session=apply-stop")
	assert.True(t, inst.Snapshot().Autonomous.AutonomousMode, "a failed write leaves the snapshot untouched")
}
