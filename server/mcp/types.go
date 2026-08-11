package mcp

import "time"

// MCPError is the structured error returned in every tool result on failure.
type MCPError struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

// MCPResult is the top-level wrapper for all tool responses.
// On success, Success=true and Error is nil. On failure, Success=false and Error is set.
type MCPResult struct {
	Success bool      `json:"success"`
	Error   *MCPError `json:"error,omitempty"`
}

// SessionSummary is returned by list_sessions and search_sessions.
type SessionSummary struct {
	ID             string    `json:"id"`
	Title          string    `json:"title"`
	Status         string    `json:"status"`
	Tags           []string  `json:"tags"`
	Branch         string    `json:"branch,omitempty"`
	Path           string    `json:"path"`
	CreatedAt      time.Time `json:"created_at"`
	LastActivityAt time.Time `json:"last_activity_at"`
}

// SessionDetail extends SessionSummary with additional fields returned by get_session.
type SessionDetail struct {
	SessionSummary
	Program     string `json:"program"`
	SessionType string `json:"session_type"`
	WorkingDir  string `json:"working_dir,omitempty"`
}

// ListSessionsResult is returned by list_sessions.
type ListSessionsResult struct {
	MCPResult
	Sessions   []SessionSummary `json:"sessions"`
	TotalCount int              `json:"total_count"`
	NextCursor *string          `json:"next_cursor"`
}

// GetSessionResult is returned by get_session.
type GetSessionResult struct {
	MCPResult
	Session *SessionDetail `json:"session,omitempty"`
}

// SearchSessionsResult is returned by search_sessions.
type SearchSessionsResult struct {
	MCPResult
	Sessions   []SessionSummary `json:"sessions"`
	TotalCount int              `json:"total_count"`
}

// Error code constants — machine-readable identifiers for all tool failures.
const (
	ErrSessionNotFound       = "SESSION_NOT_FOUND"
	ErrInvalidArgument       = "INVALID_ARGUMENT"
	ErrInternalError         = "INTERNAL_ERROR"
	ErrConfirmationRequired  = "CONFIRMATION_REQUIRED"
	ErrInvalidStatusTrans    = "INVALID_STATUS_TRANSITION"
	ErrSessionNotRunning     = "SESSION_NOT_RUNNING"
	ErrRateLimitExceeded     = "RATE_LIMIT_EXCEEDED"
	ErrSessionStartupTimeout = "SESSION_STARTUP_TIMEOUT"
	ErrInvalidPath           = "INVALID_PATH"
	ErrPTYWriteTimeout       = "PTY_WRITE_TIMEOUT"
)
