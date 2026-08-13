package session

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// writeTestJSONL writes a JSONL conversation file to a temp path and returns that path.
// Each pair is a user+assistant turn. The JSONL format matches Claude conversation files.
func writeTestJSONL(t *testing.T, turns []struct{ role, content string }) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "history-*.jsonl")
	if err != nil {
		t.Fatalf("failed to create temp JSONL file: %v", err)
	}
	defer f.Close() //nolint:errcheck

	for _, turn := range turns {
		// Write a minimal JSON line that extractMsgContent can parse.
		// The conversationMessage struct expects type + message.role + message.content.
		line := fmt.Sprintf(
			`{"type":%q,"message":{"role":%q,"content":%q},"uuid":"test-uuid","timestamp":"2024-01-01T00:00:00Z"}`,
			turn.role, turn.role, turn.content,
		)
		if _, err := fmt.Fprintln(f, line); err != nil {
			t.Fatalf("failed to write JSONL line: %v", err)
		}
	}
	return f.Name()
}

// UT-6: TestBuildContinuationPrompt_WithJSONL — reads real JSONL and includes last assistant msg.
func TestBuildContinuationPrompt_WithJSONL(t *testing.T) {
	turns := []struct{ role, content string }{
		{"user", "Please fix the bug in main.go"},
		{"assistant", "I'll start by examining the file."},
		{"user", "What did you find?"},
		{"assistant", "I found a nil pointer dereference on line 42."},
		{"user", "Can you fix it?"},
		{"assistant", "I have fixed the nil pointer by adding a guard check."},
	}

	path := writeTestJSONL(t, turns)
	inst := &Instance{Title: "test-jsonl", HistoryFilePath: path}

	got := buildContinuationPrompt(inst)
	if got == "" {
		t.Fatal("buildContinuationPrompt returned empty string")
	}
	if !strings.Contains(got, "I have fixed the nil pointer") {
		t.Errorf("expected continuation prompt to contain last assistant message, got:\n%s", got)
	}
	if !strings.Contains(got, "continue") {
		t.Errorf("expected continuation prompt to mention 'continue', got:\n%s", got)
	}
}

// UT-7: TestBuildContinuationPrompt_TruncatesLongMessage — 1000-char message is truncated at 500.
func TestBuildContinuationPrompt_TruncatesLongMessage(t *testing.T) {
	longContent := strings.Repeat("x", 1000)
	turns := []struct{ role, content string }{
		{"assistant", longContent},
	}

	path := writeTestJSONL(t, turns)
	inst := &Instance{Title: "test-truncate", HistoryFilePath: path}

	got := buildContinuationPrompt(inst)
	if !strings.Contains(got, "...") {
		t.Errorf("expected truncated prompt to contain '...', got:\n%s", got)
	}

	// Find the truncated portion: it should start with 500 'x' chars followed by "..."
	expected := strings.Repeat("x", driverContinuationMaxChars) + "..."
	if !strings.Contains(got, expected) {
		t.Errorf("expected prompt to contain %d 'x' chars followed by '...'; prompt was:\n%s", driverContinuationMaxChars, got)
	}
}

// UT-8: TestBuildContinuationPrompt_NoAssistantMessage — only user messages → generic fallback.
func TestBuildContinuationPrompt_NoAssistantMessage(t *testing.T) {
	turns := []struct{ role, content string }{
		{"user", "Hello, fix this bug."},
		{"user", "Are you there?"},
	}

	path := writeTestJSONL(t, turns)
	inst := &Instance{Title: "test-no-assistant", HistoryFilePath: path}

	got := buildContinuationPrompt(inst)
	if got == "" {
		t.Fatal("buildContinuationPrompt returned empty string")
	}
	// Should fall back to generic message, not panic.
	if !strings.Contains(strings.ToLower(got), "continue") {
		t.Errorf("expected fallback to mention 'continue', got: %q", got)
	}
}
