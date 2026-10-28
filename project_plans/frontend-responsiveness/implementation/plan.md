# Implementation Plan: frontend-responsiveness

**Feature**: Make window switching, Sessions<->Backlog navigation and Backlog load feel instant by cutting unary-RPC fan-out, multiplexing unary RPCs over one WebSocket, removing forced-layout work, and keeping recent windows alive.
**Date**: 2026-10-26
**Status**: Ready for implementation; `OPEN-DECISION-throttle-gate` resolved (Option A) and remote-listener WS unary decided in scope by the owner (Phase 3 repair passes 1 and 2 and the Phase 4 patch applied 2026-10-26; see the Review disposition table at the end). ADRs are Proposed; the Story 5.1.1 spike records Accepted, and the build proceeds on Proposed ADRs. Triad review fixes (product, UX, engineering) applied 2026-10-26; see the "Triad" section of the Review disposition
**ADRs**: [ADR-001 wire framing](../decisions/ADR-001-unary-over-websocket-wire-framing.md), [ADR-002 fallback/retry/auth](../decisions/ADR-002-ws-unary-fallback-retry-and-auth-semantics.md), [ADR-003 socket topology / head-of-line](../decisions/ADR-003-ws-unary-socket-topology-and-hol-mitigation.md)

**Numbering**: headings read `Epic N.1` and stories are `N.1.x` (Epic 1 = `1.1.x`, Epic 2 = `2.1.x`, Epic 3 = `3.1.x`, Epic 5 = `5.1.x`..`5.5.x`, Epic 4 = `4.1.x`, Epic 6 = `6.1.x`, Epic 7 = `7.1.x`). Epic 5 keeps its `5.1`..`5.5` sub-epic groupings in its story IDs. Every story reference in this file and the ADRs uses these IDs.

Paths are relative to the repo root; frontend paths are under `web-app/src/` unless they start with `web-app/` or `server/`. Evidence labels carried from research: VERIFIED = source opened or command run, INFERRED = reasoned.

**Owner decision (2026-10-26)**: unary-over-WebSocket is built as a first-class epic (Epic 5). The HTTP path stays as permanent fallback and kill switch. Epic 2 request-path fixes still land first because they cut volume regardless and the `getConnectTransport()` migration (Story 2.1.1) is the prerequisite that gives the WS transport a single swap point.

**Owner decision (2026-10-26, remote listener)**: the unary-over-WebSocket transport **includes the remote listener** (`--remote-port`, TLS, `:8444` and any remote-access origin) in the first release. This **reverses** the earlier stance in this plan that remote upgrades are refused with 403 `ws_unary_remote_disabled` and that the client disables WS on `https:` pages; that marker and the refusal are removed everywhere. It is an owner decision reversal, not a review finding (see Review disposition, "Owner decision reversal"). Remote is held to the same security bar as local through a strict unary-route origin check, a mandatory authenticated session before any dispatch, and the same deny-by-default allowlist (Story 5.2.1, Task 5.2.1g, ADR-002 items 7 and 11); whether WS is *enabled* on a remote origin is a per-origin client decision gated by the `unary_over_ws_remote` sub-flag and a remote/h2 comparison arm (Stories 5.4.1 and 5.5.1).

**Build, then prove**: the epic is not removed or deferred by any gate. Story 5.1.2 (decision gate, after Epic 2 and Story 1.1.4) and Story 5.5.1 (three-arm comparison: baseline / Epic 2 / Epic 2 + WS) decide only (a) the rollout default (`unary_over_ws` ships OFF and flips ON by default only if the numeric criteria in Story 5.5.1 pass), (b) sequencing priority of Task 5.4.1c, (c) whether a second priority socket (ADR-003 option C) is built, and (d) whether the stall is server-side (Story 2.1.8 fires and the bridge in-flight cap stays at 6). **The gate never holds Stories 5.2.1, 5.3.1 or 5.4.1a/b**: they start after Task 5.1.1b and do not wait for the owner re-capture (Task 1.1.4b); only Task 5.5.1, Task 5.5.2 and the default flip wait on the gate. Epic 5's Goal does not claim that WS removes the ~30 s stalls; Story 5.5.1 measures whether it does.

---

## Domain Glossary

| Term | Definition | Notes |
|---|---|---|
| `RpcDedupeKey` | `${method}\u0000${base64(serialized request)}` identifying identical unary reads | Branded string type (not a class); never built for mutations |
| `CoalescingInterceptor` | Connect interceptor sharing one in-flight promise among identical idempotent unary calls | Lives in `lib/api/transport.ts` chain |
| `IdempotentMethodTable` | Static table (`lib/api/idempotentMethods.ts`) with three independent columns per RPC method: `coalescible`, `replayable`, `wsEligible` (members enumerated in Task 2.1.2a and the shared fixture); plus a separate `cacheable` list (flags, programs, capabilities only) | Never one shared set: coalescing/replay are safe for read RPCs, TTL caching is not, WS eligibility is narrower still (ADR-002 section 3). Called `IdempotentMethodSet` in earlier drafts |
| `InFlightGauge` | Runtime opt-in counter of active and queued unary requests (HTTP and WS) exposed on `window.__ssqPerf` | Exists in any build but is absent unless opted in via `?ssqperf=1` or `localStorage["ssq:perf"]` (e2e runs the embedded production binary) |
| `PerfMark` | User Timing mark/measure named `ssq:window-switch` or `ssq:route-change` | Standard `performance.mark`, no custom bus |
| `LogBuffer` | Process-wide singleton buffer + flusher for `LogClientEvents` | Replaces per-`TerminalOutput` buffers |
| `ConsolePatch` | The single refcounted install of console interceptors | Fixes 14x chain duplication |
| `WindowKeepAliveHost` | Component owning an LRU of K recently used windows and rendering each in a wrapper | K = 3, flag `window_keep_alive` |
| `windowActive` / `isFocused` | Two separate props. `isFocused` = existing `isActive` meaning "focused pane" (`SessionDetailView.tsx:325-326`, passed from `PaneSplitRenderer.tsx:395`; cadence and keys). `windowActive` = new, "window is the visible one" (on/off for polling, observers, rAF, keydown). Plumbed `host -> PaneTilingContainer -> PaneSplitRenderer -> SessionDetail -> XtermTerminal` | Polling = `windowActive ? (isFocused ? fast : slow) : off`; keys = `windowActive && isFocused`; `usePublishStackTopOffset`'s `enabled` = `windowActive` (not focus) |
| `useSessionDetailActivity` | Hook `({isFocused, windowActive}) -> {pollMode, keysEnabled, measureEnabled}` (Task 2.1.0a) | The single place the two props combine |
| `SharedBodyMeasurer` | One body-level MutationObserver/ResizeObserver measuring once per frame and fanning out to subscribers | Replaces per-instance observers in `usePublishBottomEdge` |
| `UnaryMuxFrame` | One WebSocket binary message (<= 256 KiB whole frame): type byte + 4-byte request id + body (ADR-001, no Connect envelope for unary; RESPONSE/ERROR carry headers/trailers meta) | Types REQUEST, RESPONSE, CHUNK, ERROR, CANCEL, PING, PONG |
| `MuxRequestId` | uint32 client-chosen id correlating frames of one call | Branded `number` (not a class); in-flight collision check; even ids reserved for future server push (ADR-001) |
| `UnaryWsTransport` | Connect `Transport` whose `unary` sends `UnaryMuxFrame`s over `/api/ws/unary`; `stream` delegates to existing bridges | Falls back to HTTP per ADR-002 |
| `UnaryWsBridge` | Server handler on `/api/ws/unary` decoding `UnaryMuxFrame` and dispatching through a per-listener dispatcher (stamp -> HostGuard -> auth or `LocalWriteGuard` -> mux) split out of `localChainWith`/`remoteChain`; serves both the local and the remote listener, each through its own dispatcher and its own upgrade origin policy (ADR-001/002) | New file; shares an extracted upgrader/read-loop helper with `ws_stream_bridge.go` |
| `wsUnaryOriginPolicy` | Per-listener `CheckOrigin` for `/api/ws/unary`: local = the listener's HostGuard plus loopback rule; remote = strict allow-list (configured origins, or same-origin with a verified hostname), never `isAllowedOrigin`'s any-HTTPS rule | Task 5.2.1g; ADR-002 item 7; the existing stream bridges keep `isAllowedOrigin` (out of scope, noted in Risk Control) |
| `originClass` | Client classification of the page origin: `remote` when `location.protocol === "https:"`, else `local`; never by port | Input to `WsUnaryPolicy` (remote additionally needs `unary_over_ws_remote`); Story 5.4.1 |
| `wsUnaryAllowed` | Go-side compiled, read-only, deny-by-default table of methods the bridge will serve | Server authority; the client table must be a subset (shared JSON fixture, Story 5.1.1b) |
| `WsUnaryPolicy` | Master boolean flag `unary_over_ws` (+ `unary_over_ws_remote` when `originClass` is `remote`) + static client `wsEligible` column + per-session denylist + circuit-breaker state deciding WS vs HTTP per call | ADR-002; flags are `Record<string, boolean>` so no per-method flag exists; the remote sub-flag exists because both listeners share one server-wide flag store |
| `BacklogStoreHydration` | Persisted proto items (`toBinary`) loaded into `backlogItemsSlice` (with `isSnapshot=true`, because `false` is reserved for genuine live events and drives the flash treatment, `backlogItemsSlice.ts:43-50`) before the mount-time REST `listBacklogItems` refresh reconciles them | Replaces the earlier `CachedListSnapshot`/`listBacklog` design (Story 6.1.2) |
| `BacklogReconcile` | Pure function `reconcileAfterRest(mountIds, responseIds, touchedSinceMount, filter)` returning ids to `removeItem` after the first successful REST refresh of a mount | The REST response is the authoritative reconcile because neither REST nor the stream ever prunes today (Task 6.1.2c) |
| `pendingWindowId` | Local state in the tab strip that leads the URL-derived `currentWindowId` for the optimistic highlight | Cleared when `currentWindowId` equals it (+2 s safety timer) |
| `StallEscalation` | RoutePending behavior after ~1 s (skeleton at `PENDING_DELAY_MS` = 150 ms, `lib/nav/policy.ts:5`): "Still loading..." with Retry and "Go to Sessions", then error state at `ROUTE_PENDING_TIMEOUT_MS` (15 s, provisional, Story 6.1.3); stall and error text live in a sibling of `<main>`, outside `aria-busy` | Never an infinite skeleton |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|---|---|---|---|---|
| Approach (Step 0.5) | Evidence-ordered: diagnostics, volume cuts, render fixes, then WS multiplex + keep-alive | SDD | (a) WS transport first | The 14x/58x/56x duplicates are 161 of 234 stalled requests (VERIFIED, architecture.md); WS first multiplexes waste and hides the cause |
| Approach (Step 0.5) | | | (b) HTTP/2 via mkcert only | Needs per-machine cert; owner chose WS; remote :8444 already gets h2 |
| Approach (Step 0.5) | | | (c) TanStack Query migration | New data layer beside bespoke Watch* streams; the interceptor + small module caches are call-site-agnostic and ~60 lines |
| In-flight coalescing | Decorator (Connect interceptor) | GoF | per-hook caches only | Fixes all callers once all go through `getConnectTransport()` |
| Short-TTL read caches | Module-level cache with shared in-flight promise (copy `useFeatureFlagValue.ts:15-40`) | repo precedent | Context-per-RPC | Hooks mount per row/pane; module scope survives remount |
| Log batching | Singleton + refcounted install (Facade) | GoF | Per-instance buffers | Chained patches duplicate every line N times (VERIFIED) |
| `RpcDedupeKey`, `MuxRequestId` | Branded types | type-driven-design | raw string/number | Prevent mixing request ids with other numbers; keys only from the helper |
| `UnaryMuxFrame` | Sum type (discriminated union) | type-driven-design | untyped objects | Exhaustive handling of frame types in client and `switch` in Go |
| WS method gating | Server-side deny-by-default table + client static subset | Security (ADR-002) | Client-delivered per-method flag | Flags are `Record<string, boolean>`; a client-controlled gate must never be the only gate |
| WS vs HTTP choice | Strategy (`WsUnaryPolicy`) | GoF | `if (flag)` at call sites | One decision point, testable, kill switch |
| `UnaryWsBridge` dispatch | Adapter that re-enters a per-listener dispatcher (the inner stamp -> HostGuard -> auth\|`LocalWriteGuard` -> mux composition exposed by splitting `localChainWith`/`remoteChain`, Task 5.2.1f) with the upgrade request's guard-relevant headers only | GoF | Re-implement handlers; call the mux or a service handler directly | Keeps HostGuard, `LocalWriteGuard(guardedProcedures)`, auth, `withFileBodyLimits` and per-request expiry check identical to HTTP (ADR-002) |
| Keep-alive | `WindowKeepAliveHost` wrapper + `windowActive` prop, per-window state in a provider-shaped `useWindowsController()` hook (Task 4.1.2b) lifted above the route boundary (Story 4.1.5) | PoEAA Service Layer-ish seam | Rewrite `page.tsx` | Isolates hotspot (architecture.md); survives `/` -> `/backlog` -> `/` |
| Backlog cache | Hydrate the existing Redux slice behind `useWatchBacklogItems`; the mount-time REST refresh reconciles (prunes) | Existing architecture | Parallel `listBacklog` snapshot layer; "stream snapshot is the revalidation" | One source of truth; the stream has no end-of-snapshot marker when items exist (`useWatchBacklogItems.ts:318-326`), so only the REST response can prune |
| Keep-alive primitive | CSS hide + `inert` + gated effects; React `<Activity>` only if spike passes | build-vs-buy.md | `react-activation` | Unmaintained; Activity needs React 19.2 + Next 15.3.2 compat |
| Regression guard | ESLint `no-restricted-imports` + Jest count assertion | repo precedent (nav-facade) | Timing budgets only | Structural guard beats noisy timings |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|---|---|---|---|
| `app/backlog/page.tsx` (1198 lines, ~20 effects) | Data loading and rendering mixed; 3.4 s hard-load long tasks (cause unattributed) | Measure first (Story 1.1.5), then refactor (Story 6.1.1) | Hydration, revalidation state and keep-alive gating need a data/render split around `useWatchBacklogItems` |
| `app/page.tsx` (678 lines) | God component with ~15 state/effects; single `sessionDetailRef` (`:78,107,531,626`), `paneDispatch` bound to `currentWindow.id` (`:164-166`), single-window `paneSessions`/`externalAssignSession` | Isolate via seam (Story 4.1.2 `WindowKeepAliveHost`; Story 4.1.5 hoist) | Per-window state/refs/dispatch are named ACs, not just the single render line at `:633` |
| `components/pane/PaneTilingContainer.tsx` (396 lines) | Capture keydown + external-assign effects ungated; `usePaneShortcuts` (`:143`) ungated | Isolate via seam (extract `usePanePickerKeys(active, ...)`, Story 4.1.3a; gate `usePaneShortcuts`, Task 4.1.3d) | Hidden window must not swallow keys |
| `lib/shortcuts/shortcutRegistry.ts` | `register` overwrites on duplicate id and the first unmount's cleanup deletes the surviving registration (`:95-100`) | Fix (Task 4.1.3d) | Two windows registering the same id would lose the active window's shortcut |
| `lib/hooks/usePublishStackTopOffset.ts` | Per-instance body observers; 2,578 of 2,614 ms of rect reads; already has an `enabled` argument (`:14`) | Refactor-first (Story 3.1.1) | Direct measured hotspot |
| `components/sessions/SessionDetailView.tsx` (1977 lines) | Ungated window keydown (`:665-676`), FilesTab mounted when hidden (`:1213`), hash-seeded path (`:493-495`), `usePublishStackTopOffset` (`:465`), `useShortcut(..., isActive)` (`:634`) | Isolate via seam: Task 2.1.0 first, then fixed order (see Wave B) | Five stories edit it; the seam makes later stories small |
| `components/pane/PaneSplitRenderer.tsx` (510) | Needs `windowActive` prop, mobile CSS var write | Extend as-is | Stable; one prop (no-op plumbing in Task 2.1.0a) |
| `lib/transport/watch-ws-transport.ts` | Unary delegates to HTTP; unbounded queue in `fromWebSocket` | Isolate via seam (new `unary-ws-transport.ts`; instrument queue depth only, Story 5.3.2) | Keep streaming path untouched; a bound could end live terminal streams |
| `server/services/connectrpc_websocket.go` (3862 lines) | Very large file | Isolate via seam (new `server/services/ws_unary_bridge.go` + extracted shared upgrader/read-loop helper) | Do not grow the God file; unary frames carry no Connect envelope so `marshalProtoEnvelope`/`envelopeBufPool` (`:49-63`) are not reused |

---

## Migration Plan
N/A. No schema or data changes. Persisted client state: the Backlog hydration key (Story 6.1.2), versioned and scoped by instance/origin; and the last-known `unary_over_ws` and `unary_over_ws_remote` flag values (Story 5.4.1c), versioned, each removed when its flag is absent/OFF (`localStorage` is per origin, so the local and remote origins hold independent values with no extra scoping).

## Observability Plan
- **Logs**: `UnaryWsBridge` logs socket open/close (reason, in-flight count) and per-call errors with method and request id via `slog`; client logs only transitions (flag on/off, circuit open/close), never per call. The `LogBuffer` flusher never logs (reentrancy guard).
- **Metrics**: `ssq:window-switch` and `ssq:route-change` User Timing measures (also shipped as analytics `performance` events next to `WebVitalsReporter`); `window.__ssqPerf` gauge (runtime opt-in via `?ssqperf=1` or `localStorage["ssq:perf"]`, set by the e2e fixture; absent unless opted in): `inFlight`, `peakInFlight`, `queuedOver1s` (derived from Resource Timing `requestStart - fetchStart`/`startTime` via `PerformanceObserver('resource')`, not from a fetch wrapper), `dupCount[method]`; server `/actuator` counters for the bridge: `ws_unary_calls_total{method,code}`, `ws_unary_inflight`, `ws_unary_denied_total{method}`, and `ws_unary_upgrade_denied_total{listener,reason}` (`reason` in `origin`, `auth_required`, `query_string`, `no_dispatcher`; the CSWSH/probe signal on the remote listener). `ws_unary_fallback_total{reason}` is **client-side only** (a server cannot see calls that never reached it): it is emitted as an analytics `performance` event and read from `window.__ssqPerf`, never from `/actuator`. Existing `rpc:<Method>` timings (`lib/telemetry/rpcTiming.ts`) must cover the WS path.
- **Alerts**: no new alerts (single-operator tool); CI perf budget job reports advisory first (Story 7.1.3).

## Risk Control
- **Feature flags** (existing flag system: `FeatureFlagsContext.tsx` builds `Record<string, boolean>`, so every flag is a boolean). Flags are **server-defined** in `knownFeatureFlags` (`server/services/feature_flag_service.go:214`, each entry has `name`/`description`/`defaultValue`); `web-app/src/lib/features/features/flags.ts` is only the RPC-to-feature registry and defines no flags. One early task (Story 1.1.6, Task 1.1.6a) registers all four, so no later story edits that Go file: `window_keep_alive` (default OFF), `unary_over_ws` (master boolean, default OFF), `unary_over_ws_remote` (remote-origin sub-flag, default OFF; see "Remote access" below for why a second flag is the simplest correct option), `request_coalescing` (default ON; OFF-switch retained). Each needs a proof entry in `server/services/config_surface_proof_test.go` (`pendingCeiling` must stay 0); all four have no Go reader, so each gets an `exempt:` reason in the existing "client-only" form (`config_surface_proof_test.go:207`). There is **no** per-method flag. The per-method allowlist is a Go-side compiled `wsUnaryAllowed` table (server authority, deny-by-default, reads only) with the client `wsEligible` column required to be a subset via a shared JSON fixture; widening is a code PR, not a flag flip.
- **Pre-load defaults** (before `GetFeatureFlags` returns, and for the call that fetches the flags themselves): `request_coalescing` = ON (identical in-flight reads merge; `GetFeatureFlags` is coalescible, so there is no circularity); `unary_over_ws` = last-known persisted value from `localStorage["ssq:flag:unary_over_ws"]` (written by `flagStore` on every successful load; the key is **removed** when the loaded response omits the flag or reports it OFF, so a removed flag cannot leave the boot-time open armed), else OFF. `window_keep_alive` pre-load = last-known persisted value (`localStorage["ssq:flag:window_keep_alive"]`, `"1"`/`"0"`, written on every successful load, removed when the response omits the flag), else the shipped default constant in `flagStore.ts` (OFF until Story 4.1.6 flips it, then ON); the first render therefore mounts the host or the single container directly with no remount unless the server value differs from the persisted one (one accepted remount). `request_coalescing`'s pre-load default is the fixed constant ON and is not persisted. `unary_over_ws_remote` follows the same rule as `unary_over_ws` (persisted last-known value in `localStorage["ssq:flag:unary_over_ws_remote"]`, removed when the response omits it or reports it OFF, else OFF); because `localStorage` is per origin, a remote page only ever reads the value its own origin wrote, and the boot-time open on a remote page requires **both** persisted values true. So exactly three flags are persisted (`unary_over_ws`, `unary_over_ws_remote`, `window_keep_alive`), each with a defined pre-load value, and `request_coalescing` has a constant (one rule per flag, no conflict). `GetFeatureFlags` itself is never `wsEligible`, so the flag fetch always uses HTTP. The default flips (Stories 5.5.2 and 4.1.6) change `defaultValue` in `knownFeatureFlags` and the matching pre-load constant in `flagStore.ts`.
- **Kill-switch latency**: `unary_over_ws` OFF takes effect within one flag refresh (<= 30 s, the `flagStore` TTL), or immediately via the circuit breaker or the local force-HTTP override `localStorage["ssq:ws-unary:force-http"]="1"`, which `WsUnaryPolicy` reads on every call.
- **Rollback**: flip the flag (HTTP path is always present) or set the local override. Revert PR for non-flagged Epic 2/3 changes. Ship-unflagged a11y fixes (Story 4.1.4) are reverted by PR only.
- **Remote access (`--remote-port`, TLS, `:8444` and any remote-access origin) is served in the first release** (owner decision 2026-10-26; reverses the earlier 403 `ws_unary_remote_disabled` stance, which no longer exists anywhere in this plan). Server side: the remote listener has its own dispatcher (stamp(`remote`) -> `middleware.Auth` -> mux, from the split `remoteChain`, Task 5.2.1f), the same `wsUnaryAllowed` allow-list and client-header allow-list, and guard-relevant headers copied only from the upgrade request. The remote chain has no HostGuard and `isAllowedOrigin` accepts any HTTPS origin (VERIFIED, `server/server.go:1941-1948`, `server/services/connectrpc_websocket.go:104-120`), so the unary route does **not** use `isAllowedOrigin` on remote: `wsUnaryOriginPolicy` (Task 5.2.1g, ADR-002 item 7) requires a non-empty Origin that is in the configured allowed-origin set or is same-origin with a verified hostname, an authenticated session before the upgrade completes, and re-checks auth on every REQUEST. **Client side**: `originClass` is `remote` when `location.protocol === "https:"` (the remote listener is HTTPS-only per the `--remote-port` help in `main.go:888`; `createSessionWatchTransport` gates native streaming on the same `https://` test, `web-app/src/lib/transport/watch-ws-transport.ts:217-231`), **never by port**, so dynamic and manual dev ports (`findFreePort`, 62871+) stay `local`. The decision is per origin: `local` uses WS when `unary_over_ws` is ON (default ON only after the Story 5.5.1 local criteria pass); `remote` uses WS only when `unary_over_ws` **and** `unary_over_ws_remote` are ON. A second flag is the simplest correct choice because both listeners share one server-wide flag value (`flagStore` fetches the same `GetFeatureFlags` response on either origin), so a single boolean cannot be ON for local and OFF for remote; the alternative (derive the remote value from the master flag plus a hard-coded origin rule) would make the remote default un-flippable without a code change. `unary_over_ws_remote` defaults OFF and is flipped only after the remote/h2 arms of the Story 5.5.1 comparison pass (the remote default flip is part of Task 5.5.2a). Remote already negotiates HTTP/2 (`ServeTLS`, `server/server.go:1972-2010`; an existing test, `TestServer_should_NegotiateALPNHTTP2_When_StartRemoteServesOverRealTLS`, `server/server_integration_test.go:66`, asserts ALPN h2 from a real `StartRemote`; read, not run, and its client passes `NextProtos: ["h2"]` itself, so browser-side negotiation against the production `tlsCfg` is still INFERRED; Task 5.5.1d asserts `nextHopProtocol === "h2"` on the remote arm), so the expected benefit differs from local: h2 already multiplexes unary calls, and WS adds its own head-of-line behavior (one ordered byte stream). Whether WS helps or hurts on h2 is measured, not assumed (Story 5.5.1 arms D and E).
- **Remote-listener risks (pre-mortem style, added with the owner decision)**: (1) **Cross-site WebSocket hijacking (CSWSH)**: a page on a hostile origin opens a WS to the remote listener with the victim's cookie. The session cookie is `Secure` + `SameSite=Strict` (VERIFIED, `server/auth/handlers.go:290-291,385-386`), which blocks a cross-**site** handshake, but SameSite is site-scoped, not origin-scoped (INFERRED from the SameSite definition: another port or sibling host on the same registrable domain is same-site and does send the cookie), so the strict Origin check is the primary control, not a redundant one. Controls: `wsUnaryOriginPolicy`, tests T-CSWSH-1..4 (Task 5.2.1g). (2) **Token leakage in URLs**: browsers cannot set `Authorization` on a WebSocket, and any "put the token in `?token=`" shortcut would land in access logs, `otelhttp` span attributes, browser history and `Referer`. Controls: the route accepts **no query string** (a non-empty `RawQuery` answers 400 `ws_unary_query_forbidden` and increments `ws_unary_upgrade_denied_total{reason="query_string"}`), the client builds the URL from `location` with no query or fragment, no `Sec-WebSocket-Protocol` subprotocol is accepted (no token smuggling), auth is the `cs_auth` cookie (browser) or `Authorization: Bearer` header (non-browser) only. (3) **Auth-less remote chain**: `StartRemote` accepts a nil `authMW` (existing test posture, `server/server.go:1938-1940`) and `middleware.Auth` is a no-op when the validator is nil (`AuthRequires`, `server/middleware/auth.go:46-58`), leaving the remote chain open. Control: the bridge refuses the remote upgrade unless `RequestRecordFrom(ctx).AuthMode == AuthModeRequired` (403 `ws_unary_auth_required`), so WS never widens an open chain. (4) **Reverse proxies and CDNs** in front of the remote listener may strip `Upgrade`/`Connection`, buffer, or enforce idle timeouts (INFERRED, not tested here): a failed upgrade counts toward the breaker (3 failures -> HTTP-only 60 s, then one probe), so the page degrades to HTTP; the flip guide lists the required proxy settings (`Upgrade` pass-through, idle timeout above the 10 s in-flight PING interval and the 30 s probe); response buffering is irrelevant to a framed binary socket but a proxy that coalesces frames adds latency visible in arm E. (5) **TLS/cert**: the remote origin is served with a private CA (`caFile`, `main.go:1445`); a browser that does not trust it cannot load the page at all, so the `wss://` handshake inherits the page's trust decision; a mixed-content downgrade is impossible because the URL scheme is derived from the page (`https` -> `wss`, Task 5.3.1b). (6) **Cross-listener confusion**: a dispatcher selected from the wrong listener would apply local rules to remote traffic or vice versa; the dispatcher is chosen from `RequestRecordFrom(ctx).Listener` stamped by that listener's own chain and a missing/unknown listener fails closed (`unavailable`). (7) **Session expiry mid-socket** produces per-request `unauthenticated`, not a breaker trip, and the client parks WS until login completes (Story 5.4.1). (8) **The stream bridges on remote keep `isAllowedOrigin`** (out of scope; they exist today and are not changed here); a follow-up to move them onto `wsUnaryOriginPolicy`'s remote rule is recorded in Unresolved Questions, not built.
- **Staged rollout**: dev instance on isolated ports 62871+ first; owner dogfood on the live instance with the reads-only `wsUnaryAllowed`; then widen by PR. Never use `make install-service` to trial in-progress work (CLAUDE.md).

