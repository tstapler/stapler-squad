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
	ID     string   `json:"id"`
	Title  string   `json:"title"`
	Status string   `json:"status"`
	Tags   []string `json:"tags"`
	Branch string   `json:"branch,omitempty"`
	// Path is the session's original/logical repo path, set once at creation
	// and never updated to reflect an actual worktree location — it identifies
	// the repo, not where the session's process is running. Kept unchanged for
	// backward compatibility with external MCP consumers.
	// Deprecated: use ActiveDir (where the session runs) or ExistingDir (where
	// to read/write files); see [[docs/reference/state-isolation.md]]'s path
	// vocabulary, mirrored from ConnectRPC's Session.active_dir/existing_dir.
	Path string `json:"path"`
	// ActiveDir is where this session's process is actually running: its git
	// worktree directory if it has one, else Path. Use this to compare whether
	// two sessions occupy the same location.
	ActiveDir string `json:"active_dir"`
	// ExistingDir is ActiveDir when that directory still exists on disk, else
	// Path. Use this only to open/read files — pause_session deletes a
	// worktree's directory while keeping the session's branch, so two
	// paused sessions in independently cleaned-up worktrees can share an
	// ExistingDir without being in the same place; never compare it.
	ExistingDir    string    `json:"existing_dir"`
	CreatedAt      time.Time `json:"created_at"`
	LastActivityAt time.Time `json:"last_activity_at"`
}

// SessionDetail extends SessionSummary with additional fields returned by get_session.
type SessionDetail struct {
	SessionSummary
	Program     string `json:"program"`
	SessionType string `json:"session_type"`
	// WorkingDir here is the user-configured subdirectory to start in,
	// relative to Path (Instance.WorkingDir in session/instance.go) — NOT the
	// resolved, worktree-aware absolute path that ConnectRPC's identically
	// named Session.working_dir field carries (that one is populated from
	// Workspace().ActiveDir; see instance_adapter.go). Same field name,
	// different source field, different meaning. Kept unchanged for backward
	// compatibility with external MCP consumers.
	// Deprecated: use SessionSummary.ActiveDir/ExistingDir for the resolved
	// path; this field's relative-subdirectory meaning has no ActiveDir-style
	// replacement here yet.
	WorkingDir string `json:"working_dir,omitempty"`
	// CreationWarning is set once, at create_session time, when a
	// new_worktree session's source repo had an ambient checked-out HEAD
	// that diverged from the default branch it actually branched from (see
	// session.Instance.CreationWarning) — a non-fatal signal, distinct from
	// silent success, that the caller should double check nothing unrelated
	// was expected to carry into this worktree.
	CreationWarning string `json:"creation_warning,omitempty"`
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

	// Async-session-creation Epic 2.3, Story 2.3.4: distinct outcomes for
	// create_session/create_session_for_pr's wait on the Background
	// Resolution Pipeline (server/services/session_creation_await.go).
	// ErrSessionCreationFailed: the pipeline reached a terminal Failed status
	// before the session ever became Active — distinct from ErrInternalError
	// so the caller can tell "resolution failed" from "the tool itself
	// errored."
	ErrSessionCreationFailed = "SESSION_CREATION_FAILED"
	// ErrSessionCreationCancelled: the instance vanished mid-wait (a
	// concurrent CancelSessionCreation removed it) — distinct from
	// ErrSessionNotFound, which means "never existed."
	ErrSessionCreationCancelled = "SESSION_CREATION_CANCELLED"
	// ErrSessionCreationAwaitCanceled: the MCP caller's own request context
	// ended before AwaitCreationTerminal observed a terminal status, a
	// timeout, or a vanish — distinct from both of the above and from the
	// timeout's still_creating success result.
	ErrSessionCreationAwaitCanceled = "SESSION_CREATION_AWAIT_CANCELED"
	// ErrSessionNotReady: the session's PTY/tmux stream hasn't produced a
	// single byte of scrollback since creation — read_session_output/
	// run_command return this instead of an empty-output success, so a
	// caller can tell "still initializing" from "the command legitimately
	// printed nothing" (session/scrollback.ScrollbackManager.CurrentSequence
	// == 0). Never returned once the session has emitted anything at all,
	// even if a later command is itself silent.
	ErrSessionNotReady = "SESSION_NOT_READY"
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
