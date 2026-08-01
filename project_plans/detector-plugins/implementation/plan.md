# Implementation Plan: detector-plugins

**Feature**: Let users add agent status detectors by dropping a TOML file into `~/.stapler-squad/detectors/`, merged over the built-in detectors and hot-reloaded without a restart.
**Date**: 2026-08-01
**Status**: Ready for implementation
**ADRs**:
- [ADR-001](../decisions/ADR-001-go-toml-v2-for-plugin-parsing.md) — `github.com/pelletier/go-toml/v2` for plugin parsing
- [ADR-002](../decisions/ADR-002-registry-level-snapshot-not-statusdetector-yaml-path.md) — registry-level copy-on-write snapshot, not the existing `StatusDetector` YAML loader
- [ADR-003](../decisions/ADR-003-plugin-toml-schema-v1.md) — plugin TOML schema v1 (no `priority`, reserved `version`)
- [ADR-004](../decisions/ADR-004-plugin-trust-boundary-and-resource-caps.md) — RE2-only trust boundary, resource caps, no sandbox

**Scope note (read before starting)**: this is **backend-only Go work**. There
is no user-facing UI surface in the functional requirements, and
`research/ux.md` confirms it. Do **not** add any `web-app/` tasks, proto
changes, RPC, or registry (`docs/registry/features/`) entries — no RPC and no
React component is created or modified by this plan. Files touched are confined
to `session/detection/`, `main.go` (one call), and `go.mod`/`go.sum`.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| Detector Plugin | A user-authored TOML file declaring one detector's identity, the binary names it claims, and its status regexes. | Unit the user thinks in; one file. |
| Plugin Directory | `<config dir>/detectors`, resolved via `config.GetConfigDir()` + `"detectors"`. | `PluginDir() (string, error)` in `plugins.go`. Never `os.UserHomeDir()` directly — that would break `STAPLER_SQUAD_TEST_DIR`/`STAPLER_SQUAD_INSTANCE` isolation. |
| `pluginFile` | Unexported DTO the TOML decodes into: `ID`, `Version`, `BinaryNames`, `Patterns`. | `toml:"..."` tags. Never escapes the loader; converted to `dtypes.StatusPatterns`. |
| `patternEntry` | Unexported DTO for one `[[patterns]]` block: `Name`, `Regex`, `Status`, `Description`. | No `Priority` — see ADR-003. |
| Status Key | The snake_case string in a `[[patterns]]` block's `status` field. | Exactly the ten `dtypes.StatusPatterns` tags: `ready`, `processing`, `needs_approval`, `input_required`, `error`, `tests_failing`, `idle`, `active`, `success`, `waiting_for_agent`. |
| `statusField` | Table-driven lookup mapping a Status Key to a `*[]dtypes.StatusPattern` inside a `dtypes.StatusPatterns` value. | Single source of truth for valid Status Keys; also drives the error message and the example file. |
| `PluginDetector` | Exported struct implementing `dtypes.BinaryDetector` from a validated plugin file: one instance per (file × binary name). | `Name()` returns the binary name; `Patterns()` returns the compiled-from-TOML `StatusPatterns`; `FilterContent()` is the identity function. |
| `SourcePath` | Absolute path of the `.toml` file a `PluginDetector` came from. | Provenance for logs and debugging. |
| `PluginLoadError` | Value describing one rejected file: `Path`, `Field`, `Err`. | Accumulated, never returned as a single fatal error — requirement #3's log-and-skip contract. |
| `LoadPluginDir` | Scans the Plugin Directory, parses+validates every `*.toml`, returns `([]*PluginDetector, []PluginLoadError)`. | Directory-level read failure returns a single `PluginLoadError` with an empty detector slice. |
| `EnsurePluginDir` | Creates the Plugin Directory (0o755) if absent and writes the example seed file if it isn't there. | Requirement #6. |
| Example Seed File | `example.toml.sample` written into the Plugin Directory on bootstrap; fully commented, enumerates every Status Key. | `.sample` suffix keeps it out of the `*.toml` glob. |
| Built-in Detector | One of the five compiled-in detectors registered by `DefaultRegistry()` (`registry.go:6-14`). | `claude`, `gemini`, `aider`, `opencode`, `agy`. |
| `Upsert` | New non-panicking `DetectorRegistry` method: last write wins for a given binary name. | `Register` keeps its panic — that invariant is tested and must not be loosened. |
| `MergedRegistry` | Builds a fresh `*DetectorRegistry` from built-ins, then `Upsert`s user plugins over them. | User wins on binary-name collision (requirement #4). |
| Override | A user plugin claiming a binary name a built-in also claims; the plugin fully replaces the built-in for that name. | Total replacement, not pattern merging. |
| Collision | Two *user* plugin files sharing an `id` or a `binary_names` entry. | Rejected (both files' conflicting claim), unlike Override which is allowed. |
| `detectorSnapshot` | Immutable value holding `byBinary map[string]*StatusDetector` plus `provenance map[string]string`. | Replaces the `builtBinaryDetectors` package var. |
| `activeSnapshot` | `atomic.Pointer[detectorSnapshot]` holding the live snapshot. | Lock-free reads on the `DetectForProgram` hot path. |
| `snapshotWriteMu` | `sync.Mutex` serializing writers across the whole rebuild-then-store sequence. | Prevents lost updates between two rapid reloads. |
| Snapshot Swap | Building the entire next `detectorSnapshot` off to the side, then publishing it with one `Store`. | All-or-nothing; no half-applied reload is observable. |
| `buildSnapshot` | Compiles every detector in a `*DetectorRegistry` into a `detectorSnapshot` via `NewPatternSet`. | The only place plugin patterns get compiled — same path as built-ins. |
| `rebuildSnapshot` | Scans the Plugin Directory, merges, builds, and swaps a new snapshot; holds `snapshotWriteMu` throughout. | Called by `InitPlugins` and by the watcher. |
| `lookupBinaryDetector` | `func(program string) (*StatusDetector, bool)` — reads `activeSnapshot`. | Sole read accessor; replaces the bare map index at `detector.go:754`. |
| `DetectorProvenance` | `func() map[string]string` returning a copy of the live snapshot's provenance map. | Debug affordance: which file (or built-in) currently claims each binary name. |
| `InitPlugins` | `func(ctx context.Context) error` — bootstrap dir, first `rebuildSnapshot`, start watcher. | The explicit startup entry point; called once from `main.go`. |
| `PluginWatcher` | fsnotify watcher over the Plugin Directory, owning one event-loop goroutine. | Exposes `Stopped() <-chan struct{}`, matching `session/unfinished/watcher.go`'s convention. |
| `pluginReloadDebounce` | `200 * time.Millisecond` — timer reset on each filesystem event, fires one reload once events settle. | A single editor save emits 4–12 raw events. |
| `pluginRescanInterval` | `60 * time.Second` — unconditional periodic rebuild, independent of fsnotify. | Safety net mirroring `WatchDirWatcher.periodicReWalk`. |
| `maxPatternsPerPlugin` | 50 — per-file pattern count cap. | ADR-004. |
| `maxRegexLength` | 4096 bytes — per-`regex` length cap. | ADR-004. |
| `maxPluginFileSize` | 256 KiB — per-file read cap, enforced by stat before read. | ADR-004. |

**30 terms.**

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| **Overall architecture** | Registry-level copy-on-write snapshot rebuilt from a directory scan | Own synthesis; precedent `session/pipeline_engine.go:123-140` | (Creative alt A) Extend the existing unused `StatusDetector.LoadPatterns` YAML path to accept TOML | `StatusDetector` has no concept of `id`/`binary_names`/registry membership, so requirement #4's binary-name override and requirement #1's multi-binary files can't be expressed on it; its `atomic.Pointer[PatternSet]` also gives per-detector atomicity where reload is a whole-set operation. See ADR-002. |
| **Overall architecture** | (as above) | (as above) | (Creative alt C) Mutex-guarded lazy map + `os.Stat`/reparse of the plugin file at detection time, no watcher | Puts a mutex *and* a filesystem stat on `DetectForProgram`, effectively a per-PTY-read hot path — exactly the contention profile `docs/adr/011-prefer-lock-free-concurrency.md` exists to prevent. Saving one goroutine is not worth it. |
| TOML file → domain object | Data Transfer Object + Parse-Don't-Validate (`pluginFile`/`patternEntry` → validated `*PluginDetector`) | PoEAA (DTO); type-driven-design (Parse, Don't Validate) | Decode TOML straight into `dtypes.StatusPatterns` | The wire shape (a flat `[[patterns]]` array with a `status` discriminator) is deliberately *not* the domain shape (ten typed slices). A DTO gives per-field error messages naming `patterns[2].regex`, which decoding into the domain struct cannot. It also confines the go-toml dependency to one function (ADR-001). |
| Status Key → `StatusPatterns` field | Table-driven lookup returning `*[]dtypes.StatusPattern` | Go idiom (table-driven dispatch) | Reflection over `dtypes.StatusPatterns`' `yaml:"..."` struct tags | Reflection would silently couple the TOML schema to YAML tags meant for a different loader, and would break invisibly if a tag were renamed. The explicit table is compile-checked, greppable, and enumerable — it also *produces* the "valid values are: ..." error text and the example file's comment block. |
| Registry composition | Registry (PoEAA) + fresh-build-per-reload | PoEAA (Registry) | GoF Decorator / Chain of Responsibility wrapping built-in detectors with plugin detectors | There is no cross-cutting behavior to layer. Override is *total replacement* keyed by binary name, which is one map write, not a delegation chain. A chain would also make "which detector matched" ambiguous. |
| Registry override path | New non-panicking `Upsert` method alongside the existing `Register` | Go idiom (distinct method for distinct contract) | Loosen `Register` to overwrite instead of panic | `Register`'s panic-on-duplicate is a tested invariant (`registry_test.go`'s `TestDetectorRegistry_should_panic_When_duplicateNameRegistered`) protecting the built-ins from an accidental double-registration. Two contracts, two methods. |
| Live-registry concurrency | Copy-on-Write via `atomic.Pointer[detectorSnapshot]` | `docs/adr/011-prefer-lock-free-concurrency.md`; precedent `StatusDetector.patternSet` (`detector.go:49`) | `sync.RWMutex` around the map, or `xsync.MapOf` (already a dependency) | Reads dominate writes by many orders of magnitude, and the required atomicity is over the *whole set* (a reload adds, edits, and removes entries together) — a concurrent map gives per-key atomicity, which permits a half-applied reload to be observed. |
| Reload write serialization | Writer mutex held across the entire read-modify-write, not just the `Store` | Precedent `session/pipeline_engine.go`'s `pipelineModeCache` (`:123-140`, `refresh` `:162-189`) | Mutex around `Store` only | A slower-starting-but-slower-finishing rebuild can land stale data after a faster one — a lost update. Two rapid saves of the same file is exactly that race. |
| Loader error handling | Collecting Parameter (accumulate `[]PluginLoadError`) + log-and-skip | Prometheus `rule_files` reload contract (keep last-known-good, log and skip the bad one) | Fail-fast returning the first error | Requirement #3 mandates that one invalid file must not prevent other valid plugin files or the built-ins from loading. |
| Watcher | Single-owner event-loop goroutine + debounce timer + periodic re-scan safety net | Precedents `session/unfinished/watcher.go` (`fsnotifyLoop` `:145-188`, `periodicReWalk`), `session/unfinished/gogitstore/mmapwatch.go` (`packWatchLoop`), `session/external_tmux_streamer.go:358-382` (`debounceCaptures`) | fsnotify without debounce; or polling only | A single editor save emits 4–12 raw events in ~20ms, so undebounced means 4–12 full rebuilds. Polling-only means up to a full interval of latency and violates the spirit of requirement #5. The periodic re-scan is kept *in addition*, because fsnotify has enough platform edge cases (macOS FSEvents batching, atomic-rename saves) that a 60s backstop is cheap insurance. |
| Startup wiring | Initialization Method (`InitPlugins(ctx)`) called explicitly from `main.go` | Go idiom (explicit init over package-init magic) | Keep the package-level `var builtBinaryDetectors = func(){...}()` IIFE and extend it | Package init runs before `main()` — before config resolution and logging init, with no `context.Context` for a watcher goroutine's lifetime. Requirement #6 (bootstrap the directory *before* the first scan) is unimplementable there. |
| Plugin content capability | Identity `FilterContent`; no per-plugin content filter hook | ADR-004 trust boundary | Allow a `filter_regex` / `filter` key in the TOML | Every capability beyond "match a regex against text" widens the trust boundary for no requirement. `requirements.md` §Out of Scope rules out behavior beyond status detection. |

