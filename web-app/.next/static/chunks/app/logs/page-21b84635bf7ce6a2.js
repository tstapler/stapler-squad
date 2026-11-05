(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[1524],{

/***/ 3335:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   CC: () => (/* binding */ formatTimestampShort),
/* harmony export */   Ey: () => (/* binding */ formatTimestamp),
/* harmony export */   dB: () => (/* binding */ getUserTimezone),
/* harmony export */   fw: () => (/* binding */ formatRelativeTime),
/* harmony export */   jX: () => (/* binding */ TIME_RANGE_PRESETS)
/* harmony export */ });
/* unused harmony export formatTime */
/**
 * Utility functions for date and time formatting with timezone support
 */ /**
 * Format a timestamp for display with full date, time, and timezone
 * @param date - Date object or timestamp to format
 * @returns Formatted string in user's local timezone (e.g., "1/5/2025, 3:45:30 PM PST")
 */ function formatTimestamp(date) {
    const dateObj = typeof date === 'number' ? new Date(date) : date;
    return dateObj.toLocaleString(undefined, {
        year: 'numeric',
        month: 'numeric',
        day: 'numeric',
        hour: 'numeric',
        minute: '2-digit',
        second: '2-digit',
        timeZoneName: 'short'
    });
}
/**
 * Format a timestamp for display with date and time (no timezone)
 * @param date - Date object or timestamp to format
 * @returns Formatted string in user's local timezone (e.g., "1/5/2025, 3:45:30 PM")
 */ function formatTimestampShort(date) {
    const dateObj = typeof date === 'number' ? new Date(date) : date;
    return dateObj.toLocaleString(undefined, {
        year: 'numeric',
        month: 'numeric',
        day: 'numeric',
        hour: 'numeric',
        minute: '2-digit',
        second: '2-digit'
    });
}
/**
 * Format a timestamp for display with just time (no date)
 * @param date - Date object or timestamp to format
 * @returns Formatted string in user's local timezone (e.g., "3:45:30 PM")
 */ function formatTime(date) {
    const dateObj = typeof date === 'number' ? new Date(date) : date;
    return dateObj.toLocaleTimeString(undefined, {
        hour: 'numeric',
        minute: '2-digit',
        second: '2-digit'
    });
}
/**
 * Format a timestamp as relative time (e.g., "5m ago", "2h ago")
 * @param timestamp - Unix timestamp in milliseconds
 * @returns Relative time string
 */ function formatRelativeTime(timestamp) {
    const date = new Date(timestamp);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);
    const diffDays = Math.floor(diffMs / 86400000);
    if (diffMins < 1) return "Just now";
    if (diffMins < 60) return "".concat(diffMins, "m ago");
    if (diffHours < 24) return "".concat(diffHours, "h ago");
    if (diffDays < 7) return "".concat(diffDays, "d ago");
    return date.toLocaleDateString();
}
/**
 * Get the user's timezone abbreviation (e.g., "PST", "EST", "UTC")
 */ function getUserTimezone() {
    const date = new Date();
    const timeZoneName = date.toLocaleString(undefined, {
        timeZoneName: 'short'
    }).split(' ').pop();
    return timeZoneName || Intl.DateTimeFormat().resolvedOptions().timeZone;
}
/**
 * Standard time range presets for log filtering
 */ const TIME_RANGE_PRESETS = [
    {
        label: 'Last 5 minutes',
        value: '5m',
        getRange: ()=>({
                start: new Date(Date.now() - 5 * 60 * 1000),
                end: new Date()
            })
    },
    {
        label: 'Last 15 minutes',
        value: '15m',
        getRange: ()=>({
                start: new Date(Date.now() - 15 * 60 * 1000),
                end: new Date()
            })
    },
    {
        label: 'Last 1 hour',
        value: '1h',
        getRange: ()=>({
                start: new Date(Date.now() - 60 * 60 * 1000),
                end: new Date()
            })
    },
    {
        label: 'Last 4 hours',
        value: '4h',
        getRange: ()=>({
                start: new Date(Date.now() - 4 * 60 * 60 * 1000),
                end: new Date()
            })
    },
    {
        label: 'Last 24 hours',
        value: '24h',
        getRange: ()=>({
                start: new Date(Date.now() - 24 * 60 * 60 * 1000),
                end: new Date()
            })
    },
    {
        label: 'Last 7 days',
        value: '7d',
        getRange: ()=>({
                start: new Date(Date.now() - 7 * 24 * 60 * 60 * 1000),
                end: new Date()
            })
    },
    {
        label: 'All time',
        value: 'all',
        getRange: ()=>({
                start: new Date(0),
                end: new Date()
            })
    }
];


/***/ }),

/***/ 8462:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   c: () => (/* binding */ createRuntimeFn)
/* harmony export */ });
/* unused harmony export m */
function toPrimitive(t, r) {
  if ("object" != typeof t || !t) return t;
  var e = t[Symbol.toPrimitive];
  if (void 0 !== e) {
    var i = e.call(t, r || "default");
    if ("object" != typeof i) return i;
    throw new TypeError("@@toPrimitive must return a primitive value.");
  }
  return ("string" === r ? String : Number)(t);
}

function toPropertyKey(t) {
  var i = toPrimitive(t, "string");
  return "symbol" == typeof i ? i : String(i);
}

function _defineProperty(obj, key, value) {
  key = toPropertyKey(key);
  if (key in obj) {
    Object.defineProperty(obj, key, {
      value: value,
      enumerable: true,
      configurable: true,
      writable: true
    });
  } else {
    obj[key] = value;
  }
  return obj;
}

function ownKeys(e, r) {
  var t = Object.keys(e);
  if (Object.getOwnPropertySymbols) {
    var o = Object.getOwnPropertySymbols(e);
    r && (o = o.filter(function (r) {
      return Object.getOwnPropertyDescriptor(e, r).enumerable;
    })), t.push.apply(t, o);
  }
  return t;
}
function _objectSpread2(e) {
  for (var r = 1; r < arguments.length; r++) {
    var t = null != arguments[r] ? arguments[r] : {};
    r % 2 ? ownKeys(Object(t), !0).forEach(function (r) {
      _defineProperty(e, r, t[r]);
    }) : Object.getOwnPropertyDescriptors ? Object.defineProperties(e, Object.getOwnPropertyDescriptors(t)) : ownKeys(Object(t)).forEach(function (r) {
      Object.defineProperty(e, r, Object.getOwnPropertyDescriptor(t, r));
    });
  }
  return e;
}

function mapValues(input, fn) {
  var result = {};
  for (var _key in input) {
    result[_key] = fn(input[_key], _key);
  }
  return result;
}

