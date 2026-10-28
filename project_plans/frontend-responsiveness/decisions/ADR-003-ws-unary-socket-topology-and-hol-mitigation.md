# ADR-003: Socket topology, bootstrap and head-of-line mitigation for unary-over-WebSocket

**Status**: Proposed (amended in repairs 2026-10-26 pass 1, pass 2, the Phase 4 patch and the remote-listener owner decision; confirm with measurement in Stories 5.1.2, 5.5.1 and 5.5.2)
**Date**: 2026-10-26
**Decider**: tstapler

## Context
A WebSocket is one ordered byte stream. Sharing it with terminal output means a large `ListFiles`/`GetFileContent` response, or a terminal burst, delays everything behind it (`research/pitfalls.md` section 4.1). The HAR also shows ~20 sockets already open alongside the stall, which suggests open WebSockets do not occupy the 6 HTTP slots; whether Chrome counts an **in-progress handshake** against the per-host HTTP/1.1 cap is unverified (`research/architecture.md`; Story 1.1.4 measures it).

Bootstrap hole: the upgrade is an HTTP/1.1 request that may need a free slot, and the `unary_over_ws` flag normally arrives via `GetFeatureFlags` over HTTP, one of the calls that can be stalled.

## Options
A. Reuse terminal sockets for unary calls (rejected: couples keystroke latency to RPC payloads).
B. **One dedicated unary socket per page** (chosen), with a frame size cap and round-robin interleave of response chunks across request ids.
C. A second priority socket for interactive calls (not built by default; scheduled if the Story 5.5.1 head-of-line probe fails, or by the Story 5.1.2 gate if arm B shows `queuedOver1s > 0`).

Bootstrap options: (i) **open at app boot using a persisted last-known flag** (chosen); (ii) server-injected bootstrap flag in HTML or `/api/server-info` (rejected: extra coupling and another HTTP dependency); (iii) wait for the flag over HTTP (rejected: reintroduces the hole).

## Decision
Option B. One shared socket at `/api/ws/unary` (all existing bridges are `/api/...`; the SPA catch-all at `server/server.go:1475` would serve HTML for a mistyped path), opened at app boot from `RootShell` (`app/WsUnaryBoot.tsx`; the shared file both the Next `layout.tsx` and the bun `main.tsx` render) when the persisted last-known `unary_over_ws` is true (else opened lazily after the flag loads ON), shared across all `getConnectTransport()` callers in the tab. Calls made while the socket has been CONNECTING for more than 250 ms use HTTP immediately (decided once per connect attempt, ADR-002 item 4); a socket that opens late is adopted for later calls. Server writes responses as RESPONSE+CHUNK frames of at most 256 KiB, round-robin across active ids, so one large response cannot monopolize the socket. A single writer goroutine per socket serializes writes (gorilla/websocket requirement) fed by a bounded channel. Rollout default is decided by the three-arm comparison (baseline / Epic 2 / Epic 2 + WS) in Story 5.5.1; the channel is built regardless.

### Remote listener (owner decision 2026-10-26: included in the first release)
The topology is the same on the remote origin: one dedicated unary socket per page at `wss://<remote host>:<port>/api/ws/unary` (scheme derived from the page, no query string), served by the remote listener's own dispatcher and origin policy (ADR-002 items 1 and 7). What differs is the problem being solved. The remote listener already negotiates HTTP/2 (an existing test asserts ALPN h2 from a real `StartRemote`, `server/server_integration_test.go:66`, read not run; browser-side negotiation is INFERRED until plan Task 5.5.1d asserts `nextHopProtocol`), so unary calls are already multiplexed and the 6-connection cap that motivates local WS does not apply. Over h2 the unary socket adds its own head-of-line behavior (one ordered byte stream, versus h2's independent streams), so the socket is built on remote but **enabled** there only through the `unary_over_ws_remote` sub-flag (default OFF), flipped by the remote/h2 arms of plan Story 5.5.1 (R1-R5). Bootstrap on remote has no HTTP/1.1 slot problem for the upgrade (h2 connection reuse does not apply to the WS handshake, which is an HTTP/1.1 upgrade on its own connection; INFERRED), but the persisted-flag early-open still requires both persisted flags. Reverse proxies that do not forward `Upgrade` or that impose an idle timeout shorter than the socket's keepalive turn the handshake into a breaker-counted failure and the page degrades to HTTP (plan Risk Control, remote-listener risks).

## Consequences
- Terminal streams are unaffected by construction.
- If the browser counts the unary handshake against the HTTP cap, it is a single connection opened once at boot, before the first RPC burst, not per call.
- First-ever load after enabling the flag uses HTTP only (no persisted value yet); that is accepted.
- If Chrome serializes WebSocket handshakes per host (one connection CONNECTING per endpoint; INFERRED, not tested; Story 1.1.4 probes it with 14 stream sockets CONNECTING), the boot-time unary socket would queue behind the ~20 stream sockets and silently give no benefit while the flag reads ON. `WsUnaryBoot` therefore renders ahead of every stream subscriber so the unary socket is constructed first; the probe result is recorded here at Story 5.5.2.
- The bridge's in-flight cap starts at 6 (ADR-002 item 8) until Story 5.5.1 shows server latency does not grow with it.
- Reopen this ADR if Story 5.1.2 or 5.5.1 shows head-of-line or bootstrap effects above budget.
