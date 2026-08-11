# UX Research: Backlog Triage Autonomous Feature

**Date**: 2026-06-22
**Scope**: UI/UX of the triage lifecycle in `web-app/src/components/backlog/` and `web-app/src/app/backlog/`

---

## 1. Current UI State During Triage

### Backlog item detail page (`BacklogItemDetail.tsx`)

When `triageStatus === "running"` and `item.status` is `"idea"` or `"ready"`, the detail pane renders a **`TriageLoadingIndicator`** (non-compact form) in a dedicated section above the description. It shows:
- A spinner (CSS keyframe animation)
- A label that changes at 60 seconds: "Thinking about acceptance criteria..." → "Still thinking — up to 3 min"
- A seconds-elapsed counter (e.g., `42s`)
- A "Stop" button (`aria-label="Cancel triage"`)

The counter increments every 1 second via a local `setInterval`. The component hides itself when `elapsedSeconds >= 180` (TRIAGE_TIMEOUT_SECONDS), leaving a gap — there is no fallback banner shown by the `TriageLoadingIndicator` itself at timeout; the parent must detect `triageStatus === "failed"` to show the failure state.

The detail pane also **polls the backend every 5 seconds** while `triageStatus === "running"`, so the panel transitions to the review or failed state automatically when the session ends.

The `SessionMonitor` component renders inside the detail pane's "Sessions" section for the active triage session. It shows a live terminal snapshot (60 lines) or conversation messages (feature-flagged, `backlog:conversation-view`) and updates every 5 seconds. It includes a text input + quick-action buttons (1, 2, 3, y, n) that can send input to the session — useful for the current broken state where Claude waits at a prompt.

### List page (`web-app/src/app/backlog/page.tsx`)

The list page does **not** show a triage progress indicator in the table rows. Items with `triageStatus === "running"` look identical to those that have never been triaged. The table shows only: Title, Status badge, Priority badge, AC completion ratio, and Updated date.

The `TriageLoadingIndicator` has a `compact` prop (pill form for list contexts) but it is not wired into the list page or the `BacklogItemCard` board card.

### Board view (`BacklogBoard.tsx`, `BacklogItemCard.tsx`)

The board card renders a single action button per item based on `item.status`. For `"ready"` items, it shows "Trigger Triage". For `"idea"` items, it shows "Mark Ready". The card has no triage-in-progress indicator — an item currently being triaged shows the same card face as one that has never been triaged.

---

## 2. Progress Feedback During Autonomous Triage

### What the operator sees

- **Detail pane**: spinner + elapsed timer + "Stop" button. The `SessionMonitor` shows the most recent 60 lines of terminal output, refreshed every 5 seconds. This gives indirect progress visibility if the session is actually working.
- **List page**: nothing. No visual distinction between "idea — never triaged" and "idea — triage in progress."
- **Session list** (main `/?session=...` view): the triage session appears as a regular session with the title/path from the worktree. Nothing in `SessionCard.tsx` uses `session.sessionType` or a backlog role badge to identify it as a triage session. The session name depends entirely on how `TriggerTriage` names the session on the backend.

### What is absent

- No **turn counter** display — the `autonomous_max_turns` field exists in `types_pb.ts` (field 66) but is never surfaced in the triage UI. The operator cannot tell if Claude is on turn 2 of 20 or turn 18 of 20.
- No **per-phase progress** — triage is designed to run subagents for stack research, feature research, architecture research, and pitfalls sequentially. The UI shows a single spinner with no phase labels.
- No **cross-page notification** — if the operator navigates away from the detail pane while triage is running, they get no alert when it completes. The undo toast only fires within the same pane session.

### Cancel button is a stub

`handleCancelTriage` in `BacklogItemDetail.tsx` (line 199–203) is a TODO — it only reloads the item. There is no cancel RPC call. The "Stop" button gives the appearance of control with no actual effect.

---

## 3. Failure UX

