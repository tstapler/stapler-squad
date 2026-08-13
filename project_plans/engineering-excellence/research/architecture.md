# Architecture Research: Codebase Analysis

*All findings are from direct file reads and grep analysis of the actual codebase.*

---

## 1. Package Dependency Graph

### Top-Level Packages

| Package | Imports (internal) | Direction |
|---------|-------------------|-----------|
| `log/` | none (leaf) | Foundation |
| `executor/` | none (leaf) | Foundation |
| `config/` | `executor/`, `log/` | Core |
| `telemetry/` | `log/` | Cross-cutting |
| `session/` | `log/`, `session/git`, `session/scrollback`, `session/tmux` | Core domain |
| `server/` | `session/`, `config/`, `log/`, `telemetry/`, `server/events`, `server/services`, `server/analytics` | Application layer |
| `daemon/` | `config/`, `log/`, `session/` | Application layer |
| `main.go` | All of the above | Entry point |

### Verified Clean Directions

- `server/` → `session/` (expected): confirmed via `server/dependencies.go`, `server/adapters/`, `server/mcp/`
- `session/` → `log/` (expected): confirmed via `session/instance.go`
- `config/` → `log/`, `executor/` (expected): confirmed via `config/config.go`
- `telemetry/` → `log/` (expected): confirmed via `telemetry/telemetry.go`
- `daemon/` → `config/`, `log/`, `session/` (expected): confirmed via `daemon/daemon.go`

---

## 2. Illegal Cross-Package Dependencies (Architecture Violations)

**Three violations found:**

### Violation 1: `session/response_stream.go` imports `server/analytics`
```
session/response_stream.go:7: "github.com/tstapler/stapler-squad/server/analytics"
```
This is a **cycle**: `server/` depends on `session/`, and `session/` depends on `server/analytics`. This violates the layered architecture.

**Fix:** Move `server/analytics` types into `pkg/analytics/` or `session/analytics/` and have both `server/` and `session/` import from there.

### Violation 2: `session/unfinished/events.go` imports `server/events`
```
session/unfinished/events.go:6: "github.com/tstapler/stapler-squad/server/events"
session/unfinished/scanner.go:20: "github.com/tstapler/stapler-squad/server/events"
```
Same cycle pattern: `session/unfinished` reaches up into `server/events`.

**Fix:** Move shared event types (`server/events/types.go` already imports `session/`) into a `pkg/events/` package. Both `server/events` and `session/unfinished` import from `pkg/events/`.

### Summary of Violations

```
session/ ──(imports)──► server/analytics   [ILLEGAL: cycle]
session/unfinished/ ──► server/events      [ILLEGAL: cycle]
```

These two violations are the most urgent architectural repairs. They prevent tools like `depguard` from enforcing the layer boundary and create subtle initialization order issues.

---

## 3. Global Mutable State Inventory

### Critical Globals (high risk, accessed by many callers)

| Symbol | Package | File | Type | Risk |
|--------|---------|------|------|------|
| `globalCommandExecutor` | `config` | `config.go:56` | `CommandExecutor` | HIGH — used in config.go:382, replaceable but widely called |
| `globalConfig` | `log` | `log.go:86` | `*LogConfig` | HIGH — guards session log routing; set once but read in every log call |
| `sessionLoggers` | `log` | `log.go:89` | `map[string]*SessionLoggers` | HIGH — unbounded growth map, protected by `sessionMutex` |
| `structuredLogger` | `log` | `log.go:93` | `*StructuredLogger` | MEDIUM — set once at init |
| `WarningLog`, `InfoLog`, `ErrorLog`, `DebugLog` | `log` | `log.go:80-83` | `*log.Logger` | MEDIUM — legacy loggers, replaced by structured but still used |
| `logFileName` | `log` | `log.go:139` | `string` | LOW — set at package init, never mutated |
| `DefaultRepoPathManager` | `session` | `repo_path.go:203` | `*RepoPathManager` | MEDIUM — used in 3 places; contains a cache map |
| `allowedTransitions` | `session` | `state_machine.go:24` | `map[Status][]Status` | LOW — read-only after init |
| `claudeProjectsPattern` | `session` | `history_detector.go:55` | `*regexp.Regexp` | LOW — immutable |
| `timeNow` | `session` | `instance_workspace.go:562` | `func() time.Time` | MEDIUM — used for test injection, mutable in tests |

### Worst Offenders

**1. `log/log.go` — 5 package-level globals forming a mutable logging subsystem**

The `log` package uses `globalConfig *LogConfig`, `sessionLoggers map[string]*SessionLoggers`, `structuredLogger`, and four `*log.Logger` globals. These form an implicit global singleton. The map `sessionLoggers` grows indefinitely (sessions are added but never removed from the map). This is a memory leak in long-running instances.

```go
// Current (problematic)
var sessionLoggers map[string]*SessionLoggers  // never cleaned up
var globalConfig *LogConfig                    // set via InitializeWithConfig()
```

**Fix:** Inject a `*LogManager` struct (holding config + session logger cache with eviction) through constructors instead of using package-level globals.

**2. `config/config.go:56` — `globalCommandExecutor`**

```go
var globalCommandExecutor CommandExecutor = newTimeoutCommandExecutor(5 * time.Second)
```

Used in `config.go:382` to run shell commands. This is replaceable via `SetCommandExecutor()` but the setter is only used in tests. In production, the global is always used.

**Fix:** Pass `CommandExecutor` as a constructor parameter to any config function that needs it.