var shouldApplyCompound = (compoundCheck, selections, defaultVariants) => {
  for (var key of Object.keys(compoundCheck)) {
    var _selections$key;
    if (compoundCheck[key] !== ((_selections$key = selections[key]) !== null && _selections$key !== void 0 ? _selections$key : defaultVariants[key])) {
      return false;
    }
  }
  return true;
};
var createRuntimeFn = config => {
  var runtimeFn = options => {
    var className = config.defaultClassName;
    var selections = _objectSpread2(_objectSpread2({}, config.defaultVariants), options);
    for (var variantName in selections) {
      var _selections$variantNa;
      var variantSelection = (_selections$variantNa = selections[variantName]) !== null && _selections$variantNa !== void 0 ? _selections$variantNa : config.defaultVariants[variantName];
      if (variantSelection != null) {
        var selection = variantSelection;
        if (typeof selection === 'boolean') {
          // @ts-expect-error
          selection = selection === true ? 'true' : 'false';
        }
        var selectionClassName =
        // @ts-expect-error
        config.variantClassNames[variantName][selection];
        if (selectionClassName) {
          className += ' ' + selectionClassName;
        }
      }
    }
    for (var [compoundCheck, compoundClassName] of config.compoundVariants) {
      if (shouldApplyCompound(compoundCheck, selections, config.defaultVariants)) {
        className += ' ' + compoundClassName;
      }
    }
    return className;
  };
  runtimeFn.variants = () => Object.keys(config.variantClassNames);
  runtimeFn.classNames = {
    get base() {
      return config.defaultClassName.split(' ')[0];
    },
    get variants() {
      return mapValues(config.variantClassNames, classNames => mapValues(classNames, className => className.split(' ')[0]));
    }
  };
  return runtimeFn;
};




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

/***/ 30514:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 31123:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 46309));


/***/ }),