---

## Migration Plan

**Not applicable.** This feature adds no database schema change — no `ent`
schema edit, no migration, no `session/ent/` regeneration. All new state is a
directory of user-authored files on disk plus in-memory structures. The
`~/.stapler-squad/detectors/` directory is created on first run and is empty by
default, so there is no data to migrate for existing installations.

---

## Observability Plan

All logging uses `github.com/tstapler/stapler-squad/log`, already imported by
`session/detection/idle.go:10` (so no new import cycle: `go list -deps ./config`
confirms `config` pulls in only `safeexec`, `executor`, `log`, and itself — it
does not import `session/detection`, so `session/detection` may import
`config`).

- **Logs**
  - `log.Info("detector plugins loaded", "dir", dir, "plugins", n, "binaries", names, "rejected", len(errs))` — once per successful `rebuildSnapshot`.
  - `log.Warn("detector plugin rejected", "file", path, "field", field, "err", err)` — once per `PluginLoadError`. Message must name **file, field, and reason** (requirement #3). Field is a path expression like `patterns[2].regex` or `binary_names`.
  - `log.Info("detector plugin overrides built-in", "binary", name, "file", path)` — once per Override, at load, so a user who accidentally shadows `claude` sees it.
  - `log.Warn("detector plugin skipped: symlinks are not followed", "file", path)` — ADR-004 §3.
  - `log.Warn("detector plugin directory scan failed", "dir", dir, "err", err)` — directory-level failure; previous snapshot retained, not swapped.
  - `log.Debug("detector plugin reload triggered", "reason", "fsnotify"|"periodic", "event", ev)` — reload provenance.
  - `log.Warn("fsnotify unavailable for detector plugins, falling back to periodic rescan", "err", err)` — soft-fail path, matching `NewWatchDirWatcher`'s existing convention.
- **Metrics**: none added. The repo has no counter/gauge facility in
  `session/detection`, and this feature's cardinality (a handful of files,
  reloads measured in per-hour) does not justify introducing one. The
  `DetectorProvenance()` accessor covers the "what is loaded right now"
  question that a gauge would answer.
- **Alerts**: none. This is a local, single-user desktop daemon; there is no
  alerting pipeline to attach to. The failure mode (a plugin doesn't load) is
  surfaced by the `log.Warn` lines above and by `DetectorProvenance()`.

---

## Risk Control

- **Feature flag**: `STAPLER_SQUAD_DISABLE_DETECTOR_PLUGINS=1` — an
  environment-variable kill switch checked at the top of `InitPlugins`. When
  set, `InitPlugins` returns `nil` immediately without creating the directory,
  scanning, or starting a watcher, leaving the built-ins-only snapshot
  installed by package `init()` in place. Deliberately **not** a
  `config.GetFeatureFlag` entry: that returns `false` for any unset key
  (`config/config.go:999-1004`), which would leave the feature off by default
  and break acceptance criteria #1 and #5.
- **Rollback procedure**: three escalating levels, no rebuild needed for the
  first two.
  1. Delete or rename the offending `.toml` file — the watcher rebuilds within
     `pluginReloadDebounce` and the built-in reclaims the binary name.
  2. Set `STAPLER_SQUAD_DISABLE_DETECTOR_PLUGINS=1` in the systemd unit and
     restart — reverts to exactly today's behavior. (Per
     `.claude/rules/tmux-keep-server-on-restart.md`, confirm the unit passes
     `--tmux-keep-server` before restarting.)
  3. Revert the commit. The only change to pre-existing behavior is
     `DetectForProgram`'s map read becoming `lookupBinaryDetector`, and the
     built-ins-only `init()` snapshot makes that a no-op when no plugin ever
     loads — so a revert is low-risk and mechanical.
- **Staged rollout**: not applicable in the usual sense (no server fleet, no
  cohorts). The staging *is* the design: with an empty plugin directory — the
  state of every existing installation on upgrade — behavior is byte-identical
  to today, because the `init()`-installed snapshot is built from
  `DefaultRegistry()` alone. Users opt in one file at a time, and a single bad
  file affects only the binary names it claims.

---

## Hardening Addendum (Triad Review, 2026-08-01)

The engineering-lens triad review independently re-surfaced five of
`adversarial-review.md`'s open Concerns (not Blockers) as readiness gaps,
and its second pass flagged that an earlier version of this addendum only
described the fixes here rather than editing the actual tasks — that has
been corrected: **each fix below is now written directly into its owning
task**, not just summarized here. This section is now an index, not the
source of truth:

| # | Gap | Fixed in |
|---|-----|----------|
| 1 | Aggregate regex compile time unbounded within per-pattern caps (6.1s empirically) | Task 1.2.1b (`validatePluginFile`, `maxPluginCompileTime`) + boundary test in Task 1.2.1c |
| 2 | Seed-file-write failure aborts the whole `InitPlugins` flow | Task 2.3.1a (`EnsurePluginDir`) |
| 3 | No cap on total plugin file count | Task 1.2.3c (`LoadPluginDir`, `maxPluginFiles`) + test in Task 1.2.3d |
| 4 | `rebuildSnapshot` can't be cancelled mid-rebuild | Task 2.2.2a (added `ctx` param), call sites updated in Task 2.3.2a (`InitPlugins`) and Task 2.3.3c (`watchLoop`) |
| 5 | `InitPlugins` has no re-entrancy guard | Task 2.3.2a (`initPluginsOnce sync.Once`) |
| 6 | Task time estimates (2-5 min) read as human-hour sizing but aren't | Clarification only, no task change: every `##### Task` estimate here is sized for LLM-subagent execution (`subagent-driven-development`), not human wall-clock |

These are additive constraints on tasks already in Phases 1-2, not new
stories — the Dependency Visualization and critical path are unchanged
except where noted (`rebuildSnapshot`'s signature gaining a `ctx`
parameter, item 4, is a same-task change, not a new node in the graph).

---

## Unresolved Questions

None. The four questions flagged by research are resolved and recorded:

- Build on the existing unused `NewStatusDetectorFromFile`/`LoadPatterns` YAML
  machinery? **No** — reuse `PatternSet`/`NewPatternSet` (the compile step)
  only, and build new swap machinery at the registry level. ADR-002.
- Does `builtBinaryDetectors` need to become an explicit init function?
  **Yes, required** — it becomes `activeSnapshot` +
  `lookupBinaryDetector`, initialized by package `init()` with built-ins and
  re-populated by `detection.InitPlugins(ctx)`, called from `main.go`'s root
  `RunE` between `log.InitializeWithConfig(...)` (`main.go:148`) and the
  `if daemonFlag` branch (`main.go:177`). ADR-002.
- Expose `priority` in the TOML schema? **No** — it is a dead field in
  `PatternSet.compile()`/`MatchLines()`. ADR-003.
