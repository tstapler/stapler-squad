# Quick Workflows — Storage Mechanism Research

## Workflow Object Shape

The feature needs to persist objects with these fields:

| Field | Type | Notes |
|---|---|---|
| `id` | UUID | Stable primary key |
| `slug` | string | URL-friendly identifier, unique |
| `name` | string | Human display name |
| `description` | string | Optional |
| `command` | string | Shell command to run |
| `targetDirectory` | string | Working directory |
| `inputTemplate` | string | Prompt template, optional |
| `sessionType` | string | Maps to existing `session.SessionType` |
| `model` | string | AI model override, optional |
| `agentType` | string | Agent program, optional |
| `cronExpression` | string | cron schedule, optional |
| `cronEnabled` | bool | Cron active flag |
| `createdAt` | time.Time | Immutable |
| `updatedAt` | time.Time | Auto-updated |

---

## Option 1: Existing Config System (`config/config.go`)

### How it works

`config/config.go` defines a `Config` struct serialized to `~/.stapler-squad/<workspace>/config.json`. The file is loaded with `LoadConfig()` and saved atomically via a temp-file rename in `saveConfig()`. New top-level sections are added as struct fields with `json:"field_name,omitempty"` tags. The `LoadConfigFromPath` function applies zero-value defaults for missing fields after unmarshal, so forward compatibility is straightforward.

`config/state.go` follows the same pattern for a separate `state.json` that tracks UI preferences (collapsed categories, selected index, etc.). It adds file-locking (`flock`) on top for multi-process safety.

There is precedent for composite sub-objects: `SessionDefaults` (profiles, directory rules), `HibernationConfig`, `BrowserPassthroughConfig`. These are all embedded structs serialized as nested JSON objects.

### Adding workflows here

```go
// In config.go
type WorkflowDef struct {
    ID              string    `json:"id"`
    Slug            string    `json:"slug"`
    Name            string    `json:"name"`
    Description     string    `json:"description,omitempty"`
    Command         string    `json:"command"`
    TargetDirectory string    `json:"target_directory"`
    InputTemplate   string    `json:"input_template,omitempty"`
    SessionType     string    `json:"session_type,omitempty"`
    Model           string    `json:"model,omitempty"`
    AgentType       string    `json:"agent_type,omitempty"`
    CronExpression  string    `json:"cron_expression,omitempty"`
    CronEnabled     bool      `json:"cron_enabled"`
    CreatedAt       time.Time `json:"created_at"`
    UpdatedAt       time.Time `json:"updated_at"`
}

// Added to Config struct:
Workflows []WorkflowDef `json:"workflows,omitempty"`
```

The `LoadConfigFromPath` function already has a block that initializes nil slice fields, so adding:
```go
if cfg.Workflows == nil {
    cfg.Workflows = []WorkflowDef{}
}
```
is sufficient for backward compatibility.

### Assessment

**Fit**: Low. Config is for per-instance settings (listen address, feature flags, program paths). Adding a dynamic collection of user-created domain objects is a category mismatch — config is read once at startup and rarely mutated, while workflows need frequent CRUD operations.

**CRUD complexity**: High. Every create/update/delete requires read-the-whole-file → marshal entire struct → write atomically. No way to update a single workflow without reserializing all others. The flock mechanism on `state.json` is not reused by config, so concurrent writes from multiple processes (server + CLI) are not safe.

**Migration story**: Trivially backward compatible — `omitempty` means old configs load cleanly with empty `Workflows` slice. No migration code needed.

**Schema evolution**: Adding new fields to `WorkflowDef` is safe (JSON unmarshal ignores unknown keys; zero values apply for missing fields). Renaming or removing fields requires manual migration.

**Query capabilities**: List all = iterate slice. Filter by slug = O(n) linear scan. No indexing. Acceptable for small collections (<100 workflows), poor if the feature grows.

**Verdict**: Do not use. The config file is not designed for mutable collections; coupling user-created workflow definitions to application configuration will cause maintenance pain.

---

## Option 2: Separate JSON File (`workflows.json`)

### How it works

Several subsystems use separate JSON files in the config directory:

- `push-subscriptions.json` — `PushService` in `server/services/push_service.go` (in-memory map + file sync on every mutation, protected by `sync.RWMutex`)
- `vapid-keys.json` — same service, same pattern
- `notifications.json` — referenced in `server/server.go`
- `pending_approvals.json` — referenced in `server/services/session_service.go`
- `passkeys.json` — `server/auth/store.go`

