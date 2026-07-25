# Backlog UX Feature Research

## 1. UX Review of the Backlog Feature

### `/backlog/page.tsx` — List Page

**Strengths:**
- Table view with sortable columns (title, status, priority, AC, updatedAt) is clear.
- Status and priority filter chips are ergonomic.
- Resizable detail pane (pointer drag) is a good pattern for dense information.
- `VaguenessPromptModal` is a thoughtful triage UX touch.

**Gaps and issues:**
- **No deletion pathway whatsoever.** The table has no overflow menu, no row actions, and no bulk actions. Users cannot delete backlog items or their linked sessions from this view at all. This is the most critical gap (US-2).
- **No "Clear completed" affordance.** Done/archived items just accumulate with no way to clean up from the list view.
- **"List" tab is a `<button>` not a link.** It uses `aria-current="page"` but does not update the URL to `/backlog?view=list`, so the two tab states are not deep-linkable.
- **Filter state is not reflected in the URL.** Filtering by status reloads the list client-side but filters are lost on refresh or deep link.
- **No real-time update.** The page uses `listBacklogItems` (one-shot polling when filters change), not `WatchBacklogItems`. Status changes made from another tab require a manual page reload to appear.
- **No count badge anywhere.** The active/pending count (US-4) is entirely missing.
- **FooterNudge** ("no items in_progress") only appears when nothing is in progress — this is a reasonable nudge but there's no equivalent for items stuck in `ready` or `review`.

### `BacklogItemCard.tsx` — Board Card

**Strengths:**
- Single primary action per status maps cleanly to the lifecycle flow.
- `getActionSpec` makes the status→action logic centralized and testable.

**Gaps:**
- **No overflow/secondary actions.** Cards have exactly one action button. There is no way to delete, archive, or "View sessions" alongside the primary action. The `onAction` callback accepts arbitrary action strings but the card only surfaces one at a time.
- **"Refining…" and "Done ✓" states are effectively dead ends.** Their action buttons are disabled/inert but there is no secondary path (archive, delete, view sessions).
- **No linked session count indicator.** `in_progress` shows "View Session" but only if `linkedSessions.length > 0`; if there are zero linked sessions the button is just disabled with no explanation.
- **Card does not show status badge color on the card surface itself.** Status is implied only by the action button label; the board grouping usually makes this redundant but in the list view the card has no color signal.

### `BacklogItemDetail.tsx` — Detail Pane

**Strengths:**
- Rich content: triage result, gate verdict, AC criteria, linked sessions, workflow timeline, notes inline edit.
- Session monitor for active sessions is well-placed.
- Error banner with dismiss is accessible (`role="alert"`).
- Triage timer gives visible progress feedback.

**Gaps:**
- **No deletion action.** There is no "Delete item" or "Delete sessions" button in any status. US-2 requires this.
- **"Override → Done" is labeled with a danger class but is the only path out of `review` for some items.** The name is alarming for a legitimate completion flow. Consider renaming to "Force Complete."
- **`archive` action is gated to `done` status.** Items in `done` can be archived but items in `archived` status have no "unarchive" action.
- **Action discoverability:** Multiple action buttons are shown in a flat horizontal list under a "Actions" section — at `review` status there are three (Override, Re-review, and the implicit GateVerdictBox approve/reopen). This could overlap visually with `GateVerdictBox` for the same action.
- **Linked sessions list shows raw session UUIDs.** Session names or a short truncated ID would be more readable.
- **No batch delete pathway.** Users navigating item-by-item to delete sessions will have a very tedious experience.

### `SessionCard.tsx` — Session List Card

**Strengths:**
- Rich badge row: GitHub PR state, autonomous mode, memory pressure, sub-status chip, workflow badge — well-established pattern for adding new badges.
- `workflowBadge` at line 589–599 is a direct precedent for a `backlogBadge`: same structure (role="img", data-testid, title attribute), same placement in the `badges` div.

