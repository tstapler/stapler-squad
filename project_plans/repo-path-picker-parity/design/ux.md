# UX Design: Repo Path Picker Parity

Source inputs: `requirements.md`, `research/ux.md`, `implementation/plan.md`. This document
does not re-derive findings already established in research — it turns them into concrete
wireframes, interaction flows, and testable acceptance criteria for the two fields being
swapped (`Parent Directory`, `Existing Worktree Path` fallback) inside
`OmnibarCreationPanel.tsx`, a modal-nested React form.

Component facts used below (confirmed in code, not assumed):
- History rows use a clock icon (`🕒`), fs rows use a folder icon (`📁`) or file icon (`📄`);
  a `divider` `<li>` separates the two groups only when both are non-empty
  (`PathCompletionDropdown.tsx:84,103-105`).
- Dropdown only renders when `open && (allEntries.length > 0 || isLoading)`
  (`RepoPathInput.tsx:99`) — there is no dedicated "no results" row; empty means no dropdown.
- History is capped at 5 entries (`MAX_HISTORY = 5`, `RepoPathInput.tsx:31`) and filtered
  case-insensitively against the current typed value (`RepoPathInput.tsx:71-73`).
- History paths are tilde-abbreviated for display (`tildeAbbreviate`, `RepoPathInput.tsx:33-36`)
  but the full path is still the `title` attribute and the value committed on select.

---

## Surface 1 — Parent Directory: closed / empty-history state (desktop)

New Project mode, field just rendered, not yet focused, and/or the user has no session
history at all.

```
┌─ New Project ──────────────────────────────────────────────┐
│ Session Name *                                              │
│ [ my-new-thing____________________________ ]                │
│                                                               │
│ Parent Directory *                                           │
│ [ ~/Projects_______________________________ ]  ← placeholder │
│  Recent paths below are existing project folders — pick      │
│  one to use its parent, or type a new directory.             │
│                                                               │
│                              [Cancel]  [Create Project]      │
└───────────────────────────────────────────────────────────┘
```

**Flow**
1. User selects "New Project" mode → Parent Directory field mounts, `value=""`, `open=false`.
2. Field renders as a plain-looking text input (visually identical to the old `<input>`) with
   the New-Project-specific hint already visible below it — the hint is static, not
   dropdown-dependent, so it is present even before the user interacts.
3. No dropdown is shown yet, whether or not history exists — dropdown only opens on focus.

**Edge case — zero session history ever.** If `useSessionRepoPaths()` returns `[]`, focusing
the field later (Surface 2) with an empty value renders no dropdown at all (`allEntries.length
=== 0`, not loading) — the field silently behaves like a plain text input. No "no suggestions
yet" message is shown; this matches the research finding that "absence of dropdown is the
empty state" and needs no bespoke UI.

---

## Surface 2 — Parent Directory: open dropdown, history + filesystem entries (desktop)

User has focused the field; history exists; user has optionally started typing a path that
also has live filesystem matches.

```
┌─ New Project ──────────────────────────────────────────────┐
│ Parent Directory *                                           │
│ [ ~/Proj|_________________________________ ]  ← caret here   │
│ ┌───────────────────────────────────────────┐               │
│ │ 🕒 ~/Projects/stapler-squad                │ ← history     │
│ │ 🕒 ~/Projects/dotfiles                     │               │
│ │ 🕒 ~/Projects/personal-wiki                │               │
│ │ ─────────────────────────────────────────  │ ← divider     │
│ │ 📁 Projects/                                │ ← fs match    │
│ │ 📁 Projects-archive/                        │               │
│ └───────────────────────────────────────────┘               │
│  Recent paths below are existing project folders — pick      │
│  one to use its parent, or type a new directory.             │
└───────────────────────────────────────────────────────────┘
```

**Flow**
1. Focus (`onFocus`) → `open=true`. If `value===""`, all history entries (up to 5) show,
   unfiltered; no filesystem entries yet since `usePathCompletions` is `enabled: value.length
   > 0`.
2. User types → `onChange` fires on every keystroke, `open` stays `true`, `selectedIndex`
   resets to `-1`. History list filters case-insensitively; after a short debounce (150ms,
   per `usePathCompletions`, untouched by this project) filesystem entries for the typed
   prefix arrive and append below a divider, deduped against any path already shown as history.
