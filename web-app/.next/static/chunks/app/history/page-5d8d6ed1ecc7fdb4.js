(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[3429],{

/***/ 11272:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   O: () => (/* binding */ useSessionRepoPaths)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _lib_store__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(93602);
/* harmony import */ var _lib_store_sessionsSlice__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(60795);
/* __next_internal_client_entry_do_not_use__ useSessionRepoPaths auto */ 


/** Returns deduplicated list of repo root paths from all known sessions. */ function useSessionRepoPaths() {
    const sessions = (0,_lib_store__WEBPACK_IMPORTED_MODULE_1__/* .useAppSelector */ .GV)(_lib_store_sessionsSlice__WEBPACK_IMPORTED_MODULE_2__/* .selectActiveSessionsSortedByUpdatedAt */ .od);
    return (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>{
        const seen = new Set();
        const paths = [];
        for (const s of sessions){
            const p = s.repoRoot;
            if (p && !seen.has(p)) {
                seen.add(p);
                paths.push(p);
            }
        }
        return paths;
    }, [
        sessions
    ]);
}


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

/***/ 21694:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   O: () => (/* binding */ getKeepaliveConnectTransport),
/* harmony export */   VZ: () => (/* binding */ getWatchTransport),
/* harmony export */   qW: () => (/* binding */ getConnectTransport)
/* harmony export */ });
/* unused harmony export _resetTransportForTesting */
/* harmony import */ var _connectrpc_connect_web__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(46564);
/* harmony import */ var _lib_config__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(51202);
/* harmony import */ var _lib_transport_watch_ws_transport__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(90062);



let _transport = null;
let _watchTransport = null;
let _keepaliveTransport = null;
/**
 * Every unary RPC through this transport sends a Connect-Timeout-Ms header
 * with this value unless a call site passes its own timeoutMs — connect-go
 * reads that header and derives the handler's ctx deadline automatically, so
 * this is what makes "the backend can tell a caller disconnected" true for
 * ordinary RPCs (streamViaHub's WebSocket path uses connection-scoped
 * cancellation instead, since a live terminal stream has no fixed deadline).
 */ const DEFAULT_RPC_TIMEOUT_MS = 30000;
/**
 * Returns the shared ConnectRPC HTTP transport singleton.
 *
 * Every non-streaming hook should call this instead of constructing its own
 * transport. This ensures a single transport instance for all unary/streaming
 * non-watch RPCs, carrying the auth interceptor (redirects to /login on a
 * 401) in one place instead of every call site wiring it up separately.
 *
 * The streaming watch transport (createWatchTransport) is separate and should
 * NOT use this singleton — it has its own reconnect logic. Use
 * getWatchTransport() for that.
 */ function getConnectTransport() {
    if (!_transport) {
        _transport = (0,_connectrpc_connect_web__WEBPACK_IMPORTED_MODULE_2__/* .createConnectTransport */ .w)({
            baseUrl: (0,_lib_config__WEBPACK_IMPORTED_MODULE_0__/* .getApiBaseUrl */ .h)(),
            defaultTimeoutMs: DEFAULT_RPC_TIMEOUT_MS,
            interceptors: [
                (0,_lib_config__WEBPACK_IMPORTED_MODULE_0__/* .createAuthInterceptor */ ._)()
            ]
        });
    }
    return _transport;
}
/**
 * A unary transport whose requests use `fetch(..., { keepalive: true })`, so a
 * request started as the page hides or unloads still reaches the server. Bodies
 * must stay under the browser's 64KB keepalive limit; only use it for small
 * "complete this action now" RPCs (the tray's deferred clear).
 */ function getKeepaliveConnectTransport() {
    if (!_keepaliveTransport) {
        _keepaliveTransport = (0,_connectrpc_connect_web__WEBPACK_IMPORTED_MODULE_2__/* .createConnectTransport */ .w)({
            baseUrl: (0,_lib_config__WEBPACK_IMPORTED_MODULE_0__/* .getApiBaseUrl */ .h)(),
            defaultTimeoutMs: DEFAULT_RPC_TIMEOUT_MS,
            interceptors: [
                (0,_lib_config__WEBPACK_IMPORTED_MODULE_0__/* .createAuthInterceptor */ ._)()
            ],
            // Cast: Bun's global fetch type also requires `preconnect`, which a wrapper cannot provide (tsconfig.bun.json).
            fetch: (input, init)=>fetch(input, {
                    ...init,
                    keepalive: true
                })
        });
    }
    return _keepaliveTransport;
}
/**
 * Returns the shared transport singleton for server-streaming Watch* RPCs.
 *
 * Selects WebSocket-bridge vs. native ConnectRPC streaming per
 * createSessionWatchTransport's rules (see watch-ws-transport.ts) — every
 * Watch* hook should call this instead of constructing its own transport, so
 * that choice and the auth interceptor live in one place.
 */ function getWatchTransport() {
    if (!_watchTransport) {
        _watchTransport = (0,_lib_transport_watch_ws_transport__WEBPACK_IMPORTED_MODULE_1__/* .createSessionWatchTransport */ .ZX)({
            baseUrl: (0,_lib_config__WEBPACK_IMPORTED_MODULE_0__/* .getApiBaseUrl */ .h)(),
            interceptors: [
                (0,_lib_config__WEBPACK_IMPORTED_MODULE_0__/* .createAuthInterceptor */ ._)()
            ]
        });
    }
    return _watchTransport;
}
/** Reset the transport singletons (for testing only). */ function _resetTransportForTesting() {
    _transport = null;
    _watchTransport = null;
    _keepaliveTransport = null;
}


/***/ }),

/***/ 22479:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Y: () => (/* binding */ useDebouncedCallback),
/* harmony export */   d: () => (/* binding */ useDebounce)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);

/**
 * Hook that debounces a value by the specified delay
 * @param value - The value to debounce
 * @param delay - Delay in milliseconds
 * @returns The debounced value
 */ function useDebounce(value, delay) {
    const [debouncedValue, setDebouncedValue] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(value);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        const timer = setTimeout(()=>{
            setDebouncedValue(value);
        }, delay);
        return ()=>{
            clearTimeout(timer);
        };
    }, [
        value,
        delay
    ]);
    return debouncedValue;
}
/**
 * Hook that returns a debounced callback function
 * @param callback - The callback to debounce
 * @param delay - Delay in milliseconds
 * @returns Debounced callback function
 */ function useDebouncedCallback(callback, delay) {
    const timeoutIdRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    const debouncedCallback = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(function() {
        for(var _len = arguments.length, args = new Array(_len), _key = 0; _key < _len; _key++){
            args[_key] = arguments[_key];
        }
        if (timeoutIdRef.current) {
            clearTimeout(timeoutIdRef.current);
        }
        timeoutIdRef.current = setTimeout(()=>{
            timeoutIdRef.current = null;
            callback(...args);
        }, delay);
    }, [
        callback,
        delay
    ]);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        return ()=>{
            if (timeoutIdRef.current) {
                clearTimeout(timeoutIdRef.current);
            }
        };
    }, []);
    return debouncedCallback;
}


/***/ }),

/***/ 24044:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Bl: () => (/* binding */ AnalyticsContextProvider),
/* harmony export */   Ig: () => (/* binding */ AnalyticsContext),
/* harmony export */   st: () => (/* binding */ useAnalytics)
/* harmony export */ });
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(61574);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(55494);
/* __next_internal_client_entry_do_not_use__ AnalyticsContext,useAnalytics,AnalyticsContextProvider auto */ 

const AnalyticsContext = /*#__PURE__*/ (0,react__WEBPACK_IMPORTED_MODULE_1__.createContext)(null);
function useAnalytics() {
    const context = (0,react__WEBPACK_IMPORTED_MODULE_1__.useContext)(AnalyticsContext);
    if (!context) {
        throw new Error("useAnalytics must be used within an AnalyticsContextProvider");
    }
    return context;
}
function AnalyticsContextProvider(param) {
    let { provider, children } = param;
    // Stable ref to the provider so identity doesn't change across parent re-renders
    const providerRef = (0,react__WEBPACK_IMPORTED_MODULE_1__.useRef)(provider);
    providerRef.current = provider;
    (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        var _p_initialize;
        const p = provider;
        providerRef.current = p;
        void ((_p_initialize = p.initialize) === null || _p_initialize === void 0 ? void 0 : _p_initialize.call(p));
        return ()=>{
            var _p_onClose;
            void ((_p_onClose = p.onClose) === null || _p_onClose === void 0 ? void 0 : _p_onClose.call(p));
        };
    }, [
        provider
    ]);
    const contextValue = (0,react__WEBPACK_IMPORTED_MODULE_1__.useMemo)(()=>({
            provider: providerRef.current,
            track: (event)=>providerRef.current.track(event)
        }), // track delegates through the ref so consumers don't re-render on provider swap
    // eslint-disable-next-line react-hooks/exhaustive-deps
    []);
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(AnalyticsContext.Provider, {
        value: contextValue,
        children: children
    });
}


/***/ }),

/***/ 34609:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  E: () => (/* binding */ RepoPathInput)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/hooks/usePathCompletions.ts
var usePathCompletions = __webpack_require__(44762);
// EXTERNAL MODULE: ./src/lib/hooks/useSessionRepoPaths.ts
var useSessionRepoPaths = __webpack_require__(11272);
// EXTERNAL MODULE: ./src/lib/hooks/useGitHubEnterpriseHosts.ts
var useGitHubEnterpriseHosts = __webpack_require__(44635);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ./src/gen/session/v1/session_pb.ts
var session_pb = __webpack_require__(23575);
// EXTERNAL MODULE: ./src/lib/api/transport.ts
var transport = __webpack_require__(21694);
// EXTERNAL MODULE: ./src/lib/hooks/useAbortableEffect.ts
var useAbortableEffect = __webpack_require__(64343);
;// ./src/lib/hooks/useRepoPathSuggestions.ts
/* __next_internal_client_entry_do_not_use__ normalizePath,useRepoPathSuggestions auto */ 




const DEBOUNCE_MS = 150;
// Shorter than the server's own listWorktreesTimeout (20s, path_completion_service.go)
// so a genuinely stuck git subprocess surfaces as "unresolved" here well before
// the server would time it out itself.
const REQUEST_TIMEOUT_MS = 5000;
/** Strips a trailing slash so a candidate path matches its ListWorktrees entry regardless of how it was typed/stored. */ function normalizePath(p) {
    return p.length > 1 ? p.replace(/\/+$/, "") : p;
}
/** Merges one candidate path's resolved worktree family into `cache`. Returns true if anything new was added. */ function mergeWorktreeFamily(cache, worktrees) {
    const main = worktrees.find((w)=>w.isMain);
    if (!main) return false;
    const rootPath = normalizePath(main.path);
    let changed = false;
    for (const w of worktrees){
        const wp = normalizePath(w.path);
        if (!cache.has(wp)) {
            cache.set(wp, {
                rootPath,
                isMain: w.isMain,
                branch: w.branch
            });
            changed = true;
        }
    }
    return changed;
}
/**
 * Resolves one candidate, bounded by its own timeout so a stuck request
 * can't stay pending past REQUEST_TIMEOUT_MS regardless of what the outer
 * effect's shared `signal` does. Merges into `cache` and calls `onChange`
 * as soon as this specific request settles — independent of any sibling.
 */ async function resolveOnePath(client, repoPath, cache, outerSignal, onChange) {
    const controller = new AbortController();
    const onAbort = ()=>controller.abort();
    outerSignal.addEventListener("abort", onAbort);
    const timeoutTimer = setTimeout(()=>controller.abort(), REQUEST_TIMEOUT_MS);
    try {
        const result = await client.listWorktrees({
            repoPath
        }, {
            signal: controller.signal
        });
        if (mergeWorktreeFamily(cache, result.worktrees || [])) {
            onChange();
        }
    } catch (e) {
    // Aborted, timed out, or a network/server error — leave unresolved so
    // it's simply retried the next time it's a candidate.
    } finally{
        clearTimeout(timeoutTimer);
        outerSignal.removeEventListener("abort", onAbort);
    }
}
async function resolveUnresolvedPaths(candidates, cache, signal, onChange) {
    const unresolved = Array.from(new Set(candidates.map(normalizePath))).filter((p)=>p && !cache.has(p));
    if (unresolved.length === 0) return;
    const client = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
    await Promise.all(unresolved.map((repoPath)=>resolveOnePath(client, repoPath, cache, signal, onChange)));
}
function useRepoPathSuggestions(paths) {
    const [version, setVersion] = (0,react.useState)(0);
    const cacheRef = (0,react.useRef)(new Map());
    const [debounced, setDebounced] = (0,react.useState)([]);
    const key = paths.join("\n");
    (0,react.useEffect)(()=>{
        const timer = setTimeout(()=>setDebounced(paths), DEBOUNCE_MS);
        return ()=>clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        key
    ]);
    const debouncedKey = debounced.join("\n");
    (0,useAbortableEffect/* useAbortableEffect */.B)((signal)=>resolveUnresolvedPaths(debounced, cacheRef.current, signal, ()=>setVersion((v)=>v + 1)), // eslint-disable-next-line react-hooks/exhaustive-deps
    [
        debouncedKey
    ]);
    return {
        resolutions: cacheRef.current,
        version
    };
}

// EXTERNAL MODULE: ./src/components/ui/PathCompletionDropdown.tsx + 1 modules
var PathCompletionDropdown = __webpack_require__(81551);
// EXTERNAL MODULE: ./src/lib/github/urlParser.ts
var urlParser = __webpack_require__(11538);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fui%2FRepoPathInput.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61UwW7bMAy95yu4w4AEmw3JabZO_ZhCtmhbqCIKkpw4Hvbvg-Wkq521yaGwD4L4yEc96il_5o_dY2E8g98rAEdBR01WgEcjoz7g0wrgqFVsBXDGvj6t_qzySw5PObMogJNKadsIOEi_zrJn_guHwg_FsFnsbNlmxJeyemk8dVYtU7hMgIoM-atYOSWTV-gFcNdDIKPVAvbyBpV5qXQXlpW2PxOmJhuzoAe8ipf_4rXca3O6OlqXENRFoy0KsGSTbNFLe5Hz3EI6C7Cc7wKgDDh114_ESbMzrKQ-FcA-ZnRAXxs6CkBjtAs6zGcgaqq6kCbxlmTZpNm8crVSjeVY-grXL6UdBZkxCGdkhS0ZhT4R_X8k1U2hDsvKSgdZGlSpKjlZ6XgSwPIfafCdDyONpZhJY-iI6uMLo-bli5uicLaBL3rvyEdp4zz5Tl05u19YPu9v-76WxY07OTljL32jbRbJXeWf5lQP71PtP5lqt3hIZBnIdHFyxJheSVOtx9cCvo0ipfIG6yiAjUuvm_ayHjJtFfYCdozdY3e82-7zeT24Hvg4MN-Ucs2-w_nP-W6y9qsHW60U2o-vYfLPX3AcalpZBQAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FRepoPathInput_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61UwW7bMAy95yu4w4AEmw3JabZO_ZhCtmhbqCIKkpw4Hvbvg_Wkq521yaGwD4L4yEc96il_5o_dY2E8g98rAEdBR01WgEcjoz7g0wrgqFVsBXDGvj6t_qzySw5PObMogJNKadsIOEi_zrJn_guHwg_FsFnsbNlmxJeyemk8dVYtU7hMgIoM_atYOSWTV_gFcNdDIKPVAvbyBpV5qXQXlpW2PxOmJhuzoAe8ipf_4rXca3O6OlqXENRFoy0KsGSTbNFLe5Hz3EI6C7Cc7wKgDDh114_ESbMzrKQ_FcA_ZnRAXxs6CkBjtAs6zGcgaqq6kCbxlmTZpNm8crVSjeVY_grXL6UdBZkxCGdkhS0ZhT4R_X8k1U2hDsvKSgdZGlSpKjlZ6XgSwPIfafCdDyONpZhJY_iI6uMLo_bli5uicLaBL3rvyEdp4zz5Tl05u19YPu9v_76WxY07OTljL32jbRbJXeWf5lQP71PtP5lqt3hIZBnIdHFyxJheSVOtx9cCvo0ipfIG6yiAjUuvm_ayHjJtFfYCdozdY3e82_7zeT24Hvg4MN_Ucs2_w_nP_W6y9qsHW60U2o_vYfLPX3AcalpZBQAA_22_7D = __webpack_require__(12646);
;// ./src/components/ui/RepoPathInput.css.ts


var container = '_18u82lr0';
var dropdownWrapper = '_18u82lr5';
var githubHint = '_18u82lr4';
var RepoPathInput_css_hint = '_18u82lr3';
var input = '_18u82lr1';
var inputError = '_18u82lr2';
;// ./src/components/ui/RepoPathInput.tsx
/* __next_internal_client_entry_do_not_use__ RepoPathInput auto */ 








const MAX_HISTORY = 5;
function tildeAbbreviate(p) {
    const m = p.match(/^(\/(?:Users|home)\/[^/]+)(\/.*)?$/);
    var _m_;
    return m ? "~".concat((_m_ = m[2]) !== null && _m_ !== void 0 ? _m_ : "") : p;
}
function makeHistoryEntry(p, worktreeOf) {
    return {
        name: tildeAbbreviate(p),
        path: p,
        isDirectory: true,
        isHistory: true,
        ...worktreeOf ? {
            isWorktree: true,
            rootLabel: tildeAbbreviate(worktreeOf)
        } : {}
    };
}
function groupByResolvedRoot(paths, resolutions) {
    const groupOrder = [];
    const groups = new Map();
    for (const p of paths){
        const info = resolutions.get(normalizePath(p));
        const groupKey = info ? info.rootPath : normalizePath(p);
        let group = groups.get(groupKey);
        if (!group) {
            var _info_rootPath;
            group = {
                rootPath: null,
                discoveredRoot: (_info_rootPath = info === null || info === void 0 ? void 0 : info.rootPath) !== null && _info_rootPath !== void 0 ? _info_rootPath : null,
                members: []
            };
            groups.set(groupKey, group);
            groupOrder.push(groupKey);
        }
        if (!info || info.isMain) {
            group.rootPath = p;
        } else {
            group.members.push(p);
        }
    }
    return {
        groupOrder,
        groups
    };
}
function buildGroupedHistoryEntries(paths, resolutions) {
    const { groupOrder, groups } = groupByResolvedRoot(paths, resolutions);
    const entries = [];
    for (const groupKey of groupOrder){
        const group = groups.get(groupKey);
        if (!group) continue;
        if (group.rootPath) {
            entries.push(makeHistoryEntry(group.rootPath));
        } else if (group.discoveredRoot) {
            entries.push(makeHistoryEntry(group.discoveredRoot));
        }
        for (const member of group.members){
            const info = resolutions.get(normalizePath(member));
            entries.push(makeHistoryEntry(member, info === null || info === void 0 ? void 0 : info.rootPath));
        }
    }
    return entries;
}
function RepoPathInput(param) {
    let { id: idProp, value, onChange, disabled = false, error, placeholder = "/path/to/repo", required = false, hint, detectGitHubUrl = false, onSelect, "aria-label": ariaLabel, onEnter, "data-testid": testId } = param;
    const generatedId = (0,react.useId)();
    const id = idProp !== null && idProp !== void 0 ? idProp : generatedId;
    const listboxId = "".concat(id, "-listbox");
    const [open, setOpen] = (0,react.useState)(false);
    const [selectedIndex, setSelectedIndex] = (0,react.useState)(-1);
    const containerRef = (0,react.useRef)(null);
    const historyPaths = (0,useSessionRepoPaths/* useSessionRepoPaths */.O)();
    const { entries: fsEntries, isLoading } = (0,usePathCompletions/* usePathCompletions */.C)(value, {
        enabled: value.length > 0,
        directoriesOnly: true
    });
    const { hosts: enterpriseHosts } = (0,useGitHubEnterpriseHosts/* useGitHubEnterpriseHosts */.P)();
    const detectedRepo = (0,react.useMemo)(()=>{
        if (!detectGitHubUrl || !value.trim() || !(0,urlParser/* isGitHubRef */.Pu)(value, enterpriseHosts)) return null;
        return (0,urlParser/* parseGitHubRef */.GM)(value, enterpriseHosts);
    }, [
        detectGitHubUrl,
        value,
        enterpriseHosts
    ]);
    const historyCandidates = (0,react.useMemo)(()=>historyPaths.filter((p)=>value === "" || p.toLowerCase().includes(value.toLowerCase())).slice(0, MAX_HISTORY), [
        historyPaths,
        value
    ]);
    // `resolutions` is mutated in place as worktree families resolve in the
    // background, so `version` (not `resolutions` itself) is the signal that
    // grouping needs to be recomputed — see useRepoPathSuggestions' doc comment.
    const { resolutions, version } = useRepoPathSuggestions(historyCandidates);
    const { allEntries, historyCount } = (0,react.useMemo)(()=>{
        const historyEntries = buildGroupedHistoryEntries(historyCandidates, resolutions);
        const historySet = new Set(historyEntries.map((e)=>e.path));
        const fsCompletionEntries = fsEntries.filter((e)=>!historySet.has(e.path)).map((e)=>({
                name: e.name,
                path: e.path,
                isDirectory: e.isDirectory,
                isHistory: false
            }));
        return {
            allEntries: [
                ...historyEntries,
                ...fsCompletionEntries
            ],
            historyCount: historyEntries.length
        };
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        historyCandidates,
        resolutions,
        version,
        fsEntries
    ]);
    const showDropdown = open && (allEntries.length > 0 || isLoading);
    const handleSelect = (0,react.useCallback)((entry)=>{
        onChange(entry.path);
        onSelect === null || onSelect === void 0 ? void 0 : onSelect(entry);
        setOpen(false);
        setSelectedIndex(-1);
    }, [
        onChange,
        onSelect
    ]);
    const handleKeyDown = (0,react.useCallback)((e)=>{
        // Focus opens the list, so Tab must close it or it stays over the next control.
        if (e.key === "Tab") {
            setOpen(false);
            setSelectedIndex(-1);
            return;
        }
        if (e.key === "Enter" && onEnter && !e.nativeEvent.isComposing && e.keyCode !== 229 && // Safari fires the IME-confirm Enter after compositionend with keyCode 229
        !(open && selectedIndex >= 0 && selectedIndex < allEntries.length)) {
            e.preventDefault();
            setOpen(false);
            onEnter();
            return;
        }
        if (!open) {
            if (e.key === "ArrowDown" || e.key === "ArrowUp") {
                setOpen(true);
            }
            return;
        }
        switch(e.key){
            case "ArrowDown":
                e.preventDefault();
                setSelectedIndex((i)=>Math.min(i + 1, allEntries.length - 1));
                break;
            case "ArrowUp":
                e.preventDefault();
                setSelectedIndex((i)=>Math.max(i - 1, -1));
                break;
            case "Enter":
                if (selectedIndex >= 0 && selectedIndex < allEntries.length) {
                    e.preventDefault();
                    handleSelect(allEntries[selectedIndex]);
                }
                break;
            case "Escape":
                if (showDropdown) {
                    // Both calls are required: `e.stopPropagation()` sets React's own
                    // `isPropagationStopped` flag, which is what stops the synthetic bubble
                    // from reaching an ancestor's own `onKeyDown` (e.g. Omnibar's modal div);
                    // `e.nativeEvent.stopImmediatePropagation()` additionally stops the raw
                    // native event from reaching a real `document.addEventListener("keydown", ...)`
                    // ancestor (e.g. NewShellDialog's cancel-on-Escape listener). Neither call
                    // alone covers both ancestor shapes — see RepoPathInput.test.tsx's
                    // "does not bubble to a parent's own keydown handler" case. Note:
                    // stopImmediatePropagation also blocks any OTHER listener on this same
                    // input for this event, not just ancestors — fine today (only one keydown
                    // listener lives on this node) but worth remembering before adding a second.
                    e.stopPropagation();
                    e.nativeEvent.stopImmediatePropagation();
                    // Also cancel the default so a native <dialog> doesn't close on this Escape.
                    e.preventDefault();
                }
                setOpen(false);
                setSelectedIndex(-1);
                break;
        }
    }, [
        open,
        allEntries,
        selectedIndex,
        handleSelect,
        showDropdown,
        onEnter
    ]);
    (0,react.useEffect)(()=>{
        function handleClickOutside(e) {
            if (containerRef.current && !containerRef.current.contains(e.target)) {
                setOpen(false);
                setSelectedIndex(-1);
            }
        }
        document.addEventListener("mousedown", handleClickOutside);
        return ()=>document.removeEventListener("mousedown", handleClickOutside);
    }, []);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        ref: containerRef,
        className: container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                id: id,
                type: "text",
                className: [
                    input,
                    error ? inputError : ""
                ].filter(Boolean).join(" "),
                value: value,
                onChange: (e)=>{
                    onChange(e.target.value);
                    setOpen(true);
                    setSelectedIndex(-1);
                },
                onFocus: ()=>setOpen(true),
                onKeyDown: handleKeyDown,
                placeholder: placeholder,
                title: value || undefined,
                required: required,
                "aria-required": required || undefined,
                "aria-invalid": error ? true : undefined,
                role: "combobox",
                "aria-haspopup": "listbox",
                "aria-expanded": showDropdown,
                "aria-autocomplete": "list",
                "aria-controls": showDropdown ? listboxId : undefined,
                "aria-activedescendant": showDropdown && selectedIndex >= 0 ? "".concat(listboxId, "-option-").concat(selectedIndex) : undefined,
                "aria-describedby": error ? "".concat(id, "-error") : undefined,
                "aria-label": ariaLabel,
                disabled: disabled,
                "data-testid": testId,
                autoComplete: "off",
                spellCheck: false
            }),
            showDropdown && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: dropdownWrapper,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(PathCompletionDropdown/* PathCompletionDropdown */.h, {
                    id: listboxId,
                    entries: allEntries,
                    selectedIndex: selectedIndex,
                    onSelect: handleSelect,
                    isLoading: isLoading,
                    historyCount: historyCount
                })
            }),
            detectedRepo ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                className: githubHint,
                "data-testid": "repo-path-github-hint",
                children: [
                    "Will clone ",
                    (0,urlParser/* getRepoFullName */.p2)(detectedRepo),
                    " to",
                    " ",
                    "~/.stapler-squad/repos/".concat(detectedRepo.host || "github.com", "/").concat(detectedRepo.owner, "/").concat(detectedRepo.repo),
                    " ",
                    "when you save."
                ]
            }) : hint && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: RepoPathInput_css_hint,
                children: hint
            })
        ]
    });
}


/***/ }),

/***/ 41078:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   KA: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.useRouter),
/* harmony export */   N_: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.Link),
/* harmony export */   a8: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.usePathname),
/* harmony export */   nQ: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.replaceHash),
/* harmony export */   ok: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.useSearchParams),
/* harmony export */   tk: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.pushHistoryMarker),
/* harmony export */   wV: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.useReplaceSearch),
/* harmony export */   x7: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.pushTrayEntry)
/* harmony export */ });
/* harmony import */ var _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(99863);
// `@/lib/nav/impl` is aliased per build (next.config.ts, tsconfig paths, jest moduleNameMapper); everything outside this directory imports from here.




