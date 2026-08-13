# UX Review: General UX & Accessibility Audit — Stapler Squad

**Source**: Comprehensive UX/accessibility audit of session management, omnibar, review queue, and bulk actions.
**Date**: 2026-06-15
**Scope**: `web-app/src/app/page.tsx`, `web-app/src/components/sessions/`, `web-app/src/app/review-queue/`, `web-app/src/components/pane/`

**Summary**: 2 critical, 4 high, 8 medium, 9 low. 1 blocking bug (STOPPED session state mismatch).

---

## Bug

### BUG-001: STOPPED sessions show paused appearance but have no Resume button and incorrect overflow actions

**Severity**: High (two sessions currently affected in production)

**Files**:
- `web-app/src/components/sessions/SessionRow.tsx` line 53 (`getStatusDotValue`)
- `web-app/src/components/sessions/SessionActionsOverflow.tsx` lines 445, 491, 499
- `server/services/session_service.go` (`UpdateSession` handler)

**Problem**: `getStatusDotValue(STOPPED)` returns `"paused"` so STOPPED sessions visually appear paused. But `isPaused = status === SessionStatus.PAUSED` is false for STOPPED, so the primary Resume button (line 445) does not render. The overflow menu simultaneously shows both Resume (line 491, condition `!(isPaused||isReady) && onResume`) and Pause (line 499, condition `!isRunning && !isCreating && onPause`) for STOPPED sessions — contradictory actions. The backend `UpdateSession` silently ignores Resume calls on STOPPED sessions (only handles `instance.Status == session.Paused`).

Two sessions currently affected: `triage:put-backlog-behind-a-feature-flag-by-default` and `personal-wiki-grass`.

**Acceptance Criteria**:
- [ ] `getStatusDotValue(STOPPED)` returns a distinct value (e.g. `"stopped"`) with its own visual treatment, not `"paused"`
- [ ] Primary action area at line 445 extends condition to `isPaused || isStopped` so STOPPED sessions show Resume (or a Restart button if backend cannot resume from STOPPED)
- [ ] Overflow Pause condition at line 499 changes from `!isRunning` to `isPaused || isReady` so Pause only shows in states where it makes sense
- [ ] Backend `UpdateSession` either handles STOPPED → RUNNING transition, or STOPPED sessions surface Restart as the primary CTA instead of Resume
- [ ] No STOPPED session shows both Resume and Pause simultaneously in any UI surface

---

## Critical Issues

### UX-001: Delete confirmation dialog lacks portal, focus trap, and focus return

**Severity**: Critical
**Category**: Accessibility / ARIA

**File**: `web-app/src/app/page.tsx` lines 457–491

**Problem**: The `deleteConfirmTarget` dialog has `role="dialog"` and `aria-modal="true"` but no `useFocusTrap`, no `autoFocus`, no focus return on close, and is not rendered via `createPortal`. This means keyboard users cannot operate the dialog safely, and screen readers will not correctly scope focus. `SessionActionsOverflow.tsx` already implements the correct pattern with `useFocusTrap` and `createPortal` — this dialog should match it.

**Acceptance Criteria**:
- [ ] Dialog is wrapped in `createPortal(…, document.body)` so `position: fixed` stacking works regardless of ancestor transforms
- [ ] `useFocusTrap(dialogRef, !!deleteConfirmTarget)` is called to trap focus inside the dialog while open
- [ ] The Cancel button (or first focusable element) receives `autoFocus` when the dialog opens
- [ ] On close (confirm or cancel), focus returns to the element that triggered the dialog
- [ ] Pattern matches `SessionActionsOverflow.tsx` implementation as the reference

---

### UX-002: Bulk `aria-live` region for feedback mounted conditionally (WCAG ARIA22)

**Severity**: Critical
**Category**: Accessibility / ARIA

**File**: `web-app/src/components/sessions/BulkActions.tsx` line 42

**Problem**: The `aria-live` region that announces bulk operation feedback is only mounted when `feedback` is non-null. WCAG ARIA22 requires live regions to be present in the DOM before content is injected — screen readers register live regions on mount, not at announcement time. Content injected into a newly-mounted live region is silently dropped.

**Acceptance Criteria**:
- [ ] The live region is rendered unconditionally (always in the DOM), with empty content when there is no feedback: `<div role="status" aria-live="polite" aria-atomic="true">{feedback ?? ""}</div>`
- [ ] Feedback text continues to appear visually when non-null (no regression)
- [ ] Verified with a screen reader or NVDA/VoiceOver that bulk operation results are announced

---

## High Severity

