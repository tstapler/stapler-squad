(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[1679],{

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

/***/ 43963:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   i: () => (/* binding */ file_google_protobuf_timestamp),
/* harmony export */   w: () => (/* binding */ TimestampSchema)
/* harmony export */ });
/* harmony import */ var _codegenv2_file_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(1844);
/* harmony import */ var _codegenv2_message_js__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(15033);
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.


/**
 * Describes the file google/protobuf/timestamp.proto.
 */
const file_google_protobuf_timestamp = /*@__PURE__*/ (0,_codegenv2_file_js__WEBPACK_IMPORTED_MODULE_0__/* .fileDesc */ .w)("Ch9nb29nbGUvcHJvdG9idWYvdGltZXN0YW1wLnByb3RvEg9nb29nbGUucHJvdG9idWYiKwoJVGltZXN0YW1wEg8KB3NlY29uZHMYASABKAMSDQoFbmFub3MYAiABKAVChQEKE2NvbS5nb29nbGUucHJvdG9idWZCDlRpbWVzdGFtcFByb3RvUAFaMmdvb2dsZS5nb2xhbmcub3JnL3Byb3RvYnVmL3R5cGVzL2tub3duL3RpbWVzdGFtcHBi+AEBogIDR1BCqgIeR29vZ2xlLlByb3RvYnVmLldlbGxLbm93blR5cGVzYgZwcm90bzM");
/**
 * Describes the message google.protobuf.Timestamp.
 * Use `create(TimestampSchema)` to create a new message.
 */
const TimestampSchema = /*@__PURE__*/ (0,_codegenv2_message_js__WEBPACK_IMPORTED_MODULE_1__/* .messageDesc */ .Z)(file_google_protobuf_timestamp, 0);


/***/ }),

/***/ 60894:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 63367:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   RequireFeatureFlag: () => (/* binding */ RequireFeatureFlag),
/* harmony export */   h: () => (/* binding */ useFeatureFlags),
/* harmony export */   j$: () => (/* binding */ FeatureFlagsProvider),
/* harmony export */   u: () => (/* binding */ useFeatureFlag)
/* harmony export */ });
/* unused harmony export readbackMismatch */
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(61574);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(55494);
/* harmony import */ var _lib_nav__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(41078);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__ = __webpack_require__(66144);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(21694);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(23575);
/* harmony import */ var _featureFlagScopes__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(80070);
/* __next_internal_client_entry_do_not_use__ readbackMismatch,FeatureFlagsProvider,useFeatureFlags,useFeatureFlag,RequireFeatureFlag auto */ 






