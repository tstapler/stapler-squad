# ADR-014: Backlog Item Status State Machine

**Status**: Accepted
**Date**: 2026-05-10

## Context

Backlog items move through a well-defined lifecycle: `idea → ready → in_progress → review → done`, with `archived` reachable from any state and `archived → idea` as an explicit "reopen" transition. The status field must be:

- Stored durably in SQLite via the ent ORM
- Representable as a type-safe value in Go service and domain code
- Efficient for indexed queries (filter by status is the primary list query)
- Enforced at the service layer so invalid transitions are rejected with a structured error

Four storage models were considered. The choice must be consistent with all existing ent schemas in the codebase.

## Decision

Store status as `field.Int("status")` in the ent schema, with named Go constants defined in the domain layer. Transition enforcement lives in the service layer, not in ent hooks.

**Ent schema field**:

```go
// session/ent/schema/backlog_item.go
field.Int("status").
    Default(0).
    Comment("BacklogItemStatus: idea=0, ready=1, in_progress=2, review=3, done=4, archived=5")
```

**Domain layer constants** (in `session/backlog_item.go`):

```go
type BacklogItemStatus int

const (
    BacklogItemStatusIdea       BacklogItemStatus = 0
    BacklogItemStatusReady      BacklogItemStatus = 1
    BacklogItemStatusInProgress BacklogItemStatus = 2
    BacklogItemStatusReview     BacklogItemStatus = 3
    BacklogItemStatusDone       BacklogItemStatus = 4
    BacklogItemStatusArchived   BacklogItemStatus = 5
)

func (s BacklogItemStatus) IsValid() bool {
    return s >= BacklogItemStatusIdea && s <= BacklogItemStatusArchived
}

func (s BacklogItemStatus) String() string {
    switch s {
    case BacklogItemStatusIdea:       return "idea"
    case BacklogItemStatusReady:      return "ready"
    case BacklogItemStatusInProgress: return "in_progress"
    case BacklogItemStatusReview:     return "review"
    case BacklogItemStatusDone:       return "done"
    case BacklogItemStatusArchived:   return "archived"
    default:                          return "unknown"
    }
}
```

**Valid transitions** (enforced in `BacklogService` methods, not ent hooks):

| From | To | Guard | Trigger |
|---|---|---|---|
| `idea` | `ready` | `len(acceptance_criteria) > 0` | User action |
| `ready` | `in_progress` | session linked successfully | `SpawnSessionFromItem` |
| `in_progress` | `review` | — | `EventExited` hook or `request_review` MCP call |
| `in_progress` | `ready` | — | User aborts session |
| `review` | `done` | gate verdict = PASS or override_reason non-empty | Gate result or user override |
| `review` | `in_progress` | — | User requeues after FAIL/PARTIAL |
| `done` | `review` | — | User triggers re-review |
| any | `archived` | — | User action |
| `archived` | `idea` | — | User reopens (explicit `ReopenBacklogItem` RPC) |

Transition enforcement example in the service layer:

```go
func (s *BacklogService) TransitionStatus(ctx context.Context, itemID string, from, to BacklogItemStatus) error {
    if !isValidTransition(from, to) {
        return connect.NewError(connect.CodeFailedPrecondition,
            fmt.Errorf("invalid transition %s → %s", from, to))
    }
    // optimistic lock: update only if status = from
    n, err := s.storage.Client.BacklogItem.Update().
        Where(backlogitem.ID(itemID), backlogitem.Status(int(from))).
        SetStatus(int(to)).
        SetUpdatedAt(time.Now()).
        Save(ctx)
    if err != nil { return err }
    if n == 0 {
        return connect.NewError(connect.CodeAborted, fmt.Errorf("status precondition failed — concurrent modification"))
    }
    return nil
}
```

Indexes to define in `(BacklogItem).Indexes()`:

```go
index.Fields("status", "priority"),
index.Fields("status", "updated_at"),
```

## Alternatives Considered

**Option B: Ent native enum**

Ent supports `field.Enum("status").Values("idea", "ready", "in_progress", "review", "done", "archived")` which generates Go string constants and validates values at the ORM layer.

Rejected because no existing ent schema in the codebase uses native enums. The session status, session type, and project status fields all use `field.Int` or `field.String` with application-layer constants. Introducing a native enum would require learning and maintaining a different code generation path, produce inconsistent patterns with adjacent schemas, and provide no material benefit over int constants for a small fixed set of values. Adding a new enum value later also requires schema migration and regeneration.

**Option C: String field**

Store the status as `field.String("status")` with values `"idea"`, `"ready"`, etc. This is human-readable in SQL queries and avoids integer-to-string mapping.

Rejected because the existing codebase uses `field.Int` for status fields (see `session/ent/schema/` — `Session.status` is `field.Int`). Using a string field for backlog item status would diverge from this pattern, require string comparison in Go (less efficient than int comparison), and necessitate a `NOT NULL` constraint with a default string value rather than a simple `Default(0)`.

**Option D: Separate status history table**

Store the current status as a foreign key to the most recent row in a `BacklogItemStatusHistory` table. Each row records a transition with `from`, `to`, `trigger`, `actor`, and `timestamp`.

Rejected for MVP: the history table adds schema complexity (two tables instead of one field), slows all status-filtering queries (require a join or a denormalized current-status cache), and is premature optimization. The `updated_at` field on `BacklogItem` and the `ItemSession` records already provide sufficient audit trail for MVP. A history table can be added in a later milestone if retrospective transition analysis becomes a requirement.

## Consequences

**Positive**

- Consistent with all existing ent schemas: `field.Int("status")` with named Go constants is the established pattern throughout `session/ent/schema/`.
- Efficient indexing: integer comparisons in SQLite are faster than string comparisons; compound indexes on `(status, priority)` and `(status, updated_at)` are compact.
- Service-layer enforcement makes transition logic testable in isolation: a unit test calls `TransitionStatus` directly without setting up the full HTTP stack.
- Optimistic locking (the `WHERE status = <from>` predicate) prevents concurrent updates from racing past each other, addressing the phantom `review → done` race condition identified in the pitfalls research.
- Adding new statuses in future requires only adding a constant and updating `isValidTransition`; no schema migration needed if the new constant fits within the existing int range.

**Negative**

- Integer values are opaque in raw SQL queries and logs. Mitigated by the `String()` method on `BacklogItemStatus` and by using named constants everywhere in Go code.
- Ent does not validate the integer value against the known set at the ORM layer; an out-of-range int can be written directly to the DB if the service-layer guard is bypassed. Mitigated by `IsValid()` check at service entry points and a DB-level check constraint if SQLite version supports it.
- Transition logic lives in the service layer, not enforced by the database. Incorrect code in the service can violate the state machine. Mitigated by comprehensive unit tests for every transition in `server/services/backlog_service_test.go`.
- Status history is not recorded in MVP. Users cannot see who triggered a transition or when. Accepted trade-off; history table deferred.
