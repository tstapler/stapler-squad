# Requirements: durable-guidance-request

**Date**: 2026-09-12
**Type**: feature addition
**Complexity**: 3 — system design

## Problem Statement
No stapler-squad-managed agent (a backlog work session, automated triage, or a background workflow) has a durable way to ask the user a structured question and get notified asynchronously when it's answered. Today the only mechanism is a live chat turn in an interactive session. Anything running unattended has no way to pause and request guidance — it must guess, proceed silently, or block forever waiting on a chat turn that may never come (backlog item `1c08da73-d569-47df-ab9b-676301748159`).

## Baseline
- Interactive sessions: the agent can ask via a live chat turn (`AskUserQuestion`), but this requires the session to be live and the user to be actively watching it.
- Unattended flows (automated triage, paused/idle sessions, background workflows): no ask mechanism exists at all. Automated triage currently guesses or proceeds silently on ambiguous items rather than pausing (the exact gap AC3 closes).
- Sessions currently learn about async backlog changes by polling via `ScheduleWakeup` (memory: `project_backlog_wakeup_polling.md`, tracked as ssq#428) rather than a durable event subscription — the new feature must not add another poll loop to this pile.

## Users / Consumers
- **Askers**: any backlog-linked work session, automated triage runs, and (per Answerer decision below) no other programmatic answerer for v1.
- **Answerer**: the human operator (Tyler), via the web UI only — no MCP tool for agent-submitted answers in this version.
- **Consumers of the answer**: the originating session (resumed or fresh) reading back the durable record; automated triage resuming a halted run.

## Success Metrics
- An automated triage run that hits an ambiguous item creates a guidance request and halts, instead of guessing or silently proceeding (AC3) — measured by: for every `triage_guidance_halt=true` run that determines genuine ambiguity, plan.md's Observability Plan `create` log event (scope, question_id, question_type — see plan.md's Observability Plan) is present and precedes the run's exit with no `submit_triage_result` call, queryable by joining the create event's `question_id` against that item's triage-session logs; zero flag-enabled runs show an exit-on-ambiguity with no corresponding create event.
- A question created by a now-dead/paused session is still answerable, and the answer is durably retrievable by a fresh session picking up the same backlog item — measured by: an answer survives a full service restart between creation and read-back (mirrors the stuck-review durability bar).
- An operator answering a guidance request gets a working session back promptly, not a stalled item: median time from `GuidanceRequest` creation (`created_at`) to answer (`answered_at`) — measured by: querying the entity's own stored timestamps (both already in scope, per the entity field list above) across all answered requests in a rollout window; this is the outcome an operator would actually recognize as "the feature is working" (fast turnaround, few stale pending questions), as distinct from the durability/structural metrics below. **Target: median <24h during business-hours operation.** This is a starting baseline, not an SLA — this is a single-operator tool and the operator may go hours without checking it, so a tight commitment would be dishonest. Revisit the number after the first month of real (non-test) usage once actual answer latency is observable.

(A third structural criterion — "one component file used by all 3 views" — was originally listed here but is a code-structure test, not a product outcome; it now lives solely as AC4/Story 6.1.1's acceptance criteria in plan.md, not duplicated here.)

## Appetite
Medium-Large (2-3 weeks) — revised up from the original "Medium (1-2 weeks)"
estimate. plan.md's finished breakdown is 8 phases / 10 epics / 22 stories / 53
tasks (verified via direct count against `implementation/plan.md`), spanning a
new ent schema + proto/RPC layer, a new event-bus payload type, a
feature-flagged triage control-flow change with a new non-cascading FK and
orphan-detector branch, a genuinely-new three-host shared React component
(flagged in Rabbit Holes as no existing "one component, three views" pattern
to copy), a nav badge, and a self-heal sweep — even though most individual
tasks are estimated small (2-5 min each; the raw per-task estimates sum to
~4 hours, which undercounts real elapsed time once test-writing, build/lint
cycles, cross-phase integration, and review are included). Per this process's
own rule ("if scope doesn't fit the appetite, cut scope — do not move the
deadline"), the honest question is whether Phase 7 (nav badge) and Phase 8
(self-heal sweep) should be cut instead of moving the estimate — they should
not: Phase 7 is the only mechanism satisfying AC2's "durable notification...
independently verifiable" bar system-wide (design/ux.md §3.1 flags the card's
own local echo as insufficient on its own), and Phase 8 is the actual
correctness backstop for AC5's dedup/cancellation guarantees per the accepted
ADR-001 posture (best-effort precondition + sweep, not cross-table atomicity)
— both are durability/discoverability-load-bearing scope, not optional
polish, so the estimate moves instead of the scope.

**Why ~4h of raw task-time maps to a 2-3 week appetite (~20-30x), stated
honestly rather than left as "there's a lot of tasks":** the per-task
estimates are pure coding time for a single task in isolation — they exclude
review cycles, the 2-repair-loop planning overhead already observed in this
very SDD process (the architecture, adversarial, and triad review rounds
this project itself went through before implementation could start), test-
writing time beyond what each task's own estimate covers (unit tests,
concurrent-race tests, and integration tests are separate line items, not
padding on the task estimates), and realistic single-person context-
switching across a 53-task/22-story plan spanning 8 phases. This ratio is a
normal planning-estimate-to-wall-clock multiplier for a project this size
under this SDD process generally — it is not specific to something unusually
hard about this feature.
*(v1 ships the core loop end to end for all 3 views with the three form types — yes/no, multiple-choice, short-answer — a basic per-item pending cap, and durable notification. Richer UX polish (e.g. inline editing of a pending question, rich text answers) is deferred to follow-up work.)*

