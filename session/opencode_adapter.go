package session

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/internal/sqlitedsn"
	"github.com/tstapler/stapler-squad/log"
	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

type OpencodeAdapter struct{}

func NewOpencodeAdapter() *OpencodeAdapter {
	return &OpencodeAdapter{}
}

func (a *OpencodeAdapter) Name() string {
	return "opencode"
}

func (a *OpencodeAdapter) CanHandle(program string) bool {
	return strings.Contains(strings.ToLower(program), "opencode")
}

type opencodeMessageJSON struct {
	Role string `json:"role"`
}

type opencodePartJSON struct {
	Type     string                 `json:"type"`
	Text     string                 `json:"text,omitempty"`
	Tool     string                 `json:"tool,omitempty"`
	CallID   string                 `json:"callID,omitempty"`
	State    map[string]interface{} `json:"state,omitempty"`
	Result   interface{}            `json:"result,omitempty"`
	IsError  bool                   `json:"isError,omitempty"`
	Snapshot string                 `json:"snapshot,omitempty"`
}

func (a *OpencodeAdapter) Import(ctx context.Context, inst *Instance) ([]CanonicalTurn, error) {
	uuidStr := inst.GetClaudeConversationUUID()
	if uuidStr == "" {
		inst.tryExtractConversationUUID()
		uuidStr = inst.GetClaudeConversationUUID()
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	dbPath := filepath.Join(home, ".local", "share", "opencode", "opencode.db")

	if uuidStr == "" {
		// Attempt to discover session ID from opencode.db matching directory
		if f, err := os.Stat(dbPath); err == nil && !f.IsDir() {
			db, err := sql.Open("sqlite", sqlitedsn.New(dbPath).Build())
			if err == nil {
				var foundID string
				query := `SELECT id FROM session WHERE directory = ? ORDER BY time_updated DESC LIMIT 1;`
				if scanErr := db.QueryRowContext(ctx, query, inst.GetWorkingDirectory()).Scan(&foundID); scanErr == nil && foundID != "" {
					uuidStr = foundID
				}
				_ = db.Close()
			}
		}

		if uuidStr != "" {
			inst.claudeSessionMu.Lock()
			if inst.claudeSession == nil {
				inst.claudeSession = &ClaudeSessionData{}
			}
			inst.claudeSession.ConversationUUID = uuidStr
			inst.claudeSessionMu.Unlock()
		}
	}

	if uuidStr == "" {
		return nil, fmt.Errorf("no opencode conversation UUID found")
	}

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("opencode database not found at %s", dbPath)
	}

	db, err := sql.Open("sqlite", sqlitedsn.New(dbPath).Build())
	if err != nil {
		return nil, fmt.Errorf("failed to open opencode database: %w", err)
	}
	defer db.Close()

	msgRows, err := db.QueryContext(ctx, `SELECT id, time_created, data FROM message WHERE session_id = ? ORDER BY time_created ASC;`, uuidStr)
	if err != nil {
		return nil, fmt.Errorf("failed to query opencode messages: %w", err)
	}
	defer msgRows.Close()

	type msgRecord struct {
		id          string
		timeCreated int64
		data        string
	}
	var msgs []msgRecord
	for msgRows.Next() {
		var rec msgRecord
		if err := msgRows.Scan(&rec.id, &rec.timeCreated, &rec.data); err == nil {
			msgs = append(msgs, rec)
		}
	}
	if err := msgRows.Err(); err != nil {
		return nil, fmt.Errorf("error reading opencode message rows: %w", err)
	}

	var turns []CanonicalTurn
	turnIdx := 0

	for _, msg := range msgs {
		var msgMeta opencodeMessageJSON
		_ = json.Unmarshal([]byte(msg.data), &msgMeta)

		partRows, err := db.QueryContext(ctx, `SELECT id, data FROM part WHERE message_id = ? ORDER BY time_created ASC, id ASC;`, msg.id)
		if err != nil {
			continue
		}

		var blocks []CanonicalBlock
		for partRows.Next() {
			var partID, partDataStr string
			if err := partRows.Scan(&partID, &partDataStr); err != nil {
				continue
			}

			var part opencodePartJSON
			if err := json.Unmarshal([]byte(partDataStr), &part); err != nil {
				continue
			}

			switch part.Type {
			case "text", "reasoning":
				if part.Text != "" {
					blocks = append(blocks, NewTextBlock(part.Text))
				}
			case "tool":
				var argsJSON json.RawMessage
				if part.State != nil {
					if inputVal, ok := part.State["input"]; ok {
						if b, err := json.Marshal(inputVal); err == nil {
							argsJSON = b
						}
					}
				}
				blocks = append(blocks, NewToolUseBlock(part.CallID, part.Tool, argsJSON))
			case "tool_result", "tool-result":
				resultText := part.Text
				if resultText == "" && part.Result != nil {
					if resStr, ok := part.Result.(string); ok {
						resultText = resStr
					} else if b, err := json.Marshal(part.Result); err == nil {
						resultText = string(b)
					}
				}
				blocks = append(blocks, NewToolResultBlock(part.CallID, part.Tool, resultText, part.IsError))
			}
		}
		_ = partRows.Close()

		if len(blocks) == 0 {
			continue
		}

		role := RoleUser
		if msgMeta.Role == "assistant" {
			role = RoleAssistant
		}

		tTime := time.UnixMilli(msg.timeCreated)
		if msg.timeCreated == 0 {
			tTime = time.Now()
		}

		turn := CanonicalTurn{
			Role:      role,
			Blocks:    blocks,
			Timestamp: tTime,
			TurnIndex: turnIdx,
		}
		if err := turn.Validate(); err != nil {
			return nil, fmt.Errorf("invalid opencode turn at index %d: %w", turnIdx, err)
		}
		turns = append(turns, turn)
		turnIdx++
	}

	return turns, nil
}

