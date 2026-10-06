# Research: Architecture — durable-guidance-request

**Date**: 2026-09-12
**Agent**: 3 (Architecture), SDD Phase 2

## 1. Prior art already covers most of the hard structural decisions

Two prior SDD projects overlap this feature almost completely and must be built
on, not re-derived:

- `project_plans/backlog-stuck-item-visibility/decisions/ADR-001-durable-stuck-state-storage-model.md`
  (Accepted) gives the durable-notify-state template this feature's own
  constraint explicitly demands: one row per key, atomic
  `INSERT ... ON CONFLICT` upsert (`session/ent/schema/backlog_stuck_state.go:87-91`
  — a **plain, non-partial** 2-column unique index, chosen specifically because
  SQLite treats `NULL` as distinct in unique indexes, which rules out an
  append-only/episode-log design as an `OnConflictColumns` target), resolve-in-place
  (reopen a closed row rather than insert a new one), and a periodic **self-heal
  sweep** as the backstop for races the atomic pre-filter can't cover
  (ADR-001 "Consequences", `backlog-stuck-item-visibility/decisions/ADR-001-durable-stuck-state-storage-model.md:148-159`).
- `project_plans/backlog-agent-communication/` (2026-07-23, all three ADRs still
  **Proposed**, never implemented) is direct, unimplemented prior art for
  "an agent needs to ask for help," from the escalation angle rather than
  structured Q&A. Its `request_help` / `RespondToHelpRequest` design
  (`implementation/plan.md:429-541`) already solved the two hardest sub-problems
  this feature also has: **duplicate-request rejection** (Story 5.1.1, one open
  row per item) and **durable delivery to a session that may no longer be
  running** (Story 5.1.2 — live delivery via `write_to_session` when a session
  is still up, a `resume_session` respawn flag via `headless.Pool` when it
  isn't, and a `get_backlog_item`-surfaced fallback so a persisted response is
  never silently undelivered).

## 2. The live design fork: should `GuidanceRequest` converge with ADR-001/ADR-002, or stay a new entity?

**Recommendation: `GuidanceRequest` is a new, deliberately separate entity —
same recommendation ADR-002 already reached for `InfraIssueReport`, for the
same reason.** Do not fold it into `BacklogStuckState`/`StuckReason`.

Reasoning, mapped onto the actual fork:

- **ADR-001's reuse (`StuckReasonHelpRequested`, `StuckReasonVerdictDisputed`)
  works because both new signals are item-scoped 1:1** — every stuck reason,
  agent-initiated or reconciler-detected, is keyed on `(item_id, reason)`
  (`session/ent/schema/backlog_stuck_state.go:87-91`) and resolves through a
  human free-text response with no typed answer shape. `GuidanceRequest`'s
  scope enum (`backlog-item | session | standalone`, per requirements.md) means
  **two of its three scopes have no owning `BacklogItem` at all** — exactly the
  shape ADR-002 rejected `BacklogStuckState` for ("`related_item_id` as an
  *optional* field... rather than forcing it into `BacklogStuckState`'s
  item-required shape," `ADR-002-global-infra-issue-report-entity.md:26-30`).
  Forcing `session`/`standalone`-scoped guidance requests into an
  item-required table would misrepresent scope exactly the way ADR-002
  describes, and — worse — would sit alongside `StuckReason`'s "something needs
  automated attention" semantics (backoff, self-heal, `/unfinished` stuck-card
  rendering) when a guidance request is a **typed Q&A object with a captured
  answer value**, not an attention flag. `StuckReason`'s per-row shape has no
  field for "the human's answer" or "which of these N options was picked" —
  bolting that on would be exactly the kind of forced-fit ADR-002 already
  named and avoided once.