/***/ }),

/***/ 42547:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  "default": () => (/* binding */ HistoryBrowserPage)
});

// NAMESPACE OBJECT: ./src/components/history/HistoryEntryCard.css.ts
var HistoryEntryCard_css_namespaceObject = {};
__webpack_require__.r(HistoryEntryCard_css_namespaceObject);
__webpack_require__.d(HistoryEntryCard_css_namespaceObject, {
  entryBranch: () => (entryBranch),
  entryCard: () => (entryCard),
  entryDirty: () => (entryDirty),
  entryDivider: () => (entryDivider),
  entryHeader: () => (entryHeader),
  entryMessages: () => (entryMessages),
  entryMeta: () => (entryMeta),
  entryModel: () => (entryModel),
  entryName: () => (entryName),
  entryProject: () => (entryProject),
  entryTime: () => (entryTime),
  expandButton: () => (expandButton),
  expandRow: () => (expandRow),
  headerRight: () => (headerRight),
  selected: () => (HistoryEntryCard_css_selected),
  statusDot: () => (statusDot),
  statusPill: () => (statusPill),
  status_creating: () => (status_creating),
  status_paused: () => (status_paused),
  status_running: () => (status_running),
  status_stopped: () => (status_stopped)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/analytics/usePageView.ts
var usePageView = __webpack_require__(90814);
// EXTERNAL MODULE: ./src/lib/nav/index.ts
var nav = __webpack_require__(41078);
// EXTERNAL MODULE: ./src/gen/session/v1/session_pb.ts
var session_pb = __webpack_require__(23575);
// EXTERNAL MODULE: ./src/gen/session/v1/types_pb.ts
var types_pb = __webpack_require__(81432);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect-web@2.1.1+0149f91553ca585e/node_modules/@connectrpc/connect-web/dist/esm/connect-transport.js + 8 modules
var connect_transport = __webpack_require__(46564);
// EXTERNAL MODULE: ./src/lib/config.ts
var config = __webpack_require__(51202);
;// ./src/lib/hooks/useSearchHistory.ts
/* __next_internal_client_entry_do_not_use__ useSearchHistory,default auto */ 
const STORAGE_KEY = 'stapler-squad-logs-search-history';
const MAX_HISTORY = 10;
function useSearchHistory() {
    let options = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : {};
    const { maxEntries = MAX_HISTORY, storageKey = STORAGE_KEY } = options;
    const [history, setHistory] = (0,react.useState)([]);
    const [isLoaded, setIsLoaded] = (0,react.useState)(false);
    // Load history from localStorage on mount
    (0,react.useEffect)(()=>{
        try {
            const stored = localStorage.getItem(storageKey);
            if (stored) {
                const parsed = JSON.parse(stored);
                if (Array.isArray(parsed)) {
                    setHistory(parsed);
                }
            }
        } catch (err) {
            console.error('Failed to load search history:', err);
        }
        setIsLoaded(true);
    }, [
        storageKey
    ]);
    // Save history to localStorage
    const saveHistory = (0,react.useCallback)((entries)=>{
        try {
            localStorage.setItem(storageKey, JSON.stringify(entries));
        } catch (err) {
            console.error('Failed to save search history:', err);
        }
    }, [
        storageKey
    ]);
    // Add a query to history
    const addToHistory = (0,react.useCallback)((query)=>{
        if (!query.trim()) return;
        const normalizedQuery = query.trim();
        setHistory((prev)=>{
            // Remove duplicate if exists
            const filtered = prev.filter((entry)=>entry.query !== normalizedQuery);
            // Add new entry at the beginning
            const newHistory = [
                {
                    query: normalizedQuery,
                    timestamp: Date.now()
                },
                ...filtered
            ].slice(0, maxEntries);
            saveHistory(newHistory);
            return newHistory;
        });
    }, [
        maxEntries,
        saveHistory
    ]);
    // Remove a query from history
    const removeFromHistory = (0,react.useCallback)((query)=>{
        setHistory((prev)=>{
            const filtered = prev.filter((entry)=>entry.query !== query);
            saveHistory(filtered);
            return filtered;
        });
    }, [
        saveHistory
    ]);
    // Clear all history
    const clearHistory = (0,react.useCallback)(()=>{
        setHistory([]);
        try {
            localStorage.removeItem(storageKey);
        } catch (err) {
            console.error('Failed to clear search history:', err);
        }
    }, [
        storageKey
    ]);
    return {
        history,
        isLoaded,
        addToHistory,
        removeFromHistory,
        clearHistory
    };
}
/* harmony default export */ const hooks_useSearchHistory = ((/* unused pure expression or super */ null && (useSearchHistory)));

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fhistory%2FHistorySearchInput.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA81W3Y6iMBS-n6c4N5toVkzBn9HOzb7JpECRjrVt2jKim333TQsoMIrMZHez0USg7Tnn-zkHf-zpKdPkQA28hrl52-dLBD-fAKz0PwBWE2EyqQ8YtLTE0slijVK6m748Afx6-vU0bw6G_oSShlkmBQZNObHsnbqNR5baHEOI0LeX9plo4EzKjOLkhCHjtHQPCGc7ETBLDwZDQoWluhNs0QtGYiN5YX0wTjOLIYyUj5RILjWGd6InQfAabuk50udwOj4rwFthLMtOQSKFpcK2lxqw6ypZTtkut819q96lr_eSkAnOBA1iLpN9O8yyF6a-j6VOqcYQqRKM5CztwaHT667AShXcBH1o79IkZYXBsHIqARDBDqSi8moONN8YcIUSDUxkTDBHcAvVyqPqCA6gSJoysXMPVAmLtSqrqyWqwGRS2MCwM73i82RcQM9Xd3VDFQSS7HdaFiK9DfR52iYtHENaQ8emKkgW1tWEQUjhPeVbo_ZafcSnBjQPVwYoMXQGsSwDk5NUHluPu4ThTCaF8bS1w9yTqomHAfnPQpWgdzGZrLYzCBdoBtFyPXPZpr08WHGS0FzylGqf7maeqHtsPdBVulanVvHP9U6EuqZv7i8-Qg_a-GoHXOmkiKbCtj3QyNhTu7ZfUmjjgivJmtraet8QumfAe3KvcS7fh_gf5-dNV6XnAZWsVBgSwpOJa0f47iBOr0MRtZT01-eAiZSWbg78m87qenrphkPUmBrNoP7OK2UPpLyMhQWqfeEYzbg8BicMpLCyw82mO2cbc35woVEkoUFM7ZFSMWDfiwk3daX9Cfb4LVPzEEtr5WGYtBaQbSVyk95xtP6YPXyc_b9tju2j5gi_1B2kS5xzGOpsiG9b5I4BdkRdvHvTDA_4GctG3GLjk4iTTwEa8Wdm0_sX0od_fyBHf81zUpGE2VOdvM1wvdLx4CDVySjj3dCmx_ilpLCzO_2KhnT4HT1eXrccmFwzscc942c-h9vgS-5OksXAJKn6sJm7GHKWptXUtLS0wXWBcs6UYcabKWeWBn7OOrmPmqhONbuqmpHDrCLhA7jfvbppbdQMAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fhistory_2FHistorySearchInput_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81W3Y6iMBS_n6c4N5toVkzBn9HOzb7JpECRjrVt2jKim333TQsoMIrMZHez0USg7Tnn_zkHf_zpKdPkQA28hrl52_dLBD_fAKz0PwBWE2EyqQ8YtLTE0slijVK6m748Afx6_vU0bw6G_oSShlkmBQZNObHsnbqNR5baHEOI0LeX9plo4EzKjOLkhCHjtHQPCGc7ETBLDwZDQoWluhNs0QtGYiN5YX0wTjOLIYyUj5RILjWGd6InQfAabuk50udwOj4rwFthLMtOQSKFpcK2lxqw6ypZTtkut819q96lr_eSkAnOBA1iLpN9O8yyF6a_j6VOqcYQqRKM5CztwaHT667AShXcBH1o79IkZYXBsHIqARDBDqSi8moONN8YcIUSDUxkTDBHcAvVyqPqCA6gSJoysXMPVAmLtSqrqyWqwGRS2MCwM73i82RcQM9Xd3VDFQSS7HdaFiK9DfR52iYtHENaQ8emKkgW1tWEQUjhPeVbo_ZafcSnBjQPVwYoMXQGsSwDk5NUHluPu4ThTCaF8bS1w9yTqomHAfnPQpWgdzGZrLYzCBdoBtFyPXPZpr08WHGS0FzylGqf7maeqHtsPdBVulanVvHP9U6EuqZv7i8_Qg_a_GoHXOmkiKbCtj3QyNhTu7ZfUmjjgivJmtraet8QumfAe3KvcS7fh_gf5_dNV6XnAZWsVBgSwpOJa0f47iBOr0MRtZT01_eAiZSWbg78m87qenrphkPUmBrNoP7OK2UPpLyMhQWqfeEYzbg8BicMpLCyw82mO2cbc35woVEkoUFM7ZFSMWDfiwk3daX9Cfb4LVPzEEtr5WGYtBaQbSVyk95xtP6YPXyc_b9tju2j5gi_1B2kS5xzGOpsiG9b5I4BdkRdvHvTDA_4GctG3GLjk4iTTwEa8Wdm0_sX0od_fyBHf81zUpGE2VOdvM1wvdLx4CDVySjj3dCmx_ilpLCzO_2KhnT4HT1eXrccmFwzscc942c_h9vgS_5OksXAJKn6sJm7GHKWptXUtLS0wXWBcs6UYcabKWeWBn7OOrmPmqhONbuqmpHDrCLhA7jfvbppbdQMAAA_22_7D = __webpack_require__(39693);
;// ./src/components/history/HistorySearchInput.css.ts


var clearAllButton = '_1hsjkh49';
var clearButton = '_1hsjkh46';
var container = '_1hsjkh41';
var dropdown = '_1hsjkh47';
var header = '_1hsjkh48';
var historyIcon = '_1hsjkh4e';
var input = '_1hsjkh45';
var inputWrapper = '_1hsjkh42';
var item = '_1hsjkh4b';
var items = '_1hsjkh4a';
var query = '_1hsjkh4f';
var removeButton = '_1hsjkh4c';
var searchIcon = '_1hsjkh43';
var selected = '_1hsjkh4d';
var spinner = '_1hsjkh44';
var timestamp = '_1hsjkh4g';
;// ./src/components/history/HistorySearchInput.tsx
/* __next_internal_client_entry_do_not_use__ HistorySearchInput auto */ 



function HistorySearchInput(param) {
    let { value, onChange, onSubmit, placeholder = "Search conversation history...", loading = false, id = "history-search", className } = param;
    const [showHistory, setShowHistory] = (0,react.useState)(false);
    const [selectedIndex, setSelectedIndex] = (0,react.useState)(-1);
    const containerRef = (0,react.useRef)(null);
    const inputRef = (0,react.useRef)(null);
    const { history, addToHistory, removeFromHistory, clearHistory } = useSearchHistory({
        storageKey: "stapler-squad-history-search",
        maxEntries: 15
    });
    // Close dropdown when clicking outside
    (0,react.useEffect)(()=>{
        const handleClickOutside = (event)=>{
            if (containerRef.current && !containerRef.current.contains(event.target)) {
                setShowHistory(false);
            }
        };
        document.addEventListener("mousedown", handleClickOutside);
        return ()=>document.removeEventListener("mousedown", handleClickOutside);
    }, []);
    const handleFocus = ()=>{
        if (history.length > 0 && !value) {
            setShowHistory(true);
        }
    };
    const handleChange = (e)=>{
        const newValue = e.target.value;
        onChange(newValue);
        setSelectedIndex(-1);
        if (!newValue && history.length > 0) {
            setShowHistory(true);
        } else {
            setShowHistory(false);
        }
    };
    const handleSelect = (0,react.useCallback)((query)=>{
        onChange(query);
        setShowHistory(false);
        setSelectedIndex(-1);
        addToHistory(query);
        onSubmit === null || onSubmit === void 0 ? void 0 : onSubmit(query);
    }, [
        onChange,
        onSubmit,
        addToHistory
    ]);
    const handleKeyDown = (e)=>{
        if (!showHistory || history.length === 0) {
            if (e.key === "Enter" && value.trim()) {
                addToHistory(value);
                onSubmit === null || onSubmit === void 0 ? void 0 : onSubmit(value);
            }
            return;
        }
        switch(e.key){
            case "ArrowDown":
                e.preventDefault();
                setSelectedIndex((prev)=>prev < history.length - 1 ? prev + 1 : 0);
                break;
            case "ArrowUp":
                e.preventDefault();
                setSelectedIndex((prev)=>prev > 0 ? prev - 1 : history.length - 1);
                break;
            case "Enter":
                e.preventDefault();
                if (selectedIndex >= 0 && selectedIndex < history.length) {
                    handleSelect(history[selectedIndex].query);
                } else if (value.trim()) {
                    addToHistory(value);
                    onSubmit === null || onSubmit === void 0 ? void 0 : onSubmit(value);
                    setShowHistory(false);
                }
                break;
            case "Escape":
                setShowHistory(false);
                setSelectedIndex(-1);
                break;
        }
    };
    const handleRemove = (e, query)=>{
        e.stopPropagation();
        removeFromHistory(query);
    };
    const formatTime = (timestamp)=>{
        const diff = Date.now() - timestamp;
        const minutes = Math.floor(diff / 60000);
        const hours = Math.floor(diff / 3600000);
        const days = Math.floor(diff / 86400000);
        if (minutes < 1) return "just now";
        if (minutes < 60) return "".concat(minutes, "m ago");
        if (hours < 24) return "".concat(hours, "h ago");
        return "".concat(days, "d ago");
    };
    const handleClear = ()=>{
        var _inputRef_current;
        onChange("");
        setShowHistory(history.length > 0);
        (_inputRef_current = inputRef.current) === null || _inputRef_current === void 0 ? void 0 : _inputRef_current.focus();
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: "".concat(container, " ").concat(className || ""),
        ref: containerRef,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: inputWrapper,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: searchIcon,
                        children: loading ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: spinner
                        }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("svg", {
                            width: "16",
                            height: "16",
                            viewBox: "0 0 24 24",
                            fill: "none",
                            stroke: "currentColor",
                            strokeWidth: "2",
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("circle", {
                                    cx: "11",
                                    cy: "11",
                                    r: "8"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("path", {
                                    d: "M21 21l-4.35-4.35"
                                })
                            ]
                        })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        ref: inputRef,
                        id: id,
                        type: "text",
                        value: value,
                        onChange: handleChange,
                        onFocus: handleFocus,
                        onKeyDown: handleKeyDown,
                        placeholder: placeholder,
                        className: input,
                        role: "combobox",
                        "aria-label": "Search conversation history",
                        "aria-expanded": showHistory,
                        "aria-controls": "search-history-dropdown",
                        "aria-autocomplete": "list",
                        autoComplete: "off"
                    }),
                    value && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: clearButton,
                        onClick: handleClear,
                        "aria-label": "Clear search",
                        type: "button",
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("svg", {
                            width: "14",
                            height: "14",
                            viewBox: "0 0 24 24",
                            fill: "none",
                            stroke: "currentColor",
                            strokeWidth: "2",
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("path", {
                                d: "M18 6L6 18M6 6l12 12"
                            })
                        })
                    })
                ]
            }),
            showHistory && history.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                id: "search-history-dropdown",
                className: dropdown,
                role: "listbox",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: header,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                children: "Recent Searches"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: clearAllButton,
                                onClick: ()=>{
                                    clearHistory();
                                    setShowHistory(false);
                                },
                                type: "button",
                                children: "Clear all"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: items,
                        children: history.map((entry, index)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: "".concat(item, " ").concat(index === selectedIndex ? selected : ""),
                                onClick: ()=>handleSelect(entry.query),
                                role: "option",
                                "aria-selected": index === selectedIndex,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: historyIcon,
                                        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("svg", {
                                            width: "12",
                                            height: "12",
                                            viewBox: "0 0 24 24",
                                            fill: "none",
                                            stroke: "currentColor",
                                            strokeWidth: "2",
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("circle", {
                                                    cx: "12",
                                                    cy: "12",
                                                    r: "10"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("polyline", {
                                                    points: "12 6 12 12 16 14"
                                                })
                                            ]
                                        })
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: query,
                                        children: entry.query
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: timestamp,
                                        children: formatTime(entry.timestamp)
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        className: removeButton,
                                        onClick: (e)=>handleRemove(e, entry.query),
                                        "aria-label": 'Remove "'.concat(entry.query, '" from history'),
                                        type: "button",
                                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("svg", {
                                            width: "12",
                                            height: "12",
                                            viewBox: "0 0 24 24",
                                            fill: "none",
                                            stroke: "currentColor",
                                            strokeWidth: "2",
                                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("path", {
                                                d: "M18 6L6 18M6 6l12 12"
                                            })
                                        })
                                    })
                                ]
                            }, entry.query))
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fhistory%2FHistorySearchResults.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA81X226rOBR971fsl5EaKUSGhDRxXo40H3JkwCQ-AZtjmybpqP8-soEEO5BLe6QZtRKtwWtv77X2xT_29JRLUlIFP0OG1qdVhuCfFwAt7ANAS8JVLmSJQQpNNH2dL1FGt5PNC8Dny-fLrNsY2h0ZU1VBThjygh7NN-YZZEzSVDPBMaSiqEtu3mxJhSFcVsdNHyYahiEF2_KAaVoqDCnlmsozxqqyn1Qkyxjf2v8BmZVEyIzKIBFaixJDWB1BiYJl8E7kaxD8DNf0I5IfdOJ4MLce5ILrQLEPiiFcNAbs0oGy7U5jiJE1kYpCSOwBIhdwcQUYNYCDmyN3c_y1sEZeWJcW5hyiJuwACUn3WylqngWDzrxNLnG8HcBztCXJWK3OrKS1VAa3EqwjzWqKNX63m6xxQLMwVkCJolNIxDFQO5KJQ2_ZPRLeiXcq7cH6MP4Zyta7Dg8Dgqg6WpnIbUJe0RTa3xmKJ56NXKS1sjZErQvGKQYuON18waj5mXdG4_UUwjmaQrRYGsuha_jty2lQErll_Cz6lSeE1ZUY4wF1L--ouzODm0Qz7mEIzZ-GkrwwB96xLKNWkpoedXB5QYuCVYop8-qwY5oGqiKpDexBkspxd-3qtuVt4x0hfDJBW3auxD_KzJW6uxS2eah2kvG9jUXPdfI8hR2qx-HZ2IMlxNNS8rwji86PY3Bgmd5hiBBq1v4wwQmo922jSS-UAKIiKdMnDGj25mxK7YZHymdmv2zjWdBcYyC1FhtfVMtRUQ1aqW8JKFxOIVzFUwij9aiAFl5e0i_VeT-782GU0TbZyeheH1gNHWEgZvNmydTJYNcmYjiLHR_H2L6X0G1NslrrjSV1VVGZ2uZwQxIAJeOdlhd-2HbjemrIOwiZBYmkZI_BPgKz4mCwphENSyKKTT1BiylEoZHEYnKnvF46dXWEjiQ3_H6L_zXM_a9aaZafglRwTbnup7ozDnjVa__9OSxExnd0rZJn56mxcm1VUBFJuX5kTCm_MaZ4tp1RxV9x49hMKZgL_YozpkhS0GwyopUhf9tXto56yB1cM55ciuWyfyIudECKQhyoq9dibCrtAa2cHaUrCcZtmieFSPc2R5rk6tg9p_-ir96m2jbspLU01P1tDthjRovqJsFn6mL0l5UkZyVpaLpcYtBspWwZIhIYzxn3o8efrbcjyn8kvUy9gaiNg61fFuzybc8vcbu1Xc8G_gWquuJ1-WS6eQMeoKs283vsinSjhPYGxh6S_K9oGL4mNZN-q-R55Cp5HrlKnj9yHeqp-YFq9C1J6_9BLO_qu77Rar178_tzKru6l3z-C2rZbQLfEAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fhistory_2FHistorySearchResults_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81X226rOBR971fsl5EaKUSGhDRxXo40H3JkwCQ_AZtjmybpqP8_soEEO5BLe6QZtRKtwWtv77X2xT_29JRLUlIFP0OG1qdVhuCfFwAt7ANAS8JVLmSJQQpNNH2dL1FGt5PNC8Dny_fLrNsY2h0ZU1VBThjygh7NN_YZZEzSVDPBMaSiqEtu3mxJhSFcVsdNHyYahiEF2_KAaVoqDCnlmsozxqqyn1Qkyxjf2v8BmZVEyIzKIBFaixJDWB1BiYJl8E7kaxD8DNf0I5IfdOJ4MLce5ILrQLEPiiFcNAbs0oGy7U5jiJE1kYpCSOwBIhdwcQUYNYCDmyN3c_y1sEZeWJcW5hyiJuwACUn3WylqngWDzrxNLnG8HcBztCXJWK3OrKS1VAa3EqwjzWqKNX63m6xxQLMwVkCJolNIxDFQO5KJQ2_ZPRLeiXcq7cH6MP4Zyta7Dg8Dgqg6WpnIbUJe0RTa3xmKJ56NXKS1sjZErQvGKQYuON18waj5mXdG4_UUwjmaQrRYGsuha_jty2lQErll_Cz6lSeE1ZUY4wF1L_ouzODm0Qz7mEIzZ_GkrwwB96xLKNWkpoedXB5QYuCVYop8_qwY5oGqiKpDexBkspxd_3qtuVt4x0hfDJBW3auxD_KzJW6uxS2eah2kvG9jUXPdfI8hR2qx_HZ2IMlxNNS8rwji86PY3Bgmd5hiBBq1v4wwQmo922jSS_UAKIiKdMnDGj25mxK7YZHymdmv2zjWdBcYyC1FhtfVMtRUQ1aqW8JKFxOIVzFUwij9aiAFl5e0i_VeT_782GU0TbZyeheH1gNHWEgZvNmydTJYNcmYjiLHR_H2L6X0G1NslrrjSV1VVGZ2uZwQxIAJeOdlhd_2HbjemrIOwiZBYmkZI_BPgKz4mCwphENSyKKTT1BiylEoZHEYnKnvF46dXWEjiQ3_H6L_zXM_a9aaZafglRwTbnup7ozDnjVa_9OSxExnd0rZJn56mxcm1VUBFJuX5kTCm_MaZ4tp1RxV9x49hMKZgL_YozpkhS0GwyopUhf9tXto56yB1cM55ciuWyfyIudECKQhyoq9dibCrtAa2cHaUrCcZtmieFSPc2R5rk6tg9p_ir96m2jbspLU01P1tDthjRovqJsFn6mL0l5UkZyVpaLpcYtBspWwZIhIYzxn3o8efrbcjyn8kvUy9gaiNg61fFuzybc8vcbu1Xc8G_gWquuJ1_WS6eQMeoKs283vsinSjhPYGxh6S_K9oGL4mNZN_q_R55Cp5HrlKnj9yHeqp_YFq9C1J6_9BLO_qu77Rar178_tzKru6l3z_C2rZbQLfEAAA_22_7D = __webpack_require__(53254);
;// ./src/components/history/HistorySearchResults.css.ts


var buttonSpinner = '_1i09y8dm';
var HistorySearchResults_css_container = '_1i09y8d1';
var date = '_1i09y8dc';
var emptyHint = '_1i09y8dq';
var emptyIcon = '_1i09y8do';
var emptyState = '_1i09y8dn';
var emptyText = '_1i09y8dp';
var errorIcon = '_1i09y8du';
var errorState = '_1i09y8dt';
var errorText = '_1i09y8dv';
var HistorySearchResults_css_header = '_1i09y8d2';
var highlight = '_1i09y8di';
var loadMoreButton = '_1i09y8dk';
var loadMoreContainer = '_1i09y8dj';
var loadingSpinner = '_1i09y8ds';
var loadingState = '_1i09y8dr';
var modelBadge = '_1i09y8d9';
var projectPath = '_1i09y8db';
var queryTime = '_1i09y8d4';
var remainingCount = '_1i09y8dl';
var resultCard = '_1i09y8d6';
var resultCount = '_1i09y8d3';
var resultHeader = '_1i09y8d7';
var resultMeta = '_1i09y8da';
var HistorySearchResults_css_results = '_1i09y8d5';
var score = '_1i09y8dd';
var sessionName = '_1i09y8d8';
var HistorySearchResults_css_snippet = '_1i09y8df';
var snippetRole = '_1i09y8dg';
var snippetText = '_1i09y8dh';
var snippets = '_1i09y8de';
;// ./src/components/history/HistorySearchResults.tsx
/* __next_internal_client_entry_do_not_use__ HistorySearchResults auto */ 


