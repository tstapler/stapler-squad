# ADR-001: Single-Owner Actor-Model `StreamHub` Per Tmux Session

**Date**: 2026-08-20
**Status**: Accepted
**Project**: terminal-multi-connection-streaming

## Context

`streamViaControlMode` (`server/services/connectrpc_websocket.go:633`) gives every WebSocket
connection independent, unmediated authority over its session's tmux resize + capture-pane —
confirmed as the root cause of a live, reproduced corruption bug (requirements.md's Baseline,
2026-08-20). `session/tmux/control_mode.go`'s refcounted `StartControlMode`/`StopControlMode`
and `session/external_streamer.go`'s `ExternalStreamer` already implement this "single owner,
N fan-out subscribers" shape for output — just not for resize/capture, which is the actual gap.

## Decision

Introduce `StreamHub` (package `session/streamhub`): one instance per tmux session name,
created lazily on first attach via a `HubRegistry` (`xsync.Map`-backed get-or-create). The hub
is the sole caller of the session's `SetWindowSize`/`ResizePTY`/capture-pane surface. All
external requests (attach, detach, resize vote, input) become method calls into this owner,
never direct field mutation from N caller goroutines — Go's actor idiom, generalizing the
refcounting already present in `TmuxSession` to also cover resize/capture, which today sits
*outside* that guarded section.

**Dependency direction**: `session/streamhub` depends on a local `SessionController` interface
(scoped to exactly `SetWindowSize`/`ResizePTY`/`CapturePaneContent`/`StopControlMode`/
`Subscribe`-`UnsubscribeControlModeUpdates`), not the concrete `*session.Instance` type, and
never imports package `session`. `*session.Instance` satisfies `SessionController` structurally.
This is required, not stylistic: package `session` separately needs to import `session/streamhub`
(for `HubRegistry`/`StreamOwnershipLock`, ADR-003), so if `session/streamhub` also imported
package `session` for `Instance`, the two packages would import each other — a Go import cycle,
a compile failure. Defining the interface where it's consumed (per
`.claude/rules/interface-pollution-checklist.md`) makes `session` → `session/streamhub` a safe
one-way dependency instead.

## Alternatives Considered

| Alternative | Rejected because |
|---|---|
| Per-session mutex around just the resize+capture critical section (distributed lock) | Fixes the race but not the transport-coupling/"stream wherever" goal explicitly requested; kept internally as `StreamOwnershipLock`, not the top-level architecture |
| Generic in-process pub/sub broker as the top-level pattern | Solves fan-out, not the resize/capture mutual-exclusion problem, which is the actual root cause (`research/architecture.md` §3) |
| Extend `TmuxSession`'s existing refcounting to cover resize/capture directly, no new hub type | `TmuxSession` is a lower-layer primitive (raw control-mode subprocess ownership); resize negotiation and multi-subscriber batching are a distinct, higher-level concern that doesn't belong mixed into that type |

## Consequences

- `session/streamhub` is a new package with no dependents outside `server/services` (browser
  transport) and `session/` (ssq-mux transport, `Instance` integration) — it does not reach into
  `session/tmux` internals beyond the existing `SubscribeToControlModeUpdates`/`StartControlMode`
  public surface.
- Hub teardown must be reachable from both "last subscriber detached" and "flag flipped back
  while active" via the same code path (`ForceTeardown`), or a flag rollback can leak the
  underlying control-mode subprocess (`research/architecture.md` §6, failure mode 4).
- This does not, by itself, resolve the resize-negotiation model (ADR-002) or the dark-launch
  flag scoping (ADR-003) — those are separate decisions this hub design depends on.
