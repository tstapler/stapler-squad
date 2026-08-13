# ADR-002: Defaults Precedence Algorithm

Date: 2026-04-13
Status: Accepted

## Context

Three default sources can apply to a new session:
- Global defaults (always present)
- Directory-rule defaults (matched by working directory path prefix)
- Named profile (explicitly selected by user)

The merge semantics for scalar fields (program, cli_flags) vs collection fields (tags,
env_vars) are not obvious and need to be decided before implementation.

Also: when a directory rule references a named profile _and_ the user independently
selects a profile, which wins?

## Decision

**Precedence order (lowest → highest):**
```
Config.DefaultProgram (legacy fallback)
  → SessionDefaults.Global
    → DirectoryRule.Overrides (longest-prefix match)
      → Named Profile (request.profile field)
```

**Scalar fields** (program, cli_flags, auto_yes, category, prompt): higher layer overrides
lower; empty string / false means "use lower layer's value" (no sentinel needed for MVP).

**Tags**: union across all active layers. Each layer adds its tags; duplicates removed.

**EnvVars**: higher-layer key wins. A key set to `""` in a higher layer explicitly clears
the lower-layer value.

**Directory matching**: longest-prefix match wins (most specific directory). Symlinks
resolved via `filepath.EvalSymlinks()` before comparison.

**Profile collision** (directory rule specifies profile AND user selects profile):
user-selected profile wins (user intent is more explicit).

**Tags merge semantics**: additive union chosen over override because tags are labels;
the user adding global "Work" tags alongside a directory's "Project-X" tags is the
expected experience.

## Rationale

- Matches Wezterm + Zellij conventions (most specific wins)
- Union for tags matches how tagging works in all comparable tools (iTerm2, Logseq)
- Longest-prefix for directory matching is deterministic and documented; no filesystem
  traversal needed at runtime
- Pointer types not required for MVP; "empty = inherit" is simpler and covers all known
  use cases

## Consequences

- `ResolveDefaults` is a pure function given the config; easily unit-tested
- Users cannot "clear" a scalar from a lower layer using a higher-layer empty value
  (acceptable for MVP; pointer fields can be added later if needed)
- Tags accumulate across layers; users wanting to override (not add) tags must explicitly
  configure all desired tags in the highest-priority layer they use
