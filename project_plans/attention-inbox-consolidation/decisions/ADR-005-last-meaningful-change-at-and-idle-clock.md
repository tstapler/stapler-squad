# ADR-005: `LastMeaningfulChangeAt` for display; idle clock moves to `LastMeaningfulOutput`; `UpdatedAt` untouched

**Status**: Proposed (cause of the reload reset confirmed by Spike 0.1.2). Amended in repair iteration 3 and the Phase 4 patch (decision 2 no longer routes through the recorder).
**Date**: 2026-10-23
**Project**: attention-inbox-consolidation

## Context (VERIFIED against HEAD 9c6471773)

- `UpdatedAt` is a UI change-token plus a dirty stamp: `touchUpdatedAt()` (`session/instance_state.go:36`) exists so the web `upsertSession` reducer does not drop an update with an equal timestamp. It is bumped by transitions, pin, note, rename, restart, etc.
- `ToInstanceData()` writes `UpdatedAt: time.Now()` (`session/instance_serialization.go:110`), ignoring the snapshot, and DB partial updates also set `updated_at`. After a server restart the card shows time since last save.
- `UpdatedAt` is also the idle clock in the determiner (`session/review_queue_determiner.go:270,282` raw field reads; `:177` uses `Snapshot()`), so renames and status flips reset idleness.
- Consumers: sort ("Updated") `useFilteredGroupedSessions.ts:148`, `selectActiveSessionsSortedByUpdatedAt`, omnibar, `transition_gate.go` CAS on a different (`BacklogItem`) field.
- No reload-only write path was found. Research hypothesis: restart-driven; Spike 0.1.2 reproduces on an isolated instance.

## Decision

1. **Do not change `UpdatedAt`** (semantics, writers, or the line-110 overwrite). Zero regression risk for sort, reducer, CAS.
2. Add persisted `LastMeaningfulChangeAt` (new nullable column on the ent `Session` schema, `InstanceSnapshot` field, proto `last_meaningful_change_at`). It is advanced by the always-on `SessionActivity` (`Touch` / `touchLocked`), NOT by the `SessionEventRecorder` (ADR-004): the timer must keep working with `session_event_log` off. Advance sources, matching plan.md's Event Ownership table: new meaningful terminal output (at `Instance.UpdateTerminalTimestamps`' existing critical section, `session/instance_approval.go:107-115`, only when the promoted `ReviewState.UpdateTimestamps` (`session/review_state.go:217`) reports `changed`; `ReviewState` itself cannot call `touchLocked`), every `transitionTo` status change, `TurnEnded`, and lifecycle hooks. User input is not a source (a deliberate omission; add a `Touch` caller if the "Quiet" view proves too eager). Persistence rides the existing full-save cadence (`ToInstanceData` via `SaveInstances`: graceful shutdown and lifecycle saves), like `last_meaningful_output`; there is no hot-path DB writer. **Accepted loss window**: a crash (no graceful shutdown) can lose changes since the last full save, so the card timer may step backwards by that amount after a non-graceful restart; instances not `Started()` and the 4 s shutdown capture deadline are also in the window. Owner to confirm this trade-off (it replaces the earlier "survives restart via the recorder" wording). Backfill on first load = `LastMeaningfulOutput` else `CreatedAt`.
3. `SessionCard` shows "Last activity <relative>" (from `LastMeaningfulChangeAt`) and static "Created". Absolute time in `title` and in the drawer header (no hover on touch). Missing value renders "No activity recorded".
4. Determiner idle and WaitingForAgent grace clocks read `Snapshot().LastMeaningfulOutput`, not `UpdatedAt`, gated by live-settable flag `attention_idle_clock_output` (default off; flag-off is byte-identical) (fixes the latent raw-read violation at `:270,:282` and the mis-timing in both directions).
5. Client: the EXISTING `useNowTicker` hook (`web-app/src/lib/hooks/useNowTicker.ts`, default 30 s; extended with pause-when-hidden and refresh on `visibilitychange`; no new hook) plus one pure `formatRelativeTime(then, now)`; `formatTimeAgo` and SessionCard's local copy become thin wrappers/removed.

## Alternatives rejected

Make `UpdatedAt` semantic (breaks reducer dedup, sort, CAS); stop the line-110 overwrite (changes persisted semantics for every consumer; out of scope per owner).

## Spike 0.1.2 findings (2026-10-24): restart-driven only; no reload-only writer

Method: isolated instance (`PORT=62873`, `STAPLER_SQUAD_INSTANCE=claude-manual-test-2`, `STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-2`) built from branch `aic-spikes2`; session `bar-test` (program `bash`, Active); readings via `POST /api/session.v1.SessionService/GetSession {"id":"bar-test"}`; reload driven in a real browser (Playwright) on the open session pane, which opens the terminal stream (`streamViaHub` calls `MarkViewed()`, `server/services/connectrpc_websocket.go:2112`; also `:1320`, `:3609`).

| Step | `updatedAt` | `lastMeaningfulOutput` |
|---|---|---|
| after first server start | 19:27:43.298 | 2026-10-15 (loaded) |
| session opened in browser (first attach) | 19:27:43.298 (unchanged) | 19:28:47.230 (attach produced output) |
| two plain reloads, no restart (stream re-attached 2x) | 19:27:43.298 (unchanged) | 19:28:47.230 (unchanged) |
| server restart (kill, start) | 19:29:10.083 (= shutdown save time) | 19:29:20.400 (startup output) |

Findings:
- Reload-only: `updatedAt` and `lastMeaningfulOutput` did not move. `Instance.MarkViewed` (`session/instance_state.go:235`) writes only `LastViewed` and republishes the snapshot; it never touches `UpdatedAt`.
- Restart-driven writer: `ToInstanceData` writes `UpdatedAt: time.Now()` (`session/instance_serialization.go:110`) on the shutdown full save, so every Active/Hibernated session reloads with the shutdown time (the first start of the instance showed the same: Active and Hibernated rows at start time, Stopped rows unchanged). `lastMeaningfulOutput` is re-stamped by the first terminal output after startup.
- Client: `SessionCard.tsx:1272` renders `formatTimeAgo(session.updatedAt)` from the server value (`:486-490`); no mount-relative value.
- Not tested: the card value for a Claude session (a `bash` session was used), and `lastViewed` (the RPC returned null for it).

Verdict: no reload-only writer outside the `SessionActivity` owners. Tasks 4.1.1e and 4.2.2c are closed as not needed; the restart reset is fixed by Story 4.1.1 (persisted `LastMeaningfulChangeAt`, decision 2). No failing characterization test is committed (nothing to characterize).
