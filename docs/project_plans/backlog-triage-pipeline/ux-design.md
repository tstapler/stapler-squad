# Backlog Triage Pipeline — UX Design

**Project**: backlog-triage-pipeline
**Date**: 2026-05-18
**Status**: UX design complete

---

## Overview

This document provides UX guidance for all new UI surfaces introduced by the backlog triage pipeline: the vagueness prompt interstitial, the triage review panel inside `BacklogItemDetail`, and the error and empty states. It also covers interaction design decisions, accessibility requirements, and usability risk mitigations.

---

## 1. User Flow

The full journey from item creation through triage completion is a linear pipeline with two optional branch points: the vagueness check and the review decision.

```
[1] User fills BacklogItemForm
         |
         v
[2] Submit → CreateBacklogItem
         |
         v
[3] Client evaluates vagueness
    (description < 80 chars AND 0 AC)
         |
    YES  |  NO
    ─────┴──────────────────────────┐
    |                               |
    v                               v
[4a] Vagueness Prompt              [4b] Item saves, form closes
     interstitial appears               ↓
     (modal overlay)              [5] Triage auto-fires (server-side)
         |                              |
    ─────┴──────┐               [6] TriageLoadingIndicator
    "Refine"    "Run anyway"           visible in detail pane
       |             |                 |
       v             v                 v (polling / push resolves)
[4c] Form     [5] Triage          [7] triageStatus === "completed"
    re-opens       auto-fires          |
    focused on                         v
    description             [8] TriageReviewPanel renders
                                       |
                               ────────┴──────────┐
                               "Apply"          "Skip"
                                 |                 |
                                 v                 v
                          [9a] AC replaced    [9b] Panel dismissed
                               + status → ready   (localStorage flag)
                               + success toast
```

Steps 4a–4c are the vagueness branch for R2. Steps 8–9 are the diff/review flow for R4–R5.

---

## 2. UI Pattern Recommendations

### 2.1 Vagueness Prompt — Pattern: Modal Dialog with Explicit Choice

**Recommendation**: Present the vagueness prompt as a modal dialog (not a toast, not inline in the form) that blocks the next action until the user makes an explicit choice.

**Justification**:
- Nielsen Heuristic #5 (Error prevention): The purpose of this prompt is to prevent a low-quality triage run. An interstitial that requires conscious choice ensures the user reads it, unlike a toast which can be ignored.
- Nielsen Heuristic #3 (User control and freedom): The dialog offers two equally accessible paths — refine or proceed. Neither is presented as "wrong."
- The modal keeps the item creation flow intact: the item already exists in the DB by the time the prompt fires (R1 states the trigger is server-side). The prompt is purely about whether to give the AI more context, not a gate on saving.
- Avoid an inline expansion within the form itself: the form submit has already fired, so reopening it in the same space is confusing about whether the item was created.

**Anti-pattern to avoid**: Do not use a toast with an action button ("Item created — add description?"). Toasts are for transient confirmations of completed actions, not branching decisions. Steve Krug's principle of making options obvious applies: a toast is easy to miss and its action is easy to skip without realizing the consequence.

### 2.2 Triage Review Panel — Pattern: Inline Diff Panel (not a modal)

**Recommendation**: Render the triage review panel as an inline section inside `BacklogItemDetail`, inserted between the existing `TriageLoadingIndicator` position and the Description section. Do not use a modal or side drawer for the diff.

**Justification**:
- The user navigated to the item detail specifically to review this item. The context is already correct. Opening another modal on top of the detail pane adds a modal-within-panel nesting depth that increases cognitive load without benefit.
- Inline placement keeps the diff spatially adjacent to the AC list it is proposing to change, which applies the Gestalt principle of Proximity: suggestions next to current state signal the relationship without verbal explanation.
- The panel is dismissible per-item (localStorage) and has a "Skip" action, satisfying Nielsen Heuristic #3 (undo/escape).
- The diff pattern (left: current, right: suggested) is an established convention in code review tools (GitHub, GitLab). Users recognize it without instruction, satisfying Krug's principle of using familiar patterns.

