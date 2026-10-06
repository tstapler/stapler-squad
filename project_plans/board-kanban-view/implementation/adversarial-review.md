# Adversarial Review: board-kanban-view
**Date**: 2026-08-06
**Verdict**: RESOLVED (re-verified 2026-08-07 during sdd:4-validate — all 3 Blockers are
closed in the current plan.md; see per-item resolution notes below. Concerns/Minors are
follow-up quality notes, not implementation gates, and are left open below for the
implementer's awareness.)

## Blockers

- [x] **RESOLVED** (plan.md's `rowKey` Domain Glossary row and Task 3.1.1c: droppable/
  draggable ids are now scoped `` `${rowKey}:${column.key}` ``/`` `${rowKey}:${session.id}` ``,
  uniform from Phase 3 onward — plan.md cites this fix as closing this exact blocker.)
  **dnd-kit `id` collision across swimlane rows and Tag multi-membership.** Task 3.1.1c
  registers `useDroppable({ id: column.key })` and `useDraggable({ id: session.id })` using
  the raw `BoardColumnKey` string and raw session ID with no swimlane/row scoping. Phase 6
  then introduces multiple swimlane rows that each render the same 4 column keys (Task
  6.1.1a/b — `BoardSwimlane` explicitly states `BoardColumn` is "reused unmodified" so it
  "doesn't need to know it's inside a swimlane"), and the Tag-grouping Pattern Decision
  explicitly renders the *same* session's `BoardCard` in multiple rows simultaneously
  ("duplicate rendering by design, same session ID"). dnd-kit requires unique ids for every
  draggable/droppable within one `DndContext` — two rows both containing a "Running" column
  will register two droppables with the identical id `"running"`, and a two-tag session will
  register two draggables with the identical id `sess-1`. This isn't a hypothetical edge case;
  it's the direct, inevitable result of AC6 (swimlanes) and the plan's own Tag-multi-membership
  decision, and no task revisits the id scheme when Phase 6 lands. Recommendation: scope
  droppable/draggable ids by row, e.g. `` `${rowKey}:${column.key}` `` and
  `` `${rowKey}:${session.id}` ``, and resolve back to the real session/column in `onDragEnd`.

- [x] **RESOLVED** (plan.md Task 3.2.1c checks the `null`/falsy return value from
  `updateSession`/`resumeHibernatedSession` instead of a `catch`, reading the dispatched Redux
  error; further hardened during this validate pass to also classify `errorCode` so
  `network_error` is distinguishable from `rejected_by_server` — see Task 3.2.1c and
  Cross-Artifact Consistency Finding F.)
  **The plan's rejection-handling design assumes RPC wrapper functions reject on failure;
  they don't.** `web-app/src/lib/hooks/useSessionService.ts`'s `updateSession()`
  ([useSessionService.ts:301-337](/home/tstapler/Programming/stapler-squad/web-app/src/lib/hooks/useSessionService.ts#L301))
  and `resumeHibernatedSession()`
  ([useSessionService.ts:404-419](/home/tstapler/Programming/stapler-squad/web-app/src/lib/hooks/useSessionService.ts#L404))
  both catch the ConnectRPC error internally, dispatch it into Redux error state, and resolve
  to `null` — neither ever rejects its returned promise. But Task 3.2.1c's entire design is
  "Catch the `updateSession`/`resumeHibernatedSession` promise rejection in `onDragEnd`," and
  Story 3.2.1's second GWT example, the `DragOutcome.rejected_by_server` variant, and Task
  3.2.1e's test all depend on that catch firing. As written, it never will: a failed
  drag-triggered mutation resolves successfully with `null`, the optimistic move is never
  rolled back, no toast fires, no `aria-live` announcement fires — the exact "visible error
  indication" AC5 requires silently does not happen for genuine server-side rejections (only
  the *client-side* illegal-drag short-circuit in Task 3.2.1b, which never calls the RPC at
  all, actually works as designed). Since Task 4.1.1a routes `MoveToMenu` (the WCAG-mandated
  accessible fallback) through the same `attemptColumnMove` function, this also silently
  breaks error feedback for touch/keyboard users using the "official" accessible path.
  Recommendation: either check the `null`/falsy return value (and thread through the
  dispatched Redux error message) instead of relying on a rejected promise, or call the raw
  ConnectRPC client directly from `SessionBoard.tsx` to get real rejections.

- [x] **RESOLVED** (plan.md Task 3.1.1b wires `onDragCancel` on `DndContext`, resetting
  `inFlightDragSessionIds` to empty and producing `{type:"cancelled"}` — plan.md cites this fix
  as closing this exact blocker.)
  **`inFlightDragSessionId` freeze has no release path for a cancelled drag.** Task 3.1.1b
  wires only `onDragStart`/`onDragEnd` on `DndContext`. dnd-kit fires a separate
  `onDragCancel` callback (Escape key during a pointer drag, or an interrupted drag) that does
  **not** invoke `onDragEnd`. No task subscribes to `onDragCancel`, so `inFlightDragSessionId`
  (set in Task 3.1.1b, cleared only inside `onDragEnd` per Task 3.1.1d/3.2.2a) is never
  cleared for a cancelled drag — the card is permanently frozen against `watchSessions`
  updates (Story 3.2.2's own mitigation for pitfalls.md §2) until a full page reload. The
  `DragOutcome.cancelled` variant is declared in `dragOutcome.ts` (Task 3.2.1a) but no task
  ever produces or tests it, confirming this path was designed for but never wired.
  Recommendation: add `onDragCancel` to the `DndContext` wiring, clearing
  `inFlightDragSessionId` and emitting `{type: "cancelled"}`.

## Concerns

- [x] **RESOLVED** (Task 2.1.1e now implements per-column virtualization and states the
  `mergeRefs` ref-composition strategy with `useDroppable`.)
  **Load-bearing virtualization decision has no implementation task.** The Pattern
  Decisions table's "Column-level virtualization" row (plan.md:120) commits to reusing
  `react-virtuoso`/`@tanstack/react-virtual` per-`BoardColumn`, explicitly citing a real
  dnd-kit risk (pitfalls.md §7 — collision detection breaks against unmounted/off-screen cards
  in a virtualized cross-column setup) as the reason for *not* doing board-wide
  virtualization. But Task 2.1.1b's `BoardColumn` is specified as a plain
  `role="list"` scrollable container with no virtualizer — no Phase/Story/Task anywhere
  actually builds it. Either the decision is aspirational (drop it from the Pattern Decisions
  table, or add a Phase 2/7 task), or a real perf/DOM-node-count regression per pitfalls.md §5
  ships silently in v1.

- [ ] **WebSocket disconnection during a drag is unaddressed.** pitfalls.md §2 explicitly
  raises this as an open design question ("decide whether to block dragging while
  `ConnectionIndicator` shows disconnected"). The plan never mentions `ConnectionIndicator`,
  never decides this question, and doesn't even list it in Unresolved Questions. Combined with
  Blocker #2 above (failed mutations don't surface errors at all), a drag started during a
  reconnect gap has no way to tell the user anything went wrong.

- [ ] **Multi-select drag has no partial-failure semantics.** Task 6.3.1c fans out one
  `attemptColumnMove`-equivalent call per selected session ID with no batching, but never
  specifies what happens when some succeed and others fail (now the *common* case per Blocker
  #2, since failures resolve silently to `null` rather than rejecting). No aggregate
  `DragOutcome`, no per-ID toast, no rollback strategy, and Task 6.3.1d only tests the
  happy path (mutation fired once per selected ID) — never a mixed-outcome scenario.

- [ ] **`SessionBoard`'s prop interface duplicates `SessionListProps` with no shared type.**
  Task 2.1.1a requires `SessionBoard` to accept "the full mutation-callback prop surface
  `SessionList` currently accepts" — confirmed at
  [SessionList.tsx:57-97](/home/tstapler/Programming/stapler-squad/web-app/src/components/sessions/SessionList.tsx#L57),
  ~24 optional callbacks (`onDeleteSession`, `onCloneSession`, `onForkFromCheckpoint`,
  `onToggleAutonomousMode`, `onSteerAutonomousSession`, etc., most unrelated to board/column
  concerns) — purely so Task 5.1.1a can pass one shared props object to either view. Unlike
  the filter/sort/group pipeline (which the plan explicitly extracted specifically to avoid
  "zero business-logic duplication"), no shared prop type is extracted here. Every future
  addition to `SessionListProps` now needs a mirrored, easy-to-forget addition to
  `SessionBoard`'s own prop type or the "same props object" assumption in Task 5.1.1a silently
  breaks at the call site (TypeScript will only catch this if both interfaces are structurally
  compared, not if `SessionBoard`'s props type is independently declared and merely
  overlapping).

- [ ] **Filter-control parity beyond search/grouping/bulk-select is undecided.** The extracted
  `useFilteredGroupedSessions` hook (Task 1.2.1a) requires `selectedStatus`,
  `selectedCategory`, `selectedTag`, `hidePaused`, `showArchived`, `filterNeedsApproval`, and
  `reviewItemBySessionId` as inputs — the last of which `SessionList.tsx` builds itself via its
  own `useReviewQueueContext()` call
  ([SessionList.tsx:304-305](/home/tstapler/Programming/stapler-squad/web-app/src/components/sessions/SessionList.tsx#L304)).
  No Board task wires an equivalent `useReviewQueueContext()` call or exposes these filter
  controls in the Board header. Requirements Goal 4 only explicitly names grouping-axis,
  search, and bulk-select as things that must compose — it's unclear whether Board silently
  drops the other list filters (passing defaults) or is expected to duplicate that part of the
  header UI too. Worth an explicit scope call rather than leaving it to be discovered at
  Task 2.1.1a implementation time.

- [ ] **No documented decision on retry/idempotency for a timed-out drag mutation.**
  pitfalls.md §1 explicitly flags this ("check whether `UpdateSession` is idempotent... before
  wiring any automatic retry"). The plan implements no retry — the safe choice — but never
  states this as a decision anywhere (Pattern Decisions table, Unresolved Questions), leaving
  it ambiguous whether a future contributor should add one.

## Minors

- pitfalls.md §1's "stale toast" pitfall (rejection arriving after the user has navigated away
  from the board — filtered, switched to list view, or unmounted `SessionBoard`) is never
  translated into a task; no unmount-guard is mentioned for the toast/reconciliation logic in
  `attemptColumnMove`.
- The `board_drag_transition` analytics event (Observability Plan) isn't required by any AC —
  low-risk, but it's scope the plan adds on its own initiative rather than ties back to a
  requirement.
- ADR-001's React 19 fallback ("if a genuine React-19-specific defect surfaces during
  implementation, `@hello-pangea/dnd` is the documented fallback") never defines what would
  count as triggering the swap — no smoke-test or bail-out criterion is specified.
- `legalBoardTransitions["needs_review"] = []` combined with the "Running" special case being
  handled entirely outside the generic table (Task 3.3.1a) means `MoveToMenu`'s option list
  (Task 4.1.1b, "`legalBoardTransitions[currentColumn]` plus the needs_review→running special
  case") has two independent sources of truth for one column's legal moves — a plain function
  merging both would be less fragile than "the table, plus remember the one exception" spread
  across two components.
