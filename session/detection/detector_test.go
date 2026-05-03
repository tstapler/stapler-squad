package detection

import (
	"os"
	"path/filepath"
	"strings"
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

	// Test catch-all ready pattern with generic output that doesn't match other patterns
	// Note: "$ " matches StatusIdle (command_prompt pattern), not StatusReady
	output := []byte("some generic terminal output")
	status := sd.Detect(output)
	if status != StatusReady {
		t.Errorf("Detect() returned %v, expected StatusReady", status)
	}
}

func TestStatusDetector_DetectIdle(t *testing.T) {
	sd := NewStatusDetector()

	testCases := []string{
		"$ ",         // Shell command prompt
		"— INSERT —", // Vim INSERT mode
		"— NORMAL —", // Vim NORMAL mode
	}

	for _, output := range testCases {
		status := sd.Detect([]byte(output))
		if status != StatusIdle {
			t.Errorf("Detect(%q) returned %v, expected StatusIdle", output, status)
		}
	}
}

func TestStatusDetector_DetectActive(t *testing.T) {
	sd := NewStatusDetector()

	testCases := []string{
		"(esc to interrupt)",
		"Running...",
		"⠋ Processing files...",
		"Executing tests (esc to cancel)",
	}

	for _, output := range testCases {
		status := sd.Detect([]byte(output))
		if status != StatusActive {
			t.Errorf("Detect(%q) returned %v, expected StatusActive", output, status)
		}
	}
}

func TestStatusDetector_DetectSuccess(t *testing.T) {
	sd := NewStatusDetector()

	testCases := []string{
		"✓ Successfully completed the task",
		"Task completed",
		"I've completed the work",
		"All done!",
		"✓ Build complete",
		"Finished successfully",
		"All tests passed",
		"Build succeeded",
	}

	for _, output := range testCases {
		status := sd.Detect([]byte(output))
		if status != StatusSuccess {
			t.Errorf("Detect(%q) returned %v, expected StatusSuccess", output, status)
		}
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
		"ERROR: Something went wrong",
		"Exception: NullPointerException",
		"Fatal error: cannot continue",
		"Connection refused",
		"Network timeout",
		"Traceback (most recent call last):",
		"panic: runtime error",
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

	// Remove the catch-all ready pattern for this test.
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
		{StatusIdle, "Idle"},
		{StatusActive, "Active"},
		{StatusSuccess, "Success"},
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

// TestStatusDetector_VerbDurationCompletion verifies the ✻ <verb> for <duration> pattern.
// These are real output lines from Claude's task-completion summary.
func TestStatusDetector_VerbDurationCompletion(t *testing.T) {
	sd := NewStatusDetector()

	tests := []struct {
		name   string
		output string
	}{
		{name: "cooked_minutes_seconds", output: "✻ Cooked for 1m 5s"},
		{name: "crunched_minutes_seconds", output: "✻ Crunched for 1m 14s"},
		{name: "baked_seconds", output: "✻ Baked for 30s"},
		{name: "thinking_hours", output: "✻ Thinking for 2h"},
		{name: "embedded_in_multiline", output: "⏺ Agent \"Security review\" completed\n✻ Cooked for 1m 5s\n● How is Claude doing this session?"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := sd.Detect([]byte(tt.output))
			if status != StatusSuccess {
				t.Errorf("Detect(%q) = %v, want StatusSuccess", tt.output, status)
			}
		})
	}
}

// TestStatusDetector_VerbDurationNoFalsePositives ensures we don't match similar-looking text.
func TestStatusDetector_VerbDurationNoFalsePositives(t *testing.T) {
	sd := NewStatusDetector()

	tests := []struct {
		name   string
		output string
	}{
		{name: "waiting_for_no_unit", output: "✻ waiting for something"},
		{name: "no_sparkle", output: "Cooked for 1m 5s"},
		{name: "thinking_no_duration", output: "∴ Thinking..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := sd.Detect([]byte(tt.output))
			if status == StatusSuccess {
				t.Errorf("False positive: Detect(%q) = StatusSuccess, want non-Success", tt.output)
			}
		})
	}
}

// TestStatusDetector_VerbDurationPriorityOverActive verifies success wins over active patterns
// when both appear in the same content (old scrollback with Thinking... + final completion line).
func TestStatusDetector_VerbDurationPriorityOverActive(t *testing.T) {
	sd := NewStatusDetector()

	// Simulates real terminal: old "Thinking..." in scrollback + completion line at bottom
	output := "∴ Thinking...\nRunning... (esc to interrupt)\n✻ Cooked for 1m 5s"
	status := sd.Detect([]byte(output))
	if status != StatusSuccess {
		t.Errorf("Detect() = %v, want StatusSuccess (success should beat active in priority order)", status)
	}
}

