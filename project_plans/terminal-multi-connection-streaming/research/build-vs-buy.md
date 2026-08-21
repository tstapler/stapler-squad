# Build vs. Buy: Terminal Multi-Connection Streaming Hub

**Question**: should the per-session stream hub, transport abstraction, and wire-protocol
batching described in `requirements.md` be built from scratch, or sourced from an existing
library/framework/service?

## 1. Existing OSS library/framework

### 1a. Generic pub/sub or broadcast-hub abstraction

**Already-installed `github.com/puzpuzpuz/xsync/v4`** — Not a fan-out/broadcast library. It's a
concurrent map (`xsync.Map`), already used in this exact file
(`server/services/connectrpc_websocket.go:203-215`) to track per-session generation state
(`activeControlModeStreams`). It's the right tool for the hub's *registry* (session name →
hub, and hub → subscriber set) but does nothing for the broadcast/fan-out itself — that's a
different concern (delivering bytes to N goroutines), not a concurrent-map problem.
- **Verdict**: Recommended, but only as the registry component, not the hub/broadcast logic.

**`nats.go` embedded-mode (embedded NATS server in-process)** — A full pub/sub message broker
can run in-process without a separate server. Pros: mature, battle-tested wildcard subjects,
built-in backpressure/slow-consumer handling. Cons: this is a distributed-messaging system
being repurposed for single-process, single-session fan-out of byte slices — it adds a
non-trivial dependency (subject-based routing, its own goroutine pool, JetStream/config
surface) to solve a problem this repo already solves in ~15 lines three separate times (see
below). The requirements explicitly scope concurrency as "a handful of concurrent connections
per session at most," not a distributed or high-throughput case NATS is designed for.
- **Verdict**: Not recommended. Wrong tool for the scale; the abstraction NATS buys isn't one
  this repo needs (no cross-process/cross-host delivery is in scope for this pass — that's
  explicitly deferred to `ADR-002`'s Workspace Host Registry work).

**Hand-rolled channel fan-out** — This repo already implements this pattern three times,
independently, for exactly this problem shape (one producer, N consumers, byte-slice data):
  - `session/response_stream.go:310-330` (`ResponseStream.broadcast`) — `RLock`, iterate
    `map[string]subscriber`, non-blocking `select` send per subscriber.
  - `session/native_process_manager.go:443-463` (`NativeProcessManager.fanOut`) — same shape,
    closes+removes a subscriber on a full buffer rather than dropping frames (documented
    rationale: "dropping any byte corrupts ANSI/cursor state for terminal consumers").
  - `session/external_streamer.go:490` (`ExternalStreamer.broadcast`) — same shape again, for
    the ssq-mux external-session path.
  - Plus the *subscription* half already exists for tmux control-mode output specifically:
    `session/instance_tmux.go:731` (`SubscribeControlModeUpdates`) returns a subscriber ID and
    output channel — the fan-out for *output bytes* is not the missing piece; what's missing
    is fan-in/single-ownership of the *resize + capture-pane* side (see §3).
- **Verdict**: Recommended. This is a ~15-line, well-understood Go idiom (`map` + mutex +
  non-blocking channel send), the repo has three working, tested precedents for it, and a
  fourth copy for the new hub costs nothing that a dependency would offset. The real gap isn't
  "we don't know how to fan out bytes" — it's "three of the four fan-out sites don't share a
  registry, so nothing prevents two of them from acting as if they each independently own the
  underlying tmux session's resize/capture."

### 1b. Transport-agnostic streaming abstraction

**Hand-rolled interface (`type Transport interface { Send([]byte) error; ... }`)** — The
requirements already describe exactly this shape: "a small transport interface." Given three
required implementations (browser WebSocket, ssq-mux Unix socket, in-memory test transport)
sharing only "write bytes out, signal disconnect," a narrow, consumer-defined interface
(per this repo's own `.claude/rules/interface-pollution-checklist.md` — "define the interface
where it's consumed, scoped to only the methods that consumer needs") is the correct level of
abstraction, not a byte-for-byte re-implementation of an existing framework's transport model.
- **Verdict**: Recommended.

**`gorilla/websocket`** (`go.mod:32`, `v1.5.3`) — Already a direct dependency, already used in
`server/services/connectrpc_websocket.go`, `terminal_websocket.go`, `cdp_stream_handler.go`,
`vnc_proxy_handler.go`, `ws_stream_bridge.go`. It is in **maintenance mode** (the gorilla
toolkit was archived and un-archived by former maintainers; feature development is frozen,
security patches still land). For *this* redesign, gorilla/websocket is not being replaced —
it's one of the three required transport implementations (the browser WebSocket transport
wraps it, same as today). No change of library needed here.
- **Verdict**: Recommended (keep as-is) for the browser-WebSocket transport specifically.

**`nhooyr.io/websocket` (now `github.com/coder/websocket`)** — A more modern, context-aware
WebSocket library with a smaller API surface and native `io.Reader`/`io.Writer` support.
Genuinely nicer to use, but swapping it in would be a pure library-migration cost (touching
5 files: `connectrpc_websocket.go`, `terminal_websocket.go`, `cdp_stream_handler.go`,
`vnc_proxy_handler.go`, `ws_stream_bridge.go`) with no bearing on the actual problem this SDD
pass is solving (single-owner resize/capture + transport-agnostic fan-out). It would be
additive scope, not foundational.
- **Verdict**: Not recommended for this pass — out of scope; a separate, independently
  justified migration if ever pursued.

**ConnectRPC native bidi-streaming** (`connectrpc.com/connect v1.19.0`, already a heavy
dependency — used across `server/services/*.go` for RPCs, and `ServerStream[...]` generics
appear in `backlog_service_events.go`, `session_service.go`, `insights_service.go`, etc.) —
The repo already runs a ConnectRPC bidi-stream-*shaped* protocol over the raw WebSocket byteby
hand: `connectWebSocketStream` (line 529) and `marshalProtoEnvelope` (line 51) manually build a
5-byte ConnectRPC envelope header and write it over `*websocket.Conn` — i.e., today's code
already reimplements ConnectRPC's wire framing without using the generated
`*connect.BidiStream[Req, Resp]` type, likely because browsers can't originate real HTTP/2
trailer-framed ConnectRPC streams the way a Go/gRPC client can, and the WebSocket transport
needs to interoperate with `web-app/`'s browser client. This is exactly the "future/aspirational
consumer" the requirements name ("ConnectRPC bidi stream... per follow-on"), not a fit for the
in-scope browser-WebSocket transport this pass ships. Once the transport interface exists,
adding a real ConnectRPC bidi-stream transport (for a non-browser Go client, e.g. cross-host
per `ADR-002`) becomes a second `Transport` implementation with no core-logic changes — which is
precisely the payoff the requirements are architecting for.
- **Verdict**: Viable — as a **follow-on transport implementation** against the new interface,
  not as infrastructure for this pass's browser-WebSocket path (already correctly out of scope
  per `requirements.md`'s Out-of-Scope: "Cross-host session streaming").

## 2. SaaS/managed API

Not applicable, as expected. This is an internal, single-operator, localhost-first tool with an
explicit non-functional requirement that "no new external network exposure implied by this
item" and scalability is "single-operator instance, a handful of concurrent connections per
session at most." No managed streaming/pub-sub SaaS (Pusher, Ably, AWS IoT Core, etc.) offers
anything relevant to a same-process fan-out problem, and introducing one would add exactly the
external-network-dependency and operational surface the requirements rule out. No further
research warranted.

## 3. LLM-generated implementation vs. battle-tested library

Two genuinely distinct pieces of new code are in scope; they deserve different verdicts.

**(a) The core fan-out hub (single owner per session, N subscriber transports)** — This is a
small, well-understood pattern this repo has already implemented correctly three times
(`ResponseStream`, `NativeProcessManager.fanOut`, `ExternalStreamer.broadcast` — cited above).
None of those three has ever been the subject of a bug report or flaky-test finding in this
repo's history (unlike, say, the resize/quiescence logic, which `terminal-jank` and this very
item exist to fix). The bug this SDD pass fixes is not "fan-out is hard" — it's "resize+capture
authority isn't owned by anything," a *coordination* problem, not a *distribution* problem. A
hand-rolled hub (mutex/xsync-backed registry + one goroutine per session doing
resize→quiescence→capture→broadcast) is the right level of custom code. It should be built
test-first per this repo's own precedent (`golang-testing` skill, `session/response_stream.go`'s
existing test suite as a template) rather than sourced.
- **Verdict**: Build from scratch (bespoke Go), following the existing fan-out precedent.