## Unresolved Questions
- [ ] Why do 14 identical `GetFeatureFlags` calls fire at t=2.8 s when code reads as 1 (`FeatureFlagsContext.tsx:105`, `useFeatureFlagValue.ts:30`)? — blocks Task 2.1.4b (re-point provider and hook; Task 2.1.4a, the module extraction, does not wait); the coalescer (2.1.2) is defense in depth and must not mask the cause — owner: Story 1.1.1 output, then implementer.
- [ ] What held the other connection slots during the ~30 s stalls (only ~3 unary in flight while 160+ queued)? Two hypotheses, not one: browser-side holders, or **server-side contention** (`ListShells` is an in-memory lookup yet waits 4-6 s, and every call runs `resolveSessionTitle` -> `s.storage.ListInstanceData()`, a full session-list read, `server/services/session_service_shells.go:62-73,218`: the first suspect; also the client's own 30 s abort `DEFAULT_RPC_TIMEOUT_MS = 30_000`, `web-app/src/lib/api/transport.ts:18`, produces status-0 / `blocked ~30000` entries for any request that waits 30 s anywhere, not only in the browser queue) — blocks the Story 5.1.2 decision gate and ADR-003 topology confirmation (Story 5.5.2) — owner: Story 1.1.4 output; owner tstapler provides a DevTools-closed re-capture (wall-clock blocker).
- [ ] Does Chrome count in-progress WebSocket handshakes against the per-host HTTP/1.1 cap? How does the socket open while 6 HTTP calls are pending? — blocks ADR-003 confirmation — owner: Story 1.1.4 and Task 5.4.1c (open-under-saturation test).
- [ ] Does React `<Activity>` (19.2+) work under Next 15.3.2 and the Bun build? — blocks Story 4.1.2 choice of primitive — owner: Story 4.1.1 spike.
- [ ] What is the `/backlog` 3.4 s hard-load long-task cost made of (store hydration, mapping, render of N cards, the ~20 effects)? Does return navigation already hit a warm global Redux store (INFERRED)? — blocks Story 6.1.2 scope — owner: Story 1.1.5.
- [x] ~~Does the server HostGuard wrap WS routes?~~ **Closed**: yes on the `:8543` local chain (`server/guarded_procedures.go:91-113`, "HostGuard covers every route (and WebSocket upgrade)"); the remote chain has no HostGuard and relies on auth (`server/server.go:1941`). The real risk is the second layer, `LocalWriteGuard(guardedProcedures)`, keyed by per-procedure request path (see Story 5.2.1 and ADR-002). Story 5.1.1 is narrowed to the dispatch mechanism.
- [ ] Is `ListShells` / `GetVCSStatus` server latency (4-6 s / 3-4.5 s) acceptable once slots are no longer contended? — out of scope (backend), follow-up only — owner: tstapler.
- [ ] Remote listener (owner decision 2026-10-26), items not verifiable by reading: (a) which hostnames `GetHostnames()` holds on a remote-access run (`SetHostnames` at `main.go:456`, `ReplaceVerifiedHostnames`), which decides whether the same-origin clause of `wsUnaryOriginPolicy` ever fires beyond the configured-origin list (`main.go:1604-1626` already adds `https://<rpID>:<port>`, `https://<hostname>:<port>` and `https://localhost:<port>` to `srv.origins`); (b) whether a browser actually negotiates h2 to the remote listener (a Go-client ALPN test exists, `server/server_integration_test.go:66`, not run here; no browser-side assertion); (c) whether the WS handshake succeeds through the owner's real remote path (direct LAN/Tailscale vs a reverse proxy); (d) how to obtain an authenticated remote session in headless Playwright (remote auth is passkey; no existing spec logs in, `tests/e2e/login-static-assets.spec.ts` only observes the unauthenticated surface; the local-login code exchange in `server/auth/localtoken.go:116-146` mints a `cs_auth` cookie but is wired for the local listener, applicability to remote is unverified); Task 5.1.1a (seams, a-b) and Task 5.5.1d (c-d) own these; fallbacks are named there — blocks Task 5.5.1d and the `unary_over_ws_remote` flip only, no build story — owner: Task 5.1.1a / 5.5.1d output.
- [ ] Follow-up (not built): the existing stream bridges (`ws_stream_bridge.go`, `connectrpc_websocket.go`) still use `isAllowedOrigin` on the remote listener; moving them onto the strict remote origin rule is a separate change — owner: tstapler.

## Known-issues check (docs/bugs)
Checked 2026-10-26: `docs/known-issues/` does not exist (`ls docs/known-issues` -> "No such file or directory"); open items live in `docs/bugs/open/` (70 files, listed with `ls docs/bugs/open`; none is a project-level known-issues index). Read for relevance to this work, none blocks it:
- `BUG-020-vcs-status-diff-mutex-contention.md` (Status Open): `GetVCSStatus`/`GetSessionDiff` run git under a mutex. Relevant evidence for the server-side row of Story 5.1.2 and the SM1 fallback (Story 2.1.8); it is the named next suspect if the `ListInstanceData` fix does not move the `GetVCSStatus` p95. Not a blocker: that row fires only on evidence.
- `BUG-097-a11y-retry-button-tab-order-flaky-fixed-tab-budget.md` (Open): a fixed 60-Tab walk flakes. Incorporated: the criterion 10 hidden-window tab walk is bounded by the focusable count + 5 (Task 4.1.3f), not 60.
- `BUG-110-mobile-terminal-blank-after-keyboard-open.md` and `BUG-109-mobile-touch-drag-scroll-one-directional.md` (Open): mobile terminal keyboard and touch defects. Incorporated: touch-originated switches never auto-focus the terminal (Story 4.1.4). Neither is fixed or worsened by this plan.
- `BUG-079-jest-full-suite-only-failures-stuckitem-websocket-terminal-backlogdetail.md` and `BUG-083-server-services-flaky-under-full-suite-parallel-load.md` (Open): full-suite-only flakes in Jest websocket/terminal/backlog-detail tests and `server/services`. Risk, not a blocker: new transport/WS tests should run isolated (`-run WsUnary`) and follow the `deterministic-fast-tests` skill; a failure in those pre-existing tests is attributed with a rerun on `main`, not waved through.

## Open Decisions (owner)

### OPEN-DECISION-throttle-gate
**Question**: which run gates the window-switch budget (retained-window median `ssq:window-switch` <= 150 ms and no long task >= 50 ms) and Story 4.1.6 criterion 1?
- **Option A**: gate on the **unthrottled** run; the 4x CPU-throttled run is reported and advisory.
- **Option B**: gate on the **4x CPU-throttled** run; the unthrottled run is reported and advisory.
**Neutral facts**: requirements Success Metrics state no throttle; `design/ux.md` AC13 states 4x; the 0.6-1.8 s requirements baseline was measured unthrottled on empty windows and is not comparable to throttled fixture numbers.
**Referenced from** (they point here and do not restate the options): `design/ux.md` AC13, this plan's Story 1.1.3 (throttle bullet), Task 4.1.4c, Story 4.1.6 criterion 1, `implementation/validation.md` G2, SM3 and UX row 13, `requirements.md` Success Metrics.
**Decider**: tstapler. **Blocks**: the Story 4.1.6 `window_keep_alive` default flip only; no build story waits on it.
**Interim behavior until resolved**: the perf spec records both the unthrottled and the 4x numbers on every run and hard-fails on neither.
**Resolution**: Option A (unthrottled gates; 4x CPU throttle is recorded and advisory). Owner decision, 2026-10-26.

### RULE-target-miss (a standing rule, not a question)
If a numeric target (the 150 ms retained-window switch, the < 1 s cold `/backlog` load, SM1 zero-blocked, the +10% echo guard) is unreachable after the planned work for it has landed, the final report (Task 7.1.4a, `research/final-recapture.md`) records the **achieved number, its cause and the evidence**; the owner (tstapler) then decides **accept** or **extend** (a named follow-up story). A miss never silently edits the target, never removes or reopens a built epic, and never blocks unrelated flips. The requirements Success Metrics point here.

## Dependency Visualization

Labels are story IDs (`Epic N.1` headings, stories `N.1.x`).

```
Epic 1.1 Diagnostics (stories 1.1.1 .. 1.1.6)
 1.1.1a perfDebug.ts -> 1.1.2 gauge+marks (opt-in gate) -> 1.1.1b trace capture
 1.1.2 -> 1.1.3 perf spec+project -> 1.1.4 slot holders | 1.1.5 backlog attribution (needs 1.1.3)
 1.1.6 register flags (Go, no deps; must merge before Wave B)
        |  (1.1.3 baseline.json captured BEFORE any Epic 2 story merges = arm A)
        v
Epic 2.1 Request path                         Epic 3.1 Render path (parallel)
 2.1.0 seam (SessionDetailView)               3.1.1 SharedBodyMeasurer (after 2.1.0, 2.1.6c)
 2.1.1 migrate to getConnectTransport()       3.1.2 hidden-surface gating (3.1.2c server / 3.1.2d client zero-size resize guards)
 2.1.4a flagStore -> 2.1.2 coalescing (reads flagStore); 2.1.4b needs 1.1.1; 2.1.3 log (after 2.1.1d on TerminalOutput.tsx)
 2.1.5 programs, 2.1.6 per-pane dedupe (after 2.1.0)
 2.1.7 Epic 2 exit assertions (queuedOver1s == 0, status-0 == 0)
 2.1.8 server-side contention fix (conditional: fires only from the 5.1.2 table's server-side row)
 2.1.9 request priority/deferral (after 2.1.2b)
        |
        |  (5.1.1 spike + 5.1.1b vectors/stubs start after 2.1.1; Epic 5 build does NOT wait for the gate)
        v
Epic 5 Unary over WebSocket
 5.1.1 spike (5.1.1a gates 5.2.1f/5.2.1b) -> 5.1.1b vectors+stubs -> 5.2.1 server bridge (5.2.1f dispatcher split first) \
                                     -> 5.3.1 client transport    -> 5.4.1 policy/bootstrap -> 5.5.1 three-arm -> 5.5.2
 Story 5.1.2 DECISION GATE (needs Epic 2 + 1.1.4): sets default, 5.4.1c priority, priority-socket plan;
   holds only 5.5.1 / 5.5.2 / the default flip, never 5.2.1/5.3.1/5.4.1a-b
 5.3.2 queue-depth instrumentation (independent); 5.3.1d client liveness (after 5.3.1c)
 5.2.1g remote origin policy + CSWSH tests (after 5.2.1f; pure policy file independent) -> 5.5.1d remote arms/e2e (needs 5.2.1g, 5.4.1, 1.1.6a);
   remote flip (unary_over_ws_remote) waits for 5.5.1d R1-R5, never the build stories

Epic 4.1 Keep-alive (needs 2.1.0, 3.1.2, 2.1.6c)    Epic 6.1 Backlog (needs 1.1.5, 2.1.1c)
 4.1.4 a11y (unflagged; creates WindowPanel) -> 4.1.1 spike -> 4.1.2 host -> 4.1.5 lift above route -> 4.1.6 flip
 4.1.3 gating (a..e; needs 4.1.2, 2.1.6a/d)    6.1.1 hook -> 6.1.2 hydration+reconcile -> 6.1.3 prefetch/stall (needs 4.1.4a)
        |
        v
Epic 7.1 Guards: 7.1.1 ESLint (same PR as last 2.1.1 batch), 7.1.2 Jest, 7.1.3 CI budget (after 1.1.3) -> 7.1.4 final re-capture
```

Critical path: 1.1.1a -> 1.1.2 -> 1.1.3 -> 2.1.0 -> 2.1.1 -> 5.1.1/5.1.1b -> 5.2.1 || 5.3.1 -> 5.4.1 -> 5.5.1 (needs Epic 2 complete, 1.1.4 and the 5.1.2 gate) -> 5.5.2 -> 7.1.4 (owner re-capture).

### Task-level dependency edge list (generated, cycle-checked)

Generated by `python3 -I project_plans/frontend-responsiveness/implementation/check-task-deps.py project_plans/frontend-responsiveness/implementation/plan.md` from the `Depends on` lines of this file (re-run it after any change to a `Depends on` line or a task heading). A story-level line applies to every task of that story and a story target means all of its tasks (`N (all)`); `Epic N.1` and `A through B` ranges are expanded; within-story task order and the Wave B shared-file edit order are scheduling, not `Depends on` edges, and are not modeled, so the graph is an over-approximation and "acyclic" holds for the real graph too. Seven stories had no `Depends on` line and gained one taken from this plan's own wave text (2.1.1, 2.1.3, 5.3.1, 5.4.1, 5.5.2, 7.1.2, 7.1.4); "none" means no `Depends on` line, not "no prerequisite in the waves".

**Cycle check result: acyclic; 98 task nodes, 430 edges, 39 stories** (Kahn's algorithm; exit status 0).

| Task | Depends on |
|---|---|
| 1.1.1a | none |
| 1.1.1b | 1.1.2a |
| 1.1.2a | none |
| 1.1.2b | none |
| 1.1.3a | 1.1.2 (all) |
| 1.1.3b | 1.1.2 (all) |
| 1.1.3c | 1.1.2 (all) |
| 1.1.3d | 1.1.2 (all) |
| 1.1.3e | 1.1.2 (all) |
| 1.1.4a | 1.1.2 (all), 1.1.3 (all) |
| 1.1.4b | 1.1.2 (all), 1.1.3 (all) |
| 1.1.4c | 1.1.2 (all), 1.1.3 (all) |
| 1.1.5a | 1.1.3 (all) |
| 1.1.6a | none |
| 2.1.0a | none |
| 2.1.0b | none |
| 2.1.1a | 1.1.2 (all), 1.1.3 (all) |
| 2.1.1b | 1.1.2 (all), 1.1.3 (all) |
| 2.1.1c | 1.1.2 (all), 1.1.3 (all) |
| 2.1.1d | 1.1.2 (all), 1.1.3 (all) |
| 2.1.1e | 1.1.2 (all), 1.1.3 (all) |
| 2.1.2a | 1.1.6 (all), 2.1.1a, 2.1.4a |
| 2.1.2b | 1.1.6 (all), 2.1.1a, 2.1.4a |
| 2.1.3a | 2.1.1d |
| 2.1.3b | 2.1.1d |
| 2.1.4a | 1.1.6 (all) |
| 2.1.4b | 1.1.1b, 1.1.6 (all), 2.1.4a |
| 2.1.5a | none |
| 2.1.6a | none |
| 2.1.6b | none |
| 2.1.6c | 2.1.0 (all) |
| 2.1.6d | none |
| 2.1.7a | 1.1.3 (all), 2.1.1 (all), 2.1.2 (all), 2.1.3 (all), 2.1.4 (all), 2.1.5 (all), 2.1.6 (all) |
| 2.1.8a | 1.1.4c |
| 2.1.9a | 2.1.2b, 2.1.6 (all) |
| 2.1.9b | 2.1.2b, 2.1.6 (all) |
| 3.1.1a | 1.1.3 (all), 2.1.0 (all) |
| 3.1.1b | 1.1.3 (all), 2.1.0 (all) |
| 3.1.2a | none |
| 3.1.2b | none |
| 3.1.2c | none |
| 3.1.2d | none |
| 5.1.1a | none |
| 5.1.1b | none |
| 5.1.2a | 1.1.4 (all), 2.1.0 (all), 2.1.1 (all), 2.1.2 (all), 2.1.3 (all), 2.1.4 (all), 2.1.5 (all), 2.1.6 (all), 2.1.7 (all), 2.1.8 (all), 2.1.9 (all) |
| 5.2.1a | 5.1.1 (all) |
| 5.2.1f | 5.1.1 (all) |
| 5.2.1b | 5.1.1 (all) |
| 5.2.1c | 5.1.1 (all) |
| 5.2.1d | 5.1.1 (all) |
| 5.2.1g | 5.1.1 (all) |
| 5.2.1e | 5.1.1 (all) |
| 5.3.1a | 5.1.1b |
| 5.3.1b | 5.1.1b |
| 5.3.1c | 5.1.1b |
| 5.3.1d | 5.1.1b |
| 5.3.2a | none |
| 5.4.1a | 2.1.9a, 5.3.1 (all) |
| 5.4.1b | 2.1.9a, 5.3.1 (all) |
| 5.4.1c | 2.1.9a, 5.3.1 (all) |
| 5.5.1a | 2.1.7 (all), 5.1.2 (all), 5.4.1 (all) |
| 5.5.1b | 2.1.7 (all), 5.1.2 (all), 5.4.1 (all) |
| 5.5.1d | 2.1.7 (all), 5.1.2 (all), 5.4.1 (all) |
| 5.5.1c | 2.1.7 (all), 5.1.2 (all), 5.4.1 (all) |
| 5.5.2a | 5.5.1 (all) |
| 4.1.1a | none |
| 4.1.2a | 1.1.6 (all), 2.1.0 (all), 4.1.1 (all), 4.1.4b |
| 4.1.2b | 1.1.6 (all), 2.1.0 (all), 4.1.1 (all), 4.1.4b |
| 4.1.3a | 2.1.0 (all), 2.1.6a, 2.1.6c, 2.1.6d, 3.1.2 (all), 4.1.2 (all) |
| 4.1.3b | 2.1.0 (all), 2.1.6a, 2.1.6c, 2.1.6d, 3.1.2 (all), 4.1.2 (all) |
| 4.1.3c | 2.1.0 (all), 2.1.6a, 2.1.6c, 2.1.6d, 3.1.2 (all), 4.1.2 (all) |
| 4.1.3d | 2.1.0 (all), 2.1.6a, 2.1.6c, 2.1.6d, 3.1.2 (all), 4.1.2 (all) |
| 4.1.3e | 2.1.0 (all), 2.1.6a, 2.1.6c, 2.1.6d, 3.1.2 (all), 4.1.2 (all) |
| 4.1.3f | 2.1.0 (all), 2.1.6a, 2.1.6c, 2.1.6d, 3.1.2 (all), 4.1.2 (all) |
| 4.1.4a | none |
| 4.1.4b | none |
| 4.1.4c | 4.1.2 (all), 4.1.3 (all) |
| 4.1.4d | none |
| 4.1.5a | 1.1.3 (all), 4.1.2 (all), 4.1.3 (all), 4.1.4 (all) |
| 4.1.5b | 1.1.3 (all), 4.1.2 (all), 4.1.3 (all), 4.1.4 (all) |
| 4.1.5c | 1.1.3 (all), 4.1.2 (all), 4.1.3 (all), 4.1.4 (all) |
| 4.1.6a | 1.1.6 (all), 4.1.2 (all), 4.1.3 (all), 4.1.4 (all), 4.1.5 (all) |
| 6.1.1a | 1.1.5 (all), 2.1.1c |
| 6.1.1b | 1.1.5 (all), 2.1.1c |
| 6.1.2a | 1.1.5 (all), 6.1.1 (all) |
| 6.1.2b | 1.1.5 (all), 6.1.1 (all) |
| 6.1.2c | 1.1.5 (all), 6.1.1 (all) |
| 6.1.2d | 1.1.5 (all), 6.1.1 (all) |
| 6.1.3a | 4.1.4a, 6.1.2 (all) |
| 6.1.3b | 4.1.4a, 6.1.2 (all) |
| 6.1.3c | 4.1.4a, 6.1.2 (all) |
| 6.1.3d | 4.1.4a, 6.1.2 (all) |
| 7.1.1a | 2.1.1 (all) |
| 7.1.2a | 2.1.4 (all), 2.1.5 (all) |
| 7.1.2b | 2.1.4 (all), 2.1.5 (all) |
| 7.1.3a | 1.1.3 (all) |
| 7.1.3b | 1.1.3 (all) |
| 7.1.4a | 4.1.6 (all), 5.5.2 (all), 6.1.3 (all), 7.1.3 (all) |


---

## Phase 1: Diagnostics

### Epic 1.1: Evidence before fixes
**Goal**: Resolve the two unresolved questions and make every later claim measurable.

#### Story 1.1.1: Trace the 14 GetFeatureFlags calls
**As a** maintainer, **I want** to know which code path fires 14 identical flag calls, **so that** Story 2.1.4 fixes the real cause.
**Acceptance Criteria**:
- A dev-only `console.trace` (behind `window.__ssqPerf` enabled) logs the stack for every `getFeatureFlags` and `loadFlags` call.
  - *Given* an isolated instance with a restored 3-window state, *When* the page loads fresh, *Then* the captured log lists N call stacks grouped by distinct initiator, with N recorded in the story's findings note (baseline 14).
- Findings are written to `project_plans/frontend-responsiveness/research/flags-trace.md`.
  - *Given* the capture, *When* the note is committed, *Then* it names the root cause as one of: multiple provider instances, dedupe failure, or duplicate module instance.
**Files**: `lib/contexts/FeatureFlagsContext.tsx`, `lib/hooks/useFeatureFlagValue.ts`, `lib/perf/perfDebug.ts` (new)
**Order**: Task 1.1.1a creates `lib/perf/perfDebug.ts` and merges before Task 1.1.2a (the other editor of that file); Task 1.1.1b waits for Task 1.1.2a, which defines the runtime opt-in the capture needs.

##### Task 1.1.1a: Add gated trace hook (XS)
- Create `perfDebug.ts` (first editor) exporting `traceCall(label)` no-op unless `window.__ssqPerf?.trace`.
- Call it at `FeatureFlagsContext.tsx:105` and `useFeatureFlagValue.ts:30`.
- Files: `lib/perf/perfDebug.ts`, `lib/contexts/FeatureFlagsContext.tsx`, `lib/hooks/useFeatureFlagValue.ts`

##### Task 1.1.1b: Capture and write findings (S)
- Depends on Task 1.1.2a (the `ssq:perf` opt-in). Run on isolated instance (manual recipe, port 62871, `STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-1`); record stacks to `research/flags-trace.md`.
- Files: `research/flags-trace.md`

#### Story 1.1.2: In-flight gauge and window-switch / route-change marks
**As a** maintainer, **I want** peak in-flight, queued>1 s and switch/route durations visible without DevTools, **so that** before/after is comparable.
**Acceptance Criteria**:
- A runtime-opt-in `InFlightGauge` exposes `window.__ssqPerf.{inFlight,peakInFlight,queuedOver1s,dupCount}`. `queuedOver1s` is the count of unary requests whose Resource Timing entry has `requestStart - fetchStart` > 1000 ms (browser queue/blocked time), read through `PerformanceObserver('resource')`; the shared transport `fetch` wrapper counts `inFlight`/`peakInFlight`/`dupCount` only, because a wrapper cannot see queue time. Requests that fail before `requestStart` (status 0) are counted from the wrapper's rejections.
  - *Given* a page issuing 10 concurrent `ListFiles` calls against a server that delays 100 ms, *When* all settle, *Then* `peakInFlight` is 10 and `dupCount["ListFiles"]` equals the duplicates counted by request key.
  - *Given* 12 concurrent calls to a delayed endpoint under the 6-connection cap, *When* they settle, *Then* `queuedOver1s` equals the number of Resource Timing entries with blocked time > 1000 ms (asserted against a direct read of `performance.getEntriesByType("resource")`).
- `performance.measure("ssq:window-switch")` is recorded from the switch action to the double-rAF after the target window commits **and** the target's terminal has fit and taken focus (end mark names "interactive", not just "committed"); `ssq:route-change` likewise around navigation.
  - *Given* window `a1` active, *When* the user presses Alt+W then `2`, *Then* one `ssq:window-switch` measure exists with a duration in ms and `start` before the `keydown` handler returned.
- WS-routed calls never appear in Resource Timing, so the gauge also exposes `wsQueueMs` (per call: policy decision to REQUEST frame written) and `wsQueuedOver1s` (count of calls above 1000 ms), populated by Task 5.4.1b once the WS transport exists (always 0 before). Requirement metric 1 ("zero unary requests with blocked > 1 s") is `queuedOver1s + wsQueuedOver1s`, so moving calls to WS cannot satisfy it vacuously.
  - *Given* a WS call whose REQUEST waits 1.2 s in the send queue, *When* it settles, *Then* `wsQueuedOver1s` is 1.
- The gauge and `__ssqPerf` exist only when opted in at runtime (`?ssqperf=1` or `localStorage["ssq:perf"]`); the e2e fixture sets it because e2e runs the embedded production binary. The `ssq:*` User Timing measures exist in all builds.
  - *Given* a production build with no opt-in, *When* `typeof window.__ssqPerf` is read, *Then* it is `"undefined"`; *Given* the opt-in, *Then* it is an object.
**Files**: `lib/api/transport.ts`, `lib/perf/perfDebug.ts`, `lib/perf/resourceTiming.ts` (new), `lib/window/useWindowUrlSync.ts`, `lib/nav/useFocusOnNavigate.ts`

##### Task 1.1.2a: InFlightGauge + Resource Timing observer (M)
- Add a custom `fetch` to `createConnectTransport` options in `lib/api/transport.ts` to count start/finish/status-0; add `resourceTiming.ts` with the `PerformanceObserver('resource')` queue-time computation; runtime opt-in gate.
- Serialization: first of the five stories that edit `lib/api/transport.ts` (order 1.1.2a -> 2.1.1a -> 2.1.2b -> 2.1.9a -> 5.4.1b); edits `lib/perf/perfDebug.ts` after Task 1.1.1a created it.
- Files: `lib/api/transport.ts`, `lib/perf/perfDebug.ts`, `lib/perf/resourceTiming.ts`, `lib/perf/perfDebug.test.ts`

##### Task 1.1.2b: PerfMarks for window switch and route change (S)
- Add `markWindowSwitchStart()`/`markWindowSwitchEnd()` helpers (double rAF after fit + focus) and wire in `useWindowUrlSync.switchToWindow` and the pathname-change hook.
- Files: `lib/perf/perfMarks.ts` (new), `lib/window/useWindowUrlSync.ts`, `lib/nav/useFocusOnNavigate.ts`, `lib/perf/perfMarks.test.ts`

#### Story 1.1.3: Playwright perf spec (port of prof.cjs + cpu.cjs)
**Depends on**: 1.1.2 (the spec asserts `ssq:window-switch`, `peakInFlight`, `queuedOver1s` and sets the `ssq:perf` opt-in).
**As a** maintainer, **I want** a repeatable isolated-instance perf spec, **so that** every epic is measured the same way.
**Acceptance Criteria**:
- Specs live under repo-root `tests/e2e/perf/` (where `tests/e2e/global-setup.ts`, `tests/e2e/pages/` and `tests/e2e/playwright.config.ts` live; `web-app/tests/e2e/` has no `global-setup.ts` or `pages/` and its config runs `next dev`). `tests/e2e/perf/frontend-responsiveness.spec.ts` starts with `// @feature ...`, uses the `global-setup.ts` isolated instance, uses only `data-testid`/ARIA locators, and contains no `waitForTimeout`.
  - *Given* the seeded fixture (20 sessions across 4 windows: w1 = 8 panes, w2-w4 = 4 panes each, which is the 3 x 4 WebGL-budget case plus a fourth window so that visiting w1..w4 forces the K = 3 eviction of w1; 200 backlog items), *When* the spec runs, *Then* it passes and prints per-scenario JSON with long-task count/sum/max, `LayoutCount` delta, request counts by RPC, `peakInFlight`, `queuedOver1s`.
- A dedicated Playwright project `perf` in `tests/e2e/playwright.config.ts` has `testMatch: '**/perf/**/*.spec.ts'` and its own `timeout` override (the global `timeout: 30000` at `playwright.config.ts:51` cannot hold a 5-run, 4x-throttled, 16 s-load scenario; use a per-test `test.setTimeout` or a project `timeout` of 600000). Only the projects that lack a narrow `testMatch` need `testIgnore: '**/perf/**'`: `chromium` (`:144-145`) and `chromium-dom` (`:189-190`); every other project (`visual-*`, `desktop-*`, `mobile-*`, `webkit-mobile`) already has a narrow `testMatch` (VERIFIED by reading the file). `workers: 1` and `fullyParallel: false` are already global (`:59,65`) and `--project=perf` is the only supported way to run it, so timings are not shared with other specs. Seeding 20 sessions with real tmux in CI is an unpriced feasibility risk: Task 1.1.3a first proves the fixture seeds in CI (smaller fixture fallback recorded in `baseline.json` metadata if not).
  - *Given* `npx playwright test --list` with default projects, *When* run, *Then* no `perf/` spec is listed; *Given* `--project=perf`, *Then* only perf specs are listed.
- Scenarios: load `/`; switch window x7 (drop first 2 as warmup) **retained** and, separately, **cold** (first visit / LRU-evicted, reported as its own number with its own target, see Story 4.1.4b); `/` -> `/backlog` -> `/` x4; hard load `/backlog` reported **cold** (spec clears `sessionStorage`/`localStorage` and the in-memory store first) and **warm** (same-tab reload) separately so a hydrated cache cannot inflate the metric. A separate CDP Profiler pass reports `getBoundingClientRect` self-time.
  - *Given* the unmodified baseline, *When* the spec runs, *Then* it records baseline medians committed to `tests/e2e/perf/baseline.json` **before any Epic 2 story merges** (arm A of the three-arm comparison); the commit SHA is tagged `perf-baseline`.
- Per-method request-count assertions (numeric, in the spec and in the Story 7.1.3 blocking subset): `GetFeatureFlags <= 1`, `ListProgramsConfig <= 1`, `LogClientEvents <= ceil(duration_s / 5) + 1` for the scenario duration, `GetFileTransportCapabilities <= 1`, `ListWorkflows <= 1`, `GetVCSStatus <= 1 per session per mount`. Before the owning story lands the assertions are recorded as `expected-fail` against `baseline.json`, not skipped.
  - *Given* the fixture after Stories 2.1.3 and 2.1.5, *When* the load scenario runs for 16 s, *Then* `LogClientEvents <= 5` (= `ceil(16/5) + 1`, the same formula as the assertion above; baseline 56) and `ListProgramsConfig <= 1` (baseline 58).
- Noise rule (used by Story 5.5.1 criterion 2 and Story 1.1.3's own comparisons): every reported median carries the median absolute deviation (MAD) of its >= 5 kept runs; a claimed improvement counts only if it exceeds both the stated percentage and 2x the MAD expressed as a percentage of the median.
- Throttling is stated: within any comparison (baseline vs target, arm A vs B vs C) both sides use the same CPU throttle on the 20-session fixture, and both throttled (4x) and unthrottled medians are recorded for every scenario. Which of the two gates the window-switch budget is `OPEN-DECISION-throttle-gate`. The unthrottled 0.6-1.8 s figure in requirements was measured on empty windows and is not comparable to either fixture number.
- End conditions are the `ssq:*` measures, not `networkidle`.
  - *Given* a window switch, *When* the spec waits, *Then* it resolves on `performance.getEntriesByName("ssq:window-switch")` length growth.
**Files**: `tests/e2e/perf/frontend-responsiveness.spec.ts`, `tests/e2e/perf/fixture.ts`, `tests/e2e/perf/cpuProfile.ts`, `tests/e2e/perf/baseline.json`, `tests/e2e/pages/PerfPage.ts`, `tests/e2e/playwright.config.ts`

##### Task 1.1.3a: Seeded fixture + page helper (M)
- Build fixture seeding sessions/windows/backlog through RPCs against the isolated instance (shape: 20 sessions, 4 windows, w1 = 8 panes and w2-w4 = 4 panes; name it `perfFixtureShape` so every task cites one definition); add page helper under `tests/e2e/pages/`; fixture sets the `ssq:perf` opt-in and exposes `tmuxPaneDims(sessionName)` (runs `tmux -L <private socket> display-message -p -t <session> '#{pane_width}x#{pane_height}'` against the instance's `STAPLER_SQUAD_TMUX_SOCKET`; used by Task 4.1.4c).
- Files: `tests/e2e/perf/fixture.ts`, `tests/e2e/pages/PerfPage.ts`

##### Task 1.1.3b: Scenarios with User Timing + longtask observers (M)
- Port scenarios from `/tmp/ssqprof/prof.cjs` (copy into the repo; /tmp is volatile); `addInitScript` for `PerformanceObserver` longtask and `.xterm` mount counter; every scenario run at 4x CPU throttle and unthrottled, both recorded (which one gates the window-switch budget is `OPEN-DECISION-throttle-gate`); median over >= 5 kept runs; cold/warm and retained/cold-switch split; numeric per-method assertions.
- **SM6 (perceived speed)**: add a `PerformanceObserver({type: "event", durationThreshold: 16})` via `addInitScript` and record the p75 Event Timing duration (input to next paint, the INP definition) of the window-switch click, the leader-key switch and the Sessions -> Backlog nav click, plus whether the tab highlight painted in the first frame; target p75 <= 200 ms (requirements SM6, gated run per `OPEN-DECISION-throttle-gate`). Written to `baseline.json` before any Epic 2 story merges.
- **SM7 (terminal guard)**: add a `terminalEcho` scenario: type `echo __ssq_mark_<n>` through the focused terminal and time keydown dispatch to the first terminal-WebSocket frame containing the marker using Playwright `page.on("websocket")` `framereceived` timestamps (spec-side only, so it runs unchanged against the `perf-baseline` build in arm A and needs no app tap); >= 30 round trips per kept run while the window-switch workload runs; p95 recorded in `baseline.json` and compared in every later run under the SM7 rule (+10% and > 2x MAD). Task 5.5.1c keeps the in-page render-inclusive tap for B-versus-C only.
- Files: `tests/e2e/perf/frontend-responsiveness.spec.ts`

##### Task 1.1.3c: CDP profiler pass and baseline capture (S)
- Port `/tmp/ssqprof/cpu.cjs` aggregation to `cpuProfile.ts`; run separately from the timing pass; commit `baseline.json`; tag `perf-baseline`.
- Files: `tests/e2e/perf/cpuProfile.ts`, `tests/e2e/perf/baseline.json`

##### Task 1.1.3d: `perf` Playwright project, timeout override, `testIgnore` on `chromium` and `chromium-dom` (S)
- Files: `tests/e2e/playwright.config.ts`

##### Task 1.1.3e: CI feasibility spikes (S; runs before Task 1.1.3c commits `baseline.json`)
- (1) Seed the 20-session `perfFixtureShape` with real tmux in the CI runner; if it cannot, record the smaller fixture size in `baseline.json` metadata and keep thresholds on the same fixture across arms. (2) Prove `page.clock` (fake timers) against the Next router and the Bun router in the embedded production binary for the timing-based UX specs (leader 3 s timeout, 1 s `Refreshing`, 150 ms/1 s `RoutePending`); if it cannot, those specs fall back to a Jest fake-timer test plus a Playwright check with real elapsed time bounded by `expect.poll`. Write both outcomes to `research/ci-spikes.md`.
- Files: `research/ci-spikes.md`, `tests/e2e/perf/fixture.ts`

#### Story 1.1.4: Identify what held the connection slots
**Depends on**: 1.1.2 (gauge), 1.1.3 (perf spec).
**As a** maintainer, **I want** the holder of the 6 sockets during the 30 s stalls identified, **so that** Epic 5 can be proven to fix it.
**Acceptance Criteria**:
- With the gauge and Playwright `request`/`requestfinished` timing, a stall reproduction lists every request open > 5 s with method and initiator.
  - *Given* the restored 8-pane window, *When* the page loads on baseline, *Then* the report shows which requests were open when `queuedOver1s > 0` (expected: names of the 3-6 holders).
- Whether in-progress WebSocket handshakes count against the cap is recorded, and whether a WebSocket upgrade can open **while 6 HTTP calls are pending** (the bootstrap question for Epic 5).
  - *Given* 14 WebSockets opening while 6 HTTP calls are pending, *When* an extra HTTP call is issued, *Then* the note records whether it queued, with the Chrome version.
  - *Given* 6 pending HTTP calls (a handler that holds them open), *When* a new `WebSocket` is constructed, *Then* the note records the time to `open` and whether the upgrade queued behind the HTTP calls.
- **Server-phase capture.** During the stall reproduction, record per-RPC **server handler duration** and the number of **in-flight handlers**, and compare with the browser-side `requestStart - fetchStart` and `responseStart - requestStart` of the same request; also record how much of `ListShells`' wait is `ListInstanceData` (`resolveSessionTitle`, `session_service_shells.go:62-73`). The browser's own 30 s `DEFAULT_RPC_TIMEOUT_MS` abort (`transport.ts:18`) can produce status-0 / `blocked ~30000` entries for a request that waits 30 s on the server, so a status-0 entry alone does not prove browser queueing.
  - *Given* the stall reproduction on baseline, *When* the capture runs, *Then* `research/slot-holders.md` states, per stalled request, how much of its wait was browser-side (before `requestStart`) versus server-side (handler duration), the maximum concurrent handlers, and whether any handler wait p95 exceeded 1 s with fewer than 6 handlers in flight (the Story 5.1.2 server-side row).
- **WebSocket handshake serialization probe.** Chrome may serialize WebSocket handshakes per host (one connection in CONNECTING state per endpoint, RFC 6455 section 4.1; INFERRED, not tested), which would queue the boot-time unary socket behind the ~20 existing Watch/terminal stream sockets.
  - *Given* 14 stream sockets CONNECTING to the instance, *When* a 15th `WebSocket` (stand-in for `/api/ws/unary`) is constructed, *Then* the note records time-to-`open`, whether it queued behind the others, and the Chrome version; Task 5.4.1c's early-open ordering depends on this result.
**Files**: `project_plans/frontend-responsiveness/research/slot-holders.md`

##### Task 1.1.4a: Reproduce and log open requests (M)
- Extend the perf spec with a `stallProbe` scenario (including the socket-open-under-saturation probe and the WebSocket-handshake-serialization probe: open the unary stand-in socket while 14 stream sockets are CONNECTING); write findings.
- Files: `tests/e2e/perf/frontend-responsiveness.spec.ts`, `research/slot-holders.md`

##### Task 1.1.4b: Owner re-capture with DevTools closed (XS, wall-clock blocker)
- Owner exports HAR + `chrome://net-export` during a window-switch session; drop into `research/`. Blocker: owner tstapler.

##### Task 1.1.4c: Server-phase capture (S)
- First check whether a per-RPC server latency metric or in-flight handler count already exists (`/actuator`, middleware); reuse it. Otherwise add an opt-in timing interceptor (off by default; logs method, request id, start/end, in-flight count via `slog`) and a temporary timing log around `resolveSessionTitle`. The same opt-in is reused by Story 5.5.1 criterion 6. Record results in `research/slot-holders.md`.
- Files: `server/services/` (timing interceptor, only if none exists), `research/slot-holders.md`

#### Story 1.1.5: Attribute the `/backlog` hard-load cost and the `/` <-> `/backlog` return cost
**As a** maintainer, **I want** the 3.4 s hard-load long-task cost split into its parts before any Backlog fix is designed, **so that** Epic 6.1 fixes the real cost.
**Acceptance Criteria**:
- A CPU profile of a **cold** `/backlog` hard load (storage cleared) and of a `/` -> `/backlog` -> `/` return is split into: Redux store hydration (`backlogItemsSlice.upsertItem`), proto-to-domain mapping inside `useWatchBacklogItems`, render of N cards, and the ~20 page effects.
  - *Given* the seeded 200-item fixture, *When* the profile runs, *Then* `research/backlog-attribution.md` lists each bucket's self-time and share of the 3.4 s total, and states whether return navigation already hits a warm global store (INFERRED by the architecture review; this story verifies it).
- The measurement does not use a cached `/backlog` (the cold scenario clears storage; the warm scenario is reported separately) so a cache hit cannot inflate or mask the hard-load number.
  - *Given* a same-tab reload with storage intact, *When* measured, *Then* it is reported as `warm` and not mixed into the cold median.
- Story 6.1.2's scope is decided from this note (e.g. if render dominates, progressive render comes before hydration).
  - *Given* the note, *When* 6.1.1 starts, *Then* its `Depends on` line cites the committed note.
**Files**: `tests/e2e/perf/cpuProfile.ts`, `tests/e2e/perf/frontend-responsiveness.spec.ts` (backlog scenarios), `project_plans/frontend-responsiveness/research/backlog-attribution.md`
**Depends on**: 1.1.3

##### Task 1.1.5a: Profile, bucket and write the attribution note (M)
- Files: `tests/e2e/perf/cpuProfile.ts`, `research/backlog-attribution.md`

#### Story 1.1.6: Register the four feature flags (one early Go task)
**As a** maintainer, **I want** `request_coalescing`, `unary_over_ws`, `unary_over_ws_remote` and `window_keep_alive` to exist server-side with their defaults before any story reads them, **so that** no later story edits `feature_flag_service.go` and the default flips have a real file to change.
**Depends on**: none; must merge before Wave B.
**Acceptance Criteria**:
- `knownFeatureFlags` (`server/services/feature_flag_service.go:214`) gains four entries: `request_coalescing` (`defaultValue: true`), `unary_over_ws` (false), `unary_over_ws_remote` (false; description: "also use WebSocket unary on remote-access (https) origins; requires unary_over_ws"), `window_keep_alive` (false), each with a description naming its kill-switch role.
  - *Given* a fresh config, *When* `GetFeatureFlags` runs, *Then* the response reports `request_coalescing=true`, `unary_over_ws=false`, `unary_over_ws_remote=false`, `window_keep_alive=false`.
- `server/services/config_surface_proof_test.go` gains a `flag:<name>` entry for each, in the existing `exempt:` "client-only" form (`:207`: no Go reader; the web client gates on it and the server only persists/echoes it, naming the client file and its Jest test as they land); `pendingCeiling` stays 0.
  - *Given* the change, *When* `go test ./server/services -run ConfigSurface` runs, *Then* it passes (the harness at `config_surface_helpers_test.go:62-66` iterates every `knownFeatureFlags` entry).
- No frontend file defines the flags (`lib/features/features/flags.ts` is the RPC registry); the client reads them through `flagStore` (Task 2.1.4a).
**Files**: `server/services/feature_flag_service.go`, `server/services/config_surface_proof_test.go`

##### Task 1.1.6a: Register the four flags and their proof entries (S)
- Files: `server/services/feature_flag_service.go`, `server/services/config_surface_proof_test.go`

---

## Phase 2: Request-path fixes and render-path fixes (E2 and E3 in parallel)

### Epic 2.1: Request path (volume cuts before the transport work)
**Goal**: Cut duplicates so far fewer than 6 slots are needed, and put every unary caller behind one transport.

#### Story 2.1.0: `SessionDetailView` seam (lands before any other story edits that file)
**As a** maintainer, **I want** a no-op seam in `SessionDetailView.tsx` first, **so that** the five stories that touch it (2.1.6c/d, 3.1.1b, 4.1.3b/c/d) are small and orderable.
**Acceptance Criteria**:
- An optional `windowActive?: boolean` (default `true`) prop is plumbed `PaneSplitRenderer -> SessionDetail -> SessionDetailView` as a no-op, alongside the existing `isActive`/`isFocused`.
  - *Given* no caller passes `windowActive`, *When* the existing suites run, *Then* all pass unchanged.
- `useSessionDetailActivity({isFocused, windowActive})` (new file) returns `{ pollMode: "off"|"slow"|"fast", keysEnabled, measureEnabled }` with `pollMode = windowActive ? (isFocused ? "fast" : "slow") : "off"`, `keysEnabled = windowActive && isFocused`, `measureEnabled = windowActive`; with defaults it reproduces today's behavior (`useSessionVcs.ts:186` cadence, keys on focus).
  - *Given* `windowActive=true,isFocused=false`, *When* evaluated, *Then* `pollMode` is `"slow"` and `keysEnabled` is false (a visible unfocused pane keeps polling at the slow cadence).
- The window keydown at `SessionDetailView.tsx:665-676` (Escape, Shift+Arrow) moves into `useReviewNavKeys(keysEnabled, handlers)` with no behavior change: today the effect has no `isActive` in its deps (VERIFIED), so every mounted `SessionDetailView` handles Shift+Arrow. Task 2.1.0b therefore passes the **literal `true`**, not `useSessionDetailActivity().keysEnabled`; the hook's `keysEnabled = windowActive && isFocused` is wired in Task 4.1.3b, which is the deliberate behavior change (it also fixes the double `onNext`). Story 4.1.3d stays test-only for this handler.
  - *Given* the extracted hook with `keysEnabled=true`, *When* Shift+ArrowRight is pressed, *Then* `onNext` is called once as before.
  - *Given* two mounted `SessionDetailView`s where one is an unfocused pane, *When* 2.1.0 has landed, *Then* both handlers still fire exactly as on `main` (pinned by a test that Task 4.1.3b later inverts).
**Files**: `components/pane/PaneSplitRenderer.tsx`, `components/sessions/SessionDetail.tsx`, `components/sessions/SessionDetailView.tsx`, `components/sessions/useSessionDetailActivity.ts` (new), `components/sessions/useReviewNavKeys.ts` (new), tests

##### Task 2.1.0a: No-op `windowActive` prop plumbing + `useSessionDetailActivity` (S)
- Files: `components/pane/PaneSplitRenderer.tsx`, `components/sessions/SessionDetail.tsx`, `components/sessions/SessionDetailView.tsx`, `components/sessions/useSessionDetailActivity.ts`, test

##### Task 2.1.0b: Extract `useReviewNavKeys` (S)
- Files: `components/sessions/SessionDetailView.tsx`, `components/sessions/useReviewNavKeys.ts`, test

**Fixed edit order on `SessionDetailView.tsx`**: 2.1.0 -> 2.1.6c (FilesTab/hash) -> 3.1.1b (measurer, uses `measureEnabled`) -> 4.1.3b/c/d (flip the plumbed value, gate, tests). No two of these run in the same wave.

**Fixed edit order on every other shared file** (no two editors in the same wave; the later task rebases on the earlier):

| File | Order |
|---|---|
| `components/sessions/SessionDetail.tsx` | 2.1.0a -> 2.1.6a -> 4.1.3b |
| `components/pane/PaneSplitRenderer.tsx` | 2.1.0a -> 4.1.3b |
| `components/sessions/TerminalOutput.tsx` | 2.1.1d (transport migration) -> 2.1.3b (log rewire) -> 3.1.1b (measurer) |
| `lib/hooks/useSessionVcs.ts`, `lib/hooks/useVcsStatus.ts` | 2.1.6a -> 4.1.3c |
| `lib/hooks/useWorkflows.ts`, `lib/hooks/useBufferFreshness.ts` | 2.1.6d -> 4.1.3c |
| `components/sessions/XtermTerminal.tsx` | 3.1.2a -> 4.1.3e |
| `lib/transport/unary-ws-transport.ts` | 5.3.1b -> 5.3.1c -> 5.3.1d -> 5.4.1c |
| `app/page.tsx` | 4.1.4b -> 4.1.2b -> 4.1.5a |
| `app/RootShell.tsx` (the one file both the Next `layout.tsx` and the bun `main.tsx` render; `layout.tsx` is not edited) | 5.4.1c, 4.1.5a, 6.1.3b: serialized (never in the same wave) but **in any landing order**; each rebases on whichever landed first, so the small a11y fix 6.1.3b does not wait for the WS bootstrap or the keep-alive lift |
| `lib/features/flagStore.ts` | 2.1.4a -> 5.4.1c |

#### Story 2.1.1: Migrate the direct `createConnectTransport` callers
**Depends on**: 1.1.2 (first `lib/api/transport.ts` editor), 1.1.3 (`baseline.json` is committed before any Epic 2 story merges; Wave A).
**As a** maintainer, **I want** all unary callers on `getConnectTransport()`, **so that** coalescing, gauge, auth and the WS transport apply once.
**Acceptance Criteria**:
- The 37 non-test files from `grep -rl createConnectTransport web-app/src | grep -v -E 'gen/|\.test\.|__tests__|__mocks__'` (recounted 2026-10-26) split as: 2 stay (`lib/api/transport.ts`, `lib/transport/watch-ws-transport.ts`), 2 need no code change (`lib/utils/backoff.ts` mentions it in a comment only; `components/backlog/backlogItemDetailTestFixtures.ts` is a jest mock factory), and **33 migrate**. Plus `lib/hooks/useShells.ts`, which builds `createSessionWatchTransport` for unary and is migrated separately (34 edits). After migration the grep returns only those 4 files. (66 test files also reference it and are out of scope unless they mock the module.)
  - *Given* the migrated tree, *When* the grep runs, *Then* it lists exactly `lib/api/transport.ts`, `lib/transport/watch-ws-transport.ts`, `lib/utils/backoff.ts`, `components/backlog/backlogItemDetailTestFixtures.ts`.
- The codemod runs first with a dry-run count that must equal the pinned list (33 files) before any batch merges; callers that pass a custom `baseUrl`, `useBinaryFormat`, interceptors, or `fetch` are listed by the dry run and handled by the factory options, not skipped.
  - *Given* the dry run, *When* it reports a count other than 33 callers, *Then* the plan's pinned list is corrected before batch work starts.
- Callers needing different timeouts use a documented option on the shared factory (e.g. `getConnectTransport({ timeoutMs })`, memoized by `{baseUrl, timeoutMs}`).
  - *Given* `app/backlog/page.tsx:359` previously built a transport with a custom base URL, *When* migrated, *Then* its calls resolve through the shared transport and `bunx jest` for the backlog tests passes.
- No migrated caller loses timing. Today the shared transport carries **only** the auth interceptor and `defaultTimeoutMs` (`lib/api/transport.ts:32-39`, VERIFIED); `createRpcTimingInterceptor(analytics)` is wired per hook in exactly two places (`lib/hooks/useShells.ts:59`, `lib/hooks/useImportSessionService.ts:57`) because it needs the React analytics context. Task 2.1.1a therefore adds a module-level rpc-timing sink (`setRpcTimingSink(analytics)`, registered once by the analytics provider; no-op until set) and installs `createRpcTimingInterceptor` on the shared factory reading that sink; the two hooks drop their per-hook wiring when they migrate. This also gives every other migrated caller `rpc:<Method>` timings for the first time.
  - *Given* an `AnalyticsContext`, *When* `listShells` runs through the shared transport, *Then* an `rpc:ListShells` measure still exists; and a Jest test asserts the shared transport carries the rpcTiming interceptor and that `useShells`/`useImportSessionService` no longer build their own.
- The ESLint ban (Story 7.1.1) lands in the **same PR** as the last migration batch (Task 2.1.1e), not in Wave E, so no regrowth occurs between waves.
**Files (pinned)**: batch b: `app/backlog/page.tsx`, `app/unfinished/UnfinishedTab.tsx`, `components/backlog/{BacklogItemDetail,BacklogItemPanel,JulesDispatchDialog,ReviewChangesModal}.tsx`, `components/unfinished/{CommitPushModal,GitHubPRsSection,UnfinishedItemDetail,WorktreeDiffModal}.tsx`, `lib/hooks/{useBacklogCreationNotifier,useStuckBacklogItems}.ts` (12); batch c: `app/config/ConfigPageContent.tsx`, `app/history/page.tsx`, `components/errors/ErrorDashboard.tsx`, `components/onboarding/OnboardingModal.tsx`, `components/sessions/{DetectionEventsPanel,HookStatusPanel,TagEditor,TerminalOutput,WorkspaceSwitchModal}.tsx`, `components/settings/{AliasesManager,BackgroundModelsSettings,DirectoryRulesManager,GlobalDefaultsForm,JulesSettings,LLMBackendSettings,ProfilesManager,SlackNotificationSettings,TaggingClassifierSettings}.tsx`, `components/system/TmuxVersionMismatchBanner.tsx`, `components/ui/DebugMenu.tsx`, `lib/hooks/{useImportSessionService,useShells}.ts` (22); factory `lib/api/transport.ts`.

##### Task 2.1.1a: Extend shared factory options (S)
- Add `getConnectTransport(opts?)` memoized by `{baseUrl, timeoutMs}`; keep default singleton behavior; add the rpc-timing sink + interceptor (see the AC above). Second of the `lib/api/transport.ts` editors (1.1.2a -> 2.1.1a -> 2.1.2b -> 2.1.9a -> 5.4.1b).
- Files: `lib/api/transport.ts`, `lib/telemetry/rpcTiming.ts`, `lib/api/__tests__/transport.test.ts` (exists today; the earlier path `lib/api/transport.test.ts` did not)

##### Task 2.1.1b: Codemod with dry-run count (S)
- `web-app/scripts/codemods/connect-transport-to-shared.mjs` modeled on `next-nav-to-facade.mjs`; dry-run must report the 33 callers and flag non-default options.
- Files: `web-app/scripts/codemods/connect-transport-to-shared.mjs` (new)

##### Task 2.1.1c: Migrate batch b (backlog, unfinished) (M)
- The 12 pinned files; run affected Jest. Must merge before Task 6.1.1a starts (it edits `app/backlog/page.tsx:356-362`, the `rawClientRef` transport effect).
- Files: the batch-b list above

##### Task 2.1.1d: Migrate batch c (settings, sessions, config, hooks incl. `useShells`) (M)
- The 22 pinned files. First editor of `components/sessions/TerminalOutput.tsx`: Task 2.1.3b waits for this task.
- Files: the batch-c list above

##### Task 2.1.1e: ESLint ban in the final batch PR (S)
- Add the Story 7.1.1 rule in the same PR as 2.1.1d, with the test-file override (66 test files still reference `createConnectTransport`).
- Files: `web-app/eslint.config.mjs`

#### Story 2.1.2: Coalescing interceptor for identical idempotent unary reads
**As an** operator, **I want** identical in-flight reads merged, **so that** 14 identical calls become 1.
**Acceptance Criteria**:
- `CoalescingInterceptor` shares one in-flight promise per `RpcDedupeKey` for methods whose `coalescible` column is true in `IdempotentMethodTable`; mutations and unknown methods are never coalesced.
  - *Given* 14 concurrent `getFeatureFlags({})` calls, *When* they start in the same tick, *Then* exactly 1 fetch is issued and all 14 resolve with equal responses.
- One caller's abort does not cancel the others (subscriber ref-count; the shared request uses its own timeout, not any caller's signal).
  - *Given* two callers of `getVCSStatus({sessionId:"s1"})` and caller A aborts, *When* the response arrives, *Then* caller B resolves and A rejects with `Code.Canceled`.
- A read never joins a flight that started before the latest mutation that could affect it. Two generation counters feed the join check: a per-`sessionId` counter bumped by any non-`coalescible` method carrying that id (commit, save, file op), and a **global** counter bumped by every non-`coalescible` method (carrying a session id or not, e.g. `UpdateFeatureFlag`, config writes). Session-scoped reads compare both; sessionless reads (`GetFeatureFlags`, `ListProgramsConfig`, `ListWorkflows`, `ListShells`, `GetFileTransportCapabilities`) compare the global counter. Over-invalidation is accepted (a mutation briefly stops joins; it never returns stale data).
  - *Given* a `getVCSStatus({sessionId:"s1"})` in flight, *When* a `writeFile`/commit for `s1` is issued and then a second `getVCSStatus({sessionId:"s1"})` starts, *Then* the second issues its own fetch and does not receive the pre-write response.
  - *Given* a `getFeatureFlags({})` in flight, *When* `updateFeatureFlag` is issued and then a second `getFeatureFlags({})` starts, *Then* the second issues its own fetch (this is what makes Story 2.1.4's `setFlag` read-back AC hold; the provider's `minAcceptable` ordering guard at `FeatureFlagsContext.tsx:100-110` remains as a second layer).
- Failures are not cached; the next call re-fetches.
  - *Given* a shared call that rejects with `Unavailable`, *When* a new call follows, *Then* a new fetch is issued.
- Requests with different args never share.
  - *Given* `listFiles({path:"."})` and `listFiles({path:"docs"})` concurrently, *When* both start, *Then* 2 fetches are issued.
- Flag `request_coalescing` (registered in Task 1.1.6a; fixed pre-load default ON, not persisted; OFF-switch retained) disables it. It is read through `flagStore` (Task 2.1.4a), so Task 2.1.2b waits for Task 2.1.4a. `GetFeatureFlags` is itself `coalescible`, so the flag fetch is safe before the flag is known.
  - *Given* the flag OFF, *When* 2 identical reads start, *Then* 2 fetches are issued.
- Subscribers of a shared flight never share one mutable object: protobuf-es messages are mutable, so a consumer that mutates its result (or edits after `toBinary`) would corrupt every other subscriber of the 14 `getFeatureFlags` / N `getFileContent` flight. Each subscriber receives its own `clone(schema, msg)` (`@bufbuild/protobuf` 2.11.0, `dist/esm/clone.d.ts:6`, VERIFIED) of the decoded response; the first subscriber may receive the original.
  - *Given* two subscribers of one flight and subscriber A mutates a field of its result, *When* B reads its result, *Then* B's value is unchanged and the two results are not `===`.
**Depends on**: 1.1.6 (flag registered), 2.1.4a (flagStore), 2.1.1a (factory).
**Files**: `lib/api/coalescingInterceptor.ts` (new), `lib/api/idempotentMethods.ts` (new), `lib/api/transport.ts`, `lib/api/coalescingInterceptor.test.ts`

##### Task 2.1.2a: `IdempotentMethodTable` + key builder (M)
- `idempotency_level` is **absent** from `proto/session/v1/session.proto` (`grep -c idempotency_level` = 0, VERIFIED), so the table is an explicit, hand-maintained list, not derived from the proto. Columns: `coalescible` (10 methods: GetFeatureFlags, ListProgramsConfig, GetVCSStatus, GetSessionDiff, ListFiles, GetFileContent, StatFile, GetFileTransportCapabilities, ListShells, ListWorkflows; all exist in `proto/session/v1/session.proto`), `replayable` (= coalescible), `wsEligible` (**9 methods, enumerated**: GetVCSStatus, GetSessionDiff, ListFiles, GetFileContent, StatFile, GetFileTransportCapabilities, ListShells, ListWorkflows, ListProgramsConfig; excludes `GetFeatureFlags` and every mutation; `ListSessions` is deliberately not included). `GetFileContent` is the one large-response read on the socket and is what the Story 5.5.1 head-of-line probe exercises; its request is two short strings, and the write-generation guard (keyed on `sessionId`) makes coalescing it safe. The same nine names are the Go-side `wsUnaryAllowed` set and live in the shared fixture `web-app/src/lib/transport/__fixtures__/ws-unary-allowlist.json` (`wsUnaryAllowed`, `clientHeaders`, `maxInFlight`: 6), **created by this task** so the assertion below has its fixture (Task 5.1.1b only reuses it and wires the Go loader; this task has no prerequisite beyond Wave A, so 5.1.1b is not delayed); `cacheable` is a separate list (flags, programs, capabilities) so adding a method to one purpose never enables another. A Jest test asserts every listed method exists in the generated service descriptor (`gen/`), that `replayable`/`wsEligible` are subsets of `coalescible`, and that `wsEligible` equals the fixture's `wsUnaryAllowed` set exactly in this epic (client set a subset of server set is the standing rule; equality is the starting state). `buildDedupeKey(method, message)`.
- Files: `lib/api/idempotentMethods.ts`, `lib/api/idempotentMethods.test.ts`, `lib/transport/__fixtures__/ws-unary-allowlist.json` (new)

##### Task 2.1.2b: Interceptor with ref-counted abort, session + global write-generation guards + tests (M)
- Implement per Acceptance Criteria (including per-subscriber `clone` and its mutation test); export `resetCoalescingForTest`. Third `lib/api/transport.ts` editor; starts after Task 2.1.4a. The coalescer wraps the policy transport outermost (ADR-002 item 7).
- Files: `lib/api/coalescingInterceptor.ts`, `lib/api/coalescingInterceptor.test.ts`, `lib/api/transport.ts`

#### Story 2.1.3: Fix console-patch chaining and batch LogClientEvents once
**Depends on**: Task 2.1.1d (`TerminalOutput.tsx` edit order).
**As an** operator, **I want** one console patch and one log flusher, **so that** `LogClientEvents` drops from 56 per 16 s toward 1 per flush interval and entries stop duplicating 14x. Numeric e2e check lives in Story 1.1.3 (`LogClientEvents <= ceil(duration_s / 5) + 1`).
**Acceptance Criteria**:
- A refcounted `ConsolePatch` installs interceptors once regardless of mounted `TerminalOutput` count and unwinds in correct order on last release.
  - *Given* 14 mounted `TerminalOutput` instances, *When* `console.log("x")` runs once, *Then* exactly 1 entry "x" is enqueued (baseline 14).
- One `LogBuffer` flushes at 50 entries or 5 s and on `pagehide`/`visibilitychange:hidden` via `getKeepaliveConnectTransport` (64 KB cap respected); `console.error`/`onerror`/`unhandledrejection` capture is preserved; the flusher never logs.
  - *Given* 3 buffered entries, *When* `pagehide` fires, *Then* one keepalive `logClientEvents` request carries all 3.
- The sessionId tag follows the last-focused session.
  - *Given* sessions `s1` then `s2` focused, *When* a log line is emitted after focusing `s2`, *Then* it is tagged `s2`.
**Files**: `lib/hooks/useBrowserLogStream.ts`, `lib/telemetry/logBuffer.ts` (new), `components/sessions/TerminalOutput.tsx`, tests (the e2e count assertion is in `tests/e2e/perf/` and the Story 7.1.3 blocking subset)

##### Task 2.1.3a: LogBuffer + ConsolePatch modules with tests (M)
- Extract from `useBrowserLogStream.ts:33-165`; reentrancy guard; `resetLogBufferForTest`.
- Files: `lib/telemetry/logBuffer.ts`, `lib/telemetry/logBuffer.test.ts`

##### Task 2.1.3b: Rewire `useBrowserLogStream` and `TerminalOutput` (S)
- Starts after Task 2.1.1d (which already migrated `TerminalOutput.tsx`'s transport). Hook becomes thin subscriber (acquire/release patch, set focused sessionId); single `pagehide` beacon.
- Files: `lib/hooks/useBrowserLogStream.ts`, `components/sessions/TerminalOutput.tsx`, `lib/hooks/useBrowserLogStream.test.ts`

#### Story 2.1.4: Unify the two feature-flag fetchers
**As an** operator, **I want** one flag store, **so that** a page load issues at most 1 `GetFeatureFlags`. The store also persists the last-known `unary_over_ws` and `unary_over_ws_remote` values to `localStorage` (versioned keys `ssq:flag:unary_over_ws`, `ssq:flag:unary_over_ws_remote`) for the Story 5.4.1c bootstrap, removing each key when a successful response omits the flag or reports it OFF, and persists `window_keep_alive` (`ssq:flag:window_keep_alive`, `"1"`/`"0"`, removed when the response omits the flag) so the first render mounts the right container shape with no remount; both are exposed synchronously before the first fetch returns, with the shipped default constant as the fallback when no key exists. `request_coalescing`'s pre-load default is the fixed constant ON (not persisted).
**Acceptance Criteria**:
- `FeatureFlagsContext` and `useFeatureFlagValue` read one module-level store with a 30 s TTL and shared in-flight promise; the seq/minAcceptable write guard and keep-last-known-on-failure semantics are preserved, with default-OFF-until-loaded kept for every flag that has no pre-load constant (the four new flags use the constants and persisted values defined in Risk Control); `setFlag` invalidates the store.
  - *Given* the provider and 5 `useFeatureFlagValue` consumers mounting together, *When* the page loads, *Then* exactly 1 `getFeatureFlags` call is issued (baseline 14).
  - *Given* `setFlag("x", true)` then a read-back, *When* a hook consumer reads `x`, *Then* it returns `true` immediately (not stale for 30 s).
- The root cause from Story 1.1.1 is fixed or documented as covered (e.g. duplicate provider instances removed).
  - *Given* the findings note names a cause, *When* this story completes, *Then* a Jest test reproduces that cause and passes.
**Files**: `lib/contexts/FeatureFlagsContext.tsx`, `lib/hooks/useFeatureFlagValue.ts`, `lib/features/flagStore.ts` (new), tests
**Depends on**: 1.1.6 (pure extraction; Task 2.1.4a must land before Task 2.1.2b).

##### Task 2.1.4a: flagStore module + tests (M)
- Extract TTL/in-flight logic from `useFeatureFlagValue.ts:15-40`; export `resetFlagStoreForTest`; keep `resetFeatureFlagValueCache` as alias.
- Files: `lib/features/flagStore.ts`, `lib/features/flagStore.test.ts`

##### Task 2.1.4b: Re-point provider and hook (M)
- Depends on Task 2.1.4a and Task 1.1.1b (the Story 1.1.1 root cause). Provider calls store with ordering guard; hook subscribes; remove per-hook 30 s `setInterval` in favor of one store timer.
- Files: `lib/contexts/FeatureFlagsContext.tsx`, `lib/hooks/useFeatureFlagValue.ts`, existing tests

#### Story 2.1.5: Cache `useAvailablePrograms`
(`ListProgramsConfig <= 1` is asserted numerically in the perf spec, Story 1.1.3.)
**Acceptance Criteria**:
- Module-level 30 s cache with shared in-flight promise; the `/api/server-info` fallback is also single-flight; failure is not cached.
  - *Given* 58 mounted consumers (rows, headers, details), *When* the page loads, *Then* 1 `listProgramsConfig` call is issued (baseline 58).
  - *Given* `listProgramsConfig` rejects, *When* 10 consumers mount, *Then* 1 `/api/server-info` request is issued.
**Files**: `lib/hooks/useAvailablePrograms.ts`, `lib/hooks/useAvailablePrograms.test.ts`

##### Task 2.1.5a: Cache + single-flight fallback (S)
- Copy the `useFeatureFlagValue` pattern; add `resetAvailableProgramsForTest`.
- Files: `lib/hooks/useAvailablePrograms.ts`, `lib/hooks/useAvailablePrograms.test.ts`

#### Story 2.1.6: Per-pane request dedupe (VCS, diff, shells, workflows, file transport caps, FilesTab, hash)
**As an** operator, **I want** a window mount to cost a handful of requests instead of ~13 per pane, **so that** slots stay free.
**Acceptance Criteria**:
- `useSessionVcs` reads the prefetch cache and shares the in-flight `prefetchVcsStatus` promise (the cache gets in-flight sharing).
  - *Given* a `SessionDetail` mount, *When* it mounts, *Then* exactly 1 `getVCSStatus` per session is issued (baseline 2).
- `GetFileTransportCapabilities` is cached at module level keyed by `capsEpoch`.
  - *Given* 8 mounted `FilesTab`s, *When* they mount, *Then* 1 capabilities call is issued (baseline 8).
- `FilesTab` mounts its content only once its tab has been active (lazy first mount, then retained).
  - *Given* a `SessionDetailView` on the terminal tab, *When* it mounts, *Then* 0 `listFiles` calls are issued until the user opens the Files tab.
- The `#file=` hash seeds only the focused pane's `filesSelection`.
  - *Given* `#file=docs/x.html` and 8 panes, *When* the page loads, *Then* `getFileContent` for that path is issued at most once (baseline 8) and only for the focused pane's session.
- `useBufferFreshness` (`lib/hooks/useBufferFreshness.ts`) does not poll `StatFile` for an inactive tab/hidden surface.
  - *Given* the terminal tab is active, *When* 30 s pass on a fake clock, *Then* 0 `statFile` calls are made.
- `ListWorkflows` reads a shared store seeded by `OmnibarContext` instead of one fetch per detail view, keeping the 90 s/visibility backstop once globally.
  - *Given* 8 `SessionDetailView`s, *When* mounted, *Then* 1 `listWorkflows` call is issued (baseline 8).
- Unmount aborts or ignores results for `FileTree.loadDirectory` and the programs fallback (no setState after unmount).
  - *Given* a pending `listFiles` and an unmount, *When* the response arrives, *Then* no state update occurs and no React warning is logged.
**Files**: `lib/hooks/useSessionVcs.ts`, `lib/hooks/useVcsStatus.ts`, `components/sessions/SessionDetail.tsx`, `lib/hooks/useFileTransport.ts`, `components/sessions/SessionDetailView.tsx`, `components/sessions/FileTree.tsx`, `lib/hooks/useFilesDeepLink.ts`, `lib/hooks/useWorkflows.ts`, `lib/hooks/useBufferFreshness.ts`, tests

##### Task 2.1.6a: VCS prefetch cache with in-flight sharing; `useSessionVcs` reads it (M)
- Files: `lib/hooks/useVcsStatus.ts`, `lib/hooks/useSessionVcs.ts`, `components/sessions/SessionDetail.tsx`, tests

##### Task 2.1.6b: Capabilities module cache (S)
- Files: `lib/hooks/useFileTransport.ts`, test

##### Task 2.1.6c: Lazy FilesTab mount + hash scoping (M)
- Depends on Task 2.1.0 (seam); first story after it to edit `SessionDetailView.tsx`. `SessionDetailView.tsx:1213` conditional-on-first-activation; `:493-495` focused-only; keep `useFilesDeepLink` reveal working when the tab opens.
- Files: `components/sessions/SessionDetailView.tsx`, `lib/hooks/useFilesDeepLink.ts`, `components/sessions/FileTree.tsx`, tests

##### Task 2.1.6d: Workflows shared store + freshness gating + abort on unmount (M)
- Files: `lib/hooks/useWorkflows.ts`, `lib/contexts/OmnibarContext.tsx`, `lib/hooks/useBufferFreshness.ts`, tests

#### Story 2.1.7: Epic 2 exit assertions for metric 1
**As the** owner, **I want** the unary-blocking metric asserted numerically as soon as Epic 2 is done, **so that** metric 1 has a standing test independent of the WS arms.
**Depends on**: 2.1.1 through 2.1.6, 1.1.3.
**Acceptance Criteria**:
- The perf spec's load and window-switch scenarios assert `queuedOver1s == 0` and status-0 count `== 0` (the Story 1.1.2 definitions), at the same 4x throttle on the 20-session fixture, over >= 5 kept runs; both numbers are also written to `research/ws-gate.md` as arm B.
  - *Given* Epic 2 merged, *When* `--project=perf` runs, *Then* both assertions pass or the failure names which scenario and method held the queue.
- Remediation path if this fails: the Story 5.1.2 table's second row fires (Epic 5 becomes top priority, Task 5.4.1c mandatory); if arm C in Story 5.5.1 then also fails criterion 1, apply the Story 5.5.1 remediation list. This assertion starts as `expected-fail` against `baseline.json` until Epic 2 merges, and in CI joins Story 7.1.3: the status-0 assertion is in the blocking count-based subset, `queuedOver1s == 0` stays advisory there (timing-dependent); the perf spec itself still hard-fails on both.
**Files**: `tests/e2e/perf/frontend-responsiveness.spec.ts`

##### Task 2.1.7a: Add the numeric `queuedOver1s`/status-0 assertions (S)
- Files: `tests/e2e/perf/frontend-responsiveness.spec.ts`

#### Story 2.1.8: Server-side contention fix (conditional on the Story 1.1.4 server-phase capture)
**As the** operator, **I want** the cheap `ListShells` handler not to read the whole session list on every call, **so that** server-side lock/DB contention does not hold the calls the browser sees as "blocked".
**Depends on**: Task 1.1.4c finding. Fires only when the Story 5.1.2 table's server-side row fires; no other story waits on it. This is the one in-scope exception to the requirements "Out of Scope" backend line (`requirements.md`), justified by evidence only.
**Acceptance Criteria**:
- `resolveSessionTitle` (`server/services/session_service_shells.go:62-73`, called by all five shell RPCs, e.g. `ListShells` at `:218`) resolves a live session without `s.storage.ListInstanceData()`: look up the live instance by title, then by UUID, in memory, and fall back to the storage scan only on a miss.
  - *Given* a live session and 8 concurrent `ListShells` calls against a counting storage fake, *When* they run, *Then* `ListInstanceData` is called 0 times; *Given* a stored but not live session, *Then* resolution still succeeds via the fallback.
- The before/after server handler p95 for `ListShells` (same fixture, same capture as Task 1.1.4c) is written to `research/slot-holders.md`; if the p95 does not fall, the note states that `ListInstanceData` was not the contention source and names the next suspect (lock, DB, exec gate) with a follow-up.
- **SM1 fallback acceptance** (requirements SM1): when this story fires, metric 1 is done when this story has landed, `queuedOver1s == 0` and status-0 `== 0` hold for the `critical` request class in the Story 2.1.7 exit run (the `deferred` class is capped at 3 concurrent by Story 2.1.9), and the server handler wait p95 for `ListShells` and `GetVCSStatus` is <= 1 s with fewer than 6 handlers in flight, or the before/after p95 plus a named next suspect is recorded (for `GetVCSStatus` the open bug `docs/bugs/open/BUG-020-vcs-status-diff-mutex-contention.md`: git subprocess under a mutex). Anything less is reported under `RULE-target-miss`.
**Files**: `server/services/session_service_shells.go`, `server/services/session_service_shells_test.go`

##### Task 2.1.8a: In-memory session title resolution + counting-fake test (S)
- Files: `server/services/session_service_shells.go`, `server/services/session_service_shells_test.go`

#### Story 2.1.9: Request-level priority and deferral of non-critical calls
**As the** operator, **I want** critical calls (flags, the focused pane, the visible list) never to wait behind background reads, **so that** the requirements' "priority/deferral of non-critical calls" is met at the request layer and not only by removing duplicates.
**Depends on**: 2.1.2b (interceptor chain; this story is the fifth `lib/api/transport.ts` editor before 5.4.1b), 2.1.6 (call-site inventory).
**Acceptance Criteria**:
- A named classification in `lib/api/requestPriority.ts`: `critical` = `GetFeatureFlags`, `ListSessions`, `ListBacklogItems` while on `/backlog`, `GetVCSStatus`/`GetFileContent` for the focused pane, and every user-initiated mutation; `deferred` = `ListShells`, `ListWorkflows`, `GetFileTransportCapabilities`, `ListProgramsConfig`, `StatFile`, `LogClientEvents`, and `GetVCSStatus`/`ListFiles` for panes that are not focused or not visible. Unknown methods default to `critical` (fail safe).
  - *Given* a method not in the table, *When* called, *Then* it is treated as `critical`.
- Mechanism: the priority travels as a Connect `contextValues` call option (`CallOptions.contextValues`, `@connectrpc/connect` 2.1.1 `call-options.d.ts:33`, VERIFIED) read by a priority interceptor on the policy transport (outermost, applied once per ADR-002 item 7). `deferred` calls are released after the route's first paint (first animation frame after load, idle wait capped at 2 s) and at most `DEFERRED_MAX_CONCURRENT = 3` run at once, leaving at least 3 of the 6 HTTP/1.1 slots for `critical` calls; a deferred call is never held longer than the 2 s cap plus queue time (no starvation).
  - *Given* 12 deferred calls and 1 critical call issued in one tick, *When* the browser cap is 6, *Then* the critical call starts immediately and at most 3 deferred calls are in flight.
- The perf spec reports `queuedOver1s` per priority class and asserts it is 0 for `critical` in the load and window-switch scenarios.
  - *Given* the seeded fixture, *When* the load scenario runs, *Then* no `critical` call has `requestStart - fetchStart` > 1000 ms.
**Files**: `lib/api/requestPriority.ts` (new), `lib/api/requestPriority.test.ts` (new), `lib/api/transport.ts`, call sites tagged in Task 2.1.9b (`lib/hooks/useShells.ts`, `lib/hooks/useWorkflows.ts`, `lib/hooks/useFileTransport.ts`, `lib/hooks/useAvailablePrograms.ts`, `lib/hooks/useBufferFreshness.ts`, `lib/hooks/useVcsStatus.ts`, `lib/hooks/useSessionVcs.ts`)

##### Task 2.1.9a: Priority table, `contextValues` key, priority interceptor with concurrency cap (M)
- Fifth `lib/api/transport.ts` editor in the order 1.1.2a -> 2.1.1a -> 2.1.2b -> 2.1.9a -> 5.4.1b.
- Files: `lib/api/requestPriority.ts`, `lib/api/requestPriority.test.ts`, `lib/api/transport.ts`

##### Task 2.1.9b: Tag the deferred call sites; per-class `queuedOver1s` in the perf spec (S)
- Hook files edited after Tasks 2.1.6a/d (same files); perf-spec assertion added to `tests/e2e/perf/frontend-responsiveness.spec.ts`.
- Files: the hook files listed in the story, `tests/e2e/perf/frontend-responsiveness.spec.ts`

### Epic 3.1: Render path
**Goal**: Remove the measured forced-layout hotspot and stop hidden-surface rAF/observer work.

#### Story 3.1.1: Replace per-instance body observers with `SharedBodyMeasurer`
**As an** operator, **I want** layout measurement to run once per frame, **so that** `getBoundingClientRect` self-time drops >= 80% versus the committed `baseline.json` from the same harness (the 2,614 ms DevTools-attached trace figure motivates the story but is not the comparison base, because DevTools inflates it).
**Acceptance Criteria**:
- One body MutationObserver + ResizeObserver + resize listener serves all subscribers; each subscriber's element is measured at most once per frame; `publish` is skipped when the rounded value is unchanged (set-if-changed in `lib/utils/toastDock.ts`).
  - *Given* 8 mounted `SessionDetailView`s and 100 DOM insertions in one frame, *When* the frame runs, *Then* `getBoundingClientRect` is called at most (number of subscribers) times and `publish` at most once per changed value.
- Subscribers with `enabled=false` are not subscribed; `enabled` is fed `measureEnabled` (= `windowActive`) from `useSessionDetailActivity`, **not** pane focus, so the toast dock keeps tracking all visible panes (`usePublishStackTopOffset.ts:14` already has an `enabled` argument).
  - *Given* a `SessionDetailView` in a hidden window (`windowActive=false`), *When* DOM mutates, *Then* its element is never measured; *Given* a visible but unfocused pane, *Then* it is still measured.
- No feedback loop: publishing an unchanged value causes no further scheduled frame.
  - *Given* a stable layout, *When* 60 frames pass, *Then* `measure` runs 0 times after the first settled pass.
- Perf spec delta: `getBoundingClientRect` self-time reduced by >= 80% vs `baseline.json`.
  - *Given* the seeded fixture, *When* the CPU-profile pass runs, *Then* self-time <= 20% of the committed baseline.
**Depends on**: 2.1.0 (seam), 1.1.3
**Files**: `lib/hooks/usePublishStackTopOffset.ts`, `lib/utils/toastDock.ts`, `components/sessions/SessionDetailView.tsx:465`, `components/sessions/TerminalOutput.tsx:129`, `components/sessions/files/MobileBars.tsx:68`, `lib/layout/sharedBodyMeasurer.ts` (new), tests

##### Task 3.1.1a: `SharedBodyMeasurer` with unit tests (M)
- Files: `lib/layout/sharedBodyMeasurer.ts`, `lib/layout/sharedBodyMeasurer.test.ts`

##### Task 3.1.1b: Rewire `usePublishBottomEdge` + gate on `measureEnabled`; equality in `toastDock` (M)
- Files: `lib/hooks/usePublishStackTopOffset.ts`, `lib/utils/toastDock.ts`, `components/sessions/SessionDetailView.tsx`, `components/sessions/TerminalOutput.tsx`, `components/sessions/files/MobileBars.tsx`

#### Story 3.1.2: Gate remaining rAF/ResizeObserver/sampler work for hidden surfaces
**Acceptance Criteria**:
- `TerminalPool.fitOnShow` retry loop stops when its anchor is hidden or detached and re-arms on show (cap `FIT_ON_SHOW_MAX_ATTEMPTS` retained).
  - *Given* a docked terminal whose anchor is `display:none`, *When* 300 frames pass, *Then* `getBoundingClientRect` is called 0 times after the first hidden detection.
- `XtermTerminal` skips `fit()` and never sends a 0-col resize while hidden (ADR-002 of `terminal-resize-fit-loop`).
  - *Given* `isVisible=false`, *When* a ResizeObserver delivers 0x0, *Then* no `resizeSession` RPC is sent and `XtermTerminal.refit.test.tsx` still passes.
- `SessionList.tsx:689` `measureElement` uses ResizeObserver entry sizes instead of a forced rect read where possible.
  - *Given* 200 rows, *When* the list renders, *Then* `getBoundingClientRect` calls in `SessionList` are 0 on the steady-state path.
- **Server-side zero-size guard.** Keep-alive makes hidden terminals routine (`display:none`/`inert`, remount on reactivation), so a 0x0 or tiny handshake must never reach tmux. Today only `handleCurrentPaneRequest` rejects `<= 0` (`server/services/connectrpc_websocket.go:2558`); `performInitialResizeNudge` (`:1160-1180`, nil check only) and the capture-pane nudge (`:3237-3262`, nil check only) pass `targetCols/targetRows` straight to `ResizePTY` -> `SetWindowSize` (`session/instance_tmux.go:1285-1292`), and `resize.WithForcedRedraw` skips only the `cols-1` step for `cols <= 1` then still sets the real size (`session/resize/nudge.go:27-32`); other callers are `session_service_stream_terminal.go:465` and `connectrpc_websocket.go:3508`. Both nudge paths reject non-positive or implausibly small dimensions (`cols < 2 || rows < 1`; the minimum is a chosen guard, INFERRED) with the same early return used for nil, and `Instance.ResizePTY` returns an error for `cols <= 0 || rows <= 0` as defense in depth for every caller.
  - *Given* handshake dimensions 0x0, 0xN and 1x1, *When* `performInitialResizeNudge` and the capture-pane nudge run, *Then* `ResizePTY` is not called on a fake target; *Given* `Instance.ResizePTY(0, 24)`, *Then* it returns an error and `SetWindowSize` is not reached; existing resize tests pass unchanged.
- **Client handshake guard.** The stream handshake builds `targetCols/targetRows` in `lib/hooks/useTerminalStream.ts:325-387` (and the resync path `lib/hooks/useTerminalFlowControl.ts:205,361`); it must not send dimensions taken from a container with zero `clientWidth/clientHeight`, and defers the handshake dimensions until `fitOnShow` reports a real size. This extends the `resizeSession` AC above, which covers only the unary resize RPC.
  - *Given* `isVisible=false` (container 0x0), *When* the stream connects, *Then* no `targetCols/targetRows` are sent at 0x0 (the handshake waits for a fit or is sent without dimensions), asserted in Jest.
**Files**: `lib/terminal/TerminalPool.tsx`, `components/sessions/XtermTerminal.tsx`, `components/sessions/useTerminalSizing.ts`, `components/sessions/SessionList.tsx`, `lib/hooks/useTerminalStream.ts`, `lib/hooks/useTerminalFlowControl.ts`, `server/services/connectrpc_websocket.go`, `session/instance_tmux.go`, tests

##### Task 3.1.2a: `fitOnShow` and `XtermTerminal` hidden gating (M)
- Files: `lib/terminal/TerminalPool.tsx`, `components/sessions/XtermTerminal.tsx`, `components/sessions/XtermTerminal.refit.test.tsx`

##### Task 3.1.2b: `useTerminalSizing` and `SessionList` use observer entries (M)
- Files: `components/sessions/useTerminalSizing.ts`, `components/sessions/SessionList.tsx`, tests

##### Task 3.1.2c: Server zero-size resize guards (S, Go; independent of every frontend task)
- Guards in `performInitialResizeNudge`, the capture-pane nudge and `Instance.ResizePTY` as specified above, with a Go test per path (0x0, 0xN, 1x1 leave `ResizePTY`/`SetWindowSize` uncalled). Run `make test` for the touched packages; the existing `terminalguard` lint testdata is unaffected.
- Files: `server/services/connectrpc_websocket.go`, `session/instance_tmux.go`, `server/services/connectrpc_websocket_test.go` (or the nearest existing test file for the nudge), `session/instance_tmux_test.go`

##### Task 3.1.2d: Client handshake guard + Jest test (S)
- Files: `lib/hooks/useTerminalStream.ts`, `lib/hooks/useTerminalFlowControl.ts`, tests (`useTerminalStream.test.ts` or the nearest existing test file)

---

## Phase 3: Unary RPCs over WebSocket (Epic 5; build needs Story 2.1.1 and Task 5.1.1b only; the rollout default is gated by Story 5.1.2 and Story 5.5.1)

### Epic 5.1: Unary multiplexed over one WebSocket
**Goal**: Build one multiplexed unary channel and **measure** whether it removes HTTP unary queueing (the 6-connection limit) during a window-switch + Sessions<->Backlog run. Per the owner decision the channel is built regardless; it is enabled by default only if the three-arm comparison in Story 5.5.1 passes its numeric criteria. The earlier claim that WS "removes the ~30 s stalls" is withdrawn until Story 1.1.4 and Story 5.5.1 support it (the HAR shows 20 open `101` sockets alongside the stall, suggesting open WebSockets do not occupy the 6 HTTP slots). Decisions in ADR-001 (framing), ADR-002 (fallback/retry/auth/dispatch), ADR-003 (topology/bootstrap).

#### Story 5.1.1: Spike - dispatch mechanism and protocol conformance
**As a** maintainer, **I want** the dispatch seam confirmed, **so that** the bridge re-enters the real handler chain instead of reimplementing it.
HostGuard coverage is already answered (VERIFIED, `server/guarded_procedures.go:91-113`: "HostGuard covers every route (and WebSocket upgrade)"; the remote chain `server/server.go:1941` has no HostGuard and relies on auth), so the spike is narrowed to the dispatch mechanism.
**Acceptance Criteria**:
- A findings note names the exact functions for: re-entering the listener's dispatcher (the stamp -> HostGuard -> auth|`LocalWriteGuard` -> mux composition inside `localChainWith` at `server/guarded_procedures.go:103-115` / `remoteChain()` at `server/server.go:1941`, which today return the fully wrapped chain with Compress/Logging/CORS outermost and so must be split, Task 5.2.1f) from a bridge registered on the shared mux; how the per-listener dispatcher is injected lazily in `Start()`/`StartRemote()` and selected from `RequestRecordFrom(ctx).Listener`; the synthesized-request header set (see Story 5.2.1); and the unary content type (`application/proto`, raw body) versus the streaming bridge's `application/connect+proto` (`ws_stream_bridge.go:94-98`).
  - *Given* the repo at HEAD, *When* the spike completes, *Then* `research/ws-unary-seams.md` names those functions and a throwaway Go test proves a synthesized request carrying the upgrade request's `Host`, `Origin`, `RemoteAddr` reaches a real unary handler through a real `localChain()` (as `server/streaming_chain_test.go:161` does for streams), and that a `guardedProcedures` entry gets the same verdict as over HTTP. The throwaway test lives in `package server` (`server/ws_unary_spike_test.go`) because `localChain()`, `remoteChain()` and `guardedProcedures` are unexported there (the cited precedent `server/streaming_chain_test.go` is `package server`, VERIFIED).
- The spike also covers the **remote listener** (owner decision 2026-10-26): the note names (a) how `remoteChain()` (`server/server.go:1941-1948`) is split so the remote dispatcher is stamp(`remote`) -> `authMW` -> mux, and that a request through it with the upgrade request's `Cookie` reaches a real unary handler (throwaway test over `srv.remoteChain(middleware.Auth(validator), true)`, the same constructor `server/files_recon_chain_test.go:83` uses); (b) the contents of `GetOrigins()` and `GetHostnames()` on a remote-access run (`main.go:1604-1626`, `main.go:456`), so Task 5.2.1g's allow-list is written against real data; (c) that an unauthenticated upgrade through the remote chain is answered 401 by `middleware.Auth` **before** the bridge runs (`isAPIPath` covers `/api/ws/unary`, `server/middleware/auth.go:153-155`; read at HEAD, to be confirmed by the test); (d) whether `Logging`/`Compress`/otelhttp in front of the remote upgrade preserve `http.Hijacker` (`server/middleware/logging.go:40`, `server/middleware/gzip.go:106` implement it; the existing remote stream upgrades use the same chain).
  - *Given* the repo at HEAD, *When* the remote part of the spike completes, *Then* the note records items (a)-(d) with the command or test that proves each, and marks any it could not run as UNVERIFIED.
- The spike also confirms the remaining auth question: auth sessions are fixed-TTL (the only `SetCookie` calls are in the login handlers, `server/auth/handlers.go:284,378`, `server/auth/localtoken.go:136`; none in `server/middleware/auth.go`, VERIFIED by grep), so synthesized responses dropping `Set-Cookie` lose no sliding refresh; the note confirms this by running a request past the TTL.
- ADR-001/ADR-002 are confirmed or amended.
  - *Given* the spike result, *When* complete, *Then* ADR-001 and ADR-002 status is Accepted or an amendment is recorded.
**Files**: `server/server.go`, `server/guarded_procedures.go`, `server/services/connectrpc_websocket.go`, `server/services/ws_stream_bridge.go`, `project_plans/frontend-responsiveness/research/ws-unary-seams.md`

##### Task 5.1.1a: Read seams and prove dispatch with a throwaway Go test (M)
- Files: `server/ws_unary_spike_test.go` (`package server`; throwaway: owner is Task 5.2.1d, which promotes it into `server/ws_unary_chain_test.go` or deletes it in the same PR), `research/ws-unary-seams.md`

##### Task 5.1.1b: Shared conformance vectors + Go/TS test stubs (S)
- Created once here so Stories 5.2.1 and 5.3.1 can run in parallel with no shared-file conflict: `web-app/src/lib/transport/__fixtures__/unary-mux-vectors.json` (frame encode/decode cases including CHUNK/fin), `web-app/src/lib/transport/__fixtures__/ws-unary-allowlist.json` (**created by Task 2.1.2a**: `wsUnaryAllowed` = the 9 methods enumerated there, `clientHeaders`: `["Connect-Protocol-Version"]`, `maxInFlight`: 6; client `wsEligible` must be a subset of the server's `wsUnaryAllowed`; this task only reuses it and wires the Go loader), an empty Go harness `server/services/ws_unary_frame_test.go` that loads the vectors, and a TS harness `lib/transport/unary-mux-frame.test.ts` that loads them. Story 5.2.1a only adds Go cases beyond the vectors; Story 5.3.1a only adds TS cases.
- Files: `unary-mux-vectors.json`, the Go and TS harness files above, and the Go loader for `ws-unary-allowlist.json` (the fixture itself belongs to Task 2.1.2a)

#### Story 5.1.2: Decision gate (after Epic 2 and Story 1.1.4; holds only Story 5.5.1, Story 5.5.2 and the default flip)
**As the** owner, **I want** the post-Epic-2 numbers and the slot-holder finding in one place, **so that** the rollout default and priority-socket plan are chosen with evidence, without cancelling the epic.
**Acceptance Criteria**:
- `research/ws-gate.md` records arm A (`tests/e2e/perf/baseline.json`, tag `perf-baseline`, captured before Epic 2) and arm B (all of Epic 2 merged, no WS) for: HTTP unary `queuedOver1s`, status-0 count, HTTP `peakInFlight`, median `ssq:route-change`, cold `ssq:window-switch`, per-method counts, server handler p95 for `GetVCSStatus`/`ListShells` (Task 1.1.4c); and the Story 1.1.4 slot-holder list, handshake-cap and handshake-serialization results.
  - *Given* Epic 2 merged and the perf spec run with identical throttle, *When* the note is written, *Then* it contains both arms' medians (>= 5 kept runs) and the 1.1.4 findings.
- The note applies this table and states which row fired (the epic is built in every row):

| Arm B result | Effect on Epic 5 |
|---|---|
| `queuedOver1s == 0` and status-0 `== 0` and HTTP `peakInFlight <= 4` | `unary_over_ws` ships OFF; the epic still completes; default flips only if Story 5.5.1 criterion 2 (>= 20% improvement) passes; priority socket (ADR-003 C) not scheduled unless the HOL probe fails |
| `queuedOver1s > 0` or status-0 `> 0` | Epic 5 is top priority; Task 5.4.1c (early-open bootstrap) and the HOL probe are mandatory before any default flip; ADR-003 option C (priority socket) is scheduled |
| 1.1.4 shows the slot holders are not unary requests | Recorded; the epic's metric is re-scoped to what a unary channel can affect, and the owner is informed once; build continues |
| Server-side: Task 1.1.4c shows server handler wait p95 > 1 s while fewer than 6 handlers are in flight | The stall is server-side contention, not browser queueing: `unary_over_ws` stays OFF, the bridge in-flight cap stays at 6, and Story 2.1.8 (replace the `ListInstanceData` read in `resolveSessionTitle`) is in scope as a backend change (the one exception to the requirements Out of Scope line); Epic 5 still completes; metric 1 is then judged by the SM1 fallback in Story 2.1.8 (critical-class zero-blocked plus a server handler p95 bound), not by the all-class zero |

  - *Given* the filled table, *When* reviewed, *Then* no row cancels or defers Epic 5, and no Epic 5 build story (5.2.1, 5.3.1, 5.4.1a/b, 5.3.2) waits for this note or for the owner re-capture in Task 1.1.4b.
**Files**: `project_plans/frontend-responsiveness/research/ws-gate.md`
**Depends on**: Epic 2.1 stories merged (including 2.1.7), 1.1.4. **Gates**: Task 5.4.1c ordering/priority, Story 5.5.1, Story 5.5.2 and the `unary_over_ws` default flip only.

##### Task 5.1.2a: Run arm B, fill the table, inform the owner (S, wall-clock blocker: owner re-capture from 1.1.4b)
- Files: `research/ws-gate.md`

#### Story 5.2.1: Server `UnaryWsBridge` - framing, dispatch, guards
**Depends on**: 5.1.1b and Task 5.1.1a (the spike's findings gate Tasks 5.2.1f and 5.2.1b; any ADR amendment it records is applied before they start), not on the 5.1.2 gate. Task 5.2.1f (dispatcher split) runs first in this story.
**As the** server, **I want** to decode `UnaryMuxFrame`s and dispatch each call through the same guard chain HTTP uses, **so that** HostGuard, `LocalWriteGuard(guardedProcedures)`, auth, body limits and interceptors behave identically.
**Acceptance Criteria**:
- Route is `/api/ws/unary` (all existing bridges are `/api/...`; the SPA catch-all `/` at `server/server.go:1475` would silently serve HTML for a mistyped path). Upgrade origin policy is **per listener** (`wsUnaryOriginPolicy`, selected from `RequestRecordFrom(ctx).Listener`): on `:8543` HostGuard (`server/middleware/hostguard.go:37-42`) already rejects a foreign Origin before the handler runs, so the local `CheckOrigin` keeps the loopback rule; on the **remote listener** (served, owner decision 2026-10-26) there is no HostGuard and `isAllowedOrigin` (`connectrpc_websocket.go:104-120`) accepts any HTTPS origin, so the unary route uses the strict remote rule of ADR-002 item 7 instead (Task 5.2.1g) and never `isAllowedOrigin`. The route accepts no query string (400 `ws_unary_query_forbidden`) and no subprotocol.
  - *Given* Host `localhost:8543` Origin `http://localhost:8543`, *When* upgrading, *Then* 101; *Given* Origin `http://evil.example` or `https://evil.example`, *Then* 403 (same as HTTP).
  - *Given* the remote listener, a valid session cookie and Origin `https://evil.example` (accepted by `isAllowedOrigin`), *When* upgrading, *Then* 403 (CSWSH negative test T-CSWSH-1, Task 5.2.1g); *Given* the same listener, a valid cookie and an allowed Origin, *Then* 101.
  - *Given* the remote listener and no session cookie, *When* upgrading with an allowed Origin, *Then* 401 from `middleware.Auth` and no 101 (T-CSWSH-2).
- Dispatch re-enters the **listener's dispatcher** (stamp -> HostGuard -> auth|`LocalWriteGuard` -> mux; Compress, Logging and CORS excluded), a map from listener name to handler set in `Start()`/`StartRemote()` and selected from `RequestRecordFrom(ctx).Listener`; an unset dispatcher fails closed with ERROR `unavailable` and never falls back to `s.mux`. Each synthesized request copies the **upgrade request's** `Host`, `RemoteAddr`, `Origin`, `Cookie`, `Authorization`, `User-Agent`, `X-Forwarded-*`/proxy headers (so `HasProxyHeader` and rebinding verdicts agree), sets `Content-Type: application/proto`, `Accept-Encoding: identity`, `Connect-Timeout-Ms` from meta, and path `/api/<pkg.Service>/<Method>`. REQUEST `meta.headers` is restricted to the ADR-001 allowlist (`clientHeaders`); a forbidden key (`Host`, `Origin`, `Cookie`, `Authorization`, `Forwarded`, `X-Forwarded-*`, `Content-*`, `Connect-Timeout-Ms`, `Accept-Encoding`, or anything not listed) answers ERROR `invalid_argument`. (The existing `ws_stream_bridge.go:78-92` copies only `Authorization`, `Cookie`, `Connect-Protocol-Version`, leaving `Host`/`Origin`/`RemoteAddr` empty; that is not the precedent.)
  - *Given* a REQUEST for an allowed read (`ListFiles`), *When* dispatched, *Then* the handler observes the upgrade's `Host`/`Origin`/`RemoteAddr` (asserted via `services.WithRequestHost` and the request-record middleware) and the **decoded response message** (and, via the RESPONSE meta, the response headers and trailers) equals the HTTP call's. Bytes are not compared: the shared HTTP transport is JSON (`web-app/src/lib/api/transport.ts:32-39`, no `useBinaryFormat`) while the bridge carries proto.
  - *Given* a REQUEST whose `meta.headers` carries `Origin: http://evil.example` (or `Host`, `X-Forwarded-For`), *When* dispatched, *Then* ERROR `invalid_argument` and the handler is not invoked; the verdict for the same call without that meta equals the HTTP verdict.
- Server-side deny-by-default: a method not in the Go-side `wsUnaryAllowed` table (reads only, mirrors the client `wsEligible` column) is answered ERROR `permission_denied` (marker `ws_method_not_allowed`). The client-side policy is never the only gate.
  - *Given* every key of `guardedProcedures` (`server/guarded_procedures.go:42-70`: probe, write-file, prune, `UpdateFeatureFlag`, `ReplyToPendingQuestion`, Backlog stage/gate writes), *When* each is sent over WS, *Then* each is rejected `permission_denied`; and a test fails if any `guardedProcedures` key appears in `wsUnaryAllowed`.
- Unary frames carry **no Connect envelope** (ADR-001): REQUEST body = method + meta + raw proto; RESPONSE = raw proto; ERROR = JSON built from the recorder's status and body. Requests whose whole encoded frame exceeds 256 KiB (`maxFrameBytes`; e.g. `WriteFile` up to 1.5 MiB under `withFileBodyLimits`, `server/server.go:515`) are HTTP-only by policy. Oversize enforcement is two-layer (ADR-001): an application-layer length check after each `ReadMessage` answers ERROR `resource_exhausted` for the parsed request id and keeps the socket open; `conn.SetReadLimit(4 * maxFrameBytes)` is only a hard backstop, because in the vendored gorilla/websocket (`v1.5.4-0.20250319132907-e064f32e3674`, `conn.go:933-935`) exceeding the read limit sends close 1009 and returns `ErrReadLimit` (the connection is dead afterwards; no `SetReadLimit` is used anywhere in `server/` today).
  - *Given* a 300 KiB REQUEST frame, *When* received, *Then* ERROR `resource_exhausted` for that id arrives, the socket stays open, and a following small REQUEST succeeds.
  - *Given* a frame larger than 1 MiB (a protocol violation), *When* received, *Then* the socket closes with 1009 and the close is logged.
- Responses are fully buffered (`ResponseRecorder`-style) before chunking; there are no streaming semantics. Responses over 256 KiB are sent as RESPONSE (`totalLen`, `fin`) then CHUNK frames (ADR-001), round-robin across ids.
  - *Given* a 2 MiB response and a small concurrent response, *When* both are in flight, *Then* the small RESPONSE is written before the large one completes, and the client reassembles 2 MiB using `totalLen`/`fin`.
- Multiplexing: interleaved REQUESTs are answered independently and out of order.
  - *Given* REQUEST id 1 for a handler sleeping 200 ms and id 3 for an immediate handler, *When* both are sent back-to-back, *Then* RESPONSE 3 arrives before RESPONSE 1.
- Deadlines come from `Connect-Timeout-Ms` and connect-go applies them (no second `context.WithTimeout`); per-request contexts derive from the upgrade `r.Context()` so server shutdown cancels in-flight calls; CANCEL cancels the handler context.
  - *Given* id 5 for a blocking handler and a CANCEL id 5, *When* sent, *Then* the handler observes `context.Canceled` within 100 ms and no RESPONSE is written.
  - *Given* meta `timeoutMs: 50` and a 500 ms handler, *When* sent, *Then* an ERROR `deadline_exceeded` arrives.
  - *Given* server shutdown with 3 calls in flight, *When* the context is cancelled, *Then* all 3 handlers observe cancellation.
- Per-request auth (both listeners): each REQUEST traverses the listener's auth wrapper with the **upgrade request's** `Cookie`/`Authorization`, so an expired or revoked session yields ERROR `unauthenticated` for that request (the dispatcher's 401 is mapped to `unauthenticated`) while the socket stays open and other ids are unaffected; it is not a socket failure and the client does not count it toward the breaker (Story 5.4.1). Verified mechanism at HEAD: `ValidateAuthSession` deletes an expired token and returns false on every call (`server/auth/session.go:195-208`) and `isAuthenticated` is evaluated per request (`server/middleware/auth.go:157-171`). The client never re-sends credentials in-band; recovery is the login redirect and a fresh socket.
  - *Given* a cookie that expires mid-socket on the local listener with auth enabled, *When* the next REQUEST is sent, *Then* ERROR `unauthenticated`.
  - *Given* the same on the **remote** listener (a session expired or revoked via `RevokeAuthSession` after the upgrade), *When* the next REQUEST is sent, *Then* ERROR `unauthenticated`, the handler is not invoked, the socket stays open, and a PING still gets a PONG.
- Remote listener admission (Task 5.2.1g): the remote upgrade requires `AuthMode == AuthModeRequired` (else 403 `ws_unary_auth_required`), a non-empty allowed Origin (else 403), and no query string.
  - *Given* the remote chain built with a nil `authMW`, *When* upgrading, *Then* 403 `ws_unary_auth_required` (WS never widens an open chain).
- Ping/pong deadline: a socket with no pong within the deadline is closed so a dead tab does not hold in-flight slots. The client applies the symmetric deadline (Task 5.3.1d: PING every 10 s while a call is in flight, a missing PONG within 5 s is a socket failure).
- Per-socket in-flight cap, default **6** (named constant `wsUnaryMaxInFlight`, mirrored by `maxInFlight` in the shared fixture; `resource_exhausted` beyond it) and a single writer goroutine with a bounded channel. The cap starts at 6, not 32, because the stall may be server-side contention (Story 1.1.4 server-phase capture; `ListShells` calls a full session-list read per call) and multiplexing 6 browser slots into 32 concurrent handlers could make the server worse. Raising it requires Story 5.5.1 evidence that server handler p95 does not grow with the cap.
  - *Given* 7 concurrent blocking REQUESTs, *When* sent, *Then* the 7th gets ERROR `resource_exhausted`.
- Malformed frames never panic: fuzzed unit test (5.2.1a) plus an integration check (5.5.1a); the socket closes with 1003 and logs.
- Chain-parity tests (5.2.1d) live in `package server` (`server/ws_unary_chain_test.go`) because `localChain()`, `remoteChain()` and `guardedProcedures` are unexported there; the bridge takes an injected `http.Handler` dispatcher so `server/services` tests use a fake. They run the bridge over **both** a real `localChain()`-derived dispatcher and a real `remoteChain(middleware.Auth(validator), true)`-derived dispatcher, each served by `httptest.NewServer` (TLS for remote, `httptest.NewTLSServer`, with the gorilla dialer using the server's cert pool): foreign Host -> 403 (local); loopback Origin on another port -> 403 (local); a guarded procedure -> same verdict as HTTP on both; meta-header override attempt -> `invalid_argument` on both; the same allowed read returns an equal decoded message over local WS, remote WS, local HTTP and remote HTTP; remote cross-origin HTTPS upgrade with a valid cookie -> 403; remote no-cookie upgrade -> 401; `unauthenticated` per request after expiry on **both** listeners with auth enabled; remote nil-`authMW` chain -> 403 `ws_unary_auth_required`; any query string -> 400.
- No duplicated upgrader/read-loop code: the upgrader, read loop and origin check shared with `ws_stream_bridge.go` are extracted into one helper (Task 5.2.1b) so `make ready-complexity-gate` (`dupl`, new-code-only) passes.
**Files**: `server/services/ws_unary_bridge.go` (new), `server/services/ws_unary_frame.go` (new), `server/services/ws_bridge_common.go` (new, extracted shared helper), `server/services/ws_unary_allowlist.go` (new; exports `WsUnaryAllowedMethods()`), `server/guarded_procedures.go` (split `localChainWith`), `server/server.go` (split `remoteChain`, route registration, dispatcher and origin-policy injection), `server/ws_unary_origin.go` (new, `package server`; builds the per-listener origin policy from `GetOrigins()`/`GetHostnames()`), `server/ws_unary_origin_test.go` (new), `server/ws_unary_chain_test.go` (new, `package server`), `server/services/ws_unary_bridge_test.go`, `server/services/ws_unary_frame_test.go`

##### Task 5.2.1a: Frame codec (types incl. CHUNK, encode/decode, size caps) + table and fuzz tests (M)
- Adds Go cases and fuzz to the harness created in 5.1.1b; runs in parallel with 5.3.1a (no shared file).
- Files: `server/services/ws_unary_frame.go`, `server/services/ws_unary_frame_test.go`

##### Task 5.2.1f: Per-listener dispatcher split (M; first task of Story 5.2.1)
- In `package server`, split `localChainWith` (`guarded_procedures.go:103-115`) so the stamp -> HostGuard -> auth|`LocalWriteGuard` -> mux composition is a named function whose result `localChainWith` then wraps with Logging/CORS/Compress, and do the same for `remoteChain` (`server.go:1941-1948`); expose a `map[listenerName]http.Handler` set in `Start()`/`StartRemote()` and passed to the bridge constructor; unset entries fail closed. Existing chain tests must pass unchanged.
- Files: `server/guarded_procedures.go`, `server/server.go`, `server/ws_unary_chain_test.go`

##### Task 5.2.1b: Shared upgrader/read-loop helper and bridge (M)
- Extract the common upgrade/read-loop code from `ws_stream_bridge.go` into `ws_bridge_common.go` (the upgrader takes its `CheckOrigin` as a parameter; the stream bridges keep passing `isAllowedOrigin`, unchanged) and use it from both bridges; register the route at `/api/ws/unary`; consume the dispatcher map from Task 5.2.1f and the origin-policy map from Task 5.2.1g; reject any non-empty `RawQuery` (400) and accept no subprotocol.
- Files: `server/services/ws_unary_bridge.go`, `server/services/ws_bridge_common.go`, `server/services/ws_stream_bridge.go`, `server/server.go` (route registration)

##### Task 5.2.1c: Writer goroutine, CHUNK interleave, in-flight cap (default 6), cancel (drops queued CHUNKs), deadline, ping/pong, two-layer frame cap (M)
- Files: `server/services/ws_unary_bridge.go`, `server/services/ws_unary_bridge_test.go`

##### Task 5.2.1d: Deny-by-default `wsUnaryAllowed` + chain-parity tests over real `localChain()`/`remoteChain()` (M)
- The allowlist and its unit test live in `server/services`; the "no `guardedProcedures` key in `wsUnaryAllowed`" test, the spike promotion and all chain-parity tests live in `package server` and read `services.WsUnaryAllowedMethods()`. Parity is table-driven over the two real listeners (`{local, remote}` x `{HTTP, WS}`): allow-list denial, guarded-procedure verdict, meta-header override, and decoded-message equality run once per listener; the remote rows use TLS and a stub `AuthValidator`. The Origin/CSWSH rows belong to Task 5.2.1g.
- Files: `server/services/ws_unary_allowlist.go`, `server/services/ws_unary_bridge_test.go`, `server/ws_unary_chain_test.go`

##### Task 5.2.1g: Remote-listener origin policy, auth gate and CSWSH tests (M)
- **`wsUnaryOriginPolicy.remote(r)`** (`server/ws_unary_origin.go`, `package server`, so it can read `s.GetOrigins()`/`s.GetHostnames()` lazily, as `hostGuardConfig` does at `server/server.go:1928-1933`; handed to the bridge as an injected `func(*http.Request) bool` per listener name). Rules, all required:
  1. `Origin` header present and parseable. A missing Origin is rejected on the remote listener (browsers always send Origin on a WebSocket handshake; the only legitimate no-Origin caller is a non-browser client, which must then present `Authorization: Bearer` and no `Cookie`, so a cookie can never be replayed from a header-less context).
  2. Scheme is `https` (a plaintext origin never talks to the TLS listener legitimately).
  3. The origin matches, case-insensitively with the default-port form normalized (`https://h` equals `https://h:443`, the way `server/auth/handlers.go:413-422` accepts both forms), **either** an entry of the configured set `s.GetOrigins()` (remote startup adds `https://<rpID>:<port>`, `https://<hostname>:<port>` per detected hostname and `https://localhost:<port>`, VERIFIED `main.go:1604-1626`) **or** the request's own `Host` (same-origin: Origin host and port equal `r.Host`) **and** the hostname of `r.Host` is in `s.GetHostnames()`. The hostname condition blocks a DNS-rebound name that merely points at the server (Origin and Host agree but the name is attacker-controlled); the spike (Task 5.1.1a item b) confirms the list holds the remote names, and if it does not, the same-origin clause is dropped and the configured set alone is the policy.
  4. No wildcard and no suffix matching.
- **Auth gate** (in the bridge, before `Upgrade`): remote requires `RequestRecordFrom(ctx).AuthMode == services.AuthModeRequired` (else 403 `ws_unary_auth_required`); an unauthenticated request never reaches the bridge because `middleware.Auth` answers 401 first (`server/middleware/auth.go:80-92`); for a request that reaches it authenticated, the bridge does not re-validate at upgrade (the per-request re-check on every REQUEST is the authority).
- **Negative tests** (`server/ws_unary_origin_test.go` for the pure policy, `server/ws_unary_chain_test.go` over the real `remoteChain` with a real `middleware.Auth` and a stub validator that accepts one token and can revoke it):
  - T-CSWSH-1: valid cookie + `Origin: https://evil.example` -> 403 at upgrade, no 101, handler never invoked, `ws_unary_upgrade_denied_total{listener="remote",reason="origin"}` +1.
  - T-CSWSH-2: no cookie, allowed Origin -> 401, no 101.
  - T-CSWSH-3: valid cookie + `Origin: https://<configured-host>:<other-port>` (same site, wrong origin) -> 403; plaintext `http://<configured-host>:<port>` -> 403; `null` Origin -> 403; missing Origin with a cookie -> 403; missing Origin with Bearer -> 101.
  - T-CSWSH-4: `Origin: https://evil.example` with `Host: evil.example` (rebinding shape, hostname not in `GetHostnames()`) -> 403.
  - T-TOKEN-1: `GET /api/ws/unary?token=<valid>` with no cookie and no header -> 401 (the auth middleware does not read the query, so a URL token never authenticates); with a valid cookie and any non-empty query -> 400 `ws_unary_query_forbidden`.
  - T-AUTH-1: nil-`authMW` remote chain -> 403 `ws_unary_auth_required`.
  - T-EXP-1: after a successful upgrade, revoke the session; the next REQUEST -> `unauthenticated`, socket open, a PING still answered, a second socket opened with the revoked cookie -> 401.
  - Pure-policy table: default-port normalization, case, trailing slash, userinfo in Origin, IDN/punycode label, `https://localhost:<port>` accepted, wildcard attempts rejected.
- Order: after Task 5.2.1f (shares `server/server.go` and `ws_unary_chain_test.go`) and in the same PR train as Task 5.2.1b; the pure-policy file has no dependency and may land first.
- Files: `server/ws_unary_origin.go`, `server/ws_unary_origin_test.go`, `server/ws_unary_chain_test.go`, `server/server.go` (inject the policy map next to the dispatcher map)

##### Task 5.2.1e: Server counters and logs (S)
- `/actuator` counters `ws_unary_calls_total{method,code}`, `ws_unary_inflight`, `ws_unary_denied_total{method}`, `ws_unary_upgrade_denied_total{listener,reason}`; `slog` entries (upgrade denials log listener, reason and the Origin value, never the Cookie or Authorization header and never a query string). Fallback counting is client-side (see Observability Plan); no server fallback counter exists.
- Note: no new RPC is added, so no registry row is required; if any registry row changes, run `make registry-generate` and set `tested: true` plus `testIds`.
- Files: `server/services/ws_unary_bridge.go`

#### Story 5.3.1: Client `UnaryWsTransport` (single shared socket, reconnect/backoff)
**Depends on**: Task 5.1.1b (vectors and TS stubs), not on the 5.1.2 gate.
**As the** web app, **I want** a Connect `Transport` whose `unary` multiplexes over one socket, **so that** eligible unary calls stop competing for HTTP connections.
**Acceptance Criteria**:
- `unary` sends a REQUEST (raw proto body, no envelope) and resolves with the decoded response, reassembling RESPONSE+CHUNK frames by `totalLen`/`fin`; errors map to `ConnectError` with the right `Code`; response headers/trailers from the RESPONSE/ERROR meta (ADR-001) surface on the call's `header`/`trailer`.
  - *Given* an open socket and `listFiles({...})`, *When* called, *Then* it resolves with a **decoded message equal to** the HTTP transport's decoded message for the same fake server response (Jest uses the real schema both ways; bytes are not compared because HTTP is JSON and the socket is proto), and a RESPONSE meta `{headers:{"x-a":"1"},trailers:{"x-t":"2"}}` shows up as response header `x-a` and trailer `x-t`.
- Interleaved concurrent calls resolve independently; results arrive out of order.
  - *Given* calls 1, 2, 3 sent and responses delivered in order 3, 1, 2, *When* they arrive, *Then* each promise resolves with its own payload.
- Abort/timeout sends CANCEL and rejects with `Code.Canceled` / `Code.DeadlineExceeded`.
  - *Given* a pending call and `AbortSignal.abort()`, *When* aborted, *Then* a CANCEL frame with that id is sent and the promise rejects `Canceled`.
- Reconnect with exponential backoff (100 ms, 200 ms, ... capped 5 s, jitter); in-flight calls fail with `Unavailable` and calls whose `replayable` column is true replay once over HTTP per ADR-002.
  - *Given* socket drop with a pending `getVCSStatus` and a pending `createSession`, *When* the socket closes 1006, *Then* `getVCSStatus` completes via HTTP and `createSession` rejects `Unavailable`.
- Liveness: a half-open socket (laptop sleep/wake, VPN or proxy drop, tab throttling) must not hang every WS call for the full 30 s. The client sends PING every 10 s while any call is in flight (and immediately on `visibilitychange` to visible, `online`, or a wake gap), and a missing PONG within 5 s is a **socket failure**: close the socket, reject in-flight calls with `Unavailable` (replayable reads replay over HTTP), and count it toward the circuit breaker. This is symmetric with the server's ping/pong deadline (ADR-002 item 8). (Task 5.3.1d.)
  - *Given* a fake socket that never answers PING, *When* a call is in flight for 15 s on a fake clock, *Then* the socket is closed, the pending replayable read completes over HTTP, and the breaker's failure count is 1.
- Reply ordering: a late reply for a MuxRequestId that has already settled, been cancelled, or been replayed over HTTP is dropped (never delivered, never resolves a reused id), so a replayed read cannot be overwritten by a stale WS answer and hooks that `setState` from each response see no WS/HTTP reordering for the same call.
  - *Given* a read replayed over HTTP after socket close and a WS RESPONSE for its original id arriving afterwards, *When* it arrives, *Then* it is dropped and the HTTP result stands.
- Requests whose **whole encoded frame** (type + id + method + meta + body) exceeds 256 KiB are sent over HTTP, not the socket; the threshold is on frame size, not body size, so a body just under 256 KiB cannot overflow the server's frame cap.
  - *Given* a 300 KiB request for a `wsEligible` method, *When* called, *Then* the HTTP transport is used; *Given* a body of `262144 - (header + method + meta)` bytes, *Then* it goes over WS; *Given* that body plus 1 byte, *Then* HTTP.
- `unary` runs through `runUnaryCall` (`@connectrpc/connect/protocol`, installed 2.1.1) with an **empty interceptor list** (it supplies only deadline derivation, abort/CANCEL wiring and error normalization); the shared interceptors (auth, rpc timing, coalescer, priority) and the 30 s `DEFAULT_RPC_TIMEOUT_MS` are applied **once**, outermost on the policy transport (Story 5.4.1), so they cover WS and HTTP calls alike and are never double-applied (ADR-002 item 7). A Jest test asserts `rpc:<Method>` is emitted exactly once per call on the WS path and once on the HTTP path.
  - *Given* a WS call with no explicit `timeoutMs`, *When* the REQUEST is encoded, *Then* meta `timeoutMs` is 30000.
- Send queue is bounded (64 frames / 1 MiB); overflow rejects `ResourceExhausted`. The client also keeps at most `maxInFlight` (6, from the shared fixture) REQUESTs outstanding and holds further calls in this queue. This bound applies to the new unary transport only; the streaming reader queue is Story 5.3.2.
  - *Given* 65 queued sends while the socket is CONNECTING, *When* the 65th is added, *Then* it rejects `ResourceExhausted`.
- Socket URL (both listeners): derived from the page's own origin by scheme mapping only, `http` -> `ws` and `https` -> `wss`, the same derivation the existing watch transport applies (`req.url.replace(/^http/, "ws")`, `web-app/src/lib/transport/watch-ws-transport.ts:131`, VERIFIED), pointed at path `/api/ws/unary` with **no query string and no fragment** and no subprotocol list (so no credential can ride in the URL or `Sec-WebSocket-Protocol`); credentials are the browser's `cs_auth` cookie. The URL is built from `location`/the transport `baseUrl`, never from a flag or server-supplied value, so a page cannot be pointed at another host.
  - *Given* `baseUrl` `https://host:8444`, *When* the socket is constructed, *Then* the URL is exactly `wss://host:8444/api/ws/unary`; *Given* `http://localhost:62871`, *Then* `ws://localhost:62871/api/ws/unary`; *Given* a `baseUrl` carrying `?token=x` or `#x`, *Then* both are stripped.
  - *Given* an `https:` page and an `http:` baseUrl (mixed content), *When* constructing, *Then* the transport refuses (HTTP fallback) instead of opening `ws://`.
  - *Given* an upgrade that fails with a non-101 response (a reverse proxy stripping `Upgrade` answers 200/400/426; the server answers 401 or a 403 origin denial) or a TLS error, *When* it happens 3 times, *Then* the breaker opens (HTTP-only 60 s, then one probe). A browser `WebSocket` exposes no HTTP status for a failed handshake (only an `error` event and close code 1006; INFERRED from the WHATWG WebSocket API, not tested here), so the client cannot tell 401, 403 and a stripped `Upgrade` apart; Story 5.4.1 handles the 401 case through the HTTP path's auth redirect instead of through the handshake.
**Files**: `lib/transport/unary-ws-transport.ts` (new), `lib/transport/unary-mux-frame.ts` (new, codec mirroring ADR-001), tests

##### Task 5.3.1a: TS frame codec against the shared vectors (S)
- Adds TS cases to the harness from 5.1.1b; no Go file touched.
- Files: `lib/transport/unary-mux-frame.ts`, `lib/transport/unary-mux-frame.test.ts`

##### Task 5.3.1b: Transport: socket manager, id allocation, unary, CHUNK reassembly, cancel, timeout, `ws`/`wss` URL derivation (M)
- Includes the Socket URL AC above (scheme mapping, no query/fragment/subprotocol, mixed-content refusal) with Jest tests over a fake `WebSocket` constructor that records its URL and protocols arguments.
- Files: `lib/transport/unary-ws-transport.ts`, `lib/transport/unary-ws-transport.test.ts`

##### Task 5.3.1c: Reconnect/backoff, replay policy, bounded send queue, late-reply dropping (M)
- Includes the `maxInFlight` client window and the dropped-late-reply rule (assert in the transport test: settled, cancelled and replayed ids).
- Files: `lib/transport/unary-ws-transport.ts`, tests

##### Task 5.3.1d: Client PING/PONG liveness feeding the circuit breaker (S)
- PING every 10 s while a call is in flight and on `visibilitychange`/`online`/wake; PONG deadline 5 s; failure closes the socket, rejects in-flight calls `Unavailable`, replays replayable reads over HTTP and increments the breaker. Jest test with a fake socket that never answers PING (fake clock).
- Files: `lib/transport/unary-ws-transport.ts`, `lib/transport/unary-ws-transport.test.ts`

#### Story 5.3.2: Instrument the `fromWebSocket` reader queue (no bound yet)
**As a** maintainer, **I want** the streaming reader queue depth measured, **so that** any future bound is sized from burst data and cannot end live terminal streams.
Context: the earlier plan bounded this queue and failed streams with `ResourceExhausted`. Story 4.1.3 deliberately keeps hidden terminals' streams open for K windows and a backgrounded tab throttles consumers, so a bound could kill live terminals; it is also scope drift from Epic 5's unary goal. The bound is **deferred** (see Review disposition).
**Acceptance Criteria**:
- A dev/opt-in high-watermark of queued messages per stream is recorded in `window.__ssqPerf`; behavior is unchanged.
  - *Given* a stream producing 10,000 messages with a stalled consumer, *When* measured, *Then* the high-watermark reports 10,000 and no stream is closed.
- If a bound is later adopted it must be a separate flagged change that applies backpressure (pause) to watch streams and exempts terminal streams; this story only supplies the data.
**Files**: `lib/transport/watch-ws-transport.ts` (instrumentation only), `lib/transport/watch-ws-transport.test.ts`

##### Task 5.3.2a: Queue-depth high-watermark (S)

#### Story 5.4.1: `WsUnaryPolicy`, flag, bootstrap, circuit breaker and HTTP fallback
**Depends on**: 5.3.1, Task 2.1.9a (`lib/api/transport.ts` edit order), not on the 5.1.2 gate.
**As the** owner, **I want** a kill switch, a working bootstrap under HTTP saturation and a safe fallback, **so that** I can trial reads first and revert quickly.
**Acceptance Criteria**:
- `getConnectTransport()` returns a transport whose `unary` consults `WsUnaryPolicy`: master flag `unary_over_ws` OFF (or persisted last-known OFF before load), **`originClass` is `remote` and `unary_over_ws_remote` is OFF (or persisted last-known OFF before load)**, method not `wsEligible`, method in the per-session denylist, request body > 256 KiB, force-HTTP override set, socket not OPEN after 250 ms, or breaker open -> HTTP. `originClass` is `remote` iff `location.protocol === "https:"` (protocol, never port, see Risk Control); a remote page on a server that never registered the sub-flag reads it as OFF, so an old server and a new client degrade to HTTP. There is no per-method flag (flags are `Record<string, boolean>`); the per-method gate is the static `wsEligible` column, enforced server-side by `wsUnaryAllowed`.
  - *Given* `unary_over_ws` OFF, *When* `getVCSStatus` is called, *Then* the HTTP transport is used and the WS transport receives 0 calls.
  - *Given* flag ON, *When* `listSessions` (not `wsEligible`) is called, *Then* HTTP is used; *When* `getVCSStatus` (`wsEligible`) is called, *Then* WS is used; *When* `getFeatureFlags` is called, *Then* HTTP is used (never `wsEligible`).
- A server `permission_denied` with marker `ws_method_not_allowed` replays the call once over HTTP and adds the method to a per-session denylist.
  - *Given* the server denies `getSessionDiff`, *When* called, *Then* it completes via HTTP and later calls skip WS.
- Contract: the client `wsEligible` set is a subset of the server's `wsUnaryAllowed` (shared `ws-unary-allowlist.json` fixture), asserted in both Jest and Go.
- Circuit breaker: 3 consecutive socket failures -> HTTP-only for 60 s, then one probe.
  - *Given* 3 failed opens, *When* a call follows within 60 s, *Then* it uses HTTP and no open is attempted; *When* 60 s pass on a fake clock, *Then* one open is attempted.
- Kill-switch latency: flag OFF takes effect within one flag refresh (<= 30 s), or immediately via the breaker or the `localStorage["ssq:ws-unary:force-http"]` override read on every call.
  - *Given* the override set mid-session, *When* the next call runs, *Then* it uses HTTP with no flag refresh.
- A 401/`unauthenticated` ERROR triggers the same login redirect as `createAuthInterceptor`.
  - *Given* a WS ERROR `unauthenticated`, *When* it reaches the client, *Then* the auth interceptor path redirects to `/login` (assert via the mocked `@/lib/nav` with `requireActual("@/lib/nav/impl.memory")`).
- The `InFlightGauge` and `rpc:<Method>` timing cover both paths; `ws_unary_fallback_total{reason}` is a client-side counter emitted as an analytics `performance` event; the policy records `wsQueueMs`/`wsQueuedOver1s` (policy decision to REQUEST frame written; Story 1.1.2) for every WS call.
  - *Given* a WS call to `GetVCSStatus`, *When* it settles, *Then* an `rpc:GetVCSStatus` measure exists.
- Interceptor and timeout placement: the coalescer, the auth interceptor, the rpc-timing interceptor and the 30 s default deadline wrap the **policy transport** (outermost), so they apply to HTTP and WS calls alike and are applied exactly once: the inner HTTP `createConnectTransport` is built with **no interceptors and no `defaultTimeoutMs`**, the outermost layer sets `timeoutMs` = 30000 when absent (one deadline owner), and `UnaryWsTransport` runs `runUnaryCall` with an empty interceptor list (Story 5.3.1). A Jest test asserts `rpc:<Method>` is emitted once per call on each path. (Today they sit inside `createConnectTransport`, `lib/api/transport.ts:32-39`; a policy transport that routes to `UnaryWsTransport` would otherwise bypass them.)
  - *Given* flag ON and 14 concurrent `getVCSStatus({sessionId:"s1"})` calls, *When* they start in one tick, *Then* exactly 1 REQUEST frame is sent (coalesced) and its meta carries `timeoutMs: 30000`.
- Origin class is protocol-based and testable on any port (one rule, both directions): `local` = `http:` (WS on when `unary_over_ws` is ON, including dynamic and manual dev ports such as `http://localhost:62871`), `remote` = `https:` (WS on only when `unary_over_ws` **and** `unary_over_ws_remote` are ON). There is no server-side 403 marker for remote: the server serves the remote listener (Task 5.2.1g) and the decision is client-side per origin.
  - *Given* `location` = `http://localhost:62871` and `unary_over_ws` ON, *When* `getVCSStatus` is called, *Then* the WS transport is used (independent of `unary_over_ws_remote`).
  - *Given* `location` = `https://host:8444`, `unary_over_ws` ON and `unary_over_ws_remote` OFF, *Then* HTTP is used and the WS transport receives 0 calls; *Given* both ON, *Then* WS is used; *Given* `unary_over_ws` OFF and `unary_over_ws_remote` ON, *Then* HTTP (the master flag is the kill switch for both origins).
  - *Given* `location` = `https://host:8444`, both flags ON and a persisted `ssq:flag:unary_over_ws_remote` = ON but a flag response that now reports it OFF, *When* it loads, *Then* the key is removed and the next boot opens no socket.
  - *Given* an old server that does not report `unary_over_ws_remote`, *When* an `https:` page loads, *Then* HTTP is used.
- Per-call WS timeout: a `replayable` read sent over WS that has no response within `WS_CALL_TIMEOUT_MS = 8000` is cancelled (CANCEL frame) and replayed once over HTTP with the remainder of its original 30 s deadline, so a silent socket costs at most 8 s, not 30 s; non-replayable calls keep the 30 s deadline. The 8 s value is above the slowest read in the HAR (`ListShells` 4-6 s server wait; INFERRED to avoid replay storms) and is recalibrated from arm C data. These replays are counted in the fallback numerator as `ws-call-timeout` and do not count toward the breaker (the PING liveness in Story 5.3.1 covers dead sockets).
  - *Given* a `getVCSStatus` over WS with no reply, *When* 8 s pass on a fake clock, *Then* a CANCEL frame is sent and one HTTP request is issued; a late WS reply is dropped (Story 5.3.1).
- 401 handling: a browser `WebSocket` does not expose the HTTP status of a rejected handshake (see Story 5.3.1), so the client cannot read a 401 off the upgrade. Two signals are used instead. (1) **Session expiry after the socket is open** (the common remote case): the server answers ERROR `unauthenticated` for the request (ADR-002 item 7); the policy treats it exactly like the HTTP 401 path (login redirect via the auth interceptor), does **not** count it toward the breaker, closes its own socket, and parks WS until the page reloads after login. (2) **Boot-time open before login** on an auth-protected listener (remote, persisted flags ON): the handshake fails (401 invisible), which would count as one failure; the auth interceptor's 401 on the first HTTP call sets a page-level `authRequired` marker, and while it is set failed opens do not count toward the breaker. Only network errors and open timeouts with no `authRequired` marker count toward the 3-failure breaker.
  - *Given* a socket whose next REQUEST returns ERROR `unauthenticated`, *When* it arrives, *Then* the login redirect fires once, the breaker count is unchanged, and no reconnect is attempted until reload.
  - *Given* a boot-time open that fails and an HTTP 401 recorded in the same page life, *When* it happens 3 times, *Then* the breaker stays closed and calls use HTTP.
  - *Given* 3 failed opens with no 401 seen (for example a proxy that strips `Upgrade`), *Then* the breaker opens.
**Files**: `lib/api/transport.ts`, `lib/transport/wsUnaryPolicy.ts` (new), `lib/features/flagStore.ts` (pre-load constants), tests. (The flags are registered server-side in Task 1.1.6a; no `lib/features/features/*` file defines them.)

##### Task 5.4.1a: `WsUnaryPolicy` + breaker with fake-clock tests (M)
- Files: `lib/transport/wsUnaryPolicy.ts`, `lib/transport/wsUnaryPolicy.test.ts`

##### Task 5.4.1b: Wire into `getConnectTransport()`: policy transport outermost, interceptors/timeout/coalescer around it, flag reads, auth mapping, gauge/timing (M)
- Fifth and last `lib/api/transport.ts` editor (1.1.2a -> 2.1.1a -> 2.1.2b -> 2.1.9a -> 5.4.1b). Reads `unary_over_ws` through `flagStore`. Also: strips interceptors/`defaultTimeoutMs` from the inner HTTP transport (outermost layer owns them), adds the 8 s per-call WS timeout and the `wsQueueMs` measurement.
- Files: `lib/api/transport.ts`, `lib/api/__tests__/transport.test.ts`

##### Task 5.4.1c: Bootstrap under HTTP saturation (M)
- Problem: the socket upgrade is an HTTP/1.1 request that may need a free slot, and the flag normally arrives via `GetFeatureFlags` over HTTP, which can itself be stalled. Design: (1) `UnaryWsTransport` opens **at app boot**, from `RootShell` (`app/WsUnaryBoot.tsx`) before the first RPC burst, if `flagStore` exposes a persisted last-known `unary_over_ws = true` (`localStorage["ssq:flag:unary_over_ws"]`; on an `https:` page also `unary_over_ws_remote = true`, `localStorage["ssq:flag:unary_over_ws_remote"]`; first-ever load and flag OFF use HTTP only); (2) `GetFeatureFlags` is never `wsEligible`, so the flag fetch does not depend on the socket; (3) the 250 ms rule is decided **once per connect attempt**: calls in the first 250 ms of a CONNECTING socket wait at most the remainder of that window, and once the socket has been CONNECTING for more than 250 ms every call goes over HTTP immediately until it is OPEN (no call pays 250 ms twice behind saturated slots); a socket that opens late is adopted for later calls; (4) a boot-time open timeout counts toward the breaker only after 3 failures, and a 401 upgrade does not count (see Story 5.4.1); (5) `flagStore` clears `ssq:flag:unary_over_ws` when a successful response omits the flag or reports it OFF. A server-injected bootstrap flag (HTML/`server-info`) is recorded in ADR-003 as the rejected alternative (extra coupling, adds an HTTP dependency). The early-open ordering in (1) depends on the Story 1.1.4 handshake-serialization probe: if Chrome serializes WebSocket handshakes per host (the unary socket would queue behind the ~20 stream sockets), `WsUnaryBoot` renders ahead of every stream subscriber in `RootShell` so the unary socket is constructed first, and the result is recorded in ADR-003.
  - *Given* a persisted last-known ON and 6 HTTP calls held open, *When* the app boots, *Then* the socket open attempt starts before the first RPC and the test records whether it opens (informed by Story 1.1.4's result); calls made meanwhile complete over HTTP without hanging.
  - *Given* no persisted value, *When* the app boots, *Then* no socket is opened until the flag loads ON.
  - *Given* an `https:` page with persisted `unary_over_ws` ON and `unary_over_ws_remote` absent, *When* the app boots, *Then* no socket is opened.
  - *Given* a persisted ON and a flag response that omits `unary_over_ws`, *When* it loads, *Then* the key is removed and the next boot opens no socket.
  - *Given* a socket CONNECTING for 400 ms and 5 calls issued at t=300..400 ms, *Then* no call waits past the 250 ms mark of the attempt and calls after it go to HTTP with no wait.
- Files: `lib/transport/unary-ws-transport.ts`, `lib/features/flagStore.ts`, `app/RootShell.tsx` (renders a new `app/WsUnaryBoot.tsx`; `layout.tsx` is not edited), `app/WsUnaryBoot.tsx` (new), tests

#### Story 5.5.1: Cross-stack tests and the three-arm comparison
**Depends on**: 5.4.1, 2.1.7, and the 5.1.2 gate note.
**As a** maintainer, **I want** the epic's success metric asserted end to end against baseline and against Epic 2 alone, **so that** WS is credited only for what it fixes and the default flips only on evidence.
**Acceptance Criteria**:
- Go integration test runs a real handler over the bridge with an httptest server (multiplexing, cancel, deadline, auth failure, error codes, in-flight cap, malformed frames closing 1003), once over plain `ws://` (local shape) and once over `wss://` with `httptest.NewTLSServer` and a stub auth validator (remote shape; `RootCAs` from the test server's certificate), including a 2 MiB chunked response and a session revoked mid-socket.
  - *Given* the test server, *When* `go test ./server/services -run WsUnary -race` runs, *Then* it passes for both transports.
- Playwright `tests/e2e/perf/ws-unary.spec.ts` (in the `perf` project) runs the window-switch + Sessions<->Backlog scenario in three arms with the same throttle and fixture, >= 5 kept runs each: **A** baseline, **B** Epic 2 with `unary_over_ws` OFF, **C** Epic 2 with `unary_over_ws` ON (bootstrap per Task 5.4.1c). Comparability: arm A is **re-run in the same session** (same machine, same throttle, same fixture, same spec version) as B and C by building the app at tag `perf-baseline` and running the current spec against it; the committed `baseline.json` is kept as history, not as the comparison base. The 20% threshold uses the Story 1.1.3 noise rule (the improvement must exceed 2x the MAD of the >= 5 kept runs, as a percentage of the median). It reports HTTP `peakInFlight`, `queuedOver1s`, status-0 count, median `ssq:route-change`, cold `ssq:window-switch`, WS fallback rate, to `research/ws-three-arm.md`.
- **Remote/h2 arms** (owner decision 2026-10-26: remote is in the first release, so whether WS helps or hurts on a multiplexed h2 origin is measured, not assumed). The same scenario, throttle, fixture and noise rule run against the **remote-access origin** of an isolated instance (`--remote-access --remote-port`, Task 5.5.1d) in three more arms: **A-R** baseline build (`perf-baseline` tag, same-session re-run), **B-R** Epic 2 with `unary_over_ws_remote` OFF, **C-R** Epic 2 with `unary_over_ws` and `unary_over_ws_remote` both ON. Each arm records, per HTTP unary call, `PerformanceResourceTiming.nextHopProtocol` (the plan INFERS h2; the arm asserts it), plus the same metrics as A/B/C and, for C-R, WS-side head-of-line (the interactive-p95 probe of criterion 3 run on the remote origin). Results go to `research/ws-three-arm.md` in a separate "remote/h2" table.
- Remote flip criteria (`unary_over_ws_remote` flips to default ON only if **all** hold; otherwise it stays OFF, remote pages use HTTP, and the bridge stays built and reachable behind the flag; thresholds are INFERRED targets, owner-revisable):
  - R1. `nextHopProtocol === "h2"` for >= 95% of HTTP unary calls in A-R and B-R; otherwise the remote arms are relabelled h1 and R2 is judged against the local criteria instead.
  - R2. C-R improves median `ssq:route-change` or cold `ssq:window-switch` over B-R by >= 20% and more than 2x the MAD (same noise rule as criterion 2). Neutral (within noise) keeps the flag OFF: h2 already multiplexes, so WS is an extra moving part with no demonstrated gain. Worse by > 5% beyond noise is recorded as "WS hurts on h2" and opens a follow-up (candidate cause: WS head-of-line on one ordered stream).
  - R3. Head-of-line on h2: interactive `GetVCSStatus` p95 during the ~5 MiB `GetFileContent` in C-R is <= 1.25x the same measurement in B-R (h2 interleaves streams; WS must not be materially worse) and the probe is valid (>= 1 MiB, `probe-invalid` rule).
  - R4. Remote WS handshake failures (non-101 or TLS error) are 0 across the kept C-R runs and the WS fallback rate (criterion 4's definition, remote arm only) is <= 1% over >= 200 eligible calls; any proxy-stripped upgrade seen on the owner's real remote path (Task 5.5.1d item c) blocks the flip.
  - R5. Terminal stream latency p95 and server handler p95 (`GetVCSStatus`, `ListShells`) in C-R are each within +10% of B-R, with the in-flight cap at 6.
  - *Given* the remote arms, *When* run, *Then* the note states pass/fail per R1-R5 with numbers and the observed `nextHopProtocol`.
- Default-on criteria (all numeric, all required; otherwise the flag stays default OFF and the epic stays built):
  1. C: HTTP unary `queuedOver1s == 0`, `wsQueuedOver1s == 0` and status-0 `== 0`.
  2. If B already has `queuedOver1s == 0` and status-0 `== 0`, C must still improve the median `ssq:route-change` or cold `ssq:window-switch` by >= 20% over B; otherwise WS is not justified as default.
  3. Head-of-line: interactive RPC p95 (`GetVCSStatus`, a `wsEligible` method) while a ~5 MiB `GetFileContent` (also `wsEligible`, the one large-response method on the socket; Task 2.1.2a) is in flight stays <= 2x idle p95 in the same run; otherwise ADR-003 option C (priority socket) is scheduled and default stays OFF until it ships. The probe is valid only if the measured `GetFileContent` response is >= 1 MiB (the server may cap content; the spec records the actual size, and a smaller response is reported as `probe-invalid`, which fails the criterion rather than passing vacuously).
  4. WS fallback rate (client counter) <= 1%. **Denominator**: every call `WsUnaryPolicy` evaluates with `unary_over_ws` ON whose method is `wsEligible` (counted at the policy decision, including calls issued during boot that were skipped by the 250 ms rule or an open breaker). **Numerator**: those of them that completed over HTTP because of `socket-not-open`, `breaker-open`, `socket-error-replay`, `ws-call-timeout` or `server-denied`. Calls skipped for `body-over-frame-cap`, `remote-flag-off` (a remote page with `unary_over_ws_remote` OFF) or the force-HTTP override are excluded from both. The criterion is evaluated per origin class (local arms and remote arms each need >= 200 eligible calls). Minimum sample: >= 200 eligible calls in arm C across the kept runs; fewer is reported as `insufficient-data` and fails the criterion.
  5. Terminal stream message latency p95 in C is within +10% of B. **Measurement**: an echo-marker round trip timed in the page with `performance.now()`: the spec types `echo __ssq_mark_<n>` through the terminal input path of a focused session and a dev-only tap (`window.__ssqPerf.terminalLatency`, behind the `ssq:perf` opt-in) records the time from the keydown dispatch to the marker appearing in the terminal output handler; >= 30 round trips per kept run, taken while the window-switch workload runs, p95 across the kept runs, same session and fixture in B and C.
  6. Server latency: server handler p95 for `GetVCSStatus` and `ListShells` in arm C is within +10% of arm B (server timing from Task 1.1.4c), with the bridge in-flight cap at its default of 6; `ws_unary_inflight` peak is reported, and any run at a higher cap (12, 32) is a separate labelled arm that must also meet this criterion before the cap constant is raised.
  - *Given* the seeded fixture, *When* the three arms run, *Then* the note states pass/fail per criterion with numbers.
- Remediation if criterion 1 fails in arm C (the flag stays OFF and metric 1 is not yet met): (a) use the Story 1.1.4 slot-holder list to find the holders that are not unary (for example HTTP-held streams) and move those onto the existing WS stream bridges; (b) if holders are eligible unary calls queued behind the 250 ms/boot window, schedule ADR-003 option C (priority socket) and move the early-open earlier; (c) if the handshake itself queues behind saturated slots (Story 1.1.4 result), re-open the rejected HTTP/2-via-local-TLS alternative with the owner. The note names which of (a)-(c) applies and opens a follow-up story; the epic is not removed.
**Files**: `server/services/ws_unary_bridge_integration_test.go`, `tests/e2e/perf/ws-unary.spec.ts`, `tests/e2e/ws-unary-remote.spec.ts` (new, Task 5.5.1d), `tests/e2e/perf/fixture.ts` (5 MiB file seed), `research/ws-three-arm.md`

##### Task 5.5.1a: Go integration tests (M)
- Files: `server/services/ws_unary_bridge_integration_test.go`

##### Task 5.5.1b: Playwright three-arm spec (M)
- Files: `tests/e2e/perf/ws-unary.spec.ts`

##### Task 5.5.1d: Remote-access instance, authenticated remote session, remote/h2 arms and remote e2e (M; owner-run fallback)
- Start an isolated instance with a remote listener: in Playwright through `TestServer({ remoteAuth: true })` (`tests/e2e/helpers/test-server.ts:35,84`, which passes `--remote-access --remote-port <free port>` and exposes `getRemoteBaseUrl()` = `https://localhost:<port>`, with `ignoreHTTPSErrors` for the private CA, as `tests/e2e/login-static-assets.spec.ts` does); for a by-hand run, the CLAUDE.md manual-instance recipe with `PORT=62871 --remote-port 62872` (instance #2: `62873`/`62874`), a private `STAPLER_SQUAD_TMUX_SOCKET`, and never the live `:8543`/`:8444`.
- Items to resolve, each with a named fallback (none is verified yet; see Unresolved Questions): (a) **authenticated remote session in headless Chromium**: remote auth is passkey-based and no existing spec logs in; try a CDP virtual authenticator (`WebAuthn.addVirtualAuthenticator`) against the invite/setup flow; fallback: the owner runs the remote arms headed against the manual instance and pastes the JSON from `window.__ssqPerf` into the note (wall-clock blocker on tstapler); (b) **h2 negotiation**: assert `nextHopProtocol` as in R1 and record `h2`/`http/1.1`; (c) **real remote path**: one owner-run check through the owner's actual remote route (LAN/Tailscale, and a reverse proxy if one is used) that the `wss://` handshake succeeds, recording any proxy that strips `Upgrade`.
- Remote e2e specs (`tests/e2e/ws-unary-remote.spec.ts`, `// @feature` header per the e2e conventions, `data-testid`/ARIA locators only, no `waitForTimeout`): `wsUnaryRemote_should_Use101OverWss_When_AuthenticatedAndFlagsOn`, `wsUnaryRemote_should_StayOnHttp_When_RemoteSubFlagOff`, `wsUnaryRemote_should_RedirectToLoginAndNotTripBreaker_When_SessionRevokedMidSocket`, `wsUnaryRemote_should_RefuseUpgrade_When_PageOpenedUnderAnotherOriginOnSameHost` (a second origin on the same host with another port opens `wss://<remote>/api/ws/unary` from `page.evaluate`; expect the socket to error without 101).
- Files: `tests/e2e/ws-unary-remote.spec.ts` (new), `tests/e2e/perf/ws-unary.spec.ts` (remote arms), `tests/e2e/helpers/test-server.ts` (only if a remote-session helper is needed), `research/ws-three-arm.md`

##### Task 5.5.1c: HOL probe (5 MiB `GetFileContent` seed) and echo-marker terminal-latency tap (M)
- Seed a ~5 MiB text file in the isolated instance's worktree; implement the `probe-invalid` rule; add the dev-only `window.__ssqPerf.terminalLatency` tap and the echo-marker driver; compute the fallback-rate counters per the criterion 4 definition.
- Files: `tests/e2e/perf/ws-unary.spec.ts`, `tests/e2e/perf/fixture.ts`, `lib/perf/perfDebug.ts`, `components/sessions/TerminalOutput.tsx` (tap only; last editor after 3.1.1b)

#### Story 5.5.2: Confirm ADR-003, decide the default, write the rollout gate
**Depends on**: 5.5.1.
**Acceptance Criteria**:
- Using Story 1.1.4 findings and the 5.5.1 three-arm data, ADR-003 is marked Accepted (or option C scheduled) and the defaults for `unary_over_ws` (criteria 1-6) and, separately, `unary_over_ws_remote` (criteria R1-R5) are set per the 5.5.1 results; the remote default may stay OFF while the local one flips. Each default is a code change in two places: `defaultValue` of the flag's entry in `knownFeatureFlags` (`server/services/feature_flag_service.go:214`, registered by Task 1.1.6a) and the matching pre-load constant in `web-app/src/lib/features/flagStore.ts`; both flip in the same PR; the staged rollout steps are recorded in `docs/how-to/` as a flip guide (mirroring `flip-workflow-engine-enforcement.md`).
  - *Given* measurements, *When* complete, *Then* ADR-003 status is updated and `docs/how-to/flip-unary-over-ws.md` lists flip criteria (the six numeric criteria above for `unary_over_ws`; R1-R5 for `unary_over_ws_remote`), the required reverse-proxy settings for the remote origin (`Upgrade`/`Connection` pass-through, idle timeout above the PING cadence, no frame-rewriting buffering), the reads-only `wsUnaryAllowed` widening procedure (code PR; including the rule that no non-`coalescible` method is added to `wsUnaryAllowed` without an idempotency key in the proto, so a reconnect can never duplicate a mutation), rollback (flag, force-HTTP override), and runs `make registry-generate` if any registry row changed.
**Files**: `project_plans/frontend-responsiveness/decisions/ADR-003-ws-unary-socket-topology-and-hol-mitigation.md`, `docs/how-to/flip-unary-over-ws.md` (new; add row to CLAUDE.md index), `server/services/feature_flag_service.go` (default flip, only if criteria pass), `web-app/src/lib/features/flagStore.ts` (pre-load constant)

##### Task 5.5.2a: Update ADR, write the flip guide, index row, flip the default if criteria pass (S)
- Files: ADR-003, `docs/how-to/flip-unary-over-ws.md`, `CLAUDE.md`, `server/services/feature_flag_service.go`, `web-app/src/lib/features/flagStore.ts`

---

## Phase 4: Navigation (Epics 4 and 6; parallel with Phase 3 after Epic 2/3)

### Epic 4.1: Keep-alive windows and window-switch accessibility
**Goal**: A retained-window switch reveals existing DOM in < 150 ms with long tasks < 50 ms; bounded K = 3; flag `window_keep_alive` default OFF until Story 4.1.6 flips it on numeric criteria (the metric is met only with the flag ON). The announcement/focus/`tabpanel`/optimistic-highlight fixes (Story 4.1.4) ship **unflagged** because they fix a present bug independent of keep-alive; the `WindowPanel` tabpanel wrapper is part of that unflagged set.

#### Story 4.1.1: Spike - `<Activity>` vs CSS hiding
**Acceptance Criteria**:
- A throwaway branch upgrades React to >= 19.2 (installed is 19.1.1, `web-app/package.json` `^19.0.0`, Next 15.3.2) and renders a hidden xterm under `<Activity mode="hidden">` in both the Next and Bun builds; findings recorded.
  - *Given* Next 15.3.2 and React 19.2, *When* `make build` and the bun build run, *Then* the note records pass/fail for each build and whether effects cleaned up in hidden mode.
- Decision recorded: use `<Activity>` only if both builds pass; otherwise CSS hide + `inert`.
  - *Given* the findings, *When* complete, *Then* `research/activity-spike.md` states the chosen primitive; the rest of Epic 4 is written against the `windowActive` prop so either works.
**Files**: `web-app/package.json` (spike branch only), `project_plans/frontend-responsiveness/research/activity-spike.md`

##### Task 4.1.1a: Spike and note (M)
- Files: `research/activity-spike.md`

#### Story 4.1.2: `WindowKeepAliveHost` (per-window state, focus memory, terminal-pool policy)
**Depends on**: 2.1.0, 4.1.1, 4.1.4b (`WindowPanel` wrapper), 1.1.6 (flag)
**Acceptance Criteria**:
- With the flag ON the host renders up to 3 windows (current + 2 most recent) each in a keyed wrapper; the LRU evicts the oldest and unmounts it.
  - *Given* windows w1..w4 visited in order, *When* w4 is current, *Then* w2, w3, w4 are mounted and w1 is unmounted.
- Rendering several containers uses **per-window** state, not the single-window values in `app/page.tsx`: per-window `paneState`, per-window dispatch (`dispatchPane(windowId, action)` already exists; `paneDispatch` at `:164-166` is bound to `currentWindow.id` and must not be shared), per-window `sessionDetailRef` (`:78,107,531,626` today a single ref focused from several effects), and `externalAssignSession`/the external-assign counter delivered **only to the active container**.
  - *Given* two mounted windows and an external assign event, *When* it fires, *Then* only the active window's pane state changes and the hidden window's does not consume it.
- Each window renders inside the unflagged `WindowPanel` wrapper from Task 4.1.4b (`role="tabpanel"`, `aria-labelledby` = its tab id); the host only adds, for hidden windows, the chosen primitive and `hidden`/`inert`; only the active wrapper is focusable. A deactivating window that holds focus hands focus off (to the target tab or the target window per Story 4.1.4's trigger rule) **before** `inert` is applied, because `inert` on a focused subtree drops focus to `body` (INFERRED; verified by the Playwright test).
  - *Given* w2 hidden, *When* tabbing through the page, *Then* no element inside w2 receives focus; *Given* focus inside w1 when switching to w2, *Then* `document.activeElement` is never `body` at any frame of the switch.
- Per-window last-focused-pane storage: the `useWindowFocusMemory` hook (created in Task 4.1.4a so the Story 4.1.4 focus rule works with the flag OFF; the host only consumes it) keeps `Map<windowId, lastFocusedPaneId>` written on deactivation and read on reactivation; if no focusable element is found, the window's `tabpanel` wrapper (`tabIndex=-1`) takes focus.
  - *Given* w1 with pane p2 focused, *When* switching to w2 and back, *Then* focus returns to p2 and w1's scroll offset is 400 px if it was scrolled 400 px.
- A switch to an evicted window renders pane chrome and a per-pane placeholder ("Connecting <title>...", skeleton-sized to the pane, no layout shift) and mounts xterm after first paint; the tab highlight does not wait on terminal mount; a mount failure shows the pane-level "Could not connect to <title>" + Retry. **Announcement**: the "Connecting <title>..." placeholder is not a live region and is **silent** (the only announcement of the switch is `Window <n> of <total>: <name>`); if any pane of the evicted window is still not connected after 1 s, `announce()` speaks once per switch ("Still connecting <title>", or "Still connecting <k> sessions" for k > 1), and a failure speaks once via the same channel; the xterm slot, not the `tabpanel`, carries `aria-busy` while connecting.
  - *Given* an evicted w1, *When* switching to it, *Then* chrome renders in the first frame and no full-page skeleton shows.
- Terminal pool policy (`lib/terminal/TerminalPool.tsx`: `dock` is keyed by `sessionId` with a single anchor, `:296-307`; `evictLRU` skips `docked` entries, `:251`): hidden windows' terminals **stay docked**, so the pool may exceed `maxSize` by at most the retained terminals (3 windows x 4 panes = 12) and nothing hidden is reclaimed (accepted, documented); a session assigned in the incoming window that is already mounted in another retained window evicts that other window from the LRU first (a session is never docked in two windows, so one host element never moves between anchors). `webglBudget` records that an evicted pane "stays on that renderer for its lifetime" (`:3-4`): a retained window that lost WebGL does not regain it until remount (accepted); the slot's `isVisible()` is wired to `windowActive` so hidden-first eviction applies. Hidden terminals keep consuming their streams and writing to xterm, so parse/write CPU (not only fit/rAF) is accepted for K windows.
  - *Given* the same session assigned in w1 (retained) and w2 (incoming), *When* w2 mounts, *Then* w1 is unmounted and the host element is docked once.
- Flag OFF renders exactly the current single container (behavior unchanged apart from the unflagged `WindowPanel` wrapper from Task 4.1.4b, which is the only DOM difference and exists with the flag OFF too).
  - *Given* `window_keep_alive` OFF, *When* switching windows, *Then* one `PaneTilingContainer` is mounted at a time inside one `tabpanel` and existing `app/page` tests pass.
  - *Given* flag ON with 3 retained windows, *When* the Axe route sweep (`tests/e2e/a11y-route-sweep.spec.ts`) runs, *Then* it passes (`inert` on hidden windows).
**Files**: `components/pane/WindowKeepAliveHost.tsx` (new), `lib/window/useWindowsController.ts` (new) (`lib/window/useWindowFocusMemory.ts` is created in Task 4.1.4a), `lib/terminal/TerminalPool.tsx`, `app/page.tsx` (render at `:633` plus per-window state/refs), tests

##### Task 4.1.2a: Host component + LRU + consume focus memory (from 4.1.4a) + focus handoff before `inert` + terminal-pool policy with tests (M)
- The focus handoff moved here from Task 4.1.4b because it is host behavior behind the flag. Starts after Task 4.1.4b (needs `WindowPanel`).
- Files: `components/pane/WindowKeepAliveHost.tsx`, `lib/terminal/TerminalPool.tsx`, `components/pane/WindowKeepAliveHost.test.tsx`, `lib/terminal/__tests__/TerminalPool.keepalive.test.tsx` (new)

##### Task 4.1.2b: Integrate in `app/page.tsx` behind `window_keep_alive` (read via `flagStore`) with per-window state/refs/dispatch and active-only external assign (M)
- The per-window state, refs and dispatch live in a provider-shaped `useWindowsController()` hook (not inline in `HomeContent`), so Task 4.1.5a lifts the same hook into a layout-level provider instead of rewriting it. Edits `app/page.tsx` after Task 4.1.4b.
- Files: `app/page.tsx`, `lib/window/useWindowsController.ts`, tests

#### Story 4.1.3: `windowActive` gating of effects, keys and polling
**Depends on**: 2.1.0, 2.1.6a, 2.1.6c, 2.1.6d, 3.1.2, 4.1.2 (Task 4.1.3c edits `useSessionVcs`/`useVcsStatus`/`useWorkflows`/`useBufferFreshness`, so it waits for 2.1.6a and 2.1.6d)
`isActive` (as used today) means "focused pane" (`SessionDetailView.tsx:325-326`, `PaneSplitRenderer.tsx:395`) and `useSessionVcs` already treats it as cadence (`useSessionVcs.ts:186`). Gating on `isActive && windowActive` would stop polling for visible unfocused panes, so the two are separate props (`isFocused`, `windowActive`) combined only in `useSessionDetailActivity` (Task 2.1.0a).
**Acceptance Criteria**:
- `usePanePickerKeys(active, ...)` is extracted from `PaneTilingContainer`: capture keydown and `externalSessionAssign` effects only run when `windowActive`; focus-stealing effects are gated.
  - *Given* two mounted containers with `pickerPendingSession` set in the hidden one, *When* the user presses a key, *Then* the hidden handler does not `stopPropagation`.
- `usePaneShortcuts(state, dispatch, containerRef)` (`PaneTilingContainer.tsx:143`), a second ungated per-container hook, is gated by `windowActive`.
  - *Given* two containers, *When* a pane shortcut is pressed, *Then* only the active container's handler runs.
- `shortcutRegistry.ts:95-100` bug: `register` overwrites on a duplicate id (`console.warn ... overwriting`) and the returned cleanup does `this.shortcuts.delete(id)` unconditionally, so a hidden window's unmount or re-register deletes the **active** window's registration. Fixed with owner-token (or ref-counted) cleanup so a cleanup only removes its own registration, plus id namespacing for windows (`<id>@<windowId>`) where both must coexist.
  - *Given* the same shortcut id registered by w1 (hidden) and w2 (active), *When* w1 unmounts, *Then* w2's shortcut still fires; *When* the key is pressed, *Then* only the active window's handler runs.
- `useReviewNavKeys(keysEnabled)` (extracted in Task 2.1.0b) receives `keysEnabled = windowActive && isFocused`; other keydown owners from `research/pitfalls.md` section 3 are audited with a written list.
  - *Given* two mounted `SessionDetailView`s, *When* Shift+ArrowRight is pressed once, *Then* `onNext` is called exactly once.
  - *Given* a visible but unfocused pane, *When* Shift+ArrowRight is pressed after Task 4.1.3b, *Then* its handler does not fire (this inverts the Task 2.1.0b pin; the deliberate behavior change).
- `PaneSplitRenderer` writes `--mobile-pane-tab-strip-height` only when its window is active.
  - *Given* w1 active (height 40 px) and w2 hidden (height 56 px), *When* both mounted, *Then* the CSS var equals 40 px.
- Polling (`useSessionVcs`, `useVcsStatus`, `useWorkflows`, `useBufferFreshness`) uses `pollMode`: off in a hidden window, slow for a visible unfocused pane, fast for the focused pane.
  - *Given* a hidden window and a fake clock advanced 120 s, *When* intervals fire, *Then* 0 `getVCSStatus` calls occur; *Given* a visible unfocused pane, *Then* polling continues at the slow cadence.
- Hidden terminals skip `fit()` and rAF work, hand back WebGL slots (hidden-first eviction via `webglBudget`) and refit once on activation; streams stay open for K windows (documented decision) and are **not** closed or failed by any queue bound (Story 5.3.2 defers the bound).
  - *Given* 3 windows x 4 panes (12 terminals), *When* the page settles, *Then* WebGL contexts <= 8 (the slot's `isVisible()` is wired to `windowActive`) and hidden terminals report 0 fit calls over 60 frames.
- Keep-alive never calls a resume/start RPC for an archived session on window activation (frontend behavior; the Go `noarchivedrevival` analyzer is unaffected).
  - *Given* an archived session in a retained window, *When* the window becomes active, *Then* no resume/start RPC is issued.
**Files**: `components/pane/PaneTilingContainer.tsx`, `components/pane/usePanePickerKeys.ts` (new), `components/pane/PaneSplitRenderer.tsx`, `components/sessions/SessionDetailView.tsx`, `lib/pane/usePaneShortcuts.ts`, `lib/shortcuts/shortcutRegistry.ts`, `lib/hooks/useSessionVcs.ts`, `lib/hooks/useWorkflows.ts`, `components/sessions/XtermTerminal.tsx`, `lib/terminal/webglBudget.ts`, tests

##### Task 4.1.3a: Extract `usePanePickerKeys` with `windowActive` gate (M)
- Files: `components/pane/PaneTilingContainer.tsx`, `components/pane/usePanePickerKeys.ts`, `components/pane/PaneTilingContainer.test.tsx`

##### Task 4.1.3b: Flip the plumbed `windowActive`; gate CSS var and `keysEnabled` (M)
- Files: `components/pane/PaneSplitRenderer.tsx`, `components/sessions/SessionDetail.tsx`, `components/sessions/SessionDetailView.tsx`, tests

##### Task 4.1.3c: Gate polling hooks by `pollMode` (M)
- Files: `lib/hooks/useSessionVcs.ts`, `lib/hooks/useVcsStatus.ts`, `lib/hooks/useWorkflows.ts`, `lib/hooks/useBufferFreshness.ts`, tests

##### Task 4.1.3d: `usePaneShortcuts` gating and `shortcutRegistry` owner-token cleanup fix; two-instance double-handling tests (M)
- Files: `lib/pane/usePaneShortcuts.ts`, `lib/shortcuts/shortcutRegistry.ts`, `lib/shortcuts/__tests__/shortcutRegistry.test.ts` (the existing file; the earlier path without `__tests__` did not exist, `ls web-app/src/lib/shortcuts/__tests__`), `components/sessions/SessionDetailView.keys.test.tsx`

##### Task 4.1.3e: Terminal hidden gating and WebGL budget integration (M)
- Files: `components/sessions/XtermTerminal.tsx`, `lib/terminal/webglBudget.ts`, tests

##### Task 4.1.3f: `window-keepalive.spec.ts` Playwright spec (S)
- Covers UX criteria 9-12 and 34 (evicted-window placeholder silent unless over 1 s) and 11 (one keypress handled once with two windows mounted), archived-session no-resume, container-geometry parity flag ON vs OFF (the Story 4.1.5 bounding-box check), and the echo-marker latency p95 within +10% of arm B (and of baseline, SM7). The hidden-window tab walk (criterion 10) tabs until focus returns to the first focusable element, bounded by the page's focusable count plus 5, never a fixed 60-Tab budget (the flake in open bug `docs/bugs/open/BUG-097-a11y-retry-button-tab-order-flaky-fixed-tab-budget.md`). Starts with `// @feature ...`; no `waitForTimeout`; helpers in `tests/e2e/pages/`. Runs after Task 4.1.2b (needs the host) and Task 4.1.3b.
- Files: `tests/e2e/window-keepalive.spec.ts`

#### Story 4.1.4: Window-switch focus, announcement, tabpanel and optimistic highlight (ships unflagged)
**Independent of** `window_keep_alive`: with the flag OFF the highlight, announcement and focus behave identically (UX criterion 17); only S2 differs.
**Acceptance Criteria**:
- Focus after a switch depends on the trigger (design/ux.md S3): Arrow/Home/End on the tablist **and Enter/Space on a tab** keep focus on the target **tab** (APG roving tabindex, `WindowTabStrip.tsx:444`); a pointer click with a mouse, leader Alt+W + 1-9/n/p and overview pick move focus into the target window's last-focused pane (else its `tabpanel`). **Enter/Space decision**: the tab strip already uses automatic activation (each arrow key calls `onSwitch`, `WindowTabStrip.tsx:444`), so by the APG tabs pattern Enter/Space has no selection job left; focus stays on the tab and Tab moves into the panel. Chosen over "move to the pane" because it leaves one outcome per control for keyboard and screen-reader users and keeps the tab the user is operating under their cursor; the rejected variant "manual activation" would change arrow-key behavior, which is out of scope. The Enter/Space path is the explicit `onKeyDown` branch at `WindowTabStrip.tsx:251-255` (the tab is a `<div role="tab">`, not a `<button>`); a `click` with `event.detail === 0` (a synthesized activation, as assistive-tech "activate" actions typically send; INFERRED for VoiceOver, confirmed by the manual AT step in Task 4.1.4d) is treated like Enter/Space. **Touch exception**: a switch originated by touch or pen (`pointerType` not `mouse`) or a swipe **never auto-focuses the terminal** (that would open the on-screen keyboard, see open bug `docs/bugs/open/BUG-110-mobile-terminal-blank-after-keyboard-open.md`); focus stays on the tab and the content is visible. Under `(pointer: coarse)` each window tab and the Retry and "Go to Sessions" controls are at least 44x44 CSS px (above the WCAG 2.2 AA floor of 24 px, which is INFERRED from memory; 44 px is the review's requirement). The last-focused-pane memory is `useWindowFocusMemory` (created in Task 4.1.4a, not in the flagged host): with the flag OFF (single container) it records the focused pane from `paneState` at deactivation, so UX criterion 17 holds without keep-alive. Leader `,`/`o` are unchanged; switching to the already-active window does nothing.
  - *Given* w1 active and focus on a tab, *When* ArrowRight, *Then* `document.activeElement` is the w2 tab; *When* Alt+W then `2`, *Then* `document.activeElement` is inside w2's active pane.
  - *Given* focus on the w2 tab, *When* Enter or Space is pressed, *Then* `document.activeElement` is still the w2 tab and the next Tab lands in w2's panel; *Given* a touch tap on the w3 tab (`hasTouch` context), *Then* focus is on the w3 tab, no terminal is focused, and each tab's bounding box is at least 44x44 CSS px.
- Announcement: an imperative `announce(text)` (or small store) is added to the existing polite `RouteAnnouncer` live region, reusing the clear-then-set generation trick (`RouteAnnouncer.tsx:22-36`) so an identical repeated message re-announces; no second live region. Message `Window <n> of <total>: <name>` (`Window 2 of 3` when the name is empty). The pathname announcement wins in a same-tick conflict. Backlog stall text (Story 6.1.3) uses the same channel.
  - *Given* w2 named "api", *When* switching to it twice in a row, *Then* the status region reads "Window 2 of 3: api" each time and one live region exists.
- The tab highlight uses `aria-selected`, **not** `aria-current` (the earlier AC text was wrong; the component already uses `aria-selected`, and both would double-announce). It updates in the same frame as the click/keypress via local `pendingWindowId` state (the URL-derived `currentWindowId`, `useWindowUrlSync.ts:55` (`resolveCurrentWindowId`), cannot lead `router.replace`); `pendingWindowId` clears when `currentWindowId` equals it, is overwritten by a later switch (last wins), rolls back if the target id disappears, and has a 2 s safety timer.
  - *Given* a click on tab w3, *When* the next frame renders, *Then* w3 has `aria-selected="true"` (and no `aria-current`) even if the content has not yet committed.
- A `role="tabpanel"` wrapper (`WindowPanel`) with `aria-labelledby`, and `aria-controls` on each tab (tab `id`s added in `WindowTabStrip.tsx`). The wrapper is created here and rendered **regardless of `window_keep_alive`** (with the flag OFF it wraps the single current container in `app/page.tsx`; with it ON the host renders one per window), so `aria-controls` always resolves.
  - *Given* the tab strip with `window_keep_alive` OFF, *When* inspected, *Then* each tab's `aria-controls` resolves to a `tabpanel` whose `aria-labelledby` resolves back; the same holds with the flag ON.
- No transition under any motion preference; default is no transition.
- Leader keys Alt+W then 1-9/n/p/,/o and the 3 s leader timeout still behave as in `useWindowShortcuts.ts:39-44` (existing tests green).
  - *Given* Alt+W then 4 s idle, *When* `2` is pressed, *Then* no switch occurs.
- Axe route sweep (`tests/e2e/a11y-route-sweep.spec.ts`) passes with the flag OFF here; the flag-ON run (3 retained windows, `inert`) is asserted in Story 4.1.2.
**Files**: `lib/window/useWindowUrlSync.ts`, `lib/window/useWindowShortcuts.ts`, `components/window/WindowTabStrip.tsx`, `components/window/WindowPanel.tsx` (new), `app/page.tsx` (wrap the single container in `WindowPanel`), `lib/nav/RouteAnnouncer.tsx`, `lib/nav/announce.ts` (new), tests
**Order**: Task 4.1.4a (`announce()`) precedes Task 6.1.3b; Task 4.1.4b precedes Tasks 4.1.2a/4.1.2b. Tasks 4.1.4a, 4.1.4b and 4.1.4d have no dependency on the keep-alive host; Task 4.1.4c does (it measures with `window_keep_alive` ON, so it runs after Stories 4.1.2 and 4.1.3) and is the evidence generator for Story 4.1.6.

##### Task 4.1.4a: `announce()`, trigger-dependent focus, `pendingWindowId`, `useWindowFocusMemory` (M)
- Creates `lib/window/useWindowFocusMemory.ts` (flag-independent; the host in Task 4.1.2a consumes it) with its test.
- Files: `lib/nav/RouteAnnouncer.tsx`, `lib/nav/announce.ts`, `lib/window/useWindowUrlSync.ts`, `lib/window/useWindowFocusMemory.ts`, `components/window/WindowTabStrip.tsx`, tests

##### Task 4.1.4b: Unflagged `WindowPanel` tabpanel wrapper + `aria-controls` (M)
- Files: `components/window/WindowTabStrip.tsx`, `components/window/WindowPanel.tsx`, `app/page.tsx`, tests

##### Task 4.1.4c: Playwright window-switch budget spec (S)
- Run with `window_keep_alive` ON and again OFF; the report states the flag state of each number. Retained (flag ON): median `ssq:window-switch` <= 150 ms and long-task max < 50 ms on the seeded fixture, measured both unthrottled and at 4x CPU throttle; which of the two gates is `OPEN-DECISION-throttle-gate` (until resolved both are recorded and neither is a hard fail). **Cold** (first visit or LRU-evicted) is measured and reported separately with its own targets: pane chrome visible within 150 ms, `ssq:window-switch` (which ends after fit + focus) <= the baseline cold median, and **no task > 100 ms during the switch** (requirements Success Metrics, `requirements.md:25`; it applies to every switch, including cold ones); the cold switch is out of the 150 ms and 50 ms numbers only. The first visit to each window and any window evicted at K = 3 remount at the baseline cost.
- After every switch (retained and cold) the spec also asserts the tmux pane dimensions, read through the fixture's `tmuxPaneDims(sessionName)` helper (Task 1.1.3a), equal the pre-switch dimensions and are never 0xN or 1x1: this catches a hidden terminal's handshake resizing the real window (pairs with Tasks 3.1.2c/3.1.2d).
- Depends on Stories 4.1.2 and 4.1.3 (flag-ON host); feeds Story 4.1.6.
- Files: `tests/e2e/perf/frontend-responsiveness.spec.ts`, `tests/e2e/pages/PerfPage.ts`

##### Task 4.1.4d: `window-switch-a11y.spec.ts` Playwright spec (S)
- Covers UX criteria 1-8, 14-17, 33 and the automated proxy of 32 (see `implementation/validation.md`), under both `window_keep_alive` states; the manual VoiceOver script (criterion 32) gains the Enter/Space check: VO-Space on a tab leaves focus on the tab (confirms the `event.detail === 0` assumption) and Tab then enters the panel; uses `page.clock` only if the Task 1.1.3e spike passes. Starts with `// @feature ...`; no `waitForTimeout`; `data-testid`/ARIA locators only.
- Files: `tests/e2e/window-switch-a11y.spec.ts`

#### Story 4.1.5: Lift the keep-alive host above the route boundary
**As a** user, **I want** retained windows to survive `/` -> `/backlog` -> `/`, **so that** returning to Sessions is also instant.
`WindowKeepAliveHost` at `app/page.tsx:633` lives inside `HomeContent`, so a route change unmounts `app/page.tsx` and every retained window. The window model lives in `app/page.tsx` (`useWindowShortcuts` at `:180`, `useWindowUrlSync` at `:162`, `paneDispatch` at `:164-166`, `sessionDetailRef` at `:78,107,531,626`, external assign), so this is a state move plus a geometry problem, not a one-line hoist. The terminal pool is already module-level (`TerminalPool.tsx`) so remount cost may be smaller than assumed (INFERRED; measured by the `/` -> `/backlog` -> `/` scenario in Story 1.1.3).
**Depends on**: 4.1.2, 4.1.3 (polling/keys must be gated before the host outlives its route; the AC says polling and keys are off on `/backlog`), 4.1.4, 1.1.3
**Acceptance Criteria**:
- The `useWindowsController()` state from Task 4.1.2b moves to a layout-level `WindowsProvider` rendered in `app/RootShell.tsx` (shared by the Next `layout.tsx` and the bun `main.tsx`); `app/page.tsx` renders only a host slot and the tab strip contract is unchanged. While `pathname !== "/"` the retained windows stay mounted but hidden/`inert`/`windowActive=false`.
  - *Given* flag ON and windows w1..w3 retained, *When* navigating `/` -> `/backlog` -> `/`, *Then* the w1..w3 containers are not remounted (mount counter unchanged) and polling/keys are off while on `/backlog`.
- Geometry: the host renders through a portal into a slot element that `app/page.tsx` places in the same DOM position as today's container inside `<main>` (`RootShell.tsx:25-30`, after the two banners), so the retained windows occupy identical geometry and scroll container; with the flag OFF there is no portal and the DOM is unchanged.
  - *Given* flag ON and flag OFF, *When* the page loads at the same viewport, *Then* the container's bounding box and the `main` scroll height are equal within 1 px.
- `sessionDetailRef` focus handling (`page.tsx:107,531`) moves with the controller and is a no-op while off-route.
  - *Given* a navigation to `/backlog`, *When* a session-selection effect fires, *Then* no focus is taken from the Backlog page.
- Return-to-`/` median `ssq:route-change` is reported against baseline in the perf spec.
  - *Given* the fixture, *When* the scenario runs, *Then* the note states the before/after median; if the delta is below measurement noise (Story 1.1.3 noise rule) the story records the finding but the lift stays (it is flag-gated).
- Flag OFF behavior is unchanged.
**Files**: `app/RootShell.tsx`, `lib/window/WindowsProvider.tsx` (new), `lib/window/useWindowsController.ts`, `components/pane/WindowKeepAliveHost.tsx`, `app/page.tsx`, tests

##### Task 4.1.5a: Lift `useWindowsController()` into a layout-level `WindowsProvider` (state move) (M)
- Edits `app/RootShell.tsx` in a wave of its own (see the edit-order table: any landing order versus Tasks 5.4.1c and 6.1.3b); edits `app/page.tsx` after Task 4.1.2b.
- Files: `app/RootShell.tsx`, `lib/window/WindowsProvider.tsx`, `lib/window/useWindowsController.ts`, `app/page.tsx`

##### Task 4.1.5b: Portal geometry parity: slot in `<main>`, flag-OFF no-portal path, bounding-box test (M)
- Files: `components/pane/WindowKeepAliveHost.tsx`, `app/page.tsx`, tests

##### Task 4.1.5c: Move `sessionDetailRef` focus effects; gate while off-route (M)
- Files: `lib/window/useWindowsController.ts`, `app/page.tsx`, tests

#### Story 4.1.6: Flip `window_keep_alive` by default
**As the** owner, **I want** numeric criteria for turning keep-alive on, **so that** metric 3 (window switch) is met in the shipped configuration, not only behind an OFF flag.
**Depends on**: 4.1.2 through 4.1.5, 4.1.4c, 1.1.6, and the owner's resolution of `OPEN-DECISION-throttle-gate` (flip only; build stories do not wait).
**Acceptance Criteria**:
- All must pass on the seeded fixture, otherwise the flag stays default OFF: (1) retained switch median `ssq:window-switch` <= 150 ms and long-task max < 50 ms in the run that gates per `OPEN-DECISION-throttle-gate` (the other run is reported); (2) cold switch has no task > 100 ms; (3) 3 windows x 4 panes keeps WebGL contexts <= 8; (4) Playwright asserts `document.activeElement` is never `body` at any frame of a switch, and the Axe sweep passes flag ON; (5) the `/` -> `/backlog` -> `/` scenario shows no retained-window remount; (6) resource ceiling with 3 windows x 4 panes retained, as **provisional numeric targets** (INFERRED, not measured; the owner may revise them in `research/keep-alive-flip.md` after the first run): docked terminals <= 12 and open terminal streams <= 12 beyond the baseline watch streams; JS heap after 30 window switches (a >= 30 min dwell run on the seeded fixture) <= 1.5x the flag-OFF heap at the same point; active-window keystroke echo p95 (the Story 5.5.1c tap) within +10% of flag OFF. If this criterion fails the flag stays OFF and a follow-up story adds hidden-window stream pause/throttle (flag-ON only); that follow-up is not built unless triggered (hidden terminals otherwise keep consuming 12 streams and parsing output, accepted in Story 4.1.2).
  - *Given* the Story 4.1.4c spec, *When* it runs with the flag ON, *Then* each criterion is stated pass/fail with its number in `research/keep-alive-flip.md`.
- On pass, `defaultValue` for `window_keep_alive` becomes true in `knownFeatureFlags` (`server/services/feature_flag_service.go:214`, registered by Task 1.1.6a) and the pre-load constant in `web-app/src/lib/features/flagStore.ts` matches; rollback is the flag itself.
  - *Given* a fresh config after the flip, *When* `GetFeatureFlags` runs, *Then* `window_keep_alive=true`; the Story 7.1.4 final capture states this flag state.
**Files**: `server/services/feature_flag_service.go`, `web-app/src/lib/features/flagStore.ts`, `project_plans/frontend-responsiveness/research/keep-alive-flip.md`

##### Task 4.1.6a: Run criteria, write the note, flip the default if they pass (S, wall-clock blocker: owner dogfood sign-off)
- Files: `server/services/feature_flag_service.go`, `web-app/src/lib/features/flagStore.ts`, `research/keep-alive-flip.md`

### Epic 6.1: Sessions <-> Backlog and Backlog load
**Goal**: `/backlog` cold hard-load long-task total < 1 s on the seeded fixture (storage cleared), reported against the `baseline.json` cold median (the 3.4 s in requirements came from a different instance and dataset); return navigation renders from the existing warm store and the mount-time REST refresh prunes anything deleted while away; no second data layer. Scope of 6.1.2 is decided from Story 1.1.5's attribution.

#### Story 6.1.1: Refactor-first split of `app/backlog/page.tsx` around the existing watch stream
**Depends on**: 1.1.5 (committed `research/backlog-attribution.md`), 2.1.1c (merged; it edits `app/backlog/page.tsx:356-362`, the `rawClientRef` transport effect)
**Acceptance Criteria**:
- `/backlog` issues **no new** list RPC: it subscribes via `useWatchBacklogItems()` (`app/backlog/page.tsx:273`, hook at `lib/hooks/useWatchBacklogItems.ts:128`; `:56` is an import line) into the global Redux slice (`lib/store/backlogItemsSlice.ts`, `upsertItem(item, isSnapshot)`), keeping `BacklogItem` proto-shaped and mapping to the domain shape in the hook. `useBacklogData()` therefore **wraps** `useWatchBacklogItems` plus selectors (list, filters, selection, `BacklogConnectionState`); it does not own a fetch and adds no new RPC. `listBacklogItems` stays exactly as the hook runs it today: by design it fires **once on every mount** "alongside (not gated behind)" the stream (`useWatchBacklogItems.ts:178-208`, pitfalls #1) and again on gap/reconnect and the fallback poll. This story does not remove or gate it; Story 6.1.2 makes it the authoritative reconcile.
  - *Given* existing backlog Jest and e2e tests, *When* run after the split, *Then* all pass unchanged.
- The ~20 effects are classified in a note (load / derive / UI / listener) and each load effect has a single owner; the counted contract is **one REST `listBacklogItems` refresh per mount plus one `WatchBacklogItems` subscribe**, with further REST calls only on gap/reconnect/fallback poll.
  - *Given* the split, *When* `/backlog` mounts, *Then* `listBacklogItems` is called exactly once and `WatchBacklogItems` is subscribed exactly once (call-count test), and no second subscriber exists.
- The page file shrinks below 400 lines with behavior unchanged; `dupl`/`jscpd` stay under thresholds.
  - *Given* the diff, *When* `make ready-duplication-gate-web` runs, *Then* it passes.
**Files**: `app/backlog/page.tsx`, `app/backlog/useBacklogData.ts` (new), `components/backlog/*` (extractions), tests

##### Task 6.1.1a: Extract data hook wrapping `useWatchBacklogItems` with subscribe-count tests (M)
- Files: `app/backlog/useBacklogData.ts`, `app/backlog/useBacklogData.test.ts`, `app/backlog/page.tsx`

##### Task 6.1.1b: Extract list/rendering components (M)
- Files: `app/backlog/page.tsx`, `components/backlog/BacklogListView.tsx` (new), tests

#### Story 6.1.2: Hydrate the Backlog store from a persisted snapshot; progressive render
**Depends on**: 6.1.1, 1.1.5
**Acceptance Criteria**:
- Persistence is a **hydration of `backlogItemsSlice`** (logic in `lib/backlog/`, not `app/backlog/`): serialize proto items with `toBinary`/`toJson` under a versioned key **scoped by instance/origin and status filter** (`STAPLER_SQUAD_INSTANCE`/`location.origin`/`statusFilterKey`, so one instance never renders another's list and a filtered snapshot is never shown for a different filter), load into the slice with `isSnapshot=true` (`false` is reserved for genuine live events and drives the flash treatment, `backlogItemsSlice.ts:43-50`), and let the **mount-time REST refresh be the revalidation and the prune** (below). No `listBacklog` snapshot layer, no second source of truth.
- **Authoritative reconcile (ghost prevention).** Neither REST nor the stream prunes today: `refresh()` and the stream only `upsertItem` (`useWatchBacklogItems.ts:196-198`), and `snapshotComplete` is sent only when the snapshot is otherwise empty (`:318-326`), so the stream has no end-of-snapshot marker when items exist. The hook therefore records the ids present in the store at mount (hydrated **and** carried over from a previous visit) and, after the first successful REST response of that mount, removes (`removeItem`) every recorded id that is absent from the response, was not touched by a stream event since mount, has no in-flight local mutation, and would have been in scope for the request (status within `statusFilter`, not terminal, not archived, the request's `includeTerminal:false`/`includeArchived:false`). Failure of the REST call prunes nothing.
  - *Given* a persisted snapshot containing item `x`, and `x` deleted or archived server-side, *When* a cold `/backlog` hard load completes its first REST refresh, *Then* `x` disappears from the store and the list.
  - *Given* items `a,b` in the warm store from a previous visit and `b` deleted while away, *When* returning to `/backlog`, *Then* `b` is removed after the mount REST response while `a` stays and an item delivered by the stream meanwhile is not pruned.
  - *Given* a REST failure, *When* the refresh settles, *Then* nothing is removed and the list is marked out of date.
  - *Given* a persisted snapshot of 200 items for this origin, *When* a cold `/backlog` hard load starts, *Then* 200 rows render before the first REST response and stream snapshot arrive; *Given* a snapshot stored under a different origin/instance key, *Then* it is not rendered.
- Refreshing / out-of-date derive from the existing `BacklogConnectionState` (`connecting|live|reconnecting|polling|stale`), not a new flag: "Refreshing..." (`role="status"`, polite) only if revalidation exceeds ~1 s; on failure the stale list stays with an "Out of date" badge that carries an **age cue** ("Out of date, saved 3 min ago"; buckets "just now" under 60 s, then "N min ago", "N h ago", "N d ago"; a `<time dateTime>` element refreshed every 60 s only while the badge is shown, never inside a live region), Retry, and the polite message "Backlog could not refresh. Showing list saved 3 minutes ago." (falls back to "Showing saved list." when no age is known). The age comes from `savedAt` stored in the persisted snapshot envelope, or from the hook's `lastSyncedAt` (time of the last successful REST response or live stream event) for a warm store; no cache shows skeleton cards sized like real cards with `aria-busy="true"` on the list; cold failure shows "Could not load the backlog." + Retry.
  - *Given* `connectionState` becomes `stale`, *When* it settles, *Then* the stale list stays visible with badge and Retry.
- Background refresh must not move the focused or hovered card: the diff is applied on the next idle frame or the focused row keeps its position; if the focused item is removed (deleted server-side or pruned by the REST reconcile) **focus moves to the next row** (the previous row when it was last; the list heading with `tabIndex=-1` when the list is now empty), never to `body`, and the polite channel says "<title> was removed" once. The inline "Removed" placeholder is **not** built (it would keep a dead row and extra state; one behavior only). Mutations made while stale still apply to the server result, then update the persisted snapshot.
  - *Given* a focused card at index 5 and a refresh that inserts 3 items above it, *When* the diff applies, *Then* the focused card's viewport position and `document.activeElement` are unchanged.
- Return navigation already hits a warm global store (INFERRED; verified by Story 1.1.5) and is reconciled by the same mount-time REST prune, so hydration helps the hard load only; the cold/warm metrics are reported separately and the cold-load target depends on progressive render (first screenful first, off-screen cards deferred via `content-visibility: auto` + `contain-intrinsic-size` or the existing virtualizer) per the 1.1.5 attribution.
  - *Given* 200 items, *When* hard-loading `/backlog` **cold** (storage cleared), *Then* long-task total < 1 s on the seeded fixture and the delta against the `baseline.json` cold median is reported (requirements' 3.4 s came from another instance and dataset and is not the comparison base); the warm number is reported next to it.
- `sessionStorage` unavailable/full falls back to memory with no user-visible change; corrupt or wrong-version snapshots are discarded silently.
**Files**: `lib/backlog/backlogHydration.ts` (new), `lib/backlog/backlogReconcile.ts` (new), `lib/hooks/useWatchBacklogItems.ts` (record mount ids; call reconcile after the first REST success), `lib/store/backlogItemsSlice.ts`, `app/backlog/useBacklogData.ts`, `components/backlog/BacklogListView.tsx`, `components/backlog/BacklogSkeleton.tsx` (new), tests

##### Task 6.1.2a: Hydration module + slice load + scoped versioned key (M)
- Files: `lib/backlog/backlogHydration.ts`, `lib/store/backlogItemsSlice.ts`, `app/backlog/useBacklogData.ts`, tests

##### Task 6.1.2b: Skeleton, refreshing/out-of-date affordances, focus-stable diff, progressive render (M)
- Files: `components/backlog/BacklogListView.tsx`, `components/backlog/BacklogSkeleton.tsx`, tests

##### Task 6.1.2c: REST-authoritative prune (`reconcileAfterRest`) with the "hydrated item deleted server-side disappears" and warm-store tests (S)
- Pure function in `lib/backlog/backlogReconcile.ts` plus the hook wiring; respects `statusFilter`, in-flight local mutations and stream-touched ids.
- Files: `lib/backlog/backlogReconcile.ts`, `lib/hooks/useWatchBacklogItems.ts`, `lib/store/backlogItemsSlice.ts`, tests

##### Task 6.1.2d: `backlog-revalidation.spec.ts` Playwright spec (S)
- Covers UX criteria 18-23 (cached rows before RPC and clickable during revalidation, `Refreshing` only after 1 s, failure badge with age cue/Retry/polite message, skeleton height and CLS 0, focus-stable diff incl. focus moving to the next row when the focused item is removed, cold-failure Retry) with route interception; `page.clock` only if the Task 1.1.3e spike passes.
- Files: `tests/e2e/backlog-revalidation.spec.ts`

#### Story 6.1.3: Route prefetch, store warm-up and `RoutePending` stall escalation
**Depends on**: 6.1.2, 4.1.4a (`announce()`). `app/RootShell.tsx` is shared with Tasks 5.4.1c and 4.1.5a: no two editors in one wave and any landing order, each rebasing on whichever landed first; none blocks another, because 6.1.3b only adds `aria-busy` to `<main>` and the stall UI.
**Acceptance Criteria**:
- Nav links prefetch the destination chunk and run the **hydration-only** warm-up (read the persisted snapshot into `backlogItemsSlice`) on hover (~100 ms intent) and keyboard focus, once per link per page life, via `@/lib/nav` (never `next/navigation`). **No Backlog watch subscription is started by warm-up**: the hook owns the stream lifecycle (an `AbortController` per effect) and unmounting `/backlog` ends it, so a hover-started subscription would be an owner-less second subscriber whose store goes stale on `/`. The warm-up is exported from `lib/backlog/` (dependency points `app/` -> `lib/`, never `lib/nav` -> `app/backlog/`) and issues **no unary call and no stream**. Items that went stale while away are corrected by the mount-time REST prune (Story 6.1.2), not by warm-up. `prefetch` is part of the `@/lib/nav` facade: `impl.next.tsx` maps it to the Next router prefetch; `impl.router.tsx`/`impl.memory.tsx` (bun/Jest builds, chunks not split) make it a no-op, so no Next-only code leaks past the facade.
  - *Given* a nav link focused by keyboard, *When* focus lands, *Then* the route prefetch is requested once and the hydration warm-up runs once with 0 RPCs and 0 stream subscriptions; repeated focus does not repeat either; in the memory impl `prefetch` is a no-op.
- `RoutePending` keeps `PENDING_DELAY_MS = 150` (`lib/nav/policy.ts:5`; the earlier "~300 ms" would regress feedback speed): nothing before 100-150 ms (previous page remains), skeleton at 150 ms, at ~1 s a stall notice "Still loading..." with **Retry and "Go to Sessions"** (announced once via `announce()`; a persistently slow route is never a dead end even before the timeout), and an error state at `ROUTE_PENDING_TIMEOUT_MS` = 15 000 ms with Retry and "Go to Sessions" (a new constant beside `PENDING_DELAY_MS` in `lib/nav/policy.ts`; route chunk loads are not RPCs, so the 30 s `DEFAULT_RPC_TIMEOUT_MS` does not apply; **provisional**: a chosen default, not measured, to be re-set by the owner from the Task 6.1.3c timing runs). `main` (`RootShell.tsx:25`) gets `aria-busy="true"` while a route is pending (the comment at `RoutePending.tsx:5` claims it but it is not set today).
  - **Stall text placement**: the skeleton stays inside `<main aria-busy="true">`, but the stall notice ("Still loading...", Retry, "Go to Sessions") and the error state render in a **sibling element placed directly after `<main>`** (`data-testid="route-stall"`, laid over the content area with CSS), never inside the `aria-busy` region: assistive tech may ignore updates and descendants inside a busy region, and Retry must stay perceivable. When the error state shows, `aria-busy` is removed from `main` (the route is no longer pending).
  - **Error announcement (one mechanism)**: the error state has **no `role="alert"`**; focus moves to its heading (`tabIndex=-1`), whose text is read once on focus and puts keyboard users beside Retry. `announce()` is not used for the error, so nothing speaks twice. (Rejected variant: keep `role="alert"` and not move focus; that leaves keyboard focus on the nav link with no path to Retry.)
  - **Touch**: Retry and "Go to Sessions" are at least 44x44 CSS px under `(pointer: coarse)`.
  - *Given* a route chunk delayed 1.2 s, *When* navigating, *Then* the skeleton appears at 150 ms, "Still loading..." with Retry and "Go to Sessions" appears at about 1 s in the sibling element (not inside `main`), the polite region announces once, and `main` has `aria-busy="true"` only during the pending period; *Given* a chunk that never arrives, *Then* at the timeout the error heading is focused, no `role="alert"` exists in the DOM, and the heading text is announced exactly once.
- `route-meta.ts` rows unaffected (no new pages); Jest mocks `@/lib/nav` with `requireActual("@/lib/nav/impl.memory")`.
  - *Given* the suite, *When* `route-meta.test.ts` runs, *Then* it passes.
**Files**: `app/RoutePending.tsx`, `app/RootShell.tsx`, `lib/nav/policy.ts`, `lib/nav/index.ts`, `lib/nav/impl.next.tsx`, `lib/nav/impl.router.tsx`, `lib/nav/impl.memory.tsx`, `lib/nav/impl.types.ts`, `lib/backlog/warmBacklog.ts` (new), nav link components, tests

##### Task 6.1.3a: Facade `prefetch` per impl + warm-up on hover/focus (S)
- Files: `lib/nav/*`, `lib/backlog/warmBacklog.ts`, nav components, tests

##### Task 6.1.3b: `RoutePending` escalation and `aria-busy` (S)
- Adds `ROUTE_PENDING_TIMEOUT_MS` (15 s, provisional) to `lib/nav/policy.ts`; renders the stall notice and error state in the sibling `route-stall` element after `<main>` (see the AC) and moves focus to the error heading with no `role="alert"`.
- Files: `app/RoutePending.tsx`, `app/RootShell.tsx`, `lib/nav/policy.ts`, `app/RoutePending.test.tsx`

##### Task 6.1.3c: `route-pending-stall.spec.ts` Playwright spec (S)
- Covers UX criteria 24-27 (escalation timings, Retry and "Go to Sessions" from 1 s, stall text in the sibling element outside `aria-busy`, error heading focused with no `role=alert`, no dead ends, prefetch/warm-up once per link); `page.clock` only if the Task 1.1.3e spike passes.
- Files: `tests/e2e/route-pending-stall.spec.ts`

##### Task 6.1.3d: `a11y-responsiveness-states.spec.ts` (new) plus the sweep extension (S)
- `tests/e2e/a11y-route-sweep.spec.ts` iterates routes only (`:50`, VERIFIED), so it cannot see retained windows or Backlog states. This spec runs axe-core (`wcag22aa`) on `/` with `window_keep_alive` ON and 3 retained windows and on `/backlog` in warm, refreshing, out-of-date and cold states, and covers UX criteria 29-31 (names, focus ring, 4.5:1 contrast in the default and one custom theme). Runs after Tasks 4.1.2b, 6.1.2b and 6.1.3b.
- Files: `tests/e2e/a11y-responsiveness-states.spec.ts`, `tests/e2e/a11y-route-sweep.spec.ts` (only if extended instead)

---

## Phase 5: Regression guards and final measurement

### Epic 7.1: Guards
**Epic exit criteria (every epic, not only 6.1.1)**: `make ready-duplication-gate-web` (jscpd, threshold 0.14%; the plan adds many heavily-mocked test files for transport, policy and fan-out) and `make ready-complexity-gate` (Go `dupl`, new-code-only) pass for the epic's diff.

#### Story 7.1.1: ESLint ban on `createConnectTransport` outside the shared module
**Acceptance Criteria**:
- `no-restricted-imports` (and `no-restricted-syntax` with the selector `ImportExpression[source.value='@connectrpc/connect-web']` for dynamic import, covered by a fixture lint test so the rule is testable) forbids `@connectrpc/connect-web`'s `createConnectTransport` outside `lib/api/transport.ts` and `lib/transport/*`.
  - *Given* a new file importing `createConnectTransport`, *When* `bun run lint` runs, *Then* it errors citing the shared transport.
- The existing nav-facade `no-restricted-imports` entries are preserved.
  - *Given* a file importing `next/navigation`, *When* linted, *Then* it still errors.
- An explicit test-file override keeps the 66 test files that still reference `createConnectTransport` lint-clean.
  - *Given* a `*.test.ts` importing `createConnectTransport`, *When* linted, *Then* it does not error.
**Files**: `web-app/eslint.config.mjs`, `.claude/rules/` (rule doc update optional)
**Depends on**: 2.1.1; lands in the **same PR** as the last migration batch (Task 2.1.1e), not after it.

##### Task 7.1.1a: Add rule + test-file override + fixture lint test (S)
- Files: `web-app/eslint.config.mjs`

#### Story 7.1.2: Jest guard - one flags RPC for N consumers; coalescing contract
**Depends on**: 2.1.4, 2.1.5.
**Acceptance Criteria**:
- A Jest test mounts the providers with 10 `useFeatureFlag`/`useFeatureFlagValue` consumers and asserts 1 `getFeatureFlags` call; another asserts 1 `listProgramsConfig` for 20 `useAvailablePrograms` consumers.
  - *Given* 10 consumers mounting together, *When* settled, *Then* the mocked client's `getFeatureFlags` has been called once.
**Files**: `lib/features/__tests__/flagFanout.test.tsx`, `lib/hooks/__tests__/programsFanout.test.tsx`

##### Task 7.1.2a: Fan-out guard tests (S)
- Files: `lib/features/__tests__/flagFanout.test.tsx`, `lib/hooks/__tests__/programsFanout.test.tsx`

##### Task 7.1.2b: Coverage gate for the new modules (S)
- `web-app/jest.config.js` has no `coverageThreshold` and no `collectCoverage` (VERIFIED by grep), so the 80% line target in `implementation/validation.md` is otherwise unenforced. Add a path-keyed `coverageThreshold` (80% lines) for `src/lib/{api,transport,perf,layout,backlog,telemetry,window,features}/`, active only when run with `--coverage`, and document the explicit Phase 6 command: `cd web-app && bunx jest --coverage --collectCoverageFrom='src/lib/{api,transport,perf,layout,backlog,telemetry,window,features}/**/*.{ts,tsx}' --no-cache`. Go side: `make test-coverage`, reading only the new `ws_unary_*.go` and `ws_bridge_common.go`.
- Files: `web-app/jest.config.js`

#### Story 7.1.3: CI perf budget job
**Acceptance Criteria**:
- A job beside `e2e-latency` in `.github/workflows/benchmark.yml` runs the Playwright `perf` project (`tests/e2e/playwright.config.ts --project=perf`, specs under repo-root `tests/e2e/perf/`), caches baseline with a `bench-e2e-perf-` key, posts a PR comment, and starts advisory.
  - *Given* a PR touching `web-app/src/`, *When* the job runs, *Then* it uploads `perf-results.json` and comments medians vs baseline without failing the check.
- A follow-up flips the **count-based** subset to blocking after a soak window (wall-clock blocker, owner): the per-method request-count assertions from Story 1.1.3 (`GetFeatureFlags <= 1`, `ListProgramsConfig <= 1`, `LogClientEvents <= ceil(duration_s/5)+1`, `GetFileTransportCapabilities <= 1`, `ListWorkflows <= 1`), the status-0 count `== 0` from Story 2.1.7, and the `createConnectTransport` ESLint ban (already blocking in `bun run lint`). Timing-dependent metrics stay **advisory** (reported with a 3-run median, never failing the check): `peakInFlight`, long-task max, `queuedOver1s` and `wsQueuedOver1s`, window-switch and route-change medians. Reason: they depend on shared-runner speed and on the 20 real tmux sessions the fixture needs (Task 1.1.3e), and a flaky blocking check gets skipped or reverted.
  - *Given* the blocking subset enabled and a PR reintroducing 14 `getFeatureFlags` calls, *When* the job runs, *Then* the check fails.
**Files**: `.github/workflows/benchmark.yml`, `tools/ci-gates/perf-budget.js` (new), `tests/e2e/perf/baseline.json`
**Depends on**: 1.1.3

##### Task 7.1.3a: Workflow job + results/gate script, advisory (M)
- Files: `.github/workflows/benchmark.yml`, `tools/ci-gates/perf-budget.js`

##### Task 7.1.3b: Flip deterministic subset to blocking (XS, wall-clock blocker)
- Blocker: soak window and owner sign-off. Files: `.github/workflows/benchmark.yml`

#### Story 7.1.4: Final before/after re-capture
**Depends on**: 5.5.2, 4.1.6, 6.1.3, 7.1.3.
**Acceptance Criteria**:
- Owner re-captures HAR + trace with DevTools closed; results compared against requirements Success Metrics in `research/final-recapture.md`.
  - *Given* the final configuration, *When* the capture is taken, *Then* the note first states the state of `request_coalescing`, `unary_over_ws` and `window_keep_alive` (the window-switch target is met only with `window_keep_alive` ON, Story 4.1.6) so every number is attributable to a flag state.
  - *Given* the HAR, *When* analyzed, *Then* the note reports: unary requests with `blocked` > 1 s (target 0), status-0 stalls (target 0), `GetFeatureFlags` <= 1, `ListProgramsConfig`/`LogClientEvents` batched, window switch < 150 ms, `getBoundingClientRect` self-time -80% (versus `baseline.json`), `/backlog` hard-load long tasks < 1 s, SM6 (INP-style p75 <= 200 ms for the switch and the nav click) and SM7 (echo p95 within +10% of baseline). Every miss is reported under `RULE-target-miss` with the achieved number.
**Files**: `project_plans/frontend-responsiveness/research/final-recapture.md`

##### Task 7.1.4a: Analyze capture and write verdict (S, wall-clock blocker: owner capture)

---

## Test-file ownership

Every new test file named in `implementation/validation.md` has an owning task (the task's Files line gains it by this table). Colocated unit tests not listed here are created by the task that owns the code under test.

| New test file | Owning task |
|---|---|
| `tests/e2e/window-switch-a11y.spec.ts` | 4.1.4d |
| `tests/e2e/window-keepalive.spec.ts` | 4.1.3f |
| `tests/e2e/backlog-revalidation.spec.ts` | 6.1.2d |
| `tests/e2e/route-pending-stall.spec.ts` | 6.1.3c |
| `tests/e2e/a11y-responsiveness-states.spec.ts` | 6.1.3d |
| `tests/e2e/perf/findingsNotes.spec.ts` | 1.1.5a (last of the three note writers: 1.1.1b, 1.1.4a, 1.1.5a) |
| `tests/e2e/perf/ws-unary.spec.ts` | 5.5.1b |
| `lib/api/__tests__/transportCallers.test.ts` | 2.1.1e |
| `lib/__tests__/eslintTransportBan.test.ts` | 7.1.1a (same PR as 2.1.1e) |
| `web-app/scripts/codemods/__tests__/connectTransportToShared.test.ts` | 2.1.1b |
| `lib/hooks/__tests__/usePublishStackTopOffset.test.ts` | 3.1.1b |
| `components/sessions/__tests__/SessionList.measure.test.tsx` | 3.1.2b |
| `lib/terminal/__tests__/TerminalPool.fitOnShow.test.tsx` | 3.1.2a |
| `components/sessions/__tests__/useSessionDetailActivity.test.ts` | 2.1.0a |
| `lib/window/__tests__/useWindowFocusMemory.test.ts`, `lib/window/__tests__/useWindowUrlSync.test.tsx`, `lib/nav/__tests__/announce.test.tsx` | 4.1.4a |
| `components/window/__tests__/WindowTabStrip.a11y.test.tsx`, `components/window/__tests__/WindowPanel.test.tsx` | 4.1.4b |
| `lib/terminal/__tests__/TerminalPool.keepalive.test.tsx` | 4.1.2a |
| `lib/window/__tests__/WindowsProvider.test.tsx` | 4.1.5a |
| `components/backlog/__tests__/BacklogListView.focusStable.test.tsx`, `BacklogListView.a11y.test.tsx` | 6.1.2b |
| `lib/backlog/__tests__/warmBacklog.test.ts` | 6.1.3a |
| `lib/transport/__tests__/WsUnaryBoot.test.tsx` | 5.4.1c |
| `server/services/ws_unary_allowlist_test.go` | 5.2.1d |
| `lib/perf/__tests__/guardsOnTopOfBaseline.test.ts` | 1.1.3b |
| `tools/ci-gates/perf-budget.test.js` | 7.1.3a |
| `lib/api/requestPriority.test.ts` | 2.1.9a |
| `server/ws_unary_origin_test.go` | 5.2.1g |
| `tests/e2e/ws-unary-remote.spec.ts` | 5.5.1d |

## Effort estimate

Basis: bands and weights INFERRED from `ESTIMATION.md`; recalibrate after the first implementer run. Assumed output share is 12% for implementation tasks and 8% for spikes/tests-only tasks. Per-task class midpoints: XS 20k, S 55k, M 130k raw tokens; CU = raw x (1 + 4 x output share), i.e. x1.48 at 12% and x1.32 at 8% (cache reads ignored, so slightly conservative). The table below was re-derived by script from the task counts, replacing the earlier mis-summed table (which stated about 8.5M; the earlier 29 rows actually summed to 8.91M).

| Story | Tasks (count by class) | Class | Est. raw tokens | Output share | Est. CU | Wall-clock blocker |
|---|---|---|---|---|---|---|
| 1.1.1 trace flags | 1 S + 1 XS | S | 75k | 8% | 99k | none |
| 1.1.2 gauge + Resource Timing + marks | 1 M + 1 S | M | 185k | 12% | 274k | none |
| 1.1.3 Playwright perf spec + project (incl. CI spikes) | 2 M + 3 S | M+ | 425k | 12% | 629k | none |
| 1.1.4 slot holders (incl. server-phase capture) | 1 M + 1 S + 1 XS | M+ | 205k | 8% | 271k | owner re-capture (tstapler) |
| 1.1.5 backlog attribution | 1 M | M | 130k | 8% | 172k | none |
| 1.1.6 register four feature flags (Go) | 1 S | S | 55k | 12% | 81k | none |
| 2.1.0 SessionDetailView seam | 2 S | S | 110k | 12% | 163k | none |
| 2.1.1 transport migration (34 edits of 37 files) | 2 M + 3 S | M+ | 425k | 12% | 629k | none |
| 2.1.2 coalescing + write generation | 2 M | M | 260k | 12% | 385k | none |
| 2.1.3 log patch/buffer | 1 M + 1 S | M | 185k | 12% | 274k | none |
| 2.1.4 flags unify + persist | 2 M | M | 260k | 12% | 385k | needs 1.1.1 findings |
| 2.1.5 programs cache | 1 S | S | 55k | 12% | 81k | none |
| 2.1.6 per-pane dedupe | 3 M + 1 S | L (split in 4) | 445k | 12% | 659k | none |
| 2.1.7 Epic 2 exit assertions (metric 1) | 1 S | S | 55k | 12% | 81k | none |
| 2.1.8 server-side contention fix (conditional) | 1 S | S | 55k | 12% | 81k | fires only on the 5.1.2 server-side row |
| 2.1.9 request priority/deferral | 1 M + 1 S | M | 185k | 12% | 274k | none |
| 3.1.1 SharedBodyMeasurer | 2 M | M | 260k | 12% | 385k | none |
| 3.1.2 hidden gating + zero-size resize guards | 2 M + 2 S | M+ | 370k | 12% | 548k | none |
| 5.1.1 WS dispatch spike + vectors | 1 M + 1 S | M | 185k | 8% | 244k | none |
| 5.1.2 decision gate | 1 S | S | 55k | 8% | 73k | owner re-capture (1.1.4b) |
| 5.2.1 server bridge + guards (incl. remote origin policy, 5.2.1g) | 6 M + 1 S | L (split in 7) | 835k | 12% | 1236k | none |
| 5.3.1 client transport (incl. liveness) | 2 M + 2 S | M+ | 370k | 12% | 548k | none |
| 5.3.2 reader queue instrumentation | 1 S | S | 55k | 12% | 81k | none |
| 5.4.1 policy/flag/bootstrap/fallback | 3 M | M+ | 390k | 12% | 577k | none |
| 5.5.1 cross-stack + three-arm + remote/h2 arms (5.5.1d) | 4 M | M+ | 520k | 8% | 686k | owner-run remote arms if headless remote login is infeasible (tstapler) |
| 5.5.2 ADR confirm + flip guide | 1 S | S | 55k | 8% | 73k | measurement data |
| 4.1.1 Activity spike | 1 M | M | 130k | 8% | 172k | none |
| 4.1.2 host + per-window state + focus memory | 2 M | M | 260k | 12% | 385k | none |
| 4.1.3 gating (a..f) | 5 M + 1 S | L (split in 6) | 705k | 12% | 1043k | none |
| 4.1.4 focus/announce/tabpanel/highlight (unflagged) + a11y spec | 2 M + 2 S | M+ | 370k | 12% | 548k | none |
| 4.1.5 lift host above route (provider, geometry, off-route gating) | 3 M | M | 390k | 12% | 577k | none |
| 4.1.6 flip `window_keep_alive` default | 1 S | S | 55k | 8% | 73k | owner dogfood sign-off (tstapler) |
| 6.1.1 backlog hook around watch stream | 2 M | M | 260k | 12% | 385k | none |
| 6.1.2 store hydration + progressive render + spec | 2 M + 2 S | M+ | 370k | 12% | 548k | none |
| 6.1.3 prefetch + warm-up + stall + 2 specs | 4 S | S+ | 220k | 12% | 326k | none |
| 7.1.1 ESLint ban | 1 S | S | 55k | 12% | 81k | none (merges with 2.1.1e) |
| 7.1.2 Jest guards + coverage gate | 2 S | S | 110k | 12% | 163k | none |
| 7.1.3 CI budget | 1 M + 1 XS | M | 150k | 12% | 222k | soak + owner sign-off (tstapler) |
| 7.1.4 final re-capture | 1 S | S | 55k | 8% | 73k | owner capture (tstapler) |

- Sum of CU (39 story rows, 98 task headings, re-summed by script after the remote-listener patch: per-row raw tokens recomputed from the task-class midpoints and per-row CU recomputed from the output share, both matching the stated cells): raw 9.335M tokens, about 13.59M CU (sum of the stated per-row cells, 13,585k). With the verification multiplier x1.5 (review, `6-verify` layers, CI reruns): about 20.38M CU. Remote-listener patch growth from the Phase-4 figure (39 rows, 96 tasks, 9.075M raw, 13.22M CU, 19.83M with x1.5): Task 5.2.1g (remote origin policy, auth gate, CSWSH tests; Story 5.2.1 now 6 M + 1 S) and Task 5.5.1d (remote-access instance, remote/h2 arms, remote e2e; Story 5.5.1 now 4 M), +0.26M raw, +0.36M CU, +0.55M with x1.5; the client `ws`/`wss` derivation, the `unary_over_ws_remote` policy rule and the remote integration variants are folded into existing tasks (5.3.1b, 5.4.1a, 5.5.1a, 1.1.6a) with no class change. Phase-4 growth from the pass-2 figure (37 rows, 11.98M CU, 17.97M with x1.5) is: Stories 2.1.8 (conditional server-side contention fix) and 2.1.9 (request priority/deferral), Tasks 1.1.3e (CI spikes), 1.1.4c (server-phase capture), 3.1.2c/3.1.2d (zero-size resize guards), 5.3.1d (client liveness), 4.1.3f/4.1.4d/6.1.2d/6.1.3c/6.1.3d (the five previously unowned Playwright specs, one of them the a11y states spec) and 7.1.2b (coverage gate). Growth from the pass-1 figure (34 rows, 11.18M CU, 16.77M with x1.5) is the pass-2 additions: Stories 1.1.6 (flag registration), 2.1.7 (metric 1 assertions), 4.1.6 (keep-alive flip), Task 5.2.1f (dispatcher split), Task 5.5.1c raised from S to M (HOL probe and terminal-latency tap), Story 4.1.5 split into 3 tasks, Task 6.1.2c (REST prune). Earlier growth from 13.4M reflects the first repair (Stories 1.1.5, 2.1.0, 5.1.2, 5.3.2, 4.1.5, an expanded 2.1.1, a 5-task 5.2.1, 5.4.1c, the three-arm 5.5.1, and 4.1.3 split into 5 tasks).
- Triad fixes (2026-10-26) added no task and changed no task class (the product, UX and engineering items fold into Tasks 1.1.3b, 4.1.2a/4.1.3f, 4.1.4a/b/d, 6.1.2a-d, 6.1.3b/c and 7.1.4a), so the table is unchanged; re-verified by script after the edits: 39 story rows, 98 task headings (`grep -c '^##### Task'` = 98, table task counts sum to 98), raw 9,335k tokens, 13,585k CU, 20,378k CU with x1.5 (per-row raw and CU recomputed from the class midpoints and output shares, no mismatches).
- Size band: above the Large band (3-15M CU) at the verified total (20.38M with the multiplier; requirements.md carries the same note); informational only. Proceeding with the full scope; no downscale (owner decision).
- Critical path in agent waves (shared-file serialization named):
  1. Wave A: first 1.1.1a (creates `lib/perf/perfDebug.ts`) and 1.1.6 (flag registration, Go; must merge before Wave B) in parallel, then 1.1.2 (second editor of `perfDebug.ts`; also first editor of `lib/api/transport.ts`), then 1.1.3 and 1.1.1b (both need 1.1.2). 1.1.3's `baseline.json` is committed and tagged `perf-baseline` **before any Epic 2 story merges**. Task 1.1.3e (CI spikes) runs before 1.1.3c commits `baseline.json`. Then 1.1.4 (including the new server-phase task 1.1.4c) and 1.1.5 (both need 1.1.3; 1.1.4 also needs 1.1.2). Task 3.1.2c (Go zero-size guards) has no prerequisites and may start in Wave A.
  2. Wave B0: 2.1.0 seam (alone on `SessionDetailView.tsx`). Wave B (parallel, different files, with the fixed edit-order table under Story 2.1.0): 2.1.1 migration (codemod dry run first; `lib/api/transport.ts` editors serialize 1.1.2a -> 2.1.1a -> 2.1.2b -> 2.1.9a -> 5.4.1b) || 2.1.4a flagStore || 2.1.5 programs; then 2.1.2 coalescing (needs 2.1.4a and 1.1.6) || 2.1.4b (needs 1.1.1 findings) || 2.1.3 log (after 2.1.1d on `TerminalOutput.tsx`) || 2.1.6 (touches `SessionDetailView.tsx`; fixed order 2.1.0 -> 2.1.6c -> 3.1.1b -> 4.1.3b/c/d). Then 2.1.9 (request priority/deferral, after 2.1.2b; fifth `lib/api/transport.ts` editor) and 2.1.7 (numeric metric 1 assertions). Story 2.1.8 (server-side contention fix) runs only if the Story 5.1.2 server-side row fires.
  3. Wave B2: 3.1.1 measurer (after 2.1.0 and 2.1.6c) || 3.1.2 hidden gating.
  4. Wave C: 5.1.1 spike + 5.1.1b vectors/stubs (needs only 2.1.1; may start in Wave B) -> 5.2.1 (5.2.1f dispatcher split first) || 5.3.1 (parallel; no shared file because 5.1.1b created the vectors and Go/TS harnesses) -> 5.4.1a/b -> 5.4.1c. These do **not** wait for the 5.1.2 gate. Story 5.1.2 (needs Epic 2, 2.1.7 and 1.1.4; informs default, 5.4.1c priority and the priority socket; never cancels) runs in parallel and holds only 5.5.1 -> 5.5.2 and the default flip. 5.3.2 is independent.
  5. Wave D (parallel with Wave C): 4.1.4 (unflagged a11y incl. `WindowPanel`; independent of the flag; 4.1.4a before 6.1.3b, 4.1.4b before 4.1.2); 4.1.1 spike -> 4.1.2 (needs 4.1.4b) -> 4.1.3 (needs 3.1.2, 2.1.6a/c/d) -> 4.1.5a/b/c (needs 4.1.3, 4.1.4; `app/RootShell.tsx` serialized with 5.4.1c and 6.1.3b, any landing order) -> 4.1.6; 6.1.1 (needs 1.1.5 note and 2.1.1c merged) -> 6.1.2 -> 6.1.3 (needs 4.1.4a).
  6. Wave E: 7.1.1 (same PR as the last 2.1.1 batch), 7.1.2 (after 2.1.4/2.1.5), 7.1.3 (after 1.1.3), then 7.1.4.
  Longest serial chain: 1.1.1a -> 1.1.2 -> 1.1.3 -> 2.1.0 -> 2.1.1 -> 5.1.1b -> 5.2.1f -> 5.2.1 -> 5.4.1 -> 5.5.1 (also needs Epic 2 + 1.1.4 + the 5.1.2 gate) -> 5.5.2 -> 7.1.4 (about 10 waves).
- Overrun checkpoint: anchored on the corrected totals (39 rows). If spend on completed stories exceeds 1.25x their cumulative table CU (the table CU, not the earlier mis-summed 8.5M), pause and report the delta and its cause. Informational, not a call to cut scope.
- Wall-clock blockers: (1) owner re-capture of HAR/trace with DevTools closed (tstapler) for Stories 1.1.4/5.1.2 and 7.1.4; (2) CI soak window and owner sign-off before blocking mode (tstapler, Task 7.1.3b); (3) owner dogfood of `unary_over_ws` on the live instance before widening `wsUnaryAllowed` (tstapler); (4) CI queue time for the new perf job on first run; (5) owner resolution of `OPEN-DECISION-throttle-gate` before the Story 4.1.6 flip.

## Self-review notes
- Cross-references re-checked 2026-10-26 after the repair: ADR file names exist under `decisions/`; research files referenced by name (`flags-trace.md`, `slot-holders.md`, `backlog-attribution.md`, `ws-unary-seams.md`, `ws-gate.md`, `ws-three-arm.md`, `activity-spike.md`, `final-recapture.md`) are outputs of stories, not pre-existing.
- `createConnectTransport` recount: 37 non-test files (2 stay, 2 no-change, 33 migrate, plus `useShells.ts`), 66 test files.
- Paths resolved: `lib/hooks/useBufferFreshness.ts` (exists), `web-app/eslint.config.mjs` (exists), perf specs at repo-root `tests/e2e/perf/`. `marshalProtoEnvelope`/`envelopeBufPool` at `server/services/connectrpc_websocket.go:49-63` are real; the earlier `encodeEnvelope` name does not exist and is no longer cited.
- Pass-2 facts re-verified by command or Read in this repair (see the pass-2 rows below): gorilla/websocket `conn.go:933-935` (1009 close, `ErrReadLimit`); `knownFeatureFlags` at `feature_flag_service.go:214` and the `config_surface_*` harness; `transport.ts:32-39` (auth + timeout only, JSON); `runUnaryCall` exported by `@connectrpc/connect` 2.1.1; `useWatchBacklogItems.ts:128,178-208,318-326`; `backlogItemsSlice.ts:43-50`; `hostguard.go:37-42`; `SetCookie` only in login handlers; `playwright.config.ts` (`timeout` 30000, `workers: 1`, `chromium`/`chromium-dom` the only `testIgnore` projects); `TerminalPool.tsx:251,296-307`; `RootShell.tsx` used by both `layout.tsx` and `main.tsx`; `lib/api/__tests__/transport.test.ts` exists, `lib/api/transport.test.ts` does not.
- Not verified by running anything: all measurements and numeric thresholds are targets, not results; the estimate basis is INFERRED.

## Review disposition

Sources: `adversarial-review.md` (AR), `architecture-review.md` (AX), `design/ux.md` "Gaps the plan must fix" (UX). Status: fixed / rejected+reason / deferred+reason. Facts re-verified in the repo before encoding: perf harness location (`tests/e2e/global-setup.ts`, `pages/`, project list in `tests/e2e/playwright.config.ts`), `marshalProtoEnvelope`/`envelopeBufPool` (`connectrpc_websocket.go:49-63`), `isAllowedOrigin` at `:104`, `useBufferFreshness` path, `guarded_procedures.go:91-113`, `ws_stream_bridge.go:78-92`, `shortcutRegistry.ts:95-100`, `idempotency_level` count 0, `useWatchBacklogItems` at `app/backlog/page.tsx:273`, `PENDING_DELAY_MS = 150`, `Record<string, boolean>` flags, the 37-file transport grep (re-listed and pinned in Story 2.1.1).

| Finding | Disposition | Where / reason |
|---|---|---|
| AR B1 gate Epic 5 on go/no-go, three-arm, bootstrap hole | **Fixed, reframed** | Story 5.1.2 (decision gate), Story 5.5.1 (arms A/B/C, numeric criteria), Task 5.4.1c + ADR-003 (early-open at boot with persisted flag; `GetFeatureFlags` never WS), Story 1.1.4 saturation probe, Goal claim withdrawn. The reviewer's "re-scope or defer Epic 5 if E2 alone suffices" is **rejected** per the owner decision: the gate sets rollout default, priority and priority-socket topology only. |
| AR B2 e2e location, `encodeEnvelope`, `useBufferFreshness`, stale IDs, diagram, parallel conflict, estimate | **Fixed** | Specs moved to repo-root `tests/e2e/perf/` with a `perf` project and `testIgnore` on all default projects (Task 1.1.3d); `encodeEnvelope` removed (ADR-001 drops the envelope for unary); `lib/hooks/useBufferFreshness.ts`; all story IDs renumbered and a numbering note added; dependency diagram rewritten; vectors/stubs created once in Task 5.1.1b; estimate re-derived by script (34 rows, about 11.18M CU, x1.5 = 16.77M). `isAllowedOrigin` corrected to `:104`. HostGuard question closed. |
| AR B3 per-method flag impossible | **Fixed** | Master boolean `unary_over_ws` + Go-side `wsUnaryAllowed` + client `wsEligible` subset by shared fixture; pre-load defaults defined (Risk Control, ADR-002 item 3). |
| AX A1 per-procedure guard/body-limit/auth parity | **Fixed** (nuance below) | Story 5.2.1, Task 5.2.1d, ADR-002 items 1-2, 7: re-enter the listener outer chain with upgrade headers, deny-by-default table, route `/api/ws/unary`, tests over real `localChain()`/`remoteChain()`, ADR-001 chunk/FIN. Nuance: `LocalWriteGuard` is installed only when no auth middleware is configured (`guarded_procedures.go:103-113`); with auth, auth is the boundary. The review's description was slightly broader; ADR-002 states the accurate version. |
| AX A2 Backlog builds on watch stream; measure first | **Fixed** | Story 1.1.5 (attribution, cold vs cached separated), Stories 6.1.1-6.1.3 rewritten around `useWatchBacklogItems` + `backlogItemsSlice`; no `listBacklog` snapshot layer. |
| AX C1 split `isActive` into `isFocused`/`windowActive` | **Fixed** | Glossary, Task 2.1.0a `useSessionDetailActivity`, Stories 3.1.1, 4.1.3. |
| AX C2 / seam Task 2.1.0 and edit order | **Fixed** | Story 2.1.0 (Tasks 2.1.0a/b), fixed order under it and in Wave B. |
| AX C3 ADR-001 envelope mismatch, chunk/FIN | **Fixed** | ADR-001 rewritten: raw proto, CHUNK type 7, `totalLen`/`fin`, `SetReadLimit`, >256 KiB requests HTTP-only. |
| AX C4 route, timeout, contexts, ping/pong | **Fixed** | Story 5.2.1 ACs. |
| AX C5 keep-alive survives navigation | **Fixed** | Story 4.1.5 (hoist above route boundary) and the `/` -> `/backlog` -> `/` scenario in Story 1.1.3. The "accept and document partial" alternative was not taken. |
| AX C6 per-window state/refs/dispatch; `usePaneShortcuts`; registry bug | **Fixed** | Story 4.1.2 ACs, Task 4.1.3d (owner-token cleanup fix at `shortcutRegistry.ts:95-100`). |
| AX C7 layering for prefetch/warm-up | **Fixed** | `lib/backlog/warmBacklog.ts`, `lib/backlog/backlogHydration.ts` (Stories 6.1.2/6.1.3). |
| AX C8 split coalesce/replay/cache columns | **Fixed** | `IdempotentMethodTable` (Task 2.1.2a, ADR-002 item 5). |
| AX C9 37-file migration, rpcTiming kept, ESLint same PR | **Fixed** | Story 2.1.1 (pinned list, codemod dry run, rpcTiming test, Task 2.1.1e). |
| AX C10 `ws_unary_fallback_total` client-only | **Fixed** | Observability Plan, ADR-002 item 10. |
| AX C11 unresolved-question ownership | **Fixed** | Unresolved Questions updated; coalescer stays defense in depth behind Story 1.1.1. |
| AX nitpicks (baseline source, "branded" types, `IdempotentMethodSet` name, flip guide registry step, 5.2.1 class) | **Fixed** | Story 3.1.1 text, Glossary, Story 5.5.2, estimate (5.2.1 now 4 M + 1 S). |
| AR concern: `queuedOver1s` via Resource Timing; runtime opt-in | **Fixed** | Story 1.1.2, Observability Plan. |
| AR concern: coalescing returns pre-write read | **Fixed** | Story 2.1.2 per-session write-generation rule + test. |
| AR concern: `idempotency_level` absent | **Fixed** | Explicit hand-maintained table + descriptor-membership test (Task 2.1.2a, ADR-002 item 5). |
| AR concern: bounding `fromWebSocket` queue | **Deferred** | Story 5.3.2 instruments only; a bound would risk ending live terminal streams kept open by Story 4.1.3 and is outside the unary goal. A future bound must be a separate flagged change with backpressure that exempts terminals. |
| AR concern: window-switch metric holds only for retained set; end mark | **Fixed** | Story 1.1.2 (end mark after fit + focus), Task 4.1.4c (cold switch reported separately, explicitly outside the 150 ms metric), Story 1.1.3 states throttle. |
| AR concern: no numeric acceptance for `LogClientEvents`/`ListProgramsConfig` | **Fixed** | Story 1.1.3 per-method assertions, carried into the Story 7.1.3 blocking subset. |
| AR concern: hard-load metric inflated by cache | **Fixed** | Cold/warm separated (Stories 1.1.3, 1.1.5, 6.1.2). |
| AR concern: `dupl`/jscpd | **Fixed** | Shared upgrader helper (Task 5.2.1b); duplication gates in Epic 7.1 exit criteria. |
| AR concern: kill-switch latency | **Fixed** | Risk Control, Story 5.4.1: within <= 30 s, or immediately via breaker/`ssq:ws-unary:force-http` override. |
| AR concern: task granularity (4.1.3c, 2.1.1b/c) | **Fixed** | Tasks 4.1.3c (polling) / 4.1.3e (terminal+WebGL) split; 2.1.1 file lists pinned. |
| AR concern: prefetch under dual build | **Fixed** | Story 6.1.3: facade `prefetch` per impl, no-op in memory/router impls. |
| AR concern: estimate sum | **Fixed** | Re-derived; note that the corrected total (16.77M with x1.5) sits just above the 3-15M band; recorded as informational, full scope kept. |
| AR minors (noarchivedrevival wording, Files lines, spike test owner, fuzz+integration, remote mode, ESLint test override) | **Fixed** | Story 4.1.3, Task 7.1.2a, Task 5.1.1a owner = Task 5.2.1b, Story 5.2.1/5.5.1a, Risk Control (`:8444` disabled; **superseded 2026-10-26 by the owner decision to include the remote listener**, see "Owner decision reversal"), Story 7.1.1. |
| UX gap 1 focus rule by trigger; `aria-selected` | **Fixed** | Story 4.1.4. |
| UX gap 2 tabpanel/`aria-controls`, `aria-busy`, `announce()`, per-window focus memory, handoff before `inert` | **Fixed** | Stories 4.1.2, 4.1.4, 6.1.3. `inert` focus-drop behavior remains INFERRED until the Playwright test runs. |
| UX gap 3 150 ms, `pendingWindowId`, unflagged a11y, cache scoping, refresh must not move focused card | **Fixed** | Stories 6.1.3, 4.1.4, 6.1.2. |
| Nothing could not be resolved by editing the plan | n/a | Open items that need measurement, not planning: whether Chrome counts WS handshakes against the HTTP cap (Story 1.1.4), the `/backlog` cost split (Story 1.1.5), `<Activity>` compatibility (Story 4.1.1), the 14x `GetFeatureFlags` cause (Story 1.1.1). All thresholds are targets, none measured. |

### Pass 2 (inputs: `adversarial-review-r2.md` (AR2), `architecture-review-r2.md` (AX2))

Facts re-verified before encoding (command or Read this session): gorilla/websocket `conn.go:933-935` and `:1124`, `go.mod:35`; `feature_flag_service.go:214`; `config_surface_helpers_test.go:62-66`, `config_surface_proof_test.go:186-207,363-369`; `transport.ts:32-39`; `rpcTiming` wiring (`useShells.ts:59`, `useImportSessionService.ts:57` only); `runUnaryCall` in `@connectrpc/connect` 2.1.1; `useWatchBacklogItems.ts:128,178-208,318-326`; `backlogItemsSlice.ts:43-50,122`; `hostguard.go:37-42`; `isAllowedOrigin` `connectrpc_websocket.go:104-120`; `SetCookie` call sites; `playwright.config.ts` lines 51/59/65/144/189; `TerminalPool.tsx:251,296-307`; `RootShell.tsx`/`layout.tsx`/`main.tsx`; `guarded_procedures.go:103-115`, `server.go:1941-1948`; `server/streaming_chain_test.go` package name; `lib/api/__tests__/transport.test.ts`.

| Finding | Disposition | Where / reason |
|---|---|---|
| AR2 B1 oversize frame / `SetReadLimit` | **Fixed** (verified true) | gorilla closes 1009 and returns `ErrReadLimit` (`conn.go:933-935`; no `SetReadLimit` in `server/` today). ADR-001 and Story 5.2.1: application-layer `len(msg)` check answers `resource_exhausted` per id with the socket open; `SetReadLimit(4 * maxFrameBytes)` is only a hard 1009 backstop; ACs for both cases; Task 5.2.1c. |
| AR2 B2 flags not where the plan said; no registration task | **Fixed** (verified true) | Flags live in `knownFeatureFlags` (`feature_flag_service.go:214`); Story 1.1.6 / Task 1.1.6a registers all three with defaults and `exempt:` proof entries (`pendingCeiling` stays 0), merges before Wave B; every `lib/features/features/*` flag-row reference removed; Stories 5.5.2 and 4.1.6 name the Go default file plus the `flagStore.ts` constant. |
| AR2 B3 HOL probe not executable; criteria 4-5 unmeasurable | **Fixed** | `wsEligible`/`wsUnaryAllowed`/`coalescible`/`replayable` enumerated in Task 2.1.2a (9 / 9 / 10 / 10 methods) and the fixture; `GetFileContent` added as the large-read member; `probe-invalid` rule; criterion 4 numerator/denominator and minimum sample; criterion 5 echo-marker `performance.now()` tap; Task 5.5.1c raised to M. |
| AX2 B1 backlog hydration vs REST-on-mount; ghosts; warm-up owner | **Fixed** (verified true) | `useWatchBacklogItems.ts:178-208` runs REST every mount; stream has no end-of-snapshot marker with items (`:318-326`). Story 6.1.1 AC rewritten to the true contract (1 REST + 1 subscribe per mount); Story 6.1.2 makes REST the authoritative prune (mount-time ids, stream-touched and in-flight-mutation exclusions, filter scope), new Task 6.1.2c with ghost tests; hydration uses `isSnapshot=true` (`false` is live-event only, `backlogItemsSlice.ts:43-50`); Story 6.1.3 warm-up is hydration-only, no subscription. |
| AX2 B2 `meta.headers` override | **Fixed** | ADR-001/002 and Story 5.2.1: explicit `clientHeaders` allowlist, forbidden keys answer `invalid_argument`, guard headers only from the upgrade request, parity test; remote `isAllowedOrigin` any-HTTPS leniency handled: local covered by HostGuard (`hostguard.go:37-42`), remote upgrade refused 403 `ws_unary_remote_disabled` (chosen over copying the auth primary-domain rule; revisit when remote is measured) with a cross-origin test. **Superseded 2026-10-26** by the owner decision to serve the remote listener: the refusal and its marker are gone; the same concern (`isAllowedOrigin` any-HTTPS leniency) is now answered by `wsUnaryOriginPolicy` on the remote route (Task 5.2.1g, ADR-002 item 7), see "Owner decision reversal". |
| AX2 B3 wire-parity AC false; no metadata | **Fixed** (verified true) | ACs compare decoded messages; ADR-001 RESPONSE/ERROR gain `{headers, trailers}` meta (excluding `Set-Cookie`), included in the 5.1.1b vectors; 256 KiB budget measured on proto frame bytes. |
| AR2 C1 Wave A deps / conflict pair | **Fixed** | `Depends on 1.1.2` on 1.1.3/1.1.4; Task 1.1.1b waits for 1.1.2a; 1.1.1a before 1.1.2a on `perfDebug.ts`; Wave A text rewritten. |
| AR2 C2 a-g parallel/prerequisite gaps; AX2 shared-file serialization | **Fixed** | Fixed edit-order table under Story 2.1.0 for `SessionDetail.tsx`, `PaneSplitRenderer.tsx`, `TerminalOutput.tsx` (2.1.1d -> 2.1.3b -> 3.1.1b), `useSessionVcs`/`useVcsStatus`/`useWorkflows`/`useBufferFreshness`, `XtermTerminal.tsx`, `app/page.tsx`, `app/RootShell.tsx` (hedge removed; the shared file is `RootShell.tsx`, used by `layout.tsx` and `main.tsx`), `flagStore.ts`; 2.1.2b waits for 2.1.4a (split 2.1.4a/2.1.4b dependencies); 4.1.4b/4.1.2a ordering via `WindowPanel`; 6.1.3 needs 4.1.4a; 4.1.5 needs 4.1.3/4.1.4; 4.1.3c needs 2.1.6a/d. |
| AR2 C3 / AX2 C1 Go test placement; chain plumbing | **Fixed** (verified true) | Spike, chain-parity and `guardedProcedures`-vs-allowlist tests moved to `package server` (`server/ws_unary_spike_test.go`, `server/ws_unary_chain_test.go`); bridge takes an injected dispatcher; new Task 5.2.1f splits `localChainWith`/`remoteChain` and builds the listener-name map; unset dispatcher fails closed; `guarded_procedures.go` added to Files. |
| AR2 C4 gate serializes unconditional build | **Fixed** | Header, Story 5.1.2, Wave C: 5.2.1/5.3.1/5.4.1a-b start after 5.1.1b; gate holds only 5.4.1c priority, 5.5.1, 5.5.2 and the default flip. |
| AR2 C5 metric coverage (1: numeric Epic 2 assertion + remediation; 3: flag states, flip story; cold long tasks) | **Fixed** | Story 2.1.7 (numeric `queuedOver1s == 0`, status-0 `== 0`, remediation chain); Story 5.5.1 remediation list (a)-(c); Story 4.1.6 flips `window_keep_alive` on numeric criteria; Task 4.1.4c adds the cold "no task > 100 ms" assertion and states flag states; Story 7.1.4 states flag states. |
| AR2 C6 coalescer pre-write read for sessionless reads | **Fixed** | Story 2.1.2: per-session plus global write generation; AC `updateFeatureFlag` then `getFeatureFlags`. |
| AR2 C7 interceptors/timeout on WS path | **Fixed, one fact corrected** | Policy transport outermost (coalescer, auth, rpc timing, 30 s default) via `runUnaryCall`; ACs in 5.3.1/5.4.1. Correction: the review says auth, timeout **and rpcTiming** live inside `createConnectTransport`; verified that the shared factory has only auth + `defaultTimeoutMs` (`transport.ts:32-39`) and rpcTiming is wired per hook in 2 files. Story 2.1.1/Task 2.1.1a now add a module-level rpc-timing sink so the shared transport carries timing (the pass-1 AC asserting it already did was wrong). |
| AR2 C8 flag persistence edge cases | **Fixed** | One rule: only `unary_over_ws` persisted; `request_coalescing` fixed pre-load ON; key removed when flag absent/OFF; 401 upgrades do not count toward the breaker (Story 5.4.1, Task 5.4.1c). |
| AR2 C9 three-arm comparability | **Fixed** | Arm A re-run from `perf-baseline` in the same session; MAD noise rule defined in Story 1.1.3 and applied to criterion 2. |
| AR2 C10 `perf` project config | **Partly fixed, partly rejected** | Fixed: timeout override, `testIgnore` only on `chromium`/`chromium-dom` (verified), CI seeding feasibility check in Task 1.1.3a. Rejected: "needs `workers: 1`": already global (`playwright.config.ts:59,65`, `fullyParallel: false`); the plan now cites it instead of adding it. |
| AR2 C11 / AX2 tabpanel unflagged vs host | **Fixed** | Unflagged `WindowPanel` created in Task 4.1.4b wraps the single container with the flag OFF; host reuses it; AC no longer points `aria-controls` at nothing; focus handoff moved to Task 4.1.2a. |
| AR2 minors: wrong line cites, missing `transport.test.ts`, frame-size boundary, JSON vs proto, ADR status | **Fixed** | `useWatchBacklogItems.ts:128`, `useWindowUrlSync.ts:55`; the real test file is `lib/api/__tests__/transport.test.ts` (the plan's path was the wrong one; the file does exist); client threshold on whole-frame size with boundary AC; decoded-message comparison; Status line says build proceeds on Proposed and the spike records Accepted. 4x-throttle strictness kept advisory as stated. |
| AX2 concern: 250 ms rule per connect attempt | **Fixed** | ADR-002 item 4, ADR-003, Task 5.4.1c with a timing AC. |
| AX2 concern: auth expiry on long sockets (sliding session) | **Rejected as moot, kept as a spike check** | Grep: `SetCookie` appears only in login handlers (`auth/handlers.go:284,378`, `auth/localtoken.go:136`), none in `middleware/auth.go`; sessions are fixed-TTL, so dropping `Set-Cookie` loses nothing. Task 5.1.1a still confirms by running past the TTL. |
| AX2 concern: `TerminalPool`/`webglBudget` with K=3 | **Fixed** | Story 4.1.2: hidden windows stay docked (pool may exceed `maxSize`, accepted), a session is never docked in two windows (older window evicted), evicted WebGL not regained (accepted), `isVisible()` wired to `windowActive`; parse/write CPU accepted. |
| AX2 concern: Story 4.1.5 underscoped; build `WindowsProvider` first | **Fixed, variant** | Split into 4.1.5a (state move), 4.1.5b (portal/geometry parity into a slot in `<main>`), 4.1.5c (ref/focus gating); 4.1.2b builds a provider-shaped `useWindowsController()` that 4.1.5a lifts, instead of targeting the provider directly (smaller blast radius while the flag is OFF). |
| AX2 concern: Story 2.1.0b "no behavior change" | **Fixed** | 2.1.0b passes the literal `true` (preserving ungated keys); the unfocused-pane change is the deliberate Task 4.1.3b behavior change with an inverting test. |
| AX2 concern: warm-up owner | **Fixed** | Warm-up is hydration-only plus chunk prefetch; no subscription (see AX2 B1). |
| AX2 nitpicks | **Fixed** | ADR-001 "odd ids" replaced by reserved even ids; CANCEL drops queued CHUNKs; accepted parse/write CPU stated; ESLint dynamic-import AST selector named with a fixture test. |
| Open after pass 2 | n/a | Needs measurement, not planning: all thresholds are targets; whether the server caps `GetFileContent` size (probe records the actual size and fails as `probe-invalid` if under 1 MiB); the `4 * maxFrameBytes` backstop value is a choice, not measured; `inert` focus-drop behavior remains INFERRED until Playwright runs. |

### Phase 4 patch (inputs: `pre-mortem.md` (PM), `consistency-review.md` (CR), `validation.md` "Gaps and Corrections Found During Validation" (G))

Owner decision unchanged: unary-over-WebSocket is built; no item below drops or defers it. Facts re-verified in the repo before encoding (command or Read this session): `server/services/connectrpc_websocket.go:1160-1180` (`performInitialResizeNudge`, nil check only), `:2558` (`<= 0` guard only in the resync path), `:3237-3262` (capture-pane nudge, nil check only), `session/instance_tmux.go:1285-1292` (`ResizePTY` -> `SetWindowSize`, no dimension check), `session/resize/nudge.go:27-32` (`WithForcedRedraw`), other `ResizePTY` callers `session_service_stream_terminal.go:465` and `connectrpc_websocket.go:3508`; `server/services/session_service_shells.go:62-73,218` (`ListShells` -> `resolveSessionTitle` -> `ListInstanceData`); `web-app/src/lib/api/transport.ts:18` (`DEFAULT_RPC_TIMEOUT_MS = 30_000`); `TerminalPool.tsx:83,252` (max size 8, docked never evicted); `webglBudget.ts:7`; `@bufbuild/protobuf` 2.11.0 `clone.d.ts:6`; `@connectrpc/connect` `call-options.d.ts:33` (`contextValues`); `watch-ws-transport.ts:217-231` (https-only protocol gate) and `main.go:888` (remote listener is HTTPS); `lib/hooks/useTerminalStream.ts:325-387` (handshake dimensions); `web-app/src/lib/shortcuts/__tests__/shortcutRegistry.test.ts` (exists; the unprefixed path does not); `web-app/jest.config.js` (no `coverageThreshold`/`collectCoverage`); `tests/e2e/a11y-route-sweep.spec.ts:50` (route loop only).

| Finding | Disposition | Where / reason |
|---|---|---|
| PM P1 #1 stall may be server-side | **Fixed** (verified true) | Story 1.1.4 server-phase capture AC + Task 1.1.4c; Story 5.1.2 table gains the server-side row (and row 2 now schedules ADR-003 option C, resolving CR C9); bridge in-flight cap starts at 6 (Story 5.2.1, ADR-002 item 8, shared fixture `maxInFlight`); Story 5.5.1 criterion 6 (server handler p95 within +10% of arm B); Story 2.1.8 (conditional backend fix, in scope only when the row fires, noted as the exception to Out of Scope); Unresolved Questions name the server hypothesis. Correction: PM cites the 30 s constant at `transport.ts:19`, the line is `:18`. Note: with cap 6 and the client `maxInFlight` window, WS concurrency equals the HTTP cap until evidence supports raising it (intended). |
| PM P1 #2 zero-size resize reaching tmux | **Fixed** (verified true, one location corrected) | Story 3.1.2 ACs + Tasks 3.1.2c (server guards on both nudge paths and `Instance.ResizePTY`) and 3.1.2d (client handshake guard); Task 4.1.4c asserts tmux pane dimensions through a fixture helper. Corrections: PM puts the client guard in `TerminalOutput.tsx`/`XtermTerminal.tsx`; the handshake dimensions are built in `useTerminalStream.ts:325-387` and the resync path `useTerminalFlowControl.ts:205,361`. PM's `cols < 2 || rows < 1` is applied on the two nudge paths only; `Instance.ResizePTY` uses the broader invariant `<= 0` (two other callers exist). The minimum of 2 is a chosen guard, INFERRED. The Playwright dimension check reads tmux through a fixture helper because `GetPaneDimensions` is a Go method, not an RPC. |
| PM P1 #3 half-open socket | **Fixed** | Story 5.3.1 liveness AC + Task 5.3.1d (PING 10 s while in flight and on wake, PONG deadline 5 s, feeds the breaker); Story 5.4.1 per-call `WS_CALL_TIMEOUT_MS = 8000` before HTTP replay (INFERRED value above the slowest HAR read, recalibrated from arm C; does not count toward the breaker; fallback reason `ws-call-timeout` added to criterion 4); ADR-002 item 8 states symmetric liveness. |
| PM P2 #4 docked-terminal ceiling | **Partly fixed, partly deferred** | Story 4.1.6 criterion 6 now has provisional numeric targets (docked and streams <= 12, heap <= 1.5x flag-OFF after 30 switches, echo p95 +10%), labeled INFERRED and owner-revisable. The hidden-window stream pause/throttle option is **deferred**: a follow-up story only if criterion 6 fails (building it now would add a second behavior to the flag-ON path without evidence). |
| PM P2 #5 coalescer shared protobuf objects | **Fixed** (verified) | Story 2.1.2 AC + Task 2.1.2b: per-subscriber `clone(schema, msg)` and a mutation test. |
| PM P2 #6 Chrome WS handshake serialization | **Fixed** | Story 1.1.4 probe AC + Task 1.1.4a; Task 5.4.1c early-open ordering depends on it (`WsUnaryBoot` renders ahead of stream subscribers); ADR-003 consequence added. Still INFERRED until the probe runs. |
| PM P2 #7 CI flake | **Fixed, slightly widened** | Story 7.1.3: blocking subset is count-based only (per-method counts, status-0 `== 0`, ESLint ban); `peakInFlight`, long-task max, window-switch/route-change medians and `queuedOver1s`/`wsQueuedOver1s` are advisory with a 3-run median. PM named `queuedOver1s` neither way; it is timing-dependent so it is advisory in CI (the perf spec and Story 2.1.7 still hard-fail on it). Task 1.1.3e spikes the 20-session CI seeding (G5). |
| PM P2 #8 replay ordering | **Fixed** | Story 5.3.1 reply-ordering AC, Task 5.3.1c: late replies for settled, cancelled or replayed ids are dropped. |
| PM P3 #9 guard drift, #10 mutation duplication, #11 flag bootstrap | **Fixed where actionable, otherwise no change** | #10: the flip guide (Story 5.5.2) states no non-`coalescible` method is added to `wsUnaryAllowed` without an idempotency key in the proto. #9: existing tests already pin it (no new work, per PM). #11: no change beyond the existing 250 ms rule and force-HTTP override (per PM). |
| CR B1 `ux.md` S6 vs Story 6.1.3 | **Fixed** | `design/ux.md` S6 and AC27 edited to hydration-only (no stream, no RPC); matches Story 6.1.3. |
| CR B2 / G2 throttle gate | **Not resolved (owner decision pending); made single-edit** | Named decision point `OPEN-DECISION-throttle-gate` (plan Open Decisions) with both options neutral; `design/ux.md` AC13, Story 1.1.3, Task 4.1.4c, Story 4.1.6 criterion 1, validation G2/SM3/UX row 13 and requirements Success Metrics all point at it. Interim: both numbers recorded, neither hard-fails; blocks only the Story 4.1.6 flip. |
| CR C1 / G7 `LogClientEvents` bound | **Fixed** | One number: `<= 5` (= `ceil(16/5)+1`) in Story 1.1.3 and validation. |
| CR C2 fixture cannot exercise K = 3 eviction | **Fixed** | Fixture shape: 20 sessions, 4 windows (w1 = 8 panes, w2-w4 = 4 panes), named `perfFixtureShape` in Task 1.1.3a; validation updated. |
| CR C3 priority/deferral coverage | **Fixed** | New Story 2.1.9 (classification, `contextValues` priority interceptor, deferred concurrency cap 3, per-class `queuedOver1s` assertion), Tasks 2.1.9a/b; `contextValues` verified in the installed package. |
| CR C4 requirements staleness | **Fixed** | `requirements.md`: size band note (above Large), owner transport decision, resolved open questions, 34 edits of 37 files explained (33 migrate + `useShells`; 2 stay; 2 no-change). |
| CR C5 scope drift | **Fixed (recorded as accepted extensions)** | Requirements In Scope lists WS channel, unflagged a11y/stall fixes (rollback by PR) and zero-size guards. Story 5.3.2 stays (instrumentation only, still costed): CR nitpick 15's "drop it from the cost table" is rejected because the data sizes any future queue bound. |
| CR C6 gauge "dev-only" | **Fixed** | Glossary, requirements Observability and `ux.md` S7 now say runtime opt-in. |
| CR C7 `ux.md` terminology | **Fixed** | `BacklogStoreHydration`, `statusFilterKey`, S6 wording. |
| CR C8 interceptors double-applied | **Fixed** | Interceptors applied once, outermost on the policy transport; `UnaryWsTransport` calls `runUnaryCall` with an empty list; inner HTTP transport carries none; test asserts `rpc:<Method>` once per call per path (Stories 5.3.1/5.4.1, ADR-002 item 7). |
| CR C9 option C trigger | **Fixed** | Story 5.1.2 row 2 schedules ADR-003 option C; header names (d). |
| CR C10 fixture ordering | **Fixed** | `ws-unary-allowlist.json` is created by Task 2.1.2a; Task 5.1.1b reuses it. |
| CR C11 5.2.1 vs spike | **Fixed** | Story 5.2.1 depends on Task 5.1.1a for 5.2.1f/5.2.1b; diagram updated. |
| CR C12 RootShell order | **Fixed (accepted, relaxed)** | Serialized (never same wave) but any landing order; Story 6.1.3 no longer waits for 5.4.1c/4.1.5a. |
| CR C13 Story 4.1.4 vs host | **Fixed** | `useWindowFocusMemory` moved to Task 4.1.4a with a flag-OFF fallback; Task 4.1.4c documented as depending on Stories 4.1.2/4.1.3 and feeding 4.1.6 (ID kept to avoid breaking references). |
| CR C14 non-local listener rule | **Fixed** (verified) | Local = plain HTTP, remote = `https:` protocol, never port: dynamic/manual dev ports stay WS-enabled; test on port 62871 (Risk Control, Story 5.4.1, ADR-002). The classification rule stands; its consequence changed 2026-10-26 (owner decision): `remote` is no longer disabled and the server 403 `ws_unary_remote_disabled` no longer exists; `remote` now needs the `unary_over_ws_remote` sub-flag (see "Owner decision reversal"). |
| CR C15 metric 1 narrowing | **Fixed** | `wsQueueMs`/`wsQueuedOver1s` in Story 1.1.2, populated by Task 5.4.1b; criterion 1 uses both. |
| CR C16 `RoutePending` timeout | **Fixed** | `ROUTE_PENDING_TIMEOUT_MS` = 15 000 ms (chosen default, not measured) in `lib/nav/policy.ts` (Story 6.1.3). |
| CR C17 `window_keep_alive` pre-load | **Fixed** | Persisted `ssq:flag:window_keep_alive` with shipped-default fallback; Story 2.1.4 and Risk Control updated; one accepted remount only when server and persisted values differ. |
| CR C18 baselines not re-based | **Fixed** | Story 6.1.2 and Epic 6.1 goal report against the `baseline.json` cold median. |
| CR nitpicks 1, 2, 9, 12, 13 | **Fixed** | UX ACs 29-32 and 21 now have owning tasks (6.1.3d, 4.1.4d, 6.1.2d); S5 annotated resolved; requirements 37-vs-34 explained; ADR-003 "root layout" -> `RootShell`; ADR-002 CORS wording and dropped sentence start. |
| CR nitpicks 3, 4, 5, 6, 7, 8, 10, 11, 14 | **Not addressed (cosmetic)** | Left as written: `<Activity>` vs `inert` wording, "Refreshing" state mapping, under-listed diagram prerequisites, 4.1.6a blocker line, Story 2.1.7 scenario list, `ListFiles`/`ListShells` assertions, "161 of 234" phrasing, the three meanings of "14", and the history rows in this table. None changes a gate or a number. |
| G1 a11y states spec | **Fixed** | Task 6.1.3d (`a11y-responsiveness-states.spec.ts`). |
| G3 shortcutRegistry test path | **Fixed** (verified) | Task 4.1.3d cites `lib/shortcuts/__tests__/shortcutRegistry.test.ts`. |
| G4 Jest has no coverage threshold | **Fixed** (verified) | Task 7.1.2b: path-keyed `coverageThreshold` plus the explicit command. |
| G5, G8 CI seeding, `page.clock` | **Fixed as spikes** | Task 1.1.3e; fallbacks recorded. |
| G9 unowned new test files | **Fixed** | Test-file ownership table (plan) plus Tasks 4.1.3f, 4.1.4d, 6.1.2d, 6.1.3c, 6.1.3d for the five specs that had no task. |
| Estimate | **Re-summed by script** | 39 story rows and 96 task headings (table task counts sum to 96); per-row raw and CU recomputed from class midpoints and output share, all matching: raw 9.075M, CU 13.22M, 19.83M with x1.5 (the size-band and overrun-checkpoint text updated to match). Superseded by the remote-listener patch below (98 headings, 13.59M CU, 20.38M with x1.5). |
| Open after Phase 4 | n/a | `OPEN-DECISION-throttle-gate` (owner). Needs measurement, not planning: server-side vs browser-side stall cause, Chrome WS handshake serialization, 8 s per-call timeout and the provisional keep-alive ceilings, `ROUTE_PENDING_TIMEOUT_MS`. |

### Owner decision reversal: remote listener included (2026-10-26)

This is an **owner decision reversal**, not a review finding. The earlier plan (Review disposition rows "AR minors", "AX2 B2", "CR C14", Story 5.2.1, Story 5.4.1, ADR-002 item 7) refused remote upgrades with 403 `ws_unary_remote_disabled` and disabled `UnaryWsTransport` on `https:` pages. The owner decided the first release must include the remote listener (`--remote-port`, TLS, `:8444`, any remote-access origin). No item below drops or defers unary-over-WebSocket.

Facts re-verified in the repo before encoding (command or Read this session): `server/server.go:1941-1948` (`remoteChain`: stamp -> optional `authMW` -> mux; no HostGuard; a nil `authMW` leaves the chain open, `:1938-1940`); `server/server.go:1972-2010` (`StartRemote`, `ServeTLS`, `ReadTimeout` 15 s, `WriteTimeout` 0); `server/guarded_procedures.go:85-115` (`localChainWith`); `server/services/connectrpc_websocket.go:104-120` (`isAllowedOrigin` accepts any HTTPS and any loopback origin, and any request without Origin); `server/middleware/auth.go:46-58,60-92,153-171` (`AuthRequires`, 401 JSON on `/api/` before the handler, cookie `cs_auth` or `Bearer`, no Origin check, no query-token path); `server/auth/session.go:194-208` (`ValidateAuthSession` deletes an expired token each call); `server/auth/handlers.go:290-291,385-386` (cookie `Secure`, `SameSite=Strict`) and `:406-422` (an existing strict-origin precedent: `https://<primaryDomain>[:<remotePort>]`); `server/services/capture_tap_service.go:50-85` (`RequestRecord{Listener, AuthMode, Host, Origin, Proxied}`, `AuthModeRequired`); `server/middleware/hostguard.go:60-73` (`hostGuardOriginOK`); `main.go:1604-1626` (remote allowed origins), `main.go:1637` (`middleware.Auth(sessions, ...)` passed to `StartRemote`); `server/middleware/logging.go:40` and `gzip.go:106` (`Hijack` pass-through); gorilla/websocket `server.go:265` (clears the HTTP server's deadlines on upgrade, so `ReadTimeout` 15 s does not kill a hijacked socket); `web-app/src/lib/transport/watch-ws-transport.ts:131` (`req.url.replace(/^http/, "ws")`) and `:217-231` (https-only native-streaming gate); `server/server_integration_test.go:66` (ALPN h2 test for `StartRemote`; read, not run); `tests/e2e/helpers/test-server.ts:35,84,158` (`remoteAuth`, `--remote-access --remote-port`, `getRemoteBaseUrl()`).

| Item | Disposition | Where / reason |
|---|---|---|
| Remote 403 stance and `ws_unary_remote_disabled` | **Removed** (owner decision) | Plan Risk Control, Glossary, Stories 5.2.1/5.4.1, Task 5.2.1b, ADR-002 items 4 and 7, ADR-003; validation REQ-3/REQ-12 rows rewritten. |
| Dispatch through the remote chain | **Specified** | Per-listener dispatcher map (Task 5.2.1f) gets a `remote` entry = stamp -> `authMW` -> mux; same `wsUnaryAllowed`, `clientHeaders`, header-provenance rules (ADR-002 item 1). |
| Origin parity (`isAllowedOrigin` any-HTTPS concern, AX2 B2 / `architecture-review-r2.md` "Story 5.2.1 remote listener origin") | **Fixed by a stricter rule than before** | `wsUnaryOriginPolicy` remote rule: Origin required, `https`, in `GetOrigins()` or same-origin with a hostname in `GetHostnames()` (Task 5.2.1g, ADR-002 item 7); `isAllowedOrigin` is not used on the unary route; stream bridges unchanged (follow-up recorded). |
| Auth before dispatch, per-request re-check, expiry | **Specified** | `middleware.Auth` answers 401 before the upgrade; bridge requires `AuthModeRequired`; per-REQUEST ERROR `unauthenticated` (no breaker trip, client parks WS and redirects); no URL tokens, no subprotocol (ADR-002 items 7 and 11). |
| Per-origin enablement and default | **Decided: sub-flag** | `unary_over_ws_remote` (default OFF) plus `originClass` by protocol; a second flag is required because both listeners read one server-wide flag value (Risk Control). Flip gated on remote/h2 arms R1-R5 (Story 5.5.1). |
| Remote/h2 comparison | **Added** | Arms A-R/B-R/C-R and R1-R5 in Story 5.5.1; Task 5.5.1d for the remote instance, session and e2e. |
| Estimate | **Re-summed by script** | 98 task headings, table task counts sum to 98; raw 9.335M, CU 13.59M, 20.38M with x1.5 (Effort estimate). |
| Open after the reversal | n/a | Not verifiable by reading: `GetHostnames()` contents on a remote run, browser h2 negotiation to the production `tlsCfg`, behavior through the owner's real proxy path, and a headless authenticated remote session (Unresolved Questions; owners Task 5.1.1a and 5.5.1d). |

### Triad (inputs: the product, UX and engineering legs of the `pm:triad-review` of this project)

Owner decisions unchanged: unary-over-WebSocket (including the remote listener) is built; throttle gate = Option A; no downscale. Facts verified before encoding (command or Read this session): `feature_flag_service.go:214` is `var knownFeatureFlags = []struct {` (`grep -n`), and every `:207` cite was replaced (7 occurrences in this file; the historical review inputs `adversarial-review-r2.md` are left as written); 40+ other cites spot-checked by opening the line (transport.ts:18/32, policy.ts:5, RootShell.tsx:25, RoutePending.tsx:5, SessionDetailView.tsx:325/465/493/634/665/1213, PaneSplitRenderer.tsx:395, shortcutRegistry.ts:95, session_service_shells.go:62/218, config_surface_proof_test.go:99/207/360, useWatchBacklogItems.ts:128/178/318, backlogItemsSlice.ts:43/122, server.go:1941/1972, guarded_procedures.go:91/103, connectrpc_websocket.go:49/104/1160/2558/3237/3508, watch-ws-transport.ts:131/217, TerminalPool.tsx:83/251/296, hostguard.go:37/60, app/page.tsx:78/107/164/531/626/633, main.go:456/888/1445/1604/1626/1637, test-server.ts:35/84/158, playwright.config.ts:51/59/65/144/189, and others); the only other drift was `design/ux.md` citing `useWindowUrlSync.ts:56-57` for `currentWindowId` (it is `:55`), now fixed; `WindowTabStrip.tsx:251-255` confirmed to be an explicit Enter/Space `onKeyDown` branch on a `<div role="tab">`; `useWatchBacklogItems.ts:318-326` confirmed (`snapshotComplete` is sent only when the snapshot is otherwise empty).

| Finding | Disposition | Where / reason |
|---|---|---|
| Product (a) perceived-speed metric and a terminal echo / stream-latency guard (+10%) | **Fixed; one premise rejected** | Requirements SM6 (INP-style p75 <= 200 ms for the switch and nav click) and SM7 (echo p95 within +10% of baseline, fails when above 1.10x and > 2x MAD); Task 1.1.3b adds the observer and the `terminalEcho` scenario; validation SM6/SM7 rows plus a REQ-10 row. **Rejected as stated: "reuse the echo-marker tap from Task 5.5.1c"**: that tap is app code (`TerminalOutput.tsx`, Task 5.5.1c Files line) landing after Epics 2 and 3, so the `perf-baseline` build (arm A) has no tap and a baseline comparison would be impossible; criterion 5 compares only C against B. Replacement: spec-side Playwright `page.on("websocket")` frame timing (keydown dispatch to first frame carrying the marker), which runs unchanged on the baseline build; the in-page tap stays for B-versus-C. The 200 ms threshold is the web.dev INP "good" bound cited from memory, not re-fetched; owner-revisable. |
| Product (b) comparable SM3/SM4 baselines | **Fixed** | Requirements Success Metrics preamble: the 0.6-1.8 s, 150-650 ms, 2,614 ms, 18-task and 3.4 s figures are directional; the gated baseline is `baseline.json` on the 20-session fixture (Story 1.1.3); validation SM3/SM4/SM5 rows say so. |
| Product (c) SM1 fallback when stalls are server-side | **Fixed** | Requirements SM1 fallback (Story 2.1.8 landed; critical-class zero-blocked and status-0; `ListShells`/`GetVCSStatus` server p95 <= 1 s or a named next suspect); Story 2.1.8 AC; Story 5.1.2 server-side row; validation SM1 fallback row. |
| Product (d) miss-handling rule | **Fixed** | Open Decisions `RULE-target-miss` (a rule, not a question); requirements pointer; Story 7.1.4 reports each miss as the achieved number. |
| Product (e) Epic 5 value risk | **Fixed** | Requirements Feasibility Risks: framed as a measured outcome of the three-arm and remote/h2 comparison; owner decision to build recorded. |
| Product (f) usage context | **Fixed** | Requirements Users / Consumers: single operator, switch frequency and window/session counts unmeasured, fixture assumes 20 sessions across 4 windows. |
| Product (g) file-count bookkeeping out of Baseline | **Fixed** | Requirements Baseline now says "34 call-site edits" and points to Story 2.1.1, where the 37/2/2/33 recount already lives. |
| UX (a) stall text placement vs `aria-busy` | **Fixed** | Story 6.1.3: stall and error text in a sibling element after `<main>`; `ux.md` S5 and criterion 25; validation rows 24-25. |
| UX (b) route-error double announcement | **Fixed, option chosen** | Focus moves to the error heading and `role="alert"` is removed (Story 6.1.3, `ux.md` S5, criterion 24). Rejected variant: keep `role="alert"` with no focus move, which leaves keyboard focus on the nav link with no path to Retry. |
| UX (c) touch: no terminal auto-focus, 44 px targets | **Fixed** | Story 4.1.4 AC, `ux.md` S3 and criterion 33, Task 4.1.4d, validation row 33; reason cites open bug BUG-110. 44 px is the review's figure and exceeds the WCAG 2.2 AA minimum of 24 px (cited from memory). |
| UX (d) Enter/Space on a tab | **Fixed, option chosen; one variant rejected** | Focus stays on the tab (Story 4.1.4, `ux.md` S3, criteria 7-8); justified because the strip already uses automatic activation (`WindowTabStrip.tsx:444`). Rejected: "manual activation per APG" would change arrow-key behavior, which is out of scope; rejected: "move to pane" (two outcomes for one control). AT check added to the manual criterion 32 script (VoiceOver VO-Space; confirms the `detail === 0` assumption, INFERRED until run). |
| UX (e) removed-while-focused Backlog row | **Fixed, one behavior** | Focus moves to the next row (previous when last, list heading when empty) with one "<title> was removed" announcement; no inline "Removed" placeholder (Story 6.1.2, `ux.md` S4, criterion 22). |
| UX (f) "Go to Sessions" at 1 s; 15 s provisional | **Fixed** | Offered from 1 s; `ROUTE_PENDING_TIMEOUT_MS` marked provisional and owner-revisable from Task 6.1.3c timing runs (Story 6.1.3, `ux.md` S5). |
| UX (g) age cue on Out of date | **Fixed** | Badge "Out of date, saved 3 min ago" from `savedAt` / `lastSyncedAt`, `<time>` refreshed every 60 s, never in a live region (Story 6.1.2, `ux.md` S4, criterion 20). |
| UX (h) evicted-window placeholder announcement | **Fixed** | Silent; one "Still connecting ..." per switch only after 1 s (Story 4.1.2, `ux.md` S2, criterion 34). |
| Engineering (a) task-level dependency edge list, scripted, cycle-checked | **Fixed, with caveats** | New "Task-level dependency edge list" section; `implementation/check-task-deps.py`; result: acyclic, 98 task nodes, 430 edges, 39 stories. Caveats: seven stories lacked a `Depends on` line and gained one from the plan's own wave text; the parser first misread "not on the 5.1.2 gate" as an edge (5.2.1 appeared to depend on the 5.1.2 gate, contradicting the plan) and now ends the list at ", not "; within-story order and shared-file edit order are not edges, so the table over-approximates. |
| Engineering (b) stale line cites | **Fixed** | `knownFeatureFlags` is at `feature_flag_service.go:214`; 7 occurrences fixed; one more drift found and fixed in `ux.md` (see the facts paragraph above). |
| Engineering (c) docs/bugs open items | **Fixed** | "Known-issues check" section: no `docs/known-issues/`; `docs/bugs/open/` read; BUG-020, BUG-097, BUG-110/109 incorporated, BUG-079/083 recorded as risks; none blocks. |
| Estimate and counts | **Re-verified by script** | 98 task headings and 39 stories; table task counts sum to 98; 9,335k raw, 13,585k CU, 20,378k with x1.5; unchanged because no task was added. Validation counts re-derived by script: requirement table 145 rows (95 Unit, 49 Integration, 1 Manual), Success Metric table 14 rows, UX table 34 rows. |
