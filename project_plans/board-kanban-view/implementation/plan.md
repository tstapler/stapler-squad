# Implementation Plan: board-kanban-view

**Feature**: Add a Board (kanban) view as a toggleable alternative to the existing session
List view — status-based columns (Running / Needs Review / Paused / Complete), drag-and-drop
status mutation via the existing `UpdateSession` RPC plus one new backend transition branch,
full parity with List view's search/bulk-select/grouping-strategy features, and a WCAG
2.1 SC 2.5.7-compliant non-drag fallback.
**Date**: 2026-08-06
**Status**: Ready for implementation
**ADRs**: ADR-001-adopt-dnd-kit-dependency, ADR-002-derive-needs-review-column-from-substatus

---

## Step 0.5 — Creative Pass: Integration Approaches Considered

1. **(Chosen) Extract shared hook + two sibling render components.** Pull
   `SessionList.tsx:530-656`'s filter→sort→group pipeline into a new
   `useFilteredGroupedSessions` hook; `SessionList` and a new `SessionBoard` both call it.
   *Strength*: zero business-logic duplication, and each component stays focused on its own
   rendering concern (scrolling rows/cards vs. draggable columns). *Weakness*: requires a
   careful, behavior-preserving refactor of an already-1600-line file before any new board
   code is written — the riskiest single step in the whole plan if done carelessly.

2. **Single component with a render-mode switch** (extend `SessionList`'s existing
   `viewMode?: "card"|"row"` union with a third `"board"` value). *Strength*: no extraction
   step, single source of truth for state, fastest to a working prototype. *Weakness*: bolts
   DnD sensors, column/swimlane derivation, and drop-rejection animation onto an already
   large component that has zero overlap with those concerns in its two existing modes —
   directly the "single component doing too much" smell called out in
   `.claude/rules/interface-pollution-checklist.md`'s spirit (not that checklist's literal
   interface-pollution smells, but the same "one type, too many unrelated responsibilities"
   failure mode), and makes `SessionList.tsx` harder to review going forward for reasons
   unrelated to list-view changes.

