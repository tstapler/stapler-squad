# Stack Research: Backlog Workflow Engine

## 1. Graph Storage in SQLite via ent

### Decision: JSON Column on a Single Entity (WorkflowConfig)

**Recommendation: Store the workflow graph as a single ent entity with two JSON columns — one for the node list (states + gates) and one for the edge list (transitions).**

Rationale:
- The graph is read as a unit on nearly every status-validation call. A single DB row gives O(1) retrieval with no joins.
- SQLite has excellent JSON support since 3.38 (json_each, json_extract). ent's `field.JSON` maps directly to a TEXT column storing marshalled Go structs, which matches the existing pattern in the codebase. `backlog_item.acceptance_criteria` already uses `field.String(...).Comment("JSON []AcCriterion")` — the workflow config can follow exactly this pattern.
- The graph topology is user-managed (small, bounded), not a data-growth table. The single-entity approach avoids ent relationship complexity (edges, back-refs, upserts) for no query benefit.
- Normalized tables (WorkflowState, WorkflowTransition, WorkflowGate) would require 3 schema files, 3 migration files, and complex multi-table joins every time the state machine is loaded. The added flexibility is not warranted for a graph that never has more than ~20 nodes.

**ent Schema Sketch:**
```go
// session/ent/schema/workflow_config.go
field.String("workflow_id").Unique().NotEmpty(),        // e.g. "default"
field.String("states_json").Optional(),                 // JSON []WorkflowState
field.String("transitions_json").Optional(),            // JSON []WorkflowTransition
field.Bool("is_default").Default(false),
field.Time("created_at").Default(time.Now).Immutable(),
field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
```

The existing `approvalrule.go` schema is a close reference — flat fields, no edges, simple indexes.

**In-memory representation at runtime:**
Load the JSON columns once at startup (or on first use), deserialize into Go structs, and build an adjacency map for O(1) `CanTransition` lookups — exactly matching the current `validTransitions` map in `session/backlog.go`.

### ent-Specific Patterns

