# ADR-001: Adopt ConnectRPC Native Streaming for Server-Streaming-Only RPCs

**Date**: 2026-08-22 (revised 2026-08-22 during Phase 3 plan-repair, after the architecture
review and adversarial review both returned BLOCKED verdicts on the original scope)
**Status**: Accepted (revised scope — see Context for what changed and why)
**Project**: web-transport-architecture-review

## Context

`server/services/connectrpc_websocket.go` (3192 lines) hand-rolls ConnectRPC's own wire
envelope (5-byte header: 1 flags byte + 4-byte big-endian length, then a protobuf payload —
`marshalProtoEnvelope`) over a raw `gorilla/websocket.Conn`, backed by two `sync.Pool`s
purely to avoid per-frame allocation on that hand-rolled path. `server/services/ws_stream_bridge.go`
(143 lines, `StreamingWSBridge`) does the same envelope-over-WebSocket trick specifically for
the two server-streaming-only RPCs — `WatchSessions` and `WatchReviewQueue`
(`proto/session/v1/session.proto:29,54`) — registered ahead of the general Connect handler at
`server/server.go:419-427`.

Research (`research/stack.md` §5, `research/architecture.md` §3/§5, `research/build-vs-buy.md`
§0/§4) confirms two independent, verified reasons this bridge exists rather than using
ConnectRPC's native HTTP/2 streaming transport (`connectrpc.com/connect v1.19.0`, already a
direct dependency):

