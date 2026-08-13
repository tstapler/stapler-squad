# Research: Common Pitfalls for RepoPathInput Parity Change

Investigated six risk areas before implementation. Findings below, each tagged with
severity and the file/line evidence backing it.

## 1. e2e `.fill()` vs. RepoPathInput's dropdown-on-focus behavior

**Risk: LOW, but verify empirically.**

`RepoPathInput.tsx:166` sets `onFocus={() => setOpen(true)}` and the `onChange` handler
(`:161-165`) also calls `setOpen(true)` on every keystroke. Playwright's `.fill()` clicks
the element (triggering focus → dropdown opens), clears it, then dispatches a single
`input` event with the full value (not per-keystroke), so `onChange` fires once. The
dropdown will be open and populated (from `usePathCompletions` + `useSessionRepoPaths`)
at the moment `.fill()` finishes, but `.fill()` itself doesn't click any dropdown item —
it only sets the input's DOM value — so it should not "select" an unintended entry.

Two second-order effects to watch for:
- `PathCompletionDropdown` items use `onMouseDown` with `e.preventDefault()`
  (`PathCompletionDropdown.tsx:53-56`) specifically to survive a blur — this means a
  stray real click (not `.fill()`) on the dropdown area during a test could select an
  entry unexpectedly. Not a `.fill()` risk, but a risk for any e2e test that clicks near
  the field afterward (e.g. clicking "Create" immediately after fill, if the dropdown
  visually overlaps the submit button — see item 5).
- `usePathCompletions` fires a real RPC (debounced 150ms) after `.fill()`. If a test
  asserts on `canSubmit`/button-enabled state immediately after `.fill()`, there is no
  intrinsic dependency on that RPC resolving (canSubmit only checks `parentDir.trim()`
  etc., not completion state) — so no new flakiness expected there. Confirmed via
  `Omnibar.tsx:916-935` `canSubmit` logic for `new_project` mode: it only checks
  `parentDir`/`projectName` string values, not dropdown/completion state.

Existing test file `tests/e2e/session-create-new-project.spec.ts` uses
`page.getByLabel('Parent Directory *').fill(...)` at lines 40, 55, 91. Since the
`<label htmlFor="omnibar-parent-dir">` stays in `OmnibarCreationPanel.tsx` and the
`id` prop is passed straight through to `RepoPathInput`'s inner `<input id={id} .../>`
(`RepoPathInput.tsx:157`), `getByLabel` resolution is unaffected. **Recommendation:**
after the swap, add an explicit assertion in the e2e suite that the dropdown does not
obscure/intercept a subsequent click on "Project Name" or the submit button (see item 5).

## 2. RepoPathInput.test.tsx / PathCompletionDropdown — no keyboard/dropdown test coverage exists

**Risk: MEDIUM — this is a coverage gap, not a bug, but it means R6 has no regression net today.**

`RepoPathInput.test.tsx` (43 lines) mocks both `useSessionRepoPaths` and
`usePathCompletions` to return empty arrays and only tests the hint/GitHub-detection
rendering path (5 `it()` blocks, all about `hint`/`detectGitHubUrl`). **There is no
existing test exercising `open`, `handleKeyDown`, Escape, arrow-key navigation, or
`handleSelect`.** No TODO/FIXME comments found in `RepoPathInput.test.tsx`,
`PathCompletionDropdown.test.tsx`, or `usePathCompletions.test.ts` (grepped, zero hits).
This means the Escape-propagation fix (R6) must ship its own new tests from scratch —
there's nothing to extend. Write both a unit test (RepoPathInput's own Escape handler
calls `stopPropagation`/`nativeEvent.stopImmediatePropagation` when its dropdown is
open) and a component-level test that nests `RepoPathInput` inside a parent with its own
`onKeyDown` to prove the bubble is actually stopped (a unit test on `RepoPathInput`
alone can assert the method was called, but only a nested-parent test proves the fix
works end-to-end).

## 3. Existing pitfall docs on keydown propagation / controlled-input footguns

