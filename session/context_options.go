package session

import (
	"fmt"
	"strings"
)

// ContextOptions specifies which optional contexts to load when querying sessions.
// This enables optimized queries that only load the data needed for each use case.
type ContextOptions struct {
	// Context loading flags
	LoadGit        bool // Git repository context (branch, commit, remotes)
	LoadFilesystem bool // Filesystem context (directory state, file counts)
	LoadTerminal   bool // Terminal context (output, command history)
	LoadUI         bool // UI context (position, focus state, expanded/collapsed)
	LoadActivity   bool // Activity context (last active, duration, events)
	LoadCloud      bool // Cloud context (API sessions, remote state)

	// Child data loading flags (from existing LoadOptions)
	LoadWorktree      bool // Git worktree data
	LoadDiffStats     bool // Diff statistics (added/removed counts)
	LoadDiffContent   bool // Full diff content (heavy - only load when needed)
	LoadTags          bool // Session tags
	LoadClaudeSession bool // Claude Code session data
}

// Common preset configurations for different use cases
// These use the "Context" prefix to distinguish from the legacy LoadOptions presets

// ContextMinimal loads only core session data with no contexts.
// Use this for basic operations that only need session metadata.
// Memory usage: ~500 bytes per session
var ContextMinimal = ContextOptions{}

// ContextUIView loads contexts needed for list/card display.
// Optimized for responsive UI rendering with essential context only.
// Memory usage: ~2-3 KB per session
var ContextUIView = ContextOptions{
	LoadUI:       true,
	LoadActivity: true,
	LoadGit:      true, // For branch display
	LoadTags:     true,
}

// ContextTerminalView loads contexts needed for terminal preview.
// Includes terminal output and git diffs for preview panes.
// Memory usage: ~5-10 KB per session (varies with terminal output size)
var ContextTerminalView = ContextOptions{
	LoadTerminal:  true,
	LoadGit:       true,
	LoadUI:        true,
	LoadActivity:  true,
	LoadDiffStats: true,
}

// ContextDetailView loads most contexts for detail panel.
// Comprehensive data for session detail views, excluding heavy diff content.
// Memory usage: ~10-20 KB per session
var ContextDetailView = ContextOptions{
	LoadGit:           true,
	LoadFilesystem:    true,
	LoadTerminal:      true,
	LoadUI:            true,
	LoadActivity:      true,
	LoadWorktree:      true,
	LoadDiffStats:     true,
	LoadTags:          true,
	LoadClaudeSession: true,
}

// ContextFull loads all contexts and child data (expensive).
// Complete data including full diff content. Use sparingly.
// Memory usage: Can be 1-25 MB per session depending on diff size
var ContextFull = ContextOptions{
	LoadGit:           true,
	LoadFilesystem:    true,
	LoadTerminal:      true,
	LoadUI:            true,
	LoadActivity:      true,
	LoadCloud:         true,
	LoadWorktree:      true,
	LoadDiffStats:     true,
	LoadDiffContent:   true,
	LoadTags:          true,
	LoadClaudeSession: true,
}

// ContextCloudSession loads contexts for cloud/API sessions.
// Optimized for remote sessions that don't have local git/filesystem context.
// Memory usage: ~1-2 KB per session
var ContextCloudSession = ContextOptions{
	LoadCloud:    true,
	LoadActivity: true,
	LoadUI:       true,
	LoadTags:     true,
}

// ContextForSearch loads contexts needed for search operations.
// Includes tags and basic metadata for efficient filtering.
// Memory usage: ~1-2 KB per session
var ContextForSearch = ContextOptions{
	LoadGit:      true, // For branch searching
	LoadTags:     true, // For tag filtering
	LoadActivity: true, // For sorting by recency
}

// ContextForReviewQueue loads data needed for review queue operations.
// Focused on git context and change indicators.
// Memory usage: ~3-5 KB per session
var ContextForReviewQueue = ContextOptions{
	LoadGit:       true, // For branch and commit info
	LoadActivity:  true, // For queue prioritization
	LoadWorktree:  true, // For diff context
	LoadDiffStats: true, // For change indicators
	LoadTags:      true, // For filtering
}

// AnyContextLoaded returns true if any context is configured to load.
func (o ContextOptions) AnyContextLoaded() bool {
	return o.LoadGit || o.LoadFilesystem || o.LoadTerminal ||
		o.LoadUI || o.LoadActivity || o.LoadCloud
}

// AnyChildDataLoaded returns true if any child data is configured to load.
func (o ContextOptions) AnyChildDataLoaded() bool {
	return o.LoadWorktree || o.LoadDiffStats || o.LoadDiffContent ||
		o.LoadTags || o.LoadClaudeSession
}

