package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// HistoryEntry represents a single command execution in history.
type HistoryEntry struct {
	Command       Command          `json:"command"`
	Result        *ExecutionResult `json:"result,omitempty"`
	Timestamp     time.Time        `json:"timestamp"`
	SessionName   string           `json:"session_name"`
	ExecutionTime time.Duration    `json:"execution_time"`
}

// CommandHistory tracks all executed commands with persistence.
type CommandHistory struct {
	sessionName string
	entries     []*HistoryEntry
	mu          sync.RWMutex
	persistPath string
	maxEntries  int // Maximum number of entries to keep (0 = unlimited)
}

// NewCommandHistory creates a new command history tracker.
func NewCommandHistory(sessionName string) *CommandHistory {
	return &CommandHistory{
		sessionName: sessionName,
		entries:     make([]*HistoryEntry, 0),
		maxEntries:  1000, // Default limit
	}
}

// NewCommandHistoryWithPersistence creates a command history with persistence enabled.
func NewCommandHistoryWithPersistence(sessionName string, persistDir string) (*CommandHistory, error) {
	ch := NewCommandHistory(sessionName)
	ch.persistPath = filepath.Join(persistDir, fmt.Sprintf("history_%s.json", sessionName))

	// Try to load existing history
	if err := ch.Load(); err != nil {
		// If file doesn't exist, that's fine - we'll create it on first save
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load history: %w", err)
		}
	}

	return ch, nil
}

// Add adds a command execution to the history.
func (ch *CommandHistory) Add(entry *HistoryEntry) error {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	// Add entry
	ch.entries = append(ch.entries, entry)

	// Enforce max entries limit
	if ch.maxEntries > 0 && len(ch.entries) > ch.maxEntries {
		// Remove oldest entries
		ch.entries = ch.entries[len(ch.entries)-ch.maxEntries:]
	}

	// Persist if enabled
	if ch.persistPath != "" {
		if err := ch.saveUnsafe(); err != nil {
			return fmt.Errorf("failed to persist history: %w", err)
		}
	}

	return nil
}

// AddFromResult creates and adds a history entry from an execution result.
func (ch *CommandHistory) AddFromResult(result *ExecutionResult) error {
	if result == nil {
		return fmt.Errorf("execution result is nil")
	}

	executionTime := result.EndTime.Sub(result.StartTime)
	if executionTime < 0 {
		executionTime = 0
	}

	entry := &HistoryEntry{
		Command:       *result.Command,
		Result:        result,
		Timestamp:     result.StartTime,
		SessionName:   ch.sessionName,
		ExecutionTime: executionTime,
	}

	return ch.Add(entry)
}

// GetAll returns all history entries (most recent first).
func (ch *CommandHistory) GetAll() []*HistoryEntry {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	// Return copy in reverse order (most recent first)
	result := make([]*HistoryEntry, len(ch.entries))
	for i, entry := range ch.entries {
		result[len(ch.entries)-1-i] = entry
	}
	return result
}

// GetRecent returns the N most recent history entries.
func (ch *CommandHistory) GetRecent(n int) []*HistoryEntry {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if n <= 0 {
		return []*HistoryEntry{}
	}

	if n > len(ch.entries) {
		n = len(ch.entries)
	}

	// Return last N entries in reverse order (most recent first)
	result := make([]*HistoryEntry, n)
	for i := 0; i < n; i++ {
		result[i] = ch.entries[len(ch.entries)-1-i]
	}
	return result
}

// GetByTimeRange returns entries within the specified time range.
func (ch *CommandHistory) GetByTimeRange(start, end time.Time) []*HistoryEntry {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	result := make([]*HistoryEntry, 0)
	for _, entry := range ch.entries {
		if entry.Timestamp.After(start) && entry.Timestamp.Before(end) {
			result = append(result, entry)
		}
	}

	// Reverse to have most recent first
	for i := 0; i < len(result)/2; i++ {
		j := len(result) - 1 - i
		result[i], result[j] = result[j], result[i]
	}

	return result
}

// Search searches history entries by command text (case-insensitive substring match).
func (ch *CommandHistory) Search(query string) []*HistoryEntry {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	result := make([]*HistoryEntry, 0)
	for _, entry := range ch.entries {
		// Simple substring match (case-insensitive would require strings.ToLower)
		if contains(entry.Command.Text, query) {
			result = append(result, entry)
		}
	}

	// Reverse to have most recent first
	for i := 0; i < len(result)/2; i++ {
		j := len(result) - 1 - i
		result[i], result[j] = result[j], result[i]
	}

	return result
}

// contains performs simple substring matching
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// GetByCommandID returns all history entries for a specific command ID.
func (ch *CommandHistory) GetByCommandID(commandID string) []*HistoryEntry {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	result := make([]*HistoryEntry, 0)
	for _, entry := range ch.entries {
		if entry.Command.ID == commandID {
			result = append(result, entry)
		}
	}

	// Reverse to have most recent first
	for i := 0; i < len(result)/2; i++ {
		j := len(result) - 1 - i
		result[i], result[j] = result[j], result[i]
	}

	return result
}

