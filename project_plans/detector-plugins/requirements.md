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

Let users define new detectors via a TOML file dropped into the app's config
directory's `detectors/` subfolder (`~/.stapler-squad/detectors/` by
default; resolved via `config.GetConfigDir()` so it stays isolated under
`STAPLER_SQUAD_TEST_DIR`/`STAPLER_SQUAD_INSTANCE`, same as every other
per-instance state path), with no code change or rebuild required.

## Target User

Someone already running stapler-squad against an agent CLI that isn't one of
the 5 built-ins (`claude`, `gemini`, `aider`, `opencode`, `agy`) — a private
fork, an internal company tool, or a new public agent CLI that hasn't landed
a built-in detector yet — who wants working status detection today, not
after a stapler-squad release cycle. This is explicitly *not* aimed at
stapler-squad's committers (who can already just add a `binaries/*.go` file
and open a PR); it's aimed at the class of user for whom that path doesn't
exist (no write access to the fork, or the agent binary is private/internal
and can never be upstreamed).

## Success Metric

Since stapler-squad is a personal/small-team desktop tool (`Risk Control` in
`implementation/plan.md` — "no server fleet, no cohorts"), there is no
adoption dashboard to instrument against. The falsifiable success signal,
with a concrete checkpoint so it can actually fail rather than being
revisited indefinitely: **within 90 days of this feature shipping (checkpoint
date to be set in the shipping PR/changelog entry), the next non-built-in
agent CLI the author personally adopts gets a dropped-in
`~/.stapler-squad/detectors/*.toml` file, with zero commits to this repo.
(Scoped to the author's own usage, not "any stapler-squad user," because
there is genuinely no telemetry or usage-reporting mechanism to observe
other users' behavior — see `Risk Control` — so tracking a population this
doc cannot instrument would make the metric unfalsifiable in practice, the
same flaw being fixed here.) A community member independently reporting the
same outcome — via a backlog item, PR, or issue comment referencing a
plugin file instead of a `binaries/*.go` change — counts as corroborating,
bonus evidence, but is not required for the metric to resolve.** Owner of
the checkpoint: whoever ships this item (self-tracked via a backlog item or
calendar reminder set at ship time, since there is no PM/analytics team to
assign it to). If 90 days pass with no new agent onboarded either way, the
metric is inconclusive (not failed) and the checkpoint rolls to the next
new-agent event. If a new agent *is* onboarded in that window and
it still ships as a `binaries/*.go` PR instead of a `.toml` file, the demand
assumption below was wrong and the feature should not be extended further
(e.g. the deferred remote-manifest/issue-#178 work should not proceed, and
the cheaper alternative below should be tried instead).

## Risky Assumption

**Named, not yet validated:** that there exists real (not hypothetical)
demand for detecting non-built-in agents, sufficient to justify a TOML
schema, validator, and hot-reload watcher over the alternative of "just PR a
`binaries/*.go` file, it's a ~40-line addition." The originating backlog
item cites one GitHub issue as evidence; there is no usage data, user
request thread, or count of "I run a private agent CLI" reports backing it.
If this assumption is wrong, the cheaper true minimum is: keep detection
Go-only, and lower the bar for landing a new built-in detector PR (faster
review, a documented template) instead of building a parallel
user-extensible system. **That cheaper alternative was not tried before this
item was scoped** — this is a conscious sequencing choice, not an oversight:
the source backlog item already carries a "PLAN ADOPTION" verdict from prior
triage, and the cheaper alternative (faster-PR-review process) has no code
artifact this SDD pipeline can produce or validate, so testing it isn't
something a planning pass can do. It's recorded here so whoever owns the
Success Metric checkpoint has a concrete fallback in hand if the assumption
turns out wrong, rather than defaulting to "build more plugin features
instead." This item proceeds on the assumption as stated in the source
backlog item (`a8a2505e-ccaf-4120-ac2f-a77582853290`), but the Success
Metric above is the checkpoint for revisiting it.

## Scope (this item)

Local, file-based plugin loading only. Remote/distributed manifest fetching
(the herdr-style `manifest.go` / issue #178 pattern) is an explicit
non-goal — this item is the local foundation it would build on.

## Functional Requirements

1. **TOML schema** — a detector file declares:
   - `id` (string, required, unique) — detector identifier.
   - `binary_names` (list of strings, required, non-empty) — process/binary
     names this detector matches.
   - `version` (string, optional) — plugin schema version, validated against
     the set of versions this build supports (currently only `"1"`, or
     absent); a value naming an unsupported version is rejected at load with
     a clear error (ADR-003), it is not free-form author metadata.
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
