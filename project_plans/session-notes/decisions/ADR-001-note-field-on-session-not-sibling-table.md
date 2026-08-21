# ADR-001: Session Note Is a Field on `Session`, Not a `SessionGoal`-Style Sibling Table

**Date**: 2026-08-06
**Status**: Accepted
**Context**: session-notes feature — where the new `note` data lives

---

## Context

The requirements ask for one free-form markdown note per session, user-editable from the
session detail view, persisted across restarts. Two existing precedents in this codebase
point in different directions for how to store it:

1. **`SessionGoal`** (`session/ent/schema/session_goal.go`) — a separate ent table with a
   unique `session_uuid` index (1:1 with a session), merged into the in-memory `Instance` at
   load time (`session/storage.go`'s `SessionGoal.Query()` pattern). It is agent-written only,
   via an MCP tool (`server/mcp/tools_goal.go`), never through a user-facing ConnectRPC
   endpoint — the frontend only reads it (`GoalPanel.tsx` has no edit UI at all).
2. **Simple scalar fields a *user* edits from the UI** — `title`, `category`, `program`,
   `working_dir`, `tags`, `pause_reason`, etc. — all live as columns directly on the `Session`
   ent schema and are written through one shared RPC, `UpdateSession`
   (`proto/session/v1/session.proto:579`), each field independently `optional` and checked in
   the handler (`server/services/session_service.go:1702`).

`SessionGoal` is the closest *structural* analog ("one field, 1:1 with a session, persisted in
ent/SQLite, survives restart"), which made it tempting to copy its shape wholesale. But its
shape exists to solve problems the note doesn't have: multi-writer concurrency (any agent tool
call can update it mid-session) and structured payload complexity (a status enum plus a task
tree, not just a string).

## Decision

`note` is added as a plain `field.Text("note").Optional().Default("").MaxLen(10000)` column on
the existing `Session` ent schema (`session/ent/schema/session.go`), threaded through the
existing `UpdateSessionRequest`/`UpdateSessionResponse` RPC (`optional string note` at the next
free field number), not a new `SessionNote` sibling table and not a dedicated
`UpdateSessionNote` RPC.

Deciding factor: **who writes the field**. A session note is single-owner (the user editing it
from the UI), a single scalar, with no side effects and no uniqueness constraint — it matches
the `category`/`working_dir` shape exactly, not the `SessionGoal` shape (agent-written,
multi-writer, structured).

## Consequences

- **No new load-time merge wiring.** `note` rides the same `InstanceData`/`ToInstanceData`/
  `FromInstanceData`/`EntRepository` round trip every other `Session` scalar field already uses
  — no `SessionNote.Query()`-style join needs to be added anywhere `Instance`s are loaded.
- **Multi-tab sync is free.** `UpdateSession` already publishes
  `events.NewSessionUpdatedEvent(instance, updatedFields)` after any field changes, which flows
  to every open tab via `WatchSessions`. Adding `"note"` to `updatedFields` is the only wiring
  needed — no new event type.
- **No orphan-on-delete gap to inherit.** `SessionGoal` rows currently leak on `DeleteSession`
  (a real, separate, already-flagged bug — see `research/features.md` edge case 5) because they
  live in a sibling table keyed by `session_uuid`. Since `note` is a column on the `Session` row
  itself, `DeleteSession`'s existing row-delete call removes it automatically — this decision
  sidesteps that entire class of bug rather than inheriting it.
- **Consistent with the requirements' own non-goals.** The non-goals explicitly rule out
  optimistic-concurrency revisioning ("single-user model, last-write-wins is acceptable"). A
  sibling-table shape (like `SessionGoal`'s) is the kind of complexity that assumption implies
  is unnecessary here — reusing the existing single-writer `UpdateSession` RPC keeps that
  simplicity honest rather than building revisioning-capable infrastructure and then not using it.
- **One more field on an already-large struct set.** `Session` (ent schema + `InstanceSnapshot`
  + proto message) already has dozens of fields; this adds one more entry to each. Accepted as
  the smaller cost compared to standing up a second persistence path for a single string.

## Alternatives Considered

- **`SessionNote` sibling ent table** (mirrors `SessionGoal`): rejected — would require
  re-deriving `SessionGoal`'s load-time merge wiring for a field with none of the
  multi-writer/structured-payload complexity that wiring exists to serve.
- **Dedicated `UpdateSessionNote` RPC** (mirrors `RenameSession`): rejected — `RenameSession` is
  dedicated only because renaming carries an extra invariant (title-uniqueness check) that
  changes control flow. Note has no such invariant; a dedicated RPC would duplicate
  `UpdateSession`'s event-publish block for zero behavioral gain.
