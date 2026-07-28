# Implementation Plan: backlog-sdd-default-pipeline

**Feature**: Seed an "sdd" PipelineMode and let the operator opt new backlog items into it by default
**Date**: 2026-07-24
**Status**: Ready for implementation
**ADRs**: None — no non-standard technology or architecture choice; entirely additive use of the
already-ADR-001-covered `PipelineMode` persistence model and the already-shipped `knownFeatureFlags`
mechanism. See `project_plans/backlog-configurable-pipeline/decisions/ADR-001-pipeline-mode-db-persisted.md`
for the persistence-model decision this project builds on unchanged.

---

## Dependency Visualization

```
session/pipeline_mode_seed.go (new: content + EnsureDefaultSDDPipelineMode)
        │
        ├─→ server/dependencies.go (call seed before NewPipelineEngine's Load)
        │
        ├─→ session/pipeline_mode_seed_test.go (unit tests)
        │
server/services/feature_flag_service.go (new knownFeatureFlags entry)
        │
        ├─→ server/services/backlog_service_lifecycle.go (CreateBacklogItem default branch)
        │       └─→ server/services/backlog_service_lifecycle_test.go (new tests)
        │
        └─→ web-app/src/app/settings/features/page.tsx (FEATURE_META label)

web-app/src/components/backlog/BacklogItemForm.tsx (pre-select "sdd" for new items)
        └─→ web-app/src/components/backlog/BacklogItemForm.test.tsx (new tests)
```

No proto changes. No ent schema/migration changes. No new RPCs.

---

## Phase 1: Seed the "sdd" PipelineMode

### Epic 1.1: Content + seed function
**Goal**: An idempotent, boot-time, non-fatal seed that creates the "sdd" `PipelineMode` row exactly
once and never touches it again.

#### Story 1.1.1: `EnsureDefaultSDDPipelineMode`
**As a** server operator, **I want** the "sdd" pipeline mode to exist automatically after upgrading,
**so that** I don't have to hand-create it through the UI before the feature flag means anything.
**Acceptance Criteria**:
- Row created with slug `sdd` when absent.
- No-op (does not call `Create` or `Update`) when a row with slug `sdd` already exists — an
  operator's later hand-edit via `/settings/pipeline-modes` is never overwritten by a restart.
- Seed content passes `ValidatePipelineModeContent` — enforced by calling it before `Create`, not just
  by inspection.
- A `Create` failure (including a lost create-race `ErrConflict`) never returns an error that would
  abort server boot; logs and returns nil for the race case, logs+returns error only for genuine
  DB failures (caller decides whether to treat as fatal — matches `NewPipelineEngine`'s pattern of
  "caller logs a Warn and continues").
**Files**: `session/pipeline_mode_seed.go` (new)

