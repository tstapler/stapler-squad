# Warren Design Review

**Date:** 2026-05-02
**Status:** Findings documented — not yet actioned

This document captures two things:
1. Code review findings from a full read of `pkg/warren/`
2. Design improvements derived from Spring Boot's lifecycle architecture

Both sections end with a prioritised action list.

---

## Part 1: Code Review Findings

### Architecture issues

#### A1. Raw strings throughout — no typed phase keys (user-flagged)

`Phase()`, `Go()`, `OnStop()`, and `Health()` all take `string`. Phase names, goroutine
names, stop hook names, and health check names are indistinguishable by the type system.

Phases are the most structurally important: multiple packages may need to contribute to
the same phase, and a typo in a phase name is silently accepted. The other three (goroutine,
stop, health) are observability labels with open-ended per-component namespaces — raw strings
are acceptable there.

**Fix:** Introduce an opaque `PhaseKey` type (see Part 2 for the full Spring-Boot-inspired
design). `Phase()` accepts a `PhaseKey`, not a `string`. Goroutine/stop/health names stay
as strings.

---

#### A2. Startup failure does not clean up already-started phases

```go
func (a *App) Start(ctx context.Context) error {
    for _, p := range phases {
        if err := p.fn(ctx, a); err != nil {
            return fmt.Errorf("warren: phase %q: %w", p.name, err)  // leaks!
        }
    }
}
```

If phase 1 opens a DB connection and registers `OnStop("db", conn.Close)`, then phase 2
fails, the `OnStop` hook is never called. The DB connection leaks permanently.

Spring Boot solves this: `ApplicationContext.close()` is called even on startup failure,
which runs all `@PreDestroy` / `DisposableBean.destroy()` for already-initialized beans.

**This is a correctness bug, not a style issue.**

**Fix:**
```go
for i, p := range phases {
    if err := p.fn(ctx, a); err != nil {
        // Run all OnStop hooks registered so far, in reverse.
        _ = a.runStopHooks(context.Background())
        return fmt.Errorf("warren: phase %q: %w", p.name, err)
    }
}
```

---

#### A3. `New()` + exported mutable `ShutdownTimeout`

`ShutdownTimeout` is an exported field that can be modified at any time, including after
`Start()`, with no synchronisation. `TestApp` exploits this intentionally, but it is
inconsistent with the rest of the struct (all other fields are unexported).

**Fix:** Functional options.

```go
type Option func(*App)

func WithShutdownTimeout(d time.Duration) Option {
    return func(a *App) { a.shutdownTimeout = d }
}

func New(opts ...Option) *App { ... }
```

`TestApp` becomes `warren.New(warren.WithShutdownTimeout(2 * time.Second))`.

---

#### A4. `Check()` has no context — a slow check blocks everything

```go
func (a *App) Check() HealthReport
```

A hanging health check (e.g. a DB ping with no timeout) blocks all subsequent checks and
the caller. Health checks sit on the liveness/readiness probe path.

**Fix:** `func (a *App) Check(ctx context.Context) HealthReport` — pass context through to
each check function: `fn func(ctx context.Context) error`.

---

#### A5. No stopped guard — `Go()` is callable after `Stop()`

`Stop()` cancels the goroutine context and drains, but `a.goroutines` is never set to nil.
Calling `app.Go("worker", fn)` after `Stop()` adds a goroutine to an already-cancelled
group. The goroutine starts, receives an immediately-cancelled context, and exits — but is
never tracked. Silent bug.

**Fix:** Add `stopped bool` field. Check it (under mutex) in `Go()`.

---

#### A6. `Active()` returns nil before Start, empty map after

`app.Active()` returns `nil` before `Start()`. `GoroutineGroup.Active()` always returns an
allocated empty map. Inconsistent at the API boundary.

**Fix:** Always return an allocated map, document the semantics.

---

### Code style issues

#### S1. Startup failure does not clean up already-started phases (see A2)

