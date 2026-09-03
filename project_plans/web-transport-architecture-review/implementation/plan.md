# Implementation Plan: web-transport-architecture-review

**Feature**: Adopt ConnectRPC's native HTTP/2 streaming for `WatchSessions`/`WatchReviewQueue`,
scoped to the TLS remote-access listener (`Server.StartRemote`, `:8444`) where real
ALPN-negotiated HTTP/2 already works today with **zero server-side code changes**; the plain
`:8543` dev listener keeps its current `StreamingWSBridge` (WebSocket-upgrade) behavior
unconditionally, forever — cleartext HTTP/2 (h2c) is rejected outright, not merely deferred,
because no shipping browser implements it. Reject Apache Iggy and yetty outright; defer
WebTransport until `session/streamhub`'s dark-launch rollout concludes; leave CDP/VNC proxying
unchanged.
**Date**: 2026-08-22 (revised 2026-08-22 — plan-repair pass resolving the architecture-review
and adversarial-review BLOCKERs; see revision notes inline)
**Status**: Ready for implementation (deferred to a follow-on session per user's explicit appetite choice)
**ADRs**: ADR-001-adopt-connectrpc-native-streaming-for-server-streaming-rpcs (revised in this pass), ADR-002-defer-webtransport-until-streamhub-rollout-concludes (unchanged)

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `Transport` (streamhub) | The two-method sink interface (`Send([]byte) error`, `Close() error`) `session/streamhub` uses to hand a subscriber's terminal frames to the wire; deliberately excludes framing/negotiation/read. | `session/streamhub/transport.go:6-14`. Not modified by this plan. |
| `StreamingWSBridge` | The `server/services/ws_stream_bridge.go` type that wraps a Connect HTTP handler to also accept a single WebSocket connection per server-streaming RPC call, working around the browser's 6-connections-per-origin HTTP/1.1 cap. | **Revised**: this plan does **not** make it conditionally bypassable, and it is **not** eventually deletable. It stays registered unconditionally on both listeners forever — see Follow-on/Deferred Work. Its existing fallback-forwarding behavior (`ws_stream_bridge.go:44-62`) is exactly what makes the TLS-listener adoption below work with zero routing change. |
| ConnectRPC native streaming | `connectrpc.com/connect`'s own HTTP/2-based streaming transport (server-streaming, client-streaming, bidi) — no custom WebSocket envelope required. | Already a direct dependency (`v1.19.0`); this plan activates it for two RPCs, **on the TLS remote-access listener only**. |
| Server-streaming RPC | An RPC where the client sends one request and the server returns a stream of responses (no further client input). | `WatchSessions`, `WatchReviewQueue` — the only two RPCs in scope for adoption. |
| Bidirectional (bidi) / client-streaming RPC | An RPC where the client sends a continuous stream of messages (terminal input is this shape). | `StreamTerminal` and friends — explicitly out of scope; browsers cannot reliably stream request bodies over `fetch()`. |
| ConnectRPC envelope | The wire framing ConnectRPC's streaming protocols use: 1 flags byte + 4-byte big-endian length prefix, then the protobuf payload. | Hand-rolled today in `connectrpc_websocket.go`'s `marshalProtoEnvelope` and mirrored in `watch-ws-transport.ts`'s `encodeEnvelope`. Native transport does this internally; no app code touches it once adopted. |
| Cleartext HTTP/2 (`h2c`) | HTTP/2 negotiated over plain TCP without TLS. | **Revised — rejected outright, not adopted.** No shipping browser implements h2c: Firefox Bugzilla #1418832 ("Consider implementing h2c") is still open; Chromium has never shipped it (crbug #1409512 / #580796). The original plan's `Server.Protocols.SetUnencryptedHTTP2(true)` wiring on `:8543` is dropped entirely — see ADR-001's Alternatives Considered. |
| Real (ALPN) HTTP/2 on the TLS remote-access listener | `Server.StartRemote`'s `http.Server` (`server/server.go:1378-1424`) already negotiates HTTP/2 automatically via TLS ALPN — confirmed via `go doc net/http`'s HTTP/2 section ("Server ... automatically enable HTTP/2 support when using HTTPS"), true since Go 1.6, no 1.24+ feature required. | This is the actual mechanism this plan adopts. Zero code change needed to enable it — it already works today. |
| `NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING` | Frontend build-time env var flag (default unset/false) gating **intent** to use native ConnectRPC transport in the three affected hooks. | **Revised**: this is the *only* new flag in this plan (the original plan's server-side `STAPLER_SQUAD_USE_CONNECTRPC_NATIVE_STREAMING` is dropped — there is no server-side behavior left to gate). Intent alone is not sufficient: `createSessionWatchTransport` additionally requires the resolved `baseUrl` to start with `https://` before it actually returns the native transport — see Epic 1.3. |
| `createWatchTransport` | The existing WS-bridge ConnectRPC `Transport` implementation in `watch-ws-transport.ts`, consumed today by `useShells.ts`, `useReviewQueue.ts`, `useSessionService.ts`. | Stays in place as the permanent fallback for the plain `:8543` listener — not a flag-off shim scheduled for removal. |
| `createConnectTransport` | `@connectrpc/connect-web`'s stock native transport — already imported in `watch-ws-transport.ts` for unary calls, extended by this plan to also cover streaming when the flag is on **and** the connection is to the TLS remote-access listener. | No new frontend dependency. |
| Apache Iggy | A standalone Rust message-streaming broker (Kafka-class). Rejected: operational/scale mismatch, no subsystem here has a durable multi-consumer fan-out need. | `research/build-vs-buy.md` §1. No implementation tasks. Unchanged by this revision. |
| yetty | A GPU-accelerated terminal *emulator/client* (`github.com/zokrezyl/yetty`), not a transport. Category error for this review. | `research/stack.md` §2. No implementation tasks. Unchanged by this revision. |
| WebTransport | A browser API for multiplexed, QUIC/HTTP-3-based bidirectional messaging; W3C explainer. The one real transport-layer contender found, deferred per ADR-002. | Not implemented in this plan; see Follow-on/Deferred Work. Unchanged by this revision. |
| `session/streamhub` dark-launch trial | The in-flight, flag-gated rollout of the `StreamHub` design from `project_plans/terminal-multi-connection-streaming/`, not yet past its own rollback-rehearsal/trial-period gate. | This plan's adopted work (ADR-001) is fully decoupled from it. Unchanged by this revision. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Server protocol negotiation, TLS remote-access listener (`:8444`) | None needed — rely on Go stdlib's already-automatic TLS-ALPN HTTP/2 negotiation | `go doc net/http` ("Server ... automatically enable HTTP/2 support when using HTTPS"); verified against `server/server.go:1378-1424`'s `StartRemote`, which sets neither `Protocols` nor `TLSNextProto` | Explicit `Server.Protocols.SetUnencryptedHTTP2(true)` on the plain `:8543` listener (the original plan's approach) | No shipping browser implements cleartext HTTP/2 (Firefox Bugzilla #1418832 open; Chromium crbug #1409512/#580796 unimplemented) — the mechanism would deliver zero benefit to any real browser tab on `:8543`, while being a server-wide capability change with real risk to the `http.Hijacker`-dependent WS-upgrade paths (`StreamTerminal`, VNC, CDP) sharing that listener's mux — adversarial review BLOCKER |
| Server route bypass for `WatchSessions`/`WatchReviewQueue` | None — `StreamingWSBridge` registration stays unconditional, permanently, on both listeners | `ws_stream_bridge.go:44-62`'s existing fallback-forwarding behavior (verified by direct read) | Conditional registration skip when a server flag is on (the original plan's Task 1.2.1a) | Unnecessary and actively regressive: the bridge's `Handler()` already forwards every non-WebSocket-upgrade request to the same Connect handler instance registered at the general subtree path, so a native-transport client is already served correctly with the bridge registered. Skipping registration instead breaks every still-WS-upgrading client (the default, since the frontend flag defaults off) — architecture review BLOCKER |
| Frontend transport selection | Strategy (GoF) via factory function, additionally gated on the resolved connection's protocol | GoF Strategy; ConnectRPC's own `Transport` interface is already this shape | Gate on the build-time flag alone, with no runtime check | The flag alone can't distinguish which listener served a given page load, because the web UI is a **single build** served identically by both `Server.Start` (`:8543`) and `Server.StartRemote` (`:8444`) via one shared `distFS` registration (`server/server.go:1033-1058`, `registerStaticRoutes`). Gating on `opt.baseUrl.startsWith("https://")` (derived at runtime from `window.location.origin`, `web-app/src/lib/config.ts:14-29`) in addition to the flag is what makes native transport unreachable on the plain listener regardless of flag state — closing the adversarial review's Blocker 1 regression risk by construction |
| `StreamTerminal` / bidi RPC path | No pattern change — stays on the existing hand-rolled envelope-over-WebSocket in `connectrpc_websocket.go` | N/A (out of scope) | Wrapping it in an Adapter to also expose a native-streaming-compatible interface | Browsers cannot reliably stream client request bodies over `fetch()` regardless of HTTP/2 — no adapter closes a browser-platform gap; see ADR-001's Alternatives Considered. Unchanged by this revision. |
| Feature-flag rollout mechanics | Single frontend-only dark-launch flag, independently flippable, no shared server-side state | This repo's own precedent: `NEXT_PUBLIC_RECONNECT_V2` | A paired server+frontend flag (the original plan's `STAPLER_SQUAD_USE_CONNECTRPC_NATIVE_STREAMING` / `NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING`) | There is no server-side behavior left to gate once the routing-bypass (Epic 1.2) and h2c wiring (part of Epic 1.1) are dropped — a server-side flag with nothing to toggle is dead code. The frontend flag remains, but its effect is additionally scoped by the `https://` runtime guard rather than by a second, now-nonexistent server flag |

