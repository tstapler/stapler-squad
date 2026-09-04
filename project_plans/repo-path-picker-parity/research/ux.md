# UX Research: Repo Path Picker Parity

## 1. Comparable UX patterns for recent-path/history autocomplete

**This codebase's own pattern (`RepoPathInput` + `PathCompletionDropdown`)** already implements
the core moves that make history-autocomplete combos work well:

- **History-first, filesystem-second ordering with a visual divider.** `RepoPathInput.tsx:70-97`
  builds `allEntries` as `[...historyEntries, ...fsCompletionEntries]`, deduped so a path that's
  both a recent session root and a live filesystem match only appears once (as history).
  `PathCompletionDropdown.tsx:84-115` renders a `divider` between the two groups when both are
  present. This mirrors VS Code's Quick Open (recently-opened files pinned above the fuzzy-match
  results) and browser address-bar autocomplete (frecency-ranked history above raw suggestions):
  put the "you've been here before" signal first because it's near-zero-cost to the user (one
  click vs. typing a full path), and visually separate it so users learn to expect the ordering
  and don't misread a history hit as a live directory listing.
- **Distinct affordance for history rows.** A clock icon (`🕒`) plus muted text color
  (`itemHistory` style) versus a folder icon for live filesystem entries. This is the same signal
  git's `checkout <TAB>` / shell history-substring-search use: a different glyph or dimming to
  say "this came from memory, not from disk right now" — important here specifically because a
  history path could have been deleted/renamed since the session that used it existed.
- **Type-to-filter, not type-to-replace.** The input stays a plain controlled `<input>`; the
  dropdown filters both history and fs-completions against the current `value` on every
  keystroke (`historyPaths.filter(p => value === "" || p.toLowerCase().includes(...))`). This is
  the "filter list narrows as you type, but your typed text is authoritative" model used by VS
  Code's command palette and browser omniboxes — the crucial property is that the dropdown never
  fights the text field for control.
- **Keyboard model matches ARIA combobox conventions**: ArrowDown/ArrowUp move a `selectedIndex`
  cursor, Enter commits the highlighted entry, Escape closes the list without altering the typed
  value, mouse click (`onMouseDown` + `preventDefault` to dodge the input's blur-before-click
  race) also commits. This matches the git-checkout-style and address-bar keyboard flows users
  already have muscle memory for.

**Nothing new needs to be built** — the requirements doc's own finding (R1) is correct: reuse
`RepoPathInput` verbatim rather than inventing a second combobox implementation for the omnibar.

## 2. Mental model: "parent directory" vs. history of leaf/repo paths

The New Project mode's Parent Directory field asks for a directory that will **contain** the new
project (e.g. `~/Projects`), but `useSessionRepoPaths()` supplies **leaf** paths — the actual
repo/worktree roots of past sessions (e.g. `~/Projects/stapler-squad`,
`~/Projects/stapler-squad/.worktrees/feature-x`). Surfacing those unmodified in a field labeled
"Parent Directory" risks two concrete confusions:

- A user picks `~/Projects/stapler-squad` expecting it to become the *parent* of their new
  project, and instead their new project gets created *inside* an existing repo
  (`~/Projects/stapler-squad/my-new-thing`), which is surprising and can pollute an existing
  git working tree.
- A user who scans the list for "the directory I usually put things in" doesn't see it, because
  every entry is a specific repo, not a generic staging folder.

**Recommendation:** don't try to derive true "parent of X" paths (there's no reliable way to know
which ancestor directory a user considers their "projects folder" — could be one level up, could
be several). Instead, reduce ambiguity with copy, not data transformation:

- Keep the hint text on this field distinct from the generic `RepoPathInput` copy used elsewhere.
  Something like: *"Pick a folder your new project will live inside — e.g. select an existing
  project below and use its parent, or type a path."* is too clever; simplest effective fix is to
  relabel what the suggestions **are** rather than pretend they're literal parent dirs. Given the
  existing UI budget (a `hint` span under the field, `OmnibarCreationPanel.tsx:500`), the pragmatic
  move is a small addition to that hint that appears specifically in New Project mode, e.g.
  *"Recent paths below are existing project folders — pick one to use its parent, or type a new
  directory."* This sets an explicit expectation that selecting an entry does not mean "create
  inside this repo" is the only outcome, and nudges the mental model toward "these are landmarks
  near where I usually work," which is the actual job the history serves (see §5).
