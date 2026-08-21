# UX Review: Session State Visibility & Triage

**Reviewer**: UX/Usability Specialist
**Date**: 2026-04-14
**Branch**: `claude-squad-visualize-state`
**Scope**: Sessions list, session cards, approval panel/flow, review queue, navigation header

---

## Executive Summary

The current Stapler Squad web UI is structurally sound and has good bones: dark/light mode support, keyboard shortcuts, accessible landmarks, and a working review queue pipeline. However, several compounding issues make it difficult for users managing 3-20+ concurrent sessions to triage quickly.

The three known pain points from requirements.md are all confirmed by the code review, and they share a root cause: **the UI was designed around inspecting one session at a time, not around triaging many**. The session card is a detailed data form when it should be a triage dashboard card. The approval flow opens a full modal when it should surface inline. The review queue tries to do too much filtering/sorting for a flow that should be linear.

The planned improvements (terminal preview, rich status labels, non-interrupting approvals, stable queue) all address the right problems. This review provides specific findings and recommendations to shape how those improvements are designed.

**Severity breakdown**: 4 critical, 6 high, 8 medium, 4 low.

---

## Critical Issues (must fix before ship)

### C1 - Status labels are binary and untrustworthy

**Category**: Usability
**Heuristic**: Visibility of System Status (Nielsen #1)
**File**: `web-app/src/components/sessions/SessionCard.tsx` lines 85-100; `SessionCard.module.css` lines 127-156

The status enum has only five values: Running, Ready, Paused, Loading, Needs Approval. The backend already detects richer states (confirmed in requirements.md: "Claude is waiting for your input", tests failing, idle timeout, etc.) via `AttentionReason` in `ReviewItem`. These richer reasons exist in the review queue (`ReviewQueueBadge.tsx` handles INPUT_REQUIRED, ERROR_STATE, IDLE, TASK_COMPLETE, STALE, WAITING_FOR_USER) but are entirely absent from the session card status chip.

The result is that two sessions showing "Running" may have completely different actual states — one actively generating code, another blocked waiting for input. Users cannot trust the status chip and must open each session to verify, defeating the purpose of the list view.

**Recommended fix**: Wire the `AttentionReason` from the review queue context into the SessionCard. Replace the single status chip with two pieces of information: the lifecycle state (Running/Paused/Loading) and, when available, the detected activity reason (e.g., "Waiting for input", "Tests failing", "Task complete"). The `ReviewQueueBadge` component already has the reason text logic — extract it to a shared utility and use it on the card.

---

### C2 - Approval panel interrupts active work context

**Category**: Interaction Design
**Heuristic**: User Control and Freedom (Nielsen #3)
**File**: `web-app/src/components/sessions/SessionDetail.tsx` lines 258-259; `ApprovalPanel.tsx`

When a hook approval arrives, `ApprovalPanel` is rendered **inside the terminal tab** of `SessionDetail`. From the application flow (`page.tsx` lines 446-466), the session detail modal is a full-screen overlay. This means a pending approval on any session effectively hijacks the user's current view — they must navigate away from what they were doing to see and act on the approval.

The keyboard shortcut in `ApprovalPanel.tsx` (lines 44-62) fires Enter/Shift+Enter to approve/deny when exactly one approval is pending, but only when not focused on an input/textarea. This creates an invisible affordance where pressing Enter on any session detail may silently approve a tool request, which is a significant error risk.

The `ApprovalNavBadge` exists in the header (`Header.tsx` line 137) and is the right directional improvement, but it is currently only a badge — clicking it does not open an inline drawer or panel. The badge's CSS (`ApprovalNavBadge.module.css`) shows it has `pointer-events: none`, meaning it is not interactive.

**Recommended fix**: Make `ApprovalNavBadge` clickable and open a side drawer listing pending approvals. Move approval actions out of the terminal tab. The badge should open the drawer without leaving the current session view. Remove or clearly document the invisible keyboard shortcut — it violates the heuristic of error prevention by providing a powerful action with no visible affordance.

---

### C3 - Review queue items are not keyboard-accessible

**Category**: Accessibility (POUR: Operable)
**Heuristic**: Keyboard Accessibility (WCAG 2.1.1)
**File**: `web-app/src/components/sessions/ReviewQueuePanel.tsx` lines 340-428

Each review queue item has a clickable div (`div.itemClickable`, line 344) and action buttons (`approveButton`, `denyButton`, `skipButton`) in a separate adjacent div (`div.itemActions`). The clickable content area uses `onClick` but has no `role`, no `tabIndex`, no `aria-label`, and no keyboard handler (`onKeyDown`). Users navigating by keyboard cannot click into a session from the review queue.

The action buttons (approve/deny/skip) do have proper button elements with `aria-label` values and `data-testid` attributes, so those are accessible. But the navigation action (clicking to open the session) is keyboard-inaccessible.

The keyboard navigation hook `useReviewQueueNavigation` exists (`ReviewQueuePanel.tsx` line 88-95) and handles arrow key navigation, but it only fires `onSessionClick` — which navigates to the session rather than focusing the item for local keyboard interaction.

**Recommended fix**: Add `role="button"`, `tabIndex={0}`, and `onKeyDown` (Enter/Space) to `div.itemClickable`. Alternatively, restructure the item as a `<article>` or `<li>` with a visually styled link for the navigation action.

---

### C4 - Dialogs spawned from session card are not true modals

**Category**: Accessibility (POUR: Perceivable, Operable)
**Heuristic**: Error Prevention (Nielsen #5)
**File**: `web-app/src/components/sessions/SessionCard.tsx` lines 346-381 (rename dialog), 382-406 (restart dialog)

The rename dialog and restart confirm dialog in `SessionCard.tsx` are built as positioned divs with `position: fixed` and `z-index: 1000`, but they:
1. Do not trap keyboard focus. Users can Tab past the dialog and interact with the page behind it.
2. The rename dialog has no `role="dialog"` and no `aria-modal="true"` (the checkpoint and fork dialogs at lines 408-449 do have these attributes — the rename and restart dialogs were not updated to match).
3. There is no `aria-labelledby` connecting the dialog to its heading.
4. Click-outside-to-dismiss is not implemented — the backdrop `onClick` propagates to the card's `handleCardClick` but the `e.stopPropagation()` on the inner dialog content prevents dismissal via backdrop click.

The checkpoint and fork dialogs (lines 407-518) are better implemented with `role="dialog"` and `aria-modal`, but still lack focus trapping.

**Recommended fix**: Add `role="dialog"`, `aria-modal="true"`, and `aria-labelledby` to all four dialog types. Implement focus trapping using a `useEffect` that captures Tab/Shift+Tab within the dialog. The restart and rename dialogs should be updated to match the checkpoint dialog pattern as a minimum, and all four should then gain focus trapping.

---

## High Priority Issues

### H1 - Session card information density creates triage overhead

**Category**: Usability, Visual Design
**Heuristic**: Aesthetic and Minimalist Design (Nielsen #8)
**File**: `web-app/src/components/sessions/SessionCard.tsx` lines 626-696

The session card body renders Program, Branch, Path, Working Dir, Repository, Pull Request, and Cloned Repo as labeled key-value rows. For a user triaging 15 sessions, this means reading through 4-7 rows of metadata per card before reaching the "Last Activity" timestamp at the bottom. The most decision-relevant information (when did this session last do something meaningful?) is in the footer, below timestamps for Created and Updated that are rarely useful during triage.

Compare to what a user actually needs when scanning: "Is this session stuck? What is it working on? Is there something I need to do?" None of those questions are answered by the Program row or the Working Dir row.

**Recommended fix**: Restructure card information hierarchy into three tiers. Tier 1 (always visible): session name, rich status, last activity time-ago. Tier 2 (one line of context): branch and/or a one-line terminal preview. Tier 3 (on-demand): path, program, working dir, GitHub details — collapsible or accessible via the Info tab. The "Last Activity" timestamp should move to the card header area, not be buried in the footer.

---

### H2 - Actions menu is hidden behind a toggle on mobile, but also visually noisy on desktop

**Category**: Usability, Interaction Design
**Heuristic**: Flexibility and Efficiency of Use (Nielsen #7)
**File**: `web-app/src/components/sessions/SessionCard.tsx` lines 722-832; `SessionCard.module.css` lines 496-558

On desktop, the actions (Pause, Resume, Rename, Restart, Checkpoint, Fork, New Workspace, Duplicate, Delete) are always visible in the footer as an 8-9 button row. This adds significant visual weight to every card, and the footer becomes the card's dominant visual element on cards with short metadata. On mobile, these are collapsed behind an "Actions" toggle.

The symmetry is inverted from user need: the operations used most frequently during active use (Resume, Pause) should be instantly accessible, while destructive or infrequent operations (Delete, Fork from Checkpoint) should be deprioritized or require confirmation.

**Recommended fix**: On desktop, surface only 1-2 contextually relevant primary actions directly (e.g., "Resume" when paused, nothing when running). Put the rest in a "..." overflow menu. This follows the established pattern from tools like GitHub Actions, Vercel, and Netlify dashboards where each row/card has one primary action visible and a menu for the rest. The current CSS for the mobile accordion is a good implementation — the same progressive disclosure principle should apply on desktop at a card level.

---

### H3 - Review queue filter and sort UI conflicts with the triage flow

**Category**: Usability, Interaction Design
**Heuristic**: Recognition Rather Than Recall (Nielsen #6)
**File**: `web-app/src/components/sessions/ReviewQueuePanel.tsx` lines 249-313

The review queue has two complete filter systems (priority filter buttons + reason filter buttons) with a rule that changing one clears the other (`handleFilterByPriority` resets reason, and vice versa). This is non-obvious behavior that will surprise users.

More fundamentally: a review queue in a triage context should default to linear work — address the most urgent item, advance, repeat. The dual filter system implies users will want to cherry-pick items by type, which may be valid occasionally but should not be the primary UI paradigm. The filter controls currently occupy roughly 30-40% of the panel height before any items are shown.

The "Avg age" and "Oldest" statistics in the header are useful signals but are displayed as small de-emphasized text that will be missed by users scanning the panel quickly.

**Recommended fix**: Move the filter controls into a collapsible section, collapsed by default. Expose only the most critical signal prominently: the count and a visual breakdown (e.g., "3 approvals, 1 error, 2 idle"). The statistics (oldest age) should be promoted if the oldest item is over a threshold (e.g., 5 minutes) as a "heads up" callout rather than a passive stat.

---

### H4 - ReviewQueueBadge uses emoji-only indicators that fail for color-blind users and screen readers

**Category**: Accessibility (POUR: Perceivable)
**Heuristic**: WCAG 1.4.1 Use of Color
**File**: `web-app/src/components/sessions/ReviewQueueBadge.tsx` lines 22-35

Priority levels are communicated through colored circle emojis: red circle for Urgent, yellow for High, blue for Medium, white for Low. This fails on two dimensions:
1. Red/green color blindness (deuteranopia) affects roughly 8% of male users. The red vs. yellow distinction for Urgent vs. High is particularly problematic.
2. In compact mode (line 115-124), the badge shows only the emoji with no text. The `aria-label` on the compact badge correctly encodes the text ("High priority: Approval Pending"), but sighted keyboard users and low-vision users relying on text zoom get only the emoji.

The `UNCOMMITTED_CHANGES` and `STALE` reasons are both mapped to `styles.reasonUnspecified` (lines 105-108), giving them no visual distinction from truly unclassified reasons.

**Recommended fix**: Add a text label to compact badges on larger viewports (emoji + label). Ensure priority is communicated by shape or text label, not color alone. Map `UNCOMMITTED_CHANGES` and `STALE` to distinct CSS classes with unique styling. Consider replacing colored circle emojis with icon components that pair shape + color (e.g., a triangle for Urgent, diamond for High).

---

### H5 - ApprovalCard does not show session name — requires context memory from user

**Category**: Usability
**Heuristic**: Recognition Rather Than Recall (Nielsen #6)
**File**: `web-app/src/components/sessions/ApprovalCard.tsx` lines 94-101

The `ApprovalCard` body shows `approval.sessionId` (the raw ID, not a human-readable name) when a sessionId is present. A user approving tools across multiple sessions must remember which raw ID corresponds to which session. In the `ReviewQueuePanel`, session name is shown correctly in `item.sessionName`. The `ApprovalCard`, which is rendered inside `SessionDetail`, is the component most likely to be seen without that surrounding context.

**Recommended fix**: Pass the session title (not ID) to `ApprovalCard` for display. The component's prop interface only has `PendingApprovalProto` — add an optional `sessionTitle` prop and display it in place of or alongside the raw ID.

---

### H6 - Modal-for-session is 60vh at default — terminal is cramped

**Category**: Usability
**Heuristic**: Match Between System and the Real World (Nielsen #2)
**File**: `web-app/src/app/page.module.css` lines 52-55

The session detail modal is constrained to `height: 60vh` and `max-height: 60vh` by default. A fullscreen terminal emulator being presented at 60% viewport height means approximately 40% of the visible screen is unused modal backdrop. Users who want to read terminal output must immediately click the fullscreen button. The comment in the CSS notes this was "further reduced for narrow windows with dev tools," which is a developer-centric concern that has become the production default.

This forces an unnecessary step: open session, realize terminal is tiny, find and click the fullscreen button, then do the work. The fullscreen button is labeled with Unicode symbols (`⊗` for exit, `⛶` for enter) that have inconsistent rendering across operating systems and provide no text fallback.

**Recommended fix**: Increase the default modal height to 80vh or higher. Consider auto-entering fullscreen when the "terminal" tab is the initial tab (this logic already exists on tab switch in `SessionDetail.tsx` line 120 but not on initial open). Replace or supplement the fullscreen toggle symbols with text labels ("Fullscreen" / "Exit Fullscreen") or universally supported icon equivalents.

---

## Medium Priority Issues

### M1 - Tags and category are redundant organizational axes with unclear distinction

**Category**: Usability
**Heuristic**: Consistency and Standards (Nielsen #4)
**File**: `web-app/src/components/sessions/SessionCard.tsx` lines 591-611; `SessionList.tsx` lines 388-417

The session card renders both a `category` field and a `tags` array. The category is a single value displayed as a small badge; tags are displayed as blue pills. Both appear in the filter dropdowns in `SessionList.tsx`. From a user perspective, there is no UI explanation of what category vs. tag means, when to use one vs. the other, or why both exist. The requirements.md and CLAUDE.md documentation mention auto-migration of categories to tags, but this migration is not visible or explained in the UI.

The "Edit Tags" / "Add Tags" button renders on every session card at all times, adding to the visual noise discussed in H1.

**Recommended fix**: Decide on one organizational axis for the triage experience. If tags are the migration target, display only tags in the card (removing the separate category badge) and provide a single filter control. If both must coexist, add a tooltip or help text explaining the difference. Move the "Edit Tags" button into the actions menu rather than displaying it as a permanent footer element.

---

### M2 - Search does not include program name; filter defaults all show on load regardless of relevance

**Category**: Usability
**File**: `web-app/src/components/sessions/SessionList.tsx` lines 174-213

The search filter matches title, path, branch, category, and tags but not `session.program`. A user searching "claude" to find Claude Code sessions versus Aider sessions would get no results. The filter dropdowns (status, category, tag, grouping, sort field, sort direction) are always visible in the filter controls section but are only hidden on mobile behind a toggle. On desktop with few sessions, these controls dominate the layout even when most are unused.

**Recommended fix**: Add `session.program` to the search filter match criteria. On desktop, consider whether the full filter row is needed at all times, or whether search alone plus a "Filter" button (showing a dot when active, which is already implemented) would serve better.

---

### M3 - The "Review Queue" and sessions list are entirely separate views with no visual linkage

**Category**: Usability, Information Architecture
**Heuristic**: Match Between System and the Real World (Nielsen #2)
**File**: `web-app/src/components/layout/Header.tsx` lines 85-91; `web-app/src/app/page.tsx`

The Review Queue is a separate navigation destination. Sessions needing attention in the queue are not visually distinguishable on the Sessions list page. A user on the Sessions list sees a session card and status chip, but has no visual indication that the same session is in the review queue with a priority. The `reviewItem` prop on `SessionCard` exists and would show a `ReviewQueueBadge` on the card (lines 566-573), but the Sessions list (`SessionList.tsx`) never fetches or passes `reviewItem` data to cards.

This means the review-queue-aware session card display is fully wired but never exercised by the main sessions list. The visual linkage between "this session is in the review queue" and the session card is dead code.

**Recommended fix**: In `SessionList.tsx`, consume the `ReviewQueueContext` and pass matching `ReviewItem` objects to each `SessionCard`. This is a low-effort change that activates existing UI already built in `SessionCard`. The `reviewItem` display at lines 612-623 of `SessionCard.tsx` would then surface directly on sessions that need attention.

---

### M4 - Focus management is absent when modals open and close

**Category**: Accessibility (POUR: Operable)
**Heuristic**: WCAG 2.4.3 Focus Order
**File**: `web-app/src/app/page.tsx` lines 446-466

When the session detail modal opens (`setSelectedSession(session)`), focus is not moved into the modal. When the modal closes (`closeSession()`), focus is not returned to the card that was clicked to open it. A keyboard or screen reader user loses their place in the page on both transitions.

The same issue applies to the session creation wizard modal and the keyboard shortcuts help modal (lines 469-532).

**Recommended fix**: Use `useRef` on the modal container and call `.focus()` on it when it becomes visible. Store a ref to the triggering element before opening, and restore focus to it on close. The modal should have `tabIndex={-1}` to be programmatically focusable. This is a standard accessibility pattern well documented in WCAG's modal dialog techniques.

---

### M5 - "Stale" time-ago display has no reference anchor

**Category**: Usability
**Heuristic**: Visibility of System Status (Nielsen #1)
**File**: `web-app/src/components/sessions/SessionCard.tsx` lines 706-720

"Last Activity" is shown as a relative time ("5m ago", "2h ago"). This is good for immediate recency judgment, but when the value is something like "3d ago," a user does not know if that is expected (paused session) or alarming (running session that has not produced output for 3 days). The `formatDate` function is also used for "Created" and "Updated" timestamps — these show absolute dates, but the "Last Activity" field only shows relative time with no tooltip showing the absolute time.

There is a `title="Last terminal activity"` attribute, but no actual timestamp value in the title — only the static label string.

**Recommended fix**: Add the ISO timestamp string to the `title` attribute of the Last Activity `<time>` element (the existing `dateTime` attribute is correct, but the visible title should show the absolute time). Consider adding contextual coloring: if a "Running" session has Last Activity > 10 minutes ago, surface a warning indicator.

---

### M6 - Session card dialogs render outside the portal — z-index conflicts are likely

**Category**: Usability
**File**: `web-app/src/components/sessions/SessionCard.tsx` lines 338-518

All inline dialogs (rename, restart confirm, checkpoint, fork) are rendered as children of the `SessionCard` component using `position: fixed` with `z-index: 1000`. The outer session detail modal uses `z-index: 1000` too (from `page.module.css`). When a card dialog opens while the session detail modal is already open, there is a potential rendering conflict. More commonly, since card dialogs are rendered in-place in the cards list, they will also fight with the header (which has a sticky header with its own z-index stack).

**Recommended fix**: Render these dialogs through a portal (React `createPortal` to `document.body`). This is particularly important for the restart confirmation, which is a destructive action — if it is obscured by another element the user may miss the warning text.

---

### M7 - Visible indicator of loading state is limited to the initial session list load

**Category**: Usability
**Heuristic**: Visibility of System Status (Nielsen #1)
**File**: `web-app/src/app/page.tsx` lines 416-425; `SessionListSkeleton.tsx`

The `SessionListSkeleton` is shown during initial load. Once sessions are loaded and being watched via `autoWatch`, there is no persistent indicator that the sessions list is live/connected. If the WebSocket connection drops silently, the UI shows stale data with no indication that updates have stopped. Users with slow or intermittent connections have no feedback.

**Recommended fix**: Add a subtle connection status indicator (e.g., a green dot labeled "Live" or an icon in the header) that reflects whether the auto-watch connection is active. When disconnected, show a banner or indicator prompting retry. This is a common pattern in real-time dashboards (e.g., GitHub's live CI status, Vercel deployment logs).

---

### M8 - Restart confirm dialog has no accessible role on the confirmDialog element

**Category**: Accessibility (POUR: Robust)
**File**: `web-app/src/components/sessions/SessionCard.tsx` lines 382-406

The restart confirm dialog div has class `confirmDialog` but lacks `role="dialog"`, `aria-modal="true"`, and `aria-labelledby`. This is noted in C4 but worth calling out as a standalone item: the restart action is destructive and the confirmation dialog is safety-critical. Screen reader users would receive no indication that a dialog has appeared or that a destructive action is pending confirmation.

---

## Low Priority / Polish

### L1 - Emoji used as action button content without text alternative

**Category**: Accessibility (POUR: Perceivable)
**File**: `web-app/src/components/sessions/SessionCard.tsx` lines 741-831

Action buttons use emoji as the visible label (e.g., "▶️ Resume", "⏸️ Pause", "🗑️ Delete"). The emoji prefix has no `aria-hidden` attribute, meaning screen readers will read both the emoji description and the label text together. This is verbose but not broken — however, "🍴 Fork" being read as "fork and knife Fork" is confusing. The `aria-label` attributes on these buttons are correct and override the emoji reading, but only if they are properly associated.

**Recommended fix**: Wrap each emoji in a `<span aria-hidden="true">` so screen readers read only the button's `aria-label`. This is a minor polish item since the `aria-label` attributes are already present.

---

### L2 - "Group by: ..." option label in the select dropdown is wordy

**Category**: Usability
**File**: `web-app/src/components/sessions/SessionList.tsx` lines 436-443

The grouping strategy select renders each option as "Group by: Category", "Group by: Tag", etc. The label prefix "Group by:" is already present in the `aria-label="Group sessions by"` and is implied by the surrounding form control. The repeated prefix in every option makes the dropdown harder to scan quickly.

**Recommended fix**: Change option labels to just the strategy name ("Category", "Tag", "Branch", etc.). The `aria-label` on the select already provides context.

---

### L3 - The "?" floating help button duplicates functionality already in the header

**Category**: Usability
**File**: `web-app/src/app/page.tsx` lines 534-542; `web-app/src/components/layout/Header.tsx` line 162-171

There are two "?" buttons: one fixed floating button in `page.tsx` and one in the header actions in `Header.tsx`. Both fire the same keyboard shortcuts help modal. The header button is the canonical location for this action; the floating button adds visual noise and is redundant.

**Recommended fix**: Remove the floating help button from `page.tsx` and keep only the header button. If discoverability is a concern, add a tooltip to the header button.

---

### L4 - "Coming soon" items in keyboard shortcuts help modal

**Category**: Usability
**Heuristic**: Help and Documentation (Nielsen #10)
**File**: `web-app/src/app/page.tsx` lines 522-529

The keyboard shortcuts modal lists "/" for "Focus search (coming soon)" and arrow keys for "Navigate sessions (coming soon)". Advertising unimplemented features in the help dialog trains users to try actions that do not work, damaging trust in the keyboard shortcut system.

**Recommended fix**: Remove "coming soon" entries from the help modal. Document planned features in a roadmap or changelog, not in the functional help UI.

---

## Missing Patterns (things the planned design should add)

### MP1 - Terminal snapshot preview: design for the unrendered ANSI case

When terminal preview is added to session cards, ANSI escape sequences will be present in raw terminal output. The design must account for three states:
- **Rendered preview**: ANSI processed, shows colored terminal output. Best experience; requires a browser-side ANSI renderer like `ansi-to-html`.
- **Plain text fallback**: ANSI stripped, shows readable text. Acceptable and simpler.
- **Raw escape codes visible**: ANSI sequences shown as literal text (e.g., `^[[32m`). This is a failure state that will appear if preview is implemented naively. Design must explicitly prevent this.

The preview should also handle the empty/silent case: if the last N lines are all blank (terminal was cleared), show a "No recent output" placeholder rather than empty whitespace.

---

### MP2 - Rich status labels need a visual taxonomy

The planned "Waiting for your input", "Tests failing" labels need a consistent visual treatment that:
- Uses the existing status chip pattern (`SessionCard.module.css` lines 119-156) as the foundation
- Adds icons or shapes to communicate category (warning vs. info vs. action-required) without relying on color alone
- Is consistent with the `AttentionReason` badges in `ReviewQueueBadge.tsx` — the two systems should feel like one system, not two separate label schemes invented independently

Recommended: extend the existing status chip with an optional reason text below or beside it. Consider using the same color/shape vocabulary as `ReviewQueueBadge` so users build mental models that transfer between the sessions list and the review queue.

---

### MP3 - Non-interrupting approval drawer needs a clear escalation path

The planned side drawer for approvals should address what happens when an approval expires while the drawer is open. Currently, `ApprovalCard.tsx` has a live countdown timer (lines 20-33) that reaches zero, after which the Approve/Deny buttons disable and a Dismiss button appears. In a side drawer:
- If the drawer is closed and a timer expires, the item should be cleanly removed with an accessible live region announcement ("Approval expired for session X").
- If the drawer is open and the timer reaches zero, the expired card should visually indicate expiration without causing layout shift (the Dismiss button appearing in place of Approve/Deny currently changes the card's button row).
- Multiple simultaneous approvals across different sessions should be listed in priority order (urgency by time-to-expire, not by session name).

---

### MP4 - Review queue stability: snapshot-on-enter pattern

To solve the "queue jumps around mid-review" problem described in requirements.md, the planned design should adopt a snapshot pattern: when a user opens the review queue (or begins reviewing), the queue order is frozen as a snapshot. New items added while the user is reviewing should appear at the bottom with a visual indicator ("1 new item added — click to show") rather than being injected into the active list.

This is analogous to how Twitter/X handles new tweets when you are reading your feed: they accumulate behind a "Show new tweets" button rather than pushing your reading position.

The `ReviewQueuePanel.tsx` currently uses a live `items` array from `useReviewQueueContext`. The snapshot pattern would require storing a separate `reviewingItems` snapshot that is refreshed only explicitly (on page entry, after all items are cleared, or on manual refresh click).

---

### MP5 - Session card minimum touch target for the main click area

When terminal preview is added to the card, the clickable card area will expand. Ensure the primary card click target is at least 44x44 CSS pixels throughout and that the "Edit Tags" button and action buttons all meet WCAG 2.1 AA touch target requirements (44x44). The mobile CSS already adds `min-height: 44px` to action buttons (lines 543-549 of `SessionCard.module.css`), but the desktop CSS has `padding: 6px 16px` for `actionButton` which may fall short of 44px height depending on line height.

---

## Accessibility Checklist

| Check | Status | Notes |
|---|---|---|
| Skip-to-content link | Pass | Implemented in `globals.css` lines 414-432 |
| Keyboard navigation for cards | Partial | `SessionCard.tsx` has `onKeyDown` for Enter/Space; review queue clickable areas missing |
| Modal focus trapping | Fail | No focus trap in any modal; focus not restored on close |
| Dialog roles | Partial | Checkpoint and Fork dialogs have `role="dialog"`; Rename and Restart do not |
| `aria-modal` | Partial | Same as dialog roles — only checkpoint/fork have it |
| `aria-labelledby` on dialogs | Partial | Only checkpoint and fork dialogs linked to their headings |
| Color contrast (status chips) | Pass | Dark and light mode variants both appear to meet 4.5:1 for text; hardcoded hex values used in light mode may need audit |
| Color alone for meaning | Fail | `ReviewQueueBadge` uses colored circle emojis only in compact mode |
| `aria-live` regions | Partial | Review queue has a live region for count changes; no live region for approval arrivals or connection status |
| Touch target size | Partial | Mobile action buttons at 44px; desktop action buttons may be smaller |
| Reduced motion | Pass | `globals.css` line 434-444 disables animations; `ReviewQueuePanel.module.css` line 510-513 disables completion animation |
| Focus indicators | Unknown | Not auditable from code alone; depends on browser defaults since no custom `:focus-visible` styles are defined in the reviewed files |
| Form labels | Pass | Search input, filter selects all have `aria-label` |
| Image/icon alternatives | Partial | SVG icons in header use `aria-hidden`; emoji in action buttons should use `aria-hidden` |
| Semantic headings | Pass | `h2` for panel titles, `h3` for card titles, `h3` for item titles |
| Language attribute | Unknown | Not reviewed; should be present on `<html>` element in layout |

---

## Recommendations Summary Table

| ID | Issue | Component/File | Severity | Effort |
|---|---|---|---|---|
| C1 | Status labels are binary and untrustworthy | `SessionCard.tsx` | Critical | Medium |
| C2 | Approval panel interrupts active work | `SessionDetail.tsx`, `ApprovalPanel.tsx` | Critical | High |
| C3 | Review queue items not keyboard accessible | `ReviewQueuePanel.tsx` | Critical | Low |
| C4 | Card dialogs not true modals (no focus trap, missing ARIA) | `SessionCard.tsx` | Critical | Medium |
| H1 | Card information density creates triage overhead | `SessionCard.tsx` | High | High |
| H2 | Actions menu visually noisy on desktop | `SessionCard.tsx`, `SessionCard.module.css` | High | Medium |
| H3 | Review queue filter UI conflicts with triage flow | `ReviewQueuePanel.tsx` | High | Medium |
| H4 | ReviewQueueBadge fails for color-blind users | `ReviewQueueBadge.tsx` | High | Low |
| H5 | ApprovalCard shows raw session ID not name | `ApprovalCard.tsx` | High | Low |
| H6 | Modal height 60vh forces fullscreen click | `page.module.css` | High | Low |
| M1 | Tags and category are redundant with no UX distinction | `SessionCard.tsx`, `SessionList.tsx` | Medium | Medium |
| M2 | Search does not match program name | `SessionList.tsx` | Medium | Low |
| M3 | ReviewItem data not passed to SessionCard on main page | `SessionList.tsx`, `page.tsx` | Medium | Low |
| M4 | Focus management absent on modal open/close | `page.tsx` | Medium | Medium |
| M5 | Last Activity time-ago has no absolute time in title | `SessionCard.tsx` | Medium | Low |
| M6 | Card dialogs not rendered through portal | `SessionCard.tsx` | Medium | Medium |
| M7 | No live connection status indicator | `page.tsx`, `Header.tsx` | Medium | Medium |
| M8 | Restart confirm dialog missing accessible dialog role | `SessionCard.tsx` | Medium | Low |
| L1 | Emoji in action buttons not aria-hidden | `SessionCard.tsx` | Low | Low |
| L2 | "Group by:" label repeated in each dropdown option | `SessionList.tsx` | Low | Low |
| L3 | Floating "?" button duplicates header button | `page.tsx` | Low | Low |
| L4 | "Coming soon" items in keyboard shortcuts help | `page.tsx` | Low | Low |

**Quick wins** (low effort, immediate user value): C3, H4, H5, H6, M2, M3, M5, M8, L1-L4
