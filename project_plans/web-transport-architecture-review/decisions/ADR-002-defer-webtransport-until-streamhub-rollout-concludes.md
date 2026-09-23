# ADR-002: Defer WebTransport Adoption Until the `session/streamhub` Rollout Concludes

**Date**: 2026-08-22
**Status**: Accepted (deferred, not rejected)
**Project**: web-transport-architecture-review

## Context

W3C WebTransport (bidirectional, multiplexed, QUIC/HTTP-3-based) is the one of the three
external references (Apache Iggy, yetty, WebTransport) that research found to be a genuine,
non-category-error transport candidate — unlike Iggy (standalone message broker, operational
scale mismatch, `research/stack.md` §1/§4, `research/pitfalls.md` §1) and yetty (client-side
GPU terminal renderer, not a transport at all, `research/stack.md` §2, `research/pitfalls.md`
§2), both of which are rejected outright with no plan.md warranted.

As of this research (2026-08-22), WebTransport reached **Baseline** browser support in March
2026 (Safari 26.4+ joined Chrome 97+/Firefox 114+), clearing this repo's "zero-install,
normal browser tab" bar (`research/stack.md` §3, `research/pitfalls.md` §3a) — an earlier
build-vs-buy.md draft's "no Safari support" rejection reason is **stale and incorrect** as of
this research date and is explicitly not relied on here. A production-used Go server library
exists (`quic-go/webtransport-go` on `quic-go/quic-go`, `research/stack.md` §4).

Despite clearing the browser-compatibility bar, three independent costs make immediate
adoption premature:

1. **New QUIC/HTTP-3/UDP listener with no other use in this product.** `server/server.go`
   runs exactly one `http.Server` today (TCP, either plain or `ListenAndServeTLS`).
   `webtransport-go`'s `webtransport.Server` wraps `quic-go`'s own `http3.Server`, which needs
   its own UDP socket bound alongside the existing TCP listener — additive infrastructure, not
   a drop-in transport swap (`research/architecture.md` §2c, §5).
2. **Certificate rotation incompatible with this repo's current CA-import model.**
   WebTransport's `serverCertificateHashes` API (the mechanism for trusting a self-signed cert
   without a public CA) requires the pinned certificate to be **≤14 days** valid, ECDSA
   (P-256) only, and rotated with the new hash redistributed to every client roughly every 13
   days (`research/pitfalls.md` §3b, citing the W3C WebTransport cert-constraints PR). This is
   the opposite operational shape from `server/tls.go`'s `EnsureNetworkTLSCerts` — a stable
   local CA imported **once** per client device, with per-network leaf certs renewed silently
   near their own (much longer) expiry. Closing this gap means either building new ≤14-day
   automated cert-rotation + hash-redistribution tooling (a new recurring operational burden
   this single-operator tool doesn't have today), or switching to a publicly-trusted CA
   certificate with public DNS reachability — in tension with this product's "internal/
   personal-use tool; no new external network exposure" NFR (requirements.md).
3. **Must not destabilize `session/streamhub`'s in-flight dark-launch rollout.**
   `session/streamhub`'s own rollback-rehearsal/trial-period gate
   (`project_plans/terminal-multi-connection-streaming/implementation/plan.md`) has not yet
   run its course (requirements.md's Constraints). Terminal streaming is the one subsystem
   where WebTransport's multiplexing win is even theoretically relevant (`research/stack.md`
   §5, `research/architecture.md` §2c) — introducing a second wire-transport variable during
   the hub's own trial period would make it impossible to attribute any dark-launch regression
   to "the hub redesign" vs. "the new transport," undermining the rollback-rehearsal's own
   signal quality (`research/pitfalls.md` §4b/§4c).

Additionally, `research/ux.md` §1-2 found **no currently observed problem** WebTransport would
fix at this product's actual scale: head-of-line-blocking avoidance is a low-loss-network,
low-concurrency non-issue for a single operator mostly on LAN/localhost/Tailscale, and the one
real (if narrow) win — QUIC connection migration surviving a Wi-Fi→cellular network-path
change — is unverified as an actually-felt pain point (no open bug/ADR names it, unlike the
reconnect-latency gaps `ADR-023-client-reconnect-browser-lifecycle.md` was scoped to close).

## Decision

**Defer WebTransport adoption for terminal streaming** (and do not pursue it for the RPC
bridge or CDP/VNC proxying at all — `research/architecture.md` §2c and `research/build-vs-buy.md`
§3 both find no fit there: the RPC bridge's server-streaming calls have a cheaper, lower-risk
fix in ADR-001, and its bidi calls plus CDP/VNC's opaque byte pipes get nothing WebTransport
uniquely offers). This project's plan.md contains **no implementation tasks** for WebTransport
— it is recorded as deferred, considered work in plan.md's Follow-on/Deferred Work section, not
as an Epic/Story/Task breakdown, consistent with requirements.md's Appetite (no code ships in
this pass regardless of recommendation) and the sequencing constraint above.

