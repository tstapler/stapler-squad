# Stack research: frontend-responsiveness

Labels: VERIFIED = opened source / ran grep in this worktree; INFERRED = not proven here.
Paths are relative to the repo root.

## Versions (VERIFIED: web-app/package.json, node_modules/*/package.json)
- @connectrpc/connect 2.1.1, @connectrpc/connect-web 2.1.1, @bufbuild/protobuf ^2.11.0
- react 19.1.1 (installed; range ^19.0.0 at package.json:112), next 15.3.2 (package.json:111)
- @xterm/xterm 6.0.0, addon-fit 0.11.0, addon-webgl 0.19.0, addon-canvas ^0.7.0

## Transports
- VERIFIED: a shared singleton already exists. `web-app/src/lib/api/transport.ts` has `getConnectTransport()` (default 30 s timeout + auth interceptor), `getKeepaliveConnectTransport()`, `getWatchTransport()`. 81 non-test files already use `getConnectTransport` (grep count).
- VERIFIED: 37 files still call `createConnectTransport` themselves (grep -l count). They include `app/backlog/page.tsx:359`, `app/unfinished/UnfinishedTab.tsx:68`, `components/backlog/BacklogItemDetail.tsx:430,719`, `BacklogItemPanel.tsx:115`, settings/*, `components/unfinished/*`. The requirement's "each page builds its own transport" is partly stale: the work is migrating these 37 to the singleton, not creating one.
- Transport count is not itself the cause of queuing: browser connection limits are per-origin, not per-transport (INFERRED from the HTTP/1.1 6-connection rule; HAR shows HTTP/1.1).
- VERIFIED: connect-web 2.1.1 `createConnectTransport` options include `fetch`, `interceptors`, `useHttpGet`, `defaultTimeoutMs` (node_modules/@connectrpc/connect-web/dist/esm/connect-transport.d.ts). It has NO built-in batching or dedupe (grep "batch" in dist/esm/*.d.ts returned nothing). Dedupe/coalescing must be an app-level interceptor or a `fetch` wrapper.
- INFERRED: a dedupe interceptor works for idempotent unary reads (GetFeatureFlags, ListProgramsConfig) keyed by method + serialized request; `useHttpGet` (GET for `idempotency_level = NO_SIDE_EFFECTS` methods) would make them browser-cacheable but needs proto annotations (not checked).
- Streaming over WebSocket: `web-app/src/lib/transport/watch-ws-transport.ts:92` `createWatchTransport` delegates `unary` to the plain HTTP transport (`:95-97`) and only rewrites `stream()` onto a WebSocket via `runStreamingCall`; server side is `server/services/connectrpc_websocket.go` (3862 lines, `HandleWebSocket` at :861, `negotiateConnectStream` :896). Unary-over-WS would need a new `unary` implementation in this adapter plus a server bridge; the Connect envelope framing helper `encodeEnvelope` (:24) is reusable. VERIFIED by reading; effort is INFERRED.

## HTTP/2
- VERIFIED: the main :8543 listener is plain HTTP; `server/server.go:125` builds `http.Server` with no Protocols/TLSNextProto; `server_integration_test.go:182-190` asserts `SetUnencryptedHTTP2` is never used (no browser speaks h2c) and that h2c prior-knowledge falls back/fails.
- VERIFIED (test exists, not run by me): the remote server (`StartRemote`, server.go:1979, TLS via `tlsCfg`) negotiates ALPN h2 (`TestServer_should_NegotiateALPNHTTP2_When_StartRemoteServesOverRealTLS`, server_integration_test.go:66). So remote-access (:8444) users already get h2 multiplexing; localhost :8543 is where the 6-slot limit bites. Open question: which origin was the HAR captured on (HAR is all `localhost:8543`, HTTP/1.1 per requirements).
- Setting `s.tlsConfig` (SetupTLS, server.go:1481) makes :8543 serve TLS (server.go:1620-1625), which would enable h2 through Go's automatic ALPN for `ListenAndServeTLS` (INFERRED from Go stdlib behavior). Cost is cert trust on localhost.

## Duplicate-call sources (VERIFIED by reading)
- ListProgramsConfig: `web-app/src/lib/hooks/useAvailablePrograms.ts:7-30` has NO module cache or in-flight dedupe; each mount fires one RPC. Mounted by `WorkflowForm.tsx:71`, `OmnibarCreationPanel.tsx:328`, `SessionDetailView.tsx:510`, `SessionActionsOverflow.tsx:211` -> one call per mounted instance (per session/pane/row), and again on every remount from window switching. Fix pattern: copy `useFeatureFlagValue.ts:15-40` (30 s TTL cache + shared in-flight promise).
- GetFeatureFlags: two independent consumers: `lib/contexts/FeatureFlagsContext.tsx:105` (no shared cache seen) and `lib/hooks/useFeatureFlagValue.ts:27-30` (TTL cache + in-flight dedupe, its own cache, not shared with the context). 14 simultaneous calls in the HAR means more callers or cache misses than these two; remaining callers not enumerated.
- LogClientEvents: `lib/hooks/useBrowserLogStream.ts` already batches (MAX_BUFFER 50, FLUSH_INTERVAL_MS 5 s, lines 33-35) but is instantiated per terminal: `components/sessions/TerminalOutput.tsx:965` `useBrowserLogStream({enabled, sessionId, baseUrl})`. N terminals x one 5 s timer each (timer at :213) = 56 calls/16 s plausible for ~3-4 terminals remounting. Fix: single module-level buffer/flusher.

## React (installed 19.1.1)
- VERIFIED: `<Activity>` is NOT exported in 19.1.1 (react.development.js has only an internal REACT_ACTIVITY_TYPE name case at :106; no `exports.Activity`). Activity became stable in React 19.2 (INFERRED from release knowledge; verify before relying). Upgrading within ^19.0.0 is possible but must be verified against Next 15.3.2 peer range (not checked).
- VERIFIED: `startTransition` (react.development.js:1119) and `useDeferredValue` (:1174) are available. Only 4 grep hits in web-app/src (non-gen), so they are barely used.
- Without Activity, keep-alive = keep windows mounted and hide with CSS (`display:none` / `content-visibility:hidden`) plus an `isActive`/visible prop that gates effects. Cost: hidden xterm measures 0x0 under display:none (already flagged in requirements Rabbit Holes); `content-visibility:auto` or `visibility:hidden` + absolute positioning keeps layout size. INFERRED.
- `reactCompiler` is not enabled (next.config.ts:21 `experimental` only has `optimizePackageImports`).
- Dual build: Next and Bun builds coexist (`nav-facade.md`, `next.config.ts` webpack alias), so any lazy/keep-alive seam must work in both.

## xterm.js / hidden terminals
- VERIFIED: `XtermTerminal.tsx` already loads WebGL lazily (`import('@xterm/addon-webgl')` at :772), falls back to `CanvasAddon` (:21,669), and has a context budget: `lib/terminal/webglBudget.ts` `MAX_WEBGL_CONTEXTS = 8`, evicts hidden slots first (`claimWebglSlot`, `touchWebglSlot`; the 16-context browser limit comment is explicitly "not verified"). So keep-alive of several windows is bounded by 8 WebGL terminals, extras fall to canvas.
- VERIFIED: getBoundingClientRect hot-spot call sites: `components/sessions/useTerminalSizing.ts:268`, `XtermTerminal.tsx:130`, `components/pane/PaneTilingContainer.tsx:84`, `components/pane/ResizeHandle.tsx:38`, `components/sessions/SessionList.tsx:689` (react-virtual `measureElement`, one forced read per row). Resize is driven by ResizeObserver in `TerminalOutput.tsx:149` and `useTerminalSizing.ts` with a zero-size guard (:264). ResizeObserver callbacks deliver size without forcing layout (`entry.contentRect`/`borderBoxSize`); replacing the rect reads with the observer entry is the standard fix (INFERRED applicability).
- addon-fit 0.11.0 `fit()` reads computed style + the container size each call (INFERRED, not opened); debounce/skip when hidden. Hidden terminals should skip `fit()` and rAF work (xterm renders via its own rAF render service; `terminal.options`/`pause` not found, INFERRED: only dispose or skipping writes stops render cost).
- `components/sessions/useVisibilityResync.ts:266` already handles `document.visibilityState` for tab-level hide, but not for window/pane hide.

## Recommendations (stack-level)
1. Dedupe at the source first: module-level caches for `useAvailablePrograms` and `FeatureFlagsContext`, a single shared log flusher, and migrate the 37 `createConnectTransport` sites to `getConnectTransport()`. All are cheap and independent of the transport decision.
2. Add an app-level dedupe/coalescing interceptor to `getConnectTransport()` (connect-web has no built-in).
3. Unary-over-WS is the largest option; defer until after (1)-(2) are measured with a fresh HAR. h2 already works on remote-access TLS.
4. Keep-alive windows: need React 19.2 `<Activity>` or CSS-hide with gated effects; bound by `MAX_WEBGL_CONTEXTS`.