1. **No HTTP/2 on the plain `:8543` deployment.** `server/server.go`'s primary `:8543` server
   (`Server.Start`) runs over plain TCP (`listenLoopbackAware`); Go's `net/http` only negotiates
   HTTP/2 via TLS ALPN. Critically — confirmed only during this plan-repair pass, not the
   original planning pass — **no shipping browser implements cleartext HTTP/2 (h2c) at all**:
   Firefox's own tracking bug (Bugzilla #1418832, "Consider implementing h2c") is still open,
   and Chromium has never shipped it (crbug #1409512 / #580796). Without TLS, a native
   ConnectRPC streaming call from a real browser tab therefore always runs over HTTP/1.1,
   consuming one of a browser's 6 concurrent connections per origin for its entire lifetime
   (`ws_stream_bridge.go:16-19`'s own doc comment) — a real, **permanent** limitation of that
   listener, not a temporary gap closeable by server configuration.
2. **Browser bidirectional/client-streaming is a separate, harder constraint** (confirmed via
   ConnectRPC's own FAQ/discussion, `connectrpc/connect-go#254`): browsers cannot drive
   client-streaming or bidirectional request bodies reliably over `fetch()` across all target
   browsers, regardless of HTTP/2 availability. This is why terminal input
   (`connectrpc_websocket.go`'s `StreamTerminal`, a bidirectional RPC) and any other bidi RPC
   **must stay on the WebSocket bridge** — this ADR does not touch that path.

Reason 2 does **not** apply to `WatchSessions`/`WatchReviewQueue`: both are server-streaming
only, which ConnectRPC's native transport handles from a browser tab today — but reason 1 only
has a real fix on a listener that actually offers browser-supported HTTP/2, which is TLS-only.

### Corrected finding (this plan-repair pass): the TLS remote-access listener already has that fix, today, with zero code changes

`Server.StartRemote` (`server/server.go:1378-1424`) runs a second `http.Server` (`remoteSrv`)
with `TLSConfig` set and serves it via `ServeTLS`. It sets neither `Protocols` nor
`TLSNextProto`. Per Go's own `net/http` package documentation (verified via `go doc net/http`'s
"HTTP/2" section): *"Server ... automatically enable[s] HTTP/2 support when using HTTPS."* This
has been true since Go 1.6 — it is **not** a Go 1.24+ feature, and it requires no explicit
`http2.ConfigureServer` call or `Server.Protocols` wiring. This means the TLS remote-access
listener (`:8444` per project memory — the address the mobile app already connects to,
`https://onyx.staplerhome.internal:8444`) already negotiates real, browser-supported,
ALPN-based HTTP/2 today, for every request, with **no implementation work in this ADR's scope**.

### What the original version of this ADR got wrong

The original version proposed enabling `Server.Protocols.SetUnencryptedHTTP2(true)` on the
**plain** `:8543` listener instead, reasoning that Go 1.24+'s stdlib h2c support was a
lower-risk alternative to the now-deprecated `golang.org/x/net/http2/h2c` package. That framing
was correct about `h2c`'s stdlib mechanism (`golang.org/x/net/http2/h2c` really is deprecated in
its favor, confirmed by reading its package doc comment) but wrong about who could use it: **no
real browser speaks cleartext HTTP/2, regardless of which server-side mechanism enables it.**
Enabling it would have been a genuine no-op for every real client this product serves as its
primary/default deployment, while adding a server-wide protocol-negotiation change — risking the
`http.Hijacker`-dependent WS-upgrade paths (`StreamTerminal`, VNC, CDP) that share the same
listener's mux, since HTTP/2 requests in `net/http` don't support `Hijacker` the way HTTP/1.1
does. The original plan additionally proposed conditionally un-registering `StreamingWSBridge`
for the two RPC paths when a server flag was on; that step was independently unnecessary,
because `StreamingWSBridge.Handler()` (`ws_stream_bridge.go:44-62`) already forwards every
non-WebSocket-upgrade request straight through to the same Connect handler, and it actively
broke the plan's own staged rollout (enabling the server flag alone — the prescribed first
step — would have stranded every still-WS-upgrading client, the default). Both points are
recorded in full under Alternatives Considered.

## Decision

Adopt ConnectRPC's native HTTP/2 streaming for `WatchSessions` and `WatchReviewQueue`, scoped
**only** to the TLS remote-access listener (`Server.StartRemote`, `:8444`), where real
ALPN-negotiated HTTP/2 already works today with no server-side code change. The plain `:8543`
listener is explicitly **out of scope** — it keeps its current `StreamingWSBridge`
(WebSocket-upgrade) behavior unconditionally, forever, since that remains the only mechanism
that avoids tying up one of a browser's ~6 HTTP/1.1 connections per origin there.

Concretely:

- **No server-side change.** `server/server.go` is not modified by this decision.
  `StreamingWSBridge` stays registered at the exact `watchSessionsPath`/`watchReviewQueuePath`
  routes on **both** listeners, unconditionally — its own existing fallback behavior
  (`ws_stream_bridge.go:44-62`: forward every non-WebSocket-upgrade request straight to the
  wrapped Connect handler) is what serves a native-transport client correctly with zero routing
  change. No conditional bypass is introduced.
- **Frontend-only change, gated on the destination listener.** `useShells.ts`,
  `useReviewQueue.ts`, `useSessionService.ts` switch from `createWatchTransport` (the WS-bridge
  client) to `@connectrpc/connect-web`'s stock `createConnectTransport`, but **only** when the
  resolved `baseUrl` starts with `https://` — i.e., only when the page was actually served
  through the TLS remote-access listener. On the plain `:8543` listener, these hooks keep using
  `createWatchTransport` unconditionally, regardless of any build-time flag, because native
  streaming there would silently stay on HTTP/1.1 (no browser negotiates h2c) and reintroduce
  the exact connection-budget problem `StreamingWSBridge` exists to avoid.
- A single dark-launch flag, `NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING` (frontend only, default
  unset/false), gates *intent*; the `https://` runtime check gates *safety*. Both must hold for
  native transport to be used. There is **no server-side flag** — the original plan's
  `STAPLER_SQUAD_USE_CONNECTRPC_NATIVE_STREAMING` is dropped, since there is no server-side
  behavior left to gate.

`StreamTerminal` and any other bidirectional/client-streaming RPC are explicitly **out of
scope** and remain on the hand-rolled WebSocket envelope in `connectrpc_websocket.go` — this
decision does not attempt to replace that file's bidi path, per reason 2 above.

## Alternatives Considered

| Alternative | Rejected because |
|---|---|
| Enable cleartext HTTP/2 (`Server.Protocols.SetUnencryptedHTTP2(true)`) on the plain `:8543` listener (**this ADR's original decision**) | No shipping browser implements cleartext HTTP/2 — Firefox Bugzilla #1418832 (open), Chromium crbug #1409512/#580796 (unimplemented). The mechanism itself is real (Go 1.24+ stdlib; `golang.org/x/net/http2/h2c` correctly identified as deprecated in its favor) but delivers the claimed benefit to zero real browser clients on this listener, while being a server-wide capability change (not scoped to the two RPC paths) that risks the `http.Hijacker`-dependent WS-upgrade paths (`StreamTerminal`, VNC, CDP) sharing the same listener/mux — adversarial review BLOCKER, confirmed via web search and direct code read during this plan-repair pass |
| Conditionally skip `StreamingWSBridge`'s exact-path registration when a (now-dropped) server flag is on (**this ADR's original decision**) | Unnecessary and actively regressive: `StreamingWSBridge.Handler()` already forwards every non-WebSocket-upgrade request to the same Connect handler instance registered at the general subtree path, so native-transport clients are already served correctly with the bridge registered. Skipping registration instead breaks every still-WS-upgrading client (the default, since the frontend flag defaults off) — contradicting the original plan's own Staged Rollout claim that enabling the server flag alone was safe in isolation — architecture review BLOCKER, confirmed by direct code read of `ws_stream_bridge.go:44-62` |
| Enable TLS by default on `:8543` to get HTTP/2 via ALPN | Changes the "zero-install, no cert required" local-dev story this repo relies on; unnecessary now that the TLS remote-access listener already provides real HTTP/2 for the one deployment path where it actually matters |
| Leave `StreamingWSBridge` as-is indefinitely, with no native-transport adoption anywhere (status quo) | requirements.md explicitly asked this open question to be answered; on the TLS remote-access listener specifically, native ConnectRPC streaming is a real, already-available improvement (retires hand-rolled envelope-over-WebSocket handling for that path) with zero server-side implementation cost — the corrected decision captures this real, smaller win rather than discarding it along with the rejected h2c mechanism |
| Migrate `StreamTerminal` (bidi) to native ConnectRPC streaming too, in the same pass | Browsers cannot reliably drive client-streaming/bidi request bodies over `fetch()` (reason 2) — this is a hard browser-platform constraint, not an engineering-effort tradeoff; `research/features.md` §4's "Bonus finding" and `research/build-vs-buy.md` §4 both independently confirm this |
| Big-bang cutover (switch the frontend unconditionally, no flag/guard) | Violates this repo's flag-gated dark-launch convention and removes a real rollback path; the `https://`-only runtime guard specifically exists so a misconfigured or over-eager flag flip can never regress the plain-HTTP listener — it is a correctness guard, not just a togglability convenience |
| Use `golang.org/x/net/http2/h2c` for the cleartext-HTTP/2 wiring | Moot under the revised decision — cleartext HTTP/2 is rejected outright as a mechanism (see first row), not re-implemented with a different package |

## Consequences

- **No `server/server.go` change.** This decision is entirely frontend-scoped.
  `StreamingWSBridge` and its two registration lines (`server/server.go:419-427`) are **not**
  modified and **not** scheduled for deletion — they remain permanently required for the plain
  `:8543` listener, correcting the original ADR's assumption that this file becomes deletable
  after a trial period. On the TLS listener it becomes unused-in-practice (not removed) once the
  frontend fully adopts native transport there, since the same registration serves both
  listeners off one shared `srv.mux`.
- `StreamTerminal` and `connectrpc_websocket.go`'s ~3000-line envelope/pooling machinery are
  **unaffected** — this decision does not reduce that file's size or complexity, unchanged from
  the original ADR.
- The realized benefit ("frees a browser connection slot," retires hand-rolled envelope handling
  for these two RPCs) now applies **only** to traffic through the TLS remote-access listener — a
  smaller, but real and immediately available win, rather than the originally claimed
  default-deployment-wide benefit, which this plan-repair pass found does not exist for any real
  browser client on the plain listener.
- This decision is fully decoupled from `session/streamhub`'s in-flight dark-launch rollout —
  it touches a different file (`ws_stream_bridge.go`/frontend transport selection, not
  `connectrpc_websocket.go`'s `StreamHub`-owned path) and a different flag — and can proceed on
  its own schedule regardless of that rollout's trial-period status (`research/architecture.md`
  §6). Unchanged from the original ADR.
