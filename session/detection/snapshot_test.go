package detection

import (
	"os"
	"path/filepath"
	"testing"
)

// snapshotTest defines a single snapshot test case.
// fixture is a file in testdata/ containing real captured terminal output.
// expected is the DetectedStatus that should be returned for that output.
//
// To populate a failing stub:
//  1. Run the tool in the state described by `description`
//  2. Capture the terminal pane content (e.g. via `tmux capture-pane -p`)
//  3. Paste the raw output into the corresponding testdata/*.txt file
//  4. Re-run the tests — if detection is wrong, adjust the pattern or file
type snapshotTest struct {
	fixture     string         // filename under testdata/
	expected    DetectedStatus // what the detector should return
	program     string         // which tool (for documentation)
	description string         // what state this output represents
}

var snapshotTests = []snapshotTest{
	// ── Claude Code ──────────────────────────────────────────────────────────
	{
		fixture:     "claude_needs_approval.txt",
		expected:    StatusNeedsApproval,
		program:     "claude",
		description: "Claude showing a file/command permission prompt (Yes/No options with text like 'No, and tell Claude what to do differently')",
	},
	{
		fixture:     "claude_input_required.txt",
		expected:    StatusInputRequired,
		program:     "claude",
		description: "Claude showing a numbered selection prompt (❯ 1. Yes  2. No  3. Type here...)",
	},
	{
		fixture:     "claude_active.txt",
		expected:    StatusActive,
		program:     "claude",
		description: "Claude actively processing — visible 'esc to interrupt' or spinner",
	},
	{
		fixture:     "claude_idle_ready.txt",
		expected:    StatusIdle,
		program:     "claude",
		description: "Claude at the idle prompt — '? for shortcuts' visible, waiting for user input",
	},

	// ── Gemini CLI ───────────────────────────────────────────────────────────
	{
		fixture:     "gemini_needs_approval.txt",
		expected:    StatusNeedsApproval,
		program:     "gemini",
		description: "Gemini showing a permission prompt containing 'Yes, allow once'",
	},
	{
		fixture:     "gemini_active.txt",
		expected:    StatusActive,
		program:     "gemini",
		description: "Gemini actively generating or running a tool",
	},
	{
		fixture:     "gemini_idle.txt",
		expected:    StatusIdle,
		program:     "gemini",
		description: "Gemini at the idle input prompt — [INSERT] status bar visible, no Thinking... line",
	},

	// ── OpenCode ─────────────────────────────────────────────────────────────
	{
		fixture:     "opencode_needs_approval.txt",
		expected:    StatusNeedsApproval,
		program:     "opencode",
		description: "OpenCode showing a permission/approval prompt",
	},
	// OpenCode uses color-highlighted list selection (no ❯ cursor character),
	// so it does not produce a StatusInputRequired state detectable by pattern.
	// This fixture guards against false positives: OpenCode's selection UI must
	// NOT be mistaken for InputRequired.
	{
		fixture:     "opencode_input_required.txt",
		expected:    StatusUnknown,
		program:     "opencode",
		description: "OpenCode selection list (color-highlighted, no ❯) — must NOT trigger InputRequired",
	},
	{
		fixture:     "opencode_active.txt",
		expected:    StatusActive,
		program:     "opencode",
		description: "OpenCode actively processing a request",
	},

	// ── Aider ────────────────────────────────────────────────────────────────
	{
		fixture:     "aider_needs_approval.txt",
		expected:    StatusNeedsApproval,
		program:     "aider",
		description: "Aider showing '(Y)es/(N)o/(D)on't ask again' permission prompt",
	},
	{
		fixture:     "aider_active.txt",
		expected:    StatusActive,
		program:     "aider",
		description: "Aider actively editing or applying changes",
	},

	// ── False-positive guards ─────────────────────────────────────────────────
	// These fixtures contain numbered lists that SHOULD NOT trigger InputRequired.
	// They guard against regressions in the numbered_option_selector pattern.
	{
		fixture:     "gradle_numbered_output.txt",
		expected:    StatusUnknown, // or StatusActive/StatusProcessing — anything but StatusInputRequired
		program:     "any",
		description: "Gradle build output containing '> Run gradlew tasks...' — must NOT trigger InputRequired",
	},
	{
		fixture:     "markdown_blockquote_numbered.txt",
		expected:    StatusUnknown,
		program:     "any",
		description: "Markdown blockquote with numbered list (> 1. item) — must NOT trigger InputRequired",
	},
}

// TestSnapshotDetection runs the detector against each fixture file and
// checks that the detected status matches the expected value.
//
// Stub fixtures (empty files) fail with a descriptive message telling you
// exactly what terminal output to capture and paste in.
func TestSnapshotDetection(t *testing.T) {
	detector := NewStatusDetector()

	for _, tc := range snapshotTests {
		tc := tc
		t.Run(tc.fixture, func(t *testing.T) {
			path := filepath.Join("testdata", tc.fixture)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("Cannot read fixture %s: %v", path, err)
			}

			if len(data) == 0 {
				t.Fatalf(
					"Fixture %q is empty — populate it with real terminal output.\n"+
						"  Program:     %s\n"+
						"  State:       %s\n"+
						"  Expected:    %s\n\n"+
						"Capture the pane content (e.g. tmux capture-pane -p) while %s is in this state\n"+
						"and paste the raw output into session/detection/testdata/%s",
					tc.fixture, tc.program, tc.description, tc.expected.String(),
					tc.program, tc.fixture,
				)
			}

			got := detector.DetectForProgram(data, tc.program)

			// False-positive guards: the fixture must NOT be StatusInputRequired.
			// We allow any other status (Unknown, Active, Processing, etc.) since
			// the exact status depends on the surrounding content.
			if tc.fixture == "gradle_numbered_output.txt" || tc.fixture == "markdown_blockquote_numbered.txt" || tc.fixture == "opencode_input_required.txt" {
				if got == StatusInputRequired {
					t.Errorf(
						"FALSE POSITIVE in %q: got StatusInputRequired but this is NOT a selection prompt.\n"+
							"  Program: %s\n"+
							"  State:   %s\n"+
							"  The numbered_option_selector pattern matched content it should not have.",
						tc.fixture, tc.program, tc.description,
					)
				}
				return
			}

			if got != tc.expected {
				t.Errorf(
					"Fixture %q: got %s, expected %s\n"+
						"  Program: %s\n"+
						"  State:   %s\n"+
						"  If the tool's UI changed, update the fixture file and/or the expected status.",
					tc.fixture, got.String(), tc.expected.String(),
					tc.program, tc.description,
				)
			}
		})
	}
}
