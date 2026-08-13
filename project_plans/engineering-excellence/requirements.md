# Engineering Excellence: Maintainability, Observability & Architectural Standards

## Project Goal

Transform the stapler-squad codebase into a project that stays maintainable under sustained feature
development: silent regressions caught at PR time, production failures debuggable in minutes, and
architectural decay prevented by automated enforcement rather than discipline.

## Background

The project currently has:
- CI that passes but doesn't guarantee feature correctness (integration gaps, no coverage gate)
- Partial OpenTelemetry wiring but no structured logging, no trace correlation in logs, no error
  tracking
- No dependency injection framework — dependencies wired manually throughout a 32K-line main.go
- Linting limited to forbidigo / staticcheck / govet with no package-boundary rules
- Benchmark CI that captures baselines but no regression gate blocking PRs

All four pain areas are present simultaneously and must be addressed in parallel.

---

## Epics

### Epic 1: PR Validation Gates

Make CI a reliable correctness gate, not just a compilation check.

**Requirements:**

1. **Test coverage threshold** — CI fails if Go test coverage drops below a configurable threshold
   (target: 60% line coverage for `server/`, `session/`, `config/` packages as a starting baseline).
   Coverage must be measured per-package and reported as an artifact.

2. **Integration test gate** — Any PR that adds or modifies a proto RPC method must include at
   least one integration or e2e test covering that RPC. CI detects this via a gate script that
   cross-references changed proto files with test presence.

3. **Architecture lint gate** — `depguard` or equivalent enforces package-boundary rules in CI
   (e.g., `session/` must not import `server/`; `config/` must not import `session/`). Violations
   block merge.

4. **Benchmark regression gate** — CI compares current benchmark results against a committed
   baseline and fails if any benchmark regresses > 20% (configurable). Uses the existing
   `make benchmark-compare` infrastructure.

**Success criteria:**
- Merging a PR that drops coverage below threshold blocks the PR
- Adding an RPC without a test blocks the PR
- A circular import between layers blocks the PR
- A 25% benchmark regression blocks the PR

---

### Epic 2: Observability & Debugging

Make production failures debuggable in minutes, not hours.

**Requirements:**

1. **Structured logging migration** — Replace all `fmt.Println` / `log.Printf` / `log.Println`
   calls in non-test, non-cmd code with `slog` (stdlib, Go 1.21+). Each log line must carry
   request-scoped fields: `session_id`, `operation`, and `component` at minimum.

2. **Trace ID injection into logs** — When an OpenTelemetry span is active, inject `trace_id` and
   `span_id` into every slog log line via a custom slog handler. This allows correlating log lines
   with traces in any OTLP backend.

3. **Always-on lightweight profiling** — Integrate continuous profiling that is active by default
   in production (no `--profile` flag required). Pyroscope Go SDK or equivalent push-based
   continuous profiling. Must add < 5% CPU overhead and zero configuration overhead for the
   developer.

4. **Error event tracing** — When an error is returned from any ConnectRPC handler, record it as
   an OpenTelemetry span event with: error message, stack trace (first 5 frames), and the request
   metadata that caused it.

5. **Error tracking system** — Implement a persistent error registry that:
   - Deduplicates errors by fingerprint (error type + first 3 stack frames)
   - Tracks occurrence count and first/last seen timestamps
   - Persists to SQLite (reuses the existing ent ORM)
   - Exposes errors via a new ConnectRPC endpoint (`ListErrors`, `AcknowledgeError`)
   - Renders in the web UI as a simple error dashboard (count, rate, last seen, stack trace)
   - Configurable: can be disabled via config flag for minimal deployments

**Success criteria:**
- Every log line in a ConnectRPC handler includes `trace_id` when tracing is active
- Pyroscope (or equivalent) profiles are accessible without restarting the server
- The same error occurring 10 times shows as one record with count=10 in the error dashboard
- Developers can search/filter errors in the web UI and mark them acknowledged

---

### Epic 3: Dependency Injection & Architectural Standards

Prevent coupling from silently accumulating; make the architecture self-documenting.

**Requirements:**

1. **Codebase DI analysis** — Before prescribing a framework, run a full dependency graph analysis
   of the current codebase to identify:
   - Packages that use package-level global state (`var` at package scope that is mutable)
   - Functions that construct their own dependencies internally (violating inversion of control)
   - Circular or inappropriate cross-package dependencies
   - The actual "wiring cost" of moving to explicit constructor injection

2. **DI pattern decision** (research-driven) — Based on the analysis and research into Go DI
   ecosystem (Wire, Uber fx, manual constructors), choose the pattern that matches:
   - The project's solo/small-team scale
   - Compile-time safety preference (Spring Boot analogy = catch wiring errors at startup, not
     runtime)
   - Minimal ceremony for common cases
   The chosen approach must be documented as an ADR.

3. **Enforce chosen DI pattern in CI** — Once the pattern is chosen:
   - Add a linter rule (custom or via golangci-lint) that flags new global mutable state
   - Add a depguard rule for each package layer (see Epic 1 requirement 3)
   - Add a `make vet-architecture` target that runs all structural checks

4. **Migrate highest-coupling packages first** — Apply the chosen DI pattern to the top 3 most
   coupled packages identified in the analysis. This serves as both a proof of concept and a
   reference implementation for future work.

5. **Go module structure review** — Evaluate whether the current flat package structure should be
   reorganized (e.g., `internal/` for implementation details, explicit public API packages). Produce
   a recommendation in the ADR without necessarily migrating immediately.

**Success criteria:**
- ADR written and committed documenting the DI decision with rationale
- At least 3 packages migrated to the chosen DI pattern
- CI catches any new global mutable state introduced in non-test code
- Package boundary violations block PRs

---

## Non-Goals

- Full rewrite of `main.go` — decompose incrementally as part of DI migration, not a big bang
- External error tracking SaaS (Sentry, Datadog) — self-hosted SQLite-based tracking is the MVP
- 100% test coverage — pragmatic threshold, not perfection
- Replacing tmux or proto APIs — this is purely internal quality work

## Constraints

- Go 1.21+ (slog is stdlib)
- Existing ent ORM and SQLite must be reused for new persistence (no new DB dependencies)
- New CI gates must not add more than 5 minutes to the existing CI pipeline
- All changes must maintain backward compatibility with the existing config format and state files
- Solo developer / small team: patterns must be understandable without deep framework knowledge

## Research Needed

1. **Stack**: Go DI frameworks (Wire vs fx vs manual), depguard configuration, slog migration
   tooling, continuous profiling options (Pyroscope, parca, VictoriaMetrics), Go coverage gate CI
   patterns

2. **Features**: Spring Boot-inspired opinionated Go patterns, best-in-class Go error tracking
   implementations (self-hosted), benchmark regression gate implementations in Go CI

3. **Architecture**: Current coupling analysis of stapler-squad packages, package dependency graph,
   global state inventory, realistic migration path for main.go decomposition

4. **Pitfalls**: DI framework adoption anti-patterns in Go, slog migration gotchas, OTel +
   structured logging performance, benchmark flakiness in CI gates, coverage measurement in
   integration-heavy Go codebases
