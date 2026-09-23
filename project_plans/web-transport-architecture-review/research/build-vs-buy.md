# Research: Build vs. Buy — Web Transport Layer

Agent 6, Phase 2. Answers: should `session/streamhub/` (terminal), the ConnectRPC bridge
(`server/services/connectrpc_websocket.go`, `ws_stream_bridge.go`), and CDP/VNC proxying
continue to be hand-built, or adopt Apache Iggy, yetty, W3C WebTransport, ConnectRPC's own
native streaming, or a fork of an existing terminal-over-web project?

## 0. Why the custom bridge exists (confirmed, not assumed)

Two independent, confirmed reasons — not one:

1. **The main server has no HTTP/2.** `server/server.go:109-119` constructs the primary
   `:8543` `http.Server` and, per `server/server.go:1200-1204`, it serves over
   `listenLoopbackAware` (plain TCP) unless `s.tlsConfig != nil`. Go's `net/http` only
   negotiates HTTP/2 via TLS ALPN — there is no `golang.org/x/net/http2/h2c` wiring anywhere
   in `server/server.go` (grepped, absent) for cleartext HTTP/2. So the default dev/local
   deployment (`localhost:8543`, no TLS) is HTTP/1.1-only, and ConnectRPC's native
   streaming transport (which needs HTTP/2 for bidi, or degrades hard on HTTP/1.1) isn't
   available on it. The remote-access server (`server/server.go:1389`, `TLSConfig` set)
   does get HTTP/2 automatically since Go enables it by default under TLS — but that path is
   optional/secondary, not the default.
2. **Browser HTTP/1.1 6-connections-per-origin limit.** `ws_stream_bridge.go:16-19`'s doc
   comment states this explicitly: `StreamingWSBridge` exists so that long-lived
   server-streaming RPCs (`WatchSessions`, `WatchReviewQueue`) don't each consume one of a
   browser's 6 concurrent HTTP/1.1 connections per origin, which would starve other
   requests on the same origin.
3. **Browser-side streaming request bodies are a separate, harder constraint.** Per
   ConnectRPC's own docs/discussions (connectrpc/connect-go#254), gRPC-Web — and by
   extension the plain Connect protocol in a browser — can't do client- or
   bidirectional-streaming at all from `fetch()`, because browsers buffer request bodies;
   full duplex needs HTTP/2 *and* streaming-body `fetch()` support, which has inconsistent
   browser coverage. Terminal input (client→server) is exactly this shape. This is likely
   why the bridge reaches for raw WebSocket rather than "just enable h2c and use Connect
   natively" — WebSocket bidi works today in every browser regardless of HTTP version.

Conclusion: the custom envelope-over-WebSocket bridge is not pure reinvention — it works
around two real, current constraints (no HTTP/2 on the default deployment, and browser
fetch-body streaming limits) that ConnectRPC's native transport does not solve for this
server's actual deployment shape. It *is*, however, non-trivial reinvention of framing
(5-byte envelope + pooling, hand-rolled in `connectrpc_websocket.go` and `ws_stream_bridge.go`)
that a maintained library could absorb once/if the HTTP/2 gap were closed — see §4.

## 1. Apache Iggy

**What it is**: `apache/iggy` — a persistent message-streaming *server*, written in Rust,
Apache 2.0 licensed, currently an Apache Incubator project (~4.5k GitHub stars, 375 forks,
active 2026 release cadence — v0.8.0 in April 2026). It speaks QUIC, TCP (custom binary
protocol), WebSocket, and HTTP, and is built for millions-of-messages/sec throughput via a
thread-per-core, io_uring-based architecture. Official Go client exists
(`iggy-rs/iggy-go-client`, also mirrored at `apache/iggy/foreign/go`), last updated mid-2025 —
present and maintained, but immature relative to the Rust/Python SDKs; no evidence of
production Go-client case studies.

