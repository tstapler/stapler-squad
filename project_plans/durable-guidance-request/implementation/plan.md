# Implementation Plan: durable-guidance-request

**Feature**: A durable `GuidanceRequest` entity + RPCs/MCP tools + event-bus notification + feature-flagged triage halt/resume + one shared React component (3 views) + nav badge, giving any stapler-squad-managed agent (backlog session, automated triage, background workflow) a durable way to ask the human a structured question and be resumed with the answer.
**Date**: 2026-09-12
**Status**: Ready for implementation
**ADRs**: [ADR-001](../decisions/ADR-001-standalone-guidance-request-entity.md) (standalone entity, not a `StuckReason`), [ADR-002](../decisions/ADR-002-guidance-request-event-payload.md) (new scope-uniform event payload, not piggybacked), [ADR-003](../decisions/ADR-003-global-only-triage-halt-flag.md) (global-only feature flag, no per-item override in v1)

---

## Step 0.5 — Creative Pass (recorded)

Three high-level approaches considered for the overall design:

- **(A) Single `GuidanceRequest` entity + one scope-agnostic event/delivery path for all 3 scopes uniformly.** Strength: one code path for creation, cap-check, notify, and read-back regardless of scope — least branching, easiest to test exhaustively. Weakness: "uniform" delivery still has to branch somewhere, because a `backlog-item`-scoped answer needs to trigger a `headless.Pool` triage respawn while a `session`-scoped answer needs a live `write_to_session` and a `standalone`-scoped answer needs neither — a truly scope-blind delivery layer would have to invent a generic "resume callback" abstraction with only one real implementation each, which is the exact kind of premature interface `interface-pollution-checklist` warns against.
- **(B) `GuidanceRequest` entity, but `backlog-item`-scoped requests piggyback on `BacklogItemChangedEvent`/`BacklogItemEventPayload` while `session`/`standalone` get a parallel bespoke event.** Strength: item-scope UI code that already subscribes to `BacklogItemChangedEvent` (e.g. `wait_for_backlog_event`) gets guidance-answered notifications "for free" without a second subscription. Weakness: forks into two event types for one conceptual "answered" fact, and `mapBacklogChangeKind`'s panic-on-unmapped-kind design (`server/services/backlog_item_event_publisher.go:83-85`) means the item-scope half is fragile in a way the bespoke half isn't — inconsistent risk profile for supposedly the same feature.
- **(C, chosen) `GuidanceRequest` entity with ONE new scope-uniform event type for delivery notification (ADR-002), but scope-specific logic kept OUT of the event layer and pushed down into the delivery consumer** (a single `GuidanceRequestDeliveryService` that branches on `Scope` only when actually resuming the asker — `write_to_session` for `session` scope, `headless.Pool` respawn for `backlog-item` scope's triage case, no-op resume for `standalone`). Strength: one event type, one subscription point, branching lives exactly where the three scopes are genuinely different (resuming the asker) rather than being smeared across the event layer (as in B) or forced into a fake-uniform interface with one implementation per branch (as in A). Weakness: the delivery consumer itself is a single function with a 3-way switch — acceptable per Go house style (a `switch` over 3 known, closed values is not the interface-pollution anti-pattern; an interface would be, since there's exactly one implementation per scope and no plausible fourth).

**Chosen: (C).** Recorded in ADR-002; the entity-boundary and flag-scope choices are recorded in ADR-001 and ADR-003 respectively.

---

## Step 1 — System Type

