# Stack Research: Apache Iggy, yetty, W3C WebTransport, Go Ecosystem

Agent 1 (Stack) — Phase 2 research for `project_plans/web-transport-architecture-review/`.
Scope: what these three external references *are*, precisely, and what adopting each would
require. Per-subsystem fit (terminal streaming / RPC bridge / CDP-VNC) is assessed against
these facts by the architecture-fit research doc; this doc is the factual substrate.

## 1. Apache Iggy

**What it is**: a persistent message-streaming platform written in Rust — a Kafka-class
broker, not a transport library. Its own docs describe it as "a high-performance,
persistent message streaming platform... capable of processing millions of messages per
second with ultra-low latency" ([iggy.apache.org](https://iggy.apache.org)).

**Protocols exposed**: QUIC, TCP (custom binary protocol), WebSocket, and HTTP (REST API),
all with optional TLS ([github.com/apache/iggy](https://github.com/apache/iggy),
[iggy.apache.org](https://iggy.apache.org)). This is a broker-to-client wire protocol
choice, not a general-purpose transport library — a client speaks one of these four
protocols specifically to talk to an Iggy *server*.

**Deployment model — standalone server, not a library.** Iggy runs as its own process
(`cargo run --bin iggy-server`, or Docker), single node by default with multi-node
clustering via Viewstamped Replication Revisited (VSR) consensus
([github.com/apache/iggy](https://github.com/apache/iggy),
[iggy.apache.org](https://iggy.apache.org)). There is no embeddable-library mode. Per the
requirements doc's Rabbit Hole call-out, "adopt Iggy" means **operating a second daemon**
alongside the Go server — process supervision, persistence/data-directory management,
backup, its own upgrade cadence — not a `go get` + import.

**Go client SDK**: yes, official and current —
[`github.com/apache/iggy/foreign/go`](https://pkg.go.dev/github.com/apache/iggy/foreign/go),
listed alongside Rust, C#, C++, Java, Python, Node.js/TypeScript, and PHP as an officially
maintained SDK. It ships as part of the same monorepo/release train as the server, so its
version tracks server releases directly.

**Maturity / incubation status**: entered the Apache Incubator in **February 2025**. The
graduation vote to become a full Apache Top-Level Project **passed on August 14, 2026** (12
+1s, 6 binding, no 0/-1s) — as of this research (2026-08-22) it is a newly-graduated TLP,
about one week post-graduation
([mail-archive.com vote thread](http://www.mail-archive.com/general@incubator.apache.org/msg86959.html),
[incubator.apache.org/projects/iggy.html](https://incubator.apache.org/projects/iggy.html)).
Release cadence is active: v0.7.0 shipped February 2026, v0.8.0 shipped April 22, 2026 (per
GitHub release discussions
[#2760](https://github.com/apache/iggy/discussions/2760) and
[#3067](https://github.com/apache/iggy/discussions/3067)). Pre-1.0 — v0.8.0's notes describe
several changes as "groundwork for future multi-node support... not yet used by the server,"
i.e. clustering/consensus is still landing. 4.5k GitHub stars, 2,176+ commits, active
multi-company contributor base.

**Scale fit — explicitly checked per the requirements' non-functional constraint.** Iggy's
whole design center (io_uring, thread-per-core shared-nothing architecture, millions of
messages/sec, VSR clustering for durability at scale) targets distributed high-throughput
streaming. This repo is a single-operator instance with small concurrent-connection counts
and no durability/replay requirement for terminal/RPC/CDP traffic — none of the three
in-scope subsystems have a persistence or multi-consumer fan-out requirement that a broker
solves. This is the same shape of mismatch the prior `exotic-transports.md` research found
with Chrome IWA Direct Sockets: a real, well-built piece of technology solving a problem one
scale tier up from what this product has.

## 2. yetty (github.com/zokrezyl/yetty)

**Established from the repo itself, not guessed from the name.** yetty is **not** a
networking, streaming, or transport project. Per its own README: "Yetty is a GPU-accelerated
terminal and rich-content runtime. It keeps normal terminal workflows intact, but lets
programs place plots, images, diagrams, documents, video, GUI panels, remote desktops, and
AI-generated figures directly into the same scrolling surface as text"
([github.com/zokrezyl/yetty](https://github.com/zokrezyl/yetty)).

**What it actually is**: a terminal *emulator/client application* — a competitor to
iTerm2/Alacritty/Ghostty with a rich-content rendering layer bolted on, not a
server-side or protocol technology stapler-squad could adopt into its Go backend or web
transport layer. Concretely:

- Pure C implementation, WebGPU-based GPU rendering, FFI-first with generated language
  bindings.
- Dependencies: Dawn/WebGPU, libvterm, FreeType, GLFW, libuv, libco, brotli, pdfio,
  OpenH264, QuickJS, libssh2.
- Architecture: a figure-based compositor, row-anchored rich content over a standard VT/xterm
  text layer, dirty-region rendering, OSC/DCS protocol envelopes for child-process
  communication with the rich-content programs it hosts.
- Features: VT/xterm emulation, PTY support, tabs/panes/tiled workspaces, plots/charts,
  document rendering, embedded GUI surfaces, VNC/remote-desktop *client* support, and an
  MCP server for AI-agent integration.
- License: Business Source License 1.1 (source-available, free for non-production use only).

**Verdict basis**: yetty is a local terminal *client* users would run on their own machine,
analogous to choosing a different terminal emulator — it has no server-side component and no
network transport protocol of its own to evaluate against `session/streamhub`, the RPC
bridge, or CDP/VNC proxying. Its VNC-*client* capability is the one surface-level echo of
this project's VNC-*proxying* subsystem, but yetty consumes VNC as a display client, it does
not proxy or transport VNC data over the web — structurally unrelated to
`vnc_proxy_handler.go`'s job (bridging a VNC server to a browser tab over WebSocket). Given
this, and consistent with the requirements' explicit anticipation that the honest verdict
might mirror IWA Direct Sockets/WASM clients ("not applicable"), yetty has no transport
technology applicable to any of the three in-scope subsystems.

## 3. W3C WebTransport

**Source**: [w3c/webtransport explainer.md](https://github.com/w3c/webtransport/blob/main/explainer.md).

**Capabilities over WebSocket**:
- **Multiple independent streams** per connection ("many simultaneous data flows over one
  connection"), vs. WebSocket's single ordered byte stream.
- **Unreliable datagrams** — a UDP-like send/receive primitive with no browser
  WebSocket equivalent, useful for latency-sensitive data that can tolerate loss (e.g. video/
  cursor/input events) without paying retransmission-induced delay.
- **No head-of-line blocking (HOLB)**: WebSocket "suffers from head-of-line blocking...
  meaning that all messages must be sent and received in order even if they are
  independent." Because WebTransport streams are independent (QUIC-level), one slow/lost
  stream doesn't stall unrelated streams on the same connection — this is the same HOLB fix
  HTTP/3 brought over HTTP/2's single TCP connection.
- **0-RTT / connection reuse and capability negotiation**: transport reliability
  (UDP-first with fallback) is negotiated per the explainer's connection-establishment
  section.

**Server-side requirement**: WebTransport is built on **HTTP/3 over QUIC (UDP)** as its
primary transport — "This will be an HTTP/3 connection over UDP if possible. If not possible
..., then an HTTP/2 connection over TCP might be returned instead" (per the explainer). In
practice, the deployed server-side story is: **terminate HTTP/3/QUIC**, i.e. a UDP listener
with QUIC handling, not a drop-in addition to an existing HTTP/1.1 or HTTP/2-over-TCP
`net/http` server. This is the deployment-shape mismatch the requirements doc flags as a
Rabbit Hole: this repo's server today is a plain `net/http` server on `localhost:8543` with
self-signed TLS for remote access over TCP — WebTransport support means adding a second,
UDP-based listener path alongside it, not swapping a `net/http` transport.

**Browser support as of 2026 (checked against the repo's own "zero-install-step" bar)**:
WebTransport reached **Baseline** status as of **March 2026** — Safari 26.4 shipped support,
joining Chrome and Firefox, which removed the last major cross-browser blocker. Current
matrix: **Chrome 97+, Edge 98+, Firefox 114+, Safari 26.4+, Opera 83+, Samsung Internet 18+**
([WebRTC Ventures, "WebTransport Is Now Baseline"](https://webrtc.ventures/2026/04/webtransport-is-now-baseline-what-it-means-for-real-time-media/),
[MDN WebTransport](https://developer.mozilla.org/en-US/docs/Web/API/WebTransport),
[caniuse.com/webtransport](https://caniuse.com/webtransport)). This clears the
zero-install-browser-tab bar the prior `exotic-transports.md` research used to reject Chrome
IWA Direct Sockets — unlike Direct Sockets (Chrome-only, requires an installed PWA),
WebTransport now works from a normal tab in every major evergreen browser as of this
research date. The gap this repo would still need to solve is **not** browser support but
**server-side HTTP/3/QUIC termination**, addressed in section 4 below.

## 4. Go Ecosystem: WebTransport Server-Side Support

**Primary library**: [`github.com/quic-go/webtransport-go`](https://github.com/quic-go/webtransport-go),
built on [`github.com/quic-go/quic-go`](https://github.com/quic-go/quic-go).

- **quic-go**: describes itself as "a production-ready QUIC implementation in pure Go"
  covering RFC 9000/9001/9002 plus HTTP/3 (RFC 9114), QPACK (RFC 9204), and HTTP Datagrams
  (RFC 9297). It can serve HTTP/1.1, HTTP/2, and HTTP/3 from one Go process
  ([github.com/quic-go/quic-go](https://github.com/quic-go/quic-go),
  [quic-go.net/docs](https://quic-go.net/docs/)).
- **webtransport-go**: implements the WebTransport-over-HTTP/3 draft — specifically **draft-16**
  of `draft-ietf-webtrans-http3` (not yet a finalized RFC). Its `webtransport.Server` **wraps
  an `http3.Server`**, i.e. it composes with quic-go's own HTTP/3 server type, not with
  stdlib `net/http`'s TCP-based `http.Server` — a *new* server/listener alongside the
  existing one, exposed via a normal `http.Handler`-shaped API for routing WebTransport
  sessions once the QUIC/HTTP-3 layer is up
  ([github.com/quic-go/webtransport-go](https://github.com/quic-go/webtransport-go),
  [quic-go.net/docs/webtransport/server](https://quic-go.net/docs/webtransport/server/)).
- **Maturity signal**: no explicit "stable"/"production-ready" claim in the webtransport-go
  README itself (unlike quic-go's own README, which does claim production-readiness for
  QUIC/HTTP-3), but it lists real production consumers: **go-libp2p** (powers IPFS and
  Filecoin), **Centrifugo**, **MediaMTX**, **any-sync**, and WebTransport support in
  **socket.io**/**signalr** ports. It targets "the latest two Go releases."
- **No serious alternative** surfaced in this research — quic-go/webtransport-go is the de
  facto and effectively only actively-maintained Go WebTransport server implementation;
  standard-library `net/http` has no native HTTP/3/QUIC support as of Go 1.25/1.26 (full
  kernel-level QUIC integration into Go's stdlib networking stack is not expected before
  2026 at the earliest, per community tracking), so any WebTransport adoption in this repo
  routes through quic-go regardless.

**Does it compose with this repo's existing `net/http` server model?** Partially. Routing
*within* a WebTransport session (once established) uses a familiar `http.Handler`-shaped
API. But WebTransport's `webtransport.Server` needs its own UDP socket and its own
`http3.Server`, run **alongside**, not instead of, the existing TCP `net/http.Server` on
`:8543` — the existing HTTP/1.1/1.2 handlers, ConnectRPC service registration, and
static-asset serving stay on the current TCP path; only new WebTransport-specific endpoints
would live on the new QUIC/UDP path. This is an additive deployment change (new port, new
protocol, new cert-handling surface for QUIC's TLS 1.3 requirement), not a transport swap
underneath existing routes.

**Notably**: `quic-go` and its `qpack` dependency are **already present in this repo's
`go.mod`** — but only as *indirect, build-tooling* dependencies (`go mod why
github.com/quic-go/quic-go` resolves through `github.com/bufbuild/buf/cmd/buf` →
`.../command/curl`, i.e. pulled in by the `buf` CLI tool's HTTP/3 `curl` command, not by any
runtime code in this repo). There is **no existing runtime QUIC/WebTransport dependency** to
build on; adopting WebTransport would add `quic-go`/`webtransport-go` as genuine runtime
dependencies for the first time.

## 5. Version/Library Summary and Recommended Approach (as of 2026-08-22)

| Reference | What it is | Adopt as | Version to pin | 2026 community-recommended approach |
|---|---|---|---|---|
| **Apache Iggy** | Standalone Rust message-streaming *server* | A second daemon process + `github.com/apache/iggy/foreign/go` client | Server + SDK: v0.8.0 (Apr 2026), fresh TLP as of Aug 14 2026 | Only if a subsystem needs durable, replayable, multi-consumer streaming at a scale this product doesn't currently have — see requirements' scale-mismatch flag. Not a transport-layer swap-in. |
| **yetty** | Local GPU terminal-emulator client app | N/A — no server/transport component | n/a | Not applicable to any in-scope subsystem; no further evaluation warranted. |
| **W3C WebTransport (browser API)** | Web platform API: multiplexed streams + unreliable datagrams over HTTP/3/QUIC | Browser-side `WebTransport` object, paired with a QUIC-terminating Go server | Spec: Baseline as of March 2026; browsers: Chrome 97+/Edge 98+/Firefox 114+/Safari 26.4+ | Now viable from the browser-compatibility side (the bar that sank IWA Direct Sockets). Adoption cost is entirely server-side infra (new QUIC/UDP listener, TLS 1.3 for QUIC) — that's the open question for architecture-fit research, not the browser API's own maturity. |
| **Go server-side WebTransport** | `quic-go/webtransport-go` on top of `quic-go/quic-go` | New runtime dependency, additive UDP listener beside existing `net/http` TCP server | `webtransport-go` implements WebTransport draft-16 (pre-RFC); `quic-go` is itself production-ready for QUIC/HTTP-3 | The only real Go option; used in production by go-libp2p, Centrifugo, MediaMTX. Composes with `net/http`-shaped handler code but not with the existing TCP listener — it's a parallel server, not a drop-in transport for existing ConnectRPC/WebSocket endpoints. |
| **ConnectRPC native streaming** (surfaced from requirements' Open Question 4, not one of the three named references, but directly relevant to the RPC-bridge subsystem) | Connect protocol already supports all RPC streaming types over HTTP/1.1 (unary)/HTTP/2 (streaming) via plain `net/http` | Already a dependency (`connectrpc.com/connect v1.19.0`) | Already pinned in `go.mod` | Per ConnectRPC's own FAQ, the reason browsers can't fully use native gRPC/Connect bidi streaming is that "browsers don't support trailers" the way gRPC needs, and full-duplex streaming request bodies from `fetch()` has its own browser-support history — this is a strong candidate explanation for why `connectrpc_websocket.go` exists as a hand-rolled envelope-over-WebSocket bridge in the first place, worth confirming directly against that file's own design-rationale comments in the architecture-fit research pass. |

### Key takeaways for Phase 3 planning

1. **Iggy and yetty are both out** on their own facts, independent of subsystem-by-subsystem
   analysis — Iggy on operational-cost/scale-mismatch grounds (a second daemon for a
   single-operator tool with no durability/fan-out need), yetty because it has no
   server-side or transport component to adopt at all.
2. **WebTransport is the one reference with a real, current opportunity** — as of August
   2026 it clears the same browser-compatibility bar that sank the prior IWA Direct Sockets
   research, and has a maintained, production-used Go server library
   (`quic-go/webtransport-go`). The cost is additive infrastructure (new QUIC/UDP listener,
   TLS 1.3 termination for QUIC) alongside the existing TCP `net/http` server — this repo
   has zero runtime QUIC dependency today (the `quic-go` entries in `go.mod` are indirect,
   pulled in only by the `buf` CLI tool, not by any server code).
3. **A closer, cheaper reference than any of the three named ones surfaced during this
   research**: ConnectRPC's own native HTTP/2 streaming, already a first-class dependency of
   this repo, is worth weighing against WebTransport specifically for the RPC-bridge
   subsystem before reaching for a QUIC-based solution — this is Open Question 4 in the
   requirements doc and belongs in the architecture-fit/per-subsystem research pass.

## Sources

- [Apache Iggy — GitHub](https://github.com/apache/iggy)
- [Apache Iggy — iggy.apache.org](https://iggy.apache.org)
- [Apache Iggy Go SDK — pkg.go.dev](https://pkg.go.dev/github.com/apache/iggy/foreign/go)
- [Apache Incubator — Iggy graduation vote thread](http://www.mail-archive.com/general@incubator.apache.org/msg86959.html)
- [Apache Incubator — Iggy project status](https://incubator.apache.org/projects/iggy.html)
- [Apache Iggy v0.7.0 release discussion](https://github.com/apache/iggy/discussions/2760)
- [Apache Iggy v0.8.0 release discussion](https://github.com/apache/iggy/discussions/3067)
- [yetty — GitHub](https://github.com/zokrezyl/yetty)
- [W3C WebTransport explainer](https://github.com/w3c/webtransport/blob/main/explainer.md)
- [WebTransport — MDN](https://developer.mozilla.org/en-US/docs/Web/API/WebTransport)
- [WebTransport — caniuse.com](https://caniuse.com/webtransport)
- [WebTransport Is Now Baseline — WebRTC Ventures](https://webrtc.ventures/2026/04/webtransport-is-now-baseline-what-it-means-for-real-time-media/)
- [quic-go — GitHub](https://github.com/quic-go/quic-go)
- [quic-go docs](https://quic-go.net/docs/)
- [webtransport-go — GitHub](https://github.com/quic-go/webtransport-go)
- [webtransport-go server docs](https://quic-go.net/docs/webtransport/server/)
- [ConnectRPC FAQ](https://connectrpc.com/docs/faq/)
- Local: `go.mod` (`connectrpc.com/connect v1.19.0`, `github.com/gorilla/websocket v1.5.3`,
  `github.com/quic-go/quic-go v0.54.0 // indirect`), `go mod why github.com/quic-go/quic-go`
  (resolves through `bufbuild/buf/cmd/buf`, not runtime code), `session/streamhub/transport.go`
  (`Transport` interface: `Send([]byte) error`, `Close() error`),
  `server/services/connectrpc_websocket.go:1-80` (hand-rolled 5-byte ConnectRPC envelope over
  `gorilla/websocket`).
