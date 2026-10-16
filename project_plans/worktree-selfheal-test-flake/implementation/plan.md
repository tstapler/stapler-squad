# Implementation Plan: worktree-selfheal-test-flake

**Feature**: Close the root cause of `TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate`'s
CI flake by replacing the self-heal fallback's `strings.Contains` stderr matching with
ground-truth git-state re-query, plus deterministic regression coverage and a documented
local-reproduction attempt.
**Date**: 2026-08-22
**Status**: Ready for implementation
**ADRs**: [ADR-001-ground-truth-requery-over-stderr-matching.md](../decisions/ADR-001-ground-truth-requery-over-stderr-matching.md)

---

## Step 0.5 — Alternatives considered

1. **Add a third `strings.Contains` literal for `"signal: killed"`.** Strength: trivial,
   one line. Weakness: only chases the one string this investigation happened to observe —
   a future git wording change or non-English locale reopens the identical gap (the file's
   own doc comment already shows this happened once before). Rejected.
2. **Widen `runGitCommand`'s fixed 30s timeout for CI**, following the
   `STAPLER_SQUAD_TMUX_CREATE_TIMEOUT_SECONDS` precedent. Strength: real, in-repo precedent;
   reduces the odds of hitting the gap under load. Weakness: pure symptom fix per AC-4 — it
   changes probability, not correctness, and does nothing for the independently-confirmed
   locale gap. Rejected as the fix; superseded once ground-truth re-query is in place
   (see below), since re-query is correct regardless of whether the underlying call timed
   out.
3. **Replace stderr matching with ground-truth re-query** (re-check `branchRefExists` /
   `worktree list --porcelain` after any `worktree add` failure, instead of inferring the
   outcome from error text). Strength: closes the entire class — timeout-killed, locale-
   translated, and future-git-wording variants are all handled identically, because the
   decision no longer depends on error text at all; reuses existing in-repo helpers and the
   `getHeadCommitSHA` retry idiom, no new abstraction. Weakness: touches two call sites
   instead of one string list, and needs a small bounded retry to tolerate a still-in-flight
   winner. **Chosen** — see ADR-001.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| Self-Heal Fallback | The existing two-layer recovery logic in `setupNewWorktree`/`setupFromExistingBranch` (`session/git/worktree_ops.go`) that lets a caller who lost a worktree/branch-creation race reuse the race winner's result instead of hard-failing. | Pre-existing name from the code's own comments; not renamed. |
