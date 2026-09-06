package session

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// markStartedRecorder is a test LifecycleListener that records every
// EventStarted reason it observes, for asserting MarkStartedIfTmuxAlive
// fires (or doesn't fire) EventStarted as expected.
type markStartedRecorder struct {
	mu      sync.Mutex
	reasons []string
}

func (r *markStartedRecorder) OnLifecycleEvent(event LifecycleEvent, reason string) {
	if event != EventStarted {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reasons = append(r.reasons, reason)
}

func (r *markStartedRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.reasons)
}

// TestMarkStartedIfTmuxAlive_should_TransitionCreatingToActiveAndFireEvent_When_StartedIsFalse
// covers the production bug this method exists to fix: a boot-time restart
// (server/dependencies.go) left the instance stuck in Creating with
// Started()==false even though streamViaHub has already confirmed the
// underlying tmux session (and its PTY) are alive. Self-healing must bring
// Status to Active, flip started, and fire EventStarted so backlog
// lifecycle / notification listeners still see the start (BLOCKER findings
// #1 and #4 from the PR #693 review).
func TestMarkStartedIfTmuxAlive_should_TransitionCreatingToActiveAndFireEvent_When_StartedIsFalse(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test-mark-started-creating", Status: Creating}
	recorder := &markStartedRecorder{}
	inst.RegisterLifecycleListener(recorder)

	require.False(t, inst.Started())

	inst.MarkStartedIfTmuxAlive()

	assert.True(t, inst.Started(), "started must be flipped to true")
	assert.Equal(t, Active, inst.Status, "Status must be transitioned to Active alongside started, not left at Creating")
	assert.Equal(t, 1, recorder.count(), "EventStarted must fire exactly once so backlog lifecycle / notification listeners observe the self-healed start")
}

// TestMarkStartedIfTmuxAlive_should_OnlySetStarted_When_AlreadyActive verifies
// the Status==Active branch: only the started flag was missing, so no
// (redundant) transition is attempted, but EventStarted still fires since
// this is genuinely the first time Started() flips true for this run.
func TestMarkStartedIfTmuxAlive_should_OnlySetStarted_When_AlreadyActive(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test-mark-started-active", Status: Active}
	recorder := &markStartedRecorder{}
	inst.RegisterLifecycleListener(recorder)

	inst.MarkStartedIfTmuxAlive()

	assert.True(t, inst.Started())
	assert.Equal(t, Active, inst.Status)
	assert.Equal(t, 1, recorder.count())
}

// TestMarkStartedIfTmuxAlive_should_BeNoOp_When_AlreadyStarted verifies the
// method doesn't re-fire EventStarted or otherwise re-run its side effects
// once started is already true (e.g. a concurrent Start() already completed
// by the time this is invoked).
func TestMarkStartedIfTmuxAlive_should_BeNoOp_When_AlreadyStarted(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test-mark-started-noop", Status: Active}
	inst.started.Store(true)
	recorder := &markStartedRecorder{}
	inst.RegisterLifecycleListener(recorder)

	inst.MarkStartedIfTmuxAlive()

	assert.True(t, inst.Started())
	assert.Equal(t, Active, inst.Status)
	assert.Equal(t, 0, recorder.count(), "must not re-fire EventStarted when started was already true")
}

// TestMarkStartedIfTmuxAlive_should_NotOverride_When_StatusIsPausedStoppedOrPermanentlyFailed
// is BLOCKER finding #1's negative case: a deliberately-out-of-service
// instance (paused by the operator, terminally stopped, or permanently
// failed) must never be silently reported as started just because tmux
// happens to still be alive underneath it.
func TestMarkStartedIfTmuxAlive_should_NotOverride_When_StatusIsPausedStoppedOrPermanentlyFailed(t *testing.T) {
	t.Parallel()
	statuses := []Status{Paused, Stopped, PermanentlyFailed, Hibernated, Crashed, Failed}
	for _, status := range statuses {
		status := status
		t.Run(status.String(), func(t *testing.T) {
			t.Parallel()
			inst := &Instance{Title: "test-mark-started-" + status.String(), Status: status}
			recorder := &markStartedRecorder{}
			inst.RegisterLifecycleListener(recorder)

			inst.MarkStartedIfTmuxAlive()

			assert.False(t, inst.Started(), "started must remain false for Status=%s", status)
			assert.Equal(t, status, inst.Status, "Status must remain untouched for Status=%s", status)
			assert.Equal(t, 0, recorder.count(), "EventStarted must not fire for Status=%s", status)
		})
	}
}

// TestMarkStartedIfTmuxAlive_should_SerializeWithConcurrentStart_When_BothCalledTogether
// covers BLOCKER finding #2: MarkStartedIfTmuxAlive routes through the same
// actor mailbox as Start()/startLocked (runActor drains one command at a
// time), so the two can never interleave — this call either runs entirely
// before or entirely after a concurrent Start() on the same instance, never
// stomping on a Start() left mid-flight. Exercised through a real
// LiveInstance (NewLiveInstance), not a bare *Instance, since a bare
// *Instance's sendSyncErr runs fn synchronously with no mailbox at all —
// the very case that would let the two interleave.
func TestMarkStartedIfTmuxAlive_should_SerializeWithConcurrentStart_When_BothCalledTogether(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test that starts real tmux sessions")
	}
	checkTmuxAvailable(t)

	title := fmt.Sprintf("test-mark-started-race-%d", time.Now().UnixNano())
	inst, cleanup, err := NewInstanceWithCleanup(InstanceOptions{
		Title:            title,
		Path:             t.TempDir(),
		Program:          "sleep 300",
		SessionType:      SessionTypeDirectory,
		AutoYes:          false,
		TmuxPrefix:       fmt.Sprintf("test_markstarted_%d_", time.Now().UnixNano()),
		TmuxServerSocket: coldRestoreSocket(t),
	})
	require.NoError(t, err)
	defer func() {
		if cleanupErr := cleanup(); cleanupErr != nil {
			t.Logf("cleanup warning: %v", cleanupErr)
		}
	}()

	li := NewLiveInstance(inst)
	defer li.Stop()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = li.Start(true)
	}()
	go func() {
		defer wg.Done()
		li.MarkStartedIfTmuxAlive()
	}()
	wg.Wait()

	// Whichever order the actor mailbox happened to serialize the two
	// commands in, the end state must be a legal Started+Status pairing:
	// a started instance must land on Active, never a status the self-heal
	// method itself would have refused to produce, and never a torn
	// combination where one write only partially applied.
	require.True(t, li.Started(), "Start(true) must have completed to give this test something to serialize against")
	assert.Equal(t, Active, li.GetLifecycleStatus(), "a started instance must land on Active, not a torn or refused Status")
}
