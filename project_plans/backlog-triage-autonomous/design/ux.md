# UX Design: Backlog Triage Autonomous

**Date**: 2026-06-22
**Feature**: Headless autonomous triage — prompt injection, in-progress indicators, failure recovery, post-completion flow
**Surfaces designed**: 5
**UX acceptance criteria**: 28

---

## Surface Map

| # | Surface | Component | Change Type |
|---|---------|-----------|-------------|
| 1 | Detail pane — triage in-progress | `BacklogItemDetail.tsx` | Wire existing `TriageLoadingIndicator` (already present) + disable "Trigger Triage" while running |
| 2 | Detail pane — triage failure | `BacklogItemDetail.tsx` | Replace bare `<div role="alert">` with `<InlineError>` |
| 3 | Detail pane — post-completion actions | `BacklogItemDetail.tsx` | Enable "Spawn Session" / "Run Autonomously" after auto-transition to ready |
| 4 | List view — in-progress row indicator | `BacklogItemCard.tsx` | Add `hasActiveTriage` prop; render compact `TriageLoadingIndicator` |
| 5 | List view — post-completion status badge | `BacklogItemCard.tsx` | Status badge auto-updates from `idea` → `ready` after triage; no new component needed |

---

## Surface 1: Detail Pane — Triage In-Progress

### Current state

The `TriageLoadingIndicator` is already rendered in the detail pane when `triageStatus === "running"`. However, the "Trigger Triage" button remains enabled while triage is running, allowing the operator to spawn a second triage session on top of an in-flight one.

### What changes

The "Trigger Triage" button must be disabled (with a descriptive tooltip) whenever `triageStatus === "running"`.

### ASCII Wireframe

```
┌─────────────────────────────────────────────────────────────────┐
│ [Edit]                                              [Close ×]   │
│                                                                  │
│  Dark mode toggle                                                │
│  [Idea ●]  [P2]   Created Jun 22, 2026 · Updated just now       │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ╔═══════════════════════════════════════════════════════════╗   │
│  ║  ◌  Thinking about acceptance criteria...    42s  [Stop]  ║   │
│  ╚═══════════════════════════════════════════════════════════╝   │
│                                                                  │
│  Description                                                     │
│  ─────────────────────────────────────────────────────────────   │
│  Add a dark/light mode toggle to the settings panel.             │
│                                                                  │
│  Acceptance Criteria (0/0)                                       │
│  ─────────────────────────────────────────────────────────────   │
│  (none yet)                                                      │
│                                                                  │
│  Actions                                                         │
│  ─────────────────────────────────────────────────────────────   │
│  [Mark Ready ─ disabled: no AC]                                  │
│  [Trigger Triage ─ disabled: triage in progress]  ←── CHANGE    │
│                                                                  │
│  Sessions (1)                                                    │
│  ─────────────────────────────────────────────────────────────   │
│  sess-abc123   triage   Jun 22, 2026 12:00                       │
│  ┌─────────────────────────────────────────────────────────┐     │
│  │ SessionMonitor (live terminal, 60 lines, 5s poll)        │     │
│  └─────────────────────────────────────────────────────────┘     │
└─────────────────────────────────────────────────────────────────┘
```

### Interaction Flow

| Step | User action | System response |
|------|-------------|-----------------|
| 1 | Operator clicks "Trigger Triage" | Button enters loading state; `triggerTriage(id)` called; `triageStatus` transitions to `"running"` |
| 2 | Backend starts triage session | Detail pane's 5-second poll detects `triageStatus === "running"`; `TriageLoadingIndicator` renders |
| 3 | Operator reads label | Label: "Thinking about acceptance criteria..." (< 60s), "Still thinking — up to 3 min" (≥ 60s) |
| 4 | Timer ticks | Elapsed counter increments every 1s |
| 5 | Operator tries to trigger again | "Trigger Triage" is `disabled`; `title` reads "Triage is already running"; click has no effect |
| 6 | Operator clicks "Stop" | Button calls `onCancel`; currently reloads item (cancel RPC is a future enhancement) |
| 7 | Triage completes (either result or failure) | 5-second poll detects status change; indicator disappears; either Surface 3 or Surface 2 renders |

