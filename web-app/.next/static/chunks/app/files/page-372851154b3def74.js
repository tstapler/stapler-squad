(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[9638],{

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

/***/ 72559:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 94681));


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

/***/ 94681:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  "default": () => (/* binding */ FilesPage)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/nav/index.ts
var nav = __webpack_require__(41078);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/folder.mjs
var folder = __webpack_require__(56214);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/globe.mjs
var globe = __webpack_require__(12859);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/file-image.mjs
var file_image = __webpack_require__(75100);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/film.mjs
var film = __webpack_require__(11572);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/file-text.mjs
var file_text = __webpack_require__(30294);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/file.mjs
var file = __webpack_require__(68108);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/external-link.mjs
var external_link = __webpack_require__(33906);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/x.mjs
var x = __webpack_require__(91208);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/arrow-up.mjs
var arrow_up = __webpack_require__(22781);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/square-terminal.mjs
var square_terminal = __webpack_require__(34782);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/chevron-right.mjs
var chevron_right = __webpack_require__(18776);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/folder-open.mjs
var folder_open = __webpack_require__(58409);
// EXTERNAL MODULE: ./src/components/ui/RepoPathInput.tsx + 2 modules
var RepoPathInput = __webpack_require__(34609);
// EXTERNAL MODULE: ./src/lib/hooks/useSessionService.ts + 1 modules
var useSessionService = __webpack_require__(73117);
// EXTERNAL MODULE: ./src/lib/contexts/AnalyticsContext.tsx
var AnalyticsContext = __webpack_require__(24044);
// EXTERNAL MODULE: ./src/gen/session/v1/types_pb.ts
var types_pb = __webpack_require__(81432);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Ffiles%2FLocalFileBrowser.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61YzXKbMBC-5yl06Ux8IAPEcVzl0jfpyEiAYiERIWKcTt69w5-RAAlwe0niSPv37berXT_9fjnQ5J354M8DAJgWOUNXCGJGqrcH0Pz2MJUkUlRwCCLByozXJymhSaogCHz_R_1ZfBIZM3GBIKUYE_728P3w1CkPtipPUA7BJ5KPnvc7-Em-QvkVfu3qkxxhTHkyPn32m9OTkJhI7ySUEhkEQV6BQjCKR7dJexlF50SKkuOxttfdzb0ilZSfIfD1eML5eOxeI0YT7lFFsgKCiHBFpK7vudFXq4EgqO9nlHsXilU6MrwfX7THEKDGciSYkJOzk4aWHaazjqlEmJbFBPeDmZVDXoHAz9v0Cq68gn6RiVBrXpSKUU4g4IKTm0CMMsquExzLnYEDjEVUFg0anX-zgTJD6qW9b4UssyP2YQDWO7wdmv06aJrzS1dhoxtJ62Upi9rNXNCWTS6-vsC0Ls-F8LkJFkSRop9kQUgYQof5wpilPwDvZaFofPUiwRXhSj_SDZpoL1b1ipzMpji04XrL4EtegWNeubE-tFhDLtQjxLRAJ0bwbgHFo90t3wT4prLRKHIUUXWFwH_a695zoTzEmLgQrEu_OvrwRdbdq_7pyFjT4cIlDjs6T2hEc9xEl8H4oOFno2HW1HPjxTrlg0I0ydSYfv3nO0kU5hXousAaqmooU54SSZXhrVbYVvYsEW9Qd7Lzo3tvXK982yq6Zys8-maQ3bTgrN1eu3eFAJVKzFXahjlicC1uXLPNDsFSU2iToK6szoJCjEb1fxWp6iqjCddZ6m7tg0uJjlY_RG0qha4VmQ9MV5yrCDxlqR4SI7HBtWTVI2ISKl24Hbya11eZCEwb73b2h7Nz3PamcB6PXdMy6LAbDghjNC9oUR9dUqqIV-Qoaqadvsk6KnYti9hi8Atd2v6QZWuCvqMW-Vjvhr1g-yBhn8nvAs1EX_xDKK5Omm9_F93L0l5vDet2pPgfd6SPMTZLM-_8prL7X-U2eCa3o7tf7qrul80R3KYl4nkYQec2AGexawisbuWrhtJinOrRqzZZm6wmD4Ze1ejNUOWNFNb_mnz_cHonkfJiWpe_4ApR3l6VCeXDOHFL-4mJ6Oz6RiEwfCm3-LJkdFD7aW-yvbB7alname-su6W5saVlH6c_qbpcEj3MSxvm_RvDr4xgikARSUI4QByDRy0Xr4djXrU71jDK9n9FwBxQtXT0hPyeFcR2wb5nTKg-nnfdFgbXMpuFJdcmgoPF778uhl7e3BMAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Ffiles_2FLocalFileBrowser_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61YzXKbMBC_5yl06Ux8IAPEcVzl0jfpyEiAYiERIWKcTt69w5_RAAlwe0niSPv37berXT_9fjnQ5J354M8DAJgWOUNXCGJGqrcH0Pz2MJUkUlRwCCLByozXJymhSaogCHz_R_1ZfBIZM3GBIKUYE_728P3w1CkPtipPUA7BJ5KPnvc7_Em_QvkVfu3qkxxhTHkyPn32m9OTkJhI7ySUEhkEQV6BQjCKR7dJexlF50SKkuOxttfdzb0ilZSfIfD1eML5eOxeI0YT7lFFsgKCiHBFpK7vudFXq4EgqO9nlHsXilU6MrwfX7THEKDGciSYkJOzk4aWHaazjqlEmJbFBPeDmZVDXoHAz9v0Cq68gn6RiVBrXpSKUU4g4IKTm0CMMsquExzLnYEDjEVUFg0anX_zgTJD6qW9b4UssyP2YQDWO7wdmv06aJrzS1dhoxtJ62Upi9rNXNCWTS6_vsC0Ls_F8LkJFkSRop9kQUgYQof5wpilPwDvZaFofPUiwRXhSj_SDZpoL1b1ipzMpji04XrL4EtegWNeubE_tFhDLtQjxLRAJ0bwbgHFo90t3wT4prLRKHIUUXWFwH_a695zoTzEmLgQrEu_OvrwRdbdq_7pyFjT4cIlDjs6T2hEc9xEl8H4oOFno2HW1HPjxTrlg0I0ydSYfv3nO0kU5hXousAaqmooU54SSZXhrVbYVvYsEW9Qd7Lzo3tvXK982yq6Zys8_maQ3bTgrN1eu3eFAJVKzFXahjlicC1uXLPNDsFSU2iToK6szoJCjEb1fxWp6iqjCddZ6m7tg0uJjlY_RG0qha4VmQ9MV5yrCDxlqR4SI7HBtWTVI2ISKl24Hbya11eZCEwb73b2h7Nz3PamcB6PXdMy6LAbDghjNC9oUR9dUqqIV_Qoaqadvsk6KnYti9hi8Atd2v6QZWuCvqMW_Vjvhr1g_yBhn8nvAs1EX_xDKK5Omm9_F93L0l5vDet2pPgfd6SPMTZLM_8prL7X_U2eCa3o7tf7qrul80R3KYl4nkYQec2AGexawisbuWrhtJinOrRqzZZm6wmD4Ze1ejNUOWNFNb_mnz_cHonkfJiWpe_4ApR3l6VCeXDOHFL_4mJ6Oz6RiEwfCm3_LJkdFD7aW_yvbB7alname_su6W5saVlH6c_qbpcEj3MSxvm_RvDr4xgikARSUI4QByDRy0Xr4djXrU71jDK9n9FwBxQtXT0hPyeFcR2wb5nTKg_nnfdFgbXMpuFJdcmgoPF778uhl7e3BMAAA_22_7D = __webpack_require__(10767);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+recipes@0.5.7+6974e6bd53375d7a/node_modules/@vanilla-extract/recipes/dist/createRuntimeFn-62c9670f.esm.js
var createRuntimeFn_62c9670f_esm = __webpack_require__(8462);
;// ./src/components/files/LocalFileBrowser.css.ts


