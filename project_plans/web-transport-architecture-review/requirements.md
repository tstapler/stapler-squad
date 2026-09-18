# Requirements: web-transport-architecture-review

**Date**: 2026-08-22
**Type**: architecture review (existing system)
**Complexity**: 3 — system design

## Problem Statement

stapler-squad's browser-facing transport layer has grown organically across at least three
independent subsystems, each solving its own streaming/proxying problem with its own protocol:

- **Terminal streaming** (`session/streamhub/`) — a per-session hub/transport abstraction
  (`hub.go`, `batch.go`, `ownership.go`, `resize.go`, `subscriber.go`) shipped very recently
  (`project_plans/terminal-multi-connection-streaming/`) to fix a resize/capture race across
  concurrent connections. It is mid-rollout behind a feature flag with dark-launch/rollback
  mechanics already in place.
- **General RPC** (`server/services/connectrpc_websocket.go`, 3192 lines) — a hand-rolled bridge
  that speaks ConnectRPC's own wire envelope (5-byte header + protobuf) over a raw
  `gorilla/websocket` connection instead of ConnectRPC's native HTTP/2 streaming transport.
- **CDP/VNC proxying** (`cdp_stream_handler.go`, `vnc_proxy_handler.go`) — separate WebSocket
  proxy paths for browser-devtools-protocol streaming and VNC screen sharing.

No one has evaluated whether this three-way split, and WebSocket specifically as the common
carrier, is still the right foundation now that the terminal-streaming rebuild is done and the
requester has surfaced three external references worth comparing against: **Apache Iggy**
(Rust message-streaming engine), **yetty** (`github.com/zokrezyl/yetty`), and the **W3C
WebTransport** explainer (QUIC/HTTP-3-based bidirectional browser transport, a WebSocket
successor). This project answers: given what these three actually offer, and given the
subsystems above, what — if anything — should change?

## Baseline

Today, each subsystem picked its own transport independently, with no shared abstraction other
than "it's a WebSocket connection." `session/streamhub/`'s new `Transport` interface is the one
place a deliberate abstraction exists, and it currently has three implementations: browser
WebSocket, ssq-mux Unix socket, and an in-memory test transport. The RPC bridge and CDP/VNC
proxies are untouched by that work and remain direct `*websocket.Conn` consumers. No prior
research in this repo evaluates Apache Iggy, yetty, or WebTransport specifically — the closest
prior art (`project_plans/terminal-multi-connection-streaming/research/exotic-transports.md`)
evaluated Chrome IWA Direct Sockets and WASM terminal clients and ruled both out as not
applicable; it does not cover this project's three references.

## Users / Consumers

- The stapler-squad web UI (`web-app/`) — browser tabs using terminal streaming, RPC calls, and
  (for sessions with browser automation) CDP/VNC streams.
- External IDE terminals via ssq-mux (`.claude/docs/pty-multiplexing.md`).
- The Go server itself, as the maintainer of ~4,100+ lines of hand-rolled
  transport/framing/coalescing code (`server/services/websocket_transport.go`,
  `connectrpc_websocket.go`, `ws_stream_bridge.go`, plus `web-app/src/lib/transport/`) that this
  review may recommend simplifying, replacing, or leaving alone per-subsystem.

## Success Metrics

- A written recommendation, per subsystem (terminal streaming / RPC bridge / CDP-VNC proxying)
  and per external reference (Iggy / yetty / WebTransport), stating: applicable or not, why, and
  what triggering condition would change the verdict — the same rigor as the prior
  `exotic-transports.md` research doc.