### Error / Edge Cases

| Scenario | What the operator sees |
|----------|------------------------|
| `triggerTriage()` RPC fails | Existing inline error banner at top of scroll area: "Action failed." with dismiss button |
| Triage running for > 3 min and indicator hides itself | Backend poll will eventually update `triageStatus` to `"failed"`; Surface 2 renders automatically |
| Operator closes and reopens detail pane mid-triage | Pane re-fetches item; `triageStatus === "running"` → indicator re-renders; timer resets to 0 (minor: elapsed not restored, intentional) |

---

## Surface 2: Detail Pane — Triage Failure

### Current state

A bare `<div role="alert">` renders: "Triage encountered an error. Trigger triage manually to retry." No type distinction, no logs link, no inline retry.

### What changes

Replace the bare div with `<InlineError>`. Map `triageFailureReason` (a new field on `BacklogItem`) to `InlineError` type:

| `triageFailureReason` | `InlineError type` |
|-----------------------|--------------------|
| `"timeout"` | `"timeout"` |
| `"exit_code_1"` / `"session_error"` | `"permanent"` |
| `"network"` / undefined | `"transient"` |

Pass `logsSessionId` (the ID of the failed triage session) to enable the "View session logs" link.

### ASCII Wireframe — transient failure (pill)

```
┌─────────────────────────────────────────────────────────────────┐
│  Triage in-progress section — now gone                          │
│                                                                  │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │ ✕  Triage failed — Network error. The request could not │    │
│  │    be completed.                    [Retry ↺]  [×]      │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                  │
│  Actions                                                         │
│  [Mark Ready ─ disabled]  [Trigger Triage]                       │
└─────────────────────────────────────────────────────────────────┘
```

### ASCII Wireframe — permanent failure (block)

```
┌─────────────────────────────────────────────────────────────────┐
│  ┌─────────────────────────────────────────────────────────┐    │
│  │ ✕  Triage failed                               [×]      │    │
│  │    The triage session exited unexpectedly (exit code 1). │    │
│  │    Check the session logs for details.                   │    │
│  │                                                          │    │
│  │    [View session logs ↗]   [Retry ↺]                    │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                  │
│  Actions                                                         │
│  [Mark Ready ─ disabled]  [Trigger Triage]                       │
└─────────────────────────────────────────────────────────────────┘
```

### ASCII Wireframe — timeout failure (pill)

```
┌─────────────────────────────────────────────────────────────────┐
│  ┌─────────────────────────────────────────────────────────┐    │
│  │ ✕  Triage timed out — The triage session did not        │    │
│  │    complete within 3 minutes.       [Retry ↺]  [×]     │    │
│  └─────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────┘
```

### Interaction Flow

| Step | User action | System response |
|------|-------------|-----------------|
| 1 | Triage backend session ends with failure | 5-second poll sets `triageStatus === "failed"` on the item |
| 2 | Detail pane re-renders | `InlineError` renders with the appropriate `type` and the failed session ID |
| 3 | Operator reads error | Headline + body clearly describe what failed and why |
| 4 | Operator clicks "View session logs" | Opens `/sessions/{logsSessionId}/logs` in a new tab |
| 5 | Operator clicks "Retry ↺" | Calls `triggerTriage(item.id)`; error banner dismisses; `TriageLoadingIndicator` re-appears (Surface 1) |
| 6 | Operator clicks "×" (dismiss) | `InlineError` disappears; item remains in `"idea"` status; "Trigger Triage" remains available in Actions |

### Error / Edge Cases

| Scenario | What the operator sees |
|----------|------------------------|
| Retry RPC also fails | `InlineError` re-renders with `type: "transient"` |
| `logsSessionId` is undefined (session was never created) | "View session logs" link is absent; only "Retry ↺" shown |
| Operator dismisses and navigates away, then returns | Item still has `triageStatus === "failed"`; `InlineError` re-renders on re-open (dismiss is session-local, not persisted) |

---

## Surface 3: Detail Pane — Post-Completion (idea → ready transition)

