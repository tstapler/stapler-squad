# Validation Plan: web-transport-architecture-review

**Date**: 2026-08-22

## Happy Path Scenario

Given the operator's frontend is built with `NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING=true` and
served through `Server.StartRemote`'s TLS remote-access listener (`https://onyx.staplerhome.internal:8444`),
when a browser tab calls `useSessionService`'s `WatchSessions` subscription, then
`createSessionWatchTransport` selects `createConnectTransport` (native ConnectRPC HTTP/2
streaming), the request lands on `StreamingWSBridge.Handler()`'s non-WebSocket-upgrade
fallback-forwarding branch (`ws_stream_bridge.go:50-60`), and session-list updates arrive live
over that stream with no observable regression versus the WS-bridge baseline — while the same
build served through the plain `:8543` listener continues to use `createWatchTransport`
(WS-bridge) unconditionally, regardless of the flag.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| Story 1.1.1 — TLS remote-access listener already negotiates real ALPN HTTP/2, zero server code change | `server/server_integration_test.go` | `TestServer_should_NegotiateALPNHTTP2_When_StartRemoteServesOverRealTLS` | Integration | Happy path: start a `Server`, call `StartRemote` with a self-signed `tls.Config` (reuse `server/tls.go`'s `generateCA`/`generateServerCert` helpers, or an equivalent local test-only cert), dial with `golang.org/x/net/http2`'s `http2.Transport` configured for real ALPN (not `AllowHTTP`/prior-knowledge), hit `/health`, assert `resp.ProtoMajor == 2`. Doc comment must explicitly state this confirms pre-existing stdlib behavior, not newly-built code (architecture-review's Minor). |
| Story 1.1.1 — plain `:8543` listener never negotiates h2c | `server/server_integration_test.go` | `TestServer_should_RejectHTTP2PriorKnowledge_When_StartServesOverPlainHTTP` | Integration (error path) | Start a `Server` via `Start` (no TLS), attempt an `http2.Transport` connection configured with `AllowHTTP: true` + prior-knowledge dialing (the h2c handshake shape) at the plain listener's address, assert the connection either fails or silently falls back to HTTP/1.1 (`resp.ProtoMajor == 1`) — proving no `Protocols`/`TLSNextProto` wiring was introduced that would let h2c succeed. Grep assertion companion: `grep -c SetUnencryptedHTTP2 server/server.go` (via `t.Run` subtest or a build-time `sg`-style check documented in the test comment) confirms zero occurrences, so a future edit reintroducing h2c wiring fails this test rather than silently landing. |
| Adversarial-review Concern 2 / architecture-review Concern — `StreamingWSBridge`'s fallback-forwarding path has zero prior coverage and is the actual mechanism ADR-001 depends on | `server/services/ws_stream_bridge_test.go` (new file — none exists today) | `TestStreamingWSBridge_should_ForwardToWrappedHandler_When_RequestIsNotWebSocketUpgrade` | Unit (happy path) | Build a `StreamingWSBridge` wrapping a stub `http.Handler` that records the request it received and writes a fixed response body; send a plain (non-upgrade) `httptest.NewRequest("POST", ...)` through `Handler("/api")`; assert the stub handler was invoked with the `/api` prefix stripped from the URL path, and the response body matches the stub's output. This is the bridge's core "native ConnectRPC client works with zero routing change" guarantee — currently untested per both reviews. |
| Adversarial-review Concern 2 — WS-upgrade path still works (regression guard for the plain listener) | `server/services/ws_stream_bridge_test.go` | `TestStreamingWSBridge_should_UpgradeToWebSocket_When_RequestIsWebSocketUpgrade` | Unit (error/alternate path) | Send a request with `Connection: Upgrade`/`Upgrade: websocket` headers (via `httptest.NewServer` + a real client dial, since `websocket.IsWebSocketUpgrade` needs a real hijackable connection) through `Handler("/api")`; assert `handleWebSocket` is taken (e.g. the stub HTTP handler is never invoked) — proving the dispatch `if websocket.IsWebSocketUpgrade(r)` branch (`ws_stream_bridge.go:46`) still routes WS clients correctly, so this plan's zero-server-change claim doesn't silently regress the primary `:8543` WS-bridge path while adding coverage for the native path. |
| Architecture-review Concern — end-to-end native-transport call through the full chain (the specific gap both reviews flagged) | `server/services/watch_sessions_native_streaming_integration_test.go` (new file) | `TestWatchSessions_should_DeliverMultipleEventsOverNativeHTTP2Stream_When_CalledThroughStartRemoteTLSListener` | Integration | Start a real `Server` via `StartRemote` with a self-signed TLS cert (same fixture as the Story 1.1.1 test), construct a real ConnectRPC client using `connectrpc.com/connect`'s Go client with an `http2.Transport` dialer (mirroring what `createConnectTransport` does in-browser), call `WatchSessions`, trigger 2+ session mutations server-side, and assert the client receives the corresponding stream events in order over the native HTTP/2 transport — not the WS-bridge path. This is the test both the architecture review ("give `ws_stream_bridge.go` its first-ever automated test... asserts multiple incrementally-flushed messages arrive") and the adversarial review (no automated e2e coverage of the native-HTTP/2 path) explicitly recommend adding; it closes the gap without requiring the Playwright/`tests/e2e` harness extension the adversarial review found infeasible (no TLS/`StartRemote` support in `tests/e2e/helpers/test-server.ts` today). |
| Story 1.3.1 — `createSessionWatchTransport` selects native transport only when flag on AND baseUrl is https | `web-app/src/lib/transport/watch-ws-transport.test.ts` | `createSessionWatchTransport_should_ReturnNativeConnectTransport_When_FlagOnAndBaseUrlIsHttps` | Unit (happy path) | `NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING="true"`, `opt.baseUrl = "https://onyx.staplerhome.internal:8444/api"` → spy on `@connectrpc/connect-web`'s `createConnectTransport` (mocked via `jest.mock`, module-factory hoisting confirmed per architecture-review's Nitpick) and assert it — not `createWatchTransport`/`WebSocket` — is invoked. |
| Story 1.3.1 — plain-listener guard: native transport never selected over `http://`, even with the flag on | `web-app/src/lib/transport/watch-ws-transport.test.ts` | `createSessionWatchTransport_should_ReturnWsBridgeTransport_When_FlagOnButBaseUrlIsHttp` | Unit (error/regression path — the specific case the plan-repair pass added to prevent Blocker 1 from silently reappearing) | `NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING="true"`, `opt.baseUrl = "http://localhost:8543/api"` → assert `createConnectTransport` is **not** called and the returned transport behaves like `createWatchTransport`'s (e.g. its `.stream` implementation opens a `WebSocket`, spied/mocked). This is the regression guard both reviews singled out. |
| Story 1.3.1 — flag off, any baseUrl → always WS-bridge | `web-app/src/lib/transport/watch-ws-transport.test.ts` | `createSessionWatchTransport_should_ReturnWsBridgeTransport_When_FlagUnset` (parameterized/`it.each` over `["https://...", "http://..."]`) | Unit | Flag unset (default) with both an `https://` and an `http://` `baseUrl` → both resolve to `createWatchTransport`'s behavior, completing all 4 combinations from Task 1.3.1c's acceptance criteria (flag×scheme). |
| Story 1.3.1 — helper is a pure pass-through of `opt` to whichever constructor is chosen | `web-app/src/lib/transport/watch-ws-transport.test.ts` | `createSessionWatchTransport_should_ForwardOptionsUnchanged_When_SelectingEitherTransport` | Unit | Call with a representative `ConnectTransportOptions` (interceptors, `useBinaryFormat`, `jsonOptions`) for both the native and WS-bridge branches; assert the same `opt` object reaches the selected constructor unmodified — guards against a future edit silently dropping e.g. auth interceptors on one branch only. |
| Task 1.3.1b — the three hooks are wired to the new helper, not the old one directly | `web-app/src/lib/hooks/useSessionService.test.ts` (existing — extend); `web-app/src/lib/hooks/useShells.test.ts`, `useReviewQueue.test.ts` (new — neither file exists today, verified via directory listing) | `useSessionService_should_CallCreateSessionWatchTransport_When_ConnectingToWatchSessions` (and equivalents for the other two hooks) | Unit | Mock `@/lib/transport/watch-ws-transport`'s module; assert each hook's connect path calls `createSessionWatchTransport`, not `createWatchTransport` directly — a plain grep-for-import check is not durable against a future refactor, this test is. For `useSessionService.test.ts`, follow its existing top-of-file `jest.mock` conventions (check before writing, don't invent a new pattern). `useShells.test.ts`/`useReviewQueue.test.ts` are net-new files; scope them narrowly to the transport-selection call site (not full hook-behavior coverage, which is out of scope for this plan) unless the implementer judges broader coverage is cheap to add alongside. |

## UX Acceptance Tests

N/A — no user-facing surface, pure infrastructure/transport change.

## Test Stack

- **Unit (Go)**: standard `testing` + `testify` (`assert`/`require`), following this repo's
  `TestXxx_should_YYY_When_ZZZ` naming convention (see `server/services/backlog_service_events_test.go`,
  `server/services/claude_settings_watcher_test.go` for precedent). `ws_stream_bridge_test.go`
  uses `httptest.NewRecorder`/`httptest.NewServer` — no real network for the non-WS-upgrade case,
  a real `httptest.NewServer` + `gorilla/websocket` client dial for the WS-upgrade case (needed
  because `websocket.IsWebSocketUpgrade` inspects real hijacking-capable connection state).
- **Integration (Go)**: `server/server_integration_test.go` and the new
  `watch_sessions_native_streaming_integration_test.go` follow the existing pattern in that file —
  real `net.Listen`, real `Server.Start`/`StartRemote`, real HTTP client dials against the bound
  port (see `findFreePort`, `TestServer_should_ServeHealthCheck_When_StartedWithPortZero`).
  TLS fixtures reuse `server/tls.go`'s `generateCA`/`generateServerCert` (already unexported in
  the `server` package, directly callable from `server_integration_test.go` and any new
  `_test.go` file in the same package) rather than hand-rolling a second self-signed-cert helper.
  HTTP/2 client dialing uses `golang.org/x/net/http2`'s `http2.Transport` (already an indirect
  dependency via `connectrpc.com/connect`/stdlib — confirm with `go list -m golang.org/x/net`
  before assuming no `go.mod` change is needed).
