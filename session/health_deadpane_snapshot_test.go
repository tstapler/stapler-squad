package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
