# ADR-004: Project Entity Design — First-Class ent Entity vs Tag Convention

**Status**: Accepted
**Date**: 2026-04-17
**Deciders**: Tyler Stapler

---

## Context

Story 4 introduces a "Project" concept: a named group of related sessions. The UX goal is a project picker on session creation, a project grouping strategy in the existing `GroupBy` dropdown, and a project header showing aggregate stats (N running, N complete, N review-ready).

Two design paths exist: (1) a first-class `Project` entity in the ent/SQLite schema, or (2) a tag naming convention (`project:<name>`).

The codebase already has ent entities for Session, Tag, Worktree, ClaudeSession, ApprovalRule, ClassificationAnalytics, ClaudeMetadata, and DiffStats. Adding `Project` is consistent with the existing ent pattern.

---

## Decision

Add `Project` as a new ent schema entity at `session/ent/schema/project.go`. Add a nullable `project_id` FK edge on `Session`. Existing sessions retain null FK (no project = "Ungrouped"). No auto-migration from Category/tags; users assign sessions to projects explicitly via the new project picker.

Tag convention is rejected.

---

## Options Considered

### Option A: Tag convention — project:<name> (rejected)

Sessions belong to a project by holding a `project:my-project` tag.

**Rejected because**:
- Cannot support project-level aggregate queries without scanning all sessions and filtering by tag prefix
- Cannot attach project metadata (description, repo path, created date) to the concept
- Tag naming convention is fragile: `Project:X`, `project:X`, `project-x` all diverge
- Cannot enumerate projects without a full tag scan
- Does not support "rename project" atomically (requires retagging all sessions)

### Option B: First-class ent entity (accepted)

```go
// session/ent/schema/project.go
type Project struct {
    ent.Schema
}

func (Project) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").DefaultFunc(uuid.NewString).Unique().Immutable(),
        field.String("name").NotEmpty().Unique(),
        field.String("description").Optional(),
        field.String("workspace_path").Optional(),
        field.Time("created_at").Default(time.Now).Immutable(),
        field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
    }
}

func (Project) Edges() []ent.Edge {
    return []ent.Edge{
        edge.To("sessions", Session.Type),
    }
}
```

Session schema gains:
```go
edge.From("project", Project.Type).
    Ref("sessions").
    Unique().
    Optional()
```

**Accepted because**:
- Queryable: `client.Project.Query().WithSessions().All(ctx)` returns all projects with their sessions in one query
- Aggregate stats (running/complete/review-ready count) computable via ent predicates, not full session scans
- Project rename is an O(1) update on the Project row, not a tag rename across N sessions
- Backwards-compatible: null FK means "no project"; existing sessions are unaffected
- Consistent with the existing ent pattern in this codebase

---

## Migration

The ent `Schema.Create(ctx)` call with SQLite uses `CREATE TABLE IF NOT EXISTS` + `ALTER TABLE ADD COLUMN` semantics. No manual SQL migration is required. Running `go generate ./session/ent/...` followed by starting the server with the updated schema performs the migration automatically.

Existing sessions have `project_id = NULL` after migration; this is valid and the UI shows them in an "Ungrouped" catch-all group.

---

## Proto Design

```protobuf
message Project {
  string id             = 1;
  string name           = 2;
  string description    = 3;
  string workspace_path = 4;
  google.protobuf.Timestamp created_at = 5;
  google.protobuf.Timestamp updated_at = 6;
  // Aggregate stats (populated on ListProjects with include_stats=true)
  int32 session_count          = 7;
  int32 running_count          = 8;
  int32 complete_count         = 9;
  int32 review_ready_count     = 10;
}

// New RPCs on SessionService
rpc CreateProject(CreateProjectRequest) returns (CreateProjectResponse) {}
rpc ListProjects(ListProjectsRequest) returns (ListProjectsResponse) {}
rpc UpdateProject(UpdateProjectRequest) returns (UpdateProjectResponse) {}
rpc DeleteProject(DeleteProjectRequest) returns (DeleteProjectResponse) {}
rpc AssignSessionsToProject(AssignSessionsToProjectRequest)
    returns (AssignSessionsToProjectResponse) {}
```

`ListSessionsRequest` gains:
```protobuf
optional string project_id = 5;  // Filter by project
```

`CreateSessionRequest` gains:
```protobuf
optional string project_id = 14;  // Assign to project at creation
```

---

## Consequences

**Positive**:
- Clean relational model; project-level operations are natural
- Aggregate stats are fast (ent count queries, not full session loads)
- Project CRUD is straightforward; no tag scanning

**Negative / Accepted**:
- ent schema change requires `go generate ./session/ent/...` + server restart
- UI needs Project CRUD: create, rename, delete (with confirmation if sessions exist)
- Delete project with sessions: either cascade (sessions become ungrouped) or block (require reassignment first); use cascade (simplest UX)

**Implementation notes**:
- `DeleteProject`: set `project_id = NULL` on all sessions in the project before deleting the Project row (ent does not support ON DELETE SET NULL in SQLite without explicit migration)
- `ListProjects` default: include aggregate stats always (avoid N+1 by using ent count subqueries)
- GroupBy "Project" strategy: reuse existing `GroupingStrategy` interface; `GroupByProject` returns sessions grouped by `ProjectID`; sessions with `project_id = NULL` appear in "Ungrouped"
- Project picker on session creation form: dropdown of existing projects + "New project" inline creation
