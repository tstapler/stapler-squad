# Detection Architecture Research: Current Structure and Refactor Targets

## 1. StatusDetector Fields and Natural Ownership

### Full Field Inventory (`session/detection/detector.go`)

```
StatusDetector {
    // Pattern data (compile-time immutable after init)
    patterns             StatusPatterns
    readyRegexes         []*regexp.Regexp
    processingRegexes    []*regexp.Regexp
    needsApprovalRegexes []*regexp.Regexp
    inputRequiredRegexes []*regexp.Regexp
    errorRegexes         []*regexp.Regexp
    testsFailingRegexes  []*regexp.Regexp
    idleRegexes          []*regexp.Regexp
    activeRegexes        []*regexp.Regexp
    successRegexes       []*regexp.Regexp

    // Observation sink
    sessionID   string             // written under ring.mu
    ring        eventRing          // mutex-protected ring buffer
}
```

### Natural Decomposition by Responsibility

**PTYNormalizer** (pure functions, no state):
- `stripANSI(text string) string`
- `collapseCarriageReturns(s string) string`
- `hasScreenOverwrite(raw []byte) bool`
- `filterTmuxMetadata(content string) (string, int)` — currently in `claude_controller.go`

These are stateless transforms on raw bytes → cleaned text. They have no dependency on patterns, session identity, or the ring buffer. They belong in their own type (or package-level functions behind an interface) so that callers other than `StatusDetector` (e.g. `IdleDetector`, `StartupScanner`) can share them without going through the full detector.

**PatternSet** (immutable value, constructed once):
- `patterns StatusPatterns`
- `readyRegexes … successRegexes` (all 9 compiled slices)
- Methods: `compilePatterns()`, `detectFromText()`, `GetPatternNames()`, `HasPattern()`
- Load/export: `LoadPatterns()`, `ExportPatterns()`

A `PatternSet` is inherently read-only after creation. Multiple sessions could share one instance safely, since no field is mutated after `compilePatterns()` returns. Separating it from the sink makes it shareable.

**DetectionEventSink** (stateful, needs a mutex):
- `sessionID string`
- `ring eventRing`
- Methods: `SetSessionID()`, `RecentEvents()`, `appendDetectionEvent()`

The ring and `sessionID` are mutated concurrently (written by detection goroutine, read by the GetDetectionEvents RPC). The `ring.mu` already serializes access. The sink is inherently per-session.

**StatusDetector (orchestrator)** — composes the three above:
- Calls PTYNormalizer to clean input
- Delegates pattern matching to PatternSet
- Records outcomes into DetectionEventSink
- Exposes the combined `Detect()` / `DetectWithContext()` / `DetectFromLines()` API

The orchestrator itself holds no mutable state beyond what it delegates; it is the glue.

---

## 2. Concurrency Properties of StatusDetector

### Current State: NOT Concurrency-Safe as a Value

`StatusDetector` has **no exported or unexported mutex** protecting its pattern fields or `sessionID`. The only mutex is `ring.mu`, which guards the ring buffer and sessionID writes.

**What this means:**
- After construction, `patterns` and all `*Regexes` slices are never written again (only read). Go's memory model makes this safe for concurrent reads as long as the initial write (constructor) happens-before all reads — which is true when the detector is stored via `atomic.Pointer` in `ClaudeController`.
- `sessionID` is written under `ring.mu` via `SetSessionID()`. `appendDetectionEvent()` also acquires `ring.mu` before reading `sessionID`. This is correctly synchronized.
- `LoadPatterns()` mutates `patterns` and all regex slices without any lock. Calling `LoadPatterns()` concurrently with any `Detect*()` call is a **data race**.

### Sharing Between ClaudeController and IdleDetector

