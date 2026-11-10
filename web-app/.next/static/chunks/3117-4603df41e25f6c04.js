"use strict";
(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[3117],{

/***/ 8854:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   u: () => (/* binding */ getErrorMessage)
/* harmony export */ });
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(45640);

/**
 * Converts a thrown value from a ConnectRPC client call into a human-readable
 * message, using `ConnectError.rawMessage` (no `[code]` prefix) instead of the
 * `[code] message`-formatted `.message`. Non-ConnectError Errors fall back to
 * `.message`; anything else (including an empty `rawMessage`) falls back to
 * the caller-supplied fallback string.
 */ function getErrorMessage(e, fallback) {
    if (e instanceof _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T) return e.rawMessage || fallback;
    if (e instanceof Error) return e.message || fallback;
    return fallback;
}


/***/ }),

/***/ 32107:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   j: () => (/* binding */ noteDeployId)
/* harmony export */ });
/**
 * Compares a watch-stream event's deploy_id with the last one seen on that stream. A change means
 * the server process restarted (seq reset), so the caller should drop its afterSeq cursor and resync.
 * An empty id (older server) is ignored; the caller's seq-backwards-jump heuristic still covers it.
 */ function noteDeployId(lastSeen, incoming, stream) {
    if (!incoming) return false;
    const previous = lastSeen.current;
    lastSeen.current = incoming;
    if (!previous || previous === incoming) return false;
    console.warn("[deploy] stream=".concat(stream, " server redeployed ").concat(previous, " -> ").concat(incoming));
    return true;
}


/***/ }),

/***/ 36095:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   t: () => (/* binding */ useWatchStream)
/* harmony export */ });
/* harmony import */ var _deployId__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(32107);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* __next_internal_client_entry_do_not_use__ useWatchStream auto */ // useWatchStream.ts — generic, transport-agnostic connection-lifecycle
// primitive shared by every Watch* streaming RPC's frontend hook
// (useWatchWorkflows.ts, useWatchBacklogItems.ts, useReviewQueue.ts's watch,
// useSessionService.ts's watch). Owns exactly the connection mechanics that
// were previously duplicated -- with copy-pasted constants -- across those
// hooks: exponential-backoff reconnect on a real stream error/close, plus an
// idle-staleness watchdog that force-reconnects when a connection goes
// silent without ever erroring (e.g. a proxy idle-timeout that just stops
// relaying bytes, which never surfaces to the client's stream reader as an
// error or a close). Business-event dispatch, seq/afterSeq bookkeeping, and
// Redux/local-state updates stay in each call site; this hook only decides
// *when* to (re)connect and hands every non-heartbeat message to onEvent.
//
// A message counts as "activity" whether it's a real business event or a
// synthetic heartbeat -- both prove the connection is alive. Server-side
// heartbeat cadence should be well under staleThresholdMs (aim for ~1/3) so
// a couple of missed heartbeats, not just one, are required to trigger a
// reconnect.
//
// maxRetries is optional: omitted, backoff retries forever (workflows,
// review queue). Set it for a caller that wants to give up on automatic
// retries and fall back to something else (backlog items' REST poll) --
// the hook reports "exhausted" and stops; the caller's own reconnect()
// (returned below) revives it, e.g. once that fallback poll confirms the
// server is reachable again.


const LIVE = "live";
const RECONNECTING = "reconnecting";
const EXHAUSTED = "exhausted";
const DEFAULT_STALE_THRESHOLD_MS = 45000;
const WATCHDOG_INTERVAL_MS = 10000;
const MAX_BACKOFF_MS = 30000;
const VISIBILITY_DEBOUNCE_MS = 200;
const NO_SEQ = 0n;
function setConnectionState(state, next) {
    var _state_optionsRef_current_onConnectionStateChange, _state_optionsRef_current;
    if (next === state.connectionState) return;
    state.connectionState = next;
    (_state_optionsRef_current_onConnectionStateChange = (_state_optionsRef_current = state.optionsRef.current).onConnectionStateChange) === null || _state_optionsRef_current_onConnectionStateChange === void 0 ? void 0 : _state_optionsRef_current_onConnectionStateChange.call(_state_optionsRef_current, next);
}
/** Schedules the next reconnect, or reports "exhausted" and stops once maxRetries is hit. */ function scheduleReconnect(state) {
    if (state.stopped) return;
    const { maxRetries } = state.optionsRef.current;
    if (maxRetries !== undefined && state.retries >= maxRetries) {
        setConnectionState(state, EXHAUSTED);
        return;
    }
    const delay = Math.min(1000 * Math.pow(2, state.retries), MAX_BACKOFF_MS);
    state.retries++;
    setTimeout(()=>{
        if (!state.stopped) void connect(state);
    }, delay);
}
/** Dispatches one received message: seq bookkeeping, then onEvent unless it's a heartbeat. */ function handleMessage(state, event) {
    state.lastActivity = Date.now();
    const { getSeq, getDeployId, name, isHeartbeat, onEvent } = state.optionsRef.current;
    if (getDeployId && (0,_deployId__WEBPACK_IMPORTED_MODULE_1__/* .noteDeployId */ .j)(state.lastDeployIdRef, getDeployId(event), name !== null && name !== void 0 ? name : "watch")) {
        state.lastSeqRef.current = NO_SEQ; // new server process: its seq restarted from zero
    }
    if (getSeq) {
        const seq = getSeq(event);
        if (seq > NO_SEQ) state.lastSeqRef.current = seq;
    }
    if (isHeartbeat(event)) return;
    onEvent(event);
}
/** One connect-consume-reconnect cycle. Split out to keep the setup effect below flat. */ async function connect(state) {
    if (state.stopped) return;
    const signal = state.abortController.signal;
    // Treat the connect attempt itself as activity so the watchdog can engage
    // even if this attempt never yields a single message.
    state.lastActivity = Date.now();
    try {
        const stream = state.optionsRef.current.subscribe(state.lastSeqRef.current, signal);
        let firstMessage = true;
        for await (const event of stream){
            if (signal.aborted) return;
            if (firstMessage) {
                firstMessage = false;
                state.retries = 0;
                setConnectionState(state, LIVE);
            }
            handleMessage(state, event);
        }
        // Clean server-side close -- reconnect without backoff.
        if (signal.aborted) return;
        state.retries = 0;
        setConnectionState(state, RECONNECTING);
        scheduleReconnect(state);
    } catch (err) {
        var _state_optionsRef_current_isFatalError, _state_optionsRef_current;
        if (err instanceof Error && err.name === "AbortError") return;
        if (signal.aborted) return;
        console.error("[useWatchStream] stream error:", err);
        if ((_state_optionsRef_current_isFatalError = (_state_optionsRef_current = state.optionsRef.current).isFatalError) === null || _state_optionsRef_current_isFatalError === void 0 ? void 0 : _state_optionsRef_current_isFatalError.call(_state_optionsRef_current, err)) {
            setConnectionState(state, EXHAUSTED);
            return;
        }
        setConnectionState(state, RECONNECTING);
        scheduleReconnect(state);
    }
}
/**
 * Aborts whatever attempt is in flight (even one silently hung with no
 * error) and starts a brand-new one. The old attempt's `for await` rejects
 * with AbortError and no-ops in connect()'s own catch block.
 */ function forceReconnect(state) {
    if (state.stopped) return;
    state.abortController.abort();
    state.abortController = new AbortController();
    state.retries = 0;
    setConnectionState(state, "stale");
    void connect(state);
}
function isStale(state, threshold) {
    return Date.now() - state.lastActivity > threshold;
}
function newLoopState(optionsRef, lastSeqRef) {
    return {
        optionsRef,
        lastSeqRef,
        lastDeployIdRef: {
            current: ""
        },
        abortController: new AbortController(),
        retries: 0,
        stopped: false,
        lastActivity: Date.now(),
        connectionState: "connecting"
    };
}
/**
 * A connection that never reached "live" (still connecting/reconnecting/
 * stale/exhausted) is treated as stale regardless of elapsed time -- a tab
 * refocusing or the network coming back online while a connect attempt is
 * still stuck is exactly when a forced reconnect is most useful.
 */ function isStaleOrNotLive(state, threshold) {
    return state.connectionState !== LIVE || isStale(state, threshold);
}
/** Wires up the watchdog timer + tab-refocus/online check and returns a teardown fn. */ function startStaleWatchdog(state) {
    const watchdog = setInterval(()=>{
        var _state_optionsRef_current_staleThresholdMs;
        const threshold = (_state_optionsRef_current_staleThresholdMs = state.optionsRef.current.staleThresholdMs) !== null && _state_optionsRef_current_staleThresholdMs !== void 0 ? _state_optionsRef_current_staleThresholdMs : DEFAULT_STALE_THRESHOLD_MS;
        if (isStale(state, threshold)) forceReconnect(state);
    }, WATCHDOG_INTERVAL_MS);
    if (state.optionsRef.current.enableVisibilityWatchdog === false) {
        return ()=>clearInterval(watchdog);
    }
    let debounceTimer = null;
    const checkAndReconnect = ()=>{
        if (debounceTimer) clearTimeout(debounceTimer);
        debounceTimer = setTimeout(()=>{
            debounceTimer = null;
            const { staleThresholdMs, visibilityStaleThresholdMs } = state.optionsRef.current;
            var _ref;
            const threshold = (_ref = visibilityStaleThresholdMs !== null && visibilityStaleThresholdMs !== void 0 ? visibilityStaleThresholdMs : staleThresholdMs) !== null && _ref !== void 0 ? _ref : DEFAULT_STALE_THRESHOLD_MS;
            if (isStaleOrNotLive(state, threshold)) forceReconnect(state);
        }, VISIBILITY_DEBOUNCE_MS);
    };
    const handleVisibility = ()=>{
        if (document.visibilityState === "visible") checkAndReconnect();
    };
    document.addEventListener("visibilitychange", handleVisibility);
    window.addEventListener("online", checkAndReconnect);
    return ()=>{
        clearInterval(watchdog);
        if (debounceTimer) clearTimeout(debounceTimer);
        document.removeEventListener("visibilitychange", handleVisibility);
        window.removeEventListener("online", checkAndReconnect);
    };
}
/**
 * Subscribes to a generic server-streaming RPC and keeps it connected:
 * exponential backoff on real errors/closes, plus an idle-staleness
 * watchdog (checked periodically and on tab refocus) that force-reconnects
 * a connection that has gone silent without erroring.
 */ function useWatchStream(options) {
    // Ref'd so the connection effect below doesn't need to re-run (and
    // reconnect) just because the caller passed fresh callback identities.
    const optionsRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(options);
    optionsRef.current = options;
    const lastSeqRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(NO_SEQ);
    var _options_enabled;
    const enabled = (_options_enabled = options.enabled) !== null && _options_enabled !== void 0 ? _options_enabled : true;
    const reconnectRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(()=>{});
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        if (!enabled) return undefined;
        // A fresh connection (mount, or a restartKey/enabled change) always
        // starts from a clean afterSeq -- this only actually discards state on
        // a restartKey change, since mount already starts at the ref's initial
        // 0n and enabled toggling off tears the whole effect down anyway.
        lastSeqRef.current = NO_SEQ;
        const state = newLoopState(optionsRef, lastSeqRef);
        reconnectRef.current = ()=>forceReconnect(state);
        void connect(state);
        const stopWatchdog = startStaleWatchdog(state);
        return ()=>{
            state.stopped = true;
            reconnectRef.current = ()=>{};
            stopWatchdog();
            state.abortController.abort();
        };
    // subscribe/onEvent/etc. are read live via optionsRef, so identity
    // changes on those don't restart the connection -- only enabled/
    // restartKey do.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        enabled,
        options.restartKey
    ]);
    return (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>({
            reconnect: ()=>reconnectRef.current()
        }), []);
}


