(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[134],{

/***/ 4905:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 37419));


/***/ }),

/***/ 6470:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 7886:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 12646:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 20882:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   O5: () => (/* binding */ getDateGroup),
/* harmony export */   Yq: () => (/* binding */ formatDate),
/* harmony export */   ak: () => (/* binding */ formatTimeAgo),
/* harmony export */   uj: () => (/* binding */ protoTimestampToDate),
/* harmony export */   wZ: () => (/* binding */ isWithinDateFilter)
/* harmony export */ });
/* unused harmony export truncateMiddle */
/**
 * protoTimestampToDate converts a protobuf Timestamp to a Date, including the
 * sub-second nanos component (unlike this file's other helpers, which only use
 * `seconds`). Shared by TriggersPanel.tsx and TriggerExecutionHistory.tsx, which
 * previously carried two divergent inline copies of this arithmetic — one returning a
 * Date, the other returning an ISO string. Returns Date so callers format as needed
 * (toLocaleString, relative-time, etc.) rather than baking in one representation.
 */ const protoTimestampToDate = (timestamp)=>{
    if (!timestamp) return null;
    const ms = Number(timestamp.seconds) * 1000 + Math.floor(timestamp.nanos / 1e6);
    return new Date(ms);
};
const formatTimeAgo = (timestamp)=>{
    if (!timestamp) return "N/A";
    const now = Date.now();
    const date = new Date(Number(timestamp.seconds) * 1000);
    const seconds = Math.floor((now - date.getTime()) / 1000);
    if (seconds < 60) return "just now";
    if (seconds < 3600) return "".concat(Math.floor(seconds / 60), "m ago");
    if (seconds < 86400) return "".concat(Math.floor(seconds / 3600), "h ago");
    if (seconds < 604800) return "".concat(Math.floor(seconds / 86400), "d ago");
    return date.toLocaleDateString();
};
const formatDate = (timestamp)=>{
    if (!timestamp) return "N/A";
    const date = new Date(Number(timestamp.seconds) * 1000);
    return date.toLocaleString();
};
const truncateMiddle = (str, maxLength)=>{
    if (str.length <= maxLength) return str;
    const ellipsis = "...";
    const charsToShow = maxLength - ellipsis.length;
    const frontChars = Math.ceil(charsToShow / 2);
    const backChars = Math.floor(charsToShow / 2);
    return str.substring(0, frontChars) + ellipsis + str.substring(str.length - backChars);
};
const getDateGroup = (timestamp)=>{
    if (!timestamp) return "Unknown";
    const date = new Date(Number(timestamp.seconds) * 1000);
    const now = new Date();
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    const yesterday = new Date(today.getTime() - 86400000);
    const weekAgo = new Date(today.getTime() - 7 * 86400000);
    const monthAgo = new Date(today.getTime() - 30 * 86400000);
    if (date >= today) return "Today";
    if (date >= yesterday) return "Yesterday";
    if (date >= weekAgo) return "This Week";
    if (date >= monthAgo) return "This Month";
    return "Older";
};
const isWithinDateFilter = (timestamp, filter)=>{
    if (filter === "all" || !timestamp) return true;
    const date = new Date(Number(timestamp.seconds) * 1000);
    const now = new Date();
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    switch(filter){
        case "today":
            return date >= today;
        case "week":
            return date >= new Date(today.getTime() - 7 * 86400000);
        case "month":
            return date >= new Date(today.getTime() - 30 * 86400000);
        default:
            return true;
    }
};


/***/ }),

/***/ 32107:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
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

"use strict";
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

/***/ 37419:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  "default": () => (/* binding */ TriggersPage)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/connect-error.js
var connect_error = __webpack_require__(45640);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/code.js
var code = __webpack_require__(16294);
// EXTERNAL MODULE: ./src/lib/hooks/useWorkflows.ts + 1 modules
var useWorkflows = __webpack_require__(79623);
// EXTERNAL MODULE: ./src/lib/utils/timestamp.ts
var timestamp = __webpack_require__(20882);
// EXTERNAL MODULE: ./src/components/ui/Modal.tsx + 1 modules
var Modal = __webpack_require__(79898);
// EXTERNAL MODULE: ./src/components/ui/RepoPathInput.tsx + 2 modules
var RepoPathInput = __webpack_require__(34609);
// EXTERNAL MODULE: ./src/components/sessions/ApprovalRulesPanel.css.ts
var ApprovalRulesPanel_css = __webpack_require__(696);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Fa11y.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll85amk4YnAwIHsKICBwb3NpdGlvbjogYWJzb2x1dGU7CiAgd2lkdGg6IDFweDsKICBoZWlnaHQ6IDFweDsKICBwYWRkaW5nOiAwOwogIG1hcmdpbjogLTFweDsKICBvdmVyZmxvdzogaGlkZGVuOwogIGNsaXAtcGF0aDogaW5zZXQoNTAlKTsKICB3aGl0ZS1zcGFjZTogbm93cmFwOwogIGJvcmRlcjogMDsKfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Fa11y_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll85amk4YnAwIHsKICBwb3NpdGlvbjogYWJzb2x1dGU7CiAgd2lkdGg6IDFweDsKICBoZWlnaHQ6IDFweDsKICBwYWRkaW5nOiAwOwogIG1hcmdpbjogLTFweDsKICBvdmVyZmxvdzogaGlkZGVuOwogIGNsaXAtcGF0aDogaW5zZXQoNTAlKTsKICB3aGl0ZS1zcGFjZTogbm93cmFwOwogIGJvcmRlcjogMDsKfQ_22_7D = __webpack_require__(47439);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FTriggerFormModal.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA71W25LaMAx95yv0WB7MxOGy4P2YHROL4FnHdh0HsnT23ztxAo1DuE2nfQFGlqUjnWOJ2QctN0Xh8gR-TQCELK3iXwx2Cuv3CQBXMtdEeixKBhlqj64x59wyOHD3g5APusFT6k7padqc7Iz2pJQnZEDntn6ffE9m5xw05HDYHh_QeZlxdbm144VUX1dxq2kUJA1BCu5yqYk3VzjmNPafB3_LhZA6Z5BEh4vxqkN5axt-NzZydI2l-YyuL8P1rXECHQNqayiNkmKACENjWi_iuJBVeYl-wbW2NdBFl3LYw850RJnvPYNlkjS2rHKlcQyskWdetjz7zJ2ptGDgHdel5Q61D95GNc4xNBqgBU_ppdEMuFKQzOiyBOQlNqeF1GTfZV4kA0qXbG8O6No-9JLHadYxJasH7kW_YaO4W4_Ro59xrrenKNo9QxFNexy1Agx6GhFQEI2QDrO2q5lRVaEv0qLDNq4Dyj7vdIT3t5Z3hd6jI6XlWSvpWbLCIhCJtSeBzZ1xBYPKWnRZx-Not9JpVGICiwGyzS1k43pq4_VfZzoIyF8KeCMegJIaL7Kks0WUYzv-rB-wshogzR4Phxvj8d7MEH_qvzPw4g6lAyWuGiEmrfH2M6Kip-oXx9PqDidJuHA0TpCtQ_7JIHwRrtS5eAYUKKRJh7EZIUcp_H44f_G_NWP7b3rR6vOq5F6Juydm498BizuRXu-Q9M4OuU3wyHrpr4J5F_Xe8ui34alFQQebIn9pWNTTh5Nh_1LAwV8P-fos_P4NyvkBbmEJAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FTriggerFormModal_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA71W25LaMAx95yv0WB7MxOGy4P2YHROL4FnHdh0HsnT23ztxAo1DuE2nfQFGlqUjnWOJ2QctN0Xh8gR_TQCELK3iXwx2Cuv3CQBXMtdEeixKBhlqj64x59wyOHD3g5APusFT6k7padqc7Iz2pJQnZEDntn6ffE9m5xw05HDYHh_QeZlxdbm144VUX1dxq2kUJA1BCu5yqYk3VzjmNPafB3_LhZA6Z5BEh4vxqkN5axt_NzZydI2l_YyuL8P1rXECHQNqayiNkmKACENjWi_iuJBVeYl_wbW2NdBFl3LYw850RJnvPYNlkjS2rHKlcQyskWdetjz7zJ2ptGDgHdel5Q61D95GNc4xNBqgBU_ppdEMuFKQzOiyBOQlNqeF1GTfZV4kA0qXbG8O6No_9JLHadYxJasH7kW_YaO4W4_Ro59xrrenKNo9QxFNexy1Agx6GhFQEI2QDrO2q5lRVaEv0qLDNq4Dyj7vdIT3t5Z3hd6jI6XlWSvpWbLCIhCJtSeBzZ1xBYPKWnRZx_Not9JpVGICiwGyzS1k43pq4_VfZzoIyF8KeCMegJIaL7Kks0WUYzv_rB_wshogzR4Phxvj8d7MEH_qvzPw4g6lAyWuGiEmrfH2M6Kip_oXx9PqDidJuHA0TpCtQ_7JIHwRrtS5eAYUKKRJh7EZIUcp_H44f_G_NWP7b3rR6vOq5F6Juydm498BizuRXu_Q9M4OuU3wyHrpr4J5F_Xe8ui34alFQQebIn9pWNTTh5Nh_1LAwV8P_fos_P4NyvkBbmEJAAA_22_7D = __webpack_require__(55913);
;// ./src/components/sessions/TriggerFormModal.css.ts



var checkboxLabel = '_1s9mmrg0';
var fieldError = '_1s9mmrg9';
var fieldHint = '_1s9mmrga';
var fieldset = '_1s9mmrg7';
var formActionsSpaced = '_1s9mmrg2';
var legend = '_1s9mmrg8';
var promptTextarea = '_1s9mmrg1';
var secretBox = '_1s9mmrgb';
var secretBoxLegend = '_1s9mmrg3';
var secretButton = '_1s9mmrgf';
var secretCopiedNotice = '_1s9mmrgh';
var secretCopyErrorNotice = '_1s9mmrgi';
var secretMasked = '_1s9mmrge';
var secretRow = '_1s9mmrgc';
var secretValue = '_1s9mmrgd';
var secretWarning = '_1s9mmrgg';
var typeOption = '_1s9mmrg5';
var typeOptionActive = '_1s9mmrg6';
var typeSelector = '_1s9mmrg4';
var visuallyHidden = '_9ji8bp0';
;// ./src/components/sessions/TriggerFormModal.tsx
/* __next_internal_client_entry_do_not_use__ TriggerFormModal auto */ 
// +feature: trigger-form-modal





