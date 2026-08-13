# ADR-001: Dependency Injection and Lifecycle Pattern

**Status:** Accepted
**Date:** 2026-05-02
**Deciders:** Tyler Stapler

---

## Context

### Problem Statement

The stapler-squad codebase has accumulated coupling that is currently invisible to tooling:
no package-boundary linting is enforced, global mutable state is scattered across three
packages, and `main.go` (952 lines) contains three near-duplicate construction paths that
must be kept in sync manually. Beyond wiring, the codebase also lacks structured lifecycle
management: goroutines are started ad-hoc with no leak detection, shutdown hooks are
scattered, and the signal-handling loop is duplicated across entry points.

### Current State

**What works well:**

`server/dependencies.go` already implements the correct DI pattern: a typed
`ServerDependencies` aggregate struct plus three explicit phased constructors
(`BuildCoreDeps`, `BuildServiceDeps`, `BuildRuntimeDeps`). This is the reference
implementation. It was introduced deliberately and represents the team's already-chosen
direction for **wiring** dependencies.

**What needs repair:**

1. **Import cycles** — Two illegal cross-package imports exist:
   - `session/response_stream.go` → `server/analytics` (cycle: server imports session)
   - `session/unfinished/events.go` and `scanner.go` → `server/events` (same cycle pattern)
   These cycles prevent any package-boundary linting tool from being enabled.

2. **Package-level global state** — Three packages use package-level mutable globals:
   - `log/log.go`: five globals including `sessionLoggers map[string]*SessionLoggers` which
     grows without eviction (memory leak in long-running instances)
   - `config/config.go`: `globalCommandExecutor` with a test-only setter (an
     inversion-of-control violation)
   - `session/repo_path.go`: `DefaultRepoPathManager` (a shared cache; minor)

3. **Duplicated construction in `main.go`** — `session.NewEntRepository()` is called three
   times (web/test/PTY modes) and `log.InitializeWithConfig` is called four times with
   slightly different parameters. Any change to the session storage layer requires touching
   multiple paths.

4. **No structured lifecycle management** — goroutines are started with bare `go` statements
   and never tracked. Shutdown is a manual signal loop. Stop hooks are scattered. There is
   no goroutine leak detection. The ad-hoc approach cannot guarantee clean shutdown and
   makes integration testing lifecycle behavior difficult.

### Options Considered

**Option A: Google Wire (code generation)**

Wire generates `wire_gen.go` from provider declarations. Wiring errors are caught at
compile time, and the generated code is plain, readable Go with zero runtime overhead.

Pros: compile-time safety, no reflection, generated code is greppable.

Cons: requires a `wire` CLI build step (`go generate ./...`); easy to forget in PRs; adds
a new tool to install and maintain; no lifecycle management (goroutines, stop hooks, health
checks); the "giant ProviderSet" anti-pattern is common in early adoption.

**Option B: Uber fx (reflection-based runtime container)**

fx wires dependencies at application startup via reflection. Lifecycle hooks
(`OnStart`/`OnStop`) handle graceful shutdown.

Pros: no code generation, built-in lifecycle management, `fx.Module` encourages clean
separation.

Cons: 9 confirmed production flaws investigated — goroutine leak in `OnStart`, non-
deterministic value groups, interface injection requiring explicit `fx.As`, `fx.Decorate`
scope bug; wiring errors surface at runtime; reflection makes the dependency graph opaque;
the "global fx app in main.go" anti-pattern replicates the problem being solved.

**Option C: samber/do (service locator)**

A lightweight service locator backed by a `*do.Injector`. Services register themselves;
callers invoke `do.MustInvoke[T]`.

Cons: service-locator smell (callers reach into a registry rather than having dependencies
injected); v1→v2 breaking changes; solo maintainer risk; scope model unsuitable for
per-session dependencies.

**Option D: Warren (`pkg/warren/`) + manual three-phase pattern (chosen)**