// TestCollapseCarriageReturns_SpinnerSequence verifies that CR-overwritten lines are
// collapsed so old spinner frames don't confuse pattern matching.
// The stale frame "⠋ Thinking" must not appear in the result.
func TestCollapseCarriageReturns_SpinnerSequence(t *testing.T) {
	input := "⠋ Thinking\r⠙ Thinking"
	got := collapseCarriageReturns(input)
	if strings.Contains(got, "⠋ Thinking") {
		t.Errorf("collapseCarriageReturns(%q) = %q — stale frame should not appear", input, got)
	}
	if !strings.Contains(got, "⠙ Thinking") {
		t.Errorf("collapseCarriageReturns(%q) = %q — final frame should be present", input, got)
	}
}

func TestCollapseCarriageReturns_CRLFPreserved(t *testing.T) {
	input := "line1\r\nline2"
	got := collapseCarriageReturns(input)
	if got != input {
		t.Errorf("collapseCarriageReturns(%q) = %q, want unchanged %q", input, got, input)
	}
}

func TestCollapseCarriageReturns_EmptyInput(t *testing.T) {
	got := collapseCarriageReturns("")
	if got != "" {
		t.Errorf("collapseCarriageReturns(\"\") = %q, want empty", got)
	}
}

func TestCollapseCarriageReturns_NoCR(t *testing.T) {
	input := "no carriage returns here\nnext line"
	got := collapseCarriageReturns(input)
	if got != input {
		t.Errorf("collapseCarriageReturns(%q) = %q, want unchanged", input, got)
	}
}

func TestStripANSI_WithCarriageReturn_FullPipeline(t *testing.T) {
	// Spinner with ANSI color codes and CR overwriting; final state is "⠙ Thinking"
	input := "\x1b[32m⠋ Thinking\x1b[0m\r\x1b[32m⠙ Thinking\x1b[0m"
	text := stripANSI(collapseCarriageReturns(input))
	want := "⠙ Thinking"
	if text != want {
		t.Errorf("pipeline result = %q, want %q", text, want)
	}
}

// TestDetectRecentDoesNotMatchStaleContent is a regression guard: stale "esc to interrupt"
// in the first 5000 bytes must not cause StatusActive when the tail window is idle.
func TestDetectRecentDoesNotMatchStaleContent(t *testing.T) {
	sd := NewStatusDetector()

	// Build a buffer: 5000 bytes of old active output, then an idle tail.
	old := make([]byte, 5000)
	for i := range old {
		old[i] = 'x'
	}
	// Embed stale active pattern in the old section.
	copy(old[100:], []byte("esc to interrupt"))

	idleTail := []byte("\n? for shortcuts\n")
	buf := append(old, idleTail...)

	status := sd.DetectRecent(buf, StatusDetectionTailBytes)
	if status == StatusActive {
		t.Errorf("DetectRecent with stale active content returned StatusActive; want StatusIdle")
	}
	if status != StatusIdle {
		t.Errorf("DetectRecent returned %v, want StatusIdle", status)
	}
}

// TestDetectRecentTailWindowBoundary verifies a buffer exactly StatusDetectionTailBytes long
// is fully scanned.
func TestDetectRecentTailWindowBoundary(t *testing.T) {
	sd := NewStatusDetector()
	buf := make([]byte, StatusDetectionTailBytes)
	for i := range buf {
		buf[i] = 'x'
	}
	copy(buf[len(buf)-20:], []byte("esc to interrupt    "))
	status := sd.DetectRecent(buf, StatusDetectionTailBytes)
	if status != StatusActive {
		t.Errorf("DetectRecent with active pattern at tail end returned %v, want StatusActive", status)
	}
}

// TestDetectRecentShorterThanWindow verifies no panic and correct detection when buffer
// is shorter than the tail window.
func TestDetectRecentShorterThanWindow(t *testing.T) {
	sd := NewStatusDetector()
	buf := []byte("esc to interrupt")
	status := sd.DetectRecent(buf, StatusDetectionTailBytes)
	if status != StatusActive {
		t.Errorf("DetectRecent (short buffer) returned %v, want StatusActive", status)
	}
}