const TRIGGER_TYPES = [
    {
        value: "github_push",
        label: "GitHub Push"
    },
    {
        value: "cron",
        label: "Schedule (cron)"
    },
    {
        value: "webhook",
        label: "Webhook"
    }
];
const EMPTY = {
    slug: "",
    name: "",
    description: "",
    command: "",
    targetDirectory: "",
    cronEnabled: true,
    enabled: true,
    triggerType: "github_push",
    githubRepo: "",
    githubBranch: "",
    cronExpression: "",
    webhookSlug: "",
    eventFilter: "",
    labelFilter: "",
    promptTemplate: ""
};
function protoToFormData(w) {
    return {
        slug: w.slug,
        name: w.name,
        description: w.description,
        command: w.command,
        targetDirectory: w.targetDirectory,
        cronEnabled: w.cronEnabled,
        enabled: w.enabled,
        triggerType: w.triggerType || "github_push",
        githubRepo: w.githubRepo,
        githubBranch: w.githubBranch,
        cronExpression: w.cronExpression,
        webhookSlug: w.webhookSlug,
        eventFilter: w.eventFilter,
        labelFilter: w.labelFilter,
        promptTemplate: w.promptTemplate,
        expectedUpdatedAt: w.updatedAt
    };
}
/**
 * Maps a backend connect.CodeInvalidArgument rawMessage (see
 * server/services/workflow_service.go's errors.New/fmt.Errorf call sites) to the
 * specific form field it concerns, so the error renders inline near that field
 * instead of only as a generic top-of-form banner (Task 7.1.2c).
 */ function mapErrorToField(message) {
    const m = message.toLowerCase();
    if (m.includes("slug")) return "slug";
    if (m.includes("name is required")) return "name";
    if (m.includes("target_directory")) return "targetDirectory";
    if (m.includes("command is required") || m.includes("command required")) return "command";
    if (m.includes("cron_expression") || m.includes("cron expression")) return "cronExpression";
    if (m.includes("cron_enabled")) return "triggerType";
    if (m.includes("webhook_slug")) return "webhookSlug";
    if (m.includes("github_repo")) return "githubRepo";
    if (m.includes("prompt_template")) return "promptTemplate";
    return null;
}
const COMMON_FIELDS = new Set([
    "slug",
    "name",
    "targetDirectory",
    "command",
    "triggerType"
]);
const TYPE_SPECIFIC_FIELDS = {
    github_push: [
        "githubRepo",
        "githubBranch",
        "promptTemplate"
    ],
    cron: [
        "cronExpression"
    ],
    webhook: [
        "webhookSlug",
        "eventFilter",
        "labelFilter",
        "promptTemplate"
    ]
};
/** Whether `field`'s input is actually rendered for the currently selected trigger type. */ function isFieldVisible(field, triggerType) {
    return COMMON_FIELDS.has(field) || TYPE_SPECIFIC_FIELDS[triggerType].includes(field);
}
/**
 * Generates a cryptographically-random 32-byte hex secret via the Web Crypto API.
 * Returns null when `crypto.getRandomValues` isn't available — this is unreachable
 * in any evergreen browser, but a security-critical secret must never silently fall
 * back to `Math.random()` (not cryptographically secure) when it isn't. Callers
 * must disable secret generation and surface a message instead of calling this in
 * a loop or otherwise papering over a null result.
 */ function generateClientSecret() {
    if (typeof crypto === "undefined" || typeof crypto.getRandomValues !== "function") {
        return null;
    }
    const bytes = new Uint8Array(32);
    crypto.getRandomValues(bytes);
    return Array.from(bytes, (b)=>b.toString(16).padStart(2, "0")).join("");
}
function TriggerFormModal(param) {
    let { open, editTrigger, onSave, onClose } = param;
    const isEdit = !!editTrigger;
    const [formData, setFormData] = (0,react.useState)(EMPTY);
    const [saving, setSaving] = (0,react.useState)(false);
    const [formErrorMsg, setFormErrorMsg] = (0,react.useState)(null);
    const [fieldErrors, setFieldErrors] = (0,react.useState)({});
    const [liveMessage, setLiveMessage] = (0,react.useState)("");
    // Secret UI state. generatedSecret holds the plaintext value shown once (create) or
    // during an in-progress rotation (edit) — it is sent to the backend via
    // WorkflowFormData.webhookSecret on submit and never re-read from the server (see
    // SecretField's doc comment for the never-round-trip-a-real-secret convention).
    // rotatingSecret gates edit mode between the masked "unchanged" placeholder and the
    // generate/copy UI — editing a trigger defaults to leaving the stored secret alone.
    const [generatedSecret, setGeneratedSecret] = (0,react.useState)(null);
    const [secretCopied, setSecretCopied] = (0,react.useState)(false);
    const [secretCopyError, setSecretCopyError] = (0,react.useState)(false);
    const [secretGenerationUnsupported, setSecretGenerationUnsupported] = (0,react.useState)(false);
    const [rotatingSecret, setRotatingSecret] = (0,react.useState)(false);
    (0,react.useEffect)(()=>{
        if (!open) return;
        setFormData(editTrigger ? protoToFormData(editTrigger) : EMPTY);
        setFormErrorMsg(null);
        setFieldErrors({});
        setGeneratedSecret(null);
        setSecretCopied(false);
        setSecretCopyError(false);
        setSecretGenerationUnsupported(false);
        setRotatingSecret(false);
    }, [
        open,
        editTrigger
    ]);
    function handleGenerateSecret() {
        const secret = generateClientSecret();
        if (secret === null) {
            setSecretGenerationUnsupported(true);
            return;
        }
        setGeneratedSecret(secret);
        setSecretCopied(false);
        setSecretCopyError(false);
        setFieldErrors((prev)=>({
                ...prev,
                secret: undefined
            }));
    }
    async function handleCopySecret(value) {
        try {
            var _navigator_clipboard;
            if (!((_navigator_clipboard = navigator.clipboard) === null || _navigator_clipboard === void 0 ? void 0 : _navigator_clipboard.writeText)) {
                throw new Error("Clipboard API unavailable");
            }
            await navigator.clipboard.writeText(value);
            setSecretCopied(true);
            setSecretCopyError(false);
        } catch (e) {
            // Clipboard write failed (denied permission, insecure context, unfocused
            // window, etc.) — never report success for a value shown exactly once.
            setSecretCopied(false);
            setSecretCopyError(true);
        }
    }
    function setField(key, value) {
        setFormData((prev)=>({
                ...prev,
                [key]: value
            }));
        setFieldErrors((prev)=>({
                ...prev,
                [key]: undefined
            }));
    }
    function handleTypeChange(next) {
        // Clear the other types' fields client-side on switch (Task 7.1.2a) — closes the
        // same footgun the backend's Task 1.1.1e validation closes server-side (a row that
        // registers as more than one trigger mechanism at once).
        setFormData((prev)=>({
                ...prev,
                triggerType: next,
                githubRepo: next === "github_push" ? prev.githubRepo : "",
                githubBranch: next === "github_push" ? prev.githubBranch : "",
                cronExpression: next === "cron" ? prev.cronExpression : "",
                webhookSlug: next === "webhook" ? prev.webhookSlug : "",
                eventFilter: next === "webhook" ? prev.eventFilter : "",
                labelFilter: next === "webhook" ? prev.labelFilter : "",
                promptTemplate: next === "cron" ? "" : prev.promptTemplate
            }));
        setFieldErrors({});
    }
    function validateClientSide() {
        var _formData_cronExpression, _formData_githubRepo, _formData_webhookSlug;
        const errs = {};
        if (!formData.slug.trim()) errs.slug = "Slug is required.";
        if (!formData.name.trim()) errs.name = "Name is required.";
        if (!formData.command.trim()) errs.command = "Command / prompt is required.";
        if (!formData.targetDirectory.trim()) errs.targetDirectory = "Target directory is required.";
        if (formData.triggerType === "cron" && !((_formData_cronExpression = formData.cronExpression) === null || _formData_cronExpression === void 0 ? void 0 : _formData_cronExpression.trim())) {
            errs.cronExpression = "Cron expression is required for a schedule trigger.";
        }
        if (formData.triggerType === "github_push" && !((_formData_githubRepo = formData.githubRepo) === null || _formData_githubRepo === void 0 ? void 0 : _formData_githubRepo.trim())) {
            errs.githubRepo = "Repository (owner/repo) is required.";
        }
        if (formData.triggerType === "webhook" && !((_formData_webhookSlug = formData.webhookSlug) === null || _formData_webhookSlug === void 0 ? void 0 : _formData_webhookSlug.trim())) {
            errs.webhookSlug = "Webhook slug is required.";
        }
        // Rotate mode entered but no new secret generated yet: webhookSecret would end
        // up "" on submit, which the wire contract treats as "no change" — safe, but
        // silently not what the user asked for by clicking "Rotate secret".
        if (isEdit && rotatingSecret && !generatedSecret && (formData.triggerType === "webhook" || formData.triggerType === "github_push")) {
            errs.secret = "Generate a secret before saving.";
        }
        return errs;
    }
    async function handleSubmit(e) {
        e.preventDefault();
        const clientErrors = validateClientSide();
        if (Object.keys(clientErrors).length > 0) {
            setFieldErrors(clientErrors);
            setFormErrorMsg("Fix the highlighted fields and try again.");
            return;
        }
        setSaving(true);
        setFormErrorMsg(null);
        setFieldErrors({});
        try {
            // Only send a secret when the user actually generated/rotated one this session
            // — create-mode with no secret generated, or edit-mode without an explicit
            // rotation, both send "" (no change), matching the "omit means unchanged"
            // contract on UpdateWorkflowRequest.webhook_secret.
            const shouldSendSecret = !isEdit || rotatingSecret;
            await onSave({
                ...formData,
                // A cron trigger has no way to be "disabled" through this form (the checkbox
                // is disabled+forced for triggerType==="cron" below) — both fields are forced
                // true so the row both registers as a cron entry (cronEnabled) and passes the
                // webhook-handler admission gate if it's ever repurposed (enabled).
                cronEnabled: formData.triggerType === "cron" ? true : formData.cronEnabled,
                enabled: formData.triggerType === "cron" ? true : formData.enabled,
                webhookSecret: shouldSendSecret ? generatedSecret !== null && generatedSecret !== void 0 ? generatedSecret : "" : ""
            });
            setLiveMessage(isEdit ? "Trigger updated." : "Trigger created.");
            onClose();
        } catch (err) {
            const message = err instanceof Error ? err.message : "Failed to save trigger.";
            const field = mapErrorToField(message);
            const currentType = formData.triggerType || "github_push";
            if (field && isFieldVisible(field, currentType)) {
                setFieldErrors({
                    [field]: message
                });
                setFormErrorMsg(null);
            } else {
                // Field mapped but its input isn't rendered for the current type (or no field
                // could be determined) — fall back to the generic banner rather than an error
                // that would silently attach to nothing on screen.
                setFormErrorMsg(message);
            }
            setLiveMessage("Trigger save failed: ".concat(message));
        } finally{
            setSaving(false);
        }
    }
    const triggerType = formData.triggerType || "github_push";
    var _formData_enabled, _formData_githubRepo, _formData_githubBranch, _formData_promptTemplate, _formData_cronExpression, _formData_webhookSlug, _formData_eventFilter, _formData_labelFilter, _formData_promptTemplate1;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(Modal/* Modal */.aF, {
        open: open,
        onOpenChange: (o)=>{
            if (!o) onClose();
        },
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)(Modal/* ModalContent */.$m, {
            className: ApprovalRulesPanel_css/* ruleModalContent */.RK,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: ApprovalRulesPanel_css/* modalHeader */._z,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: ApprovalRulesPanel_css/* modalTitleRow */.gn,
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(Modal/* ModalTitle */.wt, {
                                children: isEdit ? "Edit trigger: ".concat(editTrigger === null || editTrigger === void 0 ? void 0 : editTrigger.name) : "Add Trigger"
                            })
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)(Modal/* ModalClose */.Oj, {
                            className: ApprovalRulesPanel_css/* modalCloseButton */.iT,
                            "aria-label": "Close dialog",
                            children: "\xd7"
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: ApprovalRulesPanel_css/* modalBody */.S3,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            "aria-live": "polite",
                            "aria-atomic": "true",
                            className: visuallyHidden,
                            children: liveMessage
                        }),
                        formErrorMsg && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: ApprovalRulesPanel_css/* formError */.zc,
                            role: "alert",
                            children: formErrorMsg
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("form", {
                            onSubmit: handleSubmit,
                            "data-testid": "trigger-form",
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: ApprovalRulesPanel_css/* formSection */.gy,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                            className: ApprovalRulesPanel_css/* formSectionHeader */.$6,
                                            children: "Trigger type"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                            className: typeSelector,
                                            role: "radiogroup",
                                            "aria-label": "Trigger type",
                                            children: TRIGGER_TYPES.map((t)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                    type: "button",
                                                    role: "radio",
                                                    "aria-checked": triggerType === t.value,
                                                    className: "".concat(typeOption, " ").concat(triggerType === t.value ? typeOptionActive : ""),
                                                    onClick: ()=>handleTypeChange(t.value),
                                                    disabled: isEdit,
                                                    "data-testid": "trigger-type-".concat(t.value),
                                                    children: t.label
                                                }, t.value))
                                        }),
                                        isEdit && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                            className: fieldHint,
                                            children: "Trigger type can't be changed after creation."
                                        })
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: ApprovalRulesPanel_css/* formSection */.gy,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                            className: ApprovalRulesPanel_css/* formSectionHeader */.$6,
                                            children: "Details"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: ApprovalRulesPanel_css/* formGrid */.lX,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                                    className: ApprovalRulesPanel_css/* label */.Pf,
                                                    children: [
                                                        "Slug *",
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                            className: ApprovalRulesPanel_css/* input */.hF,
                                                            "data-testid": "trigger-slug-input",
                                                            value: formData.slug,
                                                            onChange: (e)=>setField("slug", e.target.value),
                                                            disabled: isEdit,
                                                            placeholder: "jira-ticket"
                                                        }),
                                                        fieldErrors.slug && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                            className: fieldError,
                                                            children: fieldErrors.slug
                                                        })
                                                    ]
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                                    className: ApprovalRulesPanel_css/* label */.Pf,
                                                    children: [
                                                        "Name *",
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                            className: ApprovalRulesPanel_css/* input */.hF,
                                                            "data-testid": "trigger-name-input",
                                                            value: formData.name,
                                                            onChange: (e)=>setField("name", e.target.value),
                                                            placeholder: "Triage new Jira tickets"
                                                        }),
                                                        fieldErrors.name && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                            className: fieldError,
                                                            children: fieldErrors.name
                                                        })
                                                    ]
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                                    className: ApprovalRulesPanel_css/* label */.Pf,
                                                    children: [
                                                        "Target directory *",
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)(RepoPathInput/* RepoPathInput */.E, {
                                                            "aria-label": "Target directory",
                                                            "data-testid": "trigger-target-directory-input",
                                                            value: formData.targetDirectory,
                                                            onChange: (v)=>setField("targetDirectory", v),
                                                            placeholder: "/home/user/projects/repo"
                                                        }),
                                                        fieldErrors.targetDirectory && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                            className: fieldError,
                                                            children: fieldErrors.targetDirectory
                                                        })
                                                    ]
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                                    className: ApprovalRulesPanel_css/* label */.Pf,
                                                    children: [
                                                        "Command / prompt *",
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                            className: ApprovalRulesPanel_css/* input */.hF,
                                                            "data-testid": "trigger-command-input",
                                                            value: formData.command,
                                                            onChange: (e)=>setField("command", e.target.value),
                                                            placeholder: "Investigate and fix the reported issue"
                                                        }),
                                                        fieldErrors.command && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                            className: fieldError,
                                                            children: fieldErrors.command
                                                        })
                                                    ]
                                                })
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                            className: checkboxLabel,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                    type: "checkbox",
                                                    checked: triggerType === "cron" ? true : (_formData_enabled = formData.enabled) !== null && _formData_enabled !== void 0 ? _formData_enabled : true,
                                                    onChange: (e)=>setField("enabled", e.target.checked),
                                                    disabled: triggerType === "cron",
                                                    "data-testid": "trigger-enabled-checkbox"
                                                }),
                                                triggerType === "cron" ? "Enabled (controlled by the schedule below)" : "Enabled"
                                            ]
                                        })
                                    ]
                                }),
                                triggerType === "github_push" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("fieldset", {
                                    className: fieldset,
                                    "aria-labelledby": "tf-github-legend",
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("legend", {
                                            id: "tf-github-legend",
                                            className: legend,
                                            children: "GitHub push"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: ApprovalRulesPanel_css/* formGrid */.lX,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                                    className: ApprovalRulesPanel_css/* label */.Pf,
                                                    children: [
                                                        "Repository *",
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                            className: ApprovalRulesPanel_css/* input */.hF,
                                                            "data-testid": "trigger-github-repo-input",
                                                            value: (_formData_githubRepo = formData.githubRepo) !== null && _formData_githubRepo !== void 0 ? _formData_githubRepo : "",
                                                            onChange: (e)=>setField("githubRepo", e.target.value),
                                                            placeholder: "owner/repo"
                                                        }),
                                                        fieldErrors.githubRepo && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                            className: fieldError,
                                                            children: fieldErrors.githubRepo
                                                        })
                                                    ]
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                                    className: ApprovalRulesPanel_css/* label */.Pf,
                                                    children: [
                                                        "Branch",
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                            className: ApprovalRulesPanel_css/* input */.hF,
                                                            "data-testid": "trigger-github-branch-input",
                                                            value: (_formData_githubBranch = formData.githubBranch) !== null && _formData_githubBranch !== void 0 ? _formData_githubBranch : "",
                                                            onChange: (e)=>setField("githubBranch", e.target.value),
                                                            placeholder: "main"
                                                        })
                                                    ]
                                                })
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                            className: ApprovalRulesPanel_css/* label */.Pf,
                                            children: [
                                                "Prompt template",
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("textarea", {
                                                    className: "".concat(ApprovalRulesPanel_css/* input */.hF, " ").concat(promptTextarea),
                                                    "data-testid": "trigger-prompt-template-input",
                                                    rows: 3,
                                                    value: (_formData_promptTemplate = formData.promptTemplate) !== null && _formData_promptTemplate !== void 0 ? _formData_promptTemplate : "",
                                                    onChange: (e)=>setField("promptTemplate", e.target.value),
                                                    placeholder: "Review {{.head_commit.message}}"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: fieldHint,
                                                    children: "Go text/template, rendered against the inbound push payload."
                                                }),
                                                fieldErrors.promptTemplate && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: fieldError,
                                                    children: fieldErrors.promptTemplate
                                                })
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)(SecretField, {
                                            triggerType: "github_push",
                                            isEdit: isEdit,
                                            rotating: rotatingSecret,
                                            generatedSecret: generatedSecret,
                                            secretCopied: secretCopied,
                                            secretCopyError: secretCopyError,
                                            generationUnsupported: secretGenerationUnsupported,
                                            error: fieldErrors.secret,
                                            onStartRotate: ()=>setRotatingSecret(true),
                                            onGenerate: handleGenerateSecret,
                                            onCopy: (v)=>{
                                                void handleCopySecret(v);
                                            }
                                        })
                                    ]
                                }),
                                triggerType === "cron" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("fieldset", {
                                    className: fieldset,
                                    "aria-labelledby": "tf-cron-legend",
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("legend", {
                                            id: "tf-cron-legend",
                                            className: legend,
                                            children: "Schedule"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                            className: ApprovalRulesPanel_css/* label */.Pf,
                                            children: [
                                                "Cron expression *",
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                    className: ApprovalRulesPanel_css/* input */.hF,
                                                    "data-testid": "trigger-cron-expression-input",
                                                    value: (_formData_cronExpression = formData.cronExpression) !== null && _formData_cronExpression !== void 0 ? _formData_cronExpression : "",
                                                    onChange: (e)=>setField("cronExpression", e.target.value),
                                                    placeholder: "0 9 * * *"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: fieldHint,
                                                    children: "Standard 5-field cron syntax (minute hour dom month dow)."
                                                }),
                                                fieldErrors.cronExpression && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: fieldError,
                                                    children: fieldErrors.cronExpression
                                                })
                                            ]
                                        })
                                    ]
                                }),
                                triggerType === "webhook" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("fieldset", {
                                    className: fieldset,
                                    "aria-labelledby": "tf-webhook-legend",
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("legend", {
                                            id: "tf-webhook-legend",
                                            className: legend,
                                            children: "Webhook"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: ApprovalRulesPanel_css/* formGrid */.lX,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                                    className: ApprovalRulesPanel_css/* label */.Pf,
                                                    children: [
                                                        "Slug *",
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                            className: ApprovalRulesPanel_css/* input */.hF,
                                                            "data-testid": "trigger-webhook-slug-input",
                                                            value: (_formData_webhookSlug = formData.webhookSlug) !== null && _formData_webhookSlug !== void 0 ? _formData_webhookSlug : "",
                                                            onChange: (e)=>setField("webhookSlug", e.target.value),
                                                            placeholder: "jira-ticket",
                                                            disabled: isEdit
                                                        }),
                                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                            className: fieldHint,
                                                            children: [
                                                                "Routes POST /webhooks/",
                                                                "{slug}",
                                                                " to this trigger."
                                                            ]
                                                        }),
                                                        fieldErrors.webhookSlug && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                            className: fieldError,
                                                            children: fieldErrors.webhookSlug
                                                        })
                                                    ]
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                                    className: ApprovalRulesPanel_css/* label */.Pf,
                                                    children: [
                                                        "Event filter",
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                            className: ApprovalRulesPanel_css/* input */.hF,
                                                            "data-testid": "trigger-event-filter-input",
                                                            value: (_formData_eventFilter = formData.eventFilter) !== null && _formData_eventFilter !== void 0 ? _formData_eventFilter : "",
                                                            onChange: (e)=>setField("eventFilter", e.target.value),
                                                            placeholder: "issue_created"
                                                        })
                                                    ]
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                                    className: ApprovalRulesPanel_css/* label */.Pf,
                                                    children: [
                                                        "Label filter",
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                            className: ApprovalRulesPanel_css/* input */.hF,
                                                            "data-testid": "trigger-label-filter-input",
                                                            value: (_formData_labelFilter = formData.labelFilter) !== null && _formData_labelFilter !== void 0 ? _formData_labelFilter : "",
                                                            onChange: (e)=>setField("labelFilter", e.target.value),
                                                            placeholder: "urgent"
                                                        })
                                                    ]
                                                })
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                            className: ApprovalRulesPanel_css/* label */.Pf,
                                            children: [
                                                "Prompt template",
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("textarea", {
                                                    className: "".concat(ApprovalRulesPanel_css/* input */.hF, " ").concat(promptTextarea),
                                                    "data-testid": "trigger-prompt-template-input",
                                                    rows: 3,
                                                    value: (_formData_promptTemplate1 = formData.promptTemplate) !== null && _formData_promptTemplate1 !== void 0 ? _formData_promptTemplate1 : "",
                                                    onChange: (e)=>setField("promptTemplate", e.target.value),
                                                    placeholder: "Triage {{.issue.key}}: {{.issue.summary}}"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: fieldHint,
                                                    children: "Go text/template, rendered against the inbound JSON payload."
                                                }),
                                                fieldErrors.promptTemplate && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: fieldError,
                                                    children: fieldErrors.promptTemplate
                                                })
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)(SecretField, {
                                            triggerType: "webhook",
                                            isEdit: isEdit,
                                            rotating: rotatingSecret,
                                            generatedSecret: generatedSecret,
                                            secretCopied: secretCopied,
                                            secretCopyError: secretCopyError,
                                            generationUnsupported: secretGenerationUnsupported,
                                            error: fieldErrors.secret,
                                            onStartRotate: ()=>setRotatingSecret(true),
                                            onGenerate: handleGenerateSecret,
                                            onCopy: (v)=>{
                                                void handleCopySecret(v);
                                            }
                                        })
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: "".concat(ApprovalRulesPanel_css/* formActions */.AU, " ").concat(formActionsSpaced),
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                            type: "submit",
                                            className: ApprovalRulesPanel_css/* saveButton */.eG,
                                            disabled: saving,
                                            "data-testid": "trigger-form-submit",
                                            children: saving ? "Saving…" : isEdit ? "Save Changes" : "Create Trigger"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                            type: "button",
                                            className: ApprovalRulesPanel_css/* cancelButton */.x7,
                                            onClick: onClose,
                                            disabled: saving,
                                            children: "Cancel"
                                        })
                                    ]
                                })
                            ]
                        })
                    ]
                })
            ]
        })
    });
}
/**
 * Webhook/GitHub secret UI (Task 7.1.2b, wired to the backend in the Phase 7 follow-up
 * — Task 7.2). Show-once + copy-to-clipboard when generating a new secret (create, or
 * an explicit rotation during edit), masked placeholder otherwise — matching the
 * never-round-trip-a-real-secret convention CallbackSettings uses for its callback
 * URLs. The generated/pasted value is sent to the backend via
 * WorkflowFormData.webhookSecret on submit (see TriggerFormModal.handleSubmit) and is
 * never re-read from the server afterward.
 */ function SecretField(param) {
    let { isEdit, rotating, generatedSecret, secretCopied, secretCopyError, generationUnsupported, error, onStartRotate, onGenerate, onCopy } = param;
    const showGenerateUI = !isEdit || rotating;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: secretBox,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: "".concat(legend, " ").concat(secretBoxLegend),
                children: "Shared secret"
            }),
            !showGenerateUI ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: secretRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: secretMasked,
                        "data-testid": "trigger-secret-masked",
                        children: "•••• (unchanged)"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: secretButton,
                        onClick: onStartRotate,
                        "data-testid": "trigger-secret-rotate",
                        children: "Rotate secret"
                    })
                ]
            }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: secretRow,
                children: [
                    generatedSecret ? // Selectable + focusable so a failed clipboard write still has a manual
                    // fallback — this value is shown exactly once and never re-fetchable.
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: secretValue,
                        "data-testid": "trigger-secret-value",
                        tabIndex: 0,
                        children: generatedSecret
                    }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: fieldHint,
                        children: generationUnsupported ? "Secret generation requires a modern browser with the Web Crypto API." : "Generate a secret to verify inbound requests (HMAC)."
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: secretButton,
                        onClick: onGenerate,
                        disabled: generationUnsupported,
                        "data-testid": "trigger-secret-generate",
                        children: generatedSecret ? "Regenerate" : "Generate secret"
                    }),
                    generatedSecret && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: secretButton,
                        onClick: ()=>onCopy(generatedSecret),
                        "data-testid": "trigger-secret-copy",
                        children: secretCopied ? "Copied ✓" : "Copy"
                    })
                ]
            }),
            showGenerateUI && !generationUnsupported && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: secretWarning,
                children: "Copy this now — it won't be shown again after you save."
            }),
            secretCopied && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: secretCopiedNotice,
                children: "Copied to clipboard."
            }),
            secretCopyError && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: secretCopyErrorNotice,
                role: "alert",
                "data-testid": "trigger-secret-copy-error",
                children: "Copy failed — select the text above to copy manually."
            }),
            error && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: fieldError,
                "data-testid": "trigger-secret-field-error",
                children: error
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ./src/gen/session/v1/session_pb.ts
var session_pb = __webpack_require__(23575);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/create.js
var create = __webpack_require__(95444);
// EXTERNAL MODULE: ./src/lib/api/transport.ts
var transport = __webpack_require__(21694);
;// ./src/lib/hooks/useTriggerFireEvents.ts
/* __next_internal_client_entry_do_not_use__ useTriggerFireEvents auto */ 




