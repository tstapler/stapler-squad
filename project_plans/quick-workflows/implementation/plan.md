# Quick Workflows — Implementation Plan

**Feature branch:** `stapler-squad-quick-workflows`  
**Plan date:** 2026-06-11  
**Based on research:** stack-storage.md, omnibar-architecture.md, cron-scheduling.md, ui-patterns.md

---

## Architecture Decisions

### ADR-1: Storage — ent ORM (SQLite)

Use a `Workflow` entity in `session/ent/schema/workflow.go`, backed by the existing `sessions.db`. Auto-migrated by `client.Schema.Create(ctx)` on startup. Reference implementation: `BacklogItem` (UUID PK, timestamps) and `ApprovalRule` (Optional JSON fields for additive evolution).

**Generate command (CRITICAL — must include `--feature sql/upsert`):**
```bash
go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
```

### ADR-2: Omnibar Syntax — `@slug [arg]` at detector priority 25

`@` is unclaimed by all existing detectors and pre-processors. `WorkflowDetector` runs at priority 25 — after GitHub URL detectors (10/20/30) and before `NewSessionDetector` (35). The `@slug` token cannot be mistaken for `PathWithBranchDetector` because that detector requires a path-like prefix before `@`.

New enum value: `InputType.Workflow = "workflow"` in `web-app/src/lib/omnibar/types.ts`.

### ADR-3: New OmnibarAction — `run_workflow`

Add `{ type: "run_workflow"; workflowSlug: string; workflowArg: string; label: string }` to the discriminated union. Dispatch case builds `OmnibarSessionData` and calls `deps.createSession`. The `ActionDeps` interface gains a `runWorkflow(slug, arg)` dep provided by `OmnibarContext`.

### ADR-4: Proto API — New RPCs on existing `SessionService`

Five new RPCs added to `SessionService` in `proto/session/v1/session.proto`. No new service file needed — workflows are a first-class domain entity that fits the existing service boundary. Field numbers continue from highest existing number. Run `make generate-proto` after changes.

### ADR-8: `RunWorkflow` RPC fires via scheduler — avoids circular import

`WorkflowService.RunWorkflow` must not call `SessionService.CreateSession` directly — that would create a circular import (`services.SessionService` → `services.WorkflowService` → `services.SessionService`). Instead, `RunWorkflow` delegates to `WorkflowScheduler.FireNow(ctx, workflowID, arg)`, which already holds a `SessionServiceInterface`. This keeps `WorkflowService` free of any `SessionService` dependency.

```go
// WorkflowService.RunWorkflow delegates to the scheduler
func (s *WorkflowService) RunWorkflow(ctx context.Context, req ...) (...) {
    wf, err := s.repo.GetByID(ctx, id)
    // ...
    sessionID, err := s.scheduler.FireNow(ctx, wf, req.Msg.Arg)
    // ...
}
```

`WorkflowScheduler` gains a `FireNow(ctx context.Context, wf *ent.Workflow, arg string) (string, error)` method that calls `sessionSvc.CreateSession`, returning the created session ID.

### ADR-9: `targetDirectory` — Optional in schema, required by handler validation

`requirements.md` US-04 marks `targetDirectory` as required. The ent schema keeps it `Optional()` (for future workflow types that compute the path at runtime), but the `CreateWorkflow` RPC handler validates it as required at the application layer. This gives us schema flexibility without violating the requirement.

### ADR-5: Cron — `github.com/robfig/cron/v3`

New dependency, no existing cron library in `go.mod`. `WorkflowScheduler` struct in `server/workflows/scheduler.go` wraps `robfig/cron`. Follows the `Start(ctx context.Context)` + `<-ctx.Done()` convention used by all background services. Added to `ServerDependencies` and wired in `wireDepsIntoServer`.

### ADR-6: Session type for cron runs — one-off by default

Cron-triggered sessions use `oneOff: true` + `SessionType_DIRECTORY` (the existing one-off path in the server). No new proto enum value. The workflow schema stores `session_type` as a string; the scheduler maps it to request fields via the same logic as `OmnibarContext.sessionTypeMap`.

### ADR-7: WorkflowScheduler hot-reload

When a workflow is created/updated/deleted via RPC, the scheduler's `Reload()` method removes the old cron entry (`c.Remove(entryID)`) and, if enabled, re-adds it. A `sync.Map[string, cron.EntryID]` tracks slug→entryID.

---

## Epic Structure

| Epic | Title | Stories | Tasks |
|------|-------|---------|-------|
| E1 | Backend Storage + Repository | 3 stories | 7 tasks |
| E2 | Proto API + RPC Handlers | 2 stories | 8 tasks |
| E3 | Omnibar Integration | 3 stories | 9 tasks |
| E4 | Workflow Management UI + Cron Scheduler | 4 stories | 11 tasks |

**Total: 4 epics, 12 stories, 35 tasks**

---

## Epic 1: Backend Storage + Repository

### Story 1.1 — Workflow ent Schema

**Complexity: M**

Define the `Workflow` entity in the ent schema.

#### Task 1.1.1 — Create ent schema file

**Files to create:**
- `session/ent/schema/workflow.go`

```go
package schema

import (
    "time"
    "entgo.io/ent"
    "entgo.io/ent/schema/field"
    "entgo.io/ent/schema/index"
    "github.com/google/uuid"
)

type Workflow struct{ ent.Schema }

func (Workflow) Fields() []ent.Field {
    return []ent.Field{
        field.UUID("id", uuid.UUID{}).Default(uuid.New),
        field.String("slug").Unique().NotEmpty(),
        field.String("name").NotEmpty(),
        field.String("description").Optional(),
        field.String("command").NotEmpty(),
        field.String("target_directory").Optional(),
        field.String("input_template").Optional(),
        field.String("session_type").Optional().Default("directory"),
        field.String("model").Optional(),
        field.String("agent_type").Optional(),
        field.String("cron_expression").Optional(),
        field.Bool("cron_enabled").Default(false),
        field.Time("created_at").Default(time.Now).Immutable(),
        field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
    }
}

func (Workflow) Edges() []ent.Edge { return nil }

func (Workflow) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("slug"),
        index.Fields("cron_enabled"),
        index.Fields("created_at"),
    }
}
```

**Notes:**
- `slug` has a DB-level `UNIQUE` constraint — duplicate slug creation returns `ent.ConstraintError`
- `session_type` defaults to `"directory"` (safe default matching existing behavior)
- `target_directory` is `Optional()` because future workflow types may compute the path at runtime

#### Task 1.1.2 — Run ent code generation

