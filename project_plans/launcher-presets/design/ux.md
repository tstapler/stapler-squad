# UX Design: Launcher Presets

Status: Draft | Phase: Design (post-plan, pre-implementation)

This document specifies every user-facing surface for Launcher Presets, building directly on
`research/ux.md`'s finding that the feature should closely mirror the existing Alias pattern
(`AliasPalette.tsx`, `AliasDetector.ts`, `useAliases.ts`) and on `implementation/plan.md`'s
actual component/file structure (`OmnibarPresetList.tsx` + `.css.ts`, `useLauncherPresets.ts`,
`PresetDetector.ts`, `handlePresetSelect`, `preset-resolution-chip`). No new interaction model
is introduced — every surface below reuses an ARIA shape, a component structure, or a copy
pattern that already ships in this codebase, cited inline.

---

## 1. Surface Inventory

| # | Surface | Type | New/Reused pattern |
|---|---|---|---|
| 1 | Presets section — collapsed header | Screen region | Reuses `collapsible`/`collapsibleHeader` (Advanced Options pattern) |
| 2 | Presets section — populated list | Screen region | Reuses `AliasPalette` listbox/row shape |
| 3 | Presets section — empty state | Empty state | Reuses `AliasPalette`'s "No aliases yet" `role="status"` shape |
| 4 | Presets section — load error state | Error state | Reuses `AliasPalette`'s `alias-config-error` `role="alert"` shape |
| 5 | Preset selection → form prefill + resolution chip | Interaction flow | Reuses `alias-resolution-chip` pattern |
| 6 | Soft PATH warning on unrecognized program | Inline warning | New composition of existing warning treatment (`pathDoesNotExist`-style) |
| 7 | `preset:<id>` typed shorthand — resolved | Detector flow | Reuses `AliasDetector`/`WorkflowDetector` typed-shorthand pattern |
| 8 | `preset:<id>` typed shorthand — not found | Error state | Reuses `alias-not-found` shape |
| 9 | Loading state (presets fetch in flight) | Loading state | New (no direct alias precedent — aliases load synchronously enough that no loading UI exists today; presets need one because the RPC is fresh-per-call) |

Nine surfaces total. Surfaces 1–4 and 9 live in `OmnibarPresetList.tsx`; 5–6 live in
`Omnibar.tsx`/`OmnibarCreationPanel.tsx`; 7–8 live in the Omnibar's typed-input detection area
(same DOM region as the existing `alias-resolution-chip`/`alias-not-found` chips).

---

## 2. Surface 1–4 + 9: The Presets Section (`OmnibarPresetList.tsx`)

### 2.1 Placement

Per plan Task 5.1.1d, the Presets section sits **above** "Advanced Options" in
`OmnibarCreationPanel.tsx`, so it is visible without an extra click, matching
`research/ux.md` §1's progressive-disclosure principle (presets are a fast path layered on
top of the form, not new complexity buried in Advanced Options).

```
┌─ Omnibar Creation Panel ─────────────────────────────────────────────┐
│ ○ New branch (isolated)   ○ Existing folder   ○ Temporary  [▾ More]  │  ← SessionTypeRadioGroup
│ Path: [___________________________________]                         │
│                                                                       │
│ ▾ Presets                                                            │  ← Surface 1/2/3/4 (this section)
│   ┌───────────────────────────────────────────────────────────────┐ │
│   │ (listbox / empty / error content — see 2.2–2.5)               │ │
│   └───────────────────────────────────────────────────────────────┘ │
│                                                                       │
│ First Prompt (optional): [__________________________________]       │
│                                                                       │
│ ▸ Advanced Options                                                   │
│                                                                       │
│ [Cancel]                                              [Create Session]│
└───────────────────────────────────────────────────────────────────────┘
```

Collapse state (Task 5.1.1c): `expanded = presets.length > 0` at mount — a populated list
defaults open (the feature's value should be visible on first use, per Story 5.1.1's 4th AC);
an empty or errored list defaults collapsed (cheap to show collapsed, nothing actionable to
surface immediately). Header is always clickable/toggleable regardless of state.

### 2.2 Populated state (Surface 2)