This is **a durable async request/response primitive layered onto an existing CRUD + event-bus system** — not a green-field service. Every load-bearing pattern (atomic upsert, event publish, MCP ownership check, feature flag, React component reuse) already has a working precedent in this codebase; the work is composing them correctly for a new domain concept, not inventing new infrastructure. (Confirmed: `build-vs-buy.md` — zero new dependencies of any kind.)

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `GuidanceRequest` | The durable ent entity representing one question asked by an agent, awaiting or holding a human answer. | New entity, `session/ent/schema/guidance_request.go`. See ADR-001. |
| `RequestScope` | Enum-by-convention (`field.String` + Go `IsValid()`) identifying what the question is attached to: `backlog-item`, `session`, or `standalone`. | Determines ownership check and delivery branch (see `GuidanceRequestDeliveryService`). |
| `QuestionType` | Enum-by-convention identifying the answer shape: `yes-no`, `multiple-choice`, `short-answer`. | Drives which control the React component renders. |
| `RequestStatus` | Derived, not a stored enum field: `pending` (`answered_at IS NULL AND cancelled_at IS NULL`), `answered` (`answered_at` set), `cancelled` (`cancelled_at` set, terminal — see AC5/pitfalls.md). | Computed from timestamps, mirroring `BacklogStuckState`'s `resolved_at IS NULL` convention rather than a separate status column that could drift from the timestamps. |
| `Options` | JSON-encoded array of choice strings for a `multiple-choice` `QuestionType`; empty/absent for the other two types. | Stored as `field.String`, matches `ItemSession.ac_snapshot`'s JSON-in-a-string convention. |
| `Answer` | The stored response value (raw string — `"yes"`/`"no"`, the chosen option text, or free text). | Set exactly once, by the `answered_at`-guarded conditional UPDATE. |
| `NotifiedAt` / `AnsweredAt` / `CancelledAt` | Independent, nullable timestamps — each set by its own idempotent no-op-safe setter. | Mirrors `BacklogStuckState.notified_at`/`resolved_at`; "answered" and "notified" are separately-failing/retriable facts (pitfalls.md). |
| `PendingCap` | The configurable per-scope limit on concurrently-open (`answered_at IS NULL AND cancelled_at IS NULL`) `GuidanceRequest` rows. | Default **4** (see Pattern Decisions / Open Questions resolution below). Configurable via `config.GuidanceRequestPendingCap` (falls back to the default when unset/zero). |
| `GuidanceRequestEvent` / `GuidanceRequestEventPayload` | The new, scope-uniform `pkg/events` type published when a `GuidanceRequest` is answered. | See ADR-002. |
| `GuidanceRequestDeliveryService` | New service that reacts to `GuidanceRequestEventPayload` and resumes the asker: `write_to_session` for `session` scope, `headless.Pool` triage respawn for `backlog-item` scope, no-op for `standalone`. | `server/services/guidance_request_delivery_service.go`. |
| `TriageGuidanceHaltFlag` | The `config.FeatureFlags` key (`"triage_guidance_halt"`) gating AC3's halt-on-ambiguity behavior. Default OFF. | See ADR-003. |
| `EffectiveTriageGuidanceHaltEnabled` | The accessor function triage consults at the exact halt-decision instant. | `config.EffectiveTriageGuidanceHaltEnabled(cfg *Config) bool`, mirrors `EffectiveStreamHubEnabled`. |
| `resolveItemLink` | The existing MCP ownership check (`server/mcp/tools_backlog.go:585`), still the call site used by the MCP tools for `backlog-item`-scoped `GuidanceRequest` creation/read. | Refactored (not reimplemented) into a thin `mcpgo.CallToolResult`-shaped adapter over the new shared `session.ResolveItemLink` helper — see the next row. Its own external behavior/signature is unchanged. |
| `session.ResolveItemLink` | New package-neutral helper (`session/item_link.go` or similar), extracted from `resolveItemLink`'s body: does the storage lookup + not-found-vs-permission-denied disambiguation + remediation-text construction, with zero MCP or ConnectRPC dependency. Returns a small result type (OK / NotFound / PermissionDenied, each carrying remediation text). | `resolveItemLink` (MCP) and `GuidanceRequestService`'s `backlog-item`-scope ownership check (ConnectRPC) both wrap this same helper in their own error type — see Pattern Decisions "Ownership check" row and Task 2.1.2a-pre. |
| Self-heal sweep | The periodic reconciliation pass that (a) retries a missed `notified_at` write and (b) sets `cancelled_at` on a `GuidanceRequest` whose owning scope (item/session) was archived/deleted while pending. | New method on the existing `BacklogLifecycleListener` tick, per ADR-001/pitfalls.md's "best-effort precondition + self-heal sweep, not cross-table atomicity" posture. |
| `GuidanceRequestCard` | The one shared React component rendering a pending/answered/cancelled `GuidanceRequest`, used identically from backlog item detail, the triage panel, and the session view (AC4). | `web-app/src/components/backlog/GuidanceRequestCard.tsx`. |
| `GuidanceRequestNavBadge` | The global nav badge showing the count of pending `GuidanceRequest`s across all scopes. | `web-app/src/components/sessions/GuidanceRequestNavBadge.tsx`, modeled on `ApprovalNavBadge.tsx`. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Entity boundary | Standalone ent entity | `BacklogStuckState`'s pattern, not its table | Fold into `StuckReason` | Scope mismatch (2 of 3 scopes have no `BacklogItem`) + payload mismatch (typed answer/options fields). See ADR-001. |
| Persistence write path | Atomic `OnConflictColumns` upsert (create) + conditional `UPDATE ... WHERE answered_at IS NULL` (answer) + separate conditional `UPDATE ... WHERE notified_at IS NULL` (notify) | `MarkStuck`/`ResolveStuck`/`MarkStuckNotified` (`session/ent_repository_backlog.go:1901-2014`) | Check-then-insert / SELECT-then-UPDATE | SQLite unique-index race + double-answer race; a separate transaction between check and act is a TOCTOU window (pitfalls.md). |
| Event delivery payload | One new scope-uniform `events.GuidanceRequestEventPayload` | New, mirrors `NewBacklogItemChangedEvent`'s constructor shape (`pkg/events/types.go`) | Piggyback on `BacklogItemChangedEvent` for item-scope + bespoke type for session/standalone | Forks into two consumer code paths for one concept; `mapBacklogChangeKind` panics on unmapped kinds. See ADR-002. |
| Delivery/resume branching | One `GuidanceRequestDeliveryService` with a closed 3-way `switch` on `RequestScope` | Hand-rolled, mirrors the never-implemented `RespondToHelpRequest` design (`project_plans/backlog-agent-communication/implementation/plan.md:429-541`) | A `Resumer` interface with one implementation per scope | Exactly 3 known, closed cases with no plausible 4th — an interface here is premature abstraction per `interface-pollution-checklist`; a switch is the idiomatic Go shape (`golang-development` skill). |
| Feature flag | Global-only `config.FeatureFlags["triage_guidance_halt"]` + plain `GetFeatureFlag`/`SetFeatureFlag` RPC pair | `TymuxFeatureFlag`/`EffectiveTymuxEnabled` (`config/config.go:465-505`) | Per-item `*RolloutService` (`stream_hub`/`tymux` per-session-override style) | No identified v1 consumer needs per-item divergence; YAGNI; strictly-additive follow-up path preserved. See ADR-003. |
| Ownership check | Extract `resolveItemLink`'s pure ownership-resolution logic (storage lookup + not-found-vs-permission-denied disambiguation + remediation text) into a package-neutral helper, `session.ResolveItemLink`, reused by BOTH the MCP tool (`server/mcp`) and the new RPC handler (`server/services`) via thin protocol-specific adapters, for `backlog-item` scope; new, separate, deliberately-simpler checks for `session` (caller UUID == originating session UUID) and `standalone` (no check — human-only surface) | `server/mcp/tools_backlog.go:585-617` (extraction source) | (1) Reimplementing the disambiguation logic separately in `server/services/guidance_request_service.go` — rejected, see below; (2) One generic `checkOwnership(scope, ...)` abstraction spanning all 3 scopes | requirements.md explicitly calls out that session/standalone "need their own simpler check" — a shared abstraction over 3 structurally different authorization rules (item-link table lookup vs. UUID equality vs. no-op) would be a leaky interface for no reuse benefit (`interface-pollution-checklist`). For the `backlog-item` case specifically, `resolveItemLink` contains non-trivial logic beyond a raw storage call (a second `GetBacklogItem` call to disambiguate `ErrNotFound`-on-link from item-truly-missing, plus context-specific remediation text including the "linked to a different item" branch) — reimplementing that in a second package instead of extracting it would create two independently-maintained copies of the same subtle logic that will drift, the exact "uniform-looking API, forked implementation" anti-pattern ADR-002 already rejected for the event-delivery path. Extracting into a shared, protocol-neutral helper avoids that without introducing a real cross-scope abstraction — this is a single-scope (`backlog-item` only) code-reuse extraction, not a `checkOwnership`-style generalization. |
| Cap enforcement | Hand-written transactional `COUNT(*) WHERE scope=? AND [scope_key]=? AND answered_at IS NULL AND cancelled_at IS NULL`, executed inside the SAME transaction as the create-upsert | New, no existing precedent confirmed race-safe (pitfalls.md flags `effectiveReworkCap`/`ReworkCapOverride` as unconfirmed) | Reuse `effectiveReworkCap`/`ReworkCapOverride`; or an in-memory dedup/rate-limiter | Constraint explicitly forbids an in-memory dedup map; the existing rework-cap code's race-safety is unconfirmed per pitfalls.md, so inheriting it would inherit unverified risk instead of writing an independently-correct check. |
| React component | Hand-rolled component + local component state, ARIA/busy/error conventions copied verbatim from `TriageDiffSection.tsx`'s existing free-text answer flow | `web-app/src/components/backlog/{GateVerdictBox,PlanVerdictBox,TriageDiffSection}.tsx` | `react-hook-form`/`react-jsonschema-form`/Formik | No form library is a current dependency; exactly 3 fixed question shapes need no schema-driven engine (build-vs-buy.md). |
| Orphan-detector integration | New early-exit branch in `reconcileOrphanedTriageItems`, not a refactor of it | `session/backlog_lifecycle_triage.go:172-330` | Leave `reconcileOrphanedTriageItems` unchanged and give `GuidanceRequest` its own independent orphan sweep | Would let the existing detector misclassify a legitimately-halted triage session as an anomaly and retry-with-backoff-penalize it — defeats AC3 entirely (architecture.md's explicit warning). |
| Correctness model | Best-effort precondition check at write time + periodic self-heal sweep as the actual correctness backstop | `BacklogStuckState`'s documented posture (ADR-001 in the `backlog-stuck-item-visibility` project) | Cross-table atomic transaction spanning ownership-check + cap-check + event-bus publish | No existing precedent for that scope of transaction in this codebase; over-engineered for single-digit-QPS, human-latency-bound traffic (requirements' Non-functional Requirements). |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| `reconcileOrphanedTriageItems` (`session/backlog_lifecycle_triage.go:172-330`) | Treats a triage session that ended without a plan, or is open-and-stale, as anomalous and applies backoff-penalized retry (`retryOrphanedTriageWithBackoffGate`) — no existing branch recognizes "halted behind an open `GuidanceRequest`" as expected. | **Extend as-is: add one new early-exit branch, not a refactor.** | A halted-on-guidance-request triage session is a normal, expected state once AC3 ships behind its flag — retrying it with backoff would silently defeat the whole point of the halt (architecture.md, explicitly called a REQUIRED task). The rest of the function's existing shape-1/shape-2/shape-3 logic is untouched. |
| `ScheduleWakeup`-based polling pile (ssq#428) | Sessions poll for async backlog changes instead of subscribing durably; tracked tech debt, not this feature's job to fix. | **Leave as-is; do not add to it.** | Constraint explicitly forbids adding another poll loop. This feature rides `EventBus` + `headless.Pool` respawn instead — net negative addition to the poll-loop pile (zero new polling). |
| `effectiveReworkCap`/`ReworkCapOverride` (WIP-cap precedent) | Race-safety not confirmed for this codebase's SQLite/ent transaction model (pitfalls.md). | **Do not reuse; write an independently-correct transactional `COUNT`+insert for `GuidanceRequest`'s cap instead.** | Confirming the existing cap's race-safety is unscoped extra work; writing a fresh, explicitly-tested transactional check is cheaper and doesn't inherit unverified risk. Not a fix for the existing code — out of scope. |

---

## Migration Plan

- **Migration file**: none beyond the new ent schema file (`session/ent/schema/guidance_request.go`). This codebase has no hand-written up/down SQL migrations for ent entities — `client.Schema.Create(ctx)` (`session/ent_repository.go:184`) auto-migrates DDL for every registered schema on startup, guarded by `EntSchemaCreateMu` (`session/ent_repository.go:32`). Adding the new schema file is the entire "migration."
- **Reversibility**: trivial — a brand-new table with no backfill and no other entity's FK pointing *into* it (one outbound optional, non-cascading FK to `BacklogItem` via `item_id` — see Task 1.1.1b for why this edge deliberately has no `OnDelete(Cascade)` — plus a loose, non-edge string FK to `ItemSession.session_uuid` for `session` scope). Reverting the PR drops the schema registration; the orphaned SQLite table is inert (never referenced by other code) and can be left or cleaned up by a follow-up `DROP TABLE`, at the team's discretion.
- **Zero-downtime strategy**: not applicable — `Schema.Create` is additive-only (`CREATE TABLE IF NOT EXISTS` + `CREATE INDEX IF NOT EXISTS` semantics), runs automatically on next service start, and adds zero load to the existing running process until this feature's own code paths are deployed and invoked.
- **Rollback procedure**: revert the PR. No flag-gated data to reconcile (the only flag, `triage_guidance_halt`, defaults OFF and gates a *behavior*, not stored data whose absence would break something else).

## Observability Plan

- **Logs**: standard structured `slog`/`log.InfoLog()`/`log.WarningLog()` calls on: create (`scope`, `question_id`, `question_type`), answer (`question_id`, `scope`), notify (`question_id`, notify success/failure), cap-rejected (`scope`, `scope_key`, `current_count`, `cap`), dedup-collision (`scope`, `scope_key` — when the unique-index upsert hits an existing open row instead of creating a new one). Matches requirements.md's Observability Requirements verbatim — **no new metric, no new Grafana panel, no new oncall alert required for v1.**
- **Metrics**: none (requirements.md explicit: "not specified — low-QPS, human-latency-bound").
- **Alerts**: none (requirements.md explicit).

## Risk Control

- **Feature flag**: `triage_guidance_halt` (`config.FeatureFlags` key, `config.TriageGuidanceHaltFeatureFlag` constant), default **OFF** — mirrors `tymux`, not `stream_hub` (no rollback rehearsal precondition, since this flag gates net-new behavior rather than replacing an existing one). Global-only per ADR-003.
- **Rollback procedure**: flip `triage_guidance_halt` back to OFF live via the existing feature-flag RPC/settings panel (no restart) to stop new halts immediately; already-halted items remain halted until answered (flipping the flag off does NOT retroactively unstick them — this is a tested, explicit invariant, see Story 5.1.3 below). For anything beyond the flag (schema, RPCs, UI, nav badge — all unflagged/additive), rollback is a straight PR revert, per requirements.md's Risk Control section.
- **Staged rollout**: none required beyond the flag itself — single-operator deployment, no canary/percentage rollout mechanism exists or is needed for this scale (requirements.md Non-functional Requirements: "single-operator deployment").

## Unresolved Questions

- [ ] Exact wording/UX of the "N/cap pending" proactive display asked for by `ux.md` (badge copy, whether it shows on every card or only near the cap) — blocks Story 6.1.2 (`GuidanceRequestCard` cap display) — owner: whoever implements Epic 6, pick a reasonable default (e.g. only render "N/cap" when `N >= cap - 1`) and adjust from user feedback; not a blocker to start implementation.

Everything else research resolved explicitly (delivery mechanism, triage halt/resume mechanics, entity boundary, event payload shape, flag scope, cap default, and — as of this repair pass — the Story 3.2.3 notification-type choice, resolved to `NOTIFICATION_TYPE_STATUS_CHANGE` with no proto change; see Story 3.2.3) — see ADRs and Pattern Decisions above.

## Dependency Visualization

```
Phase 1: Schema + Storage           Phase 2: Proto + RPC + MCP
┌─────────────────────────┐         ┌──────────────────────────┐
│ GuidanceRequest ent      │ ──────▶ │ guidance_request.proto    │
│ schema + repository      │         │ GuidanceRequestService    │
│ (upsert/answer/notify/   │         │ MCP create/read tools     │
│  cap-check)              │         │ (resolveItemLink reuse)   │
└─────────────────────────┘         └──────────┬────────────────┘
                                                 │
                    ┌────────────────────────────┼─────────────────────────┐
                    ▼                            ▼                         ▼
      Phase 3: Event + Notify        Phase 4: Feature Flag      Phase 6: Frontend Component
      ┌───────────────────────┐      ┌──────────────────────┐  ┌───────────────────────────┐
      │ GuidanceRequestEvent   │      │ triage_guidance_halt  │  │ GuidanceRequestCard.tsx    │
      │ DeliveryService        │      │ flag + accessor + RPC │  │ (+ .css.ts, .test.tsx)     │
      │ (write_to_session /    │      └──────────┬────────────┘  │ wired into 3 views          │
      │  headless.Pool resume) │                 │               └──────────┬─────────────────┘
      └──────────┬─────────────┘                 │                          │
                 │                                ▼                          ▼
                 │                  Phase 5: Triage Integration    Phase 7: Nav Badge
                 │                  ┌──────────────────────────┐  ┌───────────────────────────┐
                 └─────────────────▶│ halt-on-ambiguity         │  │ GuidanceRequestNavBadge    │
                                    │ + respawn via              │  │ wired into Header/BottomNav │
                                    │ headless.Pool + REQUIRED   │  └───────────────────────────┘
                                    │ reconcileOrphanedTriage    │
                                    │ Items new branch           │
                                    └──────────────────────────┘
                                                 │
                                                 ▼
                                    Phase 8: Self-heal sweep
                                    ┌──────────────────────────┐
                                    │ notify-once retry +       │
                                    │ cancel-on-archive sweep    │
                                    │ (BacklogLifecycleListener) │
                                    └──────────────────────────┘
```

---

## Phase 1: Schema + Storage Layer

### Epic 1.1: `GuidanceRequest` ent entity + atomic repository methods
**Goal**: A durable, race-safe `GuidanceRequest` row with create/answer/notify/cancel/cap-check operations, following `BacklogStuckState`'s exact atomic-upsert shape.

#### Story 1.1.1: Define the `GuidanceRequest` ent schema
**As a** backend developer, **I want** a `GuidanceRequest` ent schema with all required fields, edges, and a unique index, **so that** the entity can be created, migrated, and queried durably.
**Acceptance Criteria** (AC1, AC5):
- A `GuidanceRequest` row for `scope=backlog-item`, `item_id=b608ab1e-...`, `question_type=yes-no`, created at `t0`, survives a full service restart and is readable at `t0+1h` with all fields intact.
  - *Given* a `GuidanceRequest` row was inserted with `scope="backlog-item"`, `item_id="b608ab1e-b86e-4130-8879-7328cd363063"`, `question_type="yes-no"`, `question_text="Should triage merge PR #780 into this item's branch?"`, *When* the service restarts and `GetGuidanceRequest(ctx, id)` is called, *Then* it returns the same row with `answered_at=nil`, `status` derived as `pending`.
- Two concurrent create calls for the same `(scope, scope_key)` dedup key resolve to exactly one open row (AC5).
  - *Given* two goroutines each call `CreateGuidanceRequest` for `scope="session"`, `session_uuid="7c1e...")`, `question_text="Use approach A or B?"` at nearly the same instant, *When* both transactions commit, *Then* exactly one `GuidanceRequest` row exists for that session with that question (verified by a unique-index-backed `OnConflictColumns` upsert test asserting row count == 1).
**Files**: `session/ent/schema/guidance_request.go`

##### Task 1.1.1a: Write the `GuidanceRequest` ent schema fields (~5 min)
- Create `session/ent/schema/guidance_request.go`. Fields: `id` (`field.UUID`, `Default(uuid.New)`), `scope` (`field.String`, comment: validated by Go-side `domain.RequestScope.IsValid()`, values `backlog-item`/`session`/`standalone`), `item_id` (`field.UUID`, `Optional().Nillable()` — set only for `backlog-item` scope), `session_uuid` (`field.String`, `Optional()` — set for `session` scope, loose string FK per `ItemSession.session_uuid`'s cross-package-cycle-avoidance convention, NOT an ent edge), `question_text` (`field.String`), `question_type` (`field.String`, comment: validated by `domain.QuestionType.IsValid()`, values `yes-no`/`multiple-choice`/`short-answer`), `options` (`field.String`, `Optional()`, comment: JSON-encoded `[]string`, only for `multiple-choice`), `answer` (`field.String`, `Optional()`), `created_at` (`field.Time`, `Default(time.Now)`), `notified_at` (`field.Time`, `Optional().Nillable()`), `answered_at` (`field.Time`, `Optional().Nillable()`), `cancelled_at` (`field.Time`, `Optional().Nillable()`).
- Files: `session/ent/schema/guidance_request.go`

##### Task 1.1.1b: Add the `item_id` edge (backlog-item scope only) and the unique dedup index (~5 min)
- Add `Edges()`: `edge.From("item", BacklogItem.Type).Ref("guidance_requests").Field("item_id").Unique()` (NOT `.Required()` — unlike `BacklogStuckState`, this edge is optional since 2 of 3 scopes have no item). Add the reverse `edge.To("guidance_requests", GuidanceRequest.Type)` to `session/ent/schema/backlog_item.go`'s `Edges()`.
- **Do NOT add `.Annotations(entsql.OnDelete(entsql.Cascade))` on this edge, despite that matching `BacklogItem`'s existing cascade convention for its other child tables.** `BacklogItem` supports genuine hard deletion (`DeleteBacklogItem`, `session/ent_repository_backlog.go:1306,1353`), not just archival. A cascade here would let a hard delete silently destroy a pending OR answered-but-undelivered `GuidanceRequest` row with no trace — directly contradicting AC1/Story 3.2.2's "no answer is ever silently lost" bar — and Phase 8's self-heal sweep (Task 8.1.1a) cannot catch it, because a cascade-deleted row is already gone by the time the sweep runs; there is nothing left to reconcile. Instead, leave the `item_id` FK to go stale on a hard delete (consistent with the `session`/`standalone` scopes, which already have no cascade or edge at all back to their owning entity). The self-heal sweep (Task 8.1.1a) is extended to treat "item not found" (not just "item archived") as cancel-worthy for the `backlog-item` scope, using the same `CancelGuidanceRequest` code path — so a hard-deleted item's still-open `GuidanceRequest` rows get a durable `cancelled_at` record on the next sweep tick instead of vanishing untraceably.
- Add `Indexes()`: a single unique index covering the dedup key. Because SQLite treats NULL as distinct (pitfalls.md), the dedup key CANNOT rely on the nullable `item_id`/`session_uuid` alone being enough on their own — use a composite: `index.Fields("scope", "item_id", "session_uuid", "question_text").Unique()`. Document in a comment why `question_text` is part of the key (an item can have more than one *different* open question; the dedup guarantee is "don't create two identical open asks," not "at most one open ask per item" — this differs from `BacklogStuckState`'s `(item_id, reason)` key precisely because `GuidanceRequest` has no closed `reason` enum to key on).
- Files: `session/ent/schema/guidance_request.go`, `session/ent/schema/backlog_item.go`

##### Task 1.1.1c: Regenerate ent and confirm build (~3 min)
- Run `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` (the exact flag required — `--feature sql/upsert` — per `session/ent/generate.go` and this repo's CLAUDE.md warning). Do NOT commit `session/ent/*.go`/`session/ent/*/` generated output (gitignored). Run `go build ./...` to confirm it compiles.
- Files: none committed (generated output only); verify via `make ent-gen` wiring already in `Makefile:552-556`.

#### Story 1.1.2: Domain validation types (`RequestScope`, `QuestionType`)
**As a** backend developer, **I want** Go-side validated types for scope and question type, **so that** invalid values are rejected before hitting the database, matching the `StuckReason`/`BacklogStatus` house style.
**Acceptance Criteria** (AC1):
- `domain.RequestScope("carrier-pigeon").IsValid()` returns `false`; `domain.RequestScope("backlog-item").IsValid()` returns `true`.
  - *Given* a caller passes `scope="carrier-pigeon"` to `CreateGuidanceRequest`, *When* the repository validates it, *Then* it returns an error without touching the database.
**Files**: `session/domain/guidance_request.go` (new — check existing `domain.StuckReason` location first)

##### Task 1.1.2a: Locate `domain.StuckReason` and add sibling types (~5 min)
- Grep for `type StuckReason` to find its file (likely `session/domain/stuck_reason.go` or similar). Add `type RequestScope string` with consts `RequestScopeBacklogItem = "backlog-item"`, `RequestScopeSession = "session"`, `RequestScopeStandalone = "standalone"`, and `IsValid() bool`. Add `type QuestionType string` with consts `QuestionTypeYesNo = "yes-no"`, `QuestionTypeMultipleChoice = "multiple-choice"`, `QuestionTypeShortAnswer = "short-answer"`, and `IsValid() bool`.
- Files: same file as (or sibling to) `domain.StuckReason`'s definition.

#### Story 1.1.3: Repository methods — create (atomic upsert), answer, notify, cancel, cap-check
**As a** backend developer, **I want** race-safe repository methods mirroring `MarkStuck`/`ResolveStuck`/`MarkStuckNotified`, **so that** concurrent creation and double-answer races are structurally impossible to corrupt.
**Acceptance Criteria** (AC5, AC7):
- Creating a 5th pending `GuidanceRequest` for a scope already at its cap of 4 is rejected with a clear, visible error — not silently dropped (AC7).
  - *Given* backlog item `b608ab1e-...` already has 4 open (unanswered, uncancelled) `GuidanceRequest` rows, *When* a 5th `CreateGuidanceRequest` call is made for the same item, *Then* it returns `(nil, ErrPendingCapExceeded)` with a message including the current count (4) and the cap (4), and this is logged at `WarningLog` level with `scope`/`scope_key`/`current_count`/`cap` fields (per Observability Plan).
- Two near-simultaneous `AnswerGuidanceRequest` calls for the same already-open row: the second is a no-op, not an overwrite (AC5).
  - *Given* `GuidanceRequest` id `q1` is open, *When* two goroutines both call `AnswerGuidanceRequest(ctx, "q1", "yes")` and `AnswerGuidanceRequest(ctx, "q1", "no")` concurrently, *Then* exactly one of the two answers is persisted (whichever's `UPDATE ... WHERE answered_at IS NULL` commits first) and the loser's call returns `(applied=false, err=nil)`.
**Files**: `session/ent_repository_guidance.go` (new sibling to `session/ent_repository_backlog.go`)

##### Task 1.1.3a: `CreateGuidanceRequest` with dedup-first upsert, THEN in-transaction cap-check (~7 min)
- **Ordering matters and must be dedup-first, not cap-first** (see adversarial-review.md Blocker 2): a legitimate re-ask of an IDENTICAL already-open question must resolve to the pre-existing row via dedup even when the scope is already at its cap — cap-rejecting it instead would be wrong, since it creates no new row and the scope's open-question count doesn't actually increase.
- New file `session/ent_repository_guidance.go`. `func (r *EntRepository) CreateGuidanceRequest(ctx context.Context, req CreateGuidanceRequestInput) (*GuidanceRequestData, error)`, in this order, all inside one `tx := r.client.Tx(ctx)`:
  1. Validate `req.Scope`/`req.QuestionType` via `IsValid()` as the first statement, before opening the transaction (fails fast, no DB round trip for a malformed request).
  2. Generate a client-side `id := uuid.New()` before touching the DB, and pass it into the upsert via `tx.GuidanceRequest.Create().SetID(id)...OnConflictColumns("scope","item_id","session_uuid","question_text").Ignore().Exec(ctx)`. This makes new-row-vs-dedup-hit detection deterministic without depending on any ent-driver-specific affected-rows API: re-fetch the row by the dedup key afterward and compare its `id` against the generated `id`. If they match, `Create()`'s insert branch fired (this call made a genuinely new row); if the fetched row's `id` differs, the conflict-update branch fired instead and the returned row is a PRE-EXISTING row created by an earlier caller. This is a real, deterministic technique (generate the ID client-side, compare after) that needs no unverified assumption about how this ent version's `OnConflictColumns`/`Ignore()` reports insert-vs-conflict — it doesn't require spiking an ent API detail mid-implementation.
  3. **Only if step 2 created a genuinely NEW row**, run `tx.GuidanceRequest.Query().Where(scope/scope-key match, answered_at IS NULL, cancelled_at IS NULL).Count(ctx)` inside the same transaction, compare against `config.LoadConfig().GetGuidanceRequestPendingCap()` (task 4.1.1a below) using `count > cap` (the newly-created row is already included in this count, so the boundary check is `count > cap`, not `count >= cap`, to correctly allow the row that brings the count exactly to the cap) — if over cap, roll back the entire transaction (undoing the just-created row) and return `ErrPendingCapExceeded`.
  4. If step 2 resolved to a pre-existing row (dedup hit), skip the cap-check entirely and return the existing row — this is what makes the dedup case in the paragraph above work correctly even at/over cap.
  5. `tx.Commit()`.
  - Mirror `MarkStuck`'s exact `begin tx / defer rollback-on-error / commit` shape (`session/ent_repository_backlog.go:1922-1970`) for the transaction lifecycle itself.
- Files: `session/ent_repository_guidance.go`

##### Task 1.1.3b: `AnswerGuidanceRequest` conditional update (~3 min)
- `func (r *EntRepository) AnswerGuidanceRequest(ctx context.Context, id uuid.UUID, answer string) (applied bool, err error)`: `r.client.GuidanceRequest.Update().Where(guidancerequest.ID(id), guidancerequest.AnsweredAtIsNil(), guidancerequest.CancelledAtIsNil()).SetAnswer(answer).SetAnsweredAt(time.Now()).Save(ctx)`, return `n > 0`. Mirrors `ResolveStuck` (`session/ent_repository_backlog.go:1972-1990`) exactly.
- Files: `session/ent_repository_guidance.go`

##### Task 1.1.3c: `MarkGuidanceRequestNotified` and `CancelGuidanceRequest` (~4 min)
- `MarkGuidanceRequestNotified(ctx, id)`: conditional `UPDATE ... WHERE notified_at IS NULL SET notified_at=now`, mirrors `MarkStuckNotified` exactly. `CancelGuidanceRequest(ctx, id, reason string)`: conditional `UPDATE ... WHERE answered_at IS NULL AND cancelled_at IS NULL SET cancelled_at=now`, same shape. Both return `(applied bool, err error)`.
- Files: `session/ent_repository_guidance.go`

##### Task 1.1.3d: Query methods — get, list-pending-by-scope, list-all-pending (~5 min)
- `GetGuidanceRequest(ctx, id) (*GuidanceRequestData, error)`; `ListPendingGuidanceRequests(ctx, scope, scopeKey string) ([]*GuidanceRequestData, int /* cap */, error)` (returns current-pending-count alongside the cap per `ux.md`'s "scope API responses should include pending-count + cap" ask); `ListAllPendingGuidanceRequests(ctx) ([]*GuidanceRequestData, error)` (backs the nav badge, Epic 7).
- Files: `session/ent_repository_guidance.go`

#### Story 1.1.4: Repository-level unit tests for race safety
**As a** backend developer, **I want** tests proving the cap-check and double-answer paths are race-safe, **so that** AC5/AC7 are enforced by CI, not just code review.
**Acceptance Criteria** (AC5, AC7):
- A table-driven test with `N` concurrent `CreateGuidanceRequest` calls, each using a DISTINCT `question_text`, against a cap of 4 results in exactly 4 successes and `N-4` `ErrPendingCapExceeded` errors.
  - *Given* 10 goroutines call `CreateGuidanceRequest` concurrently for the same scope/key, each with its OWN distinct `question_text` (e.g. `"Question 0"`.."Question 9"`) so no dedup collision occurs between them, with a cap of 4, *When* all complete, *Then* `len(successes) == 4` and `len(errors) == 6`, all errors being `ErrPendingCapExceeded`. Using identical `question_text` across the goroutines would collapse them all into a single dedup'd row (Task 1.1.3a's `OnConflictColumns` upsert) and never exercise the cap path at all — this test is specifically about the cap boundary, not dedup, so the inputs must not collide.
- A SEPARATE, dedicated test proves the dedup path itself: N concurrent `CreateGuidanceRequest` calls with an IDENTICAL `question_text` (same scope/key) resolve to exactly 1 row, not a cap rejection, even when N exceeds the cap.
  - *Given* 10 goroutines call `CreateGuidanceRequest` concurrently for the same scope/key with the SAME `question_text` and a cap of 4, *When* all complete, *Then* exactly 1 `GuidanceRequest` row exists, all 10 calls return success (the same row's data), and NONE return `ErrPendingCapExceeded` — proving Task 1.1.3a's dedup-before-cap ordering (adversarial-review.md Blocker 2).
**Files**: `session/ent_repository_guidance_test.go`

##### Task 1.1.4a: Write the concurrent cap-check test with distinct question_text per goroutine (~5 min)
- New file `session/ent_repository_guidance_test.go`, `TestCreateGuidanceRequest_ConcurrentCapEnforcement` using `t.Parallel()` goroutines + a `sync.WaitGroup`, in-memory/temp-file SQLite per existing `EntRepository` test setup convention (grep an existing `_test.go` in this package for the exact test-DB bootstrap helper to reuse). Each goroutine MUST pass a distinct `question_text` (e.g. indexed by goroutine number) — see Story 1.1.4's AC for why an identical value across goroutines would make this test meaningless.
- Files: `session/ent_repository_guidance_test.go`

##### Task 1.1.4b: Write the concurrent double-answer test (~4 min)
- `TestAnswerGuidanceRequest_ConcurrentDoubleAnswer`: 2 goroutines racing to answer the same row with different values; assert exactly one `applied=true`.
- Files: `session/ent_repository_guidance_test.go`

##### Task 1.1.4c: Write the concurrent dedup-collision test (~5 min)
- `TestCreateGuidanceRequest_ConcurrentDedupCollision`: 10 goroutines call `CreateGuidanceRequest` concurrently for the same scope/key with an IDENTICAL `question_text` and a cap of 4. Assert: exactly 1 `GuidanceRequest` row exists in the DB afterward; all 10 calls return `(row, nil)` with the same row `id`; zero calls return `ErrPendingCapExceeded`. This is the test that proves Task 1.1.3a's dedup-before-cap ordering fix — without it, this scenario would previously have either wrongly cap-rejected some callers or (per adversarial-review.md Blocker 2) never been tested at all.
- Files: `session/ent_repository_guidance_test.go`

---

## Phase 2: Proto + RPC + MCP Tools

### Epic 2.1: Proto messages and RPCs
**Goal**: `GuidanceRequest` is creatable, readable, and answerable over ConnectRPC, following the `<Verb><Noun>Request/Response` house convention.

#### Story 2.1.1: Define proto messages and service RPCs
**As a** frontend/MCP developer, **I want** typed proto messages for `GuidanceRequest`, **so that** the web UI and MCP tools share one wire contract.
**Acceptance Criteria** (AC1):
- A `CreateGuidanceRequestRequest{scope: "backlog-item", item_id: "b608ab1e-...", question_type: "yes-no", question_text: "..."}` round-trips through `make proto-gen` into a compiling Go type.
  - *Given* the new `.proto` file defines `CreateGuidanceRequestRequest`/`CreateGuidanceRequestResponse`/`AnswerGuidanceRequestRequest`/`GetGuidanceRequestResponse`/`ListGuidanceRequestsResponse` (with `pending_count`/`cap` fields per ux.md), *When* `make proto-gen` runs, *Then* `gen/proto/go/session/v1/guidance_request.pb.go` compiles with no manual edits.
**Files**: `proto/session/v1/guidance_request.proto` (new file — cleaner than extending `backlog.proto`, per stack.md's recommendation for a new bounded concept)

##### Task 2.1.1a: Write the proto message definitions (~5 min)
- New file `proto/session/v1/guidance_request.proto`. Messages: `GuidanceRequest` (id, scope, item_id, session_uuid, question_text, question_type, options (repeated string), answer, created_at, notified_at, answered_at, cancelled_at, status — computed enum for wire convenience), `CreateGuidanceRequestRequest`/`Response`, `AnswerGuidanceRequestRequest`/`Response`, `GetGuidanceRequestRequest`/`Response`, `ListGuidanceRequestsRequest{scope, scope_key}`/`Response{requests, pending_count, cap}`, `ListAllPendingGuidanceRequestsRequest`/`Response{requests}` (nav badge).
- Files: `proto/session/v1/guidance_request.proto`

##### Task 2.1.1b: Add the RPC service definition and regenerate (~4 min)
- Add `service GuidanceRequestService { rpc CreateGuidanceRequest(...); rpc AnswerGuidanceRequest(...); rpc GetGuidanceRequest(...); rpc ListGuidanceRequests(...); rpc ListAllPendingGuidanceRequests(...); }` to the same file (or register on the existing `SessionService` — check `proto/session/v1/session.proto`'s existing service registration convention first and match it). Run `make proto-gen`, confirm `go build ./...` and `cd web-app && npx tsc --noEmit` (or equivalent) succeed.
- Files: `proto/session/v1/guidance_request.proto`

#### Story 2.1.2: RPC handler service
**As a** frontend developer, **I want** a `GuidanceRequestService` Go handler, **so that** the proto RPCs are backed by the Phase 1 repository methods with ownership/cap enforcement applied.
**Acceptance Criteria** (AC6, AC7):
- Creating a `backlog-item`-scoped request for an item the caller session has no link to is rejected identically to how `resolveItemLink` rejects every other mutating backlog MCP tool (AC6).
  - *Given* session `9f2a-...` (not linked to item `b608ab1e-...`) calls `CreateGuidanceRequest` with `scope="backlog-item"`, `item_id="b608ab1e-..."`, *When* the handler runs, *Then* it returns `PERMISSION_DENIED` with the same remediation text shape `resolveItemLink` already produces for other tools.
**Files**: `server/services/guidance_request_service.go`

##### Task 2.1.2a-pre: Extract `resolveItemLink`'s ownership logic into a package-neutral helper (~6 min)
- **Why**: `resolveItemLink` (`server/mcp/tools_backlog.go:585-617`) contains non-trivial logic beyond a raw storage lookup — it disambiguates `ErrNotFound` on the item-link query from a genuinely-missing item (a second `GetBacklogItem` call) and builds context-specific remediation text (including the "linked to a different item" branch at line 611-613). The new RPC handler (Task 2.1.2a below) needs this SAME logic for its own `backlog-item`-scope ownership check. Reimplementing it in `server/services` instead of reusing it would create two independently-maintained copies that will drift — see Pattern Decisions "Ownership check" row.
- Add a new package-neutral function, e.g. `func ResolveItemLink(ctx context.Context, storage Storage, callerUUID, itemID string) (ItemSessionSummary, ItemLinkError)` in the `session` package (new file, e.g. `session/item_link.go`, or add to an existing item-session file — confirm placement via grep for where `ItemSessionSummary`/`Storage` are already defined so this doesn't introduce a new interface for a single caller). Define `type ItemLinkError struct { Code ItemLinkErrorCode /* OK, NotFound, PermissionDenied */; Message string; Remediation string }` (or equivalent small result type) carrying exactly the two failure shapes `resolveItemLink` currently produces, verbatim (same messages, same remediation text, same "other tools will also fail" warning).
- Rewrite `resolveItemLink` (`server/mcp/tools_backlog.go:585-617`) to call this new helper and translate its `ItemLinkError` into the existing `*mcpgo.CallToolResult` shape (`errResult(ErrItemNotFound, ...)` / `errResult(ErrPermissionDenied, ...)`) — `resolveItemLink`'s own signature and every existing MCP call site are UNCHANGED; only its body becomes a thin adapter. Run `go build ./... && go test ./server/mcp/...` to confirm no MCP-side behavior regressed.
- Files: `session/item_link.go` (new, or confirmed existing sibling file), `server/mcp/tools_backlog.go` (refactor only, `resolveItemLink`'s body)

##### Task 2.1.2a: Implement `CreateGuidanceRequest`/`GetGuidanceRequest`/`ListGuidanceRequests` handlers (~5 min)
- New file `server/services/guidance_request_service.go`. `CreateGuidanceRequest` handler: for `scope=="backlog-item"`, call the SAME `session.ResolveItemLink` helper added in Task 2.1.2a-pre (not a hand-rolled reimplementation) and translate its `ItemLinkError` into a `connect.Error` (`connect.CodeNotFound` / `connect.CodePermissionDenied`) with the identical remediation text `resolveItemLink` produces — this is what makes AC6's "rejected identically to how `resolveItemLink` rejects every other mutating backlog MCP tool" literally true, since both paths now call the same underlying logic; for `scope=="session"`, check caller UUID against the provided `session_uuid` field; for `scope=="standalone"`, no check. Delegate to `EntRepository.CreateGuidanceRequest`, translate `ErrPendingCapExceeded` to `connect.CodeResourceExhausted`.
- Files: `server/services/guidance_request_service.go`

##### Task 2.1.2b: Implement `AnswerGuidanceRequest`/`ListAllPendingGuidanceRequests` handlers (~4 min)
- `AnswerGuidanceRequest`: no ownership check on ANSWER (human-only, single-operator deployment per requirements' Security classification) — call `EntRepository.AnswerGuidanceRequest`, and on `applied==true` publish the `GuidanceRequestEventPayload` (Phase 3) and return the persisted row; on `applied==false` (already answered), return the CURRENT row's answer inline per ux.md's "answer-to-already-answered returns the actual answer, not just a conflict error" ask — NOT a bare error. `ListAllPendingGuidanceRequests`: thin wrapper over the repository method, backs the nav badge.
- Files: `server/services/guidance_request_service.go`

##### Task 2.1.2c: Register the new service in `server/server.go` (~3 min)
- Follow the exact registration pattern used for `StreamHubRolloutService`/`SlackConfigService` in `server/server.go` (constructor call + `mux.Handle` registration).
- Files: `server/server.go`

### Epic 2.2: MCP tools
**Goal**: A session (interactive or headless) can create a `GuidanceRequest` and read back its current state via MCP, reusing `resolveItemLink` for item-scoped ownership.

#### Story 2.2.1: `create_guidance_request` and `get_guidance_request` MCP tools
**As an** agent (e.g. automated triage), **I want** MCP tools to create a guidance request and check its current state, **so that** I can pause on ambiguity and later discover the answer without polling chat.
**Acceptance Criteria** (AC1, AC6):
- A headless triage session calls `create_guidance_request` with `scope="backlog-item"`, gets back a `question_id`, and a later `get_guidance_request` call with that id (from a fresh respawned session) returns the answer once the human has answered it.
  - *Given* triage session `t-abc` (linked to item `b608ab1e-...`) calls `create_guidance_request(item_id="b608ab1e-...", question_type="yes-no", question_text="Merge PR #780 first?")`, *When* it later calls `get_guidance_request(question_id=<returned id>)` after the human answers "yes" via the web UI, *Then* the result includes `status="answered"`, `answer="yes"`.
**Files**: `server/mcp/tools_guidance.go` (new — sibling to `tools_backlog.go`, since this is a new bounded concept per stack.md)

##### Task 2.2.1a: Implement `create_guidance_request` tool (~5 min)
- New file `server/mcp/tools_guidance.go`. For `item_id` present, call `h.resolveItemLink(ctx, callerUUID, itemID)` verbatim (AC6) before creating. Delegate to `EntRepository.CreateGuidanceRequest`, mapping `ErrPendingCapExceeded` to a clear, agent-readable MCP error (not silently swallowed — AC7's "visible, notified condition" requirement applies to the agent-facing path too, not just the UI).
- Files: `server/mcp/tools_guidance.go`

##### Task 2.2.1b: Implement `get_guidance_request` tool + tool registration (~4 min)
- `get_guidance_request(question_id)`: no ownership re-check needed for READ (a session already holding a question_id it created, or was told about via respawn context, is trusted — matches the low-security-classification posture in requirements.md). Register both tools in the MCP tool registry (find the registration list near `wait_for_backlog_event`'s own registration in `server/mcp/`).
- Files: `server/mcp/tools_guidance.go`, MCP tool registry file (grep for where `wait_for_backlog_event` is registered)

---

## Phase 3: Event Bus + Notification Wiring

### Epic 3.1: New scope-uniform event type (ADR-002)
**Goal**: Answering a `GuidanceRequest` publishes exactly one event type, regardless of scope, that downstream consumers (delivery service, any future UI live-refresh) can subscribe to uniformly.

#### Story 3.1.1: `GuidanceRequestEventPayload` and publish adapter
**As a** backend developer, **I want** a new event type published on answer, **so that** delivery/respawn logic and future subscribers don't need a live poll loop (constraint).
**Acceptance Criteria** (AC2):
- Answering `GuidanceRequest` `q1` (scope=session, session_uuid=`7c1e-...`) publishes a `GuidanceRequestEventPayload{ID: "q1", Scope: "session", SessionUUID: &"7c1e-...", Answer: "B"}` on the bus within the same request that persisted the answer.
  - *Given* a subscriber is listening on the bus for `EventTypeGuidanceRequestAnswered`, *When* `AnswerGuidanceRequest` succeeds for `q1`, *Then* the subscriber receives the event with `Scope="session"` and `Answer="B"` before the RPC handler returns its HTTP response (published synchronously inline, matching `BacklogItemEventPublisher`'s call-site timing).
**Files**: `pkg/events/types.go`, `server/services/guidance_request_event_publisher.go` (new, sibling to `backlog_item_event_publisher.go`)

##### Task 3.1.1a: Add the new `EventType` constant and payload struct (~4 min)
- In `pkg/events/types.go`: add `EventTypeGuidanceRequestAnswered EventType = "guidance_request_answered"` and `type GuidanceRequestEventPayload struct { ID uuid.UUID; Scope string; ItemID *uuid.UUID; SessionUUID *string; QuestionType string; Answer string; AnsweredAt time.Time }`, plus `func NewGuidanceRequestAnsweredEvent(payload *GuidanceRequestEventPayload) *Event` mirroring `NewBacklogItemChangedEvent`'s exact shape.
- Files: `pkg/events/types.go`

##### Task 3.1.1b: Write the publish adapter with its own `recover()` (~4 min)
- New file `server/services/guidance_request_event_publisher.go`, `type GuidanceRequestEventPublisher struct { Bus *events.EventBus }`, `func (p *GuidanceRequestEventPublisher) PublishAnswered(payload *events.GuidanceRequestEventPayload)` wrapped in its own top-level `recover()`, mirroring `BacklogItemEventPublisher.PublishItemChanged`'s exact idiom (`server/services/backlog_item_event_publisher.go:21-37`) — a best-effort side channel that must never propagate a panic into the calling RPC handler.
- Files: `server/services/guidance_request_event_publisher.go`

##### Task 3.1.1c: Wire the publisher into `GuidanceRequestService.AnswerGuidanceRequest` (~3 min)
- After a successful `applied==true` answer (Task 2.1.2b), call `p.PublishAnswered(...)`.
- Files: `server/services/guidance_request_service.go`

### Epic 3.2: Durable delivery — live `write_to_session` + `headless.Pool` respawn + durable read fallback
**Goal**: The answer reaches the asker even if the asking session is dead, paused, or the service restarted between question and answer — mirroring the never-implemented `RespondToHelpRequest` design.

#### Story 3.2.1: `GuidanceRequestDeliveryService` — scope-branching resume logic
**As an** asking session (or its automated-triage successor), **I want** to be resumed with the answer once it's given, **so that** I don't need to poll `wait_for_backlog_event` (which is live-only, ≤60s, and unsubscribes on return — confirmed via `server/mcp/tools_backlog.go:775-874`).
**Acceptance Criteria** (AC2):
- A `session`-scoped question answered while the session is still live and attached to a tmux pane is delivered via `write_to_session` within one bus-dispatch tick.
  - *Given* session `s1` is live and asked `scope="session"` question `q2`, *When* the human answers `q2` with "proceed", *Then* `GuidanceRequestDeliveryService` calls `write_to_session(s1, "Guidance request q2 answered: proceed")` (or the internal equivalent, not the MCP tool wrapper) without waiting for `s1` to poll anything.
- A `backlog-item`-scoped question answered while the triage session that asked it has already exited is delivered by respawning a fresh triage session via `headless.Pool`, seeded with the original context + the answer (AC2, and see Phase 5 for the triage-specific integration).
  - *Given* item `b608ab1e-...`'s triage session exited after creating `GuidanceRequest` `q3`, *When* the human answers `q3`, *Then* `GuidanceRequestDeliveryService` triggers `headless.Pool`-based respawn of a fresh triage session for `b608ab1e-...`, seeded with `q3`'s answer in its initial context.
- A successfully-delivered `GuidanceRequest` has `notified_at` set immediately, so the self-heal sweep (Task 8.1.1a) never re-delivers it (AC5, see Task 3.2.1c).
  - *Given* `GuidanceRequestDeliveryService` successfully delivers `q2` via `write_to_session`, *When* the next self-heal sweep tick runs, *Then* it does NOT re-call `write_to_session` for `q2`, because `MarkGuidanceRequestNotified` was already called on the success path.
**Files**: `server/services/guidance_request_delivery_service.go` (new)

##### Task 3.2.1a: Subscribe to the bus and implement the scope switch (~5 min)
- New file `server/services/guidance_request_delivery_service.go`. `type GuidanceRequestDeliveryService struct { bus *events.EventBus; sessionWriter SessionWriter /* narrow interface wrapping write_to_session's internal impl */; triageRespawner TriageRespawner /* reuse the interface `retryOrphanedTriageWithBackoffGate` already consumes, session/backlog_lifecycle_triage.go */ }`. `Run(ctx)`: `ch, _ := bus.Subscribe(ctx)`, loop, on `EventTypeGuidanceRequestAnswered` call a separate `handleAnswered(ctx, payload)` method (extracted specifically so Story 3.2.4's unit tests can call it directly without a live bus subscription) that switches on `payload.Scope`: `"session"` → `sessionWriter.WriteToSession(...)`; `"backlog-item"` → `triageRespawner.AutoRespawnTriageWithContext(ctx, itemID, seedNote)` (the NEW additive method on the SAME respawner interface the orphan detector already uses — Phase 5, Task 5.1.2a/5.1.2b/5.1.4); `"standalone"` → no-op (nothing to resume — see Task 3.2.1c for what "no-op" still means for the notify-marking step); anything else → log a warning and return without calling either mock/dependency (Story 3.2.4's unrecognized-scope case).
- Files: `server/services/guidance_request_delivery_service.go`

##### Task 3.2.1b: Wire `GuidanceRequestDeliveryService` startup in `server/dependencies.go` (~3 min)
- Construct and `Run` it alongside the other background listeners (`BacklogLifecycleListener`, etc.) — find that wiring point in `server/dependencies.go` and mirror it.
- Files: `server/dependencies.go`

##### Task 3.2.1c: Mark the request notified immediately after a successful resume, in every scope branch (~4 min)
- **Why this is its own task, not an implicit part of 3.2.1a**: without this, `EntRepository.MarkGuidanceRequestNotified` (Task 1.1.3c) is defined but never called on the happy path anywhere in the plan. Task 8.1.1a's self-heal sweep retries delivery for every row where `answered_at` is set and `notified_at` is nil, describing that as "a prior delivery attempt failed" — but if nothing ever sets `notified_at` on success, EVERY answered request matches that condition on EVERY sweep tick, forever. For `session` scope specifically this means `write_to_session` would re-inject the same "Guidance request answered: …" text into a live terminal on every sweep tick after the first successful delivery — a concrete, user-visible duplicate-injection bug.
- In each of the 3 scope branches inside Task 3.2.1a's switch, immediately after a successful resume call, call `r.MarkGuidanceRequestNotified(ctx, payload.ID)`: for `"session"`, after `sessionWriter.WriteToSession(...)` returns without error; for `"backlog-item"`, after `triageRespawner.AutoRespawnTriageWithContext(...)` returns without error; for `"standalone"`, mark notified immediately (there is nothing to resume, so "notified" just means "delivery considered complete" — call it unconditionally in that branch). On a resume error, do NOT mark notified — leave `notified_at` nil so Task 8.1.1a's sweep genuinely retries a real failure, which is what that sweep's condition is supposed to mean.
- This makes Task 8.1.1a's sweep-retry condition (`answered_at` set, `notified_at` nil) correct as originally described: it now only matches requests where delivery was attempted and failed, not every answered request unconditionally.
- Files: `server/services/guidance_request_delivery_service.go`

#### Story 3.2.2: Durable read-back fallback (works even if delivery is missed)
**As a** fresh session picking up a backlog item, **I want** to read the current `GuidanceRequest` state directly, **so that** a missed live-delivery event (bus subscriber not running, process crash between publish and delivery) still surfaces the answer.
**Acceptance Criteria** (AC1 — durability metric):
- A `GuidanceRequest` answered while NO delivery subscriber was running (service down) is still visible via `get_guidance_request`/`GetGuidanceRequests` once the service comes back up — no answer is ever silently lost, only its live-delivery timing is delayed.
  - *Given* `GuidanceRequest` `q4` is answered while the service is down, *When* the service restarts and a (possibly different, respawned) session calls `get_guidance_request(question_id="q4")`, *Then* it returns `status="answered"`, `answer=<value>` — the row itself is the source of truth, live delivery is a latency optimization on top of it, never the only path.
**Files**: none new — this is a design property of Phase 1/2's durable storage + Phase 2's read RPCs/tools, verified by an integration test.

##### Task 3.2.2a: Integration test proving durable read-back survives a missed delivery (~5 min)
- Test that answers a `GuidanceRequest` with the `GuidanceRequestDeliveryService` NOT running (simulating a crash), then asserts `GetGuidanceRequest` still returns the correct answered state. Place alongside other cross-service integration tests (check `server/services/*_integration_test.go` naming convention).
- Files: `server/services/guidance_request_delivery_integration_test.go`

#### Story 3.2.3: Follow-up "resumed" notification (ux.md JTBD mitigation)
**As the** human answerer, **I want** to know the asking session actually consumed my answer, **so that** a stale local "✓ Answered" echo isn't the only signal something happened.
**Acceptance Criteria** (AC2):
- After `GuidanceRequestDeliveryService` successfully resumes session `s1` (or respawns triage for item `b608ab1e-...`), a notification is posted through the existing notification pipeline.
  - *Given* `GuidanceRequestDeliveryService` completes a `write_to_session` resume for `s1` after answering `q2`, *When* the notification pipeline's history is queried (`get_notification_history`), *Then* it includes an entry like "Session s1 resumed after your answer to q2".
**Files**: `server/services/guidance_request_delivery_service.go`, `server/services/notification_service.go` (read only — see resolved decision below)

**Resolved decision (was previously an Unresolved Question — see B5/adversarial-review.md Blocker 3): no new proto enum value.** The notification pipeline's wire type is `sessionv1.NotificationType`, a CLOSED protobuf enum (`proto/session/v1/types.proto:940-965`, verified via direct read — 15 fixed values from `NOTIFICATION_TYPE_UNSPECIFIED` (0) through `NOTIFICATION_TYPE_AUTO_APPROVED` (13) plus `NOTIFICATION_TYPE_CUSTOM` (100), not an open Go-side `Kind` enum as an earlier draft of this plan assumed). Adding a genuinely new value would require a proto change + `make proto-gen` mid-implementation, which is disproportionate for a single follow-up notification. Instead, reuse **`NOTIFICATION_TYPE_STATUS_CHANGE`** (`proto/session/v1/types.proto:961`, doc comment "Session status changed") — this is the best existing fit for "an asking session/item transitioned from halted-awaiting-answer back to active/resumed," for both the `session`-scope (`write_to_session` resume) and `backlog-item`-scope (triage respawn) cases, and needs zero proto/schema changes.

##### Task 3.2.3a: Post a notification on successful resume, using `NOTIFICATION_TYPE_STATUS_CHANGE` (~4 min)
- In `GuidanceRequestDeliveryService`'s scope-switch (Task 3.2.1a/3.2.1c), after a successful `write_to_session`/`AutoRespawnTriage` call (and its accompanying `MarkGuidanceRequestNotified`), call the existing notification pipeline's post method (find its exact signature by reading `server/services/notification_service.go`) with `sessionv1.NotificationType_NOTIFICATION_TYPE_STATUS_CHANGE` and a message like `"Session {id} resumed after your answer to {question_id}"` / `"Item {id} triage resumed after your answer"` — no proto change needed, per the resolved decision above.
- Files: `server/services/guidance_request_delivery_service.go`

#### Story 3.2.4: Unit tests for `GuidanceRequestDeliveryService`'s scope switch
**As a** backend developer, **I want** unit tests directly exercising the 3-way scope switch with mock `SessionWriter`/`TriageRespawner`, **so that** a wrong-scope match or a `notified_at` regression is caught by CI, not discovered in production (pre-mortem P1 #3).
**Acceptance Criteria** (AC2):
- Each scope's success path, exercised directly against `handleAnswered` (Task 3.2.1a) with mock dependencies, calls the correct mock method exactly once and marks the request notified.
  - *Given* mock `SessionWriter`/`TriageRespawner` configured to succeed, *When* `handleAnswered(ctx, payload)` is called once per scope (`session`, `backlog-item`, `standalone`) with a distinct payload for each, *Then* the scope-appropriate mock method is invoked exactly once per call and `MarkGuidanceRequestNotified` (Task 3.2.1c) is called for each.
- Each scope's error path, exercised the same way, leaves `notified_at` unset.
  - *Given* mock `SessionWriter`/`TriageRespawner` configured to return an error, *When* `handleAnswered(ctx, payload)` is called once per scope, *Then* `MarkGuidanceRequestNotified` is NOT called for any of the three — this is the exact regression class Blocker 1 (prior repair round) was about, now directly guarded by a test instead of only by code review.
- An unrecognized `Scope` value is handled without panicking and without calling either dependency.
  - *Given* a `GuidanceRequestEventPayload{Scope: "carrier-pigeon"}`, *When* `handleAnswered(ctx, payload)` is called, *Then* it logs a warning, does not panic, does not call `SessionWriter`/`TriageRespawner`, and does not call `MarkGuidanceRequestNotified`.

**This test coverage is a required dependency of Story 5.1.2 (and 5.1.4): Phase 5's respawn integration is not considered complete until these unit tests exist and pass, since both stories add behavior directly inside the same switch this story tests.**
**Files**: `server/services/guidance_request_delivery_service_test.go` (new)

##### Task 3.2.4a: Extract `handleAnswered` and define mock `SessionWriter`/`TriageRespawner` test doubles (~5 min)
- Confirm Task 3.2.1a's `handleAnswered(ctx, payload)` extraction (amended above) is in place — `Run`'s loop must call it, not inline the switch, so it's callable directly from a test with no live bus subscription. In new file `server/services/guidance_request_delivery_service_test.go`, define a `mockSessionWriter` (records calls, returns a configurable error) implementing `SessionWriter`, and a `mockTriageRespawner` implementing `TriageRespawner` (`AutoRespawnTriageWithContext` from Task 5.1.2a, plus `IsLive` from Task 5.1.4b once that task lands — stub `IsLive` to return `false` if this task is implemented before 5.1.4b), both with a configurable error return and a call-count/call-args recorder.
- Files: `server/services/guidance_request_delivery_service_test.go`

##### Task 3.2.4b: Write success-path tests per scope (~5 min)
- `TestGuidanceRequestDeliveryService_HandleAnswered_SessionScope_Success`, `..._BacklogItemScope_Success`, `..._StandaloneScope_Success`: construct a `GuidanceRequestDeliveryService` with the mocks from Task 3.2.4a, call `handleAnswered(ctx, payload)` with a payload for that scope, assert the correct mock is invoked exactly once and `MarkGuidanceRequestNotified` fires (via a mock/fake repository — reuse whatever in-memory repository fake Task 1.1.4's tests already use, or a narrow interface mock if the repository dependency is itself already an interface).
- Files: `server/services/guidance_request_delivery_service_test.go`

##### Task 3.2.4c: Write error-path tests per scope, asserting `notified_at` stays nil (~5 min)
- Same 3 scopes, mock configured to return an error; assert `MarkGuidanceRequestNotified` is NOT called. Directly guards the Blocker-1-class regression (notified_at set despite a failed delivery).
- Files: `server/services/guidance_request_delivery_service_test.go`

##### Task 3.2.4d: Write the unrecognized-scope test (~3 min)
- `TestGuidanceRequestDeliveryService_HandleAnswered_UnrecognizedScope`: `payload.Scope = "carrier-pigeon"`; assert no panic, a `WarningLog` line, neither mock called, `MarkGuidanceRequestNotified` not called.
- Files: `server/services/guidance_request_delivery_service_test.go`

---

## Phase 4: Feature Flag (`triage_guidance_halt`)

### Epic 4.1: Global-only live-settable flag (ADR-003)
**Goal**: AC3's triage halt-and-wait behavior is off by default and toggleable live with no restart, no env var, no rehearsal-gate precondition.

#### Story 4.1.1: Flag constant, accessor, and pending-cap config
**As an** operator, **I want** to flip `triage_guidance_halt` on/off live, **so that** I can roll out AC3's behavior without a deploy.
**Acceptance Criteria** (AC3):
- Setting `triage_guidance_halt=true` via the settings panel takes effect for the NEXT triage halt-decision without a restart.
  - *Given* the flag is OFF and triage is mid-run for item `b608ab1e-...`, *When* an operator sets it to `true` via `UpdateFeatureFlag`, *Then* the VERY NEXT ambiguous-item triage run (not the one already in flight) halts and creates a `GuidanceRequest` instead of guessing.
**Files**: `config/config.go`

##### Task 4.1.1a: Add the flag constant, accessor, and pending-cap config field (~5 min)
- In `config/config.go`, near `TymuxFeatureFlag` (`config/config.go:465-470`): `const TriageGuidanceHaltFeatureFlag = "triage_guidance_halt"` with a doc comment mirroring `TymuxFeatureFlag`'s ("Defaults to off ... no rollback rehearsal precondition"). Add `func EffectiveTriageGuidanceHaltEnabled(cfg *Config) bool { return cfg.GetFeatureFlagWithDefault(TriageGuidanceHaltFeatureFlag, false) }` mirroring `EffectiveTymuxEnabled`. Add `GuidanceRequestPendingCap int \`json:"guidance_request_pending_cap,omitempty"\`` field to the `Config` struct and `func (c *Config) GetGuidanceRequestPendingCap() int { if c == nil || c.GuidanceRequestPendingCap <= 0 { return 4 }; return c.GuidanceRequestPendingCap }` (default **4**, configurable, per requirements' Open Questions resolution).
- Files: `config/config.go`

##### Task 4.1.1b: Register the flag name for the settings panel (~3 min)
- Add `TriageGuidanceHaltFeatureFlag` to `knownFeatureFlags` in `server/services/feature_flag_service.go` (per stack.md's citation) so the generic `GetFeatureFlags` RPC surfaces it in the existing settings panel's flag list — no new RPC or panel component needed (ADR-003: global-only, reuse the generic mechanism).
- Files: `server/services/feature_flag_service.go`

---

## Phase 5: Triage Integration

### Epic 5.1: Halt-on-ambiguity + respawn via `headless.Pool`
**Goal**: A per-item headless triage session, when it hits genuine ambiguity and the flag is on, creates a `GuidanceRequest` and exits deliberately instead of guessing or looping — and the orphan detector treats that exit as expected, not anomalous.

#### Story 5.1.1: Triage creates a `GuidanceRequest` and exits on ambiguity
**As** automated triage, **I want** to halt and ask instead of guessing when I'm genuinely unsure, **so that** AC3's zero-ambiguous-guess bar is met once the flag is on.
**Acceptance Criteria** (AC3):
- With `triage_guidance_halt=true`, a triage run for item `b608ab1e-...` that hits an ambiguous requirement calls `create_guidance_request` and exits WITHOUT calling `submit_triage_result`.
  - *Given* the flag is `true` and triage session `t-abc` (linked to `b608ab1e-...`) determines the item's scope is genuinely ambiguous, *When* it decides to halt, *Then* it calls `create_guidance_request(item_id="b608ab1e-...", question_type="short-answer", question_text="Should this item include the mobile client changes or just backend?")` and then exits its turn (no `submit_triage_result` call, no guess recorded).
- With `triage_guidance_halt=false` (default), the same ambiguous item is triaged as it is today (unchanged baseline behavior) — this story does NOT change default behavior.
  - *Given* the flag is `false`, *When* the same ambiguous item is triaged, *Then* triage proceeds exactly as it does today (guess-and-proceed baseline), unchanged.
**Files**: triage prompt/instructions surface — find the exact file (likely `server/services/backlog_service_triage.go` or a prompt-template file it references) that constructs the headless triage session's system instructions.

##### Task 5.1.1a: Locate and extend the triage prompt/instruction construction with the flag-gated halt guidance (~5 min)
- Grep `backlog_service_triage.go` for where the triage session's prompt/instructions are assembled (likely a `buildTriagePrompt`-style function). Read `config.EffectiveTriageGuidanceHaltEnabled(config.LoadConfig())` AT THE INSTANT the prompt is built (not cached earlier in the pass, per pitfalls.md's explicit warning) and conditionally include instructions telling the agent to call `create_guidance_request` and stop, instead of guessing, when genuinely ambiguous.
- Files: `server/services/backlog_service_triage.go` (exact prompt-construction function, confirm via grep before editing)

#### Story 5.1.2: Respawn via `headless.Pool` seeded with the answer
**As** automated triage, **I want** to be automatically re-run with the human's answer once it's given, **so that** the item doesn't sit halted forever waiting for a human to manually re-trigger it.
**Acceptance Criteria** (AC3):
- Once `GuidanceRequest` `q3` (created by item `b608ab1e-...`'s triage) is answered, a FRESH triage session is spawned via `headless.Pool`, with the answer included in its initial context.
  - *Given* `q3` is answered "backend only", *When* `GuidanceRequestDeliveryService` (Phase 3, Task 3.2.1a) handles the `backlog-item`-scope branch, *Then* it calls a NEW `TriageRespawner.AutoRespawnTriageWithContext` method (added alongside, not replacing, the existing `AutoRespawnTriage` that `retryOrphanedTriageWithBackoffGate` already uses at `session/backlog_lifecycle_triage.go:372-398` — see Task 5.1.2a for why this is additive), passing the answer as seed context so the new triage session sees "the human answered: backend only" rather than re-asking or re-guessing.
**Files**: `server/services/guidance_request_delivery_service.go`, `session/backlog_lifecycle_triage.go` (add a new `TriageRespawner` interface method; existing `AutoRespawnTriage` and its call sites are untouched)

##### Task 5.1.2a: Add a NEW `TriageRespawner` interface method for seed-context respawn — do not modify the existing `AutoRespawnTriage` signature (~7 min)
- **Why additive, not a signature change**: `AutoRespawnTriage(ctx context.Context, itemID string) error` (`session/backlog_lifecycle_triage.go:25-30`) has at least 2 existing call sites (`reconcileOrphanedTriageItems`'s shutdown-respawn branch, `retryOrphanedTriageWithBackoffGate`) plus a test fake (`fakeTriageRespawner`, `session/backlog_lifecycle_stuck_test.go:1835`) and direct test call sites (`server/services/backlog_service_triage_test.go`) that all pass exactly 2 args. Go has no default parameter values, so adding a `seedNote string` parameter to this method — as an earlier draft of this task proposed — is not "adding an optional parameter"; it requires editing the interface, every concrete implementation (`BacklogService.AutoRespawnTriage`, `server/services/backlog_service_triage.go:2407`), the test fake, and every existing call site purely to preserve their current behavior with a `""` argument. That is 3+ existing touch points modified to serve one new consumer — an OCP violation on a shared interface (architecture-review.md Concern, adversarial-review.md Concern 1).
- Instead, add a NEW interface method alongside the existing untouched one: `AutoRespawnTriageWithContext(ctx context.Context, itemID, seedNote string) error`. Add it to the `TriageRespawner` interface (`session/backlog_lifecycle_triage.go`) and implement it on every concrete type that implements `TriageRespawner` today — `BacklogService` (`server/services/backlog_service_triage.go`) gets a real implementation (or a one-line wrapper delegating to the existing `AutoRespawnTriage` body with the seed note threaded into the initial prompt, if a single shared implementation body is preferred), and `fakeTriageRespawner` (`session/backlog_lifecycle_stuck_test.go:1835`) gets a test-double implementation. The existing `AutoRespawnTriage` method, its signature, and its 2 existing call sites are UNCHANGED — zero edits to `reconcileOrphanedTriageItems`'s shutdown-respawn branch, `retryOrphanedTriageWithBackoffGate`, or the existing direct test call sites in `backlog_service_triage_test.go`.
- Files: `session/backlog_lifecycle_triage.go` (interface only — add the new method, do not touch the existing one), `server/services/backlog_service_triage.go` (new method implementation), `session/backlog_lifecycle_stuck_test.go` (`fakeTriageRespawner` gets the new method)

##### Task 5.1.2b: Call the NEW respawner method with the answer from `GuidanceRequestDeliveryService` (~3 min)
- In Task 3.2.1a's `backlog-item` branch, call `triageRespawner.AutoRespawnTriageWithContext(ctx, itemID, fmt.Sprintf("The human answered your guidance request: %s", payload.Answer))` — the new method from Task 5.1.2a, not the existing `AutoRespawnTriage`.
- Files: `server/services/guidance_request_delivery_service.go`

#### Story 5.1.3: REQUIRED — `reconcileOrphanedTriageItems` new branch for halted-on-guidance-request
**As** the orphan-triage detector, **I want** to recognize a triage session halted behind an open `GuidanceRequest` as expected, **so that** it doesn't retry-with-backoff-penalize a legitimately-halted item and defeat AC3.
**Acceptance Criteria** (AC3 — this is the explicitly-required non-additive touch point):
- An item whose latest triage session ended with no plan, but has an open `GuidanceRequest`, is NOT flagged as an anomaly requiring backoff-penalized retry.
  - *Given* item `b608ab1e-...`'s triage session ended (no `submit_triage_result` call) because it created `GuidanceRequest` `q3` and exited, *When* `reconcileOrphanedTriageItems` runs its periodic tick, *Then* it checks for an open `GuidanceRequest` for `b608ab1e-...` BEFORE falling through to the existing `retryOrphanedTriageWithBackoffGate` path, finds one open, and skips the item (no backoff penalty applied, no "stuck" notification fired for this reason).
- Flipping `triage_guidance_halt` off does NOT retroactively unstick an already-halted item (documented invariant from ADR-003's Consequences).
  - *Given* item `b608ab1e-...` is halted behind an open `GuidanceRequest`, *When* an operator sets `triage_guidance_halt=false`, *Then* `b608ab1e-...` remains halted (still requires the human to answer `q3`, or manually intervene) — the flag only gates whether NEW halts occur, not existing ones.
**Files**: `session/backlog_lifecycle_triage.go`

##### Task 5.1.3a: Add the new early-exit branch to `reconcileOrphanedTriageItems` (~5 min)
- In `reconcileOrphanedTriageItems` (`session/backlog_lifecycle_triage.go:172`), immediately after `latestTriage := latestTriageSession(sessions)` finds an ended-without-plan or stale-and-open session (i.e. right before the existing shape-1/2/3 branching at line ~197), add: check `l.storage.ListPendingGuidanceRequests(ctx, "backlog-item", item.ID)` (Task 1.1.3d) — if any open (unanswered, uncancelled) row exists for this item, `continue` immediately (skip this item entirely this tick, no tombstone, no `MarkStuck`, no backoff). Add a one-line comment citing this exact story: "halted behind an open GuidanceRequest is expected once AC3's flag is on — not an anomaly."
- Files: `session/backlog_lifecycle_triage.go`

##### Task 5.1.3b: Test the new branch does not fire the existing backoff/stuck path (~4 min)
- Extend or add to the existing `reconcileOrphanedTriageItems` test file (find it via grep) with a case: item has an ended triage session (no plan) AND an open `GuidanceRequest` → asserts `MarkStuck`/`retryOrphanedTriageWithBackoffGate` are NOT called (using whatever mock/fake storage this test file already uses).
- Files: `session/backlog_lifecycle_triage_test.go` (confirm exact name via grep)

#### Story 5.1.4: Idempotent, batched respawn across an item's open guidance requests (pre-mortem P1 #1)
**As** automated triage, **I want** the delivery service to respawn at most once per item even when 2+ open `GuidanceRequest`s on that item are answered near-simultaneously, **so that** concurrent answers don't spawn concurrent triage sessions racing on the same git worktree/branch.
**Acceptance Criteria** (AC3):
- Two open `GuidanceRequest`s on the same item, answered in quick succession, trigger exactly ONE respawn, seeded with both answers.
  - *Given* item `b608ab1e-...` has 2 open `GuidanceRequest`s `q1`/`q2`, *When* both are answered within the same tick (near-simultaneously, e.g. two goroutines each publishing/handling a `GuidanceRequestEventPayload`), *Then* `GuidanceRequestDeliveryService`'s `backlog-item` branch calls `triageRespawner.AutoRespawnTriageWithContext` exactly ONCE for `b608ab1e-...`, with a seed note listing both `q1`'s and `q2`'s question/answer pairs — not one call per answer.
- A respawn already in flight for an item suppresses a second concurrent respawn attempt for that same item.
  - *Given* `BacklogService.IsTriageLive("b608ab1e-...")` (`server/services/backlog_service_triage.go:2450-2453`, the existing `triageInFlight`-backed liveness check the periodic orphan sweep already treats as "the single source of truth" for in-flight triage per item) reports `true`, *When* another `GuidanceRequest` for the same item is answered concurrently, *Then* `GuidanceRequestDeliveryService` does NOT call `AutoRespawnTriageWithContext` again — it logs a skip (`item_id`) and relies on the already-running session to observe the batched answers via its own fresh prompt-construction read (Task 5.1.1a already reads current state at build time, not a stale snapshot at pass start).
**Files**: `server/services/guidance_request_delivery_service.go`, `session/backlog_lifecycle_triage.go`, `session/backlog_lifecycle_stuck_test.go`, `server/services/guidance_request_delivery_service_test.go`

##### Task 5.1.4a: Defer respawn while sibling open requests remain for the item (~5 min)
- In `GuidanceRequestDeliveryService`'s `backlog-item` branch (Task 3.2.1a/5.1.2b), before calling `triageRespawner.AutoRespawnTriageWithContext`, call `er.ListPendingGuidanceRequests(ctx, "backlog-item", itemID)` (Task 1.1.3d — returns the current open count fresh, not a stale snapshot). If the open count is greater than 0 (i.e. at least one OTHER `GuidanceRequest` for this item is still unanswered besides the one that just triggered this event), skip the respawn for this event entirely — log at `InfoLog` with `item_id`/`remaining_open_count` — and do not call `AutoRespawnTriageWithContext`. Because each `AnswerGuidanceRequest` commits before its event is published (Task 3.1.1c), and `ListPendingGuidanceRequests` re-queries live state at call time, the LAST of the near-simultaneous answers to reach this check is the one that observes `remaining_open_count == 0` and is the one that actually respawns — no separate scheduler, timer, or batching window is needed.
- Files: `server/services/guidance_request_delivery_service.go`

##### Task 5.1.4b: Batch all recently-answered requests into one seed note + add the in-flight debounce guard (~6 min)
- When Task 5.1.4a's check finds zero remaining open requests (the trigger to actually respawn), fetch every `GuidanceRequest` for this item with `answered_at` set and `notified_at` nil — the same set Task 8.1.1a's self-heal sweep already treats as "answered but not yet delivered" — and build ONE seed note listing each `question_text`/`answer` pair, instead of Task 5.1.2b's single-answer format. Immediately before calling `AutoRespawnTriageWithContext`, add the debounce guard: extend the `TriageRespawner` interface (defined in Task 5.1.2a, alongside `AutoRespawnTriageWithContext`) with `IsLive(itemID string) bool`, implemented by `BacklogService.IsTriageLive` (already exists at `server/services/backlog_service_triage.go:2450-2453` — no new liveness-tracking mechanism needed, this method is already documented as the canonical in-flight check the orphan sweep relies on). If `IsLive(itemID)` returns `true`, skip the respawn (log a skip, do not call `AutoRespawnTriageWithContext`) — a session for this item is already running and will see the batched answers itself. Add the matching `IsLive` stub to `fakeTriageRespawner` (`session/backlog_lifecycle_stuck_test.go:1835`).
- Files: `server/services/guidance_request_delivery_service.go`, `session/backlog_lifecycle_triage.go` (interface addition only), `session/backlog_lifecycle_stuck_test.go`

##### Task 5.1.4c: Test — 2 open requests answered near-simultaneously trigger exactly one respawn (~6 min)
- In `server/services/guidance_request_delivery_service_test.go` (shared with Story 3.2.4 — confirm no duplicate file is created), `TestGuidanceRequestDeliveryService_MultiQuestionItem_SingleRespawn`: create 2 open `GuidanceRequest`s for the same item, answer both concurrently (2 goroutines each driving `handleAnswered` with its own payload), assert the mock `TriageRespawner`'s `AutoRespawnTriageWithContext` is called exactly once, with a seed note containing BOTH answers. Add a second case where the mock's `IsLive` returns `true` throughout: assert `AutoRespawnTriageWithContext` is never called.
- Files: `server/services/guidance_request_delivery_service_test.go`

---

## Phase 6: Frontend Shared Component (AC4)

### Epic 6.1: `GuidanceRequestCard` — one component, three call sites
**Goal**: One component file renders a pending/answered/cancelled `GuidanceRequest` identically across backlog item detail, the triage panel, and the session view.

#### Story 6.1.1: Build `GuidanceRequestCard` with all 3 question-type controls
**As a** user, **I want** to answer a yes/no, multiple-choice, or short-answer question from any of the 3 views with the same interaction pattern, **so that** the UI doesn't fork into three different "ask" experiences (AC4).
**Acceptance Criteria** (AC4):
- The exact same `GuidanceRequestCard.tsx` file, unmodified, is imported and rendered by backlog item detail, the triage panel, and the session view.
  - *Given* `GuidanceRequest` `q5` (`question_type="multiple-choice"`, `options=["Option A","Option B","Option C"]`) is pending, *When* it's rendered via `<GuidanceRequestCard request={q5} />` from `BacklogItemDetail.tsx`, `TriageReviewPanel.tsx`, and the session view's equivalent, *Then* all three renders produce the same `role="radiogroup"` markup and the same submit behavior — verified by one shared `GuidanceRequestCard.test.tsx` exercising the component in isolation (not 3 separate per-view test files asserting 3 different things).
**Files**: `web-app/src/components/backlog/GuidanceRequestCard.tsx`, `GuidanceRequestCard.css.ts`, `GuidanceRequestCard.test.tsx`

##### Task 6.1.1a: Scaffold the component with the yes-no and short-answer controls (~5 min)
- New file `GuidanceRequestCard.tsx`, modeled on `TriageDiffSection.tsx`'s exact answer-flow shape (collapsed `Answer ▸` toggle → control → Submit/Cancel → `✓ Answered: {value}` echo). Whole card: `role="form"` + `aria-label={"Answer: " + request.questionText}` (mirrors `TriageDiffSection`'s exact pattern), wrapped in `aria-live="polite"`. Yes-no: native `role="radiogroup"` with two `<input type="radio">`, roving tabindex. Short-answer: `<textarea>` + Submit/Cancel, `aria-disabled`+`disabled` together, `aria-busy` while submitting, `InlineError type="transient"` on failure — all copied verbatim from `TriageDiffSection.tsx`'s existing conventions.
- Files: `web-app/src/components/backlog/GuidanceRequestCard.tsx`

##### Task 6.1.1b: Add the multiple-choice control (~4 min)
- `role="radiogroup"` + `<input type="radio">` per option for ≤6-8 options (per ux.md); `<select>`/`role="listbox"` above that threshold. Focus moves to the new control's first focusable element only on an explicit user action (card click), never auto-focused on mount (ux.md).
- Files: `web-app/src/components/backlog/GuidanceRequestCard.tsx`

##### Task 6.1.1c: Add the answered/cancelled/readOnly render states (~5 min)
- Answered state: static echo of the answer, no interactive controls in the DOM (not just `disabled` — actually absent), mirroring `TriageReviewPanel.tsx`'s `readOnly` prop pattern for historical/archived contexts. Cancelled state (scope archived while pending, per pitfalls.md): explicit "this question is no longer needed" copy, not silent vanish. Auto-transition to answered state using the returned answer payload when a stale pending card receives an "already answered" response (ux.md), rather than forcing a manual reload — surface "This question was already answered — reload to see the answer" only if the payload truly lacks the answer value.
- Files: `web-app/src/components/backlog/GuidanceRequestCard.tsx`

##### Task 6.1.1d: Add the `.css.ts` vanilla-extract styles and the component test (~5 min)
- `GuidanceRequestCard.css.ts` modeled on `GateVerdictBox.css.ts`'s structure. `GuidanceRequestCard.test.tsx`: render each `question_type` × each state (pending/answered/cancelled) combination, assert ARIA roles, assert no interactive control renders in the answered/cancelled states, assert the cap display (Story 6.1.2) renders when `pendingCount`/`cap` props are passed.
- Files: `web-app/src/components/backlog/GuidanceRequestCard.css.ts`, `GuidanceRequestCard.test.tsx`

#### Story 6.1.2: Proactive cap display
**As a** user, **I want** to see "N/cap pending" before I hit the cap, not just a rejection after, **so that** I understand why a create might fail (ux.md concrete ask).
**Acceptance Criteria** (AC7):
- A view showing 3 of 4 allowed pending requests for an item displays "3/4 pending" without a separate round trip.
  - *Given* `ListGuidanceRequests` response includes `pending_count=3`, `cap=4`, *When* the view renders, *Then* it shows "3/4 pending" using only the fields already present on that response (no extra fetch).
**Files**: `web-app/src/components/backlog/GuidanceRequestCard.tsx` (or a small sibling summary component if the 3 call sites render the count outside the per-question card — decide during implementation per Unresolved Questions)

##### Task 6.1.2a: Render the pending-count/cap summary (~3 min)
- Add a small summary render (exact placement/copy per Unresolved Questions — default: show only when `pendingCount >= cap - 1`) using the `pending_count`/`cap` fields already returned by `ListGuidanceRequests` (Task 2.1.1a).
- Files: `web-app/src/components/backlog/GuidanceRequestCard.tsx`

#### Story 6.1.3: Wire `GuidanceRequestCard` into the 3 views
**As a** user, **I want** to see and answer guidance requests from wherever I'm already looking at an item/session, **so that** I don't need a 4th dedicated page (AC4).
**Acceptance Criteria** (AC4):
- All 3 views render pending `GuidanceRequest`s for their respective scope.
  - *Given* item `b608ab1e-...` has a pending `GuidanceRequest`, *When* the backlog item detail view and the triage panel are both opened, *Then* both show it via `<GuidanceRequestCard>` — a stale one that didn't submit the answer invalidates its cached pending state on interaction (ux.md, no push subscription exists per ssq#428) rather than risking a duplicate-answer submission hitting the AC5 race.
**Files**: `web-app/src/components/backlog/BacklogItemDetail.tsx` (confirm exact name), `TriageReviewPanel.tsx`, the session view's top-level component (confirm exact name)

##### Task 6.1.3a: Wire into backlog item detail (~4 min)
- Fetch pending `GuidanceRequest`s for `scope="backlog-item"` via the new RPC, render `<GuidanceRequestCard>` per item, refetch on focus/interaction (no push subscription, per ssq#428/ux.md).
- Files: backlog item detail component file (confirm via grep)

##### Task 6.1.3b: Wire into the triage panel (~4 min)
- Same pattern as 6.1.3a, scoped to the triage panel's item context.
- Files: `web-app/src/components/backlog/TriageReviewPanel.tsx`

##### Task 6.1.3c: Wire into the session view (~4 min)
- Fetch pending `GuidanceRequest`s for `scope="session"` keyed by the session's UUID.
- Files: session view top-level component (confirm via grep)

---

## Phase 7: Nav Badge (Required per UX Research)

### Epic 7.1: `GuidanceRequestNavBadge`
**Goal**: A global, always-visible signal of pending guidance requests, since a stalled item burns a scarce concurrency slot (2-item WIP cap) — per ux.md, this is REQUIRED for v1, not optional polish.

#### Story 7.1.1: Build and wire the nav badge
**As a** user, **I want** a global badge showing how many guidance requests are pending, **so that** I notice a stalled item without hunting through 3 separate views.
**Acceptance Criteria** (AC2 — resumption visibility):
- With 2 pending guidance requests across all scopes, the badge shows "2" with a full-sentence `aria-label`; with 0, it's hidden.
  - *Given* `ListAllPendingGuidanceRequests` returns 2 rows, *When* `Header.tsx`/`BottomNav.tsx` render, *Then* `GuidanceRequestNavBadge` shows "2" with `aria-label="2 pending guidance requests. Click to review."`; *When* the count drops to 0 (all answered), *Then* the badge is hidden (not rendered with a "0", matching `ApprovalNavBadge`'s hide-at-zero convention).
**Files**: `web-app/src/components/sessions/GuidanceRequestNavBadge.tsx`, `Header.tsx`, `BottomNav.tsx`

##### Task 7.1.1a: Build the badge component + context hook (~5 min)
- New file `GuidanceRequestNavBadge.tsx`, modeled exactly on `ApprovalNavBadge.tsx`: a `useGuidanceRequestsContext()` hook feeding the shared `NavBadge` primitive, full-sentence `aria-label`, hides at zero.
- Files: `web-app/src/components/sessions/GuidanceRequestNavBadge.tsx`

##### Task 7.1.1b: Wire into `Header.tsx`/`BottomNav.tsx` (~3 min)
- Add `<GuidanceRequestNavBadge />` alongside the 5 existing sibling badges, matching their exact placement convention.
- Files: `web-app/src/components/.../Header.tsx`, `BottomNav.tsx` (confirm exact paths via grep for `ApprovalNavBadge` usage)

---

## Phase 8: Self-Heal Sweep

### Epic 8.1: Periodic reconciliation for missed notify-once writes and archived/deleted/torn-down-scope cancellation
**Goal**: Per ADR-001/pitfalls.md's "best-effort precondition, not cross-table atomicity" posture, a periodic sweep is the actual correctness backstop for races the inline pre-filter checks miss — covering all 3 scopes' failure modes (archived or hard-deleted `backlog-item`, torn-down `session`; `standalone` has nothing to check by design), not just `backlog-item`-archival.

#### Story 8.1.1: Self-heal sweep on the existing `BacklogLifecycleListener` tick
**As** the system, **I want** a periodic pass that retries missed `notified_at` writes and cancels requests whose owning scope was archived, hard-deleted, or torn down, **so that** a `GuidanceRequest` never sits in an indefinite, indistinguishable-from-live limbo (pitfalls.md).
**Acceptance Criteria** (AC5):
- A `GuidanceRequest` whose owning backlog item was archived while the question was still pending is transitioned to `cancelled_at` set on the next sweep tick, not left as `pending` forever.
  - *Given* `GuidanceRequest` `q6` for item `b608ab1e-...` is pending, and `b608ab1e-...` is then archived, *When* the next self-heal sweep tick runs, *Then* `q6.cancelled_at` is set, and the UI (Task 6.1.1c's cancelled-state render) reflects it on next fetch instead of showing a live-looking pending card for a dead item.
- A `GuidanceRequest` whose owning backlog item was HARD-DELETED (not just archived) while the question was still pending is likewise transitioned to `cancelled_at` set on the next sweep tick (this is the case Task 1.1.1b's deliberate non-cascading `item_id` edge exists to make possible — see B2/architecture-review.md Blocker 2).
  - *Given* `GuidanceRequest` `q7` for item `b608ab1e-...` is pending, and `b608ab1e-...` is then hard-deleted via `DeleteBacklogItem`, *When* the next self-heal sweep tick runs, *Then* `l.storage.GetBacklogItem` returns `ErrNotFound`, and `q7.cancelled_at` is set — the row survives as a durable, visible cancellation record rather than being silently destroyed by a cascade.
- A `GuidanceRequest` whose owning `session` was torn down while the question was still pending is likewise cancelled on the next sweep tick, not left pending forever with no possible answer delivery.
  - *Given* `session`-scoped `GuidanceRequest` `q8` is pending for session `7c1e-...`, and that session is later torn down/removed, *When* the next self-heal sweep tick runs, *Then* it detects the session no longer exists and sets `q8.cancelled_at`.
**Files**: `session/backlog_lifecycle.go` (or the specific file defining the periodic tick loop — confirm via grep for where `reconcileOrphanedTriageItems` itself is invoked on a timer)

##### Task 8.1.1a: Add the self-heal sweep function and wire it into the existing tick (~7 min)
- Find the existing periodic-tick dispatch point (the function that calls `reconcileOrphanedTriageItems` on a timer). Add a sibling call `reconcileGuidanceRequests(ctx, er)` covering all 3 scopes:
  - **`backlog-item` scope**: for each open `GuidanceRequest`, check the owning item's state via `l.storage.GetBacklogItem`. Treat BOTH of the following as cancel-worthy (call `CancelGuidanceRequest` in either case): (a) the item is found but `archived` (the original case), and (b) `GetBacklogItem` returns `ErrNotFound` — the item was hard-deleted (`DeleteBacklogItem`). Case (b) exists specifically because Task 1.1.1b's `item_id` edge deliberately has no `OnDelete(Cascade)`, so a hard-deleted item's `GuidanceRequest` rows survive with a now-dangling `item_id` for this sweep to find and cancel, instead of being cascade-deleted untraceably.
  - **`session` scope**: for each open `GuidanceRequest`, check whether the owning session still exists via the codebase's existing "does this session UUID still exist" check (grep `GetItemSessionBySessionUUID`'s not-found handling, or the equivalent session-existence lookup, for the exact shape) — if the session no longer exists, call `CancelGuidanceRequest`. This closes the gap architecture-review.md's Concern 1 flagged: without this, a `session`-scoped request whose owning session is torn down while pending sits `pending` forever with no automated reconciliation path (the answer would have nowhere to be delivered anyway once the session is gone).
  - **`standalone` scope**: deliberately NOT swept — there is no owning entity to check liveness against by design (a standalone request is a human-only surface with no session/item to tear down). This is a stated choice, not an omission.
  - **Notify retry (all scopes)**: for each answered-but-not-yet-notified row (`answered_at` set, `notified_at` nil — meaning a prior `MarkGuidanceRequestNotified`/delivery attempt failed; see Task 3.2.1a/3.2.1c, which now marks `notified_at` on the happy path, so this condition genuinely means "delivery was attempted and failed" rather than firing on every answered row), retry the notify/delivery path.
- Files: `session/backlog_lifecycle.go` (or confirmed equivalent), new function in `session/backlog_lifecycle_guidance.go` if the existing file is already large (check line count before deciding — matches this codebase's existing file-splitting convention, e.g. `backlog_lifecycle_triage.go` is already split out).

##### Task 8.1.1b: Test the archived-item, deleted-item, and torn-down-session cancellation paths (~7 min)
- Extend the sweep's test file with 3 cases: (1) archived item with an open `GuidanceRequest` → asserts `cancelled_at` gets set on the next tick; (2) hard-deleted item (via `DeleteBacklogItem`, `GetBacklogItem` now returns `ErrNotFound`) with an open `GuidanceRequest` → asserts `cancelled_at` gets set (this is the case that would previously have been impossible to test, since the row would have been cascade-deleted before the sweep ever ran); (3) `session`-scoped `GuidanceRequest` whose owning session no longer exists → asserts `cancelled_at` gets set. Also assert `standalone`-scoped open requests are left untouched by the sweep (deliberate, per Task 8.1.1a).
- Files: matching `_test.go` for Task 8.1.1a's file.

---

## Acceptance Criteria Coverage Summary

Verified against each story's own self-declared `**Acceptance Criteria** (ACx...)` tag in this file, as of the pre-mortem P1 repair pass that added Stories 3.2.4 and 5.1.4.

| AC | Covered By |
|----|-----------|
| AC1 (core create/durable read-back) | Stories 1.1.1, 1.1.2, 2.1.1, 2.2.1, 3.2.2 |
| AC2 (durable notification to originating session) | Stories 3.1.1, 3.2.1, 3.2.3, 3.2.4, 7.1.1 |
| AC3 (triage halts on ambiguity, feature-flagged) | Stories 4.1.1, 5.1.1, 5.1.2, 5.1.3, 5.1.4 |
| AC4 (one shared component, 3 views) | Stories 6.1.1, 6.1.3 |
| AC5 (unique-index + status-guard dedup/double-answer) | Stories 1.1.1, 1.1.3, 1.1.4, 8.1.1 |
| AC6 (ownership check reusing `resolveItemLink`) | Stories 2.1.2, 2.2.1 |
| AC7 (per-scope pending cap, visible rejection) | Stories 1.1.3, 1.1.4, 2.1.2, 6.1.2 |
