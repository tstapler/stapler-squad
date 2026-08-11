# Stack Research: TOML Detector Plugins + Hot-Reload

## 1. TOML parsing library

**Current state**: no TOML library is a dependency today. `grep -iE "toml|viper" go.mod go.sum` returns nothing except `github.com/go-viper/mapstructure/v2` — an *indirect* transitive dep (pulled in by something in the module graph, not Viper itself; Viper is not in `go.mod`). Existing structured-config parsing in this repo uses `gopkg.in/yaml.v3` (see `dtypes.StatusPattern`'s `yaml:"..."` struct tags) and plain JSON for `config.json`/`sessions.json`. So this is a genuinely new dependency, not a "just use what's already there" situation.

**Recommendation: `github.com/pelletier/go-toml/v2`.**

- `github.com/BurntSushi/toml` is effectively unmaintained at this point; community consensus (and go-toml's own docs) point new consumers at go-toml v2.
- go-toml v2 benchmarks 2–5x faster than BurntSushi on unmarshal-heavy workloads — relevant here because hot-reload means re-parsing on every filesystem event, not just once at startup.
- API is the standard `Marshal`/`Unmarshal`/struct-tag (`toml:"..."`) shape, same ergonomics as the existing `yaml.v3` usage in `dtypes.go` — trivial to review against that precedent.
- v2 supports TOML 1.0 (and has an in-progress 1.1.0 unstable path) — no spec-compliance gaps expected for a schema this simple (strings, string lists, arrays of tables).
- Not currently vendored anywhere in the repo — this will be a net-new `require` in `go.mod`. No conflicting version already pinned by another dependency's transitive graph, so no minimum-version-selection surprises expected.

**Do not use**: `BurntSushi/toml` (unmaintained), or pulling in all of Viper just for TOML decoding — Viper is a full configuration-management framework (env var binding, config-file watching, multiple format support) and would be a large speculative dependency for what's just "parse one TOML file into a struct." The plugin loader's own hot-reload logic (fsnotify, below) already covers the "watch a file" need Viper would otherwise provide.

## 2. Filesystem watching for hot-reload

**Current state**: `github.com/fsnotify/fsnotify v1.9.0` is already a direct dependency (`go.mod` require block) and already used in three places worth reading before writing the plugin watcher:

- `session/unfinished/watcher.go` (`WatchDirWatcher`) — the cleanest reference pattern. Notably: it treats `fsnotify.NewWatcher()` failure as non-fatal (`log.Warn` + fall back to a nil watcher / polling-only mode), and it runs the `fsnotify.Watcher.Events`/`.Errors` select loop as a `go` goroutine gated behind `ctx.Done()`, with a *second*, independent polling fallback (`periodicReWalk`, a `time.Ticker` every 60s) that re-scans regardless of whether fsnotify is working. That dual-path (event-driven + periodic safety-net poll) is the idiom to copy for the detector plugin directory watcher, both because it's the established local convention and because it protects against the "watching a file doesn't work well" caveat below.
- `session/history_watcher.go`, `session/mux/autodiscover.go`, `session/unfinished/gogitstore/mmapwatch.go` — other independent fsnotify consumers, confirming this is the repo's standing convention for "watch a directory, react to changes" rather than a one-off.

**Version**: v1.9.0 (already pinned) is current — no version bump needed. v1.9.0 (Apr 2025) fixed several inotify race conditions around concurrent watch add/remove and buffered-watcher behavior; nothing in the changelog since suggests an upgrade is needed for this feature.

**fsnotify v1 vs v2**: fsnotify does not have a "v2" — v1.9.0 *is* current mainline (the module path is `github.com/fsnotify/fsnotify`, no `/v2` suffix). Don't confuse this with unrelated "v2" naming in other libraries (e.g. go-toml v2, mapstructure v2) encountered during this same research pass.

**Known gotcha — editors don't write in place**: fsnotify's own docs are explicit about this ("Watching a file doesn't work well"): editors/programs that save atomically write a temp file then rename it over the original. The watch on the *original inode* is then gone — renames/moves invalidate a file-level watch. **Fix: watch the parent directory (`~/.stapler-squad/detectors/`), not individual `.toml` files**, and filter by `event.Name`/extension inside the handler. This matches the existing `WatchDirWatcher` pattern (it watches `.git` directories, not individual git files) and is exactly what the requirements doc's "add, edit, remove" hot-reload requirement implies watching a directory anyway.

**Debouncing rapid events**: a single editor save can fire 4–12 raw fsnotify events (write, chmod, rename, temp-file create/delete) within a ~20ms window. Standard idiom: buffer/coalesce events per-file behind a short timer (e.g. 100–300ms) and only trigger one reload per settled file, resetting the timer on each new event for that path — same shape as the classic "debounce" pattern from JS build tooling, ported to a `time.Timer` reset in the event-loop `select`. fsnotify's own `cmd/fsnotify` examples include a dedup/debounce reference implementation worth pulling up during implementation; this repo doesn't have a debounce helper today (`WatchDirWatcher`'s loop reacts immediately to `Write`/`Create` events without debouncing — acceptable there because `InvalidateCache`+`EnqueueRepo` are cheap and idempotent; the TOML plugin case should still debounce since each event triggers a full parse+validate+registry-swap cycle).

**API idiom to follow** (from `WatchDirWatcher`):
```go
watcher, err := fsnotify.NewWatcher()
if err != nil {
    log.Warn("fsnotify unavailable, falling back to polling", "err", err)
    // fall back to periodic-only re-scan, don't fail startup
}
// ...
if err := watcher.Add(detectorsDir); err != nil { ... }
for {
    select {
    case <-ctx.Done():
        return
    case event, ok := <-watcher.Events:
        if !ok { return }
        if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
            // debounce, then reload+re-validate the single affected file
        }
    case err, ok := <-watcher.Errors:
        if !ok { return }
        log.Warn("fsnotify error", "err", err)
    }
}
```
Note the plugin case needs `Remove`/`Rename` handling too (built-in `WatchDirWatcher` only checks `Write`/`Create` because it only cares about `.git` content changing, not files disappearing) — requirement #5 explicitly includes file removal as a hot-reload trigger.

## 3. Existing regex compilation / pattern matching (`session/detection/pattern_set.go`)

- `PatternSet` (constructed via `NewPatternSet(p dtypes.StatusPatterns) (*PatternSet, error)`) is **immutable after construction** — compiles all 10 category slices (`Ready`, `Processing`, `NeedsApproval`, `InputRequired`, `Error`, `TestsFailing`, `Idle`, `Active`, `Success`, `WaitingForAgent`) into `[]*regexp.Regexp` up front via `regexp.Compile`, comment explicitly notes "no lock needed" because of this immutability.
- Compile errors are wrapped with context: `fmt.Errorf("failed to compile %s pattern %q: %w", categoryLabel, patternName, err)` — the plugin loader's own validation errors should follow this same "which category/which named pattern" shape so failures are actionable per requirement #3.
- `MatchLines(text string, rawPTY []byte) (DetectedStatus, string, string)` runs categories in a fixed priority order (error → tests_failing → needs_approval → input_required → readline-typing special-case → waiting_for_agent → success → active → processing → screen-overwrite fallback → idle → ready-catch-all) — this priority chain is a hardcoded property of `PatternSet.MatchLines`, not data-driven by the `StatusPatterns` struct itself. **Implication for the plugin loader**: as long as a plugin populates the same `dtypes.StatusPatterns` struct (just via TOML `[[patterns]]` → grouped into the right named slice by its `status` field), it gets this exact same priority chain for free — zero new matching logic needed, which is exactly what the requirements' "reused as-is" non-functional requirement asks for.
- **RE2 / ReDoS**: Go's `regexp` package (used here) compiles to RE2, which is guaranteed linear-time in input length with no catastrophic-backtracking failure mode — unlike PCRE-style backtracking engines. This directly satisfies the non-functional requirement about hangs from malicious regexes: **no additional protection is needed against ReDoS specifically**, because `regexp.Compile`/`MatchString` cannot backtrack pathologically by construction. The only remaining guard worth adding is a simple bound on pattern *count*/*length* per plugin file (e.g. reject patterns over some KB threshold, or files with an absurd number of `[[patterns]]` blocks) purely to prevent resource exhaustion (many regexes × long lines checked per PTY read), not correctness/hang risk — this is a cheap sanity check, not a security-critical mitigation.

## 4. `dtypes.BinaryDetector` interface (`session/detection/dtypes/dtypes.go`)

```go
// StatusPattern represents a regex pattern for detecting a specific status.
type StatusPattern struct {
    Name        string `yaml:"name"`
    Pattern     string `yaml:"pattern"`
    Description string `yaml:"description"`
    Priority    int    `yaml:"priority"` // Higher priority patterns checked first
}

// StatusPatterns contains all patterns for status detection.
type StatusPatterns struct {
    Ready           []StatusPattern `yaml:"ready"`
    Processing      []StatusPattern `yaml:"processing"`
    NeedsApproval   []StatusPattern `yaml:"needs_approval"`
    InputRequired   []StatusPattern `yaml:"input_required"`
    Error           []StatusPattern `yaml:"error"`
    TestsFailing    []StatusPattern `yaml:"tests_failing"`
    Idle            []StatusPattern `yaml:"idle"`
    Active          []StatusPattern `yaml:"active"`
    Success         []StatusPattern `yaml:"success"`
    WaitingForAgent []StatusPattern `yaml:"waiting_for_agent"`
}

// BinaryDetector provides per-binary pattern sets and optional content filtering.
type BinaryDetector interface {
    Name() string
    Patterns() StatusPatterns
    FilterContent(content string) string
}
```

`session/detection/detector.go` re-exports these as type aliases (`type StatusPattern = dtypes.StatusPattern`, `type StatusPatterns = dtypes.StatusPatterns`) inside the `detection` package itself, so both names work interchangeably depending on import.

Built-in detectors (`session/detection/binaries/claude.go` et al.) are trivial structs (`type ClaudeDetector struct{}`) with a `Patterns()` method that returns a literal `dtypes.StatusPatterns{...}` — no shared base type, no constructor complexity. **A TOML-backed detector is a drop-in, structurally identical implementation**: a small struct holding a `name string` and a pre-populated `dtypes.StatusPatterns` (built by grouping the TOML file's `[[patterns]]` entries by their `status` string into the matching struct field), with `Name()`, `Patterns()`, and `FilterContent()` (can simply return content unchanged, matching `ClaudeDetector`'s no-op). No interface changes needed — confirms requirement #1's premise cleanly.

**Registry mechanics — important finding for the plan phase**: `session/detection/binary_detector.go`'s `DetectorRegistry` is a bare `map[string]BinaryDetector` with **no mutex** (fine today because it's built once at startup and never mutated after) and its `Register(d BinaryDetector)` method **panics on a duplicate name**:
```go
func (r *DetectorRegistry) Register(d BinaryDetector) {
    if _, exists := r.detectors[d.Name()]; exists {
        panic("detection: duplicate BinaryDetector registered for name: " + d.Name())
    }
    r.detectors[d.Name()] = d
}
```
This has two consequences the plan phase needs to account for:
1. **Override requires a new path, not `Register`.** Requirement #4 (user plugin overrides a built-in with the same binary name) cannot use the existing `Register` method as-is — it would panic. The loader needs either a new non-panicking upsert method (e.g. `RegisterOverride`) or to construct the merged registry by iterating built-ins first and only calling `Register` for names the user hasn't overridden, then separately upserting user overrides into the map directly.
2. **Hot-reload needs a concurrency story `DetectorRegistry` doesn't have today.** Since the registry is read from concurrently (session status-detection is presumably on a hot path per PTY read) while hot-reload wants to swap entries at runtime, a mutable-in-place map is not safe as the registry stands. Two established options already available in this codebase: (a) an `atomic.Pointer[DetectorRegistry]` copy-on-write swap — build a whole new immutable `DetectorRegistry` on every reload and atomically swap the pointer, cheapest to reason about and consistent with this repo's stated preference for copy-on-write over mutex-guarded mutation (see `go-concurrency` skill / `.claude/rules/go-double-checked-locking.md`); or (b) `github.com/puzpuzpuz/xsync/v4` (already a direct dependency, `xsync.MapOf`) for a concurrent map if per-key granular updates are preferred over whole-registry replacement. Given the registry is small (a handful of binaries) and reloads are rare (human edits a file), **copy-on-write via `atomic.Pointer[DetectorRegistry]` is the simpler, lower-risk choice** — avoids adding lock contention to what's likely a per-PTY-read lookup, and sidesteps `xsync`'s API being a bigger behavior change than needed here.

## Summary of concrete recommendations

| Concern | Recommendation |
|---|---|
| TOML parsing | `github.com/pelletier/go-toml/v2` (new dependency; BurntSushi is unmaintained, Viper is overkill) |
| Filesystem watching | `github.com/fsnotify/fsnotify` v1.9.0 (already a dependency) — watch the directory, not individual files |
| Debounce | Add a small per-file debounce timer (100–300ms) around the existing `WatchDirWatcher`-style event loop; no existing debounce helper to reuse |
| Pattern compilation | Reuse `dtypes.StatusPatterns` + `NewPatternSet` unmodified — just populate the struct from parsed TOML |
| ReDoS protection | None needed beyond what Go's RE2-based `regexp` already guarantees (linear-time); optionally cap pattern count/length per file for resource-exhaustion hygiene only |
| Registry merge/override | New non-panicking upsert path needed — `DetectorRegistry.Register` panics on duplicate names today |
| Hot-reload concurrency | `atomic.Pointer[DetectorRegistry]` copy-on-write swap on each reload (repo convention; avoids adding a mutex to a likely-hot lookup path) |

## Sources

- [pelletier/go-toml (v2 branch, README)](https://github.com/pelletier/go-toml/blob/v2/README.md)
- [go-toml v2 plan discussion #506](https://github.com/pelletier/go-toml/discussions/506)
- [fsnotify/fsnotify README — "Watching a file doesn't work well"](https://github.com/fsnotify/fsnotify)
- Local repo: `session/unfinished/watcher.go`, `session/detection/pattern_set.go`, `session/detection/dtypes/dtypes.go`, `session/detection/binary_detector.go`, `session/detection/binaries/claude.go`, `session/detection/registry.go`, `go.mod`