**Badge placement for US-3:**
- The `badges` div (lines 444–600) is where backlog status badges belong. It already contains: external, GitHub, review queue, session status, rate limit, StatusBadge, SubStatusChip, memory, autonomous, workflow.
- A `BacklogSessionBadge` should go **after** the workflow badge (last in the block), to avoid competing with primary status signals.
- The badge needs to be a link (`<a>`) rather than `<span>` to satisfy the "clicking navigates to `/backlog?item=<id>`" requirement (US-3). The existing pattern wraps clickable badges as `<button>` (autonomous) — use `<a>` here since it's navigation.
- Information density concern: at `in_progress` + with PR + autonomous mode, the badge row can already have 5+ chips. The backlog badge should be compact: just "Backlog: In Progress" with status color. Adding AC progress fraction (`2/5 ✓`) in the compact badge would push it over the noise threshold — reserve that for hover tooltip.

**Category display:** `session.category` is already rendered at line 602–604 as a plain `<span className={category}>`. When US-1 assigns `"Backlog"` as the category, this will appear. No card changes needed for category display — it's already wired.

**SessionActionsOverflow — Deletion UX:**
- `onDelete` triggers `setIsDeleteConfirmOpen(true)` → portal modal with "This action cannot be undone" warning. This is the correct pattern to reuse for "Delete sessions" in the backlog item overflow.
- For the backlog item deletion flow (US-2), the `BacklogItemDetail` and `BacklogItemCard` need overflow menus that follow the same pattern: overflow button (···) → `isDeleteConfirmOpen` state → portal dialog.
- The backlog detail pane currently has no overflow menu at all. Adding one next to the "Edit" button in the header is the most natural location.

### `Navigation.tsx`

**Gap:** The "Backlog" nav link (lines 27–29) is a plain `AppLink` with no badge. `ReviewQueueNavBadge` is the direct precedent for adding a count badge (US-4): it wraps `NavBadge` with a context hook. The implementation path is:
1. Create `BacklogNavBadge` using `useBacklogService().listBacklogItems` (polling) or a future `WatchBacklogItems` context.
2. Count items in `ready | in_progress | review` status.
3. Wrap the "Backlog" nav label: `Backlog <BacklogNavBadge inline />`.

The `Navigation.tsx` currently imports no session-specific contexts, so a new `BacklogNavBadgeContext` (or reuse `useBacklogService` directly with a short poll interval) will be needed.

---

## 2. Session Card Structure and Badge Placement

The session card renders in three logical zones:

```
┌─────────────────────────────────────────────┐
│ HEADER                                       │
│  titleRow: [title] [badges...]               │  ← badge row (line 444)
│  category (line 602)                         │
│  tagsContainer (line 605)                    │
│  reviewInfo (line 627)                       │
│  lastActivityRow (line 639)                  │
├─────────────────────────────────────────────┤
│ BODY                                         │
│  info rows: Program, Branch, Path, ...       │
│  diffStats                                   │
│  terminal snapshot (collapsible)             │
├─────────────────────────────────────────────┤
│ FOOTER                                       │
│  timestamps | SessionActionsOverflow         │
└─────────────────────────────────────────────┘
```

**Badge row order (existing):**
1. External badge (if external session)
2. GitHubBadge (PR number, state, checks)
3. ReviewQueueBadge (if review item)
4. Status badge (Active/Paused/etc.)
5. Rate limit state
6. StatusBadge (detected terminal status)
7. SubStatusChip (proto sub_status)
8. Memory badge
9. Autonomous badge / outcome badge
10. Workflow badge

**Proposed backlog badge placement:** After position 10 (workflow badge), before the closing `</div>`. This puts it last in the secondary metadata group, consistent with how workflow badge sits last for non-backlog sessions.

**Clutter risk:** The badge row will be long for sessions that have: GitHub PR + autonomous + memory pressure + backlog badge. Mitigation: backlog badge should be compact (`Backlog · In Progress`) and only shown when the session has a `backlogItemId` in its tags or metadata. Avoid including AC fraction in the inline badge; put it in a `title` tooltip.

