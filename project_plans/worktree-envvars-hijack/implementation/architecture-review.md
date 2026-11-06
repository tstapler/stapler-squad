# Architecture Review: worktree-envvars-hijack
**Date**: 2026-09-24 (re-review, iteration 2)
**Verdict**: CONCERNS

## Constitution Violations
- No constitution file found — skipped.

## Blockers

None. The prior Blocker 1 (pre-spawn guards ran after the `claude` subprocess
was already spawned) is **verified closed**, by direct read of current
source, not by trusting the plan's own claim:

- `session/instance.go`'s `startLocked` (the actor-safe body `Instance.Start()`
  calls synchronously) has `basePath = i.gitManager.GetWorktreePath()` at
  exactly line **1599**, `i.initTmuxSession()` at exactly line **1607**, and
  `i.pm().Start(startPath)` at exactly line **1654** — the plan's cited line
  numbers are byte-accurate, not stale. The proposed guard insertion point
  (after the `if i.gitManager.HasWorktree() {...}` block closes at 1600, before
  1607) genuinely runs before any tmux/process spawn.
- The `setupErr`/`defer` cleanup block (lines 1511-1518) is registered before
  the `firstTimeSetup` branch (1585) executes, so a `setupErr` set at the new
  guard's insertion point correctly triggers the existing `i.Kill()` →
  `Destroy()` cleanup chain — which also handles worktree cleanup (per
  `Destroy()`'s doc comment and the `destroyChainTimeout` comment naming
  `CleanupWorktree`). No new leak path.
- `wireCallbacks(p.instance)` runs at `session_creation_pipeline.go:252`,
  strictly before `p.instance.Start(true)` at `:258` — confirmed by direct
  read. `SessionTypeNewWorktree` sessions are created exclusively through this
  pipeline (`CreateDirectorySession`/`CreateWorktreeSession` hardcode
  `SessionTypeDirectory`/`SessionTypeExistingWorktree` respectively, so they
  never reach the new guard's `SessionType == SessionTypeNewWorktree` check
  regardless of their own wiring order). Task 3.3.2b's plan to add
  `SetPreSpawnCollisionGuard` into `wireCallbacks` therefore genuinely fires
  before `Start()` for every session the guard applies to.
- The dependency direction holds: `Instance.SetPreSpawnCollisionGuard(func(string) error)`
  is a domain-defined port (mirrors the real, already-used shape of
  `wireCallbacks`'s other injections — `SetTaggingEngine`, rate-limit/status-change/
  claude-session-ID callbacks, `SetMCPServerURLProvider`, all wired from
  `server/services` before `Start()`); `startLocked` only ever calls the
  injected function, never imports `server/services` or reads
  `s.reviewQueuePoller` directly.
- Epic 2/Epic 3 independence, previously undeclared: now genuinely true. The
  relocated guard reads only `basePath`/`i.Path`, both locals internal to
  `startLocked` — no dependency on the pipeline's `instanceRootDir` variable
  Epic 2 fixes.

## Concerns

