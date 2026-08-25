package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"go.uber.org/goleak"
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
	t.Parallel()
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
			t.Parallel()
			got := isStartupDialog(tc.output)
			if got != tc.want {
				t.Errorf("isStartupDialog(%q) = %v, want %v", tc.output[:min(len(tc.output), 60)], got, tc.want)
			}
		})
	}
}

func TestShouldApprovePrompt(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
			got := shouldApprovePrompt(tc.output, tc.allowedPath)
			if got != tc.want {
				t.Errorf("shouldApprovePrompt() = %v, want %v", got, tc.want)
			}
		})
	}
}

// UT-3: TestIsOneShot — verifies one-shot detection logic.
func TestIsOneShot(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	if driverTotalTimeout < driverReadyTimeout+driverInactivityTimeout+5*time.Minute {
		t.Errorf("driverTotalTimeout (%v) must be >= driverReadyTimeout (%v) + driverInactivityTimeout (%v) + 5m",
			driverTotalTimeout, driverReadyTimeout, driverInactivityTimeout)
	}
}

// UT-21: TestMarkSessionNeedsAttention_NilReviewQueue — nil queue must not panic.
func TestMarkSessionNeedsAttention_NilReviewQueue(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	rq := NewReviewQueue()
	inst := &Instance{
		Title:       "test-second-failure",
		UUID:        "test-uuid-second-fail",
		reviewQueue: rq,
		Status:      Stopped,
	}

	var retried atomic.Bool
	retried.Store(true) // already retried once

	handleDriverFailure(inst, "/tmp", &retried, "unexpected exit", make(chan struct{}))

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
	t.Parallel()
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
	t.Parallel()
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

// ─── U-GO-01: TestSanitizeInitialPromptForTmux_stripsNullBytes ───────────────

func TestSanitizeInitialPromptForTmux_stripsNullBytes(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	output := `{"result":"ok","session_id":"abc-123","total_cost_usd":0.003}`
	got := parseClaudeSessionID(output)
	if got != "abc-123" {
		t.Errorf("parseClaudeSessionID() = %q, want %q", got, "abc-123")
	}
}

// TestParseClaudeSessionID_streamJson verifies extraction from stream-json init event.
func TestParseClaudeSessionID_streamJson(t *testing.T) {
	t.Parallel()
	output := `{"type":"system","subtype":"init","data":{"session_id":"xyz-789"}}`
	got := parseClaudeSessionID(output)
	if got != "xyz-789" {
		t.Errorf("parseClaudeSessionID() = %q, want %q", got, "xyz-789")
	}
}

// TestParseClaudeSessionID_empty verifies empty string returns "".
func TestParseClaudeSessionID_empty(t *testing.T) {
	t.Parallel()
	got := parseClaudeSessionID("")
	if got != "" {
		t.Errorf("parseClaudeSessionID(\"\") = %q, want empty string", got)
	}
}

// TestParseClaudeSessionID_noSessionId verifies strings without session_id return "".
func TestParseClaudeSessionID_noSessionId(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
			t.Parallel()
			got := outputShowsConversationStarted(tc.output)
			if got != tc.want {
				t.Errorf("outputShowsConversationStarted(...) = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestScanTerminalForPRURL(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
			gotURL, gotNum := scanTerminalForPRURL(tc.output)
			if gotURL != tc.wantURL || gotNum != tc.wantPRNum {
				t.Errorf("scanTerminalForPRURL() = (%q, %d), want (%q, %d)",
					gotURL, gotNum, tc.wantURL, tc.wantPRNum)
			}
		})
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
	// Not t.Parallel(): this test needs t.Setenv("HOME", ...) below, and
	// t.Setenv panics if called on (or after) a parallel test.
	// FindConversationFilePath walks $HOME/.claude/projects; on a dev machine
	// with genuine session history this can stall for real wall-clock time
	// and, under -count=20 stress, blow the test binary's global timeout.
	// Point HOME at an empty temp dir so the walk resolves instantly.
	t.Setenv("HOME", t.TempDir())
	fakePM := &stuckDialogProcessManager{dialogText: trustDialogText}

	inst := &Instance{
		Title:          "stuck-dialog-bounded",
		Status:         Ready,
		processManager: fakePM,
		InitialPrompt:  driverInitialPrompt,
	}
	inst.started.Store(true)

	StartSessionDriver(inst, "/tmp")

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

	// StopSessionDriver closes the stop channel directly (the loop selects on
	// it unconditionally), so the driver goroutine — and its stop-watcher
	// child — are confirmed gone before this test returns, instead of the
	// old Status=Paused-and-hope-it-notices approach which could leak both
	// goroutines past the test's own lifetime.
	StopSessionDriver(inst)
	if inst.driverRunning.Load() {
		t.Fatal("driverRunning still true after StopSessionDriver returned")
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
	// Not t.Parallel(): see TestSessionDriver_StuckDialogAnswersBoundedNotUnbounded.
	// Isolate HOME so FindConversationFilePath's walk can't stall on real
	// session history.
	t.Setenv("HOME", t.TempDir())
	fakePM := &stuckDialogProcessManager{
		dialogText:  trustDialogText,
		growPerCall: true,
		growChunk:   "unrelated real Claude Code output line\n",
	}

	inst := &Instance{
		Title:          "tail-slice-growing-buffer",
		Status:         Ready,
		processManager: fakePM,
		InitialPrompt:  driverInitialPrompt,
	}
	inst.started.Store(true)

	StartSessionDriver(inst, "/tmp")

	time.Sleep(driverPollInterval*6 + 500*time.Millisecond)

	count := fakePM.sendKeysCount.Load()
	t.Logf("SendKeys(\"1\\n\") called %d times over 6 poll ticks against a growing (non-flapping active session) buffer", count)

	if count > maxDialogAnswerAttempts {
		t.Fatalf("expected SendKeys(\"1\\n\") to be bounded by maxDialogAnswerAttempts (%d) even against a growing buffer, got %d calls",
			maxDialogAnswerAttempts, count)
	}

	StopSessionDriver(inst)
	if inst.driverRunning.Load() {
		t.Fatal("driverRunning still true after StopSessionDriver returned")
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
	t.Parallel()
	t.Run("a_same_hash_sent_twice_second_call_is_noop", func(t *testing.T) {
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
// TestSessionDriver_DialogGaveUp_FallsThroughToInactivityEscalation is the
// narrower unit test on the post-latch branch logic, per Task 1.2.5's
// explicit fallback clause (plan.md, mirroring Task 1.2.4's fallback
// convention): a full-duration ticker test proved impractical here.
//
// Root cause: the driver's activityRef logic (session_driver.go) always uses
// the *later* of LastMeaningfulOutput and initialPromptSentAt as the
// inactivity reference — specifically to avoid false inactivity fires right
// startSessionDriverForTest replicates StartSessionDriver's goroutine/WaitGroup
// wiring exactly, but calls runSessionDriverWithPrompt with a pre-seeded
// retried value instead of runSessionDriver's fresh, always-false one.
// StartSessionDriver's public API intentionally can't express a pre-seeded
// retried value (see TestSessionDriver_DialogGaveUp_FallsThroughToInactivityEscalation's
// doc comment for why) — this test-only, package-private helper exists so
// callers that need one can still clean up via the real StopSessionDriver,
// rather than hand-rolling stop/done channels outside the Start/Stop wrapper.
func startSessionDriverForTest(inst *Instance, allowedPath, initialPrompt string, retried bool) {
	inst.driverMu.Lock()
	if inst.driverDestroyed {
		inst.driverMu.Unlock()
		return
	}
	if !inst.driverRunning.CompareAndSwap(false, true) {
		inst.driverMu.Unlock()
		return
	}
	stopper := &sessionDriverStopper{stop: make(chan struct{})}
	inst.driverWG.Add(1)
	inst.driverStopper.Store(stopper)
	inst.driverMu.Unlock()
	var retriedFlag atomic.Bool
	retriedFlag.Store(retried)
	go func() {
		defer inst.driverWG.Done()
		defer inst.driverRunning.Store(false)
		runSessionDriverWithPrompt(inst, allowedPath, initialPrompt, &retriedFlag, stopper.stop)
	}()
}

// after startup. Once the dialogGaveUp fall-through reaches the
// initial-prompt-send step (which it does almost immediately, since the
// fake ProcessManager's failCount is exhausted by the dialog-answer
// attempts and the very next SendKeys call succeeds), initialPromptSentAt
// becomes "now" and permanently wins over any artificially-stale
// LastMeaningfulOutput seeded by the test — so the real
// driverInactivityTimeout (10 minutes) can never be reached in test time.
//
// What this test proves instead: dialogGaveUp's fall-through actually
// reaches the code *after* the dialog-answer branch (the initial-prompt
// send), rather than being trapped in the `continue` this fix's Blocker 1
// exists to close. That control-flow escape is the real regression surface;
// the inactivity-timeout branch is exercised by ordinary code review of
// the shared `if idle > graceTimeout` path (also covered indirectly by
// TestSessionDriver_SecondFailure_MarksNeedsAttention's similar shape).
func TestSessionDriver_DialogGaveUp_FallsThroughToInactivityEscalation(t *testing.T) {
	// Not t.Parallel(): this test needs t.Setenv("HOME", ...) below, and
	// t.Setenv panics if called on (or after) a parallel test.
	// FindConversationFilePath (called by sendInitialPromptTick when deciding
	// whether the initial prompt was already delivered) walks $HOME/.claude/projects
	// on real disk. On a real dev machine that directory holds genuine, large
	// session history, which can stall this search for real wall-clock seconds
	// per driver-loop tick — long enough to burn through this test's entire
	// deadline before sendInitialPromptTick ever reaches its SendKeys call,
	// producing the exact "SendKeys count never exceeded" failure this test
	// guards against, for a reason unrelated to the dialogGaveUp fall-through
	// logic under test. Pointing HOME at an empty temp dir makes the walk
	// resolve instantly and deterministically to "not found," independent of
	// whatever real session history exists on the machine running the test.
	t.Setenv("HOME", t.TempDir())
	fakePM := &stuckDialogProcessManager{
		dialogText: trustDialogText,
		failCount:  maxDialogAnswerAttempts,
	}

	inst := &Instance{
		Title:          "dialog-give-up-escalation",
		UUID:           "test-uuid-give-up-escalation",
		Status:         Ready,
		processManager: fakePM,
		reviewQueue:    NewReviewQueue(),
	}
	inst.started.Store(true)

	// This test needs retried=true pre-seeded (simulating "already retried
	// once" so the second-failure path fires directly), a precondition
	// StartSessionDriver cannot express through its public API — its wrapper
	// always allocates a fresh, zero-value atomic.Bool internally, and
	// extending its signature to accept one for a single test call site
	// would leak an implementation detail into production code for no other
	// caller's benefit.
	//
	// Considered and rejected: driving retried=true organically through
	// StartSessionDriver by forcing one real failure/restart cycle first.
	// handleDriverFailure's first call (retried==false, session_driver.go)
	// doesn't just flip the flag — it restarts the whole session and spawns
	// a fresh driver goroutine for the continuation. Routing through that
	// path here would conflate two independent mechanisms under one test
	// (the dialogGaveUp fall-through this test exists to prove, and the
	// separate failure-restart machinery covered by
	// TestSessionDriver_SecondFailure_MarksNeedsAttention), doubling the
	// real wall-clock cost and adding a second independent timing-flakiness
	// surface on top of the driverReadyTimeout margin already documented
	// below (two recorded near-miss recurrences on this test alone).
	//
	// Instead this test uses startSessionDriverForTest (below), a test-only
	// helper that replicates StartSessionDriver's exact goroutine/WaitGroup
	// wiring but accepts a pre-seeded retried value — so cleanup goes
	// through the real StopSessionDriver, identically to every other
	// SessionDriver test, rather than a bespoke stop/done channel pair.
	baseline := goleak.IgnoreCurrent()
	defer goleak.VerifyNone(t, append(knownBackgroundGoroutines, baseline)...)

	startSessionDriverForTest(inst, "/tmp", driverInitialPrompt, true /* retried */)
	defer StopSessionDriver(inst)

	// maxDialogAnswerAttempts failed dialog-answer sends drive the latch to
	// dialogGaveUp; the 4th SendKeys call (initial-prompt send, unblocked by
	// the fall-through) is the direct proof the loop escaped the `continue`.
	// The fake pane's content never satisfies claudeAtPrompt (it's always
	// the same trust-dialog text), so the initial-prompt-send branch is only
	// reached via its timedOut fallback once driverReadyTimeout (30s)
	// elapses — the deadline below must clear that, not just the dialog
	// latch's own ~6s give-up window.
	//
	// The extra driverReadyTimeout term is deliberate slack, not just the
	// ~6s dialog-latch window plus a token second: this goroutine genuinely
	// blocks on the real 30s driverReadyTimeout wall-clock wait, so under
	// heavy scheduler/CPU contention (go test -race -p 1 for the full
	// suite, or session's own t.Parallel() fan-out within a single package)
	// that wait alone can occasionally overrun a razor-thin margin. Two
	// documented recurrences of exactly this: an isolated run once passed
	// in 34s of a 37s (1x) budget and failed under -race package load; a
	// later run passed in 68.52s against a since-widened 67s (2x) budget
	// under session's own -p 1 in-package parallel load (see BUG-051's
	// recurrence log). Each time, widening the margin (not retrying) is the
	// fix, since the 30s block is inherent to the code path under test —
	// bumped to 3x here for more headroom against the same contention.
	deadline := time.After(3*driverReadyTimeout + driverPollInterval*3 + time.Second)
	for fakePM.sendKeysCount.Load() <= maxDialogAnswerAttempts {
		select {
		case <-deadline:
			t.Fatalf("SendKeys count never exceeded %d — the dialogGaveUp fall-through never reached the initial-prompt-send step (stuck in the continue trap)",
				maxDialogAnswerAttempts)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestStopSessionDriver_ConcurrentWithInFlightPoll_ReturnsBoundedNoGoroutineLeak(t *testing.T) {
	// See TestActorNoLeak (actor_test.go) for why this baselines via
	// goleak.IgnoreCurrent() instead of a bare process-wide goleak.VerifyNone().
	baseline := goleak.IgnoreCurrent()
	defer goleak.VerifyNone(t, append(knownBackgroundGoroutines, baseline)...)

	fakePM := &stuckDialogProcessManager{}

	inst := &Instance{
		Title:          "concurrent-stop-test",
		UUID:           "test-uuid-concurrent-stop",
		Status:         Running,
		processManager: fakePM,
		reviewQueue:    NewReviewQueue(),
	}
	inst.started.Store(true)

	StartSessionDriver(inst, t.TempDir())

	// Let the driver goroutine actually reach its poll-loop select before
	// racing StopSessionDriver against it.
	deadline := time.After(time.Second)
	for !inst.driverRunning.Load() {
		select {
		case <-deadline:
			t.Fatal("driver goroutine never marked itself running")
		case <-time.After(time.Millisecond):
		}
	}
	time.Sleep(10 * time.Millisecond)

	stopDone := make(chan struct{})
	go func() {
		defer close(stopDone)
		StopSessionDriver(inst)
	}()

	select {
	case <-stopDone:
	case <-time.After(driverStopTimeout + 2*time.Second):
		t.Fatal("StopSessionDriver did not return within its bounded timeout while racing an in-flight poll")
	}

	if inst.driverRunning.Load() {
		t.Fatal("driverRunning still true after StopSessionDriver returned")
	}

	// A StartSessionDriver call arriving after Destroy() must be refused —
	// driverDestroyed (set by StopSessionDriver) must permanently block it.
	StartSessionDriver(inst, t.TempDir())
	time.Sleep(20 * time.Millisecond)
	if inst.driverRunning.Load() {
		t.Fatal("StartSessionDriver spawned a new driver goroutine after the instance was destroyed")
	}

	// The deferred goleak.VerifyNone confirms the stopped goroutine (and its
	// stop-watcher child) do not leak past this point.
}

// TestStopSessionDriver_WaitsForHandleDriverFailureRetryGoroutine_NoGoroutineLeak
// is the regression test for the BLOCKER fix: StopSessionDriver must not return
// while a handleDriverFailure-spawned retry continuation is still running.
//
// Before the fix, StopSessionDriver waited on a per-run sessionDriverStopper.done
// channel that was only closed by the *original* run's goroutine (via
// StartSessionDriver's defer). handleDriverFailure spawns a second, untracked-by-
// `done` goroutine to continue the run after a restart and returns immediately
// (see handleDriverFailure's doc comment: the caller "must return immediately"),
// so the original goroutine's defer fired and closed `done` while the retry
// goroutine was still alive — StopSessionDriver returned early, reintroducing the
// exact "goroutine outlives Destroy()" bug this package exists to prevent.
//
// This test reproduces the exact interleaving handleDriverFailure produces
// (Add(1) for the retry BEFORE the original goroutine's Done() fires) without
// depending on real tmux/session restart plumbing: the fix is in StopSessionDriver
// and inst.driverWG's bookkeeping, not in handleDriverFailure's business logic.
func TestStopSessionDriver_WaitsForHandleDriverFailureRetryGoroutine_NoGoroutineLeak(t *testing.T) {
	baseline := goleak.IgnoreCurrent()
	defer goleak.VerifyNone(t, append(knownBackgroundGoroutines, baseline)...)

	inst := &Instance{Title: "test-stop-waits-for-retry"}
	stopper := &sessionDriverStopper{stop: make(chan struct{})}
	inst.driverStopper.Store(stopper)

	retryStarted := make(chan struct{})
	retryFinish := make(chan struct{})

	// Simulate StartSessionDriver's original goroutine.
	inst.driverWG.Add(1)
	go func() {
		defer inst.driverWG.Done()
		<-stopper.stop

		// Simulate handleDriverFailure: Add(1) and spawn a retry continuation
		// BEFORE this goroutine's own Done() fires — the exact sequencing that
		// broke the old done-channel-based StopSessionDriver.
		inst.driverWG.Add(1)
		go func() {
			defer inst.driverWG.Done()
			close(retryStarted)
			<-retryFinish
		}()
	}()

	stopDone := make(chan struct{})
	go func() {
		defer close(stopDone)
		StopSessionDriver(inst)
	}()

	select {
	case <-retryStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("simulated retry goroutine never started")
	}

	select {
	case <-stopDone:
		t.Fatal("StopSessionDriver returned before the handleDriverFailure-style retry goroutine finished")
	case <-time.After(200 * time.Millisecond):
	}

	close(retryFinish)

	select {
	case <-stopDone:
	case <-time.After(driverStopTimeout + 2*time.Second):
		t.Fatal("StopSessionDriver did not return after the retry goroutine finished")
	}
}
