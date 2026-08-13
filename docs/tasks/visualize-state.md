# Implementation Plan: Session State Visibility & Triage UX

Status: In Progress
Feature: visualize-state
Branch: claude-squad-visualize-state
Created: 2026-04-14

---

## Overview

This feature wires the already-built `DetectedStatus` detection infrastructure to the frontend, adds a terminal snapshot preview to session cards, replaces the interrupting approval modal with a non-interrupting side drawer, and stabilizes the review queue with a snapshot-on-enter pattern. Alongside the core feature work, the UX review identified 22 findings (C1-C4, H1-H6, M1-M8, L1-L4) and 5 missing patterns (MP1-MP5) that are organized into the phases below.

Key insight from research: the scope is primarily wiring and UX fixes, not building new infrastructure. `DetectedStatus`, `CurrentPaneRequest`, `ansi-to-html`, and `is_snapshot` event handling are all already in the codebase. The two P0 streaming bugs (stale sessions after WatchSessions reconnect, unstable React keys) need fixing alongside the feature work.

Progress: 32 / 32 tasks complete (all phases done).

---

## Phase 0: Quick Wins

Low effort, standalone fixes. Do these first — they can be merged independently and unblock nothing.

### TASK-001: Add role/tabIndex/onKeyDown to ReviewQueuePanel clickable divs
From: C3
Files:
- `web-app/src/components/sessions/ReviewQueuePanel.tsx`
Effort: S
Acceptance Criteria:
- [ ] `div.itemClickable` (line 344) has `role="button"`, `tabIndex={0}`, and `onKeyDown` handler that fires `onSessionClick` on Enter and Space
- [ ] Keyboard users can navigate to a review item and activate it without a mouse
- [ ] Existing `useReviewQueueNavigation` arrow-key behavior is not broken
Notes: The approve/deny/skip buttons already use proper `<button>` elements. Only the content area click zone needs this fix. Use `e.key === 'Enter' || e.key === ' '` in the handler; call `e.preventDefault()` on Space to avoid page scroll.

---

### TASK-002: Fix ReviewQueueBadge color-blind accessibility
From: H4
Files:
- `web-app/src/components/sessions/ReviewQueueBadge.tsx`
- `web-app/src/components/sessions/ReviewQueueBadge.module.css`
Effort: S
Acceptance Criteria:
- [ ] Priority levels use shape or text in addition to color — not color alone
- [ ] Compact mode badge on viewports >= 768px shows a text abbreviation alongside the emoji (e.g., "URG", "HIGH") or replaces emojis with icon+text pairs
- [ ] `UNCOMMITTED_CHANGES` and `STALE` reasons map to distinct CSS classes (not both `reasonUnspecified`)
- [ ] Existing `aria-label` values on compact badges are preserved
Notes: The simplest approach is adding a `<span aria-hidden="true">` text abbreviation next to each colored circle emoji in compact mode, visible on larger viewports. Do not change the `aria-label` strings — they are already correct.

---

### TASK-003: Pass sessionTitle to ApprovalCard
From: H5
Files:
- `web-app/src/components/sessions/ApprovalCard.tsx`
- All call sites that render `<ApprovalCard>` (search for `ApprovalCard` usage)
Effort: S
Acceptance Criteria:
- [ ] `ApprovalCard` accepts an optional `sessionTitle?: string` prop
- [ ] When `sessionTitle` is provided, it is displayed in place of the raw `approval.sessionId`
- [ ] When `sessionTitle` is not provided, behavior is unchanged (raw ID shown, no regression)
- [ ] At least one call site (inside `ReviewQueuePanel` or `SessionDetail`) passes the title
Notes: The title is already available as `item.sessionName` in `ReviewQueuePanel`. In `SessionDetail`, the session object is in scope. Keep the prop optional so non-updated call sites don't break.

---

### TASK-004: Increase session detail modal height to 80vh
From: H6
Files:
- `web-app/src/app/page.module.css`
Effort: S
Acceptance Criteria:
- [ ] `.sessionModal` has `height: 80vh` and `max-height: 80vh` (changed from 60vh)
- [ ] Fullscreen button still works and is still needed for users who want 100vh
- [ ] Narrow viewport behavior (mobile) is not regressed — confirm with a 375px viewport check
Notes: Lines 52-55 of `page.module.css`. A one-line change. Optionally, add a comment explaining the 80vh default rationale to replace the current "reduced for dev tools" comment.

---

### TASK-005: Add program name to session search
From: M2
Files:
- `web-app/src/components/sessions/SessionList.tsx`
Effort: S
Acceptance Criteria:
- [ ] Searching "claude" surfaces sessions where `session.program` contains "claude"
- [ ] Searching "aider" surfaces sessions where `session.program` contains "aider"
- [ ] Existing search behavior for title, path, branch, category, and tags is not regressed
Notes: The search filter is at lines 174-213 of `SessionList.tsx`. Add `session.program` to the match expression alongside the existing fields. Case-insensitive match already in use — apply the same approach.

