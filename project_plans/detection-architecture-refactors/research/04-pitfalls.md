# Detection Architecture Refactor — Pitfall Analysis

> Research phase 4 of the detection-architecture-refactors plan.
> Covers: test brittleness, concurrency hazards, ring buffer semantics, and SRP-split performance.

---

## 1. Tests That Access Unexported Fields or Rely on Internal Struct Layout

### Verdict: Tests are already well-insulated — but four patterns carry latent risk.

**Safe (behavioral probes only)**

`TestNewStatusDetector` (detector_test.go:11) deliberately avoids counting internal slice
lengths; it uses `Detect()` calls on fixed strings. `TestStatusDetector_DetectUnknown_NoPatterns`
constructs an empty YAML and checks the return value. All patterns in `detector_test.go` probe
the public API only.

**Latent risk 1 — `newDetectorWithFakeClock` directly sets unexported fields**

```go
// idle_test.go:17-19
d.now = func() time.Time { return fakeNow }
d.lastStateChange = t0
d.lastActivity = t0
```

These three assignments bypass the public API and reach directly into `IdleDetector` unexported
fields. If `IdleDetector` is restructured so that `now`, `lastStateChange`, or `lastActivity`
move into a sub-struct (e.g. a `clockState` or `activityTracker` embedded struct), every test
that calls `newDetectorWithFakeClock` breaks — that is currently 9 test functions (all the
fake-clock tests in idle_test.go:173–645).

**Latent risk 2 — `freezeAt` helper (idle_test.go:464–469) does the same**

```go
d.now = func() time.Time { return frozenNow }
d.lastActivity = frozenNow
d.lastStateChange = frozenNow
```

Same exposure: any rename or relocation of these fields breaks six test cases in
`TestIdleDetector_InitializeFromTimestamp`.

**Latent risk 3 — `getDefaultPatterns()` called directly from test**

`TestGeminiPatterns_AgyCoverage` (detector_test.go:610) calls the package-private function
`getDefaultPatterns()` and inspects `patterns.Ready`, `patterns.Processing`,
`patterns.NeedsApproval`, `patterns.Error` by iterating their `Name` fields. If
`StatusPatterns` is split into sub-structs or `getDefaultPatterns` is renamed/moved, this test
breaks.

**Latent risk 4 — `mockPTYReader.Write` and `mockPTYReader.Clear` are test-internal**

These are unexported methods on a test-only struct; they don't touch `StatusDetector` or
`IdleDetector` internals. Safe.

**Mitigation for risks 1–2:** Add constructor helpers or options like
`WithFakeClock(fn func() time.Time)` and `WithInitialTimestamps(stateChange, activity time.Time)`
to `IdleDetector` before any field restructuring so tests can migrate without direct field access.

---

## 2. Concurrency Hazards: Mutexes and What They Protect

The two types involved in a shared-detector scenario are `StatusDetector` and `IdleDetector`.
They have *separate* mutexes; neither type holds the other's lock. Current call graph from
`ClaudeController`:

```
ClaudeController (goroutine: runStatusChangeLoop)  →  sd.DetectWithContextFromLines()
ClaudeController (goroutine: PTY read loop)        →  id.RecordActivity()
ClaudeController (any caller)                      →  id.DetectStateFromContent()
ClaudeController (any caller)                      →  sd.RecentEvents()
```

### `eventRing.mu` (inside `StatusDetector.ring`)

- **Type:** `sync.Mutex` (inside `eventRing`)
- **Protects:** `ring.events[EventRingCap]`, `ring.head`, `ring.count`, and the read of
  `sd.sessionID` inside `appendDetectionEvent` (held together so the session-ID read and
  ring-push are atomic with respect to `SetSessionID`).
- **Acquired by:** `appendDetectionEvent` (write path via `pushLocked`), `RecentEvents` (read
  path via `recent`), and `SetSessionID`.
- **Not protecting:** the compiled regex slices (`readyRegexes`, etc.) or the `patterns`
  field — these are written only at construction/`LoadPatterns` time and must not be mutated
  while any goroutine is detecting.

### `IdleDetector.mu`

- **Type:** `sync.RWMutex`
- **Protects:** `currentState`, `lastStateChange`, `lastActivity`, `config`, and the read of
  `id.ptyAccess` indirectly (ptyAccess is set at construction and never mutated, so the lock
  does not need to guard it, but all state fields that change after construction are covered).
