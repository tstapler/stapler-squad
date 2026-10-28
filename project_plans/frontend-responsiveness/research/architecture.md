# Architecture research: request path and window lifecycle

Labels: VERIFIED = command run or file read this session. INFERRED = reasoned, not proven.
Analysis script: `/tmp/ssqan/arch_conc.py` (reads `~/Downloads/stapler-squad-backend.har`).

## (a) Request path

### What the HAR proves about the 30 s stall

- VERIFIED: 234 entries, one origin (`localhost:8543`), HTTP/1.1. 161 entries are status 0 with `blocked` ~30,003-30,067 ms and every other timing 0 (never got a socket). 20 entries are status 101 (WebSocket), which carry no `_connectionId`. The 161 stalled entries all start between t=6.06 s and t=12.10 s.
- VERIFIED: the stalled set is dominated by `LogClientEvents` (56), `ListProgramsConfig` (53), `GetVCSStatus` (12), `ListFiles` (11), `StatFile` (8). Initiator is `script` for all, priority `High`.
- VERIFIED: unary (non-101) requests that finished used only 7 distinct `_connectionId` values in 42 s. Per-second count of unary requests past their `blocked` phase is exactly 6 at t=3-5 and t=7-10. So the 6-connection HTTP/1.1 per-origin cap is reached and enforced.
- VERIFIED (contradicts the simple story): from t=12 to t=30 only 3 unary requests are active while 160+ sit queued, and at t=31-35 zero are active while 162 remain queued. Slow completed calls (ListShells 4-6 s, GetVCSStatus 3-4.5 s) therefore do NOT explain the whole stall; something else holds the other slots. Queued requests only drain at ~t=36 when the browser's 30 s timeout fires.
- INFERRED (not provable from a HAR): the slots are held by fetches that never completed during the capture, so they have no HAR entry with a connection ID. Candidates: a fetch-based long poll or server-stream, or slow calls whose entries were never finalized. Alternative candidate: Chrome counting in-progress WebSocket handshakes against the same per-host cap. 11-16 WebSockets were open during the stall. Neither is confirmed.
- Not proven: that raising the cap (h2) fixes it, because the holder is unidentified. It would fix it if the holders are plain h1 fetches.

Cheapest confirming experiment (1 cost unit, no code): chrome://net-export or DevTools "Network > Connection ID" plus `chrome://net-internals/#sockets` during a stall to see who owns the 6 sockets. Add a dev-only in-flight gauge (already in requirements) to the shared transport.

### Source of fan-out

- VERIFIED: 14 identical `GetFeatureFlags` at t=2.8 s (per-consumer fetches). 37 `createConnectTransport` import sites (per requirements; `lib/api/transport.ts` already has a `_transport` singleton at line 34 but not every caller uses it).
- VERIFIED: unary traffic goes over plain `fetch`. `createWatchTransport` (`web-app/src/lib/transport/watch-ws-transport.ts:33`) binds `unary` to `httpTransport.unary` and only streams use `new WebSocket` (line 133).

### Server config (all VERIFIED, `server/server.go`)

- Local server (`newServerBase`, lines 125-131): `ReadTimeout 15s`, `WriteTimeout 0`, `IdleTimeout 60s`. Plain HTTP, no `x/net/http2` or `h2c` import found (`grep` of go.mod and server for `h2c`, `NextProtos` returned nothing). Plain `http.Server` over cleartext is HTTP/1.1 only.
- Local TLS mode (`SetupTLS` line 1484, `ListenAndServeTLS` line 1625): Go's `http.Server` enables h2 automatically for TLS when `TLSNextProto` is nil. So if the local server is ever run with TLS, h2 comes for free for unary calls.
- Remote server (`StartRemote`, lines 1970-2010): `http.Server` + `ServeTLS(ln, "", "")`, same timeouts. `ReadTimeout 15s` also applies to the h2 request-header read; fine for unary. So remote-access mode (`--remote-port`, HTTPS) can already negotiate h2. INFERRED: it does, since nothing disables it and `tlsCfg` sets no `NextProtos` (grep empty). Verify with `curl -vk --http2 https://host:8444/health` (not run; no remote instance available).
- WebSocket over h2: browsers open WebSockets over an h1 upgrade by default, so the 20 WS streams still use separate TCP connections in an h2 deployment; they stop competing with unary calls for the h1 per-origin cap only if they're in a separate pool (see inference above).
- Browser h2 needs TLS; `localhost:8543` plain HTTP can never use h2. Local-dev TLS means a trusted localhost cert (mkcert-style) and `https://` in every bookmark and the CORS/HostGuard origin lists. Cost is operational (owner decision), not code.

### Options compared