- Do **not** silently truncate a selected history path to its dirname (e.g. auto-strip the last
  path segment when the mode is `new_project`). That would violate R4 in spirit (surprising
  mutation of what the user picked) and break the "what I see is what gets submitted" contract
  the other four `RepoPathInput` consumers already establish. Making the copy honest is safer
  than making the behavior "smart."
- The **Existing Worktree Path fallback** field has no such mismatch — its history entries
  (other session paths) and its target value (an existing worktree path) are the same kind of
  thing, so it can use `RepoPathInput`'s default/generic hint or omit a custom one.

## 3. Accessibility: current ARIA wiring and dialog-nesting concerns

`RepoPathInput.tsx` (input, lines 156-185) currently sets:
- `aria-autocomplete="list"`
- `aria-controls={listboxId}` (only when dropdown open)
- `aria-activedescendant` (only when a row is keyboard-selected)
- `aria-invalid`, `aria-describedby` for error state

`PathCompletionDropdown.tsx` sets `role="listbox"` on the `<ul>` and `role="option"` +
`aria-selected` on each `<li>`.

**Gap found:** neither the `<input>` nor its wrapping `container` div has **`role="combobox"`**.
Per the WAI-ARIA Authoring Practices combobox pattern, the input element needs `role="combobox"`
(in addition to `aria-autocomplete`, `aria-expanded`, and `aria-controls`) for assistive tech to
announce it as a combobox rather than a plain text field with orphaned `aria-controls`/
`aria-activedescendant` attributes that a screen reader has no reason to associate with listbox
semantics. `aria-expanded` is also missing entirely (should reflect `showDropdown`). This is a
pre-existing gap in the shared component, not something introduced by this project — but since
this project's R7 asks for e2e coverage and this is exactly the kind of check `ux:review`/Axe
would flag once `RepoPathInput` gains two more high-traffic call sites, it's worth fixing
alongside this change rather than treating it as pure scope creep: add `role="combobox"`,
`aria-expanded={showDropdown}`, and `aria-haspopup="listbox"` to the input. This is a small,
additive change (no behavior change) and directly benefits all five consumers, matching the
project's own precedent of "shared hook/selector fixes benefit all consumers incidentally" (see
requirements.md's R3 framing).

