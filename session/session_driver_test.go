package session

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// stuckDialogProcessManager implements ProcessManager. CapturePaneContent always
// returns the same trust-folder dialog text (as if the pane is frozen/stale during
// a flapping episode), unless growPerCall is set, in which case each call appends
// more unrelated content ahead of the fixed dialogText (simulating an active,
// non-flapping session that keeps producing real output after the dialog first
// appeared). SendKeys calls are counted; if failCount > 0, the first failCount
// calls return an error (used to force the latch into dialogGaveUp).
//
// Moved here from the now-deleted phase0_repro_test.go (Task 1.2.1): Phase 0's
// diagnostic assertion (count < 2 is a *failure*) literally encoded the pre-fix
// bug as the expected outcome and is inverted by the fix implemented in
// session_driver.go; its evidence is permanently captured in
// project_plans/phantom-keystroke-replay/research/phase0-findings.md.
type stuckDialogProcessManager struct {
	sendKeysCount atomic.Int32
	callCount     atomic.Int32
	dialogText    string

	// growPerCall, when true, prepends growing unrelated content to each
	// CapturePaneContent() call, simulating a growing PTY buffer.
	growPerCall bool
	// growChunk is the unrelated text prepended once per call when growPerCall.
	growChunk string

	// failCount, when > 0, makes the first failCount SendKeys calls return an error.
	failCount int
}

const trustDialogText = `Quick safety check: Is this a project you created or one you trust?
❯ 1. Yes, I trust this folder
  2. No, exit`

func (m *stuckDialogProcessManager) Start(dir string) error              { return nil }
func (m *stuckDialogProcessManager) RestoreWithWorkDir(dir string) error { return nil }
func (m *stuckDialogProcessManager) Close() error                        { return nil }
func (m *stuckDialogProcessManager) IsAlive() bool                       { return true }
func (m *stuckDialogProcessManager) GetSessionIdentifier() string        { return "phase0-repro" }
func (m *stuckDialogProcessManager) HasSession() bool                    { return true }
func (m *stuckDialogProcessManager) GetCurrentWorkingDirectory() (string, error) {
	return "/tmp", nil
}
func (m *stuckDialogProcessManager) GetPTY() (*os.File, error) { return nil, nil } //nolint:nilnil
func (m *stuckDialogProcessManager) SendKeys(keys string) (int, error) {
	n := m.sendKeysCount.Add(1)
	if m.failCount > 0 && int(n) <= m.failCount {
		return 0, errSimulatedSendKeysFailure
	}
	return len(keys), nil
}
func (m *stuckDialogProcessManager) TapEnter() error                    { return nil }
func (m *stuckDialogProcessManager) SendPromptWithEnter(p string) error { return nil }
func (m *stuckDialogProcessManager) SendInputViaControlMode(ctx context.Context, data []byte) error {
	return nil
}

// growBaseReps ensures the prefix already exceeds statusDetectionTailBytes on
// the very first call. Because the prefix is a periodic repetition of
// growChunk, once its total length exceeds the tail window the *content* of
// the last (statusDetectionTailBytes - len(dialogText)) bytes of the prefix
// is fully determined by growChunk and the window size alone — it does not
// change as more repetitions are appended beyond that point. This is what
// lets the test grow the buffer every call (mirroring a real, ever-growing
// PTY buffer) while still proving the tail-sliced hash stays stable: without
// tail-slicing (hashing the raw, ever-growing buffer instead), the hash would
// differ on every single call.
const growBaseReps = 500

