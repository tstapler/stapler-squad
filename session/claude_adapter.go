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

	claudeProjectsDir := filepath.Join(home, ".claude", "projects")
	var claudeLogPath string
	_ = filepath.Walk(claudeProjectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && info.Name() == uuidStr+".jsonl" {
			claudeLogPath = path
			return fmt.Errorf("found")
		}
		return nil
	})

	if claudeLogPath == "" {
		return nil, fmt.Errorf("claude transcript file not found for UUID %s", uuidStr)
	}

	file, err := os.Open(claudeLogPath)
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
			if err := json.Unmarshal([]byte(lineStr), &raw); err == nil && raw.Message != nil {
				var canonicalBlocks []CanonicalBlock

				// Parse content which can be a string or a list of blocks
				if contentStr, ok := raw.Message.Content.(string); ok {
					if contentStr != "" {
						canonicalBlocks = append(canonicalBlocks, NewTextBlock(contentStr))
					}
				} else if contentList, ok := raw.Message.Content.([]interface{}); ok {
					for _, item := range contentList {
						var block rawClaudeMessageBlock
						itemBytes, _ := json.Marshal(item)
						if err := json.Unmarshal(itemBytes, &block); err == nil {
							switch block.Type {
							case "text":
								canonicalBlocks = append(canonicalBlocks, NewTextBlock(block.Text))
							case "thinking":
								canonicalBlocks = append(canonicalBlocks, NewThinkingBlock(block.Thinking))
							case "tool_use":
								canonicalBlocks = append(canonicalBlocks, NewToolUseBlock(block.ID, block.Name, block.Input))
							case "tool_result":
								canonicalBlocks = append(canonicalBlocks, NewToolResultBlock(block.ToolUseID, "", block.Content, block.IsError))
							}
						}
					}
				}

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
					return nil, fmt.Errorf("invalid turn parsed at index %d: %w", turnIdx, err)
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
		turnUUID := fmt.Sprintf("%s-%d", uuidStr, turn.TurnIndex)
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
		f.Write(data)
		f.Write([]byte("\n"))
		parentUUID = turnUUID
	}

	return nil
}
