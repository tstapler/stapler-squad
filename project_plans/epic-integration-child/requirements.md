# Requirements: epic-integration-child

**Date**: 2026-11-24
**Type**: feature
**Complexity**: 3 — builds on the existing dependency system; requires new DB field, service-layer logic, and pipeline-mode integration

## Problem Statement

When an epic is decomposed into child backlog items, each child can reach `done` independently (each passes its own unit-level review gate) while the _feature as a whole_ does not work. Anthropic's harness research confirmed that agents mark features done on unit-level checks until tested end-to-end. Today nothing in stapler-squad prevents this: the epic can be closed as `done` as soon as every explicit child transitions, with no requirement for an end-to-end check.

## Goals

1. When an epic is planned and its child items are created, automatically create a final "integration child" item that depends on every other child.
2. The integration child runs an end-to-end check: either a named e2e spec from the epic's definition of done or a configured command.
3. The epic closes (transitions to `done`) only when the integration child passes.
4. On integration failure, automatically create a targeted fix child attributed to the responsible slice (or escalate to the operator if attribution is ambiguous).

## Non-Goals

- Changing the existing dependency DAG structure (we reuse `BacklogItemDependency`).
- Adding a generalized "epic" item type (that is a separate prerequisite feature).
- Requiring a custom PipelineMode for items that don't use it.
- Supporting multiple integration children per epic (exactly one per epic, auto-created).

## Acceptance Criteria

### AC-1 — Integration child auto-creation
When an epic's planning phase produces N child items, the system automatically creates one additional integration child. It is created after all siblings exist, depends on every sibling, and cannot be manually queued until all siblings are `done` or `archived`.

### AC-2 — Integration check sourced from epic's definition of done
The integration child's initial prompt includes an end-to-end check command derived from the epic item's acceptance criteria (a named e2e spec or the `done_command_template` from the epic's pipeline mode). The command is validated to be non-empty before the integration child is created.

### AC-3 — Epic closes only after integration child passes
The epic item (parent) transitions to `done` only when the integration child reaches `done`. The standard `WorkflowEngine.ValidateGates` check for the epic enforces this: a pending or failed integration child blocks the epic's `done` transition.

### AC-4 — Attribution and fix-child creation on failure
When the integration child session ends with a failing check, the system:
(a) attempts to attribute the failure to a specific sibling item using the diff context from the integration session;
(b) if attribution succeeds, creates a targeted fix child linked to the attributed slice item and re-depends on it;
(c) if attribution fails or is ambiguous, escalates to the operator via a `GuidanceRequest` on the epic item.

### AC-5 — Idempotent creation
If the epic already has an integration child (by looking for the `is_integration_child` marker), no second integration child is created on re-plan or retry.

### AC-6 — Visible in the UI
The integration child is displayed with a distinct badge ("Integration Check") in the backlog list and in the epic's dependency graph. Its status is surfaced on the epic item's detail view as "awaiting integration check."

### AC-7 — Tests
Unit tests cover integration child creation (happy path, idempotency, missing e2e command), attribution logic (attributed and unattributed paths), and the epic gate check. At least one e2e spec exercises the full flow: epic created → children all done → integration child auto-created → integration session runs → epic closes.

## Dependencies

- **Epic items feature** (prerequisite): adds `item_type=epic` / parent-child item relationships and the `children` edge to `BacklogItem`. This feature builds on those concepts. Without that feature, the integration child cannot be created because there is no concept of "sibling items within an epic."

## Source References

- Dependency schema: `session/ent/schema/backlog_item_dependency.go`
- Dependency store (writers): `session/backlogstore/dependency_writers.go`
- Dependency store (queries): `session/backlogstore/dependency_queries.go`
- WorkflowEngine gate: `session/workflow_engine.go` / `session/backlog.go` `validTransitions`
- AcCriterion type: `session/domain/backlog.go`
- PipelineMode seed: `session/pipelinemode/pipeline_mode_seed.go`
- Chain firer (workflow chaining on done): `session/backlog/chains/firer.go`
- BacklogItem ent schema: `session/ent/schema/backlog_item.go`
- Guidance request (escalation): `session/ent/schema/guidance_request.go`
