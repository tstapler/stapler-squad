package session

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/internal/sessiontest"

	"github.com/stretchr/testify/require"
)

// newFakeLease returns a held lease bound to sessiontest.FakeLeaseOwner, taken from a
// throwaway instance so each call owns an independent lease.
func newFakeLease(t *testing.T) *HeldLease {
	t.Helper()
	inst := &Instance{UUID: sessiontest.FakeLeaseOwner}
	l, ok := inst.TryTerminalWriteLease(LeaseWriterOther)
	require.True(t, ok)
	return l
}

// TestSubmitDriverContent_SendsContentAndEnterAsSeparateWrites is the direct
// regression test for BUG-031 at the consolidated call site: content and the
// submit keystroke must never be concatenated into a single SendKeys call —
// this is the exact pattern that made Claude Code's TUI fold the Enter into a
// paste block instead of submitting it for long content.
func TestSubmitDriverContent_SendsContentAndEnterAsSeparateWrites(t *testing.T) {
	t.Parallel()
	inst := sessiontest.NewFakePaneSubmitter()
	inst.Updates = []bool{false, false, true} // settle immediately, then confirm the submit

	const content = "some long driver-generated prompt text"
	// A wide maxWait: the fake answers at once, and a tight window made the confirm
	// poll time out under load, which triggers the Enter retry and a third write.
	if err := SubmitDriverContent(context.Background(), inst, newFakeLease(t), content, time.Millisecond, 5*time.Second); err != nil {
		t.Fatalf("SubmitDriverContent returned unexpected error: %v", err)
	}

	if len(inst.SendCalls) != 2 {
		t.Fatalf("SendKeys called %d times, want exactly 2 (content, then Enter) — got %#v", len(inst.SendCalls), inst.SendCalls)
	}
	if inst.SendCalls[0] != content {
		t.Errorf("first SendKeys call = %q, want exactly the content with no suffix", inst.SendCalls[0])
	}
	if inst.SendCalls[1] != EnterKeySequence {
		t.Errorf("second SendKeys call = %q, want exactly EnterKeySequence sent on its own", inst.SendCalls[1])
	}
}

// TestSubmitDriverContent_ContentSendFailure_NeverSendsEnter asserts a failed
// content write short-circuits before Enter is attempted.
func TestSubmitDriverContent_ContentSendFailure_NeverSendsEnter(t *testing.T) {
	t.Parallel()
	inst := sessiontest.NewFakePaneSubmitter()
	inst.FailOnCall = 0

	err := SubmitDriverContent(context.Background(), inst, newFakeLease(t), "content", time.Millisecond, 20*time.Millisecond)
	if err == nil {
		t.Fatal("expected an error from the failed content send")
	}
	if len(inst.SendCalls) != 1 {
		t.Errorf("SendKeys called %d times, want exactly 1 (Enter must not be sent after a content failure) — got %#v", len(inst.SendCalls), inst.SendCalls)
	}
}

// TestSubmitDriverContent_SubmitKeystrokeFailure_ReportsError asserts a failed
// Enter write (after a successful content write) still surfaces as an error.
func TestSubmitDriverContent_SubmitKeystrokeFailure_ReportsError(t *testing.T) {
	t.Parallel()
	inst := sessiontest.NewFakePaneSubmitter()
	inst.FailOnCall = 1

	err := SubmitDriverContent(context.Background(), inst, newFakeLease(t), "content", time.Millisecond, 20*time.Millisecond)
	if err == nil {
		t.Fatal("expected an error from the failed submit keystroke")
	}
	if len(inst.SendCalls) != 2 {
		t.Errorf("SendKeys called %d times, want exactly 2 (content succeeded, Enter attempted and failed) — got %#v", len(inst.SendCalls), inst.SendCalls)
	}
}

