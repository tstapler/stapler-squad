package session

import (
	"strings"
	"testing"
)

// TestStatusDetector_DetectInputRequired verifies that explicit user input prompts are detected correctly.
func TestStatusDetector_DetectInputRequired(t *testing.T) {
	detector := NewStatusDetector()

	tests := []struct {
		name   string
		output string
	}{
		{
			name:   "explicit_input_prompt - enter",
			output: "Please enter your name:",
		},
		{
			name:   "explicit_input_prompt - type",
			output: "Type the API key:",
		},
		{
			name:   "explicit_input_prompt - provide",
			output: "Provide your email address:",
		},
		{
			name:   "explicit_input_prompt - input",
			output: "Input the configuration value:",
		},
		{
			name:   "explicit_input_prompt - specify",
			output: "Specify the port number:",
		},
		{
			name:   "question_prompt - what",
			output: "What database should I use?",
		},
		{
			name:   "question_prompt - which",
			output: "Which branch should I merge?",
		},
		{
			name:   "question_prompt - how",
			output: "How should I proceed?",
		},
		{
			name:   "question_prompt - when",
			output: "When should this run?",
		},
		{
			name:   "question_prompt - where",
			output: "Where should I save the file?",
		},
		{
			name:   "please_provide - provide",
			output: "Please provide the authentication token",
		},
		{
			name:   "please_provide - enter",
			output: "Please enter your credentials",
		},
		{
			name:   "please_provide - type",
			output: "Please type your password",
		},
		{
			name:   "please_provide - specify",
			output: "Please specify the configuration",
		},
		{
			name:   "please_provide - give",
			output: "Please give me the API endpoint",
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

// TestStatusDetector_DetectInputRequired_WithContext verifies context includes matched pattern.
func TestStatusDetector_DetectInputRequired_WithContext(t *testing.T) {
	detector := NewStatusDetector()

	tests := []struct {
		name            string
		output          string
		expectedPattern string
	}{
		{
			name:            "explicit_input_prompt",
			output:          "Please enter your username:",
			expectedPattern: "explicit_input_prompt",
		},
		{
			name:            "question_prompt",
			output:          "Which file should I modify?",
			expectedPattern: "question_prompt",
		},
		{
			name:            "please_provide",
			output:          "Please provide the configuration file path",
			expectedPattern: "please_provide",
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
			// Context should mention the pattern name
			if !strings.Contains(context, tt.expectedPattern) {
				t.Errorf("Expected context to contain pattern name '%s', got: %s", tt.expectedPattern, context)
			}
		})
	}
}

// TestStatusDetector_InputRequired_CaseInsensitive verifies patterns are case-insensitive.
func TestStatusDetector_InputRequired_CaseInsensitive(t *testing.T) {
	detector := NewStatusDetector()

	tests := []string{
		"ENTER YOUR NAME:",
		"Enter Your Name:",
		"enter your name:",
		"WHAT DATABASE SHOULD I USE?",
		"What Database Should I Use?",
		"what database should i use?",
		"PLEASE PROVIDE THE TOKEN",
		"Please Provide The Token",
		"please provide the token",
	}

	for _, output := range tests {
		t.Run(output, func(t *testing.T) {
			status := detector.Detect([]byte(output))
			if status != StatusInputRequired {
				t.Errorf("Expected StatusInputRequired for case-insensitive match: %s, got %s", output, status.String())
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
			output:         "Error: connection failed. Please enter retry count:",
			expectedStatus: StatusError,
			description:    "Error has higher priority than InputRequired",
		},
		{
			name:           "approval_overrides_input_required",
			output:         "Yes, allow reading. Please provide file path:",
			expectedStatus: StatusNeedsApproval,
			description:    "NeedsApproval has higher priority than InputRequired",
		},
		{
			name:           "input_required_overrides_active",
			output:         "(esc to interrupt) Please enter your choice:",
			expectedStatus: StatusInputRequired,
			description:    "InputRequired has higher priority than Active",
		},
		{
			name:           "input_required_overrides_idle",
			output:         "— INSERT — Please type your response:",
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

	expectedPatterns := []string{"explicit_input_prompt", "question_prompt", "please_provide"}
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

	if !detector.HasPattern(StatusInputRequired, "explicit_input_prompt") {
		t.Error("Expected 'explicit_input_prompt' pattern to exist for StatusInputRequired")
	}
	if !detector.HasPattern(StatusInputRequired, "question_prompt") {
		t.Error("Expected 'question_prompt' pattern to exist for StatusInputRequired")
	}
	if !detector.HasPattern(StatusInputRequired, "please_provide") {
		t.Error("Expected 'please_provide' pattern to exist for StatusInputRequired")
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