```
▾ Presets
┌─────────────────────────────────────────────────────────────────────┐
│ role="listbox" aria-label="Launcher presets"                        │
│ ┌───────────────────────────────────────────────────────────────┐   │
│ │ role="option" tabIndex=0  data-testid="preset-row"             │   │ ← selected/focused
│ │ Codex (gpt-5)                                                   │   │
│ │ codex --model gpt-5                                             │   │
│ └───────────────────────────────────────────────────────────────┘   │
│ ┌───────────────────────────────────────────────────────────────┐   │
│ │ role="option" tabIndex=-1  data-testid="preset-row"            │   │
│ │ Deploy box (ssh)                                                │   │
│ │ ssh -t deploy-box 'cd ~/repo && exec claude'                    │   │
│ └───────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

Row content (Task 5.1.1b): `label` as the primary line, `argv.join(" ")` (or `program` if the
optional field is present, else `argv[0]`) as a secondary/muted line — this is the "verify at a
glance" affordance `research/ux.md` §2 calls for, mirroring the resolution chip's job but
available *before* selection, which is strictly better for the "review before it runs"
emotional job named in §5.

### 2.3 Empty state (Surface 3)

```
▾ Presets
┌─────────────────────────────────────────────────────────────────────┐
│ role="status"  data-testid="preset-list-empty"                      │
│                                                                       │
│   No presets yet.                                                    │
│   Add one in ~/.stapler-squad/launcher-presets.json.                 │
└─────────────────────────────────────────────────────────────────────┘
```

Non-blocking: the rest of the Omnibar form functions identically with zero presets configured
(this is every existing install's default state — see plan.md's Migration Plan). Exact copy is
an Unresolved Question in the plan; the wording above is the suggested default, matching
`AliasPalette`'s two-line title/body structure.

### 2.4 Error state (Surface 4)

```
▾ Presets
┌─────────────────────────────────────────────────────────────────────┐
│ ⚠ role="alert" aria-live="assertive"  data-testid="preset-config-error" │
│                                                                       │
│   Launcher presets failed to load                                    │
│   duplicate preset id "codex" (positions 1 and 3)                    │
│                                                                       │
│   Fix ~/.stapler-squad/launcher-presets.json and reopen the Omnibar   │
│   to retry.                                                           │
└─────────────────────────────────────────────────────────────────────┘
```

The error message shown here is the raw `load_error` string from `GetLauncherPresetsResponse`
— per plan.md's backend design, this string already names the specific problem (duplicate id +
positions, or "preset `foo` has no `argv`", or a JSON parse failure) rather than a generic
"failed to load" message. The exit path is explicit and always available: closing and
reopening the Omnibar re-triggers `useLauncherPresets`'s `isOpen`-transition refetch (plan
Task 3.1.1b), so no separate "Retry" button is required — but the copy should say so, since a
user won't otherwise know that reopening helps (this is the "no dead ends" criterion in §4
below).

This state must never block the rest of the form: Create Session remains enabled, the user can
still manually fill in program/path/flags exactly as if presets didn't exist.

### 2.5 Loading state (Surface 9)

```
▾ Presets
┌─────────────────────────────────────────────────────────────────────┐
│ role="status" aria-live="polite"  data-testid="preset-list-loading"  │
│   Loading presets…                                                    │
└─────────────────────────────────────────────────────────────────────┘
```

Not covered by an existing alias precedent (`useAliases` has no dedicated loading branch
rendered in `AliasPalette` today) but is required here because `GetLauncherPresets` is a
fresh-per-call network RPC re-fired on every Omnibar open (plan.md Epic 3.1) rather than a
value already resident in app state. Shown only during the *first* fetch after the section is
rendered; a background refetch (e.g. re-opening the Omnibar quickly) should not flash this
state over an already-populated list — keep showing the last-known list until the new response
arrives, then swap (avoids layout jank on every open).

---

## 3. Surface 5: Selection → Prefill → Resolution Chip

### 3.1 Interaction flow

```
User action                         System response
──────────────────────────────────  ──────────────────────────────────────────
1. Click a preset row                → formState.program = argv[0]
   (or Tab to it + Enter/Space,        formState.extraArgs = argv[1:]
   or type "preset:<id>" + Enter/Tab)  formState.workingDir = defaultPath (if present)
                                       selectedPresetLabel = preset.label
                                       (no RPC call, no submission)
