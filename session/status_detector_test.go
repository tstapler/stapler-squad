package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewStatusDetector(t *testing.T) {
	sd := NewStatusDetector()
	if sd == nil {
		t.Fatal("NewStatusDetector() returned nil")
	}

	// Verify default patterns are loaded
	if len(sd.readyRegexes) == 0 {
		t.Error("No ready patterns loaded")
	}
	if len(sd.processingRegexes) == 0 {
		t.Error("No processing patterns loaded")
	}
	if len(sd.needsApprovalRegexes) == 0 {
		t.Error("No needs_approval patterns loaded")
	}
	if len(sd.errorRegexes) == 0 {
		t.Error("No error patterns loaded")
	}
}

func TestStatusDetector_DetectReady(t *testing.T) {
	sd := NewStatusDetector()

	// Any output should match the catch-all ready pattern if no other patterns match
	output := []byte("$ ")
	status := sd.Detect(output)
	if status != StatusReady {
		t.Errorf("Detect() returned %v, expected StatusReady", status)
	}
}

func TestStatusDetector_DetectProcessing(t *testing.T) {
	sd := NewStatusDetector()

	testCases := []string{
		"Thinking about your request...",
		"Processing the command",
		"Analyzing the code",
		"Working on it",
		"Reading file.txt",
		"Writing to output.log",
		"Executing the script",
	}

	for _, output := range testCases {
		status := sd.Detect([]byte(output))
		if status != StatusProcessing {
			t.Errorf("Detect(%q) returned %v, expected StatusProcessing", output, status)
		}
	}
}

func TestStatusDetector_DetectNeedsApproval(t *testing.T) {
	sd := NewStatusDetector()

	testCases := []string{
		"Yes, allow reading this file",
		"Yes, allow writing to this file",
		"Yes, allow once",
		"No, and tell Claude what to do differently",
		"Do you want to proceed?",
		"(Y)es/(N)o/(D)on't ask again",
	}

	for _, output := range testCases {
		status := sd.Detect([]byte(output))
		if status != StatusNeedsApproval {
			t.Errorf("Detect(%q) returned %v, expected StatusNeedsApproval", output, status)
		}
	}
}

func TestStatusDetector_DetectError(t *testing.T) {
	sd := NewStatusDetector()

	testCases := []string{
		"Error: file not found",
		"Failed to connect",
		"Exception occurred",
		"Fatal error",
		"Connection refused",
		"Network timeout",
	}

	for _, output := range testCases {
		status := sd.Detect([]byte(output))
		if status != StatusError {
			t.Errorf("Detect(%q) returned %v, expected StatusError", output, status)
		}
	}
}

func TestStatusDetector_PriorityOrder(t *testing.T) {
	sd := NewStatusDetector()

	// Error patterns should take priority over processing patterns
	output := []byte("Error while processing")
	status := sd.Detect(output)
	if status != StatusError {
		t.Errorf("Detect() returned %v, expected StatusError (priority test)", status)
	}

	// Approval should take priority over processing
	output = []byte("Reading file. Do you want to proceed?")
	status = sd.Detect(output)
	if status != StatusNeedsApproval {
		t.Errorf("Detect() returned %v, expected StatusNeedsApproval (priority test)", status)
	}
}

func TestStatusDetector_DetectWithContext(t *testing.T) {
	sd := NewStatusDetector()

	output := []byte("Error: connection refused")
	status, context := sd.DetectWithContext(output)

	if status != StatusError {
		t.Errorf("DetectWithContext() returned status %v, expected StatusError", status)
	}

	if context == "" {
		t.Error("DetectWithContext() returned empty context")
	}

	// Context should mention the pattern that matched
	if len(context) < 10 {
		t.Errorf("DetectWithContext() context too short: %s", context)
	}
}

func TestStatusDetector_DetectUnknown(t *testing.T) {
	sd := NewStatusDetector()

	// Remove the catch-all ready pattern for this test
	sd.readyRegexes = nil

	output := []byte("Some random output that doesn't match any pattern xyz123")
	status := sd.Detect(output)
	if status != StatusUnknown {
		t.Errorf("Detect() returned %v, expected StatusUnknown", status)
	}
}

