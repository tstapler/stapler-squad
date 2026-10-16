# ADR-002: Extend `TerminalOutput` With `resync_id` Instead of Reviving `CurrentPaneResponse`

**Date**: 2026-08-13
**Status**: Accepted
**Project**: terminal-resync-reliability

## Context

A resync request (`CurrentPaneRequest`) needs a reply the client can match to the specific
request that triggered it — today the client clears its "pending resync" state on *any*
subsequent `TerminalOutput`, which is imprecise once multiple resyncs can be in flight
(AC2).

Two proto messages could plausibly carry this reply:

- `CurrentPaneResponse` (`proto/session/v1/events.proto:217-228`) — named for exactly this
  purpose, but a repo-wide search confirmed it has **zero construction sites and zero
  consumption sites** anywhere in `server/` or `web-app/`. It is dead code.
- `TerminalOutput` (`events.proto:124-126`) — the message the server already sends as the
  resync reply on both existing streaming paths (`streamViaTmuxCapturePane`'s mid-stream
  handler, `streamViaControlMode`'s post-resize snapshot), with 10+ real construction sites
  today.

## Decision

Add `string resync_id = 2;` to `TerminalOutput` and have both server handlers echo the
triggering request's `resync_id` on it. Do not construct or consume `CurrentPaneResponse`.

## Alternatives Considered

| Alternative | Rejected because |
|---|---|
| Revive `CurrentPaneResponse` | Dead code with no client-side handling to build on; reviving it means writing both server construction *and* client dispatch from scratch for zero benefit over extending the message already flowing on both paths |
| New dedicated `ResyncCompleteAck` message | Adds a third response shape to `TerminalData`'s oneof and a new client dispatch branch; no behavioral gain over reusing `TerminalOutput`, which the client already listens for on every resync today |

## Consequences

- `resync_id` is proto3 `string`, defaulting to empty — fully backward-compatible with
  clients/servers not yet aware of the field (see Risk Control in `implementation/plan.md`:
  empty `resync_id` falls back to today's any-output-clears-pending heuristic).
- `CurrentPaneResponse` remains unused; a future cleanup could remove it from the proto
  entirely, but that is out of scope for this project (not named in `requirements.md`'s
  Scope).