2. (form re-renders)                 → preset-resolution-chip appears:
                                       role="status" aria-live="polite"
                                       "Preset applied: Deploy box (ssh) · ssh -t
                                        deploy-box · packages/api"
3a. User reviews prefilled fields,   → onSubmit() → CreateSession RPC with
    clicks "Create Session"            program/extraArgs/workingDir as prefilled
3b. User presses Ctrl+Enter/Cmd+Enter → same as 3a, from anywhere in the form
    (power-user path, no extra click)   (existing global shortcut, unmodified)
```

This is the same three-step "detect → confirmation chip → editable form" flow
`research/ux.md` §1 identifies as the load-bearing precedent for Success Criterion #2: **the
click is the fill, not the launch.** Selecting a preset never calls `CreateSession` — the
Create Session button (or Ctrl+Enter) remains the single point of submission, identical to how
`RadioGroup`/alias selection already behaves (`Omnibar.tsx:1073-1170`).

### 3.2 Resolution chip wireframe

```
┌───────────────────────────────────────────────────────────────────────┐
│ role="status" aria-live="polite" data-testid="preset-resolution-chip"│
│ Preset applied: Deploy box (ssh) · ssh -t deploy-box · packages/api  │
└───────────────────────────────────────────────────────────────────────┘
┌─ Program: [ssh                    ▾]  ← prefilled, still editable
┌─ Working dir: [packages/api        ]  ← prefilled, still editable
```

Placed in the same DOM region as `alias-resolution-chip` (`Omnibar.tsx:1335-1345`) for visual
consistency — a user who already understands what that chip means for aliases should
recognize this one for presets without new learning.

### 3.3 Unconditional overwrite — no silent-clobber guard

Unlike the alias auto-fill guard (`!programRef.current || programRef.current ===
lastSuggestedProgramRef.current`), preset selection **unconditionally overwrites**
`program`/`extraArgs`/`workingDir` per Pattern Decisions row 5 — this is a deliberate,
discrete click action (GoF Command), not continuous re-detection on every keystroke, so there
is no "don't clobber what I just typed while still typing" scenario to guard against. If a
user had hand-typed a program before browsing presets, clicking a preset row **replaces** it,
and the resolution chip is the signal that this happened — this must be stated in the row's
`aria-label` too (e.g. `"Preset: Deploy box (ssh). Selecting will replace Program, extra
arguments, and Working directory."`) so a screen-reader user gets the same warning a sighted
user gets implicitly from watching the fields change.

---

## 4. Surface 6: Soft PATH Warning

```
Program
┌─────────────────────────────────┐
│ codex                        ▾  │
└─────────────────────────────────┘
⚠ `codex` not found in PATH — check it's installed
```

Rendered near the Program field (Task 5.1.2a), styled distinctly from a hard error (muted
warning color, not the red `errorClass` used for form-submission failures) and **never
disables Create Session**. This is a genuine "the preset may still be valid, the environment
might just not have it yet" case per `research/ux.md`'s edge-case table — hard-rejecting here
would punish a user who saved a preset for a program they plan to install later, or one on an
absolute path the auto-detector doesn't see.

This warning is computed client-side from `useAvailablePrograms()` at render/selection time
(never a backend load-time check) — see plan.md's explicit rejection of "hard validation
against `AvailablePrograms`" in its Risk Control table, since host-installed binaries are
environment-dependent and can change after the presets file loads.

---

## 5. Surfaces 7–8: `preset:<id>` Typed Shorthand (Nice to Have)

### 5.1 Resolved (Surface 7)

```
Omnibar input: [preset:codex________________]

┌───────────────────────────────────────────────────────────────────────┐
│ role="status" aria-live="polite" data-testid="preset-resolution-chip"│
│ Preset applied: Codex (gpt-5) · codex --model gpt-5                  │
└───────────────────────────────────────────────────────────────────────┘
```