### 2.3 Cherry-Pick vs Bulk Apply — Decision: Bulk Apply with Pre-Apply Review

**Recommendation**: Implement bulk apply (replace all AC with suggestions) rather than cherry-pick individual suggestions. Provide the diff view as the review step before committing, not a checkbox-per-item selection UI.

**Justification**:
- Triage suggestions are a coherent set derived from a full research pass. Cherry-picking individual items from a set designed to work together produces a hybrid that may be internally inconsistent.
- The diff view already provides the information needed to make a quality judgment: if any single suggestion is unacceptable, the user skips and edits manually. This is the 80% case.
- Cherry-pick UI requires a checkbox list, a "select all / deselect all" affordance, and logic for partial application — this is significant UI complexity for a low-frequency use case (user rarely wants 3 of 5 suggestions from a coherent research pass).
- If cherry-pick is needed in a future sprint, it can be added to the diff panel without breaking the bulk-apply default.

### 2.4 Confirmation Before Destructive Apply — Decision: Undo Toast (no confirmation dialog)

**Recommendation**: No confirmation dialog before apply. Show an undo-capable success toast after the operation completes: "Triage applied — item is now ready. Undo."

**Justification**:
- Nielsen Heuristic #5 (Error prevention) vs. Heuristic #3 (User control): A confirmation dialog ("Are you sure you want to replace your AC?") is appropriate when the action is truly irreversible. Here, the user has just reviewed the diff and clicked a clearly-labeled "Apply suggestions" button. The diff already served as the review step. A second confirmation dialog introduces unnecessary friction.
- An undo toast provides the safety net without blocking the primary flow. The toast should persist for 6–8 seconds (longer than the 3-second default because the action is more consequential than a navigation action).
- "Undo" in the toast triggers `UpdateBacklogItem` with the previous AC list (the client should cache the pre-apply state in component state, not require a server round-trip to retrieve it).
- Krug's principle: "Don't ask me to confirm things that are obviously what I just asked for." The user clicked "Apply suggestions" after reviewing the diff. Asking "are you sure?" treats the user as error-prone rather than intentional.

---

## 3. All Key States

### 3.1 TriageReviewPanel States (inside BacklogItemDetail)

| State | Trigger condition | What to render |
|---|---|---|
| Loading (triage running) | `triageStatus === "running"` | Existing `TriageLoadingIndicator` (no change) |
| Triage complete — with suggestions | `triageStatus === "completed"` AND `triageResult.suggestions.length > 0` AND panel not dismissed | Full diff panel with description comparison + AC diff + Apply/Skip buttons |
| Triage complete — no suggestions | `triageStatus === "completed"` AND `triageResult.suggestions.length === 0` AND panel not dismissed | Summary-only panel with "No AC changes suggested" message + Skip button |
| Dismissed | localStorage flag set for item ID | Render nothing (panel absent) |
| Apply in progress | User clicked Apply, RPC in flight | Apply button shows spinner, disabled; Skip button disabled |
| Apply error | `UpdateBacklogItem` or `TransitionBacklogItemStatus` failed | Error banner inside panel with retry button; diff still visible |
| Triage result absent | `triageStatus === "completed"` but `triageResult` is null/undefined | Do not show panel; treat as dismissed (defensive — should not happen in normal flow) |

### 3.2 Vagueness Prompt States

| State | Trigger | Render |
|---|---|---|
| Not shown | description >= 80 chars OR acCriteria.length >= 1 | Never shows |
| Open | Item submitted, vagueness threshold met | Modal dialog, two actions |
| Refine selected | User clicks "Add more detail" | Modal closes, form re-opens focused on description field; triage deferred |
| Proceed selected | User clicks "Run triage anyway" | Modal closes, triage fires |

### 3.3 BacklogItemForm — Vagueness Hook Point

The existing `handleSubmit` in `BacklogItemForm` calls `onSubmit(data)` and awaits it. The vagueness check should be inserted between successful submit and form close:

