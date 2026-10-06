# Stack Research: terminal-resync-reliability

Agent 1 (Stack), Phase 2. All findings below are grounded in this repo's actual code
(paths/line numbers cited) plus current upstream docs where noted VERIFIED via WebSearch.
No new runtime dependency is required for any of the five in-scope fixes — every mechanism
below is either already vendored or available in the Go/TS standard library.

## 1. Wire protocol today

- Transport: hand-rolled ConnectRPC-over-WebSocket, not a native Connect streaming transport.
  `server/services/connectrpc_websocket.go:382` upgrades the HTTP connection with
  `gorilla/websocket` (`go.mod:21`, `github.com/gorilla/websocket v1.5.3`) via a package-level
  `wsUpgrader` (`connectrpc_websocket.go:70`), then frames protobuf `TerminalData` messages
  (`proto/session/v1/events.proto`) manually over that raw socket. This is NOT a
  `connectrpc.com/connect` bidi-stream handler in the usual sense — Connect's own streaming
  compression/codec negotiation therefore doesn't apply; any compression has to be added at
  this hand-rolled framing layer.
- `connectrpc.com/connect v1.19.0` (go.mod:6) is otherwise used for the non-terminal RPCs
  (session CRUD, backlog, etc.) via `server/services/`. VERIFIED current upstream is a 1.x
  release from 2026-05 with no breaking changes planned in the 1.x series — no version bump
  needed; the exec-gate/resync work doesn't touch this package.
- Frontend: `@connectrpc/connect` / `@connectrpc/connect-web` `^2.1.1` and `@bufbuild/protobuf`
  `^2.11.0` (`web-app/package.json:41,57-58`) generate the TS message classes consumed by
  `useTerminalStream.ts` / `useVisibilityResync.ts`. These are current majors; no bump needed.

## 2. `CurrentPaneRequest`/correlation ID: protobuf design

Current message (`proto/session/v1/events.proto:195-211`):

```proto
message CurrentPaneRequest {
  int32 lines = 1;
  bool include_escapes = 2;
  optional int32 target_cols = 3;
  optional int32 target_rows = 4;
  reserved 5; reserved "streaming_mode";
}
```

There is no ID field anywhere in this message or in `CurrentPaneResponse`
(`events.proto:216+`), which is the root cause of
`notifyResyncOutputReceived()` treating any `output`-type message as completion
(`useVisibilityResync.ts` doc comment, "No-correlation-ID heuristic").

**Recommended addition** — a single new field on both request and response, next field
number available on each (do not reuse `reserved 5`):

```proto
message CurrentPaneRequest {
  int32 lines = 1;
  bool include_escapes = 2;
  optional int32 target_cols = 3;
  optional int32 target_rows = 4;
  reserved 5; reserved "streaming_mode";
  // Correlation ID for this resync request, echoed verbatim in the matching
  // CurrentPaneResponse. Client-generated (monotonic counter or short random
  // string is sufficient — uniqueness only needs to hold within one
  // connection's lifetime). Empty string is valid for legacy/uncorrelated
  // callers during rollout (see feature-flag section).
  string resync_id = 6;
}

message CurrentPaneResponse {
  // ...existing fields...
  string resync_id = N; // echoes the request's resync_id, empty if none was sent
}
```

- Use `string`, not `int64`/`uint64`: a per-tab monotonic counter is simplest client-side
  (`useRef` counter) and a string avoids any signed/unsigned wire-format bikeshedding for a
  value nobody arithmetics on. `crypto.randomUUID()` is also fine and already available in all
  browsers this app targets (no new dependency).
- Keep it `optional`-free (plain `string`, default `""`) rather than `optional string` —
  protobuf3 presence tracking isn't needed here since `""` unambiguously means "no
  correlation," which is exactly the legacy-caller fallback behavior wanted during flag
  rollout.
- This is the "batching wedge": once `resync_id` exists, a follow-on `BatchedCurrentPaneRequest`
  (see §4) can reuse the same field name/semantics per sub-request, keeping the correlation
  contract identical whether a resync travels solo or batched.

## 3. WebSocket-level compression vs. application-level payload compression

**Finding: WebSocket compression is not currently enabled anywhere in this path.**
`wsUpgrader` at `connectrpc_websocket.go:70` constructs a `websocket.Upgrader{}` with no
`EnableCompression` field set, so it defaults to `false` — no permessage-deflate negotiation
happens today for the terminal socket. The only compression in the codebase is the unrelated
HTTP-response `middleware.Compress` (gzip) in `server/server.go:979,1162`, which wraps plain
HTTP handlers, not the upgraded WebSocket connection.

Two independent levers, and they solve different halves of Success Metric #4 (batching *and*
compression):

