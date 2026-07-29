# ADR-002: Fold single-hop chain-prevention into `ErrDuplicateOfInvalidTarget` rather than adding a 4th sentinel error

**Status**: Accepted
**Date**: 2026-07-29
**Project**: duplicate-backlog-status

## Context

requirements.md's Non-Goals section explicitly puts a single-hop chain-prevention rule **in scope**: forbid marking an item duplicate-of a target that is itself already `duplicate` status. This blocks both multi-hop chains (A→B→C) and mutual duplication (A↔B) with one guard rule, per research/features.md §3.

Separately, AC6 states: *"TransitionGuard rejects a transition to duplicate when duplicate_of_id is empty, self-referencing, or references a nonexistent item (**three new sentinel errors**)..."* — the acceptance criterion's parenthetical explicitly commits to a count of three.

This creates a tension: the guard needs to check 4 distinct conditions (empty / self-reference / nonexistent / target-is-itself-duplicate), but AC6 says "three new sentinel errors." research/architecture.md flagged this exact tension without resolving it, leaving it for the planning subagent.

## Decision

Keep exactly three sentinel errors — `ErrDuplicateOfRequired`, `ErrDuplicateOfSelf`, `ErrDuplicateOfInvalidTarget` — and fold the "target is itself already `duplicate`" chain-prevention check into `ErrDuplicateOfInvalidTarget`'s return case, rather than adding a 4th sentinel (e.g. `ErrDuplicateOfChained`).

```go
case to == BacklogStatusDuplicate:
    if item.DuplicateOfID == "" {
        return ErrDuplicateOfRequired
    }
    if item.DuplicateOfID == item.ID {
        return ErrDuplicateOfSelf
    }
    if !item.DuplicateOfExists || item.DuplicateOfStatus == BacklogStatusDuplicate {
        return ErrDuplicateOfInvalidTarget
    }
    return nil
```

## Rationale

- **AC6's literal wording is the acceptance bar, not the architecture research's tentative framing.** The architecture research explicitly deferred this exact decision to the planning subagent ("the planning subagent should settle this... use judgment, but the acceptance criterion below requires 'three new sentinel errors' so pick three, not two"). AC6 says three; this decision honors that literally.
- **Semantic fit is genuine, not just a headcount trick.** A target that is itself already `duplicate` status is not a "valid" target from the caller's perspective — it does not resolve to a canonical item at all (it resolves to *another* duplicate, one hop removed from whatever it points to). "This id doesn't resolve to a valid target" is exactly `ErrDuplicateOfInvalidTarget`'s semantic — the same framing pitfalls.md uses when describing how `mark_duplicate`'s guard-rejected cases should be surfaced ("guard-rejected transitions map to the same not-found-flavored user error as a literally-missing item, since from the caller's perspective 'this id doesn't resolve to a valid target' is the same class of problem either way").
- **Caller-facing consequence is acceptable.** Callers (RPC handler, `mark_duplicate` MCP tool) cannot distinguish "duplicate_of_id genuinely doesn't exist" from "duplicate_of_id exists but is itself a duplicate" purely from the sentinel — both need the same remediation from the caller's point of view: pick a different, non-duplicate, existing target. No test or caller in this plan needs to distinguish the two cases separately (see plan.md Task 1.1.2d, which tests both conditions but asserts the same `ErrDuplicateOfInvalidTarget` for both).
- **Naming**: the sentinel is named `ErrDuplicateOfInvalidTarget`, not `ErrDuplicateOfNotFound` (an earlier draft's name), specifically so the identifier does not imply "target absent" in the chain-prevention case where the target unambiguously exists and was just successfully fetched — it is merely ineligible (itself already `duplicate`-status), not missing. This is a naming-only change; it does not affect AC6's "three sentinels" count or the message text, which was independently verified as already accurately worded.

## Alternative Rejected

A 4th sentinel `ErrDuplicateOfChained` (or similar) was considered. It would let error messages be marginally more specific ("target is itself a duplicate" vs. "target does not exist"), but:
- It contradicts AC6's explicit "three new sentinel errors" wording.
- It buys no behavioral difference for any caller in this plan — both cases are rejected identically (return an error, do not transition).
- It would need its own dedicated test conventions and its own explicit mapping decision in `mark_duplicate`'s MCP error-code translation (another place this tension would resurface), for no caller-visible benefit.

## Consequences

- `TransitionGuard`'s single `ErrDuplicateOfInvalidTarget` return covers two distinct underlying conditions (genuinely missing, or exists-but-already-duplicate). Any future work that needs to distinguish them (e.g. a more specific UI error message) will need to re-derive the distinction by calling `GetBacklogItem` on the target directly, rather than by inspecting the guard's sentinel — this is a known, accepted limitation of this decision, not an oversight.
- Test coverage (plan.md Task 1.1.2d) explicitly asserts both conditions return `ErrDuplicateOfInvalidTarget`, documenting this folding decision in the test suite itself so it is not accidentally "fixed" into a 4th sentinel later without re-reading this ADR.
