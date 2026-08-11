package session

import (
	"encoding/json"
	"testing"
)

func TestInhibitionEngine(t *testing.T) {
	ie := NewInhibitionEngine()

	t.Run("SanitizeString redacts API keys and secrets", func(t *testing.T) {
		input := "My key is sk-123456789012345678901234 and AWS is AKIAIOSFODNN7EXAMPLE"
		expected := "My key is [REDACTED_CREDENTIAL] and AWS is [REDACTED_CREDENTIAL]"

		result := ie.SanitizeString(input)
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("SanitizeTurn redacts CanonicalBlocks", func(t *testing.T) {
		rawArgs := json.RawMessage(`{"token": "Bearer secret_token_value", "public": "hello"}`)

		turn := CanonicalTurn{
			Role: RoleUser,
			Blocks: []CanonicalBlock{
				NewTextBlock("Here is password=secretpassword123"),
				NewToolUseBlock("tool-1", "test_tool", rawArgs),
				NewToolResultBlock("tool-1", "test_tool", "Result with sk-abcdefghijklmnopqrstuvwxyz", false),
			},
		}

		sanitized := ie.SanitizeTurn(turn)

		// Check Text block
		if sanitized.Blocks[0].Text != "Here is [REDACTED_CREDENTIAL]" {
			t.Errorf("text block not redacted properly: %s", sanitized.Blocks[0].Text)
		}

		// Check ToolUse block args
		var argsMap map[string]interface{}
		if err := json.Unmarshal(sanitized.Blocks[1].ToolArgs, &argsMap); err != nil {
			t.Fatalf("failed to unmarshal sanitized tool args: %v", err)
		}
		if argsMap["token"] != "[REDACTED_CREDENTIAL]" {
			t.Errorf("tool arg token not redacted properly: %v", argsMap["token"])
		}
		if argsMap["public"] != "hello" {
			t.Errorf("public tool arg should remain unchanged: %v", argsMap["public"])
		}

		// Check ToolResult block
		if sanitized.Blocks[2].ToolResultContent != "Result with [REDACTED_CREDENTIAL]" {
			t.Errorf("tool result content not redacted properly: %s", sanitized.Blocks[2].ToolResultContent)
		}
	})
}
