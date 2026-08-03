# Implementation Plan: session-completion-summary

**Feature**: Automatically generate and durably persist a markdown "completion summary" (narrative, diff stats, approval-decision breakdown, timeline, token cost) when a stapler-squad session ends, retrievable via a Summary tab even after the session is deleted.
**Date**: 2026-08-03
**Status**: Ready for implementation
**ADRs**: [ADR-001](../decisions/ADR-001-completion-summary-generation-scope-and-storage.md) (Superseded), [ADR-002](../decisions/ADR-002-generation-trigger-scope-both-events-via-precleanup-snapshot.md) (Accepted — supersedes ADR-001 §1/§3)

---

## Step 0.5 — Alternatives Considered

**A. Fully synchronous generation inside the teardown critical path** (compute
diff/decisions/timeline/cost *and* call the LLM before `Destroy()`/the
`DeleteSession` RPC returns). *Strength*: zero race risk — nothing is deferred,
so nothing can disappear before it's read. *Weakness*: a multi-second LLM
round trip directly inside `Destroy()`/the exit callback violates FR-5 ("never
blocks or delays session teardown") and `LifecycleListener`'s documented
non-blocking contract (`session/instance.go:92`). Rejected.

**B. Fully lazy, read-time generation** (compute the summary on first
`GetSessionSummary` call, reading live `Session`/worktree/notification data at
request time). *Strength*: simplest possible implementation — no listener,
no dedup, no persisted status machine. *Weakness*: fails FR-3/AC-3 outright.
`research/architecture.md` §3 confirms `CleanupWorktree()` and
`storage.DeleteInstance` both complete before or shortly after the relevant
lifecycle event fires — by the time a user opens the tab, the worktree and/or
`Session` row may already be gone, so there is nothing left to lazily read.
Rejected.

**C. Two-stage pipeline (chosen): synchronous, in-memory-only snapshot
capture inside the lifecycle callback, async goroutine for everything
perishable-but-not-instant plus the LLM call, FR-7 dedup via an in-memory
per-session guard backed by a persisted status+staleness check for restart
recovery.** *Strength*: satisfies FR-5 (only a cached-field read — no I/O —
happens synchronously) while still beating every documented race, because the
data that would otherwise vanish (diff stats, in particular) is captured
*before* it can vanish, not read lazily later. *Weakness*: more moving parts
than A or B (two 1-line hook points in `session/instance.go`, a dedup layer,
a read-time staleness check) — accepted because A and B each fail a hard
requirement (FR-5 and FR-3 respectively) that C satisfies. See rejected
alternatives recorded per-component in the Pattern Decisions table below.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `SessionSummary` | ent entity persisting one completion-summary record per session, keyed by a plain (non-edge) unique `session_id` string. Survives `Session` row deletion by construction. | `session/ent/schema/session_summary.go` |
| `SessionSummaryStatus` | App-level Go string type + consts: `PENDING`, `GENERATING`, `READY`, `ERROR`. Stored as `field.String("status")` on the ent row (no ent enum — see Pattern Decisions). | `session/session_summary_types.go` |
| `DiffSnapshot` | In-memory/proto DTO: `FilesChanged`, `Added`, `Removed int`; `IsEmpty() bool`. Persisted as three flat ent int fields, not a JSON blob. | `session/session_summary_types.go` |
| `DecisionsSnapshot` | In-memory/proto DTO: `AutoApproved`, `ManuallyApproved`, `Denied`, `ReviewQueueResolved`, `StillOpen int`; `Total() int`; `Percent(n int) float64`. | `session/session_summary_types.go` |
| `TimelineSnapshot` | In-memory/proto DTO: `StartedAt`, `StoppedAt time.Time`; `Duration() time.Duration`. | `session/session_summary_types.go` |
| `CostSnapshot` | In-memory/proto DTO: `TotalTokens int64`; `EstimatedCostUSD float64`; `DataUnavailable bool` (distinguishes "genuinely zero tokens" from "cost data could not be read", per `research/ux.md` §4). | `session/session_summary_types.go` |
| `sessionSummaryListener` | Per-instance `LifecycleListener` shim (mirrors `instanceBacklogListener`). Captures the live `*Instance` pointer at `WireToInstance` time; `OnLifecycleEvent` does one fast in-memory read then dispatches `go`. Its `generator` field is typed as the narrow consumer-defined `summaryGenerator` interface below, not a concrete `*SessionSummaryGenerator` — so the listener's own unit tests (Task 1.1.2b) can substitute a channel-based fake with no real ent client. | `session/session_summary_listener.go` |
| `summaryGenerator` | Consumer-side interface, defined in `session/session_summary_listener.go` next to `sessionSummaryListener` (its only consumer), scoped to exactly what the listener calls: `type summaryGenerator interface { GenerateAndPersist(ctx context.Context, sessionUUID, sessionTitle string, createdAt time.Time, diffStats *git.DiffStats, reason string) }`. `*SessionSummaryGenerator` satisfies it structurally, with no explicit "implements" declaration (per `.claude/rules/interface-pollution-checklist.md`'s "define the interface where it's consumed"). | `session/session_summary_listener.go` |
| `SessionSummaryGenerator` | Domain-level orchestrator owning the `headless.PoolClient`, the ent client, and the FR-7 dedup map. `GenerateAndPersist(ctx context.Context, sessionUUID, sessionTitle string, createdAt time.Time, diffStats *git.DiffStats, reason string)` runs the full async pipeline; its method set satisfies the `summaryGenerator` interface consumed by `sessionSummaryListener`. | `session/session_summary_service.go` |
| `inFlight` | `sync.Map[string]*sync.Mutex` keyed by `session_id`, mirroring `UnfinishedWorkService.aiMu` (`server/services/unfinished_work_service.go:311-314`). Collapses concurrent/duplicate triggers per session via `tryAcquire`/`isInFlight` (Task 1.5.3a). | field on `SessionSummaryGenerator` |
| `staleGenerationTimeout` | `const = 5 * time.Minute`. Read-time staleness threshold: a row stuck in `GENERATING` older than this is treated as interrupted (server restart) and flipped to `ERROR` on next read. | `session/session_summary_service.go` |
| `FeatureKeySessionCompletionSummary` | New `headless.FeatureKey` const, distinct from the existing unused `FeatureKeySummarize` so per-feature session rotation doesn't mix narrative styles. | `session/headless/features.go` |
| `GenerateSessionCompletionNarrative` | `func(ctx, pool *Pool, snapshot NarrativeInput) (text string, err error)` — follows `DraftPRDescription`'s shape exactly. | `session/headless/features.go` |
| `isTrivialSession` | `func(diff DiffSnapshot, decisions DecisionsSnapshot, duration time.Duration) bool` — the LLM-skip threshold (FR-5+FR-6). | `session/session_summary_snapshot.go` |
| `RenderSessionSummaryMarkdown` | `func(session_title string, narrative string, fallbackUsed bool, diff DiffSnapshot, decisions DecisionsSnapshot, timeline TimelineSnapshot, cost CostSnapshot, diffLink string) string` — `strings.Builder`-based, GFM-valid. | `session/session_summary_markdown.go` |
| `SessionSummaryService` (RPC) | ConnectRPC handler implementing `sessionv1connect.SessionSummaryServiceHandler`. Reads directly from the ent `SessionSummary` table by `session_id` — never through `SessionService.findInstance`/`ListInstanceData`. **Dependency**: `RegenerateSessionSummary` additionally depends on `SessionService.FindLiveInstance` (Task 2.2.2a) to check whether the session's live `*Instance` still exists before deciding whether to refresh the diff snapshot or reuse persisted fields — this is the one place this service intentionally touches live-session state; `GetSessionSummary` never does. | `server/services/session_summary_service.go` |
| `SessionSummaryPanel` | Shared React component rendering a `SessionSummary` (narrative, sections, copy button, error/regenerate UI). Used both as the `SessionDetailView` tab content and on the standalone durable route. | `web-app/src/components/sessions/SessionSummaryPanel.tsx` |
| `useSessionSummary` | React hook: fetch + poll-while-GENERATING + `regenerate()` + `copy()`. Modeled on `useBacklogItemShipStatus.ts` (session-independent fetch) + a poll loop. | `web-app/src/lib/hooks/useSessionSummary.ts` |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Overall pipeline shape | Two-stage: sync in-memory snapshot capture + async goroutine for LLM/persist (Alternative C above) | `research/architecture.md` §1, §3 | (A) fully synchronous incl. LLM call; (B) fully lazy read-time generation | A blocks teardown (violates FR-5); B reads data that's already gone by request time (violates FR-3, confirmed race in architecture.md §3) |
| Diff-stat capture timing | Two 1-line `_ = i.UpdateDiffStats()` calls added ahead of existing calls in `session/instance.go` (`Destroy()` before `CleanupWorktree()`; `instanceOnExitCallback` before `fireLifecycleEvent(EventExited,...)`), then listener reads the now-fresh cache via `i.GetDiffStats()` (no I/O) | ADR-002 | Reading `GetSessionDiff`/`GitWorktree.Diff()` lazily inside the async goroutine | Confirmed race (`research/pitfalls.md` §2): `CleanupWorktree()` deletes the worktree directory before `EventStopped` fires, so a lazy re-read after dispatch would silently see an empty/errored diff |
| Trigger scope | Both `EventExited` (reason ≠ `reconcile-session-missing`) and `EventStopped`, same pipeline, no asymmetry | ADR-002 (supersedes ADR-001 §1) | ADR-001's original EventExited-only scoping | requirements.md FR-1/AC-1/AC-7 explicitly require both; FR-3 already requires Session-row-independent retrieval regardless, which removes ADR-001's original rationale for excluding `EventStopped` |
| ent persistence shape | New `SessionSummary` entity, plain (non-edge) unique `session_id` string field + index, flat scalar fields for diff/decisions/timeline/cost (no `field.JSON` of a custom struct) | `research/stack.md` (e), `research/features.md` §3 (AnalyticsEvent precedent); `session/ent/schema/diffstats.go` (flat-field precedent) | (1) `DiffStats`-style `edge.From(...).Unique().Required()` back to `Session`; (2) `field.JSON("diff_snapshot", session.DiffSnapshot{})` importing the parent `session` package into `ent/schema` | (1) ties referential integrity to a row that's deleted before the event fires (the exact bug ADR-001/ADR-002 exist to avoid). (2) No existing ent schema file imports a sibling non-`ent/schema` package for a JSON field default — every `field.JSON` use in this codebase is a builtin type (`map[string]string{}`, `[]string{}}`); flat ints/floats/times match `DiffStats`'s own established precedent of not nesting a custom struct, and sidestep evaluating an untested cross-package import into codegen-only `ent/schema` |
| ent status representation | `field.String("status")` + app-level `SessionSummaryStatus` Go type with an `IsValid()` method (mirrors `SessionType.IsValid()` per `.claude/rules/session-creation-registry.md`) | `research/stack.md` (e) | `field.Enum(...)` (ent's native enum field) | Zero existing `field.Enum` usage anywhere in `session/ent/schema/*.go` (confirmed via grep) — introducing the first one is a real convention deviation not justified by this feature alone; plain string matches every other status-like field in this schema set |
| Proto file layout | New dedicated `proto/session/v1/session_summary.proto` (service + request/response messages), `SessionSummaryStatus` enum added to the existing shared `proto/session/v1/types.proto` | `research/architecture.md` §2c; `unfinished.proto`/`headless.proto` precedent | Adding RPCs/messages directly into `session.proto` | Matches this repo's existing granularity convention exactly: `unfinished.proto`, `headless.proto`, `insights.proto`, `backlog.proto` are already split out from `session.proto`; and `ScanStatus` (a single-feature enum, same shape as `SessionSummaryStatus`) is already defined in `types.proto` rather than in `unfinished.proto` itself — `SessionSummaryStatus` follows that exact placement precedent |
| Markdown generation | Hand-built `strings.Builder`/`fmt.Sprintf` | `research/build-vs-buy.md` (1); `session/services/rule_prompt_builder.go`, `session/headless/features.go` `DraftPRDescription` | `text/template`/`html/template` | Zero templating-library usage anywhere in `server/`/`session/`; ~6 fixed sections doesn't warrant introducing one |
| LLM narrative call | Reuse `session/headless.Pool.CallBlocking` via a new `FeatureKeySessionCompletionSummary`, function shaped exactly like `DraftPRDescription` | `research/build-vs-buy.md` (2); `session/headless/features.go:274-290` | A dedicated Anthropic API client | No such client exists in this codebase (confirmed: no `go.mod` SDK entry, no `anthropic_ai_client.go`) — would duplicate `headless.Pool`'s already-solved problem |
| FR-7 in-process dedup | `sync.Map[string]*sync.Mutex` keyed by `session_id` (mirrors `UnfinishedWorkService.aiMu`) | `research/build-vs-buy.md` (4); `server/services/unfinished_work_service.go:311-314` | `golang.org/x/sync/singleflight.Group` | Both are valid per research; the mutex-map is chosen because Regenerate needs a *reject-if-busy* semantic (return current in-flight status, don't make the caller wait for the result), whereas `singleflight.Do` blocks the caller until the in-flight call returns — a mutex + `TryLock`-style check maps more directly onto "return in-flight status immediately" |
| FR-7 restart-survival dedup | Persisted `status` + `generation_started_at` fields; lazy staleness check performed on read (`GetSessionSummary`), not a background sweep | `research/architecture.md` §4; `research/pitfalls.md` §5 | A periodic `ReconcileStuck`-style background sweep (like `ReviewQueuePoller.reconcileSessions()`) | No FR implies a background-sweep SLA; a lazy check on the one RPC that reads the row is simpler and sufficient for this feature's v1 scope. **Accepted v1 gap**: a session whose Summary tab/route is never revisited stays silently stuck in `GENERATING` forever, since nothing ever calls `GetSessionSummary` to trigger `reconcileStaleness`. This knowingly does not fully satisfy the Success Metrics section's "100% of sessions... never silently nothing" goal for that specific never-revisited case — accepted as a v1 trade-off for a single-process self-hosted tool rather than silently omitted; a background sweep is the documented follow-up if this proves to matter in practice |
| Error-path field preservation | On any error-path upsert — the decisions-build failure (step 3) or the best-effort persist-failure fallback (step 6) — the upsert is field-scoped to `status`/`error_stage`/`error_message`/`generation_started_at` (plus, for step 3 only, the freshly-computed `DiffSnapshot`/`TimelineSnapshot` fields) — it never clears a prior successful generation's `markdown`/`narrative`/decisions fields | Architecture review concern (illegal-states/CQS review) | Full-row replace that nulls out prior content on every error write | Preserving the last-good document is better UX ("regeneration failed, showing last successful summary" — see UX design doc surface (d2)) and costs nothing extra since ent's `Update()`/`OnConflictColumns(...).Update(...)` only touches columns explicitly set |
| Bounded LLM concurrency | Reuse the single shared `deps.HeadlessPool *headless.Pool` instance (`server/dependencies.go:516-527`, already injected into `SessionService`/`BacklogService`/`ApprovalHandler`) — its `MaxConcurrentSessions` semaphore (`session/headless/pool.go:24-26`) already bounds every LLM-consuming feature server-wide | `research/architecture.md` §1b; `research/build-vs-buy.md` (4) | A separate worker-pool/queue bound specific to summaries | Every LLM call in this codebase already funnels through the one shared `Pool`; a second, feature-specific bound would be redundant and could under- or over-provision relative to the pool's existing, already-tuned limit |
| Retrieval route independent of live `Session` | New standalone Next.js route `web-app/src/app/sessions/[sessionId]/summary/page.tsx` rendering the shared `SessionSummaryPanel`, backed by `GetSessionSummary(session_id)` — fetches directly, no dependency on the Redux `allSessions` list or the `Session` proto object `SessionDetail`/`SessionDetailView` currently require as a prop | Confirmed via reading `web-app/src/components/sessions/SessionDetail.tsx:57-77` (requires a full `Session` prop, sourced from `useAppSelector(selectAllSessions)`) and confirmed no `web-app/src/app/sessions/[id]/` dynamic route exists today (`ls web-app/src/app/sessions/` shows only `new/`) | Making the existing Summary *tab* (inside `SessionDetailView`) work standalone by loosening its `Session`-prop requirement | `SessionDetailView` is a large (~700-line), heavily props-coupled component (terminal, VCS, files, browser tabs all assume a live session); reworking its prop contract to tolerate a fully-absent `Session` is high-risk/high-blast-radius for this feature. A small dedicated route reusing only the new `SessionSummaryPanel` (which itself only ever needed `sessionId`) is a 1-file addition with zero risk to the existing tabbed view |

---

## Migration Plan

- **Migration file**: generated automatically by `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` after adding `session/ent/schema/session_summary.go` (creates a `session_summaries` table; this project's ent setup auto-migrates on startup — confirm the exact call site during Task 1.2.2b, no manual SQL migration file needed for this ent-managed schema).
- **Reversibility**: additive-only (new table, no columns added to `sessions` or any existing table). Fully reversible by dropping the `session_summaries` table; no existing table/data is touched.
- **Zero-downtime strategy**: new table with no foreign key to `sessions` — safe to create at any point relative to a rolling restart; old binaries simply never write to it.
- **Rollback procedure**: revert the PR (schema file + generated `session/ent/` diff); the extra `session_summaries` table left behind by a rolled-back deploy is inert and can be dropped manually or left until the next migration pass — it has no edge/constraint pointing at any other table.

## Observability Plan

- **Logs**: `log.ForSession(instanceUUID)` (existing convention, e.g. `GetSessionDiff`) at each status transition (`PENDING→GENERATING`, `GENERATING→READY`, `GENERATING→ERROR`) and on LLM narrative failure (log the fallback substitution at `Warn`, not `Error` — it's expected graceful degradation per FR-5). Log the trivial-session LLM-skip decision at `Info` for auditability of cost control.
- **Metrics**: none required by any FR; if `AnalyticsStore`/`AnalyticsEvent` is already wired to a metrics sink elsewhere, emitting an `AnalyticsEvent` (`event_category: "session_summary"`, `event_name: "generated" | "generation_failed" | "narrative_fallback"`) is a natural low-cost addition reusing the existing analytics pipeline — optional, not blocking for v1 (flag as stretch task, not a required Story).
- **Alerts**: none — this is a best-effort background job with a visible in-UI ERROR/Regenerate affordance; no on-call alerting requirement stated in any FR.

## Risk Control

- **Feature flag**: none required — the feature is additive (new listener, new table, new tab) and degrades gracefully to "tab disabled" / "no row" if any part fails; no flag needed to gate rollout risk given FR-5/FR-6's built-in graceful-degradation design.
- **Rollback procedure**: revert the PR; `sessionSummaryListener` simply stops being registered on `WireToInstance`, no other code path depends on `SessionSummary` rows existing.
- **Staged rollout**: not applicable — single-process self-hosted app, no fleet/canary concept in this codebase.

## Unresolved Questions

None. Items (a)-(e) from the task brief and the three open questions flagged in `research/architecture.md`/`research/ux.md` are resolved above and in the tasks below:
- (a) diff-snapshot timing → Pattern Decisions row "Diff-stat capture timing" + Epic 1.1.
- (b) FR-7 dedup (both layers) → Pattern Decisions rows "FR-7 in-process dedup" / "FR-7 restart-survival dedup" + Epic 1.5 Story 1.5.3.
- (c) Summary tab reachability post-deletion → Pattern Decisions row "Retrieval route independent of live Session" + Epic 3.3.
- (d) LLM-skip threshold → `isTrivialSession` in Domain Glossary + Epic 1.4 Story 1.4.2 (concrete threshold: `diff.IsEmpty() && decisions.Total() == 0 && duration < 30*time.Second`).
- (e) Bounded concurrency → Pattern Decisions row "Bounded LLM concurrency".

---

## Dependency Visualization

```
Phase 1 — Backend Domain & Persistence
  1.1 Lifecycle Trigger & Sync Diff Snapshot
    1.1.1a (instance.go hooks) ─┐
    1.1.1b (types.go)            ├─▶ 1.1.2 (listener) ─▶ 1.5.3 (dedup) ─┐
  1.2 ent Schema                 │                                       │
    1.2.1 (session_summary_types.go, done above as 1.1.1b) ─▶ 1.2.2 (ent schema + generate) ─┐
  1.3 Deterministic Snapshot Assembly                                    │                    │
    1.3.1 (diff+timeline) ─┐                                             │                    │
    1.3.2 (decisions)       ├─▶ used by 1.5.2 ◀─────────────────────────┘                    │
    1.3.3 (cost)           ─┘                                                                 │
  1.4 Narrative                                                                                │
    1.4.1 (FeatureKey + prompt) ─▶ 1.4.2 (trivial-skip + fallback) ─▶ used by 1.5.2            │
  1.5 Markdown + Persist + Idempotency                                                         │
    1.5.1 (markdown renderer) ─▶ 1.5.2 (GenerateAndPersist, upsert) ◀── 1.2.2, 1.3.x, 1.4.x ───┘
    1.5.2 ─▶ 1.5.3 (dedup + staleness) ─▶ 1.1.2 wires into this

Phase 2 — RPC Layer (depends on 1.2.2 for entities, 1.5.2/1.5.3 for Regenerate)
  2.1.1 (proto) ─▶ 2.2.1 (GetSessionSummary handler) ─┐
                 └▶ 2.2.2 (RegenerateSessionSummary) ─┼─▶ 2.2.3 (server wiring)
                        ▲                              ┘
                        └── depends on SessionService.FindLiveInstance
                            (live-instance-gone fallback branch, Task 2.2.2a)

Phase 3 — Frontend (depends on Phase 2 proto/RPC being generated)
  3.1.1 (useSessionSummary hook) ─▶ 3.1.2 (SessionSummaryPanel) ─┬─▶ 3.2.1 (tab integration)
                                                                   └─▶ 3.3.1 (standalone route) ─▶ 3.3.2 (NotificationsPage fallback link)

Phase 4 — Registry & Tests (parallel with / after respective Phase 1-3 tasks)
  4.1 registry entries (after 2.1.1, 3.2.1)
  4.2 tests (after each corresponding implementation task)
```

---

## Phase 1: Backend Domain & Persistence

### Epic 1.1: Lifecycle Trigger & Synchronous Diff Snapshot
**Goal**: Fire the generation pipeline on both natural exit and explicit stop, with the diff stat captured before it can be lost to worktree cleanup or storage-row deletion (ADR-002).

#### Story 1.1.1: Pre-cleanup diff snapshot hooks + domain types
**As a** session-completion-summary pipeline, **I want** the diff stat captured before the worktree/session row can disappear, **so that** the deterministic snapshot is never silently empty due to a teardown race.
**Acceptance Criteria**:
- AC-1 (partial): *Given* an `Instance` with `UUID: "sess-123"` and a dirty worktree (`Added: 42, Removed: 7`), *When* `Destroy()` is called, *Then* `i.UpdateDiffStats()` runs before `i.CleanupWorktree()`, so `i.GetDiffStats()` returns `{Added: 42, Removed: 7}` even after `CleanupWorktree()` has deleted the worktree directory.
**Files**: `session/instance.go`, `session/session_summary_types.go`

##### Task 1.1.1a: Add pre-cleanup `UpdateDiffStats()` calls (~3 min)
- In `session/instance.go`, inside `Destroy()` (currently starts at line 1246), add `_ = i.UpdateDiffStats()` as the first statement immediately before the existing `if err := i.CleanupWorktree(); err != nil {` line (~1271). Error intentionally discarded — diff capture is best-effort, matching `computeDirDiffStats`'s existing "returns nil on any error" convention.
- In `session/instance.go`, inside `instanceOnExitCallback` (starts at line 785), add `_ = i.UpdateDiffStats()` immediately before the existing `i.fireLifecycleEvent(EventExited, reason)` line (~811).
- Files: `session/instance.go`

##### Task 1.1.1b: Domain snapshot types (~4 min)
- Create `session/session_summary_types.go` with: `SessionSummaryStatus` (string type + `StatusPending/StatusGenerating/StatusReady/StatusError` consts + `IsValid() bool`); `DiffSnapshot{FilesChanged, Added, Removed int}` + `IsEmpty() bool`; `DecisionsSnapshot{AutoApproved, ManuallyApproved, Denied, ReviewQueueResolved, StillOpen int}` + `Total() int` + `Percent(n int) float64`; `TimelineSnapshot{StartedAt, StoppedAt time.Time}` + `Duration() time.Duration`; `CostSnapshot{TotalTokens int64, EstimatedCostUSD float64, DataUnavailable bool}`.
- Files: `session/session_summary_types.go`

#### Story 1.1.2: `sessionSummaryListener` — the lifecycle trigger
**As a** session, **I want** both natural exit and explicit stop to trigger summary generation identically, **so that** AC-1 and AC-7 both hold with no asymmetry.
**Acceptance Criteria**:
- AC-1: *Given* an `Instance` (`UUID: "sess-123"`) whose process exits naturally, *When* `instanceOnExitCallback` fires `i.fireLifecycleEvent(EventExited, "pty-eof")`, *Then* `sessionSummaryListener.OnLifecycleEvent(EventExited, "pty-eof")` calls `instance.GetDiffStats()` synchronously and dispatches `go generator.GenerateAndPersist(context.Background(), "sess-123", <title>, <createdAt>, diffStats, "pty-eof")` (full signature per Task 1.5.2c; `generator` is the `summaryGenerator` interface field, per Task 1.1.2a) — no manual action required.
- AC-7: *Given* the same `Instance`, *When* `DeleteSession` RPC calls `inst.Destroy()` which fires `EventStopped` with reason `"operator-destroy"`, *Then* the identical dispatch happens (same function, same code path) — not a different, narrative-less pipeline.
- Reason filter: *Given* `reason == "reconcile-session-missing"`, *When* `OnLifecycleEvent(EventExited, "reconcile-session-missing")` fires, *Then* the listener returns immediately without dispatching anything (AC-1's "excluding reconciler-driven spurious exits").
**Files**: `session/session_summary_listener.go`

##### Task 1.1.2a: Implement `sessionSummaryListener` + `WireToInstance` (~5 min)
- New file `session/session_summary_listener.go`: first define the narrow consumer-side interface this file needs, scoped to exactly one method: `type summaryGenerator interface { GenerateAndPersist(ctx context.Context, sessionUUID, sessionTitle string, createdAt time.Time, diffStats *git.DiffStats, reason string) }`. `*SessionSummaryGenerator` (Task 1.5.2a) satisfies this structurally — no explicit "implements" declaration needed, per `.claude/rules/interface-pollution-checklist.md`'s "define the interface where it's consumed."
- `type sessionSummaryListener struct { generator summaryGenerator; instance *Instance }` (note: `generator` is the **interface** type above, not a concrete `*SessionSummaryGenerator` pointer — this is what makes Task 1.1.2b's fake test double constructible without a real ent client). Captures the live `*Instance` pointer, not just its UUID — needed for `GetDiffStats()`/`CreatedAt`/`Title` reads. `OnLifecycleEvent(event LifecycleEvent, reason string)`: `switch event { case EventExited, EventStopped: }` else return; `if reason == "reconcile-session-missing" { return }`; `diffStats := l.instance.GetDiffStats()` (sync, no I/O); dispatch per Task 1.5.2c's final call shape (`go l.generator.GenerateAndPersist(context.Background(), ...)` — panic recovery for this goroutine lives inside `GenerateAndPersist` itself, see Task 1.5.2b, so it protects this dispatch automatically).
- Add `func WireSessionSummaryListener(generator summaryGenerator, inst *Instance) { inst.RegisterLifecycleListener(&sessionSummaryListener{generator: generator, instance: inst}) }` — takes the interface type so callers (Task 2.2.3a) can pass a real `*SessionSummaryGenerator`, which satisfies it for free. Mirrors `BacklogLifecycleListener.WireToInstance` (`session/backlog_lifecycle.go:813-818`).
- Files: `session/session_summary_listener.go`

##### Task 1.1.2b: Unit test the listener's dispatch/filter logic (~4 min)
- New `session/session_summary_listener_test.go`: table test asserting (1) `EventStarted` is ignored, (2) `EventExited`/`EventStopped` with a normal reason dispatch, (3) `EventExited` with `reason == "reconcile-session-missing"` does not dispatch. Construct a small fake type implementing the `summaryGenerator` interface directly, e.g. `type fakeSummaryGenerator struct { calls chan struct{ sessionUUID string } }` with a `GenerateAndPersist` method that writes to `calls` (a buffered channel, since the real call is dispatched via `go`) — this requires no ent client, no `headless.Pool`, and no other `SessionSummaryGenerator` dependency, because the listener only ever depends on the interface.
- Files: `session/session_summary_listener_test.go`

---

### Epic 1.2: ent Schema

#### Story 1.2.1: `SessionSummary` ent schema
**As a** completion summary, **I want** to be stored independently of the `Session` row, **so that** AC-3/AC-7 hold after `Session`-row deletion and server restart.
**Acceptance Criteria**:
- AC-3: *Given* a `SessionSummary` row persisted with `session_id: "sess-123"`, `status: "ready"`, *When* the `Session` ent row for `"sess-123"` is deleted via `DeleteSession`, *Then* the `SessionSummary` row is untouched (no cascading delete — no edge exists to `Session`).
**Files**: `session/ent/schema/session_summary.go`

##### Task 1.2.1a: Write the ent schema file (~5 min)
- New `session/ent/schema/session_summary.go`, `SessionSummary` entity, fields: `id` (`field.String("id").Unique().NotEmpty().Immutable()`, app-generated UUID per `server/services/approval_handler.go:357` convention), `session_id` (`field.String("session_id").NotEmpty().Unique()`), `session_title` (`field.String("session_title").Optional()`), `status` (`field.String("status").Default("pending")`), `narrative` (`field.Text("narrative").Optional()`), `narrative_fallback_used` (`field.Bool("narrative_fallback_used").Default(false)`), `diff_files_changed`/`diff_added`/`diff_removed` (`field.Int(...).Default(0)`), `decisions_auto_approved`/`decisions_manually_approved`/`decisions_denied`/`decisions_review_queue_resolved`/`decisions_still_open` (`field.Int(...).Default(0)`), `session_started_at`/`session_stopped_at` (`field.Time(...).Optional().Nillable()`), `duration_ms` (`field.Int64("duration_ms").Optional().Nillable()`), `total_tokens` (`field.Int64("total_tokens").Optional().Nillable()`), `estimated_cost_usd` (`field.Float("estimated_cost_usd").Optional().Nillable()`), `cost_data_unavailable` (`field.Bool(...).Default(false)`), `markdown` (`field.Text("markdown").Optional()`), `error_message`/`error_stage` (`field.String(...).Optional()`), `generation_started_at`/`generated_at` (`field.Time(...).Optional().Nillable()`), `created_at` (`field.Time("created_at").Default(time.Now).Immutable()`), `updated_at` (`field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now)`). No `Edges()` method (deliberately no edge to `Session`, per Pattern Decisions).
- Files: `session/ent/schema/session_summary.go`

##### Task 1.2.1b: Regenerate ent code (~2 min)
- Run `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` (per `.claude/rules/ent-schema-generation.md` — omitting `--feature sql/upsert` silently breaks `Upsert*` methods needed by Story 1.5.3's regenerate-in-place). Run `go build ./...` to confirm the generated `session/ent/sessionsummary/`, `session/ent/sessionsummary.go`, etc. compile. Commit all `session/ent/` changes together with the schema file per the project convention.
- Files: `session/ent/schema/session_summary.go` (already written), generated files under `session/ent/` (not hand-edited)

---

### Epic 1.3: Deterministic Snapshot Assembly
**Goal**: Build `DiffSnapshot`/`DecisionsSnapshot`/`TimelineSnapshot`/`CostSnapshot` from already-safe data sources (live `*Instance`, `NotificationHistoryStore`, `TokenStore`) — never from `s.storage`/`ListInstanceData()`.

#### Story 1.3.1: Diff + timeline snapshot builder
**As a** `SessionSummaryGenerator`, **I want** to turn a captured `*git.DiffStats` and `*Instance` metadata into `DiffSnapshot`/`TimelineSnapshot`, **so that** FR-2's "Changes" and "Timeline" sections have data.
**Acceptance Criteria**:
- AC-6: *Given* a session with `diffStats == nil` (no worktree / directory session with no changes), *When* `BuildDiffSnapshot(nil)` is called, *Then* it returns `DiffSnapshot{FilesChanged: 0, Added: 0, Removed: 0}` with `IsEmpty() == true` — not an error.
**Files**: `session/session_summary_snapshot.go`

##### Task 1.3.1a: `BuildDiffSnapshot` + `BuildTimelineSnapshot` (~4 min)
- In new `session/session_summary_snapshot.go`: `func BuildDiffSnapshot(stats *git.DiffStats) DiffSnapshot` — nil-safe, counts files changed by parsing `stats.Content`'s `diff --git` header lines (or 0 if `stats == nil`/`Content == ""`). `func BuildTimelineSnapshot(createdAt time.Time, stoppedAt time.Time) TimelineSnapshot` — takes `instance.CreatedAt` and `time.Now()` (captured at dispatch time in the listener/generator, not re-derived later).
- Files: `session/session_summary_snapshot.go`

#### Story 1.3.2: Decisions snapshot builder
**As a** `SessionSummaryGenerator`, **I want** approval-decision counts for a session, **so that** FR-2's "Decisions" section is populated even for backlog-less sessions.
**Acceptance Criteria**:
- AC-2 (partial): *Given* `NotificationHistoryStore` contains 5 records for `session_id: "sess-123"` with decision `"allow"` (auto-approved), 1 with a manual-approval marker, 0 denied, *When* `BuildDecisionsSnapshot(ctx, "sess-123", notifStore, reviewQueueLookup)` runs, *Then* it returns `DecisionsSnapshot{AutoApproved: 5, ManuallyApproved: 1, Denied: 0, ...}`.
**Files**: `session/session_summary_snapshot.go`

##### Task 1.3.2a: `BuildDecisionsSnapshot` (~5 min)
- Add `func BuildDecisionsSnapshot(ctx context.Context, sessionID string, notifStore *notifications.NotificationHistoryStore, reviewLookup ReviewQueueLookup) (DecisionsSnapshot, error)` to `session/session_summary_snapshot.go`. Query `notifStore.List(notifications.ListOptions{SessionID: sessionID})` (an in-memory `RLock`-guarded read — per `server/notifications/store.go:236-279`, this call never returns a non-nil error today; file corruption is swallowed silently at load time, not surfaced at query time), classify by `NotificationType`/`Metadata["decision"]` into auto-approved/manually-approved/denied buckets. `ReviewQueueLookup` is a small consumer-defined interface (`ReviewQueueResolvedCount(ctx context.Context, sessionID string) (resolved, stillOpen int, err error)`) satisfied by existing `ItemSession`/`ReviewVerdict` query code — this DB-backed call is the function's **only actual error source**; wrap it in an explicit `context.WithTimeout(ctx, reviewQueueLookupTimeout)` (`const reviewQueueLookupTimeout = 2 * time.Second`, mirroring `server/review_queue_manager.go:360-362`'s `itemSessionLookupTimeout` pattern for the identical storage call) so a slow/hung DB query can't block the dispatched goroutine indefinitely. If no linked backlog item exists, return `(0, 0, nil)` (FR-6's "no backlog item" first-class empty case, per `research/features.md` §5).
- Files: `session/session_summary_snapshot.go`

#### Story 1.3.3: Cost snapshot builder
**As a** `SessionSummaryGenerator`, **I want** token usage/cost for a session, **so that** FR-2's "Token usage" section is populated, distinguishing zero-usage from unavailable data.
**Acceptance Criteria**:
- AC-2 (partial): *Given* `TokenStore.GetByUUID("sess-123")` returns a `*ParseResult{TotalTokens: 128000, EstimatedCostUSD: 1.92}`, *When* `BuildCostSnapshot("sess-123", tokenStore)` runs, *Then* it returns `CostSnapshot{TotalTokens: 128000, EstimatedCostUSD: 1.92, DataUnavailable: false}`.
- AC-6 (partial): *Given* `TokenStore.GetByUUID("sess-456")` returns `nil` (no transcript found), *When* `BuildCostSnapshot("sess-456", tokenStore)` runs, *Then* it returns `CostSnapshot{DataUnavailable: true}` (renders as "Cost data unavailable", distinct from "No tokens were used" per `research/ux.md` §4's table).
**Files**: `session/session_summary_snapshot.go`

##### Task 1.3.3a: `BuildCostSnapshot` (~3 min)
- Add `func BuildCostSnapshot(sessionUUID string, tokenStore *tokens.TokenStore) CostSnapshot` to `session/session_summary_snapshot.go`: nil-safe wrapper over `tokenStore.GetByUUID(sessionUUID)`.
- Files: `session/session_summary_snapshot.go`

##### Task 1.3.4: Unit tests for all three snapshot builders (~5 min)
- New `session/session_summary_snapshot_test.go` covering: nil diff stats → empty snapshot; populated diff stats → correct counts; decisions with 0 records → all-zero `DecisionsSnapshot`; decisions with mixed types → correct bucketing; cost with nil `ParseResult` → `DataUnavailable: true`; cost with zero-token `ParseResult` → `DataUnavailable: false, TotalTokens: 0`.
- Files: `session/session_summary_snapshot_test.go`

---

### Epic 1.4: Narrative Generation (LLM)

#### Story 1.4.1: `GenerateSessionCompletionNarrative`
**As a** `SessionSummaryGenerator`, **I want** an LLM-generated "what was done" narrative grounded strictly in retrieved data, **so that** FR-2's narrative section is populated without hallucinating structured facts.
**Acceptance Criteria**:
- AC-2 (partial): *Given* a diff (sanitized via `SanitizeDiff`, truncated via the existing `headless.MaxDiffSizeReview` constant) and a `DecisionsSnapshot`, *When* `GenerateSessionCompletionNarrative(ctx, pool, input)` is called, *Then* it returns prose text and `pool.CallBlocking` was invoked with `FeatureKeySessionCompletionSummary`.
**Files**: `session/headless/features.go`

##### Task 1.4.1a: Add `FeatureKeySessionCompletionSummary` + narrative function (~5 min)
- In `session/headless/features.go`, add `FeatureKeySessionCompletionSummary FeatureKey = "session-completion-summary"` to the const block. Add `sessionCompletionSummarySystemPrompt` const (stable prompt instructing: ground strictly in the provided diff/decisions/timeline, do not speculate, plain descriptive prose only, no markdown headings — headings are added by the deterministic renderer). Add `func GenerateSessionCompletionNarrative(ctx context.Context, pool *Pool, diff, decisionsSummary string) (string, error)`: sanitize+truncate `diff` via the existing `SanitizeDiff`/`maxDiffSizeReview`-equivalent helpers used by `session/backlog_review.go:611-615,646-680` (reuse, don't reinvent per `research/features.md` §1), build user prompt, call `pool.CallBlocking(ctx, FeatureKeySessionCompletionSummary, sessionCompletionSummarySystemPrompt, userPrompt, CallOptions{})`, return text or error (caller handles fallback).
- Files: `session/headless/features.go`

#### Story 1.4.2: Trivial-session skip threshold + deterministic fallback
**As a** `SessionSummaryGenerator`, **I want** to skip the LLM call entirely for near-empty sessions and substitute a fixed fallback line on any narrative failure, **so that** FR-5/FR-6/cost-control all hold.
**Acceptance Criteria**:
- AC-6: *Given* `DiffSnapshot.IsEmpty() == true`, `DecisionsSnapshot.Total() == 0`, and `TimelineSnapshot.Duration() < 30*time.Second`, *When* the generator decides whether to call the LLM, *Then* `isTrivialSession(...)` returns `true`, the LLM call is skipped entirely, and `narrative = "This session ended before any work was recorded."`, `narrative_fallback_used = true`.
- AC-5: *Given* a non-trivial session where `GenerateSessionCompletionNarrative` returns an error (timeout/failure), *When* the generator handles that error, *Then* `narrative = "This session made changes across the files listed in Changes below; a narrative summary could not be generated."`, `narrative_fallback_used = true`, and the pipeline continues to `READY` (does not abort).
**Files**: `session/session_summary_snapshot.go`, `session/session_summary_service.go`

##### Task 1.4.2a: `isTrivialSession` + fallback constants (~3 min)
- In `session/session_summary_snapshot.go`: `const trivialSessionMaxDuration = 30 * time.Second`; `func isTrivialSession(diff DiffSnapshot, decisions DecisionsSnapshot, duration time.Duration) bool { return diff.IsEmpty() && decisions.Total() == 0 && duration < trivialSessionMaxDuration }`.
- In `session/session_summary_service.go` (created in Epic 1.5): `const narrativeFallbackTrivial = "This session ended before any work was recorded."`; `const narrativeFallbackLLMFailure = "This session made changes across the files listed in Changes below; a narrative summary could not be generated."`; `const llmNarrativeTimeout = 60 * time.Second` (explicit `context.WithTimeout` around the `GenerateSessionCompletionNarrative` call — `research/pitfalls.md` §3's "no explicit LLM timeout" gap, resolved here).
- Files: `session/session_summary_snapshot.go`, `session/session_summary_service.go`

---

### Epic 1.5: Markdown Assembly, Persistence, Idempotency

#### Story 1.5.1: Markdown renderer
**As a** `SessionSummaryGenerator`, **I want** a single deterministic markdown-rendering function, **so that** FR-4's "valid GFM, reusable as PR body" holds and empty sections render explicit empty-state text (FR-6).
**Acceptance Criteria**:
- AC-2/AC-6: *Given* `DecisionsSnapshot{}` (all zero), *When* `RenderSessionSummaryMarkdown(...)` runs, *Then* the "## Decisions" section reads "No approval requests occurred during this session." (per `research/ux.md` §4's empty-state table) rather than "0 auto-approved (0%)" or being omitted.
**Files**: `session/session_summary_markdown.go`

##### Task 1.5.1a: Implement `RenderSessionSummaryMarkdown` (~5 min)
- New `session/session_summary_markdown.go`: `strings.Builder`-based function producing, in order: `# Session Summary: <title>` heading; `## What Was Done` (narrative text, or the trivial/fallback line); `## Changes` (file/added/removed counts + a markdown link `[View full diff](<diffLink>)`, or "No files were changed." if empty); `## Decisions` (5 counts + percentages via `DecisionsSnapshot.Percent`, or the empty-state line if `Total() == 0`); `## Timeline` (started/stopped/duration, "Duration: <1s" if `Duration() < time.Second`); `## Token Usage` (tokens + `$%.2f` cost, "No tokens were used." if zero-and-available, "Cost data unavailable." if `DataUnavailable`). No non-GFM syntax (plain `#`/`##` headings, `-` bullets, `[text](url)` links only).
- Files: `session/session_summary_markdown.go`

##### Task 1.5.1b: Unit tests for the markdown renderer (~4 min)
- New `session/session_summary_markdown_test.go`: table test asserting each empty-state string appears verbatim for its corresponding zero-value snapshot, and that a populated example renders all 5 section headings plus correct percentages (e.g. 5 of 8 total → "62.5%").
- Files: `session/session_summary_markdown_test.go`

#### Story 1.5.2: `GenerateAndPersist` — assemble-in-memory, single final status-transitioning write
**As a** `SessionSummaryGenerator`, **I want** the full document assembled in memory before the final status-transitioning write, **so that** FR-5's ERROR/READY split is clean and no partial-write state is ever visible (per `research/pitfalls.md` §4). (Wording note: this is "exactly one *final* write," not "exactly one write total" — the design correctly performs an earlier `GENERATING` upsert too, per Task 1.5.2b step 2; Story 1.5.3's restart-survival staleness check depends on that interim write's `generation_started_at` having been persisted.)
**Acceptance Criteria**:
- AC-5: *Given* `BuildDecisionsSnapshot` returns an error (the only realistic trigger is `ReviewQueueLookup.ReviewQueueResolvedCount`'s DB-backed call timing out or failing — `NotificationHistoryStore.List` itself never errors, see Task 1.3.2a), *When* `GenerateAndPersist` runs, *Then* the persisted row has `status: "error"`, `error_stage: "decisions"`, `error_message` set, the already-computed `DiffSnapshot`/`TimelineSnapshot` fields (built earlier in the same step, before the failing call) **are** persisted, and any prior successful generation's `markdown`/`narrative`/decisions fields are left untouched (field-scoped upsert, per the Pattern Decisions "Error-path field preservation" row) — the deterministic-stage failure surfaces as ERROR per FR-5, distinct from a narrative-stage failure which still reaches READY.
**Files**: `session/session_summary_service.go`

##### Task 1.5.2a: `SessionSummaryGenerator` struct + constructor (~3 min)
- New `session/session_summary_service.go`: `type SessionSummaryGenerator struct { entClient *ent.Client; pool headless.PoolClient; notifStore *notifications.NotificationHistoryStore; tokenStore *tokens.TokenStore; reviewLookup ReviewQueueLookup; inFlight sync.Map /* session_id -> *sync.Mutex */ }`. `func NewSessionSummaryGenerator(entClient *ent.Client, pool headless.PoolClient, notifStore *notifications.NotificationHistoryStore, tokenStore *tokens.TokenStore, reviewLookup ReviewQueueLookup) *SessionSummaryGenerator`. Its `GenerateAndPersist` method (Task 1.5.2b) is the sole method required to satisfy the `summaryGenerator` interface consumed by `session_summary_listener.go` (Task 1.1.2a) — no explicit interface assertion needed, Go structural typing handles it.
- Files: `session/session_summary_service.go`

##### Task 1.5.2b: `GenerateAndPersist` orchestration (~6 min)
- Signature: `func (g *SessionSummaryGenerator) GenerateAndPersist(ctx context.Context, sessionUUID, sessionTitle string, createdAt time.Time, diffStats *git.DiffStats, reason string)`.
- **Panic safety (mirrors `BacklogLifecycleListener.runStuckDetector`, `session/backlog_lifecycle.go:1407-1421`)**: since this method is always invoked via `go ...GenerateAndPersist(...)` (both the listener, Task 1.5.2c, and Regenerate, Task 2.2.2a, dispatch it as a detached goroutine), an unrecovered panic anywhere in the pipeline (LLM client, markdown rendering, ent write) would crash the entire server process, taking every live tmux session down with it — not just this feature. As the very first two statements of the method: acquire the dedup guard, then `defer release()` immediately, then `defer func() { if r := recover(); r != nil { log.WarningLog.Printf("[SessionSummary] GenerateAndPersist panicked (recovered) for session=%s: %v", sessionUUID, r) } }()`. Because Go runs deferred functions in LIFO order, the recover-wrapper (registered second) runs first and stops the panic, after which `release()` (registered first) still runs normally — so the guard is never left permanently locked by a panic, and the crash never reaches the rest of the process. (A future best-effort improvement — not required for this task — would be for the recover handler to also attempt an ERROR upsert; out of scope here since the panic could have occurred inside the DB call itself.)
- Steps: (1) `release, ok := g.tryAcquire(sessionUUID)`; if `!ok`, return immediately without touching the DB. `defer release()` right after a successful acquire (not a plain final step — see panic-safety note above). (1b) **Sequential-duplicate short-circuit (closes the gap where the in-flight guard only blocks concurrent, not sequential, duplicate triggers)**: read the current persisted row for `sessionUUID`. If `reason != "manual-regenerate"` and the row's `status == "ready"`, return immediately with no LLM call and no write — a `READY` row already means a duplicate/late-arriving trigger (e.g. a duplicate `EventExited`/`EventStopped` for the same session) has nothing to do. An explicit user-initiated Regenerate (`reason == "manual-regenerate"`, set by Task 2.2.2a) is exempt from this check and always proceeds. (1c) **Cooldown check for Regenerate** (independent of the in-flight guard, addresses rapid sequential Regenerate clicks — see Task 1.5.3c for the constant): if `reason == "manual-regenerate"` and the row's `generated_at` is non-nil and less than `regenerateCooldown` old, return immediately without a write (current row is unchanged — the caller/RPC handler returns the row as-is). (2) upsert row to `status: "generating"`, `generation_started_at: now` — same `g.entClient.SessionSummary.Create().Set...().OnConflictColumns(sessionsummary.FieldSessionID).UpdateNewValues().Exec(ctx)` shape as step 6 below (not a bare `.Create()`, which would hit `UNIQUE constraint failed: session_id` on any re-trigger for a session that already has a row — the `session_id` field's `Unique()` constraint from Task 1.2.1a makes this the core path, not an edge case); (3) build `DiffSnapshot`/`TimelineSnapshot`/`DecisionsSnapshot`/`CostSnapshot` via Epic 1.3 functions — on any error from `BuildDecisionsSnapshot(ctx, ...)` (the only builder that returns an error), upsert `status: "error"`, `error_stage: "decisions"`, `error_message`, **and still persist the already-computed `DiffSnapshot`/`TimelineSnapshot` fields** (built earlier in this same step, before the failing call) so a subsequent Regenerate always has *something* — a real diff/timeline, not nothing — to fall back to if the live `*Instance` is also gone by then (see Task 2.2.2a); leave `markdown`/`narrative`/decisions fields from any prior successful generation untouched (field-scoped upsert — see Pattern Decisions "Error-path field preservation"); return (guard release handled by the `defer` above); (4) if `isTrivialSession(...)`, set `narrative = narrativeFallbackTrivial, fallbackUsed = true`, skip LLM; else call `GenerateSessionCompletionNarrative` under a `context.WithTimeout(ctx, llmNarrativeTimeout)` — on error, `narrative = narrativeFallbackLLMFailure, fallbackUsed = true` (log at Warn, do not abort); (5) render markdown via `RenderSessionSummaryMarkdown`; (6) single final upsert write: all fields + `status: "ready"`, `generated_at: now`, via `g.entClient.SessionSummary.Create().Set...().OnConflictColumns(sessionsummary.FieldSessionID).UpdateNewValues().Exec(ctx)` (mirror the exact upsert shape used in `session/ent_repository_backlog.go:1071-1082` / `server/services/error_registry.go:52`'s `OnConflictColumns(...).Update(...)` — this repo's ent setup requires `--feature sql/upsert`, already covered by Task 1.2.1b). **On this write's own failure** (LLM cost already spent, row still says `"generating"`): attempt a best-effort fallback write, `status: "error", error_stage: "persist"`, and log at Error level — do not silently leave the row in `"generating"` for the 5-minute staleness reconciler to eventually paper over with a misleading "possibly due to a server restart" message.
- Files: `session/session_summary_service.go`

##### Task 1.5.2c: Wire `sessionSummaryListener` to call this with instance-derived args (~2 min)
- Update Task 1.1.2a's `sessionSummaryListener.OnLifecycleEvent` dispatch to `go l.generator.GenerateAndPersist(context.Background(), l.instance.UUID, l.instance.Title, l.instance.CreatedAt, diffStats, reason)`.
- Files: `session/session_summary_listener.go`

##### Task 1.5.2d: Unit tests for `GenerateAndPersist` (~7 min)
- New `session/session_summary_service_test.go` (uses ent's in-memory SQLite test client, matching existing ent test setup convention elsewhere in this package): trivial session → READY with fallback narrative, no LLM call recorded on a fake `headless.PoolClient`; decisions-builder error → ERROR with `error_stage: "decisions"` **and asserts the already-computed diff/timeline fields are persisted on that error row, not discarded**; LLM failure (fake pool returns error) → READY with `narrative_fallback_used: true`; happy path → READY with all fields populated and exactly one *final* write (assert via row's final state, and that no intermediate GENERATING row is left dangling); **sequential-duplicate short-circuit**: a second call with a non-manual `reason` against an already-`READY` row makes no LLM call and writes nothing; **manual-regenerate bypass**: a second call with `reason: "manual-regenerate"` against an already-`READY` row (outside the cooldown window) does proceed and re-runs the pipeline; **cooldown**: a `manual-regenerate` call within `regenerateCooldown` of the row's `generated_at` returns immediately with no write; **panic recovery**: a fake pool/builder that panics does not crash the test process and leaves the guard released (a follow-up call for the same session succeeds); **persist-write failure**: a fake ent client that fails only on the final upsert results in `status: "error", error_stage: "persist"`.
- Files: `session/session_summary_service_test.go`

#### Story 1.5.3: FR-7 dedup — in-memory guard + persisted staleness check
**As a** `SessionSummaryGenerator`, **I want** concurrent/duplicate triggers and repeated Regenerate clicks to collapse into at most one running pipeline per session, **so that** AC-8 holds and a crashed-mid-generation row doesn't stay stuck forever.
**Acceptance Criteria**:
- AC-8: *Given* a `SessionSummary` row for `"sess-123"` already `status: "generating"` (in-flight, guard held), *When* a duplicate `EventExited` fires and a manual Regenerate click arrive concurrently, *Then* both additional calls to `GenerateAndPersist("sess-123", ...)` return immediately without acquiring the guard, without calling `pool.CallBlocking` a second time, and without writing a second row.
- AC-8 (restart case): *Given* a row `status: "generating"`, `generation_started_at: now - 10*time.Minute` (older than `staleGenerationTimeout = 5*time.Minute`) with no in-memory guard held (simulating a server restart mid-generation), *When* `GetSessionSummary("sess-123")` is called, *Then* the read path flips the row to `status: "error"`, `error_message: "generation did not complete, possibly due to a server restart"` before returning it, and Regenerate becomes available.
- AC-8 (sequential-duplicate case): *Given* a row for `"sess-123"` already `status: "ready"` (guard free, generation finished), *When* a late-arriving duplicate `EventExited`/`EventStopped` calls `GenerateAndPersist("sess-123", ..., reason: "pty-eof")` again, *Then* it returns immediately with no LLM call and no write (Task 1.5.2b step 1b) — but an explicit `RegenerateSessionSummary` call (`reason: "manual-regenerate"`) against the same `READY` row still proceeds.
- AC-8 (cooldown case): *Given* a row's `generated_at` is 5 seconds old, *When* the user clicks Regenerate again immediately, *Then* `GenerateAndPersist` returns without a write or LLM call (Task 1.5.2b step 1c / Task 1.5.3c) until `regenerateCooldown` (30s) has elapsed since `generated_at`.
**Files**: `session/session_summary_service.go`, `server/services/session_summary_service.go`

##### Task 1.5.3a: In-memory per-session guard (~3 min)
- In `session/session_summary_service.go`, add `func (g *SessionSummaryGenerator) tryAcquire(sessionUUID string) (release func(), ok bool)`: `mu, _ := g.inFlight.LoadOrStore(sessionUUID, &sync.Mutex{})`; `m := mu.(*sync.Mutex)`; `if !m.TryLock() { return nil, false }`; return `(m.Unlock, true)`. Call this as the very first step of `GenerateAndPersist` (Task 1.5.2b step 1); if `!ok`, return without touching the DB. Also add `func (g *SessionSummaryGenerator) isInFlight(sessionUUID string) bool`: a non-blocking probe — `mu, ok := g.inFlight.Load(sessionUUID); if !ok { return false }; m := mu.(*sync.Mutex); if m.TryLock() { m.Unlock(); return false }; return true` — used by `reconcileStaleness` (Task 1.5.3b) to distinguish "still actively generating in this process" from "genuinely stuck (process restarted)".
- Files: `session/session_summary_service.go`

##### Task 1.5.3b: `const staleGenerationTimeout` + read-time staleness flip (~4 min)
- In `session/session_summary_service.go`: `const staleGenerationTimeout = 5 * time.Minute`. Add `func (g *SessionSummaryGenerator) reconcileStaleness(ctx context.Context, row *ent.SessionSummary) *ent.SessionSummary`: if `row.Status == "generating" && row.GenerationStartedAt != nil && time.Since(*row.GenerationStartedAt) > staleGenerationTimeout`: **first check `if g.isInFlight(row.SessionID) { return row }`** — a held in-memory guard means the current process is still actively working on it (a long-running LLM call, slow disk I/O, GC pause), regardless of how long it's taken, so it must not be flipped to ERROR out from under the still-running goroutine (doing so would let a later Regenerate click acquire a *new* guard attempt that immediately fails against the still-held mutex, producing a dead-end ERROR row with no way to recover until the wedged goroutine eventually finishes or the process restarts). Only if the guard is **not** held does this proceed to upsert `status: "error", error_message: "generation did not complete, possibly due to a server restart", error_stage: "restart-interrupted"` and return the updated row; else return `row` unchanged. Call this from the RPC handler's `GetSessionSummary` (Task 2.2.1), not from a background sweep.
- Files: `session/session_summary_service.go`

##### Task 1.5.3c: Regenerate cooldown (~3 min)
- In `session/session_summary_service.go`: `const regenerateCooldown = 30 * time.Second`. This is independent of the in-flight guard/dedup (Task 1.5.3a), which only prevents *concurrent* duplicate pipelines — it addresses a user repeatedly clicking Regenerate *after* each attempt completes, which would otherwise trigger a fresh real LLM call every time with no rate limit (`research/pitfalls.md` §7). Consumed by Task 1.5.2b step (1c): if `reason == "manual-regenerate"` and `row.GeneratedAt` is non-nil and `time.Since(*row.GeneratedAt) < regenerateCooldown`, `GenerateAndPersist` returns immediately without a write, and the RPC handler (Task 2.2.2a) returns the current row unchanged to the client.
- Files: `session/session_summary_service.go`

---

## Phase 2: RPC Layer

### Epic 2.1: Proto Definitions

#### Story 2.1.1: `session_summary.proto` + `SessionSummaryStatus` enum
**As a** frontend client, **I want** typed RPCs for reading and regenerating a session summary, **so that** the UI can fetch/poll/regenerate without depending on `SessionService`'s live-instance machinery.
**Acceptance Criteria**:
- AC-3: *Given* a `session_id` with no live `Session` row, *When* the client calls `GetSessionSummary(session_id: "sess-123")`, *Then* the RPC contract permits (and the handler, Epic 2.2, guarantees) a successful response sourced only from the `SessionSummary` table.
**Files**: `proto/session/v1/session_summary.proto`, `proto/session/v1/types.proto`

##### Task 2.1.1a: Add `SessionSummaryStatus` enum to `types.proto` (~2 min)
- In `proto/session/v1/types.proto`, alongside `ScanStatus` (~line 1317), add: `enum SessionSummaryStatus { SESSION_SUMMARY_STATUS_UNSPECIFIED = 0; SESSION_SUMMARY_STATUS_PENDING = 1; SESSION_SUMMARY_STATUS_GENERATING = 2; SESSION_SUMMARY_STATUS_READY = 3; SESSION_SUMMARY_STATUS_ERROR = 4; }`.
- Files: `proto/session/v1/types.proto`

##### Task 2.1.1b: New `session_summary.proto` — messages (~5 min)
- New `proto/session/v1/session_summary.proto` (`package session.v1;` importing `google/protobuf/timestamp.proto` and `session/v1/types.proto`, matching `unfinished.proto`'s header shape). Define `message SessionSummaryProto` with fields: `string session_id`, `string session_title`, `SessionSummaryStatus status`, `string narrative`, `bool narrative_fallback_used`, a `Diff` sub-message (`int32 files_changed; int32 added; int32 removed;`) as field `diff`, a `Decisions` sub-message (`int32 auto_approved; int32 manually_approved; int32 denied; int32 review_queue_resolved; int32 still_open;`) as field `decisions`, a `Timeline` sub-message (`google.protobuf.Timestamp started_at; google.protobuf.Timestamp stopped_at; int64 duration_ms;`) as field `timeline`, a `Cost` sub-message (`int64 total_tokens; double estimated_cost_usd; bool data_unavailable;`) as field `cost`, `string markdown`, `string error_message`, `string error_stage`, `google.protobuf.Timestamp generated_at`.
- Files: `proto/session/v1/session_summary.proto`

##### Task 2.1.1c: New `session_summary.proto` — service (~3 min)
- In the same file, add `service SessionSummaryService { rpc GetSessionSummary(GetSessionSummaryRequest) returns (GetSessionSummaryResponse) {} rpc RegenerateSessionSummary(RegenerateSessionSummaryRequest) returns (RegenerateSessionSummaryResponse) {} }` with `GetSessionSummaryRequest{string session_id}`, `GetSessionSummaryResponse{optional SessionSummaryProto summary}` (unset/`null` when no row exists yet — e.g. session still running), `RegenerateSessionSummaryRequest{string session_id}`, `RegenerateSessionSummaryResponse{SessionSummaryProto summary}`.
- Files: `proto/session/v1/session_summary.proto`

##### Task 2.1.1d: Run `make proto-gen` (~2 min)
- Run `make proto-gen`; confirm `session/gen/session/v1/session_summary_pb.go`, `session/gen/session/v1/sessionv1connect/session_summary.connect.go`, and `web-app/src/gen/session/v1/session_summary_pb.ts` are generated; `go build ./...` to confirm compilation.
- Files: generated only (no hand-edits)

---

### Epic 2.2: Go RPC Handler

#### Story 2.2.1: `GetSessionSummary` handler
**As a** frontend client, **I want** to fetch a session's summary by `session_id` alone, **so that** AC-3/AC-7's "retrievable independent of the Session row" holds at the API layer.
**Acceptance Criteria**:
- AC-3: *Given* a `SessionSummary` row exists for `session_id: "sess-123"` and the `Session` ent row for `"sess-123"` has been deleted, *When* `GetSessionSummary({session_id: "sess-123"})` is called, *Then* it returns the `SessionSummaryProto` (not a `NotFound` error) — the handler queries `ent.Client.SessionSummary.Query().Where(sessionsummary.SessionID("sess-123"))` directly, never `SessionService.findInstance`/`ListInstanceData`.
**Files**: `server/services/session_summary_service.go`

##### Task 2.2.1a: `SessionSummaryService` (RPC) struct + `GetSessionSummary` (~5 min)
- New `server/services/session_summary_service.go`: `type SessionSummaryService struct { entClient *ent.Client; generator *session.SessionSummaryGenerator }` implementing `sessionv1connect.SessionSummaryServiceHandler`. `GetSessionSummary`: query by `session_id` from the request; if not found, return `connect.NewResponse(&sessionv1.GetSessionSummaryResponse{Summary: nil})` (not an error — "no summary yet" is a valid state for a still-running or never-generated session); if found, call `g.reconcileStaleness(ctx, row)` (Task 1.5.3b) then map the (possibly-just-updated) row to `SessionSummaryProto` via a small `toProto(*ent.SessionSummary) *sessionv1.SessionSummaryProto` mapper (flat ent fields → nested proto sub-messages).
- **Doc comment (CQS note)**: add a Go doc comment on `GetSessionSummary` explicitly stating it performs a lazy write-on-read: *"GetSessionSummary is a read RPC that may perform a side-effecting write: on a stale GENERATING row (see reconcileStaleness), it upserts the row to ERROR before returning it, as a lazy restart-recovery mechanism (deliberately chosen over a background sweep — see plan.md's Pattern Decisions table)."* A caller polling this every 2s otherwise has no way to discover from the method's name/contract alone that it can mutate state.
- Files: `server/services/session_summary_service.go`

#### Story 2.2.2: `RegenerateSessionSummary` handler
**As a** user, **I want** an explicit Regenerate action, **so that** FR-5's ERROR-state recovery path and FR-7's "repeated Regenerate clicks don't spawn overlapping pipelines" both work at the API layer.
**Acceptance Criteria**:
- AC-8: *Given* a row already `status: "generating"`, *When* `RegenerateSessionSummary({session_id: "sess-123"})` is called, *Then* it does not start a second pipeline (the dedup guard, Task 1.5.3a, rejects it) and returns the current (still-`generating`) row rather than an error — the client's poll loop (Story 3.1.1) treats this identically to a fresh trigger.
**Files**: `server/services/session_summary_service.go`

##### Task 2.2.2a: `RegenerateSessionSummary` handler (~4 min)
- In `server/services/session_summary_service.go`: look up the row for context (`session_title`, need `Instance.CreatedAt`/live diff — if the live `*Instance` is still available via `SessionService.FindLiveInstance`, refresh the diff snapshot the same way the listener does; if not (session long gone), reuse the previously-persisted diff/timeline/cost fields as the input and only re-run the narrative + decisions steps). **"Live instance gone + row previously errored before diff/timeline were ever persisted"**: this gap is closed by Task 1.5.2b's error-path fix — a decisions-stage failure now always persists the already-computed `DiffSnapshot`/`TimelineSnapshot` fields even though the row ends in ERROR, so this fallback always has *something* real to reuse (not nothing) for any row that ever reached step 3 of `GenerateAndPersist`, regardless of which stage failed.
- **Critical: dispatch as a detached goroutine with a background context, not the request-scoped `ctx`.** Call `go g.GenerateAndPersist(context.Background(), sessionID, title, createdAt, diffStats, "manual-regenerate")` — **exactly the same shape as Task 1.5.2c's listener dispatch**, never `g.GenerateAndPersist(ctx, ...)` (without `go`) or `go g.GenerateAndPersist(ctx, ...)` (with the request's `ctx`). ConnectRPC/`net/http` cancels the request-scoped `ctx` as soon as the handler returns; since this handler is required to "return the current row state immediately (don't block on the async goroutine — client polls)," the pipeline necessarily keeps running after the handler returns, so it must not be tied to a context that dies the moment the handler returns — otherwise the narrative LLM call's own `context.WithTimeout(ctx, llmNarrativeTimeout)` (Task 1.5.2b step 4) would be cancelled almost instantly, silently degrading Regenerate to the LLM-failure fallback text on essentially every invocation. The dedup guard (Task 1.5.3a) and the cooldown/sequential-duplicate short-circuits (Task 1.5.2b steps 1b/1c) handle overlap and rate-limiting; the RPC handler itself does not need to wait for or synchronize with the goroutine. Return the current row state immediately.
- Files: `server/services/session_summary_service.go`

##### Task 2.2.2b: Unit tests for both RPC handlers (~7 min)
- New `server/services/session_summary_service_test.go`: `GetSessionSummary` returns `Summary: nil` for a session with no row; returns the row when `Session` doesn't exist in storage (assert no call to `s.storage`); staleness reconciliation flips an old `GENERATING` row to `ERROR` on read **only when the in-memory guard is not held** (add a case with the guard held via a fake `isInFlight` returning `true` — the row stays unchanged). `RegenerateSessionSummary` on an already-`GENERATING` row does not trigger a second `pool.CallBlocking` (assert via fake pool's call count); **Regenerate with a live instance still present** — diff snapshot is refreshed the same way the listener does; **Regenerate with no live instance** (`SessionService.FindLiveInstance` returns not-found) — falls back to the previously-persisted diff/timeline/cost fields as input, including the case where those fields came from a prior *error* row (Task 1.5.2b's error-path persistence fix) rather than a prior READY row; **Regenerate dispatches with `context.Background()`, not the request `ctx`** — assert the goroutine's context is not cancelled when the handler's own request context is cancelled immediately after the RPC returns (e.g. pass a `ctx` that's already cancelled by the time `GenerateAndPersist` runs, and assert the narrative call still completes rather than immediately erroring with `context.Canceled`).
- Files: `server/services/session_summary_service_test.go`

#### Story 2.2.3: Server wiring
**As a** deployed server, **I want** `SessionSummaryGenerator`/`SessionSummaryService` constructed and registered, **so that** the feature is live.
**Acceptance Criteria**:
- AC-1: *Given* the server has started, *When* any `Instance` is created via the normal session-creation path, *Then* `WireSessionSummaryListener(generator, inst)` has been called on it (alongside the existing `BacklogLifecycleListener.WireToInstance` call), so natural exit/stop triggers generation without further manual wiring.
**Files**: `server/dependencies.go`, `server/server.go`

##### Task 2.2.3a: Construct `SessionSummaryGenerator` in `dependencies.go` (~4 min)
- In `server/dependencies.go`, near the existing `deps.HeadlessPool`/`sessionService.SetHeadlessPool` wiring (~line 516-527): construct `sessionSummaryGenerator := session.NewSessionSummaryGenerator(entClient, deps.HeadlessPool, notificationHistoryStore, tokenStore, reviewQueueLookupAdapter)` (reusing already-constructed `entClient`/`notificationHistoryStore`/`tokenStore` instances — find their existing variable names at this point in the file rather than reconstructing). Find the existing per-instance wiring loop (where `BacklogLifecycleListener.WireToInstance` is called for each `Instance`) and add `session.WireSessionSummaryListener(sessionSummaryGenerator, inst)` alongside it.
- Files: `server/dependencies.go`

##### Task 2.2.3b: Register `SessionSummaryService` handler in `server.go` (~3 min)
- In `server/server.go`, alongside the `UnfinishedWorkService` registration block (~line 354-359): construct `sessionSummaryService := services.NewSessionSummaryService(entClient, sessionSummaryGenerator)`, register via `sessionv1connect.NewSessionSummaryServiceHandler(sessionSummaryService, ConnectOptions(deps.ErrorRegistry)...)`, mount at its path, log registration.
- Files: `server/server.go`

---

## Phase 3: Frontend

### Epic 3.1: Hook + Shared Panel Component

#### Story 3.1.1: `useSessionSummary` hook
**As a** React component, **I want** a hook that fetches, polls while generating, regenerates, and copies the summary, **so that** both the tab and the standalone route can share one data layer.
**Acceptance Criteria**:
- AC-5/AC-8: *Given* a fetched summary with `status: SESSION_SUMMARY_STATUS_GENERATING`, *When* the hook's poll effect runs, *Then* it re-fetches every 2s until `status` becomes `READY` or `ERROR`, then stops polling.
- (new, closes `design/ux.md`'s flagged gap) *Given* `GetSessionSummary` keeps returning `Summary: nil` (e.g. a `sessionId` that will never produce a row — typo'd URL, or a session excluded via `reconcile-session-missing`), *When* the poll effect has made `maxPollAttempts` (10) consecutive `nil` reads, *Then* it stops polling and sets `neverResolved: true` rather than polling indefinitely.
**Files**: `web-app/src/lib/hooks/useSessionSummary.ts`

##### Task 3.1.1a: Implement the hook (~6 min)
- New `web-app/src/lib/hooks/useSessionSummary.ts`, modeled on `useBacklogItemShipStatus.ts`'s fetch shape plus a poll loop: `export function useSessionSummary(sessionId: string): { data: SessionSummaryProto | null; loading: boolean; error: Error | null; neverResolved: boolean; regenerate: () => Promise<void>; copy: () => Promise<boolean> }`. `fetchSummary` calls `client.getSessionSummary({ sessionId })`; a `useEffect` polls every 2000ms via `setInterval` while `data?.status` is `PENDING`/`GENERATING` **or `data` is `null`** (a `nil` response — no row exists yet — polls identically to `PENDING`/`GENERATING`, per `design/ux.md` surface (b)'s edge case), clearing on unmount or terminal status. `regenerate` calls `client.regenerateSessionSummary({ sessionId })` then resumes polling. `copy` calls the existing `copyToClipboard` helper (`web-app/src/lib/clipboard.ts`) with `data?.markdown ?? ""`. **Terminal empty state (closes the "will never resolve" dead end flagged in `design/ux.md`'s gap summary)**: cap polling at `maxPollAttempts = 10` (10 × 2s ≈ 20s); once exceeded with `data` still `null`, stop polling and set `neverResolved: true` so callers (Task 3.1.2a, Task 3.3.1a) can render a terminal "No summary available for this session" state instead of polling forever.
- Files: `web-app/src/lib/hooks/useSessionSummary.ts`

##### Task 3.1.1b: Hook unit tests (~5 min)
- New `web-app/src/lib/hooks/useSessionSummary.test.ts` (Jest + RTL `renderHook`, mocked ConnectRPC client): fetch on mount; polling starts for `GENERATING` and stops at `READY`; polling also starts when `data` is `null`; `regenerate()` calls the RPC and resumes polling; `copy()` delegates to `copyToClipboard` and returns its boolean result; **after `maxPollAttempts` consecutive `nil` responses, polling stops and `neverResolved` becomes `true`**.
- Files: `web-app/src/lib/hooks/useSessionSummary.test.ts`

#### Story 3.1.2: `SessionSummaryPanel` shared component
**As a** user, **I want** to see the narrative, sections, and a copy button, and to see clear ERROR/empty states, **so that** AC-2/AC-4/AC-5/AC-6 hold in the UI.
**Acceptance Criteria**:
- AC-4: *Given* a `READY` summary with `markdown: "# Session Summary: fix-login\n..."`, *When* the user clicks the "Copy as Markdown" button (`aria-label="Copy summary as Markdown"`), *Then* `copyToClipboard(markdown)` is called and a success state is announced via an `aria-live="polite"` region (not silent `console.warn` on failure — per `research/ux.md` §3's explicitly-flagged gap).
- AC-5/AC-6: *Given* `status: ERROR` **and no prior successful generation exists** (`data.markdown` empty), *When* the panel renders, *Then* it shows the `error_stage`/`error_message`, a timestamp, and a prominent "Regenerate" button (disabled + "Regenerating…" label while a regenerate call is in flight, per `research/ux.md` §4).
- AC-5/AC-6 (stale-document case, closes the gap flagged in `design/ux.md` surface (d2)): *Given* `status: ERROR` **and `data.markdown` is non-empty** (a prior successful generation exists — guaranteed reachable now that Task 1.5.2b's error-path upsert preserves `markdown`/`narrative` on failure, per the Pattern Decisions "Error-path field preservation" row), *When* the panel renders, *Then* it shows the READY document (surface (c)'s rendering, reused verbatim — same Decisions-at-a-glance card, same markdown body) with a banner in place of the bare error card reading "Showing the summary from the last successful generation, dated `<generated_at>` — regeneration failed, see error above" and a `[↻ Try again]` action inside the banner (not a second Regenerate button in the toolbar); `aria-live` announces "Showing the previous summary. Regeneration failed — see the banner for details." (exact copy per `project_plans/session-completion-summary/design/ux.md` surface (d2)).
**Files**: `web-app/src/components/sessions/SessionSummaryPanel.tsx`, `web-app/src/components/sessions/SessionSummaryPanel.css.ts`

##### Task 3.1.2a: Component structure + sections (~7 min)
- New `web-app/src/components/sessions/SessionSummaryPanel.tsx`: `export function SessionSummaryPanel({ sessionId }: { sessionId: string })`, uses `useSessionSummary(sessionId)`. Renders: PENDING/GENERATING (or `data === null` and not yet `neverResolved`) → skeleton blocks + "Generating summary…" (`aria-busy="true"` on the panel container, `aria-live="polite"` status text, per `research/ux.md` §3); `neverResolved: true` → terminal "No summary available for this session" empty state (no further polling — see Task 3.1.1a); READY → `<ReactMarkdown remarkPlugins={[remarkGfm]}>{data.markdown}</ReactMarkdown>` (reusing the existing `react-markdown`+`remark-gfm` pattern from `DescriptionSection.tsx`) plus the Decisions breakdown rendered prominently near the top (not buried — per `research/ux.md` §5); **ERROR with `data.markdown` empty** → bare error state + Regenerate button per the AC above; **ERROR with `data.markdown` non-empty** → the READY rendering plus the stale-document banner described in the AC above (branch on `status === "ERROR" && !!data.markdown`, not on `status` alone); no row yet (session still running) → disabled-tab state is handled by the tab wrapper (Story 3.2.1), this component assumes it's only rendered once reachable.
- Files: `web-app/src/components/sessions/SessionSummaryPanel.tsx`

##### Task 3.1.2b: `.css.ts` styling (vanilla-extract) (~3 min)
- New `web-app/src/components/sessions/SessionSummaryPanel.css.ts` per `.claude/rules/css-architecture.md` (new component → vanilla-extract, not a `.module.css`): tokens from `web-app/src/styles/theme.css.ts` for spacing/colors; a `skeletonBlock` style for the GENERATING state; no hardcoded hex/`var()` strings.
- Files: `web-app/src/components/sessions/SessionSummaryPanel.css.ts`

##### Task 3.1.2c: Component tests (~6 min)
- New `web-app/src/components/sessions/SessionSummaryPanel.test.tsx` (Jest + RTL, mocked `useSessionSummary`): renders skeleton for GENERATING; renders markdown + decisions for READY; renders bare error + enabled Regenerate for ERROR with empty `markdown`; **renders the READY document with a stale-document banner (not the bare error card) for ERROR with non-empty `markdown`**, and asserts the banner's "Try again" action, not a duplicate toolbar Regenerate button; **renders the terminal "No summary available" empty state when `neverResolved: true`**; Regenerate button becomes disabled with "Regenerating…" label while `regenerate()` is in flight; Copy button triggers `copy()` and announces success/failure via the `aria-live` region (assert the region's text content, not just that `copyToClipboard` was called).
- Files: `web-app/src/components/sessions/SessionSummaryPanel.test.tsx`

---

### Epic 3.2: Tab Integration (live-session path)

#### Story 3.2.1: Summary tab in `SessionDetailView`
**As a** user viewing a session that just ended, **I want** a Summary tab that's enabled once the session is terminal, **so that** AC-3's "retrievable from the session's Summary tab" holds for the common case where the row is still visible in the live list.
**Acceptance Criteria**:
- AC-2: *Given* a terminal session (`Status: Stopped`) whose `SessionSummary` is `READY`, *When* the user clicks the "Summary" tab, *Then* `SessionSummaryPanel` renders showing narrative, changes, decisions, timeline, and cost.
- Non-terminal: *Given* a running session (`Status: Active`), *When* the tab strip renders, *Then* the "Summary" tab has `disabled: true` and `title="Summary is generated after the session ends."`, mirroring the existing Browser-tab `disabled`+`title` convention (`SessionDetailView.tsx:290,585`).
**Files**: `web-app/src/components/sessions/SessionDetail.tsx`, `web-app/src/components/sessions/SessionDetailView.tsx`

##### Task 3.2.1a: Extend `SessionDetailTab` union (~1 min)
- In `web-app/src/components/sessions/SessionDetail.tsx:28`, add `"summary"` to the `SessionDetailTab` union type.
- Files: `web-app/src/components/sessions/SessionDetail.tsx`

##### Task 3.2.1b: Add tab entry + panel render (~4 min)
- In `web-app/src/components/sessions/SessionDetailView.tsx`, add `{ id: "summary", label: "Summary", icon: FileText, disabled: !isSessionTerminal(session.status) }` to the `tabs` array (~line 283-291; add a small local `isSessionTerminal` helper checking `session.status` against the terminal `Status` values, or reuse an existing status-check utility if one already exists in this file — grep for `Status.Stopped`/similar comparisons in this file first). Add the tab panel: `{activeTab === "summary" && <SessionSummaryPanel sessionId={session.id} />}` alongside the other tab-panel conditionals.
- Files: `web-app/src/components/sessions/SessionDetailView.tsx`

##### Task 3.2.1c: Tab a11y — `aria-live` status + disabled tooltip (~2 min)
- Extend the existing `title={tab.disabled && tab.id === "browser" ? ... : undefined}` conditional (line 585) to also cover `tab.id === "summary"` with the text "Summary is generated after the session ends." Confirm the existing `role="tablist"`/roving-focus/`aria-selected` machinery (lines 558-581) requires no changes — new tabs get this for free.
- Files: `web-app/src/components/sessions/SessionDetailView.tsx`

##### Task 3.2.1d: Frontend integration test for the tab (~4 min)
- Extend/adjacent test file (find `SessionDetailView`'s existing test file via Glob, e.g. `web-app/src/components/sessions/__tests__/SessionDetailView*.test.tsx` — if none exists, add a focused new test file) asserting: Summary tab is `aria-disabled="true"` for an `Active` session; enabled and clickable for a `Stopped` session; clicking it renders `SessionSummaryPanel` with the correct `sessionId` prop.
- Files: `web-app/src/components/sessions/__tests__/SessionDetailView.summary-tab.test.tsx`

---

### Epic 3.3: Durable Standalone Route (post-deletion retrieval)

#### Story 3.3.1: `/sessions/[sessionId]/summary` route
**As a** user, **I want** a URL that shows a session's summary even after the session itself has been deleted, **so that** AC-3/AC-7 hold in full, not just for sessions still visible in the live list.
**Acceptance Criteria**:
- AC-3/AC-7: *Given* `session_id: "sess-123"` whose `Session` ent row has been deleted (via `DeleteSession`) and the server has since restarted, *When* a user navigates to `/sessions/sess-123/summary`, *Then* the page renders `SessionSummaryPanel` (fetching via `useSessionSummary("sess-123")`, which calls `GetSessionSummary` directly — no dependency on `useAppSelector(selectAllSessions)` or any `Session` proto object), showing the same `READY` document as before deletion.
**Files**: `web-app/src/app/sessions/[sessionId]/summary/page.tsx`

##### Task 3.3.1a: Create the route (~5 min)
- New `web-app/src/app/sessions/[sessionId]/summary/page.tsx` (Next.js App Router dynamic segment — confirmed no existing `web-app/src/app/sessions/[id]/` route exists today, only `sessions/new/`): `"use client"` page reading `sessionId` via `useParams()`, rendering a minimal page shell (title/back-link — no terminal/VCS/files tabs, no dependency on the Redux sessions list) wrapping `<SessionSummaryPanel sessionId={sessionId} />`. Reuses `SessionSummaryPanel.css.ts` styling; no new `.css.ts` file needed unless page-level chrome (header/back button) warrants one.
- **"Will never resolve" empty state** (closes `design/ux.md` surface (g)'s flagged secondary gap: without this, a typo'd/never-existing `sessionId` polls the GENERATING skeleton indefinitely, a dead end): this route relies entirely on `useSessionSummary`'s `neverResolved` flag (Task 3.1.1a) — once true, `SessionSummaryPanel` (Task 3.1.2a) already renders the terminal "No summary available for this session" state, and this page's only extra job is keeping the "← Back" link visible alongside it as the sole affordance. Per `design/ux.md`'s note that the RPC alone can't distinguish "will never exist" from "still running, not generated yet," the empty-state copy stays neutral ("No summary available for this session — it may still be running, or may not have generated one.") rather than a flat "not found."
- Files: `web-app/src/app/sessions/[sessionId]/summary/page.tsx`

##### Task 3.3.1b: Route smoke test (~3 min)
- New test (Jest + RTL, mocking `next/navigation`'s `useParams`) asserting the page renders `SessionSummaryPanel` with the `sessionId` taken from the route param, and renders successfully with no `Session` object / Redux store dependency (i.e. does not call `useAppSelector(selectAllSessions)` anywhere in the render tree it owns).
- Files: `web-app/src/app/sessions/[sessionId]/summary/__tests__/page.test.tsx`

#### Story 3.3.2: Discoverable entry point into the standalone route
**As a** user, **I want** at least one real, existing UI surface to route me to a deleted session's summary, **so that** AC-3/AC-7's "retrievable... not an orphaned write-only row" holds for a user who does not already have the exact `sessionId` memorized or bookmarked — without this, Epic 3.3's route is only reachable by URL, which the adversarial review confirmed is not scope creep to fix (no other existing surface reaches a deleted session's data today: `SessionDetail.tsx` requires a live `Session` prop, `/history` is unrelated, and `NotificationsPage.tsx`'s session links route back through the same live-list lookup that fails once the session is deleted).
**Acceptance Criteria**:
- *Given* a notification card in `NotificationsPage.tsx` referencing a `sessionId` whose live `Session` lookup fails (deleted session), *When* the user clicks that notification's session link, *Then* it navigates to `/sessions/<sessionId>/summary` instead of a dead link or a no-op.
**Files**: `web-app/src/components/notifications/NotificationsPage.tsx`

##### Task 3.3.2a: Fall back to the summary route in `NotificationsPage.tsx` (~4 min)
- In `web-app/src/components/notifications/NotificationsPage.tsx` (~line 402, the existing session-link click handler), when the live-session lookup used to build/navigate that link fails to resolve a `Session`, navigate to `/sessions/<sessionId>/summary` instead of the current live-list-dependent route. Keep the existing behavior unchanged for the common case where the live session still resolves.
- Files: `web-app/src/components/notifications/NotificationsPage.tsx`

##### Task 3.3.2b: Test the fallback (~3 min)
- Extend `NotificationsPage.tsx`'s existing test file (Glob to find it) with a case asserting: a notification for a `sessionId` with no resolvable live session navigates to `/sessions/<sessionId>/summary` on click, while a notification for a still-live session is unaffected.
- Files: existing `NotificationsPage` test file (extended)

---

## Phase 4: Registry & E2E

### Epic 4.1: Feature Registry

#### Story 4.1.1: Backend + frontend registry entries
**As a** maintainer, **I want** the new RPCs and UI features registered per `.claude/rules/feature-registry.md`, **so that** `make registry-diff` shows no unjustified coverage-gap growth.
**Acceptance Criteria**:
- *Given* the new `GetSessionSummary`/`RegenerateSessionSummary` RPCs and the Summary tab/route exist, *When* `make registry-generate` runs, *Then* `docs/registry/coverage-gaps.json`'s count does not grow (every new feature has a `tested: true` entry with real `testIds` from Phases 1-3's test tasks).
**Files**: `docs/registry/features/backend/GetSessionSummary.json`, `docs/registry/features/backend/RegenerateSessionSummary.json`, `docs/registry/features/frontend/session-summary-tab.json`, `docs/registry/features/frontend/session-summary-standalone-route.json`

##### Task 4.1.1a: Add `// +api:` markers + backend registry files (~4 min)
- Add `// +api: session-summary:get` above `GetSessionSummary` and `// +api: session-summary:regenerate` above `RegenerateSessionSummary` in `server/services/session_summary_service.go`. Create `docs/registry/features/backend/GetSessionSummary.json` and `.../RegenerateSessionSummary.json` matching the shape of `GetInsightsSummary.json` (`id`, `type: "backend"`, `service: "SessionSummaryService"`, `method`, `protoFile: "proto/session/v1/session_summary.proto"`, `markerFound: true`, `tested: true`, `testIds: [...]` from Task 2.2.2b, `lastModified`).
- Files: `server/services/session_summary_service.go`, `docs/registry/features/backend/GetSessionSummary.json`, `docs/registry/features/backend/RegenerateSessionSummary.json`

##### Task 4.1.1b: Add `// +feature:` markers + frontend registry files (~3 min)
- Add `// +feature: session-summary-tab` near the top of `SessionSummaryPanel.tsx` (per `.claude/rules/feature-registry.md`'s "first 10 lines" requirement). Create `docs/registry/features/frontend/session-summary-tab.json` and `.../session-summary-standalone-route.json` with `filePath` pointing at `SessionDetailView.tsx`/the tab entry and `web-app/src/app/sessions/[sessionId]/summary/page.tsx` respectively, `tested: true`, `testIds` from Tasks 3.2.1d/3.3.1b.
- Files: `web-app/src/components/sessions/SessionSummaryPanel.tsx`, `docs/registry/features/frontend/session-summary-tab.json`, `docs/registry/features/frontend/session-summary-standalone-route.json`

##### Task 4.1.1c: Run registry generation (~1 min)
- Run `make registry-generate`; run `make registry-diff` and confirm no unjustified growth in `coverage-gaps.json`; commit the regenerated aggregate files alongside the per-feature source files.
- Files: generated registry files (not hand-edited)

### Epic 4.2: End-to-End Test

#### Story 4.2.1: E2E coverage for the full pipeline
**As a** QA gate, **I want** an e2e test exercising trigger → generate → retrieve → copy, **so that** the feature is verified end-to-end, not just unit-by-unit.
**Acceptance Criteria**:
- AC-1 through AC-6, AC-8 (AC-7's post-deletion case is covered by the unit/integration tests in Epic 2.2/3.3 since e2e test-mode sessions are typically not deleted mid-suite — flag this as the one AC not independently re-verified at the e2e layer, acceptable given it's already covered at the RPC + component layer): *Given* the isolated e2e test server (per `tests/e2e/global-setup.ts`), *When* a trivial one-off session is created and stopped via the UI, *Then* the Summary tab becomes enabled, shows a READY document with the "no work recorded" empty-state narrative within the poll window, and the Copy button successfully copies GFM markdown to the clipboard (assert via the `aria-live` success announcement, or a `page.evaluate` clipboard read if an existing e2e clipboard-testing pattern is found in the suite).
**Files**: `tests/e2e/session-completion-summary.spec.ts`

##### Task 4.2.1a: Write the e2e spec (~5 min)
- New `tests/e2e/session-completion-summary.spec.ts`, starting with `// @feature session-summary-tab, session-summary-standalone-route` (per `.claude/rules/e2e-test-conventions.md`). Uses `data-testid`/ARIA-role locators only, no `waitForTimeout` (use `expect(locator).toHaveText(...)`/`toBeEnabled()` to wait for the GENERATING→READY transition). New page-helper methods added to `tests/e2e/pages/` if a `SessionDetailPage`-equivalent helper already exists (Glob `tests/e2e/pages/` first) rather than inlining navigation logic.
- Files: `tests/e2e/session-completion-summary.spec.ts`, possibly a new/extended file under `tests/e2e/pages/`
