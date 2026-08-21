# Pitfalls Research: Session Completion Summary

Agent 4 (Pitfalls) research for `project_plans/session-completion-summary/requirements.md`
(backlog `59bbff11-ee8b-418c-8484-64307cb14244`). All line refs are to this worktree's
checked-out tree; use `git log -1 --format=%H` locally to pin a SHA before publishing links.

## 1. Race: concurrent/duplicate generation triggers (FR-7)

**What breaks.** FR-1 triggers generation on both `EventExited` and `EventStopped`, and the
requirements doc already flags that these fire from multiple independent sources for what is
logically the *same* session end:

- `session/instance.go:811` — `EventExited` fires from the instance's own exit-detection path
  (reason e.g. `"pty-eof"`).
- `session/instance.go:1247` — `Destroy()` fires `EventStopped` unconditionally via `defer`
  (reason `"operator-destroy"`).
- `session/review_queue_poller.go` (`reconcileSessions`, ~line 451) — the reconciler fires
  `EventExited` with reason `"reconcile-session-missing"` whenever it finds a managed session
  `Active` in memory but missing from live tmux — **a duplicate exit signal for a session that
  may have already been destroyed and already had `EventStopped` fired for it once.**
- `session/backlog_lifecycle.go:804-809` treats `EventExited` and `EventStopped` identically
  (comment cites BUG-027: "a deliberate operator stop... ends the session exactly as much as an
  unexpected exit does — downstream reconciliation must not depend on which one happened").

If the summary generator is wired the same way (fire-and-forget `go` per event, per
`.claude/rules/go-double-checked-locking.md`'s "read-lock → cache miss → compute → write-lock →
conditional store" pattern), two or three of these events can each independently observe "no
summary row yet for this session_id" and each kick off a full generation pipeline (LLM call +
diff read + persistence write). Concretely: goroutine A (from `EventExited`/pty-eof) and
goroutine B (from the reconciler's `reconcile-session-missing` a few seconds later) both read
`summaryExists(sessionID) == false` before either has written its row, both call the LLM, both
attempt to persist. Depending on the persistence write's uniqueness handling this either (a)
produces two summary rows for one session, (b) causes a duplicate-key write failure that gets
misreported as an ERROR state despite generation actually succeeding once, or (c) — worst case —
the *second* writer's (possibly poorer, since spurious reconcile-fired generation may run with a
worktree that's already been deleted, see §2) result silently overwrites the first, per the
double-checked-locking rule's warning about "another goroutine's result... contradicts the
current goroutine's own computation."

**Canonical safe pattern in this repo.** Two complementary patterns already exist and should be
composed, not reinvented:

1. `.claude/rules/go-double-checked-locking.md` — always return the locally-computed value from
   a write-locked conditional-store, never re-read the shared slot afterward. Applies to the
   in-memory "is a generation already running for session X" guard.
2. `session/git/worktree_git.go`'s `IsDirtyWithHint` (~line 226, referenced by the rule file)
   wraps its slow path in `singleflight` — this repo already uses
   `golang.org/x/sync/singleflight` for exactly the "collapse concurrent callers for the same
   key into one in-flight computation" problem (also used in `github/client.go`,
   `github/user_pr_cache.go`, `server/services/search_service.go`,
   `session/unfinished/gogit_vcs_reader.go`, `session/tmux/tmux.go`). A `singleflight.Group`
   keyed on `session_id` is the idiomatic fit for FR-7: concurrent `EventExited`/`EventStopped`
   fires *and* repeated manual "Regenerate" clicks for the same session collapse onto one
   in-flight generation, and late callers get the same result rather than starting their own.
3. Durable idempotency still needs a persisted state machine (e.g. `PENDING → GENERATING →
   READY|ERROR`) written with a conditional/compare-and-swap style update (only transition into
   `GENERATING` if the row doesn't exist or is in a terminal state) — `singleflight` alone only
   protects a single process's in-memory concurrency, not two events landing far enough apart
   that the first's goroutine has already exited (e.g. `pty-eof` at T+0, reconciler's
   `reconcile-session-missing` at T+30s after the poller's next tick). The persisted-state check
   is the actual FR-7 guarantee; `singleflight` is an optimization on top that avoids redundant
   LLM calls when multiple events land within the same short window.

## 2. Race: worktree cleanup vs. async diff read (the one race the requirements doc flags for research)

**Confirmed: `CleanupWorktree()` runs synchronously, *before* `EventStopped` fires.**
`session/instance.go`:

```go
func (i *Instance) Destroy() error {
	defer i.fireLifecycleEvent(EventStopped, "operator-destroy")   // line 1247

	...
	if err := i.KillSession(); err != nil { errs = append(errs, err) }
	if err := i.CleanupWorktree(); err != nil { errs = append(errs, err) }   // line ~1268, runs first
	return i.combineErrors(errs)
}   // defer fires EventStopped only now, after worktree is already gone
```

`CleanupWorktree()` (`session/instance_worktree.go:186`) calls `i.gitManager.Cleanup()`, which
removes the actual worktree directory on disk. Because the `defer` fires *after* the function
body completes, **by the time any `EventStopped` listener (including a new completion-summary
generator) observes the event, the worktree directory is already deleted.**

This directly collides with `SessionService.GetSessionDiff`
(`server/services/session_service.go:2586`), which the requirements doc names as the diff-stat
source. It has two paths:

- **Live instance found via `findInstance`** (`server/services/session_service.go:3254`, checks
  `reviewQueuePoller` then `externalDiscovery`): calls `instance.UpdateDiffStats()` then
  `instance.GetDiffStats()`. `UpdateDiffStats()` (`session/instance_worktree.go:216`) explicitly
  "performs I/O (git diff) outside the lock" — i.e. it re-runs `git diff` against the worktree
  path at call time, it does not just return a pre-computed cached value. If the instance has
  already been evicted from whatever registry `findInstance` checks (unclear from `Destroy()`
  alone; `Destroy()` doesn't appear to remove the instance from the poller's list itself — that
  eviction is a separate concern), this path re-runs `git diff` against a now-deleted directory.
- **No live instance found → "completed session" fallback**: reconstructs a `GitWorktree` from
  the *stored* `found.Worktree.WorktreePath` (from `session.InstanceData`, i.e. persisted state)
  and calls `wt.Diff()` directly against that path on disk. This is the fallback the async
  generator will almost certainly hit once the instance ages out of the live poller list — and
  it unconditionally assumes the worktree directory still exists at that path.

**Net risk:** if generation is kicked off from the `EventStopped`/`EventExited` handler and reads
the diff asynchronously (even a `go func()` a few hundred ms later), it races its own trigger's
worktree cleanup and will very plausibly read an empty/error diff for a session whose diff was
in fact fully available a moment earlier — silently producing an incomplete summary for a
*non-trivial* session (this is worse than the FR-6 empty-session case because it looks
indistinguishable from FR-6's legitimate "explicit empty-state text" and gives no operator
signal that data was lost). `EventStopped` (`Destroy()` path) is the one guaranteed to race;
`EventExited` (natural process exit, `pty-eof`) does *not* go through `CleanupWorktree()` at all
in the exit path itself, so worktree availability there depends on whatever later step calls
`Destroy()`/cleanup — needs a design decision, not an assumption, since AC/FR text doesn't say
teardown order is guaranteed.

**Design implication for planning:** diff capture (and any other worktree-dependent data:
decisions breakdown may be fine since it comes from `NotificationHistoryStore`/`ApprovalHandler`
keyed on `session_id`, not the worktree) must happen either (a) synchronously *before*
`CleanupWorktree()` runs inside `Destroy()` — i.e. hook diff-stat capture into the teardown
sequence itself, ahead of the git worktree removal, not after — or (b) rely entirely on an
already-persisted `DiffStats` value captured earlier in the session's life (note requirements
doc flags `DiffStats`'s current `Required()` cascade-tied edge to `Session` as "the WRONG
pattern for this feature" since it needs to survive Session-row deletion — a *decoupled* snapshot
of diff stats, captured pre-cleanup and persisted independently, resolves both problems at once).
Fetching diff stats reactively from `GetSessionDiff` after the fact is the pitfall to design
around, not the solution.

## 3. LLM narrative pitfalls

- **Cost/latency on every session end, including trivial ones.** FR-6 requires "a valid
  non-error document with explicit empty-state text" for minimal-activity sessions. Read
  literally, this is a strong signal to **skip the LLM call entirely** for sessions with no
  diff, no approval decisions, and negligible timeline/token activity, and go straight to the
  deterministic fallback — not just handle the LLM's *failure* gracefully but avoid *invoking*
  it. This matters at scale: every `EventExited`/`EventStopped` firing an LLM call unconditionally
  (including short-lived one-off sessions, aborted sessions, and the reconciler's spurious
  `reconcile-session-missing` re-fires from §1) turns a passive lifecycle hook into an
  uncapped LLM spend surface. A cheap pre-check (diff line count + approval count + duration
  threshold) before deciding whether to call the LLM at all is the natural reading of FR-5 +
  FR-6 together, not just FR-5's "narrative failures degrade gracefully" framing alone.
- **Timeout handling.** FR-5 requires generation to be async/non-blocking for teardown, but
  doesn't set an explicit LLM timeout. Without one, a hung LLM call can leave a session's
  generation stuck in `GENERATING` indefinitely (compounds with §5, the restart-recovery
  pitfall) and, if `singleflight`-style coalescing is used (§1), blocks every other concurrent
  request for that same key until it resolves. Needs an explicit context deadline shorter than
  whatever "stuck" detection threshold Regenerate/restart-recovery uses.
  There's precedent for the "warn if a background goroutine is running unexpectedly long"
  pattern already in this codebase — `logSlowShutdown` in `server/services/session_service.go`
  (~line 2570s) waits on a `sync.WaitGroup` with a one-time warn-after log line rather than
  giving up, which is the wrong pattern to copy here (it never times out) but the "warn on slow"
  telemetry idea is reusable.
- **Hallucinated narrative content.** A "what was done" summary generated from a diff + timeline
  is a classic hallucination surface — the model can assert changes, rationale, or outcomes not
  actually present in the retrieved diff/decision data (e.g. claiming a test was added when it
  wasn't, or inventing a reason for an approval decision). The prompt must be strictly grounded:
  pass only the retrieved diff stat/content, approval decisions, and timeline as structured
  context, instruct the model not to speculate beyond what's given, and treat the narrative as
  strictly additive/descriptive prose over that data — never as the sole source for any
  structured field (diff numbers, decision counts, cost) that FR-2 also requires, all of which
  must come from the deterministic/retrieved data path regardless of what the LLM says.

## 4. Persistence pitfalls

- **ent generation flag.** Any new ent schema for the durable summary document (and its
  independent-persistence requirement per FR-3/AC-3) must be regenerated with
  `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`
  (see `.claude/rules/ent-schema-generation.md` and `session/ent/generate.go`). Omitting
  `--feature sql/upsert` compiles fine but silently breaks `Upsert*` methods — directly relevant
  here since idempotent regeneration (FR-7) plausibly wants an upsert-on-conflict write (create
  row if absent, overwrite if a Regenerate click supersedes a prior ERROR/READY row) rather than
  strict create-then-error-on-duplicate.
- **Don't repeat the `DiffStats` mistake.** The requirements doc explicitly calls out
  `DiffStats`'s `Required()` edge to `Session` (`session/ent/schema/diffstats.go:26-33`) as the
  wrong pattern because it cascade-deletes with the parent `Session` row, defeating AC-3
  ("survives Session-row deletion"). The correct precedent is `AnalyticsEvent`
  (`session/ent/schema/analytics_event.go:25`, a plain `field.String("session_id")` with an
  index, no FK edge) and `NotificationHistoryStore` (`server/notifications/store.go:36`, also a
  plain string `SessionID` field) — key the summary schema off a bare `session_id` string field
  + index, not an ent edge to `Session`.
- **Partial-write risk.** FR-2 requires several distinct pieces of data in one document
  (narrative, diff stat, decisions, timeline, token cost) that plausibly come from different
  subsystems (LLM output, `GetSessionDiff`, `NotificationHistoryStore`/`ApprovalHandler`,
  `session/tokens` pricing). If these are written as separate calls/rows rather than one
  transactional write, a mid-pipeline failure (e.g. document text succeeds, token-usage snapshot
  write fails) leaves a row that's neither cleanly READY nor cleanly ERROR — exactly the
  ambiguous state FR-5's "deterministic-stage failures surface an ERROR state" is trying to
  avoid. Assemble the full document object in memory first, then persist it in a single
  transaction/single upsert write with one status transition at the end, rather than
  incrementally writing sub-fields as each data source resolves.

## 5. Restart pitfall: generation stuck "in progress" forever

Nothing in the codebase's existing async-job patterns models a durable "in-flight work survives
a process restart" recovery step for this kind of job (the closest analogues —
`NotificationHistoryStore`, `AnalyticsEvent` — are write-once-then-read stores, not stateful
pipelines with a `GENERATING` intermediate state). If the server restarts (or the whole tmux
server + all sessions get rebuilt from scratch — see
`.claude/rules/tmux-keep-server-on-restart.md`, a live-confirmed failure mode in this exact repo)
while a summary is mid-`GENERATING`, the persisted row is left in that state with no process
left to ever transition it out. Without an explicit reconciliation step, this is indistinguishable
from a summary that's legitimately still running, and the UI would need to poll/wait forever (or
worse, silently show nothing).

