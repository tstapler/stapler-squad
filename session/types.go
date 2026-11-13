package session

import (
	"errors"
	"fmt"
	"github.com/tstapler/stapler-squad/session/domain"
	"time"
	"unicode"
)

// GetManagedPermissions returns full permissions for squad-managed instances
func GetManagedPermissions() domain.InstancePermissions {
	return domain.InstancePermissions{
		CanView:              true,
		CanAttach:            true,
		CanSendCommand:       true,
		CanPause:             true,
		CanResume:            true,
		CanDestroy:           true,
		CanModifyGit:         true,
		CanAddToQueue:        true,
		RequiresConfirmation: make(map[string]bool),
	}
}

// GetExternalPermissions returns limited permissions for external instances
// allowAttach controls whether attach operations are permitted (power user mode)
func GetExternalPermissions(allowAttach bool) domain.InstancePermissions {
	perms := domain.InstancePermissions{
		CanView:              true,
		CanAttach:            allowAttach,
		CanSendCommand:       allowAttach,
		CanPause:             false,
		CanResume:            false,
		CanDestroy:           false,
		CanModifyGit:         false,
		CanAddToQueue:        false,
		RequiresConfirmation: make(map[string]bool),
	}

	// If attach is allowed, require confirmation
	if allowAttach {
		perms.RequiresConfirmation["attach"] = true
		perms.RequiresConfirmation["send"] = true
	}

	return perms
}

// GetMuxExternalPermissions returns permissions for mux-enabled external instances.
// Mux instances support full bidirectional terminal access and can be destroyed
// since they're explicitly opted-in by launching through ssq-mux with tmux session.
func GetMuxExternalPermissions() domain.InstancePermissions {
	return domain.InstancePermissions{
		CanView:        true,
		CanAttach:      true,
		CanSendCommand: true,
		CanPause:       false, // Can't pause external process
		CanResume:      false, // Can't resume external process
		CanDestroy:     true,  // Can kill tmux session (unified architecture)
		CanModifyGit:   false, // No git worktree management
		CanAddToQueue:  true,  // Can add to review queue for approvals
		RequiresConfirmation: map[string]bool{
			"destroy": true, // Require confirmation for killing external sessions
		},
	}
}

// DiscoveryMode controls what instances are discovered and how they can be interacted with
type DiscoveryMode int

const (
	// DiscoveryModeManaged discovers only squad-managed sessions (default, safest)
	DiscoveryModeManaged DiscoveryMode = iota

	// DiscoveryModeExtended discovers managed + external instances in read-only mode
	DiscoveryModeExtended

	// DiscoveryModeFull discovers all instances with attach capability (power user mode)
	DiscoveryModeFull
)

func (dm DiscoveryMode) String() string {
	switch dm {
	case DiscoveryModeManaged:
		return "managed"
	case DiscoveryModeExtended:
		return "extended"
	case DiscoveryModeFull:
		return "full"
	default:
		return "unknown"
	}
}

// PTYDiscoveryConfig controls PTY discovery scope and behavior
type PTYDiscoveryConfig struct {
	// Primary tmux server socket for squad-managed sessions
	// Empty string means use the default tmux server
	PrimarySocket string

	// ExternalSockets are additional tmux servers to scan for external instances
	// Only used when Mode is Extended or Full
	ExternalSockets []string

	// Mode controls discovery scope and permissions
	Mode DiscoveryMode

	// ManagedPrefix is the tmux session prefix for squad-managed sessions
	// Default: "staplersquad_"
	ManagedPrefix string

	// DiscoverExternal enables discovery of non-prefixed Claude instances
	// Automatically enabled for Extended and Full modes
	DiscoverExternal bool

	// AllowExternalAttach permits attaching to external instances
	// Only effective in Full mode
	AllowExternalAttach bool

	// RequireConfirmation requires user confirmation for external operations
	// Recommended to keep true for safety
	RequireConfirmation bool

	// DiscoveryInterval controls how often to refresh discovery
	DiscoveryInterval time.Duration

	// ParallelDiscovery enables parallel scanning of multiple tmux servers
	ParallelDiscovery bool
}

// DefaultPTYDiscoveryConfig returns the default discovery configuration
func DefaultPTYDiscoveryConfig() PTYDiscoveryConfig {
	return PTYDiscoveryConfig{
		PrimarySocket:       "",
		ExternalSockets:     []string{},
		Mode:                DiscoveryModeManaged,
		ManagedPrefix:       "staplersquad_",
		DiscoverExternal:    false,
		AllowExternalAttach: false,
		RequireConfirmation: true,
		DiscoveryInterval:   5 * time.Second,
		ParallelDiscovery:   true,
	}
}

