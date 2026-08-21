# Architecture Research: backlog-workflow-engine

## Codebase Context

### Current status model
`BacklogStatus` is a Go `type BacklogStatus string` in `session/backlog.go`. Constants are
`idea`, `ready`, `in_progress`, `review`, `done`, `archived`. Transition validity lives in
`validTransitions map[BacklogStatus]map[BacklogStatus]bool` and business guards live in
`TransitionGuard(item BacklogItemTransitionInput, to BacklogStatus) error`.

The `BacklogItem` ent schema stores `status` as a plain `field.String("status").Default("idea")` — no enum constraint, no DB check constraint.

The `TransitionBacklogItemStatus` RPC in `server/services/backlog_service.go` calls
`CanTransitionBacklog` then `TransitionGuard`, then `storage.TransitionBacklogItemStatus`. No
events are published on backlog status changes today (the event bus is session-scoped only).

`submit_triage_result` MCP tool (in `server/mcp/tools_backlog.go`) persists triage data on the
`ItemSession` and fires a push notification. It does NOT automatically transition item status.

---

## 1. WorkflowConfig Storage Layer

### Option A: Single entity with JSON `config` column

```go
// ent/schema/workflow_config.go
field.String("workspace_id").Default("default")
field.String("states_json").Comment("[]WorkflowStateSpec")
field.String("transitions_json").Comment("[]WorkflowTransitionSpec")
field.String("gates_json").Comment("map[transition_key][]GateSpec")
```

**Query performance**: Loading the entire config is a single row read. Gate evaluation loads one
row and deserializes JSON — acceptable for a config fetched once per request. Indexed queries
over states/transitions are impossible without scanning all rows, but this is single-workspace
so there is effectively only one config row.

**Schema evolution**: Adding a new gate type requires only a JSON schema change — no DB migration.
Removing a field is backward-compatible if callers use `omitempty`. Adding a required field
requires backfill logic.

**Backward compatibility**: Trivially compatible — no existing data uses this table.

