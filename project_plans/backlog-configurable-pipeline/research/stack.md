# Stack Research: backlog-configurable-pipeline

## 1. ent schema pattern (precedent: `auto_spawn_session`)

`session/ent/schema/backlog_item.go` (`BacklogItem.Fields()`) — the direct precedent for any
new pipeline-mode field:

```go
field.Bool("auto_spawn_session").
    Default(false).
    Comment("When true, a work session is spawned automatically once the item reaches ready — no manual 'Spawn Session' click required."),
```

Pattern: simple typed field, explicit `.Default(...)`, a doc `.Comment(...)` explaining semantics.
No enum-typed ent field is used anywhere in this schema today — statuses (`status`,
`skip_review_gate`, `skip_planning`) are all plain `Bool`/`String`, validated at the Go layer, not
via ent's `field.Enum`. If `PipelineEngine` mode needs to be a small closed set (`"default"`,
`"quick"`, `"full"`), the codebase convention is a `field.String("pipeline_mode").Default("default")`
validated against a Go-side registry — **not** `field.Enum`, to stay consistent with how `status`
(a much larger state machine) is already modeled as a bare string + `BacklogStatus` type + registry
map (`session/backlog.go` `validTransitions`).

Regeneration requires the exact command in `session/ent/generate.go`:
```
go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
```
(never the bare `ent generate` — silently breaks `UpsertRule`/upsert methods).

## 2. Proto pattern (precedent: `auto_spawn_session` in backlog.proto)

`proto/session/v1/backlog.proto` adds new fields to 3 messages in lockstep — exactly the shape
described in the requirements:
- `BacklogItem` (field 24): `bool auto_spawn_session = 24;`
- `CreateBacklogItemRequest` (field 10): `bool auto_spawn_session = 10;`
- `UpdateBacklogItemRequest` (field 12): `bool auto_spawn_session = 12;`

Field numbers are simply "next available" per message (independent numbering per message, not
global). A new `pipeline_mode` field would need 3 new field numbers (one per message) at their
respective next-available slots (currently: `BacklogItem` next is 25, `CreateBacklogItemRequest`
next is 11, `UpdateBacklogItemRequest` next is 13).

**Enum precedent already exists in this same file** — `StuckReason` (lines 577–586) is a proto3
`enum` with an explicit `_UNSPECIFIED = 0` fallback value and a comment explaining it "never
panics" when mapping an unrecognized DB string. This is the closest existing precedent for a
`PipelineMode` enum, if the design goes that route instead of a plain string:
```protobuf
enum StuckReason {
  STUCK_REASON_UNSPECIFIED = 0;
  STUCK_REASON_PR_READY_UNMERGED = 1;
  ...
}
```
Comment above it explicitly says the enum "mirrors domain.StuckReason (session/domain/backlog.go)
— the validated string-backed enum" — i.e. the Go domain type is a string-backed type with its own
validation, and the proto enum is a parallel wire-safe mirror, mapped defensively (unknown →
UNSPECIFIED, never a panic). This is a strong, current-code precedent for how to wire a
`PipelineMode` both as a Go string-backed type and a proto enum without them getting out of sync.

`make proto-gen` regenerates both `session/gen/session/v1/*.go` and
`web-app/src/gen/session/v1/*_pb.ts` from this single source of truth — no manual TS/Go duplication.

## 3. Existing "named registry" patterns to mirror

Three real precedents exist; **`PluginRegistry`/`ItemSourcePlugin`** (Go, backend) is the closest
match for a `PipelineEngine` mode registry, since it's a small registry of named implementations
keyed by string ID, exactly the "code-defined registry" shape requirements.md is biased toward:

**`session/backlog_plugin.go`** — `ItemSourcePlugin` interface (narrow: `PluginID()`, `Fetch(...)`,
`MapToBacklogItem(...)`), `PluginRegistry` struct wrapping a `map[string]ItemSourcePlugin`,
`Register(p)`/`Get(id)` methods, and a `NewDefaultRegistry()` constructor that pre-registers the
built-ins:
```go
func NewDefaultRegistry() *PluginRegistry {
    r := NewPluginRegistry()
    r.Register(NewGitHubIssuesPlugin())
    r.Register(NewGitHubPRsPlugin())
    return r
}
```
This is a direct template: `PipelineEngineRegistry` (or simply a `map[string]PipelineEngine`
constructed in one place) with `Register`/`Get` and a `NewDefaultRegistry()` seeding
`"default"`/`"quick"`/`"full"` mode implementations — no DB persistence, matches requirements.md's
stated bias.

**`session/workflow_engine.go`** — `WorkflowEngine` interface (3 methods: `CanTransition`,
`ValidateGates`, `AllowedTransitions`) is the single-implementation-so-far seam pattern named
explicitly in the requirements' Constraints section as the pattern to reuse. `NewDefaultWorkflowEngine()`
**deep-copies** the package-level `validTransitions` map at construction time so no shared mutable
state leaks between instances — the same deep-copy-on-construct discipline should apply to a
`PipelineEngine`'s internal mode table if it holds mutable per-mode config.