```bash
go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
go build ./...
```

Commit all generated `session/ent/` files together with `workflow.go`. Do NOT commit partial generation.

**Files modified (auto-generated):**
- `session/ent/workflow.go` (new)
- `session/ent/workflow_create.go` (new)
- `session/ent/workflow_update.go` (new)
- `session/ent/workflow_delete.go` (new)
- `session/ent/workflow_query.go` (new)
- `session/ent/client.go` (updated — `Workflow` client accessor)
- `session/ent/ent.go` (updated — entity registration)
- `session/ent/migrate/schema.go` (updated — table DDL)

---

### Story 1.2 — WorkflowRepository

**Complexity: M**

Define repository interface and ent-backed implementation. Pattern: parallel to `session/repository.go` + `session/ent_repository.go`.

#### Task 1.2.1 — Define WorkflowRepository interface

**Files to create:**
- `session/workflow_repository.go`

```go
package session

import (
    "context"
    "github.com/google/uuid"
    "github.com/tstapler/stapler-squad/session/ent"
)

// WorkflowRepository defines persistence operations for workflow definitions.
type WorkflowRepository interface {
    Create(ctx context.Context, w WorkflowCreateInput) (*ent.Workflow, error)
    Update(ctx context.Context, id uuid.UUID, w WorkflowUpdateInput) (*ent.Workflow, error)
    Delete(ctx context.Context, id uuid.UUID) error
    GetByID(ctx context.Context, id uuid.UUID) (*ent.Workflow, error)
    GetBySlug(ctx context.Context, slug string) (*ent.Workflow, error)
    ListAll(ctx context.Context) ([]*ent.Workflow, error)
    ListEnabled(ctx context.Context) ([]*ent.Workflow, error) // cron_enabled=true
}

type WorkflowCreateInput struct {
    Slug            string
    Name            string
    Description     string
    Command         string
    TargetDirectory string
    InputTemplate   string
    SessionType     string
    Model           string
    AgentType       string
    CronExpression  string
    CronEnabled     bool
}

type WorkflowUpdateInput struct {
    Name            *string
    Description     *string
    Command         *string
    TargetDirectory *string
    InputTemplate   *string
    SessionType     *string
    Model           *string
    AgentType       *string
    CronExpression  *string
    CronEnabled     *bool
}
```

**Key types:** `WorkflowCreateInput`, `WorkflowUpdateInput` (pointer fields for partial updates)

#### Task 1.2.2 — Implement EntWorkflowRepository

**Files to create:**
- `session/ent_workflow_repository.go`

Implement `WorkflowRepository` using the generated ent client from `EntRepository.GetEntClient()`. Key methods:
- `Create`: `client.Workflow.Create().SetSlug(w.Slug)...Save(ctx)` — return `ent.ConstraintError` on duplicate slug
- `Update`: `client.Workflow.UpdateOneID(id).SetNillableName(w.Name)...Save(ctx)`
- `Delete`: `client.Workflow.DeleteOneID(id).Exec(ctx)`
- `ListAll`: `client.Workflow.Query().Order(ent.Asc(workflow.FieldCreatedAt)).All(ctx)`
- `ListEnabled`: `client.Workflow.Query().Where(workflow.CronEnabled(true)).All(ctx)`

**Files modified:**
- `server/dependencies.go` — add `WorkflowRepo session.WorkflowRepository` to `ServerDependencies`

#### Task 1.2.3 — Wire WorkflowRepository into server deps

**Files modified:**
- `server/dependencies.go` — add `WorkflowRepo session.WorkflowRepository` field to BOTH `ServerDependencies` struct AND `RuntimeDeps` struct; update `RuntimeDeps.ToServerDeps()` to map the new field (critical: if `ToServerDeps()` is not updated, `WorkflowRepo` will be `nil` in Warren-lifecycle deployments, causing nil pointer panic at runtime)
- `server/server.go` (or `server/wire.go`) — instantiate `EntWorkflowRepository` in `BuildCoreDeps`, passing `entRepo.GetEntClient()`

**Verification step:** After adding to `ServerDependencies`, grep for `ToServerDeps` in `server/dependencies.go` and confirm the new field is explicitly mapped. TypeScript has no equivalent of `ToServerDeps` — this is Go-only.

---

### Story 1.3 — Slug Validation Utility

**Complexity: S**

**Files to create:**
- `session/workflow_slug.go`

Validate slug format: `[a-z0-9][a-z0-9-]*[a-z0-9]` (min 2 chars, no consecutive hyphens, no leading/trailing hyphen). This is used by both the RPC handler (server-side validation) and can be surfaced to the frontend.

```go
package session

import (
    "regexp"
    "fmt"
)

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`)

func ValidateWorkflowSlug(slug string) error {
    if len(slug) < 2 || len(slug) > 64 {
        return fmt.Errorf("slug must be 2–64 characters")
    }
    if !slugRe.MatchString(slug) {
        return fmt.Errorf("slug must be lowercase alphanumeric with hyphens (no leading/trailing/consecutive hyphens)")
    }
    return nil
}
```

---

## Epic 2: Proto API + RPC Handlers

### Story 2.1 — Proto Definitions

**Complexity: M**

#### Task 2.1.1 — Add Workflow message + RPCs to proto

**Files modified:**
- `proto/session/v1/session.proto`

Add 5 new RPC methods to `SessionService`:

```protobuf
// CreateWorkflow creates a new workflow definition.
rpc CreateWorkflow(CreateWorkflowRequest) returns (CreateWorkflowResponse) {}

// UpdateWorkflow modifies an existing workflow definition.
rpc UpdateWorkflow(UpdateWorkflowRequest) returns (UpdateWorkflowResponse) {}

// DeleteWorkflow removes a workflow definition permanently.
rpc DeleteWorkflow(DeleteWorkflowRequest) returns (DeleteWorkflowResponse) {}

// ListWorkflows returns all saved workflow definitions.
rpc ListWorkflows(ListWorkflowsRequest) returns (ListWorkflowsResponse) {}

// RunWorkflow immediately fires a workflow (outside of cron schedule).
rpc RunWorkflow(RunWorkflowRequest) returns (RunWorkflowResponse) {}
```

Add message types (use next available field numbers):

```protobuf
message WorkflowProto {
  string id = 1;
  string slug = 2;
  string name = 3;
  string description = 4;
  string command = 5;
  string target_directory = 6;
  string input_template = 7;
  string session_type = 8;
  string model = 9;
  string agent_type = 10;
  string cron_expression = 11;
  bool cron_enabled = 12;
  google.protobuf.Timestamp created_at = 13;
  google.protobuf.Timestamp updated_at = 14;
}

