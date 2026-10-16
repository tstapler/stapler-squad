# ADR-002: `CommandRunner` Interface Defined in `session/tmux`, Reused by `session/git`

**Status**: Accepted
**Date**: 2026-08-06
**Project**: ssh-remote-workspaces

## Context

Every tmux operation in `session/tmux/tmux.go` shells out via
`safeexec.CommandContext(ctx, Binary(), args...)` (confirmed call sites: lines 298, 328, 509,
533, 555, 612, 631, 642, 898, 1902, 2295, 2318), assuming the `tmux` binary and its Unix-domain
socket are on the local machine. `session/git/worktree_git.go`'s mutating operations
(`runGitCommand`/`runExec`, lines 27, 301) have the identical local-subprocess assumption.
Neither package has any concept of "which host."

`server/services/session_streamer.go`'s `SessionStreamer` interface is this repo's own
precedent for the correct shape of seam: four methods, defined in the *consumer* package
(`server/services`), satisfied implicitly by `*session.Instance` via delegation — exactly the
pattern `.claude/rules/interface-pollution-checklist.md` asks for (interface scoped to what
the consumer needs, not declared next to its implementation).

## Decision

Define `CommandRunner` in `session/tmux` — the package that is the *consumer* of today's
`safeexec.CommandContext` calls:

```go
// session/tmux — consumer package
type CommandRunner interface {
    Run(ctx context.Context, name string, args ...string) ([]byte, error)
    Start(ctx context.Context, name string, args ...string) (stdin io.WriteCloser, stdout io.ReadCloser, wait func() error, err error)
}
```

