package detection

import (
	"strings"
	"testing"
)

// TestBug1_IndentedSpinner verifies that a spinner indented by leading whitespace
// (as rendered inside Claude Code's task manager panel) is detected as Active.
//
// Before fix: (?m)^[spinner] required the spinner at column 0; "  ✽ Roosting…" failed.
// After fix:  (?m)^\s*[spinner] allows any leading whitespace.
func TestBug1_IndentedSpinner(t *testing.T) {
	sd := NewStatusDetector()
	cases := []struct {
		name  string
		input string
	}{
		{"2-space indent", "  ✽ Roosting… (9m 52s · ↓ 2.8k tokens)"},
		{"4-space indent", "    ✻ Perambulating..."},
		{"tab indent", "\t✽ Tinkering…"},
		{"indented middle dot", "  · Herding…"},
		{"indented circle", "  ● Working…"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sd.Detect([]byte(tc.input))
			if got != StatusActive && got != StatusProcessing {
				t.Errorf("Detect(%q) = %s, want StatusActive or StatusProcessing (indented spinner must be detected)",
					tc.input, got)
			}
		})
	}
}

// TestBug1_IndentedSpinner_NoRegression ensures that non-spinner content with
// leading whitespace does not become a false-positive Active.
func TestBug1_IndentedSpinner_NoRegression(t *testing.T) {
	sd := NewStatusDetector()
	cases := []struct {
		name  string
		input string
	}{
		{"lowercase verb", "  ✽ roosting…"},         // rejected by [A-Z] (requires capital first letter)
		{"no ellipsis", "  ✽ Roosting"},             // rejected by (?:…|\.{1,3}) (requires trailing ellipsis)
		{"markdown bullet", "  * Item one"},          // rejected by (?:…|\.{1,3}) (no ellipsis after "one")
		{"timing separator", "(8m 39s · ↓ 834 tokens)"}, // · not at start of meaningful pattern; no [A-Z]verb… follows
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sd.Detect([]byte(tc.input))
			if got == StatusActive {
				t.Errorf("Detect(%q) = StatusActive, expected no match (false positive for indented spinner)",
					tc.input)
			}
		})
	}
}

// TestBug1_CRCollapse_EscToInterrupt_Preserved verifies that "esc to interrupt"
// is detected as Active even when a TUI task manager panel overwrites it via \r on
// the same terminal row.
//
// Scenario: Claude Code writes "esc to interrupt · main\r↑/↓ to select · view"
// where \r causes the task manager to visually replace the interrupt hint.
// Before fix: collapseCarriageReturns kept only "↑/↓ to select" → StatusReady.
// After fix:  DetectFromLines scans CR-split segments in reverse; last segment
//
//	(↑/↓ to select → Ready) doesn't short-circuit; earlier segment
//	(esc to interrupt → Active) is returned.
func TestBug1_CRCollapse_EscToInterrupt_Preserved(t *testing.T) {
	sd := NewStatusDetector()

	// Simulate the raw PTY line: esc-to-interrupt bar overwritten by task manager
	crLine := "esc to interrupt · ↓ to manage  ● main\r↑/↓ to select · Enter to view  ◯ research-workflow"
	lines := []string{crLine}

	got := sd.DetectFromLines(lines)
	if got != StatusActive {
		t.Errorf("DetectFromLines with CR-overwritten esc-to-interrupt: got %s, want StatusActive\n"+
			"  Line: %q\n"+
			"  The task-manager panel overwrites 'esc to interrupt' via \\r but the session IS active.",
			got, crLine)
	}
}

// TestBug1_CRCollapse_IdleStillIdle verifies that when "esc to interrupt" is
// CR-overwritten by "? for shortcuts" (session completed its turn), the result
// is Idle, not Active.  This guards against a regression in the reverse-segment fix.
func TestBug1_CRCollapse_IdleStillIdle(t *testing.T) {
	sd := NewStatusDetector()

	crLine := "esc to interrupt · ↓ to manage  ● main\r? for shortcuts"
	lines := []string{crLine}

	got := sd.DetectFromLines(lines)
	if got == StatusActive {
		t.Errorf("DetectFromLines with CR-overwritten-by-idle: got StatusActive, want Idle/Ready\n"+
			"  Line: %q\n"+
			"  When 'esc to interrupt' is replaced by '? for shortcuts' the session IS idle.",
			crLine)
	}
}

