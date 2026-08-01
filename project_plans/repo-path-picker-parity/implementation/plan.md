# Implementation Plan: repo-path-picker-parity

**Feature**: Swap the Omnibar's two plain-text repo-path inputs (New Project's Parent
Directory, Existing Branch's worktree-path fallback) for the shared `RepoPathInput`
component, fix the recency ordering + Escape-propagation bugs that reuse surfaces, and
close a pre-existing a11y gap in the shared component.
**Date**: 2026-08-01
**Status**: Ready for implementation
**ADRs**: None

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `RepoPathInput` | Shared React component (`web-app/src/components/ui/RepoPathInput.tsx`) combining a controlled text `<input>` with a history+filesystem suggestion dropdown. The single reuse target for this feature — no new component is built. | Existing 4 consumers: `BacklogItemForm`, `NewShellDialog`, `WorkflowForm`, `LocalFileBrowser`. |
| `PathCompletionDropdown` | Presentational `<ul role="listbox">` rendered by `RepoPathInput` when `showDropdown` is true; renders `CompletionEntry[]` rows, history entries first (clock icon) then filesystem entries (folder icon), separated by a divider when both groups are non-empty. | `web-app/src/components/ui/PathCompletionDropdown.tsx`. |
| `CompletionEntry` | `{ name, path, isDirectory, isHistory }` — one dropdown row. `isHistory: true` marks a row sourced from session history rather than a live filesystem listing. | Type exported from `PathCompletionDropdown.tsx`. |
| `useSessionRepoPaths` | Hook returning a deduplicated `string[]` of past session root paths, used by `RepoPathInput` as its history source. Currently sourced from `selectAllSessions` (raw adapter order); this plan swaps it to `selectActiveSessionsSortedByUpdatedAt`. | `web-app/src/lib/hooks/useSessionRepoPaths.ts`. Confirmed sole caller: `RepoPathInput`. |
| `usePathCompletions` | Hook that debounces (150ms) and RPC-fetches live filesystem directory entries for the current input value, with an LRU+TTL cache. Untouched by this plan. | `web-app/src/lib/hooks/usePathCompletions.ts`. |
| `selectActiveSessionsSortedByUpdatedAt` | Memoized Redux selector (`createSelector`) returning sessions with `status !== SessionStatus.UNSPECIFIED`, sorted by `updatedAt.seconds` descending. This plan extends its comparator with a two-level tiebreak. | `web-app/src/lib/store/sessionsSlice.ts:117-123`. |
| Recency tiebreak | The three-level comparator this plan adds: `updatedAt.seconds` desc → `createdAt.seconds` desc → `id` ascending (string compare). Guarantees a total, deterministic order even when both timestamps are absent/zero for two sessions. | `id` is `Session.id: string`, always defined and unique. Ascending is the deliberate, correct choice here — it matches Story 1.2.1's own Given/When/Then example (`"a-session" < "z-session"` sorts first) and Task 1.2.1a's implementation. `research/build-vs-buy.md` §3 has stale wording saying "desc" for this final tiebreak; that's an error in the research doc, not in this plan — this plan's ascending choice is correct and should not be "fixed" to match it. |
| Parent Directory field | The New Project mode's first text field (`id="omnibar-parent-dir"`) — the directory that will *contain* the new project folder. Does not need to exist as a full path (only its own parent chain must). | `OmnibarCreationPanel.tsx:486-501`. |
| Existing Worktree Path fallback field | The plain-text `<input id="omnibar-existing-worktree">` rendered only when git-worktree discovery (`worktrees` state) finds zero entries or errors; mutually exclusive with the `<select>` sibling rendered when worktrees are found. Out of scope: the `<select>` itself. | `OmnibarCreationPanel.tsx:651-660`. |
| Escape-propagation hazard | The bug where `RepoPathInput`'s Escape handler doesn't call `stopImmediatePropagation()`, so a bubbled Escape keydown also fires an ancestor's own Escape handler (e.g. `Omnibar.tsx`'s document-level `onClose()` listener, or `NewShellDialog.tsx`'s unconditional Escape→`onCancel()` listener). | Fixed component-locally in `RepoPathInput.tsx`, benefiting both known hazard sites. |
| `open` (RepoPathInput local state) | Boolean React state that becomes `true` on mere focus, even when the dropdown has nothing to render (e.g. empty history and no filesystem matches yet). Distinct from `showDropdown` — see next row. The Escape fix gates on `showDropdown`, NOT on this value directly, precisely because `open` can be `true` with an empty/invisible dropdown. | `RepoPathInput.tsx:55`. |
| `showDropdown` (RepoPathInput derived value) | `open && (allEntries.length > 0 || isLoading)` — the actual visible-listbox state, already used for `aria-expanded` (Task 1.1.2a). The Escape fix gates `stopImmediatePropagation()` on this being `true` at keydown time, not on `open`, so Escape only swallows the keystroke when a dropdown is actually visible. | `RepoPathInput.tsx:99`. |
| combobox a11y triad | The three ARIA attributes this plan adds to `RepoPathInput`'s `<input>`: `role="combobox"`, `aria-expanded={showDropdown}`, `aria-haspopup="listbox"` — completing the WAI-ARIA combobox pattern alongside the already-present `aria-autocomplete`, `aria-controls`, `aria-activedescendant`. | Pre-existing gap flagged in `research/ux.md`; benefits all 6 eventual consumers. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Parent Directory / Existing Worktree fields | Direct component substitution (swap `<input>` for `<RepoPathInput>`, controlled-component composition) | R1, `build-vs-buy.md` §1/§4 | (a) Build a second bespoke combobox for the omnibar; (b) pull in a new combobox library (`downshift`/`cmdk`/`react-aria`) | (a) duplicates logic the requirements doc explicitly forbids and creates two divergent path-picker UX flows in one app; (b) no functional gap exists to justify a new dependency for 2 call sites when a working in-house component already serves 4 |
| Escape-propagation fix location | Component-local fix inside `RepoPathInput.tsx`'s own `handleKeyDown`, gated on `showDropdown === true` (NOT `open` — `open` is `true` on mere focus even when the dropdown renders nothing, which would wrongly swallow Escape in the empty-dropdown case; `showDropdown` is the actual visible-listbox state, already used for `aria-expanded` in Task 1.1.2a), using `e.nativeEvent.stopImmediatePropagation()` | R6, `architecture.md` §1, `pitfalls.md` §3 | Wrap the two new `OmnibarCreationPanel` call sites in a keydown-swallowing wrapper div at the call site | A call-site workaround only patches the 2 fields this ticket touches and leaves `NewShellDialog.tsx`'s already-shipping identical bug (unconditional document-level Escape→`onCancel()`, no dropdown-open guard) unfixed; the owning component should own suppressing propagation of the keys that control its own UI, matching `Omnibar.tsx`'s own internal-dropdown precedent (lines 748/772/820/839) |
| `useSessionRepoPaths` recency source | Swap `selectAllSessions` → `selectActiveSessionsSortedByUpdatedAt` (existing memoized selector) inside the hook | R3, `architecture.md` §3 | Compute recency ordering locally inside `RepoPathInput` or `useSessionRepoPaths` itself (duplicate sort logic per-consumer) | The selector already exists, is already memoized via `createSelector`, and is the established pattern (`useSessionSearch.ts` already consumes the identical `UNSPECIFIED`-filter precedent) — duplicating sort/filter logic in the hook would diverge from that precedent for no benefit |
| Tiebreak comparator implementation | Hand-written 3-key `Array.prototype.sort` comparator, inlined in the existing `createSelector` call | R3, `build-vs-buy.md` §3, `stack.md` | Add `lodash.sortBy` (or similar) as a new dependency | No dependency exists today for this; a 3-key descending sort with `?? 0` coercion on possibly-undefined proto timestamps is not more clearly expressed by `sortBy` than a direct comparator, and adding a library for a self-contained ~5-line comparator is unjustified bundle/maintenance cost |
| `RepoPathInput` a11y attributes (`role="combobox"`, `aria-expanded`, `aria-haspopup`) | Include in this change's scope, additive-only (no behavior change) | `ux.md` §3 (explicit recommendation) | Defer to a separate follow-up ticket, since it's a pre-existing gap not introduced by this change | Small (3 attributes, no logic change), directly reduces the accessibility surface area this change is about to widen (2 new high-traffic consumers), and research explicitly recommends bundling it; tradeoff is a small scope-creep risk against shipping 2 more consumers of an under-labeled combobox — recommendation favors fixing now since the fix is additive and low-risk (see Unresolved Questions for the one open point: whether to `role=undefined` on the wrapping `<div>` vs. only the `<input>` — resolved below as input-only, matching the existing WAI-ARIA input-owns-combobox-role pattern already partially implemented via `aria-autocomplete` on the input) |
| Field-level `error`/validation for the two Omnibar fields | Out of scope — do not add new error-derivation logic to `OmnibarCreationPanel` for either field | `features.md` (no existing per-field error state today), requirements R1-R6 (none mention field-level errors) | Wire a new `errors.parentDir`/`errors.existingWorktree` object and pass through `RepoPathInput`'s `error` prop | No AC calls for this; `WorkflowForm` already sets precedent for `required` without `error`; adding new validation state is scope creep beyond "swap the input". Note: `research/ux.md` §4's recommendation to wire real-time/submit-time existence validation specifically for the Existing Worktree Path field was reviewed and is knowingly deferred as a follow-up — not missed or overlooked — because no AC in this ticket calls for it and it would expand scope beyond a component swap |
| New Project mode hint copy | Static, mode-specific hint string passed via `RepoPathInput`'s `hint` prop, replacing the generic hint only when `sessionType === "new_project"` | R1 (parity), `ux.md` §2 | Auto-truncate a selected history path to its directory name when in New Project mode | `ux.md` explicitly rejects data transformation as "smart but surprising"; violates R4's "what you picked is what you get" contract already established by the other 4 consumers |

