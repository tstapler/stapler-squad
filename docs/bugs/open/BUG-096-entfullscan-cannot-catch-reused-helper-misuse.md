# BUG-096: `entfullscan` can't catch a reviewed full-scan helper misused from a hot single-lookup call site [SEVERITY: Low]

**Status**: 🐛 Open (tracked gap, not a defect in the shipped analyzer)
**Discovered**: 2026-09-02, during the `entfullscan` static-analysis implementation
(`tools/lint/entfullscan`) that followed the `FindInstanceDataByID` full-table-scan fix.

## Problem Description

The bug that motivated `entfullscan` (`session/storage.go`'s old `FindInstanceDataByID`)
did not write a new unfiltered ent query — it called an *existing*, already-reviewed
full-scan helper (`EntRepository.ListWithOptions`, correctly unfiltered for its own
legitimate list-everything callers) from a single-ID-lookup call site inside a
60-second-cadence reconciliation loop. `entfullscan`'s AST-level detection only flags a
raw `.All(ctx)` call with no `.Where(...)` in scope; it can't see that a *call site*
using an already-filtered-looking helper is actually invoking something unfiltered
several frames away — that's a call-graph shape, not a syntactic pattern.

## Fix Approach

Tracked upstream as a proposed kibitzer check rather than solved here: a call-graph-aware
analysis that flags a hot/frequently-invoked call site reaching a function whose own
implementation is an unfiltered full scan. See tstapler/kibitzer#30.

## Related

- `tools/lint/entfullscan/analyzer.go` — the syntactic half of this bug class, which this
  analyzer does cover.
- `session/ent_repository.go`'s `FindByIDWithOptions` — the fix for the original instance
  of this bug.
