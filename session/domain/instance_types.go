package domain

import (
	"fmt"
	"time"
)

// Checkpoint represents a named bookmark of a session's state at a point in time.
// It captures the scrollback position, git SHA, and conversation UUID so that
// the session can later be forked or restored from this exact state.
type Checkpoint struct {
	ID             string `json:"id"`
	SessionID      string `json:"session_id"`
	ParentID       string `json:"parent_id,omitempty"`
	Label          string `json:"label"`
	ScrollbackSeq  uint64 `json:"scrollback_seq"`
	ScrollbackPath string `json:"scrollback_path,omitempty"`
	ClaudeConvUUID string `json:"claude_conv_uuid,omitempty"`
	// ConvLineCount is the number of JSONL lines in the Claude conversation file at
	// checkpoint time. Used by ForkClaudeConversation to truncate the fork correctly.
	ConvLineCount uint64    `json:"conv_line_count,omitempty"`
	GitCommitSHA  string    `json:"git_commit_sha,omitempty"`
	Timestamp     time.Time `json:"timestamp"`

	// New: CLI-agnostic checkpoint details.
	CanonicalTurnIndex int    `json:"canonical_turn_index,omitempty"`
	CanonicalPath      string `json:"canonical_path,omitempty"`
}

// CheckpointList is a slice of Checkpoints with helper methods.
type CheckpointList []Checkpoint

// FindByID returns the Checkpoint with the given ID, or nil if not found.
func (cl CheckpointList) FindByID(id string) *Checkpoint {
	for i := range cl {
		if cl[i].ID == id {
			return &cl[i]
		}
	}
	return nil
}

// FindByLabel returns the first Checkpoint with the given label, or nil if not found.
func (cl CheckpointList) FindByLabel(label string) *Checkpoint {
	for i := range cl {
		if cl[i].Label == label {
			return &cl[i]
		}
	}
	return nil
}

// Latest returns the Checkpoint with the most recent Timestamp, or nil if empty.
func (cl CheckpointList) Latest() *Checkpoint {
	if len(cl) == 0 {
		return nil
	}
	latest := &cl[0]
	for i := 1; i < len(cl); i++ {
		if cl[i].Timestamp.After(latest.Timestamp) {
			latest = &cl[i]
		}
	}
	return latest
}

// ReviveOutcome records the outcome of the most recent start/cold-restore
// decision (session-revive-uuid-loss AC3). See Instance.LastReviveOutcome.
type ReviveOutcome string

const (
	ReviveOutcomeUnspecified ReviveOutcome = ""
	// ReviveOutcomeResumeLive means a conversation UUID was already in memory;
	// no disk-based recovery was needed.
	ReviveOutcomeResumeLive ReviveOutcome = "resume_live"
	// ReviveOutcomeResumeRecovered means no UUID was in memory, but
	// tryExtractConversationUUID's disk-based fallback found one before launch.
	ReviveOutcomeResumeRecovered ReviveOutcome = "resume_recovered"
	// ReviveOutcomeFreshExpected means the session started fresh with no prior
	// conversation history to lose (first-time setup or genuinely no history).
	ReviveOutcomeFreshExpected ReviveOutcome = "fresh_expected"
	// ReviveOutcomeFreshLostHistory means the session started fresh even though
	// EverHadConversationHistory was true — recovery ran and found nothing.
	ReviveOutcomeFreshLostHistory ReviveOutcome = "fresh_lost_history"
)

// TurnEndSource says which path observed the end of an agent turn.
type TurnEndSource string

const (
	TurnEndSourceHook   TurnEndSource = "hook"
	TurnEndSourceScreen TurnEndSource = "screen"
)

// TurnEnded is the minimal turn-end fact; Episode/Kind arrive with Story 3.1 constructors.
type TurnEnded struct {
	Excerpt string
	Source  TurnEndSource
	At      time.Time
}

// InstanceType represents the type of session instance
type InstanceType int

const (
	// InstanceTypeManaged represents a session fully managed by stapler-squad
	// with complete lifecycle control, git worktrees, and all features
	InstanceTypeManaged InstanceType = iota

	// InstanceTypeExternal represents a Claude instance discovered externally
	// (not created by stapler-squad) with limited interaction capabilities
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

	// MuxSocketPath is the path to an ssq-mux Unix domain socket
	// If set, this instance was discovered via ssq-mux and supports
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

// ErrInvalidTransition is returned when a status transition is not allowed
// by the state machine defined in state_machine.go.
type ErrInvalidTransition struct {
	From Status
	To   Status
}

func (e ErrInvalidTransition) Error() string {
	return fmt.Sprintf("invalid transition: %s -> %s", e.From, e.To)
}