```
onSubmit(data) resolves successfully
       |
       v
isVague(data) = data.description.trim().length < 80 AND data.acCriteria.length === 0
       |
  YES  |  NO
───────┴──────
|             |
v             v
setShowVaguenessPrompt(true)    onFormDone()
(modal renders, form stays
 hidden behind overlay)
```

The form component should accept an optional `onVaguenessDetected` callback or the parent should own the vagueness check post-submit. The form itself does not need to change its submit logic — the parent (`BacklogPage` or equivalent) intercepts the resolved promise and decides whether to show the prompt.

---

## 4. ASCII Mockups

### 4.1 Vagueness Prompt — Modal Overlay

```
┌─────────────────────────────────────────────────────────┐
│                                                         │
│  Item created.                                          │
│                                                         │
│  "Add error handling to auth service"                   │
│                                                         │
│  The description is brief and has no acceptance         │
│  criteria. Triage works best with more context.         │
│                                                         │
│  What would you like to do?                             │
│                                                         │
│  ┌──────────────────────┐  ┌────────────────────────┐  │
│  │  Add more detail     │  │  Run triage anyway     │  │
│  │  (opens item editor) │  │                        │  │
│  └──────────────────────┘  └────────────────────────┘  │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

Notes:
- Dialog is centered in the viewport, not anchored to the form.
- "Add more detail" is the visually primary button (filled style) because it produces the better downstream outcome. "Run triage anyway" is secondary (outlined).
- The item title is displayed in the dialog so the user has context about which item triggered the prompt (especially relevant if they submitted quickly and the dialog appears after a short delay).
- No close/dismiss button. The user must choose one of the two options. The item is already saved; closing the dialog without choosing would leave the user uncertain whether triage fired.
- `role="dialog"`, `aria-modal="true"`, `aria-labelledby` pointing to the heading.

### 4.2 Triage Review Panel — Full Diff (inside BacklogItemDetail)

```
┌─────────────────────────────────────────────────────────────┐
│  [Triage Ready]                           [Skip] ×          │
│  Reviewed 4 research areas — 3 suggestions                  │
│                                                             │
│  Summary                                                    │
│  ─────────────────────────────────────────────────────────  │
│  The auth service error handling needs structured           │
│  logging, retry limits, and clear HTTP status codes.        │
│  The existing try/catch is swallowing context.              │
│                                                             │
│  Suggested Acceptance Criteria                              │
│  ─────────────────────────────────────────────────────────  │
│                                                             │
│  CURRENT (0 criteria)         SUGGESTED (3 criteria)        │
│  ─────────────────────        ──────────────────────────    │
│  (none)                   +   Auth errors log at ERROR      │
│                               level with request ID         │
│                           +   Retry limit of 3 with         │
│                               exponential backoff           │
│                           +   4xx vs 5xx status codes       │
│                               returned correctly            │
│                                                             │
│  [View plan]               [Skip — review later]            │
│  ↑ only if planArtifactsPath                                │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Apply suggestions  (replaces AC + marks ready)     │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

Layout notes:
- Panel has a distinct background (slightly elevated surface, like `--card-background`) with a left border accent in the status color for "idea" items (indicates this is a pre-ready state action).
- "Triage Ready" heading uses `<h3>` consistent with other `sectionTitle` headings in the detail pane.
- The dismiss button (`[Skip] ×` top right) and the "Skip — review later" button are both available and both set the localStorage dismissed flag. Having two dismiss affordances — one in the header for quick dismissal and one near the primary action button for deliberate skip — reduces the chance the user applies accidentally while trying to dismiss.
- Diff uses `+` prefix and a visual accent (green tint or border-left) for added items. No `-` items since current state is "0 criteria" in the new-item case. For items with existing AC, removed items should use red tint and `−` prefix.
- "Apply suggestions" button is full-width at the bottom of the panel, high visual weight. This is the primary CTA.
- "View plan" is a secondary text link, only rendered if `planArtifactsPath` is non-null.

### 4.3 Triage Review Panel — No Suggestions

