package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTransitionTo_should_AllowCreatingToFailed_When_PipelineFails verifies the
// FSM edge added for Epic 1.1 Story 1.1.2: an instance in Creating can
// transition to Failed when the async creation pipeline (Epic 2.2) fails.
func TestTransitionTo_should_AllowCreatingToFailed_When_PipelineFails(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:  "test-creating-to-failed",
		Status: Creating,
	}

	err := inst.transitionTo(context.Background(), Failed)

	require.NoError(t, err, "Creating -> Failed must be a legal transition")
	assert.Equal(t, Failed, inst.Status)
}

// TestTransitionTo_should_RejectStoppedToFailed_When_StoppedIsTerminal verifies
// that Stopped stays terminal — no Stopped->Failed edge was added alongside the
// Creating<->Failed pair.
func TestTransitionTo_should_RejectStoppedToFailed_When_StoppedIsTerminal(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:  "test-stopped-to-failed",
		Status: Stopped,
	}

	err := inst.transitionTo(context.Background(), Failed)

	require.Error(t, err, "Stopped -> Failed must be rejected: Stopped stays terminal")
	assert.Equal(t, ErrInvalidTransition{From: Stopped, To: Failed}, err)
	assert.Equal(t, Stopped, inst.Status, "status must not change on a rejected transition")
}

// TestTransitionTo_should_AllowFailedToCreating_When_RetryRequested verifies the
// retry path: a Failed instance can transition back to Creating (Epic 1.2's
// TryStartRetry builds on this FSM edge).
func TestTransitionTo_should_AllowFailedToCreating_When_RetryRequested(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:  "test-failed-to-creating",
		Status: Failed,
	}

	err := inst.transitionTo(context.Background(), Creating)

	require.NoError(t, err, "Failed -> Creating must be a legal transition (retry path)")
	assert.Equal(t, Creating, inst.Status)
}

// TestTransitionTo_should_RejectFailedToActive_When_MustGoThroughCreatingFirst
// verifies Failed has exactly one outgoing edge (to Creating) — a retry cannot
// skip straight back to Active.
func TestTransitionTo_should_RejectFailedToActive_When_MustGoThroughCreatingFirst(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:  "test-failed-to-active",
		Status: Failed,
	}

	err := inst.transitionTo(context.Background(), Active)

	require.Error(t, err, "Failed -> Active must be rejected: retry must go through Creating first")
	assert.Equal(t, ErrInvalidTransition{From: Failed, To: Active}, err)
}

// TestFailureReason_should_RoundTrip_When_SetViaLockedHelper verifies the
// unexported failureReason field (Task 1.1.2c) round-trips through the
// setFailureReasonLocked/FailureReason() accessor pair. There is deliberately
// no public setter — setFailureReasonLocked is only reachable from within an
// actor command (Epic 1.2's TryForceStatusIfEpoch), exercised here directly via
// sendSyncErr the same way that closure will call it.
func TestFailureReason_should_RoundTrip_When_SetViaLockedHelper(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test-failure-reason", Status: Failed}

	assert.Empty(t, inst.FailureReason(), "failureReason must be empty before it is ever set")

	err := inst.sendSyncErr(func(s *instanceState) error {
		setFailureReasonLocked(s, "worktree creation failed: disk full")
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, "worktree creation failed: disk full", inst.FailureReason())
}

// TestStatusAndFailureReason_should_ReturnBothFieldsFromOneActorRoundTrip_When_Called
// verifies the accessor server/services.AwaitCreationTerminal relies on
// (async-session-creation Epic 2.3, Story 2.3.3) reads Status and
// FailureReason together rather than via two independent calls — pinned here
// by asserting both values come back correctly paired for a Failed instance,
// and empty/Creating for a fresh one.
func TestStatusAndFailureReason_should_ReturnBothFieldsFromOneActorRoundTrip_When_Called(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test-status-and-failure-reason", Status: Creating}

	status, reason := inst.StatusAndFailureReason()
	assert.Equal(t, Creating, status)
	assert.Empty(t, reason)

	err := inst.sendSyncErr(func(s *instanceState) error {
		s.inst.Status = Failed
		setFailureReasonLocked(s, "StartupError")
		return nil
	})
	require.NoError(t, err)

	status, reason = inst.StatusAndFailureReason()
	assert.Equal(t, Failed, status)
	assert.Equal(t, "StartupError", reason)
}

// TestSetCreationProgress_should_UpdateTimestamp_When_Called verifies Task
// 1.1.4's acceptance criterion: every SetCreationProgress call bumps
// CreationProgressUpdatedAt to that call's time, in the same actor command as
// the progress-text write (not a second mailbox round-trip).
func TestSetCreationProgress_should_UpdateTimestamp_When_Called(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "test-creation-progress-timestamp", Status: Creating}

	require.True(t, inst.CreationProgressUpdatedAt().IsZero(), "timestamp must be zero before the first call")

	before := time.Now()
	inst.SetCreationProgress("Cloning repository...")
	after := time.Now()

	got := inst.CreationProgressUpdatedAt()
	assert.False(t, got.Before(before), "timestamp must not be earlier than the call")
	assert.False(t, got.After(after), "timestamp must not be later than the call returned")
	assert.Equal(t, "Cloning repository...", inst.CreationProgress)

	// A second call bumps the timestamp again.
	firstTimestamp := got
	time.Sleep(time.Millisecond)
	inst.SetCreationProgress("Starting tmux session...")
	assert.True(t, inst.CreationProgressUpdatedAt().After(firstTimestamp), "second call must advance the timestamp")
}