/**
 * Renders a text snippet with highlighted terms
 */ function HighlightedSnippet(param) {
    let { snippet } = param;
    const highlightedText = (0,react.useMemo)(()=>{
        if (!snippet.highlightRanges || snippet.highlightRanges.length === 0) {
            return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                children: snippet.text
            });
        }
        // Sort ranges by start position
        const sortedRanges = [
            ...snippet.highlightRanges
        ].sort((a, b)=>a.start - b.start);
        const parts = [];
        let lastEnd = 0;
        sortedRanges.forEach((range, i)=>{
            // Add text before this highlight
            if (range.start > lastEnd) {
                parts.push(/*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    children: snippet.text.slice(lastEnd, range.start)
                }, "text-".concat(i)));
            }
            // Add highlighted text
            parts.push(/*#__PURE__*/ (0,jsx_runtime.jsx)("mark", {
                className: highlight,
                children: snippet.text.slice(range.start, range.end)
            }, "highlight-".concat(i)));
            lastEnd = range.end;
        });
        // Add remaining text
        if (lastEnd < snippet.text.length) {
            parts.push(/*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                children: snippet.text.slice(lastEnd)
            }, "text-end"));
        }
        return /*#__PURE__*/ (0,jsx_runtime.jsx)(jsx_runtime.Fragment, {
            children: parts
        });
    }, [
        snippet
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: HistorySearchResults_css_snippet,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: snippetRole,
                children: snippet.messageRole === "user" ? "You" : "Claude"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: snippetText,
                children: highlightedText
            })
        ]
    });
}
/**
 * Renders a single search result with snippets
 */ function SearchResultCard(param) {
    let { result, onClick } = param;
    const formatDate = (date)=>{
        if (!date) return "";
        return date.toLocaleDateString("en-US", {
            month: "short",
            day: "numeric",
            year: "numeric"
        });
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: resultCard,
        onClick: onClick,
        role: "button",
        tabIndex: 0,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: resultHeader,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: sessionName,
                        children: result.sessionName || result.sessionId
                    }),
                    result.metadata.model && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: modelBadge,
                        children: result.metadata.model
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: resultMeta,
                children: [
                    result.project && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: projectPath,
                        title: result.project,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("svg", {
                                width: "12",
                                height: "12",
                                viewBox: "0 0 24 24",
                                fill: "none",
                                stroke: "currentColor",
                                strokeWidth: "2",
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("path", {
                                    d: "M3 6h18M3 12h18M3 18h18"
                                })
                            }),
                            result.project.split("/").pop()
                        ]
                    }),
                    result.metadata.createdAt && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: date,
                        children: formatDate(result.metadata.createdAt)
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: score,
                        title: "Relevance score: ".concat(result.score.toFixed(2)),
                        children: [
                            (result.score * 100).toFixed(0),
                            "% match"
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: snippets,
                children: result.snippets.slice(0, 3).map((snippet, i)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(HighlightedSnippet, {
                        snippet: snippet
                    }, i))
            })
        ]
    });
}
function HistorySearchResults(param) {
    let { results, totalMatches, queryTimeMs, hasMore, loading, error, onResultClick, onLoadMore, query } = param;
    // Empty state - no query
    if (!query) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: emptyState,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("svg", {
                    className: emptyIcon,
                    width: "48",
                    height: "48",
                    viewBox: "0 0 24 24",
                    fill: "none",
                    stroke: "currentColor",
                    strokeWidth: "1.5",
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("circle", {
                            cx: "11",
                            cy: "11",
                            r: "8"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("path", {
                            d: "M21 21l-4.35-4.35"
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: emptyText,
                    children: "Search across all your Claude conversations"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: emptyHint,
                    children: "Try searching for topics, code snippets, or specific discussions"
                })
            ]
        });
    }
    // Loading state
    if (loading && results.length === 0) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: loadingState,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: loadingSpinner
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    children: "Searching conversations..."
                })
            ]
        });
    }
    // Error state
    if (error) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: errorState,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("svg", {
                    className: errorIcon,
                    width: "24",
                    height: "24",
                    viewBox: "0 0 24 24",
                    fill: "none",
                    stroke: "currentColor",
                    strokeWidth: "2",
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("circle", {
                            cx: "12",
                            cy: "12",
                            r: "10"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("line", {
                            x1: "12",
                            y1: "8",
                            x2: "12",
                            y2: "12"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("line", {
                            x1: "12",
                            y1: "16",
                            x2: "12.01",
                            y2: "16"
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                    className: errorText,
                    children: [
                        "Search failed: ",
                        error.message
                    ]
                })
            ]
        });
    }
    // No results state
    if (results.length === 0) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: emptyState,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("svg", {
                    className: emptyIcon,
                    width: "48",
                    height: "48",
                    viewBox: "0 0 24 24",
                    fill: "none",
                    stroke: "currentColor",
                    strokeWidth: "1.5",
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("circle", {
                            cx: "11",
                            cy: "11",
                            r: "8"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("path", {
                            d: "M21 21l-4.35-4.35"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("path", {
                            d: "M8 11h6"
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                    className: emptyText,
                    children: [
                        'No results found for "',
                        query,
                        '"'
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: emptyHint,
                    children: "Try different keywords or check your spelling"
                })
            ]
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: HistorySearchResults_css_container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: HistorySearchResults_css_header,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: resultCount,
                        children: [
                            totalMatches,
                            " result",
                            totalMatches !== 1 ? "s" : "",
                            " found"
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: queryTime,
                        children: [
                            "(",
                            queryTimeMs,
                            "ms)"
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: HistorySearchResults_css_results,
                children: results.map((result, index)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(SearchResultCard, {
                        result: result,
                        onClick: ()=>onResultClick === null || onResultClick === void 0 ? void 0 : onResultClick(result)
                    }, "".concat(result.sessionId, "-").concat(result.messageIndex, "-").concat(index)))
            }),
            hasMore && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: loadMoreContainer,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    className: loadMoreButton,
                    onClick: onLoadMore,
                    disabled: loading,
                    children: loading ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: buttonSpinner
                            }),
                            "Loading..."
                        ]
                    }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            "Load More Results",
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: remainingCount,
                                children: [
                                    "(",
                                    totalMatches - results.length,
                                    " more)"
                                ]
                            })
                        ]
                    })
                })
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/utils/timestamp.ts
var utils_timestamp = __webpack_require__(20882);
// EXTERNAL MODULE: ./src/lib/hooks/usePersistedViewState.ts
var usePersistedViewState = __webpack_require__(88253);
;// ./src/lib/hooks/useHistoryFilters.ts
/* __next_internal_client_entry_do_not_use__ HistoryGroupingStrategy,GroupingStrategyLabels,useHistoryFilters auto */ 


var useHistoryFilters_HistoryGroupingStrategy = /*#__PURE__*/ function(HistoryGroupingStrategy) {
    HistoryGroupingStrategy["None"] = "none";
    HistoryGroupingStrategy["Date"] = "date";
    HistoryGroupingStrategy["Project"] = "project";
    HistoryGroupingStrategy["Model"] = "model";
    return HistoryGroupingStrategy;
}({});
const GroupingStrategyLabels = {
    ["none"]: "No Grouping",
    ["date"]: "Date",
    ["project"]: "Project",
    ["model"]: "Model"
};
const DATE_FILTERS = [
    "all",
    "today",
    "week",
    "month"
];
const SORT_FIELDS = [
    "updated",
    "created",
    "messages",
    "name"
];
const SORT_ORDERS = [
    "asc",
    "desc"
];
const SEARCH_MODES = [
    "metadata",
    "fulltext"
];
const HISTORY_FILTER_FIELDS = {
    searchQuery: {
        key: "claude-history-search-query",
        defaultValue: ""
    },
    selectedModel: {
        key: "claude-history-selected-model",
        defaultValue: "all"
    },
    dateFilter: {
        key: "claude-history-date-filter",
        defaultValue: "all",
        isValid: (v)=>DATE_FILTERS.includes(v)
    },
    sortField: {
        key: "claude-history-sort-field",
        defaultValue: "updated",
        isValid: (v)=>SORT_FIELDS.includes(v)
    },
    sortOrder: {
        key: "claude-history-sort-order",
        defaultValue: "desc",
        isValid: (v)=>SORT_ORDERS.includes(v)
    },
    groupingStrategy: {
        key: "claude-history-grouping-strategy",
        defaultValue: "date",
        isValid: (v)=>Object.values(useHistoryFilters_HistoryGroupingStrategy).includes(v)
    },
    searchMode: {
        key: "claude-history-search-mode",
        defaultValue: "metadata",
        isValid: (v)=>SEARCH_MODES.includes(v)
    }
};
// ============================================================================
// Hook
// ============================================================================
function useHistoryFilters(entries) {
    // branchFilter is intentionally not persisted, same as before migration.
    const [branchFilter, setBranchFilter] = (0,react.useState)("");
    const { state, setters, isHydrated } = (0,usePersistedViewState/* usePersistedViewState */.P)(HISTORY_FILTER_FIELDS);
    const { searchQuery, selectedModel, dateFilter, sortField, sortOrder, groupingStrategy, searchMode } = state;
    const { searchQuery: setSearchQuery, selectedModel: setSelectedModel, dateFilter: setDateFilter, sortField: setSortField, sortOrder: setSortOrder, groupingStrategy: setGroupingStrategy, searchMode: setSearchMode } = setters;
    // Extract unique models for filter dropdown
    const uniqueModels = (0,react.useMemo)(()=>{
        const modelSet = new Set();
        entries.forEach((entry)=>{
            if (entry.model) modelSet.add(entry.model);
        });
        return Array.from(modelSet).sort();
    }, [
        entries
    ]);
    // Filter and sort entries
    const filteredEntries = (0,react.useMemo)(()=>{
        let result = entries.filter((entry)=>{
            // Model filter
            if (selectedModel !== "all" && entry.model !== selectedModel) {
                return false;
            }
            // Date filter
            if (!(0,utils_timestamp/* isWithinDateFilter */.wZ)(entry.updatedAt, dateFilter)) {
                return false;
            }
            // Search filter (client-side for immediate feedback)
            if (searchQuery) {
                const query = searchQuery.toLowerCase();
                const matchesSearch = entry.name.toLowerCase().includes(query) || entry.project && entry.project.toLowerCase().includes(query) || entry.model && entry.model.toLowerCase().includes(query);
                if (!matchesSearch) return false;
            }
            // Branch filter
            if (branchFilter) {
                const b = (entry.branch || "").toLowerCase();
                if (!b.includes(branchFilter.toLowerCase())) return false;
            }
            return true;
        });
        // Sort
        result.sort((a, b)=>{
            let comparison = 0;
            switch(sortField){
                case "updated":
                    var _b_updatedAt, _a_updatedAt;
                    comparison = Number(((_b_updatedAt = b.updatedAt) === null || _b_updatedAt === void 0 ? void 0 : _b_updatedAt.seconds) || 0) - Number(((_a_updatedAt = a.updatedAt) === null || _a_updatedAt === void 0 ? void 0 : _a_updatedAt.seconds) || 0);
                    break;
                case "created":
                    var _b_createdAt, _a_createdAt;
                    comparison = Number(((_b_createdAt = b.createdAt) === null || _b_createdAt === void 0 ? void 0 : _b_createdAt.seconds) || 0) - Number(((_a_createdAt = a.createdAt) === null || _a_createdAt === void 0 ? void 0 : _a_createdAt.seconds) || 0);
                    break;
                case "messages":
                    comparison = b.messageCount - a.messageCount;
                    break;
                case "name":
                    comparison = a.name.localeCompare(b.name);
                    break;
            }
            return sortOrder === "desc" ? comparison : -comparison;
        });
        return result;
    }, [
        entries,
        selectedModel,
        dateFilter,
        searchQuery,
        sortField,
        sortOrder
    ]);
    // Check if any filters are active
    const hasActiveFilters = !!(searchQuery || branchFilter || selectedModel !== "all" || dateFilter !== "all");
    // Actions
    const clearFilters = (0,react.useCallback)(()=>{
        setSearchQuery("");
        setBranchFilter("");
        setSelectedModel("all");
        setDateFilter("all");
    }, []);
    const cycleGroupingStrategy = (0,react.useCallback)(()=>{
        const strategies = Object.values(useHistoryFilters_HistoryGroupingStrategy);
        const currentIndex = strategies.indexOf(groupingStrategy);
        const nextIndex = (currentIndex + 1) % strategies.length;
        setGroupingStrategy(strategies[nextIndex]);
    }, [
        groupingStrategy
    ]);
    return {
        filterState: {
            searchQuery,
            branchFilter,
            selectedModel,
            dateFilter,
            sortField,
            sortOrder,
            groupingStrategy,
            searchMode,
            isHydrated
        },
        setters: {
            setSearchQuery,
            setBranchFilter,
            setSelectedModel,
            setDateFilter,
            setSortField,
            setSortOrder,
            setGroupingStrategy,
            setSearchMode
        },
        derived: {
            uniqueModels,
            filteredEntries,
            hasActiveFilters
        },
        actions: {
            clearFilters,
            cycleGroupingStrategy
        }
    };
}

// EXTERNAL MODULE: ./src/components/ui/ActionBar.tsx + 1 modules
var ActionBar = __webpack_require__(93321);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fhistory%2FHistoryFilterBar.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA81V227jIBB9z1fMy0qJFEeY1E7KvvRPKmKwjYrBAtKkWeXfV2CSEte9qS-VJVuCmcNhzpnxwxN_qQ3tuIXHvKC9Ou0Q_JsBOB0-AM5QZWttOgJGO-r4fF0ixpvF3xnAeXaerS6JechgwvaSvhCoJT_6GP_NmDC8ckIrApWW-075nYb2BHLch7COmkaobKed0x0BjPxygo6n0QcMNGBQKRqVCcc7S6DiynFzg7EOGD6VQO4TesqYUM2AAPndALPThnFDIO-PYLUUDJ6pmWfZY37PT9ic-OI1KjOUib0lUMZcWj01Ru8VI6OsTciqtNRmvIXCVq2Vy6w4cRKZpMxJrau9Dfz13kmhOAGlFU-YTEJ3kesxsy1l-kAAhQf3RzDNjs6L-yXka7QEfFcuAa3wYnQw6SWteKsl4yacP3nOKO1uWq1JgaKI5ejKRYA4COZacpWm5aJpHXkjFb5KFS6Fi2IJry-0WqeSOd1finVoheMTahboT-CrREcH2762B1ptLXgFqAGhaqECRMK8_IJVQ1ccjF_y76-4dxNgr5bdesfiX-PY9YBW7Y314b0WF3U7obKrjOO23vzE2AnO9lcXp3y3OGG8isFiVErfgPb2YqTVz7Hz3mfDb6tx_4ED0cRttwM_f1At_ZBoBWNcfaN6n9Qp2N22RqinwCDhSieUi_Wa8FdYOsQpUCD0rgj5G16h1D01XLn0YhezfUmcvLDAqQ0JP5pulCjt5kRS67KqFZItBoWjLnHKfVDzBGv3iTm61Kpx4iXp1e1f8Tx76DgTFOYdPV4ad1Nu--NAcfS_H821cxqBY8TEsLuJW8e4ZFJgNAVYpkduJwI2b5FyHJFu_YQv2ef_WL8w_wQJAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fhistory_2FHistoryFilterBar_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81V227jIBB9z1fMy0qJFEeY1E7KvvRPKmKwjYrBAtKkWeXfV2CSEte9qS_VJVuCmcNhzpnxwxN_qQ3tuIXHvKC9Ou0Q_JsBOB0_AM5QZWttOgJGO_r4fF0ixpvF3xnAeXaerS6JechgwvaSvhCoJT_6GP_NmDC8ckIrApWW_075nYb2BHLch7COmkaobKed0x0BjPxygo6n0QcMNGBQKRqVCcc7S6DiynFzg7EOGD6VQO4TesqYUM2AAPndALPThnFDIO_PYLUUDJ6pmWfZY37PT9ic_OI1KjOUib0lUMZcWj01Ru8VI6OsTciqtNRmvIXCVq2Vy6w4cRKZpMxJrau9Dfz13kmhOAGlFU_YTEJ3kesxsy1l_kAAhQf3RzDNjs6L_yXka7QEfFcuAa3wYnQw6SWteKsl4yacP3nOKO1uWq1JgaKI5ejKRYA4COZacpWm5aJpHXkjFb5KFS6Fi2IJry_0WqeSOd1finVoheMTahboT_CrREcH2762B1ptLXgFqAGhaqECRMK8_IJVQ1ccjF_y76_4dxNgr5bdesfiX_PY9YBW7Y314b0WF3U7obKrjOO23vzE2AnO9lcXp3y3OGG8isFiVErfgPb2YqTVz7Hz3mfDb6tx_4ED0cRttwM_f1At_ZBoBWNcfaN6n9Qp2N22RqinwCDhSieUi_Wa8FdYOsQpUCD0rgj5G16h1D01XLn0YhezfUmcvLDAqQ0JP5pulCjt5kRS67KqFZItBoWjLnHKfVDzBGv3iTm61Kpx4iXp1e1f8Tx76DgTFOYdPV4ad1Nu_NAcfS_H821cxqBY8TEsLuJW8e4ZFJgNAVYpkduJwI2b5FyHJFu_YQv2ef_WL8w_wQJAAA_22_7D = __webpack_require__(64332);
;// ./src/components/history/HistoryFilterBar.css.ts


var filterBar = '_15apnzb1';
var filters = '_15apnzb6';
var fullTextSearchInput = '_15apnzbc';
var searchButton = '_15apnzb4';
var searchContainer = '_15apnzb2';
var searchInput = '_15apnzb3';
var searchModeButton = '_15apnzba';
var searchModeButtonActive = '_15apnzbb';
var searchModeToggle = '_15apnzb9';
var HistoryFilterBar_css_select = '_15apnzb7';
var sortOrderButton = '_15apnzb8';
var spinnerSmall = '_15apnzb5';
;// ./src/components/history/HistoryFilterBar.tsx
/* __next_internal_client_entry_do_not_use__ HistoryFilterBar auto */ 




// ============================================================================
// Component
// ============================================================================
function HistoryFilterBar(param) {
    let { searchQuery, branchFilter, selectedModel, dateFilter, sortField, sortOrder, groupingStrategy, searchMode, setSearchQuery, setBranchFilter, setSelectedModel, setDateFilter, setSortField, setSortOrder, setGroupingStrategy, setSearchMode, uniqueModels, hasActiveFilters, searching, onSearch, onClearFilters, searchInputRef, fullTextSearch } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: filterBar,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: searchModeToggle,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: "".concat(searchModeButton, " ").concat(searchMode === "metadata" ? searchModeButtonActive : ""),
                        onClick: ()=>setSearchMode("metadata"),
                        title: "Search by name, project, model",
                        children: "\uD83D\uDCCB Metadata"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: "".concat(searchModeButton, " ").concat(searchMode === "fulltext" ? searchModeButtonActive : ""),
                        onClick: ()=>setSearchMode("fulltext"),
                        title: "Search full conversation content",
                        children: "\uD83D\uDD0D Full-Text"
                    })
                ]
            }),
            searchMode === "metadata" ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: searchContainer,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        ref: searchInputRef,
                        type: "text",
                        placeholder: "Search history... (Press /)",
                        value: searchQuery,
                        onChange: (e)=>setSearchQuery(e.target.value),
                        onKeyDown: (e)=>e.key === "Enter" && onSearch(),
                        className: searchInput
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        onClick: onSearch,
                        disabled: searching,
                        className: "btn btn-primary ".concat(searchButton),
                        children: searching ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: spinnerSmall
                                }),
                                "Searching..."
                            ]
                        }) : "Search"
                    }),
                    (searchQuery || hasActiveFilters) && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        onClick: onClearFilters,
                        className: "btn btn-secondary",
                        children: "Clear"
                    })
                ]
            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: searchContainer,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(HistorySearchInput, {
                    value: fullTextSearch.query,
                    onChange: fullTextSearch.setQuery,
                    onSubmit: (value)=>fullTextSearch.search({
                            query: value
                        }),
                    loading: fullTextSearch.loading,
                    placeholder: "Search conversation content...",
                    className: fullTextSearchInput
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)(ActionBar/* ActionBar */.E, {
                scroll: true,
                compact: true,
                gap: "sm",
                className: filters,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        type: "text",
                        placeholder: "⎇ Branch…",
                        value: branchFilter,
                        onChange: (e)=>setBranchFilter(e.target.value),
                        className: HistoryFilterBar_css_select,
                        style: {
                            minWidth: "100px"
                        },
                        "aria-label": "Filter by branch"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                        "aria-label": "Filter by model",
                        value: selectedModel,
                        onChange: (e)=>setSelectedModel(e.target.value),
                        className: HistoryFilterBar_css_select,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                value: "all",
                                children: "All Models"
                            }),
                            uniqueModels.map((model)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                    value: model,
                                    children: model
                                }, model))
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                        "aria-label": "Filter by date",
                        value: dateFilter,
                        onChange: (e)=>setDateFilter(e.target.value),
                        className: HistoryFilterBar_css_select,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                value: "all",
                                children: "All Time"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                value: "today",
                                children: "Today"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                value: "week",
                                children: "This Week"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                value: "month",
                                children: "This Month"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                        "aria-label": "Sort by",
                        value: sortField,
                        onChange: (e)=>setSortField(e.target.value),
                        className: HistoryFilterBar_css_select,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                value: "updated",
                                children: "Sort: Last Updated"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                value: "created",
                                children: "Sort: Created Date"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                value: "messages",
                                children: "Sort: Message Count"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                value: "name",
                                children: "Sort: Name"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        onClick: ()=>setSortOrder(sortOrder === "asc" ? "desc" : "asc"),
                        className: sortOrderButton,
                        "aria-label": "Sort ".concat(sortOrder === "asc" ? "descending" : "ascending"),
                        title: sortOrder === "asc" ? "Ascending" : "Descending",
                        children: sortOrder === "asc" ? "↑" : "↓"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("select", {
                        "aria-label": "Group by",
                        value: groupingStrategy,
                        onChange: (e)=>setGroupingStrategy(e.target.value),
                        className: HistoryFilterBar_css_select,
                        title: "Group by (Keyboard: G)",
                        children: Object.entries(GroupingStrategyLabels).map((param)=>{
                            let [value, label] = param;
                            return /*#__PURE__*/ (0,jsx_runtime.jsxs)("option", {
                                value: value,
                                children: [
                                    "Group: ",
                                    label
                                ]
                            }, value);
                        })
                    })
                ]
            })
        ]
    });
}

;// ./src/lib/terminal/stripAnsi.ts
/**
 * Strips ANSI escape sequences (SGR + OSC hyperlinks) without an npm dependency.
 *
 * The CSI branch's final-byte class is [@-~] (0x40-0x7E per ECMA-48), not just
 * letters — CSI sequences can terminate on '@' (Insert Character), '~', and
 * other non-letter bytes. A letter-only class leaves those sequences
 * unstripped, leaking raw escape bytes into the displayed text (BUG-025).
 */ function stripAnsi(str) {
    return str.replace(/\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)/g, "") // OSC sequences (hyperlinks, etc.)
    .replace(/\x1b[@-Z\\-_]|\x1b\[[0-9;]*[@-~]/g, ""); // CSI / single-char escapes
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fhistory%2FHistoryCardPreview.css.ts.vanilla.css%22%2C%22source%22%3A%22Lnlhb3VnNTAgewogIG1hcmdpbi10b3A6IDhweDsKICBib3JkZXItdG9wOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnplKTsKICBwYWRkaW5nLXRvcDogOHB4OwogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IDZweDsKfQoueWFvdWc1MSB7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNhKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKICBmb250LXN0eWxlOiBpdGFsaWM7Cn0KLnlhb3VnNTIgewogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYSk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6eCk7Cn0KLnlhb3VnNTMgewogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYSk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7CiAgZm9udC1zdHlsZTogaXRhbGljOwp9Ci55YW91ZzU0IHsKICBkaXNwbGF5OiBmbGV4OwogIGdhcDogNnB4OwogIGFsaWduLWl0ZW1zOiBmbGV4LXN0YXJ0OwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYSk7CiAgbGluZS1oZWlnaHQ6IDEuNDsKICBwYWRkaW5nOiA0cHggNnB4OwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzYpOwp9Ci55YW91ZzU1IHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejE3KTsKfQoueWFvdWc1NiB7CiAgYmFja2dyb3VuZDogdHJhbnNwYXJlbnQ7Cn0KLnlhb3VnNTcgewogIGZsZXgtc2hyaW5rOiAwOwogIGZvbnQtc2l6ZTogMTFweDsKICBsaW5lLWhlaWdodDogMS40Owp9Ci55YW91ZzU4IHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxKTsKICB3b3JkLWJyZWFrOiBicmVhay13b3JkOwogIG92ZXJmbG93OiBoaWRkZW47CiAgZGlzcGxheTogLXdlYmtpdC1ib3g7CiAgLXdlYmtpdC1ib3gtb3JpZW50OiB2ZXJ0aWNhbDsKICAtd2Via2l0LWxpbmUtY2xhbXA6IDI7Cn0%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fhistory_2FHistoryCardPreview_css_ts_vanilla_css_22_2C_22source_22_3A_22Lnlhb3VnNTAgewogIG1hcmdpbi10b3A6IDhweDsKICBib3JkZXItdG9wOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnplKTsKICBwYWRkaW5nLXRvcDogOHB4OwogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IDZweDsKfQoueWFvdWc1MSB7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNhKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKICBmb250LXN0eWxlOiBpdGFsaWM7Cn0KLnlhb3VnNTIgewogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYSk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6eCk7Cn0KLnlhb3VnNTMgewogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYSk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7CiAgZm9udC1zdHlsZTogaXRhbGljOwp9Ci55YW91ZzU0IHsKICBkaXNwbGF5OiBmbGV4OwogIGdhcDogNnB4OwogIGFsaWduLWl0ZW1zOiBmbGV4LXN0YXJ0OwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYSk7CiAgbGluZS1oZWlnaHQ6IDEuNDsKICBwYWRkaW5nOiA0cHggNnB4OwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzYpOwp9Ci55YW91ZzU1IHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejE3KTsKfQoueWFvdWc1NiB7CiAgYmFja2dyb3VuZDogdHJhbnNwYXJlbnQ7Cn0KLnlhb3VnNTcgewogIGZsZXgtc2hyaW5rOiAwOwogIGZvbnQtc2l6ZTogMTFweDsKICBsaW5lLWhlaWdodDogMS40Owp9Ci55YW91ZzU4IHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxKTsKICB3b3JkLWJyZWFrOiBicmVhay13b3JkOwogIG92ZXJmbG93OiBoaWRkZW47CiAgZGlzcGxheTogLXdlYmtpdC1ib3g7CiAgLXdlYmtpdC1ib3gtb3JpZW50OiB2ZXJ0aWNhbDsKICAtd2Via2l0LWxpbmUtY2xhbXA6IDI7Cn0_22_7D = __webpack_require__(58684);
;// ./src/components/history/HistoryCardPreview.css.ts


var assistantMessage = 'yaoug56';
var previewContainer = 'yaoug50';
var previewContent = 'yaoug58';
var previewEmpty = 'yaoug53';
var previewError = 'yaoug52';
var previewLoading = 'yaoug51';
var previewMessage = 'yaoug54';
var previewRole = 'yaoug57';
var userMessage = 'yaoug55';
;// ./src/components/history/HistoryCardPreview.tsx
/* __next_internal_client_entry_do_not_use__ HistoryCardPreview auto */ 



function HistoryCardPreview(param) {
    let { entryId, isVisible, fetchMessages } = param;
    const [messages, setMessages] = (0,react.useState)(null);
    const [loading, setLoading] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    // Cache fetched messages per entryId so re-expanding doesn't re-fetch.
    const cacheRef = (0,react.useRef)(new Map());
    (0,react.useEffect)(()=>{
        if (!isVisible) return;
        const cached = cacheRef.current.get(entryId);
        if (cached) {
            setMessages(cached);
            return;
        }
        let cancelled = false;
        setLoading(true);
        setError(null);
        fetchMessages(entryId).then((msgs)=>{
            if (cancelled) return;
            cacheRef.current.set(entryId, msgs);
            setMessages(msgs);
        }).catch((err)=>{
            if (!cancelled) setError(String(err));
        }).finally(()=>{
            if (!cancelled) setLoading(false);
        });
        return ()=>{
            cancelled = true;
        };
    }, [
        entryId,
        isVisible,
        fetchMessages
    ]);
    if (loading) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            id: "preview-".concat(entryId),
            className: previewContainer,
            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: previewLoading,
                children: "Loading messages…"
            })
        });
    }
    if (error) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            id: "preview-".concat(entryId),
            className: previewContainer,
            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: previewError,
                children: "Failed to load preview"
            })
        });
    }
    if (!messages || messages.length === 0) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            id: "preview-".concat(entryId),
            className: previewContainer,
            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: previewEmpty,
                children: "No messages available"
            })
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        id: "preview-".concat(entryId),
        className: previewContainer,
        children: messages.map((msg, idx)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: "".concat(previewMessage, " ").concat(msg.role === "user" ? userMessage : assistantMessage),
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: previewRole,
                        children: msg.role === "user" ? "👤" : "🤖"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: previewContent,
                        children: [
                            stripAnsi(msg.content).slice(0, 300),
                            msg.content.length > 300 ? "…" : ""
                        ]
                    })
                ]
            }, idx))
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fhistory%2FHistoryEntryCard.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA51W3bKqIBi97ym-y9MFjVjtii7Pg-xBRWWH4ACmdea8-xmwH7U050wXpsBirfX9wOobn4MgWxcB_FkAlDRJuMwI4E3ZAP4qm-MCIKLxKdOqkgmBM9W_EPrGB3YN9XW39ONKJ0wTwGUDRgmeDGaxziykacIrQ2DfYseVNkoTKBWXlmn3yWoqDbdcSQJUCAhWeGuAUcOOi7-L1YMxydWZac_7hh0r4bD6uxfLaQ3YiejhpiqujMdVlRVcMgJSSXact5FqkMlpomoCgf-FZTOYGW76W-JWxIPiNP40gw5s6GETbkpBLwRSwbzlP5WxPL2gWEnLpCVgShozFDFbMybdDCp4JhG3rDAEImqYM8ENFFRnXKJIWasKArf8yGhJAIfupbP72u-eKmlRzXiWWwJfQXC8fzP8ygjg7S0N3skJvGBHmwB2f128U-GczXmStFwtayx6DjAheGm4cUN1zi1DXp2LYK1pObB9QPYti23f1M1T1k1COCEhXI4SaZUhk2suTwSCMWqb2dS2L9TWE9Tw8k1EN-2Cl5zpZUTM7qXqFTg5BKbc3c6W8PWaNNugb83OT2l5uxLbf7D_PaXdbEqdRfvZiw7_kSW3UFhVPiqrVjpBkWb0RMA_EBXiUUIpLbi4ECiUVD65xngfZvOmY7zHNhyBbXvVZBw7_OhsflEn-Eiw1D6smut1MxrdaJzFtb8oft9YR4rEt8dblk6UfNIH5dI1XTQHe_PqAO58GnTf5xlfNnfzBmdzX_z60FfPBqfVcP55OWp-1UdKPyBdxpEGYcw-IOHdONTg1Mw_QO3HkQbthnukmic2f6Rpfg_GXOP7VOJKaybtb7e7TwvJC9pelcpKGAZhe1NCXCJVWeAy5ZLbfmf46RbRs910ZpxePOhfgJ7vj2wKRm5zb7Jyjnenzu1uTtQ6dXz60E3-AQ4SmnX0CgAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fhistory_2FHistoryEntryCard_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA51W3bKqIBi97ym_y9MFjVjtii7Pg_xBRWWH4ACmdea8_xmwH7U050wXpsBirfX9wOobn4MgWxcB_FkAlDRJuMwI4E3ZAP4qm_MCIKLxKdOqkgmBM9W_EPrGB3YN9XW39ONKJ0wTwGUDRgmeDGaxziykacIrQ2DfYseVNkoTKBWXlmn3yWoqDbdcSQJUCAhWeGuAUcOOi7_L1YMxydWZac_7hh0r4bD6uxfLaQ3YiejhpiqujMdVlRVcMgJSSXact5FqkMlpomoCgf_FZTOYGW76W_JWxIPiNP40gw5s6GETbkpBLwRSwbzlP5WxPL2gWEnLpCVgShozFDFbMybdDCp4JhG3rDAEImqYM8ENFFRnXKJIWasKArf8yGhJAIfupbP72u_eKmlRzXiWWwJfQXC8fzP8ygjg7S0N3skJvGBHmwB2f128U_GczXmStFwtayx6DjAheGm4cUN1zi1DXp2LYK1pObB9QPYti23f1M1T1k1COCEhXI4SaZUhk2suTwSCMWqb2dS2L9TWE9Tw8k1EN_2Cl5zpZUTM7qXqFTg5BKbc3c6W8PWaNNugb83OT2l5uxLbf7D_PaXdbEqdRfvZiw7_kSW3UFhVPiqrVjpBkWb0RMA_EBXiUUIpLbi4ECiUVD65xngfZvOmY7zHNhyBbXvVZBw7_OhsflEn_Eiw1D6smut1MxrdaJzFtb8oft9YR4rEt8dblk6UfNIH5dI1XTQHe_PqAO58GnTf5xlfNnfzBmdzX_z60FfPBqfVcP55OWp_1UdKPyBdxpEGYcw_IOHdONTg1Mw_QO3HkQbthnukmic2f6Rpfg_GXOP7VOJKaybtb7e7TwvJC9pelcpKGAZhe1NCXCJVWeAy5ZLbfmf46RbRs910ZpxePOhfgJ7vj2wKRm5zb7Jyjnenzu1uTtQ6dXz60E3_AQ4SmnX0CgAA_22_7D = __webpack_require__(29818);
;// ./src/components/history/HistoryEntryCard.css.ts