message CreateWorkflowRequest {
  string slug = 1;
  string name = 2;
  string description = 3;
  string command = 4;
  string target_directory = 5;
  string input_template = 6;
  string session_type = 7;
  string model = 8;
  string agent_type = 9;
  string cron_expression = 10;
  bool cron_enabled = 11;
}
message CreateWorkflowResponse { WorkflowProto workflow = 1; }

message UpdateWorkflowRequest {
  string id = 1;
  // All fields optional — only provided fields are updated.
  optional string name = 2;
  optional string description = 3;
  optional string command = 4;
  optional string target_directory = 5;
  optional string input_template = 6;
  optional string session_type = 7;
  optional string model = 8;
  optional string agent_type = 9;
  optional string cron_expression = 10;
  optional bool cron_enabled = 11;
}
message UpdateWorkflowResponse { WorkflowProto workflow = 1; }

message DeleteWorkflowRequest { string id = 1; }
message DeleteWorkflowResponse {}

message ListWorkflowsRequest {}
message ListWorkflowsResponse { repeated WorkflowProto workflows = 1; }

message RunWorkflowRequest {
  string id = 1;
  string arg = 2;  // injected into input_template if present
}
message RunWorkflowResponse { string session_id = 1; }
```

**Proto backward compatibility risk:** The existing `SessionService` client continues to work — all additions are new RPCs and new messages. No existing message fields are changed. Old clients simply don't call the new RPCs.

#### Task 2.1.2 — Run proto code generation

```bash
make generate-proto
```

This regenerates:
- `session/gen/session/v1/*.go` (Go bindings)
- `web-app/src/gen/session/v1/*_pb.ts` (TypeScript bindings)

Verify: `go build ./...` and `cd web-app && npx tsc --noEmit`

---

### Story 2.2 — RPC Handler Implementation

**Complexity: L**

#### Task 2.2.1 — Add WorkflowService struct

**Files to create:**
- `server/services/workflow_service.go`

```go
package services

import (
    "context"
    sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
    "github.com/tstapler/stapler-squad/session"
    "connectrpc.com/connect"
    "google.golang.org/protobuf/types/known/timestamppb"
)

type WorkflowService struct {
    repo      session.WorkflowRepository
    scheduler WorkflowSchedulerInterface
}

func NewWorkflowService(repo session.WorkflowRepository, scheduler WorkflowSchedulerInterface) *WorkflowService {
    return &WorkflowService{repo: repo, scheduler: scheduler}
}
```

Define `WorkflowSchedulerInterface` to avoid circular import (interface defined in `server/services/` so it can be used by `WorkflowService` without importing `server/workflows`):
```go
// In server/services/workflow_service.go
type WorkflowSchedulerInterface interface {
    Reload(ctx context.Context, workflow *ent.Workflow) error
    Remove(workflowID string) error
    // FireNow immediately fires a workflow job, returning the created session ID.
    // This allows RunWorkflow RPC to avoid a circular dep back to SessionService.
    FireNow(ctx context.Context, wf *ent.Workflow, arg string) (string, error)
}
```

`WorkflowScheduler` (in `server/workflows/`) satisfies this interface because it has `SessionServiceInterface` access.

Implement each handler:
- `CreateWorkflow`: validate slug, command, targetDirectory, cronExpression → `repo.Create()` → `scheduler.Reload()` if `cron_enabled` → return proto
- `UpdateWorkflow`: `repo.Update()` → `scheduler.Reload()` or `scheduler.Remove()` based on `cron_enabled` → return proto
- `DeleteWorkflow`: `repo.Delete()` → `scheduler.Remove()` → return empty
- `ListWorkflows`: `repo.ListAll()` → map to proto list
- `RunWorkflow`: `repo.GetByID()` → `scheduler.FireNow(ctx, wf, req.Msg.Arg)` → return session ID in response

**Key validation in CreateWorkflow:**
```go
if err := session.ValidateWorkflowSlug(req.Msg.Slug); err != nil {
    return nil, connect.NewError(connect.CodeInvalidArgument, err)
}
if req.Msg.Command == "" {
    return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("command is required"))
}
// targetDirectory is required per US-04 requirements
if req.Msg.TargetDirectory == "" {
    return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("target_directory is required"))
}
// Validate cron expression if provided (from Risk 4 mitigation)
if req.Msg.CronExpression != "" {
    if err := validateCronExpression(req.Msg.CronExpression); err != nil {
        return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid cron expression: %w", err))
    }
}
```

Same cron validation applies to `UpdateWorkflow` when `CronExpression` field is present in the update request.

`validateCronExpression` is defined in `server/workflows/scheduler.go`:
```go
func validateCronExpression(expr string) error {
    parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
    _, err := parser.Parse(expr)
    return err
}
```

`WorkflowService` gains access to this via a package-level `ValidateCronExpression` export from `server/workflows`.

**Files modified:**
- `server/services/session_service.go` — add `workflowSvc *WorkflowService` field; add delegating methods `CreateWorkflow`, `UpdateWorkflow`, `DeleteWorkflow`, `ListWorkflows`, `RunWorkflow` that call `s.workflowSvc.*`

#### Task 2.2.2 — Register RPCs on SessionService

**Files modified:**
- `server/services/session_service.go` — add 5 new method receivers that satisfy the updated `sessionv1connect.SessionServiceHandler` interface

The method signatures (generated by proto):
```go
func (s *SessionService) CreateWorkflow(ctx context.Context, req *connect.Request[sessionv1.CreateWorkflowRequest]) (*connect.Response[sessionv1.CreateWorkflowResponse], error)
func (s *SessionService) UpdateWorkflow(ctx context.Context, req *connect.Request[sessionv1.UpdateWorkflowRequest]) (*connect.Response[sessionv1.UpdateWorkflowResponse], error)
func (s *SessionService) DeleteWorkflow(ctx context.Context, req *connect.Request[sessionv1.DeleteWorkflowRequest]) (*connect.Response[sessionv1.DeleteWorkflowResponse], error)
func (s *SessionService) ListWorkflows(ctx context.Context, req *connect.Request[sessionv1.ListWorkflowsRequest]) (*connect.Response[sessionv1.ListWorkflowsResponse], error)
func (s *SessionService) RunWorkflow(ctx context.Context, req *connect.Request[sessionv1.RunWorkflowRequest]) (*connect.Response[sessionv1.RunWorkflowResponse], error)
```

Each delegates to `s.workflowSvc.*`.

#### Task 2.2.3 — Wire WorkflowService into SessionService construction

**Files modified:**
- `server/dependencies.go` — add `WorkflowScheduler *workflows.Scheduler` field
- `server/server.go` (or `server/wire.go`) — in the session service constructor call, pass `workflowSvc` built from `WorkflowService{repo: deps.WorkflowRepo, scheduler: deps.WorkflowScheduler}`

#### Task 2.2.4 — Converter helper: ent Workflow → WorkflowProto

**Files to create (or add to workflow_service.go):**
```go
func entWorkflowToProto(w *ent.Workflow) *sessionv1.WorkflowProto {
    return &sessionv1.WorkflowProto{
        Id:              w.ID.String(),
        Slug:            w.Slug,
        Name:            w.Name,
        Description:     w.Description,
        Command:         w.Command,
        TargetDirectory: w.TargetDirectory,
        InputTemplate:   w.InputTemplate,
        SessionType:     w.SessionType,
        Model:           w.Model,
        AgentType:       w.AgentType,
        CronExpression:  w.CronExpression,
        CronEnabled:     w.CronEnabled,
        CreatedAt:       timestamppb.New(w.CreatedAt),
        UpdatedAt:       timestamppb.New(w.UpdatedAt),
    }
}
```

#### Task 2.2.5 — Feature registry update (backend)

**Files modified:**
- `docs/registry/backend-features.json` — add entries for all 5 new RPCs

---

## Epic 3: Omnibar Integration

### Story 3.1 — WorkflowDetector

**Complexity: M**

#### Task 3.1.1 — Add `InputType.Workflow` enum value

**Files modified:**
- `web-app/src/lib/omnibar/types.ts`

Add `Workflow = "workflow"` to the `InputType` enum. Add entry to `INPUT_TYPE_INFO`:
```typescript
[InputType.Workflow]: {
  label: "Workflow",
  icon: "⚡",
  description: "Quick workflow invocation",
},
```

#### Task 3.1.2 — Create WorkflowDetector

**Files to create:**
- `web-app/src/lib/omnibar/detectors/WorkflowDetector.ts`

Implement per the research sketch (Section 7 of omnibar-architecture.md). Key points:
- `priority = 25`
- Regex: `/^@([a-zA-Z0-9_-]+)(?:\s+(.+))?$/`
- Case-insensitive slug matching
- Unknown slug → `confidence: 0.4`, `workflowFound: false` in metadata
- Known slug → `confidence: 1.0`, full metadata including `interpolatedPrompt`
- Constructor accepts `WorkflowEntry[]` (lean interface — only fields needed for detection)

**Files modified:**
- `web-app/src/lib/omnibar/detector.ts` — add `unregister(detector: Detector): void` method to `DetectorRegistry`; add `WorkflowEntry` interface export

Registration pattern: `WorkflowDetector` is NOT in `createDefaultRegistry()` — it is dynamically registered/unregistered by `OmnibarContext` when workflows load, using `getDefaultRegistry().register(detector)`.

**Test isolation (CONCERN-1 fix):** The `getDefaultRegistry()` singleton persists between Jest test runs. Add a `resetDefaultRegistry()` export (or `setDefaultRegistry(r: DetectorRegistry)`) to `detector.ts` for use in test `beforeEach`/`afterEach` blocks. All tests that mount `OmnibarContext` or call `getDefaultRegistry()` must reset the singleton:
```typescript
// In detector.ts
let _registry: DetectorRegistry | null = null;
export function resetDefaultRegistry(): void { _registry = null; }
```
Call `resetDefaultRegistry()` in `afterEach` in any test file that registers detectors.

#### Task 3.1.3 — Tests for WorkflowDetector

**Files to create:**
- `web-app/src/lib/omnibar/detectors/WorkflowDetector.test.ts`

Required test cases (assign IDs `T-UNIT-TS-NNN` from next available slot):
- `@known-slug` → `InputType.Workflow`, confidence 1.0, correct metadata
- `@known-slug https://example.com` → interpolated prompt populated
- `@unknown-slug` → `InputType.Workflow`, confidence 0.4, `workflowFound: false`
- `https://github.com/owner/repo` → `null` (does not claim GitHub URLs)
- `/path/to/dir` → `null` (does not claim local paths)
- `@path-with-spaces` trailing whitespace stripped
- Empty `@` (bare `@` with no slug) → `null`
- Case-insensitive: `@Knowledge-Sync` matches `knowledge-sync`

**Files modified:**
- `web-app/src/lib/omnibar/detector.test.ts` — add `WorkflowDetector` describe block to verify it does NOT fire on existing patterns (pitfall guard tests)

---

### Story 3.2 — `run_workflow` OmnibarAction

**Complexity: S**

#### Task 3.2.1 — Add `run_workflow` to OmnibarAction union

**Files modified:**
- `web-app/src/lib/omnibar/actions/types.ts`

```typescript
| { type: "run_workflow"; workflowSlug: string; workflowArg: string; label: string }
```

**Files modified:**
- `web-app/src/lib/omnibar/actions/dispatch.ts` — add `case "run_workflow":` with `runWorkflow` dep

```typescript
case "run_workflow":
  void deps.runWorkflow(action.workflowSlug, action.workflowArg);
  deps.close();
  return;
```

Add `runWorkflow?: (slug: string, arg: string) => void` to `ActionDeps` interface. **Mark it optional** (matching the `spawnShell?` pattern) to avoid a breaking change to all existing call sites. Document in the code that callers not providing `runWorkflow` will silently no-op on `run_workflow` actions.

After adding the optional field, audit all call sites of `dispatchOmnibarAction` to identify any that need `runWorkflow` wired in (specifically `OmnibarContext.tsx`). All other call sites (e.g., suggestion list item clicks that only generate session navigation/create actions) can leave it absent.

#### Task 3.2.2 — Tests for `run_workflow` dispatch

**Files modified:**
- `web-app/src/lib/omnibar/actions/dispatch.test.ts`

```typescript
describe("run_workflow", () => {
  it("dispatchOmnibarAction_should_callRunWorkflow_When_actionIsRunWorkflow", () => {
    // T-UNIT-TS-NNN
    const deps = { ...mockDeps, runWorkflow: jest.fn(), close: jest.fn() };
    dispatchOmnibarAction(
      { type: "run_workflow", workflowSlug: "my-wf", workflowArg: "https://x.com", label: "My WF" },
      deps
    );
    expect(deps.runWorkflow).toHaveBeenCalledWith("my-wf", "https://x.com");
    expect(deps.close).toHaveBeenCalled();
  });
});
```

---

### Story 3.3 — Omnibar UI Integration

**Complexity: L**

#### Task 3.3.1 — OmnibarContext: workflow fetch + `runWorkflow` handler

**Files modified:**
- `web-app/src/lib/contexts/OmnibarContext.tsx`

1. Add `useWorkflows()` hook call to fetch workflow list from backend.
2. Register `WorkflowDetector` dynamically on load:
   ```typescript
   useEffect(() => {
     if (!workflows.length) return;
     const detector = new WorkflowDetector(workflows.map(w => ({
       slug: w.slug,
       name: w.name,
       description: w.description,
       targetDirectory: w.targetDirectory,
       sessionType: w.sessionType as WorkflowEntry["sessionType"],
       inputTemplate: w.inputTemplate,
     })));
     const registry = getDefaultRegistry();
     registry.register(detector);
     return () => registry.unregister(detector);
   }, [workflows]);
   ```
3. Implement `runWorkflow(slug: string, arg: string)`:
   - Look up workflow by slug from loaded list
   - Build `OmnibarSessionData`: `path = workflow.targetDirectory`, `sessionType`, `oneOff = sessionType === "one_off"`, `initialPrompt = interpolated template`, `program = workflow.agentType || defaultProgram`
   - Call `createSession(data)`
4. Pass `runWorkflow` as dep to `dispatchOmnibarAction` calls.

#### Task 3.3.2 — Omnibar.tsx: handle `InputType.Workflow` in detection effect

**Files modified:**
- `web-app/src/components/sessions/Omnibar.tsx`

In the detection effect (debounced `detect()` call), add handling for `InputType.Workflow`:
```typescript
if (result.type === InputType.Workflow && result.metadata?.workflowFound) {
  const wf = result.metadata.workflow as WorkflowEntry;
  setFormField("sessionType", wf.sessionType);
  setFormField("workingDir", wf.targetDirectory);
  setFormField("firstPrompt", result.metadata.interpolatedPrompt as string);
  setFormField("sessionName", result.suggestedName);
  dispatchMode({ type: "SET_CREATION_MODE" });
}
```

For `workflowFound: false` (unknown slug), stay in discovery mode and show "no workflow found" in results.

#### Task 3.3.3 — OmnibarResultList: Workflow suggestions section

**Files modified:**
- `web-app/src/components/sessions/OmnibarResultList.tsx`

When input starts with `@`, fetch matching workflows from the loaded list and render a "Workflows" section above session results. Each workflow item shows `@slug`, workflow name, description, and command preview. Clicking dispatches `run_workflow` action.

Pass `workflowResults: WorkflowEntry[]` prop (filtered by prefix match). Empty if input doesn't start with `@`.

---

## Epic 4: Workflow Management UI + Cron Scheduler

### Story 4.1 — WorkflowScheduler

**Complexity: M**

#### Task 4.1.1 — Create WorkflowScheduler

**Files to create:**
- `server/workflows/scheduler.go`

```go
package workflows

import (
    "context"
    "fmt"
    "sync"
    "time"

    "github.com/robfig/cron/v3"
    "github.com/tstapler/stapler-squad/server/events"
    "github.com/tstapler/stapler-squad/session"
    "github.com/tstapler/stapler-squad/session/ent"
)

type Scheduler struct {
    c          *cron.Cron
    repo       session.WorkflowRepository
    sessionSvc SessionServiceInterface
    eventBus   *events.EventBus
    mu         sync.Mutex
    entryMap   map[string]cron.EntryID // workflowID → cron.EntryID
}

// Start loads all enabled workflows and begins cron processing.
// Stops when ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) { ... }

// Reload registers or re-registers a workflow's cron job.
// Called after create/update.
func (s *Scheduler) Reload(ctx context.Context, wf *ent.Workflow) error { ... }

// Remove removes a workflow's cron job by workflow ID.
// Called after delete or when cron_enabled=false.
func (s *Scheduler) Remove(workflowID string) error { ... }

// FireNow immediately fires a workflow outside of cron schedule.
// Returns the created session ID. Used by RunWorkflow RPC and internal cron trigger.
func (s *Scheduler) FireNow(ctx context.Context, wf *ent.Workflow, arg string) (string, error) {
    prompt := wf.InputTemplate
    if prompt != "" && arg != "" {
        prompt = strings.ReplaceAll(prompt, "{{input}}", arg)
    } else if prompt == "" {
        prompt = arg
    }
    title := fmt.Sprintf("%s — %s", wf.Name, time.Now().Format("2006-01-02 15:04"))
    oneOff := wf.SessionType == "one_off"
    req := connect.NewRequest(&sessionv1.CreateSessionRequest{
        Title:         title,
        Path:          wf.TargetDirectory,
        Program:       wf.AgentType,
        InitialPrompt: prompt,
        OneOff:        oneOff,
    })
    resp, err := s.sessionSvc.CreateSession(ctx, req)
    if err != nil {
        s.publishError(wf, err)
        return "", err
    }
    s.publishInfo(wf, title)
    return resp.Msg.SessionId, nil
}

// Stop halts the cron engine (shutdown hook).
func (s *Scheduler) Stop() { ... }
```

Key implementation details:
- `Start`: calls `repo.ListEnabled(ctx)`, iterates to `c.AddFunc(wf.CronExpression, jobFn)`, stores entryID in `entryMap[wf.ID.String()]`; the cron job function calls `s.FireNow(ctx, wf, "")`
- `Reload`: `mu.Lock()`, `c.Remove(existingID)` if present, `c.AddFunc(...)` if `cron_enabled=true`, update `entryMap`
- `fireWorkflow` is replaced by `FireNow` — single implementation for both cron-triggered and manual runs

Define `SessionServiceInterface` in the same package to avoid circular import:
```go
type SessionServiceInterface interface {
    CreateSession(ctx context.Context, req *connect.Request[sessionv1.CreateSessionRequest]) (*connect.Response[sessionv1.CreateSessionResponse], error)
}
```

#### Task 4.1.2 — Add `go get` for cron library

```bash
go get github.com/robfig/cron/v3@latest
```

**Files modified:** `go.mod`, `go.sum`

#### Task 4.1.3 — Wire WorkflowScheduler into server

**Files modified:**
- `server/dependencies.go` — add `WorkflowScheduler *workflows.Scheduler` to BOTH `ServerDependencies` struct AND `RuntimeDeps` struct; update `RuntimeDeps.ToServerDeps()` to map the new field. **Critical:** omitting `ToServerDeps()` update causes nil scheduler in Warren-lifecycle deployments → nil pointer panic on `WorkflowScheduler.Start`.
- `server/server.go` (or `server/wire.go`) — instantiate scheduler in `BuildCoreDeps` AFTER `SessionService` is initialized:
  ```go
  // scheduler must be constructed after sessionSvc so it can receive the interface
  workflowScheduler := workflows.NewScheduler(deps.WorkflowRepo, sessionSvc, deps.EventBus)
  deps.WorkflowScheduler = workflowScheduler
  // Also inject scheduler back into workflowSvc so RunWorkflow can call FireNow:
  deps.WorkflowScheduler is passed into NewWorkflowService
  ```
  In `wireDepsIntoServer`:
  ```go
  if deps.WorkflowScheduler != nil {
      deps.WorkflowScheduler.Start(serverCtx)
      srv.shutdownHooks = append(srv.shutdownHooks, deps.WorkflowScheduler.Stop)
  }
  ```

**Initialization order matters:** `SessionService` → `WorkflowScheduler` → `WorkflowService`. Specifically:
1. Build `sessionSvc *SessionService` (needs `WorkflowRepo` and `WorkflowScheduler`)
2. Build `workflowScheduler *workflows.Scheduler` (needs `sessionSvc` as `SessionServiceInterface`)
3. Build `workflowSvc *WorkflowService` (needs `WorkflowRepo` + `workflowScheduler`)
4. Set `sessionSvc.workflowSvc = workflowSvc`

To avoid the bootstrapping issue in step 1 (SessionService needs WorkflowScheduler before it's created), pass `workflowSvc` into `SessionService` via a setter after both are constructed:
```go
sessionSvc := NewSessionService(/* core deps without workflowSvc */)
workflowScheduler := workflows.NewScheduler(workflowRepo, sessionSvc, eventBus)
workflowSvc := NewWorkflowService(workflowRepo, workflowScheduler)
sessionSvc.SetWorkflowService(workflowSvc)  // deferred injection
```

Add `SetWorkflowService(svc *WorkflowService)` to `SessionService` for this deferred injection pattern.

---

### Story 4.2 — Navigation + Route

**Complexity: S**

#### Task 4.2.1 — Add route constant

**Files modified:**
- `web-app/src/lib/routes.ts`

```typescript
workflows: "/workflows",
```

#### Task 4.2.2 — Add nav entry

**Files modified:**
- `web-app/src/lib/nav-pages.ts`

```typescript
import { Zap } from "lucide-react";