// Merge combines two ContextOptions, returning options that load the union of both.
// This is useful for combining requirements from multiple components.
func (o ContextOptions) Merge(other ContextOptions) ContextOptions {
	return ContextOptions{
		// Contexts
		LoadGit:        o.LoadGit || other.LoadGit,
		LoadFilesystem: o.LoadFilesystem || other.LoadFilesystem,
		LoadTerminal:   o.LoadTerminal || other.LoadTerminal,
		LoadUI:         o.LoadUI || other.LoadUI,
		LoadActivity:   o.LoadActivity || other.LoadActivity,
		LoadCloud:      o.LoadCloud || other.LoadCloud,
		// Child data
		LoadWorktree:      o.LoadWorktree || other.LoadWorktree,
		LoadDiffStats:     o.LoadDiffStats || other.LoadDiffStats,
		LoadDiffContent:   o.LoadDiffContent || other.LoadDiffContent,
		LoadTags:          o.LoadTags || other.LoadTags,
		LoadClaudeSession: o.LoadClaudeSession || other.LoadClaudeSession,
	}
}

// String returns a human-readable description of what will be loaded.
func (o ContextOptions) String() string {
	var parts []string

	// Contexts
	if o.LoadGit {
		parts = append(parts, "git")
	}
	if o.LoadFilesystem {
		parts = append(parts, "filesystem")
	}
	if o.LoadTerminal {
		parts = append(parts, "terminal")
	}
	if o.LoadUI {
		parts = append(parts, "ui")
	}
	if o.LoadActivity {
		parts = append(parts, "activity")
	}
	if o.LoadCloud {
		parts = append(parts, "cloud")
	}

	// Child data
	if o.LoadWorktree {
		parts = append(parts, "worktree")
	}
	if o.LoadDiffContent {
		parts = append(parts, "diff-content")
	} else if o.LoadDiffStats {
		parts = append(parts, "diff-stats")
	}
	if o.LoadTags {
		parts = append(parts, "tags")
	}
	if o.LoadClaudeSession {
		parts = append(parts, "claude-session")
	}

	if len(parts) == 0 {
		return "ContextOptions{minimal}"
	}

	return fmt.Sprintf("ContextOptions{%s}", strings.Join(parts, ", "))
}

// ToLoadOptions converts ContextOptions to the legacy LoadOptions type.
// This provides backward compatibility with existing code.
func (o ContextOptions) ToLoadOptions() LoadOptions {
	return LoadOptions{
		LoadWorktree:      o.LoadWorktree,
		LoadDiffStats:     o.LoadDiffStats,
		LoadDiffContent:   o.LoadDiffContent,
		LoadTags:          o.LoadTags,
		LoadClaudeSession: o.LoadClaudeSession,
	}
}

// FromLoadOptions creates ContextOptions from the legacy LoadOptions type.
// This provides backward compatibility when migrating existing code.
func FromLoadOptions(lo LoadOptions) ContextOptions {
	return ContextOptions{
		LoadWorktree:      lo.LoadWorktree,
		LoadDiffStats:     lo.LoadDiffStats,
		LoadDiffContent:   lo.LoadDiffContent,
		LoadTags:          lo.LoadTags,
		LoadClaudeSession: lo.LoadClaudeSession,
	}
}

// Builder methods for fluent configuration

// WithGit returns a copy of options with git context loading enabled.
func (o ContextOptions) WithGit() ContextOptions {
	o.LoadGit = true
	return o
}

// WithFilesystem returns a copy of options with filesystem context loading enabled.
func (o ContextOptions) WithFilesystem() ContextOptions {
	o.LoadFilesystem = true
	return o
}

// WithTerminal returns a copy of options with terminal context loading enabled.
func (o ContextOptions) WithTerminal() ContextOptions {
	o.LoadTerminal = true
	return o
}

// WithUI returns a copy of options with UI context loading enabled.
func (o ContextOptions) WithUI() ContextOptions {
	o.LoadUI = true
	return o
}

// WithActivity returns a copy of options with activity context loading enabled.
func (o ContextOptions) WithActivity() ContextOptions {
	o.LoadActivity = true
	return o
}

// WithCloud returns a copy of options with cloud context loading enabled.
func (o ContextOptions) WithCloud() ContextOptions {
	o.LoadCloud = true
	return o
}

// WithDiffContent returns a copy of options with diff content loading enabled.
func (o ContextOptions) WithDiffContent() ContextOptions {
	o.LoadDiffContent = true
	o.LoadDiffStats = true // Content implies stats
	return o
}

// WithoutDiffContent returns a copy of options with diff content loading disabled.
func (o ContextOptions) WithoutDiffContent() ContextOptions {
	o.LoadDiffContent = false
	return o
}

// WithTags returns a copy of options with tag loading enabled.
func (o ContextOptions) WithTags() ContextOptions {
	o.LoadTags = true
	return o
}

// WithoutTags returns a copy of options with tag loading disabled.
func (o ContextOptions) WithoutTags() ContextOptions {
	o.LoadTags = false
	return o
}
