# Research: Architecture — web-transport-architecture-review

Scope (Agent 3, Phase 2): map the current transport boundary of the three subsystems named in
`requirements.md`, evaluate whether Apache Iggy / yetty / WebTransport plug in anywhere, and
assess unify-vs-leave-separate. Per requirements.md §5 (EventStorming guidance) and the actual
shape of this problem (infrastructure/transport, not multi-actor business domain), no
EventStorming table is produced — none of the three subsystems have policy/reactive-business-rule
structure beyond what `terminal-multi-connection-streaming/research/architecture.md`'s table
(hub.go's own Event/Command/Policy model) already covers, and this review does not re-open that.

This document builds directly on `project_plans/terminal-multi-connection-streaming/research/architecture.md`
(hereafter **"the hub research doc"**) and its four ADRs
(`project_plans/terminal-multi-connection-streaming/decisions/ADR-001..004`). That work settled
the *ownership* design (single-owner hub per session, actor-model resize/quiescence). This review
does not re-derive or question that — it asks only whether a *different wire transport* should sit
underneath it, and whether the RPC bridge and CDP/VNC proxies (untouched by the hub project)
should change too.

## 1. Current architecture, read directly

### 1a. `session/streamhub/`'s `Transport` interface — the one deliberate abstraction that exists

[`session/streamhub/transport.go:6-14`](session/streamhub/transport.go#L6-L14) is the entire
interface, by design minimal:

```go
type Transport interface {
    Send(data []byte) error
    Close() error
}
```

Two methods, no framing, no negotiation, no read half. This is deliberately narrower than a
`net.Conn` — it's a *sink*, not a bidirectional channel. Input (resize requests, keystrokes) flows
into the hub through separate, capability-gated methods
(`RequestResize`, [`session/streamhub/types.go:21-29`](session/streamhub/types.go#L21-L29)'s
`SubscriberCapability{CanResize, CanWrite}`), not through `Transport` itself. This matters for
§2 below: `Transport` is not "the network abstraction," it's "how the hub hands a subscriber a
byte frame," and swapping what carries those bytes on the wire (WebSocket → WebTransport, say)
is a `Transport` *implementation* change, not an interface change — confirming the hub research
doc's own framing that ownership and wire transport are orthogonal.

Three implementations exist today, matching requirements.md's Baseline claim exactly:

| Implementation | File | Wire carrier |
|---|---|---|
| `WebSocketTransport` | [`server/services/websocket_transport.go:17-105`](server/services/websocket_transport.go#L17-L105) | `gorilla/websocket`, via `connectWebSocketStream.WriteMessage` (binary frames) |
| `MuxTransport` (ssq-mux) | `session/external_streamer_transport.go`, `session/external_tmux_streamer_transport.go` | Unix domain socket, wrapping the pre-existing `ExternalStreamer` (ADR-004) |
| In-memory test transport | `session/streamhub/memory_transport.go` | none (test double) |

`WebSocketTransport.Send` ([websocket_transport.go:81-86](server/services/websocket_transport.go#L81-L86))
does the minimum: CAS-check `suppressNextSend`, then one `WriteMessage(BinaryMessage, data)` call —
no batching inside the transport itself. Batching/coalescing happens *above* the transport, in the
hub's own `batchWindow` ([`session/streamhub/hub.go:105-109`](session/streamhub/hub.go#L105-L109),
an "opportunistic-with-ceiling" accumulator feeding `Broadcast`). This is a second confirmation
that transport swaps are cheap: the hub already treats "how bytes get batched" and "how bytes get
sent" as separate layers, and only the latter would change under a transport swap.

Framing on the browser leg is intentionally asymmetric: `WebSocketTransport` sends **raw
`[]byte(content)`** (streamhub has no notion of the browser's proto envelope), while the actual
ANSI-sanitized, cursor-synced, proto-enveloped initial snapshot is sent directly by
`connectrpc_websocket.go`'s `streamViaHub` using `SuppressNextSend` to dedupe against the hub's own
attach-time `CatchUpSnapshot` send (doc comment,
[websocket_transport.go:64-76](server/services/websocket_transport.go#L64-L76)). In other words:
**the hub's `Transport` boundary is below the application framing layer, not above it** — protobuf
envelopes, ANSI sanitization, and cursor sync all happen in `connectrpc_websocket.go` before bytes
ever reach `Transport.Send`. A wire-transport swap (WebSocket → WebTransport) would only touch the
`Transport` implementation and `connectWebSocketStream`'s write path, not this framing logic.

### 1b. RPC bridge (`connectrpc_websocket.go`, 3192 lines) — direct `*websocket.Conn` consumer, no `Transport`

The file's own hot-path helpers describe the wire format precisely:
[`marshalProtoEnvelope`](server/services/connectrpc_websocket.go#L52-L71) writes a pooled buffer
pre-padded with a **5-byte ConnectRPC envelope header** (1 flags byte + 4-byte big-endian length),
`MarshalAppend`s the protobuf payload into it, then calls `stream.WriteMessage(BinaryMessage, buf)`
directly — this *is* ConnectRPC's own streaming wire format (the same envelope ConnectRPC's HTTP/2
bidi-streaming transport uses), just re-implemented by hand over a raw WebSocket frame instead of
using `connectrpc.com/connect`'s native HTTP/2 streaming, which the module already depends on
(`go.mod`: `connectrpc.com/connect v1.19.0`). Two `sync.Pool`s
([`envelopeBufPool`](server/services/connectrpc_websocket.go#L44),
[`coalesceBufPool`](server/services/connectrpc_websocket.go#L46-L50)) exist purely to avoid
per-frame allocation from this hand-rolled marshal/write path.

This file does not implement or use `streamhub.Transport` — grep confirms zero references
(only the reverse: `streamhub.Transport` is implemented *by* `websocket_transport.go`, which this
file constructs and passes into `hub.AttachSubscriber`). The file's job as a whole is broader than
`Transport`'s scope anyway: it multiplexes multiple logical RPC streams (session-stream,
resize-request, control messages) over **one** WebSocket connection via the envelope header's
message-type discrimination — `Transport` in the hub sense is a single output sink for one
subscriber's terminal frames, not a multiplexed RPC channel. Unifying these under one interface
would require widening `Transport` well past the hub's minimal "sink" contract (§1a), which is the
key architectural tension explored in §3.

### 1c. CDP/VNC proxies — raw byte pipes, no framing, no `Transport`, direct `*websocket.Conn`

Both [`cdp_stream_handler.go`](server/services/cdp_stream_handler.go) (215 lines) and
[`vnc_proxy_handler.go`](server/services/vnc_proxy_handler.go) (173 lines) follow the identical
shape: `Upgrade` → spawn two goroutines sharing a `context.WithCancel` → relay bytes until either
side closes → `wg.Wait()`. Neither touches `streamhub` or ConnectRPC's envelope at all.

- **VNC** ([vnc_proxy_handler.go:124-167](server/services/vnc_proxy_handler.go#L124-L167)) is a
  textbook bidirectional TCP↔WebSocket tunnel: WS→TCP goroutine writes `wsConn.ReadMessage()`
  output straight to a `net.TCPConn`; TCP→WS goroutine does the reverse with a 32KB read buffer.
  Zero interpretation of the bytes — it's pure passthrough to `x11vnc` on `127.0.0.1`.
- **CDP** ([cdp_stream_handler.go:117-158](server/services/cdp_stream_handler.go#L117-L158)) is
  one-directional-heavy: a 15fps (66ms) ticker polls `cdpMgr.LatestFrame()` and pushes whole JPEG
  frames as binary messages (with a `bytes.Equal` dedupe against the last frame), while a second
  goroutine forwards client-sent JSON input events to Chrome via `DispatchInput`. This is polling +
  push, not a true stream — the "transport" here is really "WebSocket as a one-way frame pump plus
  a side-channel for input," architecturally closer to a video-call data channel than to a terminal
  stream or an RPC.

**Neither proxy needs the hub's ownership/negotiation machinery** — CDP/VNC each own exactly one
upstream (a `net.Conn` or a per-session frame buffer) with no multi-subscriber fan-out requirement
in the code today (no registry, no `AddConsumer`-equivalent). This is a structural difference from
terminal streaming, not just an implementation gap: `session/streamhub` exists because *multiple*
browser tabs/ssq-mux clients can attach to the *same* tmux session concurrently; nothing in the CDP
or VNC handlers suggests multiple viewers of the same CDP/VNC session are a supported or requested
scenario today.

## 2. What Apache Iggy, yetty, and WebTransport actually are

### 2a. Apache Iggy — a standalone distributed message-streaming broker (Rust), not a library

Apache Iggy (Incubating) is a persistent message-streaming platform written in Rust, positioned
as a high-throughput Kafka-alternative — a server product you run, not a Go package you import.
Client SDKs exist for Rust, Go ([`github.com/apache/iggy/foreign/go`](https://pkg.go.dev/github.com/apache/iggy/foreign/go),
formerly a separate `iggy-rs/iggy-go-client` repo now folded into the monorepo), Python, C#, Java,
Node.js, PHP. The Go client currently supports TCP protocol with a blocking implementation — no
evidence of QUIC/HTTP transport support in the Go SDK specifically as of this research.

**Fit assessment against this codebase's three subsystems: none.** Iggy solves durable,
partitioned, high-throughput topic-based pub/sub between distributed producers/consumers — the
requirements.md NFR is explicit that this product's scale is "a handful of concurrent connections
per session, single-operator instance," not a throughput or durability problem. None of the three
subsystems need message durability (terminal output is ephemeral/scrollback-backed already via
`session/scrollback`; RPC calls are request/response; CDP/VNC frames are inherently "latest frame
wins," explicitly dropped via the `bytes.Equal` dedupe in §1c) or multi-producer fan-in (every
subsystem here has exactly one producer — the tmux session, the RPC handler, or Chrome/x11vnc — and
N *consumers*, which is the opposite shape from what a message broker like Iggy is built to
mediate). Adopting Iggy would mean operating a second standalone server process
alongside `stapler-squad`'s single Go binary, for a workload none of these three subsystems has.
This is the same class of verdict the hub research doc's sibling `exotic-transports.md` reached for
Chrome IWA Direct Sockets: a real, well-built piece of technology solving a problem this product
does not have at this scale. **Verdict: not applicable.** Triggering condition that would change
this: stapler-squad growing a genuine multi-producer, durable-log requirement — e.g., an audit/event
log fanned out to multiple independent consumers across process restarts — which none of the three
in-scope subsystems currently need.

### 2b. yetty — a client-side GPU terminal *emulator*, not a transport at all

`github.com/zokrezyl/yetty` ("Terminal unchained") is a WebGPU-accelerated terminal
emulator/rich-content runtime — the same category as `ghostty-web` or `wterm`, already surveyed and
ruled out for unrelated reasons in the hub research doc's sibling
`terminal-multi-connection-streaming/research/exotic-transports.md` §2 ("WASM-compiled terminal
emulator, replacing xterm.js's parser/renderer"). It renders plots, images, video, documents, and
interactive widgets inline with terminal text, built from ~70 small C modules. **It has no network
transport component of its own** — it's a rendering/parsing layer that would sit client-side in
`web-app/`, in the same architectural slot xterm.js occupies today, wholly unrelated to how bytes
get from the Go server to the browser. Two additional disqualifiers found during this research,
neither addressed by the prior WASM-terminal survey (which looked at ghostty-web/wterm, not yetty
by name):

- **License**: yetty is Business Source License 1.1 — non-production use is free, but *production*
  use requires a commercial license. stapler-squad is a production personal tool; adopting yetty as
  a rendering layer would mean either paying for a commercial license or running afoul of the BSL
  terms, a cost/compliance question that doesn't arise for xterm.js (MIT) at all.
- **Maturity**: described by its own maintainers as "early alpha," actively rewriting core
  concepts.

**Verdict: not applicable, and not even on-topic** — the research question for this project is
transport (how bytes move), and yetty answers a completely different question (how a terminal
renders once bytes arrive). It does not compete with WebSocket/WebTransport/Iggy in any of the
three subsystems' actual gap. This mirrors requirements.md's own Rabbit Hole warning ("do not
assume relevance from the name/URL alone") — the honest finding here is a category mismatch, not a
feature gap.

### 2c. W3C WebTransport — a real transport-layer candidate, with a real infra cost

WebTransport is a browser API for bidirectional, low-latency client-server messaging over HTTP/3
(QUIC), designed partly as unreliable-datagram-capable successor to WebSocket, supporting multiple
independent streams per connection. Go server-side support exists via
[`quic-go/quic-go`](https://github.com/quic-go/quic-go) (described by its maintainers as
"production-ready," actively released — a July 2026 release found during this research) plus
[`quic-go/webtransport-go`](https://github.com/quic-go/webtransport-go) (implements IETF
`draft-ietf-webtrans-http3` draft-16; a June 2026 release found). Both Chrome and Firefox support
WebTransport over HTTP/3 as of the versions checked; the spec itself is still in IETF draft status,
meaning **wire-protocol-breaking changes remain possible** — a materially different risk profile
than WebSocket (RFC 6455, stable since 2011).

**Deployment-shape cost, concretely, against this codebase**: `server/server.go`'s `Start`
([server.go:1162-1210](server/server.go#L1162-L1210)) runs exactly one `http.Server` today, over
either plain TCP (`listenLoopbackAware`) or `ListenAndServeTLS` when `s.tlsConfig != nil` (the
remote-access path, [server.go:1194-1200](server/server.go#L1194-L1200)). WebTransport requires an
**HTTP/3 server bound to a UDP port**, which `net/http`'s `http.Server` does not provide — it needs
`quic-go`'s own `http3.Server` running *alongside* the existing TCP listener, sharing the same TLS
certificate but requiring its own UDP socket bind, its own listener lifecycle, and (per WebTransport
spec) an `Alt-Svc` header advertised from the existing HTTP/1.1-or-2 responses so browsers know to
upgrade. This is strictly additive infrastructure — not a drop-in replacement for the existing
`gorilla/websocket` upgrade path — and every remote-access deployment consideration this repo
already tracks (self-signed cert handling, `.claude/docs/codesigning.md`-adjacent concerns named in
requirements.md's Rabbit Holes) would need to be re-verified against QUIC/UDP specifically: UDP is
more commonly firewalled/NAT-unfriendly than TCP 443, which matters for the "remote access over
Tailscale/LAN" use case this product already supports (`onyx.staplerhome.internal:8444` per project
memory) even though it's less of a concern for pure `localhost:8543` dev use.

**Fit assessment against this codebase's three subsystems**:

- **Terminal streaming** (`session/streamhub`): WebTransport's multiple-streams-per-connection
  model is the one place a real architectural question exists — see §4.
- **RPC bridge**: WebTransport doesn't obsolete the more fundamental question this file already
  raises on its own (§3c) — whether the hand-rolled envelope-over-WebSocket should first become
  ConnectRPC's *native* HTTP/2 bidi streaming (a smaller, already-available change) before
  reaching for a HTTP/3-only browser API that ConnectRPC itself doesn't have first-class support
  for today (ConnectRPC's protocol targets gRPC/gRPC-Web/Connect over HTTP/1.1 and HTTP/2; HTTP/3
  transport is not part of its documented protocol surface as of `connectrpc.com/connect v1.19.0`).
- **CDP/VNC proxies**: both are single-producer/single-consumer opaque byte pipes (§1c) — the
  latency/multi-stream benefits WebTransport offers don't address any bottleneck these handlers
  actually have; VNC/CDP are not latency-sensitive in the way terminal keystroke round-trip is, and
  WebTransport buys nothing here that a 32KB-buffered WebSocket relay doesn't already provide.

**Verdict: real candidate for terminal streaming's wire carrier only, gated on an infra
investment (HTTP/3/QUIC server) this product doesn't have any other use for yet** — not a "yes,"
but not a same-category rule-out as Iggy/yetty either. Triggering condition that would move this
from "interesting" to "worth doing": either (a) WebSocket's head-of-line-blocking or single-stream
limitation becomes an actually-observed problem for terminal streaming (not observed today — the
hub's own batching solved the concurrency/coalescing problem at a different layer, §1a), or (b) this
product independently adopts HTTP/3 for some other reason (e.g. general performance), making the
UDP/QUIC infra cost already-paid rather than newly-incurred just for this.

## 3. Architectural pattern: unify vs. leave separate

**Recommendation: leave the three subsystems on separate transport implementations; do not build
a net-new shared abstraction spanning all three.** Reasoning, subsystem by subsystem:

- **Terminal streaming already has the right-shaped abstraction** (`streamhub.Transport`, §1a) —
  narrow, sink-only, deliberately excludes framing/input. Extending *this* interface to also cover
  RPC or CDP/VNC would violate its own design intent (a `Transport` for hub broadcast is not a
  general-purpose "everything speaks over one interface" abstraction — see the interface-pollution
  checklist's "speculative interface" smell: an interface should be discovered from real shared
  behavior, not imposed because three things happen to move bytes over a network).
- **The RPC bridge's actual shape is request/response + server-streaming multiplexed over one
  connection via a message-type-discriminated envelope** — categorically different from
  `Transport`'s one-sink-per-subscriber model. Coercing it into `streamhub.Transport` would mean
  either (a) creating N fake "subscribers" per multiplexed logical stream, losing the envelope's
  multiplexing benefit, or (b) widening `Transport` to carry stream IDs and request/response
  semantics, at which point it's no longer the hub's narrow sink interface — it's ConnectRPC's own
  streaming abstraction, badly reimplemented. The more direct move, independent of any of the three
  external references, is **replace the hand-rolled envelope-over-WebSocket with ConnectRPC's
  native HTTP/2 bidi streaming** (Open Question in requirements.md, "is the real gap 'wrong
  transport' or 'reinventing what ConnectRPC already provides natively'") — this eliminates
  `marshalProtoEnvelope`, both hot-path `sync.Pool`s, and ~3000 lines' worth of hand-rolled framing
  concerns, without touching WebSocket vs. WebTransport at all. This is the single highest-leverage,
  lowest-risk finding of this review and is elaborated as the primary Phase 3 candidate.
- **CDP/VNC are opaque byte pipes with no multi-consumer requirement** (§1c) — there is no fan-out
  problem to solve, so there is nothing for a `Transport`-style abstraction to add. Forcing them
  onto `streamhub.Transport` would add an unused subscriber-registry/negotiation layer to two
  handlers that are, correctly, ~200-line self-contained relay loops today. This is a case where
  the subsystems' differing requirements (requirements.md's own framing: "low-latency bidi frames
  vs. request/response+streaming vs. opaque byte-proxying") genuinely justify staying separate, not
  organizational inertia.

**Adapter-pattern note for anything that *does* change**: if WebTransport is ever adopted for
terminal streaming specifically (§2c, §4), the correct integration shape is a **new `Transport`
implementation** (`WebTransportTransport`, sibling to `WebSocketTransport` and `MuxTransport`) —
not a parallel abstraction and not a change to the `Transport` interface itself, since §1a already
showed the interface doesn't need to know what carries the bytes. This is the adapter pattern
requirements.md's Open Questions names explicitly, and it's the only one of the three "how would
this plug in" shapes (adapter / net-new shared abstraction / leave separate) that applies to any
part of this review's scope.

## 4. Data flow / consistency: does a transport swap threaten the hub's single-owner invariant?

**No, with one caveat.** The hub research doc's ADR-001 (single-owner-stream-hub-per-session)
settled ownership at the *hub* layer — one `StreamHub` goroutine per tmux session, serializing
resize/quiescence/capture decisions (`resizeApplyMu`,
[hub.go:119-120](session/streamhub/hub.go#L119-L120)) regardless of how many `Transport`s or
subscribers are attached. `Transport.Send`/`Close` are called *by* the hub, never call back into
hub-owned mutable state except through `DetachSubscriber` (§1a's `WebSocketTransport.Close`,
[websocket_transport.go:94-105](server/services/websocket_transport.go#L94-L105), which is
explicitly designed to be safely reentrant via the CAS-guarded `closed` flag). A transport swap
changes what implements two methods; it cannot, by construction, touch the ownership/negotiation
logic that lives entirely inside `StreamHub` and `SessionController`.

**The caveat is at the subscriber-cardinality mapping, not the ownership model**: today, one
`WebSocketTransport` = one browser WebSocket connection = one hub `Subscriber`
([`BindSubscriber`](server/services/websocket_transport.go#L54-L62) binds exactly one
`SubscriberID` per transport instance). WebTransport's defining feature is **multiple independent
streams per connection** — a single WebTransport session could plausibly carry several logical
streams (e.g., terminal output + a side-channel) that a naive port might be tempted to map to
multiple hub subscribers from one browser tab. **This would not break the single-owner invariant
itself** (the hub still only has one owner goroutine per tmux session, regardless of subscriber
count), but it would change the *meaning* of "subscriber count" as an observability signal — the
hub research doc's own Observability Requirements (consolidate to one connection-count source of
truth) assumed roughly one subscriber ≈ one browser tab/client. If a future `WebTransportTransport`
maps one WebTransport *stream* (not *session*) to one hub subscriber, one browser tab reconnecting
or opening a redundant stream could silently inflate the subscriber count without a corresponding
new "connection" in the sense the dark-launch rollout's metrics/logs currently assume. **Any future
WebTransport adoption plan must specify: one hub `Subscriber` per WebTransport *session*
(connection), not per stream** — mirroring today's one-WebSocket-connection-per-subscriber
mapping — to keep the existing observability and ownership invariants intact. This is a
scoping note for a future plan.md, not a blocker: nothing forces the multi-stream mapping, and the
narrow `Transport` interface (§1a, §3) doesn't require or encourage it either.

## 5. Integration points, concretely — where would each candidate plug into `server.go`/session ownership

| Candidate | Plug-in point | What has to change | Verdict |
|---|---|---|---|
| Apache Iggy | N/A — no consuming code path exists for any subsystem | Would require running a second server process, standing up topics/streams, and rewriting at least one subsystem's producer/consumer model to fit pub/sub semantics it doesn't need | Not applicable (§2a) |
| yetty | N/A — client-side rendering, not server routing | Would replace xterm.js in `web-app/`, unrelated to `server.go`/transport routing entirely | Not applicable, wrong layer (§2b) |
| WebTransport (terminal streaming only) | New `http3.Server` alongside `s.httpServer` in [`server.go`'s `Start`](server/server.go#L1162-L1210); new `WebTransportTransport` implementing `streamhub.Transport` (sibling to `websocket_transport.go`); new `Alt-Svc` advertisement on existing HTTP responses; new route registration analogous to [`server.go:396-400`](server/server.go#L396-L400)'s `wsHandler.HandleWebSocket` | UDP port + cert wiring, one new `Transport` impl (§3), subscriber-per-session mapping discipline (§4) | Real candidate, infra-gated (§2c) |
| ConnectRPC native HTTP/2 streaming (RPC bridge) | Replaces the hand-rolled envelope loop in `connectrpc_websocket.go` with `connectrpc.com/connect`'s existing bidi-streaming handler registration, alongside the unary/server-streaming ConnectRPC handlers `server.go` likely already registers elsewhere for non-WebSocket RPCs | Removes `marshalProtoEnvelope`, both `sync.Pool`s, the 5-byte-header parsing — no new transport-layer infra needed, since it rides the existing HTTP/2 the Go server + TLS setup already support | Highest-leverage, lowest-risk candidate (§3) — recommend prioritizing in plan.md |

## 6. Interaction with `session/streamhub`'s in-flight dark-launch rollout

Per requirements.md's Constraints: this review's recommendations must not destabilize the
`session/streamhub` rollout before its own rollback-rehearsal/trial-period gate
(`project_plans/terminal-multi-connection-streaming/implementation/plan.md`) completes.

- **Nothing in this review's terminal-streaming-relevant finding (WebTransport as a possible future
  `Transport` implementation, §2c/§4) requires or suggests touching `PathHubOwned` vs.
  `PathLegacyPerConnection` resolution, `StreamOwnershipLock`, or any of the flag-scoping machinery
  in `connectrpc_websocket.go` (§1a of the hub research doc, ADR-003).** A new `Transport`
  implementation is additive to the set `{WebSocketTransport, MuxTransport, memory}` and slots in
  exactly where `WebSocketTransport` does today — behind the already-resolved `PathHubOwned` branch
  ([`connectrpc_websocket.go:770`](server/services/connectrpc_websocket.go#L770)) — not as a
  parallel ownership path.
- **Recommendation: do not attempt any WebTransport work while the hub rollout is still mid-trial.**
  Even though it's additive in principle, introducing a second wire-transport variable during the
  hub's own trial period would make it harder to attribute any dark-launch regression to "the hub
  redesign" vs. "the new transport," undermining the rollback-rehearsal's own signal quality. This
  review's plan.md (Phase 3, not produced in this pass per requirements.md's Non-functional/Appetite
  split) should sequence any WebTransport work strictly after the hub rollout's trial period
  concludes.
- **The ConnectRPC-native-streaming recommendation (§3, §5) is unrelated to `session/streamhub`
  entirely** — it touches `connectrpc_websocket.go`'s RPC-multiplexing machinery, not the
  terminal-streaming subscriber path, and has no interaction with the hub rollout's flag state.
  It can proceed on its own schedule without waiting on the hub trial.
- **No recommendation in this review retires or duplicates any `session/streamhub` code.** The
  `Transport` interface, `WebSocketTransport`, `MuxTransport`, and the hub's own batching/ownership
  logic are all confirmed correct-shaped and orthogonal to the transport-carrier question (§1a, §4)
  — this review finds nothing to unwind there, only a possible future addition alongside it.

## Summary of findings for Phase 3 planning

- **Apache Iggy**: not applicable to any of the three subsystems — scale/shape mismatch, would add
  an operational commitment (standalone broker) disproportionate to a single-operator tool with no
  multi-producer durable-log requirement. No plan.md warranted.
- **yetty**: not applicable and not on-topic — it's a client-side terminal-rendering project (like
  ghostty-web/wterm, already surveyed in `exotic-transports.md`), not a transport, plus carries a
  BSL 1.1 production-license cost. No plan.md warranted.
- **WebTransport**: a real candidate for terminal streaming's wire carrier specifically, cleanly
  additive via a new `streamhub.Transport` implementation with no threat to the hub's single-owner
  invariant (multi-stream-per-connection caveat noted, §4) — but gated on a real infra investment
  (HTTP/3/QUIC server alongside the existing `http.Server`, UDP/NAT considerations for remote
  access) this product has no other use for yet, and on WebTransport's own IETF-draft protocol
  stability risk. Sequence strictly after the `session/streamhub` dark-launch trial concludes (§6).
  Not applicable to the RPC bridge or CDP/VNC proxies (§2c).
- **Highest-leverage finding, independent of all three external references**: the RPC bridge's
  hand-rolled ConnectRPC-envelope-over-WebSocket should be replaced with ConnectRPC's own native
  HTTP/2 bidi streaming (§3, §5) — smaller change, no new infra, removes ~3000 lines of hand-rolled
  framing/pooling code, and is completely decoupled from the `session/streamhub` rollout so it can
  proceed independently. Recommend this be the first candidate scoped in plan.md.
- **Unification verdict**: do not build a shared transport abstraction spanning terminal streaming
  / RPC / CDP-VNC. `streamhub.Transport`'s narrow sink shape is correct for its own subsystem and
  would be diluted by forcing RPC multiplexing or opaque proxying through it (§3) — the three
  subsystems' differing requirements are real, not organizational debt.
