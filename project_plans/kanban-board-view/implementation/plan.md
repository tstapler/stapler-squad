# Implementation Plan: kanban-board-view

**Feature**: A List/Board view toggle on the sessions dashboard, rendering sessions as a four-column kanban board (Needs Review / Running / Paused / Complete) with drag-and-drop and a keyboard-accessible "Move to…" menu that trigger existing session-control RPCs.
**Date**: 2026-08-06
**Status**: Ready for implementation
**ADRs**:
- [ADR-001 — Adopt `@dnd-kit` for board drag-and-drop](../decisions/ADR-001-adopt-dnd-kit-for-board-drag-and-drop.md)
- [ADR-002 — Drag-to-"Complete" calls `ArchiveSession`, not `DeleteSession`](../decisions/ADR-002-drag-to-complete-uses-archive-not-delete.md)
- [ADR-003 — Board column resolution: one source of truth and an ordered precedence cascade](../decisions/ADR-003-board-column-resolution-and-precedence.md)

**System type**: Client-side React UI feature. **No new backend RPCs, no proto changes, no `make proto-gen`, no schema/migration.** All state transitions reuse RPCs that exist today in `web-app/src/lib/hooks/useSessionService.ts`.

---

## Corrections to Phase 2 research (verified during planning)

Three research statements were checked against the code and are wrong or incomplete. The plan below uses the corrected facts.

1. **`SessionList` is not rendered from `app/page.tsx`.** `research/architecture.md` §1/§7 says `app/page.tsx` renders `<SessionList>` and should own the list/board toggle. VERIFIED false: `web-app/src/app/page.tsx:456` renders `<PaneTilingContainer>`; the only production render site of `<SessionList>` is `SessionListPaneBody` in **`web-app/src/components/pane/PaneSplitRenderer.tsx:167`** (`rg -n "<SessionList" -g '*.tsx'` → one production hit). The toggle therefore lives in `SessionListPaneBody`, per-pane, and reuses the existing `storageKeyPrefix={`pane-${pane.id}.`}` convention (`PaneSplitRenderer.tsx:192`).
2. **`sessionsSlice` has no `liveVersion` analog.** `research/architecture.md` §5 asks whether one exists so `BacklogBoard.tsx`'s transition animation can be reused verbatim. VERIFIED absent: `rg -n "liveVersion" web-app/src/lib/store/` matches only `backlogItemsSlice.ts:19,24,57,83`. The board's enter/exit animation therefore diffs *resolved column membership* across renders (Story 2.1.3) rather than gating on a version counter. Safe: a bulk resnapshot that does not change any card's column produces no animation, which is the property `liveVersion` existed to guarantee for the backlog board.
3. **`archiveSession` is missing from the `SessionServiceContextValue` type.** `useSessionService` returns it (`useSessionService.ts:1108`) and `GlobalSessionServiceProvider` passes the whole object through (`SessionServiceContext.tsx:103`), but the interface at `SessionServiceContext.tsx:21-50` does not declare it — so it is not reachable from a consumer without a type error. ADR-002 depends on it. Widening the interface is Task 1.2.2c.

A fourth item is INFERRED, not verified, and is scheduled as an explicit runtime check rather than assumed: `PaneLeafComponent` sets `draggable={hasSplits}` with native HTML5 `onDragStart`/`onDrop` pane-swap handlers (`PaneSplitRenderer.tsx:243-246`). Standard HTML behaviour is that `draggable=true` on an ancestor makes descendant content initiate a native drag, which would fight `@dnd-kit`'s `PointerSensor`. Task 3.1.3c verifies this in a browser and applies `draggable={false}` on the board card root if confirmed.

---

## Domain Glossary

*(Ubiquitous language. These exact names must be used in code, tests, and comments. Where a name already exists in the codebase it is marked "existing" — do not rename or shadow it.)*

| Term | Definition | Notes |
|------|-----------|-------|
| `BoardColumnKey` | String-literal union `"needsReview" \| "running" \| "paused" \| "complete"` naming the four fixed board columns. | Sum type, not `string`. Enables exhaustive `switch` in `boardTransitions.ts`. |
| `BoardColumn` | `{ key: BoardColumnKey; label: string; emptyLabel: string }` — the static descriptor for one column. | Mirrors `BacklogBoard.tsx:38-44`'s `COLUMNS` shape. |
| `BOARD_COLUMNS` | The ordered `readonly BoardColumn[]` constant defining column order left-to-right. | Order per ADR-003 §3. Reordering this array is the entire cost of changing column order. |
| `resolveBoardColumn` | Pure `(session: Session, needsReviewIds: Set<string>) => BoardColumnKey`. Total over `SessionStatus`. | ADR-003 §2. Analogous to `stageOf()` in `BacklogBoard.tsx:21-24`. |
| `needsReviewIds` | `Set<string>` of session IDs considered "needs review": review-queue items ∪ pending approvals, minus `clearedSessions`. | ADR-003 §1. Produced by `useNeedsReviewSessionIds`. |
| `useNeedsReviewSessionIds` | Hook returning `needsReviewIds`. Sole source of truth for the Needs Review column. | `web-app/src/lib/hooks/useNeedsReviewSessionIds.ts` |
| `BoardMoveIntent` | `{ sessionId: string; from: BoardColumnKey; to: BoardColumnKey }` — a requested card move, before legality is checked. | Value object; carries `from` so stale-state re-validation is possible at drop time. |
| `BoardMoveVerb` | Sum type `"pause" \| "resume" \| "resumeHibernated" \| "archive"` — the RPC a legal `BoardMoveIntent` resolves to. **`null` is not a member of this union**; illegality is expressed by the resolver's return type, not by a "nothing" verb. | Never `"delete"` (ADR-002). Folding `null` in would make `pending[id] === null` and `pendingVerb === null` representable alongside `undefined` — two encodings of "not pending," which is the illegal state the sum type exists to forbid. |
| `resolveMoveVerb` | Pure `(intent: BoardMoveIntent, session: Session) => BoardMoveVerb \| null`. Total over the triple `(intent.from, intent.to, session.status)` — every combination has a defined answer, and `null` means "this move is illegal." Takes `session` so it can pick `resumeHibernated` over `resume` and so it can reject a move whose target status the session already holds. | `web-app/src/lib/board/boardTransitions.ts` |
| `BoardMoveOutcome` | Discriminated union `{ kind: "applied" } \| { kind: "rejected"; reason: BoardMoveRejection } \| { kind: "failed"; message: string }`. | Makes "nothing happened and the user wasn't told" unrepresentable. |
| `BoardMoveRejection` | Sum type `"illegal-transition" \| "stale-source-column" \| "session-gone" \| "readonly-axis"`. | One user-facing message per variant; no free-form strings. |
| `useBoardMove` | Hook exposing `move(intent: BoardMoveIntent): Promise<BoardMoveOutcome>` plus `pending: Partial<Record<string, BoardMoveVerb>>` (sessionId → in-flight verb; absent key = not pending). | `web-app/src/lib/hooks/useBoardMove.ts`. `pending` mirrors `BacklogBoard.tsx:30`'s `pending` prop. `Partial<Record<…>>` rather than `Record<…>` so "not pending" has exactly one encoding (`undefined`). `SessionBoardCard`'s prop is correspondingly `pendingVerb?: BoardMoveVerb`. |
| `SessionBoard` | The board view component: column layout, drag wiring, transitions. Sibling of `SessionList`, not a wrapper. | `web-app/src/components/sessions/SessionBoard.tsx` |
| `SessionBoardColumn` | One rendered column: header with label + count, scrollable card area, empty state, overflow footer. | Inside `SessionBoard.tsx` (not a separate file — single consumer). |
| `SessionBoardCard` | The condensed board card: title, program icon, status/sub-status chip, one attention badge, grip handle, "Move to…" menu. | `web-app/src/components/sessions/SessionBoardCard.tsx`. Deliberately **not** `SessionCard` (ADR rationale in Pattern Decisions). |
| `DashboardViewMode` | String-literal union `"list" \| "board"`. Owned by `SessionListPaneBody`, persisted per pane. | Distinct from the existing `viewMode: "card" \| "row"` prop on `SessionList` (existing) — do not overload it. |
| `SwimlaneAxis` | `{ kind: "status" } \| { kind: "grouping"; strategy: GroupingStrategy }` — what the board's lanes represent. | Sum type. `kind: "grouping"` is read-only per ADR-003 §4. |
| `COLUMN_RENDER_CAP` | `50` — max cards rendered per column before an overflow footer replaces the remainder. | Replaces per-column virtualization in v1; see Pattern Decisions. |
| `useSessionFilters` | Extracted hook owning search / status / category / tag / hidePaused / showArchived / filterNeedsApproval / sort + their localStorage persistence, returning `filteredSessions` and `sortedSessions`. | `web-app/src/lib/hooks/useSessionFilters.ts`. Consumed by **both** `SessionList` and `SessionBoard`. |
| `useSessionSelection` | Extracted hook owning `selectMode`, `selectedSessions`, `activeSelection`, `bulkFeedback`, `showFeedback`. | `web-app/src/lib/hooks/useSessionSelection.ts`. Consumed by both views. |
| `GroupingStrategy` | *(existing)* Enum of the 8+ grouping strategies. | `web-app/src/lib/grouping/strategies.ts:7-18` — reused unchanged. |
| `groupSessions` | *(existing)* Pure `(sessions, strategy, options?) => GroupedSessions[]`. | `strategies.ts:69` — reused unchanged for `SwimlaneAxis.kind === "grouping"`. |
| `SessionStatus` | *(existing)* Proto enum, 9 values. | `web-app/src/gen/session/v1/types_pb.ts` — read only, never extended. |
| `#bulk-feedback-live` | *(existing)* The persistent visually-hidden `role="status" aria-live="polite"` region. | `SessionList.tsx:1167`. The board announces moves through this same node, not a new one. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| **Overall delivery shape** | Two-stage: read-only board + "Move to…" menu ships first (Phase 2), drag-and-drop layers on (Phase 3) | Incremental delivery | **(a)** Full `@dnd-kit` drag-and-drop in one shipment | (a)'s strength: delivers the headline interaction whole. Its weakness — a new dependency, a new component, a 1601-line-file refactor, and WCAG compliance all land together, so any single blocker (dnd×pane-drag collision, portal/z-index) stalls the entire board. Two-stage makes the mandatory accessible path the foundation instead of an afterthought, and Stage 1 alone satisfies Success Metric #1. |
| **Overall delivery shape** | *(as above)* | | **(c)** Generify `BacklogBoard.tsx` into a shared `<StatusBoard>` used by both backlog and sessions | (c)'s strength: one board implementation, no duplicated column/transition machinery. Its weakness — sessions derive column membership from three live signals while backlog items have one `status` field, so the generic would need a strategy parameter on day one against exactly two callers. That is the "speculative interface / unjustified generic" smell in `.claude/rules/interface-pollution-checklist.md` §1/§5, and it puts regression risk on an already-shipped board. **Copy the pattern by reading it, do not extract a base component.** |
| `resolveBoardColumn` | Pure total function over a sum type (type-driven-design) | Parse-don't-validate | `switch` on `session.status` with no default | A non-total mapping makes a session with an unmapped status render in no column — the exact BUG-037 failure already fixed once for the backlog board (`BacklogBoard.tsx:13-24`). A catch-all rule 4 makes vanishing unrepresentable. |
| `BoardColumnKey`, `BoardMoveVerb`, `BoardMoveRejection` | String-literal union sum types | type-driven-design | Raw `string` | Compiler-enforced exhaustive `switch` in `resolveMoveVerb` and in the rejection→message mapping; a new column or verb becomes a compile error at every site that must handle it. |
| `BoardMoveOutcome` | Discriminated union return, not `boolean`/throw | type-driven-design | `Promise<boolean>` | `boolean` collapses "illegal move," "session vanished," and "RPC failed" into one value, so the caller cannot pick the right message. The union makes silent failure unrepresentable. |
| `useBoardMove` | Transaction Script (PoEAA) — one flat orchestration per move | Fowler | Domain Model / a `BoardMoveService` class | The logic is: resolve verb → re-validate → call one RPC → map outcome. No cross-aggregate rules, no invariants to protect. A class here would be a "forwarding-only wrapper" (`interface-pollution-checklist.md` §4). |
| `resolveMoveVerb` | Explicit legality guards over the `(from, to, status)` triple, **then** an exhaustive `switch` on `to` for the verb | GoF Strategy, degenerate form | A `Record<BoardColumnKey, () => Promise<…>>` handler map keyed on `to` alone | Legality is a relation over the pair, not a property of the destination: `{from: "complete", to: "paused"}` and `{from: "needsReview", to: "running"}` are both illegal despite legal-looking destinations. A `to`-only map (or a `to`-only `switch`) cannot express that, loses the `session`-dependent branch (`HIBERNATED` → `resumeHibernated` vs `resume`, ADR-003), and loses exhaustiveness checking. Guards + `switch` give all three. |
| `SessionBoard` vs `SessionList` | Sibling components chosen by a ternary at the call site | `research/architecture.md` §7 | A `<SessionViewContainer>` wrapper whose only job is the if/else | A container with one branch and no other behaviour is a forwarding-only wrapper (`interface-pollution-checklist.md` §4). The ternary lives in `SessionListPaneBody` where the data already is. |
| Shared state | Extract `useSessionFilters` + `useSessionSelection`, **refactor `SessionList` to consume them too** | `research/architecture.md` §7 | Let `SessionBoard` keep its own parallel `useState` calls | Parallel state is precisely the "duplicated with divergent behavior" outcome `requirements.md:18` prohibits. Refactoring `SessionList` (not just avoiding growing it) is what actually shrinks the 1601-line hotspot. |
| `SessionBoardCard` | Purpose-built condensed card | `research/ux.md` §1 | Reuse `SessionCard.tsx` (893 lines) at column width | `SessionCard` carries inline title edit, tag editor, diff stats, memory badge, and an expandable terminal snapshot. At column width that is either unreadably cramped or so tall the board shows ~2 cards per column — defeating the at-a-glance goal that is the feature's entire premise. |
| Card move commit | **Pessimistic** — card shows a busy state in its origin column, moves only when `sessionsSlice` updates | `research/architecture.md` §6 | Optimistic move + snap-back on failure (recommended by `research/ux.md` §4 and `research/pitfalls.md` §6) | The two research agents disagree; architecture wins on evidence. `updateSession` (`useSessionService.ts:301-330`) dispatches `upsertSession` **only after** the RPC resolves — there is no speculative store write anywhere in this call path today. Optimistic move would be a new pattern, and it is the pattern that requires the snap-back animation, the stale-announcement retraction, and the flicker-vs-WebSocket-push interaction that `research/pitfalls.md` §1 and §4 both flag. Pessimistic sidesteps all three: nothing moved, so nothing must be un-moved. Cost: a visible round-trip delay against a local server. |
| Column overflow | Fixed `COLUMN_RENDER_CAP = 50` + "+N more — switch to list view" footer | — | Per-column virtualization with `react-virtuoso`/`@tanstack/react-virtual` | `research/stack.md` reports **no verified precedent** for `GroupedVirtuoso` + `dnd-kit`, and that both windowing and drag animation compete for CSS `transform`. A hard cap is ~10 lines, deterministic, and satisfies the perf SLO by construction. Revisit only if a column is empirically observed above 50 — record the observation, don't pre-build for it. |
| Drag overlay | `@dnd-kit` `<DragOverlay>` + explicit `createPortal(…, document.body)` if the library's own portal is not confirmed | `.claude/rules/css-architecture.md` | In-place `position: fixed` ghost | An ancestor `transform`/`filter`/`will-change` silently breaks `fixed`. Model: `SessionPeekModal.tsx`, `QuickOpenPalette.tsx`. |
| Touch | `PointerSensor` with `activationConstraint`, **drag disabled on coarse pointers**; "Move to…" menu is the universal move path on every form factor | `research/ux.md` §3, `requirements.md:52` | Full touch drag / desktop-only-with-no-mobile-path | Full touch drag reopens the scroll-vs-drag rabbit hole inside a horizontally scrolling board. Desktop-only with no fallback violates the mobile+desktop UX requirement. Menu-everywhere + drag-on-fine-pointer gives mobile a complete, tested path at near-zero extra cost, because the menu must exist for WCAG anyway. |
| Move announcements | Write into the existing `#bulk-feedback-live` node | `research/ux.md` §0 | A new board-scoped live region | That node already solves the survive-unmount-mid-announcement problem a column re-render will hit (`SessionList.tsx:1166-1167`). Two live regions racing is a known screen-reader failure mode. |
| Enter/exit column animation | Copy `BacklogBoard.tsx:168-284`'s exiting/entering ref+timer machinery, keyed on **resolved column** instead of `liveVersion` | `research/build-vs-buy.md` "Option 4" | Import/extract it from `BacklogBoard.tsx` | `sessionsSlice` has no `liveVersion` (see Corrections §2), so the trigger condition genuinely differs. Extracting a shared animation hook against two differently-triggered callers is premature. Copy ~60 lines; revisit if a third board appears. |