/**
 * React hook wrapping ListTriggerFireEvents — the per-trigger audit trail behind
 * TriggerExecutionHistory (webhook-triggers Epic 7.2). `workflowId` may be empty
 * while a panel hasn't resolved a selection yet; the hook no-ops in that case
 * rather than issuing a request for "all workflows" (the RPC is scoped per-workflow).
 */ function useTriggerFireEvents(workflowId) {
    let limit = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : 100;
    const [events, setEvents] = (0,react.useState)([]);
    // Starts true: the mount effect below fetches unconditionally (once workflowId is
    // set), so initializing to false produced a one-frame flash of the empty-history
    // UI before the real fetch landed.
    const [loading, setLoading] = (0,react.useState)(true);
    const [error, setError] = (0,react.useState)(null);
    const clientRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
    }, []);
    const refresh = (0,react.useCallback)(async ()=>{
        if (!workflowId) {
            // No-op case (see doc comment above) — not an in-flight fetch, so don't leave
            // `loading` stuck true.
            setEvents([]);
            setLoading(false);
            return;
        }
        if (!clientRef.current) {
            // Client not constructed yet (effect ordering edge case) — leave `loading` as
            // whatever it currently is rather than flipping it off prematurely.
            return;
        }
        setLoading(true);
        setError(null);
        try {
            const req = (0,create/* create */.v)(session_pb/* ListTriggerFireEventsRequestSchema */.Kap, {
                workflowId,
                limit
            });
            const resp = await clientRef.current.listTriggerFireEvents(req);
            var _resp_events;
            setEvents((_resp_events = resp.events) !== null && _resp_events !== void 0 ? _resp_events : []);
        } catch (err) {
            const e = err instanceof Error ? err : new Error("Failed to fetch trigger fire events");
            setError(e);
            console.error("Failed to fetch trigger fire events:", e);
        } finally{
            setLoading(false);
        }
    }, [
        workflowId,
        limit
    ]);
    (0,react.useEffect)(()=>{
        void refresh();
    }, [
        refresh
    ]);
    return {
        events,
        loading,
        error,
        refresh
    };
}

