package session

// execution_target.go defines the ExecutionTarget sum type (ssh-remote-workspaces
// Phase 4, Epic 4.2, Task 4.2.1a): where a session's TmuxSession/GitWorktree
// subprocess commands actually run -- the local machine, or a configured SSH
// remote. Split into its own file rather than added to instance.go (already
// 1800+ lines) for the same reason instance_tmux.go/instance_worktree.go
// already exist as separate files in this package.

import (
	"github.com/tstapler/stapler-squad/session/tmux"
)

// ExecutionTarget selects the tmux.CommandRunner a session's TmuxSession and
// GitWorktree are constructed with. It exists so "a RemoteTarget is
// configured" and "a remote CommandRunner is installed" can never disagree --
// architecture-review.md Blocker 1 (see
// project_plans/ssh-remote-workspaces/decisions/ADR-002-commandrunner-in-session-tmux.md's
// discussion of that blocker) flagged the alternative of two independently-
// settable fields (a *RemoteTarget pointer plus whichever CommandRunner
// happened to be installed) as an unrepresentable-state hazard: nothing
// enforced that the two agreed. Instance and InstanceOptions each hold
// exactly one ExecutionTarget field, defaulting to LocalTarget{}.
//
// Deliberately a small closed interface (IsRemote/Runner) rather than a
// type-switch on a concrete type -- IsRemote() is the single mechanism this
// codebase uses to branch on remoteness (see tmux.CommandRunner's own
// IsRemote method, which every ExecutionTarget.Runner() ultimately returns),
// matching the convention Phase 1's CommandRunner work already established.
type ExecutionTarget interface {
	// IsRemote reports whether this target's Runner() executes commands on a
	// different host than this process.
	IsRemote() bool
	// Runner returns the tmux.CommandRunner every TmuxSession/GitWorktree
	// constructed for this target should use (via tmux.WithCommandRunner /
	// git.WithCommandRunner).
	Runner() tmux.CommandRunner

	// executionTarget is unexported so ExecutionTarget can only be
	// implemented by the two types in this file (a closed sum type) --
	// mirrors this codebase's other sealed-interface sum types (see
	// SessionType's const-based enum for the non-interface equivalent).
	executionTarget()
}

// LocalTarget is the default ExecutionTarget. Every TmuxSession/GitWorktree
// built from it uses tmux.LocalRunner{} -- byte-for-byte the same runner
// every construction site used before ExecutionTarget existed, so local
// sessions are unaffected by this type's introduction.
type LocalTarget struct{}

// IsRemote implements ExecutionTarget. LocalTarget always runs commands on
// this host.
func (LocalTarget) IsRemote() bool { return false }

// Runner implements ExecutionTarget, always returning tmux.LocalRunner{}.
func (LocalTarget) Runner() tmux.CommandRunner { return tmux.LocalRunner{} }

func (LocalTarget) executionTarget() {}

// RemoteTarget is the plain data describing a resolved SSH remote: its
// saved name, connection coordinates, and the base path new worktrees are
// created under. It carries no live connection and no credential material --
// see config.RemoteConfig (the on-disk source this is resolved from by
// server/services/session_service.go's resolveRemoteTarget) for
// IdentityRef's own "opaque, non-secret pointer" doc comment, which applies
// equally here.
type RemoteTarget struct {
	// Name is the RemoteConfig.Name this target was resolved from.
	Name string
	// Host is the SSH-reachable hostname or address (see
	// config.RemoteConfig.Host's doc comment for its exact shape).
	Host string
	// User is the SSH login username on the remote host.
	User string
	// BasePath is the absolute path on the remote host under which new
	// session worktrees are created (see config.RemoteConfig.BasePath).
	BasePath string
	// IdentityRef is an opaque, non-secret pointer to this remote's stored
	// SSH identity, resolved via sshremote.KeyStore at dial time. Empty
	// means no identity has been registered for this remote.
	IdentityRef string
}

// RemoteExecutionTarget pairs a resolved RemoteTarget with the *tmux.SSHRunner
// already dialed to it. Fields are unexported and only settable via
// NewRemoteExecutionTarget so this value can never exist half-built (a
// Target with no matching Runner, or vice versa) -- the single-step
// atomicity architecture-review.md Blocker 1 calls for. This is a
// deliberate, Go-syntax-forced deviation from plan.md's pseudocode (which
// showed RemoteExecutionTarget{Target, Runner} as a struct literal with an
// exported `Runner` field): a field and a method cannot share the name
// `Runner` on the same type, and an exported-field struct literal would in
// any case permit exactly the partial construction (RemoteExecutionTarget{Target: t})
// this type exists to rule out.
type RemoteExecutionTarget struct {
	target RemoteTarget
	runner *tmux.SSHRunner
}

// NewRemoteExecutionTarget constructs a RemoteExecutionTarget from a resolved
// target and its already-dialed runner in one atomic step. runner must not be
// nil -- enforced here, not just documented, since a nil runner would let
// IsRemote() report true while Runner() silently returns a nil
// tmux.CommandRunner, exactly the half-built state this type exists to make
// unrepresentable.
func NewRemoteExecutionTarget(target RemoteTarget, runner *tmux.SSHRunner) RemoteExecutionTarget {
	if runner == nil {
		panic("session: NewRemoteExecutionTarget called with a nil runner")
	}
	return RemoteExecutionTarget{target: target, runner: runner}
}

// IsRemote implements ExecutionTarget. Always true.
func (t RemoteExecutionTarget) IsRemote() bool { return true }

// Runner implements ExecutionTarget, returning the dialed *tmux.SSHRunner
// (which itself implements tmux.CommandRunner).
func (t RemoteExecutionTarget) Runner() tmux.CommandRunner { return t.runner }

// Target returns the resolved RemoteTarget this execution target was built
// from (name/host/base path -- see RemoteTarget's doc comment).
func (t RemoteExecutionTarget) Target() RemoteTarget { return t.target }

func (RemoteExecutionTarget) executionTarget() {}

// compile-time checks that both types satisfy ExecutionTarget.
var (
	_ ExecutionTarget = LocalTarget{}
	_ ExecutionTarget = RemoteExecutionTarget{}
)