---

### TASK-006: Wire ReviewItem data from ReviewQueueContext into SessionList cards
From: M3
Files:
- `web-app/src/components/sessions/SessionList.tsx`
- `web-app/src/app/page.tsx` (if context is not already available there)
Effort: S
Acceptance Criteria:
- [ ] `SessionList` consumes `ReviewQueueContext` and retrieves the map of items by session ID
- [ ] Each `SessionCard` rendered in the sessions list receives the matching `ReviewItem` as the `reviewItem` prop when one exists
- [ ] Sessions in the review queue now show the `ReviewQueueBadge` on their card in the sessions list (line 566-573 of `SessionCard.tsx` activates)
- [ ] Sessions not in the queue are not affected
Notes: This is described in the UX review as "dead code that just needs to be exercised." The `reviewItem` prop and its display logic inside `SessionCard` already exist and are tested. This task is the wiring only.

---

### TASK-007: Add ISO timestamp to Last Activity title attribute
From: M5
Files:
- `web-app/src/components/sessions/SessionCard.tsx`
Effort: S
Acceptance Criteria:
- [ ] The `<time>` element for Last Activity has a `title` attribute containing the ISO timestamp string (e.g., `title="2026-04-14T10:23:45Z"`)
- [ ] Hovering over the Last Activity field shows the absolute time in a tooltip
- [ ] The existing `dateTime` attribute is preserved (it already has the ISO value — the `title` just needs to mirror it)
Notes: Lines 706-720 of `SessionCard.tsx`. The `dateTime` attribute already holds the ISO string. Set `title={lastActivity.toISOString()}` (or equivalent) to surface it on hover. Do not change the displayed relative-time text.

---

### TASK-008: Add role="dialog" to restart confirm dialog
From: M8
Files:
- `web-app/src/components/sessions/SessionCard.tsx`
Effort: S
Acceptance Criteria:
- [ ] The restart confirm dialog div (lines 382-406) has `role="dialog"`, `aria-modal="true"`, and `aria-labelledby` referencing its heading ID
- [ ] The heading inside the restart dialog has the matching `id` attribute
- [ ] Screen reader announces the dialog and its title when it opens
Notes: This is the minimum ARIA fix for a safety-critical (destructive) dialog. Full focus trapping is tracked separately in TASK-020. Matching the pattern already used in the checkpoint dialog (lines 407-449) is the reference implementation.

---

### TASK-009: Add aria-hidden to action button emoji spans
From: L1
Files:
- `web-app/src/components/sessions/SessionCard.tsx`
Effort: S
Acceptance Criteria:
- [ ] Each emoji prefix in action buttons (Resume, Pause, Delete, etc.) is wrapped in `<span aria-hidden="true">...</span>`
- [ ] Screen readers read only the button's `aria-label`, not the emoji description followed by the label
- [ ] No visible UI change
Notes: Lines 741-831. A straightforward wrapping change. The `aria-label` attributes on the buttons are already correct — this just prevents double-reading.

---

### TASK-010: Remove "Group by:" prefix from dropdown options
From: L2
Files:
- `web-app/src/components/sessions/SessionList.tsx`
Effort: S
Acceptance Criteria:
- [ ] Each `<option>` in the grouping strategy select reads only the strategy name (e.g., "Category", "Tag", "Branch") without the "Group by: " prefix
- [ ] The select's `aria-label="Group sessions by"` is preserved for context
- [ ] No functional change to grouping behavior
Notes: Lines 436-443. String change only.

---

### TASK-011: Remove floating duplicate "?" help button
From: L3
Files:
- `web-app/src/app/page.tsx`
Effort: S
Acceptance Criteria:
- [ ] The fixed floating "?" button (lines 534-542) is removed from `page.tsx`
- [ ] The help shortcut modal is still accessible via the header button in `Header.tsx` (line 162-171)
- [ ] No JavaScript errors or layout shifts after removal
Notes: Delete only the floating button JSX and any CSS class that was solely for it. Do not touch the header button or the modal itself.

---

### TASK-012: Remove "coming soon" entries from keyboard shortcuts help modal
From: L4
Files:
- `web-app/src/app/page.tsx`
Effort: S
Acceptance Criteria:
- [ ] The "/" (focus search) and arrow key (navigate sessions) entries are removed from the shortcuts list
- [ ] No remaining entries have "coming soon" or similar placeholder text
- [ ] The modal still renders correctly with fewer entries
Notes: Lines 522-529. Delete only the unimplemented entries. Document the planned shortcuts in a code comment instead if desired.

---

## Phase 1: Core Feature Work

The main visualize-state deliverables. These tasks build on or depend on each other and should be sequenced as shown.