/***/ 38690:
/***/ (() => {

// extracted by mini-css-extract-plugin

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

/***/ 46309:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  "default": () => (/* binding */ LogsPage)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/utils/datetime.ts
var datetime = __webpack_require__(3335);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Flogs%2FTimeRangePicker.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA8VXwVLbMBC95yt06UBmcEY2BIP4Bo49M4q1idXIWiPJIaTDv3ckO2AbA05C28khsax9Wr19b6XMHuLNL_EIG0p-Twgp0UonUTNiQHEnN3A3IURIWyr-zIjUSmqIFgqz9d3kZTLbR8ch-nXeUsHWB3IlVzqSDgrLSAbagfHDK14yQmdzA4V_LLkQUq_2Q4TO0v2rBc_WK4OVFlGGCg0jG27Oo-ghvoVdYnbpNMxCI8AwEpdbYlFJ0ZsFYdYgAG0BRIYLWVlGrsqQfVYZ6yNKlPvMl6hdZOUOfLK3TZJhcMkLqZ4ZObtHzTM8uyBn96BV-PFzUWlXkXvU_rFAjbbkWaDWGa73lDdZhDwJnSX24t3-w3CXeZbjBkzg_yu2btqbHZyxnvbAl5hVNoBj5XzxGdGo4Ssc1ay0jWzOBT4xQsMnKbe9mXHaXTIJi7V4jgPLrRmX9QwFW0biwCFsXRSkxoiCpfNjhdTRkxQuZyROqK9nC-GqvwadpU0tB3eTdFOc96zCFxZV5ep6YslITOkP_-CTYYSGfLhZSR2F1428vlPbPQFfNyu0-O9GXdYF2kVSi8AjpT3WEkprEK-upfIQuRQCdIeK62Hf--9ISANZzVCGqir0gNc7YGnN65hu0Ffh2_MnLh_SyYDHh5tBRy038--2_nufx_Ou0dMDjd4J_dTGbyKR2oL72Ko92JtRuRQfN9_HLt5twMtBrnIXBD_GJbX-a3v50iS1aDrAPADv20Fjzr-hs-IQnf13RfHjFbXoWzV95-bslCtB0y4X6BwWrF2bYdbGtG3R2-hRtR1dxzduTqtqewOtegWNCcjQ8LrelRZgvLM7IXBofx5Zh9YSyyOXSAaQbMn1wNl8SJVXQ4fImEZyPeq4XR93lfxur7c3fModrYWTj2iSY3gspqMt9XjC5ftfNMzeVTuv_cc0unMmpOULBWI6qn3qHtev4aOiP_n_knS6kkZ_8Ch8AnE3efkDCr8xw9wNAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Flogs_2FTimeRangePicker_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA8VXwVLbMBC95yt06UBmcEY2BIP4Bo49M4q1idXIWiPJIaTDv3ckO2AbA05C28khsax9Wr19b6XMHuLNL_EIG0p_Twgp0UonUTNiQHEnN3A3IURIWyr_zIjUSmqIFgqz9d3kZTLbR8ch_nXeUsHWB3IlVzqSDgrLSAbagfHDK14yQmdzA4V_LLkQUq_2Q4TO0v2rBc_WK4OVFlGGCg0jG27Oo_ghvoVdYnbpNMxCI8AwEpdbYlFJ0ZsFYdYgAG0BRIYLWVlGrsqQfVYZ6yNKlPvMl6hdZOUOfLK3TZJhcMkLqZ4ZObtHzTM8uyBn96BV_PFzUWlXkXvU_rFAjbbkWaDWGa73lDdZhDwJnSX24t3_w3CXeZbjBkzg_yu2btqbHZyxnvbAl5hVNoBj5XzxGdGo4Ssc1ay0jWzOBT4xQsMnKbe9mXHaXTIJi7V4jgPLrRmX9QwFW0biwCFsXRSkxoiCpfNjhdTRkxQuZyROqK9nC_GqvwadpU0tB3eTdFOc96zCFxZV5ep6YslITOkP_CTYYSGfLhZSR2F1428vlPbPQFfNyu0_O9GXdYF2kVSi8AjpT3WEkprEK_upfIQuRQCdIeK62Hf_9ISANZzVCGqir0gNc7YGnN65hu0Ffh2_MnLh_SyYDHh5tBRy038_2_nufx_Ou0dMDjd4J_dTGbyKR2oL72Ko92JtRuRQfN9_HLt5twMtBrnIXBD_GJbX_a3v50iS1aDrAPADv20Fjzr_hs_IQnf13RfHjFbXoWzV95_bslCtB0y4X6BwWrF2bYdbGtG3R2_hRtR1dxzduTqtqewOtegWNCcjQ8LrelRZgvLM7IXBofx5Zh9YSyyOXSAaQbMn1wNl8SJVXQ4fImEZyPeq4XR93lfxur7c3fModrYWTj2iSY3gspqMt9XjC5ftfNMzeVTuv_cc0unMmpOULBWI6qn3qHtev4aOiP_n_knS6kkZ_8Ch8AnE3efkDCr8xw9wNAAA_22_7D = __webpack_require__(90653);
;// ./src/components/logs/TimeRangePicker.css.ts


var applyButton = '_1vjdqevh';
var backButton = '_1vjdqevd';
var chevron = '_1vjdqev4';
var container = '_1vjdqev0';
var customButton = '_1vjdqeva';
var customHeader = '_1vjdqevc';
var customInputs = '_1vjdqeve';
var customRange = '_1vjdqevb';
var dateInput = '_1vjdqevg';
var divider = '_1vjdqev9';
var dropdown = '_1vjdqev5';
var icon = '_1vjdqev2';
var inputGroup = '_1vjdqevf';
var label = '_1vjdqev3';
var presetButton = '_1vjdqev7';
var presetButtonActive = '_1vjdqev8';
var presets = '_1vjdqev6';
var trigger = '_1vjdqev1';
;// ./src/components/logs/TimeRangePicker.tsx
/* __next_internal_client_entry_do_not_use__ TimeRangePicker auto */ 



function TimeRangePicker(param) {
    let { value, onChange, className } = param;
    const [isOpen, setIsOpen] = (0,react.useState)(false);
    const [showCustom, setShowCustom] = (0,react.useState)(false);
    const [customStart, setCustomStart] = (0,react.useState)('');
    const [customEnd, setCustomEnd] = (0,react.useState)('');
    const dropdownRef = (0,react.useRef)(null);
    // Get current display label
    const getDisplayLabel = ()=>{
        if (value.preset) {
            const preset = datetime/* TIME_RANGE_PRESETS */.jX.find((p)=>p.value === value.preset);
            return (preset === null || preset === void 0 ? void 0 : preset.label) || 'Custom';
        }
        return 'Custom range';
    };
    // Handle preset selection
    const handlePresetClick = (preset)=>{
        const range = preset.getRange();
        onChange({
            start: range.start,
            end: range.end,
            preset: preset.value
        });
        setIsOpen(false);
        setShowCustom(false);
    };
    // Handle custom range submission
    const handleCustomSubmit = ()=>{
        if (customStart && customEnd) {
            const start = new Date(customStart);
            const end = new Date(customEnd);
            if (!isNaN(start.getTime()) && !isNaN(end.getTime()) && start < end) {
                onChange({
                    start,
                    end,
                    preset: undefined
                });
                setIsOpen(false);
                setShowCustom(false);
            }
        }
    };
    // Close dropdown when clicking outside
    (0,react.useEffect)(()=>{
        const handleClickOutside = (event)=>{
            if (dropdownRef.current && !dropdownRef.current.contains(event.target)) {
                setIsOpen(false);
                setShowCustom(false);
            }
        };
        if (isOpen) {
            document.addEventListener('mousedown', handleClickOutside);
        }
        return ()=>{
            document.removeEventListener('mousedown', handleClickOutside);
        };
    }, [
        isOpen
    ]);
    // Handle keyboard navigation
    const handleKeyDown = (event)=>{
        if (event.key === 'Escape') {
            setIsOpen(false);
            setShowCustom(false);
        } else if (event.key === 'Enter' && !isOpen) {
            setIsOpen(true);
        }
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: "".concat(container, " ").concat(className || ''),
        ref: dropdownRef,
        onKeyDown: handleKeyDown,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                className: trigger,
                onClick: ()=>setIsOpen(!isOpen),
                "aria-haspopup": "listbox",
                "aria-expanded": isOpen,
                "aria-label": "Time range: ".concat(getDisplayLabel()),
                type: "button",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: icon,
                        children: "\uD83D\uDD50"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: label,
                        children: getDisplayLabel()
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: chevron,
                        children: isOpen ? '▲' : '▼'
                    })
                ]
            }),
            isOpen && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: dropdown,
                role: "listbox",
                "aria-label": "Select time range",
                children: !showCustom ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: presets,
                            children: datetime/* TIME_RANGE_PRESETS */.jX.map((preset)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: "".concat(presetButton, " ").concat(value.preset === preset.value ? presetButtonActive : ''),
                                    onClick: ()=>handlePresetClick(preset),
                                    role: "option",
                                    "aria-selected": value.preset === preset.value,
                                    children: preset.label
                                }, preset.value))
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: divider
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: customButton,
                            onClick: ()=>setShowCustom(true),
                            type: "button",
                            children: "Custom range..."
                        })
                    ]
                }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: customRange,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: customHeader,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: backButton,
                                    onClick: ()=>setShowCustom(false),
                                    type: "button",
                                    "aria-label": "Back to presets",
                                    children: "← Back"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    children: "Custom Range"
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: customInputs,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                    className: inputGroup,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            children: "Start:"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                            type: "datetime-local",
                                            value: customStart,
                                            onChange: (e)=>setCustomStart(e.target.value),
                                            className: dateInput,
                                            "aria-label": "Start date and time"
                                        })
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                    className: inputGroup,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            children: "End:"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                            type: "datetime-local",
                                            value: customEnd,
                                            onChange: (e)=>setCustomEnd(e.target.value),
                                            className: dateInput,
                                            "aria-label": "End date and time"
                                        })
                                    ]
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: applyButton,
                            onClick: handleCustomSubmit,
                            disabled: !customStart || !customEnd,
                            type: "button",
                            children: "Apply"
                        })
                    ]
                })
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Flogs%2FFilterPill.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61T0Y6bMBB8z1fsS5VGOkeGNr2r-w157HPlYAPuwS61zYWk6r9XNpA66XHXSickYNfLzDBjb_0Pnhecw88VgDKua-RJQNno4csK4pMdrewEhHtoVaHg253VbShlYypkxuvWCSg0em1Du5W2MsgO5D21ArI4_Wu1HdmyazaDjUHNZtIFyIk4n5k7qZTB6tJLRB1k8VhZ6lGxghqyAp6kfc_Yt-yzPuf2fL-JU2SVtgKybgBHjVE3U4_JFLNSmd4JyD51ozOEnjlz1oH_YeKNzVK2pjkJWO8JZUHrO1jvNTbx5euhR9_DnjCULSG5ThY6sSaP1jyrOt8kcx-W5_jmouWoTVV7ATvOk28_Ph_2gu_fe-dNeWIFodfo06WjUb7-40k9sc31tAkaXfqF5K7DEoCEOo1mrhf9ACh668JaR2aWdRPZjr-7SSybhMRtd1EdOt5KdMYbQjGSAt9mO3f315Ya-6mpoqYnbZdjycZcXtubWba5Qi2p6F1Epd4HwalLA3O1VHQUwOOVd8Mt3EMKt4tA_3Ry_jOMdimMNz4oixGlf_laFBjFej14pnRBVo5oPSptg8VXWC8E8CLCb7r1oOlaBQAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Flogs_2FFilterPill_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61T0Y6bMBB8z1fsS5VGOkeGNr2r_w157HPlYAPuwS61zYWk6r9XNpA66XHXSickYNfLzDBjb_0Pnhecw88VgDKua_RJQNno4csK4pMdrewEhHtoVaHg253VbShlYypkxuvWCSg0em1Du5W2MsgO5D21ArI4_Wu1HdmyazaDjUHNZtIFyIk4n5k7qZTB6tJLRB1k8VhZ6lGxghqyAp6kfc_Yt_yzPuf2fL_JU2SVtgKybgBHjVE3U4_JFLNSmd4JyD51ozOEnjlz1oH_YeKNzVK2pjkJWO8JZUHrO1jvNTbx5euhR9_DnjCULSG5ThY6sSaP1jyrOt8kcx_W5_jmouWoTVV7ATvOk28_Ph_2gu_fe_dNeWIFodfo06WjUb7_40k9sc31tAkaXfqF5K7DEoCEOo1mrhf9ACh668JaR2aWdRPZjr_7SSybhMRtd1EdOt5KdMYbQjGSAt9mO3f315Ya_6mpoqYnbZdjycZcXtubWba5Qi2p6F1Epd4HwalLA3O1VHQUwOOVd8Mt3EMKt4tA_3Ry_jOMdimMNz4oixGlf_laFBjFej14pnRBVo5oPSptg8VXWC8E8CLCb7r1oOlaBQAA_22_7D = __webpack_require__(30514);
;// ./src/components/logs/FilterPill.css.ts