// TestBug1_FullContent_ActiveWithTaskManager verifies that DetectFromLines on the
// full terminal content (spinner in task manager + esc to interrupt + old completion
// line) correctly returns StatusActive by scanning bottom-up.
func TestBug1_FullContent_ActiveWithTaskManager(t *testing.T) {
	sd := NewStatusDetector()

	// Simulate the full terminal content with:
	// - Old completion line (higher up in scrollback)
	// - New active turn with indented spinner + esc to interrupt bar
	content := strings.Join([]string{
		"✻ Baked for 18m 15s",
		"",
		"❯ I'm thinking about work travel with a suit jacket",
		"",
		"● research-workflow(Business carry-on suiter and garment bag research)",
		"  ✽ Roosting… (9m 52s · ↓ 2.8k tokens)",
		"  ⎿  Tip: Use /btw to ask a quick side question",
		"",
		"❯",
		"esc to interrupt · ↓ to manage  ● main",
		"↑/↓ to select · Enter to view  ◯ research-workflow (+1)  Business carry-on suiter",
	}, "\n")

	lines := strings.Split(content, "\n")
	got := sd.DetectFromLines(lines)
	if got != StatusActive {
		t.Errorf("DetectFromLines on active-with-task-manager content: got %s, want StatusActive\n"+
			"  The session has an active spinner and esc-to-interrupt even though\n"+
			"  a prior completion line (✻ Baked for X) is visible in the scrollback.",
			got)
	}
}

// TestBug1_WithContextFromLines_ActiveWithTaskManager mirrors the above but uses
// DetectWithContextFromLines (the path taken by GetCurrentStatus).
func TestBug1_WithContextFromLines_ActiveWithTaskManager(t *testing.T) {
	sd := NewStatusDetector()

	lines := []string{
		"✻ Baked for 18m 15s",
		"",
		"❯ I'm thinking about work travel",
		"",
		"● research-workflow(Business carry-on research)",
		"  ✽ Roosting… (9m 52s · ↓ 2.8k tokens)",
		"",
		"❯",
		"esc to interrupt · ↓ to manage  ● main",
		"↑/↓ to select · Enter to view  ◯ research-workflow (+1)",
	}

	got, _ := sd.DetectWithContextFromLines(lines)
	if got != StatusActive {
		t.Errorf("DetectWithContextFromLines on active-with-task-manager: got %s, want StatusActive",
			got)
	}
}

// TestBug2_InputRequired_WithSuccessScrollback verifies that DetectFromLines returns
// StatusInputRequired when a selection dialog (❯ 1.) appears below an old completion
// line (✻ Baked for X).
//
// Before fix (conceptually): full-content Detect() returns StatusSuccess because
// Success is higher priority.  DetectFromLines should return StatusInputRequired
// because it scans bottom-up and finds ❯ 1. before ✻ Baked.
func TestBug2_InputRequired_WithSuccessScrollback(t *testing.T) {
	sd := NewStatusDetector()

	lines := []string{
		"✻ Baked for 5m 30s",
		"",
		"Here is my analysis. All looks good.",
		"",
		" Do you want to proceed?",
		"",
		" ❯ 1. Create PR now (Recommended)",
		"   2. Review the changes first",
		"   3. Cancel and make edits",
		"   4. Type here to tell Claude what to do differently",
		"",
		"Enter to select · ↑/↓ to navigate · Esc to cancel",
	}

	got := sd.DetectFromLines(lines)
	if got != StatusInputRequired {
		t.Errorf("DetectFromLines with InputRequired below Success scrollback: got %s, want StatusInputRequired\n"+
			"  Session shows ❯ 1. selection dialog — must be detected as InputRequired.\n"+
			"  Old completion line (✻ Baked for X) must not override the more recent dialog.",
			got)
	}
}

// TestBug2_WithContextFromLines_InputRequired mirrors the above for GetCurrentStatus path.
func TestBug2_WithContextFromLines_InputRequired(t *testing.T) {
	sd := NewStatusDetector()

	lines := []string{
		"✻ Baked for 5m 30s",
		"",
		" Do you want to proceed?",
		"",
		" ❯ 1. Create PR now (Recommended)",
		"   2. Review the changes first",
		"   3. Cancel",
		"",
		"Enter to select · ↑/↓ to navigate · Esc to cancel",
	}

	got, _ := sd.DetectWithContextFromLines(lines)
	if got != StatusInputRequired {
		t.Errorf("DetectWithContextFromLines with InputRequired + Success scrollback: got %s, want StatusInputRequired",
			got)
	}
}