| Ground-Truth Re-Query | The replacement decision mechanism this plan introduces: after any `worktree add` failure, re-verify actual git state (`branchRefExists` via go-git, or `findWorktreeForBranch` via `git worktree list --porcelain`) instead of pattern-matching the failure's error text. | New concept for this plan; use this exact phrase in code comments so future readers can grep for the rationale (ADR-001). |
| `worktreeAddRetryAttempts` / `worktreeAddRetryDelay` | New local retry-bound constants in `session/git/worktree_ops.go`, mirroring `headSHARetryAttempts`/`headSHARetryDelay` (`session/git/util.go:285-288`), bounding how long Ground-Truth Re-Query waits for a still-in-flight race winner to finish before giving up. | Same values as the existing precedent unless testing shows otherwise: 3 attempts, 20ms delay. |
| `gitSpyCommandRunner` | Existing test double (`session/git/worktree_git_test.go:730-761`), injectable via `WithCommandRunner`. Its `runFunc` hook fires at the exact moment a "subprocess" would run, letting a test inject a side effect (e.g. creating a branch for real) at that instant. | Not new; reused for the new regression tests in Phase 2. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| `setupNewWorktree`'s `worktree add -b` failure handling | Ground-Truth Re-Query: bounded retry over `branchRefExists` (go-git, in-process, no subprocess) | ADR-001; existing `getHeadCommitSHA` retry idiom (`util.go:283-329`) | Add a third `strings.Contains` literal for `"signal: killed"` | Chases one observed string; a future git version, locale, or new failure shape reopens the same gap — doesn't close the class per AC-4 |
| `setupNewWorktree`/`setupFromExistingBranch` self-heal trigger, in general | Ground-Truth Re-Query (as above) | pitfalls.md §4, features.md §5 (supersedes architecture.md §3(c) — see reconciliation note below) | Widen `runGitCommand`'s fixed 30s timeout (CI-scoped, `STAPLER_SQUAD_TMUX_CREATE_TIMEOUT_SECONDS` precedent) | Pure symptom fix per AC-4 — reduces the probability of hitting the gap without changing what happens when it's still hit; superseded because Ground-Truth Re-Query is correct regardless of whether a timeout occurred, so the extra knob has no remaining job to do |
| `setupFromExistingBranch`'s `worktree add` (no `-b`) failure handling | Ground-Truth Re-Query: unconditional `worktree list --porcelain` + `findWorktreeForBranch` lookup on any failure | ADR-001; existing `worktreeAlreadyRegisteredForBranch` idiom (`worktree_ops.go:189-220`) | Keep the `"already checked out"`/`"already used by worktree"` OR-gate, just add more literals over time | Same class-vs-instance problem as above; also accidentally under-covers a real self-healable case today (a stale-directory `"already exists"` message reaching this call site isn't matched by either current literal here, per features.md §2) |
| Flaky test's own structure (`t.Parallel()`, unlocked direct call) | Keep as-is | architecture.md §3(b) | Wrap the test's two `setupNewWorktree()` calls in `WithRepoWorktreeLock` | Would fully serialize the two goroutines exactly like production `Setup()` does, deleting the only coverage the Self-Heal Fallback has and making the test redundant with `TestSetup_SerializesConcurrentWorktreeCreation_...` |
| Locale determinism for git subprocess stderr (`LC_ALL=C`/`LANG=C`) | Documented as a follow-up, not implemented in this fix | pitfalls.md §2 | Set `LC_ALL=C`/`LANG=C` inside `LocalRunner.Run` (`session/tmux/command_runner.go:81-85`) as part of this fix | `LocalRunner.Run` is shared by every tmux and `gh` CLI call site in the codebase, not just git — forcing a locale override there is broader blast radius than this bug's scope. Ground-Truth Re-Query already removes this bug's dependency on error-text content, so the locale gap is real but no longer load-bearing here; tracked separately (see Unresolved Questions) |
| Flake reproduction tooling | Reuse `golang.org/x/tools/cmd/stress` (go-stress), already CI-adopted | build-vs-buy.md §2, stack.md §1/§3 | Bare `go test -count=N` loop only | Doesn't pack comparable CPU-contention amplification into a fixed budget; repo already has a working go-stress precedent (`build.yml:400-438`, committed two commits prior on this branch) |
| Regression test injection mechanism | `gitSpyCommandRunner` + `runFunc` hook (existing test double) | features.md §5 | New live-timing test that actually waits out a real 30s subprocess timeout | Slow (30s+/run) and still probabilistic against real load; the existing spy gives a deterministic, fast, CI-safe regression test using this repo's established seam |

---

## Reconciliation with `research/architecture.md` §3(c)

`research/architecture.md`'s own concurrency analysis (§3, options a/b/c) reaches a different
top-level recommendation than this plan: it concludes the fix "belongs in test/CI environment
provisioning — most concretely, making `runGitCommand`'s 30s budget overrideable... **not** a
change to `WithRepoWorktreeLock` or to the self-heal string matching" (§3(c), emphasis added),
and separately argues (§3(a)) that *loosening* the fallback's error matching to also swallow
timeout-shaped errors would be actively unsafe — a timeout alone doesn't reveal whether a race
winner actually succeeded, so treating it as an automatic "self-heal" signal could silently
attach a session to a nonexistent branch or a half-initialized worktree.

Both of architecture.md's objections are correctly aimed at the specific option it evaluated —
broadening/loosening the `strings.Contains` matching to also treat unrecognized errors as
race-losses — but they do not apply to Ground-Truth Re-Query, which this plan chose instead
(ADR-001; not evaluated as an option in architecture.md, which only considered (a) harden the
string match, (b) lock the test, (c) widen the timeout). Re-Query never infers the outcome from
error text at all: it independently re-verifies real git state (`branchRefExists` /
`findWorktreeForBranch`) before deciding to self-heal, so an ambiguous, timeout-shaped, or
locale-translated error can no longer produce a silently-wrong heal — the exact failure mode
architecture.md §3(a) warns against is what Re-Query is designed to prevent, not what it risks
causing. Re-Query therefore supersedes architecture.md §3(c)'s CI-timeout-widening
recommendation for the reason already given in Step 0.5/alternative 2 above (a timeout widening
is a probability change, not a correctness fix, and does nothing for the independently-confirmed
locale gap) — this note exists so a future reader comparing the two documents sees the divergence
as a considered, cited decision rather than an unexplained contradiction.

