package session

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ClaudeAdapter struct{}

func NewClaudeAdapter() *ClaudeAdapter {
	return &ClaudeAdapter{}
}

func (a *ClaudeAdapter) Name() string {
	return "claude"
}

func (a *ClaudeAdapter) CanHandle(program string) bool {
	return strings.Contains(strings.ToLower(program), "claude")
}

type rawClaudeMessageBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type rawClaudeTurn struct {
	ParentUUID  *string `json:"parentUuid"`
	IsSidechain bool    `json:"isSidechain"`
	Type        string  `json:"type"`
	Message     *struct {
		Role    string      `json:"role"`
		Content interface{} `json:"content"`
	} `json:"message,omitempty"`
	UUID      string `json:"uuid"`
	Timestamp string `json:"timestamp"`
	SessionID string `json:"sessionId"`
	CWD       string `json:"cwd"`
}

func (a *ClaudeAdapter) Import(ctx context.Context, inst *Instance) ([]CanonicalTurn, error) {
	uuidStr := inst.GetClaudeConversationUUID()
	if uuidStr == "" {
		inst.tryExtractConversationUUID()
		uuidStr = inst.GetClaudeConversationUUID()
	}
	if uuidStr == "" {
		return nil, fmt.Errorf("no claude conversation UUID found")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	workspace := inst.GetWorkingDirectory()
	claudeProjectsDir := filepath.Join(home, ".claude", "projects")
	claudeLogPath := filepath.Join(claudeProjectsDir, ClaudeProjectDirName(workspace), uuidStr+".jsonl")

	return ReadCanonicalTurnsFromFile(claudeLogPath)
}

// ReadCanonicalTurnsFromFile parses a Claude JSONL transcript at the given
// path into CanonicalTurns, independent of any live Instance. It is the
// shared parsing core used both by ClaudeAdapter.Import (which resolves a
// path from an Instance's working directory + conversation UUID first) and
// by the import-external-session preview path (Story 1.1.4), which reads a
// resolved history file's turns without ever constructing a half-built
// Instance just to read history.
//
// Trailing partial lines (e.g. a JSONL file caught mid-write by a live
// writer) are tolerated: a line that fails to unmarshal as a well-formed
// turn is simply skipped rather than treated as an error, per the pitfalls
// research's "tolerate trailing partial lines" requirement.
func ReadCanonicalTurnsFromFile(path string) ([]CanonicalTurn, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("claude transcript file not found at %s: %w", path, err)
	}

	file, err := os.Open(path)
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
			return nil, fmt.Errorf("failed to read Claude JSONL log: %w", err)
		}

		lineStr := strings.TrimSpace(string(lineBytes))
		if lineStr != "" {
			var raw rawClaudeTurn
			if unmarshalErr := json.Unmarshal([]byte(lineStr), &raw); unmarshalErr == nil && raw.Message != nil {
				turn, buildErr := buildCanonicalTurnFromRawClaudeTurn(raw, turnIdx)
				if buildErr != nil {
					return nil, buildErr
				}
				turns = append(turns, turn)
				turnIdx++
			}
		}

		if err == io.EOF {
			break
		}
	}

	return turns, nil
}

// buildCanonicalTurnFromRawClaudeTurn converts a single parsed JSONL line
// (already known to carry a non-nil Message) into a CanonicalTurn, including
// content-block parsing, role/timestamp normalization, and validation.
func buildCanonicalTurnFromRawClaudeTurn(raw rawClaudeTurn, turnIdx int) (CanonicalTurn, error) {
	canonicalBlocks := parseClaudeMessageContent(raw.Message.Content)

	role := RoleUser
	if raw.Message.Role == "assistant" {
		role = RoleAssistant
	}

	tTime, parseErr := time.Parse(time.RFC3339Nano, raw.Timestamp)
	if parseErr != nil {
		tTime = time.Now()
	}

	turn := CanonicalTurn{
		Role:      role,
		Blocks:    canonicalBlocks,
		Timestamp: tTime,
		TurnIndex: turnIdx,
	}

	if err := turn.Validate(); err != nil {
		return CanonicalTurn{}, fmt.Errorf("invalid turn parsed at index %d: %w", turnIdx, err)
	}

	return turn, nil
}

