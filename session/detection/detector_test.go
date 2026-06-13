package detection

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// ── agy (Antigravity CLI) coverage tests (REQ-4) ────────────────────────────

// GeminiPatterns_should_matchAgyCandidateStrings_When_agySampleOutputProvided
// agy uses the same TUI codebase as Gemini CLI. Verifies that at least 4 gemini_*
// patterns exist in getDefaultPatterns() and that distinctive agy TUI strings are
// classified correctly by the standard detector.
func TestGeminiPatterns_AgyCoverage(t *testing.T) {
	patterns := getDefaultPatterns()

	// Count gemini_* patterns across all categories.
	geminiCount := 0
	allGroups := [][]StatusPattern{patterns.Ready, patterns.Processing, patterns.NeedsApproval, patterns.Error}
	for _, group := range allGroups {
		for _, p := range group {
			if strings.Contains(p.Name, "gemini") {
				geminiCount++
			}
		}
	}
	if geminiCount < 4 {
		t.Errorf("expected ≥4 gemini_* patterns (gemini_ready, gemini_working, gemini_permission, gemini_allow_execution), got %d", geminiCount)
	}

	// Verify that distinctive agy/Gemini TUI lines produce the expected status.
	// These lines are unambiguous for their category (not caught by the catch-all).
	sd := NewStatusDetector()
	distinctiveCases := []struct {
		line   string
		status DetectedStatus
	}{
		{"╰─ Yes, allow once", StatusNeedsApproval},
		{"Allow execution of: ls /tmp", StatusNeedsApproval},
		// "✦ Working..." now returns StatusActive: ✦ was added to claude_thinking_verb
		// (the Claude Code spinner pattern), which fires before gemini_working (Processing).
		// StatusActive is correct — Gemini/agy is actively processing when showing this line.
		{"✦ Working...", StatusActive},
	}
	for _, tc := range distinctiveCases {
		t.Run(tc.line, func(t *testing.T) {
			got := sd.Detect([]byte(tc.line))
			if got != tc.status {
				t.Errorf("Detect(%q) = %v, want %v", tc.line, got, tc.status)
			}
		})
	}
}

// GeminiPatterns_should_returnNeedsApproval_When_permissionPromptPresent
// Ensures the gemini_permission pattern fires for the agy "Yes, allow once" permission prompt.
func TestGeminiPatterns_NeedsApprovalState(t *testing.T) {
	sd := NewStatusDetector()
	// These strings appear in both Gemini CLI and agy (shared TUI codebase).
	permissionLines := []string{
		"╰─ Yes, allow once",
		"Yes, allow once",
		"Allow execution of: rm -rf /tmp/test",
	}
	for _, line := range permissionLines {
		t.Run(line, func(t *testing.T) {
			status := sd.Detect([]byte(line))
			if status != StatusNeedsApproval {
				t.Errorf("Detect(%q) = %v, want StatusNeedsApproval", line, status)
			}
		})
	}
}

// AgyCoverage_should_haveCommentInDetectorGo_When_noAgySpecificPatternsExist
// Canary test: if the agy coverage comment is removed from detector.go or the patterns
// diverge, this test fails to alert future maintainers.
func TestDetector_AgyCoverageCommentPresent(t *testing.T) {
	src, err := os.ReadFile("detector.go")
	if err != nil {
		t.Fatalf("failed to read detector.go: %v", err)
	}
	if !strings.Contains(string(src), "agy (Antigravity CLI)") {
		t.Error("detector.go is missing the 'agy (Antigravity CLI)' coverage comment; " +
			"if agy patterns were intentionally removed or renamed, update this test and the comment")
	}
}

// TestStatusDetector_DetectActive_StarFourPointed verifies that ✦ (U+2726 BLACK FOUR POINTED
// STAR) — Claude Code's primary thinking spinner — is detected as StatusActive.
// Regression test for the gap where claude_thinking_verb lacked ✦ in its char class.
func TestStatusDetector_DetectActive_StarFourPointed(t *testing.T) {
	sd := NewStatusDetector()
	testCases := []struct {
		input string
		desc  string
	}{
		{
			"✦ Thinking… (2m 5s · ↓ 6.4k tokens)\n",
			"claude primary spinner + Thinking verb + ellipsis",
		},
		{
			"  ✦ Searching…\n",
			"indented star-four-pointed + verb + ellipsis",
		},
		{
			"✦ Compiling...\n",
			"dot ellipsis variant (3 dots)",
		},
		{
			"✦ Ruminating… (20s · ↓ 1.2k tokens)\n",
			"random thinking verb (Ruminating)",
		},
		{
			"✦ Pondering.\n",
			"single dot ellipsis",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			status := sd.Detect([]byte(tc.input))
			if status != StatusActive {
				t.Errorf("Detect(%q) = %v, want StatusActive", tc.input, status)
			}
		})
	}
}