---

## Migration Plan
N/A — no schema or data changes. Pure code-path change inside two unexported functions plus
new test files; no persisted state, config, or API shape changes.

## Observability Plan
- **Logs**: existing `log.Info("branch already exists (lost a concurrent create race)...")`
  and `log.Info("branch is already checked out, attempting to locate existing worktree"...)`
  call sites (`worktree_ops.go:337`, `:138`) are kept, just moved to fire after the
  Ground-Truth Re-Query confirms a heal rather than after a string match — no new log line
  needed, existing ones already describe the right event.
- **Metrics**: none added or needed; this is an unexported internal fallback with no
  existing metric surface.
- **Alerts**: none — CI's own `test`/`test-race` job failure is the existing alerting
  mechanism for a regression here.

## Risk Control
- **Feature flag**: none — this is a control-flow correctness fix inside two unexported
  functions with existing test coverage; no user-facing behavior changes, so no flag is
  warranted.
- **Rollback procedure**: revert the single commit touching `session/git/worktree_ops.go`
  (and the accompanying test files); the git-history diff is self-contained to that package.
- **Staged rollout**: N/A — internal library code, ships with the next normal PR merge/CI
  cycle like any other `session/git` change.

## Unresolved Questions
- **`LC_ALL=C`/`LANG=C` hardening for git subprocess stderr** (pitfalls.md §2): a real,
  confirmed, production-reachable gap independent of this bug's root cause (any non-English
  git locale silently defeats stderr-text-dependent logic anywhere in this file, including
  the out-of-scope `rev-parse HEAD` check at `worktree_ops.go:315-317`). Not implemented
  here because the only clean seam to set it (`LocalRunner.Run`,
  `session/tmux/command_runner.go:81-85`) is shared by every tmux and `gh` CLI call site,
  not just git — file a separate, appropriately-scoped follow-up backlog item before or
  after this fix ships, rather than bundling a broader-blast-radius change into this one.
- **`go-stress` not in `Makefile`'s `install-tools`** (build-vs-buy.md §2): local repro
  today requires copying the version pin (`v0.47.0`) out of `build.yml` by hand. Not
  required for any AC here; worth a follow-up if local stress-repro becomes a recurring need
  beyond this one investigation.

## Dependency Visualization

```
Phase 1 (root-cause fix)
  Task 1.1.1a (retry constants)
        │
        ▼
  Task 1.1.1b (setupNewWorktree: Ground-Truth Re-Query) ──┐
        │                                                  │
        ▼                                                  │
  Task 1.1.2a (setupFromExistingBranch: Ground-Truth Re-Query)
        │                                                  │
        ▼                                                  │
  Task 1.1.3 (doc comments: AC-2/AC-3 findings) ◄──────────┘
        │
        ▼
Phase 2 (validation & regression coverage)
  Task 2.1.1 (baseline stress repro, pre-fix — informational, can run
              in parallel with Phase 1 since it targets pre-fix HEAD)
        │
        ▼ (after Phase 1 lands)
  Task 2.2.1 (setupNewWorktree regression test)  ──┐
  Task 2.2.2 (setupFromExistingBranch regression   │
              test)                          ───────┤
        │                                            │
        ▼                                            ▼
  Task 2.1.2 (post-fix stress repro confirmation)   Task 2.3.1 (full verification: make build/test/lint)
```

---

## Phase 1: Root-Cause Fix

### Epic 1.1: Replace stderr-matching self-heal triggers with Ground-Truth Re-Query
**Goal**: `setupNewWorktree`/`setupFromExistingBranch`'s self-heal decision stops depending
on recognizing specific git error text and instead re-verifies actual git state after any
`worktree add` failure — closing the class of unrecognized-error-string gaps (timeout,
locale, future git wording) rather than the one instance observed on PR #583.

#### Story 1.1.1: `setupNewWorktree` re-queries `branchRefExists` instead of matching `"already exists"`
**As a** maintainer of the worktree self-heal fallback, **I want** the `worktree add -b`
failure path to re-check whether the branch actually exists rather than infer it from error
text, **so that** a timeout-killed, locale-translated, or future-git-wording failure still
self-heals correctly when the race was actually won by someone else, and still hard-fails
correctly when it wasn't.