### Current state

After "Apply suggestions" in `TriageReviewPanel`, the item moves to `"ready"`. However, "Spawn Session" and "Run Autonomously" are disabled because `canSpawnSession` requires `skipPlanning || planApproved`, and `planApproved` is only set via "Approve Plan" which is gated behind `planArtifactsPath`. The autonomous triage completes without writing `planArtifactsPath`, leaving the operator with disabled buttons and an unhelpful tooltip.

### What changes

The autonomous triage must set `planArtifactsPath` on the `BacklogItem` when it calls `submit_triage_result`. Once `planArtifactsPath` is set, the "Approve Plan" button becomes visible, enabling the operator to unlock "Spawn Session" / "Run Autonomously" in one click.

### ASCII Wireframe — item in "ready" with planArtifactsPath set

```
┌─────────────────────────────────────────────────────────────────┐
│  Dark mode toggle                                                │
│  [Ready ●]  [P2]                                                 │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ╔═══════════════════════════════════════════════════════════╗   │
│  ║  Triage applied — item is now ready.        [Undo]  7s   ║   │  ← existing undo toast (no change)
│  ╚═══════════════════════════════════════════════════════════╝   │
│                                                                  │
│  Actions                                                         │
│  ─────────────────────────────────────────────────────────────   │
│  [Trigger Triage]                                                │
│  [Spawn Session ─ disabled]   ←── still disabled until approved  │
│  [Run Autonomously ─ disabled]                                   │
│  [Approve Plan]               ←── NOW VISIBLE (planArtifactsPath set)
│                                                                  │
│  Plan Artifacts                                                  │
│  ─────────────────────────────────────────────────────────────   │
│  docs/tasks/dark-mode-toggle/                ←── NEW: path shown │
└─────────────────────────────────────────────────────────────────┘
```

### ASCII Wireframe — after clicking "Approve Plan"

```
┌─────────────────────────────────────────────────────────────────┐
│  Actions                                                         │
│  [Trigger Triage]                                                │
│  [Spawn Session]          ←── NOW ENABLED                        │
│  [Run Autonomously]       ←── NOW ENABLED                        │
│                                                                  │
│  Plan Artifacts                                                  │
│  docs/tasks/dark-mode-toggle/                                    │
└─────────────────────────────────────────────────────────────────┘
```

### Interaction Flow

| Step | User action | System response |
|------|-------------|-----------------|
| 1 | Triage completes autonomously | Backend sets `triageStatus = "completed"`, stores `triageResult`, writes plan files, sets `planArtifactsPath` on item |
| 2 | Item auto-transitions to `"ready"` | Status badge changes from "Idea" to "Ready"; TriageReviewPanel with "Apply suggestions" no longer needed (item already moved) |
| 3 | Operator opens detail pane | Sees "Ready" status badge, "Plan Artifacts" section with path, "Approve Plan" button in Actions |
| 4 | Operator clicks "Approve Plan" | `approvePlan(item.id)` called; `planApproved` flag set on item |
| 5 | Detail pane reloads | "Spawn Session" and "Run Autonomously" are now enabled |
| 6 | Operator clicks "Spawn Session" | Session created; item transitions to `"in_progress"` |

### Error / Edge Cases

| Scenario | What the operator sees |
|----------|------------------------|
| `planArtifactsPath` not set (older triage path) | "Approve Plan" not shown; tooltip on "Spawn Session" reads "Approve the plan or enable skip_planning to spawn a session" (unchanged) |
| "Approve Plan" RPC fails | Existing inline error banner: "Action failed." |
| Item has `skipPlanning = true` | "Spawn Session" and "Run Autonomously" already enabled; "Approve Plan" not needed |

---

## Surface 4: List View — In-Progress Row Indicator

### Current state

`BacklogItemCard.tsx` has no triage indicator. An item with `triageStatus === "running"` looks identical to one with `triageStatus === "idle"`. The compact `TriageLoadingIndicator` exists but is not wired anywhere.

### What changes

Add `hasActiveTriage?: boolean` prop to `BacklogItemCard`. When `true` and `item.status === "idea"`, replace the "Mark Ready" action button with a compact triage indicator pill.