- Does the plugin loader also feed the generic `NewStatusDetector()` /
  `getDefaultPatterns()` path? **Superseded — see below.** ADR-002's original
  answer ("No — `session/claude_controller.go` deliberately untouched") is
  reversed by the pre-mortem finding recorded 2026-08-01 (P1 #1,
  `pre-mortem.md`): `DetectForProgram`/`lookupBinaryDetector` has **zero**
  production call sites anywhere in the repo (`grep -rln DetectForProgram
  --include='*.go' .` matches only `detector.go` itself and its own tests).
  The live status badge a user actually sees is produced by
  `session/claude_controller.go:221-228`, which constructs a program-agnostic
  `sd := detection.NewStatusDetector()` (Claude-only `getDefaultPatterns()`)
  and hands it to `NewIdleDetectorWithDetector`. Without a change there, a
  user's override or new-binary plugin can load, validate, pass every unit
  test in `validation.md`, and appear in `DetectorProvenance()` — while never
  once changing what the web UI shows, which directly falsifies acceptance
  criterion #1. **Resolution: option (b), this plan's scope is expanded** —
  see new Epic 2.4 below — rather than (a) deferring to a follow-up item or
  (c) relabeling the shipped feature as invisible-by-design; the wiring is a
  ~3-line change at a single, already-identified call site, so deferring it
  would cost more in follow-up-item overhead than it saves.

---

## Dependency Visualization

```
                        main.go  RunE (after log init, before daemonFlag branch)
                             │
                             ▼
                    detection.InitPlugins(ctx)          [Story 2.3.2]
                             │
             ┌───────────────┼────────────────────────┐
             ▼               ▼                        ▼
      EnsurePluginDir   rebuildSnapshot(dir)   StartPluginWatcher(ctx)
        [2.3.1]              │                     [2.3.3]
                             │                        │  fsnotify events
             ┌───────────────┴──────────┐             │  + 60s ticker
             ▼                          ▼             └──► debounce 200ms
       LoadPluginDir(dir)        MergedRegistry(          └──► rebuildSnapshot
          [1.2.3]                  DefaultRegistry(),
             │                      plugins)  [2.1.2]
   ┌─────────┴─────────┐                 │
   ▼                   ▼                 ▼ Upsert  [2.1.1]
 parsePluginFile  validatePluginFile   *DetectorRegistry
   [1.1.1]            [1.2.1/1.2.2]           │
      │                   │                   ▼
      ▼                   ▼            buildSnapshot        [2.2.1]
  pluginFile DTO     statusField              │  NewPatternSet (UNCHANGED)
   [1.1.1]            [1.1.2]                 ▼
                                       detectorSnapshot
                                              │ atomic Store (writeMu held)
                                              ▼
                                       activeSnapshot
                                              │
                                              ▼ Load (lock-free)
                                    lookupBinaryDetector    [2.2.1]
                                              │
                                              ▼
                                   DetectForProgram (detector.go:753)
```

Critical path: 1.1.1 → 1.1.2 → 1.2.1 → 1.2.3 → 2.1.1 → 2.1.2 → 2.2.1 → 2.2.2 → 2.3.1 → 2.3.2 → 2.3.3 → 2.4.1.
Epics 1.1 and 1.2 are pure functions with no globals, so 2.1.1 (`Upsert`) can be
written in parallel with all of Phase 1. Epic 2.4 (production wiring) depends
only on 2.2.1 (`lookupBinaryDetector`) and can land any time after it — it does
not depend on 2.3 (bootstrap/watcher), since the built-ins-only snapshot from
package `init()` already makes `lookupBinaryDetector` safe to call before
`InitPlugins` ever runs.

---

## Phase 1: Parse and Validate

### Epic 1.1: TOML schema and parsing
**Goal**: Turn the bytes of one `.toml` file into a validated in-memory DTO,
with field-level error messages, using a dependency confined to one function.

#### Story 1.1.1: Add go-toml and decode a plugin file into a DTO
**As a** plugin author, **I want** my TOML file's fields read into the loader,
**so that** stapler-squad can act on what I declared.
**Acceptance Criteria**:
- A well-formed file decodes into a `pluginFile` with all fields populated.
  - *Given* a file containing `id = "my-agent"`, `version = "1"`,
    `binary_names = ["my-agent"]`, and one `[[patterns]]` block with
    `name = "my_agent_thinking"`, `regex = "Thinking\\.\\.\\."`,
    `status = "processing"`, `description = "my-agent is thinking"`,
    *When* `parsePluginFile` is called on those bytes,
    *Then* it returns `&pluginFile{ID: "my-agent", Version: "1", BinaryNames: []string{"my-agent"}, Patterns: []patternEntry{{Name: "my_agent_thinking", Regex: `Thinking\.\.\.`, Status: "processing", Description: "my-agent is thinking"}}}` and a nil error.
- An unknown key is a loud error, not a silent no-op.
  - *Given* a file whose key is `binary_name = ["my-agent"]` (singular typo),
    *When* `parsePluginFile` is called,
    *Then* it returns an error whose message contains `binary_name`.
- A `priority` key is rejected rather than ignored (ADR-003).
  - *Given* a `[[patterns]]` block containing `priority = 10`,
    *When* `parsePluginFile` is called,
    *Then* it returns an error whose message contains `priority`.
**Files**: `go.mod`, `go.sum`, `session/detection/plugins.go` (new), `session/detection/plugins_test.go` (new)

##### Task 1.1.1a: Add the go-toml v2 dependency (~2 min)
- `go get github.com/pelletier/go-toml/v2@latest`
- `go mod tidy`
- Confirm it lands as a direct `require` (not `// indirect`) in `go.mod`.
- Do **not** touch `deps_guard_test.go` — go-toml is a wanted dependency, and
  that file's `forbiddenDeps` list is only for banned ones.
- Files: `go.mod`, `go.sum`

##### Task 1.1.1b: Define the DTO types and package doc (~4 min)
- Create `session/detection/plugins.go` in `package detection`.
- Define unexported `patternEntry{Name, Regex, Status, Description string}` with
  `toml:"name"`, `toml:"regex"`, `toml:"status"`, `toml:"description"` tags.
- Define unexported `pluginFile{ID, Version string; BinaryNames []string; Patterns []patternEntry}`
  with `toml:"id"`, `toml:"version"`, `toml:"binary_names"`, `toml:"patterns"` tags.
- Add a file-level doc comment stating: schema v1 per ADR-003, no `priority`
  key, and that these DTOs never escape the loader.
- Files: `session/detection/plugins.go`

##### Task 1.1.1c: Implement `parsePluginFile` with `DisallowUnknownFields` (~4 min)
- `func parsePluginFile(path string, data []byte) (*pluginFile, error)`.
- Decode with `toml.NewDecoder(bytes.NewReader(data))`, calling
  `.DisallowUnknownFields()` on the decoder before `Decode`.
- Wrap decode failures as `fmt.Errorf("failed to parse detector plugin %s: %w", path, err)`,
  matching the wrapping style of `PatternSet.compile` (`pattern_set.go:56-59`).
- Files: `session/detection/plugins.go`

##### Task 1.1.1d: Table tests for `parsePluginFile` (~5 min)
- Create `session/detection/plugins_test.go`.
- Cases, named `parsePluginFile_should_<effect>_When_<condition>`: full valid
  file; `binary_name` typo; `priority` key present; syntactically invalid TOML
  (`id = ` with no value); empty file.
- Use `t.TempDir()` only where a real path is needed; `parsePluginFile` itself
  takes bytes, so most cases need no filesystem.
- Files: `session/detection/plugins_test.go`

---

#### Story 1.1.2: Map status keys onto `dtypes.StatusPatterns` fields
**As a** plugin author, **I want** `status = "needs_approval"` to land in the
same bucket the built-in `claude` detector uses, **so that** my patterns get the
identical priority chain and status semantics.
**Acceptance Criteria**:
- Every one of the ten status keys resolves to its `dtypes.StatusPatterns` field.
  - *Given* `status = "waiting_for_agent"`,
    *When* `statusField` is called on a zero `dtypes.StatusPatterns`,
    *Then* it returns a pointer to that value's `WaitingForAgent` slice and `true`.
- An unknown status key is rejected with the full list of valid values.
  - *Given* `status = "processsing"` (typo),
    *When* `statusField` is called,
    *Then* it returns `(nil, false)`, and the caller's error message contains
    `processsing` and all ten valid keys `ready, processing, needs_approval,
    input_required, error, tests_failing, idle, active, success,
    waiting_for_agent`.
**Files**: `session/detection/plugins.go`, `session/detection/plugins_test.go`

##### Task 1.1.2a: Implement `statusField` and `validStatusKeys` (~4 min)
- `func statusField(p *dtypes.StatusPatterns, status string) (*[]dtypes.StatusPattern, bool)` —
  a `switch status` returning `&p.Ready`, `&p.Processing`, `&p.NeedsApproval`,
  `&p.InputRequired`, `&p.Error`, `&p.TestsFailing`, `&p.Idle`, `&p.Active`,
  `&p.Success`, `&p.WaitingForAgent`, keyed by the snake_case names from
  `dtypes.go:15-26`.
- `var validStatusKeys = []string{...}` in that same order, used to build error
  text and (Task 2.3.1b) the example file's comment block.
- Doc comment: this table and `dtypes.StatusPatterns` must be changed together.
- Files: `session/detection/plugins.go`

##### Task 1.1.2b: Implement `toStatusPatterns` (~3 min)
- `func toStatusPatterns(pf *pluginFile) (dtypes.StatusPatterns, error)` —
  iterate `pf.Patterns` in declaration order, resolve each entry's target slice
  via `statusField`, append
  `dtypes.StatusPattern{Name: e.Name, Pattern: e.Regex, Description: e.Description}`.
