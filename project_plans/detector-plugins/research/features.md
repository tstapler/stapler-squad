# Research: Feature Landscape for TOML-Based Detector Plugins

## 1. Existing patterns in this codebase

### 1.1 A YAML pattern-loading pipeline already exists — and is unused

`session/detection/detector.go` already implements almost the exact shape of
capability this item asks for, just for YAML and a single monolithic file
instead of TOML and a directory of per-detector files:

- `NewStatusDetectorFromFile(path string)` — reads a YAML file, unmarshals
  into `dtypes.StatusPatterns`, compiles it via `NewPatternSet`, and returns a
  ready `StatusDetector`.
- `(sd *StatusDetector) LoadPatterns(path string) error` — re-loads YAML from
  disk and **atomically swaps** the active pattern set:
  `sd.patternSet.Store(newSet)` where `patternSet` is an
  `atomic.Pointer[PatternSet]`. This is the hot-reload/concurrency answer
  already adopted elsewhere in the package — readers always call
  `sd.patternSet.Load()` and get either the old or new set, never a torn
  read. **The plugin loader should reuse this exact swap-a-pointer pattern**
  rather than inventing new locking.
- `validatePatternFilePath(path)` — rejects any path containing `..` (path
  traversal guard). Minimal, but establishes that path validation is already
  a recognized concern for this feature area.
- `(sd *StatusDetector) ExportPatterns(path string) error` — marshals the
  live `StatusPatterns` back out to YAML. This is a ready-made model for a
  "dump what's currently loaded" debug/test affordance (see §3.4 below).
- `GetPatternNames(status DetectedStatus) []string` — introspection: which
  named patterns are loaded for a given status category.

**Grep confirms `LoadPatterns`/`NewStatusDetectorFromFile` are called from
nowhere else in the tree (no call sites outside `detector.go` itself and its
own tests)** — this looks like a partially-built foundation for this exact
feature that was never wired up. The new plugin loader should either build
directly on this code path (swap YAML for TOML, or support both) or
explicitly document why it's superseding it. Re-deriving hot-reload/atomic
semantics from scratch would be redundant.

### 1.2 `DetectorRegistry.Register` panics on duplicate names — blocks the override requirement as-is

`session/detection/binary_detector.go`:
```go
func (r *DetectorRegistry) Register(d BinaryDetector) {
	if _, exists := r.detectors[d.Name()]; exists {
		panic("detection: duplicate BinaryDetector registered for name: " + d.Name())
	}
	r.detectors[d.Name()] = d
}
```
Requirement #4 (user plugin whose `binary_names` matches a built-in
overrides that built-in) **cannot be satisfied by calling `Register` twice
with the same name** — it will panic. The design needs either:
- a new `RegisterOverride`/`Upsert` method used only by the plugin-merge
  path, leaving `Register`'s panic-on-duplicate as a build-time safety net
  for built-in detectors, or
- building the merged registry by constructing built-ins first into a plain
  map, layering user plugins on top with explicit precedence, and passing
  the result through a constructor rather than N `Register` calls.

Confirmed via `registry_test.go`:
`TestDetectorRegistry_should_panic_When_duplicateNameRegistered` — the panic
behavior is an intentional, tested invariant for the existing API, so it
must not be loosened for all callers, only bypassed for the plugin-merge
path specifically.

### 1.3 `dtypes.StatusPattern.Priority` is a dead field

`dtypes.StatusPattern` has a `Priority int` field (`yaml:"priority"`), but
`PatternSet.compile()`/`MatchLines()` never read it — match order is
purely: fixed category priority (error > tests_failing > needs_approval >
input_required > ...) then array order within a category. This is worth
flagging during planning: should plugin-contributed patterns respect
`Priority` (finally wiring the field up), or should the TOML schema drop it
to avoid promising behavior that doesn't exist yet? Silently keeping a field
that looks load-bearing but is ignored is a foot-gun for plugin authors.

### 1.4 `~/.stapler-squad/` directory conventions

Grep across `config/` and `session/` for `.stapler-squad` shows an
established convention for user-scoped subdirectories, all resolved via
`os.UserHomeDir()` + `filepath.Join`, e.g.:
- `~/.stapler-squad/repos/...` (`session/repo_path.go`)
- `~/.stapler-squad/worktrees/...`
- `~/.stapler-squad/checkpoints`, `~/.stapler-squad/triage-artifacts`,
  `~/.stapler-squad/backlog-attachments` (`config/config.go`)
