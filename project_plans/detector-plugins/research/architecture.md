# Architecture Research: TOML-Based User-Extensible Detector Plugins

No prior architecture research for this item exists in `project_plans/*/research/architecture.md`
(all ~75 existing files were checked by name; none cover `session/detection`). This is a fresh pass.

## 1. Current Architecture, Precisely

### 1.1 The three layers involved

**`dtypes` package** (`session/detection/dtypes/dtypes.go`) — the shared vocabulary, imported by
both `session/detection` and `session/detection/binaries` to avoid an import cycle
(`dtypes.go:1-4`):

- `StatusPattern` (`dtypes.go:7-12`): `Name`, `Pattern` (regex string), `Description`, `Priority int`.
- `StatusPatterns` (`dtypes.go:15-26`): ten `[]StatusPattern` slices, one per status category
  (`Ready`, `Processing`, `NeedsApproval`, `InputRequired`, `Error`, `TestsFailing`, `Idle`,
  `Active`, `Success`, `WaitingForAgent`) — this is exactly the ten categories the TOML plugin
  schema's `status` field must validate against.
- `BinaryDetector` interface (`dtypes.go:29-33`): three methods — `Name() string`,
  `Patterns() StatusPatterns`, `FilterContent(content string) string`. This is the interface a
  TOML-constructed detector must satisfy.

