package backend

import (
	"errors"
	"fmt"
	"time"
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
	// ErrNothingToCommit means Commit found an empty index (or, with Amend, nothing to amend).
	ErrNothingToCommit = errors.New("git backend: nothing to commit")
	// ErrInvalidArgument means a typed argument could not be passed to git safely (empty, or
	// starting with "-" where git would parse it as an option).
	ErrInvalidArgument = errors.New("git backend: invalid argument")
)

// ErrLocked means another process holds a git lock file the operation needs. Match it with
// errors.Is(err, backend.ErrLocked{}) and read the details with errors.As. Age and Journaled
// are filled by the in-process lock layer (plan Story 2.3.1); the CLI backend only knows Path.
type ErrLocked struct {
	Path      string        // the lock file, e.g. <git dir>/index.lock; empty when git did not name it
	Age       time.Duration // how long the lock has existed; zero when unknown
	Journaled bool          // true when the lock journal says this server created it
}

func (e ErrLocked) Error() string {
	if e.Path == "" {
		return "git backend: repository is locked"
	}
	return fmt.Sprintf("git backend: repository is locked: %s", e.Path)
}

// Is makes errors.Is(err, ErrLocked{}) true for any ErrLocked, whatever its fields.
func (ErrLocked) Is(target error) bool {
	_, ok := target.(ErrLocked)
	return ok
}

// ErrFallbackEligible is returned by a non-CLI backend for a failure the Router may retry on
// the CLI. Wrote reports whether the failed call already changed a ref, the index or config:
// when true the Router must not replay the operation (plan Story 1.1.3). It lives here so the
// shape is fixed before any such backend exists.
type ErrFallbackEligible struct {
	Wrote bool
	Err   error
}

func (e ErrFallbackEligible) Error() string {
	return fmt.Sprintf("git backend: fallback eligible (wrote=%t): %v", e.Wrote, e.Err)
}

func (e ErrFallbackEligible) Unwrap() error { return e.Err }

// Is makes errors.Is(err, ErrFallbackEligible{}) true for any ErrFallbackEligible.
func (ErrFallbackEligible) Is(target error) bool {
	_, ok := target.(ErrFallbackEligible)
	return ok
}

// ErrNoisyOutput means output that cannot be parsed reliably because a combined-output Runner
// (no StdoutRunner) may have mixed stderr into NUL-delimited data. Returned instead of data
// that could silently be wrong; use a Runner that implements StdoutRunner.
var ErrNoisyOutput = errors.New("git backend: output may contain stderr noise; use a StdoutRunner")

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