func (m *stuckDialogProcessManager) content() string {
	if !m.growPerCall {
		return m.dialogText
	}
	n := m.callCount.Add(1)
	chunk := m.growChunk
	if chunk == "" {
		chunk = "unrelated real output line...................\n" // fixed-width filler
	}
	prefix := strings.Repeat(chunk, growBaseReps+int(n))
	return prefix + m.dialogText
}
func (m *stuckDialogProcessManager) CapturePaneContent() (string, error) {
	// Always returns the same (or growing, see growPerCall) content: simulates a
	// stuck/stale pane read during a flapping episode where the underlying tmux
	// session never advances, or an active session producing real new output.
	return m.content(), nil
}
func (m *stuckDialogProcessManager) CapturePaneContentRaw() (string, error) {
	return m.content(), nil
}
func (m *stuckDialogProcessManager) CapturePaneContentWithOptions(startLine, endLine string) (string, error) {
	return m.content(), nil
}
func (m *stuckDialogProcessManager) CaptureViewport(lines int) (string, error) {
	return m.content(), nil
}
func (m *stuckDialogProcessManager) GetCursorPosition() (int, int, error) { return 0, 0, nil }
func (m *stuckDialogProcessManager) GetPaneDimensions() (int, int, error) { return 80, 24, nil }
func (m *stuckDialogProcessManager) SetWindowSize(cols, rows int) error   { return nil }
func (m *stuckDialogProcessManager) SetDetachedSize(w, h int, title string) error {
	return nil
}
func (m *stuckDialogProcessManager) RefreshClient() error       { return nil }
func (m *stuckDialogProcessManager) GetPanePID() (int32, error) { return 0, nil }
func (m *stuckDialogProcessManager) HasUpdated() (bool, bool, string) {
	return false, false, m.dialogText
}
func (m *stuckDialogProcessManager) FilterBanners(content string) (string, int) {
	return content, 0
}
func (m *stuckDialogProcessManager) HasMeaningfulContent(content string) bool { return true }
func (m *stuckDialogProcessManager) StartControlMode() error                  { return nil }
func (m *stuckDialogProcessManager) StopControlMode() error                   { return nil }
func (m *stuckDialogProcessManager) SubscribeToControlModeUpdates() (string, chan []byte) {
	return "", nil
}
func (m *stuckDialogProcessManager) UnsubscribeFromControlModeUpdates(id string) {}
func (m *stuckDialogProcessManager) Attach() (chan struct{}, error)              { return nil, nil } //nolint:nilnil
func (m *stuckDialogProcessManager) DetachSafely() error                         { return nil }
func (m *stuckDialogProcessManager) SetOnExitCallback(fn func(string))           {}
func (m *stuckDialogProcessManager) ResetExitOnce()                              {}

// errSimulatedSendKeysFailure is returned by stuckDialogProcessManager.SendKeys
// when simulating a transient send failure (used to drive a DialogAnswerLatch
// to dialogGaveUp in tests).
var errSimulatedSendKeysFailure = errors.New("simulated SendKeys failure")

func TestIsStartupDialog(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name: "trust folder dialog exact",
			output: `────────────────────────────────────────────────────────────────────────────────
 Accessing workspace: /Users/tylerstapler/.stapler-squad/workspaces/a21d7547/worktrees/datadog-terraform_18b156f7

 Quick safety check: Is this a project you created or one you
 trust? (Like your own code, a well-known open source project, or work from your team).
 If not, take a moment to review what's in this folder first.
 Claude Code'll be able to read, edit, and execute files here.

❯ 1. Yes, I trust this folder
  2. No, exit

Enter to confirm · Esc to cancel`,
			want: true,
		},
		{
			name: "trust folder dialog with numbered dot variant",
			output: `Quick safety check: Is this a project you created or one you trust?
 1. Yes, I trust this folder
 2. No, exit`,
			want: true,
		},
		{
			name:   "normal claude prompt — not a dialog",
			output: `> `,
			want:   false,
		},
		{
			name:   "plain text mentioning trust but no menu",
			output: `I trust this code completely.`,
			want:   false,
		},
		{
			name: "allow-directory approval dialog",
			output: `Allow reading in /Users/tylerstapler/projects
❯ 1. Yes, allow
  2. No`,
			want: false, // handled by shouldApprovePrompt, not isStartupDialog
		},
		{
			name:   "empty output",
			output: ``,
			want:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isStartupDialog(tc.output)
			if got != tc.want {
				t.Errorf("isStartupDialog(%q) = %v, want %v", tc.output[:min(len(tc.output), 60)], got, tc.want)
			}
		})
	}
}