**`binaries` package** (`session/detection/binaries/*.go`) — one concrete struct per built-in
agent (`ClaudeDetector`, `GeminiDetector`, `AiderDetector`, `OpencodeDetector`, `AgyDetector`),
each a zero-field struct whose `Patterns()` method returns a hard-coded `dtypes.StatusPatterns`
literal (`binaries/claude.go:7-16` for the type/constructor, `:18-21` for `Patterns()`'s start).
A plugin-backed detector is structurally identical to these — a `Name()`, a `StatusPatterns`
value, and (per requirements, out of scope for real filtering) a no-op `FilterContent`.

**`detection` package registry + compiler + StatusDetector**:

- `DetectorRegistry` (`session/detection/binary_detector.go:4-40`) is a plain
  `map[string]BinaryDetector` wrapper: `NewDetectorRegistry()` (`:9-11`), `Register(d)`
  (`:15-20`, **panics** on duplicate name — `:16-18`), `Lookup(name)` (`:23-26`), `Names()`
  (`:29-35`), `Len()` (`:38-40`). No locking anywhere — it is built once and never mutated again.
- `DefaultRegistry()` (`session/detection/registry.go:6-14`) constructs a `DetectorRegistry` and
  registers the five built-ins by calling their `New*Detector()` constructors.
- `PatternSet` (`session/detection/pattern_set.go:10-23`) holds ten `[]*regexp.Regexp` slices
  compiled from a `StatusPatterns` value. Doc comment at `pattern_set.go:9`: **"Immutable after
  `NewPatternSet` returns — no lock needed."** `NewPatternSet` (`:26-32`) calls `compile()`
  (`:35-65`), which iterates all ten groups and calls `regexp.Compile` per pattern, returning a
  wrapped error identifying the group label and pattern name on first failure (`:56-59`) — this
  is the exact validation error shape the plugin loader should reuse/mirror.
  `MatchLines` (`:69-141`) runs the fixed priority chain (Error → TestsFailing → NeedsApproval →
  InputRequired → readline-typing special-case → WaitingForAgent → Success → Active → Processing
  → screen-overwrite fallback → Idle → Ready-catch-all) — this priority order is a cross-cutting
  invariant plugins do not get to change; a plugin only contributes additional patterns *into*
  existing categories.
- `StatusDetector` (`session/detection/detector.go:48-52`) wraps `atomic.Pointer[PatternSet]`
  (`:49`) — **this is the one place in the current codebase where a `PatternSet` is swapped at
  runtime**: `NewStatusDetectorFromFile` / `LoadPatterns` (`:71-115`) read a YAML file, build a new
  `PatternSet`, and `sd.patternSet.Store(newSet)` (`:113`). Reads go through
  `detectFromText`/`ps := sd.patternSet.Load()` (`:250-251`) — lock-free reads, single atomic
  store on write. **This YAML hot-swap mechanism already exists for a single detector's patterns**
  but is never wired to a filesystem watcher today (no fsnotify call anywhere in
  `session/detection/`) — a caller would have to invoke `LoadPatterns` explicitly. There is no
  equivalent "swap the whole registry" primitive yet.

### 1.2 Where `DefaultRegistry()` is actually consumed — the critical finding

`grep -rn "DefaultRegistry("` (excluding tests) returns exactly one call site, and it is **not**
in `main.go`, `daemon/daemon.go`, or any RPC/service wiring:

```go
// session/detection/detector.go:733-746
var builtBinaryDetectors = func() map[string]*StatusDetector {
    m := make(map[string]*StatusDetector)
    reg := DefaultRegistry()
    for _, name := range reg.Names() {
        bd, _ := reg.Lookup(name)
        ps, _ := NewPatternSet(bd.Patterns())
        bsd := &StatusDetector{}
        bsd.patternSet.Store(ps)
        m[name] = bsd
    }
    return m
}()
```

This is a **package-level `var` initialized by an immediately-invoked function literal** — Go
runs this during package initialization (before `main()`), not as part of any explicit app
startup sequence. `DetectForProgram` (`detector.go:753-763`) reads `builtBinaryDetectors[program]`
as a plain map index (`:754`) — no lock, because the map is built once at process start and never
written again. This is the load-bearing consequence for hot-reload design: **there is no existing
"startup path" call site to hook a loader into** — the mechanism that needs to become mutable is
this package-level map itself, and the natural trigger point becomes an explicit initialization
call (e.g. from `main.go`/`daemon.go`) rather than package-init magic, since package `var`
initializers cannot depend on runtime state like `~/.stapler-squad/detectors/` contents changing
later.

`session/detection/registry.go:6` is otherwise dead as a runtime API — `DefaultRegistry()` itself
is called nowhere else in non-test code. `session/backlog_plugin.go`'s unrelated `PluginRegistry`
(`NewDefaultRegistry` at `session/backlog_plugin.go:52-57`, consumed at
`server/dependencies.go:924`) is a different plugin system entirely (GitHub issue/PR source
plugins) but is a useful sibling precedent: a `map[string]ItemSourcePlugin`
(`backlog_plugin.go:31-33`) built once by `Register` calls, no runtime mutation, no hot-reload —
i.e., today's whole codebase has **zero** examples of a hot-reloadable registry; the closest
precedent for the *mechanism* (not the registry shape) is elsewhere (see §2).

## 2. Architectural Patterns for "load declarative config → validate → merge with built-ins → hot-reload"

### 2.1 Registry pattern — already established, reusable almost as-is

`DetectorRegistry`'s shape (map + Register/Lookup/Names) is the right level of abstraction; the
plugin loader does not need a new registry type. It needs a **constructor that composes two
sources** rather than mutating `DefaultRegistry()` in place:

```go
func MergedRegistry(builtins *DetectorRegistry, userPlugins []BinaryDetector) *DetectorRegistry
```

Because `Register` panics on duplicate name (`binary_detector.go:16-18`), and requirement #4
requires **user plugins to override same-named built-ins**, the merge constructor cannot just
call `Register` for both sets in registration order — it must special-case "user plugin
overrides built-in of the same name" (a decorator/precedence-override, not a raw merge), e.g.
build the map directly: seed with built-ins, then overwrite (not `Register`, which would panic)
with user entries. This is the "decorator/precedence override" pattern called out in the research
question — concretely, just "last write wins keyed by binary name in a plain map build step," no
GoF Decorator wrapping needed since `BinaryDetector` has no shared cross-cutting behavior to layer.

### 2.2 Watcher goroutine + debounce + atomic swap of an immutable snapshot

This is the right shape, and the repo already has **two independent precedents** for both halves:

**Debounced fsnotify watcher** — `session/unfinished/watcher.go`'s `WatchDirWatcher`
(`fsnotifyLoop`, `:145-188`): a goroutine reading `w.watcher.Events`/`w.watcher.Errors`, checking
`event.Has(fsnotify.Write) || event.Has(fsnotify.Create)`, then invalidating and re-enqueueing.
It does not itself debounce (it enqueues into a scanner queue that presumably absorbs bursts), but
`session/external_tmux_streamer.go:358-382`'s `debounceCaptures` shows the canonical debounce-timer
idiom in this repo: a `time.NewTimer(debounceDelay)` (`:365`, using a 50ms constant at `:364`),
reset on each notification (`:382`), firing the real work only after quiet. The plugin watcher
should combine both: fsnotify events on `~/.stapler-squad/detectors/*.toml` → reset a debounce
timer (a few hundred ms, since editors often do write+rename or multiple writes per save) → on
fire, re-scan the whole directory and rebuild.

**Atomic copy-on-write swap of a whole map** — `session/pipeline_engine.go`'s
`pipelineModeCache` (`:123-140`) is the closest structural precedent in the entire codebase:
`atomic.Pointer[map[string]resolvedPipelineMode]` plus a `sync.Mutex` (`writeMu`) that serializes
writers across the *entire* read-modify-write sequence, not just the final `Store`
(`refresh`, `:162-189`, doc comment at `:129-136` explains why: holding the mutex only around the
`Store` would let a slower-starting-but-slower-finishing writer's stale data land after a faster
writer's fresh data — a lost update). The plugin registry should copy this exactly:

```go
type pluginRegistryCache struct {
    ptr     atomic.Pointer[DetectorRegistry] // or map[string]*StatusDetector, matching builtBinaryDetectors' shape
    writeMu sync.Mutex
}

func (c *pluginRegistryCache) reload(dir string) error {
    c.writeMu.Lock()
    defer c.writeMu.Unlock()
    // 1. scan dir for *.toml
    // 2. parse + validate each (log-and-skip invalid files, per FR #3)
    // 3. build BinaryDetector per valid file
    // 4. build a *new* DetectorRegistry/map: builtins, then user overrides
    // 5. atomic Store of the fully-built new snapshot
    ...
}
```

`writeMu` matters here specifically because two fsnotify events (e.g. edit + rename-into-place)
firing close together, after debounce, could still race two `reload` calls; without the mutex
serializing the *whole* rescan+rebuild, the slower one (reading a half-written file, or racing a
delete) could overwrite the faster one's more-current result.

### 2.3 Double-checked-locking / "return the locally-computed value" rule

`.claude/rules/go-double-checked-locking.md` warns against re-reading the shared slot after a
guarded write and returning *that* instead of the value this goroutine just computed, because a
lost write race means the slot may hold a different goroutine's result. This rule is about
**read-after-write inside one call**, and does not directly apply to `reload()` above, because
`reload()` doesn't need to return anything to its caller (the caller is the watcher's debounce
timer firing, not a request needing a value back) — it only needs to `Store` and return. It
**does** apply if a future RPC ("reload detectors now" button, an explicit `POST /plugins/reload`)
is added that both triggers a reload and wants to report back the resulting `Names()`/error list to
the caller: that handler must return the locally-built list from its own `reload` invocation, not
re-`Load()` the shared pointer afterward — otherwise a concurrent second reload finishing first
would make the first caller report the second caller's result set instead of its own. Worth
calling out explicitly in the plan if a manual-reload RPC is in scope; not applicable to the
watcher-only path in this item's requirements.

### 2.4 Compilation/validation reuses `PatternSet` unchanged

Per NFR, plugins must produce the same shape of data as built-ins and go through the same
compile path. Concretely: TOML `[[patterns]]` blocks (`name`, `regex`, `status`) map 1:1 onto
`dtypes.StatusPattern{Name, Pattern, Description, Priority}` (status string selects which of the
ten `StatusPatterns` slice fields to append to; `Priority` can default to something reasonable
since the TOML schema in the requirements doesn't mention it) — then `NewPatternSet(patterns)`
(`pattern_set.go:26-32`) is called exactly as today, so `regexp.Compile` failures are caught with
the same wrapped-error shape (`:56-59`) the loader should surface per-file/per-field (FR #3).
Because `regexp` in Go is RE2-based (linear time, no backtracking), the NFR's catastrophic-
backtracking concern is already structurally mitigated by using the same `regexp.Compile` call
built-ins use — no additional guard is needed beyond what's already true of the existing
mechanism; only a defensive max-pattern-length/max-pattern-count sanity check (arbitrary-size file
guard, not a regex-safety guard) might be worth adding at the loader boundary, since RE2 protects
against exponential blowup but not against a maliciously huge single pattern string.

## 3. Integration Points

- **No existing call site to hook into `DefaultRegistry()`'s single consumer** — `detector.go:735`
  is a package-`var` initializer, which runs at import time and cannot depend on runtime
  filesystem state or be re-triggered later. The hot-reload registry therefore needs its own
  explicit initialization + `Start(ctx)` call from an actual startup path (`main.go` or
  `daemon/daemon.go`), analogous to how `WatchDirWatcher.Start(ctx)` is presumably invoked from
  wherever `unfinished.Scanner` wiring happens today. `builtBinaryDetectors` as a bare package
  `var` should become something like a package-level (or dependency-injected) `*pluginAwareRegistry`
  whose zero-plugin-directory behavior is identical to today's built-ins-only map, so all existing
  callers of `DetectForProgram` (`detector.go:753`) keep working unmodified if the user has no
  `~/.stapler-squad/detectors/` files.
- **`config.GetConfigDir()`** (`config/config.go:117-119`, resolving to `~/.stapler-squad` under
  normal operation, with test/instance overrides — see doc comment `:110-116`) is the existing
  helper for locating the base state dir; `~/.stapler-squad/detectors/` should be derived from it
  (`filepath.Join(configDir, "detectors")`) rather than hard-coding `os.UserHomeDir()`, so it
  automatically respects `STAPLER_SQUAD_TEST_DIR`/`STAPLER_SQUAD_INSTANCE` isolation (important for
  e2e tests per this repo's test-isolation conventions) and multi-instance state dirs
  (`.claude/docs/state-isolation.md`).
- **Long-lived holders of a stale registry reference**: every current call site
  (`session/claude_controller.go:224`, `session/detection/idle.go:84,102`,
  `session/startup_scanner.go:25`, `session/review_queue_poller.go:146`) calls
  `detection.NewStatusDetector()` — a *generic* detector using `getDefaultPatterns()`
  (`detector.go:304`), **not** the per-binary `builtBinaryDetectors` map. Only
  `DetectForProgram` (`:753`) reads the per-binary map, and it reads it fresh on every call (plain
  map index, no caching of the map itself inside a long-lived struct) — so as long as the shared
  map/registry becomes an `atomic.Pointer`-backed swappable value read via `.Load()` on each
  `DetectForProgram` call (mirroring `StatusDetector.patternSet.Load()` at `:250`), there is
  **no long-lived goroutine or session object holding a stale copy of the whole registry** that
  would need explicit invalidation — every `DetectForProgram` call already re-reads at call time.
  The risk is narrower than it first appears: it's confined to the one global variable, not
  scattered across session objects.

## 4. Data Flow and Consistency Requirements

**In-flight session mid-reload**: because `DetectForProgram` (`detector.go:753-763`) takes no
detector reference as state — it looks up the shared map fresh on every single invocation (each
call to `Detect()`/`DetectForProgram()` happens on a polling cadence, e.g. driven by
`idle.go`'s ~100ms-scale polling per `claude_controller.go`'s doc comments) — **a plugin file
change takes effect on the very next detection call after the atomic `Store` completes, not just
for new sessions.** This matches the "atomic swap of an immutable snapshot" framing in the
research question directly: there is no per-session cached detector copy to go stale, so no
special-casing of "existing session keeps old detector, new sessions get new detector" is needed
or desirable — every `PatternSet`/`StatusPatterns` value here is already immutable-after-construction
by convention (`pattern_set.go:9`'s doc comment), so the swap is a clean instant cut-over, not a
gradual migration.

**Failure isolation**: FR #3 requires one bad file not to block others. This falls directly out of
the `reload()` shape in §2.2 — parse+validate happens per-file inside the loop that builds the
*next* map; a failure just means "log and `continue`, don't add this entry," and the already-valid
`Store()` of the previous snapshot is untouched until the new build completes. The **whole
directory scan either fully replaces the snapshot or (on a directory-read-level error, not a
per-file error) leaves the old snapshot in place** — never a partial half-applied state, since the
new map is built completely off to the side before the single atomic `Store`.

**Startup ordering**: directory bootstrap (FR #6, creating `~/.stapler-squad/detectors/` and an
example file on first run) must happen *before* the first `reload()`/initial scan, and before the
fsnotify watch is registered on that directory (watching a nonexistent directory is either an
error or silently watches nothing, depending on the fsnotify backend) — mirrors
`WatchDirWatcher.Start` (`watcher.go:40-53`) doing an initial walk before starting the event loop
goroutine.

## 5. EventStorming Table (file-watch → validate → merge → swap)

Included because the flow has enough distinct failure branches (per-file invalid vs. directory
error vs. override collision) that a compact command/event/policy view clarifies ordering better
than prose alone.

| Command | Event | Policy (reaction) |
|---|---|---|
| `ScanPluginDir` (startup, or debounced fsnotify fire) | `PluginDirScanned{files}` | For each `.toml` file → `ParsePluginFile` |
| `ParsePluginFile(path)` | `PluginFileParsed{id, binaryNames, patterns}` or `PluginFileRejected{path, field, reason}` | On `Parsed` → `ValidatePluginFile`; on `Rejected` → log-and-skip (continue scan) |
| `ValidatePluginFile` (required fields, regex compile via `NewPatternSet`, known `status` values, no collision with another *user* file) | `PluginFileValid{detector}` or `PluginFileRejected{path, field, reason}` | On `Valid` → accumulate into next-snapshot build; on `Rejected` → log-and-skip |
| `BuildMergedSnapshot` (built-ins + accumulated valid user detectors, user wins on binary-name collision) | `SnapshotBuilt{registry}` | → `SwapRegistry` |
| `SwapRegistry` (guarded by `writeMu`, single `atomic.Pointer.Store`) | `RegistrySwapped{names}` | Next `DetectForProgram` call reads new snapshot — no further action needed (no session holds a stale reference, per §3) |
| *(directory-level read error, e.g. permissions)* | `PluginDirScanFailed{err}` | Log; **do not** swap — previous snapshot remains authoritative |

## Summary of Concrete File-Level Anchors for the Plan Phase

| Concern | File:Line |
|---|---|
| `BinaryDetector` interface to satisfy | `session/detection/dtypes/dtypes.go:29-33` |
| `StatusPatterns`/`StatusPattern` shape TOML maps onto | `session/detection/dtypes/dtypes.go:7-26` |
| `DetectorRegistry` (map, Register/panic-on-dup/Lookup/Names) | `session/detection/binary_detector.go:4-40` |
| `DefaultRegistry()` — built-ins only, single call site | `session/detection/registry.go:6-14`, consumed at `session/detection/detector.go:735-746` |
| `PatternSet` compile + immutability doc comment | `session/detection/pattern_set.go:9,26-65` |
| Existing atomic-swap precedent (single detector) | `session/detection/detector.go:49,71-115,250-251` |
| Existing atomic-swap-of-a-map precedent (best structural match) | `session/pipeline_engine.go:123-189` |
| Existing fsnotify watcher goroutine precedent | `session/unfinished/watcher.go:145-188` (event loop), `:40-53` (initial-walk-then-watch ordering) |
| Existing debounce-timer idiom | `session/external_tmux_streamer.go:358-382` |
| Config/state dir helper for `~/.stapler-squad/detectors/` | `config/config.go:117-123` |
| `fsnotify` already a dependency | `go.mod:16` (`github.com/fsnotify/fsnotify v1.9.0`) — no new dependency needed for the watcher |
| TOML parsing library | **not yet a dependency** — `go.mod` has no `toml` entry; only `gopkg.in/yaml.v3` is used today (`detector.go:13`) for the existing `LoadPatterns`/`ExportPatterns` YAML path. A TOML library (e.g. `BurntSushi/toml` or `pelletier/go-toml/v2`) must be added. |
