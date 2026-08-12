# Implementation Plan: backlog-operator-feedback-loop

**Feature**: Close the operator feedback loop — answer triage questions, request plan revisions,
and steer backlog-linked sessions, all from the backlog item detail view.
**Date**: 2026-08-12
**Status**: Ready for implementation
**ADRs**:
- `project_plans/backlog-operator-feedback-loop/decisions/ADR-001-steer-via-widened-update-session.md` (Gap 2)
- `project_plans/backlog-operator-feedback-loop/decisions/ADR-002-steerable-scope-instance-backed-live-sessions.md` (Gap 2)
- `project_plans/plan-approval-ux/decisions/ADR-001-plan-review-state-durable-fields.md` (Gap 3)
- `project_plans/plan-approval-ux/decisions/ADR-002-reject-plan-manual-retrigger.md` (Gap 3)
- No new ADR written by this plan — see §6 "New ADR needed?" below.

---

## 0. Creative Pass — already resolved by research, recorded here

Research (`research/architecture.md`, `research/build-vs-buy.md`, `research/pitfalls.md`)
already ran the alternatives analysis for all three gaps; this plan does not re-derive it. The
one explicit scope call this phase must make and record:

**Gap 3 scope trim — DEFER `GetPlanArtifactContent` (in-browser plan.md rendering) and its
paired `expected_modified_at_unix_ms` optimistic-concurrency token.** Rationale: none of ACs
1–8 require in-browser plan-content rendering — AC3/4/5 only need the approve/reject state
machine and a visible status, which a plain `<code>{item.planArtifactsPath}</code>` (the
existing display, unchanged) already supports. Building a second RPC, a path-traversal-safe
file reader, and a round-tripped mtime token to satisfy zero stated acceptance criteria is the
textbook case the `ponytail`/YAGNI convention exists for. Consequence: the two-tab
concurrent-approve-vs-reject race (`pitfalls.md` §Gap-3 carried-forward risk #4) has no token to
guard against it — this plan explicitly accepts that as an out-of-scope solo-operator race
(pitfalls.md's own option (a)), recorded in Unresolved Questions, not silently dropped.
Consequently this plan also drops `plan_artifacts_set_at` and the widened
`reconcilePlanNotApprovedItems` stuck-detection (bc0955d41's other extra scope) — no AC calls
for escalating a stale `changes_requested` item, and `pitfalls.md`'s Gap-3 reconciler section
explicitly recommends treating that as out of scope for this pass.

Everything else — Gap 1's stateless client-side composition, Gap 2's widen-not-duplicate RPC
decision (ADR-001) and narrowed steerable scope (ADR-002), Gap 3's revive-not-redesign backend
plus fresh frontend — was already decided in research/ADRs and is recorded in the Pattern
Decisions table below with its rejected alternative, not re-litigated here.

---

## 1. System Type

CRUD-adjacent feature wiring existing RPCs/hooks to three new UI surfaces, plus one small,
already-ADR'd state machine (Gap 3's 5-state derived `PlanReviewStatus`, not a new persisted
enum). No new aggregate root, no new bounded context, no new external dependency (confirmed by
`research/build-vs-buy.md` — every sub-feature is CRUD/RPC wiring on top of existing primitives).

---

## 2. Domain Glossary

| Term | Definition | Notes |
|---|---|---|
| `TriageSuggestion` | Existing type `{text, rationale}` — a single triage suggestion; `rationale === "question"` marks a clarifying question. No stable ID field. | Reused as-is (`web-app/src/lib/hooks/useBacklogService.ts:37-40`) — not extended. |
| `composeQuestionAnswerFeedback` | New pure function: given one or more `{questionText, answerText}` pairs, returns the `"Q: <text>\nA: <text>"`-shaped string handed to the existing `feedback` field. | New file `web-app/src/lib/backlog/composeQuestionAnswerFeedback.ts`. The only new Gap-1 abstraction. |
| Answered-question marker | Client-side, session-local (not persisted) visual "✓ answered" state on a question row after a successful submit. Discarded on any new `TriageResult` (new `iteration`). | Local `useState` in `TriageDiffSection`, per `research/ux.md` Gap 1 and requirements.md's "default to stateless" resolution of Open Question 2. |
| `isSteerable` | New predicate: `LinkedSession` is steerable iff `classifySessionKind(session)` is `"work"` or `"review"` AND `!session.endedAt`. | `web-app/src/lib/backlog/sessionKind.ts` — exact shape specified in ADR-002. |
| `classifySessionKind` / `SessionKind` | Existing closed classification (`"work" \| "review" \| "headless_diagnostic" \| "blocked_guardrail" \| "manual_review_marker"`) distinguishing Instance-backed "Real Sessions" from DB-only "Synthetic Sessions". | Reused unchanged (`web-app/src/lib/backlog/sessionKind.ts:9-43`). |
| `steer_message` (widened) | Existing `UpdateSessionRequest.steer_message` field/handler, widened per ADR-001 to fall back to `Instance.SendKeys` for non-autonomous sessions instead of hard-rejecting. No new proto field. | `server/services/session_service.go` (~line 2012-2033 pre-change). |
| `PlanRejectionReason` / `PlanRejectedAt` | New nullable fields on `BacklogItemData`/`BacklogItemUpdate`/ent schema. Reused term from `plan-approval-ux`'s ADR-001 — cleared on `ApprovePlan`, on the next `TriggerTriage` regeneration completion, and on the existing backward-transition reset block. | `session/ent/schema/backlog_item.go`; **`plan_artifacts_set_at` is explicitly NOT ported** (§0 scope trim). |
| `RejectPlan` | New RPC: `item_id` + `reason` (required, non-empty). Persists `plan_rejection_reason`/`plan_rejected_at`, clears `plan_approved`. Does not itself trigger regeneration (ADR-002, plan-approval-ux). **No `expected_modified_at_unix_ms` field** (§0 scope trim — that field belonged to the deferred `GetPlanArtifactContent` pairing). | `proto/session/v1/backlog.proto` message + `BacklogService.RejectPlan`. |
| `PlanReviewStatus` | The 5-state derived status: `no_plan`, `pending_review`, `approved`, `changes_requested`, `skipped`. Never persisted — computed from `planArtifactsPath`, `planApproved`, `planRejectionReason`, `skipPlanning`. Reused verbatim from `plan-approval-ux`'s domain glossary. | TS-only in this pass (no Go-side mirror — same deferral `plan-approval-ux`'s own Unresolved Question 5 already recorded). |
| `derivePlanReviewStatus` | Pure function computing `PlanReviewStatus`. Single source of truth — both the new plan-review UI and `ActionsSection`'s spawn gate call it, per `pitfalls.md`'s "no canonical derivation function" warning. | `web-app/src/lib/backlog/planReviewStatus.ts` (new file, ported design). |
| `PlanVerdictBox` | New React component: persistent plan-review status card + reject-with-reason form + regenerate CTA. Reused name from `plan-approval-ux` (component never built there — this is the fresh implementation). Modeled on `GateVerdictBox.tsx`'s existing card/toggle/form pattern. | `web-app/src/components/backlog/PlanVerdictBox.tsx` (new file). |
| "Revisions requested" (display copy) | The on-screen label for `PlanReviewStatus === "changes_requested"`. Deliberately **not** "Changes requested" verbatim. | Avoids the naming collision `pitfalls.md` §Gap-3 risk #5 flags against `MergeabilityPill`'s GitHub-PR-review `"changes_requested"` string, which can render on the same page. |
| Approval/rejection symmetry fix | `RejectPlan` clears `plan_approved`; `ApprovePlan` clears `plan_rejection_reason`; `TriggerTriage`'s regeneration-completion write clears both `plan_approved` and `plan_rejection_reason`. Three write sites, one invariant: never let a stale approval and a stale rejection reason coexist. | `pitfalls.md` carried-forward risks #1/#2, both P1 must-design-against. |
| Steer composer | New small inline composer (single-line input + Send/Cancel) attached to a steerable `LinkedSession` row in `SessionsSection`, calling `updateSession(sessionId, {steerMessage})`. Not `SessionActionsOverflow` embedded wholesale (architecture.md §3.3 option 1). | `web-app/src/components/backlog/detail/SessionsSection.tsx`. |
| `TriageQuestionAnswered` | Ephemeral (not persisted) domain event from `architecture.md`'s Event-Command-Policy table: operator fills ≥1 per-question answer field and submits, composing a `feedback` string for the existing `triggerTriage` call. | Naming only — not a new backend concept. |

14 glossary terms.

---

## 3. Pattern Decisions

| # | Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|---|---|---|---|---|---|
| P1 | Gap 1 answer→feedback | Client-side string composition (`"Q: ...\nA: ..."`) into the existing `TriggerTriageRequest.feedback` field | `research/architecture.md` §2, `research/stack.md` Sub-feature 1 | New `repeated QuestionAnswer answers` proto field / per-question ID scheme | No stable question ID exists anywhere in the stack; requirements.md's out-of-scope line forbids changing `feedback` semantics; composition satisfies AC1/AC2 with zero backend surface. |
| P2 | Gap 1 submission granularity | Per-question submit (GitHub inline-reply model) | `research/ux.md` Gap 1 | Batch "submit all answered questions" | ux.md recommends per-question (avoids "did I lose my other answers" on one failed submit) but flags it as a product call, not a foreclosed one; per-question also fits the existing single-toggle-then-textarea shape already used by `TriageReviewPanel`'s refine box. |
| P3 | Gap 2 steer transport | Widen `UpdateSession.steer_message`'s handler to fall back to `Instance.SendKeys` for non-autonomous sessions | ADR-001 | New dedicated `SteerSession` RPC mirroring the MCP tool 1:1 | A second browser-reachable steer RPC is literally the "parallel steering implementation" AC7 forbids; the existing field/handler/hook/notification-publish path already exists — this is a ~20-line additive handler change, not a new surface. |
| P4 | Gap 2 steerable scope | `isSteerable()` — Instance-backed (`work`/`review`) + not-ended sessions only | ADR-002 | Build a headless mid-run steering side-channel (file-polled or kill-and-re-invoke) | AC7 forbids a new steering mechanism; headless triage has no live `Instance` — structurally, not probabilistically, unsteerable through either existing path. |
| P5 | Gap 2 UI surface | New, lighter Steer button + inline composer added to `SessionsSection`'s existing row (label+input+Send/Cancel), not `SessionActionsOverflow` embedded | `research/architecture.md` §3.3 option 1, `research/ux.md` recommendation | Fetch the full `Session` proto object per linked session and reuse `SessionActionsOverflow` wholesale | `SessionsSection` only has `LinkedSession` (no `autonomousMode`, no `title`); the overflow menu's other 90% (Rename, Clone, Tags, Checkpoints) is irrelevant/confusing on this surface; smaller diff, no risk to the general session list. |
| P6 | Gap 3 state model | Durable non-enum fields (`plan_rejection_reason`/`plan_rejected_at`) + derived (never persisted) `PlanReviewStatus` | ADR-001 (plan-approval-ux), reconfirmed live against current `main` in `research/architecture.md` §1.3 | (a) Reuse `BacklogStatusEvent` as a same-status pseudo-transition; (b) replace `plan_approved` with a `plan_status` enum string | (a) breaks the `from_status != to_status` invariant every status-event reader assumes; (b) is a breaking change to `ApprovePlanRequest`'s existing bool-gate contract used at two live spawn-gate call sites. |
| P7 | Gap 3 reject→regen coupling | `RejectPlan` persists state only; regeneration is a separate, explicit "Regenerate with This Feedback" button reusing the existing `triggerTriage(id, reason)` | ADR-002 (plan-approval-ux) | Auto-invoke `TriggerTriage` synchronously inside `RejectPlan` | Would require refactoring `TriggerTriage`'s in-flight-guard/orphan-tombstone sequence — out of proportion; matches Gap 1's own click-count philosophy question (see Unresolved Questions). |
| P8 | Gap 3 backend scope | **Rebase/port** `bc0955d41`'s data-model + `RejectPlan` handler; **drop** `GetPlanArtifactContent`, `expected_modified_at_unix_ms`, `plan_artifacts_set_at`, and the reconciler widening | `research/architecture.md` §1.4, `research/build-vs-buy.md` §1, this plan's §0 | Port `bc0955d41` in full (all 4 pieces) | None of ACs 1-8 require in-browser plan rendering or stuck-item escalation; the cherry-pick's own one real conflict is in the code this trim doesn't touch, so trimming doesn't reduce port confidence. |
| P9 | Gap 3 naming | Display label "Revisions requested" (not "Changes requested") for `changes_requested` status | `pitfalls.md` §Gap-3 risk #5 | Reuse the bare literal "Changes requested" as UI copy | `MergeabilityPill.tsx` already renders a GitHub-PR-review `"changes_requested"` value on the same item detail page once a PR exists; identical on-screen text for two unrelated domains is a legibility bug, not a hygiene nit. |
| P10 | Cross-gap inline-disclosure shape | Hand-roll the toggle→form→focus-on-open→focus-return shape independently in each of the 3 new components (Gap 1 answer form, Gap 2 steer composer, Gap 3 reject form) | `research/ux.md` cross-cutting note 1 (flagged, not mandated) | Extract a shared `useDisclosureForm` hook now, before implementing any of the three | Only 3 call sites, each with slightly different focus/validation needs (per-question keying vs. session-id keying vs. length-gated reason); extracting an abstraction before a third *real* usage proves the shared shape is premature per `.claude/rules/interface-pollution-checklist.md`'s "no speculative interface/generic" guidance — noted as a good follow-up once a fourth consumer appears, not built now. |
| P11 | Gap 3 spawn-gate coupling | `ActionsSection`'s `canSpawnSession` calls `derivePlanReviewStatus(item)` instead of re-deriving `item.skipPlanning \|\| item.planApproved` inline | `pitfalls.md` §6 (no canonical derivation function) | Leave `ActionsSection`'s existing inline boolean check untouched | Two independent derivations of "is the gate open" is exactly the drift pattern that produced the original plan-approval-ux correctness bug (a stale approval bypassing the gate) — one function, two call sites. |

---

## 4. Migration Plan

**Schema**: two new nullable columns on `backlog_item`:
- `plan_rejection_reason string` (optional, default `""`)
- `plan_rejected_at timestamp` (optional, nillable)

Both purely additive — no backfill, no default beyond ent's zero-value handling. Every existing
row reads as "no rejection recorded" (`""`/`nil`), correct for 100% of pre-existing items.
**Reversibility**: a future drop of both columns is safe (nothing else in the codebase would
read a value from them once the RejectPlan RPC/handler is also removed) — this is a forward-only
plan, no down-migration is authored, consistent with every other additive field this schema has
shipped (`category`, `pipeline_mode`, etc., per `plan-approval-ux/implementation/plan.md`'s own
migration note). **Zero-downtime**: SQLite/ent auto-migrates additive nullable columns at
startup (`client.Schema.Create`) — no manual migration script.

**Codegen** (non-negotiable, per `.claude/rules/ent-schema-generation.md` — the stale `bc0955d41`
branch's own generated-code output must NOT be cherry-picked and trusted; regenerate fresh):
```
go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
```
Commit all regenerated `session/ent/` files in the same commit as the schema edit.

**Proto**: one new message pair (`RejectPlanRequest{item_id=1, reason=2}` /
`RejectPlanResponse{item=1}`), two new fields on `BacklogItem` — current `main`'s highest in-use
field number on `BacklogItem` is **32** (`allowed_transitions`, verified via direct proto read at
implementation-plan time, not assumed from the stale `bc0955d41` branch's `30`/`31`), so new
fields are `plan_rejection_reason = 33`, `plan_rejected_at = 34`. Run `make proto-gen` after the
edit — regenerates `session/gen/session/v1/*.go` and `web-app/src/gen/session/v1/*_pb.ts`.

**No data migration script** — consistent with every prior additive field on this schema.

**Backward-compatibility checkpoint**: after Epic 3, run the existing `TestApprovePlan_*` suite
unmodified — `ApprovePlanRequest{item_id}` must still succeed exactly as before, proving the
additive change didn't alter default behavior.

---

## Observability Plan

- `RejectPlan` follows `ApprovePlan`'s existing convention: no explicit log line on the happy
  path (errors are typed `connect` errors returned to the client), matching the existing
  low-risk-metadata-write pattern.
- The `RejectPlan`-clears-`PlanApproved` / `ApprovePlan`-clears-`PlanRejectionReason` /
  `TriggerTriage`-clears-both writes (the symmetry fix, P1 in Domain Glossary) fold into the
  *same* `session.BacklogItemUpdate` calls already covered by `TriggerTriage`'s existing
  `persistFailures`/`notifyTriagePersistFailure` operator-notification path — zero new logging
  code needed for that specific failure mode.
- Gap 2's widened `steer_message` branch (ADR-001) returns a real `FailedPrecondition` to the
  caller on `SendKeys` failure (unlike the existing autonomous branch, which only logs) — this
  is a deliberate new signal surfaced to the UI, not a new server-side log line; the UI is
  responsible for displaying it (Epic 2, Story 2.2).
- No new metrics/dashboards for any of the three gaps — none have throughput/latency
  characteristics differing from the RPCs they extend (`UpdateSession`, `TriggerTriage`,
  `ApprovePlan`'s sibling `RejectPlan`).

---

## Risk Control

| Risk | Mitigation | Where addressed |
|---|---|---|
| Approve/Reject leave `plan_approved=true` AND a non-empty rejection reason simultaneously, bypassing the backend spawn gate (`pitfalls.md` #1, P1) | `RejectPlan` clears `plan_approved` in the same write; regression test asserts the spawn gate itself still blocks post-reject-after-approve | Epic 3, Story 3.2 |
| Regeneration-completion write leaves a stale rejection reason visible on a plan the operator never saw (`pitfalls.md` #2, P1) | `TriggerTriage`'s completion write clears `plan_rejection_reason` in the same call that resets `plan_approved` | Epic 3, Story 3.2 |
| Empty/whitespace-only Request Changes text reaches the backend (AC4, explicit) | Server-side `strings.TrimSpace` + `InvalidArgument` on empty reason — not a UI-only check | Epic 3, Story 3.2 |
| `--feature sql/upsert` omitted during ent regen → silent breakage | Task states the exact command verbatim; `go build ./...` run immediately after as a smoke check | Epic 3, Story 3.1 |
| Two-tab concurrent Approve-vs-Reject race, second write silently wins (`pitfalls.md` #4) | **Explicitly accepted, not mitigated** — solo-operator, sequential single-tab usage is the assumed model per §0's scope trim (no optimistic-concurrency token this pass) | Recorded in Unresolved Questions, not silently absorbed |
| `changes_requested`/GitHub-PR-mergeability naming collision on the same page (`pitfalls.md` #5) | Distinct display copy "Revisions requested"; distinct badge color from `MergeabilityPill` | Epic 4, Story 4.2 (P9) |
| Headless triage/review session steer button renders but always 404s (`pitfalls.md` P1 — "affordance that has no effect") | `isSteerable()` gates rendering entirely for synthetic session kinds; ended work/review sessions render disabled-with-reason, never enabled-then-failing | Epic 2, Story 2.2 (ADR-002) |
| Backlog-linked non-autonomous sessions can never be steered because the existing RPC hard-rejects (`pitfalls.md`/ADR-001 root problem) | Widen the handler's non-autonomous branch to `Instance.SendKeys`, preserving the autonomous branch unchanged | Epic 2, Story 2.1 (ADR-001) |
| Stale in-flight triage guard rejects an answer-triggered retriage with a generic error (`pitfalls.md` Gap-1 #2) | Surface the existing `alreadyInFlight` error message as-is via the same error-toast path `TriageReviewPanel`'s refine flow already uses — no new locking, no silent retry | Epic 1, Story 1.1 |
| Stale question-answer form outlives the `TriageResult` it was answering (`pitfalls.md` Gap-1 #1) | Per-question local form state is keyed to the current render only; a new `iteration` on `triageResult` naturally remounts `TriageDiffSection`'s question list, discarding stale open forms | Epic 1, Story 1.1 |
| `PlanVerdictBox`/`ActionsSection` independently re-derive plan-review status and drift (`pitfalls.md` #6) | Single `derivePlanReviewStatus` pure function, unit-tested, imported by both | Epic 4, Story 4.1 (P11) |
| Feature registry / e2e coverage debt (`.claude/rules/feature-registry.md`) | Dedicated registry-file tasks per new RPC/component; `make registry-generate` run and `coverage-gaps.json` diff checked as the final task | Epic 5 |

---

## Unresolved Questions

Carried forward from requirements.md, resolved or explicitly re-scoped by this research/plan
pass:

1. **(requirements.md Open Question 1 — closed.)** Reuse `feedback`/status machinery vs. a
   dedicated RPC for Gap 3: **dedicated `RejectPlan` RPC** (ADR-001/002, ratified). Gap 1 (a
   structurally different, lower-stakes case) goes the *other* way — reuses `feedback` as-is,
   no new RPC — and this divergence is intentional, not an inconsistency: Gap 1's answer is
   input to the *next* triage run (cheap, retriable), Gap 3's rejection is a durable state
   transition an operator and the backend gate both need to observe persistently.
2. **(requirements.md Open Question 2 — closed.)** Should answered questions carry persistent
   resolved/unresolved state? **No** — stateless. A client-side, session-local "answered ✓"
   marker (discarded on any new `TriageResult`) is the full extent of "memory" this pass builds;
   nothing is persisted server-side. Confirmed as the correct default by every research pass
   that touched it (architecture.md, pitfalls.md, ux.md all independently converge here).
3. **(requirements.md Open Question 3 — closed via ADR-002.)** Steering a headless
   triage/review run does not, and structurally cannot, behave like steering an interactive
   session — AC6 is narrowed accordingly (see ADR-002's full reasoning).
4. **(requirements.md Open Question 4 — closed.)** Ship as one issue or split? **One ticket,
   five epics** (this plan) — the scope fit within a single `/sdd:full` planning pass once Gap
   3's backend was recognized as a port rather than a fresh design, matching the item's own
   "split only if Gap 3's state machine proves large enough" caveat, which it did not.
5. **(New, from `pitfalls.md` #4 — genuinely still open, by design, not oversight.)** The
   two-tab concurrent Approve-vs-Reject race has no guard in this pass (§0's scope trim dropped
   the optimistic-concurrency token along with `GetPlanArtifactContent`). Accepted for a
   solo-operator, single-tab-at-a-time usage model. If multi-tab/multi-operator usage becomes
   real, the fix is re-adding `expected_modified_at_unix_ms` (already fully designed in
   `bc0955d41`/plan-approval-ux, just not ported here) — not a new design.
6. **(New, from ADR-001's "Consequences" — a stated follow-up, not a defect.)** The widened
   `steer_message` handler's two branches now have asymmetric error contracts (autonomous
   branch logs-only-on-failure and still returns success; non-autonomous branch returns a real
   `FailedPrecondition`). Left asymmetric deliberately for this pass; unifying them would change
   the existing autonomous UI's behavior, which is out of scope here.

---

## Dependency Visualization

```
Epic 1 (Gap 1: Triage Q&A) ─────────────────────────────────────────┐
  frontend-only, no backend dep                                     │
                                                                      │
Epic 2 (Gap 2: Steer)                                                │
  Story 2.1 (backend: widen steer_message) ──► Story 2.2 (frontend)  │
                                                                      │
Epic 3 (Gap 3a: backend port)                                       │
  Story 3.1 (ent schema + repo) ──► Story 3.2 (RejectPlan RPC +      │
                                       symmetry fixes)                │
                                   └► Story 3.3 (backward-transition  │
                                       reset extension)               │
                        │                                            │
                        ▼                                            │
Epic 4 (Gap 3b: frontend, new)                                       │
  Story 4.1 (derivePlanReviewStatus + hook) ──► Story 4.2             │
    (PlanVerdictBox component) ──► Story 4.3 (wiring +               │
    ActionsSection gate reuse)                                       │
                        │                                            │
                        ▼                                            ▼
Epic 5 (Registry + E2E, AC8) ◄────────────────────────────────────────
  depends on Epics 1-4 all landing (final integration pass)
```

Epics 1 and 2 have no dependency on each other or on Epic 3/4 and can proceed fully in parallel.
Epic 4 cannot start until Epic 3's RPC exists. Epic 5 is last (needs every new
RPC/component to register against and every new UI path to exercise in e2e).

---

## Phase 1: Gap 1 — Triage Question Answering

### Epic 1.1: Per-question answer → composed feedback → existing retriage path

**Goal**: An operator answers a specific rendered triage question inline, without retyping it,
and the answer reaches the existing `triggerTriage(id, feedback)` retriage path.

#### Story 1.1.1: Per-question answer form in `TriageDiffSection`

**As a** backlog operator, **I want** to answer a specific triage clarifying question inline,
**so that** I don't have to hold the question in memory and retype it into an unrelated
feedback box.

**Acceptance Criteria** (AC1):
- An operator can submit an answer to a specific rendered question without retyping the
  question text, and that answer is delivered as feedback for the item's next triage run.
  - *Given* a `TriageResult` with a suggestion `{text: "Should retries be per-workflow or
    global?", rationale: "question"}`, *When* the operator clicks that question's "Answer ▸"
    toggle, types "Per-workflow, default to global" into the opened textarea, and clicks
    Submit, *Then* `triggerTriage(item.id, "Q: Should retries be per-workflow or global?\nA:
    Per-workflow, default to global")` is called — the question text is sourced from the
    already-rendered `TriageSuggestion.text`, never retyped by the operator.

**Files**: `web-app/src/components/backlog/TriageDiffSection.tsx`,
`web-app/src/components/backlog/TriageDiffSection.css.ts`,
`web-app/src/lib/backlog/composeQuestionAnswerFeedback.ts` (new),
`web-app/src/lib/backlog/composeQuestionAnswerFeedback.test.ts` (new).

##### Task 1.1.1a: `composeQuestionAnswerFeedback` pure function (~3 min)
- Create `web-app/src/lib/backlog/composeQuestionAnswerFeedback.ts`:
  ```ts
  /**
   * Composes a Q:/A: feedback string for a single answered triage question,
   * preserving the question↔answer link without requiring a stable question
   * ID (none exists — see architecture.md §2.1). Handed as-is to the
   * existing triggerTriage(id, feedback) call.
   */
  export function composeQuestionAnswerFeedback(questionText: string, answerText: string): string {
    return `Q: ${questionText.trim()}\nA: ${answerText.trim()}`;
  }
  ```
- Files: `web-app/src/lib/backlog/composeQuestionAnswerFeedback.ts`

##### Task 1.1.1b: Unit test for the composer (~2 min)
- `composeQuestionAnswerFeedback.test.ts`: asserts exact output shape, trims whitespace on
  both sides, handles multi-line answer text without corrupting the `Q:`/`A:` prefixes.
- Files: `web-app/src/lib/backlog/composeQuestionAnswerFeedback.test.ts`

##### Task 1.1.1c: Add `onAnswerQuestion` prop and per-question local state (~5 min)
- In `TriageDiffSection.tsx`, add a new prop to `TriageDiffSectionProps`:
  ```ts
  interface TriageDiffSectionProps {
    currentCriteria: AcCriterion[];
    suggestedSuggestions: TriageSuggestion[];
    /** Called with a composed "Q:.../A:..." feedback string when the operator submits an answer to one question. Absent in read-only historical renders. */
    onAnswerQuestion?: (feedback: string) => Promise<void>;
  }
  ```
- Add local state: `const [openIndex, setOpenIndex] = useState<number | null>(null);`,
  `const [answerDrafts, setAnswerDrafts] = useState<Record<number, string>>({});`,
  `const [answeredIndices, setAnsweredIndices] = useState<Set<number>>(new Set());`,
  `const [submittingIndex, setSubmittingIndex] = useState<number | null>(null);`.
- Files: `web-app/src/components/backlog/TriageDiffSection.tsx`

##### Task 1.1.1d: Render the "Answer ▸" toggle + inline textarea per question (~5 min)
- Replace the read-only `questionSuggestions.map((q, i) => <div key={i}>{q.text}</div>)`
  block with, per question: the static question text (unchanged), an "Answer ▸" `<button
  aria-expanded={openIndex === i} aria-controls={`triage-question-answer-input-${i}`}
  data-testid={`triage-question-answer-toggle-${i}`}>` toggling `openIndex`, and — when open —
  a `<textarea id={`triage-question-answer-input-${i}`}
  data-testid={`triage-question-answer-input-${i}`}>` bound to `answerDrafts[i]`, focused via
  `useEffect` on open. If `answeredIndices.has(i)`, render a read-only "✓ Answered: {draft}"
  line instead of the toggle (matches Google-Docs-resolved-comment treatment per `ux.md`).
- Files: `web-app/src/components/backlog/TriageDiffSection.tsx`

##### Task 1.1.1e: Submit/Cancel handlers (~4 min)
- Submit button (`data-testid={`triage-question-answer-submit-${i}`}`, `aria-disabled`+
  `disabled` while `answerDrafts[i]?.trim()` is empty, enforced in the click handler too):
  calls `composeQuestionAnswerFeedback(q.text, answerDrafts[i])`, then
  `await onAnswerQuestion(composed)`; on success sets `answeredIndices` to include `i`, closes
  the form (`setOpenIndex(null)`), and clears the draft. On failure, surfaces the existing
  in-flight-triage error (`pitfalls.md` Gap-1 #2) inline via the same `TriageErrorBanner`
  pattern `TriageReviewPanel`'s refine form already uses — do not silently drop the answer.
  Cancel button: `setOpenIndex(null)`, returns focus to the toggle button via a `ref` captured
  before opening (per `ux.md`'s "don't inherit the Cancel-focus-return gap" note — this
  component is new, get it right from the start).
- Files: `web-app/src/components/backlog/TriageDiffSection.tsx`

##### Task 1.1.1f: Styles (~3 min)
- Extend `TriageDiffSection.css.ts` (vanilla-extract, `vars.*` tokens per
  `.claude/rules/css-architecture.md`) with `answerToggle`, `answerForm`, `answerTextarea`,
  `answerActions`, `answeredMarker` styles, mirroring `TriageReviewPanel.css.ts`'s
  `refineForm`/`refineTextarea`/button styles rather than inventing new visual language.
  Stack Submit/Cancel full-width below `breakpoints.sm` per `ux.md`'s mobile note.
- Files: `web-app/src/components/backlog/TriageDiffSection.css.ts`

##### Task 1.1.1g: `role="status" aria-live="polite"` on the answered-marker transition (~2 min)
- Wrap the "✓ Answered" swap in a `role="status" aria-live="polite"` region (routine, expected
  outcome — not `role="alert"`, reserved for a failed submit per `ux.md`'s `InlineNotice` vs
  `InlineError` split).
- Files: `web-app/src/components/backlog/TriageDiffSection.tsx`

##### Task 1.1.1h: Component tests (~5 min)
- New/extended `TriageDiffSection.test.tsx`: toggle opens/closes the form with correct
  `aria-expanded`; Submit disabled until non-empty; Submit calls `onAnswerQuestion` with the
  exact composed string (spy assertion, not a snapshot); after successful submit the question
  renders "✓ Answered"; Cancel returns focus to the toggle button; component renders nothing
  extra when `onAnswerQuestion` is absent (read-only historical mode, matches
  `TriageReviewPanel`'s existing `readOnly` convention).
- Files: `web-app/src/components/backlog/TriageDiffSection.test.tsx` (new or extended if one
  exists)

---

#### Story 1.1.2: Wire the new prop through `TriageReviewPanel` to the existing retriage call

**As a** backlog operator, **I want** my question answer to actually trigger a re-triage,
**so that** the agent revises the item using my answer, with no new triage mechanism.

**Acceptance Criteria** (AC2):
- Submitting an answer triggers (or explicitly queues) a re-triage using the existing
  feedback-driven re-triage path.
  - *Given* an operator has just submitted an answer via Task 1.1.1e, *When* the composed
    feedback string reaches `BacklogItemDetail`, *Then* it is passed to the exact same
    `handleRefineTriage`/`triggerTriage(item.id, feedback)` call the existing generic
    "Not quite — give feedback" button already uses (`BacklogItemDetail.tsx:787-793`) — no new
    RPC, no new triage-trigger code path.

**Files**: `web-app/src/components/backlog/TriageReviewPanel.tsx`,
`web-app/src/components/backlog/BacklogItemDetail.tsx`.

##### Task 1.1.2a: Thread `onAnswerQuestion` through `TriageReviewPanel` (~3 min)
- Add `onAnswerQuestion?: (feedback: string) => Promise<void>;` to
  `TriageReviewPanelWriteProps` (alongside the existing `onRefine`), and pass it straight
  through to the `<TriageDiffSection onAnswerQuestion={onAnswerQuestion} ... />` render call
  (~line 228).
- Files: `web-app/src/components/backlog/TriageReviewPanel.tsx`

##### Task 1.1.2b: Wire `onAnswerQuestion={handleRefineTriage}` in `BacklogItemDetail` (~2 min)
- At the existing `<TriageReviewPanel onRefine={handleRefineTriage} ...>` call site
  (~line 1235-1246), add `onAnswerQuestion={handleRefineTriage}` — the *same* handler already
  wired to `onRefine`, since both ultimately call `triggerTriage(item.id, feedback)` with a
  differently-composed `feedback` string. No new callback needs to be defined.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 1.1.2c: Regression test — answer submission calls `triggerTriage` (~4 min)
- Extend `BacklogItemDetail.test.tsx` (or add a focused RTL test): render the detail view with
  a mocked `TriageResult` containing one question, open its answer form, submit an answer,
  assert the mocked `triggerTriage` client call fires with `{itemId, feedback: "Q: ...\nA:
  ..."}` — proving the wiring reaches the real RPC call, not just the composer function tested
  in isolation (Task 1.1.1b).
- Files: `web-app/src/components/backlog/BacklogItemDetail.test.tsx`

##### Task 1.1.2d: Registry entry touch (~2 min)
- `docs/registry/features/backend/backlog/trigger-triage.json`: no new RPC, but per
  `.claude/rules/feature-registry.md`'s "Modified RPC" step, append this project's new e2e test
  ID (added in Epic 5) to `testIds` and bump `lastModified`.
- Files: `docs/registry/features/backend/backlog/trigger-triage.json`

---

## Phase 2: Gap 2 — Steer a Backlog-Linked Session

### Epic 2.1: Widen the existing steer RPC for non-autonomous sessions (ADR-001)

**Goal**: `UpdateSession.steer_message` works for ordinary (non-autonomous) work/review
sessions, not just autonomous-mode ones, without adding a second RPC.

#### Story 2.1.1: Backend — fall back to `Instance.SendKeys` for non-autonomous sessions

**As a** backlog operator, **I want** the existing steer mechanism to actually work for the
ordinary work/review sessions backlog items spawn, **so that** the new UI affordance isn't a
button that always fails.

**Acceptance Criteria** (AC7 — reuses the existing `steer_session` path, no parallel
implementation):
- *Given* a live, non-autonomous-mode `Instance` backing a work session, *When*
  `UpdateSession(id, {steerMessage: "focus on the auth module first"})` is called, *Then* the
  handler sends the message via `instance.SendKeys(msg + "\r")` — the same primitive the MCP
  `steer_session` tool's PTY-fallback branch already uses — and returns success; the existing
  autonomous branch (`ClaudeController.SendCommandImmediate`) is unchanged for
  `autonomousMode: true` sessions.

**Files**: `server/services/session_service.go`, `server/services/session_service_test.go`.

##### Task 2.1.1a: Widen the `SteerMessage` handler branch (~5 min)
- In `server/services/session_service.go`, replace the current unconditional
  `if !instance.AutonomousMode { return ... FailedPrecondition }` (current lines ~2012-2016)
  with the two-branch shape ADR-001 specifies verbatim:
  ```go
  if req.Msg.SteerMessage != nil && *req.Msg.SteerMessage != "" {
      if instance.AutonomousMode {
          // Unchanged: autonomous sessions keep the ClaudeController command-queue path.
          if controller := instance.GetController(); controller != nil {
              if _, sendErr := controller.SendCommandImmediate(*req.Msg.SteerMessage + "\r"); sendErr != nil {
                  log.Warn("[UpdateSession] failed to send steer_message", "session", instance.Title, "err", sendErr)
              }
          }
      } else {
          // New: non-autonomous, Instance-backed sessions get the same PTY send
          // primitive the MCP steer_session tool already falls back to
          // (tools_terminal.go's SendKeys branch). Unlike the autonomous branch, a
          // send failure IS returned to the caller so the UI can surface it.
          if err := instance.SendKeys(*req.Msg.SteerMessage + "\r"); err != nil {
              return nil, connect.NewError(connect.CodeFailedPrecondition,
                  fmt.Errorf("failed to steer session %q: %w", instance.Title, err))
          }
      }
  }
  ```
  Both branches keep publishing the existing "Steering input sent" `NotificationEvent`
  unchanged.
- Files: `server/services/session_service.go`

##### Task 2.1.1b: Backend test — non-autonomous session steers via `SendKeys` (~5 min)
- Add `TestUpdateSession_SteerMessage_NonAutonomousSession_SendsViaSendKeys`: spin up a
  non-autonomous test `Instance` backed by a fake/mock that records `SendKeys` calls, call
  `UpdateSession` with `steerMessage` set, assert the mock recorded the exact message +
  `"\r"` suffix and the RPC returned success. Add
  `TestUpdateSession_SteerMessage_NonAutonomousSession_SendKeysFailure_ReturnsFailedPrecondition`:
  same setup with a `SendKeys` that returns an error, assert the RPC returns
  `connect.CodeFailedPrecondition`. Add
  `TestUpdateSession_SteerMessage_AutonomousSession_StillUsesController` as a regression guard
  that the pre-existing autonomous path is untouched.
- Files: `server/services/session_service_test.go`

##### Task 2.1.1c: Registry touch (~2 min)
- `docs/registry/features/backend/session/update-session.json` (or equivalent existing entry —
  locate via `grep -l "session:update"` under `docs/registry/features/backend/`): append the
  three new test IDs to `testIds`, bump `lastModified`. This is a widened existing RPC, not a
  new one — no new registry file.
- Files: whichever `docs/registry/features/backend/**/update-session.json`-equivalent file
  `session:update`'s marker currently lives in.

---

### Epic 2.2: Frontend — Steer control on backlog-linked session rows (ADR-002)

**Goal**: A Steer button appears only for Instance-backed, live work/review sessions in
`SessionsSection`, and calls the widened `UpdateSession` RPC.

#### Story 2.2.1: `isSteerable` predicate

**As a** developer wiring the Steer control, **I want** one canonical function deciding whether
a `LinkedSession` row is steerable, **so that** the row's render branch and the Steer button's
visibility never independently drift.

**Acceptance Criteria** (part of AC6, narrowed per ADR-002):
- *Given* a `LinkedSession` with `role: "triage"` (or a `headless-`-prefixed `sessionId`),
  *When* `isSteerable(session)` is called, *Then* it returns `false` — a triage session is never
  steerable, structurally.
- *Given* a `LinkedSession` with `role: "work"` and `endedAt` unset, *When* `isSteerable(session)`
  is called, *Then* it returns `true`.
- *Given* the same work session but with `endedAt` set, *When* `isSteerable(session)` is called,
  *Then* it returns `false`.

**Files**: `web-app/src/lib/backlog/sessionKind.ts`, `web-app/src/lib/backlog/sessionKind.test.ts`.

##### Task 2.2.1a: Add `isSteerable` export (~2 min)
- In `web-app/src/lib/backlog/sessionKind.ts`, add (exact shape from ADR-002):
  ```ts
  /**
   * A LinkedSession is steerable iff it is Instance-backed ("work"/"review",
   * per classifySessionKind) and has not ended. Synthetic session kinds
   * (headless triage/review, blocked-guardrail, manual-review-marker) are
   * never steerable — no session.Instance was ever created for them. See
   * ADR-002 (project_plans/backlog-operator-feedback-loop/decisions/).
   */
  export function isSteerable(session: Pick<LinkedSession, "role" | "sessionId" | "endedAt">): boolean {
    const kind = classifySessionKind(session);
    return (kind === "work" || kind === "review") && !session.endedAt;
  }
  ```
- Files: `web-app/src/lib/backlog/sessionKind.ts`

##### Task 2.2.1b: Unit tests (~3 min)
- `sessionKind.test.ts`: one case per `SessionKind` value confirming `isSteerable` returns
  `true` only for `work`/`review` with no `endedAt`, `false` for all three synthetic kinds
  regardless of `endedAt`, and `false` for `work`/`review` with `endedAt` set.
- Files: `web-app/src/lib/backlog/sessionKind.test.ts`

---

#### Story 2.2.2: Steer button + inline composer in `SessionsSection`

**As a** backlog operator, **I want** to steer a running work/review session without leaving
the item detail view, **so that** I don't lose the context I built up reading the item.

**Acceptance Criteria** (AC6, narrowed per ADR-002; AC7):
- *Given* a `ready`-status backlog item with an active, non-ended work session, *When* the item
  detail view renders `SessionsSection`, *Then* a "Steer" button is visible next to that
  session's row, and clicking it → typing a message → Send calls
  `updateSession(sessionId, {steerMessage: message})` — the same `UpdateSession` RPC/hook the
  general session list's own Steer dialog uses (AC7).
- *Given* the same view but for a `headless-`-prefixed triage session row, *When* the row
  renders, *Then* no Steer control is rendered at all (not disabled — absent), since that row
  already renders as a collapsed `SessionDiagnosticPanel`, not an action surface.
- *Given* an ended work session row, *When* it renders, *Then* the Steer button is present but
  `disabled`+`aria-disabled` with `title="Session has ended — steering is unavailable"`.

**Files**: `web-app/src/components/backlog/detail/SessionsSection.tsx`,
`web-app/src/components/backlog/detail/SessionsSection.css.ts` (or equivalent),
`web-app/src/components/backlog/BacklogItemDetail.tsx`.

##### Task 2.2.2a: Add `onSteerSession` prop to `SessionsSectionProps` (~2 min)
- Extend `SessionsSectionProps` with `onSteerSession: (session: LinkedSession, message:
  string) => Promise<void>;` and `steeringSessionId: string | null;` (mirrors the existing
  `deletingSessionId`/`onDeleteSession` pattern one prop above it).
- Files: `web-app/src/components/backlog/detail/SessionsSection.tsx`

##### Task 2.2.2b: Render the Steer button in the non-synthetic row branch (~5 min)
- In the `!isSynthetic` `<a className={styles.sessionLink}>` branch (~lines 146-164), add a
  `Steer` button next to the existing Delete button, following the same inline-button
  precedent (not a new `···` overflow menu, per P5):
  ```tsx
  <button
    type="button"
    className={styles.sessionSteerBtn}
    disabled={!isSteerable(s) || steeringSessionId === s.sessionId}
    aria-disabled={!isSteerable(s)}
    title={!isSteerable(s) && s.endedAt ? "Session has ended — steering is unavailable" : undefined}
    aria-label={`Steer session ${s.sessionId}`}
    data-testid={`session-steer-toggle-${s.sessionId}`}
    onClick={(e) => { e.preventDefault(); setOpenSteerFor(s.sessionId); }}
  >
    Steer
  </button>
  ```
  Compute `isSteerable(s)` once per row (imported from `sessionKind.ts`) rather than inline
  three times. Add local `const [openSteerFor, setOpenSteerFor] = useState<string | null>(null);`
  at the top of the component. **Do not** render this button at all for the `isSynthetic`
  branch — no conditional-disabled version there, per ADR-002.
- Files: `web-app/src/components/backlog/detail/SessionsSection.tsx`

##### Task 2.2.2c: Inline composer (single-line input + Send/Cancel) (~5 min)
- When `openSteerFor === s.sessionId`, render a small composer row directly below that
  session's row (matching Gap 1's disclosure shape — label + text input + Send/Cancel,
  submit-on-Enter):
  ```tsx
  {openSteerFor === s.sessionId && (
    <div className={styles.steerComposer} role="form" aria-label={`Steer session ${s.sessionId}`}>
      <input
        ref={steerInputRef}
        type="text"
        value={steerDraft}
        onChange={(e) => setSteerDraft(e.target.value)}
        onKeyDown={(e) => { if (e.key === "Enter") void handleSteerSubmit(s); if (e.key === "Escape") handleSteerCancel(); }}
        data-testid={`session-steer-input-${s.sessionId}`}
        disabled={steeringSessionId === s.sessionId}
      />
      <button
        type="button"
        onClick={() => void handleSteerSubmit(s)}
        disabled={steeringSessionId === s.sessionId || !steerDraft.trim()}
        aria-busy={steeringSessionId === s.sessionId}
        data-testid={`session-steer-submit-${s.sessionId}`}
      >
        {steeringSessionId === s.sessionId ? "Sending…" : "Send"}
      </button>
      <button type="button" onClick={handleSteerCancel} data-testid={`session-steer-cancel-${s.sessionId}`}>
        Cancel
      </button>
    </div>
  )}
  ```
  `handleSteerSubmit` calls `onSteerSession(s, steerDraft.trim())`, then on success clears
  `steerDraft` and closes (`setOpenSteerFor(null)`), returning focus to the toggle button
  (captured `ref`, per Gap 1's same focus-return discipline). On failure, keep the composer
  open and surface the RPC's error message inline (this is the branch that now returns a real
  `FailedPrecondition` per Task 2.1.1a, not a silent log-only failure).
- Files: `web-app/src/components/backlog/detail/SessionsSection.tsx`

##### Task 2.2.2d: Styles (~3 min)
- Add `sessionSteerBtn`, `steerComposer` styles to `SessionsSection.css.ts` (vanilla-extract,
  `vars.*` tokens), ≥44×44px touch target per `ux.md`'s mobile note, stacking full-width below
  `breakpoints.sm` rather than wrapping awkwardly next to the Delete button. Use a `data-*`
  attribute + `selectors`, not inline `style={{flexDirection}}`, per
  `.claude/rules/css-architecture.md`.
- Files: `web-app/src/components/backlog/detail/SessionsSection.css.ts`

##### Task 2.2.2e: Wire `onSteerSession` in `BacklogItemDetail` (~4 min)
- Import `updateSession` from the already-imported `useSessionService()` hook (alongside the
  existing `deleteSession`, `~line 99`). Add `handleSteerSession`, mirroring
  `handleDeleteSession`'s existing shape (~line 462-491):
  ```ts
  const handleSteerSession = useCallback(
    async (s: LinkedSession, message: string) => {
      const toastKey = `${s.sessionId}:steer`;
      setSteeringSessionId(s.sessionId);
      try {
        await updateSession(s.sessionId, { steerMessage: message });
        showActionToast("Steering message sent.", "success", toastKey);
      } catch (err) {
        const msg = err instanceof Error ? err.message : "Failed to steer session.";
        showActionToast(msg, "error", toastKey);
        throw err;
      } finally {
        setSteeringSessionId(null);
      }
    },
    [updateSession, showActionToast]
  );
  ```
  Add `const [steeringSessionId, setSteeringSessionId] = useState<string | null>(null);` near
  `deletingSessionId`'s declaration. Pass `onSteerSession={handleSteerSession}
  steeringSessionId={steeringSessionId}` into the existing `<SessionsSection>` call
  (~line 1404-1412).
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 2.2.2f: Component tests (~5 min)
- `SessionsSection.test.tsx`: Steer button absent for a `headless-`-prefixed row; present and
  enabled for a live work-session row; disabled+`title` set for an ended work-session row;
  clicking Steer → typing → Send calls `onSteerSession` with the exact session + trimmed
  message; Enter key submits, Escape cancels and returns focus.
- Files: `web-app/src/components/backlog/detail/SessionsSection.test.tsx` (new or extended)

##### Task 2.2.2g: Registry touch (~2 min)
- No new RPC (widened existing `session:update`). No new frontend registry entry is strictly
  required (`SessionsSection` already has one, if any) — if it doesn't, add
  `docs/registry/features/frontend/backlog-session-steer.json` per
  `.claude/rules/feature-registry.md`'s "New UI feature" template, `filePath:
  "web-app/src/components/backlog/detail/SessionsSection.tsx"`.
- Files: `docs/registry/features/frontend/backlog-session-steer.json` (new, if not already
  covered by an existing `SessionsSection` entry)

---

## Phase 3: Gap 3a — Plan Rejection Data Model + `RejectPlan` RPC (backend port)

**Provenance note for every task in this phase**: tasks marked **[PORT]** are a near-verbatim
port of orphaned commit `bc0955d41` (`recover/plan-approval-ux` branch), re-derived against
current `main` per `research/build-vs-buy.md`'s verified-clean-cherry-pick finding — near-zero
net-new design, just rebase + regenerate + retest. Tasks marked **[NEW]** are this project's
own addition, not present in `bc0955d41` (the symmetry-fix write in `TriggerTriage`'s
completion path is `bc0955d41`'s own content, so it's marked **[PORT]** too — only the exact
field numbering and the *omission* of `plan_artifacts_set_at`/`expected_modified_at_unix_ms`
are this plan's changes).

### Epic 3.1: ent schema + repository plumbing **[PORT]**

**Goal**: `plan_rejection_reason`/`plan_rejected_at` exist as durable fields on `BacklogItem`,
readable/writable through the existing repository layer.

#### Story 3.1.1: Schema fields

**Acceptance Criteria**: schema change is purely additive, no backfill needed (Migration Plan
§4).

**Files**: `session/ent/schema/backlog_item.go`, `session/repository.go`,
`session/ent_repository_backlog.go`.

##### Task 3.1.1a: ent schema fields **[PORT]** (~3 min)
- After the `plan_artifacts_path` field, add:
  ```go
  field.String("plan_rejection_reason").
      Optional().
      Comment("Free-text reason from the most recent RejectPlan call. Cleared on ApprovePlan, on the next TriggerTriage completion, and on backward transition to idea/refining. See project_plans/plan-approval-ux/decisions/ADR-001."),
  field.Time("plan_rejected_at").
      Optional().
      Nillable(),
  ```
  (`plan_artifacts_set_at` deliberately **not** added — §0 scope trim.)
- Files: `session/ent/schema/backlog_item.go`

##### Task 3.1.1b: Regenerate ent codegen **[PORT]** (~3 min, terminal + generated files)
- Run the exact command from `session/ent/generate.go` (per
  `.claude/rules/ent-schema-generation.md`):
  ```
  go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
  ```
  then `go build ./...`. Commit every changed file under `session/ent/` together with Task
  3.1.1a's schema edit in the same commit. **Do not** hand-port `bc0955d41`'s own (111-commit
  stale) generated output — regenerate fresh.
- Files: `session/ent/*` (generated — do not hand-edit)

##### Task 3.1.1c: `BacklogItemData`/`BacklogItemUpdate` struct fields **[PORT]** (~3 min)
- In `session/repository.go`, add to `BacklogItemData` (after `PlanArtifactsPath`):
  `PlanRejectionReason string`, `PlanRejectedAt *time.Time`. Add to `BacklogItemUpdate` (same
  location): `PlanRejectionReason *string`, `PlanRejectedAt *time.Time`.
- Files: `session/repository.go`

##### Task 3.1.1d: Repository mapping — read path, create, update **[PORT]** (~5 min)
- In `session/ent_repository_backlog.go`, three edits: (1) `backlogItemToData` — add
  `PlanRejectionReason: item.PlanRejectionReason, PlanRejectedAt: item.PlanRejectedAt,` after
  the `PlanArtifactsPath` line; (2) `CreateBacklogItem` builder chain — add
  `.SetNillablePlanRejectionReason(&data.PlanRejectionReason).SetNillablePlanRejectedAt(data.PlanRejectedAt)`
  after `.SetNillablePlanArtifactsPath(...)`; (3) `UpdateBacklogItem`'s partial-update block —
  add the two `if update.X != nil { u.SetX(*update.X) }` guards for both new fields.
- Files: `session/ent_repository_backlog.go`

##### Task 3.1.1e: `updatedFieldsFromBacklogItemUpdate` **[PORT]** (~2 min)
- Add the two new fields' change-tracking entries (`fields = append(fields,
  "planRejectionReason")` / `"planRejectedAt"`) after the existing `planArtifactsPath` block.
- Files: `session/ent_repository_backlog.go`

##### Task 3.1.1f: Backward-compat regression check (~2 min, terminal)
- Run the existing `TestApprovePlan_*` suite unmodified: `go test ./server/services/
  -run TestApprovePlan` — confirms the additive schema change didn't alter
  `ApprovePlanRequest{item_id}`'s existing behavior (Migration Plan checkpoint).

---

### Epic 3.2: `RejectPlan` RPC + Approve/Reject/Regenerate symmetry fixes

**Goal**: An operator can reject a plan with a required reason; the three write sites
(`RejectPlan`, `ApprovePlan`, `TriggerTriage` completion) never leave a stale approval and a
stale rejection reason coexisting.

#### Story 3.2.1: Proto + `RejectPlan` handler **[PORT, trimmed]**

**Acceptance Criteria** (AC4 — an empty rejection is not possible, enforced server-side):
- *Given* a `ready`-status item with `plan_artifacts_path` set, *When* `RejectPlan(item_id,
  reason: "")`  (or whitespace-only) is called, *Then* the RPC returns `InvalidArgument` — not
  a UI-only check.
- *Given* the same item, *When* `RejectPlan(item_id, reason: "missing caching plan")` is
  called, *Then* the response's `item.planRejectionReason == "missing caching plan"`, a
  non-nil `planRejectedAt`, and `item.planApproved == false` (symmetry fix — even if the plan
  had previously been approved).

**Files**: `proto/session/v1/backlog.proto`, `server/services/backlog_service_lifecycle.go`,
`server/services/backlog_service_test.go`.

##### Task 3.2.1a: Proto message + RPC registration (~3 min)
- Add after `ApprovePlanResponse` (current line 469):
  ```protobuf
  message RejectPlanRequest {
    string item_id = 1;
    // reason is required free-text feedback explaining what should change,
    // mirroring TriggerTriageRequest.feedback's refinement-input pattern.
    // RejectPlan does not itself trigger regeneration — see
    // project_plans/plan-approval-ux/decisions/ADR-002.
    string reason = 2;
  }
  message RejectPlanResponse {
    BacklogItem item = 1;
  }
  ```
  Add `plan_rejection_reason = 33` and `plan_rejected_at = 34` to `message BacklogItem` (after
  `allowed_transitions = 32`). Register `rpc RejectPlan(RejectPlanRequest) returns
  (RejectPlanResponse) {}` alongside `rpc ApprovePlan(...)` (current line 807).
  **No `expected_modified_at_unix_ms` field** — §0 scope trim.
- Files: `proto/session/v1/backlog.proto`

##### Task 3.2.1b: `make proto-gen` (~2 min, terminal)
- Regenerates `session/gen/session/v1/*.go` and `web-app/src/gen/session/v1/*_pb.ts`. Commit
  all generated output.

##### Task 3.2.1c: `RejectPlan` handler **[PORT, trimmed — no freshness check]** (~5 min)
- Add after `ApprovePlan` in `server/services/backlog_service_lifecycle.go`:
  ```go
  // RejectPlan records a rejection reason for the item's current plan
  // artifacts and clears any existing approval. Does not itself trigger
  // regeneration — see project_plans/plan-approval-ux/decisions/ADR-002.
  // +api: backlog:reject-plan
  func (s *BacklogService) RejectPlan(
      ctx context.Context,
      req *connect.Request[sessionv1.RejectPlanRequest],
  ) (*connect.Response[sessionv1.RejectPlanResponse], error) {
      if s.storage == nil {
          return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
      }
      reason := strings.TrimSpace(req.Msg.Reason)
      if reason == "" {
          return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("reason is required"))
      }
      item, err := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)
      if err != nil {
          if ent.IsNotFound(err) {
              return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
          }
          return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get backlog item: %w", err))
      }
      if item.PlanArtifactsPath == "" {
          return nil, connect.NewError(connect.CodeFailedPrecondition,
              fmt.Errorf("no plan artifacts found — run TriggerTriage first"))
      }
      now := time.Now()
      approvalReset := false
      update := session.BacklogItemUpdate{
          PlanRejectionReason: &reason,
          PlanRejectedAt:      &now,
          PlanApproved:        &approvalReset,
      }
      updated, err := s.storage.UpdateBacklogItem(ctx, req.Msg.ItemId, update, nil)
      if err != nil {
          return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to reject plan: %w", err))
      }
      return connect.NewResponse(&sessionv1.RejectPlanResponse{
          Item: backlogItemToProto(updated, s.buildCostLookup()),
      }), nil
  }
  ```
  `PlanApproved: &approvalReset` is required, not optional polish (the symmetry fix — see Risk
  Control) — without it a normal "approved, then noticed a problem, rejected" flow leaves
  `plan_approved=true` AND a non-empty rejection reason simultaneously, and the backend spawn
  gates check the raw bool directly.
- Files: `server/services/backlog_service_lifecycle.go`

##### Task 3.2.1d: `RejectPlan` handler tests (~5 min)
- Add `TestRejectPlan_HappyPath_SetsReasonAndTimestamp`,
  `TestRejectPlan_EmptyReason_ReturnsInvalidArgument`,
  `TestRejectPlan_WhitespaceOnlyReason_ReturnsInvalidArgument`,
  `TestRejectPlan_MissingPlanArtifactsPath_ReturnsFailedPrecondition` (mirrors `ApprovePlan`'s
  existing test shape).
- Files: `server/services/backlog_service_test.go`

##### Task 3.2.1e: Symmetry regression test — reject clears a stale approval (~4 min)
- Add `TestRejectPlan_ClearsExistingApproval`: approve a plan (`item.PlanApproved == true`),
  then reject it, assert `item.PlanApproved == false` in the `RejectPlanResponse.Item` and via
  a fresh `GetBacklogItem` read. Also call whatever helper the existing spawn-gate tests use
  and confirm it still returns the "plan not approved" precondition error after
  reject-following-approve — the concrete case the symmetry fix prevents, not just a
  field-value check.
- Files: `server/services/backlog_service_test.go`

---

#### Story 3.2.2: `ApprovePlan` clears a stale rejection reason **[PORT]**

**Acceptance Criteria** (part of AC5 — the state must be distinguishable and correctly
reset in both directions):
- *Given* an item with `plan_rejection_reason = "missing caching plan"`, *When* `ApprovePlan`
  is called, *Then* `item.planRejectionReason == ""` afterward.

**Files**: `server/services/backlog_service_lifecycle.go`, `server/services/backlog_service_test.go`.

##### Task 3.2.2a: Extend `ApprovePlan`'s update literal **[PORT]** (~3 min)
- In the existing `ApprovePlan` handler (current lines 771-776), extend the update literal:
  ```go
  now := time.Now()
  approved := true
  clearedReason := ""
  update := session.BacklogItemUpdate{
      PlanApproved:        &approved,
      PlanApprovedAt:      &now,
      PlanRejectionReason: &clearedReason,
  }
  ```
- Files: `server/services/backlog_service_lifecycle.go`

##### Task 3.2.2b: Regression test (~3 min)
- Add `TestApprovePlan_ClearsExistingRejectionReason`: reject a plan, then approve it, assert
  `plan_rejection_reason == ""`.
- Files: `server/services/backlog_service_test.go`

---

#### Story 3.2.3: `TriggerTriage`'s regeneration-completion write clears both fields **[PORT]**

**Acceptance Criteria** (part of AC2/AC5 — a freshly-regenerated plan must not carry stale
approval or stale rejection feedback that the regeneration was meant to address):
- *Given* an item with `plan_approved=true` and/or `plan_rejection_reason` set, *When*
  `TriggerTriage(item_id, feedback)` completes successfully and regenerates the plan, *Then*
  `item.planApproved == false` and `item.planRejectionReason == ""` afterward — the newly
  generated plan is `pending_review`, not carrying forward either stale label.

**Files**: `server/services/backlog_service_triage.go`, `server/services/backlog_service_test.go`.

##### Task 3.2.3a: Extend the completion-write block **[PORT]** (~3 min)
- The current completion block (line 2525-2529, quoted verbatim from live `HEAD`):
  ```go
  pap := artifactAbsPath
  if item.PipelineMode == session.DefaultSDDPipelineModeSlug {
      pap = filepath.Join(triageWorkDir, "project_plans", result.Title, "implementation")
  }
  update := session.BacklogItemUpdate{PlanArtifactsPath: &pap}
  ```
  Extend to also reset approval and clear the rejection reason, **preserving the existing
  SDD-pipeline-mode `pap` computation** (this is the one hunk `research/architecture.md`
  §1.2 already flagged as needing a careful merge, not a raw overwrite):
  ```go
  pap := artifactAbsPath
  if item.PipelineMode == session.DefaultSDDPipelineModeSlug {
      pap = filepath.Join(triageWorkDir, "project_plans", result.Title, "implementation")
  }
  approvalReset := false
  clearedReason := ""
  update := session.BacklogItemUpdate{
      PlanArtifactsPath:   &pap,
      PlanApproved:        &approvalReset,
      PlanRejectionReason: &clearedReason,
  }
  ```
  Do not reset `PlanRejectedAt`/`PlanApprovedAt` (best-effort, matches the existing
  `PlanApprovedAt`-left-stale convention already used at the backward-transition reset block).
- Files: `server/services/backlog_service_triage.go`

##### Task 3.2.3b: Regression tests (~5 min)
- Add `TestTriggerTriage_RefineWithFeedback_ResetsPlanApproved`: approve a plan, call
  `TriggerTriage` with feedback against a seeded prior triage result, poll until the async
  goroutine completes (reuse the existing `TestTriggerTriage_RefineWithFeedback` test's polling
  helper), assert `item.PlanApproved == false`. Add
  `TestTriggerTriage_RefineWithFeedback_ClearsRejectionReason`: reject a plan with a reason,
  call `TriggerTriage` with feedback, assert `item.PlanRejectionReason == ""` after completion.
- Files: `server/services/backlog_service_test.go`

##### Task 3.2.3c: Registry touch (~2 min)
- Update `docs/registry/features/backend/backlog/trigger-triage.json`'s `testIds`/
  `lastModified` (same file Task 1.1.2d touches — coordinate, don't clobber).
- Files: `docs/registry/features/backend/backlog/trigger-triage.json`

##### Task 3.2.3d: New backend registry entry for `RejectPlan` (~2 min)
- Create `docs/registry/features/backend/backlog/reject-plan.json`:
  ```json
  {
    "id": "backlog:reject-plan",
    "type": "backend",
    "service": "BacklogService",
    "method": "RejectPlan",
    "protoFile": "proto/session/v1/backlog.proto",
    "markerFound": true,
    "handlerFile": "server/services/backlog_service_lifecycle.go",
    "tested": true,
    "testIds": [
      "TestRejectPlan_HappyPath_SetsReasonAndTimestamp",
      "TestRejectPlan_EmptyReason_ReturnsInvalidArgument",
      "TestRejectPlan_WhitespaceOnlyReason_ReturnsInvalidArgument",
      "TestRejectPlan_MissingPlanArtifactsPath_ReturnsFailedPrecondition",
      "TestRejectPlan_ClearsExistingApproval"
    ],
    "lastModified": "2026-08-12T00:00:00Z"
  }
  ```
- Files: `docs/registry/features/backend/backlog/reject-plan.json`

---

### Epic 3.3: Backward-transition reset extension **[PORT]**

#### Story 3.3.1: Sending an item back to Idea/Refining clears the rejection reason too

**Acceptance Criteria**:
- *Given* an item in `changes_requested` state (non-empty `plan_rejection_reason`), *When* the
  user sends it back to `idea`/`refining` (`TransitionBacklogItemStatus`), *Then*
  `plan_rejection_reason` is cleared to `""` in the same update that already clears
  `plan_approved`/`plan_artifacts_path`.

**Files**: `server/services/backlog_service_lifecycle.go`, `server/services/backlog_service_test.go`.

##### Task 3.3.1a: Extend the existing reset block **[PORT]** (~3 min)
- Extend the reset block at current line 595 (`if to == session.BacklogStatusIdea || to ==
  session.BacklogStatusRefining { ... }`) to also clear `PlanRejectionReason`:
  ```go
  planApproved := false
  planArtifactsPath := ""
  rejectionReason := ""
  update := session.BacklogItemUpdate{
      PlanApproved:        &planApproved,
      PlanArtifactsPath:   &planArtifactsPath,
      PlanRejectionReason: &rejectionReason,
  }
  ```
- Files: `server/services/backlog_service_lifecycle.go`

##### Task 3.3.1b: Regression test (~4 min)
- Add `TestTransitionBacklogItemStatus_SendBackToIdea_ClearsRejectionReason`: reject a plan,
  transition to `idea`, assert `plan_rejection_reason == ""`.
- Files: `server/services/backlog_service_test.go`

##### Task 3.3.1c: Full backend validation (~5 min, terminal)
- Run `make build && go test ./server/services/... ./session/...` — full regression pass for
  Phase 3 before Phase 4's frontend depends on it.

---

## Phase 4: Gap 3b — Frontend Plan Review UI (new, from `plan-approval-ux` spec, scoped)

### Epic 4.1: Status derivation + hook wiring

**Goal**: A single, canonical function computes the 5-state plan-review status; the frontend
`BacklogItem` type carries the two new fields.

#### Story 4.1.1: `derivePlanReviewStatus`

**Acceptance Criteria** (part of AC5 — a state distinguishable from both "approved" and "never
reviewed"):
- *Given* an item with `skipPlanning=true` and no plan artifacts, *When*
  `derivePlanReviewStatus(item)` is called, *Then* it returns `"skipped"`, not `"no_plan"`.
- *Given* an item with `planRejectionReason` set (non-empty), *When*
  `derivePlanReviewStatus(item)` is called, *Then* it returns `"changes_requested"` regardless
  of `planApproved`'s value (defensive — should never coexist post-symmetry-fix, but the
  derivation must still resolve correctly if it ever does).

**Files**: `web-app/src/lib/backlog/planReviewStatus.ts` (new),
`web-app/src/lib/backlog/planReviewStatus.test.ts` (new),
`web-app/src/lib/hooks/useBacklogService.ts`.

##### Task 4.1.1a: `derivePlanReviewStatus` **[NEW — adapted from plan-approval-ux spec]** (~4 min)
- Create `web-app/src/lib/backlog/planReviewStatus.ts`:
  ```ts
  import type { BacklogItem } from "@/lib/hooks/useBacklogService";

  export type PlanReviewStatus =
    | "no_plan"
    | "pending_review"
    | "approved"
    | "changes_requested"
    | "skipped";

  /**
   * Single source of truth for the 5-state plan-review status — never
   * persisted server-side, always derived. See
   * project_plans/plan-approval-ux/decisions/ADR-001.
   */
  export function derivePlanReviewStatus(
    item: Pick<BacklogItem, "skipPlanning" | "planApproved" | "planArtifactsPath" | "planRejectionReason">,
  ): PlanReviewStatus {
    if (item.skipPlanning) return "skipped";
    if (item.planRejectionReason) return "changes_requested";
    if (item.planApproved) return "approved";
    if (item.planArtifactsPath) return "pending_review";
    return "no_plan";
  }
  ```
- Files: `web-app/src/lib/backlog/planReviewStatus.ts`

##### Task 4.1.1b: Unit tests (~3 min)
- One test per branch: `skipped` wins over everything (including a non-empty rejection
  reason); `changes_requested` wins over `approved` (defensive case above);
  `pending_review`; `no_plan`.
- Files: `web-app/src/lib/backlog/planReviewStatus.test.ts`

##### Task 4.1.1c: Extend `BacklogItem` TS type + `mapBacklogItem` (~3 min)
- Add `planRejectionReason?: string;` and `planRejectedAt?: string;` to the `BacklogItem`
  interface (alongside `planApproved`/`planArtifactsPath`, current lines 108-109), and map them
  in `mapBacklogItem` (alongside the existing `planApproved`/`planArtifactsPath` mappings,
  current lines 471-472):
  ```ts
  planRejectionReason: p.planRejectionReason || undefined,
  planRejectedAt: p.planRejectedAt ? new Date(Number(p.planRejectedAt.seconds) * 1000).toISOString() : undefined,
  ```
- Files: `web-app/src/lib/hooks/useBacklogService.ts`

##### Task 4.1.1d: `rejectPlan` hook method (~3 min)
- Add `rejectPlan`, mirroring the existing `approvePlan` (current line 845) shape:
  ```ts
  const rejectPlan = useCallback(async (id: string, reason: string): Promise<BacklogItem | null> => {
    if (!clientRef.current) return null;
    try {
      const resp = await clientRef.current.rejectPlan({ itemId: id, reason });
      return resp.item ? mapBacklogItem(resp.item) : null;
    } catch (err) {
      console.error("[useBacklogService] rejectPlan:", err);
      setLastError(err instanceof Error ? err : new Error(String(err)));
      throw err;
    }
  }, []);
  ```
  Export it from the hook's returned object alongside `approvePlan`.
- Files: `web-app/src/lib/hooks/useBacklogService.ts`

---

### Epic 4.2: `PlanVerdictBox` component

#### Story 4.2.1: Status card + reject-with-reason form

**Acceptance Criteria** (AC3, AC4, AC5):
- *Given* a `ready`-status item with a pending-review plan, *When* the item detail view
  renders, *Then* a persistent status card reads "Pending review", visible alongside an
  Approve action (existing, in `ActionsSection`) and a "Request Changes" action (new, in
  `PlanVerdictBox`) — **both present in the same place** (AC3).
  - Confirms AC5's second half: this state is visible in the item detail view, not inferred.
- *Given* the same item, *When* the operator clicks "Request Changes" without typing anything,
  *Then* the Submit button is disabled (`aria-disabled`+`disabled`) — an empty rejection cannot
  be submitted from the UI, matching the server-side guard from Task 3.2.1c (AC4, defense in
  depth).
- *Given* the operator types a non-empty reason and clicks Submit, *When* the call succeeds,
  *Then* the card updates to "Revisions requested" (distinct copy per P9), the reason text
  renders read-only below it, and a "Regenerate Plan with This Feedback" button appears — a
  state visually and lexically distinct from both "Plan approved" and "Pending review" (AC5).

**Files**: `web-app/src/components/backlog/PlanVerdictBox.tsx` (new),
`web-app/src/components/backlog/PlanVerdictBox.css.ts` (new),
`web-app/src/components/backlog/PlanVerdictBox.test.tsx` (new).

##### Task 4.2.1a: Card styles **[NEW — adapted from `GateVerdictBox.css.ts`]** (~5 min)
- Create `PlanVerdictBox.css.ts`: 5 variants reusing `vars.*` tokens, modeled on
  `GateVerdictBox.css.ts`'s existing PASS/PARTIAL/FAIL/PENDING/UNVERIFIABLE color pattern:
  `approved` → pass-green, `pending_review` → pending-blue, `changes_requested` → a color
  **distinct from `MergeabilityPill`'s own "changes requested" pill color** (P9 — check
  `MergeabilityPill.css.ts`'s existing palette choice before picking this one), `no_plan` →
  unverifiable-grey, `skipped` → a new neutral variant (distinct border/icon, "intentionally
  bypassed" is semantically distinct from all 5 existing verdict meanings).
- Files: `web-app/src/components/backlog/PlanVerdictBox.css.ts`

##### Task 4.2.1b: Read-only status card render **[NEW]** (~5 min)
- Create `PlanVerdictBox.tsx`. Icon+label per state, never color-only (a11y):
  ```tsx
  const STATUS_CONFIG: Record<PlanReviewStatus, { icon: string; label: string }> = {
    no_plan: { icon: "○", label: "No plan yet" },
    pending_review: { icon: "◌", label: "Pending review" },
    approved: { icon: "✓", label: "Plan approved" },
    changes_requested: { icon: "✎", label: "Revisions requested" },
    skipped: { icon: "⊘", label: "Planning skipped" },
  };
  ```
  `role="status" aria-live="polite" aria-atomic="true"` on the section root. Props: `status:
  PlanReviewStatus`, `rejectionReason?: string`, `readOnly?: boolean`, `onReject?: (reason:
  string) => Promise<void>`, `onRegenerateWithFeedback?: () => Promise<void>`,
  `actionPending?: boolean`. **No approve action here** — `ActionsSection` keeps that button
  (avoid duplicating the same action in two places).
- Files: `web-app/src/components/backlog/PlanVerdictBox.tsx`

##### Task 4.2.1c: Request Changes form **[NEW — mirrors `GateVerdictBox`'s reopen-form shape]** (~5 min)
- `<button data-testid="backlog-action-reject-plan">` toggling a `<textarea
  data-testid="plan-reject-reason">`, focus-on-open via `useEffect`, Cancel returns focus to
  the toggle, Submit (`data-testid="backlog-action-reject-plan-submit"`) `aria-disabled`+
  `disabled` while `reason.trim()` is empty, guard enforced in the click handler too (not just
  the attribute). Reuse `GateVerdictBox.tsx`'s exact existing toggle/focus/aria-expanded
  pattern rather than reinventing it.
- Files: `web-app/src/components/backlog/PlanVerdictBox.tsx`

##### Task 4.2.1d: "Regenerate Plan with This Feedback" CTA **[PORT — ADR-002 pattern]** (~3 min)
- When `status === "changes_requested"`, render the persisted reason text (read-only) plus
  `<button data-testid="backlog-action-regenerate-plan">` calling `onRegenerateWithFeedback` —
  per ADR-002, a visibly distinct second action, not a side effect of the reject submit.
- Files: `web-app/src/components/backlog/PlanVerdictBox.tsx`

##### Task 4.2.1e: Component tests (~5 min)
- RTL: renders correct icon+label per status (5 cases); reject submit disabled until non-empty
  text (both attribute and click-handler guard); "Regenerate" button only appears in
  `changes_requested` state; `role="status" aria-live="polite"` present; Cancel returns focus
  to the toggle button.
- Files: `web-app/src/components/backlog/PlanVerdictBox.test.tsx`

---

### Epic 4.3: Wire `PlanVerdictBox` into `BacklogItemDetail`; reuse `derivePlanReviewStatus` in the spawn gate

#### Story 4.3.1: End-to-end wiring

**Acceptance Criteria** (AC3, AC5 combined):
- *Given* a `ready`-status item, *When* the item detail view renders, *Then* `PlanVerdictBox`
  is visible alongside `ActionsSection`'s existing Approve button (both reachable without
  navigating away — no vanishing button), and clicking Request Changes → typing a reason →
  Submit calls `rejectPlan(item.id, reason)` and the card updates in place.

**Files**: `web-app/src/components/backlog/BacklogItemDetail.tsx`,
`web-app/src/components/backlog/detail/ActionsSection.tsx`,
`web-app/src/components/backlog/BacklogItemDetail.test.tsx`.

##### Task 4.3.1a: `handleRejectPlan`/`handleRegeneratePlanWithFeedback` callbacks **[NEW]** (~5 min)
- In `BacklogItemDetail.tsx`, near the existing action-handler callbacks:
  ```ts
  const handleRejectPlan = useCallback(async (reason: string) => {
    if (!item) return;
    const toastKey = `${item.id}:reject_plan`;
    setActionLoading("reject_plan");
    try {
      await rejectPlan(item.id, reason);
      showActionToast("Revisions requested.", "success", toastKey);
      await load();
    } catch (e) {
      showActionToast(e instanceof Error ? e.message : "Reject failed.", "error", toastKey);
      throw e;
    } finally {
      if (mountedRef.current) setActionLoading(null);
    }
  }, [item, rejectPlan, load, showActionToast]);

  const handleRegeneratePlanWithFeedback = useCallback(async () => {
    if (!item?.planRejectionReason) return;
    await triggerTriage(item.id, item.planRejectionReason);
    await load();
  }, [item, triggerTriage, load]);
  ```
  Destructure `rejectPlan` from the existing `useBacklogService()` call (alongside
  `triggerTriage`/`approvePlan`, current line ~85-98).
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 4.3.1b: Render `PlanVerdictBox` above `ActionsSection`, below plan-artifacts display (~4 min)
- Insert `<PlanVerdictBox>` immediately before the existing `<ActionsSection>` render, gated on
  the item having ever had a plan or being in a status where planning matters:
  ```tsx
  {(item.status === "ready" || item.status === "queued" || derivePlanReviewStatus(item) !== "no_plan") && (
    <PlanVerdictBox
      status={derivePlanReviewStatus(item)}
      rejectionReason={item.planRejectionReason}
      readOnly={terminalState !== null}
      actionPending={actionLoading === "reject_plan"}
      onReject={handleRejectPlan}
      onRegenerateWithFeedback={handleRegeneratePlanWithFeedback}
    />
  )}
  ```
  Placed after the existing (unchanged) `<code>{item.planArtifactsPath}</code>` plan-artifacts
  display — content-before-verdict ordering, matching the precedent
  `plan-approval-ux/research/ux.md` §7.2 established (comparable review tools never put an
  approve/reject action above content the user hasn't seen — here "content" is just the path
  string, since §0 deferred in-browser rendering, but the ordering principle still applies).
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 4.3.1c: `ActionsSection` reuses `derivePlanReviewStatus` for its spawn gate (P11) (~3 min)
- Replace the raw `item.skipPlanning || item.planApproved` check (current line 70) with:
  ```ts
  import { derivePlanReviewStatus } from "@/lib/backlog/planReviewStatus";
  // ...
  const planStatus = derivePlanReviewStatus(item);
  const canSpawnSession = actions.has("spawn_session") && (planStatus === "skipped" || planStatus === "approved");
  ```
  **Do not** change the button's `title`/copy — pure internal-logic refactor, behaviorally
  identical for every case that mattered before (a rejected plan was never approved, so it was
  already blocking spawn; adding `changes_requested` doesn't loosen or tighten the gate).
- Files: `web-app/src/components/backlog/detail/ActionsSection.tsx`

##### Task 4.3.1d: Integration test (~5 min)
- Extend `BacklogItemDetail.test.tsx`: render with a `ready`-status item and a pending-review
  plan, assert `PlanVerdictBox` shows "Pending review" and `ActionsSection`'s Approve button is
  simultaneously visible (AC3's "both in the same place"); click Request Changes → type reason
  → Submit; assert the mocked `rejectPlan` client call fires with the typed reason and the card
  re-renders "Revisions requested" with the reason text visible and a "Regenerate" button
  present.
- Files: `web-app/src/components/backlog/BacklogItemDetail.test.tsx`

##### Task 4.3.1e: Frontend registry entry (~2 min)
- Create `docs/registry/features/frontend/plan-verdict-box.json`:
  `id: "backlog-plan-verdict-box"`, `component: "PlanVerdictBox"`, `path:
  "web-app/src/components/backlog/PlanVerdictBox.tsx"`, `testIds` from Task 4.2.1e. Requires
  adding `// +feature: backlog-plan-verdict-box` in the component's first 10 lines (part of
  Task 4.2.1b).
- Files: `docs/registry/features/frontend/plan-verdict-box.json`

---

## Phase 5: Registry & E2E Coverage (AC8, cross-cutting)

### Epic 5.1: E2E specs for AC1, AC3, AC6

**Acceptance Criteria** (AC8):
- Each of criteria 1, 3, and 6 has a Playwright e2e test in `tests/e2e/`, following
  `.claude/rules/e2e-test-conventions.md` (`@feature` header, no `waitForTimeout`,
  `data-testid`/ARIA locators only, new page helpers under `tests/e2e/pages/`), and the
  new/changed RPCs and components are registered per `.claude/rules/feature-registry.md`.

**Files**: `tests/e2e/triage-question-answer.spec.ts` (new),
`tests/e2e/plan-review.spec.ts` (new), `tests/e2e/backlog-session-steer.spec.ts` (new),
`tests/e2e/pages/` (as needed).

#### Story 5.1.1: AC1 e2e — answer a triage question

##### Task 5.1.1a: Spec file (~5 min)
```ts
// @feature backlog:trigger-triage, triage-question-answer
```
Test: seed/trigger a triage run producing ≥1 clarifying question, open the item detail view,
locate the question row via `getByTestId("triage-question-answer-toggle-0")`, click it, fill
`getByTestId("triage-question-answer-input-0")`, click
`getByTestId("triage-question-answer-submit-0")`, assert the row transitions to the
"✓ Answered" state (`expect(locator).toHaveText(...)`, no `waitForTimeout` — poll via
`expect(...).toBeVisible()`), and assert a re-triage request fired (mock/intercept the
`TriggerTriage` RPC, or assert on a subsequent triage-result iteration bump if the test drives
a real backend).
- Files: `tests/e2e/triage-question-answer.spec.ts`

#### Story 5.1.2: AC3 e2e — Approve + Request Changes side by side

##### Task 5.1.2a: Spec file (~5 min)
```ts
// @feature backlog:reject-plan, backlog-plan-verdict-box
```
Test: create/triage a `ready`-status item with `plan_artifacts_path` set, open detail, assert
both `getByTestId("backlog-action-spawn-session")` (or the existing Approve action's testid —
confirm exact id in `ActionsSection.tsx` at implementation time) and
`getByTestId("backlog-action-reject-plan")` are simultaneously visible, click reject → type
reason → submit, assert status updates to "Revisions requested" and the reason text is
visible, assert "Regenerate Plan with This Feedback" button appears.
- Files: `tests/e2e/plan-review.spec.ts`

#### Story 5.1.3: AC6 e2e — steer a work session, absent for triage

##### Task 5.1.3a: Spec file (~5 min)
```ts
// @feature session:update, backlog-session-steer
```
Test: spawn or seed a backlog item with an active work session, open detail, assert
`getByTestId("session-steer-toggle-<sessionId>")` is visible and enabled, click it, fill
`getByTestId("session-steer-input-<sessionId>")`, click
`getByTestId("session-steer-submit-<sessionId>")`, assert a success toast/state. **Second
assertion in the same file** (regression-tests ADR-002's narrowing): for a `headless-`-prefixed
triage session row on the same or a second item, assert no `session-steer-toggle-*` element
exists at all (`expect(locator).toHaveCount(0)`, not just `not.toBeVisible()`).
- Files: `tests/e2e/backlog-session-steer.spec.ts`

---

### Epic 5.2: Final registry generation + full validation

##### Task 5.2.1: `make registry-generate` and gap check (~3 min, terminal)
- Run `make registry-generate`, diff `docs/registry/coverage-gaps.json` against its pre-change
  state, confirm no net increase in untested features. Any new entry from this feature that
  appears in the gap list must get its missing test added before this epic is done.

##### Task 5.2.2: Full validation gate (~5 min, terminal)
- Run `make build && make test && make lint` (backend) and `cd web-app && npx jest
  --no-coverage` (frontend) as the final gate before handing this plan to `/sdd:4-validate`.

##### Task 5.2.3: E2E regression run (~5 min, terminal)
- Run `cd tests/e2e && npx playwright test triage-question-answer.spec.ts plan-review.spec.ts
  backlog-session-steer.spec.ts` plus the existing `plan-gate.spec.ts` (regression check — this
  project's `ActionsSection` refactor in Task 4.3.1c must not change its existing button
  copy/testids, confirmed here rather than assumed).

---

## Summary

- **5 phases / 5 epics** (Gap 1 = Epic 1; Gap 2 = Epic 2, split backend/frontend; Gap 3 = Epics
  3-4, split backend-port/frontend-new; cross-cutting registry+e2e = Epic 5).
- **13 stories**, **~50 tasks**, each scoped to 2-5 minutes and 1-5 files.
- **14 domain glossary terms.**
- **4 ADRs referenced** (this project's ADR-001/002 for Gap 2; `plan-approval-ux`'s ADR-001/002
  for Gap 3), **0 new ADRs written** — see §6 below.
- **11 pattern decisions**, each with a rejected alternative and reason.
- Gap 3's `GetPlanArtifactContent` RPC, `expected_modified_at_unix_ms` optimistic-concurrency
  token, `plan_artifacts_set_at` field, and the widened stuck-item reconciler are **explicitly
  deferred** (§0) — no AC requires them, and pulling them in would violate this repo's YAGNI
  convention for zero acceptance-criterion benefit.

## New ADR needed?

**No.** Every architecture decision this plan makes was already resolved by one of the four
referenced ADRs (this project's own ADR-001/ADR-002 for Gap 2, `plan-approval-ux`'s ADR-001/
ADR-002 for Gap 3) or is a small, self-contained scope call recorded in the Pattern Decisions
table (P8's Gap-3 trim, P9's naming-collision copy fix) that doesn't rise to ADR weight — none
of them are hard to reverse, none involve a genuine architectural fork this plan is choosing
between. Gap 1 needed no ADR in research either (pure frontend composition on an unchanged
backend field).
