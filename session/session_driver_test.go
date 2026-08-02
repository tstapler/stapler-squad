package session

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

// TestShouldAnswerStartupDialog verifies the latch guard that prevents
// re-sending "1\n" while the dialog we already answered is still visible —
// regardless of how long the terminal takes to redraw. This is the extracted
// helper used in runSessionDriverWithPrompt.
func TestShouldAnswerStartupDialog(t *testing.T) {
	t.Run("returns true when dialog present and not awaiting clear", func(t *testing.T) {
		got := shouldAnswerStartupDialog(true, false)
		if !got {
			t.Error("expected true for fresh dialog")
		}
	})

	t.Run("returns false while awaiting clear, no matter how long it's been", func(t *testing.T) {
		got := shouldAnswerStartupDialog(true, true)
		if got {
			t.Error("expected false while awaiting clear")
		}
	})

	t.Run("returns false for non-dialog output regardless of latch state", func(t *testing.T) {
		got := shouldAnswerStartupDialog(false, false)
		if got {
			t.Error("expected false for non-dialog output")
		}
	})
}

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

// TestShouldApprovePromptOnce covers #165: the driver re-checks NeedsApproval
// every poll tick, and the status can remain NeedsApproval for an arbitrarily
// long number of ticks after "1\r" is sent while the PTY redraws. A fixed-
// duration cooldown can still expire before the redraw finishes on a slow
// terminal, resending "1" into whatever appears next once it does — the latch
// must stay armed for as long as the dialog is actually still visible, with
// no time limit.
func TestShouldApprovePromptOnce(t *testing.T) {
	t.Run("returns true when dialog present and not awaiting clear", func(t *testing.T) {
		if !shouldApprovePromptOnce(true, false) {
			t.Error("expected true for fresh dialog")
		}
	})

	t.Run("returns false while awaiting clear, no matter how long it's been", func(t *testing.T) {
		if shouldApprovePromptOnce(true, true) {
			t.Error("expected false while awaiting clear — prevents repeated 1s")
		}
	})

	t.Run("returns false for non-matching output regardless of latch state", func(t *testing.T) {
		if shouldApprovePromptOnce(false, false) {
			t.Error("expected false for non-dialog output")
		}
	})
}

// TestApprovalAwaitingClearLatch_PreventsPhantomReplayAcrossReconnectChurn
// regression-tests backlog item 04089969-0f19-499c-be34-2e8bcfc4f13e (phantom
// keystroke replay across reconnect churn). The ticket's log excerpt showed
// `[streamViaControlMode] capture-pane failed, sending stopped notice
// err="session not started or paused"` while the driver's NeedsApproval
// polling loop kept resending "1\r" on every tick the stale preview buffer
// still showed the dialog. This drives processApprovalTick (Task 1.1.2.1's
// extraction from runSessionDriverWithPrompt's inline NeedsApproval block)
// directly across a simulated 6-tick flap — not a reimplementation of its
// awaitingClear threading rule — so the test exercises the real production
// state machine, per pre-mortem.md Failure #5.
func TestApprovalAwaitingClearLatch_PreventsPhantomReplayAcrossReconnectChurn(t *testing.T) {
	const allowedPath = "/home/user/project"
	dialogVisibleOutput := "Do you want to proceed? Allow reading /home/user/project"
	dialogGoneOutput := "some other terminal output, no dialog here"

	// tick 1: visible, unarmed; ticks 2-5: still visible, armed; tick 6: gone.
	ticks := []string{
		dialogVisibleOutput,
		dialogVisibleOutput,
		dialogVisibleOutput,
		dialogVisibleOutput,
		dialogVisibleOutput,
		dialogGoneOutput,
	}

	var sendKeysCalls int
	sendKeys := func() error {
		sendKeysCalls++
		return nil
	}

	var awaitingClear bool
	for _, output := range ticks {
		awaitingClear = processApprovalTick(nil, output, allowedPath, awaitingClear, sendKeys)
	}

	if sendKeysCalls != 1 {
		t.Errorf("expected sendKeys to be invoked exactly once across the 6-tick flap, got %d", sendKeysCalls)
	}
}

