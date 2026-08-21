# Research 01 — Stack & Existing Patterns

## (a) CSS Approach

The project has fully adopted **vanilla-extract** per ADR-009. `ApprovalRulesPanel.css.ts` uses `@vanilla-extract/css` (`style`, `globalStyle`) throughout, importing all design tokens from `@/styles/theme.css` (`vars`). No `module.css` files are used in this component.

Key patterns observed:
- All colors via `vars.color.*` (e.g. `vars.color.cardBackground`, `vars.color.textPrimary`, `vars.color.primary`).
- Pseudo-selectors via the `selectors` object: `"&:hover"`, `"&:disabled"`.
- Responsive breakpoints via `@media` inside `style({})`.
- `globalStyle()` used for child selectors that vanilla-extract's scoped `style()` cannot express (e.g. `${row}:last-child td`).
- `@vanilla-extract/recipes` is available in `package.json` but not yet used in this component — good fit for the multi-variant `decision`/`source` badge patterns.

## (b) Existing Form Patterns

`ApprovalRulesPanel.tsx` implements a flat, show/hide form:

- `showForm` boolean toggles between the "+ Add Custom Rule" button and an inline form `<div>`.
- `RuleFormState` is a plain TypeScript interface; no form library is used (no react-hook-form here, even though it is installed).
- All fields are uncontrolled native `<input>` and `<select>` elements with inline `onChange` lambdas.
- `formGrid` uses `grid-template-columns: repeat(auto-fill, minmax(220px, 1fr))` — simple responsive grid.
- No edit mode exists: the form only creates new rules. There is no "Edit" button on existing rows.
- The `handleSave` function does basic required-field validation inline before calling `upsertRule`.
- There is no step-by-step wizard, no tab navigation within the form, no progressive disclosure.
- Form error is shown in a single `formError` div at the top of the form.

## (c) Existing ApprovalRuleProto Fields

From `proto/session/v1/types.proto`, the current `ApprovalRuleProto` message (field numbers 1–14):

| # | Field | Type |
|---|-------|------|
| 1 | `id` | string |
| 2 | `name` | string |
| 3 | `tool_name` | string |
| 4 | `tool_pattern` | string |
| 5 | `command_pattern` | string |
| 6 | `file_pattern` | string |
| 7 | `decision` | AutoDecision enum |
| 8 | `risk_level` | string |
| 9 | `reason` | string |
| 10 | `alternative` | string |
| 11 | `priority` | int32 |
| 12 | `enabled` | bool |
| 13 | `source` | string |
| 14 | `created_at` | Timestamp |

## (d) Missing Proto Fields for CommandCriteria

The backend `CommandCriteria` struct (in `pkg/classifier/classifier.go`) has these fields that are NOT in `ApprovalRuleProto`:

| Classifier field | Go type | Notes |
|-----------------|---------|-------|
| `Programs` | `[]string` | Primary program matching with prefix support |
| `Subcommands` | `[]string` | Allow-list of subcommand values |
| `BlockedSubcommands` | `[]string` | Deny-list of subcommands |
| `RequiredFlags` | `[]string` | At least one must match (exact token) |
| `RequiredFlagPrefixes` | `[]string` | At least one arg must start with prefix |
| `ForbiddenFlags` | `[]string` | None may appear in args |
| `PythonModes` | `[]string` | `"inline"`, `"module"`, `"script"`, `"version"` |
| `SafePythonImportsOnly` | `bool` | Restricts inline Python to safe stdlib |
| `RedirectionPattern` | `*regexp.Regexp` | Matches redirection file paths (skip for UI — too advanced) |

The requirements doc proposes adding fields 20–26 to proto. Field numbers 15–19 are currently unused, so starting at 20 is safe. `RequiredFlagPrefixes` (not mentioned in requirements) should be considered for inclusion — it is used by the seed `sed -i` escalation rule.

Additionally, `RuleSpec` in `rules_store.go` has a `ToolCategory` field (for matching builtin-agent, mcp-read, etc.) that is persisted to the ent DB but is NOT in `ApprovalRuleProto`. This should be included when extending the proto.

## (e) Available Component Libraries

From `web-app/package.json`:

- **`@radix-ui/react-dialog`** (v1.1.15) — accessible modal/dialog primitive, already installed. Perfect for template picker and rule builder modals.
- **`@radix-ui/react-slot`** (v1.2.4) — composition primitive.
- **`react-hook-form`** (v7.63.0) + **`@hookform/resolvers`** — full-featured form state management with validation. Not yet used in `ApprovalRulesPanel` but available.
- **`zod`** (v4.1.11) — schema validation, pairs naturally with react-hook-form + `@hookform/resolvers/zod`.
- **`@vanilla-extract/recipes`** (v0.5.7) — multi-variant style recipes (useful for tag pill, decision badge variants).
- **No shadcn/ui** — components must be built from scratch with vanilla-extract + Radix primitives.
- **No Headless UI, no MUI, no Chakra** — stack is intentionally minimal.
- **`fuse.js`** (v7.3.0) — fuzzy search, available for template library search.

### Key Implications for Implementation

1. **Tag input** will be custom-built (no tag-input library is installed). Use react-hook-form for field state, vanilla-extract for styling, and a simple `<input>` + chip-array pattern.
2. **Template picker modal** should use `@radix-ui/react-dialog` — it handles focus trap, Escape key, and ARIA automatically.
3. **Form validation** should switch from inline hand-rolled validation to `react-hook-form` + `zod` for the new complex form.
4. **Recipe-based styling** (`@vanilla-extract/recipes`) is ideal for the multi-state decision badge and tag chip components.
5. **No additional installs needed** for the core feature — all required primitives are already in `package.json`.
