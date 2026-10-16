# ADR-003: Dedicated Exec-Gate Fast Lane, Not a Raised Shared Slot Count

**Date**: 2026-08-13
**Status**: Accepted
**Project**: terminal-resync-reliability

## Context

`requirements.md` Scope item 3b asks for an "exec-gate capacity/fast lane" so a resync
storm cannot starve ordinary tmux operations sharing the same flock-based gate. Tracing the
call chain (`session/tmux/exec_gate.go`'s `AcquireExecSlot`/`runGated`, called from
`session/tmux/tmux.go`'s `CapturePaneContent()`/`RefreshClient()`, keyed on
`t.serverSocket`) confirmed all tmux subprocess calls for a given server socket — resync or
not — already funnel through one `AcquireExecSlot` pool sized by
`TmuxExecGateConfig.SlotsOrDefault()` (default 8, `config/types.go:84-103`).

## Decision

Give resync-triggered capture/refresh calls their own, separate slot pool: a new
`AcquireResyncExecSlot(ctx, serverSocket)` keyed on `serverSocket + "#resync"` via the
existing, unmodified `gateDir` function (which already sanitizes whatever string key it's
given), sized by a new `ResyncFastLaneSlots` config field (default 4). This pool is
threaded through new `CapturePaneContentPriority()`/`RefreshClientPriority()` methods on
`TmuxSession`, and forwarded through `ProcessManager`, `Instance`, and `panePTY`/
`shellPanePTY` — four mechanical layers, each mirroring an existing sibling method,
matching this codebase's established layered-interface pattern (no new interface
introduced; see `.claude/rules/interface-pollution-checklist.md`).

## Alternatives Considered

| Alternative | Rejected because |
|---|---|
| Raise `defaultTmuxExecGateSlots` globally | Simplest one-line change, but affects *every* tmux operation on the socket, not just resync — exactly the blast-radius failure requirements.md's Rabbit Holes section warns against; does nothing to prevent a resync burst from still starving other resync-unrelated calls that share the same, now-larger pool |
| In-process `golang.org/x/time/rate.Limiter` | Doesn't coordinate across the multiple OS processes that already share the flock-based gate for the same tmux server socket — this repo's exec-gate is explicitly flock-based because tmux capture/refresh calls can originate from more than one process |
| A `bool priority` parameter on the existing `CapturePaneContent`/`RefreshClient` methods | Same-typed-parameter smell flagged by `.claude/rules/primitive-obsession-checklist.md`; a distinct method name (`...Priority`) is unambiguous at every call site instead of a boolean that could be silently flipped |

## Consequences

- The fast lane's 4-slot default (`ResyncFastLaneSlotsOrDefault()`) is a placeholder
  pending real burst-size telemetry (Unresolved Question #2 in `implementation/plan.md`)
  — it is a runtime config value, re-tunable without a redeploy.
- Four files gain two new, mechanically-forwarding methods each
  (`session/tmux/tmux.go`, `session/process_manager.go`, `session/instance_tmux.go`,
  `server/services/connectrpc_websocket.go`) — an accepted small maintenance surface
  increase in exchange for isolation.
