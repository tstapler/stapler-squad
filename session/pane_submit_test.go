package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/sendkeysguard"
)

// fakePaneSubmitter is a scripted paneSubmitter: it records every SendKeys
// argument in call order (so a test can assert content and Enter travelled as
// two separate writes, not one concatenated string) and can be told to fail
// on a specific call index. HasUpdated pops the next value off updates
// (repeating the last one once exhausted, mirroring fakePaneSettleChecker),
// so a test can script both the pre-Enter settle wait and the post-Enter
// submit-confirmation poll independently.
type fakePaneSubmitter struct {
	sendCalls   []string
	failOnCall  int // -1 (default) means never fail
	updates     []bool
	updateCalls int
}

func (f *fakePaneSubmitter) SendKeys(keys string) error {
	idx := len(f.sendCalls)
	f.sendCalls = append(f.sendCalls, keys)
	if f.failOnCall == idx {
		return errors.New("fake send failure")
	}
	return nil
}

func (f *fakePaneSubmitter) HasUpdated() (bool, bool) {
	if len(f.updates) == 0 {
		return false, false
	}
	idx := f.updateCalls
	f.updateCalls++
	if idx >= len(f.updates) {
		idx = len(f.updates) - 1
	}
	return f.updates[idx], false
}

// newFakePaneSubmitter returns a fake that reports a pane update on every
// HasUpdated call — i.e. content settles immediately and every submit is
// confirmed — unless the test overrides updates itself.
func newFakePaneSubmitter() *fakePaneSubmitter {
	return &fakePaneSubmitter{failOnCall: -1, updates: []bool{false, true}}
}

// TestSubmitDriverContent_SendsContentAndEnterAsSeparateWrites is the direct
// regression test for BUG-031 at the consolidated call site: content and the
// submit keystroke must never be concatenated into a single SendKeys call —
// this is the exact pattern that made Claude Code's TUI fold the Enter into a
// paste block instead of submitting it for long content.
func TestSubmitDriverContent_SendsContentAndEnterAsSeparateWrites(t *testing.T) {
	t.Parallel()
	inst := newFakePaneSubmitter()
	inst.updates = []bool{false, false, true} // settle immediately, then confirm the submit

	const content = "some long driver-generated prompt text"
	if err := SubmitDriverContent(context.Background(), inst, content, time.Millisecond, 20*time.Millisecond); err != nil {
		t.Fatalf("SubmitDriverContent returned unexpected error: %v", err)
	}

	if len(inst.sendCalls) != 2 {
		t.Fatalf("SendKeys called %d times, want exactly 2 (content, then Enter) — got %#v", len(inst.sendCalls), inst.sendCalls)
	}
	if inst.sendCalls[0] != content {
		t.Errorf("first SendKeys call = %q, want exactly the content with no suffix", inst.sendCalls[0])
	}
	if inst.sendCalls[1] != EnterKeySequence {
		t.Errorf("second SendKeys call = %q, want exactly EnterKeySequence sent on its own", inst.sendCalls[1])
	}
}

// TestSubmitDriverContent_ContentSendFailure_NeverSendsEnter asserts a failed
// content write short-circuits before Enter is attempted.
func TestSubmitDriverContent_ContentSendFailure_NeverSendsEnter(t *testing.T) {
	t.Parallel()
	inst := newFakePaneSubmitter()
	inst.failOnCall = 0

	err := SubmitDriverContent(context.Background(), inst, "content", time.Millisecond, 20*time.Millisecond)
	if err == nil {
		t.Fatal("expected an error from the failed content send")
	}
	if len(inst.sendCalls) != 1 {
		t.Errorf("SendKeys called %d times, want exactly 1 (Enter must not be sent after a content failure) — got %#v", len(inst.sendCalls), inst.sendCalls)
	}
}

// TestSubmitDriverContent_SubmitKeystrokeFailure_ReportsError asserts a failed
// Enter write (after a successful content write) still surfaces as an error.
func TestSubmitDriverContent_SubmitKeystrokeFailure_ReportsError(t *testing.T) {
	t.Parallel()
	inst := newFakePaneSubmitter()
	inst.failOnCall = 1

	err := SubmitDriverContent(context.Background(), inst, "content", time.Millisecond, 20*time.Millisecond)
	if err == nil {
		t.Fatal("expected an error from the failed submit keystroke")
	}
	if len(inst.sendCalls) != 2 {
		t.Errorf("SendKeys called %d times, want exactly 2 (content succeeded, Enter attempted and failed) — got %#v", len(inst.sendCalls), inst.sendCalls)
	}
}