- For any recommendation marked "adopt," a scoped `plan.md` (task breakdown, migration/rollout
  approach consistent with this repo's flag-gated dark-launch convention, and a rollback path) —
  ready to execute in a later implementation session. No code changes ship in this pass.
- Explicit call-out of anything the review finds that duplicates or could retire code from the
  just-shipped `session/streamhub/` work, since that code is still mid-rollout and any
  recommendation touching it must not contradict its own dark-launch/rollback plan.

## Appetite

Medium (1–2 weeks) for the research + plan artifacts in this pass. No code is implemented in
this pass regardless of what the plan recommends — a follow-on session executes it.
*(Scope must fit the appetite. If it doesn't fit, cut scope — do not move the deadline.)*

## Constraints

- `session/streamhub/`'s dark-launch rollout is live and mid-trial; this review must not
  recommend anything that destabilizes it before its own rollback-rehearsal/trial-period gate
  (`project_plans/terminal-multi-connection-streaming/implementation/plan.md`) has run its
  course. Any recommendation touching terminal streaming must explicitly say how it interacts
  with that in-flight rollout.
- Single-operator, no-staging-environment production deployment (same constraint as the prior
  streaming project) — any adoption plan needs a real rollback path, not just "revert the
  commit."
- Browser client compatibility: any new transport must work from a normal browser tab with zero
  install step (this repo's `exotic-transports.md` research already rejected Chrome IWA Direct
  Sockets on exactly this ground — WebTransport's own browser/HTTP-3 support surface must be
  checked against the same bar in Phase 2).

## Non-functional Requirements

- **Performance SLO**: not formally specified; qualitatively, must not regress terminal redraw
  latency or input responsiveness (same bar as `terminal-multi-connection-streaming`).
- **Scalability**: single-operator instance, small concurrent-connection counts — this is about
  architectural fit and maintainability, not high-throughput multi-tenant scale. Apache Iggy is
  built for high-throughput distributed message streaming; Phase 2 research must explicitly
  assess whether that's solving a problem this product actually has at this scale.
- **Security classification**: internal/personal-use tool; no new external network exposure
  implied unless a specific recommendation calls for it, in which case it needs its own review
  (same precedent as the Workspace Host Registry ADR cited in the prior project).
- **Data residency**: not applicable.

## Scope

### In Scope
- Evaluate Apache Iggy, yetty, and WebTransport against each of: terminal streaming
  (`session/streamhub/`), the RPC bridge (`connectrpc_websocket.go`), and CDP/VNC proxying.
- Assess whether the current split across three independently-evolved subsystems should be
  unified (e.g., all three riding the same transport abstraction) or is justified by their
  differing requirements.
- Produce research docs per Phase 2 dimension and a plan.md for any recommended adoption.

### Out of Scope
- Implementing any recommendation — this pass produces research + plan artifacts only.
- Re-litigating `session/streamhub/`'s already-decided design (hub ownership model, resize
  negotiation, batching approach) — those are settled by its own ADRs
  (`project_plans/terminal-multi-connection-streaming/decisions/`); this review only asks
  whether a different *transport* underneath that design is warranted.
- Re-evaluating Chrome IWA Direct Sockets or WASM terminal clients — already covered by
  `exotic-transports.md`; carry that verdict forward rather than redoing it.

## Rabbit Holes

- **Apache Iggy is a standalone server product**, not a library — "adopt Iggy" would mean
  running and operating a separate message-streaming service, a materially different
  operational commitment than a Go package or protocol swap. Phase 2 must make this cost
  explicit rather than comparing it as if it were a drop-in library choice.
- **yetty is unfamiliar** — no prior knowledge of what it does or solves. Phase 2 research must
  establish what it actually is before any comparison is possible; do not assume relevance from
  the name/URL alone.
- **WebTransport browser/infra support** — it requires HTTP/3 (QUIC) termination, which is a
  different deployment shape than this repo's current plain HTTP(S)/WebSocket server
  (`localhost:8543`, self-signed TLS for remote access). Phase 2 must check whether Go's
  stdlib/ecosystem WebTransport server support and this repo's existing TLS/cert setup
  (`.claude/docs/codesigning.md`-adjacent remote-access config) are compatible before treating
  it as viable, not after.
- **Scope creep across three subsystems at once** — evaluating three technologies against three
  subsystems is a 3x3 matrix; Phase 3 planning must resist producing a maximal "redo everything"
  plan and instead scope adoption per-subsystem based on where each technology actually fits.

## Alternatives Considered

- **Narrow the review to terminal streaming only** (the subsystem most directly tied to the
  references) — rejected per explicit requester direction to cover all three subsystems, since
  the RPC bridge's custom envelope-over-WebSocket design and the CDP/VNC proxies are equally
  plausible candidates for a WebTransport-style improvement and have received no comparable
  review to date.
- **Skip the plan.md and stop at a recommendation doc** — rejected per explicit requester
  direction; the requester wants a scoped plan ready to execute for any adopted recommendation,
  not just a writeup.

## Feasibility Risks

- Go ecosystem WebTransport server support maturity is unverified — Phase 2 must confirm
  library options and their production-readiness rather than assuming parity with the browser
  API's own maturity.
- Apache Iggy's Go client library maturity/existence is unverified.
- What yetty actually does is unknown pending Phase 2 research; it is possible the honest
  verdict is "not applicable" the same way IWA Direct Sockets and WASM clients were ruled out in
  the prior project — Phase 2 must reach that conclusion on evidence, not assume it either way.

## Observability Requirements

Not applicable — this pass produces no running code. Any Phase 3 plan.md for an adopted
recommendation must define its own observability requirements consistent with
`terminal-multi-connection-streaming`'s precedent (structured logging, per-session/per-connection
visibility, flag-state visibility for dark-launch).

## Risk Control

Not applicable to this pass (no code ships). Any Phase 3 plan.md for an adopted recommendation
must follow this repo's flag-gated dark-launch convention
(`STAPLER_SQUAD_USE_CONTROL_MODE`/`session/streamhub`'s own flag as precedent) with an explicit
rollback path, especially for anything touching `session/streamhub/` while its own rollout is
still in trial.

## Open Questions

- What is yetty, concretely, and does it solve a problem any of the three subsystems actually
  has? → Phase 2 research.
- Does WebTransport's HTTP/3 requirement fit this repo's deployment model (localhost dev server,
  self-signed-TLS remote access), or does it require infra changes this product doesn't want to
  take on? → Phase 2 research.
- Is Apache Iggy solving a throughput/durability problem this product has, or is it scale
  mismatched to a single-operator tool the way the prior IWA Direct Sockets research found
  mismatched packaging costs? → Phase 2 research.
- Should the RPC bridge's custom ConnectRPC-envelope-over-WebSocket design be replaced with
  ConnectRPC's native HTTP/2 streaming instead of (or in addition to) considering WebTransport —
  i.e., is the real gap "wrong transport" or "reinventing what ConnectRPC already provides
  natively"? → Phase 2 research.
- Does unifying all three subsystems onto one transport abstraction (extending
  `session/streamhub`'s `Transport` interface, or a new shared one) pay for itself, or do the
  subsystems' differing requirements (terminal: low-latency bidi frames; RPC: request/response +
  server-streaming; CDP/VNC: opaque byte-proxying) justify staying separate? → Phase 3 planning.