// parseClaudeMessageContent parses a raw Claude message's content field,
// which may be either a plain string or a list of typed content blocks.
func parseClaudeMessageContent(content interface{}) []CanonicalBlock {
	if contentStr, ok := content.(string); ok {
		if contentStr == "" {
			return nil
		}
		return []CanonicalBlock{NewTextBlock(contentStr)}
	}

	contentList, ok := content.([]interface{})
	if !ok {
		return nil
	}

	var canonicalBlocks []CanonicalBlock
	for _, item := range contentList {
		if block, ok := parseClaudeContentBlockItem(item); ok {
			canonicalBlocks = append(canonicalBlocks, block)
		}
	}
	return canonicalBlocks
}

// parseClaudeContentBlockItem parses a single entry from a Claude message's
// content list into a CanonicalBlock. It returns ok=false for entries that
// fail to unmarshal or carry an unrecognized block type, which are silently
// skipped by the caller.
func parseClaudeContentBlockItem(item interface{}) (CanonicalBlock, bool) {
	var block rawClaudeMessageBlock
	itemBytes, err := json.Marshal(item)
	if err != nil {
		return CanonicalBlock{}, false
	}
	if err := json.Unmarshal(itemBytes, &block); err != nil {
		return CanonicalBlock{}, false
	}

	switch block.Type {
	case "text":
		return NewTextBlock(block.Text), true
	case "thinking":
		return NewThinkingBlock(block.Thinking), true
	case "tool_use":
		return NewToolUseBlock(block.ID, block.Name, block.Input), true
	case "tool_result":
		return NewToolResultBlock(block.ToolUseID, "", block.Content, block.IsError), true
	default:
		return CanonicalBlock{}, false
	}
}

func (a *ClaudeAdapter) Export(ctx context.Context, turns []CanonicalTurn, inst *Instance) error {
	uuidStr := inst.GetClaudeConversationUUID()
	if uuidStr == "" {
		return fmt.Errorf("no claude conversation UUID found")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	workspace := inst.GetWorkingDirectory()
	projectDir := ClaudeProjectDirName(workspace)
	claudeProjectDir := filepath.Join(home, ".claude", "projects", projectDir)
	if err := os.MkdirAll(claudeProjectDir, 0700); err != nil {
		return err
	}

	claudeLogPath := filepath.Join(claudeProjectDir, uuidStr+".jsonl")
	f, err := os.Create(claudeLogPath)
	if err != nil {
		return err
	}
	defer f.Close()

	parentUUID := ""
	for _, turn := range turns {
		turnUUID := uuid.New().String()
		var parentVal *string
		if parentUUID != "" {
			parentVal = &parentUUID
		}

		var content []map[string]interface{}
		for _, block := range turn.Blocks {
			switch block.Kind {
			case BlockKindText:
				content = append(content, map[string]interface{}{
					"type": "text",
					"text": block.Text,
				})
			case BlockKindThinking:
				content = append(content, map[string]interface{}{
					"type":     "thinking",
					"thinking": block.Text,
				})
			case BlockKindToolUse:
				var args map[string]interface{}
				_ = json.Unmarshal(block.ToolArgs, &args)
				content = append(content, map[string]interface{}{
					"type":  "tool_use",
					"id":    block.ToolID,
					"name":  block.ToolName,
					"input": args,
				})
			case BlockKindToolResult:
				content = append(content, map[string]interface{}{
					"type":        "tool_result",
					"tool_use_id": block.ToolResultID,
					"content":     block.ToolResultContent,
					"is_error":    block.ToolResultIsError,
				})
			case BlockKindImage:
				return fmt.Errorf("image blocks not yet supported")
			}
		}

		roleStr := "user"
		if turn.Role == RoleAssistant {
			roleStr = "assistant"
		}

		raw := rawClaudeTurn{
			ParentUUID:  parentVal,
			IsSidechain: false,
			Type:        roleStr,
			Message: &struct {
				Role    string      `json:"role"`
				Content interface{} `json:"content"`
			}{
				Role:    roleStr,
				Content: content,
			},
			UUID:      turnUUID,
			Timestamp: turn.Timestamp.Format(time.RFC3339Nano),
			SessionID: uuidStr,
			CWD:       workspace,
		}

		data, err := json.Marshal(raw)
		if err != nil {
			return err
		}
		if _, err := f.Write(data); err != nil {
			return fmt.Errorf("failed to write turn to claude transcript: %w", err)
		}
		if _, err := f.Write([]byte("\n")); err != nil {
			return fmt.Errorf("failed to write newline to claude transcript: %w", err)
		}
		parentUUID = turnUUID
	}

	return nil
}