- Leave `Priority` at its zero value and note in a comment that
  `PatternSet.MatchLines` (`pattern_set.go:69-141`) never reads it — ADR-003.
- Files: `session/detection/plugins.go`

##### Task 1.1.2c: Tests for status mapping (~4 min)
- One subtest per status key asserting the pattern lands in the right slice.
- One test asserting declaration order is preserved within a category (two
  `processing` patterns, `first` then `second`, land at index 0 and 1 of
  `Processing`).
- One test asserting the unknown-status error text lists all ten valid keys.
- Files: `session/detection/plugins_test.go`

---

### Epic 1.2: Validation and directory loading
**Goal**: Turn a directory of files into a list of valid `*PluginDetector`s
plus a list of precisely-described rejections, never a fatal error.

#### Story 1.2.1: Validate a single plugin file
**As a** plugin author who made a mistake, **I want** an error naming my file,
the offending field, and why, **so that** I can fix it without guessing.
**Acceptance Criteria**:
- A missing required field is rejected by name.
  - *Given* `/tmp/detectors/noid.toml` containing only `binary_names = ["my-agent"]`
    and one valid `[[patterns]]` block,
    *When* `validatePluginFile` runs,
    *Then* it returns a `PluginLoadError{Path: "/tmp/detectors/noid.toml", Field: "id"}`
    whose error text reads `id is required and must be non-empty`.
- An empty `binary_names` is rejected.
  - *Given* `binary_names = []`,
    *When* `validatePluginFile` runs,
    *Then* `Field` is `binary_names` and the text reads
    `binary_names is required and must contain at least one name`.
- A regex that does not compile is rejected with the compile error attached.
  - *Given* a `[[patterns]]` block with `regex = "Thinking(\\.\\.\\."` (unclosed group),
    *When* `validatePluginFile` runs,
    *Then* `Field` is `patterns[0].regex` and the wrapped error is the
    `regexp.Compile` error `error parsing regexp: missing closing ): ` + "`Thinking(\\.\\.\\.`".
- An unsupported `version` is rejected (ADR-003).
  - *Given* `version = "2"`,
    *When* `validatePluginFile` runs,
    *Then* `Field` is `version` and the text reads
    `unsupported schema version "2" (this build supports "1")`.
  - *Given* no `version` key at all,
    *When* `validatePluginFile` runs,
    *Then* it is accepted (absent means `"1"`).
**Files**: `session/detection/plugins.go`, `session/detection/plugins_test.go`

##### Task 1.2.1a: Define `PluginLoadError` (~2 min)
- `type PluginLoadError struct { Path string; Field string; Err error }`
- `func (e PluginLoadError) Error() string` →
  `fmt.Sprintf("detector plugin %s: field %s: %v", e.Path, e.Field, e.Err)`
- `func (e PluginLoadError) Unwrap() error { return e.Err }`
- Files: `session/detection/plugins.go`

##### Task 1.2.1b: Implement `validatePluginFile` (~5 min)
- `func validatePluginFile(path string, pf *pluginFile) []PluginLoadError` —
  accumulate *all* problems in the file rather than returning on the first, so
  a user fixing three typos sees three messages, not three edit-run cycles.
- Checks in order: `version` gate; `id` non-empty; `binary_names` non-empty and
  each entry non-empty; at least one `[[patterns]]`; per pattern: `name`
  non-empty, `status` resolves via `statusField`, `regex` non-empty and
  `regexp.Compile`s.
- **Wall-clock compile budget (Hardening Addendum item 1, corrected
  location — not Task 1.1.2b as originally noted, since `regexp.Compile` is
  called here, in `validatePluginFile`, not in `toStatusPatterns`)**: track
  elapsed time across the per-pattern `regexp.Compile` loop with
  `const maxPluginCompileTime = 500 * time.Millisecond`; if cumulative
  compile time for the file exceeds it, stop compiling further patterns and
  append `PluginLoadError{Path: path, Field: "patterns", Err:
  fmt.Errorf("compiling took longer than %s, rejected", maxPluginCompileTime)}`
  — same log-and-skip contract as every other rejection, verified in
  `adversarial-review.md`'s empirical case (50 patterns of
  `(4000-byte-literal){500}` measured at 6.1s).
- Field names use path expressions: `id`, `version`, `binary_names`,
  `binary_names[1]`, `patterns`, `patterns[0].name`, `patterns[0].status`,
  `patterns[0].regex`.
- Files: `session/detection/plugins.go`

##### Task 1.2.1c: Tests for per-file validation (~5 min)
- One case per acceptance criterion above, asserting on `Field` exactly and on
  `Error()` containing the required substring.
- One case asserting a file with three separate problems yields three
  `PluginLoadError`s.
- One case asserting the `maxPluginCompileTime` budget: 50 patterns (at the
  `maxPatternsPerPlugin` cap) each shaped as `(4000-byte-literal){500}` (at
  the `maxRegexLength` cap) is rejected on `Field: "patterns"` rather than
  hanging the test — this is the adversarial-review boundary case
  (cap-compliant but expensive), generated programmatically with
  `strings.Repeat`, not a fixture file.
- Files: `session/detection/plugins_test.go`

---

#### Story 1.2.2: Enforce resource caps
**As an** operator, **I want** pathological plugin files rejected cheaply,
**so that** a pasted machine-generated file can't quietly slow every PTY read.
**Acceptance Criteria**:
- A file with more than 50 patterns is rejected.
  - *Given* a file with 51 `[[patterns]]` blocks,
    *When* `validatePluginFile` runs,
    *Then* `Field` is `patterns` and the text reads
    `51 patterns exceeds the per-file limit of 50`.
- A regex longer than 4096 bytes is rejected.
  - *Given* `patterns[0].regex` of 4097 bytes,
    *When* `validatePluginFile` runs,
    *Then* `Field` is `patterns[0].regex` and the text reads
    `regex length 4097 exceeds the limit of 4096 bytes`.
- A file larger than 256 KiB is rejected without being read into memory.
  - *Given* a 300 KiB `huge.toml`,
    *When* `LoadPluginDir` scans the directory,
    *Then* a `PluginLoadError` with `Field: "file"` and text
    `file size 307200 exceeds the limit of 262144 bytes` is returned, and
    `os.ReadFile` was never called on it (enforced by stat-then-read ordering).
**Files**: `session/detection/plugins.go`, `session/detection/plugins_test.go`

##### Task 1.2.2a: Add the cap constants and count/length checks (~3 min)
- `const maxPatternsPerPlugin = 50`, `maxRegexLength = 4096`,
  `maxPluginFileSize = 256 * 1024` with a doc comment pointing at ADR-004
  (these are hygiene bounds, not ReDoS protection — RE2 forecloses that).
- Wire the first two into `validatePluginFile`.
- Files: `session/detection/plugins.go`

##### Task 1.2.2b: Tests for the caps (~4 min)
- Generate the 51-pattern and 4097-byte cases programmatically (`strings.Repeat`),
  not as fixture files.
- Files: `session/detection/plugins_test.go`

---

#### Story 1.2.3: Scan the directory, detect collisions, build detectors
**As a** user with several plugins, **I want** one bad file to be skipped and
the rest to load, **so that** a typo doesn't disable everything.
**Acceptance Criteria**:
- Valid files load; invalid ones are reported and skipped.
  - *Given* a directory containing valid `my-agent.toml` (id `my-agent`,
    binary `my-agent`) and invalid `broken.toml` (regex `Thinking(\.\.\.`),
    *When* `LoadPluginDir(dir)` runs,
    *Then* it returns one `*PluginDetector` with `Name() == "my-agent"` and
    exactly one `PluginLoadError` whose `Path` ends in `broken.toml`.
- One file with several binary names yields one detector per name, sharing patterns.
  - *Given* `binary_names = ["my-agent", "my-agent-beta"]` and one `processing`
    pattern `Thinking\.\.\.`,
    *When* `LoadPluginDir(dir)` runs,
    *Then* it returns two `*PluginDetector`s with `Name()` `my-agent` and
    `my-agent-beta`, both with `SourcePath()` equal to that file and
    `Patterns().Processing[0].Pattern == "Thinking\\.\\.\\."`.
- Two user files claiming the same `id` are both rejected on that field.
  - *Given* `a.toml` and `b.toml` both with `id = "my-agent"`,
    *When* `LoadPluginDir(dir)` runs,
    *Then* `b.toml` (the later filename in sorted order) yields a
    `PluginLoadError{Field: "id"}` reading
    `duplicate id "my-agent" (already declared by a.toml)`, and `a.toml` loads.
- Two user files claiming the same binary name are rejected on that field.
  - *Given* `a.toml` with `binary_names = ["my-agent"]` and `z.toml` with
    `binary_names = ["my-agent"]`,
    *When* `LoadPluginDir(dir)` runs,
    *Then* `z.toml` yields `PluginLoadError{Field: "binary_names"}` reading
    `duplicate binary name "my-agent" (already claimed by a.toml)`, and
    `a.toml`'s detector is returned.
- Collision winners are deterministic across runs.
  - *Given* the same two colliding files,
    *When* `LoadPluginDir` is called ten times,
    *Then* `a.toml` wins every time (files are processed in `os.ReadDir`'s
    filename order, which is sorted).
- Non-`.toml` entries, subdirectories, and symlinks are skipped.
  - *Given* a directory containing `example.toml.sample`, a subdirectory
    `archive/`, and a symlink `linked.toml -> /etc/passwd`,
    *When* `LoadPluginDir(dir)` runs,
    *Then* none of the three produces a detector, and only the symlink produces
    a log line (`detector plugin skipped: symlinks are not followed`).
