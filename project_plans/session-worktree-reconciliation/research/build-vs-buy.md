# Build vs. Buy — session-worktree-reconciliation

Agent 6, SDD research phase. Scope: is any external dependency justified for the
periodic sweep that detects `sessions` ↔ `worktrees` row drift and validates worktree
state on disk, or does this repo's existing internal tooling already cover it.

## 1. Reconciliation-loop framework vs. hand-rolled ticker sweep

**Option A — adopt a "desired state controller" library** (e.g.
`sigs.k8s.io/controller-runtime`, or lighter alternatives like informer-less
reconcile-loop helpers).

- Pros: standardized `Reconcile(ctx, req) (Result, error)` shape, built-in
  requeue/backoff semantics, work-queue deduplication.
- Cons:
  - controller-runtime is built around a Kubernetes API server as the source of
    truth — watches, informers, a `client.Client` backed by REST + a local cache,
    leader election for multi-replica HA. None of that exists here: this is a
    single Go process reading its own local SQLite/ent DB. Adopting it would mean
    standing up a fake/adapter API-server-shaped layer just to satisfy the
    library's assumptions — pure translation overhead, no behavior gained.
  - `go.mod` has zero `k8s.io/*` or controller-runtime dependencies today
    (verified: `grep -n "go-git\|controller-runtime\|k8s.io" go.mod` returns only
    the two existing `go-git` lines). Introducing that dependency tree (client-go,
    apimachinery, etc.) for one sweep is a large, ongoing maintenance/CVE surface
    for a feature that's ~100-300 lines of plain Go in the existing style.
  - This repo already has 4 working, tested instances of the pattern it would
    replace: `session.ReconcileOrphanedTmuxSessions` (`session/orphan_sweep.go:46`),
    `session.ReconcileSuspendedProcesses` (`session/import_reconcile.go:34`),
    `(*BacklogLifecycleListener).reconcileStaleWorkSessions`
    (`session/backlog_lifecycle_stale.go:64`), and `(*SessionRetentionSweeper).sweep`
    (`server/services/session_retention_sweeper.go:70`) — each `time.NewTicker` +
    list-candidates + best-effort-fix-or-log, none blocking their caller. Adding a
    5th sweep in a structurally different style (a new framework's `Reconciler`
    interface) fragments the codebase's own established convention rather than
    extending it.
- **Verdict: reject.** Wrong problem shape (single-process/local vs.
  distributed-desired-state), and it doesn't need capabilities the current
  approach lacks (all 4 existing sweeps handle retries, partial failure, and
  logging fine without a framework).

**Option B — hand-rolled ticker sweep matching existing style.**

- Pros: matches 4 existing precedents exactly (reviewers, on-call, and future
  maintainers already know the shape); zero new dependency; trivially testable
  the same way the existing 4 are (inject fakes, call the sweep function
  directly, assert on results — no framework mocking needed).
- Cons: none specific to this feature — it's the same tradeoff the codebase has
  already made 4 times and lives with.
- **Verdict: adopt.** This is squarely the "extends an existing pattern" case
  the requirements doc (`project_plans/session-worktree-reconciliation/requirements.md:9`)
  already calls out as Complexity 2-3, "no new subsystem."

## 2. Git worktree validation — wrap go-git/shell out, or reuse `session/git`

The repo already has exactly the two primitives ask #1 needs, both internal:

- **"Is this directory a registered git worktree?"** → `session/git`'s
  `nativeListWorktrees(repoPath string) ([]NativeWorktreeEntry, error)`
  (`session/git/native_worktree_list.go:40`). Its own doc comment
  (`native_worktree_list.go:11-13`) states it is "the pure-Go replacement for
  parsing `git worktree list --porcelain` output" (Epic 2.3, Story 2.3.1) — it
  reads `.git/worktrees/<name>/gitdir` and `HEAD` directly via `os.ReadDir`/file
  reads rather than shelling out and parsing text. It's already called from
  three call sites (`session/git/worktree.go:492`,
  `session/git/native_worktree_prune.go:20`, `session/git/worktree_ops.go:383`),
  and already computes a `Prunable` classification
  (`native_worktree_list.go:28-31`) for "WorktreePath no longer exists on disk."
  It is unexported (package-private) — the new sweep either needs an exported
  wrapper added to `session/git`, or (more likely, per the existing call sites'
  pattern) a small exported helper like `git.ListWorktrees`/`git.WorktreeState`
  that the sweep package calls, same shape as its 3 existing internal callers.
