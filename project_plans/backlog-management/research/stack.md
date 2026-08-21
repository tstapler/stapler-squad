# Stack Research: Backlog Management Layer

**Date**: 2026-05-10  
**Scope**: Existing patterns in `session/ent/schema/`, `server/services/`, `proto/session/v1/`, `server/mcp/`

---

## Summary

The project is a Go web server (ConnectRPC over HTTP, React SPA frontend) with SQLite storage via entgo ORM. The MCP server is fully implemented in-process at `server/mcp/` using `github.com/mark3labs/mcp-go`. New services follow a well-worn three-layer pattern: ent schema → Go service struct + Storage methods → proto definition → server registration. Proto generation uses `buf generate proto` (not `make generate-proto`); the correct Makefile target is `proto-gen`.

---

## Ent ORM Patterns

All schemas live in `session/ent/schema/` as Go files with a `package schema` declaration. Every entity follows this structure:

```go
type BacklogItem struct{ ent.Schema }

func (BacklogItem) Fields() []ent.Field { ... }
func (BacklogItem) Edges()  []ent.Edge  { ... }
func (BacklogItem) Indexes() []ent.Index { ... }
```

### Field conventions observed

| Pattern | Example |
|---|---|
| String unique identifier | `field.String("name").Unique().NotEmpty()` |
| Integer status/enum (stored as `int`) | `field.Int("status").Comment("...")` |
| Optional string | `field.String("branch").Optional()` |
| Immutable created_at | `field.Time("created_at").Default(time.Now).Immutable()` |
| Auto-updating updated_at | `field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now)` |
| Boolean with default | `field.Bool("enabled").Default(true)` |
| Nillable timestamp (nullable) | `field.Time("last_viewed").Optional().Nillable()` |
| Text blob / long string | Use `field.String(...)` — no TEXT type distinction |

### Status state machine pattern

The project stores status as `field.Int("status")` (not a native ent enum). The mapping between integer constants and named states is maintained in Go code (see `session/instance.go` `SessionStatus` constants). For backlog items, the same pattern applies:

```go
// In the schema:
field.Int("status").Comment("BacklogItemStatus: idea=0, ready=1, in_progress=2, review=3, done=4, archived=5")

// In session/backlog_item.go (domain layer):
type BacklogItemStatus int
const (
    BacklogItemStatusIdea       BacklogItemStatus = 0
    BacklogItemStatusReady      BacklogItemStatus = 1
    BacklogItemStatusInProgress BacklogItemStatus = 2
    BacklogItemStatusReview     BacklogItemStatus = 3
    BacklogItemStatusDone       BacklogItemStatus = 4
    BacklogItemStatusArchived   BacklogItemStatus = 5
)
```

### Edge patterns

| Relationship | Pattern |
|---|---|
| One-to-many (owner side) | `edge.To("sessions", Session.Type)` in `Project` |
| Many-to-one (back-reference) | `edge.From("project", Project.Type).Ref("sessions").Unique()` in `Session` |
| Many-to-many | `edge.To("tags", Tag.Type)` in `Session`; `edge.From("sessions", Session.Type).Ref("tags")` in `Tag` |
| One-to-one | `edge.To("worktree", Worktree.Type).Unique()` |

For `BacklogItem ↔ Session` (many-to-many, one item has multiple attempt sessions), model it as:
```go
// In BacklogItem schema:
edge.To("sessions", Session.Type)
// In Session schema (add back-ref):
edge.From("backlog_items", BacklogItem.Type).Ref("sessions")
```

### Generate command (CRITICAL)

The `--feature sql/upsert` flag is required (documented in CLAUDE.md):

```bash
go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
```

---

## ConnectRPC Service Patterns

### Service file structure

`server/services/project_service.go` is the cleanest reference implementation (122 lines, CRUD only). Pattern:

```go
package services

import (
    "context"
    sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
    "connectrpc.com/connect"
    "google.golang.org/protobuf/types/known/timestamppb"
)

type BacklogService struct {
    storage *session.Storage
}

func NewBacklogService(storage *session.Storage) *BacklogService {
    return &BacklogService{storage: storage}
}

func (s *BacklogService) CreateBacklogItem(
    ctx context.Context,
    req *connect.Request[sessionv1.CreateBacklogItemRequest],
) (*connect.Response[sessionv1.CreateBacklogItemResponse], error) {
    if s.storage == nil {
        return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
    }
    // validation...
    // storage call...
    return connect.NewResponse(&sessionv1.CreateBacklogItemResponse{Item: proto}), nil
}
```

Key conventions:
- Guard `s.storage == nil` at top of every method (test environments have no storage).
- Use `connect.NewError(connect.Code..., fmt.Errorf("..."))` — never return raw errors.
- Domain data structs live in `session/` package (e.g., `session.ProjectData`).
- Proto ↔ domain conversion via a package-private `xxxToProto()` function.
- Timestamps always use `timestamppb.New(t)`.