var entryBranch = '_1v00g3ma';
var entryCard = '_1v00g3m0';
var entryDirty = '_1v00g3mb';
var entryDivider = '_1v00g3m7';
var entryHeader = '_1v00g3m2';
var entryMessages = '_1v00g3m8';
var entryMeta = '_1v00g3m5';
var entryModel = '_1v00g3m6';
var entryName = '_1v00g3m3';
var entryProject = '_1v00g3m9';
var entryTime = '_1v00g3m4';
var expandButton = '_1v00g3mk';
var expandRow = '_1v00g3mj';
var headerRight = '_1v00g3mc';
var HistoryEntryCard_css_selected = '_1v00g3m1';
var statusDot = '_1v00g3mi';
var statusPill = '_1v00g3md';
var status_creating = '_1v00g3mg';
var status_paused = '_1v00g3mf';
var status_running = '_1v00g3me';
var status_stopped = '_1v00g3mh';
;// ./src/components/history/HistoryEntryCard.tsx
/* __next_internal_client_entry_do_not_use__ HistoryEntryCard auto */ 




function statusLabel(s) {
    switch(s){
        case types_pb/* SessionStatus */.Ph.ACTIVE:
            return "Running";
        case types_pb/* SessionStatus */.Ph.PAUSED:
            return "Paused";
        case types_pb/* SessionStatus */.Ph.CREATING:
            return "Starting";
        case types_pb/* SessionStatus */.Ph.HIBERNATED:
            return "Hibernated";
        case types_pb/* SessionStatus */.Ph.STOPPED:
            return "Stopped";
        default:
            return "";
    }
}
function statusVariant(s) {
    switch(s){
        case types_pb/* SessionStatus */.Ph.ACTIVE:
            return "running";
        case types_pb/* SessionStatus */.Ph.PAUSED:
            return "paused";
        case types_pb/* SessionStatus */.Ph.CREATING:
            return "creating";
        default:
            return "stopped";
    }
}
function HistoryEntryCard_HistoryEntryCard(param) {
    let { entry, isSelected, enrichedEntry, isExpanded, onToggleExpand, onSelect, fetchMessages } = param;
    var _enrichedEntry_vcsStatus;
    const vcs = isSelected && enrichedEntry ? enrichedEntry.vcsStatus : undefined;
    const isDirty = vcs ? !vcs.isClean : false;
    // Prefer list-level branch from Story 1 enrichment; fall back to VCS detail
    const branch = entry.branch || (isSelected && enrichedEntry ? (_enrichedEntry_vcsStatus = enrichedEntry.vcsStatus) === null || _enrichedEntry_vcsStatus === void 0 ? void 0 : _enrichedEntry_vcsStatus.branch : undefined);
    const hasLiveStatus = entry.sessionStatus !== types_pb/* SessionStatus */.Ph.UNSPECIFIED && entry.sessionStatus !== types_pb/* SessionStatus */.Ph.STOPPED && entry.sessionStatus !== types_pb/* SessionStatus */.Ph.PERMANENTLY_FAILED;
    const label = statusLabel(entry.sessionStatus);
    const variant = statusVariant(entry.sessionStatus);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: "".concat(entryCard, " ").concat(isSelected ? HistoryEntryCard_css_selected : ""),
        role: "option",
        tabIndex: 0,
        "aria-selected": isSelected,
        onClick: onSelect,
        onKeyDown: (e)=>{
            if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                onSelect();
            }
        },
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: entryHeader,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: entryName,
                        children: entry.name
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: headerRight,
                        children: [
                            label && hasLiveStatus && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: "".concat(statusPill, " ").concat(HistoryEntryCard_css_namespaceObject["status_".concat(variant)]),
                                children: [
                                    variant === "running" && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: statusDot
                                    }),
                                    label
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: entryTime,
                                children: (0,utils_timestamp/* formatTimeAgo */.ak)(entry.updatedAt)
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: entryMeta,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: entryModel,
                        children: entry.model
                    }),
                    branch && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: entryDivider,
                                children: "•"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: entryBranch,
                                title: "Branch",
                                children: [
                                    "⎇ ",
                                    branch
                                ]
                            })
                        ]
                    }),
                    isDirty && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: entryDirty,
                        title: "Uncommitted changes",
                        children: "✦"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: entryDivider,
                        children: "•"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: entryMessages,
                        children: [
                            entry.messageCount,
                            " ",
                            entry.messageCount === 1 ? "msg" : "msgs"
                        ]
                    })
                ]
            }),
            entry.project && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: entryProject,
                title: entry.project,
                children: entry.project.replace(/.*\//, "…/")
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: expandRow,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    className: expandButton,
                    "aria-label": isExpanded ? "Collapse preview" : "Show ".concat(entry.messageCount, " messages"),
                    "aria-expanded": isExpanded,
                    "aria-controls": "preview-".concat(entry.id),
                    onClick: (e)=>{
                        e.stopPropagation();
                        onToggleExpand(entry.id);
                    },
                    children: isExpanded ? "▲ Collapse" : "▼ Show messages"
                })
            }),
            isExpanded && /*#__PURE__*/ (0,jsx_runtime.jsx)(HistoryCardPreview, {
                entryId: entry.id,
                isVisible: isExpanded,
                fetchMessages: fetchMessages
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fhistory%2FHistoryGroupView.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA81UQZLaMBC884o54qqIkg0Li3LJT1JaaWxrY0sqSQZDir-nJBuwCTmwp5xc1oyme7rbXn1ytdtrCr8XAFJ52_ATg7LB_vsC0pNI5VAEZTQDYZqu1bHCG1VpogK2noFAHdDF48_OB1WeiDA6oA7TkuVSKl0x2FLbQ0FtQgjYB5KG3Xsvi9XAKk-sSqMD8eqMDPL34VY6OqKq6hDn0XjWclcpzdJkoJCPAMI0xjE4cLck5Ge-x3PhzjSboBT_0-4AH1z8qpzptHxkvctS3TiJjkFhe5Dc1ygf2nDSRhyXqvMMknC3ldePwm5GYQcRyYcJwbQM8u3s2uYVP_4p_N0qCjQSAzrBeEsYUxG00Thd_Pr-FKBNAKJzPtasUVdZJ6yVrtGpMDOG3gyRKIzjg-edlugaFfHuBFltDugSzacU9DRc21fDVXF71T0K1ZN6lFXwRiwHpIPCozUujLVvkFMqD3UGBNYxXEmDSLJszJGcGPAumMm6xA0j56HYJaoPCZi3vP8VgPWLAcizm9DBce1L41oGnbXoBPfJ1wZDQEe85WKwZvU2DecQm5wOubnvdGU8Sjfm_5Zk24M3jXr2sdzW23_JrFGiHy1KxWEZPTsqGWoGOS02ts_S1Fke5s5u6PhDuCwufwDId8YckAUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fhistory_2FHistoryGroupView_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81UQZLaMBC884o54qqIkg0Li3LJT1JaaWxrY0sqSQZDir_nJBuwCTmwp5xc1oyme7rbXn1ytdtrCr8XAFJ52_ATg7LB_vsC0pNI5VAEZTQDYZqu1bHCG1VpogK2noFAHdDF48_OB1WeiDA6oA7TkuVSKl0x2FLbQ0FtQgjYB5KG3Xsvi9XAKk_sSqMD8eqMDPL34VY6OqKq6hDn0XjWclcpzdJkoJCPAMI0xjE4cLck5Ge_x3PhzjSboBT_0_4AH1z8qpzptHxkvctS3TiJjkFhe5Dc1ygf2nDSRhyXqvMMknC3ldePwm5GYQcRyYcJwbQM8u3s2uYVP_4p_N0qCjQSAzrBeEsYUxG00Thd_Pr_FKBNAKJzPtasUVdZJ6yVrtGpMDOG3gyRKIzjg_edlugaFfHuBFltDugSzacU9DRc21fDVXF71T0K1ZN6lFXwRiwHpIPCozUujLVvkFMqD3UGBNYxXEmDSLJszJGcGPAumMm6xA0j56HYJaoPCZi3vP8VgPWLAcizm9DBce1L41oGnbXoBPfJ1wZDQEe85WKwZvU2DecQm5wOubnvdGU8Sjfm_5Zk24M3jXr2sdzW23_JrFGiHy1KxWEZPTsqGWoGOS02ts_S1Fke5s5u6PhDuCwufwDId8YckAUAAA_22_7D = __webpack_require__(2858);
;// ./src/components/history/HistoryGroupView.css.ts


var categoryContent = 'jai79n9';
var categoryGroup = 'jai79n7';
var categoryTitle = 'jai79n8';
var emptyStateContainer = 'jai79n2';
var emptyStateIcon = 'jai79n3';
var emptyStateTitle = 'jai79n4';
var entryCards = 'jai79n6';
var linkButton = 'jai79n5';
var loadingContainer = 'jai79n0';
var loadingTitle = 'jai79n1';
;// ./src/components/history/HistoryGroupView.tsx
/* __next_internal_client_entry_do_not_use__ HistoryGroupView auto */ 




function HistoryGroupView(param) {
    let { groupedEntries, flatEntries, selectedEntry, enrichedEntry, loading, entriesCount, filteredCount, hasActiveFilters, groupingStrategy, onSelectEntry, onClearFilters, fetchMessages } = param;
    const [expandedIds, setExpandedIds] = useState(new Set());
    const handleToggleExpand = useCallback((id)=>{
        setExpandedIds((prev)=>{
            const next = new Set(prev);
            if (next.has(id)) {
                next.delete(id);
            } else {
                next.add(id);
            }
            return next;
        });
    }, []);
    if (loading) {
        return /*#__PURE__*/ _jsxs("div", {
            className: styles.loadingContainer,
            children: [
                /*#__PURE__*/ _jsx("div", {
                    className: "spinner"
                }),
                /*#__PURE__*/ _jsx("div", {
                    className: styles.loadingTitle,
                    children: "Loading Claude History..."
                }),
                /*#__PURE__*/ _jsx("div", {
                    className: "text-muted",
                    style: {
                        fontSize: "14px"
                    },
                    children: entriesCount === 0 ? "This may take a few moments on first load..." : "Refreshing..."
                })
            ]
        });
    }
    if (filteredCount === 0) {
        return /*#__PURE__*/ _jsx("div", {
            className: styles.emptyStateContainer,
            children: hasActiveFilters ? /*#__PURE__*/ _jsxs(_Fragment, {
                children: [
                    /*#__PURE__*/ _jsx("div", {
                        className: styles.emptyStateIcon,
                        children: "\uD83D\uDD0D"
                    }),
                    /*#__PURE__*/ _jsx("h3", {
                        className: styles.emptyStateTitle,
                        children: "No results found"
                    }),
                    /*#__PURE__*/ _jsxs("p", {
                        className: "text-muted",
                        children: [
                            "Try adjusting your filters or",
                            " ",
                            /*#__PURE__*/ _jsx("button", {
                                onClick: onClearFilters,
                                className: styles.linkButton,
                                children: "clear all filters"
                            })
                        ]
                    })
                ]
            }) : entriesCount === 0 ? /*#__PURE__*/ _jsxs(_Fragment, {
                children: [
                    /*#__PURE__*/ _jsx("div", {
                        className: styles.emptyStateIcon,
                        children: "\uD83D\uDCDA"
                    }),
                    /*#__PURE__*/ _jsx("h3", {
                        className: styles.emptyStateTitle,
                        children: "No conversation history yet"
                    }),
                    /*#__PURE__*/ _jsx("p", {
                        className: "text-muted",
                        children: "Your Claude conversation history will appear here once you start using sessions."
                    })
                ]
            }) : /*#__PURE__*/ _jsxs(_Fragment, {
                children: [
                    /*#__PURE__*/ _jsx("div", {
                        className: styles.emptyStateIcon,
                        children: "\uD83D\uDCED"
                    }),
                    /*#__PURE__*/ _jsx("h3", {
                        className: styles.emptyStateTitle,
                        children: "No entries match your criteria"
                    }),
                    /*#__PURE__*/ _jsx("p", {
                        className: "text-muted",
                        children: "Adjust your filters to see more results."
                    })
                ]
            })
        });
    }
    return /*#__PURE__*/ _jsx("div", {
        className: styles.entryCards,
        children: groupedEntries.map((param)=>{
            let { groupKey, displayName, entries: groupEntries } = param;
            return /*#__PURE__*/ _jsxs("div", {
                className: styles.categoryGroup,
                children: [
                    groupingStrategy !== HistoryGroupingStrategy.None && /*#__PURE__*/ _jsxs("h3", {
                        className: styles.categoryTitle,
                        children: [
                            displayName,
                            " (",
                            groupEntries.length,
                            ")"
                        ]
                    }),
                    /*#__PURE__*/ _jsx("div", {
                        className: styles.categoryContent,
                        children: groupEntries.map((entry)=>{
                            const entryIndex = flatEntries.indexOf(entry);
                            const isSelected = (selectedEntry === null || selectedEntry === void 0 ? void 0 : selectedEntry.id) === entry.id;
                            return /*#__PURE__*/ _jsx(HistoryEntryCard, {
                                entry: entry,
                                isSelected: isSelected,
                                enrichedEntry: isSelected ? enrichedEntry : undefined,
                                isExpanded: expandedIds.has(entry.id),
                                onToggleExpand: handleToggleExpand,
                                onSelect: ()=>onSelectEntry(entry, entryIndex),
                                fetchMessages: fetchMessages
                            }, entry.id);
                        })
                    })
                ]
            }, groupKey);
        })
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fshared%2FVcsStatusDisplay.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xb2cybHdnMCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogNnB4OwogIHBhZGRpbmc6IDEwcHggMTJweDsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejgpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6ZSk7CiAgYm9yZGVyLXJhZGl1czogNnB4OwogIGZvbnQtc2l6ZTogMTNweDsKfQouXzFvZzJsd2cxIHsKICBkaXNwbGF5OiBmbGV4OwogIGFsaWduLWl0ZW1zOiBjZW50ZXI7CiAgZ2FwOiA4cHg7Cn0KLl8xb2cybHdnMiB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7CiAgZm9udC1zaXplOiAxMnB4OwogIG1pbi13aWR0aDogNTJweDsKICBmbGV4LXNocmluazogMDsKfQouXzFvZzJsd2czIHsKICBmb250LWZhbWlseTogdmFyKC0tXzE5ZXoycnoydSk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6bSk7CiAgZm9udC13ZWlnaHQ6IDUwMDsKfQouXzFvZzJsd2c0IHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnp1KTsKICBmb250LXdlaWdodDogNTAwOwp9Ci5fMW9nMmx3ZzUgewogIGNvbG9yOiB2YXIoLS1fMTllejJyengpOwogIGZvbnQtd2VpZ2h0OiA1MDA7Cn0KLl8xb2cybHdnNiB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LXdyYXA6IHdyYXA7CiAgZ2FwOiA2cHg7CiAgcGFkZGluZy1sZWZ0OiA2MHB4Owp9Ci5fMW9nMmx3ZzcgewogIGZvbnQtc2l6ZTogMTFweDsKICBwYWRkaW5nOiAycHggNnB4OwogIGJvcmRlci1yYWRpdXM6IDRweDsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyemUpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejEpOwogIGZvbnQtZmFtaWx5OiB2YXIoLS1fMTllejJyejJ1KTsKfQouXzFvZzJsd2c4IHsKICBmb250LXNpemU6IDEycHg7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7CiAgZm9udC1mYW1pbHk6IHZhcigtLV8xOWV6MnJ6MnUpOwp9Ci5fMW9nMmx3ZzkgewogIGZvbnQtc2l6ZTogMTRweDsKfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fshared_2FVcsStatusDisplay_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xb2cybHdnMCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogNnB4OwogIHBhZGRpbmc6IDEwcHggMTJweDsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejgpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6ZSk7CiAgYm9yZGVyLXJhZGl1czogNnB4OwogIGZvbnQtc2l6ZTogMTNweDsKfQouXzFvZzJsd2cxIHsKICBkaXNwbGF5OiBmbGV4OwogIGFsaWduLWl0ZW1zOiBjZW50ZXI7CiAgZ2FwOiA4cHg7Cn0KLl8xb2cybHdnMiB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7CiAgZm9udC1zaXplOiAxMnB4OwogIG1pbi13aWR0aDogNTJweDsKICBmbGV4LXNocmluazogMDsKfQouXzFvZzJsd2czIHsKICBmb250LWZhbWlseTogdmFyKC0tXzE5ZXoycnoydSk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6bSk7CiAgZm9udC13ZWlnaHQ6IDUwMDsKfQouXzFvZzJsd2c0IHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnp1KTsKICBmb250LXdlaWdodDogNTAwOwp9Ci5fMW9nMmx3ZzUgewogIGNvbG9yOiB2YXIoLS1fMTllejJyengpOwogIGZvbnQtd2VpZ2h0OiA1MDA7Cn0KLl8xb2cybHdnNiB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LXdyYXA6IHdyYXA7CiAgZ2FwOiA2cHg7CiAgcGFkZGluZy1sZWZ0OiA2MHB4Owp9Ci5fMW9nMmx3ZzcgewogIGZvbnQtc2l6ZTogMTFweDsKICBwYWRkaW5nOiAycHggNnB4OwogIGJvcmRlci1yYWRpdXM6IDRweDsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyemUpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejEpOwogIGZvbnQtZmFtaWx5OiB2YXIoLS1fMTllejJyejJ1KTsKfQouXzFvZzJsd2c4IHsKICBmb250LXNpemU6IDEycHg7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7CiAgZm9udC1mYW1pbHk6IHZhcigtLV8xOWV6MnJ6MnUpOwp9Ci5fMW9nMmx3ZzkgewogIGZvbnQtc2l6ZTogMTRweDsKfQ_22_7D = __webpack_require__(72034);
;// ./src/components/shared/VcsStatusDisplay.css.ts


var branch = '_1og2lwg3';
var changes = '_1og2lwg6';
var clean = '_1og2lwg4';
var VcsStatusDisplay_css_container = '_1og2lwg0';
var dirty = '_1og2lwg5';
var label = '_1og2lwg2';
var remote = '_1og2lwg8';
var row = '_1og2lwg1';
var stat = '_1og2lwg7';
var vcsTypeIcon = '_1og2lwg9';
;// ./src/components/shared/VcsStatusDisplay.tsx
/* __next_internal_client_entry_do_not_use__ VcsStatusDisplay auto */ 


function vcsTypeName(type) {
    switch(type){
        case VCSType.VCS_TYPE_GIT:
            return "Git";
        case VCSType.VCS_TYPE_JUJUTSU:
            return "Jujutsu";
        default:
            return "VCS";
    }
}
/**
 * Pure display component for VCSStatus. Accepts an already-fetched VCSStatus
 * proto and renders branch, clean/dirty state, file counts, and remote sync.
 *
 * Used by both the history detail panel (where status comes from
 * GetClaudeHistoryDetail) and the session VCS tab (where it comes from
 * GetVCSStatus). Keep data-fetching out of this component.
 */ function VcsStatusDisplay(param) {
    let { status } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: VcsStatusDisplay_css_container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: row,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: label,
                        children: "Branch:"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: branch,
                        children: [
                            "⎇ ",
                            status.branch || "(detached)"
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: row,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: label,
                        children: "Status:"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: status.isClean ? clean : dirty,
                        children: status.isClean ? "✓ Clean" : "✦ Uncommitted changes"
                    })
                ]
            }),
            !status.isClean && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: changes,
                children: [
                    status.hasStaged && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: stat,
                        title: "Staged files",
                        children: [
                            "+",
                            status.stagedFiles.length,
                            " staged"
                        ]
                    }),
                    status.hasUnstaged && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: stat,
                        title: "Modified files",
                        children: [
                            "~",
                            status.unstagedFiles.length,
                            " modified"
                        ]
                    }),
                    status.hasUntracked && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: stat,
                        title: "Untracked files",
                        children: [
                            "?",
                            status.untrackedFiles.length,
                            " untracked"
                        ]
                    }),
                    status.hasConflicts && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: stat,
                        title: "Conflicted files",
                        children: [
                            "⚠ ",
                            status.conflictFiles.length,
                            " conflicts"
                        ]
                    })
                ]
            }),
            (status.aheadBy > 0 || status.behindBy > 0) && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: row,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: label,
                        children: "Remote:"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: remote,
                        children: [
                            status.aheadBy > 0 && "↑".concat(status.aheadBy, " ahead"),
                            status.aheadBy > 0 && status.behindBy > 0 && " · ",
                            status.behindBy > 0 && "↓".concat(status.behindBy, " behind")
                        ]
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fhistory%2FHistoryDetailPanel.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61Wy47iMBC88xV9WQkkjJwQGPBc9k9WJjGJB8f2OgbCrPj3VZwHzgNmNdoDQrhDd1d1dcWrX8FNS7P-wPBnBnDlic0IrHdYl-8zgKNgJSoyw-WJAK5ODjQ-pUadZULgQs0coV_Bnn2G5vNt4eLKJMwQCHQJhRI8GTzFvKeQoQk_FwR2dTVNk4TLlEDYlM9piTLG08wSiKmI53WuC2dXrYxtYksIME4u2QIQhFXnroS6MHMU6opuBOjZqvfZfbZq0QYObU5NyiU6KGtVTiAIG9BKWlTwT0Yg2HpH16aTLXZMxEooMyQBL3p1Qlcn4YUW9EYcnR2tCTcstlxJUqU657KKpFS3Vb00kUsz1cQAQlS3O9lasBiCa_BaVlpkDZXFUZmcwFlrZmJasCoomLXMoELT2I0GrzaD5jbTGKngqUTcsrwgEDNpmekA7gYpti6FLy2pJPPl1P7uJBLpspVNfDZFBVcr3lYZ6KuhxWHkNeWPYoBXYdFvh2SVekZNjZXs_entMaMjzbm4EciVVBVx7Anxk2PK3ZiuyiToYBg9EXBfiArRq7f7nrS6zXK6sUoPt68-a4XfEFmfvVhpr7G9a-wxKKzLbqGd1JwyfE1M8hAOs2pvZ50ZeVH6IL-mONr1cHYrPtysg28EPeT_i454ek4f58Ly4w3FSlomLQEnFHRg9sqYfLFAk6bl1Uu-pYvd2HDXuBnaSydl0-X8pJ0UdrrsBDjY0O3Yetf1keCSdT0Fq6hX_TjaUZMe6HyzX0KwxksIo-0S8Cp48mqafHggvHS6xDpaQrB_W8I--rLC4NlBgazW7uhN63MRDYbM3X9e2PzYP6oTF8m4Zc7PGQFtGLoaqnvJP0Y7MTnIxsR8ZpzBamqYtN--C2y_9MYJkUy8Anyzp0I0Lj9tQT74f_L-upHJHn_7oJ6i8CqeXmxQ3-VEpxQCQd-OkKkWpCMRe9HaKcYPeInzGu4jm2DHl8mexqvASP_hZrNsP7X8PS3hxhLus585SziFeWVCzUU0wGGky4Vrb9W_qnaX1QDjH-_uwHev9sJwn93_AoE1UwLkCgAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fhistory_2FHistoryDetailPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61Wy47iMBC88xV9WQkkjJwQGPBc9k9WJjGJB8f2OgbCrPj3VZwHzgNmNdoDQrhDd1d1dcWrX8FNS7P_wPBnBnDlic0IrHdYl_8zgKNgJSoyw_WJAK5ODjQ_pUadZULgQs0coV_Bnn2G5vNt4eLKJMwQCHQJhRI8GTzFvKeQoQk_FwR2dTVNk4TLlEDYlM9piTLG08wSiKmI53WuC2dXrYxtYksIME4u2QIQhFXnroS6MHMU6opuBOjZqvfZfbZq0QYObU5NyiU6KGtVTiAIG9BKWlTwT0Yg2HpH16aTLXZMxEooMyQBL3p1Qlcn4YUW9EYcnR2tCTcstlxJUqU657KKpFS3Vb00kUsz1cQAQlS3O9lasBiCa_BaVlpkDZXFUZmcwFlrZmJasCoomLXMoELT2I0GrzaD5jbTGKngqUTcsrwgEDNpmekA7gYpti6FLy2pJPPl1P7uJBLpspVNfDZFBVcr3lYZ6KuhxWHkNeWPYoBXYdFvh2SVekZNjZXs_entMaMjzbm4EciVVBVx7Anxk2PK3ZiuyiToYBg9EXBfiArRq7f7nrS6zXK6sUoPt68_a4XfEFmfvVhpr7G9a_wxKKzLbqGd1JwyfE1M8hAOs2pvZ50ZeVH6IL_mONr1cHYrPtysg28EPeT_i454ek4f58Ly4w3FSlomLQEnFHRg9sqYfLFAk6bl1Uu_pYvd2HDXuBnaSydl0_X8pJ0UdrrsBDjY0O3Yetf1keCSdT0Fq6hX_TjaUZMe6HyzX0KwxksIo_0S8Cp48mqafHggvHS6xDpaQrB_W8I_rLC4NlBgazW7uhN63MRDYbM3X9e2PzYP6oTF8m4Zc7PGQFtGLoaqnvJP0Y7MTnIxsR8ZpzBamqYtN_C2y_9MYJkUy8Anyzp0I0Lj9tQT74f_L_upHJHn_7oJ6i8CqeXmxQ3_VEpxQCQd_OkKkWpCMRe9HaKcYPeInzGu4jm2DHl8mexqvASP_hZrNsP7X8PS3hxhLus585SziFeWVCzUU0wGGky4Vrb9W_qnaX1QDjH_uwHev9sJwn93_AoE1UwLkCgAA_22_7D = __webpack_require__(51657);
;// ./src/components/history/HistoryDetailPanel.css.ts