### TASK-013: Extend SessionStatusChangedEvent proto with detected_status fields
From: C1, synthesis.md recommendation #1
Files:
- `proto/session/v1/events.proto`
- Generated Go and TypeScript files (regenerate with `make generate-proto`)
Effort: S
Acceptance Criteria:
- [ ] `SessionStatusChangedEvent` has two new optional fields: `detected_status` (string, field 4) and `detected_context` (string, field 5)
- [ ] Proto compiles and `make generate-proto` succeeds without errors
- [ ] No existing fields renumbered; new fields are additive only
- [ ] Generated TypeScript types include `detectedStatus` and `detectedContext` as optional string fields
Notes: Confirm field numbers 4 and 5 are not already in use in `events.proto` before adding. The `DetectedStatus` constant names from `session/detection/detector.go` are the intended values for `detected_status`. This is a prerequisite for TASK-014.
Dependencies: None (proto change is standalone)

---

### TASK-014: Wire InstanceStatusManager to emit detected_status on status change
From: C1, synthesis.md recommendation #1
Files:
- `server/services/session_service.go` (locate `InstanceStatusManager`)
- `session/detection/detector.go` (read-only reference)
Effort: M
Acceptance Criteria:
- [ ] `InstanceStatusManager` calls `detector.DetectWithContext()` on new scrollback arriving from a session
- [ ] Detection is debounced at 200ms to avoid per-character detection churn
- [ ] A `SessionStatusChangedEvent` is emitted only when `DetectedStatus` changes (not on every scrollback)
- [ ] The emitted event populates `detected_status` and `detected_context` from the detector result
- [ ] Existing `SessionStatusChangedEvent` emission for lifecycle state changes (Running/Paused) is not affected
Notes: `InstanceStatusManager` is in `session/instance_status.go`. Do NOT add `OnPatternDetected()` to it — it is a controller registry, not a detector. Instead: in the existing `SessionStatusChangedEvent` emission path (wherever the service sends status updates to WatchSessions subscribers), call `statusManager.GetStatus(inst)` and populate the new `detected_status`/`detected_context` fields from `statusInfo.ClaudeStatus` and `statusInfo.StatusContext`. The `ReviewQueuePoller` at `review_queue_poller.go:340` already does this exact call as a reference pattern. Add debounce (200ms) to avoid emitting on every scrollback write.
Dependencies: TASK-013

---

### TASK-015: Display detected_status context in SessionCard
From: C1, synthesis.md recommendation #1, MP2
Files:
- `web-app/src/components/sessions/SessionCard.tsx`
- `web-app/src/components/sessions/SessionCard.module.css`
Effort: M
Acceptance Criteria:
- [ ] When `detectedContext` is present on a session's status event, it is displayed below the lifecycle status chip on the session card
- [ ] The detected context uses the same color/shape vocabulary as `ReviewQueueBadge` (so the two systems feel unified — see MP2)
- [ ] Displaying "Waiting for input" and "Tests failing" are visually distinct (different icon or shape, not color alone — see MP2 requirement)
- [ ] When `detectedContext` is absent or empty, the card renders identically to today
- [ ] "Running" sessions with meaningfully different detected states are now visually distinguishable at a glance
Notes: Extract the reason-to-label mapping from `ReviewQueueBadge.tsx` into a shared utility function that both `SessionCard` and `ReviewQueueBadge` import. Do not duplicate the label strings.
Dependencies: TASK-013

---

### TASK-016: Fix WatchSessions reconnect stale-state bug (P0)
From: synthesis.md P0 streaming bug, M7
Files:
- `web-app/src/` (find the WatchSessions subscription / autoWatch hook)
- `server/services/session_service.go` (WatchSessions handler if server-side fix is needed)
Effort: M
Acceptance Criteria:
- [ ] On WebSocket reconnect, the client calls `ListSessions()` before re-subscribing to `WatchSessions` to populate full current state
- [ ] Sessions that changed while disconnected are updated immediately on reconnect (not showing stale data)
- [ ] A `lastEventTime` ref is tracked in the WatchSessions subscription hook
- [ ] If `Date.now() - lastEventTime > 15_000ms`, a "status stale" indicator is shown on all session cards (see TASK-017)
Notes: The review queue already handles reconnect correctly via `is_snapshot = true`. Mirror that approach for `WatchSessions`. This is the P0 bug from the pitfalls research — it causes silent stale data with no user feedback.
Dependencies: None (standalone streaming fix)

---

### TASK-017: Add live connection status indicator to header
From: M7, synthesis.md P0 staleness indicator
Files:
- `web-app/src/components/layout/Header.tsx`
- `web-app/src/components/layout/Header.module.css` (or Header.css.ts for new styles)
Effort: S
Acceptance Criteria:
- [ ] A "Live" indicator (green dot + label, or equivalent) is shown in the header when the WatchSessions connection is active
- [ ] When disconnected or stale (>15s since last event), the indicator changes to a "Disconnected" or "Stale" state with a yellow/amber color
- [ ] The indicator does not use color as the only signal (add text or shape per accessibility rules)
- [ ] Clicking the disconnected state triggers a reconnect or page-refresh prompt
Notes: This pairs with TASK-016. The indicator reads the connection state that TASK-016 tracks. New styles should use vanilla-extract (.css.ts) per the css-architecture rules.
Dependencies: TASK-016

