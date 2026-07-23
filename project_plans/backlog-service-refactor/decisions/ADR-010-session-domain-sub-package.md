# ADR-010: `session/domain` Sub-Package for Pure Leaf Types

**Date**: 2026-07-09
**Status**: Accepted
**Deciders**: backlog-service-refactor planning

---

## Context

The `session` package is imported by 65+ packages in the module. Several of those importers
(primarily `pkg/events` and `server/adapters`) only need pure domain types —
`BacklogStatus`, `AcCriterion`, `AcCriteriaJSON`, `ReviewOutcome` — but pulling in `session`
brings headless, git, tmux, and ent infrastructure alongside.

A sub-package `session/domain` would let these consumers import only what they need.

## Decision

Create `session/domain` as a sub-package containing pure leaf types with zero imports from
the `session` parent package (or any infra sub-package). Add transparent type aliases in
`session/` (`type BacklogStatus = domain.BacklogStatus`) to preserve backward compatibility
for all 65+ existing importers without requiring any changes in those files.

Only update the 2–3 importers that need only domain types (`pkg/events`, `server/adapters`)
to point at `session/domain` directly.

## Alternatives Considered

**Option A: Keep types in `session` root** — Avoids all migration risk; `session` is already
the domain package by convention in this codebase. Rejected because the requirements
explicitly mandate a `session/domain` sub-package, and the alias bridge removes the
migration cost.

**Option C: Big-bang rename of all 65+ importers** — Consistent, no aliases. Rejected
because the blast radius of a 65-file import change is too large for a behavior-preserving
refactor; it risks merge conflicts and makes the PR unreviable.

## Consequences

- `session/domain` MUST NOT import `github.com/tstapler/stapler-squad/session` (no cycle).
  Enforced via `goda reach` pre-flight and `go build` compile-time check.
- Type aliases in `session/` are transparent — callers see no API change.
- Future work: once all callers are migrated, the aliases can be removed in a follow-up
  cleanup; this ADR does not mandate that timeline.
- The `depguard no_server_in_core` rule in `.golangci.yml` already covers `session/domain`
  (it applies to `**/session/**`), so the new package inherits the correct lint constraints.