var HistoryDetailPanel_css_assistantMessage = '_1ypnr3jg';
var copyButton = '_1ypnr3j6';
var detailActions = '_1ypnr3j8';
var detailField = '_1ypnr3j3';
var detailFields = '_1ypnr3j2';
var detailPanel = '_1ypnr3j0';
var HistoryDetailPanel_css_emptyState = '_1ypnr3j9';
var HistoryDetailPanel_css_emptyStateIcon = '_1ypnr3ja';
var fieldLabel = '_1ypnr3j4';
var idField = '_1ypnr3j5';
var messagePreview = '_1ypnr3jb';
var HistoryDetailPanel_css_previewContent = '_1ypnr3ji';
var previewHeader = '_1ypnr3jc';
var HistoryDetailPanel_css_previewMessage = '_1ypnr3je';
var HistoryDetailPanel_css_previewMessages = '_1ypnr3jd';
var HistoryDetailPanel_css_previewRole = '_1ypnr3jh';
var HistoryDetailPanel_css_projectPath = '_1ypnr3j7';
var sectionTitle = '_1ypnr3j1';
var splitButton = '_1ypnr3jk';
var splitButtonChevron = '_1ypnr3jm';
var splitButtonMain = '_1ypnr3jl';
var HistoryDetailPanel_css_userMessage = '_1ypnr3jf';
var viewMoreButton = '_1ypnr3jj';
;// ./src/components/history/HistoryDetailPanel.tsx
/* __next_internal_client_entry_do_not_use__ HistoryDetailPanel auto */ 




function HistoryDetailPanel(param) {
    let { entry, previewMessages, loadingPreview, loadingMessages, resuming, onResume, onFork, onViewMessages, onExport, onCopyId } = param;
    if (!entry) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: detailPanel,
            children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: HistoryDetailPanel_css_emptyState,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: HistoryDetailPanel_css_emptyStateIcon,
                        children: "\uD83D\uDC46"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        children: "Select an entry to view details"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: "text-muted",
                        style: {
                            fontSize: "12px",
                            marginTop: "10px"
                        },
                        children: "Use ↑↓ or j/k to navigate"
                    })
                ]
            })
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: detailPanel,
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                    className: sectionTitle,
                    children: "Entry Details"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: detailFields,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: detailField,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: fieldLabel,
                                    children: "Name:"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: "text-primary",
                                    children: entry.name
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: detailField,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: fieldLabel,
                                    children: "ID:"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: idField,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("code", {
                                            className: "text-muted",
                                            children: [
                                                entry.id.substring(0, 8),
                                                "..."
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                            onClick: ()=>onCopyId(entry.id),
                                            className: copyButton,
                                            title: "Copy full ID",
                                            children: "\uD83D\uDCCB"
                                        })
                                    ]
                                })
                            ]
                        }),
                        entry.project && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: detailField,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: fieldLabel,
                                    children: "Project:"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: HistoryDetailPanel_css_projectPath,
                                    title: entry.project,
                                    children: entry.project
                                })
                            ]
                        }),
                        entry.vcsStatus && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: detailField,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: fieldLabel,
                                    children: entry.vcsStatus.type === types_pb/* VCSType */.Nz.VCS_TYPE_JUJUTSU ? "Jujutsu State" : "Git State"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)(VcsStatusDisplay, {
                                    status: entry.vcsStatus
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: detailField,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: fieldLabel,
                                    children: "Model:"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: "text-primary",
                                    children: entry.model
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: detailField,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: fieldLabel,
                                    children: "Message Count:"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: "text-primary",
                                    children: entry.messageCount
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: detailField,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: fieldLabel,
                                    children: "Created:"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: "text-secondary",
                                    style: {
                                        fontSize: "13px"
                                    },
                                    children: (0,utils_timestamp/* formatDate */.Yq)(entry.createdAt)
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: detailField,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: fieldLabel,
                                    children: "Last Updated:"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: "text-secondary",
                                    style: {
                                        fontSize: "13px"
                                    },
                                    children: (0,utils_timestamp/* formatDate */.Yq)(entry.updatedAt)
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: messagePreview,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: previewHeader,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: fieldLabel,
                                            children: "Recent Messages"
                                        }),
                                        loadingPreview && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: "text-muted",
                                            style: {
                                                fontSize: "12px"
                                            },
                                            children: "Loading..."
                                        })
                                    ]
                                }),
                                previewMessages.length > 0 ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: HistoryDetailPanel_css_previewMessages,
                                    children: [
                                        previewMessages.map((msg, idx)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: "".concat(HistoryDetailPanel_css_previewMessage, " ").concat(msg.role === "user" ? HistoryDetailPanel_css_userMessage : HistoryDetailPanel_css_assistantMessage),
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                                        className: HistoryDetailPanel_css_previewRole,
                                                        children: msg.role === "user" ? "👤" : "🤖"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                                        className: HistoryDetailPanel_css_previewContent,
                                                        children: msg.content.length > 200 ? msg.content.substring(0, 200) + "..." : msg.content
                                                    })
                                                ]
                                            }, idx)),
                                        entry.messageCount > 5 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                            onClick: ()=>onViewMessages(entry.id),
                                            className: viewMoreButton,
                                            children: [
                                                "View all ",
                                                entry.messageCount,
                                                " messages →"
                                            ]
                                        })
                                    ]
                                }) : !loadingPreview ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: "text-muted",
                                    style: {
                                        fontSize: "12px",
                                        fontStyle: "italic"
                                    },
                                    children: "No messages available"
                                }) : null
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: detailActions,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: splitButton,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                            onClick: ()=>onResume(entry),
                                            disabled: resuming || !entry.project,
                                            className: "btn btn-primary ".concat(splitButtonMain),
                                            title: entry.project ? "Start a new session resuming this conversation" : "Cannot resume: No project path",
                                            children: resuming ? "Starting..." : "▶️ Resume"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                            onClick: ()=>onFork(entry),
                                            disabled: resuming || !entry.project,
                                            className: "btn btn-primary ".concat(splitButtonChevron),
                                            title: "Fork conversation to a new session",
                                            "aria-label": "Fork conversation",
                                            children: "\uD83C\uDF74"
                                        })
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    onClick: ()=>onViewMessages(entry.id),
                                    disabled: loadingMessages,
                                    className: "btn btn-secondary",
                                    children: loadingMessages ? "Loading..." : "💬 View Messages"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    onClick: ()=>onExport(entry),
                                    className: "btn btn-secondary",
                                    title: "Export conversation as JSON",
                                    children: "\uD83D\uDCE5 Export"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    onClick: ()=>onCopyId(entry.id),
                                    className: "btn btn-secondary",
                                    title: "Copy conversation ID",
                                    children: "\uD83D\uDCCB Copy ID"
                                })
                            ]
                        })
                    ]
                })
            ]
        })
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fhistory%2FHistoryMessagesModal.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA81W25LiIBB99yt42aqxarBI1ExkXvZPtjC0hpkIWUI0uuW_bwFJhmTiZd-2tAxBaE6fPt3N4vz7uPkwBP2ZIVSqShihJEU70QB_nyFkVEkRsaMCdqYdarHPu_FWGaMO3QvLPvda1ZJTpPdb9kJeUftdJHO7gouqLNiZol0BjZ1ghdhLLAwcKooykAa0nf6oKyN2Z5wpaUCa8K8LFpJDQ1FEiDu1ZJwLuacoJmXzPrvOFt6pyDkVYjoy_YLxr2gDl1hfPKKt0hw01oyLuqIoikuH6yS4yd0ZP-zrgTW4ndoQ4pfYuRw8F-n6mE_6Z5-YCw2ZZzZTRX2Q_uAGVznj6kQRcdhRYn_GxC3ngU-x8-lZFvespChKPNwBTShe-dnW_S6MUdmgShWCj7iCee9MlWshP13Ee1hLB2unpMGVuABFUerNu6lTS9JWFdwTp_dCtprJVKH0ODTk_nErf1zhVDBJu_O8xTBJTm9rPbbV85SWTS8HT9MjfkZaaom_rcC3-SMGAkaXA3Gv6U5ldeXAq9oUQgJFUkkIcEwaPoRySr6lyNDE13uApJNOVuvK2i-V6ATXc7cKuJuEEd0hzGgmu0rEigKRRVzd1UNCc3UE_SDf4T7bgbW71IZZaz9xl7PrzSuKluQVxavEJm4cGn0by8wC3hXqhM8UsdqoWynaW0idBZ87fb7GZJTc0ZOiGzGfDqqBr_WrG1ofKGjzv6CKQlRsukx-6ypVyTLAWzAnAPn-3REv4d7s1pk95cIAdlspKjXgk2al6xlKc7zVwD4pcg9sZ8Z53GaPlVXfPqJF8pw2s2nHbneZf--vfdBcN-pCaaAx2BkL1z4XT-pyhLMqh6fqZjognY97yyrt-u8wVslgGzzVkhLyqAn1zQoR1xHCurN7unzeLMWTZTRALWQOWphBYEgfEA6Z0szHvJYctFVVCDAojJMQpBPXzwNwwdBLcMt5S9KymbuNg-vU8NqzIf7ac_1aFrfLnCRtZlDU5Ud3IWn7QrBp1W5qWVv61aM72HV2_QtSb4IWrwoAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fhistory_2FHistoryMessagesModal_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81W25LiIBB99yt42aqxarBI1ExkXvZPtjC0hpkIWUI0uuW_bwFJhmTiZd_2tAxBaE6fPt3N4vz7uPkwBP2ZIVSqShihJEU70QB_nyFkVEkRsaMCdqYdarHPu_FWGaMO3QvLPvda1ZJTpPdb9kJeUftdJHO7gouqLNiZol0BjZ1ghdhLLAwcKooykAa0nf6oKyN2Z5wpaUCa8K8LFpJDQ1FEiDu1ZJwLuacoJmXzPrvOFt6pyDkVYjoy_YLxr2gDl1hfPKKt0hw01oyLuqIoikuH6yS4yd0ZP_zrgTW4ndoQ4pfYuRw8F_n6mE_6Z5_YCw2ZZzZTRX2Q_uAGVznj6kQRcdhRYn_GxC3ngU_x8_lZFvespChKPNwBTShe_dnW_S6MUdmgShWCj7iCee9MlWshP13Ee1hLB2unpMGVuABFUerNu6lTS9JWFdwTp_dCtprJVKH0ODTk_nErf1zhVDBJu_O8xTBJTm9rPbbV85SWTS8HT9MjfkZaaom_rcC3_SMGAkaXA3Gv6U5ldeXAq9oUQgJFUkkIcEwaPoRySr6lyNDE13uApJNOVuvK2i_V6ATXc7cKuJuEEd0hzGgmu0rEigKRRVzd1UNCc3UE_SDf4T7bgbW71IZZaz9xl7PrzSuKluQVxavEJm4cGn0by8wC3hXqhM8UsdqoWynaW0idBZ87fb7GZJTc0ZOiGzGfDqqBr_WrG1ofKGjzv6CKQlRsukx_6ypVyTLAWzAnAPn_3REv4d7s1pk95cIAdlspKjXgk2al6xlKc7zVwD4pcg9sZ8Z53GaPlVXfPqJF8pw2s2nHbneZf_vfdBcN_pCaaAx2BkL1z4XT_pyhLMqh6fqZjognY97yyrt_u8wVslgGzzVkhLyqAn1zQoR1xHCurN7unzeLMWTZTRALWQOWphBYEgfEA6Z0szHvJYctFVVCDAojJMQpBPXzwNwwdBLcMt5S9KymbuNg_vU8NqzIf7ac_1aFrfLnCRtZlDU5Ud3IWn7QrBp1W5qWVv61aM72HV2_QtSb4IWrwoAAA_22_7D = __webpack_require__(36143);
;// ./src/components/history/HistoryMessagesModal.css.ts


var HistoryMessagesModal_css_emptyStateContainer = 'yqv9jtc';
var HistoryMessagesModal_css_emptyStateIcon = 'yqv9jtd';
var HistoryMessagesModal_css_emptyStateTitle = 'yqv9jte';
var HistoryMessagesModal_css_linkButton = 'yqv9jtf';
var messageAssistant = 'yqv9jt9';
var messageContent = 'yqv9jtb';
var messageHeader = 'yqv9jta';
var messageSearchContainer = 'yqv9jt4';
var messageSearchInput = 'yqv9jt5';
var messageUser = 'yqv9jt8';
var modal = 'yqv9jt1';
var modalCloseButton = 'yqv9jt6';
var modalContent = 'yqv9jt7';
var modalHeader = 'yqv9jt2';
var modalOverlay = 'yqv9jt0';
var modalTitle = 'yqv9jt3';
;// ./src/components/history/HistoryMessagesModal.tsx
/* __next_internal_client_entry_do_not_use__ HistoryMessagesModal auto */ 



function HistoryMessagesModal(param) {
    let { open, messages, messageSearchQuery, onSearchChange, onClose } = param;
    const modalRef = (0,react.useRef)(null);
    // Filter messages in modal
    const filteredMessages = (0,react.useMemo)(()=>{
        if (!messageSearchQuery) return messages;
        const query = messageSearchQuery.toLowerCase();
        return messages.filter((msg)=>msg.content.toLowerCase().includes(query));
    }, [
        messages,
        messageSearchQuery
    ]);
    // Focus trap for modal
    (0,react.useEffect)(()=>{
        if (open && modalRef.current) {
            const focusableElements = modalRef.current.querySelectorAll('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])');
            const firstElement = focusableElements[0];
            const lastElement = focusableElements[focusableElements.length - 1];
            const handleTabKey = (e)=>{
                if (e.key !== "Tab") return;
                if (e.shiftKey) {
                    if (document.activeElement === firstElement) {
                        e.preventDefault();
                        lastElement === null || lastElement === void 0 ? void 0 : lastElement.focus();
                    }
                } else {
                    if (document.activeElement === lastElement) {
                        e.preventDefault();
                        firstElement === null || firstElement === void 0 ? void 0 : firstElement.focus();
                    }
                }
            };
            firstElement === null || firstElement === void 0 ? void 0 : firstElement.focus();
            window.addEventListener("keydown", handleTabKey);
            return ()=>window.removeEventListener("keydown", handleTabKey);
        }
    }, [
        open
    ]);
    if (!open) return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: modalOverlay,
        onClick: onClose,
        role: "dialog",
        "aria-modal": "true",
        "aria-labelledby": "messages-modal-title",
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            ref: modalRef,
            className: modal,
            onClick: (e)=>e.stopPropagation(),
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: modalHeader,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("h2", {
                            id: "messages-modal-title",
                            className: modalTitle,
                            children: [
                                "Conversation Messages (",
                                filteredMessages.length,
                                messageSearchQuery && " of ".concat(messages.length),
                                ")"
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: messageSearchContainer,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                    type: "text",
                                    placeholder: "Search in messages...",
                                    value: messageSearchQuery,
                                    onChange: (e)=>onSearchChange(e.target.value),
                                    className: messageSearchInput
                                }),
                                messageSearchQuery && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    onClick: ()=>onSearchChange(""),
                                    className: "btn btn-ghost btn-sm",
                                    children: "Clear"
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            onClick: onClose,
                            className: modalCloseButton,
                            "aria-label": "Close messages dialog",
                            children: "✕"
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: modalContent,
                    children: filteredMessages.length === 0 && messageSearchQuery ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: HistoryMessagesModal_css_emptyStateContainer,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: HistoryMessagesModal_css_emptyStateIcon,
                                children: "\uD83D\uDD0D"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("h3", {
                                className: HistoryMessagesModal_css_emptyStateTitle,
                                children: [
                                    'No messages match "',
                                    messageSearchQuery,
                                    '"'
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                onClick: ()=>onSearchChange(""),
                                className: HistoryMessagesModal_css_linkButton,
                                children: "Clear search"
                            })
                        ]
                    }) : filteredMessages.map((msg, idx)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: msg.role === "user" ? messageUser : messageAssistant,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: messageHeader,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                            style: {
                                                fontWeight: "600",
                                                color: msg.role === "user" ? "var(--primary)" : "var(--text-secondary)",
                                                textTransform: "capitalize"
                                            },
                                            children: msg.role
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                            className: "text-muted",
                                            style: {
                                                fontSize: "12px"
                                            },
                                            children: (0,utils_timestamp/* formatDate */.Yq)(msg.timestamp)
                                        })
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: messageContent,
                                    children: msg.content
                                }),
                                msg.model && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: "text-muted",
                                    style: {
                                        fontSize: "11px",
                                        marginTop: "8px",
                                        fontFamily: "monospace"
                                    },
                                    children: [
                                        "Model: ",
                                        msg.model
                                    ]
                                })
                            ]
                        }, idx))
                })
            ]
        })
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@tanstack+react-virtual@3.13.25+498059a1009c1789/node_modules/@tanstack/react-virtual/dist/esm/index.js + 3 modules
var esm = __webpack_require__(28396);
;// ./src/components/history/VirtualHistoryList.tsx
/* __next_internal_client_entry_do_not_use__ VirtualHistoryList auto */ 





function VirtualHistoryList(param) {
    let { groupedEntries, flatEntries, selectedEntry, enrichedEntry, loading, entriesCount, filteredCount, hasActiveFilters, groupingStrategy, hasNextPage, loadingMore, onSelectEntry, onClearFilters, onLoadMore, fetchMessages, virtualizerRef } = param;
    const [expandedIds, setExpandedIds] = (0,react.useState)(new Set());
    const scrollRef = (0,react.useRef)(null);
    // Guard: prevent scroll-anchor correction during expand animation.
    const adjustRef = (0,react.useRef)(true);
    const handleToggleExpand = (0,react.useCallback)((id)=>{
        adjustRef.current = false;
        setExpandedIds((prev)=>{
            const next = new Set(prev);
            if (next.has(id)) {
                next.delete(id);
            } else {
                next.add(id);
            }
            return next;
        });
        requestAnimationFrame(()=>{
            adjustRef.current = true;
        });
    }, []);
    // Build flat virtual items array from groups.
    const items = [];
    for (const { groupKey, displayName, entries } of groupedEntries){
        if (groupingStrategy !== useHistoryFilters_HistoryGroupingStrategy.None) {
            items.push({
                type: "header",
                groupKey,
                displayName,
                count: entries.length
            });
        }
        for (const entry of entries){
            items.push({
                type: "card",
                entry,
                flatIndex: flatEntries.indexOf(entry)
            });
        }
    }
    // Sentinel item for load-more trigger.
    if (hasNextPage) items.push({
        type: "header",
        groupKey: "__sentinel__",
        displayName: "",
        count: 0
    });
    const virtualizer = (0,esm/* useVirtualizer */.Te)({
        count: items.length,
        getScrollElement: ()=>scrollRef.current,
        estimateSize: ()=>88,
        overscan: 8,
        measureElement: (el)=>el.getBoundingClientRect().height
    });
    // Expose scrollToIndex so the parent keyboard handler can drive it.
    (0,react.useEffect)(()=>{
        if (virtualizerRef) {
            virtualizerRef.current = {
                scrollToIndex: (i)=>virtualizer.scrollToIndex(i, {
                        align: "auto"
                    })
            };
        }
    }, [
        virtualizer,
        virtualizerRef
    ]);
    // Infinite scroll: observe the sentinel item.
    const sentinelRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        if (!sentinelRef.current || !hasNextPage || loadingMore) return;
        const observer = new IntersectionObserver((entries)=>{
            var _entries_;
            if ((_entries_ = entries[0]) === null || _entries_ === void 0 ? void 0 : _entries_.isIntersecting) onLoadMore();
        }, {
            root: scrollRef.current,
            threshold: 0
        });
        observer.observe(sentinelRef.current);
        return ()=>observer.disconnect();
    }, [
        hasNextPage,
        loadingMore,
        onLoadMore
    ]);
    if (loading) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: loadingContainer,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: "spinner"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: loadingTitle,
                    children: "Loading Claude History..."
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: "text-muted",
                    style: {
                        fontSize: "14px"
                    },
                    children: entriesCount === 0 ? "This may take a few moments on first load..." : "Refreshing..."
                })
            ]
        });
    }
    if (filteredCount === 0) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: emptyStateContainer,
            children: hasActiveFilters ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: emptyStateIcon,
                        children: "\uD83D\uDD0D"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: emptyStateTitle,
                        children: "No results found"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                        className: "text-muted",
                        children: [
                            "Try adjusting your filters or",
                            " ",
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                onClick: onClearFilters,
                                className: linkButton,
                                children: "clear all filters"
                            })
                        ]
                    })
                ]
            }) : entriesCount === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: emptyStateIcon,
                        children: "\uD83D\uDCDA"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: emptyStateTitle,
                        children: "No conversation history yet"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: "text-muted",
                        children: "Start a Claude session to see history here."
                    })
                ]
            }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: emptyStateIcon,
                        children: "\uD83D\uDCED"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: emptyStateTitle,
                        children: "No entries match your criteria"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: "text-muted",
                        children: "Adjust your filters to see more results."
                    })
                ]
            })
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        ref: scrollRef,
        style: {
            overflowY: "auto",
            height: "100%",
            maxHeight: "calc(var(--viewport-height, 100dvh) - 280px)"
        },
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            style: {
                height: "".concat(virtualizer.getTotalSize(), "px"),
                position: "relative"
            },
            children: virtualizer.getVirtualItems().map((vItem)=>{
                const item = items[vItem.index];
                return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    "data-index": vItem.index,
                    ref: virtualizer.measureElement,
                    style: {
                        position: "absolute",
                        top: 0,
                        left: 0,
                        width: "100%",
                        transform: "translateY(".concat(vItem.start, "px)")
                    },
                    children: item.type === "header" ? item.groupKey === "__sentinel__" ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        ref: sentinelRef,
                        style: {
                            height: "1px"
                        },
                        "aria-hidden": "true",
                        children: loadingMore && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: "text-muted",
                            style: {
                                textAlign: "center",
                                padding: "8px",
                                fontSize: "13px"
                            },
                            children: "Loading more…"
                        })
                    }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("h3", {
                        className: categoryTitle,
                        style: {
                            padding: "8px 0 4px"
                        },
                        children: [
                            item.displayName,
                            " (",
                            item.count,
                            ")"
                        ]
                    }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        style: {
                            paddingBottom: "6px"
                        },
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(HistoryEntryCard_HistoryEntryCard, {
                            entry: item.entry,
                            isSelected: (selectedEntry === null || selectedEntry === void 0 ? void 0 : selectedEntry.id) === item.entry.id,
                            enrichedEntry: (selectedEntry === null || selectedEntry === void 0 ? void 0 : selectedEntry.id) === item.entry.id ? enrichedEntry : undefined,
                            isExpanded: expandedIds.has(item.entry.id),
                            onToggleExpand: handleToggleExpand,
                            onSelect: ()=>onSelectEntry(item.entry, item.flatIndex),
                            fetchMessages: fetchMessages
                        })
                    })
                }, vItem.key);
            })
        })
    });
}