#### S2. `wireEntry.err` is `string`, not `error`

```go
type wireEntry struct {
    err string  // should be error
}
```

Internal error state as `string` prevents `errors.Is`/`errors.As` and violates Go
convention. Change to `error`.

---

#### S3. `MultiError.Error()` single-element branch is dead code

`multiError()` never creates a `*MultiError` with one element — it returns the raw error
directly. The `len(m.Errors) == 1` branch in `MultiError.Error()` can never be reached.
Remove it.

---

#### S4. `Wire.Mark()` silently auto-registers unknown names

```go
// Auto-register if not declared via Require.
w.entries = append(w.entries, wireEntry{name: name, applied: true})
```

A typo in `Mark("HistryLinker")` silently adds a new applied entry instead of surfacing
the mistake. This defeats the entire purpose of `Require`. `Mark` on an undeclared name
should panic (wiring is startup code — fail fast is correct).

---

#### S5. `GoroutineGroup.Wait()` spawns a new goroutine on every call

```go
func (g *GoroutineGroup) Wait(timeout time.Duration) []string {
    done := make(chan struct{})
    go func() { g.wg.Wait(); close(done) }()  // new goroutine each call
    ...
}
```

Each call leaks a goroutine until `g.wg.Wait()` returns. In tests that call `Wait()`
multiple times (e.g. via `defer g.Wait(time.Second)` and explicit calls), these stack up.
Add a `sync.Once` or document `Wait` as a one-shot function with a guard.

---

#### S6. `leakReport` is defined in `app.go` but belongs in `goroutines.go`

It formats goroutine names for error messages — goroutine domain, not App domain.

---

#### S7. `Run()` should document why it uses `context.Background()` for stop context

```go
// Use Background, not ctx — ctx is already Done at this point; inheriting it
// would give stopCtx zero remaining deadline instead of ShutdownTimeout.
stopCtx, cancel := context.WithTimeout(context.Background(), a.ShutdownTimeout)
```

---

## Part 2: Spring Boot Design Improvements

Spring Boot's lifecycle model has been refined across 15 years and ~3 billion deployments.
The concepts most applicable to Warren — without importing the service-registry or reflection
patterns Warren deliberately avoids — are:

1. **Integer-ordered phases** (SmartLifecycle)
2. **Startup failure cleanup**
3. **Liveness vs Readiness health distinction** (Actuator)
4. **Module bundling** (auto-configuration's real lesson — see SB4)
5. **Lifecycle events at milestones**
6. **Per-phase stop timeout**

---

### SB1. Integer-ordered PhaseKey (SmartLifecycle)

Spring's `SmartLifecycle.getPhase()` returns an `int`. Beans with lower phase numbers start
first and stop last. This is additive: any package can register at `PhaseCore` without
knowing what else runs there. Spring ships constants at `Integer.MIN_VALUE` (earliest) and
`Integer.MAX_VALUE` (latest).

Warren's sequential registration order is fine for a single-file wiring layer, but as soon
as multiple packages contribute to startup, someone needs to know the global registration
order. Integer phases dissolve that coupling.

**Proposed Warren design:**

```go
// Built-in phase order constants (users can define their own between these).
const (
    PhaseOrderInfrastructure = -1000  // logging, config, DB, tracing setup
    PhaseOrderCore           = 0      // default: core services
    PhaseOrderIntegration    = 1000   // HTTP server, queue consumers, gRPC
    PhaseOrderRuntime        = 2000   // begin accepting work, warm caches
)

// PhaseKey is an opaque, comparable phase identifier.
// Declare as package-level variables for discoverability.
//
//   var CorePhase = warren.NewPhase("core", warren.PhaseOrderCore)
type PhaseKey struct {
    name  string
    order int
}

func NewPhase(name string, order int, opts ...PhaseOption) PhaseKey

// Phase registers fn to run at key.Order during Start().
// Multiple registrations to the same PhaseKey accumulate in registration order
// within that phase level.
func (a *App) Phase(key PhaseKey, fn func(ctx context.Context, app *App) error) *App
```

Start() sorts registered phases by `order` before running them, then runs same-order phases
in registration order (stable). Stop runs in reverse order (lowest order stops last,
matching Spring: "last started, first stopped").

---

### SB2. Startup failure cleanup (already covered as A2)

Spring always runs `close()` on failure. Warren must run `OnStop` hooks in reverse for
already-completed phases when a later phase fails. This is the single most impactful
correctness fix.

---

### SB3. Liveness vs Readiness (Spring Boot Actuator)

Spring Boot Actuator distinguishes:
- **Liveness**: Is the application alive and not in a permanently broken state?
  Kubernetes restarts the pod on liveness failure.
- **Readiness**: Can the application serve traffic right now?
  Kubernetes removes the pod from the load balancer on readiness failure.
  Readiness can temporarily fail during startup, cache warming, etc.

Warren's flat `Health()` registry maps to neither. Both probe types need their own endpoint.

**Proposed Warren design:**

```go
type ProbeGroup string

const (
    // Liveness checks: fail = pod is broken, restart it.
    // Register DB connection health, required config, invariant assertions.
    Liveness ProbeGroup = "liveness"

    // Readiness checks: fail = pod is not ready to serve, remove from LB.
    // Register DB query latency, external service reachability, cache warmth.
    Readiness ProbeGroup = "readiness"
)

// Health registers fn in the given probe groups (default: both).
func (a *App) Health(name string, fn func(ctx context.Context) error, groups ...ProbeGroup)

// CheckGroup returns a HealthReport for only the checks in the given group.
// Use this to serve /healthz/live and /healthz/ready separately.
func (a *App) CheckGroup(ctx context.Context, group ProbeGroup) HealthReport

// Check returns a HealthReport for all registered checks (all groups).
func (a *App) Check(ctx context.Context) HealthReport
```

Example wiring:
```go
app.Health("db-connection",  db.Ping,         warren.Liveness)
app.Health("db-latency",     db.QueryLatency,  warren.Readiness)
app.Health("cache-warm",     cache.IsWarm,     warren.Readiness)
app.Health("config-loaded",  cfg.Validate,     warren.Liveness)
```

---

### SB4. Module bundling (the real lesson from auto-configuration)

Auto-configuration and component scanning exist to solve a specific problem: `main.go`
becoming a coupling point that every feature PR must touch. Every new subsystem requires
editing the central wiring file to add its construction, stop hook, and health check. Spring
solves this with reflection-based discovery. Go doesn't need reflection — the solution is
a `Module` type that each package exposes.

**The problem Warren has today:**

```go
// main.go — every new component requires editing this file
app.Phase(CorePhase, func(ctx context.Context, a *warren.App) error {
    db, err := postgres.Open(cfg.DatabaseURL)    // knows about db internals
    a.OnStop("db", db.Close)
    a.Health("db", db.Ping, warren.Liveness)
    cache := redis.Open(cfg.RedisAddr)           // knows about cache internals
    a.OnStop("cache", cache.Close)
    a.Health("cache", cache.Ping, warren.Readiness)
    // ... every new component appended here
    return err
})
```

**With `warren.Module`:**

```go
// pkg/db/module.go — db package describes its own wiring
var Module = warren.NewModule("db",
    warren.PhaseFunc(warren.PhaseOrderInfrastructure, func(ctx context.Context, a *warren.App) error {
        conn, err := postgres.Open(cfg.DatabaseURL)
        if err != nil { return err }
        a.OnStop("db", conn.Close)
        a.Health("db-conn",    conn.Ping,       warren.Liveness)
        a.Health("db-latency", conn.QueryCheck,  warren.Readiness)
        return nil
    }),
)

// pkg/cache/module.go — cache package describes its own wiring
var Module = warren.NewModule("cache", ...)

// main.go — knows nothing about how db or cache are wired internally
app.Use(db.Module)
app.Use(cache.Module)
app.Use(session.Module)
app.Use(server.Module)

if cfg.WorkerEnabled {
    app.Use(worker.Module)  // conditions are just if statements
}
```

`app.Use` registers the module's phases, stop hooks, and health checks. The `App` holds no
references to services — it remains a lifecycle coordinator. This is exactly what `fx.Module`
was designed to do; fx earned its bad reputation by adding reflection-based injection
*on top* of modules. The module concept itself is sound.

**What NOT to take from auto-configuration:**

- Classpath / reflection scanning — Go has no classpath; `init()` auto-registration is an
  anti-pattern (non-deterministic order, invisible coupling)
- `@ConditionalOnMissingBean` — requires a service registry; Go's `if` is the replacement
- `spring.factories` file-based discovery — the Go equivalent of plugins is complex and
  platform-specific; explicit `app.Use()` is always preferable

**Implementation:**

```go
type Module struct {
    name    string
    entries []moduleEntry
}

type moduleEntry struct {
    key PhaseKey
    fn  func(ctx context.Context, app *App) error
}

func NewModule(name string, entries ...ModuleEntry) Module

// PhaseFunc creates a ModuleEntry for a given phase.
func PhaseFunc(key PhaseKey, fn func(ctx context.Context, app *App) error) ModuleEntry

// Use registers all of a module's phase functions into the app.
// Equivalent to calling Phase() for each entry in the module.
func (a *App) Use(m Module) *App
```

`PhaseKey` integer ordering (SB1) is a prerequisite: modules from different packages all
registering at `PhaseOrderInfrastructure` need to sort correctly without coordinating their
registration sequence with each other.

---

### SB5. Lifecycle events at milestones

Spring publishes typed `ApplicationEvent` instances at each lifecycle milestone:
`ApplicationStartedEvent`, `ApplicationReadyEvent`, `ApplicationFailedEvent`,
`ApplicationStoppingEvent`. Any bean can listen without being explicitly wired to the
publisher.

Warren could offer a lightweight version without the full event bus:

```go
type EventType string
const (
    EventStarted  EventType = "started"   // all phases complete, goroutines running
    EventReady    EventType = "ready"     // app passed initial health checks
    EventStopping EventType = "stopping"  // Stop() called, goroutines being cancelled
    EventFailed   EventType = "failed"    // Start() failed (cleanup already run)
)

func (a *App) On(event EventType, fn func(ctx context.Context) error)
```

`EventReady` is distinct from `EventStarted`: it fires after `Check()` passes for the first
time. This is useful for "pre-warm" hooks that should run only once everything is healthy.

**Scope note:** This is the lowest-priority SB improvement. Warren's explicit phase model
already provides most of what events give. Add this only if there's a concrete use case.

---

### SB6. Per-phase stop timeout

Spring's `spring.lifecycle.timeout-per-shutdown-phase` applies a timeout per phase level.
Slow components in `PhaseRuntime` don't eat into the timeout budget for `PhaseCore`.

```go
var RuntimePhase = warren.NewPhase("runtime", PhaseOrderRuntime,
    warren.WithStopTimeout(15*time.Second),
)
```

When stopping, phases are processed in reverse order. Each phase gets its own timeout for
goroutines registered within it. If the runtime goroutines don't exit in 15s, the runtime
phase is reported as leaked, but core-phase goroutines still get their full timeout.

**Implementation note:** This requires goroutines to be associated with the phase that
registered them — `app.Go()` inside a phase function tags the goroutine with that phase's
`PhaseKey`. This is a deeper change to `GoroutineGroup`.

---

## Proposed Warren v2 API surface

Combining all of the above, the new public API would be:

```go
// Construction
func New(opts ...Option) *App
func WithShutdownTimeout(d time.Duration) Option

// Phase declaration
type PhaseKey struct{ name string; order int }
func NewPhase(name string, order int, opts ...PhaseOption) PhaseKey
func WithStopTimeout(d time.Duration) PhaseOption

const (
    PhaseOrderInfrastructure = -1000
    PhaseOrderCore           = 0
    PhaseOrderIntegration    = 1000
    PhaseOrderRuntime        = 2000
)

// Lifecycle registration (unchanged signatures except PhaseKey)
func (a *App) Phase(key PhaseKey, fn func(ctx context.Context, app *App) error) *App
func (a *App) Use(m Module) *App                                                 // new
func (a *App) Go(name string, fn func(ctx context.Context))
func (a *App) OnStop(name string, fn func(ctx context.Context) error)
func (a *App) On(event EventType, fn func(ctx context.Context) error)            // new

// Module bundling
type Module struct{ ... }
func NewModule(name string, entries ...ModuleEntry) Module                       // new
func PhaseFunc(key PhaseKey, fn func(ctx context.Context, app *App) error) ModuleEntry // new

// Health (context + probe groups)
type ProbeGroup string
const Liveness, Readiness ProbeGroup
func (a *App) Health(name string, fn func(ctx context.Context) error, groups ...ProbeGroup)
func (a *App) Check(ctx context.Context) HealthReport
func (a *App) CheckGroup(ctx context.Context, group ProbeGroup) HealthReport     // new

// Lifecycle
func (a *App) Start(ctx context.Context) error  // runs OnStop on failure (fixed)
func (a *App) Stop(ctx context.Context) error
func (a *App) Run(ctx context.Context) error
func (a *App) Active() map[string]int

// Test helper
func TestApp(t testing.TB, opts ...Option) *App
```

`Wire` and `Binding[T]` are unchanged — the review found no architectural issues with them
beyond `wireEntry.err` (string → error) and `Mark()` silent auto-register.

---

## Priority-ordered action list

### P0 — Correctness (fix before Warren is used in production wiring)

| # | Issue | File | Change |
|---|-------|------|--------|
| 1 | Startup failure leaks resources | `app.go` | Run `OnStop` in `Start()` error path |
| 2 | `Mark()` silently swallows typos | `wire.go` | Panic on undeclared name |
| 3 | `Go()` callable after `Stop()` | `app.go` | Add `stopped` guard |

### P1 — Architecture (core design improvements)

| # | Issue | File | Change |
|---|-------|------|--------|
| 4 | Raw strings for phases | `app.go` | `PhaseKey` type + integer ordering |
| 5 | `Check()` has no context | `app.go`, `health.go` | Add `context.Context` parameter |
| 6 | No Liveness/Readiness distinction | `app.go`, `health.go` | `ProbeGroup` + `CheckGroup()` |
| 7 | Exported mutable `ShutdownTimeout` | `app.go` | Functional options `WithShutdownTimeout` |
| 8 | `main.go` is a coupling point for all components | `app.go` (new) | `Module` type + `app.Use()` |

### P2 — Code style (clean up in a single pass)

| # | Issue | File | Change |
|---|-------|------|--------|
| 9 | `wireEntry.err` is `string` | `wire.go` | Change to `error` |
| 10 | Dead branch in `MultiError.Error()` | `errors.go` | Remove single-error branch |
| 11 | `Wait()` goroutine per call | `goroutines.go` | `sync.Once` guard |
| 12 | `Active()` nil vs empty map | `app.go` | Always return allocated map |
| 13 | `leakReport` in wrong file | `app.go` → `goroutines.go` | Move function |
| 14 | Missing comment on `context.Background()` in `Run()` | `app.go` | Add comment |

### P3 — Spring Boot additions (add when there's a concrete use case)

| # | Feature | Notes |
|---|---------|-------|
| 15 | Per-phase stop timeout | Requires tagging goroutines with their phase |
| 16 | Lifecycle events (`EventReady`, `EventFailed`, etc.) | Low priority — phases cover most use cases |