**Confirmed: `.claude/rules/css-architecture.md` documents the exact escape-hatch pattern
already in use, and the precedent for the R6 fix lives directly in `Omnibar.tsx`.**

No rule doc specifically calls out keydown propagation, but the pattern is
self-documented in code:
- `Omnibar.tsx:891-901` — a `document`-level native `keydown` listener that calls
  `onClose()` on any bubbled Escape while `isOpen`.
- `Omnibar.tsx:1207-1210` — the `modal` div's own React `onKeyDown={handleKeyDown}`,
  which (at `:837-848`) treats an unstopped Escape as "reset to discovery" or "close".
- Three existing internal-dropdown-dismiss call sites *inside* `Omnibar.tsx` itself
  already call `e.nativeEvent.stopImmediatePropagation()` before doing local
  dropdown-dismiss state updates, specifically to defeat both of the above escalation
  points in one call: `:748` (`@`-suggestion dropdown), `:772` (discovery
  result-highlight), `:820` (path-completion dropdown, with an explicit comment: "Stop
  the native event so the global document listener doesn't also call onClose() —
  first Escape dismisses the dropdown only"), and `:839` (the top-level Escape handler
  stopping the *document* listener from double-firing after it handles Escape itself).

`RepoPathInput.tsx:134-137`'s Escape case only calls `setOpen(false)` /
`setSelectedIndex(-1)` — it does not call `stopPropagation()` on the synthetic event or
`stopImmediatePropagation()` on the native event. **The fix must mirror the existing
in-repo pattern exactly** (`e.nativeEvent.stopImmediatePropagation()`, not just
`e.stopPropagation()`) — `stopPropagation()` alone stops the React synthetic bubble to
`Omnibar.tsx`'s `handleKeyDown`, but since `Omnibar.tsx`'s document-level listener is a
*native* `addEventListener` at the document, only `stopImmediatePropagation()` on the
native event reliably prevents both the synthetic-bubble path *and* the raw native
document listener from also firing on the same keypress. Note the fix should only stop
propagation when `RepoPathInput` actually has its own dropdown open (`open === true`);
when closed, Escape should be left alone so Omnibar's own reset/close behavior is
unaffected (this is exactly what R6 calls out under "no regression... when no
RepoPathInput dropdown is open").

No CSS-architecture or Redux-selector-memoization pitfall docs exist beyond what's
already noted for item 6 below (`createSelector` usage is already correct/memoized in
`sessionsSlice.ts:117-123`).

## 4. usePathCompletions and non-existent target paths (New Project "Parent Directory")

**Risk: LOW — no error/blocking UX for missing paths; but no explicit design for the
"container may not exist" case addressed either. Behavior degrades gracefully.**

`usePathCompletions` (`usePathCompletions.ts`) has no assumption that the *exact*
`pathPrefix` exists — it calls `listPathCompletions({ pathPrefix, directoriesOnly,
maxResults })` and surfaces `baseDirExists`/`pathExists` flags in its return value, but
**`RepoPathInput` never reads `baseDirExists` or `pathExists`** — it only destructures
`entries` and `isLoading` (`RepoPathInput.tsx:60`). Consequently:
- If the parent directory prefix itself exists but the leaf doesn't (the "New Project"
  case — you're typing the container path, e.g. `~/Projects`, which does exist, and the
  *new subfolder* is what doesn't exist yet), `usePathCompletions` will resolve normally
  against the existing parent and return real subdirectory entries — no problem, this is
  the common case since Parent Directory is meant to be an existing folder.
- If the user types a path prefix that does not resolve to any existing directory at
  all (e.g. mistyped), `PathCompletionDropdown` simply renders nothing for the live
  filesystem section (`PathCompletionDropdown.tsx:82`: `if (entries.length === 0) return
  null`) — no "not found" error is shown, and critically **no blocking/validation
  behavior is triggered** since `RepoPathInput` doesn't wire `error`/`pathDoesNotExist`
  itself (that's left to the caller via the `error` prop, which `OmnibarCreationPanel`
  does not currently pass for these two fields). So typing a not-yet-created path is
  silently fine — dropdown just goes empty, free text stays in the field. This matches
  R4 (manual free-text entry must keep working) with no code change needed for that part.
- Contrast with `Omnibar.tsx:912-913`'s separate `pathDoesNotExist` signal (used by the
  *other*, older completion system for the main omnibar input) — that mechanism is
  explicitly out of scope per the requirements doc and is not shared with
  `RepoPathInput`.