### Step 0.5 — Alternatives considered for plan scope (creative pass)

*(Unchanged from the original planning pass — this section evaluates plan-scope alternatives,
not the specific transport mechanism that was corrected below.)*

1. **Maximal "redo everything" plan** — replace all three subsystems' transports in one pass
   (WebTransport for terminal streaming, ConnectRPC-native for the RPC bridge, a new
   abstraction for CDP/VNC). *Strength*: theoretically "solves" the whole review in one shot.
   *Weakness*: directly contradicts requirements.md's own Rabbit Hole warning against scope
   creep, and would touch `session/streamhub` mid-rollout, violating the Constraints section
   outright. **Rejected** — recorded in the Pattern Decisions table implicitly via every
   subsystem-specific rejection above.
2. **Recommendation-only, no plan.md** — write up findings and stop. *Strength*: fastest,
   avoids committing to task-level detail that might be wrong. *Weakness*: requirements.md
   explicitly asks for a scoped, executable plan for any "adopt" verdict — stopping short
   would not meet the stated Appetite. **Rejected** per explicit requirement.
3. **Scope adoption narrowly to the one place all six research docs converge (ConnectRPC-native
   for server-streaming-only RPCs), defer WebTransport, reject Iggy/yetty outright, leave
   CDP/VNC alone** (chosen). *Strength*: every research doc independently reaches the same
   subsystem-by-subsystem verdict — this is convergent evidence, not one agent's opinion.
   *Weakness*: it's a small, single-epic plan relative to the review's three-subsystem scope —
   but per requirements.md's own Rabbit Hole #4 ("Phase 3 planning must resist producing a
   maximal 'redo everything' plan"), small-and-correct is the intended outcome, not a
   shortcoming.

---

## Technology Verdicts

