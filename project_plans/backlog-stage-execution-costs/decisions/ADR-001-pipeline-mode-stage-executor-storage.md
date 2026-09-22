# ADR-001: Store PipelineMode Per-Stage Executors as a JSON String Column, Not Flat Fields or ent's `field.JSON`

## Status
Accepted

## Context
Two SDD Phase 2 research agents disagreed on how to add per-stage program/model
override storage to `PipelineMode` (`session/ent/schema/pipeline_mode.go`):

- **stack.md** proposed 6 new flat `field.String(...).Optional()` columns
  (`triage_program`, `triage_model`, `review_program`, `review_model`,
  `work_program`, `work_model`), reasoning that ent has no first-class
  nested-struct field type.
- **architecture.md** recommended a `map[string]PipelineStageExecutor{Program,
  Model}` sub-structure for the *new* fields only, calling this
  "Refactor-first, narrowly scoped," and sketched it as ent's native
  `field.JSON(...)` column type — while also noting, mid-document, that this
  would introduce a second JSON-storage convention alongside the schema's
  existing string-column approach.

Direct reads of `session/ent/schema/pipeline_mode.go` and
`session/ent/schema/item_session.go` resolve this. `ItemSession` already has
two fields holding JSON in a plain string column, not ent's `field.JSON`
type: `ac_snapshot` ("JSON []AcCriterion at spawn time",
`item_session.go:41-43`) and `triage_result` ("JSON triage suggestions",
`session/repository.go:50-52`'s equivalent on `BacklogItemData`). Both are
marshaled/unmarshaled at the repository boundary via helpers like
`ParseAcCriteria`/`SerializeAcCriteria` (`session/backlog.go:141-161`), not
via ent's separate JSON column mechanism.

## Decision
Add **one** new ent field to `PipelineMode`:

```go
field.String("stage_executors_json").
    Optional().
    Default("{}").
    Comment("JSON-serialized map[StageRole]PipelineStageExecutor{Program,Model} — per-stage execution override. Empty/\"{}\" means no stage has an override; a stage role absent from the map inherits the pool/session default. Marshaled/unmarshaled at the repository boundary (session/pipeline_mode_repository.go), mirroring ac_snapshot's and triage_result's existing string-column-holds-JSON convention rather than introducing ent's field.JSON as a second JSON-storage mechanism in this schema."),
```

This is simultaneously:
- **A single flat ent string field** — satisfying stack.md's constraint that
  ent has no native nested-struct column type, and keeping `PipelineMode`'s
  storage mechanism uniform (every JSON-shaped value in this schema family is
  a string column, never `field.JSON`).
- **A sub-structure at the domain-model level** — satisfying
  architecture.md's recommendation. `session/pipeline_mode_repository.go`'s
  `PipelineModeCreateInput`/`PipelineModeUpdateInput` expose
  `StageExecutors map[StageRole]PipelineStageExecutor`
  (create) / `*map[StageRole]PipelineStageExecutor` (update), and
  `EntPipelineModeRepository` marshals/unmarshals the JSON string at exactly
  the same repository-boundary location `ac_snapshot`/`triage_result`
  already use — no new marshaling-location convention.

## Consequences
- `backlog_service_pipeline_mode.go` (already on the `no_ent_in_services`
  grandfathered depguard exclusion list, `.golangci.yml:115`) only ever
  touches the already-unmarshaled `map[StageRole]PipelineStageExecutor` Go
  value the repository hands back — no new ent-typed surface is added to
  that exemption for this project.
- The 9 pre-existing content-template flat fields on `PipelineMode` are left
  untouched; only the new executor-config axis gets the sub-structure
  treatment (see plan.md's Tech Debt Disposition — "Refactor-first, narrowly
  scoped").
- The new field does **not** participate in `ComputeContentHash`'s existing
  9-field content hash (`pipeline_engine.go`) — program/model is an
  execution-config concern, not rendered-content, and folding it in would
  silently change what "content drift" means for every already-shipped
  session snapshot. A separate `executor_snapshot_hash` field on
  `ItemSession` tracks executor drift instead (see plan.md Phase 2).