Warren is a purpose-built lifecycle coordinator written for this project. It wraps the
existing manual three-phase pattern with:

- `App.Phase(name, fn)` — ordered initialization with explicit error propagation
- `App.Go(name, fn)` — named, tracked goroutines with leak detection
- `App.OnStop(name, fn)` — reverse-order shutdown hooks
- `App.Run(ctx)` — block until context cancelled, then drain gracefully
- `Binding[T]` — typed, test-overridable service references
- `Wire` — dependency setter tracking with validation and apply reporting

Warren adds zero framework dependencies to the production binary. The wiring itself remains
plain constructor injection — Warren only manages the lifecycle and validation layer on top.

---

## Decision

**Adopt Warren (`pkg/warren/`) as the lifecycle coordinator, and continue using manual
constructor injection as the wiring pattern.**

The governing principles:

1. **Wiring stays explicit and compile-time safe** — dependencies are constructor parameters,
   not registry lookups. The `BuildCoreDeps/BuildServiceDeps/BuildRuntimeDeps` chain is the
   wiring layer and remains unchanged structurally.

2. **Lifecycle management goes through Warren** — `App.Phase`, `App.Go`, `App.OnStop`, and
   `App.Run` replace the ad-hoc signal loop, bare `go` statements, and scattered shutdown
   code in `main.go`.

3. **No new package-level mutable globals** — new code that needs shared state receives it
   as a constructor parameter. `gochecknoglobals` enforces this in CI.

4. **Interfaces for all injected dependencies** — production code declares minimal
   interfaces for its dependencies so tests can inject doubles. Interfaces live in the
   package that consumes them.

The specific commitments:

1. **All new packages follow constructor injection:**
   ```go
   type FooService struct {
       storage InstanceStore  // unexported field
       logger  *slog.Logger   // unexported field
   }
   func NewFooService(storage InstanceStore, logger *slog.Logger) *FooService {
       return &FooService{storage: storage, logger: logger}
   }
   ```

2. **Phase boundaries are explicit** — new dependencies are placed in the correct phase
   struct (`CoreDeps`, `ServiceDeps`, or `RuntimeDeps`) based on initialization requirements.
   `RuntimeDeps` is the only phase allowed to start goroutines or open file handles.

3. **Goroutines started via `app.Go()`** — bare `go func()` in the wiring layer is
   forbidden for long-running goroutines. `app.Go` provides naming, leak detection, and
   graceful context propagation.

4. **`BuildOptions` struct for variadic construction** — when a `BuildXxxDeps` function
   needs optional parameters (e.g., a pre-constructed `ent.Client` for test or MCP mode),
   an options struct is used. The zero value must produce correct behavior.

---

## Consequences

### Positive

- Zero new framework dependencies. Warren is in-tree (`pkg/warren/`), has no external
  imports, and is fully tested (36 tests).
- Compile-time safety: missing dependencies are compiler errors, not runtime panics.
- Built-in lifecycle management: `app.Run()` handles signal propagation, ordered shutdown,
  goroutine leak detection, and health checks. No more manual signal loops.
- Goroutine leak detection in tests: `warren.TestApp(t)` gives tests an App with a 2-second
  shutdown timeout and auto-Stop via `t.Cleanup`, catching goroutine leaks immediately.
- The dependency graph is always legible: follow the `BuildCoreDeps` call chain to
  understand the full initialization order.
- Tests can construct subsystems in isolation by passing mock implementations of the
  interfaces declared in each package. `Binding[T].Override(t, ...)` provides test-scoped
  overrides that restore state automatically.
- The `gochecknoglobals` linter enforcement creates a ratchet: once a package is migrated,
  new globals are caught in CI immediately.

### Negative

- More boilerplate than Wire or fx for large graphs. If the graph grows beyond ~40 nodes,
  `BuildRuntimeDeps` will become long. Mitigate by splitting into feature-domain sub-
  builders (e.g., `BuildHistoryDeps`, `BuildDiscoveryDeps`).
