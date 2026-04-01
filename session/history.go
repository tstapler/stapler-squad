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

// conversationMessage represents a single message in a Claude conversation file
type conversationMessage struct {
	Type      string `json:"type"`
	UUID      string `json:"uuid"`
	SessionID string `json:"sessionId"`
	Timestamp string `json:"timestamp"`
	CWD       string `json:"cwd"`
	Message   struct {
		Role    string      `json:"role"`
		Model   string      `json:"model,omitempty"`
		Content interface{} `json:"content"`
	} `json:"message"`
}

// Reload reloads the history by scanning all conversation files in ~/.claude/projects/
func (sh *ClaudeSessionHistory) Reload() error {
	sh.mu.Lock()
	defer sh.mu.Unlock()

	// Clear existing data
	sh.entries = make([]ClaudeHistoryEntry, 0)
	sh.projectIndex = make(map[string][]int)

	// Get projects directory
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	projectsDir := filepath.Join(home, ".claude", "projects")

	// Check if projects directory exists
	if _, err := os.Stat(projectsDir); os.IsNotExist(err) {
		// No projects yet - return empty history
		sh.lastLoad = time.Now()
		return nil
	}

	// Walk through all project directories
	err = filepath.Walk(projectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Only process .jsonl files (but not agent files)
		if !info.IsDir() && filepath.Ext(path) == ".jsonl" && !strings.Contains(filepath.Base(path), "agent-") {
			entry, err := sh.parseConversationFile(path)
			if err != nil {
				// Skip invalid files
				return nil
			}

			// Add to entries list
			idx := len(sh.entries)
			sh.entries = append(sh.entries, *entry)

			// Index by project
			if entry.Project != "" {
				sh.projectIndex[entry.Project] = append(sh.projectIndex[entry.Project], idx)
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("error scanning projects directory: %w", err)
	}

	// Sort entries by UpdatedAt descending (most recent first)
	sort.Slice(sh.entries, func(i, j int) bool {
		return sh.entries[i].UpdatedAt.After(sh.entries[j].UpdatedAt)
	})

	sh.lastLoad = time.Now()
	return nil
}

// parseConversationFile extracts metadata from a Claude conversation file
// Optimized to parse only what's needed for listing (fast preview mode)
func (sh *ClaudeSessionHistory) parseConversationFile(filePath string) (*ClaudeHistoryEntry, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Check file size - skip files larger than 50MB (likely corrupted or test data)
	fileInfo, err := file.Stat()
	if err == nil && fileInfo.Size() > 50*1024*1024 {
		return nil, fmt.Errorf("file too large: %d bytes", fileInfo.Size())
	}

	var entry ClaudeHistoryEntry
	var messageCount int
	var model string
	var firstTimestamp, lastTimestamp time.Time

	scanner := bufio.NewScanner(file)
	const maxCapacity = 1024 * 1024 // 1MB per line
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	lineNum := 0
	const maxLinesToScan = 1000 // Optimization: Only scan first 1000 lines for preview

	for scanner.Scan() && lineNum < maxLinesToScan {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var msg conversationMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}

		// Count messages (skip file-history-snapshot entries)
		if msg.Type == "user" || msg.Type == "assistant" {
			messageCount++

			// Extract model from first assistant message
			if model == "" && msg.Message.Model != "" {
				model = msg.Message.Model
			}

			// Parse timestamp
			if ts, err := time.Parse(time.RFC3339, msg.Timestamp); err == nil {
				if firstTimestamp.IsZero() || ts.Before(firstTimestamp) {
					firstTimestamp = ts
				}
				if lastTimestamp.IsZero() || ts.After(lastTimestamp) {
					lastTimestamp = ts
				}
			}

			// Extract session ID and project path
			if entry.ID == "" && msg.SessionID != "" {
				entry.ID = msg.SessionID
			}
			if entry.Project == "" && msg.CWD != "" {
				entry.Project = msg.CWD
			}

		}
	}

	// If we hit the line limit, continue counting only (faster - no JSON parsing)
	if lineNum >= maxLinesToScan && scanner.Scan() {
		remainingCount := 0
		for scanner.Scan() {
			line := scanner.Bytes()
			// Quick heuristic: lines with "type":"user" or "type":"assistant"
			if strings.Contains(string(line), `"type":"user"`) ||
				strings.Contains(string(line), `"type":"assistant"`) {
				remainingCount++

				// Update lastTimestamp from last few messages
				if remainingCount > 0 && strings.Contains(string(line), `"timestamp"`) {
					var msg conversationMessage
					if err := json.Unmarshal(line, &msg); err == nil {
						if ts, err := time.Parse(time.RFC3339, msg.Timestamp); err == nil {
							if lastTimestamp.IsZero() || ts.After(lastTimestamp) {
								lastTimestamp = ts
							}
						}
					}
				}
			}
		}
		messageCount += remainingCount
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	// Skip entries with no messages or no session ID
	if messageCount == 0 || entry.ID == "" {
		return nil, fmt.Errorf("invalid conversation file: no messages or session ID")
	}

	// Generate a name from the project path
	if entry.Project != "" {
		entry.Name = filepath.Base(entry.Project)
	} else {
		entry.Name = "Unknown"
	}

	entry.MessageCount = messageCount
	entry.Model = model
	entry.CreatedAt = firstTimestamp
	entry.UpdatedAt = lastTimestamp

	return &entry, nil
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

// ClaudeConversationMessage represents a message in a conversation
type ClaudeConversationMessage struct {
	Role      string
	Content   string
	Timestamp time.Time
	Model     string
}

// GetMessagesFromConversationFile reads all messages from a conversation file
func (sh *ClaudeSessionHistory) GetMessagesFromConversationFile(sessionID string) ([]ClaudeConversationMessage, error) {
	// Find the conversation file for this session ID
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	projectsDir := filepath.Join(home, ".claude", "projects")
	var conversationFile string

	// Search for the conversation file with this session ID
	err = filepath.Walk(projectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// Check if this is a conversation file (not agent file)
		if !info.IsDir() && filepath.Ext(path) == ".jsonl" && !strings.Contains(filepath.Base(path), "agent-") {
			// Quick check if this file contains our session ID
			file, err := os.Open(path)
			if err != nil {
				return nil
			}
			defer file.Close()

			scanner := bufio.NewScanner(file)
			scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

			// Check first few lines for session ID
			for i := 0; i < 5 && scanner.Scan(); i++ {
				if strings.Contains(string(scanner.Bytes()), sessionID) {
					conversationFile = path
					return filepath.SkipDir // Found it, stop walking
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error searching for conversation file: %w", err)
	}

	if conversationFile == "" {
		return nil, fmt.Errorf("conversation file not found for session ID: %s", sessionID)
	}

	// Parse the conversation file
	file, err := os.Open(conversationFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open conversation file: %w", err)
	}
	defer file.Close()

	var messages []ClaudeConversationMessage
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var msg conversationMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}

		// Extract user and assistant messages
		if msg.Type == "user" || msg.Type == "assistant" {
			var content string

			// Extract content directly from the parsed struct
			switch v := msg.Message.Content.(type) {
			case string:
				// Simple string content (typically user messages)
				content = v
			case []interface{}:
				// Content is an array (tool use, text, etc. - typically assistant messages)
				for _, item := range v {
					if itemMap, ok := item.(map[string]interface{}); ok {
						if itemType, ok := itemMap["type"].(string); ok {
							if itemType == "text" {
								if text, ok := itemMap["text"].(string); ok {
									content += text + "\n"
								}
							}
						}
					}
				}
			default:
				// Fallback: stringify the entire message
				contentJSON, _ := json.Marshal(msg.Message)
				content = string(contentJSON)
			}

			ts, _ := time.Parse(time.RFC3339, msg.Timestamp)

			messages = append(messages, ClaudeConversationMessage{
				Role:      msg.Message.Role,
				Content:   content,
				Timestamp: ts,
				Model:     msg.Message.Model,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading conversation file: %w", err)
	}

	return messages, nil
}
