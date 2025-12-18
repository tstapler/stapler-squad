package session

// LoadOptions controls what child data is loaded for sessions.
// This allows selective loading to optimize performance by avoiding unnecessary data retrieval.
type LoadOptions struct {
	// LoadWorktree controls whether git worktree data is loaded
	LoadWorktree bool

	// LoadDiffStats controls whether diff statistics (added/removed counts) are loaded
	LoadDiffStats bool

	// LoadDiffContent controls whether full diff content is loaded
	// Note: This implies LoadDiffStats=true, as we need counts to interpret content
	LoadDiffContent bool

	// LoadTags controls whether session tags are loaded
	LoadTags bool

	// LoadClaudeSession controls whether Claude Code session data is loaded
	LoadClaudeSession bool
}

// Common preset configurations for different use cases
//
// MIGRATION NOTE: These presets are for use with InstanceData operations.
// For new code using the Session domain model, prefer ContextOptions presets
// in context_options.go (e.g., ContextMinimal, ContextUIView, ContextFull).
// See session.go for InstanceToSession and SessionToInstance adapters.

// LoadMinimal loads only the core session fields without any child data.
// Use this when you only need session metadata (title, path, status, etc.)
//
// Deprecated: For new code, use ContextMinimal with GetSession/ListSessions.
var LoadMinimal = LoadOptions{}

// LoadSummary loads lightweight child data suitable for list views.
// This includes everything except the heavy diff content.
// Memory usage: ~1-2 KB per session
//
// Deprecated: For new code, use ContextUIView with GetSession/ListSessions.
var LoadSummary = LoadOptions{
	LoadWorktree:      true,
	LoadDiffStats:     true, // Only counts, not content
	LoadDiffContent:   false,
	LoadTags:          true,
	LoadClaudeSession: true,
}

// LoadFull loads all available data including full diff content.
// Use this for detail views where you need complete information.
// Memory usage: Can be 1-25 MB per session depending on diff size
//
// Deprecated: For new code, use ContextFull with GetSession/ListSessions.
var LoadFull = LoadOptions{
	LoadWorktree:      true,
	LoadDiffStats:     true,
	LoadDiffContent:   true, // Loads full diff text
	LoadTags:          true,
	LoadClaudeSession: true,
}

// LoadDiffOnly loads only diff-related data, useful for preview panes.
//
// Deprecated: For new code, use ContextTerminalView.WithDiffContent() with GetSession/ListSessions.
var LoadDiffOnly = LoadOptions{
	LoadWorktree:    true, // Needed for diff context
	LoadDiffStats:   true,
	LoadDiffContent: true,
}

// LoadForReviewQueue loads data needed for review queue operations.
//
// Deprecated: For new code, use ContextForReviewQueue with GetSession/ListSessions.
var LoadForReviewQueue = LoadOptions{
	LoadWorktree:      true, // For branch info
	LoadDiffStats:     true, // For change indicators
	LoadDiffContent:   false,
	LoadTags:          true,  // For filtering
	LoadClaudeSession: false,
}

// WithDiffContent returns a copy of options with diff content loading enabled.
func (o LoadOptions) WithDiffContent() LoadOptions {
	o.LoadDiffContent = true
	o.LoadDiffStats = true // Content implies stats
	return o
}

// WithoutDiffContent returns a copy of options with diff content loading disabled.
func (o LoadOptions) WithoutDiffContent() LoadOptions {
	o.LoadDiffContent = false
	return o
}

// WithTags returns a copy of options with tag loading enabled.
func (o LoadOptions) WithTags() LoadOptions {
	o.LoadTags = true
	return o
}

// WithoutTags returns a copy of options with tag loading disabled.
func (o LoadOptions) WithoutTags() LoadOptions {
	o.LoadTags = false
	return o
}