// TestSubmitDriverContent_SwallowedSubmit_RetriesOnceThenReturnsErrSubmitNotConfirmed
// is the direct regression test for AC4/AC5: a submit that never shows up as
// a pane change (the paste-detector-swallowed-it case) must not report
// success — it must retry the Enter keystroke once, and if that also shows no
// change, return ErrSubmitNotConfirmed rather than nil.
func TestSubmitDriverContent_SwallowedSubmit_RetriesOnceThenReturnsErrSubmitNotConfirmed(t *testing.T) {
	t.Parallel()
	inst := sessiontest.NewFakePaneSubmitter()
	inst.Updates = []bool{false} // settles immediately (no changes), never confirms (repeats false forever)

	err := SubmitDriverContent(context.Background(), inst, newFakeLease(t), "content", time.Millisecond, 5*time.Millisecond)
	if !errors.Is(err, ErrSubmitNotConfirmed) {
		t.Fatalf("SubmitDriverContent error = %v, want ErrSubmitNotConfirmed", err)
	}
	if len(inst.SendCalls) != 3 {
		t.Fatalf("SendKeys called %d times, want exactly 3 (content, Enter, retry Enter) — got %#v", len(inst.SendCalls), inst.SendCalls)
	}
	if inst.SendCalls[1] != EnterKeySequence || inst.SendCalls[2] != EnterKeySequence {
		t.Errorf("expected both the second and third SendKeys calls to be EnterKeySequence, got %#v", inst.SendCalls)
	}
}

// retryConfirmFake is a sessiontest.FakePaneSubmitter whose pane only shows a change once
// the retry Enter (the 3rd SendKeys call) has been sent — deterministically
// exercising the "first confirmation attempt fails, retry succeeds" path
// without depending on timing/call-count coincidences.
type retryConfirmFake struct {
	*sessiontest.FakePaneSubmitter
}

func (f *retryConfirmFake) HasUpdated() (bool, bool) {
	return len(f.SendCalls) >= 3, false
}

// TestSubmitDriverContent_ConfirmedOnRetry_Succeeds asserts that when the
// first Enter appears swallowed but the retry Enter is confirmed, the overall
// call succeeds rather than returning ErrSubmitNotConfirmed.
func TestSubmitDriverContent_ConfirmedOnRetry_Succeeds(t *testing.T) {
	t.Parallel()
	inst := &retryConfirmFake{FakePaneSubmitter: sessiontest.NewFakePaneSubmitter()}

	err := SubmitDriverContent(context.Background(), inst, newFakeLease(t), "content", time.Millisecond, 5*time.Millisecond)
	if err != nil {
		t.Fatalf("SubmitDriverContent returned unexpected error: %v", err)
	}
	if len(inst.SendCalls) != 3 {
		t.Fatalf("SendKeys called %d times, want exactly 3 (content, Enter, retry Enter) — got %#v", len(inst.SendCalls), inst.SendCalls)
	}
}

// TestSubmitDriverContent_ContextAlreadyCancelled_NeverSendsKeys is the direct
// regression test for the abandoned-goroutine bug: a ctx that's already
// cancelled/expired before SubmitDriverContent is even called must short
// circuit immediately, without writing any keys to the pane at all — the
// old behavior fired the content SendKeys, the Enter SendKeys, and (if
// unconfirmed) the retry Enter SendKeys unconditionally regardless of ctx
// state, so a caller's wrapper (submitContentWithEnter) timing out could
// still leave a real Enter keystroke landing in the pane afterward.
func TestSubmitDriverContent_ContextAlreadyCancelled_NeverSendsKeys(t *testing.T) {
	t.Parallel()
	inst := sessiontest.NewFakePaneSubmitter()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := SubmitDriverContent(ctx, inst, newFakeLease(t), "content", time.Millisecond, 20*time.Millisecond)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SubmitDriverContent error = %v, want wrapping context.Canceled", err)
	}
	if len(inst.SendCalls) != 0 {
		t.Fatalf("SendKeys called %d times, want 0 — a cancelled context must stop before any write, got %#v", len(inst.SendCalls), inst.SendCalls)
	}
}

// TestSubmitDriverContent_ContextExpiresDuringSettle_StopsBeforeEnter asserts
// that a context which expires during the settle wait (between the content
// write and the Enter write) prevents the Enter write from firing — not just
// a ctx that's already dead before the call starts.
func TestSubmitDriverContent_ContextExpiresDuringSettle_StopsBeforeEnter(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inst := sessiontest.NewFakePaneSubmitter()
	inst.Updates = []bool{false} // never updates
	inst.OnSend = func(calls int) {
		if calls == 1 {
			cancel()
		}
	}

	// pollInterval is longer than ctx's deadline, so waitForPaneSettle's
	// internal select hits <-ctx.Done() before its first <-time.After(poll)
	// tick — exercising "context expires mid-wait," not "settle finishes
	// quickly on its own."
	err := SubmitDriverContent(ctx, inst, newFakeLease(t), "content", 50*time.Millisecond, 200*time.Millisecond)
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("SubmitDriverContent error = %v, want context cancellation", err)
	}
	if len(inst.SendCalls) != 1 {
		t.Fatalf("SendKeys called %d times, want exactly 1 (content only — ctx expired before Enter) — got %#v", len(inst.SendCalls), inst.SendCalls)
	}
}