- **Acquired for write by:** `DetectState`, `DetectStateFromContent`, `mapStatusToIdleState`
  (callers must hold write lock), `Reset`, `UpdateConfig`, `RecordActivity`,
  `InitializeFromTimestamp`.
- **Acquired for read by:** `GetState`, `GetLastActivity`, `GetIdleDuration`, `GetStateInfo`.

### Invariants that must hold if a single `StatusDetector` is shared between `ClaudeController` and `IdleDetector`

Currently `IdleDetector` creates its own private `StatusDetector` in
`NewIdleDetectorWithConfig` (idle.go:80). If these were unified:

1. **`LoadPatterns` is not safe under concurrent detection.** It mutates `sd.patterns` and all
   nine compiled-regex slices outside any lock. Any in-flight `detectFromText` call that reads
   `sd.readyRegexes` concurrently with `LoadPatterns` is an unprotected data race. A shared
   detector would need a reader-writer lock around pattern fields, or `LoadPatterns` must be
   restricted to "before first Detect call" semantics.

2. **`sessionID` is currently owned by `StatusDetector` and protected by `ring.mu`.** A shared
   detector means both `ClaudeController` and `IdleDetector` calls produce events tagged with
   the same `sessionID` — that is harmless. But `SetSessionID` must still be called exactly
   once before any concurrent Detect calls begin; this is already the current convention
   (claude_controller.go:209).

3. **`appendDetectionEvent` is safe under concurrent callers** because `ring.mu` serialises
   all pushes. No ordering issue between the two call sites.

4. **No deadlock risk** between `ring.mu` and `IdleDetector.mu` because they are never acquired
   together — `IdleDetector.DetectStateFromContent` holds `id.mu` (write), calls
   `sd.DetectFromLines` which internally calls `sd.DetectWithContext` which calls
   `sd.appendDetectionEvent` which acquires `ring.mu`. The nesting order is always
   `id.mu → ring.mu`; no code path acquires them in reverse order.

---

## 3. Ring Buffer: Capacity and Semantics Under Sharing

### Current layout

| Component | Ring buffer | Capacity | Purpose |
|---|---|---|---|
| `StatusDetector` (owned by ClaudeController) | `eventRing` embedded in struct | 500 events | Records every `Detect`/`DetectWithContext` call for debugging |
| `StatusDetector` (owned by IdleDetector) | Same — separate instance | 500 events | Records calls made by `IdleDetector` methods |

`IdleDetector` holds its own private `*StatusDetector`; the two instances have independent ring
buffers. Each `Detect` call from `ClaudeController.GetCurrentStatus` goes into the
ClaudeController's detector ring; each call from `IdleDetector.DetectState` goes into the
IdleDetector's detector ring.

### What sharing one detector changes

If `IdleDetector` were given the same `*StatusDetector` instance that `ClaudeController` owns:

- The 500-event ring would be shared. Under the current polling frequency (status loop + idle
  poller both running continuously) the effective capacity roughly halves — whichever path
  generates more calls will displace the other's events. With a 500-event cap and typical
  detection cadence (~1 Hz each) this means ~4 minutes of history instead of ~8 per goroutine.
  This is not functionally breaking but reduces the debuggability window.
- `IdleDetector.DetectStateFromContent` splits content into lines and calls
  `sd.DetectWithContext` *per line* (via `sd.detectFromLines` → `sd.DetectWithContext` for each
  non-blank line). For a 50-line terminal window that is up to 50 `appendDetectionEvent` calls
  per status check rather than 1. Ring capacity would be exhausted much faster if sharing is
  combined with the line-by-line call pattern.
- `TailSnippet` stored per event (capped at 512 bytes) would mix ClaudeController-level tail
  snippets (whole-buffer tail) with per-line snippets from `IdleDetector`, making the ring
  harder to read when debugging.

**Recommendation:** If sharing is introduced, increase `EventRingCap` from 500 to at least 2000
or split the ring into separate per-caller rings (e.g. `detectionRing` vs `idleRing`).

---

## 4. SRP Split for `PTYNormalizer`: Hot-Path Allocation Risk

The hot path for a single status check is:

```
GetCurrentStatus()
  → pa.GetBuffer() []byte → string(raw)          // allocation: []byte → string copy
  → tailContent(content, N) string                // sub-slice, no alloc if no copy needed
  → lines = lastNLines(filtered, M) []string      // strings.Split → []string allocation
  → sd.DetectWithContextFromLines(lines)
      → detectFromLines()
          → sd.DetectWithContext([]byte(lines[i])) // string → []byte per non-blank line
              → stripANSI(collapseCarriageReturns(string(output)))
```

