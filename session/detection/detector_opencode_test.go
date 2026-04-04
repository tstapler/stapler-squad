package detection

import (
	"testing"
)

func TestOpencode_DetectInputRequired_NumberedOptions(t *testing.T) {
	detector := NewStatusDetector()

	testCases := []struct {
		name   string
		output string
	}{
		{
			name:   "numbered_option_with_arrow_selector",
			output: "❯ 1. First option\n   2. Second option",
		},
		{
			name:   "bar_prefixed_numbered_option",
			output: "┃  4. Icons:",
		},
		{
			name: "opencode_footer_with_options",
			output: `┃  4. Icons:
┃      - Do you have existing app icons?
┃      - Or should I create simple placeholder SVGs?
┃
┃  5. Scope priority:
┃      - Push notifications first, then mobile improvements?
┃      - Both in parallel?`,
		},
		{
			name:   "bar_prefixed_numbered_options_no_spaces",
			output: "┃ 1. Option A\n┃ 2. Option B",
		},
		{
			name:   "bar_prefixed_with_leading_space",
			output: "┃  4. Icons:\n┃  5. Scope priority",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			status := detector.Detect([]byte(tc.output))
			if status != StatusInputRequired {
				t.Errorf("Detect(%q) returned %v, expected StatusInputRequired", tc.output, status)
			}
		})
	}
}

func TestOpencode_DetectActive_EscInterrupt(t *testing.T) {
	detector := NewStatusDetector()

	testCases := []struct {
		name   string
		output string
	}{
		{
			name:   "esc_interrupt_short",
			output: "esc interrupt",
		},
		{
			name:   "esc_interrupt_with_parens",
			output: "(esc to interrupt)",
		},
		{
			name:   "esc_interrupt_full",
			output: "esc to interrupt",
		},
		{
			name:   "with_build_indicator",
			output: "▣  Build · big-pickle\nesc interrupt",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			status := detector.Detect([]byte(tc.output))
			if status != StatusActive {
				t.Errorf("Detect(%q) returned %v, expected StatusActive", tc.output, status)
			}
		})
	}
}

func TestOpencode_DetectPriority_InputRequiredOverActive(t *testing.T) {
	detector := NewStatusDetector()

	output := `❯ 1. First option
   2. Second option
esc interrupt`

	status := detector.Detect([]byte(output))
	if status != StatusInputRequired {
		t.Errorf("Detect() with both numbered options and esc interrupt returned %v, expected StatusInputRequired (higher priority)", status)
	}
}

func TestOpencode_DetectIdle_NoPrompts(t *testing.T) {
	detector := NewStatusDetector()

	testCases := []struct {
		name   string
		output string
	}{
		{
			name:   "regular_terminal_output",
			output: "some regular terminal output",
		},
		{
			name:   "empty_line",
			output: "",
		},
		{
			name:   "plan_mode_without_prompt",
			output: "▣  Plan · big-pickle",
		},
		{
			name:   "implementing_text",
			output: "Implementing the feature",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			status := detector.Detect([]byte(tc.output))
			if status != StatusIdle && status != StatusReady {
				t.Errorf("Detect(%q) returned %v, expected StatusIdle or StatusReady", tc.output, status)
			}
		})
	}
}

func TestOpencode_DetectRealExamples(t *testing.T) {
	detector := NewStatusDetector()

	workingExample := `┃  Thinking: There's no pnpm lock file. The project uses npm (package-lock.json).
     → Read MODULE.bazel
     ▣  Build · big-pickle
     esc interrupt`

	idleExample := `┃  4. Icons:
┃      - Do you have existing app icons?
┃      - Or should I create simple placeholder SVGs?
┃
┃  5. Scope priority:
┃      - Push notifications first, then mobile improvements?
┃      - Both in parallel?`

	t.Run("working_session_has_esc_interrupt", func(t *testing.T) {
		status := detector.Detect([]byte(workingExample))
		if status != StatusActive {
			t.Errorf("Working example (with esc interrupt) returned %v, expected StatusActive", status)
		}
	})

	t.Run("idle_session_has_bar_prefixed_options", func(t *testing.T) {
		status := detector.Detect([]byte(idleExample))
		if status != StatusInputRequired {
			t.Errorf("Idle example (with ┃ prefixed options) returned %v, expected StatusInputRequired", status)
		}
	})
}

