package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

// newAwaitTestInstance builds a *session.Instance (no tmux/Start()), wraps it
// in a session.LiveInstance (mirroring CreateManagedInstance's real
// construction order — session/create_managed_instance.go registers the live
// actor before the instance is ever exposed to a second goroutine) so
// sendSyncErr-routed methods (TryForceStatusIfEpoch, TryStartRetry,
// StatusAndFailureReason) are actually serialized through the actor mailbox
// instead of falling back to the unlocked direct-call path sendSyncErr uses
// when no live actor is registered, and registers it with svc's live poller
// under title so FindLiveInstance(title) resolves it. Status starts at
// session.Creating (the zero value).
func newAwaitTestInstance(t *testing.T, svc *SessionService, title string) *session.Instance {
	t.Helper()
	inst, err := session.NewInstance(session.InstanceOptions{
		Title: title,
		Path:  t.TempDir(),
	})
	require.NoError(t, err)
	live := session.NewLiveInstance(inst)
	t.Cleanup(live.Stop)
	svc.reviewQueuePoller.AddInstance(inst)
	return inst
}

// Story 2.3.3: AwaitCreationTerminal — the bounded wait primitive.

// TestAwaitCreationTerminal_should_ReturnActiveOutcome_When_InstanceTransitionsBeforeTimeout
// pins the success path: an instance that transitions to Active partway
// through a much-longer timeout is observed within roughly one poll interval
// of the transition, not at the full timeout.
func TestAwaitCreationTerminal_should_ReturnActiveOutcome_When_InstanceTransitionsBeforeTimeout(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)
	inst := newAwaitTestInstance(t, svc, "await-active")
	epoch := inst.CreationEpoch()

	go func() {
		time.Sleep(30 * time.Millisecond)
		applied := inst.TryForceStatusIfEpoch(epoch, session.Active, "")
		assert.True(t, applied)
	}()

	start := time.Now()
	outcome, err := svc.awaitCreationTerminal(context.Background(), "await-active", 5*time.Second, 10*time.Millisecond)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, session.Active, outcome.Status)
	assert.Empty(t, outcome.FailureReason)
	assert.Equal(t, "await-active", outcome.InstanceID)
	assert.Less(t, elapsed, 2*time.Second, "should return shortly after the transition, not wait out the full timeout")
}

// TestAwaitCreationTerminal_should_ReturnFailedOutcome_When_InstanceIsAlreadyFailed
// covers the terminal-Failed branch, and that FailureReason travels with it.
func TestAwaitCreationTerminal_should_ReturnFailedOutcome_When_InstanceIsAlreadyFailed(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)
	inst := newAwaitTestInstance(t, svc, "await-failed")
	epoch := inst.CreationEpoch()
	require.True(t, inst.TryForceStatusIfEpoch(epoch, session.Failed, "GitHubResolutionError"))

	outcome, err := svc.awaitCreationTerminal(context.Background(), "await-failed", time.Second, 10*time.Millisecond)

	require.NoError(t, err)
	assert.Equal(t, session.Failed, outcome.Status)
	assert.Equal(t, "GitHubResolutionError", outcome.FailureReason)
	assert.Equal(t, "await-failed", outcome.InstanceID)
}

// TestAwaitCreationTerminal_should_ReturnErrCreationAwaitTimeout_When_InstanceNeverLeavesCreating
// covers the timeout branch: an instance that stays Creating for the whole
// window times out rather than hanging indefinitely.
func TestAwaitCreationTerminal_should_ReturnErrCreationAwaitTimeout_When_InstanceNeverLeavesCreating(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)
	newAwaitTestInstance(t, svc, "await-stuck")

	outcome, err := svc.awaitCreationTerminal(context.Background(), "await-stuck", 60*time.Millisecond, 10*time.Millisecond)

	require.ErrorIs(t, err, ErrCreationAwaitTimeout)
	assert.Equal(t, CreationOutcome{}, outcome)
}