- **Is it a library or a server?** A standalone server process, confirmed. There is no
  "embed Iggy in your Go binary" mode — adopting it means running and operating a second
  long-lived service (its own storage engine, its own port, its own health/backup/upgrade
  lifecycle) alongside `stapler-squad`.
- **Pros**: durable, replayable message log; strong throughput/latency numbers; genuinely
  well-maintained (ASF incubation, active releases); protocol flexibility (its own transports
  include WebSocket, ironically the same primitive this review is evaluating replacing).
- **Cons**: solves distributed-systems-scale problems (persistence, replay, millions of
  msg/sec, multi-consumer fan-out across processes) that none of the three subsystems have.
  Terminal streaming, RPC watch-streams, and CDP/VNC proxying are all single-producer,
  single-or-few-consumer, ephemeral (no need to replay a terminal frame from an hour ago),
  and bounded by one operator's session count — not throughput. Running a second server
  process directly conflicts with this repo's single-operator, no-staging-environment,
  "real rollback path" constraint (Requirements §Constraints): a new stateful service is a
  new failure mode, upgrade path, and backup/restore story with no operational precedent in
  this codebase. Go client is comparatively unproven.
- **Verdict: Not recommended**, for all three subsystems. This is the same shape of mismatch
  the prior `exotic-transports.md` research found for Chrome IWA Direct Sockets — solving a
  scale problem this product doesn't have, at a packaging/operational cost the product's
  constraints explicitly rule out. No triggering condition within this product's plausible
  roadmap (single operator, personal tool) changes this: it would take multi-tenant,
  durable-replay, or cross-process fan-out requirements materializing, none of which are on
  this project's horizon.

## 2. yetty (`github.com/zokrezyl/yetty`)

**What it actually is** (fetched and read, not assumed): a GPU-accelerated **terminal
*client* application** (a replacement for iTerm2/Alacritty/kitty, not a transport or
streaming library). It renders via WebGPU and lets CLI programs push rich inline content —
plots, images, video, remote-desktop panels, AI-generated figures — into the same scrollback
surface as text, using a custom OSC/DCS-based "rich terminal protocol" that child processes
emit and the terminal decodes into row-anchored figures. Written in C, FFI-first. License:
Business Source License 1.1 (free for non-production use; production use requires a
commercial license). Early alpha: 78 stars, 574 commits, unstable API.

- **Relevance check**: yetty is a terminal *emulator/renderer* users would run on their own
  machine, analogous to xterm.js in this repo's web UI — not a wire transport, not a
  streaming server, not anything `session/streamhub/`, the RPC bridge, or CDP/VNC proxying
  could "adopt" as a dependency. It has no server-side component this repo could embed, and
  its protocol (OSC/DCS envelopes for rich content) is a presentation-layer concept, not a
  transport for getting bytes from tmux to a browser.
- **Pros**: none applicable — it doesn't solve a transport problem this project has.
- **Cons**: wrong category entirely (client app vs. transport/library); BSL 1.1 licensing
  would be an added complication for anything production-facing even if it were relevant;
  alpha-stage, unstable, C/WebGPU stack orthogonal to this repo's Go/React stack; would
  require rewriting the web UI's terminal rendering (`xterm.js`-based) to a WebGPU-based
  desktop-app model, which is a different product shape (desktop terminal emulator vs.
  browser-hosted session manager).
- **Verdict: Not recommended / not applicable**, for all three subsystems — same "ruled out
  on evidence, not assumption" outcome the requirements anticipated as possible. There is no
  triggering condition that makes yetty relevant to this review's scope; it would take this
  product deciding to ship a native desktop terminal client instead of a browser UI, which is
  a different project entirely.

## 3. W3C WebTransport (as "which Go server library")

