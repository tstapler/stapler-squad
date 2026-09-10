package session

import (
	"fmt"
	"os"
	"strings"
	"sync"
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// TestBuildContinuationPrompt_ConcurrentWithSetHistoryInfo guards against a
// data race: buildContinuationPrompt used to read inst.HistoryFilePath
// directly with no lock, while SetHistoryInfo (the HistoryLinker's setter)
// writes it under claudeSessionMu/i.mu. Unlike FindInstanceByHistoryPath
// (session/artifact_lookup.go), which is documented as safe because it only
// ever runs on the same goroutine that sets the field, buildContinuationPrompt
// has no such guarantee — it can run concurrently with a live HistoryLinker
// scan. Run with -race: this must fail on a raw-field read and pass once the
// read goes through Snapshot().
func TestBuildContinuationPrompt_ConcurrentWithSetHistoryInfo(t *testing.T) {
	t.Parallel()
	inst := makeTestInstance("continuation-prompt-race")

	var writerWG, readerWG, ready sync.WaitGroup
	stop := make(chan struct{})

	const numWriters = 4
	const numReaders = 4
	ready.Add(numWriters + numReaders)

	for w := 0; w < numWriters; w++ {
		writerWG.Add(1)
		go func(w int) {
			defer writerWG.Done()
			ready.Done()
			ready.Wait() // start all goroutines at once to maximize overlap
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
					inst.SetHistoryInfo(fmt.Sprintf("uuid-%d-%d", w, i), fmt.Sprintf("/path/%d/%d.jsonl", w, i))
				}
			}
		}(w)
	}

	for r := 0; r < numReaders; r++ {
		readerWG.Add(1)
		go func() {
			defer readerWG.Done()
			ready.Done()
			ready.Wait()
			for i := 0; i < 5000; i++ {
				_ = buildContinuationPrompt(inst)
			}
		}()
	}

	readerWG.Wait() // readers finish their fixed workload first
	close(stop)     // then signal writers to stop looping
	writerWG.Wait()
}