---

### TASK-018: Implement terminal snapshot fetch and render in SessionCard
From: synthesis.md recommendation #2, MP1
Files:
- `web-app/src/components/sessions/SessionCard.tsx`
- `web-app/src/components/sessions/SessionCard.css.ts` (new vanilla-extract styles)
- `web-app/src/hooks/useTerminalSnapshot.ts` (new hook, or inline in card)
Effort: L
Acceptance Criteria:
- [ ] Each session card fetches the last 20 lines of terminal output via `CurrentPaneRequest` with `include_escapes: true`
- [ ] Output is rendered using `ansi-to-html` (already in `package.json`) — no raw ANSI escape codes visible in the UI
- [ ] Snapshot is shown as a fixed-height (120px, overflow: hidden) preview pane on the card, collapsible or on-hover
- [ ] Rendered HTML is cached in React state with a 5-second TTL — not re-fetched on every render
- [ ] If the last N lines are all blank (terminal was cleared), "No recent output" placeholder is shown instead of empty whitespace (MP1 requirement)
- [ ] If ANSI rendering fails, plain text fallback is used — the raw escape code failure state (e.g., `^[[32m` visible) must not occur (MP1 requirement)
- [ ] Snapshot fetch does not block card render — card appears immediately, snapshot loads async
Notes: `CurrentPaneRequest` CANNOT be called standalone — it is the required handshake for opening a `StreamTerminal` WebSocket. Use a different approach: add a new `GetTerminalSnapshot(session_id, last_n_lines)` RPC backed by `inst.Preview()` (already in `dependencies.go:114`). This returns terminal content as a string without requiring an active stream, making it suitable for polling from session cards. Styles must be in a `.css.ts` file per project CSS architecture rules. The `xterm.js` per-card approach is explicitly rejected (10MB memory, 15-30 FPS on 20 cards) — use `ansi-to-html` only.
Dependencies: None (can be built in parallel with status wiring)

---

### TASK-019: Replace approval modal with non-interrupting side drawer
From: C2, synthesis.md recommendation #4, MP3
Files:
- `web-app/src/components/sessions/ApprovalDrawer.tsx` (new component)
- `web-app/src/components/sessions/ApprovalDrawer.css.ts` (new vanilla-extract styles)
- `web-app/src/components/layout/Header.tsx` (make ApprovalNavBadge clickable)
- `web-app/src/components/layout/ApprovalNavBadge.module.css` (remove `pointer-events: none`)
- `web-app/src/components/sessions/SessionDetail.tsx` (remove `ApprovalPanel` from terminal tab)
Effort: L
Acceptance Criteria:
- [ ] `ApprovalNavBadge` in the header is interactive (`pointer-events: auto`); clicking it opens the approval side drawer
- [ ] The approval drawer is non-modal (does not block the rest of the UI)
- [ ] The drawer lists all pending approvals in priority order (urgency by time-to-expire, not session name — MP3 requirement)
- [ ] Each approval entry in the drawer shows session title (not raw ID), tool name, expiry countdown, and Approve/Deny buttons
- [ ] `ApprovalPanel` is removed from the terminal tab in `SessionDetail.tsx` — approvals no longer interrupt the terminal view
- [ ] When an approval expires while the drawer is closed, it is cleanly removed with an `aria-live` region announcement (MP3 requirement)
- [ ] When the drawer is open and a timer expires, the expired card shows an expiry state without layout shift (Dismiss replaces Approve/Deny — MP3 concern)
- [ ] The invisible Enter-to-approve keyboard shortcut in `ApprovalPanel.tsx` (lines 44-62) is removed or clearly documented with a visible affordance
- [ ] HTTP blocking approval protocol (`ApprovalStore`) is unchanged — only the UI layer changes
Notes: The drawer should be a right-side panel anchored to the viewport, not a route change. The badge already shows the count — the only missing piece is `pointer-events` and a drawer component. New styles in `.css.ts`. `PendingApproval.ExpiresAt` is fully populated and sent to frontend already — no backend changes needed for the expiry countdown.
Dependencies: TASK-003 (sessionTitle on ApprovalCard — reuse that work here)

---