`Run` covers the one-shot exec call sites (`has-session`, `kill-session`, `list-sessions`,
etc.). `Start` covers the long-lived attached case (`tmux -C` control-mode process) that needs
piped stdin/stdout and an explicit wait, which a naive `Run`-only interface cannot express (see
plan.md's Step 0.5 creative-pass notes on why a single-method interface was rejected).
`LocalRunner` wraps today's `safeexec.CommandContext(...)`. `SSHRunner` (Phase 2) wraps a
persistent `*ssh.Client`'s `NewSession()`/`Session.Run`/`Session.Start`. `session/git` imports
and reuses the same `CommandRunner` type from `session/tmux` for its own mutating call sites
(`runGitCommand`/`runExec`) rather than defining a second, near-identical interface — one seam,
two consumers.

## Alternatives Considered

- **Define `CommandRunner` in a new top-level package (e.g. `session/exec` or
  `session/sshremote`), imported by both `session/tmux` and `session/git`.** Rejected per
  `.claude/rules/interface-pollution-checklist.md` smell #2 ("interface defined next to its
  implementation" — inverted here to "interface defined in neither consumer") — a
  freestanding package with no behavior of its own beyond the interface declaration is exactly
  the speculative-abstraction shape the checklist warns against. `session/tmux` is the
  original, primary consumer; `session/git` importing from `session/tmux` is a normal
  same-module dependency, not a reason to extract a third package.
- **A single-method `Run(ctx, name, args) ([]byte, error)` interface, with control-mode
  handled as a special case outside `CommandRunner`.** Rejected: this was the initial creative-
  pass candidate (see plan.md Step 0.5, Approach A). It under-serves the persistent `tmux -C`
  attach flow, which needs piped stdin/stdout it can write to and read from incrementally, not
  a single buffered `[]byte` result — forcing that case through a workaround would mean two
  parallel abstractions (one for one-shot exec, one for streaming) instead of one.
- **Adapter that makes `*ssh.Session` satisfy the existing `*exec.Cmd`-shaped call sites
  directly (no new interface at all).** Rejected: `*exec.Cmd` exposes OS-process-specific
  surface (`Process.Pid`, `ProcessState`, signal delivery) that has no SSH analog and that
  several `tmux.go` call sites depend on (e.g. `kill -WINCH <panePID>` at line 2062) — forcing
  an `*ssh.Session` to impersonate an `*exec.Cmd` would be a leakier abstraction than a small
  purpose-built interface.

## Consequences

- All 12+ `safeexec.CommandContext` call sites in `tmux.go` become `runner.Run(...)` /
  `runner.Start(...)`, with `LocalRunner` as the zero-behavior-change default — Phase 1 ships
  with no SSH capability at all, purely as a refactor validated by existing tests plus new
  characterization tests (Epic 1.4).
- `session/git/worktree_git.go`'s remote-worktree mutations (Phase 2) route through the same
  interface, which is the repo's own documented exception to
  `.claude/rules/prefer-go-git-over-subshells.md` (go-git has no remote-filesystem transport
  for worktree operations at all).
- `session.Instance` does not need a "remote" concept of its own — the `CommandRunner` is
  selected once, at `TmuxSession`/`GitWorktree` construction time, and every downstream method
  is unchanged in signature.

## Addendum 1 (2026-08-16): injection mechanism, `dir` parameter, and known gaps

Phase 1 code review (FIX-FIRST) found two structural gaps in the first implementation pass, both
fixed in the same change. Recorded here per this ADR's own practice of capturing signature
decisions, not as a rewrite of the sections above.

### Injection: `WithCommandRunner` functional options

The first pass gave every `TmuxSession`/`GitWorktree` constructor a hardcoded `runner:
LocalRunner{}` with no way for any caller to override it — meaning the seam existed but nothing
could actually plug an `SSHRunner` (or a test spy) into it. Fixed by adding a
`WithCommandRunner` functional option to both types, mirroring the pattern each package already
used for its own equivalent problem:

- `session/tmux`: `WithCommandRunner(r CommandRunner) TmuxSessionOption`, alongside the existing
  `WithRegistry` (`session/tmux/tmux.go`). `opts ...TmuxSessionOption` was added as a trailing,
  backward-compatible parameter to every constructor that builds a `*TmuxSession` (`NewTmuxSession`,
  `NewTmuxSessionWithPrefix`, `NewTmuxSessionWithCleanup`, `NewTmuxSessionWithPrefixAndCleanup`,
  `NewTmuxSessionWithServerSocketAndCleanup`, `NewTmuxSessionWithDeps`,
  `NewTmuxSessionFromExisting`, `NewTmuxSessionFromExistingWithServerSocket`, plus the internal
  `newTmuxSession`) — `NewTmuxSessionWithServerSocket` already took `opts ...TmuxSessionOption`.
- `session/git`: no functional-option mechanism existed yet (constructors take explicit
  `cmdExec executor.Executor` parameters instead), so a new, symmetric `GitWorktreeOption
  func(*GitWorktree)` type plus `WithCommandRunner(r tmux.CommandRunner) GitWorktreeOption` was
  introduced and added as a trailing `opts ...GitWorktreeOption` parameter to every constructor
  that builds a `*GitWorktree` (`NewGitWorktreeFromCommitSHA`,
  `NewGitWorktreeFromStorageWithExecutor` and its `NewGitWorktreeFromStorage` wrapper,
  `NewGitWorktreeWithBranchAndExecutor` and its `NewGitWorktree`/`NewGitWorktreeWithBranch`
  wrappers, `NewGitWorktreeFromExistingWithExecutor` and its `NewGitWorktreeFromExisting`
  wrapper).

Both are trailing/variadic, so no existing call site anywhere in the repo needed to change.
`TestWithCommandRunner_InjectedRunnerIsActuallyUsed` (one in `session/tmux/command_runner_test.go`,
one in `session/git/worktree_git_test.go`) each construct with a test spy `CommandRunner`,
exercise a real method (`TmuxSession.RefreshClient`'s SIGWINCH fallback;
`GitWorktree.PushBranch`), and assert the spy — not `LocalRunner` — received the call, proving
the injected value is actually consulted, not merely stored on the struct.

### `dir` parameter on `Run`/`Start`

The first pass's `Run`/`Start` had no way to express a working directory, so every
`session/git/worktree_git.go` call site that needs one (`cmd.Dir = g.worktreePath`, on all ~14
git/gh invocations in that file) couldn't be migrated onto `CommandRunner` at all — the review
correctly flagged this as a real interface gap, not a shortcut. Fixed by adding a `dir` parameter
to both methods:

```go
type CommandRunner interface {
    Run(ctx context.Context, dir, name string, args ...string) ([]byte, error)
    Start(ctx context.Context, dir, name string, args ...string) (stdin io.WriteCloser, stdout io.ReadCloser, wait func() error, err error)
    IsRemote() bool
}
```

`LocalRunner` sets `cmd.Dir = dir` before executing (a no-op when `dir` is `""`, matching
`(*exec.Cmd).Dir`'s own empty-string-means-caller's-cwd semantics). Every already-migrated
`session/tmux` call site passes `""` (tmux commands are server-level, with no meaningful working
directory); every `session/git/worktree_git.go` call site now passes `g.worktreePath` (or the
generic `path` parameter, for `runGitCommand`'s callers — see the correction below), which is
also how `runExec`/`runCombinedOutput` (now deleted) and every PR/branch operation
(`PushChanges`, `PushBranch`, `OpenBranchURL`, `CreatePR`, `findExistingPR`, `GetPRStatus`,
`EnablePRAutoMerge`, `RequestCopilotReview`, `ClosePR`, `IsPRMerged`) were migrated onto
`g.commandRunner().Run(ctx, g.worktreePath, ...)` directly, retiring the `*exec.Cmd`-passing
`runExec`/`runCombinedOutput` helpers entirely.
>
> **Correction (see Addendum 2 below):** this paragraph originally claimed `runGitCommand`'s
> `g.cmdExec`-gated branch was left in place because "only `IsDirtyWithHint`'s path is actually
> exercised with a custom mock." That was true of *test* coverage but wrong about *production*
> reachability: `runGitCommand` backs ~25 production call sites (every `worktree_ops.go`
> `worktree add`/`remove`/`prune`/`list` call among them), and every production `GitWorktree`
> constructor always defaults `cmdExec` to a non-nil `executor.MakeExecutor()` — so the
> `g.commandRunner().Run(...)` branch this addendum describes was **dead code in production**,
> and `WithCommandRunner` was never consulted for anything routed through `runGitCommand`. Fixed
> in Addendum 2.

**Primitive-obsession check** (`.claude/rules/primitive-obsession-checklist.md`): `dir` and
`name` are adjacent same-typed (`string`) parameters, which is the checklist's smell #6-adjacent
pattern. Left as plain positional parameters rather than wrapped in a newtype: the checklist's
motivating harm is a swap that *silently* produces a different-but-plausible result (its
`RepoRef`/`AccountRef` examples — swapping `owner`/`repo` still names a real, valid repo).
Swapping `dir` and `name` here instead fails loudly and immediately at exec time (a directory
path is never a valid program name, and a program name is essentially never also a valid
directory in the worktree/tmux contexts these are called from) — the specific silent-plausible-
wrongness a newtype exists to prevent isn't present. `Run(ctx, dir, name, args...)` (dir before
name) was chosen over `Run(ctx, name, dir, args...)` for readability: it reads as "in dir, run
name with args," and keeps `dir` out from between `name` and the `args` that belong to it.

### WARNING (acknowledged, not fixed): `Run`'s combined-output semantics

`ListAllSessions` and `KillOrphanedControlModeClients` (`session/tmux/tmux.go`) previously used
`.Output()` (stdout only, with stderr surfaced separately via `err.(*exec.ExitError).Stderr`);
migrating them onto `CommandRunner.Run` switched them to `.CombinedOutput()` semantics (stdout
and stderr merged into one buffer), since `Run` only exposes one shape. This is a low-risk,
already-inline-documented tradeoff (tmux does not write to stderr on the success paths these
functions parse) accepted deliberately rather than adding a second `Run`-vs-`Output` method to
`CommandRunner` — a second method for this narrow a gap would be its own
interface-pollution risk for marginal benefit. Not revisited further in Phase 1.

### WARNING (deferred to Phase 2, not fixed): package-level free functions have no runner seam

`session/tmux`'s package-level free functions (`ListAllSessions`, `checkServerNotRunning`,
`EnsureServerRunning`, `KillOrphanedControlModeClients`, `SetExitEmpty`,
`CreateKeepaliveSession`) and `CleanupSessionsOnServer` still hardcode `LocalRunner{}` inline,
with no parameter to thread a different `CommandRunner` through — because they operate at the
tmux-server level (keyed by `serverSocket string`), not on a `*TmuxSession`/`*GitWorktree`
instance, `WithCommandRunner` has nothing to attach to for them. Real work, correctly scoped to
Phase 2 (when a per-server-socket-to-runner mapping needs to exist for remote server
provisioning/teardown), not Phase 1.