// TestOpencode_NoFalsePositives_BodyNumberedLists ensures body content numbered
// lists are NOT incorrectly detected as InputRequired. Only the footer/prompt
// area (prefixed with ┃) should trigger InputRequired.
func TestOpencode_NoFalsePositives_BodyNumberedLists(t *testing.T) {
	detector := NewStatusDetector()

	testCases := []struct {
		name   string
		output string
	}{
		{
			name:   "body_numbered_points",
			output: "1. Point 1\n2. Point 2\n3. Point 3",
		},
		{
			name: "body_task_list",
			output: `Here are the implementation steps:
1. Update the detector patterns
2. Add test coverage
3. Run regression tests`,
		},
		{
			name: "body_numbered_subitems",
			output: `  4. Icons:
    - Do you have existing app icons?
    - Or should I create simple placeholder SVGs?

  5. Scope priority:
    - Push notifications first, then mobile improvements?
    - Both in parallel?`,
		},
		{
			name:   "body_markdown_ordered_list",
			output: "Steps:\n1. First step\n2. Second step\n3. Third step",
		},
		{
			name:   "body_single_numbered_item",
			output: "1. Only one item here",
		},
		{
			name: "body_numbered_with_prose_before",
			output: `I'll create a detailed implementation plan with file locations.

1. Check existing patterns
2. Update detector
3. Verify tests pass`,
		},
		{
			name: "body_numbered_indented",
			output: `The plan has three phases:
   1. Research
   2. Implementation
   3. Testing`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			status := detector.Detect([]byte(tc.output))
			if status == StatusInputRequired {
				t.Errorf("False positive: body content %q was incorrectly detected as StatusInputRequired", tc.output)
			}
		})
	}
}

// TestOpencode_DetectProcessing_Thinking verifies OpenCode "Thinking:" output
// is detected as Processing.
func TestOpencode_DetectProcessing_Thinking(t *testing.T) {
	detector := NewStatusDetector()

	testCases := []struct {
		name   string
		output string
	}{
		{
			name:   "thinking_with_colon",
			output: "Thinking: There's no pnpm lock file",
		},
		{
			name:   "bar_prefixed_thinking",
			output: "┃  Thinking: analyzing the codebase",
		},
		{
			name:   "arrow_read",
			output: "→ Read MODULE.bazel",
		},
		{
			name:   "arrow_write",
			output: "→ Write src/main.go",
		},
		{
			name:   "arrow_edit",
			output: "→ Edit config.yaml",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			status := detector.Detect([]byte(tc.output))
			if status != StatusProcessing {
				t.Errorf("Detect(%q) returned %v, expected StatusProcessing", tc.output, status)
			}
		})
	}
}

// TestOpencode_PermissionButtons verifies OpenCode permission dialog detection.
func TestOpencode_PermissionButtons(t *testing.T) {
	detector := NewStatusDetector()

	testCases := []struct {
		name   string
		output string
	}{
		{
			name:   "bar_prefixed_permission",
			output: "┃  Allow once   Allow always   Reject",
		},
		{
			name:   "permission_reversed_order",
			output: "Reject   Allow always   Allow once",
		},
		{
			name:   "permission_no_bar",
			output: "Allow once   Allow always   Reject",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			status := detector.Detect([]byte(tc.output))
			if status != StatusInputRequired {
				t.Errorf("Detect(%q) returned %v, expected StatusInputRequired", tc.output, status)
			}
		})
	}
}