### TASK-020: Implement stable review queue snapshot-on-enter pattern
From: MP4, requirements.md (queue jumps around mid-review)
Files:
- `web-app/src/components/sessions/ReviewQueuePanel.tsx`
- `web-app/src/context/ReviewQueueContext.tsx` (or wherever the live items array lives)
Effort: M
Acceptance Criteria:
- [ ] When the user opens the review queue panel, the current item list is captured as a frozen `reviewingItems` snapshot
- [ ] New items arriving via `useReviewQueueContext` while the user is reviewing appear at the bottom with a "N new item(s) added — click to refresh" notice, not injected into the active list
- [ ] Clicking the notice (or re-opening the panel) refreshes the snapshot to include new items
- [ ] Items completed during review are removed from the snapshot (the queue advances correctly)
- [ ] The pattern mirrors Twitter-style feed updates — position is stable until the user chooses to refresh
Notes: The context currently exposes a live `items` array. This task adds a `reviewingItems` state that is set on panel open and updated only on explicit refresh or panel re-entry. The live array continues to drive the nav badge count so the badge stays accurate.
Dependencies: None (can build in parallel with other Phase 1 tasks)

---

## Phase 2: UX Fixes

Higher-effort UX findings that should be coordinated with Phase 1 feature work. These improve the product significantly but are not hard prerequisites for the core feature.

### TASK-021: Add ARIA roles and focus trapping to all four SessionCard dialog types
From: C4, M8 (M8 is the minimum ARIA fix; C4 adds focus trapping)
Files:
- `web-app/src/components/sessions/SessionCard.tsx`
- `web-app/src/hooks/useFocusTrap.ts` (new shared hook, or use an existing library)
Effort: M
Acceptance Criteria:
- [ ] All four dialog types (rename, restart confirm, checkpoint, fork) have `role="dialog"`, `aria-modal="true"`, and `aria-labelledby` referencing their heading
- [ ] Keyboard focus is trapped inside each dialog while it is open (Tab/Shift+Tab cycle within the dialog)
- [ ] Pressing Escape closes the dialog and returns focus to the triggering element
- [ ] Opening a dialog moves focus to the first interactive element inside it
- [ ] The rename and restart dialogs are updated to match the checkpoint/fork pattern (which already has `role="dialog"` and `aria-modal`)
Notes: TASK-008 does the minimum ARIA fix for the restart dialog in Phase 0. This task adds the focus trap to all four and ensures consistency. A `useFocusTrap` hook using `useEffect` + `querySelectorAll` for focusable elements is the recommended approach. If a well-maintained library like `focus-trap-react` is preferred, check bundle size impact first.
Dependencies: TASK-008 (restart dialog ARIA from Phase 0 is the starting reference)

---

### TASK-022: Restructure session card information hierarchy
From: H1
Files:
- `web-app/src/components/sessions/SessionCard.tsx`
- `web-app/src/components/sessions/SessionCard.module.css`
Effort: L
Acceptance Criteria:
- [ ] Tier 1 (always visible): session name, rich status (lifecycle + detected context), last activity time-ago — all in the card header area
- [ ] Tier 2 (one line of context): branch and/or terminal snapshot preview (from TASK-018)
- [ ] Tier 3 (on-demand): path, program, working dir, GitHub details — accessible via the Info tab in `SessionDetail`, not cluttering the card body
- [ ] "Last Activity" timestamp is in the card header, not buried below Created/Updated rows
- [ ] The change is visually validated at both narrow (375px) and wide (1280px) viewports
Notes: This is a significant visual restructure. Coordinate with TASK-015 (detected status display) and TASK-018 (terminal snapshot) since both add content to the card. The goal is a card where a user triaging 15 sessions can answer "Is this stuck? What is it doing?" in under 2 seconds.
Dependencies: TASK-015, TASK-018 (to place the new elements correctly)

---

### TASK-023: Implement actions menu progressive disclosure on desktop
From: H2
Files:
- `web-app/src/components/sessions/SessionCard.tsx`
- `web-app/src/components/sessions/SessionCard.module.css`
Effort: M
Acceptance Criteria:
- [ ] On desktop (>= 768px), only 1-2 contextually relevant primary actions are shown directly on the card (e.g., "Resume" when paused, "Pause" when running, nothing when loading)
- [ ] All other actions are in a "..." overflow menu button
- [ ] Destructive actions (Delete, Restart) remain behind confirmation flows
- [ ] Mobile accordion behavior (existing) is not changed
- [ ] The overflow menu is keyboard accessible (button + dropdown with arrow key navigation)
Notes: The CSS mobile accordion (`SessionCard.module.css` lines 543-549) is the reference pattern for "show the important action, hide the rest." Apply the same principle on desktop at a per-card level. The primary action shown should be determined by the session's lifecycle status.

---

