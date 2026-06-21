# UX Design: AliasesManager

**Date**: 2026-06-21
**Feature**: Settings > General > Aliases CRUD panel
**Surfaces designed**: 8
**UX acceptance criteria**: 32

---

## Surface Inventory

1. Alias list — empty state
2. Alias list — with items
3. Add form — primary fields (form collapsed / advanced hidden)
4. Add form — advanced section expanded
5. Edit form
6. Delete confirmation
7. Error states (name conflict, invalid name, save failure, load failure)
8. Success banner

---

## Surface 1: Alias List — Empty State

### Wireframe

```
┌──────────────────────────────────────────────────────────────┐
│  Aliases                              [+ New Alias]          │
├──────────────────────────────────────────────────────────────┤
│                                                              │
│   No aliases configured.                                     │
│   Add one to launch sessions faster using @name in the       │
│   omnibar.                                                   │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

### Interaction Flow

| User action | System response |
|---|---|
| Page loads (Settings > General tab active) | Component mounts, calls `listAliases` RPC, sets `loading = true` |
| `listAliases` resolves with empty array | `loading = false`, `aliases = []`, empty state message renders |
| User clicks "New Alias" button | `showForm = true`, `editingName = null`, form resets to `emptyForm`, focus moves to Name input |

### Edge Cases

- If `listAliases` fails: show error banner above empty state text ("Failed to load aliases: …"); empty state body still visible below the banner; "New Alias" button remains enabled so the user can still create aliases.
- Loading state: render `<h2>Aliases</h2>` + "Loading…" text in place of the list; "New Alias" button is hidden until load completes to prevent creation before state is known.

---

## Surface 2: Alias List — With Items

### Wireframe

```
┌──────────────────────────────────────────────────────────────┐
│  Aliases                              [+ New Alias]          │
├──────────────────────────────────────────────────────────────┤
│ ┌──────────────────────────────────────────────────────────┐ │
│ │ myproj                                   [Edit] [Delete] │ │
│ │ My main project                                          │ │
│ │ Group: work  ~/code/myproject  [claude]                  │ │
│ └──────────────────────────────────────────────────────────┘ │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │ infra-ops                                [Edit] [Delete] │ │
│ │                                                          │ │
│ │ ~/infra  [aider]                                         │ │
│ └──────────────────────────────────────────────────────────┘ │
│ ┌──────────────────────────────────────────────────────────┐ │
│ │ quick-note                               [Edit] [Delete] │ │
│ │ Scratchpad session                                       │ │
│ │                            (no path, no program)         │ │
│ └──────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
```

### Interaction Flow

| User action | System response |
|---|---|
| `listAliases` resolves with N aliases | List renders N rows; each row shows name (bold), description, group chip, path chip, program chip (only for fields that are set) |
| User clicks "Edit" on a row | `handleEdit(alias)` fires: form populates with all alias fields, `editingName` set to the alias name, `showForm = true`, focus moves to first editable field (Description, since Name is disabled) |
| User clicks "Delete" on a row | Row's Delete button is replaced by "Confirm delete?" button (auto-reverts after 3s if not clicked) |

### Row Anatomy

```
aliasName     — bold, primary color, always shown
aliasDesc     — muted, shown only if description is non-empty
aliasMeta     — pill/chip style, one per: "Group: {group}", "{path}", "[{program}]"
aliasActions  — right-aligned: "Edit" (secondary), "Delete" (danger)
```

### Edge Cases

- Alias with no description, no path, no program: only the name row renders; the metadata line is absent (no empty chips).
- Long alias names: truncate with ellipsis at row boundary.
- Long paths: truncate with ellipsis at 40 chars; full path available on hover via `title` attribute.

---

## Surface 3: Add Form — Primary Fields (Advanced Hidden)

### Wireframe

```
┌──────────────────────────────────────────────────────────────┐
│  Aliases                              [+ New Alias]          │
├──────────────────────────────────────────────────────────────┤
│  myproj row ...                                              │
├──────────────────────────────────────────────────────────────┤
│ ┌── New Alias ───────────────────────────────────────────┐   │
│ │                                                        │   │
│ │  Name *                                                │   │
│ │  [______________________________]  ← focus on open     │   │
│ │  Available in the omnibar as @name                     │   │
│ │                                                        │   │
│ │  Description                                           │   │
│ │  [______________________________]                      │   │
│ │                                                        │   │
│ │  Group                                                 │   │
│ │  [______________________________]                      │   │
│ │  Groups aliases in the @ palette. Leave blank for      │   │
│ │  ungrouped.                                            │   │
│ │                                                        │   │
│ │  Path                                                  │   │
│ │  [______________________________]                      │   │
│ │  Supports ~ expansion. Optional.                       │   │
│ │                                                        │   │
│ │  Profile                                               │   │
│ │  [______________________________]                      │   │
│ │                                                        │   │
│ │  Program                                               │   │
│ │  [Default ▼]                                           │   │
│ │                                                        │   │
│ │  [✓] Auto-yes                                          │   │
│ │                                                        │   │
│ │  Tags                                                  │   │
│ │  [backend] [x]  [frontend] [x]                         │   │
│ │  [Add a tag...      ] [Add]                            │   │
│ │                                                        │   │
│ │  ── Preview in omnibar ─────────────────────────────   │   │
│ │  @myproj  ~/code/myproject  [claude]                   │   │
│ │                                                        │   │
│ │  [ ] Advanced options                                  │   │
│ │                                                        │   │
│ │  [Save]  [Cancel]                                      │   │
│ └────────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────────┘
```

### Interaction Flow

| User action | System response |
|---|---|
| "New Alias" clicked | Form card slides into view below the list; focus lands on Name input |
| User types in Name field | `previewHint` text updates live: "Available in the omnibar as @{name}" |
| User types in Name, Path, or Program | Omnibar preview block updates live: shows `@{name}  {path}  [{program}]`; empty fields are omitted (no blank chips) |
| User presses Enter in tag input | Tag chip appended to tag list; tag input cleared; form submit NOT triggered |
| User clicks "Add" in tag row | Same as Enter: tag appended, input cleared |
| Tag chip [x] clicked | Tag removed from list |
| User clicks "Save" | Client-side validation runs (see Surface 7); if valid, `upsertAlias` RPC called, button shows "Saving…" and is disabled |
| RPC succeeds | Success banner appears ("Alias \"@myproj\" saved."), form hides, list refreshes |
| User clicks "Cancel" | Form closes, all state resets, no RPC call |

### Field Order

1. Name * (required, `aria-required="true"`)
2. Description
3. Group (with hint)
4. Path (with hint)
5. Profile (text input — free-form reference to a profile name)
6. Program (select from PROGRAMS constant)
7. Auto-yes (checkbox)
8. Tags (chip editor)
9. Preview in omnibar (read-only, derived from form state)
10. Advanced options (checkbox toggle)
11. Form actions: Save / Cancel

---

## Surface 4: Add Form — Advanced Section Expanded

### Wireframe

```
│  [ ] Advanced options                                        │
│  ─── (expanded) ───────────────────────────────────────────  │
│                                                              │
│  Environment Variables                                       │
│  Use ${VAR} to reference shell variables.                    │
│  ┌──────────────────────────────────────────────┐           │
│  │  [KEY          ] [value              ] [Remove]│           │
│  │  [DATABASE_URL ] [postgres://localhost] [Remove]│          │
│  └──────────────────────────────────────────────┘           │
│  [+ Add Variable]                                            │
│                                                              │
│  CLI Flags                                                   │
│  [e.g. --no-ansi                    ]                        │
```

### Interaction Flow

| User action | System response |
|---|---|
| User checks "Advanced options" checkbox | `form.showAdvanced = true`; env vars editor and CLI Flags input animate into view below the checkbox |
| "Add Variable" clicked | New `{ key: "", value: "" }` row appended to `envVars` array; focus moves to the new KEY input |
| User types in KEY or value input | `handleEnvVarChange(index, field, value)` updates the specific row |
| "Remove" clicked on a row | That row removed from `envVars` array; if only row, array becomes `[]` |
| User unchecks "Advanced options" | `form.showAdvanced = false`; env vars and CLI flags collapse but state is preserved (re-checking re-shows existing rows) |

### Edge Cases

- Empty KEY with a value: row is silently skipped when building `envVarsMap` on save (blank keys are not written to config).
- Env var row with both KEY and value empty: also skipped on save.
- No env var rows when section first expanded: shows empty table with only the "Add Variable" button.

---

## Surface 5: Edit Form

### Wireframe

```
┌── Edit Alias: myproj ──────────────────────────────────────┐
│                                                             │
│  Name *                                                     │
│  [myproj                    ]  ← DISABLED (grey, no cursor) │
│  Available in the omnibar as @myproj                        │
│                                                             │
│  Description                                                │
│  [My main project           ]  ← FOCUSED                   │
│                                                             │
│  Group                                                      │
│  [work                      ]                               │
│  Groups aliases in the @ palette. Leave blank for ungrouped.│
│                                                             │
│  Path                                                       │
│  [~/code/myproject          ]                               │
│  Supports ~ expansion. Optional.                            │
│                                                             │
│  Profile                                                    │
│  [                          ]                               │
│                                                             │
│  Program                                                    │
│  [claude ▼]                                                 │
│                                                             │
│  [ ] Auto-yes                                               │
│                                                             │
│  Tags                                                       │
│  [backend] [x]                                              │
│  [Add a tag...   ] [Add]                                    │
│                                                             │
│  ── Preview in omnibar ───────────────────────────────────  │
│  @myproj  ~/code/myproject  [claude]                        │
│                                                             │
│  [✓] Advanced options  ← auto-expanded if alias has env vars│
│  ── (expanded if env_vars or cli_flags exist on the alias) ─│
│  ...                                                        │
│                                                             │
│  [Save]  [Cancel]                                           │
└─────────────────────────────────────────────────────────────┘
```

### Interaction Flow

| User action | System response |
|---|---|
| "Edit" clicked on a list row | `handleEdit(alias)` populates all form fields; Name input set to `disabled`; focus moves to Description (first editable field) |
| "Advanced options" auto-state | `form.showAdvanced` initializes to `true` if the alias being edited has any `envVars` or `cliFlags`; otherwise starts `false` |
| User edits any field | Preview block updates live |
| User clicks "Save" | Client-side validation runs (name conflict check skipped in edit mode); `upsertAlias` RPC called; on success form closes and list refreshes |

### Key Edit-Mode Differences from Add Form

- Form title: "Edit Alias: {name}" instead of "New Alias"
- Name input: `disabled={true}`, visually greyed, cursor non-interactive
- Focus target on open: Description field (not Name, which is locked)
- Name conflict check: skipped entirely (editing an existing alias by its immutable key)
- "Advanced options" auto-expands when the alias has `envVars` or `cliFlags`

---

## Surface 6: Delete Confirmation

### Wireframe

```
┌──────────────────────────────────────────────────────────────┐
│ ┌──────────────────────────────────────────────────────────┐ │
│ │ myproj                        [Edit] [Confirm delete?]   │ │  ← after first click
│ │ My main project                                          │ │
│ │ Group: work  ~/code/myproject  [claude]                  │ │
│ └──────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
```

The "Delete" button is replaced in-row by a "Confirm delete?" button when first clicked.
If the user does not click "Confirm delete?" within 3 seconds, the button reverts to "Delete".

### Interaction Flow

| User action | System response |
|---|---|
| User clicks "Delete" on a list row | Row's Delete button is replaced by "Confirm delete?" button (auto-reverts after 3s if not clicked) |
| 3 seconds elapse without confirmation | `pendingDeleteName` resets to `null`; "Delete" button reappears; no RPC called; list unchanged |
| User clicks "Confirm delete?" | `deleteAlias({ name: "myproj" })` RPC called; "Confirm delete?" button removed while request is in flight |
| RPC succeeds | Success banner: "Alias \"@myproj\" deleted." (3 s auto-dismiss); list refreshes; deleted row is gone |
| RPC fails | Top-level error banner: "Failed to delete alias: {error message}"; list not refreshed; row still visible |

### Notes

The inline two-click confirmation pattern is used instead of `window.confirm()`. `window.confirm()` is untestable in jsdom (used by Jest/RTL), inaccessible to screen readers in some configurations, and blocked by some browser policies. The inline pattern is fully testable via RTL, keyboard-accessible, and consistent with the rest of the React component tree.

---

## Surface 7: Error States

### 7a. Inline Error — Name Empty

```
  Name *
  [                    ]  ← red border
  Name is required.       ← fieldError span, red text, below input
```

**Trigger**: User clicks "Save" with blank Name field.
**Exit path**: User types a name; inline error clears once the field is non-empty (cleared on next save attempt after fix).

---

### 7b. Inline Error — Name Format Invalid

```
  Name *
  [my project           ]  ← red border (space in name)
  Name may only contain letters, digits, hyphens, and underscores.
```

**Trigger**: User clicks "Save" with a name containing spaces, slashes, or other disallowed characters.
**Exit path**: User corrects the name; inline error clears on next valid save attempt.
**Regex**: `^[\w-]+$`

---

### 7c. Inline Error — Name Conflict (Create Mode Only)

```
  Name *
  [myproj               ]  ← red border
  An alias named "@myproj" already exists. Edit it instead.
```

**Trigger**: User clicks "Save" (in New Alias form, not Edit) with a name that matches an existing alias (case-insensitive).
**Exit path**: User changes the name to something unique, OR clicks "Cancel" then "Edit" on the existing alias row.

---

### 7d. Top-Level Error — Save Failure (Network / Server)

```
┌──────────────────────────────────────────────────────────────┐
│  Aliases                              [+ New Alias]          │
├──────────────────────────────────────────────────────────────┤
│ ╔══════════════════════════════════════════════════════════╗  │
│ ║ Failed to save alias: connect: internal — disk full      ║  │
│ ╚══════════════════════════════════════════════════════════╝  │
│                                                              │
│ ┌── New Alias ─────────────────────────────────────────┐    │
│ │  ... (form remains open with all data intact) ...    │    │
│ └──────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────┘
```

**Role**: `role="alert"` on the error div (announces to screen readers immediately).
**Trigger**: `upsertAlias` or `deleteAlias` RPC throws.
**Exit path**: Error banner persists until the user retries a save (clears `error` at start of next save attempt) or dismisses by clicking "Cancel" (which clears form and resets state).
**Form state**: Form remains open with all user input intact so the user can retry without re-entering data.

---

### 7e. Top-Level Error — Load Failure

```
╔════════════════════════════════════════════════════════╗
║ Failed to load aliases: connect: unavailable           ║
╚════════════════════════════════════════════════════════╝
```

**Trigger**: `listAliases` RPC throws on component mount.
**Exit path**: User can refresh the page. The "New Alias" button remains available — if the user creates an alias and saves successfully, `loadAliases` is re-called on success and may recover.

---

### 7f. Top-Level Error — Delete Failure

```
╔════════════════════════════════════════════════════════╗
║ Failed to delete alias: connect: not_found             ║
╚════════════════════════════════════════════════════════╝
```

**Trigger**: `deleteAlias` RPC throws (e.g., alias removed by another process between page load and delete click).
**Exit path**: Error banner persists; list is re-fetched on next successful operation or page reload. The row that failed to delete remains visible.

---

## Surface 8: Success Banner

### Wireframe

```
╔════════════════════════════════════════════════════════╗
║ Alias "@myproj" saved.                                 ║
╚════════════════════════════════════════════════════════╝
```

**Variants**:
- Save: `Alias "@{name}" saved.`
- Delete: `Alias "@{name}" deleted.`

**Behavior**: Auto-dismisses after 3 seconds (`setTimeout(() => setSuccess(null), 3000)`). Role: `role="status"` (non-blocking announcement to screen readers).

---

## UX Acceptance Criteria

### Task Completion

**AC-01**: A user can create a new alias (name + path only) in 4 steps: click "New Alias", type name, type path, click "Save". The alias appears in the list immediately after save.

**AC-02**: A user can edit an alias description in 3 steps: click "Edit" on the row, clear and retype the Description field, click "Save". The updated description appears in the list row after save.

**AC-03**: A user can delete an alias in ≤2 clicks: click "Delete" on a row, then click "Confirm delete?". The row is removed from the list. No browser dialog is involved.

**AC-04**: A user can add an environment variable in ≤ 5 steps: open/edit form, check "Advanced options", click "Add Variable", type KEY and value, click "Save".

**AC-05**: A user can add a tag in ≤ 3 steps from the open form: type tag in the tag input, press Enter (or click "Add"), confirm the chip appears. No Save required for the chip to appear in the form.

**AC-06**: A user can cancel the form at any point by clicking "Cancel" without any data being persisted. The list is unchanged after cancel.

### Empty State

**AC-07**: When no aliases are configured, the panel shows a message containing "No aliases configured" and an explanation referencing the `@name` omnibar flow. The "New Alias" button is visible and functional.

**AC-08**: When aliases exist, the empty-state message is not visible.

### Error States — Inline Validation

**AC-09**: Clicking "Save" with an empty Name field does not call the RPC. The Name input has a red border and the text "Name is required." appears below it. The form remains open.

**AC-10**: Clicking "Save" with a Name containing a space (e.g., "my project") does not call the RPC. The text "Name may only contain letters, digits, hyphens, and underscores." appears below the Name input.

**AC-11**: Clicking "Save" in New Alias mode with a name that exactly or case-insensitively matches an existing alias does not call the RPC. The text "An alias named \"@{name}\" already exists. Edit it instead." appears below the Name input.

**AC-12**: In Edit mode, saving with the same name as the alias being edited (the immutable key) does not trigger a name conflict error. The save proceeds normally.

**AC-13**: After a validation error is shown, the user can correct the field and click "Save" again. The error clears and the save proceeds if the corrected value is valid.

### Error States — Network / Server

**AC-14**: When save fails (server error), an error banner appears above the form with text starting "Failed to save alias:". The form remains open with all fields intact so the user can retry.

**AC-15**: When delete fails (server error), an error banner appears with text starting "Failed to delete alias:". The list is not modified; the targeted row is still visible.

**AC-16**: When load fails (server error), an error banner appears with text starting "Failed to load aliases:". The "New Alias" button is still accessible.

**AC-17**: Every error state has an exit path: inline errors clear on the next valid save; the top-level error banner clears at the start of the next save attempt or when "Cancel" is clicked.

### Success States

**AC-18**: After a successful save, a green success banner appears with the text `Alias "@{name}" saved.` The banner auto-dismisses after approximately 3 seconds without user action.

**AC-19**: After a successful delete, a green success banner appears with the text `Alias "@{name}" deleted.` The banner auto-dismisses after approximately 3 seconds.

### Form Behavior

**AC-20**: When "New Alias" is clicked, the Name input receives focus automatically (keyboard users do not need to tab to it).

**AC-21**: In Edit mode, focus moves to the Description field (first editable field) when the form opens, because the Name field is disabled.

**AC-22**: The Name field is enabled and editable in New Alias mode. The Name field is disabled (grey, no cursor interaction) in Edit mode.

**AC-23**: The form title reads "New Alias" in create mode and "Edit Alias: {name}" in edit mode.

**AC-24**: The "Advanced options" section is collapsed by default for new aliases. When editing an alias that has env vars or CLI flags, the "Advanced options" section is expanded automatically.

**AC-25**: Checking "Advanced options" reveals the Environment Variables editor and CLI Flags input. Unchecking collapses them but preserves the entered values (re-checking shows the same rows).

**AC-26**: The preview block below the primary fields updates in real-time as the user types in the Name, Path, or Program fields. If Name is empty, the preview shows "@name" as a placeholder.

### Omnibar Preview

**AC-27**: The live preview hint below the Name field reads "Available in the omnibar as @{name}" and updates as the user types. If the name field is empty, the hint shows "Available in the omnibar as @name".

**AC-28**: The omnibar preview block mirrors the format of the AliasPalette: alias name, path, and [program] badge. Fields that are empty are omitted from the preview (no blank chips).

### Accessibility

**AC-29**: All interactive elements in the AliasesManager are reachable and activatable via keyboard alone (Tab to navigate, Enter/Space to activate buttons and checkboxes, Enter in tag input to add tag).

**AC-30**: The form card is wrapped in a `<section>` element with `aria-labelledby` pointing to the form title (`<h3 id="alias-form-title">`). Screen readers announce the section heading when focus enters.

**AC-31**: Error banners use `role="alert"` so screen readers announce them immediately when they appear. Success banners use `role="status"` for a polite (non-interrupting) announcement.

**AC-32**: Tag remove buttons have `aria-label="Remove tag {tagName}"`. Environment variable "Remove" buttons have `aria-label="Remove environment variable {key}"` (or the index if key is empty). Foreground text on all colored elements meets WCAG AA color contrast ratio (≥ 4.5:1).

---

## Complete User Flow Diagrams

### Create Alias (Happy Path)

```
User clicks "New Alias"
        │
        ▼
Form opens, focus → Name input
        │
        ▼
User fills Name, Description, Path, Program
        │
        ▼ (optional)
User opens "Advanced options", adds env vars
        │
        ▼
User clicks "Save"
        │
        ├─ Client validation ──→ FAIL → inline error shown, form stays open
        │
        ├─ RPC upsertAlias ───→ FAIL → top-level error banner, form stays open
        │
        └─ RPC upsertAlias ───→ SUCCESS → success banner (3s), form closes,
                                           list refreshes with new alias row
```

### Edit Alias (Happy Path)

```
User clicks "Edit" on alias row
        │
        ▼
Form opens, fields pre-populated, Name disabled
Focus → Description (first editable field)
        │
        ▼
User changes Description (or any field)
        │
        ▼
User clicks "Save"
        │
        ├─ Client validation ──→ FAIL → inline error shown (no conflict check)
        │
        ├─ RPC upsertAlias ───→ FAIL → top-level error banner, form stays open
        │
        └─ RPC upsertAlias ───→ SUCCESS → success banner (3s), form closes,
                                           list row updates
```

### Delete Alias (Happy Path)

```
User clicks "Delete" on alias row
        │
        ▼
Row's Delete button replaced by "Confirm delete?" button
(auto-reverts to "Delete" after 3s if not clicked)
        │
        ├─ 3s elapse → "Delete" button restored, no change
        │
        └─ User clicks "Confirm delete?"
                  │
                  ├─ RPC deleteAlias ──→ FAIL → top-level error banner, row remains
                  │
                  └─ RPC deleteAlias ──→ SUCCESS → success banner (3s), row removed
```

---

## Layout in Settings Page

The AliasesManager section is placed after DirectoryRulesManager and before the Help section in the Settings > General tab:

```
Settings > General
  ├── Profiles (ProfilesManager)
  ├── Directory Rules (DirectoryRulesManager)
  ├── Aliases (AliasesManager)   ← NEW
  └── Help / Config Files links
```

Each section is wrapped in a `<section className={styles.section}>` element, consistent with the existing Settings page structure.