var clearAllButton = 'tq02c05';
var FilterPill_css_container = 'tq02c00';
var FilterPill_css_label = 'tq02c02';
var pill = 'tq02c01';
var removeButton = 'tq02c04';
var FilterPill_css_value = 'tq02c03';
;// ./src/components/logs/FilterPill.tsx
/* __next_internal_client_entry_do_not_use__ FilterPill,FilterPills auto */ 


function FilterPill(param) {
    let { label, value, onRemove, color, className } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: "".concat(pill, " ").concat(className || ''),
        style: color ? {
            borderColor: color
        } : undefined,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                className: FilterPill_css_label,
                children: [
                    label,
                    ":"
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: FilterPill_css_value,
                style: color ? {
                    color
                } : undefined,
                children: value
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                className: removeButton,
                onClick: onRemove,
                "aria-label": "Remove ".concat(label, ": ").concat(value, " filter"),
                type: "button",
                children: "\xd7"
            })
        ]
    });
}
function FilterPills(param) {
    let { children, onClearAll, className } = param;
    const hasChildren = react.Children.count(children) > 0;
    if (!hasChildren) {
        return null;
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: "".concat(FilterPill_css_container, " ").concat(className || ''),
        children: [
            children,
            onClearAll && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                className: clearAllButton,
                onClick: onClearAll,
                type: "button",
                "aria-label": "Clear all filters",
                children: "Clear all"
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/timestamp.js
var timestamp = __webpack_require__(28765);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Flogs%2FExportButton.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA71Ty27bMBC8-yv2UjgGKkGSn6G_wceeA1pcS4QprkBSfgX594C0lEqq0-YQ9CZSu7PDmdn4JV1kVXW0CbxOAGqy0knSDAwq7uQJt5O3SdwVpaFISFsrfmVwUHjZTgC4koWOpMPKMshROzT-uuA1gyReGqz8seZCSF10V5DE6-7XnufHwlCjRZSTIsPgxM1TFL2kz3jLzG09C1VkBBoGaX0BS0qKUdUxVD0ESHoAkeFCNpbBog7s88ZY31GT7JgfSLvIyht6spuOZbg98EqqK4PpjjTPafoTpjvUKnz82jfaNbAj7Y8VabI1z9H3OsN1Jy1XCpI4s0NpWUknNEyTe2JCWr5XKGZB73-ps-k_7mFFNhvN6gYEfKp5Lt01GNMXRJOLuFJ0RjFoz0ZR4XtLqnH3d1LNIE2SH_5gZFE6Bon_rrgppI7C_1b473EdH1m7aifQJbIlF3QeY89VaLtFUgu8BMqBpdTRWQpXMkg3yR3E23JQHqKUQqAeaDH_po1YZZ_vhHdCY1-L7vyXrP-XVHdStX47vPjEyEIzUHhw4-CPDYckTpfDNZjf1-DLue-1LkLT4JHP7RtbnlnraJ9nZ04PaPkb6Iz3CC99PHolqz9nfRj36QZ-LIHXhgFvHA1A1wG0bEemX1sS7AF7Gm2OtpO3d-grlqXbBQAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Flogs_2FExportButton_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA71Ty27bMBC8_yv2UjgGKkGSn6G_wceeA1pcS4QprkBSfgX594C0lEqq0_YQ9CZSu7PDmdn4JV1kVXW0CbxOAGqy0knSDAwq7uQJt5O3SdwVpaFISFsrfmVwUHjZTgC4koWOpMPKMshROzT_uuA1gyReGqz8seZCSF10V5DE6_7XnufHwlCjRZSTIsPgxM1TFL2kz3jLzG09C1VkBBoGaX0BS0qKUdUxVD0ESHoAkeFCNpbBog7s88ZY31GT7JgfSLvIyht6spuOZbg98EqqK4PpjjTPafoTpjvUKnz82jfaNbAj7Y8VabI1z9H3OsN1Jy1XCpI4s0NpWUknNEyTe2JCWr5XKGZB73_ps_k_7mFFNhvN6gYEfKp5Lt01GNMXRJOLuFJ0RjFoz0ZR4XtLqnH3d1LNIE2SH_5gZFE6Bon_rrgppI7C_1b473EdH1m7aifQJbIlF3QeY89VaLtFUgu8BMqBpdTRWQpXMkg3yR3E23JQHqKUQqAeaDH_po1YZZ_vhHdCY1_L7vyXrP_XVHdStX47vPjEyEIzUHhw4_CPDYckTpfDNZjf1_DLue_1LkLT4JHP7RtbnlnraJ9nZ04PaPkb6Iz3CC99PHolqz9nfRj36QZ_LIHXhgFvHA1A1wG0bEemX1sS7AF7Gm2OtpO3d_grlqXbBQAA_22_7D = __webpack_require__(38690);
;// ./src/components/logs/ExportButton.css.ts


var ExportButton_css_button = '_142mmks1';
var ExportButton_css_container = '_142mmks0';
var ExportButton_css_divider = '_142mmks7';
var ExportButton_css_dropdown = '_142mmks2';
var ExportButton_css_icon = '_142mmks4';
var ExportButton_css_option = '_142mmks3';
var optionDesc = '_142mmks6';
var optionLabel = '_142mmks5';
;// ./src/components/logs/ExportButton.tsx
/* __next_internal_client_entry_do_not_use__ ExportButton auto */ 



function ExportButton(param) {
    let { logs, disabled, className } = param;
    const [showDropdown, setShowDropdown] = (0,react.useState)(false);
    const [exporting, setExporting] = (0,react.useState)(false);
    const containerRef = (0,react.useRef)(null);
    // Close dropdown when clicking outside
    (0,react.useEffect)(()=>{
        const handleClickOutside = (event)=>{
            if (containerRef.current && !containerRef.current.contains(event.target)) {
                setShowDropdown(false);
            }
        };
        if (showDropdown) {
            document.addEventListener('mousedown', handleClickOutside);
        }
        return ()=>{
            document.removeEventListener('mousedown', handleClickOutside);
        };
    }, [
        showDropdown
    ]);
    // Format log entry for export
    const formatLogEntry = (log)=>({
            timestamp: log.timestamp ? (0,timestamp/* timestampDate */.FP)(log.timestamp).toISOString() : '',
            level: log.level,
            source: log.source || '',
            message: log.message
        });
    // Export as JSON
    const exportJSON = ()=>{
        const data = logs.map(formatLogEntry);
        const blob = new Blob([
            JSON.stringify(data, null, 2)
        ], {
            type: 'application/json'
        });
        downloadBlob(blob, "logs-".concat(getTimestamp(), ".json"));
    };
    // Export as CSV
    const exportCSV = ()=>{
        const headers = [
            'timestamp',
            'level',
            'source',
            'message'
        ];
        const rows = logs.map((log)=>{
            const entry = formatLogEntry(log);
            return [
                entry.timestamp,
                entry.level,
                entry.source,
                // Escape quotes and wrap in quotes if contains comma or newline
                '"'.concat(entry.message.replace(/"/g, '""'), '"')
            ].join(',');
        });
        const csv = [
            headers.join(','),
            ...rows
        ].join('\n');
        const blob = new Blob([
            csv
        ], {
            type: 'text/csv'
        });
        downloadBlob(blob, "logs-".concat(getTimestamp(), ".csv"));
    };
    // Export as plain text
    const exportTXT = ()=>{
        const lines = logs.map((log)=>{
            const entry = formatLogEntry(log);
            return "[".concat(entry.timestamp, "] [").concat(entry.level, "] [").concat(entry.source, "] ").concat(entry.message);
        });
        const text = lines.join('\n');
        const blob = new Blob([
            text
        ], {
            type: 'text/plain'
        });
        downloadBlob(blob, "logs-".concat(getTimestamp(), ".txt"));
    };
    // Generate timestamp for filename
    const getTimestamp = ()=>{
        return new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
    };
    // Download blob as file
    const downloadBlob = (blob, filename)=>{
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = filename;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);
    };
    // Handle export
    const handleExport = async (format)=>{
        setExporting(true);
        setShowDropdown(false);
        try {
            switch(format){
                case 'json':
                    exportJSON();
                    break;
                case 'csv':
                    exportCSV();
                    break;
                case 'txt':
                    exportTXT();
                    break;
            }
        } finally{
            setExporting(false);
        }
    };
    // Copy to clipboard
    const copyToClipboard = async ()=>{
        setShowDropdown(false);
        const lines = logs.map((log)=>{
            const entry = formatLogEntry(log);
            return "[".concat(entry.timestamp, "] [").concat(entry.level, "] [").concat(entry.source, "] ").concat(entry.message);
        });
        await navigator.clipboard.writeText(lines.join('\n'));
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: "".concat(ExportButton_css_container, " ").concat(className || ''),
        ref: containerRef,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                className: ExportButton_css_button,
                onClick: ()=>setShowDropdown(!showDropdown),
                disabled: disabled || logs.length === 0 || exporting,
                "aria-haspopup": "menu",
                "aria-expanded": showDropdown,
                "aria-label": "Export ".concat(logs.length, " logs"),
                title: logs.length === 0 ? 'No logs to export' : "Export ".concat(logs.length, " logs"),
                children: [
                    exporting ? '...' : '↓',
                    " Export"
                ]
            }),
            showDropdown && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: ExportButton_css_dropdown,
                role: "menu",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                        className: ExportButton_css_option,
                        onClick: ()=>handleExport('json'),
                        role: "menuitem",
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: ExportButton_css_icon
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: optionLabel,
                                children: "JSON"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: optionDesc,
                                children: "Structured data"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                        className: ExportButton_css_option,
                        onClick: ()=>handleExport('csv'),
                        role: "menuitem",
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: ExportButton_css_icon,
                                children: "\uD83D\uDCCA"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: optionLabel,
                                children: "CSV"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: optionDesc,
                                children: "Spreadsheet"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                        className: ExportButton_css_option,
                        onClick: ()=>handleExport('txt'),
                        role: "menuitem",
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: ExportButton_css_icon,
                                children: "\uD83D\uDCC4"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: optionLabel,
                                children: "Plain Text"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: optionDesc,
                                children: "Simple format"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: ExportButton_css_divider
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                        className: ExportButton_css_option,
                        onClick: copyToClipboard,
                        role: "menuitem",
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: ExportButton_css_icon,
                                children: "\uD83D\uDCCB"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: optionLabel,
                                children: "Copy All"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: optionDesc,
                                children: "To clipboard"
                            })
                        ]
                    })
                ]
            })
        ]
    });
}