### When triage session ends without a result (`triageStatus === "failed"`)

`BacklogItemDetail.tsx` (line 518–524) renders a plain `<div role="alert">` with the message: "Triage encountered an error. Trigger triage manually to retry."

This renders only on `item.status === "idea"`. The message is sparse:
- No distinction between timeout vs. crash vs. session-never-started.
- No link to the session logs.
- No direct "Retry" button — the operator must scroll down to "Actions" and click "Trigger Triage" manually.

The `InlineError` component (`InlineError.tsx`) has richer failure copy: three typed states (`transient`, `timeout`, `permanent`) with a "Retry ↺" button and an optional "View session logs" link. **However, `InlineError` is not used in `BacklogItemDetail.tsx`** — it is only used in `GateVerdictBox.tsx` for review failures. The triage failure path uses the raw inline `<div>` instead of the purpose-built component.

### When triage times out (client-side)

The `TriageLoadingIndicator` renders nothing when `elapsedSeconds >= 180`. The UI goes blank at that point (the indicator vanishes) and shows no error state. The backend poll will eventually update `triageStatus` to `"failed"` when the session ends, at which point the simple failure message appears — but there is a gap window where the user sees no indicator at all.

### Re-triggering

"Trigger Triage" remains available for `"idea"` items regardless of prior triage attempts (no disabled state, no warning about overwriting existing results). If a completed triage result exists, the operator can blindly re-trigger and the new session will overwrite the prior result when `submit_triage_result` is called.

---

## 4. Completion UX

When `submit_triage_result` succeeds (triage session ends with a valid result):

1. The backend sets `triageSession.endedAt` and stores the `triageResult` on the `ItemSession` record.
2. The detail pane's 5-second poll detects `triageStatus === "completed"` and re-renders.
3. The `TriageReviewPanel` appears: "Triage Ready" heading, summary text, two-column AC diff (current vs. suggested), implementation task list with estimates and categories, and two action buttons: "Apply suggestions" or "Skip — review later".

**The item does NOT automatically transition to `"ready"`** — that is an explicit operator decision via "Apply suggestions". This is by design (per the `TriageReviewPanel` comment referencing UX spec Section 3.1).

"Apply suggestions" does two things atomically from the UI perspective:
1. Calls `updateBacklogItem` to replace AC criteria with the suggestions.
2. Calls `transitionStatus(item.id, "ready", "idea")` with an `"idea"` precondition.

On success, a 7-second undo toast appears at the bottom of the viewport via `createPortal` reading "Triage applied — item is now ready." with an "Undo" button.

**There is no persistent notification** — if the operator is not viewing the detail pane when triage completes, they will not be notified. They have to remember to revisit the item.

Dismiss state is persisted to `localStorage` (`triage-panel-dismissed-{id}`), so dismissing on one reload persists across page refreshes, protecting against accidental re-application.

---

## 5. Job-to-be-Done and Friction Points

**Operator's goal**: Convert a raw feature idea ("dark mode toggle") into a "ready" backlog item with acceptance criteria, an implementation task list, and estimates — without doing the research manually. Expected time: 3–5 minutes of autonomous agent work + 30 seconds of human review.

### Current friction points (even after the autonomous fix lands)

1. **No global completion signal**: The operator triggers triage and walks away. They have no way to know when it finishes except by returning to the same detail pane. No badge on the Backlog nav link, no browser notification, no toast visible from the session list.

2. **No turn visibility**: An operator who wants to understand what Claude is doing must scroll to the `SessionMonitor` inside the detail pane and read raw terminal output. There is no "working on step 3 of 5" summary, no phase label, no turn counter. For non-developer operators this is opaque.

3. **Sparse failure messaging**: When triage fails, the operator sees a one-sentence generic error with no actionable next steps beyond "try again." The `InlineError` component already exists with the richer UI pattern — it is simply not wired in.

