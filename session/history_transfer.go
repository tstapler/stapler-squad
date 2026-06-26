package session

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/tstapler/stapler-squad/log"
)

// ConversationID represents a validated Claude/Antigravity conversation UUID.
type ConversationID string

// ParseConversationID parses and validates a raw string as a ConversationID.
func ParseConversationID(s string) (ConversationID, error) {
	if !isValidUUID(s) {
		return "", fmt.Errorf("invalid conversation ID format: %q", s)
	}
	return ConversationID(s), nil
}

// WorkspacePath represents a cleaned, resolved workspace root path.
type WorkspacePath string

// NewWorkspacePath resolves symlinks and cleans a path to guarantee a single canonical representation.
func NewWorkspacePath(s string) (WorkspacePath, error) {
	if s == "" {
		return "", fmt.Errorf("workspace path cannot be empty")
	}
	resolved, err := filepath.EvalSymlinks(s)
	if err != nil {
		resolved = s
	}
	return WorkspacePath(filepath.Clean(resolved)), nil
}

// SkippedTurn represents a meta-turn or skipped turn.
type SkippedTurn struct{}

func (SkippedTurn) unifiedTurn() {}

// UnifiedTurn represents a domain-level turn in the transcript.
type UnifiedTurn interface {
	unifiedTurn()
}

// UserMessage represents a turn initiated by the user.
type UserMessage struct {
	Content   string
	Timestamp string
}

func (UserMessage) unifiedTurn() {}

// AssistantMessage represents a turn responded by the assistant, including optional tool calls.
type AssistantMessage struct {
	Content   string
	ToolCalls []UnifiedToolCall
	Timestamp string
}

func (AssistantMessage) unifiedTurn() {}

// UnifiedToolCall represents a generic tool execution in the transcript.
type UnifiedToolCall struct {
	Name string
	Args map[string]interface{}
}

// ParseClaudeTurn parses a raw Claude JSONL line into a UnifiedTurn.
func ParseClaudeTurn(line []byte) (UnifiedTurn, error) {
	type RawClaudeTurn struct {
		Type      string `json:"type"`
		Timestamp string `json:"timestamp"`
		Message   *struct {
			Role    string      `json:"role"`
			Content interface{} `json:"content"`
		} `json:"message,omitempty"`
	}

	var raw RawClaudeTurn
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Claude turn: %w", err)
	}

	switch raw.Type {
	case "user":
		if raw.Message == nil {
			return nil, fmt.Errorf("user turn is missing message details")
		}
		contentStr := ""
		if s, ok := raw.Message.Content.(string); ok {
			contentStr = s
		} else if contentList, ok := raw.Message.Content.([]interface{}); ok {
			var parts []string
			for _, c := range contentList {
				if cMap, ok := c.(map[string]interface{}); ok {
					switch cMap["type"] {
					case "text":
						if txt, ok := cMap["text"].(string); ok {
							parts = append(parts, txt)
						}
					case "tool_result":
						if content, ok := cMap["content"].(string); ok {
							parts = append(parts, content)
						}
					}
				}
			}
			contentStr = strings.Join(parts, "\n")
		}
		return UserMessage{
			Content:   contentStr,
			Timestamp: raw.Timestamp,
		}, nil

	case "assistant":
		if raw.Message == nil {
			return nil, fmt.Errorf("assistant turn is missing message details")
		}
		var textParts []string
		var toolCalls []UnifiedToolCall

		if contentList, ok := raw.Message.Content.([]interface{}); ok {
			for _, c := range contentList {
				if cMap, ok := c.(map[string]interface{}); ok {
					cType := cMap["type"]
					switch cType {
					case "text":
						if txt, ok := cMap["text"].(string); ok {
							textParts = append(textParts, txt)
						}
					case "tool_use":
						args, _ := cMap["input"].(map[string]interface{})
						name, _ := cMap["name"].(string)
						toolCalls = append(toolCalls, UnifiedToolCall{
							Name: name,
							Args: args,
						})
					}
				}
			}
		}
		return AssistantMessage{
			Content:   strings.Join(textParts, "\n"),
			ToolCalls: toolCalls,
			Timestamp: raw.Timestamp,
		}, nil

	default:
		// Skip other meta-events (queue-operation, last-prompt, attachment etc)
		return SkippedTurn{}, nil
	}
}