**(b) Wire-protocol batching/coalescing (escape-sequence-boundary-safe framing)** — This is the
one place where "just hand-roll it" is a real risk, and the requirements say so explicitly
(Rabbit Holes: "naive frame coalescing can reorder or merge output in ways that break
escape-sequence integrity"). But the repo has *already* built and shipped this exact logic
once, successfully: the current `streamViaControlMode` coalescing loop
(`server/services/connectrpc_websocket.go:790-845`, using `coalesceBufPool`) drains
`updateChan` opportunistically and coalesces bursts into fewer wire writes, and the file's own
comments (line 733: "that snapshot" referencing the "garbled overlapping-column" fix, plus the
dedicated `pkg/ansi` package for ED3/escape-sequence filtering) show this problem — batching
raw tmux control-mode bytes without corrupting escape sequences — was already solved in
`project_plans/terminal-jank`. This is a case for **adapting existing, tested in-repo code**
(the coalescing-loop pattern plus `pkg/ansi`'s escape-sequence-aware handling) rather than
writing new batching logic from a blank page or reaching for an external library — no external
Go library targets "coalesce ANSI-safe terminal byte streams for multi-subscriber broadcast"
as a packaged concern; this is inherently coupled to this repo's own capture-pane/ED3 details.
- **Verdict**: Adapt existing in-repo logic (`coalesceBufPool` pattern + `pkg/ansi`), don't
  design batching from scratch and don't look for an external library — but *do* lift the
  per-subscriber coalescing loop that lives inline in `streamViaControlMode` today into a
  reusable piece the hub owns once, since the goal here is exactly to stop N subscribers from
  each independently doing this coalescing/framing work against a resize/capture race.

**Conclusion on repo precedent**: the pattern of hand-rolling bespoke solutions for terminal
streaming (ED3 filtering, quiescence detection, resize dead-banding, and now three independent
fan-out implementations) is still the right call here. The problem hasn't grown complex enough
to justify a dependency — what changed is the *number of independent copies* of coordination
logic (four fan-out sites, zero shared registries), which is a code-organization problem this
SDD pass's hub design solves directly, not evidence that hand-rolling itself was the mistake.

## 4. Fork or adapt an existing project

- **ttyd / gotty / wetty** — Single-process Go/C terminal-over-websocket servers. All three are
  built around "one PTY, one WebSocket connection" (gotty explicitly documents itself as
  single-client-oriented; ttyd added multi-client support later as a bolt-on, not a
  first-class hub design). None of them front a tmux control-mode session with multiple
  independent transport *types* (WebSocket + Unix socket + in-memory test) — the core ask of
  this item. Forking one would mean ripping out its transport layer and PTY-ownership model
  entirely, i.e., rewriting the exact thing this item is designing, while inheriting an
  unrelated project's HTML/JS frontend, auth model, and release cadence as dead weight.
  - **Verdict**: Not recommended.
- **xterm.js's attach addon** (`@xterm/addon-attach`) — A frontend-only convenience that pipes a
  WebSocket directly into an xterm.js instance. `web-app/`'s `XtermTerminal.tsx` already has
  its own React-integrated streaming client (`useTerminalStream.ts`) with reconnect and
  batching-aware handling; the attach addon assumes exactly the "one WebSocket, no batching
  protocol, no multi-subscriber semantics" model this item is moving away from. Irrelevant to
  the server-side hub design and not a fit for the browser client's existing architecture.
  - **Verdict**: Not recommended.
- **tmux's own multi-client control-mode handling** — Not forkable (C, embedded in tmux's own
  server/client protocol), but explicitly useful as a **reference design**, which is exactly
  how the requirements already treat it: the Rabbit Holes section flags that tmux control-mode
  (`-C`) has no native "multiple resize authorities" concept and that real `tmux attach`
  multi-client semantics (smallest-common-size negotiation across attached clients) may or may
  not carry over — Phase 2/3 must verify tmux's actual behavior directly rather than assume it.
  Reading `tmux`'s `tty-term.c`/`client.c` multi-attach resize negotiation as a design
  reference (not a dependency) is worth doing during Phase 3 design, not this research pass.
  - **Verdict**: Reference only, not a fork target.
- **Zellij's client/server split** — A Rust terminal multiplexer with a real multi-client
  attach model (multiple clients can attach to one session with independent pane layouts).
  Architecturally the closest conceptual analog to "single owning server, N attached clients,"
  but it's a different language (Rust, not Go), a different process model (Zellij owns the PTY
  itself; this repo drives tmux as an external process via control mode), and adapting its
  design would mean reading its multi-client sync protocol for inspiration, not integrating
  any of its code. Not a fork/adapt candidate; at most a design-reference alongside tmux's own
  multi-attach handling.
  - **Verdict**: Reference only, not a fork target.