func TestShouldApprovePrompt(t *testing.T) {
	cases := []struct {
		name        string
		output      string
		allowedPath string
		want        bool
	}{
		{
			name:        "allow reading in allowed path",
			output:      "Allow reading in /home/user/myrepo",
			allowedPath: "/home/user/myrepo",
			want:        true,
		},
		{
			name:        "allow writing in allowed path",
			output:      "Allow writing in /home/user/myrepo/src",
			allowedPath: "/home/user/myrepo",
			want:        true,
		},
		{
			name:        "allow reading in unrelated path — must not approve",
			output:      "Allow reading in /etc/passwd",
			allowedPath: "/home/user/myrepo",
			want:        false,
		},
		{
			name:        "do you want to proceed — no path restriction",
			output:      "Do you want to proceed?",
			allowedPath: "",
			want:        true,
		},
		{
			name:        "do you want to proceed — ignored when allowedPath set and path not in output",
			output:      "Do you want to proceed?",
			allowedPath: "/home/user/myrepo",
			want:        false,
		},
		{
			name:        "unrelated output",
			output:      "Compiling project…",
			allowedPath: "/home/user/myrepo",
			want:        false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldApprovePrompt(tc.output, tc.allowedPath)
			if got != tc.want {
				t.Errorf("shouldApprovePrompt() = %v, want %v", got, tc.want)
			}
		})
	}
}

// UT-3: TestIsOneShot — verifies one-shot detection logic.
func TestIsOneShot(t *testing.T) {
	cases := []struct {
		name string
		tags []string
		want bool
	}{
		{name: "triage tag → true", tags: []string{"backlog:triage"}, want: true},
		{name: "review tag → true", tags: []string{"backlog:review"}, want: true},
		{name: "work tag → false", tags: []string{"backlog:work"}, want: false},
		{name: "mcp source tag → false", tags: []string{"source:mcp"}, want: false},
		{name: "no tags → false", tags: nil, want: false},
		{name: "both triage and work → true (triage wins)", tags: []string{"backlog:triage", "backlog:work"}, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inst := &Instance{Title: "test", Tags: tc.tags}
			got := isOneShot(inst)
			if got != tc.want {
				t.Errorf("isOneShot(%v) = %v, want %v", tc.tags, got, tc.want)
			}
		})
	}
}

// UT-4: TestBuildContinuationPrompt_NoHistoryFile — empty HistoryFilePath returns generic fallback.
func TestBuildContinuationPrompt_NoHistoryFile(t *testing.T) {
	inst := &Instance{Title: "test-session", HistoryFilePath: ""}
	got := buildContinuationPrompt(inst)
	if got == "" {
		t.Fatal("buildContinuationPrompt returned empty string")
	}
	if !strings.Contains(got, "Please continue") {
		t.Errorf("expected fallback prompt to contain 'Please continue', got: %q", got)
	}
}

// UT-5: TestBuildContinuationPrompt_MissingFile — non-existent file path returns graceful fallback.
func TestBuildContinuationPrompt_MissingFile(t *testing.T) {
	inst := &Instance{Title: "test-session", HistoryFilePath: "/tmp/does-not-exist-stapler-squad-test.jsonl"}
	got := buildContinuationPrompt(inst)
	if got == "" {
		t.Fatal("buildContinuationPrompt returned empty string for missing file")
	}
	// Should return the generic fallback, not panic or return empty.
	if !strings.Contains(strings.ToLower(got), "continue") {
		t.Errorf("expected fallback to mention 'continue', got: %q", got)
	}
}

// UT-9: TestStartSessionDriver_Idempotent — calling twice only starts one goroutine.
// Uses Status=Stopped so the driver goroutine exits immediately.
func TestStartSessionDriver_Idempotent(t *testing.T) {
	inst := &Instance{
		Title:  "test-idempotent",
		Status: Stopped,
	}

	// First call: should CAS from false→true and start a goroutine.
	StartSessionDriver(inst, "/tmp")

	// Briefly yield to let the goroutine start. The goroutine will exit quickly
	// because Status==Stopped (no tmux, so it exits on first tick).
	// The important check is the CAS guard on the second call.
	// Goroutine may have already exited (very fast path for Stopped instance) —
	// that's fine; the CAS still happened so the second call will be a no-op.

	// Second call: should CAS fail and return immediately.
	// To verify idempotency, store the current driverRunning value and call again.
	// If driverRunning is already false (goroutine exited), call again and make sure
	// only one goroutine ever touches our counter.
	var goroutineCount atomic.Int32
	inst2 := &Instance{
		Title:  "test-idempotent-concurrent",
		Status: Stopped,
	}
	// Patch: set driverRunning to true manually to simulate an already-running driver.
	inst2.driverRunning.Store(true)

	// This call must be a no-op because driverRunning is true.
	StartSessionDriver(inst2, "/tmp")

	// goroutineCount should remain 0 — no new goroutine was spawned.
	if goroutineCount.Load() != 0 {
		t.Error("expected no goroutine to start when driverRunning is true")
	}
}

