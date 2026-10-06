package git

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/tstapler/stapler-squad/session/tmux"
)

// ErrRemoteBasePathMissing is returned by RemoteWorktreeOps.CreateWorktree when the
// remote directory a new worktree would be created under does not exist. Distinct
// from a connection/transport failure (an unreachable host, a dead SSH channel) so
// callers can tell "host is fine, path is wrong" apart from "host is unreachable" —
// see Task 2.2.1a and adversarial-review.md Blocker 3's TOCTOU note (this check is
// advisory only; the real error surfaces from `git worktree add` itself if the
// directory is removed/unmounted between this check and the add).
var ErrRemoteBasePathMissing = errors.New("remote worktree base path does not exist")

// RemoteWorktreeOps performs git worktree creation, removal, and new-project
// initialization on a remote host, executed entirely through an injected
// tmux.CommandRunner (session/tmux, reused here per ADR-002 — see
// session/git/worktree.go's runner field doc comment for the same pattern
// applied to GitWorktree). It mirrors ops.go's free-function shape (explicit
// path/branch parameters, no persisted per-worktree state) rather than
// GitWorktree's stateful builder shape, since — unlike GitWorktree, which
// owns exactly one worktree's lifecycle — a single RemoteWorktreeOps is
// meant to be reused across many worktrees on the same remote CommandRunner.
type RemoteWorktreeOps struct {
	runner tmux.CommandRunner
}

// NewRemoteWorktreeOps constructs a RemoteWorktreeOps that runs every command
// through runner (a *tmux.SSHRunner in production, tmux.LocalRunner{} or a
// test spy in tests).
func NewRemoteWorktreeOps(runner tmux.CommandRunner) *RemoteWorktreeOps {
	return &RemoteWorktreeOps{runner: runner}
}

// RemoteWorktree identifies one git worktree on a remote host: the existing
// repository it is attached to (RepoPath — what `git worktree add`/`remove` runs
// against, mirroring GitWorktree.repoPath) and the worktree's own directory
// (WorktreePath — mirroring GitWorktree.worktreePath). Branch is the branch
// CreateWorktree checks out into the new worktree; it is ignored by
// RemoveWorktree. RepoPath and WorktreePath are bundled into this single value
// object rather than passed as separate positional string parameters so a caller
// can't silently transpose them at the call site
// (the `primitive-obsession-checklist` skill).
type RemoteWorktree struct {
	// RepoPath is the existing repository on the remote host that `git worktree
	// add`/`remove` runs against (the CommandRunner.Run "dir" argument).
	RepoPath string
	// WorktreePath is the new worktree's own directory. Its parent directory is
	// the "base_path" CreateWorktree existence-checks before creating anything.
	WorktreePath string
	// Branch is the (already-existing) branch CreateWorktree checks out into the
	// new worktree. CreateWorktree does not create the branch itself — mirroring
	// GitWorktree's setupFromExistingBranch `git worktree add <path> <branch>`
	// shape (worktree_ops.go), not the `-b <branch>` new-branch shape.
	Branch string
}

// CreateWorktree runs `git worktree add <WorktreePath> <Branch>` on the remote
// host via the injected CommandRunner, mirroring GitWorktree.setupFromExistingBranch's
// argument construction (worktree_ops.go's `g.runGitCommand(g.repoPath, "worktree",
// "add", g.worktreePath, g.branchName)`) and returning the same result shape (an
// error only — the worktree's path is already known to the caller via
// w.WorktreePath, exactly as GitWorktree.Setup() returns only an error since the
// path is already stored on the receiver).
//
// Before running `git worktree add`, this checks that w.WorktreePath's parent
// directory ("base_path") exists via `test -d <base_path>`, returning
// ErrRemoteBasePathMissing (not a generic git error) if it does not. This check is
// advisory only, not atomic with the add that follows (see ErrRemoteBasePathMissing's
// doc comment) — it exists to give a fast, distinguishable error for the common case
// (base_path never existed / was never mounted), not as a guarantee against a race
// with something removing it in between.
func (r *RemoteWorktreeOps) CreateWorktree(ctx context.Context, w RemoteWorktree) error {
	basePath := path.Dir(w.WorktreePath)
	if out, err := r.runner.Run(ctx, "", "test", "-d", basePath); err != nil {
		return fmt.Errorf("%w: %s: %s (%v)", ErrRemoteBasePathMissing, basePath, bytesToTrimmedString(out), err)
	}

	if out, err := r.runner.Run(ctx, w.RepoPath, "git", "worktree", "add", w.WorktreePath, w.Branch); err != nil {
		return fmt.Errorf("remote git worktree add failed: %s (%w)", bytesToTrimmedString(out), err)
	}
	return nil
}

