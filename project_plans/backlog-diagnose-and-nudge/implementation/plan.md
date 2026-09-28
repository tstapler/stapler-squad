# Implementation Plan: backlog-diagnose-and-nudge

**Feature**: Diagnose stuck backlog items via a dispatched headless agent that reads a
bounded context bundle and autonomously files a bug, nudges the stuck session, or
posts an inconclusive note — with hard identity/idle/cap safety gates on the nudge
write path and full outcome visibility in the web UI.
**Date**: 2026-09-27
**Status**: Ready for implementation
**ADRs**:
- `../decisions/ADR-001-extend-superseded-session-sweeper.md`
- `../decisions/ADR-002-dual-identity-reverification-before-nudge-write.md`
- `../decisions/ADR-003-nudge-cap-cooldown-shape.md`

---

## Creative Pass (Step 0.5): Alternatives Explored

**Approach A — Fork the existing reconciler shape, insert gate checks at the existing MCP write call sites.**
A new `server/services/diagnose_dispatcher.go` forks
`backlog_lifecycle_triage.go`'s `reconcileOrphanedTriageItems`/
`retryOrphanedTriageWithBackoffGate` loop shape; the 5-part safety gate (Mandatory
Design Decision #5) is a small ordered pipeline of functions inserted directly into
`writeToSession`/`steerSession`/`resumeSession` in `server/mcp/tools_terminal.go` /
`tools_lifecycle.go`.
*Strength*: matches existing, reviewer-familiar reconciler and MCP-handler idioms
almost exactly — minimal new abstraction, lowest regression risk to already-shipped
code.
*Weakness*: a future fourth write-capable tool could still skip the gate; nothing
structurally prevents it beyond code review and the narrow lint ratchet this plan adds
(Phase 4, Epic 4.2).

**Approach B — Single `NudgeGateway` chokepoint; refactor all three MCP write tools to funnel through it.**
Refactor `resume_session`/`steer_session`/`write_to_session` so their handlers become
thin wrappers over one `NudgeGateway.Write(ctx, target, payload)` method that owns all
five gate checks internally — structurally impossible to bypass for any current or
future write path.
*Strength*: the strongest structural guarantee against a bypassed gate — one chokepoint,
not three call sites to remember.
*Weakness*: requires refactoring three already-shipped, already-tested MCP tools used
by every other feature that steers a session (not just this one) — high regression
risk to stable code for a feature whose actual novelty is the new diagnostic-agent
logic, not the write plumbing.

**Approach C — Fully async nudge-request queue; diagnostic agent posts a row, a separate worker performs the gated write.**
The diagnostic agent posts a "nudge request" row to a new job table; a decoupled
background worker polls it and performs the gate checks + write, independent of the
MCP request/response cycle.
*Strength*: a durable queue gives retry/audit semantics almost for free, and cleanly
separates the read-heavy diagnose path from the write-heavy nudge path.
*Weakness*: introduces new async-worker/queue infrastructure for a single-user,
low-traffic internal tool, and **widens**, rather than closes, the TOCTOU window
between "diagnostic agent decided to nudge" and "the write actually happens" —
exactly the race ADR-002 exists to minimize.

**Chosen: Approach A**, with Approach B's chokepoint insight adopted only at the
*function* level (the `verifyIdentityImmediatelyBeforeWrite` facade, ADR-002, which
bundles both identity checks into one un-splittable call) rather than a full handler
refactor, and with the narrow lint ratchet (Phase 4, Epic 4.2 — resolving Mandatory
Design Decision #11b) as the structural backstop against a future bypassed gate,
instead of Approach B's full chokepoint refactor. Approach C's queue infrastructure is
rejected as disproportionate to this feature's single-user, low-traffic scale (see
`research/build-vs-buy.md`) and because it does not actually shrink the identity-race
window. See the Pattern Decisions table below for the per-component versions of this
choice, including "Alternative Rejected"/"Reason" for B and C.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `DiagnosticBundle` | The assembled, budget-capped evidence package (item data, AC, history, verdicts, session snapshot, logs, diff, linked-session transcript excerpt) handed to a dispatched diagnostic agent. | New type, `session/diagnose/bundle.go`. |
| `DiagnosticBundleAssembler` | The service that builds a `DiagnosticBundle` for one backlog item + its linked session(s). | Transaction-Script-style; see Pattern Decisions. |
| `DiagnosticBundleConfig` | Per-section byte budgets (derived from the `~4 bytes/token` heuristic) plus the overall ceiling; mirrors `HandoffSummaryConfig`'s shape. | `session/diagnose/bundle_config.go`. |
| `BundleSection` | A closed enum identifying one budgeted slice of the bundle: `Description`, `AcceptanceCriteria`, `History`, `PriorVerdicts`, `SessionSnapshot`, `Logs`, `Diff`, `LinkedTranscript`. | Drives per-section truncation, not one flat cap. |
| `SectionBudget` | A value object pairing a `BundleSection` with its byte ceiling. | Immutable; `DiagnosticBundleConfig.Sections() []SectionBudget`. |
| `NudgeGate` | The ordered pipeline of gate-check functions a nudge write must pass before executing. | `session/diagnose/nudge_gate.go`. |
| `NudgeGateCheck` | A function type `func(ctx, GateInput) (bool, SafetyGateReason)` — one link in the `NudgeGate` pipeline. | Named per gate: flag, idle, identity, cap. |
| `DiagnoseOutcome` | The closed sum type describing what a diagnose dispatch ultimately did: `Nudged`, `BugFiled`, `InconclusiveNoteFiled`, `SkippedSafetyGate`, `DispatchFailed`. Also carries `WriteAttempted *bool`, non-nil only when a write call was attempted but its outcome could not be confirmed (see Story 4.1.4). | Discriminated struct with `DiagnoseOutcomeKind`; see Pattern Decisions. |
| `DiagnoseOutcomeKind` | The closed enum discriminating `DiagnoseOutcome` variants — 5 values, all *post-completion*. Does **not** represent the pre-completion "Diagnosing…" state (see `DiagnoseDispatchStatus`) or the live, non-persisted "Nudging disabled" state (read directly from `DiagnoseNudgeExecutionFeatureFlag`, never stored). | Exhaustive-switch-checked (Go vet / lint). |
| `DiagnoseDispatchStatus` | The lifecycle enum on the `DiagnoseDispatch` row itself: `Pending` (dispatch started, no outcome yet) or `Completed` (an outcome has been recorded). Distinct from, and orthogonal to, `DiagnoseOutcomeKind` — a `Pending` row has no `OutcomeKind` yet. Backs the durable "Diagnosing (in-flight)" UI state so it survives navigation/refresh per `research/ux.md`. | Mirrors `HandoffSummaryStatus`'s `pending → generating → {ready, error}` pattern; `session/ent/schema/diagnosedispatch.go`. |
| `SafetyGateReason` | The closed enum naming *which* gate failed: `NotIdle`, `IdentityMismatchInstance`, `IdentityMismatchTmuxMarker`, `NudgeCapReached`, `NudgeCooldownActive`, `NudgeExecutionDisabled`, `DuplicateWriteAttemptForDispatch`. | Drives the UI's "name the specific gate" requirement. |
| `SessionIdentity` | Immutable value object `{SessionUUID, TmuxOwnerMarker}` used by both reverification checks. | `session/tmux/write_gate_ownership.go`. |
| `verifyIdentityImmediatelyBeforeWrite` | The facade function performing the `Snapshot()` check and the tmux-pane-marker check back-to-back, with no I/O between, as the literal last step before a nudge write. | See ADR-002. |
| `readSessionOwnerMarker` | This feature's own tmux-pane marker read-back (distinct from the unmerged `ce71ad1a` branch's `ReadSessionOwnerUUID`). | New, `session/tmux/write_gate_ownership.go`. |
| `NudgeCapRecord` | The durable ent-backed row tracking `NudgeCount`/`WindowStartAt`/`LastNudgeAt` per backlog item. | New ent schema; see ADR-003. |
| `NudgeCapStore` | The narrow repository interface (`CheckAndReserve`, `Get`) over `NudgeCapRecord`, mirroring `supersededSessionStore`'s narrow-interface style. | `server/services/nudge_cap_store.go`. |
| `diagnoseNudgeGuardMu` | The package-level `sync.Mutex` serializing check-then-reserve across concurrent dispatchers, mirroring `julesSpendGuardMu`. | `server/services/nudge_cap_store.go`. |
| `DiagnoseDispatcher` | The orchestrator service, forked in shape from `reconcileOrphanedTriageItems`/`retryOrphanedTriageWithBackoffGate`, that assembles a bundle, dispatches the diagnostic agent session, and records the outcome. | `server/services/diagnose_dispatcher.go`. |
| `RequestDiagnosis` | The entry method (`DiagnoseDispatcher.RequestDiagnosis(ctx, itemID)`) that both the RPC handler and (future) a reconciler call; owns the per-item concurrency guard. | Mirrors `steerInFlight.LoadOrStore` idiom. |
| `DiagnoseBacklogItem` | The new ConnectRPC RPC exposing `RequestDiagnosis` to the web UI. | `proto/session/v1/session.proto`. |
| `ListDiagnoseDispatches` | The new ConnectRPC RPC returning an item's diagnose-dispatch history (durable, chronological, survives navigation). | `proto/session/v1/session.proto`. |
| `DiagnoseDispatch` (ent) | The durable row for one dispatch, persisting both its lifecycle (`Status`: `Pending`/`Completed`) and, once completed, its outcome (kind, gate reason, bug link, note text, `WriteAttempted`, timestamps) for `ListDiagnoseDispatches`. The row is created `Pending` at dispatch start (Story 5.1.1) and updated to `Completed` at outcome time (Story 5.2.2) — never created only after the fact. | New ent schema. |
| `WriteAttemptedAt` | Nullable timestamp field on the `DiagnoseDispatch` row, set the moment *any* write is attempted for that dispatch — checked-and-set atomically immediately before the underlying write call, independent of the `WriteAttempted *bool` outcome-classification field above (which is set once, at completion, by Story 5.2.2d). Added alongside, not in place of, `WriteAttempted`: the two have different timing (pre-write live gate vs. post-completion audit) and different consumers. Backs the dispatch-level duplicate-write guard (Story 4.1.4, closing the adversarial re-review's residual CONCERNS finding). | `session/ent/schema/diagnosedispatch.go`; checked via `DiagnoseDispatchStore.CheckAndSetWriteAttempted`. |
| `HeadlessDiagnosticSessionIDPrefix` | The constant `"headless-diagnose-"`, a session-ID prefix that slots into `sessionKind.ts`'s existing `startsWith("headless-")` check with zero new classification code. | `session/diagnose/dispatch.go` (Go) mirrored as a literal in `sessionKind.ts` tests only — no new TS logic. |
| `handoffThenCleanup` | The poll-loop helper: `BeginGeneration` → `GenerateAndPersist` → poll `FindRowBySessionID` until `ready`/`error`/caller-timeout → archive (or the documented `error` fallback). | `server/services/diagnose_stale_session_cleanup.go`. |
| `findIdleStaleSessions` | The new sweep predicate (distinct from `findSupersededSessions`) identifying sessions that are idle-and-stalled with no newer round, layered on the same archival primitives. | `server/services/superseded_session_sweeper.go` (sibling function) or adjacent file. |
| `DiagnoseNudgeExecutionFeatureFlag` | The `config.Config.FeatureFlags` key gating the nudge *write call itself*, default OFF. | `config/config.go`, following `TriageGuidanceHaltFeatureFlag`'s shape. |
| `config.DiagnoseNudgeConfig` | The config block holding `MaxNudgesPerItem`, `CooldownSeconds`, `IdleSettleWindowSeconds`, `BundleTokenBudget`. | `config/config.go`, following `AutonomousMaxTurns*`'s const-pair + `OrDefault()` shape. |
| `notifyDiagnoseEvent` | The two-part durable-write-plus-live-publish notifier for every dispatch/nudge/bug-filed/cap-hit event, extending `EventBusNotifier`. | `server/services/backlog_notifier.go` (extended), `backlog_service_triage.go`'s `notifyReworkCapHit` pattern. |
| `onDiagnose` | The new prop-callback (`(itemId: string) => Promise<void>`) added to `StuckItemDetail.tsx` and `BacklogItemDetail.tsx`, following the `onApprovePlan` convention. | Parent owns the ConnectRPC call. |
| `DiagnoseOutcomeDisplay` | The new React component rendering one of the seven outcome states, reusing `GateVerdictBox`/`TriageReviewPanel` (`readOnly`). | `web-app/src/components/backlog/detail/DiagnoseOutcomeDisplay.tsx`. |
| `DiagnoseHistoryList` | The new component rendering the chronological dispatch history (not just latest), sourced from `ListDiagnoseDispatches`. | `web-app/src/components/backlog/detail/DiagnoseHistoryList.tsx`. |

Glossary term count: **32**.

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Overall architecture | Fork existing reconciler shape + gate checks inserted at existing MCP write call sites (Approach A) | Creative pass, MDD #10 | Approach B: single `NudgeGateway` chokepoint refactor of all 3 MCP tools | Refactoring 3 already-shipped, widely-depended-on MCP handlers carries regression risk disproportionate to this feature's actual novelty (the diagnostic-agent logic, not the write plumbing). |
| Overall architecture | (same as above) | Creative pass | Approach C: async nudge-request queue + worker | Adds queue infrastructure disproportionate to a single-user, low-traffic tool, and widens rather than shrinks the identity-race window ADR-002 targets. |
| Bundle assembly | Transaction Script (PoEAA) | `research/build-vs-buy.md`, `session/backlog_context.go`'s existing procedural style | Domain Model (rich `DiagnosticBundle` object with assembly behavior) | No recurring business rule complexity that would benefit from a domain model; matches the existing procedural style of `BuildTokenBudgetedPrompt`, the direct precedent being generalized. |
| Nudge safety gate | Chain of Responsibility / ordered pipeline of `NudgeGateCheck` functions | GoF (Chain of Responsibility), `isSafeSteerStatus`/`IsReadyForSteer` precedent | Single monolithic boolean function (`isNudgeSafe(...) bool`) | A monolithic function can't report *which* gate failed, breaking the UX requirement to "name the specific gate" (skipped-safety-gate outcome). |
| Nudge cap/cooldown storage | Repository (PoEAA) | `supersededSessionStore`'s narrow-interface precedent | Unit of Work | No multi-row transactional consistency is needed beyond one row's guarded read-modify-write (ADR-003); a full UoW is unwarranted machinery. |
| Dispatch orchestration | Fork of `reconcileOrphanedTriageItems`/`retryOrphanedTriageWithBackoffGate`'s Transaction-Script + backoff-gate shape | MDD #10, `research/build-vs-buy.md` | Build directly on `AutonomousDriver` | `AutonomousDriver` drives a session's own internal polling loop; this feature's nudge is a one-shot, externally-triggered write into a *different* session — a different loop shape (`research/stack.md` §3). |
| Diagnose outcome representation | Sum type: discriminated struct + closed `DiagnoseOutcomeKind` enum, exhaustive-switch-checked | Type-driven design | Independent boolean flags (`bugFiled bool`, `nudged bool`, `inconclusive bool`) | Booleans allow illegal states (e.g., two flags true at once) — exactly the primitive-obsession/illegal-state problem type-driven design exists to close. |
| Identity reverification | Value Object (`SessionIdentity`) + Facade (`verifyIdentityImmediatelyBeforeWrite`) | Type-driven design, GoF (Facade) | Two separate exported check functions called independently at each write site | A facade structurally prevents a future edit from inserting I/O between the two checks (ADR-002's "no I/O between" requirement); two free functions rely on convention alone. |
| Nudge-execution feature flag | Direct reuse of `config.Config.FeatureFlags`/`GetFeatureFlagWithDefault` | `research/stack.md` §4 | New dedicated flag storage/table | Duplicates an existing, already-live-settable mechanism for no benefit; violates the "rollout flags: live-settable, no env vars" project memory by inventing a second mechanism. |
| Stale-session cleanup | Extend `SupersededSessionSweeper` with a sibling predicate | ADR-001 | New parallel reconciler with its own "which session is current" logic | Reproduces the exact "ninth bypass" shape `research/pitfalls.md` §3 warns against. |
| Cross-session transcript compaction | Direct reuse of `HandoffSummaryGenerator` for the linked-session transcript sub-piece only | MDD #2, `research/stack.md` §2 | Building a second bespoke compaction engine for the whole bundle | Explicitly rejected by requirements.md's Out of Scope and `research/build-vs-buy.md`; `HandoffSummaryGenerator`'s API is transcript-shaped, not bundle-shaped, so it's correct for only that one sub-piece. |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| `superseded-rework-session-retirement`'s 8 spawn-site bypasses (still open, `research/features.md` §1) | Bypasses are covered only by `SupersededSessionSweeper`'s defense-in-depth backstop, not fixed at spawn time. | **Isolate via seam.** | This feature's cleanup path routes through the same sweeper primitives (ADR-001) rather than touching the 8 bypass call sites — it neither fixes nor deepens that pre-existing gap; fixing the 8 bypasses at spawn time is explicitly a separate, larger change per the prior plan's own deferral. |
| MCP write path (`writeToSession`/`steerSession`/`resumeSession`) has zero ownership verification today | Confirmed by direct code read (`research/architecture.md` §1.5); the `ce71ad1a` fix doesn't cover this layer even once merged. | **Refactor-first.** | This feature's nudge write cannot ship safely without it — Phase 3 (identity reverification) is sequenced *before* Phase 5 (dispatch orchestration), which depends on it. This is the one item in this table that is genuinely fixed, not just isolated, for the three known call sites — but not eliminated structurally: per the Creative Pass (Approach A's stated weakness, above), a future fourth write-capable tool could still skip the gate; nothing but code review and the narrow lint ratchet/rules-file guardrail (Phase 4, Epic 4.2, see the row below) backstops that residual case. |
| `detection.StatusReady` ambiguity (still referenced in 5+ call sites despite being flagged dead in project memory) | `session/session_driver.go:625`'s own comment distinguishes `StatusReady` (catch-all) from `StatusIdle` (precise). | **Isolate via seam.** | The new idle gate keys on `isSafeSteerStatus` (itself bare `StatusIdle` + a context allowlist, never the `StatusReady`-including union) and does not touch or attempt to resolve `StatusReady`'s broader ambiguity elsewhere in the detector. |
| Four near-duplicate `findInstance(sessionID)`-style lookups across `tools_vcs.go`, `tools_backlog.go`, `tools_goal.go`, `tools_terminal.go` | Primitive-obsession-adjacent smell noted in `research/architecture.md` §1.5, not this feature's job to fully fix. | **Extend as-is.** | The nudge gate hooks into the write call sites' *existing* `terminalHandlers.findInstance` (`tools_terminal.go:734`) rather than adding a fifth near-duplicate lookup. Filed as a follow-on backlog item (Phase 4, Story 4.1.1's task list) for a future consolidation pass — out of this feature's scope to fix now. |
| No lint ratchet enforces `IsArchived()` consultation on new automated-lifecycle paths (`superseded-rework-session-retirement` follow-up #1, still open) | "A seventh auto-lifecycle path can be added tomorrow with no archived check and nothing fails." | **Refactor-first (narrow scope).** | This feature is explicitly the next new automated-lifecycle path research warned about (MDD #11b). Rather than a repo-wide retrofit (out of scope), Phase 4 Epic 4.2 adds a narrow lint analyzer scoped only to this feature's new packages (`server/services/diagnose_*.go`, `session/diagnose/*.go`), closing the gap for the one path this feature adds without re-litigating the 8 pre-existing bypasses. **Scope decision Tyler should be aware of**: requirements.md's In-Scope list does not itself ask for new lint tooling — a cheaper alternative (a `.claude/rules/diagnose-nudge-archived-check.md` glob-scoped guardrail, this repo's existing pattern for this exact problem shape, per `instance-lock-free-reads.md`) was considered and rejected in favor of the analyzer because this repo's own precedent (`norawghrequest.md`) pairs a rules file with a real CI-blocking analyzer for gaps that matter structurally, not either/or — a rules file alone only guards edits made through Claude Code, not a CI gate that fails for any author. If Tyler judges the analyzer's build cost disproportionate for this single feature, downgrading to the rules-file-only mitigation is the documented fallback (Epic 4.2's tasks would then be replaced by adding that one rules file). |

---

## Migration Plan

Two new ent schemas are required — this is a real schema change, not omittable:

1. **`NudgeCapRecord`** (`session/ent/schema/nudgecaprecord.go`, hand-written per
   `session/ent/generate.go`'s policy): fields `ItemID` (string, indexed, unique),
   `NudgeCount` (int), `WindowStartAt` (time), `LastNudgeAt` (time, nullable). Backs
   `NudgeCapStore` (ADR-003). Generated output (`session/ent/nudgecaprecord/*.go`) is
   gitignored per this repo's ent policy — only the schema file is committed.
2. **`DiagnoseDispatch`** (`session/ent/schema/diagnosedispatch.go`): fields `ItemID`
   (string, indexed), `TargetSessionUUID` (string), `DiagnosticSessionUUID` (string),
   `Status` (string enum: `Pending`/`Completed` — see `DiagnoseDispatchStatus`,
   written `Pending` at dispatch start per Story 5.1.1's Task 5.1.1d, before the
   diagnostic session even exists, so a mid-dispatch page refresh has a row to read),
   `OutcomeKind` (string enum, nullable until `Status` is `Completed`),
   `SafetyGateReason` (string, nullable), `BugItemID` (string, nullable), `NoteText`
   (string, nullable), `WriteAttempted` (bool, nullable — see Story 4.1.4),
   `WriteAttemptedAt` (time, nullable — added alongside `WriteAttempted`, not
   replacing it; see Story 4.1.4's dispatch-level write-attempt guard, closing the
   adversarial re-review's residual CONCERNS finding),
   `CreatedAt`/`CompletedAt` (time, `CompletedAt` nullable until completion).
   Backs `ListDiagnoseDispatches` and the durable half of `notifyDiagnoseEvent`.
   The table exists at all (rather than structured logs alone) because
   `research/ux.md`'s Kubernetes-Events-style "history, not just latest" requirement
   (§4) needs a queryable, chronological, per-item list for the UI — logs alone
   can't serve a list view in this codebase's existing architecture; see that
   research doc for the full justification, not repeated here.

Both are additive-only (`CREATE TABLE`, no column changes to existing tables) — no
backfill, no down-migration risk beyond dropping the two new tables. Generate via the
required `--feature sql/upsert` flag (`session/ent/generate.go`) and confirm with
`go build ./...`; do **not** commit generated output (Phase 1, Story 1.3 tasks cover
this explicitly).

## Observability Plan

- **Logs** (structured, `log` package, one line per event — never log-only for a
  blocking decision): `diagnose.dispatch.started`, `diagnose.dispatch.completed`
  (`outcome_kind`, `item_id`, `diagnostic_session_uuid`), `diagnose.dispatch.failed`
  (`reason` — e.g. MCP ECONNREFUSED), `diagnose.nudge.attempted`,
  `diagnose.nudge.skipped` (`safety_gate_reason`), `diagnose.nudge.cap_hit`,
  `diagnose.cleanup.handoff_ready`/`diagnose.cleanup.handoff_error_fallback`.
- **Metrics**: none dashboarded per requirements.md's Non-functional Requirements
  ("verified qualitatively, not a dashboarded KPI") — the durable `DiagnoseDispatch`
  table itself is the queryable record (`SELECT outcome_kind, COUNT(*) ...` ad hoc, not
  a standing metric).
- **Alerts**: none new — `notifyDiagnoseEvent`'s live `eventBus.Publish` surfaces every
  event through the existing toast/notification channel Tyler already watches; no
  separate alerting pipeline is warranted for a single-user internal tool.

## Risk Control

- **Feature flag**: `DiagnoseNudgeExecutionFeatureFlag` (`"diagnose_nudge_execution"`),
  default OFF, gates the nudge *write call itself* inside the `NudgeGate` pipeline —
  not just whether a diagnostic session is dispatched. Diagnose/bundle-assembly/
  bug-filing/note-posting are never gated by this flag (always available). Checked
  live, immediately before the write, alongside (not instead of) the identity/idle/cap
  checks — an in-flight diagnostic agent that decided to nudge before the flag flipped
  off is still blocked at the write call site (resolves the crash-loop-storm-class
  concern from `research/pitfalls.md` §5).
- **Rollback procedure**: flip `DiagnoseNudgeExecutionFeatureFlag` to `false` (live,
  via the existing feature-flag RPC/panel) — reverts to diagnose-and-report-only
  (bug/note), with zero code rollback needed. A full code rollback (revert the PR) is
  the fallback only if diagnose/bundle-assembly itself misbehaves, which the flag
  cannot mitigate.
- **Staged rollout**: ship with the flag OFF; Tyler manually clicks Diagnose on a
  handful of known-stuck items (bug-filing/note-posting only) to validate bundle
  quality and outcome-UI correctness before ever flipping the nudge flag on; then flip
  on for a period with `MaxNudgesPerItem` at its conservative default (2) and observe
  the durable `DiagnoseDispatch` history before considering raising it.

## Unresolved Questions

- [ ] Exact default values for `MaxNudgesPerItem` (proposed: 2) and `CooldownSeconds`
      (proposed: 900) have no empirical basis yet (`requirements.md`'s Rabbit Holes)
      — blocks Story 3.3.1 only in the sense that the *shipped* defaults are a
      judgment call, not a blocking unknown; both are live-settable post-ship, so this
      does not block implementation, only final tuning. Owner: Tyler, post-rollout.
- [ ] Whether `IdleSettleWindowSeconds` should equal or exceed `AutonomousDriver`'s
      existing settle window exactly, or intentionally differ (nudge here is a
      one-shot external write, arguably warranting a *longer* settle window than a
      self-driving loop's own internal nudge) — blocks Story 3.2.1's final constant
      choice. Owner: implementer, resolve by matching `AutonomousDriver`'s value
      unless a concrete reason to diverge emerges during Story 3.2.1's testing.
- [ ] Whether `findIdleStaleSessions` (Story 6.1.1) shares `SupersededSessionSweeper`'s
      existing 60s ticker or needs its own cadence, given handoff generation can take
      up to `handoffSummaryTimeout` (60s) itself — blocks Story 6.1.1's ticker wiring.
      Owner: implementer, default to sharing the existing ticker unless the handoff
      step's latency is observed to starve the existing supersede-sweep pass.

## Dependency Visualization

```
Phase 1 (Config + Domain Types)
   |
   +--> Phase 2 (Bundle Assembly) ---------------------+
   |                                                     |
   +--> Phase 3 (Nudge Safety Gate: identity, idle, cap) |
   |         |                                           |
   |         v                                           v
   +--> Phase 4 (MCP Write-Path Gate Integration +       |
   |            narrow lint ratchet)                     |
   |         |                                           |
   |         v                                           v
   |    Phase 5 (Diagnose Dispatch Orchestration) <------+
   |         |
   |         +--> Phase 6 (Stale-Session Cleanup, extends
   |         |             SupersededSessionSweeper per ADR-001)
   |         v
   |    Phase 7 (RPC + Backend API Surface)
   |         |
   |         v
   +--> Phase 8 (Frontend UI + Feature Registry)
```

Phase 3 (safety gate) and Phase 2 (bundle assembly) can proceed in parallel once
Phase 1 lands — neither depends on the other. Phase 5 (dispatch orchestration) is the
integration point requiring both. Phase 6 (cleanup) depends only on Phase 1 (config)
and the existing `SupersededSessionSweeper` — it does not depend on Phase 5, and could
in principle ship independently, but is sequenced after Phase 5 here since it shares
the same review/testing wave. Phase 8 (frontend) depends on Phase 7 (RPC surface)
existing, but its component-reuse tasks (Epic 8.2) can be stubbed against a mock RPC
client earlier if desired.

---

## Phase 1: Domain Foundations & Configuration

### Epic 1.1: Config & Feature Flag
**Goal**: Establish the live-settable configuration surface (cap, cooldown,
idle-settle window, bundle token budget) and the nudge-execution feature flag before
any gate logic is written against them.

#### Story 1.1.1: `config.DiagnoseNudgeConfig` block
**As a** implementer, **I want** a config block with `OrDefault()` accessors for
nudge cap/cooldown/idle-settle-window/bundle-budget, **so that** later gate code has
one canonical, live-settable source of these numbers instead of hardcoded constants.
**Acceptance Criteria**:
- `config.Config.DiagnoseNudge.MaxNudgesPerItemOrDefault()` returns 2 when
  `MaxNudgesPerItem` is unset or non-positive, and clamps to a hard ceiling of 10.
  - *Given* a `config.Config{}` zero value, *When*
    `cfg.DiagnoseNudge.MaxNudgesPerItemOrDefault()` is called, *Then* it returns `2`.
  - *Given* `config.Config{DiagnoseNudge: DiagnoseNudgeConfig{MaxNudgesPerItem: 999}}`,
    *When* `MaxNudgesPerItemOrDefault()` is called, *Then* it returns `10` (clamped).
- `CooldownSecondsOrDefault()` returns `900` when unset or non-positive, and clamps to
  a hard ceiling of `86400` (24h — a cooldown longer than a day defeats the point of
  cooldown-then-retry).
- `IdleSettleWindowSecondsOrDefault()` returns `AutonomousDriver`'s existing
  settle-window value (resolve exact constant in Story 3.2.1; use a `TODO`-free
  placeholder equal to that value here) when unset or non-positive, and clamps to a
  hard ceiling of `3600` (1h — a nudge-safety settle window has no legitimate reason
  to exceed this).
- `BundleTokenBudgetOrDefault()` returns `250000` when unset or non-positive, and
  clamps to a hard ceiling of `500000` (2x the default; a materially larger bundle
  risks the same cost/latency concerns requirements.md's Feasibility Risks flags for
  `HandoffSummaryGenerator`'s own timeout).
**Files**: `config/config.go`, `config/config_test.go`

##### Task 1.1.1a: Define `DiagnoseNudgeConfig` struct + default/ceiling consts (~3 min)
- Add `DiagnoseNudgeConfig struct { MaxNudgesPerItem int `json:"maxNudgesPerItem,omitempty"`; CooldownSeconds int `json:"cooldownSeconds,omitempty"`; IdleSettleWindowSeconds int `json:"idleSettleWindowSeconds,omitempty"`; BundleTokenBudget int `json:"bundleTokenBudget,omitempty"` }` to `config/config.go`, field `DiagnoseNudge DiagnoseNudgeConfig` on `Config`.
- Add `diagnoseNudgeMaxNudgesDefault = 2`, `diagnoseNudgeMaxNudgesHardCeiling = 10`,
  `diagnoseNudgeCooldownSecondsDefault = 900`, `diagnoseNudgeCooldownSecondsHardCeiling = 86400`,
  `diagnoseNudgeIdleSettleWindowSecondsHardCeiling = 3600`,
  `diagnoseNudgeBundleTokenBudgetDefault = 250000`, `diagnoseNudgeBundleTokenBudgetHardCeiling = 500000`
  consts, following `AutonomousMaxTurns*`'s naming. (`IdleSettleWindowSeconds`'s
  default has no standalone const — it equals `AutonomousDriver`'s existing
  settle-window constant, resolved in Story 3.2.1, not redefined here.)
- Files: `config/config.go`

##### Task 1.1.1b: Add `OrDefault()` accessors (~4 min)
- Implement `MaxNudgesPerItemOrDefault`, `CooldownSecondsOrDefault`,
  `IdleSettleWindowSecondsOrDefault`, `BundleTokenBudgetOrDefault` on
  `DiagnoseNudgeConfig`, each clamping `[1, hardCeiling]`/falling back per
  `AutonomousMaxTurnsOrDefault()`'s exact pattern.
- Files: `config/config.go`

##### Task 1.1.1c: Unit tests for defaults, clamping, and non-positive fallback (~4 min)
- Table-driven test covering zero value, valid value, over-ceiling value, negative
  value for each accessor.
- Files: `config/config_test.go`

#### Story 1.1.2: `DiagnoseNudgeExecutionFeatureFlag`
**As a** implementer, **I want** a named feature-flag constant with default-OFF
semantics wired into the existing accessor family, **so that** the nudge write path
has a single, live-settable kill switch consistent with `TriageGuidanceHaltFeatureFlag`.
**Acceptance Criteria**:
- `cfg.GetFeatureFlagWithDefault(DiagnoseNudgeExecutionFeatureFlag, false)` returns
  `false` on a fresh `config.Config{}`.
  - *Given* a fresh `config.Config{}` with no `FeatureFlags` entries, *When*
    `cfg.GetFeatureFlagWithDefault(DiagnoseNudgeExecutionFeatureFlag, false)` is
    called, *Then* it returns `false`.
  - *Given* `cfg.SetFeatureFlag(DiagnoseNudgeExecutionFeatureFlag, true)` has been
    called, *When* the same getter is called again, *Then* it returns `true`.
**Files**: `config/config.go`, `config/config_test.go`

##### Task 1.1.2a: Add the const (~2 min)
- Add `const DiagnoseNudgeExecutionFeatureFlag = "diagnose_nudge_execution"` next to
  `TriageGuidanceHaltFeatureFlag`, with a one-line doc comment naming what it gates
  (the nudge write call, not dispatch start).
- Files: `config/config.go`

##### Task 1.1.2b: Test default-off and live-set round-trip (~3 min)
- Mirror `TestTriageGuidanceHaltFeatureFlag`-style test (find and follow its exact
  shape) for the new const.
- Files: `config/config_test.go`

### Epic 1.2: Domain Types (Sum Types & Value Objects)
**Goal**: Encode the outcome/gate-reason/identity concepts as types before any service
code references them by ad hoc strings or booleans.

#### Story 1.2.1: `DiagnoseOutcome` / `DiagnoseOutcomeKind` / `SafetyGateReason`
**As a** implementer, **I want** a closed, exhaustive-switch-checked outcome type,
**so that** a dispatch's result can never simultaneously claim two outcomes and every
call site handling it is forced to cover all five post-completion outcome states.
**Note on the UI's "seven states" (`research/ux.md`'s outcome table)**: no single
enum represents all seven — they come from three distinct sources, and conflating
them was an error in an earlier draft of this story. `DiagnoseOutcomeKind` (this
story, 5 values) covers only the *post-completion* states: Nudged, Skipped (safety
gate), Bug filed, Inconclusive, Dispatch failed. The 6th state, **Diagnosing
(in-flight)**, is *pre-completion* and comes from `DiagnoseDispatchStatus.Pending`
(new field on the `DiagnoseDispatch` row, persisted at dispatch start — see Story
5.1.1's Task 5.1.1d and the architecture review's Blocker 1) — a `DiagnoseOutcomeKind`
value cannot represent it because no outcome exists yet at that point. The 7th state,
**Nudging disabled (flag off)**, is neither an outcome nor a dispatch-lifecycle state
at all — it's read live from `DiagnoseNudgeExecutionFeatureFlag` and never persisted
on any row, correctly excluded from both enums.
**Acceptance Criteria**:
- `DiagnoseOutcomeKind` has exactly the values `Nudged`, `BugFiled`,
  `InconclusiveNoteFiled`, `SkippedSafetyGate`, `DispatchFailed`; `DiagnoseOutcome` is
  a struct carrying `Kind DiagnoseOutcomeKind` plus kind-specific fields
  (`GateReason *SafetyGateReason`, `BugItemID *string`, `NoteText *string`,
  `FailureReason *string`, `WriteAttempted *bool` — see Story 4.1.4), never a bag of
  independent booleans.
  - *Given* a `DiagnoseOutcome{Kind: SkippedSafetyGate, GateReason: &SafetyGateReasonNudgeCapReached}`,
    *When* code switches exhaustively on `.Kind`, *Then* the `SkippedSafetyGate` case
    is the only one reading `.GateReason`, and a `go vet`/lint exhaustiveness check
    fails the build if a new `DiagnoseOutcomeKind` value is added without a
    corresponding case.
- `SafetyGateReason` has exactly the values `NotIdle`, `IdentityMismatchInstance`,
  `IdentityMismatchTmuxMarker`, `NudgeCapReached`, `NudgeCooldownActive`,
  `NudgeExecutionDisabled`.
**Files**: `session/diagnose/outcome.go`, `session/diagnose/outcome_test.go`

##### Task 1.2.1a: Define `DiagnoseOutcomeKind` and `DiagnoseOutcome` struct (~4 min)
- New package `session/diagnose`; define the enum (typed `string` per this codebase's
  existing `HandoffSummaryStatus`-style enum convention) and the discriminated struct,
  including the `WriteAttempted *bool` field (nil unless a write's outcome was
  ambiguous — Story 4.1.4 sets it).
- Also define `DiagnoseDispatchStatus` (`Pending`, `Completed`) here as the sibling
  lifecycle enum — it lives on the `DiagnoseDispatch` row (Story 5.2.1's ent schema),
  not on `DiagnoseOutcome` itself, since a `Pending` row has no outcome yet.
- Files: `session/diagnose/outcome.go`

##### Task 1.2.1b: Define `SafetyGateReason` enum + `String()` (~3 min)
- Typed-string enum with a `String()` method for log-line formatting.
- Files: `session/diagnose/outcome.go`

##### Task 1.2.1c: Exhaustiveness test + doc comment (~3 min)
- A test that fails to compile (or fails via reflection-based enumeration check) if a
  `DiagnoseOutcomeKind` value is added without a matching case in a canonical switch
  helper (`func (o DiagnoseOutcome) Validate() error`, rejecting kind/field mismatches
  e.g. `Nudged` with a non-nil `GateReason`).
- Files: `session/diagnose/outcome_test.go`

#### Story 1.2.2: `SessionIdentity` value object
**As a** implementer, **I want** an immutable `SessionIdentity{SessionUUID, TmuxOwnerMarker}`
with an `Equals` method, **so that** identity comparison logic lives in one place
instead of being re-derived ad hoc at each nudge call site.
**Acceptance Criteria**:
- `SessionIdentity{SessionUUID: "abc", TmuxOwnerMarker: "abc"}.Matches("abc")` is
  `true`; a mismatched marker or UUID returns `false` with a named reason.
  - *Given* `SessionIdentity{SessionUUID: "s1", TmuxOwnerMarker: "s2"}`, *When*
    `.Matches("s1")` is called, *Then* it returns `(false, SafetyGateReasonIdentityMismatchTmuxMarker)`.
**Files**: `session/tmux/write_gate_ownership.go`, `session/tmux/write_gate_ownership_test.go`

##### Task 1.2.2a: Define `SessionIdentity` struct + `Matches` (~4 min)
- Files: `session/tmux/write_gate_ownership.go`

##### Task 1.2.2b: Unit tests for match/mismatch on each field independently (~3 min)
- Files: `session/tmux/write_gate_ownership_test.go`

#### Story 1.2.3: `DiagnosticBundleConfig` + `SectionBudget`
**As a** implementer, **I want** a config type expressing per-section byte budgets
that sum to the overall ceiling, **so that** bundle assembly (Phase 2) has a single
source of truth for "how much of each section fits."
**Acceptance Criteria**:
- `DefaultDiagnosticBundleConfig(250000)` returns 8 `SectionBudget`s (one per
  `BundleSection`) whose byte ceilings sum to `<= 250000 * 4` bytes.
  - *Given* `DefaultDiagnosticBundleConfig(250000)`, *When* all returned
    `SectionBudget.MaxBytes` values are summed, *Then* the sum is `<= 1000000`.
**Files**: `session/diagnose/bundle_config.go`, `session/diagnose/bundle_config_test.go`

##### Task 1.2.3a: Define `BundleSection` enum + `SectionBudget` struct (~3 min)
- Files: `session/diagnose/bundle_config.go`

##### Task 1.2.3b: Implement `DefaultDiagnosticBundleConfig(tokenBudget int) DiagnosticBundleConfig` with the allocation: Description 8%, AcceptanceCriteria 4%, History 16%, PriorVerdicts 8%, SessionSnapshot 4%, Logs 24%, Diff 16%, LinkedTranscript 20% of the byte ceiling (~5 min)
- Percentages chosen so Logs+Diff+LinkedTranscript (the three variable-size,
  evidence-heavy sections) get 60% of budget, matching `research/build-vs-buy.md`'s
  per-section-budget recommendation over one flat cap.
- Files: `session/diagnose/bundle_config.go`

##### Task 1.2.3c: Unit test: allocation sums correctly, each section non-zero (~3 min)
- Files: `session/diagnose/bundle_config_test.go`

---

## Phase 2: Diagnostic Bundle Assembly

### Epic 2.1: Bundle Assembler Core
**Goal**: Build the static (non-transcript) sections of the `DiagnosticBundle`,
generalizing `BuildTokenBudgetedPrompt`'s heuristic into per-section budgets.

#### Story 2.1.1: Assemble item/AC/history/verdict sections
**As a** diagnostic agent, **I want** the bundle's item-description, AC, history, and
prior-review-verdict sections populated from the backlog item's existing data, **so
that** I have the same evidentiary basis Tyler would gather by hand.
**Acceptance Criteria**:
- For backlog item `e6c2a88e` with 3 prior sessions, the assembled bundle's `History`
  section contains a summary of all 3 `ItemSessionSummary` rows, truncated
  oldest-first if `History`'s section budget is exceeded (mirroring
  `BuildTokenBudgetedPrompt`'s "drop prior sessions" pass, but per-section rather than
  whole-prompt).
  - *Given* backlog item `e6c2a88e` with `ItemSessionSummary` history of 3 sessions
    totaling 40,000 bytes, and a `History` `SectionBudget.MaxBytes` of 30,000, *When*
    `DiagnosticBundleAssembler.assembleHistory` runs, *Then* the returned section
    contains the 2 most recent sessions' summaries and a one-line note stating 1 older
    session was dropped for budget.
**Files**: `session/diagnose/bundle.go`, `session/diagnose/bundle_test.go`

##### Task 2.1.1a: `DiagnosticBundleAssembler` struct + `AssembleDescriptionAndAC` (~5 min)
- Files: `session/diagnose/bundle.go`

##### Task 2.1.1b: `assembleHistory` with oldest-first drop when over budget (~5 min)
- Files: `session/diagnose/bundle.go`

##### Task 2.1.1c: `assemblePriorVerdicts` (reuse `GetRecentReviewVerdictSummaries`) (~4 min)
- Files: `session/diagnose/bundle.go`

##### Task 2.1.1d: Unit tests for each section, including the drop-oldest-first case (~5 min)
- Files: `session/diagnose/bundle_test.go`

#### Story 2.1.2: Session snapshot, logs, and diff sections
**As a** diagnostic agent, **I want** the bundle's session-snapshot, recent-log, and
git-diff sections populated via `Snapshot()` (never a raw field), **so that** the
evidence reflects the target session's actual current state without racing its
mutators.
**Acceptance Criteria**:
- The `SessionSnapshot` section is built from `inst.Snapshot()`, never `inst.Path`/
  `inst.Branch` directly.
  - *Given* an `*Instance` whose `Snapshot()` reports `{Path: "/repo/worktree-3",
    Branch: "backlog/item-e6c2a88e"}`, *When* `assembleSessionSnapshot(inst)` runs,
    *Then* the returned section text contains `"/repo/worktree-3"` and
    `"backlog/item-e6c2a88e"`, sourced only via the `Snapshot()` call (verified by a
    test double that panics if `.Path`/`.Branch` fields are read directly).
- The `Diff` section runs `git diff`/`git log` scoped to the session's worktree path
  (from `Workspace().ActiveDir`, per `.claude/rules/instance-lock-free-reads.md`), not
  `ExistingDir`.
**Files**: `session/diagnose/bundle.go`, `session/diagnose/bundle_test.go`

##### Task 2.1.2a: `assembleSessionSnapshot(inst *session.Instance) BundleSectionContent` via `Snapshot()` (~4 min)
- Files: `session/diagnose/bundle.go`

##### Task 2.1.2b: `assembleRecentLogs` — tail `log.GetConfigDir()`-resolved log file for the item's session window (~5 min)
- Files: `session/diagnose/bundle.go`

##### Task 2.1.2c: `assembleDiff` via go-git against `Workspace().ActiveDir` (per `prefer-go-git-over-subshells` skill — no raw `git` subshell) (~5 min)
- Files: `session/diagnose/bundle.go`

##### Task 2.1.2d: Unit test asserting `Snapshot()`-only access via a field-read-detecting test double (~4 min)
- Files: `session/diagnose/bundle_test.go`

#### Story 2.1.3: Per-section byte-budget enforcement
**As a** implementer, **I want** a single `enforceBudget(section BundleSection, content string, cfg DiagnosticBundleConfig) string` helper, **so that** every section's
truncation logic is consistent instead of each section reinventing its own cutoff.
**Acceptance Criteria**:
- Content exceeding its section's `MaxBytes` is truncated to fit, with a trailing
  marker noting truncation occurred (never a silent, unmarked cut).
  - *Given* `Logs` section content of 500,000 bytes and a `Logs` `SectionBudget.MaxBytes`
    of 240,000, *When* `enforceBudget(BundleSectionLogs, content, cfg)` runs, *Then*
    the returned string is `<= 240,000` bytes and ends with
    `"[... truncated, N bytes omitted for budget ...]"`.
**Files**: `session/diagnose/bundle_budget.go`, `session/diagnose/bundle_budget_test.go`

##### Task 2.1.3a: Implement `enforceBudget` (~4 min)
- Files: `session/diagnose/bundle_budget.go`

##### Task 2.1.3b: Wire `enforceBudget` into every `assemble*` function from Stories 2.1.1/2.1.2 (~5 min)
- Files: `session/diagnose/bundle.go`

##### Task 2.1.3c: Unit tests: under-budget passthrough, over-budget truncation-with-marker (~4 min)
- Files: `session/diagnose/bundle_budget_test.go`

### Epic 2.2: Linked-Session Transcript Compaction
**Goal**: Wire `HandoffSummaryGenerator` into the bundle for the one section it's
actually shaped for — the linked session's own transcript.

#### Story 2.2.1: `HandoffSummaryGenerator` integration for `LinkedTranscript`
**As a** diagnostic agent, **I want** the linked session's transcript compacted via
the existing `HandoffSummaryGenerator` when it exceeds the `LinkedTranscript` section
budget, and included raw otherwise, **so that** the bundle never conflates this
transcript-shaped compaction with the flat byte-truncation the other sections use.
**Acceptance Criteria**:
- When the linked session's transcript is under `LinkedTranscript`'s byte budget, it's
  included verbatim; when over, `BeginGeneration`/`GenerateAndPersist` is invoked and
  the assembler polls `FindRowBySessionID` (bounded by `handoffSummaryTimeout`) before
  substituting the generated summary.
  - *Given* a linked session whose transcript is 300,000 bytes and a
    `LinkedTranscript` budget of 200,000 bytes, *When* `assembleLinkedTranscript` runs,
    *Then* it calls `HandoffSummaryGenerator.BeginGeneration`, polls until the row is
    `ready`, and the bundle's `LinkedTranscript` section contains the generated
    summary text, not the raw transcript.
  - *Given* the same setup but the row resolves to `error` within the timeout, *When*
    `assembleLinkedTranscript` runs, *Then* the section instead contains a one-line
    note: `"Linked session transcript too large to summarize; summary generation
    failed."` and assembly proceeds without blocking the rest of the bundle.
**Files**: `session/diagnose/bundle_transcript.go`, `session/diagnose/bundle_transcript_test.go`

##### Task 2.2.1a: `assembleLinkedTranscript` under-budget verbatim path (~4 min)
- Files: `session/diagnose/bundle_transcript.go`

##### Task 2.2.1b: Over-budget path calling `BeginGeneration`/`GenerateAndPersist` + poll loop (bounded by `handoffSummaryTimeout`, distinct from the Phase 6 handoff-then-cleanup poll) (~5 min)
- Files: `session/diagnose/bundle_transcript.go`

##### Task 2.2.1c: `error`/timeout fallback note text (~3 min)
- Files: `session/diagnose/bundle_transcript.go`

##### Task 2.2.1d: Unit tests for verbatim, ready, and error/timeout paths using a fake `HandoffSummaryGenerator` (~5 min)
- Files: `session/diagnose/bundle_transcript_test.go`

---

## Phase 3: Nudge Safety Gate

### Epic 3.1: Identity Reverification
**Goal**: Implement ADR-002's dual `Snapshot()` + tmux-pane-marker check as a single
un-splittable facade.

#### Story 3.1.1: `readSessionOwnerMarker` — this feature's own tmux marker read-back
**As a** implementer, **I want** a purpose-built tmux-pane marker reader distinct from
the unmerged `ce71ad1a` branch's helper, **so that** the nudge write path gets
ownership verification without depending on that branch merging.
**Acceptance Criteria**:
- `readSessionOwnerMarker(ctx, socket, paneName)` returns the `STAPLER_SESSION_UUID`
  environment marker stamped on the pane, or a non-nil error if the pane is gone or
  the marker is absent — never a default/empty-string success.
  - *Given* a tmux pane named `stapler-e6c2a88e-work` whose `STAPLER_SESSION_UUID` env
    var is `"9f2c...-work"`, *When* `readSessionOwnerMarker(ctx, socket,
    "stapler-e6c2a88e-work")` is called, *Then* it returns `("9f2c...-work", nil)`.
  - *Given* the same pane has been killed and a new, unrelated pane created under the
    same name with a different marker, *When* `readSessionOwnerMarker` is called with
    the original expected UUID, *Then* the caller compares the returned marker and
    detects a mismatch (this task only covers the read; comparison is Story 3.1.3).
**Files**: `session/tmux/write_gate_ownership.go`, `session/tmux/write_gate_ownership_test.go`

##### Task 3.1.1a: Implement `readSessionOwnerMarker` using the same `tmux show-environment`-style read-back mechanism `orphan_sweep.go`/`workspace_peers.go` already use (~5 min)
- Files: `session/tmux/write_gate_ownership.go`

##### Task 3.1.1b: Error path: pane absent, marker absent, malformed marker (~4 min)
- Files: `session/tmux/write_gate_ownership.go`

##### Task 3.1.1c: Unit tests against a fake tmux socket/harness (reuse whatever test harness `orphan_sweep_test.go` already provides) (~5 min)
- Files: `session/tmux/write_gate_ownership_test.go`

#### Story 3.1.2: `verifyIdentityImmediatelyBeforeWrite` facade
**As a** implementer, **I want** one function performing the `Snapshot()` check and
the tmux-marker check back-to-back with no I/O between, **so that** the two checks
can never be accidentally separated by a future edit (ADR-002).
**Acceptance Criteria**:
- The facade returns `(SessionIdentity, nil)` only when both checks agree with the
  expected UUID; any mismatch or read error returns a non-nil error naming which
  check failed, via `SafetyGateReason`.
  - *Given* `inst.Snapshot().SessionUUID == "9f2c...-work"` and
    `readSessionOwnerMarker` returns `"9f2c...-work"`, and `expectedSessionUUID ==
    "9f2c...-work"`, *When* `verifyIdentityImmediatelyBeforeWrite(ctx, inst,
    "9f2c...-work")` is called, *Then* it returns `(SessionIdentity{...}, nil)`.
  - *Given* the same setup but `readSessionOwnerMarker` returns a different UUID,
    *When* the facade is called, *Then* it returns `(SessionIdentity{}, err)` where
    `errors.Is`-unwrapping `err` yields `SafetyGateReasonIdentityMismatchTmuxMarker`.
**Files**: `session/tmux/write_gate_ownership.go`, `session/tmux/write_gate_ownership_test.go`

##### Task 3.1.2a: Implement the facade as a single function body (no helper calls that could be reordered from outside) (~5 min)
- Files: `session/tmux/write_gate_ownership.go`

##### Task 3.1.2b: Unit tests: both-match, Instance-mismatch, marker-mismatch, marker-read-error (~5 min)
- Files: `session/tmux/write_gate_ownership_test.go`

##### Task 3.1.2c: Doc comment citing ADR-002 and the "last two ops before write, no I/O between" invariant (~2 min)
- Files: `session/tmux/write_gate_ownership.go`

### Epic 3.2: Idle Gate
**Goal**: Resolve Mandatory Design Decision #5b — gate on `isSafeSteerStatus`, which is
itself bare `StatusIdle` plus a context allowlist, sustained through an idle-settle
window.

#### Story 3.2.1: Idle-settle-window gate using `isSafeSteerStatus`
**As a** implementer, **I want** the idle gate to require `isSafeSteerStatus` to hold
continuously for `IdleSettleWindowSecondsOrDefault()`, **so that** a session that
flickers idle for one poll (but is actually still working) is not misread as safe to
nudge.
**Acceptance Criteria**:
- A session whose status is `isSafeSteerStatus`-true for the entire settle window
  passes the gate; a session whose status leaves the allowlist at any point during the
  window resets the settle timer.
  - *Given* a target session reporting `detection.StatusIdle` with context
    `claude_readline_prompt` continuously for 45 seconds, and
    `IdleSettleWindowSecondsOrDefault() == 30`, *When* the idle gate is evaluated,
    *Then* it passes.
  - *Given* the same session reports `command_prompt` (not on the
    `safeIdleStatusContexts` allowlist) 5 seconds into the window, *When* the idle
    gate is evaluated at second 35, *Then* it fails with
    `SafetyGateReasonNotIdle`, because the settle timer reset at second 5.
- The gate explicitly does **not** treat `StatusReady`/`StatusSuccess` as idle —
  resolves MDD #5b in favor of the requirements' literal "bare `StatusIdle`" wording,
  narrowed further by `isSafeSteerStatus`'s context allowlist (stricter than either
  bare `StatusIdle` alone or the 3-way `isIdleStatus` union).
**Files**: `session/diagnose/idle_gate.go`, `session/diagnose/idle_gate_test.go`

##### Task 3.2.1a: Extract/reuse `isSafeSteerStatus` and `safeIdleStatusContexts` (defined at `server/services/session_service.go:1035-1057` — verified by direct grep; corrected from an earlier draft's wrong citation of `backlog_service_pr_fix_steer.go:180`, which is actually `isClaudeCodeProgram`, a different helper referenced correctly in Task 3.2.1c) as an importable helper rather than duplicating it (~4 min)
- If `isSafeSteerStatus` is unexported in its current package, add a small exported
  wrapper there rather than copying its logic — avoids the duplication
  `research/features.md` §2 warns is a smell when re-derived instead of reused.
- Files: `server/services/session_service.go` (export wrapper only), `session/diagnose/idle_gate.go`

##### Task 3.2.1b: Implement settle-window state machine (reset-on-leave, pass-on-sustained) (~5 min)
- Files: `session/diagnose/idle_gate.go`

##### Task 3.2.1c: Reuse `isClaudeCodeProgram` guard so a non-Claude-Code program in the pane never passes (~3 min)
- Files: `session/diagnose/idle_gate.go`

##### Task 3.2.1d: Unit tests: sustained-pass, reset-on-leave, non-Claude-Code-program rejection (~5 min)
- Files: `session/diagnose/idle_gate_test.go`

### Epic 3.3: Nudge Cap/Cooldown
**Goal**: Implement ADR-003's durable, mutex-guarded, actually-blocking cap.

#### Story 3.3.1: `NudgeCapRecord` ent schema + `NudgeCapStore`
**As a** implementer, **I want** a durable per-item nudge-count/cooldown row and a
narrow repository over it, **so that** cap state survives restart and is visible as
history in the UI.
**Acceptance Criteria**:
- `NudgeCapStore.Get(ctx, itemID)` returns a zero-value record for an item never
  nudged; `CheckAndReserve(ctx, itemID, cap, cooldown)` atomically increments and
  returns whether the reservation succeeded.
  - *Given* an item `ce71ad1a` with no existing `NudgeCapRecord`, *When*
    `CheckAndReserve(ctx, "ce71ad1a", cap=2, cooldown=900s)` is called, *Then* it
    creates a row with `NudgeCount=1` and returns `(true, nil)`.
  - *Given* that same item now has `NudgeCount=2`, *When* `CheckAndReserve` is called
    again with `cap=2`, *Then* it returns `(false, nil)` without incrementing further.
**Files**: `session/ent/schema/nudgecaprecord.go`, `server/services/nudge_cap_store.go`, `server/services/nudge_cap_store_test.go`

##### Task 3.3.1a: Write ent schema `nudgecaprecord.go` (~4 min)
- Files: `session/ent/schema/nudgecaprecord.go`

##### Task 3.3.1b: Run `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`, confirm `go build ./...` (~3 min)
- Do not commit generated output per this repo's ent policy.
- Files: (generated, not committed)

##### Task 3.3.1c: Implement `NudgeCapStore` interface + ent-backed implementation (~5 min)
- Files: `server/services/nudge_cap_store.go`

##### Task 3.3.1d: Unit tests: fresh item, at-cap item, cooldown-active item (using in-memory sqlite ent client per this repo's test-isolation convention) (~5 min)
- Files: `server/services/nudge_cap_store_test.go`

#### Story 3.3.2: `diagnoseNudgeGuardMu` package-level serialization
**As a** implementer, **I want** every `CheckAndReserve` call serialized through a
package-level mutex, **so that** two concurrent dispatches for the same or different
items cannot both observe a stale under-cap count (the JulesDispatchService race,
`research/pitfalls.md` §1).
**Acceptance Criteria**:
- Two goroutines calling `CheckAndReserve` concurrently for the same item with
  `cap=1` result in exactly one success and one failure, never two successes.
  - *Given* `cap=1` and two goroutines both call `CheckAndReserve(ctx, "item-x", 1,
    900s)` at the same time, *When* both complete, *Then* exactly one returned
    `(true, nil)` and the other returned `(false, nil)`, verified by a
    `-race`-clean test running both concurrently 100 times.
**Files**: `server/services/nudge_cap_store.go`, `server/services/nudge_cap_store_test.go`

##### Task 3.3.2a: Wrap `CheckAndReserve`'s body in `diagnoseNudgeGuardMu.Lock()`/`defer Unlock()` (~3 min)
- Files: `server/services/nudge_cap_store.go`

##### Task 3.3.2b: Concurrency test: N goroutines, cap=1, assert exactly 1 success, run with `-race` (~5 min)
- Files: `server/services/nudge_cap_store_test.go`

#### Story 3.3.3: Cap hit actually blocks, never logs-only
**As a** implementer, **I want** a failed `CheckAndReserve` to translate directly into
a `SafetyGateReasonNudgeCapReached` gate failure that aborts the write, **so that**
the crash-loop-restart-storm's log-only-cap failure mode cannot recur here.
**Acceptance Criteria**:
- The `NudgeGate` pipeline's cap-check link returns `(false,
  SafetyGateReasonNudgeCapReached)` on a failed reservation, and the pipeline's
  contract (tested in Story 3.4.1) guarantees any `false` result aborts the write —
  this story only covers the cap-check link itself.
  - *Given* `CheckAndReserve` returns `(false, nil)` for an at-cap item, *When* the
    cap-check `NudgeGateCheck` runs, *Then* it returns `(false,
    SafetyGateReasonNudgeCapReached)`, never `(true, ...)`.
**Files**: `session/diagnose/nudge_gate.go`, `session/diagnose/nudge_gate_test.go`

##### Task 3.3.3a: Implement the cap-check `NudgeGateCheck` (~3 min)
- Files: `session/diagnose/nudge_gate.go`

##### Task 3.3.3b: Unit test: at-cap → abort, under-cap → proceed (~3 min)
- Files: `session/diagnose/nudge_gate_test.go`

### Epic 3.4: Gate Pipeline Composition
**Goal**: Compose the flag/idle/identity/cap checks into one fail-closed pipeline.

#### Story 3.4.1: `NudgeGate` ordered pipeline with fail-closed contract
**As a** implementer, **I want** an ordered pipeline running flag → idle → identity →
cap checks (identity check last, immediately before the caller's write), **so that**
any single check's failure — or any ambiguity — aborts the write with a named reason,
never a default-allow.
**Acceptance Criteria**:
- If any check returns `false`, the pipeline stops immediately (short-circuits) and
  returns that check's `SafetyGateReason` — later checks are never evaluated,
  including the identity check, preserving its "immediately before write" placement
  guarantee at the *call site* (the gate itself is evaluated by the caller
  immediately before the write, per Epic 4.1).
  - *Given* the flag check fails (`DiagnoseNudgeExecutionFeatureFlag` off), *When*
    `NudgeGate.Evaluate(ctx, input)` runs, *Then* it returns
    `SafetyGateReasonNudgeExecutionDisabled` and does not evaluate the idle, identity,
    or cap checks (verified via a spy that asserts zero calls to the later checks).
  - *Given* all four checks pass, *When* `NudgeGate.Evaluate` runs, *Then* it returns
    `(true, nil)` and the caller proceeds to call
    `verifyIdentityImmediatelyBeforeWrite` one final time as the literal last step
    before the write (per ADR-002 — the pipeline's identity check and this final
    re-check are deliberately the same function, called twice is acceptable since it's
    idempotent and read-only; documented in the doc comment, not a bug).
**Files**: `session/diagnose/nudge_gate.go`, `session/diagnose/nudge_gate_test.go`

##### Task 3.4.1a: Implement `NudgeGate.Evaluate` short-circuiting pipeline (~5 min)
- Files: `session/diagnose/nudge_gate.go`

##### Task 3.4.1b: Unit tests: each single-check failure short-circuits correctly; all-pass proceeds (~5 min)
- Files: `session/diagnose/nudge_gate_test.go`

##### Task 3.4.1c: Doc comment explaining the double-identity-check design (pipeline check + final pre-write re-check) and why it's intentional, not redundant dead code (~3 min)
- Files: `session/diagnose/nudge_gate.go`

---

## Phase 4: MCP Write-Path Gate Integration

### Epic 4.1: Wire Gate Into Existing Call Sites
**Goal**: Insert the `NudgeGate` + final `verifyIdentityImmediatelyBeforeWrite` at the
three existing MCP write handlers, per Approach A (Creative Pass) — no new tool
surface, no handler refactor beyond the gate insertion.

#### Story 4.1.1: Gate insertion in `steerSession`
**As a** diagnostic agent (via MCP), **I want** `steer_session` to run the full gate
before writing, **so that** a nudge issued through this tool is exactly as safe as one
issued through any future dedicated nudge path.
**Acceptance Criteria**:
- `steerSession` calls `NudgeGate.Evaluate` then `verifyIdentityImmediatelyBeforeWrite`
  immediately before `SendKeysWithTimeout`/equivalent; a gate failure returns an MCP
  error result naming the `SafetyGateReason`, never a silent no-op.
  - *Given* `steer_session` is called for session `ce71ad1a-a6a5-485f-8245-c5a502754a8b`
    while `DiagnoseNudgeExecutionFeatureFlag` is off, *When* the handler runs, *Then*
    it returns an MCP tool error result containing `"nudge_execution_disabled"` and
    performs no write.
  - *Given* the flag is on and all gates pass, *When* `steer_session` runs, *Then*
    `verifyIdentityImmediatelyBeforeWrite` is called as the literal last statement
    before the existing write call, with no intervening I/O (verified by a code-level
    review checklist item, not a runtime assertion — Go cannot enforce "no I/O between
    two lines" automatically).
- This story reuses `terminalHandlers.findInstance` (`tools_terminal.go:734`) for
  UUID→`*Instance` resolution — no fifth near-duplicate lookup is added (Tech Debt
  Disposition: Extend as-is).
**Files**: `server/mcp/tools_terminal.go`, `server/mcp/tools_terminal_test.go`

##### Task 4.1.1a: Insert `NudgeGate.Evaluate` call at the top of `steerSession`'s write branch (~4 min)
- Files: `server/mcp/tools_terminal.go`

##### Task 4.1.1b: Insert `verifyIdentityImmediatelyBeforeWrite` as the last statement before the existing write call (~4 min)
- Files: `server/mcp/tools_terminal.go`

##### Task 4.1.1c: Map each `SafetyGateReason` to an MCP tool error result string (~3 min)
- Files: `server/mcp/tools_terminal.go`

##### Task 4.1.1d: Tests: flag-off abort, idle-fail abort, identity-mismatch abort, all-pass write-proceeds (~5 min)
- Files: `server/mcp/tools_terminal_test.go`

##### Task 4.1.1e: File a follow-on backlog item (via `create_backlog_item` or a TODO doc note if MCP is unavailable) for consolidating the 4 near-duplicate `findInstance`-style lookups — out of this feature's scope, tracked not silently dropped (~3 min)
- Files: (backlog item, no repo file)

#### Story 4.1.2: Gate insertion in `writeToSession`
**As a** diagnostic agent (via MCP), **I want** `write_to_session`'s raw PTY write path
gated identically to `steer_session`, **so that** the lower-level tool isn't a
gate-bypassing back door.
**Acceptance Criteria**:
- Same gate-insertion contract as Story 4.1.1, applied to `writeToSession` at
  `server/mcp/tools_terminal.go:277`.
  - *Given* the flag is on, gates pass, *When* `write_to_session` is called for a
    verified-idle, verified-identity-matched session, *Then* the write proceeds and a
    `diagnose.nudge.attempted`/outcome-`Nudged` event is emitted per Phase 5's
    notifier (cross-referenced, not implemented in this story).
**Files**: `server/mcp/tools_terminal.go`, `server/mcp/tools_terminal_test.go`

##### Task 4.1.2a: Insert gate + final identity re-check (mirrors Task 4.1.1a/b) (~4 min)
- Files: `server/mcp/tools_terminal.go`

##### Task 4.1.2b: Tests mirroring Story 4.1.1's (~4 min)
- Files: `server/mcp/tools_terminal_test.go`

#### Story 4.1.3: Gate insertion in `resumeSession`
**As a** diagnostic agent (via MCP), **I want** `resume_session`'s Paused-session
revival path also gated, **so that** resuming a session as a "nudge" carries the same
safety guarantees as steering a live one.
**Acceptance Criteria**:
- `resumeSession` additionally checks `Instance.IsArchived()` before proceeding
  (per MDD #11b and the Tech Debt Disposition's "Refactor-first" entry) — an archived
  session is never resumed by this path, gate result
  `SafetyGateReasonIdentityMismatchInstance` (archived counts as identity-invalid,
  since an archived `Instance` no longer represents a live target).
  - *Given* `resume_session` is called for a session whose `Instance.IsArchived()`
    returns `true`, *When* the handler runs, *Then* it aborts before recreating any
    worktree/tmux state and returns an MCP error naming the mismatch.
**Files**: `server/mcp/tools_lifecycle.go`, `server/mcp/tools_lifecycle_test.go`

##### Task 4.1.3a: Insert `IsArchived()` check + `NudgeGate.Evaluate` at the top of `resumeSession` (~4 min)
- Files: `server/mcp/tools_lifecycle.go`

##### Task 4.1.3b: Tests: archived-session abort, all-pass proceeds (~4 min)
- Files: `server/mcp/tools_lifecycle_test.go`

#### Story 4.1.4: Ambiguous write-outcome handling — never blindly retry (Resolves adversarial-review Blocker 1 and its re-review residual CONCERNS finding)
**As a** Tyler, **I want** an MCP disconnect/timeout during the nudge *write itself*
(not just during dispatch creation, which Story 5.1.3 already covers) to be treated as
"write outcome unknown" — distinct from both "confirmed sent" and "confirmed not
sent" — with no automatic retry, **so that** an ambiguous failure can never silently
deliver two nudges to the same session (the write is fully autonomous, with no human
approval step to catch a double-send).
**Verified ordering (per the adversarial review's request to confirm, not assume,
this)**: Task 4.1.1a inserts `NudgeGate.Evaluate` — whose last-evaluated check is the
cap-check link (`CheckAndReserve`, Story 3.3.3), which *increments* the durable
`NudgeCapRecord` on success — "at the top of `steerSession`'s write branch," i.e.
strictly *before* the underlying write call each handler makes. The same insertion
point applies to `writeToSession` (Task 4.1.2a) and `resumeSession` (Task 4.1.3a). So
the cap reservation for a given nudge attempt is already durably recorded before that
attempt's write is ever issued — an ambiguous write's cap slot is not later refunded,
and any subsequent attempt for the same item (blind retry or a fresh Diagnose dispatch)
is already counted against the same cap/cooldown window. The feared "reservation
happens after the write" ordering does **not** occur in this plan; no change to the
gate pipeline's position relative to the write is needed — only the write-outcome
classification below is new.
**Dispatch-level guard (closes the adversarial re-review's residual CONCERNS
finding)**: the ordering above only *bounds* a same-dispatch retry — at most 2 real
writes ever land per item, per the shipped `MaxNudgesPerItem = 2` default — it does
not *prevent* the realistic single-retry case, since a second write attempt within the
same dispatch still finds cap headroom (`NudgeCount = 1 < cap = 2`) and proceeds. The
re-review (`adversarial-review.md`'s Concern 1) correctly identified that the only
remaining backstop for that specific case was the diagnostic agent's prompt
instruction (Task 5.1.2d) — a soft LLM-compliance constraint, not code-enforced.
Tasks 4.1.4e-h below close this structurally, per the re-review's own recommended
fix: a new `WriteAttemptedAt` timestamp field on the `DiagnoseDispatch` row (already
created and persisted at dispatch start, Task 5.1.1d), checked-and-set atomically
immediately before each write call, rejects any second write attempt for the same
`dispatchID` outright — independent of, and prior to, the item-level cap.
**Acceptance Criteria**:
- When the underlying write operation (the tmux `SendKeysWithTimeout`-equivalent call
  each of `steerSession`/`writeToSession`/`resumeSession` makes, after the gate has
  already passed) returns a connection-shaped or timeout error, the handler classifies
  this as `write outcome unknown` — a case distinct from both a gate failure
  (`SafetyGateReason`, meaning "never attempted") and a confirmed successful write
  (`Nudged`) — and returns a distinct MCP tool result marker
  (`"write_outcome_unknown"`) rather than a plain, generically-retryable error.
  - *Given* the gate has passed and the underlying `SendKeysWithTimeout`-equivalent
    call returns an `ECONNREFUSED`-shaped error, *When* `steerSession` handles this,
    *Then* it returns an MCP tool result containing `"write_outcome_unknown"`, and the
    handler itself performs no internal retry of the write.
- The handler never blindly retries the write itself; retry-avoidance is enforced at
  two independent layers, not just agent-prompt convention: (1) the handler's own
  code path returns immediately on this classification with no retry loop, and (2) the
  dispatched diagnostic agent's own instructions (Story 5.1.2's prompt — see that
  story's Task 5.1.2d, sequenced after this story only because `prompt.go` itself
  isn't created until Phase 5; this story only specifies the required instruction
  content) explicitly forbid it from calling any of
  `resume_session`/`steer_session`/`write_to_session` again for the same target
  session within the same dispatch after seeing `"write_outcome_unknown"` — it must
  instead post an inconclusive note via `post_backlog_update`.
  - *Given* the diagnostic agent receives a `"write_outcome_unknown"` tool result,
    *When* it follows its dispatch instructions, *Then* its next tool call is
    `post_backlog_update` (an inconclusive note referencing the ambiguous write), never
    a second call to any of the three write tools for that session.
- The persisted outcome distinguishes this case from an ordinary "couldn't figure out
  what to do" inconclusive result: `DiagnoseOutcome{Kind: InconclusiveNoteFiled,
  WriteAttempted: &true, NoteText: &"..."}` — `WriteAttempted` is otherwise nil (not
  applicable) for every other outcome kind except when explicitly set `false` for a
  gate failure that's known with certainty never to have attempted a write.
  - *Given* the diagnostic session's final action was `post_backlog_update` following
    a `"write_outcome_unknown"` result, *When* `DiagnoseDispatcher` interprets the
    dispatch's outcome (Story 5.2.2), *Then* it records `DiagnoseOutcome{Kind:
    InconclusiveNoteFiled, WriteAttempted: &true, ...}`, letting the durable history
    (Story 8.2.2) distinguish "we don't know if this nudge landed" from an unrelated
    inconclusive result.
- Explicitly out of scope (proportionality): no request-ID/idempotency-key subsystem
  is added to the MCP write tools themselves — the minimal fix is classify-don't-retry
  plus letting the already-reserved cap/cooldown (verified above) do its job for any
  case where a retry is attempted anyway (e.g. a future careless prompt edit).
- A dispatch-level write-attempt guard rejects a second real write attempt for the
  same `dispatchID` regardless of remaining nudge-cap headroom, independent of and
  prior to the existing cap check.
  - *Given* a dispatch has already recorded a write attempt (regardless of outcome),
    *When* the diagnostic agent's LLM disobeys its no-retry instruction (Task 5.1.2d)
    and calls `steer_session`/`write_to_session`/`resume_session` again for the same
    dispatch, *Then* the gate rejects the second call with
    `SafetyGateReasonDuplicateWriteAttemptForDispatch` before any write occurs, and
    the nudge cap is NOT consulted or consumed for this rejection (it's a
    dispatch-level guard, independent of the item-level cap).
**Files**: `server/mcp/tools_terminal.go`, `server/mcp/tools_lifecycle.go`, `session/diagnose/outcome.go`, `server/mcp/tools_terminal_test.go`, `server/mcp/tools_lifecycle_test.go` (the prompt-instruction wiring itself lands in Story 5.1.2's `session/diagnose/prompt.go`, Task 5.1.2d, per the cross-phase note above), `session/ent/schema/diagnosedispatch.go`, `server/services/diagnose_dispatch_store.go`, `server/services/diagnose_dispatch_store_test.go` (dispatch-level guard, Tasks 4.1.4e-h — see their cross-phase dependency note)

##### Task 4.1.4a: Classify connection/timeout errors from the underlying write call as a distinct `write outcome unknown` case (not a bare error) in all three handlers (`steerSession`, `writeToSession`, `resumeSession`), returning the `"write_outcome_unknown"` MCP tool result marker (~5 min)
- Files: `server/mcp/tools_terminal.go`, `server/mcp/tools_lifecycle.go`

##### Task 4.1.4b: Specify the required never-retry instruction content as a doc comment/constant string on the `"write_outcome_unknown"` marker itself (in the same file as Task 4.1.4a), so Story 5.1.2's Task 5.1.2d has an unambiguous source of truth to embed into the prompt rather than re-deriving the wording (~3 min)
- Files: `server/mcp/tools_terminal.go`

##### Task 4.1.4c: Confirm `WriteAttempted *bool` exists on `DiagnoseOutcome` (added by Story 1.2.1's Task 1.2.1a) — this story only consumes it; the actual wiring of `DiagnoseDispatcher`'s outcome interpretation (setting it to `&true` when the diagnostic session's tool-call record shows a `"write_outcome_unknown"` result) is Phase 5's Task 5.2.2d, since `DiagnoseDispatcher` doesn't exist until that phase (~2 min)
- Files: `session/diagnose/outcome.go` (verification only, no new code expected)

##### Task 4.1.4d: Tests: simulated write-layer connection/timeout error → handler returns `"write_outcome_unknown"`, no internal retry, cap slot remains consumed (not refunded) across all three handlers (~5 min)
- Files: `server/mcp/tools_terminal_test.go`, `server/mcp/tools_lifecycle_test.go`

##### Task 4.1.4e: Add `WriteAttemptedAt *time.Time` (nullable) field to the `DiagnoseDispatch` ent schema, alongside — not replacing — the existing `WriteAttempted *bool` field: `WriteAttemptedAt` is set the moment any write is attempted for this dispatch (before the underlying write call), while `WriteAttempted` remains the post-completion audit flag Task 5.2.2d sets when interpreting the final outcome. **Cross-phase dependency note** (mirrors Task 5.1.1d's precedent): this field lives on the ent schema Story 5.2.1 defines in Phase 5, one phase after this story; despite the phase/story numbering, implement Story 5.2.1's schema change and Task 4.1.4f's store method before wiring the check into the handlers in Task 4.1.4g — the same kind of forward dependency the Dependency Visualization section already tolerates elsewhere (~3 min)
- Files: `session/ent/schema/diagnosedispatch.go`

##### Task 4.1.4f: Add `DiagnoseDispatchStore.CheckAndSetWriteAttempted(ctx, dispatchID) (alreadyAttempted bool, err error)`: atomically reads `WriteAttemptedAt`; if nil, sets it to now and returns `(false, nil)`; if already set, returns `(true, nil)` without modifying it. Serialize the check-then-set through `diagnoseNudgeGuardMu` (Story 3.3.2's existing package-level mutex), reused here keyed by `dispatchID` rather than `itemID` — one guard for every nudge-adjacent check-and-reserve race rather than a second mutex for a structurally identical problem (~4 min)
- Files: `server/services/diagnose_dispatch_store.go`

##### Task 4.1.4g: Add `SafetyGateReasonDuplicateWriteAttemptForDispatch` to the `SafetyGateReason` enum (Story 1.2.1b). In all three handlers (`steerSession`, `writeToSession`, `resumeSession`), immediately before the underlying write call (same insertion point as `verifyIdentityImmediatelyBeforeWrite`, Tasks 4.1.1b/4.1.2a/4.1.3a), resolve the calling session's own UUID (already available to MCP handlers as caller context) to its `DiagnoseDispatch` row via `DiagnosticSessionUUID`, call `CheckAndSetWriteAttempted`, and abort with `SafetyGateReasonDuplicateWriteAttemptForDispatch` if `alreadyAttempted` is true. This check runs outside `NudgeGate.Evaluate` and does not consult or consume the cap — it has no `dispatchID` in its `GateInput`. A caller with no matching `DiagnoseDispatch` row (e.g. Tyler manually steering a session, not a diagnose dispatch) skips this guard entirely — it is additive to, not a replacement for, the existing per-item `NudgeGate` checks every caller still goes through (~5 min)
- Files: `server/mcp/tools_terminal.go`, `server/mcp/tools_lifecycle.go`, `session/diagnose/outcome.go`

##### Task 4.1.4h: Tests: first write attempt for a dispatch sets `WriteAttemptedAt` and proceeds; a second attempt for the same `dispatchID` is rejected with `SafetyGateReasonDuplicateWriteAttemptForDispatch` even though the cap still has headroom; a concurrent-attempt race test (`-race`, mirrors Task 3.3.2b) asserting exactly one of two simultaneous calls for the same `dispatchID` proceeds; a caller with no `DiagnoseDispatch` row is unaffected by this guard (~5 min)
- Files: `server/mcp/tools_terminal_test.go`, `server/mcp/tools_lifecycle_test.go`, `server/services/diagnose_dispatch_store_test.go`

### Epic 4.2: Narrow Lint Ratchet (Resolves MDD #11b)
**Goal**: Close the "next auto-lifecycle path has no `IsArchived()` enforcement" gap
for this feature's own new code, without a repo-wide retrofit.

#### Story 4.2.1: `requirearchivedcheck` analyzer scoped to `diagnose` packages
**As a** implementer, **I want** a narrow static analyzer flagging any function in
`server/services/diagnose_*.go`/`session/diagnose/*.go` that performs a session
archive/kill/revive call without a preceding `IsArchived()` check in the same
function, **so that** this feature's own automated-lifecycle code can't silently
regress the way the 8 prior bypasses did.
**Acceptance Criteria**:
- The analyzer flags a function in a matched file calling
  `ArchiveSessionByUUID`/`KillTmuxPaneOnly`/`ResumeSession`-equivalent without a
  same-function call to `.IsArchived()` on the same `*Instance`; it does not flag any
  file outside the two matched path prefixes (the 8 pre-existing bypasses are
  explicitly out of scope, per Tech Debt Disposition).
  - *Given* a test fixture function in `session/diagnose/fixture_bad.go` that calls
    `ArchiveSessionByUUID` with no `.IsArchived()` check, *When* `make lint-custom`
    (or the analyzer's own test harness) runs, *Then* it reports a finding on that
    function.
  - *Given* an identical pattern in `server/services/superseded_session_sweeper.go`
    (outside the matched prefixes), *When* the analyzer runs, *Then* it reports no
    finding there (explicitly out of scope, matching `norawghrequest`'s precedent of
    function-declaration-scoped exemption).
**Files**: `tools/lint/requirearchivedcheck/analyzer.go`, `tools/lint/requirearchivedcheck/analyzer_test.go`, `.golangci.yml`

##### Task 4.2.1a: Scaffold the analyzer following `tools/lint/norawghrequest`'s structure (package path match + call-site AST walk) (~5 min)
- Files: `tools/lint/requirearchivedcheck/analyzer.go`

##### Task 4.2.1b: Implement the same-function-preceding-check detection (~5 min)
- Files: `tools/lint/requirearchivedcheck/analyzer.go`

##### Task 4.2.1c: Test fixtures: bad (flagged), good (has check, not flagged), out-of-scope-path (not flagged) (~5 min)
- Files: `tools/lint/requirearchivedcheck/analyzer_test.go`

##### Task 4.2.1d: Wire into `.golangci.yml`/`make lint-custom` (~3 min)
- Files: `.golangci.yml`

---

## Phase 5: Diagnose Dispatch Orchestration

### Epic 5.1: Dispatcher Service
**Goal**: Build `DiagnoseDispatcher`, forked in shape from
`reconcileOrphanedTriageItems`/`retryOrphanedTriageWithBackoffGate`.

#### Story 5.1.1: `RequestDiagnosis` entry + per-item concurrency guard
**As a** Tyler (via the UI), **I want** clicking Diagnose to be safe against a
duplicate click, **and** to see a durable "Diagnosing…" state if I navigate away and
back, **so that** two dispatches never race for the same item and an in-flight
dispatch is never invisible to a fresh page load.
**Acceptance Criteria**:
- A second `RequestDiagnosis(ctx, "e6c2a88e")` call while a dispatch for that item is
  already in flight returns an "already diagnosing" result without starting a second
  dispatch, mirroring `steerInFlight.LoadOrStore`.
  - *Given* `RequestDiagnosis(ctx, "e6c2a88e")` is in flight, *When* a second call for
    the same item ID arrives, *Then* it returns immediately with a result indicating
    an in-flight dispatch already exists, and `diagnoseInFlight` still contains
    exactly one entry for `"e6c2a88e"`.
- **(Resolves architecture-review Blocker 1)** `RequestDiagnosis` persists a
  `DiagnoseDispatch` row with `Status: Pending` (no `OutcomeKind` yet) *before* the
  diagnostic agent session is created — not just an in-process `sync.Map` entry — so
  `ListDiagnoseDispatches` has something durable to return for an in-flight dispatch,
  satisfying `research/ux.md`'s "sourced from durable, persisted state... a page
  refresh must show the same outcome as was live moments earlier" requirement for the
  "Diagnosing (in-flight)" state specifically.
  - *Given* `RequestDiagnosis(ctx, "e6c2a88e")` is called and passes the in-flight
    guard, *When* bundle assembly begins (before `DiagnoseDispatcher.dispatch`, Story
    5.1.2, creates the actual session), *Then* a `DiagnoseDispatch{ItemID:
    "e6c2a88e", Status: Pending}` row already exists such that
    `ListDiagnoseDispatches(ctx, "e6c2a88e")` returns it, even if the caller queries
    immediately (simulating a page refresh mid-dispatch) before the diagnostic session
    finishes.
**Files**: `server/services/diagnose_dispatcher.go`, `server/services/diagnose_dispatcher_test.go`

##### Task 5.1.1a: `DiagnoseDispatcher` struct + `diagnoseInFlight sync.Map` guard (~4 min)
- Files: `server/services/diagnose_dispatcher.go`

##### Task 5.1.1b: `RequestDiagnosis` entry method: guard, assemble bundle (Phase 2), call dispatch (Story 5.1.2) (~5 min)
- Files: `server/services/diagnose_dispatcher.go`

##### Task 5.1.1c: Unit test: concurrent duplicate-call rejection (~4 min)
- Files: `server/services/diagnose_dispatcher_test.go`

##### Task 5.1.1d: Persist the `Pending` `DiagnoseDispatch` row via `DiagnoseDispatchStore.Record` immediately after the in-flight guard passes, before calling `dispatch` (Story 5.1.2) (~4 min)
- **Cross-phase dependency note**: this task needs `DiagnoseDispatchStore` and the
  `DiagnoseDispatch`/`DiagnoseDispatchStatus` ent schema, both defined in Story 5.2.1
  — despite the story numbering, implement Story 5.2.1's schema + store *before* this
  task, not after. (Phase numbering elsewhere in this plan already tolerates this kind
  of forward dependency — see the Dependency Visualization section's own notes on
  Phase 6 being sequenced after Phase 5 for review-wave reasons, not a hard need.)
- The returned dispatch ID from this write becomes the row later updated to
  `Completed` by `notifyDiagnoseEvent` (Story 5.2.2) — same row, not a second insert.
- Files: `server/services/diagnose_dispatcher.go`

##### Task 5.1.1e: Unit test: `Pending` row is queryable via `ListByItem` immediately after `RequestDiagnosis` returns, before the diagnostic session completes (simulates a page-refresh race) (~4 min)
- Files: `server/services/diagnose_dispatcher_test.go`

#### Story 5.1.2: Dispatch headless_diagnostic session with bundle prompt
**As a** diagnostic agent, **I want** to be dispatched as a `headless-diagnose-*`
prefixed session carrying the assembled bundle as its initial prompt, **so that** I
appear in the Sessions list as a `headless_diagnostic` Synthetic Session for free
(per `sessionKind.ts`'s existing `startsWith("headless-")` check).
**Acceptance Criteria**:
- The dispatched session's ID begins with `HeadlessDiagnosticSessionIDPrefix`
  (`"headless-diagnose-"`) followed by the item ID and a dispatch UUID.
  - *Given* `RequestDiagnosis(ctx, "e6c2a88e")` succeeds in assembling a bundle, *When*
    the dispatch call is made, *Then* the created session's ID matches
    `"headless-diagnose-e6c2a88e-<uuid>"`, and the initial prompt text equals the
    assembled `DiagnosticBundle`'s rendered form plus an instruction block naming the
    three allowed tools (`create_backlog_item`, `post_backlog_update`,
    `resume_session`/`steer_session`/`write_to_session`), the evidentiary bar for
    filing a bug ("same as `ce71ad1a`"), and the never-blindly-retry-on-ambiguous-write
    instruction (Task 4.1.4b, Phase 4 — added to this same prompt, cross-referenced
    here since Story 5.1.2 owns the prompt file).
**Files**: `server/services/diagnose_dispatcher.go`, `session/diagnose/prompt.go`, `session/diagnose/prompt_test.go`

##### Task 5.1.2a: `session/diagnose/prompt.go`: render `DiagnosticBundle` + instructions into the dispatch prompt (~5 min)
- Files: `session/diagnose/prompt.go`

##### Task 5.1.2b: `DiagnoseDispatcher.dispatch`: create the session with the `headless-diagnose-` prefix, existing headless-session creation primitives (reuse, don't reinvent) (~5 min)
- Files: `server/services/diagnose_dispatcher.go`

##### Task 5.1.2c: Unit test: session ID prefix format, prompt content includes bundle sections and instruction block (~4 min)
- Files: `session/diagnose/prompt_test.go`

##### Task 5.1.2d: Add the never-blindly-retry-on-ambiguous-write instruction (content specified by Phase 4's Task 4.1.4b) to the instruction block: on any `"write_outcome_unknown"` tool result, the agent must not call any write tool again for that session this dispatch, and must instead call `post_backlog_update` (Resolves adversarial-review Blocker 1) (~3 min)
- Files: `session/diagnose/prompt.go`, `session/diagnose/prompt_test.go`

#### Story 5.1.3: MCP disconnect / dispatch-failure handling
**As a** Tyler, **I want** an MCP-unreachable failure during dispatch to surface as a
distinct `DispatchFailed` outcome, **so that** it's visually distinguishable from a
dispatch that started but produced no clear outcome (per `research/features.md` §7.3).
**Acceptance Criteria**:
- A dispatch-creation error (e.g., session-creation RPC returns a connection error)
  produces `DiagnoseOutcome{Kind: DispatchFailed, FailureReason: &"..."}`, persisted
  by updating the same `Pending` `DiagnoseDispatch` row Task 5.1.1d already wrote (to
  `Status: Completed`, `OutcomeKind: DispatchFailed`) — never a second, separate row
  and never a silently-dropped goroutine failure.
  - *Given* the underlying session-creation call returns `ECONNREFUSED`-shaped error,
    *When* `RequestDiagnosis` runs, *Then* it updates the existing `Pending` row to
    `DiagnoseOutcome{Kind: DispatchFailed, FailureReason: &"session dispatch
    unreachable: ..."}`  and does **not** retry automatically (avoids the
    double-dispatch risk `research/pitfalls.md` §4 flags for ambiguous failures).
**Files**: `server/services/diagnose_dispatcher.go`, `server/services/diagnose_dispatcher_test.go`

##### Task 5.1.3a: Wrap the dispatch-creation call, translate error into `DispatchFailed` outcome (~4 min)
- Files: `server/services/diagnose_dispatcher.go`

##### Task 5.1.3b: Unit test: simulated dispatch-creation error → `DispatchFailed`, no retry (~4 min)
- Files: `server/services/diagnose_dispatcher_test.go`

### Epic 5.2: Outcome Persistence & Durable Notification
**Goal**: Persist every outcome and notify via the `notifyReworkCapHit` two-part
pattern.

#### Story 5.2.1: `DiagnoseDispatch` ent schema + repository
**As a** Tyler, **I want** every dispatch's lifecycle and outcome durably recorded,
**so that** the UI's history view — including an in-flight dispatch's "Diagnosing…"
state — survives navigation away and back and process restarts.
**Acceptance Criteria**:
- `DiagnoseDispatchStore.Record(ctx, dispatch)` persists a new row with `Status:
  Pending` and no `OutcomeKind` (used by Task 5.1.1d, at dispatch start);
  `MarkCompleted(ctx, dispatchID, outcome)` updates that same row to `Status:
  Completed` with the given `DiagnoseOutcome` fields and `CompletedAt` set (used by
  Story 5.2.2 and Story 5.1.3, at dispatch end); `ListByItem(ctx, itemID)` returns all
  rows for an item, chronological, oldest-first, each carrying its own `Status`.
  - *Given* three `DiagnoseDispatch` rows recorded for item `e6c2a88e` at times T1 <
    T2 < T3, *When* `ListByItem(ctx, "e6c2a88e")` is called, *Then* it returns them
    in order `[T1, T2, T3]`.
  - *Given* `Record(ctx, dispatch)` created a row with `Status: Pending`, *When*
    `MarkCompleted(ctx, dispatchID, DiagnoseOutcome{Kind: Nudged})` is called, *Then*
    `ListByItem` returns that same row (same `ID`/`CreatedAt`) now showing `Status:
    Completed`, `OutcomeKind: Nudged`, `CompletedAt` set — not a second row.
**Files**: `session/ent/schema/diagnosedispatch.go`, `server/services/diagnose_dispatch_store.go`, `server/services/diagnose_dispatch_store_test.go`

##### Task 5.2.1a: Write ent schema `diagnosedispatch.go`, including the `Status` field (string enum, `DiagnoseDispatchStatus`, default `Pending`) alongside the existing outcome fields (nullable until completion) (~5 min)
- Files: `session/ent/schema/diagnosedispatch.go`

##### Task 5.2.1b: Regenerate ent (`--feature sql/upsert`), confirm `go build ./...` (~3 min)
- Files: (generated, not committed)

##### Task 5.2.1c: Implement `DiagnoseDispatchStore` (`Record` — insert `Pending`; `MarkCompleted` — update to `Completed` + outcome; `ListByItem`) (~5 min)
- Files: `server/services/diagnose_dispatch_store.go`

##### Task 5.2.1d: Unit tests: record-as-pending, mark-completed updates the same row (not a duplicate), chronological list reflecting mixed pending/completed rows (~5 min)
- Files: `server/services/diagnose_dispatch_store_test.go`

#### Story 5.2.2: `notifyDiagnoseEvent` two-part pattern
**As a** Tyler, **I want** every dispatch/nudge/bug-filed/cap-hit event to post both a
durable, `MarkStuck`-equivalent record and a live toast, **so that** I never have to
dig through logs to learn an autonomous action happened (project memory:
"document AI decisions in edge cases").
**Acceptance Criteria**:
- `notifyDiagnoseEvent(ctx, dispatchID, itemID, outcome)` unconditionally calls
  `DiagnoseDispatchStore.MarkCompleted(ctx, dispatchID, outcome)` — updating the
  `Pending` row Task 5.1.1d already created to `Completed`, never inserting a fresh
  row — then publishes via `EventBusNotifier`, using `itemID` as the event's
  `sessionID` field for the coalescing key, mirroring `notifyReworkCapHit` exactly.
  - *Given* `notifyDiagnoseEvent(ctx, dispatchID, "e6c2a88e", DiagnoseOutcome{Kind:
    Nudged})` is called and the live toast publish fails (event bus temporarily
    down), *When* the call completes, *Then* the durable `DiagnoseDispatch` row is
    still updated to `Completed`/`Nudged` (durability is unconditional, independent
    of the live-publish outcome — same guarantee `notifyReworkCapHit` gives).
**Files**: `server/services/backlog_notifier.go`, `server/services/diagnose_dispatcher.go`, `server/services/diagnose_dispatcher_test.go`

##### Task 5.2.2a: Extend `EventBusNotifier` (or add a sibling method) for diagnose events, following `notifyReworkCapHit`'s exact two-part shape, calling `MarkCompleted` (Task 5.2.1c) rather than `Record` (~5 min)
- Files: `server/services/backlog_notifier.go`

##### Task 5.2.2b: Call `notifyDiagnoseEvent(ctx, dispatchID, ...)` from every outcome branch in `DiagnoseDispatcher`, threading through the `dispatchID` returned by Task 5.1.1d's `Record` call so the correct `Pending` row is updated (~4 min)
- Files: `server/services/diagnose_dispatcher.go`

##### Task 5.2.2c: Unit test: durability survives a live-publish failure (~4 min)
- Files: `server/services/diagnose_dispatcher_test.go`

##### Task 5.2.2d: Wire `DiagnoseDispatcher`'s outcome interpretation to set `WriteAttempted: &true` on the `DiagnoseOutcome` passed to `notifyDiagnoseEvent` when the diagnostic session's tool-call record shows a `"write_outcome_unknown"` result occurred (per Phase 4's Story 4.1.4) (~4 min)
- Files: `server/services/diagnose_dispatcher.go`, `server/services/diagnose_dispatcher_test.go`

---

## Phase 6: Stale-Session Cleanup

### Epic 6.1: Extend `SupersededSessionSweeper` Family (per ADR-001)
**Goal**: Add the idle-stale predicate and handoff-then-cleanup poll loop.

#### Story 6.1.1: `findIdleStaleSessions` predicate
**As a** Reconciler, **I want** to identify sessions that are idle-and-stalled with no
newer round to supersede them, **so that** cleanup isn't limited to
`SupersededSessionSweeper`'s existing "a newer round exists" scope.
**Acceptance Criteria**:
- A session idle (via the Story 3.2.1 idle gate's sustained check) for longer than a
  configurable stale threshold, with no newer `ItemSessionSummary` round for the same
  item+role, is returned by `findIdleStaleSessions`; a session with a newer round is
  left to the existing `findSupersededSessions` path instead (no double-handling).
  - *Given* an item with one work-role session, idle-sustained for 2 hours, and no
    other session for that item+role, *When* `findIdleStaleSessions(sessions,
    staleThreshold=1h)` runs, *Then* it returns that session.
  - *Given* the same session but a newer work-role round now exists, *When*
    `findIdleStaleSessions` runs, *Then* it returns nothing for that item+role
    (already covered by `findSupersededSessions`, avoiding double-archival attempts).
**Files**: `server/services/superseded_session_sweeper.go`, `server/services/superseded_session_sweeper_test.go`

##### Task 6.1.1a: Implement `findIdleStaleSessions` sibling to `findSupersededSessions`, sharing the `supersededSessionStore` interface (~5 min)
- Files: `server/services/superseded_session_sweeper.go`

##### Task 6.1.1b: Wire into the existing 60s ticker's sweep tick (per Unresolved Questions — default to sharing the ticker) (~4 min)
- Files: `server/services/superseded_session_sweeper.go`

##### Task 6.1.1c: Unit tests: idle-no-newer-round → returned; idle-with-newer-round → not returned (no double-handling) (~5 min)
- Files: `server/services/superseded_session_sweeper_test.go`

#### Story 6.1.2: `handoffThenCleanup` poll loop + documented fallback
**As a** Reconciler, **I want** to generate a handoff summary before archiving a stale
session, falling back to "archive anyway, log loudly, post a diagnostic note" on
error/timeout, **so that** cleanup never destroys context silently (MDD #3).
**Acceptance Criteria**:
- On `ready`, the session is archived via `ArchiveSessionByUUID` after the summary row
  is confirmed durable; on `error`/timeout, the session is still archived (never left
  running forever), but a loud log line and a `post_backlog_update`-equivalent
  diagnostic note are emitted first.
  - *Given* a stale session whose `HandoffSummaryGenerator.GenerateAndPersist` resolves
    to `ready` within 45 seconds, *When* `handoffThenCleanup` runs, *Then* it calls
    `ArchiveSessionByUUID` only after observing the `ready` row, and never before.
  - *Given* the same session's row instead resolves to `error` (or the poll exceeds
    `handoffSummaryTimeout`), *When* `handoffThenCleanup` runs, *Then* it logs at
    warning level (`diagnose.cleanup.handoff_error_fallback`), posts a note via the
    existing `post_backlog_update` mechanism stating the handoff summary could not be
    generated, and *still* calls `ArchiveSessionByUUID` (never blocks cleanup forever,
    per MDD #3's resolved default).
- Never calls `StopSessionByUUID` (which would delete the worktree) — only
  `ArchiveSessionByUUID`/`KillTmuxPaneOnly`, and only after `IsArchived()`/
  `IsSessionLive` are consulted (Tech Debt Disposition: Refactor-first for the
  ownership check; ADR-001 for the archival primitive reuse).
**Files**: `server/services/diagnose_stale_session_cleanup.go`, `server/services/diagnose_stale_session_cleanup_test.go`

##### Task 6.1.2a: Implement the `BeginGeneration`→dispatch `GenerateAndPersist`→poll `FindRowBySessionID` loop, bounded by `handoffSummaryTimeout` (~5 min)
- Files: `server/services/diagnose_stale_session_cleanup.go`

##### Task 6.1.2b: `ready` path: confirm durable, then `ArchiveSessionByUUID` (~3 min)
- Files: `server/services/diagnose_stale_session_cleanup.go`

##### Task 6.1.2c: `error`/timeout fallback: warning log + diagnostic note + archive-anyway (~4 min)
- Files: `server/services/diagnose_stale_session_cleanup.go`

##### Task 6.1.2d: Unit tests: ready-path ordering, error-path fallback ordering, never `StopSessionByUUID` (assert via a test double that fails if that method is called) (~5 min)
- Files: `server/services/diagnose_stale_session_cleanup_test.go`

#### Story 6.1.3: `IsArchived()` consultation wired into the new cleanup path
**As a** implementer, **I want** `handoffThenCleanup` to call `Instance.IsArchived()`
itself before acting, **so that** this feature's cleanup path doesn't become the
uncaught "seventh auto-lifecycle path" the still-open follow-up warns about — and the
Phase 4 lint analyzer (Epic 4.2) covers this file since it lives under
`server/services/diagnose_*.go`.
**Acceptance Criteria**:
- `handoffThenCleanup` returns immediately, no-op, if `inst.IsArchived()` is already
  `true` when the cleanup attempt begins.
  - *Given* a session whose `Instance.IsArchived()` already returns `true`, *When*
    `handoffThenCleanup` is invoked for it, *Then* it performs no handoff generation
    and no archive call, returning immediately.
**Files**: `server/services/diagnose_stale_session_cleanup.go`, `server/services/diagnose_stale_session_cleanup_test.go`

##### Task 6.1.3a: Add the `IsArchived()` guard as the first statement in `handoffThenCleanup` (~2 min)
- Files: `server/services/diagnose_stale_session_cleanup.go`

##### Task 6.1.3b: Unit test + confirm `make lint-custom`'s new `requirearchivedcheck` analyzer (Epic 4.2) passes on this file (~3 min)
- Files: `server/services/diagnose_stale_session_cleanup_test.go`

---

## Phase 7: RPC + Backend API Surface

### Epic 7.1: Proto + Service Handler
**Goal**: Expose `RequestDiagnosis`/`ListDiagnoseDispatches` to the web UI.

#### Story 7.1.1: `DiagnoseBacklogItem` and `ListDiagnoseDispatches` RPCs
**As a** web UI, **I want** ConnectRPC endpoints for triggering a diagnose dispatch
and listing its history, **so that** `StuckItemDetail`/`BacklogItemDetail` can call
through a typed client instead of ad hoc HTTP.
**Acceptance Criteria**:
- `DiagnoseBacklogItemRequest{item_id: "e6c2a88e"}` returns
  `DiagnoseBacklogItemResponse{dispatch_id, diagnostic_session_id}` or a gRPC error if
  `RequestDiagnosis` rejects (e.g., already-in-flight).
  - *Given* item `e6c2a88e` has no in-flight dispatch, *When*
    `DiagnoseBacklogItem({item_id: "e6c2a88e"})` is called, *Then* it returns
    `{dispatch_id: "<uuid>", diagnostic_session_id: "headless-diagnose-e6c2a88e-<uuid>"}`.
  - *Given* `ListDiagnoseDispatches({item_id: "e6c2a88e"})` is called after 2 prior
    dispatches, *Then* it returns both, chronological, each with `status`
    (`Pending`/`Completed`), `outcome_kind` (empty/unset when `status` is `Pending`),
    `safety_gate_reason` (if applicable), `bug_item_id`/`note_text` (if applicable),
    timestamps.
  - *Given* one of those dispatches is still in flight (`status: Pending`, no
    `outcome_kind` yet), *When* the same RPC is called again (simulating a page
    refresh), *Then* it returns the same `Pending` row unchanged — this is the
    concrete API-level fix for architecture-review Blocker 1.
**Files**: `proto/session/v1/session.proto`, `server/services/backlog_service.go` (or a new `server/services/diagnose_service.go`), `server/services/diagnose_service_test.go`

##### Task 7.1.1a: Add `DiagnoseBacklogItem`/`ListDiagnoseDispatches` RPC + message defs to `session.proto`, including a `status` enum field (`PENDING`/`COMPLETED`) on the dispatch message, distinct from `outcome_kind` (~5 min)
- Files: `proto/session/v1/session.proto`

##### Task 7.1.1b: Run `make proto-gen`, confirm generated stubs compile (~3 min)
- Files: (generated, gitignored per this repo's `gen/` policy)

##### Task 7.1.1c: Implement the handler in a new `server/services/diagnose_service.go`, delegating to `DiagnoseDispatcher`/`DiagnoseDispatchStore` (~5 min)
- Files: `server/services/diagnose_service.go`

##### Task 7.1.1d: Register the handler in `server/server.go` (~2 min)
- Files: `server/server.go`

##### Task 7.1.1e: Handler tests: success, already-in-flight rejection, empty history list (~5 min)
- Files: `server/services/diagnose_service_test.go`

#### Story 7.1.2: Feature registry entry (RPC)
**As a** maintainer, **I want** the new RPC registered in `docs/registry/features/`,
**so that** `make registry-diff` doesn't flag it as missing.
**Acceptance Criteria**:
- `make registry-generate` picks up `// +api: backlog:diagnose` markers on the new
  handler and produces/updates a per-feature JSON file with no manual JSON editing.
  - *Given* the `// +api: backlog:diagnose` marker is added to
    `diagnose_service.go`'s handler functions, *When* `make registry-generate` runs,
    *Then* `docs/registry/features/backlog-diagnose.json` (or equivalent generated
    filename) is created/updated with those RPC entries, and `make registry-diff`
    reports no drift afterward.
**Files**: `server/services/diagnose_service.go`, `docs/registry/features/*.json` (generated)

##### Task 7.1.2a: Add `// +api: backlog:diagnose` markers to the new handler functions (~2 min)
- Files: `server/services/diagnose_service.go`

##### Task 7.1.2b: Run `make registry-generate`, commit the generated/updated per-feature file (~3 min)
- Files: `docs/registry/features/*.json`

---

## Phase 8: Frontend UI

### Epic 8.1: Diagnose Action Wiring
**Goal**: Add the `onDiagnose` prop-callback to both detail components, following the
`onApprovePlan` convention.

#### Story 8.1.1: `onDiagnose` prop on `StuckItemDetail.tsx`
**As a** Tyler, **I want** a "Diagnose" button on a stuck item's detail view, **so
that** I can trigger diagnosis without leaving the page.
**Acceptance Criteria**:
- The button is a real `<button>`, Tab-reachable, `aria-label="Diagnose this stuck
  item"`, disabled + `aria-busy="true"` + label "Diagnosing…" while in flight; calling
  `onDiagnose` rejects on failure and surfaces a `role="alert"` inline error,
  mirroring `onApprovePlan`'s existing error-handling contract exactly.
  - *Given* `StuckItemDetail` is rendered for item `e6c2a88e` with `onDiagnose` bound
    to a function that resolves after 200ms, *When* the user activates the Diagnose
    button via Enter, *Then* the button becomes disabled with `aria-busy="true"` and
    text "Diagnosing…" immediately, and reverts to its normal enabled state once the
    promise resolves.
  - *Given* `onDiagnose` rejects with `"already diagnosing"`, *When* that rejection is
    caught, *Then* a `role="alert"` element renders the message text, matching
    `StuckItemDetail.tsx`'s existing `overrideState === "error"` pattern.
**Files**: `web-app/src/components/backlog-stuck/StuckItemDetail.tsx`, `web-app/src/components/backlog-stuck/StuckItemDetail.test.tsx`

##### Task 8.1.1a: Add `onDiagnose?: (itemId: string) => Promise<void>` prop + JSDoc following the `onApprovePlan` doc-comment convention (~3 min)
- Files: `web-app/src/components/backlog-stuck/StuckItemDetail.tsx`

##### Task 8.1.1b: Add the button + idle/pending/error state machine (mirrors the existing 3-state form pattern) (~5 min)
- Files: `web-app/src/components/backlog-stuck/StuckItemDetail.tsx`

##### Task 8.1.1c: Tests: idle→pending→success, idle→pending→error, keyboard activation (~5 min)
- Files: `web-app/src/components/backlog-stuck/StuckItemDetail.test.tsx`

#### Story 8.1.2: `onDiagnose` prop on `BacklogItemDetail.tsx`
**As a** Tyler, **I want** the same Diagnose action available on any item's detail
view (not just stuck ones), **so that** I can proactively diagnose an item that isn't
yet flagged stuck.
**Acceptance Criteria**:
- Same contract as Story 8.1.1, applied to `BacklogItemDetail.tsx`.
  - *Given* `BacklogItemDetail` is rendered for a non-stuck item `abc123` with
    `onDiagnose` bound, *When* the user clicks Diagnose, *Then* the same
    pending/success/error states render identically to `StuckItemDetail`'s.
**Files**: `web-app/src/components/backlog/BacklogItemDetail.tsx`, `web-app/src/components/backlog/BacklogItemDetail.test.tsx`

##### Task 8.1.2a: Add `onDiagnose` prop + button, reusing the same state-machine logic (extract a shared hook if duplication would otherwise exceed the jscpd 20-line/200-token threshold) (~5 min)
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`, `web-app/src/hooks/useDiagnoseAction.ts` (new, if extraction is warranted)

##### Task 8.1.2b: Tests mirroring Story 8.1.1's (~4 min)
- Files: `web-app/src/components/backlog/BacklogItemDetail.test.tsx`

### Epic 8.2: Outcome Display (Seven States)
**Goal**: Render all seven outcome states distinctly, reusing
`GateVerdictBox`/`TriageReviewPanel` (`readOnly`) and `ActivityLogSection`.

#### Story 8.2.1: `DiagnoseOutcomeDisplay` component
**As a** Tyler, **I want** each outcome rendered with its own copy and icon (never
color-only), **so that** I can tell at a glance what happened without opening the
diagnostic session's transcript.
**Acceptance Criteria**:
- All seven states from `research/ux.md`'s outcome table render distinct copy + icon
  + `data-testid`; `SkippedSafetyGate` names the specific `SafetyGateReason` in its
  copy (e.g., "nudge skipped (session wasn't idle)" vs. "nudge skipped (identity check
  failed)"), never a generic "skipped."
- **(Resolves architecture-review Blocker 1)** The "Diagnosing (in-flight)" state is
  rendered whenever `ListDiagnoseDispatches`' most recent row for the item has
  `status: Pending` — sourced from that durable, persisted field, not only from the
  transient button-pending state Stories 8.1.1/8.1.2 own. A page refresh mid-dispatch
  must still show "Diagnosing…", `aria-busy="true"`, because the fresh
  `ListDiagnoseDispatches` call (not any client-only cache) returns the `Pending` row.
  - *Given* `ListDiagnoseDispatches` returns a most-recent row with `status: Pending`
    for item `e6c2a88e`, *When* `DiagnoseOutcomeDisplay` renders for that item
    immediately after a fresh page load (no button-click state in memory), *Then* it
    shows "Diagnosing…" with `aria-busy="true"`, distinct from the button's own
    transient pending state, which this replaces as the source of truth once a
    dispatch ID exists server-side.
  - *Given* a `DiagnoseDispatch` with `outcome_kind: "SkippedSafetyGate"`,
    `safety_gate_reason: "NudgeCapReached"`, *When* `DiagnoseOutcomeDisplay` renders
    it, *Then* the visible text reads exactly "Diagnosed \<time\> ago — nudge skipped
    (nudge cap reached: 2/2 this window)." per `research/ux.md`'s
    `formatReworkCapOverride`-style "N / cap" convention, with a 🟡 neutral icon (not
    styled as failure).
  - *Given* `outcome_kind: "Nudged"`, *When* rendered, *Then* the text reads
    "Diagnosed \<time\> ago — nudged the session." with a link to the diagnostic
    session record, 🟢 "acted" styling, distinct from any "resolved" implication (per
    `research/ux.md`'s "never a green chip implying the underlying condition is
    resolved").
  - *Given* `DiagnoseNudgeExecutionFeatureFlag` is off, *When* `StuckItemDetail`
    renders proactively (before any dispatch), *Then* it shows the
    "nudging-disabled(flag-off)" informational state stating diagnosis will only file
    a bug or post a note.
**Files**: `web-app/src/components/backlog/detail/DiagnoseOutcomeDisplay.tsx`, `web-app/src/components/backlog/detail/DiagnoseOutcomeDisplay.test.tsx`

##### Task 8.2.1a: Implement the 7-state switch rendering — 5 `DiagnoseOutcomeKind` cases (Nudged/BugFiled/InconclusiveNoteFiled/SkippedSafetyGate/DispatchFailed, reusing `GateVerdictBox`/`TriageReviewPanel` `readOnly` mode) plus the `status: Pending` case ("Diagnosing…", sourced from `ListDiagnoseDispatches`, not client state) — the flag-off case is Task 8.2.1b, kept separate since it's not sourced from any dispatch row at all (~5 min)
- Files: `web-app/src/components/backlog/detail/DiagnoseOutcomeDisplay.tsx`

##### Task 8.2.1b: Implement the flag-off proactive banner (shown before any dispatch, per `research/ux.md` §3 accessibility requirements) (~4 min)
- Files: `web-app/src/components/backlog/detail/DiagnoseOutcomeDisplay.tsx`

##### Task 8.2.1c: `aria-live="polite"` for routine outcomes, `role="alert"`/`aria-live="assertive"` for `DispatchFailed` only (~3 min)
- Files: `web-app/src/components/backlog/detail/DiagnoseOutcomeDisplay.tsx`

##### Task 8.2.1d: Tests: all 7 states render distinct copy+icon+testid; cap-reached shows "N/cap"; flag-off banner shown proactively (~5 min)
- Files: `web-app/src/components/backlog/detail/DiagnoseOutcomeDisplay.test.tsx`

#### Story 8.2.2: History list via `ActivityLogSection`/new `DiagnoseHistoryList`
**As a** Tyler, **I want** to see every past diagnose dispatch for an item, not just
the latest, **so that** a pattern of repeated ineffective nudges is visible (per
`research/ux.md`'s Kubernetes-Events analogy).
**Acceptance Criteria**:
- The history list renders in chronological order, each entry keyboard-navigable
  (`role="list"`/`role="listitem"`, matching `ActivityLogSection`'s existing pattern),
  and survives a page refresh (sourced from `ListDiagnoseDispatches`, not client state).
  - *Given* item `e6c2a88e` has 3 prior dispatches (Nudged, SkippedSafetyGate,
    BugFiled, in that order), *When* the page is refreshed, *Then* all 3 render in the
    same order as before the refresh, sourced from a fresh `ListDiagnoseDispatches`
    call, not from any client-only cache.
  - The `InconclusiveNoteFiled` outcome's note text is additionally surfaced via the
    existing `ActivityLogSection` feed (reused, not a new note-type UI), per MDD #8.
**Files**: `web-app/src/components/backlog/detail/DiagnoseHistoryList.tsx`, `web-app/src/components/backlog/detail/DiagnoseHistoryList.test.tsx`, `web-app/src/components/backlog/detail/ActivityLogSection.tsx`

##### Task 8.2.2a: Implement `DiagnoseHistoryList` fetching via `ListDiagnoseDispatches`, `role="list"` semantics (~5 min)
- Files: `web-app/src/components/backlog/detail/DiagnoseHistoryList.tsx`

##### Task 8.2.2b: Route `InconclusiveNoteFiled` entries into the existing `ActivityLogSection` feed rather than a new component (~4 min)
- Files: `web-app/src/components/backlog/detail/ActivityLogSection.tsx`

##### Task 8.2.2c: Tests: chronological order, refresh-persistence (mock RPC returning the same data twice), keyboard navigation (~5 min)
- Files: `web-app/src/components/backlog/detail/DiagnoseHistoryList.test.tsx`

### Epic 8.3: Synthetic Session Classification (Zero New Code Verification)
**Goal**: Confirm the `headless-diagnose-*` prefix slots into `sessionKind.ts`'s
existing check with no new classification code, per `research/stack.md` §7.

#### Story 8.3.1: Verify + pin `headless-diagnose-*` classification
**As a** Tyler, **I want** a dispatched diagnostic session to automatically appear in
the Sessions list as a `headless_diagnostic` Synthetic Session, **so that** I get this
for free rather than building a parallel UI.
**Acceptance Criteria**:
- `classifySessionKind({ id: "headless-diagnose-e6c2a88e-<uuid>", ... })` returns
  `"headless_diagnostic"` using the existing, unmodified `startsWith("headless-")`
  branch — no new `if`/`case` added to `sessionKind.ts`.
  - *Given* a session object with `id: "headless-diagnose-e6c2a88e-abcd1234"`, *When*
    `classifySessionKind(session)` is called, *Then* it returns `"headless_diagnostic"`,
    and a diff of `sessionKind.ts` for this story shows zero lines changed in the
    classification function itself (only a new test is added).
**Files**: `web-app/src/lib/backlog/sessionKind.test.ts`

##### Task 8.3.1a: Add a regression test pinning `headless-diagnose-*` → `headless_diagnostic` (~3 min)
- Files: `web-app/src/lib/backlog/sessionKind.test.ts`

##### Task 8.3.1b: Confirm no production code changes were needed in `sessionKind.ts`; if the prefix check is more specific than `startsWith("headless-")` (e.g. an exact enum of known prefixes), adjust minimally and document why (~3 min)
- Files: `web-app/src/lib/backlog/sessionKind.ts` (only if Task 8.3.1a's test reveals a gap)

### Epic 8.4: Feature Registry Entry (Component)
**Goal**: Register the new component/prop per `docs/reference/feature-registry.md`.

#### Story 8.4.1: `// +feature:` markers + registry generation
**As a** maintainer, **I want** the new React components marked and registered, **so
that** `make registry-diff` shows no drift.
**Acceptance Criteria**:
- `// +feature: backlog-diagnose-nudge` appears in the first 10 lines of
  `DiagnoseOutcomeDisplay.tsx` and `DiagnoseHistoryList.tsx`; `make registry-generate`
  picks them up with no manual JSON edits.
  - *Given* the markers are added, *When* `make registry-generate` runs, *Then* the
    corresponding `docs/registry/features/*.json` file lists both components, and
    `make registry-diff` reports no drift.
**Files**: `web-app/src/components/backlog/detail/DiagnoseOutcomeDisplay.tsx`, `web-app/src/components/backlog/detail/DiagnoseHistoryList.tsx`, `docs/registry/features/*.json`

##### Task 8.4.1a: Add `// +feature: backlog-diagnose-nudge` markers (~2 min)
- Files: `web-app/src/components/backlog/detail/DiagnoseOutcomeDisplay.tsx`, `web-app/src/components/backlog/detail/DiagnoseHistoryList.tsx`

##### Task 8.4.1b: Run `make registry-generate`, commit the updated per-feature JSON (~3 min)
- Files: `docs/registry/features/*.json`

##### Task 8.4.1c: Run the 7-touchpoint session-creation registry checklist (`docs/reference/session-creation-registry.md`) against the new `headless-diagnose-*` dispatch path and address any missed touchpoint (~5 min)
- Files: per whichever touchpoints the checklist identifies as incomplete