```
┌─────────────────────────────────────────────────────────────┐
│  [Triage Ready]                           [Skip] ×          │
│                                                             │
│  Summary                                                    │
│  ─────────────────────────────────────────────────────────  │
│  The auth service error handling looks well-specified.      │
│  No additional acceptance criteria needed.                  │
│                                                             │
│  No AC changes suggested.                                   │
│  You can mark this item ready manually.                     │
│                                                             │
│                              [Mark ready]  [Skip]           │
└─────────────────────────────────────────────────────────────┘
```

Notes:
- No diff section renders. Summary-only.
- "Mark ready" button triggers the existing `transitionStatus(item.id, "ready")` action, shortcutting the need for the user to scroll down to the Actions section. This is a convenience — same RPC already available in the actions panel.
- "Skip" dismisses the panel without transitioning.

### 4.4 Apply Error State

```
┌─────────────────────────────────────────────────────────────┐
│  [Triage Ready]                                             │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  ⚠  Failed to apply suggestions                     │   │
│  │  The item may have been updated by another process. │   │
│  │  Reload and try again.                              │   │
│  │                                                     │   │
│  │  [Reload item]          [Skip without applying]     │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  (diff still visible below for reference)                   │
└─────────────────────────────────────────────────────────────┘
```

Notes:
- Error banner renders inside the panel, above the diff, not as a page-level error state. This preserves the user's context (the diff they were reviewing) while surfacing the failure.
- "Reload item" calls `load()` (existing refresh function) to get fresh state before retry.
- "Skip without applying" allows the user to dismiss the panel without retrying, preserving the dismiss-as-escape pattern.
- Error text avoids technical language. "The item may have been updated by another process" covers the optimistic concurrency failure case mentioned in R5.
- `role="alert"` on the error container ensures screen readers announce it immediately.

### 4.5 Apply In Progress State (within the panel)

```
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Applying…  [spinner]                               │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  [View plan]     [Skip — review later]  (disabled)         │
```

Notes:
- Apply button label changes to "Applying…" with an inline spinner (not a full overlay).
- All interactive elements in the panel are disabled during the in-flight state.
- Using a button-level spinner rather than a panel overlay avoids the "entire panel disappears and is replaced by a loading state" problem, which would disorient the user and remove the diff they were reviewing.

---

## 5. Interaction Decisions Summary

| Decision | Recommendation | Rationale |
|---|---|---|
| Vagueness prompt placement | Modal overlay, post-submit | Blocks accidental skip; item already saved, so not a form gate |
| Cherry-pick vs bulk apply | Bulk apply | Suggestions are a coherent set; diff view provides sufficient pre-apply review |
| Confirmation before apply | No dialog; undo toast instead | Diff view is the review step; a second confirmation is friction without safety benefit |
| Undo toast duration | 6–8 seconds | Longer than standard toast (3s) because the action is consequential |
| Dismiss affordance | Two: header × and footer Skip | Reduces accidental apply, gives clear exit at both scan-end and action-end |
| Error treatment | Inline panel banner, not page error | Preserves diff context during error recovery |
| "No suggestions" state | Summary-only + "Mark ready" shortcut | Avoids dead panel with no action path |
| Plan link visibility | Conditional on `planArtifactsPath` | Only renders when there is something to link to |

---

## 6. Notification Deep-Link — URL-Driven Detail Pane

R6 requires that notification clicks navigate to `/backlog?item=<item_id>` and open the item detail panel for that item.

The existing `BacklogItemDetail` component is driven by an `itemId` prop. The backlog page needs to:
1. Read `?item=<id>` from `useSearchParams()` on mount.
2. Pass that `itemId` to `BacklogItemDetail` (or the sidebar/panel that wraps it).
3. If the item is not in the current filtered view, either clear filters or show the item regardless of filters.

**UX recommendation**: When the URL contains `?item=<id>`, always show the item detail regardless of active filters. Do not silently fail to open the panel because the item is filtered out of the list. Show a banner: "Showing item from notification link — some filters are active." This satisfies Nielsen Heuristic #1 (visibility of system status).

If `item_id` in the URL refers to a non-existent item (deleted, archived), show the existing item-not-found error state in the detail pane rather than silently ignoring the URL parameter.

---

## 7. Accessibility Requirements