### Service registration in `server/server.go`

Two-step: generate the handler then register it:

```go
// In server/server.go (inside registerRoutes or equivalent):
path, handler := sessionv1connect.NewBacklogServiceHandler(deps.BacklogService, ConnectOptions(deps.ErrorRegistry)...)
srv.RegisterConnectHandler("/api"+path, http.StripPrefix("/api", handler))
```

The generated `New<Service>Handler` function comes from the buf-generated `sessionv1connect` package at `gen/proto/go/session/v1/sessionv1connect/`. The service must be threaded through the `Deps` struct (defined in `server/server.go` or a deps file) before it can be passed here.

### Deps wiring pattern

Services are instantiated upstream (likely in `main.go` or a factory) and passed as a `Deps` struct field. Look for the existing `Deps` struct to add `BacklogService *services.BacklogService`.

---

## Proto Conventions

### File and package naming

| Item | Convention |
|---|---|
| Package | `session.v1` (single package for all session-domain types) |
| File naming | domain-noun.proto (e.g., `session.proto`, `types.proto`, `unfinished.proto`) |
| Service name suffix | `Service` enforced by buf lint (`service_suffix: Service`) |
| Enum zero value | `_UNSPECIFIED` suffix enforced by buf lint |
| Request/Response | `Create<Entity>Request` / `Create<Entity>Response` |

### Recommended new file

Create `proto/session/v1/backlog.proto` (same package `session.v1`, same directory). This keeps all types in one generated Go package (`sessionv1`) and one TypeScript package (`web-app/src/gen/session/v1/`).

### Sample service stub

```protobuf
syntax = "proto3";
package session.v1;

import "google/protobuf/timestamp.proto";
import "session/v1/types.proto";

service BacklogService {
  rpc CreateBacklogItem(CreateBacklogItemRequest) returns (CreateBacklogItemResponse) {}
  rpc ListBacklogItems(ListBacklogItemsRequest)   returns (ListBacklogItemsResponse)  {}
  rpc UpdateBacklogItem(UpdateBacklogItemRequest) returns (UpdateBacklogItemResponse) {}
  rpc DeleteBacklogItem(DeleteBacklogItemRequest) returns (DeleteBacklogItemResponse) {}
  rpc GetBacklogItem(GetBacklogItemRequest)       returns (GetBacklogItemResponse)    {}
  rpc SpawnSessionFromItem(SpawnSessionFromItemRequest) returns (SpawnSessionFromItemResponse) {}
}

enum BacklogItemStatus {
  BACKLOG_ITEM_STATUS_UNSPECIFIED = 0;
  BACKLOG_ITEM_STATUS_IDEA        = 1;
  BACKLOG_ITEM_STATUS_READY       = 2;
  BACKLOG_ITEM_STATUS_IN_PROGRESS = 3;
  BACKLOG_ITEM_STATUS_REVIEW      = 4;
  BACKLOG_ITEM_STATUS_DONE        = 5;
  BACKLOG_ITEM_STATUS_ARCHIVED    = 6;
}
```

### Imports

- `import "google/protobuf/timestamp.proto"` — for all timestamp fields
- `import "session/v1/types.proto"` — to reference existing `SessionType`, `SessionStatus`, etc.
- buf resolves `googleapis` via `buf.build/googleapis/googleapis` dep in `buf.yaml`

---

## MCP Integration

### Architecture: in-process HTTP transport (already implemented)

The MCP server is **not** a separate process or project plan. It is fully implemented in `server/mcp/` and already running at `http://localhost:8543/mcp`. It uses the `github.com/mark3labs/mcp-go` library.

Key files:
- `server/mcp/server.go` — `NewCore()` wires all tool groups; `NewHTTPHandler()` mounts it on `/mcp`
- `server/mcp/tools_lifecycle.go` — session lifecycle tools (create, pause, resume, stop, update)
- `server/mcp/tools_discovery.go` — list/get/search sessions
- `server/mcp/tools_terminal.go` — PTY read/write
- `server/mcp/tools_vcs.go` — git operations
- `server/mcp/types.go` — shared result structs (`MCPResult`, `MCPError`, `SessionSummary`)

### Tool registration pattern