var breadcrumbLink = '_56igjla';
var breadcrumbSep = '_56igjl9';
var breadcrumbs = '_56igjl7';
var content = (0,createRuntimeFn_62c9670f_esm.c)({defaultClassName:'_56igjlb',variantClassNames:{fileOpen:{true:'_56igjlc',false:'_56igjld'}},defaultVariants:{fileOpen:false},compoundVariants:[]});
var crumbGroup = '_56igjl8';
var externalButton = '_56igjlr';
var fileEntry = (0,createRuntimeFn_62c9670f_esm.c)({defaultClassName:'_56igjlg',variantClassNames:{active:{true:'_56igjlh',false:'_56igjli'}},defaultVariants:{active:false},compoundVariants:[]});
var fileIcon = '_56igjlj';
var fileName = '_56igjlk';
var fileSize = '_56igjll';
var page = '_56igjl0';
var pathBar = '_56igjl1';
var pathButton = '_56igjl5';
var pathForm = '_56igjl2';
var LocalFileBrowser_css_pathInput = '_56igjl4';
var pathInputWrapper = '_56igjl3';
var sidebar = '_56igjle';
var sidebarEmpty = '_56igjlf';
var upButton = '_56igjl6';
var viewer = '_56igjlm';
var viewerEmpty = '_56igjln';
var viewerFrame = '_56igjls';
var viewerHint = '_56igjlw';
var viewerImage = '_56igjlt';
var viewerLabel = '_56igjlq';
var viewerText = '_56igjlv';
var viewerToolbar = '_56igjlp';
var viewerVideo = '_56igjlu';
var viewerWrapper = '_56igjlo';
;// ./src/components/files/LocalFileBrowser.tsx
// +feature: local-file-browser
/* __next_internal_client_entry_do_not_use__ serveUrl,LocalFileBrowser auto */ 








