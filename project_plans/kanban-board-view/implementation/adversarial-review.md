# Adversarial Review: kanban-board-view
**Date**: 2026-08-06
**Verdict**: BLOCKED

Reviewed: `requirements.md`, `implementation/plan.md` (940 lines, 26 stories, 72 tasks — counts
derived by `rg -c "^#### Story "` / `rg -c "^##### Task "`), ADR-001, ADR-002, ADR-003.

Every file:line citation in the plan and the ADRs was spot-checked against the working tree.
**The plan's citation hygiene is excellent** — `PaneSplitRenderer.tsx:167,192,243-246,263`;
`SessionList.tsx:298,304,314,317-355,472-481,530-585,639-642,788-806,1101,1167`;
`BacklogBoard.tsx:2,21-24,38-44,51-52,160-284`; `useSessionService.ts:110-111,301-335,1108-1109`;
`SessionServiceContext.tsx:21-50`; `useReviewQueue.ts:235-272`; `strategies.ts:7-18,69,101-110`;
`theme-contract.css.ts` zIndex ladder; `page.tsx:478`. All accurate. The three "Corrections to
Phase 2 research" are all correct: `<SessionList>` has exactly one production render site
(`PaneSplitRenderer.tsx:167`), `liveVersion` exists only in `backlogItemsSlice.ts`, and
`archiveSession` is genuinely absent from `SessionServiceContextValue` while present on the
returned object.

The blockers below are **not** citation errors. They are behaviours of the cited code that the
plan reads past.

## Blockers