func (a *OpencodeAdapter) Export(ctx context.Context, turns []CanonicalTurn, inst *Instance) error {
	uuidStr := inst.GetClaudeConversationUUID()
	if uuidStr == "" {
		uuidStr = "ses_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:16]
	}

	inst.claudeSessionMu.Lock()
	if inst.claudeSession == nil {
		inst.claudeSession = &ClaudeSessionData{}
	}
	inst.claudeSession.ConversationUUID = uuidStr
	inst.claudeSessionMu.Unlock()

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	dbPath := filepath.Join(home, ".local", "share", "opencode", "opencode.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		return fmt.Errorf("failed to create directory for opencode database: %w", err)
	}

	db, err := sql.Open("sqlite", sqlitedsn.New(dbPath).Build())
	if err != nil {
		return fmt.Errorf("failed to open opencode database: %w", err)
	}
	defer db.Close()

	schema := []string{
		`CREATE TABLE IF NOT EXISTS project (
			id TEXT PRIMARY KEY,
			directory TEXT NOT NULL,
			name TEXT NOT NULL,
			time_created INTEGER NOT NULL,
			time_updated INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS session (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			directory TEXT NOT NULL,
			title TEXT NOT NULL,
			version TEXT NOT NULL,
			slug TEXT NOT NULL DEFAULT '',
			time_created INTEGER NOT NULL,
			time_updated INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS message (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			time_created INTEGER NOT NULL,
			time_updated INTEGER NOT NULL,
			data TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS part (
			id TEXT PRIMARY KEY,
			message_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			time_created INTEGER NOT NULL,
			time_updated INTEGER NOT NULL,
			data TEXT NOT NULL
		);`,
	}
	for _, q := range schema {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("failed to initialize opencode schema: %w", err)
		}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	now := time.Now().UnixMilli()
	workDir := inst.GetWorkingDirectory()
	projID := "proj_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:16]

	// Find existing project if any
	var existingProjID string
	_ = tx.QueryRowContext(ctx, `SELECT id FROM project WHERE directory = ? LIMIT 1;`, workDir).Scan(&existingProjID)
	if existingProjID != "" {
		projID = existingProjID
	} else {
		_, err := tx.ExecContext(ctx, `INSERT INTO project (id, directory, name, time_created, time_updated) VALUES (?, ?, ?, ?, ?);`,
			projID, workDir, filepath.Base(workDir), now, now)
		if err != nil {
			return fmt.Errorf("failed to insert opencode project: %w", err)
		}
	}

	title := inst.Title
	if title == "" {
		title = "Session " + uuidStr
	}

	_, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO session (id, project_id, directory, title, version, slug, time_created, time_updated) VALUES (?, ?, ?, ?, '1.0.0', ?, ?, ?);`,
		uuidStr, projID, workDir, title, title, now, now)
	if err != nil {
		return fmt.Errorf("failed to upsert opencode session: %w", err)
	}

	// Clean existing messages and parts for this session
	_, _ = tx.ExecContext(ctx, `DELETE FROM part WHERE session_id = ?;`, uuidStr)
	_, _ = tx.ExecContext(ctx, `DELETE FROM message WHERE session_id = ?;`, uuidStr)

	for _, turn := range turns {
		msgID := fmt.Sprintf("msg_%04d_%s", turn.TurnIndex, uuidStr)
		roleStr := "user"
		if turn.Role == RoleAssistant {
			roleStr = "assistant"
		}
		turnTime := turn.Timestamp.UnixMilli()
		if turnTime == 0 {
			turnTime = now
		}

		msgData := map[string]interface{}{
			"role": roleStr,
			"time": map[string]interface{}{
				"created": turnTime,
			},
		}
		msgDataBytes, err := json.Marshal(msgData)
		if err != nil {
			return fmt.Errorf("failed to marshal message data: %w", err)
		}

		_, err = tx.ExecContext(ctx, `INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES (?, ?, ?, ?, ?);`,
			msgID, uuidStr, turnTime, turnTime, string(msgDataBytes))
		if err != nil {
			return fmt.Errorf("failed to insert opencode message: %w", err)
		}

		for blockIdx, block := range turn.Blocks {
			partID := fmt.Sprintf("prt_%04d_%02d_%s", turn.TurnIndex, blockIdx, uuidStr)
			var partData map[string]interface{}

			switch block.Kind {
			case BlockKindText:
				partData = map[string]interface{}{
					"type": "text",
					"text": block.Text,
				}
			case BlockKindToolUse:
				var argsMap interface{}
				if len(block.ToolArgs) > 0 {
					_ = json.Unmarshal(block.ToolArgs, &argsMap)
				}
				partData = map[string]interface{}{
					"type":   "tool",
					"tool":   block.ToolName,
					"callID": block.ToolID,
					"state": map[string]interface{}{
						"input": argsMap,
					},
				}
			case BlockKindToolResult:
				callID := block.ToolResultID
				if callID == "" {
					callID = block.ToolID
				}
				partData = map[string]interface{}{
					"type":    "tool_result",
					"tool":    block.ToolName,
					"callID":  callID,
					"text":    block.ToolResultContent,
					"isError": block.ToolResultIsError,
				}
			}

			if partData != nil {
				partBytes, err := json.Marshal(partData)
				if err != nil {
					return fmt.Errorf("failed to marshal part data: %w", err)
				}
				_, err = tx.ExecContext(ctx, `INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES (?, ?, ?, ?, ?, ?);`,
					partID, msgID, uuidStr, turnTime, turnTime, string(partBytes))
				if err != nil {
					return fmt.Errorf("failed to insert opencode part: %w", err)
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit opencode transaction: %w", err)
	}

	log.Info("OpencodeAdapter: exported session history", "session", inst.Title, "uuid", uuidStr, "turns", len(turns))
	return nil
}

// Suppress unused imports warning if any
var _ = bufio.MaxScanTokenSize
