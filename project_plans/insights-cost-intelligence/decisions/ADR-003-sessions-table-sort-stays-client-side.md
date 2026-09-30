# ADR-003: New Sessions-Table Sort Columns Stay Client-Side; `ListSessionTokens` Is Not Wired Up

**Date**: 2026-09-03
**Status**: Accepted

## Context

`ListSessionTokens` already implements server-side `sort_by` (`"cost"|"tokens"|"date"`)
and offset pagination (`server/services/insights_service.go:353-499`), but has zero
frontend callers today (confirmed by grep — only a feature-registry `rpcId` string
references it). `SessionsTable.tsx` instead does a full client-side scan: `Fuse.js`
fuzzy search over every session in memory, then an in-memory sort
(`SessionsTable.tsx:86-136`). `requirements.md`'s Rabbit Holes section left this as
an explicit open question for Phase 3 to resolve rather than discover mid-implementation:
does richer sort/search finally wire up `ListSessionTokens`, or extend the existing
client-side path?

## Options considered

1. **Wire the sessions table to `ListSessionTokens`**, extending its `sort_by` switch
   with the four new derived columns (`duration`, `cost_per_message`, `cache_roi`,
   `waste_score`) server-side.
   - *Strength*: uses the RPC that was already built for this purpose; sorting
     happens once, server-side, over data already resident in `TokenStore`.
   - *Weakness*: `research/pitfalls.md` §3 documents two real problems this
     creates that the current architecture doesn't have: (a) Fuse.js needs the
     full candidate set to fuzzy-rank, so a server-paginated response silently
     regresses today's full-dataset search to page-scoped search unless search
     is *also* moved server-side (new proto/backend surface, not currently
     scoped) or made mutually exclusive with sorting (a real UX mode-switch, not
     a transparent upgrade); (b) offset pagination combined with a sort key that
     can change between page fetches (live sessions still writing JSONL) is the
     well-documented "page drift" bug class (Stripe/GitHub API docs both warn
     about this for time-ordered-but-mutable collections) — a session could
     appear on two pages or be skipped entirely across a live-updating dashboard.

2. **Extend the existing client-side sort/search in `SessionsTable.tsx`** with the
   four new derived columns, computed the same way `duration`/`cost_per_message`
   already would be — one-line derivations from fields already on each
   `SessionTokenSummary` in memory (the same array the table already receives in
   full from `GetInsightsSummary`, independent of `ListSessionTokens`).
   - *Strength*: no page-drift risk (there is no pagination to drift), no
     search/sort mode-switch UX to design, and no new proto/backend surface
     beyond what Epics 1–2 already add (`cache_roi_usd`, `waste_score` on
     `SessionTokenSummary`). At the NFR's stated scale (~600 sessions, tens of
     millions of tokens for a single operator), an in-memory `Array.sort` over
     already-fetched data is not a performance concern.
   - *Weakness*: leaves `ListSessionTokens`'s server-side sort/pagination
     permanently unused for the sessions table — dead-but-tested code, not
     dead-and-untested, but still a maintenance question for a future pass if
     session count ever grows enough that fetching the full set becomes
     expensive (not indicated by anything in this project's NFRs).

## Decision

**Option 2.** The four new sort columns are added as client-side comparator cases
in `SessionsTable.tsx`'s existing sort `useMemo`, following the exact
missing-value-bucketing precedent the current `"cost"` column already
establishes (`SessionsTable.tsx:113-120`). `ListSessionTokens` remains
implemented and tested server-side (useful for a future non-table consumer, or if
this decision is revisited at a larger scale) but is not called from the sessions
table in this pass. This directly resolves `requirements.md`'s open question
("Whether `ListSessionTokens` server-side sort/pagination replaces the current
client-side Fuse.js search entirely, or the two need to coexist") in favor of the
option that adds zero new page-drift/search-scope risk for a workstream whose own
NFR explicitly says current scale doesn't need it.

## Consequences

- No `ListSessionTokens` proto/backend changes are needed for Epic 3 (richer
  sort/search) — this ADR is purely a frontend-architecture decision.
- If a future project needs true server-side pagination (session count grows an
  order of magnitude, or a non-table consumer needs `ListSessionTokens`), revisit
  this ADR rather than silently bolting server-side sort onto the table without
  addressing the search-scope and page-drift problems named above.
- `duration`, `cost_per_message`, `cache_roi`, and `waste_score` each need their
  own missing-value handling in the client-side comparator (see
  `research/features.md` §4) — `waste_score` in particular needs a third bucket
  ("not evaluated" vs. "evaluated as 0" vs. "unpriced") distinct from the
  two-bucket pattern `cost` already uses.
