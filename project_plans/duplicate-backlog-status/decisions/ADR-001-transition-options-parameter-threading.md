# ADR-001: Thread `duplicate_of_id` via a `TransitionOptions` parameter object, not a second method

**Status**: Accepted
**Date**: 2026-07-29
**Project**: duplicate-backlog-status

## Context

`TransitionBacklogItemStatus` is declared at 3 layers (`session.Repository` interface, `session.Storage` passthrough, `session.EntRepository` implementation) and called from 6 sites (5 in `server/services/backlog_service.go` at lines 661, 953, 1046, 1119, 1356; 1 in `server/mcp/tools_backlog.go` at line 375). Only one of those 6 call sites (the RPC handler at line 661, plus the new `mark_duplicate` MCP tool) ever needs to pass `duplicate_of_id`. The other 5 transition to statuses other than `duplicate` and have no use for it.

Two designs were considered for getting `duplicate_of_id` from the caller into the write layer:

1. **Fork a second method**, e.g. `TransitionBacklogItemStatusToDuplicate(ctx, id, duplicateOfID, precondition)`, leaving the original method untouched for the other 5 call sites.
2. **Extend the existing method's signature** with a 5th parameter, a small `TransitionOptions{ DuplicateOfID string }` struct, and update all 6 call sites (5 pass `nil`, 1-2 pass a populated struct).

## Decision

Option 2: extend the existing signature with `opts *TransitionOptions`, threaded through all 3 layers, with all 6 existing call sites updated to pass `nil`.

## Rationale

- **One write path, not two.** A forked method is exactly the shape of bug this feature is trying to avoid at the domain-model level (see requirements.md's insistence that RPC and MCP entry points share one write path "so they can't drift"). If `mark_duplicate`'s specific validation/atomic-write logic lived in a separate method, a future change to the core transition logic (e.g. a new precondition field, a new audit-event shape) would have to be applied in two places, and it is exactly the kind of thing that is easy to update in one and forget in the other.
- **Parameter Object is the standard fix for a signature that would otherwise grow an ad-hoc parameter for every future edge case.** `TransitionOptions` costs one new 3-line struct and is extensible without another N-call-site diff if a future transition (not necessarily `duplicate`) needs its own side-channel field.
- **The mechanical cost is small and one-time.** 3 signature edits + 6 one-line call-site edits (`nil` in 5, a real struct in 1-2), all caught immediately by `go build` if missed — there is no silent-failure risk here, unlike, say, a missing frontend `STATUS_CLASS` entry.
- **Alternative considered and rejected**: a bare `duplicateOfID string` 5th parameter (no struct) was considered and rejected in favor of the named struct — an unnamed positional string at 6 call sites is worse self-documentation than `opts.DuplicateOfID`, and the struct costs nothing extra to add.

## Consequences

- Every future addition to `TransitionBacklogItemStatus`'s side-channel data (if any) goes into `TransitionOptions`, not a new positional parameter or a new method.
- All 6 existing call sites required a one-line edit as part of this feature (tracked in plan.md Story 2.2.2) — this is a one-time cost paid now in exchange for not paying a larger, more error-prone cost (two divergent write paths) later.