// TestBug2_EscToCancel_CaseInsensitive verifies that "Esc to cancel" (capital E)
// in a selection dialog footer does NOT match the esc_to_interrupt Active pattern.
// If it did match Active, the scan would stop before finding ❯ 1. (InputRequired).
func TestBug2_EscToCancel_NotActive(t *testing.T) {
	sd := NewStatusDetector()

	// This is the footer line of a selection dialog
	footer := "Enter to select · ↑/↓ to navigate · Esc to cancel"
	got := sd.Detect([]byte(footer))
	if got == StatusActive {
		t.Errorf("Detect(%q) = StatusActive, but 'Esc to cancel' must NOT trigger esc_to_interrupt\n"+
			"  The pattern is case-sensitive (lowercase 'esc') to prevent dialog footers\n"+
			"  from being mistaken for active operation interrupts.",
			footer)
	}
}

// TestCRCollapse_LastSegmentSuccessIsAuthoritative verifies that when the LAST CR
// segment produces StatusSuccess (e.g., the task manager writes a completion line
// that overwrites "esc to interrupt"), Success wins — the session is done.
//
// This is the mirror of TestBug1_CRCollapse_EscToInterrupt_Preserved: there the
// EARLIER segment was Active and the last was Ready.  Here the LAST segment is
// Success, which must be authoritative even though an earlier segment was Active.
func TestCRCollapse_LastSegmentSuccessIsAuthoritative(t *testing.T) {
	sd := NewStatusDetector()

	// Last segment (✻ Baked for 5s) overwrites the active indicator — session is done.
	crLine := "esc to interrupt · ↓ to manage  ● main\r✻ Baked for 5s"
	got := sd.DetectFromLines([]string{crLine})
	if got == StatusActive {
		t.Errorf("DetectFromLines(%q) = StatusActive, want Success/other\n"+
			"  The last CR segment (✻ Baked) is authoritative — session completed its turn.\n"+
			"  An earlier Active segment must not override the later Success.",
			crLine)
	}
}

// TestMapStatusToIdleState_ExplicitCoverage verifies that all DetectedStatus values
// are handled explicitly in mapStatusToIdleState (no silent fall-through to default).
func TestMapStatusToIdleState_ExplicitCoverage(t *testing.T) {
	id := NewIdleDetector("test", nil)

	cases := []struct {
		status      DetectedStatus
		wantAny     []IdleState // any of these is acceptable
		mustNot     IdleState   // must not be this (ignored when skipMustNot is true)
		skipMustNot bool
		name        string
	}{
		{StatusActive, []IdleState{IdleStateActive}, IdleStateWaiting, false, "Active → IdleStateActive"},
		{StatusProcessing, []IdleState{IdleStateActive}, IdleStateWaiting, false, "Processing → IdleStateActive"},
		{StatusInputRequired, []IdleState{IdleStateWaiting}, 0, true, "InputRequired → IdleStateWaiting"},
		{StatusSuccess, []IdleState{IdleStateWaiting}, 0, true, "Success → IdleStateWaiting"},
		{StatusNeedsApproval, []IdleState{IdleStateWaiting}, 0, true, "NeedsApproval → IdleStateWaiting"},
		{StatusError, []IdleState{IdleStateWaiting}, 0, true, "Error → IdleStateWaiting"},
		// Time-dependent: may return Timeout after idle threshold, so both are valid.
		{StatusIdle, []IdleState{IdleStateWaiting, IdleStateTimeout}, 0, true, "Idle → IdleStateWaiting/Timeout"},
		{StatusReady, []IdleState{IdleStateWaiting, IdleStateTimeout}, 0, true, "Ready → IdleStateWaiting/Timeout"},
		// These fall to default — documenting the expected behavior explicitly.
		{StatusTestsFailing, []IdleState{IdleStateWaiting}, 0, true, "TestsFailing → IdleStateWaiting (default)"},
		{StatusUnknown, []IdleState{IdleStateWaiting}, 0, true, "Unknown → IdleStateWaiting (default)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := id.mapStatusToIdleState(tc.status)
			found := false
			for _, want := range tc.wantAny {
				if got == want {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("mapStatusToIdleState(%s) = %s, want one of %v", tc.status, got, tc.wantAny)
			}
			if !tc.skipMustNot && got == tc.mustNot {
				t.Errorf("mapStatusToIdleState(%s) = %s, must NOT be %s", tc.status, got, tc.mustNot)
			}
		})
	}
}

