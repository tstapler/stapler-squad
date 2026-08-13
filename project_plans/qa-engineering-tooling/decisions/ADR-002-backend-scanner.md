# ADR-002: Backend Scanner Approach

Status: Accepted
Date: 2026-04-17
Deciders: Solo developer (owner)

---

## Context

The backend feature scanner must discover all ConnectRPC service methods in the Stapler Squad Go codebase and produce entries in `docs/registry/backend-features.json`. The scanner must have a false positive rate below 5% and must exclude generated protobuf code (`.pb.go` files).

Stapler Squad's Go stack:
- ConnectRPC over HTTP/2 (not gRPC wire protocol)
- Single service: `SessionService` with 45+ RPC methods defined in `proto/session/v1/session.proto`
- Handlers implemented in `server/services/session_service.go` and sibling files
- Generated code in `gen/session/v1/` (`.pb.go` files)
- buf CLI already configured via `buf.yaml` and `buf.gen.yaml`

---

## Options Considered

### Option A: buf CLI + Proto Extraction Only

Run `buf export` or `buf build --as-file-descriptor-set` to extract all service and method definitions from proto files. Parse the file descriptor set to enumerate `(service, method)` pairs.

Pros:
- Zero false positives on generated code (proto files are not Go code)
- buf CLI is already configured in the repo (`buf.yaml`)
- No Go AST parsing required
- Captures all proto-defined methods even if handler implementation is incomplete

Cons:
- Cannot detect whether a handler is actually registered in the server at runtime
- Cannot capture ad-hoc HTTP routes (none exist currently, but this is a future risk)
- Does not validate that `// +api:` markers exist on handler code

Accuracy: ~85% of ground truth (proto-defined but may miss dead/unregistered handlers)

### Option B: Go Runtime Reflection

Instantiate the ConnectRPC service handlers, use reflection to introspect registered procedure names at runtime.

Pros:
- Captures only actually-registered handlers
- ~95% accuracy (runtime reality)

Cons:
- Requires a compilable, runnable server binary — adds build dependency to scanner
- Circular: scanner must import the server package; server package change breaks scanner
- Not suitable for CI use before the binary is built
- Rejected as over-complex for the problem size

### Option C: Go AST + Marker Comments Only

Parse all Go files; skip `*.pb.go`, `*_test.go`; find functions with `// +api: {feature-id}` comments; extract function names.

Pros:
- Captures developer-declared features explicitly
- Near-zero false positives (marker = explicit intent)
- Simple implementation using `go/ast` standard library

Cons:
- False negatives: any RPC method without a marker is invisible
- Requires developer discipline from day one
- Accuracy depends entirely on marker adoption; starts low on first scan

Accuracy: ~100% precision, coverage proportional to marker adoption (starts low)

### Option D: Dual-Scan (buf proto + Go AST markers) — Chosen

Run both the proto extraction (Option A) and Go AST marker scan (Option C) independently. Merge results: every proto RPC method gets a registry entry. For each entry, check whether a corresponding `// +api:` marker was found in Go handler files. Set `markerFound: true/false`.

Pros:
- Proto scan guarantees completeness (all 45+ RPC methods appear in registry)
- Marker scan validates handler coverage (developers can see which methods lack markers)
- False positive rate is bounded by proto scan accuracy (proto files don't lie)
- CI can warn on `markerFound: false` entries to drive marker adoption over time

Cons:
- Two independent scans to maintain
- `markerFound: false` is an advisory warning, not an error — markers are opt-in initially

Accuracy: ~98% (proto scan completeness + explicit marker validation)

---

## Decision

**Dual-scan: buf CLI proto extraction + Go AST `// +api:` marker scan.**

The proto extraction is the completeness guarantee. The marker scan is the coverage signal. Together they give a registry that is both complete (no proto method is missing) and informative (developer can see which handlers have been explicitly annotated).

---

## Implementation Details

**Proto extraction approach**: Use `github.com/bufbuild/protocompile` Go library (not buf CLI subprocess) to parse proto files in-process. This avoids subprocess execution in CI and allows unit testing without the buf binary. Alternative: if protocompile proves complex, fall back to subprocess `buf build --as-file-descriptor-set -o -` and parse the binary file descriptor.

**Go AST marker pattern**:
```go
// +api: session:create
func (s *SessionService) CreateSession(ctx context.Context, req *connect.Request[sessionv1.CreateSessionRequest]) (*connect.Response[sessionv1.CreateSessionResponse], error) {
```

Marker must appear in the comment immediately preceding the function declaration. Scanner uses `ast.CommentGroup` from the Go AST to find this.

**Excluded patterns**:
- Files matching `*.pb.go` (generated protobuf)
- Files matching `*.gen.go` (any generated code)
- Files matching `*_test.go` (test files)
- Directories matching `gen/`, `vendor/`, `.git/`

**Feature ID convention**: `{service-scope}:{action}` in snake_case
- `session:create`, `session:pause`, `session:resume`, `session:delete`
- `history:search`, `history:list`
- `workspace:switch`, `workspace:list-targets`
- `notification:send`, `notification:list`

---

## Consequences

- All 45+ RPC methods from `proto/session/v1/session.proto` will appear in the backend registry on first run
- Methods without markers will have `markerFound: false` — this is expected initially
- Marker adoption is measured by the `markerFound` rate; target 80% adoption within the first implementation sprint
- Developers add `// +api:` comments incrementally; no big-bang annotation sprint required
- Proto scan is deterministic; running the scanner twice produces identical output

---

## References

- `project_plans/qa-engineering-tooling/research/findings-architecture.md` — Options 2A through 2D
- `project_plans/qa-engineering-tooling/research/findings-stack.md` — Go AST option analysis
- `project_plans/qa-engineering-tooling/research/findings-pitfalls.md` — False positive analysis for Go scanners
- `proto/session/v1/session.proto` — Current RPC method inventory (45+ methods as of 2026-04-17)