// TestStatusDetector_DetectActive_ScreenOverwrite verifies that bare \r (carriage return
// not followed by \n) and ANSI cursor-up sequences are detected as StatusActive when
// no text-based pattern matched. This catches spinner-based TUIs that animate via CR.
func TestStatusDetector_DetectActive_ScreenOverwrite(t *testing.T) {
	sd := NewStatusDetector()

	t.Run("pure CR spinner — no keywords", func(t *testing.T) {
		input := "⠋ Thinking\r⠙ Thinking\r⠹ Thinking\n"
		status := sd.Detect([]byte(input))
		if status != StatusActive {
			t.Errorf("Detect(CR spinner) = %v, want StatusActive", status)
		}
	})

	t.Run("ANSI cursor-up with non-keyword content triggers screen-overwrite", func(t *testing.T) {
		// "Working..." would match the Processing text pattern, so use content with
		// no keyword match — screen-overwrite fires before the Ready catch-all.
		input := "......\x1b[A......\n"
		status := sd.Detect([]byte(input))
		if status != StatusActive {
			t.Errorf("Detect(cursor-up) = %v, want StatusActive", status)
		}
	})

	t.Run("Windows CRLF newlines must NOT trigger screen-overwrite", func(t *testing.T) {
		input := "Normal line\r\nAnother line\r\n"
		status := sd.Detect([]byte(input))
		if status == StatusActive {
			t.Errorf("Detect(CRLF newlines) = StatusActive; CRLF is a line ending, not a screen overwrite")
		}
	})

	t.Run("higher-priority error pattern wins over screen-overwrite", func(t *testing.T) {
		// Error text must be on a separate line (\n) so CR collapse doesn't erase it.
		// "Error: ...\r⠋ Retrying\r" would collapse the error text away (CR overwrite);
		// use \n to keep both lines visible so the error pattern matches first.
		input := "Error: connection refused\n⠋ Retrying\r"
		status := sd.Detect([]byte(input))
		if status != StatusError {
			t.Errorf("Detect(error+overwrite) = %v, want StatusError (error has higher priority)", status)
		}
	})
}

// TestRecentEvents verifies the detection event ring buffer and SetSessionID.
func TestRecentEvents(t *testing.T) {
	sd := NewStatusDetector()
	sd.SetSessionID("test-session")

	// No events yet
	if got := sd.RecentEvents(10); len(got) != 0 {
		t.Errorf("expected 0 events before any detection, got %d", len(got))
	}

	// Run a detection — should produce one event
	_ = sd.Detect([]byte("✦ Thinking…\n"))
	events := sd.RecentEvents(10)
	if len(events) != 1 {
		t.Fatalf("expected 1 event after 1 detection, got %d", len(events))
	}
	if events[0].SessionID != "test-session" {
		t.Errorf("event.SessionID = %q, want %q", events[0].SessionID, "test-session")
	}
	if events[0].ResultStatus != StatusActive {
		t.Errorf("event.ResultStatus = %v, want StatusActive", events[0].ResultStatus)
	}
	if events[0].MatchedPattern == "" {
		t.Error("event.MatchedPattern should not be empty for a matched pattern")
	}

	// No-match case: TailSnippet should be populated
	sd2 := NewStatusDetector()
	_ = sd2.Detect([]byte("some unrecognized output"))
	ev2 := sd2.RecentEvents(1)
	if len(ev2) == 0 {
		t.Fatal("expected at least 1 event from no-match detection")
	}
	if ev2[0].TailSnippet == "" {
		t.Error("TailSnippet should be non-empty for no-match events (needed for debugging)")
	}
	if ev2[0].MatchedPattern != "<none>" && ev2[0].MatchedPattern != "claude_prompt" {
		// claude_prompt has a catch-all ".*" ready pattern that may match — both are acceptable
		t.Logf("no-match MatchedPattern = %q (acceptable)", ev2[0].MatchedPattern)
	}
}

// TestEventRing_ConcurrentPushRecent verifies that concurrent calls to Detect and
// RecentEvents do not trigger data races on the ring buffer or the sessionID field.
// Run with: go test ./session/detection/... -race
func TestEventRing_ConcurrentPushRecent(t *testing.T) {
	sd := NewStatusDetector()
	sd.SetSessionID("race-test")

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = sd.Detect([]byte("✦ Thinking…\n"))
				_ = sd.RecentEvents(10)
			}
		}()
	}
	wg.Wait()
}