Existing allocations on every `GetCurrentStatus` call (before any SRP split):

1. `string(raw)` — unavoidable byte-to-string copy.
2. `strings.Split` in `lastNLines` (or equivalent) → `[]string`.
3. `[]byte(lines[i])` inside `detectFromLines` → per-line byte allocation (converts each
   string line back to `[]byte` to call `DetectWithContext`).
4. `ansiStripRegex.ReplaceAllString` — allocates the result string.
5. `collapseCarriageReturns` — `strings.Split` + `strings.Join` inside each line segment.

### What a `PTYNormalizer` extraction adds

If normalization (ANSI stripping, CR collapsing) is lifted into a `PTYNormalizer` struct that
returns a pre-cleaned `string`, the call chain becomes:

```
normalizer.Normalize(raw []byte) string
  → string(raw)                          // same: unavoidable
  → collapseCarriageReturns(...)         // same allocation
  → stripANSI(...)                       // same allocation
```

**No new allocations** if `PTYNormalizer.Normalize` replaces the identical calls currently
inside `Detect`/`DetectWithContext`. The SRP split is allocation-neutral.

However, two patterns introduce *new* allocations if the interface is designed naively:

**Risk A — interface-box allocation on each Normalize call.** If `PTYNormalizer` is defined
as an interface rather than a concrete struct, each call through the interface allocates an
`itab` pointer pair on the heap for the `[]byte` argument if it escapes. Use a concrete struct
or pass a concrete receiver to avoid this.

**Risk B — double-normalisation if `detectFromLines` still calls `DetectWithContext([]byte)` per
line.** The current code in `detectFromLines` passes each line as `[]byte` to `DetectWithContext`,
which re-runs `stripANSI(collapseCarriageReturns(...))` on every line. If `PTYNormalizer` is
applied at the top level but `detectFromLines` still calls the full `DetectWithContext` pipeline
internally, normalization runs twice: once at the top and once per line. This doubles ANSI-strip
allocations in the `DetectFromLines` path. The fix is to have `detectFromLines` call
`detectFromText` directly (which it can, since both are on `*StatusDetector`) with pre-normalized
line bytes, bypassing the normalization step.

**Risk C — `[]byte` vs `string` boundary.** `Detect` and `DetectWithContext` accept `[]byte`;
`detectFromLines` converts each `string` line to `[]byte` before passing it in. If
`PTYNormalizer` works on `string`, the flip-flop `[]byte→string→[]byte→string` on the per-line
path adds two extra copies per line per status check. Since the hot path checks up to ~50 lines,
this is up to 100 extra copies per second at 1 Hz polling. Profile before committing to a
`[]byte`-in / `string`-out interface; a `string`-in / `string`-out interface that works directly
with `detectFromText` eliminates the boundary entirely.

---

## Summary

1. **Test brittleness:** Nine idle tests directly assign `d.now`, `d.lastStateChange`, and
   `d.lastActivity` without going through the public API. Any restructuring of `IdleDetector`
   fields into sub-structs breaks them all. Add injection constructors or options before
   refactoring.

2. **Mutex invariants:** `StatusDetector` has one mutex (`ring.mu`) protecting the event ring
   and `sessionID`; `IdleDetector` has one `sync.RWMutex` protecting all mutable state fields.
   The nesting order under a shared-detector scenario is always `id.mu → ring.mu` with no
   reverse path, so no deadlock risk. The critical hazard is `LoadPatterns`, which mutates
   regex slices outside any lock and is unsafe for concurrent use with `Detect` calls.

3. **Ring buffer:** Sharing one `StatusDetector` between `ClaudeController` and `IdleDetector`
   reduces effective event history from ~500 events per call site to a shared 500 total; the
   per-line call pattern in `detectFromLines` multiplies this by up to 50x, draining the ring
   in seconds. Either increase `EventRingCap` significantly or keep separate detector instances.

4. **PTYNormalizer SRP split is allocation-neutral** if it replaces existing normalization calls
   directly. The main risks are: double-normalization if `detectFromLines` still calls the full
   `DetectWithContext` pipeline per line (fix: call `detectFromText` directly), and a
   `[]byte`/`string` boundary mismatch that adds per-line copies (fix: choose a consistent
   string-level interface throughout the inner path).