/***/ }),

/***/ 46798:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Et: () => (/* binding */ BackoffState),
/* harmony export */   GN: () => (/* binding */ isWorktreeMissingError),
/* harmony export */   Ir: () => (/* binding */ connectTimeoutMs),
/* harmony export */   Qi: () => (/* binding */ isNonRetriableConnectError),
/* harmony export */   ku: () => (/* binding */ getWsCloseCode)
/* harmony export */ });
/* unused harmony exports jitteredDelay, NON_RETRIABLE_WS_CODES, isRetriableCloseCode, FOREGROUND_CONNECT_TIMEOUT_MS, CONNECT_TIMEOUT_MS, FOREGROUND_FAST_ATTEMPTS */
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(45640);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(16294);

/**
 * Full-jitter exponential backoff delay.
 * Formula: Math.random() * Math.min(capMs, baseMs * 2^attempt)
 * Recommended by the AWS architecture blog to prevent thundering-herd bursts.
 */ function jitteredDelay(baseMs, capMs, attempt) {
    const ceiling = Math.min(capMs, baseMs * Math.pow(2, attempt));
    return Math.random() * ceiling;
}
/**
 * Stateful backoff tracker that increments the attempt counter on each call to next().
 */ class BackoffState {
    get attempt() {
        return this._attempt;
    }
    /** Returns the next jittered delay in milliseconds and increments the attempt counter. */ next() {
        const delay = jitteredDelay(this.baseMs, this.capMs, this._attempt);
        this._attempt++;
        return delay;
    }
    /** Resets the attempt counter to 0. */ reset() {
        this._attempt = 0;
    }
    constructor(baseMs, capMs){
        this.baseMs = baseMs;
        this.capMs = capMs;
        this._attempt = 0;
    }
}
// ---------------------------------------------------------------------------
// WebSocket close-code helpers
// ---------------------------------------------------------------------------
/**
 * WebSocket close codes that should NOT trigger a reconnect.
 *   4001 — authentication failure (reconnecting won't help)
 *   4004 — session not found (session is gone; reconnecting is pointless)
 */ const NON_RETRIABLE_WS_CODES = new Set([
    4001,
    4004
]);
/**
 * Returns true if the given WebSocket close code should trigger a reconnect attempt.
 */ function isRetriableCloseCode(code) {
    return !NON_RETRIABLE_WS_CODES.has(code);
}
/**
 * Extracts the `ws-close-code` header value from a ConnectError, returning the
 * numeric code or null if the error is not a ConnectError or the header is absent.
 */ function getWsCloseCode(err) {
    var _err_metadata;
    if (!(err instanceof _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T)) return null;
    var _err_metadata_get;
    const raw = (_err_metadata_get = (_err_metadata = err.metadata) === null || _err_metadata === void 0 ? void 0 : _err_metadata.get("ws-close-code")) !== null && _err_metadata_get !== void 0 ? _err_metadata_get : "";
    const code = parseInt(raw, 10);
    return isNaN(code) ? null : code;
}
/**
 * Returns true if `err` represents a non-retriable failure, across BOTH the
 * WS-bridge transport (createWatchTransport) and the native ConnectRPC
 * transport (createConnectTransport).
 *
 * The WS-bridge signals this via a `ws-close-code` header (see getWsCloseCode);
 * createConnectTransport never sets that header, so it's checked first and,
 * when absent, falls back to comparing the ConnectError's own `code` against
 * the native transport's equivalent signal for the same underlying
 * server-side conditions (auth failure, session gone). Mirrors
 * NON_RETRIABLE_WS_CODES' semantics:
 *   Code.Unauthenticated — equivalent to ws-close-code 4001 (auth failure)
 *   Code.NotFound        — equivalent to ws-close-code 4004 (session not found)
 *
 * The `Code.*` comparison is deferred until this function actually runs (not
 * hoisted into a module-level constant) so importing this module never
 * touches the `Code` enum eagerly — several existing tests mock
 * `@connectrpc/connect` with only a subset of its exports (e.g. just
 * `createClient`), and a top-level `Code.Unauthenticated` reference would
 * throw at import time under those mocks even though the mocked test never
 * exercises this branch.
 *
 * Deliberately does NOT include Code.FailedPrecondition: this helper is
 * shared by the session-list "watch" stream (useSessionService.ts) as well
 * as the terminal stream, and FailedPrecondition's one current meaning
 * (see isWorktreeMissingError below) is specific to the terminal stream's
 * missing-working-directory case. A future unrelated FailedPrecondition on
 * the watch stream must not silently stop reconnecting there too — callers
 * that need the worktree-missing behavior check isWorktreeMissingError
 * explicitly alongside this function instead.
 */ function isNonRetriableConnectError(err) {
    if (!(err instanceof _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T)) return false;
    const wsCode = getWsCloseCode(err);
    if (wsCode !== null) {
        return !isRetriableCloseCode(wsCode);
    }
    return err.code === _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__/* .Code */ .C.Unauthenticated || err.code === _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__/* .Code */ .C.NotFound;
}
/**
 * Returns true if `err` is specifically the "session's working directory no
 * longer exists" stream failure (server/services/connectrpc_websocket.go's
 * handleTmuxRestoreFailure sends Code.FailedPrecondition only for this
 * case — see isNonRetriableConnectError above). Lets a caller show a more
 * actionable message than the generic hard-failed banner: reconnecting the
 * terminal stream can never fix this on its own, but the session-level
 * "Retry now" action re-creates the worktree before restarting.
 */ function isWorktreeMissingError(err) {
    return err instanceof _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T && err.code === _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__/* .Code */ .C.FailedPrecondition;
}
// ---------------------------------------------------------------------------
// Connect-timeout policy: per-attempt duration cap (foreground vs. background),
// distinct from BackoffState's delay-between-attempts above.
// ---------------------------------------------------------------------------
/**
 * Fast connect-timeout for the first FOREGROUND_FAST_ATTEMPTS attempts since a
 * terminal became foreground. Unvalidated starting guess (herdr-web's real value
 * isn't inspectable) — validate against real p95/p99 connect-to-first-message
 * latency on VPN/high-RTT links before enabling NEXT_PUBLIC_RECONNECT_V2 broadly.
 */ const FOREGROUND_CONNECT_TIMEOUT_MS = 1200;
/** Normal connect-timeout: background attempts, and foreground attempts beyond FOREGROUND_FAST_ATTEMPTS. */ const CONNECT_TIMEOUT_MS = 3500;
/** Number of connect attempts (since the most recent foreground transition) eligible for the fast timeout. */ const FOREGROUND_FAST_ATTEMPTS = 2;
/**
 * Returns the connect-timeout (ms) for a reconnect attempt: the maximum time
 * to wait for the first stream message before abandoning the attempt.
 * Foreground terminals get a shorter timeout for their first
 * FOREGROUND_FAST_ATTEMPTS attempts since becoming foreground; all other
 * attempts (background, or foreground beyond the fast window) use the
 * normal timeout.
 */ function connectTimeoutMs(foreground, attemptsSinceForeground) {
    if (foreground && attemptsSinceForeground < FOREGROUND_FAST_ATTEMPTS) {
        return FOREGROUND_CONNECT_TIMEOUT_MS;
    }
    return CONNECT_TIMEOUT_MS;
}


/***/ }),

/***/ 73117:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  D: () => (/* binding */ useSessionService)
});

// UNUSED EXPORTS: LIST_SESSIONS_TIMEOUT_MS