// blockingKeySender's SendKeys blocks until unblock is closed, simulating a
// wedged PTY write — the scenario SendKeysWithTimeout exists to bound.
type blockingKeySender struct {
	unblock chan struct{}
}

func (b *blockingKeySender) LeaseOwnerUUID() string { return sessiontest.FakeLeaseOwner }

func (b *blockingKeySender) SendKeys(_ string) error {
	<-b.unblock
	return nil
}

// TestSendKeysWithTimeout_WedgedWrite_ReturnsDeadlineExceeded proves a wedged
// PTY write bounds out instead of hanging the caller forever.
func TestSendKeysWithTimeout_WedgedWrite_ReturnsDeadlineExceeded(t *testing.T) {
	t.Parallel()
	inst := &blockingKeySender{unblock: make(chan struct{})}
	defer close(inst.unblock) // let the leaked goroutine's SendKeys return

	err := SendKeysWithTimeout(context.Background(), inst, newFakeLease(t), "input", 20*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("SendKeysWithTimeout error = %v, want context.DeadlineExceeded", err)
	}
}

// TestSendKeysWithTimeout_NormalWrite_ReturnsUnderlyingError proves the
// non-timeout path still surfaces SendKeys' own error rather than masking it.
func TestSendKeysWithTimeout_NormalWrite_ReturnsUnderlyingError(t *testing.T) {
	t.Parallel()
	inst := sessiontest.NewFakePaneSubmitter()
	inst.FailOnCall = 0

	err := SendKeysWithTimeout(context.Background(), inst, newFakeLease(t), "input", time.Second)
	if err == nil || err.Error() != "fake send failure" {
		t.Fatalf("SendKeysWithTimeout error = %v, want the underlying SendKeys failure", err)
	}
}

// TestSubmitContentWithEnter_DelegatesToSubmitDriverContent proves the shared
// wrapper still sends content and Enter as two separate writes.
func TestSubmitContentWithEnter_DelegatesToSubmitDriverContent(t *testing.T) {
	t.Parallel()
	inst := sessiontest.NewFakePaneSubmitter()

	if err := SubmitContentWithEnter(context.Background(), inst, newFakeLease(t), "hello"); err != nil {
		t.Fatalf("SubmitContentWithEnter error = %v, want nil", err)
	}
	if len(inst.SendCalls) != 2 || inst.SendCalls[0] != "hello" || inst.SendCalls[1] != EnterKeySequence {
		t.Fatalf("sendCalls = %#v, want [\"hello\", EnterKeySequence]", inst.SendCalls)
	}
}

// TestSubmitDriverContent_ContextExpiresBeforeRetry_StopsBeforeRetryEnter
// covers the one ctx.Err() guard in SubmitDriverContent without direct
// coverage elsewhere: ctx expiring after the first confirmation poll finds no
// change, but before the retry Enter would be sent — sibling to
// TestSubmitDriverContent_ContextExpiresDuringSettle_StopsBeforeEnter.
func TestSubmitDriverContent_ContextExpiresBeforeRetry_StopsBeforeRetryEnter(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inst := sessiontest.NewFakePaneSubmitter()
	inst.Updates = []bool{false} // settles immediately; confirmation poll never sees a change
	inst.OnSend = func(calls int) {
		if calls == 2 {
			cancel()
		}
	}

	err := SubmitDriverContent(ctx, inst, newFakeLease(t), "content", time.Millisecond, 50*time.Millisecond)
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("SubmitDriverContent error = %v, want context cancellation", err)
	}
	if len(inst.SendCalls) != 2 {
		t.Fatalf("SendKeys called %d times, want exactly 2 (content, first Enter — no retry) — got %#v", len(inst.SendCalls), inst.SendCalls)
	}
}

type ownerCheckedSubmitter struct {
	*sessiontest.FakePaneSubmitter
	ownerErr error
}

func (o *ownerCheckedSubmitter) VerifyPaneOwner(context.Context) error { return o.ownerErr }

