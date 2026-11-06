# Build vs Buy: frontend-responsiveness

Date: 2026-10-25. Labels: VERIFIED = file read / command run in this worktree or a fetched URL; INFERRED = reasoned, not proven.

## What the repo already has (VERIFIED unless noted)

- Installed: React 19.1.1 (`web-app/node_modules/react/package.json`; `package.json` range `^19.0.0`), Next 15.3.2, `@connectrpc/connect(-web)` ^2.1.1, `@tanstack/react-virtual` ^3.13.25, `react-virtuoso` ^4.18.7. No `@tanstack/react-query`, `swr`, `connect-query`, or `react-activation` (grep of `web-app/src` and `package.json`).
- A shared unary transport singleton already exists: `getConnectTransport()` in `web-app/src/lib/api/transport.ts:32` (auth interceptor, 30 s default timeout). Yet 34 non-test files outside `lib/api/transport.ts` still call `createConnectTransport(` directly (`grep -rln` list: settings/*, backlog/*, unfinished/*, sessions/*, `app/backlog/page.tsx`, ...). 37 total including `transport.ts` and `watch-ws-transport.ts`. So "one shared transport" is a migration, not new design.
- Flags: `FeatureFlagsContext.tsx:100-125` does a single `client.getFeatureFlags` in a provider `useEffect` with a seq/minAcceptable guard. The 14 simultaneous `GetFeatureFlags` calls therefore come from other callers (`lib/hooks/useFeatureFlagValue.ts`, `lib/features/features/flags.ts` also reference it) -- which hook issues them is INFERRED, needs a trace of call sites. `useAvailablePrograms.ts` is the `ListProgramsConfig` caller and `useBrowserLogStream.ts` the `LogClientEvents` caller (grep hits).
- Streaming over WS: `web-app/src/lib/transport/watch-ws-transport.ts` (`createWatchTransport`) passes `unary` straight to `createConnectTransport` and implements only `stream` over WebSocket via `runStreamingCall`; server side is `server/services/ws_stream_bridge.go` (`StreamingWSBridge`, one request envelope in, one envelope per event out, end-stream envelope last). There is also `websocket-transport.ts` (bidirectional, used for terminal streams).
- HTTP/2: the only server TLS listener is the remote-access one (`server/server.go:1625` `ListenAndServeTLS`, :8444). The code comment at `watch-ws-transport.ts:213-220` states no shipping browser supports cleartext HTTP/2, so :8543 is HTTP/1.1 only.
- Virtualization already used in: `SessionList.tsx`, `BoardColumn.tsx`, `TrayList.tsx`, `VirtualLogList.tsx`, `VirtualHistoryList.tsx`, `SearchResults.tsx`, `insights/SessionsTable.tsx`, `useLogViewer.ts`.

## 1. Request dedupe / cache / batching

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| TanStack Query v5 (5.104.5, published 2026-10-19 per npm registry) | In-flight dedupe by key, staleTime, `gcTime`, background refetch, devtools; widely battle-tested; handles `useFeatureFlags`, programs config trivially | New dependency + provider; 34+ call sites to migrate hook-by-hook; does not fix the connection limit or batch `LogClientEvents` (a write/fire-and-forget); two data layers during migration (Watch* streams stay bespoke) | Recommended for read RPCs (flags, programs config, lists) |
| `@connectrpc/connect-query` 2.3.1 (2026-08-09) | Generates `useQuery` per RPC with proto-typed keys on top of TanStack Query; peer deps `react ^18\|\|^19`, `@connectrpc/connect ^2.0.1`, `@bufbuild/protobuf 2.x`, `@tanstack/react-query >=5.62.0` (npm registry) -- compatible with this repo's versions | Requires TanStack Query anyway; opinionated hook API means rewriting each call site rather than wrapping | Viable (thin layer, adopt only if hook-per-RPC style is wanted; plain TanStack Query + `createClient` is less coupling) |
| SWR 2.5.1 (2026-08-12) | Smaller; dedupe interval | Weaker mutation/invalidations and devtools; would be a second paradigm if TanStack Virtual's maintainers' ecosystem is chosen elsewhere; no Connect integration | Not recommended (no advantage over TanStack Query here) |
| In-repo Connect interceptor + the existing shared transport | No dependency; interceptor can coalesce identical unary requests (key = method + serialized message) in ~60 lines, apply `Connect-Timeout`/priority, and batch `LogClientEvents` in a client-side queue; fixes the 14x `GetFeatureFlags` without touching call sites once all 34 files use `getConnectTransport()` | Only dedupes concurrent calls (no staleTime/cache, no invalidation); must handle abort-signal sharing (one caller aborting must not cancel others); bespoke code to maintain | Recommended as the first layer (cheap, call-site-agnostic), combined with TanStack Query for cached reads |

Note: dedupe/batching reduces concurrent slot use but does not by itself remove HTTP/1.1's 6-connection head-of-line limit (INFERRED from requirements Evidence 1; the cause of the 30 s blocks is not proven by a HAR).

## 2. Unary over WebSocket

- Existing OSS Connect-over-WS transports: INFERRED none maintained as of this research. Connect's official transports are HTTP only (`connect-web`, `connect-node`); WS is not in connect-es. I did not find or verify a maintained library; treat "buy" as unavailable. Name the gap: no search was done beyond npm registry lookups for connect-query/react-query/swr/react-activation.
- Extend in-repo: client side is small -- replace `unary: httpTransport.unary.bind(...)` in `createWatchTransport` with a `runUnaryCall` over a single multiplexed socket, one WS per call is not an option (the HAR already shows 20 open sockets), so it must multiplex over one socket. Server side is the larger part: `StreamingWSBridge` expects one request/one WS per stream; unary needs request IDs and multiplexing on one socket, HostGuard/auth parity, cancel semantics, backpressure, reconnect with in-flight replay (requirements Rabbit Holes).
- Pros: removes the 6-slot cap entirely on localhost, matches the existing stream bridge's design. Cons: bespoke protocol (not Connect wire-compatible), debugging loses DevTools network view for unary calls, every server interceptor/middleware must be re-applied, large blast radius (all 34+ call sites go through it).
- Verdict: Viable but Not recommended as the first move; revisit only if post-dedupe HAR still shows slot saturation. If done, extend `watch-ws-transport.ts` + `ws_stream_bridge.go` rather than adopting an unproven third-party lib.

## 3. HTTP/2 on localhost

- Browsers only do h2 over TLS; no cleartext h2c (repo's own comment, `watch-ws-transport.ts:213`; consistent with INFERRED general browser behavior, not re-verified against a URL here). Go `net/http` negotiates h2 automatically on `ListenAndServeTLS` -- the :8444 listener already does this.
- mkcert (local CA + trust-store install) would give a trusted `https://localhost:8543`. Pros: ~100 concurrent streams per connection, removes the limit with zero protocol change, native Connect streaming works (the existing `NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING` flag already supports https). Cons: per-machine cert/CA setup, expiry, breaks the "go build and run" simplicity and `STAPLER_SQUAD` e2e/ manual instances (each port needs a cert SAN), `http://localhost:8543` bookmarks, launchd/systemd service must find certs; self-signed alternatives prompt warnings.
- Verdict: Viable as an opt-in mode (remote-access listener is the proof it works); Not recommended as the default localhost path because it adds setup cost for every machine.

## 4. Keep-alive / offscreen windows

- React `<Activity>` shipped stable in React 19.2 (https://react.dev/blog/2025/10/01/react-19-2 -- fetched, page describes `<Activity>` as an alternative to conditional rendering). Installed is 19.1.1 and `node_modules/react/cjs/react.production.js` contains zero `Activity` matches (VERIFIED), so it needs a bump to >= 19.2 (npm latest is 19.3.1). Whether Next 15.3.2's app router honors a user-upgraded `react` vs its vendored React is INFERRED-unknown; verify in a spike. Behavior (from the docs, partially read): hidden Activity trees keep state, unmount effects/cleanups, and deprioritize renders -- this is the right semantics for pausing hidden terminals' rAF work and key handlers (effects clean up), but xterm DOM/WebGL context is retained.
- `react-activation` 0.13.4 (last published 2025-05-21 per npm registry; ~5 months stale) -- portal-based KeepAlive; Not recommended (unmaintained relative to the built-in, extra DOM juggling).
- Custom `display:none`: cheapest, works now, but effects keep running (rAF, resize observers, window keydown handlers) -- the exact cost the trace shows -- and hidden xterm measures 0x0 (requirements Rabbit Holes). Needs a bespoke `isVisible` plumbed to every effect.
- Verdict: `<Activity>` Recommended (after React bump + spike, bounded to N recent windows, behind the existing feature flag); custom `display:none` Viable only as fallback; `react-activation` Not recommended.

## 5. Virtualization

- Already adopted widely (list above); both TanStack Virtual and Virtuoso are installed. VERIFIED that `SessionList` and `BoardColumn` (backlog columns) virtualize. So the ~3,300 elements are INFERRED to come mostly from non-list surfaces (terminal DOM, panes, tray/chrome, un-virtualized backlog bodies) -- needs a DOM-count-by-component measurement before spending effort here.
- Choosing a library: stay on TanStack Virtual / Virtuoso already present; do not add react-window (third overlapping library).
- Verdict: reuse existing = Recommended; add react-window = Not recommended; new virtualization work = defer until DOM attribution shows a non-virtualized list.

## 6. LLM-generated bespoke vs battle-tested library, per area

| Area | Lean | Why |
|---|---|---|
| Read caching/dedupe | Library (TanStack Query) | Cache invalidation, abort sharing, and stale races are exactly where generated code is subtly wrong (note the hand-rolled seq/minAcceptable guard in `FeatureFlagsContext.tsx:95-125` already compensating) |
| In-flight coalescing at transport | Bespoke, small | No library fits Connect interceptors; ~60 lines, unit-testable |
| Log event batching | Bespoke | Domain-specific queue + `keepalive` flush on page hide (transport already has `getKeepaliveConnectTransport`) |
| Unary-over-WS | Bespoke only if needed | No maintained OSS found; protocol design needs human review |
| HTTP/2 | Neither (config) | Go stdlib + mkcert |
| Keep-alive | Library primitive (`<Activity>`) | First-party React; bespoke visibility plumbing only for xterm/WebGL |
| Virtualization | Library (existing) | Do not hand-roll |

## Recommended combined approach

1. Migrate the 34 stray `createConnectTransport(` call sites to `getConnectTransport()` and add one interceptor there: in-flight request coalescing (flags, programs config, VCS status) and a client-side `LogClientEvents` batcher flushed via the keepalive transport. Cheapest, call-site-agnostic; target metrics: <=1 `GetFeatureFlags`, deduped programs config and log events.
2. Add TanStack Query (plain, optionally connect-query later) for cached reads (`GetFeatureFlags`, `ListProgramsConfig`, shells/VCS) with `staleTime`; keep Watch* streams on the existing hooks.
3. Keep-alive windows via `<Activity>` (React >= 19.2, spike on Next 15.3.2 first) for a bounded set of recent windows, flag-gated, with hidden-surface rAF/resize/key handlers stopped by effect cleanup.
4. Re-capture HAR with DevTools closed. Only if slots still saturate: opt-in mkcert HTTPS (h2) before building unary-over-WS; unary-over-WS stays last resort.
5. Virtualization: no new work until a per-component DOM count shows a non-virtualized list; reuse TanStack Virtual / Virtuoso.

## Gaps

- Cause of the 30 s status-0 blocks is unproven (requirements Evidence 1).
- Next 15.3.2 + React 19.2 `<Activity>` compatibility not tested.
- OSS Connect-over-WebSocket search was limited to npm registry names I tried; absence is not verified.
