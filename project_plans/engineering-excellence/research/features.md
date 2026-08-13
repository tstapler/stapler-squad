# Features Research: Engineering Excellence

## 1. Spring Boot Patterns That Translate to Go

Spring Boot's key contribution to Java is _opinionated autoconfiguration_ — you declare what you need, the container wires it. Go deliberately has no built-in DI container, so the community has converged on three patterns that achieve the same safety guarantees:

### Pattern 1: Constructor Injection (Direct Spring Analog)

Spring `@Component` + `@Autowired` constructor → Go plain constructor functions:

```java
// Spring Boot
@Component
public class SessionService {
    private final Storage storage;
    @Autowired
    public SessionService(Storage storage) { this.storage = storage; }
}
```

```go
// Go equivalent
type SessionService struct {
    storage InstanceStore
}
func NewSessionService(storage InstanceStore) *SessionService {
    return &SessionService{storage: storage}
}
```

The Go version is strictly better in one dimension: the dependency contract is enforced by the compiler (not a runtime annotation scan) and it's explicit (no magic).

### Pattern 2: Aggregate "Deps" Struct (Spring ApplicationContext analog)

Spring's ApplicationContext is a map of beans. In Go, the `server/dependencies.go` `ServerDependencies` struct is the idiomatic equivalent:

```go
type ServerDependencies struct {
    SessionService    *services.SessionService
    Storage           *session.Storage
    EventBus          *events.EventBus
    ScrollbackManager *scrollback.ScrollbackManager
    // ...16 more fields
}
```

This pattern is used by many large Go services (Prometheus, InfluxDB, CockroachDB). The build function (`BuildDependencies`) acts as the Spring container's `refresh()` — it instantiates and wires everything in the correct order.

### Pattern 3: Interface-Based Mocking (Spring `@MockBean` analog)

Spring's `@MockBean` replaces a bean with a Mockito mock in tests. In Go, the analog is declaring a minimal interface and providing a test double:

```go
// In production code
type InstanceStore interface {
    GetInstance(id string) (*Instance, error)
    // ...
}

// In tests
type mockStore struct { /* test-local fields */ }
func (m *mockStore) GetInstance(id string) (*Instance, error) { return m.sessions[id], nil }
```

### What the Go Community Does Not Do

- No annotation scanning (`reflect`-based struct tags for DI are an anti-pattern in Go)
- No XML/YAML bean definitions
- No singleton registries (singletons are achieved by constructing once and passing the pointer)
- No `@PostConstruct` — use explicit `Start(ctx context.Context) error` methods instead

### Recommendation

The stapler-squad codebase is already following the right patterns. The missing piece is moving the remaining wiring out of `main.go` (952 lines) into the three-phase `BuildCoreDeps/BuildServiceDeps/BuildRuntimeDeps` structure that `server/dependencies.go` already defines.

---

## 2. Self-Hosted Error Tracking for Go

### Options Ranked for Solo/Small Team

| Tool | Infrastructure | Sentry SDK compatible | Maintenance burden |
|------|---------------|----------------------|-------------------|
| GlitchTip | 4 containers (~2GB RAM) | Yes | Low |
| Bugsink | 1 container (~256MB RAM) | Yes | Very low |
| Sentry (self-hosted) | 40+ containers (~16GB RAM) | Yes | Very high |
| Highlight.io | Was cloud-only | Yes | DEAD — shut down Feb 2026 |

### GlitchTip (Recommended)

GlitchTip is a drop-in Sentry replacement that runs on 4 containers (web, worker, postgres, redis). It implements the Sentry SDK protocol, so existing `@sentry/go` instrumentation works unchanged — just update the DSN URL.

**Go integration:**
```go
import "github.com/getsentry/sentry-go"

sentry.Init(sentry.ClientOptions{
    Dsn: "https://key@glitchtip.your-host.internal/1",  // just change the host
    Release: version,
    Environment: "production",
})

// Capture panics automatically
defer sentry.Recover()

// Manual error capture
sentry.CaptureException(err)
```

**Resource requirements:** Comfortably runs on a 2-CPU, 2GB VM. For a single-server dev tool, this is an easy Docker Compose deployment.

### Bugsink (If Minimalism is Priority)

Bugsink is a single-container error tracker (SQLite or Postgres backend) that focuses on one job: "tell you when something broke and why." No dashboards, no traces, no uptime checks. It is Sentry SDK compatible.

**When to choose Bugsink:** You want the smallest possible self-hosted footprint, and you don't need the Sentry ecosystem's full feature set.

### SQLite-Native Error Registry Pattern

If no external service is desired, a lightweight pattern used in smaller Go projects:

```go
// pkg/errorregistry/registry.go
type ErrorRegistry struct {
    db *sql.DB
}

func (r *ErrorRegistry) Record(ctx context.Context, err error, attrs map[string]any) {
    // Insert into errors table with trace_id, timestamp, stack, attrs
}
```

SQLite is appropriate for low-volume error tracking (< 10K errors/day). Above that, use GlitchTip.

---

## 3. Benchmark Regression Gate CI Patterns

### The Standard Toolchain

The Go benchmark ecosystem has converged on:
1. `go test -bench=. -benchmem -count=5` — run benchmarks with statistical sampling
2. `benchstat` (rsc/benchstat, v2) — compare two benchmark result files with statistical significance
3. `github-action-benchmark` or `bencherdev/bencher` — store and compare results across commits

### Pattern: benchstat in CI

```yaml
# .github/workflows/bench.yml
- name: Run benchmarks (current)
  run: go test -bench=. -benchmem -count=5 ./... > bench-current.txt

- name: Checkout baseline
  run: git checkout main -- bench-baseline.txt

- name: Compare with benchstat
  run: |
    go install golang.org/x/perf/cmd/benchstat@latest
    benchstat bench-baseline.txt bench-current.txt > bench-diff.txt
    cat bench-diff.txt
    # Fail if any benchmark regressed > 10%
    if grep -E '\+[0-9]{2,}%' bench-diff.txt; then
      echo "Benchmark regression detected"
      exit 1
    fi
```

### Threshold Guidance

- **10% regression**: common starting threshold; catches meaningful regressions without noise
- **25% threshold**: for inherently noisy benchmarks (filesystem, network-touching)
- **Statistical approach**: benchstat v2 computes p-values — only flag regressions with p < 0.05

### How Real Projects Do It

**Go stdlib:** Uses `perf.golang.org` continuous benchmarking dashboard. PRs that touch performance-sensitive packages get benchmark comparisons posted as comments.

**Prometheus:** Stores benchmark results in a dedicated branch and uses benchstat to compare PR vs main before merge.

**cob (knqyf263/cob):** A simple tool that compares benchmarks between the latest and previous commit. Good for fast feedback in individual PR runs.

**github-action-benchmark:** Stores results as GitHub Pages JSON, draws trend charts, posts PR comments with delta, and optionally fails the check on regression.

### Stapler Squad Current State

The codebase already has `make benchmark-tier1` and `make benchmark-compare` with a baseline file pattern. The missing piece is the CI step that:
1. Runs benchmarks against `main` and saves as baseline
2. Runs against the PR branch
3. Compares with benchstat and fails on > 10% regression

---

## 4. Integration Test Gate Automation: Detecting "RPC Without Test"

### The Problem

When a developer adds a new RPC method to `proto/session/v1/session.proto`, the CI should detect if no corresponding test covers it.

### Approach 1: Feature Registry CI Check (Already Partially Implemented)

The codebase has `docs/registry/features/` with per-RPC JSON files. The CI already warns when registry files diverge from what the scanner would generate. This can be extended to fail (not just warn) when `"tested": false` on a new RPC.

```yaml
- name: Check new RPCs have tests
  run: |
    make registry-generate
    # Fail if any new file has tested: false
    new_files=$(git diff --name-only HEAD~1 -- 'docs/registry/features/backend/**')
    for f in $new_files; do
      if jq -e '.tested == false' "$f" > /dev/null; then
        echo "New RPC $f has no test coverage (tested: false)"
        exit 1
      fi
    done
```

### Approach 2: Proto Diff + Go Test Grep

```bash
# In CI: find new RPC method names in proto diff
new_rpcs=$(git diff HEAD~1 -- proto/ | grep '^+  rpc ' | awk '{print $3}')
for rpc in $new_rpcs; do
  if ! grep -r "Test.*${rpc}\|${rpc}.*Test" server/services/ session/ --include="*_test.go" -q; then
    echo "No test found for new RPC: $rpc"
    exit 1
  fi
done
```

### Approach 3: Connect RPC Test Coverage via Handler Annotations

The `// +api:` marker system already in the codebase is the cleanest hook. When a handler is added without a marker, the registry scanner marks it as `markerFound: false`. CI can fail on unmarked new handlers:

```yaml
- name: Verify all new handlers have +api markers
  run: |
    make registry-generate
    git diff --exit-code docs/registry/features/ || {
      echo "Uncommitted registry changes — run make registry-generate"
      exit 1
    }
    # Check for markerFound: false on recently modified files
    if find docs/registry/features/backend -newer docs/registry/features -name "*.json" \
       | xargs jq -e '.markerFound == false' 2>/dev/null | grep true; then
      echo "New handlers missing +api markers"
      exit 1
    fi
```
