# ADR-011: Domain DTOs for `ItemSession`, `ReviewVerdict`, and Related ent Types

**Date**: 2026-07-09
**Status**: Accepted
**Deciders**: backlog-service-refactor planning

---

## Context

`session.Storage` methods return `*ent.ItemSession`, `*ent.ReviewVerdict`, `*ent.SourceSyncEvent`,
and `*ent.BacklogStatusEvent` directly. This forces `server/services/backlog_service.go` to
import `session/ent` (a generated ORM package) to iterate over these return values.

The result: every ent schema field addition requires a concurrent change in both
`ent_repository_backlog.go` AND `backlog_service.go`, explaining the observed 0.78
change-coupling ratio between those files.

## Decision

Introduce domain DTOs in `session/repository.go`:
- `ItemSessionSummary` — replaces `*ent.ItemSession` in all Storage returns and in `BacklogItemData.ItemSessions`
- `BacklogStatusEventData` — replaces `*ent.BacklogStatusEvent` in `BacklogItemData.StatusEvents`
- `SourceSyncEventData` — replaces `*ent.SourceSyncEvent` in `ListSourceSyncEvents`

Conversion functions (`itemSessionToSummary`, etc.) live in `session/ent_repository_backlog.go`
— the one file that is allowed to import `session/ent` for mapping purposes.

Add all ItemSession CRUD methods to the `Repository` interface (with domain return types),
removing the type-assertion workarounds that currently bypass the abstraction.

## Error handling

`ent_repository_backlog.go` must consistently wrap `ent.IsNotFound` as `session.ErrNotFound`
before returning to callers. Callers in `server/services` must use `errors.Is(err, session.ErrNotFound)`
only. A `forbidigo` lint rule enforces this.

## Alternatives Considered

**Keep `*ent.ItemSession` but extract helper methods** — Reduces the explicit leakage but
leaves the ent import in `server/services`. Rejected because the coupling ratio problem
persists: callers still depend on ent's struct field layout.

**Introduce a full ORM-agnostic repository interface for all backlog types** — Correct DDD
approach but exceeds the scope of a behavior-preserving refactor. A future feature could
upgrade to a full interface; this ADR takes the minimum viable step.

## Consequences

- `server/services/backlog_service.go` removes its `session/ent` import after P6 lands.
- `BacklogItemData.ItemSessions` field type changes from `[]*ent.ItemSession` to
  `[]ItemSessionSummary` — this is a breaking change for any caller that accessed ent fields
  directly. All such callers are inside this module (no external API consumers); the breakage
  is caught by `make build`.
- The `no_ent_in_services` depguard rule is activated in `.golangci.yml` after this ADR's
  changes land, preventing future reintroduction.
- `ItemSessionSummary` deliberately omits ent-internal fields (e.g., `Edges`). If new fields
  are needed by callers, they are added to the DTO explicitly — not by reverting to `*ent.ItemSession`.