// TestStatusDetector_DetectSuccess_CostSummaryLine verifies the cost summary pattern.
func TestStatusDetector_DetectSuccess_CostSummaryLine(t *testing.T) {
	sd := NewStatusDetector()
	cases := []string{
		"$0.42 • 3 tool uses",
		"$1.23 •",
		"$10.05 • some detail",
	}
	for _, c := range cases {
		status := sd.Detect([]byte(c))
		if status != StatusSuccess {
			t.Errorf("Detect(%q) = %v, want StatusSuccess", c, status)
		}
	}
}

// TestStatusDetector_DetectIdle_ReadlinePrompt verifies the readline prompt pattern.
func TestStatusDetector_DetectIdle_ReadlinePrompt(t *testing.T) {
	sd := NewStatusDetector()
	cases := []string{
		">\n",
		">\n? for shortcuts",
		"some output\n>\n",
	}
	for _, c := range cases {
		status := sd.Detect([]byte(c))
		if status != StatusIdle {
			t.Errorf("Detect(%q) = %v, want StatusIdle", c, status)
		}
	}
}

// TestStatusDetector_ReadlinePrompt_NotMatchedMidLine ensures "> some text" does not trigger idle.
func TestStatusDetector_ReadlinePrompt_NotMatchedMidLine(t *testing.T) {
	sd := NewStatusDetector()
	output := "> some text here"
	status := sd.Detect([]byte(output))
	if status == StatusIdle {
		t.Errorf("Detect(%q) returned StatusIdle but '> text' should not match readline prompt", output)
	}
}

// TestStatusDetector_DetectActive_ThinkingVerb verifies the claude_thinking_verb pattern.
func TestStatusDetector_DetectActive_ThinkingVerb(t *testing.T) {
	sd := NewStatusDetector()
	cases := []string{
		"* Moonwalking…",
		"* Ebbing...",
		"* Pondering.",
		"* Thinking…",
	}
	for _, c := range cases {
		status := sd.Detect([]byte(c))
		if status != StatusActive {
			t.Errorf("Detect(%q) = %v, want StatusActive", c, status)
		}
	}
}

// TestStatusDetector_DetectSuccess_VerbDurationCompletion verifies the turn-completion
// bullet pattern handles both ✻ and ◉ bullets with any past-tense verb.
func TestStatusDetector_DetectSuccess_VerbDurationCompletion(t *testing.T) {
	sd := NewStatusDetector()
	cases := []string{
		"✻ Cooked for 2m",
		"◉ Baked for 10s",
		"◉ Pondered for 45s",
		"◉ Synthesized for 1m",
		"✻ Analyzed for 3h",
	}
	for _, c := range cases {
		status := sd.Detect([]byte(c))
		if status != StatusSuccess {
			t.Errorf("Detect(%q) = %v, want StatusSuccess", c, status)
		}
	}
}

// TestDetectWithContextFromLines_StaleScrollback is the core regression guard for
// the scrollback poisoning problem: a session that completed its turn (showing
// "? for shortcuts" on the last line) must NOT be misclassified as Active because
// an old "esc to interrupt" line exists earlier in the same window.
func TestDetectWithContextFromLines_StaleScrollback(t *testing.T) {
	sd := NewStatusDetector()

	// Simulate 20 lines of terminal content: "esc to interrupt" on line 15 (stale),
	// "? for shortcuts" on the last line (current state).
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = "some output line"
	}
	lines[14] = "esc to interrupt                                   10% until auto-compact"
	lines[19] = "? for shortcuts"

	status, _ := sd.DetectWithContextFromLines(lines)
	if status != StatusIdle {
		t.Errorf("DetectWithContextFromLines with stale 'esc to interrupt' returned %v, want StatusIdle — last-line idle prompt must win", status)
	}
}

// TestDetectWithContextFromLines_ActiveWhenNoIdlePrompt verifies that Active is
// correctly returned when the last line shows an active indicator.
func TestDetectWithContextFromLines_ActiveWhenNoIdlePrompt(t *testing.T) {
	sd := NewStatusDetector()

	lines := []string{
		"some output",
		"* Moonwalking… (4m 18s · ↓ 2.0k tokens · thinking)",
		"  └ Tip: Use /btw to ask a quick side question",
		"",
		"> ▌",
		"esc to interrupt                                   10% until auto-compact",
	}

	status, _ := sd.DetectWithContextFromLines(lines)
	if status != StatusActive {
		t.Errorf("DetectWithContextFromLines with active terminal returned %v, want StatusActive", status)
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