---

## 3. Deletion UX — Existing Pattern and Backlog Extension

### Current delete flow (SessionActionsOverflow)

1. User clicks ··· overflow button.
2. `isDeleteConfirmOpen` state set to `true`.
3. Portal renders a modal overlay (`confirmDialog`) with `role="dialog" aria-modal="true"`.
4. Modal shows session title, warning: "This action cannot be undone."
5. `dangerButton` confirm → calls `onDelete()` → sets `isDeleting` → clears on completion or shows `deleteError`.
6. Escape key closes modal.
7. Focus trapped via `useFocusTrap` hook.

### What's needed for US-2 (backlog item deletion)

**Per-item deletion (from `/backlog` list or detail pane):**
- Add an overflow menu (`···`) to `BacklogItemDetail` header (next to "Edit" button).
- Menu item: "Delete sessions" — opens a confirmation modal with variant text: "This will delete all [N] session(s) linked to this item. The item itself will remain." (or optionally delete the item too as a separate action).
- Alternatively add "Delete item + sessions" as a second option.
- Backend already has `DeleteSession` RPC; the frontend just needs to call it for each `linkedSessions` entry.
- A new `DeleteBacklogItemSessions(item_id)` RPC (per requirements) would be cleaner than N sequential calls.

**Bulk "Clear completed" (from `/backlog` page):**
- A sticky action bar or header button that appears when `items.some(i => i.status === "done" || i.status === "archived")`.
- Pattern: similar to `BulkActions.tsx` in the sessions list — shows "Clear completed (N)" button.
- Clicking opens a confirmation modal: "Delete all sessions for [N] done/archived items. This cannot be undone."
- After confirmation, items are archived and their linked sessions deleted.

**Auto-deletion on done transition (US-2 third bullet):**
- This is a backend concern — `backlog_service.go` should auto-delete triage and review sessions when an item transitions to `done`. Work sessions remain.
- No new frontend affordance needed for this behavior.

---

## 4. Notification/Toast Pattern

### Existing pattern

`NotificationToast` in `/web-app/src/components/ui/NotificationToast.tsx` is the full-featured notification component:

- **Animated entry/exit** via `isVisible` / `isExiting` state + CSS transitions.
- **Auto-close timer:** centralized in `lib/notification-policy.ts` via `toastAutoCloseMs()`.
- **Auto-minimize timer:** collapses to compact pill after `toastAutoMinimizeMs()` ms.
- **`role="alert"` / `aria-live`:** polite for info, assertive for approval_needed.
- **Action buttons:** Approve/Deny/View Session/Dismiss.
- **Stacking:** managed by `NotificationPanel` (not shown here; stacks multiple toasts).

### Usage for new backlog actions (US-2, US-5)

For "Delete sessions" success/error and "Create backlog item" success (US-5), the toast pattern to use is:

```ts
// Success
pushNotification({
  notificationType: "info",
  title: "Backlog item created",
  message: `"${title}" created in idea status.`,
  onView: () => router.push(`/backlog?item=${newItem.id}`),
});

// Error
pushNotification({
  notificationType: "error",
  title: "Delete failed",
  message: err.message,
});
```

The `pushNotification` function is provided by `NotificationContext` (or equivalent context — check `NotificationPanel.tsx` for the exact API). `NotificationData` type lives in `lib/types/notification.ts`.

**Key constraint:** Backlog-related toasts should use `notificationType: "info"` (not `approval_needed`) so they auto-close and don't demand user attention. The "link to created item" should be in the `onView` callback, which renders a "View Session" button that can be repurposed as "View Item."

---

## 5. Multi-Level Grouping — Art of the Possible

### Current `GroupedSessions` type

```ts
interface GroupedSessions {
  groupKey: string;
  displayName: string;
  sessions: Session[];
}
```

`groupSessions()` returns `GroupedSessions[]` — a flat array of groups. There is no nesting support.

### Changes needed for two-level (Project → secondary) grouping

**Option A: New `GroupedSessionsNested` type (recommended)**