**Frontend `DetectorRegistry`** (`web-app/src/lib/omnibar/detector.ts`) — priority-sorted list of
detector objects, `createDefaultRegistry()` factory, dynamic detectors registered separately at
runtime via context effects. Less directly applicable here (no proposed frontend "detection" need
for pipeline mode — it's a static selector, not an auto-detector) but establishes the sibling
naming convention (`createDefaultRegistry`) if a TS-side registry of pipeline-mode metadata
(labels/hints for the UI selector) is also wanted, mirroring `SESSION_TYPES` in
`OmnibarCreationPanel.tsx` more than `DetectorRegistry` itself.

No `session/detection/` registry (that directory holds `binary_detector.go`, a single-purpose PATH
binary prober — not a strategy registry, not a relevant precedent).

## 4. Wiring pattern: `WorkflowEngine` construction → injection

`server/dependencies.go` (`BuildRuntimeDeps`, ~line 457-459):
```go
// WorkflowEngine governs backlog state transitions; constructed once and shared
// by the service layer.
workflowEngine := session.NewDefaultWorkflowEngine()
```
Constructed once, as a concrete value, then passed into `NewBacklogService(storage, creator, cfg,
engine)` (`server/services/backlog_service.go:134`). The service also nil-guards at construction:
```go
func NewBacklogService(storage *session.Storage, creator SessionCreator, cfg *config.Config, engine session.WorkflowEngine) *BacklogService {
    if engine == nil {
        engine = session.NewDefaultWorkflowEngine()
    }
    ...
}
```
This is the wiring template a `PipelineEngine` (or `PipelineEngineRegistry`) should follow:
construct once in `dependencies.go`, inject via constructor param on `BacklogService` (or wherever
`WriteSlashCommands`/triage-prompt-builder/review-gate-runner live), with the same nil-safe
default-engine fallback for tests that construct `BacklogService` directly without going through
`BuildRuntimeDeps`.

## 5. Fixed command/stage set to be made pluggable

`session/backlog_commands.go` `WriteSlashCommands(item *BacklogItemData, worktreePath string)` is
the literal target for "different command set per mode." It currently writes a **fixed** file set
unconditionally: `status.md`, `done-N.md`/`fail-N.md` per AC criterion, `review.md`, `ship.md`,
`help.md` — always the same regardless of item. Command bodies embed the item ID via `fmt.Sprintf`
directly (itemID is a UUID from storage, not user free text, so today's interpolation is safe —
but this is exactly the code path the requirements' injection-risk constraint is warning about:
any future mode-driven text must not flow unsanitized into these `fmt.Sprintf` calls).

`/sdd:quick` vs `/sdd:full` (checked in `~/.config/opencode/agents/skills/sdd/skills/{quick,full}/SKILL.md`
— note: **not** present under `~/.claude/skills/sdd/`, only under the opencode config path on this
machine) differ by **phase-gate count and artifact-writing**, not by a different skill roster:
`full` orchestrates all 7 phase files sequentially with disk artifacts and a resume checkpoint
between phases; `quick` collapses everything into one ungated pass with no artifacts written. This
maps more naturally to "which stages run, and whether they gate/pause" than to "a different set of
skills invoked" — reinforcing requirements.md's framing that `PipelineEngine` should answer
*which stages exist and in what sequence* (mirroring `SkipReviewGate`/`SkipPlanning`'s existing
"skip a stage" pattern, generalized to "choose a stage sequence"), rather than swapping which
underlying Claude Code skill implements a given stage.

## 6. Dependency versions

- Go: `go 1.26.3` (go.mod)
- `entgo.io/ent v0.14.5`
- `connectrpc.com/connect v1.19.0`, `connectrpc.com/otelconnect v0.8.0`
- `google.golang.org/protobuf v1.36.11`
- Frontend: Next.js `15.3.2`, React `^19.0.0`, TypeScript `^5.9.3`, `@bufbuild/protobuf ^2.11.0`,
  `@connectrpc/connect` / `@connectrpc/connect-web` `^2.1.1`

No compatibility constraints found for adding enum-like fields — proto3 enums, ent string fields,
and ConnectRPC's codegen are all already exercised by `StuckReason` and the existing `Bool`/`String`
backlog fields at these exact versions; no version bump is implicated by this feature.

## Recommendations feeding into planning

- Model `pipeline_mode` as a plain `field.String` in ent (mirrors `status`, not a `field.Enum`),
  paired with a Go string-backed type + validation (mirrors `domain.StuckReason` string-backed
  pattern) and a parallel proto `enum PipelineMode` with `_UNSPECIFIED = 0` fallback (mirrors
  `StuckReason` exactly, including the "never panics on unknown string" defensive-mapping comment).
- Build `PipelineEngine` as a `PluginRegistry`-shaped registry (`map[string]PipelineEngine` +
  `Register`/`Get` + `NewDefaultRegistry()`), not a DB-backed table — matches the existing
  `PluginRegistry` precedent and requirements.md's stated bias toward a simple code-defined
  registry.
- Wire construction in `server/dependencies.go` alongside `workflowEngine`, inject via constructor
  param into whatever service(s) end up owning `WriteSlashCommands`/triage-prompt-building/review-gate
  running, with the same nil-safe default-instance fallback `NewBacklogService` already uses for
  `WorkflowEngine`.
- Target `WriteSlashCommands` (`session/backlog_commands.go`) as the concrete seam for "stage
  sequence changes what commands exist" — today's fixed file set is the literal thing to make mode-
  aware; validate any mode-driven string against the registry before it reaches the `fmt.Sprintf`
  calls that build command file bodies (injection-risk constraint from requirements.md).
