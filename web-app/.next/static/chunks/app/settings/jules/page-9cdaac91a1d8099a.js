(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[2262],{

/***/ 3728:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   PageViewTracker: () => (/* binding */ PageViewTracker)
/* harmony export */ });
/* harmony import */ var _lib_analytics_usePageView__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(90814);
/* __next_internal_client_entry_do_not_use__ PageViewTracker auto */ 
function PageViewTracker() {
    (0,_lib_analytics_usePageView__WEBPACK_IMPORTED_MODULE_0__/* .usePageView */ .O)();
    return null;
}


/***/ }),

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

/***/ 12646:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 14217:
/***/ (() => {

// extracted by mini-css-extract-plugin

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

/***/ 33545:
/***/ (() => {

// extracted by mini-css-extract-plugin

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

/***/ 61128:
/***/ (() => {

// extracted by mini-css-extract-plugin

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

/***/ 86024:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  JulesSettings: () => (/* binding */ JulesSettings)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/gen/session/v1/session_pb.ts
var session_pb = __webpack_require__(23575);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect-web@2.1.1+0149f91553ca585e/node_modules/@connectrpc/connect-web/dist/esm/connect-transport.js + 8 modules
var connect_transport = __webpack_require__(46564);
// EXTERNAL MODULE: ./src/lib/config.ts
var config = __webpack_require__(51202);
// EXTERNAL MODULE: ./src/components/ui/RepoPathInput.tsx + 2 modules
var RepoPathInput = __webpack_require__(34609);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2FsettingsPanelForm.css.ts.vanilla.css%22%2C%22source%22%3A%22LmFvZ2NhbTAgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IDFyZW07CiAgbWF4LXdpZHRoOiA2NDBweDsKfQouYW9nY2FtMSB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7CiAgZm9udC1zaXplOiAxLjI1cmVtOwogIGZvbnQtd2VpZ2h0OiA2MDA7CiAgbWFyZ2luOiAwOwp9Ci5hb2djYW0yIHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKICBmb250LXNpemU6IDAuODc1cmVtOwogIG1hcmdpbjogMDsKfQouYW9nY2FtMyB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7Cn0KLmFvZ2NhbTQgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IDEuMjVyZW07Cn0KLmFvZ2NhbTUgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IDAuNXJlbTsKfQouYW9nY2FtNiB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7CiAgZm9udC1zaXplOiAwLjg3NXJlbTsKICBmb250LXdlaWdodDogNjAwOwp9Ci5hb2djYW03IHsKICBkaXNwbGF5OiBmbGV4OwogIGdhcDogMC41cmVtOwogIGFsaWduLWl0ZW1zOiBjZW50ZXI7Cn0KLmFvZ2NhbTggewogIHBhZGRpbmc6IDAuNXJlbSAwLjc1cmVtOwogIGJhY2tncm91bmQtY29sb3I6IHZhcigtLV8xOWV6MnJ6MWEpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6ayk7CiAgYm9yZGVyLXJhZGl1czogNHB4OwogIGNvbG9yOiB2YXIoLS1fMTllejJyejFiKTsKICBmb250LXNpemU6IDAuODc1cmVtOwogIGZsZXg6IDE7Cn0KLmFvZ2NhbTg6Zm9jdXMgewogIG91dGxpbmU6IG5vbmU7CiAgYm9yZGVyLWNvbG9yOiB2YXIoLS1fMTllejJyemwpOwp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2FsettingsPanelForm_css_ts_vanilla_css_22_2C_22source_22_3A_22LmFvZ2NhbTAgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IDFyZW07CiAgbWF4LXdpZHRoOiA2NDBweDsKfQouYW9nY2FtMSB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7CiAgZm9udC1zaXplOiAxLjI1cmVtOwogIGZvbnQtd2VpZ2h0OiA2MDA7CiAgbWFyZ2luOiAwOwp9Ci5hb2djYW0yIHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKICBmb250LXNpemU6IDAuODc1cmVtOwogIG1hcmdpbjogMDsKfQouYW9nY2FtMyB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7Cn0KLmFvZ2NhbTQgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IDEuMjVyZW07Cn0KLmFvZ2NhbTUgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IDAuNXJlbTsKfQouYW9nY2FtNiB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7CiAgZm9udC1zaXplOiAwLjg3NXJlbTsKICBmb250LXdlaWdodDogNjAwOwp9Ci5hb2djYW03IHsKICBkaXNwbGF5OiBmbGV4OwogIGdhcDogMC41cmVtOwogIGFsaWduLWl0ZW1zOiBjZW50ZXI7Cn0KLmFvZ2NhbTggewogIHBhZGRpbmc6IDAuNXJlbSAwLjc1cmVtOwogIGJhY2tncm91bmQtY29sb3I6IHZhcigtLV8xOWV6MnJ6MWEpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6ayk7CiAgYm9yZGVyLXJhZGl1czogNHB4OwogIGNvbG9yOiB2YXIoLS1fMTllejJyejFiKTsKICBmb250LXNpemU6IDAuODc1cmVtOwogIGZsZXg6IDE7Cn0KLmFvZ2NhbTg6Zm9jdXMgewogIG91dGxpbmU6IG5vbmU7CiAgYm9yZGVyLWNvbG9yOiB2YXIoLS1fMTllejJyemwpOwp9_22_7D = __webpack_require__(33545);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsettings%2FJulesSettings.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA62U33biIBDG732Kudy9wJNobZQ-TA-GMc4WgQIxxj199x6ibbJu_tieXgIzH9_8ZmD-vH5d-tdjAn9nALlRxnE4CveLsed0g-eFOy9-P80AdkYH5umMHJL5Ol2sHB7i_kG4gjSH5Gn2Nptf1dJhtXOjthX5S-FMqSXrjaovUcZJdBxSewJvFMmbqFMnijkhqfQcHuwp7lohJekiul1m0Swk88dP11-qZtFUI8lbJWoOO4XNDUJRoRkFPPjLJvNBuBCPCmGj8EW1FVoOY0l6IGe3-Q9T-VNY0-wuri_3cb1SzcahtvZX_RwbdJIc5oGM5rG-8qA7GJdZT3u6TuJKkQ_Mh1ohB200di9-vKOBOeqALm7_KX2gXc1yowPqwMFbkSPbYqgQ9X_tHUUyRRoHSbfus29NDYA5otspU3HYk5QX5wFPgbUHqBRZTz4eVXsKyJpaI8HKCdt1sW5c9L2qttip6Uvvmr40GRm_fuHxLyovnY9J1tBHj6eK5fsIqSnZWJFTqKPm5t8YSV5sFcrbsFX3Um0CE0qZCmU3ezPc1Orr_6340d97-6Nqef_ju76hz-EZeIzXgWPB9P6o8nsYWwEcFhidqrd3orAnkTkHAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsettings_2FJulesSettings_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA62U33biIBDG732Kudy9wJNobZQ_TA_GMc4WgQIxxj199x6ibbJu_tieXgIzH9_8ZmD_vH5d_tdjAn9nALlRxnE4CveLsed0g_eFOy9_P80AdkYH5umMHJL5Ol2sHB7i_kG4gjSH5Gn2Nptf1dJhtXOjthX5S_FMqSXrjaovUcZJdBxSewJvFMmbqFMnijkhqfQcHuwp7lohJekiul1m0Swk88dP11_qZtFUI8lbJWoOO4XNDUJRoRkFPPjLJvNBuBCPCmGj8EW1FVoOY0l6IGe3_Q9T_VNY0_wuri_3cb1SzcahtvZX_RwbdJIc5oGM5rG_8qA7GJdZT3u6TuJKkQ_Mh1ohB200di9_vKOBOeqALm7_KX2gXc1yowPqwMFbkSPbYqgQ9X_tHUUyRRoHSbfus29NDYA5otspU3HYk5QX5wFPgbUHqBRZTz4eVXsKyJpaI8HKCdt1sW5c9L2qttip6Uvvmr40GRm_fuHxLyovnY9J1tBHj6eK5fsIqSnZWJFTqKPm5t8YSV5sFcrbsFX3Um0CE0qZCmU3ezPc1Orr_6340d97_6Nqef_ju76hz_EZeIzXgWPB9P6o8nsYWwEcFhidqrd3orAnkTkHAAA_22_7D = __webpack_require__(14217);
;// ./src/components/settings/JulesSettings.css.ts



var actions = '_8q3sqvc';
var container = 'aogcam0';
var description = 'aogcam2';
var emptyRepoNote = '_8q3sqva';
var field = 'aogcam5';
var JulesSettings_css_form = 'aogcam4';
var heading = 'aogcam1';
var hint = '_8q3sqv0';
var input = 'aogcam8';
var inputRow = 'aogcam7';
var label = 'aogcam6';
var loadingText = 'aogcam3';
var repoList = '_8q3sqv5';
var repoName = '_8q3sqv7';
var repoRow = '_8q3sqv6';
var revokeBtn = '_8q3sqv8';
var revokedNote = '_8q3sqv9';
var JulesSettings_css_saveError = '_8q3sqve';
var saveStatus = '_8q3sqvd';
var testResultStatus = '_8q3sqv4';
var toggleLabel = '_8q3sqv3';
var toggleRow = '_8q3sqv2';
var usageNote = '_8q3sqvb';
var warningText = '_8q3sqv1';
;// ./src/components/settings/JulesSettings.tsx
/* __next_internal_client_entry_do_not_use__ JulesSettings auto */ 
// +feature: jules-settings
// JulesSettings — Settings -> Jules panel (google-jules-integration Epic
// 3.1, Story 3.1.1). Modeled on SlackNotificationSettings.tsx's structure
// (load -> form -> save, plus a synchronous "test" action). See
// project_plans/google-jules-integration/design/ux.md §2 for the full
// wireframe/interaction spec this implements.







// Caps clamped client-side to the same range the server enforces (ux.md
// §2.2 step 6) so the user sees the value they'll actually get before
// saving, rather than submitting one the server silently rewrites.
const MIN_CONCURRENT_SESSIONS = 2;
const MAX_CONCURRENT_SESSIONS = 10;
const MIN_SESSIONS_PER_DAY = 15;
const MAX_SESSIONS_PER_DAY = 300;
function clamp(value, min, max) {
    if (Number.isNaN(value)) return min;
    return Math.min(max, Math.max(min, value));
}
// shortRepoLabel derives a display-friendly "owner/repo" from the full
// filesystem path stored in EgressAcknowledgedRepos (e.g.
// "/home/tstapler/code/github.com/tstapler/stapler-squad" ->
// "tstapler/stapler-squad"), matching this monorepo's own
// ~/code/<host>/<owner>/<repo> clone convention (see repo CLAUDE.md). This
// is a display-only heuristic — the server resolves the authoritative
// owner/repo from the git remote itself (TestJulesConnection).
function shortRepoLabel(repoPath) {
    const segments = repoPath.split("/").filter(Boolean);
    if (segments.length < 2) return repoPath;
    return segments.slice(-2).join("/");
}
function JulesSettings() {
    const [loading, setLoading] = (0,react.useState)(true);
    const [loadError, setLoadError] = (0,react.useState)(null);
    const [enabled, setEnabled] = (0,react.useState)(false);
    const [hasApiKey, setHasApiKey] = (0,react.useState)(false);
    const [apiKeyInput, setApiKeyInput] = (0,react.useState)("");
    const [maxConcurrentJulesSessions, setMaxConcurrentJulesSessions] = (0,react.useState)(MIN_CONCURRENT_SESSIONS);
    const [maxJulesSessionsPerDay, setMaxJulesSessionsPerDay] = (0,react.useState)(MIN_SESSIONS_PER_DAY);
    const [egressAcknowledgedRepos, setEgressAcknowledgedRepos] = (0,react.useState)([]);
    const [testRepoPath, setTestRepoPath] = (0,react.useState)("");
    const [testSubmitting, setTestSubmitting] = (0,react.useState)(false);
    const [testResult, setTestResult] = (0,react.useState)(null);
    const [saving, setSaving] = (0,react.useState)(false);
    const [saveError, setSaveError] = (0,react.useState)(null);
    const [saveStatusMessage, setSaveStatusMessage] = (0,react.useState)(null);
    const [revokingRepo, setRevokingRepo] = (0,react.useState)(null);
    const [revokedNoteRepo, setRevokedNoteRepo] = (0,react.useState)(null);
    const [usage, setUsage] = (0,react.useState)({
        dispatched: 0,
        completed: 0,
        failed: 0
    });
    const clientRef = (0,react.useRef)(null);
    const loadConfig = (0,react.useCallback)(async ()=>{
        if (!clientRef.current) return;
        try {
            setLoading(true);
            setLoadError(null);
            const response = await clientRef.current.getJulesConfig({});
            const cfg = response.config;
            if (cfg) {
                var _cfg_usage, _cfg_usage1, _cfg_usage2;
                setEnabled(cfg.enabled);
                setHasApiKey(cfg.hasApiKey);
                setEgressAcknowledgedRepos(cfg.egressAcknowledgedRepos);
                setMaxConcurrentJulesSessions(cfg.maxConcurrentJulesSessions || MIN_CONCURRENT_SESSIONS);
                setMaxJulesSessionsPerDay(cfg.maxJulesSessionsPerDay || MIN_SESSIONS_PER_DAY);
                var _cfg_usage_sessionDispatched, _cfg_usage_sessionCompleted, _cfg_usage_sessionFailed;
                setUsage({
                    dispatched: Number((_cfg_usage_sessionDispatched = (_cfg_usage = cfg.usage) === null || _cfg_usage === void 0 ? void 0 : _cfg_usage.sessionDispatched) !== null && _cfg_usage_sessionDispatched !== void 0 ? _cfg_usage_sessionDispatched : 0),
                    completed: Number((_cfg_usage_sessionCompleted = (_cfg_usage1 = cfg.usage) === null || _cfg_usage1 === void 0 ? void 0 : _cfg_usage1.sessionCompleted) !== null && _cfg_usage_sessionCompleted !== void 0 ? _cfg_usage_sessionCompleted : 0),
                    failed: Number((_cfg_usage_sessionFailed = (_cfg_usage2 = cfg.usage) === null || _cfg_usage2 === void 0 ? void 0 : _cfg_usage2.sessionFailed) !== null && _cfg_usage_sessionFailed !== void 0 ? _cfg_usage_sessionFailed : 0)
                });
            }
        } catch (e) {
            setLoadError("Couldn't load Jules settings.");
        } finally{
            setLoading(false);
        }
    }, []);
    (0,react.useEffect)(()=>{
        const transport = (0,connect_transport/* createConnectTransport */.w)({
            baseUrl: (0,config/* getApiBaseUrl */.h)()
        });
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, transport);
        loadConfig();
    }, [
        loadConfig
    ]);
    function handleConcurrentBlur() {
        setMaxConcurrentJulesSessions((v)=>clamp(v, MIN_CONCURRENT_SESSIONS, MAX_CONCURRENT_SESSIONS));
    }
    function handlePerDayBlur() {
        setMaxJulesSessionsPerDay((v)=>clamp(v, MIN_SESSIONS_PER_DAY, MAX_SESSIONS_PER_DAY));
    }
    async function handleSave() {
        if (!clientRef.current) return;
        setSaving(true);
        setSaveError(null);
        setSaveStatusMessage(null);
        try {
            await clientRef.current.updateJulesConfig({
                apiKey: apiKeyInput.trim(),
                enabled,
                maxConcurrentJulesSessions: clamp(maxConcurrentJulesSessions, MIN_CONCURRENT_SESSIONS, MAX_CONCURRENT_SESSIONS),
                maxJulesSessionsPerDay: clamp(maxJulesSessionsPerDay, MIN_SESSIONS_PER_DAY, MAX_SESSIONS_PER_DAY)
            });
            setApiKeyInput("");
            setSaveStatusMessage(apiKeyInput.trim() ? "Key saved." : "Saved.");
            await loadConfig();
        } catch (err) {
            setSaveError(err instanceof Error ? err.message : String(err));
        } finally{
            setSaving(false);
        }
    }
    async function handleTestConnection() {
        if (!clientRef.current) return;
        const repoPath = testRepoPath.trim();
        if (!repoPath) return;
        setTestSubmitting(true);
        setTestResult(null);
        try {
            const resp = await clientRef.current.testJulesConnection({
                repoPath
            });
            setTestResult({
                ok: resp.ok,
                message: resp.message
            });
        } catch (err) {
            setTestResult({
                ok: false,
                message: "Couldn't reach Jules right now. Try again in a moment. (".concat(err instanceof Error ? err.message : String(err), ")")
            });
        } finally{
            setTestSubmitting(false);
        }
    }
    // handleRevoke calls the dedicated RevokeEgressConsent RPC — the removal
    // counterpart to ConfirmEgressConsent, and the only path that may remove
    // an entry from EgressAcknowledgedRepos (server/services/
    // jules_config_service.go). The row is only removed from local state once
    // the RPC confirms the removal, so a page reload reflects it too.
    async function handleRevoke(repoPath) {
        if (!clientRef.current) return;
        setRevokingRepo(repoPath);
        setSaveError(null);
        try {
            const resp = await clientRef.current.revokeEgressConsent({
                repoPath
            });
            setEgressAcknowledgedRepos(resp.egressAcknowledgedRepos);
            setRevokedNoteRepo(repoPath);
        } catch (err) {
            setSaveError(err instanceof Error ? err.message : String(err));
        } finally{
            setRevokingRepo(null);
        }
    }
    if (loading) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: container,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                    className: heading,
                    children: "Google Jules"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: loadingText,
                    children: "Loading…"
                })
            ]
        });
    }
    if (loadError) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: container,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                    className: heading,
                    children: "Google Jules"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    role: "alert",
                    children: loadError
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: actions,
                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: "btn btn-primary",
                        onClick: loadConfig,
                        children: "Retry"
                    })
                })
            ]
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                className: heading,
                children: "Google Jules"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: description,
                children: "Dispatch backlog items to Jules' cloud coding agent. Code for a dispatched item is sent to Google's infrastructure — see \"Cloud egress\" below before enabling."
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: JulesSettings_css_form,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: toggleRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "jules-enabled",
                                type: "checkbox",
                                checked: enabled,
                                onChange: (e)=>setEnabled(e.target.checked)
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                htmlFor: "jules-enabled",
                                className: toggleLabel,
                                children: "Enable Jules integration"
                            })
                        ]
                    }),
                    enabled && !hasApiKey && !apiKeyInput.trim() && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: warningText,
                        children: "Add an API key below to activate this."
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: field,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                htmlFor: "jules-api-key",
                                className: label,
                                children: "API key"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "jules-api-key",
                                type: "password",
                                className: input,
                                value: apiKeyInput,
                                onChange: (e)=>setApiKeyInput(e.target.value),
                                placeholder: hasApiKey ? "Key stored — enter a new key to replace it" : "Paste your Jules API key",
                                "aria-describedby": "jules-api-key-hint",
                                autoComplete: "off"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                id: "jules-api-key-hint",
                                className: hint,
                                children: "Get a key at jules.google.com/settings"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: field,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                htmlFor: "jules-max-concurrent",
                                className: label,
                                children: "Max concurrent Jules sessions"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "jules-max-concurrent",
                                type: "number",
                                min: MIN_CONCURRENT_SESSIONS,
                                max: MAX_CONCURRENT_SESSIONS,
                                className: input,
                                value: maxConcurrentJulesSessions,
                                onChange: (e)=>setMaxConcurrentJulesSessions(parseInt(e.target.value, 10) || 0),
                                onBlur: handleConcurrentBlur,
                                "aria-describedby": "jules-max-concurrent-hint"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                                id: "jules-max-concurrent-hint",
                                className: hint,
                                children: [
                                    "Default ",
                                    MIN_CONCURRENT_SESSIONS,
                                    ", max ",
                                    MAX_CONCURRENT_SESSIONS
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: field,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                htmlFor: "jules-max-per-day",
                                className: label,
                                children: "Max Jules sessions per day"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "jules-max-per-day",
                                type: "number",
                                min: MIN_SESSIONS_PER_DAY,
                                max: MAX_SESSIONS_PER_DAY,
                                className: input,
                                value: maxJulesSessionsPerDay,
                                onChange: (e)=>setMaxJulesSessionsPerDay(parseInt(e.target.value, 10) || 0),
                                onBlur: handlePerDayBlur,
                                "aria-describedby": "jules-max-per-day-hint"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                                id: "jules-max-per-day-hint",
                                className: hint,
                                children: [
                                    "Default ",
                                    MIN_SESSIONS_PER_DAY,
                                    ", max ",
                                    MAX_SESSIONS_PER_DAY
                                ]
                            })
                        ]
                    }),
                    saveError && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        role: "alert",
                        className: JulesSettings_css_saveError,
                        children: saveError
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: actions,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: "btn btn-primary",
                                onClick: handleSave,
                                disabled: saving,
                                children: saving ? "Saving…" : "Save"
                            }),
                            saveStatusMessage && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                role: "status",
                                className: saveStatus,
                                children: saveStatusMessage
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: field,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                htmlFor: "jules-test-repo-path",
                                className: label,
                                children: "Test connection — repo path"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: inputRow,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(RepoPathInput/* RepoPathInput */.E, {
                                        id: "jules-test-repo-path",
                                        value: testRepoPath,
                                        onChange: setTestRepoPath,
                                        placeholder: "/home/you/code/github.com/owner/repo"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        type: "button",
                                        className: "btn btn-secondary",
                                        onClick: handleTestConnection,
                                        disabled: testSubmitting || !testRepoPath.trim(),
                                        children: testSubmitting ? "Testing…" : "Test connection"
                                    })
                                ]
                            }),
                            testResult && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                role: "status",
                                "data-testid": "jules-test-connection-result",
                                className: testResultStatus,
                                children: testResult.ok ? "Connected — this repo is reachable from Jules." : testResult.message
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: field,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: label,
                                children: "Cloud egress — repos you've allowed"
                            }),
                            egressAcknowledgedRepos.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                className: emptyRepoNote,
                                children: "No repos yet — the confirmation appears the first time you dispatch an item from a new repo."
                            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                                className: repoList,
                                role: "list",
                                children: egressAcknowledgedRepos.map((repoPath)=>{
                                    const label = shortRepoLabel(repoPath);
                                    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                                        className: repoRow,
                                        role: "listitem",
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: repoName,
                                                title: repoPath,
                                                children: label
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                type: "button",
                                                className: revokeBtn,
                                                onClick: ()=>handleRevoke(repoPath),
                                                disabled: revokingRepo === repoPath,
                                                "aria-label": "Revoke cloud-egress consent for ".concat(label),
                                                children: revokingRepo === repoPath ? "Revoking…" : "Revoke"
                                            })
                                        ]
                                    }, repoPath);
                                })
                            }),
                            revokedNoteRepo && /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                                role: "status",
                                className: revokedNote,
                                children: [
                                    "Removed ",
                                    shortRepoLabel(revokedNoteRepo),
                                    "."
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: field,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: label,
                                children: "Usage"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                                className: usageNote,
                                children: [
                                    usage.dispatched,
                                    " dispatched \xb7 ",
                                    usage.completed,
                                    " completed \xb7",
                                    " ",
                                    usage.failed,
                                    " failed"
                                ]
                            })
                        ]
                    })
                ]
            })
        ]
    });
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

/***/ 97363:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 3728));
;
Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 86024));


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
/******/ __webpack_require__.O(0, [1869,852,4202,9353,8182,7924,8165,3575,2513,9691,3516,7358], () => (__webpack_exec__(97363)));
/******/ var __webpack_exports__ = __webpack_require__.O();
/******/ _N_E = __webpack_exports__;
/******/ }
]);
//# sourceMappingURL=page-9cdaac91a1d8099a.js.map