### 7.1 Vagueness Prompt Modal

- `role="dialog"` and `aria-modal="true"` on the modal container.
- `aria-labelledby` pointing to the dialog heading ("Item created").
- Focus must move to the first focusable element inside the dialog when it opens (the "Add more detail" primary button).
- Focus must be trapped inside the modal while it is open. Tab and Shift+Tab cycle only between the two buttons.
- Keyboard: Enter or Space activates focused button. There is no Escape dismissal (the user must choose).
- When the dialog closes, focus returns to the element that triggered it (the form submit button or its containing area).

### 7.2 Triage Review Panel

- The panel must be announced to screen readers when it appears. Use `aria-live="polite"` on a container that is present in the DOM before the panel renders, or use `role="status"` on the panel itself.
- The "Triage Ready" heading uses `<h3>` (consistent with other section headings in `BacklogItemDetail` which already use `<h3>` for `sectionTitle`).
- The diff section should use `<dl>` (definition list) semantics: `<dt>Current</dt><dd>…</dd><dt>Suggested</dt><dd>…</dd>` — or a `<table>` with `<thead>` if the two-column layout is implemented as a grid table.
- Added items in the diff must not rely on color alone for conveying "new". Use a `+` prefix or `aria-label="Added:"` on each added item.
- Removed items must use a `-` prefix or `aria-label="Removed:"` in addition to visual styling.
- "Apply suggestions" button must have a descriptive `aria-label`: `"Apply triage suggestions — replaces acceptance criteria and marks item ready"`. The visible label "Apply suggestions" is acceptable, but the aria-label provides fuller context for screen reader users who may not have read the panel heading.
- Apply in-progress state: the button should use `aria-busy="true"` and `aria-label="Applying suggestions"` to announce the state change.
- Error banner must use `role="alert"` so it is announced immediately without the user navigating to it.

### 7.3 Focus Management

- When `triageStatus` transitions from `running` to `completed` and the panel appears, do not auto-move focus to the panel. The user may be reading another part of the detail pane. Announce via `aria-live="polite"`.
- When "Apply" succeeds and the panel unmounts, return focus to the first focusable element in the detail pane header (the close button or item title, whichever is focusable).
- When "Skip" is pressed and the panel dismisses, same focus return behavior.

### 7.4 Color Contrast

- Diff "added" items: the green accent or `+` prefix must meet 4.5:1 contrast ratio against the panel background. Do not use low-contrast green-on-white for the tint — use a dark text color with a light green background (`--success-bg` token exists) rather than green text.
- The panel's left border accent (status indicator) is decorative — it does not convey information alone, as the heading "Triage Ready" provides the text alternative.
- Error state: red warning icon (`⚠`) is decorative; the text message conveys the error. Error text must meet 4.5:1 contrast.

### 7.5 Keyboard Navigation

| Element | Keyboard behavior |
|---|---|
| Vagueness modal | Tab cycles between 2 buttons; Enter/Space activates |
| Panel "Apply" button | Tab-focusable; Enter/Space activates |
| Panel "Skip" button | Tab-focusable; Enter/Space activates |
| Panel dismiss × | Tab-focusable; Enter/Space activates; aria-label="Dismiss triage review" |
| Panel "View plan" link | Tab-focusable; Enter activates (standard link) |
| Error "Reload item" button | Tab-focusable; Enter/Space activates |

---

## 8. Usability Risks and Mitigations

### Risk 1: Auto-trigger surprise

**Problem**: Users who create an item and immediately navigate away will see a triage running indicator when they return. Without explanation of why, this is confusing.

**Mitigation**: The `TriageLoadingIndicator` label "Thinking about acceptance criteria..." (which already exists for the manually triggered case) is adequate context. Ensure this same copy is used for the auto-triggered case. Consider adding a one-time tooltip or informational callout the first time a user sees the auto-trigger indicator: "Triage runs automatically on new items." This can be localStorage-gated (show once, never again).

### Risk 2: Notification fatigue

**Problem**: If a user creates several items quickly, they receive a notification per item. R6 specifies `NOTIFICATION_PRIORITY_NORMAL` which is appropriate, but batching should be considered.

