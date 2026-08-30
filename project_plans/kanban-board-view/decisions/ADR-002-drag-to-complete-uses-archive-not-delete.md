# ADR-002: Drag-to-"Complete" calls `ArchiveSession`, not `DeleteSession`

**Date**: 2026-08-06
**Status**: Accepted
**Project**: kanban-board-view
**Deciders**: SDD Phase 3 planning

## Context

`requirements.md:25` constrains the board to reuse existing session-control RPCs —
"`pause_session`, `resume_session`, `stop_session` equivalents … no new backend session-state
RPC."

Phase 2 architecture research established that **no `StopSession` RPC exists**:
`proto/session/v1/session.proto` has no such method. The board's "Complete" column drop
therefore has no literal `stop_session` to call, and the plan must choose among the RPCs that
do exist (`web-app/src/lib/hooks/useSessionService.ts:93-111`):

```ts
updateSession: (id, updates: Partial<UpdateSessionRequest>) => Promise<Session | null>;  // :93
deleteSession: (id, force?: boolean) => Promise<boolean>;                                 // :94
pauseSession:  (id) => Promise<Session | null>;                                           // :97
resumeSession: (id, updates?) => Promise<Session | null>;                                 // :98
hibernateSession: (id) => Promise<Session | null>;                                        // :99
archiveSession:   (id) => Promise<boolean>;                                               // :110
unarchiveSession: (id) => Promise<boolean>;                                               // :111
```

`pauseSession`/`resumeSession` (`useSessionService.ts:364-383`) are thin wrappers over a single
`updateSession(id, { status })` call — there is no dedicated pause/resume RPC either.

UX research (`research/ux.md` §2) identifies the specific hazard: users arriving from
Trello/GitHub Projects read a drag to a "Done" column as *soft categorization with no side
effect on the underlying work*. In this app the columns are live session lifecycle states, so
a mismatch here is not a cosmetic issue — the wrong choice destroys a worktree.

## Decision

**Drag-to-"Complete" calls `archiveSession(id)`.**

Specifically:

- `BoardMoveIntent` for target column `"complete"` maps to `archiveSession`.
- `deleteSession` is **never** reachable from a board drag or from the "Move to…" menu. Deletion
  stays behind its existing explicit confirm modal (`web-app/src/app/page.tsx:478-514`) and the
  overflow menu, unchanged.
- `unarchiveSession(id)` is **not** wired as a drag-out-of-Complete action in v1. Dragging *out*
  of the Complete column is a rejected drop target (see ADR-003 / Story 3.1.4) — an archived,
  stopped session has no single legal one-hop transition back to Running, and inventing one
  would violate `requirements.md:46`'s "no new state model" scope.

## Rationale

1. **Reversibility.** `archiveSession` has a documented inverse, `unarchiveSession`
   (`useSessionService.ts:111`), reachable today from the list view's "show archived" toggle
   (`SessionList.tsx:579`). `deleteSession` has none. A drag gesture is far easier to trigger
   accidentally than a click-through-a-confirm-modal, so the action it performs must be
   undoable — this is the mitigation `research/ux.md` §2 calls for, satisfied structurally
   rather than by adding a confirm dialog that would defeat the point of dragging.
2. **Semantic fit.** A kanban "Done" column means "out of my active working set," which is
   exactly `archivedAt` — `SessionList.tsx:579` already hides archived sessions from the default
   list view. Archiving is the existing "remove from my board" primitive.
3. **Constraint compliance.** Both are existing RPCs; no proto change, no new enum value, no
   `make proto-gen`. Satisfies `requirements.md:25` and `:46`.
4. **Blast radius.** A misread drag that archives is recoverable in two clicks. A misread drag
   that deletes destroys a git worktree and its tmux session.

## Alternatives considered

| Alternative | Reason rejected |
|---|---|
| `deleteSession(id)` | Irreversible; destroys worktree + tmux session. Unacceptable behind a gesture as easy to trigger accidentally as a drag. Directly contradicts `research/ux.md` §2's Trello-metaphor-misread risk. |
| `deleteSession(id)` behind a drop-triggered confirm modal | Restores safety but defeats the purpose of drag as a shortcut, and adds a modal to a gesture flow — still ends at an irreversible action for a low-intent input. |
| `hibernateSession(id)` | Hibernate is a resource-reclamation state, not a completion state, and `resumeHibernatedSession` treats it as resumable — which is the *Paused* column's semantics, not Complete's. Hibernated sessions are routed to the Paused column instead (ADR-003). |
| Add a `StopSession` RPC | Explicitly out of scope (`requirements.md:25,46`). Would also require the 7-touchpoint proto/Go/TS change set for no user-visible gain over archive. |
| Make Complete a label-only column with no RPC | Contradicts `requirements.md:39` ("drag to trigger the corresponding state-change action") and leaves the board's most-used lane inert. |

## Consequences

- `SessionServiceContextValue` (`web-app/src/lib/contexts/SessionServiceContext.tsx:21-50`) does
  **not currently declare `archiveSession`**, even though the underlying `useSessionService`
  return object provides it (`useSessionService.ts:1108`) and the provider passes the whole
  object through (`SessionServiceContext.tsx:103`). The type must be widened — this is Task
  1.2.2c, a required prerequisite for the Complete drop target.
- The Complete column shows archived-and/or-stopped sessions. Because the list view hides
  archived sessions by default (`SessionList.tsx:579`), the board and list will legitimately
  disagree on visible session count while `showArchived` is off. The board's Complete column
  header count must therefore be labelled unambiguously ("Complete (N)") and the board must
  respect the same `showArchived` filter it shares with the list via `useSessionFilters` —
  meaning **with `showArchived` off, a session dragged to Complete disappears from the board
  entirely rather than landing visibly in the Complete column.** That transition must be
  announced ("Archived 'fix-login-bug' — hidden by the Archived filter") rather than silently
  vanishing; see Story 2.3.3.
- No proto, Go, or generated-binding changes. No `make proto-gen`.

## Verification

- `rg -n "deleteSession" web-app/src/components/sessions/SessionBoard*.tsx web-app/src/lib/hooks/useBoardMove.ts` returns no hits.
- Unit test `useBoardMove_should_CallArchiveSession_When_TargetColumnIsComplete` asserts
  `archiveSession` was called with the session id and `deleteSession` was not called at all.
