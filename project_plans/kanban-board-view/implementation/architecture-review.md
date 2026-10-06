# Architecture Review: kanban-board-view
**Date**: 2026-08-06
**Verdict**: BLOCKED

**Reviewer lenses**: structural integrity (SOLID / layering / testability), type-level design
(primitive obsession, illegal states, parse-at-boundary), pattern selection (PoEAA / GoF / API
contract stability), plus a targeted pass against `.claude/rules/interface-pollution-checklist.md`.

**Evidence discipline**: every finding below cites a file and line that was opened, or a command
that was run. Items I could not verify are labelled UNVERIFIED and carry a proposed verification
step rather than a conclusion.

## Constitution Violations

None — **no architecture constitution exists in this repo.** `ls docs/adr/ADR-000*` and
`ls docs/adr/000*` both return no matches (VERIFIED). `docs/adr/` holds 27 topic ADRs
(`003-no-static-sleeps-in-tests.md` … `ADR-026-mergeability-state-synthesis.md`) but no
`ADR-000-architecture-constitution.md`. The closest binding documents are
`.claude/rules/css-architecture.md` (ADR-009), `.claude/rules/interface-pollution-checklist.md`,
and `.claude/rules/e2e-test-conventions.md`, all of which the plan cites and, with the exceptions
noted below, complies with.

## Blockers

- [ ] **Story 1.2.2 / Task 1.2.2b — `resolveMoveVerb` as specified cannot satisfy its own
      acceptance criteria; legality is a `(from, to)` relation, not a function of `to`.**
      Task 1.2.2b says: "`"complete"` → `"archive"`; `"paused"` → `"pause"`; `"running"` →
      `session.status === HIBERNATED ? "resumeHibernated" : "resume"`; `"needsReview"` → `null`.
      Guard `intent.from === intent.to` → `null` before the switch." Run that algorithm against
      Story 1.2.2's own third criterion — `{from: "complete", to: "running"}` — and it returns
      `"resume"`, not `null`. The criterion and the task contradict each other. Worse,
      `{from: "complete", to: "paused"}` returns `"pause"`, which would fire
      `updateSession(id, {status: PAUSED})` on an archived, `STOPPED` session — directly
      contradicting ADR-002's "Dragging *out* of the Complete column is a rejected drop target."
      **Remediation**: express legality over the pair. Before the `switch`, add explicit guards
      `if (intent.from === intent.to) return null;` and `if (intent.from === "complete") return
      null;` and `if (intent.to === "needsReview") return null;`, then switch on `to` for the
      verb. Rewrite Story 4.1.1's 4×4×9 matrix criterion to assert the **full legality table**
      (which pairs are `null`), not merely that non-`null` results belong to the verb set — the
      current assertion passes even with the bug present.