## Addendum 2 (2026-08-16): `runGitCommand` was dead-code-gated in production, not just untested

A second re-review caught what Addendum 1 undersold. `runGitCommand`
(`session/git/worktree_git.go`) — the helper behind `RenameBranch`, `stageAndCommit`,
`StageAllExceptScaffolding`, `HasStagedChanges`, `IsDirtyWithHint`, and, critically, every
`worktree_ops.go` `worktree add`/`remove`/`prune`/`list` call (the exact operations Phase 2's
`RemoteWorktreeOps` needs to build on) — still branched on `g.cmdExec != nil`, and **every**
production `GitWorktree` constructor unconditionally defaults `cmdExec` to a non-nil
`executor.MakeExecutor()`. So the `g.commandRunner().Run(...)` branch Addendum 1 added was
**dead code for all ~25 production call sites**: `WithCommandRunner` was never consulted for
anything that went through `runGitCommand`, confirmed empirically (a `CommandRunner` spy injected
via `WithCommandRunner` and driven through `runGitCommand` recorded zero calls). This directly
contradicted plan.md's Task 1.3.1b ("migrate `runGitCommand`... unconditionally") and this ADR's
own Consequences bullet claiming Phase 2's remote-worktree mutations route through this
interface — they did not, for the one helper that matters most for worktree creation/teardown.