- A missing directory is not an error.
  - *Given* `dir` does not exist,
    *When* `LoadPluginDir(dir)` runs,
    *Then* it returns `(nil, nil)` — zero detectors, zero errors.
**Files**: `session/detection/plugins.go`, `session/detection/plugins_test.go`

##### Task 1.2.3a: Implement `PluginDetector` (~3 min)
- `type PluginDetector struct { id, sourcePath, binaryName string; patterns dtypes.StatusPatterns }`
- `Name() string` → `binaryName`; `Patterns() dtypes.StatusPatterns` → `patterns`;
  `FilterContent(content string) string` → `content` (identity, per ADR-004);
  plus exported `ID()` and `SourcePath()` accessors for provenance.
- Add a compile-time assertion: `var _ dtypes.BinaryDetector = (*PluginDetector)(nil)`.
- Files: `session/detection/plugins.go`

##### Task 1.2.3b: Implement `PluginDir` (~2 min)
- `func PluginDir() (string, error)` → `config.GetConfigDir()` then
  `filepath.Join(cfgDir, "detectors")`.
- Comment why it isn't `os.UserHomeDir()`: `STAPLER_SQUAD_TEST_DIR` /
  `STAPLER_SQUAD_INSTANCE` isolation (`config/config.go:117-131`).
- Confirm no import cycle by running `go build ./session/detection/`.
- Files: `session/detection/plugins.go`

##### Task 1.2.3c: Implement `LoadPluginDir` (~5 min)
- `func LoadPluginDir(dir string) ([]*PluginDetector, []PluginLoadError)`.
- `os.ReadDir(dir)`; on `os.IsNotExist` return `(nil, nil)`; on any other error
  return one `PluginLoadError{Path: dir, Field: "directory"}`.
- Skip entries that are directories, that don't end in `.toml`, or whose
  `os.Lstat` mode has `os.ModeSymlink` (log the symlink case).
- **Total-file-count cap (Hardening Addendum item 3)**: `const
  maxPluginFiles = 200`; after filtering to `.toml` entries (sorted by
  `os.ReadDir`), process only the first `maxPluginFiles` and append one
  `PluginLoadError{Path: dir, Field: "file_count", Err: fmt.Errorf("directory
  contains more than %d .toml files, remainder skipped", maxPluginFiles)}`
  for the rest. **Field is deliberately `"file_count"`, not `"directory"`**
  — `rebuildSnapshot` (Task 2.2.2a) treats a `Field == "directory"` error as
  a fatal "the whole scan failed, keep the previous snapshot" signal; the
  count cap is a successful, partial scan (200 detectors still load fine)
  and must not trip that fatal path.
- Per file: size check (`maxPluginFileSize`) → `os.ReadFile` →
  `parsePluginFile` → `validatePluginFile` (now with the compile-time budget
  from Task 1.2.1b) → `toStatusPatterns`.
- Track `seenIDs map[string]string` and `seenBinaries map[string]string`
  (value = winning filename) for the two collision passes; `os.ReadDir` already
  returns entries sorted by filename, which is what makes winners deterministic.
- Files: `session/detection/plugins.go`

