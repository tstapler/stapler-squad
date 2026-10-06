# Build vs. Buy / Revive vs. Rebuild — Agent 6

Scope note: this is primarily a **revive vs. rebuild** question for Gap 3 (prior art exists),
plus two standard build-vs-buy calls for Gaps 1/2 (no prior art, pure app wiring). All findings
below are VERIFIED by running the actual commands, not inferred from reading commit messages.

## 1. Gap 3: rebase/cherry-pick `bc0955d41` onto current main

**Staleness (VERIFIED)**:
- `git log --oneline main..recover/plan-approval-ux` → exactly 1 commit: `bc0955d41`.
- `git log --oneline recover/plan-approval-ux..main | wc -l` → **111** commits of drift.
- `git show bc0955d41 --stat` → 17 files, +1693/-48. Backend-only: proto, 2 service files,
  ent-generated code (schema/create/update/where — mechanically regenerable), repository glue,
  and tests.

**Actual cherry-pick attempt (VERIFIED — ran `git cherry-pick --no-commit bc0955d41` in a
disposable detached worktree off current `main`, then discarded it)**:
- 16 of 17 files auto-merged cleanly.
- **Exactly 1 conflict**, in `server/services/backlog_service_triage.go`, a single ~14-line
  hunk. Main had independently added SDD-pipeline-mode-aware `pap` (plan-artifacts-path)
  computation in the same `TriggerTriage` block the old branch touched; the old branch's
  literal `update := session.BacklogItemUpdate{PlanArtifactsPath: &pap, ...}` needs to be
  folded together with main's new `if item.PipelineMode == ...` branch. This is a mechanical,
  low-risk 3-way merge — both sides touch the same `update` struct literal for unrelated
  reasons (SDD-mode path vs. new rejection fields), not the same logic.
- No conflicts in the ent-generated files despite `session/backlog_lifecycle.go` having been
  split into 5 files by a later refactor (`caf68a8a6`, "split backlog_lifecycle.go by concern")
  — git's rename/hunk-tracking handled it because bc0955d41's edits to that file were additive
  and in a region the split didn't restructure.

**Does the field surface still exist, or was it independently reimplemented?** Checked directly
(VERIFIED via grep on current main):
- `plan_artifacts_path` / `PlanArtifactsPath` — **already exists on main**, independently of
  this branch (it predates `bc0955d41`'s parent commit; the branch's own diff to the ent schema
  only adds `plan_rejection_reason`, `plan_rejected_at`, `plan_artifacts_set_at` — not this
  field). A separate, already-merged PR (`ed0fda703`, "stop plan-approval UI flicker on stuck
  items and item detail", #386) built real UI on top of `plan_artifacts_path` for the *existing*
  Approve-only flow — confirming the field is live and depended-on, not dead.
- `RejectPlan`, `GetPlanArtifactContent`, `PlanRejectionReason`, `PlanRejectedAt` — **zero
  matches anywhere in the tree**. These are genuinely still missing; nothing else has
  reimplemented this surface in the 111-commit gap.

**Critical scope gap — the orphaned commit is backend-only, not full-stack**: reading
`project_plans/plan-approval-ux/implementation/plan.md`'s own Epic breakdown (Epics 1–9)
against `bc0955d41`'s diff shows the commit implements Epics 1–4 (approval-reset correctness
fix, plan-rejection data model, `RejectPlan` RPC, `GetPlanArtifactContent` RPC) and part of
Epic 8 (widened stuck detection). **Epics 5–7 — the frontend `PlanVerdictBox` component, its
wiring into `BacklogItemDetail`, and plan-content rendering — and Epic 9 (registry + e2e) were
never implemented.** `grep -rl "rejectPlan\|RequestChanges" web-app/src` returns nothing. So
even a clean rebase only delivers roughly half of Gap 3's original scope; the current
requirements' AC3–5 (Approve/Request-Changes side by side, required rejection text, a visibly
distinct status) still need frontend implementation regardless of which path (1) or (2) is
chosen below.

