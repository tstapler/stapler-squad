# UX Design: plan-feedback

SDD Phase 3. Covers the `SendBackFeedbackBox` component (`web-app/src/components/backlog/detail/SendBackFeedbackBox.tsx`), its wiring into `ActionsSection.tsx`, and the `handleSendBackWithFeedback` handler in `BacklogItemDetail.tsx` (see `project_plans/plan-feedback/implementation/plan.md` Epics 2.1-2.2). Built directly on `project_plans/plan-feedback/research/ux.md`'s recommendations and forked from the existing `PlanVerdictBox.tsx` toggle/form pattern — no new interaction model is introduced.

Target status under the hood is `ready` (ADR-001); feedback text reuses the existing `PlanRejectionReason` field. The combined submit is three sequential RPC calls from one user action: `transitionStatus` → `rejectPlan` → `triggerTriage` (plan.md Task 2.2.1a).

---

## Surfaces designed

1. Toggle button (collapsed state)
2. Expanded form (idle, ready-to-type state)
3. Expanded form — active-session warning sub-state
4. Expanded form — submitting (pending) state
5. Success state (toast + resulting item state)
6. Error state — call 1 fails (`transitionStatus` rejects, nothing changed)
7. Error state — calls 2/3 fail after call 1 succeeded (partial failure, recoverable via `PlanVerdictBox` for two sub-cases, via an in-place Retry for the third — see table below)
8. Escape-to-cancel flow
9. Empty/validation state (Submit disabled until text typed)

9 surfaces / sub-states total.

---

## 1. Collapsed toggle button

This replaces today's bare "↩ Back to Ready" button in `ActionsSection.tsx` (plan.md Task 2.2.1c). Rendered whenever `actions.has("send_back_ready")` is true — i.e. item status is `in_progress`, `review`, `pr_pending`, or `done`.

```
┌─────────────────────────────────────┐
│  [ ↩ Send back for re-planning ]     │   ← data-testid="backlog-action-send-back-feedback"
└─────────────────────────────────────┘        aria-expanded="false"
```

