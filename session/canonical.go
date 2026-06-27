package session

import (
	"encoding/json"
	"fmt"
	"time"
)

type CanonicalBlockKind string

const (
	BlockKindText       CanonicalBlockKind = "text"
	BlockKindThinking   CanonicalBlockKind = "thinking"
	BlockKindToolUse    CanonicalBlockKind = "tool_use"
	BlockKindToolResult CanonicalBlockKind = "tool_result"
	BlockKindImage      CanonicalBlockKind = "image"
)

type CanonicalRole string

const (
	RoleUser      CanonicalRole = "user"
	RoleAssistant CanonicalRole = "assistant"
)

type CanonicalBlock struct {
	Kind              CanonicalBlockKind `json:"kind"`
	Text              string             `json:"text,omitempty"`
	ToolID            string             `json:"tool_id,omitempty"`
	ToolName          string             `json:"tool_name,omitempty"`
	ToolArgs          json.RawMessage    `json:"tool_args,omitempty"`
	ToolResultID      string             `json:"tool_result_id,omitempty"`
	ToolResultContent string             `json:"tool_result_content,omitempty"`
	ToolResultIsError bool               `json:"tool_result_is_error,omitempty"`
}

// NewTextBlock constructs a valid CanonicalBlock of text kind.
func NewTextBlock(text string) CanonicalBlock {
	return CanonicalBlock{
		Kind: BlockKindText,
		Text: text,
	}
}

// NewThinkingBlock constructs a valid CanonicalBlock of thinking kind.
func NewThinkingBlock(text string) CanonicalBlock {
	return CanonicalBlock{
		Kind: BlockKindThinking,
		Text: text,
	}
}

// NewToolUseBlock constructs a valid CanonicalBlock of tool_use kind.
func NewToolUseBlock(id, name string, args json.RawMessage) CanonicalBlock {
	return CanonicalBlock{
		Kind:     BlockKindToolUse,
		ToolID:   id,
		ToolName: name,
		ToolArgs: args,
	}
}

// NewToolResultBlock constructs a valid CanonicalBlock of tool_result kind.
func NewToolResultBlock(id, name, content string, isError bool) CanonicalBlock {
	return CanonicalBlock{
		Kind:              BlockKindToolResult,
		ToolResultID:      id,
		ToolName:          name,
		ToolResultContent: content,
		ToolResultIsError: isError,
	}
}

// Validate checks if the block is in a valid state.
func (b CanonicalBlock) Validate() error {
	switch b.Kind {
	case BlockKindText:
		if b.ToolID != "" || b.ToolName != "" || b.ToolArgs != nil || b.ToolResultID != "" || b.ToolResultContent != "" {
			return fmt.Errorf("text block cannot have tool use/result fields populated")
		}
	case BlockKindThinking:
		if b.Text == "" {
			return fmt.Errorf("thinking block cannot have empty text")
		}
	case BlockKindToolUse:
		if b.ToolName == "" {
			return fmt.Errorf("tool use block must have tool name")
		}
		if b.Text != "" || b.ToolResultID != "" || b.ToolResultContent != "" {
			return fmt.Errorf("tool use block cannot have text or tool result fields populated")
		}
	case BlockKindToolResult:
		if b.Text != "" || b.ToolArgs != nil {
			return fmt.Errorf("tool result block cannot have text or tool use arguments populated")
		}
	}
	return nil
}

type CanonicalTurn struct {
	Role      CanonicalRole    `json:"role"`
	Blocks    []CanonicalBlock `json:"blocks"`
	Timestamp time.Time        `json:"timestamp"`
	TurnIndex int              `json:"turn_index"`
	Model     string           `json:"model,omitempty"`
}

// Validate checks if the turn and all of its blocks are in a valid state.
func (t CanonicalTurn) Validate() error {
	if t.Role != RoleUser && t.Role != RoleAssistant {
		return fmt.Errorf("invalid role: %q", t.Role)
	}
	if len(t.Blocks) == 0 {
		return fmt.Errorf("turn must have at least one block")
	}
	for i, b := range t.Blocks {
		if err := b.Validate(); err != nil {
			return fmt.Errorf("block %d invalid: %w", i, err)
		}
	}
	return nil
}