- **`gorilla/websocket`'s `EnableCompression: true`** (permessage-deflate, RFC 7692) —
  zero new dependency, just a one-line `Upgrader`/`Dialer` config change on both ends. VERIFIED
  via WebSearch (gorilla/websocket docs, Centrifugo engineering blog 2024-08-19): gorilla only
  implements the "no context takeover" variant (compression context reset per message, since a
  per-connection `flate.Writer`/`flate.Reader` pair for context takeover is memory-expensive at
  scale), and enabling it is a real CPU/memory-for-bandwidth trade — Centrifugo's own
  production experience found it "much slower" under high message-rate/high-concurrency
  workloads unless paired with a `sync.Pool` for the flate reader/writer. **Recommendation for
  this project**: enable it selectively only on resync-response frames (gorilla supports
  per-message `SetCompressionLevel`/`EnableWriteCompression` at the connection level, so the
  same connection can compress the occasional large resync payload while leaving
  high-frequency small keystroke-echo frames uncompressed) rather than globally — matches the
  "resync path only, not general streaming" scoping in Out of Scope. Needs a before/after
  byte-count benchmark (Observability Requirements) before flipping the flag broadly, given
  gorilla's own documented CPU cost.
- **Application-level: batching N `CurrentPaneRequest`s into one message** (see §4) reduces
  *round trips and protocol overhead* (repeated framing, TLS/WS overhead per message,
  exec-gate acquisitions), independent of whether bytes-on-the-wire are also deflate-compressed.
  This is the primary lever for the "N simultaneous requests -> 1" success metric; permessage-
  deflate is a secondary, orthogonal lever for the "fewer bytes" success metric. They compose:
  ship batching first (larger, more certain win, no CPU trade-off), evaluate permessage-deflate
  as a follow-on once batching's own byte-count metrics show whether it's still worth it (a
  batched payload with repeated tmux-capture escape sequences across panes may compress
  unusually well versus N independent captures — worth measuring, not assuming).

## 4. Batching design: `BatchedCurrentPaneRequest`

Given the correlation-ID field from §2, the smallest protobuf change that satisfies "coalescing
multiple `CurrentPaneRequest`s into one message" (Scope item 5, Rabbit Holes) is a wrapper
message carrying a repeated field, added as a new `oneof` case alongside the existing
`current_pane_request` in the outer envelope (`events.proto:75`):

```proto
message BatchedCurrentPaneRequest {
  repeated CurrentPaneRequest requests = 1; // each with its own tmux_session_name + resync_id
}
message BatchedCurrentPaneResponse {
  repeated CurrentPaneResponse responses = 1; // order-independent; matched by resync_id
}
```

This requires `CurrentPaneRequest` to carry a target-pane identifier (check whether one
already exists on the outer envelope vs. per-pane — outer `TerminalData` envelope is
per-connection/per-session today per `events.proto:75`, so a batch spanning multiple terminals
*within one session* is natural; batching *across sessions* would need a session ID added
per-request, which is a bigger change and not required by the requirements — multiple mounted
terminals belonging to the same session, e.g. split-pane shells, is the actual burst pattern
described in the Problem Statement). Scope batching to same-session, same-connection panes
first; cross-session/cross-connection batching is out of the Large appetite's critical path per
the Rabbit Holes warning against scope creep.

## 5. Go concurrency primitives for staggering/rate-limiting server-side

No new dependency needed — both already-vendored packages cover this:

- **`golang.org/x/time/rate`** (go.mod:46, `golang.org/x/time v0.15.0`) — not currently imported
  by `session/tmux/exec_gate.go`, which instead hand-rolls a flock-based slot semaphore
  (`session/tmux/exec_gate.go:1-80`, `acquireSlot`/`gateDir`, retry backoff constants
  `execGateAcquireBackoffStart`/`Max`). `rate.Limiter` is the standard idiom for "spread N
  requests over time" (token bucket) and is a good fit for a **new, separate resync-specific
  fast lane** (Rabbit Holes: "a new, separate gate key/fast lane" rather than raising the
  shared `"default"` slot count) — e.g. `rate.NewLimiter(rate.Every(50*time.Millisecond), 1)`
  per session to stagger a same-session multi-pane batch's sub-captures without touching the
  existing flock-based cross-process gate at all.
- **`golang.org/x/sync`** (go.mod:193, `v0.21.0`) already vendored — `errgroup` for running a
  batch's per-pane captures concurrently with bounded parallelism
  (`errgroup.Group.SetLimit(n)`), and `singleflight` if multiple near-simultaneous
  `CurrentPaneRequest`s for the *same* pane should collapse into one tmux `capture-pane` call
  (relevant if staggering client-side isn't perfect and two requests for the same pane land
  close together server-side).