// After the `rules` entry (secondary section):
{ href: routes.workflows, label: "Workflows", icon: Zap, mobileNav: false, headerNav: false },
```

`mobileNav: false` — workflows management is desktop-only (not a primary mobile workflow). `headerNav: false` — in drawer/hamburger only.

#### Task 4.2.3 — Create page route

**Files to create:**
- `web-app/src/app/workflows/layout.tsx` — metadata wrapper (title: "Workflows - Stapler Squad")
- `web-app/src/app/workflows/page.tsx` — `"use client"` page with `// +feature: workflows-management`, Suspense boundary wrapping `<WorkflowsPanel />`
- `web-app/src/app/workflows/page.css.ts` — vanilla-extract page/main layout (copy from `rules/page.css.ts` pattern, `maxWidth: "960px"`)

---

### Story 4.3 — WorkflowsPanel + WorkflowForm Components

**Complexity: L**

#### Task 4.3.1 — Create useWorkflows hook

**Files to create:**
- `web-app/src/lib/hooks/useWorkflows.ts`

Follows `useApprovalRules.ts` pattern:
- `clientRef` via `getConnectTransport()` singleton
- `fetchWorkflows()` called on mount
- `createWorkflow(data)`, `updateWorkflow(id, data)`, `deleteWorkflow(id)` as `useCallback`s
- Optimistic delete (local state update), re-fetch after create/update
- `loading: boolean`, `error: Error | null` state
- Returns: `{ workflows, loading, error, createWorkflow, updateWorkflow, deleteWorkflow, refresh }`