- `~/.stapler-squad/cdp-bins/<sessionID>/` (`session/cdp/manager.go`)
- `~/.stapler-squad/scripts/` (`session/mux/hooks.go`, user-overridable hook
  scripts — closest existing precedent to "user drops a file in a
  well-known directory to extend behavior")

`~/.stapler-squad/detectors/` slots directly into this convention. None of
the existing directories are fsnotify-watched at runtime, though — they're
read once at startup or on demand. `session/history_watcher.go` and
`session/unfinished/watcher.go` are the two real fsnotify precedents in the
repo (watching `~/.claude/projects/` and backlog/unfinished-work dirs
respectively); both follow the same shape:
- watch the directory (and, in `history_watcher.go`, walk + watch existing
  subdirectories),
- filter events by suffix/prefix in `handleEvent`,
- degrade gracefully (log-and-continue, not fail) if the directory doesn't
  exist at watch-start time,
- run event handling in a dedicated goroutine off a `context.Context`,
  exposing a `Stopped()` channel for shutdown synchronization.

This is the fsnotify idiom the detector-plugins watcher should match,
rather than introducing a different one.

### 1.5 Config-level "user-extensible via config.json" precedent (weaker fit)

`config/types.go` has `AliasConfig` (`Aliases []AliasConfig` on the config
struct) — named session presets users define in `config.json`, matched by
`^[\w-]+$` name, invoked via `@name` in the omnibar. It's user-extensible
without a rebuild, but:
- lives in the single `config.json` (not a directory of independent files),
- is not hot-reloaded via fsnotify — `config/state.go` only has
  `RefreshState()`, an on-demand poll/reload, not a filesystem watcher,
- has no "collides with a built-in" concern (aliases are pure user
  namespace).

Frontend has an analogous *dynamic-detector* concept — `WorkflowDetector`
and `AliasDetector` in the omnibar's `DetectorRegistry`
(`web-app/src/lib/omnibar/detector.ts`) are registered/unregistered at
runtime from `OmnibarContext.tsx` effects specifically because they need
runtime-fetched, user-defined data, layered on top of the static
`createDefaultRegistry()` list. It's a different registry (input-detection,
not agent-status-detection) but the same shape of problem: a fixed
build-time list plus a variable, user/data-driven overlay resolved at
runtime. Confirms this "static + dynamic overlay" shape is already an
accepted architecture pattern in this codebase, just not yet done with
filesystem-watched TOML on the backend.

### 1.6 No TOML library currently in `go.mod`/`go.sum`

