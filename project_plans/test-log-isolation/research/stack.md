# Research: Stack (Go stdlib log/slog synchronization + test isolation)

Agent 1 — SDD Phase 2, `test-log-isolation`. All findings verified against
`origin/main` (git worktree source) and the installed Go toolchain source
(`go1.26.4`, `$(go env GOROOT)/src/log/slog/*`, `$(go env GOROOT)/src/net/http/httptest/server.go`),
not the stale worktree checkout (per requirements.md's note that this
worktree is ~559 commits behind).

## 1. What `slog.SetDefault` actually guarantees about `log.Print`

VERIFIED by reading `$(go env GOROOT)/src/log/slog/logger.go:62-75` (Go 1.26.4):

```go
func SetDefault(l *Logger) {
	defaultLogger.Store(l)
	// If the default's handler is a defaultHandler, then don't use a handleWriter,
	// or we'll deadlock as they both try to acquire the log default mutex.
	...
	if _, ok := l.Handler().(*defaultHandler); !ok {
		capturePC := log.Flags()&(log.Lshortfile|log.Llongfile) != 0
		log.SetOutput(&handlerWriter{l.Handler(), &logLoggerLevel, capturePC})
		log.SetFlags(0) // we want just the log message, no time or location
	}
}
```

`SetDefault` does two things atomically-in-sequence: it stores the new
`*slog.Logger` in `slog`'s own atomic default, **and** it calls
`log.SetOutput(&handlerWriter{l.Handler(), ...})` — i.e. it rewires the
stdlib `log` package's default logger's output `io.Writer` to a
`slog.handlerWriter` that wraps the *same handler* just installed. `log.Print`
internally writes to `log.std`'s output writer, so after any `slog.SetDefault`
call (with a non-default handler), every `log.Print`/`log.Printf` call in the
process is routed through that slog `Handler.Handle()` call, whatever it is.

