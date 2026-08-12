# ADR-002: No pagination for the per-session breakdown table (MVP)

## Context

`requirements.md` explicitly deferred the question of whether the per-session breakdown table needs pagination to Phase 3 planning, rather than making a call. The table's row source is a `GROUP BY session_id` query, so its size is bounded by *distinct sessions with at least one matching `EscapeEvent` in the filtered time range* — not by total event count.

## Decision

No pagination for MVP. Render the full per-session breakdown result set client-side, matching the existing Insights Dashboard precedent (`web-app/src/app/insights/SessionsTable.tsx`), which also renders its per-session table unpaginated.

## Consequences

- Frontend table component (`SessionEscapeBreakdownTable.tsx`) can be built now without a cursor/`LIMIT` param on the RPC, and without client-side virtualization — reduces MVP scope.
- If real-world distinct-session counts grow large enough to make this impractical (e.g. thousands of one-off sessions each contributing a handful of events), the fix is additive: a `LIMIT`/cursor added to the existing `GroupBy(session_id)` query, or client-side virtualization — neither requires a proto or handler redesign, since the row shape (`SessionEscapeSummary`) is unaffected either way.
- Follow-up trigger, not a blocking condition: if `len(PerSession)` commonly exceeds roughly 200 in practice, revisit pagination or virtualization for the breakdown table.

## Alternatives Considered

- **Add pagination now** (cursor or offset-based, mirroring `useEscapeEvents`'s existing `nextPageToken` pattern): rejected for MVP. `ux.md` and `architecture.md` both lean toward "not needed at launch" given the row-count bound described above, and building pagination now would add proto fields, hook complexity, and UI (page controls) for a scale problem not yet observed. Revisiting later is a small additive change, not a rework, so deferring costs little.
