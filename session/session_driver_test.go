package session

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

// TestShouldAnswerStartupDialog verifies the cooldown guard that prevents
// re-sending "1\n" within 5 seconds of the first answer. This is the extracted
// helper used in runSessionDriverWithPrompt.
func TestShouldAnswerStartupDialog(t *testing.T) {
	dialogOutput := `Quick safety check: Is this a project you created or one you trust?
 1. Yes, I trust this folder
 2. No, exit`

	t.Run("returns true when dialog present and cooldown has not started", func(t *testing.T) {
		var zeroTime time.Time
		got := shouldAnswerStartupDialog(dialogOutput, zeroTime, 5*time.Second)
		if !got {
			t.Error("expected true for fresh dialog with zero lastAnsweredAt")
		}
	})

	t.Run("returns false when within the cooldown window", func(t *testing.T) {
		recent := time.Now()
		got := shouldAnswerStartupDialog(dialogOutput, recent, 5*time.Second)
		if got {
			t.Error("expected false within cooldown window")
		}
	})

	t.Run("returns true when cooldown has elapsed", func(t *testing.T) {
		old := time.Now().Add(-10 * time.Second)
		got := shouldAnswerStartupDialog(dialogOutput, old, 5*time.Second)
		if !got {
			t.Error("expected true after cooldown has elapsed")
		}
	})

	t.Run("returns false for non-dialog output regardless of cooldown", func(t *testing.T) {
		var zeroTime time.Time
		got := shouldAnswerStartupDialog("> ", zeroTime, 5*time.Second)
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

func TestScanTerminalForPRURL(t *testing.T) {
	cases := []struct {
		name        string
		output      string
		wantURL     string
		wantPRNum   int
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