TypeScript types: `WorkflowFormData` (maps to `CreateWorkflowRequest` fields, all `string | boolean`).

#### Task 4.3.2 — Create WorkflowForm component

**Files to create:**
- `web-app/src/components/workflows/WorkflowForm.tsx`
- `web-app/src/components/workflows/WorkflowForm.css.ts`

Fields (in order):
1. **Slug** — `Input` component, monospace hint: "used in omnibar trigger: @slug"
2. **Name** — `Input` component
3. **Description** — native `<textarea>` (no shared Textarea component)
4. **Command** — `Input` component, hint: "slash command or skill, e.g. /knowledge:synthesize"
5. **Target Directory** — `Input` component, hint: absolute path
6. **Input Template** — native `<textarea>`, hint: "use {{input}} for omnibar argument"
7. **Session Type** — native `<select>` with options: directory, one_off, new_worktree, existing_worktree
8. **Model** — `Input` component, optional, placeholder: "leave blank for default"
9. **Agent Type** — `Input` component, optional
10. **Cron Expression** — `Input` component, optional, hint: "5-field cron, e.g. 0 8 * * 1"
11. **Cron Enabled** — `<input type="checkbox">`

Footer: `<Button intent="secondary">Cancel</Button>` + `<Button intent="primary">Save</Button>`