function scopesOf(flag) {
    return Object.fromEntries(flag.scopes.map((s)=>[
            s.scope,
            s.enabled
        ]));
}
function metaOf(f) {
    return {
        name: f.name,
        enabled: f.enabled,
        description: f.description,
        statusDetail: f.statusDetail,
        scopes: scopesOf(f)
    };
}
/** What the server must have persisted for `change`, or null when the read-back agrees. */ function readbackMismatch(change, flag) {
    if (change.mutation === "reset") return null;
    if (change.scope === _featureFlagScopes__WEBPACK_IMPORTED_MODULE_5__/* .GLOBAL_SCOPE */ .N) {
        return change.mutation === "set" && flag.enabled !== change.enabled ? "global value" : null;
    }
    const entry = flag.scopes.find((s)=>s.scope === change.scope);
    if (change.mutation === "clear") return entry ? change.scope : null;
    return (entry === null || entry === void 0 ? void 0 : entry.enabled) === change.enabled ? null : change.scope;
}
const FeatureFlagsContext = /*#__PURE__*/ (0,react__WEBPACK_IMPORTED_MODULE_1__.createContext)({
    flags: {},
    flagList: [],
    isLoading: true,
    error: null,
    setFlag: async ()=>{},
    refresh: async ()=>{}
});
function FeatureFlagsProvider(param) {
    let { children } = param;
    const [flags, setFlags] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)({});
    const [flagList, setFlagList] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)([]);
    const [isLoading, setIsLoading] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(true);
    const [error, setError] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(null);
    const client = (0,react__WEBPACK_IMPORTED_MODULE_1__.useMemo)(()=>(0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_4__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_3__/* .getConnectTransport */ .qW)()), []);
    const fetchFlags = (0,react__WEBPACK_IMPORTED_MODULE_1__.useCallback)(async ()=>{
        try {
            const res = await client.getFeatureFlags({});
            const map = {};
            const list = [];
            for (const f of res.flags){
                map[f.name] = f.enabled;
                list.push(metaOf(f));
            }
            setFlags(map);
            setFlagList(list);
            setError(null);
        } catch (e) {
            setError("Failed to load feature flags");
        } finally{
            setIsLoading(false);
        }
    }, [
        client
    ]);
    (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        fetchFlags();
    }, [
        fetchFlags
    ]);
    // The displayed state is always the server's read-back, never an optimistic
    // value: a failed or ignored change leaves the previous state and re-reads it.
    const setFlag = (0,react__WEBPACK_IMPORTED_MODULE_1__.useCallback)(async (name, change)=>{
        const req = typeof change === "boolean" ? {
            name,
            enabled: change
        } : change.mutation === "set" ? {
            name,
            scope: change.scope,
            mutation: change.enabled ? _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_4__/* .FlagMutation */ .s34.SET_ENABLED : _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_4__/* .FlagMutation */ .s34.SET_DISABLED
        } : change.mutation === "clear" ? {
            name,
            scope: change.scope,
            mutation: _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_4__/* .FlagMutation */ .s34.CLEAR_SCOPE
        } : {
            name,
            mutation: _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_4__/* .FlagMutation */ .s34.RESET_GLOBAL
        };
        try {
            const res = await client.updateFeatureFlag(req);
            const flag = res.flag;
            if (!flag) throw new Error("response carried no flag");
            if (typeof change !== "boolean") {
                const bad = readbackMismatch(change, flag);
                if (bad) {
                    // An older server that ignores `scope` answers with success and no entry.
                    throw new Error("server read-back does not show the change for ".concat(bad));
                }
            }
            setFlags((prev)=>({
                    ...prev,
                    [flag.name]: flag.enabled
                }));
            setFlagList((prev)=>prev.map((f)=>f.name === flag.name ? {
                        ...f,
                        ...metaOf(flag)
                    } : f));
        } catch (err) {
            console.error("Failed to update feature flag", name, err);
            // Re-read first: a successful fetch clears the error, and this one must outlive it.
            await fetchFlags();
            setError("Failed to update feature flag");
        }
    }, [
        client,
        fetchFlags
    ]);
    const value = (0,react__WEBPACK_IMPORTED_MODULE_1__.useMemo)(()=>({
            flags,
            flagList,
            isLoading,
            error,
            setFlag,
            refresh: fetchFlags
        }), [
        flags,
        flagList,
        isLoading,
        error,
        setFlag,
        fetchFlags
    ]);
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(FeatureFlagsContext.Provider, {
        value: value,
        children: children
    });
}
function useFeatureFlags() {
    return (0,react__WEBPACK_IMPORTED_MODULE_1__.useContext)(FeatureFlagsContext);
}
function useFeatureFlag(name) {
    const { flags } = (0,react__WEBPACK_IMPORTED_MODULE_1__.useContext)(FeatureFlagsContext);
    var _flags_name;
    return (_flags_name = flags[name]) !== null && _flags_name !== void 0 ? _flags_name : false;
}
/**
 * Gates `children` behind a feature flag: renders nothing (and redirects to
 * `redirectTo`) while the flag is disabled or still loading; renders
 * `children` once the flag is confirmed enabled.
 *
 * Intentionally usable from more than one place in a route's render tree
 * (e.g. a segment's `layout.tsx` AND its `page.tsx`) — that's not
 * duplication, it's defense in depth: the layout keeps a disabled flag from
 * ever mounting the page's client bundle, and the same check inside the
 * page guarantees the page's own render output can never show gated content
 * even if it's ever rendered outside that layout (unit tests, future route
 * restructuring). Using a Server Component `page.tsx` to render this Client
 * Component also lets the page keep a static `metadata` export, which a
 * fully-client page cannot do.
 */ function RequireFeatureFlag(param) {
    let { flag, redirectTo = "/", children } = param;
    const { flags, isLoading } = useFeatureFlags();
    const router = (0,_lib_nav__WEBPACK_IMPORTED_MODULE_2__/* .useRouter */ .KA)();
    (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        if (!isLoading && !flags[flag]) {
            router.replace(redirectTo);
        }
    }, [
        isLoading,
        flags,
        flag,
        redirectTo,
        router
    ]);
    if (isLoading) return null;
    if (!flags[flag]) return null;
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.Fragment, {
        children: children
    });
}