4. **Cancel is non-functional**: "Stop" button reloads the item data but does not cancel the backend session. An operator who wants to abort a stuck session must go to the session list and kill it there.

5. **No re-triage guard**: After a successful triage result exists, clicking "Trigger Triage" again silently creates a new triage session. There is no confirmation dialog ("This item already has a triage result. Start fresh?"). The old result becomes unreachable the moment the new session completes.

6. **List/board views are blind**: A table of 20 items with 3 in-progress triages looks identical to a table with 0. The compact `TriageLoadingIndicator` was designed for this use case (it has a `compact` prop) but is not wired anywhere in the list or board.

7. **Completion to implementation gap**: After "Apply suggestions" the item moves to `"ready"`. The operator's next step is "Spawn Session" or "Run Autonomously". These buttons are disabled until `skipPlanning || planApproved`, but that flag is only set via "Approve Plan" — which requires `planArtifactsPath` to exist. Since triage does not write `planArtifactsPath`, the operator is presented with greyed-out "Spawn Session" and "Run Autonomously" buttons with no explanation of why they are disabled, and the tooltip reads "Approve the plan or enable skip_planning to spawn a session." — a confusing next step for a fresh item.

---

## 6. Accessibility

### Present

- `TriageLoadingIndicator`: `role="status"`, `aria-live="polite"`, `aria-label` updates in 30-second increments, "Cancel triage" aria-label on the Stop button.
- `TriageReviewPanel`: `aria-live="polite"` on the section, explicit `aria-label="Apply triage suggestions — replaces acceptance criteria and marks item ready"` on the primary action, `aria-busy={isApplying}` during apply.
- `TriageErrorBanner`: `role="alert"` with immediate screen reader announcement.
- Undo toast: `role="status"` rendered via `createPortal`.
- `VaguenessPromptModal`: `role="dialog"`, `aria-modal="true"`, focus trap (Tab cycles between two buttons), focus auto-moved to primary button on open.
- `TriageDiffSection`: `aria-label="Added: {text}"` and `aria-label="Removed: {text}"` on diff rows.

### Absent / Gaps

- No keyboard shortcut to trigger triage (e.g., `T` or `Ctrl+T` from the detail pane). The "Trigger Triage" button is keyboard-focusable but has no `aria-keyshortcuts`.
- No `aria-describedby` on "Trigger Triage" buttons explaining preconditions (the "Mark Ready" button has `aria-disabled` + `title` for the AC requirement, but "Trigger Triage" has neither).
- Cancel triage button: given it is currently a stub (no actual cancel), the button is semantically misleading.
- The `TriageLoadingIndicator` `spinnerHidden` style class is identical to `spinner` (both have the animation) — the "Hidden" name suggests an intent to hide it for reduced-motion users, but there is no `@media (prefers-reduced-motion)` guard in the styles.

---

## Summary of Key UX Gaps

| Gap | Severity | Location |
|---|---|---|
| No completion notification when operator is not on the detail pane | High | Global |
| List/board show no triage-in-progress indicator | Medium | `backlog/page.tsx`, `BacklogItemCard.tsx` |
| Failure state uses a bare `<div>` instead of the `InlineError` component | Medium | `BacklogItemDetail.tsx` line 518 |
| Cancel button is a stub (TODO comment, no RPC) | Medium | `BacklogItemDetail.tsx` line 199 |
| No turn counter or phase labels visible during autonomous run | Medium | `BacklogItemDetail.tsx`, `SessionMonitor.tsx` |
| After "Apply suggestions", Spawn Session / Run Autonomously are disabled with unclear tooltip | Medium | `BacklogItemDetail.tsx` lines 600–626 |
| No guard before re-triggering triage on an item with an existing result | Low | `BacklogItemDetail.tsx` line 576 |
| `spinnerHidden` class name suggests reduced-motion intent that is not implemented | Low | `TriageLoadingIndicator.css.ts` |
