(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[664],{

/***/ 4305:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 6579:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 10671:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Or: () => (/* reexport safe */ _usePageView__WEBPACK_IMPORTED_MODULE_0__.O),
/* harmony export */   st: () => (/* reexport safe */ _lib_contexts_AnalyticsContext__WEBPACK_IMPORTED_MODULE_1__.st)
/* harmony export */ });
/* harmony import */ var _usePageView__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(90814);
/* harmony import */ var _lib_contexts_AnalyticsContext__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(24044);






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

/***/ 26991:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 28765:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   FP: () => (/* binding */ timestampDate),
/* harmony export */   LI: () => (/* binding */ timestampFromDate),
/* harmony export */   R5: () => (/* binding */ timestampMs)
/* harmony export */ });
/* unused harmony exports timestampNow, timestampFromMs */
/* harmony import */ var _gen_google_protobuf_timestamp_pb_js__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(43963);
/* harmony import */ var _create_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(95444);
/* harmony import */ var _proto_int64_js__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(2952);
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
 * Create a google.protobuf.Timestamp for the current time.
 */
function timestampNow() {
    return timestampFromDate(new Date());
}
/**
 * Create a google.protobuf.Timestamp message from an ECMAScript Date.
 */
function timestampFromDate(date) {
    return timestampFromMs(date.getTime());
}
/**
 * Convert a google.protobuf.Timestamp message to an ECMAScript Date.
 */
function timestampDate(timestamp) {
    return new Date(timestampMs(timestamp));
}
/**
 * Create a google.protobuf.Timestamp message from a Unix timestamp in milliseconds.
 */
function timestampFromMs(timestampMs) {
    const seconds = Math.floor(timestampMs / 1000);
    return (0,_create_js__WEBPACK_IMPORTED_MODULE_0__/* .create */ .v)(_gen_google_protobuf_timestamp_pb_js__WEBPACK_IMPORTED_MODULE_1__/* .TimestampSchema */ .w, {
        seconds: _proto_int64_js__WEBPACK_IMPORTED_MODULE_2__/* .protoInt64 */ .M.parse(seconds),
        nanos: (timestampMs - seconds * 1000) * 1000000,
    });
}
/**
 * Convert a google.protobuf.Timestamp to a Unix timestamp in milliseconds.
 */
function timestampMs(timestamp) {
    return (Number(timestamp.seconds) * 1000 + Math.round(timestamp.nanos / 1000000));
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

/***/ 49051:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   HN: () => (/* binding */ PI_SUPPORT_FLAG_NAME),
/* harmony export */   i: () => (/* binding */ PROGRAMS),
/* harmony export */   kg: () => (/* binding */ MODEL_AUTOCOMPLETE_OPTIONS),
/* harmony export */   oV: () => (/* binding */ isKnownProgram),
/* harmony export */   pw: () => (/* binding */ getProgramDisplay),
/* harmony export */   qG: () => (/* binding */ getPickerPrograms)
/* harmony export */ });
/* unused harmony exports DEFAULT_PROGRAM, CLAUDE_MODELS, MODEL_FAMILIES */
// pi-support's flag name (mirrors config.FeaturePiSupport / feature_flag_service.go's
// piSupportFlagName — kept as a plain string constant here since the frontend has no
// shared import path to those Go constants). Single source of truth for the frontend;
// import this everywhere instead of redeclaring the literal.
const PI_SUPPORT_FLAG_NAME = "pi-support";
const PROGRAMS = [
    {
        value: "",
        label: "System default",
        description: "Use default_program from config (default: claude)"
    },
    {
        value: "claude",
        label: "Claude Code",
        description: "Anthropic's CLI assistant"
    },
    {
        value: "env -u CLAUDE_CODE_USE_BEDROCK ANTHROPIC_BASE_URL=http://localhost:47000 claude",
        label: "Claude Code (Proxy via localhost:47000)",
        description: "Via local proxy"
    },
    {
        value: "pi",
        label: "pi",
        description: "@earendil-works/pi-coding-agent — TypeScript-extensible CLI"
    },
    {
        value: "aider",
        label: "Aider",
        description: "AI pair programming with git"
    },
    {
        value: "aider --model ollama_chat/gemma3:1b",
        label: "Aider (Ollama Gemma 1B)",
        description: "Local model"
    },
    {
        value: "opencode",
        label: "OpenCode",
        description: "OpenCode CLI assistant"
    },
    {
        value: "gemini",
        label: "Gemini CLI",
        description: "Google Gemini CLI"
    },
    {
        value: "agy",
        label: "Antigravity",
        description: "Antigravity CLI (agy)"
    },
    {
        value: "bash",
        label: "Terminal",
        description: "Interactive shell session"
    }
];
const DEFAULT_PROGRAM = "claude";
/** Known Claude model IDs, newest first. */ const CLAUDE_MODELS = [
    {
        value: "claude-opus-4-8",
        label: "Claude Opus 4.8 (latest)"
    },
    {
        value: "claude-sonnet-4-6",
        label: "Claude Sonnet 4.6"
    },
    {
        value: "claude-haiku-4-5-20251001",
        label: "Claude Haiku 4.5"
    },
    {
        value: "claude-opus-4-5",
        label: "Claude Opus 4.5"
    },
    {
        value: "claude-sonnet-4-5",
        label: "Claude Sonnet 4.5"
    },
    {
        value: "claude-haiku-4-5",
        label: "Claude Haiku 4.5 (base)"
    },
    {
        value: "claude-opus-4",
        label: "Claude Opus 4"
    },
    {
        value: "claude-sonnet-4",
        label: "Claude Sonnet 4"
    },
    {
        value: "claude-haiku-4",
        label: "Claude Haiku 4"
    }
];
/**
 * Model-family pseudo-entries. The `family:` prefix is intentionally left
 * unresolved here — resolution to a concrete model ID happens server-side at
 * workflow fire-time (server/workflows/model_families.go's ResolveModel,
 * called from scheduler.go's FireNow), not client-side, so a new Anthropic
 * release becomes "latest" without a frontend redeploy.
 */ const MODEL_FAMILIES = [
    {
        value: "family:opus",
        label: "Opus (latest)"
    },
    {
        value: "family:sonnet",
        label: "Sonnet (latest)"
    },
    {
        value: "family:haiku",
        label: "Haiku (latest)"
    }
];
/** Every model suggestion offered in the Workflow model picker: families first, then concrete IDs. */ const MODEL_AUTOCOMPLETE_OPTIONS = [
    ...MODEL_FAMILIES,
    ...CLAUDE_MODELS
];
function getProgramDisplay(program) {
    if (!program) return "Claude Code (default)";
    const option = PROGRAMS.find((p)=>p.value === program);
    if (option) return option.label;
    if (program.startsWith("aider --model")) return program;
    return program;
}
function isKnownProgram(program) {
    return PROGRAMS.some((p)=>p.value === program) || program.startsWith("aider --model");
}
/**
 * Programs to offer in the creation-panel picker, filtered from whichever program
 * list the caller has (usually `PROGRAMS`, but the creation panel derives its own
 * list via useAvailablePrograms() to include extra programs detected on the host).
 * `PROGRAMS` itself stays unconditional (getProgramDisplay/isKnownProgram must
 * recognize "pi" even with the flag off, for sessions that already have
 * program: "pi" set from before pi-support was disabled) -- this helper filters
 * only the rendered option list, per pi-support's "opt-in invisibility" requirement.
 */ function getPickerPrograms(programs, piSupportEnabled) {
    if (piSupportEnabled) return programs;
    return programs.filter((p)=>p.value !== "pi");
}


/***/ }),

/***/ 58203:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 69797));


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

/***/ 69797:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  "default": () => (/* binding */ FeaturesPage)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/contexts/FeatureFlagsContext.tsx
var FeatureFlagsContext = __webpack_require__(63367);
// EXTERNAL MODULE: ./src/lib/contexts/featureFlagScopes.ts
var featureFlagScopes = __webpack_require__(80070);
// EXTERNAL MODULE: ./src/lib/analytics/index.ts
var analytics = __webpack_require__(10671);
// EXTERNAL MODULE: ./src/styles/theme.css.ts
var theme_css = __webpack_require__(90377);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ./src/gen/session/v1/session_pb.ts
var session_pb = __webpack_require__(23575);
// EXTERNAL MODULE: ./src/lib/api/transport.ts
var api_transport = __webpack_require__(21694);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsettings%2FStreamHubRolloutPanel.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA7VW25KiMBB99yvyuD5AERxF48dMBdJiZmLCJmG8bO2_bxEQw00dp_Y1lfS59OmG8OPEsMUR-jND6EB1zmVgVUFQrOGwnSFUUMa4zLuHqdIMdH2GixMySnCGvqj-FQTveAOXWF9gvp39nYV1fezqZ0ooTXr3onlVcaekDQy_AEE4xPGyAXLHR-D53hKURNH2xjJV1qoDQVFYX26x4m9gReFmkYyCrXywipTjhKIh3mIaDw_x1jdxgksI9g0eDt9GxOEu1Ns0VDyESp4Aci2MwpbTI3OXjgHjphD0TNBOwKl6RgXPZcAtHAxBGUgLujr-KI3lu3OQKWlBWoJMQTMIUrBHAFndyGlxVdlmrYJdNXZ7aWs9eSZwq6dC0Gt2t1FJT3rSlc6lszUVKvvskccN-XDZnRdNGS9Nx-_xhg3JtSzWjkVKs89cq1KyvrrSqWuEH_fcgvd48-Ax-I-HUW7r0DuRr4M4jYHroagtme4mjuYjznXvLJJ5z3nnYJun0X4-mrHUaRPc2MDYswCCpJLg74KoC-q9zb41HS77Xtc9HT8LP3uFRs-dZjH0hwBc6aoiQXiU9E3PM717FAN4kKZkOrEjy34w0zsnZ6hh_ZKIa1Cm6R6m6f6--60YW1dZqU1Vp1D82ks38IHbshWdo6aFr5YwbmgqoA6IKmjG7dnJ9utJZQMqhDoC8x7n_i-CgJ0liJZW9TJw3Xyr-P-mwGoqTUE1SHt3ZU1t2IF3N52vmPQPH9boBEcJAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsettings_2FStreamHubRolloutPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA7VW25KiMBB99yvyuD5AERxF48dMBdJiZmLCJmG8bO2_bxEQw00dp_Y1lfS59OmG8OPEsMUR_jND6EB1zmVgVUFQrOGwnSFUUMa4zLuHqdIMdH2GixMySnCGvqj_FQTveAOXWF9gvp39nYV1fezqZ0ooTXr3onlVcaekDQy_AEE4xPGyAXLHR_D53hKURNH2xjJV1qoDQVFYX26x4m9gReFmkYyCrXywipTjhKIh3mIaDw_x1jdxgksI9g0eDt9GxOEu1Ns0VDyESp4Aci2MwpbTI3OXjgHjphD0TNBOwKl6RgXPZcAtHAxBGUgLujr_KI3lu3OQKWlBWoJMQTMIUrBHAFndyGlxVdlmrYJdNXZ7aWs9eSZwq6dC0Gt2t1FJT3rSlc6lszUVKvvskccN_XDZnRdNGS9Nx_xhg3JtSzWjkVKs89cq1KyvrrSqWuEH_fcgvd48_Ax_I_HUW7r0DuRr4M4jYHroagtme4mjuYjznXvLJJ5z3nnYJun0X4_mrHUaRPc2MDYswCCpJLg74KoC_q9zb41HS77Xtc9HT8LP3uFRs_dZjH0hwBc6aoiQXiU9E3PM717FAN4kKZkOrEjy34w0zsnZ6hh_ZKIa1Cm6R6m6f6_60YW1dZqU1Vp1D82ks38IHbshWdo6aFr5YwbmgqoA6IKmjG7dnJ9utJZQMqhDoC8x7n_i_CgJ0liJZW9TJw3Xyr_P_mwGoqTUE1SHt3ZU1t2IF3N52vmPQPH9boBEcJAAA_22_7D = __webpack_require__(4305);
;// ./src/components/settings/StreamHubRolloutPanel.css.ts


