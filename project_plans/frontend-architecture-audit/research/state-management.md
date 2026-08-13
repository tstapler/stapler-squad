# State Management Patterns — Research Findings

## Context Inventory

`web-app/src/lib/contexts/` contains 10 files (9 context providers + 1 test):

| Context | State held | Provider location | Key consumers |
|---|---|---|---|
| `SessionServiceContext.tsx` | `sessions[]`, `loading`, `error`, `connectionState`, 15+ CRUD methods | App layout (global) | CockpitShell, SessionList, pane components |
| `OmnibarContext.tsx` | `isOpen`, `initialMode` | App layout (global) | Header, BottomNav, keyboard shortcuts |
| `ApprovalsContext.tsx` | `approvals[]`, `loading`, `error`, approve/deny/refresh | App layout (global) | ApprovalNavBadge, ReviewQueuePanel |
| `ReviewQueueContext.tsx` | Full `UseReviewQueueReturn` (queue, items, stats, filters, pagination) | App layout (global) | ReviewQueuePanel, ReviewQueueNavBadge |
| `NavigationContext.tsx` | `isDrawerOpen`, open/close/toggle + localStorage persistence | App layout (global) | Header, DrawerNav, BottomNav |
| `NotificationContext.tsx` | `notifications[]`, `notificationHistory[]`, `isPanelOpen`, 12 methods | App layout (global) | Header, NotificationPanel, SessionServiceContext |
| `ThemeContext.tsx` | Active theme name, theme-switching | App layout (global) | ThemePicker, root layout |
| `AuthContext.tsx` | `authEnabled`, `authenticated`, `loading` | App layout (global) | All data-fetching hooks via `enabled` guard |
| `CockpitActionsContext.ts` | 20 callback props (sessions[], loading, error, all session actions) | CockpitShell (local) | SessionList, SessionCard (via prop drilling bridge) |
| `SessionVcsContext.tsx` | VCS status + diff state for one session | SessionDetail (scoped) | VcsPanel, FilesTab, DiffViewer |

## Overlapping Responsibilities

**Sessions data appears in three places simultaneously:**

1. `SessionServiceContext` — holds `sessions[]` + all CRUD methods, backed by Redux (`sessionsSlice`)
2. `CockpitActionsContext` — re-exposes `sessions[]`, `loading`, `error` alongside the action callbacks
3. `OmnibarContext` — creates its own separate `useSessionService({ enabled })` instance solely for the `createSession` method (line 45, `OmnibarContext.tsx`)

This means `useSessionService` is instantiated at minimum **three times** in the app simultaneously: once in `GlobalSessionServiceProvider`, once in `OmnibarProvider`, and once in `useSessionActions` (which is called from CockpitShell). All three connect to the same Redux store but each runs its own hook initialization.

**Approvals split across hook and context:**

- `useApprovals` hook (`lib/hooks/useApprovals.ts`, 103 lines) — used directly by `ApprovalDrawer` and `ApprovalPanel`
- `ApprovalsContext` (`lib/contexts/ApprovalsContext.tsx`, 67 lines) — wraps the same RTK Query call, used by `ApprovalNavBadge` and `ReviewQueuePanel`

Both call `useGetApprovalsQuery` independently with `pollingInterval: 5000`. This starts **two independent 5-second polling timers** when both consumers are mounted simultaneously.

## Prop Drilling Observations

`CockpitActionsContext` was introduced to solve prop drilling from `CockpitShell` into `SessionList` → `SessionCard`, but it has grown to 20 callback slots — itself becoming a prop-drilling proxy object rather than a true context. The interface in `CockpitActionsContext.ts` at line 6 lists: `onDeleteSession`, `onPauseSession`, `onResumeSession`, `onDirectResumeSession`, `onCloneSession`, `onNewWorkspaceSession`, `onRenameSession`, `onRestartSession`, `onUpdateTags`, `onNewSession`, `onCreateCheckpoint`, `onListCheckpoints`, `onForkFromCheckpoint`, `onRunOneShot`, `onSetRateLimitEnabled`, `onClearConversationState`, `onListSessions` — all 20 passed as a single opaque bag.

`ReviewQueuePanel` (848 lines) consumes both `useApprovalsContext` (for approve/deny) and `useReviewQueueContext` simultaneously, then passes session-level callbacks down into child rows. This component is the junction point where three contexts converge.

## Boundary Violations

`OmnibarContext.tsx` imports `Omnibar` component (a `sessions` layer component) and renders it inside the provider. The `eslint-plugin-boundaries` config (`lib` → `sessions` is allowed), so this does not currently fail CI, but it means the context and component are tightly coupled — the context cannot be tested without the full Omnibar component tree.

## Summary

- 9 global context providers all mount at the app layout root, creating a deep nesting of providers; only `SessionVcsContext` is scoped to a sub-tree
- Sessions state is duplicated across `SessionServiceContext`, `CockpitActionsContext`, and a third `useSessionService` instance inside `OmnibarContext`, all backed by the same Redux slice
- Approvals polling runs twice simultaneously (`useApprovals` hook + `ApprovalsContext`) when both `ApprovalPanel` and `ApprovalNavBadge` are mounted