## Constraints
- Feature-flag mechanism for AC3 (triage halt-and-wait) must use the existing live-settable rollout pattern — `config.FeatureFlags`-backed global default + per-scope override (see [ADR-003](decisions/ADR-003-global-only-triage-halt-flag.md) for the accepted v1 exception: global-only, per-item override deferred as a low-risk additive follow-up — no identified v1 consumer needs per-item divergence), both settable at runtime via the flag's own RPC + settings panel, **no env var, no rehearsal-gate precondition** — mirroring `stream_hub`/`tymux`/`native_git_*` (memory: `feedback_rollout_flags_live_settable_no_env_vars.md`). Do not gate this behind an env var "for convenience."
- Must not add a new `ScheduleWakeup`-style poll loop for notification delivery — ride the existing `BacklogItemEventPublisher`/`EventBus` pattern that `wait_for_backlog_event` and `useWatchBacklogItems.ts` already use.
- Durable notify state must follow the `BacklogStuckState` precedent (ADR-001): one row per scope with a unique key, atomic upsert (`INSERT ... ON CONFLICT`), resolve-in-place — never an in-memory dedup map that resets on restart. This is the direct fix target for AC5 ("avoiding the known notify-once/stuck-review bug class").
- Authorization for item-scoped requests must reuse the existing `resolveItemLink`-style ownership check (`server/mcp/tools_backlog.go`), not a new ACL scheme.

## Non-functional Requirements
- **Performance SLO**: not specified — this is low-QPS, human-latency-bound (question creation/answer, not a hot path).
- **Scalability**: expected volume is low (single-digit pending questions per item/session at a time, per-scope cap enforces this — AC7).
- **Security classification**: internal — single-operator deployment, no multi-tenant concern, but AC6's ownership check still applies (a session must not create requests scoped to items/sessions it doesn't own).
- **Data residency**: not applicable.