- [ ] **Epic 4.4 / Epic 4.5 (Story 4.4.1, Story 4.5.1) — the stated test mechanism ("swap `gitManager` for a test double implementing the mockable `GitManager` interface") does not exist and cannot compile as described; the plan's own Glossary entry backing it is factually wrong.**

  Verified: `Instance.gitManager` (`session/instance.go:592`) is declared as
  the **concrete struct type** `GitWorktreeManager` (`session/git_worktree_manager.go:28`),
  not the `GitManager` interface the plan's Glossary cites at
  `session/git_worktree_manager.go:511`. That interface has **zero other
  usages anywhere in the codebase** (`grep -rn "GitManager\b"` outside its own
  declaration file returns nothing) — it is dead/unwired, not "already
  mockable in tests" as claimed. A field declared as a concrete struct type
  cannot hold an interface-implementing test double; existing tests that
  populate `gitManager` (`session/instance_test.go:42,76`,
  `session/review_queue_uncommitted_changes_test.go:84,259,324`) all do so via
  a literal `GitWorktreeManager{worktree: ...}` value, confirming this.

  This is fixable, but not the way either story currently describes it:
  - **Epic 4.4** (`session/instance_test.go`, same package) can still
    construct a real `GitWorktreeManager{worktree: &git.GitWorktree{...}}`
    struct literal directly — the existing in-package precedent already does
    this — so its goal (force `GetWorktreePath()`/`HasWorktree()` to a chosen
    value) is achievable, just not via an "interface test double."
  - **Epic 4.5** (`server/services/session_creation_pipeline_test.go`, a
    *different* package) cannot reach `gitManager` at all — it's an
    unexported field. This is not a hypothetical problem: it's already been
    solved once, for the exact same purpose. `server/services/session_service_worktree_guard_test.go:31-48`
    (`newWorktreeGuardTarget`) forces an artificial worktree-path collision for
    an existing `RefuseIfWorktreeSharedWithOtherLiveSession`-style guard test
    via the already-exported `Instance.SetGitWorktree(*git.GitWorktree)`
    (`session/instance_worktree.go:540`) + `git.NewGitWorktreeFromStorage(...)`
    (`session/git/worktree.go:193`) — its own doc comment states almost
    exactly this finding: *"GitWorktreeManager is a value-typed field, not an
    interface, so its zero value is usable, and SetGitWorktree only touches
    that field."*

  **Recommendation**: rewrite Epic 4.4/4.5's Acceptance Criteria and Task text
  to use `inst.SetGitWorktree(git.NewGitWorktreeFromStorage(repoPath, colliding-or-distinct-path, title, branch, sha))`
  (matching `newWorktreeGuardTarget`'s established pattern) instead of a
  nonexistent interface test double. The underlying test goal is fully
  achievable — this is a plan-text correction, not a redesign — but as
  written, an implementer would burn time on a mechanism that doesn't
  compile before finding the real seam. Worth fixing before Epic 4.4/4.5 are
  implemented, since these two epics are what closes the previous review's
  Concern 1 (guard's `blocked=true` branch had no test coverage) — if their
  described mechanism is followed literally, that coverage gap reopens.

- [ ] **Pattern Decisions table — `Instance.SetStatusManager` is cited as a "wired by SessionService before Start()" precedent; it is actually wired *after* `Start()` everywhere it's called, for a documented reason.**

  Verified: every real call site —
  `server/services/session_creation_pipeline.go:307-312`,
  `server/services/session_service.go:1703-1710` (`CreateDirectorySession`),
  and `:1769-1776` (`CreateWorktreeSession`) — calls
  `instance.SetStatusManager(s.statusManager)` **after** the adjacent
  `instance.Start(true)` call, each with a matching comment: *"Wire the status
  manager and start the controller AFTER Start() returns so the tmux
  attach-session process has had time to fully initialize. Starting the
  controller inside Start() caused immediate PTY EIO..."* — i.e., this is a
  deliberate, opposite-direction ordering choice, not an example of
  before-`Start()` wiring.

  This does **not** undermine the actual mechanism the plan chose — `wireCallbacks`
  (a different function) genuinely is wired before `Start()` everywhere, as
  verified above under Blockers — so the guard's own correctness doesn't
  depend on this citation. But the citation is the wrong example, in the same
  table cell this re-review was asked to verify claim-by-claim, and a future
  reader relying on it to justify some other late-wired callback's safety
  would be relying on a false premise. **Recommendation**: replace the
  `SetStatusManager` citation with `wireCallbacks` itself, or with
  `ProcessManager.SetOnExitCallback` (genuinely wired inside `startLocked` at
  `session/instance.go:1496`/`:1753`, which the plan also cites correctly).

## Nitpicks
- The previous review's Nitpick (guard checks accumulating in the already-large
  `runBackgroundResolutionPipeline`) is effectively moot post-relocation: Epic
  3.3's checks no longer land in that function at all (only Task 3.3.3a's
  small error-classification change remains there). Not explicitly
  called out as "resolved" anywhere in the plan text, but no action needed —
  noting it here so the thread doesn't need re-deriving.
- Epic 1.4.2's plan to reuse "`SetPreSpawnCollisionGuard`'s companion port" for
  the `DetectByPath` ownership check is appropriately hedged ("likely reusing
  or extending") rather than asserted as settled — the actual data need
  (which live instance, if any, owns a given path) is a different shape than
  the pass/fail collision guard, so this will likely need its own port when
  Task 1.4.2a is implemented. Fine to leave underspecified at plan granularity
  since Story 1.4.1's audit gates whether any implementation is needed at all.
