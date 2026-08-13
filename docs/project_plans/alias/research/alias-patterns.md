# Research: Alias Patterns in Developer Tools

**Date**: 2026-06-20

## Tool Survey

**SSH `~/.ssh/config`**: Stanza-per-host with no native grouping. Naming conventions (`work-api`, `dev-db`) carry all organizational semantics. Scales poorly past ~30 aliases without discipline; searching is purely by name recall.

**GitHub CLI (`gh alias`)**: Flat YAML key→string map under `aliases:`. Supports `$1` positional args in alias strings. Cannot express rich structure — adding metadata requires a format break. Simple to implement, impossible to evolve. The string-map approach is a dead end for aliases that need multiple fields.

**kubectl contexts (`~/.kube/config`)**: Array of `{name, context: {cluster, user, namespace}}` objects. Name is the only organization primitive. Grouping falls to naming conventions. No display-time grouping in `kubectl config get-contexts`.

**tmux key bindings**: Imperative (`bind-key C-a command`), not declarative config. Not relevant to a JSON config schema.

**VS Code `tasks.json`**: Array of task objects where each has `label`, `type`, `command`, and a `group` object (`{kind: "build"|"test"|"none", isDefault: bool}`). Group is a first-class field. The VS Code command palette groups by category prefix in the label string (e.g., `"Git: Commit"`) — fuzzy search collapses groups dynamically. Two-phase UX: grouped sections when browsing, flat fuzzy results when filtering.

**Warp Workflows**: YAML files, each with `name`, `description`, `command`, `tags` (array), and `arguments` (typed parameters with defaults). Tags enable multi-dimensional grouping at render time. This is the closest analogue to stapler-squad aliases.

**Fish abbreviations / zsh aliases**: Flat string maps. No metadata, no grouping. De-facto baseline — every tool that wants more invents its own format.

## Config Format Recommendation

**Array of objects with optional `group` field** (Option D from analysis):

```json
"aliases": [
  {"name": "myproj", "path": "~/code/myproj", "program": "claude"},
  {"name": "work-infra", "path": "~/infra", "program": "aider", "group": "work"},
  {"name": "work-api", "path": "~/api", "group": "work", "description": "API monorepo"}
]
```

**Why arrays beat grouped objects**: Moving an alias between groups requires editing one `group` field, not restructuring the entire JSON tree. Adding metadata fields later (tags, icon, hotkey) is non-breaking.

**Why arrays beat string maps**: String maps (`"name": "command"`) cannot evolve to structured objects without a migration. Array-of-objects lets you add fields freely. Starting with a string map is a one-way door.

## Display Recommendation

**Two-phase**: grouped sections when browsing (`@` alone), flat fuzzy list when filtering (`@wo`). This is validated by VS Code command palette adoption.

- Grouped sections improve *discoverability* (browsing aliases set up weeks ago)
- Flat lists improve *recall speed* (when you remember the alias name)
- Most users are in recall mode; discoverability matters for the long tail

Avoid tag-based filtering in v1 — it requires a secondary UI interaction and adds complexity without benefit at small alias counts (<50).

## Syntax Recommendation

**`@alias-name` only as the omnibar trigger**. No `!` or `#` — `@` is the most semantically meaningful choice for "named thing belonging to me."

**Note**: `@` is already used in this codebase as a branch separator (`path@branch` in `PathWithBranchDetector`). These do not conflict because `PathWithBranchDetector` requires content before the `@` (`^(.+)@`), while the alias trigger requires `@` at the start of input.

## Parameterization Recommendation

Consistent with existing detector grammar (`:` for branch, as in `repo:branch` GitHub shorthand):

```
@alias-name[:branch][ label text][ --extra-flags]
```

- Branch override uses `:` (consistent with `GitHubShorthandDetector`)
- Session label is space-separated text before any `--` token
- Invocation-time flags start at the first `--` token and are appended to alias `cli_flags`

Skip `$1` positional templates for v1 (each tool that implements this — `gh`, Warp, VS Code snippets — chose a different syntax; it's non-trivial to implement correctly).

## Red Flags

1. **String-map evolution trap**: If you start with `"myalias": "/path"`, you cannot add metadata without a migration. Array-of-objects is immune.
2. **Empty-name alias**: Validate `name` is non-empty, no spaces, no `@` prefix stored.
3. **Case sensitivity**: `@MyProj` and `@myproj` should resolve identically. Store and match lowercase.
4. **Fallthrough behavior**: `@nonexistent` should NOT fall through to session search. The alias detector must claim all `^@` input.
