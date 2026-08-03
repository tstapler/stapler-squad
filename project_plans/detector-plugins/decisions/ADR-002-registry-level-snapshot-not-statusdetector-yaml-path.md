# ADR-002: Registry-Level Copy-on-Write Snapshot, Not the Existing `StatusDetector` YAML Loader

## Status
Accepted — 2026-08-01

## Context

There are two plausible places to hang user-defined detectors, and the repo
already contains a partially-built, **completely unused** implementation of
one of them.

### The existing, unused YAML machinery

`session/detection/detector.go` already has a full file→patterns→atomic-swap
pipeline:

- `validatePatternFilePath(path)` (`detector.go:63-68`) — rejects `..`
- `NewStatusDetectorFromFile(path)` (`detector.go:71-92`) — reads a YAML file,
  `yaml.Unmarshal` into `StatusPatterns`, `NewPatternSet`, `Store`
- `(sd *StatusDetector) LoadPatterns(path) error` (`detector.go:95-115`) —
  same, but swaps into an existing detector via
  `sd.patternSet.Store(newSet)` (`detector.go:113`)
- `StatusDetector.patternSet` is already an `atomic.Pointer[PatternSet]`
  (`detector.go:49`), read lock-free on the hot path.

Grep confirms **none of these have a call site outside `detector.go` and its
own tests**. This is a foundation that was built and never wired up.

### Where per-binary detection actually happens

`DetectForProgram` (`detector.go:753-763`) is the only consumer of per-binary
detectors. It reads a package-level map:

```go
// detector.go:735-746
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

This is a package-level `var` initialized by an IIFE at Go package-init time —
**before `main()` runs**, before config or logging is initialized, and with no
`context.Context` in scope. `DetectForProgram` reads it as a plain,
unsynchronized map index (`detector.go:754`).

`DefaultRegistry()` (`registry.go:6-14`) has exactly this one consumer.

## Decision

**Build new machinery at the registry level. Leave the existing
`StatusDetector` YAML loader untouched and unused.**

Concretely:

1. **Reuse the compile step unchanged.** A plugin's patterns are converted to
   a `dtypes.StatusPatterns` value and compiled by the existing
   `NewPatternSet` (`pattern_set.go:26-32`). Plugin patterns therefore get the
   exact same fixed priority chain in `PatternSet.MatchLines`
   (`pattern_set.go:69-141`) that built-ins get, for free. This satisfies the
   non-functional requirement that "no new detector-related Go code paths
   bypass the existing `PatternSet`/`StatusPatterns` compilation and matching
   pipeline."

2. **Build new swap machinery one level up.** Replace the
   `builtBinaryDetectors` package `var` with an
   `atomic.Pointer[detectorSnapshot]` holding the whole
   `map[string]*StatusDetector` plus per-binary provenance, rebuilt
   copy-on-write and swapped in a single `Store`.

3. **Add an explicit initialization function** — `detection.InitPlugins(ctx)`
   — called from `main.go`'s root `RunE` after
   `log.InitializeWithConfig(...)` (`main.go:148`) and **before** the
   `if daemonFlag { return daemon.RunDaemon(cfg) }` branch (`main.go:177`),
   so both the daemon and the web-server paths are covered by one call site.

4. **Keep a built-ins-only snapshot installed by package `init()`**, so any
   code path (or test) that never calls `InitPlugins` behaves byte-identically
   to today.

## Why not extend the `StatusDetector` YAML path

- **Wrong unit of change.** `StatusDetector` is a single pattern set. It has
  no concept of `id`, `binary_names`, or registry membership. Requirement #1
  lets one file declare multiple `binary_names`, and requirement #4 requires a
  user file to *override a built-in keyed by binary name*. Neither can be
  expressed on a `StatusDetector`; both are naturally expressed on the map
  keyed by binary name.
- **Wrong atomicity boundary.** `atomic.Pointer[PatternSet]` gives per-detector
  atomicity. A reload of the plugin directory is a whole-set operation — files
  added, edited, and removed together. Swapping N detectors one at a time
  makes a half-applied reload observable to `DetectForProgram`. Requirement #3's
  "a single invalid file must not prevent other valid plugin files, or the
  built-in detectors, from loading" is much easier to guarantee when the
  entire next state is built off to the side, validated, and swapped once.
- **Wrong format.** Extending `LoadPatterns` to sniff YAML-vs-TOML would put
  two formats behind one function for no user benefit; the requirements fix
  TOML.

The existing YAML functions stay exactly as they are — not deleted (out of
scope for this item; they have their own tests), not extended, not called.
Their existence is noted here so a future reader doesn't conclude this feature
duplicated them by accident.

## Concurrency shape

```go
type detectorSnapshot struct {
    byBinary   map[string]*StatusDetector // binary name -> compiled detector
    provenance map[string]string          // binary name -> source .toml path ("" = built-in)
}

var (
    activeSnapshot  atomic.Pointer[detectorSnapshot]
    snapshotWriteMu sync.Mutex
)
```

- Readers (`DetectForProgram`) call `activeSnapshot.Load()` — lock-free, on
  what is effectively a per-PTY-read hot path.
- Writers (`rebuildSnapshot`) hold `snapshotWriteMu` across the **entire**
  read-modify-write, not just the `Store`. Precedent and rationale:
  `session/pipeline_engine.go`'s `pipelineModeCache` (`:123-140`, `refresh`
  `:162-189`) — holding the mutex only around `Store` lets a
  slower-starting-but-slower-finishing writer's stale data land after a faster
  writer's, a lost update. Two rapid saves of the same plugin file is exactly
  that race.
- Aligned with `docs/adr/011-prefer-lock-free-concurrency.md` and
  `.claude/rules/go-double-checked-locking.md`. Note the double-checked-locking
  rule's "return the locally-computed value" clause does **not** bind
  `rebuildSnapshot`, because no caller consumes a value from it; it would only
  matter if a future "reload now" RPC needed to report its own result back.

## Scope boundary: per-binary path only

The generic detector path — `NewStatusDetector()` / `getDefaultPatterns()`,
used by `session/claude_controller.go:224`, `session/detection/idle.go:84,102`,
`session/startup_scanner.go:25`, `session/review_queue_poller.go:146` — is
**not** touched by this feature. `requirements.md` describes plugins entirely
in terms of per-binary detectors (`binary_names`, "matches a built-in
detector's binary name"), and there is no requirement that a plugin extend the
generic fallback patterns. Those call sites are deliberately unmodified; a
reviewer noticing that `claude_controller.go` is absent from the diff should
read this paragraph rather than file it as an omission.

## Consequences

- `builtBinaryDetectors` (a `var`) becomes `activeSnapshot` (an
  `atomic.Pointer`) plus a `lookupBinaryDetector(program)` accessor. This is a
  required structural change, not optional — there is no other place to hook
  hot-reload in, and package-init ordering cannot satisfy requirement #6
  (bootstrap the directory *before* the first scan) because package init runs
  before config resolution.
- A plugin change takes effect on the very next `DetectForProgram` call after
  the `Store` completes, for all sessions simultaneously. No session object or
  long-lived goroutine caches the map, so there is no per-session staleness to
  reconcile.
- Any future "list loaded detectors" / "which file won" affordance reads the
  snapshot's `provenance` map — the data is already there.
