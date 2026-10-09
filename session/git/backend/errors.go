package backend

import (
	"errors"
	"fmt"
)

// Sentinel errors every Backend implementation maps its failures onto, so callers (and the
// Router's fallback logic) branch with errors.Is instead of parsing git text.
var (
	// ErrUnborn means HEAD names a branch that has no commit yet (git exit 128 on a fresh repo).
	ErrUnborn = errors.New("git backend: HEAD is unborn (no commits yet)")
	// ErrDetachedHead means HEAD is not a symbolic ref, so there is no current branch.
	ErrDetachedHead = errors.New("git backend: HEAD is detached")
	// ErrRefNotFound means the named ref does not resolve in this repository.
	ErrRefNotFound = errors.New("git backend: ref not found")
	// ErrObjectNotFound means a SHA is well-formed but its object is missing from the object database.
	ErrObjectNotFound = errors.New("git backend: object not found")
	// ErrNoMergeBase means the two revisions share no common ancestor.
	ErrNoMergeBase = errors.New("git backend: no merge base")
	// ErrNotARepo means the location is not inside a git repository.
	ErrNotARepo = errors.New("git backend: not a git repository")
	// ErrConfigUnset means the requested config key has no value.
	ErrConfigUnset = errors.New("git backend: config key not set")
	// ErrNoRemoteRunner means a Remote location carried a nil Runner. It is never recovered by
	// running the command locally.
	ErrNoRemoteRunner = errors.New("git backend: remote location has no runner")
	// ErrNoLocalRunner means a Local location was used on a backend built without a local Runner.
	ErrNoLocalRunner = errors.New("git backend: backend has no local runner")
	// ErrLocked means another process holds a git lock the operation needs.
	ErrLocked = errors.New("git backend: repository is locked")
	// ErrInvalidArgument means a typed argument could not be passed to git safely (empty, or
	// starting with "-" where git would parse it as an option).
	ErrInvalidArgument = errors.New("git backend: invalid argument")
)

// CommandError is the failure of one git invocation: which operation, the (credential-scrubbed)
// output git printed, and the underlying runner error. It unwraps to Err, so exit-status checks
// on the runner's error type keep working.
type CommandError struct {
	Operation OperationName
	Output    string
	Err       error
}

func (e *CommandError) Error() string {
	if e.Output == "" {
		return fmt.Sprintf("git %s failed: %v", e.Operation, e.Err)
	}
	return fmt.Sprintf("git %s failed: %s (%v)", e.Operation, e.Output, e.Err)
}

func (e *CommandError) Unwrap() error { return e.Err }
