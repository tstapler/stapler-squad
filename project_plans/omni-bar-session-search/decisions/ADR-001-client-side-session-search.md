# ADR-001: Client-Side Session Search (No New RPCs)

Status: Accepted
Date: 2026-04-14
Deciders: Tyler Stapler

## Context

Session search in the Omnibar needs a data source for existing sessions. Two architectural options exist:
1. Client-side filtering of the Redux session store
2. A new `SearchSessions` RPC endpoint on the backend

The full session list is already loaded into the Redux entity adapter (`sessionsSlice`) via `useSessionService`'s `listSessions` call and `WatchSessions` streaming. The `selectAllSessions` selector gives zero-latency O(1) access to the complete session list.

The existing `session/search/` package is a BM25 engine over Claude _conversation history_ (JSONL files in `~/.claude/`). It is architecturally separate from the live session metadata store and unsuitable for this use case: BM25 requires exact token matches, has no consecutive-character or word-boundary bonuses, and would need server-side index maintenance for live session metadata. "squad" does not match "stapler-squad" through BM25 because the IDF weighting on short corpora produces near-identical scores.

## Decision

Session search operates entirely client-side, filtering the Redux session store with a fuzzy algorithm (Fuse.js). No new RPC endpoints, no proto changes.

## Rationale

1. **Zero additional latency.** The data is already in memory. A round-trip RPC adds 5-20ms per keystroke even on localhost. Client-side is synchronous and updates on every keypress.
2. **Data is already fresh.** The `WatchSessions` stream keeps Redux up to date in real time. A server search would be operating on the same data with an unnecessary network round trip.
3. **Scale is bounded.** Even at 500 sessions, Fuse.js Bitap runs in under 1ms. The worst-case scale for a solo-practitioner tool is well within JavaScript budget.
4. **No coupling to backend.** Session search is a UI navigation concern — which sessions the user wants to jump to is a function of what they see on screen, not what the server computes.
5. **Resilient during reconnect.** Client-side filtering continues working during brief WebSocket reconnects; server search would degrade silently.
6. **Consistent with existing pattern.** `SessionList.tsx` already does client-side filtering via a `filteredSessions` useMemo. This is an upgrade of that established pattern.

## Consequences

- `useSessionSearch(query)` is a pure frontend addition in `web-app/src/lib/hooks/useSessionSearch.ts`.
- Zero changes to proto files, server handlers, or the ConnectRPC layer.
- If session count grows to 10,000+ in a multi-user deployment, this decision should be revisited. The `ListSessionsRequest.search_query` field (proto field 4) exists as a future hook for a server-side `SearchSessions` upgrade path.