The list/board parent must pass `hasActiveTriage={item.triageStatus === "running"}`.

### ASCII Wireframe — card without triage (baseline)

```
┌──────────────────────────────────────┐
│  Dark mode toggle            [P2]    │
│  ──────────────────────────────────  │
│  0/0 done            [Mark Ready ─] │
└──────────────────────────────────────┘
```

### ASCII Wireframe — card with active triage

```
┌──────────────────────────────────────┐
│  Dark mode toggle            [P2]    │
│  ──────────────────────────────────  │
│  0/0 done   ◌ Thinking...  12s  [×] │
└──────────────────────────────────────┘
```

Notes:
- The "Mark Ready" button is replaced by the compact pill; no button/pill overlap
- The `[×]` is the compact cancel button (currently a stub; still renders for visual consistency)
- `context="list"` is passed to `TriageLoadingIndicator` so it uses list-appropriate labels

### Interaction Flow

| Step | User action | System response |
|------|-------------|-----------------|
| 1 | Operator views backlog list | Cards with `triageStatus === "running"` show compact pill instead of action button |
| 2 | Triage completes (5-second list poll or server-sent event) | Card re-renders; pill disappears; status badge changes from "Idea" to "Ready" (or failure is silent in list — operator must open detail) |
| 3 | Operator clicks `[×]` on pill | TODO stub behavior — reloads the card's item data; no actual cancel (same as detail pane) |
| 4 | Operator clicks the card body | Detail pane opens; Surface 1 or Surface 2 renders depending on current state |

### Error / Edge Cases

| Scenario | What the operator sees |
|----------|------------------------|
| Triage fails while list is open | Compact pill disappears (status changed to `"failed"`); no failure indicator in the card; operator must open detail to see `InlineError` |
| Multiple items triaging simultaneously | Each card independently shows its own compact pill |
| `elapsedSeconds >= 180` on compact pill | `TriageLoadingIndicator` renders `null`; card shows nothing in footer (acceptable — operator should open detail) |

---

## Surface 5: List View — Post-Completion Status Badge

### Current state

When `apply_suggestions` moves an item from `idea` → `ready`, the list must reflect the new status badge without a manual page refresh.

### What changes

No new component. The list page already polls or re-fetches when items change. The status badge in `BacklogItemCard` reads from `item.status`. When the item transitions to `"ready"`, the badge updates automatically on the next data refresh cycle.

### ASCII Wireframe — before

```
┌──────────────────────────────────────┐
│  Dark mode toggle            [P2]    │
│  ──────────────────────────────────  │
│  3/3 done        [Trigger Triage]   │
└──────────────────────────────────────┘
  Status: Idea
```

### ASCII Wireframe — after autonomous triage completes

```
┌──────────────────────────────────────┐
│  Dark mode toggle            [P2]    │
│  ──────────────────────────────────  │
│  3/3 done        [Trigger Triage]   │
└──────────────────────────────────────┘
  Status: Ready   ←── badge updates on next poll
```

No new interaction flow for this surface — it is a passive data-driven update.

---

## Complete Interaction Flow Diagram

```
Operator                 UI                       Backend
───────                  ──                       ───────
Click "Trigger Triage"
                    ──► triggerTriage(id) RPC ──► spawn triage session
                                                  inject triage prompt
                    ◄── triageStatus="running" ──
                    (poll/5s)
                    show TriageLoadingIndicator
                    disable "Trigger Triage" btn
                    show compact pill in list

                         ...autonomous LLM runs...

                    ◄── triageStatus="failed"  ──  (path A: failure)
                    show InlineError
                    (type based on failureReason)
Click "Retry ↺"
                    ──► triggerTriage(id)      ──► spawn new triage session
                    (returns to in-progress state)

                    ◄── triageStatus="completed" ─ (path B: success)
                         item.status = "ready"
                         planArtifactsPath set
                    show TriageReviewPanel
                    (if item still "idea") OR
                    show "Plan Artifacts" section
                    enable "Approve Plan" btn
                    list card: pill → Ready badge

Click "Approve Plan"
                    ──► approvePlan(id)        ──► planApproved = true
                    enable "Spawn Session"
                    enable "Run Autonomously"

Click "Spawn Session"
                    ──► spawnSessionFromItem   ──► item → in_progress
```

