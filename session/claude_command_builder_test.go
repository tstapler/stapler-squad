package session

import (
	"testing"
)

// TestClaudeCommandBuilder_Build tests the Build() method with various scenarios
func TestClaudeCommandBuilder_Build(t *testing.T) {
	validSessionID := "550e8400-e29b-41d4-a716-446655440000"
	invalidSessionID := "not-a-uuid"

	tests := []struct {
		name          string
		baseProgram   string
		claudeSession *ClaudeSessionData
		expected      string
		description   string
	}{
		{
			name:        "claude command with valid session",
			baseProgram: "claude",
			claudeSession: &ClaudeSessionData{
				ConversationUUID: validSessionID,
			},
			expected:    "claude --resume 550e8400-e29b-41d4-a716-446655440000",
			description: "Should append --resume flag with valid session ID",
		},
		{
			name:        "claude command with flags and valid session",
			baseProgram: "claude --model sonnet",
			claudeSession: &ClaudeSessionData{
				ConversationUUID: validSessionID,
			},
			expected:    "claude --model sonnet --resume 550e8400-e29b-41d4-a716-446655440000",
			description: "Should append --resume to command with existing flags",
		},
		{
			name:        "claude full path with valid session",
			baseProgram: "/usr/local/bin/claude",
			claudeSession: &ClaudeSessionData{
				ConversationUUID: validSessionID,
			},
			expected:    "/usr/local/bin/claude --resume 550e8400-e29b-41d4-a716-446655440000",
			description: "Should handle full path commands",
		},
		{
			name:          "claude command without session data",
			baseProgram:   "claude",
			claudeSession: nil,
			expected:      "claude",
			description:   "Should return unchanged when no session data",
		},
		{
			name:        "claude command with empty session ID",
			baseProgram: "claude",
			claudeSession: &ClaudeSessionData{
				ConversationUUID: "",
			},
			expected:    "claude",
			description: "Should return unchanged when session ID is empty",
		},
		{
			name:        "claude command with invalid UUID",
			baseProgram: "claude",
			claudeSession: &ClaudeSessionData{
				ConversationUUID: invalidSessionID,
			},
			expected:    "claude",
			description: "Should return unchanged when session ID is not valid UUID",
		},
		{
			name:        "non-claude command with session data",
			baseProgram: "aider --model ollama_chat/gemma3:1b",
			claudeSession: &ClaudeSessionData{
				ConversationUUID: validSessionID,
			},
			expected:    "aider --model ollama_chat/gemma3:1b",
			description: "Should not modify non-Claude commands",
		},
		{
			name:          "empty command",
			baseProgram:   "",
			claudeSession: nil,
			expected:      "",
			description:   "Should handle empty command string",
		},
		{
			name:        "mixed case claude command",
			baseProgram: "Claude",
			claudeSession: &ClaudeSessionData{
				ConversationUUID: validSessionID,
			},
			expected:    "Claude --resume 550e8400-e29b-41d4-a716-446655440000",
			description: "Should handle case-insensitive Claude detection",
		},
		{
			name:        "claude in path with args",
			baseProgram: "/home/user/.local/bin/claude --verbose",
			claudeSession: &ClaudeSessionData{
				ConversationUUID: validSessionID,
			},
			expected:    "/home/user/.local/bin/claude --verbose --resume 550e8400-e29b-41d4-a716-446655440000",
			description: "Should handle full path with arguments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewClaudeCommandBuilder(tt.baseProgram, tt.claudeSession)
			result := builder.Build()

			if result != tt.expected {
				t.Errorf("%s\nExpected: %s\nGot:      %s", tt.description, tt.expected, result)
			}
		})
	}
}