WebTransport is a browser API + QUIC/HTTP-3 wire protocol, not a single library choice on the
server. The live Go options are `quic-go/webtransport-go` (built on `quic-go/quic-go`,
implements WebTransport draft-16; quic-go itself is explicitly "production-ready," with
active 2026 releases including FIPS 140-3 support in v0.60) and a lighter fork,
`adriancable/webtransport-go`. Both require running a QUIC/HTTP-3 listener.

- **Browser support (checked against this review's explicit bar)**: Chrome 97+ and Firefox
  114+ support WebTransport fully. **Safari has no support** (not even behind a flag on
  desktop; only an experimental flag on iOS 18). This fails the requirement's own bar
  ("must work from a normal browser tab with zero install step") for any user on Safari/iOS
  — the same class of rejection `exotic-transports.md` applied to Direct Sockets, just for a
  narrower browser slice instead of a Chrome-only requirement.
- **Deployment-shape mismatch (the "Rabbit Hole" flagged in requirements)**: WebTransport
  requires HTTP/3 (QUIC/UDP) termination. This repo's default deployment is plain HTTP/1.1 on
  `localhost:8543` (§0 above) — no TLS, let alone QUIC. Even the TLS remote-access path
  (`server/server.go:1389`) is HTTP/2-over-TCP, not HTTP/3-over-UDP. Standing up WebTransport
  would mean adding a second listener/protocol stack (QUIC on a UDP port) purely to serve one
  browser feature, on top of the existing TCP/TLS setup this repo already documents as
  nontrivial (`.claude/docs/codesigning.md`-adjacent remote-access cert handling). That's a
  materially larger infra footprint than anything the other options require.
- **Pros** (in the abstract): real perf/latency wins over WebSocket for lossy/high-latency
  links (independent QUIC streams avoid head-of-line blocking); native datagram support;
  purpose-built as WebSocket's successor.
- **Cons for this project**: Safari exclusion is disqualifying on its own per this review's
  explicit browser-compatibility bar; QUIC/UDP infra is a new deployment shape with no
  current precedent in this repo; `webtransport-go` is pre-1.0 (still tracking IETF drafts,
  not a finalized spec) — production-readiness risk stacked on top of the infra cost;
  benefits (better performance under packet loss) don't address a documented problem here —
  this product runs on LAN/Tailscale/loopback, not lossy mobile networks.
- **Verdict: Not recommended**, for all three subsystems, at this time. Fails the review's
  own browser-compatibility bar (Safari) and its own deployment-shape check (HTTP/3
  requirement vs. current plain-HTTP/TLS setup) — both checks the requirements explicitly
  asked to run before treating it as viable, and both come back negative before any deeper
  comparison is warranted. Triggering condition that would flip this: universal Safari
  WebTransport support *and* a decision to make QUIC/HTTP-3 the default transport for the
  whole server (a much bigger, separate infra decision) — neither is close.

## 4. ConnectRPC's own native HTTP/2 streaming

This is the option the requirements flagged as possibly the *real* gap (§Open Questions):
"is the real gap 'wrong transport' or 'reinventing what ConnectRPC already provides
natively'?" `connectrpc.com/connect v1.19.0` is already a direct dependency
(confirmed: `go.mod`), alongside `connectrpc.com/otelconnect` and the older
`bufbuild/connect-go` (an indirect legacy alias). ConnectRPC's Connect protocol natively
supports server-streaming over HTTP/1.1 or HTTP/2, and full bidirectional streaming when
running over HTTP/2 — no custom envelope-over-WebSocket needed, if HTTP/2 is available.

- **Scoped verdict — RPC bridge (`WatchSessions`/`WatchReviewQueue`, server-streaming only)**:
  **Viable, conditionally recommended**, but gated on closing the HTTP/2 gap identified in
  §0: either (a) terminate TLS on the default `:8543` server (enabling Go's automatic HTTP/2)
  or (b) wire `golang.org/x/net/http2/h2c` for cleartext HTTP/2 on plain TCP. Either change
  is itself a nontrivial infra decision (TLS-by-default changes the "zero-install,
  self-signed-cert-optional" local dev story this repo currently has; h2c is lower-risk but
  still new). Once either exists, `StreamingWSBridge` (143 lines) and its hand-rolled
  WebSocket envelope logic become deletable in favor of Connect's native transport — that's
  the actual "reinventing the wheel" the requirements suspected, and it's real: 143 lines of
  bespoke bridging code for something the dependency already does, once the HTTP/2
  precondition is met.
- **Scoped verdict — terminal streaming and RPC bidirectional/client-streaming paths
  (terminal input is client→server)**: **Not recommended to migrate**, even after closing the
  HTTP/2 gap, because of the browser fetch-body-streaming constraint from §0 point 3 —
  Connect's native transport over `fetch()` still can't reliably do client-streaming/bidi from
  a browser tab across all target browsers the way a raw WebSocket message loop can. Terminal
  input is a continuous client→server bidi stream; WebSocket remains the more broadly
  compatible choice for that direction regardless of server-side HTTP/2 support.
- **Cons of adopting even the conditional part**: requires the HTTP/2 infra change first
  (not free — see above); would need the frontend's `createWebsocketBasedTransport`
  (`web-app/src/lib/transport/websocket-transport.ts`) and `watch-ws-transport.ts` to be
  swapped for Connect's stock browser transport for the affected RPCs only, which is a
  frontend+backend coordinated change, not a backend-only one.
- **Pros**: deletes real hand-rolled code (`ws_stream_bridge.go` in full, ~143 lines) with no
  new external dependency (already in `go.mod`); reduces protocol surface area; is literally
  "use what you already depend on" rather than adding anything.
- **Verdict: Recommended for the RPC bridge's server-streaming-only calls
  (`WatchSessions`/`WatchReviewQueue`) once/if HTTP/2 is enabled on the default server; Not
  recommended for terminal streaming (`session/streamhub/`) or any client/bidi-streaming RPC
  path**, due to the separate browser fetch-body-streaming limitation that HTTP/2 alone
  doesn't resolve. This is the strongest "adopt" candidate found in this review, but it's
  gated on an HTTP/2 decision this document surfaces rather than makes — see plan.md for how
  to scope that as its own small, reversible step.

## 5. Fork or adapt an existing terminal-over-web project

Surveyed: `sorenisanerd/gotty` (Go, WebSocket relay between PTY and xterm.js — the spiritual
ancestor of this pattern), `ttyd` (C port of gotty, same WS architecture), Coder's
`cdr.dev/wsep` (command-execution-over-WebSocket protocol, "SSH over WebSockets without
encryption"), `tty2web`/`WeidiDeng/ttyd-go` (further gotty/ttyd derivatives).

- **Finding**: every one of these uses the same fundamental transport primitive this repo
  already uses — a raw WebSocket carrying PTY bytes in one direction and input bytes in the
  other. None of them use WebTransport, ConnectRPC, or a message broker. This is not a
  coincidence: it's the industry-converged pattern for this exact problem (browser xterm.js ↔
  server PTY), for the same reasons cited in §0 (WebSocket is the one bidi-streaming
  primitive with universal, zero-install browser support). None of these projects offer a
  transport *abstraction* more sophisticated than what `session/streamhub/`'s `Transport`
  interface (`session/streamhub/transport.go`) already provides — if anything,
  `session/streamhub/` is more general (it already supports three implementations: browser
  WebSocket, ssq-mux Unix socket, in-memory test transport) than any of these single-purpose
  tools.
- **Pros of forking one**: none identified beyond "using someone else's WebSocket-relay
  loop" — but `session/streamhub/`'s hub/subscriber/ownership model already solves the
  specific problem (concurrent-connection resize/capture races) that these simpler
  single-consumer-assumed relays don't handle at all; adopting one would be a regression from
  the just-shipped multi-connection design, not an improvement.
- **Cons**: these projects are single-viewer-oriented (gotty/ttyd's classic model has no
  concept of the multi-subscriber ownership/ordering `session/streamhub/` was specifically
  built to fix); adapting one in would mean re-deriving the resize/capture-race fixes that
  motivated the current rebuild, or bolting `session/streamhub/`'s hub logic on top of a
  foreign relay loop for no clear benefit over the code already in place.
- **Verdict: Not recommended.** `session/streamhub/` is already more capable than what's
  available to fork, for this repo's specific concurrent-connection requirements. This also
  independently corroborates that WebSocket, as the terminal-streaming carrier, is the
  correct choice rather than an accident of history — the entire ecosystem doing the same
  browser-facing terminal problem converges on it.

## Summary table

| Option | Terminal streaming (`streamhub`) | RPC bridge (server-streaming, `WatchSessions`/`WatchReviewQueue`) | RPC bridge (bidi/client-streaming) | CDP/VNC proxying |
|---|---|---|---|---|
| Apache Iggy | Not recommended — scale mismatch, new server to operate | Not recommended — same | Not recommended — same | Not recommended — same |
| yetty | Not applicable — wrong category (terminal client, not transport) | Not applicable | Not applicable | Not applicable |
| W3C WebTransport | Not recommended — Safari unsupported, needs HTTP/3 infra | Not recommended — same | Not recommended — same | Not recommended — same |
| ConnectRPC native streaming | Not recommended — browser fetch-body bidi limits | **Recommended, gated on HTTP/2 availability** | Not recommended — browser fetch-body bidi limits | N/A — CDP/VNC are opaque byte proxies, not ConnectRPC calls |
| Fork ttyd/gotty/wsep | Not recommended — regresses below `streamhub`'s multi-connection design | N/A | N/A | N/A |
| **Stay hand-built (WebSocket)** | **Recommended — status quo, mid-rollout, don't destabilize** | Viable fallback if HTTP/2 gate isn't pursued | **Recommended — status quo** | **Recommended — status quo (noVNC/CDP are natively WS-oriented; handlers are small, 173/215 lines)** |

## Interaction with `session/streamhub/`'s in-flight rollout

No recommendation in this document touches `session/streamhub/`'s transport choice (browser
WebSocket remains the carrier for terminal streaming in every scenario evaluated) or its
hub/ownership/resize design. The one "Recommended" adoption (§4, ConnectRPC native streaming
for server-streaming-only RPC calls) is scoped entirely to `WatchSessions`/`WatchReviewQueue`
in `server/services/ws_stream_bridge.go` and `server/server.go:419-427` — a different code
path from `connectrpc_websocket.go`'s terminal-streaming handler and unrelated to
`streamhub`'s `STAPLER_SQUAD_USE_STREAM_HUB` flag or its rollback-rehearsal gate
(`project_plans/terminal-multi-connection-streaming/implementation/plan.md`). It can proceed
or be deferred independently of that rollout's trial period.

## Answer to the unification question (§Open Questions, Phase 3 input)

Given the above, unifying all three subsystems onto one transport abstraction does not pay
for itself: terminal streaming and CDP/VNC proxying both have hard, confirmed reasons to stay
on WebSocket (§0, §5, §3's Safari finding), while the RPC bridge's server-streaming calls are
the one place with a real, narrow "delete hand-rolled code" opportunity — and that
opportunity is ConnectRPC's *existing* native transport, not a new shared abstraction layered
over WebSocket. Extending `session/streamhub/`'s `Transport` interface to cover the RPC
bridge or CDP/VNC would add abstraction for two subsystems whose payloads (protobuf
request/response frames; opaque proxied bytes) don't share `Transport.Send([]byte)`'s
terminal-frame semantics meaningfully. Recommendation for Phase 3: scope any plan to the
single HTTP/2-gated ConnectRPC-native migration for `WatchSessions`/`WatchReviewQueue`; leave
the other subsystems as-is.
