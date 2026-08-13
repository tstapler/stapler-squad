# Stack Research: detection package architecture

## 1. Narrow Interface Placement — Consumer vs Producer

**Idiomatic Go rule**: interfaces belong in the *consumer* package, not the producer.
Go Proverbs: "Accept interfaces, return concrete types." The stdlib follows this rigorously
(`io.Reader` is defined in `io`, not in `os` or `bufio`).

**What the codebase already does correctly:**

`session/detection/idle.go` lines 40–43 define `PTYReader` inside the `detection` package
(the consumer), not in `session` (the producer):

```go
// PTYReader provides access to recent terminal output.
// Implemented by *session.PTYAccess; defined here as an interface to avoid
// a circular import between session/detection and session.
type PTYReader interface {
    GetRecentOutput(n int) []byte
}
```

The comment is explicit: this is a deliberate anti-circular-import pattern. The concrete
type `*session.PTYAccess` satisfies the interface without the `detection` package needing
to import `session`.

**Implication for refactors**: any new interfaces for detection input (e.g., a
`TerminalContentProvider` or `BinaryDetector`) should be defined in the `detection`
package (consumer), not in the caller packages.

---

## 2. Registry Pattern — `map[string]Factory` vs `init()` Registration

**Two canonical Go patterns:**

| Pattern | Mechanism | Trade-offs |
|---|---|---|
| `map[string]Factory` | Explicit `Register(name, factory)` at startup | Testable, explicit, no global side effects |
| `init()` registration | `import _ "pkg/drivers/postgres"` side-effect import | Implicit, can cause link order surprises, hard to test in isolation |

The Go stdlib uses `database/sql` as the canonical `init()`-registration example; the
`encoding` package family (`gob`, `json`) uses explicit registration calls.

**What the codebase currently has:**

There is **no registry** yet. The `DEPRECATED` comment on `DetectForProgram`
(detector.go lines 738–743) explicitly names the missing abstraction:

```go
// DetectForProgram detects the status for output from a named program.
// DEPRECATED: the program parameter is currently ignored — all binaries share the same
// pattern set. For per-binary dispatch, implement the BinaryDetector interface and
// register it with a DetectorRegistry. This method will be removed once that interface
// is in place.
func (sd *StatusDetector) DetectForProgram(output []byte, program string) DetectedStatus {
    return sd.Detect(output)
}
```

The comment prescribes `BinaryDetector` interface + `DetectorRegistry`. The word "register"
suggests an explicit `map[string]Factory`-style registry rather than `init()` side-effect
imports — matching the idiomatic Go preference for testability. A `map[string]BinaryDetector`
keyed by binary name (e.g., `"claude"`, `"aider"`, `"gemini"`) with a registered fallback
is the natural fit.

---

## 3. `StatusPatterns` Struct

Defined in `detector.go` lines 39–49:

```go
type StatusPatterns struct {
    Ready         []StatusPattern `yaml:"ready"`
    Processing    []StatusPattern `yaml:"processing"`
    NeedsApproval []StatusPattern `yaml:"needs_approval"`
    InputRequired []StatusPattern `yaml:"input_required"`
    Error         []StatusPattern `yaml:"error"`
    TestsFailing  []StatusPattern `yaml:"tests_failing"`
    Idle          []StatusPattern `yaml:"idle"`
    Active        []StatusPattern `yaml:"active"`
    Success       []StatusPattern `yaml:"success"`
}
```

Each field is a `[]StatusPattern` (lines 31–36):

```go
type StatusPattern struct {
    Name        string `yaml:"name"`
    Pattern     string `yaml:"pattern"`
    Description string `yaml:"description"`
    Priority    int    `yaml:"priority"`
}
```

`Priority` is embedded in the struct but **not used for sorting** — `compilePatterns()`
(lines 132–161) compiles patterns in the order they appear in each slice without sorting
by priority. Priority is metadata only. The actual evaluation order is determined by the
hardcoded `detectFromText` priority chain (lines 233–303): Error → TestsFailing →
NeedsApproval → InputRequired → readlineTyping → Success → Active → Processing →
screenOverwrite → Idle → Ready.

`TestsFailing` is populated as an empty slice `[]StatusPattern{}` (line 484) and
carries a large comment block explaining why it is disabled.

---

## 4. Ring Buffer in `StatusDetector`

**Struct definition** (`events.go` lines 25–31):

```go
type eventRing struct {
    mu     sync.Mutex
    events [EventRingCap]DetectionEvent  // fixed-size array, not a slice
    head   int  // next write position (wraps mod EventRingCap)
    count  int  // filled slots, capped at EventRingCap
}
```

Constants (events.go lines 19–23):
- `EventRingCap = 500` — max events retained
- `TailSnippetBytes = 512` — max bytes in each event's `TailSnippet`

**Write path** (`pushLocked`, events.go lines 35–41): caller MUST hold `r.mu`.

```go
func (r *eventRing) pushLocked(e DetectionEvent) {
    r.events[r.head] = e
    r.head = (r.head + 1) % EventRingCap
    if r.count < EventRingCap {
        r.count++
    }
}
```

**Read path** (`recent`, events.go lines 44–59): acquires `r.mu` internally,
walks backward from `head-1` to return events newest-first:

```go
idx := (r.head - 1 - i + EventRingCap) % EventRingCap
```

**Locking contract** (detector.go lines 309–324): `appendDetectionEvent` acquires
`sd.ring.mu` to atomically read `sd.sessionID` and call `pushLocked`. This eliminates a
data race between `SetSessionID` and concurrent `Detect` calls:

```go
func (sd *StatusDetector) appendDetectionEvent(...) {
    ...
    sd.ring.mu.Lock()
    defer sd.ring.mu.Unlock()
    sd.ring.pushLocked(DetectionEvent{
        SessionID: sd.sessionID,
        ...
    })
}
```

`SetSessionID` (detector.go lines 328–332) also acquires `sd.ring.mu`:

```go
func (sd *StatusDetector) SetSessionID(id string) {
    sd.ring.mu.Lock()
    sd.sessionID = id
    sd.ring.mu.Unlock()
}
```

Note: `sessionID` field lives on `StatusDetector`, not on `eventRing`, but its mutation
and the ring write are serialized under the same mutex — a deliberate design choice.