Edit mode: pre-fill all fields from existing workflow. Slug is read-only in edit mode (changing slug would break existing bookmarks/cron references; document this).

**Props:**
```typescript
interface WorkflowFormProps {
  workflow?: WorkflowProto;      // undefined = create mode
  onSave: (data: WorkflowFormData) => Promise<void>;
  onCancel: () => void;
  saving?: boolean;
  error?: string;
}
```

**CSS:** Use `WorkflowForm.css.ts` with vanilla-extract. Import `vars` from `@/styles/theme.css`. No hardcoded colors or z-indices.

#### Task 4.3.3 — Create WorkflowsPanel component

**Files to create:**
- `web-app/src/components/workflows/WorkflowsPanel.tsx`
- `web-app/src/components/workflows/WorkflowsPanel.css.ts`

Add `// +feature: workflows-management` in first 10 lines.

Layout:
- **Header**: "Workflows" title + "New Workflow" button (right-aligned)
- **List mode** (default): sorted alphabetically by `name`, each workflow as a `Card` with:
  - Slug badge (`@slug`)
  - Name (bold) + description (muted)
  - Command chip
  - Schedule chip (if `cron_enabled`: shows cron expression; else: "Manual only")
  - Edit + Delete buttons
- **Create/Edit mode**: renders `WorkflowForm` inline (or modal — inline preferred for discoverability)
- **Empty state**: centered message "No workflows yet. Create one to get started."
- **Delete confirmation**: use `Modal` with `ModalContent` from `@/components/ui` (always `createPortal` pattern — avoid `position: fixed` without portal)

