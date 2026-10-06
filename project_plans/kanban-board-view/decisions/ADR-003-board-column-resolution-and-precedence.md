# ADR-003: Board column resolution — one source of truth and an ordered precedence cascade

**Date**: 2026-08-06
**Status**: Accepted
**Project**: kanban-board-view
**Deciders**: SDD Phase 3 planning

## Context

`requirements.md:53` and `:72` flag this as an unresolved rabbit hole, and Phase 2 research
(`research/features.md` §3, `research/ux.md` §0) confirmed it is a real ambiguity in the data
model, not a hypothetical:

- `SESSION_STATUS_NEEDS_APPROVAL = 5` is **deprecated** in `proto/session/v1/types.proto`
  ("NeedsApproval is now a sub-status"), so a board column cannot be built from it.
- "Needs review" is signalled by **two independent, orthogonal subsystems**:
  - `useReviewQueueContext()` — server-computed `ReviewItem[]`, WebSocket-pushed via
    `WatchReviewQueue` (`web-app/src/lib/contexts/ReviewQueueContext.tsx:11`,
    `web-app/src/lib/hooks/useReviewQueue.ts:291-343`).
  - `useApprovalsContext()` — pending tool-use `PlainApproval[]`, RTK-Query polled every 5s
    (`web-app/src/lib/contexts/ApprovalsContext.tsx:40-42`), plus a `clearedSessions` set used
    for optimistic suppression after an approve/deny.
- A session can be `SESSION_STATUS_ACTIVE` **and** have a `ReviewItem` **and** have a
  `PlainApproval` simultaneously. The board slots each card into exactly one column.
- Nine `SessionStatus` enum values exist; only three map cleanly to the four columns
  (`ACTIVE`→Running, `PAUSED`→Paused, `STOPPED`→Complete). `CREATING`, `RESTORING`,
  `HIBERNATED`, `READY`, `LOADING`, and `UNSPECIFIED` have no obvious home, and a session that
  matches no column would silently vanish from the board — the exact class of bug already
  fixed once in this repo for the backlog board (BUG-037, see the comment at
  `web-app/src/components/backlog/BacklogBoard.tsx:13-24`).

## Decision

### 1. One source of truth for "needs review"

`useNeedsReviewSessionIds()` (`web-app/src/lib/hooks/useNeedsReviewSessionIds.ts`) returns a
`Set<string>` computed as:

```
(sessionIds appearing in useReviewQueueContext().items)
  ∪ (sessionIds appearing in useApprovalsContext().approvals)
  \ useApprovalsContext().clearedSessions
```

The **union**, not either source alone — `SessionCard`/`SessionRow` already show a review badge
from `reviewItemBySessionId` (`SessionList.tsx:304-308,1588`) while `ApprovalPanel`/
`ApprovalDrawer` gate on `useApprovals({ sessionId })`. Picking one source would make the board
disagree with a badge already visible on the same card in the list view — the specific
"divergent behavior" `requirements.md:18` prohibits. `clearedSessions` is subtracted so the
board honours the same optimistic-clear suppression the list already applies
(`SessionList.tsx:313-314`).

### 2. `resolveBoardColumn` — an ordered, total precedence cascade

`resolveBoardColumn(session: Session, needsReviewIds: Set<string>): BoardColumnKey`, a pure
function in `web-app/src/lib/board/boardColumns.ts`, evaluated **top to bottom, first match
wins**:

| # | Predicate | Column |
|---|---|---|
| 1 | `session.archivedAt` is set, **or** `session.status === SessionStatus.STOPPED` | `"complete"` |
| 2 | `needsReviewIds.has(session.id)` | `"needsReview"` |
| 3 | `session.status === SessionStatus.PAUSED` **or** `=== SessionStatus.HIBERNATED` | `"paused"` |
| 4 | *(all remaining: `ACTIVE`/`RUNNING`, `READY`, `LOADING`, `CREATING`, `RESTORING`, `UNSPECIFIED`)* | `"running"` |

Rule 4 is a **catch-all, not an enumeration** — the function is total over `SessionStatus` by
construction, so a future proto enum value lands in Running rather than making a session
invisible. `CREATING` and `RESTORING` are distinguished inside the Running column by the
existing "Starting…" sub-badge already rendered by `SessionCard.tsx:213`, not by a fifth column
(`requirements.md:47` fixes the column set at four).

### 3. Column order: Needs Review → Running → Paused → Complete

