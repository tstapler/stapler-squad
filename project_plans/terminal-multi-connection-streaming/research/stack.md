# Research: Technology Stack

Scope: what the current terminal-streaming stack is, what's already a dependency vs. what
would be new, and what 2026 community practice recommends for the pieces this redesign
touches (broadcast hub, transport abstraction, frame batching, ConnectRPC bidi streaming).

## 1. Current Stack (as-is)

| Layer | File | Notes |
|---|---|---|
| WebSocket transport | [`server/services/connectrpc_websocket.go`](server/services/connectrpc_websocket.go) (2698 lines) | Hand-rolled: parses a ConnectRPC-style envelope over a raw `gorilla/websocket` connection, not a real ConnectRPC-generated bidi stream. |
| Wire framing | [`server/protocol/envelope.go`](server/protocol/envelope.go) | `CreateEnvelope(flags byte, data []byte) []byte` / `ParseEnvelope` — `[1 byte flags][4 bytes length][N bytes data]`, one message per event, no batching. |
| Session ownership | `streamViaControlMode` (connectrpc_websocket.go:633), `streamShellViaControlMode` (:1187), `streamViaTmuxCapturePane` (:2170) | Each is invoked once per WebSocket connection; no per-session registry. |
| Race workaround (observability only) | `recordControlModeStreamStart`/generation counter (:206-261) | `xsync.Map[string, controlModeStreamGeneration]` logs a WARN on overlap; does not prevent it. |
| Snapshot cache | `snapshotCache *xsync.Map[string, sessionSnapshot]` (:203, :323-343) | Already uses `xsync.Map.Compute` for lock-free dirty-marking. |
| Second, separate transport | `ssq-mux` — see `.claude/docs/pty-multiplexing.md` | Unix domain socket (`/tmp/ssq-mux-<PID>.sock`), its own message types (Output/Input/Resize/Metadata/Ping-Pong), completely independent code path from the WebSocket one — no shared registry or hub today. |
| Frontend | `web-app/src/components/sessions/XtermTerminal.tsx`, `TerminalOutput.tsx`, `web-app/src/lib/hooks/useTerminalStream.ts` (673 lines) | xterm.js + TS; consumes the raw WebSocket envelope stream directly. |
| Proto | `proto/session/v1/session.proto:33` | `rpc StreamTerminal(stream TerminalData) returns (stream TerminalData) {}` — already declared as a real bidi-streaming RPC in the `.proto`, but the actual wire path in `connectrpc_websocket.go` bypasses generated ConnectRPC stream plumbing in favor of the hand-rolled envelope-over-gorilla-websocket approach above (the "ConnectRPC-adjacent" framing in the task prompt is accurate — the envelope format matches ConnectRPC's wire format but travels over a plain `websocket.Conn`, not `connect.BidiStream`).

Other `gorilla/websocket` call sites in the repo for context (not all in scope): `session/cdp/manager.go`, `server/services/ws_stream_bridge.go`, `server/services/cdp_stream_handler.go`, `server/services/terminal_websocket.go`, `server/services/vnc_proxy_handler.go`.

## 2. Already a Dependency (go.mod) — reuse these first

| Package | Version (go.mod) | Relevance to this redesign |
|---|---|---|
| `github.com/puzpuzpuz/xsync/v4` | v4.5.0 | Already used in this exact file for `snapshotCache` and `activeControlModeStreams`. Directly applicable to the new per-session connection registry (`hub.subscribers`) and a session→hub map (`hubRegistry map[string]*sessionHub` as `xsync.Map[string, *sessionHub]`). No exported lock — matches this repo's concurrency skill guidance (`golang-concurrency`) that xsync is structurally safe against holding a lock across I/O. |
| `golang.org/x/sync` | v0.21.0 | `errgroup` for hub shutdown fan-out (closing N subscriber goroutines and waiting), `semaphore` if bounding concurrent hubs/subscribers becomes necessary. Not currently imported in `connectrpc_websocket.go` but already in the module graph — zero new dependency cost. |
| `github.com/gorilla/websocket` | v1.5.3 | Current transport; see §4 for whether to keep it. |
| `connectrpc.com/connect` | v1.19.0 | Already used for other ConnectRPC services in `server/services/`; the proto already declares `StreamTerminal` as bidi (`stream TerminalData` both ways). Using a *real* `connect.BidiStream` here (instead of the hand-rolled envelope-over-websocket) is a legitimate architectural option — see §5. |
| `connectrpc.com/otelconnect` | v0.8.0 | Already wired for observability on other RPCs; would give the hub's connection lifecycle free tracing if migrated to real ConnectRPC bidi streaming. |

**Nothing in the "hub/broadcast" or "transport abstraction" space needs a new third-party dependency.** The hub can be built entirely from stdlib (`sync.Mutex`/channels for per-hub subscriber fan-out, per the `golang-concurrency` skill's decision tree — a hub's subscriber list is a "rarely-modified list read on broadcast," a good fit for a COW slice or a small `xsync.MapOf` keyed by subscriber ID) plus `xsync` (already present) for the session→hub registry.

## 3. Broadcast/Fan-Out Hub Pattern — precedent and shape

No existing `session/mux`-style hub for *this* purpose exists in the repo yet: `session/mux/` (`multiplexer.go`, `socket_registry.go`, `tmux_socket.go`, `picker.go`, `discovery.go`) is about **tmux socket discovery/selection** (finding and picking tmux server sockets across workspaces), not a pub/sub broadcast hub — it's a different concern with a similar-sounding name. It's a candidate location to model conventions from (structured logging style, `xsync` usage) but not something to extend directly.

The generation-counter code at `connectrpc_websocket.go:206-261` is the closest existing artifact to "hub ownership" — it already tracks "which invocation owns tmux session X right now" via `xsync.Map[string, controlModeStreamGeneration]`, but only for logging. The new hub design's session-registry responsibility (`hubRegistry: xsync.Map[string, *sessionHub]`, one entry per tmux session, `Compute`-based get-or-create to avoid a race on hub creation — the same `Compute` idiom already used for `snapshotCache`) is a structural generalization of this existing map, not a new pattern for the codebase.

**Standard Go hub-and-spoke shape** (per 2026 community practice — see Sources), directly applicable:
- A `Hub`/`sessionHub` struct owns the single tmux control-mode connection for a session (resize + quiescence-wait + capture-pane happen only inside the hub, never per-connection).
- Each attached transport gets a per-subscriber outbound channel; the hub writes to all subscriber channels on each captured frame (fan-out), never doing I/O itself inside a lock.
- Slow subscriber isolation: give each subscriber its own goroutine draining its channel into `transport.Send()`, so one slow WebSocket write doesn't stall the hub or other subscribers — this is the load-tested failure mode called out in the 2026 ConnectRPC/gRPC search results ("naive per-message broadcast/goroutine fan-out can hit scaling bottlenecks" at high publisher counts) but is a non-issue at this project's stated concurrency scale ("a handful of concurrent connections per session at most" per the requirements' Non-Functional Requirements).
- Register/unregister via channels or `xsync.MapOf[subscriberID, *subscriber]` rather than a `sync.RWMutex`-guarded map — avoids the classic hub gotcha (RWMutex contention on register/unregister racing with broadcast) that the community pattern otherwise defaults to.

## 4. Transport Abstraction Pattern

No transport-agnostic interface exists yet in this codebase for the terminal stream — `connectWebSocketStream` (connectrpc_websocket.go:529-540) wraps `*websocket.Conn` directly and is passed by concrete type into `streamViaControlMode` et al. This is exactly the smell flagged by this repo's own [`interface-pollution-checklist.md`](.claude/rules/interface-pollution-checklist.md) — except here it's the *opposite* problem (a missing interface, not a speculative one): three real implementations are named in scope (browser WebSocket, ssq-mux Unix socket, in-memory test transport), so per the checklist's guidance ("use a concrete type until a second real implementation exists or is imminent") an interface is justified now, and per the repo convention it should be **defined where it's consumed** (the hub's package/consumer side) with only the methods the hub actually calls (e.g. `Send([]byte) error`, `Close() error`, maybe `Resize(cols, rows int)` if resize requests flow transport→hub) — not a broad "Transport" interface mirroring `*websocket.Conn`'s full API.

`gorilla/websocket` vs `coder/websocket` (formerly `nhooyr/websocket`): 2026 community consensus (see Sources) is to prefer `coder/websocket` for *new* code — it's actively maintained (gorilla/websocket's upstream org archived in late 2022, though the websocket package specifically still receives patches), uses `context.Context` idiomatically, and — notably for this exact redesign — handles concurrent writes internally, whereas `gorilla/websocket` panics if two goroutines call `WriteMessage` concurrently (the existing `connectWebSocketStream.WriteMessage` at connectrpc_websocket.go:536-540 works around this today with an explicit `sync.Mutex`). This is a real, evaluable option for the browser-WebSocket transport implementation, but swapping the underlying library is orthogonal to the transport-interface work and shouldn't block it — the interface should be defined so either library satisfies it. Recommendation: **keep `gorilla/websocket` for this pass** (it's a working, hidden-behind-a-new-interface implementation detail; the redesign's blast radius is already large per the Appetite/Constraints sections) and note `coder/websocket` as a candidate follow-on swap once the transport interface exists and can be validated against a second implementation without touching hub logic.

## 5. Frame/Message Batching

No existing batching/coalescing utility in this repo for WebSocket frames. The closest prior art is `project_plans/terminal-jank` and `project_plans/terminal-resize-fit-loop` (named directly in the requirements' Rabbit Holes and NFRs) — their "decoupled sampler" tick-interval approach is the established precedent for time-windowed coalescing in this codebase's terminal-output path, and the requirements explicitly direct that the new batching design be reviewed against that prior work rather than designed from scratch (escape-sequence integrity across a batch boundary is the named risk). No third-party batching library is warranted — this is a small, bespoke buffer-and-flush-on-tick loop (stdlib `time.Ticker` or a channel-based debounce), matching the qualitative/low-scale performance NFR ("a handful of concurrent connections per session at most").

## 6. ConnectRPC Bidi Streaming as a Transport Alternative

The `.proto` already declares `StreamTerminal` as a true bidi-streaming RPC (`proto/session/v1/session.proto:33`), and `connectrpc.com/connect` v1.19.0 is already a dependency used elsewhere in `server/services/`. This makes "replace the hand-rolled envelope-over-`*websocket.Conn`" with a real `connect.BidiStream[TerminalData, TerminalData]` a legitimate architectural alternative to keeping the current approach, and it would compose cleanly with the hub design: the connect-generated stream would just be one more `Transport` implementation, using its own `Send`/`Receive` instead of `WriteMessage`/`ReadMessage`. Trade-offs to weigh in Phase 3 planning (not decided here):

- **Pro**: eliminates the hand-rolled envelope parsing entirely for the browser path (real ConnectRPC handles framing, content negotiation, and interop with `connectrpc.com/otelconnect` tracing for free); aligns the actual wire path with what the `.proto` already promises.
- **Con**: `connect.BidiStream` over plain HTTP/1.1 (no TLS) doesn't get real client-driven half-duplex the way HTTP/2 does — ConnectRPC's docs note bidi streaming needs end-to-end HTTP/2, which typically requires TLS termination in front of the Go server (`net/http`'s server and client only negotiate HTTP/2 automatically over TLS); this repo's server binds `localhost:8543` with no TLS today (per CLAUDE.md), so adopting native bidi ConnectRPC would need either an h2c (cleartext HTTP/2) listener or a reverse proxy in front, a nontrivial deployment change for a single-operator internal tool. This is very likely **out of appetite for this pass** and worth flagging explicitly as a Phase 3 "considered, deferred" item rather than silently dropped.
- The existing envelope-over-WebSocket approach sidesteps the HTTP/2 requirement entirely (WebSocket upgrades work fine over plain HTTP/1.1), which is almost certainly *why* it was hand-rolled this way originally — worth confirming with `git blame`/prior ADRs if this decision's history matters to the plan, but not verified as part of this research pass.

**Recommendation**: keep the WebSocket-carrying-envelopes approach as the near-term transport for the browser client (folded behind the new `Transport` interface), and record native ConnectRPC bidi streaming as a documented, deferred alternative — not a decision to make in this pass, consistent with the Appetite section's scoping-down.

## Summary of New vs. Existing Dependencies

| Need | Existing dependency reused | New dependency required? |
|---|---|---|
| Session→hub registry, subscriber registry | `xsync.Map`/`xsync.MapOf` (already in go.mod, already used in this file) | No |
| Hub shutdown coordination | `golang.org/x/sync/errgroup` (already in go.mod, not yet imported here) | No |
| Browser WebSocket transport | `gorilla/websocket` (already in go.mod) | No (keep for this pass; `coder/websocket` is a documented future swap, see §4) |
| ssq-mux transport | stdlib `net` (Unix domain sockets) — ssq-mux's existing protocol | No |
| In-memory test transport | stdlib only (channels) | No |
| Frame batching | stdlib `time.Ticker`/channels, following `terminal-jank`/`terminal-resize-fit-loop` precedent | No |
| Native ConnectRPC bidi (deferred alternative, §6) | `connectrpc.com/connect`, `connectrpc.com/otelconnect` (already in go.mod) | No, but would need an h2c/TLS deployment change — out of scope for this pass |

**Bottom line: this redesign is achievable with zero new Go dependencies.** Everything needed (lock-free concurrent maps, error-group coordination, an existing WebSocket library, stdlib channels/tickers) is already in `go.mod`. The work is architectural (introducing the hub, the transport interface, and the batching loop), not a dependency-acquisition problem.

## Sources

- [Go WebSocket Server Guide: coder/websocket vs Gorilla](https://websocket.org/guides/languages/go/)
- [coder/websocket GitHub](https://github.com/coder/websocket)
- [gorilla/websocket GitHub](https://github.com/gorilla/websocket)
- [Connect RPC — Streaming (Go)](https://connectrpc.com/docs/go/streaming/)
- [The Mysterious Gotcha of gRPC Stream Performance — Ably](https://ably.com/blog/grpc-stream-performance)
- [How to Handle Bidirectional Streaming in gRPC — OneUptime](https://oneuptime.com/blog/post/2026-01-24-grpc-bidirectional-streaming/view)