- **What *should* be reused is the pattern, not the table**: `GuidanceRequest`
  should copy `BacklogStuckState`'s atomic-upsert/resolve-in-place storage
  contract (a plain unique index scoped to whatever "one live request" means
  per scope — see the Event-Command-Policy table's dedup row) and copy
  `backlog-agent-communication`'s `RespondToHelpRequest` delivery design
  (live-session delivery + resume-fallback + durable-fallback-via-read), not
  the entity itself. This is the same relationship `InfraIssueReport` already
  has to `BacklogStuckState` today: sibling durable-signal tables sharing an
  upsert idiom, not a merged schema. Requirements.md's own scoping (a
  dedicated `GuidanceRequest` entity with `question`, `type`, `options`,
  `scope`, `status`, `answer`, `notified_at`/`answered_at`) already reflects
  this — this research confirms it against the two concrete precedents rather
  than leaving it assumed.
- **Ownership-check reuse still applies only to the `backlog-item` scope.**
  `resolveItemLink` (`server/mcp/tools_backlog.go:585-617`) is inherently
  item-anchored (it resolves a caller session ↔ `BacklogItem` link and
  distinguishes `ITEM_NOT_FOUND` from `PERMISSION_DENIED`). For
  `session`-scoped and `standalone`-scoped requests there is no backlog item
  to link against; those scopes need their own, simpler ownership check
  (e.g. "does the calling session UUID match the request's originating
  session," or, for `standalone`, no session-identity check at all since
  nothing but the human answers it). This is a new, small piece of logic —
  not a gap prior art already closes — and should be scoped explicitly in
  Phase 3 planning rather than assumed to reuse `resolveItemLink` uniformly
  across all three scopes.

## 3. Codebase verification of the atomic-upsert/event-bus pattern

- `session/ent/schema/backlog_stuck_state.go:26-92`: confirms the concrete ent
  shape — nullable `notified_at`/`resolved_at` timestamps (NULL = open/not yet
  fired), a `context` free-text field, and the single `index.Fields("item_id",
  "reason").Unique()` (line 90) that is both the correctness guarantee and the
  `OnConflictColumns` upsert target. `GuidanceRequest` should mirror this
  nullable-timestamp idiom for `notified_at`/`answered_at` per requirements.md.
- `server/services/backlog_item_event_publisher.go:32-57`: `PublishItemChanged`
  is the **entire** adapter surface between the session package and
  `*events.EventBus` — it wraps a fixed `BacklogItemEventPayload` shape
  (`Kind`, `Item`, `OldStatus`/`NewStatus`, `SessionID`, etc.) and an explicit
  `mapBacklogChangeKind` switch that **panics on an unmapped kind** (caught by
  the adapter's own `recover()`, never propagated) — i.e. adding a new event
  kind (e.g. `ChangeGuidanceRequestAnswered`) requires touching both
  `session.BacklogChangeKind` and this switch, or the publish path panics
  (harmlessly, but silently drops the event) at runtime. This is the exact
  seam a `GuidanceRequestAnswered` event must be threaded through if guidance
  requests piggyback on `BacklogItemChangedEvent` for the `backlog-item`
  scope; `session`/`standalone` scopes have no `BacklogItem` payload to attach
  to and need either a new `events.Event` variant or a synthetic
  zero-value-item payload — a decision Phase 3 planning must make explicitly,
  not infer.

## 4. Open Question resolved: `wait_for_backlog_event` does NOT support restart-reconnect delivery

`waitForBacklogEvent` (`server/mcp/tools_backlog.go:775-874`) is **only a live
blocking call**, bounded to at most 60 seconds
(`server/mcp/tools_backlog.go:793-799` clamps `timeoutSecs` to ≤60): it
subscribes to the in-process `*events.EventBus` for the duration of one call
(`h.eventBus.Subscribe(waitCtx)`, line 809), and unsubscribes via `defer` when
the call returns or times out. There is **no persistence of "this session is
waiting for event X" across a process restart or a dropped MCP connection** —
if the daemon restarts, or the calling session's tmux/headless process dies,
the subscription is gone and nothing redelivers the event when a new session
reconnects. The tool's own timeout message even tells the agent to fall back
to `ScheduleWakeup`/polling once it decides not to keep re-blocking
(`server/mcp/tools_backlog.go:834`).

**Consequence for `GuidanceRequest`'s "notify the originating session when
answered" requirement**: `wait_for_backlog_event` can serve a session that
stays alive and keeps re-calling it (loop of ≤60s blocking calls), but it
cannot be the **sole** durable-delivery mechanism for a question that may take
a human hours to answer, nor for a session that exited (or was reaped) while
waiting. The feature must follow `backlog-agent-communication`'s
`RespondToHelpRequest` design (`implementation/plan.md:497-522`): live-session
delivery via `write_to_session` when a session is still up, a
`resume_session`-style respawn via `headless.Pool` when it isn't, and the
answer also persisted and surfaced through a plain read path (`get_backlog_item`
equivalent, or a new `GetGuidanceRequest`) as the durable fallback that never
depends on delivery timing. This satisfies the "no new `ScheduleWakeup`-style
poll loop" constraint because the actual durable state (has this been
answered) lives in the `GuidanceRequest` row itself, read on demand — the
event bus is a **best-effort low-latency nudge**, never the only path to the
answer.