- The existing `session/tmux/exec_gate.go` flock-slot pattern (`acquireSlot`, cross-process via
  `github.com/gofrs/flock`, already a dependency) is the right model to *extend* (new gate key)
  rather than replace — it already handles the cross-process case (main daemon + `--mcp`
  processes) that a pure in-process `rate.Limiter` would miss, per the doc comment at
  `exec_gate.go:29-38` ("across this and every other process touching the same tmux server").
  A resync fast lane should likely be a second flock directory keyed off e.g.
  `serverSocket + ":resync"` with its own `SlotsOrDefault`-style config, reusing
  `acquireSlot`/`runGated` rather than duplicating the locking logic.

## 6. React/TypeScript patterns for client-side staggering

This repo already has the primitives needed; no new npm dependency required:

- **`useDebouncedCallback`** (`web-app/src/lib/hooks/useDebounce.ts:26-53`) is already used by
  `useVisibilityResync.ts` (`RESYNC_DEBOUNCE_MS = 300`) to coalesce rapid
  `visibilitychange`/`focus` events into one resync trigger *per terminal instance*. It does
  **not** solve the cross-terminal fan-out problem described in the Problem Statement (N
  mounted terminals each independently listening to the same document-level event) — that's a
  fan-out/coordination problem across component instances, not a per-instance debounce problem.
- **Fix for Scope item 1 (visibility scoping)**: `TerminalOutput.tsx:535` already computes
  `foreground: isVisible` and passes it into `useTerminalFlowControl` — but per the
  architectural review cited in Requirements, `isVisible`
  (`TerminalOutput.tsx:68,91,1016-1022`) is wired into `useTerminalFlowControl`, not into
  `useVisibilityResync`'s trigger path (`useVisibilityResync.ts` takes no `isVisible`/
  `foreground` param at all in its `UseVisibilityResyncParams`, confirmed by the param list at
  lines 12-26). The direct fix is threading the same `isVisible` prop already available in
  `TerminalOutput.tsx` into `useVisibilityResync`'s `handleVisibilityOrFocusResyncInner` guard
  (alongside the existing `document.visibilityState !== 'visible'` check) — no new pattern or
  library needed, just closing a prop-plumbing gap that already exists for the sibling hook.
- **Fix for Scope item 4 (stagger bursts across multiple *simultaneously* visible terminals,
  e.g. split view)**: since each `TerminalOutput`/`useVisibilityResync` instance is independent
  (no shared parent coordinator today), staggering requires either (a) lifting a shared
  coordinator into `SessionDetailView.tsx` (which already owns the mount/keep-alive list at
  lines 741-868) that hands out a small deterministic delay per visible terminal index — e.g.
  `index * 50ms` via `setTimeout`, no library needed — or (b) a lightweight shared module-level
  queue (a plain array + `setTimeout` drain loop, not a new dependency) that every
  `useVisibilityResync` instance enqueues into instead of firing immediately. Given
  `SessionDetailView.tsx` already threads `isVisible` per terminal (§ above), (a) is the lower-
  risk option: it reuses existing prop plumbing and keeps `useVisibilityResync` itself simple,
  versus introducing new cross-instance shared state that has to interact correctly with
  React's StrictMode double-invoke and the existing per-instance refs
  (`pendingResyncCompletionRef`, `bannerShownRef`) that the file's own comments describe as
  already fragile ("never share a ref between timers" guardrail visible in the file's
  comments).
- No case for `lodash.debounce`/`use-debounce` (npm) or `requestIdleCallback`-based scheduling
  — the existing hand-rolled `useDebouncedCallback` plus a simple index-based `setTimeout`
  stagger covers every pattern in Scope items 1 and 4 without adding a frontend dependency.

## Summary of dependency changes

| Layer | New dependency needed? | Notes |
|---|---|---|
| Correlation ID (protobuf) | No | Add `string resync_id` field to existing messages |
| Batching (protobuf) | No | New message type in existing `.proto`, regenerate via `make proto-gen` |
| WS compression | No | `gorilla/websocket` (already vendored) `EnableCompression`/`SetCompressionLevel` |
| Server staggering/rate-limit | No | `golang.org/x/time/rate` and `golang.org/x/sync` (`errgroup`, `singleflight`) already in go.mod; extend existing `session/tmux/exec_gate.go` flock pattern for a resync fast lane |
| Client staggering | No | Existing `useDebouncedCallback` (`web-app/src/lib/hooks/useDebounce.ts`) + plain `setTimeout` index-based stagger in `SessionDetailView.tsx`/`useVisibilityResync.ts` |
| Feature flag | No | Follow existing `TmuxExecGateConfig` pattern in `config/types.go:84-105` |