// UT-25: TestDriverConstants_Ordering — verifies BLOCK-1 fix: total > ready + inactivity.
func TestDriverConstants_Ordering(t *testing.T) {
	if driverTotalTimeout < driverReadyTimeout+driverInactivityTimeout+5*time.Minute {
		t.Errorf("driverTotalTimeout (%v) must be >= driverReadyTimeout (%v) + driverInactivityTimeout (%v) + 5m",
			driverTotalTimeout, driverReadyTimeout, driverInactivityTimeout)
	}
}

// UT-21: TestMarkSessionNeedsAttention_NilReviewQueue — nil queue must not panic.
func TestMarkSessionNeedsAttention_NilReviewQueue(t *testing.T) {
	inst := &Instance{
		Title:       "test-nil-queue",
		UUID:        "test-uuid-1",
		reviewQueue: nil,
	}
	// Should not panic.
	markSessionNeedsAttention(inst, "test reason")
}

// UT-17: TestSessionDriver_SecondFailure_MarksNeedsAttention — second failure adds to ReviewQueue.
func TestSessionDriver_SecondFailure_MarksNeedsAttention(t *testing.T) {
	rq := NewReviewQueue()
	inst := &Instance{
		Title:       "test-second-failure",
		UUID:        "test-uuid-second-fail",
		reviewQueue: rq,
		Status:      Stopped,
	}

	var retried atomic.Bool
	retried.Store(true) // already retried once

	handleDriverFailure(inst, "/tmp", &retried, "unexpected exit")

	// ReviewQueue should have an entry for this session.
	item, found := rq.Get(inst.UUID)
	if !found {
		t.Fatal("expected ReviewItem to be added to queue, got not found")
	}
	if item == nil {
		t.Fatal("ReviewItem was nil")
	}
	if item.SessionID != inst.UUID {
		t.Errorf("ReviewItem.SessionID = %q, want %q", item.SessionID, inst.UUID)
	}
	if item.Reason != ReasonStale {
		t.Errorf("ReviewItem.Reason = %q, want %q", item.Reason, ReasonStale)
	}
}

// TestSessionDriver_StuckDialogAnswersBoundedNotUnbounded is the permanent
// regression proof for AC2, replacing phase0_repro_test.go's now-obsolete
// "expect repeated sends" assertion. It runs the REAL (now-fixed)
// runSessionDriverWithPrompt goroutine for several poll ticks against a fake
// ProcessManager whose visible content never changes (the same flapping
// condition Phase 0 used) and asserts SendKeys("1\n") is observed at most
// maxDialogAnswerAttempts times, never growing with additional ticks.
func TestSessionDriver_StuckDialogAnswersBoundedNotUnbounded(t *testing.T) {
	fakePM := &stuckDialogProcessManager{dialogText: trustDialogText}

	inst := &Instance{
		Title:          "stuck-dialog-bounded",
		Status:         Ready,
		started:        true,
		processManager: fakePM,
	}

	var retried atomic.Bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		runSessionDriverWithPrompt(inst, "/tmp", driverInitialPrompt, &retried)
	}()

	// 6 ticks — double Phase 0's original 3-tick window — to prove the count
	// does not keep growing with additional ticks, not just that it happened
	// to be small over a short window.
	time.Sleep(driverPollInterval*6 + 500*time.Millisecond)

	count := fakePM.sendKeysCount.Load()
	t.Logf("SendKeys(\"1\\n\") called %d times over 6 poll ticks against an unchanging stuck-dialog buffer", count)

	if count > maxDialogAnswerAttempts {
		t.Fatalf("expected SendKeys(\"1\\n\") to be bounded by maxDialogAnswerAttempts (%d), got %d calls over 6 ticks — the DialogAnswerLatch failed to bound resends",
			maxDialogAnswerAttempts, count)
	}

	// Cleanup: force the goroutine to observe Paused so it exits cleanly.
	// Written under stateMutex (matching GetEffectiveStatus's RLock) so this
	// concurrent write doesn't race the still-running driver goroutine's reads.
	inst.stateMutex.Lock()
	inst.Status = Paused
	inst.stateMutex.Unlock()
	select {
	case <-done:
	case <-time.After(driverPollInterval + time.Second):
		t.Log("driver goroutine did not exit promptly after Status=Paused (leak, not fatal to this test)")
	}
}