Unlike `session/tmux`'s `buildTmuxCommandContext`/`listSessionsRaw` cmdExec branches — which
genuinely wrap a `CircuitBreakerExecutor`, a real, orthogonal, production-live seam — `session/git`'s
`cmdExec` is never circuit-breaker-wrapped anywhere in the repository. It was a plain
test-injection seam with no production behavior `CommandRunner` couldn't also provide, so there
was no architectural justification (unlike the tmux control-mode cases, which keep a documented,
intentional local-only exception) for leaving it unmigrated.

**Fix**: `runGitCommand` now routes through `g.commandRunner().Run(ctx, path, "git", args...)`
unconditionally, exactly like every other call site in the file. The `-C path` git flag
previously prepended to every invocation's args was dropped in favor of the `dir` parameter
Addendum 1 added — functionally identical (`-C <path>` and `(*exec.Cmd).Dir` both chdir before
git runs; confirmed via `git`'s own documented behavior), and now consistent with how every other
migrated call site in this file passes its directory.

The `cmdExec executor.Executor` field, and the `cmdExec` parameter on
`NewGitWorktreeFromStorageWithExecutor`/`NewGitWorktreeWithBranchAndExecutor`/
`NewGitWorktreeFromExistingWithExecutor`, are removed entirely: confirmed via full-repo grep that
`cmdExec` had no other functional consumer anywhere in `session/git` after this fix, and no
caller outside `session/git` itself ever invoked those three `...WithExecutor` constructors
directly (only their `cmdExec`-less wrappers — `NewGitWorktreeFromStorage`, `NewGitWorktree`,
`NewGitWorktreeWithBranch`, `NewGitWorktreeFromExisting` — and the package's own tests did). The
three constructors keep their `...WithExecutor` names (a deliberate, bounded decision — renaming
long-standing exported constructors was judged out of scope for this fix, since it isn't required
for correctness and the confirmed-zero blast radius outside this package means it can be a
low-risk fast-follow rather than something this change needs to force through); their doc
comments now say so explicitly and point to `WithCommandRunner` as the replacement seam.

`IsDirtyWithHint`'s two race/error-injection tests
(`TestIsDirtyWithHint_ReturnsLocallyComputedValue_WhenCacheIsWrittenByRacingGoroutine`,
`TestIsDirtyWithHint_BacksOffAfterError`) — the only real consumers of the old `cmdExec` branch —
were ported from `executor.Executor`-implementing mocks (`raceSimulatorExecutor`,
`countingErrExecutor`, both deleted) onto `gitSpyCommandRunner` (the existing `tmux.CommandRunner`
test spy in `worktree_git_test.go`, extended with a `runFunc` hook so a test can inject a
side-effect — e.g. writing to the dirty cache mid-call, reproducing `raceSetup`'s role — at the
exact moment a call happens, not just script a fixed return value) injected via
`WithCommandRunner`. A new `TestRunGitCommand_UsesInjectedCommandRunner` calls `runGitCommand`
directly and asserts the spy received `{dir, "git", args}`, proving the seam is live for its
default path specifically, not merely that the pre-existing tests still pass.

Verified: `go build`/`go vet` clean on `session/tmux` and `session/git`; full-repo `go build
./...`/`go vet ./...` show no new breakage; `go test ./session/tmux/... ./session/git/...
-race -count=1` both `ok` (zero modified assertions in any pre-existing test — the two ported
`IsDirtyWithHint` tests changed their mock plumbing, not their assertions or expected values);
`gofmt -l session/tmux/ session/git/` clean.