---

## Migration Plan

**Omitted — no schema, proto, or data changes.** No `.proto` edit, no `make proto-gen`, no ent schema regeneration, no backend Go change. The entire diff is `web-app/` plus one registry JSON file and one e2e spec.

---

## Observability Plan

Per `requirements.md:66` — client-side only, no new backend surface, no new metrics or alerts.

- **Logs**: every `BoardMoveOutcome` of kind `"failed"` logs `console.error("[useBoardMove] move failed:", { sessionId, from, to, verb, error })` from `useBoardMove.ts`, matching the existing `console.error("[useSessionService] …")` shape at `useSessionService.ts:395`. Outcomes of kind `"rejected"` log at `console.warn` with the same payload plus `reason` — rejections are expected (stale state, illegal target), not errors.
- **User-visible surfacing**: every non-`"applied"` outcome produces (a) a toast via the existing `useNotifications()` channel and (b) an announcement written to `#bulk-feedback-live`. No outcome is silent — enforced structurally by `BoardMoveOutcome` being a union the caller must exhaustively handle.
- **Metrics**: none. No new operation exceeds 100 ms server-side; every RPC used already exists and is already instrumented where the backend instruments it.
- **Alerts**: no new alerts required.

## Risk Control

- **Feature flag**: not gated. The board is unreachable unless the user actively toggles to it, and falling back is one click or one `b` keypress — `requirements.md:69` explicitly rules a flag unnecessary. The one refactor with real blast radius (Epic 1.1, which touches `SessionList.tsx`) is gated instead by "the four existing `SessionList.*.test.tsx` suites pass unchanged," which is a stronger guarantee than a flag would give.
- **Rollback procedure**: standard revert via PR revert commit. No proto, no schema, no persisted-format change to unwind. The only persisted artifact is a `stapler-squad-dashboard-view-mode` localStorage value, which a reverted build simply ignores.
- **Staged rollout**: full rollout on merge.
- **Blast-radius containment for Epic 1.1**: the hook extractions are pure lift-and-shift — the acceptance criterion is byte-identical behaviour proven by the pre-existing test suites, with no new behaviour introduced in the same story. Any behaviour change discovered during extraction is a separate commit.

## Unresolved Questions

- [ ] **Column order deviates from `requirements.md:38`.** ADR-003 §3 orders columns Needs Review → Running → Paused → Complete; the requirements text lists Running first. Confirm or veto. — blocks **Story 2.1.2** — owner: requirements author. *Cost of reversal: reorder the `BOARD_COLUMNS` array in `web-app/src/lib/board/boardColumns.ts`. No other file changes.*
- [ ] **Archived sessions disappear from the board on drag-to-Complete while `showArchived` is off.** ADR-002 Consequences documents the behaviour and Story 2.3.3 announces it, but confirm this is acceptable versus auto-enabling `showArchived` after a Complete move. — blocks **Story 2.3.1** — owner: requirements author.
- [ ] **Does the pane-level `draggable={hasSplits}` on `PaneSplitRenderer.tsx:243` actually intercept `PointerSensor` drags on descendant cards?** INFERRED from standard HTML `draggable` inheritance, not verified. — blocks **Story 3.1.3** — owner: implementer, resolved by the browser check in Task 3.1.3c (not by reading more code).
- [ ] **Does `@dnd-kit`'s `<DragOverlay>` portal to `document.body` by default in v6.3.1, or does it render in place?** ADR-001 assumes it portals; `.claude/rules/css-architecture.md` compliance depends on it. — blocks **Story 3.1.3** — owner: implementer, resolved by inspecting the rendered DOM in Task 3.1.3b.

## Dependency Visualization

```
Phase 1 — Foundation (no user-visible change)
┌─────────────────────────────────────────────────────────────┐
│ 1.1.1 useSessionFilters ──┐                                 │
│ 1.1.2 useSessionSelection ─┼──> SessionList refactor green  │
│ 1.1.3 useNeedsReviewSessionIds ─┐                           │
│ 1.2.1 boardColumns.ts <─────────┘                           │
│ 1.2.2 boardTransitions.ts + SessionServiceContext widening  │
└──────────────────────────────┬──────────────────────────────┘
                               v
Phase 2 — Read-only board + toggle + Move-to menu  ◀── SHIPPABLE
┌─────────────────────────────────────────────────────────────┐
│ 2.1.1 SessionBoardCard                                      │
│        v                                                    │
│ 2.1.2 SessionBoardColumn ──> 2.1.3 SessionBoard shell       │
│                                     v                       │
│ 2.2.1 view toggle + persistence ──> 2.2.2 `b` shortcut      │
│                                     v                       │
│ 2.3.1 useBoardMove ──> 2.3.2 Move-to menu ──> 2.3.3 a11y    │
└──────────────────────────────┬──────────────────────────────┘
                               v
Phase 3 — Drag and drop + parity
┌─────────────────────────────────────────────────────────────┐
│ 3.1.1 dep + zIndex ──> 3.1.2 DndContext ──> 3.1.3 overlay   │
│                                   v                         │
│                    3.1.4 drop legality ──> 3.1.5 stale check│
│ 3.2.1 search ─┐                                             │
│ 3.2.2 bulk    ─┼── (independent of 3.1.*, parallelizable)   │
│ 3.2.3 axis    ─┘                                            │
└──────────────────────────────┬──────────────────────────────┘
                               v
Phase 4 — Tests, registry, docs
  4.1.1 unit ─┬─> 4.1.3 e2e
  4.1.2 comp ─┘
  4.2.1 feature registry     (independent, any time after 2.1.3)
  4.2.2 rules/docs note      (independent)
```

Critical path: **1.1.3 → 1.2.1 → 2.1.2 → 2.1.3 → 2.3.1 → 3.1.2 → 3.1.4 → 4.1.3**.
Stories 3.2.1–3.2.3 and 4.2.1–4.2.2 are off the critical path.

---

## Phase 1: Foundation — shared state extraction and the column model

No user-visible change lands in this phase. Its output is the shared hooks and pure functions Phase 2 builds on.

### Epic 1.1: Extract shared session-view state out of `SessionList.tsx`

**Goal**: Both views read one copy of search/filter/sort and selection state, so board and list cannot diverge — and `SessionList.tsx` shrinks rather than merely stopping growing.

#### Story 1.1.1: Extract `useSessionFilters`
**As a** developer, **I want** search/filter/sort state and its localStorage persistence in one hook, **so that** `SessionBoard` and `SessionList` filter identically without duplicated `useState`.