var actionButton = 'jxd1t1f';
var addRow = 'jxd1t1d';
var badge = 'jxd1t17';
var badgeDisabled = 'jxd1t19';
var badgeEnabled = 'jxd1t18';
var description = 'jxd1t13';
var errorMessage = 'jxd1t1a';
var heading = 'jxd1t11';
var hint = 'jxd1t14';
var input = 'jxd1t1e';
var overrideList = 'jxd1t1b';
var overrideRow = 'jxd1t1c';
var panel = 'jxd1t10';
var removeButton = 'jxd1t1g';
var statusLabel = 'jxd1t16';
var statusRow = 'jxd1t15';
var subheading = 'jxd1t12';
;// ./src/components/settings/RolloutStatusSection.tsx
/* __next_internal_client_entry_do_not_use__ RolloutStatusSection,RolloutOverrideList auto */ 

/**
 * Shared status/controls block for the stream-hub and tymux rollout panels:
 * error banner, global-override toggle, rollback-rehearsal status, and the
 * per-session override list. Both panels are shaped identically here — they
 * differ only in wording, testid prefix, and which RPC client backs the
 * callbacks — so this is the deduplicated common body; each panel keeps its
 * own heading/description and the add-override input, which do differ.
 */ function RolloutStatusSection(param) {
    let { testIdPrefix, subjectLabel, notSetLabel, error, globalOverride, onSetGlobalOverride, busy, rehearsalCompletedAt, onCompleteRehearsal, overrides, onRemoveOverride } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            error && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: errorMessage,
                role: "alert",
                children: error
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: statusRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: statusLabel,
                        children: "Global override"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: "".concat(badge, " ").concat(globalOverride === true ? badgeEnabled : badgeDisabled),
                        children: globalOverride === undefined ? notSetLabel : globalOverride ? "Forced on" : "Forced off"
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: addRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: actionButton,
                        disabled: busy || globalOverride === true,
                        onClick: ()=>onSetGlobalOverride(true),
                        "data-testid": "".concat(testIdPrefix, "-global-override-on"),
                        "aria-label": "Force ".concat(subjectLabel, " on for all sessions"),
                        children: "Force on for everything"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: actionButton,
                        disabled: busy || globalOverride === false,
                        onClick: ()=>onSetGlobalOverride(false),
                        "data-testid": "".concat(testIdPrefix, "-global-override-off"),
                        "aria-label": "Force ".concat(subjectLabel, " off for all sessions"),
                        children: "Force off for everything"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: removeButton,
                        disabled: busy || globalOverride === undefined,
                        onClick: ()=>onSetGlobalOverride(undefined),
                        "data-testid": "".concat(testIdPrefix, "-global-override-clear"),
                        "aria-label": "Clear global override, revert to the default",
                        children: "Clear override"
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: statusRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: statusLabel,
                        children: "Rollback rehearsal"
                    }),
                    rehearsalCompletedAt ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: "".concat(badge, " ").concat(badgeEnabled),
                        children: [
                            "Completed ",
                            rehearsalCompletedAt.toLocaleString()
                        ]
                    }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: actionButton,
                        "data-testid": "".concat(testIdPrefix, "-complete-rehearsal"),
                        disabled: busy,
                        onClick: onCompleteRehearsal,
                        children: "Mark rehearsal complete"
                    })
                ]
            })
        ]
    });
}
/** Per-session canary override list shared by both rollout panels. */ function RolloutOverrideList(param) {
    let { testIdPrefix, overrides, busy, onRemoveOverride } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                className: subheading,
                children: "Per-session canary overrides"
            }),
            overrides.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: description,
                children: "No sessions are currently overridden."
            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                className: overrideList,
                children: overrides.map((o)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                        className: overrideRow,
                        "data-testid": "".concat(testIdPrefix, "-override-row"),
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: statusLabel,
                                children: o.sessionName
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: "".concat(badge, " ").concat(o.forced ? badgeEnabled : badgeDisabled),
                                children: o.forced ? "Forced on" : "Forced off"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: removeButton,
                                "data-testid": "".concat(testIdPrefix, "-remove-override"),
                                disabled: busy,
                                onClick: ()=>onRemoveOverride(o.sessionName),
                                children: "Remove"
                            })
                        ]
                    }, o.sessionName))
            })
        ]
    });
}

;// ./src/components/settings/StreamHubRolloutPanel.tsx
/* __next_internal_client_entry_do_not_use__ StreamHubRolloutPanel auto */ 
// +feature: stream-hub-rollout







