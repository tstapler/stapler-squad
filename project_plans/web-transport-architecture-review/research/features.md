# Research: Feature Landscape (Agent 2)

Scope: comparable products, failure-mode gap analysis of the three subsystems, unstated
operator needs, and fit of Iggy/yetty/WebTransport against those needs. Does not duplicate
Agent 1's library-mechanics research — focused on *fit to need*.

## 1. Comparable products and their transport choice

| Tool | What it streams | Transport |
|---|---|---|
| [gotty](https://github.com/sorenisanerd/gotty) | PTY → browser (xterm.js) | Raw WebSocket, relay-only (server pipes TTY bytes out, browser input bytes in) |
| [ttyd](https://tsl0922.github.io/ttyd/) | PTY → browser (xterm.js), C port of gotty | Raw WebSocket via libwebsockets |
| [Wetty](https://deepwiki.com/butlerx/wetty) | SSH/login-shell → browser (xterm.js), Node.js server | Raw WebSocket bridged to a spawned `sshd`/`login` process |
| [code-server](https://github.com/coder/code-server) / Coder web IDE | VS Code + integrated terminal | WebSocket over TLS, password/token auth |
| Codex CLI app-server (comparable "app-server" pattern) | JSON-RPC control channel | One JSON-RPC message per WS text frame; server survives across reconnecting clients; separate `/healthz`/`/readyz` HTTP endpoints |

**Finding:** every terminal-in-browser tool surveyed uses a raw WebSocket as the wire
transport — none use WebTransport, SSE, or long-polling for the interactive path. This is
consistent with stapler-squad's own choice and is not itself evidence the choice is wrong;
it does mean "switch the terminal transport to WebTransport" would be a novel move with no
comparable production precedent in this space, not a catch-up move. All of them are simpler
than stapler-squad's split: one WS connection, one relay loop, no envelope/framing protocol,
no batching, no resync-on-reconnect (a gotty/ttyd reconnect just gets a fresh shell or
whatever `tmux`/`screen` state the shell itself preserves — none of them implement a
capture-pane resync-without-teardown protocol the way `session/streamhub` + `connectrpc_websocket.go`
do). stapler-squad's transport is materially more sophisticated than any of these tools' —
the added complexity buys reconnect-without-visible-garbling, which the comparables don't
attempt.

Sources: [gotty](https://github.com/sorenisanerd/gotty), [ttyd](https://tsl0922.github.io/ttyd/), [Wetty (DeepWiki)](https://deepwiki.com/butlerx/wetty), [code-server](https://github.com/coder/code-server), [Coder web IDE docs](https://coder.com/docs/coder-oss/latest/ides/web-ides), [Codex app-server guide](https://codex.danielvaughan.com/2026/04/15/codex-app-server-complete-guide/)

## 2. Failure-mode coverage today, per subsystem (from code, not docs)

### Terminal streaming (`session/streamhub/` + `server/services/connectrpc_websocket.go`)
Most mature of the three. Confirmed in code:
- **Backpressure**: `TestStreamHub_should_EvictSlowSubscriberUnderSustainedBackpressure_And_IncrementDropsCounterExactlyOnce` (`session/streamhub/failure_modes_test.go:119`) — a slow subscriber is evicted with a counted metric, not allowed to block the hub or grow memory unbounded.
- **Reconnect without full resync**: `connectrpc_websocket.go` sends an unconditional resize-nudge on every reconnect/handshake, quiescence-gates a `capture-pane` snapshot (300ms deadline, 100ms quiet window — `waitForQuiescence`, lines ~428–475, ~1264–1268) so a reconnect during an in-flight terminal reflow doesn't capture partially-redrawn content, and threads a `resync_id` so a client can request just a resync without dropping the stream (line ~2419).
- **Coalescing/framing**: a pooled buffer (`coalesceBufPool`) batches back-to-back frames into one WS write, capped by `streamhub.MaxBatchWindow` (20ms, `batch.go:14`) with an explicit `FlushReason` enum (opportunistic vs ceiling vs bypass) so control/quiescence messages skip batching latency.
- **Idle/dead-connection detection**: 30s `SetReadDeadline` on the handshake read (line 605–608); clean-close (`CloseNormalClosure`/`CloseGoingAway`) is distinguished from abnormal close throughout (not logged as an error).
- **Concurrency safety**: `TestOverlapInvariant_should_LogErrorAndIncrementMetric_When_TwoOwnersAreForciblySimulated` and a 1000-cycle attach/detach goroutine-leak test (`failure_modes_test.go:237,258`) plus a dedicated multi-session mass-reconnect integration test (`reconnect_storm_integration_test.go:151`).

### RPC bridge (`connectrpc_websocket.go`, general non-terminal RPC path)
Shares the same file/coalesce/deadline machinery as terminal streaming (same envelope
format, same 30s handshake deadline, same coalesce-with-cap pattern), so it inherits most of
the above. What's specific to it: a 5-byte-header + protobuf envelope hand-rolled to imitate
ConnectRPC's own wire format over a raw `gorilla/websocket.Conn`, at 3192 lines. That size
is itself the finding — it's reimplementing framing ConnectRPC's own HTTP/2 transport gives
for free, purely because a browser can't drive raw HTTP/2 frames from JS (see §4).

### CDP/VNC proxying (`cdp_stream_handler.go`, `vnc_proxy_handler.go`)
Materially thinner than the other two. Confirmed in code:
- Both are synchronous byte-pipe proxies: `wsConn` ↔ CDP process / `tcpConn` (VNC), each
  direction in its own goroutine, torn down together via a shared `context.Context` when
  either side closes (`vnc_proxy_handler.go:111-152`, `cdp_stream_handler.go:92-200`).
- **No reconnect/resync protocol**: unlike terminal streaming's capture-pane resync, a
  dropped CDP or VNC WebSocket just ends the tunnel — there is no `resync_id`-equivalent, no
  quiescence-gated resnapshot, no batching. A reconnecting client gets a brand-new proxy
  tunnel with no continuity, and (for VNC) presumably no application-level replay of
  whatever the remote VNC server considers "framebuffer state" beyond what a fresh RFB
  handshake gives it.
- **Backpressure**: no explicit slow-consumer eviction/counter comparable to streamhub's —
  these proxies rely implicitly on the underlying synchronous `ReadMessage`/`Write` pipe
  blocking (standard TCP-style backpressure) rather than an application-level buffer with a
  drop policy. That's not necessarily wrong for an opaque byte-relay (no framing to
  reassemble), but it also means a stalled downlink blocks the read goroutine rather than
  being observable/counted the way streamhub's drops are — no equivalent metric exists.
- Read/write buffer sizes are set per-handler (CDP: 4KB read / 128KB write; VNC: 32KB/32KB)
  but neither has a documented rationale in-file for those specific sizes.
- `isNetError`/timeout-vs-real-error distinction exists for CDP's input-read loop
  (`cdp_stream_handler.go:172-210`) to avoid logging a deliberate shutdown deadline as an
  error — same idiom as terminal streaming's clean-close handling, just narrower in scope.

**Net gap**: CDP/VNC proxying is the subsystem least equipped to survive the flaky/mobile
network conditions the operator's actual remote-access use case implies (§3) — no resync,
no reconnect continuity, no backpressure visibility. This is a legitimate "what should a
transport redesign handle" finding independent of which technology gets picked: whatever
lands under CDP/VNC should get at least backpressure visibility parity with streamhub, and a
deliberate decision on whether reconnect-without-teardown is worth building for these (VNC/CDP
byte-opaque streams may not have a clean "resync" concept the way a terminal capture-pane
does, so this may be a justified asymmetry rather than a bug — Phase 3 should say which).

## 3. Unstated operator needs (beyond requirements.md)

1. **Multiplexing many streams over one connection — real, currently unmet.** A single
   browser tab viewing one session opens *at least four independent WebSocket connections*:
   terminal (`useTerminalStream.ts`), RPC/watch-streams e.g. `WatchBacklogItems`
   (`watch-ws-transport.ts`), CDP (`CDPViewer.tsx`), and (by the same pattern) VNC. Each has
   its own reconnect/backoff state (`web-app/src/lib/utils/backoff.ts`'s
   `BackoffState`/`jitteredDelay` is shared code, but each consumer instantiates and drives
   it independently — there's no cross-connection "the network just came back, resume
   everything together" signal). On a flaky mobile connection this means up to 4 independent
   reconnect storms per drop instead of one. This is exactly the class of problem
   WebTransport's one-QUIC-connection/many-streams model and (in principle) a unified
   `streamhub.Transport`-style abstraction shared across subsystems would address — it is not
   addressed by any of today's three subsystems individually.

2. **Working across flaky/mobile networks — partially met, at real engineering cost.**
   Terminal streaming already hand-builds a lot of what a modern multiplexed transport with
   stream-level reliability would give for free: resync-without-full-reconnect, quiescence
   gating, coalescing. That investment is evidence the need is real (confirmed by
   `project_plans/terminal-multi-connection-streaming/`'s own motivation), but it's also
   evidence of the cost of solving it per-subsystem in hand-rolled Go rather than getting it
   from the transport layer. CDP/VNC don't have this investment at all (§2).

3. **Corporate proxies / restrictive networks.** Not discussed in requirements.md, but a real
   consideration for a single operator who may connect from a work network. Raw WebSocket
   (current state) traverses ordinary HTTP(S) proxies via the standard `Upgrade` mechanism
   and is broadly proxy-compatible. WebTransport's QUIC/HTTP-3 requirement runs over UDP/443,
   which is blocked or degraded on a non-trivial fraction of enterprise networks and some
   mobile carriers, **with no automatic fallback to TCP** — a materially different risk
   profile from WebSocket for exactly the "connect from wherever" use case this operator has.

4. **Tailscale + self-signed TLS remote access.** This repo already runs a real solution here:
   `server/tls.go`'s `EnsureNetworkTLSCerts` issues a stable local CA plus per-network leaf
   certs (one per LAN/Tailscale IP), so a phone imports the CA once and then trusts every
   network's leaf cert going forward — the documented mobile-app endpoint
   (`https://onyx.staplerhome.internal:8444`, private-CA-required) is this in production.
   `server/server.go`'s `StartRemote` binds that as a second HTTPS server on a plain
   `net.Listen("tcp", ...)` + `http.Server.ServeTLS` — no HTTP/3/QUIC involved anywhere in the
   current deployment shape. WebTransport's `serverCertificateHashes` API (lets a client pin
   a specific self-signed cert without OS/CA trust at all) is conceptually adjacent to this
   need, but since the operator's private-CA setup already works and is already deployed,
   WebTransport's cert story is a "nice, redundant with something that already works" fit,
   not a gap it uniquely closes — and adopting it would require standing up a UDP/443
   listener alongside (or instead of) the existing TCP `ServeTLS` path, a real deployment-shape
   change the current single-operator/no-staging constraint (requirements.md) should weigh
   carefully against the proxy/carrier-blocking risk in point 3, especially since Tailscale's
   own transport is itself UDP (WireGuard) — QUIC-over-UDP tunneled inside a WireGuard/UDP
   path is plausible but adds a second UDP-reachability dependency where today there is none.

## 4. Do Iggy / yetty / WebTransport map onto these needs?

### Apache Iggy — does not map onto any surfaced need
Iggy is a standalone Rust message-streaming *server* (topics/partitions/consumer groups,
high-throughput durable pub/sub) — adopting it means operating a second service, not
importing a library (confirmed, matches requirements.md's stated rabbit hole). None of the
three subsystems have a message-broker-shaped problem: there's no fan-out to independent
consumer groups, no durability/replay requirement beyond what a `capture-pane` snapshot
already gives for free, and no multi-tenant throughput need at single-operator scale. Its Go
SDK is TCP-only and blocking, and even Iggy's own ecosystem description (no Kafka
compatibility, no connector ecosystem, limited ops tooling — still Apache-Incubator-stage as
of Feb 2025) signals it isn't a mature drop-in even where message streaming *would* fit. This
is the same scale-mismatch pattern `exotic-transports.md` already found for Chrome IWA Direct
Sockets. **Verdict leans not applicable**, consistent across all three subsystems and all four
unstated needs in §3 — none of them are a durable-message-log problem.
Sources: [Apache Iggy](https://iggy.apache.org/), [iggy-go-client](https://github.com/iggy-rs/iggy-go-client), ["What the Heck is Apache Iggy?"](https://hackernoon.com/what-the-heck-is-apache-iggy)

### yetty — orthogonal to this review, not a transport at all
yetty (`github.com/zokrezyl/yetty`) is a WebGPU-accelerated **terminal emulator and
rich-content renderer** — it competes with xterm.js on the client-rendering side (plots,
images, video, documents rendered inline in the terminal surface), not with WebSocket,
WebTransport, or any network transport. It has no bearing on how bytes move from server to
browser, so it doesn't map onto any of §3's needs (multiplexing, flaky-network resilience,
proxy traversal, or the Tailscale/cert story) — those are all about the wire, and yetty
doesn't touch the wire. **Verdict: not applicable to this review**, for the same reason
`exotic-transports.md` ruled out WASM terminal clients — it's a client-rendering concern, not
a transport one. (It could be relevant to a *separate*, hypothetical "replace xterm.js"
project, but that's out of scope here and not implied by anything in requirements.md.)
Source: [yetty](https://github.com/zokrezyl/yetty), [yetty.dev](https://yetty.dev/)

### WebTransport — the one real transport-layer contender, with a genuine trade-off
The only one of the three that actually maps onto a §3 need:
- **Multiplexing (need #1)**: direct fit — one QUIC connection, independent streams, would
  let terminal + RPC + CDP + VNC share one connection and one reconnect state machine instead
  of four. This is the strongest positive finding in this doc.
- **Flaky networks (need #2)**: QUIC's per-stream loss recovery (no head-of-line blocking
  across streams, unlike TCP-backed WebSocket) is a plausible structural win for the resync
  machinery `connectrpc_websocket.go` currently hand-builds — but not a proven one without a
  prototype; this doc can't quantify the win, only note the direction.
- **Corporate proxies (need #3)**: negative — UDP/443 blocking with no TCP fallback is a real
  conflict with "operator connects from an arbitrary network."
- **Tailscale/self-signed TLS (need #4)**: neutral-to-mildly-positive but redundant with an
  already-working solution, and adds a second UDP-reachability dependency (see §3.4).

Library maturity (a named Feasibility Risk in requirements.md) is largely resolved:
`quic-go` is described as production-ready and `quic-go/webtransport-go` is actively
maintained (a release as recent as June 2026, implementing WebTransport draft-16). Browser
support only reached Baseline in March 2026 (Safari 26.4 was the last holdout) — recent
enough that the "zero-install-step, normal browser tab" hard constraint is only now met
across *current* browser versions, not necessarily whatever the operator's phone/work laptop
is pinned to. The remaining risk is genuinely deployment-shape (a UDP/443 listener is a
different shape than today's `net.Listen("tcp")` + `ServeTLS`, requiring either a second
listener alongside the existing HTTPS server or a bigger restructuring) plus the no-fallback
proxy risk — not "does a Go library exist."

Sources: [WebSocket vs WebTransport](https://websocket.org/comparisons/webtransport/), [quic-go/webtransport-go](https://github.com/quic-go/webtransport-go), [webtransport-go (adriancable)](https://github.com/adriancable/webtransport-go), [quic-go](https://github.com/quic-go/quic-go)

### Bonus finding: the RPC bridge's "reinventing ConnectRPC" open question is answered — no
requirements.md's open question ("is the real gap 'wrong transport' or 'reinventing what
ConnectRPC already provides natively'?") has a concrete answer from browser-platform
constraints, independent of the three references: **browsers cannot drive raw HTTP/2 frames
from JavaScript**, so native gRPC/ConnectRPC bidirectional streaming is not accessible from a
browser tab at all — only gRPC-Web's unary/server-streaming subset is, via a proxy. The
hand-rolled WebSocket envelope bridge in `connectrpc_websocket.go` exists to work around a
real browser-API gap, not to reinvent something ConnectRPC already exposes to browsers. This
doesn't mean the current 3192-line implementation is optimally sized (that's Agent 1's/Phase
3's call), but it means "just use ConnectRPC's native streaming instead" is not a viable
alternative for the browser bidirectional case — whatever replaces the bridge (if anything)
still needs to be WebSocket, WebTransport, or some other browser-drivable bidi transport, not
plain ConnectRPC-over-HTTP/2.
Source: [WebSocket vs gRPC](https://websocket.org/comparisons/grpc/), [Connect RPC FAQ](https://connectrpc.com/docs/faq/)

## Summary for Phase 3 planning

- Iggy: not applicable to any subsystem — scale/operational-cost mismatch, same pattern as
  prior IWA Direct Sockets rejection.
- yetty: not applicable — it's a terminal renderer, not a transport; doesn't compete in this
  review's problem space at all.
- WebTransport: the only real contender, and only for the multiplexing need — its
  UDP/443-no-fallback risk is a direct conflict with the operator's actual mobile/corporate
  network usage pattern (§3.3), so any "adopt" recommendation should be scoped as additive
  (WebTransport where available, WebSocket fallback always present) rather than a replacement,
  and should weigh the deployment-shape change (second UDP listener) against the size of the
  multiplexing win, which this research doc cannot quantify without a prototype.
- CDP/VNC proxying is the subsystem with the most room for improvement *regardless* of which
  transport technology is chosen — it lacks reconnect continuity and backpressure visibility
  that terminal streaming already has, and that gap exists independent of the Iggy/yetty/
  WebTransport question.