// EXTERNAL MODULE: ./src/components/sessions/TriggersPanel.css.ts
var TriggersPanel_css = __webpack_require__(61823);
;// ./src/components/sessions/TriggerExecutionHistory.tsx
/* __next_internal_client_entry_do_not_use__ TriggerExecutionHistory auto */ 
// +feature: trigger-execution-history




function outcomeLabel(outcome) {
    switch(outcome){
        case "fired_success":
            return "Fired";
        case "fired_failed":
            return "Session creation failed";
        case "rejected":
            return "Rejected";
        case "no_match":
            return "No match";
        default:
            return outcome;
    }
}
function outcomeClass(outcome) {
    switch(outcome){
        case "fired_success":
            return TriggersPanel_css/* statusFiredSuccess */.SA;
        case "fired_failed":
            return TriggersPanel_css/* statusFiredFailed */.Mn;
        case "rejected":
            return TriggersPanel_css/* statusRejected */.rP;
        case "no_match":
            return TriggersPanel_css/* statusNoMatch */.Pn;
        default:
            return TriggersPanel_css/* statusNoMatch */.Pn;
    }
}
/**
 * TriggerExecutionHistory — per-trigger fire-attempt log (webhook-triggers Epic 7.2).
 * Shows the 5-state outcome (fired_success/fired_failed/rejected collapsed here since
 * "disabled" is a Workflow-level state shown on TriggersPanel's row, not a per-event
 * outcome) behind an "N received / M matched" counter that collapses no_match entries
 * by default — per research/ux.md §2, users don't expect every non-matching event by
 * default, but need the received-vs-matched split to distinguish "correctly filtering"
 * from "dead and receiving nothing."
 */ function TriggerExecutionHistory(param) {
    let { workflowId } = param;
    const { events, loading, error } = useTriggerFireEvents(workflowId);
    const [showNoMatch, setShowNoMatch] = (0,react.useState)(false);
    const { matched, noMatchCount } = (0,react.useMemo)(()=>{
        const noMatch = events.filter((e)=>e.outcome === "no_match");
        const rest = events.filter((e)=>e.outcome !== "no_match");
        return {
            matched: rest,
            noMatchCount: noMatch.length
        };
    }, [
        events
    ]);
    const visibleEvents = showNoMatch ? events : matched;
    if (loading && events.length === 0) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: TriggersPanel_css/* loading */.Lq,
            "data-testid": "trigger-history-loading",
            children: "Loading history…"
        });
    }
    if (error) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: TriggersPanel_css/* empty */.Ie,
            role: "alert",
            children: [
                "Failed to load trigger history: ",
                error.message
            ]
        });
    }
    if (events.length === 0) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: TriggersPanel_css/* empty */.Ie,
            "data-testid": "trigger-history-empty",
            children: "No events received yet."
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        "data-testid": "trigger-execution-history",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: TriggersPanel_css/* historyCounter */.Db,
                children: [
                    events.length,
                    " received / ",
                    matched.length,
                    " matched",
                    noMatchCount > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: TriggersPanel_css/* showNoMatchToggle */.sH,
                        onClick: ()=>setShowNoMatch((v)=>!v),
                        "data-testid": "trigger-history-toggle-no-match",
                        children: showNoMatch ? "Hide non-matching" : "Show ".concat(noMatchCount, " non-matching")
                    })
                ]
            }),
            visibleEvents.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: TriggersPanel_css/* empty */.Ie,
                children: "No matched events yet."
            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                className: TriggersPanel_css/* historyList */.PI,
                children: visibleEvents.map((ev)=>{
                    const date = (0,timestamp/* protoTimestampToDate */.uj)(ev.createdAt);
                    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                        className: TriggersPanel_css/* historyEntry */.xg,
                        "data-testid": "trigger-history-entry-".concat(ev.id),
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: "".concat(TriggersPanel_css/* statusBadge */.Od, " ").concat(outcomeClass(ev.outcome)),
                                children: outcomeLabel(ev.outcome)
                            }),
                            date && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: TriggersPanel_css/* historyTimestamp */.ZU,
                                children: date.toLocaleString()
                            }),
                            ev.errorMessage && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: TriggersPanel_css/* historyError */.V9,
                                children: ev.errorMessage
                            }),
                            ev.sessionId && /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                                className: TriggersPanel_css/* historySessionLink */.bX,
                                href: "/?session=".concat(ev.sessionId),
                                "data-testid": "trigger-history-session-link-".concat(ev.id),
                                children: "View session"
                            })
                        ]
                    }, ev.id);
                })
            })
        ]
    });
}

;// ./src/components/sessions/TriggersPanel.tsx
/* __next_internal_client_entry_do_not_use__ TriggersPanel auto */ 
// +feature: triggers-panel