### UX-003: "Clear Conversation" not styled as destructive

**Severity**: High
**Category**: UX / Visual Design

**File**: `web-app/src/components/sessions/SessionActionsOverflow.tsx` lines 654–661

**Problem**: "Clear Conversation" uses the neutral `overflowMenuItem` class while Restart and Delete use `overflowMenuItemDanger`. Clearing conversation state is irreversible and should be communicated as such. Users may trigger it accidentally expecting a reversible action.

**Acceptance Criteria**:
- [ ] "Clear Conversation" uses `overflowMenuItemDanger` styling
- [ ] A confirmation dialog is shown before executing the clear, matching the Restart/Delete confirm dialog pattern already in the component
- [ ] Dialog copy makes the irreversibility clear (e.g., "This will permanently clear the conversation history and cannot be undone.")

---

### UX-004: Approval countdown urgency communicated by color alone (WCAG 1.4.1)

**Severity**: High
**Category**: Accessibility / Color

**File**: `web-app/src/components/sessions/ApprovalCard.tsx` lines 86–99

**Problem**: `countdownNormal`, `countdownWarning`, and `countdownUrgent` CSS classes are the sole visual indicator of approval time urgency. WCAG 1.4.1 prohibits color as the only means of conveying information. Users with color vision deficiency cannot distinguish an urgent countdown from a normal one.

**Acceptance Criteria**:
- [ ] When the countdown is under 30 seconds (the `countdownWarning` or `countdownUrgent` threshold), a secondary indicator is added — a warning triangle icon (`⚠`) or bold weight is acceptable
- [ ] The secondary indicator is visible regardless of color perception
- [ ] The icon/weight change does not disrupt the countdown digit layout

---

### UX-005: Pane picker overlay has no announcement or focus management

**Severity**: High
**Category**: Accessibility / ARIA

**File**: `web-app/src/components/pane/PaneTilingContainer.tsx` lines 224–285

**Problem**: The `pickerActionBar` appears without an `aria-live` announcement, no focus is moved to it, and it has no `role="dialog"` or `role="toolbar"`. Keyboard and screen reader users have no indication the action bar appeared and cannot reach it without sighted mouse use.

**Acceptance Criteria**:
- [ ] An `aria-live="assertive"` region announces the picker appearance (e.g., "Select a pane to move the session into")
- [ ] A `useEffect` on `pickerPendingSession` sets focus to the first button in the action bar when it becomes visible
- [ ] The action bar has an appropriate ARIA role (`role="toolbar"` or `role="group"` with `aria-label`)
- [ ] Focus returns to the triggering element when the picker is dismissed

---

### UX-006: Session creation mode labels use git internals jargon

**Severity**: High
**Category**: UX / Language / Vocabulary

**File**: `web-app/src/components/sessions/OmnibarCreationPanel.tsx` lines 26–33

**Problem**: The SESSION_TYPES array uses "New Worktree", "Directory", "Use Worktree", and "One-off" as labels. "Worktree" is a git-internal term invisible to most users. Users cannot decode what each option does without git expertise, making session creation unpredictable.

**Acceptance Criteria**:
- [ ] Labels are renamed to user-goal language:
  - "New Worktree" → "New branch (isolated)"
  - "Directory" → "Existing folder"
  - "Use Worktree" → "Existing branch"
  - "One-off" → "Temporary (no git)"
- [ ] Each radio option has a brief hint/description line below the label explaining what happens (e.g., "Creates an isolated git branch in a new worktree")
- [ ] Underlying `sessionType` string values and proto enum values are unchanged — only display labels change

---

## Medium Severity

### UX-007: Restart styled same as Delete despite being reversible

**Severity**: Medium
**Category**: UX / Visual Design

**File**: `web-app/src/components/sessions/SessionActionsOverflow.tsx` lines 547–556

**Problem**: Restart and Delete both use `overflowMenuItemDanger`. Restart is a reversible operation; Delete is permanent. Treating them identically creates unnecessary alarm around Restart and dilutes the urgency signal for Delete.

**Acceptance Criteria**:
- [ ] Restart uses a warning/neutral style (e.g., `overflowMenuItemWarning` or plain `overflowMenuItem`) rather than `overflowMenuItemDanger`
- [ ] `overflowMenuItemDanger` is reserved for permanent-destruction actions only (Delete, Clear Conversation)
- [ ] If a new `overflowMenuItemWarning` class is added, it is defined in the corresponding `.css.ts` file using vanilla-extract per CSS architecture rules

---

### UX-008: Bulk "Stop" and "Pause" actions map to the same handler

