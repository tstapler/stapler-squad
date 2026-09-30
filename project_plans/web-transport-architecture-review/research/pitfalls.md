# Research: Pitfalls for web-transport-architecture-review

Covers risks specific to *evaluating/adopting* Apache Iggy, yetty, and WebTransport in this
repo. Does not repeat `project_plans/terminal-multi-connection-streaming/research/pitfalls.md`
(broadcast-hub fan-out, batching/coalescing correctness, escape-sequence integrity, tmux
resize-authority races) — that doc's findings stand for the hub's *internal* design regardless
of what transport sits underneath it; this doc is about the transport swap itself.

## 1. Apache Iggy — operational burden mismatched to this product's scale

### 1a. Iggy is a standalone server product, not a library, and its own docs frame it as a
Kafka-class system

Apache Iggy is a persistent message-streaming platform in the Kafka/Redpanda category — a
Rust, thread-per-core, io_uring-based broker with its own storage engine, topics/partitions,
consumer groups, and (per its own docs) planned Viewstamped-Replication-based clustering
([Architecture | Apache Iggy](https://iggy.apache.org/docs/introduction/architecture/),
[About | Apache Iggy](https://iggy.apache.org/docs/introduction/about/)). It ships as a single
binary with no *external* dependencies (no separate ZooKeeper/etcd), which lowers the floor
versus Kafka, but "adopt Iggy" still means running, monitoring, backing up, and upgrading a
second long-lived server process next to `stapler-squad` itself — not adding a Go import.
Requirements.md's Rabbit Holes section already names this cost explicitly; the general
"you don't need Kafka" literature converges on the same trigger condition this repo should
apply: message-broker adoption pays for itself when you have independent
producers/consumers that must survive each other's downtime, replay/backlog semantics, or
multi-consumer fan-out across service boundaries — none of which describes a single Go process
streaming its own tmux output to browser tabs it also serves the HTML for
([No, You Don't Need Kafka for Everything](https://medium.com/@thecodealchemistX/no-you-dont-need-kafka-for-everything-5c7f05e9feeb),
[Kafka Is Overkill for 80% of Projects](https://medium.com/@techInFocus/kafka-is-overkill-for-80-of-projects-prove-me-wrong-0d966988b58d)).
This product has zero of Iggy's motivating problems today: one process owns the tmux session,
the browser tab and the server are colocated, and there is no cross-service fan-out requirement
in scope.

### 1b. The specific failure mode: adopting broker infrastructure "just in case," then paying
its ongoing tax forever

The recurring cautionary pattern across the sources above (and this repo's own
`exotic-transports.md` precedent, which rejected IWA Direct Sockets on an analogous
packaging-cost-vs-benefit basis) is not that message brokers are bad — it's that a broker added
for a problem the product doesn't yet have becomes a second thing to operate, secure, upgrade,
and reason about during every incident, permanently, for a benefit that never materializes. For
this repo specifically: no staging environment (requirements.md's own Constraints section) means
every Iggy upgrade is a production upgrade of a second stateful service, with its own
crash-recovery and disk-persistence behavior to reason about *in addition to* tmux/session state
recovery this repo already has to reason about. A `docker compose up` local dev loop doesn't
remove this cost — it just deprioritizes noticing it until an incident.

### 1c. Go SDK maturity: worse than "unverified," it's actively deprecated

Requirements.md flagged Iggy's Go client maturity as unverified. It's now verified and
concerning: the standalone `iggy-rs/iggy-go-client` repo is **archived** (read-only as of
June 24, 2025) with its own README's usage section marked `OBSOLETE; will be updated soon` and
an incomplete-feature roadmap (TCP/HTTP/QUIC protocol coverage and tests both listed as
not-yet-done) ([iggy-rs/iggy-go-client](https://github.com/iggy-rs/iggy-go-client)). The Go SDK
has since moved into the `apache/iggy` monorepo at
`github.com/apache/iggy/foreign/go` ([apache/iggy](https://github.com/apache/iggy)), listed
alongside Rust/C#/Java/Python/Node.js — but with no maturity disclaimer of its own, and
noticeably thinner documentation than the Rust SDK, which every one of Iggy's own docs and
samples treats as primary. **Any Iggy adoption plan should treat the Go client as the least
battle-tested path into the project, not a peer of the Rust SDK**, and budget for reading Iggy's
Go source directly (not just docs) when something breaks — with no staging environment, that
debugging happens in production.

### 1d. Verdict for Phase 3

High-confidence "not applicable" the same way IWA Direct Sockets was ruled out: adopting Iggy
solves a distributed-systems-scale problem (durable multi-consumer streams surviving producer
downtime, replay across service boundaries) this single-process, single-operator tool does not
have, at the cost of a second production service with an immature Go client and no staging
environment to validate upgrades against. The trigger condition that would flip this verdict:
stapler-squad growing a genuinely separate consumer of tmux/session output that must survive
the main server process restarting independently (e.g., a durable audit/replay service) — not
in scope today per requirements.md's Users/Consumers section.

## 2. yetty — not a transport at all; scope mismatch with the review's own premise

### 2a. What yetty actually is

`github.com/zokrezyl/yetty` is a **GPU-accelerated terminal emulator application**, not a
streaming transport or protocol library. Its own README: *"A GPU-accelerated terminal and rich
content runtime. It keeps normal terminal workflows intact, but lets programs place plots,
images, diagrams, documents, video, GUI panels, remote desktops, and AI-generated figures
directly into the same scrolling surface as text."* It's ~570 commits of pure C with WebGPU
rendering, built around a "figure" composition model (nested GPU-backed renderable units
sharing the terminal's scroll surface) ([zokrezyl/yetty](https://github.com/zokrezyl/yetty)).
It does define a wire protocol — OSC/DCS escape-sequence envelopes a child process can emit to
place rich content — but that protocol is a *local terminal-to-application* content-embedding
scheme (the same category as iTerm2's inline-image escape sequences or Kitty's graphics
protocol), not a browser-to-server network transport. There is no client/server streaming
architecture to compare against WebSocket, ConnectRPC, or WebTransport.

### 2b. Why this matters for the review

This is the same "does the name describe what I assumed" trap `exotic-transports.md` avoided
by disambiguating "WASM terminal client" before evaluating it. yetty is much closer to that
prior project's ghostty-web/wterm category (a client-side terminal *rendering* engine) than to
Apache Iggy or WebTransport (both genuinely transport-layer technologies). Comparing yetty
against "should this be our RPC bridge / CDP-VNC transport" is a category error — it answers a
different question ("should xterm.js be replaced with a GPU-rendered terminal that can show
rich content inline") that, per `exotic-transports.md`'s own precedent, belongs to a separate,
much smaller frontend-rendering track fully decoupled from this transport review.

### 2c. Bus-factor / license risk, if evaluated on rendering-swap merits anyway

Should Phase 3 want to note yetty for that separate rendering-swap track: it's licensed
**Business Source License 1.1** — free for non-production use, but production use requires a
commercial license (a materially different cost/commitment than the MIT/Apache-2.0 licensing of
xterm.js or ghostty-web). It's a small project (78 stars, 3 forks, 117 issues open, single
primary maintainer visible in the repo) — meaningful bus-factor risk for a dependency this
product would rely on for its primary UI surface, and the BSL terms would need explicit
legal/cost review before any production use, which is disproportionate to what this personal
tool needs from a terminal renderer today.

### 2d. Verdict for Phase 3

Not applicable to the transport question this review asks. If yetty is worth tracking at all,
it belongs alongside ghostty-web/wterm as a *rendering-layer* candidate outside this review's
scope (per `exotic-transports.md`'s own precedent for that category), not as a transport
comparison point against Iggy/WebTransport.

## 3. WebTransport — real technology, but two specific footguns and one deployment-shape blocker

### 3a. Browser support: now broadly viable, but verify the floor, not the ceiling

As of March 2026, WebTransport reached Baseline status across major browsers: Safari 26.4+
(macOS and iOS, shipped without flags as of March 2026), Firefox 114+, and Chrome 97+/Edge
98+/Opera 83+ ([WebTransport Is Now Baseline](https://webrtc.ventures/2026/04/webtransport-is-now-baseline-what-it-means-for-real-time-media/),
[WebTransport: Browser Support](https://www.testmuai.com/learning-hub/webtransport-browser-support/)).
This clears the "zero install step, normal browser tab" bar requirements.md sets — a materially
different outcome than the IWA Direct Sockets verdict. But the operator's own device/browser mix
should be checked against actual versions in use (an older pinned Safari or a locked-down
managed browser could still be below the floor) rather than assuming "Baseline" means every
device this product is actually used from is covered.

### 3b. Deployment-shape blocker: HTTP/3/QUIC termination clashes with this repo's existing
long-lived, import-once TLS model

This repo's current remote-access TLS (`server/tls.go`) is built around a **stable local CA
imported once per client device**, with per-network leaf certs regenerated only near their own
expiry (`EnsureNetworkTLSCerts`, `server/tls.go:46-58` — "The CA is intentionally kept stable
across SAN changes so that phones only need to import it once"). WebTransport's
`serverCertificateHashes` API — the mechanism that lets a browser trust a **self-signed**
certificate without a publicly-trusted CA — requires the pinned certificate's validity to be
**at most 14 days**, use an ECDSA (P-256) key (RSA is rejected), and be regenerated roughly every
13 days with the new hash redistributed to clients before the old one expires
([W3C WebTransport certificate constraints, via PR #375](https://github.com/w3c/webtransport/pull/375),
[WebTransport with serverCertificateHashes](https://viroh.net/serverhash)). That is the opposite
operational shape from this repo's current "import the CA once, leaf certs renew silently near
expiry" model — it would require either (a) building automated cert rotation + hash
redistribution to every client every ~13 days, a new recurring operational burden this
single-operator tool doesn't have today, or (b) using a real publicly-trusted CA-issued
certificate over standard HTTP/3 (no `serverCertificateHashes` needed), which requires a public
DNS name and public reachability — in tension with this repo's "internal/personal-use tool; no
new external network exposure implied" NFR. **Phase 3 must pick one of these two paths
explicitly and cost it out** — treating WebTransport as a drop-in swap for the existing WebSocket
TLS setup would understate the real deployment change.

### 3c. Go server-side library maturity: functional, but pinned to a moving IETF draft

`quic-go/webtransport-go` is the standard Go WebTransport server implementation, built on
`quic-go` (itself described as "production-ready QUIC"), but it implements **draft-16** of the
still-unfinalized IETF WebTransport-over-HTTP/3 spec, and its own docs warn: *"There is no
guarantee that browsers will update in a backwards-compatible way, or that webtransport-go will
support multiple draft versions at the same time. Support for WebTransport therefore might break
for a transition period"* ([webtransport-go](https://github.com/quic-go/webtransport-go),
[quic-go WebTransport docs](https://quic-go.net/docs/webtransport/)). An older, unrelated
`ollikarppinen/webtransport-go` implements draft-02 and is explicitly marked *not production
ready* — don't confuse the two packages when researching further. **Practical implication for a
single-operator, no-staging tool**: a browser auto-update landing a newer WebTransport draft
version could break the server-side connection with no warning and no staging environment to
catch it first — a materially different risk profile than WebSocket, whose wire protocol has
been stable (RFC 6455) for over a decade.

### 3d. Note: `quic-go`/`qpack` already appear in `go.sum`, but only as build-tool transitives

`go.mod` lists `github.com/quic-go/quic-go` and `github.com/quic-go/qpack` as indirect
dependencies. Verified via `go mod why github.com/quic-go/quic-go`: the only path is
`stapler-squad → buf CLI (github.com/bufbuild/buf/cmd/buf) → quic-go`, i.e. they're pulled in
by the `buf` protobuf-tooling dependency, not by anything the server itself imports. Their
presence in `go.sum` is not evidence toward WebTransport viability and should not be cited as
"we already depend on QUIC" in Phase 3 — adopting WebTransport would still mean adding a real,
direct dependency edge.

### 3e. Footgun: WebTransport is not "WebSocket over QUIC" — datagram/stream semantics are easy
to misuse

The most commonly cited WebTransport adoption mistake: assuming it's a drop-in WebSocket
replacement. WebTransport actually exposes two distinct primitives — **datagrams** (unreliable,
unordered, best-effort — for "latest value beats a stale one" data) and **streams** (reliable,
ordered, and *independent of each other*, so one slow stream never head-of-line-blocks another
on the same connection) — and porting a WebSocket protocol onto a single WebTransport
bidirectional stream leaves the entire benefit on the table
([Low-Latency Browser Communication with WebTransport](https://blog.openreplay.com/low-latency-browser-communication-webtransport/)).
Concretely dangerous for this repo: **`session/streamhub`'s `Transport` interface today is
`Send(data []byte) error` / `Close() error` — a single ordered byte-stream abstraction**
(`session/streamhub/transport.go:6-14`). If a future WebTransport adapter were bolted onto that
interface by routing terminal output through WebTransport *datagrams* for "lower latency,"
it would silently violate every invariant `terminal-multi-connection-streaming/research/pitfalls.md`
already documents as load-bearing — reordering (§2d of that doc), and worse, **datagram loss
landing mid-ANSI-escape-sequence**, which `EscapeSequenceParser.ts`'s partial-escape-holdback
logic (that doc's §2a) has no way to recover from, because it assumes byte-exact, in-order,
lossless delivery the way TCP-backed WebSocket guarantees today. Any WebTransport adapter for
terminal streaming **must** use a reliable stream, never a datagram, for PTY output — this is a
hard constraint the Phase 3 plan should state explicitly, not leave to implementation-time
judgment.

### 3f. Footgun: connection migration is a feature, not free — has session-identity implications

QUIC's headline connection-migration property (a connection surviving a client IP/network
change, e.g. switching wifi to cellular) is a genuine advantage over WebSocket-over-TCP, which
must fully reconnect on any network change. But it changes an assumption this repo's existing
code may rely on: a "connection" is no longer strongly tied to one network path, so any
transport-layer code that keys state by remote address (rather than an explicit stream/session
identifier) would misbehave under migration. Grep this repo's transport code for any
`r.RemoteAddr`-keyed state before assuming a WebTransport migration is purely a client-experience
win — CDP/VNC proxying in particular deals in raw byte-proxying where identity assumptions are
easy to bake in implicitly.

### 3g. Verdict for Phase 3

Applicable in principle (real technology, Baseline browser support, a real Go server library
exists) but **not a drop-in transport swap** — three separate costs must each be explicitly
scoped if recommended: (1) a new recurring cert-rotation operational burden or a public-DNS/
publicly-trusted-cert requirement in tension with the "no new external exposure" NFR, (2) a
still-drafting IETF spec with a Go library that warns about breaking on draft-version transitions
and no staging environment to catch it, and (3) a from-scratch design (not a `Transport`
interface swap) to actually get any benefit over WebSocket without violating the streamhub's
existing ordered-delivery assumptions. If recommended, Phase 3 should scope it as the RPC bridge
or CDP/VNC subsystem first (both already opaque byte/request-response oriented, lower blast
radius) rather than terminal streaming, given the mid-rollout constraint below.

## 4. Cross-cutting risk: destabilizing `session/streamhub`'s in-flight dark-launch rollout

### 4a. The specific failure mode

`session/streamhub`'s `Transport` interface (`session/streamhub/transport.go:6-14`) is
deliberately minimal — `Send([]byte) error` / `Close() error` — which is exactly right for its
three current implementations (browser WebSocket, ssq-mux Unix socket, in-memory test double),
all of which are genuinely ordered, reliable, single-logical-stream carriers. That minimalism is
also the risk surface: **the interface itself encodes no ordering/reliability contract**, so a
future transport implementation (WebTransport datagrams per §3e above, or any hypothetical Iggy
adapter treating each hub broadcast as an independent message with no guaranteed delivery order
across consumers) could satisfy the interface's Go type signature while violating the semantic
assumptions every consumer of `Transport` — the hub's own batching/sequencing logic
(`session/streamhub/batch.go`, `sequence.go`), and the client-side `EscapeSequenceParser.ts` —
already depends on. The interface's own doc comment ("the delivery mechanism a Subscriber uses
to receive hub output") never states ordering or reliability as a requirement because, until
now, every implementation trivially provided it via TCP/Unix-socket semantics.

### 4b. Why this is a live risk *right now*, not a hypothetical

`session/streamhub` is mid dark-launch (requirements.md's Constraints section, and
`terminal-multi-connection-streaming/implementation/plan.md`'s own rollback-rehearsal/trial-period
gate not yet run its course). Any Phase 3 recommendation that touches terminal streaming has to
assume its audience is a team mid-rollout on a design that has already had one full pitfalls pass
(the sibling doc read above) covering fan-out, batching, and resize-authority races — all of
which assumed transport-level ordering/reliability as a given. Introducing a transport swap
underneath that design, even behind its own separate flag, risks two failure classes
specifically:
1. **Silent semantic violation**: a new `Transport` implementation that compiles and passes the
   interface's trivial contract but drops/reorders frames under load — the existing test suite
   (`session/streamhub/*_test.go`, notably `memory_transport_test.go` and
   `failure_modes_test.go`) exercises failure modes of the *existing* reliable transports
   (slow subscriber, disconnect mid-stream) but has no test today asserting the hub's output
   is *not* silently correct under a transport that reorders or drops — because no such
   transport has existed until this review considers one.
2. **Rollback-path contamination**: if a transport swap is implemented as a change *inside* one
   of the existing `Transport` implementations (e.g., swapping the browser WebSocket transport's
   internals for a WebTransport stream without introducing a new, separately-flagged
   implementation), the streamhub rollback flag (`useStreamHub()`,
   `server/services/connectrpc_websocket.go:391`) no longer isolates the two changes — flipping
   it back to the old code path would not undo a transport change made *inside* the new path's
   own transport implementation, defeating the existing rollback mechanism's whole purpose.

### 4c. What Phase 3 must design against explicitly

- **Any transport change must ship as a wholly new `Transport` implementation behind its own
  independent flag**, never as an in-place modification of the existing WebSocket/ssq-mux/
  in-memory implementations — so the streamhub rollback flag and any new transport's rollback
  flag can be flipped independently, and a transport-level regression can be rolled back without
  touching streamhub's own already-in-trial rollout.
- **The `Transport` interface's doc comment should be upgraded to state its ordering/reliability
  contract explicitly** (e.g. "Send must deliver frames in the order called, exactly once, or
  return an error and be evicted — implementations backed by inherently unordered/lossy carriers
  such as datagrams must provide their own resequencing/reliability layer before satisfying this
  interface") *before* any new implementation is written against it, turning an implicit
  assumption into a checked one future contributors (including future Claude sessions) can't
  silently violate.
- **A dedicated regression test for the "transport violates ordering/reliability" case** should
  be added to `session/streamhub`'s existing test suite as part of any Phase 3 plan that
  introduces a new transport — a fake transport that deliberately reorders or drops frames,
  asserting the hub/subscriber layer either detects and resyncs or fails loudly, never silently
  corrupts. This closes the gap `failure_modes_test.go` currently has (§4b.1 above).
- **Sequencing** — do not attempt a transport swap for terminal streaming until
  `terminal-multi-connection-streaming`'s own rollback-rehearsal/trial-period gate has completed,
  per requirements.md's Constraints section; this is a hard precondition already stated in this
  project's own spec, restated here because it is the single highest-consequence way this review
  could go wrong.

## 5. What to explicitly design against in Phase 3's plan.md

1. **Do not recommend Apache Iggy** for any subsystem in this product's current single-process,
   single-operator shape — no cited use case overcomes the cost of operating a second stateful
   service with no staging environment and an unmaintained/immature Go client (§1).
2. **Do not treat yetty as a transport candidate** — it answers a rendering question, not a
   transport question; if tracked at all, route it to a separate frontend-rendering evaluation
   outside this review, and flag its BSL 1.1 license and small-project bus-factor if it ever is
   (§2).
3. **If WebTransport is recommended for any subsystem, cost the TLS/cert-rotation model change
   explicitly** — pick and state (a) automated ≤14-day cert rotation + hash redistribution, or
   (b) a publicly-trusted CA cert with public reachability, and reconcile whichever is chosen
   against the "no new external exposure" NFR (§3b).
4. **If WebTransport is recommended for terminal streaming specifically, mandate reliable
   streams only, never datagrams**, for any data currently carrying `EscapeSequenceParser.ts`'s
   ordering/completeness assumptions — state this as a hard constraint in the plan, not an
   implementation detail (§3e).
5. **Any new `Transport` implementation must ship behind its own independent rollback flag**,
   separate from streamhub's own dark-launch flag, and must not modify any existing `Transport`
   implementation in place (§4c).
6. **Sequence any terminal-streaming transport work strictly after
   `terminal-multi-connection-streaming`'s rollback-rehearsal/trial-period gate completes** — this
   is a precondition, not a scheduling nicety (§4c).
7. **Prefer scoping any first WebTransport adoption to the RPC bridge or CDP/VNC proxying**, both
   already request/response- or opaque-byte-oriented with lower coupling to the
   in-trial streamhub design, over terminal streaming, if a "prove it works before touching the
   riskiest subsystem" staged rollout is wanted (§3g).
8. **State the `Transport` interface's ordering/reliability contract explicitly in code** as part
   of any plan that adds a new implementation, and add a reordering/loss regression test to
   `session/streamhub`'s suite — turning §4a's implicit assumption into a checked one (§4c).

## Sources

- `project_plans/web-transport-architecture-review/requirements.md` (this project's own spec)
- `project_plans/terminal-multi-connection-streaming/research/pitfalls.md` (prior pitfalls pass
  for the streamhub design itself — not repeated here)
- `project_plans/terminal-multi-connection-streaming/research/exotic-transports.md` (prior
  "not applicable" precedent and methodology this doc follows for Iggy/yetty)
- `session/streamhub/transport.go` (the `Transport` interface, lines 6-14)
- `server/tls.go` (`EnsureNetworkTLSCerts`, lines 46-58 — this repo's stable-CA/import-once TLS
  model)
- `server/services/connectrpc_websocket.go` (ConnectRPC-envelope-over-WebSocket bridge,
  `useStreamHub()` flag at line 391)
- `go.mod` / `go mod why github.com/quic-go/quic-go` (confirms quic-go is a `buf` CLI transitive,
  not a direct dependency)
- [Apache Iggy: Architecture](https://iggy.apache.org/docs/introduction/architecture/)
- [Apache Iggy: About](https://iggy.apache.org/docs/introduction/about/)
- [apache/iggy (GitHub)](https://github.com/apache/iggy)
- [iggy-rs/iggy-go-client (archived)](https://github.com/iggy-rs/iggy-go-client)
- [No, You Don't Need Kafka for Everything](https://medium.com/@thecodealchemistX/no-you-dont-need-kafka-for-everything-5c7f05e9feeb)
- [Kafka Is Overkill for 80% of Projects](https://medium.com/@techInFocus/kafka-is-overkill-for-80-of-projects-prove-me-wrong-0d966988b58d)
- [zokrezyl/yetty (GitHub)](https://github.com/zokrezyl/yetty)
- [WebTransport Is Now Baseline](https://webrtc.ventures/2026/04/webtransport-is-now-baseline-what-it-means-for-real-time-media/)
- [WebTransport: Browser Support, Features, Use Cases](https://www.testmuai.com/learning-hub/webtransport-browser-support/)
- [quic-go/webtransport-go (GitHub)](https://github.com/quic-go/webtransport-go)
- [quic-go WebTransport docs](https://quic-go.net/docs/webtransport/)
- [ollikarppinen/webtransport-go (draft-02, not production ready)](https://github.com/ollikarppinen/webtransport-go)
- [W3C WebTransport PR #375 — certificate hash constraints](https://github.com/w3c/webtransport/pull/375)
- [WebTransport with serverCertificateHashes](https://viroh.net/serverhash)
- [Low-Latency Browser Communication with WebTransport](https://blog.openreplay.com/low-latency-browser-communication-webtransport/)