| Option | Fixes cap? | Cost | Risk | Verdict |
|---|---|---|---|---|
| A. Dedupe/coalesce + single shared transport + priority queue | Yes, by never reaching 6 (HAR: 161 of 234 requests are duplicates/chatter) | Low-medium, all in web-app | Low; each change is local | Do first. It is also the only option that still helps if the unknown slot holder is a non-fetch thing |
| B. Unary-over-WebSocket mux | Yes (one socket) | High: a new server bridge. Existing `StreamingWSBridge` (`server/services/ws_stream_bridge.go`) is per-path and streaming-only (registered per watch path, `server.go:521-593`); needs generic unary framing, correlation IDs, HostGuard/auth parity, reconnect/backpressure, a custom Connect `Transport.unary`, and a Playwright mock rework | Medium-high; reimplements what h2 gives for free; head-of-line blocking on one socket if a slow unary (ListShells 6 s) is not on its own stream | Defer; revisit only if A + h2-on-remote does not meet SLO |
| C. HTTP/2 via TLS | Yes (100+ streams/conn) | Low code, medium ops (trusted localhost cert, HTTPS everywhere) | Local-only users must trust a cert; h2 does not reduce server work | Recommend as opt-in for local, already effectively available for remote |
| D. Shared fetch transport with dedupe/batching/priority | Same as A | Medium; interceptor in `lib/api/transport.ts` | Low | This is the implementation form of A |

Recommendation: A/D now (collapse `GetFeatureFlags` to one cached promise, TTL-cache `ListProgramsConfig`, buffer `LogClientEvents` into one flush per N seconds, cap non-critical concurrency to 3-4 so two slots stay free for interactive calls, abort superseded calls on window switch), plus a dev-only in-flight gauge. Keep C as an owner-optional flag, and drop B unless A fails the success metrics. The owner expected B; the evidence says B is the most expensive route to a cap that A removes and C removes more cheaply.

Open: which component owns each of the 56 `LogClientEvents` and 53 `ListProgramsConfig` calls (Agent for call-site research). Not answered here.

## (b) Window and route lifecycle

### Current structure (all VERIFIED by reading)

- `web-app/src/app/page.tsx:162-163`: `useWindowUrlSync(windows, isWindowsRestored)` derives `currentWindowId` from `?window=`; `currentWindow` is passed as `paneState={currentWindow.paneState}` to one `<PaneTilingContainer>` (line 633). Switch = `router.replace("/?window=...")` (`useWindowUrlSync.ts`, `switchToWindow`). Only one window's tree is ever rendered.
- `PaneSplitRenderer.tsx:387-395`: `<SessionDetail key={`${pane.id}-${pane.sessionId}`} isActive={isFocused} />`. Changing window changes pane IDs, so every terminal unmounts and mounts. `isActive={isFocused}` already exists and is passed to `SessionDetail` and `SessionVcsProvider` (`SessionDetail.tsx:65,148`).
- `PaneTilingContainer.tsx:~188-235`: `window.addEventListener("keydown", ..., { capture: true })` active only while `pickerPendingSession` is set; it calls `e.stopPropagation()` so a hidden window's instance would swallow keys. The `externalSessionAssign` effect also fires per container.
- `RootShell.tsx` wraps all routes in providers + `CockpitShell`; `{children}` (the route) is swapped by the router, so Sessions<->Backlog fully unmounts the page (including every terminal) unless the page is kept alive above the router.
- Router builds: `lib/nav` has `impl.next.tsx`, `impl.router.tsx` (bun), `impl.memory.tsx` (tests). Keep-alive of the Sessions page across `/` <-> `/backlog` cannot rely on Next's own cache; implementing at the `RootShell` level (mount `/` view persistently and hide it) works for both builds but both `useSearchParams`/`usePathname` consumers must then be gated by "is this route active".
- Terminals: `XtermTerminal.tsx` uses xterm addons with a budget `MAX_WEBGL_CONTEXTS = 8` (`lib/terminal/webglBudget.ts:7`), `claimWebglSlot` evicts others and prefers hidden victims, so the WebGL context limit is already handled in code. `document.visibilitychange` resync exists (line 1542), keyed to tab visibility, not pane visibility.
- 41 non-test files add `window`/`document` `keydown` listeners (grep count); only a few are in the cockpit tree (PaneTilingContainer, SessionList, ApprovalDrawer, Header, BottomNav, NewShellDialog, TerminalLinkMenu).

### Keep-alive design (INFERRED, not prototyped)

Bounded LRU of K=2-3 recently used windows, behind a flag (default off).

