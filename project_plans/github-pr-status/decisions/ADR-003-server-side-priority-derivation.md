# ADR-003: Server-Side PR Priority Derivation

**Status:** Accepted
**Date:** 2026-04-09
**Decision:** Compute the `PRPriority` enum in Go on the server, not in the React client.

## Context

PR status is a compound concept: review decision (APPROVED, CHANGES_REQUESTED), CI conclusion (SUCCESS, FAILURE), lifecycle state (open, merged, closed), and draft flag. Comparable tools (GitHub, Linear, Raycast) use derived summary states for scanning:

- GitHub: "Ready to merge", "Changes requested", "Checks failing"
- Linear: "In Review", "Ready to Merge"
- Raycast: Single color-coded pill per PR

The question is where to compute the derived priority: server-side (Go) or client-side (TypeScript).

## Decision

Compute `PRPriority` server-side in Go via a `DerivePRPriority()` function. Send the derived enum string to the client alongside raw counts for tooltip display.

Priority taxonomy:
```
blocking   -- CHANGES_REQUESTED or CI FAILURE or ACTION_REQUIRED
ready      -- APPROVED and checks SUCCESS (or no checks)
pending    -- checks IN_PROGRESS or awaiting first review
draft      -- isDraft = true
complete   -- MERGED or CLOSED
no_pr      -- no PR found for branch
auth_error -- gh not installed or unauthenticated
```

Derivation precedence (first match wins):
1. MERGED or CLOSED -> `complete`
2. isDraft -> `draft`
3. CHANGES_REQUESTED count > 0 -> `blocking`
4. checks FAILURE or ACTION_REQUIRED -> `blocking`
5. APPROVED count > 0 AND checks SUCCESS -> `ready`
6. checks IN_PROGRESS or PENDING -> `pending`
7. default -> `pending`

## Consequences

### Positive

- **Single source of truth** for priority logic. No risk of Go and TypeScript implementations diverging.
- **Simpler client code.** React component receives a `priority` string and maps to color/icon. No business logic in the badge.
- **Testable in Go.** Table-driven unit tests for derivation logic. No browser-based testing needed for priority correctness.
- **Smaller proto payload.** One string field (`priority`) instead of shipping raw review arrays.

### Negative

- **Server must re-derive on every status change.** Trivial -- derivation is a pure function on 4 fields.
- **Client cannot override priority.** Acceptable for Phase 1. If client-side overrides are needed later, add a separate `priority_override` field.

## Alternatives Considered

### Client-side derivation

Rejected. Duplicates business logic in TypeScript. Requires shipping full review/check arrays in proto. Creates two codepaths to maintain and test.

### Hybrid (server derives, client can override)

Deferred to Phase 2. No current use case for client overrides.