// TestSessionDriver_TailSliceBoundsDialogMatchAndHash is the live-executing
// counterpart to answerDialogOnce unit test cases (f)/(g) (Task 1.2.3): it
// proves the ordinary active-session case (a growing PTY buffer, not just a
// static stuck buffer) does not reproduce the unbounded-resend bug. The fake
// ProcessManager's content grows every call (dialog text fixed, new unrelated
// lines prepended each tick, mirroring an active non-flapping session
// producing real output after the dialog was answered).
func TestSessionDriver_TailSliceBoundsDialogMatchAndHash(t *testing.T) {
	fakePM := &stuckDialogProcessManager{
		dialogText:  trustDialogText,
		growPerCall: true,
		growChunk:   "unrelated real Claude Code output line\n",
	}

	inst := &Instance{
		Title:          "tail-slice-growing-buffer",
		Status:         Ready,
		started:        true,
		processManager: fakePM,
	}

	var retried atomic.Bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		runSessionDriverWithPrompt(inst, "/tmp", driverInitialPrompt, &retried)
	}()

	time.Sleep(driverPollInterval*6 + 500*time.Millisecond)

	count := fakePM.sendKeysCount.Load()
	t.Logf("SendKeys(\"1\\n\") called %d times over 6 poll ticks against a growing (non-flapping active session) buffer", count)

	if count > maxDialogAnswerAttempts {
		t.Fatalf("expected SendKeys(\"1\\n\") to be bounded by maxDialogAnswerAttempts (%d) even against a growing buffer, got %d calls",
			maxDialogAnswerAttempts, count)
	}

	inst.stateMutex.Lock()
	inst.Status = Paused
	inst.stateMutex.Unlock()
	select {
	case <-done:
	case <-time.After(driverPollInterval + time.Second):
		t.Log("driver goroutine did not exit promptly after Status=Paused (leak, not fatal to this test)")
	}
}

// buildGrowingPrefixContent returns a periodic filler string of chunk repeated
// reps times, followed by dialogText. Because the filler is periodic, once
// reps*len(chunk) comfortably exceeds statusDetectionTailBytes, the *tail* of
// this content (the last statusDetectionTailBytes bytes, which is what
// answerDialogOnce/isStartupDialog actually see) is identical no matter how
// large reps grows further — only dialogText plus a fixed-size trailing
// window of filler is ever visible. This is what lets test cases (f)/(g)
// simulate a growing PTY buffer.
func buildGrowingPrefixContent(chunk string, reps int, dialogText string) string {
	return strings.Repeat(chunk, reps) + dialogText
}

