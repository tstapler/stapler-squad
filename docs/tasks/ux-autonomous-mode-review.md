# UX Review: Autonomous Mode — Stapler Squad

**Source**: UX audit of autonomous mode controls, status indicators, and driver lifecycle management.
**Date**: 2026-06-14
**Scope**: `web-app/src/components/sessions/SessionActionsOverflow.tsx`, `web-app/src/components/sessions/SessionCard.tsx`, `server/services/session_service.go`, `session/autonomous_driver.go`

**Summary**: 2 critical, 4 high, 5 medium, 3 low. 9 items are effort S (quick wins).

---

## Critical Issues

### Task C-1: Fix identical icon for enabled/disabled autonomous mode toggle

**File**: `web-app/src/components/sessions/SessionActionsOverflow.tsx` line 491

**Problem**: The toggle renders `"🤖"` in both branches of the ternary `session.autonomousMode ? "🤖" : "🤖"`. Users cannot distinguish the current state from the action being offered. The `onSetRateLimitEnabled` menu item (lines nearby) already uses `"⏸"` vs `"▶"` as a model for this pattern.

**Acceptance Criteria**:
- [ ] When `session.autonomousMode` is `false` (action will enable), the menu item shows `"🤖"` (or equivalent "start" icon)
- [ ] When `session.autonomousMode` is `true` (action will disable), the menu item shows a distinct stop/pause icon such as `"⏹"` — or uses a `"✓ 🤖"` / checkmark prefix to denote the active state
- [ ] The icon change is also reflected in any `aria-label` or accessible name for the menu item
- [ ] Behavior is verified in the browser against the `onSetRateLimitEnabled` item as a reference

**Effort**: S

---

### Task C-2: Add confirmation dialog before enabling autonomous mode

**File**: `web-app/src/components/sessions/SessionActionsOverflow.tsx` lines 486–494

**Problem**: Enabling autonomous mode immediately starts the AutonomousDriver goroutine, injecting LLM-generated prompts into the live session with no warning. Unlike Restart and Delete (both show a confirm dialog), this fires on single click. Users cannot easily undo prompts already sent.

**Acceptance Criteria**:
- [ ] Clicking "Enable autonomous mode" opens a confirmation dialog before the RPC is called
- [ ] Dialog uses the existing `confirmDialog` / `dialogContent` pattern already in the same component
- [ ] Warning text communicates: "Autonomous mode will inject up to 20 AI-generated prompts into this session automatically. The session will run without user input until it completes or gets stuck."
- [ ] Confirmation is required only on enable — disabling autonomous mode does not require a dialog
- [ ] Canceling the dialog leaves the session state unchanged

**Effort**: S

---

## High Severity

### Task H-1: Surface turn progress in the "Auto" badge

**Files**: `web-app/src/components/sessions/SessionCard.tsx` lines 514–524, `session/autonomous_driver.go` lines 146, 178, 194, 206

**Problem**: The driver maintains a turn counter (`turnCount` up to `maxTurns = 20`) but nothing is surfaced in the UI. Users see a static "Auto" badge with no indication of progress, turns remaining, or whether the driver is stuck.

**Short-term acceptance criteria**:
- [ ] Verify that `autonomous_mode` is set to `false` on the server when the driver completes — confirm `onAutonomousDriverComplete` sets `instance.AutonomousMode = false` (not merely calls `stopAndDeregisterDriver`)
- [ ] If the above is not already the case, fix it so the badge disappears automatically on driver exit

**Medium-term acceptance criteria**:
- [ ] Add `autonomous_turn int32` and `autonomous_max_turns int32` fields to the proto `Session` message
- [ ] Populate both fields in the driver on each turn
- [ ] Display "Auto 3/20" in the badge on `SessionCard` when `autonomous_mode` is true and `autonomous_max_turns > 0`

**Effort**: S (short-term), M (medium-term)

---

### Task H-2: Ensure disabling autonomous mode stops the driver promptly

**File**: `server/services/session_service.go` lines 1307–1313

**Problem**: When the user disables autonomous mode, `stopAndDeregisterDriver` is called which cancels the context. If the driver is blocked inside `CallBlockingWithOptions` (an LLM API call taking 30–60s), the badge disappears but the driver may keep running and inject one more prompt.

**Acceptance Criteria**:
- [ ] Verify whether the headless pool client respects context cancellation inside `CallBlockingWithOptions`; document the finding in a code comment
- [ ] If cancellation is not respected: add informational UI text "Autonomous mode may take a moment to stop" (shown while `autonomous_mode` transitions to false)
- [ ] Consider keeping `autonomous_mode: true` on the session until the driver goroutine actually exits, using `driverRunning` state as the badge source of truth — implement if the cancellation race is confirmed
- [ ] No regression: disabling still cancels the context and deregisters the driver

**Effort**: M

---

### Task H-3: Add visual separators and group overflow menu items by function

**File**: `web-app/src/components/sessions/SessionActionsOverflow.tsx` (full menu)