`gopkg.in/yaml.v3` is already a dependency (used by `detector.go` above and
elsewhere); no `BurntSushi/toml`, `pelletier/go-toml`, or `toml` of any kind
appears in `go.mod`/`go.sum`. Adding TOML support means a new dependency.
Worth raising in the stack-research phase: `BurntSushi/toml` (most common,
simple, well-maintained but hasn't added new TOML spec features in a while)
vs. `pelletier/go-toml/v2` (faster, more actively developed, supports
TOML v1.0.0 fully, has a `Marshal`/`Unmarshal` + a decode-with-error-context
API that would help requirement #3's "clear, actionable error" demand for
field-level error locations).

## 2. Industry precedent for "user-extensible detection/plugin via config file"

| System | Mechanism | Relevant lesson |
|---|---|---|
| **fail2ban** | `jail.d/*.conf`, `filter.d/*.conf` — user drops `.conf` files defining regex `failregex`/`ignoreregex` per service, merged with shipped `filter.d/` definitions; user files in `jail.local`/`jail.d/` explicitly override shipped `jail.conf` by load-order convention (`.local` and `.d/` loaded after and win). | Directly analogous override-by-convention model for requirement #4 — "last loaded wins" / "more specific path wins" is a simpler mental model than field-level precedence logic, and is exactly what's being asked for (user plugin beats built-in for the same binary name). |
| **Prometheus relabel_configs / recording rules** | YAML rule files loaded from a directory glob (`rule_files: ["rules/*.yml"]`), validated at load time with `promtool check rules`, hot-reloaded via `SIGHUP` or the `/-/reload` HTTP endpoint — reload is all-or-nothing per file set, and a malformed rule file causes Prometheus to **keep serving the last-known-good config** rather than crash or partially apply. | Strong precedent for requirement #3's "one bad file must not break others" — the "keep last-good, log-and-skip-new" strategy (not "fail startup") is the standard reload contract, and matches the atomic-pointer-swap pattern already in `detector.go` (§1.1): build the *whole* new registry off-thread, validate it completely, and only then swap the pointer — never mutate the live registry in place file-by-file. |
| **Logstash / Fluentd grok patterns** | Custom `patterns_dir` of user-defined named regex fragments, referenced by name in filter configs; a broken custom pattern fails that specific filter/pipeline stage with a named error, not the whole agent. | Same "bounded blast radius per named unit" lesson; also demonstrates the value of a `--config.test` / dry-run mode (see `logstash --config.test_and_exit`) as the standard way operators validate config before a live reload — directly maps to the "test a detector before trusting it live" unstated need (§3.1). |
| **ESLint / Prettier plugin resolution** | Plugins are just npm packages/config objects merged into a config tree; conflicting rule IDs are resolved by "last config wins" (extends order), and `eslint --print-config` exists specifically so users can debug what actually got merged. | Precedent for both the override-semantics simplicity and for a debug/introspection command (`GetPatternNames` in `detector.go` is already halfway there, per §1.1). |
| **tmux / vim user config directories** (`~/.tmux/plugins/`, `~/.vim/pack/`, `~/.config/nvim/lua/`) | Convention: absence of the directory is normal and silently creates it or no-ops; a broken plugin file often breaks only that plugin's load, with the tool printing a warning banner rather than refusing to start. | Reinforces requirement #6 (auto-create dir on first run) as a near-universal convention, and that "warn, don't crash" is the expected UX even for tools with much less operational stakes than a running dev-session manager. |
| **systemd drop-in directories** (`*.service.d/*.conf`, `sudoers.d/`) | Directory of independent files, each parsed independently; naming convention (`NN-name.conf`) gives deterministic override/merge order when multiple files could apply to the same unit. | If TOML plugin `id`/`binary_names` collisions across user files need a deterministic tiebreak beyond "reject as invalid" (requirement #3 already says user-user collisions should be rejected, so this mostly reinforces that choice is correct — don't invent silent ordering-based tiebreaks for user-vs-user, only for user-vs-built-in). |

## 3. Unstated user needs

### 3.1 Test a new detector against captured output before trusting it live

None of the functional requirements mention a dry-run/test mode, but every
industry precedent above (`promtool check rules`, `logstash
--config.test_and_exit`, `eslint --print-config`) treats "validate/preview
before it affects production" as core, not optional. Concretely for this
feature: a user writing regexes against real terminal output (which is
famously fiddly — see the ANSI-stripping complexity already in
`detector.go`'s `ansiStripRegex` and the dozens of fixture files in
`session/detection/testdata/*.txt`) has no way to check "does my regex
actually match this captured scrollback" without restarting a real session
and watching it live. A CLI subcommand or debug endpoint that runs a
candidate TOML file's patterns against a pasted/`--file` blob of captured
PTY text (reusing `PatternSet.MatchLines`) would close this gap cheaply,
and reuse the existing `ExportPatterns`-style plumbing.

### 3.2 Discoverability of the schema

Requirement #6 says the seed directory is "optionally seeded with a
commented example file documenting the schema" — this should be treated as
load-bearing, not optional. Users won't discover `id`/`binary_names`/
`[[patterns]]`/`status` field names from anywhere else (there's no existing
JSON-schema/doc site for this). The seeded example file is the only
realistic discovery path, so it needs to enumerate every valid `status`
value from `dtypes.StatusPatterns` (ready, processing, needs_approval,
input_required, error, tests_failing, idle, active, success,
waiting_for_agent) inline as a comment, since a typo'd status name is one of
the explicitly-listed rejection cases (requirement #3) and the user has no
other way to know the exact valid set.

### 3.3 Versioning/migration if the TOML schema changes later

Requirement #1 already includes an optional `version` field described as
"for the author's own tracking" (i.e., not currently used by
stapler-squad). This is worth flagging as a likely future need even though
explicitly out of scope now: once real users have plugin files in the wild,
a schema change (e.g. adding a new pattern field, renaming `status` values)
needs a compatibility story. Precedent from Prometheus (`rule_files`
schema versions tied to server version, breaking changes documented in
release notes) and ESLint (`eslintrc` v8→v9 flat-config migration, which
caused significant ecosystem pain) both suggest: reserve the `version`
field's *meaning* now (e.g. "absence or `1` = current schema") even if no
migration logic is built yet, so a future breaking change has a documented
field to branch on instead of guessing from file shape.

### 3.4 Debugging why a detector isn't matching

Beyond the dry-run need (§3.1), there's a live-debugging need: once a
plugin is loaded, if it's not firing as expected, the user needs visibility
into (a) whether their file loaded at all vs. was rejected, (b) which
detector actually claimed a given `binary_name` (built-in or which plugin
file), and (c) which specific pattern last matched for a live session.
`GetPatternNames` and the existing per-match `patternName`/`description`
return values from `PatternSet.MatchLines` (already surfaced today, e.g. in
`session/detection/snapshot_test.go`-style fixtures) show the data already
exists internally — the gap is exposing it (log line, debug endpoint, or
CLI command) rather than computing something new.

### 3.5 Sharing/exporting detectors between users

Requirement's explicit non-goal is *remote* distribution (issue #178
territory), but "share a single file with a coworker/forum/gist" is a much
lower bar than a manifest server and is likely to be requested immediately
once this ships (every industry precedent above — fail2ban jails, ESLint
configs, Prometheus rules, ohmyzsh/tmux plugins — has a thriving copy-paste
sharing culture before any package registry exists for it). No action
needed for this item, but the TOML file should be trivially self-contained
(no relative-path includes, no environment-specific absolute paths) so nothing
architecturally forecloses "paste this file into your own
`~/.stapler-squad/detectors/`" as the de facto v1 sharing mechanism.

## 4. Edge cases and failure modes to design for

Grouped by category, each cross-referenced to precedent above:

**Parsing/validation**
- Malformed TOML syntax (unparseable file) — reject with file path + parse
  error, continue loading other files (Prometheus rule_files precedent,
  §2; requirement #3 already specifies this).
- Missing/empty required fields (`id`, `binary_names`) — reject per-file
  with field name in the error.
- `status` value not in the known `StatusPatterns` category set — reject
  with the invalid value **and** the list of valid values inline (ties to
  §3.2 discoverability — don't make the user go find the list separately).
- Regex fails to compile (Go's RE2 syntax, not PCRE — TOML authors coming
  from grep/PCRE backgrounds will hit unsupported constructs like
  lookahead/lookbehind and backreferences; Go's `regexp` rejects these at
  `Compile` time with a clear-ish error, which should be surfaced verbatim
  plus the pattern's `name` field and source file).
- Empty `[[patterns]]` list, or a pattern block missing `regex`/`status`.

**Collisions**
- `id` collision between two *user* plugin files — reject the
  second-loaded one per requirement #3; needs a deterministic load order
  (e.g. lexical filename sort) so "which one wins" is reproducible, not
  directory-iteration-order-dependent (Go's `os.ReadDir` already returns
  sorted-by-name, so this is "free" if the loader uses `ReadDir` rather
  than an unsorted walk — worth confirming in implementation).
- `binary_names` collision between two *user* plugin files (not just
  `id`) — same treatment, since two plugins both claiming to own `claude`
  is ambiguous in a different way than duplicate `id`.
- `binary_names` collision with a **built-in** — this one is *not* an
  error, it's the explicit override behavior of requirement #4. The
  validator needs to distinguish "colliding with another user file" (reject)
  from "colliding with a built-in" (allow, override) — these are opposite
  outcomes for superficially the same collision check, so the validation
  code must check user-vs-user and user-vs-built-in as two separate passes
  against two separate name sets.
- `DetectorRegistry.Register`'s existing panic-on-duplicate (§1.2) will fire
  if the merge path naively calls `Register` for both a built-in and an
  overriding plugin — must be handled explicitly, not discovered at runtime
  via a panic in production.

**Regex safety**
- Catastrophic backtracking: Go's `regexp` package compiles to RE2
  automata — guaranteed linear-time matching, no exponential-blowup
  backtracking is possible **by construction**, unlike PCRE/Python `re`.
  This should be stated plainly in the plan/ADR as "no additional
  ReDoS-guard code is needed for this reason," rather than left
  unconfirmed — it's a real difference from most industry precedent
  (fail2ban/logstash/ESLint's underlying regex engines can all
  backtrack catastrophically; Go's cannot). The NFR's suggestion of a
  "max-pattern-length guard" is about compile-time cost / memory (RE2
  automaton size can still grow large for pathological patterns) and
  about limiting how much text a user can shove into one `regex` string —
  a reasonable belt-and-suspenders cap (e.g. reject regex source over
  some KB threshold) is cheap defense-in-depth, not a correctness
  requirement.
- Regex source containing constructs RE2 doesn't support at all
  (backreferences `\1`, lookahead `(?=...)`) — these are outright
  `Compile` errors already (not a hang risk), so this is a UX problem
  (users need to know *why* their PCRE-flavored regex won't compile), not
  a safety problem.

**Filesystem/concurrency**
- Partial-write mid-scan: fsnotify fires on `Write` events as a file is
  being saved, so a detector loader could read a half-written TOML file
  and get a truncated-parse error. `history_watcher.go`'s pattern of
  reacting to `Create|Rename|Write` doesn't debounce; a naive
  implementation would try to parse a 0-byte or half-written file on the
  first `Write` event of a multi-write save (common with editors that
  truncate-then-write or write via temp-file-then-rename). Needs either a
  short debounce window per path or reliance on the editor's
  temp-file+atomic-`rename` pattern (which most editors — vim, VSCode —
  actually use, meaning the final event is a `Rename`/`Create` of a
  complete file, not a `Write` mid-stream) — worth explicitly testing both
  save styles.
- Symlinks: `os.ReadDir` + a naive `*.toml` glob will follow a symlinked
  `.toml` file's target transparently for `os.ReadFile`, but a symlinked
  *directory* inside `~/.stapler-squad/detectors/` needs an explicit
  decision (follow it like `history_watcher.go`'s subdirectory walk does,
  or ignore non-regular-file dirents) — should be a stated, tested choice,
  not an accident of whatever `os.ReadDir`/`filepath.WalkDir` happens to do.
- Very large detector directories: no stated scale requirement, but the
  loader should be O(n) per reload (not O(n²), e.g. don't re-scan +
  re-validate every other file when only one file changed) — the
  atomic-pointer-swap-of-a-whole-new-registry pattern from `detector.go`
  makes "just rebuild everything on any change" the simplest correct
  approach; only worth optimizing to incremental rebuild if directory
  sizes in practice turn out to be large (no evidence of that requirement
  here — YAGNI unless research turns up a real number).
- Concurrent reload while a session is actively being matched: solved
  by the existing `atomic.Pointer[PatternSet]` swap pattern in
  `detector.go` (§1.1) — readers never block on a write, and never see a
  half-updated set. The registry-level merge (built-ins + plugins) needs
  the same treatment at the `DetectorRegistry` level: build the complete
  merged registry off to the side, then swap a pointer to it, rather than
  mutating a shared `map[string]BinaryDetector` in place while lookups are
  in flight from other goroutines.
- Directory doesn't exist yet on first run — `history_watcher.go`
  already establishes the "log a warning, degrade gracefully, don't error"
  precedent (§1.4); requirement #6 additionally wants the directory
  *created* (not just tolerated absent), which is a step further than the
  existing precedent takes it, but consistent in spirit.

## 5. Summary of design implications for the planning phase

1. Reuse `detector.go`'s `atomic.Pointer[PatternSet]`-swap idiom at both the
   `PatternSet` level (already there) and the `DetectorRegistry` level (new)
   — don't invent new locking.
2. `DetectorRegistry.Register`'s panic-on-duplicate is a tested invariant
   that must stay intact for its existing callers; the plugin-merge path
   needs a separate, explicit override mechanism.
3. TOML parsing needs a new dependency — compare `BurntSushi/toml` vs.
   `pelletier/go-toml/v2` in the stack-research phase, weighing field-level
   error reporting quality (helps requirement #3's actionable-error bar)
   against the fact `gopkg.in/yaml.v3` (already a dependency) proves the
   codebase is comfortable depsing a config-format library per format.
4. Validation must run two independent collision checks (user-vs-user:
   reject; user-vs-built-in: allow-and-override) rather than one generic
   "already claimed" check.
5. Go's RE2-backed `regexp` already forecloses catastrophic backtracking;
   the NFR's "guard" ask is about compile-time/memory cost of pathological
   patterns, not ReDoS — a simple max-length cap on regex source covers it.
6. A dry-run/test-against-captured-output affordance and a
   which-detector-matched-and-why debug affordance are both cheap
   (reuse `PatternSet.MatchLines` and `GetPatternNames`) and address the
   two biggest unstated needs (§3.1, §3.4) — worth scoping into this item
   rather than deferring, even though neither is in the explicit
   requirements.