// EXTERNAL MODULE: ./src/components/ui/RepoPathInput.tsx + 2 modules
var RepoPathInput = __webpack_require__(34609);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fhistory%2FForkModal.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA51UbY_iIBD-7q_gyyWaHIZWVyv7Yy4UaOW2BQJ0W93sf79APa4vVncvTTTtDDPPywzbqr5SZBH4WAGQK8O4wSDRHbCqEgy8E7OG8Fdy4tfUXPnmNWZBQ5hoLJ6k7LI-h9C30qhGsmnCMcSpqpSZhlAIacKYkCUGyL_VpIOtYO6MwT5DuvPfbu8n9N72eDpoz4SpFgMEUqQ7cPA_pszJGv0Mz3a3eV19rrY3thh7gMwo3fMeoB2deolkfC4sROW8PnnVmHWqu2HNJFSK4NN9j5UJqytywaCoePjg_yEThlMnlMReiaaWPlISjUFy8Odi1TRULZR00Iorn6ndOxLiLRfl2c0yzpteRlMKedN0UfzYdvekbb5sYjrtF4vuZ1qPT16Wi3bjych0B5K0F_jxNB7-6bNIJAJ8CQC_69jYsMOXlHtkWLksQzJEexwP3Nc1OQ62-OmuL7qVkGeLvCwCHfHAhaKNDWxU4yohOQZSST5gcrdLvZlvv39S3U1ncj9smI3uur-dptfOYF--OxHZaCJO96eKVKKUUDheWwwol46bcYH_3L9ee9oY62Naib5yhEMCnNsVmiD0I4ChHsGyzPF0fp_M78Y6UVwgVdJx6TCwmlAOc-5azuVjLg_mKB22pvdbDwWbwQhGcckGBkOnokV_AA6lhf79BgAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fhistory_2FForkModal_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA51UbY_iIBD_7q_gyyWaHIZWVyv7Yy4UaOW2BQJ0W93sf79APa4vVncvTTTtDDPPywzbqr5SZBH4WAGQK8O4wSDRHbCqEgy8E7OG8Fdy4tfUXPnmNWZBQ5hoLJ6k7LI_h9C30qhGsmnCMcSpqpSZhlAIacKYkCUGyL_VpIOtYO6MwT5DuvPfbu8n9N72eDpoz4SpFgMEUqQ7cPA_pszJGv0Mz3a3eV19rrY3thh7gMwo3fMeoB2deolkfC4sROW8PnnVmHWqu2HNJFSK4NN9j5UJqytywaCoePjg_yEThlMnlMReiaaWPlISjUFy8Odi1TRULZR00Iorn6ndOxLiLRfl2c0yzpteRlMKedN0UfzYdvekbb5sYjrtF4vuZ1qPT16Wi3bjych0B5K0F_jxNB7_6bNIJAJ8CQC_69jYsMOXlHtkWLksQzJEexwP3Nc1OQ62_OmuL7qVkGeLvCwCHfHAhaKNDWxU4yohOQZSST5gcrdLvZlvv39S3U1ncj9smI3uur_dptfOYF_OxHZaCJO96eKVKKUUDheWwwol46bcYH_3L9ee9oY62Naib5yhEMCnNsVmiD0I4ChHsGyzPF0fp_M78Y6UVwgVdJx6TCwmlAOc_5azuVjLg_mKB22pvdbDwWbwQhGcckGBkOnokV_AA6lhf79BgAA_22_7D = __webpack_require__(53853);
;// ./src/components/history/ForkModal.css.ts


var actions = 'lmzc0sc';
var dialog = 'lmzc0s0';
var errorBanner = 'lmzc0s4';
var field = 'lmzc0s5';
var ForkModal_css_form = 'lmzc0s1';
var ForkModal_css_input = 'lmzc0s7';
var ForkModal_css_label = 'lmzc0s6';
var radioGroup = 'lmzc0s8';
var radioLabel = 'lmzc0s9';
var slider = 'lmzc0sa';
var sliderLabels = 'lmzc0sb';
var subtitle = 'lmzc0s3';
var ForkModal_css_title = 'lmzc0s2';
;// ./src/components/history/ForkModal.tsx
/* __next_internal_client_entry_do_not_use__ ForkModal auto */ 




function ForkModal(param) {
    let { entry, submitting, error, onClose, onSubmit } = param;
    const dialogRef = (0,react.useRef)(null);
    const [title, setTitle] = (0,react.useState)("");
    const [path, setPath] = (0,react.useState)("");
    const [branch, setBranch] = (0,react.useState)("");
    const [sessionType, setSessionType] = (0,react.useState)(types_pb/* SessionType */.dF.DIRECTORY);
    const [forkAt, setForkAt] = (0,react.useState)(0);
    // Open/close dialog and populate fields when entry changes.
    (0,react.useEffect)(()=>{
        var _dialogRef_current;
        if (!entry) {
            var _dialogRef_current1;
            (_dialogRef_current1 = dialogRef.current) === null || _dialogRef_current1 === void 0 ? void 0 : _dialogRef_current1.close();
            return;
        }
        setTitle(entry.name.substring(0, 60));
        setPath(entry.project);
        setBranch("resume/".concat(entry.id.substring(0, 8)));
        setForkAt(0);
        setSessionType(types_pb/* SessionType */.dF.DIRECTORY);
        (_dialogRef_current = dialogRef.current) === null || _dialogRef_current === void 0 ? void 0 : _dialogRef_current.showModal();
    }, [
        entry
    ]);
    // Close on Escape via dialog native behavior — also fire onClose.
    (0,react.useEffect)(()=>{
        const el = dialogRef.current;
        if (!el) return;
        const handler = ()=>onClose();
        el.addEventListener("close", handler);
        return ()=>el.removeEventListener("close", handler);
    }, [
        onClose
    ]);
    if (!entry) return null;
    const handleSubmit = (e)=>{
        e.preventDefault();
        onSubmit({
            title: title.trim(),
            path,
            branch: branch.trim(),
            sessionType,
            forkAtMessage: forkAt
        });
    };
    const maxMsg = entry.messageCount;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("dialog", {
        ref: dialogRef,
        className: dialog,
        "aria-labelledby": "fork-modal-title",
        onClick: (e)=>{
            if (e.target === dialogRef.current) onClose();
        },
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("form", {
            onSubmit: handleSubmit,
            className: ForkModal_css_form,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                    id: "fork-modal-title",
                    className: ForkModal_css_title,
                    children: "Fork Conversation"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: subtitle,
                    children: "Create a new session branching from this conversation."
                }),
                error && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: errorBanner,
                    children: error
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: field,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                            className: ForkModal_css_label,
                            htmlFor: "fork-title",
                            children: "Session name"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                            id: "fork-title",
                            type: "text",
                            className: ForkModal_css_input,
                            value: title,
                            onChange: (e)=>setTitle(e.target.value),
                            required: true,
                            autoFocus: true,
                            maxLength: 100
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: field,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                            className: ForkModal_css_label,
                            htmlFor: "fork-path",
                            children: "Directory"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)(RepoPathInput/* RepoPathInput */.E, {
                            id: "fork-path",
                            value: path,
                            onChange: setPath,
                            required: true
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: field,
                    children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("fieldset", {
                        className: radioGroup,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("legend", {
                                className: ForkModal_css_label,
                                children: "Session type"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: radioLabel,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        type: "radio",
                                        name: "session-type",
                                        value: "directory",
                                        checked: sessionType === types_pb/* SessionType */.dF.DIRECTORY,
                                        onChange: ()=>setSessionType(types_pb/* SessionType */.dF.DIRECTORY)
                                    }),
                                    "Open in directory"
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: radioLabel,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        type: "radio",
                                        name: "session-type",
                                        value: "worktree",
                                        checked: sessionType === types_pb/* SessionType */.dF.NEW_WORKTREE,
                                        onChange: ()=>setSessionType(types_pb/* SessionType */.dF.NEW_WORKTREE)
                                    }),
                                    "New git worktree"
                                ]
                            })
                        ]
                    })
                }),
                sessionType === types_pb/* SessionType */.dF.NEW_WORKTREE && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: field,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                            className: ForkModal_css_label,
                            htmlFor: "fork-branch",
                            children: "Branch name"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                            id: "fork-branch",
                            type: "text",
                            className: ForkModal_css_input,
                            value: branch,
                            onChange: (e)=>setBranch(e.target.value)
                        })
                    ]
                }),
                maxMsg > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: field,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                            className: ForkModal_css_label,
                            htmlFor: "fork-at",
                            children: [
                                "Fork at message ",
                                forkAt === 0 ? "(all messages)" : "".concat(forkAt, " of ").concat(maxMsg)
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                            id: "fork-at",
                            type: "range",
                            min: 0,
                            max: maxMsg,
                            value: forkAt,
                            onChange: (e)=>setForkAt(Number(e.target.value)),
                            className: slider
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: sliderLabels,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    children: "Start"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    children: [
                                        "All ",
                                        maxMsg,
                                        " messages"
                                    ]
                                })
                            ]
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: actions,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            type: "button",
                            onClick: onClose,
                            className: "btn btn-secondary",
                            disabled: submitting,
                            children: "Cancel"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            type: "submit",
                            className: "btn btn-primary",
                            disabled: submitting || !title.trim(),
                            children: submitting ? "Forking…" : "🍴 Fork Session"
                        })
                    ]
                })
            ]
        })
    });
}

;// ./src/components/history/index.ts










// EXTERNAL MODULE: ./src/lib/hooks/useHistoryFullTextSearch.ts
var useHistoryFullTextSearch = __webpack_require__(86082);
;// ./src/lib/hooks/useHistoryGrouping.ts
/* __next_internal_client_entry_do_not_use__ useHistoryGrouping auto */ 


// ============================================================================
// Hook
// ============================================================================
function useHistoryGrouping(filteredEntries, groupingStrategy) {
    // Group entries
    const groupedEntries = (0,react.useMemo)(()=>{
        if (groupingStrategy === useHistoryFilters_HistoryGroupingStrategy.None) {
            return [
                {
                    groupKey: "all",
                    displayName: "All Entries",
                    entries: filteredEntries
                }
            ];
        }
        const groups = new Map();
        filteredEntries.forEach((entry)=>{
            let groupKey;
            switch(groupingStrategy){
                case useHistoryFilters_HistoryGroupingStrategy.Date:
                    groupKey = (0,utils_timestamp/* getDateGroup */.O5)(entry.updatedAt);
                    break;
                case useHistoryFilters_HistoryGroupingStrategy.Project:
                    groupKey = entry.project || "No Project";
                    break;
                case useHistoryFilters_HistoryGroupingStrategy.Model:
                    groupKey = entry.model || "Unknown Model";
                    break;
                default:
                    groupKey = "all";
            }
            if (!groups.has(groupKey)) {
                groups.set(groupKey, []);
            }
            groups.get(groupKey).push(entry);
        });
        // Sort groups (Date groups have specific order)
        const dateOrder = [
            "Today",
            "Yesterday",
            "This Week",
            "This Month",
            "Older",
            "Unknown"
        ];
        const sortedGroups = Array.from(groups.entries()).sort((param, param1)=>{
            let [a] = param, [b] = param1;
            if (groupingStrategy === useHistoryFilters_HistoryGroupingStrategy.Date) {
                return dateOrder.indexOf(a) - dateOrder.indexOf(b);
            }
            return a.localeCompare(b);
        });
        return sortedGroups.map((param)=>{
            let [key, entries] = param;
            return {
                groupKey: key,
                displayName: key,
                entries
            };
        });
    }, [
        filteredEntries,
        groupingStrategy
    ]);
    // Flatten for keyboard navigation
    const flatEntries = (0,react.useMemo)(()=>{
        return groupedEntries.flatMap((g)=>g.entries);
    }, [
        groupedEntries
    ]);
    return {
        groupedEntries,
        flatEntries
    };
}

// EXTERNAL MODULE: ./src/lib/contexts/AnalyticsContext.tsx
var AnalyticsContext = __webpack_require__(24044);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Fhistory%2Fhistory.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA51X227iMBB95yvmZaUi1cgJNKTuy_5J5cQOuCR25DglZdV_X8VOyAVToBIFyR7PzDk-M-Ou3jfyg8YRhn8LgJIyJuSOQIjL5m0BUNAGHQUzewJBhM-LeickAQy0NsquCIn2XOz2hkBK8_Tpk-onhD4FP5ZKm27vGQKM2ed-CQhiXDbL9igTVZnTLwJZzq339hcxoXlqhJIEUpXXhXxbfC9WXa6BzfXi4EddGZF9oVRJw6UhUJU05Sjh5si5bC1oLnYSCcOLikDKpeF6wIMSZYwqWqAtzCFcaMNlShpUiRMnEG4cD3bp2MFOVM7axVTlShNwBLwHr_wU6hNeTngbe1_7wVxJdUdLAvEovMso6DLyBg9s8ISmh51WtWTz_a3dP199VDYQhM5fojTjGmnKRF3ZrWGVQFA2UKlcsJlDvhwD3Mzp6517kw0nZ18eJyfokjzjacOdV8cs6F1Cn8IQP8M6dn941XF1AdBnu156KIrHJXJVUtEvcHWstbYEgrG37YVA8TRcPBj0co0wvkutZwjh1OWrH4FN9VwffarTBjGRP3WpTS27joM9Jx9oGADqk-ssV0cCe8EYnzSRxAae31N4WVqRp9hvsTeESe_sVbPb7vuvX8Vd1kaN9H6rvu8p2mtq_l3xMjdSVCXcvWSi4bZF2rwtgTnP-mvVoyvurwN7SxY_Q_dZRf4RcqWOfiD9hIRkVoPYXe1sEg6ouEV1ne3oLrY_fGz33A6x47KBdbfaj2GM_8wm80uvlkQ1qNpT1ioe28whar_mtK0fm7yTzjowkV009thTKts7G82sL-wunK9vjjivo72_AG9gnUEV12bY3V3BZWh4Y5DRVFaZ0gWBuiy5TmnFXTEYwzVqXy329vEKb3gxTuPjhvYCepf4Dj7xRT_RmyyvPDjOUo1Hr4aZUq0qxcmadUETZQ1VbXIhOQGpJJ8AJZlK68rBdUe8ieWThnMYbimjhci_CBRKKvsMfLSNPfpgwt4HU8fSUWmGEs3pgYD9QTTPx5nnd04Jq1ou2VCT0znhRsLGifdvwZmgUKWacwlUMngaNY1tFJfN0sZdTf4HGA-dThTfI6OgM7peQLP-aw0rQ7Vxm5PXzNhz2nk-0-BU0RrdAhPg9s0xRfN6M9Hvxfd_LuJzLf8MAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Fhistory_2Fhistory_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA51X227iMBB95yvmZaUi1cgJNKTuy_5J5cQOuCR25DglZdV_X8VOyAVToBIFyR7PzDk_M_Ou3jfyg8YRhn8LgJIyJuSOQIjL5m0BUNAGHQUzewJBhM_LeickAQy0NsquCIn2XOz2hkBK8_Tpk_onhD4FP5ZKm27vGQKM2ed_CQhiXDbL9igTVZnTLwJZzq339hcxoXlqhJIEUpXXhXxbfC9WXa6BzfXi4EddGZF9oVRJw6UhUJU05Sjh5si5bC1oLnYSCcOLikDKpeF6wIMSZYwqWqAtzCFcaMNlShpUiRMnEG4cD3bp2MFOVM7axVTlShNwBLwHr_wU6hNeTngbe1_7wVxJdUdLAvEovMso6DLyBg9s8ISmh51WtWTz_a3dP199VDYQhM5fojTjGmnKRF3ZrWGVQFA2UKlcsJlDvhwD3Mzp6517kw0nZ18eJyfokjzjacOdV8cs6F1Cn8IQP8M6dn941XF1AdBnu156KIrHJXJVUtEvcHWstbYEgrG37YVA8TRcPBj0co0wvkutZwjh1OWrH4FN9VwffarTBjGRP3WpTS27joM9Jx9oGADqk_ssV0cCe8EYnzSRxAae31N4WVqRp9hvsTeESe_sVbPb7vuvX8Vd1kaN9H6rvu8p2mtq_l3xMjdSVCXcvWSi4bZF2rwtgTnP_mvVoyvurwN7SxY_Q_dZRf4RcqWOfiD9hIRkVoPYXe1sEg6ouEV1ne3oLrY_fGz33A6x47KBdbfaj2GM_8wm80uvlkQ1qNpT1ioe28whar_mtK0fm7yTzjowkV009thTKts7G82sL_wunK9vjjivo72_AG9gnUEV12bY3V3BZWh4Y5DRVFaZ0gWBuiy5TmnFXTEYwzVqXy329vEKb3gxTuPjhvYCepf4Dj7xRT_RmyyvPDjOUo1Hr4aZUq0qxcmadUETZQ1VbXIhOQGpJJ8AJZlK68rBdUe8ieWThnMYbimjhci_CBRKKvsMfLSNPfpgwt4HU8fSUWmGEs3pgYD9QTTPx5nnd04Jq1ou2VCT0znhRsLGifdvwZmgUKWacwlUMngaNY1tFJfN0sZdTf4HGA_dThTfI6OgM7peQLP_aw0rQ7Vxm5PXzNhz2nk_0_BU0RrdAhPg9s0xRfN6M9Hvxfd_LuJzLf8MAAA_22_7D = __webpack_require__(81506);
;// ./src/app/history/history.css.ts


var history_css_container = '_4nja860';
var content = '_4nja869';
var entryList = '_4nja86a';
var history_css_errorBanner = '_4nja865';
var errorContent = '_4nja866';
var history_css_errorIcon = '_4nja867';
var errorTitle = '_4nja868';
var groupingIndicator = '_4nja863';
var history_css_header = '_4nja861';
var keyboardHints = '_4nja86c';
var history_css_modalOverlay = '_4nja86d';
var resumeModal = '_4nja86e';
var resumeModalActions = '_4nja86l';
var resumeModalField = '_4nja86h';
var resumeModalInput = '_4nja86j';
var resumeModalLabel = '_4nja86i';
var resumeModalPath = '_4nja86k';
var resumeModalSubtitle = '_4nja86g';
var resumeModalTitle = '_4nja86f';
var history_css_sectionTitle = '_4nja86b';
var shortcutHint = '_4nja864';
var title = '_4nja862';
;// ./src/app/history/page.tsx
/* __next_internal_client_entry_do_not_use__ default auto */ 
// +feature: history-search history-list














