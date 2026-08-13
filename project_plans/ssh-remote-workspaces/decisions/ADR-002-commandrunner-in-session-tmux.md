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
