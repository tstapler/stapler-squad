# UX Design: Plan Approval UX

**Phase**: 3 — Design | **Project**: `plan-approval-ux`
**Inputs**: `requirements.md`, `research/ux.md`, `implementation/plan.md` Epics 5–7
**Grounded in existing codebase patterns**: `GateVerdictBox.tsx`, `BlockerChip.tsx`,
`InlineNotice.tsx`, `InlineError.tsx`, `ActionsSection.tsx`, `DescriptionSection.tsx`

This document designs the concrete layout, interaction flow, and acceptance criteria for
every user-facing surface introduced by Epics 5–7 of the implementation plan. It does not
relitigate component/prop shapes already decided there (`PlanVerdictBox`'s 5-state model,
`derivePlanReviewStatus`, the reject-with-reason + regenerate two-button flow,
`PlanArtifactsSection`'s fetch+render conversion) — it designs *around* them.

---

## Surfaces designed (5)

1. `PlanVerdictBox` card — 5 states
2. Reject-with-reason inline form (nested inside surface 1)
3. Rendered plan-content viewer inside `PlanArtifactsSection`
4. "Newer plan available" stale-content notice (nested inside surface 3)
5. Error states (missing plan file / failed RPC / empty rejection reason / stale-token
   conflict on submit) — cross-cutting, documented per-surface below plus in its own
   consolidated table (§6)

---

## 1. `PlanVerdictBox` card (5 states)

### Wireframe — read/write mode, `pending_review` state (the common case)

```
┌─ role="status" aria-live="polite" aria-label (implicit via chip content) ──────┐
│  ◌  Pending review                                                             │
│                                                                                  │
│  [ Request Changes ▸ ]                                                         │
└──────────────────────────────────────────────────────────────────────────────┘
```

### Wireframe — `approved` state

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  ✓  Plan approved                                                              │
│                                                                                  │
│  (no action buttons here — Approve/re-approve stays on ActionsSection          │
│   per Task 5.2.2's explicit "no approve action here, avoid duplicating")       │
└──────────────────────────────────────────────────────────────────────────────┘
```

### Wireframe — `changes_requested` state (reason persisted + regenerate CTA)

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  ✎  Changes requested                                                          │
│                                                                                  │
│  "The caching case isn't addressed — plan needs a section on invalidation     │
│   before I'll approve it."                                                     │
│                                    ↑ persisted plan_rejection_reason, read-only │
│                                                                                  │
│  [ Regenerate Plan with This Feedback ]                                        │
│         data-testid="backlog-action-regenerate-plan"                          │
└──────────────────────────────────────────────────────────────────────────────┘
```

### Wireframe — `no_plan` / `skipped` states (compact, no actions)

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  ○  No plan yet                              │  ⊘  Planning skipped           │
└────────────────────────────────────────────────────────────────────────────────┘
```

### Interaction flow

| Step | User does | System responds |
|---|---|---|
| 1 | Opens a `ready`/`queued` item, or any item that has ever had a plan | `PlanVerdictBox` renders immediately above `ActionsSection`, showing the current `derivePlanReviewStatus(item)` state — never a vanishing button (Success Criterion 1). Icon + text label always paired (`BlockerChip.tsx:16-18` "never color-only" rule). |
| 2 | Clicks "Request Changes ▸" (only visible in `pending_review`) | Toggles the reject-with-reason form open inline (see Surface 2). Button gets `aria-expanded` state, mirroring `GateVerdictBox`'s override-toggle pattern (`GateVerdictBox.tsx:427-434`). |
| 3 | (After a successful reject, see Surface 2) | Card transitions to `changes_requested`: reason text renders read-only below the chip, "Regenerate Plan with This Feedback" button appears. `role="status" aria-live="polite"` announces the state change to screen readers without needing an explicit focus move (same no-op-focus precedent as `GateVerdictBox`'s post-approve re-render). |
| 4 | Clicks "Regenerate Plan with This Feedback" | Calls `onRegenerateWithFeedback` → `triggerTriage(item.id, item.planRejectionReason)` (Task 6.1.2) → item reloads. Button shows pending state via `actionPending`/`aria-busy` (same convention as every other `ActionButtonLabel` consumer). On success the next triage run's completion resets `plan_approved=false` and (per the correctness fix in Epic 1/glossary) presumably clears `plan_rejection_reason` once a new plan lands — verify this reset is wired; if the reason is never cleared, `changes_requested` state would persist even after a fresh plan is generated, which is a **state-transition gap worth flagging to Epic 3/4 review**, not something this design doc can resolve since it's an Epic 1–4 backend concern, not a frontend layout concern. |
| 5 | (Terminal state — item archived/removed elsewhere) | `PlanVerdictBox` receives `readOnly={terminalState !== null}` (Task 6.1.3) — renders the chip and, for `changes_requested`, the persisted reason text, but omits the "Request Changes"/"Regenerate" buttons entirely (same `readOnly` contract as `GateVerdictBox`, per its own doc comment at `GateVerdictBox.tsx:16-24`). |

### Accessibility (testable)

- **AC-1.1**: Every state's icon has `aria-hidden="true"`; the label text is always present and never conveyed by color alone. Verify via `PlanVerdictBox.test.tsx` snapshot of accessible name for all 5 states.
- **AC-1.2**: The card's root element carries `role="status" aria-live="polite" aria-atomic="true"`, matching `GateVerdictBox.tsx:253-257` verbatim — routine state changes are announced without interrupting the user (not `assertive`/`alert`, which is reserved for genuine failures per §6).
- **AC-1.3**: Keyboard: "Request Changes ▸" and "Regenerate Plan with This Feedback" are real `<button>` elements, reachable via Tab in document order, activatable via Enter/Space — no custom key handling required beyond native button semantics (contrast with `GateVerdictBox`'s Ctrl+Enter approve shortcut, which is optional convenience, not a requirement here since there's no single-keystroke "approve" action inside this box).
- **AC-1.4**: Color contrast ≥ 4.5:1 for all 5 card-color variants (`approved`/`pending_review`/`changes_requested`/`no_plan`/`skipped`) against their background, verified the same way existing `GateVerdictBox.css.ts` PASS/PARTIAL/FAIL/PENDING/UNVERIFIABLE variants are — reuse token pairs already proven compliant, do not hand-pick new colors for the `skipped` variant without checking contrast (Task 5.2.1 introduces one net-new variant).

---

## 2. Reject-with-reason inline form

### Wireframe (open state, mirrors `GateVerdictBox`'s reopen-form shape)

```
┌─ role="form" aria-label="Request changes" ─────────────────────────────────────┐
│  Feedback for the agent (required)                                             │
│  ┌────────────────────────────────────────────────────────────────────────┐   │
│  │ [cursor here on open — focus-on-open via useEffect]                     │   │
│  │                                                                          │   │
│  └────────────────────────────────────────────────────────────────────────┘   │
│  data-testid="plan-reject-reason"                                              │
│                                                                                  │
│                                          [ Cancel ]  [ Submit ]                 │
│  data-testid="backlog-action-reject-plan-submit" on Submit                     │
└──────────────────────────────────────────────────────────────────────────────┘
```

### Interaction flow

| Step | User does | System responds |
|---|---|---|
| 1 | Clicks "Request Changes ▸" | Form opens; focus moves into the textarea (`useEffect` on open, same as `GateVerdictBox.tsx:120-128`'s `reopen-feedback` focus). |
| 2 | Types reason | Submit stays `aria-disabled` + `disabled` while `reason.trim() === ""` — exact reuse of the `manual-review-summary` guard (`ActionsSection.tsx:302`: `disabled={!manualReviewSummary.trim() || actionLoading !== null}`). |
| 3a | Clicks "Cancel" | Form closes, textarea content discarded, state reverts to `pending_review` unchanged. **Gap**: see §7.1 — `GateVerdictBox`'s own Cancel handlers (`GateVerdictBox.tsx:398-404`, `462-466`) do not return focus to the toggle button on Cancel-click (only Escape does, via `handleOverrideFormKeyDown`, and only for the override form). If `PlanVerdictBox` mirrors the reopen-form "exactly" as instructed, it inherits this same omission. |
| 3b | Clicks "Submit" with non-empty reason | Calls `onReject(reason)` → `handleRejectPlan` (Task 6.1.2) → `rejectPlan(item.id, reason)` RPC. Button shows pending state (`actionPending`/`aria-busy`), form stays open and disabled-during-submit (mirroring `GateVerdictBox.handleReopenSubmit`'s `localPending` guard). |
| 4 | RPC succeeds | Form closes, textarea clears, card transitions to `changes_requested` (Surface 1, step 3). Toast: "Changes requested." (`showActionToast`, Task 6.1.2). |
| 5 | RPC fails | Toast: the thrown error message or "Reject failed." (Task 6.1.2's catch block), **and** — by direct analogy to `GateVerdictBox.handleReopenSubmit`'s `catch` (`GateVerdictBox.tsx:181-186`, `setActionError` → rendered via `<InlineError type="transient">` at line 416-423) — `PlanVerdictBox`'s internal submit handler should catch the rethrown error and render an inline `InlineError` below the form so the failure is visible without requiring the user to notice a toast. This is a direct, precedented application (confirmed: `handleGateReopen` in `BacklogItemDetail.tsx:805-827` already does exactly this double-surfacing — toast + rethrow, caught by the box component for inline display), not an invented pattern. Form stays open with the typed reason preserved (not cleared on failure) so the user doesn't retype it. |

### Error / edge case: empty or whitespace-only reason

| Case | UX | Precedent |
|---|---|---|
| User clicks "Submit" with an empty/whitespace-only textarea | Submit button is `aria-disabled` + `disabled`; click is a no-op (button not actionable) — no error message needed because the guard prevents the RPC from ever firing. `title` attribute optional here since the empty-textarea case is visually self-evident (contrast with `ActionsSection`'s `title` usage, which explains *non-obvious* disable reasons like "approve the plan first" — an empty textarea needs no such explanation). | `manual-review-summary` guard, `ActionsSection.tsx:300-308` |

### Accessibility (testable)

- **AC-2.1**: Focus moves into the textarea when the form opens (verify via RTL `document.activeElement`).
- **AC-2.2**: Submit is `aria-disabled="true"` when trimmed value is empty, and the `onClick` handler no-ops even if triggered (per `.claude`'s own caveat in research/ux.md §3: `aria-disabled` does not block JS handlers — the guard must be in the handler, not just the attribute).
- **AC-2.3 (gap flagged, not resolved here)**: Cancel does not return focus to the "Request Changes ▸" toggle button, per the inherited `GateVerdictBox` omission above. **Recommendation for the plan phase**: add `.focus()` on the toggle button ref in the Cancel `onClick`, matching what `handleOverrideFormKeyDown`'s Escape path already does correctly — this is a one-line fix, not a new pattern, and closes a real WCAG 2.4.3 (Focus Order) gap in both the new component and the one it mirrors.
- **AC-2.4**: User can complete the reject flow (open form → type reason → submit) in exactly 3 steps/clicks (click "Request Changes", type, click "Submit") — no additional confirmation dialog, consistent with `GateVerdictBox`'s reopen flow (which has no confirm step) and distinct from its Skip Gate flow (which does, because skip-gate is irreversible in a way reject-with-reason is not).

---

## 3. Rendered plan-content viewer (`PlanArtifactsSection`)

### Wireframe — collapsed (only when status is NOT `pending_review`, e.g. already `approved`)

```
▸ Plan Artifacts
```

### Wireframe — expanded (default when `pending_review`, per Task 7.1.4)

```
▾ Plan Artifacts
┌──────────────────────────────────────────────────────────────────────────────┐
│  /home/.../worktrees/item-xyz/plan_artifacts/plan.md          ← <code> path  │
│                                                                                  │
│  # Implementation Plan: ...                                                    │
│  ## 1. System Type                                                             │
│  ...rendered markdown: headings, tables, checklists via react-markdown...      │
│                                                                                  │
│  data-testid="backlog-plan-content-rendered"                                   │
└──────────────────────────────────────────────────────────────────────────────┘
```

### Interaction flow

| Step | User does | System responds |
|---|---|---|
| 1 | Item enters `pending_review` (plan artifacts exist, not yet approved/rejected) | `PlanArtifactsSection` auto-expands (Task 7.1.4: `defaultExpanded={planArtifactsExpanded \|\| derivePlanReviewStatus(item) === "pending_review"}`) and fetches `plan.md` content on mount (Task 7.1.3's `useEffect` on `[item.id, item.planArtifactsPath, item.updatedAt]`). |
| 2 | Content loads | Renders as formatted markdown (`react-markdown` + `remark-gfm`, GFM tables styled per Task 7.1.2) inside `markdownBody` — same visual treatment as `DescriptionSection.tsx`'s existing markdown body, no new visual language introduced. |
| 3 | User reads the plan, decides to approve or reject | See §6 for the **page-ordering gap** this design flags: as currently wired (Task 6.1.3 places `PlanVerdictBox`/`ActionsSection` above `PlanArtifactsSection` in the DOM), the Approve/Request-Changes controls are reachable *before* the rendered content appears on the page, even though the content is auto-expanded. This directly contradicts research/ux.md's own cited precedent ("the plan is always shown in full before the approve action is reachable... none of these tools put Approve behind a collapsed/hidden section") — auto-expanding fixes visibility-on-scroll but not *ordering*. |

### Accessibility (testable)

- **AC-3.1**: The rendered markdown container is a plain `<div>` (no ARIA widget role needed — static readable content), matching `DescriptionSection.tsx`'s precedent exactly.
- **AC-3.2**: Tables rendered via GFM styling meet ≥ 4.5:1 contrast for `th`/`td` text against `vars.color.cardBackground` (Task 7.1.2's header background) — verify token pairing, don't hand-pick.
- **AC-3.3**: `data-testid="backlog-plan-content-rendered"` present whenever `content !== null`, per e2e-test-conventions.md's data-testid-only locator rule.

---

## 4. "Newer plan available" stale-content notice

### Wireframe

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  /home/.../plan_artifacts/plan.md                                              │
│                                                                                  │
│  ⓘ A newer plan is available.                                    [ Reload ]   │
│    data-testid="plan-content-stale-notice"     role="status" aria-live="polite"│
│                                                                                  │
│  # Implementation Plan: ...   ← STALE content still shown, not swapped out    │
│  ...                                                                            │
└──────────────────────────────────────────────────────────────────────────────┘
```

### Interaction flow

| Step | User does | System responds |
|---|---|---|
| 1 | Background re-fetch (triggered by `item.updatedAt` changing — e.g. another triage run completed) returns content with a different `modifiedAtUnixMs` than `displayedMtime` | Content is **not** silently swapped (Task 7.1.3's explicit guard: `if (displayedMtime !== null && Number(res.modifiedAtUnixMs) !== displayedMtime) { setNewerAvailable(true); return; }`). `InlineNotice` renders above the (still-old) content. |
| 2 | User clicks "Reload" | `setNewerAvailable(false); setDisplayedMtime(null); void fetchContent();` — re-fetches and this time accepts the new content unconditionally (since `displayedMtime` is now `null`, the staleness guard's condition is false). |
| 3 | User ignores the notice and keeps reading old content | No forced interruption — `InlineNotice` uses `role="status" aria-live="polite"`, not `assertive`; the notice persists until dismissed or reloaded, it does not auto-dismiss or auto-swap content out from under an in-progress read. |

### **Flagged gap**: `InlineNotice` prop-shape mismatch in Task 7.1.3

Task 7.1.3's code sample writes:
```tsx
<InlineNotice
  message="A newer plan is available."
  actionLabel="Reload"
  onAction={() => { ... }}
  data-testid="plan-content-stale-notice"
/>
```
but the actual `InlineNotice` component (`web-app/src/components/common/InlineNotice.tsx:23-28`) has no `actionLabel`/`onAction` props — its real signature is:
```ts
interface InlineNoticeProps {
  message: string;
  actions?: InlineNoticeAction[];   // { label, onClick, variant?: "primary" | "secondary" }
  onDismiss?: () => void;
  "data-testid"?: string;
}
```
The correct call, using the actual API and matching the "reload" CTA styling research/ux.md calls out (`InlineNotice.tsx:16-21`, `variant: "primary"`), is:
```tsx
<InlineNotice
  message="A newer plan is available."
  actions={[{ label: "Reload", onClick: () => { setNewerAvailable(false); setDisplayedMtime(null); void fetchContent(); }, variant: "primary" }]}
  data-testid="plan-content-stale-notice"
/>
```
This is a straightforward fix to Task 7.1.3's example code, not a design decision — flagging so implementation doesn't copy the plan's pseudocode verbatim and hit a compile error.

### Accessibility (testable)

- **AC-4.1**: Notice uses `role="status" aria-live="polite"` (not `alert`/`assertive`) — a newer plan being available is routine, not an error, matching `InlineNotice`'s own documented rationale (`InlineNotice.tsx:7-12`).
- **AC-4.2**: "Reload" is a real `<button>` with `variant="primary"` styling (filled, per `InlineNoticeAction`'s doc comment), reachable by keyboard, and is the notice's sole exit path — no dead end, since ignoring it is also a valid (non-blocking) choice.
- **AC-4.3**: Old content remains fully readable and un-truncated while the notice is visible — verify via `PlanArtifactsSection.test.tsx`'s existing planned assertion ("Reload action applies the newer content," Task 7.1.5) plus a new assertion that pre-Reload content is unchanged.

---

## 5. Error states (missing plan file / failed RPC)

### Wireframe — plan file missing or fetch failed

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  /home/.../plan_artifacts/plan.md                                              │
│                                                                                  │
│  ✕ Triage failed — Failed to load plan content.          [ Retry ↺ ]          │
│    role="alert" aria-live="assertive"                                          │
└──────────────────────────────────────────────────────────────────────────────┘
```

**Flagged copy issue**: `InlineError`'s `type="transient"` copy table (`InlineError.tsx:23-39`) hardcodes the headline `"Triage failed"` for every `transient`/`timeout` case — there is no `type` variant whose default headline fits a *plan-content-fetch* failure. Task 7.1.3's code passes `customMessage={error}` (overriding the *body* text) but the **headline** "Triage failed" is left as-is, which is misleading in this context (nothing about triage failed — the plan-content fetch failed). This is worth flagging to the plan/implementation phase: either (a) accept the mismatched headline as a known copy debt (`InlineError` has no plan-content-specific `type` and adding one is out of this feature's stated scope), or (b) add a `headline` override prop to `InlineError` alongside the existing `customMessage` body override — a one-line addition to an existing component, not a new pattern. This design doc does not resolve which; it flags the concrete inconsistency for the plan to decide.

### Interaction flow

| Step | User does | System responds |
|---|---|---|
| 1 | `getPlanArtifactContent` RPC fails (file missing, network error, server error) | `error` state is set (Task 7.1.3's catch block); `InlineError type="transient"` renders with `customMessage={error}` — actual server error text surfaces (e.g. a `NotFound` connect error's message), not a generic string, so "the file may have been moved or deleted outside the app" (research/ux.md's recommended copy) only appears if the server's `NotFound` error message says so. **Recommendation**: confirm `GetPlanArtifactContent`'s `NotFound` error message (Epic 4 backend) matches or is compatible with this copy — a raw ConnectRPC error string ("not_found: ...") surfacing directly to the user would be a regression from the friendlier copy research/ux.md specified. |
| 2 | User clicks "Retry ↺" | `onRetry={() => void fetchContent()}` — re-attempts the fetch. No dead end: retry is always available, and `onDismiss` (also present) lets the user clear the error and keep the rest of the item detail view usable. |

### Error state: `ApprovePlan`/`RejectPlan` stale-content-token conflict (`FailedPrecondition`)

Per Epic 4 (P5), `ApprovePlan`/`RejectPlan` accept an `expected_modified_at_unix_ms` token and return `FailedPrecondition` on a mismatch (someone else — or a background regeneration — changed `plan.md` since the token was captured).

**Flagged gap (significant)**: as wired in Epic 5–7, this guard is **effectively unreachable from the UI**:
- `displayedMtime` (the fetched content's mtime) is local `useState` inside `PlanArtifactsSection` (Task 7.1.3) — it is never lifted to `BacklogItemDetail`, never exposed via props/context, and never threaded to `handleRejectPlan`/`handleApprovePlan`.
- `rejectPlan(id, reason, expectedModifiedAtUnixMs?)` (Task 6.1.1) accepts the token as an optional third argument, but `handleRejectPlan` (Task 6.1.2) calls `rejectPlan(item.id, reason)` — **omitting it entirely**, which defaults to `0n`. Per P5, `0` means "skip the check."
- `ActionsSection`'s "Approve Plan" button (`onAction("approve_plan")`) has no path to the mtime either — it lives in a sibling component with no shared state connecting it to `PlanArtifactsSection`'s fetch.

**Net effect**: the optimistic-concurrency protection Epic 4 builds server-side, and which pitfalls.md §1 specifically calls out ("stale-tab approve/reject racing a background plan regeneration"), never actually fires from either write path as currently spec'd — a user can approve or reject a plan that was silently regenerated out from under them while they were reading, with no `FailedPrecondition` ever triggered client-side. This is a real, concrete gap between the backend guard (Epic 4) and its frontend wiring (Epic 5–7), not a UX nuance — flagging it rather than inventing a fix, since resolving it requires an architectural decision (lift `displayedMtime` state up to `BacklogItemDetail`, or fetch a fresh mtime immediately before submit) that belongs in the plan, not this design doc.

**If/when wired**, the UX for a `FailedPrecondition` response should be: inline error inside `PlanVerdictBox` (same pattern as §2 step 5's RPC-failure handling) reading "The plan changed since you last viewed it — reload the Plan Artifacts section before approving/rejecting," with the InlineError's `onRetry` re-pointed to a content refetch rather than a blind resubmit (resubmitting with the same stale token would just fail again). No new component needed — this is a copy/wiring variant of the existing `InlineError` pattern, not a new pattern.

### Accessibility (testable)

- **AC-5.1**: All error states use `role="alert" aria-live="assertive"` (genuine failures — correctly distinct from the `polite` routine-notice cases in §1/§4, per research/ux.md's explicit `InlineNotice` vs `InlineError` semantic split).
- **AC-5.2**: Every error state offers an explicit exit path — "Retry ↺" and/or "Dismiss" (×) — no error renders as a dead end with no actionable button.
- **AC-5.3**: Error text is not a raw, unfiltered RPC/connect-error string presented as the sole message where a friendlier `customMessage` was specified in research — verify server error messages for `GetPlanArtifactContent`'s `NotFound` case are human-readable before shipping (cross-check with Epic 4's backend implementation).

---

## 6. Consolidated UX acceptance criteria

1. **AC-1.1 to AC-1.4** (PlanVerdictBox, §1) — icon+label always paired, `role="status" aria-live="polite"`, native-button keyboard access, ≥4.5:1 contrast on all 5 variants.
2. **AC-2.1 to AC-2.4** (Reject form, §2) — focus-on-open, `aria-disabled` guard on empty reason enforced in the handler not just the attribute, Cancel-focus-return gap flagged, 3-click completion.
3. **AC-3.1 to AC-3.3** (Content viewer, §3) — plain-div markdown container, GFM table contrast, stable `data-testid`.
4. **AC-4.1 to AC-4.3** (Stale notice, §4) — `polite` live region, primary-styled Reload button as sole/non-forced exit path, old content preserved until Reload.
5. **AC-5.1 to AC-5.3** (Error states, §5) — `assertive` live region for genuine failures, Retry/Dismiss always present, no raw error strings leaking to the user.
6. **AC-6 (cross-surface)**: A user can go from opening a `pending_review` item to submitting "Request Changes" with a reason in **≤ 3 clicks/interactions** (open item [not counted] → click "Request Changes" → type reason → click Submit), and from `changes_requested` to triggering regeneration in **1 click** ("Regenerate Plan with This Feedback").
7. **AC-7 (no dead ends)**: every error/notice state in this document (missing file, failed RPC, stale content, stale token) has at least one explicit, labeled exit action (Retry, Dismiss, Reload, or Cancel) — verified per-surface above; none rely on the user navigating away or reloading the page manually.
8. **AC-8 (persistence, Success Criterion 1)**: the plan-review status is visible via `PlanVerdictBox` in all 5 states without requiring any user action to reveal it (no click-to-expand needed to see the current state) — contrast with the pre-existing bug (`ActionsSection`'s vanishing "Approve Plan" button) this feature replaces.

---

## 7. Gaps flagged for the plan/implementation phase

These are concrete, confirmed inconsistencies or missing pieces found while grounding this design in the actual codebase and the actual plan.md text — not invented behavior. Each needs a decision or a one-line fix before/during implementation, not a UX redesign.

### 7.1 — Reject-form Cancel does not return focus to the toggle button
`GateVerdictBox`'s reopen-form Cancel handler (`GateVerdictBox.tsx:398-404`) and override-form Cancel handler (`GateVerdictBox.tsx:462-466`) both close the form and clear its text but do not call `.focus()` on the toggle button that opened it — only the override form's *Escape*-key path does (`handleOverrideFormKeyDown`, line 246). Since Task 5.2.3 instructs mirroring the reopen-form shape "exactly," `PlanVerdictBox` will inherit the same omission for its "Request Changes" Cancel button. **Recommendation**: add a ref + `.focus()` call on Cancel-click (not just Escape) for both the existing `GateVerdictBox` forms and the new `PlanVerdictBox` reject form — small, consistent fix, WCAG 2.4.3 Focus Order.

### 7.2 — Page ordering puts Approve/Reject controls above the plan content they gate
`PlanVerdictBox` and `ActionsSection` (Task 6.1.3, inserted "right before `<ActionsSection`" at ~line 1154) render above `PlanArtifactsSection` (rendered later, ~line 1218, inside the secondary/collapsible sections area) in `BacklogItemDetail.tsx`'s DOM order. Task 7.1.4 fixes the *collapsed* state (auto-expand when `pending_review`) but not the *position* — a user scanning top-to-bottom reaches the Approve/Request-Changes buttons before the rendered plan content. This directly contradicts the plan's own cited research (research/ux.md §1: "the plan is always shown in full before the approve action is reachable... none of these tools put Approve behind a collapsed/hidden section"). **Recommendation**: either move `PlanArtifactsSection` above `PlanVerdictBox`/`ActionsSection` when `derivePlanReviewStatus(item) === "pending_review"`, or accept the current ordering as an intentional scope cut and document why — this design doc does not choose for the plan phase, it surfaces the contradiction.

### 7.3 — `expected_modified_at_unix_ms` optimistic-concurrency token is unreachable from the UI as spec'd
Detailed in §5 above. The mtime captured by `PlanArtifactsSection`'s fetch never reaches `handleApprovePlan`/`handleRejectPlan`, so Epic 4's `FailedPrecondition` staleness guard cannot fire from either write path in Epic 5–7's current wiring. Needs an explicit decision: lift state, fetch-fresh-before-submit, or accept as a known limitation for v1 (with a note, not silence).

### 7.4 — `InlineNotice` prop-shape mismatch in Task 7.1.3's example code
Detailed in §4 above (`actionLabel`/`onAction` don't exist on the real component; use `actions: InlineNoticeAction[]`). Trivial fix, flagged so it isn't copy-pasted verbatim.

### 7.5 — `InlineError`'s hardcoded "Triage failed" headline is wrong for plan-content-fetch failures
Detailed in §5 above. `customMessage` only overrides the body, not the headline. Needs either an accepted-as-is decision or a small `headline` override prop addition.

### 7.6 — Unclear whether `plan_rejection_reason` is cleared on successful regeneration
Noted in §1 step 4. If the reason field is not cleared when a new plan lands (post-`TriggerTriage`), `changes_requested` could persist stale text describing feedback that's already been addressed by the new plan — this is a backend/data-model question for Epic 1–4, flagged here because it directly affects what Surface 1's `changes_requested` card displays.

---

## Summary

- **5 surfaces designed**: `PlanVerdictBox` (5-state card), reject-with-reason inline form,
  rendered plan-content viewer, "newer plan available" stale-content notice, and the
  consolidated error-state set (missing file / failed RPC / empty reason / stale-token
  conflict).
- **20 testable UX acceptance criteria** (AC-1.1–1.4, AC-2.1–2.4, AC-3.1–3.3, AC-4.1–4.3,
  AC-5.1–5.3, plus AC-6/7/8 cross-surface = 4+4+3+3+3+3 = 20 numbered ACs across §1–§6).
- **6 gaps flagged** for the plan/implementation phase (§7): reject-form focus-return on
  Cancel, page-ordering contradiction with cited research, unreachable optimistic-concurrency
  token, an `InlineNotice` prop mismatch in the plan's own example code, an `InlineError`
  headline mismatch, and an open question about rejection-reason clearing on regeneration.
  None were silently resolved with invented behavior — each is surfaced for a decision.