// TestBug_ToolPermissionDialog_DetectedAsInputRequired guards against a regression where
// a Claude Code tool permission dialog ("Do you want to proceed? / ❯ 1. Yes") was not
// detected as StatusInputRequired when earlier scrollback contained "Error: Exit code 1"
// from a failed command.
//
// Root cause: Detect() (full-block) fires error_message on the earlier "Error:" line
// before reaching the dialog patterns.  DetectFromLines (bottom-up scan) must reach
// ❯ 1. Yes first and return StatusInputRequired without being blocked by the stale error.
//
// Observed: session running /github:pr-ship showed "Error: Exit code 1" from
// `gh pr checks 149 --watch=false`, then hit the python3 tool permission dialog.
// The session list displayed the wrong status instead of needs-input.
func TestBug_ToolPermissionDialog_DetectedAsInputRequired(t *testing.T) {
	sd := NewStatusDetector()

	// Scrollback: a prior failed command ("Error: Exit code 1"), then the tool dialog.
	lines := []string{
		"● Bash(gh pr checks 149 --watch=false 2>&1)",
		"  ⎿  Error: Exit code 1",
		"     no checks reported on the 'stelekit-bazel' branch",
		"",
		"──────────────────────────────────────────────────────────────────────────────",
		" Bash command",
		"   gh run list --branch stelekit-bazel --json",
		`   databaseId,name,status,conclusion,createdAt --limit 10 2>&1 | python3 -c`,
		`   "import json,sys; runs=json.load(sys.stdin); [print(r['databaseId'],`,
		`   r['status'], r['conclusion'], r['name'][:30]) for r in runs]"`,
		"   List all recent CI runs with status for stelekit-bazel",
		" Do you want to proceed?",
		" ❯ 1. Yes",
		`  2.Yes, and don't ask  : python3 -c "import json,sys;`,
		`    again for           runs=json.load(sys.stdin); [print(r['databaseId'],`,
		`    r['status'], r['conclusion'], r['name'][:30]) for r in runs]"`,
		"   3. No",
		" Esc to cancel · Tab to amend · ctrl+e to explain",
	}

	got := sd.DetectFromLines(lines)
	if got != StatusInputRequired {
		t.Errorf("DetectFromLines with tool permission dialog after error scrollback: got %s, want StatusInputRequired\n"+
			"  The session was waiting at '❯ 1. Yes' dialog — must be detected as InputRequired.\n"+
			"  Earlier 'Error: Exit code 1' in scrollback must NOT override the current dialog state.\n"+
			"  Use DetectFromLines (bottom-up scan) so the dialog is found before the stale error.",
			got)
	}
}