// TestSubmitDriverContent_SwallowedSubmit_RetriesOnceThenReturnsErrSubmitNotConfirmed
// is the direct regression test for AC4/AC5: a submit that never shows up as
// a pane change (the paste-detector-swallowed-it case) must not report
// success — it must retry the Enter keystroke once, and if that also shows no
// change, return ErrSubmitNotConfirmed rather than nil.
func TestSubmitDriverContent_SwallowedSubmit_RetriesOnceThenReturnsErrSubmitNotConfirmed(t *testing.T) {
	t.Parallel()
	inst := newFakePaneSubmitter()
	inst.updates = []bool{false} // settles immediately (no changes), never confirms (repeats false forever)

	err := SubmitDriverContent(context.Background(), inst, "content", time.Millisecond, 5*time.Millisecond)
	if !errors.Is(err, ErrSubmitNotConfirmed) {
		t.Fatalf("SubmitDriverContent error = %v, want ErrSubmitNotConfirmed", err)
	}
	if len(inst.sendCalls) != 3 {
		t.Fatalf("SendKeys called %d times, want exactly 3 (content, Enter, retry Enter) — got %#v", len(inst.sendCalls), inst.sendCalls)
	}
	if inst.sendCalls[1] != EnterKeySequence || inst.sendCalls[2] != EnterKeySequence {
		t.Errorf("expected both the second and third SendKeys calls to be EnterKeySequence, got %#v", inst.sendCalls)
	}
}

// retryConfirmFake is a fakePaneSubmitter whose pane only shows a change once
// the retry Enter (the 3rd SendKeys call) has been sent — deterministically
// exercising the "first confirmation attempt fails, retry succeeds" path
// without depending on timing/call-count coincidences.
type retryConfirmFake struct {
	*fakePaneSubmitter
}

func (f *retryConfirmFake) HasUpdated() (bool, bool) {
	return len(f.sendCalls) >= 3, false
}

// TestSubmitDriverContent_ConfirmedOnRetry_Succeeds asserts that when the
// first Enter appears swallowed but the retry Enter is confirmed, the overall
// call succeeds rather than returning ErrSubmitNotConfirmed.
func TestSubmitDriverContent_ConfirmedOnRetry_Succeeds(t *testing.T) {
	t.Parallel()
	inst := &retryConfirmFake{fakePaneSubmitter: newFakePaneSubmitter()}

	err := SubmitDriverContent(context.Background(), inst, "content", time.Millisecond, 5*time.Millisecond)
	if err != nil {
		t.Fatalf("SubmitDriverContent returned unexpected error: %v", err)
	}
	if len(inst.sendCalls) != 3 {
		t.Fatalf("SendKeys called %d times, want exactly 3 (content, Enter, retry Enter) — got %#v", len(inst.sendCalls), inst.sendCalls)
	}
}

// TestWaitForPaneUpdate_ReturnsTrue_AsSoonAsChangeObserved is the direct
// regression test for the "same message delivered repeatedly" bug: the old
// fixed-500ms-sleep verification in sendInitialPromptTick declared a send
// "swallowed" (and resent the identical prompt) whenever the pane's actual
// update landed slower than 500ms. waitForPaneUpdate must return true the
// moment a change is observed, not wait out the full window first.
func TestWaitForPaneUpdate_ReturnsTrue_AsSoonAsChangeObserved(t *testing.T) {
	t.Parallel()
	// Scripted: two "no change yet" polls, then a change — simulating a
	// capture-pane round trip slower than any fixed short sleep would allow for.
	checker := &fakePaneSettleChecker{updates: []bool{false, false, true}}

	start := time.Now()
	updated := waitForPaneUpdate(context.Background(), checker, 5*time.Millisecond, time.Second)
	elapsed := time.Since(start)

	if !updated {
		t.Fatal("waitForPaneUpdate returned false even though the pane did change within the window")
	}
	if elapsed >= time.Second {
		t.Errorf("waitForPaneUpdate took %v — should have returned as soon as the change was observed, not waited out the full window", elapsed)
	}
}

// TestWaitForPaneUpdate_ReturnsFalse_When_PaneNeverChanges verifies the real
// swallow case (pane genuinely never updates) is still detected once maxWait
// elapses, preserving the fallback exact-content check in sendInitialPromptTick.
func TestWaitForPaneUpdate_ReturnsFalse_When_PaneNeverChanges(t *testing.T) {
	t.Parallel()
	checker := &fakePaneSettleChecker{updates: []bool{false}}

	if waitForPaneUpdate(context.Background(), checker, time.Millisecond, 20*time.Millisecond) {
		t.Error("waitForPaneUpdate returned true even though the pane never reported a change")
	}
}

// TestWaitForPaneUpdate_ReturnsFalse_When_ContextCancelled mirrors
// TestWaitForPaneSettle_should_returnImmediately_When_ContextCancelled —
// waitForPaneUpdate must not block past context cancellation.
func TestWaitForPaneUpdate_ReturnsFalse_When_ContextCancelled(t *testing.T) {
	t.Parallel()
	checker := &fakePaneSettleChecker{updates: []bool{false}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if waitForPaneUpdate(ctx, checker, time.Millisecond, time.Second) {
		t.Error("waitForPaneUpdate returned true after context was already cancelled")
	}
}

// TestSessionPackage_NoDirectSendKeysPlusEnterConcatenation is a structural
// regression guard for BUG-031 — see sendkeysguard's doc comment. Also run
// against server/mcp, server/services, and session/tymux, since the pattern
// this guards against reappeared independently in all three.
func TestSessionPackage_NoDirectSendKeysPlusEnterConcatenation(t *testing.T) {
	t.Parallel()
	sendkeysguard.CheckNoSingleWriteEnterConcatenation(t, ".", "pane_submit.go")
}