**Problem**: Up to 17 menu items with no visual grouping or separators. The autonomous mode toggle sits near the bottom adjacent to "auto-resume" (a different feature with a similar name), making it hard to find and easy to confuse.

**Acceptance Criteria**:
- [ ] CSS separators (dividers) divide the menu into functional groups, in this order:
  1. Session control: Resume / Pause / Hibernate
  2. Workflow: PR / Checkpoint / Restart
  3. Organization: Rename / Clone / Tags / Workspace
  4. Mode toggles: Auto-resume / Autonomous mode
  5. Destructive: Clear / Delete
- [ ] Autonomous mode toggle is in "Mode toggles" (group 4), not near workspace actions
- [ ] "Auto-resume" and "Autonomous mode" are adjacent within the same group to clarify they are distinct features
- [ ] No existing menu item behavior changes; only presentation and ordering change

**Effort**: S

---

### Task H-4: Surface injected prompts as session notifications

**Files**: `server/services/session_service.go` (driver callback), `session/autonomous_driver.go`

**Problem**: The driver logs every injected turn but users have no way to review what prompts were sent, why the task ended, or what happened when stuck. There is no audit trail visible in the UI.

**Short-term acceptance criteria**:
- [ ] Each injected prompt is emitted as a session notification using `NotificationType_INFO`
- [ ] Notification message format: "Autonomous turn N/20: [truncated prompt text, max 120 chars]"
- [ ] Notifications appear in the existing notification panel without any new UI components

**Long-term acceptance criteria (deferred)**:
- [ ] An "Autonomous history" section in the session detail panel lists `(turn N, timestamp, prompt_text)` from a new backend endpoint

**Effort**: S (notification), L (history panel)

---

## Medium Severity

### Task M-1: Rename autonomous mode labels to user-facing language

**Files**: `web-app/src/components/sessions/SessionActionsOverflow.tsx` lines 492–493, `web-app/src/components/sessions/SessionCard.tsx` line 523

**Problem**: "Enable autonomous mode" and the "Auto" badge are internal vocabulary. Users cannot distinguish this from "auto-resume" (a nearby menu item that does something entirely different).

**Acceptance Criteria**:
- [ ] Menu item label changes from "Enable autonomous mode" → "Run autonomously" (or "Let AI drive to completion")
- [ ] Menu item label changes from "Disable autonomous mode" → "Stop running autonomously"
- [ ] Badge text changes from "Auto" → "Auto-pilot" (or "Driving")
- [ ] A `title` tooltip is added to the menu item: "AI will inject prompts automatically until the task is complete (up to 20 turns)."
- [ ] No functional behavior changes; only label text and tooltip

**Effort**: S

---

### Task M-2: Make the autonomous mode badge interactive

**File**: `web-app/src/components/sessions/SessionCard.tsx` line 514

**Problem**: The "Auto" badge is display-only. Users naturally expect clicking a status badge to toggle or control it, but nothing happens on click. The only path to toggle is through the overflow menu.

**Acceptance Criteria**:
- [ ] The `autonomousBadge` element is an interactive button when `session.autonomousMode` is true
- [ ] Clicking the badge either (a) disables autonomous mode directly (with a confirmation dialog matching C-2 behavior for re-enable) or (b) opens the overflow menu scrolled to the autonomous mode toggle
- [ ] Button has `aria-label="Disable autonomous mode"` (or equivalent per M-1 rename)
- [ ] Keyboard focus and Enter/Space activation work
- [ ] Badge is not interactive when `session.autonomousMode` is false (it does not appear in that state)

**Effort**: S

---

### Task M-3: Show outcome indicator when autonomous mode ends

**Files**: `web-app/src/components/sessions/SessionCard.tsx`, `server/services/session_service.go` lines 3400–3418

**Problem**: When the driver completes, the "Auto" badge disappears with no card-level indication of whether the task completed successfully or got stuck. Users must check the notification panel.

**Acceptance Criteria**:
- [ ] Add an `autonomous_outcome` string field to the proto `Session` message with values `""`, `"done"`, `"stuck"`
- [ ] Populate `autonomous_outcome` in `onAutonomousDriverComplete` based on driver exit reason
- [ ] `SessionCard` renders a transient outcome badge when `autonomous_outcome` is non-empty:
  - `"done"`: green "Done" badge, auto-clears after 30 seconds
  - `"stuck"`: amber "Stuck" badge, persists until dismissed by user click
- [ ] Outcome badge does not appear when `autonomous_outcome` is `""` (default / never ran)

**Effort**: M

---

### Task M-4: Return error when autonomous mode is enabled without headless pool

**Files**: `server/services/session_service.go` lines 674–676, 1153

**Problem**: When the headless LLM is not configured (`headlessPool == nil`), `StartAutonomousDriverForInstance` silently returns early but `autonomous_mode` is still set to `true` in the DB, showing the badge despite nothing happening.