// ce71ad1a: a pane whose owner can't be confirmed must receive no bytes.
func TestSubmitDriverContent_OwnerMismatch_WritesNothing(t *testing.T) {
	sub := &ownerCheckedSubmitter{FakePaneSubmitter: sessiontest.NewFakePaneSubmitter(), ownerErr: errors.New("pane ownership mismatch")}

	err := SubmitDriverContent(context.Background(), sub, newFakeLease(t), "review prompt", time.Millisecond, time.Millisecond)

	require.ErrorContains(t, err, "pane owner not verified")
	require.Empty(t, sub.SendCalls, "no content or Enter may be written to a pane with a mismatched owner")
}

func TestSubmitDriverContent_OwnerVerified_Writes(t *testing.T) {
	sub := &ownerCheckedSubmitter{FakePaneSubmitter: sessiontest.NewFakePaneSubmitter()}

	require.NoError(t, SubmitDriverContent(context.Background(), sub, newFakeLease(t), "review prompt", time.Millisecond, time.Millisecond))
	require.Equal(t, "review prompt", sub.SendCalls[0])
}

// T-WL-03: the lease stays held until the abandoned writer goroutine's write
// returns, even though the caller already got DeadlineExceeded.
func TestSubmitContentWithEnterAndSendKeysWithTimeout_ShouldKeepTheLeaseUntilTheAbandonedWriterGoroutineExits_WhenTheCallerTimesOut(t *testing.T) {
	t.Parallel()

	t.Run("SendKeysWithTimeout", func(t *testing.T) {
		t.Parallel()
		inst := leaseInstance(t, sessiontest.FakeLeaseOwner)
		lease, ok := inst.TryTerminalWriteLease(LeaseWriterMCP)
		require.True(t, ok)
		pane := &blockingKeySender{unblock: make(chan struct{})}

		err := SendKeysWithTimeout(context.Background(), pane, lease, "x", 20*time.Millisecond)
		require.ErrorIs(t, err, context.DeadlineExceeded)

		_, ok = inst.TryTerminalWriteLease(LeaseWriterSteer)
		require.False(t, ok, "the abandoned write still holds the lease")

		close(pane.unblock)
		waitLeaseFree(t, inst)
	})

	t.Run("SubmitContentWithEnter", func(t *testing.T) {
		t.Parallel()
		inst := leaseInstance(t, sessiontest.FakeLeaseOwner)
		lease, ok := inst.TryTerminalWriteLease(LeaseWriterMCP)
		require.True(t, ok)

		unblock := make(chan struct{})
		entered := make(chan struct{})
		pane := sessiontest.NewFakePaneSubmitter()
		var once sync.Once
		pane.OnSend = func(int) {
			once.Do(func() { close(entered) })
			<-unblock
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- SubmitContentWithEnter(ctx, pane, lease, "x") }()

		<-entered
		cancel()
		require.ErrorIs(t, <-done, context.DeadlineExceeded)

		_, ok = inst.TryTerminalWriteLease(LeaseWriterSteer)
		require.False(t, ok, "the abandoned write still holds the lease")

		close(unblock)
		waitLeaseFree(t, inst)
	})
}

// stolenUpdateFake never reports HasUpdated (a concurrent poller consumed the
// diff) but its pane content changes once Enter is sent.
type stolenUpdateFake struct {
	*sessiontest.FakePaneSubmitter
}

func (f *stolenUpdateFake) HasUpdated() (bool, bool) { return false, false }

func (f *stolenUpdateFake) CapturePaneContent() (string, error) {
	if len(f.SendCalls) >= 2 {
		return "prompt cleared, agent working", nil
	}
	return "prompt with typed text", nil
}

// TestSubmitDriverContent_ConfirmsViaOwnSnapshot_WhenHasUpdatedStolen: a
// delivered message must not be reported ErrSubmitNotConfirmed just because a
// shared HasUpdated consumer ate the change.
func TestSubmitDriverContent_ConfirmsViaOwnSnapshot_WhenHasUpdatedStolen(t *testing.T) {
	t.Parallel()
	inst := &stolenUpdateFake{FakePaneSubmitter: sessiontest.NewFakePaneSubmitter()}

	err := SubmitDriverContent(context.Background(), inst, newFakeLease(t), "content", time.Millisecond, 20*time.Millisecond)
	require.NoError(t, err)
	require.Len(t, inst.SendCalls, 2, "no retry Enter once the pane visibly changed")
}
