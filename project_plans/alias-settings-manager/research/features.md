# AliasesManager — Feature Research

## 1. What the codebase already has

### AliasConfig struct (config/config.go, line 441)
```go
type AliasConfig struct {
    Name        string            // required; must match ^[\w-]+$
    Group       string            // optional display group
    Path        string            // supports ~/... expansion
    Description string            // shown in palette
    Profile     string            // named profile to apply
    Program     string            // overrides default program
    AutoYes     bool
    Tags        []string
    EnvVars     map[string]string
    CLIFlags    string
}
```

### Existing RPCs
- `ListAliases` — read-only, already implemented in `defaults_service.go`
- **NO** `UpsertAlias` or `DeleteAlias` RPC exists yet. Both must be added to the proto and service layer before the UI can write.

### AliasPalette.tsx (web-app/src/components/ui/AliasPalette.tsx)
The palette is a **read-only selector** used inside the omnibar. It:
- Filters by `name`, `description`, and `group` against the `@`-prefixed input
- Groups aliases visually by `group` when not filtering
- Renders name, description, path, and program per row
- Shows an error state and a "copy config.json path" empty state

The palette does **not** overlap with settings management — it is purely a launch surface, not a CRUD interface. The manager needs to produce what the palette consumes, not replace it.

### useAliases.ts
Fetches via `listAliases` RPC; maps to `AliasEntry` which exposes `name`, `group`, `path`, `description`, `profile`, `program`, `autoYes`, `tags`. Note: **`envVars` and `cliFlags` are NOT in `AliasEntry`** — the hook omits them, so the manager will need either a separate fetch path or an extended hook.

### useAliasSuggestions.ts
Pure client-side filter over an `AliasEntry[]` from `useAliases`. Drives the omnibar suggestion mode. Not relevant to the settings manager except as a correctness test: names must remain `^[\w-]+$` to work with the `@`-trigger regex.

---

## 2. Edge cases the alias form must handle

### Name field
- **Required** — a blank name must block save (same guard as ProfilesManager).
- **Pattern enforcement** — only `[\w-]+` is valid (letters, digits, underscore, hyphen). Spaces and `@` must be rejected client-side with a clear message: "Name may only contain letters, digits, hyphens, and underscores."
- **Uniqueness** — since aliases are stored as a slice (not a map), duplicates are physically possible in `config.json`. The form must reject a name that already exists on **create**; on **edit** it must allow saving under the same name.
- **Case sensitivity** — the omnibar filter is case-insensitive (`toLowerCase()`), but names are stored as-is. Two aliases `Foo` and `foo` would both respond to `@foo`. The form should warn when a name collides case-insensitively with an existing alias.
- **Immutability on edit** — consistent with `ProfilesManager`, the name field should be disabled when editing (the name is the natural key). A rename is a delete-then-create.

### Path field
- **Optional** (unlike `DirectoryRulesManager` where path is the primary key and required).
- When present, must support `~/...` expansion (the backend expands it; the UI should not validate the path exists, only show a hint like "Supports ~ expansion").
- When absent, the alias functions as a pure config preset (program + flags + env_vars) with no working directory locked in.

### EnvVars map
- Must support zero or more `KEY=VALUE` pairs.
- The UI needs an **add-row** affordance (key input + value input + add button), a list of existing pairs each with a remove button, and inline validation that keys are non-empty and do not contain `=`.
- `ProfilesManager` currently passes `envVars: {}` (hardcoded empty), so there is no prior pattern in the codebase to copy. Look at how `GlobalDefaultsForm.tsx` handles `env_vars` — it may already have a key-value editor widget.
- `${VAR}` substitution is applied server-side (`ExpandEnvVars`); the form should surface a hint that shell variables are expanded at session creation time.

### CLIFlags
- Free-text string appended to the program command. No validation needed beyond trimming.
- Should be visually subordinate (advanced field) since most users won't need it.

### Tags
- Same chip-input pattern as `ProfilesManager` and `DirectoryRulesManager` — tag text field + Enter/Add button → rendered removable chips.
- Duplicate tags within one alias should be silently deduplicated (ProfilesManager already does this: `!form.tags.includes(trimmed)`).

### AutoYes
- Checkbox, same as ProfilesManager. No special edge cases.

### Profile reference
- Dropdown populated from `getSessionDefaults().profiles`. If the referenced profile is later deleted, the alias silently loses its profile override at resolution time. The form could show a warning badge next to a profile name that no longer exists.

---

## 3. Unstated user needs beyond CRUD

### Duplicate / Clone
Users who maintain many project aliases with similar shapes (same program, same group, different paths) will want a **"Duplicate" or "Clone"** action — copies an alias with a fresh name, opens the edit form pre-populated. This is a common config-UI pattern absent from both `ProfilesManager` and `DirectoryRulesManager`.

