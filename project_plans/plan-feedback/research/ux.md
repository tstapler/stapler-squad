# UX Research: plan-feedback

Agent 5, SDD Phase 2. Covers: comparable in-repo patterns, mental model /
weight-signaling, accessibility, error states, and jobs-to-be-done for the
new "send back with feedback" box on `in_progress`/`review`/`pr_pending`/`done`
backlog items.

## 1. Comparable UX patterns already in this codebase

Three existing feedback-box implementations were read in full/near-full:
`web-app/src/components/backlog/PlanVerdictBox.tsx` (reject-plan, ADR-002),
`web-app/src/components/backlog/TriageReviewPanel.tsx` ("Not quite — give
feedback" refine box), and `web-app/src/components/backlog/detail/ActionsSection.tsx`
(the bare `send_back_idea`/`send_back_ready` buttons at lines 428-451). All
three converge on the same shape — the new box should match it exactly
rather than invent a fourth variant:

**Interaction pattern: inline expand-to-form, never a modal.** Every one of
the three surfaces is a toggle button that reveals an inline `<div>`/form in
place — `PlanVerdictBox`'s `showReject` state toggling a `role="form"` block
(lines 193-243), `TriageReviewPanel`'s `showRefineForm` toggling
`role="form" aria-label="Refine triage with feedback"` (lines 320-385). No
`<dialog>`, portal-modal, or `window.confirm`-as-form pattern exists anywhere
in `backlog/` for a *feedback-capture* flow (portals are used only for toast
overlays, e.g. `TriageReviewPanel`'s `undoToast`). The new box should be a
third inline toggle-to-form block, not a modal.

**Toggle button:** stays permanently mounted (never conditionally
unmounted) even while its form is open — `PlanVerdictBox`'s comment at
lines 195-198 explains why: a ref to the toggle button is needed to restore
focus on cancel, and an unmounted button leaves that ref null. The toggle
carries `aria-expanded={showX}` and a `data-testid`.

**Form container:** `<div role="form" aria-label="<verb phrase>">` —
`"Request changes to plan"` in PlanVerdictBox, `"Refine triage with
feedback"` in TriageReviewPanel. The new box's form should get its own
distinct `aria-label`, e.g. `"Send back for re-planning"` or similar,
matching this exact `role="form" aria-label="..."` shape (not `<form>` — the
codebase consistently uses `role="form"` on a `div`, not a native `<form>`
element, in both precedents).

**Textarea:** `rows={3}` in both PlanVerdictBox and TriageReviewPanel's
refine box (the ManualReview summary textarea in ActionsSection.tsx uses
`rows={4}`, but that's an unrelated flow — 3 is the norm for "why/what
should change" boxes specifically). A `<label htmlFor=...>` always precedes
it (`"What should change? (required)"` / `"What should change?"`), and
`placeholder` gives a concrete example rather than a generic hint
(`"Explain what should be different about this plan…"` /
`"e.g. missed the mobile case, re-check the auth approach"`). Neither box
implements a character counter — despite `maxRejectReasonLength = 10000`
existing server-side (`backlog_service_lifecycle.go:892`), there is no
client-side counter or `maxLength` attribute anywhere in these two
components. **No character counter should be added** unless the plan phase
decides consistency with `maxRejectReasonLength` requires one — it would be
a new pattern, not a reused one. `textareaRef` is focused via `useEffect`
when the form opens (PlanVerdictBox lines 95-99); TriageReviewPanel doesn't
autofocus but does `disabled={isRefining}` while submitting.

**Escape-to-cancel:** PlanVerdictBox wires `onKeyDown` on the textarea to
call `handleCancel()` on `Escape` (lines 107-111, 214-224) — TriageReviewPanel's
refine box does not implement this. Since the new box is closer to
PlanVerdictBox's "reject with reason" semantics (a considered, named action)
than TriageReviewPanel's inline nudge, follow PlanVerdictBox's Escape
handling.

**Submit/Cancel button placement and labels:** both precedents put Cancel
(secondary style) to the left, Submit/primary action to the right, inside a
`styles.formActions`/`styles.actions` flex row. Submit is disabled when the
textarea is empty (`reason.trim().length > 0`) or a request is pending.
Button label swaps to a present-participle "…ing" form while pending
(`"Requesting…"` / `"Refining…"`) and both use `aria-busy={isPending}` plus
`aria-disabled`/`disabled` in tandem (PlanVerdictBox lines 231-233). No
precedent grays out with a spinner icon — text-only pending state is the
convention (see also `ActionButtonLabel` component used throughout
ActionsSection.tsx for the same text-swap pattern on every other button).

**Loading/pending state:** local `useState` boolean (`localPending`/
`regeneratePending`/`refineState==="submitting"`) drives both the button
label and `aria-busy`; the whole app does not use a spinner component for
this class of action.

**Success toast wording:** `BacklogItemDetail.tsx`'s `ACTION_SUCCESS_MESSAGES`
dictionary (lines 76-94) uses short, past-tense, plain-English fragments
ending in a period: `"Sent back to ready."`, `"Sent back to triage."`,
`"Triage re-triggered."`, `"Revisions requested."` (the existing message for
`reject_plan`, set directly via `showActionToast("Revisions requested.",
"success", toastKey)` at line 997 rather than through the dictionary — reject-plan
bypasses `handleAction`'s generic dispatch entirely and has its own handler,
`handleRejectPlan`). The dead `send_back_refining: "Sent back to refining."`
entry already exists in the dictionary (line 92) but nothing ever fires that
action id (per requirements.md gap #2). **Whatever the new action is named,
its toast message should follow this exact tense/length/punctuation
convention** — e.g. if the chosen target status is `ready` under the hood
(see requirements.md's Rabbit Holes), something like `"Feedback sent —
retriage started."` rather than a longer sentence.

**PlanVerdictBox's error handling is the closest precedent to reuse
directly** (see §4 below) since the new flow, like `onReject`, is a
consequential status-changing write with a required reason — TriageReviewPanel's
`onRefine` is the second-closest precedent (it also triggers backend work,
not just a status write).

## 2. User mental model — should the UI signal extra weight?

**Recommendation: yes, but through copy and framing, not a second
confirmation click.** Two lines of evidence from this codebase argue against
a `window.confirm()`-gated second step:

- The codebase's existing `confirm()` usage (`BacklogItemDetail.tsx` lines
  772-776, 783) is reserved for **irreversible, undoable-nothing** actions —
  delete (`"Permanently delete this item and all its history? This cannot
  be undone."`) and archive (worktree deletion, `"cannot be recreated"`).
  Sending an item back for re-planning is not in that category: it doesn't
  destroy data, and (per requirements.md's Rabbit Holes leaning toward
  `ready` as the target status) it explicitly *preserves* the prior plan as
  retriage context. A `confirm()` here would miscalibrate the user's sense
  of which actions in this app are truly destructive.
- requirements.md's own Scope section is explicit that the user wants
  **"a single operator action, not a two-click reject-then-regenerate
  flow"** — a deliberate, stated deviation from ADR-002's two-click
  precedent. Adding a browser `confirm()` dialog would reintroduce exactly
  the extra click the requirements doc says to avoid, just via a different
  mechanism.

Instead, signal the weight through:

- **The required-reason pattern itself** (identical to PlanVerdictBox's
  `"(required)"` label suffix and disabled-until-non-empty submit) already
  raises the bar above the current zero-friction bare button — you cannot
  fire this action accidentally with one click the way you can today's
  "↩ Back to Ready."
- **An inline warning line when there's a live/active session**, using the
  same `activeSessionCount`/`activeWorkSessionCount` data
  `BacklogItemDetail.tsx` already computes at line 249
  (`item.linkedSessions.filter(s => s.role === "work" && !s.endedAt).length`)
  and threads into `VersionControlSection` at line 1729. This is exactly the
  situation requirements.md's Out-of-Scope section flags — this flow doesn't
  steer a *live* session, so if one is still running, the operator should
  see something like *"This item has an active session — it will not be
  interrupted, but its work may be superseded once re-planning completes."*
  rendered as a non-blocking `InlineNotice` (the same component ActionsSection
  already uses for the archived/removed terminal-state notice) above the
  textarea, not a dialog that blocks submission.
- **Distinct button copy from the existing bare "↩ Back to Ready"** so the
  two affordances read as different in weight even before expansion —
  something like "Send back for re-planning" rather than reusing "↩ Back to
  Ready" wording, so the operator doesn't conflate the new, feedback-carrying
  action with the old context-free one it's replacing/supplementing.

**Industry comparables**, for calibration (general knowledge, not sourced
from this repo): GitHub PR "Request changes" is a *required-comment,
single-submit* review verdict — no separate confirmation step beyond
typing the comment and clicking "Submit review," which is exactly the
inline-expand-to-form-then-one-submit shape this codebase already has via
PlanVerdictBox. Jira's "reopen with comment" is similarly one dialog, one
field, one submit — no second confirmation gate. Neither product treats
"reject with reason" as needing a *second* are-you-sure step; the reason
requirement itself is treated as sufficient friction. This matches the
recommendation above.

## 3. Accessibility

Both precedents already establish a consistent, ARIA-role-only pattern
(consistent with `e2e-test-conventions` skill's "`data-testid` or ARIA roles
only" locator rule, `.claude/skills/e2e-test-conventions/SKILL.md`). Reuse
verbatim:

- Outer toggle button: plain `<button>` with `aria-expanded={showForm}` and
  a `data-testid`. No `aria-controls` is used by either precedent (an
  omission worth flagging but not worth introducing inconsistently in just
  this one new component — match what exists).
- Form wrapper: `<div role="form" aria-label="<description of the
  action>">` — gives assistive tech a named landmark without needing a
  native `<form>` element (avoids native form-submit-on-Enter semantics,
  which neither precedent wants since Enter in a multi-line textarea should
  insert a newline, not submit).
- `<label htmlFor="unique-id">` paired with the textarea's matching `id`
  (`plan-reject-reason` / `triage-refine-feedback`) — always a real
  `<label for>`, never `aria-label` alone, for the primary input.
  `data-testid` on the textarea itself for e2e targeting
  (`plan-reject-reason` / `triage-refine-textarea`).
- Keyboard: `Escape` closes the form and returns focus to the toggle button
  (PlanVerdictBox's `handleTextareaKeyDown` + `toggleRef.current?.focus()`
  in `handleCancel`) — carry this over, since TriageReviewPanel's simpler
  refine box lacks it and PlanVerdictBox is the closer precedent for this
  action's weight (see §1).
- Submit button: `aria-busy={isPending}` and `aria-disabled`+`disabled` kept
  in sync (both set together, not just one) while pending or while the
  textarea is empty.
- Any inline warning about an active session (§2) should use `InlineNotice`
  (already accessible — used elsewhere for terminal-state notices) rather
  than inventing new markup.
- Error display: `InlineError` (see §4) already sets `role="alert"
  aria-live="assertive"` on both its pill and block variants — reuse it
  rather than a bespoke error element, so screen readers get the same
  assertive announcement behavior as every other action-failure in this
  surface.
- The success path relies on the existing toast system
  (`useNotifications`'s `showActionToast`), which is out of scope for this
  research to re-audit — no new accessibility surface is introduced there.

## 4. Error states — precedent from `PlanVerdictBox`'s `setActionErrorHeadline`

`PlanVerdictBox.tsx` (lines 84-85, 113-127, 180-191) is the exact precedent
for "the mutation started but something downstream failed" — its
`handleSubmit` wraps `onReject(reason)` in try/catch, and on failure calls
`setActionErrorHeadline("Failed to request changes")` +
`setActionError("Action failed. Please try again.")`, rendering an
`InlineError type="transient"` with `onDismiss` but deliberately **no
`onRetry`** — the comment at lines 180-184 explains why: *"neither
reject-plan nor regenerate-plan failure has a wireable retry here (the user
re-triggers the action via the still-open form/button below), so offering a
'Retry' that just clears state would misrepresent what the button does."*
The form/textarea state (the typed reason) is preserved across the failed
submit — `handleSubmit` only clears `reason`/`setShowReject(false)` in the
success path, so a failed submit leaves the operator's typed feedback intact
to resubmit, rather than discarding it.

This maps directly onto requirements.md's Rabbit Hole about the combined
action possibly being two RPC calls behind one submit (status transition +
`TriggerTriage`). If that's how Phase 3 implements it, the error headline
should distinguish **which half failed**, since they have different retry
implications:

- Status transition itself fails → nothing changed; the existing
  `"Failed to..."` + generic body pattern applies directly, and the form
  should stay open with the typed feedback intact (mirroring PlanVerdictBox).
- Status transition **succeeds** but `TriggerTriage` fails to start →
  this is the case explicitly called out as a risk in the task prompt. The
  item has *already moved* (e.g. to `ready`) but no retriage is running.
  This is a materially different failure than "nothing happened," and the
  generic `"Action failed. Please try again."` body would be actively
  misleading (retrying would attempt to re-run the status transition, which
  may no-op or error since the item is no longer in its prior status). The
  error headline/body should say something like *"Sent back, but retriage
  didn't start — use Trigger Triage below to retry"* and point at the
  already-visible `trigger_triage` action in `ActionsSection.tsx` (which
  `getAvailableActions` will now expose once the item is back at a
  triage-eligible status), rather than re-offering the same combined
  submit button. This is a UI/copy decision for Phase 3, not something to
  resolve here, but the precedent (PlanVerdictBox's no-misleading-retry
  discipline) directly informs it: **don't offer a "Retry" affordance that
  would re-attempt the wrong half of a two-step operation.**

## 5. Jobs-to-be-done (solo operator)

Per requirements.md's explicit single-operator framing (not a
multi-reviewer tool):

- **Functional job:** "Redirect work that's already progressed, without
  losing what it already established, and without me having to manually
  re-trigger triage outside the UI or retype my reasoning somewhere
  unrelated." This is the concrete gap requirements.md's Baseline section
  names — today's alternative is re-typing feedback into "an unrelated box
  elsewhere in the UI," which is the same generic-box complaint
  `backlog-operator-feedback-loop`/PR #457 already fixed once for a
  different surface (per MEMORY.md's project index, this is a recurring
  theme — the fix pattern here should be consistent with that resolved
  precedent rather than reopening the same complaint on a new surface).
- **Emotional job:** confidence that the feedback was actually *heard* by
  the next triage pass, not just filed as a status change. This is why the
  success metric in requirements.md explicitly requires the feedback be
  "verifiably used by the resulting triage/plan run (visible in the new
  `triage_result`/plan diff)" — the UI's job isn't done at "toast says
  success," it's done when the operator can later *see* their words
  reflected in the next plan. That argues for the toast or a follow-up
  affordance pointing at where to go check (the triage panel/diff), though
  the exact mechanism is a Phase 3 decision.
- **Social job:** none in the traditional sense (no reviewer/reviewee
  relationship — this is the same operator talking to their own future
  self/agent), but there is a "future self" analog: the feedback text
  becomes a durable record of *why* the item was sent back, which matters
  for a solo operator returning to an item days later and needing to
  remember their own reasoning — the same job `PlanRejectionReason`
  already serves for the pre-implementation case (it's persisted and
  displayed back in `PlanVerdictBox`'s `changes_requested` card, lines
  161-163). Whatever field Phase 3 chooses for this later-lifecycle
  feedback should preserve that same "show me what I said last time"
  readback, not just consume the text once and discard it.

## Summary of concrete recommendations for Phase 3 planning

1. Inline expand-to-form toggle button (not a modal), following
   `PlanVerdictBox.tsx`'s structure most closely (required-reason,
   Escape-to-cancel, error-preserves-input) since this action is closer in
   weight to "reject a plan" than to TriageReviewPanel's lighter
   nudge-to-refine.
2. `role="form" aria-label="..."`, `<label htmlFor>` + matching textarea
   `id`, `rows={3}`, concrete-example placeholder, `data-testid` on every
   interactive element — no new locator strategy.
3. No character counter (no existing precedent has one); no
   `window.confirm()` gate (reserved for irreversible actions in this
   codebase, and explicitly against the requirements doc's one-click
   intent) — signal weight via distinct button copy and an `InlineNotice`
   warning only when `activeWorkSessionCount > 0` (data already computed at
   `BacklogItemDetail.tsx:249`).
4. Toast message follows the `ACTION_SUCCESS_MESSAGES` dictionary's
   short/past-tense/period convention; replace or repurpose the currently-dead
   `send_back_refining` entry rather than leaving it orphaned.
5. Error handling reuses `InlineError` with a headline set via the
   `setActionErrorHeadline`-equivalent pattern, no misleading `onRetry` on
   a two-call combined action, and must distinguish "nothing happened" from
   "status changed but retriage didn't start" if the implementation ends up
   as two sequential RPC calls.
