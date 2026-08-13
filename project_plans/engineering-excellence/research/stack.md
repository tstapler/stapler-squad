# Stack Research: Engineering Excellence

## 1. Go DI Frameworks: Wire vs Uber fx vs Manual Injection

### Overview

| Framework | Approach | Overhead | Team Fit |
|-----------|----------|----------|----------|
| Google Wire | Code generation, compile-time | Zero runtime | Small teams, compile-time safety critical |
| Uber fx | Reflection, runtime | ~1-5ms startup | Large teams, lifecycle-heavy apps |
| Manual (constructor) | Explicit, no framework | Zero | Any size, maximum readability |

### Google Wire

Wire is a compile-time code-generation tool. You write _provider_ functions (plain Go constructors) and a _wire set_ that declares dependencies between them. Running `wire` generates a `wire_gen.go` file containing explicit initialization code. The generated code is readable, debuggable, and carries zero runtime overhead.

**Pros:**
- Wiring errors caught at compile time (not at `main()` boot)
- Generated code is plain Go — no magic, fully greppable
- No import of Wire itself in production binary
- Server restart time is shorter because zero reflection

**Cons:**
- Requires `wire` CLI as a build step (`go generate ./...`)
- Provider declarations require discipline (`wire.NewSet`, `wire.Bind` idioms have a learning curve)
- Adding a new dependency requires re-running `wire` — easy to forget in PRs

**Example provider pattern:**
```go
// session/providers.go
func NewStorage(cfg *config.Config, repo *ent.Client) *Storage { ... }
func NewSessionService(store InstanceStore) *services.SessionService { ... }

// wire.go (NOT committed, only used to generate wire_gen.go)
//go:build wireinject
func InitializeServer(cfg *config.Config) (*server.Server, error) {
    wire.Build(
        config.NewEntClient,
        session.NewStorage,
        services.NewSessionService,
        server.NewServer,
    )
    return nil, nil
}
```

### Uber fx

fx is a full application framework built on top of Uber's `dig` container. It uses reflection at startup to wire dependencies, and provides lifecycle hooks (`fx.Hook` with `OnStart`/`OnStop`) for graceful shutdown.

**Pros:**
- No code generation step
- Built-in lifecycle management (graceful shutdown is first-class)
- `fx.Module` encourages clean separation of concerns
- Very popular in large Uber-style Go services

**Cons:**
- Reflection-based: wiring errors surface at runtime on first `app.Run()`
- 1-5ms startup overhead (negligible for servers, matters for CLIs/tests)
- Steeper learning curve: `fx.In` structs, `fx.Provide`, `fx.Invoke`
- Makes the dependency graph opaque without tooling

**Example:**
```go
app := fx.New(
    fx.Provide(
        config.NewConfig,
        session.NewStorage,
        server.NewServer,
    ),
    fx.Invoke(func(srv *server.Server) { /* start */ }),
)
app.Run()
```

### Manual Constructor Injection (Recommended for Stapler Squad)

For a solo/small-team project, manual injection remains the Go community consensus recommendation. The explicit `BuildCoreDeps` → `BuildServiceDeps` → `BuildRuntimeDeps` pattern already present in `server/dependencies.go` is the correct approach — it just needs to finish migrating the remaining wiring out of `main.go`.

**Why manual fits this codebase:**
- `server/dependencies.go` already defines `ServerDependencies` as a typed aggregate — this is the Wire pattern without the tool
- The phased construction (`Phase 1/2/3`) maps directly to explicit constructor call chains
- No additional tooling to install, configure, or maintain
- Full IDE support (no generated code to confuse IntelliSense)

**Recommendation:** Complete the manual DI migration — pull the remaining construction in `main.go` into `BuildRuntimeDeps` or equivalent. If the graph exceeds 30+ nodes and the boilerplate becomes a maintenance burden, adopt Wire (code-gen) over fx (reflection) because this codebase values compile-time correctness.

---

## 2. depguard: Layer Dependency Rules in .golangci.yml

depguard v2 supports per-file rule sets. The canonical pattern for enforcing `session/ must not import server/`:

```yaml
# .golangci.yml
linters:
  enable:
    - depguard

linters-settings:
  depguard:
    rules:
      # Core packages (session, config, log) must not import server layer
      no_server_in_core:
        list-mode: deny
        files:
          - "**/session/**/*.go"
          - "**/config/**/*.go"
          - "**/log/**/*.go"
          - "!**/*_test.go"
        deny:
          - pkg: "github.com/tstapler/stapler-squad/server"
            desc: "session/config/log must not import server (architecture violation)"
          - pkg: "github.com/tstapler/stapler-squad/server/**"
            desc: "session/config/log must not import server subpackages"

      # Ban deprecated io/ioutil
      no_ioutil:
        list-mode: deny
        files: ["$all"]
        deny:
          - pkg: "io/ioutil"
            desc: "replaced by io and os packages since Go 1.16"
```

**Known violation to fix first:** `session/response_stream.go` currently imports `server/analytics` — this is a cycle. The analytics types need to move to a shared `pkg/analytics/` package or the import direction inverted.

**File variables:**
- `$all` — every .go file
- `$test` — only `*_test.go` files
- `!$test` — exclude test files from a rule

---

## 3. slog Migration: Tooling and Patterns

### Is There Auto-Migration Tooling?

