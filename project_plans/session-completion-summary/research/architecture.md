# Architecture Research: Session Completion Summary

Agent 3 (Architecture). Scope: pattern selection, integration points, data-flow/
durability verification, and the FR-7 dedup mechanism. No Event-Command-Policy
table — this is a linear detect→snapshot→generate→persist→serve pipeline, not a
multi-actor domain (the one policy-ish piece, approval-decision aggregation, is a
straight counting/bucketing operation over already-classified records, not
worth a table).

## 1. Pattern: no exact precedent exists — compose three existing patterns

Searched for an existing "durable async artifact + state machine + Regenerate"
pattern to mirror wholesale. None exists as a single reusable abstraction. The
closest three partial precedents, and what to take from each:

**a) `instanceBacklogListener` (`session/backlog_lifecycle.go:791-811`) — the
lifecycle-listener shim shape.** Confirms the wiring pattern this feature should
copy exactly:
```go
type instanceBacklogListener struct {
    parent       *BacklogLifecycleListener
    instanceUUID string
}
func (il *instanceBacklogListener) OnLifecycleEvent(event LifecycleEvent, _ string) {
    switch event {
    case EventExited, EventStopped:
        go il.parent.onSessionExited(il.instanceUUID)
    }
}
```
A new `sessionSummaryListener` (or a method added to a new
`SessionSummaryService`) should follow this exact shape: a per-instance shim
capturing `instanceUUID`, registered via a `WireToInstance(inst *Instance)`
method, firing `go svc.onSessionEnded(instanceUUID, reason)` — never doing work
on the callback goroutine itself (satisfies the `LifecycleListener` doc
contract and FR-5's non-blocking-teardown requirement for free). **Reason
filtering happens inside the handler, not the switch**: reject
`reason == "reconcile-session-missing"` (FR-1's exclusion) at the top of
`onSessionEnded`, mirroring how `onSessionExited` already receives `reason` but
`instanceBacklogListener.OnLifecycleEvent` currently discards it (the `_`
param) — the new listener must NOT discard it.

**Own listener vs. extend `instanceBacklogListener`?** Own listener,
registered separately via its own `WireToInstance` call at the same call site
that wires `instanceBacklogListener` today (search for existing
`.WireToInstance(inst)` call sites — likely `session/manager.go` or wherever
instances are constructed/loaded). Reasons: (1) `instanceBacklogListener` is
scoped to backlog-item bookkeeping and is conditionally enabled
(`il.parent.enabled.Load()`) — summary generation must fire for *every*
session, backlog-linked or not (FR-1 says nothing about backlog scoping); (2)
keeping it separate avoids adding an eighth concern to an already
4200-line file; (3) it composes cleanly — both listeners subscribe to the same
`Instance` independently, `RegisterLifecycleListener` already supports
multiple listeners per instance (it's a slice: `lifecycleListeners
[]LifecycleListener` at `session/instance.go:392`).

**b) `headless.Pool` + `FeatureKey` (`session/headless/pool.go`,
`session/headless/features.go`) — the LLM-call pattern to use for the
narrative step, NOT `GetWorktreeAISummary`'s raw exec pattern.** The pool
already provides session reuse (prefix-cache optimization), a
`MaxConcurrentSessions` semaphore, and a `maxConsecutiveErrors` circuit
breaker — exactly what FR-5's "narrative failures degrade gracefully" needs
without reinventing retry/backoff. `session/headless/features.go` defines
typed feature functions keyed by `FeatureKey` constants
(`FeatureKeyPRDescription`, `FeatureKeyCommitMessage`, `FeatureKeySummarize`,
etc.), each with a stable system prompt for prefix-caching. Add
`FeatureKeySessionSummary` and a `GenerateSessionNarrative(pool, ...)`
function following that same shape.

By contrast, `UnfinishedWorkService.GetWorktreeAISummary`
(`server/services/unfinished_work_service.go:288-369`) shells out directly
(`exec.LookPath("claude")` + `safeexec.CommandContext` piping `git diff` into
`claude -p`), with its own bespoke semaphore (`aiSemaphore`) and per-key
`sync.Map` mutex for dedup. This is an older, one-off pattern predating (or
bypassing) `headless.Pool`'s generalized version of the same problem
(concurrency limiting + dedup + session reuse). **Do not copy this file's
approach** — it duplicates machinery `headless.Pool` already owns. It's useful
only as a secondary reference for the FR-7 per-key dedup mutex idiom
(`sync.Map` of `*sync.Mutex`), which is exactly what FR-7 needs (see §4).

**c) `ScanStatus` enum (`proto/session/v1/types.proto:1318-1325`) — the
status-enum naming/shape precedent for the summary's state machine.**
```protobuf
enum ScanStatus {
  SCAN_STATUS_UNSPECIFIED = 0;
  SCAN_STATUS_OK          = 1;
  SCAN_STATUS_TIMEOUT     = 2;
  SCAN_STATUS_PERMISSION  = 3;
  SCAN_STATUS_ERROR       = 4;
}
```
Mirror this shape for the new enum:
```protobuf
enum SessionSummaryStatus {
  SESSION_SUMMARY_STATUS_UNSPECIFIED = 0;
  SESSION_SUMMARY_STATUS_PENDING     = 1;  // queued, not yet started
  SESSION_SUMMARY_STATUS_GENERATING  = 2;  // deterministic snapshot + narrative in flight
  SESSION_SUMMARY_STATUS_READY       = 3;  // document available (narrative may have degraded to fallback text — still READY per FR-5)
  SESSION_SUMMARY_STATUS_ERROR       = 4;  // deterministic stage failed; Regenerate offered
}
```
Note FR-5's asymmetry: narrative (LLM) failure still yields `READY` (with
fallback text substituted for the narrative section); only a
**deterministic**-stage failure (diff/decisions/timeline/cost snapshot itself
erroring, e.g. can't compute diff at all) yields `ERROR`. This means the state
machine has a soft-fail branch inside the GENERATING→READY transition, not a
hard fork — model it as one boolean field (`narrative_degraded bool` /
`narrative_fallback_used`) on the READY document rather than a fifth enum
value.

**Rejected as a mirror candidate: `AutonomousDriver` / turn-based orchestration
loop** (`server/services/autonomous_orchestration_service.go`). This runs a
multi-turn agentic loop with its own driver registry, stuck-detection, and
review-gate triggering — built for an open-ended agent conversation, not a
single bounded generate-and-persist job. Using it here would be significant
over-engineering; note it explicitly so the plan phase doesn't reach for it.

**`PipelineEngine`** (`session/pipeline_engine.go:69`) is a prompt-template
provider (`SlashCommandSet`, `TriagePromptFor`, `ReviewPromptFor`, etc.), not a
job-runner — it's the right place to add a `SummaryNarrativePromptFor(...)`
method alongside the existing `*PromptFor` methods if the narrative prompt
needs to be pipeline-mode-overridable (matches how review/triage prompts are
customizable per backlog pipeline mode). Confirm in the plan phase whether
session summaries should be pipeline-mode-aware at all — the requirements
don't mention it, so this may be YAGNI for v1 (plain constant prompt is
simpler and consistent with `unfinished_work_service.go`'s single hardcoded
prompt).

## 2. Integration points

### 2a. Lifecycle listener
New type, e.g. `sessionSummaryListener` in a new
`session/session_summary_listener.go`, following `instanceBacklogListener`'s
shape (§1a). Owning service: a new `SessionSummaryService` (parallels
`AutonomousOrchestrationService`'s role as the owner of a lifecycle-driven
async subsystem) that holds the `headless.Pool` reference, the ent client (or
a narrow store interface over it), and the FR-7 in-flight guard.

### 2b. ent schema entity
New entity, `SessionSummary`, following the `AnalyticsEvent` plain-string
pattern (`session/ent/schema/analytics_event.go`) — **not** `DiffStats`'s
`edge.From("session", ...).Required()` pattern, per the requirements' explicit
warning (AC-3 needs the row to outlive the `Session` row; a required edge
would either block Session deletion via FK constraint or cascade-delete the
summary with it, either way defeating the requirement).

Proposed fields (illustrative, not final — plan phase should firm up types):
```go
field.String("id").Unique().NotEmpty().Immutable(),      // uuid
field.String("session_id").NotEmpty(),                    // plain string, no edge — survives Session row deletion
field.String("session_title").Optional(),                 // display fallback once Session row is gone
field.Int("status").Comment("PENDING/GENERATING/READY/ERROR"),
field.Text("narrative").Optional(),
field.Bool("narrative_fallback_used").Default(false),
field.JSON("diff_snapshot", DiffSnapshot{}).Optional(),
field.JSON("decisions_snapshot", DecisionsSnapshot{}).Optional(),
field.JSON("timeline_snapshot", []TimelineEntry{}).Optional(),
field.JSON("cost_snapshot", CostSnapshot{}).Optional(),
field.Text("markdown").Optional(),                        // final rendered GFM doc, cached for FR-4 export
field.Text("error_message").Optional(),
field.Time("generation_started_at").Optional().Nillable(),// FR-7 staleness check, see §4
field.Time("generated_at").Optional().Nillable(),
field.Time("created_at").Default(time.Now).Immutable(),
field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
```
```go
Indexes: index.Fields("session_id"),  // one-summary-per-session lookup; consider Unique() on session_id if the model is strictly 1:1 (Regenerate overwrites in place rather than appending)
```
`session_id` should be the session's stable `UUID` (matches
`AnalyticsEvent.session_id` and `NotificationRecord.SessionID`'s convention),
not `Title` (titles can be edited; UUID is immutable per
`session/instance.go`'s doc comment on `Instance.UUID`).

Regenerate semantics: decide in the plan phase whether Regenerate mutates the
existing row (status→GENERATING, clear narrative/markdown, keep same ID) or
inserts a new row and supersedes the old one. Mutate-in-place is simpler and
matches "one summary per session" from FR-3/FR-4's phrasing ("the" summary,
singular); an audit trail of regenerations is a non-goal not mentioned in
requirements.

Must run `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert
./session/ent/schema` after adding this schema file (`.claude/rules/ent-schema-generation.md`).

### 2c. ConnectRPC service + proto messages
New RPC(s) in `proto/session/v1/session.proto` (or a new
`session_summary.proto` if the plan phase prefers a dedicated file — check
repo convention; `unfinished.proto` and `headless.proto` are both split out as
dedicated files for their respective services, suggesting a dedicated
`session_summary.proto` fits the existing granularity better than piling onto
the already-large `session.proto`):
- `GetSessionSummary(session_id) → SessionSummary` — used by the new Summary
  tab; must work for a session_id whose live `Session` row is gone (queries
  the new ent table directly by `session_id`, not through `SessionService`'s
  existing `findInstance`/`ListInstanceData` machinery which requires a live
  or storage-backed instance).
- `RegenerateSessionSummary(session_id) → SessionSummary` (or empty response +
  client re-polls `GetSessionSummary`) — the FR-5/FR-7 Regenerate action.
  Idempotent per FR-7: if already GENERATING, either return the in-flight
  status (no-op) or reject with a clear "already generating" error — client
  should treat both identically (poll until READY/ERROR).

New enum `SessionSummaryStatus` goes in `types.proto` alongside `ScanStatus`
(§1c). Request/response messages plus the `SessionSummary` message itself
follow existing message conventions (see `UnfinishedWorktree` message shape in
`types.proto` for a same-file precedent of a status+payload struct).

Run `make proto-gen` after any proto change (regenerates
`session/gen/session/v1/*.go` and `web-app/src/gen/session/v1/*_pb.ts`).

Register the new service in `server/server.go` alongside the other
`*Service` registrations (same place `UnfinishedWorkService`,
`ReviewQueueService` etc. are wired).

### 2d. React component(s)/hook(s)
- New tab entry in `SessionDetailView.tsx`'s `tabs` array (~line 283) —
  `SessionDetailTab` type (defined in `SessionDetail.tsx`, imported at
  `SessionDetailView.tsx:43`) needs a new variant, e.g. `"summary"`.
  **Important**: this tab must remain reachable/functional even when the
  session itself is gone from the live/`ListInstanceData` view (FR-3's
  "reachable even once the live Session row is gone") — check whatever
  code currently gates `SessionDetailView`'s mount/tab-visibility on session
  liveness; if the parent view unmounts entirely once a session disappears
  from the list, the Summary tab needs its own route/access path independent
  of that gate (e.g. a `/sessions/:id/summary` deep link that only needs
  `session_id`, not a resolved live `Session`). Flag this as a plan-phase
  question — Agent 3's search of `SessionDetailView.tsx` didn't establish
  whether the view itself survives session deletion; the UX/frontend research
  agent should confirm.
- New hook `useSessionSummary(sessionId)` — model on `useGenerateRule.ts`'s
  shape (loading/error/data + an explicit trigger function) for the
  `Regenerate` action, but this hook additionally needs **polling** while
  status is GENERATING/PENDING (unlike `useGenerateRule`, which is a single
  request/response RPC with no server-side async job to poll). No existing
  hook in `web-app/src/lib/hooks/` does exactly this (checked
  `useApprovalAnalytics.ts`, `useBacklogItemShipStatus.ts` — the latter is the
  closest analog by name; worth checking in the plan phase whether it already
  implements a poll-until-terminal-status pattern worth copying instead of
  writing one from scratch).
- Copy-to-clipboard (FR-4): render `markdown` field via the browser Clipboard
  API — no server round-trip needed since the RPC already returns the cached
  `markdown` string; this is a pure frontend concern, not an architecture
  question.

## 3. Data flow and durability verification (AC-3 blocker check)

Requirements ask to confirm each data source remains readable after the
`Session` ent row is deleted. Traced `DeleteSession` (`server/services/session_service.go:1955-2034`):
call order is (1) cancel pending approvals, (2) **`s.storage.DeleteInstance(sessionTitle)`**
— this removes the row the moment `DeleteSession` RPC executes, several steps
*before* `inst.Destroy()` even runs (`Destroy()` is dispatched in a goroutine
earlier in the function, at line ~1999, racing independently of the storage
deletion) — then (3) publish `SessionDeletedEvent`. **The storage row is gone
essentially immediately, not after `EventStopped` fires** — confirms the
summary generation pipeline cannot rely on `Session`/`InstanceData` storage
still existing by the time its listener callback runs; it also can't reliably
read the row synchronously inside `OnLifecycleEvent` before storage deletion,
because `OnLifecycleEvent`'s own dispatch is `go`-deferred and storage
deletion is not sequenced against it. **This is the core design constraint,
not just a hypothetical**: the fire-and-forget goroutine in
`instanceBacklogListener.OnLifecycleEvent` already exhibits the same race
today for the backlog path — the plan phase must decide whether the summary
listener needs to snapshot from the *live in-memory* `Instance` object
(passed directly into the shim's closure, not looked up later by ID) rather
than re-reading storage, since `Instance` itself is still fully populated in
memory at `Destroy()`-call time before any deletion touches it. **Recommend
passing the `*Instance` pointer itself into the per-instance listener shim's
`onSessionEnded` (like `instanceBacklogListener` already captures
`instanceUUID`, capture the `*Instance` too, or at minimum snapshot
`instance.GetDiffStats()` synchronously inside `OnLifecycleEvent` before the
`go` dispatch, and only hand the async goroutine the already-extracted
struct)** — this sidesteps the storage-race entirely for the diff/instance
metadata portion of the snapshot.

Per-source verification:

- **Diff stats** — `GetSessionDiff` (`server/services/session_service.go:2586-2641`)
  has two paths: live-instance (`instance.GetDiffStats()`, works fine, in
  memory) and completed-session fallback (`s.storage.ListInstanceData()` scan
  by ID, then reconstructs a worktree and diffs on demand). **The fallback
  path depends on `ListInstanceData()` still containing the session** — per
  the trace above, that row is deleted essentially synchronously with
  `DeleteSession`, so this fallback is NOT usable once the row is gone. ✅ Not
  a blocker *if* the summary snapshots diff stats from the live `*Instance`
  synchronously at `EventStopped`/`EventExited` time (before storage deletion
  can race ahead) rather than trying to re-derive it later from
  `GetSessionDiff`. ❌ Would be a blocker if the design instead tried to call
  `GetSessionDiff`/`GetDiffStats`-equivalent lazily at *read* time (e.g. from
  the new `GetSessionSummary` RPC) after the Session row may already be gone
  — don't do this; snapshot the diff into the new `SessionSummary` row's
  `diff_snapshot` JSON field at generation time, never re-derive it at serve
  time.

- **Approval decisions / notification history** — `NotificationHistoryStore`
  is JSON-file-backed (`server/notifications/store.go`), independent of the
  ent `Session` row, so it superficially looks safe. **However**, it runs its
  own orphan-pruning sweep: `enforceRetention()` (called on every `Append`,
  `server/notifications/store.go:511-537`) invokes `pruneOrphanedRecords`
  against a `SetSessionExistenceLookup` callback wired in `server/server.go:220`
  (`buildSessionExistenceLookup`, `server/server.go:1036-1060`), which is
  itself backed by `storage.ListInstanceData()` — the same store that loses
  the row on `DeleteSession`. Records with `SessionScoped==true` and no
  `Metadata["item_id"]` get deleted once their `SessionID` no longer appears
  in that lookup (gated by a 5-minute-uptime safety window,
  `pruneOrphanedMinUptime`, but no comparable "wait until the summary has
  been generated" grace period). **This is a genuine, confirmed blocker for
  any design that reads approval/notification data lazily** — a summary
  generated even a few minutes late (e.g. queued behind FR-7's dedup guard,
  or a slow narrative call) risks its source notification records already
  being orphan-pruned out from under it. ✅ Not a blocker if approval-decision
  counts are snapshotted into `decisions_snapshot` synchronously as part of
  the deterministic generation stage, immediately on `EventExited`/
  `EventStopped`, well before any plausible prune sweep interval. Flag this
  explicitly in the plan phase: generation must read `NotificationHistoryStore`
  (via `List(ListOptions{SessionID: ...})`) as close to session-end as
  possible, not on a delay.

- **Token usage / cost** — `session/tokens.TokenStore.GetByUUID(uuid)`
  (`session/tokens/store.go:113`) parses Claude Code's own JSONL transcript
  history files from disk, keyed by session UUID — **not** derived from the
  ent `Session` row or `InstanceData` at all. This source is durable
  independent of `DeleteSession` (the transcript files aren't touched by
  session deletion). ✅ Confirmed safe to read even after the `Session` row
  is gone, though still recommend snapshotting into `cost_snapshot` at
  generation time anyway, both for consistency with the other sources and
  because Claude's own transcript retention/rotation policy is out of this
  feature's control.

- **Timeline** — no single existing source; will need to be assembled from
  whatever mix of `NotificationHistoryStore` records + instance start/stop
  timestamps + (if in scope) tmux/scrollback markers the plan phase decides
  on. Same snapshot-at-generation-time rule applies transitively, since it's
  downstream of notification history.

**Summary of the AC-3 finding**: no data source is unconditionally safe to
read lazily at serve time after `Session`-row deletion. Diff stats and
notification-derived data are actively unsafe past a short window (diff via
storage-row deletion racing `Destroy()`; notifications via the
5-minute-gated orphan-prune sweep). Token/cost data is the only source that's
independently durable. **The architecture must snapshot every FR-2 data
category into the new `SessionSummary` row synchronously during the
deterministic generation stage, triggered as early as possible after the
lifecycle event fires** — this isn't just good practice, it's required for
correctness given the orphan-pruning and storage-deletion races documented
above. This should be called out as a hard constraint in the plan phase, not
left as a "nice to have."

## 4. FR-7 dedup mechanism

Two failure modes to cover:

1. **Concurrent/duplicate fires within one running process** (e.g.
   `EventExited` then `EventStopped` both firing for the same session-end, or
   a double-click on Regenerate) — an in-memory guard is sufficient here.
   Follow the `.claude/rules/go-double-checked-locking.md` convention (the
   canonical implementation is `IsDirty` in `session/git/worktree_git.go`):
   read-lock check → if already GENERATING for this session_id, return/no-op
   → write-lock → re-check → mark GENERATING → do work → always use the
   locally-computed result, never re-read a shared slot afterward. A
   `sync.Map[string]*sync.Mutex` keyed by `session_id` (the same idiom
   `UnfinishedWorkService.aiMu` already uses at
   `server/services/unfinished_work_service.go:311-314`, and worth reusing
   verbatim rather than reinventing) is enough to serialize concurrent calls
   for the same session_id within a process.

2. **Crash/restart leaving a row stuck in GENERATING** — the in-memory guard
   above is process-local and provides *zero* protection here: if the process
   crashes mid-generation, the in-memory mutex/map entry is gone on restart,
   but the persisted `SessionSummary` row is left with `status=GENERATING`
   forever, and nothing would ever flip it to ERROR or retry it — the Summary
   tab would show a permanently spinning/stuck state. **This does need to
   survive a restart, confirming the requirements' hint.** Recommend:
   - Persist `generation_started_at` (timestamp) when transitioning
     PENDING→GENERATING (already in the proposed schema, §2b).
   - On **read** (`GetSessionSummary`), if `status == GENERATING` and
     `now - generation_started_at > staleGenerationTimeout` (e.g. a small
     multiple of the narrative call's own timeout — the LLM call already has
     an implicit bound via `headless.Pool`'s call semantics; reuse or slightly
     pad that), treat it as stale: flip to `ERROR` with an explanatory
     `error_message` ("generation did not complete, possibly due to a server
     restart") and surface the Regenerate action, same as any other ERROR
     state. This is a lazy staleness check at read time, not a background
     sweep — simpler, and consistent with this feature's "don't add new
     background reconciliation loops for a v1" scope (no `ReconcileStuck`-
     style periodic sweep is implied by any FR here; the existing
     `BacklogLifecycleListener.ReconcileStuck` machinery is a much heavier
     precedent and out of scope for a single-artifact feature like this).
   - No need for a separate persisted "lock" table/row — the `status` field
     itself plus `generation_started_at` is the persisted lock, and the
     staleness check on read is the recovery mechanism. This avoids adding a
     second source of truth (a lock row that could itself go stale
     independently of the status field).
   - The in-memory guard (mode 1) and the persisted-staleness check (mode 2)
     are complementary, not redundant: the in-memory guard prevents wasted
     duplicate LLM calls/goroutines within a live process; the persisted
     staleness check is the only thing that can recover a row after a crash,
     since there's no in-memory state left to consult.

## Open questions to carry into the plan phase

- Whether `SessionSummary` should be `session_id`-unique (overwrite-in-place
  on Regenerate) or allow multiple rows per session (audit trail) — requirements
  read as singular ("the" summary), recommend unique.
- Whether the Summary tab/route needs to work when the parent
  `SessionDetailView` itself is gated on session liveness (§2d) — needs
  frontend-research-agent input, not resolved here.
- Whether `SessionSummaryService` should be a new file/service or a method
  set on an existing service — recommend new, small, single-purpose service
  (mirrors `AutonomousOrchestrationService`'s standalone-service precedent)
  rather than growing `SessionService` or `BacklogLifecycleListener` further.