```go
// In server/mcp/tools_backlog.go (new file):
type backlogHandlers struct {
    store   session.InstanceStore
    storage *session.Storage  // for ent queries
}

func registerBacklogTools(s *mcpserver.MCPServer, bh *backlogHandlers) {
    s.AddTool(
        mcpgo.NewTool("get_backlog_item",
            mcpgo.WithDescription("Get full context for a backlog item by ID."),
            mcpgo.WithString("item_id", mcpgo.Description("BacklogItem ID"), mcpgo.Required()),
        ),
        bh.getBacklogItem,
    )
    s.AddTool(
        mcpgo.NewTool("report_progress",
            mcpgo.WithDescription("Report progress against an acceptance criteria item."),
            mcpgo.WithString("item_id", mcpgo.Required()),
            mcpgo.WithNumber("criteria_index", mcpgo.Required()),
            mcpgo.WithString("status", mcpgo.Enum("pass", "fail", "in_progress"), mcpgo.Required()),
            mcpgo.WithString("note"),
        ),
        bh.reportProgress,
    )
    s.AddTool(
        mcpgo.NewTool("request_review",
            mcpgo.WithDescription("Pause agent work and request human review."),
            mcpgo.WithString("item_id", mcpgo.Required()),
            mcpgo.WithString("message", mcpgo.Required()),
        ),
        bh.requestReview,
    )
}
```

Tool handlers return `([]mcpgo.Content, error)` — serialize response as JSON text content:
```go
func (bh *backlogHandlers) getBacklogItem(ctx context.Context, req mcpgo.CallToolRequest) ([]mcpgo.Content, error) {
    result := GetBacklogItemResult{MCPResult: MCPResult{Success: true}, Item: &item}
    b, _ := json.Marshal(result)
    return []mcpgo.Content{mcpgo.NewTextContent(string(b))}, nil
}
```

Register in `NewCore()` in `server/mcp/server.go`:
```go
registerBacklogTools(s, &backlogHandlers{store: store, storage: storage})
```

`NewCore` signature will need `storage *session.Storage` added as a parameter (currently takes `store session.InstanceStore`, `svc *services.SessionService`, `sbMgr *scrollback.ScrollbackManager`).

### No separate MCP server project plan

The `project_plans/stapler-squad-mcp-server/` directory does not exist — it was referenced as a future plan in the requirements but the implementation already lives in the main repo.

---

## Proto/Generate Pipeline

### Tools and config

| Item | Details |
|---|---|
| Generator | `buf` CLI (`buf generate proto`) |
| Config | `buf.yaml` at repo root (v2 format, `modules: [{path: proto}]`) |
| Plugin config | `buf.gen.yaml` at repo root |
| Go output | `gen/proto/go/session/v1/` (messages) + `gen/proto/go/session/v1/sessionv1connect/` (ConnectRPC stubs) |
| TypeScript output | `web-app/src/gen/session/v1/` |

### Plugins (from buf.gen.yaml)

1. `buf.build/protocolbuffers/go` → Go message types at `gen/proto/go`
2. `buf.build/connectrpc/go` → Go ConnectRPC service stubs at `gen/proto/go`
3. `protoc-gen-es` (local npm binary) → TypeScript message types at `web-app/src/gen`

No TypeScript ConnectRPC stubs are generated; the frontend calls ConnectRPC over HTTP using the generated message types directly.

### Makefile target

```bash
make proto-gen          # Generates only if .proto files are newer than stamp
buf generate proto      # Force regeneration (bypasses stamp check)
```

The `generate-proto` alias mentioned in CLAUDE.md maps to `proto-gen` in the Makefile.

### Adding a new proto file

1. Create `proto/session/v1/backlog.proto` with `package session.v1;`
2. Run `buf generate proto` (or `make proto-gen` after touching the file)
3. Generated Go code appears in `gen/proto/go/session/v1/` — no path changes needed
4. Generated TypeScript code appears in `web-app/src/gen/session/v1/`
5. The `sessionv1connect` package automatically includes the new service handler constructor

---

## Recommendations

1. **Separate proto file**: Create `proto/session/v1/backlog.proto` in the existing `session.v1` package. This avoids a new generated package and keeps the service handler in the existing `sessionv1connect` import path.

2. **Status as int, validated in domain layer**: Follow the existing `field.Int("status")` pattern with a Go `BacklogItemStatus` type + constants for the state machine. Avoid native ent enums — they require code generation changes and the existing codebase does not use them.

3. **Three ent schemas minimum**: `BacklogItem` (core entity), `BacklogSessionLink` (join table with extra fields: verdict, override, linked_at), and optionally `BacklogSource` (GitHub sync config). The `BacklogItem ↔ Session` edge is many-to-many with payload — model it as an explicit join entity rather than a bare ent edge.

4. **MCP tools in new file, minimal NewCore change**: Add `server/mcp/tools_backlog.go` + register in `NewCore`. The only change to `server/mcp/server.go` is adding a `storage *session.Storage` parameter and calling `registerBacklogTools`. This is a backward-compatible change since `NewCore` is only called from `NewHTTPHandler` and `RunServer`.

5. **ProjectService as the implementation template**: `server/services/project_service.go` is 122 lines and covers CRUD cleanly. Model `BacklogService` on it exactly — same nil-storage guard, same `xxxToProto()` helper, same `connect.NewError` error wrapping. Avoid starting from `session_service.go` (103 KB, too complex).