### TASK-024: Collapse review queue filter controls by default
From: H3
Files:
- `web-app/src/components/sessions/ReviewQueuePanel.tsx`
- `web-app/src/components/sessions/ReviewQueuePanel.module.css`
Effort: M
Acceptance Criteria:
- [ ] Filter controls (priority buttons + reason buttons) are in a collapsible section, collapsed by default
- [ ] A "Filter" toggle button shows the current active filter state (e.g., "Filter: Urgent" when active, "Filter" when inactive)
- [ ] The header prominently shows a count summary (e.g., "3 approvals, 1 error, 2 idle") replacing the de-emphasized statistics
- [ ] If the oldest item is over 5 minutes old, a "heads up" callout is shown (e.g., "Oldest item: 8m") prominently, not as a passive stat
- [ ] The mutual-exclusion behavior between priority and reason filters is preserved (or removed if deemed too surprising — document the decision)
Notes: The filter controls currently occupy 30-40% of panel height before any items are shown. This task reclaims that space for the item list. The summary count is the primary triage signal; filters are the advanced option.

---

### TASK-025: Implement focus management on modal open and close
From: M4
Files:
- `web-app/src/app/page.tsx`
Effort: M
Acceptance Criteria:
- [ ] When the session detail modal opens, focus moves to the modal container (which has `tabIndex={-1}`)
- [ ] When the modal closes, focus returns to the session card that was clicked to open it
- [ ] The same pattern applies to the session creation wizard modal
- [ ] The same pattern applies to the keyboard shortcuts help modal
- [ ] A keyboard or screen reader user does not lose their place in the page on either transition
Notes: Store a ref to the triggering element before calling `setSelectedSession()`. On close (`closeSession()`), call `.focus()` on the stored ref. The modal container should have `tabIndex={-1}` to be programmatically focusable. This is WCAG 2.4.3.

---

### TASK-026: Render session card dialogs through React portal
From: M6
Files:
- `web-app/src/components/sessions/SessionCard.tsx`
Effort: M
Acceptance Criteria:
- [ ] Rename, restart confirm, checkpoint, and fork dialogs are rendered via `ReactDOM.createPortal` to `document.body`
- [ ] No z-index conflicts with the session detail modal, sticky header, or other fixed elements
- [ ] Dialog behavior (open, close, form submission) is unchanged
- [ ] The restart confirmation dialog (destructive action) is fully visible and not obscured by other elements
Notes: Portal rendering is particularly important for the restart confirmation, which could be obscured under the current in-place rendering. The backdrop click behavior (currently broken per C4) can be fixed simultaneously: with portal rendering, a backdrop `<div>` covering the viewport and an inner dialog `<div>` with `stopPropagation` is the standard pattern.

---

### TASK-027: Build unified status label visual taxonomy
From: MP2
Files:
- `web-app/src/components/sessions/StatusBadge.tsx` (new shared component, or extend existing)
- `web-app/src/components/sessions/StatusBadge.css.ts` (vanilla-extract styles)
- `web-app/src/components/sessions/ReviewQueueBadge.tsx` (update to use shared component)
- `web-app/src/components/sessions/SessionCard.tsx` (update to use shared component)
Effort: M
Acceptance Criteria:
- [ ] A shared `StatusBadge` component encapsulates the color + shape + text treatment for all status/reason labels
- [ ] `ReviewQueueBadge` and `SessionCard` both use this component — they cannot diverge
- [ ] Each reason category has a unique shape or icon in addition to color (warning triangle for errors, clock for idle, input cursor for waiting, checkmark for complete)
- [ ] The badge is readable at the sizes used in both the review queue and the session card
- [ ] Adding a new `DetectedStatus` reason requires changing only the taxonomy in `StatusBadge`, not in both components separately
Notes: This component is the result of extracting the reason-to-label mapping from `ReviewQueueBadge.tsx` (referenced in TASK-015). Build it properly here so both consumers share it. New styles in `.css.ts` per CSS architecture rules.
Dependencies: TASK-015 (identifies what the shared logic is)

---

## Phase 3: Polish and Accessibility

These improve quality and are not blockers, but should be completed before the feature is considered done.

### TASK-028: Resolve tags vs. category redundancy
From: M1
Files:
- `web-app/src/components/sessions/SessionCard.tsx`
- `web-app/src/components/sessions/SessionList.tsx`
Effort: M
Acceptance Criteria:
- [ ] The card no longer renders both a `category` badge and `tags` pills as separate distinct elements
- [ ] If tags are the migration target (per CLAUDE.md and docs/category-vs-tag-analysis.md), only tags are shown on the card; the category badge is removed
- [ ] A single filter control covers tags (the category filter dropdown is consolidated or removed)
- [ ] The "Edit Tags" / "Add Tags" button is moved into the actions overflow menu rather than being a permanent card footer element
Notes: Read `docs/category-vs-tag-analysis.md` before implementing — there may be an existing decision on this. The auto-migration of categories to tags is already implemented (per CLAUDE.md); this task cleans up the dual display that results.

---