---

## UX Acceptance Criteria

### AC-1 — Trigger Triage button disabled while triage is running

> User cannot trigger a second triage while one is in progress.

- **Test**: While `triageStatus === "running"`, the "Trigger Triage" button in the detail pane must have `disabled` attribute set.
- **Test**: The button's `title` attribute must read "Triage is already running" (or equivalent) while disabled.
- **Measurable**: Verified by inspecting the DOM; button cannot be clicked.

### AC-2 — In-progress indicator appears within one poll cycle

> User sees the triage indicator without a manual refresh.

- **Test**: After clicking "Trigger Triage", within 6 seconds (one 5-second poll + render), `TriageLoadingIndicator` is visible in the detail pane.
- **Test**: The indicator shows a spinner, a label, and an elapsed counter.

### AC-3 — In-progress indicator label changes at 60 seconds

> User gets updated messaging when triage takes longer than expected.

- **Test**: At `elapsedSeconds < 60`, label reads "Thinking about acceptance criteria..."
- **Test**: At `elapsedSeconds >= 60`, label reads "Still thinking — up to 3 min"

### AC-4 — Compact pill appears in list view during triage

> User can see which backlog items are being triaged without opening the detail pane.

- **Test**: A card with `item.triageStatus === "running"` and `item.status === "idea"` shows the compact `TriageLoadingIndicator` in the card footer.
- **Test**: The compact pill shows a spinner, a label, an elapsed counter, and a `[×]` cancel button.
- **Test**: The "Mark Ready" action button is NOT visible on the same card when the compact pill is shown.
- **Test**: A card with `item.triageStatus !== "running"` shows the normal action button (no pill).

### AC-5 — No dead end after triage failure

> User has a clear path to retry or inspect logs from the failure state.

- **Test**: When `triageStatus === "failed"`, the detail pane renders `<InlineError>` (not a bare `<div>`).
- **Test**: The `InlineError` component is present with `role="alert"`.
- **Test**: A "Retry ↺" button is present and clickable within the error component.
- **Test**: Clicking "Retry ↺" calls `triggerTriage(item.id)` and returns the item to in-progress state.

### AC-6 — Failure error type matches failure reason

> User sees a specific error message, not a generic one.

- **Test**: `triageFailureReason === "timeout"` → `InlineError` headline reads "Triage timed out", body reads "The triage session did not complete within 3 minutes."
- **Test**: `triageFailureReason === "exit_code_1"` → `InlineError` headline reads "Triage failed", body references "exit code 1" and instructs to check logs.
- **Test**: `triageFailureReason === "network"` or undefined → `InlineError` headline reads "Triage failed", body references "Network error."

### AC-7 — View session logs link present on permanent failure

> User can inspect raw session output when triage crashes.

- **Test**: When `InlineError type === "permanent"` and `logsSessionId` is defined, a "View session logs" link is rendered.
- **Test**: The link `href` is `/sessions/{logsSessionId}/logs`.
- **Test**: The link has `target="_blank"` and `rel="noopener noreferrer"`.
- **Test**: When `logsSessionId` is undefined, no logs link is rendered (no broken link in DOM).

### AC-8 — "Plan Artifacts" section visible after successful triage

> User can see where plan files were written without navigating elsewhere.

- **Test**: When `item.planArtifactsPath` is non-empty, the "Plan Artifacts" section is visible in the detail pane.
- **Test**: The path is rendered in a `<code>` element showing the full path string.

### AC-9 — "Approve Plan" button appears when planArtifactsPath is set

> User has a single clear action to unlock spawning a session.

- **Test**: When `item.status === "ready"` and `item.planArtifactsPath` is non-empty, "Approve Plan" button is visible in the Actions section.
- **Test**: "Approve Plan" is NOT rendered when `planArtifactsPath` is empty or absent.