**Severity**: Medium
**Category**: UX / Correctness

**File**: `web-app/src/components/sessions/SessionList.tsx` lines 505–539

**Problem**: `handleStopSelected` and `handlePauseSelected` both call `onPauseSession`. Two differently-named bulk actions with identical side effects is either a bug or dead UI that needs removal — users cannot predict which one actually runs.

**Acceptance Criteria**:
- [ ] Either: unify into a single "Pause/Stop" bulk action with a single handler, or
- [ ] Wire `handleStopSelected` to a distinct RPC that genuinely stops (terminates) the session rather than pausing it
- [ ] Whichever approach is chosen, the button label and handler behavior match

---

### UX-009: Session count hides total when filters are active

**Severity**: Medium
**Category**: UX / Feedback

**File**: `web-app/src/components/sessions/SessionList.tsx` line 587

**Problem**: The header shows "Sessions (3)" with no indication filters are reducing the visible set. Users looking for a session they know exists may believe it was deleted.

**Acceptance Criteria**:
- [ ] When any filter is active, the count renders as "3 of 30 sessions" or "Sessions (3 filtered from 30)"
- [ ] When no filter is active, the count renders as "Sessions (30)" (current behavior unchanged)
- [ ] Total unfiltered count is available without an additional RPC (use the full in-memory list length before filter application)

---

### UX-010: Keyboard shortcut discoverability requires knowing about `?` key

**Severity**: Medium
**Category**: UX / Discoverability

**File**: `web-app/src/components/sessions/SessionDetailBar.tsx` lines 42–52

**Problem**: Only t/p/r shortcuts are shown inline. 12+ cockpit shortcuts exist but are accessible only via the `?` key, which is itself not surfaced anywhere in the main cockpit view. Review queue page (lines 376–383) already has a floating `?` button — main cockpit does not.

**Acceptance Criteria**:
- [ ] A floating `?` button is added to the main cockpit (`CockpitShell` or equivalent) matching the review queue page implementation
- [ ] The button opens the existing shortcuts modal
- [ ] Button placement does not overlap primary session content on mobile (bottom-right corner, same as review queue)

---

### UX-011: Radio group in OmnibarCreationPanel has edge-case tabIndex gap

**Severity**: Medium
**Category**: Accessibility / Keyboard

**File**: `web-app/src/components/sessions/OmnibarCreationPanel.tsx` lines 51–87

**Problem**: If `formState.sessionType` does not match any `SESSION_TYPES` value, no radio button gets `tabIndex={0}`, making the entire radio group unreachable by keyboard. This can occur when a new session type is added to the backend without updating the frontend `SESSION_TYPES` array.

**Acceptance Criteria**:
- [ ] The `tabIndex` logic defaults `tabIndex={0}` to the first item when no `SESSION_TYPES` value matches `formState.sessionType`:
  ```ts
  tabIndex={value === type.value ? 0 : currentIndex === -1 && idx === 0 ? 0 : -1}
  ```
- [ ] A unit test verifies that the radio group is keyboard-reachable when `formState.sessionType` is an unrecognized string

---

### UX-012: Bulk delete modal `aria-labelledby` association unverified

**Severity**: Medium
**Category**: Accessibility / ARIA

**File**: `web-app/src/components/sessions/SessionList.tsx` lines 1118–1139

**Problem**: The bulk delete modal uses `<Modal>` + `<ModalContent fallbackTitle="Confirm delete">`. It is unclear whether `Modal` injects `aria-labelledby` pointing to the `ModalTitle` element ID. If the association is absent, screen readers announce the dialog without a name.

**Acceptance Criteria**:
- [ ] Verify (via DOM inspection or unit test) that the rendered `<div role="dialog">` has `aria-labelledby` pointing to the ID of the modal title element
- [ ] If the association is absent, add `aria-labelledby` explicitly on the dialog container
- [ ] Add a regression test or snapshot assertion to prevent the association from being lost in future Modal refactors

---

### UX-013: Restart in wrong menu group with destructive styling

**Severity**: Medium
**Category**: UX / Information Architecture

**File**: `web-app/src/components/sessions/SessionActionsOverflow.tsx` lines 547–557

**Problem**: Restart appears in Group 2 (workflow actions: Checkpoint, Restart, org separator) with `overflowMenuItemDanger` styling. Destructive/irreversible actions belong at the bottom of the menu (Group 5 alongside Delete). Mixing reversible workflow actions and destructive actions in the same group violates standard menu grouping conventions.