const TriggersPanel_TRIGGER_TYPES = [
    "github_push",
    "cron",
    "webhook"
];
function typeLabel(t) {
    switch(t){
        case "github_push":
            return "GitHub Push";
        case "cron":
            return "Cron";
        case "webhook":
            return "Webhook";
        default:
            return t;
    }
}
function typeClass(t) {
    switch(t){
        case "github_push":
            return TriggersPanel_css/* typeGithubPush */.C7;
        case "cron":
            return TriggersPanel_css/* typeCron */.x6;
        case "webhook":
            return TriggersPanel_css/* typeWebhook */.qN;
        default:
            return TriggersPanel_css/* typeCron */.x6;
    }
}
function relativeTime(date) {
    if (!date) return "";
    const diffMs = Date.now() - date.getTime();
    const diffSec = Math.floor(diffMs / 1000);
    if (diffSec < 60) return "just now";
    const diffMin = Math.floor(diffSec / 60);
    if (diffMin < 60) return "".concat(diffMin, "m ago");
    const diffHr = Math.floor(diffMin / 60);
    if (diffHr < 24) return "".concat(diffHr, "h ago");
    const diffDay = Math.floor(diffHr / 24);
    return "".concat(diffDay, "d ago");
}
/** LastFired renders a trigger's last-fired indicator — shared between the desktop
 * table row and mobile card so both stay in sync without duplicating the ternary. */ function LastFired(param) {
    let { lastFiredAt, showTitle } = param;
    const date = (0,timestamp/* protoTimestampToDate */.uj)(lastFiredAt);
    if (!date) return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
        className: TriggersPanel_css/* neverFired */.df,
        children: "Never fired"
    });
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
        className: TriggersPanel_css/* lastFired */.Ft,
        title: showTitle ? date.toLocaleString() : undefined,
        children: relativeTime(date)
    });
}
/** TriggerToggle renders the enable/disable button — shared between the desktop table
 * row and mobile card. testId is optional since only the desktop row currently exposes
 * one (preserving each layout's existing locators exactly). */ function TriggerToggle(param) {
    let { workflow, disabled, onToggle, testId } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
        className: "".concat(TriggersPanel_css/* toggle */.L$, " ").concat(workflow.enabled ? TriggersPanel_css/* toggleOn */._s : TriggersPanel_css/* toggleOff */.St),
        onClick: onToggle,
        disabled: disabled,
        "aria-label": workflow.enabled ? "Disable trigger ".concat(workflow.name || workflow.slug) : "Enable trigger ".concat(workflow.name || workflow.slug),
        "data-testid": testId,
        children: workflow.enabled ? "ON" : "OFF"
    });
}
/** TriggerRowActions renders the Edit + History buttons — shared between the desktop
 * table row and mobile card. ariaLabel/testId props are optional since only the desktop
 * row currently exposes them (preserving each layout's existing locators exactly). */ function TriggerRowActions(param) {
    let { workflow, isExpanded, onEdit, onToggleHistory, editAriaLabel, editTestId, historyTestId } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: TriggersPanel_css/* rowActions */.cm,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                className: TriggersPanel_css/* iconButton */.gb,
                onClick: onEdit,
                "aria-label": editAriaLabel,
                "data-testid": editTestId,
                children: "Edit"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                className: TriggersPanel_css/* historyToggle */.bK,
                onClick: onToggleHistory,
                "aria-expanded": isExpanded,
                "data-testid": historyTestId,
                children: isExpanded ? "Hide history" : "History"
            })
        ]
    });
}
/**
 * TriggersPanel — the trigger config surface (webhook-triggers Epic 7.1): list of
 * cron/github_push/webhook Workflow rows with type badges, last-fired timestamp,
 * enable/disable toggle, and create/edit via TriggerFormModal. Extends
 * ApprovalRulesPanel's established shape (hits column → last-fired, source badge →
 * type badge, toggle, mobile FAB) per research/ux.md.
 *
 * Plain "manual" (@slug, non-triggered) Workflow rows are managed by the existing
 * WorkflowsPanel (/workflows) and are intentionally excluded here — this panel is
 * scoped to rows with an inbound/scheduled activation mechanism.
 */ function TriggersPanel() {
    const { workflows, loading, error, createWorkflow, updateWorkflow, refresh } = (0,useWorkflows/* useWorkflows */.m)();
    const [typeFilter, setTypeFilter] = (0,react.useState)("all");
    const [modalOpen, setModalOpen] = (0,react.useState)(false);
    const [editTrigger, setEditTrigger] = (0,react.useState)(null);
    const [expandedId, setExpandedId] = (0,react.useState)(null);
    const [liveMessage, setLiveMessage] = (0,react.useState)("");
    const [togglingId, setTogglingId] = (0,react.useState)(null);
    const [toggleErrorMsg, setToggleErrorMsg] = (0,react.useState)(null);
    const toggleErrorTimeoutRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        return ()=>{
            if (toggleErrorTimeoutRef.current) clearTimeout(toggleErrorTimeoutRef.current);
        };
    }, []);
    const triggers = (0,react.useMemo)(()=>workflows.filter((w)=>TriggersPanel_TRIGGER_TYPES.includes(w.triggerType)), [
        workflows
    ]);
    const visibleTriggers = (0,react.useMemo)(()=>typeFilter === "all" ? triggers : triggers.filter((w)=>w.triggerType === typeFilter), [
        triggers,
        typeFilter
    ]);
    function openCreate() {
        setEditTrigger(null);
        setModalOpen(true);
    }
    function openEdit(w) {
        setEditTrigger(w);
        setModalOpen(true);
    }
    async function handleSave(data) {
        if (editTrigger) {
            await updateWorkflow(editTrigger.id, data);
        } else {
            await createWorkflow(data);
        }
    }
    async function handleToggle(w) {
        setTogglingId(w.id);
        setToggleErrorMsg(null);
        try {
            const nextEnabled = !w.enabled;
            // Cron firing is gated on cronEnabled, not enabled (Scheduler never reads
            // enabled — see workflow_service.go's validateTriggerTypeFieldConsistency doc
            // comment). Keep the two fields in lockstep for cron-type rows so this single
            // toggle still controls whether a cron trigger actually fires; for
            // webhook/github_push rows only enabled matters, so cronEnabled is left alone.
            // expectedUpdatedAt (AC9's CAS precondition) is included so two tabs/users
            // clicking the same toggle in quick succession can't both silently win — the
            // second one gets a CodeAborted conflict instead.
            await updateWorkflow(w.id, {
                enabled: nextEnabled,
                ...w.triggerType === "cron" && {
                    cronEnabled: nextEnabled
                },
                expectedUpdatedAt: w.updatedAt
            });
            setLiveMessage("".concat(w.name || w.slug, " ").concat(w.enabled ? "disabled" : "enabled", "."));
        } catch (e) {
            console.error("Failed to toggle trigger:", e);
            const isConflict = e instanceof connect_error/* ConnectError */.T && e.code === code/* Code */.C.Aborted;
            const message = isConflict ? "".concat(w.name || w.slug, " was changed elsewhere — refresh and try again.") : "Failed to ".concat(w.enabled ? "disable" : "enable", " ").concat(w.name || w.slug, ".");
            setLiveMessage(message);
            // Visible (not just screen-reader-only) error for sighted users — transient,
            // clears itself after a few seconds or on the next toggle attempt.
            setToggleErrorMsg(message);
            if (toggleErrorTimeoutRef.current) clearTimeout(toggleErrorTimeoutRef.current);
            toggleErrorTimeoutRef.current = setTimeout(()=>setToggleErrorMsg((prev)=>prev === message ? null : prev), 5000);
        } finally{
            setTogglingId(null);
        }
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: TriggersPanel_css/* panel */.nd,
        "data-testid": "triggers-panel",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: TriggersPanel_css/* header */.wx,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: TriggersPanel_css/* titleRow */.$,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                                className: TriggersPanel_css/* title */.DD,
                                children: "Triggers"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: TriggersPanel_css/* headerButtons */.Pz,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        className: "".concat(TriggersPanel_css/* addButton */.c9, " ").concat(TriggersPanel_css/* headerButtonsHiddenOnMobile */.k),
                                        onClick: openCreate,
                                        "data-testid": "add-trigger-button",
                                        children: "+ Add Trigger"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        onClick: ()=>void refresh(),
                                        className: TriggersPanel_css/* refreshButton */._c,
                                        disabled: loading,
                                        "aria-label": "Refresh triggers",
                                        title: "Refresh triggers",
                                        children: loading ? "⟳" : "↻"
                                    })
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: TriggersPanel_css/* subtitle */.VA,
                        children: "Automated triggers create sessions from GitHub pushes, schedules, or inbound webhooks."
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                "aria-live": "polite",
                "aria-atomic": "true",
                className: TriggersPanel_css/* visuallyHidden */.Qg,
                children: liveMessage
            }),
            toggleErrorMsg && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: TriggersPanel_css/* toggleError */.vT,
                role: "alert",
                "data-testid": "trigger-toggle-error",
                children: toggleErrorMsg
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: TriggersPanel_css/* tabs */.vR,
                children: [
                    "all",
                    ...TriggersPanel_TRIGGER_TYPES
                ].map((t)=>{
                    const count = t === "all" ? triggers.length : triggers.filter((w)=>w.triggerType === t).length;
                    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                        className: "".concat(TriggersPanel_css/* tab */.Mf, " ").concat(typeFilter === t ? TriggersPanel_css/* tabActive */.yR : ""),
                        onClick: ()=>setTypeFilter(t),
                        "data-testid": "trigger-tab-".concat(t),
                        children: [
                            t === "all" ? "All" : typeLabel(t),
                            " (",
                            count,
                            ")"
                        ]
                    }, t);
                })
            }),
            error && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: TriggersPanel_css/* error */.z3,
                children: [
                    "Failed to load triggers: ",
                    error.message,
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        onClick: ()=>void refresh(),
                        className: TriggersPanel_css/* retryButton */.ft,
                        children: "Retry"
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: TriggersPanel_css/* tableWrapper */.wY,
                children: loading && visibleTriggers.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: TriggersPanel_css/* loading */.Lq,
                    children: "Loading triggers…"
                }) : visibleTriggers.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: TriggersPanel_css/* empty */.Ie,
                    "data-testid": "triggers-empty-state",
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                            children: [
                                "No triggers configured",
                                typeFilter !== "all" ? " for ".concat(typeLabel(typeFilter)) : "",
                                " yet."
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: TriggersPanel_css/* emptyStateLink */.FP,
                                    onClick: openCreate,
                                    children: "Add Trigger"
                                }),
                                " ",
                                "to create sessions automatically from GitHub pushes, schedules, or webhooks."
                            ]
                        })
                    ]
                }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("table", {
                    className: TriggersPanel_css/* table */.tp,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("thead", {
                            children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: TriggersPanel_css.th,
                                        children: "Name"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: TriggersPanel_css.th,
                                        children: "Type"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: TriggersPanel_css.th,
                                        children: "Last fired"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: TriggersPanel_css.th,
                                        children: "Enabled"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: TriggersPanel_css.th
                                    })
                                ]
                            })
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("tbody", {
                            children: visibleTriggers.map((w)=>{
                                const isExpanded = expandedId === w.id;
                                return /*#__PURE__*/ (0,jsx_runtime.jsxs)(react.Fragment, {
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                            className: "".concat(TriggersPanel_css/* row */.nM, " ").concat(!w.enabled ? TriggersPanel_css/* rowDisabled */.lr : ""),
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("td", {
                                                    className: TriggersPanel_css.td,
                                                    children: [
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                            className: TriggersPanel_css/* triggerName */.am,
                                                            children: w.name || w.slug
                                                        }),
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                            className: TriggersPanel_css/* triggerSlug */.O6,
                                                            children: w.slug
                                                        })
                                                    ]
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: TriggersPanel_css.td,
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: "".concat(TriggersPanel_css/* typeBadge */.AP, " ").concat(typeClass(w.triggerType)),
                                                        children: typeLabel(w.triggerType)
                                                    })
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: TriggersPanel_css.td,
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(LastFired, {
                                                        lastFiredAt: w.lastFiredAt,
                                                        showTitle: true
                                                    })
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: "".concat(TriggersPanel_css.td, " ").concat(TriggersPanel_css/* tdCenter */._S),
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(TriggerToggle, {
                                                        workflow: w,
                                                        disabled: togglingId === w.id,
                                                        onToggle: ()=>void handleToggle(w),
                                                        testId: "trigger-toggle-".concat(w.id)
                                                    })
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: TriggersPanel_css.td,
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(TriggerRowActions, {
                                                        workflow: w,
                                                        isExpanded: isExpanded,
                                                        onEdit: ()=>openEdit(w),
                                                        onToggleHistory: ()=>setExpandedId(isExpanded ? null : w.id),
                                                        editAriaLabel: "Edit trigger ".concat(w.name || w.slug),
                                                        editTestId: "trigger-edit-".concat(w.id),
                                                        historyTestId: "trigger-history-toggle-".concat(w.id)
                                                    })
                                                })
                                            ]
                                        }),
                                        isExpanded && /*#__PURE__*/ (0,jsx_runtime.jsx)("tr", {
                                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                className: TriggersPanel_css.td,
                                                colSpan: 5,
                                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                                    className: TriggersPanel_css/* historyWrapper */.qR,
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(TriggerExecutionHistory, {
                                                        workflowId: w.id
                                                    })
                                                })
                                            })
                                        })
                                    ]
                                }, w.id);
                            })
                        })
                    ]
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: TriggersPanel_css/* cardList */.hL,
                children: visibleTriggers.map((w)=>{
                    const isExpanded = expandedId === w.id;
                    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: TriggersPanel_css/* card */.Nr,
                        "data-testid": "trigger-card-".concat(w.id),
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: TriggersPanel_css/* cardTop */.MM,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: TriggersPanel_css/* triggerName */.am,
                                                children: w.name || w.slug
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: TriggersPanel_css/* triggerSlug */.O6,
                                                children: w.slug
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(TriggerToggle, {
                                        workflow: w,
                                        disabled: togglingId === w.id,
                                        onToggle: ()=>void handleToggle(w)
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: TriggersPanel_css/* cardMeta */.o0,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: "".concat(TriggersPanel_css/* typeBadge */.AP, " ").concat(typeClass(w.triggerType)),
                                        children: typeLabel(w.triggerType)
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(LastFired, {
                                        lastFiredAt: w.lastFiredAt
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(TriggerRowActions, {
                                workflow: w,
                                isExpanded: isExpanded,
                                onEdit: ()=>openEdit(w),
                                onToggleHistory: ()=>setExpandedId(isExpanded ? null : w.id)
                            }),
                            isExpanded && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: TriggersPanel_css/* historyWrapper */.qR,
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(TriggerExecutionHistory, {
                                    workflowId: w.id
                                })
                            })
                        ]
                    }, w.id);
                })
            }),
            visibleTriggers.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: TriggersPanel_css/* rowCount */.oW,
                children: [
                    visibleTriggers.length,
                    " trigger",
                    visibleTriggers.length !== 1 ? "s" : "",
                    typeFilter !== "all" && " (filtered from ".concat(triggers.length, " total)")
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                className: TriggersPanel_css/* mobileAddFab */.B7,
                onClick: openCreate,
                "aria-label": "Add trigger",
                "data-testid": "add-trigger-fab",
                children: "+"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(TriggerFormModal, {
                open: modalOpen,
                editTrigger: editTrigger,
                onSave: handleSave,
                onClose: ()=>{
                    setModalOpen(false);
                    setEditTrigger(null);
                }
            })
        ]
    });
}

;// ./src/lib/hooks/useCallbackConfig.ts
/* __next_internal_client_entry_do_not_use__ useCallbackConfig auto */ 