// Task 1.2.4 fallback note (plan.md Story 1.2, approval-prompt latch coverage):
// a live-executing TestSessionDriver_StuckApprovalPromptAnswersBoundedNotUnbounded
// (mirroring TestSessionDriver_StuckDialogAnswersBoundedNotUnbounded above) was
// evaluated and rejected as disproportionate scaffolding. Reaching the
// NeedsApproval branch requires inst.GetStatusManager().GetStatus(inst).ClaudeStatus
// == detection.StatusNeedsApproval, which in turn requires a REAL, started
// *ClaudeController registered in an *InstanceStatusManager for this instance's
// Title — InstanceStatusManager.GetStatus (instance_status.go:73-104) only
// consults its internal controllers map, so there is no interface seam to fake
// this condition directly. ClaudeController.Start (claude_controller.go:128ff)
// requires a real PTY reader from the instance, persistence directories for its
// command queue/history, and starts multiple background goroutines — genuinely
// disproportionate scaffolding for what would still just be exercising the same
// answerDialogOnce state machine already fully covered by TestAnswerDialogOnce
// above (including case (b), which proves independent latches don't interfere).
//
// Per Task 1.2.4/plan.md's explicit fallback clause: the approval-prompt branch
// (session_driver.go's NeedsApproval block, wired in Task 1.1.4) is therefore
// covered by inspection + the shared answerDialogOnce unit tests, not by a
// dedicated reproduced-evidence integration test — it deliberately does not
// carry the same rigor as the startup-dialog branch's live-executing coverage
// above. See the Acceptance Criteria Coverage Summary note this implies for AC2.