**Acceptance Criteria**:
- [ ] Restart is moved to Group 5 (alongside Delete and Clear Conversation)
- [ ] The `hasGroup2` computed flag is updated to exclude Restart
- [ ] Group 2 contains only non-destructive workflow actions (Checkpoint and similar)
- [ ] After the move, Restart visually uses warning/neutral styling per UX-007

---

### UX-014: One-shot PR URL not surfaced after success

**Severity**: Medium
**Category**: UX / Feedback

**File**: `web-app/src/components/sessions/SessionActionsOverflow.tsx` lines 176–188, 527–534

**Problem**: When `handleRunOneShot` succeeds, it shows "PR Created" but does not capture or display the PR URL. The ReviewQueuePanel already shows the URL on success. Users must navigate separately to find the PR link.

**Acceptance Criteria**:
- [ ] `handleRunOneShot` captures the PR URL from the RPC response
- [ ] Success display renders "PR Created — View PR" as a clickable link (or opens in a new tab), or shows the URL in a toast notification
- [ ] Implementation matches the ReviewQueuePanel pattern for consistency

---

### UX-015: Omnibar closes before session appears in list

**Severity**: Medium
**Category**: UX / Feedback

**File**: `web-app/src/components/sessions/Omnibar.tsx` / `web-app/src/lib/contexts/OmnibarContext.tsx`

**Problem**: The omnibar closes on submission before the new session appears in the streaming session list. Users are left without any feedback that creation is in progress — they cannot tell if the action succeeded or was dropped.

**Acceptance Criteria**:
- [ ] One of the following approaches is implemented:
  - Show a transient optimistic "Creating session…" entry in the session list immediately on submission, replaced by the real entry when the stream delivers it, or
  - Keep the omnibar open with a "Creating…" spinner state until the session appears in the list
- [ ] If creation fails, an error message is shown rather than silent close
- [ ] The optimistic/loading state is removed if creation fails (no orphan entries)

---

## Low Severity

### UX-016: Nested `<main>` elements on review-queue page

**Severity**: Low
**Category**: Accessibility / HTML Semantics

**File**: `web-app/src/app/review-queue/page.tsx` line 283

**Problem**: An inner `<main id="main-content">` is nested inside the layout's outer `<main>`. A document must have at most one `<main>` landmark. Screen readers expose all `<main>` elements as top-level landmarks, causing duplicate navigation entries.

**Acceptance Criteria**:
- [ ] Inner `<main id="main-content">` is changed to `<div id="main-content">`
- [ ] Skip-link targeting `#main-content` continues to work (href unchanged)
- [ ] HTML validation passes with no duplicate `<main>` landmark error

---

### UX-017: Resume modal silently renames session on title conflict

**Severity**: Low
**Category**: UX / Transparency

**File**: `web-app/src/components/sessions/ResumeSessionModal.tsx` lines 26–36

**Problem**: `hasConflict` is computed when the original session title is already in use, but the conflict state is never surfaced to the user. The title is silently modified. Users may resume a session under an unexpected name.

**Acceptance Criteria**:
- [ ] When `hasConflict` is true, an inline callout renders below the title input: "The name '[original]' is already in use. A unique name has been suggested."
- [ ] The user may edit the suggested name before confirming
- [ ] When `hasConflict` is false, no callout is shown (no regression)

---

### UX-018: Project group header duplicated with inline styles violating CSS architecture

**Severity**: Low
**Category**: Code Quality / CSS Architecture

**File**: `web-app/src/components/sessions/SessionList.tsx` lines 855–1073

**Problem**: Row mode and card mode duplicate 180+ lines of identical JSX for the project group header, including inline `style` strings. This violates `.claude/rules/css-architecture.md` which prohibits inline styles and requires `.css.ts` files for new components.

**Acceptance Criteria**:
- [ ] A `ProjectGroupHeader` component is extracted, shared by both row mode and card mode
- [ ] Styles are moved to `ProjectGroupHeader.css.ts` using vanilla-extract `recipe` for status badge variants
- [ ] No inline `style={{...}}` strings remain in the extracted component
- [ ] All existing visual states (collapsed/expanded, status badge variants) continue to render correctly

---

### UX-019: Mobile shortcut discoverability gap on main cockpit

**Severity**: Low
**Category**: UX / Mobile / Discoverability

**File**: `web-app/src/components/sessions/SessionDetailBar.css.ts` lines 41–53

**Problem**: Shortcut hints are hidden at viewport widths ≤768px. The review queue has a floating `?` button for mobile access to shortcuts, but the main cockpit does not. Mobile users have no path to discover keyboard shortcuts.

