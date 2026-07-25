# UX Design: Alias Feature

**Date**: 2026-06-20
**Status**: Ready for implementation
**Author**: UX review, pre-implementation design artifact

---

## Overview

The alias feature adds a new omnibar input mode triggered by `@`. It integrates into the existing omnibar as a parallel detection path alongside path input, GitHub URLs, and session search. The design must respect the existing two-phase (discovery / creation) mode model while adding alias-specific states.

---

## Surface Inventory

Seven user-facing surfaces are designed:

1. Alias browse palette — `@` alone (discovery sub-state)
2. Alias completion mode — `@myp` partial name, no space
3. Alias invocation — `@myproj working on auth --model haiku`
4. Alias resolution chip — inline badge showing what resolved
5. Alias not-found state — `@nonexistent`
6. Empty state — no aliases configured
7. Config parse error state

---

## Surface 1: Alias Browse Palette (`@` alone)

### Wireframe

```
+------------------------------------------------------------------+
|  @  [ @                                              ]           |
+------------------------------------------------------------------+
|  Aliases                                             [Esc] Close |
+------------------------------------------------------------------+
| WORK                                                             |
|  @ work-infra   Infrastructure monorepo   ~/infra  [aider]      |
|  @ work-fe      Frontend monorepo          ~/web    [claude]     |
|                                                                  |
| TOOLS                                                            |
|  @ quick        --model claude-haiku-4-5-20251001               |
|                                                                  |
| (no group)                                                       |
|  @ myproj       ~/code/myproj              [claude]              |
|                                                                  |
+------------------------------------------------------------------+
|  [↑↓] Navigate   [↵] Select   [Tab] Complete   [Esc] Close      |
+------------------------------------------------------------------+
```

### Layout notes

- Ungrouped aliases render first, above all group sections, with no section header (avoids the "General" label confusion flagged in ALIAS-008)
- Named groups render as uppercase section headers, visually distinct from alias rows
- Each alias row: `@ <name>` (left), `<description>` (center), `<path> [program]` (right)
- Right-column metadata is truncated with ellipsis at viewport width; full value shown in tooltip on hover/focus
- The list is scrollable when aliases exceed viewport height; keyboard focus tracks into scroll

### Interaction flow

| Step | User action | System response |
|------|-------------|-----------------|
| 1 | Types `@` | Omnibar stays in discovery mode; alias palette replaces session-result list |
| 2 | `@` alone, no aliases | Shows empty state (Surface 6) |
| 3 | `@` alone, config parse error | Shows error state (Surface 7) |
| 4 | `@` alone, aliases present | Grouped alias list appears within 100ms |
| 5 | Arrow Down | Focus moves to first alias row; active row highlighted |
| 6 | Arrow Up/Down | Focus navigates through rows including across group boundaries |
| 7 | Enter on focused row | Alias name is inserted into input as `@<name> ` (with trailing space), triggering alias invocation mode (Surface 3) |
| 8 | Tab on focused row | Same as Enter — completes to `@<name> ` |
| 9 | Esc | Clears `@` from input; returns to standard discovery mode |

### ARIA

```
role="listbox" aria-label="Alias palette"
  role="option" aria-label="@work-infra — Infrastructure monorepo, path ~/infra, program aider"
  role="option" ...
```

Group headers use `role="presentation"` with a visible `<li>` styled as a group label (not a focusable option).

---

## Surface 2: Alias Completion Mode (`@myp` — partial name, no space)

### Wireframe

```
+------------------------------------------------------------------+
|  @  [ @myp                                           ]           |
+------------------------------------------------------------------+
|  Matching aliases                                                |
+------------------------------------------------------------------+
|  @ myproj       ~/code/myproj              [claude]    <-- focus |
|  @ myproj-api   ~/code/myproj-api          [claude]              |
+------------------------------------------------------------------+
|  [↑↓] Navigate   [Tab] Complete   [Esc] Clear                   |
+------------------------------------------------------------------+
```

### Behavior

- AliasDetector fires as soon as input matches `^@[\w-]` (even before a space)
- Fuzzy filter: case-insensitive substring/prefix match across alias names, groups, and descriptions
- Group headers are hidden in filtered mode — flat list only (VS Code / Raycast pattern)
- First match is auto-focused but not auto-selected; user must press Tab or Enter to commit
- If exactly one match remains, the resolution chip (Surface 4) appears proactively

### Interaction flow