### Pros (rebase)
- Skips re-deriving and re-reviewing the hardest part of Gap 3: the state-machine design
  (which fields, which RPC, optimistic-concurrency token via
  `expected_modified_at_unix_ms`/`checkPlanArtifactFreshness`) — this is exactly the "genuinely
  open state-machine design question" the requirements doc flags as the reason for `/sdd:full`.
  That design work is already done, already ADR'd (`ADR-001-plan-review-state-durable-fields.md`,
  `ADR-002-reject-plan-manual-retrigger.md`), and already has a documented adversarial/
  architecture review pass (`implementation/adversarial-review.md`,
  `implementation/architecture-review.md`).
- The one real conflict is small and mechanical — verified directly, not estimated.
- Backend tests ship with the commit (`backlog_service_test.go` +465 lines,
  `backlog_lifecycle_test.go` +170 lines) — a rebase inherits real test coverage, not just
  production code.
- Full validation.md/pre-mortem.md already exist for the whole feature (backend + frontend),
  reusable as reference even for the parts to be freshly written.

### Cons (rebase)
- Only recovers ~50% of Gap 3 (backend, Epics 1–4/8). Frontend (Epics 5–7, 9) must be
  implemented fresh either way — so the "revive" saves real time but doesn't eliminate new
  implementation work or its own review cycle.
- 111 commits of drift means the *rest* of the file (outside the touched hunks) has moved a lot;
  the auto-merge succeeding cleanly on 16/17 files is a good signal but not a substitute for
  running `make build && make test` after resolving — ent-generated code in particular
  (`session/ent/backlogitem_create.go`, `_update.go`, `backlogitem/where.go`) should be
  regenerated via the correct `--feature sql/upsert` command
  (`.claude/rules/ent-schema-generation.md`) rather than trusted as merged, since hand-merged
  generated code is a known footgun.