function HistoryBrowserPageInner() {
    (0,usePageView/* usePageView */.O)();
    const { track } = (0,AnalyticsContext/* useAnalytics */.st)();
    const router = (0,nav/* useRouter */.KA)();
    const searchParams = (0,nav/* useSearchParams */.ok)();
    // Core state
    const [entries, setEntries] = (0,react.useState)([]);
    const [nextPageToken, setNextPageToken] = (0,react.useState)("");
    const [loadingMore, setLoadingMore] = (0,react.useState)(false);
    const [selectedEntry, setSelectedEntry] = (0,react.useState)(null);
    const [selectedIndex, setSelectedIndex] = (0,react.useState)(-1);
    const [messages, setMessages] = (0,react.useState)([]);
    const [isMessagesOpen, setShowMessages] = (0,react.useState)(false);
    const [loadingMessages, setLoadingMessages] = (0,react.useState)(false);
    const [previewMessages, setPreviewMessages] = (0,react.useState)([]);
    const [loadingPreview, setLoadingPreview] = (0,react.useState)(false);
    const [loading, setLoading] = (0,react.useState)(true);
    const [searching, setSearching] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    const [resuming, setResuming] = (0,react.useState)(false);
    const [messageSearchQuery, setMessageSearchQuery] = (0,react.useState)("");
    const [resumeTarget, setResumeTarget] = (0,react.useState)(null);
    const [resumeTitle, setResumeTitle] = (0,react.useState)("");
    const [forkTarget, setForkTarget] = (0,react.useState)(null);
    const [forking, setForking] = (0,react.useState)(false);
    const [forkError, setForkError] = (0,react.useState)(null);
    const virtualizerRef = (0,react.useRef)(null);
    // Hooks
    const { filterState, setters, derived, actions } = useHistoryFilters(entries);
    const { searchQuery, branchFilter, selectedModel, dateFilter, sortField, sortOrder, groupingStrategy, searchMode } = filterState;
    const { setSearchQuery, setBranchFilter, setSelectedModel, setDateFilter, setSortField, setSortOrder, setGroupingStrategy, setSearchMode } = setters;
    const { uniqueModels, filteredEntries, hasActiveFilters } = derived;
    const { clearFilters, cycleGroupingStrategy } = actions;
    const fullTextSearch = (0,useHistoryFullTextSearch/* useHistoryFullTextSearch */.c)({
        debounceMs: 300,
        autoSearch: true
    });
    const { groupedEntries, flatEntries } = useHistoryGrouping(filteredEntries, groupingStrategy);
    // Refs
    const clientRef = (0,react.useRef)(null);
    const searchInputRef = (0,react.useRef)(null);
    const entryListRef = (0,react.useRef)(null);
    // Initialize ConnectRPC client and load data
    (0,react.useEffect)(()=>{
        const transport = (0,connect_transport/* createConnectTransport */.w)({
            baseUrl: (0,config/* getApiBaseUrl */.h)()
        });
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, transport);
    }, []);
    // Data loading callbacks
    const loadHistory = (0,react.useCallback)(async (query)=>{
        if (!clientRef.current) return;
        try {
            setLoading(true);
            setError(null);
            const response = await clientRef.current.listClaudeHistory({
                pageSize: 100,
                searchQuery: query
            });
            setEntries(response.entries);
            setNextPageToken(response.nextPageToken);
        } catch (err) {
            setError("Failed to load history: ".concat(err));
        } finally{
            setLoading(false);
        }
    }, []);
    (0,react.useEffect)(()=>{
        loadHistory();
    }, [
        loadHistory
    ]);
    const loadMoreHistory = (0,react.useCallback)(async ()=>{
        if (!clientRef.current || !nextPageToken || loadingMore) return;
        try {
            setLoadingMore(true);
            setError(null);
            const response = await clientRef.current.listClaudeHistory({
                pageSize: 100,
                pageToken: nextPageToken
            });
            setEntries((prev)=>[
                    ...prev,
                    ...response.entries
                ]);
            setNextPageToken(response.nextPageToken);
        } catch (err) {
            setError("Failed to load more history: ".concat(err));
        } finally{
            setLoadingMore(false);
        }
    }, [
        nextPageToken,
        loadingMore
    ]);
    const loadEntryDetail = (0,react.useCallback)(async (id)=>{
        if (!clientRef.current) return;
        try {
            setError(null);
            setLoadingPreview(true);
            setPreviewMessages([]);
            const [detailResponse, messagesResponse] = await Promise.all([
                clientRef.current.getClaudeHistoryDetail({
                    id
                }),
                clientRef.current.getClaudeHistoryMessages({
                    id,
                    limit: 5,
                    tail: true
                })
            ]);
            if (detailResponse.entry) setSelectedEntry(detailResponse.entry);
            if (messagesResponse.messages) setPreviewMessages([
                ...messagesResponse.messages
            ].reverse());
        } catch (err) {
            setError("Failed to load entry details: ".concat(err));
        } finally{
            setLoadingPreview(false);
        }
    }, []);
    // Deep-link support: ?sessionId=<id>&messageIndex=<n> opens that session
    // directly on mount (e.g. from TriageRelatedWorkSection's result cards),
    // bypassing the normal list-click flow. messageIndex centers the messages
    // modal on that index when present; sessionId alone falls back to the
    // session's default (unanchored) view.
    (0,react.useEffect)(()=>{
        const deepLinkSessionId = searchParams.get("sessionId");
        if (!deepLinkSessionId || !clientRef.current) return;
        loadEntryDetail(deepLinkSessionId);
        const messageIndexParam = searchParams.get("messageIndex");
        if (messageIndexParam === null) return;
        const messageIndex = Number(messageIndexParam);
        if (!Number.isFinite(messageIndex)) return;
        (async ()=>{
            if (!clientRef.current) return;
            try {
                const response = await clientRef.current.getClaudeHistoryMessages({
                    id: deepLinkSessionId,
                    anchorIndex: messageIndex,
                    limit: 21
                });
                setMessages(response.messages);
                setShowMessages(true);
            } catch (err) {
                setError("Failed to load anchored messages: ".concat(err));
            }
        })();
    // Only run once on mount — the deep link should not re-fire on unrelated re-renders.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
    const handleSearch = (0,react.useCallback)(async ()=>{
        setSearching(true);
        try {
            await loadHistory(searchQuery || undefined);
        } finally{
            setSearching(false);
        }
    }, [
        searchQuery,
        loadHistory
    ]);
    const loadMessages = (0,react.useCallback)(async (id)=>{
        if (!clientRef.current) return;
        try {
            setLoadingMessages(true);
            setError(null);
            const response = await clientRef.current.getClaudeHistoryMessages({
                id
            });
            setMessages(response.messages);
            setShowMessages(true);
            setMessageSearchQuery("");
        } catch (err) {
            setError("Failed to load messages: ".concat(err));
        } finally{
            setLoadingMessages(false);
        }
    }, []);
    // Event handlers
    const selectEntry = (0,react.useCallback)((entry, index)=>{
        setSelectedIndex(index);
        loadEntryDetail(entry.id);
    }, [
        loadEntryDetail
    ]);
    const handleSearchResultClick = (0,react.useCallback)((result)=>{
        const existingEntry = entries.find((e)=>e.id === result.sessionId);
        if (existingEntry) {
            const index = flatEntries.indexOf(existingEntry);
            setSelectedIndex(index >= 0 ? index : 0);
            loadEntryDetail(existingEntry.id);
        } else {
            loadEntryDetail(result.sessionId);
        }
        setSearchMode("metadata");
    }, [
        entries,
        flatEntries,
        loadEntryDetail,
        setSearchMode
    ]);
    const handleCopyId = (0,react.useCallback)(async (id)=>{
        try {
            await navigator.clipboard.writeText(id);
        } catch (err) {
            console.error("Failed to copy ID:", err);
        }
    }, []);
    const handleExportEntry = (0,react.useCallback)(async (entry)=>{
        if (!clientRef.current) return;
        try {
            const response = await clientRef.current.getClaudeHistoryMessages({
                id: entry.id
            });
            const exportData = {
                name: entry.name,
                id: entry.id,
                project: entry.project,
                model: entry.model,
                messageCount: entry.messageCount,
                createdAt: entry.createdAt ? new Date(Number(entry.createdAt.seconds) * 1000).toISOString() : null,
                updatedAt: entry.updatedAt ? new Date(Number(entry.updatedAt.seconds) * 1000).toISOString() : null,
                messages: response.messages.map((msg)=>({
                        role: msg.role,
                        content: msg.content,
                        model: msg.model,
                        timestamp: msg.timestamp ? new Date(Number(msg.timestamp.seconds) * 1000).toISOString() : null
                    }))
            };
            const blob = new Blob([
                JSON.stringify(exportData, null, 2)
            ], {
                type: "application/json"
            });
            const url = URL.createObjectURL(blob);
            const a = document.createElement("a");
            a.href = url;
            a.download = "".concat(entry.name.replace(/[^a-z0-9]/gi, "_"), "_").concat(entry.id.substring(0, 8), ".json");
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            URL.revokeObjectURL(url);
        } catch (err) {
            setError("Failed to export: ".concat(err));
        }
    }, []);
    const openResumeModal = (0,react.useCallback)((entry)=>{
        if (!entry.project) {
            setError("Cannot resume: No project path recorded for this session");
            return;
        }
        setResumeTarget(entry);
        setResumeTitle(entry.name.substring(0, 60));
    }, []);
    const handleResumeSession = (0,react.useCallback)(async ()=>{
        if (!clientRef.current || !resumeTarget) return;
        if (!resumeTarget.project) {
            setError("Cannot resume: Project path is required");
            return;
        }
        const title = resumeTitle.trim() || resumeTarget.name.substring(0, 60);
        try {
            setResuming(true);
            setError(null);
            track({
                name: "history_resume_session",
                category: "user_action"
            });
            const response = await clientRef.current.createSession({
                title,
                path: resumeTarget.project,
                resumeId: resumeTarget.id,
                category: "Resumed",
                sessionType: types_pb/* SessionType */.dF.DIRECTORY
            });
            if (response.session) {
                setResumeTarget(null);
                router.push("/");
            }
        } catch (err) {
            setError("Failed to resume session: ".concat(err));
        } finally{
            setResuming(false);
        }
    }, [
        router,
        resumeTarget,
        resumeTitle,
        track
    ]);
    // Callback for card inline previews — fetches last 5 messages.
    const fetchPreview = (0,react.useCallback)(async (id)=>{
        if (!clientRef.current) return [];
        const res = await clientRef.current.getClaudeHistoryMessages({
            id,
            limit: 5,
            tail: true
        });
        return [
            ...res.messages
        ].reverse();
    }, []);
    const openForkModal = (0,react.useCallback)((entry)=>{
        setForkError(null);
        setForkTarget(entry);
    }, []);
    const handleFork = (0,react.useCallback)(async (params)=>{
        if (!clientRef.current || !forkTarget) return;
        try {
            setForking(true);
            setForkError(null);
            track({
                name: "history_fork_session",
                category: "user_action"
            });
            const response = await clientRef.current.createSession({
                title: params.title,
                path: params.path,
                branch: params.sessionType === types_pb/* SessionType */.dF.NEW_WORKTREE ? params.branch : undefined,
                sessionType: params.sessionType,
                forkSourceId: forkTarget.id,
                forkAtMessage: params.forkAtMessage,
                category: "Forked"
            });
            if (response.session) {
                setForkTarget(null);
                router.push("/");
            }
        } catch (err) {
            setForkError("Fork failed: ".concat(err));
        } finally{
            setForking(false);
        }
    }, [
        forkTarget,
        router,
        track
    ]);
    // Keyboard navigation
    (0,react.useEffect)(()=>{
        const handleKeyDown = (e)=>{
            var _document_activeElement, _document_activeElement1;
            const isInInput = ((_document_activeElement = document.activeElement) === null || _document_activeElement === void 0 ? void 0 : _document_activeElement.tagName) === "INPUT" || ((_document_activeElement1 = document.activeElement) === null || _document_activeElement1 === void 0 ? void 0 : _document_activeElement1.tagName) === "TEXTAREA";
            if (resumeTarget) {
                if (e.key === "Escape") {
                    e.preventDefault();
                    setResumeTarget(null);
                }
                return;
            }
            if (isMessagesOpen) {
                if (e.key === "Escape") {
                    e.preventDefault();
                    setShowMessages(false);
                }
                return;
            }
            if (e.key === "/" && !isInInput) {
                var _searchInputRef_current;
                e.preventDefault();
                (_searchInputRef_current = searchInputRef.current) === null || _searchInputRef_current === void 0 ? void 0 : _searchInputRef_current.focus();
                return;
            }
            if (e.key === "Escape") {
                if (isInInput) {
                    var _document_activeElement2;
                    (_document_activeElement2 = document.activeElement) === null || _document_activeElement2 === void 0 ? void 0 : _document_activeElement2.blur();
                    return;
                }
                if (searchQuery) {
                    setSearchQuery("");
                    return;
                }
            }
            if (isInInput) return;
            if (e.key === "ArrowDown" || e.key === "j") {
                e.preventDefault();
                const newIndex = Math.min(selectedIndex + 1, flatEntries.length - 1);
                if (newIndex >= 0 && flatEntries[newIndex]) {
                    var _virtualizerRef_current;
                    selectEntry(flatEntries[newIndex], newIndex);
                    (_virtualizerRef_current = virtualizerRef.current) === null || _virtualizerRef_current === void 0 ? void 0 : _virtualizerRef_current.scrollToIndex(newIndex);
                }
                return;
            }
            if (e.key === "ArrowUp" || e.key === "k") {
                e.preventDefault();
                const newIndex = Math.max(selectedIndex - 1, 0);
                if (flatEntries[newIndex]) {
                    var _virtualizerRef_current1;
                    selectEntry(flatEntries[newIndex], newIndex);
                    (_virtualizerRef_current1 = virtualizerRef.current) === null || _virtualizerRef_current1 === void 0 ? void 0 : _virtualizerRef_current1.scrollToIndex(newIndex);
                }
                return;
            }
            if (e.key === "Enter" && selectedEntry) {
                e.preventDefault();
                loadMessages(selectedEntry.id);
                return;
            }
            if (e.key === "g" || e.key === "G") {
                e.preventDefault();
                cycleGroupingStrategy();
                return;
            }
            if (e.key === "?") {
                e.preventDefault();
                return;
            }
        };
        window.addEventListener("keydown", handleKeyDown);
        return ()=>window.removeEventListener("keydown", handleKeyDown);
    }, [
        resumeTarget,
        isMessagesOpen,
        searchQuery,
        selectedIndex,
        selectedEntry,
        flatEntries,
        selectEntry,
        loadMessages,
        cycleGroupingStrategy,
        setSearchQuery
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("main", {
        id: "main-content",
        className: history_css_container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: history_css_header,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h1", {
                        className: title,
                        children: "\uD83D\uDCDA Claude History Browser"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: groupingIndicator,
                        children: [
                            "\uD83D\uDCCA ",
                            GroupingStrategyLabels[groupingStrategy],
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: shortcutHint,
                                children: "(Press G to cycle)"
                            })
                        ]
                    })
                ]
            }),
            error && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: history_css_errorBanner,
                role: "alert",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: errorContent,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: history_css_errorIcon,
                                children: "⚠️"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                        className: errorTitle,
                                        children: "Error"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                        className: "text-muted",
                                        children: error
                                    })
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        onClick: ()=>loadHistory(searchQuery || undefined),
                        className: "btn btn-secondary btn-sm",
                        children: "Retry"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        onClick: ()=>setError(null),
                        className: "btn btn-ghost btn-sm",
                        "aria-label": "Dismiss error",
                        children: "✕"
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(HistoryFilterBar, {
                searchQuery: searchQuery,
                branchFilter: branchFilter,
                selectedModel: selectedModel,
                dateFilter: dateFilter,
                sortField: sortField,
                sortOrder: sortOrder,
                groupingStrategy: groupingStrategy,
                searchMode: searchMode,
                setSearchQuery: setSearchQuery,
                setBranchFilter: setBranchFilter,
                setSelectedModel: setSelectedModel,
                setDateFilter: setDateFilter,
                setSortField: setSortField,
                setSortOrder: setSortOrder,
                setGroupingStrategy: setGroupingStrategy,
                setSearchMode: setSearchMode,
                uniqueModels: uniqueModels,
                hasActiveFilters: hasActiveFilters,
                searching: searching,
                onSearch: handleSearch,
                onClearFilters: clearFilters,
                searchInputRef: searchInputRef,
                fullTextSearch: fullTextSearch
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: content,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: entryList,
                        ref: entryListRef,
                        children: searchMode === "fulltext" ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                                    className: history_css_sectionTitle,
                                    children: "Full-Text Search"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)(HistorySearchResults, {
                                    results: fullTextSearch.results,
                                    totalMatches: fullTextSearch.totalMatches,
                                    queryTimeMs: fullTextSearch.queryTimeMs,
                                    hasMore: fullTextSearch.hasMore,
                                    loading: fullTextSearch.loading,
                                    error: fullTextSearch.error,
                                    query: fullTextSearch.query,
                                    onResultClick: handleSearchResultClick,
                                    onLoadMore: fullTextSearch.loadMore
                                })
                            ]
                        }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("h2", {
                                    className: history_css_sectionTitle,
                                    children: [
                                        "History (",
                                        filteredEntries.length,
                                        " of ",
                                        entries.length,
                                        " entries",
                                        nextPageToken ? "+" : "",
                                        ")"
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: entryList,
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(VirtualHistoryList, {
                                        groupedEntries: groupedEntries,
                                        flatEntries: flatEntries,
                                        selectedEntry: selectedEntry,
                                        enrichedEntry: selectedEntry,
                                        loading: loading,
                                        entriesCount: entries.length,
                                        filteredCount: filteredEntries.length,
                                        hasActiveFilters: hasActiveFilters,
                                        groupingStrategy: groupingStrategy,
                                        hasNextPage: !!nextPageToken,
                                        loadingMore: loadingMore,
                                        onSelectEntry: selectEntry,
                                        onClearFilters: clearFilters,
                                        onLoadMore: loadMoreHistory,
                                        fetchMessages: fetchPreview,
                                        virtualizerRef: virtualizerRef
                                    })
                                })
                            ]
                        })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(HistoryDetailPanel, {
                        entry: selectedEntry,
                        previewMessages: previewMessages,
                        loadingPreview: loadingPreview,
                        loadingMessages: loadingMessages,
                        resuming: resuming,
                        onResume: openResumeModal,
                        onFork: openForkModal,
                        onViewMessages: loadMessages,
                        onExport: handleExportEntry,
                        onCopyId: handleCopyId
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: keyboardHints,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("kbd", {
                                children: "/"
                            }),
                            " Search"
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("kbd", {
                                children: "↑↓"
                            }),
                            " Navigate"
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("kbd", {
                                children: "Enter"
                            }),
                            " View Messages"
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("kbd", {
                                children: "G"
                            }),
                            " Cycle Grouping"
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("kbd", {
                                children: "Esc"
                            }),
                            " Clear/Close"
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(HistoryMessagesModal, {
                open: isMessagesOpen,
                messages: messages,
                messageSearchQuery: messageSearchQuery,
                onSearchChange: setMessageSearchQuery,
                onClose: ()=>setShowMessages(false)
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(ForkModal, {
                entry: forkTarget,
                submitting: forking,
                error: forkError,
                onClose: ()=>setForkTarget(null),
                onSubmit: handleFork
            }),
            resumeTarget && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: history_css_modalOverlay,
                onClick: (e)=>{
                    if (e.target === e.currentTarget) setResumeTarget(null);
                },
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: resumeModal,
                    role: "dialog",
                    "aria-modal": "true",
                    "aria-labelledby": "resume-modal-title",
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                            id: "resume-modal-title",
                            className: resumeModalTitle,
                            children: "Resume Session"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                            className: resumeModalSubtitle,
                            children: "This will start a new session continuing the conversation with Claude."
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: resumeModalField,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                    htmlFor: "resume-title",
                                    className: resumeModalLabel,
                                    children: "Session name"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                    id: "resume-title",
                                    type: "text",
                                    className: resumeModalInput,
                                    value: resumeTitle,
                                    onChange: (e)=>setResumeTitle(e.target.value),
                                    onKeyDown: (e)=>{
                                        if (e.key === "Enter") handleResumeSession();
                                        if (e.key === "Escape") setResumeTarget(null);
                                    },
                                    autoFocus: true,
                                    maxLength: 100
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: resumeModalField,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: resumeModalLabel,
                                    children: "Directory"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                                    className: resumeModalPath,
                                    children: resumeTarget.project
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: resumeModalActions,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    onClick: ()=>setResumeTarget(null),
                                    className: "btn btn-secondary",
                                    children: "Cancel"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    onClick: handleResumeSession,
                                    disabled: resuming || !resumeTitle.trim(),
                                    className: "btn btn-primary",
                                    children: resuming ? "Starting..." : "▶️ Resume"
                                })
                            ]
                        })
                    ]
                })
            })
        ]
    });
}
function HistoryBrowserPage() {
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(react.Suspense, {
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(HistoryBrowserPageInner, {})
    });
}


/***/ }),

/***/ 44762:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   C: () => (/* binding */ usePathCompletions)
/* harmony export */ });
/* unused harmony export clearCompletionCache */
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(66144);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(23575);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(21694);
/* __next_internal_client_entry_do_not_use__ clearCompletionCache,usePathCompletions auto */ 



const DEBOUNCE_MS = 150;
const CACHE_MAX = 100;
const CACHE_TTL_MS = 30000;
const completionCache = new Map();
/** Clears all cached completions. Exposed for testing. */ function clearCompletionCache() {
    completionCache.clear();
}
function getCached(key) {
    const entry = completionCache.get(key);
    if (!entry) return null;
    if (Date.now() > entry.expiresAt) {
        completionCache.delete(key);
        return null;
    }
    // Refresh LRU position.
    completionCache.delete(key);
    completionCache.set(key, entry);
    return entry;
}
function setCached(key, value) {
    if (completionCache.size >= CACHE_MAX) {
        const firstKey = completionCache.keys().next().value;
        if (firstKey !== undefined) {
            completionCache.delete(firstKey);
        }
    }
    completionCache.set(key, value);
}
/**
 * Fetches real-time filesystem path completions for a given path prefix.
 *
 * Three-layer protection against redundant requests:
 *  1. 150ms debounce – coalesces rapid keystrokes.
 *  2. AbortController – cancels the in-flight HTTP request when the prefix changes.
 *  3. Generation counter – discards responses that arrive after a newer request fired.
 *
 * Results are cached in a module-level LRU (100 entries, 30s TTL) to avoid
 * redundant RPCs for burst typing that returns to the same prefix.
 */ function usePathCompletions(pathPrefix) {
    let options = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : {};
    const { enabled = true, directoriesOnly = true, maxResults = 50 } = options;
    const [result, setResult] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)({
        entries: [],
        baseDir: "",
        baseDirExists: false,
        pathExists: false,
        isLoading: false,
        error: null
    });
    const generationRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(0);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        if (!enabled || pathPrefix === "") {
            setResult((prev)=>({
                    ...prev,
                    entries: [],
                    baseDirExists: false,
                    pathExists: false,
                    isLoading: false,
                    error: null
                }));
            return;
        }
        const generation = ++generationRef.current;
        // Serve from cache immediately (no loading state needed).
        const cacheKey = "".concat(pathPrefix, "::").concat(directoriesOnly, "::").concat(maxResults);
        const cached = getCached(cacheKey);
        if (cached) {
            setResult({
                entries: cached.entries,
                baseDir: cached.baseDir,
                baseDirExists: cached.baseDirExists,
                pathExists: cached.pathExists,
                isLoading: false,
                error: null
            });
            return;
        }
        setResult((prev)=>({
                ...prev,
                isLoading: true,
                error: null
            }));
        const abortController = new AbortController();
        const debounceTimer = setTimeout(async ()=>{
            try {
                const client = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_2__/* .getConnectTransport */ .qW)());
                const response = await client.listPathCompletions({
                    pathPrefix,
                    directoriesOnly,
                    maxResults
                }, {
                    signal: abortController.signal
                });
                if (generation !== generationRef.current) return;
                const newResult = {
                    entries: response.entries,
                    baseDir: response.baseDir,
                    baseDirExists: response.baseDirExists,
                    pathExists: response.pathExists
                };
                setCached(cacheKey, {
                    ...newResult,
                    expiresAt: Date.now() + CACHE_TTL_MS
                });
                setResult({
                    ...newResult,
                    isLoading: false,
                    error: null
                });
            } catch (err) {
                if (generation !== generationRef.current) return;
                if (abortController.signal.aborted) return;
                const message = err instanceof Error ? err.message : "Failed to fetch completions";
                setResult((prev)=>({
                        ...prev,
                        isLoading: false,
                        error: message
                    }));
            }
        }, DEBOUNCE_MS);
        return ()=>{
            clearTimeout(debounceTimer);
            abortController.abort();
        };
    }, [
        pathPrefix,
        enabled,
        directoriesOnly,
        maxResults
    ]);
    return result;
}


/***/ }),

/***/ 77882:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 42547));


/***/ }),

/***/ 81551:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  h: () => (/* binding */ PathCompletionDropdown)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fui%2FPathCompletionDropdown.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll85YXB0bmYwIHsKICBsaXN0LXN0eWxlOiBub25lOwogIG1hcmdpbjogMDsKICBwYWRkaW5nOiA0cHggMDsKICBtYXgtaGVpZ2h0OiAyMDBweDsKICBvdmVyZmxvdy15OiBhdXRvOwogIGJvcmRlci1ib3R0b206IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6Nyk7Cn0KLl85YXB0bmYxIHsKICBkaXNwbGF5OiBmbGV4OwogIGFsaWduLWl0ZW1zOiBmbGV4LXN0YXJ0OwogIGdhcDogOHB4OwogIHBhZGRpbmc6IDdweCAxNnB4OwogIGN1cnNvcjogcG9pbnRlcjsKICBmb250LXNpemU6IDEzcHg7CiAgZm9udC1mYW1pbHk6IHZhcigtLV8xOWV6MnJ6MnUpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejApOwogIHVzZXItc2VsZWN0OiBub25lOwp9Ci5fOWFwdG5mMTpob3ZlciB7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnoxNyk7Cn0KLl85YXB0bmYyIHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejE3KTsKfQouXzlhcHRuZjMgewogIGZvbnQtc2l6ZTogMTRweDsKICBmbGV4LXNocmluazogMDsKICB3aWR0aDogMThweDsKICB0ZXh0LWFsaWduOiBjZW50ZXI7Cn0KLl85YXB0bmY0IHsKICBmbGV4OiAxOwogIG92ZXJmbG93LXdyYXA6IGFueXdoZXJlOwp9Ci5fOWFwdG5mNSB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7CiAgZmxleC1zaHJpbms6IDA7Cn0KLl85YXB0bmY2IHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKfQouXzlhcHRuZjcgewogIGhlaWdodDogMXB4OwogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6ZSk7CiAgbWFyZ2luOiA0cHggOHB4OwogIHBhZGRpbmc6IDA7CiAgbGlzdC1zdHlsZTogbm9uZTsKfQouXzlhcHRuZjggewogIHBhZGRpbmc6IDEwcHggMTZweDsKICBmb250LXNpemU6IDEzcHg7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7CiAgYm9yZGVyLWJvdHRvbTogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6ZSk7Cn0%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FPathCompletionDropdown_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll85YXB0bmYwIHsKICBsaXN0LXN0eWxlOiBub25lOwogIG1hcmdpbjogMDsKICBwYWRkaW5nOiA0cHggMDsKICBtYXgtaGVpZ2h0OiAyMDBweDsKICBvdmVyZmxvdy15OiBhdXRvOwogIGJvcmRlci1ib3R0b206IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6Nyk7Cn0KLl85YXB0bmYxIHsKICBkaXNwbGF5OiBmbGV4OwogIGFsaWduLWl0ZW1zOiBmbGV4LXN0YXJ0OwogIGdhcDogOHB4OwogIHBhZGRpbmc6IDdweCAxNnB4OwogIGN1cnNvcjogcG9pbnRlcjsKICBmb250LXNpemU6IDEzcHg7CiAgZm9udC1mYW1pbHk6IHZhcigtLV8xOWV6MnJ6MnUpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejApOwogIHVzZXItc2VsZWN0OiBub25lOwp9Ci5fOWFwdG5mMTpob3ZlciB7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnoxNyk7Cn0KLl85YXB0bmYyIHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejE3KTsKfQouXzlhcHRuZjMgewogIGZvbnQtc2l6ZTogMTRweDsKICBmbGV4LXNocmluazogMDsKICB3aWR0aDogMThweDsKICB0ZXh0LWFsaWduOiBjZW50ZXI7Cn0KLl85YXB0bmY0IHsKICBmbGV4OiAxOwogIG92ZXJmbG93LXdyYXA6IGFueXdoZXJlOwp9Ci5fOWFwdG5mNSB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7CiAgZmxleC1zaHJpbms6IDA7Cn0KLl85YXB0bmY2IHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKfQouXzlhcHRuZjcgewogIGhlaWdodDogMXB4OwogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6ZSk7CiAgbWFyZ2luOiA0cHggOHB4OwogIHBhZGRpbmc6IDA7CiAgbGlzdC1zdHlsZTogbm9uZTsKfQouXzlhcHRuZjggewogIHBhZGRpbmc6IDEwcHggMTZweDsKICBmb250LXNpemU6IDEzcHg7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7CiAgYm9yZGVyLWJvdHRvbTogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6ZSk7Cn0_22_7D = __webpack_require__(61128);
;// ./src/components/ui/PathCompletionDropdown.css.ts


var divider = '_9aptnf7';
var dropdown = '_9aptnf0';
var icon = '_9aptnf3';
var item = '_9aptnf1';
var itemHistory = '_9aptnf6';
var itemSelected = '_9aptnf2';
var loading = '_9aptnf8';
var PathCompletionDropdown_css_name = '_9aptnf4';
var suffix = '_9aptnf5';
;// ./src/components/ui/PathCompletionDropdown.tsx
/* __next_internal_client_entry_do_not_use__ PathCompletionDropdown auto */ 

function EntryItem(param) {
    let { entry, index, selectedIndex, onSelect, id } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
        id: "".concat(id, "-option-").concat(index),
        className: [
            item,
            index === selectedIndex ? itemSelected : "",
            entry.isHistory ? itemHistory : ""
        ].filter(Boolean).join(" "),
        role: "option",
        "aria-selected": index === selectedIndex,
        title: entry.path,
        onMouseDown: (e)=>{
            e.preventDefault();
            onSelect(entry);
        },
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: icon,
                "aria-hidden": "true",
                children: entry.isHistory ? "🕒" : entry.isDirectory ? "📁" : "📄"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: PathCompletionDropdown_css_name,
                children: entry.name
            }),
            entry.isWorktree && entry.rootLabel && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                className: suffix,
                children: [
                    "worktree of ",
                    entry.rootLabel
                ]
            }),
            entry.isDirectory && !entry.isHistory && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: suffix,
                "aria-hidden": "true",
                children: "/"
            })
        ]
    });
}
function PathCompletionDropdown(param) {
    let { entries, selectedIndex, onSelect, isLoading, historyCount = 0, id = "path-completion-listbox" } = param;
    if (isLoading && entries.length === 0) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: loading,
            children: "Loading completions…"
        });
    }
    if (entries.length === 0) return null;
    const showDivider = historyCount > 0 && historyCount < entries.length;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("ul", {
        id: id,
        className: dropdown,
        role: "listbox",
        "aria-label": "Path completions",
        "data-testid": "path-completion-dropdown",
        children: [
            entries.slice(0, historyCount).map((entry, i)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(EntryItem, {
                    entry: entry,
                    index: i,
                    selectedIndex: selectedIndex,
                    onSelect: onSelect,
                    id: id
                }, entry.path)),
            showDivider && /*#__PURE__*/ (0,jsx_runtime.jsx)("li", {
                className: divider,
                role: "presentation",
                "aria-hidden": "true"
            }),
            entries.slice(historyCount).map((entry, i)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(EntryItem, {
                    entry: entry,
                    index: historyCount + i,
                    selectedIndex: selectedIndex,
                    onSelect: onSelect,
                    id: id
                }, entry.path))
        ]
    });
}


/***/ }),

/***/ 86082:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   c: () => (/* binding */ useHistoryFullTextSearch)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(66144);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__ = __webpack_require__(45640);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_7__ = __webpack_require__(16294);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(23575);
/* harmony import */ var _bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(28765);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(21694);
/* harmony import */ var _useDebounce__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(22479);
/* __next_internal_client_entry_do_not_use__ useHistoryFullTextSearch,default auto */ 