##### Task 1.1.1a: Write the 9 template fields (~5 min)
- Design content per `research/expressiveness-and-design.md` §1–3: plain-text (no backtick/`$(`/`;`/`|`/`&&`),
  only recognized placeholders, instructs the spawned session to invoke `sdd:2-research` →
  `sdd:3-plan` → `sdd:4-validate` → `sdd:6-verify` itself via its own tool access, skips the
  interactive `sdd:1-ideate` interview (writes `project_plans/<slug>/requirements.md` directly from
  the item's own title/description/acceptance-criteria instead), and still funnels through the exact
  same `report_progress`/`request_review`/`submit_review_verdict` MCP tool contract the default
  pipeline uses so `BacklogLifecycleListener`/`WorkflowEngine` transitions keep working unmodified.
- `review_command_template` and `initial_prompt_template` interpolate the live
  `MaxSameSessionReviewAttempts` constant via `fmt.Sprintf` at seed-build time (matches
  `buildDefaultSlashCommandSet`'s own pattern) rather than hardcoding "3" as literal text.
- Files: `session/pipeline_mode_seed.go`

##### Task 1.1.1b: `EnsureDefaultSDDPipelineMode` (~5 min)
- `GetBySlug("sdd")` → exists: return nil. `ErrNotFound`: validate + `Create`. Any other error: wrap
  and return.
- Files: `session/pipeline_mode_seed.go`

##### Task 1.1.1c: Wire into boot (~3 min)
- In `server/dependencies.go`, call `session.EnsureDefaultSDDPipelineMode(ctx, pipelineModeRepo)`
  immediately after `pipelineModeRepo = session.NewEntPipelineModeRepository(entClient)` and **before**
  `session.NewPipelineEngine(pipelineModeRepo)`, so the engine's first cache `Load` already sees the
  seeded row in one pass. Log-and-continue on error, exactly like the `NewPipelineEngine` failure
  branch immediately below it.
- Files: `server/dependencies.go`

##### Task 1.1.1d: Tests (~5 min)
- `TestEnsureDefaultSDDPipelineMode_should_CreateRow_When_Missing`
- `TestEnsureDefaultSDDPipelineMode_should_BeNoOp_When_AlreadyExists` (assert `Update`/`Create` never
  called via a spy repository, or assert content unchanged after seeding twice with a mutation in
  between)
- `TestEnsureDefaultSDDPipelineMode_should_PassContentValidation` (calls `ValidatePipelineModeContent`
  directly against the seed content, independent of the repository)
- `TestEnsureDefaultSDDPipelineMode_should_NotError_When_CreateRaceLoses` (repository double returns
  `ErrConflict` from `Create`)
- Files: `session/pipeline_mode_seed_test.go` (new)

---

## Phase 2: Opt-in default flag

### Epic 2.1: Backend flag + CreateBacklogItem default
**Goal**: An off-by-default flag that, when on, makes a brand-new item with no explicit
`pipeline_mode` default to `sdd` — never touching any existing item.

#### Story 2.1.1: `backlog:sdd-default-pipeline` flag
**Acceptance Criteria**:
- Appears in `GetFeatureFlags` response, defaults to `enabled: false` on a fresh config.
- Toggleable via `UpdateFeatureFlag` like any other flag (no `FeatureController` needed — it's a pure
  data flag, read fresh at creation time, not a running subsystem to enable/disable).
**Files**: `server/services/feature_flag_service.go`

##### Task 2.1.1a: Add flag entry (~2 min)
- Files: `server/services/feature_flag_service.go`

#### Story 2.1.2: `CreateBacklogItem` default-resolution
**As an** operator with the flag on, **I want** an item created without an explicit pipeline mode to
default to `sdd`, **so that** every new item benefits without me selecting it by hand every time.
**Acceptance Criteria**:
- `req.Msg.PipelineMode == nil` (field truly absent) + flag on → `data.PipelineMode = "sdd"`.
- `req.Msg.PipelineMode != nil` (explicit value, including `""`) → always respected verbatim,
  regardless of flag state — never overrides an explicit choice.
- Flag off → unchanged existing behavior (`req.Msg.GetPipelineMode()`, i.e. `""` when omitted).
- No existing item is ever touched by this — `CreateBacklogItem` only ever runs once, at creation.
**Files**: `server/services/backlog_service_lifecycle.go`

##### Task 2.1.2a: Implement + tests (~5 min)
- Read flag via `config.LoadConfig().GetFeatureFlag(...)`, matching the existing idiom used in
  `server/server.go`'s interceptor and `session/instance_vnc.go` — not `s.cfg`, which several call
  sites (`GetFeatureFlags` itself) intentionally bypass in favor of a fresh disk read so a flag
  toggled via `UpdateFeatureFlag` (which persists through its own `config.LoadConfig()` /
  `SaveConfig()` round-trip) is visible immediately without depending on `BacklogService`'s
  constructor-injected `cfg` pointer being the same live object.
- `TestCreateBacklogItem_should_DefaultPipelineModeToSDD_When_FlagEnabledAndFieldOmitted`
- `TestCreateBacklogItem_should_NotDefaultPipelineMode_When_FlagDisabled`
- `TestCreateBacklogItem_should_RespectExplicitPipelineMode_When_FlagEnabledButFieldSet` (including
  explicit `""`)
- Files: `server/services/backlog_service_lifecycle.go`, `server/services/backlog_service_lifecycle_test.go`

### Epic 2.2: Frontend pre-selection + settings label
**Goal**: The existing selector defaults to `sdd` for a new item when the flag is on and the mode is
available — same escape hatch (manual override) as today, no new UI surface.

#### Story 2.2.1: `BacklogItemForm.tsx` default pre-selection
**Acceptance Criteria**:
- New item (`!initialValues?.id`) + flag on + `sdd` present and enabled in `listPipelineModes()` →
  `pipelineMode` state initializes to `"sdd"` before the user interacts with the field.
- Editing an existing item → never auto-changed, regardless of flag state (only applies at the moment
  a NEW item's form mounts).
- Flag off, or `sdd` absent/disabled, or flags/modes still loading → unchanged current behavior
  (defaults to `""`, i.e. today's flat pipeline).
- User can still change the pre-selected value before submitting — this is a default, not a lock.
**Files**: `web-app/src/components/backlog/BacklogItemForm.tsx`

##### Task 2.2.1a: Implement (~5 min)
- `useFeatureFlag("backlog:sdd-default-pipeline")` + a one-shot effect gated on
  `!initialValues?.id && !autoDefaultApplied && !modesLoading && availableModes` finding an enabled
  `sdd` entry, guarded by a ref/state flag so it only ever applies once (never re-fires after the user
  edits the field back to `""`).
- Files: `web-app/src/components/backlog/BacklogItemForm.tsx`

##### Task 2.2.1b: Tests (~5 min)
- `BacklogItemForm_should_PreselectSddMode_When_CreatingNewItemWithFlagOnAndModeAvailable`
- `BacklogItemForm_should_NotPreselectMode_When_EditingExistingItem`
- `BacklogItemForm_should_NotPreselectMode_When_FlagOff`
- `BacklogItemForm_should_NotPreselectMode_When_SddModeAbsent`
- `BacklogItemForm_should_LetUserOverridePreselectedMode`
- Files: `web-app/src/components/backlog/__tests__/BacklogItemForm.test.tsx` (or existing test file,
  whichever this repo currently uses — confirm exact path before writing)

#### Story 2.2.2: Settings page label
**Files**: `web-app/src/app/settings/features/page.tsx` — one `FEATURE_META` entry, no logic change.

---

## Pattern Decisions

| Decision | Choice | Why |
|---|---|---|
| Seed vs. migration-time data | Boot-time idempotent Go function, not a raw-SQL data migration | ent's migration path in this repo is schema-only (`AutoMigrate`), no existing precedent for seed data via SQL migration; a Go seed matches the existing `BackfillStuckStates` boot-time pattern in `server/dependencies.go` |
| Overwrite vs. create-if-missing | Create-if-missing only | ADR-001 explicitly frames `PipelineMode` content as runtime-operator-editable; overwriting on every boot would silently discard operator customization |
| Default mechanism | Off-by-default feature flag | See requirements.md "Alternatives Considered" — matches this project's own explicit Risk Control precedent for the parent seam |
| Backend default location | `config.LoadConfig()` fresh read, not `s.cfg` | Matches the exact idiom every other live flag-read call site in this codebase already uses (`server/server.go`, `session/instance_vnc.go`, `session/instance_cdp.go`) |
