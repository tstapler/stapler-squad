# Requirements: User-Extensible Agent Detection Plugins (TOML-driven detectors)

Source: backlog item `a8a2505e-ccaf-4120-ac2f-a77582853290`, migrated from
`TylerStaplerAtFanatics/stapler-squad#182`.

## Problem

Agent detectors (`session/detection/binaries/*.go` — `claude.go`, `aider.go`,
`gemini.go`, `opencode.go`, `agy.go`) are hard-coded Go, registered in
`session/detection/registry.go`'s `DefaultRegistry()`. Each implements
`dtypes.BinaryDetector` and supplies a `StatusPatterns` set compiled by
`PatternSet` (`session/detection/pattern_set.go`) into regexes for status
categories (ready, processing, needsApproval, inputRequired, error,
testsFailing, idle, active, success, waitingForAgent).

Adding support for a new agent binary today requires a Go code change and a
stapler-squad release. Users running niche, internal, or forked agent CLIs
have no way to teach stapler-squad how to detect that agent's status from
its terminal output.

## Goal

Let users define new detectors via a TOML file dropped into
`~/.stapler-squad/detectors/`, with no code change or rebuild required.

## Scope (this item)

Local, file-based plugin loading only. Remote/distributed manifest fetching
(the herdr-style `manifest.go` / issue #178 pattern) is an explicit
non-goal — this item is the local foundation it would build on.

## Functional Requirements

1. **TOML schema** — a detector file declares:
   - `id` (string, required, unique) — detector identifier.
   - `binary_names` (list of strings, required, non-empty) — process/binary
     names this detector matches.
   - `version` (string, optional) — plugin schema/content version for the
     author's own tracking.
   - `[[patterns]]` blocks, each with `name`, `regex`, and `status` (must map
     to one of the existing `StatusPatterns` categories in
     `session/detection/pattern_set.go`).

2. **Loader** — on startup, scan `~/.stapler-squad/detectors/*.toml`, parse
   and validate each file, and construct a `dtypes.BinaryDetector` (or the
   equivalent `StatusPatterns` input) per valid file.

3. **Validation** — reject a plugin file with a clear, actionable error
   (which file, which field, why) when:
   - required fields are missing or empty,
   - a regex fails to compile,
   - `status` does not match a known category,
   - `id` or `binary_names` collide with another *user* plugin file.
   A single invalid file must not prevent other valid plugin files, or the
   built-in detectors, from loading (log-and-skip, not fail-fast for the
   whole process).

4. **Merge with built-ins** — user-defined plugins are merged into the
   registry alongside `DefaultRegistry()`'s built-in detectors. A user
   plugin whose `binary_names` matches a built-in detector's binary name
   takes precedence over that built-in for that binary.

5. **Hot-reload** — changes to files in `~/.stapler-squad/detectors/` (add,
   edit, remove) are picked up without restarting stapler-squad, via
   filesystem watching (fsnotify, already used elsewhere in the
   repo/ecosystem — confirm during research).

6. **Directory bootstrap** — `~/.stapler-squad/detectors/` is created (if
   absent) on first run, optionally seeded with a commented example file
   documenting the schema.

## Non-Functional Requirements

- No new detector-related Go code paths bypass the existing
  `PatternSet`/`StatusPatterns` compilation and matching pipeline — plugins
  produce the same shape of data the built-in detectors do today, so
  existing status-matching logic, tests, and snapshot fixtures keep working
  unmodified.
- Malformed or malicious plugin content has bounded blast radius: arbitrary
  regexes only (no code execution, no shell-out, no file/network access from
  plugin content).
- Regex compilation must not be able to hang the process (catastrophic
  backtracking) in a way that blocks startup indefinitely — document what
  protection Go's `regexp` package already provides (RE2, linear-time) vs.
  what if anything needs to be added (e.g., a max-pattern-length guard).

## Out of Scope

- Remote plugin distribution / manifest server (issue #178 territory).
- A plugin system for behavior beyond status detection (e.g., custom
  actions, custom UI) — this is detection-only, matching the maki-style
  "host exposes primitives" idea but scoped to regex-based status patterns.
- Any Lua/scripting runtime (maki's approach) — TOML only, per the herdr
  pattern this item adopts.

## Acceptance Criteria

1. A user can drop a TOML file into `~/.stapler-squad/detectors/` declaring
   `id`, `binary_names`, and `[[patterns]]` with `name`/`regex`/`status`,
   and stapler-squad detects that agent's status using those patterns
   without a rebuild or restart.
2. An invalid plugin file (bad regex, missing required field, unknown
   `status` value) is rejected with a clear error identifying the file and
   the problem, and does not prevent other detectors (built-in or other
   user plugins) from working.
3. A user plugin whose `binary_names` overlaps a built-in detector
   (e.g., `claude`) overrides the built-in for that binary name.
4. Editing or removing a plugin TOML file while stapler-squad is running is
   reflected in detection behavior without a restart.
5. `~/.stapler-squad/detectors/` is created automatically on first run if it
   doesn't exist.
6. Existing built-in detector tests and snapshot fixtures
   (`session/detection/*_test.go`, `testdata/`) continue to pass unmodified.

## Files Likely Touched (from backlog item + repo check)

- `session/detection/registry.go` — merge user plugins into
  `DefaultRegistry()` (or a new constructor wrapping it).
- `session/detection/plugins.go` (new) — TOML loader + validator.
- `session/detection/pattern_set.go` — reused as-is for compiling plugin
  patterns; confirm `StatusPatterns` field names map cleanly to TOML
  `status` strings.
- `session/detection/binaries/` — confirm plugin-constructed detectors
  satisfy `dtypes.BinaryDetector` the same way built-ins do.
- Hot-reload watcher (new, fsnotify-based).
- `~/.stapler-squad/detectors/` — created directory + example file.