| Step | User action | System response |
|------|-------------|-----------------|
| 1 | Types `@m` | Alias palette switches to flat filtered list; all aliases with `m` in name/description appear |
| 2 | Types `@my` | List narrows; group headers disappear |
| 3 | Types `@myp` | If 2 matches: both shown, first focused. If 1 match: resolution chip appears |
| 4 | Tab | Completes to `@myproj ` (longest unambiguous prefix if multiple matches; full name if 1 match) |
| 5 | Enter (with focus on row) | Completes to `@myproj ` |
| 6 | Esc | Clears filter text back to `@` (returns to browse palette, not close) |
| 7 | Backspace to `@` | Returns to browse palette |
| 8 | Space typed after partial | If text so far is an exact alias match, resolve it; if not, show not-found state (Surface 5) |

### ARIA

```
role="combobox" aria-autocomplete="list" aria-controls="alias-listbox"
  ...
role="listbox" id="alias-listbox" aria-label="Alias matches"
  role="option" aria-selected="true"  (first item)
  role="option" aria-selected="false"
```

---

## Surface 3: Alias Invocation (`@myproj working on auth --model haiku`)

### Wireframe

```
+------------------------------------------------------------------+
|  @  [ @myproj working on auth --model haiku           ]         |
+------------------------------------------------------------------+
|  +--[myproj]----------------------------------+                  |
|  |  Path:    ~/code/myproj                   |                  |
|  |  Program: claude                          |                  |
|  |  Label:   "working on auth"               |                  |
|  |  Flags:   --model haiku  (appended)       |                  |
|  +-------------------------------------------+                  |
|                                                                  |
|   Session name: [ working on auth             ]                  |
|   Branch:       [ (auto from label)            ]                 |
|   Program:      [claude  v]                                      |
|   Type:         [New Worktree v]                                 |
|                                                                  |
|   [Cancel]                           [⌘↵ Create Session]        |
+------------------------------------------------------------------+
|  [Esc] Back   [⌘↵] Create                                       |
+------------------------------------------------------------------+
```

### Grammar parsing (visual representation)

```
@myproj : feature/auth   working on auth   --model haiku
 ^alias   ^:branch        ^label text       ^extra flags
```

- Text before first `--` token (after alias/branch) is the session label
- Text starting at first `--` is appended to alias's static `cli_flags`
- `:branch` suffix on the alias name overrides the worktree branch

### Interaction flow

| Step | User action | System response |
|------|-------------|-----------------|
| 1 | Space typed after resolved alias name | Omnibar transitions to creation mode; alias resolution chip appears (Surface 4) |
| 2 | Label text typed | Session name field updates live (derived from label) |
| 3 | `:branch` suffix typed | Branch field populates in creation form |
| 4 | `--flag` typed | Flags are shown in the resolution chip as "appended flags" |
| 5 | Enter / Cmd+Enter | Session created with merged config; omnibar closes |
| 6 | Esc | Returns to discovery mode without creating |

### ARIA

The creation panel form fields each have associated `<label>` elements. Alias-derived values appear with `aria-describedby` pointing to a description reading "Pre-filled from alias myproj".

---

## Surface 4: Alias Resolution Chip

### Wireframe (inline in input area)

```
+------------------------------------------------------------------+
|  @  [ @myproj working on auth --model haiku           ]         |
|     +-- Alias resolved --------------------------------+         |
|     |  @myproj  ~/code/myproj  [claude]               |         |
|     |  Label: "working on auth"                        |         |
|     |  Extra flags: --model haiku                      |         |
|     +---------------------------------------------------+        |
+------------------------------------------------------------------+
```

### Chip appears when

- The exact alias name is matched (full match, with space typed after it)
- OR when exactly one fuzzy match remains in completion mode (proactive preview)

### Chip content

- Alias name (bold)
- Resolved path (if any)
- Program (if any)
- Parsed label (if any text between alias and first `--`)
- Extra flags being appended (if any `--` tokens)
- Profile inherited (if `profile` field is set in alias config)

### States

| State | Display |
|-------|---------|
| Resolved clean | Green left border, "Alias resolved" label |
| Resolved with extra flags | Green left border, extra flags section shown |
| Not yet resolved (partial match in autocomplete) | Muted/gray, "Matches @myproj..." |

### ARIA

```html
<div role="status" aria-live="polite" aria-label="Alias resolution: myproj, path ~/code/myproj, program claude, label working on auth">
```

---

## Surface 5: Alias Not-Found State

### Wireframe