This is exactly the mechanism in the requirements.md baseline stack trace:
`httptest.(*Server).logCloseHangDebugInfo` → `log.Print` → (rewired writer) →
`slog.(*TextHandler).Handle` → `bytes.(*Buffer).Write`. This is documented,
intentional stdlib behavior (see `log/slog` package doc, "Writing to the
default logger" / "Bridging log.Logger"), not a bug or an edge case — any
process-wide `slog.SetDefault` call unconditionally captures the stdlib
`log` package's default output for the process's lifetime (or until the next
`SetDefault`/`log.SetOutput` call).

**Consequence for this bug**: a test doesn't need to call `slog.*` or even
know slog exists to be affected — any code path (including stdlib internals
like `httptest.Server.Close()`'s hang detector) that calls `log.Print` is
silently routed through whatever `slog.SetDefault` last installed,
process-wide, for as long as that install lasts.

## 2. Is `TextHandler`/`JSONHandler` (and `bytes.Buffer`) safe for concurrent `Handle()`?

**The `Handler` interface itself is documented as required to be concurrency-safe.**
`$(go env GOROOT)/src/log/slog/handler.go:24-26`:

> "Any of the Handler's methods may be called concurrently with itself or
> with other methods. It is the responsibility of the Handler to manage this
> concurrency."

`slog.NewTextHandler` / `slog.NewJSONHandler` satisfy that contract by giving
every handler instance (and every clone produced by `WithAttrs`/`WithGroup`)
a **shared `*sync.Mutex`**:

- `text_handler.go:37` / `json_handler.go:39`: `mu: &sync.Mutex{}` at
  construction.
- `handler.go:202-217` (`commonHandler` struct + `clone()`): `mu *sync.Mutex`
  is explicitly *not* copied by value on clone — `mu: h.mu, // mutex shared
  among all clones of this handler` — so all handlers derived from one
  `NewTextHandler`/`NewJSONHandler` call share one lock.
- `handler.go:270-320` (`commonHandler.handle`): `h.mu.Lock()` wraps the
  actual `h.w.Write(...)` call.

**So: `slog.NewTextHandler(buf, ...).Handle()` calls ARE safe to call
concurrently from multiple goroutines** — the handler's own mutex serializes
every `Write` call it makes to the underlying `io.Writer` (here, a
`bytes.Buffer`). **What is NOT covered**: that mutex only guards writes made
*through the handler*. `bytes.Buffer` itself has zero internal
synchronization (its doc: "A Buffer is not safe for concurrent use... a
Buffer only needs to be safe if concurrent access is required"). If test code
holds a `*bytes.Buffer` and calls `buf.String()` directly — bypassing the
handler entirely, which is exactly what every `captureLogs`-style test in
this repo does — that read races the handler's `h.mu`-protected `Write`,
because the read is not participating in that lock at all. This is the
precise root cause named in requirements.md: "one goroutine writes into it
via the slog handler while the owning test concurrently reads it with
`buf.String()`."

**Conclusion**: `-race`'s finding is a real, stdlib-documented gap — not a
false positive and not something a "safer" choice of `TextHandler` vs
`JSONHandler` would fix. The handler's own concurrency safety guarantee
covers `Handle()`-to-`Handle()` races; it explicitly does not cover a second,
independent reader of the same destination buffer.

## 3. Idiomatic pattern for an injectable per-test logger (and this codebase's own `log/log.go`)

**Does `*slog.Logger` support being passed as a plain value independent of
`slog.Default()`?** Yes — `slog.New(handler)` returns a `*slog.Logger` value
with no dependency on the package-global default; `logger.Info(...)` etc.
call the handler directly, never touching `slog.Default()`. Passing one as a
struct field or function parameter and calling its methods is completely
independent of the process-global swap problem — this is the textbook
"dependency injection over global mutable state" fix and requires no stdlib
workarounds.

**This codebase's own `log/log.go`** (`github.com/tstapler/stapler-squad/log`,
read via `git show origin/main:log/log.go`) is a package-level wrapper: its
`Debug`/`Info`/`Warn`/`Error` functions call straight through to `slog`'s
package-level default (confirmed by `session/streamhub/observability_test.go`'s
own comment: *"log.Info/Warn/Error ... delegate straight to slog's
package-level default logger, so this is the seam available to assert on
structured log output"*). The wrapper has **no optional injectable
`*slog.Logger` field or parameter today** — every call site (386 per
requirements.md) implicitly targets whatever `slog.SetDefault` last
installed.

**Would adding an injectable `*slog.Logger` be low-diff?** This is where the
"time-box discovery" rabbit-hole warning in requirements.md matters. Two
sub-cases:

- If `captureLogs`-affected code paths are a **small, already-identified
  subset** (per requirements.md's own baseline: the two confirmed instances
  are `pumpControlModeOutputIntoHub` — fixed via goroutine lifecycle, not
  logger injection — and `httptest.Server.Close()`'s **stdlib-internal**
  `logCloseHangDebugInfo`, which calls `log.Print` from inside
  `net/http/httptest`, a package this repo does not own and cannot inject a
  logger into), then full DI is not just expensive, it's **impossible** for
  instance #2: there is no parameter on `httptest.Server` to give its
  internal hang-detector a different `*slog.Logger` or `io.Writer`. Directly
  injecting `*slog.Logger` fixes code *this repo owns*, but the confirmed
  failing case's *actual* logger call is stdlib-internal and unreachable by
  DI. This narrows Direction 1 down to: it cannot fully eliminate this
  specific trigger by itself; only Direction 2 (bound/eliminate the trigger)
  or fixing the *destination buffer's* thread-safety (see below) can.
- Adding a field to the handful of structs actually under test in
  `captureLogs`-based tests (not all 386 call sites) is plausible and
  matches the rabbit-hole guidance, but doesn't address instance #2 at all,
  since the offending call (`httptest`'s `log.Print`) is unreachable from any
  struct this repo controls.

**A third option, evidenced directly in this codebase and cheaper than
either Direction 1 or 2**: two other packages already solve exactly this
problem without touching call sites at all, by making the *capture
destination* itself safe for concurrent access instead of trying to control
who writes to it:

- `executor/safeexec/safeexec_pg_test.go:22-40` — `syncBuffer` (a
  `sync.Mutex`-guarded `bytes.Buffer` with `Write`/`String` methods), with an
  explicit doc comment: *"syncBuffer wraps bytes.Buffer with a mutex so it's
  safe to use as slog's output while a test polls String(): the escalation
  path deliberately logs from a background time.AfterFunc goroutine,
  concurrently with the test goroutine reading the buffer, which a plain
  bytes.Buffer does not support."* — this is the **exact same failure
  pattern** as this item's instance #2 (a background `time.AfterFunc`
  writing to a captured buffer that a live test is reading).
- `session/streamhub/observability_test.go:24-38` — `syncLogBuffer`, same
  shape, with a doc comment naming cross-test log interleaving via the
  shared process-global default explicitly and noting it's harmless for that
  package's assertions (substring/ordering checks, not exclusivity checks).

`server/services/autonomous_orchestration_service_test.go`'s own
`captureLogs` (lines 407-427) is the **odd one out**: it uses a bare
`*bytes.Buffer`, not a `syncBuffer`/`syncLogBuffer`, and relies entirely on
`slogDefaultMu` (a mutex serializing *test execution*, not buffer *access*)
to prevent concurrent writers. That protects tests that know to hold the
lock, but as requirements.md documents, does nothing for a writer (like
`httptest`'s internal hang-detector goroutine) that has no reason to know
`slogDefaultMu` exists.

## 4. Is the existing `atomicLogger` (PR #576) pattern reusable here?

**No — it solves a different, narrower problem**, confirmed by reading
`log/log.go` (`git show origin/main:log/log.go:135-152`):

```go
type atomicLogger struct {
	ptr atomic.Pointer[log.Logger]
}
func (a *atomicLogger) Load() *log.Logger              { return a.ptr.Load() }
func (a *atomicLogger) Store(l *log.Logger)            { a.ptr.Store(l) }
func (a *atomicLogger) Swap(l *log.Logger) *log.Logger { return a.ptr.Swap(l) }
```

Its own doc comment states the rationale precisely: it exists so that
"other goroutines [can] concurrently read [a `*log.Logger` value] while
[it's] swapped ... with no risk of a torn or racy read" for the package's
**stdlib `*log.Logger`-based streams** (`WarningLog`/`InfoLog`/`ErrorLog` —
the legacy `log.New(...)`-based loggers, not the `slog.Debug/Info/Warn/Error`
package functions). `atomic.Pointer` swap makes *reading which `*log.Logger`
pointer is current* race-free.

That does not help here for two reasons:

1. **It's the wrong axis.** `atomic.Pointer[log.Logger]` makes *which
   logger-pointer-value is currently installed* race-free to read/swap. The
   bug in this item is not a torn/racy read of *which pointer is current* —
   `slog.Default()` is already stored via an internal `atomic.Value`
   (confirmed by `slogDefaultMu`'s own doc comment: *"slog.Default() is
   stored in an unexported atomic pointer, so -race never flags concurrent
   swaps"*). The race is in the **destination `bytes.Buffer` behind the
   handler** — a plain `bytes.Buffer.Write()` racing a plain
   `bytes.Buffer.String()` call, which no pointer-swap pattern touches at
   all.
2. **`slog.Handler` isn't a `*log.Logger`.** `atomicLogger` is typed
   specifically around `*log.Logger`; the object under contention here is a
   `*bytes.Buffer` acting as a `slog.Handler`'s `io.Writer`, a structurally
   different problem (concurrent access to a shared mutable value, not
   "which value is currently installed").

The applicable existing precedent in this codebase is **not** `atomicLogger`
but the `syncBuffer`/`syncLogBuffer` pattern from §3 — a mutex around the
actual shared buffer, at the exact point of contention.

## 5. Go version and recent stdlib changes

- **Go version in use**: `go.mod` (`git show origin/main:go.mod`, and this
  worktree's own copy) pins `go 1.26.4`. Installed toolchain matches
  (`go version` → `go1.26.4`).
- **`slog` package**: added in Go 1.21 (2023) — well before this repo's
  pinned toolchain; no version-gating concern.
- **`httptest.Server.Close()`'s 5-second hang detector**
  (`logCloseHangDebugInfo`, `$(go env GOROOT)/src/net/http/httptest/server.go:230-291`):
  this is **not a recent addition**. Per Go's own commit history (CL 15151,
  fixing golang/go issues #12789/#12781, ~2015/Go 1.6 era — "net/http/httptest:
  change Server to use http.Server.ConnState for accounting of outstanding
  requests", [commit a3156aa](https://github.com/golang/go/commit/a3156aaa121446c4136927f8c2139fefe05ba82c)),
  this mechanism has existed for roughly a decade and is unrelated to any
  slog-era change. It is **hardcoded and not configurable**: reading the full
  `Close()` body confirms `time.AfterFunc(5*time.Second, s.logCloseHangDebugInfo)`
  with no exported field, flag, or option on `httptest.Server` (or
  `HandlerOptions`) to change the 5s threshold or disable the log call —
  `httptest.Server`'s only configuration knobs are `Config`, `TLS`,
  `EnableHTTP2`, `Client`, none of which touch this. There is no supported
  way to stop this stdlib code path from calling `log.Print` if `Close()`
  is slow; the only levers available to this item are (a) make sure `Close()`
  never takes ≥5s (Direction 2: bound the trigger) or (b) make whatever
  `log.Print` is currently routed to at that moment safe to be written into
  concurrently (Direction 3 from §3: fix the buffer, not the caller).

## Summary of implications for Phase 3 planning

- Direction 1 (full DI of `*slog.Logger` through `server/services`) **cannot
  fully close instance #2** on its own — the actual failing log call
  (`httptest`'s internal hang detector) is stdlib code this repo doesn't own
  and has no injection point for. DI only helps for log calls this repo's
  own code makes.
- Direction 2 (bound the `httptest.Server` teardown so `Close()` reliably
  returns before 5s, as already done for instance #1 via
  `ForceTeardown`-per-iteration) directly addresses the specific confirmed
  trigger, matches existing precedent in this exact codebase, and requires
  no call-site threading.
- A third option not enumerated in requirements.md's Alternatives Considered
  but directly evidenced by two sibling packages in this same repo
  (`executor/safeexec`, `session/streamhub`): swap
  `server/services/autonomous_orchestration_service_test.go`'s bare
  `*bytes.Buffer` in `captureLogs` for a mutex-guarded `syncBuffer`/
  `syncLogBuffer` (identical pattern, ~15 lines, no call-site changes,
  already precedented and reviewed twice in this codebase). This closes the
  race at the actual point of contention (the buffer) regardless of whether
  the writer holds `slogDefaultMu` or even knows it exists, and is
  effective against *any* future background-goroutine/slow-resource trigger,
  not just the two confirmed instances — directly satisfying the "fix must
  generalize" success metric without the appetite risk of full DI.
- These two are not mutually exclusive; a Small-appetite fix plausibly pairs
  both: (1) fast/deterministic teardown for the confirmed
  `TestAnthropicAIClient_Complete_CancelsOnCtxDone` trigger (cheap,
  eliminates the specific instance immediately) + (2) `syncBuffer`-style hardening
  of `server/services`'s shared `captureLogs` helper (cheap, closes the race
  class structurally so a third instance can't recur) — both are ~single-file,
  low-diff changes consistent with the Small appetite.
