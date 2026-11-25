# Plan: epic-integration-child

**Date**: 2026-11-24
**Status**: planning

## Phased Implementation

### Phase 1 — Schema and Data Model

**Task 1.1** — Add `is_integration_child` bool field to `BacklogItem` ent schema (`session/ent/schema/backlog_item.go`). Default `false`. Run `make ent-gen`.

**Task 1.2** — Add `IntegrationChildID` optional nillable UUID field to `BacklogItem` schema (on the _epic_ item), pointing to its integration child. This allows the epic's gate check and UI to find the integration child in O(1) without scanning dependencies. Alternative: query by `is_integration_child=true` + dependency edges; use the field for performance.

**Task 1.3** — Add `EpicGate` validator: `WorkflowEngine.ValidateGates` reads `IntegrationChildID` on the epic; if set and integration child is not `done`/`archived`, block the `done` transition with `ErrIntegrationCheckPending`.

**Wire in `server/dependencies_backlog.go`** after schema generation.

---

### Phase 2 — Integration Child Creation Service

**Task 2.1** — New file `session/backlogstore/integration_child_writer.go`:
```go
// CreateIntegrationChild creates the integration child for an epic item,
// adding dependencies on all sibling IDs, and sets IntegrationChildID on the epic.
func CreateIntegrationChild(ctx, epicID, siblingIDs []string, e2eCommand string) (*domain.BacklogItemData, error)
```
Idempotency: return early if epic already has `IntegrationChildID` set.

**Task 2.2** — New service hook `server/services/backlog_service_epic.go` — `OnEpicPlanningComplete(ctx, epicID)`:
1. Fetch epic and its children (requires Epic items feature's parent-child edge).
2. Extract e2e check command: scan ACs for `e2e:` prefix, else fall back to PipelineMode `done_command_template`.
3. If no command found, log warning and return (feature degrades gracefully — epic can still close manually).
4. Call `CreateIntegrationChild`.
5. Set `auto_spawn_session=true` on the integration child so it queues automatically.

---

### Phase 3 — Failure Attribution and Fix-Child Creation

**Task 3.1** — New file `session/backlogstore/integration_attribution.go`:
```go
// AttributeIntegrationFailure scores sibling items by file-path overlap with
// the failing integration session's diff. Returns the best-match sibling ID
// and a confidence score, or empty string if below threshold (0.6).
func AttributeIntegrationFailure(ctx, epicID, integrationSessionID string) (siblingID string, confidence float64, err error)
```

**Task 3.2** — `OnIntegrationChildFailed(ctx, integrationChildID)`:
1. Call `AttributeIntegrationFailure`.
2. If confidence ≥ 0.6: create fix child with dependency on attributed sibling and new dependency from integration child on fix child.
3. If confidence < 0.6: create `GuidanceRequest` on epic item with failure details.

---

### Phase 4 — UI

**Task 4.1** — Add `is_integration_child` and integration child status to the backlog list row component (`web-app/src/components/backlog/BacklogItemRow.tsx`): a small "Integration" badge on the chip.

**Task 4.2** — Add integration check status section to the epic item detail view (`web-app/src/components/backlog/BacklogItemDetail.tsx`): shows "Awaiting Integration Check" or "Integration Passed/Failed" with a link to the integration child.

---

### Phase 5 — Tests

**Task 5.1** — Unit tests in `session/backlogstore/integration_child_writer_test.go`:
- Happy path: integration child created with correct dependencies
- Idempotency: second call returns existing child, no new row
- Missing e2e command: no child created, no error

**Task 5.2** — Unit tests in `session/backlogstore/integration_attribution_test.go`:
- Attribution above threshold: fix child created for correct sibling
- Attribution below threshold: guidance request created on epic

**Task 5.3** — E2E spec `tests/e2e/epic-integration-child.spec.ts`:
1. Create epic with e2e command in ACs
2. Complete all sibling children
3. Assert integration child auto-created and gating epic
4. Simulate integration session pass
5. Assert epic transitions to `done`

---

## Key Files

| File | Change |
|---|---|
| `session/ent/schema/backlog_item.go` | Add `is_integration_child`, `integration_child_id` fields |
| `session/workflow_engine.go` | Add `EpicGate` in `ValidateGates` |
| `session/backlogstore/integration_child_writer.go` | New: child creation + idempotency |
| `session/backlogstore/integration_attribution.go` | New: failure attribution heuristic |
| `server/services/backlog_service_epic.go` | New: `OnEpicPlanningComplete`, `OnIntegrationChildFailed` hooks |
| `server/dependencies_backlog.go` | Wire new service |
| `proto/session/v1/backlog.proto` | Add `is_integration_child` field to `BacklogItem` message |
| `web-app/src/components/backlog/BacklogItemRow.tsx` | Integration badge |
| `web-app/src/components/backlog/BacklogItemDetail.tsx` | Integration check status section |
| `tests/e2e/epic-integration-child.spec.ts` | New e2e spec |

## Open Questions

1. **Trigger point**: The `OnEpicPlanningComplete` hook needs to fire after epic planning creates all children. The prerequisite "Epic items" feature must define an observable event for this — confirm the hook point exists before Phase 2.
2. **Fix-child dependency chain**: When a fix child is created and succeeds, should the integration child automatically retry? Or require a manual re-queue? Recommend: auto-retry via `next_workflow_id` chain on fix child `done`.
3. **Multiple failure rounds**: Cap fix-child creation attempts at `rework_cap_override` to prevent unbounded escalation chains.
