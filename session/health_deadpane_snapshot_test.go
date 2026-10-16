package session

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/domain"
)

// A dead pane is never nudged: whatever the detector last saw, the snapshot
// reads exited/crashed, and the decision table (even with nudge fully
// allowed) picks respawn for that state.
func TestLogDeadPaneSnapshot_should_neverReadAsNudgeable_When_PaneIsDead(t *testing.T) {
	statuses := []struct {
		name   string
		status detection.DetectedStatus
	}{
		{"idle", detection.StatusIdle},
		{"executing", detection.StatusExecuting},
		{"error", detection.StatusError},
		{"needs_approval", detection.StatusNeedsApproval},
		{"unknown", detection.StatusUnknown},
	}
	exits := []struct {
		name string
		code int
		want domain.ObservedState
	}{
		{"exit_0", 0, domain.ObservedExited},
		{"exit_1", 1, domain.ObservedCrashed},
	}
	for _, st := range statuses {
		for _, ex := range exits {
			t.Run(st.name+"/"+ex.name, func(t *testing.T) {
				src := &fakeStopSource{status: st.status, tmuxAlive: true, ctrlActive: true, tail: "last words"}
				got := logDeadPaneSnapshot("s", src, ex.code, "")
				assert.Equal(t, ex.want, got)
				action := domain.DecideStallAction(domain.StallDecisionInput{
					Reason: domain.StopReasonDeadPane, State: got,
					NudgeEnabled: true, NudgeAllowed: true, ItemInWorkStatus: true,
				})
				assert.Equal(t, domain.StallActionRespawn, action)
			})
		}
	}
}

func TestLogDeadPaneSnapshot_should_stillClassify_When_CaptureFails(t *testing.T) {
	src := &fakeStopSource{tailPanics: true, tmuxAlive: true, ctrlActive: true}
	assert.Equal(t, domain.ObservedCrashed, logDeadPaneSnapshot("s", src, 2, "SIGKILL"))
}

type recordedDeadPane struct {
	uuid, title string
	state       domain.ObservedState
	snap        domain.StopSnapshot
}

// Drives handleDeadPane itself: a crashed pane reaches the recorder once, as
// crashed with PaneDead forced, before the respawn attempt; a clean exit (no
// respawn) and a debounced first miss never reach it.
func TestHandleDeadPane_should_recordSnapshotBeforeRespawn_When_PaneCrashed(t *testing.T) {
	cases := []struct {
		name       string
		exitCode   int
		exitSignal string
		misses     int
		wantCalls  int
		wantState  domain.ObservedState
	}{
		{"crash_after_debounce", 1, "", 2, 1, domain.ObservedCrashed},
		{"signal_with_zero_code_reads_exited", 0, "SIGKILL", 2, 1, domain.ObservedExited},
		{"first_miss_is_debounced", 1, "", 1, 0, ""},
		{"clean_exit_is_not_a_respawn", 0, "", 2, 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls []recordedDeadPane
			h := NewSessionHealthChecker(nil, WithDeadPaneRecorder(
				func(uuid, title string, snap domain.StopSnapshot, state domain.ObservedState) {
					calls = append(calls, recordedDeadPane{uuid, title, state, snap})
				}))
			inst := makeTestInstance("dead-pane-" + tc.name)
			inst.UUID = "uuid-" + tc.name

			for i := 0; i < tc.misses; i++ {
				h.handleDeadPane(inst, tc.exitCode, tc.exitSignal, &HealthCheckResult{})
			}

			require.Len(t, calls, tc.wantCalls)
			if tc.wantCalls == 1 {
				assert.Equal(t, inst.UUID, calls[0].uuid)
				assert.Equal(t, tc.wantState, calls[0].state)
				assert.True(t, calls[0].snap.PaneDead)
				assert.Equal(t, domain.StallActionRespawn, domain.DecideStallAction(domain.StallDecisionInput{
					Reason: domain.StopReasonDeadPane, State: calls[0].state,
					NudgeEnabled: true, NudgeAllowed: true, ItemInWorkStatus: true,
				}))
			}
		})
	}
}

// Every detector status the pane could last have shown, through handleDeadPane
// itself: the recorder always sees exited/crashed (never a nudgeable state) and
// the decision is respawn, whatever the detector last saw.
func TestHandleDeadPane_should_neverBeNudgeable_When_AnyLastDetectedStatus(t *testing.T) {
	statuses := []detection.DetectedStatus{
		detection.StatusIdle, detection.StatusExecuting, detection.StatusError,
		detection.StatusNeedsApproval, detection.StatusUnknown,
	}
	for _, st := range statuses {
		for _, exit := range []int{1, 137} {
			t.Run(fmt.Sprintf("%v/exit_%d", st, exit), func(t *testing.T) {
				var got []domain.ObservedState
				h := NewSessionHealthChecker(nil, WithDeadPaneRecorder(
					func(_, _ string, snap domain.StopSnapshot, state domain.ObservedState) {
						got = append(got, state)
						assert.NotEmpty(t, snap.PaneTail, "BoundedStopTail-processed tail is carried to the recorder")
					}))
				h.stopSourceFor = func(*Instance) stopSource {
					return &fakeStopSource{status: st, tmuxAlive: true, ctrlActive: true, tail: "last words"}
				}
				inst := makeTestInstance("dead-any-status")
				for i := 0; i < failureThreshold; i++ {
					h.handleDeadPane(inst, exit, "", &HealthCheckResult{})
				}
				require.Equal(t, []domain.ObservedState{domain.ObservedCrashed}, got)
				assert.Equal(t, domain.StallActionRespawn, domain.DecideStallAction(domain.StallDecisionInput{
					Reason: domain.StopReasonDeadPane, State: got[0],
					NudgeEnabled: true, NudgeAllowed: true, ItemInWorkStatus: true,
				}))
			})
		}
	}
}
