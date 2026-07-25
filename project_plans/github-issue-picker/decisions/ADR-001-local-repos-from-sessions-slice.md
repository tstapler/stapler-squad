# ADR-001: Local Repos Sourced from Redux sessionsSlice, Not a New RPC

**Date**: 2026-07-03
**Status**: Accepted
**Feature**: github-issue-picker

---

## Context

The `GitHubIssuePicker` requires an instant (< 100ms) first tier of repos derived from sessions and worktrees the user already has open. Two approaches were evaluated:

**Option A: `SearchGitHubRepos(local_only: true)` RPC**
The backend queries `s.storage.ListInstanceData()` for sessions with non-empty `GitHubOwner`, returns them as `GitHubRepoEntry` with `is_local: true`. The frontend fires this RPC on modal open.

**Option B: Read from Redux `sessionsSlice` (chosen)**
The Redux store already holds the complete set of sessions, including `Session.githubOwner` and `Session.githubRepo` fields (defined in `proto/session/v1/types.proto` lines 90–93). The store is populated by the `WatchSessions` stream and kept current. The frontend derives local repos synchronously from `useAppSelector(selectAllSessions)` inside `useGitHubIssuePicker`, with no network call.

---

## Decision

Use **Option B**: read local repos directly from the Redux `sessionsSlice` in the frontend.

---

## Rationale

1. **No network latency**: Reading from an already-loaded Redux store is O(n) in-memory work. The SLO of < 100ms is trivially met — no round-trip, no serialization.

2. **No new RPC surface**: Adding `local_only: true` to `SearchGitHubRepos` was only needed to serve the local tier. Removing that requirement simplifies the proto schema: `SearchGitHubReposRequest` has only `query` and `limit` fields, and the handler is a single `gh repo list` call with no conditional branching.

3. **Already proven pattern**: `useSessionRepoPaths` (`web-app/src/lib/hooks/useSessionRepoPaths.ts`) already derives `string[]` from `selectAllSessions`. The new `useGitHubIssuePicker` follows the same `useAppSelector(selectAllSessions)` + `useMemo` pattern, extended to extract `{ owner, repo }` pairs.

4. **Data freshness**: The WatchSessions stream keeps the Redux store current. Sessions added since page load are already reflected — no staleness concern.

5. **Reduced complexity**: The backend `SearchGitHubRepos` handler is now a pure GitHub-tier operation. It does not need `s.storage.ListInstanceData()` access, a `local_only` conditional, or deduplication logic. This makes the handler easier to test and reason about.

---

## Consequences

- The `SearchGitHubRepos` proto message does not include a `local_only` field. Any future caller needing local repos from the backend must use a different mechanism (e.g., add a dedicated RPC or read from the existing `ListSessions` endpoint).
- If a session's `githubOwner`/`githubRepo` fields are empty (e.g., local directory sessions with no GitHub remote), they are correctly excluded from the local repo tier by the `useMemo` filter.
- Users who open the picker before the WatchSessions stream has delivered the first snapshot will see an empty local tier until the stream connects. This is expected behavior (the "No local repos found" empty state handles it) and typically resolves within 500ms.

---

## Rejected Alternative: `SearchGitHubRepos(local_only: true)`

**Key weakness**: Adds a round-trip (even if fast, ~30–80ms SQLite + HTTP overhead) that cannot beat the zero-latency in-memory read. Also forces the proto schema to carry a flag that only exists to serve one transient UI concern.
