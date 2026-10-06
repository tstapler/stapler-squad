# ADR-001: Dangling blocker resolution (archived and hard-deleted blockers)

## Status

Accepted

## Context

A backlog item dependency (`blocked_id` waits on `blocker_id`) is meant to gate dequeue/start
on the blocker reaching a terminal, resolved state. The backlog status enum
(`session/domain/backlog.go`) has no literal "shipped" status — `done` is the ship-equivalent
terminal state, and `archived` is a separate terminal state meaning "will never ship."
`DeleteBacklogItem` is a genuine hard delete (`session/ent_repository_backlog.go`), not a
soft-archive alias.

Two dangling cases need an explicit answer, or a dependent can get stuck forever with no path
to become eligible for dequeue again (see `research/pitfalls.md` Pitfall 4 for the original
analysis):

1. **Blocker is archived.** It will never reach `done`. If "resolved" means only `done`, the
   dependent is permanently blocked.
2. **Blocker is hard-deleted.** The dependency row's FK target no longer exists — what does
   "is my blocker resolved" mean when the blocker itself is gone?

## Decision

- **Archived counts as resolved**, same as `done`. `UnresolvedBlockerItemIDs`
  (`session/ent_repository_backlog.go`) excludes blockers whose status is `done` OR
  `archived` from the unresolved set. Rationale: archived means the item will never ship, so
  waiting on it can never be satisfied — treating it as still-blocking has no useful outcome
  other than stranding the dependent. Archiving a blocker is the operator's explicit signal
  that the work isn't happening; the correct response is to let dependents proceed, not to
  wait on a promise that was withdrawn.
- **A hard-deleted blocker counts as resolved.** The `blocking_dependencies` and
  `blocked_by_dependencies` edges on `BacklogItem` (`session/ent/schema/backlog_item.go`) both
  carry `entsql.OnDelete(entsql.Cascade)`. Deleting either side of a dependency pair removes
  the `BacklogItemDependency` join row automatically. There is no dangling FK to reason about
  at query time — once the blocker row is gone, its dependency edges are gone too, and
  `UnresolvedBlockerItemIDs`'s query simply finds no unresolved-blocker row for that dependent.
  This is the same outcome as the archived case (deletion is a strictly harder form of "will
  never ship"), reached structurally via the schema rather than an explicit status check.

## Consequences

- No dependent can be permanently and silently stuck by an abandoned blocker — every path off
  a blocker (ship, archive, delete) resolves its dependents.
- Losing a blocker (via archive or delete) does not surface as an error to the dependent; it
  silently proceeds. This favors availability over correctness-of-intent: a dependent that
  genuinely required the blocker's actual output (not just its completion) has no signal that
  the blocker never shipped. If this becomes a real problem, a follow-up could add a
  UI-visible "blocker was archived/deleted, not completed" annotation on the dependent's
  history — this ADR intentionally does not add that now, since no such requirement exists
  yet.
- Cascade delete is enforced at the schema level (SQLite FK), not re-implemented in
  application code — one fewer place for this invariant to drift.