### Group management
The `AliasPalette` groups aliases visually, but there is no way in settings to see which groups exist, rename a group across aliases, or move an alias between groups. A **group filter/badge** in the list view (showing how many aliases each group contains) would help users who use groups heavily.

### Reorder / prioritize
Aliases are stored as an ordered slice; the palette renders them in config order (ungrouped first, then grouped). The manager should either respect the order visually (drag-to-reorder or up/down arrows) or clearly state that order is alphabetical within groups.

### Search / filter in the list view
For users with 10+ aliases the manager list will become hard to scan. A **search input** above the list (filtering by name, group, description) mirrors the palette's filter behavior and prevents the "I can't find the alias I want to edit" problem.

### Import from omnibar history / recent paths
The omnibar already tracks recently-used paths. A **"Create alias from current session"** button (either here or in the session context menu) would bootstrap an alias from an already-opened session's path and program. This is outside scope of the settings manager itself but is an important discovery path.

### Bulk delete
No precedent in sibling managers, but reasonable for power users. Low priority.

### Live preview
Show the `@name` token that would invoke this alias, and a simulated launch string (path + program + cli_flags) below the form so users can verify before saving.

---

## 4. AliasPalette overlap analysis

| Concern | AliasPalette | AliasesManager |
|---|---|---|
| Data source | `useAliases` (read-only RPC) | `getSessionDefaults` + new upsert/delete RPCs |
| Purpose | Omnibar launch surface | CRUD config management |
| Fields shown | name, group, description, path, program | all 10 fields |
| User action | select → create session | create / edit / delete / reorder |
| Group display | visual grouping in list | group is an editable string field |

No overlap in function. The palette is the consumer; the manager is the producer.

---

## 5. Field frequency / priority classification

### Primary fields (always visible)
| Field | Rationale |
|---|---|
| `name` | Required, the `@name` trigger |
| `description` | Shown directly in palette row — users set it to remember what the alias does |
| `path` | Most aliases encode a project directory |
| `program` | Select from available programs; drives which agent runs |
| `group` | Short string; important for palette organization |

### Secondary fields (visible by default, can scroll)
| Field | Rationale |
|---|---|
| `profile` | Dropdown linking to a named profile |
| `autoYes` | Checkbox; frequently used for unattended sessions |
| `tags` | Tag chip input; used for session organization |

### Advanced / collapsible fields
| Field | Rationale |
|---|---|
| `envVars` | Map editor; complex UI, rarely needed by casual users |
| `cliFlags` | Free-text; power-user escape hatch |

The split mirrors the `DirectoryRulesManager` pattern where `showOverrides` gates a collapsible section. An "Advanced" disclosure (collapsed by default) for `envVars` and `cliFlags` keeps the form approachable.

---

## 6. EnvVars UI patterns (how similar tools handle key→value maps)

Common patterns observed in config management UIs:

1. **Table with editable rows** — each row has a key input, value input, and delete button. An "Add row" button appends a blank row. Used by Kubernetes ConfigMap editors, Vercel env var UI, GitHub Actions secrets.
2. **Chip-style key=value pairs** — user types `KEY=value`, presses Enter to add a chip. Compact but error-prone for long values.
3. **Side-by-side input + Add button** — two text inputs (key, value) side by side, plus an Add button. Added pairs render as a list with X buttons. Closest to the tag-input pattern already in use.

**Recommendation for AliasesManager**: Use pattern 3 (side-by-side inputs + Add), which aligns with the existing tag chip pattern in `ProfilesManager`. Key validation: non-empty, no `=` character. Value validation: none (values may contain `=`). Show a hint: "Use \${VAR} to reference shell variables (expanded at session start)."

---

## 7. Missing backend RPCs — critical gap

Before any UI code can write aliases, two RPCs must be added:

### UpsertAlias
```protobuf
message UpsertAliasRequest  { AliasProto alias = 1; }
message UpsertAliasResponse {}
rpc UpsertAlias(UpsertAliasRequest) returns (UpsertAliasResponse) {}
```
Backend logic: load config → find existing alias by name → replace or append → save config.

### DeleteAlias
```protobuf
message DeleteAliasRequest  { string name = 1; }
message DeleteAliasResponse {}
rpc DeleteAlias(DeleteAliasRequest) returns (DeleteAliasResponse) {}
```
Backend logic: load config → filter out alias by name → save config; return CodeNotFound if absent.

These follow the exact pattern of `UpsertProfile` / `DeleteProfile` in `defaults_service.go`.

---

## 8. Integration with settings page

The `AliasesManager` component drops into `web-app/src/app/settings/page.tsx` as a `<section>` inside the `general` tab, after `DirectoryRulesManager` and before the Help section — consistent with the existing left-to-right hierarchy (global defaults → profiles → directory rules → aliases → help).

```tsx
<section className={styles.section}>
  <AliasesManager />
</section>
```

No tab changes needed; no routing changes needed.