// TestClaudeCommandBuilder_isClaudeCommand tests Claude command detection
func TestClaudeCommandBuilder_isClaudeCommand(t *testing.T) {
	tests := []struct {
		name        string
		baseProgram string
		expected    bool
		description string
	}{
		{
			name:        "simple claude command",
			baseProgram: "claude",
			expected:    true,
			description: "Should detect simple 'claude' command",
		},
		{
			name:        "claude with flags",
			baseProgram: "claude --model sonnet",
			expected:    true,
			description: "Should detect claude with flags",
		},
		{
			name:        "claude full path",
			baseProgram: "/usr/local/bin/claude",
			expected:    true,
			description: "Should detect claude with full path",
		},
		{
			name:        "mixed case claude",
			baseProgram: "Claude",
			expected:    true,
			description: "Should detect case-insensitive",
		},
		{
			name:        "aider command",
			baseProgram: "aider",
			expected:    false,
			description: "Should not detect aider as claude",
		},
		{
			name:        "empty command",
			baseProgram: "",
			expected:    false,
			description: "Should return false for empty command",
		},
		{
			name:        "command containing claude but not claude itself",
			baseProgram: "myclaude-tool",
			expected:    false,
			description: "Should not detect commands that merely contain 'claude'",
		},
		{
			name:        "path with claude in directory but different command",
			baseProgram: "/claude/bin/aider",
			expected:    false,
			description: "Should check basename, not full path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewClaudeCommandBuilder(tt.baseProgram, nil)
			result := builder.isClaudeCommand()

			if result != tt.expected {
				t.Errorf("%s\nExpected: %v\nGot:      %v", tt.description, tt.expected, result)
			}
		})
	}
}

// TestIsValidUUID tests UUID validation
func TestIsValidUUID(t *testing.T) {
	tests := []struct {
		name        string
		uuid        string
		expected    bool
		description string
	}{
		{
			name:        "valid UUID v4",
			uuid:        "550e8400-e29b-41d4-a716-446655440000",
			expected:    true,
			description: "Should accept valid UUID v4 format",
		},
		{
			name:        "valid UUID with uppercase",
			uuid:        "550E8400-E29B-41D4-A716-446655440000",
			expected:    true,
			description: "Should accept uppercase UUID",
		},
		{
			name:        "invalid UUID - missing section",
			uuid:        "550e8400-e29b-41d4-446655440000",
			expected:    false,
			description: "Should reject UUID missing a section",
		},
		{
			name:        "invalid UUID - wrong format",
			uuid:        "not-a-uuid",
			expected:    false,
			description: "Should reject non-UUID strings",
		},
		{
			name:        "invalid UUID - too short",
			uuid:        "550e8400",
			expected:    false,
			description: "Should reject UUID that's too short",
		},
		{
			name:        "invalid UUID - no hyphens",
			uuid:        "550e8400e29b41d4a716446655440000",
			expected:    false,
			description: "Should reject UUID without hyphens",
		},
		{
			name:        "empty string",
			uuid:        "",
			expected:    false,
			description: "Should reject empty string",
		},
		{
			name:        "invalid UUID - contains invalid characters",
			uuid:        "550e8400-e29b-41d4-a716-44665544000g",
			expected:    false,
			description: "Should reject UUID with non-hex characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidUUID(tt.uuid)

			if result != tt.expected {
				t.Errorf("%s\nUUID:     %s\nExpected: %v\nGot:      %v", tt.description, tt.uuid, tt.expected, result)
			}
		})
	}
}

// BenchmarkClaudeCommandBuilder_Build benchmarks the Build() method
func BenchmarkClaudeCommandBuilder_Build(b *testing.B) {
	validSession := &ClaudeSessionData{
		ConversationUUID: "550e8400-e29b-41d4-a716-446655440000",
	}

	benchmarks := []struct {
		name        string
		baseProgram string
		session     *ClaudeSessionData
	}{
		{
			name:        "claude with session",
			baseProgram: "claude",
			session:     validSession,
		},
		{
			name:        "claude without session",
			baseProgram: "claude",
			session:     nil,
		},
		{
			name:        "non-claude command",
			baseProgram: "aider --model ollama",
			session:     validSession,
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			builder := NewClaudeCommandBuilder(bm.baseProgram, bm.session)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = builder.Build()
			}
		})
	}
}