/**
 * React hook for the global outbound-callback URL settings (webhook-triggers Epic 5.1/7.3).
 * GetCallbackConfig only ever returns booleans ("is a URL configured") — the raw URL is
 * never echoed back on any read path, so this hook never holds a plaintext URL in state.
 */ function useCallbackConfig() {
    const [config, setConfig] = (0,react.useState)(null);
    // Starts true: the mount effect below fetches unconditionally, so initializing to
    // false produced a one-frame flash of the "not configured" UI before the real
    // fetch landed.
    const [loading, setLoading] = (0,react.useState)(true);
    const [error, setError] = (0,react.useState)(null);
    const clientRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
    }, []);
    const refresh = (0,react.useCallback)(async ()=>{
        if (!clientRef.current) return;
        setLoading(true);
        setError(null);
        try {
            const req = (0,create/* create */.v)(session_pb/* GetCallbackConfigRequestSchema */.gr$, {});
            const resp = await clientRef.current.getCallbackConfig(req);
            var _resp_config;
            setConfig((_resp_config = resp.config) !== null && _resp_config !== void 0 ? _resp_config : null);
        } catch (err) {
            const e = err instanceof Error ? err : new Error("Failed to fetch callback config");
            setError(e);
            console.error("Failed to fetch callback config:", e);
        } finally{
            setLoading(false);
        }
    }, []);
    (0,react.useEffect)(()=>{
        void refresh();
    }, [
        refresh
    ]);
    const updateConfig = (0,react.useCallback)(async (data)=>{
        if (!clientRef.current) return;
        const req = (0,create/* create */.v)(session_pb/* UpdateCallbackConfigRequestSchema */.X1o, {
            ...data.onSessionCompleteUrl !== undefined && {
                onSessionCompleteUrl: data.onSessionCompleteUrl
            },
            ...data.onSessionStaleUrl !== undefined && {
                onSessionStaleUrl: data.onSessionStaleUrl
            },
            ...data.onQueueItemCreatedUrl !== undefined && {
                onQueueItemCreatedUrl: data.onQueueItemCreatedUrl
            }
        });
        const resp = await clientRef.current.updateCallbackConfig(req);
        var _resp_config;
        setConfig((_resp_config = resp.config) !== null && _resp_config !== void 0 ? _resp_config : null);
    }, []);
    return {
        config,
        loading,
        error,
        updateConfig,
        refresh
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FCallbackSettings.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA81U0XKrIBB971fsY30wIzZRy_2YDgoxTBC8iEmaO_33Dpq0QjCmfbovOgO7h7Nnz-7qDR1Mo9UpgX9PACWp9rVWvaQYDkQ_x_EbemXnVJ_z6I-9V5oyjQG1J-iU4NSLYpOoWBPK-w4DStuTPW4JpVzWGNJkPKC8awV5x7AVbDiw_5hyzSrDlcRQKdE30t7UpMWAMpv38bS6kkYD6Z_CrD2UdEBpiK65xJAMCEqauONn9k12ODoyXu8MhjwZwiollPaVSiIH_eUuOnoZ0YNIyEVa_6paX7NNGIUIXsuYG9Z0GComDdNfEIUHkQ0QgSocjTaPa5S7nLgUXLK4FKraO8axtsvHpzyPrScELpxCfctGTvM2P0SznHuXc_HfcC7nOacu59dw77-bfHHTUdsT-3XSycKOQOShJbEPLYnM2xF5e3LleGBekuhaAQYECNLsAtBwGR85NbthAKcV4a2q-m6oS_XGthCDVJJNGAafalxhywVlit9vz6Aw6a0w6cIiAah63dm7VvFxvKcF4J06ML3UYDRvNeTNdHWzJNAdhp5R6c-X1GiWCQZbKKZxeuI1_ap-7qlfWPWLW_XX84MbrPfvYkfY2BEslXnGlHekFIxGo1NbUnHzjiFZFRsv6RrpB26m70llYiKEOjLqpG9venbPVd5CrIdkV6lL-qKjliYDJXdGI2zHNAqujo9PayNgHPIIAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FCallbackSettings_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81U0XKrIBB971fsY30wIzZRy_2YDgoxTBC8iEmaO_33Dpq0QjCmfbovOgO7h7Nnz_7qDR1Mo9UpgX9PACWp9rVWvaQYDkQ_x_EbemXnVJ_z6I_9V5oyjQG1J_iU4NSLYpOoWBPK_w4DStuTPW4JpVzWGNJkPKC8awV5x7AVbDiw_5hyzSrDlcRQKdE30t7UpMWAMpv38bS6kkYD6Z_CrD2UdEBpiK65xJAMCEqauONn9k12ODoyXu8MhjwZwiollPaVSiIH_eUuOnoZ0YNIyEVa_6paX7NNGIUIXsuYG9Z0GComDdNfEIUHkQ0QgSocjTaPa5S7nLgUXLK4FKraO8axtsvHpzyPrScELpxCfctGTvM2P0SznHuXc_HfcC7nOacu59dw77_bfHHTUdsT_3XSycKOQOShJbEPLYnM2xF5e3LleGBekuhaAQYECNLsAtBwGR85NbthAKcV4a2q_m6oS_XGthCDVJJNGAafalxhywVlit9vz6Aw6a0w6cIiAah63dm7VvFxvKcF4J06ML3UYDRvNeTNdHWzJNAdhp5R6c_X1GiWCQZbKKZxeuI1_ap_7qlfWPWLW_XX84MbrPfvYkfY2BEslXnGlHekFIxGo1NbUnHzjiFZFRsv6RrpB26m70llYiKEOjLqpG9venbPVd5CrIdkV6lL_qKjliYDJXdGI2zHNAqujo9PayNgHPIIAAA_22_7D = __webpack_require__(7886);
;// ./src/components/sessions/CallbackSettings.css.ts



var clearButton = '_1vtmroxb';
var configuredBadge = '_1vtmrox7';
var errorBanner = '_1vtmroxg';
var fieldLabel = '_1vtmrox6';
var fieldLabelRow = '_1vtmrox5';
var fieldRow = '_1vtmrox4';
var header = '_1vtmrox1';
var hint = '_1vtmroxc';
var input = '_1vtmroxa';
var inputRow = '_1vtmrox9';
var notConfiguredBadge = '_1vtmrox8';
var panel = '_1vtmrox0';
var saveButton = '_1vtmroxe';
var saveRow = '_1vtmroxd';
var statusMessage = '_1vtmroxf';
var subtitle = '_1vtmrox3';
var title = '_1vtmrox2';
var CallbackSettings_css_visuallyHidden = '_9ji8bp0';
;// ./src/components/sessions/CallbackSettings.tsx
/* __next_internal_client_entry_do_not_use__ CallbackSettings auto */ 
// +feature: callback-settings



const FIELDS = [
    {
        key: "onSessionCompleteUrl",
        configuredKey: "onSessionCompleteConfigured",
        label: "On session complete",
        hint: "POSTed when a trigger-created session's backlog item transitions to done.",
        testId: "callback-on-session-complete"
    },
    {
        key: "onSessionStaleUrl",
        configuredKey: "onSessionStaleConfigured",
        label: "On session stale",
        hint: "POSTed when a session is marked stuck/stale.",
        testId: "callback-on-session-stale"
    },
    {
        key: "onQueueItemCreatedUrl",
        configuredKey: "onQueueItemCreatedConfigured",
        label: "On review-queue item created",
        hint: "POSTed when a new item lands in the human review queue.",
        testId: "callback-on-queue-item-created"
    }
];
/**
 * CallbackSettings — the outbound-callback URL config surface (webhook-triggers
 * Epic 5.1/7.3). Same masked-placeholder-on-edit convention as TriggerFormModal's
 * webhook secret field: GetCallbackConfig only ever returns booleans ("is a URL
 * configured"), never the URL itself, so this form never displays a real URL —
 * only whether one is set, plus an editable field to replace or clear it.
 */ function CallbackSettings() {
    const { config, loading, error, updateConfig } = useCallbackConfig();
    const [edits, setEdits] = (0,react.useState)({});
    const [saving, setSaving] = (0,react.useState)(false);
    const [saveError, setSaveError] = (0,react.useState)(null);
    const [savedMessage, setSavedMessage] = (0,react.useState)(null);
    const [liveMessage, setLiveMessage] = (0,react.useState)("");
    function setEdit(key, value) {
        setEdits((prev)=>({
                ...prev,
                [key]: value
            }));
        setSavedMessage(null);
    }
    const hasEdits = Object.keys(edits).length > 0;
    async function handleSave() {
        setSaving(true);
        setSaveError(null);
        try {
            const payload = {};
            for (const f of FIELDS){
                if (edits[f.key] !== undefined) {
                    payload[f.key] = edits[f.key];
                }
            }
            await updateConfig(payload);
            setEdits({});
            setSavedMessage("Callback settings saved.");
            setLiveMessage("Callback settings saved.");
        } catch (err) {
            const message = err instanceof Error ? err.message : "Failed to save callback settings.";
            setSaveError(message);
            setLiveMessage("Failed to save callback settings: ".concat(message));
        } finally{
            setSaving(false);
        }
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: panel,
        "data-testid": "callback-settings",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: header,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                        className: title,
                        children: "Outbound Callbacks"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: subtitle,
                        children: "Global lifecycle-event webhooks. URLs are never displayed once saved — only whether one is configured."
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                "aria-live": "polite",
                "aria-atomic": "true",
                className: CallbackSettings_css_visuallyHidden,
                children: liveMessage
            }),
            error && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: errorBanner,
                children: [
                    "Failed to load callback settings: ",
                    error.message
                ]
            }),
            saveError && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: errorBanner,
                role: "alert",
                children: saveError
            }),
            loading && !config ? /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: hint,
                children: "Loading…"
            }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    FIELDS.map((f)=>{
                        var _config_f_configuredKey;
                        const configured = (_config_f_configuredKey = config === null || config === void 0 ? void 0 : config[f.configuredKey]) !== null && _config_f_configuredKey !== void 0 ? _config_f_configuredKey : false;
                        const isEdited = edits[f.key] !== undefined;
                        var _edits_f_key;
                        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: fieldRow,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: fieldLabelRow,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                            className: fieldLabel,
                                            htmlFor: "cb-".concat(f.key),
                                            children: f.label
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: configured ? configuredBadge : notConfiguredBadge,
                                            "data-testid": "".concat(f.testId, "-status"),
                                            children: configured ? "Configured" : "Not configured"
                                        })
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: inputRow,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                            id: "cb-".concat(f.key),
                                            className: input,
                                            type: "url",
                                            "data-testid": f.testId,
                                            placeholder: configured ? "•••• (unchanged) — enter a new URL to replace it" : "https://example.com/hook",
                                            value: isEdited ? (_edits_f_key = edits[f.key]) !== null && _edits_f_key !== void 0 ? _edits_f_key : "" : "",
                                            onChange: (e)=>setEdit(f.key, e.target.value)
                                        }),
                                        configured && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                            type: "button",
                                            className: clearButton,
                                            onClick: ()=>setEdit(f.key, ""),
                                            "data-testid": "".concat(f.testId, "-clear"),
                                            children: isEdited && edits[f.key] === "" ? "Will clear" : "Clear"
                                        })
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: hint,
                                    children: f.hint
                                })
                            ]
                        }, f.key);
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: saveRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: saveButton,
                                onClick: ()=>void handleSave(),
                                disabled: saving || !hasEdits,
                                "data-testid": "callback-settings-save",
                                children: saving ? "Saving…" : "Save"
                            }),
                            savedMessage && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: statusMessage,
                                children: savedMessage
                            })
                        ]
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Ftriggers%2Fpage.css.ts.vanilla.css%22%2C%22source%22%3A%22LmE2dTZ6czAgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBtaW4taGVpZ2h0OiBjYWxjKHZhcigtLXZpZXdwb3J0LWhlaWdodCwgMTAwZHZoKSAtIHZhcigtLWhlYWRlci1oZWlnaHQpKTsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejYpOwp9Ci5hNnU2enMxIHsKICBmbGV4OiAxOwogIG1heC13aWR0aDogMTIwMHB4OwogIHdpZHRoOiAxMDAlOwogIG1hcmdpbjogMCBhdXRvOwogIHBhZGRpbmc6IDJyZW07CiAgb3ZlcmZsb3cteTogYXV0bzsKICBvdmVyZmxvdy14OiBoaWRkZW47CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogMnJlbTsKfQpAbWVkaWEgc2NyZWVuIGFuZCAobWF4LXdpZHRoOiA3NjhweCkgewogIC5hNnU2enMxIHsKICAgIHBhZGRpbmc6IDFyZW07CiAgfQp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Ftriggers_2Fpage_css_ts_vanilla_css_22_2C_22source_22_3A_22LmE2dTZ6czAgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBtaW4taGVpZ2h0OiBjYWxjKHZhcigtLXZpZXdwb3J0LWhlaWdodCwgMTAwZHZoKSAtIHZhcigtLWhlYWRlci1oZWlnaHQpKTsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejYpOwp9Ci5hNnU2enMxIHsKICBmbGV4OiAxOwogIG1heC13aWR0aDogMTIwMHB4OwogIHdpZHRoOiAxMDAlOwogIG1hcmdpbjogMCBhdXRvOwogIHBhZGRpbmc6IDJyZW07CiAgb3ZlcmZsb3cteTogYXV0bzsKICBvdmVyZmxvdy14OiBoaWRkZW47CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogMnJlbTsKfQpAbWVkaWEgc2NyZWVuIGFuZCAobWF4LXdpZHRoOiA3NjhweCkgewogIC5hNnU2enMxIHsKICAgIHBhZGRpbmc6IDFyZW07CiAgfQp9_22_7D = __webpack_require__(41029);
;// ./src/app/triggers/page.css.ts