;// ./src/lib/logs/logPatterns.ts
// logPatterns.ts — Datadog-style "log pattern" clustering: groups log
// entries by message with dynamic content (UUIDs, paths, embedded objects,
// numbers) normalized into placeholders, so structurally identical messages
// collapse into one row even when a value is interpolated into the message
// text.
const UUID_RE = /[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}/g;
const PATH_RE = /(\/[A-Za-z0-9_.-]+){2,}/g;
const OBJ_RE = /\{[^{}]*\}/g;
const NUMBER_RE = /[0-9]+/g;
/** Normalizes one log message into a pattern with placeholders substituted for dynamic content. */ function normalizeMessage(message) {
    return message.replace(UUID_RE, "<uuid>").replace(PATH_RE, "<path>").replace(OBJ_RE, "<obj>").replace(NUMBER_RE, "<n>");
}
/**
 * Groups log entries into patterns by (level, normalized message), sorted by
 * count descending, so a page full of near-identical lines collapses into
 * one row with an expandable list of the actual matching entries (which
 * still show whatever varied — the path, the UUID, the count — since
 * normalization only affects the grouping key, not what's displayed on
 * expand).
 */ function groupLogsByPattern(entries) {
    const groups = new Map();
    for (const entry of entries){
        const pattern = normalizeMessage(entry.message);
        const key = "".concat(entry.level, " ").concat(pattern);
        const existing = groups.get(key);
        if (existing) {
            existing.count += 1;
            existing.entries.push(entry);
        } else {
            groups.set(key, {
                key,
                level: entry.level,
                pattern,
                count: 1,
                entries: [
                    entry
                ]
            });
        }
    }
    return Array.from(groups.values()).sort((a, b)=>b.count - a.count);
}