/**
 * Controls for the terminal-multi-connection-streaming hub. Everything here
 * takes effect immediately, config.json-backed, for session connections
 * resolved from that point on — an already-connected session's resolution
 * is cached for its lifetime and can't be moved: per-session canary
 * overrides and a live global override of the "stream_hub" feature flag
 * (on by default), no process restart required.
 */ function StreamHubRolloutPanel() {
    const { track } = (0,analytics/* useAnalytics */.st)();
    const client = (0,react.useMemo)(()=>(0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,api_transport/* getConnectTransport */.qW)()), []);
    const [loading, setLoading] = (0,react.useState)(true);
    const [error, setError] = (0,react.useState)(null);
    const [globalOverride, setGlobalOverride] = (0,react.useState)(undefined);
    const [rehearsalCompletedAt, setRehearsalCompletedAt] = (0,react.useState)(null);
    const [overrides, setOverrides] = (0,react.useState)([]);
    const [sessionNames, setSessionNames] = (0,react.useState)([]);
    const [newOverrideName, setNewOverrideName] = (0,react.useState)("");
    const [busy, setBusy] = (0,react.useState)(false);
    const applyStatus = (0,react.useCallback)((status)=>{
        const ts = status.rollbackRehearsalCompletedAt;
        setRehearsalCompletedAt(ts ? new Date(Number(ts.seconds) * 1000) : null);
        setOverrides(status.sessionOverrides);
        setGlobalOverride(status.globalOverride);
    }, []);
    const load = (0,react.useCallback)(async ()=>{
        try {
            const status = await client.getStreamHubRolloutStatus({});
            applyStatus(status);
        } catch (e) {
            setError("Failed to load stream-hub rollout status");
        } finally{
            setLoading(false);
        }
    }, [
        client,
        applyStatus
    ]);
    (0,react.useEffect)(()=>{
        void load();
        client.listSessions({}).then((res)=>{
            setSessionNames(res.sessions.map((s)=>s.title).filter(Boolean));
        }).catch(()=>{
        // Non-fatal — the override input still works as free text.
        });
    }, [
        load,
        client
    ]);
    const completeRehearsal = (0,react.useCallback)(async ()=>{
        track({
            name: "stream_hub_rehearsal_completed",
            category: "user_action",
            component: "StreamHubRolloutPanel"
        });
        setBusy(true);
        setError(null);
        try {
            const status = await client.completeStreamHubRollbackRehearsal({});
            applyStatus(status);
        } catch (e) {
            setError("Failed to record rollback rehearsal");
        } finally{
            setBusy(false);
        }
    }, [
        client,
        applyStatus,
        track
    ]);
    const addOverride = (0,react.useCallback)(async ()=>{
        const sessionName = newOverrideName.trim();
        if (!sessionName) return;
        track({
            name: "stream_hub_override_added",
            category: "user_action",
            component: "StreamHubRolloutPanel"
        });
        setBusy(true);
        setError(null);
        try {
            const status = await client.setStreamHubSessionOverride({
                sessionName,
                forceHub: true
            });
            applyStatus(status);
            setNewOverrideName("");
        } catch (e) {
            setError("Failed to set session override");
        } finally{
            setBusy(false);
        }
    }, [
        client,
        applyStatus,
        newOverrideName,
        track
    ]);
    const setGlobalOverrideValue = (0,react.useCallback)(async (forceHub)=>{
        track({
            name: "stream_hub_global_override_changed",
            category: "user_action",
            component: "StreamHubRolloutPanel",
            labels: {
                forceHub: forceHub === undefined ? "clear" : String(forceHub)
            }
        });
        setBusy(true);
        setError(null);
        try {
            const status = await client.setStreamHubGlobalOverride({
                forceHub
            });
            applyStatus(status);
        } catch (e) {
            setError(forceHub === true ? "Failed to enable the global override — the rollback rehearsal may not be completed yet" : "Failed to update the global override");
        } finally{
            setBusy(false);
        }
    }, [
        client,
        applyStatus,
        track
    ]);
    const removeOverride = (0,react.useCallback)(async (sessionName)=>{
        track({
            name: "stream_hub_override_removed",
            category: "user_action",
            component: "StreamHubRolloutPanel"
        });
        setBusy(true);
        setError(null);
        try {
            const status = await client.setStreamHubSessionOverride({
                sessionName
            });
            applyStatus(status);
        } catch (e) {
            setError("Failed to clear session override");
        } finally{
            setBusy(false);
        }
    }, [
        client,
        applyStatus,
        track
    ]);
    if (loading) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
            className: description,
            children: "Loading…"
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
        className: panel,
        "data-testid": "stream-hub-rollout-panel",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                className: heading,
                children: "Stream Hub Rollout"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: description,
                children: 'Staged rollout for the terminal-multi-connection-streaming hub. The "stream_hub" feature flag defaults to on; the override below takes effect immediately for any session connection resolved from this point on (an already-connected session can\'t be moved) — no process restart required.'
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(RolloutStatusSection, {
                testIdPrefix: "stream-hub",
                subjectLabel: "stream hub",
                notSetLabel: "Not set (default: on)",
                error: error,
                globalOverride: globalOverride,
                onSetGlobalOverride: setGlobalOverrideValue,
                busy: busy,
                rehearsalCompletedAt: rehearsalCompletedAt,
                onCompleteRehearsal: completeRehearsal,
                overrides: overrides.map((o)=>({
                        sessionName: o.sessionName,
                        forced: o.forceHub
                    })),
                onRemoveOverride: removeOverride
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: hint,
                children: "Only mark this after manually verifying: flip a per-session override on, use it briefly, remove it, confirm a clean reconnect under the legacy path."
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(RolloutOverrideList, {
                testIdPrefix: "stream-hub",
                overrides: overrides.map((o)=>({
                        sessionName: o.sessionName,
                        forced: o.forceHub
                    })),
                busy: busy,
                onRemoveOverride: removeOverride
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: addRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        className: input,
                        list: "stream-hub-session-names",
                        placeholder: "Session name",
                        value: newOverrideName,
                        onChange: (e)=>setNewOverrideName(e.target.value),
                        "data-testid": "stream-hub-override-input"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("datalist", {
                        id: "stream-hub-session-names",
                        children: sessionNames.map((name)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                value: name
                            }, name))
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: actionButton,
                        disabled: busy || !newOverrideName.trim(),
                        onClick: addOverride,
                        "data-testid": "stream-hub-add-override",
                        children: "Force hub on for session"
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/connect-error.js
var connect_error = __webpack_require__(45640);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/code.js
var code = __webpack_require__(16294);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/codegenv2/file.js
var file = __webpack_require__(1844);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/codegenv2/service.js
var service = __webpack_require__(95765);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/gen/google/protobuf/timestamp_pb.js
var timestamp_pb = __webpack_require__(43963);
;// ./src/gen/session/v1/tymux_rollout_pb.ts
// @generated by protoc-gen-es v2.11.0 with parameter "target=ts,ts_nocheck=false,keep_empty_files=true"
// @generated from file session/v1/tymux_rollout.proto (package session.v1, syntax proto3)
/* eslint-disable */ 

/**
 * Describes the file session/v1/tymux_rollout.proto.
 */ const file_session_v1_tymux_rollout = /*@__PURE__*/ (0,file/* fileDesc */.w)("Ch5zZXNzaW9uL3YxL3R5bXV4X3JvbGxvdXQucHJvdG8SCnNlc3Npb24udjEiHgocR2V0VHltdXhSb2xsb3V0U3RhdHVzUmVxdWVzdCJGChlUeW11eFNlc3Npb25PdmVycmlkZUVudHJ5EhQKDHNlc3Npb25fbmFtZRgBIAEoCRITCgtmb3JjZV90eW11eBgCIAEoCCKYAgoSVHltdXhSb2xsb3V0U3RhdHVzEhoKEmdsb2JhbF9lbnZfdmFyX3NldBgBIAEoCBJDCh9yb2xsYmFja19yZWhlYXJzYWxfY29tcGxldGVkX2F0GAIgASgLMhouZ29vZ2xlLnByb3RvYnVmLlRpbWVzdGFtcBJAChFzZXNzaW9uX292ZXJyaWRlcxgDIAMoCzIlLnNlc3Npb24udjEuVHltdXhTZXNzaW9uT3ZlcnJpZGVFbnRyeRIcCg9nbG9iYWxfb3ZlcnJpZGUYBCABKAhIAIgBARItCgZkYWVtb24YBSABKAsyHS5zZXNzaW9uLnYxLlR5bXV4ZFZlcnNpb25JbmZvQhIKEF9nbG9iYWxfb3ZlcnJpZGUinAEKEVR5bXV4ZFZlcnNpb25JbmZvEg8KB3J1bm5pbmcYASABKAgSCwoDcGlkGAIgASgFEhcKD3J1bm5pbmdfdmVyc2lvbhgDIAEoCRIZChFpbnN0YWxsZWRfdmVyc2lvbhgEIAEoCRI1CglkaXJlY3Rpb24YBSABKA4yIi5zZXNzaW9uLnYxLlR5bXV4ZFZlcnNpb25EaXJlY3Rpb24iPQoUUmVwbGFjZVR5bXV4ZFJlcXVlc3QSJQodYWxsb3dfaW50ZXJydXB0X2xpdmVfc2Vzc2lvbnMYASABKAgiJwolQ29tcGxldGVUeW11eFJvbGxiYWNrUmVoZWFyc2FsUmVxdWVzdCJgCh5TZXRUeW11eFNlc3Npb25PdmVycmlkZVJlcXVlc3QSFAoMc2Vzc2lvbl9uYW1lGAEgASgJEhgKC2ZvcmNlX3R5bXV4GAIgASgISACIAQFCDgoMX2ZvcmNlX3R5bXV4IkkKHVNldFR5bXV4R2xvYmFsT3ZlcnJpZGVSZXF1ZXN0EhgKC2ZvcmNlX3R5bXV4GAEgASgISACIAQFCDgoMX2ZvcmNlX3R5bXV4KrMBChZUeW11eGRWZXJzaW9uRGlyZWN0aW9uEigKJFRZTVVYRF9WRVJTSU9OX0RJUkVDVElPTl9VTlNQRUNJRklFRBAAEiEKHVRZTVVYRF9WRVJTSU9OX0RJUkVDVElPTl9TQU1FEAESJAogVFlNVVhEX1ZFUlNJT05fRElSRUNUSU9OX1VQR1JBREUQAhImCiJUWU1VWERfVkVSU0lPTl9ESVJFQ1RJT05fRE9XTkdSQURFEAMylgQKE1R5bXV4Um9sbG91dFNlcnZpY2USYwoVR2V0VHltdXhSb2xsb3V0U3RhdHVzEiguc2Vzc2lvbi52MS5HZXRUeW11eFJvbGxvdXRTdGF0dXNSZXF1ZXN0Gh4uc2Vzc2lvbi52MS5UeW11eFJvbGxvdXRTdGF0dXMiABJ1Ch5Db21wbGV0ZVR5bXV4Um9sbGJhY2tSZWhlYXJzYWwSMS5zZXNzaW9uLnYxLkNvbXBsZXRlVHltdXhSb2xsYmFja1JlaGVhcnNhbFJlcXVlc3QaHi5zZXNzaW9uLnYxLlR5bXV4Um9sbG91dFN0YXR1cyIAEmcKF1NldFR5bXV4U2Vzc2lvbk92ZXJyaWRlEiouc2Vzc2lvbi52MS5TZXRUeW11eFNlc3Npb25PdmVycmlkZVJlcXVlc3QaHi5zZXNzaW9uLnYxLlR5bXV4Um9sbG91dFN0YXR1cyIAEmUKFlNldFR5bXV4R2xvYmFsT3ZlcnJpZGUSKS5zZXNzaW9uLnYxLlNldFR5bXV4R2xvYmFsT3ZlcnJpZGVSZXF1ZXN0Gh4uc2Vzc2lvbi52MS5UeW11eFJvbGxvdXRTdGF0dXMiABJTCg1SZXBsYWNlVHltdXhkEiAuc2Vzc2lvbi52MS5SZXBsYWNlVHltdXhkUmVxdWVzdBoeLnNlc3Npb24udjEuVHltdXhSb2xsb3V0U3RhdHVzIgBCsQEKDmNvbS5zZXNzaW9uLnYxQhFUeW11eFJvbGxvdXRQcm90b1ABWkNnaXRodWIuY29tL3RzdGFwbGVyL3N0YXBsZXItc3F1YWQvZ2VuL3Byb3RvL2dvL3Nlc3Npb24vdjE7c2Vzc2lvbnYxogIDU1hYqgIKU2Vzc2lvbi5WMcoCClNlc3Npb25cVjHiAhZTZXNzaW9uXFYxXEdQQk1ldGFkYXRh6gILU2Vzc2lvbjo6VjFiBnByb3RvMw", [
    timestamp_pb/* file_google_protobuf_timestamp */.i
]);
/**
 * Describes the message session.v1.GetTymuxRolloutStatusRequest.
 * Use `create(GetTymuxRolloutStatusRequestSchema)` to create a new message.
 */ const GetTymuxRolloutStatusRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_tymux_rollout, 0)));
/**
 * Describes the message session.v1.TymuxSessionOverrideEntry.
 * Use `create(TymuxSessionOverrideEntrySchema)` to create a new message.
 */ const TymuxSessionOverrideEntrySchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_tymux_rollout, 1)));
/**
 * Describes the message session.v1.TymuxRolloutStatus.
 * Use `create(TymuxRolloutStatusSchema)` to create a new message.
 */ const TymuxRolloutStatusSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_tymux_rollout, 2)));
/**
 * Describes the message session.v1.TymuxdVersionInfo.
 * Use `create(TymuxdVersionInfoSchema)` to create a new message.
 */ const TymuxdVersionInfoSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_tymux_rollout, 3)));
/**
 * Describes the message session.v1.ReplaceTymuxdRequest.
 * Use `create(ReplaceTymuxdRequestSchema)` to create a new message.
 */ const ReplaceTymuxdRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_tymux_rollout, 4)));
/**
 * Describes the message session.v1.CompleteTymuxRollbackRehearsalRequest.
 * Use `create(CompleteTymuxRollbackRehearsalRequestSchema)` to create a new message.
 */ const CompleteTymuxRollbackRehearsalRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_tymux_rollout, 5)));
/**
 * Describes the message session.v1.SetTymuxSessionOverrideRequest.
 * Use `create(SetTymuxSessionOverrideRequestSchema)` to create a new message.
 */ const SetTymuxSessionOverrideRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_tymux_rollout, 6)));
/**
 * Describes the message session.v1.SetTymuxGlobalOverrideRequest.
 * Use `create(SetTymuxGlobalOverrideRequestSchema)` to create a new message.
 */ const SetTymuxGlobalOverrideRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_tymux_rollout, 7)));
/**
 * @generated from enum session.v1.TymuxdVersionDirection
 */ var TymuxdVersionDirection = /*#__PURE__*/ function(TymuxdVersionDirection) {
    /**
   * a version could not be determined
   *
   * @generated from enum value: TYMUXD_VERSION_DIRECTION_UNSPECIFIED = 0;
   */ TymuxdVersionDirection[TymuxdVersionDirection["UNSPECIFIED"] = 0] = "UNSPECIFIED";
    /**
   * @generated from enum value: TYMUXD_VERSION_DIRECTION_SAME = 1;
   */ TymuxdVersionDirection[TymuxdVersionDirection["SAME"] = 1] = "SAME";
    /**
   * the configured binary is newer than the running daemon
   *
   * @generated from enum value: TYMUXD_VERSION_DIRECTION_UPGRADE = 2;
   */ TymuxdVersionDirection[TymuxdVersionDirection["UPGRADE"] = 2] = "UPGRADE";
    /**
   * the configured binary is older than the running daemon
   *
   * @generated from enum value: TYMUXD_VERSION_DIRECTION_DOWNGRADE = 3;
   */ TymuxdVersionDirection[TymuxdVersionDirection["DOWNGRADE"] = 3] = "DOWNGRADE";
    return TymuxdVersionDirection;
}({});
/**
 * Describes the enum session.v1.TymuxdVersionDirection.
 */ const TymuxdVersionDirectionSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_session_v1_tymux_rollout, 0)));