- Distinct copy from the old "↩ Back to Ready" label (ux.md §2) — signals this is a different, feedback-carrying action, not the old context-free transition it replaces.
- Disabled (grayed, non-interactive but still focusable) whenever another `ActionsSection` action is in flight (`disabled={actionLoading !== null && actionLoading !== "send_back_ready"}`) — same convention as every other button in this section.
- Stays mounted at all times while `visible` is true, even while its own form is open below it (needed so `toggleRef` survives for focus-restore on Cancel/Escape — see `PlanVerdictBox.tsx:195-198`). Once `visible` goes false (the item's status left the send-back-eligible set), the toggle itself unmounts — but the surrounding `SendBackFeedbackBox` may still be mounted on its own, showing only a pending error with no toggle, if `actionError` is set (Surface 7's mechanism note; BLOCKER fix, iteration 2 repair pass), or showing only its own pending form/"Sending…" button with no toggle and no error, if a row-4 Retry is in flight (Surface 4's note; BLOCKER fix, iteration 4 repair pass).

**Interaction:** Click toggles `showForm`. No confirmation dialog (ux.md §2 — reserved for irreversible actions only; this isn't one).

---

## 2. Expanded form — idle state

```
┌─────────────────────────────────────┐
│  [ ↩ Send back for re-planning ]     │  aria-expanded="true"
├───────────────────────────────────────────────────────────┐
│ role="form" aria-label="Send back for re-planning"         │
│                                                              │
│  What should change? (required)                            │
│  ┌────────────────────────────────────────────────────┐   │
│  │ e.g. missed the mobile case, re-check the auth      │   │  ← placeholder,
│  │ approach                                            │   │    rows=3
│  │                                                      │   │
│  └────────────────────────────────────────────────────┘   │
│                                                              │
│                                   [ Cancel ]  [ Submit ]    │  Submit disabled
└───────────────────────────────────────────────────────────┘  (empty textarea)
```

- Textarea auto-focuses on open (`useEffect` on `showForm`, mirrors `PlanVerdictBox.tsx:95-99`).
- `id="send-back-feedback"` paired with a real `<label htmlFor>` (not `aria-label` alone).
- `data-testid="send-back-feedback-textarea"` on the textarea, `data-testid="backlog-action-send-back-feedback-submit"` on Submit.
- No character counter (ux.md §1 — no existing precedent has one; server-side cap `maxRejectReasonLength = 10000` is enforced server-side only).
- Submit starts **disabled**: `feedback.trim().length === 0`.

---

## 3. Expanded form — active-session warning sub-state

Rendered above the textarea whenever `activeWorkSessionCount > 0` (reuses the existing derived value at `BacklogItemDetail.tsx:249` — no new computation).

```
┌───────────────────────────────────────────────────────────┐
│ role="form" aria-label="Send back for re-planning"         │
│                                                              │
│  ⓘ This item has an active session — submitting this will   │
│    stop it, and its work may be superseded once re-planning │  ← InlineNotice
│    completes.                                                │     (non-blocking)
│                                                              │
│  What should change? (required)                            │
│  ┌────────────────────────────────────────────────────┐   │
│  │                                                      │   │
│  └────────────────────────────────────────────────────┘   │
│                                   [ Cancel ]  [ Submit ]    │
└───────────────────────────────────────────────────────────┘
```

- **Non-blocking**: this is an `InlineNotice`, not a `window.confirm()` gate. It does not disable Submit and does not require acknowledgment. This is deliberate — see requirements.md Out of Scope ("this flow does not steer a live session in place") and ux.md §2 (a `confirm()` here would miscalibrate the app's existing "irreversible-only" confirm convention and reintroduce the extra click requirements.md explicitly asks to avoid).
- Copy is fixed, not templated with a session count — matches `InlineNotice`'s existing terminal-state-notice usage elsewhere in `ActionsSection.tsx`.

---

## 4. Expanded form — submitting (pending) state

```
┌───────────────────────────────────────────────────────────┐
│ role="form" aria-label="Send back for re-planning"         │
│                                                              │
│  What should change? (required)                            │
│  ┌────────────────────────────────────────────────────┐   │
│  │ missed the mobile layout, redo with touch targets   │   │  ← value preserved,
│  └────────────────────────────────────────────────────┘   │    not cleared yet
│                                                              │
│                                   [ Cancel ]  [ Sending… ]  │  aria-busy="true"
└───────────────────────────────────────────────────────────┘  both disabled,
                                                                  aria-disabled="true"
```

- Button label swaps to present-participle `"Sending…"` (text-only, no spinner icon — matches the app-wide convention, ux.md §1).
- `aria-busy={isPending}` and `aria-disabled`/`disabled` set together, never independently (ux.md §3).
- **Cancel is gated the same as Submit while pending** (BLOCKER fix): Cancel is
  rendered `disabled`/`aria-disabled={isPending}`, and `Escape` inside the
  textarea is equally inert during this state — both call the same
  `handleCancel`, which early-returns while `isPending` is `true` (plan.md
  Task 2.1.1b). Without this, clicking Cancel or pressing Escape mid-submit
  would visually collapse the form — exactly as if the action were aborted —
  while the in-flight `transitionStatus`→`rejectPlan`→`triggerTriage` chain
  keeps running unseen: a success would then fire its toast into what looks
  like an aborted action, and a failure would drop its `InlineError` into a
  subtree that's no longer mounted, with no recovery path shown. This matches
  Submit's existing pending-gating exactly, so the whole form — not just one
  button — is inert while `isPending` is `true`.
- This single pending state covers all three chained RPC calls (`transitionStatus` → `rejectPlan` → `triggerTriage`) — the UI does not expose per-call progress ("transitioning… / rejecting… / triggering…"); from the operator's perspective this is one action, matching requirements.md's "single operator action" framing. (A three-step progress indicator was considered and rejected — see "Decisions" below.)
- **This pending UI renders identically during a Retry, including one launched from a `visible={false}` state (BLOCKER fix, iteration 4 repair pass):** row 4's Retry (Surface 7) sets `localPending` true and clears `actionError` to `null` before its own `onSubmit` resolves, while `visible` is still `false` (the item's status stays `idea` until the retry's chain settles). The mount guard therefore also checks `isPending` — `if (!visible && !actionError && !isPending) return null;` (plan.md Task 2.1.1b) — so this surface's "Sending…"/`aria-busy` UI stays on screen for a retry's pending phase exactly as it does for the original submit, instead of the component disappearing for the duration and reappearing only once the retry settles.

---

## 5. Success state

On all three calls resolving:

```
Form collapses. Textarea clears. Focus does NOT move to the toggle
(unlike Cancel/Escape) — the item detail view re-renders with the new
state below, so directing focus back to a now-stale toggle would strand
the keyboard user away from the result.

┌─────────────────────────────────────────┐
│ ✓ Feedback sent — retriage started.      │  ← toast, success, auto-dismiss
└─────────────────────────────────────────┘     (existing toast system,
                                                   ACTION_SUCCESS_MESSAGES-style
                                                   short/past-tense/period copy)

Item detail view (after load() re-fetch):
┌─────────────────────────────────────┐
│ Status: ready                         │
│ ┌───────────────────────────────────┐│
│ │ Plan Review                        ││
│ │ ✎ Revisions requested              ││  ← PlanVerdictBox, changes_requested
│ │ "missed the mobile layout, redo    ││     card — shows the operator's own
│ │  with touch targets"               ││     feedback read back (ux.md §5,
│ └───────────────────────────────────┘│     "future self" durable record)
└─────────────────────────────────────┘
```

- Toast copy: **"Feedback sent — retriage started."** — follows the `ACTION_SUCCESS_MESSAGES` dictionary convention exactly (short, past-tense, ends in period; matches `"Sent back to ready."` / `"Triage re-triggered."` precedent).
- The operator's "was I heard" job (ux.md §5) is closed by the readback in `PlanVerdictBox`'s `changes_requested` card, not just the toast — the toast confirms the click worked; the card confirms the words landed.

---

## 6. Error state — call 1 fails (`transitionStatus` rejects)

Nothing has changed server-side. This is the direct `PlanVerdictBox` precedent (ux.md §4).

```
┌───────────────────────────────────────────────────────────┐
│ role="form" aria-label="Send back for re-planning"         │
│                                                              │
│  What should change? (required)                            │
│  ┌────────────────────────────────────────────────────┐   │
│  │ missed the mobile layout, redo with touch targets   │   │  ← preserved,
│  └────────────────────────────────────────────────────┘   │    NOT cleared
│                                                              │
│  ⚠ Failed to send back                          [Dismiss]  │  ← InlineError
│    <server error text, or "Failed to send back."           │    type="transient"
│     if none is available>                                   │    role="alert"
│                                   [ Cancel ]  [ Submit ]    │    aria-live="assertive"
└───────────────────────────────────────────────────────────┘    re-enabled, no onRetry
```

- Headline: **"Failed to send back"**.
- Body: `getErrorMessage(err, "Failed to send back.")` — the real server error
  string when one is available, otherwise the fallback literal **"Failed to
  send back."** (Earlier draft of this doc said the fallback body reads
  "Action failed. Please try again."; that string is never actually
  rendered — `getErrorMessage`'s fallback is the string passed at the call
  site, which `SendBackFeedbackBox.tsx`/`handleSendBackWithFeedback` set to
  "Failed to send back.", matching the headline. That headline/body
  redundancy on the no-server-error branch is acceptable: the
  headline/body distinction still does real work whenever a real server
  error string is present, which is the common case.)
- Form stays open, typed text intact — operator re-clicks Submit to retry the whole sequence from the top; no separate "Retry" affordance is offered (there's nothing to retry differently — resubmitting the form *is* the retry).
- `InlineError` gets **no `onRetry` prop** — same "no misleading retry" discipline as `PlanVerdictBox` (a wired Retry button would imply a one-call retry when this is actually a fresh three-call sequence).

---

## 7. Error state — calls 2/3 fail after call 1 succeeded (partial failure)

**This is the case the adversarial reviewer is explicitly checking — be concrete.** The item has *already moved* to `ready` (call 1 succeeded) before `rejectPlan` or `triggerTriage` throws. Silently showing the same generic "Action failed" message here would be actively misleading: the operator would re-click Submit, and `transitionStatus(item.id, "ready", { expectedStatus: item.status, ... })` would now run with `expectedStatus` mismatched against the item's *already-`ready`* status (or, if `rejectPlan` succeeded too and only `triggerTriage` failed, get double-called), producing a confusing second error on top of the first.

**Mechanism note — how this copy actually gets on screen (fixed BLOCKER, iteration 2 repair pass):** `SendBackFeedbackBox`'s `visible` prop is `actions.has("send_back_ready")`, which is `false` the instant the item's status moves to `ready` or `idea` — exactly the statuses every partial-failure branch below lands on. `handleSendBackWithFeedback`'s catch block (Task 2.2.1a) calls `load()` before re-throwing, so by the time the component below would render this error, `visible` has already flipped to `false`. A naive `if (!visible) return null;` guard would unmount the component at that exact moment, discarding the `actionError` state this section's copy lives in before the operator ever saw it — the fresh UX triad-lens BLOCKER a later review round caught, since the original design never traced this interaction through. The fix (Task 2.1.1b): the guard is `if (!visible && !actionError) return null;` — the component stays mounted, with its own toggle button hidden (the action is no longer eligible) but its form and error still shown, for exactly as long as `actionError` is set. Because `actionError` is local `useState`, it survives the `load()`-triggered re-render the same way any local state survives any parent re-render, as long as the component instance itself isn't unmounted. Dismissing the error (or clicking Cancel) clears `actionError`, and the component unmounts cleanly on its next render.

**Mechanism note, continued (BLOCKER fix, iteration 4 repair pass):** that guard has one more gap — row 4's own Retry action. `handleSubmit` (invoked by `handleRetry`) sets `localPending` true and clears `actionError` to `null` in the same call, before its `onSubmit` promise resolves. For the whole duration of that retry, `visible` is still `false` (the item's status doesn't leave `idea` until the retry's own `transitionStatus`→`rejectPlan`→`triggerTriage` chain settles) *and* `actionError` is now `null` — exactly the combination `if (!visible && !actionError) return null;` treats as "nothing to show," so it unmounted the entire component (toggle, form, "Sending…" button, `aria-busy` indicator) for the retry's whole pending phase, reappearing only once the promise settled. The guard now also checks the pending flag: `if (!visible && !actionError && !isPending) return null;`, reusing the `isPending` value already computed for Submit's own pending gating rather than adding a second one. Checked against every reachable state: `visible=true` (any error/pending combination) short-circuits past the guard unaffected; `visible=false` with `actionError` set still renders (unaffected, per the mechanism above); `visible=false` with `actionError` clear and `isPending` true now renders too (the fix); and `visible=false` with both `actionError` clear and `isPending` false still correctly returns `null` — so the component still unmounts cleanly once a retry finishes with no error and the item remains outside `CAN_SEND_BACK_READY`, with no stuck-mounted-forever regression.

**What the operator sees, in order:**

1. **Inside `SendBackFeedbackBox`'s own error path** (per plan.md's `handleSendBackWithFeedback`, which re-throws after showing a toast — Task 2.2.1a): the toast fires first, with copy that no longer contradicts the more accurate in-form message below it (CONCERN fix, iteration 2 repair pass) —

   ```
   ┌───────────────────────────────────────────────┐
   │ ✗ Send-back needs attention — see details      │   ← toast, error
   │   below.                                        │
   └───────────────────────────────────────────────┘
   ```

   (Exact copy: for a `SendBackError` whose `failedAt !== "transition"` —
   i.e. any of this surface's partial-failure branches — the toast reads
   **"Send-back needs attention — see details below."**, a neutral pointer at
   the in-form `InlineError` rather than a claim of total failure. Case 6
   (`transitionStatus` itself fails, nothing changed) keeps the original
   `getErrorMessage(e, "Failed to send back.")` copy, since "Failed to send
   back." is accurate there. This matches `handleRejectPlan`'s existing
   toast-error shape at `BacklogItemDetail.tsx:990-1007` for the non-partial
   case.)

2. **The form's own `InlineError`** (rendered by `SendBackFeedbackBox`'s `handleSubmit` catch block, since the parent's rejected promise reaches it too):

   ```
   ┌───────────────────────────────────────────────────────────┐
   │ role="form" aria-label="Send back for re-planning"         │
   │                                                              │
   │  What should change? (required)                            │
   │  ┌────────────────────────────────────────────────────┐   │
   │  │ missed the mobile layout, redo with touch targets   │   │  ← preserved
   │  └────────────────────────────────────────────────────┘   │
   │                                                              │
   │  ⚠ Sent back, but retriage didn't start                    │  ← distinct headline,
   │    The item already moved to "ready" and your feedback was  │    NOT generic
   │    recorded. Close this form and use the "Regenerate Plan   │
   │    with This Feedback" button below to retry — do not       │
   │    resubmit this box.                             [Dismiss] │
   │                                                              │
   │                                   [ Cancel ]  [ Submit ]    │
   └───────────────────────────────────────────────────────────┘
   ```

   - Headline: **"Sent back, but retriage didn't start"** — distinct from case 6's "Failed to send back," so the operator can tell these apart at a glance.
   - Body: **"The item already moved to \"ready\" and your feedback was recorded. Close this form and use the \"Regenerate Plan with This Feedback\" button below to retry — do not resubmit this box."** — names the concrete state change (status is now `ready`, feedback recorded) and points at the concrete recovery affordance. **BLOCKER fix, iteration 3 repair pass:** earlier copy here pointed at the plain "Trigger Triage" button instead. That button (`ActionsSection.tsx`) calls `triggerTriage(item.id)` with no feedback argument, and the backend only ever reads feedback from the RPC request — never from the `PlanRejectionReason` field `rejectPlan` (call 2) just persisted — so following that instruction would have silently discarded the operator's feedback. At this point `rejectPlan` succeeded, so `PlanVerdictBox` is already showing its `changes_requested` card with a **"Regenerate Plan with This Feedback"** button (`PlanVerdictBox.tsx`) that explicitly passes `item.planRejectionReason` to `triggerTriage` — the correct, feedback-preserving affordance. Both buttons are simultaneously visible in this state, so naming the specific one matters.
   - **No `onRetry` wired to this box's own Submit for this specific failure mode** — Submit stays clickable (nothing disables it), but the copy actively steers the operator away from reusing it, rather than the UI silently allowing a broken retry path. (Contrast with case 7's fourth row below, where `onRetry` *is* wired — that retry is genuinely correct there, unlike here.)