- **"Does `base_commit_sha` resolve in this repo?"** → `session/git.CommitInfo`
  (`session/git/ops.go:480-496`), which does `OpenRepo(repoPath)` (`util.go:43`,
  go-git-backed, the mandated entry point per `norawgitopen`/
  `prefer-go-git-over-subshells`) then
  `repo.CommitObject(plumbing.NewHash(sha))`, returning a wrapped error if the
  hash doesn't resolve. This is precisely "is `base_commit_sha` resolvable,"
  already built on go-git's structured API, no porcelain-parsing involved.

Both are answerable from *existing internal code* — no new go-git usage pattern,
no new subshell, no new library. "Build" here literally means: call
`session/git`'s existing functions (exporting one currently-unexported helper if
needed) from the new sweep. `go-git` itself (`github.com/go-git/go-git/v5
v5.19.2`, `go.mod:30`) is already a first-class dependency; nothing new is added
to `go.mod`.

**Verdict: reuse `session/git` as-is (plus one export, if `nativeListWorktrees`'s
result type is needed outside the package). No shell-out, no new dependency.**

## 3. Correctness risk: hand-parsed porcelain output vs. structured API vs. simple ent query

Three distinct pieces, three different risk profiles:

- **Git worktree listing** — real correctness risk *was* here, and the codebase
  already paid down that risk: `nativeListWorktrees`'s doc comment explicitly
  frames it as replacing a `git worktree list --porcelain`-parsing predecessor
  (`native_worktree_list.go:13`, "Epic 2.3"). Porcelain-format parsing is
  notoriously fragile (format has changed across git versions, embedded
  whitespace/locking annotations, etc.) — this is exactly the class of problem
  `.claude/rules/norawghrequest.md`'s reasoning generalizes ("compiles, passes
  tests on the happy path, silently wrong on the edge case"). The new sweep
  should use `nativeListWorktrees`/its structured `NativeWorktreeEntry`, never
  re-parse `git worktree list` output itself.
- **Commit-SHA resolution** — already on go-git's typed `CommitObject` API
  (`ops.go:485`), not string-matching command output. Reusing `CommitInfo`
  (or a thin “does it resolve” wrapper around the same call) carries no
  parsing risk.
- **The anti-join query** (sessions with no matching worktree row) — genuinely
  low-risk, ordinary ent usage. The schema already models this as a plain edge:
  `Session` → `edge.To("worktree", Worktree.Type)` (optional, not `.Required()`)
  at `session/ent/schema/session.go:181`, with `Worktree` → `edge.From("session",
  Session.Type)` `.Required()` at `session/ent/schema/worktree.go:34`. Ent's
  generated client exposes this as a standard predicate
  (`session.HasWorktree()` / `session.Not(session.HasWorktree())`) — no raw SQL,
  no bespoke join logic, and it's exactly the shape of query the existing
  `EntRepository` methods already write throughout `session/ent_repository*.go`.
  This part is trivially safe to hand-write; it doesn't need a library or even
  extra scrutiny beyond a normal ent query review.

**Verdict:** no part of ask #1 requires new bespoke parsing. The one place that
historically *did* carry parsing risk (worktree listing) was already fixed by
this codebase before this project started; the new sweep just needs to consume
that fix, not redo the risky version.

## 4. Fork an existing sweep almost verbatim?

Comparing shapes:

| Sweep | Candidate list source | Fix/flag action | Structural fit for #1 |
|---|---|---|---|
| `ReconcileOrphanedTmuxSessions` (`orphan_sweep.go:46`) | `tmux list-sessions` vs. in-memory `[]*Instance` | kill tmux session | Low — external-process reconciliation, not DB/git-state |
| `ReconcileSuspendedProcesses` (`import_reconcile.go:34`) | `SuspendedProcessStore.List()` vs. `InstanceStore.ListInstanceData()` | SIGCONT + remove record | Medium — one-shot startup-only pattern (not periodic), and single boolean "is it still managed" check, not multi-field validation |
| `reconcileStaleWorkSessions` (`backlog_lifecycle_stale.go:64`) | `ListBacklogItems` (in_progress) → `ListItemSessions` → staleness check | `MarkStuck` + notify, optional `StaleWorkRemediator.RemediateStaleWorkSession` | **Highest** — this is a periodic ticker sweep over live sessions, does a per-item multi-condition check (`staleWork(lastProgress, now, maxNoProgress)`), and on finding a problem it **flags via `MarkStuck` (visible operator-facing signal), with an optional injected remediator interface for auto-repair** — this is exactly the two-tier "auto-repair when unambiguous, else flag" shape the requirements doc (`requirements.md:83-88`) asks for. |
| `(*SessionRetentionSweeper).sweep` (`session_retention_sweeper.go:70`) | `ListInstanceDataWithWorktree()` → per-candidate multi-check (`sessionSafeToDelete`) | `DeleteSession` RPC | Medium-high — good model for "eager-load the worktree edge up front" (`storage.go`'s `ListInstanceDataWithWorktree`, referenced at `session_retention_sweeper.go:81`, already exists and is the exact eager-load the new sweep needs to avoid the `LoadMinimal` gap called out in that file's own comment at lines 77-80) and for "byUUID snapshot once per cycle" batching. |

**Verdict:** don't fork one file wholesale (each existing sweep's fix-action is
domain-specific and wouldn't transplant cleanly), but the new sweep should
closely mirror `reconcileStaleWorkSessions`'s *control-flow skeleton*
(ticker → list candidates → per-candidate multi-field check → flag via the
existing `MarkStuck`/notify pipe, with an optional injected "repair" interface
for the unambiguous-fix case) and reuse `SessionRetentionSweeper`'s eager-load
call, `storage.ListInstanceDataWithWorktree()` (`session_retention_sweeper.go:81`),
directly rather than re-deriving it — that call already loads exactly the
`Worktree` edge data the anti-join/validation checks need, avoiding a second
new query. This keeps net-new code to the sweep's own detection logic and the
one already-scoped ent anti-join, minimizing `dupl`/duplication-gate exposure
under `make ready-complexity-gate`.

## Overall recommendation

The stated expectation — **build using existing internal packages, no new
external dependency** — holds and is confirmed, not just assumed:

- No reconciliation/controller framework exists in `go.mod` today, and adopting
  one (controller-runtime-style) would be a poor fit for a single local process
  with no distributed state (Section 1).
- Git worktree/commit validation primitives already exist in `session/git`
  (`nativeListWorktrees`, `CommitInfo`/`OpenRepo`), already on go-git, already
  free of the porcelain-parsing risk the codebase deliberately eliminated
  (Section 2, 3).
- The sessions↔worktrees anti-join is a standard ent query against an edge the
  schema already models (Section 3).
- The sweep's control-flow skeleton should be adapted from
  `reconcileStaleWorkSessions` (flag-with-optional-auto-repair shape) and reuse
  `SessionRetentionSweeper`'s existing eager-load query, not forked verbatim
  from any single file (Section 4).

Net new surface for planning: the sweep's own ticker/loop wiring, the per-field
consistency checks (missing worktree row / unresolvable `repo_path` /
unresolvable `base_commit_sha`), one possibly-new exported wrapper in
`session/git` if `NativeWorktreeEntry`/`nativeListWorktrees` needs to be called
from outside the package, and the notify/flag wiring. No new dependency in
`go.mod`.