// EXTERNAL MODULE: ./src/lib/analytics/index.ts
var analytics = __webpack_require__(10671);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Flogs%2FPatternsView.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA6VUy3LjIBC8-ys4xgdcQn6tyceksBhJlCVQDSh-bOXfUyDZFrLlZGtPsoceZrp7mMUHK9iB5SIhf2eE5BWcOGHvM0LMJ2BemSMnonXGR2qlaQmqKB0niQ_sRXYo0LRa0sxUBjn5FPhG6QfbwSXFy2YeUAYlIEUhVWvHkOV2gOGENSdiTaXkCAbz99nXbHFtloVm-4v3xjlTT-du49w05Eplm0qceWDsOxCVKjRVDmrLSQbaAfpwIRpOksV2jVD7_42QUunCxzYI9fAoa9F6DRqjrtm50Y5adYEH2mJ-O89FrarzGJG2o7Z56R3piP-g-4jxMiQdlXQlJ6zv1vOmtkSlD72bT69KQ58OhbbKKaN59zs36LkzGxVahUI3ACdonHDwtkskFHFP69u4xU34Ges7XS6uyjo4ORoM4gT9_I2cYJ0Tqx7-euD6ofyFhBOSJHfrjv1r2CRJxG7zM7vVjd3jTT3jgZBt0wBmwkJUZhvKTPo2QP6ZRtYxcjeNPMVIMY1kSQzdj5fLsVQOqG1EBpw0CPSIogkHBiXdI4gDJ-FDfeSlF4M62WuaI63XI9dkyL7P1fVx377p_bE_7I9gtlQIWfdMMlO1tR6skGWXOqgHzzfReOX87xL53YTkEyMbOaVN59Mgr5iWvHso_2z14PLy9YBftXHnCjhRTlQqi_JV7OgymPhkqdz3_bRU30rZY0cqBwAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Flogs_2FPatternsView_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA6VUy3LjIBC8_ys4xgdcQn6tyceksBhJlCVQDSh_bOXfUyDZFrLlZGtPsoceZrp7mMUHK9iB5SIhf2eE5BWcOGHvM0LMJ2BemSMnonXGR2qlaQmqKB0niQ_sRXYo0LRa0sxUBjn5FPhG6QfbwSXFy2YeUAYlIEUhVWvHkOV2gOGENSdiTaXkCAbz99nXbHFtloVm_4v3xjlTT_du49w05Eplm0qceWDsOxCVKjRVDmrLSQbaAfpwIRpOksV2jVD7_42QUunCxzYI9fAoa9F6DRqjrtm50Y5adYEH2mJ_O89FrarzGJG2o7Z56R3piP_g_4jxMiQdlXQlJ6zv1vOmtkSlD72bT69KQ58OhbbKKaN59zs36LkzGxVahUI3ACdonHDwtkskFHFP69u4xU34Ges7XS6uyjo4ORoM4gT9_I2cYJ0Tqx7_euD6ofyFhBOSJHfrjv1r2CRJxG7zM7vVjd3jTT3jgZBt0wBmwkJUZhvKTPo2QP6ZRtYxcjeNPMVIMY1kSQzdj5fLsVQOqG1EBpw0CPSIogkHBiXdI4gDJ_FDfeSlF4M62WuaI63XI9dkyL7P1fVx377p_bE_7I9gtlQIWfdMMlO1tR6skGWXOqgHzzfReOX87xL53YTkEyMbOaVN59Mgr5iWvHso_2z14PLy9YBftXHnCjhRTlQqi_JV7OgymPhkqdz3_bRU30rZY0cqBwAA_22_7D = __webpack_require__(95113);
;// ./src/components/logs/PatternsView.css.ts


var PatternsView_css_container = '_1g1k1fa0';
var count = '_1g1k1fa5';
var disclosure = '_1g1k1fa3';
var disclosureOpen = '_1g1k1fa4';
var empty = '_1g1k1fai';
var example = '_1g1k1fae';
var exampleMessage = '_1g1k1fag';
var exampleTimestamp = '_1g1k1faf';
var examples = '_1g1k1fad';
var examplesMore = '_1g1k1fah';
var level = '_1g1k1fa6';
var levelDebug = '_1g1k1fa7';
var levelError = '_1g1k1faa';
var levelInfo = '_1g1k1fa8';
var levelWarning = '_1g1k1fa9';
var pattern = '_1g1k1fab';
var placeholder = '_1g1k1fac';
var row = '_1g1k1fa1';
var rowHeader = '_1g1k1fa2';
;// ./src/components/logs/PatternsView.tsx
/* __next_internal_client_entry_do_not_use__ PatternsView auto */ 





const PLACEHOLDER_RE = /(<uuid>|<path>|<obj>|<n>)/g;
const LEVEL_STYLES = {
    DEBUG: levelDebug,
    INFO: levelInfo,
    WARN: levelWarning,
    ERROR: levelError
};
/** Renders a normalized pattern string, styling `<placeholder>` tokens distinctly from literal text. */ function PatternText(param) {
    let { pattern } = param;
    const parts = pattern.split(PLACEHOLDER_RE);
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(jsx_runtime.Fragment, {
        children: parts.map((part, i)=>PLACEHOLDER_RE.test(part) ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: "".concat(placeholder),
                children: part
            }, i) : /*#__PURE__*/ (0,jsx_runtime.jsx)(react.Fragment, {
                children: part
            }, i))
    });
}
/**
 * PatternsView — Datadog-style "Log Patterns" clustering for the app logs
 * page: groups the currently-loaded entries by normalized message (see
 * lib/logs/logPatterns.ts), most-frequent first. Expanding a row shows the
 * raw matching entries — including whatever the pattern's placeholders
 * normalized away (the actual path, UUID, or count).
 */ function PatternsView(param) {
    let { entries, maxExamplesPerPattern = 20 } = param;
    const { track } = (0,analytics/* useAnalytics */.st)();
    const [expandedKey, setExpandedKey] = (0,react.useState)(null);
    const groups = (0,react.useMemo)(()=>groupLogsByPattern(entries), [
        entries
    ]);
    if (groups.length === 0) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: "".concat(PatternsView_css_container),
            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: "".concat(empty),
                children: "No log entries loaded to cluster into patterns."
            })
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: "".concat(PatternsView_css_container),
        "data-testid": "patterns-view",
        children: groups.map((group)=>{
            const isOpen = expandedKey === group.key;
            var _LEVEL_STYLES_group_level;
            return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: "".concat(row),
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: "".concat(rowHeader),
                        role: "button",
                        tabIndex: 0,
                        "aria-expanded": isOpen,
                        onClick: ()=>{
                            track({
                                name: "log_pattern_toggled",
                                category: "user_action",
                                labels: {
                                    expanded: String(!isOpen)
                                }
                            });
                            setExpandedKey(isOpen ? null : group.key);
                        },
                        onKeyDown: (e)=>{
                            if (e.key === "Enter" || e.key === " ") {
                                e.preventDefault();
                                track({
                                    name: "log_pattern_toggled",
                                    category: "user_action",
                                    labels: {
                                        expanded: String(!isOpen)
                                    }
                                });
                                setExpandedKey(isOpen ? null : group.key);
                            }
                        },
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: "".concat(disclosure, " ").concat(isOpen ? disclosureOpen : ""),
                                children: "▶"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: "".concat(count),
                                children: group.count.toLocaleString()
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: "".concat(level, " ").concat((_LEVEL_STYLES_group_level = LEVEL_STYLES[group.level]) !== null && _LEVEL_STYLES_group_level !== void 0 ? _LEVEL_STYLES_group_level : ""),
                                children: group.level
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: "".concat(pattern),
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(PatternText, {
                                    pattern: group.pattern
                                })
                            })
                        ]
                    }),
                    isOpen && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: "".concat(examples),
                        children: [
                            group.entries.slice(0, maxExamplesPerPattern).map((entry)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: "".concat(example),
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: "".concat(exampleTimestamp),
                                            children: (0,datetime/* formatTimestampShort */.CC)(new Date(entry.timestamp))
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: "".concat(exampleMessage),
                                            children: entry.message
                                        })
                                    ]
                                }, entry.id)),
                            group.entries.length > maxExamplesPerPattern && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: "".concat(examplesMore),
                                children: [
                                    "…and ",
                                    (group.entries.length - maxExamplesPerPattern).toLocaleString(),
                                    " more"
                                ]
                            })
                        ]
                    })
                ]
            }, group.key);
        })
    });
}