```
+------------------------------------------------------------------+
|  @  [ @nonexistnt                                     ]         |
+------------------------------------------------------------------+
|  +-- Not found -------------------------------------------+     |
|  |  No alias '@nonexistnt'                                 |     |
|  |  Did you mean: @nonexistent ?                           |     |
|  |                                                         |     |
|  |  [Add alias to config.json]   [Clear]                   |     |
|  +----------------------------------------------------------+     |
+------------------------------------------------------------------+
|  [Esc] Clear                                                     |
+------------------------------------------------------------------+
```

### Behavior

- AliasDetector claims all `^@[\w-]` input; it does NOT fall through to SessionSearchDetector (ALIAS-001)
- Fuzzy nearest-match suggestion: Levenshtein distance over all known alias names; suggest if distance <= 2
- If zero aliases are configured and user typed `@something`, show the empty-state variant with "no aliases configured" message
- "Add alias to config.json" opens config.json in the system editor via a shell command (v1: copy path to clipboard with a toast; v2: open editor)
- "Clear" clears the `@` prefix text and returns to discovery mode

### Interaction flow

| Step | User action | System response |
|------|-------------|-----------------|
| 1 | Types `@nnn` (no match) | Not-found state appears; session search is NOT shown |
| 2 | Nearest match exists | "Did you mean: @<name>?" link shown |
| 3 | Click "Did you mean" | Input replaced with `@<name> ` |
| 4 | Esc | Clears `@nnn` from input, returns to empty discovery mode |
| 5 | Backspace | Returns to partial filter mode |
| 6 | "Add alias to config.json" clicked | Toast: "Path copied: ~/.stapler-squad/config.json" (v1) |

### ARIA

```html
<div role="alert" aria-live="assertive">
  No alias '@nonexistnt' found.
  <a href="#" aria-label="Use closest match: @nonexistent">Did you mean @nonexistent?</a>
</div>
```

---

## Surface 6: Empty State (No Aliases Configured)

### Wireframe

```
+------------------------------------------------------------------+
|  @  [ @                                              ]           |
+------------------------------------------------------------------+
|  Aliases                                                         |
+------------------------------------------------------------------+
|                                                                  |
|                  No aliases yet                                  |
|                                                                  |
|     Add them in config.json to launch sessions faster.          |
|                                                                  |
|     Example:                                                     |
|     {                                                            |
|       "name": "myproj",                                          |
|       "path": "~/code/myproj",                                   |
|       "program": "claude"                                        |
|     }                                                            |
|                                                                  |
|     [Copy config.json path]                                      |
|                                                                  |
+------------------------------------------------------------------+
|  [Esc] Close                                                     |
+------------------------------------------------------------------+
```

### Copy content

"No aliases yet — add them in config.json to launch sessions faster."

Below the copy, a small code snippet shows the minimal alias definition with syntax highlighting (JSON).

### Action

"Copy config.json path" copies `~/.stapler-squad/config.json` (resolved) to clipboard. A toast confirms: "Path copied — open config.json in your editor to add aliases."

### ARIA

```html
<div role="status" aria-label="No aliases configured. Add aliases in config.json to get started.">
```

---

## Surface 7: Config Parse Error State

### Wireframe

```
+------------------------------------------------------------------+
|  @  [ @                                              ]           |
+------------------------------------------------------------------+
|  Aliases                                                         |
+------------------------------------------------------------------+
|                                                                  |
|   [!] Alias config failed to load                                |
|                                                                  |
|       config.json has a syntax error. Aliases are               |
|       unavailable until the error is fixed.                      |
|                                                                  |
|       Error detail:                                              |
|       Line 42: unexpected token '}'                              |
|                                                                  |
|       [Copy config.json path]   [Dismiss]                        |
|                                                                  |
+------------------------------------------------------------------+
|  [Esc] Close                                                     |
+------------------------------------------------------------------+
```

### Behavior

- Config parse error is detected at server startup and surfaced via the existing config API
- The alias palette shows this state instead of the alias list
- Error message shows the line number and parser message if available
- "Dismiss" hides the error for the session but aliases remain unavailable
- This state does not block other omnibar functions (path input, GitHub URLs, session search still work)

### ARIA

```html
<div role="alert" aria-live="assertive" aria-label="Error: alias config failed to load. config.json syntax error on line 42.">
```

---

## Interaction Flows

### Flow A: Expert user (knows alias name)

```
User types:  @ m y p r o j   w o r k i n g   o n   a u t h
              |               |
              v               v
         Completion     Alias resolved;
         list appears   creation mode
         (fuzzy filter) + chip shown

                                    User presses Cmd+Enter
                                    -> Session created
                                    -> Omnibar closes
```