---

## Observability Plan
- **Logs**: None added — this is client-side form UI with no new server-side code path; existing `usePathCompletions` RPC logging (server-side ConnectRPC handler logs) is unchanged.
- **Metrics**: None added — no new metric warrants a one-off UI-plumbing change of this size; if omnibar usage analytics exist elsewhere, this change does not add or remove instrumented events.
- **Alerts**: None applicable.

## Risk Control
- **Feature flag**: None — this is a pure component substitution with no behavior fork; shipping as a single atomic change is appropriate for a UI-plumbing fix of this size. Phases 1-3 ship as ONE PR, not staged across multiple PRs — there is no reason a change this small needs an interim merge point, and splitting it would only introduce a window where Phase 2 code depends on unmerged Phase 1 code. If broader rollout caution is desired, it can ship as a normal PR reviewed via screenshots/e2e rather than a flag, since rollback is a single revert.
- **Rollback procedure**: `git revert` the merge commit. No data migration, no schema change, no backend change — a revert fully restores prior behavior with no cleanup required.
- **Staged rollout**: Not applicable — internal tool, no user cohorting infrastructure in this app for frontend-only changes.

## Unresolved Questions
None.

## Dependency Visualization

```
Phase 1: Foundational fixes (shared component + selector — no consumer changes yet)
┌─────────────────────────────────────┐   ┌──────────────────────────────────────┐
│ Epic 1.1: RepoPathInput bugfixes     │   │ Epic 1.2: Recency ordering fix        │
│  1.1.1a Escape stopImmediatePropag.  │   │  1.2.1a Tiebreak comparator            │
│  1.1.1b combobox a11y attributes     │   │  1.2.1b useSessionRepoPaths swap       │
│  1.1.2a Unit test: Escape isolated   │   │  1.2.2a Unit tests: tiebreak           │
│  1.1.2b Unit test: Escape nested     │   │                                        │
└───────────────┬───────────────────────┘   └──────────────┬───────────────────────┘
                │                                          │
                └───────────────┬──────────────────────────┘
                                 ▼
Phase 2: Consumer integration (depends on Phase 1's code existing in the same PR —
built sequentially, shipped together as a single atomic change; see Risk Control)
┌───────────────────────────────────────────────────────────────────┐
│ Epic 2.1: OmnibarCreationPanel field swaps                        │
│  2.1.1a Parent Directory → RepoPathInput + New-Project hint       │
│  2.1.1b Existing Worktree Path fallback → RepoPathInput           │
└───────────────────────────────┬────────────────────────────────────┘
                                 ▼
Phase 3: Verification (depends on Phase 2 code existing)
┌───────────────────────────────────────────────────────────────────┐
│ Epic 3.1: e2e coverage                                            │
│  3.1.1a Create new e2e spec file + shared setup                   │
│  3.1.1b History suggestions appear — Parent Directory field       │
│  3.1.1c History suggestions appear — Existing Worktree fallback   │
│  3.1.1d Manual free-text entry not overridden (both fields)       │
│  3.1.1e Escape closes dropdown only vs. resets panel               │
│  3.1.1f 390×844 viewport — no h-overflow, no v-clip                │
│  3.1.1f-contingency Open-upward fallback IF 3.1.1f finds clipping  │
│  3.1.1g Existing session-create-new-project.spec.ts still passes   │
│  3.1.1h Existing session-create-existing-worktree.spec.ts passes   │
├─────────────────────────────────────────────────────────────────────┤
│ Epic 3.2: Feature registry                                        │
│  3.2.1a Create registry JSON                                      │
│  3.2.1b make registry-generate + confirm no coverage-gap growth   │
└───────────────────────────────────────────────────────────────────┘
```