// TestAwaitCreationTerminal_should_ReturnErrCreationVanished_When_InstanceIsRemovedMidWait
// covers the vanished branch: an instance observed alive that then disappears
// (mirroring what a concurrent CancelSessionCreation does — remove the row
// rather than write a terminal status) is reported distinctly from a timeout,
// and immediately rather than waiting out the full timeout.
func TestAwaitCreationTerminal_should_ReturnErrCreationVanished_When_InstanceIsRemovedMidWait(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)
	newAwaitTestInstance(t, svc, "await-vanish")

	go func() {
		time.Sleep(30 * time.Millisecond)
		svc.reviewQueuePoller.RemoveInstance("await-vanish")
	}()

	start := time.Now()
	outcome, err := svc.awaitCreationTerminal(context.Background(), "await-vanish", 5*time.Second, 10*time.Millisecond)
	elapsed := time.Since(start)

	require.ErrorIs(t, err, ErrCreationVanished)
	assert.Equal(t, CreationOutcome{}, outcome)
	assert.Less(t, elapsed, 2*time.Second, "vanish should be observed promptly, not at the full timeout")
}

// TestAwaitCreationTerminal_should_ReturnCtxErr_When_CallerContextEndsFirst
// covers the fourth exit: the caller's own ctx ending before a terminal
// status, a timeout, or a vanish is observed — returned as-is (ctx.Err()),
// never wrapped in one of this package's own sentinels.
func TestAwaitCreationTerminal_should_ReturnCtxErr_When_CallerContextEndsFirst(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)
	newAwaitTestInstance(t, svc, "await-ctx-done")

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	outcome, err := svc.awaitCreationTerminal(ctx, "await-ctx-done", 5*time.Second, 10*time.Millisecond)

	require.ErrorIs(t, err, context.Canceled)
	assert.NotErrorIs(t, err, ErrCreationAwaitTimeout)
	assert.NotErrorIs(t, err, ErrCreationVanished)
	assert.Equal(t, CreationOutcome{}, outcome)
}

// TestAwaitCreationTerminal_should_ReturnConsistentSnapshot_When_StatusAndFailureReasonAreReadTogether
// is the regression test for the non-atomic-reader race CreationOutcome
// exists to close (see the Domain Glossary's CreationOutcome entry): the
// returned Status/FailureReason pair must always be one the instance actually
// held simultaneously, never a torn combination assembled from two separate
// reads taken at different times. This exercises the underlying
// session.Instance.StatusAndFailureReason accessor AwaitCreationTerminal
// relies on, under concurrent writers, and asserts every observation is one
// of the two legitimate states this scenario can be in.
func TestAwaitCreationTerminal_should_ReturnConsistentSnapshot_When_StatusAndFailureReasonAreReadTogether(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)
	inst := newAwaitTestInstance(t, svc, "await-consistent")
	epoch := inst.CreationEpoch()
	require.True(t, inst.TryForceStatusIfEpoch(epoch, session.Failed, "attempt-1"))

	done := make(chan struct{})
	go func() {
		defer close(done)
		// TryStartRetry resets Status to Creating without clearing the prior
		// attempt's FailureReason (only TryForceStatusIfEpoch's closure does
		// that) — so a torn two-read implementation could observe
		// Status=Creating alongside the stale "attempt-1" reason and treat it
		// as a bogus terminal outcome. Race this against concurrent reads.
		_, _ = inst.TryStartRetry()
	}()

	for i := 0; i < 200; i++ {
		status, reason := inst.StatusAndFailureReason()
		switch status {
		case session.Failed:
			assert.Equal(t, "attempt-1", reason, "a Failed read must carry its own attempt's reason, never a torn value")
		case session.Creating:
			// Legitimate post-retry state; FailureReason may still be the
			// stale "attempt-1" until the next terminal write clears it —
			// that's fine, since AwaitCreationTerminal never treats Creating
			// as terminal and so never surfaces this combination as an
			// outcome.
		default:
			t.Fatalf("unexpected status observed: %v", status)
		}
	}
	<-done
}