// TestBug_ToolPermissionDialog_WithConcurrentWaiting guards against a regression where
// a Claude Code tool permission dialog was NOT detected as StatusInputRequired when the
// pane also showed a concurrent "⎿  Waiting…" tool call and "❯ /github:pr-ship 149"
// readline input above the dialog.
//
// Root cause candidates:
//   - readlineTypingRegex (`^❯[ \t]+[^0-9\s]`) matches "❯ /github:pr-ship 149" and
//     returns StatusIdle if the scan somehow reaches that line before ❯ 1. Yes.
//   - The dialog's option-2 description is 4 wrapped lines (longer than earlier tests),
//     pushing the total dialog height above the statusDetectionLinesWindow if window is tight.
//
// DetectFromLines (bottom-up) must return StatusInputRequired from "❯ 1. Yes" without
// being intercepted by the readline cursor line higher in the pane.
func TestBug_ToolPermissionDialog_WithConcurrentWaiting(t *testing.T) {
	sd := NewStatusDetector()

	lines := []string{
		// Earlier output in the pane
		"❯ /github:pr-ship 149",
		`      "import json,sys; runs=json.load(sys.stdin); [p…)`,
		"  ⎿  Waiting…",
		"",
		// Tool permission dialog
		"──────────────────────────────────────────────────────────────────────────────",
		" Bash command",
		"   gh run list --branch stelekit-bazel --json",
		"   databaseId,name,status,conclusion,headSha --limit 8 2>&1 | python3 -c",
		`   "import json,sys; runs=json.load(sys.stdin); [print(r['databaseId'],`,
		`   r['status'][:12], r['conclusion'] or 'pending', r['headSha'][:8],`,
		`   r['name'][:35]) for r in runs]"`,
		"   List all CI runs on stelekit-bazel with commit SHAs",
		"",
		" Do you want to proceed?",
		" ❯ 1. Yes",
		`  2.Yes, and don't ask  : python3 -c "import json,sys;`,
		`    again for           runs=json.load(sys.stdin); [print(r['databaseId'],`,
		`                        r['status'][:12], r['conclusion'] or 'pending',`,
		`                        r['headSha'][:8], r['name'][:35]) for r in runs]"`,
		"   3. No",
		"",
		" Esc to cancel · Tab to amend · ctrl+e to explain",
	}

	got := sd.DetectFromLines(lines)
	if got != StatusInputRequired {
		t.Errorf("DetectFromLines with tool permission dialog + concurrent Waiting: got %s, want StatusInputRequired\n"+
			"  Session was waiting at '❯ 1. Yes' tool dialog.\n"+
			"  '❯ /github:pr-ship 149' readline line above dialog must NOT intercept the scan.\n"+
			"  '⎿  Waiting…' concurrent tool line must NOT override the dialog detection.",
			got)
	}
}

// TestBug3_BoxDrawingSeparator_NotReadlineTyping guards against readlineTypingRegex
// false-positives on the horizontal separator line "❯ ──────────────────..." that
// Claude Code renders in the input-area border when an operation is running.
//
// Root cause: readlineTypingRegex (`^❯[ \t]+[^0-9\s]`) matched "❯ ─────..." because
// ─ (U+2500 BOX DRAWINGS LIGHT HORIZONTAL) satisfies [^0-9\s]. This caused StatusIdle
// ("readline_typing") to be returned before Active patterns were checked, so sessions
// showing "✻ Cooking… (2h 54m 38s · ↓ 308.9k tokens)" with "esc to interrupt" were
// incorrectly reported as idle.
//
// Fix: readlineTypingRegex now excludes U+2500–U+257F (Box Drawing block):
// `^❯[ \t]+[^\s0-9\x{2500}-\x{257F}]`
//
// Observed terminal layout (user report 2026-06-15):
//
//	● Bash(npx playwright test …)  ⎿  Running… (1m 37s · timeout 3m)
//	✻ Cooking… (2h 54m 38s · ↓ 308.9k tokens)
//	  ⎿  Tip: Use /clear to start fresh when switching topics and free up context
//	──────────────────────────────────────────────────────────────────────────────
//	❯ ──────────────────────────────────────────────────────────────────────────
//	  esc to interrupt                                    ✘ Auto-update failed
func TestBug3_BoxDrawingSeparator_NotReadlineTyping(t *testing.T) {
	sd := NewStatusDetector()

	lines := []string{
		"● Bash(npx playwright test tests/02-trip-creation.spec.ts --reporter=line 2>&1)  ⎿  Running… (1m 37s · timeout 3m)",
		"   (ctrl+b ctrl+b (twice) to run in background)",
		"✻ Cooking… (2h 54m 38s · ↓ 308.9k tokens)",
		"  ⎿  Tip: Use /clear to start fresh when switching topics and free up context",
		"──────────────────────────────────────────────────────────────────────────────────────────────────────────────────",
		"❯ ──────────────────────────────────────────────────────────────────────────────────────────────────────────────────",
		"  esc to interrupt                                                            ✘ Auto-update failed · Run /doctor",
	}
	fullContent := strings.Join(lines, "\n")

	// Detect() on the full block (path used by deprecated DetectState() / PTY buffer path).
	// The ❯ ─────... separator line must not trigger readline_typing before Active patterns.
	gotBlock := sd.Detect([]byte(fullContent))
	if gotBlock != StatusActive {
		t.Errorf("Detect() on full content with ❯ box-drawing separator: got %s, want StatusActive\n"+
			"  ❯ followed by ─ (U+2500) is a UI separator, not user typing.\n"+
			"  readlineTypingRegex must not match U+2500–U+257F box-drawing chars.",
			gotBlock)
	}

	// DetectFromLines() (path used by DetectStateFromContent / tmux capture-pane).
	gotLines := sd.DetectFromLines(lines)
	if gotLines != StatusActive {
		t.Errorf("DetectFromLines() on active session with ❯ box-drawing separator: got %s, want StatusActive",
			gotLines)
	}
}