func TestStatusDetector_LoadPatterns(t *testing.T) {
	// Create temporary YAML file
	tmpDir := t.TempDir()
	patternsFile := filepath.Join(tmpDir, "patterns.yaml")

	yamlContent := `
ready:
  - name: test_ready
    pattern: "ready>"
    description: "Test ready pattern"
    priority: 1

processing:
  - name: test_processing
    pattern: "test_processing"
    description: "Test processing pattern"
    priority: 10

needs_approval:
  - name: test_approval
    pattern: "approve\\?"
    description: "Test approval pattern"
    priority: 20

error:
  - name: test_error
    pattern: "test_error"
    description: "Test error pattern"
    priority: 30
`

	if err := os.WriteFile(patternsFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to create test patterns file: %v", err)
	}

	sd := NewStatusDetector()
	if err := sd.LoadPatterns(patternsFile); err != nil {
		t.Fatalf("LoadPatterns() failed: %v", err)
	}

	// Test loaded patterns
	if status := sd.Detect([]byte("ready>")); status != StatusReady {
		t.Errorf("Loaded pattern 'ready>' not working, got status %v", status)
	}

	if status := sd.Detect([]byte("test_processing")); status != StatusProcessing {
		t.Errorf("Loaded pattern 'test_processing' not working, got status %v", status)
	}

	if status := sd.Detect([]byte("approve?")); status != StatusNeedsApproval {
		t.Errorf("Loaded pattern 'approve?' not working, got status %v", status)
	}

	if status := sd.Detect([]byte("test_error")); status != StatusError {
		t.Errorf("Loaded pattern 'test_error' not working, got status %v", status)
	}
}

func TestNewStatusDetectorFromFile(t *testing.T) {
	// Create temporary YAML file
	tmpDir := t.TempDir()
	patternsFile := filepath.Join(tmpDir, "patterns.yaml")

	yamlContent := `
ready:
  - name: custom_ready
    pattern: "custom>"
    description: "Custom ready pattern"
    priority: 1

processing: []
needs_approval: []
error: []
`

	if err := os.WriteFile(patternsFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to create test patterns file: %v", err)
	}

	sd, err := NewStatusDetectorFromFile(patternsFile)
	if err != nil {
		t.Fatalf("NewStatusDetectorFromFile() failed: %v", err)
	}

	if status := sd.Detect([]byte("custom>")); status != StatusReady {
		t.Errorf("Pattern from file not working, got status %v", status)
	}
}

func TestStatusDetector_LoadPatternsInvalidFile(t *testing.T) {
	sd := NewStatusDetector()
	err := sd.LoadPatterns("/nonexistent/patterns.yaml")
	if err == nil {
		t.Error("LoadPatterns() should fail with nonexistent file")
	}
}

func TestStatusDetector_LoadPatternsInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	patternsFile := filepath.Join(tmpDir, "invalid.yaml")

	invalidYAML := `
ready: [this is not valid yaml
`

	if err := os.WriteFile(patternsFile, []byte(invalidYAML), 0644); err != nil {
		t.Fatalf("Failed to create invalid YAML file: %v", err)
	}

	sd := NewStatusDetector()
	err := sd.LoadPatterns(patternsFile)
	if err == nil {
		t.Error("LoadPatterns() should fail with invalid YAML")
	}
}

func TestStatusDetector_LoadPatternsInvalidRegex(t *testing.T) {
	tmpDir := t.TempDir()
	patternsFile := filepath.Join(tmpDir, "invalid_regex.yaml")

	yamlContent := `
ready:
  - name: bad_regex
    pattern: "(?P<invalid"
    description: "Invalid regex pattern"
    priority: 1

processing: []
needs_approval: []
error: []
`

	if err := os.WriteFile(patternsFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to create test patterns file: %v", err)
	}

	sd := NewStatusDetector()
	err := sd.LoadPatterns(patternsFile)
	if err == nil {
		t.Error("LoadPatterns() should fail with invalid regex")
	}
}

