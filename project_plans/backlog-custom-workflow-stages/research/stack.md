# Research: Technology Stack — backlog-custom-workflow-stages

## Summary

This project has **no new library dependency to add**. Every technical need
(DB-persisted config, in-process caching, state-machine seam, gate execution,
proto/RPC surface, UI editing surface) already has a repo-internal precedent
from `WorkflowRepository`/`ent.Workflow` (webhook/cron triggers — a
different, unrelated "Workflow" concept, not the BacklogStatus state
machine) and, more directly, from the sibling `backlog-configurable-pipeline`
project's `PipelineMode`/`PipelineEngine`. The one real gap is the frontend:
**no directed-graph/node-edge visualization library exists in
`web-app/package.json`** — Milestone 2's transition-editor UI is new frontend
surface, not an extension of an existing graph widget.

## ent ORM conventions (follow exactly — do not invent new ones)

Two existing ent schemas are the templates, at
`session/ent/schema/workflow.go` and `session/ent/schema/pipeline_mode.go`
(both `entgo.io/ent v0.14.5`, per `go.mod:19`). Shared shape:

- `field.UUID("id", uuid.UUID{}).Default(uuid.New)`
- `field.String("slug").Unique().NotEmpty()` — the addressable key external
  callers use (never the UUID)
- `field.Bool("enabled").Default(true)`
- `field.Time("created_at").Default(time.Now).Immutable()`
- `field.Time("updated_at").Default(...).UpdateDefault(...)` — `pipeline_mode.go`
  uses local `time.Now`; `workflow.go`'s is UTC-normalized
  (`func() time.Time { return time.Now().UTC() }`) specifically because
  `mattn/go-sqlite3` stores a `time.Time` TEXT column in the value's own
  Location, and `UpdateWorkflowRequest.expected_updated_at`-style CAS
  preconditions compare against `AsTime()`'s UTC. **New stage/transition/
  liveness tables should use the UTC form** if any RPC exposes an
  optimistic-concurrency `expected_updated_at` field on them (`WorkflowRepository.UpdateConditional`,
  `session/workflow_repository.go:20`, is that CAS pattern to mirror).
- `Indexes()` on `slug`, `enabled`/`cron_enabled`, `created_at` at minimum.

