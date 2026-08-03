# ADR-003: Detector Plugin TOML Schema v1

## Status
Accepted — 2026-08-01

## Context

Users will author `.toml` files against whatever ships first, and copy-paste
them between machines and to each other. The schema is a **public API from day
one** — renames and removals are breaking changes for people who never read a
changelog. Every field admitted in v1 is a permanent commitment, so v1 should
admit the minimum that satisfies the requirements.

Two specific questions had to be settled before writing the parser.

### Question 1: does the schema expose `Priority`?

`dtypes.StatusPattern` has a `Priority int` field
(`session/detection/dtypes/dtypes.go:11`, commented "Higher priority patterns
checked first"), and every built-in detector populates it — e.g.
`session/detection/binaries/claude.go:26,34,40,48`.

**It is a dead field.** `PatternSet.compile()` (`pattern_set.go:35-65`) never
reads it, and `PatternSet.MatchLines()` (`pattern_set.go:69-141`) never reads
it. Match order is a hardcoded category chain (error → tests_failing →
needs_approval → input_required → readline-typing → waiting_for_agent →
success → active → processing → screen-overwrite → idle → ready) and, within a
category, plain slice order. The `Priority` numbers in the built-ins are
decorative.

### Question 2: what does `version` mean?

`requirements.md` §1 defines it as "plugin schema/content version for the
author's own tracking" — optional, no behavior attached. Shipping a field with
literally no defined semantics invites two different users to assign it two
different meanings.

## Decision

### Schema v1

```toml
# Required. Unique across user plugin files. Identifies this plugin in logs.
id = "my-agent"

# Optional. Schema version. Absent or "1" means schema v1.
version = "1"

# Required, non-empty. Binary/program names this detector claims.
binary_names = ["my-agent", "my-agent-beta"]

# One or more. Order within a status category is match order.
[[patterns]]
name = "my_agent_thinking"        # required, non-empty
regex = "Thinking\\.\\.\\."       # required, must compile with Go regexp (RE2)
status = "processing"             # required, one of the ten status keys below
description = "my-agent is thinking"  # optional, surfaced in detection events
```

Valid `status` values — exactly the ten fields of `dtypes.StatusPatterns`
(`dtypes.go:15-26`), spelled with their existing snake_case tags:

`ready`, `processing`, `needs_approval`, `input_required`, `error`,
`tests_failing`, `idle`, `active`, `success`, `waiting_for_agent`

### `priority` is NOT in schema v1

Omitted deliberately. Accepting a `priority` key would promise ordering
control that `MatchLines` does not implement — the single most confusing
possible foot-gun for a plugin author debugging why their "higher priority"
pattern lost. With `DisallowUnknownFields()` (ADR-001), a file containing
`priority = 10` fails to load with a message naming the unknown key, which is
strictly better than silently ignoring it.

Ordering within a category is documented as "declaration order wins" — which
is the behavior `MatchLines` actually has.

If `Priority` is ever wired up in `MatchLines` for the built-ins, adding an
optional `priority` key to the schema is a purely additive change and remains
available.

### `version` semantics, reserved now

- Absent → treated as `"1"`.
- `"1"` → schema v1, parsed as above.
- Any other value → **rejected at load** with
  `unsupported schema version %q (this build supports "1")`.

Reserving the field's meaning now (rather than ignoring the value) is what
makes a future v2 parser branch possible without breaking v1 files. No
migration logic is built in this item — only the version gate.

### Evolution rules

- Adding a new **optional** key is free.
- Renaming or removing a key requires either a `version`-gated parser branch
  or a deprecation window that logs a loud warning while still accepting the
  old spelling.
- New `status` values may be added only if `dtypes.StatusPatterns` gains a
  matching field; the status table is generated from that struct's fields by
  hand and must be kept in lockstep (there is exactly one table, in
  `statusField`, so this is a single-site change).

### `id` vs `binary_names`

`id` is the plugin's identity for collision detection and log messages. It is
*not* used as a registry key. `binary_names` are the registry keys. A file may
declare several `binary_names`; each produces one `PluginDetector` whose
`Name()` returns that binary name, all sharing the file's compiled patterns
and carrying the file's `id` and path as provenance.

### Self-containment

A plugin file has no includes, no imports, and no path references of any kind.
Everything a detector needs is in the one file, so "share a detector" is
"send someone a file" — the lowest useful bar given remote distribution is an
explicit non-goal.

## Consequences

- The seeded example file (`example.toml.sample`) must enumerate all ten
  `status` values inline as comments. With no UI and no docs site, that file
  is the only schema discovery path a user has.
- The example is named `.toml.sample`, not `.toml`, specifically so the
  `*.toml` glob skips it — a fully-commented `.toml` would parse to an empty
  document and be rejected for a missing `id` on every scan.
- `description` being optional means detection events for plugin patterns may
  have an empty description; that field is already free-form in
  `PatternSet.MatchLines`'s return, so nothing downstream breaks.