3. **After the operator closes the form (Cancel) or dismisses the error** and looks at the item detail view: `PlanVerdictBox` is already showing the `changes_requested` card with the operator's feedback (since `rejectPlan` — call 2 — succeeded whenever it's call 3/`triggerTriage` that failed; if call 2 itself failed, the card shows no reason yet, and the recovery path is `PlanVerdictBox`'s own "Request Changes" flow, unchanged from today). This card's **"Regenerate Plan with This Feedback"** button (existing ADR-002 flow, untouched by this feature) is the actual recoverable retry for these two sub-cases — not a new bespoke affordance (plan.md Story 2.2.1's third acceptance criterion). **Exception: the fourth row (item lands at `idea`) has no `PlanVerdictBox` card to point at at all — that row's recovery is the in-place Retry button described above, not this step** (BLOCKER B, iteration 3 repair pass).

**Which sub-case produces which recovery path** (for implementer/reviewer clarity — not shown to the operator as separate messages, since `handleSendBackWithFeedback` doesn't distinguish them in its catch block per plan.md, but the *result state* differs):

| Failure point | Item status after | `planRejectionReason` set? | Recovery affordance visible |
|---|---|---|---|
| `transitionStatus` throws | unchanged (e.g. still `review`) | no | Resubmit this box (case 6) |
| `rejectPlan` throws (after transition succeeded) | `ready` | no | `PlanVerdictBox`'s "Request Changes" (item shows `pending_review`, no reason yet) |
| `triggerTriage` throws (after both prior succeeded), item's `ready→idea` CAS did NOT fire or was rolled back | `ready` | yes (the typed feedback) | `PlanVerdictBox`'s "Regenerate Plan with This Feedback" (item shows `changes_requested`) |
| `triggerTriage` throws *after* its own internal `ready→idea` CAS already committed (`backlog_service_trigger_triage.go`'s step 3b, lines 249-260 — fires before the later artifact-dir/headless-pool checks that can still fail) | **`idea`**, not `ready` | yes (the typed feedback, written by the `rejectPlan` call that ran before `triggerTriage`) | **This box's own Retry action** (`InlineError`'s `onRetry`) — neither `PlanVerdictBox` card renders at `idea` (it's `ready`-only), so recovery has to happen inside this box itself; see plan.md pre-mortem P1 #1 and Story 2.2.1's dedicated AC/task for this row |

The copy above ("Sent back, but retriage didn't start... use Regenerate Plan with This Feedback below") is accurate for the third row. The middle row (`rejectPlan` itself fails) points at "Request Changes" instead, per the table. The fourth row — pre-mortem P1 #1 — needs its own distinct copy and its own distinct *mechanism*, since neither existing `PlanVerdictBox` affordance is visible at `idea`: plan.md's Task 2.2.1a re-fetches the item's actual post-failure status (not just re-showing `load()`'s side effect) and, when it comes back `idea` instead of `ready`, `SendBackFeedbackBox` shows headline **"Sent back, but retriage didn't start"** with body **"The item moved back to \"idea\" before retriage could start. Your feedback is still in this box — click Retry to send it back to \"ready\" and start retriage again."**, plus a working **Retry** button (`InlineError`'s `onRetry`) right there in the error region.

**BLOCKER fix, iteration 3 repair pass:** the previous version of this row's copy read *"Close this form and re-run 'Send back for re-planning' from there, or check the item's triage status directly — resubmitting this box now will fail."* This was wrong on two counts, both code-verified: (1) `SendBackFeedbackBox`'s own `visible` prop is `actions.has("send_back_ready")`, which `CAN_SEND_BACK_READY` (`itemActions.ts:122-127`) never includes `idea` in — so "re-run Send back for re-planning from there" pointed at a toggle button that literally cannot render at `idea`; there is no "there" to close this form and go to. (2) The claim that "resubmitting this box now will fail" is also wrong: `idea→ready` is itself a structurally valid transition (`session/domain/backlog.go`'s `validTransitions` map has `BacklogStatusIdea: {BacklogStatusReady: true, ...}`), and the operator's typed feedback is still sitting in this component's own local textarea state — it was never cleared on this failure path, only on an explicit Cancel/dismiss. So resubmitting the *same* `transitionStatus`→`rejectPlan`→`triggerTriage` chain, just starting from `idea` instead of the original `in_progress`/`review`/`pr_pending`/`done`, is not just possible but is exactly the correct recovery. The fix (plan.md Task 2.1.1b/2.2.1a): keep the feedback text in place, and surface it as a real, wired `onRetry` on the `InlineError` — reusing the component's own existing "no misleading retry" convention correctly here, since this retry is genuinely accurate, unlike the third row's case where `onRetry` is deliberately withheld.

*(A separate constraint was traced through and documented, not silently assumed: `TransitionGuard` additionally requires the item's `AcCriteria` to be non-empty for the `idea→ready` edge specifically — `session/domain/backlog.go:617-621`. This doesn't block the fix: an item can only reach the `in_progress`/`review`/`pr_pending`/`done` statuses this feature targets by first passing through `ready`, and the only two edges that reach `ready` from an idea-shaped item already require non-empty `AcCriteria` — so an item that got far enough to trigger this failure mode already satisfies the retry's precondition by construction. See plan.md Story 2.2.1's dedicated note for the full trace and the fallback behavior if this ever is hit anyway.)*

---

## 8. Escape-to-cancel flow

Applies only while the form is idle or showing a resolved error — **not**
while a submit is in flight. See Surface 4: while `isPending` is `true`,
`Escape` (and clicking Cancel) is a no-op, matching Submit's own gating.

```
Operator has form open, textarea contains "redo the auth approach".
Presses Escape while textarea is focused (isPending is false).
   │
   ▼
Form closes immediately (no confirmation).
Textarea value resets to "" (not preserved — distinct from the
error-preserves-text case; this is an intentional cancel, not a failure).
Focus moves to the toggle button (`toggleRef.current?.focus()`).
   │
   ▼
┌─────────────────────────────────────┐
│  [ ↩ Send back for re-planning ]     │  ← focused, aria-expanded="false"
└─────────────────────────────────────┘
```

Same mechanism as clicking Cancel — both call `handleCancel()`.

---

## 9. Empty/validation state

```
Textarea empty →  [ Submit ]  greyed out, aria-disabled="true", disabled
Textarea has only whitespace ("   ") → same (feedback.trim().length === 0)
Textarea has ≥1 non-whitespace char → [ Submit ]  enabled
```

No inline validation message is shown for the empty case (matches both precedents — the disabled-button state *is* the validation signal, no separate "this field is required" text). This is consistent with `PlanVerdictBox` and `TriageReviewPanel`, neither of which shows a validation message either.

---

## UX Acceptance Criteria

All are human-testable against the running web app (`make install-service` build or a manual dev instance per this repo's `CLAUDE.md`).

### Task completion
1. **User can send an item back with feedback in ≤ 3 clicks**: click toggle → click into textarea and type → click Submit. (2 clicks + 1 typing action; no dialog, no second confirmation step.)
2. **User can cancel with 1 click or 1 keypress**: click "Cancel" or press `Escape` while the textarea is focused — both close the form immediately with no confirmation prompt. **Except while a submit is in flight** (`isPending`, Surface 4): Cancel is rendered `disabled`/`aria-disabled="true"` and `Escape` is a no-op, matching Submit's own pending-gating — both call the same `handleCancel`, which early-returns while `isPending` is `true` (BLOCKER fix, plan.md Task 2.1.1b). This prevents the form from visually collapsing as if aborted while the in-flight `transitionStatus`→`rejectPlan`→`triggerTriage` chain is still running unseen.
3. **The success path requires no navigation away from the item detail view** — the toast and the updated `PlanVerdictBox` card both render in place; the operator never leaves `BacklogItemDetail.tsx`. **No focus-restore is attempted on success** (resolved decision, see "Decisions made in this design pass" below) — unlike Cancel/Escape, which restore focus to the toggle button because the form (and toggle) are still present in that case. On success the item's status moves away from the send-back-eligible set, so `SendBackFeedbackBox`'s own toggle is typically no longer rendered after `load()` completes and there is usually no stable element to restore focus to; focus is left wherever the DOM naturally puts it after the re-render, rather than the form attempting to direct it anywhere specific.

### Error states
4. **Call-1 failure**: shows headline **"Failed to send back"**, body `getErrorMessage(err, "Failed to send back.")` — the real server error string when available, otherwise the literal **"Failed to send back."** (matching plan.md's actual code, per Surface 6) — form stays open with typed text intact, Submit re-enabled. Exit path: resubmit, or Cancel/Escape to abandon.
5. **Call-2/3 (partial) failure**: shows headline **"Sent back, but retriage didn't start"**, body directs the operator to the item's own `PlanVerdictBox` card (either "Request Changes" or "Regenerate Plan with This Feedback" depending on which call failed — see table above; **iteration 3 repair pass:** the third row now names "Regenerate Plan with This Feedback," not the plain "Trigger Triage" button, since that button drops the feedback — BLOCKER A) rather than resubmitting this box. Exit path: Cancel/dismiss this form, then use the pointed-to affordance on `PlanVerdictBox` — verified reachable without a page reload. When `triggerTriage` fails *after* its own internal `ready→idea` transition has already committed (table row 4 above), the body instead names the item's actual `idea` status **and offers a working Retry button right in the error region** (`InlineError`'s `onRetry`) rather than pointing at `PlanVerdictBox` (which isn't showing) or claiming resubmission will fail (BLOCKER B, iteration 3 repair pass — the earlier copy did both) — verified by the re-fetch-and-branch logic in Task 2.2.1a/2.1.1b, not assumed to always be `ready`. The toast fired ahead of this in-form message reads **"Send-back needs attention — see details below."**, distinct from case 6's generic failure toast, so the two never contradict each other (CONCERN fix, iteration 2 repair pass). This in-form message is guaranteed to actually render — including after the item's status has already moved outside `CAN_SEND_BACK_READY` — because `SendBackFeedbackBox` stays mounted whenever it has a pending error to show (BLOCKER fix, same pass; see Surface 7's mechanism note and plan.md Task 2.1.1b). **The same mount guard also covers the row-4 Retry's own pending phase** (BLOCKER fix, iteration 4 repair pass): clicking that Retry clears `actionError` and sets `isPending` while `visible` is still `false`, and the guard's added `!isPending` clause keeps the form, textarea, and "Sending…" button on screen throughout — not just its own initial error and the state after it resolves — so the pending UI (Surface 4) is never invisible mid-retry.
6. **No dead ends**: every error state in this document (cases 6, 7) has at least one visible, clickable exit — either "Dismiss" + a still-enabled Submit, or Cancel, a named alternate affordance (`PlanVerdictBox`'s buttons), or — for the `idea`-landing row specifically — a working in-place **Retry** (BLOCKER B, iteration 3 repair pass: the previous copy for this row pointed at a toggle that cannot render at `idea` and claimed resubmission would fail, both wrong; see table row 4 and its BLOCKER-fix note above). No error state leaves the operator with only a disabled UI and no path forward. This now also holds once the item's status has moved outside `CAN_SEND_BACK_READY` (case 7's `ready`/`idea` outcomes): `SendBackFeedbackBox` stays mounted — rather than unmounting and silently dropping the error — for as long as an unresolved `actionError` is pending, and both Cancel and the `InlineError`'s Dismiss clear it, letting the component unmount cleanly on its next render (BLOCKER fix, iteration 2 repair pass; plan.md Task 2.1.1b).
7. **Active-session notice never blocks submission** — with `activeWorkSessionCount > 0`, Submit remains clickable as soon as text is typed; the `InlineNotice` is present but does not gate the button's `disabled`/`aria-disabled` state.

### Accessibility (matches `PlanVerdictBox.tsx`'s existing ARIA conventions, per ux.md §3)
8. Toggle button exposes `aria-expanded` reflecting form open/closed state, and is reachable and activatable via keyboard alone (`Tab` to focus, `Enter`/`Space` to activate).
9. Form wrapper is `<div role="form" aria-label="Send back for re-planning">` — a screen reader announces it as a named form landmark when focus enters it.
10. Textarea has a true `<label htmlFor="send-back-feedback">` (not `aria-label`-only) pointing at a matching `id`; screen reader announces the label before the field.
11. Submit button keeps `aria-busy` and `aria-disabled`/`disabled` set together (never one without the other) across idle → pending → resolved transitions. The `idea`-row Retry button (BLOCKER B, iteration 3 repair pass) is `InlineError`'s existing `onRetry` button, which inherits that component's own established ARIA handling — no new pattern introduced.
12. Error region (`InlineError`) uses `role="alert" aria-live="assertive"` so a screen-reader user is interrupted with the failure the moment it appears, without needing to navigate to find it.
13. `Escape` inside the textarea closes the form and moves focus to the toggle button — verified via keyboard only, no mouse.
14. Color contrast of all new text (label, placeholder, button text, `InlineNotice` and `InlineError` text) is ≥ 4.5:1 against its background, matching the existing `PlanVerdictBox.css.ts`/`InlineError`/`InlineNotice` styles this component reuses verbatim (no new colors introduced — Task 2.1.1a forks existing style blocks unchanged).
    - **Mobile/touch targets**: no new mobile-specific work is needed. `SendBackFeedbackBox`'s toggle/Cancel/Submit buttons fork `PlanVerdictBox.css.ts`'s `buttonBase` style verbatim (Task 2.1.1a), which sets `minHeight: "44px"` (confirmed by reading `web-app/src/components/backlog/PlanVerdictBox.css.ts:127-136`) — the standard minimum touch-target size — so this component inherits that sizing and `PlanVerdictBox`'s existing responsive layout automatically.
15. All interactive elements (toggle, textarea, Cancel, Submit, notice/error dismiss) are reachable in a single forward `Tab` sequence with no keyboard trap, and the tab order matches visual order (toggle → notice dismiss if present → textarea → error dismiss if present → Cancel → Submit).

### Consistency / regression
16. The removed dead `send_back_refining` code path produces zero matches when the codebase is grepped for that string in `BacklogItemDetail.tsx` (mirrors plan.md Story 2.2.1's own acceptance criterion — a UX-visible consequence: no button anywhere fires a "Sent back to refining." toast that used to be silently unreachable).
17. The unrelated `send_back_idea` ("↩ Return to Triage") button is visually and behaviorally unchanged — this feature does not alter it (requirements.md Out of Scope).

---

## Decisions made in this design pass

- **Single combined pending state, not a 3-step progress indicator.** Considered exposing "Transitioning… → Recording feedback… → Starting retriage…" as the button label cycles through the three calls, but rejected: the app has no existing multi-step-progress convention on a single button (ux.md §1 — every precedent is a flat pending/not-pending boolean), and requirements.md frames this explicitly as "a single operator action," which a 3-step progress readout would undercut by re-exposing the multi-call implementation detail the requirement says to hide.
- **Distinct error copy for the partial-failure case (case 7), not a shared generic message.** This directly follows ux.md §4's own instruction ("don't offer a 'Retry' affordance that would re-attempt the wrong half of a two-step operation") and the task prompt's explicit callout that the reviewer is checking this. The generic fallback body, `getErrorMessage(err, "Failed to send back.")`'s literal `"Failed to send back."` (see Surface 6), is reserved for case 6 only, where it's accurate.
- **No focus-to-toggle on success, unlike Cancel/Escape (resolved, not left open).** Considered matching Cancel's focus-restore behavior for consistency, but rejected: a successful submit moves the item's status away from the send-back-eligible set (`in_progress`/`review`/`pr_pending`/`done` → `ready`), so per `CAN_SEND_BACK_READY` (`itemActions.ts:122-127`) `SendBackFeedbackBox`'s own toggle is typically no longer rendered after `load()` completes — there usually isn't a stable element left to restore focus to. Restoring focus to a toggle that's about to disappear would either strand focus on a vanished element or misdirect the operator away from the new state (the updated `PlanVerdictBox` card) they most need to see. Decision: attempt no focus-restore on success at all — leave focus wherever the DOM naturally puts it after the re-render. This is a deliberate, documented choice (see UX Acceptance Criteria #3), not an unresolved open question for implementation review to settle.
- **Keep `SendBackFeedbackBox` mounted while an unresolved `actionError` is pending, even once `visible` goes false (BLOCKER fix, iteration 2 repair pass).** A fresh UX triad-lens review, code-verified against plan.md's own samples, found that the original `if (!visible) return null;` guard unmounted the component — discarding `actionError` — the instant `handleSendBackWithFeedback`'s catch-block `load()` re-fetched the item into a `ready`/`idea` status outside `CAN_SEND_BACK_READY`, which is every partial-failure outcome in case 7's table. That meant Surface 7's whole carefully-designed distinct error copy could never actually reach the operator. Two options were available: move the error display out of `SendBackFeedbackBox` into a different, always-mounted component, or keep `SendBackFeedbackBox` itself mounted whenever it has an error to show. The smaller, more surgical option was chosen: the guard becomes `if (!visible && !actionError) return null;` (plan.md Task 2.1.1b), the component's own toggle button is hidden once `!visible` (the action is no longer eligible, so offering it again would be wrong), and both Cancel and the `InlineError`'s Dismiss clear `actionError` so the component still unmounts cleanly once the operator is done with it. See Surface 7's mechanism note above for the full trace.
- **Point Surface 7's row-3 copy at "Regenerate Plan with This Feedback," not "Trigger Triage" (BLOCKER A, iteration 3 repair pass).** A fresh UX triad-lens review, code-verified against `ActionsSection.tsx`/`PlanVerdictBox.tsx`, found that the plain "Trigger Triage" button calls `triggerTriage(item.id)` with no feedback argument, and the backend never reads feedback from the stored `PlanRejectionReason` field — only from the RPC request. Following the old copy's instruction would have silently dropped the operator's already-recorded feedback. `PlanVerdictBox`'s "Regenerate Plan with This Feedback" button is the one that actually forwards `item.planRejectionReason`, and it's simultaneously visible in this exact state — so the copy now names it specifically.
- **Give the `idea`-landing row (Surface 7, row 4) a real in-place Retry instead of a dead pointer (BLOCKER B, iteration 3 repair pass).** The previous copy told the operator to "close this form and re-run Send back for re-planning from there" — but `SendBackFeedbackBox`'s own `visible` prop excludes `idea` (`CAN_SEND_BACK_READY` never includes it), so there is no "there": that toggle cannot render at `idea`. The copy also claimed resubmitting would fail, which is also wrong — `idea→ready` is a structurally valid transition, and the operator's typed feedback is still sitting untouched in the component's own local state (this failure path never clears it). Two fix shapes were considered: (a) build a new, separate recovery surface elsewhere in the item detail view for this one case, or (b) keep the fix local — surface a real `onRetry` on the `InlineError` already rendered in this exact spot, reusing `handleSubmit`'s existing logic (which already reads the current feedback text and, via the parent's `useCallback` closure, the current — now `idea` — status). Option (b) was chosen: it's the smaller change, needs no new component or affordance, and correctly reuses `InlineError`'s own "wire `onRetry` only when the retry is real" convention — this is the one row in Surface 7 where that condition is actually met. A constraint surfaced while tracing this through (`TransitionGuard`'s `AcCriteria`-non-empty requirement on `idea→ready`) was checked and found non-blocking by construction; see the table-row note above for the full trace.
- **Add an `isPending` escape hatch to the mount guard, not a second parallel pending flag (BLOCKER, iteration 4 repair pass).** A fresh UX triad-lens review, code-verified, found that row 4's own Retry blanks the whole component during its own pending phase: `handleSubmit` sets `localPending` true and clears `actionError` to `null` before `onSubmit` resolves, while `visible` is still `false` throughout (the item stays at `idea` until the retry's chain settles) — so `if (!visible && !actionError) return null;` evaluated `true` for the entire retry, unmounting the toggle, form, "Sending…" button, and `aria-busy` indicator, and remounting only once the promise settled. The fix reuses the `isPending` value the component already computes for Submit's own pending gating (`const isPending = localPending || actionPending;`) rather than introducing a second, parallel pending flag: `if (!visible && !actionError && !isPending) return null;` (plan.md Task 2.1.1b). Traced against every state the component can be in (`visible+idle`, `visible+pending`, `visible+error`, `!visible→unmounted`, `!visible+pending` — the bug — and `!visible+error` — the iteration-2 fix): the new clause changes behavior only for `!visible+pending`, which now renders instead of unmounting; every other combination, including the "pending finishes with no error and the item stays outside `CAN_SEND_BACK_READY`" case, still correctly returns `null` on its next render, so the component does not get stuck mounted forever.

## Residual gap flagged for implementation

None remaining as of the Phase 4 plan-repair pass (iteration 4): iteration 1 closed pre-mortem P1 #1 and BLOCKER 5.2 (distinct per-failure copy); iteration 2 closed the UX triad-lens BLOCKER that made Surface 7's copy unreachable in practice (`SendBackFeedbackBox` unmounting before its own error could render) and its accompanying CONCERN (the outer toast's wording contradicting the in-form message); iteration 3 closed two further BLOCKERs — Surface 7 row 3 pointing at the wrong button ("Trigger Triage" instead of "Regenerate Plan with This Feedback," which silently dropped feedback) and row 4 pointing at a nonexistent affordance while claiming a genuinely-working retry would fail (now a real in-place Retry) — plus a CONCERN about `actionError`'s cross-re-render persistence resting on an unverified batching assumption (now covered by Task 2.2.1d's integration test using real async Promise timing); this iteration closes one further BLOCKER from a fresh UX triad-lens pass — row 4's own Retry blanking the entire component (toggle, form, "Sending…"/`aria-busy` UI) for the duration of its own pending phase, since it clears `actionError` to `null` while `visible` is still `false` — fixed by extending the mount guard with the component's existing `isPending` flag rather than a new one.