// ShouldDiscoverExternal returns true if external instances should be discovered
func (c *PTYDiscoveryConfig) ShouldDiscoverExternal() bool {
	return c.DiscoverExternal || c.Mode >= DiscoveryModeExtended
}

// CanAttachExternal returns true if attaching to external instances is allowed
func (c *PTYDiscoveryConfig) CanAttachExternal() bool {
	return c.AllowExternalAttach && c.Mode == DiscoveryModeFull
}

// Title validation constants
const (
	MinTitleLength = 1
	MaxTitleLength = 32
)

// Title validation errors
var (
	ErrInvalidTitleLength = errors.New("title must be 1-32 characters")
	ErrInvalidTitleChars  = errors.New("title contains invalid characters")
	ErrDuplicateTitle     = errors.New("a session with this title already exists")
	ErrCannotRestart      = errors.New("session cannot be restarted in current state")
	ErrPauseNotPermitted  = errors.New("session does not permit pause")
	ErrResumeNotPermitted = errors.New("session does not permit resume")
	// ErrDirtyStateUnknown means the worktree's uncommitted-changes check failed, so
	// pause/stop kept the worktree rather than risk deleting uncommitted work.
	ErrDirtyStateUnknown = errors.New("cannot determine whether the worktree has uncommitted changes; keeping it to avoid data loss")
	// ErrWorktreeDirty means Destroy kept the worktree because it holds uncommitted
	// changes. Only ForceDestroy discards them.
	ErrWorktreeDirty = errors.New("worktree has uncommitted changes; keeping it (force to discard)")
)

// ErrDuplicateTag is returned when adding a tag that already exists.
type ErrDuplicateTag struct {
	Tag string
}

func (e ErrDuplicateTag) Error() string {
	return fmt.Sprintf("duplicate tag: %q", e.Tag)
}

// ErrTagTooLong is returned when a tag exceeds the maximum length.
type ErrTagTooLong struct {
	Tag    string
	MaxLen int
}

func (e ErrTagTooLong) Error() string {
	return fmt.Sprintf("tag %q exceeds maximum length %d", e.Tag, e.MaxLen)
}

// MaxTagLength is the maximum allowed length for a single tag.
const MaxTagLength = 50

// MaxTagCount is the maximum number of tags allowed per session.
const MaxTagCount = 100

// ErrTooManyTags is returned when setting more tags than MaxTagCount allows.
type ErrTooManyTags struct {
	Count    int
	MaxCount int
}

func (e ErrTooManyTags) Error() string {
	return fmt.Sprintf("too many tags: %d (maximum is %d)", e.Count, e.MaxCount)
}

// isValidTitle validates that a title contains only allowed characters:
// alphanumeric, spaces, dashes, and underscores
func isValidTitle(title string) bool {
	for _, r := range title {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != ' ' && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

// Workspace describes where a session is operating. It names the four path
// concepts the domain actually has; picking the wrong one has already shipped a
// bug. Why, and the full story: .claude/rules/instance-lock-free-reads.md.
//
// Which one you want:
//
//	comparing two sessions' locations  -> ActiveDir
//	reading or writing files           -> ExistingDir
//	"same repository?"                 -> RepoRoot, or Instance.WorkspaceKey()
//
// Use Instance.Workspace() to obtain this value; do not construct directly.
type Workspace struct {
	// RepoRoot is the path the session was created against, as recorded on
	// Instance.Path. For a session created from a pre-existing worktree this is
	// that worktree, not the main checkout -- Instance.MainRepoPath holds the
	// main checkout in that case.
	RepoRoot string

	// WorktreeDir is the dedicated git worktree created for this session, or ""
	// when the session works directly in RepoRoot.
	WorktreeDir string

	// ActiveDir is where this session works: WorktreeDir if it has one, else
	// RepoRoot. Deliberately does NOT check whether the directory still exists,
	// so it stays stable across a worktree cleanup -- which makes it the correct
	// key for comparing, correlating, or identifying sessions, and the right
	// default when you are unsure.
	ActiveDir string

	// ExistingDir is ActiveDir when that directory exists on disk, else
	// RepoRoot. Use it only to open files: pause_session deletes a worktree
	// while keeping its branch, and without the fallback filesystem callers
	// surface a bare "directory not found: .".
	//
	// Lossy by construction -- two sessions whose worktrees were both cleaned up
	// share an ExistingDir without being in the same place, so never compare it.
	ExistingDir string
}

// RestartState holds the state needed to restart a session
type RestartState struct {
	// Working directory to restore
	WorkingDir string
	// Claude session ID for --resume flag
	ClaudeSessionID string
	// Environment variables to restore
	Environment map[string]string
	// Original command/program
	Program string
	// AutoYes flag
	AutoYes bool
	// Original prompt
	Prompt string
}
