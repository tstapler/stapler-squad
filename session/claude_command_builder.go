package session

import (
	"fmt"
	"github.com/tstapler/stapler-squad/log"
	"regexp"
	"strings"
)

// ClaudeCommandBuilder constructs Claude CLI commands with session resumption support.
// This builder intelligently adds the --resume flag when appropriate to maintain
// conversation continuity across session restarts.
type ClaudeCommandBuilder struct {
	baseProgram   string
	claudeSession *ClaudeSessionData
}

// NewClaudeCommandBuilder creates a new command builder for constructing Claude CLI commands.
// Parameters:
//   - baseProgram: The base command string (e.g., "claude", "claude --model sonnet", "aider")
//   - claudeSession: Optional session data for resumption support (can be nil)
func NewClaudeCommandBuilder(baseProgram string, claudeSession *ClaudeSessionData) *ClaudeCommandBuilder {
	return &ClaudeCommandBuilder{
		baseProgram:   baseProgram,
		claudeSession: claudeSession,
	}
}

// Build constructs the final command string with session resumption if applicable.
// The method follows these rules:
//  1. If not a Claude command, returns baseProgram unchanged
//  2. If no session data exists, returns baseProgram unchanged
//  3. If session ID is invalid UUID, returns baseProgram unchanged with warning
//  4. If all conditions met, returns "baseProgram --resume <sessionId>"
func (b *ClaudeCommandBuilder) Build() string {
	// Non-Claude commands pass through unchanged
	if !b.isClaudeCommand() {
		return b.baseProgram
	}

	// No session data means no resumption possible. This is a routine, expected
	// condition (any non-resuming session start) rather than something worth a
	// log line -- Build() runs on every session start/restart, and a benchmark
	// exercising it at b.N in the tens of millions turned this single line into
	// gigabytes of output (and skewed timing with real log I/O per call). See
	// the analogous fix to getOrRefreshSnapshot's per-call logging.
	if b.claudeSession == nil || b.claudeSession.ConversationUUID == "" {
		return b.baseProgram
	}

	// Validate session ID format before using it
	sessionID := b.claudeSession.ConversationUUID
	if !isValidUUID(sessionID) {
		log.Warn("invalid uuid format for session id, skipping resumption", "uuid", sessionID)
		return b.baseProgram
	}

	// Construct command with resumption flag. Same reasoning as above for not
	// logging here: this runs on every session start/restart, a routine and
	// frequent event, not a hot-path-per-call log line.
	enrichedCommand := fmt.Sprintf("%s --resume %s", b.baseProgram, sessionID)
	return enrichedCommand
}

// isClaudeCommand checks if the base program is the Claude CLI.
// This supports various command formats:
//   - "claude" (simple)
//   - "claude --model sonnet" (with flags)
//   - "/usr/local/bin/claude" (full path)
//   - "/path/to/claude --arg" (full path with args)
func (b *ClaudeCommandBuilder) isClaudeCommand() bool {
	// Extract the command name (first word or basename if path)
	commandParts := strings.Fields(b.baseProgram)
	if len(commandParts) == 0 {
		return false
	}

	commandName := commandParts[0]
	// Get basename if it's a path
	if strings.Contains(commandName, "/") {
		parts := strings.Split(commandName, "/")
		commandName = parts[len(parts)-1]
	}

	return strings.ToLower(commandName) == "claude"
}

// isValidUUID validates that a string matches UUID v4 format.
// Format: 8-4-4-4-12 hexadecimal digits (e.g., 550e8400-e29b-41d4-a716-446655440000)
// This validation is critical because Claude CLI requires valid UUIDs for session IDs.
func isValidUUID(uuid string) bool {
	uuidRegex := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	return uuidRegex.MatchString(strings.ToLower(uuid))
}