// ParseAgyStep parses a raw Antigravity JSONL line into a UnifiedTurn.
func ParseAgyStep(line []byte) (UnifiedTurn, error) {
	type RawAgyStep struct {
		StepIndex int    `json:"step_index"`
		Source    string `json:"source"`
		Type      string `json:"type"`
		Status    string `json:"status"`
		CreatedAt string `json:"created_at"`
		Content   string `json:"content"`
		ToolCalls []struct {
			Name string                 `json:"name"`
			Args map[string]interface{} `json:"args"`
		} `json:"tool_calls"`
	}

	var raw RawAgyStep
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Antigravity step: %w", err)
	}

	switch raw.Type {
	case "USER_INPUT":
		return UserMessage{
			Content:   raw.Content,
			Timestamp: raw.CreatedAt,
		}, nil
	case "PLANNER_RESPONSE":
		var toolCalls []UnifiedToolCall
		for _, tc := range raw.ToolCalls {
			toolCalls = append(toolCalls, UnifiedToolCall{
				Name: tc.Name,
				Args: tc.Args,
			})
		}
		return AssistantMessage{
			Content:   raw.Content,
			ToolCalls: toolCalls,
			Timestamp: raw.CreatedAt,
		}, nil
	default:
		return SkippedTurn{}, nil // Ignore other step types
	}
}

// MarshalTurnToClaude converts a UnifiedTurn into Claude JSONL format.
func MarshalTurnToClaude(turn UnifiedTurn, sessionId string, cwd string, parentUuid string, turnUuid string) ([]byte, error) {
	switch t := turn.(type) {
	case UserMessage:
		m := map[string]interface{}{
			"parentUuid":  nilIfEmptyInterface(parentUuid),
			"isSidechain": false,
			"type":        "user",
			"message": map[string]interface{}{
				"role":    "user",
				"content": t.Content,
			},
			"uuid":      turnUuid,
			"timestamp": t.Timestamp,
			"sessionId": sessionId,
			"cwd":       cwd,
		}
		return json.Marshal(m)
	case AssistantMessage:
		var content []map[string]interface{}
		if t.Content != "" {
			content = append(content, map[string]interface{}{
				"type": "text",
				"text": t.Content,
			})
		}
		for _, tc := range t.ToolCalls {
			content = append(content, map[string]interface{}{
				"type":  "tool_use",
				"name":  tc.Name,
				"input": tc.Args,
			})
		}
		m := map[string]interface{}{
			"parentUuid":  nilIfEmptyInterface(parentUuid),
			"isSidechain": false,
			"type":        "assistant",
			"message": map[string]interface{}{
				"role":    "assistant",
				"content": content,
			},
			"uuid":      turnUuid,
			"timestamp": t.Timestamp,
			"sessionId": sessionId,
			"cwd":       cwd,
		}
		return json.Marshal(m)
	default:
		return nil, fmt.Errorf("unsupported turn type: %T", turn)
	}
}

type AgyStepOut struct {
	StepIndex int                      `json:"step_index"`
	Source    string                   `json:"source"`
	Type      string                   `json:"type"`
	Status    string                   `json:"status"`
	CreatedAt string                   `json:"created_at"`
	Content   string                   `json:"content,omitempty"`
	ToolCalls []map[string]interface{} `json:"tool_calls,omitempty"`
}

// MarshalTurnToAgy converts a UnifiedTurn into Antigravity JSONL step format.
func MarshalTurnToAgy(turn UnifiedTurn, stepIdx int) (AgyStepOut, error) {
	switch t := turn.(type) {
	case UserMessage:
		return AgyStepOut{
			StepIndex: stepIdx,
			Source:    "USER_EXPLICIT",
			Type:      "USER_INPUT",
			Status:    "DONE",
			CreatedAt: t.Timestamp,
			Content:   t.Content,
		}, nil
	case AssistantMessage:
		var toolCalls []map[string]interface{}
		for _, tc := range t.ToolCalls {
			toolCalls = append(toolCalls, map[string]interface{}{
				"name": tc.Name,
				"args": tc.Args,
			})
		}
		return AgyStepOut{
			StepIndex: stepIdx,
			Source:    "MODEL",
			Type:      "PLANNER_RESPONSE",
			Status:    "DONE",
			CreatedAt: t.Timestamp,
			Content:   t.Content,
			ToolCalls: toolCalls,
		}, nil
	default:
		return AgyStepOut{}, fmt.Errorf("unsupported turn type: %T", turn)
	}
}

