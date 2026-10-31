# ADR-035: Unary ConnectRPC over WebSocket for writes and telemetry, by delivery class

Status: Proposed — 2026-10-30. Extends ADR-001/002 (unary mux, reads only). Composes with ADR-033 (terminal pool).

## Context

Owner decision: "a generic websocket transport for connect rpc and then route all of our writes like analytics etc over it."

Today `/api/ws/unary` (`server/services/ws_unary_bridge.go`) carries only an allow-listed set of idempotent reads
(`wsUnaryAllowed`, mirrored by `wsEligible` in `web-app/src/lib/api/idempotentMethods.ts`). Everything else is HTTP.
Owner HAR (`stapler-squad-backend-har-10-30.har`, 468 requests; counts below are the task brief's, not re-derived here): 442 are
HTTP/1.1 (6-connection cap per origin), and the write/beacon traffic is all plain HTTP: `/api/analytics` POST 54 (p50 0.7 s,
p95 6.8 s wait), `LogClientEvents` 39, `/api/client-logs` 15. Those compete for the same 6 connections as the interactive reads.

What the code does today (read at `origin/main` f305e57ff):

| Fact | Source |
|---|---|
| The bridge synthesizes `POST /api<method>` with the upgrade request's Host, RemoteAddr, TLS, Origin, Cookie and proxy headers, then calls the **listener's full dispatcher** `http.Handler`. HostGuard, auth, `LocalWriteGuard` (`guardedProcedures`), body limits and Connect interceptors therefore run unchanged per call. | `ws_unary_bridge.go` `synthesizeUnaryRequest`, `handleRequest` |
| Deny-by-default server table plus REQUEST `meta.headers` allow-list (`Connect-Protocol-Version` only). | `ws_unary_allowlist.go` |
| Client replays a read over HTTP after `WS_CALL_TIMEOUT_MS` = 8 s, on `resource_exhausted`/`unavailable`, and on socket loss; only for names in `replayable`. A non-replayable call that loses its socket rejects. | `wsUnaryPolicy.ts` `viaWs`, `onWsError` |
| One in-flight cap per socket (64), one writer with whole-frame priority over CHUNKs, one client send queue (64 frames / 1 MiB) shared by every call. | `ws_unary_bridge.go`, `ws_unary_writer.go`, `unary-ws-transport.ts` |
| `POST /api/analytics` and `POST /api/client-logs` are **not Connect RPCs** (plain mux handlers, JSON). `LogClientEvents` is. | `server/server.go`, `client_log_handler.go`, `session.proto` |
| #1307 (open) makes the analytics batch insert `OnConflictColumns(id).DoNothing()` with a server-assigned id when the client sends none. The model for a naturally idempotent write. | PR 1307 diff |
| Unload today: analytics drains by `sendBeacon` within a 60 KB aggregate budget then keepalive fetch; `logBuffer` flushes `LogClientEvents` through a keepalive transport; `clientLogShipper` uses keepalive fetch. | `HttpAnalyticsProvider.ts`, `logBuffer.ts`, `clientLogShipper.ts` |

The bridge's re-entry design already gives interceptor parity for any method it dispatches. What it lacks for writes is
(1) a statement of what a write may do after an ambiguous failure, (2) a way to dedupe a re-sent write, (3) isolation of
slow or bulky telemetry from interactive calls, and (4) an unload story.

## Decision

Keep one wire protocol (the ADR-001 unary mux frames; additive fields only) and one dispatcher re-entry. Add **delivery classes**
that decide, per procedure, which transport rules apply. The class table lives on the server (authoritative) and is mirrored
in the client fixture `ws-unary-allowlist.json`; a parity test fails when they differ (same pattern as #1300). A client can
never upgrade its own class: the server ignores any client-claimed class.

### Delivery classes

| Class | Meaning | On WS timeout / socket drop after the frame was written | Initial members |
|---|---|---|---|
| **R** idempotent read | Today's behaviour, unchanged. | Replay over HTTP (unchanged). | Current allow-list (#1300). |
| **W** idempotent write with key | Handler is naturally idempotent for a repeated request; server also dedupes by `Idempotency-Key`. | Replay over HTTP **with the same key**. The server store makes the replay join or return the original execution. | Analytics ingest, `LogClientEvents`, `PinSession`/`UnpinSession` (proto comments say "Idempotent"). |
| **M** at-most-once mutation | Not safe to run twice. | **Never replayed automatically.** The call rejects with `unavailable` plus marker `outcome_unknown`. See "Class M". | Empty at launch; per-method opt-in only. |
| **T** fire-and-forget telemetry | Loss of some events is acceptable; duplicates are not. | Not replayed per call; the lane's own retry-with-same-batch-id applies, then drop. | Analytics, `LogClientEvents`, client-logs. |

A procedure that is both W and T (analytics) is class T on the client lane and W on the server (dedupe is what makes the
lane's retries safe). The server table therefore stores `{class, requiresKey, maxBody, perClassInflightCap}`.

Admission rule for W: a method may be listed only if a repeat of the same request body is harmless **without** the key store,
because the store is in memory and lost on restart. This is a review checklist item and a code comment on the table, not
something a test can prove. Methods whose "idempotent" comment is only about a missing-row no-op get an explicit handler test
that calls twice.

### Idempotency key and store

* Key travels as an HTTP header `Idempotency-Key` (1-64 chars `[A-Za-z0-9_-]`), on **both** transports. Over WS it is carried
  in `meta.headers`; the allow-list grows from `Connect-Protocol-Version` to that plus `Idempotency-Key`; `synthesizeUnaryRequest`
  already copies allow-listed meta headers onto the request. Because the key rides in a header, the HTTP replay after a WS
  failure uses the identical mechanism and the identical store.
* The store is **middleware inside the dispatcher chain**, after auth and `LocalWriteGuard`, so it sees a transport-independent
  `*http.Request` and cannot be bypassed by choosing HTTP. Only procedures with `requiresKey` consult it; a missing or malformed
  key on such a procedure is `invalid_argument` (never silently unkeyed).
* Entry: `(listener, principal-or-"anon", procedure, key) -> {bodyHash, state in_flight|done, status, response bytes}`.
  Same key and same body hash while `in_flight` joins the original (singleflight, waits on the joiner's own deadline); while
  `done` returns the stored response byte for byte. Same key with a different body hash is `failed_precondition` marker
  `idempotency_key_reuse`. Principal comes from the auth layer. On the unauthenticated local listener every local caller is `anon`, so the key must carry a client-generated per-window random prefix (`<windowId>.<uuid>`, window id generated once per page life) to avoid cross-tab collisions; the bridge validates the key pattern at the REQUEST, not only the middleware.
* Bounds: TTL 10 min, LRU, total retained bytes capped at 8 MiB, one stored response at most 64 KiB (a larger response is
  stored as `done` with no body; a replay then gets `unavailable` marker `result_not_retained` and the caller must reconcile).
  Only a terminal 2xx or a deterministic 4xx (`invalid_argument`, `failed_precondition`, `not_found`) becomes `done`; a 5xx, `unavailable`, `deadline_exceeded`, `canceled` or a handler panic deletes the entry so a retry re-executes. A keyed handler runs on a context detached from the first caller's CANCEL (it keeps the 5 min clamp and server shutdown), so one caller aborting does not fail joiners; if every waiter has gone, the handler is cancelled. `in_flight` entries are never evicted; when the byte cap is full of them, a new keyed call is refused `resource_exhausted` with `not_executed` (checked before dispatch). Counters: hits, joins, conflicts, evictions, bytes.
* Not durable. A restart forgets keys. This is acceptable for W (handler is idempotent anyway) and is stated as a limit for M.

### Class M (at-most-once)

Ambiguity is defined by the client socket state machine, not by timing:

| Situation | Client action |
|---|---|
| Frame never handed to `WebSocket.send` (queued while connecting, queue full, socket unusable) | Safe to send over HTTP once; the frame never left. |
| Server answers with ERROR carrying `not_executed` | Safe to send over HTTP once. The bridge stamps `not_executed` **only** at its own pre-dispatch `replyError` sites in `handleRequest` (method not allowed, disallowed header, bad timeout, dispatcher not ready, duplicate id, in-flight/class cap) and strips any `marker`/`not_executed` key from handler or dispatcher `details` before forwarding, so a handler cannot forge it. Post-dispatch errors never carry it: recorder overflow (`resource_exhausted` at `ws_unary_bridge.go:774`), a 429/502/503 mapped to `unavailable` by `connectCodeForStatus`, and rate-budget refusals from inside the dispatcher are all ambiguous-or-executed. A test asserts a handler-returned `resource_exhausted` or marker never counts as `not_executed`. |
| Frame sent, then socket close, ping timeout, call deadline, or client abort, with no RESPONSE/ERROR | Ambiguous. Reject with `unavailable` and marker `outcome_unknown`; `outcome_unknown` is synthesized by the client, never sent by the server, and also replaces a server `deadline_exceeded` for M (the server keeps running an abandoned handler, `abandon()`). No auto-replay. |

The caller (UI) decides: it may reconcile by reading state, or the user retries. A retry **reuses the call's idempotency key**
(the client generates one per user intent and keeps it on the pending-action object, the pattern `ReplyToPendingQuestion.reply_id` already uses, `session.proto:1958`), so within the store's TTL and process life the server returns the original result instead of
running twice. That is the "query by key": retry-with-same-key, no new status RPC. Outside that window (restart, TTL) a retry
can run twice; M methods therefore stay on HTTP until their UI path reconciles, and the table starts empty. M adds no new
failure mode on HTTP: the same rules apply to an HTTP network error, and the key store covers both.
There is no 8 s timeout race for M or W: they use the call's own deadline (default 30 s); the 8 s race exists only for R,
where it replays a slow read. W after socket loss replays immediately with its key.

### Fire-and-forget telemetry lane (class T)

* Own **socket** (second `/api/ws/unary` connection, same endpoint and admission) so a telemetry frame, a stuck write queue or a
  slow ack cannot delay interactive reads in the client send queue or in TCP. The server also gives each class its own in-flight
  cap inside a socket (R 48, W/M 8, T 8 of the default 64; at other totals R gets the remainder after fixed W/M 8 and T 8, floor 16; enforced before the `inflight` insert so a refusal is `not_executed`. The 32-zombie cap stays per socket, so a telemetry or write zombie flood closes only the telemetry/interactive socket it is on, which is another reason telemetry has its own socket) and
  the writer already puts small frames ahead of CHUNKs, so even a single-socket fallback (see tuning) cannot let telemetry
  starve reads of slots.
* Client `TelemetryLane`: queue bounded by events (200) and bytes (256 KiB); on overflow **drop oldest** and count
  `reason=overflow`. One batch in flight at a time, existing chunking (<=100 events, <=60 KB). A batch carries its own stable
  batch id (header `Idempotency-Key`) and per-event ids so a re-send is deduped by the key store and by `ON CONFLICT DO NOTHING`.
  The unary response is the ack. No ack in 10 s or an `unavailable`: retry the same batch up to 3 times with backoff, then drop
  with `reason=retry_exhausted`. A closed or unavailable telemetry socket never reconnect-loops faster than the existing
  backoff, and while it is down events go to the HTTP path (today's `fetch`) at the existing cadence: at most one request per 20 s per tab, batched, with the same queue bounds, so the fallback cannot reintroduce connection pressure and telemetry is never silently black-holed by the flag. Owner decision 2026-10-30.
* Backpressure: if `WebSocket.bufferedAmount` or the lane's unacked bytes exceed 128 KiB the lane stops handing out new
  batches (events queue, then drop oldest). A refusal from the T class cap (bridge-side, so `not_executed`) or from the existing rate budgets (`analytics_rate_budget.go`, `maxClientLogBatch`, which fire inside the dispatcher and carry no marker) makes the lane drop that batch (`dropped_budget`); class T never retries a refusal, to avoid amplifying the overload it signals.
* **Unload.** WebSockets cannot be flushed on `pagehide`. On `pagehide` and `visibilitychange: hidden` the lane moves every
  queued and every unacked batch to the existing `sendBeacon`/keepalive path (HTTP `/api/analytics`, `/api/client-logs`,
  `LogClientEvents` over the keepalive transport), **same batch ids**. A beacon cannot set headers, so the batch id also travels in the **body** (new additive `batch_id` field on the analytics JSON body, `IngestClientLogs`, and `LogClientEventsRequest`), and the two HTTP handlers dedupe on it themselves (the key store is not on the plain-HTTP path). Analytics dedupes per event id (#1307 `ON CONFLICT`); #1307's id is optional today, so the client now always sends one. `ClientLogEntry` has no id today: Step 2 adds `entry_id` and a unique index with `DoNothing`, otherwise log batches are at-least-once with duplicates, and the ADR does not claim otherwise. Where a WS frame had been received the beacon copy is deduped; where it had not, the beacon delivers it. The sum is at-least-once with dedupe, so the
  lane never loses more than today and usually loses less (today a non-keepalive chunk in flight at navigation is aborted
  with no retry). The 60 KB aggregate beacon budget still bounds what a very large backlog can send; the excess is counted
  `reason=unload_budget`.
* HTTP endpoints stay for old clients, beacons and the HTTP fallback. `/api/analytics` and `/api/client-logs` are not
  Connect RPCs, so they get additive Connect twins (`IngestAnalyticsEvents`, `IngestClientLogs`) whose handlers call the same
  core function as the HTTP handler (one implementation of validation, rate budgets and insert). That keeps the "one
  dispatcher" rule: the WS path never special-cases a non-RPC URL, and the shared core is what the parity test exercises.
  Proto additions only; `buf breaking` must pass.

### Interceptor, auth, CSRF and origin parity

Parity is by construction, not by copy: every WS call re-enters `s.b.dispatchers(listener)`. For writes this needs two
additions, both testable:

1. **Origin and CSRF.** The WS upgrade applies the listener's origin policy once (strict match on remote, loopback on local).
   `LocalWriteGuard` re-checks the copied `Origin`/`Host` per call, so a write class gets the same verdict as `fetch`. The
   WS path has no cookie-ambient CSRF risk beyond the upgrade because the browser always attaches `Origin` to the handshake
   and the policy rejects foreign ones; per-call POST/Content-Type checks are synthesized as `POST` `application/proto`.
2. **Guard coverage test (inverse of the read-side test).** Reads assert "allow-listed => not in `guardedProcedures`".
   For writes the server test enumerates every method in the class table and asserts that, for each one that is in
   `guardedProcedures` or has a body limit, a WS call and a direct HTTP call return the same status for the same hostile
   input matrix (foreign Origin, rebinding Host, oversize body, unauthenticated on `:8444`). A method added to the table
   without a row in that matrix fails the build.

Request-size limits (`BodyLimit` wrappers, `maxClientLogBatch`) run in the dispatcher chain, but the frame cap (256 KiB,
`unaryMaxFrameBytes`) is stricter than the WriteFile body limit, so large writes stay on HTTP by size (existing
`frame.length > MAX_FRAME_BYTES => HTTP`). Rate limits and the Backlog feature-flag interceptor live in the same chain and need no copy.

**Auth lifetime.** The bridge copies the upgrade request's Cookie/Authorization onto every call, so a logout or rotated session is caught only if the server-side session check runs per call against current state. Step 2 verifies this with a test (expire the session mid-socket, assert a W/M call returns `unauthenticated`) and fixes the gap if it exists. A write that gets `unauthenticated` is never replayed over HTTP; it parks the WS and surfaces the error.

**Rate budgets.** `analytics_rate_budget.go` budgets are global (loopback RemoteAddr), so WS and HTTP callers spend the same budget; this is intended. The T class cap per socket is added on top. The per-window id is a metrics/diagnostic label only, not a security boundary.

### Deadlines, cancellation, ordering, flow control

* Per-call deadline: `meta.timeoutMs` becomes the handler context deadline (clamped 5 min), as now. A client abort sends
  CANCEL and cancels the handler context, the same as an HTTP client disconnect; W/M handlers already tolerate that.
  The zombie accounting is unchanged and applies equally to writes; a write that ignores ctx and runs on counts toward the
  32-zombie cap.
* **Ordering.** No ordering guarantee across calls, same as parallel HTTP. Calls that need order must await each other (as the
  HTTP path requires). Within the telemetry lane batches are sent sequentially, so batches arrive in order. Per-key
  serialization is not offered; adding it would reintroduce head-of-line blocking.
* **Head-of-line.** Interactive isolation comes from three layers: separate telemetry socket, per-class in-flight caps,
  writer priority (small frames before CHUNKs). A slow W/M handler occupies one of 8 W/M slots, not a read slot.
* Flow control: unchanged response budgets (256 MiB process, 32 MiB per response); telemetry responses are tiny.
  Request direction is bounded by the 256 KiB frame cap and the client send queue (64 frames / 1 MiB), split per lane.

### Composition with the terminal pool (ADR-033)

Do **not** merge RPC traffic into `/api/ws/terminal-mux`, and do not make terminal streams ride the unary socket (ADR-033
option c stays rejected). Terminal output is long-lived, credit-flow-controlled and large; a write or telemetry frame must
never queue behind it, and an RPC cap must never close a terminal.

Share the **shape**, not the sockets: an `RpcSocketProvider` interface (`transportFor(deliveryClass) -> UnaryWsTransport`)
mirrors `TerminalSocketPool.openStream`. Callers never know how many sockets exist. Default mapping: lane `interactive`
(R, W, M) and lane `telemetry` (T) onto 2 sockets. Tuning precedence copies ADR-033 item 8: `localStorage ssq:rpc-socket-count`
(1 or 2) > server value > default 2. Count 1 maps all classes onto the interactive socket and relies on the per-class server
caps (a valid but weaker isolation). Both RPC sockets together plus terminal sockets stay inside the browser's ~255 global
WebSocket budget measured in ADR-033; the process-level `unary sockets per process = 64` cap is raised proportionally with a
test, not silently.

### Flags and rollout

`unary_writes_over_ws` (default off; enables classes W and T). `unary_mutations_over_ws` (default off; enables class M and is
meaningful only once a method is on the table). Both require the existing `unary_over_ws` master and, on `https`,
`unary_over_ws_remote`. Absent flag = off, so an old server degrades to HTTP, as for #1300. Rollback is the flag; the HTTP
endpoints are never removed.

### Metrics, traces, loss measurement

Bounded labels throughout. Server: `ws_unary_class_inflight{class}`, `ws_unary_idempotency_total{result=hit|join|conflict|miss|evicted}`,
`ws_unary_idempotency_bytes`, `ws_unary_not_executed_total{reason}`. Client (via `client_metrics.go` allow-list, as #1300 does):
`ws_unary_fallback_total{reason}` gains `outcome-unknown`, `idempotent-replay`, `class-flag-off`;
`client_telemetry_events_total{kind=analytics|logs, outcome=acked|dropped_overflow|dropped_retry|dropped_budget|beaconed|http_fallback}`,
`client_telemetry_queue_depth`, `client_telemetry_ack_ms`. Tempo: the bridge's existing per-call span gains `delivery.class`
and `idempotency.result` attributes; the Grafana "Unary WebSocket" row gains per-class panels (additive, stable uids).

**Measuring loss, before and after.** Today and after use the same Playwright run on a throwaway instance (ports 62877+):
generate N tagged events, hide/close the page at a controlled point, and count rows received by tag. Run with the flag off
(baseline) and on; report events generated vs persisted, `/api/analytics` POST count and p95 from `ws_unary` vs HTTP timing,
and HTTP/1.1 connection usage from the HAR-equivalent resource timing. Numbers go in the PR body; this ADR claims none.

## Consequences

* Telemetry leaves the 6-connection pool, which is the evidenced contention (54 analytics POSTs with p95 6.8 s wait in the
  owner's HAR). The size of the interactive gain is not measured yet and is the first thing Step 2 reports.
* One more server component with memory (the key store, 8 MiB cap) and one more table to keep in parity (class table).
* Writes over WS add a third failure mode for M (`outcome_unknown`); that is why M ships empty.
* The WS path is still best-effort infrastructure: the HTTP path is always available and is what the unload drain uses.

## Alternatives rejected

| Option | Why not |
|---|---|
| Move writes to WS with the read replay rules | Replaying a non-idempotent write after an 8 s race or a socket drop can double-execute. |
| Client-only dedupe | The ambiguous case is exactly where the client cannot know whether the server ran the call. |
| Durable (SQLite) key store | Heavier and not needed for W (handler idempotent) or for a no-reuse-after-restart M pilot; revisit if M gains real members. |
| One shared socket for everything with priorities only | Cannot isolate TCP-level and client-queue head-of-line; kept as the `count=1` degraded mode only. |
| Put RPCs on the terminal mux | Couples terminal output latency to RPC traffic and terminal caps to RPC caps (ADR-033 option c). |
| Special-case `/api/analytics` URLs in the bridge | Two dispatch paths to keep in parity; a Connect twin over a shared core keeps one. |
| HTTP/2 or HTTP/3 instead | Removes the 6-connection cap without any of this, but depends on the deployment (local plain HTTP/1.1, remote proxies) and does not give per-class isolation or unload semantics. Worth measuring separately; not a substitute. |

## Definitions

*Lane*: one client socket plus its send queue (`interactive`, `telemetry`). *T cap*: per-socket in-flight limit for class T. `RpcSocketProvider`: client interface returning the lane transport for a class. Call deadline: client default 30 s (`DEFAULT_TIMEOUT_MS`), server clamp 5 min.

## Review round 1

A fresh reviewer found an unsound `not_executed` rule, beacon dedupe without ids, undefined key-store cancel/error semantics, `anon` key collisions, auth lifetime on long sockets and unenforced class caps; all are folded into the sections above. HAR figures and ADR-033's numbers are cited from the brief and that ADR, not re-derived.

## Round 2 amendments (these supersede earlier text where they conflict)

Three fresh reviewers (delivery, security, concurrency) found the following; each is a decision.

**Key store semantics**
* While a handler goroutine is alive its entry stays `in_flight` regardless of the caller's deadline or CANCEL (`abandon()` replies `deadline_exceeded` and the handler keeps running). A later call with the same key joins it; it never starts a second run. A joiner's own timeout returns `deadline_exceeded` and leaves the entry.
* When the handler returns: terminal 2xx or deterministic 4xx becomes `done`. For W, any other outcome (5xx, `unavailable`, panic) deletes the entry so a retry re-executes (safe because W is naturally idempotent). For M, those outcomes become `done(unknown)` and a replay returns `outcome_unknown`; M is "at-most-once within TTL and process life".
* The key is enforced (`invalid_argument` on missing) only for calls stamped `transport=ws` by the bridge; on HTTP it is optional so old clients, beacons and keyless callers keep working. Body-`batch_id`/event-id dedupe on the plain HTTP handlers is a separate mechanism and is a precondition of flag-on: the `entry_id` unique index for logs must land first, otherwise logs are at-least-once with duplicates and the flag stays off.
* The hash is over the canonical decoded request message, not the wire bytes, so a WS-to-HTTP replay (proto vs JSON) matches. Test the replay path across codecs.
* Stored response bodies for W/T are reduced to status plus an `ok` marker (their responses carry no data), 4 KiB max otherwise; compare hash constant-time; add an entry-count cap (10 000) and a per-socket quota of live keys; the refusal path does not allocate. Principal: the auth session id, never the raw token (rotation must not fork the namespace). On the unauthenticated local listener the principal is `anon`, so another local process that learns a key can replay it; this is accepted (the listener already trusts any local process) and noted in the threat model.
* A keyed handler continuing after its caller abandoned is not counted as a zombie; it is bounded by the keyed in-flight quota. Joiners wait without taking a class slot, capped at 8 joiners per key.
* Upgrade-time `Idempotency-Key` headers never reach a handler (test); the bridge validates the key pattern at REQUEST.

**Caps and lanes**
* Add `class` to `inflightCall` and a per-class counter checked under `s.mu` with the `dup`/`full` checks, released where `releaseCall` and `detachLocked` delete the entry. State: slots bound *accepted* calls, not running handlers; running-but-abandoned handlers are bounded only by the per-socket 32 zombie cap and the process 128 cap.
* Class caps (R/W+M/T) apply only when the write flags are on; flags off keeps today's single 64. Split: W+M 8, T 8, R the remainder, only for totals >= 32; below that, one shared cap.
* The table carries `class` and a separate `lane` (`interactive` or `telemetry`), so analytics is class W on lane telemetry. `LogClientEvents` is listed once: W, telemetry lane.
* One `UnaryWsBreaker` per lane; telemetry failures never open the interactive breaker. Auth parking is per page, not a breaker failure, and parks both lanes. Reconnect backoff has jitter, and the telemetry socket opens lazily on first telemetry flush (not at page load) to avoid handshake contention (ADR-033 serialized opens).
* `count=1` keeps logical per-lane queues in the client with pump priority R, then W/M, then T, even on one transport. Writer priority orders bytes, not slots; slot protection comes only from the caps.
* Socket math: sockets per tab = 2 + terminal pool N; the process unary cap (64) is raised to 128 with a test, since two sockets per tab halves tab capacity otherwise. The server config value for `rpc-socket-count` rides in the first `PONG` (additive field); until then the client uses the default.
* Mixed versions: an old server answers the new meta header with `invalid_argument` (`firstDisallowedHeader`); the client treats that for a keyed call as HTTP-safe (it fails before dispatch) and sets a per-page denylist. Where class flags are read: at call time via `readWsFlag`, as for the master flag.
* Metrics: the per-window id is logs/spans only, never a label. `not_executed` also stamps the oversize-frame reply (`ws_unary_bridge.go:419`). For M, server `internal` (panic) also maps to `outcome_unknown`.

**Guards and auth**
* Every method in the class table must be in `guardedProcedures` (Rebinding profile) or carry an explicit reviewed-unguarded entry; the hostile-input matrix (foreign Origin, rebinding Host, oversize body, unauthenticated on :8444) runs over the whole table, and compares three paths: plain HTTP handler, twin over HTTP, twin over WS. The twins join the body-limit wrappers.
* Auth runs per call via `validator.ValidateAuthSession(cookie)` on the copied upgrade Cookie, so a logout is visible only if that validator reads live state. Step 2 tests: after logout the same socket returns `unauthenticated`; a stored `done` entry never bypasses auth (the store sits after auth). Over WS the Authorization header is frozen at upgrade, unlike HTTP; the server closes the socket on logout/rotation if the validator cannot tell. The local upgrade accepts a missing Origin and relies on HostGuard; non-browser local processes are in the threat model.
* Rate budgets key on loopback RemoteAddr; a test confirms WS and HTTP callers share them before calling that intended.

**Client policy and unload**
* Event ids are assigned at `track()` time and batch composition is frozen for same-key retries.
* W gets its own branch in `onWsError`: replay over HTTP with the same key on `unavailable`, `resource_exhausted`, socket close and timeout, without `ac.abort()` (no CANCEL).
* Unload: the over-budget excess stays queued for a tab that returns; it is dropped (`unload_budget`) only on `pagehide` with `persisted=false`. One terminal outcome per event id (`acked`, `beaconed`, `dropped_*`); a batch beaconed and later acked counts as `acked`. Unacked bytes count toward the 256 KiB bound and an in-flight batch is never evicted by drop-oldest. After a `visible` return the lane resumes.
* "Never loses more than today" is restated: no more than today for events that reach `pagehide`, with the log-dedupe precondition above.

## Owner decisions (2026-10-30)

1. Class M stays empty at launch; no pilot.
2. Telemetry falls back to HTTP while the telemetry socket is down, rate-capped as above.
