package session

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
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

// historyJSONLEntry represents a single line in ~/.claude/history.jsonl.
// Each line records one user message sent to Claude across all sessions.
type historyJSONLEntry struct {
	Display   string `json:"display"`   // first ~200 chars of the user message
	Timestamp int64  `json:"timestamp"` // Unix ms since epoch
	Project   string `json:"project"`   // working directory at time of message
	SessionID string `json:"sessionId"` // conversation UUID
}

// Reload loads history from ~/.claude/history.jsonl, which Claude maintains as a
// compact index of all conversations. Each line is one user message; we aggregate
// by sessionId to reconstruct per-session metadata (name, timestamps, message count).
func (sh *ClaudeSessionHistory) Reload() error {
	sh.mu.Lock()
	defer sh.mu.Unlock()

	sh.entries = make([]ClaudeHistoryEntry, 0)
	sh.projectIndex = make(map[string][]int)

	histFile, err := os.Open(sh.historyPath)
	if err != nil {
		if os.IsNotExist(err) {
			sh.lastLoad = time.Now()
			return nil
		}
		return fmt.Errorf("failed to open history file: %w", err)
	}
	defer histFile.Close()

	type sessionAgg struct {
		firstDisplay string
		project      string
		firstTs      int64
		lastTs       int64
		msgCount     int
	}

	sessionMap := make(map[string]*sessionAgg)
	// Track insertion order so we can sort stably later.
	var sessionOrder []string

	scanner := bufio.NewScanner(histFile)
	const maxLine = 1024 * 1024
	scanner.Buffer(make([]byte, 64*1024), maxLine)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry historyJSONLEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.SessionID == "" {
			continue
		}

		agg, exists := sessionMap[entry.SessionID]
		if !exists {
			agg = &sessionAgg{
				firstDisplay: entry.Display,
				project:      entry.Project,
				firstTs:      entry.Timestamp,
				lastTs:       entry.Timestamp,
			}
			sessionMap[entry.SessionID] = agg
			sessionOrder = append(sessionOrder, entry.SessionID)
		} else {
			if entry.Timestamp < agg.firstTs {
				agg.firstTs = entry.Timestamp
				agg.firstDisplay = entry.Display
				if entry.Project != "" {
					agg.project = entry.Project
				}
			}
			if entry.Timestamp > agg.lastTs {
				agg.lastTs = entry.Timestamp
			}
		}
		agg.msgCount++
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading history file: %w", err)
	}

	for _, sessionID := range sessionOrder {
		agg := sessionMap[sessionID]
		name := cleanDisplayName(agg.firstDisplay)
		if name == "" {
			name = filepath.Base(agg.project)
		}
		if name == "" || name == "." {
			name = "Unknown"
		}

		sh.entries = append(sh.entries, ClaudeHistoryEntry{
			ID:           sessionID,
			Name:         name,
			Project:      agg.project,
			CreatedAt:    time.UnixMilli(agg.firstTs),
			UpdatedAt:    time.UnixMilli(agg.lastTs),
			MessageCount: agg.msgCount,
		})
	}

	// Sort before indexing so projectIndex holds post-sort positions.
	sort.Slice(sh.entries, func(i, j int) bool {
		return sh.entries[i].UpdatedAt.After(sh.entries[j].UpdatedAt)
	})

	for idx, entry := range sh.entries {
		if entry.Project != "" {
			sh.projectIndex[entry.Project] = append(sh.projectIndex[entry.Project], idx)
		}
	}

	sh.lastLoad = time.Now()
	return nil
}

// cleanDisplayName truncates a raw user message for use as a session name.
func cleanDisplayName(display string) string {
	// Trim leading slash (skill/command invocations like "/quality:review")
	display = strings.TrimLeft(display, "/")
	display = strings.TrimSpace(display)
	if len(display) > 100 {
		display = display[:97] + "..."
	}
	return display
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

// conversationScanBufPool pools the 1MB scanner buffer used by
// findConversationFilePath so a filepath.WalkDir over thousands of conversation
// files doesn't allocate a fresh buffer per file.
var conversationScanBufPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 1024*1024)
		return &buf
	},
}

// findConversationFilePath searches projectsDir for the JSONL file that
// contains the given sessionID. It checks the first 5 lines of each file for a
// reference to the sessionID, then stops the walk as soon as it finds a match.
//
// projectsDir empty falls back to ~/.claude/projects. It is a parameter rather
// than a package var or env var so tests can pass a t.TempDir() root without
// forbidding t.Parallel().
//
// ctx is checked once per directory entry so a caller (e.g. the session driver
// loop) can abort an in-flight walk instead of blocking until it finishes on
// its own — under -count=20 stress, concurrent walks over a large
// ~/.claude/projects/ tree could otherwise hold a driver goroutine past
// StopSessionDriver's stop signal long enough to blow the test binary's
// global timeout.
func findConversationFilePath(ctx context.Context, projectsDir, sessionID string) (string, error) {
	if projectsDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get user home directory: %w", err)
		}
		projectsDir = filepath.Join(home, ".claude", "projects")
	}
	var conversationFile string

	err := filepath.WalkDir(projectsDir, func(path string, d fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() || filepath.Ext(path) != ".jsonl" || strings.Contains(filepath.Base(path), "agent-") {
			return nil
		}

		file, err := os.Open(path) // #nosec G304 -- path comes from filepath.WalkDir enumerating the projects dir, not external input
		if err != nil {
			return nil
		}
		defer file.Close() //nolint:errcheck

		bufPtr := conversationScanBufPool.Get().(*[]byte)
		defer conversationScanBufPool.Put(bufPtr) //nolint:staticcheck

		scanner := bufio.NewScanner(file)
		scanner.Buffer(*bufPtr, 1024*1024)

		for i := 0; i < 5 && scanner.Scan(); i++ {
			if strings.Contains(string(scanner.Bytes()), sessionID) {
				conversationFile = path
				return filepath.SkipAll // stop the entire walk
			}
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("error searching for conversation file: %w", err)
	}
	if conversationFile == "" {
		return "", fmt.Errorf("conversation file not found for session ID: %s", sessionID)
	}
	return conversationFile, nil
}

// FindConversationFilePath is the exported wrapper for findConversationFilePath.
// It searches ~/.claude/projects/ for the JSONL file containing sessionID.
func FindConversationFilePath(ctx context.Context, sessionID string) (string, error) {
	return findConversationFilePath(ctx, "", sessionID)
}

// GetMessagesFromConversationFile reads messages from the conversation file for
// the given sessionID. When limit > 0 only the last limit messages are returned
// (using an efficient reverse-read that avoids loading the full file). When
// limit == 0 all messages are returned.
//
// Results are always in chronological order (oldest first).
func (sh *ClaudeSessionHistory) GetMessagesFromConversationFile(sessionID string, limit int) ([]ClaudeConversationMessage, error) {
	conversationFile, err := findConversationFilePath(context.Background(), "", sessionID)
	if err != nil {
		return nil, err
	}

	if limit > 0 {
		return readLastNMessagesFromFile(conversationFile, limit)
	}
	return readAllMessagesFromFile(conversationFile)
}