/***/ }),

/***/ 65916:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 71120:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 88653));


/***/ }),

/***/ 80070:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   N: () => (/* binding */ GLOBAL_SCOPE)
/* harmony export */ });
/** The scope literal a client sends for a flag's global value (mirrors the server's globalScopeLiteral). */ const GLOBAL_SCOPE = "global";


/***/ }),

/***/ 88653:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  "default": () => (/* binding */ ChangelogPage)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fchangelog%2Fchangelog.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA7VWTXPbIBC951dw6UxywIMUx3bUS_9JBou1xFQCFeFIdsf_vQP6CCDLUVvnYo3ZhV3e233L6i3apeQXaQj6_YAQ43VV0FOCDgW03x-Q_eJG0SpB5tcs0YJnAnMNZZ2gFIQGZZYz4_NO1SPGb9ErnGN1fiZPxlJSlXGRIGL-VJQxLrIZ1z1Nf2ZKHgULHbadXSoGKkFR1aJaFpwFXuB4YUUZP9aTSN1JByk0rvkZJva9taeykCq0mSQvD6sBs-g6ZleQiM8dElzghjOd92DId1CHQjY9wlScmhwUeEFiG-RqOnEIr7Pt2W67bltb28gEWUXxi4ISkZX5LEXwMy7KeRzLRRRYewM8y_XEI_OZePGZ4KLgAvBAyEzF_i8Ahs28T2-9rtqOjHZgOCLkm0tygnLOGAizpqHV-MMARcGrmte3W-AGoGoRoDYqg1QqqrkUCRJSgDE0OdeA64qmYBa7TnfhTXKTrAV5Pj_hc7LpK3AEZLchA0h9VSJ61NKjIjY0RD0JA7ghkviUjDvHtdbFdy9bA4Q9sqdyL1svve18X5EQzWisC68mt4T41yEh7pEPyc5pSqzlVDCjO6vcbT11MnvtuO2OnO1ZZwP9gnnRq2TQP8snhlZU1BVVILQL41Dly7gOoqSLpKjvLXvVBBVwsAmkR1WbiJXkw62nmuFguv9M6v9KMdfEV_00VH2y-SfZ_yp9clJlQapT6q8Xw9XYQQ-CNxinB4fB4tNdhtWyF8UhnNojCtgUlVGieCBryeMhs-fdJ7f8dnU6nnz69vhRAuMUPTrjYLMmVftkXVf-K7SXEcYVpN2gSmVxLK2yB3pSawU6zTvLvKRc3CCbPshYX1HXBdsBWc97O6TkjoP4w_XyB3wAi5JDCwAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fchangelog_2Fchangelog_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA7VWTXPbIBC951dw6UxywIMUx3bUS_9JBou1xFQCFeFIdsf_vQP6CCDLUVvnYo3ZhV3e233L6i3apeQXaQj6_YAQ43VV0FOCDgW03x_Q_eJG0SpB5tcs0YJnAnMNZZ2gFIQGZZYz4_NO1SPGb9ErnGN1fiZPxlJSlXGRIGL_VJQxLrIZ1z1Nf2ZKHgULHbadXSoGKkFR1aJaFpwFXuB4YUUZP9aTSN1JByk0rvkZJva9taeykCq0mSQvD6sBs_g6ZleQiM8dElzghjOd92DId1CHQjY9wlScmhwUeEFiG_RqOnEIr7Pt2W67bltb28gEWUXxi4ISkZX5LEXwMy7KeRzLRRRYewM8y_XEI_OZePGZ4KLgAvBAyEzF_i8Ahs28T2_9rtqOjHZgOCLkm0tygnLOGAizpqHV_MMARcGrmte3W_AGoGoRoDYqg1QqqrkUCRJSgDE0OdeA64qmYBa7TnfhTXKTrAV5Pj_hc7LpK3AEZLchA0h9VSJ61NKjIjY0RD0JA7ghkviUjDvHtdbFdy9bA4Q9sqdyL1svve18X5EQzWisC68mt4T41yEh7pEPyc5pSqzlVDCjO6vcbT11MnvtuO2OnO1ZZwP9gnnRq2TQP8snhlZU1BVVILQL41Dly7gOoqSLpKjvLXvVBBVwsAmkR1WbiJXkw62nmuFguv9M6v9KMdfEV_00VH2y_SfZ_yp9clJlQapT6q8Xw9XYQQ_CNxinB4fB4tNdhtWyF8UhnNojCtgUlVGieCBryeMhs_fdJ7f8dnU6nnz69vhRAuMUPTrjYLMmVftkXVf_K7SXEcYVpN2gSmVxLK2yB3pSawU6zTvLvKRc3CCbPshYX1HXBdsBWc97O6TkjoP4w_XyB3wAi5JDCwAA_22_7D = __webpack_require__(60894);
;// ./src/components/changelog/changelog.css.ts