##### Task 1.2.3d: Tests for `LoadPluginDir` (~5 min)
- Use `t.TempDir()` and write fixture files inline with `os.WriteFile` — do
  **not** add fixtures under `session/detection/testdata/`, which is used by
  the existing snapshot tests and must stay untouched (acceptance criterion #6).
- One test per acceptance criterion above, including the ten-iteration
  determinism loop and the `os.Symlink` case (skip on Windows via
  `runtime.GOOS`).
- One test for the `maxPluginFiles` cap: write 201 trivially-valid `.toml`
  files, assert exactly 200 detectors are returned and one
  `PluginLoadError{Field: "file_count"}` (**not** `"directory"` — see Task
  1.2.3c's note on why the count cap must use a distinct `Field` from the
  fatal directory-read-failure case) mentions the count.
- Files: `session/detection/plugins_test.go`

---

## Phase 2: Registry Merge, Snapshot, and Hot-Reload

### Epic 2.1: Registry override path
**Goal**: Let user plugins take precedence over built-ins for a binary name,
without weakening `Register`'s panic-on-duplicate invariant.

#### Story 2.1.1: Non-panicking `Upsert`
**As a** loader, **I want** to overwrite a registry entry by name, **so that**
requirement #4's override is expressible without touching `Register`.
**Acceptance Criteria**:
- `Upsert` replaces an existing entry instead of panicking.
  - *Given* a registry with `binaries.NewClaudeDetector()` registered,
    *When* `Upsert` is called with a `*PluginDetector` whose `Name()` is `claude`,
    *Then* `Lookup("claude")` returns the `*PluginDetector`, `Len()` is 1, and
    no panic occurs.
- `Register` still panics on a duplicate.
  - *Given* a registry with `claude` registered,
    *When* `Register` is called with another detector named `claude`,
    *Then* it panics with `detection: duplicate BinaryDetector registered for name: claude`
    — i.e. `TestDetectorRegistry_should_panic_When_duplicateNameRegistered` in
    `session/detection/registry_test.go` passes unmodified.
**Files**: `session/detection/binary_detector.go`, `session/detection/registry_test.go`

##### Task 2.1.1a: Add `Upsert` (~2 min)
- `func (r *DetectorRegistry) Upsert(d BinaryDetector) { r.detectors[d.Name()] = d }`
  in `session/detection/binary_detector.go`, immediately after `Register`
  (`binary_detector.go:15-20`).
- Doc comment must state explicitly that this is the *only* sanctioned way to
  replace an entry, that `Register`'s panic is a deliberate guard for
  compiled-in detectors, and that neither is safe for concurrent use — the
  registry is built once and then published via the snapshot (ADR-002).
- Files: `session/detection/binary_detector.go`

##### Task 2.1.1b: Test `Upsert` and re-assert `Register`'s panic (~3 min)
- Append `TestDetectorRegistry_should_replaceEntry_When_upsertCalledWithExistingName`
  to `session/detection/registry_test.go`.
- Leave `TestDetectorRegistry_should_panic_When_duplicateNameRegistered`
  byte-for-byte unmodified.
- Files: `session/detection/registry_test.go`

---

#### Story 2.1.2: `MergedRegistry`
**As a** user, **I want** my `claude`-claiming plugin to beat the built-in,
**so that** I can fix or fork detection for an agent I actually run.
**Acceptance Criteria**:
- A plugin overriding a built-in wins, and the registry does not grow.
  - *Given* `DefaultRegistry()` (5 built-ins: `claude`, `gemini`, `aider`,
    `opencode`, `agy`) and one `*PluginDetector` named `claude` from
    `claude-fork.toml` with a single `active` pattern `FORKED-BUSY`,
    *When* `MergedRegistry(DefaultRegistry(), plugins)` runs,
    *Then* `Len()` is 5 (not 6), and `Lookup("claude")` returns the
    `*PluginDetector`, whose `Patterns().Active[0].Pattern` is `FORKED-BUSY`.
- A plugin with a fresh binary name is added alongside the built-ins.
  - *Given* the same built-ins and a `*PluginDetector` named `my-agent`,
    *When* `MergedRegistry` runs,
    *Then* `Len()` is 6 and `Lookup("my-agent")` succeeds.
- The input registry is not mutated.
  - *Given* a `builtins := DefaultRegistry()` passed to `MergedRegistry` with a
    `claude` override,
    *When* the call returns,
    *Then* `builtins.Lookup("claude")` still returns `*binaries.ClaudeDetector`.
**Files**: `session/detection/registry.go`, `session/detection/registry_test.go`

##### Task 2.1.2a: Implement `MergedRegistry` (~4 min)
- `func MergedRegistry(builtins *DetectorRegistry, plugins []BinaryDetector) *DetectorRegistry`
  in `session/detection/registry.go`, below `DefaultRegistry` (`registry.go:6-14`).
- Build a **new** `NewDetectorRegistry()`, `Upsert` every built-in from
  `builtins.Names()`/`Lookup`, then `Upsert` every plugin — so the caller's
  registry is never mutated.
- When a plugin's name was already present, `log.Info("detector plugin overrides built-in", ...)`
  including the plugin's `SourcePath()` when it is a `*PluginDetector`.
- Files: `session/detection/registry.go`

##### Task 2.1.2b: Tests for `MergedRegistry` (~4 min)
- Three tests, one per acceptance criterion, in `session/detection/registry_test.go`.
- Files: `session/detection/registry_test.go`

---

### Epic 2.2: Atomic snapshot replacing `builtBinaryDetectors`
**Goal**: Make the per-binary detector map swappable at runtime with no
read-path locking and no observable half-applied state.

#### Story 2.2.1: `detectorSnapshot` and the atomic accessor
**As a** detection call, **I want** a lock-free read of the current detector
set, **so that** hot-reload costs nothing on the per-PTY-read path.
**Acceptance Criteria**:
- With no plugins loaded, behavior is identical to today.
  - *Given* a fresh process where `InitPlugins` is never called,
    *When* `DetectForProgram([]byte("esc to interrupt"), "claude")` is called,
    *Then* it returns `StatusExecuting` — the built-in `claude` `esc_to_interrupt`
    pattern (`binaries/claude.go:114-118`) — exactly as before this change.
- Provenance is reported for every claimed binary name.
  - *Given* the built-ins-only snapshot,
    *When* `DetectorProvenance()` is called,
    *Then* it returns a 5-entry map with `"claude" -> ""` (empty path meaning
    built-in), and mutating the returned map does not affect the snapshot.
**Files**: `session/detection/detector_snapshot.go` (new), `session/detection/detector.go`, `session/detection/detector_snapshot_test.go` (new)

##### Task 2.2.1a: Create `detector_snapshot.go` with the snapshot type (~4 min)
- New file `session/detection/detector_snapshot.go`, `package detection`.
- `type detectorSnapshot struct { byBinary map[string]*StatusDetector; provenance map[string]string }`
- `var activeSnapshot atomic.Pointer[detectorSnapshot]` and
  `var snapshotWriteMu sync.Mutex`.
- Doc comment citing ADR-002 and `session/pipeline_engine.go:123-140` for why
  the write mutex spans the whole rebuild, not just the `Store`.
- Files: `session/detection/detector_snapshot.go`

##### Task 2.2.1b: Implement `buildSnapshot` and package `init()` (~4 min)
- `func buildSnapshot(reg *DetectorRegistry, provenance map[string]string) *detectorSnapshot` —
  for each `reg.Names()`, `Lookup`, `NewPatternSet(bd.Patterns())`, construct a
  `&StatusDetector{}` and `Store` the pattern set. This is a straight lift of
  the existing IIFE body at `detector.go:735-746`.
- Compile failures here cannot happen for built-ins (patterns are code) and
  cannot happen for plugins (already compiled during validation), but log
  `log.Warn` and skip the entry rather than dropping the whole snapshot.
- `func init() { activeSnapshot.Store(buildSnapshot(DefaultRegistry(), nil)) }` —
  preserves today's package-init behavior for every code path that never calls
  `InitPlugins`, which is what keeps acceptance criterion #6 true.
- Files: `session/detection/detector_snapshot.go`

##### Task 2.2.1c: Implement `lookupBinaryDetector` and `DetectorProvenance` (~3 min)
- `func lookupBinaryDetector(program string) (*StatusDetector, bool)` —
  `activeSnapshot.Load()`, nil-guard, map index.
- `func DetectorProvenance() map[string]string` — returns a defensive copy.
- Files: `session/detection/detector_snapshot.go`

##### Task 2.2.1d: Delete the `builtBinaryDetectors` var and repoint `DetectForProgram` (~3 min)
- Remove the package-level `var builtBinaryDetectors = func() ... }()` block
  (`detector.go:733-746`).
- Change `detector.go:754` from `if bsd, ok := builtBinaryDetectors[program]; ok {`
  to `if bsd, ok := lookupBinaryDetector(program); ok {`. Nothing else in
  `DetectForProgram` changes.
- `grep -rn "builtBinaryDetectors" session/` must return nothing afterwards.
- Files: `session/detection/detector.go`

##### Task 2.2.1e: Snapshot tests + full existing-suite regression run (~5 min)
- New `session/detection/detector_snapshot_test.go` covering the two
  acceptance criteria above.
- Run `go test ./session/detection/...` and confirm every pre-existing test —
  including `snapshot_test.go`, `bug_regression_test.go`, and the
  `session/detection/testdata/` fixtures — passes with zero source changes.
- Files: `session/detection/detector_snapshot_test.go`

---

#### Story 2.2.2: `rebuildSnapshot`
**As a** loader or watcher, **I want** one function that scans, merges,
compiles, and publishes, **so that** every reload path is identical and
all-or-nothing.
**Acceptance Criteria**:
- A rebuild with a valid plugin makes it detectable.
  - *Given* `<dir>/my-agent.toml` declaring binary `my-agent` and a `processing`
    pattern `Thinking\.\.\.`,
    *When* `rebuildSnapshot(ctx, dir)` completes,
    *Then* `DetectForProgram([]byte("Thinking..."), "my-agent")` returns
    `StatusProcessing` and `DetectorProvenance()["my-agent"]` is that file's path.
- A directory-level failure leaves the previous snapshot intact.
  - *Given* a successful rebuild has loaded `my-agent`, and the directory is
    then replaced by a regular file (making `os.ReadDir` fail with `ENOTDIR`),
    *When* `rebuildSnapshot(ctx, dir)` runs again,
    *Then* it returns an error, `DetectorProvenance()` still contains
    `my-agent`, and no `Store` occurred.
- Per-file rejections do not block the rest of the rebuild.
  - *Given* a directory with valid `my-agent.toml` and invalid `broken.toml`,
    *When* `rebuildSnapshot(ctx, dir)` runs,
    *Then* it returns nil error, `my-agent` is detectable, `claude` still
    resolves to the built-in, and one `detector plugin rejected` warning was
    logged naming `broken.toml`.
- An already-cancelled context short-circuits before any work (Hardening
  Addendum item 4).
  - *Given* a `ctx` whose `context.CancelFunc` has already been called,
    *When* `rebuildSnapshot(ctx, dir)` runs,
    *Then* it returns `ctx.Err()` immediately, `snapshotWriteMu` is never
    locked, and no `Store` occurs.
- The `file_count` cap error does not trip the directory-level fatal path
  (Hardening Addendum item 3, regression guard for the `Field`
  disambiguation).
  - *Given* a directory with 201 valid `.toml` files,
    *When* `rebuildSnapshot(ctx, dir)` runs,
    *Then* it returns nil error, 200 of the 201 detectors are published in
    the new snapshot, and one `PluginLoadError{Field: "file_count"}` was
    logged rather than the rebuild being skipped.
**Files**: `session/detection/detector_snapshot.go`, `session/detection/detector_snapshot_test.go`

##### Task 2.2.2a: Implement `rebuildSnapshot` (~5 min)
- `func rebuildSnapshot(ctx context.Context, dir string) error` — **`ctx`
  parameter added per Hardening Addendum item 4** (original signature had no
  `ctx`; threaded from `InitPlugins(ctx)` and `watchLoop`'s already-in-scope
  `ctx`):
  1. If `ctx.Err() != nil`, return it immediately, before taking the lock —
     coarse-grained shutdown check, not mid-file cancellation (a single
     file's compile is already time-bounded by `maxPluginCompileTime`, Task
     1.2.1b, so it can't block shutdown indefinitely on its own).
  2. `snapshotWriteMu.Lock(); defer Unlock()` — held across the whole sequence.
  3. `LoadPluginDir(dir)`; if the returned errors contain one with
     `Field == "directory"`, log and `return` **without** storing. (A
     `Field == "file_count"` error from the Hardening Addendum item 3 cap is
     *not* this case — it accompanies a successful partial load and falls
     through to steps 4-7 normally.)
  4. `log.Warn` each remaining `PluginLoadError`.
  5. Build `provenance`: every built-in name → `""`, then each plugin's
     `Name()` → its `SourcePath()`.
  6. `MergedRegistry(DefaultRegistry(), asBinaryDetectors(plugins))`.
  7. `buildSnapshot(merged, provenance)` → `activeSnapshot.Store(...)`.
  8. `log.Info("detector plugins loaded", ...)`.
- Note in a comment that the whole directory is rebuilt on every reload rather
  than incrementally patching the changed file: `id`/`binary_names` collision
  detection and override precedence are **directory-global** properties that a
  per-file update cannot evaluate, and the directory is single-digit sized
  (now enforced up to `maxPluginFiles`, Task 1.2.3c).
- Files: `session/detection/detector_snapshot.go`

##### Task 2.2.2b: Tests for `rebuildSnapshot` (~6 min)
- Five tests, one per acceptance criterion (including the two Hardening
  Addendum criteria added above — `rebuildSnapshot_should_returnCtxErr_When_contextAlreadyCancelled`
  and the 201-file `file_count`-cap case), using `t.TempDir()`.
- Because `activeSnapshot` is package-global, each test must restore the
  built-ins-only snapshot in `t.Cleanup` via
  `activeSnapshot.Store(buildSnapshot(DefaultRegistry(), nil))` so tests don't
  leak state into `snapshot_test.go` and friends. Do **not** use `t.Parallel()`
  in this file.
- Files: `session/detection/detector_snapshot_test.go`

---

### Epic 2.3: Bootstrap, startup wiring, and hot-reload
**Goal**: Create the directory on first run, load once at startup, and keep it
current without a restart.

#### Story 2.3.1: Directory bootstrap and example seed file
**As a** first-time user, **I want** the directory to already exist with a
documented example, **so that** I know where to put a file and what to write.
**Acceptance Criteria**:
- The directory is created on first run.
  - *Given* `STAPLER_SQUAD_TEST_DIR=/tmp/ssq-bootstrap-1` with no `detectors/`
    subdirectory,
    *When* `EnsurePluginDir()` runs,
    *Then* `/tmp/ssq-bootstrap-1/detectors/` exists with mode `0o755`.
- The example file is seeded and documents every status key.
  - *Given* the same fresh directory,
    *When* `EnsurePluginDir()` runs,
    *Then* `/tmp/ssq-bootstrap-1/detectors/example.toml.sample` exists and its
    contents contain each of `ready`, `processing`, `needs_approval`,
    `input_required`, `error`, `tests_failing`, `idle`, `active`, `success`,
    `waiting_for_agent`.
- The example is not itself loaded as a plugin.
  - *Given* a directory containing only `example.toml.sample`,
    *When* `LoadPluginDir(dir)` runs,
    *Then* it returns zero detectors and zero errors (the `*.toml` glob does not
    match `.toml.sample`).
- Bootstrap is idempotent and never clobbers user edits.
  - *Given* `example.toml.sample` already exists with the single line `# edited`,
    *When* `EnsurePluginDir()` runs again,
    *Then* the file still contains exactly `# edited`.
**Files**: `session/detection/plugins.go`, `session/detection/plugins_test.go`

##### Task 2.3.1a: Implement `EnsurePluginDir` (~3 min)
- `func EnsurePluginDir() (string, error)` — `PluginDir()`, `os.MkdirAll(dir, 0o755)`.
  A failure here **is** returned — the directory itself is essential.
- Then write the example only if `os.Stat` reports it absent. **Per
  Hardening Addendum item 2: a failure writing the seed file is
  `log.Warn`'d and swallowed, not returned** — it's cosmetic (requirement
  #6 says "optionally seeded"), and propagating it as a fatal
  `EnsurePluginDir` error would abort the caller's subsequent scan+watch
  steps (Task 2.3.2a) even when the directory itself, and any real
  pre-existing plugin files in it, are perfectly readable.
- Files: `session/detection/plugins.go`

##### Task 2.3.1b: Write the example seed content (~4 min)
- Embed the sample as a `const examplePluginFile = `...`` in `plugins.go` (a
  Go string constant, not a `go:embed` of a repo file — the repo ships no
  `.toml` and this keeps the content next to the schema it documents).
- Content: fully commented, showing `id`, `version`, `binary_names`, and two
  `[[patterns]]` blocks (one `processing` with `regex = "Thinking\\.\\.\\."`,
  one `needs_approval` with `regex = "Do you want to proceed\\?"`), plus a
  comment block listing all ten status keys, a note that within a status
  category declaration order is match order, a note that regexes are Go RE2
  syntax (no backreferences, no lookahead), and a note that a `priority` key is
  **not** supported.
- Files: `session/detection/plugins.go`

##### Task 2.3.1c: Bootstrap tests (~4 min)
- Set `STAPLER_SQUAD_TEST_DIR` via `t.Setenv` so `config.GetConfigDir()`
  resolves into a temp dir (`config/config.go:124-131`).
- One test per acceptance criterion, including a loop asserting all ten status
  keys appear in the seeded file — this test is what keeps the example in sync
  if a status key is ever added.
- Files: `session/detection/plugins_test.go`

---

#### Story 2.3.2: `InitPlugins` and the `main.go` call site
**As an** operator, **I want** plugins loaded once at daemon startup after
logging is up, **so that** rejections are visible in the log and the directory
exists before the first scan.
**Acceptance Criteria**:
- Startup bootstraps, loads, and starts watching in the right order.
  - *Given* `STAPLER_SQUAD_TEST_DIR=/tmp/ssq-init-1` with no `detectors/` and
    no plugin files,
    *When* `InitPlugins(ctx)` runs,
    *Then* the directory and `example.toml.sample` exist, `DetectorProvenance()`
    has exactly the 5 built-in names, and a watcher goroutine is running.
- The kill switch fully disables the feature.
  - *Given* `STAPLER_SQUAD_DISABLE_DETECTOR_PLUGINS=1`,
    *When* `InitPlugins(ctx)` runs,
    *Then* it returns nil, `<dir>/detectors/` is **not** created, no watcher
    goroutine starts, and `DetectorProvenance()` has exactly the 5 built-ins.
- Startup failure never blocks the daemon.
  - *Given* a plugin directory path that cannot be created (its parent is a
    regular file),
    *When* `InitPlugins(ctx)` runs,
    *Then* it logs a warning and returns nil, and the built-ins-only snapshot
    stays active.
**Files**: `session/detection/plugins.go`, `main.go`, `session/detection/plugins_test.go`

##### Task 2.3.2a: Implement `InitPlugins` (~4 min)
- `var initPluginsOnce sync.Once` at package scope (Hardening Addendum item
  5 — re-entrancy guard).
- `func InitPlugins(ctx context.Context) error`, body wrapped in
  `initPluginsOnce.Do(func() { ... })` — a second call in the same process
  is a documented no-op instead of starting a duplicate watcher:
  1. `if os.Getenv("STAPLER_SQUAD_DISABLE_DETECTOR_PLUGINS") != "" { log.Info(...); return nil }`
  2. `dir, err := EnsurePluginDir()` — on error, `log.Warn` and `return nil`
     (never fatal; a broken plugin directory must not stop the daemon).
  3. `_ = rebuildSnapshot(ctx, dir)` — errors already logged inside; `ctx`
     threaded per Task 2.2.2a's updated signature.
  4. `StartPluginWatcher(ctx, dir)` — on error, `log.Warn` and continue.
- Doc comment: safe to call more than once (the `sync.Once` makes later
  calls no-ops), but intended call site is exactly once, from `main.go`,
  after logging init.
- Files: `session/detection/plugins.go`

##### Task 2.3.2b: Wire the call into `main.go` (~2 min)
- In the root command's `RunE`, insert after the
  `log.InitializeWithConfig(daemonFlag, buildLogConfig(...))` block that ends at
  `main.go:152`, and **before** `if daemonFlag { ... daemon.RunDaemon(cfg) }`
  at `main.go:177`:
  ```go
  // Load user-defined detector plugins from ~/.stapler-squad/detectors/ and
  // start watching that directory for changes. Never fatal — see ADR-002.
  if err := detection.InitPlugins(ctx); err != nil {
      log.Warn("failed to initialize detector plugins", "err", err)
  }
  ```
- Add the `github.com/tstapler/stapler-squad/session/detection` import.
- Placing it before the `daemonFlag` branch means one call site covers both the
  daemon and web-server modes. `ctx` here is the `signal.NotifyContext` from
  `main.go:62`, so the watcher goroutine is torn down on SIGTERM/SIGINT.
- Files: `main.go`

##### Task 2.3.2c: Tests for `InitPlugins` (~4 min)
- `t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())` per test; kill-switch test
  additionally sets `STAPLER_SQUAD_DISABLE_DETECTOR_PLUGINS`.
- Use a `context.WithCancel` and cancel in `t.Cleanup` so watcher goroutines
  don't leak across tests.
- Files: `session/detection/plugins_test.go`

---

#### Story 2.3.3: `PluginWatcher` hot-reload
**As a** plugin author, **I want** my edit to take effect without restarting
stapler-squad, **so that** iterating on a regex takes seconds, not a service
restart that would kill every live tmux session.
**Acceptance Criteria**:
- Adding a file is picked up without a restart.
  - *Given* a running watcher over an empty directory,
    *When* `my-agent.toml` (binary `my-agent`, `processing` pattern
    `Thinking\.\.\.`) is written into it,
    *Then* within 2 seconds `DetectForProgram([]byte("Thinking..."), "my-agent")`
    returns `StatusProcessing`.
- Editing a file is picked up without a restart.
  - *Given* the loaded `my-agent.toml` above,
    *When* its `regex` is changed from `Thinking\.\.\.` to `Pondering\.\.\.`,
    *Then* within 2 seconds `DetectForProgram([]byte("Pondering..."), "my-agent")`
    returns `StatusProcessing`.
- Removing a file is picked up without a restart.
  - *Given* the loaded `my-agent.toml`,
    *When* the file is deleted,
    *Then* within 2 seconds `DetectorProvenance()` no longer contains
    `my-agent`.
- Removing a file that overrode a built-in restores the built-in.
  - *Given* a loaded `claude-fork.toml` claiming binary `claude`,
    *When* it is deleted,
    *Then* within 2 seconds `DetectorProvenance()["claude"]` is `""` and
    `DetectForProgram([]byte("esc to interrupt"), "claude")` returns
    `StatusExecuting` again.
- A burst of events causes one reload, not many.
  - *Given* a running watcher,
    *When* the same file is written 10 times in a 50ms loop,
    *Then* exactly one `detector plugins loaded` line is logged for that burst
    (verified via a rebuild counter injected for the test).
- fsnotify being unavailable degrades to periodic rescan, not failure.
  - *Given* `fsnotify.NewWatcher()` returns an error,
    *When* `StartPluginWatcher` runs,
    *Then* it logs a warning, returns a watcher whose only loop is the 60s
    ticker, and `InitPlugins` still returns nil.
**Files**: `session/detection/plugin_watcher.go` (new), `session/detection/plugin_watcher_test.go` (new), `session/detection/plugins.go`

##### Task 2.3.3a: Create `plugin_watcher.go` skeleton (~4 min)
- New file `session/detection/plugin_watcher.go`, `package detection`.
- `const pluginReloadDebounce = 200 * time.Millisecond` and
  `const pluginRescanInterval = 60 * time.Second`, both with comments citing
  `session/unfinished/gogitstore/mmapwatch.go`'s `packWatchDebounce` and
  `session/unfinished/watcher.go`'s `periodicReWalk` as the precedents.
- `type PluginWatcher struct { dir string; watcher *fsnotify.Watcher; stopped chan struct{} }`
  with `func (w *PluginWatcher) Stopped() <-chan struct{}`.
- Files: `session/detection/plugin_watcher.go`

##### Task 2.3.3b: Implement `StartPluginWatcher` with soft-fail (~4 min)
- `func StartPluginWatcher(ctx context.Context, dir string) (*PluginWatcher, error)`.
- `fsnotify.NewWatcher()` failure → `log.Warn` and continue with a nil
  `*fsnotify.Watcher` (periodic-only mode), mirroring `NewWatchDirWatcher`'s
  existing fallback. Same for `watcher.Add(dir)` failing.
- **Watch the directory, never individual files** — editors save by writing a
  temp file and renaming over the original, so per-file watches silently stop
  firing after the first save (fsnotify's own documented caveat).
- Launch exactly one goroutine running `w.watchLoop(ctx)`; close `w.stopped` on
  return. Single-owner drain of `Events`/`Errors` matches both existing repo
  precedents.
- Files: `session/detection/plugin_watcher.go`

##### Task 2.3.3c: Implement `watchLoop` with debounce + periodic rescan (~5 min)
- `select` over `ctx.Done()`, `w.watcher.Events`, `w.watcher.Errors`,
  `debounce.C`, and `ticker.C` (`pluginRescanInterval`).
- React to `event.Has(fsnotify.Write|Create|Remove|Rename|Chmod)` — note
  `Remove`/`Rename` are required by requirement #5 and are *not* handled by the
  existing `WatchDirWatcher.fsnotifyLoop` (`watcher.go:156`, Write/Create only),
  which is why this loop is new code rather than a reuse.
- Filter to `filepath.Ext(event.Name) == ".toml"` before arming the timer.
- Debounce: `time.NewTimer` armed/reset to `pluginReloadDebounce` on each
  qualifying event; on fire, call `rebuildSnapshot(ctx, w.dir)` (`ctx` is
  `watchLoop`'s own parameter, already in scope for the `select`).
- Ticker fires `rebuildSnapshot(ctx, w.dir)` unconditionally as the safety net.
- `w.watcher.Errors` → `log.Warn("fsnotify error", "err", err)`, matching
  `watcher.go:184`.
- `defer w.watcher.Close()` guarded for the nil (periodic-only) case.
- Files: `session/detection/plugin_watcher.go`

##### Task 2.3.3d: Watcher tests (~5 min)
- New `session/detection/plugin_watcher_test.go`.
- Use `testutil/wait` (already an existing test dependency of this package) or
  `require.Eventually`-style polling with a 2s budget — **never**
  `time.Sleep` as the assertion mechanism, per `docs/adr/003-no-static-sleeps-in-tests.md`.
- Add a package-level `var rebuildCount atomic.Int64` incremented inside
  `rebuildSnapshot` so the debounce-burst test can assert exactly one rebuild.
- Restore the built-ins-only snapshot in `t.Cleanup`, and cancel the watcher
  context, in every test.
- Files: `session/detection/plugin_watcher_test.go`, `session/detection/detector_snapshot.go`

##### Task 2.3.3e: Full verification gate (~5 min)
- `gofmt -w .`
- `make build && make test`
- `make lint`
- `make nil-safety` — the new atomic-pointer load path is exactly the shape
  NilAway flags; `lookupBinaryDetector` must nil-guard `activeSnapshot.Load()`.
- `go test -race ./session/detection/...` — mandatory here, since this feature
  introduces a concurrent write to a structure that was previously
  write-once at package init.
- Files: none (verification only)

---

### Epic 2.4: Wire plugin-aware detection into the live status path
**Goal**: Close the gap identified in `pre-mortem.md` P1 #1 — make
`DetectForProgram`/`lookupBinaryDetector` (Epic 2.2) actually reachable from
the code path that produces the status badge a user sees, so a loaded
override or new-binary plugin changes real behavior, not just
`DetectorProvenance()`. Without this epic, acceptance criterion #1 is false
for the shipped app even though every test in `validation.md` passes.

#### Story 2.4.1: `ClaudeController.Start` resolves its detector per-program
**As a** user whose plugin overrides `claude` (or adds a new binary),
**I want** my session's actual status detector to be the one my plugin
declares, **so that** the plugin I loaded is the plugin that runs.
**Acceptance Criteria**:
- A per-program detector is used when one is registered.
  - *Given* a loaded `*PluginDetector` claiming binary `my-agent` (or an
    override of `claude`), and a session whose `Instance.Program` is that
    name,
    *When* `ClaudeController.Start` runs,
    *Then* the `StatusDetector` it constructs was built from
    `lookupBinaryDetector("my-agent")`'s pattern set, not
    `getDefaultPatterns()` — verified by asserting the constructed
    detector's compiled patterns match the plugin's, not Claude's built-in
    `esc_to_interrupt` pattern.
- Behavior is unchanged for a program with no matching detector.
  - *Given* `Instance.Program` is some value with no built-in or plugin
    detector registered for it (e.g. a bare shell),
    *When* `ClaudeController.Start` runs,
    *Then* the constructed `StatusDetector` falls back to today's
    `getDefaultPatterns()` behavior exactly — this is not a regression for
    the common case, only an addition for the matched case.
- A hot-reload while a session is running is picked up by *new* sessions
  immediately and does not require restarting already-running sessions to
  benefit from a plugin fix (existing sessions keep whatever detector they
  were constructed with — no requirement to hot-swap a live session's
  in-flight `StatusDetector`, since `Start` runs once per session lifecycle).
**Files**: `session/claude_controller.go`, `session/claude_controller_test.go`

##### Task 2.4.1a: Resolve the detector by program name in `Start` (~4 min)
- In `session/claude_controller.go`, replace the unconditional
  `sd := detection.NewStatusDetector()` (currently line ~221) with: look up
  `cc.instance.Program` via `detection.DetectorForProgram(program string)
  (*detection.StatusDetector, bool)` — a small new exported wrapper around
  `lookupBinaryDetector` (package-private today; Epic 2.2 built it
  unexported since it previously had no external caller) — and fall back to
  `detection.NewStatusDetector()` when the lookup misses.
- Keep the existing `sd.SetSessionID(cc.sessionName)` call on whichever
  detector is chosen, so detection-event attribution keeps working for both
  paths.
- Files: `session/claude_controller.go`

##### Task 2.4.1b: Export `DetectorForProgram` from the snapshot (~2 min)
- Add `func DetectorForProgram(program string) (*StatusDetector, bool)` to
  `session/detection/detector_snapshot.go` as a thin exported alias of
  `lookupBinaryDetector` — same nil-guard, same lock-free `Load()`. Keeping
  `lookupBinaryDetector` itself unexported and used internally by
  `DetectForProgram` (the method) avoids two names colliding in godoc for
  what would otherwise be the same lookup exposed twice.
- Files: `session/detection/detector_snapshot.go`

##### Task 2.4.1c: Tests (~5 min)
- `session/claude_controller_test.go`: one test per acceptance criterion
  above, using a fake/injected registry snapshot (via the same
  `t.Cleanup`-restore pattern Task 2.2.2b established) rather than a real
  plugin file on disk.
- `session/detection/detector_snapshot_test.go`: test `DetectorForProgram`
  directly (thin wrapper, but still a public API surface — cover the hit and
  miss cases).
- Files: `session/claude_controller_test.go`, `session/detection/detector_snapshot_test.go`

---

## Phase 3 (Optional): Plugin Debugging Affordance

**Status: recommended but not required.** No acceptance criterion in
`requirements.md` depends on this phase; drop it if scope is tight. It exists
because both `research/features.md` (unstated need #1, #4) and
`research/pitfalls.md` (item 8) identify the same gap: structural validation
catches "bad regex", but the far more common failure is "regex compiles fine
and never matches real output," which nothing in Phases 1–2 helps with. Direct
prior art: `fail2ban-regex --print-all-missed`.

### Epic 3.1: `detectors` CLI subcommand
**Goal**: Answer "what is loaded?" and "why doesn't my pattern match?" without
a UI and without reading a log file.

#### Story 3.1.1: `stapler-squad detectors list`
**As a** plugin author, **I want** to see which file claims each binary name,
**so that** I can tell whether my plugin loaded and whether it overrode a
built-in.
**Acceptance Criteria**:
- Loaded plugins and built-ins are both listed with their source.
  - *Given* `~/.stapler-squad/detectors/my-agent.toml` loaded,
    *When* `stapler-squad detectors list` runs,
    *Then* stdout contains a line `my-agent  <path>/my-agent.toml` and a line
    `claude    (built-in)`.
- Rejected files are reported with the reason.
  - *Given* `broken.toml` with regex `Thinking(\.\.\.`,
    *When* `stapler-squad detectors list` runs,
    *Then* stdout contains `broken.toml  REJECTED  patterns[0].regex: ...`.
**Files**: `main.go`, `session/detection/plugins.go`

##### Task 3.1.1a: Add the `detectors` parent command and `list` subcommand (~5 min)
- Define `detectorsCmd` and `detectorsListCmd` in `main.go` alongside the
  existing `debugCmd` (`main.go:404-440`), and register with
  `rootCmd.AddCommand`.
- `list` calls `EnsurePluginDir()` + `LoadPluginDir(dir)` directly (not
  `InitPlugins` — no watcher, no snapshot swap) and prints a table plus the
  `[]PluginLoadError`s.
- Files: `main.go`

---

#### Story 3.1.2: `stapler-squad detectors test`
**As a** plugin author, **I want** to run my patterns against captured output,
**so that** I can confirm a regex matches before trusting it live.
**Acceptance Criteria**:
- A matching pattern reports the status and the pattern name.
  - *Given* `my-agent.toml` with a `processing` pattern named
    `my_agent_thinking` and regex `Thinking\.\.\.`, and a file `sample.txt`
    containing `Thinking...`,
    *When* `stapler-squad detectors test --file my-agent.toml --input sample.txt` runs,
    *Then* stdout contains `status=processing pattern=my_agent_thinking`.
- A non-matching input names every pattern that was tried.
  - *Given* the same plugin and `sample.txt` containing `Pondering...`,
    *When* the same command runs,
    *Then* stdout contains `no match` and lists `my_agent_thinking` under
    `patterns tried`.
**Files**: `main.go`, `session/detection/plugins.go`

##### Task 3.1.2a: Add the `test` subcommand (~5 min)
- Flags `--file` (path to a `.toml`) and `--input` (path to captured output;
  `-` reads stdin).
- Parse + validate the one file, `NewPatternSet(patterns)`, call
  `MatchLines(text, raw)` (`pattern_set.go:69`), print the returned status,
  pattern name, and description.
- On no match, print every pattern name from the file (reusing the existing
  `GetPatternNames` shape in `detector.go`) so the author sees what was tried.
- Files: `main.go`

##### Task 3.1.2b: Tests for the CLI subcommands (~4 min)
- Table-driven tests in `main_test.go` invoking the cobra commands with
  `SetArgs` and capturing output via `SetOut`.
- Files: `main_test.go`
