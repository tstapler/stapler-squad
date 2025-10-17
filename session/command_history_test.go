package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewCommandHistory(t *testing.T) {
	history := NewCommandHistory("test-session")

	if history == nil {
		t.Fatal("NewCommandHistory() returned nil")
	}

	if history.sessionName != "test-session" {
		t.Errorf("Session name = %q, expected %q", history.sessionName, "test-session")
	}

	if history.Count() != 0 {
		t.Errorf("Initial count = %d, expected 0", history.Count())
	}
}

func TestNewCommandHistoryWithPersistence(t *testing.T) {
	tmpDir := t.TempDir()

	history, err := NewCommandHistoryWithPersistence("test-session", tmpDir)
	if err != nil {
		t.Fatalf("NewCommandHistoryWithPersistence() failed: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, "history_test-session.json")
	if history.GetPersistPath() != expectedPath {
		t.Errorf("Persist path = %q, expected %q", history.GetPersistPath(), expectedPath)
	}
}

func TestCommandHistory_Add(t *testing.T) {
	history := NewCommandHistory("test-session")

	entry := &HistoryEntry{
		Command: Command{
			ID:   "cmd-1",
			Text: "test command",
		},
		Timestamp:     time.Now(),
		SessionName:   "test-session",
		ExecutionTime: 1 * time.Second,
	}

	if err := history.Add(entry); err != nil {
		t.Fatalf("Add() failed: %v", err)
	}

	if history.Count() != 1 {
		t.Errorf("Count = %d, expected 1", history.Count())
	}
}

func TestCommandHistory_AddFromResult(t *testing.T) {
	history := NewCommandHistory("test-session")

	cmd := &Command{
		ID:     "cmd-1",
		Text:   "test command",
		Status: CommandCompleted,
	}

	result := &ExecutionResult{
		Command:   cmd,
		Success:   true,
		StartTime: time.Now(),
		EndTime:   time.Now().Add(2 * time.Second),
	}

	if err := history.AddFromResult(result); err != nil {
		t.Fatalf("AddFromResult() failed: %v", err)
	}

	if history.Count() != 1 {
		t.Errorf("Count = %d, expected 1", history.Count())
	}

	entries := history.GetAll()
	if len(entries) != 1 {
		t.Fatalf("GetAll() length = %d, expected 1", len(entries))
	}

	// Allow small timing variations due to system precision
	if entries[0].ExecutionTime < 2*time.Second || entries[0].ExecutionTime > 2*time.Second+100*time.Millisecond {
		t.Errorf("Execution time = %v, expected ~2s", entries[0].ExecutionTime)
	}
}

func TestCommandHistory_AddFromResultNil(t *testing.T) {
	history := NewCommandHistory("test-session")

	err := history.AddFromResult(nil)
	if err == nil {
		t.Error("AddFromResult(nil) should fail")
	}
}

func TestCommandHistory_GetAll(t *testing.T) {
	history := NewCommandHistory("test-session")

	// Add multiple entries
	for i := 1; i <= 3; i++ {
		entry := &HistoryEntry{
			Command: Command{
				ID:   string(rune('a' + i - 1)),
				Text: "command",
			},
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
		}
		history.Add(entry)
	}

	all := history.GetAll()
	if len(all) != 3 {
		t.Errorf("GetAll() length = %d, expected 3", len(all))
	}

	// Should be in reverse order (most recent first)
	if all[0].Command.ID != "c" {
		t.Errorf("First entry ID = %q, expected %q", all[0].Command.ID, "c")
	}
}

func TestCommandHistory_GetRecent(t *testing.T) {
	history := NewCommandHistory("test-session")

	// Add 5 entries
	for i := 1; i <= 5; i++ {
		entry := &HistoryEntry{
			Command: Command{ID: string(rune('a' + i - 1))},
		}
		history.Add(entry)
	}

	// Get 3 most recent
	recent := history.GetRecent(3)
	if len(recent) != 3 {
		t.Errorf("GetRecent(3) length = %d, expected 3", len(recent))
	}

	// Should be most recent first
	if recent[0].Command.ID != "e" {
		t.Errorf("First recent ID = %q, expected %q", recent[0].Command.ID, "e")
	}
}

func TestCommandHistory_GetRecentZero(t *testing.T) {
	history := NewCommandHistory("test-session")

	recent := history.GetRecent(0)
	if len(recent) != 0 {
		t.Errorf("GetRecent(0) length = %d, expected 0", len(recent))
	}
}

func TestCommandHistory_GetRecentMoreThanExists(t *testing.T) {
	history := NewCommandHistory("test-session")

	history.Add(&HistoryEntry{Command: Command{ID: "a"}})

	recent := history.GetRecent(10)
	if len(recent) != 1 {
		t.Errorf("GetRecent(10) with 1 entry length = %d, expected 1", len(recent))
	}
}

func TestCommandHistory_GetByTimeRange(t *testing.T) {
	history := NewCommandHistory("test-session")

	now := time.Now()

	// Add entries with different timestamps
	for i := 0; i < 5; i++ {
		entry := &HistoryEntry{
			Command:   Command{ID: string(rune('a' + i))},
			Timestamp: now.Add(time.Duration(i) * time.Hour),
		}
		history.Add(entry)
	}

	// Get entries between 0.5 hours and 3.5 hours (should get entries at 1, 2, and 3 hours)
	start := now.Add(30 * time.Minute)
	end := now.Add(3*time.Hour + 30*time.Minute)

	entries := history.GetByTimeRange(start, end)

	if len(entries) != 3 {
		t.Errorf("GetByTimeRange() length = %d, expected 3", len(entries))
	}
}

func TestCommandHistory_Search(t *testing.T) {
	history := NewCommandHistory("test-session")

	entries := []*HistoryEntry{
		{Command: Command{ID: "1", Text: "echo hello"}},
		{Command: Command{ID: "2", Text: "ls -la"}},
		{Command: Command{ID: "3", Text: "echo world"}},
	}

	for _, e := range entries {
		history.Add(e)
	}

	results := history.Search("echo")
	if len(results) != 2 {
		t.Errorf("Search('echo') length = %d, expected 2", len(results))
	}
}

func TestCommandHistory_GetByCommandID(t *testing.T) {
	history := NewCommandHistory("test-session")

	// Add multiple executions of same command ID
	for i := 0; i < 3; i++ {
		entry := &HistoryEntry{
			Command:   Command{ID: "cmd-1", Text: "test"},
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
		}
		history.Add(entry)
	}

	// Add different command
	history.Add(&HistoryEntry{
		Command: Command{ID: "cmd-2", Text: "other"},
	})

	results := history.GetByCommandID("cmd-1")
	if len(results) != 3 {
		t.Errorf("GetByCommandID() length = %d, expected 3", len(results))
	}
}

func TestCommandHistory_GetByStatus(t *testing.T) {
	history := NewCommandHistory("test-session")

	statuses := []CommandStatus{CommandCompleted, CommandFailed, CommandCompleted}

	for i, status := range statuses {
		entry := &HistoryEntry{
			Command: Command{
				ID:     string(rune('a' + i)),
				Status: status,
			},
		}
		history.Add(entry)
	}

	completed := history.GetByStatus(CommandCompleted)
	if len(completed) != 2 {
		t.Errorf("GetByStatus(Completed) length = %d, expected 2", len(completed))
	}

	failed := history.GetByStatus(CommandFailed)
	if len(failed) != 1 {
		t.Errorf("GetByStatus(Failed) length = %d, expected 1", len(failed))
	}
}

func TestCommandHistory_GetSuccessful(t *testing.T) {
	history := NewCommandHistory("test-session")

	history.Add(&HistoryEntry{Command: Command{ID: "1", Status: CommandCompleted}})
	history.Add(&HistoryEntry{Command: Command{ID: "2", Status: CommandFailed}})
	history.Add(&HistoryEntry{Command: Command{ID: "3", Status: CommandCompleted}})

	successful := history.GetSuccessful()
	if len(successful) != 2 {
		t.Errorf("GetSuccessful() length = %d, expected 2", len(successful))
	}
}

func TestCommandHistory_GetFailed(t *testing.T) {
	history := NewCommandHistory("test-session")

	history.Add(&HistoryEntry{Command: Command{ID: "1", Status: CommandCompleted}})
	history.Add(&HistoryEntry{Command: Command{ID: "2", Status: CommandFailed}})

	failed := history.GetFailed()
	if len(failed) != 1 {
		t.Errorf("GetFailed() length = %d, expected 1", len(failed))
	}
}

func TestCommandHistory_Clear(t *testing.T) {
	history := NewCommandHistory("test-session")

	// Add entries
	for i := 0; i < 3; i++ {
		history.Add(&HistoryEntry{Command: Command{ID: string(rune('a' + i))}})
	}

	if err := history.Clear(); err != nil {
		t.Fatalf("Clear() failed: %v", err)
	}

	if history.Count() != 0 {
		t.Errorf("Count after clear = %d, expected 0", history.Count())
	}
}

func TestCommandHistory_SetMaxEntries(t *testing.T) {
	history := NewCommandHistory("test-session")

	// Add 10 entries
	for i := 0; i < 10; i++ {
		history.Add(&HistoryEntry{Command: Command{ID: string(rune('a' + i))}})
	}

	// Set max to 5
	history.SetMaxEntries(5)

	if history.Count() != 5 {
		t.Errorf("Count after SetMaxEntries(5) = %d, expected 5", history.Count())
	}

	// Verify oldest were removed (should have f-j, not a-e)
	all := history.GetAll()
	if all[len(all)-1].Command.ID != "f" {
		t.Errorf("Oldest entry ID = %q, expected %q", all[len(all)-1].Command.ID, "f")
	}
}

func TestCommandHistory_SetMaxEntriesUnlimited(t *testing.T) {
	history := NewCommandHistory("test-session")

	history.SetMaxEntries(0) // Unlimited

	if history.GetMaxEntries() != 0 {
		t.Errorf("GetMaxEntries() = %d, expected 0", history.GetMaxEntries())
	}
}

func TestCommandHistory_MaxEntriesEnforcement(t *testing.T) {
	history := NewCommandHistory("test-session")
	history.SetMaxEntries(3)

	// Add 5 entries
	for i := 0; i < 5; i++ {
		history.Add(&HistoryEntry{Command: Command{ID: string(rune('a' + i))}})
	}

	// Should only have 3 most recent
	if history.Count() != 3 {
		t.Errorf("Count = %d, expected 3", history.Count())
	}
}

func TestCommandHistory_GetStatistics(t *testing.T) {
	history := NewCommandHistory("test-session")

	now := time.Now()

	entries := []*HistoryEntry{
		{
			Command:       Command{Status: CommandCompleted},
			ExecutionTime: 1 * time.Second,
			Timestamp:     now,
		},
		{
			Command:       Command{Status: CommandFailed},
			ExecutionTime: 2 * time.Second,
			Timestamp:     now.Add(1 * time.Second),
		},
		{
			Command:       Command{Status: CommandCompleted},
			ExecutionTime: 3 * time.Second,
			Timestamp:     now.Add(2 * time.Second),
		},
	}

	for _, e := range entries {
		history.Add(e)
	}

	stats := history.GetStatistics()

	if stats.TotalCommands != 3 {
		t.Errorf("TotalCommands = %d, expected 3", stats.TotalCommands)
	}

	if stats.SuccessfulCommands != 2 {
		t.Errorf("SuccessfulCommands = %d, expected 2", stats.SuccessfulCommands)
	}

	if stats.FailedCommands != 1 {
		t.Errorf("FailedCommands = %d, expected 1", stats.FailedCommands)
	}

	if stats.AverageExecutionTime != 2*time.Second {
		t.Errorf("AverageExecutionTime = %v, expected 2s", stats.AverageExecutionTime)
	}
}

func TestCommandHistory_GetStatisticsEmpty(t *testing.T) {
	history := NewCommandHistory("test-session")

	stats := history.GetStatistics()

	if stats.TotalCommands != 0 {
		t.Errorf("TotalCommands = %d, expected 0", stats.TotalCommands)
	}
}

func TestCommandHistory_Persistence(t *testing.T) {
	tmpDir := t.TempDir()

	// Create history with persistence
	history1, err := NewCommandHistoryWithPersistence("test-session", tmpDir)
	if err != nil {
		t.Fatalf("NewCommandHistoryWithPersistence() failed: %v", err)
	}

	// Add entries
	history1.Add(&HistoryEntry{Command: Command{ID: "1", Text: "first"}})
	history1.Add(&HistoryEntry{Command: Command{ID: "2", Text: "second"}})

	// Save
	if err := history1.Save(); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Create new history and load
	history2, err := NewCommandHistoryWithPersistence("test-session", tmpDir)
	if err != nil {
		t.Fatalf("NewCommandHistoryWithPersistence() failed: %v", err)
	}

	if history2.Count() != 2 {
		t.Errorf("Loaded history count = %d, expected 2", history2.Count())
	}

	all := history2.GetAll()
	if all[0].Command.ID != "2" {
		t.Errorf("First entry ID = %q, expected %q", all[0].Command.ID, "2")
	}
}

func TestCommandHistory_PersistenceAutoSave(t *testing.T) {
	tmpDir := t.TempDir()

	history, err := NewCommandHistoryWithPersistence("test-session", tmpDir)
	if err != nil {
		t.Fatalf("NewCommandHistoryWithPersistence() failed: %v", err)
	}

	history.Add(&HistoryEntry{Command: Command{ID: "1"}})

	// Should auto-save on add
	persistPath := history.GetPersistPath()
	if _, err := os.Stat(persistPath); os.IsNotExist(err) {
		t.Error("Persist file should be created after add")
	}
}

func TestCommandHistory_LoadNonExistent(t *testing.T) {
	history := NewCommandHistory("test-session")
	history.SetPersistPath("/nonexistent/path/history.json")

	err := history.Load()
	if err == nil {
		t.Error("Load() should fail with non-existent file")
	}
}

func TestCommandHistory_GetSessionName(t *testing.T) {
	history := NewCommandHistory("test-session")

	if history.GetSessionName() != "test-session" {
		t.Errorf("GetSessionName() = %q, expected %q", history.GetSessionName(), "test-session")
	}
}

func Benchmark_CommandHistory_Add(b *testing.B) {
	history := NewCommandHistory("test-session")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry := &HistoryEntry{
			Command:   Command{ID: "benchmark", Text: "test"},
			Timestamp: time.Now(),
		}
		history.Add(entry)
	}
}

func Benchmark_CommandHistory_Search(b *testing.B) {
	history := NewCommandHistory("test-session")

	// Pre-populate with 1000 entries
	for i := 0; i < 1000; i++ {
		entry := &HistoryEntry{
			Command: Command{
				ID:   string(rune('a' + (i % 26))),
				Text: "command text with various keywords",
			},
		}
		history.Add(entry)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		history.Search("keywords")
	}
}

func Benchmark_CommandHistory_GetStatistics(b *testing.B) {
	history := NewCommandHistory("test-session")

	// Pre-populate
	for i := 0; i < 1000; i++ {
		entry := &HistoryEntry{
			Command:       Command{ID: "cmd", Status: CommandCompleted},
			ExecutionTime: time.Duration(i) * time.Millisecond,
		}
		history.Add(entry)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		history.GetStatistics()
	}
}
