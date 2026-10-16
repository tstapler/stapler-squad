# Architecture: worktree-selfheal-test-flake

## 0. Prior art check

- `project_plans/worktree-branch-exists-race/research/architecture.md` (same file,
  `session/git/worktree_ops.go`, a prior fix) already did the heavy lifting on the
  `Setup()`/`setupLocked()`/`setupNewWorktree()` control-flow map, the full non-test caller
  list for `.Setup()`, and the interface-pollution assessment of `branchRefExists` — cited
  and built on below rather than re-derived. That doc's own §2 already names, as an
  explicitly out-of-scope "Known Gap", a **different** race (two independent `Setup()` calls
  for the same deterministic branch, each seeing the branch legitimately absent) — this
  current investigation is about a related but distinct question: not whether that TOCTOU
  exists (it does, and is already tracked), but whether the *test* that exercises the
  self-heal fallback for it reflects a reachable production path at all.
- No `code-hotspot-analysis` or `quality:architecture-review` run has ever targeted
  `session/git` specifically — `grep -rl "session/git" project_plans/*/research/architecture.md`
  returns 13 files, all of which are other projects' architecture docs that merely mention
  `session/git` in passing (caller lists, wrapping chains), not a dedicated review of the
  package itself.
- Agent 1's `research/stack.md` (this project) already did the CI-environment and
  string-matching legwork and its own summary leans toward "test timing" over "unrecognized
  error string" or "real fallback gap" — this doc confirms that conclusion from the
  concurrency-architecture side and adds the structural argument for *why* it's a test-only
  artifact, not a production gap.

## 1. Is it true that every production path into `setupNewWorktree` goes through `Setup()`
   (and thus the lock)?

**Yes, confirmed.** `grep -rn "setupNewWorktree(\|setupFromExistingBranch(" --include="*.go" .`
finds exactly these non-test call sites, all inside `session/git/worktree_ops.go` itself:

| Line | Caller | Reached via |
|---|---|---|
| 100, 102 | `setupLocked()` dispatches to one or the other based on `branchRefExists` | — |
| 297 | `setupNewWorktree()`'s own upfront branch-exists check falls through to `setupFromExistingBranch()` | internal |
| 338 | `setupNewWorktree()`'s `worktree add -b` failure self-heal | internal |

`setupLocked()` itself has exactly two callers, both lock-guarded:

- `Setup()` (`worktree_ops.go:39-41`): `return WithRepoWorktreeLock(g.repoPath, g.setupLocked)`
- `SetupLocked()` (`worktree_ops.go:53-55`): documented as "assumes the caller already holds
  repoPath's worktree lock" — its sole non-test caller, `session/instance_worktree.go:256`,
  calls it from *inside* a `git.WithRepoWorktreeLock(resolvedRepo, func() error { ... })`
  closure (`instance_worktree.go:240`), confirmed by reading that call site directly.

The only unlocked callers of `setupNewWorktree()`/`setupFromExistingBranch()` anywhere in the
repo are in `session/git/worktree_ops_test.go` — including the flaky test itself, and its own
doc comment (lines 318-337) explicitly and self-awarely names this as the deliberate design
choice ("Unlike `TestSetup_Serializes...` — which calls the public, now-lock-wrapped `Setup()`
— this test calls the unlocked `setupNewWorktree()` directly"). So the requirements doc's
premise is verified, not just asserted: **no production caller can reach `setupNewWorktree()`
without first passing through `WithRepoWorktreeLock`.**

## 2. Control/data flow and the two 30s timeouts

Full chain: `Setup()` → `WithRepoWorktreeLock` (acquire `mu` + `flock`) → `setupLocked()` →
branch-exists check → (`setupFromExistingBranch` | `setupNewWorktree`) → on `git worktree add
-b` "already exists" → `setupFromExistingBranch` → on `git worktree add` "already checked
out"/"already used by worktree" → find-existing-worktree fallback (`findWorktreeForBranch`
over `git worktree list --porcelain`).

Two independent 30-second timeouts are in play, and they do **not** compound the way the
requirements doc's phrasing ("does the lock's own 30s interact with or compound the
runGitCommand 30s timeout") might suggest:

- **`worktreeLockTimeout = 30 * time.Second`** (`worktree_lock.go:20`) bounds only
  `l.flock.TryLockContext(ctx, 100*time.Millisecond)` (`worktree_lock.go:97`) — i.e. how long
  a caller will wait to *acquire* the cross-process flock. Once acquired, `fn()` (which is
  `setupLocked` or the caller's `SetupLocked`-wrapping closure) runs to completion with **no**
  time budget imposed by `WithRepoWorktreeLock` itself.
- **`runGitCommand`'s `context.WithTimeout(context.Background(), 30*time.Second)`**
  (`worktree_git.go:38`) is a *separate*, per-subprocess timeout, applied fresh to each
  individual `git` invocation (`worktree add`, `worktree remove`, `worktree list
  --porcelain`, etc.) inside `setupNewWorktree`/`setupFromExistingBranch`. `setupLocked`'s
  full sequence can issue several of these back-to-back (cleanup remove, branch check via
  go-git — not subject to this timeout, it's a local ref read — then `worktree add -b`), each
  independently budgeted at 30s, not against a shared 30s wall-clock budget for the whole
  `Setup()` call.

So the two timeouts are **orthogonal, not additive or interacting**: the lock's 30s only
gates queueing behind another process; the git-subprocess 30s only gates one subprocess call.
Neither is scaled for CI load anywhere in this codebase — confirmed by Agent 1's stack.md,
which notes the tmux-session-creation timeout (`STAPLER_SQUAD_TMUX_CREATE_TIMEOUT_SECONDS`)
*is* widened for CI (`build.yml:206`) but no equivalent widening exists for
`runGitCommand`'s fixed 30s.

**Structural consequence — this is the key finding**: because `WithRepoWorktreeLock`'s `mu`
is a plain `sync.Mutex` with no timeout on the *held* critical section, two goroutines calling
`Setup()` (or `SetupLocked()` under an already-held lock) for the same `repoPath` are **fully
serialized end-to-end**, not merely rate-limited. The second caller's `l.mu.Lock()` blocks
until the first caller's entire `setupLocked()` — including any self-heal fallback it took —
has returned. By the time the second caller's `setupLocked()` runs its own `branchRefExists`
check, the first caller's branch (if it created one) already exists, so the second call routes
straight into `setupFromExistingBranch()` and never reaches `setupNewWorktree()`'s `worktree
add -b` at all. **The exact interleaving
`TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate` constructs — two
concurrent `git worktree add -b <same-branch>` subprocesses genuinely racing each other — is
structurally unreachable through any `Setup()`/`SetupLocked()`-mediated production call.** This
isn't an empirical "haven't seen it happen" claim; it follows directly from `sync.Mutex`
semantics plus the caller-list audit in §1.

## 3. Is the test itself legitimate, and where should the fix live?

**The test's design is legitimate, but by construction it cannot be validating a
production-reachable scenario** — those are two separate questions and both are true at once.

- It's legitimate unit-test design in the narrow sense that the self-heal fallback code
  (`setupNewWorktree`'s "already exists" catch at `worktree_ops.go:336` and
  `setupFromExistingBranch`'s "already checked out"/"already used by worktree" catch at
  `worktree_ops.go:136`) is otherwise **unreachable from any covered entry point** given §2's
  finding — without a test that calls the unlocked method directly and forces the race by
  hand, this defensive code has zero coverage. The test's own doc comment shows the author
  already reasoned through this distinction (contrasting itself with
  `TestSetup_SerializesConcurrentWorktreeCreation_When_MultipleGoroutinesRaceOnSameRepo`,
  which uses the locked, public `Setup()` specifically to prove the *serialization itself*
  holds). That's the correct pattern: one test proves the lock prevents the race in
  production, a second test proves the defensive fallback still behaves if the race is forced
  synthetically.
- But "unreachable via this app's own `Setup()`/`SetupLocked()` calls" is not the same as "the
  fallback code is worthless" — it's still real defense-in-depth against an actor *outside*
  this app's lock discipline (a human running `git worktree add` by hand in the same repo
  while a session is being created, another tool, a future internal caller that's added
  without going through `Setup()`). Keeping the fallback, and a test that exercises it, is
  correct. The question this task poses is narrower: given the fallback is legitimately
  defensive-only from this app's own production traffic's point of view, does its *test*
  need a fix, and where.

Evaluating the requirements doc's three options directly:

- **(a) Leave the test unlocked-direct-call, harden the fallback's error matching.**
  Not the right target. Per Agent 1's stack.md, the flake's proximate trigger is very likely a
  genuine `runGitCommand` subprocess timeout under CI CPU contention (`-race`-instrumented,
  `t.Parallel()`-fanned-out, unwidened 30s budget, 2-vCPU runner) — an error whose text will
  not contain "already exists", "already checked out", or "already used by worktree" no matter
  how the matching is broadened, because a timeout kills the subprocess before git emits any
  of that stderr. "Hardening" the matching to also swallow timeout-shaped errors as if they
  were a race-loss would be actively wrong: a genuine timeout doesn't tell you *which* outcome
  occurred (branch created? worktree registered? neither?), so blindly falling into
  `setupFromExistingBranch()` on a timeout could attach to a nonexistent branch or a
  half-initialized worktree — trading a correctly-loud hard failure for a silently-wrong
  self-heal. The fallback's current conservatism (only swallow the exact errors that mean "I
  definitely lost a legitimate race, the winner definitely succeeded") is correct and
  shouldn't be loosened to paper over a timeout.
- **(b) Wrap the test's calls in `WithRepoWorktreeLock` too.** Also wrong, for the reason the
  requirements doc already anticipates: per §2, wrapping the calls in the lock would fully
  serialize them exactly like production `Setup()` does — the second call would then always
  observe `branchExists == true` and never reach `setupNewWorktree()`'s race window at all.
  This doesn't fix the flake; it deletes the only coverage this defensive code has, silently
  turning a real (if narrow) test into a no-op. It would also make this test redundant with
  `TestSetup_SerializesConcurrentWorktreeCreation_...`, which already covers the locked path.
- **(c) Something else — fix belongs at the test/CI-environment layer, not in
  `worktree_lock.go` or in the fallback's matching logic.** Given the root cause is CI-load
  timing against a fixed, unscaled 30s subprocess budget — not a lock gap (§1/§2 show there
  isn't one reachable from production) and not a matching-logic gap (§3(a) shows loosening it
  would trade correctness for flake-suppression) — the fix should target the thing that's
  actually under-provisioned for the environment it runs in: `runGitCommand`'s fixed 30s
  timeout has no override seam today (confirmed: it's a literal inside the function body,
  `worktree_git.go:38`, not a package var/const or field on `*GitWorktree`, unlike the
  `commandRunner` field which already has a working injection seam via `WithCommandRunner`/
  `tmux.CommandRunner` used elsewhere in this package for test doubles). This repo has direct
  precedent for CI-only timeout widening without touching the underlying business logic
  (`STAPLER_SQUAD_TMUX_CREATE_TIMEOUT_SECONDS`, per Agent 1's stack.md) — the analogous shape
  here is a small, test/CI-scoped widening of the git-subprocess budget (e.g. an
  env-var-driven override or an injectable timeout, defaulting to the current 30s in
  production), not a change to `WithRepoWorktreeLock` or to the self-heal string matching.
  This keeps the fix scoped to the actual root cause (test environment timing) while leaving
  the two pieces of production logic that were already independently confirmed correct —
  the lock's full serialization guarantee, and the fallback's conservative error
  classification — untouched.

## Summary of architectural shape for the fix

- **Confirmed**: every production call path into `setupNewWorktree()`/`setupFromExistingBranch()`
  is `Setup()`- or `SetupLocked()`-mediated, hence `WithRepoWorktreeLock`-serialized. No
  unlocked production caller exists (full grep audit in §1).
- **Confirmed structurally** (not just empirically): the lock's plain, timeout-free
  `sync.Mutex` makes the two-concurrent-`worktree-add-b` race this test constructs
  unreachable via any `Setup()`/`SetupLocked()` call — the second caller always observes the
  first's completed branch/worktree state, never a live race, because `mu` blocks for the
  entire critical section, not just lock acquisition (§2).
- **AC-3 resolution**: this is a **test-only artifact**, not a production-reachable gap. The
  self-heal fallback it exercises is legitimate defense-in-depth (protects against non-`Setup()`
  actors), and the test's choice to call the unlocked private method directly is the only way
  to give that fallback any coverage — that design should be kept, not "fixed" into
  redundancy with the lock-serialization test.
- **Root cause** (per Agent 1's stack.md, confirmed consistent with this doc's concurrency
  analysis): CI-load-induced `runGitCommand` subprocess timeout (fixed 30s, not scaled for CI
  anywhere in this codebase, unlike the tmux-creation timeout precedent) producing an error
  that matches none of the fallback's deliberately narrow string checks — correctly, since a
  timeout is not evidence of a race-loss.
- **Where the fix belongs**: neither `worktree_lock.go` (already correct/sufficient for every
  production path) nor the fallback's error-matching in `worktree_ops.go` (already correctly
  conservative — loosening it would mask a real failure mode). The fix belongs in test/CI
  environment provisioning — most concretely, making `runGitCommand`'s 30s budget
  overrideable (env var or injectable field, defaulting to today's 30s) so this test can run
  with headroom under CI's CPU-contended, `-race`-instrumented conditions, following this
  repo's own existing precedent for CI-scoped timeout widening elsewhere.
