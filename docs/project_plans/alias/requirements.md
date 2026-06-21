# Requirements: Alias Feature

**Date**: 2026-06-20
**Branch**: stapler-squad-alias
**Status**: Research complete — ready for planning

---

## Problem Statement

Users frequently create sessions with the same configuration: same path, same program, same flags. Today they must re-enter all fields every time. There is no shortcut for "launch my usual setup for project X."

## Jobs-to-be-Done

1. **Primary**: When I start a session for a recurring context, I want to launch it in one gesture so I don't reconstruct the same config from memory every time.
2. **Parameterization**: When I use an alias as a template, I want to vary one thing (branch, session label, extra flags) without defining a separate alias for every permutation.

## Target Users

- Developers who manage multiple projects or contexts in stapler-squad
- Users with per-project program/flag preferences (e.g., different Claude models per project)
- Users who use `DirectoryRules` or named `Profiles` but want even faster invocation

## Functional Requirements

### FR-1: Alias definition in config

Aliases are defined in `config.json` as an array of objects:

```json
"aliases": [
  {"name": "myproj", "path": "~/code/myproj", "program": "claude"},
  {"name": "work-infra", "path": "~/infra", "program": "aider", "group": "work", "description": "Infrastructure monorepo"},
  {"name": "quick", "cli_flags": "--model claude-haiku-4-5-20251001", "group": "tools"}
]
```

**`AliasConfig` fields**:
- `name` — required; must match `^[\w-]+$`; stored and matched case-insensitively (lowercase)
- `group` — optional; display category in the alias palette
- `path` — optional default working directory; `~` expanded at runtime
- `description` — optional; shown as a hint line in the alias palette
- `profile` — optional; name of an existing `SessionDefaults.Profiles` entry to inherit from
- `program`, `auto_yes`, `tags`, `env_vars`, `cli_flags` — same semantics as `ProfileDefaults`; applied after profile resolution

**Validation rules**:
- Names must be unique across all aliases (case-insensitive)
- Names must match `^[\w-]+$` (alphanumeric + underscore + hyphen)
- No spaces, no `@` prefix in stored names

### FR-2: Omnibar trigger — `@` prefix

The user activates aliases by typing `@` in the omnibar. The `AliasDetector` is registered in the detector registry with a priority between `NewSessionDetector` (35) and `GitHubShorthandDetector` (40) — suggested priority **36**.

**Detection grammar**:
```
@<name>[:branch][ label text][ --extra-flags]
```

- `@name` — required; resolves the named alias
- `:branch` — optional; overrides the branch for worktree creation
- `label text` — optional; any text before the first `--` token becomes the session title
- `--extra-flags` — optional; text starting with the first `--` token is appended to `CLIFlags`

**Examples**:
- `@myproj` → resolve alias `myproj` exactly
- `@myproj:feature/auth` → with branch override
- `@myproj working on auth` → alias with session label "working on auth"
- `@myproj --model claude-haiku-4-5-20251001` → alias with flag override
- `@myproj:feat working on auth --model claude-haiku-4-5-20251001` → all parameters

**Unrecognized alias**: If `@nonexistent` is typed and no alias matches, `AliasDetector` returns a `DetectionResult` with `type: InputType.AliasNotFound` — **it does not fall through to SessionSearchDetector**. The UI shows "No alias '@nonexistent' — create one?" instead of displaying search results.

### FR-3: Alias palette (browse mode)

When the user types `@` with nothing after it:
- **Grouped display**: aliases organized into sections by `group` field; ungrouped aliases appear first with no section header
- **Empty state**: if no aliases are configured, show "No aliases yet — add them in config.json"

When the user types `@` followed by any characters:
- **Flat fuzzy-filtered list** across all groups (group headers disappear during active filtering)

### FR-4: Environment variable support

`EnvVars map[string]string` in `AliasConfig` follows the same merge semantics as `ProfileDefaults.EnvVars`. Additionally, values may use `${VAR_NAME}` syntax to reference host shell environment variables:

```json
{"name": "work", "env_vars": {"ANTHROPIC_API_KEY": "${ANTHROPIC_API_KEY}"}}
```