- Warren is an in-tree library. If a future contributor wants to open-source it, the move
  requires extracting it to a separate module and updating the import path. This is a one-
  time rename, not an architectural change.
- The existing `main.go` still has the three duplicate construction paths until E3-S5-T3
  is complete. The decision accepts this debt as a tracked migration item.

### Neutral

- If the project grows to > 5 active contributors or the graph exceeds ~50 nodes, revisit
  Warren's feature set vs. Google Wire (compile-time graph validation). fx is not
  recommended for this codebase because it trades compile-time safety for convenience.
- Warren can be extracted to `github.com/tstapler/warren` as an open-source module without
  any code changes — only `go.mod` and import paths need updating.

---

## Migration Path

### Phase 1: Unblock enforcement (Pre-work tasks)

1. Extract `pkg/analytics/types.go` — fix `session/` → `server/analytics` cycle (PW-1)
2. Extract `pkg/events/types.go` — fix `session/unfinished/` → `server/events` cycle (PW-2)
3. Enable `depguard` in `.golangci.yml` with `no_server_in_core` rule (E1-S3-T1)
4. Enable `gochecknoglobals` with exclusions for the three known violating files (E3-S2-T1)

### Phase 2: Migrate the three highest-coupling packages

**Package 1: `log/` (highest risk, done first)**

Introduce `LogManager` struct wrapping all current globals. Keep package-level shims
(`var InfoLog`, etc.) as deprecated aliases populated from the default `LogManager`.
Add a `CloseSession(id string)` method to fix the unbounded map leak. Remove the
`gochecknoglobals` exclusion for `log/log.go` once complete.

Reference: E3-S3-T1 through E3-S3-T3.

**Package 2: `config/` (lowest risk, second)**

Add `CommandExecutor` as a constructor parameter to the config type. Keep the zero-arg
`NewConfig()` constructor as a wrapper using a default executor. Remove `SetCommandExecutor`
(test-only setter). Remove the `gochecknoglobals` exclusion for `config/config.go`.

Reference: E3-S4-T1 through E3-S4-T3.

**Package 3: `main.go` construction consolidation + Warren lifecycle (riskiest, last)**

1. Extract a `buildLogConfig` helper to deduplicate the four `InitializeWithConfig` calls.
2. Add `BuildOptions` struct to `BuildCoreDepsWithOptions` so MCP and PTY modes can reuse
   the same construction chain as the web server mode.
3. Wrap the three-phase build in `warren.App` lifecycle phases (E3-S5-T3).
4. Replace the manual signal loop with `app.Run(ctx)` (E3-S5-T4). This is the final step
   that removes the last ad-hoc lifecycle code from `main.go`.

Reference: E3-S5-T1 through E3-S5-T4.

### Phase 3: Ongoing enforcement (steady state)

- `gochecknoglobals` is active and exclusions are fully removed
- `depguard` blocks any new `session/` → `server/` import
- All new packages follow constructor injection; all goroutines go through `app.Go()`
- PRs that introduce globals fail lint; `// nolint:gochecknoglobals` with a justification
  comment is acceptable for intentional package-level constants or error sentinels

---

## Go Module Structure Recommendation

The current flat package structure is adequate for the current codebase size. No
reorganization is recommended at this time, with one exception:

The `pkg/` prefix introduced by PW-1 and PW-2 (for `pkg/analytics/` and `pkg/events/`)
establishes a convention for genuinely shared cross-layer types. Warren lives in `pkg/warren/`
under the same convention. Future shared contracts (e.g., event type definitions used by both
`session/` and `server/`) should go into `pkg/` rather than one layer importing from the other.

An `internal/` reorganization is not warranted for a self-contained application binary. The
application is not a library with external consumers, so `internal/` adds friction without
protecting a public API. Revisit if the project ever exposes a Go SDK.