State: `{ mode: "list" | "create" | "edit", editingId: string | null }`

Use `useWorkflows()` for all data operations. Show `Skeleton` during loading.

#### Task 4.3.4 — Feature registry update (frontend)

**Files modified:**
- `docs/registry/frontend-features.json` — add:
  ```json
  {
    "id": "workflows-management",
    "type": "frontend",
    "component": "WorkflowsPanel",
    "path": "web-app/src/components/workflows/WorkflowsPanel.tsx",
    "tested": false,
    "testIds": []
  }
  ```
- `docs/registry/backend-features.json` — set `"tested": true` and add test IDs after Go tests are written

---

### Story 4.4 — Tests + E2E

**Complexity: M**

#### Task 4.4.1 — Go unit tests for WorkflowService

**Files to create:**
- `server/services/workflow_service_test.go`

Test cases:
- `TestCreateWorkflow_ValidInput` — happy path, returns WorkflowProto
- `TestCreateWorkflow_DuplicateSlug` — returns `CodeAlreadyExists`
- `TestCreateWorkflow_InvalidSlug` — returns `CodeInvalidArgument`
- `TestUpdateWorkflow_NotFound` — returns `CodeNotFound`
- `TestDeleteWorkflow_TriggersSchedulerRemove` — verifies scheduler interface called
- `TestRunWorkflow_InjectsArg` — verifies `initialPrompt` is interpolated correctly

#### Task 4.4.2 — Go unit tests for WorkflowScheduler

**Files to create:**
- `server/workflows/scheduler_test.go`

Test cases:
- `TestScheduler_Start_LoadsEnabledWorkflows` — mocked repo, verifies cron entries created
- `TestScheduler_Reload_UpdatesExistingEntry` — verifies old entry removed and new one added
- `TestScheduler_Remove_DeletesEntry` — verifies entry removed from cron
- `TestScheduler_FireWorkflow_PublishesNotification` — verifies event bus called

#### Task 4.4.3 — Frontend Jest tests for useWorkflows

**Files to create:**
- `web-app/src/lib/hooks/useWorkflows.test.ts`

Test cases using `msw` or jest mock for ConnectRPC client:
- Fetch on mount, returns sorted list
- Create triggers re-fetch
- Delete performs optimistic update
- Error state on fetch failure

#### Task 4.4.4 — E2E Playwright test

**Files to create:**
- `tests/e2e/workflows.spec.ts`

Required:
- `// @feature workflows-management` in first line
- No `waitForTimeout`; use ARIA roles and `data-testid`
- Test cases:
  - `workflows_should_showEmptyState_When_noWorkflows`
  - `workflows_should_createWorkflow_When_formSubmitted`
  - `workflows_should_deleteWorkflow_When_deleteConfirmed`
  - `workflows_should_invokeFromOmnibar_When_atSlugTyped` — **requires `beforeEach` fixture**: use the management panel to create a workflow (or call `createWorkflow` RPC) before testing omnibar invocation; the detector needs a real workflow in the loaded list to return `workflowFound: true`

---

## Migration Safety

### Ent schema migration

`client.Schema.Create(ctx)` creates the `workflows` table automatically on first startup after upgrade. No manual SQL or migration scripts needed. The new `workflows` table is purely additive — no existing tables are touched.

**Risk: None for additive migration.** If a column is ever renamed or removed, follow the pattern in `session/migrate.go`.

### Proto backward compatibility

All changes are additive: new RPCs, new message types, new field numbers. No existing proto messages are modified. Old Go/TypeScript clients compiled against the old proto continue to work; they simply don't call the new RPCs.

**Risk: Low.** Only concern is field number exhaustion if the proto already uses high numbers. Verify the highest existing field number in `CreateSessionRequest` before assigning new numbers to `WorkflowProto`.

---

## Risks

### Risk 1 (RESOLVED): Circular import — WorkflowScheduler ↔ SessionService

Two circular dependency scenarios exist and both are resolved:

**Scenario A** (`WorkflowScheduler` → `SessionService`): `WorkflowScheduler` (in `server/workflows/`) defines `SessionServiceInterface` locally. `SessionService` satisfies it structurally. No import of `server/services` from `server/workflows`.

**Scenario B** (`RunWorkflow` → `SessionService` → `WorkflowService` → back to `SessionService`): Resolved by `RunWorkflow` delegating to `scheduler.FireNow()` instead of calling `sessionSvc.CreateSession()` directly. `WorkflowService` only depends on `WorkflowSchedulerInterface` (defined in `server/services/workflow_service.go`) and `WorkflowRepository`.

**Bootstrapping order** (deferred injection pattern):
1. Build `sessionSvc` (without workflowSvc)
2. Build `workflowScheduler` (with sessionSvc as interface)
3. Build `workflowSvc` (with workflowRepo + workflowScheduler)
4. `sessionSvc.SetWorkflowService(workflowSvc)` — deferred setter injection

### Risk 2 (MEDIUM): Ent slug uniqueness error surfacing

When `CreateWorkflow` fails due to duplicate slug, ent returns `*ent.ConstraintError`. The handler must explicitly check `ent.IsConstraintError(err)` and return `connect.CodeAlreadyExists`, otherwise the caller gets an opaque 500.

```go
if ent.IsConstraintError(err) {
    return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("workflow with slug %q already exists", req.Msg.Slug))
}
```

### Risk 3 (MEDIUM): WorkflowDetector dynamic registration in singleton registry

`getDefaultRegistry()` returns a singleton `DetectorRegistry`. Registering and unregistering `WorkflowDetector` on it from React effects is safe as long as `register`/`unregister` are synchronous (they are — just array insert/remove). However, if the omnibar is mounted multiple times simultaneously (modal reopens quickly), a second registration may fire before the first cleanup runs.

**Mitigation:** The `useEffect` cleanup unregisters before re-running. React guarantees cleanup before the next effect. Document this assumption in code.

### Risk 4 (LOW): Cron expression validation

`robfig/cron/v3` accepts 5-field expressions. If the user provides a 6-field expression (with seconds), `c.AddFunc` will fail silently or return an error. The `RunWorkflow` RPC handler should validate the expression at create/update time using `cron.New().AddFunc(expr, func(){})` to catch parse errors before they reach the scheduler.