- [ ] **Story 2.3.2 / ADR-003 §2 — moves out of the Needs Review column fire an RPC and visibly
      do nothing.** The precedence cascade puts `needsReviewIds.has(id)` (rule 2) *above*
      `PAUSED`/`HIBERNATED` (rule 3) and above the running catch-all (rule 4). So for a card in
      `needsReview`: `→ "paused"` resolves to `"pause"`, the RPC succeeds, and
      `resolveBoardColumn` still returns `"needsReview"` because the review item is untouched.
      `→ "running"` behaves the same. Only `→ "complete"` actually changes the resolved column
      (rule 1 outranks rule 2). Under the plan's **pessimistic** commit model the user sees a
      busy state, then the card sitting exactly where it was — indistinguishable from the failure
      mode `BoardMoveOutcome` was designed to make unrepresentable. Story 2.3.2's criteria only
      specify the menu contents for a card in `"running"`; the `needsReview` case — the column
      the whole feature exists for (`research/ux.md` §5, cited in ADR-003 Rationale) — is
      unspecified.
      **Remediation**: pick one and pin it with an acceptance criterion. (a) Make `moveTargets`
      `from`-aware so a `needsReview` card offers only `"Complete"`; or (b) keep pause/resume
      legal but require Story 2.3.3 to announce the non-move explicitly ("Paused 'fix-login-bug'
      — stays in Needs Review until the approval is resolved") and require `SessionBoardCard` to
      render the new status chip so *something* observably changes. Option (a) is the smaller
      diff and follows from the same "legality is a `(from, to)` relation" fix as the blocker
      above.

- [ ] **Story 2.3.3 / Task 2.3.3a — the live region the board announces into does not exist in
      board mode.** `#bulk-feedback-live` is rendered **inside `SessionList`**, at
      `web-app/src/components/sessions/SessionList.tsx:1167`, and
      `rg -n "bulk-feedback-live" web-app/src` returns exactly that one hit (VERIFIED). Story
      2.2.1 renders `<SessionList>` **or** `<SessionBoard>` by ternary, so in board mode
      `SessionList` is unmounted and `document.getElementById("bulk-feedback-live")` is `null`.
      Task 2.3.3a's fallback — "else through `showFeedback` from `useSessionSelection`" — does not
      help: `showFeedback` only sets `bulkFeedback`, whose sole renderer is that same unmounted
      node at `:1167`. Every Story 2.3.3 announcement, plus the archived-disappearance
      announcement ADR-002's Consequences section depends on, is a silent no-op. The Pattern
      Decisions rationale ("that node already solves the survive-unmount-mid-announcement
      problem") is false for the one unmount that matters — the view toggle itself.
      **Remediation**: hoist the live region out of `SessionList` into `SessionListPaneBody`
      (`web-app/src/components/pane/PaneSplitRenderer.tsx`), rendered **outside** the ternary so
      it survives both views, and pass a `showFeedback` callback down to both. Add this as a task
      under Story 2.2.1 (it is a prerequisite of 2.3.3, not part of it). Add an acceptance
      criterion asserting `document.getElementById("bulk-feedback-live")` is non-null while
      `dashboardViewMode === "board"`.

- [ ] **Story 2.3.1 / ADR-002 — the pessimistic commit model has no state change to wait for on
      the Complete column.** Two independent verified facts:
      (a) **Client**: `archiveSession` (`web-app/src/lib/hooks/useSessionService.ts:600-612`)
      returns `Promise<boolean>` and **dispatches nothing** — contrast `updateSession`, which
      dispatches `upsertSession(response.session)` at `:323-325`, the exact evidence the plan's
      Pattern Decisions table cites for choosing pessimistic. That evidence does not generalise
      to the archive verb.
      (b) **Server**: `ArchiveSession` (`server/services/session_service.go:4276-4298`) calls
      `inst.ArchiveWithStop(now)` then `s.storage.SaveInstances(...)` and returns — with **no**
      `s.eventBus.Publish(...)` anywhere in the handler, unlike `RestartSession`
      (`:2983`), `UpdateSession` (`:1884-1892`), and the pause/resume paths (`:1954`, `:1999`).
      So a successful drag-to-Complete may set `pending`, resolve, clear `pending`, and leave the
      card in its origin column with nothing having visibly happened.
      **UNVERIFIED**: `ArchiveWithStop` → `stopIfNotStoppedLocked`
      (`session/instance_actor_setters.go:255-259`) *might* fire the `EventExited` lifecycle
      event that `sessionExitedPublisher` (`server/services/session_service.go:3895-3903`)
      listens for, which would publish a `SessionUpdatedEvent` asynchronously. That is a
      plausible path, not a confirmed one.
      **Remediation**: add a task **before** Story 2.3.1 that (1) verifies at runtime whether a
      `WatchSessions` event arrives after an `ArchiveSession` call, and (2) if it does not,
      dispatches `upsertSession`/`removeSession` client-side inside `archiveSession` on success
      (a two-line change, symmetric with `updateSession`) or publishes the event server-side.
      Until that is settled, ADR-002's chosen RPC does not satisfy the pessimistic model the plan
      committed to, and the two decisions are incompatible as written.

- [ ] **Story 2.3.1 / Task 2.3.1a — none of the four verbs' RPCs throw, so `useBoardMove` as
      specified reports `{kind: "applied"}` on every failure.** `updateSession` catches and
      returns `null` (`useSessionService.ts:331-337`), and `pauseSession`/`resumeSession` are
      thin wrappers over it (`:364-383`). `archiveSession` catches and returns `false`
      (`:606-609`). `resumeHibernatedSession` catches and returns `null` (`:412-415`). Task
      2.3.1a says "`await` the verb's RPC … clear `pending` in a `finally`" with no return-value
      check, so the absence of a throw is treated as success. Story 2.3.1's fifth criterion —
      "*Given* `pauseSession` rejecting with `new Error("session already paused")`" — describes
      behaviour the production hook cannot produce; the test passes against a mock that is unlike
      the real dependency, which is the definition of a self-testing guard. This is the exact
      silent-failure mode `BoardMoveOutcome`'s union was introduced to make unrepresentable, and
      it defeats it at the boundary.
      **Remediation**: this is a parse-at-boundary problem — add a small verb→executor table that
      normalises each RPC's sentinel return into an outcome:
      `pause|resume|resumeHibernated: (id) => (await rpc(id)) !== null`,
      `archive: (id) => await archiveSession(id)`, with a shared
      `false → {kind:"failed", message: <last error>}` mapping. Rewrite the criterion to use the
      real shapes (*Given* `pauseSession` **resolving** `null`) and add a second criterion for
      `archiveSession` resolving `false`. Note the error text itself is only reachable via
      `sessionsSlice`'s `error` (set by `dispatch(setError(...))`), so `message` must either read
      that or be a generic per-verb string — decide which, explicitly.

- [ ] **`COLUMN_RENDER_CAP` × Story 3.2.2 — Select All can select, and bulk-delete, sessions the
      board never rendered.** `activeSelection` is the intersection of `selectedSessions` with
      `filteredSessions` (`SessionList.tsx:639-642`, which Story 3.2.2's second criterion
      explicitly reuses), while the board renders at most 50 cards per column (Story 2.1.2, third
      criterion: 63 → 50 + footer). `BulkActions` exposes `onDeleteAll`
      (`web-app/src/components/sessions/BulkActions.tsx:15`, VERIFIED). So on a board with an
      overflowing column, "Select all" → "Delete all" destroys sessions — and their worktrees —
      that were never on screen. This is the same hazard class ADR-002 was written to eliminate
      ("A misread drag that deletes destroys a git worktree and its tmux session"), reintroduced
      through a different door. In the list view the cap does not exist, so everything selectable
      is reachable by scrolling; the board's cap breaks that invariant.
      **Remediation**: on the board, compute `activeSelection` over **rendered** card ids rather
      than `filteredSessions`, or disable Select All (with a tooltip naming the reason) whenever
      any column overflows. Add an acceptance criterion to Story 3.2.2: *Given* 63 sessions in
      `"complete"` and `selectMode`, *When* the user activates Select All, *Then*
      `selectedCount === 50` (or the control is `aria-disabled`), never `63`.

## Concerns

- [ ] **Task 1.1.1b — `useSessionFilters` acquires a network dependency, breaking its
      testability and its single responsibility.** The task moves `useInsightsSummary` into the
      hook. That hook is not a selector: it builds its own Connect transport and client and opens
      a watch stream (`web-app/src/lib/hooks/useInsightsService.ts:39-56`). A hook described as
      "search / status / category / tag / hidePaused / showArchived / filterNeedsApproval / sort
      + their localStorage persistence" would then own an RPC client, and every new board test
      (Stories 2.1.2c, 2.1.3c, 3.2.1b, 3.2.2c) inherits a transport to stub.
      **Recommendation**: leave `useInsightsSummary` in the view and pass `costById` into
      `useSessionFilters` as an argument — exactly the treatment Task 1.1.1b already applies to
      `pendingDeleteIds`. The hook stays pure state + derivation and is unit-testable with no
      providers.

- [ ] **Task 1.1.1a — the `showArchived` re-fetch effect is inside the moved line range but is
      not a persistence effect, and no task assigns it an owner.** `SessionList.tsx:476-480` is
      an effect that calls the `onFetchArchivedSessions` **prop** when `showArchived` flips; it
      sits inside the `:449-504` block Task 1.1.1a describes as "their eleven `saveToStorage`
      effects." If it moves, the hook gains a second infrastructure concern; if it stays,
      toggling Show Archived on the board fetches nothing, archived sessions never enter the
      store, and the Complete column stays empty — silently contradicting ADR-002's whole
      Consequences discussion.
      **Recommendation**: thread it explicitly —
      `useSessionFilters({ sessions, storageKeyPrefix, pendingDeleteIds, onFetchArchivedSessions })`
      — and add an acceptance criterion to Story 1.1.1 that enabling `showArchived` invokes the
      callback exactly once, from either view.

- [ ] **Story 3.2.3 — there is no lane type; `BoardColumnKey` cannot express a grouping lane.**
      The glossary defines `BoardColumn` as `{ key: BoardColumnKey; label; emptyLabel }`, and
      Task 3.2.3b says grouping-axis lanes are "mapped to the same lane shape." A tag or category
      lane's key is an arbitrary user string, so satisfying that instruction means widening `key`
      to `string` — which destroys the exhaustive-`switch` guarantee that is the stated reason
      `BoardColumnKey` is a sum type at all. The `data-testid` scheme has the same problem:
      `board-column-${key}` becomes `board-column-my tag/with spaces`.
      **Recommendation**: add a `BoardLane` sum type to the Domain Glossary —
      `{ kind: "status"; column: BoardColumn; sessions: Session[] } | { kind: "group"; slug: string; label: string; sessions: Session[] }`
      — and have `SessionBoardColumn` accept `BoardLane`. Keep `BoardColumnKey` exclusively for
      the status axis and for `resolveMoveVerb`. Specify a slug function for group testids.

- [ ] **Domain Glossary — `BoardMoveVerb` folds `null` into the verb union, creating three
      distinct "nothing" states.** With `BoardMoveVerb = "pause" | … | null`, the declared
      `pending: Record<string, BoardMoveVerb>` admits `pending[id] === null` (meaningless) and
      `SessionBoardCard`'s `pendingVerb: BoardMoveVerb | undefined` admits `null` **and**
      `undefined` for "not pending." That is an illegal state the type system is being used to
      *permit* rather than forbid — the opposite of the glossary's own stated intent.
      **Recommendation**: `type BoardMoveVerb = "pause" | "resume" | "resumeHibernated" |
      "archive"` (no `null`); `resolveMoveVerb(...): BoardMoveVerb | null`;
      `pending: Partial<Record<string, BoardMoveVerb>>`; `pendingVerb?: BoardMoveVerb`.

- [ ] **`BoardMoveRejection["readonly-axis"]` is unreachable — the guard lives only in the
      view.** No story or task produces that rejection: Story 2.3.1 covers `illegal-transition`
      and `session-gone`, Story 3.1.5 covers `stale-source-column`, and Story 3.2.3 enforces
      read-only purely presentationally (`aria-disabled`, no grip handles). A disabled control is
      not an invariant; a stale menu, a keyboard path, or a future caller bypasses it, and the
      executor will happily archive a session from a tag lane.
      **Recommendation**: pass the current `SwimlaneAxis` into `useBoardMove` and return
      `{ kind: "rejected", reason: "readonly-axis" }` from `move()` when
      `axis.kind !== "status"`. Add the matching acceptance criterion to Story 3.2.3 — enforce
      at the executor, decorate at the view.

- [ ] **`useNeedsReviewSessionIds` has no production call site in the plan.** Story 1.1.3 creates
      it, Story 2.1.3a lists `needsReviewIds` among `SessionBoardProps`, and Task 2.2.1c (the
      wiring task) mentions only the toggle and the ternary. No task invokes the hook or threads
      its result. **Recommendation**: extend Task 2.2.1c to call `useNeedsReviewSessionIds()` in
      `SessionListPaneBody` and pass it to `<SessionBoard>` — and state whether the list view
      also consumes it (per ADR-003 §1's "the board must not disagree with a badge the list
      already shows", it arguably should, replacing `SessionList.tsx:304-308`'s local
      derivation).

- [ ] **Story 2.1.2 and Story 3.2.1 give the column header two contradictory count semantics.**
      2.1.2: "Each column header shows its label and its **exact card count**." 3.2.1: "the count
      is **over rendered cards** so it never contradicts what the user can see." With
      `COLUMN_RENDER_CAP`, a 63-session Complete column makes these `63` and `50`. Capping the
      count also defeats Success Metric #1 (`requirements.md:17` — "identify … how many sessions
      are in each of Running / Needs Review / Paused / Complete").
      **Recommendation**: header shows the **filtered total**; the overflow footer carries the
      rendered-vs-total gap ("+13 more"). Fix Story 3.2.1's wording and add the 63-session case
      to Story 2.1.2's header criterion so the two cannot drift.

- [ ] **Selection state is per-view and silently lost on toggle.** Both views call
      `useSessionSelection` independently (Story 3.2.1 states the hooks are deliberately not
      hoisted), and unlike the filters there is no localStorage backing for `selectedSessions`.
      Pressing `b` with three cards selected drops the selection with no announcement. The plan's
      own Pattern Decisions row argues parallel state is "precisely the 'duplicated with
      divergent behavior' outcome `requirements.md:18` prohibits" — and then ships parallel
      instances, relying on last-writer-wins localStorage for the filter half and nothing at all
      for the selection half. The claim in the glossary that both views "read one copy" of the
      state is not accurate; they share *logic*, not state.
      **Recommendation**: either hoist `useSessionSelection` into `SessionListPaneBody` and pass
      values down through `SessionList`'s existing props, or add an explicit acceptance criterion
      that a view switch clears the selection and announces it. Also correct the glossary wording
      to "one implementation," not "one copy."

- [ ] **`SessionBoard.tsx` is planned to absorb ten responsibilities — the same accretion
      `requirements.md:63` flags for `SessionList.tsx` (1601 lines, VERIFIED via `wc -l`).**
      Assigned to that one file: the shell, `SessionBoardColumn`, enter/exit transition machinery
      (~60 copied lines), `DndContext` + sensors, `DragOverlay`, drop-legality/hover affordances,
      stale-drag re-validation with ref-backed handlers, the search header, `BulkActions`, the
      swimlane axis selector, announcements, and focus restoration. `SessionCard.tsx` is already
      893 lines as a cautionary comparison.
      **Recommendation**: pre-declare two seams justified by **testability**, not by speculative
      reuse (so the interface-pollution checklist is not violated): `useBoardColumnTransitions(
      columnBySessionId)` — the ref+timer state machine, which Story 2.1.3c wants to test with
      fake timers anyway and which is far easier to test outside a rendered board — and
      `SessionBoardHeader.tsx` (toggle + search + axis + bulk). Both have one consumer; the
      justification is isolation of stateful logic, which is the checklist's §4 carve-out ("only
      add a wrapping type when it contributes real behaviour"), not reuse.

- [ ] **`useSessionFilters`' returned surface is very wide, and three of its keys are list-only.**
      `BASE_STORAGE_KEYS` (`SessionList.tsx:223-236`) has twelve entries; `COLLAPSED_GROUPS`,
      `VISIBLE_COLUMNS`, and `GROUPING_STRATEGY` are list-chrome concerns the board never reads
      (the board gets its own `BOARD_SWIMLANE_AXIS` key per Story 3.2.3a). Mounting the hook on
      the board therefore mounts persistence effects for state it does not use — an Interface
      Segregation smell and needless localStorage churn on every board render pass.
      **Recommendation**: either return a nested shape (`{ filters, sort, listChrome }`) so each
      consumer destructures only its slice, or split the three list-only keys into a small
      `useSessionListChrome` that only `SessionList` mounts.

## Nitpicks

- `SessionStatus` uses `option allow_alias = true` (`proto/session/v1/types.proto:326`), so
  `ACTIVE` and `RUNNING` are both wire value `1` (`:331-333`) — ADR-003 §Context says "Nine
  `SessionStatus` enum values exist" while Story 4.1.1's array lists **ten** identifiers over nine
  distinct values, and Story 4.1.1b's "4×4×9 matrix" is really 4×4×(9 distinct). Harmless for
  `resolveBoardColumn` (rule 4 is a catch-all), but any future exhaustive `switch` on
  `SessionStatus` cannot have both alias cases. Worth one sentence in the `boardColumns.ts` header
  comment.
- Recount the counts the plan asserts: Task 1.1.1a says "eleven `useState` initialisers at
  `:317-355`" and "eleven `saveToStorage` effects at `:449-504`". `grep -c useState` over
  `SessionList.tsx:310-360` returns **14**, and `:449-506` contains **twelve** `saveToStorage`
  effects plus the archived re-fetch effect. Per the repo's evidence rule, derive these by command
  rather than by estimate before an implementer treats them as a checklist.
- `COLUMN_RENDER_CAP` is a rendering budget exported from `lib/board/boardColumns.ts` (Task
  1.2.1b), the module the plan otherwise frames as the pure domain model. Put it in
  `SessionBoard.tsx` or a `boardLayout.ts` so the domain module stays render-agnostic.
- `DashboardViewMode` is restored from localStorage via `loadFromStorage(..., "list")` with no
  validation (Task 2.2.1c). A stale or hand-edited value falls through the ternary to list mode,
  which is safe by accident rather than by parse. A three-line
  `parseViewMode(raw): DashboardViewMode` would make it safe by construction, consistent with the
  plan's own parse-don't-validate framing elsewhere.
- Copying ~60 lines of transition machinery out of `BacklogBoard.tsx:168-284` (Task 2.1.3b) is the
  right call per the interface-pollution checklist, but add a cross-reference comment in **both**
  files ("a near-copy of this machinery lives in …; fix both or extract") so a future bug fix in
  one is discoverable from the other. The plan currently annotates only the new side.
- ADR-001 keeps `@dnd-kit/sortable` with an instruction to drop it if `SortableContext` proves
  unused. Nothing in Phase 3's stories uses `SortableContext` (3.1.2b specifies `useDraggable`/
  `useDroppable` only), so the likely outcome is already visible at planning time — consider
  starting with `@dnd-kit/core` + `@dnd-kit/utilities` and adding `sortable` only if within-column
  ordering appears.