/**
 * TymuxRolloutService is the operator-facing RPC surface for the
 * tymux-bundled-integration rollout, mirroring StreamHubRolloutService's
 * shape (proto/session/v1/session.proto) exactly: a live, config.json-backed
 * "tymux" feature flag for the global default (config.EffectiveTymuxEnabled),
 * no env var, no rollback-rehearsal gate — banked as a historical record
 * only — plus per-session canary overrides.
 *
 * @generated from service session.v1.TymuxRolloutService
 */ const TymuxRolloutService = /*@__PURE__*/ (0,service/* serviceDesc */.Z)(file_session_v1_tymux_rollout, 0);

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/timestamp.js
var timestamp = __webpack_require__(28765);
;// ./src/components/settings/TymuxRolloutPanel.tsx
/* __next_internal_client_entry_do_not_use__ TymuxRolloutPanel auto */ 
// +feature: tymux-rollout







// ponytail: reuses StreamHubRolloutPanel's stylesheet — the class names
// (panel, heading, statusRow, ...) are generic, not stream-hub-specific, and
// this panel's markup is deliberately shaped the same way.


// Mirrors session/tmux/tmux.go's toStaplerSquadTmuxNameWithPrefix exactly.
// TymuxSessionOverrides is keyed by the *sanitized* tmux session name, not
// the raw title (see ResolveSessionBackend's doc comment and #162) — typing
// a title with spaces/dots/colons here and sending it unsanitized would
// silently never match at CreateSession time.
function toStaplerSquadSessionName(title) {
    return "staplersquad_".concat(title.replace(/\s+/g, "").replace(/[.:]/g, "_"));
}
/**
 * Controls for the tymux-bundled-integration rollout of BackendTymux.
 * Everything here takes effect immediately, config.json-backed: the
 * "tymux" feature flag (off by default) via a live global override, and
 * per-session canary overrides — no process restart required. Mirrors
 * StreamHubRolloutPanel's shape exactly.
 */ function TymuxRolloutPanel() {
    const { track } = (0,analytics/* useAnalytics */.st)();
    const transport = (0,react.useMemo)(()=>(0,api_transport/* getConnectTransport */.qW)(), []);
    const client = (0,react.useMemo)(()=>(0,promise_client/* createClient */.UU)(TymuxRolloutService, transport), [
        transport
    ]);
    const sessionClient = (0,react.useMemo)(()=>(0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, transport), [
        transport
    ]);
    const [loading, setLoading] = (0,react.useState)(true);
    const [error, setError] = (0,react.useState)(null);
    const [globalOverride, setGlobalOverride] = (0,react.useState)(undefined);
    const [rehearsalCompletedAt, setRehearsalCompletedAt] = (0,react.useState)(null);
    const [overrides, setOverrides] = (0,react.useState)([]);
    const [sessionNames, setSessionNames] = (0,react.useState)([]);
    const [newOverrideName, setNewOverrideName] = (0,react.useState)("");
    const [busy, setBusy] = (0,react.useState)(false);
    const [migrateTitle, setMigrateTitle] = (0,react.useState)("");
    const [migrateStatus, setMigrateStatus] = (0,react.useState)(null);
    const [daemon, setDaemon] = (0,react.useState)(undefined);
    const [replaceBlockedByLiveSessions, setReplaceBlockedByLiveSessions] = (0,react.useState)(false);
    const applyStatus = (0,react.useCallback)((status)=>{
        const ts = status.rollbackRehearsalCompletedAt;
        setRehearsalCompletedAt(ts ? (0,timestamp/* timestampDate */.FP)(ts) : null);
        setOverrides(status.sessionOverrides);
        setGlobalOverride(status.globalOverride);
        // Only Get/Replace carry daemon info; keep the last known value otherwise.
        if (status.daemon) setDaemon(status.daemon);
    }, []);
    const load = (0,react.useCallback)(async ()=>{
        try {
            const status = await client.getTymuxRolloutStatus({});
            applyStatus(status);
        } catch (e) {
            setError("Failed to load tymux rollout status");
        } finally{
            setLoading(false);
        }
    }, [
        client,
        applyStatus
    ]);
    (0,react.useEffect)(()=>{
        let cancelled = false;
        void load();
        sessionClient.listSessions({}).then((res)=>{
            if (cancelled) return;
            setSessionNames(res.sessions.map((s)=>s.title).filter(Boolean));
        }).catch(()=>{
        // Non-fatal — the override input still works as free text.
        });
        return ()=>{
            cancelled = true;
        };
    }, [
        load,
        sessionClient
    ]);
    const completeRehearsal = (0,react.useCallback)(async ()=>{
        track({
            name: "tymux_rehearsal_completed",
            category: "user_action",
            component: "TymuxRolloutPanel"
        });
        setBusy(true);
        setError(null);
        try {
            const status = await client.completeTymuxRollbackRehearsal({});
            applyStatus(status);
        } catch (e) {
            setError("Failed to record rollback rehearsal");
        } finally{
            setBusy(false);
        }
    }, [
        client,
        applyStatus,
        track
    ]);
    const addOverride = (0,react.useCallback)(async ()=>{
        const title = newOverrideName.trim();
        if (!title) return;
        const sessionName = toStaplerSquadSessionName(title);
        track({
            name: "tymux_override_added",
            category: "user_action",
            component: "TymuxRolloutPanel"
        });
        setBusy(true);
        setError(null);
        try {
            const status = await client.setTymuxSessionOverride({
                sessionName,
                forceTymux: true
            });
            applyStatus(status);
            setNewOverrideName("");
        } catch (e) {
            setError("Failed to set session override");
        } finally{
            setBusy(false);
        }
    }, [
        client,
        applyStatus,
        newOverrideName,
        track
    ]);
    const setGlobalOverrideValue = (0,react.useCallback)(async (forceTymux)=>{
        track({
            name: "tymux_global_override_changed",
            category: "user_action",
            component: "TymuxRolloutPanel",
            labels: {
                forceTymux: forceTymux === undefined ? "clear" : String(forceTymux)
            }
        });
        setBusy(true);
        setError(null);
        try {
            const status = await client.setTymuxGlobalOverride({
                forceTymux
            });
            applyStatus(status);
        } catch (e) {
            setError("Failed to update the global override");
        } finally{
            setBusy(false);
        }
    }, [
        client,
        applyStatus,
        track
    ]);
    // kibitzer:ignore replace-magic-literal -- this is a TS union type literal (the two valid
    // ProcessManagerBackend values), not a repeated data literal; the other flagged occurrences
    // (testIdPrefix/subjectLabel props below) are unrelated pre-existing JSX attributes.
    const migrateSessionBackend = (0,react.useCallback)(async (backendOverride)=>{
        const id = migrateTitle.trim();
        if (!id) return;
        track({
            name: "tymux_session_migrated",
            category: "user_action",
            component: "TymuxRolloutPanel",
            labels: {
                backendOverride
            }
        });
        setBusy(true);
        setError(null);
        setMigrateStatus(null);
        try {
            await sessionClient.restartSession({
                id,
                preserveOutput: true,
                backendOverride
            });
            setMigrateStatus('"'.concat(id, '" restarted on ').concat(backendOverride, "."));
        } catch (e) {
            setError('Failed to migrate "'.concat(id, '" to ').concat(backendOverride));
        } finally{
            setBusy(false);
        }
    }, [
        sessionClient,
        migrateTitle,
        track
    ]);
    const replaceDaemon = (0,react.useCallback)(async (allowInterruptLiveSessions)=>{
        track({
            name: "tymuxd_replace",
            category: "user_action",
            component: "TymuxRolloutPanel",
            labels: {
                allowInterruptLiveSessions: String(allowInterruptLiveSessions)
            }
        });
        setBusy(true);
        setError(null);
        setReplaceBlockedByLiveSessions(false);
        try {
            applyStatus(await client.replaceTymuxd({
                allowInterruptLiveSessions
            }));
        } catch (e) {
            if (connect_error/* ConnectError */.T.from(e).code === code/* Code */.C.FailedPrecondition) {
                setReplaceBlockedByLiveSessions(true);
            } else {
                setError("Failed to replace tymuxd");
            }
        } finally{
            setBusy(false);
        }
    }, [
        client,
        applyStatus,
        track
    ]);
    const removeOverride = (0,react.useCallback)(async (sessionName)=>{
        track({
            name: "tymux_override_removed",
            category: "user_action",
            component: "TymuxRolloutPanel"
        });
        setBusy(true);
        setError(null);
        try {
            const status = await client.setTymuxSessionOverride({
                sessionName
            });
            applyStatus(status);
        } catch (e) {
            setError("Failed to clear session override");
        } finally{
            setBusy(false);
        }
    }, [
        client,
        applyStatus,
        track
    ]);
    if (loading) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
            className: description,
            children: "Loading…"
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
        className: panel,
        "data-testid": "tymux-rollout-panel",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                className: heading,
                children: "Tymux Backend Rollout"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                className: description,
                children: [
                    "Staged rollout for the tymux-backed ",
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                        children: "ProcessManager"
                    }),
                    '. The "tymux" feature flag defaults to off; the override below takes effect immediately for any session created from this point on — no restart required.'
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(RolloutStatusSection, {
                testIdPrefix: "tymux",
                subjectLabel: "tymux",
                notSetLabel: "Not set (default: off)",
                error: error,
                globalOverride: globalOverride,
                onSetGlobalOverride: setGlobalOverrideValue,
                busy: busy,
                rehearsalCompletedAt: rehearsalCompletedAt,
                onCompleteRehearsal: completeRehearsal,
                overrides: overrides.map((o)=>({
                        sessionName: o.sessionName,
                        forced: o.forceTymux
                    })),
                onRemoveOverride: removeOverride
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: hint,
                children: "Only mark this after manually verifying: flip a per-session override on, run a real session through it, confirm a clean disconnect/reconnect."
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(RolloutOverrideList, {
                testIdPrefix: "tymux",
                overrides: overrides.map((o)=>({
                        sessionName: o.sessionName,
                        forced: o.forceTymux
                    })),
                busy: busy,
                onRemoveOverride: removeOverride
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: addRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        className: input,
                        list: "tymux-session-names",
                        placeholder: "Session title (existing or one you're about to create)",
                        "aria-label": "Session title",
                        value: newOverrideName,
                        onChange: (e)=>setNewOverrideName(e.target.value),
                        "data-testid": "tymux-override-input"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("datalist", {
                        id: "tymux-session-names",
                        children: sessionNames.map((name)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                value: name
                            }, name))
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: actionButton,
                        disabled: busy || !newOverrideName.trim(),
                        onClick: addOverride,
                        "data-testid": "tymux-add-override",
                        children: "Force tymux on for session"
                    })
                ]
            }),
            newOverrideName.trim() && /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                className: hint,
                children: [
                    "Stored as ",
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                        children: toStaplerSquadSessionName(newOverrideName.trim())
                    }),
                    " — the sanitized tmux session name a session with this title resolves to."
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                className: heading,
                children: "Migrate a live session"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: description,
                children: "Restarts an existing session under the target backend right now — kills the current process, rebuilds it on tmux or tymux, and resumes. There is no live hand-off between backends (they're different daemons), so this is a restart, not an in-place swap."
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: addRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        className: input,
                        list: "tymux-session-names",
                        placeholder: "Session title to migrate",
                        "aria-label": "Session title to migrate",
                        value: migrateTitle,
                        onChange: (e)=>setMigrateTitle(e.target.value),
                        "data-testid": "tymux-migrate-input"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: actionButton,
                        disabled: busy || !migrateTitle.trim(),
                        onClick: ()=>migrateSessionBackend("tymux"),
                        "data-testid": "tymux-migrate-to-tymux",
                        children: "Restart on tymux"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: actionButton,
                        disabled: busy || !migrateTitle.trim(),
                        onClick: ()=>migrateSessionBackend("tmux"),
                        "data-testid": "tymux-migrate-to-tmux",
                        children: "Restart on tmux"
                    })
                ]
            }),
            migrateStatus && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: hint,
                children: migrateStatus
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                className: heading,
                children: "tymuxd version"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                "data-testid": "tymuxd-version",
                children: (daemon === null || daemon === void 0 ? void 0 : daemon.running) ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                    className: description,
                    children: [
                        "Running ",
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                            children: daemon.runningVersion || "unknown"
                        }),
                        " (pid ",
                        daemon.pid,
                        "); configured binary",
                        " ",
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                            children: daemon.installedVersion || "unknown"
                        }),
                        daemon.direction === TymuxdVersionDirection.SAME && " — up to date.",
                        daemon.direction === TymuxdVersionDirection.UPGRADE && " — an upgrade is available.",
                        daemon.direction === TymuxdVersionDirection.DOWNGRADE && " — the configured binary is older (a downgrade)."
                    ]
                }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: description,
                    children: "No tymuxd is running."
                })
            }),
            (daemon === null || daemon === void 0 ? void 0 : daemon.running) && daemon.direction !== TymuxdVersionDirection.SAME && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: hint,
                        children: "Replacing restarts the daemon; each live session's program is respawned when it reconnects, interrupting any running agent."
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: actionButton,
                        disabled: busy,
                        onClick: ()=>replaceDaemon(false),
                        "data-testid": "tymuxd-replace",
                        children: "Replace with configured version"
                    })
                ]
            }),
            replaceBlockedByLiveSessions && /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                className: hint,
                role: "alert",
                children: [
                    "Live sessions would be interrupted.",
                    " ",
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: actionButton,
                        disabled: busy,
                        onClick: ()=>replaceDaemon(true),
                        "data-testid": "tymuxd-replace-anyway",
                        children: "Replace anyway"
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react-dom/index.js
var react_dom = __webpack_require__(80883);
// EXTERNAL MODULE: ./src/lib/hooks/useDialogFocusTrap.ts
var useDialogFocusTrap = __webpack_require__(82279);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2FmodalChrome.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA9VVTW-jMBC951fMpVKQ1pGhbZqQS__GnlYONuAWbNZ2UpJV__sKG1JwgGSl3cNKETH-ePPmzfPw-s5OqSIl01BvxbqkGH4tAFIlSzsAkBVJuDnFgHcLgM8FgJH-UuiWPhevPlzowxlFhE6lKmM3LIhh35cRrupgdyve-FnsH7ywWTkOkT1eSc0NlyKGlNeM7ixw1YYpWGraoeJZ3o330hhZdi8kec-UPAgaw5GoJUI_wi07R-pMLAPKdVWQUwxpwepmghQ8E4gbVuoYEiYMU83020Ebnp5QIoVhwvSXzogLyuoYQoxtTCJ4SRzrrjx4FWlgRLPdV4aPNsNpgtvAZaMoU0gRyg86hjCqLM2KUMpF1mZZkhp9cGryGJ6bqjRz7fsWP3Q7cuZU2uBjPpp7848oVyxx7BNZHErhWNRI54TKjxgwNCFg3TxUtidL_A3a3-oxGBUgbJauBHhyJe4yiZ6q2j3CdTvqCdBVNaxq0LLg1FPrLeghP1vkkqiMi4YwhgbWSpVKYZDmZxZDp5Sd-mjFWbsaJrKQyq8I7sdYD2N42GHLfhQn7OO8TBmdC83MlY3RKOK_M_OlPMOAj4McNp6Vx0n2DT1fyCvbe8E3gW9Kb0MZzHGPAu_SPG2GlyYhRbIMMX4ABJFi5bi609clI9W8YFvXYb_84m2mwZUxvR15MGvTgTMvYYkNO-nJOUaJDwpQcMEuTSVcPfcC7W2gK8XGdHFsryxpxWWCfkVFtvMPT0fnvqrJXTYsp5X7OXCokILd9uLLnNWiczDmg9tCz5U-cwkclG4yqCTvLq_9vrZtxDsjBjrFuTwydZda3sFUJgeNjlzzfcEsgDyYxggxRBMX2undbkMyTW1bs9-xCzCd4GIzqohiwsx209tthd3RVv7DUtI_KOWmL8HoDj6E_svF_g3PZClKtwoAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2FmodalChrome_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9VVTW_jMBC951fMpVKQ1pGhbZqQS_GnlYONuAWbNZ2UpJV_sKG1JwgGSl3cNKETH_ePPmzfPw_s5OqSIl01BvxbqkGH4tAFIlSzsAkBVJuDnFgHcLgM8FgJH_UuiWPhevPlzowxlFhE6lKmM3LIhh35cRrupgdyve_FnsH7ywWTkOkT1eSc0NlyKGlNeM7ixw1YYpWGraoeJZ3o330hhZdi8kec_UPAgaw5GoJUI_wi07R_pMLAPKdVWQUwxpwepmghQ8E4gbVuoYEiYMU83020Ebnp5QIoVhwvSXzogLyuoYQoxtTCJ4SRzrrjx4FWlgRLPdV4aPNsNpgtvAZaMoU0gRyg86hjCqLM2KUMpF1mZZkhp9cGryGJ6bqjRz7fsWP3Q7cuZU2uBjPpp7848oVyxx7BNZHErhWNRI54TKjxgwNCFg3TxUtidL_A3a3_oxGBUgbJauBHhyJe4yiZ6q2j3CdTvqCdBVNaxq0LLg1FPrLeghP1vkkqiMi4YwhgbWSpVKYZDmZxZDp5Sd_mjFWbsaJrKQyq8I7sdYD2N42GHLfhQn7OO8TBmdC83MlY3RKOK_M_OlPMOAj4McNp6Vx0n2DT1fyCvbe8E3gW9Kb0MZzHGPAu_SPG2GlyYhRbIMMX4ABJFi5bi609clI9W8YFvXYb_84m2mwZUxvR15MGvTgTMvYYkNO_nJOUaJDwpQcMEuTSVcPfcC7W2gK8XGdHFsryxpxWWCfkVFtvMPT0fnvqrJXTYsp5X7OXCokILd9uLLnNWiczDmg9tCz5U_cwkclG4yqCTvLq_9vrZtxDsjBjrFuTwydZda3sFUJgeNjlzzfcEsgDyYxggxRBMX2undbkMyTW1bs9_xCzCd4GIzqohiwsx209tthd3RVv7DUtI_KOWmL8HoDj6E_svF_g3PZClKtwoAAA_22_7D = __webpack_require__(6579);
;// ./src/components/settings/PiDisableWarningDialog.css.ts


var actions = 'x9n6mdb';
var body = 'x9n6mda';
var dialog = 'x9n6md8';
var PiDisableWarningDialog_css_heading = 'x9n6md9';
var overlay = 'x9n6md7';
var primaryButton = 'x9n6mdc';
var secondaryButton = 'x9n6mdd';
;// ./src/components/settings/PiDisableWarningDialog.tsx
/* __next_internal_client_entry_do_not_use__ PiDisableWarningDialog auto */ 




/**
 * PiDisableWarningDialog — mandatory acknowledgment gate for turning the
 * pi-support flag off (Epic 2.1, Story 2.1.2). Only shown when the
 * pi-extension-status check reports the global approval extension
 * (ssq-approval.ts) is actually installed — otherwise the flag toggles
 * immediately like any other flag, no dialog at all.
 *
 * A mandatory warning was chosen over auto-uninstalling the extension: see
 * project_plans/pi-support/implementation/plan.md's "Decision — mandatory
 * warning, not auto-uninstall" for Story 2.1.2. Modeled on
 * BackwardSyncConfirmDialog's focus-trap + portal pattern.
 */ function PiDisableWarningDialog(param) {
    let { onAcknowledge, onCancel } = param;
    const headingId = (0,react.useId)();
    const dialogRef = (0,react.useRef)(null);
    const acknowledgeButtonRef = (0,react.useRef)(null);
    (0,useDialogFocusTrap/* useDialogFocusTrap */.L)({
        dialogRef,
        initialFocusRef: acknowledgeButtonRef,
        onEscape: onCancel
    });
    const content = /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: overlay,
        "data-testid": "pi-disable-warning-overlay",
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            ref: dialogRef,
            role: "dialog",
            "aria-modal": "true",
            "aria-labelledby": headingId,
            className: dialog,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                    id: headingId,
                    className: PiDisableWarningDialog_css_heading,
                    children: "pi approval extension stays installed"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                    className: body,
                    "data-testid": "pi-disable-warning-body",
                    children: [
                        "Disabling pi-support does NOT remove the pi approval extension (",
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                            children: "ssq-approval.ts"
                        }),
                        ") already installed at",
                        " ",
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                            children: "~/.pi/agent/extensions/ssq-approval.ts"
                        }),
                        ". Direct ",
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                            children: "pi"
                        }),
                        " usage outside stapler-squad remains subject to it. Run",
                        " ",
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                            children: "ssq-hooks install pi --uninstall"
                        }),
                        " to remove it."
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: actions,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            type: "button",
                            className: secondaryButton,
                            onClick: onCancel,
                            "data-testid": "pi-disable-warning-cancel",
                            children: "Cancel"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            ref: acknowledgeButtonRef,
                            type: "button",
                            className: primaryButton,
                            onClick: onAcknowledge,
                            "data-testid": "pi-disable-warning-acknowledge",
                            children: "I understand"
                        })
                    ]
                })
            ]
        })
    });
    // createPortal ensures the dialog renders at document.body, escaping any
    // ancestor CSS transform/filter/will-change that would break position:fixed
    // (ADR-009 / docs/reference/css-architecture.md).
    if (typeof document === "undefined") return null;
    return /*#__PURE__*/ (0,react_dom.createPortal)(content, document.body);
}

