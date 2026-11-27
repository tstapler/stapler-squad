# Research: Architecture — epic-integration-child

**Date**: 2026-11-24

## Existing Building Blocks

### 1. Dependency DAG (`BacklogItemDependency`)

`session/ent/schema/backlog_item_dependency.go` defines a `(blocker_id, blocked_id)` join table with:
- BFS cycle detection on insert (`session/backlogstore/dependency_writers.go`)
- Batch unresolved-blocker query used by the dequeue gate (`UnresolvedBlockerItemIDs`)
- Resolved = `done` OR `archived`
- `StuckReasonBlockedByDependency` already exists in the domain

The integration child can register each sibling as a dependency via `AddBacklogItemDependency` — no schema change needed.

### 2. `BacklogItem` Schema Fields (relevant to this feature)

From `session/ent/schema/backlog_item.go`:
- `acceptance_criteria` (JSON `[]AcCriterion`) — where the epic's e2e check command lives
- `pipeline_mode` (string slug) — links to PipelineMode whose `done_command_template` is an alternative source for the check command
- `next_workflow_id` (Optional UUID) — sequential chaining; could carry the integration child creation trigger, but not suitable (fires arbitrary workflow, not backlog item creation)
- `auto_spawn_session`, `skip_review_gate`, `skip_planning` — behavioral flags available on the integration child

No `is_integration_child`, `parent_id`, or `item_type` fields exist today. These require a new field or ent schema extension.

### 3. WorkflowEngine Gates

`session/backlog.go` → `session/workflow_engine.go`:
- `ValidateGates` checks ACs, review verdict, etc. before `done` transition
- Currently has no "wait for integration child" check
- Adding a gate: check if the epic has an integration child and it is `done`

### 4. Chain Firer (`session/backlog/chains/firer.go`)

Fires a configured `Workflow` when a `BacklogItem` reaches `done`. Not designed for item creation; would require significant adaptation. Not the right hook for integration child creation.

### 5. Guidance Request (Escalation)

`session/ent/schema/guidance_request.go` — the existing escalation path for operator review. Used when attribution fails. The guidance request is linked to the epic item, not the integration child.

---

## Gap Analysis

| Need | Gap | Resolution |
|---|---|---|
| Mark an item as "integration child" | No `is_integration_child` field | Add bool field to `BacklogItem` ent schema |
| Trigger integration child creation after all siblings created | No "all children created" hook | New service-layer hook after epic planning completes |
| Source e2e check command | No dedicated field; lives in AC text or `done_command_template` | Extract from AC with `e2e:` prefix OR use pipeline mode `done_command_template` |
| Gate epic's `done` transition | `ValidateGates` doesn't check integration child | Add gate: `IntegrationChildRequired` → check if integration child is `done` |
| Attribute failure to a sibling slice | No attribution logic exists | New service: compare integration session diff against sibling diffs to find overlap |
| Create fix child and link to slice | Exists via `CreateBacklogItem` + `AddBacklogItemDependency` | Compose existing primitives |

---

## Proposed Architecture

### New Schema Field

```go
// session/ent/schema/backlog_item.go
field.Bool("is_integration_child").Default(false).Optional()
```

This avoids adding a foreign key to "parent epic" (which belongs in the prerequisite "Epic items" feature). The integration child points to the epic indirectly via the dependency edges.

### Integration Child Lifecycle

```
EpicPlanningComplete
  → createIntegrationChild(epicID, siblingIDs, e2eCommand)
       → CreateBacklogItem(title="Integration Check: <epic title>", is_integration_child=true)
       → for each sibling: AddBacklogItemDependency(blocker=sibling, blocked=integrationChild)
       → item dequeues only after all siblings done (existing gate)

IntegrationChildSession
  → runs e2e check command
  → review gate: pass → epic transitions to done
                 fail → AttributeFailure(diff context) → create fix child OR GuidanceRequest
```

### Attribution Heuristic

1. Extract changed files from the integration session's diff.
2. For each sibling item, fetch its item sessions' commit diffs.
3. Score overlap by file paths.
4. If one sibling accounts for >60% of relevant changed files → attribute to that sibling.
5. Otherwise → escalate via GuidanceRequest.

This is a heuristic, not a guarantee — the GuidanceRequest escalation path is the safety net.

---

## Risks

- **Epic items feature not shipped**: This feature depends on epic item planning triggering child creation. Without a "parent epic" link on children, the integration child cannot reliably enumerate its siblings. **Mitigation**: scope the trigger to pipeline-mode epics only (a tagged item with `item_type=epic` from the prerequisite feature).
- **Attribution false positives**: Blaming the wrong sibling creates misdirected fix children. **Mitigation**: lower confidence threshold → escalate rather than auto-create fix child when below threshold.
- **Integration child blocks epic forever if e2e is flaky**: **Mitigation**: operator can manually archive the integration child (counts as resolved in the dependency gate) or waive the AC.
