# ADR-004: Session-Detail Route Uses `?sessionId=` Query Param, Not a `[sessionId]` Dynamic Segment

**Date**: 2026-09-04
**Status**: Accepted

## Context

`requirements.md` (Scope item 4) and `plan.md`'s Step-0.5 creative pass both specify a
**path-based** bookmarkable route, `/insights/session/[sessionId]`, and `plan.md`
explicitly rejects the query-param alternative: "(c) keep it a query-param-driven modal
only, no path segment — rejected outright, since the requirement is explicitly a
path-based, bookmarkable route." `research/stack.md` researched and endorsed the
dynamic-segment approach without checking it against `web-app/next.config.ts`'s
`output: "export"`.

During Epic 1.4 implementation, a real Playwright cold-navigation test against the
actual static export (`npx next build` + serving `out/` over a plain static file
server, not `next dev`) proved `/insights/session/[sessionId]` renders the dashboard
instead of the session-detail content on a true cold load — `generateStaticParams()`
can't pre-render an unbounded set of session IDs at build time, and a static host has
no server to resolve an unknown path segment at request time the way `next dev` or a
Node.js `next start` server would.

## Options considered

1. **Keep the dynamic-segment route**, and add `generateStaticParams()` returning
   every known session ID at build time.
   - *Weakness*: session IDs are runtime data (new sessions are created continuously),
     not known at build time — this can never produce a complete list, so cold
     navigation to any session created after the last build always 404s.

2. **Switch `output: "export"` to a Node.js server build** so dynamic segments resolve
   at request time.
   - *Weakness*: out of scope for this feature — the whole app is statically exported
     today (a documented, deliberate architecture choice per `docs/reference/`), and
     changing that build mode is a repo-wide change no story in this plan authorizes.

3. **Switch the route to a `?sessionId=` query param** on a static path
   (`/insights/session-detail/page.tsx`), following the existing precedent at
   `web-app/src/app/sessions/summary/page.tsx`.
   - *Strength*: a query string is resolved entirely client-side after the static
     shell loads — no build-time enumeration, no server round-trip, works identically
     for a session created one second ago or one year ago. Directly reuses an
     already-established pattern in this same codebase rather than inventing a new one.
   - *Weakness*: the URL is `/insights/session-detail?sessionId=X` rather than
     `/insights/session/X` — a query param reads as slightly less "canonical" than a
     path segment, though it is equally bookmarkable, shareable, and shows up
     identically in browser history.

## Decision

**Option 3.** The route lives at `web-app/src/app/insights/session-detail/page.tsx`
and reads `sessionId` via `useSearchParams()`. This reverses the path-based decision
`requirements.md` and `plan.md` made, but that decision was made without accounting
for `next.config.ts`'s static-export constraint — `research/stack.md` scoped its
recommendation to routing ergonomics alone and never cross-checked it against the
build mode. Verified via a real Playwright run against the actual static export
(not `next dev`) that cold navigation to `/insights/session-detail/?sessionId=X`
renders the session-detail content correctly, matching the same verification method
that caught the original bug.

## Consequences

- `FindingsPanel.tsx`'s per-finding "View session" action and
  `SessionDetailDrawer.tsx`'s "Open full page" link both target
  `/insights/session-detail?sessionId=<id>` (falling back to `conversationId` for an
  orphan session with no `sessionId`) rather than a `/insights/session/<id>` path.
- Any future route needing a per-item deep link under this app's static-export build
  should default to the same `?id=` query-param pattern
  (`sessions/summary`, now `insights/session-detail`) rather than a dynamic path
  segment, unless that route also revisits option 2 above.
- `next.config.ts`'s `trailingSlash: true` (part of the same static-export config)
  means the resolved URL and any `href` built against this route carry a trailing
  slash before the query string (`/insights/session-detail/?sessionId=X`) — tests and
  future callers constructing this URL by hand need to account for that, not assume
  `/insights/session-detail?sessionId=X` matches exactly.