var main = 'a6u6zs1';
var page = 'a6u6zs0';
;// ./src/app/triggers/page.tsx
// analytics-exempt
/* __next_internal_client_entry_do_not_use__ default auto */ 
// Note: no +feature marker here — TriggersPanel.tsx and CallbackSettings.tsx each
// carry their own canonical marker (unlike RulesPage's ApprovalRulesPanel, which has
// none of its own).



function TriggersPage() {
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: page,
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("main", {
            id: "main-content",
            className: main,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)(TriggersPanel, {}),
                /*#__PURE__*/ (0,jsx_runtime.jsx)(CallbackSettings, {})
            ]
        })
    });
}


/***/ }),

/***/ 41029:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 47439:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 55913:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 61128:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 61823:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   $: () => (/* binding */ titleRow),
/* harmony export */   AP: () => (/* binding */ typeBadge),
/* harmony export */   B7: () => (/* binding */ mobileAddFab),
/* harmony export */   C7: () => (/* binding */ typeGithubPush),
/* harmony export */   DD: () => (/* binding */ title),
/* harmony export */   Db: () => (/* binding */ historyCounter),
/* harmony export */   FP: () => (/* binding */ emptyStateLink),
/* harmony export */   Ft: () => (/* binding */ lastFired),
/* harmony export */   Ie: () => (/* binding */ empty),
/* harmony export */   L$: () => (/* binding */ toggle),
/* harmony export */   Lq: () => (/* binding */ loading),
/* harmony export */   MM: () => (/* binding */ cardTop),
/* harmony export */   Mf: () => (/* binding */ tab),
/* harmony export */   Mn: () => (/* binding */ statusFiredFailed),
/* harmony export */   Nr: () => (/* binding */ card),
/* harmony export */   O6: () => (/* binding */ triggerSlug),
/* harmony export */   Od: () => (/* binding */ statusBadge),
/* harmony export */   PI: () => (/* binding */ historyList),
/* harmony export */   Pn: () => (/* binding */ statusNoMatch),
/* harmony export */   Pz: () => (/* binding */ headerButtons),
/* harmony export */   Qg: () => (/* binding */ visuallyHidden),
/* harmony export */   SA: () => (/* binding */ statusFiredSuccess),
/* harmony export */   St: () => (/* binding */ toggleOff),
/* harmony export */   V9: () => (/* binding */ historyError),
/* harmony export */   VA: () => (/* binding */ subtitle),
/* harmony export */   ZU: () => (/* binding */ historyTimestamp),
/* harmony export */   _S: () => (/* binding */ tdCenter),
/* harmony export */   _c: () => (/* binding */ refreshButton),
/* harmony export */   _s: () => (/* binding */ toggleOn),
/* harmony export */   am: () => (/* binding */ triggerName),
/* harmony export */   bK: () => (/* binding */ historyToggle),
/* harmony export */   bX: () => (/* binding */ historySessionLink),
/* harmony export */   c9: () => (/* binding */ addButton),
/* harmony export */   cm: () => (/* binding */ rowActions),
/* harmony export */   df: () => (/* binding */ neverFired),
/* harmony export */   fZ: () => (/* binding */ attributionBadge),
/* harmony export */   ft: () => (/* binding */ retryButton),
/* harmony export */   gb: () => (/* binding */ iconButton),
/* harmony export */   hL: () => (/* binding */ cardList),
/* harmony export */   k: () => (/* binding */ headerButtonsHiddenOnMobile),
/* harmony export */   lr: () => (/* binding */ rowDisabled),
/* harmony export */   nM: () => (/* binding */ row),
/* harmony export */   nd: () => (/* binding */ panel),
/* harmony export */   o0: () => (/* binding */ cardMeta),
/* harmony export */   oW: () => (/* binding */ rowCount),
/* harmony export */   qN: () => (/* binding */ typeWebhook),
/* harmony export */   qR: () => (/* binding */ historyWrapper),
/* harmony export */   rP: () => (/* binding */ statusRejected),
/* harmony export */   sH: () => (/* binding */ showNoMatchToggle),
/* harmony export */   td: () => (/* binding */ td),
/* harmony export */   th: () => (/* binding */ th),
/* harmony export */   tp: () => (/* binding */ table),
/* harmony export */   vR: () => (/* binding */ tabs),
/* harmony export */   vT: () => (/* binding */ toggleError),
/* harmony export */   wY: () => (/* binding */ tableWrapper),
/* harmony export */   wx: () => (/* binding */ header),
/* harmony export */   x6: () => (/* binding */ typeCron),
/* harmony export */   xg: () => (/* binding */ historyEntry),
/* harmony export */   yR: () => (/* binding */ tabActive),
/* harmony export */   z3: () => (/* binding */ error)
/* harmony export */ });
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(65916);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Fa11y_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll85amk4YnAwIHsKICBwb3NpdGlvbjogYWJzb2x1dGU7CiAgd2lkdGg6IDFweDsKICBoZWlnaHQ6IDFweDsKICBwYWRkaW5nOiAwOwogIG1hcmdpbjogLTFweDsKICBvdmVyZmxvdzogaGlkZGVuOwogIGNsaXAtcGF0aDogaW5zZXQoNTAlKTsKICB3aGl0ZS1zcGFjZTogbm93cmFwOwogIGJvcmRlcjogMDsKfQ_22_7D__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(47439);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Fa11y_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll85amk4YnAwIHsKICBwb3NpdGlvbjogYWJzb2x1dGU7CiAgd2lkdGg6IDFweDsKICBoZWlnaHQ6IDFweDsKICBwYWRkaW5nOiAwOwogIG1hcmdpbjogLTFweDsKICBvdmVyZmxvdzogaGlkZGVuOwogIGNsaXAtcGF0aDogaW5zZXQoNTAlKTsKICB3aGl0ZS1zcGFjZTogbm93cmFwOwogIGJvcmRlcjogMDsKfQ_22_7D__WEBPACK_IMPORTED_MODULE_1___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Fa11y_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll85amk4YnAwIHsKICBwb3NpdGlvbjogYWJzb2x1dGU7CiAgd2lkdGg6IDFweDsKICBoZWlnaHQ6IDFweDsKICBwYWRkaW5nOiAwOwogIG1hcmdpbjogLTFweDsKICBvdmVyZmxvdzogaGlkZGVuOwogIGNsaXAtcGF0aDogaW5zZXQoNTAlKTsKICB3aGl0ZS1zcGFjZTogbm93cmFwOwogIGJvcmRlcjogMDsKfQ_22_7D__WEBPACK_IMPORTED_MODULE_1__);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FTriggersPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81ZUW_jOBB_31_hl5Ma3RFhCknKvtw_WTngJG6MTY2TkJz6309gCDbYQLLt6VRp2zX2eGb8zczn8fJXcMXrN_aDf34AsEXJcS_4iaUxOCPx4nm_4Bu_BeK2XvysvnORYhEDmJeg4JSkvVlYm_UJlJJTEQMY5GU1nKM0JWwfg8BXAykpcoquMdhRXA9Uv72UCJxIwlkMEk5PGau_7FEeA7hS6zLCvAMm_4OMgX9fVxwEYcd65PPHsjEL1mY9ulFY7dMJCexC3k_FJLurl3AmMZMxKHKUYG_L5QXjWhiiZM88InFWxCDBTGKhy321y61VMP0a3BYz5IW1vAyJPWGtZziTXkFuOAZBqLxXD10a9639elrCKRf9Tf2FLjwaFQ5flXCrIGgIWk0gbfM80jY9oK3yEkBfM7tRtsFRchJFpW3OiXLliCMAkAKxgii8IEqBvwwKgFGBDePiAz9jETMuX_KUFGhLcbqYsBhuTAfdF9breI4SIq8x8JeRrjXj0kOU8gtO9dXretHdBW3wjewOZzkc_iMetx97sHCgZAB5K7DbqPfNaNxMedO0h3GGLYqHPai85uXdmJmwaDyi2xeYqr6ZZ9FGoMRldXRkz3RjnaFjbtHLTej7t9gOHG76tPu_23OEHbAg8q5aihMukAqmE0uxoESJuFtyzzDa4k6nxDR7UwX69yJ99RjSe0hIR1J9I7muSRdRjVT_6qvx96XMVS8OIs2Ttvw_NzYcKRNGw5yJVc6ckyRnVardhKRM94NVXube6sPYaq8y9BmLHeUXr4wBOkn_Ux_7dmMZKu_8JWpJ0POFrlPjUKtxIak8VPnS_8M0kKK8wDXJqf_yHm0njPQKiK8hQk8rFO_kgE_sRvhEE33Oc9nqlm65lDybcsvlQCT2at5VZSEVNwDkvEVdIUlyvNaq87xJKjePsBSXMYC62e8jZp_xkCRBtDVd8vxhVWcA91irYEvd3RwaU1RILzkQmgKp0kpPD5WNuyVZO09jEmGkz2BmdtpSnhwHRxv5BrPmY2saZEEtj_xQRuh1QGxPbucovCjG6dXH10uquakCYVUR8e6adEWxq_12FuDSuYdqK9o6dT4m8s523NRO0O_kwsyQVExIuroFlYYgaXX1HPrWJ1r_0WGcJiw_uy0_GZafv8qFl3m81U01O1Hl80gLDEHXWtCAt8xC6e2htTpDkleKY0AkoiQxLuu_ls6eYe5T0Glvu0MSM8ZUVMOhqa5hOOxBBL1yDH3zBtful_IdOtG6aJqXOm0p_CrcwmAWaZxx0HCsT9G4w5EBRjgtDPv48ZfrSODMONxQu8HrLn8NnHz0KUIVDu8ONSRyJDCTjovMLHYLw1n01mwBwOgBDuuMlH47pL4lrYahYmsMjRG5j8WMKGoQ7vJKpHlFC4aNGQ0rE3l9ZgM3E9_fjITyWNfS73mvTW_PNhM3Q2gNs7WmM_qSbqNqiUokpKmIttF2pDeqx6x5TXVF_lM1BCZPFBGdIIsKrdoxKZ646lmafkEHwxZ_Fr0NnqM3YxwdD01LPOxp_NWPMWn3cMW_U_j2ftD3zv6pHnkDB0qKe0VvnWg0ifWGjrbpwb7pGJO0FPm_01dOcrmaisJ5Dxw7E77qrjxFeEavPhbOMkFt4bt7V0snCvaO_hePUQ1fAjBdDLOfqs39epo58PJCLCEdRMUVRfD_tqim_VMBc_PzemSNXy5y_UFgAMSm2mOWkcLn9nOCUIFInAmAHEUvBSdcYagr0K_bxULyZml21Q5wH41CY1XMGSU_R_0I6U1XNJNdimwwTR5EXZrIY8hs4N0fwLrINKmz9r3rRQC4Xyasuk9IaS34ie4G2jvGpYlAbnHqkO373lFwWtKl17sR2xvADab0VOUFq4h_7REluVwtWDLEr5pW_ka_uCKbLYP14NXwPKOJiLEknOuJlc77TjIotBkSCKX_zlW7QYrFrPgtnGDTN3xTSesiqBn_8CeXkeangfAAA_22_7D__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(6470);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FTriggersPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81ZUW_jOBB_31_hl5Ma3RFhCknKvtw_WTngJG6MTY2TkJz6309gCDbYQLLt6VRp2zX2eGb8zczn8fJXcMXrN_aDf34AsEXJcS_4iaUxOCPx4nm_4Bu_BeK2XvysvnORYhEDmJeg4JSkvVlYm_UJlJJTEQMY5GU1nKM0JWwfg8BXAykpcoquMdhRXA9Uv72UCJxIwlkMEk5PGau_7FEeA7hS6zLCvAMm_4OMgX9fVxwEYcd65PPHsjEL1mY9ulFY7dMJCexC3k_FJLurl3AmMZMxKHKUYG_L5QXjWhiiZM88InFWxCDBTGKhy321y61VMP0a3BYz5IW1vAyJPWGtZziTXkFuOAZBqLxXD10a9639elrCKRf9Tf2FLjwaFQ5flXCrIGgIWk0gbfM80jY9oK3yEkBfM7tRtsFRchJFpW3OiXLliCMAkAKxgii8IEqBvwwKgFGBDePiAz9jETMuX_KUFGhLcbqYsBhuTAfdF9breI4SIq8x8JeRrjXj0kOU8gtO9dXretHdBW3wjewOZzkc_iMetx97sHCgZAB5K7DbqPfNaNxMedO0h3GGLYqHPai85uXdmJmwaDyi2xeYqr6ZZ9FGoMRldXRkz3RjnaFjbtHLTej7t9gOHG76tPu_23OEHbAg8q5aihMukAqmE0uxoESJuFtyzzDa4k6nxDR7UwX69yJ99RjSe0hIR1J9I7muSRdRjVT_6qvx96XMVS8OIs2Ttvw_NzYcKRNGw5yJVc6ckyRnVardhKRM94NVXube6sPYaq8y9BmLHeUXr4wBOkn_Ux_7dmMZKu_8JWpJ0POFrlPjUKtxIak8VPnS_8M0kKK8wDXJqf_yHm0njPQKiK8hQk8rFO_kgE_sRvhEE33Oc9nqlm65lDybcsvlQCT2at5VZSEVNwDkvEVdIUlyvNaq87xJKjePsBSXMYC62e8jZp_xkCRBtDVd8vxhVWcA91irYEvd3RwaU1RILzkQmgKp0kpPD5WNuyVZO09jEmGkz2BmdtpSnhwHRxv5BrPmY2saZEEtj_xQRuh1QGxPbucovCjG6dXH10uquakCYVUR8e6adEWxq_12FuDSuYdqK9o6dT4m8s523NRO0O_kwsyQVExIuroFlYYgaXX1HPrWJ1r_0WGcJiw_uy0_GZafv8qFl3m81U01O1Hl80gLDEHXWtCAt8xC6e2htTpDkleKY0AkoiQxLuu_ls6eYe5T0Glvu0MSM8ZUVMOhqa5hOOxBBL1yDH3zBtful_IdOtG6aJqXOm0p_CrcwmAWaZxx0HCsT9G4w5EBRjgtDPv48ZfrSODMONxQu8HrLn8NnHz0KUIVDu8ONSRyJDCTjovMLHYLw1n01mwBwOgBDuuMlH47pL4lrYahYmsMjRG5j8WMKGoQ7vJKpHlFC4aNGQ0rE3l9ZgM3E9_fjITyWNfS73mvTW_PNhM3Q2gNs7WmM_qSbqNqiUokpKmIttF2pDeqx6x5TXVF_lM1BCZPFBGdIIsKrdoxKZ646lmafkEHwxZ_Fr0NnqM3YxwdD01LPOxp_NWPMWn3cMW_U_j2ftD3zv6pHnkDB0qKe0VvnWg0ifWGjrbpwb7pGJO0FPm_01dOcrmaisJ5Dxw7E77qrjxFeEavPhbOMkFt4bt7V0snCvaO_hePUQ1fAjBdDLOfqs39epo58PJCLCEdRMUVRfD_tqim_VMBc_PzemSNXy5y_UFgAMSm2mOWkcLn9nOCUIFInAmAHEUvBSdcYagr0K_bxULyZml21Q5wH41CY1XMGSU_R_0I6U1XNJNdimwwTR5EXZrIY8hs4N0fwLrINKmz9r3rRQC4Xyasuk9IaS34ie4G2jvGpYlAbnHqkO373lFwWtKl17sR2xvADab0VOUFq4h_7REluVwtWDLEr5pW_ka_uCKbLYP14NXwPKOJiLEknOuJlc77TjIotBkSCKX_zlW7QYrFrPgtnGDTN3xTSesiqBn_8CeXkeangfAAA_22_7D__WEBPACK_IMPORTED_MODULE_2___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FTriggersPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81ZUW_jOBB_31_hl5Ma3RFhCknKvtw_WTngJG6MTY2TkJz6309gCDbYQLLt6VRp2zX2eGb8zczn8fJXcMXrN_aDf34AsEXJcS_4iaUxOCPx4nm_4Bu_BeK2XvysvnORYhEDmJeg4JSkvVlYm_UJlJJTEQMY5GU1nKM0JWwfg8BXAykpcoquMdhRXA9Uv72UCJxIwlkMEk5PGau_7FEeA7hS6zLCvAMm_4OMgX9fVxwEYcd65PPHsjEL1mY9ulFY7dMJCexC3k_FJLurl3AmMZMxKHKUYG_L5QXjWhiiZM88InFWxCDBTGKhy321y61VMP0a3BYz5IW1vAyJPWGtZziTXkFuOAZBqLxXD10a9639elrCKRf9Tf2FLjwaFQ5flXCrIGgIWk0gbfM80jY9oK3yEkBfM7tRtsFRchJFpW3OiXLliCMAkAKxgii8IEqBvwwKgFGBDePiAz9jETMuX_KUFGhLcbqYsBhuTAfdF9breI4SIq8x8JeRrjXj0kOU8gtO9dXretHdBW3wjewOZzkc_iMetx97sHCgZAB5K7DbqPfNaNxMedO0h3GGLYqHPai85uXdmJmwaDyi2xeYqr6ZZ9FGoMRldXRkz3RjnaFjbtHLTej7t9gOHG76tPu_23OEHbAg8q5aihMukAqmE0uxoESJuFtyzzDa4k6nxDR7UwX69yJ99RjSe0hIR1J9I7muSRdRjVT_6qvx96XMVS8OIs2Ttvw_NzYcKRNGw5yJVc6ckyRnVardhKRM94NVXube6sPYaq8y9BmLHeUXr4wBOkn_Ux_7dmMZKu_8JWpJ0POFrlPjUKtxIak8VPnS_8M0kKK8wDXJqf_yHm0njPQKiK8hQk8rFO_kgE_sRvhEE33Oc9nqlm65lDybcsvlQCT2at5VZSEVNwDkvEVdIUlyvNaq87xJKjePsBSXMYC62e8jZp_xkCRBtDVd8vxhVWcA91irYEvd3RwaU1RILzkQmgKp0kpPD5WNuyVZO09jEmGkz2BmdtpSnhwHRxv5BrPmY2saZEEtj_xQRuh1QGxPbucovCjG6dXH10uquakCYVUR8e6adEWxq_12FuDSuYdqK9o6dT4m8s523NRO0O_kwsyQVExIuroFlYYgaXX1HPrWJ1r_0WGcJiw_uy0_GZafv8qFl3m81U01O1Hl80gLDEHXWtCAt8xC6e2htTpDkleKY0AkoiQxLuu_ls6eYe5T0Glvu0MSM8ZUVMOhqa5hOOxBBL1yDH3zBtful_IdOtG6aJqXOm0p_CrcwmAWaZxx0HCsT9G4w5EBRjgtDPv48ZfrSODMONxQu8HrLn8NnHz0KUIVDu8ONSRyJDCTjovMLHYLw1n01mwBwOgBDuuMlH47pL4lrYahYmsMjRG5j8WMKGoQ7vJKpHlFC4aNGQ0rE3l9ZgM3E9_fjITyWNfS73mvTW_PNhM3Q2gNs7WmM_qSbqNqiUokpKmIttF2pDeqx6x5TXVF_lM1BCZPFBGdIIsKrdoxKZ646lmafkEHwxZ_Fr0NnqM3YxwdD01LPOxp_NWPMWn3cMW_U_j2ftD3zv6pHnkDB0qKe0VvnWg0ifWGjrbpwb7pGJO0FPm_01dOcrmaisJ5Dxw7E77qrjxFeEavPhbOMkFt4bt7V0snCvaO_hePUQ1fAjBdDLOfqs39epo58PJCLCEdRMUVRfD_tqim_VMBc_PzemSNXy5y_UFgAMSm2mOWkcLn9nOCUIFInAmAHEUvBSdcYagr0K_bxULyZml21Q5wH41CY1XMGSU_R_0I6U1XNJNdimwwTR5EXZrIY8hs4N0fwLrINKmz9r3rRQC4Xyasuk9IaS34ie4G2jvGpYlAbnHqkO373lFwWtKl17sR2xvADab0VOUFq4h_7REluVwtWDLEr5pW_ka_uCKbLYP14NXwPKOJiLEknOuJlc77TjIotBkSCKX_zlW7QYrFrPgtnGDTN3xTSesiqBn_8CeXkeangfAAA_22_7D__WEBPACK_IMPORTED_MODULE_2__);



