# Pitfalls Research: worktree-selfheal-test-flake

Agent 4 (Pitfalls). Scope: known failure modes for (a) concurrent git subprocess races over
a shared `.git/worktrees/` metadata dir, (b) string-matching on subprocess stderr for control
flow, (c) fixed context timeouts under CI load, and what the field considers a robust fix vs a
superficial one.

## 1. Concurrent real-git-subprocess races + `t.Parallel()` compounding

`session/git/worktree_ops_test.go:339` (`TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate`)
calls `t.Parallel()` and then spins up its own two goroutines racing real `git` subprocesses
against one `setupTestRepo(t)`-created repo. `t.Parallel()` does not just mean "this test may
run concurrently with siblings in the same file" — the Go test runner holds all `t.Parallel()`
tests in the package until every non-parallel test finishes, then releases them together up to
`-parallel` (default `GOMAXPROCS`). That means this test's two *intentional* racing goroutines
are themselves scheduled on top of however many *other* parallel tests the runner picked to run
concurrently in `session/git` — real OS-level CPU/scheduler contention this test's design
doesn't control or account for.

Count of `t.Parallel()` call sites, by file (`grep -c "t.Parallel()" session/git/*_test.go`):

| File | Count |
|---|---|
| `ops_test.go` | 28 |
| `worktree_git_test.go` | 21 |
| `worktree_ops_test.go` | 11 |
| `drift_test.go` | 8 |
| `worktree_test.go` | 6 |
| `util_test.go` | 6 |
| `worktree_git_stage_test.go` | 3 |
| `diff_test.go` | 1 |
| `scaffolding_test.go` | 1 |
| `ssh_test_server_test.go` | 0 |
| `remote_worktree_test.go` | 0 |
| `worktree_creation_test.go` | 0 |

**84 total parallel-marked tests in the package**, many of which (`ops_test.go`,
`worktree_git_test.go`, `worktree_ops_test.go` itself) spawn their own real `git`
subprocesses. Under `go test ./session/git -race` with the default parallelism cap, dozens of
these can be runnable at once, each forking a `git` process — this is exactly the kind of
"full-suite CI load" the requirements doc's problem statement describes (PR #583's run failed
under full-suite load, not in isolation). `-race` further slows every goroutine schedule point
and subprocess wait, stretching the two racing goroutines' actual wall-clock window against the
fixed 30s timeout in `runGitCommand` (`session/git/worktree_git.go:38`) — i.e. more real
external load makes the 30s ceiling relatively tighter, not looser. This is consistent with
AC-1's instruction to attempt reproduction via CPU contention / `-race` / `-count=N`.

Known pitfall class this matches: parallel Go subtests that shell out to a real, stateful CLI
(git) against a shared filesystem path are a classic source of "reproduces only under full
suite, never in isolation" flakes, because the two axes (test-authored concurrency +
test-runner-authored concurrency) are invisible to each other. `t.Parallel()` is the right tool
for wall-clock speedup across *independent* tests; it is not neutral when the test body itself
is a concurrency stress test with a fixed timeout budget.

## 2. String-matching git stderr for control flow

Both fallback branches keyed off raw substring matches:
- `session/git/worktree_ops.go:136`: `strings.Contains(err.Error(), "already checked out") || strings.Contains(err.Error(), "already used by worktree")`
- `session/git/worktree_ops.go:336`: `strings.Contains(err.Error(), "already exists")`
- (Also present, same file, `rev-parse HEAD` handling at :315-317, matching `"fatal: ambiguous argument 'HEAD'"` etc. — same pattern, same risk class, not in scope of this bug but worth noting as the same anti-pattern recurring in the file.)

Risks specific to this technique, confirmed against this codebase:

- **No locale pinning.** `grep -rn "LC_ALL\|LANG=" session/git/*.go executor/safeexec/*.go`
  returns nothing relevant — `LocalRunner.Run` (`session/tmux/command_runner.go:81-85`) and
  `safeexec.CommandContext` (`executor/safeexec/safeexec.go:30`) both inherit the parent
  process's environment unmodified; nothing sets `LC_ALL=C` or `LANG=C` before invoking `git`.
  Git localizes its fatal/error messages when a translation exists and `gettext` support is
  compiled in (Debian/Ubuntu git packages typically are) and `LC_ALL`/`LANG`/`LANGUAGE` is set to
  a non-English locale. A CI runner or a contributor's machine with e.g. `LANG=de_DE.UTF-8` would
  make `git worktree add` emit a non-English "already exists" / "already checked out" /
  "already used by worktree" string, silently defeating both `strings.Contains` checks and
  turning the self-heal path into a hard failure — indistinguishable at the call site from the
  timeout case in §3 below. This is a live, not just theoretical, gap: nothing in this code path
  forces English output the way e.g. `git -c core.quotepath=false` or `GIT_TERMINAL_PROMPT=0` is
  sometimes forced elsewhere for deterministic parsing.