3. **Fully separate board module with its own data fetching** (new route/page, own
   `useSessionServiceContext()` subscription, no shared hook). *Strength*: maximum isolation,
   zero risk of destabilizing `SessionList.tsx`. *Weakness*: directly violates requirements.md
   Goal 4 ("compose with existing session-list features rather than duplicating or forking
   them") — search, filters, and bulk-select would silently drift out of sync with list view
   over time, and it would open a second `watchSessions()` stream, contradicting the
   `GlobalSessionServiceProvider` singleton pattern this app relies on.

**Decision: Option 1** (extracted hook + sibling components), per architecture.md's own
recommendation and because it is the only option that satisfies both the "parity not
similarity" bar in the ACs and the single-live-stream constraint. Options 2 and 3 are recorded
above and in the Pattern Decisions table below as rejected alternatives.

---

## System Type

Frontend feature (new page-level view mode + drag-and-drop interaction layer) with one
scoped backend gap-fix (a new state-machine-transition branch exposed through the existing
`UpdateSession` RPC). No new RPCs, no new database/storage schema, no new proto messages —
one new proto-adjacent Go method (`Instance.StopByUser`) and no `.proto` file changes at all
(the `status` field already exists on `UpdateSessionRequest`).

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `SessionViewMode` | `"list" \| "board"` — the new page-level List/Board toggle state. | **Not** the same as `SessionList`'s existing `viewMode?: "card"\|"row"` density prop (`SessionList.tsx:96`) — different concept, different name, to avoid the collision stack.md explicitly flagged. |
| `BoardColumnKey` | `"running" \| "needs_review" \| "paused" \| "complete"` — the 4 fixed status-based board columns. | Not a 1:1 mirror of `SessionStatus`; `"paused"` covers 2 statuses, `"needs_review"` covers zero statuses (it's a `SubStatus`-derived bucket). |
| `getBoardColumnKey(session)` | Pure function: `Session → BoardColumnKey`. | New file `web-app/src/lib/board/columns.ts`. See §"Column membership" below for the exact branching logic. |
| `legalBoardTransitions` | `Record<BoardColumnKey, BoardColumnKey[]>` — client-side pre-check of which column-to-column drags are attempted-legal, checked *before* firing any RPC. | New file `web-app/src/lib/board/transitions.ts`. Derived from (but not identical to) `session/state_machine.go`'s `transitionDefs`, because board columns aren't 1:1 with `SessionStatus`. |
| `SessionBoard` | Top-level board component; owns `DndContext`, column/swimlane derivation, and renders one `BoardColumn` per group. | New file `web-app/src/components/sessions/SessionBoard.tsx`, sibling to `SessionList.tsx`. |
| `BoardColumn` | A single column: header (label + count badge), drop-target wiring, scrollable/virtualized card list. | New file `web-app/src/components/sessions/BoardColumn.tsx`. |
| `BoardCard` | Thin wrapper around `SessionCard` adding a drag handle and the `MoveToMenu` fallback trigger — does not fork `SessionCard`'s rendering. | New file `web-app/src/components/sessions/BoardCard.tsx`. |
| `useFilteredGroupedSessions` | Extracted hook: `(sessions, filterState, groupingStrategy) → { filteredSessions, sortedSessions, groupedSessions, filteredSessionIds }`. | New file `web-app/src/lib/hooks/useFilteredGroupedSessions.ts`, extracted from `SessionList.tsx:530-656`. Both `SessionList` and `SessionBoard` call it — the single "second real call site" that justifies the extraction per the interface-pollution checklist's concrete-type-first rule. |
| `DragOutcome` | Discriminated union: `{type:"moved"} \| {type:"rejected", reason:string} \| {type:"network_error"} \| {type:"cancelled"}` — result of one completed drag or `MoveToMenu` action. | New type in `web-app/src/lib/board/dragOutcome.ts`. Drives the toast/snap-back UI (pitfalls.md §1's "distinguish rejected from changed-elsewhere"). |
| `MoveToMenu` | The non-drag fallback control on `BoardCard` (WCAG SC 2.5.7 requirement) — also the keyboard-operable path, per ux.md §3's "one implementation serves both". | New file `web-app/src/components/sessions/MoveToMenu.tsx`, modeled on the existing overflow-menu pattern already used in `web-app/src/components/sessions/` for per-card actions. |
| `ApprovalResolution` | The distinct drag-out-of-Needs-Review action path: calls `ResolveApproval` (approve/deny), not `updateSession`. | Implemented inline in `SessionBoard.tsx`'s drop handler — no new RPC, `ResolveApproval` already exists (`proto/session/v1/session.proto:122`). |
| `inFlightDragSessionId` | Local `SessionBoard` state: the session ID currently mid-drag (or mid-`MoveToMenu`-action). Suppresses `watchSessions`-driven column reassignment for that ID until the drag/action resolves. | Addresses pitfalls.md §2's "live push racing an in-progress drag." |
| `swimlaneGroupingStrategy` | The existing `GroupingStrategy` value (from `web-app/src/lib/grouping/strategies.ts`) selected as the **row** axis when board view is not using the default status-only layout. | Reuses `groupSessions()` unmodified; status columns are always present as the column axis (see Pattern Decision #7). |
| `ViewModeStorageKey` | `` `ws-${currentWorkspaceId}.stapler-squad-session-view-mode` `` — workspace-scoped localStorage key for AC9. | `currentWorkspaceId` sourced from `useDatabases().currentId` (`web-app/src/lib/hooks/useDatabase.ts`), mirroring the *shape* of `SessionList`'s existing `pane-${pane.id}.` prefix convention (`makeStorageKeys`, `SessionList.tsx:238-242`) but keyed by workspace, not pane. |
| `StopByUser` | New `Instance` method, `session/instance.go`, mirroring `pauseLocked`'s cleanup (stop controller, commit-if-dirty, kill tmux, remove worktree if present) but calling `transitionToLocked(ctx, Stopped)` instead of `Paused`. | Backs the new `→ Stopped` branch in `UpdateSession`. See Pattern Decision #5. |
| `classifyStopErr` | Go error classifier for the new `UpdateSession` `→ Stopped` branch, structurally identical to `classifyPauseResumeErr` (`server/services/session_service.go:1690-1698`). | New function, same file. |

---

## Column Membership (`getBoardColumnKey`)

```
if session.status === ACTIVE && (session.subStatus === NEEDS_APPROVAL || session.subStatus === INPUT_REQUIRED):
    → "needs_review"
elif session.status === ACTIVE || session.status === CREATING || session.status === RESTORING:
    → "running"
elif session.status === PAUSED || session.status === HIBERNATED:
    → "paused"
elif session.status === STOPPED:
    → "complete"
else: // UNSPECIFIED, defensive fallback
    → "running"
```

`CREATING`/`RESTORING` render inside "Running" with the existing transient/loading chip
`SessionCard` already renders for those statuses — no separate transient column (per
architecture.md's recommendation, avoids inventing a 5th/6th column the requirements'
Non-Goals implicitly reject).

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Drag-and-drop library | `@dnd-kit/core` + `@dnd-kit/sortable` + `@dnd-kit/utilities` | stack.md, build-vs-buy.md | `react-beautiful-dnd` (archived Aug 2025, no React 19), `@hello-pangea/dnd` (single-maintainer bus factor), `@atlaskit/pragmatic-drag-and-drop` (smaller bundle but no built-in keyboard sensor — more integration work for the same v1 scope), hand-rolled HTML5 DnD (no keyboard path at all, would have to reimplement touch+keyboard+autoscroll from scratch) | Built-in `KeyboardSensor` + live-region announcements satisfy AC10's keyboard path essentially for free; React 19 runs today despite the "not yet advertised as tested" gap (loose `>=16.8.0` peer range, confirmed via `npm view`) |
| Board/List integration shape | Extract `useFilteredGroupedSessions` hook; `SessionBoard.tsx` as a new sibling component | architecture.md §3, Step 0.5 above | Third `SessionList.viewMode` value (Option 2); fully separate module with its own data fetch (Option 3) | Sibling+hook is the only option satisfying both "parity not similarity" (ACs 7/8) and the single-live-stream constraint, without bloating an already ~1600-line file with unrelated DnD concerns |
| "Needs Review" column semantics | Derived from `subStatus` (Option (a) in features.md §2), populated only by cards that already satisfy the condition; dragging **out** triggers `ApprovalResolution` (`ResolveApproval` RPC), not `updateSession`; dragging **into** it via raw drop is disallowed (rejected client-side, no legal edge exists) | features.md §2, architecture.md §2, pitfalls.md summary #1 | Treating it as an ordinary status column with a normal `UpdateSession` drag both ways | No `SessionStatus` value corresponds to "enter Needs Review" — it's a `SubStatus` layered on `ACTIVE`; forcing it through `UpdateSession` would either no-op silently or require inventing new backend semantics the Non-Goals explicitly reject |
| "Paused" column membership | `PAUSED ∪ HIBERNATED` merged into one visual column, distinguished by `SessionCard`'s existing status chip | architecture.md §1, features.md §1 table | A 5th dedicated "Hibernated" column; silently excluding Hibernated sessions from the board | A 5th column contradicts the requirements' 4-column framing and Non-Goals ("no free-form add-a-column builder" implies not growing the fixed set either); excluding sessions from the board silently violates ux.md's explicit anti-cap guidance ("an arbitrary cap would silently hide sessions") |
| "Complete" column drop (new backend capability) | New `Instance.StopByUser()` (Go) mirroring `pauseLocked`'s cleanup (stop controller, commit-if-dirty, kill tmux, remove worktree) but landing in `Stopped` via `transitionToLocked`; gated on the existing `CanPause` permission (no new permission field) | architecture.md §2 gap #1, session/instance.go's `Pause()` as the mirrored pattern | Reusing `DeleteSession` (fully destroys — no `Stopped→Active` resume path, contradicts the state machine's own allowance of that edge); reusing `ForceStatus` (explicitly documented as bypassing state-machine validation — "callers must hold no locks... error recovery paths", not an operational drag-triggered path) | `Stopped→Active` is a valid state-machine edge (`session/state_machine.go:50`), which only makes sense if a `Stopped` session retains enough state to resume — mirroring `Pause()`'s cleanup (not `Destroy()`'s) keeps that invariant intact regardless of which status a session stopped from |
| Persistence key scoping (AC9) | `` `ws-${useDatabases().currentId}.stapler-squad-session-view-mode` `` | architecture.md §4, this plan's own re-verification of `useDatabase.ts` | Plain unscoped key (stack.md's original recommendation) | **stack.md's premise doesn't hold for this app's actual workspace-switch affordance**: `useDatabase.ts`'s `switchDatabase()` calls `SwitchDatabaseRequest`, waits for the *same* server process to restart, then does `window.location.reload()` on the *same* origin/port (`getApiBaseUrl()` is never changed) — confirmed by reading `useDatabase.ts:88-127`. Stack.md's "different workspaces = different port = different origin" framing describes the separate-process multi-instance testing convention, not this in-app switcher. Since a real, same-origin, per-workspace ID (`current_workspace_id`, `proto/session/v1/session.proto:1470`) already exists and is trivially available, using it is strictly safer than betting on an origin-scoping assumption that's false for this exact flow |
| Swimlane axis vs. status columns | 2D grid: grouping-strategy value becomes horizontal **swimlane rows**, status columns are always the column axis | ux.md §2, requirements.md Goal 4 read against Jira/Trello/Linear precedent | Grouping axis **replaces** status columns outright (literal reading of requirements.md's "re-groups board columns by that axis instead of status") | Replacing columns breaks "drag = status change" (what would dragging between `tag:frontend` and `tag:backend` "columns" even mutate?) for every reference product surveyed; crossing preserves both the swimlane goal and the drag-semantics invariant |
| `Tag` grouping multi-membership on the board | Render the same session's `BoardCard` in every matching tag swimlane row (duplicate rendering by design, same session ID); drag only mutates status (column), never row/tag membership | features.md §4 | Dedupe to first tag only; exclude `Tag` from the swimlane selector in board view | Deduping silently hides real multi-tag membership (data loss in the view); excluding `Tag` contradicts requirements.md's explicit "reuse the existing grouping-strategy selector" framing |
| Optimistic vs. confirmed status mutation | Optimistic move on drop, reconciled against the `UpdateSession` response and any concurrent `watchSessions` push in one merge pass keyed by session ID | pitfalls.md §1-2, ux.md §4 | Confirmed-only (card doesn't move until RPC resolves) | Matches the Linear/GitHub Projects precedent (ux.md §1) and keeps the board feeling responsive; the single-merge-pass discipline (not two independent re-renders) directly avoids pitfalls.md's "snap-back double-move" flicker |
| Within-column reorder | Not supported — drag only mutates column (status), never sort position within a column | ux.md §2 | Cosmetic-only reorder with no backend effect | No backend concept of session ordering exists (confirmed via scan); adding a purely-visual reorder risks users believing it persisted something real, per ux.md's explicit "drag = state change" mental-model analysis |
| Multi-select drag semantics | If the dragged card is part of the current `BoardSelection`, moving it moves the **whole selection** (client-side fan-out: one `updateSession` call per selected session ID, same target status); if the dragged card is not selected, only that card moves | features.md §7 (Trello/Linear precedent), design-patterns guidance | A single batched "MoveSelection" command/DTO sent to the backend | No batched-move RPC exists (and adding one is out of this plan's scope per the backend Non-Goals); a plain per-ID loop over the existing single-item `updateSession` call is "the minimum code that works" — inventing a batch abstraction for a client-side loop over ≤ a few dozen IDs would be premature generalization |
| Column-level virtualization | Reuse the same `react-virtuoso`/`@tanstack/react-virtual` approach `SessionList`'s row mode already uses, scoped per-`BoardColumn` (not board-wide) | pitfalls.md §5, ux.md §4 | No virtualization; a single board-wide virtualizer spanning all columns | No virtualization risks visible jank per pitfalls.md §5's DOM-node-count analysis; a board-wide virtualizer collides with dnd-kit's own documented rough edge (collision detection needs every droppable's rect, which breaks for off-screen/unmounted cards in cross-column virtualized setups per pitfalls.md §7) — per-column scoping keeps each column's virtualization independent and simpler |
| Non-drag fallback vs. keyboard-drag sensor | Build one `MoveToMenu` component, invoked identically by touch tap and by keyboard focus+Enter; do **not** additionally wire dnd-kit's `KeyboardSensor` arrow-key drag mode | pitfalls.md §3, ux.md §3 | Rely on dnd-kit's built-in `KeyboardSensor` for the keyboard path, build `MoveToMenu` only for touch | AC10 already mandates a touch-friendly non-drag action regardless of library choice; building only one control that serves keyboard, screen-reader, *and* touch users is cheaper than maintaining two independent accessible paths (dnd-kit's arrow-key sensor plus a separate touch menu) for the same underlying capability |

---

## Migration Plan

No database/storage schema migration. The one backend change is additive at the Go-method
level: a new `Instance.StopByUser()` method and a new `→ Stopped` branch inside the existing
`UpdateSession` handler (`server/services/session_service.go:1851-1879`). No proto file
changes are required — `UpdateSessionRequest.status` (`proto/session/v1/session.proto:579`)
already accepts `SESSION_STATUS_STOPPED`; today's handler simply never sends that branch's
target status anywhere except the Pause/Resume checks. This is purely additive and
backward-compatible: existing callers sending `status: PAUSED` or omitting `status` are
unaffected. No data backfill, no version gate, no feature flag needed for the backend change
itself (see Risk Control below for the frontend rollout flag).

## Observability Plan

- **Logs**: `StopByUser` logs at the same level/shape as `pauseLocked`'s existing
  `log.ForSession(i.Title).Info("session paused")` — add a matching `"session stopped by
  user"` line in `server/services/session_service.go`'s new branch, so `~/.stapler-squad/logs/stapler-squad.log`
  distinguishes user-triggered stops from tmux-exit-driven ones for future debugging.
- **Metrics**: none new required — this reuses the existing `events.NewSessionUpdatedEvent`
  publish path (`session_service.go:1888-1902`), which already feeds whatever session-event
  metrics/analytics exist. Frontend: emit one `track({name: "board_drag_transition", ...})`
  analytics event per completed drag/`MoveToMenu` action (success and rejection both), using
  the existing `useAnalytics()` hook already used elsewhere in `page.tsx` (e.g.
  `sessions_refreshed`), so drag-success/rejection rates are visible without new
  infrastructure.
- **Alerts**: none — this is a client-visible feature with existing error surfaces (toast +
  `console.error`); no new alerting is warranted for a v1 UI feature.

## Risk Control

- **Feature flag**: none required at the backend level (additive, non-breaking). At the
  frontend level, the List/Board toggle itself is the natural "flag" — if `SessionBoard`
  ships with a defect, users can immediately toggle back to List (which remains fully
  functional and untouched in its rendering, per the extraction discipline in Phase 1).
- **Rollback procedure**: revert the frontend PR (toggle, `SessionBoard`, `BoardColumn`,
  `BoardCard`, `MoveToMenu`, the extracted hook's *call sites* in `SessionList.tsx` can be
  reverted independently of the hook's existence since the hook is additive) and separately
  revert the Go `StopByUser`/`UpdateSession` branch if server-side issues surface — the two
  are independently revertable since the frontend only calls the new branch when a user drags
  into "Complete," a net-new interaction with no prior callers.
- **Staged rollout**: no gradual/percentage rollout mechanism exists in this codebase for
  frontend features (single-tenant desktop-style app, not a multi-tenant SaaS) — ship behind
  the reversible List/Board toggle itself as the staging mechanism.

## Unresolved Questions

- [ ] Should `StopByUser` gate on a new dedicated `InstancePermissions.CanStop` field instead
      of reusing `CanPause`? — blocks Story 0.1.1 only if product wants finer-grained
      permission control than "anything pausable is also user-stoppable" — owner: Tyler
      (product decision, not a research gap). Recommendation in this plan: reuse `CanPause`
      for v1 to avoid scope creep into the permissions model; revisit if a real need for
      differentiated control surfaces.
- [ ] Exact wording/severity styling for the AC5 rejection toast (e.g. does a network-error
      rejection look visually distinct from a business-rule rejection, per ux.md §4's table)
      — blocks Story 3.2.1's final polish, not its core mechanism — owner: whoever implements
      Phase 3, using ux.md §4's table as the starting spec.
- [ ] Whether analytics event names (`board_drag_transition` proposed above) should be
      registered anywhere beyond the feature registry — blocks nothing, but confirm during
      Phase 8's registry pass whether `docs/registry/` expects analytics events as a distinct
      category (research did not surface this) — owner: implementer, resolve by inspection of
      `docs/registry/schema.json` at Phase 8 time.

## Dependency Visualization

```
Phase 0 (Backend: StopByUser + UpdateSession branch)
   |
   v
Phase 1 (Frontend foundation: dnd-kit dep, extracted hook, column/transition tables, persistence)
   |
   +--------------------------+-----------------------------+
   v                          v                             v
Phase 2 (Board shell:     Phase 5 (Toggle + 'b'         Phase 6 (Swimlanes,
 SessionBoard/BoardColumn/  shortcut — needs             search, bulk-select
 BoardCard skeleton)        SessionBoard to exist        parity — needs hook +
   |                        as a target to render)        board shell)
   v                          |                             |
Phase 3 (Drag-and-drop        |                             |
 mechanic — needs Phase 0's   |                             |
 backend branch + Phase 2's   |                             |
 columns to drop onto)        |                             |
   |                          |                             |
   v                          |                             |
Phase 4 (Non-drag fallback    |                             |
 & accessibility — needs      |                             |
 Phase 3's DragOutcome/       |                             |
 transition table)            |                             |
   |                          |                             |
   +--------------------------+-----------------------------+
                              v
                    Phase 7 (Responsive/mobile CSS)
                              |
                              v
                    Phase 8 (Tests, registry, docs)
```

---

## Phase 0: Backend — Stop Capability

### Epic 0.1: Wire a user-triggered `→ Stopped` transition into `UpdateSession`

**Goal**: Close the RPC-surface-area gap architecture.md identified — `UpdateSession` today
only ever calls `instance.Pause()` or `instance.Resume()`; dragging a card into "Complete"
needs a third, new outcome that lands the session in `Stopped` without destroying it.

#### Story 0.1.1: Add `Instance.StopByUser()` and wire it into `UpdateSession`
**As a** board-view user, **I want** dragging a session card into the "Complete" column to
actually stop that session, **so that** the board reflects a real, durable state change (not
just a visual reorder).
**Acceptance Criteria**:
- Dragging a session card from "Running" (status `ACTIVE`) to "Complete" transitions its
  backend `SessionStatus` to `STOPPED`.
  - *Given* session `sess-123` with `status = SessionStatus.ACTIVE`, *When* the frontend calls
    `updateSession("sess-123", { status: SessionStatus.STOPPED })`, *Then*
    `UpdateSession`'s new branch calls `instance.StopByUser()`, which kills the tmux session,
    removes the worktree (if any), and transitions to `Stopped` via `transitionToLocked`, and
    the RPC response's `Session.status` is `SESSION_STATUS_STOPPED`.
- An already-`Stopped` or otherwise-illegal target is rejected with the same error shape as
  the existing Pause/Resume path.
  - *Given* session `sess-999` with `status = SessionStatus.CREATING`, *When*
    `updateSession("sess-999", { status: SessionStatus.STOPPED })` is called (an illegal edge
    per `session/state_machine.go` — only `Creating→Active`/`Creating→Stopped` from
    `Creating`... actually `Creating→Stopped` *is* valid, so use a genuinely illegal case:
    `Restoring→Stopped`, which has no entry in `transitionDefs`), *Then* `StopByUser` returns
    `session.ErrInvalidTransition{From: Restoring, To: Stopped}`, `classifyStopErr` maps it to
    `connect.CodeFailedPrecondition`, and no partial state change is persisted.
**Files**: `session/instance.go`, `session/state_machine.go` (read-only reference, no changes
needed — `Active/Paused/Hibernated → Stopped` are all already valid edges), `server/services/session_service.go`

##### Task 0.1.1a: Add `Instance.StopByUser()` (~5 min)
- In `session/instance.go`, immediately after `Resume()` (ends at line ~1508, before
  `Restart`), add:
  ```go
  // StopByUser transitions an Active, Paused, or Hibernated session to Stopped in
  // response to a direct user action (e.g. a board-view drag into "Complete").
  // Mirrors pauseLocked's cleanup (stop controller, commit-if-dirty, kill tmux, remove
  // worktree) but lands in Stopped instead of Paused, so a later Stopped→Active
  // transition has the same amount of state to reconstruct from as Paused→Active does.
  func (i *Instance) StopByUser() error {
  	return i.sendSyncErr(func(s *instanceState) error { return stopByUserLocked(s) })
  }

  func stopByUserLocked(s *instanceState) error {
  	i := s.inst
  	if !i.Permissions.CanPause {
  		return ErrPauseNotPermitted
  	}
  	if i.Status == Stopped {
  		return fmt.Errorf("instance is already stopped")
  	}
  	stopControllerLocked(s)
  	var errs []error
  	if i.IsWorktree {
  		if dirty, err := i.gitManager.IsDirty(); err != nil {
  			errs = append(errs, fmt.Errorf("failed to check if worktree is dirty: %w", err))
  		} else if dirty {
  			commitMsg := fmt.Sprintf("[claudesquad] update from '%s' on %s (stopped)", i.Title, time.Now().Format(time.RFC822))
  			if err := i.gitManager.CommitChanges(commitMsg); err != nil {
  				return i.combineErrors(append(errs, fmt.Errorf("failed to commit changes: %w", err)))
  			}
  		}
  	}
  	if err := i.KillSession(); err != nil {
  		if detachErr := i.pm().DetachSafely(); detachErr != nil {
  			errs = append(errs, fmt.Errorf("failed to detach tmux session: %w", detachErr))
  		}
  	}
  	if i.IsWorktree {
  		if _, err := os.Stat(i.gitManager.GetWorktreePath()); err == nil {
  			if err := i.gitManager.Remove(); err != nil {
  				return i.combineErrors(append(errs, fmt.Errorf("failed to remove git worktree: %w", err)))
  			}
  			_ = i.gitManager.Prune()
  		}
  	}
  	if err := i.combineErrors(errs); err != nil {
  		return err
  	}
  	if err := transitionToLocked(s, context.Background(), Stopped); err != nil {
  		return fmt.Errorf("failed to transition to Stopped: %w", err)
  	}
  	i.gitManager.InvalidateDirtyCache()
  	log.ForSession(i.Title).Info("session stopped by user")
  	return nil
  }
  ```
- Files: `session/instance.go`

##### Task 0.1.1b: Add `classifyStopErr` and wire the new branch in `UpdateSession` (~4 min)
- In `server/services/session_service.go`, add `classifyStopErr` right after
  `classifyPauseResumeErr` (line ~1698), identical body but with opDesc `"stop"`.
- In the "Handle status change" block (lines 1854-1879), add a new `else if` branch **before**
  the existing Paused-related branches (Stopped targets take priority — a drag straight from
  Paused to Complete must call `StopByUser`, not `Resume` followed by nothing):
  ```go
  if targetStatus == session.Stopped && instance.Status != session.Stopped {
  	if err := instance.StopByUser(); err != nil {
  		return nil, classifyStopErr(err, "stop")
  	}
  	updatedFields = append(updatedFields, "status")
  } else if targetStatus == session.Paused && instance.Status != session.Paused {
  	...
  ```
- Files: `server/services/session_service.go`

##### Task 0.1.1c: Add Go tests for the new branch (~5 min)
- In `server/services/session_service_test.go` (or the nearest existing `UpdateSession` test
  file — confirm exact filename via `grep -l "func TestUpdateSession" server/services/*_test.go`
  at implementation time), add:
  - `TestUpdateSession_should_StopSession_When_TargetIsStoppedFromActive`
  - `TestUpdateSession_should_RejectStop_When_TransitionIsIllegal` (e.g. from `Restoring`)
- Files: `server/services/session_service_test.go` (or confirmed equivalent)

##### Task 0.1.1d: Add a Go test for `StopByUser` itself (~4 min)
- In `session/instance_state_test.go` (or nearest existing state-machine test file), add
  `TestStopByUser_should_KillTmuxAndTransitionToStopped_When_SessionIsActive`, following the
  existing `Pause()`/`Resume()` test pattern for constructing a test `Instance`.
- Files: `session/instance_state_test.go` (or confirmed equivalent)

---

## Phase 1: Frontend Foundation

### Epic 1.1: Add `@dnd-kit` dependency

#### Story 1.1.1: Install `@dnd-kit/core`, `@dnd-kit/sortable`, `@dnd-kit/utilities`
**As a** developer, **I want** the chosen DnD library available as a project dependency,
**so that** the board's drag mechanic can be built on a maintained, accessible primitive
instead of hand-rolled HTML5 drag events.
**Acceptance Criteria**:
- `web-app/package.json` lists the three packages as `dependencies` (not `devDependencies`).
  - *Given* a fresh `pnpm install` in `web-app/`, *When* `require.resolve("@dnd-kit/core")` is
    checked, *Then* it resolves to `@dnd-kit/core@^6.3.1` without a peer-dependency warning
    breaking the install.
**Files**: `web-app/package.json`, `web-app/pnpm-lock.yaml`

##### Task 1.1.1a: Add the three packages to `package.json` (~2 min)
- In `web-app/package.json`'s `"dependencies"` block (alphabetically, after
  `"@codemirror/lang-html"` and before whatever comes next), add:
  ```json
  "@dnd-kit/core": "^6.3.1",
  "@dnd-kit/sortable": "^10.0.0",
  "@dnd-kit/utilities": "^3.2.2",
  ```
- Files: `web-app/package.json`

##### Task 1.1.1b: Run `pnpm install` and commit the lockfile (~2 min)
- `cd web-app && pnpm install`
- Verify `pnpm ls @dnd-kit/core @dnd-kit/sortable @dnd-kit/utilities` shows the expected
  versions with no `ERR_PNPM_PEER_DEP_ISSUES` failure (warnings are acceptable per stack.md's
  "not yet advertised as tested" note).
- Files: `web-app/pnpm-lock.yaml`

### Epic 1.2: Extract the shared filter/sort/group pipeline

#### Story 1.2.1: Extract `useFilteredGroupedSessions` from `SessionList.tsx`
**As a** developer, **I want** the filter→sort→group logic available as a standalone hook,
**so that** `SessionBoard` can consume identical filtering/search/grouping behavior without
duplicating `SessionList.tsx:530-656`'s business logic.
**Acceptance Criteria**:
- List view's behavior is unchanged after the extraction (AC11).
  - *Given* `SessionList.tsx`'s existing test suite (e.g.
    `SessionList.collapse.test.tsx`), *When* `SessionList` is refactored to call
    `useFilteredGroupedSessions(sessions, filterState, groupingStrategy)` instead of its
    inline `useMemo` chain, *Then* `cd web-app && npx jest --no-coverage
    --testPathPatterns="SessionList"` passes with no changed assertions.
**Files**: `web-app/src/lib/hooks/useFilteredGroupedSessions.ts` (new),
`web-app/src/components/sessions/SessionList.tsx`

##### Task 1.2.1a: Create `useFilteredGroupedSessions.ts` with the extracted logic (~5 min)
- Create `web-app/src/lib/hooks/useFilteredGroupedSessions.ts`. Move the body of
  `filteredSessions` (`SessionList.tsx:530-589`), `sortedSessions` (`:598-630`),
  `filteredSessionIds` (`:633-636`), and `groupedSessions` (`:648-650`) `useMemo` blocks
  into this hook, parameterized by the same inputs those `useMemo`s currently close over
  (`sessions`, `searchQuery`, `selectedStatus`, `selectedCategory`, `selectedTag`,
  `hidePaused`, `showArchived`, `filterNeedsApproval`, `reviewItemBySessionId`, `sortField`,
  `sortDir`, `costById`, `groupingStrategy`). Return
  `{ filteredSessions, sortedSessions, groupedSessions, filteredSessionIds }`.
- Files: `web-app/src/lib/hooks/useFilteredGroupedSessions.ts`

##### Task 1.2.1b: Wire `SessionList.tsx` to call the new hook (~5 min)
- Replace the four `useMemo` blocks at `SessionList.tsx:530-650` with a single call to
  `useFilteredGroupedSessions(...)`, destructuring the same four values so every downstream
  reference (`flatItems`, `cardFlatSessions`, `activeSelection`, etc.) is untouched.
- Files: `web-app/src/components/sessions/SessionList.tsx`

##### Task 1.2.1c: Run the existing SessionList test suite to confirm zero regressions (~3 min)
- `cd web-app && npx jest --no-coverage --testPathPatterns="SessionList"`
- Confirm all pre-existing tests pass unchanged; fix any that broke from the refactor before
  proceeding (do not adjust expected values to match new behavior — the extraction must be
  behavior-preserving per AC11).
- Files: none (verification task)

### Epic 1.3: Board domain types and transition table

#### Story 1.3.1: Column membership and legal-transition tables
**As a** developer, **I want** pure, testable functions for column membership and legal
column-to-column drags, **so that** `SessionBoard`'s drop handler can reject illegal drags
client-side before firing any RPC (AC5).
**Acceptance Criteria**:
- `getBoardColumnKey` implements the branching logic in the "Column Membership" section above.
  - *Given* a `Session` with `status = SessionStatus.ACTIVE` and
    `subStatus = SubStatus.NEEDS_APPROVAL`, *When* `getBoardColumnKey(session)` is called,
    *Then* it returns `"needs_review"`.
- `legalBoardTransitions["complete"]` is an empty array (Stopped→Active is a valid backend
  edge, but "Complete" is a terminal *board* column with no defined outbound drag per Pattern
  Decision "Needs Review column semantics" and requirements' Non-Goals framing — dragging out
  of Complete is out of scope for this plan; only *into* Complete is wired).
**Files**: `web-app/src/lib/board/columns.ts` (new), `web-app/src/lib/board/transitions.ts` (new),
`web-app/src/lib/board/columns.test.ts` (new), `web-app/src/lib/board/transitions.test.ts` (new)

##### Task 1.3.1a: Implement `getBoardColumnKey` (~4 min)
- Create `web-app/src/lib/board/columns.ts` exporting `type BoardColumnKey = "running" |
  "needs_review" | "paused" | "complete"`, `BOARD_COLUMNS: {key: BoardColumnKey, label:
  string}[]` (in display order: Running, Needs Review, Paused, Complete), and
  `getBoardColumnKey(session: Session): BoardColumnKey` implementing the branching logic
  above, importing `SessionStatus`/`SubStatus` from `@/gen/session/v1/types_pb`.
- Files: `web-app/src/lib/board/columns.ts`

##### Task 1.3.1b: Implement `legalBoardTransitions` (~4 min)
- Create `web-app/src/lib/board/transitions.ts` exporting
  `legalBoardTransitions: Record<BoardColumnKey, BoardColumnKey[]>`:
  ```ts
  export const legalBoardTransitions: Record<BoardColumnKey, BoardColumnKey[]> = {
    running: ["paused", "complete"],
    paused: ["running", "complete"],
    needs_review: [], // handled via ApprovalResolution, not a raw column-to-column drag
    complete: [],
  };
  export function isLegalBoardDrag(from: BoardColumnKey, to: BoardColumnKey): boolean {
    return legalBoardTransitions[from]?.includes(to) ?? false;
  }
  ```
- Files: `web-app/src/lib/board/transitions.ts`

##### Task 1.3.1c: Unit tests for both modules (~5 min)
- `columns.test.ts`: one test per `BoardColumnKey` branch (5 cases: needs_review, running via
  ACTIVE, running via CREATING, paused via HIBERNATED, complete).
- `transitions.test.ts`: table-driven test asserting `isLegalBoardDrag` for every
  `(from, to)` pair in `BOARD_COLUMNS`, matching the table above exactly.
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="lib/board"`.
- Files: `web-app/src/lib/board/columns.test.ts`, `web-app/src/lib/board/transitions.test.ts`

### Epic 1.4: Workspace-scoped view-mode persistence

#### Story 1.4.1: Persist `SessionViewMode` per workspace
**As a** user with multiple workspace databases, **I want** my last-used List/Board choice
remembered independently per workspace, **so that** switching workspaces doesn't silently
carry over an unrelated view preference (AC9).
**Acceptance Criteria**:
- The storage key is scoped by `useDatabases().currentId`.
  - *Given* `currentWorkspaceId = "ws-abc123"` and the user switches to `SessionViewMode =
    "board"`, *When* the browser reloads, *Then* `localStorage.getItem("ws-ws-abc123.stapler-squad-session-view-mode")`
    returns `"board"`.
  - *Given* the user then calls `switchDatabase()` to `"ws-def456"` (same-origin reload per
    the Pattern Decision above), *When* the page reloads, *Then*
    `localStorage.getItem("ws-ws-def456.stapler-squad-session-view-mode")` is `null` (defaults
    to `"list"`), independent of `ws-abc123`'s stored value.
**Files**: `web-app/src/lib/hooks/useSessionViewMode.ts` (new)

##### Task 1.4.1a: Implement `useSessionViewMode` hook (~5 min)
- Create `web-app/src/lib/hooks/useSessionViewMode.ts`:
  ```ts
  import { useState, useCallback } from "react";
  import { useDatabases } from "./useDatabase";

  export type SessionViewMode = "list" | "board";

  function storageKey(workspaceId: string) {
    return `ws-${workspaceId}.stapler-squad-session-view-mode`;
  }

  export function useSessionViewMode(): [SessionViewMode, (m: SessionViewMode) => void] {
    const { currentId } = useDatabases();
    const [mode, setModeState] = useState<SessionViewMode>(() => {
      try {
        const raw = currentId ? localStorage.getItem(storageKey(currentId)) : null;
        return raw === "board" ? "board" : "list";
      } catch {
        return "list";
      }
    });
    const setMode = useCallback((m: SessionViewMode) => {
      setModeState(m);
      try {
        if (currentId) localStorage.setItem(storageKey(currentId), m);
      } catch { /* localStorage unavailable — in-memory state still updates */ }
    }, [currentId]);
    return [mode, setMode];
  }
  ```
- Files: `web-app/src/lib/hooks/useSessionViewMode.ts`

##### Task 1.4.1b: Unit test for workspace-scoping (~4 min)
- `useSessionViewMode.test.ts`: mock `useDatabases` to return two different `currentId`
  values across renders, assert the storage key changes and values don't leak across IDs
  (directly encodes the AC9 GWT above).
- Files: `web-app/src/lib/hooks/useSessionViewMode.test.ts`

---

## Phase 2: Board Shell

### Epic 2.1: `SessionBoard` + `BoardColumn` skeleton

#### Story 2.1.1: Render 4 status columns with count badges (no drag yet)
**As a** user, **I want** to see my sessions organized into Running/Needs Review/Paused/
Complete columns, **so that** I can scan workflow state at a glance (AC2, AC3).
**Acceptance Criteria**:
- Columns render in the fixed order Running, Needs Review, Paused, Complete, each showing a
  count badge (AC2, AC3).
  - *Given* 2 sessions map to `"running"`, 1 to `"needs_review"`, 4 to `"paused"` (3 `PAUSED`
    + 1 `HIBERNATED`), 0 to `"complete"`, *When* `SessionBoard` renders, *Then* the header
    badges read "2", "1", "4", "0" respectively, and the empty "Complete" column still renders
    its column shell (not collapsed to zero width) with an empty-state message.
**Files**: `web-app/src/components/sessions/SessionBoard.tsx` (new),
`web-app/src/components/sessions/SessionBoard.css.ts` (new),
`web-app/src/components/sessions/BoardColumn.tsx` (new),
`web-app/src/components/sessions/BoardColumn.css.ts` (new)

##### Task 2.1.1a: Scaffold `SessionBoard.tsx` (no DnD yet) (~5 min)
- Create `SessionBoard.tsx` accepting the same `sessions: Session[]` +
  `storageKeyPrefix?: string` + the full mutation-callback prop surface `SessionList`
  currently accepts (so `SessionListPaneBody` can swap between the two with identical props
  in Phase 5). Internally: call `useFilteredGroupedSessions` with `groupingStrategy =
  GroupingStrategy.None` for v1 (swimlanes land in Phase 6), then bucket `filteredSessions`
  into the 4 `BOARD_COLUMNS` via `getBoardColumnKey`, and render one `BoardColumn` per bucket.
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 2.1.1b: Scaffold `BoardColumn.tsx` — header, count badge, empty state (~5 min)
- Create `BoardColumn.tsx` modeled directly on `BacklogBoard.tsx`'s `BoardColumn` function
  (`web-app/src/components/backlog/BacklogBoard.tsx:63-138`): `<section aria-label="{label}
  column">` wrapping a header (`<h3>` + count badge `<span aria-label="{count} sessions">`)
  and a `role="list"` card container. Empty state: reuse the visual language of
  `SessionListEmptyState.css.ts` per ux.md §4's explicit recommendation, rendering a short
  "No sessions" message instead of collapsing the column.
- Files: `web-app/src/components/sessions/BoardColumn.tsx`

##### Task 2.1.1c: Add `SessionBoard.css.ts` and `BoardColumn.css.ts` (vanilla-extract) (~5 min)
- `SessionBoard.css.ts`: a `board` style — `display: flex`, `gap: vars.space["4"]`,
  `overflowX: "auto"` (desktop: all 4 columns visible side by side; mobile handled in Phase 7).
- `BoardColumn.css.ts`: `column` (fixed width e.g. `320px`, `flexShrink: 0`, background via
  `vars.color.*` token, `display: flex, flexDirection: column`), `columnHeader`,
  `columnTitle`, `columnCount` (badge), `columnCards` (`overflowY: auto`), `emptyColumn` —
  modeled on `BacklogBoard.css.ts`'s equivalent class names. No hardcoded hex/z-index; if a
  z-index is needed for sticky headers, add a named slot to `theme-contract.css.ts`'s
  `zIndex` map first.
- Files: `web-app/src/components/sessions/SessionBoard.css.ts`,
  `web-app/src/components/sessions/BoardColumn.css.ts`

##### Task 2.1.1d: Component test for column bucketing and count badges (~5 min)
- `SessionBoard.test.tsx`: render with a fixed array of sessions covering all 4 buckets
  (including one `HIBERNATED` and one `CREATING`), assert each `BoardColumn`'s count badge
  text matches expectation, and that the "Complete" column renders its empty-state message
  when it has zero matching sessions.
- Files: `web-app/src/components/sessions/SessionBoard.test.tsx`

### Epic 2.2: `BoardCard` wrapper

#### Story 2.2.1: Wrap `SessionCard` with a drag handle (no drag behavior yet)
**As a** developer, **I want** a thin wrapper around `SessionCard` that owns board-specific
chrome, **so that** the board doesn't fork or duplicate `SessionCard`'s ~900 lines of
rendering logic.
**Acceptance Criteria**:
- `BoardCard` renders `SessionCard` unmodified plus a visually distinct drag-handle affordance.
  - *Given* a session rendered via `<BoardCard session={session} .../>`, *When* the component
    tree is inspected, *Then* it contains exactly one `<SessionCard>` instance (no duplicated
    card markup) plus a drag-handle element with `aria-label` describing the drag action.
**Files**: `web-app/src/components/sessions/BoardCard.tsx` (new),
`web-app/src/components/sessions/BoardCard.css.ts` (new)

##### Task 2.2.1a: Create `BoardCard.tsx` wrapping `SessionCard` (~5 min)
- Create `BoardCard.tsx` accepting `session: Session` plus the same mutation-callback props
  `SessionCard` accepts, passed straight through. Wrap `<SessionCard {...props} />` in a
  `<div>` with a drag-handle icon button (`aria-label="Drag {session.title} to move"`) — no
  `useDraggable` wiring yet (added in Phase 3).
- Files: `web-app/src/components/sessions/BoardCard.tsx`

##### Task 2.2.1b: Add `BoardCard.css.ts` (~3 min)
- Drag handle styling (cursor `grab`, `touch-action: none` per pitfalls.md §4's requirement —
  set now even though drag isn't wired yet, so Phase 3 doesn't have to remember it),
  positioned via `vars.*` tokens.
- Files: `web-app/src/components/sessions/BoardCard.css.ts`

##### Task 2.2.1c: Wire `BoardColumn` to render `BoardCard` per session (~3 min)
- Update `BoardColumn.tsx`'s card-list rendering to map over its bucket's sessions and render
  `<BoardCard key={session.id} session={session} .../>` instead of a placeholder.
- Files: `web-app/src/components/sessions/BoardColumn.tsx`

---

## Phase 3: Drag-and-Drop Mechanic

### Epic 3.1: `DndContext` wiring and optimistic move

#### Story 3.1.1: Dragging a card to a valid column fires `updateSession` and moves the card
**As a** user, **I want** dragging a session card between columns to change its status,
**so that** the board is a real control surface, not just a read-only visualization (AC4).
**Acceptance Criteria**:
- A legal drag fires `updateSession` with the correct target status and the card renders in
  its new column immediately.
  - *Given* session `sess-123` (`status = ACTIVE`, board column `"running"`), *When* the user
    drags its `BoardCard` and drops it on the `"paused"` `BoardColumn`, *Then*
    `updateSession("sess-123", { status: SessionStatus.PAUSED })` fires exactly once and
    `sess-123`'s `BoardCard` renders under "Paused" immediately (optimistic), with a pending
    visual state until the RPC resolves.
**Files**: `web-app/src/components/sessions/SessionBoard.tsx`,
`web-app/src/components/sessions/BoardColumn.tsx`,
`web-app/src/components/sessions/BoardCard.tsx`,
`web-app/src/lib/board/statusForColumnMove.ts` (new)

##### Task 3.1.1a: Add `statusForColumnMove` mapping (~4 min)
- Create `web-app/src/lib/board/statusForColumnMove.ts` exporting
  `statusForColumnMove(session: Session, targetColumn: BoardColumnKey): SessionStatus | null`
  — returns the concrete `SessionStatus` to send for a given drop, branching on the
  session's *current* status (not just its column), since "paused" column members can be
  either `PAUSED` or `HIBERNATED` and need different handling on the way *out*:
  - target `"paused"` → `SessionStatus.PAUSED`
  - target `"complete"` → `SessionStatus.STOPPED`
  - target `"running"` from a `PAUSED` session → `SessionStatus.ACTIVE` (via `updateSession`)
  - target `"running"` from a `HIBERNATED` session → returns `null` (caller must call
    `resumeHibernatedSession` instead — a different RPC entirely, per architecture.md's
    finding that hibernation has its own dedicated RPC)
  - Files: `web-app/src/lib/board/statusForColumnMove.ts`

##### Task 3.1.1b: Add `DndContext` + sensors to `SessionBoard` (~5 min)
- Wrap `SessionBoard`'s column row in `<DndContext sensors={sensors} onDragStart={...}
  onDragEnd={...}>` using `useSensors(useSensor(PointerSensor), useSensor(TouchSensor, {
  activationConstraint: { delay: 200, tolerance: 8 } }))` per pitfalls.md §4's touch-hold
  guidance. `onDragStart` sets `inFlightDragSessionId`.
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 3.1.1c: Make `BoardColumn` a `useDroppable` target and `BoardCard` a `useDraggable` source (~5 min)
- `BoardColumn`: wrap its card-list container with `useDroppable({ id: column.key })`.
- `BoardCard`: wrap the drag-handle element with `useDraggable({ id: session.id })`, applying
  the returned `transform` via `CSS.Translate.toString(transform)` (from
  `@dnd-kit/utilities`) as an inline style — the one sanctioned "CSS custom property bridge"
  runtime-dynamic-value pattern per `.claude/rules/css-architecture.md`.
- Files: `web-app/src/components/sessions/BoardColumn.tsx`,
  `web-app/src/components/sessions/BoardCard.tsx`

##### Task 3.1.1d: Implement `onDragEnd` — call `statusForColumnMove` + fire the mutation (~5 min)
- In `SessionBoard.tsx`'s `onDragEnd(event)`: resolve the dragged session and target column
  key from `event.active`/`event.over`; if `!isLegalBoardDrag(fromColumn, toColumn)`, treat as
  a rejection (Phase 3.2) with no RPC call; otherwise resolve the target `SessionStatus` via
  `statusForColumnMove`, optimistically re-bucket the session locally (a small local
  `Map<sessionId, BoardColumnKey override>` state, cleared once the real `sessions` prop
  reflects the change), then call `updateSession`/`resumeHibernatedSession` as appropriate,
  and clear `inFlightDragSessionId`.
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 3.1.1e: Component test for a legal drag firing the correct RPC call (~5 min)
- `SessionBoard.dragdrop.test.tsx`: simulate a drop via dnd-kit's test utilities (or a direct
  call to the extracted `onDragEnd` handler with a constructed `DragEndEvent`), assert
  `onUpdateSession`-equivalent mock is called once with `{status: SessionStatus.PAUSED}` for
  an Active→Paused drag.
- Files: `web-app/src/components/sessions/SessionBoard.dragdrop.test.tsx`

### Epic 3.2: Rejection handling and live-push reconciliation

#### Story 3.2.1: Invalid drags are rejected client-side with a visible error (AC5)
**As a** user, **I want** an invalid drag to bounce back with a clear explanation, **so that**
I understand why my action didn't take effect instead of it looking like a silent bug.
**Acceptance Criteria**:
- An illegal drag never calls the RPC and snaps back with a distinct toast.
  - *Given* session `sess-456` with `status = STOPPED` (board column `"complete"`), *When* the
    user drags it and drops it on `"needs_review"`, *Then* `isLegalBoardDrag("complete",
    "needs_review")` returns `false`, no `updateSession`/`ResolveApproval` call fires, the
    card animates back to "Complete", and a toast reads "Can't move a completed session to
    Needs Review."
- A server-rejected (but client-legal-looking) drag also snaps back with a *different*
  message, distinguishing "your action failed" from "the system changed it."
  - *Given* session `sess-789` (`status = ACTIVE`, column `"running"`) is dragged to
    `"complete"`, and the `UpdateSession` RPC call rejects with
    `connect.CodeFailedPrecondition` (e.g. it transitioned to `STOPPED` via tmux-exit-detection
    microseconds before the drop, making the RPC's own state-machine check now see
    `Stopped→Stopped`), *When* the rejection response arrives, *Then* `DragOutcome` resolves
    to `{type: "rejected", reason: "..."}`, the card re-renders in its *actual current*
    server-confirmed column (not necessarily its pre-drag column), and a toast reads "Session
    sess-789 already changed state — showing its current status."
**Files**: `web-app/src/components/sessions/SessionBoard.tsx`,
`web-app/src/lib/board/dragOutcome.ts` (new), `web-app/src/components/sessions/BoardToast.tsx` (new, or reuse existing toast primitive if one exists)

##### Task 3.2.1a: Define `DragOutcome` type (~2 min)
- Create `web-app/src/lib/board/dragOutcome.ts`:
  ```ts
  export type DragOutcome =
    | { type: "moved" }
    | { type: "rejected_illegal"; from: BoardColumnKey; to: BoardColumnKey }
    | { type: "rejected_by_server"; reason: string }
    | { type: "network_error" }
    | { type: "cancelled" };
  ```
- Files: `web-app/src/lib/board/dragOutcome.ts`

##### Task 3.2.1b: Client-side illegal-drag short-circuit in `onDragEnd` (~3 min)
- Before calling any RPC in `onDragEnd`, check `isLegalBoardDrag`; if false, produce
  `{type: "rejected_illegal", from, to}`, skip the optimistic re-bucket entirely (card never
  visually leaves its column), and surface the toast.
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 3.2.1c: Reconcile a server-rejected drag against the session's actual current state (~5 min)
- Catch the `updateSession`/`resumeHibernatedSession` promise rejection in `onDragEnd`;
  clear the optimistic override for that session ID (letting it re-derive its column from the
  latest `sessions` prop — the pitfalls.md-mandated "return the authoritative value, not the
  stale one" discipline), produce `{type: "rejected_by_server", reason}`, and surface a toast
  whose copy is visibly distinct from the illegal-drag toast (per ux.md §4's table).
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 3.2.1d: Add a minimal toast surface if none exists (~5 min)
- Grep for an existing toast/notification primitive (`web-app/src/components/ui/NotificationToast.css.ts`
  suggests one exists — confirm and reuse it via its existing hook/component rather than
  building a new one; only create `BoardToast.tsx` if genuinely no reusable primitive is
  found).
- Files: `web-app/src/components/sessions/SessionBoard.tsx` (wired to whichever toast
  mechanism is found/confirmed)

##### Task 3.2.1e: Tests for both rejection paths (~5 min)
- `SessionBoard.dragdrop.test.tsx` (extend): one test for the illegal-drag short-circuit (no
  RPC call, card stays put), one for the server-rejection path (RPC called, promise rejected,
  card re-renders from the (mocked) authoritative `sessions` prop, distinct toast text).
- Files: `web-app/src/components/sessions/SessionBoard.dragdrop.test.tsx`

#### Story 3.2.2: Freeze the dragged card's column during live pushes
**As a** user, **I want** my in-progress drag to not be yanked out from under my cursor by an
unrelated real-time update, **so that** the drag gesture completes predictably.
**Acceptance Criteria**:
- A `watchSessions` push for the dragged session's ID during an active drag does not change
  its rendered column until the drag resolves.
  - *Given* `inFlightDragSessionId = "sess-123"` (mid-drag), *When* a `watchSessions` push
    arrives changing `sess-123.status` to `STOPPED` (e.g. an unrelated tmux-exit event),
    *Then* `SessionBoard` continues rendering `sess-123`'s card in its pre-drag column until
    `onDragEnd` fires, after which it reconciles against the (now-current) server state.
**Files**: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 3.2.2a: Suppress column recomputation for the in-flight drag ID (~4 min)
- In `SessionBoard`'s bucketing logic, when computing `getBoardColumnKey(session)` for the
  session matching `inFlightDragSessionId`, use a snapshot of that session's status captured
  at `onDragStart` time instead of the live `sessions` prop value, until `onDragEnd` clears
  `inFlightDragSessionId`.
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 3.2.2b: Test for drag-freeze against a live push (~4 min)
- Simulate: start a drag on `sess-123`, update the `sessions` prop (simulating a
  `watchSessions` push) to move `sess-123` to a different column, assert the rendered column
  is unchanged until `onDragEnd` is invoked.
- Files: `web-app/src/components/sessions/SessionBoard.dragdrop.test.tsx`

### Epic 3.3: "Needs Review" special-case drag semantics

#### Story 3.3.1: Dragging out of Needs Review resolves the approval, not a status change
**As a** user, **I want** dragging a card out of "Needs Review" to actually resolve the
pending approval, **so that** the board's drag gesture matches what "leaving that column"
really means for this app's domain.
**Acceptance Criteria**:
- Dragging a Needs-Review card to "Running" calls `ResolveApproval` (approve), not
  `updateSession`.
  - *Given* session `sess-321` (`status = ACTIVE`, `subStatus = NEEDS_APPROVAL`, column
    `"needs_review"`), *When* the user drags it to `"running"`, *Then* `ResolveApproval` is
    called with an approve decision (not `updateSession`), and once resolved, `sess-321`'s
    `subStatus` clears and it naturally re-buckets to `"running"` via the normal
    `getBoardColumnKey` derivation (no manual override needed).
- Dragging a Needs-Review card is disallowed for any target other than "Running" (approve is
  the only board-drag-representable resolution; deny requires explicit confirmation and stays
  a `SessionCard`-level action, not a raw drag).
  - *Given* session `sess-321` is dragged from `"needs_review"` to `"complete"`, *When*
    `onDragEnd` evaluates the drop, *Then* it is treated as `{type: "rejected_illegal", from:
    "needs_review", to: "complete"}` since `legalBoardTransitions["needs_review"] = []` — the
    "Running" case is handled as a special-cased exception in `onDragEnd`, not by adding an
    edge to the generic transition table.
**Files**: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 3.3.1a: Special-case Needs-Review→Running in `onDragEnd` (~5 min)
- Before the generic `isLegalBoardDrag` check, add: if `fromColumn === "needs_review" &&
  toColumn === "running"`, call `ResolveApproval` (approve) via the existing
  `useSessionService()`/approval-hook equivalent (confirm exact function name via `grep -n
  "resolveApproval" web-app/src/lib/hooks/*.ts` at implementation time) instead of
  `updateSession`; any other `fromColumn === "needs_review"` target falls through to the
  generic (always-illegal, per the empty transitions array) path.
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 3.3.1b: Test for the Needs-Review→Running special case (~4 min)
- `SessionBoard.dragdrop.test.tsx` (extend): assert `ResolveApproval`-equivalent mock is
  called (not `updateSession`) for a needs_review→running drag, and that a
  needs_review→complete drag is rejected as illegal.
- Files: `web-app/src/components/sessions/SessionBoard.dragdrop.test.tsx`

---

## Phase 4: Non-Drag Fallback & Accessibility

### Epic 4.1: `MoveToMenu` — the shared touch/keyboard fallback

#### Story 4.1.1: Every card has a non-drag "Move to..." action (AC10, WCAG SC 2.5.7)
**As a** touch or keyboard-only user, **I want** a menu-based way to move a session between
columns, **so that** I'm not locked out of the board's core interaction by device or ability.
**Acceptance Criteria**:
- Every `BoardCard` exposes a `MoveToMenu` listing only the *legal* target columns for that
  card's current column.
  - *Given* `BoardCard` for a session in column `"running"`, *When* its `MoveToMenu` is
    opened (via tap or `Enter` while focused), *Then* it lists exactly "Paused" and
    "Complete" as options (from `legalBoardTransitions["running"]`), and selecting "Paused"
    calls the same `statusForColumnMove` + `updateSession` path a completed drag would have.
  - *Given* `BoardCard` for a session in column `"needs_review"`, *When* its `MoveToMenu` is
    opened, *Then* it lists "Running" (mapped to the `ResolveApproval`-approve path from Story
    3.3.1, not a raw status change), reusing the exact same special-case branch.
**Files**: `web-app/src/components/sessions/MoveToMenu.tsx` (new),
`web-app/src/components/sessions/MoveToMenu.css.ts` (new),
`web-app/src/components/sessions/BoardCard.tsx`

##### Task 4.1.1a: Extract the "attempt column move" function so drag and menu share it (~4 min)
- In `SessionBoard.tsx`, factor `onDragEnd`'s post-legality-check body (statusForColumnMove →
  RPC call → optimistic update → `DragOutcome`) into a standalone `attemptColumnMove(session,
  fromColumn, toColumn): Promise<DragOutcome>` function, called by both `onDragEnd` and (via a
  prop) `MoveToMenu`. This directly satisfies stack.md's flagged requirement that "the drop
  handler and the menu action must converge on one shared function rather than duplicating
  mutation logic."
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 4.1.1b: Build `MoveToMenu.tsx` (~5 min)
- A small dropdown/menu component (model on the existing overflow-menu pattern used
  elsewhere in `web-app/src/components/sessions/` — confirm the exact file via `grep -rl
  "SessionActionsOverflow\|role=\"menu\"" web-app/src/components/sessions/` at implementation
  time) listing `legalBoardTransitions[currentColumn]` (plus the needs_review→running special
  case) as menu items, calling `attemptColumnMove` on selection, and closing on completion.
  `aria-haspopup="menu"`, trigger button `aria-label="Move {session.title} to another column"`.
- Files: `web-app/src/components/sessions/MoveToMenu.tsx`

##### Task 4.1.1c: Add `MoveToMenu.css.ts` and wire it into `BoardCard` (~4 min)
- Position via `vars.*` tokens; use `zIndex.dropdown` from `theme-contract.css.ts` (already
  defined, no new slot needed) for the open menu layer.
- Files: `web-app/src/components/sessions/MoveToMenu.css.ts`,
  `web-app/src/components/sessions/BoardCard.tsx`

##### Task 4.1.1d: Tests for `MoveToMenu` (~4 min)
- Assert menu options match `legalBoardTransitions` for a few representative columns, and
  that selecting an option calls `attemptColumnMove` with the right arguments.
- Files: `web-app/src/components/sessions/MoveToMenu.test.tsx`

### Epic 4.2: ARIA live-region announcements and focus management

#### Story 4.2.1: Screen-reader announcements for drag/move outcomes
**As a** screen-reader user, **I want** to hear when a card moves, is rejected, or fails,
**so that** I have the same feedback a sighted user gets from the bounce-back animation.
**Acceptance Criteria**:
- Every `DragOutcome` produces a distinct `aria-live="polite"` announcement.
  - *Given* a `{type: "moved"}` outcome for session "Fix login bug" moving to "Paused", *When*
    the announcement region updates, *Then* it reads "Fix login bug moved to Paused."
  - *Given* a `{type: "rejected_illegal", ...}` outcome, *Then* it reads "Can't move Fix login
    bug to Needs Review."
**Files**: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 4.2.1a: Add a visually-hidden `aria-live` region to `SessionBoard` (~4 min)
- Mirror `SessionList.tsx:1169`'s existing pattern (`<div id="empty-state-live" role="status"
  aria-live="polite" aria-atomic="true" style={{position: "absolute", width: 1, height: 1,
  overflow: "hidden", clipPath: "inset(50%)", whiteSpace: "nowrap"}}>`) for a new
  `board-drag-live` region, updated from `attemptColumnMove`'s resolved `DragOutcome`.
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 4.2.1b: Focus management after a successful move (~3 min)
- After `attemptColumnMove` resolves to `{type: "moved"}` (whether via drag or `MoveToMenu`),
  programmatically move focus to the card's new DOM location (its drag handle or menu
  trigger) rather than letting focus fall back to `<body>`.
- Files: `web-app/src/components/sessions/SessionBoard.tsx`,
  `web-app/src/components/sessions/BoardCard.tsx`

---

## Phase 5: View Toggle & Keyboard Shortcut

### Epic 5.1: List/Board toggle control

#### Story 5.1.1: A toggle in the dashboard renders `SessionList` or `SessionBoard`
**As a** user, **I want** a visible List/Board switch, **so that** I can pick the mental model
that fits what I'm doing right now (AC1).
**Acceptance Criteria**:
- Toggling preserves filters/search/selection state (they live in the shared hook/state, not
  duplicated per view).
  - *Given* `searchQuery = "auth-fix"` while in `SessionViewMode = "list"`, *When* the user
    clicks the toggle to switch to `"board"`, *Then* `SessionBoard` renders using the same
    `searchQuery` value (no reset), and no new `listSessions()`/RPC refetch occurs.
**Files**: `web-app/src/components/pane/PaneSplitRenderer.tsx`

##### Task 5.1.1a: Lift view-mode state into `SessionListPaneBody` (~5 min)
- In `PaneSplitRenderer.tsx`'s `SessionListPaneBody` (lines ~150-195), call
  `useSessionViewMode()`, add a small toggle control (two buttons or a segmented control:
  "List" / "Board") above the existing `<div className={sessionListScroll}>`, and
  conditionally render `<SessionList ...>` or `<SessionBoard ...>` with the *same* props
  object (both components accept the same `sessions` + mutation-callback shape by
  construction, per Task 2.1.1a).
- Files: `web-app/src/components/pane/PaneSplitRenderer.tsx`

##### Task 5.1.1b: Component test for the toggle preserving filter state (~5 min)
- Render `SessionListPaneBody`, type into the search box while in list view, click the Board
  toggle, assert `SessionBoard` receives/reflects the same search string (via whatever shared
  state mechanism Phase 1's extraction produced) and no additional `listSessions` mock call
  fired.
- Files: `web-app/src/components/pane/__tests__/PaneSplitRenderer.viewToggle.test.tsx` (new)

### Epic 5.2: `b` keyboard shortcut

#### Story 5.2.1: Pressing `b` toggles list/board (AC1)
**As a** keyboard user, **I want** a one-key toggle, **so that** switching views doesn't
require reaching for the mouse.
**Acceptance Criteria**:
- `b` toggles the view when no text input is focused; does nothing when one is.
  - *Given* the dashboard has focus and no `INPUT`/`TEXTAREA`/`SELECT` element is focused,
    *When* the user presses `b`, *Then* `SessionViewMode` flips from `"list"` to `"board"` (or
    back).
  - *Given* the user is typing "bug" into the instant search box (an `INPUT` element
    currently focused), *When* they type the `b` in "bug", *Then* the view mode does **not**
    toggle (the `b` character is simply typed into the search box).
**Files**: `web-app/src/app/page.tsx`, `web-app/src/components/pane/PaneSplitRenderer.tsx`

##### Task 5.2.1a: Register the `b` handler via `useKeyboard` (~4 min)
- `useKeyboard`'s existing `ignoreElements: ["INPUT", "TEXTAREA", "SELECT"]` default (used
  already at `page.tsx:353`) already satisfies "don't fire while a text input is focused" for
  standard form elements — confirm the omnibar's own input is a real `<input>` element (grep
  `Omnibar.tsx` for its root element tag) so it's covered by the same guard; if it's a
  contenteditable `div` instead, add an explicit `document.activeElement` check. Add a `"b"`
  entry to the existing `useKeyboard({...})` call at `page.tsx:353`, calling a new callback
  prop threaded down to `SessionListPaneBody`/`PaneSplitRenderer` (or, if view-mode state
  needs to live above `page.tsx` for keyboard routing to reach it, lift `useSessionViewMode()`
  one level higher — confirm exact wiring point at implementation time by checking how
  `page.tsx` currently reaches into pane state for other shortcuts like `j`/`k`).
- Files: `web-app/src/app/page.tsx`

##### Task 5.2.1b: Test for the `b` shortcut and its text-input guard (~4 min)
- Simulate a `b` keydown with `document.body` focused → assert view mode toggles; simulate a
  `b` keydown with a focused `<input>` → assert no toggle and the character types normally
  (jsdom won't actually insert text, but assert the toggle callback was **not** invoked).
- Files: `web-app/src/app/__tests__/page.viewToggleShortcut.test.tsx` (new, or nearest
  existing `page.tsx` keyboard test file)

---

## Phase 6: Swimlanes, Search, Bulk-Select Parity

### Epic 6.1: Grouping-strategy swimlanes crossed with status columns

#### Story 6.1.1: Selecting a grouping strategy adds swimlane rows (AC6)
**As a** user, **I want** to see my sessions grouped by branch/tag/etc. *and* by status
simultaneously, **so that** I get both mental models without losing either.
**Acceptance Criteria**:
- Selecting `GroupingStrategy.Branch` renders one row per branch, each containing the same 4
  status columns.
  - *Given* sessions on branches `"feature/login"` (2 sessions: 1 Running, 1 Paused) and
    `"main"` (1 session: Complete), *When* the user selects `GroupingStrategy.Branch` as the
    swimlane axis, *Then* `SessionBoard` renders two swimlane rows labeled "feature/login" and
    "main", each with its own Running/Needs Review/Paused/Complete column set, and the
    "feature/login" row's Running and Paused columns each show a count of "1".
- `Tag` grouping's multi-membership renders the same card in every matching row.
  - *Given* session `sess-1` has tags `["frontend", "urgent"]`, *When*
    `GroupingStrategy.Tag` is selected, *Then* `sess-1`'s `BoardCard` renders in both the
    "frontend" row and the "urgent" row (same session ID, two DOM instances), and dragging
    either instance mutates the same underlying session's status (column axis), never its row
    membership.
**Files**: `web-app/src/components/sessions/SessionBoard.tsx`,
`web-app/src/components/sessions/BoardSwimlane.tsx` (new)

##### Task 6.1.1a: Add swimlane derivation using `groupSessions` (~5 min)
- In `SessionBoard.tsx`, when `groupingStrategy !== GroupingStrategy.None`, call
  `groupSessions(sortedSessions, groupingStrategy)` (already returns `GroupedSessions[]` with
  multi-membership support built in for `Tag`) to get the row list; for each
  `GroupedSessions` entry, bucket *its* `sessions` array into the 4 `BoardColumnKey`s exactly
  as the default (ungrouped) path already does.
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 6.1.1b: Create `BoardSwimlane.tsx` — a labeled row of 4 columns (~5 min)
- A thin component: row label (from `GroupedSessions.displayName`) + one `BoardColumn` per
  `BoardColumnKey`, reusing `BoardColumn` unmodified (it doesn't need to know it's inside a
  swimlane).
- Files: `web-app/src/components/sessions/BoardSwimlane.tsx`

##### Task 6.1.1c: Test multi-row rendering and Tag multi-membership (~5 min)
- Encodes both GWT examples above directly.
- Files: `web-app/src/components/sessions/SessionBoard.test.tsx`

### Epic 6.2: Search parity across columns

#### Story 6.2.1: Instant search filters cards in every column (AC7)
**As a** user, **I want** typing in the search box to filter the board the same way it
filters the list, **so that** the two views feel like one consistent tool.
**Acceptance Criteria**:
- *Given* `searchQuery = "login"` matches 2 of 10 sessions, *When* `SessionBoard` re-renders
  (consuming `filteredSessions` from the same `useFilteredGroupedSessions` call `SessionList`
  uses), *Then* only those 2 sessions' cards appear across all 4 columns combined, and every
  column's count badge reflects only its filtered subset.
**Files**: none new — this is a direct consequence of Phase 1's extraction + Phase 2's
bucketing already operating on `filteredSessions`, not the raw `sessions` prop.

##### Task 6.2.1a: Verify (and test) that `SessionBoard` buckets from `filteredSessions`, not `sessions` (~3 min)
- Confirm `SessionBoard.tsx`'s column-bucketing call site reads from
  `useFilteredGroupedSessions(...).filteredSessions`, not the raw `sessions` prop (a likely
  copy-paste mistake to guard against explicitly, since both are in scope at that call site).
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 6.2.1b: Add a search-filtering test (~4 min)
- Render `SessionBoard` with a search query pre-set, assert only matching sessions'
  `BoardCard`s appear and column counts reflect the filtered set.
- Files: `web-app/src/components/sessions/SessionBoard.test.tsx`

### Epic 6.3: Bulk-select parity + multi-select drag

#### Story 6.3.1: Bulk select works across columns; bulk actions apply to the full selection (AC8)
**As a** user, **I want** to select cards from multiple columns and bulk-pause them,
**so that** I don't have to repeat an action once per column.
**Acceptance Criteria**:
- *Given* the user selects one card each from "Running" and "Needs Review" (`BoardSelection
  = {"sess-1", "sess-2"}`), *When* they click `BulkActions`' "Pause" action, *Then*
  `onPauseAll` is invoked once and results in both `sess-1` and `sess-2` receiving a
  `updateSession({status: PAUSED})` call, and `BulkActions`' `selectedCount` prop reads `2`
  throughout, independent of which columns those 2 sessions are in.
**Files**: `web-app/src/components/sessions/SessionBoard.tsx`,
`web-app/src/components/sessions/BoardCard.tsx`

##### Task 6.3.1a: Add cross-column `selectedSessions: Set<string>` state to `SessionBoard` (~4 min)
- Mirror `SessionList.tsx`'s `selectMode`/`selectedSessions` state shape exactly (same
  variable names/semantics), computing `activeSelection = selectedSessions ∩
  filteredSessionIds` the same way `SessionList.tsx:639-646` does, so the intersection-with-
  filtered-set behavior (selection survives filter changes) is identical across both views.
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 6.3.1b: Thread selection props into `BoardCard` and render `BulkActions` (~4 min)
- Pass `isSelected`/`onToggleSelect` into `BoardCard` → `SessionCard` (already supports these
  props per `SessionCardProps` at `SessionCard.tsx:120-122`); render `<BulkActions
  selectedCount={activeSelection.size} totalCount={filteredSessions.length} ... />` unchanged,
  identical to how `SessionList` already renders it.
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 6.3.1c: Multi-select drag — moving a selected card moves the whole selection (~5 min)
- In `attemptColumnMove` (Task 4.1.1a), check whether the dragged session ID is a member of
  the current `activeSelection`; if so, fan out `attemptColumnMove`-equivalent RPC calls for
  every ID in `activeSelection` targeting the same `toColumn`, not just the single dragged ID
  (per the Pattern Decision above — a client-side loop, no batch RPC).
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 6.3.1d: Tests for cross-column selection and multi-select drag fan-out (~5 min)
- One test selecting cards across 2 columns and asserting `BulkActions`' `selectedCount`; one
  test dragging a selected card and asserting the mutation fired once per selected ID.
- Files: `web-app/src/components/sessions/SessionBoard.test.tsx`

---

## Phase 7: Responsive / Mobile CSS

### Epic 7.1: Board layout under the 768px breakpoint (AC10)

#### Story 7.1.1: Columns are usable on mobile without relying on drag
**As a** mobile user, **I want** the board to be legible and operable without a mouse or
reliable drag gestures, **so that** the feature isn't desktop-only in practice.
**Acceptance Criteria**:
- *Given* a viewport width of 375px, *When* `SessionBoard` renders, *Then* columns are
  horizontally scrollable (one column's width ≈ viewport width, `scroll-snap` between them)
  rather than all 4 squeezed into view, and `MoveToMenu` (not drag) is the primary
  documented/expected interaction path at this width (drag remains technically available via
  `TouchSensor` but is not required).
**Files**: `web-app/src/components/sessions/SessionBoard.css.ts`,
`web-app/src/components/sessions/BoardColumn.css.ts`

##### Task 7.1.1a: Add the `768px` breakpoint to `SessionBoard.css.ts` (~4 min)
- Add a `"@media": { "(max-width: 768px)": {...} }` block to the `board` style: switch from
  `gap: vars.space["4"]` to `vars.space["3"]` (matching `SessionList.css.ts:1-18`'s exact
  scaling pattern) and add `scrollSnapType: "x mandatory"`.
- Files: `web-app/src/components/sessions/SessionBoard.css.ts`

##### Task 7.1.1b: Make each `BoardColumn` scroll-snap and near-full-width under 768px (~4 min)
- Add `scrollSnapAlign: "start"` and a mobile-width override (e.g. `calc(100vw - 2 * spacing)`
  under the same breakpoint) to `BoardColumn.css.ts`'s `column` style.
- Files: `web-app/src/components/sessions/BoardColumn.css.ts`

##### Task 7.1.1c: Verify `touch-action: none` is present on the drag handle only, not the whole card (~3 min)
- Confirm (from Task 2.2.1b) that `touch-action: none` is scoped to `BoardCard`'s drag-handle
  element specifically — the card body itself must retain normal touch scrolling behavior so
  a user scrolling the column doesn't accidentally trigger a drag from tapping the card body.
- Files: `web-app/src/components/sessions/BoardCard.css.ts`

---

## Phase 8: Tests, Registry, Docs

### Epic 8.1: Full unit/component test pass

#### Story 8.1.1: Run the complete frontend test suite for touched areas
**Acceptance Criteria**:
- `cd web-app && npx jest --no-coverage --testPathPatterns="SessionList|SessionBoard|BoardColumn|BoardCard|MoveToMenu|useFilteredGroupedSessions|useSessionViewMode|lib/board"` passes.
**Files**: none new (verification)

##### Task 8.1.1a: Run the full targeted test pass and fix any failures (~5 min, repeat as needed)
- Files: none (verification task; fixes land in whichever file the failure points to)

### Epic 8.2: E2E coverage

#### Story 8.2.1: Playwright spec for the board view happy path
**As a** developer, **I want** an E2E test proving the toggle, columns, and a drag work
end-to-end, **so that** this feature has the same CI coverage bar as every other user-facing
feature per `.claude/rules/feature-registry.md`.
**Acceptance Criteria**:
- A new spec starting with `// @feature session-board-view` exercises: toggling to Board via
  the header control, verifying 4 columns render with count badges, performing one drag (via
  Playwright's mouse-based drag simulation) from Running to Paused, and asserting the card
  lands in the Paused column with the underlying session's status updated (verified via a
  subsequent API read-back, not just DOM state).
**Files**: `tests/e2e/session-board-view.spec.ts` (new), `tests/e2e/pages/` (new page helper
if the interaction is complex enough to warrant one, per e2e conventions)

##### Task 8.2.1a: Write the toggle + column-rendering assertions (~5 min)
- Locators: `data-testid`/ARIA roles only, no CSS classes, per
  `.claude/rules/e2e-test-conventions.md`. No `waitForTimeout` — use `toHaveText`/
  `waitForSelector` on the count badges.
- Files: `tests/e2e/session-board-view.spec.ts`

##### Task 8.2.1b: Write the drag-and-drop assertion (~5 min)
- Use Playwright's `dragTo()` or manual `mouse.down/move/up` sequence on the drag-handle
  locator; assert the card's new column via `data-testid` on `BoardColumn`.
- Files: `tests/e2e/session-board-view.spec.ts`

##### Task 8.2.1c: Run the new spec locally against the isolated test server (~3 min)
- `cd tests/e2e && npx playwright test session-board-view.spec.ts`
- Files: none (verification)

### Epic 8.3: Feature registry

#### Story 8.3.1: Register the new frontend feature and backend marker
**Acceptance Criteria**:
- Per `.claude/rules/feature-registry.md`: a new `docs/registry/features/frontend/session-board-view.json`
  exists with `tested: true` and `testIds` referencing the new Playwright `describe` block(s);
  the existing `// +api: session:update` marker on `UpdateSession` needs no new entry (no new
  RPC was added), but its per-feature file's `testIds` should gain the two new Go test names
  from Task 0.1.1c.
**Files**: `docs/registry/features/frontend/session-board-view.json` (new),
`docs/registry/features/backend/session-update.json` (or confirmed equivalent filename)

##### Task 8.3.1a: Create the frontend registry entry (~3 min)
- Files: `docs/registry/features/frontend/session-board-view.json`

##### Task 8.3.1b: Update the backend `session:update` registry entry's `testIds` (~2 min)
- Files: `docs/registry/features/backend/session-update.json` (confirm exact filename via
  `grep -rl "session:update" docs/registry/features/backend/` at implementation time)

##### Task 8.3.1c: Run `make registry-generate` and commit changed generated files (~2 min)
- Files: generated aggregate files under `docs/registry/` (per the Makefile target)