The config directory path comes from `config.GetConfigDir()`, which handles workspace isolation, test isolation, and instance naming consistently.

A `workflows.json` approach would look like:

```go
// Path: filepath.Join(config.GetConfigDir(), "workflows.json")

type WorkflowStore struct {
    mu        sync.RWMutex
    path      string
    workflows map[string]WorkflowDef // keyed by ID
}

func (s *WorkflowStore) List() []WorkflowDef { ... }          // read lock, return sorted slice
func (s *WorkflowStore) GetBySlug(slug string) (*WorkflowDef, error) { ... }
func (s *WorkflowStore) Create(w WorkflowDef) error { ... }   // write lock, save to disk
func (s *WorkflowStore) Update(w WorkflowDef) error { ... }   // write lock, save to disk
func (s *WorkflowStore) Delete(id string) error { ... }       // write lock, save to disk
```

The `push-subscriptions.json` pattern (`PushService`) is a reference implementation: load on startup into an in-memory map, mutex-protect all access, write the whole file on each mutation. Atomic writes use `os.WriteFile` (not the temp-file rename pattern, though the config system uses that pattern and it is preferable).

### Assessment

**Fit**: Medium. There is clear precedent for separate JSON files for service-specific state. It is a natural extension of existing patterns and is completely decoupled from `config.json`. The file lives in `GetConfigDir()` so workspace isolation and test isolation are free.

**CRUD complexity**: Low-medium. Write a simple in-memory cache backed by disk, similar to `PushService`. About 150 lines of boilerplate. No transaction semantics — a crash mid-write can corrupt the file (mitigated by atomic rename pattern). All queries are O(n) unless you maintain secondary maps (e.g., `slugIndex map[string]string`).

**Migration story**: Zero migration work. If the file doesn't exist on first run, start with an empty store. The format can evolve because JSON is self-describing and new fields get zero values.

**Schema evolution**: Same as Option 1 — add optional fields freely. Renaming keys requires a migration pass over the JSON array.

**Query capabilities**: List all = return in-memory slice. Filter by slug = O(1) with secondary slug→ID map. Cron queries = iterate slice (small enough to be fine). No relational queries needed for workflows.

**Concurrency**: In-process mutex is sufficient for a single-server deployment. Multi-process scenarios (CLI + server) would need flock, same as `config/state.go`.

**Verdict**: Viable for an MVP. Simple, self-contained, no schema migration tooling required. Best choice if workflows are expected to stay small in number (<500) and the feature set does not grow to require relational queries (e.g., "which sessions ran this workflow").

---

## Option 3: SQLite via Ent ORM (`session/ent/schema/`)

### How it works

The project uses the `entgo.io/ent` ORM backed by SQLite. The database lives at `<configDir>/sessions.db` — confirmed in multiple places:
- `session/ent_repository.go`: default path `~/.stapler-squad/sessions.db`
- `server/services/session_service.go`: `dbPath := configDir + "/sessions.db"`
- `cmd/migrate_data.go`: `dbPath := filepath.Join(configDir, "sessions.db")`

The ent client is opened in `NewEntRepository` with WAL mode, a single write connection (`SetMaxOpenConns(1)`) to serialize writes, and automatic schema migration via `client.Schema.Create(ctx)` on startup. This means any new schema entity is created automatically the first time the app starts after the binary is updated.

**CRITICAL** (from CLAUDE.md): The generate command requires `--feature sql/upsert`:
```bash
go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
```

Adding a `Workflow` entity follows the exact same pattern as `BacklogItem` (which uses a UUID primary key) or `ApprovalRule` (which uses a string unique key):

```go
// session/ent/schema/workflow.go
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
        field.String("session_type").Optional(),
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
    }
}
```

After adding this file, run:
```bash
go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
go build ./...
```

Ent generates typed builders (`tx.Workflow.Create()`, `tx.Workflow.Query()`, etc.) in the `session/ent/` directory. The `client.Schema.Create(ctx)` call in `NewEntRepository` creates the `workflows` table on startup — no manual migration SQL required.

For accessing workflows from outside `session/`, the pattern is to call `repo.GetEntClient()` (see `GetEntClient()` in `ent_repository.go`) and use the typed client directly, or wrap operations in a new `WorkflowRepository` struct parallel to `EntRepository`.

### Assessment