// TestBug3_BoxDrawingSeparator_ReadlineTypingStillWorks verifies that the box-drawing
// exclusion does not break detection of actual user typing at the ❯ prompt.
func TestBug3_BoxDrawingSeparator_ReadlineTypingStillWorks(t *testing.T) {
	sd := NewStatusDetector()

	// These must still be detected as readline typing (StatusIdle), overriding
	// a stale Active marker in scrollback.
	cases := []struct {
		name  string
		input string
	}{
		{"slash command", "❯ /github:pr-ship 149"},
		{"plain text", "❯ hello world"},
		{"exclamation", "❯ !ls"},
		{"letter start", "❯ what is the plan?"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sd.Detect([]byte(tc.input))
			if got != StatusIdle {
				t.Errorf("Detect(%q) = %s, want StatusIdle (readline typing)\n"+
					"  ASCII user input after ❯ must still trigger readline_typing.",
					tc.input, got)
			}
			gotLines := sd.DetectFromLines(strings.Split(tc.input, "\n"))
			if gotLines != StatusIdle {
				t.Errorf("DetectFromLines(%q) = %s, want StatusIdle (readline typing)\n"+
					"  ASCII user input after ❯ must still trigger readline_typing via DetectFromLines.",
					tc.input, gotLines)
			}
		})
	}
}

// TestBug_ThinkingWithStillThinkingSuffix documents that a Claude Code spinner line
// with a "· still thinking" suffix in the duration annotation is still detected as
// StatusActive, not silently dropped.
//
// Observed: Claude Code appends "· still thinking" to the duration when a turn has been
// running unusually long (e.g. "✻ Imagining… (7m 5s · ↓ 21.4k tokens · still thinking)").
// The surrounding terminal content also contains a bare ❯ cursor line and a
// "esc to interrupt" status bar, so DetectFromLines must pick up Active from one of them.
func TestBug_ThinkingWithStillThinkingSuffix(t *testing.T) {
	sd := NewStatusDetector()

	lines := []string{
		"✻ Imagining… (7m 5s · ↓ 21.4k tokens · still thinking)",
		"──────────────────────────────────────────────────────────────────────────────",
		"❯ ",
		"──────────────────────────────────────────────────────────────────────────────",
		"  2 shells · esc to interrupt · ↓ to manage",
	}

	got := sd.DetectFromLines(lines)
	if got != StatusActive {
		t.Errorf("DetectFromLines with '✻ Imagining… · still thinking' spinner: got %s, want StatusActive\n"+
			"  The session is actively thinking — spinner line and 'esc to interrupt' both indicate Active.\n"+
			"  The '· still thinking' suffix on the spinner must not prevent Active detection.",
			got)
	}
}