**Needed:** a startup reconciliation pass (natural fit alongside the existing
`ReviewQueuePoller.reconcileSessions()` pattern, which already does "scan persisted state vs.
live reality and fix up inconsistencies" for sessions) that finds any summary row stuck in
`GENERATING` older than some staleness threshold at process start and transitions it to `ERROR`
(with a reason like `"generation-interrupted-by-restart"`, mirroring the existing
`"reconcile-session-missing"` reason-string convention) — surfacing the Regenerate action per
FR-5 rather than leaving it silently stuck. A generation-started timestamp on the row is required
input for this (staleness check needs an age, not just a status enum value).

## 6. UI pitfalls

- **Copy-to-clipboard.** Repo already has `web-app/src/lib/clipboard.ts`'s `copyToClipboard()`
  helper: tries `navigator.clipboard.writeText`, falls back to a hidden-textarea +
  `document.execCommand("copy")` for contexts where `navigator.clipboard` is undefined (plain
  HTTP / non-secure-context LAN access — relevant since this is a self-hosted localhost/LAN tool,
  not always served over HTTPS). FR-4 should reuse this helper as-is rather than a bare
  `navigator.clipboard.writeText` call, and should surface both failure and success feedback
  (the helper returns a boolean; callers must check it — a `false` return with no UI feedback is
  a silent failure the user has no way to notice, especially over a plain-HTTP LAN session where
  the primary API is expected to be unavailable, not just an edge case).
- **Large-document rendering.** `react-markdown` is already a project dependency (used in
  `web-app/src/components/backlog/detail/DescriptionSection.tsx` and
  `web-app/src/app/help/page.tsx`) and is the natural fit for rendering the summary, including
  its embedded diff content. However, FR-2's diff content + FR-2's timeline for a long session
  can be large; `react-markdown` re-parses and re-renders its full children tree on every prop
  change, and a large embedded diff block (which FR-2 also wants export-ready as GFM, so likely
  wrapped in a fenced code block) is exactly the shape that can freeze the tab if rendered
  unvirtualized/un-truncated inside a scrolling detail view. Precedent to check: how
  `SessionDetailView.tsx`'s existing diff/log views (e.g. terminal scrollback, which is
  presumably large too) handle this today — likely virtualization or truncation-with-expand.
  Apply the same treatment to the summary's diff section rather than dumping the full diff into
  one `react-markdown` tree unconditionally; a truncate-with-"show full diff" pattern (consistent
  with the CSS architecture rule's page-scroll convention in
  `.claude/rules/css-architecture.md` for the new Summary tab's scroll container) keeps first
  render cheap.

## 7. General "async job triggered by a lifecycle event" pitfalls

- **Retry storms.** If Regenerate (FR-7) or an automatic retry-on-ERROR path exists, an
  unbounded retry loop against a failing LLM provider (rate limit, outage) could hammer the
  provider once per user click or once per some automatic interval — needs explicit backoff or
  at minimum a cooldown between allowed regenerations for the same session, not just the
  singleflight in-flight dedupe from §1 (that only prevents *concurrent* duplicates, not rapid
  sequential retries).
- **Unbounded goroutine growth.** A bare `go generateSummary(sessionID)` per lifecycle event,
  with no worker pool/queue bound, means a burst of session completions (e.g. many backlog items
  finishing around the same time, or the reconciler's `reconcileSessions()` sweep firing
  `EventExited` for several stale sessions in one poller tick — see §1) spawns that many
  concurrent LLM calls with no ceiling. The repo's existing async patterns
  (`session/backlog_lifecycle.go`'s `go il.parent.onSessionExited(...)`) are fire-and-forget with
  no pool; the completion-summary generator should not copy this pattern verbatim for LLM-backed
  work — a bounded worker pool or a queue with a concurrency cap is warranted given the added
  cost/latency profile of an LLM call vs. the cheap bookkeeping `onSessionExited` currently does.
- **Silent failures / no operator visibility.** FR-5's ERROR state + Regenerate action is the
  right user-facing recovery path, but nothing in the requirements calls for structured logging
  or metrics on generation failures. Given this is a fire-and-forget background trigger off a
  lifecycle event (easy to lose track of), failures should log with the same
  `log.ForSession(...)`/structured-field convention used elsewhere in `session/instance.go` (e.g.
  `log.Warn("failed to update diff stats", "session", ..., "err", ...)` pattern already used in
  `GetSessionDiff`) so a stuck-ERROR summary is discoverable in logs, not just invisible until a
  user happens to open the Summary tab.

## Key file/line references

- `session/instance.go:77-87` — `EventExited`/`EventStopped` definitions and comments.
- `session/instance.go:1246-1272` — `Destroy()`; confirms `CleanupWorktree()` runs before the
  deferred `EventStopped` fire.
- `session/instance_worktree.go:186-193` — `CleanupWorktree()`.
- `session/instance_worktree.go:216` — `UpdateDiffStats()` doc comment ("performs I/O (git diff)
  outside the lock").
- `session/backlog_lifecycle.go:804-811` — BUG-027 comment, `EventExited`/`EventStopped` treated
  identically.
- `session/review_queue_poller.go` (`reconcileSessions`, ~line 451) — `reconcile-session-missing`
  duplicate `EventExited` source.
- `server/services/session_service.go:2586-2646` — `GetSessionDiff`, live vs. completed-session
  fallback paths.
- `server/services/session_service.go:3254-3266` — `findInstance`.
- `session/git/worktree_git.go:222-240` — `IsDirtyWithHint`, canonical
  cache+singleflight+double-checked-locking pattern referenced by
  `.claude/rules/go-double-checked-locking.md`.
- `session/ent/schema/diffstats.go:26-33` — the `Required()` cascade edge to avoid replicating.
- `session/ent/schema/analytics_event.go:25` — plain `session_id` string field, the pattern to
  follow instead.
- `server/notifications/store.go:36` — same plain-`SessionID` pattern.
- `web-app/src/lib/clipboard.ts` — existing `copyToClipboard()` helper to reuse for FR-4.
- `web-app/package.json:87` — `react-markdown` already a dependency.