**Acceptance Criteria**:
- [ ] In the `UpdateSession` handler, check `headlessPool == nil` before setting `autonomous_mode: true`
- [ ] If `headlessPool == nil`, return `connect.CodeFailedPrecondition` with message: "Autonomous mode requires a headless LLM to be configured. See settings."
- [ ] The frontend `handleToggleAutonomousMode` callback handles the error response and shows the error message to the user (currently has no `.catch()`)
- [ ] `autonomous_mode` in the DB remains `false` when the precondition fails
- [ ] No change to behavior when `headlessPool` is configured

**Effort**: S (backend) + S (frontend)

---

### Task M-5: Add mid-run steering action for autonomous sessions

**File**: `web-app/src/components/sessions/SessionActionsOverflow.tsx`

**Problem**: The `goal` is set once at driver construction from `instance.Prompt`. Users cannot redirect the agent mid-run without disabling and re-enabling autonomous mode, which resets all context.

**Acceptance Criteria**:
- [ ] A "Give direction" (or "Steer") menu item appears in the overflow menu only when `session.autonomousMode` is true
- [ ] Clicking the item opens a small text input (inline or modal)
- [ ] Submitted text is sent via `SendCommandImmediate` to the session
- [ ] The steering event is logged distinctly (e.g., as a `NotificationType_INFO` notification: "Steering input sent: [text]")
- [ ] Item is absent from the menu when `session.autonomousMode` is false

**Effort**: M

---

## Low Severity

### Task L-1: Update stuck notification body to include next-action guidance

**File**: `server/services/session_service.go` line 3410

**Problem**: Stuck notification reads `"<session name>: max turns reached"` — accurate but gives no next action. Users are left uncertain about what to do next.

**Acceptance Criteria**:
- [ ] Stuck notification body changes to: "Session '[name]' stopped after 20 turns without completing. Open the session to review what was accomplished and give the next instruction."
- [ ] The session name interpolation is preserved
- [ ] No other notification behavior changes

**Effort**: S

---

### Task L-2: Use stable "Autonomous mode" label with checkmark for active state

**File**: `web-app/src/components/sessions/SessionActionsOverflow.tsx` lines 492–493

**Problem**: Label flips between "Enable" and "Disable" based on state (pending M-1 rename, between "Run autonomously" and "Stop running autonomously"). Common app menu convention uses a stable label with a checkmark prefix when active.

**Acceptance Criteria**:
- [ ] Menu item label is stable: "Run autonomously" (per M-1 rename) in both states
- [ ] When `session.autonomousMode` is true, a `"✓"` prefix or `aria-checked="true"` marks the active state
- [ ] Screen reader announces the checked state via `role="menuitemcheckbox"` and `aria-checked`
- [ ] This task depends on M-1 (rename) being completed first

**Effort**: S

---

### Task L-3: Update overflow button aria-label to mention autonomous mode controls

**File**: `web-app/src/components/sessions/SessionActionsOverflow.tsx` line 363

**Problem**: Screen reader users hear "More session actions" with no hint that autonomous mode controls are inside. Long-term, M-2 (on-card affordance) makes this less important, but the aria-label is still a quick accessibility improvement.

**Acceptance Criteria**:
- [ ] When `session.autonomousMode` is true, the overflow button `aria-label` becomes "More session actions (autonomous mode active)"
- [ ] When `session.autonomousMode` is false, the existing label "More session actions" is preserved
- [ ] The label is computed from a derived constant, not a duplicated string literal

**Effort**: S

---

## Quick Wins (All Effort S — completable in one sitting)

Tasks suitable for a single focused session. All are self-contained with no cross-task dependencies except where noted.

| Task | File | One-line description |
|------|------|----------------------|
| C-1 | `SessionActionsOverflow.tsx:491` | Fix identical icon — use `"⏹"` when disabling |
| C-2 | `SessionActionsOverflow.tsx:486–494` | Add confirm dialog before enabling |
| H-1 (short-term) | `session_service.go` / `autonomous_driver.go` | Verify driver sets `autonomous_mode = false` on exit |
| H-3 | `SessionActionsOverflow.tsx` (full menu) | Add separators and group menu items by function |
| H-4 (notification only) | `session_service.go` / `autonomous_driver.go` | Emit each injected prompt as a `NotificationType_INFO` notification |
| M-1 | `SessionActionsOverflow.tsx:492–493`, `SessionCard.tsx:523` | Rename labels to user-facing language |
| M-2 | `SessionCard.tsx:514` | Make "Auto" badge an interactive button |
| M-4 (backend) | `session_service.go:674–676` | Return `FailedPrecondition` when headless pool is nil |
| M-4 (frontend) | `SessionActionsOverflow.tsx` | Add `.catch()` to `handleToggleAutonomousMode` |
| L-1 | `session_service.go:3410` | Rewrite stuck notification to include next-action guidance |
| L-2 | `SessionActionsOverflow.tsx:492–493` | Stable label + checkmark for active state (depends on M-1) |
| L-3 | `SessionActionsOverflow.tsx:363` | Update overflow `aria-label` when autonomous mode is active |