// TestApprovalAwaitingClearLatch_ReapprovesNewDialogAfterPriorOneFullyClears
// proves the awaitingClear latch is a same-dialog-resend guard, not a
// permanent "approve nothing after the first prompt ever" latch: once a
// dialog fully clears (approvalVisible goes false, resetting
// approvalAwaitingClear to false), a genuinely new dialog appearing
// afterward must still be approved once.
func TestApprovalAwaitingClearLatch_ReapprovesNewDialogAfterPriorOneFullyClears(t *testing.T) {
	const allowedPath = "/home/user/project"
	dialogVisibleOutput := "Do you want to proceed? Allow reading /home/user/project"
	dialogGoneOutput := "some other terminal output, no dialog here"

	var sendKeysCalls int
	sendKeys := func() error {
		sendKeysCalls++
		return nil
	}

	var awaitingClear bool

	// First dialog appears and is approved.
	awaitingClear = processApprovalTick(nil, dialogVisibleOutput, allowedPath, awaitingClear, sendKeys)
	if sendKeysCalls != 1 {
		t.Fatalf("expected first dialog to be approved, sendKeys called %d times", sendKeysCalls)
	}

	// Dialog fully clears — latch resets.
	awaitingClear = processApprovalTick(nil, dialogGoneOutput, allowedPath, awaitingClear, sendKeys)
	if awaitingClear {
		t.Fatalf("expected awaitingClear to reset to false once the dialog clears")
	}

	// A new dialog appears afterward — must be approved again.
	awaitingClear = processApprovalTick(nil, dialogVisibleOutput, allowedPath, awaitingClear, sendKeys)
	if sendKeysCalls != 2 {
		t.Errorf("expected a genuinely new dialog to be approved once, sendKeys called %d times", sendKeysCalls)
	}
	if !awaitingClear {
		t.Errorf("expected awaitingClear to be armed after approving the new dialog")
	}
}