- The old plan's requirements/ACs were written against an earlier version of this ticket's
  scope (it's `plan-approval-ux`, a narrower predecessor project, not `backlog-operator-
  feedback-loop`) — Phase 3 planning still needs to re-map `bc0955d41`'s RPCs against the
  *current* ACs (1–8 in this project's requirements.md) rather than assume 1:1 coverage.

### Verdict: **Rebase for the backend half, treat the frontend half as fresh implementation using the old plan as reference material.** This is not a binary choice — it is a hybrid:
1. Cherry-pick/rebase `bc0955d41` onto current main (single small conflict, mechanical fix,
   verified above). Regenerate ent code properly rather than trust the merged generated files.
2. Re-run Phase 3 planning for Gap 3's frontend against the *existing* `PlanVerdictBox`
   design in `project_plans/plan-approval-ux/implementation/plan.md` (Epics 5–7) as a strong
   reference/starting draft, adjusted for whatever this project's ADR phase decides about
   question 1 (reuse `feedback` field vs. dedicated RPC — already answered by ADR-002 in the
   old plan, worth citing/ratifying rather than re-litigating from scratch).

Full re-plan-from-scratch (option 2 in the task framing) is **not** recommended: the diff size
is small, the one conflict is trivial, the state-machine ADRs are sound and unchallenged by the
111 commits of drift (none of the intervening commits touch plan-approval semantics except the
UI-flicker fix, which is compatible/additive), and discarding a working, tested backend
implementation to re-derive the same design from the same reference docs would be pure waste.

## 2. Gap 1 and Gap 2: standard build-vs-buy (no prior art)

Both are pure internal app-wiring problems with no external library need — confirmed by reading
the actual reference components, not just their names in the requirements doc.

### Gap 1 — per-question answer field in Triage Questions section
- Reference pattern: `web-app/src/components/backlog/TriageReviewPanel.tsx` (371 lines) already
  has the exact shape needed — a `readOnly` vs. write-mode discriminated prop pair
  (`TriageReviewPanelReadOnlyProps` / `TriageReviewPanelWriteProps`), an `onRefine: (feedback:
  string) => Promise<void>` callback, and local `useState` for the textarea content. Adding a
  per-question answer field to `TriageDiffSection.tsx` is the same shape: local state per
  question, a submit callback that calls the existing `triggerTriage(id, feedback)` hook
  (`useBacklogService.ts:589,822`) — no new RPC, no new abstraction.
- **Verdict: fork the existing pattern.** No new library, no new backend surface (per
  requirements' explicit out-of-scope: "Any change to `TriggerTriageRequest.feedback`
  semantics").

### Gap 2 — Steer affordance in backlog item's `SessionsSection.tsx`
- Reference pattern: `web-app/src/components/sessions/SessionActionsOverflow.tsx`'s Steer
  dialog — verified directly: `isSteerOpen`/`steerMessage` local state, a `useFocusTrap` hook,
  a `createPortal`-rendered dialog (`renameDialog`/`dialogContent` styles), calling the existing
  `steer_session` MCP tool path. `SessionsSection.tsx` currently renders sessions as plain
  `<a>` tags with no overflow menu at all — the gap is wiring `SessionActionsOverflow` (or its
  Steer-only subset) into that list, not building a new steering mechanism.
- **Verdict: reuse `SessionActionsOverflow` directly** rather than duplicating its dialog markup
  — this is exactly the "no parallel steering implementation" constraint in AC7. If
  `SessionActionsOverflow` currently assumes a full `Session` object with more context than
  `SessionsSection.tsx`'s linked-session summary carries, that's a component-props question for
  Phase 3, not a build-vs-buy question — still zero new external dependencies either way.

## 3. LLM-generated code vs. tested pattern — is there a nontrivial algorithm anywhere?

No. Every piece of new work identified above is CRUD/RPC wiring:
- Gap 1: local textarea state → existing `triggerTriage(id, feedback)` call.
- Gap 2: local dialog state → existing `steer_session` MCP tool call (already proven to work
  from `SessionRow.tsx`/`SessionCard.tsx`/`PaneHeader.tsx`).
- Gap 3 backend (already written in `bc0955d41`): a status-field state transition with an
  optimistic-concurrency timestamp check (`checkPlanArtifactFreshness`) — not an algorithm in
  the sense of nontrivial computation, but it *is* the one place with real design risk (state
  machine correctness), which is exactly why it already has two ADRs and an adversarial review
  pass rather than being treated as routine wiring.
- Gap 3 frontend (still to build): a status-derivation function (`PlanVerdictBox`'s "Story 5.1
  — Status derivation") mapping the persisted fields to a UI state — small, pure, testable
  logic, not an algorithm.

Per `.claude/rules/interface-pollution-checklist.md` and
`.claude/rules/primitive-obsession-checklist.md`: none of this wiring justifies a new
interface, a new `Manager`/`Service` type, or a generic abstraction. Plain functions/components
calling existing hooks and RPCs are the correct shape throughout — the one place a real
abstraction already exists and should be reused as-is (not re-abstracted) is `RepoRef`/
`AccountRef`-style typed identifiers if any new function ends up threading multiple same-typed
string/id parameters (none of the surfaces above currently need this, based on the reference
files read).

## Summary

| Gap | Prior art? | Decision |
|---|---|---|
| 1 (triage Q&A) | No | Fork `TriageReviewPanel.tsx`'s write-mode pattern; no new RPC |
| 2 (steer from backlog) | No | Reuse `SessionActionsOverflow` directly; no new dialog |
| 3 backend (reject plan) | Yes — `bc0955d41` | **Rebase** (1 trivial conflict, verified) |
| 3 frontend (reject UI) | Partial — plan.md Epics 5-7 designed, never built | Build fresh, plan.md as reference |