// TestBug4_NonBreakingSpace_ReadlineTyping guards against readlineTypingRegex missing the
// Claude Code readline prompt when it uses U+00A0 NON-BREAKING SPACE between ❯ and the
// typed text instead of a regular ASCII space.
//
// Root cause (found via live-session exploration 2026-06-15):
//   Claude Code inserts U+00A0 (NBSP,  ) between the ❯ cursor and the user's typed
//   text. The old readlineTypingRegex used [ \t]+ which only matches ASCII whitespace,
//   so "❯ what else can we clean up" did NOT fire readline_typing. The scan then
//   fell through to find "✻ Baked for 32s" in scrollback → incorrectly StatusSuccess.
//
// Observed in three live sessions:
//   - staplersquad_Slowing: "❯ what else can we clean up" + ✻ Baked for 32s
//   - staplersquad_stelekit-bazel: "❯ push it to github" + ✻ Worked for 1h 22m 22s
//   - staplersquad_stapler-squad-background-agent: "❯ merge when CI passes" + ✻ Churned for 2m 38s
//
// All three were incorrectly reported as StatusSuccess instead of StatusIdle.
func TestBug4_NonBreakingSpace_ReadlineTyping(t *testing.T) {
	sd := NewStatusDetector()

	// The exact terminal layout from staplersquad_Slowing (captured 2026-06-15).
	// U+00A0 NBSP between ❯ and the typed text.
	lines := []string{
		"  Disk is now at 204 GB free and the home directory is considerably tidier. Anything else",
		"  you'd like to clean up?",
		"",
		"✻ Baked for 32s",
		"",
		"───────────────────────────────────────────────────────────────────────────────────────────",
		"❯ what else can we clean up in the home directory",
		"───────────────────────────────────────────────────────────────────────────────────────────",
		"  ⏵⏵ accept edits on (shift+tab to cycle) · ← for agents",
	}

	got := sd.DetectFromLines(lines)
	if got != StatusIdle {
		t.Errorf("DetectFromLines with NBSP readline prompt + ✻ completion in scrollback: got %s, want StatusIdle\n"+
			"  '❯\\u00a0what else...' uses U+00A0 NON-BREAKING SPACE after ❯.\n"+
			"  readlineTypingRegex must match NBSP so the readline line is recognized as typing,\n"+
			"  preventing the stale '✻ Baked for 32s' completion line from being returned as Success.",
			got)
	}
}

// TestBug4_NonBreakingSpace_OtherLayouts verifies the fix against the
// other two live-session layouts that triggered the same bug.
func TestBug4_NonBreakingSpace_OtherLayouts(t *testing.T) {
	sd := NewStatusDetector()

	cases := []struct {
		name  string
		lines []string
	}{
		{
			name: "stelekit-bazel layout",
			lines: []string{
				"  Ready to ship. Want me to initialize a git repo?",
				"",
				"✻ Worked for 1h 22m 22s",
				"",
				"──────────────────────────────────────────────────────────────────────────────────────────────────────────────────",
				"❯ push it to github",
				"──────────────────────────────────────────────────────────────────────────────────────────────────────────────────",
				"  ⏵⏵ accept edits on (shift+tab to cycle) · ← for agents",
			},
		},
		{
			name: "background-agent layout",
			lines: []string{
				"● Pushed 85fad4bd. Now waiting on CI.",
				"",
				"✻ Churned for 2m 38s",
				"",
				"──────────────────────────────────────────────────────────────────────────────────────────────────────────",
				"❯ merge when CI passes",
				"──────────────────────────────────────────────────────────────────────────────────────────────────────────",
				"  PR #115 · ← for agents",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sd.DetectFromLines(tc.lines)
			if got != StatusIdle {
				t.Errorf("DetectFromLines (%s): got %s, want StatusIdle\n"+
					"  NBSP after ❯ must trigger readline_typing to prevent stale completion from winning.",
					tc.name, got)
			}
		})
	}
}

// TestBug4_AcceptEditsPattern verifies that the ⏵⏵ accept-edits status bar is recognized
// as StatusIdle when it is the only status indicator visible (no typed readline message).
func TestBug4_AcceptEditsPattern(t *testing.T) {
	sd := NewStatusDetector()

	// Session where the user has not yet started typing — only the ⏵⏵ bar is visible.
	// Without the claude_accept_edits pattern, this falls through to StatusReady (catch-all),
	// and any stale completion line in scrollback would win as StatusSuccess.
	lines := []string{
		"✻ Baked for 14s",
		"",
		"───────────────────────────────────────────────────────────────────────────────────────────",
		"❯ ",
		"───────────────────────────────────────────────────────────────────────────────────────────",
		"  ⏵⏵ accept edits on (shift+tab to cycle) · ← for agents",
	}

	got := sd.DetectFromLines(lines)
	if got == StatusSuccess {
		t.Errorf("DetectFromLines with ⏵⏵ accept-edits bar + completion in scrollback: got StatusSuccess, want StatusIdle\n"+
			"  ⏵⏵ accept edits on must be recognized as an idle state so the stale ✻ completion\n"+
			"  in scrollback does not make the session appear as Success (which means 'done, no action needed').")
	}
	if got != StatusIdle {
		t.Errorf("DetectFromLines with ⏵⏵ accept-edits bar: got %s, want StatusIdle", got)
	}
}
