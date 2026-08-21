# UX Research: AliasesManager

**Date**: 2026-06-21
**Feature**: Settings > General > Aliases CRUD panel
**Researcher**: Claude Code (UX Research Agent)

---

## Existing Pattern Audit

Both `ProfilesManager.tsx` and `DirectoryRulesManager.tsx` establish the definitive pattern for settings list managers in this codebase. The pattern is:

**Structure (consistent across both):**
- `headerRow`: heading + primary CTA button ("New Profile" / "New Rule") flush-right
- Alert banners for `error` and `success` states above the list; success auto-dismisses after 3 s
- List rows with `info` section (name/description/meta) and `actions` section (Edit / Delete buttons)
- `formCard` rendered **below the list**, not inline in a row — a single card that replaces the need for a modal
- `formTitle` shows "New X" vs "Edit X: <key>" based on editing state
- `formFields` with labeled inputs; required fields marked with `*`
- Primary key (profile `name`, rule `path`) is **disabled** during edit (immutable identity field)
- `formActions`: "Save" (primary, disabled while saving, shows "Saving...") + "Cancel" (secondary)
- Error state shown in the shared top-level error banner, not inside the form
- Tag editing uses an add-on-Enter chip pattern (`tagInputRow`)

**Key behavioral notes:**
- `showForm` is a single boolean; only one item is editable at a time (no expand-in-place rows)
- Edit opens the form card at the bottom; the list stays rendered above it
- Delete uses `window.confirm()` (blocking confirm dialog)
- `DirectoryRulesManager` adds a collapsible "field overrides" section controlled by a checkbox — this is the established pattern for advanced/optional fields
- `GlobalDefaultsForm.tsx` establishes the key-value pair editor for `env_vars`: a table of row pairs with `KEY` / `value` inputs and an "Add Variable" button — **not** a textarea

---

## Q1: List + Inline Form Pattern

**Finding**: The codebase has a clear, consistent answer: **single bottom-anchored form card**. Neither expand-in-place rows (accordion) nor modals are used anywhere in Settings.

**Recommendation**: Follow the same pattern for `AliasesManager`:
1. List rows with alias name, description, group, path, program as metadata chips
2. Clicking "Edit" populates the shared form card below the list
3. Clicking "New Alias" clears the form card and shows it
4. No modals; no row expansion

**Rationale**: Consistency with ProfilesManager and DirectoryRulesManager is more valuable than marginal UX gains from alternative patterns. The bottom-form pattern also works well for long forms (many alias fields) because it doesn't truncate the list.

---

## Q2: env_vars — Key-Value Editor vs KEY=VALUE Textarea

**Finding**: `GlobalDefaultsForm.tsx` already implements the structured key-value pair editor and it is the right choice for this project. The implementation uses:
- A `{ key: string; value: string }[]` array in state
- Separate `KEY` and `value` text inputs per row in an `envVarTable` / `envVarRow` layout
- "Add Variable" button appends a blank row
- "Delete" button per row (with `aria-label="Remove environment variable"`)

**Recommendation**: Use the identical key-value editor pattern from `GlobalDefaultsForm`. Do not use a `KEY=VALUE` textarea because:
1. The pattern already exists and is tested
2. Textarea parsing has edge cases (quoted values, values with `=`, blank lines)
3. Individual inputs give better error targeting (highlight the invalid KEY field specifically)

**Field placement**: Put `env_vars` in the advanced/collapsible section (see Q3).

---

## Q3: Primary vs Advanced Fields

The alias proto has 10 fields. Not all deserve equal prominence. Proposed split:

**Primary (always visible in the form):**
- `name` — required; immutable identity key (disabled on edit, same as ProfilesManager `name`)
- `description` — optional short text; reinforces what `@name` does in the omnibar
- `group` — optional; shown in the palette as a section divider; common to set on creation
- `path` — the working directory for the session; critical for the core JTBD ("start in my project")
- `profile` — select from existing profiles (cross-reference like DirectoryRulesManager's profile select)
- `program` — select from `PROGRAMS` constant (same as ProfilesManager)
- `auto_yes` — checkbox (same as ProfilesManager)
- `tags` — chip input (same as ProfilesManager)

**Advanced (behind a "More options" / "Advanced" collapsible, default collapsed):**
- `env_vars` — key-value pair editor; rarely set, adds form height
- `cli_flags` — plain text input; power-user option

**Pattern precedent**: `DirectoryRulesManager` uses a checkbox (`showOverrides`) to toggle an `overridesSection`. Use the same checkbox-gated collapsible for `env_vars` + `cli_flags`, labeled "Advanced options".

---

## Q4: Keyboard Navigation and ARIA (WCAG AA)

**Current state of existing managers:**
- `ProfilesManager` tags have `aria-label="Remove tag <name>"` — good
- `AliasPalette.tsx` has full `role="listbox"` / `role="option"` / `aria-activedescendant` / `aria-selected` — excellent reference for the read path
- No `role="form"` or `aria-labelledby` on the form cards in either existing manager (gap)
- No focus management after "Edit" click — form card appears below but focus stays on the Edit button (gap)

**Required for WCAG AA compliance in AliasesManager:**

1. **Form labeling**: wrap `formCard` in a `<section>` with `aria-labelledby` pointing to the `formTitle` h3.
2. **Focus management**: when the form card opens (New or Edit), move focus to the first field (`name` input) via `useEffect` + `ref.focus()`.
3. **Announce state changes**: use `role="status"` on the success/error banners (currently using div — should add `role="alert"` for errors, `role="status"` for success). `AliasPalette.tsx` already uses `role="alert"` on errors and `role="status"` on empty state — follow that pattern.
4. **Delete confirm dialog**: `window.confirm()` (current pattern) is accessible but visually dated. It is acceptable for this iteration — improvement to a custom confirm is out of scope per requirements.
5. **Tag remove buttons**: already `aria-label="Remove tag <name>"` — replicate exactly.
6. **env_vars row delete**: must have `aria-label="Remove environment variable <key>"` (dynamic based on key value if available, or index fallback).
7. **Keyboard in form**: `Enter` in `tagInput` adds tag (preventDefault to stop form submit). Same pattern in DirectoryRulesManager — replicate.
8. **Required field indication**: `*` in label text (existing pattern) is acceptable; add `aria-required="true"` on the name input.
9. **List rows**: existing managers use `div` for rows with `button` children — this is fine (buttons are keyboard-reachable). No ARIA list role needed.
10. **Select elements**: all selects need a visible label with `htmlFor` → `id` pairing — existing pattern does this correctly.

**Gap to address specifically in AliasesManager (not present in existing managers):**
- Focus management on form open (see #2 above)
- `<section aria-labelledby>` wrapper on form card (see #1 above)

---

## Q5: Error States

**Error taxonomy for AliasesManager:**

| Error | Source | UX Treatment |
|---|---|---|
| Name empty | Client validation | Inline below name field (same as `pathError` in DirectoryRulesManager): red border on input + `<span className={fieldError}>` below |
| Name conflict (alias name already exists and this is a "New" not "Edit" action) | Server 409 or client-side check against loaded aliases | Show in top-level `error` banner: "An alias named '@foo' already exists." |
| Invalid path (blank when not required vs. server-side validation) | Client + server | Path is optional for aliases (unlike directory rules). If provided, validate it starts with `/` or `~` (client-side). Server errors caught in `catch` block → top banner. |
| Name format validation (regex) | Client validation | Name should match `[\w-]+` (same constraint as AliasDetector which does `nameMatch(/^@([\w-]+)/)`). Validate on blur: "Name can only contain letters, numbers, hyphens, and underscores." Inline below field. |
| Save failure (network / server error) | Server | Top-level `error` banner with raw error string (existing pattern) |
| Load failure | Server | Top-level `error` banner (existing pattern) |
| Delete failure | Server | Top-level `error` banner (existing pattern) |

**Name validation regex**: `^[\w-]+$` — derived from AliasDetector's grammar (`[\w-]+`).

**Name conflict detection**: client-side pre-check on save (compare `form.name` against loaded `aliases.map(a => a.name)` for case-insensitive match) is better than relying on a server 409 because: (a) the feedback is immediate, (b) the server may return a generic error. Show: "An alias named '@foo' already exists. Edit it instead."

**Restart-required notice**: per requirements, if live reload is not available, show a dismissible info banner after a successful save: "Alias saved. Restart stapler-squad for changes to take effect." Use `role="status"` (non-blocking). This is different from the 3-second auto-dismiss success — it should persist until dismissed or next page load.

---

## Q6: Job-to-Be-Done Alignment

**JTBD**: "When I want to quickly start a session for my recurring project, I want to type @myproj and have it just work."

**How the manager supports this mental model:**

The AliasesManager is the configuration backstage for the omnibar `@name` flow. The connection between typing `@myproj` and "it just works" depends on three things being true:
1. The alias exists in config with the right `path` and `program`
2. The alias appears in the `AliasPalette` when typing `@` (live lookup from `useAliases` → `ListAliases` RPC)
3. The session starts in the right working directory with the right program

**UX implications for the manager:**

- **Preview badge in the list**: each alias row should show a compact "preview tag" showing `@name`, `path`, and `[program]` — exactly the same fields shown in `AliasPalette`'s `AliasRow`. This makes the link between "what I configured" and "what I see in the omnibar" visually obvious.
- **Omnibar preview hint in the form**: below the `name` field, show a small grey hint: "Available in the omnibar as `@<name>`" (update dynamically as the user types the name). This is the `@myproj → just works` connection made explicit.
- **Empty state**: When no aliases are configured, the empty state should mirror the messaging in `AliasPalette.tsx`'s `emptyBody` ("Add them to launch sessions faster") but replace the config.json instruction with a "New Alias" button. Bridge the two surfaces so the user discovers the manager from the palette.
- **Group field hint**: "Group organizes aliases in the `@` palette. Leave blank for ungrouped." — this is the only field whose purpose is non-obvious without context.

---

## Q7: Omnibar Preview

**Recommendation: Yes, with constraints.**

**What to show:** In the form card, after the `name` field, render a non-interactive preview chip that shows exactly how the alias will appear in the `AliasPalette`:

```
@myproj  ~/code/myproject  [claude]
```

This uses the same visual treatment as `AliasPalette`'s `rowName` / `rowDesc` / `rowMeta` / `rowProgram` elements. Update live as the user edits `name`, `path`, `program`, `description`.

**What not to show:** Do not attempt to simulate what session would be created — too complex and potentially inaccurate until the alias is actually saved. Keep the preview scoped to "how it looks in the palette."

**Implementation note**: The preview can be a small static presentational block using `AliasPalette`'s existing CSS classes (or extract shared styles). It requires no new RPC calls — it derives purely from the form state.

**Placement**: Between the primary fields section and the advanced options section. Label it: "Preview in omnibar" with a subdued heading style.

---

## Summary of Recommendations

### Pattern consistency (non-negotiable)
- Single bottom-anchored `formCard`, same as ProfilesManager / DirectoryRulesManager
- `headerRow` with "New Alias" CTA
- Success/error alert banners, 3 s auto-dismiss for success
- Name field disabled on edit (immutable key)
- Chip-based tag editor with Enter-to-add
- Key-value pair editor for `env_vars` (from GlobalDefaultsForm)
- `window.confirm()` for delete

### New additions vs existing pattern
- Focus management: `useEffect` + `ref.focus()` when form opens
- `<section aria-labelledby={formTitleId}>` wrapping the form card
- Inline field error for name format/empty (fieldError pattern from DirectoryRulesManager)
- Client-side name conflict check before save
- Live `@name` preview below name field
- "Advanced options" collapsible (checkbox pattern from DirectoryRulesManager) containing `env_vars` + `cli_flags`
- Restart-required persistent info banner after save (if backend does not hot-reload)
- Group field with contextual hint text

### Field ordering in the form
1. Name * (with live `@name` hint below)
2. Description
3. Group (with hint)
4. Path
5. Profile (select)
6. Program (select)
7. Auto-yes (checkbox)
8. Tags (chip editor)
9. [Omnibar preview block]
10. "Advanced options" checkbox → env_vars + cli_flags
