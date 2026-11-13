package domain

import (
	"errors"
	"time"
)

// MaxSessionPRLinks caps how many PR links one session or backlog item may hold.
const MaxSessionPRLinks = 50

var (
	// ErrSessionPRLinkCap means a session or item already holds MaxSessionPRLinks links.
	ErrSessionPRLinkCap = errors.New("session: session already has the maximum number of PR links")
	// ErrNoSessionUUID means the session has no uuid, so no link row can reference it.
	ErrNoSessionUUID = errors.New("session: session has no uuid")
	// ErrSessionNotPersisted means the session row does not exist yet; the first
	// full save writes the PR set instead.
	ErrSessionNotPersisted = errors.New("session: session row not persisted yet")
)

// PRInitial is the state a caller actually observed when inserting a state row;
// the zero value inserts `unknown`, non-terminal (it observed nothing).
type PRInitial struct {
	State      PRState
	Terminal   bool
	URL        string
	Title      string
	HeadBranch string
	BaseBranch string
	IsDraft    bool
}

// StateOrUnknown returns State, or PRStateUnknown when the caller observed nothing.
func (i PRInitial) StateOrUnknown() PRState {
	if i.State == "" {
		return PRStateUnknown
	}
	return i.State
}

// PRLink describes the link UpsertSessionPR should create.
type PRLink struct {
	Role    PRRole // requested role; primary is downgraded to followup when the session already has one
	Source  AttachSource
	Initial PRInitial
	// AttachedAt, when set, is stored as the link's attached_at instead of now.
	AttachedAt time.Time
}

// PRLinkResult reports what UpsertSessionPR did.
type PRLinkResult struct {
	StateID    int
	Created    bool // a new link row was inserted
	Tombstoned bool // the owner detached this PR earlier; nothing was written
	Role       PRRole
}