function fromRawListing(raw) {
    return {
        path: raw.path,
        total: raw.total,
        hasMore: raw.has_more,
        entries: raw.entries.map((e)=>({
                name: e.name,
                path: e.path,
                isDir: e.is_dir,
                size: e.size
            }))
    };
}
// Module-level constant — avoids allocating a new Set on every render/call.
const TEXT_EXTENSIONS = new Set([
    "txt",
    "md",
    "markdown",
    "rst",
    "log",
    "diff",
    "patch",
    "js",
    "mjs",
    "cjs",
    "ts",
    "tsx",
    "jsx",
    "css",
    "scss",
    "sass",
    "less",
    "json",
    "jsonc",
    "yaml",
    "yml",
    "toml",
    "ini",
    "env",
    "envrc",
    "conf",
    "cfg",
    "xml",
    "go",
    "py",
    "rb",
    "rs",
    "swift",
    "kt",
    "java",
    "scala",
    "c",
    "cpp",
    "cc",
    "cxx",
    "h",
    "hpp",
    "sh",
    "bash",
    "zsh",
    "fish",
    "bat",
    "cmd",
    "ps1",
    "sql",
    "graphql",
    "gql",
    "proto",
    "tf",
    "tfvars",
    "hcl",
    "mod",
    "sum",
    "lock",
    "dockerfile",
    "makefile",
    "mk",
    "gitignore",
    "gitattributes",
    "editorconfig",
    "php",
    "lua",
    "r",
    "pl",
    "pm"
]);
const MAX_TEXT_PREVIEW_BYTES = 2 * 1024 * 1024; // 2 MB
function inferRenderMode(filename) {
    var _filename_split_pop;
    var _filename_split_pop_toLowerCase;
    const ext = (_filename_split_pop_toLowerCase = (_filename_split_pop = filename.split(".").pop()) === null || _filename_split_pop === void 0 ? void 0 : _filename_split_pop.toLowerCase()) !== null && _filename_split_pop_toLowerCase !== void 0 ? _filename_split_pop_toLowerCase : "";
    if ([
        "html",
        "htm"
    ].includes(ext)) return "html";
    if (ext === "svg") return "svg";
    if ([
        "jpg",
        "jpeg",
        "png",
        "gif",
        "webp",
        "ico",
        "bmp",
        "avif",
        "tiff",
        "tif"
    ].includes(ext)) return "image";
    if (ext === "pdf") return "pdf";
    if ([
        "mp4",
        "webm",
        "ogv",
        "mov",
        "avi"
    ].includes(ext)) return "video";
    if (TEXT_EXTENSIONS.has(ext)) return "text";
    return "binary";
}
function fileIconForEntry(entry) {
    if (entry.isDir) return folder/* default */.A;
    const mode = inferRenderMode(entry.name);
    switch(mode){
        case "html":
            return globe/* default */.A;
        case "svg":
        case "image":
            return file_image/* default */.A;
        case "video":
            return film/* default */.A;
        case "text":
            return file_text/* default */.A;
        default:
            return file/* default */.A;
    }
}
function serveUrl(absPath) {
    // Per-segment encoding so `#`, `?`, spaces, and unicode in filenames survive
    // as path characters instead of being read as a URL fragment/query or breaking.
    const encoded = absPath.split("/").map(encodeURIComponent).join("/");
    return "/api/local/serve".concat(encoded);
}
function formatSize(bytes) {
    if (bytes < 1024) return "".concat(bytes, " B");
    if (bytes < 1024 * 1024) return "".concat((bytes / 1024).toFixed(1), " KB");
    return "".concat((bytes / (1024 * 1024)).toFixed(1), " MB");
}
function parentDir(path) {
    const cleaned = path.replace(/\/$/, "");
    const idx = cleaned.lastIndexOf("/");
    if (idx <= 0) return "/";
    return cleaned.slice(0, idx);
}
function buildBreadcrumbs(path) {
    const parts = path.split("/").filter(Boolean);
    const crumbs = [
        {
            label: "/",
            path: "/"
        }
    ];
    for(let i = 0; i < parts.length; i++){
        crumbs.push({
            label: parts[i],
            path: "/" + parts.slice(0, i + 1).join("/")
        });
    }
    return crumbs;
}
function ViewerToolbar(param) {
    let { name, url, openLabel = "Open in new tab", onClose } = param;
    const handleOpen = ()=>window.open(url, "_blank", "noopener");
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: viewerToolbar,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: viewerLabel,
                children: name
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                onClick: handleOpen,
                className: externalButton,
                title: openLabel,
                "data-testid": "file-browser-viewer-open-external",
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(external_link/* default */.A, {
                    size: 14
                })
            }),
            onClose && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                onClick: onClose,
                className: externalButton,
                title: "Close file viewer",
                "data-testid": "file-browser-viewer-close",
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(x/* default */.A, {
                    size: 14
                })
            })
        ]
    });
}
function FileViewer(param) {
    let { entry, onClose } = param;
    const [textContent, setTextContent] = (0,react.useState)(null);
    const [textError, setTextError] = (0,react.useState)(null);
    const url = serveUrl(entry.path);
    const mode = inferRenderMode(entry.name);
    (0,react.useEffect)(()=>{
        if (mode !== "text") {
            setTextContent(null);
            setTextError(null);
            return;
        }
        if (entry.size > MAX_TEXT_PREVIEW_BYTES) {
            setTextError("File too large to preview (".concat(formatSize(entry.size), ") — open externally"));
            return;
        }
        const controller = new AbortController();
        setTextContent(null);
        setTextError(null);
        fetch(url, {
            signal: controller.signal
        }).then((r)=>{
            if (!r.ok) throw new Error("HTTP ".concat(r.status));
            return r.text();
        }).then(setTextContent).catch((e)=>{
            if (e instanceof Error && e.name === "AbortError") return;
            setTextError(e instanceof Error ? e.message : String(e));
        });
        return ()=>controller.abort();
    }, [
        url,
        mode,
        entry.size
    ]);
    switch(mode){
        case "html":
            return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: viewerWrapper,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(ViewerToolbar, {
                        name: entry.name,
                        url: url,
                        onClose: onClose
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("iframe", {
                        src: url,
                        className: viewerFrame,
                        sandbox: "allow-scripts allow-forms allow-popups allow-modals allow-downloads",
                        title: entry.name
                    })
                ]
            });
        case "svg":
        case "image":
            return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: viewerWrapper,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(ViewerToolbar, {
                        name: entry.name,
                        url: url,
                        onClose: onClose
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("img", {
                        src: url,
                        alt: entry.name,
                        className: viewerImage
                    })
                ]
            });
        case "pdf":
            return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: viewerWrapper,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(ViewerToolbar, {
                        name: entry.name,
                        url: url,
                        onClose: onClose
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("iframe", {
                        src: url,
                        className: viewerFrame,
                        sandbox: "allow-scripts allow-forms allow-popups",
                        title: entry.name
                    })
                ]
            });
        case "video":
            return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: viewerWrapper,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(ViewerToolbar, {
                        name: entry.name,
                        url: url,
                        onClose: onClose
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("video", {
                        controls: true,
                        src: url,
                        className: viewerVideo
                    })
                ]
            });
        case "text":
            if (textError) {
                return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: viewerWrapper,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)(ViewerToolbar, {
                            name: entry.name,
                            url: url,
                            onClose: onClose
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: viewerEmpty,
                            children: [
                                "Failed to load: ",
                                textError
                            ]
                        })
                    ]
                });
            }
            if (textContent === null) {
                return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: viewerWrapper,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)(ViewerToolbar, {
                            name: entry.name,
                            url: url,
                            onClose: onClose
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: viewerEmpty,
                            children: "Loading…"
                        })
                    ]
                });
            }
            return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: viewerWrapper,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(ViewerToolbar, {
                        name: entry.name,
                        url: url,
                        openLabel: "Open raw",
                        onClose: onClose
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("pre", {
                        className: viewerText,
                        children: textContent
                    })
                ]
            });
        default:
            return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: viewerWrapper,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(ViewerToolbar, {
                        name: entry.name,
                        url: url,
                        onClose: onClose
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: viewerEmpty,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(file/* default */.A, {
                                size: 32
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                children: entry.name
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: viewerHint,
                                children: [
                                    formatSize(entry.size),
                                    " — binary file"
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                onClick: ()=>window.open(url, "_blank", "noopener"),
                                className: externalButton,
                                "data-testid": "file-browser-viewer-download",
                                children: [
                                    "Open / Download ",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(external_link/* default */.A, {
                                        size: 14
                                    })
                                ]
                            })
                        ]
                    })
                ]
            });
    }
}
function LocalFileBrowser() {
    const searchParams = (0,nav/* useSearchParams */.ok)();
    const router = (0,nav/* useRouter */.KA)();
    var _searchParams_get;
    const initialPath = (_searchParams_get = searchParams.get("path")) !== null && _searchParams_get !== void 0 ? _searchParams_get : "/";
    const [pathInput, setPathInput] = (0,react.useState)(initialPath);
    const [currentPath, setCurrentPath] = (0,react.useState)(initialPath);
    const [listing, setListing] = (0,react.useState)(null);
    const [listError, setListError] = (0,react.useState)(null);
    const [selectedEntry, setSelectedEntry] = (0,react.useState)(null);
    const [loading, setLoading] = (0,react.useState)(false);
    const [filterText, setFilterText] = (0,react.useState)("");
    const [terminalError, setTerminalError] = (0,react.useState)(null);
    const [openingTerminal, setOpeningTerminal] = (0,react.useState)(false);
    const { createSession } = (0,useSessionService/* useSessionService */.D)({
        enabled: true
    });
    const analytics = (0,AnalyticsContext/* useAnalytics */.st)();
    const navigate = (0,react.useCallback)((path)=>{
        setCurrentPath(path);
        setPathInput(path);
        setSelectedEntry(null);
        setFilterText("");
        router.replace("/files?path=".concat(encodeURIComponent(path)));
    }, [
        router
    ]);
    (0,react.useEffect)(()=>{
        const controller = new AbortController();
        setLoading(true);
        setListError(null);
        setListing(null);
        fetch("/api/local/files/list?path=".concat(encodeURIComponent(currentPath)), {
            signal: controller.signal
        }).then((r)=>{
            if (!r.ok) return r.text().then((t)=>{
                throw new Error((t || "HTTP ".concat(r.status)).trim());
            });
            return r.json();
        }).then((data)=>{
            setListing(fromRawListing(data));
        }).catch((e)=>{
            if (e instanceof Error && e.name === "AbortError") return;
            setListError(e instanceof Error ? e.message : String(e));
        }).finally(()=>setLoading(false));
        return ()=>controller.abort();
    }, [
        currentPath
    ]);
    const handlePathSubmit = (e)=>{
        e.preventDefault();
        navigate(pathInput.trim() || "/");
    };
    const handleEntryClick = (entry)=>{
        if (entry.isDir) {
            navigate(entry.path);
        } else {
            setSelectedEntry(entry);
        }
    };
    const handleOpenTerminal = async ()=>{
        setTerminalError(null);
        setOpeningTerminal(true);
        try {
            const session = await createSession({
                path: currentPath,
                sessionType: types_pb/* SessionType */.dF.DIRECTORY
            });
            analytics.track({
                name: "file_browser.open_terminal",
                category: "user_action",
                component: "LocalFileBrowser"
            });
            if (session) {
                router.push("/?session=".concat(session.id));
            } else {
                setTerminalError("Failed to create session");
            }
        } catch (e) {
            setTerminalError(e instanceof Error ? e.message : String(e));
        } finally{
            setOpeningTerminal(false);
        }
    };
    const crumbs = buildBreadcrumbs(currentPath);
    var _listing_entries;
    const filteredEntries = ((_listing_entries = listing === null || listing === void 0 ? void 0 : listing.entries) !== null && _listing_entries !== void 0 ? _listing_entries : []).filter((entry)=>entry.name.toLowerCase().includes(filterText.trim().toLowerCase()));
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: page,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: pathBar,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("form", {
                        onSubmit: handlePathSubmit,
                        className: pathForm,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: pathInputWrapper,
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(RepoPathInput/* RepoPathInput */.E, {
                                    value: pathInput,
                                    onChange: setPathInput,
                                    onSelect: (entry)=>navigate(entry.path),
                                    placeholder: "/path/to/directory",
                                    "data-testid": "file-browser-path-input"
                                })
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "submit",
                                className: pathButton,
                                "data-testid": "file-browser-go-button",
                                children: "Go"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: upButton,
                                onClick: ()=>navigate(parentDir(currentPath)),
                                disabled: currentPath === "/",
                                title: "Go up one level",
                                "data-testid": "file-browser-up-button",
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(arrow_up/* default */.A, {
                                    size: 16
                                })
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: upButton,
                                onClick: handleOpenTerminal,
                                disabled: openingTerminal,
                                title: "Open terminal here",
                                "data-testid": "file-browser-open-terminal",
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(square_terminal/* default */.A, {
                                    size: 16
                                })
                            })
                        ]
                    }),
                    terminalError && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: sidebarEmpty,
                        children: terminalError
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("nav", {
                        className: breadcrumbs,
                        "aria-label": "Path breadcrumbs",
                        children: crumbs.map((crumb, i)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: crumbGroup,
                                children: [
                                    i > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: breadcrumbSep,
                                        "aria-hidden": "true",
                                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(chevron_right/* default */.A, {
                                            size: 12
                                        })
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        className: breadcrumbLink,
                                        onClick: ()=>navigate(crumb.path),
                                        "data-testid": "file-browser-breadcrumb-".concat(i),
                                        children: crumb.label
                                    })
                                ]
                            }, crumb.path))
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        className: LocalFileBrowser_css_pathInput,
                        value: filterText,
                        onChange: (e)=>setFilterText(e.target.value),
                        placeholder: "Filter entries…",
                        "aria-label": "Filter entries",
                        spellCheck: false,
                        "data-testid": "file-browser-filter-input"
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: content({
                    fileOpen: !!selectedEntry
                }),
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("aside", {
                        className: sidebar,
                        "data-testid": "file-browser-entry-list",
                        children: [
                            loading && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: sidebarEmpty,
                                children: "Loading…"
                            }),
                            listError && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: sidebarEmpty,
                                children: listError
                            }),
                            !loading && !listError && listing && listing.entries.length === 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: sidebarEmpty,
                                children: "Empty directory"
                            }),
                            !loading && !listError && listing && listing.entries.length > 0 && filteredEntries.length === 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: sidebarEmpty,
                                children: "No matching entries"
                            }),
                            (listing === null || listing === void 0 ? void 0 : listing.hasMore) && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: sidebarEmpty,
                                "data-testid": "file-browser-truncation-notice",
                                children: [
                                    "Showing first ",
                                    listing.entries.length,
                                    " of ",
                                    listing.total,
                                    " entries"
                                ]
                            }),
                            filteredEntries.map((entry)=>{
                                const Icon = fileIconForEntry(entry);
                                const isActive = (selectedEntry === null || selectedEntry === void 0 ? void 0 : selectedEntry.path) === entry.path;
                                return /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                    className: fileEntry({
                                        active: isActive
                                    }),
                                    onClick: ()=>handleEntryClick(entry),
                                    title: entry.path,
                                    "data-testid": "file-browser-entry",
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: fileIcon,
                                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(Icon, {
                                                size: 15
                                            })
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                            className: fileName,
                                            children: [
                                                entry.name,
                                                entry.isDir ? "/" : ""
                                            ]
                                        }),
                                        !entry.isDir && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: fileSize,
                                            children: formatSize(entry.size)
                                        })
                                    ]
                                }, entry.path);
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: viewer,
                        children: selectedEntry ? /*#__PURE__*/ (0,jsx_runtime.jsx)(FileViewer, {
                            entry: selectedEntry,
                            onClose: ()=>setSelectedEntry(null)
                        }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: viewerEmpty,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)(folder_open/* default */.A, {
                                    size: 40
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    children: "Select a file to view"
                                })
                            ]
                        })
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/analytics/usePageView.ts
var usePageView = __webpack_require__(90814);
;// ./src/app/files/page.tsx
/* __next_internal_client_entry_do_not_use__ default auto */ 
// +feature: local-file-browser



function FilesPage() {
    (0,usePageView/* usePageView */.O)();
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(react.Suspense, {
        fallback: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            children: "Loading file browser..."
        }),
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(LocalFileBrowser, {})
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
/******/ __webpack_require__.O(0, [1869,852,4202,9353,8182,7924,2099,4598,8165,3575,2513,2275,3117,9691,3516,7358], () => (__webpack_exec__(72559)));
/******/ var __webpack_exports__ = __webpack_require__.O();
/******/ _N_E = __webpack_exports__;
/******/ }
]);
//# sourceMappingURL=page-372851154b3def74.js.map