# Architecture Review: board-kanban-view
**Date**: 2026-08-06
**Verdict**: RESOLVED (re-verified 2026-08-07 during sdd:4-validate — both Blockers are closed
in the current plan.md, plus the StopByUser-ordering Concern below, patched during this pass
per pre-mortem.md P1 finding #1. Remaining Concerns/Nitpicks are follow-up quality notes, not
implementation gates.)

`/home/tstapler/Programming/stapler-squad/docs/adr/ADR-000-architecture-constitution.md` does
not exist in this repo, so no constitution section applies.

## Blockers

- [x] **RESOLVED** (Task 2.1.1e now implements per-column virtualization and states the
  `mergeRefs` ref-composition strategy for `useDroppable` + the virtualizer scroll ref — plan.md
  cites this fix as closing this exact blocker.)
  **Epic 2.1 / Pattern Decisions "Column-level virtualization" row** — The plan cites a
  concrete architectural decision ("reuse the same `react-virtuoso`/`@tanstack/react-virtual`
  approach `SessionList`'s row mode already uses, scoped per-`BoardColumn`") and justifies it
  by name-checking `pitfalls.md §5`'s DOM-node-count jank risk and `pitfalls.md §7`'s dnd-kit
  cross-column-collision-detection rough edge. But no Phase/Epic/Story/Task anywhere in the
  1168-line plan actually implements it — `grep -ni virtual project_plans/board-kanban-view/implementation/plan.md`
  returns only the glossary row and the Pattern Decisions table row, nothing else. Task
  2.1.1b's `BoardColumn` scaffold (`implementation/plan.md:555-562`) describes a plain
  `role="list"` card container with no virtualizer wiring, and no later task revisits it.
  Confirmed `react-virtuoso`/`@tanstack/react-virtual` are already project dependencies
  (`web-app/package.json:66,89`) and `SessionList.tsx` already imports both
  (`useVirtualizer`, `GroupedVirtuoso`), so the omission isn't "library not available" — it's
  a missing task. This is exactly the class of gap an architecture review before code exists
  is supposed to catch: retrofitting a virtualizer onto `BoardColumn`/`BoardCard` *after*
  `useDroppable`/`useDraggable` refs, drag-handle DOM structure, and column CSS already exist
  is materially more invasive than building it in from Epic 2.1. It also has a real ref
  conflict to resolve up front: `useDroppable`'s `setNodeRef` and a virtualizer's scroll
  container ref both need to attach to the same scrollable element inside `BoardColumn`, and
  the plan never states how those two refs compose (e.g. a `mergeRefs` helper).
  **Remediation**: add an explicit task under Epic 2.1 (e.g. Task 2.1.1e) that wires
  per-column virtualization using the same library/pattern `SessionList.tsx` uses, and states
  the ref-composition strategy for `useDroppable` + the virtualizer's scroll ref. If v1 is
  intentionally deferring virtualization (e.g. because realistic per-column card counts are
  low), say so explicitly and drop the Pattern Decision row rather than leaving a decision on
  record that Phase 8's "done" checklist has nothing to verify against.

- [x] **RESOLVED** (Domain Glossary's `inFlightDragSessionIds` is specced as
  `ReadonlySet<string>` from Phase 3 onward — plan.md cites this fix as closing this exact
  blocker. Note: pre-mortem.md P2 finding #2 flags that Task 3.2.2a's prose still needs to be
  read carefully by the implementer to build a per-ID snapshot map, not a singular snapshot —
  tracked there, not re-blocking here since the underlying state shape is already plural.)
  **Epic 6.3 / Story 3.2.2 interaction (`inFlightDragSessionId` vs. multi-select drag)** —
  `inFlightDragSessionId` (`implementation/plan.md:75`) is specced as a single
  `string | null`, and Story 3.2.2 (`implementation/plan.md:749-772`) uses it to freeze the
  *one* dragged session's rendered column against a racing `watchSessions` push during a drag.
  Task 6.3.1c (`implementation/plan.md:1056-1061`), added three phases later, fans a single
  drag gesture out into N `attemptColumnMove` calls — one per selected session ID — when the
  dragged card is part of the active selection. Because the freeze mechanism only ever tracks
  one ID, N-1 of the N sessions being moved in a multi-select drag have **no** protection
  against exactly the race Story 3.2.2 was built to close: a `watchSessions` push for any of
  the other selected sessions arriving mid-flight will immediately re-bucket that card out from
  under the in-progress bulk operation, silently reintroducing the bug Phase 3 fixed, and it
  will only ever manifest once Phase 6 ships — i.e. this is a real regression introduced by a
  later phase against an earlier phase's own fix, not a hypothetical.
  **Remediation**: change `inFlightDragSessionId: string | null` to
  `inFlightDragSessionIds: ReadonlySet<string>` (or equivalent) before Phase 3 is implemented,
  so Phase 6's multi-select fan-out is additive to an already-plural mechanism instead of
  requiring a breaking rename/refactor of Phase 3's state shape mid-plan.

## Concerns

- [ ] **`SessionBoard.tsx` becomes the new "does everything" component the plan explicitly
  warned against for the rejected Option 2** — Step 0.5 rejects "single component with a
  render-mode switch" specifically because it would bolt "DnD sensors, column/swimlane
  derivation, and drop-rejection animation onto an already large component" (lines 24-33).
  The chosen Option 1 extracts `useFilteredGroupedSessions` out of `SessionList.tsx`, but every
  other piece of board logic — `DndContext`/sensor wiring (3.1), optimistic move + rejection
  reconciliation (3.2), the needs-review special case (3.3), `attemptColumnMove` (4.1),
  ARIA live-region + focus management (4.2), swimlane derivation (6.1), and cross-column
  selection + multi-select fan-out (6.3) — all live as local state/closures directly inside
  `SessionBoard.tsx`. This just relocates the "one type, too many responsibilities" smell from
  `SessionList.tsx` into a new file rather than avoiding it, and it undercuts the plan's own
  precedent: `useFilteredGroupedSessions` was correctly identified as hook-extractable because
  it has ≥2 real call sites and is independently testable; the drag-orchestration logic is
  equally independently-testable (it doesn't need a mounted DOM to reason about "given this
  drag event and this session list, what RPC call and what outcome") but isn't extracted.
  **Remediation**: extract a `useBoardDragAndDrop(sessions, groupingStrategy, ...)` hook
  (parallel treatment to `useFilteredGroupedSessions`) owning `inFlightDragSessionIds`,
  the optimistic-override map, `attemptColumnMove`, and `DragOutcome` production, returning
  `{ onDragEnd, attemptColumnMove, dragOutcome, ... }` to a `SessionBoard` that composes hooks
  and renders. Testable via `renderHook`, not full-component mount.

- [x] **RESOLVED** (Task 1.3.1b adds the status-aware `isLegalBoardDragForSession` wrapper,
  which `onDragEnd` and `MoveToMenu` call instead of the raw column-only
  `isLegalBoardDrag` — plan.md cites this fix as closing this exact blocker.)
  **Story 1.3.1 / `legalBoardTransitions` conflates board-column granularity with
  backend-status granularity, producing false "legal" client-side pre-checks for transient
  sessions** — `getBoardColumnKey` explicitly buckets `ACTIVE`, `CREATING`, and `RESTORING`
  into the single `"running"` column (Column Membership section, `implementation/plan.md:88`,
  reaffirmed at line 98: "`CREATING`/`RESTORING` render inside Running"). But the backend state
  machine (`session/state_machine.go:38-56`, confirmed by reading the file) gives these three
  statuses **different** real outbound edges: `Active → Paused|Stopped|Hibernated`,
  `Creating → Active|Stopped` (no `Paused`), and `Restoring` has **zero** entries as a `From`
  value anywhere in `transitionDefs` (it isn't part of the "5-state model" the file's own
  header comment describes). `legalBoardTransitions["running"] = ["paused", "complete"]`
  (`implementation/plan.md:450`) is flat across the whole column, so the client-side pre-check
  Story 1.3.1 exists specifically to avoid firing doomed RPCs
  ("so `SessionBoard`'s drop handler can reject illegal drags client-side before firing any
  RPC", line 423) will incorrectly say "yes, legal" for dragging any `Creating` or `Restoring`
  session to "Paused," and the drag will only fail after a round trip via Story 3.2.1's
  server-rejection path. This isn't a rare edge case — `Creating`/`Restoring` are common,
  visible states the plan itself gives a dedicated "transient/loading chip" for, i.e. sessions
  users are likely to see and interact with on the board.
  **Remediation**: make the legality pre-check status-aware, not just column-aware — either
  key `legalBoardTransitions` by concrete `SessionStatus` (mapping many-to-one into
  `BoardColumnKey` only for rendering, not for legality), or add a guard in
  `statusForColumnMove`/`onDragEnd` that special-cases `CREATING`/`RESTORING` as "no legal
  outbound board drag" the same way `"needs_review"`/`"complete"` are already special-cased.

- [ ] **`statusForColumnMove` uses a `null` return as an implicit "call a different RPC
  entirely" signal (Task 3.1.1a, `implementation/plan.md:637-649`)** — the function's return
  type is `SessionStatus | null`, where `null` doesn't mean "no status," it means "the caller
  must remember to branch to `resumeHibernatedSession` instead." This is exactly the kind of
  sentinel the type system should be encoding as a discriminated union instead of a value that
  looks like "no-op" but actually means "different code path required" — a future edit that
  adds a fourth special case (or a refactor that forgets the null check) fails silently rather
  than at compile time. Combined with the `ResolveApproval` special-case living as an ad hoc
  `if` in `onDragEnd` (Task 3.3.1a) rather than in this same function, there are effectively
  three different action-selection mechanisms (`statusForColumnMove`'s null sentinel, the
  needs-review `if` in `onDragEnd`, and the default `updateSession` fallthrough) doing the same
  conceptual job — "decide which RPC resolves this drag" — via three different shapes.
  **Remediation**: replace `statusForColumnMove` with a function returning a `BoardMoveAction`
  discriminated union (e.g. `{kind:"updateStatus", status} | {kind:"resumeHibernated"} |
  {kind:"resolveApproval", decision:"approve"} | {kind:"unsupported"}`), and have `onDragEnd`
  switch exhaustively over it — the same "exhaustive switch is a compile-time guard against a
  missed case" discipline `.claude/rules/feature-testing-registry.md`'s `OmnibarAction` union
  already uses elsewhere in this codebase for the identical shape of problem.

- [ ] **No `BatchDragOutcome` for multi-select drag partial failure (Task 6.3.1c)** — Task
  6.3.1c fans a single drag into N independent `attemptColumnMove` calls, but `DragOutcome`
  (Task 3.2.1a) is a single-item type with no representation for "3 of 5 succeeded, 2 failed
  (and why)." The plan never specifies the toast/reconciliation UX for a partially-successful
  bulk move, leaving it to be improvised at implementation time — exactly the kind of decision
  this phase (`sdd:3-plan`/architecture review) exists to pin down before code is written.
  **Remediation**: add a `BatchDragOutcome { succeeded: string[]; failed: {sessionId: string;
  reason: string}[] }` type and specify (even briefly) what the toast says and whether
  succeeded cards stay moved while failed ones snap back (independent per-ID reconciliation,
  matching the existing single-item optimistic-override-clear pattern) versus an all-or-nothing
  rollback.

- [ ] **`useFilteredGroupedSessions`'s parameter surface is 13 loose scalars, not a value
  object (Task 1.2.1a, `implementation/plan.md:395-404`)** — the hook is specced to take
  `sessions, searchQuery, selectedStatus, selectedCategory, selectedTag, hidePaused,
  showArchived, filterNeedsApproval, reviewItemBySessionId, sortField, sortDir, costById,
  groupingStrategy` as (apparently) independent parameters. This is the extraction's own new
  call-boundary, and a wide loose-scalar signature at a boundary two components must both call
  identically is fragile: adding a 14th filter later requires updating both `SessionList` and
  `SessionBoard` call sites in lockstep with no compiler help distinguishing "forgot to thread
  the new filter through" from "intentionally omitted." **Remediation**: bundle the filter-ish
  parameters into a single `SessionFilterState` value object/type that both components
  construct and pass as one argument — the shared "second real call site" that justified
  extracting the hook in the first place is the same argument for bundling its parameters.

- [x] **RESOLVED** (the Domain Glossary's `DragOutcome` row now points to Task 3.2.1a as the
  canonical definition and matches its shape exactly, with a note flagging the prior mismatch
  was corrected.)
  **Domain Glossary's `DragOutcome` definition doesn't match its own implementation task's
  definition** — the glossary (`implementation/plan.md:72`) defines `DragOutcome` as
  `{type:"moved"} | {type:"rejected", reason:string} | {type:"network_error"} |
  {type:"cancelled"}`, but Task 3.2.1a (`implementation/plan.md:709-719`) defines a materially
  different shape splitting `"rejected"` into `"rejected_illegal"` and `"rejected_by_server"`
  with different fields. The glossary is meant to be the authoritative reference an
  implementer skims before writing code; as written it will point them at the wrong shape.
  **Remediation**: update the glossary entry to match Task 3.2.1a's shape (the more detailed,
  presumably-correct one) before implementation starts.

- [x] **RESOLVED 2026-08-07** (patched during sdd:4-validate per pre-mortem.md P1 finding #1:
  Task 0.1.1a's `stopByUserLocked` now calls `canTransitionLocked(s, Stopped)` and returns
  early *before* `stopControllerLocked`/`KillSession()`/`gitManager.Remove()`; Task 0.1.1d adds
  `TestStopByUser_should_RejectTransition_When_SessionIsRestoring` asserting the tmux session
  and worktree survive a rejected call.)
  **`StopByUser` performs destructive side effects (kill tmux, remove worktree) before
  the state-machine legality check, so "no partial state change is persisted" (Story 0.1.1's
  AC) is true only for the `status` field, not for the session's actual runtime/filesystem
  state** — Task 0.1.1a's `stopByUserLocked` (`implementation/plan.md:264-311`) calls
  `stopControllerLocked`, `KillSession()`, and `gitManager.Remove()` *before* the final
  `transitionToLocked(s, ctx, Stopped)` call that can reject the transition. The illegal-case
  AC test (`Restoring→Stopped`) will therefore kill the tmux session and delete the worktree
  for a `Restoring` session and only then discover the transition itself is illegal and return
  an error — the tmux process and worktree are gone regardless. This mirrors `pauseLocked`'s
  existing (pre-existing, not introduced by this plan) ordering, so it isn't a novel pattern,
  but `Stopped` is a more consequential/harder-to-recover-from terminal state than `Paused`,
  and the plan's own AC language ("no partial state change is persisted") reads as a stronger
  guarantee than the code actually provides. **Remediation**: either check
  `session/state_machine.go`'s edge legality for `(i.Status, Stopped)` *before* performing any
  destructive side effect in `stopByUserLocked`, or soften the AC wording so reviewers don't
  read it as "fully atomic, no side effects survive a rejected transition."

## Nitpicks

- `SessionBoard.tsx`'s prop surface is described as "the full mutation-callback prop surface
  `SessionList` currently accepts" (Task 2.1.1a) purely by convention/comment, with no shared
  TypeScript type constraining both components to the same shape — consider a shared
  `SessionListPaneViewProps` interface both `SessionList` and `SessionBoard` implement, so a
  future prop added to one and not the other is a compile error, not a silent drift caught only
  by Task 5.1.1a's "same props object" wiring at the call site.
- Task 3.3.1a's `grep -n "resolveApproval" web-app/src/lib/hooks/*.ts` and Task 4.1.1b's
  `grep -rl "SessionActionsOverflow..."` "confirm exact name at implementation time" pattern
  appears repeatedly for names the plan is fairly confident about — fine for a plan, but worth
  resolving these to concrete names before Phase 5's implementer hits the same open question
  independently multiple times across Phases 3, 4, and 6.
- The Dependency Visualization diagram doesn't show Phase 6 (Swimlanes/Search/Bulk-select)
  depending on Phase 3's `attemptColumnMove` extraction, even though Task 6.3.1c explicitly
  reuses/extends it — the diagram shows Phase 6 depending only on "hook + board shell" (Phase 1
  + Phase 2), which understates the Phase 3/4 coupling the BLOCKER above is about.