Typing `preset:<id>` and pressing Tab or Enter routes through the exact same
`handlePresetSelect` used by clicking a row (plan Task 4.2.1c) — one code path, two entry
points, identical resulting UI. No separate browse/autocomplete mode for this shorthand (unlike
`@alias`'s filtered palette) — matching requirements.md's explicit "resolves directly... skipping
manual selection" scope for this Nice-to-Have.

### 5.2 Not found (Surface 8)

```
Omnibar input: [preset:doesnotexist__________]

┌───────────────────────────────────────────────────────────────────────┐
│ role="alert" aria-live="assertive" data-testid="preset-not-found"    │
│ No preset 'preset:doesnotexist'                                       │
└───────────────────────────────────────────────────────────────────────┘
```

Mirrors `alias-not-found`'s exact shape and copy pattern. Critically, this must be a **distinct
detection result**, not `null` (plan Task 3.2.1b's 2nd AC) — if `PresetDetector` returned
`null` for an unknown id, `SessionSearchDetector` (priority 200, lowest) would silently treat
`preset:doesnotexist` as a search query, producing a confusing "no results for
'preset:doesnotexist'" search-empty-state instead of a clear "that preset id doesn't exist"
message. The exit path is immediate: the user edits the typed id or deletes it and falls back
to browsing the Presets list (Surface 2) — no dead end.

---

## 6. End-to-End Flow Diagram

```
┌────────────┐     open Omnibar      ┌──────────────────────┐
│ Any screen │ ─────────────────────▶│ Omnibar (input mode)  │
└────────────┘                       └──────────┬────────────┘
                                                  │
                     ┌────────────────────────────┼─────────────────────────────┐
                     │ type path/branch/etc.       │ type "preset:<id>"           │
                     ▼                              ▼                             │
          ┌─────────────────────┐        ┌────────────────────────┐              │
          │ Creation panel opens │        │ Detector resolves inline│              │
          │ (Presets section     │        │ (Surface 7 or 8)         │              │
          │  visible, Surface     │        └─────────┬────────────────┘              │
          │  1–4/9)               │                  │ found                         │
          └──────────┬───────────┘                  ▼                               │
                     │                    ┌────────────────────────┐                │
        click/Enter  │                    │ Form prefilled + chip    │◀───────────────┘
        on a preset  │                    │ (Surface 5)              │
        row (Surface2)▼                   └─────────┬────────────────┘
          ┌─────────────────────┐                    │
          │ Form prefilled + chip│◀───────────────────┘
          │ (Surface 5)           │
          └──────────┬───────────┘
                     │
        review, optionally edit any field (program/path/branch/prompt)
                     │
        ┌────────────┴─────────────┐
        │ Click "Create Session"    │  or  Ctrl+Enter / Cmd+Enter (from anywhere)
        └────────────┬─────────────┘
                     ▼
          ┌─────────────────────┐
          │ CreateSession RPC     │
          │ (extra_args carried   │
          │  verbatim, argv-safe) │
          └──────────┬───────────┘
                     ▼
          ┌─────────────────────┐
          │ Session card appears  │  (existing post-creation UI, unmodified)
          └─────────────────────┘
```

Every path converges on the same review-then-submit step — there is no branch that launches
without passing through the editable form, satisfying Success Criterion #2 for both the mouse
(row click) and keyboard (`preset:<id>` shorthand) entry points.

---

## 7. Error & Edge-Case Handling Summary

| Case | Surface | User-visible message | Exit path |
|---|---|---|---|
| No presets configured | 3 (empty state) | "No presets yet. Add one in `~/.stapler-squad/launcher-presets.json`." | None needed — form works normally without presets |
| Malformed JSON in config file | 4 (error state) | Raw parse error surfaced via `load_error`, e.g. "failed to parse launcher presets: invalid character ',' looking for beginning of value" | Fix the file, reopen Omnibar (refetches automatically) |
| Duplicate preset `id` | 4 (error state) | "duplicate preset id \"codex\" (positions 1 and 3)" | Same as above |
| Preset with empty `argv` | 4 (error state) | "preset \"codex\" has no argv" | Same as above |
| Preset references an unavailable program | 6 (soft warning) | "`codex` not found in PATH — check it's installed" | Non-blocking — Create Session stays enabled; user can install the binary or edit the Program field manually |
| `preset:<id>` typed for unknown id | 8 (not found) | "No preset 'preset:doesnotexist'" | Edit/clear the typed input; browse the Presets list instead |
| RPC transport failure (server unreachable) | Not separately designed — falls back to the hook's generic `error` state | Existing generic network-error handling (out of scope to redesign per this feature) | Retry via reopening Omnibar |

General principle carried over from `research/ux.md`: **every failure mode for a hand-edited
config file must be diagnosable from the UI alone** — none of the error states above require
the user to check `~/.stapler-squad/logs/stapler-squad.log`.

---

## 8. Keyboard & Accessibility Reference

| Interaction | Key(s) | Behavior |
|---|---|---|
| Move focus within the Presets listbox | ArrowUp / ArrowDown | Roving `tabIndex`, moves `aria-activedescendant` — matches `RadioGroup`/`AliasPalette` |
| Select the focused preset row | Enter or Space | Fires `onSelect` — same handler as a mouse click |
| Toggle Presets section open/closed | Enter/Space on the collapsible header (it's a `<div onClick>` today in Advanced Options — should be a real `<button>` or have `role="button" tabIndex={0}` + `onKeyDown` added for the Presets header specifically, since Advanced Options's existing header has this same latent gap) | Expands/collapses the section |
| Accept a typed `preset:<id>` shorthand | Tab or Enter | Resolves and prefills, mirroring `AliasDetector`'s Tab/Enter-to-accept convention |
| Back out of an in-progress detection | Escape | Returns to plain input mode, matching `AliasPalette`'s existing Escape handling |
| Submit from anywhere after prefill | Ctrl+Enter / Cmd+Enter | Existing global shortcut (`Omnibar.tsx:863-865`), unmodified, works after preset prefill exactly as it does today for manual entry |

**ARIA roles** (per `.claude/rules/e2e-test-conventions.md` — `data-testid`/ARIA roles only,
no CSS class locators in tests):

| Element | Role | `data-testid` |
|---|---|---|
| Presets list container | `listbox` | — |
| Each preset row | `option`, `aria-selected` | `preset-row` |
| Empty state | `status` | `preset-list-empty` |
| Loading state | `status`, `aria-live="polite"` | `preset-list-loading` |
| Error/load-failure state | `alert`, `aria-live="assertive"` | `preset-config-error` |
| Resolution chip | `status`, `aria-live="polite"` | `preset-resolution-chip` |
| `preset:<id>` not-found chip | `alert`, `aria-live="assertive"` | `preset-not-found` |
| PATH soft warning | plain text, no alert role (non-blocking, not an error) | `preset-program-warning` |

**Color contrast**: reuse the existing warning/error/status token colors already validated for
`AliasPalette` (`vars.*` from the shared theme contract, per `.claude/rules/css-architecture.md`
— no new hex values). The PATH soft-warning text and the error-state text must each
independently meet ≥4.5:1 against their background, same bar as the alias error chip; verify
with the existing token pairs rather than introducing a new color.

---

## 9. UX Acceptance Criteria

Each criterion below is testable by a human clicking through the running app (or, where noted,
by a Playwright/RTL test asserting the same observable state).

### Task completion

1. **Select-and-launch from the Presets list**: a user with at least one preset configured can
   go from an already-open Omnibar to a submitted `CreateSession` call in **≤ 2 interactions**:
   (1) click/Enter on a preset row, (2) Ctrl+Enter or click Create Session. No path requires
   opening Advanced Options first.
2. **Select-and-launch via typed shorthand**: a user who knows a preset's `id` can go from an
   empty Omnibar input to a submitted `CreateSession` call in **≤ 2 interactions**: (1) type
   `preset:<id>` + Tab/Enter, (2) Ctrl+Enter. This must be at least as fast as the listbox path,
   not slower — it exists specifically for power users who already know the id.
3. **Discover presets exist with zero prior knowledge**: a first-time user who opens the
   Omnibar with ≥1 preset configured sees the Presets section already expanded (not requiring
   a click to discover) within the same view as the session-type radio group, in **0 additional
   interactions** beyond opening the Omnibar itself.
4. **Recover from a malformed config file**: a user who breaks their `launcher-presets.json`
   sees the specific parse/validation error (not a generic failure) inside the Omnibar itself,
   and can fix the file and see the corrected preset list appear by simply closing and
   reopening the Omnibar — **no app restart required** (Success Criterion 1's explicit "without
   restarting the server" wording extends to the client-visible experience here).

### Error states — specific message + specific action

5. Malformed JSON → error state shows the raw parse error text (not a generic "something went
   wrong") and implicitly offers "reopen the Omnibar after fixing the file" as the recovery
   action (stated in the error copy itself).
6. Duplicate preset `id` → error state names the colliding `id` and its position(s) in the
   file, and offers the same reopen-to-retry action.
7. Preset with empty `argv` → error state names the specific preset `id`, same recovery action.
8. Preset referencing an unavailable program → inline warning names the specific missing
   program (`` `codex` not found in PATH ``) and offers "check it's installed" as the implied
   action, while explicitly **not** blocking the Create Session button (the action of last
   resort — "launch anyway" — is always available without extra confirmation).
9. `preset:<id>` typed for an unknown id → error state echoes the exact typed id back to the
   user ("No preset 'preset:doesnotexist'") so they can immediately see any typo, and the input
   remains editable in place (no forced dismissal/reset) as the recovery action.

### No dead ends

10. Every error/empty state above (empty list, load error, PATH warning, preset-not-found) has
    a working exit path that does not require leaving the Omnibar: closing/reopening it
    (load error, empty list), editing the input in place (`preset:<id>` not-found), or ignoring
    the warning and proceeding (PATH warning). None of these states disable Create Session or
    trap the user in a modal with no way to back out.
11. Selecting a preset never itself becomes a dead end even when its data is stale (e.g. a
    preset's program was uninstalled since the file was last edited) — the form remains fully
    editable after prefill, so the user can always override any preset-supplied field by hand.

### Accessibility

12. The entire Presets flow — browsing the list, selecting a row, dismissing errors, using the
    `preset:<id>` shorthand — is operable with keyboard only, with no mouse-only affordance
    (verified via the Keyboard Reference table in §8; RTL test: fire `keydown` events only, no
    `.click()`, and assert the same end state as a mouse-driven test).
13. Every interactive element in the Presets section has an accessible name: preset rows via
    `aria-label` (including the "selecting will replace..." clobber warning per §3.3), the
    listbox via `aria-label="Launcher presets"`, error/status regions via their visible text
    content (no `aria-label` needed when visible text already serves as the accessible name).
14. Status vs. alert roles are never swapped: confirmations (`preset-resolution-chip`, empty
    state, loading state) use `role="status" aria-live="polite"`; genuine failures
    (`preset-config-error`, `preset-not-found`) use `role="alert" aria-live="assertive"`. A
    screen-reader user must never have a load failure announced at the same passive priority as
    "no presets configured."
15. All new text (row labels, argv preview line, empty/error/warning copy) meets **≥4.5:1**
    contrast against its background in both light and dark theme, reusing existing validated
    `vars.*` tokens rather than new hex values (per `.claude/rules/css-architecture.md`) — no
    new token introduced by this feature skips the existing contrast-checked palette.
16. No CSS class selectors are required to test any Presets surface — every assertion in this
    document is expressible via `getByRole`/`getByTestId`, per
    `.claude/rules/e2e-test-conventions.md`.

---

## Sources

- `project_plans/launcher-presets/requirements.md` (full read)
- `project_plans/launcher-presets/research/ux.md` (full read — primary input, extended not
  duplicated)
- `project_plans/launcher-presets/implementation/plan.md` (full read — component/file
  structure, ACs, data shapes)
- `web-app/src/components/ui/AliasPalette.tsx` (full read — listbox/row/error/empty shapes)
- `web-app/src/components/sessions/OmnibarCreationPanel.tsx` (partial read — collapsible
  section pattern, Advanced Options/Program field structure, `SessionTypeRadioGroup`)
- `web-app/src/components/sessions/Omnibar.tsx` (partial read — alias resolution chip markup,
  detection-effect wiring region)
