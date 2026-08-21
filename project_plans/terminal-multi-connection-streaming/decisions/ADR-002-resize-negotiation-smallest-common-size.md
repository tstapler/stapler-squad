# ADR-002: Smallest-Common-Size Resize Negotiation With Capability-Gated Votes

**Date**: 2026-08-20
**Status**: Accepted
**Project**: terminal-multi-connection-streaming

## Context

tmux control-mode has no native concept of multiple resize authorities (requirements.md's
Rabbit Holes; confirmed by `research/architecture.md` §2a — the existing refcounted
`StartControlMode` guarantees tmux sees exactly one control-mode client, so resize/capture is
this project's problem to solve, not tmux's). `research/features.md` §1 surveyed tmux, mosh,
ttyd, GoTTY, Wetty, Zellij, upterm, tty-share, and VS Code Live Share for prior art on
N-viewer resize negotiation.

## Decision

The `StreamHub`'s `NegotiatedSize` — itself a `TerminalSize` value object (`{Cols, Rows int}`,
constructed only via the validating `NewTerminalSize(cols, rows int) (TerminalSize, error)`,
which rejects non-positive dimensions) — is the component-wise minimum (`min(cols)`, `min(rows)`)
across all live `ResizeVote`s from subscribers whose `SubscriberCapability.CanResize` is
`true`. `ResizeVote` and `RequestResize`'s parameter share this same `TerminalSize` type rather
than each inlining their own `{Cols, Rows int}` shape. A subscriber that has never voted
implicitly votes the hub's current `NegotiatedSize`
at the moment negotiation runs (not a hardcoded default), avoiding the GoTTY bug where a
never-resizing client permanently pins a stale/default size. Subscribers without `CanResize`
(e.g. a future read-only sink) never get a vote. Each subscriber's `Transport` still receives
its own framed output independent of the shared pane's dimensions (Zellij's per-client-render
decoupling), so a resize loser sees a correctly-redrawn pane at the negotiated size, not a
garbled one.

## Alternatives Considered

| Alternative | Rejected because |
|---|---|
| Authoritative-single-subscriber (VS Code Live Share's host-authoritative model) | Designed for a host + untrusted-guest trust asymmetry that doesn't exist here — every subscriber on a stapler-squad session is the same operator (or an equally-trusted IDE terminal); an arbitrary "host" pick would be an unjustified asymmetry |
| `largest`/`latest`-wins (tmux's other configurable options) | `largest` risks clipping smaller viewports' content unpredictably; `latest`-wins reintroduces a resize race indistinguishable from today's bug, just scoped to "most recent voter wins" instead of "most recent capturer wins" |
| No negotiation — first subscriber's size is permanent | Fails the realistic multi-device scenario (phone + desktop) named in requirements.md's Users/Consumers; a later-attaching desktop tab would be stuck at a phone-sized pane forever |

## Consequences

- A subscriber's requested size is not guaranteed to be honored — this is a deliberate,
  user-visible behavior documented in `research/ux.md` (§2): the fix for a mismatch is an
  honest indicator (Phase 4, Story 4.2.2), not a promise that resizing one tab never affects
  another's rendered pane dimensions.
- Read-only future sinks (audit/recording, SSE viewer) are structurally prevented from ever
  shrinking the shared pane for everyone else, by construction of the capability gate — this
  did not require any additional design work once `SubscriberCapability` existed.
- ssq-mux's own subscribers do not participate in this negotiation in this pass (ADR-004) —
  `MuxTransport` is attached with `CanResize: false`, so this negotiation model applies fully
  only among hub-native (browser + test) subscribers for now.