// EXTERNAL MODULE: ./src/lib/constants/programs.ts
var programs = __webpack_require__(49051);
;// ./src/app/settings/features/useGateStats.ts
/* __next_internal_client_entry_do_not_use__ GATE_STATS_POLL_MS,loadGateStats,useGateStats auto */ 



const GATE_STATS_POLL_MS = 30000;
// One poller per load function: every component on the page that reads the same
// stats shares one interval and one in-flight request instead of polling separately.
const pollers = new Map();
/** The production source; a module-level function so all callers share its poller. */ const loadGateStats = async ()=>{
    const client = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,api_transport/* getConnectTransport */.qW)());
    return client.getDeliveryGateStats({});
};
function subscribe(load, pollMs, listener) {
    let poller = pollers.get(load);
    if (!poller) {
        const created = {
            listeners: new Set(),
            last: undefined,
            timer: undefined
        };
        const tick = async ()=>{
            let value = null;
            try {
                value = await load();
            } catch (e) {
            // A failed poll renders as "no data"; the next tick retries.
            }
            created.last = {
                value
            };
            created.listeners.forEach((l)=>l(value));
        };
        created.timer = setInterval(()=>void tick(), pollMs);
        pollers.set(load, created);
        void tick();
        poller = created;
    }
    poller.listeners.add(listener);
    if (poller.last) listener(poller.last.value);
    return ()=>{
        poller.listeners.delete(listener);
        if (poller.listeners.size === 0) {
            clearInterval(poller.timer);
            pollers.delete(load);
        }
    };
}
/** Latest result of `load`, polled every `pollMs` while any subscriber is mounted; null before the first result or after a failure. */ function useGateStats(load) {
    let pollMs = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : GATE_STATS_POLL_MS;
    const [value, setValue] = (0,react.useState)(null);
    (0,react.useEffect)(()=>subscribe(load, pollMs, (v)=>setValue(v)), [
        load,
        pollMs
    ]);
    return value;
}

