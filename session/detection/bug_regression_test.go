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