func TestStatusDetector_ExportPatterns(t *testing.T) {
	sd := NewStatusDetector()

	tmpDir := t.TempDir()
	exportFile := filepath.Join(tmpDir, "exported.yaml")

	if err := sd.ExportPatterns(exportFile); err != nil {
		t.Fatalf("ExportPatterns() failed: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(exportFile); os.IsNotExist(err) {
		t.Error("ExportPatterns() did not create file")
	}

	// Try loading the exported patterns
	sd2, err := NewStatusDetectorFromFile(exportFile)
	if err != nil {
		t.Fatalf("Failed to load exported patterns: %v", err)
	}

	// Verify patterns work the same
	testOutput := []byte("thinking about it")
	if sd.Detect(testOutput) != sd2.Detect(testOutput) {
		t.Error("Exported patterns don't match original")
	}
}

func TestStatusDetector_GetPatternNames(t *testing.T) {
	sd := NewStatusDetector()

	readyNames := sd.GetPatternNames(StatusReady)
	if len(readyNames) == 0 {
		t.Error("GetPatternNames(StatusReady) returned empty slice")
	}

	processingNames := sd.GetPatternNames(StatusProcessing)
	if len(processingNames) == 0 {
		t.Error("GetPatternNames(StatusProcessing) returned empty slice")
	}

	unknownNames := sd.GetPatternNames(StatusUnknown)
	if unknownNames != nil {
		t.Error("GetPatternNames(StatusUnknown) should return nil")
	}
}

func TestStatusDetector_DetectFromString(t *testing.T) {
	sd := NewStatusDetector()

	status := sd.DetectFromString("Error occurred")
	if status != StatusError {
		t.Errorf("DetectFromString() returned %v, expected StatusError", status)
	}
}

func TestStatusDetector_DetectFromLines(t *testing.T) {
	sd := NewStatusDetector()

	lines := []string{
		"Starting process",
		"Processing data",
		"Error: failed",
	}

	// Should detect most recent matching status (Error in this case)
	status := sd.DetectFromLines(lines)
	if status != StatusError {
		t.Errorf("DetectFromLines() returned %v, expected StatusError", status)
	}

	// Test with only processing indicators
	lines = []string{
		"Starting",
		"Processing data",
		"Working on it",
	}
	status = sd.DetectFromLines(lines)
	if status != StatusProcessing {
		t.Errorf("DetectFromLines() returned %v, expected StatusProcessing", status)
	}
}

func TestStatusDetector_DetectRecent(t *testing.T) {
	sd := NewStatusDetector()

	output := []byte("Some old output that we don't care about. Error: failed")

	// Detect from last 20 bytes
	status := sd.DetectRecent(output, 20)
	if status != StatusError {
		t.Errorf("DetectRecent() returned %v, expected StatusError", status)
	}

	// Detect from last 5 bytes (shouldn't match)
	status = sd.DetectRecent(output, 5)
	// "ailed" shouldn't match error pattern
	if status == StatusError {
		t.Errorf("DetectRecent() with 5 bytes should not detect error")
	}
}

func TestStatusDetector_HasPattern(t *testing.T) {
	sd := NewStatusDetector()

	// Test existing pattern
	if !sd.HasPattern(StatusError, "error_message") {
		t.Error("HasPattern() should return true for existing pattern")
	}

	// Test non-existing pattern
	if sd.HasPattern(StatusError, "nonexistent_pattern") {
		t.Error("HasPattern() should return false for non-existing pattern")
	}

	// Test case insensitivity
	if !sd.HasPattern(StatusError, "ERROR_MESSAGE") {
		t.Error("HasPattern() should be case insensitive")
	}
}

func TestStatusString(t *testing.T) {
	testCases := []struct {
		status   DetectedStatus
		expected string
	}{
		{StatusReady, "Ready"},
		{StatusProcessing, "Processing"},
		{StatusNeedsApproval, "Needs Approval"},
		{StatusError, "Error"},
		{StatusUnknown, "Unknown"},
	}

	for _, tc := range testCases {
		result := tc.status.String()
		if result != tc.expected {
			t.Errorf("Status %v String() = %q, expected %q", tc.status, result, tc.expected)
		}
	}
}

func TestStatusDetector_MultilinePatterns(t *testing.T) {
	sd := NewStatusDetector()

	// Test that patterns work across multiple lines
	output := []byte(`
Some output here
Do you want to proceed?
Yes or no
`)

	status := sd.Detect(output)
	if status != StatusNeedsApproval {
		t.Errorf("Detect() with multiline output returned %v, expected StatusNeedsApproval", status)
	}
}

func TestStatusDetector_EmptyOutput(t *testing.T) {
	sd := NewStatusDetector()

	status := sd.Detect([]byte(""))
	// Empty output might match catch-all ready pattern or be unknown
	// depending on pattern configuration
	if status != StatusReady && status != StatusUnknown {
		t.Errorf("Detect() with empty output returned %v", status)
	}
}

func TestStatusDetector_CaseInsensitivity(t *testing.T) {
	sd := NewStatusDetector()

	// Test case variations
	testCases := []string{
		"ERROR occurred",
		"error occurred",
		"ErRoR occurred",
		"ERROR OCCURRED",
	}

	for _, output := range testCases {
		status := sd.Detect([]byte(output))
		if status != StatusError {
			t.Errorf("Detect(%q) returned %v, expected StatusError (case insensitive)", output, status)
		}
	}
}

func Benchmark_StatusDetector_Detect(b *testing.B) {
	sd := NewStatusDetector()
	output := []byte("Processing your request... thinking about the best approach")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sd.Detect(output)
	}
}

func Benchmark_StatusDetector_DetectWithContext(b *testing.B) {
	sd := NewStatusDetector()
	output := []byte("Error: connection refused")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sd.DetectWithContext(output)
	}
}
