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

	"github.com/tstapler/stapler-squad/internal/sqlitedsn"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
	_ "modernc.org/sqlite" // Pure Go SQLite driver
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
	// "gemini" is intentionally excluded: it names the standalone Gemini CLI, whose
	// history format differs from Antigravity's own ~/.gemini/antigravity-cli/... storage
	// that this adapter reads and writes.
	return strings.Contains(p, "agy") || strings.Contains(p, "antigravity")
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

func (a *AgyAdapter) Import(ctx context.Context, inst *instance.Instance) ([]instance.CanonicalTurn, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	uuidStr := inst.GetClaudeConversationUUID()
	if uuidStr == "" {
		inst.TryExtractConversationUUID()
		uuidStr = inst.GetClaudeConversationUUID()
	}

	if uuidStr == "" {
		// Look up in history.jsonl
		uuidStr = agyConversationFromHistory(filepath.Join(home, ".gemini", "antigravity-cli", "history.jsonl"), inst.ActiveDir())

		if uuidStr != "" {
			inst.WithClaudeSession(func(d *domain.ClaudeSessionData) { d.ConversationUUID = uuidStr })
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

	file, err := os.Open(agyLogPath) // #nosec G304 -- agyLogPath is home dir + constants + uuidStr, which is either internal instance state (GetClaudeConversationUUID) or a conversationId this same app wrote into its own history.jsonl, never raw external input
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var turns []instance.CanonicalTurn
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
				turn, ok, turnErr := agyStepToTurn(raw, turnIdx)
				if turnErr != nil {
					return nil, turnErr
				}
				if ok {
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

// agyConversationFromHistory returns the conversation ID of the last
// history.jsonl entry whose workspace matches workspace, or "" if none.
func agyConversationFromHistory(historyPath, workspace string) string {
	f, err := os.Open(historyPath) // #nosec G304 -- historyPath is home dir + hardcoded constant components, no variable path segment
	if err != nil {
		return ""
	}
	defer f.Close()
	var found string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry struct {
			Workspace      string `json:"workspace"`
			ConversationID string `json:"conversationId"`
		}
		if json.Unmarshal(scanner.Bytes(), &entry) == nil {
			if filepath.Clean(entry.Workspace) == filepath.Clean(workspace) {
				found = entry.ConversationID
			}
		}
	}
	return found
}

// agyStepToTurn converts one transcript step to a canonical turn. ok is false
// for steps that produce no turn (unrecognised types, empty planner responses).
func agyStepToTurn(raw rawAgyStep, turnIdx int) (turn instance.CanonicalTurn, ok bool, err error) {
	tTime, parseErr := time.Parse(time.RFC3339, raw.CreatedAt)
	if parseErr != nil {
		tTime = time.Now()
	}

	switch {
	case raw.Type == "USER_INPUT":
		turn = instance.CanonicalTurn{
			Role: instance.RoleUser,
			Blocks: []instance.CanonicalBlock{
				instance.NewTextBlock(raw.Content),
			},
			Timestamp: tTime,
			TurnIndex: turnIdx,
		}
	case raw.Type == "PLANNER_RESPONSE":
		var blocks []instance.CanonicalBlock
		if raw.Content != "" {
			blocks = append(blocks, instance.NewTextBlock(raw.Content))
		}
		for _, tc := range raw.ToolCalls {
			blocks = append(blocks, instance.NewToolUseBlock(tc.ID, tc.Name, tc.Args))
		}
		if len(blocks) == 0 {
			// Skip empty planner responses that have no text and no tool calls.
			return turn, false, nil
		}
		turn = instance.CanonicalTurn{
			Role:      instance.RoleAssistant,
			Blocks:    blocks,
			Timestamp: tTime,
			TurnIndex: turnIdx,
		}
	case raw.Source == "MODEL" && raw.Type != "":
		// Tool result or other model execution output
		turn = instance.CanonicalTurn{
			Role: instance.RoleUser,
			Blocks: []instance.CanonicalBlock{
				instance.NewToolResultBlock("", raw.Type, raw.Content, false),
			},
			Timestamp: tTime,
			TurnIndex: turnIdx,
		}
	default:
		return turn, false, nil
	}
	if err := turn.Validate(); err != nil {
		return turn, false, fmt.Errorf("invalid turn parsed at index %d: %w", turnIdx, err)
	}
	return turn, true, nil
}

func (a *AgyAdapter) Export(ctx context.Context, turns []instance.CanonicalTurn, inst *instance.Instance) error {
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
		if err := writeAgyTranscript(p, turns); err != nil {
			return err
		}
	}

	// Create SQLite DB
	dbPath := filepath.Join(home, ".gemini", "antigravity-cli", "conversations", uuidStr+".db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		return err
	}
	return writeAgyConversationDB(dbPath, uuidStr, turns)
}

// writeAgyTranscript writes turns to p as one JSON step per line.
func writeAgyTranscript(p string, turns []instance.CanonicalTurn) error {
	f, err := os.Create(p) // #nosec G304 -- p is one of two fixed filenames joined onto agyBrainDir (home + constants + internal uuidStr), not externally controlled
	if err != nil {
		return err
	}
	for _, turn := range turns {
		data, err := json.Marshal(agyStepFromTurn(turn))
		if err != nil {
			_ = f.Close()
			return err
		}
		if _, werr := f.Write(data); werr != nil {
			_ = f.Close()
			return fmt.Errorf("failed to write step to %s: %w", p, werr)
		}
		if _, werr := f.Write([]byte("\n")); werr != nil {
			_ = f.Close()
			return fmt.Errorf("failed to write newline to %s: %w", p, werr)
		}
	}
	// A failed Close here can mean the last buffered writes never made
	// it to disk, so the transcript file would silently be incomplete —
	// surface it instead of discarding.
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to close transcript file %s: %w", p, err)
	}
	return nil
}

// agyStepFromTurn converts a canonical turn to a transcript step.
func agyStepFromTurn(turn instance.CanonicalTurn) rawAgyStep {
	var step rawAgyStep
	step.StepIndex = turn.TurnIndex
	step.CreatedAt = turn.Timestamp.Format(time.RFC3339)
	step.Status = "DONE"

	if turn.Role == instance.RoleUser {
		fillAgyUserStep(&step, turn)
		return step
	}
	step.Source = "MODEL"
	step.Type = "PLANNER_RESPONSE"
	var textBlocks []string
	for _, block := range turn.Blocks {
		switch block.Kind {
		case instance.BlockKindText:
			textBlocks = append(textBlocks, block.Text)
		case instance.BlockKindToolUse:
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
	return step
}

// fillAgyUserStep fills step for a user-role turn: a tool result when one of the
// blocks is a tool result, otherwise the joined text blocks as user input.
func fillAgyUserStep(step *rawAgyStep, turn instance.CanonicalTurn) {
	for _, block := range turn.Blocks {
		if block.Kind == instance.BlockKindToolResult {
			step.Source = "MODEL"
			step.Type = strings.ToUpper(block.ToolName)
			if step.Type == "" {
				step.Type = "RUN_COMMAND" // default
			}
			step.Content = block.ToolResultContent
			return
		}
	}
	step.Source = "USER_EXPLICIT"
	step.Type = "USER_INPUT"
	var textBlocks []string
	for _, block := range turn.Blocks {
		if block.Kind == instance.BlockKindText {
			textBlocks = append(textBlocks, block.Text)
		}
	}
	step.Content = strings.Join(textBlocks, "\n")
}

// agySchemaQueries creates the Antigravity conversation tables.
var agySchemaQueries = []string{
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

// writeAgyConversationDB creates the conversation SQLite DB at dbPath and
// inserts one steps/gen_metadata row per turn.
func writeAgyConversationDB(dbPath, uuidStr string, turns []instance.CanonicalTurn) error {
	db, err := sql.Open("sqlite", sqlitedsn.New(dbPath).Build()) //nolint:norawdb writes to the agy-owned database
	if err != nil {
		return err
	}
	defer db.Close()

	for _, q := range agySchemaQueries {
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
		stepType, contentLen := agyTurnSize(turn)

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

// agyTurnSize returns the step type (14 user, 15 planner response) and the
// approximate content length of turn.
func agyTurnSize(turn instance.CanonicalTurn) (stepType, contentLen int) {
	if turn.Role == instance.RoleUser {
		for _, b := range turn.Blocks {
			if b.Kind == instance.BlockKindToolResult {
				contentLen += len(b.ToolResultContent)
			} else {
				contentLen += len(b.Text)
			}
		}
		return 14, contentLen
	}
	for _, b := range turn.Blocks {
		contentLen += len(b.Text) + len(b.ToolArgs)
	}
	return 15, contentLen
}