3. Keyboard: ArrowDown/ArrowUp move `selectedIndex` through the combined list (history rows
   first, then fs rows); Enter on a highlighted row commits it (`onChange(entry.path)`,
   closes dropdown). Mouse click (`onMouseDown` + `preventDefault`) commits identically.
4. Selecting a history row is a full-path replace of the typed value — per `ux.md` §2, the
   app deliberately does **not** truncate the selected history path to its dirname even
   though the field is labeled "Parent Directory." The static hint (visible in Surface 1 and
   here) is the only mitigation; this is a copy-only fix, not a behavior change.

**Edge case — RPC failure in `usePathCompletions`.** This hook is untouched by this project
(explicitly out of scope, `requirements.md` "Out of scope"). Current behavior on fetch failure:
`fsEntries` stays empty and `isLoading` resolves to `false` — the filesystem section of the
dropdown is simply absent, with history entries (if any) still shown normally above where the
divider would have been. No error banner, no retry button, no visible "completions
unavailable" state. This is a pre-existing behavior this project does not change; call out
in the acceptance criteria below as a known non-goal so a reviewer doesn't mistake its
absence for a regression.

---

## Surface 3 — Parent Directory: typed text with no match (desktop)

User types a path that matches neither session history nor any live filesystem entry.

```
┌─ New Project ──────────────────────────────────────────────┐
│ Parent Directory *                                           │
│ [ /tmp/brand-new-parent-dir________________ ]                │
│  (no dropdown — allEntries.length === 0, isLoading === false)│
│  Recent paths below are existing project folders — pick      │
│  one to use its parent, or type a new directory.             │
└───────────────────────────────────────────────────────────┘
```

**Flow**
1. User types a path with no history/fs match → `allEntries` becomes `[]` once any pending
   fs-completion request resolves empty → `showDropdown` becomes `false` → dropdown
   disappears entirely (not "shown empty," per `PathCompletionDropdown.tsx:82` returning
   `null` at zero entries and non-loading).
