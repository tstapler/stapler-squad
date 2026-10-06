# ADR-004: ssq-mux Becomes an Output-Only `Transport`; Resize-Authority Unification Deferred

**Date**: 2026-08-20
**Status**: Accepted (go on output unification, explicit no-go on resize unification this pass)
**Project**: terminal-multi-connection-streaming

## Context

`ExternalStreamer` (`session/external_streamer.go`) is already close to the target `Transport`
shape — an owner + consumer registry + ring buffer for catch-up (`research/architecture.md`
§2b). But its `SendResize` (line 250) forwards resize requests straight to the socket with zero
negotiation, and `session/mux/multiplexer.go:549-553` lets *any* connected ssq-mux client call
`SetWindowSize` directly — the same unmediated-race shape this project exists to fix for the
browser path, already present independently on the ssq-mux side. Folding ssq-mux's resize
authority into the hub would require rewiring `Multiplexer.handleClient`'s client loop, which
`research/pitfalls.md` §4b and requirements.md's Rabbit Holes both flag as a plausible
multi-week redesign of ssq-mux's existing multi-IDE-terminal behavior on its own — plus a
distinct trust-boundary mismatch (filesystem-permission-as-auth vs. middleware-enforced auth,
§4a) and a lifecycle mismatch (`Multiplexer.Shutdown()` currently tears down the tmux session
itself; a hub-attached transport must not conflate "I disconnected" with "the session should
die," §4c).

## Decision

**Go**: wrap `ExternalStreamer` as `MuxTransport`, satisfying the `Transport` interface and
attached to a session's `StreamHub` for **output only** — the hub's broadcast reaches ssq-mux
clients through `ExternalStreamer`'s own existing consumer-callback fan-out, with zero changes
to `session/streamhub/hub.go`. This satisfies the Success Metric ("formalizing ssq-mux as a
subscriber... happens by implementing one transport interface, with zero changes to the
hub/broadcast code") and the Scope requirement for a third real transport implementation.

**No-go, this pass**: `MuxTransport` is attached with `SubscriberCapability{CanResize: false,
CanWrite: false}`. `session/mux/multiplexer.go`'s existing direct `SetWindowSize` calls from
`handleClient` are left completely untouched. This means a session with both a browser
connection and an active ssq-mux attachment is **not** fully race-free after this pass — ssq-mux's
own resize race, which predates this project and is unchanged by it, still exists independently.

## Alternatives Considered

| Alternative | Rejected because |
|---|---|
| Full unification: rewire `Multiplexer.handleClient` to forward resize requests to the hub | Named explicitly as a possible multi-week redesign in its own right (Rabbit Holes, pitfalls §4b) — would make this pass's blast radius unbounded relative to its "foundational, phased" appetite |
| Don't build a ssq-mux transport at all this pass | Contradicts requirements.md's explicit Scope ("at least three transport implementations" naming ssq-mux Unix socket by name); the output-only version is cheap and low-risk to ship now |
| Silently claim the race is "fixed" for ssq-mux because it now nominally implements `Transport` | Would overclaim against the Success Metric's actual bar and against this repo's evidence-and-claims discipline — the residual gap is named explicitly in the plan's Unresolved Questions instead |

## Consequences

- This is a strict improvement over today, not a partial regression: today, ssq-mux's resize
  race is completely unmediated and undetected; after this pass, it is unchanged but now
  explicitly named, tracked (Unresolved Questions), and scoped as its own follow-on phase.
- The browser-vs-browser and browser-vs-in-memory-test-transport combinations are fully
  race-free after this pass; the browser-vs-ssq-mux combination is not, until the deferred
  follow-on phase lands.
- A future "ssq-mux resize-authority unification" phase should read this ADR plus
  `research/pitfalls.md` §4a-c before scoping its own work — the trust-boundary and lifecycle
  mismatches identified there are real design questions, not just the resize-routing mechanics.