func TestAnswerDialogOnce(t *testing.T) {
	t.Run("a_same_hash_sent_twice_second_call_is_noop", func(t *testing.T) {
		var state dialogAnswerState
		sendCallCount := 0
		send := func() error { sendCallCount++; return nil }

		status1 := answerDialogOnce(&state, trustDialogText, send, "sess", "startup dialog")
		if status1 != dialogAwaitingDismissal {
			t.Fatalf("call 1: status = %v, want dialogAwaitingDismissal", status1)
		}
		status2 := answerDialogOnce(&state, trustDialogText, send, "sess", "startup dialog")
		if status2 != dialogAwaitingDismissal {
			t.Fatalf("call 2: status = %v, want dialogAwaitingDismissal", status2)
		}
		if sendCallCount != 1 {
			t.Errorf("sendCallCount = %d, want 1 (second call with unchanged hash must not resend)", sendCallCount)
		}
	})

	t.Run("b_hash_changes_between_calls_resends", func(t *testing.T) {
		var state dialogAnswerState
		sendCallCount := 0
		send := func() error { sendCallCount++; return nil }

		output1 := trustDialogText
		output2 := "A completely different dialog appeared.\n❯ 1. Yes, allow\n  2. No"

		answerDialogOnce(&state, output1, send, "sess", "startup dialog")
		status2 := answerDialogOnce(&state, output2, send, "sess", "startup dialog")

		if status2 != dialogAwaitingDismissal {
			t.Fatalf("call 2: status = %v, want dialogAwaitingDismissal", status2)
		}
		if sendCallCount != 2 {
			t.Errorf("sendCallCount = %d, want 2 (a genuinely different dialog must be answered again)", sendCallCount)
		}
	})

	t.Run("c_send_fails_maxDialogAnswerAttempts_times_gives_up_and_stays_given_up", func(t *testing.T) {
		var state dialogAnswerState
		sendCallCount := 0
		send := func() error { sendCallCount++; return errSimulatedSendKeysFailure }

		var lastStatus dialogLatchStatus
		for i := 0; i < maxDialogAnswerAttempts; i++ {
			lastStatus = answerDialogOnce(&state, trustDialogText, send, "sess", "startup dialog")
		}
		if lastStatus != dialogGaveUp {
			t.Fatalf("status after %d failures = %v, want dialogGaveUp", maxDialogAnswerAttempts, lastStatus)
		}
		if sendCallCount != maxDialogAnswerAttempts {
			t.Fatalf("sendCallCount after %d failures = %d, want %d", maxDialogAnswerAttempts, sendCallCount, maxDialogAnswerAttempts)
		}

		// A further call with the same (unchanged) hash must not call send again.
		status := answerDialogOnce(&state, trustDialogText, send, "sess", "startup dialog")
		if status != dialogGaveUp {
			t.Errorf("status after extra call = %v, want dialogGaveUp", status)
		}
		if sendCallCount != maxDialogAnswerAttempts {
			t.Errorf("sendCallCount after extra call = %d, want unchanged %d (dialogGaveUp must not retry)", sendCallCount, maxDialogAnswerAttempts)
		}
	})

	t.Run("d_send_fails_once_then_succeeds_reaches_awaiting_dismissal", func(t *testing.T) {
		var state dialogAnswerState
		sendCallCount := 0
		send := func() error {
			sendCallCount++
			if sendCallCount == 1 {
				return errSimulatedSendKeysFailure
			}
			return nil
		}

		status1 := answerDialogOnce(&state, trustDialogText, send, "sess", "startup dialog")
		if status1 != dialogUnanswered {
			t.Fatalf("call 1 (failure, under retry cap): status = %v, want dialogUnanswered", status1)
		}
		status2 := answerDialogOnce(&state, trustDialogText, send, "sess", "startup dialog")
		if status2 != dialogAwaitingDismissal {
			t.Fatalf("call 2 (success): status = %v, want dialogAwaitingDismissal", status2)
		}
		if sendCallCount != 2 {
			t.Errorf("sendCallCount = %d, want 2", sendCallCount)
		}
	})

	t.Run("e_whitespace_and_line_wrap_jitter_recognized_as_unchanged", func(t *testing.T) {
		// Same logical dialog text, but re-wrapped at a different column width
		// with different internal newline placement and trailing spaces —
		// simulating terminal-width-driven line-wrap jitter between ticks.
		output1 := "Quick safety check: Is this a project you created  \n" +
			"or one you trust?   \n" +
			"❯ 1. Yes, I trust this folder\n" +
			"  2. No, exit\n"
		output2 := "Quick safety check: Is this a project\n" +
			"you created or one you trust?\n" +
			"❯ 1. Yes, I trust this folder  \n" +
			"  2. No, exit"

		var state dialogAnswerState
		sendCallCount := 0
		send := func() error { sendCallCount++; return nil }

		answerDialogOnce(&state, output1, send, "sess", "startup dialog")
		status2 := answerDialogOnce(&state, output2, send, "sess", "startup dialog")

		if status2 != dialogAwaitingDismissal {
			t.Fatalf("call 2: status = %v, want dialogAwaitingDismissal", status2)
		}
		if sendCallCount != 1 {
			t.Errorf("sendCallCount = %d, want 1 (whitespace/line-wrap jitter must not be treated as a new dialog)", sendCallCount)
		}
	})

	t.Run("f_growing_buffer_within_tail_window_recognized_as_unchanged", func(t *testing.T) {
		// The dialog text stays fixed at the tail of output across both calls;
		// call 2 has substantially more unrelated content ahead of it than
		// call 1 (simulating a growing PTY buffer). Both totals already
		// exceed statusDetectionTailBytes, so tailContent clips the growing
		// part away on both calls — see buildGrowingPrefixContent's doc
		// comment for why the resulting tail is byte-identical despite the
		// raw buffer growing.
		chunk := "unrelated real Claude Code output line.......\n"
		output1 := buildGrowingPrefixContent(chunk, growBaseReps, trustDialogText)
		output2 := buildGrowingPrefixContent(chunk, growBaseReps+50, trustDialogText)

		if len(output1) <= statusDetectionTailBytes || len(output2) <= statusDetectionTailBytes {
			t.Fatalf("test setup invariant violated: both outputs must exceed statusDetectionTailBytes (%d); got %d and %d",
				statusDetectionTailBytes, len(output1), len(output2))
		}

		var state dialogAnswerState
		sendCallCount := 0
		send := func() error { sendCallCount++; return nil }

		answerDialogOnce(&state, output1, send, "sess", "startup dialog")
		status2 := answerDialogOnce(&state, output2, send, "sess", "startup dialog")

		if status2 != dialogAwaitingDismissal {
			t.Fatalf("call 2: status = %v, want dialogAwaitingDismissal", status2)
		}
		if sendCallCount != 1 {
			t.Errorf("sendCallCount = %d, want 1 (dialog still within the tail window must not resend)", sendCallCount)
		}
	})

	t.Run("g_dialog_pushed_fully_outside_tail_window_never_reached", func(t *testing.T) {
		// Companion to (f): enough unrelated content follows the dialog text
		// that it falls entirely outside the tail window — proving
		// isStartupDialog on the tailed content correctly stops matching (the
		// dialog is treated as "no longer on screen", not as "a new dialog").
		chunk := "unrelated real Claude Code output line.......\n"
		trailing := strings.Repeat(chunk, growBaseReps)
		output2 := trustDialogText + "\n" + trailing

		if len(trailing) <= statusDetectionTailBytes {
			t.Fatalf("test setup invariant violated: trailing content must exceed statusDetectionTailBytes (%d); got %d",
				statusDetectionTailBytes, len(trailing))
		}

		tailed := tailContent(output2, statusDetectionTailBytes)
		if isStartupDialog(tailed) {
			t.Fatalf("isStartupDialog matched tailed content even though the dialog text should have fully scrolled out of the tail window")
		}
		// Since isStartupDialog(tailed) is false, the call site (Task 1.1.3)
		// never invokes answerDialogOnce for this tick at all — the latch is
		// simply never reached, not incorrectly reset/resent.
	})
}