### TASK-029: Handle terminal preview empty and error states
From: MP1
Files:
- `web-app/src/components/sessions/SessionCard.tsx` (preview section from TASK-018)
- `web-app/src/hooks/useTerminalSnapshot.ts` (from TASK-018)
Effort: S
Acceptance Criteria:
- [ ] If the last 20 lines are all blank (terminal cleared), the preview shows "No recent output" placeholder text
- [ ] If `CurrentPaneRequest` returns an error or times out, the preview shows a graceful fallback (e.g., "Preview unavailable") rather than an error state or empty box
- [ ] If ANSI rendering throws, the fallback shows plain text (ANSI stripped) — raw escape codes must never be shown to the user
- [ ] All three states are tested (happy path, empty, error)
Notes: These are the three states from MP1. TASK-018 builds the happy path; this task adds the error states. Treat as a follow-on to TASK-018.
Dependencies: TASK-018

---

### TASK-030: Add approval expiry handling to side drawer
From: MP3
Files:
- `web-app/src/components/sessions/ApprovalDrawer.tsx` (from TASK-019)
Effort: S
Acceptance Criteria:
- [ ] When an approval expires while the drawer is closed, an `aria-live` region announces "Approval expired for [session name]"
- [ ] When the drawer is open and a timer reaches zero, the expired card transitions to an "Expired" state with Dismiss button; Approve/Deny disable; layout does not shift
- [ ] Multiple simultaneous approvals are sorted by time-to-expire (soonest first), not session name
Notes: `PendingApproval.ExpiresAt` is fully populated and already sent to frontend as `timestamppb.Timestamp`. No backend changes needed. The `aria-live` announcement on expiry is the only new frontend work here.
Dependencies: TASK-019

---

### TASK-031: Audit and fix desktop action button touch targets
From: MP5
Files:
- `web-app/src/components/sessions/SessionCard.module.css`
Effort: S
Acceptance Criteria:
- [ ] All action buttons on desktop meet 44x44 CSS pixel minimum touch target size (WCAG 2.1 AA)
- [ ] The `actionButton` class at `padding: 6px 16px` is verified for height — if line-height + padding < 44px, increase padding or add `min-height: 44px`
- [ ] The "Edit Tags" button (if kept visible per TASK-028 outcome) also meets the 44px minimum
- [ ] Mobile action buttons already at `min-height: 44px` are not regressed
Notes: This is a CSS audit and fix. Measure actual rendered height in browser dev tools before and after.

---

### TASK-032: Fix Session ID stability — use UUID instead of title as React key
From: synthesis.md P1 pitfall, open question OQ-1
Files:
- `session/instance.go` (check how `id` is currently assigned)
- `proto/session/v1/types.proto` (review `id` field comment)
- Session creation code path (assign UUID at creation time)
- `web-app/src/` (update any React key that uses `session.id` — verify stable key behavior)
Effort: M
Acceptance Criteria:
- [ ] `Session.id` is a stable UUID generated at session creation time, not derived from the session title
- [ ] Renaming a session does not change its `id`
- [ ] React keys in the sessions list and review queue use `session.id` and no longer thrash on rename
- [ ] Existing sessions loaded from persisted state continue to work (migration path for sessions that currently use title as ID)
Notes: Resolve open question OQ-1 first: check `session/instance.go` to confirm whether a UUID is already generated but not exposed, or if title is truly the only ID. If a UUID already exists, this is a proto and wiring fix. If not, it requires generating a UUID at creation. Either way, the migration path for existing sessions must be handled gracefully (title-based IDs become the fallback for old sessions).

---

## Bug Tracker

Independent bugs found during UX review and research. These are separate from planned feature tasks.

### BUG-014: ApprovalNavBadge is not interactive (pointer-events: none)
Severity: High
Status: Open (fix included in TASK-019)
File: `web-app/src/components/layout/ApprovalNavBadge.module.css`
Impact: The approval count badge in the header is visible but cannot be clicked. Users have no way to access pending approvals without navigating to the review queue manually.
Fix: Remove `pointer-events: none` from the badge CSS. This is done as part of TASK-019 (approval drawer). If TASK-019 is delayed, this single-line CSS fix can ship independently.

### BUG-015: Invisible Enter-to-approve keyboard shortcut fires without visible affordance
Severity: High
Status: Open (fix included in TASK-019)
File: `web-app/src/components/sessions/ApprovalPanel.tsx` lines 44-62
Impact: Pressing Enter anywhere in the session detail (when not focused on an input) may silently approve a pending tool request. No visual affordance communicates this behavior. This is a significant error-risk for users who press Enter to confirm other UI elements.
Fix: Remove the shortcut, or add a visible tooltip/label explaining it. Addressed in TASK-019 when the panel is replaced by the drawer.

### BUG-016: Mutual-exclusion behavior in review queue filters is undiscoverable
Severity: Medium
Status: Open (addressed by TASK-024)
File: `web-app/src/components/sessions/ReviewQueuePanel.tsx` lines 249-313
Impact: Changing the priority filter silently clears the reason filter and vice versa. No UI feedback communicates this. Users who set both filters discover one has been cleared only after seeing unexpected results.
Fix: Either add a visible notification when a filter is cleared ("Reason filter cleared"), or remove the mutual exclusion. Addressed in TASK-024.