### Trigger conditions for revisiting this decision

All of the following should hold before WebTransport is re-evaluated for implementation:

1. **`session/streamhub`'s dark-launch trial has concluded successfully** — its own
   rollback-rehearsal/trial-period gate
   (`project_plans/terminal-multi-connection-streaming/implementation/plan.md`) has run its
   course, and the hub design is no longer mid-rollout. Any future WebTransport work for
   terminal streaming must ship as a **wholly new `streamhub.Transport` implementation**
   (`WebTransportTransport`, sibling to `WebSocketTransport`/`MuxTransport`) behind its own
   independent rollback flag, never as an in-place modification of an existing implementation
   (`research/pitfalls.md` §4c) — this is a hard design constraint for whenever this work is
   picked up, not just a sequencing note.
2. **Either automated ≤14-day cert rotation + hash redistribution exists, or a
   publicly-trusted-cert model becomes acceptable** for this product's remote-access story —
   i.e. one of the two paths named in `research/pitfalls.md` §3b has actually been built or
   the "no new external exposure" NFR has been explicitly revisited and relaxed. Treating
   WebTransport as a drop-in swap for the existing WebSocket TLS setup without resolving this
   first understates the real deployment change.
3. **A concrete, observed perceived-latency or reconnection problem exists that WebTransport
   would fix** — `research/ux.md` found no such problem today; this trigger requires an actual
   documented pain point (e.g. a bug report or user complaint about mobile network-switch
   reconnect visibility), not a theoretical HOL-blocking argument. Absent this, WebTransport's
   real infra/cert costs (triggers 1-2) are not justified by a demonstrated user need.

If WebTransport is ever adopted, `research/pitfalls.md` §3e's hard constraint must be honored:
any adapter **must use WebTransport's reliable streams, never unreliable datagrams**, for PTY
output — `session/streamhub/transport.go`'s `Transport` interface's ordering/reliability
contract (currently implicit, relying on every existing implementation being TCP/Unix-socket
backed) should be made explicit in its doc comment before any new implementation is written
against it, and a regression test added to `session/streamhub`'s suite asserting a
reordering/lossy transport is detected and fails loudly rather than silently corrupting
output (`research/pitfalls.md` §4a/§4c).

## Alternatives Considered

| Alternative | Rejected because |
|---|---|
| Adopt WebTransport now for terminal streaming, behind a flag, in parallel with the streamhub trial | Directly conflicts with requirements.md's Constraints — introducing a second wire-transport variable during the hub's own trial period undermines the trial's ability to attribute regressions to one change or the other |
| Adopt WebTransport now for the RPC bridge or CDP/VNC instead, as a "lower blast radius" starting point | `research/architecture.md` §2c and `research/build-vs-buy.md` §3 both find no benefit there — CDP/VNC are latency-insensitive opaque byte pipes and the RPC bridge's actual gap (§ADR-001) is cheaper to close without QUIC at all; adopting WebTransport there would be infra cost with no matching benefit |
| Reject WebTransport outright, same as Iggy/yetty | Not supported by the evidence — WebTransport clears the browser-compatibility bar that sank the prior IWA Direct Sockets research, has a real Go server library, and has one genuine (if currently unverified) UX win; treating it the same as a category-error (yetty) or scale-mismatch (Iggy) misrepresents the research |
| Build the cert-rotation automation now, as a prerequisite investment, even without a driving need | No observed problem justifies the engineering cost today (`research/ux.md`); building infrastructure for a hypothetical future need before a real trigger exists is the same anti-pattern `research/pitfalls.md` §1b warns against for Apache Iggy, applied to a different subsystem |

## Consequences

- No code changes ship for WebTransport in this pass, consistent with requirements.md's
  Appetite.
- `session/streamhub/transport.go`'s `Transport` interface and its three existing
  implementations (`WebSocketTransport`, `MuxTransport`, in-memory test transport) are
  unaffected — this decision recommends no changes to the hub's current design.
- A future session picking up trigger conditions 1-3 has this ADR as its starting context —
  in particular, the hard requirement that any WebTransport adapter be a new, independently-
  flagged `Transport` implementation with explicit reliable-delivery semantics, not a
  modification of an existing one.
- This decision does not block or gate ADR-001 (ConnectRPC native streaming for
  `WatchSessions`/`WatchReviewQueue`), which is fully decoupled from both `session/streamhub`
  and from any WebTransport consideration.