---

## Phase 1: Shared component + selector fixes

### Epic 1.1: RepoPathInput bugfixes and a11y
**Goal**: Fix the Escape-propagation hazard and the missing combobox ARIA attributes
inside `RepoPathInput.tsx` itself, so both fixes automatically benefit every current and
future consumer (including the two new Omnibar fields added in Phase 2, and the
already-shipping `NewShellDialog` bug).

#### Story 1.1.1: Escape closes only this component's own dropdown when nested under a keydown-capturing ancestor
**As a** user of any `RepoPathInput` instance nested inside a modal/dialog with its own
Escape handling (e.g. the Omnibar, `NewShellDialog`), **I want** pressing Escape while the
suggestion dropdown is open to close only the dropdown, **so that** I don't accidentally
close the whole panel/dialog I was still using.
**Acceptance Criteria** (AC6):
- Escape with the dropdown visible stops native event propagation and closes only the dropdown.
  - *Given* a `RepoPathInput` instance with `showDropdown === true` (dropdown actually
    rendered — i.e. `open === true` AND (`allEntries.length > 0` OR `isLoading`)), *When*
    the user presses Escape while the input is focused, *Then* `e.nativeEvent.stopImmediatePropagation()`
    is called, `open` becomes `false`, `selectedIndex` becomes `-1`, and no ancestor
    `keydown`/`Escape` handler (e.g. a parent `onKeyDown` or a `document`-level listener)
    observes the event.
- Escape with the dropdown not visible is left alone (no regression) — this covers BOTH
  "never focused" and "focused but empty" cases.
  - *Given* a `RepoPathInput` instance with `showDropdown === false` (either `open === false`,
    or `open === true` but `allEntries.length === 0` and not loading — e.g. an empty field
    with no history and no filesystem matches), *When* the user presses Escape while the
    input is focused, *Then* `stopImmediatePropagation()` is NOT called and the event
    bubbles normally to any ancestor handler (e.g. `Omnibar.tsx`'s `handleKeyDown` still
    runs its own "reset to discovery" / "close" logic, or `NewShellDialog.tsx`'s
    Escape→`onCancel()` still fires). This is the case a gate on `open` alone would get
    wrong — see the `open`/`showDropdown` Domain Glossary rows above.
**Files**: `web-app/src/components/ui/RepoPathInput.tsx`, `web-app/src/components/ui/RepoPathInput.test.tsx`

Note: `usePathCompletions`'s RPC failures degrade silently to history-only suggestions
today (its `error` field is discarded by `RepoPathInput`) — this is pre-existing behavior
across all current consumers, unchanged and not fixed by this plan.

##### Task 1.1.1a: Add `stopImmediatePropagation()` guard to the Escape case (~3 min)
- In `RepoPathInput.tsx`'s `handleKeyDown` (`case "Escape":`, currently lines 134-137),
  change the body to gate on `showDropdown`, NOT `open` — `open` becomes `true` on mere
  focus even when the dropdown renders nothing (empty history + no filesystem matches),
  so gating on `open` alone would wrongly swallow Escape in that case and regress
  Omnibar's/`NewShellDialog`'s own Escape-to-close when no dropdown is actually visible:
  ```ts
  case "Escape":
    if (showDropdown) {
      e.nativeEvent.stopImmediatePropagation();
    }
    setOpen(false);
    setSelectedIndex(-1);
    break;
  ```
- `showDropdown` (`open && (allEntries.length > 0 || isLoading)`, already computed at line 99)
  depends on `isLoading`, which `open` alone does not — update the `handleKeyDown`
  `useCallback`'s dependency array from `[open, allEntries, selectedIndex, handleSelect]`
  (line 140) to include `isLoading` (or depend on `showDropdown` directly instead of its
  three constituent values, whichever reads more clearly at the call site).
- Files: `web-app/src/components/ui/RepoPathInput.tsx`

##### Task 1.1.1b: Unit test — Escape stops propagation when dropdown is open (isolated) (~5 min)
- **Prerequisite sub-step**: `RepoPathInput.test.tsx`'s current `useSessionRepoPaths` mock
  is a fixed `() => []` (always empty). Restructure it into a `jest.fn()` so individual
  tests can override its return value per-case via `mockReturnValue`/`mockReturnValueOnce`
  (e.g. `const mockUseSessionRepoPaths = useSessionRepoPaths as jest.Mock;` at module scope,
  then `mockUseSessionRepoPaths.mockReturnValue([...])` inside each test, resetting in
  `beforeEach`). Do this once, here, as the first task that needs a non-empty history mock
  — later test tasks (1.1.1c, 1.1.2b, and the new empty-history Escape case below) all
  assume this restructuring is already in place and just call `mockReturnValue(...)`.
- Add a new `describe("RepoPathInput — Escape key handling", ...)` block to
  `RepoPathInput.test.tsx`.
- Test: render `RepoPathInput` with the `useSessionRepoPaths` mock returning at least one
  path (e.g. `["/home/user/project-a"]`) so the dropdown has content to open; focus the
  input (`fireEvent.focus`), assert the listbox (`role="listbox"`) is present; press
  Escape (`fireEvent.keyDown(input, { key: "Escape" })`); assert the listbox is no longer
  in the document (`queryByRole("listbox")` is null).