// EXTERNAL MODULE: ./src/components/shared/LogViewer.tsx + 18 modules
var LogViewer = __webpack_require__(42692);
// EXTERNAL MODULE: ./src/components/ui/ActionBar.tsx + 1 modules
var ActionBar = __webpack_require__(93321);
// EXTERNAL MODULE: ./src/lib/analytics/usePageView.ts
var usePageView = __webpack_require__(90814);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Flogs%2Fpage.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA71WXY_iIBR991eQeVqTwZQ66ojZX7LZbGi5tey00AD1azP_fVOqDu22VWdcX2zkwr2cwzkXJsU-mJo8QH9GCHFhioztKUoy2K1GyH0xFxpiK5SkKFZZmcsqkoJYp5aiDdPfMN4I2BZKW1wPPyMSBHyTjquZagM6ydSWolRwDm51wTgXck0Rmcw05NVQxOK3tVal5DhWmdKn1L_IEg6hPsxdss5Q4EKJkhYnLBfZvj0hLMer0ftoUmMl3Vh_l8aKZI9jJS1IS5EpWAw4AruFetcsE2uJhYXcUBSDtKCr4ZzptZA4Utaq_APSuV7YXa8n25oVFJFmhqnLcCYtcBWOky4zt3D0REpz0BSRYoeMygRvzQJvFtaMi9K0E03nH0QbcYB_4lH_GYV1qNSmiqWQFR6-l4fhG9DPaW-FEqez-BobsYtbzaQRtX3aSFAwCY3PA00rtzg2LqF-9SU965aYp6Vemfpm_AThfeTcciiuy2x1tdnq18M1v906Qdt-iy5xXQOTsKswvPULi0R3MFXcp86rG96CJioujeNBlTYTEiiSSoK3uc79Z36W1zaPL5VJg8myl04n_YJpkPaLDg3_p0Mv8XjRw2RmnpH3xyftRkcPdKhzzuVV2fLxpdPN--vNG_VuwCD9haypmPAoFAs7i52FffM-rumQYMCxDaYjB6DqOhSRvrdMLiQ-vYaC1W2vqOsePffqtGdcscN1vBCs8i4Jz96LxiV8v-fSw2-XXvuz4YZzposP3K5Hngbwfqo-8etDs_6Rcb_NwA_OLMO1KL8_WV3C08_mqrrfv_8FZzzPQ_ALAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Flogs_2Fpage_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA71WXY_iIBR991eQeVqTwZQ66ojZX7LZbGi5tey00AD1azP_fVOqDu22VWdcX2zkwr2cwzkXJsU_mJo8QH9GCHFhioztKUoy2K1GyH0xFxpiK5SkKFZZmcsqkoJYp5aiDdPfMN4I2BZKW1wPPyMSBHyTjquZagM6ydSWolRwDm51wTgXck0Rmcw05NVQxOK3tVal5DhWmdKn1L_IEg6hPsxdss5Q4EKJkhYnLBfZvj0hLMer0ftoUmMl3Vh_l8aKZI9jJS1IS5EpWAw4AruFetcsE2uJhYXcUBSDtKCr4ZzptZA4Utaq_APSuV7YXa8n25oVFJFmhqnLcCYtcBWOky4zt3D0REpz0BSRYoeMygRvzQJvFtaMi9K0E03nH0QbcYB_4lH_GYV1qNSmiqWQFR6_l4fhG9DPaW_FEqez_BobsYtbzaQRtX3aSFAwCY3PA00rtzg2LqF_9SU965aYp6Vemfpm_AThfeTcciiuy2x1tdnq18M1v906Qdt_iy5xXQOTsKswvPULi0R3MFXcp86rG96CJioujeNBlTYTEiiSSoK3uc79Z36W1zaPL5VJg8myl04n_YJpkPaLDg3_p0Mv8XjRw2RmnpH3xyftRkcPdKhzzuVV2fLxpdPN_vNG_VuwCD9haypmPAoFAs7i52FffM_rumQYMCxDaYjB6DqOhSRvrdMLiQ_vYaC1W2vqOsePffqtGdcscN1vBCs8i4Jz96LxiV8v_fSw2_XXvuz4YZzposP3K5Hngbwfqo_8etDs_6Rcb_NwA_OLMO1KL8_WV3C08_mqrrfv_8FZzzPQ_ALAAA_22_7D = __webpack_require__(86436);
;// ./src/app/logs/page.css.ts


var page_css_container = 'py03sm0';
var error = 'py03sma';
var filterGroup = 'py03sm6';
var filters = 'py03sm5';
var footer = 'py03smc';
var header = 'py03sm1';
var headerActions = 'py03sm2';
var logsContainer = 'py03smb';
var refreshButton = 'py03sm4';
var page_css_select = 'py03sm7';
var shortcuts = 'py03smd';
var timezone = 'py03sm3';
var viewPane = 'py03sme';
var viewTab = 'py03sm8';
var viewTabActive = 'py03sm9 py03sm8';
;// ./src/app/logs/page.tsx
/* __next_internal_client_entry_do_not_use__ default auto */ 