**3. `session/repo_path.go:203` — `DefaultRepoPathManager`**

```go
var DefaultRepoPathManager = NewRepoPathManager()
```

Contains an internal cache. Used in 3 places. Tests that need different behavior must work around this global.

**Fix:** Inject `*RepoPathManager` through `NewSessionService` or similar constructors.

---

## 4. `main.go` Coupling Analysis

**File size:** 952 lines, 7 top-level functions.

### What It Instantiates

- `log.InitializeWithConfig(...)` — called 4 separate times for different command modes
- `telemetry.Initialize(ctx, cfg)` — one call
- `session.NewEntRepository()` — called 3 times (web server mode, test mode, PTY mode)
- `session.NewStorageWithRepository(repo)` — called 3 times
- `server.NewServer(address)` — one call
- `server.BuildDependencies()` — via `BuildCoreDeps` pattern
- `mcpserver.InitMCPLogging()` — package-level init side effect

### The Wiring Cost

`main.go` contains a large `init()` function (line 621) registering Cobra flags, plus the entire command dispatch via a `rootCmd` cobra tree. The actual server startup logic is mostly delegated to `BuildDependencies()` in `server/dependencies.go`, which is the right pattern.

**Main problems:**
1. `log.InitializeWithConfig` is called 4× with slightly different parameters — this should be a single `buildLogConfig()` helper
2. `session.NewEntRepository()` is called 3× — each call opens a new SQLite connection; the MCP mode calls `buildMCPDeps()` which correctly delegates, but the test/PTY modes inline the construction
3. The `init()` function performs meaningful work (not just flag registration), making it hard to test

### Wiring Cost Score: HIGH

The 3 duplicate construction paths (web/test/PTY mode) and the 4 `InitializeWithConfig` calls represent significant maintenance surface. Any change to the session storage layer requires touching 3 code paths in main.go.

---

## 5. Constructor Patterns by Package

### Packages Using Constructor Injection (Good)

| Package | Constructor | Struct fields private? |
|---------|-------------|----------------------|
| `server/` | `server.NewServer(addr)` | Yes |
| `server/services/` | `services.NewSessionService(store, ...)` | Yes |
| `session/` | `session.NewStorage(repo)`, `session.NewEntRepository()` | Yes |
| `session/scrollback` | `scrollback.NewScrollbackManager(cfg)` | Yes |
| `server/dependencies.go` | `BuildCoreDeps()` / `BuildServiceDeps()` / `BuildRuntimeDeps()` | N/A (aggregate) |

### Packages Using Package-Level Globals (Needs Migration)

| Package | Global | Impact |
|---------|--------|--------|
| `log/` | `globalConfig`, `sessionLoggers`, `WarningLog`, `InfoLog`, etc. | HIGH |
| `config/` | `globalCommandExecutor` | MEDIUM |
| `session/` | `DefaultRepoPathManager` | LOW-MEDIUM |

---

## 6. Top 3 Most Coupled Packages (Migrate First)

### Rank 1: `log/` — The Most Dangerous Global State

**Why first:** Every other package imports `log/`. Its 5 package-level globals (including the `sessionLoggers` map that never evicts entries) are a latent memory leak and make the package impossible to test in isolation. Migrating `log/` to a `LogManager` struct with constructor injection unblocks all other packages.

**Migration effort:** Medium. Introduce `type LogManager struct` wrapping current globals. Keep the free functions (`log.InfoLog.Printf`) as package-level shims backed by a default `LogManager` during transition. Replace call sites incrementally.

### Rank 2: `session/` — The Largest Package (105K-line instance.go)

**Why second:** `session/instance.go` is 105,400 bytes — likely the largest single Go file in the codebase. It combines lifecycle management, state machine, tmux integration, git worktree management, terminal state, and approval logic. This god-file is the source of the `session/` → `server/analytics` and `session/unfinished/` → `server/events` cycles.

**Migration effort:** High. Split `instance.go` along feature lines first (lifecycle, approval, terminal, git). Then fix the import cycles by extracting shared types to `pkg/`.

### Rank 3: `config/` — The Config God Package

**Why third:** `config/config.go` is 22.8K, `session/config.go` is also large. `config` imports both `executor/` and `log/`, and is imported by almost every other package. The `globalCommandExecutor` global and the side effects in package initialization make config hard to test.

**Migration effort:** Low-medium. The `CommandExecutor` injection pattern is already partially in place (test setter exists). Completing it is straightforward.

---

## 7. Structural Summary

```
main.go (952 lines, 3 duplicated construction paths)
    │
    ├── server/dependencies.go ◄── GOOD: three-phase build pattern
    │       ├── session/ ◄── GOOD: constructor injection throughout
    │       │      ├── session/response_stream.go ◄── BAD: imports server/analytics [CYCLE]
    │       │      └── session/unfinished/ ◄── BAD: imports server/events [CYCLE]
    │       ├── config/ ◄── MEDIUM: globalCommandExecutor global
    │       └── log/ ◄── BAD: 5 package-level globals, never-evicting map
    │
    └── telemetry/ → log/ (clean)
```

**Priority order for migration:**
1. Fix `session/` → `server/` cycles (extract `pkg/analytics/` and `pkg/events/`)
2. Migrate `log/` globals to `LogManager` struct
3. Complete the `main.go` deduplication (single construction path for all modes)
4. Migrate `config.globalCommandExecutor` to constructor parameter