1. Render: map over `[currentWindow, ...lruRecent]`, each in a wrapper `<div data-window-id hidden={!active} style={{display: active?"flex":"none"}}>` containing its own `PaneTilingContainer`. Key by window id. Use `visibility:hidden; position:absolute; inset:0; pointer-events:none` instead of `display:none` if xterm sizing requires non-zero layout (see below).
2. Add an `active` prop (not just `isFocused`), plumbed `page -> PaneTilingContainer -> PaneSplitRenderer -> SessionDetail -> XtermTerminal/VCS providers`. `isActive={isFocused && windowActive}`.
3. Effects that need an `active` gate (each must early-return or detach when inactive):
   - `PaneTilingContainer` capture keydown (picker) and `externalSessionAssign`/`prevVersionRef` effect (otherwise a hidden window opens a picker and swallows keys). Also focus-stealing effects (`pickerBarRef` focus, mobile sheet focus).
   - `PaneSplitRenderer` mobile effect writing `--mobile-pane-tab-strip-height` on `documentElement` (global CSS var; only the active window may write it).
   - `useVcsStatus` / `SessionVcsProvider` (polls GetVCSStatus; 16 calls in the HAR), `useWorkflows`, other hooks already referencing `visibilityState`: add `windowActive` so hidden windows poll nothing. This is a direct request-count win.
   - XtermTerminal: pause the decoupled size sampler and ResizeObserver work when inactive; this attacks the `getBoundingClientRect` hot path (top JS self-time, 2.6 s). On re-activate do one `fit()` + `postFitRepaint`.
   - `useWindowUrlSync` and `?window=` consumers: unaffected, but any component that reads `useSearchParams` for `?session=` deep links (`page.tsx` `lookup`, `externalAssignSession`) must apply only to the active window.
   - Per-session streams (`StreamTerminal` WebSockets): decide per window: keep open for K recent windows (cost: 8-16 sockets as in the HAR) or close on deactivate and reconnect with scrollback replay. INFERRED: keeping them open multiplies sockets; prefer pausing output rendering while keeping the stream, or closing after a short grace period.
4. xterm under `display:none`: `FitAddon.fit()` reads 0x0 and the repo already notes this (`XtermTerminal.tsx` ~272, "next non-zero ResizeObserver delivery"), so on re-show it relies on the observer. With keep-alive, ensure activation triggers an explicit refit; do not call `resize` while hidden (would send a 0-col PTY resize to tmux; VERIFY with the existing `XtermTerminal.refit.test.tsx`).
5. WebGL: budget of 8 with hidden-first eviction already exists. Hidden windows' terminals should release their slot (or be the first evicted); re-claim on activation. No new limit machinery needed; check that a window's `active` is passed through the existing `touchWebglSlot`/hidden notion.
6. Lifecycle bound: evict LRU window beyond K (unmount). Memory is bounded by K x panes-per-window.

Test impact (INFERRED): `PaneTilingContainer.test.tsx`, `PaneSplitRenderer.*.test.tsx`, `RootShell.test.tsx`, and Playwright cockpit specs assume one container; add a "hidden window ignores keys/polls nothing" test.

### Dispositions for touched hotspots

- `PaneTilingContainer.tsx` (396 lines): Isolate via seam. Extract the picker keydown + externalSessionAssign effects into a small hook (`usePanePickerKeys(active, ...)`) that takes `active`; then the keep-alive wrapper only needs to pass one prop. Do not refactor the rest.
- `app/page.tsx` (678 lines, 15 `useState`/`useEffect`): Isolate via seam. Add a `WindowKeepAliveHost` component that owns the LRU and renders N containers; `page.tsx` changes from rendering one container to rendering the host. No logic moves out of `page.tsx` beyond that. A Refactor-first of page.tsx is not needed for this goal.
- `app/backlog/page.tsx` (1198 lines, 20 hooks): Refactor-first for any load-time work (its hard-load cost is 3.4 s long-task total per requirements); extract data loading and the list render into hooks/components before adding deferral or virtualization, because the current file mixes them. If scope is only "keep Backlog mounted when navigating away", Extend as-is by hosting it in the RootShell keep-alive (needs the same `active` gate on its 20 effects, which is why it is a refactor candidate).
- `PaneSplitRenderer.tsx` (510 lines): Extend as-is (add `windowActive` prop).
- `RootShell.tsx` (39 lines): Extend as-is if cross-route keep-alive is wanted; this is the only place that works for both Next and bun router builds.

## Summary of proven vs inferred

- Proven: the 6-slot cap is hit; 161 of 234 requests stalled 30 s and never connected; the stalls are mostly duplicate/chatty calls; the stall persisted when zero slow calls were active.
- Inferred: what holds the remaining slots (candidate: never-completed fetches or WS handshakes); that h2 resolves it; remote mode uses h2; keep-alive details.
