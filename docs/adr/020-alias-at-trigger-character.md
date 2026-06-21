# ADR-020: Use `@` as Alias Trigger Character, Scoped to Start-of-Input

**Status**: Accepted
**Date**: 2026-06-20

---

## Context

The alias feature needs an unambiguous trigger character in the omnibar. Two existing uses of `@` exist in the codebase:

1. `WorkflowDetector` (priority 25) matches `@slug [arg]` — registered dynamically in `OmnibarContext.tsx`
2. `PathWithBranchDetector` (priority 50) matches `/path/to/repo@branch-name` — `@` appears mid-string

`PathWithBranchDetector`'s regex `^(.+)@([a-zA-Z0-9_/.-]+)$` requires at least one character before `@`. An alias invocation `@alias-name` always starts with `@` at position 0. The two uses cannot collide because the positional constraint is structural.

`WorkflowDetector` and `AliasDetector` both use start-of-input `@`. They must remain disjoint: workflow slugs come from the Workflow DB; alias names come from `config.json`. Priority resolves ties — `AliasDetector` is given priority 36 (after `WorkflowDetector` at 25, before `GitHubShorthandDetector` at 40). When a slug matches both a workflow and an alias, the workflow wins.

---

## Decision

Use `@` as the trigger character for aliases, scoped to start-of-input only.

`AliasDetector` fires only when the input begins with `@`, is followed by a valid name (`[\w-]+`), and contains at least one space (indicating the user has committed to an invocation, not just completing a name). Bare `@slug` without a trailing space is completion mode, handled by `useAliasSuggestions` alongside `WorkflowDetector`.

When a workflow and alias share the same name: **the workflow wins** (lower priority number = higher precedence). This is intentional — workflows are dynamic scheduled actions; aliases are simple session presets. An identically-named workflow signals intentional override.

---

## Consequences

**Positive**:
- Consistent UX — users already expect `@` to invoke a named thing (workflows already use this)
- No collision with `PathWithBranchDetector` due to positional constraint
- The `@` prefix is memorable: same semantic as GitHub `@mention` and Slack `/command`

**Negative**:
- If a user names both a workflow and an alias identically, the workflow always wins silently
- Mitigated: alias names validated at config load (`^[\w-]+$`); collision is visible at config write time

**Future risk**:
- If priority order between workflows and aliases ever needs to flip, it is a breaking UX change
- Document in user-facing help: "alias names that conflict with workflow slugs are shadowed by the workflow"
