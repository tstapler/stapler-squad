# Component Structure & Coupling — Research Findings

## Top 10 Largest Components by Line Count

| Rank | File | Lines | Category |
|---|---|---|---|
| 1 | `components/sessions/TerminalOutput.tsx` | 1,275 | Terminal rendering |
| 2 | `components/sessions/SessionCard.tsx` | 1,183 | Session list item |
| 3 | `components/sessions/Omnibar.tsx` | 1,146 | Creation/navigation modal |
| 4 | `components/sessions/SessionDetail.tsx` | 1,132 | Session detail view |
| 5 | `components/sessions/SessionWizard.tsx` | 912 | Session creation wizard |
| 6 | `components/sessions/SessionList.tsx` | 903 | Session list container |
| 7 | `components/sessions/ReviewQueuePanel.tsx` | 848 | Review queue |
| 8 | `components/sessions/OmnibarCreationPanel.tsx` | 725 | Creation form inside Omnibar |
| 9 | `components/sessions/FileTree.tsx` | 698 | File browser |
| 10 | `app/config/page.tsx` | 697 | Config page |

All top 7 are in `components/sessions/`. The `lib/` layer also contributes: `lib/hooks/useSessionService.ts` at 646 lines and `lib/terminal/StateApplicator.ts` at 677 lines.

## Data-Fetching vs Presentation Analysis

**SessionDetail.tsx (1,132 lines):** Props interface (lines 39–54) accepts 13 props, all of which are presentational/callback — no data-fetching in props. The component itself uses `useSessionActions` (which calls `useSessionService` internally) and `useAppSelector(selectAllSessions)` directly (line 29). It also manages `useEffect` for tab sync. This is a mixed-responsibility component: it handles local UI state (active tab, fullscreen flag, modal open states), reads from Redux, uses a service hook, and renders a full multi-tab layout with terminal, diff, VCS, logs, files, and info tabs — all in one file.

**SessionCard.tsx (1,183 lines):** Props interface (lines 86–110) accepts 22 props — 3 data props (`session`, `reviewItem`, `detectedStatus`) and 19 callback props. This is a pure presentation component receiving all data and actions as props. It manages only local UI state (confirm dialogs, tag editor open state, etc.). The excessive prop count (22) is the coupling smell — callers must know about all possible actions even when only a subset applies.

**Omnibar.tsx (1,146 lines):** Manages its own form state (`OmnibarFormState`, `OmnibarUIState` at lines 42–80), runs a complete path detection pipeline, and renders two modes (discovery and creation). The component is split across `Omnibar.tsx` (1,146 lines) + `OmnibarCreationPanel.tsx` (725 lines) — together 1,871 lines for one feature.

**ReviewQueuePanel.tsx (848 lines):** Fetches data via `useReviewQueueContext` and `useApprovalsContext` (lines 6, 212), then renders the full queue with filtering, sorting, pagination, approval actions, and one-shot prompting. This is a full data-fetching + business-logic + presentation component.

**SessionList.tsx (903 lines):** Consumes `useSessionServiceContext` (for sessions) and `useReviewQueueContext` (for queue items) and implements filtering, sorting, grouping, and bulk selection. Contains both data orchestration and render logic.

## Near-Duplicate Components

Three "nav badge" components follow an identical pattern — render a count badge using a single context hook:

- `ApprovalNavBadge.tsx` — calls `useApprovalsContext()`, renders approval count
- `ReviewQueueNavBadge.tsx` — calls `useReviewQueueContext()`, renders queue count
- `NotificationsNavBadge.tsx` — calls `useNotifications()`, renders unread count

All three are ~20 lines each with identical badge rendering logic. They differ only in which context they consume and what count field they read.

Two "panel" components for overlapping concerns:
- `ApprovalPanel.tsx` — calls `useApprovals({ sessionId })` directly, scoped per-session
- `ApprovalDrawer.tsx` — calls `useApprovals({})` (all sessions), rendered globally

Both render the same approval list UI with approve/deny buttons, duplicating ~200 lines of JSX.

## Components in `components/sessions/` Exceeding 200 Lines

All 7 top-ranked components above are in `components/sessions/` and all exceed 200 lines. Additional components in that directory exceeding 200 lines (not already listed):
- `ApprovalAnalyticsPanel.tsx` — 300+ lines (analytics charts + data)
- `DiffViewer.tsx` — 300+ lines (diff rendering)
- `VcsPanel.tsx` — 250+ lines (VCS status panel)
- `SessionWizard.tsx` — 912 lines (full creation wizard, parallel to Omnibar)

`SessionWizard.tsx` (912 lines) and `Omnibar.tsx` + `OmnibarCreationPanel.tsx` (1,871 lines combined) both implement session creation flows. It is unclear from static analysis whether they are intended to be parallel paths or if one should supersede the other.

## Layer Separation Assessment

The `eslint-plugin-boundaries` rules enforce that `sessions` components can only import from `providers`, `ui`, `shared`, `lib`, `gen`. This prevents `sessions` from importing `layout`, `history`, or `logs` — a reasonable constraint. However, within `sessions/` itself there is no enforced separation between data-fetching components and pure presentational ones. Large components like `SessionDetail` and `SessionList` freely mix context reads, Redux selects, service hook calls, and render logic.
