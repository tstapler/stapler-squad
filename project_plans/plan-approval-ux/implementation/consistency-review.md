# Cross-Artifact Consistency Check: plan-approval-ux (Phase 4)

**Date**: 2026-08-01
**Verdict**: RESOLVED — all 3 BLOCKERs patched into `plan.md` (see §10 "Phase 4 Patches Applied"). Concerns/Nitpicks below remain open, tracked for a future pass.

Read in full: `requirements.md`, `implementation/plan.md`, `design/ux.md`.

---

## 1. Coverage Gaps (requirements → plan)

All four **Must Have** items have ≥1 story (status indicator → Epic 5; reject/request-changes → Epic 3+6; in-app rendering → Epic 7; research phase → already completed in Phase 2). No Must-Have is uncovered.

| Finding | Artifacts | Severity | Resolution |
|---|---|---|---|
| Success Criterion 5 ("line-level feedback capability") has zero corresponding story — explicitly deferred via P6/Unresolved Question 2 | requirements.md ↔ plan.md | **CONCERN** (not BLOCKER — requirements.md itself pre-authorizes deferral) | Track as a named follow-up project before closing this one out. |
| Should-Have "approval/rejection history visible in the item's timeline" is delivered only as *most-recent-reason* visibility, not append-only history | requirements.md ↔ plan.md | **CONCERN** | Accept reduced scope explicitly, or scope a follow-up story. |

## 2. Scope Drift

Epic 8 (widened stuck-item detection) traces cleanly to Success Criterion 2 / the Should-Have's "or an explicit documented decision not to" clause — P8's decision table *is* that documented decision. **Not drift.**

The `expected_modified_at_unix_ms` mechanism (Epic 3/4, P5) traces to requirements.md's Pitfalls research dimension. **Not drift** — grounded, was under-executed (see §5, now fixed).

No unjustified scope-creep found.

## 3. UX-Plan Misalignment

| Finding | Severity | Resolution |
|---|---|---|
| ux.md AC-2.3 recommends `.focus()`-on-Cancel for the reject form (WCAG 2.4.3) — no plan.md task implements it | **NITPICK** | Add a one-line task to Epic 5, or accept as known a11y debt. |
| Epic 8's notification copy change (Task 8.1.2) has no ux.md design coverage | **NITPICK** | Low-stakes string change — note and move on. |

## 4. Terminology Drift

| Finding | Severity | Resolution |
|---|---|---|
| requirements.md Success Criterion 1 names only 4 states; plan.md/ux.md implement a 5th (`skipped`) | **NITPICK** | Update requirements.md's prose to mention the 5th state. |
| ux.md §7.6 flags "unclear whether `plan_rejection_reason` is cleared on regeneration" as open — plan.md's Task 2.4.1 already resolves this | **CONCERN** (doc staleness only) | Strike/annotate ux.md §7.6 as resolved by Task 2.4.1. |

## 5. Direct Contradictions (BLOCKERs — all resolved this pass)

| Finding | Resolution Applied |
|---|---|
| Plan's Risk Control table (§6) claimed the stale-tab race was mitigated via `expected_modified_at_unix_ms`, but Epic 6/7's actual task-level wiring never threaded the token from fetch to Approve/Reject — dead-code safeguard. | Fixed: mtime lifted into `BacklogItemDetail` state (Task 6.1.2), `approvePlan` hook extended (Task 6.1.2b), `PlanArtifactsSection` reports mtime via new `onMtimeChange` prop (Task 7.1.3), regression test added (Task 6.1.5). |
| Task 6.1.3 placed `PlanVerdictBox`/`ActionsSection` **above** `PlanArtifactsSection` in DOM order, contradicting the cited UX research (never hide plan content behind/below the approve action). | Fixed: Task 6.1.3 now renders `PlanArtifactsSection` first, then `PlanVerdictBox`, before `ActionsSection`; Task 7.1.4 marked superseded for placement. |
| Task 7.1.3's code sample called `<InlineNotice actionLabel=... onAction=... />`, but the real component's props are `actions: InlineNoticeAction[]` — would not compile. | Fixed in place with the correct `actions={[{ label, onClick, variant }]}` shape. |

---

**Original summary**: 10 findings — 3 blockers, 3 concerns, 4 nitpicks. All 3 blockers resolved via direct `plan.md` patches during Phase 4 (before the implementation readiness gate). Remaining concerns/nitpicks are non-blocking and left for a future pass or explicit scope sign-off.