var addButton = '_2ye79n15';
var attributionBadge = '_2ye79n1m';
var card = '_2ye79n19';
var cardList = '_2ye79n18';
var cardMeta = '_2ye79n1b';
var cardTop = '_2ye79n1a';
var empty = '_2ye79na';
var emptyStateLink = '_2ye79nb';
var error = '_2ye79n7';
var header = '_2ye79n1';
var headerButtons = '_2ye79n3';
var headerButtonsHiddenOnMobile = '_2ye79n17';
var historyCounter = '_2ye79n1f';
var historyEntry = '_2ye79n1h';
var historyError = '_2ye79n1j';
var historyList = '_2ye79n1g';
var historySessionLink = '_2ye79n1k';
var historyTimestamp = '_2ye79n1i';
var historyToggle = '_2ye79n1d';
var historyWrapper = '_2ye79n1e';
var iconButton = '_2ye79n14';
var lastFired = '_2ye79ny';
var loading = '_2ye79n9';
var mobileAddFab = '_2ye79n16';
var neverFired = '_2ye79nz';
var panel = '_2ye79n0';
var refreshButton = '_2ye79n6';
var retryButton = '_2ye79n8';
var row = '_2ye79nl';
var rowActions = '_2ye79n13';
var rowCount = '_2ye79n1c';
var rowDisabled = '_2ye79nm';
var showNoMatchToggle = '_2ye79n1l';
var statusBadge = '_2ye79nt';
var statusFiredFailed = '_2ye79nv';
var statusFiredSuccess = '_2ye79nu';
var statusNoMatch = '_2ye79nx';
var statusRejected = '_2ye79nw';
var subtitle = '_2ye79n5';
var tab = '_2ye79ne';
var tabActive = '_2ye79nf';
var table = '_2ye79nh';
var tableWrapper = '_2ye79ng';
var tabs = '_2ye79nd';
var td = '_2ye79nj';
var tdCenter = '_2ye79nk';
var th = '_2ye79ni';
var title = '_2ye79n4';
var titleRow = '_2ye79n2';
var toggle = '_2ye79n10';
var toggleError = '_2ye79nc';
var toggleOff = '_2ye79n12';
var toggleOn = '_2ye79n11';
var triggerName = '_2ye79nn';
var triggerSlug = '_2ye79no';
var typeBadge = '_2ye79np';
var typeCron = '_2ye79nq';
var typeGithubPush = '_2ye79nr';
var typeWebhook = '_2ye79ns';
var visuallyHidden = '_9ji8bp0';

/***/ }),

/***/ 86061:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ })

},
/******/ __webpack_require__ => { // webpackRuntimeModules
/******/ var __webpack_exec__ = (moduleId) => (__webpack_require__(__webpack_require__.s = moduleId))
/******/ __webpack_require__.O(0, [1869,4202,9353,8182,7924,4069,8165,3575,2513,4247,2648,9691,3516,7358], () => (__webpack_exec__(4905)));
/******/ var __webpack_exports__ = __webpack_require__.O();
/******/ _N_E = __webpack_exports__;
/******/ }
]);
//# sourceMappingURL=page-2f5f76ef787d35b7.js.map