// PortSessionHistory translates and syncs history between Claude Code and Antigravity CLI.
func PortSessionHistory(ctx context.Context, oldProgram, newProgram string, i *Instance) error {
	isOldClaude := strings.Contains(oldProgram, "claude")
	isNewClaude := strings.Contains(newProgram, "claude")
	isOldAgy := strings.Contains(oldProgram, "agy") || strings.Contains(oldProgram, "antigravity")
	isNewAgy := strings.Contains(newProgram, "agy") || strings.Contains(newProgram, "antigravity")

	if isOldClaude && isNewAgy {
		return portClaudeToAgy(i)
	} else if isOldAgy && isNewClaude {
		return portAgyToClaude(i)
	}

	return nil
}

func portClaudeToAgy(i *Instance) error {
	// 1. Get Claude Session ID
	uuidStr := i.GetClaudeConversationUUID()
	if uuidStr == "" {
		// Try to extract it
		i.tryExtractConversationUUID()
		uuidStr = i.GetClaudeConversationUUID()
	}
	if uuidStr == "" {
		log.Info("no claude conversation UUID found to port", "session", i.Title)
		return nil
	}

	// Parse boundary input to a verified domain type
	uuid, err := ParseConversationID(uuidStr)
	if err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	// 2. Find Claude JSONL file
	claudeProjectsDir := filepath.Join(home, ".claude", "projects")
	var claudeLogPath string
	_ = filepath.Walk(claudeProjectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && info.Name() == string(uuid)+".jsonl" {
			claudeLogPath = path
			return fmt.Errorf("found") // Use error to break early
		}
		return nil
	})

	if claudeLogPath == "" {
		log.Warn("could not find Claude transcript file to port", "uuid", uuid)
		return nil
	}

	// 3. Parse Claude JSONL into UnifiedTurns
	file, err := os.Open(claudeLogPath)
	if err != nil {
		return err
	}
	defer file.Close()

	var unifiedTurns []UnifiedTurn
	reader := bufio.NewReader(file)
	for {
		lineBytes, err := reader.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return fmt.Errorf("failed to read Claude JSONL log: %w", err)
		}

		lineStr := strings.TrimSpace(string(lineBytes))
		if lineStr != "" {
			turn, err := ParseClaudeTurn([]byte(lineStr))
			if err != nil {
				log.Warn("skipping invalid jsonl line in Claude transcript", "err", err)
			} else if turn != nil {
				if _, ok := turn.(SkippedTurn); !ok {
					unifiedTurns = append(unifiedTurns, turn)
				}
			}
		}

		if err == io.EOF {
			break
		}
	}

	// 4. Create Antigravity brain dir and write transcript files
	agyBrainDir := filepath.Join(home, ".gemini", "antigravity-cli", "brain", string(uuid), ".system_generated", "logs")
	if err := os.MkdirAll(agyBrainDir, 0700); err != nil {
		return err
	}

	transcriptPath := filepath.Join(agyBrainDir, "transcript.jsonl")
	transcriptFullPath := filepath.Join(agyBrainDir, "transcript_full.jsonl")

	for _, p := range []string{transcriptPath, transcriptFullPath} {
		f, err := os.Create(p)
		if err != nil {
			return err
		}
		for idx, turn := range unifiedTurns {
			agyStep, err := MarshalTurnToAgy(turn, idx)
			if err != nil {
				f.Close()
				return err
			}
			data, err := json.Marshal(agyStep)
			if err != nil {
				f.Close()
				return err
			}
			f.Write(data)
			f.Write([]byte("\n"))
		}
		f.Close()
	}

	// 5. Append history entry
	agyHistoryPath := filepath.Join(home, ".gemini", "antigravity-cli", "history.jsonl")
	historyEntry := map[string]interface{}{
		"display":        fmt.Sprintf("Ported from Claude: %s", i.Title),
		"timestamp":      time.Now().UnixMilli(),
		"workspace":      i.GetWorkingDirectory(),
		"conversationId": string(uuid),
	}
	historyData, err := json.Marshal(historyEntry)
	if err == nil {
		f, err := os.OpenFile(agyHistoryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			f.Write(historyData)
			f.Write([]byte("\n"))
			f.Close()
		}
	}

	// 6. Create SQLite DB
	dbPath := filepath.Join(home, ".gemini", "antigravity-cli", "conversations", string(uuid)+".db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		return err
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	// Create tables and indexes matching the real Antigravity DB schema exactly.
	// Verified against live .db files — all 7 tables and both indexes must exist
	// before Antigravity opens the database, otherwise it may fail migrations.
	queries := []string{
		`CREATE TABLE IF NOT EXISTS trajectory_meta (
			trajectory_id TEXT PRIMARY KEY,
			cascade_id TEXT,
			trajectory_type INTEGER,
			source INTEGER
		);`,
		`CREATE TABLE IF NOT EXISTS steps (
			idx INTEGER PRIMARY KEY,
			step_type INTEGER NOT NULL DEFAULT 0,
			status INTEGER NOT NULL DEFAULT 0,
			has_subtrajectory NUMERIC NOT NULL DEFAULT false,
			metadata BLOB,
			error_details BLOB,
			permissions BLOB,
			task_details BLOB,
			render_info BLOB,
			step_payload BLOB,
			step_format INTEGER NOT NULL DEFAULT 0
		);`,
		// Indexes — critical for step_type and status queries (e.g. "all USER_INPUT steps").
		// Without these every search is a full table scan.
		`CREATE INDEX IF NOT EXISTS idx_steps_status    ON steps(status);`,
		`CREATE INDEX IF NOT EXISTS idx_steps_step_type ON steps(step_type);`,
		// Supporting tables — Antigravity expects these to be present on open.
		`CREATE TABLE IF NOT EXISTS gen_metadata (
			idx  INTEGER PRIMARY KEY,
			data BLOB,
			size INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS executor_metadata (
			idx  INTEGER PRIMARY KEY,
			data BLOB
		);`,
		`CREATE TABLE IF NOT EXISTS parent_references (
			idx  INTEGER PRIMARY KEY,
			data BLOB
		);`,
		`CREATE TABLE IF NOT EXISTS trajectory_metadata_blob (
			id   TEXT DEFAULT "main",
			data BLOB,
			PRIMARY KEY (id)
		);`,
		`CREATE TABLE IF NOT EXISTS battle_mode_infos (
			idx  INTEGER PRIMARY KEY,
			data BLOB
		);`,
	}
	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}

	// Begin transaction
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Insert meta
	if _, err := tx.Exec(`INSERT OR REPLACE INTO trajectory_meta (trajectory_id, cascade_id, trajectory_type, source) VALUES (?, ?, 0, 0);`, string(uuid), string(uuid)); err != nil {
		return err
	}

	// Insert steps (simple placeholder steps so SQLite is structurally valid)
	for idx, turn := range unifiedTurns {
		stepType := 15 // PLANNER_RESPONSE
		if _, ok := turn.(UserMessage); ok {
			stepType = 14
		}
		if _, err := tx.Exec(`INSERT OR REPLACE INTO steps (idx, step_type, status, has_subtrajectory) VALUES (?, ?, 3, 0);`, idx, stepType); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	log.Info("ported session history from Claude to Antigravity", "uuid", uuid, "session", i.Title)
	return nil
}