## Scope
### In Scope
- A durable `GuidanceRequest`/"Question" ent entity (question text, type: yes-no | multiple-choice | short-answer, options for multiple-choice, scope: backlog-item | session | standalone, status: pending | answered, answer value, `notified_at`/`answered_at` timestamps), proto messages, and CRUD/query RPCs.
- MCP tool(s) for a session to create a guidance request and to read back its current state/answer.
- One shared React component rendering a pending/answered guidance request, used from backlog item detail, the triage panel, and the session view (AC4).
- Durable, event-bus-based notification to the originating session when answered (AC2), following the `BacklogStuckState` atomic-upsert/resolve-in-place pattern (AC5).
- Automated triage creating a guidance request and halting on ambiguity, gated behind the live-settable feature flag described in Constraints (AC3).
- Ownership check on creation reusing `resolveItemLink` (AC6).
- A per-scope cap on pending (unanswered) requests with a configurable limit, rejecting creation past the cap with a clear error (AC7).
- Unique-index + status-guard duplicate handling for concurrent creation/double-answer (AC5).
- A global nav badge (`GuidanceRequestNavBadge`) showing the count of pending guidance requests across all scopes, discoverable from every page without navigation — justified by `research/ux.md`'s discoverability findings for a WIP-cap-constrained multi-item workflow (a stalled item burns a scarce concurrency slot, so a standing signal is needed rather than requiring the operator to hunt through 3 separate views; see design/ux.md §6).
- A periodic self-heal sweep reconciling missed `notified_at` writes and cancelling requests whose owning scope (backlog item or session) was archived, hard-deleted, or torn down while a request was still pending — justified by the ADR-001 durability precedent (`BacklogStuckState`'s "best-effort precondition + self-heal sweep, not cross-table atomicity" posture) this feature is required to follow, and required to satisfy AC5.

### Out of Scope
- Agent-to-agent or MCP-tool-submitted answers — answering is human-only via UI in v1 (per Answerer decision).
- Arbitrary custom-field/multi-step form schemas — only the three fixed types (yes/no, multiple-choice, short-answer).
- Replacing or changing the existing interactive `AskUserQuestion` live-chat-turn flow — this is a purely additive channel for the unattended/durable case.
- Batched/multi-question forms in a single request (one question per `GuidanceRequest`).
- Observability dashboards beyond standard structured logging (see Observability Requirements below) — no new Grafana panel in v1.
- Editing or retracting a pending question once created (a follow-up concern, not v1).

## Rabbit Holes
- **The shared UI component across 3 views is genuinely new** — the codebase survey found no existing "one component reused verbatim across 3 views" pattern (`ApprovalRulesPanel.tsx` lives only on its own page; triage rendering lives inside `BacklogItemDetail.tsx`). Budget real design time here; don't assume it's a trivial extraction.
- **Notification durability wiring**: `wait_for_backlog_event`/`BacklogItemEventPublisher` is the right target, but confirm during research whether it already covers "notify a specific session across a restart" or only "a live blocking RPC call" — if the latter, closing that gap is itself non-trivial and central to AC2.
- **Per-scope pending cap** (AC7): no existing per-item cap pattern exists (the WIP cap is board-wide, not per-item) — the counting query and race-safety of the cap check need explicit design, not just a copy-paste.
- **Triage halt-and-resume mechanics** (AC3): "halt and resume once an answer is available" implies triage's run loop needs a genuine pause/resume checkpoint, not just "don't act yet" — confirm how triage runs are currently modeled (single long-lived process vs. re-entrant per-tick) before assuming this is cheap.

## Alternatives Considered
- **Reuse `ApprovalRulesPanel`/approval-rule schema as the storage template**: rejected — it's global tool policy with no `item_id`/`session_id` scoping or ownership model, a poor structural fit (confirmed via codebase survey).
- **Extend `PendingApproval`** (`server/services/approval_store.go`): closer conceptually (a session-keyed pending ask) but it's in-memory only, not durable — would need the same durability work as building fresh, with less schema fit for the three question types.
- **Keep polling via `ScheduleWakeup`** for notification: rejected per Constraints — would add to the exact poll-loop pile ssq#428 already tracks as tech debt.
- **A leaner v1 deferring the nav badge (Phase 7) and/or the self-heal sweep (Phase 8)**: seriously considered, not just implicitly assumed necessary, and rejected on a per-cut basis. Deferring the nav badge would leave pending questions discoverable only by opening each item/session individually — `research/ux.md`'s finding on the 2-item WIP cap says directly that this undermines the feature's core value, since a stalled item silently burns a scarce concurrency slot with no standing signal that it's stuck on a question. Deferring the self-heal sweep would leave AC5's dedup/notify-once guarantee un-backstopped for the exact race conditions ADR-001 was written to fix — i.e. shipping the known-bad pattern (in-memory dedup state that resets on restart, no reconciliation for a scope torn down mid-flight) this feature exists to replace. Both cuts were weighed and rejected because they are durability/discoverability-load-bearing for the acceptance criteria already in scope, not additive polish on top of them.

## Feasibility Risks
- Triage's current control flow may not have a clean "halt here, resume there" seam — if triage is a single synchronous run rather than a resumable state machine, AC3 could require more triage-side refactoring than the guidance-request feature itself.
- The three-view shared component must handle three different surrounding layouts (item detail page, triage panel, session view) without forking — a naive first pass risks quietly becoming three components with one name.

## Observability Requirements
Standard structured logging (`slog`) on create/answer/notify/cap-rejected/dedup-collision events, tagged with scope (item/session id) and question id — sufficient for v1. No new metric or oncall alert; a `pending_guidance_requests` gauge is a reasonable follow-up if usage grows, not required now.

## Risk Control
Feature flag for AC3 only (triage halt-and-wait), per Constraints above — live-settable global (per ADR-003's accepted v1 exception, no per-scope override — see Constraints) via the existing rollout-flag RPC/panel pattern, no env var. The rest of the feature (schema, RPCs, UI, notification) is additive and has no flag — rollback is a straight revert if needed.

## Open Questions
- Does `BacklogItemEventPublisher`/`wait_for_backlog_event` already support "deliver to a session that reconnects after a restart," or does that gap need closing as part of this feature? (Flagged as a rabbit hole above — Research phase should confirm against the actual event-bus implementation.)
- What is triage's current execution model (long-lived loop vs. re-entrant per-tick)? Determines how expensive AC3's halt/resume actually is.
- Exact default value for the per-scope pending-request cap (AC7) — a reasonable default (e.g. 3-5) should come out of Research/Plan, not be guessed here.