Total steps from opening omnibar: type `@myproj ` (8 chars) + Cmd+Enter = 1 gesture

### Flow B: Discovery user (browsing aliases)

```
User types @
  -> Browse palette opens (grouped sections)

User presses Arrow Down
  -> First alias focused

User presses Arrow Down again
  -> Second alias focused

User presses Enter
  -> Input fills to "@work-infra "
  -> Creation panel opens with alias data pre-populated

User presses Cmd+Enter
  -> Session created
```

Total steps: `@` + 2x Arrow Down + Enter + Cmd+Enter = 5 key presses

### Flow C: Typo recovery

```
User types @myrojc (typo)
  -> Not-found state appears
  -> Suggestion: "Did you mean @myproj?"

User clicks suggestion / presses Enter on it
  -> Input replaced with "@myproj "
  -> Alias resolved, creation panel opens

User presses Cmd+Enter
  -> Session created
```

### Flow D: Alias with branch and extra flags

```
User types: @myproj:feature/auth working on auth --model haiku

Parsed as:
  alias name: myproj
  branch:     feature/auth
  label:      "working on auth"
  extra flags: --model haiku

Resolution chip shows all four components.
Creation form: session name = "working on auth", branch = "feature/auth"
On submit: merged cli_flags = "<alias.cli_flags> --model haiku"
```

---

## Omnibar Placeholder Update

Per ALIAS-005, the discovery mode placeholder must be updated to mention `@alias`:

**Current**: "Jump to session or search repos..."

**Updated**: "Jump to session, @alias, or search repos..."

This makes alias discovery visible to users who have never typed `@` before (recognition over recall).

---

## UX Acceptance Criteria

### Invocation efficiency

- **AC-01**: User can invoke a saved alias in <= 2 keystrokes after `@` for aliases with names of 3 characters or fewer (e.g., `@fe` + Enter = 3 total after `@`)
- **AC-02**: Alias browse palette appears within 100ms of typing `@` alone
- **AC-03**: User can launch a session from an alias in <= 5 total key presses from discovery mode (browse flow B above)
- **AC-04**: Cmd+Enter submits the alias-prefilled creation form without requiring mouse interaction

### Alias resolution feedback

- **AC-05**: Alias resolution chip appears as the user types — not only after pressing Enter — when a single alias matches
- **AC-06**: The chip displays at minimum: alias name, resolved path (if any), program (if any)
- **AC-07**: Extra flags appended at invocation time are shown in the chip as "appended flags" before submission
- **AC-08**: The chip updates live when the user edits label text or extra flags inline

### Error handling — no dead ends

- **AC-09**: Typing `@nonexistent` shows the not-found state, not session search results (ALIAS-001)
- **AC-10**: The not-found state offers a fuzzy nearest-match suggestion when Levenshtein distance <= 2
- **AC-11**: The not-found state provides "Clear" and "Esc" exits; user is never stuck
- **AC-12**: Config parse error state shows the specific error message (line/token) when available
- **AC-13**: Config parse error state provides a path-copy action and a Dismiss action
- **AC-14**: Config parse error does not block other omnibar modes (path input, GitHub URLs, session search remain functional)

### Empty state

- **AC-15**: Typing `@` with no aliases configured shows copy "No aliases yet — add them in config.json" plus a code example
- **AC-16**: The empty state provides a "Copy config.json path" action so users can immediately open the file
- **AC-17**: The empty state is not a dead end — Esc or clicking outside closes the palette

### Discovery and filtering

- **AC-18**: Aliases appear in grouped sections (group header + alias rows) when browsing (`@` alone)
- **AC-19**: Ungrouped aliases render above all groups with no section header (not labeled "General")
- **AC-20**: Group headers disappear when filtering is active (`@` + any character)
- **AC-21**: Filtering is fuzzy and case-insensitive, matching across alias name, group, and description fields
- **AC-22**: Tab key completes to the longest unambiguous alias prefix when multiple matches exist

### Grammar and parsing

- **AC-23**: All text between alias name/branch and the first `--` token is interpreted as the session label
- **AC-24**: `:branch` suffix on the alias name overrides the worktree branch with no additional UI step
- **AC-25**: `--extra-flags` typed after the alias name are appended to (not replace) the alias's static `cli_flags`
- **AC-26**: Case-insensitive alias matching: `@MyProj` resolves the same as `@myproj`

### Keyboard navigation