**Acceptance Criteria**:
- AC-4 (closes root cause, not a blind timeout bump or assertion loosening): the self-heal
  decision no longer inspects `err.Error()` content at all.
  - *Given* the redesigned `setupNewWorktree`, *When* its `worktree add -b <branch>
    <headCommit>` call fails with `"signal: killed"` (a timeout-killed subprocess) **and**
    `branchRefExists(repo, branchRef)` confirms the branch now exists, *Then*
    `setupNewWorktree` delegates to `setupFromExistingBranch()` and returns no error —
    exactly as it does today for a matched `"already exists"` string, but now for any
    failure text.
  - *Given* the same failure, *When* `branchRefExists` confirms the branch still does **not**
    exist after `worktreeAddRetryAttempts` retries, *Then* `setupNewWorktree` returns the
    original hard error (`"failed to create worktree from commit %s: %w"`), preserving
    today's correct-hard-fail behavior for genuine failures (e.g. disk full).

**Files**: `session/git/worktree_ops.go`

##### Task 1.1.1a: Add `worktreeAddRetryAttempts`/`worktreeAddRetryDelay` constants (~3 min)
- **Pre-mortem addendum (P1, see `pre-mortem.md` #1)**: `headSHARetryAttempts`/
  `headSHARetryDelay` (`session/git/util.go:285-288`) size a retry loop that waits out an
  in-process go-git torn-read against a local file write — microsecond-scale. This retry
  instead needs to wait out a *concurrent `git worktree add` subprocess*, which per this
  plan's own root-cause finding (ADR-001, pitfalls.md §3) can run up to `runGitCommand`'s 30s
  timeout under the exact CI load that caused the original flake. Do not copy
  `3 attempts / 20ms` unexamined:
  - Run Story 2.1.1's baseline stress repro first (or in parallel) and capture how long the
    "winner" goroutine's `worktree add` actually takes to complete under contention in that
    run.
  - Size `worktreeAddRetryAttempts`/`worktreeAddRetryDelay` so the total retry window is a
    meaningful fraction of that observed contended-completion time (with backoff), not a
    fixed 60ms — document the chosen values' rationale in the doc comment instead of just
    citing the `getHeadCommitSHA` precedent by analogy.
  - If Story 2.1.1's repro doesn't reproduce contention long enough to observe this, default
    to a materially larger bound than 60ms (e.g. attempts × delay approaching low seconds,
    not milliseconds) and revisit once Story 2.1.2's post-fix repro data is in.
- Add a `const` block near `branchRefExists` (after line 32) in `session/git/worktree_ops.go`:
  `worktreeAddRetryAttempts`, `worktreeAddRetryDelay` (values per the sizing above), with a
  comment cross-referencing both the `getHeadCommitSHA` precedent (util.go:285-288, for the
  retry *idiom*) and the sizing rationale above (for why the *values* differ from it), plus
  ADR-001.
- Files: `session/git/worktree_ops.go`

##### Task 1.1.1b: Replace the `"already exists"` string match with Ground-Truth Re-Query (~5 min)
- In `setupNewWorktree` (`worktree_ops.go:329-341`), replace the
  `if strings.Contains(err.Error(), "already exists")` branch with: on *any* error from the
  `worktree add -b` call, loop up to `worktreeAddRetryAttempts` times (sleeping
  `worktreeAddRetryDelay` between attempts after the first) calling
  `branchRefExists(repo, branchRef)` (the same `repo`/`branchRef` already in scope from
  lines 282-288); on the first `true`, log
  `"branch already exists (lost a concurrent create race), reusing it for worktree"` (as
  today) and `return g.setupFromExistingBranch()`; if still `false` after all attempts (or
  `branchRefExists` itself errors), fall through to today's hard error at line 340 unchanged.
- Update the doc comment at lines 330-335 to describe Ground-Truth Re-Query instead of the
  git-version-wording rationale it currently cites (that rationale moves to ADR-001).
- Files: `session/git/worktree_ops.go`

#### Story 1.1.2: `setupFromExistingBranch` re-queries `findWorktreeForBranch` instead of matching two literal strings
**As a** maintainer of the worktree self-heal fallback, **I want** the `worktree add`
(no `-b`) failure path to unconditionally re-check `git worktree list --porcelain` for the
branch rather than gate on two specific English error strings, **so that** the same
timeout/locale/future-wording class of failures is closed at this second layer too.

**Acceptance Criteria**:
- AC-4: the fallback no longer gates on `"already checked out"`/`"already used by worktree"`.
  - *Given* the redesigned `setupFromExistingBranch`, *When* its `worktree add <path>
    <branch>` call fails with any error **and** `g.findWorktreeForBranch` (via a fresh
    `worktree list --porcelain`) finds the branch registered to some worktree path, *Then*
    `setupFromExistingBranch` adopts that path (as today) and returns no error.
  - *Given* the same failure, *When* `findWorktreeForBranch` does not find the branch
    registered anywhere, *Then* `setupFromExistingBranch` returns the original hard error
    (`"failed to create worktree from branch %s: %w"`), unchanged from today.

**Files**: `session/git/worktree_ops.go`

##### Task 1.1.2a: Replace the two-string OR-gate with an unconditional, retried Ground-Truth Re-Query (~8 min)
- In `setupFromExistingBranch` (`worktree_ops.go:131-164`), remove the
  `if strings.Contains(err.Error(), "already checked out") || strings.Contains(err.Error(),
  "already used by worktree")` gate at line 136 — run the existing lookup body (lines
  141-157: `worktree list --porcelain` → `g.findWorktreeForBranch` → canonicalize/adopt path)
  unconditionally whenever the preceding `worktree add` call returns any error, not just a
  matched one. Keep the existing "if we can't find the existing worktree, return the
  original error" fallback (line 159-160) and the final hard error (line 163) unchanged as
  the not-found path.
- **Pre-mortem addendum (P1, see `pre-mortem.md` #4)**: give this re-query the same bounded
  retry treatment as `setupNewWorktree`'s (Task 1.1.1b), not a single unconditional check —
  loop the `worktree list --porcelain` + `findWorktreeForBranch` lookup up to
  `worktreeAddRetryAttempts` times (sleeping `worktreeAddRetryDelay` between attempts) before
  falling through to the not-found path. Without this, `setupNewWorktree`'s layer tolerates
  an in-flight race winner but this second layer doesn't, reopening the identical
  under-CI-load flake one call deeper in the same fallback chain.
- **Pre-mortem addendum (P2, see `pre-mortem.md` #3)**: before adopting a path returned by
  `findWorktreeForBranch`, add the same `os.Stat` liveness check `worktreeAlreadyRegisteredForBranch`
  already applies (`worktree_ops.go:203-206`) — otherwise a stale/prunable `worktree list
  --porcelain` entry for the same branch name (left by an earlier crashed run) can get
  silently adopted as a live worktree when the real `worktree add` failure was actually a
  genuine, unrelated error (disk full, permission denied), masking it instead of surfacing
  it to the caller.
- Update the doc comment at lines 132-135 (the git-2.50.1-wording rationale) to reference
  Ground-Truth Re-Query / ADR-001 instead, since git's exact wording no longer gates
  anything here.
- Files: `session/git/worktree_ops.go`

##### Task 1.1.3: Update the self-heal fallback's top-level doc comments with AC-2/AC-3 findings (~3 min)
- Add or amend a short doc comment (3-5 sentences, per CLAUDE.md's proportionality rule —
  explain the code, not the investigation) above `setupNewWorktree` and/or
  `setupFromExistingBranch` recording: (1) every production caller reaches these functions
  only through `Setup()`/`SetupLocked()`, which serialize via `WithRepoWorktreeLock`, so the
  specific two-goroutine race the flaky test constructs cannot occur outside that test; (2)
  the fallback itself is still real defense-in-depth for a branch that exists for other
  reasons (manual `git branch`, a prior partial run); (3) point to ADR-001 for why the
  decision mechanism is Ground-Truth Re-Query, not stderr matching.
- Files: `session/git/worktree_ops.go`

---

## Phase 2: Validation & Regression Coverage

### Epic 2.1: Local reproduction (AC-1)
**Goal**: Determine, with an actually-executed command and a recorded outcome, whether the
flake reproduces outside CI — closing AC-1 with evidence rather than the existing static
analysis alone (features.md's direct repro confirmed the *error shape*, `"signal: killed"`,
but did not run the flaky test itself under amplified load).

#### Story 2.1.1: Baseline stress reproduction attempt against pre-fix code
**As a** developer investigating this flake, **I want** to run the established go-stress
CI idiom against the current (pre-fix) `TestSetupNewWorktree_SelfHeals_...` locally, **so
that** AC-1 is closed with a real command and outcome, not just static analysis.

**Acceptance Criteria**:
- AC-1: local reproduction is attempted and documented either way.
  - *Given* pre-fix `session/git` HEAD, *When* running
    `go test -race -c -o /tmp/worktree_ops.test ./session/git && timeout 90s stress -p
    $(nproc) /tmp/worktree_ops.test
    -test.run='TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate' || [
    $? -eq 124 ]`, *Then* the exact command and its outcome (a failure surfaced within the
    90s budget — reproduced locally — or a clean exit 124 for the full budget — did not
    reproduce locally, CI-specific) are recorded in this story's task output / the
    implementation session's notes, and referenced in the eventual PR description.

**Files**: none (shell-only task; no source files change). Uses the pre-installed
`golang.org/x/tools/cmd/stress@v0.47.0` binary (`go install
golang.org/x/tools/cmd/stress@v0.47.0` if not already on `$PATH`).

##### Task 2.1.1a: Run the baseline stress repro and record the outcome (~5 min)
- `go install golang.org/x/tools/cmd/stress@v0.47.0` (if not already installed).
- `go test -race -c -o /tmp/worktree_ops.test ./session/git`
- `timeout 90s stress -p $(nproc) /tmp/worktree_ops.test
  -test.run='TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate' || [
  $? -eq 124 ]`
- Record the outcome (reproduced / not reproduced within budget) verbatim for AC-1.
- Files: none.

#### Story 2.1.2: Post-fix stress reproduction confirmation
**As a** developer shipping this fix, **I want** to re-run the same stress command against
the fixed code, **so that** the fix's effect on the flake's reproducibility is directly
observed, not assumed.

**Acceptance Criteria**:
- AC-1 (closure): the fix's effect is directly observed.
  - *Given* post-fix `session/git` HEAD (Phase 1 complete), *When* re-running the identical
    command from Task 2.1.1a, *Then* the run completes clean for the full 90s budget (exit
    124) — if Story 2.1.1 reproduced the flake locally, this is a direct before/after
    confirmation; if it didn't, this run's clean result is recorded as consistent with (not
    proof of) the fix.

**Files**: none.

##### Task 2.1.2a: Run the post-fix stress repro and record the outcome (~5 min)
- Rebuild: `go test -race -c -o /tmp/worktree_ops.test ./session/git` (post-fix).
- Re-run: `timeout 90s stress -p $(nproc) /tmp/worktree_ops.test
  -test.run='TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate' || [
  $? -eq 124 ]`
- Record the outcome for the PR description / AC-1 closure note.
- Files: none.

### Epic 2.2: Deterministic regression tests (AC-5)
**Goal**: Add fast, deterministic tests — using the existing `gitSpyCommandRunner` test
double — that would have caught the original failure mode without depending on real CI
timing, per `.claude/rules/fix-flaky-tests-dont-defer.md`'s worked-example standard.

#### Story 2.2.1: Regression test for `setupNewWorktree`'s Ground-Truth Re-Query
**As a** maintainer, **I want** a deterministic test proving `setupNewWorktree` self-heals
on an unrecognized error string when the branch was actually created by a race winner, **so
that** this failure mode can never silently regress back to a hard error.

**Acceptance Criteria**:
- AC-5: a regression mechanism exists that would have caught the original failure.
  - *Given* a real `setupTestRepo(t)` repo and a `GitWorktree` constructed with
    `WithCommandRunner(spy)` where `spy.runFunc`, on the call matching `["worktree", "add",
    "-b", branchName, ...]`, first creates `branchName` for real via a direct git command
    against the repo (simulating the race winner completing at that instant) and then
    returns `("", errors.New("signal: killed"))`, *When*
    `TestSetupNewWorktree_SelfHeals_When_WorktreeAddFailsWithUnrecognizedError` calls
    `wt.setupNewWorktree()`, *Then* the call returns no error (Ground-Truth Re-Query finds
    the branch now exists and delegates to `setupFromExistingBranch`), which the pre-fix
    code would have failed (the `"signal: killed"` string matches neither of the old
    literals).

**Files**: `session/git/worktree_ops_test.go`

##### Task 2.2.1a: Write `TestSetupNewWorktree_SelfHeals_When_WorktreeAddFailsWithUnrecognizedError` (~5 min)
- Add the test described in Story 2.2.1's Given-When-Then to
  `session/git/worktree_ops_test.go`, placed near the existing
  `TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate` (after line 373).
  Use `gitSpyCommandRunner`'s `runFunc` hook (inspecting `spy.runCalls[len(spy.runCalls)-1]`
  for the matching `worktree add -b` call) to create the branch via a direct
  `safeexec.CommandContext(ctx, "git", "-C", repoDir, "branch", branchName)` call at the
  moment the "subprocess" would run, then return the killed-signal error.
- **Pre-mortem addendum (P2, see `pre-mortem.md` #5)**: also add a second case (or a second
  test) where branch creation is deliberately *delayed* until the retry loop's second or
  later `branchRefExists` check (e.g. count calls to `branchRefExists` via a test hook or a
  package-level counter reset per test, and only create the branch once the count reaches
  2) — the immediate-success variant above never actually exercises the retry loop's
  looping/backoff, so it wouldn't catch an off-by-one or early-return bug in the loop itself.
- Files: `session/git/worktree_ops_test.go`

#### Story 2.2.2: Regression test for `setupFromExistingBranch`'s Ground-Truth Re-Query
**As a** maintainer, **I want** a deterministic test proving `setupFromExistingBranch`
self-heals on an unrecognized error string when the winner already registered the worktree,
**so that** this failure mode's second layer can never silently regress either.

**Acceptance Criteria**:
- AC-5 (second layer): a regression mechanism exists for the `worktree_ops.go:131-164` path.
  - *Given* a `GitWorktree` constructed with `WithCommandRunner(spy)` where `spy.runFunc`
    returns `("", errors.New("signal: killed"))` for the call matching `["worktree", "add",
    g.worktreePath, g.branchName]` (no `-b`) and a synthesized `git worktree list
    --porcelain`-shaped string (reporting `g.branchName` checked out at a distinct simulated
    "winner" path) for the call matching `["worktree", "list", "--porcelain"]`, *When*
    `TestSetupFromExistingBranch_SelfHeals_When_WorktreeAddFailsWithUnrecognizedError` calls
    `wt.setupFromExistingBranch()`, *Then* the call returns no error and `g.worktreePath` is
    updated to the simulated winner's path — which the pre-fix code would have failed (the
    `"signal: killed"` string matches neither of the old literals).

**Files**: `session/git/worktree_ops_test.go`

##### Task 2.2.2a: Write `TestSetupFromExistingBranch_SelfHeals_When_WorktreeAddFailsWithUnrecognizedError` (~5 min)
- Add the test described in Story 2.2.2's Given-When-Then to
  `session/git/worktree_ops_test.go`, near the same location as Task 2.2.1a. `spy.runFunc`
  branches on the last recorded call's args (`worktree add` vs. `worktree list
  --porcelain`) to return the two different scripted responses.
- Files: `session/git/worktree_ops_test.go`

### Epic 2.3: Full verification
**Goal**: Confirm the fix and its new tests integrate cleanly with the rest of the suite and
this repo's required checks before handoff.

#### Story 2.3.1: Run the repo's standard verification gate
**As a** developer shipping this fix, **I want** `make build`, `make test`, and `make lint`
to pass with the new code and tests in place, **so that** the fix is provably ready for
review, not just locally plausible.

**Acceptance Criteria**:
- All ACs (integration confirmation): the fix compiles, passes existing and new tests, and
  passes lint.
  - *Given* Phase 1 and Epic 2.2's changes committed, *When* running `make build && make
    test && make lint` (or `make quick-check`), *Then* all three succeed, including the two
    new regression tests from Epic 2.2 and the existing
    `TestSetupNewWorktree_SelfHeals_When_ConcurrentSpawnsRaceOnBranchCreate` and
    `TestSetup_SerializesConcurrentWorktreeCreation_When_MultipleGoroutinesRaceOnSameRepo`.

**Files**: none (verification only).

##### Task 2.3.1a: Run `make quick-check` and resolve any failures (~5 min)
- Run `make quick-check` (build + test + lint). If a failure is unrelated pre-existing debt,
  apply `.claude/rules/fix-flaky-tests-dont-defer.md`'s standard: root-cause and fix it in
  this session, or file it immediately as its own tracked bug — do not silently route around
  it.
- Files: none (fixes, if any, land in whichever file the failure points to).