var entryList = '_18c0q0wf';
var hereChip = '_18c0q0wc';
var muted = '_18c0q0wh';
var page = '_18c0q0w6';
var provenance = '_18c0q0w0';
var provenanceItem = '_18c0q0w1';
var provenanceLabel = '_18c0q0w2';
var provenanceValue = '_18c0q0w3';
var release = '_18c0q0w8';
var releaseBody = '_18c0q0wd';
var releaseCurrent = '_18c0q0w9';
var releaseDate = '_18c0q0wb';
var releaseHeader = '_18c0q0wa';
var releaseHeading = '_18c0q0wi';
var scope = '_18c0q0wg';
var sectionTitle = '_18c0q0we';
var sourceBadge = '_18c0q0w4';
var title = '_18c0q0w7';
var updateBadge = '_18c0q0w5';
;// ./src/components/changelog/BuildProvenance.tsx
/* __next_internal_client_entry_do_not_use__ BuildProvenance auto */ 
// +feature: changelog

function BuildProvenance(param) {
    let { version, branch, commit, isSourceBuild } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("dl", {
        className: provenance,
        "data-testid": "build-provenance",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: provenanceItem,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("dt", {
                        className: provenanceLabel,
                        children: "Version"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("dd", {
                        className: provenanceValue,
                        children: version || "unknown"
                    })
                ]
            }),
            commit && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: provenanceItem,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("dt", {
                        className: provenanceLabel,
                        children: "Commit"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("dd", {
                        className: provenanceValue,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                            children: commit
                        })
                    })
                ]
            }),
            branch && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: provenanceItem,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("dt", {
                        className: provenanceLabel,
                        children: "Branch"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("dd", {
                        className: provenanceValue,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                            children: branch
                        })
                    })
                ]
            }),
            isSourceBuild && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: sourceBadge,
                "data-testid": "source-build-badge",
                children: "Source build"
            })
        ]
    });
}

;// ./src/lib/api/changelogApi.ts
async function getJSON(path, signal) {
    const res = await fetch(path, {
        signal
    });
    if (!res.ok) throw new Error("".concat(path, ": HTTP ").concat(res.status));
    return await res.json();
}
const fetchChangelog = (signal)=>getJSON("/api/changelog", signal);
const fetchUpdateCheck = (signal)=>getJSON("/api/update-check", signal);

// EXTERNAL MODULE: ./src/lib/contexts/FeatureFlagsContext.tsx
var FeatureFlagsContext = __webpack_require__(63367);
;// ./src/components/changelog/UpdateBadge.tsx
/* __next_internal_client_entry_do_not_use__ UpdateBadge auto */ 
// +feature: changelog




/** Small link chip shown only when the opt-in update check found a newer release. */ function UpdateBadge() {
    const { flags } = (0,FeatureFlagsContext/* useFeatureFlags */.h)();
    const enabled = !!flags["changelog"];
    const [result, setResult] = (0,react.useState)(null);
    (0,react.useEffect)(()=>{
        if (!enabled || typeof fetch !== "function") return;
        const ctrl = new AbortController();
        fetchUpdateCheck(ctrl.signal).then(setResult).catch(()=>{
        // Best-effort: a missing badge is the correct failure mode.
        });
        return ()=>ctrl.abort();
    }, [
        enabled
    ]);
    if (!enabled || !(result === null || result === void 0 ? void 0 : result.enabled) || result.is_source_build || !result.is_outdated) return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("a", {
        className: updateBadge,
        href: result.latest_url || "/changelog",
        target: "_blank",
        rel: "noopener noreferrer",
        title: "Version ".concat(result.latest_version, " is available (you have ").concat(result.current_version, ")"),
        "data-testid": "update-badge",
        children: [
            "Update available: v",
            result.latest_version
        ]
    });
}

;// ./src/app/changelog/page.tsx
// analytics-exempt
/* __next_internal_client_entry_do_not_use__ default auto */ 
// +feature: changelog