2. The typed value in the `<input>` is never altered, cleared, or reverted — `value` is fully
   controlled by the parent's `parentDir` state via `onChange`, and nothing inside
   `RepoPathInput`/`usePathCompletions` writes back to it except an explicit user selection
   (R4's "manual free-text entry must keep working" contract).
3. Submitting the form with this value is allowed — New Project mode does not validate parent
   directory existence client-side (confirmed: no `error` prop is wired for this field per the
   plan's Pattern Decisions table — "Out of scope: field-level error/validation for the two
   Omnibar fields"). Any invalid-path failure surfaces later from the backend `CreateSession`
   call, using whatever error-surfacing the Omnibar already has for RPC failures (unchanged by
   this project).

---

## Surface 4 — Existing Worktree Path fallback: closed / empty-history state (desktop)

Existing Branch mode, worktree auto-discovery already ran and found zero worktrees (or
errored) — the fallback field is now the only path input for this mode.

```
┌─ Existing Branch ────────────────────────────────────────────┐
│ Branch *                                                       │
│ [ feature/my-branch________________________ ]                 │
│                                                                  │
│ Existing Worktree Path *                                       │
│ [ /path/to/existing/worktree_______________ ]  ← placeholder   │
│  No worktrees found for this branch. Enter the path manually.  │
│  (existing sibling hint span — unchanged, panel-level, not a    │
│   RepoPathInput prop)                                           │
│                              [Cancel]  [Create Session]        │
└────────────────────────────────────────────────────────────┘
```

**Flow**
1. Same mount/focus/dropdown lifecycle as Surface 1 — no New-Project-specific hint here.
   `RepoPathInput` is rendered with no `hint` prop for this field (plan explicitly avoids a
   second overlapping hint string; the panel's existing discovery-state span stays outside
   the component, immediately below it).
2. Because this field's value and its history entries are literally the same kind of thing
   (a worktree/repo root path), there is no "parent vs. leaf" mismatch to caption — unlike
   Surface 1, no copy change is needed for this field (per `ux.md` §2's explicit carve-out).

---

## Surface 5 — Existing Worktree Path fallback: open dropdown (desktop)

```
┌─ Existing Branch ────────────────────────────────────────────┐
│ Existing Worktree Path *                                       │
│ [ ~/.stapler-squad/worktrees/rep|__________ ]                  │
│ ┌────────────────────────────────────────────┐                │
│ │ 🕒 ~/.stapler-squad/worktrees/repo-x         │                │
│ │ 🕒 ~/.stapler-squad/worktrees/repo-y         │                │
│ │ ─────────────────────────────────────────   │                │
│ │ 📁 repo-x-backup/                             │                │
│ └────────────────────────────────────────────┘                │
│  No worktrees found for this branch. Enter the path manually.  │
└────────────────────────────────────────────────────────────┘
```

**Flow** — identical mechanics to Surface 2 (history-first, filesystem-second, divider,
type-to-filter, keyboard/mouse commit). Same RPC-failure behavior caveat applies (silently
empty fs section, no error UI change from this project).

**Edge case — selecting a wrong/stale history path.** Per `ux.md` §5, a wrong path is a *real*
error for this field specifically (unlike Parent Directory) because worktree discovery already
failed and there's no other server-side fallback validation surfaced inline. This project
intentionally does **not** add new inline validation (`error` prop) for this field — see
Surface 3's note on scope. The failure mode a user hits is: pick/type a bad path → submit →
whatever `CreateSession` RPC error handling already exists in the Omnibar fires (unchanged,
out of scope). Flagged here as a known gap for a future ticket, not a defect of this change.

---

## Surface 6 — Existing Worktree Path fallback: typed text with no match (desktop)

Same shape as Surface 3: dropdown disappears when nothing matches, typed value is preserved
verbatim, no auto-correction or reversion.

```
┌─ Existing Branch ────────────────────────────────────────────┐
│ Existing Worktree Path *                                       │
│ [ /home/user/typo-path-not-found___________ ]                  │
│  (no dropdown)                                                  │
│  No worktrees found for this branch. Enter the path manually.  │
└────────────────────────────────────────────────────────────┘
```

---

## Surface 7 — Escape key: dropdown open

```
Before Escape                          After Escape (1st press)
┌───────────────────────────┐          ┌───────────────────────────┐
│ Parent Directory *          │          │ Parent Directory *          │
│ [ ~/Proj|_________ ]        │  Esc →   │ [ ~/Proj|_________ ]        │
│ ┌─────────────────────┐    │          │ (dropdown closed, value      │
│ │ 🕒 ~/Projects/x       │    │          │  unchanged, New Project      │
│ │ 🕒 ~/Projects/y       │    │          │  mode still selected,        │
│ └─────────────────────┘    │          │  panel still open)           │
└───────────────────────────┘          └───────────────────────────┘
     RepoPathInput: showDropdown=true       RepoPathInput: showDropdown=false
     (dropdown visible)                     event.nativeEvent
                                             .stopImmediatePropagation()
                                             called → Omnibar's own
                                             keydown handler never runs
```

**Flow**
1. Dropdown is visibly rendered (`showDropdown === true`, i.e. `open === true` AND
   (`allEntries.length > 0` OR `isLoading`)). User presses Escape while the input has focus.
2. `RepoPathInput`'s `handleKeyDown` "Escape" case (fixed by this project, Task 1.1.1a) checks
   `showDropdown` — **not** `open` — and calls `e.nativeEvent.stopImmediatePropagation()`
   before closing, only when `showDropdown` is `true`. Gating on `open` alone would be wrong:
   `open` becomes `true` on mere focus even when the dropdown renders nothing (e.g. empty
   history and no filesystem matches), which would wrongly swallow Escape in that case too
   — see Surface 8's "focused-but-empty" sub-case below and the plan's Domain Glossary
   `open`/`showDropdown` distinction.
3. Local state closes (`setOpen(false)`, `setSelectedIndex(-1)`); the typed `value` is **not**
   altered.
4. Because propagation is stopped at the native-event level, `Omnibar.tsx`'s `modal`-level
   `onKeyDown={handleKeyDown}` (React synthetic listener) and its `document`-level native
   Escape listener (which would otherwise call `onClose()`) never observe this keydown at all.
   Net effect: exactly one Escape press closes only the suggestion list; the New Project /
   Existing Branch panel, its selected mode, and all other field values remain untouched.
5. A second Escape press (dropdown now closed, `showDropdown === false`) behaves as Surface 8
   below — this is the "two presses to fully back out" model, matching how VS Code's command
   palette and browser omniboxes handle a nested suggestion list under a larger dismissible
   surface.

---

## Surface 8 — Escape key: dropdown not visibly rendered (`showDropdown === false`)

```
┌───────────────────────────┐          ┌──────────────────────────────┐
│ Parent Directory *          │  Esc →   │ (Omnibar's own Escape         │
│ [ ~/Projects/foo___ ]       │          │  behavior fires normally:     │
│ (no dropdown open)          │          │  reset-to-discovery / close   │
└───────────────────────────┘          │  omnibar, per pre-existing    │
                                          │  Omnibar.tsx logic, unchanged)│
                                          └──────────────────────────────┘
     RepoPathInput: showDropdown=false
```

**Flow**
1. `showDropdown` is `false` — either the field was never focused (`open === false`), a
   prior Escape already closed it (see Surface 7 step 5), **or the field is focused with
   `open === true` but the dropdown has nothing to render** (see the "focused-but-empty"
   sub-case below). All three collapse to the same observable behavior.
2. `RepoPathInput`'s Escape case checks `showDropdown` — not `open` — sees it is `false`, and
   does **not** call `stopImmediatePropagation()` (Task 1.1.1a's guard).
3. The keydown bubbles normally to `Omnibar.tsx`'s `modal`-level handler and, if that doesn't
   consume it, to the `document`-level listener — both fire exactly as they did before this
   project touched anything (R6's explicit "no regression to existing Escape-to-reset
   behavior when no dropdown is open" requirement).

**Sub-case — focused but empty (`open === true`, `showDropdown === false`).** This is the
case an Escape guard gated on `open` alone would get wrong, and it is exactly why the fix
gates on `showDropdown` instead (see Surface 7 step 2 and the plan's Domain Glossary
`open`/`showDropdown` distinction). A user focuses the field (`open` becomes `true`) while
it has no session history and no filesystem matches yet (`allEntries.length === 0`,
`isLoading === false`) — per Surface 1's "zero session history ever" edge case, no dropdown
renders at all. Pressing Escape here must **not** stop propagation, even though `open` is
technically `true`: there is nothing visible to the user to dismiss, so the keydown should
bubble exactly like the fully-closed case above and let `Omnibar.tsx`'s own Escape handler
fire normally. Plan Task 1.1.1c's third unit test case exists specifically to pin this
behavior.

---

## Surface 9 — Mobile (390×844): Parent Directory, dropdown open

```
┌ 390px ─────────────────────────────────┐
│ ← New Project                            │
│                                           │
│ Session Name *                           │
│ [ my-new-thing_______________ ]          │
│                                           │
│ Parent Directory *                       │
│ [ ~/Proj|_____________________ ]         │
│ ┌───────────────────────────────┐       │
│ │ 🕒 ~/Projects/stapler-squad     │  30-  │
│ │ 🕒 ~/Projects/dotfiles          │  32px │
│ │ 🕒 ~/Projects/personal-wiki     │  rows │
│ │ ──────────────────────────────  │       │
│ │ 📁 Projects/                     │       │
│ └───────────────────────────────┘ ↕ scroll│
│  Recent paths below are existing         │
│  project folders — pick one to use       │
│  its parent, or type a new directory.    │
│                                           │
│ ══════════ on-screen keyboard ══════════ │
│  q w e r t y u i o p                     │
│  a s d f g h j k l                       │
│    z x c v b n m  ⌫                       │
└─────────────────────────────────────────┘
```

**Flow / risk called out by research (`ux.md` §4)**
1. `PathCompletionDropdown` is full-width (`left:0; right:0` on its wrapper) — confirmed no
   horizontal overflow risk at 390px, since it's bounded by the input's own container, not a
   fixed pixel width.
2. `maxHeight: 200` with `overflowY: auto` — at 390×844 with an on-screen keyboard consuming
   roughly the bottom half of the viewport (~400px), the dropdown could render partially or
   fully behind/under the keyboard if the field sits low in the panel's scroll area. This is
   the single highest-risk item flagged in research and is exactly what Surface 9/10's e2e
   coverage (`Task 3.1.1f`, viewport bounding-box assertion) exists to catch.
3. Touch scroll inside the 200px-capped list must work via touch gesture, not just
   wheel/keyboard — verify manually on a real device or emulated touch, not just a mouse-driven
   Playwright run, since Playwright's synthetic scroll doesn't always exercise the same code
   path as native touch momentum scrolling.
4. Row height (~30–32px) clears WCAG 2.2 AA's 24×24px minimum target size but is below the
   44×44px iOS/Material comfort target. Acceptable per AA, flagged as a nice-to-have polish
   item, not a blocking defect (see Acceptance Criteria below).

---

## Surface 10 — Mobile (390×844): Existing Worktree Path fallback, dropdown open

Same layout and risk profile as Surface 9, mirrored for the Existing Branch mode panel and
the fallback field's own sibling hint (worktree-discovery status text) instead of the
New-Project hint.

```
┌ 390px ─────────────────────────────────┐
│ ← Existing Branch                        │
│                                           │
│ Branch *                                 │
│ [ feature/my-branch__________ ]          │
│                                           │
│ Existing Worktree Path *                 │
│ [ ~/.stapler-squad/worktre|__ ]          │
│ ┌───────────────────────────────┐       │
│ │ 🕒 ~/.stapler-squad/worktrees/  │       │
│ │    repo-x                       │       │
│ │ 🕒 ~/.stapler-squad/worktrees/  │       │
│ │    repo-y                       │       │
│ └───────────────────────────────┘ ↕ scroll│
│  No worktrees found for this branch.     │
│  Enter the path manually.                 │
│ ══════════ on-screen keyboard ══════════ │
└─────────────────────────────────────────┘
```

Same on-screen-keyboard-occlusion risk as Surface 9 — same e2e check pattern applies to both
fields (Task 3.1.1f targets Parent Directory as the "deepest-in-form" worst case; this
surface gets the same automated `getBoundingClientRect()` assertion, including the row-size
tap-target check, via plan.md's Task 3.1.1f-worktree / `T-E2E-RPP-008`).

---

## UX Acceptance Criteria

Numbered `UX-AC-#` for traceability back to this design doc; cross-referenced to the
requirements doc's `AC#` and plan Story numbers where applicable.

### Task completion / efficiency

1. **UX-AC-1** — A user who has created a session against a given path before can create a
   *new* New-Project session using that path's parent by: focus Parent Directory field (1
   action) → click the matching history row (1 action) → 2 total interactions, 0 characters
   typed. (Maps to AC2/AC4, Surface 2.)
2. **UX-AC-2** — A user filling the Existing Worktree Path fallback with a previously-used
   worktree path can do so in ≤ 2 actions (focus + click), identical to UX-AC-1. (Surface 5.)
3. **UX-AC-3** — A user typing a brand-new path never previously seen completes entry in
   exactly as many keystrokes as the path's length — the dropdown never intercepts, replaces,
   or appends to what they type. (Maps to AC4, Surfaces 3 & 6.)

### Error / edge-case handling — no dead ends

4. **UX-AC-4** — When `usePathCompletions` returns zero results or is loading with zero
   history, the field shows no dropdown and no error text; the user can always still type a
   full path manually and submit — this is not a dead end because the plain-text path remains
   fully functional. (Surfaces 1, 3, 4, 6.)
5. **UX-AC-5** — Selecting a history entry in the Parent Directory field never blocks
   subsequent editing: after selection, the field is a normal focused, editable input and the
   user can immediately type to modify the selected path further. (Surface 2, step 4.)
6. **UX-AC-6** — A submit attempt with an invalid/nonexistent Existing Worktree Path does not
   silently fail — whatever error surfacing the Omnibar/`CreateSession` RPC already provides
   pre-project remains reachable and visible after this change (regression check only; this
   project does not add new inline validation for this field — confirmed out of scope). No UI
   state introduced by this change can leave the user stuck with no visible next step.

### Escape-key interaction

7. **UX-AC-7** — Given the dropdown is open, exactly one Escape press closes only the dropdown;
   the panel, selected session-type mode, and all field values are unchanged, and no
   ancestor Escape handler (reset-to-discovery, close-omnibar) fires on that same press. (Maps
   to AC6, Surface 7.)
8. **UX-AC-8** — Given the dropdown is closed, Escape produces the exact same panel
   reset/close behavior as before this project shipped — zero regression, verified against the
   pre-existing Omnibar Escape contract. (Maps to AC6, Surface 8.)
9. **UX-AC-9** — From any state (dropdown open or closed, any field), the user can always fully
   exit the New Project / Existing Branch panel using no more than two consecutive Escape
   presses (one to close a dropdown if open, one to close/reset the panel) — no state exists
   where Escape does nothing or where more than two presses are required.

### Mobile (390×844)

10. **UX-AC-10** — At 390×844, the dropdown for both fields renders with zero horizontal
    overflow, verified via `page.evaluate()` + `getBoundingClientRect()` inside the browser
    context (`dropdown.right <= 390`), **not** Playwright's `boundingBox()` — `boundingBox()`
    reports the element's own box regardless of visual clipping by an `overflow: hidden`
    ancestor (the Omnibar modal), which would produce a false-positive pass on exactly the
    bug this AC exists to catch. This is the same method and rationale plan.md's Task 3.1.1f
    commits to; see that task for the full snippet. (Maps to AC5, Surfaces 9–10.)
11. **UX-AC-11** — At 390×844, with the on-screen keyboard visible, the dropdown for the
    Parent Directory field (the deepest/lowest field in the New Project panel, the worst-case
    position) is not clipped below the visible viewport — its full row set is either fully
    visible or scrollable via touch within its own `overflowY:auto` container, never hidden
    entirely behind the keyboard with no way to reveal it. (Maps to AC5, Surface 9.)
12. **UX-AC-12** — Every dropdown row is a full-width tap target ≥ 24×24 CSS px (WCAG 2.2 AA
    SC 2.5.8), verified at 390×844 via the same `getBoundingClientRect()` pass used for
    UX-AC-10/11 (plan.md Tasks 3.1.1f and 3.1.1f-worktree). (Surfaces 9–10; current ~30–32px
    rows already clear this — criterion exists to catch a future regression, not a known gap
    today.)

### Accessibility

13. **UX-AC-13** — Both fields' inputs are keyboard-operable end-to-end with no mouse: Tab to
    focus, type to filter, ArrowDown/ArrowUp to move selection, Enter to commit, Escape to
    close — no interaction in Surfaces 2/5 requires a pointer.
14. **UX-AC-14** — Both fields' inputs expose `role="combobox"`, `aria-expanded` (reflecting
    live open state), `aria-haspopup="listbox"`, `aria-autocomplete="list"`,
    `aria-controls` (when open), and `aria-activedescendant` (when a row is keyboard-selected)
    — the full WAI-ARIA combobox triad plus pre-existing relationship attributes. Verified via
    the unit tests in Story 1.1.2 and spot-checked with a screen reader (VoiceOver/NVDA) on at
    least one field.
15. **UX-AC-15** — Each dropdown row is announced with its full path (via `title` and visible
    text) and its provenance (history vs. filesystem) is not solely conveyed by icon color/glyph
    alone to a screen reader — `PathCompletionDropdown.tsx`'s icons are `aria-hidden="true"`,
    so the row's accessible name must come from visible text alone; confirm the rendered name
    (tilde-abbreviated path) is unambiguous without needing to see the 🕒/📁 glyph. (Flag as a
    verification item — no visible defect found in this design pass, but call it out since
    icon-only distinction with `aria-hidden` icons is a common a11y miss.)
16. **UX-AC-16** — Text contrast for the input, hint text, and dropdown row text (including the
    muted `itemHistory` style) meets ≥ 4.5:1 against their respective backgrounds in both light
    and dark theme, per the repo's existing token contract (`web-app/src/app/globals.css`) —
    verify the muted history-row color specifically, since dimmed/secondary text is the most
    likely contrast failure point in this design.
17. **UX-AC-17** — Both field labels (`Parent Directory *`, `Existing Worktree Path *`) remain
    correctly associated to their inputs via `<label htmlFor>` after the swap — verified by the
    existing e2e locators `page.getByLabel('Parent Directory *')` /
    `page.getByLabel('Existing Worktree Path *')` continuing to resolve correctly (regression
    guard, not a new criterion — restates AC1/AC7's "existing tests still pass" requirement in
    UX terms).

### No dead ends (summary check)

18. **UX-AC-18** — Every surface documented above (1–10) has a confirmed exit path: Escape
    (Surfaces 7–8), click-outside (`RepoPathInput.tsx:143-152`, closes dropdown, not tested
    above but present in code), Tab to the next field, or simply continuing to type. No surface
    in this design traps focus or input.

---

## Summary of what this document does not re-litigate

- Whether to build a new picker vs. reuse `RepoPathInput` — settled by R1/plan, not a UX
  design question.
- Auto-truncating a selected history path to its parent directory for the New-Project field —
  explicitly rejected in `ux.md` §2 and the plan's Pattern Decisions table; this document's
  Surface 2/3 wireframes reflect that rejection (full-path replace, copy-only mitigation).
- New inline validation/error UI for either field — explicitly out of scope per the plan;
  Surfaces 3 and 6 describe the resulting (unchanged) behavior rather than proposing new error
  states.