`IdleDetector` does **not** share a `StatusDetector` instance with `ClaudeController`. It creates its own via `NewStatusDetector()` in `NewIdleDetectorWithConfig()`. This means:
- Two separate `StatusDetector` instances exist per session: one in `ClaudeController.statusDetector` (atomic pointer), one embedded in `IdleDetector.statusDetector`.
- Both are constructed once and not mutated after construction (patterns are frozen; `SetSessionID` is only called on the controller's instance).
- This duplication is the primary inefficiency a refactor should address: `PatternSet` is identical in both and could be shared safely.

### Safety of the Current atomic.Pointer Pattern in ClaudeController

`ClaudeController` stores the detector in `atomic.Pointer[detection.StatusDetector]`. This provides:
- Safe publish/subscribe: `Store()` in `Start()` and `Load()` in `GetCurrentStatus()` are linearizable.
- No lock contention: readers never block on Stop().

Sharing the same `*StatusDetector` between `ClaudeController` and `IdleDetector` would be safe for the read paths (`Detect*()` calls), but risky if `LoadPatterns()` can be called at runtime. If `LoadPatterns()` is removed (patterns frozen at construction), sharing becomes fully safe.

---

## 3. Minimal TerminalDetector Interface — Confirmed Call Sites

Five confirmed call sites from outside the `detection` package:

| # | Location | Method Called | Purpose |
|---|----------|---------------|---------|
| 1 | `session/claude_controller.go:654` | `sd.DetectWithContextFromLines(lines)` | Primary status detection for GetCurrentStatus() |
| 2 | `session/command_executor.go:368` | `ce.statusDetector.DetectWithContext(outputBuffer)` | Terminal status during command execution wait-loop |
| 3 | `session/review_queue_determiner.go:195` | `detector.DetectWithContextFromLines(lines)` | No-controller sessions in review queue poller |
| 4 | `session/startup_scanner.go:46` | via `Determine(inst, content, statusInfo, ss.detector)` → `DetectWithContextFromLines` | One-shot startup scan |
| 5 | `server/services/session_service.go:3818` | `controller.GetStatusDetector().RecentEvents(limit)` | GetDetectionEvents RPC — reads event ring |

**Minimal `TerminalDetector` interface:**

```go
// TerminalDetector is the minimal interface for terminal output status detection.
// All callers outside the detection package use exactly these four methods.
type TerminalDetector interface {
    // DetectWithContext returns the detected status and a human-readable description.
    // Used by CommandExecutor (full-block detection) and as the per-line primitive
    // inside DetectWithContextFromLines.
    DetectWithContext(output []byte) (DetectedStatus, string)

    // DetectWithContextFromLines analyzes lines bottom-up (most-recent-first) and
    // returns the dominant status. Used by ClaudeController, ReviewQueueDeterminer,
    // and StartupScanner.
    DetectWithContextFromLines(lines []string) (DetectedStatus, string)

    // RecentEvents returns the n most-recent detection events for debugging.
    // Used by the GetDetectionEvents RPC in session_service.go.
    RecentEvents(n int) []DetectionEvent
}
```

`SetSessionID()` is called inside `ClaudeController.Start()` after construction — it could be a constructor argument instead, removing the need to expose it in the interface. `Detect()`, `DetectFromString()`, `DetectForProgram()`, `DetectRecent()`, `GetPatternNames()`, `HasPattern()`, `LoadPatterns()`, `ExportPatterns()` are either test helpers, internal, or deprecated (`DetectForProgram` is explicitly marked deprecated) and do not need to appear in the interface.

---

## 4. Relevant ADRs and Existing Patterns

### ADR-011: Prefer Lock-Free Concurrency (`docs/adr/011-prefer-lock-free-concurrency.md`)
**Directly applicable.** The `ClaudeController` already follows this: sub-components are stored in `atomic.Pointer[T]` fields with cache-line padding. The recommended refactor path is:
- Keep `PatternSet` as a value type (frozen after construction) shared via a plain pointer — no mutex needed since patterns are never written after init.
- Keep `DetectionEventSink` as a mutex-protected per-session value.
- Expose `TerminalDetector` as an interface so `ClaudeController` can hold `atomic.Pointer[TerminalDetector]` without needing to import the concrete type.

### ADR-007: Enum-Based State Transitions (`docs/adr/007-enum-based-state-transitions.md`)
**Indirectly applicable.** `DetectedStatus` is already an `int` enum with a `String()` method. The ADR's guidance on stable serialization (never reorder, always add at end, provide `String()`) is already followed. Any refactor that adds new `DetectedStatus` values must comply.

### Existing Patterns to Follow

**`Locked[T]` generic wrapper** (`session/claude_controller.go`): The codebase uses a `Locked[T]` type for mutex-guarded values with `Read(func(T))` / `Write(func(*T))` accessors. `DetectionEventSink` should adopt this rather than exposing `ring.mu` directly.

**`atomic.Pointer[T]` with cache-line padding**: `ClaudeController` uses `[64]byte` padding between hot atomic slots and locked fields to prevent false sharing (Go issue #67764). Any new atomic pointer fields in the detection layer should follow this pattern.

**Constructor injection of shared state**: `NewCommandExecutor` already receives `*detection.StatusDetector` as a parameter — the wiring happens in `ClaudeController.Start()`. A refactored `PatternSet` would follow the same pattern: constructed once in `Start()`, injected into both the `StatusDetector` orchestrator and `IdleDetector`.

**Interface to break circular imports**: `PTYReader` in `idle.go` is already defined as an interface (`GetRecentOutput(n int) []byte`) to avoid a circular import between `session/detection` and `session`. The `TerminalDetector` interface above should live in the same package (`session/detection`) or in a separate `session/detection/iface` package for the same reason.

**Deprecation comment pattern** (`DetectForProgram`): The codebase already uses `// DEPRECATED:` comments with migration guidance. Any methods removed from `StatusDetector` during refactor should follow this pattern before deletion.

---

## Summary of Key Findings

1. **Field decomposition**: `StatusDetector`'s 11 fields split cleanly into three concerns — (a) PTYNormalizer (stateless byte transforms), (b) PatternSet (immutable compiled regexes), and (c) DetectionEventSink (per-session ring buffer with mutex). The orchestrator `StatusDetector` is glue only.

2. **Concurrency**: The detector is safe for concurrent reads after construction but has a latent data race in `LoadPatterns()` (no lock on pattern fields). `IdleDetector` creates a second, identical `PatternSet` per session — sharing a frozen `PatternSet` via a plain pointer is safe under ADR-011 and eliminates the duplication.

3. **Minimal interface**: All five external call sites use only `DetectWithContext`, `DetectWithContextFromLines`, and `RecentEvents`. Extracting `TerminalDetector` with these three methods enables mock injection in tests, removes the concrete-type dependency in `ClaudeController`, and opens the door to per-binary detector dispatch (the stated goal of `DetectForProgram`'s replacement).