## Summary Table

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| xsync.Map for hub registry | Already a dependency, already used for this exact purpose one file over | Doesn't solve broadcast/fan-out itself | **Recommended** (registry only) |
| nats.go embedded | Mature, handles backpressure | Wrong scale (distributed broker for single-process fan-out), new dependency surface | Not recommended |
| Hand-rolled channel fan-out | Repo has 3 working precedents, ~15 lines, no new deps | None significant at this scale | **Recommended** |
| Hand-rolled `Transport` interface | Matches repo's interface-pollution rule, fits 3 required implementations exactly | — | **Recommended** |
| gorilla/websocket (keep) | Already integrated across 5 files | Maintenance-mode upstream (patches only) | **Recommended** (no change) |
| nhooyr.io/websocket (coder/websocket) | Nicer API, context-native | Pure migration cost, 5 files, unrelated to this item's problem | Not recommended (this pass) |
| ConnectRPC native bidi-stream | Already a heavy dependency; fits future non-browser/cross-host transports | Browsers can't originate it; today's browser path already hand-rolls the wire framing for browser compat | Viable (follow-on transport only) |
| SaaS pub/sub | — | No fit: local-only, single-operator, no external exposure wanted | Not recommended |
| Bespoke hub (build) | Matches repo precedent, testable, right-sized | — | **Recommended** |
| Bespoke batching from scratch | — | Escape-sequence-boundary risk already solved once; re-solving from a blank page risks regressing `terminal-jank` fixes | Not recommended — adapt existing `coalesceBufPool`/`pkg/ansi` logic instead |
| Fork ttyd/gotty/wetty | Existing PTY-over-WS servers | Single-client-oriented cores; forking = rewriting this item's exact design anyway | Not recommended |
| xterm.js attach addon | — | Frontend-only, assumes the pre-redesign model | Not recommended |
| tmux control-mode / Zellij client-server | Real multi-client design precedent | Not forkable/adaptable (different language/process model) | Reference only |

## Bottom Line

Build, not buy — with one caveat: don't build the batching logic from a blank page. Ship a
bespoke hub (registry via `xsync.Map`, fan-out via the repo's existing channel-broadcast
pattern) and a narrow `Transport` interface with the three required implementations
(WebSocket wrapping the already-integrated `gorilla/websocket`, a Unix-socket implementation
for ssq-mux, and an in-memory test double). For the coalescing/batching wire protocol, lift and
generalize the existing `coalesceBufPool` loop and `pkg/ansi` escape-handling from
`streamViaControlMode` rather than redesigning frame-safety logic from scratch. Treat
ConnectRPC native bidi-streaming as a documented follow-on transport (fits the interface
cleanly once it exists) and cross-host/audit-sink work as out-of-scope per the requirements'
own Appetite section.