;// ./src/app/settings/features/GateStatusLine.tsx
/* __next_internal_client_entry_do_not_use__ GATE_STATS_POLL_MS,summarizeGateStats,GateStatusLine auto */ 


const DAY_MS = 24 * 60 * 60 * 1000;

/** Buckets are hourly, so "last 24h" means buckets that started within 24h. */ function last24hCount(stats, counter, now) {
    const cutoff = now.getTime() - DAY_MS;
    let total = 0;
    for (const b of stats.buckets){
        if (!b.hourStart || (0,timestamp/* timestampDate */.FP)(b.hourStart).getTime() < cutoff) continue;
        for (const c of b.counters){
            if (c.counter === counter) total += Number(c.count);
        }
    }
    return total;
}
function summarizeGateStats(stats, now) {
    const suppressed = last24hCount(stats, "suppressed", now);
    const wouldSuppress = last24hCount(stats, "would_suppress", now);
    const unresolved = last24hCount(stats, "unresolved", now);
    const unversioned = stats.sinceProcessStart.filter((c)=>c.counter === "rpc_unversioned").reduce((n, c)=>n + Number(c.count), 0);
    return "Last 24h: ".concat(suppressed, " hidden events suppressed; ").concat(wouldSuppress, " would have been suppressed (gate off); ") + "".concat(unresolved, " unresolved fail-open; ").concat(unversioned, " unversioned ssq-notify");
}
/**
 * The standing reminder under hidden_session_gate: reads GetDeliveryGateStats
 * (one poller shared with the kind overrides) while the page is open and stops
 * polling when it unmounts. Failures render nothing (the flag row is still usable).
 */ function GateStatusLine(param) {
    let { className, fetchStats, pollMs = GATE_STATS_POLL_MS } = param;
    const stats = useGateStats(fetchStats !== null && fetchStats !== void 0 ? fetchStats : loadGateStats, pollMs);
    const text = stats ? summarizeGateStats(stats, new Date()) : null;
    if (!text) return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: className,
        "data-testid": "gate-status-line",
        children: text
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Fsettings%2Ffeatures%2Fpage.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA7VWyXKjMBC95yt0marxQRSQOA6aj0kJaINiIVGSsB1Pzb9PSSwSGDvJLBcfml7fe91y9JpeOsUhRj8fEGroGZ9YaWqCsjhuzz-cTVVMEBQj2hlpLS0tSyYqglIFDUqirYLG2vcczgQlLogJXAOrakNQbA3yCGrP5Qm_kyHPr4doqJ242oXkUhF0pOo7xq9JBpdUXeKNSyyFwZpdgITVrPE01NjFse8V59IY2RAUD86-VHq7VLIsFUcvu7HYInG6SPvo0pZMt5y-EweEjaKcVQIzA43ujVgbqoz99NZpw_bvuJDCgDAE6ZYWgHMwJwBhPSrahuNOoCdRam09rLlUJaipr6Q9Iy05KxezwWbWLeFUG1zUjJeu8UUWIQWE_k_OaWLXf9h-iriRo-eeoxnC2eMEsYGzwUZRofdSNQQVtGWGcnaBVWbTJbXPX6M2ScfCnAmYxJpET2HS3TQ61rVi4jDIeViSNJra9wlGy4CqoiXrNEFZlmX9RvUfRpwRKjqlbdOtZMKAcmBYHJhhUhCU0-JQKdmJEsVRstUIqHZxrRxdFHBq2HHG2otr3fvQXEveGRdpZGtBSLzAh4GSyOPiJ_K2xUzb-JuzTh0SdKrZUCMYgcPezJr3bWbz1WHC0ZFzWRxmso-HLqalvmpm0sSC6t3qxXieXQzbIAlSH0EZVlCO3QoT1LCy5LO-ab85weRzzXVOc4McB1B8dP5BNITR12L2iYo7qk83S3IWDv1ejHK8dTySeLOC9tzncbdZsOVQR8kaIaPq_BTlH57l4CiON9EnheFJcwQPir-qvP_C2zO7GmtiWlnk8CEMrsX9p6KAPty3Wa0_MO4wnZR9KezvzUSffm_6DZhoC_i8xrf-d9D5pGz1HIzzfiRV-LRSx_8jBNWsLC0Cvoe3XjjrzFnz9fX3QIUHZ3now1V057GlCoS5uer3YbxSm5_g8MF5ae4eJ35v_vVJ51u2nP_v6frfyP0GMAO74QcLAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Fsettings_2Ffeatures_2Fpage_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA7VWyXKjMBC95yt0marxQRSQOA6aj0kJaINiIVGSsB1Pzb9PSSwSGDvJLBcfml7fe91y9JpeOsUhRj8fEGroGZ9YaWqCsjhuzz_cTVVMEBQj2hlpLS0tSyYqglIFDUqirYLG2vcczgQlLogJXAOrakNQbA3yCGrP5Qm_kyHPr4doqJ242oXkUhF0pOo7xq9JBpdUXeKNSyyFwZpdgITVrPE01NjFse8V59IY2RAUD86_VHq7VLIsFUcvu7HYInG6SPvo0pZMt5y_EweEjaKcVQIzA43ujVgbqoz99NZpw_bvuJDCgDAE6ZYWgHMwJwBhPSrahuNOoCdRam09rLlUJaipr6Q9Iy05KxezwWbWLeFUG1zUjJeu8UUWIQWE_k_OaWLXf9h_iriRo_eeoxnC2eMEsYGzwUZRofdSNQQVtGWGcnaBVWbTJbXPX6M2ScfCnAmYxJpET2HS3TQ61rVi4jDIeViSNJra9wlGy4CqoiXrNEFZlmX9RvUfRpwRKjqlbdOtZMKAcmBYHJhhUhCU0_JQKdmJEsVRstUIqHZxrRxdFHBq2HHG2otr3fvQXEveGRdpZGtBSLzAh4GSyOPiJ_K2xUzb_JuzTh0SdKrZUCMYgcPezJr3bWbz1WHC0ZFzWRxmso_HLqalvmpm0sSC6t3qxXieXQzbIAlSH0EZVlCO3QoT1LCy5LO_ab85weRzzXVOc4McB1B8dP5BNITR12L2iYo7qk83S3IWDv1ejHK8dTySeLOC9tzncbdZsOVQR8kaIaPq_BTlH57l4CiON9EnheFJcwQPir_qvP_C2zO7GmtiWlnk8CEMrsX9p6KAPty3Wa0_MO4wnZR9KezvzUSffm_6DZhoC_i8xrf_d9D5pGz1HIzzfiRV_LRSx_8jBNWsLC0Cvoe3XjjrzFnz9fX3QIUHZ3now1V057GlCoS5uer3YbxSm5_g8MF5ae4eJ35v_vVJ51u2nP_v6frfyP0GMAO74QcLAAA_22_7D = __webpack_require__(26991);
;// ./src/app/settings/features/page.css.ts


var page_css_badge = '_2zurle9';
var page_css_badgeDisabled = '_2zurleb';
var page_css_badgeEnabled = '_2zurlea';
var container = '_2zurle0';
var emptyMessage = '_2zurled';
var page_css_errorMessage = '_2zurlec';
var flagDescription = '_2zurle6';
var flagInfo = '_2zurle4';
var flagName = '_2zurle5';
var flagRow = '_2zurle3';
var overrideLabel = '_2zurleh';
var page_css_overrideRow = '_2zurleg';
var overrideSegment = '_2zurlej';
var overrideSegmentActive = '_2zurlek';
var overrideSegments = '_2zurlei';
var overrides = '_2zurlee';
var overridesSummary = '_2zurlef';
var resetButton = '_2zurlel';
var subtitle = '_2zurle2';
var title = '_2zurle1';
var toggle = '_2zurle7';
var toggleThumb = '_2zurle8';
;// ./src/app/settings/features/GateKindOverrides.tsx
/* __next_internal_client_entry_do_not_use__ GATE_OVERRIDE_KINDS,OVERRIDE_POLL_MS,effectiveText,modeOf,GateKindOverrides auto */ 



/**
 * The closed set of hidden kinds an override may name, mirroring
 * deliverygate.ScopableKinds. kind:triage is absent on purpose: Spike 1.3h
 * found no producer that creates a triage-tagged hidden session.
 */ const GATE_OVERRIDE_KINDS = [
    "review",
    "diagnose",
    "other"
];
const OVERRIDE_POLL_MS = 30000;
function countsOf(raw) {
    if (!(raw === null || raw === void 0 ? void 0 : raw.eventsByKind24h)) return null;
    return Object.fromEntries(Object.entries(raw.eventsByKind24h).map((param)=>{
        let [k, v] = param;
        return [
            k,
            Number(v)
        ];
    }));
}
const MODES = [
    {
        mode: "inherit",
        label: "Inherit"
    },
    {
        mode: "on",
        label: "On"
    },
    {
        mode: "off",
        label: "Off"
    }
];
/** Effective value and where it comes from: "On - override" or "Off - from global". */ function effectiveText(scopes, kind, globalEnabled) {
    const override = scopes === null || scopes === void 0 ? void 0 : scopes["kind:".concat(kind)];
    if (override !== undefined) return "".concat(override ? "On" : "Off", " - override");
    return "".concat(globalEnabled ? "On" : "Off", " - from global");
}
function modeOf(scopes, kind) {
    const v = scopes === null || scopes === void 0 ? void 0 : scopes["kind:".concat(kind)];
    return v === undefined ? "inherit" : v ? "on" : "off";
}
/**
 * Per-session-kind overrides of hidden_session_gate: Inherit / On / Off per
 * reachable kind with the last-24h count of events resolved to that kind (so a
 * dead scope is visible), and a global "Reset to default".
 */ function GateKindOverrides(param) {
    let { scopes, globalEnabled, onChange, onReset, fetchCounts, pollMs = OVERRIDE_POLL_MS } = param;
    // Without a seam this reads the same shared poller as GateStatusLine (one request per tick).
    const raw = useGateStats(fetchCounts !== null && fetchCounts !== void 0 ? fetchCounts : loadGateStats, pollMs);
    const counts = fetchCounts ? raw : countsOf(raw);
    const radioRefs = (0,react.useRef)({});
    const onRadioKey = (e, kind, index, current)=>{
        var _radioRefs_current_kind_next, _radioRefs_current_kind;
        const step = e.key === "ArrowRight" || e.key === "ArrowDown" ? 1 : e.key === "ArrowLeft" || e.key === "ArrowUp" ? -1 : 0;
        if (step === 0) return;
        e.preventDefault();
        const next = (index + step + MODES.length) % MODES.length;
        (_radioRefs_current_kind = radioRefs.current[kind]) === null || _radioRefs_current_kind === void 0 ? void 0 : (_radioRefs_current_kind_next = _radioRefs_current_kind[next]) === null || _radioRefs_current_kind_next === void 0 ? void 0 : _radioRefs_current_kind_next.focus();
        if (MODES[next].mode !== current) onChange("kind:".concat(kind), MODES[next].mode);
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("details", {
        className: overrides,
        "data-testid": "gate-kind-overrides",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("summary", {
                className: overridesSummary,
                children: "Overrides by session kind"
            }),
            GATE_OVERRIDE_KINDS.map((kind)=>{
                const current = modeOf(scopes, kind);
                var _counts_kind;
                const count = counts ? (_counts_kind = counts[kind]) !== null && _counts_kind !== void 0 ? _counts_kind : 0 : null;
                return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: page_css_overrideRow,
                    "data-testid": "gate-override-".concat(kind),
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                            className: overrideLabel,
                            children: [
                                kind,
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    "data-testid": "gate-override-count-".concat(kind),
                                    children: count === null ? "" : " (".concat(count, " events in the last 24h)")
                                }),
                                globalEnabled !== undefined && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    "data-testid": "gate-override-effective-".concat(kind),
                                    children: [
                                        " ",
                                        effectiveText(scopes, kind, globalEnabled)
                                    ]
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: overrideSegments,
                            role: "radiogroup",
                            "aria-label": "Gate override for ".concat(kind, " sessions"),
                            children: MODES.map((param, index)=>{
                                let { mode, label } = param;
                                return /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    ref: (el)=>{
                                        var _radioRefs_current, _kind;
                                        var _;
                                        ((_ = (_radioRefs_current = radioRefs.current)[_kind = kind]) !== null && _ !== void 0 ? _ : _radioRefs_current[_kind] = [])[index] = el;
                                    },
                                    type: "button",
                                    role: "radio",
                                    "aria-checked": current === mode,
                                    tabIndex: current === mode ? 0 : -1,
                                    onKeyDown: (e)=>onRadioKey(e, kind, index, current),
                                    className: "".concat(overrideSegment, " ").concat(current === mode ? overrideSegmentActive : ""),
                                    onClick: ()=>current !== mode && onChange("kind:".concat(kind), mode),
                                    children: label
                                }, mode);
                            })
                        })
                    ]
                }, kind);
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: flagDescription,
                children: "Events from sessions that cannot be resolved to a kind, and events that are not from a session, use the global value."
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                className: resetButton,
                onClick: onReset,
                "data-testid": "gate-reset-default",
                children: "Reset to default"
            })
        ]
    });
}

