package session

import (
	"bytes"
	"strings"
)

// StaleResumeDetector is a strategy interface for detecting program-specific
// error messages emitted when resuming a stale or expired session ID.
type StaleResumeDetector interface {
	Name() string
	CanHandle(program string) bool
	IsStaleResumeExit(exitContent []byte) bool
}

// resolveStaleResumeDetector returns the StaleResumeDetector registered for program,
// or DefaultStaleResumeDetector if no program-specific detector claims it.
func resolveStaleResumeDetector(program string) StaleResumeDetector {
	p := strings.ToLower(program)
	if strings.Contains(p, "claude") {
		return &ClaudeStaleResumeDetector{}
	}
	if strings.Contains(p, "opencode") {
		return &OpencodeStaleResumeDetector{}
	}
	if strings.Contains(p, "agy") || strings.Contains(p, "antigravity") {
		return &AgyStaleResumeDetector{}
	}
	if strings.Contains(p, "gemini") {
		return &GeminiStaleResumeDetector{}
	}
	if strings.Contains(p, "pi") {
		return &PiStaleResumeDetector{}
	}
	return &DefaultStaleResumeDetector{}
}

// ClaudeStaleResumeDetector detects stale session exit patterns emitted by Claude Code.
type ClaudeStaleResumeDetector struct{}

func (d *ClaudeStaleResumeDetector) Name() string { return "claude" }
func (d *ClaudeStaleResumeDetector) CanHandle(program string) bool {
	return strings.Contains(strings.ToLower(program), "claude")
}
func (d *ClaudeStaleResumeDetector) IsStaleResumeExit(exitContent []byte) bool {
	return containsAnyPattern(exitContent, []string{
		"No conversation found with session ID",
		"No conversation found with ID",
		"session expired",
		"invalid session id",
	})
}

// OpencodeStaleResumeDetector detects stale session exit patterns emitted by OpenCode CLI.
type OpencodeStaleResumeDetector struct{}

func (d *OpencodeStaleResumeDetector) Name() string { return "opencode" }
func (d *OpencodeStaleResumeDetector) CanHandle(program string) bool {
	return strings.Contains(strings.ToLower(program), "opencode")
}
func (d *OpencodeStaleResumeDetector) IsStaleResumeExit(exitContent []byte) bool {
	return containsAnyPattern(exitContent, []string{
		"session not found",
		"Session not found",
		"invalid session id",
		"failed to load session",
	})
}

// AgyStaleResumeDetector detects stale session exit patterns emitted by Antigravity CLI.
type AgyStaleResumeDetector struct{}

func (d *AgyStaleResumeDetector) Name() string { return "agy" }
func (d *AgyStaleResumeDetector) CanHandle(program string) bool {
	p := strings.ToLower(program)
	return strings.Contains(p, "agy") || strings.Contains(p, "antigravity")
}
func (d *AgyStaleResumeDetector) IsStaleResumeExit(exitContent []byte) bool {
	return containsAnyPattern(exitContent, []string{
		"trajectory not found",
		"No conversation found",
		"invalid trajectory_id",
		"Session not found",
		"session not found",
	})
}

// GeminiStaleResumeDetector detects stale session exit patterns emitted by Gemini CLI.
type GeminiStaleResumeDetector struct{}

func (d *GeminiStaleResumeDetector) Name() string { return "gemini" }
func (d *GeminiStaleResumeDetector) CanHandle(program string) bool {
	return strings.Contains(strings.ToLower(program), "gemini")
}
func (d *GeminiStaleResumeDetector) IsStaleResumeExit(exitContent []byte) bool {
	return containsAnyPattern(exitContent, []string{
		"conversation not found",
		"invalid session",
		"Session not found",
		"session not found",
	})
}

// PiStaleResumeDetector detects stale session exit patterns emitted by Pi CLI.
type PiStaleResumeDetector struct{}

func (d *PiStaleResumeDetector) Name() string { return "pi" }
func (d *PiStaleResumeDetector) CanHandle(program string) bool {
	return strings.Contains(strings.ToLower(program), "pi")
}
func (d *PiStaleResumeDetector) IsStaleResumeExit(exitContent []byte) bool {
	return containsAnyPattern(exitContent, []string{
		"Session not found",
		"session not found",
		"No session found",
		"unknown session id",
	})
}

// DefaultStaleResumeDetector is a fallback detector checking common session-not-found patterns.
type DefaultStaleResumeDetector struct{}

func (d *DefaultStaleResumeDetector) Name() string                  { return "default" }
func (d *DefaultStaleResumeDetector) CanHandle(program string) bool { return true }
func (d *DefaultStaleResumeDetector) IsStaleResumeExit(exitContent []byte) bool {
	return containsAnyPattern(exitContent, []string{
		"session not found",
		"Session not found",
		"No conversation found",
		"invalid session",
	})
}

// containsAnyPattern checks whether exitContent (with ANSI escape sequences stripped)
// contains any of the given plain-text error patterns (case-insensitive where helpful).
func containsAnyPattern(exitContent []byte, patterns []string) bool {
	if len(exitContent) == 0 {
		return false
	}
	clean := stripANSISimple(exitContent)
	for _, p := range patterns {
		if bytes.Contains(clean, []byte(p)) || bytes.Contains(bytes.ToLower(clean), []byte(strings.ToLower(p))) {
			return true
		}
	}
	return false
}