// TestProcessApprovalTick_SendKeysError_LeavesAwaitingClearUnchanged verifies
// the untested error branch in processApprovalTick: when sendKeys() fails,
// awaitingClear must be returned unchanged (matching the original inline
// semantics this extraction preserves — a failed send must not falsely arm
// the double-fire guard, since the dialog was never actually answered).
// Verified by mutation: changing `if err != nil { return awaitingClear }` to
// `return true` left the pre-existing test suite green, so this test exists
// to catch exactly that regression.
//
// awaitingClear must start false here: shouldApprovePromptOnce only calls
// sendKeys when approvalVisible && !awaitingClear, so an awaitingClear=true
// starting point never reaches the branch under test at all.
func TestProcessApprovalTick_SendKeysError_LeavesAwaitingClearUnchanged(t *testing.T) {
	const allowedPath = "/home/user/project"
	dialogVisibleOutput := "Do you want to proceed? Allow reading /home/user/project"

	var sendKeysCalls int
	sendKeysErr := func() error {
		sendKeysCalls++
		return errors.New("tmux: invalid argument")
	}

	got := processApprovalTick(nil, dialogVisibleOutput, allowedPath, false, sendKeysErr)

	if sendKeysCalls != 1 {
		t.Fatalf("expected sendKeys to be attempted exactly once, got %d", sendKeysCalls)
	}
	if got {
		t.Errorf("expected awaitingClear to remain false after a failed sendKeys, got true — "+
			"a mutant returning true here would falsely arm the double-fire guard for a dialog "+
			"that was never actually answered, got awaitingClear=%v", got)
	}

	// Regression guard for the consequence of the mutation: because
	// awaitingClear correctly stayed false, the driver retries the send on
	// the very next tick while the dialog is still visible (the desired
	// "keep trying on transient send failure" behavior) rather than being
	// permanently latched as if the dialog had already been answered.
	got2 := processApprovalTick(nil, dialogVisibleOutput, allowedPath, got, sendKeysErr)
	if sendKeysCalls != 2 {
		t.Errorf("expected a retried send on the next tick since awaitingClear stayed false, "+
			"sendKeys called %d times", sendKeysCalls)
	}
	if got2 {
		t.Errorf("expected awaitingClear to still be false after a second failed sendKeys, got true")
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

// UT-9: TestStartSessionDriver_Idempotent — calling twice on the same instance only
// starts one goroutine. The CAS guard in StartSessionDriver prevents a second spawn.
func TestStartSessionDriver_Idempotent(t *testing.T) {
	inst := &Instance{
		Title:  "test-idempotent",
		Status: Stopped,
	}

	// Pre-set driverRunning = true to simulate an already-running driver.
	// This is the state the instance would be in after the first StartSessionDriver call
	// while the goroutine is still executing.
	inst.driverRunning.Store(true)

	// This call must be a no-op because driverRunning is already true.
	// StartSessionDriver checks CompareAndSwap(false, true) — when driverRunning is true,
	// the CAS fails and the function returns immediately without spawning a goroutine.
	StartSessionDriver(inst, "/tmp")

	// driverRunning should still be true (the no-op call did not reset it).
	if !inst.driverRunning.Load() {
		t.Error("driverRunning should still be true after no-op second call")
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

// BUG-041: TestAttemptBacklogNudge_FailedSend_StillReturnsNonZeroTime verifies that a
// failed SendKeys still produces a non-zero timestamp for the caller to record as
// nudgeSentAt. Before the fix, nudgeSentAt was only assigned in the success branch, so
// on failure the driver loop's guard (nudgeSentAt.IsZero() && idle > driverBacklogNudgeDelay)
// stayed true forever and retried the identical send on every subsequent tick — live
// evidence was 392 consecutive failed sends over ~13 minutes against one dead-pane session.
func TestAttemptBacklogNudge_FailedSend_StillReturnsNonZeroTime(t *testing.T) {
	// An Instance that was never started (SetTmuxSession was never called, so the
	// internal `started` atomic.Bool is false) makes SendKeys deterministically fail
	// with "cannot send keys to instance that has not been started or is paused" —
	// the same shape of permanent, non-retryable failure as a dead tmux pane.
	inst := &Instance{
		Title:  "test-nudge-failed-send",
		Status: Ready,
	}

	before := time.Now()
	got := attemptBacklogNudge(inst, 6*time.Minute)

	if got.IsZero() {
		t.Fatal("attemptBacklogNudge returned zero time on failed send — this reproduces BUG-041: " +
			"the caller's nudgeSentAt guard would stay zero, causing the identical send to retry every driver tick")
	}
	if got.Before(before) {
		t.Errorf("attemptBacklogNudge returned a time before the call started: got %v, want >= %v", got, before)
	}
}

// BUG-041 regression: TestAttemptBacklogNudge_FailedSend_RateLimitsRetry asserts the
// actual driver-loop guard condition (`nudgeSentAt.IsZero() && idle > driverBacklogNudgeDelay`)
// no longer re-fires on the very next tick after a failed send, since nudgeSentAt is now
// non-zero regardless of send outcome.
func TestAttemptBacklogNudge_FailedSend_RateLimitsRetry(t *testing.T) {
	inst := &Instance{
		Title:  "test-nudge-rate-limit",
		Status: Ready,
	}

	var nudgeSentAt time.Time
	idle := driverBacklogNudgeDelay + time.Minute

	// First tick: guard should be open (no nudge sent yet, idle past the delay).
	if !nudgeSentAt.IsZero() || idle <= driverBacklogNudgeDelay {
		t.Fatal("test setup invalid: nudge guard should be open before the first attempt")
	}
	nudgeSentAt = attemptBacklogNudge(inst, idle)

	// Second tick (simulating the very next driver poll, "idle" recomputed relative to the
	// just-set nudgeSentAt so it is effectively ~0): guard must now be closed even though
	// the send failed, or the driver would retry the identical send immediately — the bug.
	if nudgeSentAt.IsZero() && idle > driverBacklogNudgeDelay {
		t.Fatal("nudge guard re-opened immediately after a failed send — the driver would retry " +
			"the identical SendKeys call on every subsequent tick forever (BUG-041)")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ─── U-GO-01: TestSanitizeInitialPromptForTmux_stripsNullBytes ───────────────

func TestSanitizeInitialPromptForTmux_stripsNullBytes(t *testing.T) {
	input := "hello\x00world\x00"
	got := sanitizeInitialPromptForTmux(input)
	if strings.Contains(got, "\x00") {
		t.Errorf("sanitizeInitialPromptForTmux(%q) = %q, still contains null bytes", input, got)
	}
	if got != "helloworld" {
		t.Errorf("sanitizeInitialPromptForTmux(%q) = %q, want %q", input, got, "helloworld")
	}
}

// ─── U-GO-02: TestSanitizeInitialPromptForTmux_collapsesNewlines ─────────────

func TestSanitizeInitialPromptForTmux_collapsesNewlines(t *testing.T) {
	input := "line1\nline2\rline3"
	got := sanitizeInitialPromptForTmux(input)
	if strings.Contains(got, "\n") || strings.Contains(got, "\r") {
		t.Errorf("sanitizeInitialPromptForTmux(%q) = %q, still contains newlines", input, got)
	}
	// Newlines should be replaced with spaces
	if got != "line1 line2 line3" {
		t.Errorf("sanitizeInitialPromptForTmux(%q) = %q, want %q", input, got, "line1 line2 line3")
	}
}

// ─── U-GO-03: TestSanitizeInitialPromptForTmux_truncatesAt4096 ───────────────

func TestSanitizeInitialPromptForTmux_truncatesAt4096(t *testing.T) {
	input := strings.Repeat("a", 5000)
	got := sanitizeInitialPromptForTmux(input)
	if len(got) > 4096 {
		t.Errorf("sanitizeInitialPromptForTmux: len(got) = %d, want <= 4096", len(got))
	}
	if len(got) != 4096 {
		t.Errorf("sanitizeInitialPromptForTmux: len(got) = %d, want exactly 4096", len(got))
	}
}

// ─── U-GO-04: TestSanitizeInitialPromptForTmux_whitespaceOnlyFallsThrough ────

func TestSanitizeInitialPromptForTmux_whitespaceOnlyFallsThrough(t *testing.T) {
	input := "   \t  "
	got := sanitizeInitialPromptForTmux(input)
	if got != "" {
		t.Errorf("sanitizeInitialPromptForTmux(%q) = %q, want empty string (caller falls back to driverInitialPrompt)", input, got)
	}
}

// ─── U-GO-05: TestRunSessionDriver_usesInitialPromptWhenNonEmpty ─────────────
// These tests verify the runSessionDriver logic by inspecting the Instance struct
// rather than running a full driver loop (which requires tmux).
//
// NOTE (M-14): These tests re-implement the prompt-selection logic from runSessionDriver.
// If that logic changes, update these tests to match. The selection logic is a simple
// 4-line block in runSessionDriver that checks inst.InitialPrompt and calls
// sanitizeInitialPromptForTmux — extracting it to a standalone helper was judged
// too invasive given the minimal complexity.

func TestRunSessionDriver_selectsInitialPromptWhenNonEmpty(t *testing.T) {
	// Verify the selection logic directly: if InitialPrompt is set, it should
	// be used (after sanitization), not driverInitialPrompt.
	inst := &Instance{
		Title:         "test-custom-prompt",
		InitialPrompt: "do the thing",
		Status:        Stopped,
	}
	// Simulate the selection logic from runSessionDriver.
	initialPrompt := driverInitialPrompt
	if inst.InitialPrompt != "" {
		sanitized := sanitizeInitialPromptForTmux(inst.InitialPrompt)
		if sanitized != "" {
			initialPrompt = sanitized
		}
	}
	if initialPrompt != "do the thing" {
		t.Errorf("expected initialPrompt = %q, got %q", "do the thing", initialPrompt)
	}
}

// ─── U-GO-06: TestRunSessionDriver_fallsBackToStaticPromptWhenEmpty ──────────

func TestRunSessionDriver_fallsBackToStaticPromptWhenEmpty(t *testing.T) {
	inst := &Instance{
		Title:         "test-empty-prompt",
		InitialPrompt: "",
		Status:        Stopped,
	}
	initialPrompt := driverInitialPrompt
	if inst.InitialPrompt != "" {
		sanitized := sanitizeInitialPromptForTmux(inst.InitialPrompt)
		if sanitized != "" {
			initialPrompt = sanitized
		}
	}
	if initialPrompt != driverInitialPrompt {
		t.Errorf("expected driverInitialPrompt fallback, got %q", initialPrompt)
	}
}

// ─── U-GO-07: TestRunSessionDriver_fallsBackToStaticPromptWhenWhitespace ─────

func TestRunSessionDriver_fallsBackToStaticPromptWhenWhitespace(t *testing.T) {
	inst := &Instance{
		Title:         "test-whitespace-prompt",
		InitialPrompt: "   ",
		Status:        Stopped,
	}
	initialPrompt := driverInitialPrompt
	if inst.InitialPrompt != "" {
		sanitized := sanitizeInitialPromptForTmux(inst.InitialPrompt)
		if sanitized != "" {
			initialPrompt = sanitized
		}
	}
	if initialPrompt != driverInitialPrompt {
		t.Errorf("expected driverInitialPrompt fallback for whitespace-only InitialPrompt, got %q", initialPrompt)
	}
}

// ─── U-GO-08: TestSanitizeInitialPromptForTmux_utf8BoundaryNotSplit ───────────

func TestSanitizeInitialPromptForTmux_utf8BoundaryNotSplit(t *testing.T) {
	// Build a 4098-byte input: 4090 ASCII bytes + 2 emoji (😀 = 4 bytes each = 8 bytes total).
	// After truncation at 4096 bytes, the second emoji straddles the boundary (bytes 4093-4096).
	// The sanitizer must step back to a valid UTF-8 boundary.
	input := strings.Repeat("a", 4090) + "😀😀"
	if len(input) != 4098 {
		t.Fatalf("test setup: expected input length 4098, got %d", len(input))
	}

	got := sanitizeInitialPromptForTmux(input)

	if len(got) > 4096 {
		t.Errorf("sanitizeInitialPromptForTmux: len(got) = %d, want <= 4096", len(got))
	}
	if !utf8.ValidString(got) {
		t.Errorf("sanitizeInitialPromptForTmux: result is not valid UTF-8: %q", got[:min(len(got), 60)])
	}
}

// ─── TestParseClaudeSessionID ─────────────────────────────────────────────────

// TestParseClaudeSessionID_json verifies extraction from --output-format json output.
func TestParseClaudeSessionID_json(t *testing.T) {
	output := `{"result":"ok","session_id":"abc-123","total_cost_usd":0.003}`
	got := parseClaudeSessionID(output)
	if got != "abc-123" {
		t.Errorf("parseClaudeSessionID() = %q, want %q", got, "abc-123")
	}
}

// TestParseClaudeSessionID_streamJson verifies extraction from stream-json init event.
func TestParseClaudeSessionID_streamJson(t *testing.T) {
	output := `{"type":"system","subtype":"init","data":{"session_id":"xyz-789"}}`
	got := parseClaudeSessionID(output)
	if got != "xyz-789" {
		t.Errorf("parseClaudeSessionID() = %q, want %q", got, "xyz-789")
	}
}

// TestParseClaudeSessionID_empty verifies empty string returns "".
func TestParseClaudeSessionID_empty(t *testing.T) {
	got := parseClaudeSessionID("")
	if got != "" {
		t.Errorf("parseClaudeSessionID(\"\") = %q, want empty string", got)
	}
}

// TestParseClaudeSessionID_noSessionId verifies strings without session_id return "".
func TestParseClaudeSessionID_noSessionId(t *testing.T) {
	output := `{"result":"ok","cost":0.003}`
	got := parseClaudeSessionID(output)
	if got != "" {
		t.Errorf("parseClaudeSessionID(%q) = %q, want empty string", output, got)
	}
}

func readTestdata(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("detection", "testdata", name))
	if err != nil {
		t.Fatalf("readTestdata(%q): %v", name, err)
	}
	return string(data)
}

// TestOutputShowsConversationStarted verifies that live terminal output patterns
// reliably distinguish an active or completed conversation from a fresh session.
func TestOutputShowsConversationStarted(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   bool
	}{
		// ── positive: active processing ──────────────────────────────────────
		{
			name:   "active spinner with esc to interrupt",
			output: readTestdata(t, "claude_active.txt"),
			want:   true,
		},
		{
			name:   "asterism active spinner",
			output: readTestdata(t, "claude_asterism_active.txt"),
			want:   true,
		},
		{
			name:   "task manager active (✽ asterism + esc to interrupt)",
			output: readTestdata(t, "claude_active_task_manager.txt"),
			want:   true,
		},
		{
			name:   "thinking verb active",
			output: readTestdata(t, "claude_thinking_verb.txt"),
			want:   true,
		},
		// ── positive: completed/post-conversation states ───────────────────
		{
			name:   "asterism completion verb (past tense + for Xm)",
			output: readTestdata(t, "claude_asterism_success.txt"),
			want:   true,
		},
		{
			name:   "cost summary (⎿  $X.XX)",
			output: readTestdata(t, "claude_cost_summary.txt"),
			want:   true,
		},
		{
			name:   "baked idle with ◉ Baked for marker",
			output: readTestdata(t, "claude_baked_idle.txt"),
			want:   true,
		},
		// ── positive: inline signals ──────────────────────────────────────
		{
			name:   "esc to interrupt inline",
			output: "Some output\nesc to interrupt\n",
			want:   true,
		},
		{
			name:   "spinner time suffix only",
			output: "✻ Wandering... (2m 3s · ↑ 1.2k tokens)\n> ▌\n",
			want:   true,
		},
		{
			name:   "cost summary only",
			output: "⎿  $0.18 · 5 tool uses · 800 tokens\n",
			want:   true,
		},
		{
			name:   "baked marker",
			output: "◉ Baked for 30s\n> ▌\n",
			want:   true,
		},
		{
			name:   "resuming marker",
			output: "◉ Claude resuming /loop wakeup (May 2 11:55pm)\n",
			want:   true,
		},
		// ── negative: no conversation started ────────────────────────────
		{
			name:   "empty string",
			output: "",
			want:   false,
		},
		{
			name:   "bare readline prompt",
			output: ">\n? for shortcuts\n",
			want:   false,
		},
		{
			name:   "startup trust dialog",
			output: "Quick safety check: Is this a project you created or one you trust?\n 1. Yes, I trust this folder\n 2. No, exit",
			want:   false,
		},
		{
			name:   "plain text mentioning esc (not the sentinel phrase)",
			output: "Press esc key to cancel the operation\n",
			want:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := outputShowsConversationStarted(tc.output)
			if got != tc.want {
				t.Errorf("outputShowsConversationStarted(...) = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestScanTerminalForPRURL(t *testing.T) {
	cases := []struct {
		name      string
		output    string
		wantURL   string
		wantPRNum int
	}{
		{
			name: "git push output with PR create link",
			output: `remote: Create a pull request for 'feat/my-feature' on GitHub by visiting:
remote:      https://github.com/tstapler/stapler-squad/pull/128
remote:`,
			wantURL:   "https://github.com/tstapler/stapler-squad/pull/128",
			wantPRNum: 128,
		},
		{
			name: "git push output with PR update link",
			output: `To github.com:tstapler/stapler-squad.git
   5353e50..abcd123  feat/my-feature -> feat/my-feature
remote: https://github.com/tstapler/stapler-squad/pull/42`,
			wantURL:   "https://github.com/tstapler/stapler-squad/pull/42",
			wantPRNum: 42,
		},
		{
			name:      "no PR URL in output",
			output:    `remote: Resolving deltas: 100% (3/3), done.`,
			wantURL:   "",
			wantPRNum: 0,
		},
		{
			name:      "empty output",
			output:    "",
			wantURL:   "",
			wantPRNum: 0,
		},
		{
			name:      "URL with trailing punctuation stripped",
			output:    `See: https://github.com/tstapler/stapler-squad/pull/99.`,
			wantURL:   "https://github.com/tstapler/stapler-squad/pull/99",
			wantPRNum: 99,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotURL, gotNum := scanTerminalForPRURL(tc.output)
			if gotURL != tc.wantURL || gotNum != tc.wantPRNum {
				t.Errorf("scanTerminalForPRURL() = (%q, %d), want (%q, %d)",
					gotURL, gotNum, tc.wantURL, tc.wantPRNum)
			}
		})
	}
}
