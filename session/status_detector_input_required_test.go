package session

import (
	"testing"
)

// TestStatusDetector_DetectInputRequired verifies that Claude Code's numbered selection prompts are detected.
func TestStatusDetector_DetectInputRequired(t *testing.T) {
	detector := NewStatusDetector()

	tests := []struct {
		name   string
		output string
	}{
		{
			name:   "numbered_option_with_arrow_selector",
			output: " ❯ 1. Yes",
		},
		{
			name:   "numbered_option_with_text",
			output: " ❯ 1. Type here to tell Claude what to do differently",
		},
		{
			name:   "ascii_arrow_selector",
			output: " > 1. Yes",
		},
		{
			name:   "three_options_with_selector",
			output: " ❯ 1. Option A\n   2. Option B\n   3. Option C",
		},
		{
			name:   "question_with_selector",
			output: "Which option?\n ❯ 1. First\n   2. Second",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := detector.Detect([]byte(tt.output))
			if status != StatusInputRequired {
				t.Errorf("Expected StatusInputRequired, got %s for output: %s", status.String(), tt.output)
			}
		})
	}
}

// TestStatusDetector_DetectInputRequired_WithContext verifies context includes matched pattern description.
func TestStatusDetector_DetectInputRequired_WithContext(t *testing.T) {
	detector := NewStatusDetector()

	tests := []struct {
		name                string
		output              string
		expectedDescription string
	}{
		{
			name:                "numbered_option_selector",
			output:              " ❯ 1. Yes",
			expectedDescription: "Selection prompt with numbered options",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, context := detector.DetectWithContext([]byte(tt.output))
			if status != StatusInputRequired {
				t.Errorf("Expected StatusInputRequired, got %s", status.String())
			}
			if context == "" {
				t.Errorf("Expected non-empty context, got empty string")
			}
			// Context should contain the pattern description
			if context != tt.expectedDescription {
				t.Errorf("Expected context '%s', got: %s", tt.expectedDescription, context)
			}
		})
	}
}

// TestStatusDetector_InputRequired_CaseInsensitive verifies patterns work regardless of surrounding text.
func TestStatusDetector_InputRequired_CaseInsensitive(t *testing.T) {
	detector := NewStatusDetector()

	tests := []string{
		" ❯ 1. YES",
		" ❯ 1. yes",
		" ❯ 1. Yes",
		" > 1. YES",
		" > 1. yes",
	}

	for _, output := range tests {
		t.Run(output, func(t *testing.T) {
			status := detector.Detect([]byte(output))
			if status != StatusInputRequired {
				t.Errorf("Expected StatusInputRequired for: %s, got %s", output, status.String())
			}
		})
	}
}

// TestStatusDetector_InputRequired_NoFalsePositives verifies we don't incorrectly match non-input text.
func TestStatusDetector_InputRequired_NoFalsePositives(t *testing.T) {
	detector := NewStatusDetector()

	tests := []struct {
		name   string
		output string
	}{
		// Generic text that should NOT match
		{
			name:   "narrative_text_with_question",
			output: "I was wondering what you think about this approach?",
		},
		{
			name:   "descriptive_text",
			output: "The system will automatically enter maintenance mode",
		},
		{
			name:   "informational_text",
			output: "Please note that the database is ready",
		},
		{
			name:   "status_message",
			output: "Currently processing your request",
		},
		{
			name:   "completion_message",
			output: "Task completed successfully",
		},
		// Previously problematic false positives
		{
			name:   "content_type_header",
			output: "Content type: markdown",
		},
		{
			name:   "file_type_metadata",
			output: "File type: go",
		},
		{
			name:   "input_type_declaration",
			output: "Input type: text",
		},
		{
			name:   "session_type_info",
			output: "Session type: worktree",
		},
		{
			name:   "mime_type",
			output: "mime-type: application/json",
		},
		{
			name:   "http_content_type",
			output: "Content-Type: text/html",
		},
		{
			name:   "type_colon_error",
			output: "type: error",
		},
		{
			name:   "task_type_analysis",
			output: "Task type: analysis",
		},
		{
			name:   "data_type_colon",
			output: "data type:",
		},
		{
			name:   "input_file_log",
			output: "Input file: test.go",
		},
		{
			name:   "input_path_log",
			output: "Input path: /home/user",
		},
		{
			name:   "enter_key_pressed",
			output: "enter: pressed",
		},
		{
			name:   "provide_context",
			output: "provide context:",
		},
		{
			name:   "log_style_input",
			output: "-- Input: /path/to/file",
		},
		{
			name:   "agent_task_description",
			output: "project-coordinator(Analyze TODO.md and recommend next step)",
		},
		{
			name:   "slash_command_running",
			output: "/plan:next-step is running...",
		},
		{
			name:   "subagent_type_declaration",
			output: "subagent_type=Explore",
		},
		// Generic question text (not numbered options)
		{
			name:   "question_what",
			output: "What database should I use?",
		},
		{
			name:   "question_which",
			output: "Which branch should I merge?",
		},
		{
			name:   "question_how",
			output: "How should I proceed?",
		},
		// Numbered lists that are NOT selection prompts
		{
			name:   "numbered_list_no_selector",
			output: "1. First item\n2. Second item",
		},
		{
			name:   "numbered_list_with_period",
			output: "Here are the steps:\n1. Do this\n2. Do that",
		},
		// Enter/type prompts without numbered options
		{
			name:   "enter_prompt_no_options",
			output: "Please enter your name:",
		},
		{
			name:   "type_prompt_no_options",
			output: "Type the API key:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := detector.Detect([]byte(tt.output))
			if status == StatusInputRequired {
				t.Errorf("False positive: Expected non-InputRequired status for: %s, got StatusInputRequired", tt.output)
			}
		})
	}
}