- Use `field.JSON("states", []WorkflowState{})` instead of `field.String` + manual marshal/unmarshal. ent v0.14 (this project uses 0.14.5) supports `field.JSON` on SQLite backed by a TEXT column.
- Use `--feature sql/upsert` flag (already required by the project's `generate.go`) to support upsert on the `workflow_id` unique key.
- Index `workflow_id` and `is_default` only — no other indexes needed.

---

## 2. Proto Design for WorkflowConfig

### Recommendation: Nested messages, DAG encoded as edge list

```protobuf
message WorkflowState {
  string id = 1;                  // stable slug, e.g. "refining"
  string display_name = 2;
  bool is_terminal = 3;
  repeated string gate_ids = 4;   // gates that must pass before entering this state
}

message WorkflowTransition {
  string from_state_id = 1;
  string to_state_id = 2;
  string label = 3;               // optional human label, e.g. "Approve"
}

message WorkflowGate {
  string id = 1;
  string type = 2;                // "field", "triage", "approval", "command", "ci"
  string config_json = 3;         // gate-type-specific config (opaque blob)
  bool blocking = 4;              // true = hard block; false = warning only
  bool enabled = 5;
}

message WorkflowConfig {
  string id = 1;
  bool is_default = 2;
  repeated WorkflowState states = 3;
  repeated WorkflowTransition transitions = 4;
  repeated WorkflowGate gates = 5;
  google.protobuf.Timestamp created_at = 6;
  google.protobuf.Timestamp updated_at = 7;
}
```

**Key decisions:**
- Edge list (not adjacency list in the proto) keeps the proto flat and easy to add/remove edges individually via CRUD APIs.
- Gates are a separate top-level list referenced by ID from `WorkflowState.gate_ids`. This allows gates to be reused across states (an approval gate shared by `review` and `done`).
- `config_json` on WorkflowGate is an opaque blob per gate type. This avoids adding a proto oneof for every gate variant upfront while keeping extensibility. The gate evaluator unmarshals it in Go.
- Avoid proto `oneof` for DAG encoding — DAGs map cleanly to repeated edge messages; oneof adds type-switch boilerplate without benefit here.

**Wire compatibility note:** `BacklogItem.status` is already a `string` field (field 6), not an enum. This is what enables backward compatibility — see section 3.

---

## 3. Backward Compatibility Approach

### Current State

`BacklogItem.status` in both the proto and the ent schema is a plain `string`, not a protobuf enum. The hardcoded constants live in `session/backlog.go` as Go `const` values, not as proto enum values. The `validTransitions` map and `TransitionGuard` function in `backlog.go` are the only places that enforce topology.

### Recommended Approach: Layered Resolver

Because `status` is already a string, no proto change is needed for backward compatibility. The approach is:

1. **Built-in default workflow.** On first use, if no WorkflowConfig exists in the DB, the system materializes the hardcoded 6-state graph into a `WorkflowConfig` row with `is_default=true` and `workflow_id="default"`. Existing items' `status` strings match the state `id` values exactly — no migration required.

2. **Runtime resolver.** Replace the hardcoded `validTransitions` map and `TransitionGuard` function with a `WorkflowResolver` that:
   - Loads the active WorkflowConfig (cached; invalidated on config updates)
   - Evaluates `CanTransition(from, to)` against the loaded graph
   - Evaluates gates for a given transition

3. **Sentinel status handling.** The hardcoded sentinel checks in `backlog_service.go` (e.g. "item must be in ready status to spawn a session") are business logic, not graph topology. They can remain as named constants keyed on the `id` field of the built-in states. Custom states will not trigger these sentinels unless explicitly configured.

4. **No migration needed.** Since DB rows store status as freeform strings and the default workflow uses the same IDs as the current constants (`"idea"`, `"ready"`, etc.), existing data is valid in the new system with zero schema changes.

**The `refining` state (S1) is purely additive.** Add it to the default WorkflowConfig's state list and add transitions `idea→refining` and `refining→ready`. No existing items are in `refining`, so no rows are affected.

---

## 4. Gate Evaluation: Command Gates and CI Gates

### Command Gates (shell exit 0)

**Pattern: sandbox execution via `os/exec` with hard timeout, isolated working directory, and no shell expansion.**

The project already uses `os/exec` extensively (tmux management, git operations via `github.com/go-git/go-git/v5`). The existing subprocess infrastructure in `session/tmux.go` and related files is the reference.

**Recommended approach:**
```go
ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
defer cancel()
cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
cmd.Dir = workingDir     // item's repo_path
cmd.Env = safeEnv()      // stripped env: PATH, HOME, GOPATH only
out, err := cmd.CombinedOutput()
```

**Security constraints:**
- Parse command as a fixed argv slice (never pass to `sh -c`). The `mvdan.cc/sh/v3` package (already in go.mod) can parse shell syntax into an argv slice safely without spawning a shell.
- Strip environment variables to a minimal safe set.
- Apply `context.WithTimeout` with a configurable max (default 30s).
- Gate commands should run in the item's `repo_path` only — prevent directory traversal by resolving the path through `filepath.Abs` and validating it is within the configured workspace root.
- Log command, exit code, stdout/stderr to the gate evaluation record.
- Do NOT run as a persistent goroutine — evaluate synchronously and return the result. Gate evaluation is on the transition hot path but commands are user-configured; keep it simple.

No new Go library is needed. `os/exec` + `mvdan.cc/sh/v3` (already a dependency) cover this.

### CI Gates (GitHub PR checks)

**Pattern: GitHub REST API via `net/http` + lightweight wrapper, no SDK.**

The project has no GitHub API client dependency (`google/go-github` is absent from go.mod). Given the narrow use case — polling check-run status for a PR — a lightweight approach is appropriate:

**Option A (recommended): stdlib `net/http` with a thin wrapper**
```go
// GET /repos/{owner}/{repo}/commits/{ref}/check-runs
// Authorization: token <GITHUB_TOKEN>
```
One function, one response struct, one HTTP call. The GitHub REST API for check runs is stable, well-documented, and requires no SDK to use.

**Option B: Add `google/go-github`**
Justified if the feature scope expands to PR creation, comment posting, etc. For gate evaluation (read-only check-run status), it's more dependency than needed.

**Token storage:** The project already has AES-GCM token encryption infrastructure (`session.EncryptToken`, `config.GetOrCreateEncryptionKey`). CI gate config should store the GitHub token via the same mechanism used by `ItemSource` (see `encryptAndMergeToken` in `backlog_service.go`).

**Polling vs webhook:** Start with polling (check on each gate evaluation call). Webhooks add infrastructure complexity; polling is sufficient for gate evaluation which only runs on user-triggered transitions.

**Security:** GitHub tokens scoped to `repo:status` (read-only checks) are sufficient. Never log token values.

---

## 5. Visual Graph Editor: React Library Recommendation

### Recommendation: React Flow (`@xyflow/react`)

**React Flow** (package `@xyflow/react`, formerly `reactflow`) is the clear best fit for this use case.

| Library | SSR / Next.js 15 | vanilla-extract | Bundle size | Drag-and-drop | Verdict |
|---|---|---|---|---|---|
| **React Flow** (`@xyflow/react`) | Yes (dynamic import with `ssr:false`) | Compatible — uses CSS variables which can be overridden | ~180 KB min+gz | First-class | **Recommended** |
| D3 | Yes | Compatible | Modular, ~100 KB for layout modules | DIY | Too low-level; 10x more code for a graph editor |
| dagre | No (layout only, not a UI library) | N/A | Very small | N/A | Not a UI library; use as layout engine with React Flow |
| `@dagrejs/dagre` | N/A | N/A | ~80 KB | N/A | Pair with React Flow for auto-layout |
| Cytoscape.js | Requires dynamic import | External CSS conflicts with VE | ~420 KB | Via plugin | Too large; jQuery-era API feel |

**Integration notes:**
- Import with `dynamic(() => import('@xyflow/react'), { ssr: false })` in the Next.js page component. The graph editor is inherently client-side (drag-and-drop); SSR is not needed.
- React Flow renders its own CSS via `import '@xyflow/react/dist/style.css'`. Scope it to the graph editor container so it does not bleed into the global styles. vanilla-extract styles are statically applied to custom node components; React Flow's internal edge/canvas CSS is isolated.
- Use `@dagrejs/dagre` (or React Flow's built-in `elk` layout option) to auto-position nodes on first load. This keeps the stored data as a pure node/edge list without requiring x/y coordinates in the WorkflowConfig proto.
- Custom node and edge components should use vanilla-extract `.css.ts` files following the project's ADR-009 pattern.
- React Flow v12 (`@xyflow/react`) is React 19 compatible (the project uses React 19).

**Bundle size mitigation:** Tree-shake by importing only the components used (`ReactFlow`, `Handle`, `BaseEdge`). Lazy-load the graph editor route so the ~180 KB is not included in the main bundle. The project already enforces a 5 MB total JS bundle limit via `size-limit`.

**No drag-and-drop library is needed separately.** React Flow includes it. The project's existing `react-arborist` is for the file tree; it is not relevant to graph editing.