- This test only proves the component's own open/close state changes correctly — it does
  NOT prove propagation is stopped (that requires Task 1.1.1c's nested-parent test, per
  `pitfalls.md`'s explicit call-out that a unit test alone cannot prove the bubble stops).
- Files: `web-app/src/components/ui/RepoPathInput.test.tsx`

##### Task 1.1.1c: Unit test — Escape does not bubble to a parent's own keydown handler when dropdown is open (~5 min)
- In the same `describe` block, add a test that wraps `RepoPathInput` in a parent `<div>`
  with its own `onKeyDown` spy (`jest.fn()`) attached, mirroring how `Omnibar.tsx`'s
  `modal` div attaches `onKeyDown={handleKeyDown}` to an ancestor of `RepoPathInput`.
  ```tsx
  const parentKeyDown = jest.fn();
  render(
    <div onKeyDown={parentKeyDown}>
      <RepoPathInput value="" onChange={jest.fn()} />
    </div>
  );
  ```
  (with `useSessionRepoPaths` mocked to return `["/home/user/project-a"]"` so the dropdown
  opens on focus.)
- Focus the input, confirm dropdown open, press Escape, assert `parentKeyDown` was NOT
  called (proves `stopImmediatePropagation` actually stopped the bubble the way
  `e.stopPropagation()` alone would not have for a native-listener ancestor — see
  `pitfalls.md` §3 for why `nativeEvent.stopImmediatePropagation()` specifically is
  required, not just `e.stopPropagation()`).
- Add a second case in the same test (or a sibling test) covering the "no regression"
  half of AC6: dropdown NOT open (e.g. `value=""` and no focus triggering `open`), press
  Escape, assert `parentKeyDown` WAS called — proving Escape still bubbles normally when
  there's nothing local to close.
- Add a THIRD case — the one the original plan never tested, called out in review as the
  actual gap `open`-gating would have missed: mock `useSessionRepoPaths` to return `[]`
  (empty — no history entries) AND ensure `usePathCompletions` also reports no filesystem
  matches (not loading), so `open` becomes `true` on focus but `showDropdown` stays
  `false` because `allEntries.length === 0` and `isLoading` is `false`. Render inside the
  same `<div onKeyDown={parentKeyDown}>` wrapper, focus the input (dropdown does not
  render), press Escape, and assert BOTH: `stopImmediatePropagation` was NOT triggered
  (the listbox was never present, so there's nothing to additionally assert there) AND
  `parentKeyDown` WAS called — proving the event bubbled normally to the parent's own
  `onKeyDown`, i.e. focusing an empty `RepoPathInput` does not silently break the parent's
  Escape-to-close.
- Files: `web-app/src/components/ui/RepoPathInput.test.tsx`

#### Story 1.1.2: RepoPathInput's input is announced as a combobox to assistive tech
**As a** screen-reader user interacting with any `RepoPathInput` field, **I want** the
input to be announced with combobox semantics, **so that** I understand it drives a
listbox rather than being a plain text field with orphaned ARIA relationship attributes.
**Acceptance Criteria** (supports AC5's broader UX-quality bar; not a lettered AC of its
own but explicitly recommended by `ux.md` and folded into this change per the Pattern
Decisions table above):
- The input carries the full combobox ARIA triad at all times, with `aria-expanded`
  reflecting live dropdown state.
  - *Given* a `RepoPathInput` instance rendered with `showDropdown` currently `false`,
    *When* the DOM is inspected, *Then* the `<input>` has `role="combobox"`,
    `aria-haspopup="listbox"`, and `aria-expanded="false"`.
  - *Given* the same instance after the user types a character that opens the dropdown
    (`showDropdown` becomes `true`), *When* the DOM is inspected again, *Then*
    `aria-expanded="true"` on the same `<input>`, with `aria-controls`/`aria-activedescendant`
    unchanged from their existing behavior.
**Files**: `web-app/src/components/ui/RepoPathInput.tsx`, `web-app/src/components/ui/RepoPathInput.test.tsx`

##### Task 1.1.2a: Add combobox ARIA triad to the input (~3 min)
- In `RepoPathInput.tsx`'s `<input>` JSX (lines 156-185), add three attributes alongside
  the existing `aria-autocomplete="list"` (line 173):
  ```tsx
  role="combobox"
  aria-haspopup="listbox"
  aria-expanded={showDropdown}
  ```
- `showDropdown` is already computed at line 99 (`open && (allEntries.length > 0 || isLoading)`)
  — no new state needed.
- Files: `web-app/src/components/ui/RepoPathInput.tsx`

##### Task 1.1.2b: Unit test — combobox ARIA attributes present and reflect dropdown state (~4 min)
- Add a `describe("RepoPathInput — combobox a11y", ...)` block to `RepoPathInput.test.tsx`.
- Test 1: render with `useSessionRepoPaths` mocked to `[]` and `usePathCompletions`
  mocked to empty/not-loading (dropdown closed); assert the input has
  `role="combobox"`, `aria-haspopup="listbox"`, `aria-expanded="false"`.
- Test 2: render with `useSessionRepoPaths` mocked to return `["/home/user/x"]`; focus
  the input; assert `aria-expanded="true"`.
- Files: `web-app/src/components/ui/RepoPathInput.test.tsx`

---

### Epic 1.2: Recency-ordered, deterministically-tiebroken session-path suggestions
**Goal**: Make `useSessionRepoPaths`' history list genuinely most-recent-first with a
fully defined order, instead of incidental entity-adapter insertion order.

#### Story 1.2.1: `selectActiveSessionsSortedByUpdatedAt` has a total, deterministic order
**As a** developer relying on `selectActiveSessionsSortedByUpdatedAt` for any
recency-ordered UI, **I want** ties in `updatedAt` to be broken deterministically, **so
that** the resulting order never depends on incidental array/adapter ordering.
**Acceptance Criteria** (AC3, first half):
- Two sessions with equal `updatedAt` are ordered by `createdAt` descending.
  - *Given* two sessions, `Session{id:"a", updatedAt:{seconds:100n}, createdAt:{seconds:50n}}`
    and `Session{id:"b", updatedAt:{seconds:100n}, createdAt:{seconds:60n}}`, both with
    `status: SessionStatus.ACTIVE`, *When* `selectActiveSessionsSortedByUpdatedAt` is
    evaluated against a store containing both, *Then* the result is `["b", "a"]` (session
    `b`'s later `createdAt` wins the tie).
- Two sessions with equal `updatedAt` AND equal/missing `createdAt` are ordered by `id` ascending.
  - *Given* two sessions, `Session{id:"z-session"}` and `Session{id:"a-session"}`, neither
    with `updatedAt` or `createdAt` set, both `status: SessionStatus.ACTIVE`, *When* the
    selector is evaluated, *Then* the result is `["a-session", "z-session"]`
    (`"a-session" < "z-session"` lexicographically).
**Files**: `web-app/src/lib/store/sessionsSlice.ts`, `web-app/src/lib/store/__tests__/sessionsSlice.test.ts`

##### Task 1.2.1a: Extend the comparator with the createdAt → id tiebreak (~4 min)
- In `sessionsSlice.ts`, replace the `selectActiveSessionsSortedByUpdatedAt` body
  (lines 117-123) with:
  ```ts
  export const selectActiveSessionsSortedByUpdatedAt = createSelector(
    selectAllSessions,
    (sessions) =>
      sessions
        .filter((s) => s.status !== SessionStatus.UNSPECIFIED)
        .sort((a, b) => {
          const byUpdated = Number(b.updatedAt?.seconds ?? 0) - Number(a.updatedAt?.seconds ?? 0);
          if (byUpdated !== 0) return byUpdated;
          const byCreated = Number(b.createdAt?.seconds ?? 0) - Number(a.createdAt?.seconds ?? 0);
          if (byCreated !== 0) return byCreated;
          return a.id < b.id ? -1 : a.id > b.id ? 1 : 0;
        })
  );
  ```
- No import changes needed — `createSelector` and `SessionStatus` are already imported
  at the top of the file; `Session.createdAt` is already on the generated proto type.
- Files: `web-app/src/lib/store/sessionsSlice.ts`

##### Task 1.2.1b: Unit tests for the tiebreak comparator (~5 min)
- Add a `describe("selectActiveSessionsSortedByUpdatedAt — tiebreak", ...)` block to
  `sessionsSlice.test.ts`, following the file's existing `makeStore()`/`makeSession()`
  helper pattern (note: `makeSession(id, title)` doesn't currently set `updatedAt`/
  `createdAt`/`status` — extend inline with `create(SessionSchema, {...})` calls, as the
  file already does elsewhere, e.g. lines 91/95/103/105).
- Test 1 (primary order still works): two sessions with distinct `updatedAt`, assert the
  later one sorts first.
- Test 2 (createdAt tiebreak): two sessions with equal `updatedAt.seconds`, distinct
  `createdAt.seconds`; assert the later `createdAt` sorts first. Matches the Given/When/Then
  in Story 1.2.1 above.
- Test 3 (id tiebreak, final level): two sessions with no `updatedAt`/`createdAt` set at
  all; assert ascending `id` order. Matches Story 1.2.1's second Given/When/Then.
- Test 4 (UNSPECIFIED filter unaffected by the tiebreak change — regression guard):
  session with `status: SessionStatus.UNSPECIFIED` is excluded regardless of how recent
  its `updatedAt`/`createdAt` are.
- Files: `web-app/src/lib/store/__tests__/sessionsSlice.test.ts`

#### Story 1.2.2: `useSessionRepoPaths` sources from the recency-ordered, deterministic selector
**As a** user of any `RepoPathInput` field, **I want** history suggestions ordered
most-recently-used first, **so that** the path I want is usually at the top.
**Acceptance Criteria** (AC3, second half):
- `useSessionRepoPaths` returns paths in the order produced by
  `selectActiveSessionsSortedByUpdatedAt`, deduplicated, dropping `UNSPECIFIED`-status
  sessions.
  - *Given* a Redux store with three sessions —
    `{id:"s1", path:"/repo/a", updatedAt:{seconds:300n}, status:ACTIVE}`,
    `{id:"s2", path:"/repo/b", updatedAt:{seconds:100n}, status:ACTIVE}`,
    `{id:"s3", path:"/repo/c", status:UNSPECIFIED}` — *When* `useSessionRepoPaths()` is
    called (via a test render hook), *Then* it returns `["/repo/a", "/repo/b"]` (most
    recent first, `s3` excluded).
**Files**: `web-app/src/lib/hooks/useSessionRepoPaths.ts`

##### Task 1.2.2a: Swap the selector import/call (~2 min)
- In `useSessionRepoPaths.ts`, change:
  ```ts
  import { selectAllSessions } from "@/lib/store/sessionsSlice";
  // ...
  const sessions = useAppSelector(selectAllSessions);
  ```
  to:
  ```ts
  import { selectActiveSessionsSortedByUpdatedAt } from "@/lib/store/sessionsSlice";
  // ...
  const sessions = useAppSelector(selectActiveSessionsSortedByUpdatedAt);
  ```
- The existing dedup loop (lines 10-19) is unchanged — it already preserves input order,
  so switching the input's order to recency-first automatically makes the deduped output
  recency-first too.
- Files: `web-app/src/lib/hooks/useSessionRepoPaths.ts`

---

## Phase 2: Consumer integration — OmnibarCreationPanel

### Epic 2.1: Replace the two plain-text repo-path inputs with RepoPathInput
**Goal**: Give the Parent Directory and Existing Worktree Path fallback fields the same
history-suggestion + filesystem-completion UX every other repo-path field in the app has.

#### Story 2.1.1: Parent Directory field uses RepoPathInput with New-Project-specific hint copy
**As a** user creating a New Project session, **I want** the Parent Directory field to
suggest paths from my session history, **so that** I don't have to retype a directory
I've used before, while understanding that suggested paths are existing project folders,
not literal parent directories.
**Acceptance Criteria** (AC1, AC2, AC4 for this field; hint copy per `ux.md`):
- The field renders as a `RepoPathInput` instance, preserving its existing `id`/label wiring.
  - *Given* the Omnibar is open in New Project mode, *When* the Parent Directory field is
    inspected, *Then* it is a `RepoPathInput` instance whose inner `<input id="omnibar-parent-dir">`
    is still reachable via `page.getByLabel('Parent Directory *')` (the `<label htmlFor="omnibar-parent-dir">`
    stays in `OmnibarCreationPanel.tsx`, unchanged).
- History suggestions appear and are recency-ordered (composes with Epic 1.2).
  - *Given* the user has a prior session with `path: "/home/user/Projects/foo"` and
    `updatedAt` more recent than any other session, *When* the user focuses the Parent
    Directory field, *Then* `/home/user/Projects/foo` appears in the dropdown's history
    section, above any older history entries.
- Manual free-text entry is not overridden.
  - *Given* the Parent Directory field is focused with the dropdown open showing history
    entries, *When* the user types `/tmp/brand-new-parent-dir` (a path matching no
    history/filesystem entry), *Then* the field's value is exactly what was typed, no
    dropdown selection is auto-applied, and `canSubmit`'s `parentDir.trim()` check
    (`Omnibar.tsx` `canSubmit`, `new_project` branch) still evaluates against the typed value.
- New-Project-specific hint replaces the generic one.
  - *Given* `sessionType === "new_project"`, *When* the Parent Directory field is
    rendered, *Then* its `hint` prop reads a New-Project-specific string (e.g. "Recent
    paths below are existing project folders — pick one to use its parent, or type a new
    directory.") rather than `RepoPathInput`'s default undefined/generic hint.
**Files**: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

##### Task 2.1.1a: Import RepoPathInput and replace the Parent Directory `<input>` (~5 min)
- Add `import { RepoPathInput } from "@/components/ui/RepoPathInput";` to
  `OmnibarCreationPanel.tsx`'s import block (near the other `@/components/ui/*` imports,
  e.g. alongside `RadioGroup`, `SlashCommandDropdown`).
- Replace the block at lines 488-501:
  ```tsx
  <input
    id="omnibar-parent-dir"
    type="text"
    className={fieldInput}
    placeholder="~/Projects"
    value={parentDir}
    onChange={(e) => setFormField("parentDir", e.target.value)}
  />
  <span className={hint}>Directory where the new project folder will be created</span>
  ```
  with:
  ```tsx
  <RepoPathInput
    id="omnibar-parent-dir"
    placeholder="~/Projects"
    value={parentDir}
    onChange={(v) => setFormField("parentDir", v)}
    required
    hint="Recent paths below are existing project folders — pick one to use its parent, or type a new directory."
  />
  ```
- Keep the surrounding `<label className={labelClass} htmlFor="omnibar-parent-dir">Parent Directory *</label>`
  wrapper unchanged (it's outside this replaced block, at lines 486-488).
- Note: `RepoPathInput` renders its own `hint`/error styling internally via
  `styles.hint`/`styles.githubHint` (its own `.css.ts`), so the standalone `<span className={hint}>`
  that previously sat below the raw `<input>` is fully replaced by the `hint` prop — do not
  render both.
- Files: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

#### Story 2.1.2: Existing Worktree Path fallback uses RepoPathInput
**As a** user creating an Existing Branch session where worktree auto-discovery found
nothing, **I want** the manual path-entry fallback to suggest paths from my session
history, **so that** I can quickly pick a worktree path I've used before instead of
retyping it.
**Acceptance Criteria** (AC1, AC2, AC4 for this field):
- The fallback field renders as `RepoPathInput`, preserving the shared `id` with its
  `<select>` sibling.
  - *Given* `sessionType === "existing_worktree"` and `worktrees.length === 0` and not
    loading, *When* the field is inspected, *Then* it is a `RepoPathInput` instance whose
    inner `<input id="omnibar-existing-worktree">` is reachable via
    `page.getByLabel('Existing Worktree Path *')`.
  - The shared `id="omnibar-existing-worktree"` across the `<select>`/`RepoPathInput`
    branches is safe because the three render branches
    (`isWorktreesLoading ? <select disabled> : worktrees.length > 0 ? <select> : <input>`
    — now `<RepoPathInput>`) are mutually exclusive arms of a single ternary, so exactly
    one element carrying that `id` is ever present in the DOM at a time; the id is never
    duplicated.
- History suggestions appear (same recency behavior as Story 2.1.1, composes with Epic 1.2).
  - *Given* a prior session with `path: "/home/user/.stapler-squad/worktrees/repo-x"`,
    *When* the user focuses this field, *Then* that path appears in the dropdown's
    history section.
- Manual free-text entry is not overridden (same guarantee as Story 2.1.1).
  - *Given* the field is focused with history suggestions showing, *When* the user types
    a path not present in history, *Then* the typed value is preserved verbatim in the field.
**Files**: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

##### Task 2.1.2a: Replace the Existing Worktree Path fallback `<input>` (~4 min)
- Replace the block at lines 651-660 (the `else` branch of the `worktrees.length > 0 ? ... : (...)` ternary):
  ```tsx
  <input
    id="omnibar-existing-worktree"
    type="text"
    className={fieldInput}
    placeholder="/path/to/existing/worktree"
    value={existingWorktree}
    onChange={(e) => setFormField("existingWorktree", e.target.value)}
  />
  ```
  with:
  ```tsx
  <RepoPathInput
    id="omnibar-existing-worktree"
    placeholder="/path/to/existing/worktree"
    value={existingWorktree}
    onChange={(v) => setFormField("existingWorktree", v)}
  />
  ```
- Do not touch the `<select id="omnibar-existing-worktree" ...>` branch immediately above
  it (worktrees-found case) — it's a distinct, out-of-scope conditional render sharing
  only the `id`.
- The existing `<span className={hint}>{...}</span>` below the ternary (lines ~662-670,
  showing "Scanning for git worktrees…" / worktreesError / etc.) stays exactly as-is — it
  is a sibling of the field, not nested inside it, and already handles this field's
  hint/error messaging at the panel level; do not also pass a `hint` prop into
  `RepoPathInput` here (would produce two overlapping hint strings).
- Files: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

---

## Phase 3: Verification

### Epic 3.1: e2e coverage
**Goal**: Cover AC2, AC4, AC5, AC6 end-to-end per `.claude/rules/e2e-test-conventions.md`
(feature-annotation header, no `waitForTimeout`, `data-testid`/ARIA locators only, page
helpers in `tests/e2e/pages/` if warranted), and confirm no regression to the existing
`session-create-new-project.spec.ts` suite.

#### Story 3.1.1: New e2e spec covers history suggestions, free-text entry, Escape scoping, and mobile viewport for both fields
**As a** developer verifying this change didn't regress the Omnibar or break mobile
usability, **I want** automated e2e coverage of the new `RepoPathInput` behavior in both
fields, **so that** future changes can't silently reintroduce the Escape-propagation bug
or break suggestion ordering.
**Acceptance Criteria** (AC2, AC4, AC5, AC6, plus AC1/AC7's "existing tests still pass"):
- History suggestions appear for both fields (AC2).
  - *Given* a session was previously created with a known path (seeded via the test's own
    prior `CreateSession` call or fixture), *When* the user opens the Omnibar, selects New
    Project mode, and focuses the Parent Directory field, *Then* a listbox
    (`role="listbox"`) becomes visible containing an option with that path's text.
- Manual free-text entry is not overridden (AC4).
  - *Given* the dropdown is open with suggestions, *When* the user types a path not in the
    suggestion list into either field, *Then* `expect(locator).toHaveValue(typedPath)`
    holds immediately after typing, with no dropdown-driven overwrite.
- Escape closes only the dropdown when open; closes/resets the panel as before when no
  dropdown is open (AC6).
  - *Given* the Parent Directory field is focused with its dropdown open, *When* Escape is
    pressed once, *Then* the listbox becomes hidden AND the Omnibar/creation panel remains
    open with New Project mode still selected (`page.getByRole('radio', { name: 'New Project' })`
    still `aria-checked="true"`).
  - *Given* the Parent Directory field is focused with its dropdown closed (e.g. after a
    prior Escape already closed it, or the field has never been focused), *When* Escape is
    pressed, *Then* the existing reset-to-discovery/close behavior fires exactly as before
    this change (assert via existing Omnibar close/reset locators already used elsewhere
    in the e2e suite).
- Dropdown visible/usable at 390×844 with no horizontal overflow and no vertical clipping (AC5).
  - *Given* the viewport is set to 390×844 (`page.setViewportSize`), *When* the Parent
    Directory field is focused and its dropdown opens, *Then* a `page.evaluate(() => {...})`
    computation using `getBoundingClientRect()` on both the dropdown element and the
    modal element confirms `dropdown.right <= 390` (no horizontal overflow) and
    `dropdown.bottom <= modal.bottom` (no vertical clipping by the modal's
    `overflow: hidden` container). This is the single committed verification method — see
    Task 3.1.1f; `boundingBox()` is explicitly NOT used here because it reports the
    element's own box regardless of visual clipping by an `overflow: hidden` ancestor,
    which is exactly the failure mode this AC exists to catch.
  - *Given* the check above finds clipping, *Then* Task 3.1.1f-contingency's open-upward
    fallback is implemented and this assertion is re-run to confirm it now passes (see
    Task 3.1.1f-contingency below for the fallback approach).
- Existing `session-create-new-project.spec.ts` locators still pass (AC1/AC7 regression guard).
  - *Given* this change is applied, *When* `tests/e2e/session-create-new-project.spec.ts`
    is run unmodified, *Then* all 7 existing `T-E2E-NP-*` tests pass, in particular
    `T-E2E-NP-003`'s `page.getByLabel('Parent Directory *').fill('~/Projects')` call.
- Existing `session-create-existing-worktree.spec.ts` locators still pass (AC1/AC7 regression
  guard, Story 2.1.2's field specifically).
  - *Given* this change is applied, *When* `tests/e2e/session-create-existing-worktree.spec.ts`
    is run unmodified, *Then* all its existing tests pass, in particular the
    `page.getByLabel('Existing Worktree Path').fill(...)` case and the `canSubmit` gating
    test for an empty path.
**Files**: `tests/e2e/repo-path-picker-parity.spec.ts` (new), `tests/e2e/session-create-new-project.spec.ts` (unmodified, verify only), `tests/e2e/session-create-existing-worktree.spec.ts` (unmodified, verify only), `web-app/src/components/ui/RepoPathInput.tsx` and/or `web-app/src/components/ui/RepoPathInput.css.ts` (only touched if Task 3.1.1f-contingency's fallback is triggered)

##### Task 3.1.1a: Create the new e2e spec file with feature annotation and shared setup (~5 min)
- Create `tests/e2e/repo-path-picker-parity.spec.ts` starting with:
  ```ts
  // @feature session:create, repo-path-picker-parity
  import { test, expect } from '@playwright/test';
  ```
- Reuse the `openInCreationMode` helper pattern from `session-create-new-project.spec.ts`
  (either import it if exported, or duplicate the small helper locally — check whether
  `session-create-new-project.spec.ts` exports it before duplicating) to open the Omnibar
  and select New Project mode for the Parent Directory tests, and Existing Branch mode for
  the Existing Worktree Path tests.
- Files: `tests/e2e/repo-path-picker-parity.spec.ts`

##### Task 3.1.1b: Test — history suggestions appear for Parent Directory field (~5 min)
- Seed a session with a known path first (via UI: create a directory-mode session against
  a temp dir, matching the pattern `local-file-browser.spec.ts` uses with
  `fs.mkdtempSync`), then open the Omnibar in New Project mode, focus Parent Directory,
  assert the listbox contains an option with that path.
- Locator convention: `page.getByRole('listbox')`, `page.getByRole('option', { name: /.../ })`
  — no CSS class selectors, per `.claude/rules/e2e-test-conventions.md`.
- Files: `tests/e2e/repo-path-picker-parity.spec.ts`

##### Task 3.1.1c: Test — history suggestions appear for Existing Worktree Path fallback field (~5 min)
- Same shape as 3.1.1b but for Existing Branch mode's fallback field (only reachable when
  worktree discovery returns zero results — use a directory with no `.git` worktrees, or
  mock/force the empty-discovery path per however the existing test suite triggers it;
  check `session-create-existing-branch` or similar existing spec for the discovery-empty
  trigger pattern before inventing a new one).
- Files: `tests/e2e/repo-path-picker-parity.spec.ts`

##### Task 3.1.1d: Test — manual free-text entry is preserved verbatim, both fields (~4 min)
- For each field: fill with a path guaranteed absent from history/filesystem (e.g.
  `/tmp/brand-new-e2e-${Date.now()}`), assert `toHaveValue` immediately, per
  `.claude/rules/e2e-test-conventions.md`'s "no waitForTimeout" rule (`toHaveValue` is a
  polling assertion, not a fixed sleep).
- Files: `tests/e2e/repo-path-picker-parity.spec.ts`

##### Task 3.1.1e: Test — Escape closes dropdown only, then Escape again resets/closes as before (~5 min)
- Focus Parent Directory with dropdown open, press Escape, assert listbox hidden AND New
  Project radio still `aria-checked="true"` (panel didn't reset).
- Press Escape again (dropdown now closed), assert the pre-existing reset-to-discovery
  behavior fires (radiogroup for session type disappears / omnibar returns to discovery
  input — match whatever assertion `session-create-new-project.spec.ts` or `Omnibar`'s
  own existing spec already uses for this transition, for locator consistency).
- Files: `tests/e2e/repo-path-picker-parity.spec.ts`

##### Task 3.1.1f: Test — 390×844 viewport, no horizontal overflow, no vertical clip (~5 min)
- `await page.setViewportSize({ width: 390, height: 844 });` before opening the Omnibar.
- Focus Parent Directory (deepest-in-form field per `pitfalls.md`'s flagged worst case),
  open dropdown. Add `data-testid="path-completion-dropdown"` to
  `PathCompletionDropdown.tsx`'s wrapper for a stable locator if one doesn't already
  exist (check first before adding), and ensure the Omnibar's modal wrapper also has a
  stable locator (existing `data-testid` or role, e.g. `page.getByRole('dialog')`).
- **Committed verification method** (do not hedge between multiple approaches — this is
  the one and only method used): use `page.evaluate(() => {...})` to call
  `getBoundingClientRect()` on both the dropdown element and the modal element inside the
  browser context, and return the computed rects. This is more CI-stable than Playwright's
  `boundingBox()` for elements inside an `overflow: hidden` ancestor, since `boundingBox()`
  reports the element's own box regardless of visual clipping — it would report a
  "successful" box even when the modal's `overflow: hidden` is actually clipping the
  dropdown, which is exactly the failure mode `research/pitfalls.md` flags as real and
  non-hypothetical.
  ```ts
  const rects = await page.evaluate(() => {
    const dropdown = document.querySelector('[data-testid="path-completion-dropdown"]');
    const modal = document.querySelector('[role="dialog"]'); // or the modal's actual selector
    return {
      dropdown: dropdown?.getBoundingClientRect().toJSON(),
      modal: modal?.getBoundingClientRect().toJSON(),
    };
  });
  expect(rects.dropdown.right).toBeLessThanOrEqual(390);
  expect(rects.dropdown.bottom).toBeLessThanOrEqual(rects.modal.bottom);
  ```
- If both assertions pass, this task is done. If either fails (clipping detected), proceed
  to Task 3.1.1f-contingency below — this is a required next step during Phase 3, not an
  open-ended "investigate later."
- Files: `tests/e2e/repo-path-picker-parity.spec.ts`

##### Task 3.1.1f-contingency: Open-upward fallback IF Task 3.1.1f detects clipping (conditional; ~15 min if triggered)
- **Trigger condition**: only implement this task if Task 3.1.1f's `getBoundingClientRect()`
  assertions fail (i.e. the dropdown is actually clipped by the Omnibar modal's
  `overflow: hidden` at 390×844). If Task 3.1.1f passes cleanly, skip this task entirely.
- **Approach**: in `web-app/src/components/ui/RepoPathInput.tsx`, compute the available
  vertical space below the input by comparing the input's own `getBoundingClientRect()`
  against the modal's (or viewport's, if no modal ancestor) visible bounds — e.g. on open
  (or via a `useLayoutEffect` keyed on `open`), check whether
  `inputRect.bottom + dropdownMaxHeight > containerRect.bottom` (using the existing
  `maxHeight: 200` constant already referenced in `research/pitfalls.md`). If there isn't
  enough room below, apply a CSS modifier (e.g. a `data-placement="above"` attribute on the
  dropdown wrapper, matching this repo's `data-*` + `selectors` convention from
  `.claude/rules/css-architecture.md` rather than an inline `style={{ ... }}` override) so
  `RepoPathInput.css.ts` can flip the dropdown's positioning to `top: auto; bottom: calc(100% + 2px)`
  (open upward) instead of the default `top: calc(100% + 2px)` (open downward).
- Re-run Task 3.1.1f's assertions after implementing the fallback to confirm the dropdown
  is no longer clipped at 390×844.
- Files: `web-app/src/components/ui/RepoPathInput.tsx`, `web-app/src/components/ui/RepoPathInput.css.ts`, `tests/e2e/repo-path-picker-parity.spec.ts`

##### Task 3.1.1g: Verify existing session-create-new-project.spec.ts still passes (~3 min)
- Run `cd tests/e2e && npx playwright test session-create-new-project.spec.ts` after Phase 2
  lands; confirm all 7 `T-E2E-NP-*` tests pass unmodified.
- No file changes expected — this is a verification step, not a code task. If it fails,
  the failure indicates a real regression in Task 2.1.1a's replacement and must be fixed
  there, not by editing this pre-existing spec to work around it.
- Files: `tests/e2e/session-create-new-project.spec.ts` (read-only verification)

##### Task 3.1.1h: Verify existing session-create-existing-worktree.spec.ts still passes (~3 min)
- Required, blocking verification step after Task 2.1.2a lands (Story 2.1.2 replaces the
  Existing Worktree Path fallback `<input>` with `RepoPathInput`, and this spec directly
  exercises that field). Run
  `cd tests/e2e && npx playwright test session-create-existing-worktree.spec.ts`; confirm
  all existing tests pass unmodified, in particular the
  `page.getByLabel('Existing Worktree Path').fill(...)` case and the `canSubmit` gating
  test for an empty path — this is the only coverage AC6's "no regression to canSubmit
  gating" claim has for this specific field.
- No file changes expected — this is a verification step, not a code task. If it fails,
  the failure indicates a real regression in Task 2.1.2a's replacement and must be fixed
  there, not by editing this pre-existing spec to work around it.
- Files: `tests/e2e/session-create-existing-worktree.spec.ts` (read-only verification)

---

### Epic 3.2: Feature registry
**Goal**: Register the changed/new-behavior frontend feature per
`.claude/rules/feature-registry.md`, and confirm no net growth in untested-feature count.

#### Story 3.2.1: Feature registry reflects the repo-path-picker-parity change
**As a** maintainer scanning `docs/registry/features/frontend/`, **I want** an entry
documenting this feature and its test coverage, **so that** `make registry-generate`
correctly reports it as tested.
**Acceptance Criteria** (AC7):
- A new per-feature JSON file exists under `docs/registry/features/frontend/`.
  - *Given* the registry directory structure (`docs/registry/features/frontend/ui/` holds
    component-level entries like `local-file-browser.json`), *When*
    `docs/registry/features/frontend/ui/repo-path-picker-parity.json` is read, *Then* it
    has `"tested": true` and a non-empty `"testIds"` array referencing the new e2e spec's
    test names and/or the `RepoPathInput`/`sessionsSlice` unit test describe-block names.
- `make registry-generate` produces no net growth in `docs/registry/coverage-gaps.json`.
  - *Given* the registry file from the previous criterion exists, *When*
    `make registry-generate` is run, *Then* the resulting `coverage-gaps.json`'s
    untested-feature count is `<=` its count before this change (ideally lower, since this
    change also indirectly documents `RepoPathInput`/`useSessionRepoPaths`/
    `selectActiveSessionsSortedByUpdatedAt` reuse for the first time if no entry covered
    them before).
**Files**: `docs/registry/features/frontend/ui/repo-path-picker-parity.json` (new)

##### Task 3.2.1a: Create the feature registry entry (~4 min)
- Create `docs/registry/features/frontend/ui/repo-path-picker-parity.json`, following the
  shape of `docs/registry/features/frontend/ui/local-file-browser.json`:
  ```json
  {
    "id": "repo-path-picker-parity",
    "type": "frontend",
    "component": "OmnibarCreationPanel",
    "path": "web-app/src/components/sessions/OmnibarCreationPanel.tsx",
    "tested": true,
    "testIds": [
      "repo-path-picker-parity.spec.ts",
      "RepoPathInput — Escape key handling",
      "RepoPathInput — combobox a11y",
      "selectActiveSessionsSortedByUpdatedAt — tiebreak"
    ],
    "lastModified": "2026-08-01T00:00:00Z"
  }
  ```
- Files: `docs/registry/features/frontend/ui/repo-path-picker-parity.json`

##### Task 3.2.1b: Run make registry-generate and confirm no coverage-gap growth (~3 min)
- Run `make registry-generate` from the repo root.
- Diff `docs/registry/coverage-gaps.json` before/after (or inspect the generated count
  directly) — confirm the untested-feature count did not increase. Commit the regenerated
  aggregate files alongside the per-feature source file.
- Files: `docs/registry/coverage-gaps.json`, `docs/registry/frontend-features.json` (both generated, verification only)