| Technology | Verdict | Rationale (1-2 lines) | Trigger condition to revisit | Citation |
|-----------|---------|------------------------|-------------------------------|----------|
| Apache Iggy | Not applicable | Standalone Rust message broker (Kafka-class); none of the three subsystems have a durable, multi-consumer fan-out need, and adopting it means operating a second stateful production service with no staging environment and an immature Go client. | A concrete durable, multi-consumer fan-out requirement emerges (e.g. persisting terminal output for later replay to multiple independent audit/recording sinks) that a single-owner in-process hub can't serve — not merely "more concurrent viewers of one session," which `session/streamhub` already handles. | `research/build-vs-buy.md` §1 |
| yetty | Not applicable | GPU-accelerated terminal *emulator/client* app (`github.com/zokrezyl/yetty`), not a network transport; also BSL 1.1 licensed (production use requires a commercial license). | None — category error, not a transport-layer decision; would only become relevant to a separate, unrelated "swap the terminal renderer" review (out of scope here, per `research/stack.md`'s citation of the prior `exotic-transports.md` pattern for that class of question). | `research/stack.md` §2 |
| WebTransport (terminal streaming) | Deferred | Real technology, Baseline browser support as of March 2026, but requires a new QUIC/UDP listener this product has no other use for, an incompatible ≤14-day cert-rotation model for its self-signed-cert path, and must not run concurrently with `session/streamhub`'s in-flight dark-launch trial. | See the three numbered conditions in Follow-on/Deferred Work / ADR-002. | ADR-002 |
| ConnectRPC-native (server-streaming RPCs: `WatchSessions`, `WatchReviewQueue`) | Adopt — **scoped to the TLS remote-access listener only** (revised) | Already a direct dependency. `StreamingWSBridge` (143 lines) remains required, unconditionally, on the plain `:8543` listener — no shipping browser implements cleartext HTTP/2, so there is no way to close that gap there. The TLS remote-access listener (`:8444`) already negotiates real ALPN HTTP/2 automatically (`go doc net/http`, verified) — adopting native transport there needs zero server-side code, only a guarded frontend transport switch. | N/A — this is the adopted verdict; see Phase 1 below. | ADR-001 (revised) |
| CDP/VNC proxying | No change | Opaque, single-producer/single-consumer byte pipes with no multi-consumer or resync requirement any of the three technologies address; every research doc independently reaches "status quo." Also unaffected by this plan's revised scope: no `Server.Protocols`/`SetUnencryptedHTTP2` change is made anywhere, so the WS-upgrade (`http.Hijacker`-dependent) paths these proxies share a mux with are provably untouched. | A concrete new requirement emerges (e.g. multi-viewer CDP/VNC support) — a product decision, not a transport-technology one; see Follow-on/Deferred Work. | `research/architecture.md` §1c/§5, `research/build-vs-buy.md` summary table |

**Unification verdict** (closes requirements.md's Scope/Open-Questions question of whether the
three subsystems should share one transport abstraction): **No — stay separate.** Terminal
streaming (low-latency bidi frames, single-owner fan-out via `session/streamhub`), RPC (mixed
unary/server-streaming/bidi, now split across native ConnectRPC HTTP/2 and the WS bridge
depending on listener), and CDP/VNC (opaque single-consumer byte proxying) have structurally
different requirements — see `research/architecture.md`'s per-subsystem transport-boundary
mapping. Each subsystem's technology choice is decided independently above; no shared
abstraction beyond ConnectRPC's own `Transport` interface (already used piecemeal) would reduce
real duplication without forcing an artificial common denominator across genuinely different
problems.

## Migration Plan

Not applicable — no schema or persisted-data changes. This plan changes frontend transport
selection only; no server routing or protocol-negotiation code is touched.

## Observability Plan

- **Logs**: No new server-side log line is added. The original plan's flag-state log
  (originally Task 1.2.1c, which the architecture review recommended reattaching to Epic 1.1)
  is now moot: this revised plan adds no server-side flag and no `Server.Protocols` change of
  any kind, so there is no new server-side state to log. The existing
  `log.Info("Registered StreamingWSBridge", ...)` line (`server/server.go:427`) continues to
  accurately describe the server's (now permanently unconditional) behavior on both listeners.
  Per-connection protocol (HTTP/1.1 vs. HTTP/2) is already visible via the existing
  `otelhttp.NewHandler` wrapper's span attributes for both `Start()` (`server/server.go:1180`)
  and `StartRemote()` (`server/server.go:1383`) — no new instrumentation needed to observe the
  TLS listener's already-automatic HTTP/2 negotiation.
- **Metrics**: none new — this repo has no formal performance SLO for this change
  (requirements.md's Non-functional Requirements), and existing Connect-layer request
  metrics (if any) apply unchanged regardless of which transport served the request.
- **Alerts**: none — single-operator, personal-use tool; no on-call/paging infrastructure to
  wire into (requirements.md's Constraints).

## Risk Control

- **Feature flag**: a single frontend-only flag, `NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING`
  (Next.js public env, default unset/false). There is **no server-side flag** — the server
  makes zero behavior changes under this revised plan; `StreamingWSBridge` stays registered
  unconditionally on both listeners exactly as it does today. The frontend flag is further
  guarded at runtime: `createSessionWatchTransport` only returns the native
  `createConnectTransport` when **both** the flag is `"true"` **and** the resolved `baseUrl`
  starts with `https://` (i.e., the app was served through the TLS remote-access listener).
  Every other case — the plain `:8543`/`http://` listener, regardless of flag state — uses
  today's `createWatchTransport` unconditionally. This closes the adversarial review's Blocker 1
  regression risk (a long-lived `fetch()` silently occupying an HTTP/1.1 connection slot) by
  construction: the native path is literally unreachable on the plain listener.
- **Rollback procedure**: unset `NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING` and rebuild the
  frontend, or simply revert the frontend commit — there is no server-side state, routing, or
  protocol-negotiation change to roll back, since `server/server.go` is not modified by this
  plan at all. This is a smaller, more real rollback path than the original plan's, which had to
  roll back both a server flag and a frontend flag together.
- **Staged rollout**: (1) build the frontend with the flag enabled and deploy it to (or access
  it through) the operator's own TLS remote-access instance — the address the mobile app
  already connects to, `https://onyx.staplerhome.internal:8444` per project memory — since that
  is the only place this plan changes any behavior; (2) watch `WatchSessions`/`WatchReviewQueue`
  reconnect/error logs from that device for a trial period, confirming the pre-existing
  `StreamingWSBridge` fallback-forwarding path (already shipped, unmodified) correctly serves
  native-protocol requests over the listener's already-automatic ALPN HTTP/2; (3) once the trial
  is clean, flip the flag on by default for remote-access builds. There is no `StreamingWSBridge`
  deletion step at any point — see Follow-on/Deferred Work.

## Follow-on / Deferred Work

**WebTransport for terminal streaming — deferred, not implemented in this plan.** See
ADR-002 for the full rationale. Summary of trigger conditions for revisiting (all three must
hold):

1. `session/streamhub`'s dark-launch trial (`project_plans/terminal-multi-connection-streaming/implementation/plan.md`)
   has completed its rollback-rehearsal/trial-period gate. Any future WebTransport work must
   ship as a **new, independently-flagged `streamhub.Transport` implementation**
   (`WebTransportTransport`), never an in-place edit to `WebSocketTransport`/`MuxTransport`.
2. Either automated ≤14-day cert rotation + hash redistribution has been built for the
   `serverCertificateHashes` self-signed-cert path, or this product has explicitly relaxed its
   "no new external exposure" NFR in favor of a publicly-trusted CA certificate.
3. A concrete, *observed* perceived-latency or reconnection problem exists that WebTransport
   would fix (e.g. a filed bug about mobile network-switch reconnect visibility) —
   `research/ux.md` found no such problem exists today; a theoretical HOL-blocking argument is
   not sufficient on its own.

If revisited, the mandatory design constraint from `research/pitfalls.md` §3e applies: any
WebTransport adapter must use reliable streams, never unreliable datagrams, for PTY output,
and `session/streamhub/transport.go`'s `Transport` interface doc comment should be upgraded to
state its ordering/reliability contract explicitly before a new implementation is written
against it, paired with a regression test asserting a reordering/lossy transport is detected
and fails loudly rather than silently corrupting terminal output.

**CDP/VNC proxying is explicitly out of scope for changes.** `cdp_stream_handler.go` (215
lines) and `vnc_proxy_handler.go` (173 lines) are single-producer/single-consumer opaque byte
pipes with no multi-consumer fan-out requirement in the code today; none of Iggy, yetty, or
WebTransport address a gap they actually have, and every research doc independently reaches
"status quo" for this subsystem (`research/architecture.md` §1c/§5, `research/build-vs-buy.md`
summary table). No trigger condition is defined here because no evaluated technology maps onto
a need this subsystem has — a future review would need to identify a concrete new requirement
(e.g. multi-viewer CDP/VNC support) before re-opening this, which is a product decision, not a
transport-technology one.

**`StreamingWSBridge` is not deletable — corrected from the original plan (new).** The
original plan's Story 1.4.2 assumed `ws_stream_bridge.go` and its registration lines become
deletable once a trial period succeeds. That assumption depended on the (now-dropped)
`Server.Protocols.SetUnencryptedHTTP2` wiring on `:8543` eventually making the WS-upgrade bridge
unnecessary everywhere. Since no shipping browser implements cleartext HTTP/2, that path never
existed for real clients, so `StreamingWSBridge` remains the only mechanism that avoids a
long-lived stream holding one of a browser's ~6 HTTP/1.1 connections per origin on the plain
`:8543` listener — permanently, not transitionally. There is no future state, reachable by this
plan or its follow-ups, in which `ws_stream_bridge.go` is deleted.

**CI Go-version pin drift — observed, not blocking this plan (new).** `go.mod:3` requires
`go 1.26.3`; 7 of 8 checked GitHub Actions workflows
(`build.yml`, `lint.yml`, `e2e-video.yml`, `mcp-integration.yml`, `release-please.yml`,
`release.yml`, `registry-validation.yml`) pin `go-version: '1.25.0'` — only `benchmark.yml` uses
`go-version-file: 'go.mod'`. The adversarial review flagged this as load-bearing for the
*original* plan, because `Server.Protocols`/`SetUnencryptedHTTP2` are Go 1.24+ stdlib APIs that
don't exist on a 1.25 toolchain. This revised plan drops that API entirely (real ALPN HTTP/2 on
the TLS remote-access listener has been automatic since Go 1.6, confirmed via `go doc
net/http`), so the gap is **no longer load-bearing for this plan specifically** — nothing this
plan adds depends on the 1.26 toolchain being present in CI. The drift is still real,
pre-existing debt worth a separate follow-up chore (bump the pinned `go-version` in the affected
workflows to `1.26.3`, or switch them to `go-version-file: 'go.mod'` as `benchmark.yml` already
does) — not added as a task to this plan since nothing here requires it.

## Unresolved Questions

- [x] ~~Does the existing middleware chain ... correctly pass through unencrypted-HTTP/2
  connections without breaking streaming (e.g. `middleware.Compress`'s buffering behavior)~~ —
  **Resolved during this plan-repair pass.** `middleware.Compress` (`server/middleware/gzip.go:160-163`)
  skips **all** `/api/*` requests unconditionally, before any gzip/zstd wrapping happens — not
  merely "flushes per write" as the original plan's Unresolved Question assumed. ConnectRPC's
  own message-level framing is untouched by HTTP-level compression regardless of which protocol
  (HTTP/1.1, TLS-ALPN HTTP/2) served the request. No verification task is needed; this question
  is also moot on its own terms now, since this plan makes no `Server.Protocols` change on
  `:8543` at all.
- [ ] The `opt.baseUrl.startsWith("https://")` runtime guard in `createSessionWatchTransport`
  (Epic 1.3) assumes the only way a page is served over `https://` is via `Server.StartRemote`'s
  TLS listener negotiating real HTTP/2 directly. If a future deployment puts a TLS-terminating
  reverse proxy in front of the plain `:8543` listener, `window.location.protocol` would read
  `https:` even though the Go process itself still only speaks HTTP/1.1 behind the proxy,
  silently reintroducing the connection-slot regression this guard exists to prevent.
  **Correction (Phase 4 adversarial re-review + pre-mortem #1): this deployment pattern already
  exists in this repo** — `.claude/docs/slack-phase2-public-reachability.md` documents a
  TLS-proxy-in-front-of-`:8543` setup, currently scoped to a single path (Slack Phase 2
  interactive approvals), not yet covering `WatchSessions`/`WatchReviewQueue`. This is not a
  blocker for shipping this plan (the exposed paths don't overlap today), but is a real,
  documented precedent, not a hypothetical — implementer must add the guard's one-line comment
  *and* confirm the Slack Phase 2 proxy path doesn't (and isn't planned to) cover the two
  affected RPCs before this ships — owner: implementer, non-blocking but must be checked, not
  assumed away.
- [ ] Now that `StreamingWSBridge` is confirmed permanent rather than transitional (see
  Follow-on/Deferred Work), should its doc comment (`ws_stream_bridge.go:15-26`) be updated to
  say so explicitly, rather than implying — as the comment currently reads — that it's purely a
  workaround for a closeable gap? Low-priority doc-clarity item — owner: implementer,
  non-blocking.

## Dependency Visualization

```
1.1.1a (verification-only: integration test confirms the TLS remote-access
        listener already negotiates real ALPN HTTP/2 today; zero server code
        change — StartRemote is not modified)
   |
   +--> 1.1.1b (unit tests: StreamingWSBridge's forward + WS-upgrade dispatch
   |            branches — independent of 1.1.1a, can run in parallel)
   |
   +--> 1.1.1c (integration: real native-HTTP/2 client through StartRemote +
   |            the real bridge fallback path — depends on 1.1.1a's TLS
   |            fixture, independent of 1.1.1b)
   |
   v
1.3.1a (frontend: transport-selection helper, gated on NEXT_PUBLIC flag AND
        opt.baseUrl.startsWith("https://"))
   |
   v
1.3.1b (wire useShells / useReviewQueue / useSessionService to the helper)
   |
   v
1.3.1c (frontend unit tests for flag + https-guard branching, all 4 combinations)
   |
   v
1.3.1d (reconnect-parity test/fix: non-retriable errors must stop retrying
        over the native transport too — pre-mortem P1 #2)
   |
   v
1.4.1 (manual trial: enable the frontend flag, verify against the operator's
       actual TLS remote-access instance — the plain :8543 listener is
       provably unaffected by construction, no local-dev trial needed)
   |
   v
1.4.1b (bump CACHE_VERSION in the service worker as part of this deploy —
        pre-mortem P1 #5, so the PWA-installed mobile client actually
        receives the new bundle)
```

Epic 1.2 (the original plan's routing bypass, Tasks 1.2.1a-c) and Task 1.1.1b/1.1.1c's original
`Server.Protocols.SetUnencryptedHTTP2` wiring are **dropped entirely** — see the Architecture
Review BLOCKER and Adversarial Review BLOCKERs 1 and 2, both resolved by removing the mechanism
rather than patching it. Story 1.4.2 ("delete `StreamingWSBridge`") is likewise dropped — see
Follow-on/Deferred Work. (Note: Task 1.1.1b/1.1.1c above are new tasks added during Phase 4 —
distinct from, and unrelated to, the original plan's identically-numbered h2c-wiring tasks that
were dropped; the numbers were reused after the drop rather than skipped.)

---

## Phase 1: Adopt ConnectRPC Native Streaming for Server-Streaming RPCs (TLS remote-access listener only)

### Epic 1.1: Verify the TLS remote-access listener already provides real, browser-supported HTTP/2 — no server code change
**Goal**: Confirm — not build — that `Server.StartRemote`'s TLS listener (`:8444`) already
negotiates HTTP/2 via TLS ALPN automatically, per Go's stdlib default behavior, with zero
explicit `Protocols`/`SetUnencryptedHTTP2` wiring. This replaces the original Epic 1.1
(which added cleartext-HTTP/2 support to the plain `:8543` listener) — that mechanism is
rejected outright per the adversarial review's Blocker 1, since no shipping browser implements
h2c.

#### Story 1.1.1 (revised): As the stapler-squad server, I already accept real ALPN-negotiated HTTP/2 connections on the TLS remote-access listener with no code change, and I must not add cleartext HTTP/2 (h2c) to the plain `:8543` listener, since no shipping browser implements it.
**Acceptance Criteria**:
- `server/server.go`'s `StartRemote` (`:1378-1424`) constructs its `http.Server` (`remoteSrv`)
  without setting `Protocols` or `TLSNextProto`; per Go's own `net/http` package doc ("Server
  ... automatically enable[s] HTTP/2 support when using HTTPS," verified via `go doc net/http`),
  this means HTTP/2 is negotiated automatically via TLS ALPN for any client that offers it —
  including every real browser. This has been true since Go 1.6; it is not a Go 1.24+ feature
  and requires no `http2.ConfigureServer` call.
  - *Given* a real browser (or `curl --http2`) connects to `https://<remote-host>:8444`, *When*
    it makes a `fetch()`/HTTP request, *Then* the connection negotiates HTTP/2 via ALPN with no
    application-level configuration change required.
- No `Server.Protocols.SetUnencryptedHTTP2` call is added anywhere in this codebase — cleartext
  HTTP/2 (h2c) is explicitly **rejected as a mechanism**, not merely deferred, because no
  shipping browser implements it (Firefox Bugzilla #1418832, open; Chromium crbug
  #1409512/#580796, unimplemented) — verified via web search during the adversarial review,
  re-confirmed during this plan-repair pass.
  - *Given* the plain `:8543` listener (`Server.Start`, non-TLS), *When* any client attempts
    unencrypted-HTTP/2 (h2c) negotiation, *Then* it fails exactly as it does today — this plan
    makes no change to that listener's protocol negotiation at all.
**Files**: none (verification only; `server/server.go` is not modified by this epic)

##### Task 1.1.1a: Integration test confirming the TLS listener's existing automatic ALPN HTTP/2 (~5 min)
- In `server/server_integration_test.go`, add a test that starts a `Server`, calls `StartRemote`
  with a self-signed `tls.Config` (reuse this test file's existing TLS-test-fixture helper if
  one exists — grep first), dials the resulting address with `golang.org/x/net/http2`'s
  `http2.Transport` configured for real TLS negotiation (ALPN, **not** `AllowHTTP`/prior
  knowledge), hits `/health`, and asserts a 200 response with `resp.ProtoMajor == 2` — proving
  HTTP/2 is negotiated with zero application code beyond what `StartRemote` already does today.
  Label this test's doc comment explicitly as **confirming existing stdlib protocol-negotiation
  behavior**, not testing newly-built code — per the adversarial review's Minors note, to avoid
  implying this is "the browser-facing streaming benefit" being newly implemented (it is a
  pre-existing capability; only the frontend's use of it, in Epic 1.3, is new).
- Files: `server/server_integration_test.go`

##### Task 1.1.1b: Unit tests for `StreamingWSBridge`'s dispatch branches — zero coverage today (~6 min)
*(Added during Phase 4 validation/triad review — architecture-review Concern + adversarial-review
Concern 2, both flagged `ws_stream_bridge.go` has no existing test file despite being the
mechanism ADR-001's entire "zero server code change" claim rests on.)*
- In new file `server/services/ws_stream_bridge_test.go`, add
  `TestStreamingWSBridge_should_ForwardToWrappedHandler_When_RequestIsNotWebSocketUpgrade`: build
  a `StreamingWSBridge` wrapping a stub `http.Handler`, send a plain (non-upgrade)
  `httptest.NewRequest("POST", ...)` through `Handler("/api")`, assert the stub was invoked with
  the `/api` prefix stripped and its response passed through unchanged. This is the "native
  ConnectRPC client works with zero routing change" guarantee ADR-001 depends on.
- Add `TestStreamingWSBridge_should_UpgradeToWebSocket_When_RequestIsWebSocketUpgrade`: via
  `httptest.NewServer` + a real client dial (needed for `websocket.IsWebSocketUpgrade` to see a
  real hijackable connection), send a `Connection: Upgrade`/`Upgrade: websocket` request through
  `Handler("/api")`, assert `handleWebSocket` is taken (stub HTTP handler never invoked) — the
  regression guard proving the plain `:8543` WS-bridge path is unaffected by this plan.
- Files: `server/services/ws_stream_bridge_test.go` (new)

##### Task 1.1.1c: End-to-end native-HTTP/2 streaming test through the real bridge (~8 min)
*(Added during Phase 4 validation/triad review — closes the specific gap both the architecture
review and adversarial review named: no automated test exercises a real ConnectRPC-native client
against the real `StartRemote` TLS listener and the real `StreamingWSBridge` fallback path
together, only `/health` via Task 1.1.1a.)*
- In new file `server/services/watch_sessions_native_streaming_integration_test.go`, add
  `TestWatchSessions_should_DeliverMultipleEventsOverNativeHTTP2Stream_When_CalledThroughStartRemoteTLSListener`:
  start a real `Server` via `StartRemote` with a self-signed TLS cert (reuse `server/tls.go`'s
  `generateCA`/`generateServerCert`, same fixture as Task 1.1.1a), construct a real ConnectRPC Go
  client with an `http2.Transport` dialer (mirroring what `createConnectTransport` does
  in-browser), call `WatchSessions`, trigger 2+ session mutations server-side, and assert the
  client receives the corresponding stream events in order over native HTTP/2 — proving
  end-to-end delivery through the real bridge fallback branch, not just a single `/health` hit.
- Files: `server/services/watch_sessions_native_streaming_integration_test.go` (new)

---

### Epic 1.2: Dropped

The original Epic 1.2 ("Route `WatchSessions`/`WatchReviewQueue` to the native Connect handler
when the flag is on," Tasks 1.2.1a-c) is **removed entirely** per the architecture review's
BLOCKER. `StreamingWSBridge.Handler()` (`server/services/ws_stream_bridge.go:44-62`, verified
by direct read) already forwards every non-WebSocket-upgrade request to the same Connect
handler instance registered at the general subtree path — so once a client uses
`createConnectTransport` (a plain `fetch()` POST, never a WS-upgrade handshake), it is *already*
served natively by the existing, unconditional `StreamingWSBridge` registration
(`server/server.go:425-426`). No routing change of any kind is needed, on either listener.
`srv.mux.Handle(watchSessionsPath, wsBridge.Handler("/api"))` and the `watchReviewQueuePath`
equivalent stay exactly as they are today, unconditionally, forever — see Follow-on/Deferred
Work for why `StreamingWSBridge` is not a deletable transitional shim.

---

### Epic 1.3: Frontend transport selection for the three affected hooks, gated to the TLS remote-access listener
**Goal**: `useShells.ts`, `useReviewQueue.ts`, `useSessionService.ts` use ConnectRPC's native
`createConnectTransport` instead of the WS-bridge `createWatchTransport` only when (a) the
frontend flag is on, and (b) the resolved `baseUrl` is `https://` — i.e., only when actually
connected through the TLS remote-access listener where real ALPN HTTP/2 is in effect. Every
other case (the plain `:8543` dev listener, regardless of flag state) is unconditionally
unaffected.

#### Story 1.3.1 (revised): As a developer maintaining these three hooks, I want a single transport-selection helper that only activates native ConnectRPC streaming over a TLS connection, so the plain-HTTP dev listener can never regress into holding a browser connection slot for a long-lived stream.
**Acceptance Criteria**:
- A new exported function `createSessionWatchTransport(opt: ConnectTransportOptions): Transport`
  returns `createConnectTransport(opt)` only when **both**
  `process.env.NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING === "true"` **and**
  `opt.baseUrl.startsWith("https://")`; it returns `createWatchTransport(opt)` in every other
  case (flag off, or flag on but `baseUrl` is `http://`).
  - *Given* `NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING=true` at build time and `opt.baseUrl` is
    `"http://localhost:8543/api"` (the default local-dev/plain-listener case), *When*
    `createSessionWatchTransport(opt)` is called, *Then* the returned object is
    `createWatchTransport`'s result — unchanged from today, regardless of the flag.
  - *Given* `NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING=true` and `opt.baseUrl` is
    `"https://onyx.staplerhome.internal:8444/api"` (the TLS remote-access listener), *When*
    `createSessionWatchTransport(opt)` is called, *Then* the returned object is
    `@connectrpc/connect-web`'s stock `createConnectTransport(opt)` result.
  - *Given* the flag is unset (default), *When* `createSessionWatchTransport(opt)` is called
    with any `baseUrl`, *Then* the returned object is always `createWatchTransport`'s result.
- Implementer note: `opt.baseUrl` is derived at runtime from `getApiBaseUrl()`
  (`web-app/src/lib/config.ts:14-29`), which returns `window.location.origin + '/api'` in the
  browser — so the `https://` check is a genuine runtime signal for "which listener served this
  page," not a build-time assumption. This matters because the web UI's static assets are a
  **single build** served identically by both `Server.Start` (`:8543`) and `Server.StartRemote`
  (`:8444`) — `registerStaticRoutes` (`server/server.go:1033-1058`) registers the `distFS`
  handler once, on the shared `srv.mux` both listeners use. A build-time-only flag could not, on
  its own, distinguish which listener served a given page load; the `baseUrl` runtime check is
  what makes the gating correct despite the shared build.
- `watch-ws-transport.ts` itself is not modified beyond adding this new exported function (its
  existing `createWatchTransport`/`fromWebSocket`/`encodeEnvelope` are untouched, preserving the
  flag-off/plain-listener rollback path exactly).
**Files**: `web-app/src/lib/transport/watch-ws-transport.ts`

##### Task 1.3.1a: Add `createSessionWatchTransport` helper with the https-baseUrl guard (~5 min)
- In `web-app/src/lib/transport/watch-ws-transport.ts`, add (near the bottom, after the
  existing `createWatchTransport` function):
  ```ts
  export function createSessionWatchTransport(opt: ConnectTransportOptions): Transport {
    const nativeStreamingEnabled =
      process.env.NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING === "true";
    // Native ConnectRPC streaming only helps over a real HTTP/2 connection —
    // that's the TLS remote-access listener (:8444) only. No shipping browser
    // supports cleartext HTTP/2, so on the plain :8543 dev listener native
    // streaming would silently stay on HTTP/1.1 and hold a long-lived fetch()
    // open, reintroducing the exact browser connection-slot problem
    // StreamingWSBridge exists to avoid. Gate on baseUrl, not just the flag.
    if (nativeStreamingEnabled && opt.baseUrl.startsWith("https://")) {
      return createConnectTransport(opt);
    }
    return createWatchTransport(opt);
  }
  ```
  (`createConnectTransport` is already imported at the top of this file for `httpTransport`'s
  unary calls, so no new import is needed.)
- Files: `web-app/src/lib/transport/watch-ws-transport.ts`

##### Task 1.3.1b: Wire `useShells.ts`, `useReviewQueue.ts`, `useSessionService.ts` to the new helper (~5 min)
- In each of `web-app/src/lib/hooks/useShells.ts:9,61-63`,
  `web-app/src/lib/hooks/useReviewQueue.ts:6,134-140`, and
  `web-app/src/lib/hooks/useSessionService.ts:5,227-235`, change the import from
  `import { createWatchTransport } from "@/lib/transport/watch-ws-transport";` to
  `import { createSessionWatchTransport } from "@/lib/transport/watch-ws-transport";`, and
  replace the `createWatchTransport({...})` call site with
  `createSessionWatchTransport({...})` (same arguments, no other changes).
- Files: `web-app/src/lib/hooks/useShells.ts`, `web-app/src/lib/hooks/useReviewQueue.ts`, `web-app/src/lib/hooks/useSessionService.ts`

##### Task 1.3.1c: Unit tests for the new helper's flag + https-guard branching (~6 min)
- In `web-app/src/lib/transport/watch-ws-transport.test.ts`, add a test suite for
  `createSessionWatchTransport` covering all four combinations of {flag on/off} × {`baseUrl`
  http/https}, asserting native transport is selected only for {flag on, https}, and
  `createWatchTransport` (WS-bridge) for the other three combinations — the http-with-flag-on
  case is the one this plan-repair pass added specifically to prevent the adversarial review's
  Blocker 1 regression from being reintroduced silently in the future. Mock/spy on
  `@connectrpc/connect-web`'s `createConnectTransport` and on `WebSocket` construction using
  this test file's existing conventions (double-check ESM module-mock hoisting behavior per the
  architecture review's Nitpick before assuming the spy pattern works as described).
- Files: `web-app/src/lib/transport/watch-ws-transport.test.ts`

##### Task 1.3.1d: Reconnect-parity test — non-retriable errors must stop retrying over the native transport too (~6 min)
*(Added during Phase 4 validation — pre-mortem P1 #2.)* `useSessionService.ts`'s reconnect state
machine (`startStream`, `useSessionService.ts:1004-1019`) decides whether to stop retrying by
reading a `ws-close-code` header that only `fromWebSocket()` (`watch-ws-transport.ts:53`) ever
sets. `createConnectTransport` errors never carry that header, so `getWsCloseCode(err)` is always
`null` on the native path and the "non-retriable close code, stop reconnecting" branch never
fires — every failure over the native transport, including ones that should stop retrying, falls
through to indefinite exponential backoff. This is silent on the mobile app's TLS-listener path,
the one real consumer of this change.
- In `web-app/src/lib/hooks/useSessionService.test.ts` (or the appropriate hook's existing test
  file), add a test that simulates a non-retriable server error (e.g. an auth/protocol failure)
  arriving over `createConnectTransport`, and assert the reconnect state machine stops retrying —
  not just that the correct transport constructor was called (Task 1.3.1c only spies on
  selection, not on downstream reconnect behavior).
- Fix `getWsCloseCode`/`isRetriableCloseCode` (or the call site in `useSessionService.ts`) so
  non-retriable-error detection has parity across both transports — e.g. inspect the
  `ConnectError` code/message shape `createConnectTransport` actually throws for a
  non-retriable failure, and treat that equivalently to today's `ws-close-code`-based check,
  rather than only ever recognizing the WS-bridge's header.
- Files: `web-app/src/lib/hooks/useSessionService.ts`, `web-app/src/lib/hooks/useSessionService.test.ts`

---

### Epic 1.4: Trial (gated, not auto-executed)
**Goal**: Validate the flag-on path against the operator's actual TLS remote-access instance
for a trial period. There is no meaningful local-`:8543` trial to run — that listener is
provably unaffected by this plan (Epic 1.1 makes no server-side change, Epic 1.2 is dropped,
and Epic 1.3's guard makes the plain listener flag-inert).

#### Story 1.4.1 (revised): As the operator, I want to run the frontend flag enabled against my real TLS remote-access instance for a trial period and confirm `WatchSessions`/`WatchReviewQueue` behave identically to today before flipping the flag on by default.
**Acceptance Criteria**:
- `NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING=true` is set in a frontend build accessed through the
  operator's actual TLS remote-access listener — per project memory,
  `https://onyx.staplerhome.internal:8444` — and session list / review queue updates are
  observed to arrive live with no visible regression over a trial period.
  - *Given* the flag is enabled and the operator is connected through the TLS remote-access
    listener, *When* a session is created/updated/deleted, *Then* the session list view updates
    live with no increase in perceived latency or missed events compared to the flag-off
    baseline observed through the same listener beforehand.
- No new error-level log lines referencing `WatchSessions`/`WatchReviewQueue` appear during the
  trial that don't also appear in the flag-off baseline.
- The plain `:8543` listener is **not** part of this trial — its behavior is unchanged by
  construction (Epic 1.3's `https://` guard), so there is nothing new to validate there.
**Files**: none (manual verification task, not a code change)

##### Task 1.4.1a: Run manual trial against the TLS remote-access listener (~5 min to set up, trial duration operator-determined)
- Build the frontend with `NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING=true` and access it through
  the operator's own TLS remote-access URL. Note: a manual `~/.stapler-squad/manual-builds/`
  instance from CLAUDE.md's port block (e.g. `PORT=62871`) is a **plain HTTP** listener and, per
  Epic 1.3's guard, will always exercise the flag-off code path regardless of the flag — it
  cannot validate this specific change. The trial must go through an actual TLS-terminated
  listener (`--remote-port`/`StartRemote`, or the operator's deployed remote-access instance).
  Click around the session list / review queue views for a trial period. Record any discrepancy
  against the flag-off baseline.
- Files: none

##### Task 1.4.1b: Bump `CACHE_VERSION` in the service worker as part of this deploy (~2 min)
*(Added during Phase 4 validation — pre-mortem P1 #5.)* `web-app/public/cache-sw.js` is a
cache-first service worker for all GET requests, and the mobile app — the one real target of
this change — is installed as a PWA (`manifest.json`). Without a `CACHE_VERSION` bump, an
installed PWA client keeps serving the pre-change JS bundle indefinitely after deploy, so the
flag could be flipped on and rebuilt with zero observable effect on the actual mobile consumer.
- Bump `CACHE_VERSION` in `web-app/public/cache-sw.js` as part of the same deploy that ships
  Task 1.3.1a-d's transport-selection code, so the PWA-installed mobile client picks up the new
  bundle instead of serving a stale cached one. Apply this same requirement to any *future*
  change to transport-selection code, not just this initial rollout.
- Files: `web-app/public/cache-sw.js`

#### Story 1.4.2 (revised): `StreamingWSBridge` is not scheduled for deletion — correcting the original plan's premise.
**Acceptance Criteria**:
- Unlike the original plan (which assumed `StreamingWSBridge` becomes deletable once the trial
  succeeds), this plan-repair pass establishes that `StreamingWSBridge` remains **permanently
  required** for the plain `:8543` listener: it is the only mechanism that avoids a long-lived
  streaming request holding one of a browser's ~6 concurrent HTTP/1.1 connections per origin on
  that listener, since no shipping browser will ever negotiate HTTP/2 there (h2c is unsupported
  browser-wide, not a temporary gap). There is no future state in which `ws_stream_bridge.go` is
  deleted as a consequence of this plan.
- Once the Story 1.4.1 trial succeeds, the only follow-up is a small config default flip
  (`NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING` defaults to `true` in the operator's own
  remote-access build config) — no code deletion, and no `server/server.go` change (there was
  never one to begin with in this revised plan).
**Files**: none — this story documents a corrected understanding, not a code task

*(No sub-tasks — this story exists to explicitly close out the original Task 1.4.2's now-false
premise, so a future reader doesn't rediscover the correction by re-deriving it.)*