;// ./src/app/settings/features/page.tsx
// +feature: settings-features
/* __next_internal_client_entry_do_not_use__ default auto */ 












// Mirrors config.HiddenSessionGateFeatureFlag on the server.
const HIDDEN_SESSION_GATE_FLAG = "hidden_session_gate";
// The notification flags are read live by the page (FG-1): no reload after a flip.
const LIVE_FLAG_NOTE = "Takes effect without reload";
const FEATURE_META = {
    backlog: {
        label: "Backlog"
    },
    "programs:cli-flag-probe": {
        label: "Program flag discovery (check binary and read --help)"
    },
    "pi-support": {
        label: "pi coding agent"
    },
    "backlog:sdd-default-pipeline": {
        label: "Backlog: default new items to SDD pipeline"
    },
    "backlog-triage-results-panel": {
        label: "Backlog: formatted triage results panel"
    },
    "backlog-review-results-panel": {
        label: "Backlog: formatted review results panel"
    },
    "review:block-approval-on-ci-failure": {
        label: "Block Approve when CI is failing"
    },
    "terminal:resync-visibility-scope": {
        label: "Terminal resync: scope to visible terminal"
    },
    "terminal:resync-correlation-id": {
        label: "Terminal resync: correlation IDs"
    },
    "terminal:resync-skip-stale-dimension-slowpath": {
        label: "Terminal resync: skip slow path for backgrounded terminals"
    },
    "terminal:resync-exec-gate-fast-lane": {
        label: "Terminal resync: exec-gate fast lane"
    },
    "terminal:resync-stagger": {
        label: "Terminal resync: stagger bursts"
    },
    "terminal:resync-compression": {
        label: "Terminal resync: wire compression"
    },
    "terminal:resync-batching": {
        label: "Terminal resync: batch requests"
    },
    [HIDDEN_SESSION_GATE_FLAG]: {
        label: "Notifications: hidden-session delivery gate"
    }
};
function FeaturesPage() {
    (0,analytics/* usePageView */.Or)();
    const { flagList, isLoading, error, setFlag } = (0,FeatureFlagsContext/* useFeatureFlags */.h)();
    const [pendingPiDisable, setPendingPiDisable] = (0,react.useState)(false);
    // Story 2.1.2: toggling pi-support off is gated on a mandatory warning ONLY
    // when the global pi approval extension is actually installed on disk —
    // otherwise this behaves exactly like toggling any other flag. The check
    // runs server-side (~/.pi is on the server's filesystem, not the browser's)
    // via GET /api/pi-extension-status.
    const handleToggle = (0,react.useCallback)(async (name, currentEnabled)=>{
        const disablingPiSupport = name === programs/* PI_SUPPORT_FLAG_NAME */.HN && currentEnabled;
        if (!disablingPiSupport) {
            // The scopable gate flag always sends the explicit "global" scope, so a
            // dropped scope can never write the global value by accident.
            setFlag(name, name === HIDDEN_SESSION_GATE_FLAG ? {
                mutation: "set",
                scope: featureFlagScopes/* GLOBAL_SCOPE */.N,
                enabled: !currentEnabled
            } : !currentEnabled);
            return;
        }
        try {
            const res = await fetch("/api/pi-extension-status");
            if (!res.ok) {
                // Fail closed: a non-2xx response (e.g. a 500) means we can't trust
                // the body's `installed` field — show the warning rather than
                // silently falling through to a disable. Matches ADR-003's
                // fail-closed posture elsewhere in this feature.
                setPendingPiDisable(true);
                return;
            }
            const data = await res.json();
            if (data.installed) {
                setPendingPiDisable(true);
                return;
            }
        } catch (e) {
            // Fail closed: if the status check itself fails, show the warning
            // rather than silently disabling — matches this feature's fail-closed
            // posture elsewhere (ADR-003) rather than assuming "not installed".
            setPendingPiDisable(true);
            return;
        }
        setFlag(name, false);
    }, [
        setFlag
    ]);
    const confirmPiDisable = (0,react.useCallback)(()=>{
        setPendingPiDisable(false);
        setFlag(programs/* PI_SUPPORT_FLAG_NAME */.HN, false);
    }, [
        setFlag
    ]);
    const changeKindOverride = (0,react.useCallback)((scope, mode)=>{
        setFlag(HIDDEN_SESSION_GATE_FLAG, mode === "inherit" ? {
            mutation: "clear",
            scope
        } : {
            mutation: "set",
            scope,
            enabled: mode === "on"
        });
    }, [
        setFlag
    ]);
    const resetGateDefault = (0,react.useCallback)(()=>{
        setFlag(HIDDEN_SESSION_GATE_FLAG, {
            mutation: "reset"
        });
    }, [
        setFlag
    ]);
    const cancelPiDisable = (0,react.useCallback)(()=>{
        setPendingPiDisable(false);
    }, []);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("main", {
        id: "main-content",
        className: container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("h1", {
                className: title,
                children: "Feature Flags"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: subtitle,
                children: "Toggle experimental or optional features. Changes take effect immediately — no restart needed."
            }),
            error && /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                className: page_css_errorMessage,
                role: "alert",
                children: [
                    error,
                    ". Please refresh."
                ]
            }),
            isLoading ? /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: flagDescription,
                children: "Loading…"
            }) : !error && flagList.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: emptyMessage,
                children: "No feature flags configured."
            }) : flagList.map((param)=>{
                let { name, enabled, description, statusDetail, scopes } = param;
                const meta = FEATURE_META[name];
                var _meta_label;
                const label = (_meta_label = meta === null || meta === void 0 ? void 0 : meta.label) !== null && _meta_label !== void 0 ? _meta_label : name;
                return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: flagRow,
                    "data-testid": "feature-flag-row",
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: flagInfo,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: flagName,
                                    "data-testid": "feature-flag-name",
                                    children: [
                                        label,
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: "".concat(page_css_badge, " ").concat(enabled ? page_css_badgeEnabled : page_css_badgeDisabled),
                                            "data-testid": "feature-flag-status",
                                            children: enabled ? "On" : "Off"
                                        })
                                    ]
                                }),
                                description && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: flagDescription,
                                    children: description
                                }),
                                name === HIDDEN_SESSION_GATE_FLAG && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: flagDescription,
                                    "data-testid": "flag-live-note",
                                    children: LIVE_FLAG_NOTE
                                }),
                                statusDetail && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: flagDescription,
                                    children: statusDetail
                                }),
                                name === HIDDEN_SESSION_GATE_FLAG && /*#__PURE__*/ (0,jsx_runtime.jsx)(GateStatusLine, {
                                    className: flagDescription
                                }),
                                name === HIDDEN_SESSION_GATE_FLAG && /*#__PURE__*/ (0,jsx_runtime.jsx)(GateKindOverrides, {
                                    scopes: scopes,
                                    globalEnabled: enabled,
                                    onChange: changeKindOverride,
                                    onReset: resetGateDefault
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: toggle,
                            style: {
                                background: enabled ? theme_css.vars.color.primary : theme_css.vars.color.borderColor
                            },
                            "aria-label": "".concat(enabled ? "Disable" : "Enable", " ").concat(label),
                            "aria-pressed": enabled,
                            onClick: ()=>handleToggle(name, enabled),
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: toggleThumb,
                                style: {
                                    left: enabled ? "1.375rem" : "0.1875rem"
                                }
                            })
                        })
                    ]
                }, name);
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(StreamHubRolloutPanel, {}),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(TymuxRolloutPanel, {}),
            pendingPiDisable && /*#__PURE__*/ (0,jsx_runtime.jsx)(PiDisableWarningDialog, {
                onAcknowledge: confirmPiDisable,
                onCancel: cancelPiDisable
            })
        ]
    });
}


/***/ }),

/***/ 80070:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   N: () => (/* binding */ GLOBAL_SCOPE)
/* harmony export */ });
/** The scope literal a client sends for a flag's global value (mirrors the server's globalScopeLiteral). */ const GLOBAL_SCOPE = "global";


