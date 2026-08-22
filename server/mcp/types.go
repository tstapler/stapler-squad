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

// LinkSessionToItemResult is returned by link_session_to_item.
type LinkSessionToItemResult struct {
	MCPResult
	ItemID                   string `json:"item_id"`
	SessionUUID              string `json:"session_uuid"`
	ItemSessionID            string `json:"item_session_id"`
	AlreadyLinked            bool   `json:"already_linked"`
	PreviouslyLinkedItemID   string `json:"previously_linked_item_id,omitempty"`
	SlashCommandsRegenerated bool   `json:"slash_commands_regenerated"`
	ItemStatus               string `json:"item_status"`
}

// GetLinkedItemResult is returned by get_linked_item.
type GetLinkedItemResult struct {
	MCPResult
	Linked     bool       `json:"linked"`
	ItemID     string     `json:"item_id,omitempty"`
	ItemTitle  string     `json:"item_title,omitempty"`
	ItemStatus string     `json:"item_status,omitempty"`
	Role       string     `json:"role,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
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

// BacklogItemSummaryResult is a trimmed backlog item shown in list_backlog_items results.
type BacklogItemSummaryResult struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	Priority  int       `json:"priority"`
	CreatedAt time.Time `json:"created_at"`
}

// ListBacklogItemsResult is returned by list_backlog_items.
type ListBacklogItemsResult struct {
	MCPResult
	Items      []BacklogItemSummaryResult `json:"items"`
	TotalCount int                        `json:"total_count"`
	HasMore    bool                       `json:"has_more"`
}

// NotificationRecordResult is a single notification record returned by get_notification_history.
type NotificationRecordResult struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Type      string    `json:"type"`
	Priority  string    `json:"priority"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
	IsRead    bool      `json:"is_read"`
}

// GetNotificationHistoryResult is returned by get_notification_history.
type GetNotificationHistoryResult struct {
	MCPResult
	Notifications []NotificationRecordResult `json:"notifications"`
	TotalCount    int                        `json:"total_count"`
	UnreadCount   int                        `json:"unread_count"`
	HasMore       bool                       `json:"has_more"`
}

// SnippetResult is a single context snippet within a SearchResultSummary.
type SnippetResult struct {
	Text        string `json:"text"`
	MessageRole string `json:"message_role"`
}

// SearchResultSummary is a single matching conversation returned by search_claude_history.
type SearchResultSummary struct {
	SessionID   string          `json:"session_id"`
	SessionName string          `json:"session_name"`
	Project     string          `json:"project"`
	Score       float32         `json:"score"`
	Snippets    []SnippetResult `json:"snippets"`
}

// SearchClaudeHistoryResult is returned by search_claude_history.
type SearchClaudeHistoryResult struct {
	MCPResult
	Results     []SearchResultSummary `json:"results"`
	TotalCount  int                   `json:"total_count"`
	HasMore     bool                  `json:"has_more"`
	QueryTimeMs int64                 `json:"query_time_ms"`
}
