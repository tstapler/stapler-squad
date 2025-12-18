package session

import (
	"errors"
	"time"
	"unicode"
)

// InstanceType represents the type of session instance
type InstanceType int

const (
	// InstanceTypeManaged represents a session fully managed by claude-squad
	// with complete lifecycle control, git worktrees, and all features
	InstanceTypeManaged InstanceType = iota

	// InstanceTypeExternal represents a Claude instance discovered externally
	// (not created by claude-squad) with limited interaction capabilities
	InstanceTypeExternal
)

func (it InstanceType) String() string {
	switch it {
	case InstanceTypeManaged:
		return "Managed"
	case InstanceTypeExternal:
		return "External"
	default:
		return "Unknown"
	}
}

// ExternalInstanceMetadata contains metadata for externally discovered Claude instances
type ExternalInstanceMetadata struct {
	// TmuxSocket is the tmux server socket this instance belongs to
	// Empty string means the default tmux server
	TmuxSocket string

	// TmuxSessionName is the full tmux session name
	TmuxSessionName string

	// DiscoveredAt is when this external instance was first discovered
	DiscoveredAt time.Time

	// LastSeen is when this instance was last seen during discovery
	LastSeen time.Time

	// OriginalPID is the process ID when first discovered
	OriginalPID int

	// MuxSocketPath is the path to a claude-mux Unix domain socket
	// If set, this instance was discovered via claude-mux and supports
	// full bidirectional terminal access
	MuxSocketPath string

	// MuxEnabled indicates whether this instance supports mux protocol
	MuxEnabled bool

	// SourceTerminal identifies the source (e.g., "IntelliJ", "Terminal", "tmux")
	SourceTerminal string
}

// InstancePermissions defines what operations are allowed on an instance
type InstancePermissions struct {
	// View operations
	CanView bool

	// Attach to the terminal session
	CanAttach bool

	// Send commands to the terminal
	CanSendCommand bool

	// Pause the session (stop tmux, keep worktree)
	CanPause bool

	// Resume a paused session
	CanResume bool

	// Destroy the session completely
	CanDestroy bool

	// Perform git operations (commit, push, worktree management)
	CanModifyGit bool

	// Add to review queue
	CanAddToQueue bool

	// RequiresConfirmation maps operation names to whether they need confirmation
	// Used for high-risk operations on external instances
	RequiresConfirmation map[string]bool
}

// GetManagedPermissions returns full permissions for squad-managed instances
func GetManagedPermissions() InstancePermissions {
	return InstancePermissions{
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
func GetExternalPermissions(allowAttach bool) InstancePermissions {
	perms := InstancePermissions{
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
// since they're explicitly opted-in by launching through claude-mux with tmux session.
func GetMuxExternalPermissions() InstancePermissions {
	return InstancePermissions{
		CanView:              true,
		CanAttach:            true,
		CanSendCommand:       true,
		CanPause:             false, // Can't pause external process
		CanResume:            false, // Can't resume external process
		CanDestroy:           true,  // Can kill tmux session (unified architecture)
		CanModifyGit:         false, // No git worktree management
		CanAddToQueue:        true,  // Can add to review queue for approvals
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

// ParseDiscoveryMode parses a string into a DiscoveryMode
func ParseDiscoveryMode(s string) DiscoveryMode {
	switch s {
	case "managed":
		return DiscoveryModeManaged
	case "extended":
		return DiscoveryModeExtended
	case "full":
		return DiscoveryModeFull
	default:
		return DiscoveryModeManaged // Safe default
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
	// Default: "claudesquad_"
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
		ManagedPrefix:       "claudesquad_",
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
)

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