```ts
interface SubGroup {
  groupKey: string;
  displayName: string;
  sessions: Session[];
}

interface GroupedSessionsNested {
  groupKey: string;
  displayName: string;
  subGroups: SubGroup[];
  sessions: Session[];  // direct members (sessions with no secondary key)
}
```

Add a new `groupSessionsNested(sessions, primary, secondary)` function alongside the existing `groupSessions`. The existing function stays unchanged (backward compatible). The `SessionList` component can check whether the result is `GroupedSessions[]` or `GroupedSessionsNested[]` based on the selected strategy.

**Option B: Flat encoding with compound keys**

`groupKey` becomes `"Project A::tag:frontend"`. The `SessionList` component interprets `::` to render nested headers. This avoids a new type but is brittle and harder to collapse at the project level.

**Option A is preferred** because:
- Collapsing a project group (to hide all sub-groups) requires the parent to know its children.
- The `SessionList` rendering loop needs to be redesigned regardless; a proper type makes it explicit.

### `strategies.ts` changes

The current `groupSessions` and `GroupingStrategy` enum stay unchanged. Add:

1. **New enum values:**
   ```ts
   export enum CompositeGroupingStrategy {
     ProjectThenTag = "project_then_tag",
     ProjectThenStatus = "project_then_status",
   }
   ```
   (Or add to `GroupingStrategy` with a naming convention: `ProjectTag`, `ProjectStatus`.)

2. **New function:**
   ```ts
   export function groupSessionsNested(
     sessions: Session[],
     primary: GroupingStrategy,
     secondary: GroupingStrategy,
     options?: GroupSessionsOptions,
   ): GroupedSessionsNested[]
   ```
   Internally calls the existing single-dimension helpers per project group.

3. **Updated `cycleGroupingStrategy`:** Add composite strategies to the cycle, or make them a separate cycle that the UI can invoke.

### `SessionList` rendering impact

The current `SessionList` maps over `GroupedSessions[]` and renders a flat group header + session cards. Two-level grouping requires:
- Outer loop: project group header + collapse toggle.
- Inner loop: secondary group header + session cards.
- Collapse state needs to be per-group-key (`Map<string, boolean>`), extended to support nested `parentKey::childKey` pairs.

This is a medium-complexity change to `SessionList.tsx` but the data model change in `strategies.ts` is the load-bearing part.

### Graceful degradation

Sessions with no `projectId` go into a "No Project" primary group. Within "No Project", secondary grouping still works (e.g., tags within "No Project"). This satisfies the requirement: "degrades gracefully: sessions with no project assignment appear under 'No Project → [secondary group]'."

---

## Summary of Key Findings

1. **The backlog page has zero deletion affordances.** There is no overflow menu on rows, no bulk action bar, and no "delete sessions" in the item detail. This is the most critical gap to address (US-2). The session card's `SessionActionsOverflow` delete pattern (portal modal, focus trap, danger button, error inline) is the exact pattern to replicate.

2. **The `BacklogItemBadge` component already exists** (`BacklogItemBadge.tsx`) and is styled with per-status colors, but it is not wired into `SessionCard`. Placing it in the `badges` div after the `workflowBadge` (as a navigation link `<a href="/backlog?item=...">`) is the minimal change needed for US-3. Badge must be compact to avoid over-crowding.

3. **Navigation nav badge infrastructure is fully reusable.** `NavBadge` + `ReviewQueueNavBadge` is the exact pattern for `BacklogNavBadge`. The only new piece is a lightweight context/hook that counts active backlog items (status in `ready | in_progress | review`) with a poll interval or WatchBacklogItems stream — and wiring it into the "Backlog" nav item in `Navigation.tsx`.

4. **Multi-level grouping requires a new `GroupedSessionsNested` type** and a parallel `groupSessionsNested()` function in `strategies.ts`, plus a redesigned rendering loop in `SessionList`. The existing `GroupingStrategy` enum and `groupSessions` function stay unchanged, preserving all 9 current strategies.
