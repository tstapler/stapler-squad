package session

import (
	"time"

	"github.com/google/uuid"
)

// newCheckpointID generates a new unique checkpoint ID.
func newCheckpointID() string {
	return uuid.New().String()
}

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