**Acceptance Criteria**:
- [ ] A floating `?` button is added to `CockpitShell` (or equivalent wrapper) for mobile viewports
- [ ] Button is hidden on desktop (≥769px) to avoid duplication if UX-010 is also implemented
- [ ] Button opens the same shortcuts modal as the desktop `?` key
- [ ] Button placement is bottom-right, consistent with the review queue pattern

---

### UX-020: New-session `+` button has redundant `aria-label` and `title`

**Severity**: Low
**Category**: Accessibility / Redundant Attributes

**File**: `web-app/src/components/sessions/SessionList.tsx` lines 597–604

**Problem**: Both `aria-label` and `title` are set to "Create new session (Ctrl+K)". The `title` attribute causes an additional tooltip on hover that is redundant with the `aria-label`. On touch devices, `title` tooltips are inaccessible. Screen readers may announce the label twice.

**Acceptance Criteria**:
- [ ] `title` attribute is removed from the button
- [ ] `aria-label="Create new session (Ctrl+K)"` is retained as the sole accessible name
- [ ] No visible tooltip regression on hover (if a tooltip is desired, use the project's standard tooltip component instead of `title`)

---

### UX-021: Auto-advance checkbox preference has no save confirmation

**Severity**: Low
**Category**: UX / Micro-feedback

**File**: `web-app/src/app/review-queue/page.tsx` lines 285–297

**Problem**: The auto-advance preference is saved to `localStorage` silently on toggle. Users get no confirmation that the preference was persisted, which can cause doubt about whether the toggle worked.

**Acceptance Criteria**:
- [ ] A brief "Preference saved" micro-notification (toast or inline) appears after the checkbox is toggled
- [ ] Notification auto-dismisses after 2–3 seconds
- [ ] Notification does not block page interaction

---

### UX-022: Memory pressure callout not dismissible

**Severity**: Low
**Category**: UX / Persistent UI

**File**: `web-app/src/components/sessions/` (MemoryPressureCallout component)

**Problem**: The memory pressure banner is persistent with no dismiss or snooze affordance. Once shown, it occupies layout space indefinitely even if the user has acknowledged it and wants to proceed.

**Acceptance Criteria**:
- [ ] A dismiss (×) button is added to the callout
- [ ] On dismiss, a `sessionStorage` flag is set so the callout does not reappear within the current browser session
- [ ] The callout reappears in new sessions if memory pressure persists (does not use `localStorage` which would suppress it permanently)

---

### UX-023: Errors silently discarded when overflow dialog dismissed with error showing

**Severity**: Low
**Category**: UX / Error Handling

**File**: `web-app/src/components/sessions/SessionActionsOverflow.tsx` lines 101–110

**Problem**: When the overflow dialog is dismissed while an error is actively displayed, the error state is cleared without being persisted anywhere. Users lose the error message with no opportunity to act on it or report it.

**Acceptance Criteria**:
- [ ] When the overflow dialog closes while `error` state is non-null, the error message is shown as a toast notification before the error state is cleared
- [ ] The toast persists long enough for the user to read it (minimum 5 seconds or user-dismissible)
- [ ] Normal close with no error continues to work without triggering a toast

---

### UX-024: `aria-live` region for `bulkFeedback` conditionally mounted

**Severity**: Low
**Category**: Accessibility / ARIA

**Note**: This is related to but distinct from UX-002 (BulkActions.tsx). See also the critical issue BUG in `BulkActions.tsx` line 42 documented above as UX-002.

**File**: `web-app/src/components/sessions/BulkActions.tsx` line 42

**Problem**: Tracked as part of UX-002 (Critical). This entry tracks the lower-severity aspect: even if the region is made unconditional, verify that `aria-atomic="true"` is set so partial updates within the region are announced as a complete replacement rather than additive announcements.

**Acceptance Criteria**:
- [ ] After UX-002 fix lands, confirm `aria-atomic="true"` is present on the live region
- [ ] Verify that rapid successive feedback messages (e.g., during multi-step bulk delete) announce correctly without partial-update artifacts

---

## Dependency Notes

The following tasks share files and should be coordinated to avoid merge conflicts:

- **UX-007 + UX-019** both add a floating `?` button — implement as a single shared component
- **UX-003 + UX-007 + UX-013** all modify `SessionActionsOverflow.tsx` — sequence or batch
- **UX-001 + UX-005** both involve `createPortal` + focus trap patterns — the same `useFocusTrap` import pattern applies to both
- **BUG-001** modifies `SessionActionsOverflow.tsx` and `SessionRow.tsx` — coordinate with UX-003 and UX-013
- **UX-002 + UX-024** are the same file; implement together in one pass