## 5. Open Question resolved: automated triage is a re-entrant per-item session, not a long-lived loop — "halt and resume" is not cheap

Automated triage runs as a **headless, single-shot Claude Code session per
item**, spawned through `BacklogService.SpawnSessionFromItem`
(`server/services/backlog_service_triage.go:597`) using the same
`headless.Pool` session-creation path work/review sessions use, ending when
the session calls `submit_triage_result` once
(`server/mcp/tools_backlog.go:2676`, tool registration at `:3186`) or exits.
There is **no persistent in-process "triage loop"** that a `GuidanceRequest`
could simply block inside and later resume in place.

Corroborating evidence: `session/backlog_lifecycle_triage.go`'s
`reconcileOrphanedTriageItems` (`:172-320`) already exists specifically to
detect a triage session that **ended without producing a usable plan** and
treats it as "orphaned," retrying it with a backoff/penalty path
(`retryOrphanedTriageWithBackoffGate`, `:372`) — the codebase's own current
model is that a triage session going away mid-work is an anomaly to detect
and recover from, not a suspended-and-resumable unit of work.

**Consequence for AC3 ("triage creates a request and halts on ambiguity")**:
"halt" cannot mean "the Go process blocks a goroutine waiting for an answer" —
triage isn't a Go-side loop, it's a Claude Code session. The realistic
options, in order of fit:

1. **The triage session itself loops on `wait_for_backlog_event`-style
   short calls** (bounded, ≤60s each) until answered or its own turn/time
   budget runs out, then exits normally leaving the item in a
   "waiting-for-guidance" sub-state. Cheap to implement, but only covers
   fast answers (minutes) before the session gives up and exits anyway —
   so option 2 is still required as the general case.
2. **The triage session creates the `GuidanceRequest` and exits deliberately**,
   and answering the request triggers a `headless.Pool` respawn of a **fresh**
   triage session seeded with the original triage context plus the answer —
   structurally identical to `RespondToHelpRequest`'s `resume_session` path
   (`backlog-agent-communication/implementation/plan.md:510-514`). This is the
   general-case answer and should be the one Phase 3 plans for.
3. **`reconcileOrphanedTriageItems` must be taught to recognize "orphaned
   because it's waiting on an open `GuidanceRequest`" as a distinct, expected
   state**, not an anomaly — otherwise the existing orphan detector will
   misclassify a legitimately-paused triage session as broken and retry it
   with a backoff penalty (`retryOrphanedTriageWithBackoffGate`) while a
   `GuidanceRequest` is still open, defeating the halt. This is a required
   touch point, not an optional nice-to-have, and should be named explicitly
   as a task in Phase 3's plan rather than discovered during implementation.