// RemoveWorktree runs `git worktree remove <WorktreePath> --force` on the remote
// host via the injected CommandRunner, mirroring CreateWorktree's argument
// construction. Callers use this as best-effort compensating cleanup (Task
// 4.2.1e's partial-failure handling, adversarial-review.md Blocker 3: if remote
// tmux setup fails after CreateWorktree already succeeded, or the SSH connection
// drops between the two steps, the orchestrating caller attempts this before
// surfacing the *original* error) — "best-effort" describes how a caller should
// treat this method's return value (log and continue, don't let a cleanup failure
// mask the real error), not behavior internal to this method itself, which always
// reports its own failures rather than swallowing them.
func (r *RemoteWorktreeOps) RemoveWorktree(ctx context.Context, w RemoteWorktree) error {
	if out, err := r.runner.Run(ctx, w.RepoPath, "git", "worktree", "remove", w.WorktreePath, "--force"); err != nil {
		return fmt.Errorf("remote git worktree remove failed: %s (%w)", bytesToTrimmedString(out), err)
	}
	return nil
}

// InitializeProjectDirectory mirrors session.SessionTypeNewProject's local
// init flow (InitializeProjectDirectory in util.go, driven by
// session/instance_worktree.go's setupFirstTimeWorktree) on a remote host: a
// no-op if projectPath is already a git repo, otherwise create the directory,
// `git init`, and make an initial commit (git worktrees need at least one commit
// to exist before a worktree can be added against them).
//
// The whole flow is shelled as a single `sh -c <script>` CommandRunner.Run call
// rather than several separate Run calls, for two reasons: (1) writing
// .gitignore's content requires shell output redirection ('> .gitignore'), which
// CommandRunner.Run cannot express directly — Run has no stdin parameter to pipe
// content through (see command_runner.go's doc comment: only Start is piped, and
// only for the one long-lived control-mode case), and buildRemoteCommand
// shell-quotes each argument individually, so passing '>' as a bare Run argument
// would reach the remote as a literal, inert string rather than a shell operator;
// (2) it keeps this to one round trip against what may be a slow remote link,
// rather than five.
//
// The already-a-repo check tests for a ".git" entry directly under projectPath
// (`[ -e .git ]` after cd, not `-d`: a git worktree's ".git" is a *file*
// containing a `gitdir:` pointer, not a directory) rather than
// `git rev-parse --is-inside-work-tree`, which walks UP the directory tree
// looking for the nearest .git — that would wrongly report "already a repo"
// for a projectPath nested inside some unrelated ancestor repository. This
// matches go-git's PlainOpen(path) semantics (no DetectDotGit, so no upward
// search; and PlainOpen itself accepts either a .git directory or a .git
// file), which is what the local InitializeProjectDirectory uses for its own
// no-op check.
//
// Deliberately simpler than the local flow's rollback-on-failure behavior (which
// os.RemoveAll's the directory it created if `git init` or the initial commit
// fails): there is no local os.RemoveAll equivalent here cheap enough to add
// safely — embedding an `rm -rf` in a remote script is exactly the kind of command
// this package should be conservative about — and there is no production caller
// of this method yet (Epic 2.2 wires no caller; Phase 4 is where remote
// SessionTypeNewProject support would consume it). A failed remote init can leave
// a partially-initialized directory behind for a caller/operator to inspect or
// retry against.
func (r *RemoteWorktreeOps) InitializeProjectDirectory(ctx context.Context, projectPath string) error {
	// The `git rev-parse --verify -q HEAD` check right before the destructive
	// .gitignore write/commit is a self-defending guard, not the primary gate
	// (`[ -e .git ]` above already prevents the normal re-run case) — mirrors
	// createInitialCommit's repoHasAnyRef guard in util.go.
	script := fmt.Sprintf(`set -e
mkdir -p %[1]s
cd %[1]s
if [ -e .git ]; then
  exit 0
fi
git init .
if git rev-parse --verify -q HEAD >/dev/null 2>&1; then
  echo "refusing to create initial commit: repository already has existing refs" >&2
  exit 1
fi
printf '%%s' '# Project gitignore
' > .gitignore
git add .gitignore
git -c user.name='Stapler Squad' -c user.email='stapler-squad@localhost' commit -m 'Initial commit'
`, posixShellQuote(projectPath))

	if out, err := r.runner.Run(ctx, "", "sh", "-c", script); err != nil {
		return fmt.Errorf("remote project init failed at %s: %s (%w)", projectPath, bytesToTrimmedString(out), err)
	}
	return nil
}

// posixShellQuote POSIX-single-quotes s so it survives the remote `sh -c` this
// file builds unmodified regardless of embedded spaces or shell metacharacters
// (a project path is not attacker-controlled in practice, but quoting it
// defensively costs nothing and avoids depending on that assumption staying
// true). Deliberately package-local rather than reusing session/tmux's
// unexported shellQuote in ssh_runner.go, which this package cannot import
// (unexported, different package) — same escaping strategy (close quote,
// literal quote, reopen quote), independently duplicated because it is a
// three-line pure function, not worth promoting to a shared export for one
// caller on each side.
func posixShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// bytesToTrimmedString renders a CommandRunner.Run output byte slice for
// inclusion in an error message, trimming trailing newlines so error strings
// don't carry an ugly blank line from git/shell output that always ends in \n.
func bytesToTrimmedString(b []byte) string {
	return strings.TrimRight(string(b), "\n")
}