This **deviates from the literal left-to-right order in `requirements.md:38`** ("Running, Needs
Review, Paused, Complete").

### 4. Multi-membership guard for a `GroupingStrategy` swimlane axis

When the board's swimlane axis is switched away from the default status columns to a
`GroupingStrategy` (`requirements.md:40`), `GroupingStrategy.Tag` places one session in **every**
one of its tag groups simultaneously (`web-app/src/lib/grouping/strategies.ts:101-110`). Card
position then no longer implies exclusive state, so **drag-and-drop and the "Move to…" menu are
both disabled whenever the swimlane axis is not the default status axis** — the board is
read-only in `GroupingStrategy` mode. Dragging out of one tag column cannot coherently mean
"remove that tag" while the card remains in another tag column.

## Rationale

**Why terminal beats review (rule 1 over rule 2):** a stopped or archived session cannot act on
a pending approval. A leftover `ReviewItem` for a stopped session is stale by definition;
routing it to Needs Review would put an unactionable card in the one column whose entire purpose
is "things you can act on now."

**Why review beats paused and running (rule 2 over rules 3–4):** this is the feature's core
emotional job per `research/ux.md` §5 — an agent blocked on approval is burning wall-clock time
doing nothing until noticed. Surfacing it is the point. This also matches the precedent already
encoded in `groupSessions`' `statusOrder` table (`strategies.ts:179-192`), where Needs Approval
sorts ahead of Paused/Stopped.

**Why `HIBERNATED` → Paused, not Complete:** `resumeHibernatedSession` exists
(`useSessionService.ts:38` in the context type) and treats hibernated sessions as resumable,
unlike terminal `STOPPED` (`types.proto:337`, "cannot transition further"). Routing it to Paused
means the Paused column's drop-to-resume action is meaningful for those cards. `SessionCard.tsx`
currently reuses the *stopped-style* token for hibernated — that is a visual precedent, not a
lifecycle one, and lifecycle wins here. **Consequence**: the Paused column's resume handler must
branch on `status === HIBERNATED` → `resumeHibernatedSession(id)` vs. otherwise →
`resumeSession(id)`; a single `resumeSession` call for a hibernated card is a bug.

**Why Needs Review is leftmost (decision 3):** users open this board to answer "what needs me,"
not "what is executing." Running sessions require no action by definition. Leftmost/topmost is
where the F-pattern scan lands first (`research/ux.md` §2). The requirements doc's order reads
as prose enumeration rather than a designed ordering, and reversing it is a one-line change to
the `BOARD_COLUMNS` array in `web-app/src/lib/board/boardColumns.ts` if the user disagrees —
this deviation is logged in plan.md's Unresolved Questions for cheap veto before Story 2.1.2.

## Alternatives considered

| Alternative | Reason rejected |
|---|---|
| Source Needs Review from `ReviewQueueContext` only | Would omit sessions whose only pending signal is a tool-use `PlainApproval`, contradicting the review badge those sessions already show in list view. |
| Source from `ApprovalsContext` only | Symmetrically omits server-computed `ReviewItem`s (CI failures, escalations) that have no tool-use approval attached. |
| Source from `session.status === NEEDS_APPROVAL` | The enum value is deprecated and not reliably populated going forward (`types.proto:341`). |
| Running beats Needs Review (a session is "in progress" first) | Buries the actionable cards inside the largest column — directly defeats Success Metric #1. |
| A fifth "Starting" column for `CREATING`/`RESTORING` | `requirements.md:47` fixes the column set at four; the existing "Starting…" sub-badge already conveys it. |
| An "Unknown" fallback column shown only when non-empty | Adds a conditional column for a state (`UNSPECIFIED`) that is defensive-only and should never occur. The catch-all into Running is simpler and equally non-vanishing. |
| Allow drag on a `GroupingStrategy` swimlane axis | Incoherent under `Tag`'s multi-membership; would need a per-strategy drag semantics table that nothing in the requirements asks for. |

## Consequences

- `resolveBoardColumn` is pure and exhaustively unit-testable — one test case per
  `SessionStatus` value plus the four precedence-collision cases (Story 4.1.1).
- The board re-derives column membership on every render from `sessions` + `needsReviewIds`,
  both of which are already live (Redux `sessionsSlice` fed by `WatchSessions`; review queue fed
  by `WatchReviewQueue`). No second source of truth, so peer-driven moves animate for free
  (`research/architecture.md` §5).
- Because membership is reactive, a mid-drag `itemRemoved` push can change a dragged card's
  column under the pointer. Handled by snapshotting `resolveBoardColumn` at `onDragStart` and
  re-validating at `onDragEnd` (Story 3.1.5), not by freezing the data.
- The `GroupingStrategy` swimlane axis ships read-only. `requirements.md:40` asks for the axis
  switch, not for drag within it, so this is a scope clarification rather than a cut — but it is
  a behaviour difference a reviewer will notice, so the axis selector shows an inline
  "Read-only — moves are available on the Status axis" hint (Story 3.2.3).

## Verification

- `web-app/src/lib/board/__tests__/boardColumns.test.ts` covers all nine `SessionStatus` values,
  asserting no input returns `undefined`.
- `resolveBoardColumn_should_ReturnComplete_When_StoppedAndInReviewQueue` proves rule 1 beats
  rule 2.
- `resolveBoardColumn_should_ReturnNeedsReview_When_ActiveAndInApprovalsButNotCleared` and
  `resolveBoardColumn_should_ReturnRunning_When_ActiveAndInApprovalsAndCleared` prove the
  `clearedSessions` subtraction.