// EXTERNAL MODULE: ./src/lib/hooks/deployId.ts
var deployId = __webpack_require__(32107);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/connect-error.js
var connect_error = __webpack_require__(45640);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/code.js
var code = __webpack_require__(16294);
// EXTERNAL MODULE: ./src/lib/transport/watch-ws-transport.ts
var watch_ws_transport = __webpack_require__(90062);
// EXTERNAL MODULE: ./src/gen/session/v1/session_pb.ts
var session_pb = __webpack_require__(23575);
// EXTERNAL MODULE: ./src/gen/session/v1/types_pb.ts
var types_pb = __webpack_require__(81432);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/create.js
var create = __webpack_require__(95444);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/timestamp.js
var timestamp = __webpack_require__(28765);
// EXTERNAL MODULE: ./src/lib/config.ts
var config = __webpack_require__(51202);
// EXTERNAL MODULE: ./src/lib/utils/backoff.ts
var backoff = __webpack_require__(46798);
;// ./src/lib/utils/refreshCoordinator.ts
/**
 * Coalesces concurrent `request()` calls into at most one in-flight fetch
 * plus at most one queued rerun, so callers never trigger two concurrent
 * fetches for the same resource. A fetch that's been superseded by a newer
 * request before it starts is discarded (its fetcher never runs); its
 * `onResult` is correspondingly never invoked. Every caller's own returned
 * promise settles (resolve or reject) exactly once, including callers whose
 * request was coalesced away — this is what lets a caller's own
 * `finally { setLoading(false) }` always run.
 *
 * Never dispatch the return value of `request()` — it resolves to
 * `Promise<void>`. All side effects happen inside `onResult`, invoked by the
 * coordinator itself, only for the fetcher whose result actually wins.
 *
 * Modeled after herdr-web's `createSnapshotRefreshController`, adapted to a
 * strictly serialized (never >1 fetch in flight) design and a discriminated
 * `CoordinatorState<T>` in place of two independent booleans.
 */ class RefreshCoordinator {
    request(fetcher, onResult) {
        let opts = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : {};
        var _opts_guarded;
        const guarded = (_opts_guarded = opts.guarded) !== null && _opts_guarded !== void 0 ? _opts_guarded : false;
        if (this.state.kind === "idle") {
            return this.run(fetcher, onResult);
        }
        const existingPending = this.state.kind === "inFlightWithPending" ? this.state.pending : undefined;
        var _existingPending_waiters;
        // Blocker 2 fix: never let a non-guarded caller overwrite an
        // already-queued guarded fetcher — the guarded RPC must still fire.
        const pending = existingPending && existingPending.guarded && !guarded ? existingPending : {
            fetcher,
            onResult,
            guarded,
            waiters: (_existingPending_waiters = existingPending === null || existingPending === void 0 ? void 0 : existingPending.waiters) !== null && _existingPending_waiters !== void 0 ? _existingPending_waiters : []
        };
        const promise = new Promise((resolve, reject)=>{
            pending.waiters.push({
                resolve,
                reject
            });
        });
        this.state = {
            kind: "inFlightWithPending",
            pending
        };
        return promise;
    }
    async run(fetcher, onResult) {
        const myGeneration = ++this.generation;
        this.state = {
            kind: "inFlight"
        };
        try {
            const result = await fetcher();
            // Defensive: under the strictly serialized design (at most 1 fetch in
            // flight, see request()'s pending overwrite) a superseded fetcher is
            // never even invoked, so this can't currently mismatch — retained as
            // a guard against a future refactor breaking that invariant.
            if (myGeneration === this.generation) {
                onResult(result);
            }
        } finally{
            // No await between reading/clearing state and kicking off the queued
            // rerun (if any) — this runs synchronously so no other request() call
            // can observe a torn state between "current fetch done" and "next
            // fetch started."
            this.drain();
        }
    }
    drain() {
        const pending = this.state.kind === "inFlightWithPending" ? this.state.pending : undefined;
        this.state = {
            kind: "idle"
        };
        if (!pending) return;
        this.run(pending.fetcher, pending.onResult).then(()=>pending.waiters.forEach((w)=>w.resolve()), (err)=>pending.waiters.forEach((w)=>w.reject(err)));
    }
    constructor(){
        this.state = {
            kind: "idle"
        };
        // Per-fetch-start counter; see the myGeneration check in run() for why.
        this.generation = 0;
    }
}

// EXTERNAL MODULE: ./src/lib/hooks/useWatchStream.ts
var useWatchStream = __webpack_require__(36095);
// EXTERNAL MODULE: ./src/lib/telemetry/rpcTiming.ts
var rpcTiming = __webpack_require__(77665);
// EXTERNAL MODULE: ./src/lib/utils/connectError.ts
var connectError = __webpack_require__(8854);
// EXTERNAL MODULE: ./src/lib/contexts/AnalyticsContext.tsx
var AnalyticsContext = __webpack_require__(24044);
// EXTERNAL MODULE: ./src/lib/contexts/NotificationContext.tsx + 12 modules
var NotificationContext = __webpack_require__(62275);
// EXTERNAL MODULE: ./src/lib/store/index.ts
var store = __webpack_require__(93602);
// EXTERNAL MODULE: ./src/lib/store/sessionsSlice.ts
var sessionsSlice = __webpack_require__(60795);
// EXTERNAL MODULE: ./src/lib/store/remotesSlice.ts
var remotesSlice = __webpack_require__(81921);
// EXTERNAL MODULE: ./src/lib/store/reviewQueueSlice.ts
var reviewQueueSlice = __webpack_require__(68453);
;// ./src/lib/hooks/useSessionService.ts
/* provided dependency */ var process = __webpack_require__(36843);
/* __next_internal_client_entry_do_not_use__ LIST_SESSIONS_TIMEOUT_MS,useSessionService auto */ 

















// NOTE (backlog #488, 2026-08-17): this file's other `dispatch(setError(...))`
// sites are deliberately NOT converted to the getErrorMessage() helper
// (web-app/src/lib/utils/connectError.ts) used everywhere in components/backlog/**.
// `setError` here writes to the shared sessionsSlice Redux store, consumed by
// non-backlog surfaces (the general session list/terminal views) — stripping the
// ConnectRPC `[code]` prefix there is a behavior change with a materially larger
// blast radius than the local useState catch-block conversions this PR made, and
// is out of scope for a backlog-toast presentation fix. Tracked as a follow-up,
// not fixed here.
//
// updateSession()'s setError call below is the one exception: BacklogItemDetail.tsx's
// handleSteerSession reads this exact value back via selectSessionsError() and
// re-throws it into a backlog action toast, so it IS a backlog-facing surface
// this PR touches — stripped here to match.


// ponytail: stable empty array so non-watching callers (e.g. useSessionActions
// in each SessionCard) don't subscribe to the full sessions list. Without this,
// 29 cards × N events/second = N*29 card re-renders/second saturating the JS thread.
const EMPTY_SESSIONS = [];
const selectNoSessions = ()=>EMPTY_SESSIONS;