func portAgyToClaude(i *Instance) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	// 1. Locate Agy Session ID
	agyHistoryPath := filepath.Join(home, ".gemini", "antigravity-cli", "history.jsonl")
	if _, err := os.Stat(agyHistoryPath); os.IsNotExist(err) {
		log.Info("no antigravity history file found to port from", "session", i.Title)
		return nil
	}

	workspaceStr := i.GetWorkingDirectory()
	workspace, err := NewWorkspacePath(workspaceStr)
	if err != nil {
		return err
	}

	// Read history.jsonl
	data, err := os.ReadFile(agyHistoryPath)
	if err != nil {
		return err
	}

	var uuid ConversationID
	lines := strings.Split(string(data), "\n")
	for idx := len(lines) - 1; idx >= 0; idx-- {
		line := strings.TrimSpace(lines[idx])
		if line == "" {
			continue
		}
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err == nil {
			entryWorkspaceStr, _ := entry["workspace"].(string)
			entryWorkspace, err := NewWorkspacePath(entryWorkspaceStr)
			if err == nil && entryWorkspace == workspace {
				if convID, ok := entry["conversationId"].(string); ok && convID != "" {
					parsedUUID, err := ParseConversationID(convID)
					if err == nil {
						uuid = parsedUUID
						break
					}
				}
			}
		}
	}

	if uuid == "" {
		log.Info("no matching antigravity session found in history to port from", "workspace", workspace)
		return nil
	}

	// 2. Open Antigravity JSONL
	agyLogPath := filepath.Join(home, ".gemini", "antigravity-cli", "brain", string(uuid), ".system_generated", "logs", "transcript_full.jsonl")
	if _, err := os.Stat(agyLogPath); os.IsNotExist(err) {
		agyLogPath = filepath.Join(home, ".gemini", "antigravity-cli", "brain", string(uuid), ".system_generated", "logs", "transcript.jsonl")
		if _, err := os.Stat(agyLogPath); os.IsNotExist(err) {
			log.Warn("antigravity log file not found", "uuid", uuid)
			return nil
		}
	}

	file, err := os.Open(agyLogPath)
	if err != nil {
		return err
	}
	defer file.Close()

	var unifiedTurns []UnifiedTurn
	reader := bufio.NewReader(file)
	for {
		lineBytes, err := reader.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return fmt.Errorf("failed to read Antigravity JSONL log: %w", err)
		}

		lineStr := strings.TrimSpace(string(lineBytes))
		if lineStr != "" {
			turn, err := ParseAgyStep([]byte(lineStr))
			if err != nil {
				log.Warn("skipping invalid jsonl line in Antigravity transcript", "err", err)
			} else if turn != nil {
				if _, ok := turn.(SkippedTurn); !ok {
					unifiedTurns = append(unifiedTurns, turn)
				}
			}
		}

		if err == io.EOF {
			break
		}
	}

	// 3. Write Claude project transcript
	projectDir := ClaudeProjectDirName(string(workspace))
	claudeProjectDir := filepath.Join(home, ".claude", "projects", projectDir)
	if err := os.MkdirAll(claudeProjectDir, 0700); err != nil {
		return err
	}

	claudeLogPath := filepath.Join(claudeProjectDir, string(uuid)+".jsonl")
	f, err := os.Create(claudeLogPath)
	if err != nil {
		return err
	}
	defer f.Close()

	parentUUID := ""
	for idx, turn := range unifiedTurns {
		turnUUID := fmt.Sprintf("%s-%d", string(uuid), idx)
		data, err := MarshalTurnToClaude(turn, string(uuid), string(workspace), parentUUID, turnUUID)
		if err != nil {
			return err
		}
		f.Write(data)
		f.Write([]byte("\n"))
		parentUUID = turnUUID
	}

	// 4. Append to Claude history.jsonl
	claudeHistoryPath := filepath.Join(home, ".claude", "history.jsonl")
	historyEntry := map[string]interface{}{
		"display":        fmt.Sprintf("Ported from Antigravity: %s", i.Title),
		"pastedContents": map[string]interface{}{},
		"timestamp":      time.Now().UnixMilli(),
		"project":        string(workspace),
		"sessionId":      string(uuid),
	}
	historyData, err := json.Marshal(historyEntry)
	if err == nil {
		hf, err := os.OpenFile(claudeHistoryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			hf.Write(historyData)
			hf.Write([]byte("\n"))
			hf.Close()
		}
	}

	// 5. Update the instance's Claude session data under mutex lock and trigger persistence
	i.stateMutex.Lock()
	if i.claudeSession == nil {
		i.claudeSession = &ClaudeSessionData{}
	}
	i.claudeSession.ConversationUUID = string(uuid)
	i.claudeSession.ProjectName = i.Title
	i.claudeSession.LastAttached = time.Now()
	if i.claudeSession.Metadata == nil {
		i.claudeSession.Metadata = make(map[string]string)
	}
	i.claudeSession.Metadata["working_dir"] = string(workspace)
	cb := i.claudeSessionIDSavedCallback
	i.stateMutex.Unlock()

	if cb != nil {
		cb()
	}

	log.Info("ported session history from Antigravity to Claude", "uuid", uuid, "session", i.Title)
	return nil
}

func nilIfEmptyInterface(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