function LogsPage() {
    var _TIME_RANGE_PRESETS_find;
    (0,usePageView/* usePageView */.O)();
    const { track } = (0,analytics/* useAnalytics */.st)();
    // Time range and limit are the two real, wired filters for this page —
    // LogViewer owns search/level filtering itself (see LogViewerToolbar,
    // rendered inside LogViewer) since those need to apply to the same fetch
    // it's already doing; duplicating them here would just be a second,
    // easy-to-desync copy of the same state.
    const [timeRange, setTimeRange] = (0,react.useState)(()=>{
        const preset = datetime/* TIME_RANGE_PRESETS */.jX.find((p)=>p.value === "1h");
        const range = (preset === null || preset === void 0 ? void 0 : preset.getRange()) || {
            start: new Date(Date.now() - 60 * 60 * 1000),
            end: new Date()
        };
        return {
            ...range,
            preset: "1h"
        };
    });
    const [limit, setLimit] = (0,react.useState)(200);
    const [viewMode, setViewMode] = (0,react.useState)("table");
    // Lifted from LogViewer via onStateChange — this is the single source of
    // truth for what's actually displayed, used by Export, the Patterns view,
    // and the footer count.
    const [logs, setLogs] = (0,react.useState)([]);
    const [rawEntries, setRawEntries] = (0,react.useState)([]);
    const [serverTotalCount, setServerTotalCount] = (0,react.useState)(0);
    const [lastRefresh, setLastRefresh] = (0,react.useState)(null);
    const [logsError, setLogsError] = (0,react.useState)(null);
    const logViewerRef = (0,react.useRef)(null);
    const handleLogViewerStateChange = (0,react.useCallback)((state)=>{
        setLogs(state.logs);
        setRawEntries(state.rawEntries);
        setServerTotalCount(state.totalCount);
        setLastRefresh(state.lastRefresh);
        setLogsError(state.error);
    }, []);
    // "all" means no bound — everything else is a real time-bounded window.
    const activeTimeRange = timeRange.preset === "all" ? undefined : {
        start: timeRange.start,
        end: timeRange.end
    };
    const removeTimeRangeFilter = ()=>{
        const preset = datetime/* TIME_RANGE_PRESETS */.jX.find((p)=>p.value === "all");
        if (preset) {
            const range = preset.getRange();
            setTimeRange({
                ...range,
                preset: "all"
            });
        }
    };
    const hasActiveFilters = Boolean(timeRange.preset && timeRange.preset !== "all");
    // LogViewer already implements /, Escape, g, G, =, ?, Cmd+F, and j/k
    // internally (see its own handleKeyDown) for everything scoped to the log
    // list itself — only page-level actions belong here.
    (0,react.useEffect)(()=>{
        const isInputFocused = ()=>{
            const active = document.activeElement;
            return active instanceof HTMLInputElement || active instanceof HTMLTextAreaElement || active instanceof HTMLSelectElement;
        };
        const handleKeyDown = (e)=>{
            if (e.key === "r" && !isInputFocused()) {
                var _logViewerRef_current;
                e.preventDefault();
                (_logViewerRef_current = logViewerRef.current) === null || _logViewerRef_current === void 0 ? void 0 : _logViewerRef_current.refresh();
            }
        };
        document.addEventListener("keydown", handleKeyDown);
        return ()=>document.removeEventListener("keydown", handleKeyDown);
    }, []);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("main", {
        id: "main-content",
        className: page_css_container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("header", {
                className: header,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h1", {
                        children: "Application Logs"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)(ActionBar/* ActionBar */.E, {
                        scroll: true,
                        compact: true,
                        gap: "md",
                        className: headerActions,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(TimeRangePicker, {
                                value: timeRange,
                                onChange: setTimeRange
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: timezone,
                                title: "Your local timezone",
                                children: (0,datetime/* getUserTimezone */.dB)()
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                onClick: ()=>{
                                    var _logViewerRef_current;
                                    track({
                                        name: "logs_refresh_clicked",
                                        category: "user_action"
                                    });
                                    (_logViewerRef_current = logViewerRef.current) === null || _logViewerRef_current === void 0 ? void 0 : _logViewerRef_current.refresh();
                                },
                                className: refreshButton,
                                "aria-label": "Refresh logs",
                                title: lastRefresh ? "Last updated: ".concat((0,datetime/* formatRelativeTime */.fw)(lastRefresh.getTime())) : "Not yet refreshed",
                                children: "\uD83D\uDD04 Refresh"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(ExportButton, {
                                logs: rawEntries,
                                disabled: rawEntries.length === 0
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)(ActionBar/* ActionBar */.E, {
                scroll: true,
                compact: true,
                gap: "md",
                className: filters,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: filterGroup,
                        role: "tablist",
                        "aria-label": "Log view",
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                role: "tab",
                                "aria-selected": viewMode === "table",
                                className: viewMode === "table" ? viewTabActive : viewTab,
                                onClick: ()=>{
                                    track({
                                        name: "logs_view_mode_changed",
                                        category: "user_action",
                                        labels: {
                                            mode: "table"
                                        }
                                    });
                                    setViewMode("table");
                                },
                                children: "Table"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                role: "tab",
                                "aria-selected": viewMode === "patterns",
                                className: viewMode === "patterns" ? viewTabActive : viewTab,
                                onClick: ()=>{
                                    track({
                                        name: "logs_view_mode_changed",
                                        category: "user_action",
                                        labels: {
                                            mode: "patterns"
                                        }
                                    });
                                    setViewMode("patterns");
                                },
                                children: "Patterns"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: filterGroup,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                htmlFor: "limit",
                                children: "Limit:"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                                id: "limit",
                                value: limit,
                                onChange: (e)=>setLimit(Number(e.target.value)),
                                className: page_css_select,
                                "aria-label": "Results per page",
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                        value: "50",
                                        children: "50"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                        value: "100",
                                        children: "100"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                        value: "200",
                                        children: "200"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                        value: "500",
                                        children: "500"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                        value: "1000",
                                        children: "1000"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                        value: "2000",
                                        children: "2000"
                                    })
                                ]
                            })
                        ]
                    })
                ]
            }),
            hasActiveFilters && /*#__PURE__*/ (0,jsx_runtime.jsx)(FilterPills, {
                onClearAll: removeTimeRangeFilter,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(FilterPill, {
                    label: "Time",
                    value: ((_TIME_RANGE_PRESETS_find = datetime/* TIME_RANGE_PRESETS */.jX.find((p)=>p.value === timeRange.preset)) === null || _TIME_RANGE_PRESETS_find === void 0 ? void 0 : _TIME_RANGE_PRESETS_find.label) || "Custom",
                    onRemove: removeTimeRangeFilter
                })
            }),
            logsError && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: error,
                role: "alert",
                "data-testid": "logs-error-banner",
                children: [
                    "Error loading logs: ",
                    logsError
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: viewPane,
                "data-hidden": viewMode !== "table",
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: logsContainer,
                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(LogViewer/* LogViewer */.r, {
                        ref: logViewerRef,
                        source: "app",
                        timeRange: activeTimeRange,
                        limit: limit,
                        onStateChange: handleLogViewerStateChange
                    })
                })
            }),
            viewMode === "patterns" && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: logsContainer,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(PatternsView, {
                    entries: logs
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("footer", {
                className: footer,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        children: [
                            "Showing ",
                            logs.length,
                            " of ",
                            serverTotalCount,
                            " log entries"
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: shortcuts,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("kbd", {
                                children: "R"
                            }),
                            " Refresh • ",
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("kbd", {
                                children: "/"
                            }),
                            " Search • ",
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("kbd", {
                                children: "?"
                            }),
                            " All shortcuts"
                        ]
                    })
                ]
            })
        ]
    });
}


/***/ }),

/***/ 65916:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 86436:
/***/ (() => {

// extracted by mini-css-extract-plugin

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

/***/ 90653:
/***/ (() => {

// extracted by mini-css-extract-plugin

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

/***/ 95113:
/***/ (() => {

// extracted by mini-css-extract-plugin

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
/******/ __webpack_require__.O(0, [1869,852,4202,9353,8362,30,4713,8165,3575,2692,9691,3516,7358], () => (__webpack_exec__(31123)));
/******/ var __webpack_exports__ = __webpack_require__.O();
/******/ _N_E = __webpack_exports__;
/******/ }
]);
//# sourceMappingURL=page-21b84635bf7ce6a2.js.map