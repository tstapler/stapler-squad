package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ClaudeHistoryEntry represents a single entry from Claude's history.jsonl file
type ClaudeHistoryEntry struct {
	// ID is the unique identifier for this conversation
	ID string `json:"id"`
	// Name is the conversation title
	Name string `json:"name"`
	// Project is the project/directory path
	Project string `json:"project"`
	// CreatedAt is when the conversation started
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is when the conversation was last updated
	UpdatedAt time.Time `json:"updated_at"`
	// Model is the Claude model used (e.g., "claude-sonnet-4")
	Model string `json:"model"`
	// MessageCount is the number of messages in the conversation
	MessageCount int `json:"message_count"`
}

// ClaudeSessionHistory manages access to Claude session history
type ClaudeSessionHistory struct {
	// historyPath is the path to history.jsonl
	historyPath string
	// entries caches all parsed history entries
	entries []ClaudeHistoryEntry
	// projectIndex maps project paths to their entries for fast lookup
	projectIndex map[string][]int
	// mu provides thread-safe access
	mu sync.RWMutex
	// lastLoad tracks when the history was last loaded from disk
	lastLoad time.Time
}

// NewClaudeSessionHistory creates a new ClaudeSessionHistory instance
func NewClaudeSessionHistory(historyPath string) (*ClaudeSessionHistory, error) {
	sh := &ClaudeSessionHistory{
		historyPath:  historyPath,
		entries:      make([]ClaudeHistoryEntry, 0),
		projectIndex: make(map[string][]int),
	}

	// Load initial data
	if err := sh.Reload(); err != nil {
		// If file doesn't exist, that's okay - we'll start with empty history
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load history: %w", err)
		}
	}

	return sh, nil
}

// NewClaudeSessionHistoryFromClaudeDir creates a ClaudeSessionHistory from ~/.claude directory
func NewClaudeSessionHistoryFromClaudeDir() (*ClaudeSessionHistory, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	historyPath := filepath.Join(home, ".claude", "history.jsonl")
	return NewClaudeSessionHistory(historyPath)
}

// Reload reloads the history from disk, parsing all entries
func (sh *ClaudeSessionHistory) Reload() error {
	sh.mu.Lock()
	defer sh.mu.Unlock()

	// Open history file
	file, err := os.Open(sh.historyPath)
	if err != nil {
		return fmt.Errorf("failed to open history file: %w", err)
	}
	defer file.Close()

	// Clear existing data
	sh.entries = make([]ClaudeHistoryEntry, 0)
	sh.projectIndex = make(map[string][]int)

	// Parse JSONL format (one JSON object per line)
	scanner := bufio.NewScanner(file)
	// Increase buffer size for large lines
	const maxCapacity = 1024 * 1024 // 1MB per line
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()

		// Skip empty lines
		if len(line) == 0 {
			continue
		}

		var entry ClaudeHistoryEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			// Log but don't fail - skip corrupted entries
			continue
		}

		// Add to entries list
		idx := len(sh.entries)
		sh.entries = append(sh.entries, entry)

		// Index by project
		if entry.Project != "" {
			sh.projectIndex[entry.Project] = append(sh.projectIndex[entry.Project], idx)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading history file: %w", err)
	}

	// Sort entries by UpdatedAt descending (most recent first)
	sort.Slice(sh.entries, func(i, j int) bool {
		return sh.entries[i].UpdatedAt.After(sh.entries[j].UpdatedAt)
	})

	sh.lastLoad = time.Now()
	return nil
}

// GetAll returns all history entries, sorted by UpdatedAt descending
func (sh *ClaudeSessionHistory) GetAll() []ClaudeHistoryEntry {
	sh.mu.RLock()
	defer sh.mu.RUnlock()

	// Return a copy to prevent external modification
	entries := make([]ClaudeHistoryEntry, len(sh.entries))
	copy(entries, sh.entries)
	return entries
}

// GetByProject returns all history entries for a specific project path
func (sh *ClaudeSessionHistory) GetByProject(projectPath string) []ClaudeHistoryEntry {
	sh.mu.RLock()
	defer sh.mu.RUnlock()

	// Normalize path for comparison
	normalizedPath := filepath.Clean(projectPath)

	indices, exists := sh.projectIndex[normalizedPath]
	if !exists {
		return []ClaudeHistoryEntry{}
	}

	entries := make([]ClaudeHistoryEntry, 0, len(indices))
	for _, idx := range indices {
		if idx < len(sh.entries) {
			entries = append(entries, sh.entries[idx])
		}
	}

	return entries
}

// GetByID returns a specific history entry by ID
func (sh *ClaudeSessionHistory) GetByID(id string) (*ClaudeHistoryEntry, error) {
	sh.mu.RLock()
	defer sh.mu.RUnlock()

	for _, entry := range sh.entries {
		if entry.ID == id {
			// Return a copy
			entryCopy := entry
			return &entryCopy, nil
		}
	}

	return nil, fmt.Errorf("history entry not found: %s", id)
}

// Search searches history entries by name or project path
func (sh *ClaudeSessionHistory) Search(query string) []ClaudeHistoryEntry {
	sh.mu.RLock()
	defer sh.mu.RUnlock()

	query = strings.ToLower(query)
	results := make([]ClaudeHistoryEntry, 0)

	for _, entry := range sh.entries {
		// Search in name and project
		if strings.Contains(strings.ToLower(entry.Name), query) ||
			strings.Contains(strings.ToLower(entry.Project), query) {
			results = append(results, entry)
		}
	}

	return results
}

// GetProjects returns a list of unique project paths from history
func (sh *ClaudeSessionHistory) GetProjects() []string {
	sh.mu.RLock()
	defer sh.mu.RUnlock()

	projects := make([]string, 0, len(sh.projectIndex))
	for project := range sh.projectIndex {
		projects = append(projects, project)
	}

	sort.Strings(projects)
	return projects
}

// Count returns the total number of history entries
func (sh *ClaudeSessionHistory) Count() int {
	sh.mu.RLock()
	defer sh.mu.RUnlock()

	return len(sh.entries)
}

// LastLoadTime returns when the history was last loaded from disk
func (sh *ClaudeSessionHistory) LastLoadTime() time.Time {
	sh.mu.RLock()
	defer sh.mu.RUnlock()

	return sh.lastLoad
}