### AC-10 — "Spawn Session" and "Run Autonomously" enabled after approving plan

> User can complete the triage → implementation flow in ≤ 2 clicks after triage completes.

- **Test**: After clicking "Approve Plan", "Spawn Session" and "Run Autonomously" buttons are enabled (no `disabled` attribute).
- **Test**: Flow is completable in ≤ 2 clicks from a ready item with `planArtifactsPath` set: 1 click "Approve Plan", 1 click "Spawn Session".

### AC-11 — No dead ends in failure state

> Every failure state has at least one exit path.

- **Test (transient failure)**: "Retry ↺" button + optional "×" dismiss button present.
- **Test (timeout failure)**: "Retry ↺" button present.
- **Test (permanent failure)**: "Retry ↺" button + "View session logs" link present.
- **Test**: After dismissing `InlineError`, "Trigger Triage" button remains available in Actions section.

### AC-12 — Item status badge updates automatically after triage completes

> User sees "Ready" badge without manually refreshing the page.

- **Test**: When `item.status` transitions to `"ready"` (detected via poll), the status badge in the detail pane header updates without a full page navigation.
- **Test**: When `item.status` transitions to `"ready"`, the status badge in the list view card updates on the next data refresh cycle (≤ 10 seconds).

### AC-13 — Triage indicator disappears after triage completes or fails

> The spinner does not persist after triage ends.

- **Test**: When `triageStatus` changes from `"running"` to `"completed"` or `"failed"`, `TriageLoadingIndicator` is no longer rendered in the detail pane.
- **Test**: When `triageStatus` changes, the compact pill in the list card is no longer rendered.

### AC-14 — Accessible: in-progress indicator announces to screen readers

> Screen reader users know triage is running.

- **Test**: `TriageLoadingIndicator` has `role="status"` and `aria-live="polite"`.
- **Test**: `aria-label` on the container reads "Triage in progress, N seconds elapsed" (in 30-second increments per existing component design).

### AC-15 — Accessible: error state announces immediately to screen readers

> Screen reader users are immediately notified of triage failure.

- **Test**: `InlineError` renders with `role="alert"` and `aria-live="assertive"`.
- **Test**: Headline and body text are readable by screen readers (no aria-hidden on text content).

### AC-16 — Accessible: "Retry ↺" button has descriptive aria-label

> Screen reader users know the retry button's purpose.

- **Test**: The retry button has `aria-label="Retry triage"` (matches existing `InlineError` implementation).

### AC-17 — Accessible: "Trigger Triage" disabled state communicated to screen readers

> Screen reader users know the button is unavailable and why.

- **Test**: While `triageStatus === "running"`, "Trigger Triage" button has `disabled` attribute (which maps to `aria-disabled` in HTML).
- **Test**: The `title` attribute on the disabled button is non-empty and describes why it is disabled.

### AC-18 — Keyboard navigation: retry is reachable via Tab

> Keyboard-only users can reach the retry action without a mouse.

- **Test**: With keyboard focus, Tab navigation reaches "Retry ↺" within the visible `InlineError` block.
- **Test**: Enter key on the focused "Retry ↺" button triggers the retry action.

### AC-19 — Keyboard navigation: cancel pill reachable in list view

> Keyboard-only users can interact with the compact triage indicator.

- **Test**: The `[×]` cancel button in the compact pill is focusable via Tab.
- **Test**: Enter key on the focused `[×]` button triggers `onCancel`.

### AC-20 — Color contrast: error state meets WCAG AA

> Error text is readable by users with low vision.

- **Test**: Error headline and body text have contrast ratio ≥ 4.5:1 against the block container background.
- **Test**: "Retry ↺" and "View session logs" action button text have contrast ratio ≥ 4.5:1.

### AC-21 — Color contrast: in-progress indicator meets WCAG AA

> Triage indicator text is readable by users with low vision.

- **Test**: Label text and elapsed counter in `TriageLoadingIndicator` have contrast ratio ≥ 4.5:1 against the indicator container background.

### AC-22 — Reduced motion: spinner respects prefers-reduced-motion

> Animated spinner does not cause issues for users with motion sensitivity.