- **Unit (TypeScript/Jest)**: `web-app/src/lib/transport/watch-ws-transport.test.ts` extends the
  existing suite (see current `fromWebSocket` tests for the file's mocking conventions — plain
  `jest.fn()`-based mock objects, no test-double library). `jest.mock("@connectrpc/connect-web")`
  spies on `createConnectTransport`; `global.WebSocket` is stubbed the same way the existing
  `makeMockWS()` helper does, reused/extended for the new suite rather than duplicated.
- **E2E / UX**: N/A for this plan. Both reviews independently confirmed
  `tests/e2e/helpers/test-server.ts` has no `StartRemote`/TLS-listener support today, so a
  genuine browser-driven Playwright test of the native-transport path is infrastructure this
  plan does not build (out of scope — see plan.md's Follow-on/Deferred Work and the adversarial
  review's second Concern). The Go integration test above
  (`TestWatchSessions_should_DeliverMultipleEventsOverNativeHTTP2Stream_When_CalledThroughStartRemoteTLSListener`)
  is the closest automated substitute: it exercises a real ConnectRPC-native HTTP/2 client
  against a real `StartRemote` TLS listener and the real `StreamingWSBridge`, just not through an
  actual browser tab. Epic 1.4's manual, operator-duration trial against
  `https://onyx.staplerhome.internal:8444` remains the only check that covers an actual browser
  (per project memory of that address) — this is a deliberate, documented interim position, not
  an oversight.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |
| TypeScript/Jest | `npx jest --coverage --coverageThreshold='{"global":{"lines":80}}'` | ≥80% line |

Package-specific run for this plan's changed surface (faster than the full suite while iterating):

```bash
go test ./server/... -run 'TestServer_should_NegotiateALPNHTTP2|TestServer_should_RejectHTTP2PriorKnowledge|TestStreamingWSBridge|TestWatchSessions_should_DeliverMultipleEvents' -v
cd web-app && npx jest --testPathPatterns="watch-ws-transport|useShells|useReviewQueue|useSessionService" --no-coverage
```