So: AC3's halt-and-resume is not expensive computationally, but it is **not
free structurally** — it requires the same session-exit/respawn plumbing as
`backlog-agent-communication`'s Epic 5, plus one required edit to the existing
orphan-detection reconciler so it doesn't fight the new halt state.

## 6. Feature-flag scaffolding to replicate for the AC3 halt-and-wait flag

The exact pattern to copy is `config.StreamHubFeatureFlag`
(`config/config.go:463`) + `EffectiveStreamHubEnabled`
(`config/config.go:523-525`, a one-line `cfg.GetFeatureFlagWithDefault(key,
default)` call) + a per-scope override map (`StreamHubSessionOverrides
map[string]bool`, `config/config.go:411`) + a small `*RolloutService` struct
exposing `Get*RolloutStatus`/`Set*GlobalOverride`/`Set*SessionOverride` RPCs
(`server/services/stream_hub_rollout_service.go:25-145`, fully self-contained,
no second implementation — matches the `interface-pollution-checklist`
concrete-type guidance already called out in that file's own doc comment).
For AC3, the global flag would be a new `config.FeatureFlags["triage_guidance_halt"]`
key with its own `EffectiveTriageGuidanceHaltEnabled(cfg)` accessor, and the
override scope would be per-backlog-item (or per-item-category) rather than
per-session, following `StreamHubSessionOverrides`' map-keyed-by-string shape.
`tymux`'s flag (`config.TymuxFeatureFlag`, `config/config.go:466-470`,
defaults **off** unlike `stream_hub`'s default-on) is the better default-value
precedent here — AC3's halt behavior should default off until proven safe,
exactly like `tymux`, not default-on like `stream_hub` (which had a rollback
rehearsal already completed before shipping).

## 7. Event-Command-Policy table

| Domain Event | Policy trigger | Command | Actor / System |
|---|---|---|---|
| `GuidanceRequestCreated` | An agent (backlog session, triage session, standalone workflow) determines it needs a structured answer | `CreateGuidanceRequest(scope, question, type, options, related_item_id?)` | Asking session (via new MCP tool) |
| — (pre-command guard) | A `CreateGuidanceRequest` call targets an item-scoped request | Ownership check: `resolveItemLink`-equivalent for `backlog-item` scope; new session-identity/no-op check for `session`/`standalone` scopes (§2) | System (MCP handler), before persisting |
| — (pre-command guard) | A `CreateGuidanceRequest` call would exceed the per-scope cap on pending requests | Reject with a clear error (mirrors `request_help`'s duplicate-rejection message shape, `implementation/plan.md:448-451`) | System (MCP handler) |
| — (pre-command guard) | Two concurrent `CreateGuidanceRequest` calls race for the same dedup key (e.g. same item + same question fingerprint, or "only one pending request per scope key") | Atomic `INSERT ... ON CONFLICT` upsert on the scope's unique index (ADR-001 pattern, `backlog_stuck_state.go:87-91`) — second caller gets the existing row back, not a duplicate | System (storage layer) |
| `GuidanceRequestPersisted` | `CreateGuidanceRequest` succeeds | Publish `GuidanceRequestCreated`-shaped event onto `*events.EventBus` (new `BacklogChangeKind`/event variant per §3, or a new standalone event type for non-item scopes) + fire `Notifier.Notify` for human visibility | System (storage + event publisher) |
| `TriageHaltedOnAmbiguity` | Triage session hits an ambiguous requirement and `EffectiveTriageGuidanceHaltEnabled(cfg)` is true | `CreateGuidanceRequest(scope=backlog-item, ...)` then exit the triage session cleanly (§5) | Triage session |
| — (reconciler guard) | `reconcileOrphanedTriageItems` runs its periodic sweep and finds a triage session that ended | Check for an open `GuidanceRequest` tied to the item **before** classifying as orphaned/retrying with backoff (§5, item 3) | System (`BacklogLifecycleListener`) |
| `GuidanceRequestAnswered` | The human operator answers via the shared React component (backlog item detail, triage panel, or session view) | `AnswerGuidanceRequest(request_id, answer, if_version)` — status-guard: only transitions `pending → answered`, rejects a second answer attempt on an already-answered row (duplicate-answer guard) | Human operator (via web UI RPC) |
| — (delivery) | `AnswerGuidanceRequest` succeeds | If the originating session is live: deliver via `write_to_session` (mirrors `RespondToHelpRequest`, `plan.md:502-507`). Else: respawn via `headless.Pool` if the scope's semantics call for resuming (e.g. triage), and always persist for on-demand read as fallback (§4) | System (RPC handler + delivery adapter) |
| `GuidanceRequestNotified` | Delivery (live write or respawn) completes, or the answer is read on-demand by a reconnected session | Set `notified_at` (nullable timestamp, ADR-001 idiom) | System (storage layer) |
| `GuidanceRequestSelfHealed` | Periodic self-heal sweep (ADR-001 backstop pattern, `ADR-001:148-159`) finds a `GuidanceRequest` answered but never marked notified, or open past a sane staleness bound | Re-attempt delivery / surface via `/unfinished`-equivalent view | System (self-heal sweep, same cadence as `ReconcileStuck`) |

## 8. Tech Debt Disposition

**Extend as-is, no violation found.** The touched areas —
`server/mcp/tools_backlog.go`'s handler-per-tool structure,
`session/ent/schema/*` entity-per-concern layout, the
`config.FeatureFlags` + `*RolloutService` scaffolding, and the
`*events.EventBus`/`BacklogItemEventPublisher` adapter — are all consistent,
already-idiomatic patterns for this exact class of feature (a new durable
signal + MCP tool + event-bus notification + feature-flagged rollout). The
one required structural touch point outside pure addition is
`reconcileOrphanedTriageItems` (§5, item 3), which needs a new branch, not a
refactor, to recognize an open `GuidanceRequest` as an expected pause state.
No seam or refactor-first work is needed before building this feature.

## Sources

- `project_plans/backlog-stuck-item-visibility/decisions/ADR-001-durable-stuck-state-storage-model.md`
- `session/ent/schema/backlog_stuck_state.go:26-92`
- `server/services/backlog_item_event_publisher.go:1-87`
- `project_plans/backlog-agent-communication/requirements.md`
- `project_plans/backlog-agent-communication/research/architecture.md`
- `project_plans/backlog-agent-communication/decisions/ADR-001-agent-initiated-stuck-reason-rows.md`
- `project_plans/backlog-agent-communication/decisions/ADR-002-global-infra-issue-report-entity.md`
- `project_plans/backlog-agent-communication/decisions/ADR-003-defer-persistent-master-agent.md`
- `project_plans/backlog-agent-communication/implementation/plan.md:420-549`
- `server/mcp/tools_backlog.go:585-617` (`resolveItemLink`), `:775-874` (`waitForBacklogEvent`), `:2676-2860` (`submit_triage_result`)
- `server/services/backlog_service_triage.go:597` (`SpawnSessionFromItem`)
- `session/backlog_lifecycle_triage.go:172-320` (`reconcileOrphanedTriageItems`), `:372` (`retryOrphanedTriageWithBackoffGate`)
- `config/config.go:326-329,403-480,485-525` (`FeatureFlags`, `StreamHubFeatureFlag`/`TymuxFeatureFlag`, `Effective*Enabled`)
- `server/services/stream_hub_rollout_service.go:1-146`
- `session/backlog_remediation.go:31,51` (`remediationBackoffSchedule`, `MaxRemediationAttempts`)
- `web-app/src/lib/hooks/useWatchBacklogItems.ts:1-30`