/**
 * Hook for full-text search across Claude conversation history.
 * Provides debounced search with pagination support.
 */ function useHistoryFullTextSearch() {
    let options = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : {};
    const { debounceMs = 300, autoSearch = true } = options;
    const [state, setState] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)({
        results: [],
        totalMatches: 0,
        queryTimeMs: 0,
        hasMore: false,
        loading: false,
        error: null
    });
    const [query, setQuery] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)("");
    const debouncedQuery = (0,_useDebounce__WEBPACK_IMPORTED_MODULE_3__/* .useDebounce */ .d)(query, debounceMs);
    const clientRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    const abortControllerRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    // Initialize ConnectRPC client
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        clientRef.current = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_4__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_2__/* .getConnectTransport */ .qW)());
    }, []);
    const toSearchMessageItems = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)((msgs)=>msgs.map((m)=>({
                role: m.role,
                content: m.content,
                timestamp: m.timestamp ? (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_5__/* .timestampDate */ .FP)(m.timestamp) : null,
                model: m.model
            })), []);
    // Convert protobuf SearchResult to our interface
    const convertResult = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)((result)=>{
        var _result_metadata, _result_metadata1, _result_metadata2, _result_metadata3;
        var _result_metadata_isMetadataMatch, _result_metadata_matchSource, _result_metadata_model, _result_moreMatchesInSessionCount;
        return {
            sessionId: result.sessionId,
            sessionName: result.sessionName,
            project: result.project,
            messageIndex: result.messageIndex,
            score: result.score,
            snippets: result.snippets.map((snippet)=>({
                    text: snippet.text,
                    highlightRanges: snippet.highlightRanges.map((hr)=>({
                            start: hr.start,
                            end: hr.end
                        })),
                    messageRole: snippet.messageRole,
                    messageTime: snippet.messageTime ? (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_5__/* .timestampDate */ .FP)(snippet.messageTime) : null
                })),
            metadata: {
                isMetadataMatch: (_result_metadata_isMetadataMatch = (_result_metadata = result.metadata) === null || _result_metadata === void 0 ? void 0 : _result_metadata.isMetadataMatch) !== null && _result_metadata_isMetadataMatch !== void 0 ? _result_metadata_isMetadataMatch : false,
                matchSource: (_result_metadata_matchSource = (_result_metadata1 = result.metadata) === null || _result_metadata1 === void 0 ? void 0 : _result_metadata1.matchSource) !== null && _result_metadata_matchSource !== void 0 ? _result_metadata_matchSource : "",
                model: (_result_metadata_model = (_result_metadata2 = result.metadata) === null || _result_metadata2 === void 0 ? void 0 : _result_metadata2.model) !== null && _result_metadata_model !== void 0 ? _result_metadata_model : "",
                createdAt: ((_result_metadata3 = result.metadata) === null || _result_metadata3 === void 0 ? void 0 : _result_metadata3.createdAt) ? (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_5__/* .timestampDate */ .FP)(result.metadata.createdAt) : null
            },
            moreMatchesInSessionCount: (_result_moreMatchesInSessionCount = result.moreMatchesInSessionCount) !== null && _result_moreMatchesInSessionCount !== void 0 ? _result_moreMatchesInSessionCount : 0,
            contextWindow: toSearchMessageItems(result.contextWindow),
            bookendFirst: toSearchMessageItems(result.bookendFirst),
            bookendLast: toSearchMessageItems(result.bookendLast)
        };
    }, [
        toSearchMessageItems
    ]);
    // Execute search
    const search = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (searchOptions)=>{
        if (!clientRef.current) return;
        if (!searchOptions.query.trim()) {
            setState((prev)=>({
                    ...prev,
                    results: [],
                    totalMatches: 0,
                    hasMore: false,
                    loading: false,
                    error: null
                }));
            return;
        }
        // Cancel any pending request
        if (abortControllerRef.current) {
            abortControllerRef.current.abort();
        }
        abortControllerRef.current = new AbortController();
        setState((prev)=>({
                ...prev,
                loading: true,
                error: null
            }));
        try {
            var _searchOptions_limit, _searchOptions_offset, _searchOptions_project, _searchOptions_model, _searchOptions_groupBySession, _searchOptions_includeContext, _searchOptions_excludeAutomationSessions;
            const response = await clientRef.current.searchClaudeHistory({
                query: searchOptions.query,
                limit: (_searchOptions_limit = searchOptions.limit) !== null && _searchOptions_limit !== void 0 ? _searchOptions_limit : 20,
                offset: (_searchOptions_offset = searchOptions.offset) !== null && _searchOptions_offset !== void 0 ? _searchOptions_offset : 0,
                project: (_searchOptions_project = searchOptions.project) !== null && _searchOptions_project !== void 0 ? _searchOptions_project : "",
                model: (_searchOptions_model = searchOptions.model) !== null && _searchOptions_model !== void 0 ? _searchOptions_model : "",
                startTime: searchOptions.startTime ? (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_5__/* .timestampFromDate */ .LI)(searchOptions.startTime) : undefined,
                endTime: searchOptions.endTime ? (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_5__/* .timestampFromDate */ .LI)(searchOptions.endTime) : undefined,
                groupBySession: (_searchOptions_groupBySession = searchOptions.groupBySession) !== null && _searchOptions_groupBySession !== void 0 ? _searchOptions_groupBySession : false,
                includeContext: (_searchOptions_includeContext = searchOptions.includeContext) !== null && _searchOptions_includeContext !== void 0 ? _searchOptions_includeContext : false,
                excludeAutomationSessions: (_searchOptions_excludeAutomationSessions = searchOptions.excludeAutomationSessions) !== null && _searchOptions_excludeAutomationSessions !== void 0 ? _searchOptions_excludeAutomationSessions : false
            }, {
                signal: abortControllerRef.current.signal
            });
            const newResults = response.results.map(convertResult);
            setState((prev)=>({
                    // Append results if append flag is set, otherwise replace
                    results: searchOptions.append ? [
                        ...prev.results,
                        ...newResults
                    ] : newResults,
                    totalMatches: response.totalMatches,
                    queryTimeMs: Number(response.queryTimeMs),
                    hasMore: response.hasMore,
                    loading: false,
                    error: null
                }));
        } catch (err) {
            // Comprehensive abort/cancel error detection
            // 1. Native AbortError
            if (err instanceof Error && err.name === "AbortError") {
                return;
            }
            // 2. DOMException AbortError
            if (err instanceof DOMException && err.name === "AbortError") {
                return;
            }
            // 3. ConnectRPC error codes for cancellation
            if (err instanceof _connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__/* .ConnectError */ .T && (err.code === _connectrpc_connect__WEBPACK_IMPORTED_MODULE_7__/* .Code */ .C.Canceled || err.code === _connectrpc_connect__WEBPACK_IMPORTED_MODULE_7__/* .Code */ .C.Aborted)) {
                return;
            }
            // 4. Check error message for abort/cancel indicators (fallback)
            if (err instanceof Error) {
                const msg = err.message.toLowerCase();
                if (msg.includes("abort") || msg.includes("cancel")) {
                    return;
                }
            }
            // Debug: log unexpected errors to help diagnose
            console.debug("[FullTextSearch] Search error:", err, "type:", typeof err, "name:", err === null || err === void 0 ? void 0 : err.name, "message:", err === null || err === void 0 ? void 0 : err.message);
            const error = err instanceof Error ? err : new Error("Search failed");
            setState((prev)=>({
                    ...prev,
                    loading: false,
                    error
                }));
        }
    }, [
        convertResult
    ]);
    // Load more results (pagination)
    const loadMore = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async ()=>{
        if (!state.hasMore || state.loading) return;
        await search({
            query: debouncedQuery,
            offset: state.results.length,
            append: true
        });
    }, [
        search,
        debouncedQuery,
        state.hasMore,
        state.loading,
        state.results.length
    ]);
    // Clear search results
    const clearSearch = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>{
        setQuery("");
        setState({
            results: [],
            totalMatches: 0,
            queryTimeMs: 0,
            hasMore: false,
            loading: false,
            error: null
        });
    }, []);
    // Auto-search when debounced query changes
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        if (autoSearch && debouncedQuery) {
            search({
                query: debouncedQuery
            });
        } else if (!debouncedQuery) {
            setState((prev)=>({
                    ...prev,
                    results: [],
                    totalMatches: 0,
                    hasMore: false
                }));
        }
    }, [
        autoSearch,
        debouncedQuery,
        search
    ]);
    // Cleanup on unmount
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        return ()=>{
            if (abortControllerRef.current) {
                abortControllerRef.current.abort();
            }
        };
    }, []);
    return {
        // State
        ...state,
        query,
        // Actions
        setQuery,
        search,
        loadMore,
        clearSearch
    };
}
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (useHistoryFullTextSearch)));


/***/ }),

/***/ 88253:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   P: () => (/* binding */ usePersistedViewState)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* __next_internal_client_entry_do_not_use__ usePersistedViewState auto */ 
function readRaw(key) {
    if (false) {}
    try {
        return window.localStorage.getItem(key);
    } catch (e) {
        return null;
    }
}
function writeRaw(key, value) {
    if (false) {}
    try {
        window.localStorage.setItem(key, value);
    } catch (e) {
    // Ignore storage errors (private browsing / quota exceeded) — state
    // still lives in memory, it just won't survive a reload.
    }
}
function removeRaw(key) {
    if (false) {}
    try {
        window.localStorage.removeItem(key);
    } catch (e) {
    // Ignore storage errors
    }
}
function loadField(config) {
    const raw = readRaw(config.key);
    if (raw === null) return config.defaultValue;
    let parsed;
    try {
        parsed = JSON.parse(raw);
    } catch (e) {
        return config.defaultValue;
    }
    const value = config.deserialize ? config.deserialize(parsed) : parsed;
    if (config.isValid && !config.isValid(value)) return config.defaultValue;
    return value;
}
function saveField(config, value) {
    const serialized = config.serialize ? config.serialize(value) : value;
    writeRaw(config.key, JSON.stringify(serialized));
}
function buildDefaults(fields) {
    const out = {};
    Object.keys(fields).forEach((name)=>{
        out[name] = fields[name].defaultValue;
    });
    return out;
}
/**
 * Generic hydration-safe localStorage persistence for a page/component's view
 * state (search, filters, sort, grouping, etc). State always starts at the
 * hardcoded defaults so SSR/static-export first paint can never mismatch a
 * client-only localStorage read; a mount-only effect then loads persisted
 * values and flips `isHydrated`.
 */ function usePersistedViewState(fields) {
    const fieldsRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(fields);
    fieldsRef.current = fields;
    const defaults = (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>buildDefaults(fields), [
        fields
    ]);
    const defaultsRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(defaults);
    defaultsRef.current = defaults;
    const [state, setState] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(defaults);
    const [isHydrated, setIsHydrated] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(false);
    // Skips the persistence effect's next run once, so resetToDefaults's
    // removeRaw calls aren't immediately undone by re-saving the new (default)
    // state back into localStorage.
    const skipPersistRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(false);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        const loaded = {};
        Object.keys(fieldsRef.current).forEach((name)=>{
            loaded[name] = loadField(fieldsRef.current[name]);
        });
        setState(loaded);
        setIsHydrated(true);
    // Mount-only: hydration must run exactly once regardless of prop identity.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        if (!isHydrated) return;
        if (skipPersistRef.current) {
            skipPersistRef.current = false;
            return;
        }
        Object.keys(fieldsRef.current).forEach((name)=>{
            saveField(fieldsRef.current[name], state[name]);
        });
    }, [
        state,
        isHydrated
    ]);
    const setters = (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>{
        const out = {};
        Object.keys(fields).forEach((name)=>{
            out[name] = (value)=>{
                setState((prev)=>({
                        ...prev,
                        [name]: typeof value === "function" ? value(prev[name]) : value
                    }));
            };
        });
        return out;
    }, [
        fields
    ]);
    const clearStorage = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>{
        Object.keys(fieldsRef.current).forEach((name)=>{
            removeRaw(fieldsRef.current[name].key);
        });
    }, []);
    const resetToDefaults = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>{
        skipPersistRef.current = true;
        clearStorage();
        setState(defaultsRef.current);
    }, [
        clearStorage
    ]);
    return {
        state,
        setters,
        isHydrated,
        resetToDefaults,
        clearStorage
    };
}


/***/ }),

/***/ 90062:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   ZX: () => (/* binding */ createSessionWatchTransport)
/* harmony export */ });
/* unused harmony exports fromWebSocket, createWatchTransport */
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(45640);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(16294);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__ = __webpack_require__(13399);
/* harmony import */ var _connectrpc_connect_web__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(46564);
/* harmony import */ var _connectrpc_connect_protocol__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(70050);
/* harmony import */ var _connectrpc_connect_protocol__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(69522);
/* harmony import */ var _connectrpc_connect_protocol__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(9103);
/* harmony import */ var _connectrpc_connect_protocol__WEBPACK_IMPORTED_MODULE_7__ = __webpack_require__(48210);
/* harmony import */ var _connectrpc_connect_protocol_connect__WEBPACK_IMPORTED_MODULE_8__ = __webpack_require__(60713);
/* provided dependency */ var process = __webpack_require__(36843);
/* __next_internal_client_entry_do_not_use__ fromWebSocket,createWatchTransport,createSessionWatchTransport auto */ 



function encodeEnvelope(flags, data) {
    const buf = new Uint8Array(5 + data.length);
    buf[0] = flags;
    new DataView(buf.buffer).setUint32(1, data.length, false);
    buf.set(data, 5);
    return buf;
}
async function* fromWebSocket(ws, signal) {
    const queue = [];
    let notify = null;
    const push = (item)=>{
        queue.push(item);
        notify === null || notify === void 0 ? void 0 : notify();
        notify = null;
    };
    ws.onmessage = (e)=>push(new Uint8Array(e.data));
    ws.onerror = ()=>push(new _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T("WebSocket error", _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__/* .Code */ .C.Unavailable));
    ws.onclose = (ev)=>{
        if ((signal === null || signal === void 0 ? void 0 : signal.aborted) || ev.code === 1000) {
            push(null); // intentional close (1000=normal) or signal abort
        } else {
            // Code 1001 (Going Away / server restart) sets wasClean=true via TCP FIN but IS retriable.
            // Only 1000 and explicit signal abort are treated as non-retriable clean closes.
            push(new _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T("WebSocket closed", _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__/* .Code */ .C.Unavailable, new Headers({
                "ws-close-code": String(ev.code)
            })));
        }
    };
    const abortHandler = ()=>{
        ws.close();
        push(null);
    };
    signal === null || signal === void 0 ? void 0 : signal.addEventListener("abort", abortHandler);
    try {
        while(true){
            while(queue.length === 0){
                await new Promise((r)=>{
                    notify = r;
                });
            }
            const item = queue.shift();
            if (item === null) return;
            if (item instanceof Error) throw item;
            yield item;
        }
    } finally{
        signal === null || signal === void 0 ? void 0 : signal.removeEventListener("abort", abortHandler);
    }
}
/**
 * Creates a ConnectRPC transport that uses standard HTTP fetch for unary calls
 * and WebSocket for server-streaming calls. Compatible with StreamingWSBridge on
 * the server (server/services/ws_stream_bridge.go).
 *
 * Protocol (streaming):
 *   Client → first binary WS message: Connect request envelope (5-byte header + proto)
 *   Server → N binary WS messages: Connect response envelopes
 *   Server → final binary WS message: Connect end-stream envelope
 *
 * Auth: cookie-based (browser sends cookies automatically on WS upgrade).
 */ function createWatchTransport(opt) {
    const httpTransport = (0,_connectrpc_connect_web__WEBPACK_IMPORTED_MODULE_2__/* .createConnectTransport */ .w)(opt);
    return {
        unary: httpTransport.unary.bind(httpTransport),
        async stream (method, signal, timeoutMs, header, input, contextValues) {
            var _opt_useBinaryFormat;
            const useBinaryFormat = (_opt_useBinaryFormat = opt.useBinaryFormat) !== null && _opt_useBinaryFormat !== void 0 ? _opt_useBinaryFormat : true;
            const { serialize, parse } = (0,_connectrpc_connect_protocol__WEBPACK_IMPORTED_MODULE_3__/* .createClientMethodSerializers */ .Ef)(method, useBinaryFormat, opt.jsonOptions, opt.binaryOptions);
            const methodUrl = (0,_connectrpc_connect_protocol__WEBPACK_IMPORTED_MODULE_4__/* .createMethodUrl */ .j)(opt.baseUrl, method);
            return (0,_connectrpc_connect_protocol__WEBPACK_IMPORTED_MODULE_5__/* .runStreamingCall */ .u)({
                interceptors: opt.interceptors,
                timeoutMs,
                signal,
                req: {
                    stream: true,
                    service: method.parent,
                    method,
                    url: methodUrl,
                    requestMethod: "POST",
                    header: new Headers(header instanceof Headers ? header : undefined),
                    contextValues: contextValues !== null && contextValues !== void 0 ? contextValues : (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__/* .createContextValues */ .k)(),
                    message: input
                },
                next: async (req)=>{
                    const wsUrl = req.url.replace(/^http/, "ws");
                    const ws = new WebSocket(wsUrl);
                    ws.binaryType = "arraybuffer";
                    await new Promise((resolve, reject)=>{
                        ws.onopen = ()=>resolve();
                        ws.onerror = ()=>reject(new _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T("WebSocket connection failed", _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__/* .Code */ .C.Unavailable));
                    });
                    // Read the first (and only) request message and send it as a Connect envelope.
                    const iter = req.message[Symbol.asyncIterator]();
                    const first = await iter.next();
                    if (first.done) {
                        ws.close();
                        throw new _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T("missing request message", _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__/* .Code */ .C.Internal);
                    }
                    ws.send(encodeEnvelope(0, serialize(first.value)));
                    const trailer = new Headers();
                    async function* parseResponses() {
                        let buf = new Uint8Array(0);
                        let endStreamReceived = false;
                        for await (const chunk of fromWebSocket(ws, signal)){
                            // Accumulate chunk into buffer
                            const merged = new Uint8Array(buf.length + chunk.length);
                            merged.set(buf);
                            merged.set(chunk, buf.length);
                            buf = merged;
                            // Parse all complete envelopes
                            while(buf.length >= 5){
                                const flags = buf[0];
                                const msgLen = new DataView(buf.buffer, buf.byteOffset).getUint32(1, false);
                                if (buf.length < 5 + msgLen) break;
                                const data = buf.slice(5, 5 + msgLen);
                                buf = buf.slice(5 + msgLen);
                                if (flags & _connectrpc_connect_protocol__WEBPACK_IMPORTED_MODULE_7__/* .compressedFlag */ .y) {
                                    throw new _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T("unexpected compressed response", _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__/* .Code */ .C.Internal);
                                }
                                if (flags & _connectrpc_connect_protocol_connect__WEBPACK_IMPORTED_MODULE_8__/* .endStreamFlag */ .Cu) {
                                    endStreamReceived = true;
                                    const end = (0,_connectrpc_connect_protocol_connect__WEBPACK_IMPORTED_MODULE_8__/* .endStreamFromJson */ .mo)(data);
                                    if (end.error) throw end.error;
                                    end.metadata.forEach((v, k)=>trailer.set(k, v));
                                } else {
                                    yield parse(data);
                                }
                            }
                        }
                        if (!endStreamReceived) {
                            throw new _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T("stream ended without end-stream message", _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__/* .Code */ .C.Internal);
                        }
                    }
                    return {
                        ...req,
                        header: new Headers({
                            "Content-Type": "application/connect+proto"
                        }),
                        trailer,
                        message: parseResponses()
                    };
                }
            });
        }
    };
}
/**
 * Selects the transport for any server-streaming Watch* RPC (SessionService,
 * ReviewQueueService, BacklogService, InsightsService,
 * UnfinishedWorkService, ...) whose path is registered with server.go's
 * StreamingWSBridge.
 *
 * Native ConnectRPC streaming only helps over a real HTTP/2 connection —
 * that's the TLS remote-access listener (:8444) only. No shipping browser
 * supports cleartext HTTP/2, so on the plain :8543 dev listener native
 * streaming would silently stay on HTTP/1.1 and hold a long-lived fetch()
 * open, reintroducing the exact browser connection-slot problem
 * StreamingWSBridge exists to avoid. Gate on baseUrl, not just the flag.
 */ function createSessionWatchTransport(opt) {
    const nativeStreamingEnabled = process.env.NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING === "true";
    if (nativeStreamingEnabled && opt.baseUrl.startsWith("https://")) {
        return (0,_connectrpc_connect_web__WEBPACK_IMPORTED_MODULE_2__/* .createConnectTransport */ .w)(opt);
    }
    return createWatchTransport(opt);
}


/***/ }),

/***/ 90814:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   O: () => (/* binding */ usePageView)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _lib_nav__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(41078);
/* harmony import */ var _lib_contexts_AnalyticsContext__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(24044);
/* __next_internal_client_entry_do_not_use__ usePageView auto */ 


function usePageView() {
    const pathname = (0,_lib_nav__WEBPACK_IMPORTED_MODULE_1__/* .usePathname */ .a8)();
    const { track } = (0,_lib_contexts_AnalyticsContext__WEBPACK_IMPORTED_MODULE_2__/* .useAnalytics */ .st)();
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        track({
            name: "page_view",
            category: "navigation",
            page: pathname
        });
    }, [
        pathname,
        track
    ]);
}


/***/ }),

/***/ 93321:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  E: () => (/* binding */ ActionBar)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fui%2FActionBar.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA8WSy26DMBBF93zFLJOqRiTN05Gq_kk1wIS4gTGynUBa5d8rIKDQvKpuusFifOb6-nr891GZh8ifAXx5ALGyeYoHCeuUypUH9SoKg7mE6luVMFUJC-UosxIiYkemKicVs0czEAIjpzSLEI1IMH-GwJ8ayoYr7-j57Xmj-ryfrITAH1dwjx3fZC_Ql5vo_IKd1OzHzjq1PohIsyN2zdWFdWhcj57eoYnjHju7ztocIxIhuYKIew3z6w1tukfvLaNYIdjIEDEgxzDIsBSFit1GwigYT_JyWIt0mov69_QwXawAx4dy89niptrZQLBuRwJA78msU12IUgLunG6qoqBwq5zodm1kdJoqTiQ4vYs2DdZUq7c6GWDN1Fg9dyBlq9c1nEx1Y3u9EV7h6dy-3RjFWwnB7-KYTYKLOJb_Hsfyr3EsH8XxDcl0doYUBAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FActionBar_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA8WSy26DMBBF93zFLJOqRiTN05Gq_kk1wIS4gTGynUBa5d8rIKDQvKpuusFifOb6_nr891GZh8ifAXx5ALGyeYoHCeuUypUH9SoKg7mE6luVMFUJC_UosxIiYkemKicVs0czEAIjpzSLEI1IMH_GwJ8ayoYr7_j57Xmj_ryfrITAH1dwjx3fZC_Ql5vo_IKd1OzHzjq1PohIsyN2zdWFdWhcj57eoYnjHju7ztocIxIhuYKIew3z6w1tukfvLaNYIdjIEDEgxzDIsBSFit1GwigYT_JyWIt0mov69_QwXawAx4dy89niptrZQLBuRwJA78msU12IUgLunG6qoqBwq5zodm1kdJoqTiQ4vYs2DdZUq7c6GWDN1Fg9dyBlq9c1nEx1Y3u9EV7h6dy_3RjFWwnB7_KYTYKLOJb_Hsfyr3EsH8XxDcl0doYUBAAA_22_7D = __webpack_require__(58842);
;// ./src/components/ui/ActionBar.css.ts

var actionBar = '_1xpbanz0';
var ActionBar_css_compact = '_1xpbanz8';
var gapLg = '_1xpbanz3';
var gapMd = '_1xpbanz2';
var gapSm = '_1xpbanz1';
var justifyBetween = '_1xpbanz6';
var justifyCenter = '_1xpbanz7';
var justifyEnd = '_1xpbanz5';
var justifyStart = '_1xpbanz4';
var ActionBar_css_scroll = '_1xpbanz9';
;// ./src/components/ui/ActionBar.tsx
/* __next_internal_client_entry_do_not_use__ ActionBar auto */ 

const gapClass = {
    sm: gapSm,
    md: gapMd,
    lg: gapLg
};
const justifyClass = {
    start: justifyStart,
    end: justifyEnd,
    between: justifyBetween,
    center: justifyCenter
};
function ActionBar(param) {
    let { children, gap = "md", justify = "start", scroll, compact, className, id } = param;
    const classes = [
        actionBar,
        gapClass[gap],
        justifyClass[justify],
        scroll ? ActionBar_css_scroll : undefined,
        compact ? ActionBar_css_compact : undefined,
        className
    ].filter(Boolean).join(" ");
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        id: id,
        className: classes,
        children: children
    });
}


/***/ }),

/***/ 99863:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  Link: () => (/* binding */ Link),
  Redirect: () => (/* binding */ Redirect),
  pushHistoryMarker: () => (/* binding */ pushHistoryMarker),
  pushTrayEntry: () => (/* binding */ pushTrayEntry),
  replaceHash: () => (/* binding */ replaceHash),
  usePathname: () => (/* binding */ usePathname),
  useReplaceSearch: () => (/* binding */ useReplaceSearch),
  useRouter: () => (/* binding */ useRouter),
  useSearchParams: () => (/* binding */ useSearchParams)
});

// UNUSED EXPORTS: nextImpl

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/client/app-dir/link.js
var app_dir_link = __webpack_require__(6024);
var link_default = /*#__PURE__*/__webpack_require__.n(app_dir_link);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/api/navigation.js
var navigation = __webpack_require__(45853);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
;// ./src/lib/nav/history.ts
// The only place outside an impl that touches raw History; every impl shares it so foreign state survives the same way.
const TRAY_MARKER = "ssqTray";
function replaceSearchInHistory(search) {
    const query = search === "" || search.startsWith("?") ? search : "?".concat(search);
    window.history.replaceState(window.history.state, "", query === "" ? window.location.pathname : query);
}
function replaceHashInHistory(hash) {
    const url = hash ? "".concat(window.location.pathname).concat(window.location.search).concat(hash.startsWith("#") ? hash : "#".concat(hash)) : window.location.pathname + window.location.search;
    window.history.replaceState(window.history.state, "", url);
}
function pushHistoryMarkerInHistory(key, value) {
    let replace = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : false;
    var _window_history_state;
    const state = {
        ...(_window_history_state = window.history.state) !== null && _window_history_state !== void 0 ? _window_history_state : {},
        [key]: value
    };
    if (replace) window.history.replaceState(state, "");
    else window.history.pushState(state, "");
}
function pushTrayEntryInHistory() {
    let replace = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : false;
    pushHistoryMarkerInHistory(TRAY_MARKER, true, replace);
}

;// ./src/lib/nav/normalize.ts
/** Strips trailing slashes (Next's `trailingSlash` export reports `/x/`) but keeps the root `/`. */ function normalizePathname(pathname) {
    const trimmed = pathname.replace(/\/+$/, "");
    return trimmed === "" ? "/" : trimmed;
}

;// ./src/lib/nav/impl.next.tsx
/* __next_internal_client_entry_do_not_use__ useRouter,useSearchParams,usePathname,Link,Redirect,useReplaceSearch,replaceHash,pushTrayEntry,pushHistoryMarker,nextImpl auto */ 





const toNextOptions = (options)=>(options === null || options === void 0 ? void 0 : options.resetScroll) === false ? {
        scroll: false
    } : undefined;
function useRouter() {
    const router = (0,navigation.useRouter)();
    return (0,react.useMemo)(()=>({
            push: (href, options)=>router.push(href, toNextOptions(options)),
            replace: (href, options)=>router.replace(href, toNextOptions(options)),
            back: ()=>router.back(),
            forward: ()=>router.forward()
        }), [
        router
    ]);
}
function useSearchParams() {
    return (0,navigation.useSearchParams)();
}
function usePathname() {
    var _useNextPathname;
    return normalizePathname((_useNextPathname = (0,navigation.usePathname)()) !== null && _useNextPathname !== void 0 ? _useNextPathname : "/");
}
/** Prefetch stays off: it triggers CSS preload warnings (the reason the NavLink this replaces existed). */ const Link = /*#__PURE__*/ (0,react.forwardRef)(function Link(param, ref) {
    let { resetScroll, ...props } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)((link_default()), {
        ref: ref,
        prefetch: false,
        scroll: resetScroll === false ? false : undefined,
        ...props
    });
});
/** Next's render-time `redirect()`: the behaviour the two redirect-only pages have today. */ function Redirect(param) {
    let { to } = param;
    (0,navigation.redirect)(to);
}
function useReplaceSearch() {
    return (0,react.useCallback)((search)=>replaceSearchInHistory(search), []);
}
const replaceHash = replaceHashInHistory;
const pushTrayEntry = pushTrayEntryInHistory;
const pushHistoryMarker = pushHistoryMarkerInHistory;
const nextImpl = {
    useRouter,
    useSearchParams,
    usePathname,
    Link,
    Redirect,
    useReplaceSearch,
    replaceHash,
    pushTrayEntry,
    pushHistoryMarker
};


/***/ })

},
/******/ __webpack_require__ => { // webpackRuntimeModules
/******/ var __webpack_exec__ = (moduleId) => (__webpack_require__(__webpack_require__.s = moduleId))
/******/ __webpack_require__.O(0, [1869,852,4202,9353,8182,7924,5522,8165,3575,2513,9691,3516,7358], () => (__webpack_exec__(77882)));
/******/ var __webpack_exports__ = __webpack_require__.O();
/******/ _N_E = __webpack_exports__;
/******/ }
]);
//# sourceMappingURL=page-5d8d6ed1ecc7fdb4.js.map