- **Test**: With `@media (prefers-reduced-motion: reduce)` active, the CSS spinner animation is paused or replaced with a static indicator.
- **Note**: `TriageLoadingIndicator.css.ts` must add this guard (currently absent per UX research findings).

### AC-23 — Re-trigger guard: no silent result overwrite

> User is not surprised by triage results being silently replaced.

- **Test**: When `triageStatus === "completed"` and `triageResult` exists, clicking "Trigger Triage" shows a confirmation dialog: "This item already has a triage result. Start fresh?" with "Confirm" and "Cancel" buttons.
- **Test**: Clicking "Cancel" in the dialog does nothing and leaves the existing result intact.
- **Test**: Clicking "Confirm" triggers a new triage session.
- **Note**: This is a future enhancement; the AC is defined here to be testable when implemented.

### AC-24 — Empty state: list cards without triage show no pill

> No visual noise for items not currently being triaged.

- **Test**: Cards with `item.triageStatus !== "running"` do not render `TriageLoadingIndicator` in any form.
- **Test**: The "Mark Ready" or other action button renders normally on such cards.

### AC-25 — Triage indicator does not block card click

> Operator can still open the detail pane from a card with an active triage pill.

- **Test**: Clicking anywhere on the card body (not on the pill's `[×]` button) while the compact pill is visible opens the detail pane for that item.

### AC-26 — "Stop" button present but clearly limited in scope

> User understands the stop button reloads state; no false confidence in cancellation.

- **Test**: The "Stop" button renders and is clickable; clicking it reloads the item data.
- **Future test (when cancel RPC is implemented)**: Clicking "Stop" calls the cancel triage RPC and the session terminates.

### AC-27 — Failure state is visible immediately when backend transitions to failed

> No gap window where the user sees neither the indicator nor the error.

- **Test**: When `triageStatus` transitions from `"running"` to `"failed"`, within one poll cycle (≤ 6 seconds), the `InlineError` component is visible.
- **Test**: There is no state where both `TriageLoadingIndicator` and `InlineError` are simultaneously visible.

### AC-28 — Post-completion: item transitions to ready without operator action

> When triage completes autonomously, the item does not require "Apply suggestions" — the autonomous flow handles the transition.

- **Test**: After `submit_triage_result` is called by the autonomous session, `item.status` transitions to `"ready"` without any operator interaction.
- **Test**: The `TriageReviewPanel` ("Apply suggestions") does NOT appear after autonomous triage — the panel is only shown when `triageStatus === "completed"` AND `item.status === "idea"` (i.e., the manual-apply path has not yet run). Autonomous triage sets `item.status === "ready"` before the UI polls, so the panel never renders.

---

## Design Decisions

### Why InlineError type is driven by triageFailureReason

The `BacklogItem` proto/model already has `triageStatus`. A new `triageFailureReason` field (string enum: `"timeout"`, `"exit_code_1"`, `"network"`) must be set by the backend when `triageStatus` becomes `"failed"`. Without this, the frontend cannot distinguish a timeout from a crash, and the error message defaults to `"transient"`, which is misleading for permanent failures. This is a **required backend change** for AC-6 and AC-7.

### Why the compact pill replaces the action button

The card footer has limited horizontal space. Showing both the action button and the compact pill would require a layout change or overflow. Replacing the button while triage is running is safe: "Mark Ready" is irrelevant while triage is running (triage will handle the transition), and "Trigger Triage" is correctly suppressed to prevent duplicate sessions.

### Why the undo toast is not shown for autonomous triage

The autonomous triage sets `item.status = "ready"` directly on the backend, bypassing the `handleApplyTriageSuggestions` function that triggers the undo toast. This is intentional: there is no meaningful "undo" for a completed autonomous run (the item was not modified by the operator). The operator can still manually revert via `transitionStatus(id, "idea")` if needed (a future enhancement).

### Why "Approve Plan" remains a manual step

The plan artifacts need human review before spawning an autonomous work session. Auto-approving the plan would remove the only human gate between triage completion and an autonomous agent making code changes. This is a deliberate UX boundary.