- **AC-27**: All alias palette interactions are navigable by keyboard alone: arrow keys to navigate rows, Enter to select, Tab to complete, Esc to exit or clear
- **AC-28**: First Esc when in alias not-found or filtered state clears the filter (back to `@`), not close
- **AC-29**: First Esc in browse palette (`@` alone) clears the `@` and returns to discovery mode, not close
- **AC-30**: Second Esc (from discovery mode) closes the omnibar entirely

### Accessibility — screen reader

- **AC-31**: Alias palette container has `role="listbox"` and `aria-label="Alias palette"`
- **AC-32**: Each alias row has `role="option"` with an `aria-label` that reads: `@<name>, <description>, path <path>, program <program>`
- **AC-33**: Resolution chip uses `role="status"` with `aria-live="polite"` and announces the resolved alias components
- **AC-34**: Not-found state uses `role="alert"` with `aria-live="assertive"` so screen readers announce the error immediately
- **AC-35**: Config parse error state uses `role="alert"` with `aria-live="assertive"`
- **AC-36**: All action buttons (Copy path, Dismiss, Did you mean) have descriptive `aria-label` values

### Accessibility — visual

- **AC-37**: Color contrast for alias name, description, and metadata text meets WCAG AA minimum (4.5:1 for normal text, 3:1 for large/bold text)
- **AC-38**: Active/focused alias row uses a focus indicator that is visible in both light and dark themes and meets 3:1 contrast ratio against adjacent colors
- **AC-39**: Resolution chip green left-border affordance is supplemented by a text label ("Alias resolved") — color alone does not convey state
- **AC-40**: Error states use icon plus text (not color alone) to communicate error status

### Omnibar placeholder

- **AC-41**: Discovery mode placeholder reads "Jump to session, @alias, or search repos..." (or equivalent that includes `@alias`)

---

## Component Architecture Notes

These notes are informational for the implementation phase — they describe where new elements slot into the existing component tree.

### New states in existing flow

The alias feature introduces a new detection result type (`InputType.AliasResolved`, `InputType.AliasNotFound`) that slots alongside existing `InputType` values. The omnibar's existing `detection?.type` switch handles routing to the correct surface.

### New sub-components

| Component | Location | Purpose |
|-----------|----------|---------|
| `AliasPalette` | `components/ui/` | Grouped/filtered alias list (Surfaces 1, 2) |
| `AliasResolutionChip` | `components/ui/` | Inline resolved alias display (Surface 4) |
| `AliasNotFound` | `components/ui/` | Not-found error with fuzzy suggestion (Surface 5) |
| `AliasEmptyState` | inside `AliasPalette` | Empty state copy and action (Surface 6) |
| `AliasConfigError` | inside `AliasPalette` | Parse error state (Surface 7) |

### Integration point in `Omnibar.tsx`

The `isAtDropdownVisible` boolean currently gates the `AtCommandDropdown`. Alias browse mode requires a parallel gate `isAliasPaletteVisible` that activates when input starts with `@` AND the `AliasDetector` is the active detector. This replaces `AtCommandDropdown` for the `@` trigger once the alias feature is live (or the two are unified if `@slug` workflow commands and aliases coexist).

### Detection priority

`AliasDetector` priority = 36 (between `NewSessionDetector` at 35 and `GitHubShorthandDetector` at 40). It must return `AliasNotFound` — never `null` — for `^@[\w-]` inputs, preventing fall-through to `SessionSearchDetector`.

---

## Open Questions (Deferred to Implementation)

1. **@slug coexistence** — ~~DEFERRED~~ **RESOLVED by ADR-020** (2026-06-20): WorkflowDetector is registered at priority 25; AliasDetector is registered at priority 36. Lower number = higher precedence. Workflows win when a slug matches both a workflow and an alias name. This is intentional — workflows are dynamic scheduled actions; aliases are simple session presets. An identically-named workflow signals intentional override. See `docs/adr/020-alias-at-trigger-character.md`. No UX regression for existing workflow users.

2. **"Copy config.json path" vs. "Open in editor"**: v1 spec says copy to clipboard. This should be validated with users; opening directly in `$EDITOR` may be higher value. Leave as clipboard for v1, plan editor integration for v2.

3. **Proactive chip threshold**: Chip appears when "exactly one match remains." Edge case: if user types exact alias name character-by-character and a longer alias also matches (e.g., `@foo` and `@foobar` both exist), chip should not show until `:` or space is typed to signal intent to use `@foo`. Implementation should use exact-match check, not single-remaining-match check.