Repository layer convention (`session/workflow_repository.go`, 97 lines +
`session/ent_workflow_repository.go`, 324 lines): a narrow Go interface
(`Create`, `Update`, `UpdateConditional`, `Delete`, `GetByID`, `GetBySlug`,
`ListAll`, `ListEnabled`) plus paired `*CreateInput`/`*UpdateInput` structs.
`*UpdateInput` fields are **all pointers** for partial-update semantics
(`nil` = don't touch) — this is the pattern the requirements doc's
Constraints section explicitly calls out to avoid repeating the
`SkipReviewGate`-as-non-optional-proto3-bool clobbering bug. New
stage/transition/liveness repositories should copy this shape verbatim.

**Reminder from this repo's `CLAUDE.md`**: any ent schema change must be
regenerated with
`go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`
(not the flag-less form — it breaks `UpsertRule`), and the generated
`session/ent/*.go` output is gitignored/never committed — only
`session/ent/schema/*.go` changes are.

## State machine / DAG library

**No third-party state-machine or graph library is used anywhere in
`go.mod`.** `grep -iE "state|fsm|graph|dag"` over `go.mod` returns nothing
relevant. The existing "state machine" is hand-rolled plain Go:

- `session/backlog.go`'s `validTransitions map[BacklogStatus]map[BacklogStatus]bool`
  (the 9-state adjacency map) + `TransitionGuard` (per-transition gate
  functions).
- `session/workflow_engine.go`'s `WorkflowEngine` interface — 3 methods only
  (`CanTransition(from, to) bool`, `ValidateGates(item, to) error`,
  `AllowedTransitions(from) []BacklogStatus`) — wrapping that map.
  `DefaultWorkflowEngine` deep-copies `validTransitions` at construction
  (`NewDefaultWorkflowEngine`) so no caller can mutate shared state through
  the map.

Per the requirements doc's own Feasibility Risks, this 3-method interface
"was designed for status-transition legality only, with no liveness concept
at all" — Phase 3 must design any extension, not bolt on a 4th method
mechanically. There is no existing DAG package pulled in to reach for; a
`ConfiguredWorkflowEngine` would model the DB-persisted adjacency the same
hand-rolled way (`map[stageSlug]map[stageSlug]bool` or equivalent), matching
`DefaultWorkflowEngine`'s existing shape rather than adopting an external
graph library.

## Caching layer (reuse this, don't build a second one)

`backlog-configurable-pipeline` already solved "DB-persisted, slug-addressed
config, cheap to read on hot paths" with a **hand-rolled, dependency-free**
copy-on-write cache — no `groupcache`/`ristretto`/`otter`/similar library
involved (`go.mod` only has `golang/groupcache` as a transitive/indirect dep
of something unrelated, unused here). The pattern
(`session/pipeline_engine.go:123-200`, `pipelineModeCache`):

```go
type pipelineModeCache struct {
    ptr     atomic.Pointer[map[string]resolvedPipelineMode]
    writeMu sync.Mutex
}
```

- **Read path** (`Get`): single atomic `Load` + map lookup, lock-free, never
  touches `writeMu` — a reader is never blocked behind a writer.
- **Write path** (`Load`/`Invalidate`, both call shared `refresh`): `writeMu`
  is held across the *entire* DB-read-then-`Store` sequence — not just the
  `Store` — specifically to prevent a lost-update race where a slower
  concurrent `Invalidate`'s `Store` lands after a faster, later-started
  caller's `Store` and reverts the cache to stale data. Documented rationale:
  `project_plans/backlog-configurable-pipeline/implementation/plan.md`'s
  "Pattern Decisions" table.
- Cache entries (`resolvedPipelineMode`) are deep-copied by value from the
  `*ent.PipelineMode` at load time — mirrors `NewDefaultWorkflowEngine`'s
  deep-copy-on-construct discipline — so concurrent readers never observe a
  partially-updated ent object mid-swap.
- `NewPipelineEngine` does one synchronous `cache.Load` at construction; a
  `Load` failure is logged at Warn and construction still succeeds with an
  empty cache (fail-open to default behavior, never a crash) — directly
  matches this project's own Risk Control requirement ("fail closed to
  default built-in behavior with a loud Warn log").
- Write RPC handlers call `InvalidateCache` after every mutating RPC.

**Recommendation for this project**: reuse this exact `atomic.Pointer[map[...]]`
+ `sync.Mutex`-on-write pattern for the new stage/transition/liveness config
cache, per the requirements doc's own NFR ("Phase 3 planning should evaluate
reusing that same cache infrastructure rather than building a second one").
No new caching dependency is needed either way — this is a Go-stdlib-only
pattern (`sync/atomic`, `sync`).

## Background sweep / reconciliation loops

`session/backlog_lifecycle*.go` hosts the `reconcile*` stuck-detection
sweeps that the new liveness model must plug into instead of reading
hardcoded constants directly:

- `session/backlog_lifecycle_stale.go:62` — `const maxWorkSessionStaleness = 2 * time.Hour`
  (general in-progress work staleness).
- `session/backlog_lifecycle_triage.go:64` — `const maxHeadlessTriageSessionStaleness = 35 * time.Minute`
  (headless triage staleness — the motivating bug's constant; a comment says
  it "MUST stay strictly greater than" `triageCallBudget` in
  `server/services/backlog_service_triage.go:434`, enforced only by a unit
  test assertion today, `session/backlog_lifecycle_stuck_test.go:1457-1458`
  — exactly the BUG-055 invariant the new liveness model must make structural).
- `session/backlog_lifecycle_review.go:732` — `const reviewVerdictIdleThreshold = maxWorkSessionStaleness`
  (derived, not independent — a precedent for "derive one threshold from
  another" that the new model should generalize).
- `session/backlog_remediation.go` — the shared backoff/retry gate consulted
  by the sweeps: `evaluateRemediation`, `RemediationDue`, `RemediationBlocked`,
  `nextRemediationAt`/`nextRemediationAtForAttempt` (backoff schedule),
  `remediationColdRetryInterval = 7 * 24 * time.Hour` (BUG-083's cold-retry
  heartbeat, already in place — this is why Milestone 1 can ship
  independently per the requirements doc's Risk Control section).

No scheduling/cron library is involved here either — these are plain Go
functions invoked from an existing periodic sweep loop already present in
`session/backlog_lifecycle.go`.

## Automated-review-verdict gate (reuse, don't reinvent)

`session/review_gate.go`'s `ReviewGateRunner` (`NewReviewGateRunner`, `.Run`)
already implements the PASS/FAIL/UNVERIFIABLE automated-reviewer flow the
requirements doc says to generalize rather than duplicate. It already
consumes `PipelineEngine`'s `ReviewPromptFor` for mode-specific review
prompts, so plugging a configurable-transition automated-review gate on top
should parameterize `ReviewGateRunner`, not build a second review-invocation
path.

## Proto / Connect-RPC conventions

`connectrpc.com/connect v1.20.0` (`go.mod:17`), `google.golang.org/protobuf v1.36.12`.
`proto/session/v1/backlog.proto` already has the CRUD RPC pattern to mirror
for the new stage/transition/gate/liveness config, exemplified by
`PipelineMode` (message at line 247; RPCs at lines 995-1007):
`CreatePipelineMode`, `UpdatePipelineMode`, `DeletePipelineMode`,
`GetPipelineMode`, `ListPipelineModes` — a flat 5-RPC CRUD surface per
config entity. New stage/transition/gate/liveness entities should each get
an equivalent RPC quintet (or a combined message if Phase 3 planning decides
transitions/gates nest under a stage rather than standing alone), regenerated
via `make proto-gen` per this repo's `CLAUDE.md`.

## Frontend stack

`web-app/package.json` (Next.js 15.3.2, React 19, TypeScript 5.9.3) relevant
libraries:

- **No graph/node-edge visualization library exists** — confirmed absent:
  no `reactflow`/`@xyflow/react`, `dagre`, `cytoscape`, `d3` (only `recharts`
  ^3.8.1 for charts, which is not a graph/diagram library), `vis-network`, or
  similar. **This is a real gap**: Milestone 2's stage/transition
  management UI (explicitly flagged in the requirements doc's Rabbit Holes
  as "a graph/transition editor is a different shape than
  `backlog-configurable-pipeline`'s flat mode-selector dropdown") has no
  existing widget to extend and will need either a new dependency (e.g.
  `@xyflow/react`, the current maintained successor to `react-flow`) or a
  hand-built layout (the existing `react-arborist` ^3.4.3 is a *tree*
  component — single-parent hierarchy — not a suitable substitute for
  arbitrary-transition directed-graph editing with cycles/multiple
  in/out edges).
- `@dnd-kit/core`/`@dnd-kit/sortable`/`@dnd-kit/utilities` — already used
  for `BacklogBoard.tsx`'s kanban drag-and-drop; relevant only if Milestone
  2's stage-list ordering UI wants drag-to-reorder, not for the graph editor
  itself.
- `react-hook-form` ^7.63.0 + `@hookform/resolvers` ^5.2.2 + `zod` ^4.1.11 —
  the existing form-validation stack, used by
  `web-app/src/app/settings/pipeline-modes/PipelineModeForm.tsx` (the direct
  precedent for a stage/transition/liveness definition form).
- `@radix-ui/react-dialog`, `-tabs`, `-accordion`, `-tooltip` — existing
  primitives for any modal/tabbed editor chrome.
- `@monaco-editor/react` / `monaco-editor` — already used for
  template-content editing (pipeline mode prompt templates); could be reused
  if a custom/pluggable gate check's script content needs editing, but that
  is Milestone 2+ scope per the requirements doc's own scope-bounding
  guidance on custom checks.

`web-app/src/components/backlog/BacklogBoard.tsx:52` — `COLUMNS` is a
hardcoded `{ status: BacklogItemStatus; label: string }[]` array (confirmed
by grep; the requirements doc's characterization is accurate) that must
become dynamic once stages are DB-configured.

## Key file/version references

| Item | Location |
|---|---|
| ent ORM version | `go.mod:19` — `entgo.io/ent v0.14.5` |
| Connect-RPC version | `go.mod:17` — `connectrpc.com/connect v1.20.0` |
| protobuf version | `go.mod:61` — `google.golang.org/protobuf v1.36.12` |
| `Workflow` ent schema (webhook/cron trigger — unrelated concept, don't confuse with BacklogStatus workflow) | `session/ent/schema/workflow.go` |
| `PipelineMode` ent schema (direct precedent) | `session/ent/schema/pipeline_mode.go` |
| `WorkflowRepository` interface + CAS pattern | `session/workflow_repository.go` |
| ent-backed repository impl | `session/ent_workflow_repository.go` |
| `WorkflowEngine` interface (3 methods, to extend) | `session/workflow_engine.go` |
| Hardcoded transition map | `session/backlog.go` (`validTransitions`, `TransitionGuard`) |
| `PipelineEngine` + copy-on-write cache | `session/pipeline_engine.go` |
| Cache pattern rationale | `project_plans/backlog-configurable-pipeline/implementation/plan.md` ("Pattern Decisions" table) |
| Stuck-detection sweeps | `session/backlog_lifecycle.go`, `backlog_lifecycle_stale.go`, `backlog_lifecycle_triage.go`, `backlog_lifecycle_review.go` |
| Remediation backoff gate | `session/backlog_remediation.go` |
| Automated review gate (reuse target) | `session/review_gate.go` |
| PipelineMode proto CRUD (RPC precedent) | `proto/session/v1/backlog.proto:247` (message), `:995-1007` (RPCs) |
| PipelineMode settings UI (form precedent) | `web-app/src/app/settings/pipeline-modes/PipelineModeForm.tsx` |
| BacklogBoard hardcoded columns | `web-app/src/components/backlog/BacklogBoard.tsx:52` |
| Frontend dependency manifest | `web-app/package.json` |