**Dialog nesting:** `Omnibar.tsx` already renders its container with `role="dialog"` and
`aria-modal="true"` (`Omnibar.tsx:1203-1204`), and `OmnibarCreationPanel` is a descendant of that
dialog, not a second nested dialog — so there's no double-dialog/focus-trap conflict to worry
about structurally. The only real hazard is exactly what requirements.md already identified: the
Escape-key bubbling problem (R6). Confirmed in code:
- `Omnibar.tsx:1210` attaches `onKeyDown={handleKeyDown}` to the outer `modal` div; React's
  synthetic event system means any `keydown` on a descendant (including inside
  `RepoPathInput`'s `<input>`) bubbles into this handler.
- Four existing dropdown-closing code paths inside `Omnibar.tsx` (`handleKeyDown`, around lines
  748, 772, 820, 839) already call `e.nativeEvent.stopImmediatePropagation()` specifically to
  prevent a "close my own dropdown" Escape from also triggering the document-level listener at
  `Omnibar.tsx:891-901` that calls `onClose()`.
- `RepoPathInput.tsx:134-137`'s own Escape case (`setOpen(false); setSelectedIndex(-1);`) does
  **not** call `stopPropagation`/`stopImmediatePropagation` today, because none of its four
  existing call sites are nested inside a keydown-capturing ancestor.

**Fix required (confirms R6):** `RepoPathInput`'s Escape handler needs to stop propagation *only
when it actually closed an open dropdown* — i.e. guard on `open` being true before calling
`e.stopPropagation()` (React synthetic `stopPropagation` is sufficient here since the parent
listener is also a React `onKeyDown`, unlike the native `document` listener the existing
Omnibar code paths are defending against — worth double-checking whether `e.stopPropagation()`
or the nativeEvent variant is correct for this specific ancestor). This preserves normal Escape
behavor (bubble through, let Omnibar reset/close) when the dropdown is already closed, matching
the "no regression to existing Escape-to-reset behavior when no dropdown is open" requirement.

## 4. Error states and edge cases

- **Empty history (no sessions yet).** `PathCompletionDropdown` returns `null` when
  `entries.length === 0` and not loading (`PathCompletionDropdown.tsx:82`), and `RepoPathInput`
  only renders the wrapper when `showDropdown` is true (`allEntries.length > 0 || isLoading`).
  So a first-run user with zero session history sees no empty-state dropdown flash — the field
  behaves like a plain text input until filesystem completions start arriving (once `value` is
  non-empty, `usePathCompletions` kicks in). This is correct/expected; no dedicated empty-state
  UI is needed since the absence of a dropdown *is* the empty state, consistent with how a
  browser address bar shows nothing until there's something to show.
- **Typed path that doesn't exist yet — New Project mode.** This is expected and desired: the
  whole point of Parent Directory is a directory that may not contain the project yet (though the
  *parent* directory itself is generally expected to already exist, since the app creates the
  project folder inside it, not the parent chain). `RepoPathInput` doesn't validate existence
  itself — `usePathCompletions` simply won't return filesystem matches for a path that doesn't
  resolve, and the dropdown just falls back to history-only or closes. No error state is
  triggered by a nonexistent path, which matches this mode's needs. Confirm `OmnibarCreationPanel`
  doesn't add its own existence-validation error affordance for `parentDir` that would
  contradict this (worth a quick grep during implementation — out of scope for this research
  pass to fully audit `canSubmit` logic).
- **Typed path that doesn't exist — Existing Worktree Path fallback.** Here a nonexistent path
  *is* a real error (the field only appears when worktree discovery already failed/returned
  nothing, so there's no server-side fallback validation happening later either). `RepoPathInput`
  has an `error`/`hint` prop pair already used by `BacklogItemForm` for exactly this kind of
  validation surface (`error={errors.repoPath}`). The implementation should wire real-time or
  submit-time existence/validity checking through that existing `error` prop rather than
  inventing new error UI — but the *dropdown* itself doesn't need new error-state design, since
  "no filesystem completions found" is already handled silently (empty section, not an error
  message inside the dropdown).
- **Touch targets at 390×844.** Each `PathCompletionDropdown` row (`item` style,
  `PathCompletionDropdown.css.ts:14-29`) has `padding: "7px 16px"` at `fontSize: 13`, giving a row
  height of roughly 30-32px — full width (`width: 100%` on the dropdown wrapper,
  `RepoPathInput.css.ts:58-62`, `left:0; right:0` so it can't overflow the input's own container
  horizontally). 30-32px clears the WCAG 2.2 AA minimum target size (24×24 CSS px, SC 2.5.8) but
  falls short of the AAA-level 44×44 comfort target most mobile design systems (iOS HIG, Material)
  recommend for primary tap targets. Since rows are full-width and stacked (not adjacent
  small targets), the AA bar is what matters most for compliance, but implementers should verify
  visually at 390×844 that: (a) the dropdown's `maxHeight: 200` combined with a phone's on-screen
  keyboard taking up roughly half the viewport doesn't push the list off-screen or force it
  under the keyboard, and (b) `overflowY: "auto"` scrolling inside the 200px cap works with touch
  scroll gestures, not just wheel/keyboard. This is squarely what R5 already asks to verify
  manually/via Playwright at the 390×844 viewport — no code change is obviously required here
  unless real-device testing shows a problem, but it's the one item in this list most likely to
  need one if the on-screen keyboard occludes the dropdown in practice.

## 5. Jobs-to-be-done

The recent-path suggestion feature is hired for three related jobs, in priority order:

1. **Speed** (functional): avoid re-typing or re-navigating a full absolute path the user has
   typed before for a different session against the same repo. This is the dominant job — most
   users creating a second/third session against a repo they've already worked in want zero
   friction, not discovery.
2. **Typo/error reduction** (functional): selecting from a list eliminates the class of bugs
   where a hand-typed path has a subtle wrong segment (case, trailing slash, symlink vs. real
   path) that only surfaces as a confusing "path not found" error later. This matters more for
   the Existing Worktree Path fallback (where a wrong path is a hard error) than for New Project
   mode (where the field tolerantly accepts arbitrary future paths).
3. **Discoverability / recall** (emotional + functional): for a user managing many concurrent
   repos/worktrees (this app's core use case — multi-session AI agent orchestration), the history
   list acts as a lightweight "recently used projects" memory aid, reducing the cognitive load of
   remembering exactly where on disk each project lives. This is the weakest-but-still-real job
   for the Parent Directory field specifically, where the suggested paths aren't literally what
   the user wants but serve as landmarks ("oh right, my projects live under `~/Projects/...`").

The recency-ordering fix (R3) directly serves job #1 and #3: an incidental adapter-order list
optimizes for none of these jobs (a user's most recently touched project should be the fastest to
reach), while `selectActiveSessionsSortedByUpdatedAt` with a defined tiebreak makes "most recent
first" a reliable, learnable property of the list — the same property that makes browser history
autocomplete and VS Code's MRU file list trustworthy enough that users stop double-checking them.