**Mitigation:** Add `validateCronExpression(expr string) error` utility in `server/workflows/scheduler.go`:
```go
func validateCronExpression(expr string) error {
    _, err := cron.NewParser(cron.Minute|cron.Hour|cron.Dom|cron.Month|cron.Dow).Parse(expr)
    return err
}
```
Call from `CreateWorkflow`/`UpdateWorkflow` handlers when `cron_expression` is non-empty.

### Risk 5 (LOW): Omnibar detector priority ordering

Research confirmed `@slug` does not conflict with any existing detector. However, if a future detector is added at priority 20–30 that could claim `@`-prefixed inputs, the ordering would need review.

**Mitigation:** Document the priority slot in the `DetectorRegistry` registration call with a comment explaining the ordering rationale.

### Risk 5b (HIGH): `RuntimeDeps.ToServerDeps()` must be updated for both new fields

`server/dependencies.go` has a `RuntimeDeps.ToServerDeps()` conversion function that manually maps every field. Any new fields added to `ServerDependencies` but not added to `ToServerDeps()` will be `nil` in Warren-lifecycle deployments.

**Mitigation:** Tasks 1.2.3 and 4.1.3 explicitly require updating `ToServerDeps()`. Add a CI linting check or comment in `ToServerDeps()` that reminds maintainers to keep it in sync. Post-implementation: verify by grepping for the new field names in `ToServerDeps`.

### Risk 6 (LOW): Missed-fire policy

`robfig/cron` does not backfill missed fires during downtime. If the server was offline when a scheduled workflow was supposed to fire, the run is silently skipped. This is acceptable for v1.

**Document as non-goal** in PR description.

---

## File Summary

### New files

| File | Epic/Story |
|------|-----------|
| `session/ent/schema/workflow.go` | E1/S1.1 |
| `session/workflow_repository.go` | E1/S1.2 |
| `session/ent_workflow_repository.go` | E1/S1.2 |
| `session/workflow_slug.go` | E1/S1.3 |
| `server/services/workflow_service.go` | E2/S2.2 |
| `server/workflows/scheduler.go` | E4/S4.1 |
| `server/workflows/scheduler_test.go` | E4/S4.4 |
| `server/services/workflow_service_test.go` | E4/S4.4 |
| `web-app/src/lib/omnibar/detectors/WorkflowDetector.ts` | E3/S3.1 |
| `web-app/src/lib/omnibar/detectors/WorkflowDetector.test.ts` | E3/S3.1 |
| `web-app/src/lib/hooks/useWorkflows.ts` | E4/S4.3 |
| `web-app/src/lib/hooks/useWorkflows.test.ts` | E4/S4.4 |
| `web-app/src/app/workflows/layout.tsx` | E4/S4.2 |
| `web-app/src/app/workflows/page.tsx` | E4/S4.2 |
| `web-app/src/app/workflows/page.css.ts` | E4/S4.2 |
| `web-app/src/components/workflows/WorkflowsPanel.tsx` | E4/S4.3 |
| `web-app/src/components/workflows/WorkflowsPanel.css.ts` | E4/S4.3 |
| `web-app/src/components/workflows/WorkflowForm.tsx` | E4/S4.3 |
| `web-app/src/components/workflows/WorkflowForm.css.ts` | E4/S4.3 |
| `tests/e2e/workflows.spec.ts` | E4/S4.4 |
| (auto-generated ent files) | E1/S1.1 |

### Modified files

| File | Epic/Story | Change |
|------|-----------|--------|
| `proto/session/v1/session.proto` | E2/S2.1 | +5 RPCs, +7 messages |
| `server/services/session_service.go` | E2/S2.2 | +5 method delegates, +workflowSvc field |
| `server/dependencies.go` | E1/S1.2, E4/S4.1 | +WorkflowRepo, +WorkflowScheduler fields |
| `server/server.go` | E1/S1.2, E4/S4.1 | wire up repo + scheduler |
| `web-app/src/lib/omnibar/types.ts` | E3/S3.1 | +InputType.Workflow |
| `web-app/src/lib/omnibar/detector.ts` | E3/S3.1 | +unregister() method |
| `web-app/src/lib/omnibar/actions/types.ts` | E3/S3.2 | +run_workflow variant |
| `web-app/src/lib/omnibar/actions/dispatch.ts` | E3/S3.2 | +run_workflow case, +runWorkflow dep |
| `web-app/src/lib/omnibar/actions/dispatch.test.ts` | E3/S3.2 | +run_workflow test |
| `web-app/src/lib/omnibar/detector.test.ts` | E3/S3.1 | +pitfall guard tests |
| `web-app/src/components/sessions/Omnibar.tsx` | E3/S3.3 | +InputType.Workflow detection |
| `web-app/src/lib/contexts/OmnibarContext.tsx` | E3/S3.3 | +runWorkflow handler, workflow fetch, detector reg |
| `web-app/src/components/sessions/OmnibarResultList.tsx` | E3/S3.3 | +workflowResults section |
| `web-app/src/lib/routes.ts` | E4/S4.2 | +workflows route |
| `web-app/src/lib/nav-pages.ts` | E4/S4.2 | +Workflows nav entry |
| `docs/registry/backend-features.json` | E2/S2.2 | +5 RPC entries |
| `docs/registry/frontend-features.json` | E4/S4.3 | +workflows-management entry |
| `go.mod`, `go.sum` | E4/S4.1 | +robfig/cron/v3 |

---

## Session Creation Mode Registry

The `run_workflow` action creates sessions through the existing `create_session` / `one_off` paths. It does NOT introduce a new `SESSION_TYPE_WORKFLOW` proto enum value. Therefore, the 7-touchpoint session creation registry is NOT triggered for this feature. The workflow is treated as a configuration layer on top of existing session types (`directory`, `one_off`, etc.).

If a future version adds a `workflow_run` session type for tracking purposes, the 7-touchpoint registry must be followed at that time.

---

## Recommended Implementation Order

1. **E1 → E2 → E4/S4.1 → E3 → E4/S4.2-4.4**
2. Concurrency note: E3 (omnibar) and E4/S4.2+ (UI panel) can be developed in parallel after E2 is complete (proto generated, RPCs stubbed)
3. Do NOT merge E3 omnibar changes before detector tests pass — risk of breaking existing omnibar behavior
4. Do NOT merge scheduler code until `WorkflowSchedulerInterface` is resolved (circular import risk)