**Mitigation**: The notification system is out of scope for this document, but the UX guidance is: the notification message format ("Item title — N suggestion(s). Click to review.") is appropriately terse. Do not use `NOTIFICATION_PRIORITY_HIGH` as specified in R6. If future observation shows notification volume is a problem, a batch digest ("3 triage results ready") is the correct solution, not suppression.

### Risk 3: Destructive apply without review

**Problem**: A user who is not paying attention could click "Apply suggestions" without reading the diff.

**Mitigation**:
- The undo toast (6–8s) provides a recovery path.
- The Apply button is positioned at the bottom of the panel, below the diff, so the user must visually scan past the diff to reach the button. This is intentional friction — the layout enforces the review step through spatial order (Krug: "make the design do the work").
- The button label is "Apply suggestions" not "Accept" or "OK" — the word "suggestions" signals what is being applied, priming the user to have reviewed them.

### Risk 4: Panel reappearing after dismiss

**Problem**: If localStorage is cleared or the user switches devices, the dismissed panel reappears. This is unexpected if the user already applied suggestions or manually edited AC.

**Mitigation**: The dismissed state check should also inspect whether `triageStatus` has been superseded. Specifically:
- If `item.status === "ready"` (not "idea"), the triage review panel should never show, even if the localStorage dismiss flag is absent. The status transition to "ready" implies the suggestions were either applied or the user manually addressed AC.
- This makes the panel's visibility condition: `triageStatus === "completed" AND item.status === "idea" AND !isDismissed(item.id)`.

### Risk 5: Vagueness prompt races with optimistic UI

**Problem**: The vagueness check fires client-side on form submit, but the server also auto-triggers triage immediately. If the user chooses "Add more detail" and the form re-opens, triage is already running on the thin description. The user edits description, saves, and the triage result is for the original thin version.

**Mitigation**:
- The requirements specify `skip_triage: true` as an opt-out flag on `CreateBacklogItem`. When the vagueness check fires and the user selects "Add more detail", the original `CreateBacklogItem` call should have been made with `skip_triage: true` (the client knows it detected vagueness before submitting).
- The hook point in `BacklogItemForm` must evaluate vagueness synchronously before calling `onSubmit`, and pass `skipTriage: isVague` as part of the submitted data.
- When the user re-submits the refined description via the re-opened form, the `UpdateBacklogItem` call should trigger `TriggerTriage` explicitly (or the `CreateBacklogItem` was already made with `skip_triage: false` and another call is needed).
- This requires the parent to know whether the item was created with skip_triage and whether triage has since been triggered. The simplest implementation: if `item.triageStatus === null` after "Add more detail" flow, show a "Start triage" button in the detail pane, or trigger triage automatically when the description is updated via `UpdateBacklogItem`.

---

## 9. Component Placement Summary

| New component | Location | Replaces/Augments |
|---|---|---|
| `VaguenessPromptModal` | New file, near `BacklogItemForm` | Augments — modal rendered from parent page context |
| `TriageReviewPanel` | New file, imported into `BacklogItemDetail` | Replaces the `null` render for `triageStatus === "completed"` |
| `TriageDiffSection` | Sub-component inside `TriageReviewPanel` | New, handles the two-column diff display |
| `TriageErrorBanner` | Sub-component inside `TriageReviewPanel` | New, inline error state |

In `BacklogItemDetail`, the `TriageReviewPanel` replaces the current conditional:

```
// CURRENT: triageStatus === "completed" renders nothing
// FUTURE: triageStatus === "completed" AND status === "idea" AND !dismissed renders TriageReviewPanel
```

The existing condition `item.triageStatus === "running"` that renders `TriageLoadingIndicator` is unchanged.

---

## UX Readiness Gate

- [x] User flow mapped — full journey from item creation through triage apply/skip documented in Section 1
- [x] Key states identified — 7 panel states, 4 vagueness prompt states, hook point in form identified (Section 3)
- [x] Accessibility requirements noted — focus management, ARIA roles, contrast, keyboard navigation (Section 7)