// TestStatusDetector_InputRequired_Priority verifies InputRequired has correct priority.
func TestStatusDetector_InputRequired_Priority(t *testing.T) {
	detector := NewStatusDetector()

	tests := []struct {
		name           string
		output         string
		expectedStatus DetectedStatus
		description    string
	}{
		{
			name:           "error_overrides_input_required",
			output:         "Error: connection failed.\n ❯ 1. Retry",
			expectedStatus: StatusError,
			description:    "Error has higher priority than InputRequired",
		},
		{
			name:           "approval_overrides_input_required",
			output:         "Yes, allow reading\n ❯ 1. Continue",
			expectedStatus: StatusNeedsApproval,
			description:    "NeedsApproval has higher priority than InputRequired",
		},
		{
			name:           "input_required_overrides_active",
			output:         "(esc to interrupt)\n ❯ 1. Yes",
			expectedStatus: StatusInputRequired,
			description:    "InputRequired has higher priority than Active",
		},
		{
			name:           "input_required_overrides_idle",
			output:         "— INSERT —\n ❯ 1. Continue",
			expectedStatus: StatusInputRequired,
			description:    "InputRequired has higher priority than Idle",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := detector.Detect([]byte(tt.output))
			if status != tt.expectedStatus {
				t.Errorf("%s: Expected %s, got %s", tt.description, tt.expectedStatus.String(), status.String())
			}
		})
	}
}

// TestStatusDetector_InputRequired_GetPatternNames verifies pattern names can be retrieved.
func TestStatusDetector_InputRequired_GetPatternNames(t *testing.T) {
	detector := NewStatusDetector()

	names := detector.GetPatternNames(StatusInputRequired)
	if len(names) == 0 {
		t.Error("Expected non-empty pattern names for StatusInputRequired")
	}

	expectedPatterns := []string{"numbered_option_selector"}
	for _, expected := range expectedPatterns {
		found := false
		for _, name := range names {
			if name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected pattern name '%s' not found in: %v", expected, names)
		}
	}
}

// TestStatusDetector_InputRequired_HasPattern verifies pattern existence check.
func TestStatusDetector_InputRequired_HasPattern(t *testing.T) {
	detector := NewStatusDetector()

	if !detector.HasPattern(StatusInputRequired, "numbered_option_selector") {
		t.Error("Expected 'numbered_option_selector' pattern to exist for StatusInputRequired")
	}

	// Old patterns should NOT exist
	if detector.HasPattern(StatusInputRequired, "explicit_input_prompt") {
		t.Error("Old 'explicit_input_prompt' pattern should not exist")
	}
	if detector.HasPattern(StatusInputRequired, "question_prompt") {
		t.Error("Old 'question_prompt' pattern should not exist")
	}
	if detector.HasPattern(StatusInputRequired, "please_provide") {
		t.Error("Old 'please_provide' pattern should not exist")
	}

	if detector.HasPattern(StatusInputRequired, "nonexistent_pattern") {
		t.Error("Expected 'nonexistent_pattern' to not exist for StatusInputRequired")
	}
}

// TestStatusDetector_InputRequired_String verifies string conversion.
func TestStatusDetector_InputRequired_String(t *testing.T) {
	status := StatusInputRequired
	expected := "Input Required"
	if status.String() != expected {
		t.Errorf("Expected StatusInputRequired.String() to return '%s', got '%s'", expected, status.String())
	}
}