At session creation time:
- `${VAR}` references are expanded via `os.Getenv`
- If the referenced variable is not set in the environment, the key is **omitted** (not passed as empty string)
- Literal values are passed through unchanged

### FR-5: CLI flags

**Definition-time** (static): `cli_flags` field in `AliasConfig`. Applied every time the alias is invoked.

**Invocation-time** (dynamic): text starting with `--` in the omnibar input after the alias name. These are **appended** to the alias's static `cli_flags`. Since most CLI tools use last-value-wins semantics for duplicate flags, invocation flags override alias flags.

### FR-6: Default resolution order

When an alias is invoked, the system resolves configuration in this order (lowest → highest priority):

1. Global `SessionDefaults` (program, env_vars, cli_flags, tags)
2. `DirectoryRule` for `alias.Path`
3. Named `Profile` referenced by `alias.Profile`
4. Alias inline fields (`program`, `env_vars`, `cli_flags`, `tags`, `auto_yes`)
5. Invocation-time overrides (branch, label, extra flags)

### FR-7: Pre-existing wire gap fix (prerequisite)

`ResolveDefaults` already computes `EnvVars` and `CLIFlags` but they are not applied in `CreateSession` — neither field exists in `InstanceOptions` or `CreateSessionRequest`. Aliases require this to be fixed:

- Add `EnvVars map[string]string` and `CLIFlags string` to `InstanceOptions`
- Add `env_vars` and `cli_flags` to `CreateSessionRequest` proto
- Wire `resolved.EnvVars` and `resolved.CLIFlags` into `instanceOpts` in `session_service.go`
- Apply `InstanceOptions.EnvVars` when building the tmux session environment

## Non-Functional Requirements

- **Performance**: alias lookup is O(n) over the alias list; n < 1000 in all practical cases; no caching needed
- **Backward compatibility**: config.json without `aliases` key loads without error (zero value is empty slice)
- **No proto breaking changes**: new fields use next available field numbers

## Out of Scope (v1)

- In-app alias creation UI (users edit config.json directly)
- "Save session as alias" from the session overflow menu
- Positional `$1`-style parameter templates
- Team/shared alias config
- Alias analytics / MRU ordering
- Path override at invocation time
- Tag-based alias filtering in the palette

## UX Requirements

(From UX review conducted 2026-06-20)

- **ALIAS-001** (critical): Unrecognized `@alias` must not fall through to session search — show "alias not found" state
- **ALIAS-002** (critical): Show inline chip/badge as soon as a valid alias prefix resolves (visual confirmation mid-type)
- **ALIAS-003** (high): `@` is already used as branch separator in `path@branch` — document the distinction clearly in placeholder/hint text
- **ALIAS-005** (high): Omnibar placeholder must mention `@alias` as a supported input type
- **ALIAS-006** (high): Define empty-state copy for when no aliases are configured
- **ALIAS-009** (medium): Case-insensitive matching; names normalized to lowercase at config write time
- **ALIAS-010** (medium): Everything before the first `--` token (after alias name/branch) is the session label

## Success Metrics

**Outcome metrics** (measurable business outcomes):
- ≥50% of users who configure ≥1 alias invoke it within 7 days of adding it (alias retention)
- Alias session creation accounts for ≥10% of all session creations within 30 days of shipping (adoption)
- Zero regression in existing `DirectoryRules` and `Profile` session creation rates after FR-7 wire-gap fix

**Acceptance criteria** (verifiable at release):
- User can invoke a saved alias in ≤ 2 keystrokes after `@`
- Zero config required for the simplest alias: `{"name": "foo", "path": "~/foo"}`
- `env_vars` and `cli_flags` from `ResolveDefaults` are applied in all new sessions (not just alias sessions)

**Risky assumptions** (named for tracking):
- Users will discover the feature via the omnibar placeholder hint without in-app onboarding (mitigation: v1 ships with placeholder text change; v2 adds onboarding tooltip)
- Config-file-only alias creation is acceptable for v1 (mitigation: validate in beta; "Save as alias" is planned v2)
- FR-7 wire-gap fix (`EnvVars`/`CLIFlags` wired through `InstanceOptions`) carries no regression risk for existing non-alias sessions