- [ ] **B1 — Every move RPC swallows its own error, so `useBoardMove` can never return
  `{kind: "failed"}` in production.** VERIFIED by reading the implementations, not the type
  signatures: `archiveSession` (`web-app/src/lib/hooks/useSessionService.ts:600-612`) catches,
  `dispatch(setError(…))`, and `return false`; `unarchiveSession` (`:614-626`) is identical;
  `pauseSession`/`resumeSession` (`:363-383`) delegate to `updateSession` (`:301-335`) which
  catches, `console.error`s, and `return response.session ?? null`; `hibernateSession`
  (`:385-399`) same shape. **Nothing throws.** All four additionally return `false`/`null`
  *silently* when `clientRef.current` is null. Task 2.3.1b instructs "Map a thrown/rejected RPC
  to `{ kind: "failed", message: err.message }`" — that branch is unreachable, so every failed
  move reports `{kind: "applied"}`, announces `"Moved 'X' to Paused"` into `#bulk-feedback-live`,
  and (Story 2.3.3 AC #5) moves focus into the destination column where the card is not. This is
  exactly the "nothing happened and the user wasn't told" outcome `BoardMoveOutcome` was designed
  to make unrepresentable, and the Observability Plan's "No outcome is silent — enforced
  structurally" claim is false as written. Compounding it: Story 2.3.1 AC #5 mocks `pauseSession`
  to **reject**, so the unit test goes green against behaviour production never exhibits.
  — **Recommendation**: `useBoardMove` must branch on the *return value* (`false` for archive,
  `null` for pause/resume/resumeHibernated) → `{kind:"failed"}`, sourcing the message from the
  `setError` value now in `sessionsSlice` or a generic fallback. Rewrite AC #5 to mock a
  **resolved** `false`/`null` and keep a rejection case alongside it. Also decide what the global
  `setError` banner does when a board move fails — right now it double-surfaces.

- [ ] **B2 — `resolveMoveVerb` switches only on `intent.to`, so it emits no-op verbs for moves
  out of `needsReview`.** Task 1.2.2b defines `"running"` → `resume` (or `resumeHibernated` when
  `status === HIBERNATED`). But by ADR-003 rule 2 a card in the Needs Review column is typically
  `SessionStatus.ACTIVE` — dragging it to Running resolves to `resume`, i.e.
  `updateSession(id, {status: RUNNING})` on an already-running session. Symmetrically, a
  `CREATING`/`RESTORING` card (ADR-003 rule 4 puts both in Running) dragged to Paused fires
  `pauseSession` on a session that has not started. Story 1.2.2's ACs cover `from === "complete"`,
  `to === "needsReview"`, and `from === to`, but never `from === "needsReview"` → `to === "running"`.
  The Story 4.1.1 4×4×9 matrix only asserts the verb is in the legal *set*, not that it is
  meaningful for the given status — so it passes too. Stacked on B1, the resulting server
  rejection is invisible to the user.
  — **Recommendation**: make `resolveMoveVerb` total over `(from, to, session.status)` and return
  `null` whenever the target status already equals `session.status`. Add an AC for the
  `needsReview → running` case and strengthen the 4×4×9 matrix to assert no emitted verb targets
  the session's current status.

- [ ] **B3 — Phase 2 is declared "independently shippable … satisfying every Success Metric";
  it does not satisfy Success Metric #2.** `requirements.md:18` requires that "search, tag
  grouping, bulk select are not degraded or duplicated with divergent behavior **when board view
  ships**." Search (Story 3.2.1), bulk select (3.2.2) and the swimlane axis (3.2.3) are all in
  Phase 3. A Phase-2-only ship is a board with no search box, no bulk select, no axis switch, and
  a hard `COLUMN_RENDER_CAP = 50` whose overflow footer's only remedy is *"switch to list view."*
  This is not a phantom split — Phase 2 is real, substantial work and the accessible-path-first
  sequencing is genuinely the right call — but the shippability claim in the Phase 2 header is an
  overclaim, and merging Phase 2 alone would regress the user against the list on exactly the
  capabilities the requirements protect.
  — **Recommendation**: the plan's own Dependency Visualization notes Epic 3.2 is "independent of
  3.1.*, parallelizable." Move Story 3.2.1 (search) — and preferably 3.2.2 — into Phase 2, or
  amend the Phase 2 header to state it satisfies Success Metrics #1 and #3 only and is not a
  standalone user-facing release.

## Concerns

- [ ] **C1 — No connection/staleness handling anywhere in the board.** `connectionState` and
  `reconnectAttemptCount` are on `SessionServiceContextValue` (`SessionServiceContext.tsx:26,28`)
  but `rg` finds **zero** references in `SessionList.tsx`, `PaneSplitRenderer.tsx`, or
  `app/page.tsx`. The board being copied, `BacklogBoard.tsx`, explicitly renders "one
  ConnectionIndicator per board" (comment at `:289`). The session board plan has no story, task,
  or AC for a dropped `WatchSessions` stream: columns keep rendering the last snapshot with no
  staleness signal, and a "Move to…" acts on a possibly-minutes-old view. The board makes
  staleness actionable in a way the list never did — an at-a-glance column count that is silently
  stale is worse than no board.
  — **Recommendation**: add a story rendering the existing connection state in the board header
  (mirror `BacklogBoard`'s `ConnectionIndicator`), and either disable move controls or add a
  `BoardMoveRejection` variant while disconnected.

- [ ] **C2 — No timeout, and no guard against a second move while one is in flight.**
  `useBoardMove` clears `pending` in a `finally`, but if the Connect call hangs (server wedged,
  stream down) the promise may not settle for a long time and the card sits `aria-busy` with no
  escape. `BoardMoveRejection` has no "already pending" variant. Story 2.1.1 makes the *card*
  non-interactive when pending, but Story 3.1.2's drag path never consults `pending` — so a drag
  can fire a second RPC for a session whose menu-initiated move is still in flight.
  — **Recommendation**: put an `AbortSignal`/timeout on the move, and add a `"move-in-flight"`
  rejection checked inside `useBoardMove` itself rather than only at the card.

- [ ] **C3 — Per-workspace persistence (the requirement) was silently substituted with per-pane
  persistence.** `requirements.md:19` Success Metric #3 reads "View choice (list vs. board)
  persists across reloads **per workspace**." Task 2.2.1c persists under `pane-${pane.id}.`.
  VERIFIED: pane ids are `crypto.randomUUID().slice(0, 8)` (`web-app/src/lib/pane/paneUtils.ts:9-10`)
  and `usePaneReducer` **clears its localStorage on `RESET_LAYOUT`** (`usePaneReducer.ts:105,121`).
  A layout reset therefore discards the view preference and orphans the key permanently. Unlike
  the ADR-003 column-order deviation, this one is **not** logged as an Unresolved Question.
  — **Recommendation**: log it as a fifth Unresolved Question, or persist at an unprefixed
  workspace-level key with the pane key as an optional override.

- [ ] **C4 — The `showArchived` refetch effect has no owner, and the board cannot turn
  `showArchived` on.** VERIFIED: `SessionList.tsx:472-481` is a **separate** effect from the
  eleven `saveToStorage` effects Task 1.1.1a moves, and it calls the `onFetchArchivedSessions`
  prop supplied at `PaneSplitRenderer.tsx:191`. Task 1.1.1a never mentions it; `SessionBoardProps`
  (Task 2.1.3a) never lists it; and Story 2.2.1's own AC asserts `show-archived-toggle` is `null`
  in board mode. Net: a board inheriting `showArchived: true` from the list never issues the
  `includeArchived` fetch, so its Complete column silently under-reports — and Story 2.3.3's
  "Show archived" toast action has **no control on the board to flip**.
  — **Recommendation**: decide explicitly — move the refetch effect into `useSessionFilters` and
  thread `onFetchArchivedSessions` into the board, then either give the board its own archived
  toggle or make the toast action switch the pane back to list view.

- [ ] **C5 — `needsReviewIds` will not be referentially stable in production, contradicting
  Story 1.1.3's stated rationale.** `approvals` comes from `useGetApprovalsQuery` with
  `pollingInterval: 5000` (`web-app/src/lib/contexts/ApprovalsContext.tsx:39-41`). RTK Query has
  no structural sharing, so each poll yields a fresh array identity, invalidating the
  `useMemo([items, approvals, clearedSessions])` every 5 s and returning a new `Set` — which
  invalidates the board's column `useMemo` and re-renders up to 4 × 50 = 200 cards on a
  five-second cadence, against a perf SLO (`requirements.md:30`) the plan satisfies "by
  construction." Story 1.1.3's AC #2 is written against referentially-identical inputs, so it
  passes while the production property it exists to guarantee does not hold. (Also: the glossary
  types `clearedSessions` as `Set<string>`; it is `ReadonlySet<string>` at
  `ApprovalsContext.tsx:18`.)
  — **Recommendation**: memoize on a derived stable key (sorted joined ids) rather than array
  identity, and add an AC asserting stability across a simulated poll returning an equal-but-new
  `approvals` array.

- [ ] **C6 — Story 2.2.2's "only the focused pane responds" has no wiring task.** VERIFIED:
  `SessionListPaneBody` is invoked as `<SessionListPaneBody pane={pane} dispatch={dispatch} />`
  (`PaneSplitRenderer.tsx:263`) — it receives no `isFocused` and no `state`. `isFocused` is
  computed one level up in `PaneLeafComponent` from `state.focusedPaneId === pane.id`. Task 2.2.2a
  says to guard "on `isFocused` for this pane," but no task adds the prop. Separately,
  `SessionListPaneBody` has early returns for `loading`/`error` at `PaneSplitRenderer.tsx:154-163`,
  so the new `useState`/`useEffect` from Task 2.2.1c must be placed **above** them — a hook-order
  hazard nothing in the plan flags.
  — **Recommendation**: add a task threading `isFocused` (or `state`) into `SessionListPaneBody`,
  and note the hooks-before-early-return constraint in Task 2.2.1c.

- [ ] **C7 — `useNotifications` has no toast-with-named-action API; Story 2.3.3 requires two.**
  VERIFIED at `web-app/src/lib/contexts/NotificationContext.tsx:55-73,242-253`: the context
  exposes only `showActionToast(message, type: "success"|"error", key)` — a plain toast with **no**
  action button — and `showUndoToast(message, onUndo, durationMs)`, which renders the
  `notificationType: "undo"` variant. Story 2.3.3 requires a toast with a **"Show archived"**
  action and a toast with a single **"Retry"** action. Neither is expressible today. Task 2.3.3b
  budgets ~4 min and its Files list omits `NotificationContext.tsx` entirely — extending a shared
  provider used app-wide is not a 4-minute change.
  — **Recommendation**: either promote "add a generic action-toast variant" to its own story with
  its own blast-radius acceptance criteria, or downgrade both ACs to `showActionToast` text plus
  the live-region announcement and drop the action buttons from v1.

- [ ] **C8 — `COLUMN_RENDER_CAP = 50` will bite first on the one column with no natural bound.**
  The cap is a defensible trade — `research/stack.md`'s "no verified precedent for
  `GroupedVirtuoso` + dnd-kit" is a real finding, and the repo *does* already need
  `GroupedVirtuoso` + `useVirtualizer` for the list (`SessionList.tsx:5-6`), which is evidence
  session counts get large. But the Complete column is fed by every `STOPPED` session that
  accumulates over time, and the escape hatch ("+N more — switch to list view") sends the user out
  of the feature on the board's most-likely-to-overflow lane. "Revisit only if a column is
  empirically observed above 50 — record the observation" has no mechanism that would ever record
  it.
  — **Recommendation**: keep the cap, but (a) sort Complete newest-first so truncation is
  meaningful, (b) make the overflow footer expand the column rather than abandon the board, and
  (c) emit a `console.warn` when any column exceeds the cap so the revisit trigger actually fires.

- [ ] **C9 — `@dnd-kit`'s 2024-12-05 last-publish is accepted risk with no re-check trigger and
  no contingency task.** ADR-001 names `@hello-pangea/dnd@18.0.1` as a "fallback only" contingency
  but no story, task, or timebox covers switching to it, and ADR-001's "re-check at the next
  dependency audit" names no date or owner. Two of the four Unresolved Questions (overlay
  portalling, pane `draggable={hasSplits}` collision at `PaneSplitRenderer.tsx:243`) are
  concentrated in Epic 3.1 and are both resolvable only by manual browser checks — so the realistic
  failure mode is "Phase 3 stalls mid-epic." Adopting the library is still the right call on the
  evidence (React 19 peer support, `KeyboardSensor`, MIT, no runtime CSS-in-JS); the gap is the
  absence of an abort path.
  — **Recommendation**: add an explicit abort criterion to Story 3.1.3 ("if 3.1.3b/c are not
  resolved in one sitting, ship Phase 2 and re-open Phase 3 as its own PR") — which only works if
  B3 is fixed first — and put a dated re-check into the ADR.

## Minors

- ADR-003 §3's column-order override of `requirements.md:38` is unrequested: the requirement lists
  Running first, and the ADR's justification ("reads as prose enumeration rather than a designed
  ordering") is an assumption about the author's intent on a UX decision they did not ask to have
  revisited. The process handling is correct — logged as a vetoable Unresolved Question, cost of
  reversal one array — but it blocks Story 2.1.2, which sits on the stated critical path
  (`1.1.3 → 1.2.1 → 2.1.2 → …`). Get the veto before Phase 1 finishes, not at Story 2.1.2.
- Task 1.1.1a says "the eleven `useState` initialisers at `SessionList.tsx:317-355`". That range
  holds **twelve** (`searchQuery`, `selectedStatus`, `selectedCategory`, `selectedTag`,
  `hidePaused`, `showArchived`, `filterNeedsApproval`, `groupingStrategy`, `collapsedGroups`,
  `sortField`, `sortDir`, `visibleColumns`), plus `columnPickerOpen` at `:357`. Four of them
  (`collapsedGroups`, `visibleColumns`, `columnPickerOpen`, and arguably `groupingStrategy` given
  Story 3.2.3 gives the board its own axis key) are list-only UI state the board will mount and
  never use — mild interface pollution in a hook whose whole justification is shared use.
- All 72 tasks are estimated at 2–5 minutes (~4 hours total) against a Medium 1–2 week appetite
  covering a 1601-line-component refactor, a new dependency, 12 new test files, an e2e spec, and
  two manual browser verifications. Not load-bearing for correctness, but any scheduling built on
  these numbers will be wrong by an order of magnitude. Task 1.1.1b ("Move the derived
  filter/sort/group memos (~5 min)" — ~100 lines out of a 1601-line component, keeping four suites
  byte-identically green) is the clearest example.
- Task 2.2.2a's claim that `rg -n 'key === "b"'` finds no existing binding is VERIFIED — zero hits
  across `web-app/src`. Note there are two precedents for unmodified single-letter globals on this
  surface: `e.key === "n"` (`OmnibarContext.tsx:172`, already guarded by an `isInputElement`
  helper) and the `d` delete-confirm shortcut (`app/page.tsx:477`). Reuse `isInputElement` rather
  than re-implementing the `INPUT`/`TEXTAREA`/`isContentEditable` guard a third time.
- Story 3.2.2 covers cross-column bulk select but says nothing about the `Escape`-clears /
  `Cmd+A`-selects-all document listeners that Story 1.1.2 moves into `useSessionSelection`. Those
  will now be live on the board too; no AC pins their board behaviour, and `Cmd+A` selecting
  across all four columns at once is worth one explicit assertion.
- The Definition of Done requires `make ci`, which the plan correctly notes leaves the Go side
  untouched. Given `.claude/rules/fix-flaky-tests-dont-defer.md`, add a line stating what to do if
  a pre-existing Go flake fires on that run — otherwise the first red `make ci` becomes an
  ad-hoc judgement call at the worst moment.