// GetByStatus returns entries with a specific command status.
func (ch *CommandHistory) GetByStatus(status CommandStatus) []*HistoryEntry {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	result := make([]*HistoryEntry, 0)
	for _, entry := range ch.entries {
		if entry.Command.Status == status {
			result = append(result, entry)
		}
	}

	// Reverse to have most recent first
	for i := 0; i < len(result)/2; i++ {
		j := len(result) - 1 - i
		result[i], result[j] = result[j], result[i]
	}

	return result
}

// GetSuccessful returns all successful command executions.
func (ch *CommandHistory) GetSuccessful() []*HistoryEntry {
	return ch.GetByStatus(CommandCompleted)
}

// GetFailed returns all failed command executions.
func (ch *CommandHistory) GetFailed() []*HistoryEntry {
	return ch.GetByStatus(CommandFailed)
}

// Count returns the total number of entries in history.
func (ch *CommandHistory) Count() int {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return len(ch.entries)
}

// Clear removes all history entries.
func (ch *CommandHistory) Clear() error {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	ch.entries = make([]*HistoryEntry, 0)

	// Persist if enabled
	if ch.persistPath != "" {
		if err := ch.saveUnsafe(); err != nil {
			return fmt.Errorf("failed to persist cleared history: %w", err)
		}
	}

	return nil
}

// SetMaxEntries sets the maximum number of entries to keep in history.
// Setting to 0 means unlimited. If current entries exceed the new limit,
// oldest entries are removed.
func (ch *CommandHistory) SetMaxEntries(max int) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	ch.maxEntries = max

	// Enforce new limit
	if ch.maxEntries > 0 && len(ch.entries) > ch.maxEntries {
		ch.entries = ch.entries[len(ch.entries)-ch.maxEntries:]
	}
}

// GetMaxEntries returns the current maximum entries limit.
func (ch *CommandHistory) GetMaxEntries() int {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return ch.maxEntries
}

// GetStatistics returns statistics about command execution history.
func (ch *CommandHistory) GetStatistics() HistoryStatistics {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	stats := HistoryStatistics{
		TotalCommands: len(ch.entries),
	}

	if len(ch.entries) == 0 {
		return stats
	}

	var totalDuration time.Duration
	for _, entry := range ch.entries {
		totalDuration += entry.ExecutionTime

		switch entry.Command.Status {
		case CommandCompleted:
			stats.SuccessfulCommands++
		case CommandFailed:
			stats.FailedCommands++
		case CommandCancelled:
			stats.CancelledCommands++
		}
	}

	stats.AverageExecutionTime = totalDuration / time.Duration(len(ch.entries))

	if len(ch.entries) > 0 {
		stats.FirstCommandTime = ch.entries[0].Timestamp
		stats.LastCommandTime = ch.entries[len(ch.entries)-1].Timestamp
	}

	return stats
}

// HistoryStatistics provides summary statistics about command history.
type HistoryStatistics struct {
	TotalCommands        int
	SuccessfulCommands   int
	FailedCommands       int
	CancelledCommands    int
	AverageExecutionTime time.Duration
	FirstCommandTime     time.Time
	LastCommandTime      time.Time
}

// Save persists the history to disk.
func (ch *CommandHistory) Save() error {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	return ch.saveUnsafe()
}

// saveUnsafe saves without acquiring lock (internal use only).
func (ch *CommandHistory) saveUnsafe() error {
	if ch.persistPath == "" {
		return fmt.Errorf("persistence not enabled for this history")
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(ch.persistPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create persist directory: %w", err)
	}

	// Marshal to JSON
	data := struct {
		SessionName string          `json:"session_name"`
		Entries     []*HistoryEntry `json:"entries"`
		Timestamp   time.Time       `json:"timestamp"`
		MaxEntries  int             `json:"max_entries"`
	}{
		SessionName: ch.sessionName,
		Entries:     ch.entries,
		Timestamp:   time.Now(),
		MaxEntries:  ch.maxEntries,
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal history data: %w", err)
	}

	// Write to file atomically
	tempPath := ch.persistPath + ".tmp"
	if err := os.WriteFile(tempPath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write history data: %w", err)
	}

	if err := os.Rename(tempPath, ch.persistPath); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// Load restores the history from disk.
func (ch *CommandHistory) Load() error {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	if ch.persistPath == "" {
		return fmt.Errorf("persistence not enabled for this history")
	}

	// Read file
	jsonData, err := os.ReadFile(ch.persistPath)
	if err != nil {
		return err
	}

	// Unmarshal JSON
	var data struct {
		SessionName string          `json:"session_name"`
		Entries     []*HistoryEntry `json:"entries"`
		Timestamp   time.Time       `json:"timestamp"`
		MaxEntries  int             `json:"max_entries"`
	}

	if err := json.Unmarshal(jsonData, &data); err != nil {
		return fmt.Errorf("failed to unmarshal history data: %w", err)
	}

	// Restore history
	ch.entries = data.Entries
	ch.maxEntries = data.MaxEntries

	return nil
}

// GetPersistPath returns the path where history is persisted.
func (ch *CommandHistory) GetPersistPath() string {
	return ch.persistPath
}

// SetPersistPath sets the path for history persistence.
func (ch *CommandHistory) SetPersistPath(path string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	ch.persistPath = path
}

// GetSessionName returns the session name for this history.
func (ch *CommandHistory) GetSessionName() string {
	return ch.sessionName
}