### BUG-017: Modal backdrop click does not dismiss session card dialogs
Severity: Medium
Status: Open (fixed as part of TASK-026)
File: `web-app/src/components/sessions/SessionCard.tsx` lines 338-518
Impact: Users expect clicking outside a dialog to dismiss it. The current `e.stopPropagation()` on dialog content prevents this, but the backdrop does not have its own dismiss handler either. Click-outside is non-functional for rename, restart, checkpoint, and fork dialogs.
Fix: With portal rendering (TASK-026), add an `onClick` handler to the backdrop element that calls `e.stopPropagation()` and closes the dialog. The inner dialog content stops propagation to prevent clicks on the content from closing.

---

## Dependency Map

```
Phase 0 tasks (TASK-001 through TASK-012)
  All independent — can be done in any order, any can ship first

Phase 1 core work:
  TASK-013 (proto extension)
    |
    +-- TASK-014 (Go wiring for detection emission)
    +-- TASK-015 (frontend status display)
         |
         +-- TASK-022 (card hierarchy, Phase 2) — best done after 015

  TASK-016 (reconnect fix)
    |
    +-- TASK-017 (staleness indicator)

  TASK-018 (terminal snapshot) — independent, parallel with above
    |
    +-- TASK-029 (empty/error states, Phase 3)

  TASK-003 (sessionTitle prop, Phase 0)
    |
    +-- TASK-019 (approval drawer) — reuses sessionTitle work
         |
         +-- TASK-030 (expiry handling, Phase 3)

  TASK-020 (review queue snapshot) — independent

Phase 2 dependencies:
  TASK-015 --> TASK-027 (shared status taxonomy)
  TASK-018 --> TASK-022 (card hierarchy needs snapshot placement)
  TASK-008 (Phase 0, restart ARIA) --> TASK-021 (full focus trapping)
```

---

## Open Questions

All open questions resolved via codebase investigation (2026-04-14).

### OQ-1: Session ID stability — RESOLVED
**Answer**: `Instance.Title` IS the session identifier. No UUID field exists anywhere in `Instance`. `InstanceStatusManager` uses `instance.Title` as its map key. This is confirmed current behavior.
**Impact on TASK-032**: This is a real structural issue. Renaming a session changes its ID everywhere — React keys, status manager map, review queue items. TASK-032 requires adding a UUID field at Instance creation time and migrating existing sessions.

### OQ-2: CurrentPaneRequest outside active StreamTerminal — RESOLVED
**Answer**: `CurrentPaneRequest` is **required as the first handshake message** when opening a `StreamTerminal` WebSocket — it cannot be called standalone. `connectrpc_websocket.go:442-443` returns an error if it is missing.
**Impact on TASK-018**: The snapshot strategy must change. Options:
1. **Use `inst.Preview()`** — already called in `scanSessionsOnStartup` (`dependencies.go:114`); returns terminal content string without a stream. Add a new lightweight `GetTerminalSnapshot(session_id) → string` RPC backed by this method. **Recommended.**
2. Open a short-lived `StreamTerminal` connection per card — expensive, not suitable for 20+ cards.
**Action**: TASK-018 should add a `GetTerminalSnapshot` RPC, not use `CurrentPaneRequest`.

### OQ-3: InstanceStatusManager location and interface — RESOLVED
**Answer**: Located in `session/instance_status.go`. It is a controller registry — it does NOT run detection itself. `GetStatus(instance)` calls `controller.GetCurrentStatus()` which returns `(DetectedStatus, string context)`. The `InstanceStatusInfo` struct already has `ClaudeStatus detection.DetectedStatus` and `StatusContext string` fields.
**Impact on TASK-014**: Do NOT add `OnPatternDetected()` to `InstanceStatusManager`. Instead: in the `SessionStatusChangedEvent` emission path, call `statusManager.GetStatus(inst)` (already used by review queue poller at `review_queue_poller.go:340`) and include `ClaudeStatus` + `StatusContext` in the event. The detection is already happening via `ClaudeController.GetCurrentStatus()` — just needs to be wired to the event stream.

### OQ-4: PendingApproval.ExpiresAt — RESOLVED (non-issue)
**Answer**: Fully implemented end-to-end. `ExpiresAt` is populated at creation (`approval_handler.go:244`: `time.Now().Add(h.approvalTimeout())`), sent to the frontend as `timestamppb.Timestamp` (`approval_service.go:120`), and `remaining seconds` is already calculated (`approval_service.go:100`). The countdown timer in `ApprovalCard.tsx` already works with real data. TASK-019 and TASK-030 can proceed without any backend changes.