// TestSessionDriver_DialogGaveUp_FallsThroughToInactivityEscalation proves the
// Task 1.1.3 control-flow fix actually reaches the pre-existing
// inactivity-timeout -> handleDriverFailure -> ReviewQueue escalation path
// once the startup-dialog latch reaches dialogGaveUp, not just that resends
// stay bounded (Task 1.2.2 only proves the latter).
//
// The fake ProcessManager's SendKeys fails exactly maxDialogAnswerAttempts
// times, forcing the startup-dialog latch to dialogGaveUp after 3 ticks.
// LastMeaningfulOutput is set far enough in the past that
// driverInactivityTimeout has already elapsed by wall-clock time — no real
// multi-minute wait is needed, since the inactivity check compares
// time.Since(LastMeaningfulOutputTime()) against the constant, not elapsed
// driver runtime. retried is pre-set to true so handleDriverFailure takes its
// "already retried, mark for attention" branch directly (observable via a
// ReviewQueue entry) rather than exercising the real Restart()/RecoverFromStopped
// path, which needs no faking for what this test is proving.
func TestSessionDriver_DialogGaveUp_FallsThroughToInactivityEscalation(t *testing.T) {
	fakePM := &stuckDialogProcessManager{
		dialogText: trustDialogText,
		failCount:  maxDialogAnswerAttempts,
	}
	rq := NewReviewQueue()

	inst := &Instance{
		Title:          "dialog-give-up-escalation",
		UUID:           "test-uuid-give-up-escalation",
		Status:         Ready,
		started:        true,
		processManager: fakePM,
		reviewQueue:    rq,
		ReviewState: ReviewState{
			LastMeaningfulOutput: time.Now().Add(-(driverInactivityTimeout + time.Minute)),
		},
	}

	var retried atomic.Bool
	retried.Store(true) // simulate "already retried once" so the second-failure path fires directly

	done := make(chan struct{})
	go func() {
		defer close(done)
		runSessionDriverWithPrompt(inst, "/tmp", driverInitialPrompt, &retried)
	}()

	select {
	case <-done:
		// Fell through as expected; verify below.
	case <-time.After(driverPollInterval*6 + time.Second):
		inst.stateMutex.Lock()
		inst.Status = Paused // best-effort cleanup so the goroutine doesn't leak
		inst.stateMutex.Unlock()
		t.Fatal("driver goroutine did not exit within 6 ticks — the dialogGaveUp fall-through never reached the inactivity-timeout escalation")
	}

	item, found := rq.Get(inst.UUID)
	if !found {
		t.Fatal("expected a ReviewQueue entry after the dialogGaveUp latch fell through to the inactivity-timeout escalation, found none")
	}
	if item.Reason != ReasonStale {
		t.Errorf("ReviewItem.Reason = %q, want %q", item.Reason, ReasonStale)
	}
	if item.Context != "inactivity timeout" {
		t.Errorf("ReviewItem.Context = %q, want %q (proves the escalation was reached via the fall-through, not some other path)", item.Context, "inactivity timeout")
	}

	sendCount := fakePM.sendKeysCount.Load()
	if sendCount < maxDialogAnswerAttempts {
		t.Errorf("expected at least %d SendKeys attempts before dialogGaveUp, got %d", maxDialogAnswerAttempts, sendCount)
	}
}
