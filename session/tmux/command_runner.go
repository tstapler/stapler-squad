package tmux

import (
	"context"
	"io"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// CommandRunner is the execution seam between session/tmux (and session/git,
// which imports and reuses this type for its own mutating git/gh call sites)
// and the host a command actually runs on. Every tmux/git subprocess
// invocation that can be expressed as "run this and get combined output" or
// "start this and talk to it over stdin/stdout" goes through a CommandRunner
// instead of calling safeexec.CommandContext directly, so that a future
// SSH-backed implementation (Phase 2 of the ssh-remote-workspaces project)
// can be substituted at TmuxSession/GitWorktree construction time with no
// change to any downstream method's signature or behavior. See
// project_plans/ssh-remote-workspaces/decisions/ADR-002-commandrunner-in-session-tmux.md
// for why this interface lives here (the original consumer package) rather
// than in a new freestanding package.
//
// Run covers one-shot invocations (has-session, kill-session, list-sessions,
// git commit, gh pr view, etc.) where the caller wants buffered combined
// output. Start covers the one long-lived, piped case in this package: the
// tmux "-C" control-mode attach client, which needs stdin/stdout it can
// write to and read from incrementally rather than a single buffered
// result.
//
// Both take a dir parameter mirroring (*exec.Cmd).Dir: the working directory
// the command should run in, or "" for the caller's own current directory.
// This is required for session/git's worktree-scoped git/gh invocations
// (every git/gh call in worktree_git.go sets cmd.Dir to the worktree path)
// and is a no-op ("") for session/tmux's server-level commands, which have
// no meaningful working directory. dir and name are adjacent same-typed
// strings (see .claude/rules/primitive-obsession-checklist.md), but are left
// as plain positional parameters rather than wrapped in a newtype: unlike
// e.g. RepoRef's owner/repo (where a swap silently produces a different,
// still-valid repo), swapping dir and name here fails loudly and immediately
// at exec time (a directory path is never a valid program name and vice
// versa) rather than silently producing a plausible-but-wrong result — the
// specific harm newtypes in that checklist exist to prevent. See ADR-002's
// addendum for the full record of this decision.
//
// IsRemote is the single mechanism code holding only a CommandRunner value
// (e.g. tmux.go, worktree_git.go) uses to branch on remoteness. It exists
// for the rare call site whose behavior cannot be expressed through
// Run/Start alone because it depends on OS-process-specific semantics (PID,
// signal delivery, exec.Cmd.Process) that have no SSH analog — see
// ADR-002's "Alternatives Considered" for why that surface is deliberately
// kept out of this interface rather than leaked into it.
type CommandRunner interface {
	// Run executes name with args to completion in dir (or the caller's own
	// current directory if dir is "") and returns combined stdout+stderr
	// output, matching the shape of safeexec.CommandContext(ctx, name,
	// args...) with Dir set to dir, then .CombinedOutput().
	Run(ctx context.Context, dir, name string, args ...string) ([]byte, error)

	// Start begins a persistent, piped invocation of name with args in dir
	// (or the caller's own current directory if dir is "") and returns its
	// stdin/stdout plus a wait function that blocks until the process exits
	// (mirroring (*exec.Cmd).Wait()).
	Start(ctx context.Context, dir, name string, args ...string) (stdin io.WriteCloser, stdout io.ReadCloser, wait func() error, err error)

	// IsRemote reports whether commands run on a different host than this
	// process. LocalRunner always returns false.
	IsRemote() bool
}

// LocalRunner runs commands as local OS subprocesses via
// safeexec.CommandContext. It is the zero-behavior-change default for every
// TmuxSession and GitWorktree today: swapping a direct
// safeexec.CommandContext(...) call site for LocalRunner{} changes nothing
// observable, since it wraps the exact same stdlib calls used before this
// seam existed.
type LocalRunner struct{}

// Run implements CommandRunner by wrapping
// safeexec.CommandContext(ctx, name, args...).CombinedOutput(), setting
// cmd.Dir to dir first (a no-op when dir is "").
func (LocalRunner) Run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := safeexec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

// Start implements CommandRunner by wrapping safeexec.CommandContext(ctx,
// name, args...) with StdinPipe()/StdoutPipe() and Start(), returning a wait
// closure over Cmd.Wait(). cmd.Dir is set to dir first (a no-op when dir is "").
func (LocalRunner) Start(ctx context.Context, dir, name string, args ...string) (io.WriteCloser, io.ReadCloser, func() error, error) {
	cmd := safeexec.CommandContext(ctx, name, args...)
	cmd.Dir = dir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, nil, nil, err
	}
	return stdin, stdout, cmd.Wait, nil
}

// IsRemote implements CommandRunner. LocalRunner always runs on this host.
func (LocalRunner) IsRemote() bool { return false }