**Ent complexity**: One schema file. Config is loaded as a struct, validated in Go.
The downside is that ent cannot enforce referential integrity between states and transitions at
the DB layer (a transition can reference a state key that doesn't exist).

**Verdict**: Best fit for S1/S2 scope. Single-workspace, config is read-heavy (evaluated on
every transition), rarely written. JSON avoids a 3-table join on every status change evaluation.

---

### Option B: Normalized entities (`WorkflowState`, `WorkflowTransition`, `WorkflowGate`)

```go
// Three ent schemas linked by edges
WorkflowState      — id, key, label, is_terminal, config_id
WorkflowTransition — id, from_state_id, to_state_id, config_id
WorkflowGate       — id, transition_id, type, params_json, blocking, enabled
```

**Query performance**: Loading the full graph for evaluation requires three queries (or a JOIN)
on every `TransitionBacklogItemStatus` call. Ent eager-loading (`.WithEdges(...)`) generates
JOIN queries — acceptable but heavier than a JSON blob read.

**Schema evolution**: Adding a new gate type requires adding a new `type` enum value or a new
`params_json` sub-schema. Adding a first-class field to `WorkflowGate` requires a migration.

**Backward compatibility**: No existing data, so migration cost is zero at adoption time.
Future gate-type evolution still requires migrations for new params columns.

**Ent complexity**: Three schema files, three edge definitions, careful foreign-key management.
ent enforces referential integrity for state references. `WorkflowGate` references `WorkflowTransition`
via edge — ent's cascaded deletes handle cleanup.

**Verdict**: Overkill for S1/S2. Justified only if S3 (Custom state CRUD) or S5 (many gate types)
are committed. A normalized schema makes the CRUD API and visual builder (S4) cleaner because
each entity can be independently updated.

---

### Option C: Hybrid — normalized states/transitions, JSON gates per transition

```go
// WorkflowState and WorkflowTransition as ent entities
// Gates stored as JSON on WorkflowTransition
field.String("gates_json").Optional().Comment("[]GateSpec")
```

**Query performance**: Loading states + transitions is two queries; gate evaluation reads from
transition rows (no third table join). Slightly worse than pure JSON for the common read path,
but cleaner for the builder UI.

**Schema evolution**: New gate types require no migration. Transition structure changes (adding a
label, a description) do require migrations.

**Backward compatibility**: Same as Option B — no existing data.

**Ent complexity**: Two schema files. Lower than full normalization.

**Verdict**: The best pragmatic choice if S3/S4 are planned. It gives relational integrity where
it matters most (state keys are FK-enforced) while keeping gate evolution cheap (JSON).

---

### Recommendation

**For S1 (just `refining` status): use Option A (JSON blob)** — the default workflow can be
seeded as a hardcoded JSON config and the feature ships without new ent schemas.

**For S2+ (WorkflowConfig API): use Option C (hybrid)** — normalized states/transitions let the
CRUD API and settings UI work cleanly against ent, and JSON gates defer schema migrations for
the many gate types in S5.

Implement S1 first with a `defaultWorkflowConfig()` Go func that returns the hardcoded graph.
When S2 lands, migrate by persisting that default as a seeded DB row.

---

## 2. Proto Schema Design

### `WorkflowConfig` message

```protobuf
// WorkflowState describes a single node in the workflow graph.
message WorkflowState {
  string key         = 1; // machine key: "idea", "refining", "ready", ...
  string label       = 2; // human label: "Idea", "Refining", ...
  bool   is_terminal = 3; // no outbound transitions when true (done, archived)
  string color       = 4; // optional hex color for UI (e.g. "#6366f1")
}

// GateSpec defines a single gate on a transition.
message GateSpec {
  string type      = 1; // "field", "triage", "approval", "command", "ci"
  bool   blocking  = 2; // blocks transition when false; warning only when false
  bool   enabled   = 3;
  string params    = 4; // JSON-encoded gate-type-specific params
}

// WorkflowTransition describes an allowed edge in the graph.
message WorkflowTransition {
  string           from_state = 1; // state key
  string           to_state   = 2; // state key
  repeated GateSpec gates     = 3;
}

// WorkflowConfig is the complete workflow definition for a workspace.
message WorkflowConfig {
  string                    id          = 1;
  string                    name        = 2; // display name
  repeated WorkflowState    states      = 3;
  repeated WorkflowTransition transitions = 4;
  google.protobuf.Timestamp updated_at  = 5;
}
```

### Gate params shapes (encoded in `GateSpec.params` as JSON)

| type       | params shape |
|------------|--------------|
| `field`    | `{"field":"acceptance_criteria","op":"non_empty"}` |
| `triage`   | `{"require_no_open_questions":true}` |
| `approval` | `{"prompt":"Approve plan?"}` |
| `command`  | `{"cmd":"make ci","timeout_sec":120,"working_dir":"item_repo_path"}` |
| `ci`       | `{"pr_url_field":"pr_url","required_checks":["ci/test"]}` |

### RPCs needed

```protobuf
// WorkflowService (new service, or extend BacklogService)

// GetWorkflowConfig returns the active workflow for the workspace.
rpc GetWorkflowConfig(GetWorkflowConfigRequest) returns (GetWorkflowConfigResponse) {}

// UpdateWorkflowConfig replaces the workflow definition.
rpc UpdateWorkflowConfig(UpdateWorkflowConfigRequest) returns (UpdateWorkflowConfigResponse) {}

// EvaluateGates checks all gates on a transition without executing it.
// Used by the UI for real-time feedback before the user submits.
rpc EvaluateGates(EvaluateGatesRequest) returns (EvaluateGatesResponse) {}
```

`TransitionBacklogItemStatus` already exists and should remain unchanged for backward compat.
It will internally call `EvaluateGatesForTransition` (server-side).

---

## 3. Transition Validation Architecture

### Decision: Client evaluates for UX, server validates authoritatively

**Client-side only**: Bypassable via direct RPC calls (MCP tool, any HTTP client). Unacceptable
for blocking gates (plan required, AC required).

**Server-side only**: The existing `TransitionGuard` pattern is already server-only and works
well. The problem is latency: a user who clicks "Move to in_progress" with no plan approved
sees an error only after the round-trip. For complex command gates this is unavoidable, but
for field gates (AC required, plan approved) the client can evaluate immediately.

**Dual validation (recommended)**:
- `EvaluateGates` RPC: evaluates all non-command, non-CI gates synchronously and returns a
  `[]GateResult{gate_type, passed, reason}`. The UI calls this to render ✓/✗ badges on
  transition buttons before the user clicks.
- `TransitionBacklogItemStatus`: server always re-evaluates all gates; command/CI gates run
  here (not in `EvaluateGates`). Returns `CodeFailedPrecondition` if any blocking gate fails.

### Command gate safety

The server has access to `item.repo_path` — this is the worktree path for items that have been
spawned. Command gate execution:

1. Validate `cmd` is not empty and does not contain shell metacharacters (`$(`, `` ` ``, `&&`, etc.) — use a simple allowlist or subprocess (`exec.Command("sh", "-c", cmd)` with a dedicated sandbox user).
2. Run in `item.repo_path` with a configurable timeout (default 60s).
3. Gate passes if exit code is 0.
4. Gate evaluation is async for CI gates (poll GitHub PR check status); sync for command gates.
5. Command gates are **not** evaluated in `EvaluateGates` RPC — only in `TransitionBacklogItemStatus`. The UI shows a "will run on server" badge.

Key risk: if `item.repo_path` is empty (idea-stage items), command gates must be skipped or
error. Guard: require `repo_path` non-empty before evaluating command gates.

---

## 4. Backward Compatibility Architecture

### Current state

`BacklogItem.status` is `field.String("status").Default("idea")` in ent — already a plain
string, no DB enum constraint. The proto `BacklogItem.status` field is `string status = 6` in
`backlog.proto`. There is no proto enum.

**This is the critical finding**: the codebase already uses plain strings for status in both
the DB schema and the proto. The "hardcoded" constraint lives only in Go constants and the
`validTransitions` map — not in the wire format or the DB schema.

### Migration strategy: no breaking changes

**Keep `BacklogItem.status` as `string` in proto** — no field renaming, no field addition.
WorkflowConfig defines which string values are valid states. The server validates `target_status`
against the active WorkflowConfig's state keys in `TransitionBacklogItemStatus`.

**Extend `TransitionBacklogItemStatus` guard logic**:
1. Load active `WorkflowConfig`.
2. Replace `CanTransitionBacklog(from, to)` lookup with `WorkflowConfig.IsValidTransition(from, to)`.
3. Replace `TransitionGuard` sentinel errors with `WorkflowConfig.EvaluateGates(item, from, to)`.
4. The default workflow config seeds the existing transitions — existing items continue to work.

**Default workflow config** seeds these states and transitions on first startup (or as a
migration):
```
idea → ready (gate: field:acceptance_criteria non_empty)
ready → in_progress (gate: field:plan_approved, field:skip_planning OR plan_approved)
in_progress → review
review → done (gate: field:review_verdict PASS OR override_reason non_empty)
done → archived
idea → archived
ready → idea
done → review
archived → idea
```

**Adding `refining` state (S1)**: Add `refining` as a new key in the default config. Items
currently in `idea` can transition to `refining`; from `refining` they can go to `ready` or
back to `idea`. No existing items are in `refining`, so no migration is needed.

**Items created before WorkflowConfig**: `status = "idea"` remains valid because `idea` is a
key in the default config. No migration SQL needed.

---

## 5. `refining` State Integration

### Where does triage set `refining`?

**Option A — `submit_triage_result` auto-transitions**: When `clarifying_questions` is non-empty,
the tool calls `TransitionBacklogItemStatus(item_id, "refining")` internally.

**Option B — Triage calls `TransitionBacklogItemStatus` via MCP tool directly**.

**Recommendation: Option A (automatic)**

The `submit_triage_result` MCP tool already has direct access to `storage` and the item ID.
Auto-transitioning to `refining` when questions are present is the semantically correct behavior:
the triage agent is declaring "this item is not ready to progress until these questions are
answered." Forcing the agent to make a second MCP call adds complexity with no benefit.

Implementation:
```go
// In submit_triage_result handler, after persisting triage result:
if len(clarifyingQuestions) > 0 {
    currentItem, _ := h.storage.GetBacklogItem(ctx, itemID)
    if currentItem.Status == string(session.BacklogStatusIdea) {
        _, _ = h.storage.TransitionBacklogItemStatus(ctx, itemID,
            session.BacklogStatusRefining, nil)
    }
}
```

The human resolves questions via the UI, then manually transitions `refining → ready` once
satisfied. If the workflow engine is active, the `ready` gate can still require AC to be present.

**Gate for `refining → ready`**: A `triage` gate type that checks
`len(open_clarifying_questions) == 0`. This can be enforced as a warning gate initially
(S1) and upgraded to blocking in S2 when WorkflowConfig is configurable.

---

## 6. Event Bus Integration

### Current state

The event bus (`pkg/events`) is session-scoped. Events carry `*session.Instance` and `session.Status`
(the tmux session status, not the backlog status). No backlog lifecycle events exist today.

### Should workflow transitions publish events?

**Yes.** The event bus is the established internal notification mechanism. The
`StartDeliverySubscriber` pattern (fan-out to push notifiers) is already in use.

### New event type: `EventBacklogItemTransitioned`

```go
// pkg/events/types.go — add:
EventBacklogItemTransitioned EventType = "backlog.item.transitioned"

// Event struct — add fields:
BacklogItemID     string
BacklogFromStatus string
BacklogToStatus   string
```

### Consumers that need this event

| Consumer | Why |
|----------|-----|
| **Push notifier** | Notify user when item moves to `review` (needs human oversight) or `done` |
| **Review queue manager** | Today, `review_queue_manager.go` polls. Transitions could trigger immediate queue refresh |
| **Frontend SSE stream** | The React UI needs to know when an item changes status to re-render the board without polling |
| **CI gate poller** (S5) | When item enters a state with a CI gate, start polling the GitHub PR check |
| **MCP context refresh** | If a triage session is open for an item that just moved to `done`, the agent should be notified |

### Publishing point

Publish `EventBacklogItemTransitioned` inside `storage.TransitionBacklogItemStatus` after the DB
write commits, or in `backlog_service.TransitionBacklogItemStatus` after `storage.Transition...`
returns successfully. The service layer is preferable because it already has access to the event
bus (currently passed to MCP handlers). Inject `*events.EventBus` into `BacklogService` as a
new optional field (same degradation contract as `sessionCreator`).

---

## Summary

1. **Storage**: Use JSON blob (`Option A`) for S1 to ship `refining` fast; migrate to hybrid
   normalized (Option C) when S3/S4 demand a CRUD API for custom states. The ent schema already
   uses a plain `string` field for status — no DB migration needed to add new state values.

2. **Proto / backward compat**: `status` is already a `string` in both ent and proto — the
   "hardcoded" constraint exists only in the `validTransitions` Go map. Replace that map with a
   loaded `WorkflowConfig` at the service layer; existing items continue to work unchanged.

3. **Transition validation**: dual-layer — `EvaluateGates` RPC for client-side UX feedback
   (field and triage gates only), server-side re-evaluation in `TransitionBacklogItemStatus`
   for all gate types including command gates. Command gates run in `item.repo_path` with a
   timeout; they are never evaluated client-side.

4. **`refining` integration**: `submit_triage_result` auto-transitions to `refining` when
   `clarifying_questions` is non-empty. No second MCP call needed.

5. **Event bus**: Add `EventBacklogItemTransitioned` to `pkg/events`. Inject `*events.EventBus`
   into `BacklogService` (optional, same degradation pattern). Consumers: push notifier, review
   queue manager, frontend SSE stream, future CI gate poller.
