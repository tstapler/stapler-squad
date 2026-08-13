# ADR-001: WatchSessions unconditionally suppresses hidden sessions

**Date**: 2026-06-15
**Status**: Accepted

## Context

`WatchSessions` streams a real-time session list to all connected clients. It currently sends all sessions in the initial snapshot and live-event loop with no `Hidden` filter. Triage and review sessions are created with `hidden: true` to prevent them from polluting the main session list. `ListSessions` already suppresses hidden sessions by default (line 798 guard). The mismatch means any client using `WatchSessions` sees hidden sessions.

Two options were considered:

**Option A** — always suppress hidden sessions (no proto change): mirrors `ListSessions` default behaviour. No API surface change. Simpler. Covers the three code paths: snapshot loop, live-event loop, and `EventsSince` replay.

**Option B** — add `bool include_hidden = 4` to `WatchSessionsRequest`: more flexible but no current use case requires it. Adds proto churn and a regeneration step.

## Decision

Option A: unconditionally suppress sessions where `inst.Hidden == true` from all three `WatchSessions` code paths. No proto change.

## Consequences

- Clients that currently rely on seeing hidden sessions via `WatchSessions` will no longer receive them. There are no known clients that depend on this (hidden sessions are backlog infrastructure only).
- The `EventsSince` replay path (which had zero filtering) gets consistent behaviour with the other two paths.
- If a future use case requires exposing hidden sessions to a specific client, extend `WatchSessionsRequest` with `include_hidden` at that time.
