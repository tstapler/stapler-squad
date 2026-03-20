// Package workspace provides workspace tracking and status management for stapler-squad sessions.
// It supports multi-pod deployments via distributed locking and cache invalidation.
package workspace

import (
	"time"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/vc"
)

// TrackedWorkspace represents a directory tracked by stapler-squad
type TrackedWorkspace struct {
	// Identity
	Path           string // Absolute path to workspace
	RepositoryRoot string // Root of the git/jj repository
	WorktreePath   string // Path if this is a worktree (empty if not)
	MainRepoPath   string // Main repo path if this is a worktree

	// Session association
	SessionTitle  string         // Title of associated session (empty if orphaned)
	SessionStatus session.Status // Status of associated session
	IsOrphaned    bool           // True if no active session owns this workspace

	// VCS information
	VCSType vc.VCSType // Git or Jujutsu

	// Timestamps
	LastChecked  time.Time // When VCS status was last refreshed
	LastActivity time.Time // Last file modification detected

	// Attention flags
	NeedsAttention  bool   // Has issues requiring action
	AttentionReason string // Why attention is needed
}

// WorkspaceStatus combines VCS status with workspace-specific information
type WorkspaceStatus struct {
	// Embedded VCS status
	VCSStatus *vc.VCSStatus

	// Workspace context
	WorkspacePath string         // Path to this workspace
	SessionTitle  string         // Associated session (if any)
	SessionStatus session.Status // Session state
	IsOrphaned    bool           // No active session
	IsWorktree    bool           // Is a git worktree

	// Activity tracking
	LastActivity time.Time // Last file modification
	LastChecked  time.Time // When status was collected

	// Attention flags
	NeedsAttention  bool   // Has issues requiring action
	AttentionReason string // Why attention is needed

	// Error state
	Error     error  // Error during status collection (nil if success)
	ErrorMsg  string // Human-readable error message
	IsPartial bool   // True if status is incomplete due to errors
}

// ChangesSummary provides aggregated statistics across all workspaces
type ChangesSummary struct {
	TotalRepositories  int // Distinct repository roots
	TotalWorkspaces    int // All tracked workspaces
	TotalUncommitted   int // Files with uncommitted changes
	TotalUntracked     int // Untracked files
	TotalStaged        int // Staged files
	TotalConflicts     int // Conflicted files
	WorkspacesWithWork int // Workspaces with any pending changes
	OrphanedWorkspaces int // Workspaces without active sessions
}

// WorkspaceFilter defines criteria for filtering workspace queries
type WorkspaceFilter struct {
	// Path filters
	RepositoryRoot string // Filter by repository root
	PathPrefix     string // Filter by path prefix

	// Status filters
	IncludeOrphaned  bool // Include orphaned workspaces
	OnlyWithChanges  bool // Only workspaces with uncommitted changes
	OnlyWithConflict bool // Only workspaces with conflicts

	// Session filters
	SessionStatus *session.Status // Filter by session status
}

// StatusRefreshOptions controls how status refresh behaves
type StatusRefreshOptions struct {
	// Force refresh even if cache is fresh
	Force bool

	// Maximum age for cached status to be considered fresh
	MaxAge time.Duration

	// Timeout for individual git operations
	Timeout time.Duration

	// Continue on errors (return partial results)
	ContinueOnError bool
}

// DefaultStatusRefreshOptions returns sensible defaults
func DefaultStatusRefreshOptions() StatusRefreshOptions {
	return StatusRefreshOptions{
		Force:           false,
		MaxAge:          30 * time.Second,
		Timeout:         5 * time.Second,
		ContinueOnError: true,
	}
}
