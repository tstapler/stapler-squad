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
)

type AgyAdapter struct{}

func NewAgyAdapter() *AgyAdapter {
	return &AgyAdapter{}
}

func (a *AgyAdapter) Name() string {
	return "agy"
}

func (a *AgyAdapter) CanHandle(program string) bool {
	p := strings.ToLower(program)
	return strings.Contains(p, "agy") || strings.Contains(p, "antigravity") || strings.Contains(p, "gemini")
}

type rawAgyStep struct {
	StepIndex int    `json:"step_index"`
	Source    string `json:"source"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	Content   string `json:"content,omitempty"`
	ToolCalls []struct {
		Name string          `json:"name"`
		Args json.RawMessage `json:"args"`
		ID   string          `json:"id,omitempty"`
	} `json:"tool_calls,omitempty"`
}

func (a *AgyAdapter) Import(ctx context.Context, inst *Instance) ([]CanonicalTurn, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	uuidStr := inst.GetClaudeConversationUUID()
	if uuidStr == "" {
		inst.tryExtractConversationUUID()
		uuidStr = inst.GetClaudeConversationUUID()
	}

	if uuidStr == "" {
		// Look up in history.jsonl
		agyHistoryPath := filepath.Join(home, ".gemini", "antigravity-cli", "history.jsonl")
		if f, err := os.Open(agyHistoryPath); err == nil {
			defer f.Close()
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				var entry struct {
					Workspace      string `json:"workspace"`
					ConversationID string `json:"conversationId"`
				}
				if json.Unmarshal(scanner.Bytes(), &entry) == nil {
					if filepath.Clean(entry.Workspace) == filepath.Clean(inst.GetWorkingDirectory()) {
						uuidStr = entry.ConversationID
					}
				}
			}
		}

		if uuidStr != "" {
			inst.stateMutex.Lock()
			if inst.claudeSession == nil {
				inst.claudeSession = &ClaudeSessionData{}
			}
			inst.claudeSession.ConversationUUID = uuidStr
			inst.stateMutex.Unlock()
		}
	}

	if uuidStr == "" {
		return nil, fmt.Errorf("no conversation UUID found")
	}

	agyLogPath := filepath.Join(home, ".gemini", "antigravity-cli", "brain", uuidStr, ".system_generated", "logs", "transcript_full.jsonl")
	if _, err := os.Stat(agyLogPath); os.IsNotExist(err) {
		agyLogPath = filepath.Join(home, ".gemini", "antigravity-cli", "brain", uuidStr, ".system_generated", "logs", "transcript.jsonl")
		if _, err := os.Stat(agyLogPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("antigravity log file not found for UUID %s", uuidStr)
		}
	}

	file, err := os.Open(agyLogPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var turns []CanonicalTurn
	reader := bufio.NewReader(file)
	turnIdx := 0

	for {
		lineBytes, err := reader.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("failed to read Antigravity JSONL log: %w", err)
		}

		lineStr := strings.TrimSpace(string(lineBytes))
		if lineStr != "" {
			var raw rawAgyStep
			if err := json.Unmarshal([]byte(lineStr), &raw); err == nil {
				tTime, parseErr := time.Parse(time.RFC3339, raw.CreatedAt)
				if parseErr != nil {
					tTime = time.Now()
				}

				if raw.Type == "USER_INPUT" {
					turn := CanonicalTurn{
						Role: RoleUser,
						Blocks: []CanonicalBlock{
							NewTextBlock(raw.Content),
						},
						Timestamp: tTime,
						TurnIndex: turnIdx,
					}
					if err := turn.Validate(); err != nil {
						return nil, fmt.Errorf("invalid turn parsed at index %d: %w", turnIdx, err)
					}
					turns = append(turns, turn)
					turnIdx++
				} else if raw.Type == "PLANNER_RESPONSE" {
					var blocks []CanonicalBlock
					if raw.Content != "" {
						blocks = append(blocks, NewTextBlock(raw.Content))
					}
					for _, tc := range raw.ToolCalls {
						blocks = append(blocks, NewToolUseBlock(tc.ID, tc.Name, tc.Args))
					}
					if len(blocks) == 0 {
						// Skip empty planner responses that have no text and no tool calls.
						continue
					}
					turn := CanonicalTurn{
						Role:      RoleAssistant,
						Blocks:    blocks,
						Timestamp: tTime,
						TurnIndex: turnIdx,
					}
					if err := turn.Validate(); err != nil {
						return nil, fmt.Errorf("invalid turn parsed at index %d: %w", turnIdx, err)
					}
					turns = append(turns, turn)
					turnIdx++
				} else if raw.Source == "MODEL" && raw.Type != "" {
					// Tool result or other model execution output
					turn := CanonicalTurn{
						Role: RoleUser,
						Blocks: []CanonicalBlock{
							NewToolResultBlock("", raw.Type, raw.Content, false),
						},
						Timestamp: tTime,
						TurnIndex: turnIdx,
					}
					if err := turn.Validate(); err != nil {
						return nil, fmt.Errorf("invalid turn parsed at index %d: %w", turnIdx, err)
					}
					turns = append(turns, turn)
					turnIdx++
				}
			}
		}

		if err == io.EOF {
			break
		}
	}

	return turns, nil
}

func (a *AgyAdapter) Export(ctx context.Context, turns []CanonicalTurn, inst *Instance) error {
	uuidStr := inst.GetClaudeConversationUUID()
	if uuidStr == "" {
		return fmt.Errorf("no conversation UUID found")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	agyBrainDir := filepath.Join(home, ".gemini", "antigravity-cli", "brain", uuidStr, ".system_generated", "logs")
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
		for _, turn := range turns {
			var step rawAgyStep
			step.StepIndex = turn.TurnIndex
			step.CreatedAt = turn.Timestamp.Format(time.RFC3339)
			step.Status = "DONE"

			if turn.Role == RoleUser {
				isToolResult := false
				for _, block := range turn.Blocks {
					if block.Kind == BlockKindToolResult {
						step.Source = "MODEL"
						step.Type = strings.ToUpper(block.ToolName)
						if step.Type == "" {
							step.Type = "RUN_COMMAND" // default
						}
						step.Content = block.ToolResultContent
						isToolResult = true
						break
					}
				}
				if !isToolResult {
					step.Source = "USER_EXPLICIT"
					step.Type = "USER_INPUT"
					var textBlocks []string
					for _, block := range turn.Blocks {
						if block.Kind == BlockKindText {
							textBlocks = append(textBlocks, block.Text)
						}
					}
					step.Content = strings.Join(textBlocks, "\n")
				}
			} else {
				step.Source = "MODEL"
				step.Type = "PLANNER_RESPONSE"
				var textBlocks []string
				for _, block := range turn.Blocks {
					if block.Kind == BlockKindText {
						textBlocks = append(textBlocks, block.Text)
					} else if block.Kind == BlockKindToolUse {
						step.ToolCalls = append(step.ToolCalls, struct {
							Name string          `json:"name"`
							Args json.RawMessage `json:"args"`
							ID   string          `json:"id,omitempty"`
						}{
							Name: block.ToolName,
							Args: block.ToolArgs,
							ID:   block.ToolID,
						})
					}
				}
				step.Content = strings.Join(textBlocks, "\n")
			}

			data, err := json.Marshal(step)
			if err != nil {
				f.Close()
				return err
			}
			if _, werr := f.Write(data); werr != nil {
				f.Close()
				return fmt.Errorf("failed to write step to %s: %w", p, werr)
			}
			if _, werr := f.Write([]byte("\n")); werr != nil {
				f.Close()
				return fmt.Errorf("failed to write newline to %s: %w", p, werr)
			}
		}
		f.Close()
	}

	// Create SQLite DB
	dbPath := filepath.Join(home, ".gemini", "antigravity-cli", "conversations", uuidStr+".db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		return err
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

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
		`CREATE INDEX IF NOT EXISTS idx_steps_status    ON steps(status);`,
		`CREATE INDEX IF NOT EXISTS idx_steps_step_type ON steps(step_type);`,
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

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`INSERT OR REPLACE INTO trajectory_meta (trajectory_id, cascade_id, trajectory_type, source) VALUES (?, ?, 0, 0);`, uuidStr, uuidStr); err != nil {
		return err
	}

	for _, turn := range turns {
		stepType := 15 // PLANNER_RESPONSE
		contentLen := 0
		if turn.Role == RoleUser {
			stepType = 14
			for _, b := range turn.Blocks {
				if b.Kind == BlockKindToolResult {
					contentLen += len(b.ToolResultContent)
				} else {
					contentLen += len(b.Text)
				}
			}
		} else {
			for _, b := range turn.Blocks {
				contentLen += len(b.Text) + len(b.ToolArgs)
			}
		}

		if _, err := tx.Exec(`INSERT OR REPLACE INTO steps (idx, step_type, status, has_subtrajectory) VALUES (?, ?, 3, 0);`, turn.TurnIndex, stepType); err != nil {
			return err
		}

		// Insert approximate size into gen_metadata for Gemini limits querying
		if _, err := tx.Exec(`INSERT OR REPLACE INTO gen_metadata (idx, data, size) VALUES (?, ?, ?);`, turn.TurnIndex, []byte{}, contentLen); err != nil {
			return err
		}
	}

	return tx.Commit()
}