/***/ }),

/***/ 82279:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   L: () => (/* binding */ useDialogFocusTrap)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);

const FOCUSABLE_SELECTOR = 'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';
/**
 * Shared modal focus-trap behavior: focuses `initialFocusRef` on mount,
 * restores focus to whatever was focused before the dialog opened once it
 * unmounts, and cycles Tab/Shift+Tab within `dialogRef` while routing
 * Escape to `onEscape`.
 *
 * Extracted from BackwardSyncConfirmDialog and HostKeyTrustDialog, which
 * both implemented this identically apart from which button receives
 * initial focus (Confirm vs Cancel).
 */ function useDialogFocusTrap(param) {
    let { dialogRef, initialFocusRef, onEscape } = param;
    // Capture whatever had focus when the dialog mounted (the element that
    // triggered it) so focus can be restored on close.
    const previouslyFocusedRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(typeof document !== "undefined" ? document.activeElement : null);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        var _initialFocusRef_current;
        (_initialFocusRef_current = initialFocusRef.current) === null || _initialFocusRef_current === void 0 ? void 0 : _initialFocusRef_current.focus();
        return ()=>{
            var _previouslyFocusedRef_current;
            (_previouslyFocusedRef_current = previouslyFocusedRef.current) === null || _previouslyFocusedRef_current === void 0 ? void 0 : _previouslyFocusedRef_current.focus();
        };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- run once on mount/unmount only
    }, []);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        const handleKeyDown = (e)=>{
            if (e.key === "Escape") {
                e.preventDefault();
                onEscape();
                return;
            }
            if (e.key === "Tab" && dialogRef.current) {
                const focusable = Array.from(dialogRef.current.querySelectorAll(FOCUSABLE_SELECTOR));
                if (focusable.length === 0) return;
                const first = focusable[0];
                const last = focusable[focusable.length - 1];
                if (e.shiftKey && document.activeElement === first) {
                    e.preventDefault();
                    last.focus();
                } else if (!e.shiftKey && document.activeElement === last) {
                    e.preventDefault();
                    first.focus();
                }
            }
        };
        document.addEventListener("keydown", handleKeyDown);
        return ()=>document.removeEventListener("keydown", handleKeyDown);
    }, [
        dialogRef,
        onEscape
    ]);
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

/***/ 90377:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
__webpack_require__.r(__webpack_exports__);
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   breakpoints: () => (/* binding */ breakpoints),
/* harmony export */   cleanTheme: () => (/* binding */ cleanTheme),
/* harmony export */   cyberpunk77Theme: () => (/* binding */ cyberpunk77Theme),
/* harmony export */   darkTheme: () => (/* binding */ darkTheme),
/* harmony export */   lightTheme: () => (/* binding */ lightTheme),
/* harmony export */   matrixTheme: () => (/* binding */ matrixTheme),
/* harmony export */   vars: () => (/* binding */ vars),
/* harmony export */   wh40kTheme: () => (/* binding */ wh40kTheme),
/* harmony export */   zIndex: () => (/* binding */ zIndex)
/* harmony export */ });
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(65916);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__);

var breakpoints = {sm:'640px',md:'768px',lg:'1024px',xl:'1280px',outer:'390px',fold:'600px',inner:'900px'};
var cleanTheme = '_1b1axoi5';
var cyberpunk77Theme = '_1b1axoi3';
var darkTheme = '_1b1axoi1';
var lightTheme = '_1b1axoi0';
var matrixTheme = '_1b1axoi2';
var vars = {color:{textPrimary:'var(--_19ez2rz0)',textSecondary:'var(--_19ez2rz1)',textMuted:'var(--_19ez2rz2)',textDisabled:'var(--_19ez2rz3)',textTertiary:'var(--_19ez2rz4)',textInverse:'var(--_19ez2rz5)',background:'var(--_19ez2rz6)',cardBackground:'var(--_19ez2rz7)',hoverBackground:'var(--_19ez2rz8)',modalBackground:'var(--_19ez2rz9)',overlayBackground:'var(--_19ez2rza)',panelBgSecondary:'var(--_19ez2rzb)',surfaceSubtle:'var(--_19ez2rzc)',surfaceMuted:'var(--_19ez2rzd)',borderColor:'var(--_19ez2rze)',borderSubtle:'var(--_19ez2rzf)',borderMuted:'var(--_19ez2rzg)',borderStrong:'var(--_19ez2rzh)',borderHover:'var(--_19ez2rzi)',modalBorder:'var(--_19ez2rzj)',inputBorder:'var(--_19ez2rzk)',inputFocusBorder:'var(--_19ez2rzl)',primary:'var(--_19ez2rzm)',primaryHover:'var(--_19ez2rzn)',primaryActive:'var(--_19ez2rzo)',primaryDark:'var(--_19ez2rzp)',primaryText:'var(--_19ez2rzq)',onPrimaryFill:'var(--_19ez2rzr)',onSuccessFill:'var(--_19ez2rzs)',onErrorFill:'var(--_19ez2rzt)',success:'var(--_19ez2rzu)',successBg:'var(--_19ez2rzv)',successText:'var(--_19ez2rzw)',warning:'var(--_19ez2rzx)',warningBg:'var(--_19ez2rzy)',warningText:'var(--_19ez2rzz)',error:'var(--_19ez2rz10)',errorBg:'var(--_19ez2rz11)',errorText:'var(--_19ez2rz12)',errorDark:'var(--_19ez2rz13)',critical:'var(--_19ez2rz14)',criticalBg:'var(--_19ez2rz15)',criticalText:'var(--_19ez2rz16)',accentBg:'var(--_19ez2rz17)',accentHover:'var(--_19ez2rz18)',accentText:'var(--_19ez2rz19)',inputBackground:'var(--_19ez2rz1a)',inputText:'var(--_19ez2rz1b)',placeholderColor:'var(--_19ez2rz1c)',terminalBackground:'var(--_19ez2rz1d)',terminalForeground:'var(--_19ez2rz1e)',terminalBorder:'var(--_19ez2rz1f)',terminalHeaderBg:'var(--_19ez2rz1g)',terminalHeaderFg:'var(--_19ez2rz1h)',terminalTabsBg:'var(--_19ez2rz1i)',terminalTextMuted:'var(--_19ez2rz1j)',terminalHoverBg:'var(--_19ez2rz1k)',headerTextPrimary:'var(--_19ez2rz1l)',headerTextSecondary:'var(--_19ez2rz1m)',logError:'var(--_19ez2rz1n)',logWarn:'var(--_19ez2rz1o)',logInfo:'var(--_19ez2rz1p)',logDebug:'var(--_19ez2rz1q)',logTrace:'var(--_19ez2rz1r)',logOnDark:'var(--_19ez2rz1s)',logOnAmber:'var(--_19ez2rz1t)',logLive:'var(--_19ez2rz1u)',gitModified:'var(--_19ez2rz1v)',gitModifiedBg:'var(--_19ez2rz1w)',gitAddedBg:'var(--_19ez2rz1x)',gitDeletedBg:'var(--_19ez2rz1y)',gitAdded:'var(--_19ez2rz1z)',gitDeleted:'var(--_19ez2rz20)',gitRenamed:'var(--_19ez2rz21)',gitUntracked:'var(--_19ez2rz22)',gitConflict:'var(--_19ez2rz23)',glowPrimary:'var(--_19ez2rz24)',glowSecondary:'var(--_19ez2rz25)',scanlineColor:'var(--_19ez2rz26)',terminalCursor:'var(--_19ez2rz27)',statusDot:{running:'var(--_19ez2rz28)',paused:'var(--_19ez2rz29)',idle:'var(--_19ez2rz2a)'}},statusBadge:{approvalBg:'var(--_19ez2rz2b)',approvalFg:'var(--_19ez2rz2c)',approvalBorder:'var(--_19ez2rz2d)',inputBg:'var(--_19ez2rz2e)',inputFg:'var(--_19ez2rz2f)',inputBorder:'var(--_19ez2rz2g)',completeBg:'var(--_19ez2rz2h)',completeFg:'var(--_19ez2rz2i)',completeBorder:'var(--_19ez2rz2j)',uncommittedBg:'var(--_19ez2rz2k)',uncommittedFg:'var(--_19ez2rz2l)',uncommittedBorder:'var(--_19ez2rz2m)',idleBg:'var(--_19ez2rz2n)',idleFg:'var(--_19ez2rz2o)',idleBorder:'var(--_19ez2rz2p)',staleFg:'var(--_19ez2rz2q)',processingBg:'var(--_19ez2rz2r)',processingFg:'var(--_19ez2rz2s)',processingBorder:'var(--_19ez2rz2t)'},font:{mono:'var(--_19ez2rz2u)',sans:'var(--_19ez2rz2v)',display:'var(--_19ez2rz2w)'},space:{'0':'var(--_19ez2rz2x)','1':'var(--_19ez2rz2y)','2':'var(--_19ez2rz2z)','3':'var(--_19ez2rz30)','4':'var(--_19ez2rz31)','6':'var(--_19ez2rz32)','8':'var(--_19ez2rz33)','12':'var(--_19ez2rz34)','16':'var(--_19ez2rz35)'},radii:{sm:'var(--_19ez2rz36)',md:'var(--_19ez2rz37)',lg:'var(--_19ez2rz38)',full:'var(--_19ez2rz39)'},fontSize:{xs:'var(--_19ez2rz3a)',sm:'var(--_19ez2rz3b)',base:'var(--_19ez2rz3c)',lg:'var(--_19ez2rz3d)',xl:'var(--_19ez2rz3e)'},fontWeight:{normal:'var(--_19ez2rz3f)',medium:'var(--_19ez2rz3g)',semibold:'var(--_19ez2rz3h)',bold:'var(--_19ez2rz3i)'},shadow:{none:'var(--_19ez2rz3j)',sm:'var(--_19ez2rz3k)',md:'var(--_19ez2rz3l)',lg:'var(--_19ez2rz3m)'},transition:{fast:'var(--_19ez2rz3n)',base:'var(--_19ez2rz3o)',slow:'var(--_19ez2rz3p)'}};
var wh40kTheme = '_1b1axoi4';
var zIndex = {base:0,tableHeader:1,raised:10,terminalOverlay:15,terminalOverlayPill:20,terminalPaneChrome:25,terminalFilterPanel:26,header:100,dropdown:500,slideOver:700,modal:1000,bottomNavMoreBackdrop:1040,bottomNavMoreSheet:1045,bottomNav:1050,mobilePickerBackdrop:1060,mobilePickerSheet:1065,dialog:1070,toast:1080,floatingTerminalUI:1085,tooltip:1100};

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
/******/ __webpack_require__.O(0, [1869,852,4202,9353,8165,3575,9691,3516,7358], () => (__webpack_exec__(58203)));
/******/ var __webpack_exports__ = __webpack_require__.O();
/******/ _N_E = __webpack_exports__;
/******/ }
]);
//# sourceMappingURL=page-cfee65018f28baeb.js.map