**Fit**: High. The project already uses ent as its primary persistence layer for all domain entities (sessions, projects, backlog items, approval rules, analytics events). Workflows are a first-class domain entity and belong here.

**CRUD complexity**: Low after the schema is defined. Ent generates all the CRUD builders. A list query with slug filtering:
```go
client.Workflow.Query().
    Where(workflow.SlugEQ(slug)).
    Only(ctx)
```
Transactions, error handling, and type safety are all handled by the generated code.

**Migration story**: Automatic. `client.Schema.Create(ctx)` runs on every startup and creates any missing tables/columns. SQLite `ALTER TABLE ADD COLUMN` is idempotent for new columns. No migration scripts needed for additive changes. The existing `migrate.go` and `status_remap.go` patterns show how to handle non-additive changes (column renames, value remaps) when they become necessary.

**Schema evolution**: Adding new fields is trivially safe — mark them `Optional()` with a `Default()` so existing rows get the default value on read. The `approvalrule.go` schema demonstrates this pattern: it added multiple `field.JSON()` fields with `Optional().Default([]string{})` so old rows (which have NULL in those columns) load cleanly.

**Query capabilities**: Full SQL capability via ent predicates. Can filter by `cron_enabled=true`, order by `created_at`, join to sessions if needed (via edge), paginate, etc. All queries compile to typed Go code.

**Relational growth**: If the feature evolves to track "which sessions ran this workflow," adding an edge from `Session` to `Workflow` (or a join table) is straightforward. This would be very hard with flat JSON files.

---

## Other Persistence Patterns Observed

- **`config/state.go`**: File-locked JSON for UI ephemeral state (selected index, collapsed categories). Pattern: flock + full-file read/write. Not suitable for mutable user data collections.
- **`server/auth/store.go` (`passkeys.json`)**: Another in-memory-map-backed JSON file. Same pattern as `push_service.go`.
- **`server/services/rules_store.go`**: Exports approval rules to `~/.config/stapler-squad/rules.json` — a write-only export path, not a primary store.
- **In-memory only**: Some transient state (pending approvals queue in `session_service.go`) uses in-memory maps without disk persistence. Not applicable for workflows which must survive restarts.

---

## Recommendation: SQLite via Ent ORM (Option 3)

Use the existing ent ORM to add a `Workflow` entity in `session/ent/schema/workflow.go`.

### Justification

1. **Architectural fit**: Every user-created domain entity in this codebase lives in the ent schema. Sessions, backlog items, projects, approval rules — all are ent entities backed by `sessions.db`. Workflows belong in the same layer.

2. **Zero migration infrastructure cost**: `client.Schema.Create(ctx)` handles table creation automatically on first run after upgrade. No migration scripts, no migration service, no user action required.

3. **Schema evolution is safe by default**: The `approvalrule.go` schema demonstrates the established pattern for additive migrations — `Optional().Default(x)` on new fields. Existing rows get the default transparently.

4. **CRUD is generated, not hand-rolled**: After defining the schema, ent generates type-safe builders and query predicates. No hand-written SQL or JSON marshaling. The `BacklogItem` and `ApprovalRule` entities are the closest reference implementations.

5. **Future-proof for relational queries**: If the feature needs to link workflows to sessions ("sessions created from this workflow"), an edge can be added to the schema. This is not possible with flat JSON files without re-architecting.

6. **Concurrency handled**: The existing `sessions.db` uses `SetMaxOpenConns(1)` + WAL mode to serialize writes. Workflow CRUD operations through the same ent client inherit this.

### The only scenario where Option 2 is preferable

If Quick Workflows is intended to be a standalone, purely configuration-like feature (no session linkage, no analytics, no relational queries ever), and the team wants to avoid touching the ent schema for a quicker MVP, then the separate `workflows.json` file (Option 2) is viable and lower-risk in the short term. The `PushService` in `server/services/push_service.go` is a clean reference implementation. However, the technical debt of migrating to SQLite later (when the feature grows) is non-trivial, so this is only justified if the scope is explicitly constrained.

### Implementation checklist (Option 3)

- [ ] Create `session/ent/schema/workflow.go` with the schema above
- [ ] Run `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`
- [ ] Run `go build ./...` to verify generated code compiles
- [ ] Add a `WorkflowRepository` interface + `EntWorkflowRepository` struct in `session/` (parallel to `repository.go` / `ent_repository.go`), or expose workflow operations through `GetEntClient()` from existing `EntRepository`
- [ ] Commit all generated `session/ent/` files together with the schema change