const normalize = (v)=>v.replace(/^v/, "");
function ChangelogPage() {
    const [data, setData] = (0,react.useState)(null);
    const [error, setError] = (0,react.useState)(null);
    const [open, setOpen] = (0,react.useState)({});
    const currentRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        const ctrl = new AbortController();
        fetchChangelog(ctrl.signal).then((d)=>{
            setData(d);
            const cur = normalize(d.current_version || "");
            const match = d.releases.find((r)=>r.version === cur);
            const first = match !== null && match !== void 0 ? match : d.releases[0];
            if (first) setOpen({
                [first.version]: true
            });
        }).catch((e)=>{
            if (!ctrl.signal.aborted) setError(e instanceof Error ? e.message : "Failed to load changelog");
        });
        return ()=>ctrl.abort();
    }, []);
    (0,react.useEffect)(()=>{
        if (data && currentRef.current && typeof currentRef.current.scrollIntoView === "function") {
            currentRef.current.scrollIntoView({
                block: "start"
            });
        }
    }, [
        data
    ]);
    if (error) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("main", {
            id: "main-content",
            className: page,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("h1", {
                    className: title,
                    children: "Changelog"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                    role: "alert",
                    children: [
                        "Couldn't load the changelog: ",
                        error
                    ]
                })
            ]
        });
    }
    if (!data) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("main", {
            id: "main-content",
            className: page,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("h1", {
                    className: title,
                    children: "Changelog"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: muted,
                    children: "Loading…"
                })
            ]
        });
    }
    const current = normalize(data.current_version || "");
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("main", {
        id: "main-content",
        className: page,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("h1", {
                className: title,
                children: "Changelog"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(BuildProvenance, {
                version: data.current_version,
                branch: data.branch,
                commit: data.commit,
                isSourceBuild: data.is_source_build
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(UpdateBadge, {}),
            data.releases.length === 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: muted,
                children: "No release notes are bundled with this build."
            }),
            data.releases.map((rel)=>{
                const isCurrent = rel.version === current;
                const isOpen = !!open[rel.version];
                const bodyId = "release-".concat(rel.version);
                return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
                    id: bodyId,
                    ref: isCurrent ? currentRef : undefined,
                    className: "".concat(release, " ").concat(isCurrent ? releaseCurrent : ""),
                    "data-testid": "release-".concat(rel.version),
                    "aria-current": isCurrent ? "true" : undefined,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                            className: releaseHeading,
                            children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                type: "button",
                                className: releaseHeader,
                                "aria-expanded": isOpen,
                                "aria-controls": "".concat(bodyId, "-body"),
                                onClick: ()=>setOpen((o)=>({
                                            ...o,
                                            [rel.version]: !o[rel.version]
                                        })),
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                        children: [
                                            "v",
                                            rel.version
                                        ]
                                    }),
                                    rel.date && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: releaseDate,
                                        children: rel.date
                                    }),
                                    isCurrent && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: hereChip,
                                        children: "You are here"
                                    })
                                ]
                            })
                        }),
                        isOpen && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            id: "".concat(bodyId, "-body"),
                            className: releaseBody,
                            children: rel.sections.map((sec)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                                            className: sectionTitle,
                                            children: sec.title
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                                            className: entryList,
                                            children: sec.entries.map((e, i)=>{
                                                var _e_prs;
                                                return /*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                                                    children: [
                                                        e.scope && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                            className: scope,
                                                            children: [
                                                                e.scope,
                                                                ": "
                                                            ]
                                                        }),
                                                        e.text,
                                                        (_e_prs = e.prs) === null || _e_prs === void 0 ? void 0 : _e_prs.map((n)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                                className: muted,
                                                                children: [
                                                                    " ",
                                                                    "(#",
                                                                    n,
                                                                    ")"
                                                                ]
                                                            }, n))
                                                    ]
                                                }, i);
                                            })
                                        })
                                    ]
                                }, sec.title))
                        })
                    ]
                }, rel.version);
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
/******/ __webpack_require__.O(0, [1869,852,4202,9353,8165,3575,9691,3516,7358], () => (__webpack_exec__(71120)));
/******/ var __webpack_exports__ = __webpack_require__.O();
/******/ _N_E = __webpack_exports__;
/******/ }
]);
//# sourceMappingURL=page-bdf58abd30338335.js.map