// Bounds the createSession RPC so a stalled backend (e.g. a stuck tmux start
// or hung GitHub clone) can't leave the omnibar's Create button grayed out
// forever — the promise always settles, letting the caller's catch handler
// reset isSubmitting and surface an error. Kept comfortably above the
// server's own createSessionTimeout (150s in session_service.go); if that
// value changes, update this one too.
const CREATE_SESSION_TIMEOUT_MS = 160000;
// Bounds every ListSessions RPC (all 4 call sites share refreshCoordinatorRef,
// see below) so a hung/slow backend can't wedge the coordinator's single
// in-flight slot forever — adversarial-review Blocker 1. A read-only list
// call is expected to be fast; well above typical latency but far below
// CREATE_SESSION_TIMEOUT_MS since no backend work (tmux/clone) is involved.
const LIST_SESSIONS_TIMEOUT_MS = 15000;
const DISCONNECTED = "disconnected";
function useSessionService() {
    let options = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : {};
    const { baseUrl = (0,config/* getApiBaseUrl */.h)(), autoWatch = false, enabled = true, onNotification, onReconnect, onApprovalResponse, onSessionDeleted } = options;
    const analytics = (0,AnalyticsContext/* useAnalytics */.st)();
    const onReconnectRef = (0,react.useRef)(onReconnect);
    (0,react.useEffect)(()=>{
        onReconnectRef.current = onReconnect;
    }, [
        onReconnect
    ]);
    const onNotificationRef = (0,react.useRef)(onNotification);
    const onApprovalResponseRef = (0,react.useRef)(onApprovalResponse);
    const onSessionDeletedRef = (0,react.useRef)(onSessionDeleted);
    // Keep ref updated for callback in streaming loop
    (0,react.useEffect)(()=>{
        onNotificationRef.current = onNotification;
    }, [
        onNotification
    ]);
    (0,react.useEffect)(()=>{
        onApprovalResponseRef.current = onApprovalResponse;
    }, [
        onApprovalResponse
    ]);
    (0,react.useEffect)(()=>{
        onSessionDeletedRef.current = onSessionDeleted;
    }, [
        onSessionDeleted
    ]);
    const dispatch = (0,store/* useAppDispatch */.jL)();
    const { addNotification } = (0,NotificationContext/* useNotifications */.E$)();
    const [systemMemoryPct, setSystemMemoryPct] = (0,react.useState)(0);
    const [reconnectAttemptCount, setReconnectAttemptCount] = (0,react.useState)(0);
    const sessions = (0,store/* useAppSelector */.GV)(autoWatch ? sessionsSlice/* selectAllSessions */.rt : selectNoSessions);
    const sessionsRef = (0,react.useRef)(sessions);
    sessionsRef.current = sessions;
    const loading = (0,store/* useAppSelector */.GV)(sessionsSlice/* selectSessionsLoading */.OS);
    const hasLoadedOnce = (0,store/* useAppSelector */.GV)(sessionsSlice/* selectSessionsHasLoadedOnce */.Qr);
    const errorStr = (0,store/* useAppSelector */.GV)(sessionsSlice/* selectSessionsError */.AY);
    // Async-session-creation Epic 5.3 (Surface 4): fire a global failure toast
    // exactly once per Creating -> Failed transition, regardless of which
    // page/session the user is currently viewing. This reads Redux `sessions`
    // state (populated by both the WatchSessions stream and ListSessions
    // snapshots via the same upsertSession/setSessions actions) rather than
    // hooking the raw stream event directly, so it fires identically whichever
    // path delivered the transition.
    //
    // `lastKnownStatusRef` is the dedup guard: a session's id is only ever
    // recorded here on a render where it was already present with a
    // *different* status, so (a) a session's first-ever appearance (including
    // one that is already Failed on initial load, e.g. after a page refresh)
    // never fires a toast, and (b) once fired, subsequent renders with the
    // same Failed status never re-fire — a status transition notifies exactly
    // once, not once per re-render or redundant stream event.
    const lastKnownStatusRef = (0,react.useRef)(new Map());
    (0,react.useEffect)(()=>{
        if (!enabled || !autoWatch) return;
        const seen = lastKnownStatusRef.current;
        for (const s of sessions){
            const prevStatus = seen.get(s.id);
            if (prevStatus !== undefined && prevStatus !== types_pb/* SessionStatus */.Ph.FAILED && s.status === types_pb/* SessionStatus */.Ph.FAILED) {
                addNotification({
                    sessionId: s.id,
                    sessionName: s.title || s.id,
                    title: "Session creation failed",
                    message: (0,NotificationContext/* getFailureReasonToastMessage */.rN)(s.failureReason),
                    notificationType: "task_failed",
                    priority: "high",
                    metadata: {
                        failure_reason: s.failureReason
                    }
                });
            }
            seen.set(s.id, s.status);
        }
        // Prune ids no longer present so a deleted-then-recreated session (same
        // title reused) is treated as a fresh first-appearance, not a transition.
        const currentIds = new Set(sessions.map((s)=>s.id));
        for (const id of seen.keys()){
            if (!currentIds.has(id)) seen.delete(id);
        }
    }, [
        sessions,
        enabled,
        autoWatch,
        addNotification
    ]);
    const clientRef = (0,react.useRef)(null);
    // Coalesces concurrent ListSessions fetches across all 4 call sites in
    // this hook (listSessions, watch-stream initial snapshot, backwards-jump
    // resync ×2) — see refreshCoordinator.ts.
    const refreshCoordinatorRef = (0,react.useRef)(new RefreshCoordinator());
    // Last seen event sequence number — passed as after_seq on reconnect so the
    // server replays any events missed during the disconnect window (up to 1 hour).
    // Kept separately from useWatchStream's own internal afterSeq bookkeeping
    // because this copy is also used for backwards-jump detection (below).
    const lastSeqRef = (0,react.useRef)(0n);
    const lastDeployIdRef = (0,react.useRef)("");
    // Stores current watch options so reconnects use the latest options without stale closure
    const watchOptionsRef = (0,react.useRef)(undefined);
    // True once watchSessions() has been called (autoWatch, or a manual call) and
    // stopWatching() hasn't run since — gates useWatchStream's connection effect.
    const [isWatching, setIsWatching] = (0,react.useState)(false);
    const isWatchingRef = (0,react.useRef)(false);
    // True once the current stream has delivered at least one message.
    const isLiveRef = (0,react.useRef)(false);
    // Timestamp of the last received event, used only by the bespoke
    // visibility/online handler below (staleThresholdMs's own periodic
    // watchdog inside useWatchStream already covers heartbeat-driven
    // liveness on a slower cadence).
    const lastActivityRef = (0,react.useRef)(Date.now());
    // Debounce timer for visibilitychange/online handlers
    const debounceTimerRef = (0,react.useRef)(null);
    // Initialize ConnectRPC client — uses HTTP for unary, WebSocket for streaming Watch* RPCs
    (0,react.useEffect)(()=>{
        const transport = (0,watch_ws_transport/* createSessionWatchTransport */.ZX)({
            baseUrl,
            interceptors: [
                (0,config/* createAuthInterceptor */._)(),
                (0,rpcTiming/* createRpcTimingInterceptor */.r)(analytics)
            ]
        });
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, transport);
    // analytics identity is stable (memoized in AnalyticsContextProvider); baseUrl is the only real dep
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        baseUrl
    ]);
    // List sessions with retry logic
    const listSessions = (0,react.useCallback)(async (listOptions)=>{
        if (!clientRef.current) return;
        dispatch((0,sessionsSlice/* setLoading */.r1)(true));
        dispatch((0,sessionsSlice/* setError */.Nh)(null));
        try {
            await refreshCoordinatorRef.current.request(()=>clientRef.current.listSessions({
                    category: listOptions === null || listOptions === void 0 ? void 0 : listOptions.category,
                    status: listOptions === null || listOptions === void 0 ? void 0 : listOptions.status,
                    includeArchived: listOptions === null || listOptions === void 0 ? void 0 : listOptions.includeArchived,
                    includeHidden: listOptions === null || listOptions === void 0 ? void 0 : listOptions.includeHidden
                }, {
                    timeoutMs: LIST_SESSIONS_TIMEOUT_MS
                }), (response)=>{
                dispatch((0,sessionsSlice/* setSessions */.YI)(response.sessions));
                dispatch((0,sessionsSlice/* setError */.Nh)(null)); // Clear any previous errors
                if (response.systemMemoryPct > 0) {
                    setSystemMemoryPct(response.systemMemoryPct);
                }
            });
        } catch (err) {
            const error = err instanceof Error ? err : new Error("Failed to list sessions");
            dispatch((0,sessionsSlice/* setError */.Nh)(error.message));
            console.error("Failed to list sessions:", error);
        } finally{
            // Known limitation (sdd:6-verify follow-up, not fixed here): this
            // clears as soon as THIS call's own request() settles, even if it
            // got coalesced behind a still-in-flight rerun whose data hasn't
            // landed yet — `loading` can briefly read false mid-refresh. A
            // correct fix needs a site-scoped "is my own work done" signal, not
            // the coordinator's shared busy state (which also reflects unrelated
            // background stream reconnects from sites #2/#3/#3b).
            dispatch((0,sessionsSlice/* setLoading */.r1)(false));
        }
    }, [
        dispatch
    ]);
    // Get single session
    const getSession = (0,react.useCallback)(async (id, options)=>{
        if (!clientRef.current) return null;
        try {
            const response = await clientRef.current.getSession({
                id
            });
            var _response_session;
            return (_response_session = response.session) !== null && _response_session !== void 0 ? _response_session : null;
        } catch (err) {
            if (options === null || options === void 0 ? void 0 : options.onFailure) {
                options.onFailure(err);
            } else {
                dispatch((0,sessionsSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to get session"));
            }
            return null;
        }
    }, [
        dispatch
    ]);
    // Create session
    const createSession = (0,react.useCallback)(async (request)=>{
        if (!clientRef.current) return null;
        dispatch((0,sessionsSlice/* setError */.Nh)(null));
        try {
            var _request_title, _request_path, _request_autoApprove, _request_createIfMissing, _request_autonomousMode, _request_permissionMode, _request_aliasName, _request_cliFlags, _request_extraArgs;
            const response = await clientRef.current.createSession({
                title: (_request_title = request.title) !== null && _request_title !== void 0 ? _request_title : "",
                path: (_request_path = request.path) !== null && _request_path !== void 0 ? _request_path : "",
                workingDir: request.workingDir,
                branch: request.branch,
                program: request.program,
                category: request.category,
                prompt: request.prompt,
                autoYes: request.autoYes,
                autoApprove: (_request_autoApprove = request.autoApprove) !== null && _request_autoApprove !== void 0 ? _request_autoApprove : false,
                existingWorktree: request.existingWorktree,
                sessionType: request.sessionType,
                createIfMissing: (_request_createIfMissing = request.createIfMissing) !== null && _request_createIfMissing !== void 0 ? _request_createIfMissing : false,
                initialPrompt: request.initialPrompt,
                autonomousMode: (_request_autonomousMode = request.autonomousMode) !== null && _request_autonomousMode !== void 0 ? _request_autonomousMode : false,
                // No default (unlike autonomousMode) -- an omitted remote must stay omitted on
                // the wire, not coerced to a zero-value RemoteTarget, so local session creation
                // is byte-identical to pre-change behavior (ADR-001: remote-as-orthogonal-flag).
                remote: request.remote,
                permissionMode: (_request_permissionMode = request.permissionMode) !== null && _request_permissionMode !== void 0 ? _request_permissionMode : "",
                aliasName: (_request_aliasName = request.aliasName) !== null && _request_aliasName !== void 0 ? _request_aliasName : "",
                cliFlags: (_request_cliFlags = request.cliFlags) !== null && _request_cliFlags !== void 0 ? _request_cliFlags : "",
                extraArgs: (_request_extraArgs = request.extraArgs) !== null && _request_extraArgs !== void 0 ? _request_extraArgs : [],
                restartFromSessionId: request.restartFromSessionId,
                confirmRestartWithLiveSource: request.confirmRestartWithLiveSource
            }, {
                timeoutMs: CREATE_SESSION_TIMEOUT_MS
            });
            // Add to store (with duplicate check handled by entity adapter upsertOne)
            if (response.session) {
                dispatch((0,sessionsSlice/* upsertSession */.ih)(response.session));
            }
            var _response_session;
            return (_response_session = response.session) !== null && _response_session !== void 0 ? _response_session : null;
        } catch (err) {
            const wrappedErr = err instanceof Error ? err : new Error("Failed to create session");
            // Deliberately NOT dispatch(setError(...)) here: `sessions.error` is what
            // PaneSplitRenderer's SessionListPaneBody checks to decide whether to render
            // the whole session list or replace it with a full-screen "Failed to Load
            // Sessions / Unable to connect to the server" state (see PaneSplitRenderer.tsx).
            // That's meant for list-load/watch-stream connectivity failures (listSessions'
            // and watchSessions' own catch blocks set it correctly). A createSession
            // rejection -- including an expected, synchronous validation failure like a
            // duplicate title -- is neither: the list loaded fine and the server is
            // reachable, it just refused this one request. Setting the same flag here
            // blanked the entire (already-populated) session list behind a misleading
            // connectivity error for as long as nothing else happened to clear it, which
            // is exactly what session-creation-async.spec.ts's "duplicate title keeps the
            // omnibar open with inline error" test was hitting: the omnibar's own
            // `omnibar-create-error` correctly showed the rejection, but the session card
            // it was asserting on had vanished behind this unrelated global error state.
            // The thrown error already reaches the caller (OmnibarContext/Omnibar.tsx),
            // which surfaces it via its own local, create-scoped error state -- no need
            // for a second, differently-scoped copy here.
            throw wrappedErr;
        }
    }, [
        dispatch
    ]);
    // Update session
    const updateSession = (0,react.useCallback)(async (id, updates)=>{
        if (!clientRef.current) return null;
        dispatch((0,sessionsSlice/* setError */.Nh)(null));
        try {
            var _updates_tags;
            const response = await clientRef.current.updateSession({
                id,
                status: updates.status,
                category: updates.category,
                note: updates.note,
                title: updates.title,
                program: updates.program,
                tags: (_updates_tags = updates.tags) !== null && _updates_tags !== void 0 ? _updates_tags : [],
                workingDir: updates.workingDir,
                rateLimitEnabled: updates.rateLimitEnabled,
                autonomousMode: updates.autonomousMode,
                autoApprove: updates.autoApprove,
                steerMessage: updates.steerMessage
            });
            // Update in store
            if (response.session) {
                dispatch((0,sessionsSlice/* upsertSession */.ih)(response.session));
            }
            var _response_session;
            return (_response_session = response.session) !== null && _response_session !== void 0 ? _response_session : null;
        } catch (err) {
            console.error("[useSessionService] updateSession failed:", err);
            dispatch((0,sessionsSlice/* setError */.Nh)((0,connectError/* getErrorMessage */.u)(err, "Failed to update session")));
            // Board drag-rejection reconciliation (SessionBoard's attemptColumnMove) needs the
            // ConnectRPC code to distinguish a transport failure from a business-rule rejection —
            // see sessionsSlice.ts's errorCode doc comment.
            if (err instanceof connect_error/* ConnectError */.T) {
                dispatch((0,sessionsSlice/* setErrorCode */.QA)(err.code));
            }
            return null;
        }
    }, [
        dispatch
    ]);
    // Delete session
    const deleteSession = (0,react.useCallback)(async function(id) {
        let force = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : false;
        if (!clientRef.current) return false;
        dispatch((0,sessionsSlice/* setError */.Nh)(null));
        try {
            const response = await clientRef.current.deleteSession({
                id,
                force
            });
            // Remove from store
            if (response.success) {
                dispatch((0,sessionsSlice/* removeSession */.dF)(id));
            }
            return response.success;
        } catch (err) {
            dispatch((0,sessionsSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to delete session"));
            return false;
        }
    }, [
        dispatch
    ]);
    // Pause session
    const pauseSession = (0,react.useCallback)(async (id)=>{
        return updateSession(id, {
            status: types_pb/* SessionStatus */.Ph.PAUSED
        });
    }, [
        updateSession
    ]);
    // Resume session with optional metadata updates (title, tags)
    const resumeSession = (0,react.useCallback)(async (id, updates)=>{
        return updateSession(id, {
            status: types_pb/* SessionStatus */.Ph.RUNNING,
            ...(updates === null || updates === void 0 ? void 0 : updates.title) ? {
                title: updates.title
            } : {},
            ...(updates === null || updates === void 0 ? void 0 : updates.tags) && updates.tags.length > 0 ? {
                tags: updates.tags
            } : {}
        });
    }, [
        updateSession
    ]);
    // Hibernate session (Active → Hibernated)
    const hibernateSession = (0,react.useCallback)(async (id)=>{
        if (!clientRef.current) return null;
        dispatch((0,sessionsSlice/* setError */.Nh)(null));
        try {
            const response = await clientRef.current.hibernateSession({
                id,
                reason: "manual"
            });
            if (response.session) dispatch((0,sessionsSlice/* upsertSession */.ih)(response.session));
            var _response_session;
            return (_response_session = response.session) !== null && _response_session !== void 0 ? _response_session : null;
        } catch (err) {
            console.error("[useSessionService] hibernateSession failed:", err);
            dispatch((0,sessionsSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to hibernate session"));
            return null;
        }
    }, [
        dispatch
    ]);
    // Resume a hibernated session (Hibernated → Active)
    const resumeHibernatedSession = (0,react.useCallback)(async (id)=>{
        if (!clientRef.current) return null;
        dispatch((0,sessionsSlice/* setError */.Nh)(null));
        try {
            const response = await clientRef.current.resumeHibernatedSession({
                id
            });
            if (response.session) dispatch((0,sessionsSlice/* upsertSession */.ih)(response.session));
            var _response_session;
            return (_response_session = response.session) !== null && _response_session !== void 0 ? _response_session : null;
        } catch (err) {
            console.error("[useSessionService] resumeHibernatedSession failed:", err);
            dispatch((0,sessionsSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to resume hibernated session"));
            if (err instanceof connect_error/* ConnectError */.T) {
                dispatch((0,sessionsSlice/* setErrorCode */.QA)(err.code));
            }
            return null;
        }
    }, [
        dispatch
    ]);
    // Resume a crashed session (Crashed → Active). Server-side resume of a dead
    // tmux pane detected by SessionHealthChecker — threads --resume automatically
    // when a conversation UUID is known, no manual copy/paste required.
    const resumeCrashedSession = (0,react.useCallback)(async (id)=>{
        if (!clientRef.current) return null;
        dispatch((0,sessionsSlice/* setError */.Nh)(null));
        try {
            const response = await clientRef.current.resumeCrashedSession({
                id
            });
            if (response.session) dispatch((0,sessionsSlice/* upsertSession */.ih)(response.session));
            var _response_session;
            return (_response_session = response.session) !== null && _response_session !== void 0 ? _response_session : null;
        } catch (err) {
            console.error("[useSessionService] resumeCrashedSession failed:", err);
            dispatch((0,sessionsSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to resume crashed session"));
            return null;
        }
    }, [
        dispatch
    ]);
    // Rename session
    const renameSession = (0,react.useCallback)(async (id, newTitle)=>{
        if (!clientRef.current) return false;
        dispatch((0,sessionsSlice/* setError */.Nh)(null));
        try {
            const response = await clientRef.current.renameSession({
                id,
                newTitle
            });
            // Update in store
            if (response.session) {
                dispatch((0,sessionsSlice/* upsertSession */.ih)(response.session));
            }
            return !!response.session;
        } catch (err) {
            dispatch((0,sessionsSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to rename session"));
            return false;
        }
    }, [
        dispatch
    ]);
    // Restart session
    const restartSession = (0,react.useCallback)(async (id)=>{
        if (!clientRef.current) return false;
        dispatch((0,sessionsSlice/* setError */.Nh)(null));
        try {
            const response = await clientRef.current.restartSession({
                id
            });
            // Update in store
            if (response.success && response.session) {
                dispatch((0,sessionsSlice/* upsertSession */.ih)(response.session));
            }
            return response.success;
        } catch (err) {
            dispatch((0,sessionsSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to restart session"));
            return false;
        }
    }, [
        dispatch
    ]);
    // Retry a session immediately, bypassing any pending backoff delay —
    // including from PERMANENTLY_FAILED (session-retry-backoff, AC6).
    const retrySession = (0,react.useCallback)(async (id)=>{
        if (!clientRef.current) return false;
        dispatch((0,sessionsSlice/* setError */.Nh)(null));
        try {
            const response = await clientRef.current.retrySession({
                id
            });
            if (response.success && response.session) {
                dispatch((0,sessionsSlice/* upsertSession */.ih)(response.session));
            }
            return response.success;
        } catch (err) {
            dispatch((0,sessionsSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to retry session"));
            return false;
        }
    }, [
        dispatch
    ]);
    // Cancel an in-progress (Creating) session's creation pipeline. Epic 5.4
    // (async-session-creation). Success means the instance was removed
    // server-side (session_service.go's CancelSessionCreation deletes it) --
    // the caller should remove the card from the list. A FailedPrecondition
    // means cancel lost the race with the pipeline's own terminal write (the
    // session is already Active/Failed); the normal watchSessions stream
    // event already carries that real status, so this just reports the loss
    // without touching the store -- no stale optimistic removal.
    const cancelSessionCreation = (0,react.useCallback)(async (id)=>{
        if (!clientRef.current) return {
            success: false,
            lostRace: false
        };
        dispatch((0,sessionsSlice/* setError */.Nh)(null));
        try {
            const response = await clientRef.current.cancelSessionCreation({
                id
            });
            if (response.success) {
                dispatch((0,sessionsSlice/* removeSession */.dF)(id));
            }
            return {
                success: response.success,
                lostRace: false
            };
        } catch (err) {
            if (err instanceof connect_error/* ConnectError */.T && err.code === code/* Code */.C.FailedPrecondition) {
                // Pipeline won the race -- leave the store alone, the stream's
                // own status update (Active/Failed) is already on its way.
                return {
                    success: false,
                    lostRace: true
                };
            }
            dispatch((0,sessionsSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to cancel session creation"));
            return {
                success: false,
                lostRace: false
            };
        }
    }, [
        dispatch
    ]);
    // Retry a Failed session creation in place. Epic 5.4. The RPC itself only
    // returns a bool -- the same instance's status flips Failed -> Creating
    // server-side and the existing watchSessions stream delivers that update,
    // so this wrapper doesn't need to (and must not) synthesize a session
    // update itself. A FailedPrecondition means the instance was no longer
    // Failed by the time the retry command ran (e.g. a concurrent retry already
    // won); treated as a no-op failure, not an error toast.
    const retrySessionCreation = (0,react.useCallback)(async (id)=>{
        if (!clientRef.current) return false;
        dispatch((0,sessionsSlice/* setError */.Nh)(null));
        try {
            const response = await clientRef.current.retrySessionCreation({
                id
            });
            return response.success;
        } catch (err) {
            if (err instanceof connect_error/* ConnectError */.T && err.code === code/* Code */.C.FailedPrecondition) {
                return false;
            }
            dispatch((0,sessionsSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to retry session creation"));
            return false;
        }
    }, [
        dispatch
    ]);
    // Clear the stored Claude conversation UUID so next resume starts fresh
    const clearConversationState = (0,react.useCallback)(async (id)=>{
        if (!clientRef.current) return false;
        dispatch((0,sessionsSlice/* setError */.Nh)(null));
        try {
            const response = await clientRef.current.clearConversationState({
                id
            });
            return response.success;
        } catch (err) {
            dispatch((0,sessionsSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to clear conversation state"));
            return false;
        }
    }, [
        dispatch
    ]);
    // Create checkpoint for a session
    const createCheckpoint = (0,react.useCallback)(async (sessionId, label)=>{
        if (!clientRef.current) return false;
        dispatch((0,sessionsSlice/* setError */.Nh)(null));
        try {
            await clientRef.current.createCheckpoint({
                sessionId,
                label
            });
            return true;
        } catch (err) {
            dispatch((0,sessionsSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to create checkpoint"));
            return false;
        }
    }, [
        dispatch
    ]);
    // List checkpoints for a session
    const listCheckpoints = (0,react.useCallback)(async (sessionId)=>{
        if (!clientRef.current) return [];
        try {
            const response = await clientRef.current.listCheckpoints({
                sessionId
            });
            return response.checkpoints;
        } catch (e) {
            return [];
        }
    }, []);
    // Fork a session from a checkpoint
    const forkSession = (0,react.useCallback)(async (sessionId, checkpointId, newTitle)=>{
        if (!clientRef.current) return null;
        dispatch((0,sessionsSlice/* setError */.Nh)(null));
        try {
            const response = await clientRef.current.forkSession({
                sessionId,
                checkpointId,
                newTitle
            });
            if (response.session) {
                dispatch((0,sessionsSlice/* upsertSession */.ih)(response.session));
            }
            var _response_session;
            return (_response_session = response.session) !== null && _response_session !== void 0 ? _response_session : null;
        } catch (err) {
            dispatch((0,sessionsSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to fork session"));
            return null;
        }
    }, [
        dispatch
    ]);
    // Acknowledge session (skip from review queue)
    const acknowledgeSession = (0,react.useCallback)(async (id)=>{
        if (!clientRef.current) return false;
        dispatch((0,sessionsSlice/* setError */.Nh)(null));
        try {
            const response = await clientRef.current.acknowledgeSession({
                id
            });
            return response.success;
        } catch (err) {
            dispatch((0,sessionsSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to acknowledge session"));
            return false;
        }
    }, [
        dispatch
    ]);
    // Run one-shot claude command for a session (S3)
    const runOneShot = (0,react.useCallback)(async (sessionId, prompt, timeoutSeconds)=>{
        if (!clientRef.current) return null;
        dispatch((0,sessionsSlice/* setError */.Nh)(null));
        try {
            const response = await clientRef.current.runOneShot({
                sessionId,
                prompt,
                timeoutSeconds: timeoutSeconds !== null && timeoutSeconds !== void 0 ? timeoutSeconds : 0
            });
            return response;
        } catch (err) {
            dispatch((0,sessionsSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to run one-shot"));
            return null;
        }
    }, [
        dispatch
    ]);
    // Fetch the pre-filled draft (title/body/base branch) for a session's PR (AC4)
    const draftPullRequest = (0,react.useCallback)(async (sessionId)=>{
        if (!clientRef.current) return null;
        dispatch((0,sessionsSlice/* setError */.Nh)(null));
        try {
            const response = await clientRef.current.draftPullRequest({
                sessionId
            });
            return response;
        } catch (err) {
            dispatch((0,sessionsSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to draft pull request"));
            return null;
        }
    }, [
        dispatch
    ]);
    // Create the pull request for a session (AC3/AC7)
    const createPullRequest = (0,react.useCallback)(async (req)=>{
        if (!clientRef.current) return null;
        dispatch((0,sessionsSlice/* setError */.Nh)(null));
        try {
            const response = await clientRef.current.createPullRequest({
                sessionId: req.sessionId,
                title: req.title,
                body: req.body,
                baseBranch: req.baseBranch
            });
            return response;
        } catch (err) {
            dispatch((0,sessionsSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to create pull request"));
            return null;
        }
    }, [
        dispatch
    ]);
    // List prompt history entries (S1)
    const listPromptHistory = (0,react.useCallback)(async (limit)=>{
        if (!clientRef.current) return [];
        try {
            const response = await clientRef.current.listPromptHistory({
                limit: limit !== null && limit !== void 0 ? limit : 20
            });
            return response.entries;
        } catch (e) {
            return [];
        }
    }, []);
    // Fire a workflow immediately (outside of cron schedule).
    const archiveSession = (0,react.useCallback)(async (id)=>{
        if (!clientRef.current) return false;
        try {
            await clientRef.current.archiveSession((0,create/* create */.v)(session_pb/* ArchiveSessionRequestSchema */.fMI, {
                sessionId: id
            }));
            return true;
        } catch (err) {
            dispatch((0,sessionsSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to archive session"));
            return false;
        }
    }, [
        dispatch
    ]);
    const unarchiveSession = (0,react.useCallback)(async (id)=>{
        if (!clientRef.current) return false;
        try {
            await clientRef.current.unarchiveSession((0,create/* create */.v)(session_pb/* UnarchiveSessionRequestSchema */.E2p, {
                sessionId: id
            }));
            return true;
        } catch (err) {
            dispatch((0,sessionsSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to unarchive session"));
            return false;
        }
    }, [
        dispatch
    ]);
    // Optimistic pin toggle: flips the store immediately, restores the exact
    // previous session object if the RPC fails.
    const setPinned = (0,react.useCallback)(async (id, pinned)=>{
        if (!clientRef.current) return false;
        const previous = sessionsRef.current.find((s)=>s.id === id);
        // New updatedAt so the store's unchanged-updatedAt dedup doesn't drop the optimistic write.
        if (previous) dispatch((0,sessionsSlice/* upsertSession */.ih)({
            ...previous,
            pinned,
            updatedAt: (0,timestamp/* timestampFromDate */.LI)(new Date())
        }));
        try {
            if (pinned) {
                await clientRef.current.pinSession((0,create/* create */.v)(session_pb/* PinSessionRequestSchema */.u70, {
                    sessionId: id
                }));
            } else {
                await clientRef.current.unpinSession((0,create/* create */.v)(session_pb/* UnpinSessionRequestSchema */.$yU, {
                    sessionId: id
                }));
            }
            return true;
        } catch (err) {
            if (previous) dispatch((0,sessionsSlice/* upsertSession */.ih)(previous));
            dispatch((0,sessionsSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to ".concat(pinned ? "pin" : "unpin", " session")));
            return false;
        }
    }, [
        dispatch
    ]);
    const pinSession = (0,react.useCallback)((id)=>setPinned(id, true), [
        setPinned
    ]);
    const unpinSession = (0,react.useCallback)((id)=>setPinned(id, false), [
        setPinned
    ]);
    const listSessionsByWorkflow = (0,react.useCallback)(async function(workflowId) {
        let includeArchived = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : true;
        if (!clientRef.current) return [];
        try {
            const req = (0,create/* create */.v)(session_pb/* ListSessionsRequestSchema */.ul_, {
                workflowId,
                includeArchived
            });
            const response = await clientRef.current.listSessions(req);
            var _response_sessions;
            return (_response_sessions = response.sessions) !== null && _response_sessions !== void 0 ? _response_sessions : [];
        } catch (err) {
            dispatch((0,sessionsSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to list workflow runs"));
            return [];
        }
    }, [
        dispatch
    ]);
    const runWorkflow = (0,react.useCallback)(async (request)=>{
        if (!clientRef.current) return null;
        try {
            var _request_arg;
            const req = (0,create/* create */.v)(session_pb/* RunWorkflowRequestSchema */.Fft, {
                id: request.id,
                arg: (_request_arg = request.arg) !== null && _request_arg !== void 0 ? _request_arg : ""
            });
            const response = await clientRef.current.runWorkflow(req);
            var _response_sessionId;
            return (_response_sessionId = response.sessionId) !== null && _response_sessionId !== void 0 ? _response_sessionId : null;
        } catch (err) {
            dispatch((0,sessionsSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to run workflow"));
            return null;
        }
    }, [
        dispatch
    ]);
    // Spawn a new shell attached to a session
    const spawnShell = (0,react.useCallback)(async (request)=>{
        if (!clientRef.current) return null;
        try {
            var _request_sessionId, _request_name, _request_command, _request_workingDir;
            const response = await clientRef.current.spawnShell({
                sessionId: (_request_sessionId = request.sessionId) !== null && _request_sessionId !== void 0 ? _request_sessionId : "",
                name: (_request_name = request.name) !== null && _request_name !== void 0 ? _request_name : "",
                command: (_request_command = request.command) !== null && _request_command !== void 0 ? _request_command : "",
                workingDir: (_request_workingDir = request.workingDir) !== null && _request_workingDir !== void 0 ? _request_workingDir : ""
            });
            var _response_shell;
            return (_response_shell = response.shell) !== null && _response_shell !== void 0 ? _response_shell : null;
        } catch (err) {
            dispatch((0,sessionsSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to spawn shell"));
            return null;
        }
    }, [
        dispatch
    ]);
    // Stop a running shell
    const stopShell = (0,react.useCallback)(async (sessionId, shellId)=>{
        if (!clientRef.current) return false;
        try {
            const response = await clientRef.current.stopShell({
                sessionId,
                shellId
            });
            return response.success;
        } catch (err) {
            dispatch((0,sessionsSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to stop shell"));
            return false;
        }
    }, [
        dispatch
    ]);
    // Restart a shell
    const restartShell = (0,react.useCallback)(async (sessionId, shellId)=>{
        if (!clientRef.current) return false;
        try {
            const response = await clientRef.current.restartShell({
                sessionId,
                shellId
            });
            return response.success;
        } catch (err) {
            dispatch((0,sessionsSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to restart shell"));
            return false;
        }
    }, [
        dispatch
    ]);
    // List all shells for a session
    const listShells = (0,react.useCallback)(async (sessionId)=>{
        if (!clientRef.current) return [];
        try {
            const response = await clientRef.current.listShells({
                sessionId
            });
            return response.shells;
        } catch (e) {
            return [];
        }
    }, []);
    // Delete a shell (stop + remove from storage)
    const deleteShell = (0,react.useCallback)(async (sessionId, shellId)=>{
        if (!clientRef.current) return false;
        try {
            const response = await clientRef.current.deleteShell({
                sessionId,
                shellId
            });
            return response.success;
        } catch (err) {
            dispatch((0,sessionsSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to delete shell"));
            return false;
        }
    }, [
        dispatch
    ]);
    // Handle session events from watch stream
    const handleSessionEvent = (0,react.useCallback)((event)=>{
        lastActivityRef.current = Date.now();
        // A new deploy_id means the server restarted and seq reset: start the cursor over rather than
        // reading the lower seq as an anomaly below.
        const redeployed = (0,deployId/* noteDeployId */.j)(lastDeployIdRef, event.deployId, "sessions");
        // Advance the sequence cursor so reconnects can request a targeted replay.
        const prevSeq = redeployed ? 0n : lastSeqRef.current;
        if (event.seq > prevSeq) {
            lastSeqRef.current = event.seq;
        }
        // Seq backwards-jump detection: indicates a server restart. No separate
        // full-resync flag needed -- subscribeSessions (above) already runs a
        // guarded ListSessions snapshot before every single (re)connect attempt.
        if (event.seq > 0n && event.seq < prevSeq) {
            console.warn("[reconnect] seq backwards-jump detected — resetting afterSeq to 0");
            lastSeqRef.current = 0n;
        }
        // Handle different event types based on oneof case
        switch(event.event.case){
            case "sessionCreated":
                {
                    const session = event.event.value.session;
                    if (!session) return;
                    // Entity adapter handles deduplication via upsertOne
                    dispatch((0,sessionsSlice/* upsertSession */.ih)(session));
                    break;
                }
            case "sessionUpdated":
                {
                    const session = event.event.value.session;
                    if (!session) return;
                    dispatch((0,sessionsSlice/* upsertSession */.ih)(session));
                    break;
                }
            case "sessionDeleted":
                {
                    var _onSessionDeletedRef_current;
                    const sessionId = event.event.value.sessionId;
                    dispatch((0,sessionsSlice/* removeSession */.dF)(sessionId));
                    dispatch((0,reviewQueueSlice/* removeItem */.Ai)(sessionId));
                    dispatch((0,sessionsSlice/* removeDetectedStatus */.pu)(sessionId));
                    (_onSessionDeletedRef_current = onSessionDeletedRef.current) === null || _onSessionDeletedRef_current === void 0 ? void 0 : _onSessionDeletedRef_current.call(onSessionDeletedRef, sessionId);
                    break;
                }
            case "notification":
                {
                    // Route notification events to the callback
                    if (onNotificationRef.current) {
                        onNotificationRef.current(event.event.value);
                    }
                    break;
                }
            case "approvalResponse":
                {
                    var _event_event_value_context;
                    // An approval was resolved on this device or another — remove the toast
                    // preemptively and refresh history to show the resolved badge.
                    const approvalId = (_event_event_value_context = event.event.value.context) !== null && _event_event_value_context !== void 0 ? _event_event_value_context : "";
                    var _event_event_value_sessionId;
                    const sessionId = (_event_event_value_sessionId = event.event.value.sessionId) !== null && _event_event_value_sessionId !== void 0 ? _event_event_value_sessionId : "";
                    if (event.event.value.approved && sessionId) {
                        dispatch((0,sessionsSlice/* removeDetectedStatus */.pu)(sessionId));
                    }
                    if (approvalId) {
                        var _onApprovalResponseRef_current;
                        (_onApprovalResponseRef_current = onApprovalResponseRef.current) === null || _onApprovalResponseRef_current === void 0 ? void 0 : _onApprovalResponseRef_current.call(onApprovalResponseRef, approvalId, sessionId);
                    }
                    break;
                }
            case "sessionAcknowledged":
                {
                    var _event_event_value_sessionId1;
                    const sessionId = (_event_event_value_sessionId1 = event.event.value.sessionId) !== null && _event_event_value_sessionId1 !== void 0 ? _event_event_value_sessionId1 : "";
                    if (sessionId) {
                        dispatch((0,sessionsSlice/* removeDetectedStatus */.pu)(sessionId));
                        dispatch((0,reviewQueueSlice/* removeItem */.Ai)(sessionId));
                    }
                    break;
                }
            case "remoteHealthChanged":
                {
                    // ssh-remote-workspaces Epic 6.2: a configured remote's SSH connection
                    // health transitioned (session/sshremote.RemoteHealthProber, pushed
                    // over this same WatchSessions stream -- no separate subscription or
                    // polling). Routed into remotesSlice so RemoteConnectionIndicator can
                    // read it via selectRemoteConnectionState.
                    const remoteHealth = event.event.value;
                    if (remoteHealth.remoteName) {
                        dispatch((0,remotesSlice/* remoteHealthChanged */.ng)({
                            remoteName: remoteHealth.remoteName,
                            state: remoteHealth.state,
                            previousState: remoteHealth.previousState
                        }));
                    }
                    break;
                }
        }
    }, [
        dispatch
    ]);
    // Subscribes to WatchSessions. Connection mechanics (backoff reconnect,
    // idle-staleness watchdog incl. tab-refocus/online, afterSeq bookkeeping)
    // live in the shared useWatchStream.ts; this closure supplies the
    // session-specific request shape plus the guarded ListSessions snapshot
    // flush that must precede every (re)connect attempt so a client that
    // missed events during a gap never renders stale data. Deliberately
    // ignores useWatchStream's own afterSeq parameter in favor of this hook's
    // own lastSeqRef, which handleSessionEvent also uses for backwards-jump
    // detection (see there).
    const subscribeSessions = (0,react.useCallback)(async function*(_afterSeq, signal) {
        var _watchOptionsRef_current, _watchOptionsRef_current1;
        await refreshCoordinatorRef.current.request(()=>{
            var _watchOptionsRef_current, _watchOptionsRef_current1;
            return clientRef.current.listSessions({
                category: (_watchOptionsRef_current = watchOptionsRef.current) === null || _watchOptionsRef_current === void 0 ? void 0 : _watchOptionsRef_current.categoryFilter,
                status: (_watchOptionsRef_current1 = watchOptionsRef.current) === null || _watchOptionsRef_current1 === void 0 ? void 0 : _watchOptionsRef_current1.statusFilter
            }, {
                timeoutMs: LIST_SESSIONS_TIMEOUT_MS
            });
        }, (response)=>{
            if (!signal.aborted) dispatch((0,sessionsSlice/* setSessions */.YI)(response.sessions));
        }, {
            guarded: true
        });
        yield* clientRef.current.watchSessions({
            categoryFilter: (_watchOptionsRef_current = watchOptionsRef.current) === null || _watchOptionsRef_current === void 0 ? void 0 : _watchOptionsRef_current.categoryFilter,
            statusFilter: (_watchOptionsRef_current1 = watchOptionsRef.current) === null || _watchOptionsRef_current1 === void 0 ? void 0 : _watchOptionsRef_current1.statusFilter,
            afterSeq: lastSeqRef.current
        }, {
            signal
        });
    }, [
        dispatch
    ]);
    const onWatchConnectionStateChange = (0,react.useCallback)((next)=>{
        isLiveRef.current = next === "live";
        switch(next){
            case "live":
                setReconnectAttemptCount(0);
                dispatch((0,sessionsSlice/* setConnectionState */.aA)("connected"));
                break;
            case "stale":
                dispatch((0,sessionsSlice/* setConnectionState */.aA)("stale"));
                break;
            case "reconnecting":
                var _onReconnectRef_current;
                setReconnectAttemptCount((n)=>n + 1);
                (_onReconnectRef_current = onReconnectRef.current) === null || _onReconnectRef_current === void 0 ? void 0 : _onReconnectRef_current.call(onReconnectRef);
                dispatch((0,sessionsSlice/* setConnectionState */.aA)(DISCONNECTED));
                break;
            case "exhausted":
                // Non-retriable failure (see isFatalError below) -- give up until
                // something calls watchSessions() again.
                dispatch((0,sessionsSlice/* setConnectionState */.aA)(DISCONNECTED));
                isWatchingRef.current = false;
                setIsWatching(false);
                break;
            case "connecting":
                break;
        }
    }, [
        dispatch
    ]);
    // A WS-bridge close code, or the equivalent ConnectError code on the
    // native transport (no ws-close-code header exists there) -- see
    // isNonRetriableConnectError. Reported as "exhausted" with no backoff
    // retry, unlike a transient network error.
    const isFatalError = (0,react.useCallback)((err)=>{
        if (!(0,backoff/* isNonRetriableConnectError */.Qi)(err)) return false;
        const wsCode = (0,backoff/* getWsCloseCode */.ku)(err);
        const reason = wsCode !== null ? "close code=".concat(wsCode) : "connect code=".concat(err.code);
        console.warn("[reconnect] stream=watch non-retriable ".concat(reason, ", stopping reconnect"));
        return true;
    }, []);
    const { reconnect: reconnectStream } = (0,useWatchStream/* useWatchStream */.t)({
        subscribe: subscribeSessions,
        onEvent: handleSessionEvent,
        isHeartbeat: (event)=>event.heartbeat,
        onConnectionStateChange: onWatchConnectionStateChange,
        isFatalError,
        staleThresholdMs: 30000,
        // Sessions registers its own visibility/online handler below (env-flag
        // gated, and active even before the first watchSessions() call) rather
        // than using useWatchStream's built-in one.
        enableVisibilityWatchdog: false,
        enabled: isWatching
    });
    // Watch sessions for real-time updates with automatic reconnect on failure.
    // A first call starts the (otherwise disabled) stream; a call while
    // already watching forces an immediate fresh reconnect with the latest
    // options (e.g. the manual retry button, or the staleness backstop).
    const watchSessions = (0,react.useCallback)((watchOptions)=>{
        if (!clientRef.current) return;
        watchOptionsRef.current = watchOptions;
        if (isWatchingRef.current) {
            reconnectStream();
        } else {
            isWatchingRef.current = true;
            setIsWatching(true);
        }
    }, [
        reconnectStream
    ]);
    // Stop watching sessions
    const stopWatching = (0,react.useCallback)(()=>{
        isWatchingRef.current = false;
        setIsWatching(false);
        dispatch((0,sessionsSlice/* setConnectionState */.aA)(DISCONNECTED));
    }, [
        dispatch
    ]);
    // Auto-watch on mount if enabled and authenticated
    (0,react.useEffect)(()=>{
        if (!enabled) return;
        if (autoWatch) {
            watchSessions();
        }
        return ()=>{
            stopWatching();
        };
    }, [
        enabled,
        autoWatch,
        watchSessions,
        stopWatching
    ]);
    // Browser lifecycle listeners: reconnect on tab visibility restore or
    // network online. Behind NEXT_PUBLIC_RECONNECT_V2 and, unlike every other
    // watcher's shared useWatchStream-driven watchdog, active even before the
    // first watchSessions() call (in which case it starts watching, rather
    // than reconnecting an existing stream).
    const handleVisibilityOrOnline = (0,react.useCallback)((ev)=>{
        if (document.visibilityState !== "visible" && ev.type !== "online") return;
        if (debounceTimerRef.current) clearTimeout(debounceTimerRef.current);
        debounceTimerRef.current = setTimeout(()=>{
            debounceTimerRef.current = null;
            if (!isWatchingRef.current) {
                watchSessions(watchOptionsRef.current);
                return;
            }
            const stale = Date.now() - lastActivityRef.current > 15000;
            if (!isLiveRef.current || stale) {
                if (stale) dispatch((0,sessionsSlice/* setConnectionState */.aA)("stale"));
                reconnectStream();
            }
        }, 200);
    }, [
        dispatch,
        reconnectStream,
        watchSessions
    ]);
    (0,react.useEffect)(()=>{
        if (!enabled || process.env.NEXT_PUBLIC_RECONNECT_V2 !== "true") return;
        document.addEventListener("visibilitychange", handleVisibilityOrOnline);
        window.addEventListener("online", handleVisibilityOrOnline);
        return ()=>{
            if (debounceTimerRef.current) clearTimeout(debounceTimerRef.current);
            document.removeEventListener("visibilitychange", handleVisibilityOrOnline);
            window.removeEventListener("online", handleVisibilityOrOnline);
        };
    }, [
        enabled,
        handleVisibilityOrOnline
    ]);
    // Initial load — only for the watching instance (autoWatch: true).
    // Non-watching callers (useSessionActions, OmnibarContext, etc.) should read
    // from Redux directly; firing listSessions() from every caller causes N × 5
    // synchronous dispatches on mount that saturate the React render queue.
    (0,react.useEffect)(()=>{
        if (!enabled || !autoWatch) return;
        listSessions();
    }, [
        enabled,
        autoWatch,
        listSessions
    ]);
    // Convert error string back to Error object for backward compatibility
    const error = (0,react.useMemo)(()=>errorStr ? new Error(errorStr) : null, [
        errorStr
    ]);
    const connectionState = (0,store/* useAppSelector */.GV)(sessionsSlice/* selectConnectionState */.mN);
    const getTerminalSnapshot = (0,react.useCallback)(async function(sessionId) {
        let lastNLines = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : 50;
        if (!clientRef.current) return "";
        // Deliberately not caught here: a fetch failure must propagate to the
        // caller (SessionMonitor) so it can render a distinct "failed to load"
        // state instead of silently rendering identically to real emptiness.
        const resp = await clientRef.current.getTerminalSnapshot({
            sessionId,
            lastNLines
        });
        var _resp_content;
        return (_resp_content = resp.content) !== null && _resp_content !== void 0 ? _resp_content : "";
    }, []);
    const writeToSession = (0,react.useCallback)(async function(sessionId, input) {
        let pressEnter = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : true;
        if (!clientRef.current) return false;
        try {
            const resp = await clientRef.current.writeToSession({
                sessionId,
                input,
                pressEnter
            });
            var _resp_success;
            return (_resp_success = resp.success) !== null && _resp_success !== void 0 ? _resp_success : false;
        } catch (e) {
            return false;
        }
    }, []);
    const getConversationMessages = (0,react.useCallback)(async function(sessionId) {
        let limit = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : 30;
        if (!clientRef.current) return [];
        // See getTerminalSnapshot above: fetch failures propagate to the caller
        // instead of resolving to an empty array indistinguishable from "no
        // conversation history yet".
        const resp = await clientRef.current.getClaudeHistoryMessages({
            id: sessionId,
            limit,
            tail: true
        });
        var _resp_messages;
        return ((_resp_messages = resp.messages) !== null && _resp_messages !== void 0 ? _resp_messages : []).map((m)=>({
                role: m.role,
                content: m.content,
                timestamp: m.timestamp ? new Date(Number(m.timestamp.seconds) * 1000).toISOString() : undefined,
                model: m.model
            }));
    }, []);
    return {
        sessions,
        loading,
        hasLoadedOnce,
        error,
        connectionState,
        systemMemoryPct,
        reconnectAttemptCount,
        listSessions,
        getSession,
        createSession,
        updateSession,
        deleteSession,
        pauseSession,
        resumeSession,
        hibernateSession,
        resumeHibernatedSession,
        resumeCrashedSession,
        renameSession,
        restartSession,
        retrySession,
        cancelSessionCreation,
        retrySessionCreation,
        clearConversationState,
        acknowledgeSession,
        createCheckpoint,
        listCheckpoints,
        forkSession,
        runOneShot,
        draftPullRequest,
        createPullRequest,
        listPromptHistory,
        watchSessions,
        stopWatching,
        archiveSession,
        unarchiveSession,
        pinSession,
        unpinSession,
        listSessionsByWorkflow,
        runWorkflow,
        spawnShell,
        stopShell,
        restartShell,
        listShells,
        deleteShell,
        getTerminalSnapshot,
        writeToSession,
        getConversationMessages
    };
}


/***/ }),

/***/ 77665:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   r: () => (/* binding */ createRpcTimingInterceptor)
/* harmony export */ });
/* harmony import */ var _opentelemetry_api__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(82099);

/**
 * Network-queueing-delay attribute name set on the active OTel span (see
 * getNetworkQueueDelayMs below). Named to sort next to the standard
 * `http.*`/`url.*` fetch-instrumentation attributes in a trace viewer.
 */ const NETWORK_QUEUE_TIME_ATTRIBUTE = "network.queue_time_ms";
/**
 * Returns a proxy for how long this call's underlying fetch spent queued or
 * negotiating a connection before the browser actually put bytes on the
 * wire — the gap between the Resource Timing API's `fetchStart` and
 * `requestStart` for the matching entry. On an HTTP/1.1 origin capped at 6
 * concurrent connections per browser (this app, prior to the WS-bridge fix
 * for the Watch-family RPCs and the terminal stream — see
 * docs/how-to/enable-opentelemetry.md's "Browser network queueing delay"
 * section), a request that has to wait for a free connection shows a large
 * gap here even though the server never saw it start late. Distinguishing
 * that from genuine server-side latency was previously only possible by
 * hand-querying Tempo and noticing an RPC had *no* server-side span at all.
 *
 * Matches by exact URL (same-origin, so ConnectRPC's per-method absolute
 * URL is unique enough in practice) and picks the most recently recorded
 * matching entry, since a prior call to the same RPC method would have left
 * an earlier, already-superseded entry in the resource timing buffer.
 * Returns undefined if the Resource Timing API is unavailable (non-browser
 * test environment) or no matching/complete entry is found.
 */ function getNetworkQueueDelayMs(url) {
    if (typeof performance === "undefined" || typeof performance.getEntriesByType !== "function") {
        return undefined;
    }
    const entries = performance.getEntriesByType("resource");
    for(let i = entries.length - 1; i >= 0; i--){
        const entry = entries[i];
        if (entry.name !== url) {
            continue;
        }
        if (entry.requestStart > 0 && entry.fetchStart > 0) {
            return entry.requestStart - entry.fetchStart;
        }
        return undefined;
    }
    return undefined;
}
/**
 * ConnectRPC interceptor that records timing for every unary RPC call.
 *
 * Each call emits a Performance API entry:
 *   performance.mark(`rpc:<MethodName>:start`)
 *   performance.measure(`rpc:<MethodName>`, start, end)
 *
 * Entries are visible in Chrome DevTools > Performance > Timings track and
 * accessible via window.performance.getEntriesByType('measure').
 *
 * Attributes recorded in the measure detail:
 *   { method, url, ok, durationMs, queueDelayMs }
 *
 * queueDelayMs (see getNetworkQueueDelayMs) is also set as the
 * `network.queue_time_ms` attribute on the currently active OTel span —
 * normally the fetch span web-app/src/lib/telemetry/otel-init.ts's
 * FetchInstrumentation created for this exact call, which stays active
 * through the fetch promise chain via ZoneContextManager — so a queued
 * request is visibly distinguishable from a slow server-side one directly
 * in Tempo/Grafana. A no-op (nothing thrown) when OTel tracing isn't
 * enabled, since trace.getActiveSpan() returns undefined in that case.
 *
 * When an optional analytics provider is supplied, each RPC call also enqueues
 * an analytics event with category "rpc" for latency tracking.
 */ function createRpcTimingInterceptor(analytics) {
    return (next)=>async (req)=>{
            const method = req.method.name;
            const startMark = "rpc:".concat(method, ":start");
            if (typeof performance !== "undefined") {
                performance.mark(startMark);
            }
            const wallStart = Date.now();
            let ok = true;
            try {
                return await next(req);
            } catch (err) {
                ok = false;
                throw err;
            } finally{
                const durationMs = Date.now() - wallStart;
                const queueDelayMs = getNetworkQueueDelayMs(req.url);
                if (typeof performance !== "undefined") {
                    try {
                        performance.measure("rpc:".concat(method), {
                            start: startMark,
                            detail: {
                                method,
                                url: req.url,
                                ok,
                                durationMs,
                                queueDelayMs
                            }
                        });
                    } catch (e) {
                    // start mark may be missing in some edge cases; ignore
                    }
                }
                if (queueDelayMs !== undefined) {
                    var _trace_getActiveSpan;
                    (_trace_getActiveSpan = _opentelemetry_api__WEBPACK_IMPORTED_MODULE_0__/* .trace */ .u.getActiveSpan()) === null || _trace_getActiveSpan === void 0 ? void 0 : _trace_getActiveSpan.setAttribute(NETWORK_QUEUE_TIME_ATTRIBUTE, queueDelayMs);
                }
                if (false) {}
                analytics === null || analytics === void 0 ? void 0 : analytics.track({
                    name: "rpc.".concat(method),
                    category: "rpc",
                    durationMs,
                    labels: {
                        method,
                        ok: String(ok),
                        ...queueDelayMs !== undefined ? {
                            networkQueueDelayMs: String(queueDelayMs)
                        } : {}
                    }
                });
            }
        };
}


/***/ })

}]);
//# sourceMappingURL=3117-4603df41e25f6c04.js.map