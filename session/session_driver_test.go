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
func (m *stuckDialogProcessManager) HasLiveSessionNoCache() bool         { return true }
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

// stringOutputWantCase is the shared case shape for simple func(output
// string) bool predicate tests (isStartupDialog, outputShowsConversationStarted).
type stringOutputWantCase struct {
	name   string
	output string
	want   bool
}

// isStartupDialogPositiveCases covers outputs isStartupDialog must recognize
// as the startup trust-folder dialog.
func isStartupDialogPositiveCases() []stringOutputWantCase {
	return []stringOutputWantCase{
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
	}
}

// isStartupDialogNegativeCases covers outputs that must NOT be recognized as
// the startup dialog.
func isStartupDialogNegativeCases() []stringOutputWantCase {
	return []stringOutputWantCase{
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
}

func TestIsStartupDialog(t *testing.T) {
	t.Parallel()
	cases := append(isStartupDialogPositiveCases(), isStartupDialogNegativeCases()...)
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

// shouldApprovePromptPathCases covers the "Allow reading/writing in <path>"
// output shape, gated on whether the path falls under allowedPath.
func shouldApprovePromptPathCases() []struct {
	name        string
	output      string
	allowedPath string
	want        bool
} {
	return []struct {
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
	}
}

// shouldApprovePromptGenericCases covers the "Do you want to proceed?" shape
// and unrelated output, independent of the path-scoped Allow reading/writing
// prompts above.
func shouldApprovePromptGenericCases() []struct {
	name        string
	output      string
	allowedPath string
	want        bool
} {
	return []struct {
		name        string
		output      string
		allowedPath string
		want        bool
	}{
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
}

func TestShouldApprovePrompt(t *testing.T) {
	t.Parallel()
	cases := append(shouldApprovePromptPathCases(), shouldApprovePromptGenericCases()...)

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

// UT-17: TestSessionDriver_SecondFailure_MarksNeedsAttention — a failure with
// an already-exhausted retry budget transitions to PermanentlyFailed and adds
// to the ReviewQueue (re-derived for the RetryState/RetryPolicy machinery per
// .claude/rules/fix-flaky-tests-dont-defer.md's spirit: the equivalent
// assertion, not a relaxed one — this test previously simulated "already
// retried once" via a bare atomic.Bool; the same intent is now expressed as
// RetryState already at its resolved cap).
func TestSessionDriver_SecondFailure_MarksNeedsAttention(t *testing.T) {
	t.Parallel()
	rq := NewReviewQueue()
	inst := &Instance{
		Title:       "test-second-failure",
		UUID:        "test-uuid-second-fail",
		reviewQueue: rq,
		Status:      Stopped,
	}
	inst.RetryAttempt = 1
	inst.RetryMaxAttempts = 1 // already at cap — next failure is exhausted

	policy := RetryPolicy{Enabled: true, MaxAttempts: 1, RetryOn: []string{"crashed", "stalled", "tmux_exited"}}
	handleDriverFailure(inst, "/tmp", policy, "unexpected exit", make(chan struct{}))

	if inst.Status != PermanentlyFailed {
		t.Errorf("Status = %v, want PermanentlyFailed", inst.Status)
	}

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

// resolveInitialPromptForTest re-implements the prompt-selection logic from
// runSessionDriver (see NOTE M-14 above) so the three prompt-selection tests
// below share one copy instead of repeating it.
func resolveInitialPromptForTest(inst *Instance) string {
	initialPrompt := driverInitialPrompt
	if inst.InitialPrompt != "" {
		sanitized := sanitizeInitialPromptForTmux(inst.InitialPrompt)
		if sanitized != "" {
			initialPrompt = sanitized
		}
	}
	return initialPrompt
}

func TestRunSessionDriver_selectsInitialPromptWhenNonEmpty(t *testing.T) {
	t.Parallel()
	// Verify the selection logic directly: if InitialPrompt is set, it should
	// be used (after sanitization), not driverInitialPrompt.
	inst := &Instance{
		Title:         "test-custom-prompt",
		InitialPrompt: "do the thing",
		Status:        Stopped,
	}
	if initialPrompt := resolveInitialPromptForTest(inst); initialPrompt != "do the thing" {
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
	if initialPrompt := resolveInitialPromptForTest(inst); initialPrompt != driverInitialPrompt {
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
	if initialPrompt := resolveInitialPromptForTest(inst); initialPrompt != driverInitialPrompt {
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
// outputShowsConversationStartedActiveCases covers live/active-processing
// output shapes (spinners, "esc to interrupt", thinking verbs).
func outputShowsConversationStartedActiveCases(t *testing.T) []stringOutputWantCase {
	t.Helper()
	return []stringOutputWantCase{
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
	}
}

// outputShowsConversationStartedCompletedCases covers completed/post-
// conversation output shapes (past-tense completion verbs, cost summaries).
func outputShowsConversationStartedCompletedCases(t *testing.T) []stringOutputWantCase {
	t.Helper()
	return []stringOutputWantCase{
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
	}
}

// outputShowsConversationStartedInlineCases covers positive inline signals
// that don't need testdata fixtures.
func outputShowsConversationStartedInlineCases() []stringOutputWantCase {
	return []stringOutputWantCase{
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
	}
}

// outputShowsConversationStartedNegativeCases covers output shapes where no
// conversation has started.
func outputShowsConversationStartedNegativeCases() []stringOutputWantCase {
	return []stringOutputWantCase{
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
}

func TestOutputShowsConversationStarted(t *testing.T) {
	t.Parallel()
	cases := append(outputShowsConversationStartedActiveCases(t), outputShowsConversationStartedCompletedCases(t)...)
	cases = append(cases, outputShowsConversationStartedInlineCases()...)
	cases = append(cases, outputShowsConversationStartedNegativeCases()...)

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

// prURLCase is the shared case shape for TestScanTerminalForPRURL.
type prURLCase struct {
	name      string
	output    string
	wantURL   string
	wantPRNum int
}

// scanTerminalForPRURLMatchCases covers output containing a GitHub PR URL.
func scanTerminalForPRURLMatchCases() []prURLCase {
	return []prURLCase{
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
			name:      "URL with trailing punctuation stripped",
			output:    `See: https://github.com/tstapler/stapler-squad/pull/99.`,
			wantURL:   "https://github.com/tstapler/stapler-squad/pull/99",
			wantPRNum: 99,
		},
	}
}

// scanTerminalForPRURLNoMatchCases covers output with no GitHub PR URL.
func scanTerminalForPRURLNoMatchCases() []prURLCase {
	return []prURLCase{
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
	}
}

func TestScanTerminalForPRURL(t *testing.T) {
	t.Parallel()
	cases := append(scanTerminalForPRURLMatchCases(), scanTerminalForPRURLNoMatchCases()...)
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

// runBoundedDialogAnswerScenario starts a real session driver against fakePM,
// waits 6 poll ticks (double Phase 0's original 3-tick reproduction window),
// and asserts SendKeys("1\n") never exceeds maxDialogAnswerAttempts — the
// shared body of the stuck-buffer and growing-buffer regression tests below.
func runBoundedDialogAnswerScenario(t *testing.T, title string, fakePM *stuckDialogProcessManager, logMsg string) {
	t.Helper()
	inst := &Instance{
		Title:          title,
		Status:         Ready,
		processManager: fakePM,
		InitialPrompt:  driverInitialPrompt,
	}
	inst.started.Store(true)

	StartSessionDriver(inst, "/tmp")
	time.Sleep(driverPollInterval*6 + 500*time.Millisecond)

	count := fakePM.sendKeysCount.Load()
	t.Logf(logMsg, count)
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

// TestSessionDriver_StuckDialogAnswersBoundedNotUnbounded is the permanent
// regression proof for AC2, replacing phase0_repro_test.go's now-obsolete
// "expect repeated sends" assertion. It runs the REAL (now-fixed)
// runSessionDriverWithPrompt goroutine for several poll ticks against a fake
// ProcessManager whose visible content never changes (the same flapping
// condition Phase 0 used) and asserts SendKeys("1\n") is observed at most
// maxDialogAnswerAttempts times, never growing with additional ticks.
func TestSessionDriver_StuckDialogAnswersBoundedNotUnbounded(t *testing.T) {
	// Not t.Parallel(): t.Setenv panics on a parallel test, and HOME must be
	// isolated so FindConversationFilePath's walk of $HOME/.claude/projects
	// can't stall on real session history.
	t.Setenv("HOME", t.TempDir())
	fakePM := &stuckDialogProcessManager{dialogText: trustDialogText}
	runBoundedDialogAnswerScenario(t, "stuck-dialog-bounded", fakePM,
		"SendKeys(\"1\\n\") called %d times over 6 poll ticks against an unchanging stuck-dialog buffer")
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
	runBoundedDialogAnswerScenario(t, "tail-slice-growing-buffer", fakePM,
		"SendKeys(\"1\\n\") called %d times over 6 poll ticks against a growing (non-flapping active session) buffer")
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

// answerDialogOnceCaseSameHashIsNoop is TestAnswerDialogOnce case (a): a
// second call with an unchanged hash must not resend.
func answerDialogOnceCaseSameHashIsNoop(t *testing.T) {
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
}

// answerDialogOnceCaseHashChangeResends is TestAnswerDialogOnce case (b): a
// genuinely different dialog must be answered again.
func answerDialogOnceCaseHashChangeResends(t *testing.T) {
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
}

// answerDialogOnceCaseGivesUpAfterMaxFailures is TestAnswerDialogOnce case
// (c): send failing maxDialogAnswerAttempts times gives up and stays given up.
func answerDialogOnceCaseGivesUpAfterMaxFailures(t *testing.T) {
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
}

// answerDialogOnceCaseRecoversAfterOneFailure is TestAnswerDialogOnce case
// (d): send failing once then succeeding reaches dialogAwaitingDismissal.
func answerDialogOnceCaseRecoversAfterOneFailure(t *testing.T) {
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
}

// answerDialogOnceCaseWhitespaceJitterUnchanged is TestAnswerDialogOnce case
// (e): terminal-width-driven line-wrap jitter between ticks must not be
// treated as a new dialog.
func answerDialogOnceCaseWhitespaceJitterUnchanged(t *testing.T) {
	t.Parallel()
	// Same logical dialog text, but re-wrapped at a different column width
	// with different internal newline placement and trailing spaces.
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
}

// answerDialogOnceCaseGrowingBufferWithinTailWindowUnchanged is
// TestAnswerDialogOnce case (f): the dialog text stays fixed at the tail of
// output across both calls while unrelated content grows ahead of it
// (simulating a growing PTY buffer); once both totals exceed
// statusDetectionTailBytes, tailContent clips the growing part away on both
// calls — see buildGrowingPrefixContent's doc comment for why the resulting
// tail is byte-identical despite the raw buffer growing.
func answerDialogOnceCaseGrowingBufferWithinTailWindowUnchanged(t *testing.T) {
	t.Parallel()
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
}

// answerDialogOnceCaseDialogOutsideTailWindowNeverReached is
// TestAnswerDialogOnce case (g), companion to (f): enough unrelated content
// follows the dialog text that it falls entirely outside the tail window —
// proving isStartupDialog on the tailed content correctly stops matching
// (the dialog is treated as "no longer on screen", not as "a new dialog"),
// so the call site never invokes answerDialogOnce for this tick at all.
func answerDialogOnceCaseDialogOutsideTailWindowNeverReached(t *testing.T) {
	t.Parallel()
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
}

func TestAnswerDialogOnce(t *testing.T) {
	t.Parallel()
	t.Run("a_same_hash_sent_twice_second_call_is_noop", answerDialogOnceCaseSameHashIsNoop)
	t.Run("b_hash_changes_between_calls_resends", answerDialogOnceCaseHashChangeResends)
	t.Run("c_send_fails_maxDialogAnswerAttempts_times_gives_up_and_stays_given_up", answerDialogOnceCaseGivesUpAfterMaxFailures)
	t.Run("d_send_fails_once_then_succeeds_reaches_awaiting_dismissal", answerDialogOnceCaseRecoversAfterOneFailure)
	t.Run("e_whitespace_and_line_wrap_jitter_recognized_as_unchanged", answerDialogOnceCaseWhitespaceJitterUnchanged)
	t.Run("f_growing_buffer_within_tail_window_recognized_as_unchanged", answerDialogOnceCaseGrowingBufferWithinTailWindowUnchanged)
	t.Run("g_dialog_pushed_fully_outside_tail_window_never_reached", answerDialogOnceCaseDialogOutsideTailWindowNeverReached)
}

// startSessionDriverForTest replicates StartSessionDriver's goroutine/WaitGroup
// wiring exactly, but calls runSessionDriverWithPrompt with a caller-supplied
// RetryPolicy instead of runSessionDriver's config-resolved one.
// StartSessionDriver's public API intentionally can't express a pre-resolved
// policy (see TestSessionDriver_DialogGaveUp_FallsThroughToInactivityEscalation's
// doc comment for why) — this test-only, package-private helper exists so
// callers that need one can still clean up via the real StopSessionDriver,
// rather than hand-rolling stop/done channels outside the Start/Stop wrapper.
func startSessionDriverForTest(inst *Instance, allowedPath, initialPrompt string, policy RetryPolicy) {
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
	go func() {
		defer inst.driverWG.Done()
		defer inst.driverRunning.Store(false)
		runSessionDriverWithPrompt(inst, allowedPath, initialPrompt, policy, stopper.stop)
	}()
}

// TestSessionDriver_DialogGaveUp_FallsThroughToInactivityEscalation proves
// that once the startup-dialog latch reaches dialogGaveUp, the driver loop
// falls through to the code after the dialog-answer branch (the
// initial-prompt send) instead of getting stuck in the `continue` — the
// regression this test guards against.
//
// It can't also exercise the inactivity-timeout escalation that follows:
// activityRef always uses the later of LastMeaningfulOutput and
// initialPromptSentAt, so once the fall-through sends the initial prompt,
// initialPromptSentAt becomes "now" and permanently outweighs the
// artificially-stale LastMeaningfulOutput seeded below. So this test proves
// only the control-flow escape (via SendKeys count); the inactivity-timeout
// branch itself is covered by TestSessionDriver_SecondFailure_MarksNeedsAttention.
// newDialogGiveUpEscalationInstance builds the Instance + RetryPolicy fixture
// for TestSessionDriver_DialogGaveUp_FallsThroughToInactivityEscalation:
// RetryAttempt already at RetryMaxAttempts simulates "already retried once"
// so the second-failure path fires directly.
func newDialogGiveUpEscalationInstance(fakePM *stuckDialogProcessManager) (*Instance, RetryPolicy) {
	inst := &Instance{
		Title:          "dialog-give-up-escalation",
		UUID:           "test-uuid-give-up-escalation",
		Status:         Ready,
		processManager: fakePM,
		reviewQueue:    NewReviewQueue(),
	}
	inst.started.Store(true)
	inst.RetryAttempt = 1
	inst.RetryMaxAttempts = 1
	policy := RetryPolicy{Enabled: true, MaxAttempts: 1, RetryOn: []string{"crashed", "stalled", "tmux_exited"}}
	return inst, policy
}

func TestSessionDriver_DialogGaveUp_FallsThroughToInactivityEscalation(t *testing.T) {
	// Not t.Parallel(): t.Setenv panics on a parallel test, and HOME must be
	// isolated so FindConversationFilePath's walk can't stall on real session
	// history.
	t.Setenv("HOME", t.TempDir())
	fakePM := &stuckDialogProcessManager{
		dialogText: trustDialogText,
		failCount:  maxDialogAnswerAttempts,
	}
	inst, policy := newDialogGiveUpEscalationInstance(fakePM)

	baseline := goleak.IgnoreCurrent()
	defer goleak.VerifyNone(t, append(knownBackgroundGoroutines, baseline)...)

	// startSessionDriverForTest exists because StartSessionDriver always
	// resolves RetryPolicy fresh from config — see its doc comment for why.
	startSessionDriverForTest(inst, "/tmp", driverInitialPrompt, policy)
	defer StopSessionDriver(inst)

	// The next SendKeys call after maxDialogAnswerAttempts dialog-answer
	// failures is the initial-prompt send — proof the loop escaped the
	// `continue`. It only fires via the timedOut fallback once
	// driverReadyTimeout elapses; the 3x margin absorbs scheduler contention
	// under -race (confirmed flaky at tighter budgets).
	deadline := time.After(3*driverReadyTimeout + driverPollInterval*3 + time.Second)
	waitForSendKeysCountAbove(t, fakePM, maxDialogAnswerAttempts, deadline,
		"SendKeys count never exceeded the dialog-answer cap — the dialogGaveUp fall-through never reached the initial-prompt-send step (stuck in the continue trap)")
}

// waitForSendKeysCountAbove polls fakePM.sendKeysCount until it exceeds
// threshold or deadline fires, failing the test with failMsg in the latter
// case.
func waitForSendKeysCountAbove(t *testing.T, fakePM *stuckDialogProcessManager, threshold int32, deadline <-chan time.Time, failMsg string) {
	t.Helper()
	for fakePM.sendKeysCount.Load() <= threshold {
		select {
		case <-deadline:
			t.Fatal(failMsg)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// waitForDriverRunning blocks until inst.driverRunning is set (the driver
// goroutine has reached its poll-loop select) or 1s elapses.
func waitForDriverRunning(t *testing.T, inst *Instance) {
	t.Helper()
	deadline := time.After(time.Second)
	for !inst.driverRunning.Load() {
		select {
		case <-deadline:
			t.Fatal("driver goroutine never marked itself running")
		case <-time.After(time.Millisecond):
		}
	}
}

// stopSessionDriverConcurrently runs StopSessionDriver in its own goroutine
// and asserts it returns within its bounded timeout.
func stopSessionDriverConcurrently(t *testing.T, inst *Instance) {
	t.Helper()
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
	waitForDriverRunning(t, inst)
	time.Sleep(10 * time.Millisecond)

	stopSessionDriverConcurrently(t, inst)

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
// simulateHandleDriverFailureRetryInterleaving reproduces the exact
// interleaving handleDriverFailure produces: Add(1) and spawn a retry
// continuation goroutine BEFORE the simulated original goroutine's own
// Done() fires — the sequencing that broke the old done-channel-based
// StopSessionDriver. The returned channels let the caller observe the retry
// goroutine starting and release it when done.
func simulateHandleDriverFailureRetryInterleaving(inst *Instance, stopper *sessionDriverStopper) (retryStarted, retryFinish chan struct{}) {
	retryStarted = make(chan struct{})
	retryFinish = make(chan struct{})

	inst.driverWG.Add(1)
	go func() {
		defer inst.driverWG.Done()
		<-stopper.stop

		inst.driverWG.Add(1)
		go func() {
			defer inst.driverWG.Done()
			close(retryStarted)
			<-retryFinish
		}()
	}()
	return retryStarted, retryFinish
}

func TestStopSessionDriver_WaitsForHandleDriverFailureRetryGoroutine_NoGoroutineLeak(t *testing.T) {
	baseline := goleak.IgnoreCurrent()
	defer goleak.VerifyNone(t, append(knownBackgroundGoroutines, baseline)...)

	inst := &Instance{Title: "test-stop-waits-for-retry"}
	stopper := &sessionDriverStopper{stop: make(chan struct{})}
	inst.driverStopper.Store(stopper)

	retryStarted, retryFinish := simulateHandleDriverFailureRetryInterleaving(inst, stopper)

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

// TestScanAndLinkPRURL_RepublishesSnapshot is a regression test for a
// staleness bug a code review of backlog 10fc3913 caught: scanAndLinkPRURL
// wrote GitHubPRURL/GitHubPRNumber under inst.mu but never republished the
// snapshot, so GitHub() (and instance_adapter.go's API responses, which
// already read via Snapshot()) could never observe the auto-linked PR.
func TestScanAndLinkPRURL_RepublishesSnapshot(t *testing.T) {
	inst := &Instance{Title: "scan-pr-url-test", GitHubOwner: "octocat", GitHubRepo: "Hello-World"}
	finishInstanceConstruction(inst)

	output := "remote: Create a pull request for 'foo' on GitHub by visiting:\nremote:      https://github.com/octocat/Hello-World/pull/42\n"
	linked := scanAndLinkPRURL(inst, true, false, nil, output)

	if !linked {
		t.Fatal("expected scanAndLinkPRURL to report the PR as linked")
	}
	gh := inst.GitHub()
	if gh.PRNumber != 42 {
		t.Errorf("GitHub().PRNumber = %d, want 42 (snapshot was not republished after the raw write)", gh.PRNumber)
	}
	if gh.PRURL == "" {
		t.Error("GitHub().PRURL is empty, want the linked PR URL (snapshot was not republished after the raw write)")
	}
}