- **Version-dependent wording**, already partially documented in-repo: the doc comment at
  `worktree_ops.go:133-135` explicitly notes older git says "already checked out" while git
  2.50.1 says "already used by worktree at '<path>'" — which is *why* both strings are matched
  today. This is direct evidence the pattern is already known to be version-fragile; a future git
  release changing wording again (or a CI image pinned to a different git version than the
  developer's local git) reopens the same gap the two-string OR was patched in to close, and
  nothing here guards against a *third* future wording.
- **Truncation/wrapping**: `runGitCommand` (`worktree_git.go:37-47`) builds the returned error as
  `fmt.Errorf("git command failed: %s (%w)", output, err)` where `output` is combined
  stdout+stderr from `cmd.CombinedOutput()`. As long as git's fatal line makes it into that
  buffer intact this isn't truncated in this codebase (no length-capping observed), but it means
  the substring check is being run against a synthetically-reconstructed message, not git's raw
  stderr — an unrelated stdout line landing between the git binary's write of "Preparing worktree
  ..." progress output and the fatal line could theoretically split things, though this is a
  lower-probability contributor than the locale/version issues above.

## 3. `context.DeadlineExceeded` error shape vs. the string checks

Confirmed via `go doc os/exec.Cmd` (go1.26.4) and reading `executor/safeexec/safeexec.go`:

- `runGitCommand` uses `context.WithTimeout(..., 30*time.Second)` (`worktree_git.go:38`), then
  calls `g.commandRunner().Run(...)`, which for `LocalRunner` is
  `safeexec.CommandContext(ctx, name, args...)` + `cmd.CombinedOutput()`
  (`session/tmux/command_runner.go:81-85`).
- `safeexec.CommandContext` (`executor/safeexec/safeexec.go:28-33`) sets `cmd.WaitDelay =
  2*time.Second` but does **not** override `cmd.Cancel`, so the stdlib default from
  `exec.CommandContext` still applies: "CommandContext sets the command's Cancel function to
  invoke the Kill method on its Process" (`go doc os/exec.CommandContext`).
- Per `go doc -all os/exec.Cmd`'s `Cancel` field docs: *"If the command exits with a success
  status after Cancel is called ... Wait ... will return a non-nil error... (If the command
  exits with a non-success status ... Wait ... continue[s] to return the command's usual exit
  status.)"* A `git` process SIGKILL'd mid `worktree add -b` exits with a non-success (killed)
  status, so per this documented behavior **`Wait`/`CombinedOutput` returns the ordinary
  killed-process error, not one wrapping `ctx.Err()`/`context.DeadlineExceeded`.** The resulting
  `err.Error()` string is `"signal: killed"` (a `*exec.ExitError`), or, if the process doesn't
  exit within `safeexec`'s 2s `WaitDelay` after the kill signal, an `"exec: WaitDelay expired
  before I/O complete"`-shaped error instead.
- Neither of those strings, nor "context deadline exceeded" (which per the doc above the code
  path won't even produce here), contains `"already exists"`, `"already checked out"`, or
  `"already used by worktree"`. **Confirmed: a timeout-triggered kill under load cannot be caught
  by either fallback branch** and will fall through to `worktree_ops.go:340`
  (`return fmt.Errorf("failed to create worktree from commit %s: %w", headCommit, err)`) — a hard
  failure, exactly matching the observed PR #583 CI failure shape (self-heal fallback "itself
  returned a hard error").
- This makes the timeout path and the locale-mismatch path (§2) two *independently sufficient*
  explanations for the same observed symptom (hard failure despite the self-heal code existing).
  Distinguishing which one actually fired on PR #583's run requires the captured `err.Error()`/CI
  log text, not just static analysis — flag this for whoever runs AC-1's reproduction attempt:
  capture and log the raw error string on failure before concluding which hypothesis holds.

## 4. Why bumping the timeout or the string list is a symptom fix, and what this repo already does instead

- **Bumping `strings.Contains` matching** (adding more literal strings) only chases the specific
  wording git happens to emit in whatever locale/version combination was just observed. It cannot
  close the class: the same repo's own doc comment at `worktree_ops.go:133-135` shows this
  pattern already required a second string once (git 2.50.1's rewording), and §2 shows a locale
  change reopens it structurally, not just as an edge case.
- **Bumping the 30s timeout** is a pure symptom fix for the CI-load case: it changes the
  probability of hitting the race window without changing what happens on the (still possible,
  just rarer) occasion it's hit — the resulting error still isn't recognized as "the race we know
  how to self-heal from," it just hard-fails less often. It also does nothing for the locale
  gap in §2, which is orthogonal to timing entirely.
- **What this repo already does elsewhere for the same class of problem**: `session/git/ops.go`
  (`FetchBranch` at :23-35, and two more sites at :536, :551) unwraps to `*exec.ExitError` via a
  type assertion (`if exitErr, ok := err.(*exec.ExitError); ok`) rather than string-matching, and
  `session/tmux/tmux.go` does the same at :452, :2303, :2807, :2840, :3046 (some further checking
  `.ExitCode() == 1` for a specific known "no match" exit code). This is the established in-repo
  pattern for "branch on *why* a subprocess failed" — structured (exit code / error type), not
  textual.
  - Caveat worth flagging for the planning phase: git's exit code for `worktree add` fatal errors
    is uniformly 128 regardless of *which* fatal condition fired (already-exists, already-checked-
    out, and a killed/timeout case all plausibly surface as a non-zero/exit code), so exit code
    alone cannot replace the string check's job of distinguishing "self-healable race" from "some
    other fatal error" — it can only reliably distinguish "killed by signal" (no `ExitCode()`
    signal-based path, `.Error()` "signal: killed", or in Go's `ProcessState`, `.Success() ==
    false` with `.ExitCode() == -1` for a signal death vs. a real 128 exit) from "git actually ran
    to completion and refused." That distinction alone is enough to rule out the timeout case
    from ever being misrouted into a string-match branch it can't satisfy, but not enough to
    remove string-matching for the exists-vs-checked-out distinction outright.
  - The **structurally robust alternative** for the actual self-heal decision (matches how
    `worktreeAlreadyRegisteredForBranch` and `findWorktreeForBranch` already work in this same
    file) is to stop relying on stderr wording to *infer* the race happened, and instead
    **re-check ground truth after any `worktree add` failure**: run `git worktree list
    --porcelain` (already available via `g.findWorktreeForBranch`) and/or `git branch --list
    <branch>` to see whether the branch/worktree that *should* exist if a concurrent winner
    created it actually does, independent of what git's error text said. That converts "did we
    lose a race" from a text-parsing question into a state-query question, which is immune to
    both git version and locale changes, and is the same self-healing idiom
    `setupFromExistingBranch`'s locked-worktree handling (`worktree_ops.go:189-220`) already uses
    for a structurally identical problem ("is this worktree really in the state we assumed").
    A bounded retry loop (capped attempts) around that re-check, rather than a single
    string-matched fallback, is the standard shape for this idiom elsewhere in Go codebases
    handling optimistic-concurrency races against an external CLI tool.

## Summary for planning phase

1. **Timeout-under-load is real and provably unrecoverable today**: a `context.WithTimeout`
   SIGKILL under CI contention produces `"signal: killed"` (or a `WaitDelay`-expiry message),
   never a string containing `"already exists"`/`"already checked out"`/`"already used by
   worktree"` — confirmed via `go doc os/exec.Cmd`'s documented `Cancel`/non-success-exit
   behavior and `executor/safeexec/safeexec.go`'s `WaitDelay` wrapper, not just inference. This
   is one of two independently-sufficient explanations for the observed PR #583 hard failure.
2. **The string-match fallback has a second, orthogonal, currently-open gap**: no `LC_ALL=C`/
   `LANG=C` is set anywhere in `runGitCommand`'s call chain (`worktree_git.go`,
   `command_runner.go`, `safeexec.go`), so any non-English git locale on the running machine
   silently breaks both fallback branches — this is a pre-existing, not test-only, production
   reachability gap (AC-3), even though it's less likely to be *what fired on PR #583* than the
   timeout.
3. **The robust fix shape, with in-repo precedent** (`ops.go`'s `*exec.ExitError` type
   assertions, `worktree_ops.go`'s own `worktreeAlreadyRegisteredForBranch`/
   `findWorktreeForBranch` state-verification idiom): after a `worktree add` failure, re-query
   `git worktree list --porcelain` / `git branch --list` to check whether the race's *winner*
   already produced the expected state, instead of inferring the race from stderr wording alone.
   Bumping the timeout or adding another literal string are both symptom fixes explicitly
   disallowed by AC-4 — they reduce the failure's probability or scope without changing what
   happens when either underlying condition (load-triggered kill, non-English locale) recurs.
