package session

// interfaces.go defines narrow interfaces at the server/session boundary.
// Consumers of *Instance in the server layer should depend on these interfaces
// rather than the concrete type wherever the full surface is not required.

import (
	"time"

	"github.com/tstapler/stapler-squad/session/git"
)

// InstanceReader exposes a minimal read-only view of an Instance for server-layer
// code that only needs to observe session state. It is not yet used at every call
// site (some helpers still take *Instance directly for field access); adopt it
// incrementally as call sites are converted to use getter methods.
//
// *Instance satisfies this interface automatically. Use it to supply lightweight
// test doubles without starting a real tmux session.
type InstanceReader interface {
	// Identity
	GetTitle() string
	GetStableID() string

	// Descriptive metadata
	GetWorkingDirectory() string

	// GetStatus returns the current lifecycle status as int.
	// Deprecated: use GetLifecycleStatus() or the typed predicates below.
	GetStatus() int

	// GetLifecycleStatus returns the current lifecycle status as a typed Status value.
	GetLifecycleStatus() Status

	// Typed state predicates — prefer these over comparing GetStatus() against constants.
	IsActive() bool
	IsPaused() bool
	IsHibernated() bool
	IsStopped() bool

	// Git / diff
	GetDiffStats() *git.DiffStats

	// Activity timestamps
	GetTimeSinceLastMeaningfulOutput() time.Duration
}