- **Recommendation:** no code fix required here, but worth an explicit e2e assertion
  (R7) that typing a not-yet-existing "New Project" parent path does not show a false
  "not found" state and does not block `canSubmit`.

## 5. Mobile/viewport: dropdown overflow risk inside the Omnibar modal

**Risk: MEDIUM — flag for manual/e2e verification at 390×844, two distinct concerns.**

- **Horizontal:** `RepoPathInput.css.ts` container is `width: "100%"` and
  `dropdownWrapper` is `position: absolute; left: 0; right: 0` (`:58-69`) — it's sized
  to its own parent's width, not viewport width, so horizontal overflow is unlikely
  *unless* the parent `field` div in `OmnibarCreationPanel.tsx` itself overflows the
  600px-`maxWidth` modal at 390px viewport. Since `Omnibar.css.ts:42-49`'s `.modal` is
  `width: "100%"; maxWidth: 600` (no fixed px width), it should already shrink to fit a
  390px viewport, and the dropdown inherits that width — low horizontal-overflow risk,
  but still worth a screenshot check since no existing test currently renders
  `RepoPathInput` inside `OmnibarCreationPanel` at a mobile viewport.
- **Vertical clipping — the sharper risk:** `Omnibar.css.ts:42-49`'s `.modal` sets
  `overflow: "hidden"` on the *entire modal*, and `RepoPathInput`'s dropdown is
  `position: absolute` relative to its own field container (`RepoPathInput.css.ts:5-8`,
  `:58-63`), opening **downward** (`top: calc(100% + 2px)`). If the Parent
  Directory/Existing Worktree field sits low enough in the panel (true for "New
  Project" mode, which has Parent Directory → Project Name → path preview → radio group
  → conditional branch field all stacked after it, and the `Omnibar.css.ts` modal has
  `paddingTop: "10vh"` eating into available vertical space, especially at 844px mobile
  height), the up-to-200px-tall dropdown (`PathCompletionDropdown.css.ts:8`: `maxHeight:
  200`) risks being clipped by the modal's own `overflow: hidden` boundary before the
  user can see/tap the lower entries. This is the same failure class flagged generically
  in `.claude/rules/css-architecture.md` ("`position: fixed`/`absolute` ... silently
  breaks when any ancestor has ... `overflow`... Always use `createPortal`") — though
  that rule's remedy (portal to `document.body`) is a bigger structural change than this
  ticket's scope implies; at minimum this needs an explicit **manual/e2e visual check**
  at 390×844 with the Parent Directory field focused and the dropdown open, not an
  assumption that "it'll just work" because it already works in `BacklogItemForm`/
  `NewShellDialog`/`WorkflowForm` (those consumers' modals may have different overflow
  rules — not verified as part of this pass; worth a quick sanity check if time
  allows, since if any of them already have this problem, it isn't new here and can be
  deprioritized).
- **Touch-target sizing:** `PathCompletionDropdown.css.ts:14-19`'s `.item` style is
  `padding: "7px 16px"` with `fontSize: 13` — the resulting row height is roughly
  28-32px, under the ~44px minimum recommended touch-target size. This is pre-existing
  in `RepoPathInput`'s shared implementation (not introduced by this change), so it's
  arguably out of scope to fix here, but AC5 explicitly calls out "tappable row
  targets" for the *new* consumers — recommend at least confirming via manual test on a
  real/emulated touch viewport that mis-taps aren't a practical problem, and filing a
  follow-up if it is, rather than scope-creeping a touch-target redesign into this
  ticket.

## 6. `useSessionRepoPaths` selector swap — regression risk for existing consumers

**Risk: LOW — the filtering behavior change is consistent with an established
in-repo precedent, not a novel risk, but it IS a real behavior change worth calling out
explicitly in the PR description.**

Current: `useSessionRepoPaths.ts:9` sources `selectAllSessions` (raw entity-adapter
order, **no status filter**). Target (per R3): `selectActiveSessionsSortedByUpdatedAt`
(`sessionsSlice.ts:117-123`), which does two things at once:
1. Re-orders by `updatedAt.seconds` descending (the actual point of R3).
2. **Filters out** any session with `status === SessionStatus.UNSPECIFIED`
   (`sessionsSlice.ts:121`) — this filter does not exist in `selectAllSessions`/
   `useSessionRepoPaths` today.

This filter is not a novel invention for this change — `SESSION_STATUS_UNSPECIFIED`'s
proto doc comment (`session_pb.ts:1376`) states "means no live session matches this
history entry," and `useSessionSearch.ts:46` already applies the *identical* filter
(`sessions.filter((s) => s.status !== SessionStatus.UNSPECIFIED)`) for a comparable
purpose (excluding non-live entries from search results). So switching
`useSessionRepoPaths` to the same filter is consistent with existing conventions, not a
new invention.

Practical effect on the four current `RepoPathInput` consumers found via
`grep -rln "RepoPathInput" web-app/src`: `BacklogItemForm.tsx`, `LocalFileBrowser.tsx`,
`NewShellDialog.tsx`, `WorkflowForm.tsx` — all four consume `useSessionRepoPaths()`
only indirectly through `RepoPathInput`'s internal history-suggestion list, never
call the hook directly, and none inspect ordering/filtering themselves. So the R3
change is transparent at their call sites: they'll simply see a cleaner
(no-`UNSPECIFIED`), more accurately-recency-ordered suggestion list — strictly an
improvement, not a functional break. No consumer-side code changes required for R3
beyond the hook/selector edit itself.

**One additional finding not explicitly called out in requirements.md:** the current
`selectActiveSessionsSortedByUpdatedAt` sort (`sessionsSlice.ts:122`) mutates via
`Array.prototype.sort` on the array returned by `selectAllSessions` (a memoized
`createSelector` input). `Array.prototype.sort` sorts **in place**. Since
`selectAllSessions` is `adapterSelectors.selectAll`, which itself returns a fresh array
each time the adapter's normalized state changes (entity adapters recompute `selectAll`
via their own memoized selector keyed on `state.sessions.ids`/`entities`), sorting it
in place is safe *in this specific case* — it's not sorting a cached array reference
shared elsewhere. Confirmed by reading `createSelector(selectAllSessions, (sessions) =>
sessions.filter(...).sort(...))`: `.filter()` already returns a new array before
`.sort()` mutates it, so no shared-reference mutation bug exists. Flagging only because
"sort mutates its input" is a common footgun class — this instance is already safe by
construction (filter-then-sort), so **no fix needed**, just confirmed as safe during
this research pass.

## Summary of action items for implementation

| # | Area | Action required |
|---|---|---|
| 1 | e2e `.fill()` timing | No code fix; add e2e assertion dropdown doesn't intercept subsequent clicks |
| 2 | Test coverage gap | Write new Escape/keyboard tests from scratch — none exist today |
| 3 | Escape propagation | Use `e.nativeEvent.stopImmediatePropagation()` (not just `stopPropagation()`), gated on `open === true`, mirroring `Omnibar.tsx:820`'s existing pattern exactly |
| 4 | Non-existent path UX | No code fix; RepoPathInput already degrades gracefully (empty dropdown, no blocking). Add e2e assertion for New Project mode |
| 5 | Mobile viewport | Verify dropdown isn't clipped by `Omnibar.css.ts` `.modal`'s `overflow: hidden` at 390×844, especially for Parent Directory field (deep in New Project form); pre-existing touch-target sizing (~30px rows) is shared/out-of-scope but worth a manual check |
| 6 | Selector swap regression | No code fix beyond the selector swap itself; behavior change (UNSPECIFIED-status sessions dropped from history) is consistent with existing `useSessionSearch.ts` precedent — call out explicitly in PR description, not a silent behavior change |