There is no purpose-built `log.Printf` → `slog.InfoContext` codemod tool as of 2026. The closest approaches:

1. **`slog.SetDefault` bridge** (zero-code migration): Call `slog.SetDefault(yourSlogLogger)` in `main()`. From that point, all `log.Printf` calls automatically route through the slog handler. This is the lowest-risk first step.

2. **`gofmt -w` + sed/gorename** for mechanical substitution — community-maintained scripts exist but are not official.

3. **`gopatch`** (google/gopatch) — a Go AST patch tool that can express transformations like:
   ```
   @@
   @@
   -log.Printf(%s, ...)
   +slog.InfoContext(ctx, %s, ...)
   ```
   Requires context variable to be in scope at each call site.

### Recommended Migration Strategy

Given the codebase uses a custom `log/log.go` package wrapping stdlib `log.Logger`, the migration path is:

**Phase 1 — Bridge:** Add `slog.SetDefault(slog.New(yourHandler))` after `log.InitializeWithConfig` in main. Existing `log.Printf` calls continue working but now emit structured log records.

**Phase 2 — New code only:** Enforce that all new code uses `slog.InfoContext(ctx, ...)` instead of `log.InfoLog.Printf(...)`. Add a `nolintlint` or custom linter rule.

**Phase 3 — Gradual replacement:** Migrate hot paths (session creation, streaming) first. Add `// nolint:slogmigrate` suppressions on frozen code.

### Production Patterns

- Always use `slog.InfoContext(ctx, "msg", "key", val)` over `slog.Info` to carry trace IDs automatically
- Pre-build `slog.Logger` with `.With("component", "session")` per subsystem — avoids repeating component key
- Keep `log.Logger` globals as deprecated shims until fully migrated; don't delete them in the first pass

---

## 4. Continuous Profiling: Pyroscope vs Parca vs eBPF

### Comparison Matrix

| Tool | Deployment | Overhead | Go Support | Self-Host Complexity |
|------|-----------|----------|------------|---------------------|
| Pyroscope (Grafana) | Push (SDK in app) | 0.5-2% | Native Go agent | Medium (Docker) |
| Parca (Polar Signals) | Pull (eBPF or Go agent) | <1% (eBPF) | Yes | Low (single binary) |
| eBPF-only (perf/bpftrace) | Kernel-level | <0.5% | Language-agnostic | High (kernel access) |

### Recommendations for Stapler Squad

The codebase already has `profiling/` and `--profile` flags with pprof HTTP endpoints. This is the pragmatic foundation.

**Tier 1 (already implemented):** pprof endpoints on `localhost:6060`. Sufficient for on-demand profiling of specific lock-ups.

**Tier 2 (add Pyroscope):** For continuous profiling, the Pyroscope Go SDK is the lowest-friction addition:
```go
import "github.com/grafana/pyroscope-go"

pyroscope.Start(pyroscope.Config{
    ApplicationName: "stapler-squad",
    ServerAddress:   "http://pyroscope:4040",
    ProfileTypes: []pyroscope.ProfileType{
        pyroscope.ProfileCPU,
        pyroscope.ProfileAllocObjects,
        pyroscope.ProfileGoroutines,
    },
})
```

**Tier 3 (Parca for infra-wide):** If running in Kubernetes or want zero-code-change profiling, Parca's eBPF agent profiles all processes without SDK changes. Overhead is under 1%.

**Overhead guidance:** At 10Hz sampling (Pyroscope default), CPU overhead is 0.5-1% for a typical Go HTTP server. For a localhost dev tool like stapler-squad, this overhead is irrelevant in production but worth disabling in tests.

---

## 5. Go Test Coverage Gates in CI

### Tool Stack

```yaml
# .github/workflows/ci.yml
- name: Run tests with coverage
  run: go test -coverprofile=coverage.out -covermode=atomic ./...

- name: Coverage gate (go-test-coverage)
  uses: vladopajic/go-test-coverage@v2
  with:
    profile: coverage.out
    local-threshold: 70       # per-file minimum
    global-threshold: 75      # overall minimum
    badge-file-name: coverage.svg

- name: Vulnerability scan
  run: |
    go install golang.org/x/vuln/cmd/govulncheck@latest
    govulncheck ./...
```

### go-test-coverage

`vladopajic/go-test-coverage` is the most-used Go-specific coverage gate action. It supports:
- Global threshold (fail if overall < N%)
- Per-file threshold (fail if any file < N%)
- Per-package threshold
- Badge generation for README
- Delta comparison (fail if coverage _decreased_ even if above threshold)

### Integration Test Coverage (Go 1.20+)

For packages like `session/` that are heavily tested via integration tests rather than unit tests, use `go build -cover` to instrument the binary:

```bash
go build -cover -o stapler-squad-cov .
GOCOVERDIR=/tmp/covdata ./stapler-squad-cov &
# run integration tests
go tool covdata textfmt -i=/tmp/covdata -o=integration.out
go tool cover -func=integration.out
```

This approach merges unit and integration coverage into a single profile, giving a more accurate picture than unit tests alone.

### govulncheck

`govulncheck` scans for known CVEs in transitive dependencies. It reports only vulnerabilities in code paths that are actually called (not just imported), significantly reducing false positives vs. `go list -m all | snyk`.

```yaml
- name: Vulnerability check
  run: govulncheck ./...
  continue-on-error: false   # block the build on critical CVEs
```