**Acceptance Criteria**:
- `useSessionFilters({ sessions, storageKeyPrefix })` returns every filter value + setter currently held in `SessionList.tsx:317-355`, plus derived `filteredSessions`, `sortedSessions`, `hasActiveFilters`, `categories`, `tags`.
  - *Given* `sessions` containing a `Session` with `title: "fix-login-bug"` and one with `title: "add-metrics"`, and `storageKeyPrefix: "pane-p1."`, *When* the caller invokes `setSearchQuery("login")`, *Then* `filteredSessions` has `length === 1` and `filteredSessions[0].title === "fix-login-bug"`, and `window.localStorage.getItem("pane-p1.stapler-squad-search-query")` equals `"\"login\""`.
- The hook preserves the exact filter semantics of `SessionList.tsx:530-585`, including the `pendingDeleteIds` exclusion, the `filterNeedsApproval` `SubStatus.NEEDS_APPROVAL || SubStatus.INPUT_REQUIRED` condition, and the `showArchived`/`archivedAt` rule.
  - *Given* a `Session` with `status: SessionStatus.ACTIVE`, `subStatus: SubStatus.INPUT_REQUIRED`, *When* `filterNeedsApproval` is `true`, *Then* that session is present in `filteredSessions` (matching `SessionList.tsx:572`'s `||` branch, not only `NEEDS_APPROVAL`).
- `SessionList.tsx` consumes the hook and its four existing test suites pass with **zero test-file edits**.
  - *Given* the unmodified test files `SessionList.archived.test.tsx`, `SessionList.collapse.test.tsx`, `SessionList.mobile.test.tsx`, `SessionList.tokenCostSort.test.tsx`, *When* `cd web-app && npx jest --no-coverage --testPathPatterns="SessionList"` runs against the refactored component, *Then* every test passes and `git diff --stat web-app/src/components/sessions/__tests__/` reports no changes.

**Files**: `web-app/src/lib/hooks/useSessionFilters.ts` (new), `web-app/src/components/sessions/SessionList.tsx`

##### Task 1.1.1a: Create the hook module with state + persistence (~5 min)
- Create `web-app/src/lib/hooks/useSessionFilters.ts`.
- Move `BASE_STORAGE_KEYS`, `makeStorageKeys`, `loadFromStorage`, `saveToStorage`, and `getTimestampMs` verbatim from `SessionList.tsx:223-269` into the new module; re-export `loadFromStorage`/`saveToStorage`/`makeStorageKeys` (both `SessionList.tsx` and the new view-mode persistence in Story 2.2.1 need them).
- Move the eleven `useState` initialisers at `SessionList.tsx:317-355` and their eleven `saveToStorage` effects at `SessionList.tsx:449-504`.
- Files: `web-app/src/lib/hooks/useSessionFilters.ts`

##### Task 1.1.1b: Move the derived filter/sort/group memos (~5 min)
- Move `categories` (`SessionList.tsx:508-516`), `tags` (`:519-527`), `filteredSessions` (`:530-585`), `costById` + `useInsightsSummary` (`:588-595`), `sortedSessions` (`:598-630`), and `hasActiveFilters` (`:645`) into the hook. Accept `pendingDeleteIds: Set<string>` as a hook argument — it stays owned by `SessionList` (delete batching is list-only).
- Files: `web-app/src/lib/hooks/useSessionFilters.ts`

##### Task 1.1.1c: Rewire `SessionList.tsx` to consume the hook (~5 min)
- Replace the removed blocks with one destructuring call. Delete the now-dead imports (`useInsightsSummary`, `compareSessionsByCost`) if no longer referenced.
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="SessionList"` and paste the output into the commit body.
- Files: `web-app/src/components/sessions/SessionList.tsx`

---

#### Story 1.1.2: Extract `useSessionSelection`
**As a** developer, **I want** bulk-selection state in one hook, **so that** `BulkActions` behaves identically whether the board or the list is mounted.

**Acceptance Criteria**:
- `useSessionSelection({ filteredSessions })` returns `selectMode`, `setSelectMode`, `selectedSessions`, `toggleSession`, `activeSelection`, `handleSelectAll`, `handleClearSelection`, `bulkFeedback`, `showFeedback` — matching the behaviour at `SessionList.tsx:632-641,780-806`.
  - *Given* `selectedSessions` is `new Set(["sess-a", "sess-b"])` and `filteredSessions` contains only the session with `id: "sess-a"`, *When* the caller reads `activeSelection`, *Then* it equals `new Set(["sess-a"])` — `"sess-b"` is excluded because it is not currently visible, matching `SessionList.tsx:639-642`.
- The `Escape`-clears-selection and `Cmd/Ctrl+A`-selects-all document listeners move with the hook, keeping the `INPUT`/`TEXTAREA`/`isContentEditable` guard at `SessionList.tsx:791-792` byte-identical.
  - *Given* `selectMode === true` and focus inside an `<input>` element, *When* the user presses `Cmd+A`, *Then* `handleSelectAll` is **not** called and the browser's native select-all proceeds (the `if (inInput) return` branch at `SessionList.tsx:794`).

**Files**: `web-app/src/lib/hooks/useSessionSelection.ts` (new), `web-app/src/components/sessions/SessionList.tsx`

##### Task 1.1.2a: Create the hook with selection state and the keydown effect (~4 min)
- Create `web-app/src/lib/hooks/useSessionSelection.ts` with the state from `SessionList.tsx:632-645` and the effect at `:788-806`.
- Files: `web-app/src/lib/hooks/useSessionSelection.ts`

##### Task 1.1.2b: Rewire `SessionList.tsx` and re-run its suites (~3 min)
- Replace the removed blocks; keep `selectButtonRef` in `SessionList` and pass it into the hook so `handleClearSelection`'s focus-restore still works.
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="SessionList"`.
- Files: `web-app/src/components/sessions/SessionList.tsx`

---

#### Story 1.1.3: Extract `useNeedsReviewSessionIds`
**As a** user, **I want** the board's Needs Review column to agree with the review badge already on my session cards, **so that** the two views never tell me different things about the same session.

**Acceptance Criteria**:
- `useNeedsReviewSessionIds(): Set<string>` returns the union of review-queue and approvals session IDs minus `clearedSessions`, per ADR-003 §1.
  - *Given* `useReviewQueueContext().items` contains a `ReviewItem` with `sessionId: "sess-a"`, `useApprovalsContext().approvals` contains a `PlainApproval` with `sessionId: "sess-b"`, and `clearedSessions` is `new Set(["sess-b"])`, *When* the hook is read, *Then* it returns `new Set(["sess-a"])`.
- The returned `Set` is referentially stable across renders when its inputs are unchanged.
  - *Given* two consecutive renders where `items`, `approvals`, and `clearedSessions` are all referentially identical, *When* the hook is read in both, *Then* `first === second` is `true` (it is a `useMemo` over exactly those three dependencies) — so `SessionBoard`'s column memos do not recompute on every parent render.

**Files**: `web-app/src/lib/hooks/useNeedsReviewSessionIds.ts` (new)

##### Task 1.1.3a: Implement the hook (~3 min)
- Read `items` from `useReviewQueueContext()` (shape confirmed at `SessionList.tsx:304`) and `approvals` + `clearedSessions` from `useApprovalsContext()` (`SessionList.tsx:314`).
- `useMemo` over `[items, approvals, clearedSessions]`; build the union, then delete every id in `clearedSessions`.
- Files: `web-app/src/lib/hooks/useNeedsReviewSessionIds.ts`

##### Task 1.1.3b: Add its unit test (~4 min)
- Cover: review-only, approval-only, both, cleared-suppression, empty inputs.
- Files: `web-app/src/lib/hooks/__tests__/useNeedsReviewSessionIds.test.ts`

---

### Epic 1.2: The board column model

**Goal**: Column membership and move legality are pure, total, exhaustively-tested functions with no React dependency — so every precedence question from ADR-003 is settled by a test, not by reading a component.

#### Story 1.2.1: `boardColumns.ts` — column keys, order, and resolution
**As a** user, **I want** every session to appear in exactly one column, **so that** a session can never silently vanish from the board.

**Acceptance Criteria**:
- `resolveBoardColumn` is total: no `SessionStatus` value returns `undefined`.
  - *Given* a `Session` with `status: SessionStatus.RESTORING`, `archivedAt: undefined`, and `needsReviewIds = new Set()`, *When* `resolveBoardColumn(session, needsReviewIds)` is called, *Then* it returns `"running"` (rule 4's catch-all), not `undefined`.
- Terminal state beats a pending review (ADR-003 rule 1 over rule 2).
  - *Given* a `Session` with `id: "sess-a"`, `status: SessionStatus.STOPPED`, and `needsReviewIds = new Set(["sess-a"])`, *When* resolved, *Then* it returns `"complete"`.
- A pending review beats both Active and Paused (rule 2 over rules 3–4).
  - *Given* a `Session` with `id: "sess-a"`, `status: SessionStatus.PAUSED`, and `needsReviewIds = new Set(["sess-a"])`, *When* resolved, *Then* it returns `"needsReview"`.
- Hibernated routes to Paused, not Complete.
  - *Given* a `Session` with `status: SessionStatus.HIBERNATED` and empty `needsReviewIds`, *When* resolved, *Then* it returns `"paused"`.
- An archived-but-Active session lands in Complete.
  - *Given* a `Session` with `status: SessionStatus.ACTIVE` and `archivedAt` set to a non-zero `Timestamp`, *When* resolved, *Then* it returns `"complete"`.

**Files**: `web-app/src/lib/board/boardColumns.ts` (new)

##### Task 1.2.1a: Define `BoardColumnKey`, `BoardColumn`, `BOARD_COLUMNS` (~3 min)
- Order per ADR-003 §3: `needsReview`, `running`, `paused`, `complete`, with labels `"Needs Review"`, `"Running"`, `"Paused"`, `"Complete"` and `emptyLabel`s `"Nothing needs review"`, `"No running sessions"`, `"No paused sessions"`, `"Nothing completed yet"`.
- Add a file-header comment naming ADR-003 as the source of the order, so a future reader does not "fix" it back to the requirements-doc order.
- Files: `web-app/src/lib/board/boardColumns.ts`

##### Task 1.2.1b: Implement `resolveBoardColumn` as the four-rule cascade (~4 min)
- Rules exactly as ADR-003 §2. Rule 4 is a bare `return "running"`, not a `switch` — deliberately, so it is total.
- Export `COLUMN_RENDER_CAP = 50` from the same module.
- Files: `web-app/src/lib/board/boardColumns.ts`

---

#### Story 1.2.2: `boardTransitions.ts` — move verbs and legality
**As a** user, **I want** columns I cannot legally move a card into to be visibly rejected, **so that** I never make a gesture that silently does nothing or errors after the fact.

**Acceptance Criteria**:

`resolveMoveVerb` is **total over the triple `(intent.from, intent.to, session.status)`** — legality is a relation over the pair, never a property of `to` alone. The full legality table it must implement, over the 16 `(from, to)` pairs:

| from ↓ / to → | `needsReview` | `running` | `paused` | `complete` |
|---|---|---|---|---|
| `needsReview` | `null` (same column) | `null` (illegal — see below) | `null` (illegal) | `"archive"` |
| `running` | `null` (server-derived) | `null` (same column) | `"pause"` | `"archive"` |
| `paused` | `null` (server-derived) | `"resume"` / `"resumeHibernated"` | `null` (same column) | `"archive"` |
| `complete` | `null` (terminal) | `null` (terminal) | `null` (terminal) | `null` (same column) |

Only **five** of the sixteen pairs are legal. The three guards that produce the `null`s, evaluated before any verb lookup: `from === to`, `from === "complete"`, `to === "needsReview"`, plus the `from === "needsReview"` restriction below.

- `resolveMoveVerb` returns `"archive"` for any legal move into `"complete"`, and never `"delete"` (ADR-002).
  - *Given* `intent = { sessionId: "sess-a", from: "running", to: "complete" }` and a `Session` with `status: SessionStatus.ACTIVE`, *When* `resolveMoveVerb(intent, session)` is called, *Then* it returns `"archive"`.
- Resuming a hibernated session resolves to the hibernation-specific verb (ADR-003 rationale).
  - *Given* `intent = { sessionId: "sess-a", from: "paused", to: "running" }` and a `Session` with `status: SessionStatus.HIBERNATED`, *When* resolved, *Then* it returns `"resumeHibernated"`, not `"resume"`.
- Moving out of `"complete"` is illegal **for every target**, not only `"running"` (no one-hop un-stop RPC exists; ADR-002 "Dragging *out* of the Complete column is a rejected drop target").
  - *Given* `intent = { sessionId: "sess-a", from: "complete", to: "running" }` and a `Session` with `status: SessionStatus.STOPPED`, *When* resolved, *Then* it returns `null`.
  - *Given* `intent = { sessionId: "sess-a", from: "complete", to: "paused" }` and the same `Session`, *When* resolved, *Then* it returns `null` — **not** `"pause"`. A `to`-only switch returns `"pause"` here and would fire `updateSession(id, { status: PAUSED })` against an archived, `STOPPED` session.
- Moving into `"needsReview"` is illegal — review state is server-derived and cannot be set by the client.
  - *Given* `intent = { sessionId: "sess-a", from: "running", to: "needsReview" }`, *When* resolved, *Then* it returns `null`.
- **Moving out of `"needsReview"` is legal only to `"complete"`.** A card sits in Needs Review because `needsReviewIds.has(id)` (ADR-003 rule 2), which outranks `PAUSED`/`HIBERNATED` (rule 3) and the running catch-all (rule 4). So a `pause`/`resume` from that column would succeed at the RPC and leave `resolveBoardColumn` returning `"needsReview"` unchanged — under the pessimistic commit model the user sees a busy state and then a card that never moved, which is indistinguishable from the failure `BoardMoveOutcome` exists to make unrepresentable. Only rule 1 (`archivedAt`/`STOPPED`) outranks rule 2, so only `"complete"` produces an observable change.
  - *Given* `intent = { sessionId: "sess-a", from: "needsReview", to: "running" }` and a `Session` with `status: SessionStatus.ACTIVE`, *When* resolved, *Then* it returns `null` — **not** `"resume"`.
  - *Given* `intent = { sessionId: "sess-a", from: "needsReview", to: "paused" }` and a `Session` with `status: SessionStatus.ACTIVE`, *When* resolved, *Then* it returns `null` — **not** `"pause"`.
  - *Given* `intent = { sessionId: "sess-a", from: "needsReview", to: "complete" }` and a `Session` with `status: SessionStatus.ACTIVE`, *When* resolved, *Then* it returns `"archive"`.
- A same-column move is illegal (a no-op must not fire an RPC), for all four columns.
  - *Given* `intent = { sessionId: "sess-a", from: "running", to: "running" }`, *When* resolved, *Then* it returns `null`.
  - *Given* each of `{from: "needsReview", to: "needsReview"}`, `{from: "paused", to: "paused"}`, `{from: "complete", to: "complete"}`, *When* resolved, *Then* each returns `null`.
- `SessionServiceContextValue` declares `archiveSession`, so ADR-002's chosen RPC is reachable from a consumer without a type error.
  - *Given* a component calling `useSessionServiceContext().archiveSession("sess-a")`, *When* `cd web-app && npx tsc --noEmit` runs, *Then* it exits `0`.

**Files**: `web-app/src/lib/board/boardTransitions.ts` (new), `web-app/src/lib/contexts/SessionServiceContext.tsx`

##### Task 1.2.2a: Define `BoardMoveIntent`, `BoardMoveVerb`, `BoardMoveRejection`, `BoardMoveOutcome` (~3 min)
- All four as the sum types in the Domain Glossary.
- Files: `web-app/src/lib/board/boardTransitions.ts`

##### Task 1.2.2b: Implement `resolveMoveVerb` as legality guards over `(from, to)` followed by an exhaustive `switch` on `intent.to` (~4 min)
- Signature: `resolveMoveVerb(intent: BoardMoveIntent, session: Session): BoardMoveVerb | null`. `null` is the return type's second member, **not** a member of `BoardMoveVerb` (Domain Glossary).
- **Guards first, in this order — all four return `null` before any verb is computed:**
  1. `if (intent.from === intent.to) return null;` — a same-column move must not fire an RPC.
  2. `if (intent.from === "complete") return null;` — nothing legally moves out of Complete (ADR-002; no one-hop un-stop RPC exists). This guard, not the `switch`, is what stops `{from: "complete", to: "paused"}` resolving to `"pause"`.
  3. `if (intent.to === "needsReview") return null;` — review state is server-derived.
  4. `if (intent.from === "needsReview" && intent.to !== "complete") return null;` — per ADR-003's precedence cascade only rule 1 outranks rule 2, so `"complete"` is the only target from Needs Review that changes the resolved column. Anything else succeeds at the RPC and visibly does nothing.
- **Then** `switch (intent.to)`: `"complete"` → `"archive"`; `"paused"` → `"pause"`; `"running"` → `session.status === SessionStatus.HIBERNATED ? "resumeHibernated" : "resume"`; `"needsReview"` → unreachable after guard 3, but keep the case returning `null` so the `switch` stays exhaustive over `BoardColumnKey` and adding a fifth column is a compile error.
- Add a file comment reproducing the 16-cell legality table from Story 1.2.2 and naming the reason each `null` cell is `null`, so a future reader does not "simplify" the guards back into the `switch`.
- Add `MOVE_REJECTION_MESSAGE: Record<BoardMoveRejection, string>` in the same file so the four user-facing strings live next to the type that enumerates them.
- Files: `web-app/src/lib/board/boardTransitions.ts`

##### Task 1.2.2c: Widen `SessionServiceContextValue` with `archiveSession`/`unarchiveSession` (~2 min)
- Add `archiveSession: (id: string) => Promise<boolean>;` and `unarchiveSession: (id: string) => Promise<boolean>;` to the interface at `SessionServiceContext.tsx:21-50`. No runtime change — the provider already passes these through from `useSessionService.ts:1108-1109`.
- Files: `web-app/src/lib/contexts/SessionServiceContext.tsx`

---

## Phase 2: Read-only board, view toggle, and the accessible move path

**This phase is independently shippable.** At its end the board renders, the toggle persists, and every move is performable via the "Move to…" menu — satisfying every Success Metric and the WCAG keyboard requirement, with zero drag code.

### Epic 2.1: The `SessionBoard` component

**Goal**: A four-column board that renders live session state with counts, empty states, and cross-column transition animation.

#### Story 2.1.1: `SessionBoardCard` — the condensed board card
**As a** user, **I want** board cards dense enough that a column shows many at once, **so that** I can see my whole fleet's state without scrolling.

**Acceptance Criteria**:
- The card renders title, program indicator, status/sub-status chip, at most one attention badge, and a grip handle — and does **not** render the tag editor, diff stats, memory badge, or terminal snapshot that `SessionCard` renders.
  - *Given* a `Session` with `title: "fix-login-bug"`, `program: "claude"`, `status: SessionStatus.ACTIVE`, `subStatus: SubStatus.PROCESSING`, and `tags: ["backend", "urgent"]`, *When* `SessionBoardCard` renders it, *Then* `getByTestId("session-board-card-sess-a")` contains the text `"fix-login-bug"` and `queryByTestId("session-tag-editor")` is `null`.
- The card is a single fixed-height-bounded element so a column of 10 cards fits a 900px viewport without per-card height jitter.
  - *Given* two `Session`s, one with a 12-character title and one with an 80-character title, *When* both render as `SessionBoardCard`, *Then* both root elements report the same `offsetHeight` (the long title is clamped, not wrapped unbounded).
- Clicking the card body opens the session; the grip handle does not.
  - *Given* a rendered `SessionBoardCard` with `onSessionClick` spy attached, *When* the user clicks the element with `data-testid="session-board-card-body-sess-a"`, *Then* `onSessionClick` is called once with `"sess-a"`; *When* the user instead clicks `data-testid="session-board-card-grip-sess-a"`, *Then* `onSessionClick` is not called.
- A card whose session has an in-flight move shows a busy state and is not interactive.
  - *Given* `pending = { "sess-a": "pause" }`, *When* the card for `"sess-a"` renders, *Then* its root has `aria-busy="true"` and `data-pending="pause"`.

**Files**: `web-app/src/components/sessions/SessionBoardCard.tsx` (new), `web-app/src/components/sessions/SessionBoardCard.css.ts` (new)

##### Task 2.1.1a: Write `SessionBoardCard.css.ts` (~5 min)
- vanilla-extract only, all values from `vars.*` in `web-app/src/styles/theme.css.ts`. Title clamped with `WebkitLineClamp: 2`. Grip handle `opacity: 0` by default, `1` on `:hover`/`:focus-within`, `cursor: "grab"`.
- Busy state via `selectors: { '&[data-pending]': { … } }` — a `data-*` attribute, never an inline `style` (`.claude/rules/css-architecture.md`).
- Files: `web-app/src/components/sessions/SessionBoardCard.css.ts`

##### Task 2.1.1b: Implement the component (~5 min)
- Reuse `StatusBadge` and `SubStatusChip` (`web-app/src/components/sessions/`) rather than reimplementing status rendering.
- Props: `session`, `needsReview: boolean`, `pendingVerb: BoardMoveVerb | undefined`, `onSessionClick`, `moveTargets: BoardColumnKey[]`, `onMove`.
- `data-testid="session-board-card-${session.id}"`, body `…-body-${id}`, grip `…-grip-${id}`.
- Files: `web-app/src/components/sessions/SessionBoardCard.tsx`

##### Task 2.1.1c: Add the component test (~4 min)
- Cover the four criteria above.
- Files: `web-app/src/components/sessions/__tests__/SessionBoardCard.test.tsx`

---

#### Story 2.1.2: `SessionBoardColumn` — header count, empty state, overflow cap
**As a** user, **I want** each column to show how many sessions it holds, **so that** I can answer "how many need review" without counting cards.

> **Blocked on** the column-order Unresolved Question. Start the story only after that is confirmed or vetoed.

**Acceptance Criteria**:
- Each column header shows its label and its exact card count.
  - *Given* `sessions` where `resolveBoardColumn` maps three of them to `"needsReview"`, *When* the board renders, *Then* `getByTestId("board-column-header-needsReview")` has accessible text `"Needs Review (3)"`.
- An empty column renders its `emptyLabel`, keeps its full width, and remains a valid drop target region.
  - *Given* zero sessions resolving to `"paused"`, *When* the board renders, *Then* `getByTestId("board-column-paused")` is in the document, contains the text `"No paused sessions"`, and its `offsetWidth` equals that of the non-empty `"running"` column.
- A column exceeding `COLUMN_RENDER_CAP` renders exactly the cap plus an overflow footer.
  - *Given* 63 sessions resolving to `"complete"`, *When* the board renders, *Then* `getAllByTestId(/^session-board-card-/)` within `board-column-complete` has `length === 50`, and `getByTestId("board-column-overflow-complete")` has text `"+13 more — switch to list view"`.
- Each column scrolls internally; the header stays visible.
  - *Given* a column with 50 cards in a 600px-tall board, *When* the user scrolls inside `board-column-body-complete`, *Then* `board-column-header-complete` remains within the viewport (the header is outside the scrolling element, not inside it).

**Files**: `web-app/src/components/sessions/SessionBoard.tsx` (new), `web-app/src/components/sessions/SessionBoard.css.ts` (new)

##### Task 2.1.2a: Write `SessionBoard.css.ts` column layout (~5 min)
- Board = horizontal flex, `overflowX: "auto"`. Column = vertical flex with a fixed `minWidth` from `vars.space`, header `flexShrink: 0`, body `flex: 1` + `overflowY: "auto"`.
- Adapt the visual language of `web-app/src/components/backlog/BacklogBoard.css.ts` (read it, do not import from it).
- Files: `web-app/src/components/sessions/SessionBoard.css.ts`

##### Task 2.1.2b: Implement `SessionBoardColumn` inside `SessionBoard.tsx` (~5 min)
- Props: `column: BoardColumn`, `sessions: Session[]`, `exitingIds`, `enteringIds`, `pending`, plus card callbacks. `role="list"` on the body, matching `BacklogBoard.tsx`'s structural approach.
- Apply `COLUMN_RENDER_CAP` with the overflow footer; the footer's click switches the pane back to list mode.
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 2.1.2c: Add the column-behaviour test (~4 min)
- Cover count, empty state, and the 63→50+footer case.
- Files: `web-app/src/components/sessions/__tests__/SessionBoard.columns.test.tsx`

---

#### Story 2.1.3: `SessionBoard` shell with live cross-column transitions
**As a** user, **I want** a card that changes column because of a live event to visibly move rather than blink, **so that** I notice a peer's or an agent's state change.

**Acceptance Criteria**:
- The board derives all four columns from one `useMemo` over `sortedSessions` + `needsReviewIds`.
  - *Given* `sortedSessions` of length 20 and `needsReviewIds` of size 2, *When* the board renders, *Then* the sum of the four columns' card counts is exactly 20 — no session appears twice and none is dropped (this is `resolveBoardColumn`'s totality observed end-to-end).
- A session whose resolved column changes between renders fades out of its origin column and flashes in its destination.
  - *Given* a `Session` with `id: "sess-a"` resolving to `"running"`, *When* a `WatchSessions` event updates it to `status: SessionStatus.PAUSED` and the board re-renders, *Then* `board-column-running` still contains `session-board-card-sess-a` with `data-exiting="true"` for 200 ms, and `board-column-paused` contains it with `data-entering="true"` for 250 ms.
- Under `prefers-reduced-motion: reduce`, both durations collapse to 0 ms.
  - *Given* `window.matchMedia("(prefers-reduced-motion: reduce)").matches === true`, *When* the same column change occurs, *Then* `data-exiting` is never observed and the card appears only in `board-column-paused` on the next paint.
- A card whose column is unchanged across a re-render never animates.
  - *Given* a re-render triggered by an unrelated `sessions` array identity change where every session's `resolveBoardColumn` result is unchanged, *When* the board re-renders, *Then* no element has `data-entering` or `data-exiting` (this is the guarantee `liveVersion` provides for the backlog board; here it falls out of diffing resolved columns directly — see Corrections §2).

**Files**: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 2.1.3a: Implement the board shell and column derivation (~5 min)
- `SessionBoardProps`: `sessions`, `needsReviewIds`, `swimlaneAxis`, selection props, `onSessionClick`, `onSwitchToList`, plus the same action callbacks `SessionList` receives.
- One `useMemo` producing `Record<BoardColumnKey, Session[]>`.
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 2.1.3b: Port the enter/exit transition machinery (~5 min)
- Copy the ref+timer structure from `BacklogBoard.tsx:168-284` — `exitingItems`/`enteringIds` state, `exitingMapRef`/`enteringSetRef`, `prevColumnRef` (replacing `prevStatusRef`), timer maps, the `reducedMotionRef` effect, the flap-protection first pass, and the unmount timer cleanup.
- Replace the `isGenuineLiveChange` `liveVersion` gate with a plain `prevColumn !== nextColumn` check (Corrections §2). Keep the flap-protection pass — a rapid pause→resume→pause still needs it.
- Constants `EXIT_TRANSITION_MS = 200`, `ENTER_FLASH_MS = 250`, matching `BacklogBoard.tsx:51-52`.
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 2.1.3c: Add the transition test (~5 min)
- Use fake timers; assert `data-exiting`/`data-entering` presence and the reduced-motion and no-change cases.
- Files: `web-app/src/components/sessions/__tests__/SessionBoard.transitions.test.tsx`

---

### Epic 2.2: View toggle and persistence

**Goal**: The user can switch views and their choice survives a reload, per pane.

#### Story 2.2.1: List/Board toggle in the pane, persisted
**As a** user, **I want** my view choice remembered, **so that** I don't re-toggle on every reload.

**Acceptance Criteria**:
- `SessionListPaneBody` owns `DashboardViewMode` and renders `<SessionList>` or `<SessionBoard>` by ternary.
  - *Given* a pane with `pane.id === "p1"` and `dashboardViewMode === "board"`, *When* the pane renders, *Then* `getByTestId("session-board")` is present and `queryByTestId("show-archived-toggle")` is `null` (that control exists only in `SessionList`'s header, `SessionList.tsx:1101` — verified as the only list-only `data-testid` on that component today); *Given* the same pane with `dashboardViewMode === "list"`, *Then* the reverse holds. Both render inside the existing `session-list-scroll` wrapper (`PaneSplitRenderer.tsx:166`), which stays as the pane's scroll container in either mode.
- The choice persists under a pane-prefixed key using the existing helpers.
  - *Given* a pane with `pane.id === "p1"`, *When* the user activates the toggle button `getByRole("button", { name: "Board view" })`, *Then* `window.localStorage.getItem("pane-p1.stapler-squad-dashboard-view-mode")` equals `"\"board\""`, and after a remount the board renders without further interaction.
- Two panes hold independent view modes.
  - *Given* pane `"p1"` set to `"board"` and pane `"p2"` set to `"list"`, *When* both render, *Then* `p1` shows `session-board` and `p2` shows the list — mirroring the `storageKeyPrefix` isolation already covered by `SessionList.collapse.test.tsx:230-231`.
- The toggle is a two-button radio group, not an icon with no label.
  - *Given* the toggle rendered in list mode, *When* a screen reader reads it, *Then* it exposes `role="radiogroup"` with `aria-label="Session view mode"` and two radios named `"List view"` and `"Board view"`, the former with `aria-checked="true"`.

**Files**: `web-app/src/components/pane/PaneSplitRenderer.tsx`, `web-app/src/components/sessions/SessionViewToggle.tsx` (new), `web-app/src/components/sessions/SessionViewToggle.css.ts` (new)

##### Task 2.2.1a: Add `DASHBOARD_VIEW_MODE` to the storage key set (~2 min)
- Add `DASHBOARD_VIEW_MODE: 'stapler-squad-dashboard-view-mode'` to `BASE_STORAGE_KEYS` in `web-app/src/lib/hooks/useSessionFilters.ts` (moved there in Task 1.1.1a), so it picks up `makeStorageKeys`' prefixing for free.
- Files: `web-app/src/lib/hooks/useSessionFilters.ts`

##### Task 2.2.1b: Build `SessionViewToggle` (~4 min)
- `role="radiogroup"`, two `role="radio"` buttons with `List` / `LayoutGrid` lucide icons plus visible text labels. Props: `mode`, `onChange`.
- Files: `web-app/src/components/sessions/SessionViewToggle.tsx`, `web-app/src/components/sessions/SessionViewToggle.css.ts`

##### Task 2.2.1c: Wire the toggle and ternary into `SessionListPaneBody` (~5 min)
- Add `useState<DashboardViewMode>` initialised from `loadFromStorage(makeStorageKeys(\`pane-${pane.id}.\`).DASHBOARD_VIEW_MODE, "list")` and a `saveToStorage` effect.
- Pass the toggle into `SessionList`'s existing `extraHeaderActions` prop (`SessionList.tsx:298`) in list mode, and render it in `SessionBoard`'s own header in board mode, so the control sits in the same place in both.
- Files: `web-app/src/components/pane/PaneSplitRenderer.tsx`

##### Task 2.2.1d: Add the toggle/persistence test (~4 min)
- Cover persistence, per-pane isolation, and the radiogroup ARIA shape.
- Files: `web-app/src/components/sessions/__tests__/SessionViewToggle.test.tsx`

---

#### Story 2.2.2: `b` keyboard shortcut
**As a** user, **I want** to press `b` to flip views, **so that** switching is as fast as glancing.

**Acceptance Criteria**:
- `b` toggles the focused pane's view mode.
  - *Given* the pane in `"list"` mode with focus on `document.body`, *When* the user presses `b`, *Then* the mode becomes `"board"` and the persisted value updates; pressing `b` again returns it to `"list"`.
- `b` is inert while a text input has focus.
  - *Given* focus inside the session search `<input>`, *When* the user types `b`, *Then* the character is inserted into the input and the view mode is unchanged — using the same `target.tagName === "INPUT" || "TEXTAREA" || isContentEditable` guard shape as `SessionList.tsx:791-792`.
- `b` is inert when a modifier is held, so `Cmd+B`/`Ctrl+B` browser shortcuts still work.
  - *Given* the pane in `"list"` mode, *When* the user presses `Cmd+b`, *Then* the view mode is unchanged.
- Only the focused pane responds.
  - *Given* two panes with `state.focusedPaneId === "p1"`, *When* the user presses `b`, *Then* pane `"p1"`'s mode changes and pane `"p2"`'s does not.

**Files**: `web-app/src/components/pane/PaneSplitRenderer.tsx`

##### Task 2.2.2a: Add the guarded keydown effect (~4 min)
- Register on `document`; guard on input focus, on `e.metaKey || e.ctrlKey || e.altKey`, and on `isFocused` for this pane. `rg -n 'key === "b"'` across `web-app/src` returned no existing binding, so no collision to resolve.
- Files: `web-app/src/components/pane/PaneSplitRenderer.tsx`

##### Task 2.2.2b: Add the shortcut test (~4 min)
- Cover all four criteria.
- Files: `web-app/src/components/sessions/__tests__/SessionViewToggle.shortcut.test.tsx`

---

### Epic 2.3: The move path (accessible-first)

**Goal**: Every legal move is performable by keyboard and by touch before any drag code exists. This is the WCAG-mandatory path, built as the foundation rather than a fallback.

#### Story 2.3.1: `useBoardMove` — the move executor
**As a** user, **I want** a failed move to tell me why, **so that** I'm never left wondering whether it worked.

> **Blocked on** the archived-disappearance Unresolved Question.

**Acceptance Criteria**:
- A legal move calls exactly one RPC and returns `{ kind: "applied" }`.
  - *Given* a `Session` with `id: "sess-a"`, `status: SessionStatus.ACTIVE` and `intent = { sessionId: "sess-a", from: "running", to: "paused" }`, *When* `move(intent)` is awaited, *Then* `pauseSession` was called once with `"sess-a"`, `archiveSession`/`deleteSession`/`resumeSession` were not called, and the result is `{ kind: "applied" }`.
- A move to Complete archives and never deletes (ADR-002).
  - *Given* `intent = { …, to: "complete" }`, *When* `move` is awaited, *Then* `archiveSession` was called once and `deleteSession` was called zero times.
- An illegal move returns a rejection without calling any RPC.
  - *Given* `intent = { sessionId: "sess-a", from: "complete", to: "running" }`, *When* `move` is awaited, *Then* the result is `{ kind: "rejected", reason: "illegal-transition" }` and no RPC mock was called.
- A session that no longer exists returns `"session-gone"`.
  - *Given* `intent.sessionId === "sess-gone"` and `sessions` containing no such id, *When* `move` is awaited, *Then* the result is `{ kind: "rejected", reason: "session-gone" }` and no RPC was called.
- A rejected RPC surfaces its message and is not retried.
  - *Given* `pauseSession` rejecting with `new Error("session already paused")`, *When* `move({ …, to: "paused" })` is awaited, *Then* the result is `{ kind: "failed", message: "session already paused" }`, `pauseSession` was called exactly once, and `console.error` was called once with a payload containing `sessionId`, `from`, `to`, and `verb`.
- `pending` is set for the duration of the RPC and cleared on both success and failure.
  - *Given* a `pauseSession` mock that resolves after a controllable deferred, *When* `move` is in flight, *Then* `pending["sess-a"] === "pause"`; *When* the deferred settles either way, *Then* `pending["sess-a"]` is `undefined`.

**Files**: `web-app/src/lib/hooks/useBoardMove.ts` (new)

##### Task 2.3.1a: Implement the hook (~5 min)
- Look up the session from the `sessions` argument → `"session-gone"` if absent. Call `resolveMoveVerb` → `"illegal-transition"` if `null`. Set `pending`, `await` the verb's RPC from `useSessionServiceContext()`, clear `pending` in a `finally`.
- Verb→RPC: `pause`→`pauseSession`, `resume`→`resumeSession`, `resumeHibernated`→`resumeHibernatedSession`, `archive`→`archiveSession`.
- Files: `web-app/src/lib/hooks/useBoardMove.ts`

##### Task 2.3.1b: Add error mapping and the `console.error` log line (~3 min)
- Map a thrown/rejected RPC to `{ kind: "failed", message: err.message }`; log per the Observability Plan. Never auto-retry.
- Files: `web-app/src/lib/hooks/useBoardMove.ts`

##### Task 2.3.1c: Add the hook test (~5 min)
- Cover all six criteria with mocked `useSessionServiceContext`.
- Files: `web-app/src/lib/hooks/__tests__/useBoardMove.test.ts`

---

#### Story 2.3.2: "Move to…" menu on every board card
**As a** keyboard or touch user, **I want** a menu that moves a card between columns, **so that** the board is fully operable without dragging.

**Acceptance Criteria**:
- Every card exposes a "Move to…" control listing only legal target columns.
  - *Given* a card in `"running"` for a `Session` with `status: SessionStatus.ACTIVE`, *When* the user opens `getByTestId("session-board-card-move-sess-a")`, *Then* the menu contains exactly two items, `"Paused"` and `"Complete"` — `"Needs Review"` and `"Running"` are absent because `resolveMoveVerb` returns `null` for both.
- A card in Complete offers no move targets and the control is disabled, not missing.
  - *Given* a card in `"complete"`, *When* it renders, *Then* `getByTestId("session-board-card-move-sess-a")` has `aria-disabled="true"` and `title="No moves available from Complete"`.
- Selecting a menu item performs the move.
  - *Given* the menu open on a `"running"` card for `"sess-a"`, *When* the user activates `"Paused"`, *Then* `useBoardMove.move` is called once with `{ sessionId: "sess-a", from: "running", to: "paused" }`.
- The menu is fully keyboard-operable.
  - *Given* focus on `session-board-card-move-sess-a`, *When* the user presses `Enter`, then `ArrowDown`, then `Enter`, *Then* the menu opens, the first item receives focus, and the move fires — without any pointer event.
- The menu is portaled, per `.claude/rules/css-architecture.md`.
  - *Given* the menu open inside a column that has `overflowY: "auto"`, *When* the DOM is inspected, *Then* the menu's root element's `parentElement` is `document.body`, not the column — so it is not clipped by the column's scroll container.

**Files**: `web-app/src/components/sessions/SessionBoardCard.tsx`, `web-app/src/components/sessions/SessionBoardCard.css.ts`

##### Task 2.3.2a: Add the move control and its portaled menu (~5 min)
- Follow the existing overflow-menu pattern in `web-app/src/components/sessions/SessionActionsOverflow.tsx`; portal via `createPortal(…, document.body)` modelled on `SessionPeekModal.tsx`.
- Compute `moveTargets` by calling `resolveMoveVerb` for each `BoardColumnKey` and keeping the non-`null` ones.
- Files: `web-app/src/components/sessions/SessionBoardCard.tsx`

##### Task 2.3.2b: Wire the menu into the board and pass `move` down (~3 min)
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 2.3.2c: Add the menu test (~5 min)
- Cover all five criteria, including the keyboard-only path and the `parentElement === document.body` assertion.
- Files: `web-app/src/components/sessions/__tests__/SessionBoardCard.moveMenu.test.tsx`

---

#### Story 2.3.3: Move announcements, toasts, and focus management
**As a** screen-reader user, **I want** every move outcome announced and my focus preserved, **so that** I always know where I am and what happened.

**Acceptance Criteria**:
- A successful move announces through the existing live region.
  - *Given* a `Session` with `title: "fix-login-bug"` moved from `"running"` to `"paused"`, *When* the move returns `{ kind: "applied" }`, *Then* `document.getElementById("bulk-feedback-live")` has `textContent === "Moved 'fix-login-bug' to Paused"`.
- A move that archives while the Archived filter is off announces the disappearance explicitly (ADR-002 Consequences).
  - *Given* `showArchived === false` and a successful move of `"fix-login-bug"` to `"complete"`, *When* the move settles, *Then* the live region reads `"Archived 'fix-login-bug' — hidden by the Archived filter"` and a toast with the same text and a "Show archived" action appears.
- A rejected move announces its mapped reason and fires no RPC-failure toast.
  - *Given* a move returning `{ kind: "rejected", reason: "stale-source-column" }`, *When* it settles, *Then* the live region reads `MOVE_REJECTION_MESSAGE["stale-source-column"]` — `"That session moved before the drop landed. Nothing changed."` — and the card is unchanged.
- A failed move announces the RPC's own message and offers one retry.
  - *Given* a move returning `{ kind: "failed", message: "session already paused" }`, *When* it settles, *Then* a toast reads `"Couldn't move 'fix-login-bug': session already paused"` with a single "Retry" action, and the live region carries the same text.
- Focus lands on the moved card in its destination column on success, and stays on the origin card otherwise.
  - *Given* focus on `session-board-card-sess-a` in `"running"`, *When* a move to `"paused"` returns `{ kind: "applied" }` and the card re-renders under `board-column-paused`, *Then* `document.activeElement` is the card element inside `board-column-paused`; *When* the outcome is `"rejected"` or `"failed"` instead, *Then* `document.activeElement` is still the card inside `board-column-running` — never `document.body`.

**Files**: `web-app/src/components/sessions/SessionBoard.tsx`, `web-app/src/lib/board/boardTransitions.ts`

##### Task 2.3.3a: Write announcements into `#bulk-feedback-live` (~4 min)
- Write via `document.getElementById("bulk-feedback-live")` if present, else through `showFeedback` from `useSessionSelection`. Do **not** create a second live region.
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 2.3.3b: Add outcome toasts via `useNotifications` (~4 min)
- One exhaustive `switch` over `BoardMoveOutcome`; the `"failed"` branch gets a single Retry action that re-issues the same intent once.
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 2.3.3c: Implement post-move focus restoration (~4 min)
- Keep a `movedCardFocusRef` set at move start; in a `useEffect` keyed on the card's resolved column, call `.focus()` on the card in its settled column.
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 2.3.3d: Add the announcement/focus test (~5 min)
- Files: `web-app/src/components/sessions/__tests__/SessionBoard.announce.test.tsx`

---

## Phase 3: Drag-and-drop and list-parity features

### Epic 3.1: `@dnd-kit` wiring

**Goal**: Drag becomes a shortcut for the Story 2.3 move path — same `BoardMoveIntent`, same `useBoardMove`, same outcomes. Drag adds a gesture, not a second code path.

#### Story 3.1.1: Add the dependency and the drag-overlay z-index slot
**As a** developer, **I want** the DnD library and its stacking slot in place, **so that** no later task hardcodes a z-index.

**Acceptance Criteria**:
- The three `@dnd-kit` packages install cleanly on React 19.
  - *Given* `web-app/package.json` with `"@dnd-kit/core": "^6.3.1"`, `"@dnd-kit/sortable": "^10.0.0"`, `"@dnd-kit/utilities": "^3.2.2"`, *When* `cd web-app && pnpm install` runs, *Then* it exits `0` with no `ERESOLVE`/peer-dependency error mentioning `react@19`.
- A named `dragOverlay` slot exists on the z-index ladder.
  - *Given* `zIndex` in `web-app/src/styles/theme-contract.css.ts:195-217`, *When* the slot is added, *Then* `zIndex.dragOverlay === 1090` — above `floatingTerminalUI: 1085` so the dragged card floats over pane chrome, below `tooltip: 1100` so a drop-target tooltip stays readable above it.
- No hardcoded z-index appears in board CSS.
  - *Given* `web-app/src/components/sessions/SessionBoard.css.ts` and `SessionBoardCard.css.ts`, *When* `rg -n "zIndex: [0-9]" web-app/src/components/sessions/SessionBoard*.css.ts` runs, *Then* it returns no matches.

**Files**: `web-app/package.json`, `web-app/src/styles/theme-contract.css.ts`

##### Task 3.1.1a: Add the dependencies (~2 min)
- Files: `web-app/package.json`, `web-app/pnpm-lock.yaml`

##### Task 3.1.1b: Add the `dragOverlay: 1090` slot with a rationale comment (~2 min)
- Match the commenting style already used for the 1040–1065 navigation block.
- Files: `web-app/src/styles/theme-contract.css.ts`

##### Task 3.1.1c: Measure and record the bundle delta (~4 min)
- Run `cd web-app && pnpm run build` before and after, and record the First Load JS delta for the `/` route in the PR body. Replaces ADR-001's UNVERIFIED community estimate with a measured number.
- Files: none (evidence for the PR body)

---

#### Story 3.1.2: `DndContext`, draggable cards, droppable columns
**As a** mouse user, **I want** to drag a card into another column, **so that** I can change a session's state in one gesture.

**Acceptance Criteria**:
- Dropping a card on another column issues exactly the intent the menu would have.
  - *Given* a card for `"sess-a"` in `"running"`, *When* the user drags it and drops it on `board-column-paused`, *Then* `useBoardMove.move` is called once with `{ sessionId: "sess-a", from: "running", to: "paused" }` — byte-identical to the Story 2.3.2 menu path.
- Drag is initiated only from the grip handle, not the card body.
  - *Given* a card with an inline title, *When* the user presses and moves on `session-board-card-body-sess-a`, *Then* no drag starts; *When* they press and move on `session-board-card-grip-sess-a`, *Then* `onDragStart` fires.
- A short press without movement is a click, not a drag.
  - *Given* `PointerSensor` configured with `activationConstraint: { distance: 8 }`, *When* the user presses the grip and releases after 3px of movement, *Then* no `onDragEnd` fires and no move is issued.
- Drag is disabled on coarse pointers; the menu remains.
  - *Given* `window.matchMedia("(pointer: coarse)").matches === true`, *When* the board renders, *Then* no `PointerSensor` is registered, `session-board-card-grip-sess-a` is absent, and `session-board-card-move-sess-a` is present.
- Drag is disabled while the swimlane axis is a `GroupingStrategy` (ADR-003 §4).
  - *Given* `swimlaneAxis = { kind: "grouping", strategy: GroupingStrategy.Tag }`, *When* the board renders, *Then* no grip handles are rendered and each card's move control is `aria-disabled="true"`.

**Files**: `web-app/src/components/sessions/SessionBoard.tsx`, `web-app/src/components/sessions/SessionBoardCard.tsx`

##### Task 3.1.2a: Wrap the board in `DndContext` with sensors (~5 min)
- `PointerSensor` with `activationConstraint: { distance: 8 }` plus `KeyboardSensor`. Register neither when `(pointer: coarse)` matches or `swimlaneAxis.kind === "grouping"`.
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 3.1.2b: Make columns droppable and cards draggable (~5 min)
- `useDroppable({ id: column.key })` on each column body; `useDraggable({ id: session.id, data: { from: columnKey } })` bound to the grip via `listeners`/`attributes` on the handle element only.
- Files: `web-app/src/components/sessions/SessionBoard.tsx`, `web-app/src/components/sessions/SessionBoardCard.tsx`

##### Task 3.1.2c: Implement `onDragEnd` → `BoardMoveIntent` → `move` (~4 min)
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

---

#### Story 3.1.3: Portaled `DragOverlay` and the native-drag collision guard
**As a** user, **I want** the dragged card to follow my cursor correctly everywhere on screen, **so that** the gesture isn't clipped or hijacked by the pane layout.

> **Blocked on** the two Unresolved Questions about `<DragOverlay>` portalling and pane `draggable` inheritance. Both are resolved by the runtime checks in this story's tasks — not by further code reading.

**Acceptance Criteria**:
- The drag overlay renders as a direct child of `document.body`.
  - *Given* an active drag of `"sess-a"`, *When* the DOM is inspected, *Then* the overlay element's `closest("[data-testid='session-board']")` is `null` and its nearest positioned ancestor chain terminates at `document.body`.
- The overlay uses the named z-index slot.
  - *Given* the overlay rendered, *When* its computed `z-index` is read, *Then* it equals `1090`, sourced from `zIndex.dragOverlay`, with no literal in the `.css.ts`.
- Starting a card drag never swaps panes.
  - *Given* a split layout where `hasSplits === true` (so `PaneLeafComponent` sets `draggable={true}`, `PaneSplitRenderer.tsx:243`), *When* the user drags `session-board-card-grip-sess-a` from one pane and drops it on a column in the same pane, *Then* no `SWAP_PANES` action is dispatched and the pane layout is unchanged.
- The overlay is visually distinguishable from the in-column card.
  - *Given* an active drag, *When* the overlay and the origin card are compared, *Then* the origin card has `data-drag-source="true"` with reduced opacity and the overlay has a raised shadow — so the user can see both where the card came from and where it is now.

**Files**: `web-app/src/components/sessions/SessionBoard.tsx`, `web-app/src/components/sessions/SessionBoardCard.tsx`, `web-app/src/components/sessions/SessionBoard.css.ts`

##### Task 3.1.3a: Render `<DragOverlay>` with `SessionBoardCard` inside (~4 min)
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 3.1.3b: Verify the overlay's actual parent, and wrap in `createPortal` if it is not `document.body` (~4 min)
- Start a drag in a browser (`PORT=8999 STAPLER_SQUAD_INSTANCE=claude-manual-test /tmp/ssq-manual-test --tmux-keep-server &`, per `CLAUDE.md`'s manual-testing section — **not** `make install-service`) and read the overlay's `parentElement` in devtools. Record the observed value in the PR body; this closes Unresolved Question #4.
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 3.1.3c: Verify and, if confirmed, fix the pane native-drag collision (~4 min)
- In the same manual instance, split the pane so `hasSplits === true`, then drag a card grip. If a native pane-swap drag starts, set `draggable={false}` on the `SessionBoardCard` root and re-verify. Record the observed behaviour either way — this closes Unresolved Question #3.
- Files: `web-app/src/components/sessions/SessionBoardCard.tsx`

---

#### Story 3.1.4: Drop-target legality and the verb-in-header affordance
**As a** user, **I want** to see what a drop will do before I commit, **so that** I don't accidentally archive a session I meant to pause.

**Acceptance Criteria**:
- An illegal target rejects visibly on hover rather than accepting and erroring after the drop.
  - *Given* a drag of a `"complete"`-column card, *When* the pointer enters `board-column-running`, *Then* that column has `data-drop-state="rejected"` and `aria-dropeffect` is **not** used (deprecated in ARIA 1.2+ per `research/ux.md` §3); dropping there issues no `move` call.
- A legal target shows the RPC verb in its header while hovered.
  - *Given* a drag of a `"running"`-column card, *When* the pointer enters `board-column-complete`, *Then* `board-column-header-complete` reads `"Drop to archive session"` and reverts to `"Complete (N)"` on drag end.
- The legal/illegal distinction is conveyed by more than colour (WCAG 1.4.1).
  - *Given* a hovered rejected column, *When* it renders, *Then* it has both a distinct border style (dashed vs. solid) and the header text `"Can't move here"` — not only a background tint.
- Drop-state styling is driven by `data-*` + `selectors`, never inline layout style.
  - *Given* `SessionBoard.css.ts`, *When* `rg -n "style=\{\{" web-app/src/components/sessions/SessionBoard.tsx` runs, *Then* the only matches are CSS-custom-property bridges (`--*`), per `.claude/rules/css-architecture.md`.

**Files**: `web-app/src/components/sessions/SessionBoard.tsx`, `web-app/src/components/sessions/SessionBoard.css.ts`

##### Task 3.1.4a: Track `activeDrag` and compute per-column legality on `onDragOver` (~5 min)
- Reuse `resolveMoveVerb` — no second legality rule.
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 3.1.4b: Add the header verb text and `data-drop-state` styling (~5 min)
- Verb→copy map: `pause`→`"Drop to pause session"`, `resume`/`resumeHibernated`→`"Drop to resume session"`, `archive`→`"Drop to archive session"`, `null`→`"Can't move here"`.
- Files: `web-app/src/components/sessions/SessionBoard.tsx`, `web-app/src/components/sessions/SessionBoard.css.ts`

---

#### Story 3.1.5: Stale-state re-validation at drop
**As a** user, **I want** a drop to be cancelled rather than mis-applied if the session changed mid-drag, **so that** a live event never causes an action I didn't intend.

**Acceptance Criteria**:
- The source column is snapshotted at drag start and re-checked at drop.
  - *Given* a drag started on `"sess-a"` while it resolved to `"running"`, *When* a `WatchReviewQueue` `itemAdded` event moves it to `"needsReview"` before the drop lands on `board-column-paused`, *Then* `move` is not called and the outcome surfaced is `{ kind: "rejected", reason: "stale-source-column" }`.
- A session deleted mid-drag cancels the drop rather than firing an RPC.
  - *Given* a drag of `"sess-a"`, *When* a `removeSession("sess-a")` dispatch lands before the drop, *Then* the outcome is `{ kind: "rejected", reason: "session-gone" }` and no RPC mock was called.
- Column membership is **not** frozen during the drag — only the source snapshot is.
  - *Given* an active drag of `"sess-a"` from `"running"`, *When* an unrelated session `"sess-b"` transitions from `"running"` to `"paused"` mid-drag, *Then* `"sess-b"`'s card moves columns normally and the drag of `"sess-a"` is unaffected (the fix is a snapshot of one card's source column, not a frozen board).
- Drop handlers read live state through a ref, not a closure.
  - *Given* `onDragEnd` registered once at mount, *When* it executes after `sessions` has changed identity three times, *Then* it operates on the latest `sessions` — via the ref-backed-handler pattern already used at `web-app/src/lib/hooks/useReviewQueue.ts:235-272`, not by closing over React state directly (`research/pitfalls.md` §1).

**Files**: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 3.1.5a: Snapshot the source column at `onDragStart` (~3 min)
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 3.1.5b: Re-validate at `onDragEnd` and reject on mismatch (~4 min)
- Compare the snapshot against a freshly computed `resolveBoardColumn`; on mismatch return `"stale-source-column"` without calling `move`.
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 3.1.5c: Convert drag handlers to the ref-backed pattern (~4 min)
- Mirror `useReviewQueue.ts:235-272`'s `handleReviewQueueEventRef` shape.
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 3.1.5d: Add the stale-state test (~5 min)
- Files: `web-app/src/components/sessions/__tests__/SessionBoard.staleDrag.test.tsx`

---

### Epic 3.2: List-parity features

**Goal**: Search, bulk select, and the swimlane axis behave on the board exactly as they do in the list — because they are literally the same hooks.

#### Story 3.2.1: Instant search filters cards across all columns
**As a** user, **I want** search to narrow the board, **so that** I don't have to switch views to find something.

**Acceptance Criteria**:
- One search box filters every column simultaneously, using `useSessionFilters`.
  - *Given* six sessions — two in `"running"` and one in `"paused"` matching `"login"`, three elsewhere not matching — *When* the user types `"login"` into `getByTestId("session-board-search")`, *Then* `board-column-running` shows 2 cards, `board-column-paused` shows 1, and `board-column-complete` shows its empty state.
- Column header counts reflect the filtered set, not the total.
  - *Given* the same state, *When* the counts are read, *Then* `board-column-header-running` reads `"Running (2)"`, not `"Running (5)"` — the count is over rendered cards so it never contradicts what the user can see.
- Search state carries across a view switch.
  - *Given* the user typed `"login"` on the board in pane `"p1"` and then pressed `b` to switch to list, *When* the list renders, *Then* its search input's value is `"login"`. Mechanism: each view calls `useSessionFilters` internally with the same `storageKeyPrefix`, and the hook initialises from `pane-p1.stapler-squad-search-query` — the value survives the remount via the persisted key, not via a shared instance. (The hook is deliberately **not** hoisted into `SessionListPaneBody` and passed down: that would change `SessionList`'s prop API and break the "existing suites pass unedited" criterion of Story 1.1.1.)

**Files**: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 3.2.1a: Render the shared search input in the board header (~3 min)
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 3.2.1b: Add the cross-column search test (~4 min)
- Files: `web-app/src/components/sessions/__tests__/SessionBoard.search.test.tsx`

---

#### Story 3.2.2: Bulk select across columns
**As a** user, **I want** to select cards from different columns and act on all of them, **so that** the board doesn't lose a capability the list has.

**Acceptance Criteria**:
- `BulkActions` renders on the board with a count spanning columns.
  - *Given* `selectMode === true` with `"sess-a"` selected in `"running"` and `"sess-b"` in `"paused"`, *When* the board renders, *Then* `BulkActions` shows `selectedCount === 2`.
- `activeSelection` still excludes cards hidden by search.
  - *Given* `"sess-a"` and `"sess-b"` selected and a search query matching only `"sess-a"`, *When* the board renders, *Then* `BulkActions` shows `selectedCount === 1` — the same `activeSelection` intersection as `SessionList.tsx:639-642`.
- Bulk pause acts on the cross-column selection.
  - *Given* the two-column selection above, *When* the user activates `onPauseAll`, *Then* `onPauseSession` is called with `"sess-a"` and with `"sess-b"`, and the selection clears.

**Files**: `web-app/src/components/sessions/SessionBoard.tsx`, `web-app/src/components/sessions/SessionBoardCard.tsx`

##### Task 3.2.2a: Add a selection checkbox to `SessionBoardCard` (~3 min)
- Shown only when `selectMode` is true; does not trigger `onSessionClick`.
- Files: `web-app/src/components/sessions/SessionBoardCard.tsx`

##### Task 3.2.2b: Render `BulkActions` in the board header from `useSessionSelection` (~4 min)
- `BulkActions` is prop-driven with no internal fetching (`BulkActions.tsx:9-20`), so it drops straight in.
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 3.2.2c: Add the cross-column bulk test (~4 min)
- Files: `web-app/src/components/sessions/__tests__/SessionBoard.bulk.test.tsx`

---

#### Story 3.2.3: Swimlane axis switching (read-only)
**As a** user, **I want** to re-lane the board by tag or category, **so that** I can see my fleet grouped the way I already group the list.

**Acceptance Criteria**:
- Switching the axis to a `GroupingStrategy` re-lanes the board using `groupSessions` unchanged.
  - *Given* six sessions with `category` values `"backend"` (×4) and `"frontend"` (×2), *When* the user selects `GroupingStrategy.Category` in `getByTestId("board-swimlane-axis")`, *Then* the board renders exactly two lanes, headed `"backend (4)"` and `"frontend (2)"`, produced by `groupSessions(sortedSessions, GroupingStrategy.Category)`.
- The axis choice persists under a key **distinct** from the list's grouping strategy.
  - *Given* the list's `stapler-squad-grouping-strategy` set to `GroupingStrategy.Category`, *When* the user sets the board axis to `GroupingStrategy.Tag`, *Then* `pane-p1.stapler-squad-board-swimlane-axis` holds the tag value and `pane-p1.stapler-squad-grouping-strategy` is unchanged — switching views does not silently re-group the other one (`research/features.md` §8).
- A non-status axis is read-only and says so (ADR-003 §4).
  - *Given* `swimlaneAxis = { kind: "grouping", strategy: GroupingStrategy.Tag }`, *When* the board renders, *Then* `getByTestId("board-swimlane-readonly-hint")` reads `"Read-only — moves are available on the Status axis"`, no grip handles render, and every move control is `aria-disabled="true"`.
- Switching back to the status axis restores moves.
  - *Given* the board on the Tag axis, *When* the user selects `"Status"`, *Then* the four `BOARD_COLUMNS` lanes return and grip handles render again (on a fine pointer).

**Files**: `web-app/src/components/sessions/SessionBoard.tsx`, `web-app/src/lib/hooks/useSessionFilters.ts`

##### Task 3.2.3a: Add `BOARD_SWIMLANE_AXIS` to the storage key set (~2 min)
- Add `BOARD_SWIMLANE_AXIS: 'stapler-squad-board-swimlane-axis'` to `BASE_STORAGE_KEYS`.
- Files: `web-app/src/lib/hooks/useSessionFilters.ts`

##### Task 3.2.3b: Render the axis selector and branch lane derivation (~5 min)
- `swimlaneAxis.kind === "status"` → `BOARD_COLUMNS` + `resolveBoardColumn`; `"grouping"` → `groupSessions(sortedSessions, strategy)` mapped to the same lane shape.
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 3.2.3c: Add the read-only hint and disable moves on a grouping axis (~3 min)
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 3.2.3d: Add the axis test (~5 min)
- Files: `web-app/src/components/sessions/__tests__/SessionBoard.swimlane.test.tsx`

---

## Phase 4: Tests, registry, and documentation

### Epic 4.1: Test coverage

**Goal**: Every ADR decision is pinned by a test, and the e2e spec follows `.claude/rules/e2e-test-conventions.md`.

#### Story 4.1.1: Exhaustive unit tests for the pure board model
**As a** reviewer, **I want** every precedence rule pinned by a named test, **so that** a future change to column mapping cannot silently alter behaviour.

**Acceptance Criteria**:
- Every `SessionStatus` value has a `resolveBoardColumn` case and none returns `undefined`.
  - *Given* the array `[UNSPECIFIED, ACTIVE, READY, LOADING, PAUSED, NEEDS_APPROVAL, CREATING, STOPPED, HIBERNATED, RESTORING]`, *When* each is resolved with empty `needsReviewIds`, *Then* every result is one of the four `BoardColumnKey` values and the test asserts `result !== undefined` for each.
- Every precedence collision from ADR-003 has a named test.
  - *Given* the test file, *When* `cd web-app && npx jest --no-coverage --testPathPatterns="boardColumns"` runs, *Then* the reported test names include `resolveBoardColumn_should_ReturnComplete_When_StoppedAndInReviewQueue`, `resolveBoardColumn_should_ReturnNeedsReview_When_PausedAndInReviewQueue`, `resolveBoardColumn_should_ReturnPaused_When_Hibernated`, and `resolveBoardColumn_should_ReturnComplete_When_ActiveButArchived`.
- `resolveMoveVerb` never returns a delete-shaped verb.
  - *Given* the exhaustive 4×4 matrix of `(from, to)` `BoardColumnKey` pairs against a `Session` in each of the nine statuses, *When* each is resolved, *Then* every non-`null` result is one of `"pause" | "resume" | "resumeHibernated" | "archive"` — asserted against the literal set, so adding a delete verb fails the test.

**Files**: `web-app/src/lib/board/__tests__/boardColumns.test.ts` (new), `web-app/src/lib/board/__tests__/boardTransitions.test.ts` (new)

##### Task 4.1.1a: Write `boardColumns.test.ts` (~5 min)
- Files: `web-app/src/lib/board/__tests__/boardColumns.test.ts`

##### Task 4.1.1b: Write `boardTransitions.test.ts` including the 4×4×9 matrix (~5 min)
- Files: `web-app/src/lib/board/__tests__/boardTransitions.test.ts`

---

#### Story 4.1.2: Confirm no test-timing flakiness was introduced
**As a** maintainer, **I want** the board's animation tests to be deterministic, **so that** this feature doesn't add to the repo's flake backlog.

**Acceptance Criteria**:
- No board test uses real timers for the transition animation.
  - *Given* the board test files, *When* `rg -n "setTimeout|requestAnimationFrame" web-app/src/components/sessions/__tests__/SessionBoard*.test.tsx` runs, *Then* every match is inside a `jest.useFakeTimers()` block — no rAF-coalesced assertions, per `research/pitfalls.md` §5.
- The board suite passes 20 consecutive runs.
  - *Given* the completed board tests, *When* `cd web-app && for i in $(seq 1 20); do npx jest --no-coverage --testPathPatterns="SessionBoard" || break; done` runs, *Then* all 20 iterations pass; paste the final count into the PR body.

**Files**: all `web-app/src/components/sessions/__tests__/SessionBoard*.test.tsx`

##### Task 4.1.2a: Audit board tests for real timers and rAF (~3 min)
- Files: `web-app/src/components/sessions/__tests__/`

##### Task 4.1.2b: Run the 20× repeat loop and record the result (~5 min)
- Files: none (evidence for the PR body)

---

#### Story 4.1.3: End-to-end spec
**As a** maintainer, **I want** a Playwright spec covering the toggle and both move paths, **so that** the board is regression-protected in CI.

**Acceptance Criteria**:
- The spec opens with a feature annotation and uses no `waitForTimeout` (`.claude/rules/e2e-test-conventions.md`).
  - *Given* `tests/e2e/session-board-view.spec.ts`, *When* its first line is read, *Then* it is `// @feature session:list, session:update, session:archive`; *When* `rg -n "waitForTimeout" tests/e2e/session-board-view.spec.ts` runs, *Then* it returns no matches.
- All locators are `data-testid` or ARIA roles.
  - *Given* the spec, *When* `rg -n "locator\('\.'" tests/e2e/session-board-view.spec.ts` runs, *Then* it returns no matches — no CSS class selectors.
- The spec exercises toggle → board → menu-move → column change.
  - *Given* a seeded session titled `"e2e-board-session"` in the Running column, *When* the test presses `b`, opens that card's move menu, and selects `"Paused"`, *Then* `expect(page.getByTestId("board-column-paused").getByText("e2e-board-session")).toBeVisible()` resolves.
- A page helper is extracted rather than inlining board interactions.
  - *Given* the spec, *When* it is read, *Then* board navigation and card interaction go through `tests/e2e/pages/SessionBoardPage.ts`, following the existing `tests/e2e/pages/SessionsPage.ts` convention.

**Files**: `tests/e2e/session-board-view.spec.ts` (new), `tests/e2e/pages/SessionBoardPage.ts` (new)

##### Task 4.1.3a: Write `SessionBoardPage.ts` (~5 min)
- Methods: `switchToBoard()`, `columnCount(key)`, `openMoveMenu(title)`, `moveTo(title, columnLabel)`, `cardInColumn(key, title)`.
- Files: `tests/e2e/pages/SessionBoardPage.ts`

##### Task 4.1.3b: Write the spec (~5 min)
- Files: `tests/e2e/session-board-view.spec.ts`

##### Task 4.1.3c: Run it and paste the output (~5 min)
- `cd tests/e2e && npx playwright test session-board-view.spec.ts` — the isolated server is auto-managed by `global-setup.ts`; do **not** start one by hand.
- Files: none (evidence for the PR body)

---

### Epic 4.2: Registry and documentation

**Goal**: The feature is discoverable through the repo's own registries, and the two registries that do **not** apply are documented as not applying so a reviewer doesn't file a false-positive blocker.

#### Story 4.2.1: Feature registry entry
**As a** maintainer, **I want** the board registered, **so that** `make registry-generate` reports it as covered.

**Acceptance Criteria**:
- A per-feature file exists with the correct schema.
  - *Given* `docs/registry/features/frontend/session-board-view.json`, *When* it is read, *Then* it contains `"id": "session-board-view"`, `"type": "frontend"`, `"filePath": "web-app/src/components/sessions/SessionBoard.tsx"`, `"tested": true`, and `"testIds"` listing the `describe > test` names from `tests/e2e/session-board-view.spec.ts`.
- Regeneration introduces no net new coverage gap.
  - *Given* the count in `docs/registry/coverage-gaps.json` before the change, *When* `make registry-generate` runs, *Then* the count after is less than or equal to the count before; record both numbers, derived by command, in the PR body.
- A `// +feature:` marker sits in the component's first 10 lines.
  - *Given* `web-app/src/components/sessions/SessionBoard.tsx`, *When* its first 10 lines are read, *Then* one is `// +feature: session-board-view`, matching the convention at `BacklogBoard.tsx:2`.

**Files**: `docs/registry/features/frontend/session-board-view.json` (new), `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 4.2.1a: Add the `// +feature:` marker (~2 min)
- Files: `web-app/src/components/sessions/SessionBoard.tsx`

##### Task 4.2.1b: Create the registry file and run `make registry-generate` (~4 min)
- Files: `docs/registry/features/frontend/session-board-view.json`

---

#### Story 4.2.2: Document the registries that deliberately do not apply
**As a** reviewer, **I want** the plan's registry reasoning visible in the diff, **so that** I don't block the PR over a touchpoint that genuinely doesn't apply.

**Acceptance Criteria**:
- The PR body states, with reasons, that no `OmnibarAction`, `DetectorRegistry`, or session-creation-mode touchpoint applies.
  - *Given* the PR body, *When* it is read, *Then* it contains: no `OmnibarAction` union member or `dispatch.ts` case (drag is a direct RPC from board UI, not an omnibar-dispatched action); no `Detector` (a drag gesture has no input string to detect); and none of the 7 session-creation touchpoints (`.claude/rules/session-creation-registry.md` scopes them to new `SessionType` values, and `requirements.md:46` puts new statuses out of scope) — per `research/pitfalls.md` §2.
- The board's column-precedence rule is discoverable from the code, not only from the ADR.
  - *Given* `web-app/src/lib/board/boardColumns.ts`, *When* its header comment is read, *Then* it names ADR-003 by path and states the four-rule cascade in one sentence — so a reader who never opens `project_plans/` still learns why Stopped beats Needs Review.

**Files**: `web-app/src/lib/board/boardColumns.ts` (comment only)

##### Task 4.2.2a: Write the PR-body registry-exemption paragraph (~3 min)
- Files: none (PR body)

##### Task 4.2.2b: Verify the ADR-003 header comment landed in Task 1.2.1a (~2 min)
- Files: `web-app/src/lib/board/boardColumns.ts`

---

## Definition of Done

Every item below must be green, with the command output pasted into the PR body — "green first, then done."

- [ ] `cd web-app && pnpm run lint` exits `0`
- [ ] `cd web-app && npx tsc --noEmit` exits `0`
- [ ] `cd web-app && npx jest --no-coverage` exits `0`, with the four pre-existing `SessionList.*.test.tsx` suites passing **unedited** (`git diff --stat web-app/src/components/sessions/__tests__/SessionList.*` shows no changes)
- [ ] `cd tests/e2e && npx playwright test session-board-view.spec.ts` passes
- [ ] `make ci` passes (Go side is untouched, but the gate is the definitive pre-push check per `CLAUDE.md`)
- [ ] `make registry-generate` run and changed files committed; before/after `coverage-gaps.json` counts recorded
- [ ] Both runtime verifications from Story 3.1.3 (overlay parent, pane-drag collision) performed against a **manual instance on `PORT=8999`** — never `make install-service`, which restarts the live service and kills every tmux session (`.claude/rules/tmux-keep-server-on-restart.md`)
- [ ] All four Unresolved Questions closed, with the answers recorded in the PR body
- [ ] Planning artifacts under `project_plans/kanban-board-view/` committed (`.claude/rules/sdd-planning-artifacts-commit.md`)
