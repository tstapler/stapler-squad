(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[6076],{

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

/***/ 4047:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 74116));


/***/ }),

/***/ 20550:
/***/ (() => {

// extracted by mini-css-extract-plugin

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

/***/ 74116:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  "default": () => (/* binding */ ErrorsPage)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect-web@2.1.1+0149f91553ca585e/node_modules/@connectrpc/connect-web/dist/esm/connect-transport.js + 8 modules
var connect_transport = __webpack_require__(46564);
// EXTERNAL MODULE: ./src/gen/session/v1/session_pb.ts
var session_pb = __webpack_require__(23575);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/timestamp.js
var timestamp = __webpack_require__(28765);
// EXTERNAL MODULE: ./src/lib/config.ts
var config = __webpack_require__(51202);
// EXTERNAL MODULE: ./src/lib/utils/datetime.ts
var datetime = __webpack_require__(3335);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Ferrors%2FErrorDashboard.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA8VY23LjKBB9z1fwslWTqiUl2ZtMQj5mCqOWRYKAIGTZ3tp_3wJZEnZ0wU6m5sUX0TTdp09f0AOwZ1q8JejfO4QyXmlBDwTlAvavd8h_44wbYJYrSRBToi6lWymAbwtL0I6aHxjvODRaGYvbx3-jNEmyXXHvJNUOTC5UQ1DBswz8bk2zjMttt_1X-gLHlTmuU79jQ9n71qhaZpgpocyl2JOXGl1K_FKupMU5Lbk4XAqsdvevd__dPbRup-Nuv9WV5fkBMyUtSEtQpSkDvAHbQOsAFXwrMbdQVgQxkBaMe1xSs-USb5S1qpzwzmPaGKoJcp_u0db9ubDzGNq58nZ6typ-hE-aYXC7OQtML1EsYdaaTlASHLseh2fC-UUv_vmqunVo6hTKrcwEw1bH-3H5Jc79bKWUycBgQzNeV59ODmUISvUeVUrw7EIMQkgev4zwYQj9KDU204Fv-chqU7k1rXh3Tl2BwRUIYJYgqSQEFj95i0c2UeYMHUevDH3-6TWcgiggtwTR2qp5N-i0G6uoAD6f02Kl92NcXQrxS-jIc5uVAvYEpWGpw4fBpW-nzIs_teGZLYirs38FpzAlBNUV-Ertfy2To1dMvWKtKt4W-8py9n5wCqzSvjAgdMRcZr2_MUnTq9-06q_MSwt7i306EOS4EsOSG-vgajjQGiqrXJmSoFprMIy2SAqw1qWGpsw7kTwkj1AGAehq0lwkEWoKbsFrAZdfbRvogWIeqEiFeQgxI4JWFrOCi2xMyUUqM1I4xraSEenTb2yVK4eCPTgUHoNFuCnOOzCWMyq6WFulZxtWf1w-tMaJjl8vVMiZ0pLG1ZY0XSwuh4ji8jRNDlcv9_iU9Ksk0fup0crzd1gAIbiuePUathkuBZeAN0Kx9wDKbVva52aE3ob1jTbMU78Ygnl9_u6HOO-o4VRaLOsSDGcEWbqpBTXuQYQZfBqIVRSZJmIYY1tvxNtvJfZlPG_l1DyQ7-fjzYl3C1POd_TorzWJbUSVFlFl81QZpotLr6-M0rebVleH2mQ7JMx6oC5O7PrDMI90_2cyYWQQ7OOXXDurMr_uOdLdL9PQ4KBfLTcGfd6Hpsm0AHk5DfnH_RhaMUV-BLavE9ZPLafh7dIrlDykj2F50Vc0f3mGK8l4RTcCxqaAwTOp3NwmVANZsPljdjg4fMPlbGn2uu4mFDO9L929rgrLx60zWdyWs4m8umlSO8nj7lLQq7O_v2GdFTRtAHedtVEmwxsD9J0g_-W4F_8uaYk6efT0No1m13C7yvap44a3xrG3MfVcuNbrT9eloaPeNMmc3Q13s2cvNbnoPvkd6bb8dqj3qvkDiP4POrr1qHEVAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Ferrors_2FErrorDashboard_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA8VY23LjKBB9z1fwslWTqiUl2ZtMQj5mCqOWRYKAIGTZ3tp_3wJZEnZ0wU6m5sUX0TTdp09f0AOwZ1q8JejfO4QyXmlBDwTlAvavd8h_44wbYJYrSRBToi6lWymAbwtL0I6aHxjvODRaGYvbx3_jNEmyXXHvJNUOTC5UQ1DBswz8bk2zjMttt_1X_gLHlTmuU79jQ9n71qhaZpgpocyl2JOXGl1K_FKupMU5Lbk4XAqsdvevd_dPbRup_Nuv9WV5fkBMyUtSEtQpSkDvAHbQOsAFXwrMbdQVgQxkBaMe1xSs_USb5S1qpzwzmPaGKoJcp_u0db9ubDzGNq58nZ6typ_hE_aYXC7OQtML1EsYdaaTlASHLseh2fC_UUv_vmqunVo6hTKrcwEw1bH_3H5Jc79bKWUycBgQzNeV59ODmUISvUeVUrw7EIMQkgev4zwYQj9KDU204Fv_chqU7k1rXh3Tl2BwRUIYJYgqSQEFj95i0c2UeYMHUevDH3_6TWcgiggtwTR2qp5N_i0G6uoAD6f02Kl92NcXQrxS_jIc5uVAvYEpWGpw4fBpW_nzIs_teGZLYirs38FpzAlBNUV_Ertfy2To1dMvWKtKt4W_8py9n5wCqzSvjAgdMRcZr2_MUnTq9_06q_MSwt7i306EOS4EsOSG_vgajjQGiqrXJmSoFprMIy2SAqw1qWGpsw7kTwkj1AGAehq0lwkEWoKbsFrAZdfbRvogWIeqEiFeQgxI4JWFrOCi2xMyUUqM1I4xraSEenTb2yVK4eCPTgUHoNFuCnOOzCWMyq6WFulZxtWf1w_tMaJjl8vVMiZ0pLG1ZY0XSwuh4ji8jRNDlcv9_iU9Ksk0fup0crzd1gAIbiuePUathkuBZeAN0Kx9wDKbVva52aE3ob1jTbMU78Ygnl9_u6HOO_o4VRaLOsSDGcEWbqpBTXuQYQZfBqIVRSZJmIYY1tvxNtvJfZlPG_l1DyQ7_fjzYl3C1POd_TorzWJbUSVFlFl81QZpotLr6_M0rebVleH2mQ7JMx6oC5O7PrDMI90_2cyYWQQ7OOXXDurMr_uOdLdL9PQ4KBfLTcGfd6Hpsm0AHk5DfnH_RhaMUV_BLavE9ZPLafh7dIrlDykj2F50Vc0f3mGK8l4RTcCxqaAwTOp3NwmVANZsPljdjg4fMPlbGn2uu4mFDO9L929rgrLx60zWdyWs4m8umlSO8nj7lLQq7O_v2GdFTRtAHedtVEmwxsD9J0g_W4F_8uaYk6efT0No1m13C7yvap44a3xrG3MfVcuNbrT9eloaPeNMmc3Q13s2cvNbnoPvkd6bb8dqj3qvkDiP4POrr1qHEVAAA_22_7D = __webpack_require__(20550);
;// ./src/components/errors/ErrorDashboard.css.ts

var acknowledgeButton = 'ec8ahjp';
var actionCell = 'ec8ahjn';
var container = 'ec8ahj0';
var count = 'ec8ahjh';
var countBadge = 'ec8ahj7';
var emptyState = 'ec8ahju';
var errorState = 'ec8ahjv';
var errorType = 'ec8ahjf';
var expandButton = 'ec8ahjo';
var filterCheckbox = 'ec8ahj6';
var filterLabel = 'ec8ahj5';
var filterRow = 'ec8ahj4';
var header = 'ec8ahj1';
var headerActions = 'ec8ahj3';
var loadingState = 'ec8ahjw';
var message = 'ec8ahjg';
var procedure = 'ec8ahjj';
var refreshButton = 'ec8ahjq';
var stackTrace = 'ec8ahjt';
var stackTraceCell = 'ec8ahjs';
var stackTraceRow = 'ec8ahjr';
var statusAcknowledged = 'ec8ahjm';
var statusActive = 'ec8ahjl';
var statusBadge = 'ec8ahjk';
var table = 'ec8ahj9';
var tableWrapper = 'ec8ahj8';
var td = 'ec8ahje';
var th = 'ec8ahjb';
var thead = 'ec8ahja';
var ErrorDashboard_css_timestamp = 'ec8ahji';
var title = 'ec8ahj2';
var tr = 'ec8ahjc';
var trAcknowledged = 'ec8ahjd';
;// ./src/components/errors/ErrorDashboard.tsx
/* __next_internal_client_entry_do_not_use__ ErrorDashboard auto */ 
// +feature: error-dashboard








function ErrorDashboard() {
    const [errors, setErrors] = (0,react.useState)([]);
    const [loading, setLoading] = (0,react.useState)(true);
    const [fetchError, setFetchError] = (0,react.useState)(null);
    const [includeAcknowledged, setIncludeAcknowledged] = (0,react.useState)(false);
    const [expandedRow, setExpandedRow] = (0,react.useState)(null);
    const [acknowledging, setAcknowledging] = (0,react.useState)(new Set());
    const clientRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        const transport = (0,connect_transport/* createConnectTransport */.w)({
            baseUrl: (0,config/* getApiBaseUrl */.h)()
        });
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, transport);
    }, []);
    const fetchErrors = (0,react.useCallback)(async ()=>{
        if (!clientRef.current) return;
        setFetchError(null);
        setLoading(true);
        try {
            const resp = await clientRef.current.listErrors({
                includeAcknowledged
            });
            setErrors(resp.errors);
        } catch (e) {
            setFetchError(e instanceof Error ? e.message : String(e));
        } finally{
            setLoading(false);
        }
    }, [
        includeAcknowledged
    ]);
    (0,react.useEffect)(()=>{
        fetchErrors();
    }, [
        fetchErrors
    ]);
    const handleAcknowledge = (0,react.useCallback)(async (fingerprint)=>{
        if (!clientRef.current) return;
        setAcknowledging((prev)=>new Set(prev).add(fingerprint));
        try {
            await clientRef.current.acknowledgeError({
                fingerprint
            });
            setErrors((prev)=>prev.map((e)=>e.fingerprint === fingerprint ? {
                        ...e,
                        acknowledged: true
                    } : e));
        } catch (e) {
            setFetchError(e instanceof Error ? e.message : String(e));
        } finally{
            setAcknowledging((prev)=>{
                const next = new Set(prev);
                next.delete(fingerprint);
                return next;
            });
        }
    }, []);
    const formatLastSeen = (record)=>{
        if (!record.lastSeen) return "—";
        return (0,datetime/* formatTimestamp */.Ey)((0,timestamp/* timestampDate */.FP)(record.lastSeen));
    };
    const activeCount = errors.filter((e)=>!e.acknowledged).length;
    const displayedErrors = includeAcknowledged ? errors : errors.filter((e)=>!e.acknowledged);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: header,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h1", {
                        className: title,
                        children: "Error Dashboard"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: headerActions,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: refreshButton,
                            onClick: fetchErrors,
                            children: "Refresh"
                        })
                    })
                ]
            }),
            fetchError && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: errorState,
                children: fetchError
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: filterRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                        className: filterLabel,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                type: "checkbox",
                                className: filterCheckbox,
                                checked: includeAcknowledged,
                                onChange: (e)=>setIncludeAcknowledged(e.target.checked)
                            }),
                            "Show acknowledged"
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: countBadge,
                        children: [
                            activeCount,
                            " active error",
                            activeCount !== 1 ? "s" : ""
                        ]
                    })
                ]
            }),
            loading ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: loadingState,
                children: "Loading errors…"
            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: tableWrapper,
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("table", {
                    className: table,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("thead", {
                            className: thead,
                            children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: th
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: th,
                                        children: "Error Type"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: th,
                                        children: "Message"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: th,
                                        children: "Count"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: th,
                                        children: "Last Seen"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: th,
                                        children: "Procedure"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: th,
                                        children: "Status"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: th
                                    })
                                ]
                            })
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("tbody", {
                            children: displayedErrors.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("tr", {
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                    colSpan: 8,
                                    className: emptyState,
                                    children: includeAcknowledged ? "No errors recorded." : "No active errors. 🎉"
                                })
                            }) : displayedErrors.map((err)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)(react.Fragment, {
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                            className: "".concat(tr, " ").concat(err.acknowledged ? trAcknowledged : ""),
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: td,
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                        className: expandButton,
                                                        onClick: ()=>setExpandedRow(expandedRow === err.fingerprint ? null : err.fingerprint),
                                                        title: expandedRow === err.fingerprint ? "Collapse stack trace" : "Expand stack trace",
                                                        children: expandedRow === err.fingerprint ? "▾" : "▸"
                                                    })
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: td,
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: errorType,
                                                        title: err.errorType,
                                                        children: err.errorType || "unknown"
                                                    })
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: td,
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: message,
                                                        title: err.message,
                                                        children: err.message || "—"
                                                    })
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: td,
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: count,
                                                        children: err.occurrenceCount.toLocaleString()
                                                    })
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: td,
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: ErrorDashboard_css_timestamp,
                                                        children: formatLastSeen(err)
                                                    })
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: td,
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: procedure,
                                                        title: err.rpcProcedure,
                                                        children: err.rpcProcedure || "—"
                                                    })
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: td,
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: "".concat(statusBadge, " ").concat(err.acknowledged ? statusAcknowledged : statusActive),
                                                        children: err.acknowledged ? "Acknowledged" : "Active"
                                                    })
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: "".concat(td, " ").concat(actionCell),
                                                    children: !err.acknowledged && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                        className: acknowledgeButton,
                                                        onClick: ()=>handleAcknowledge(err.fingerprint),
                                                        disabled: acknowledging.has(err.fingerprint),
                                                        children: acknowledging.has(err.fingerprint) ? "…" : "Acknowledge"
                                                    })
                                                })
                                            ]
                                        }, err.fingerprint),
                                        expandedRow === err.fingerprint && /*#__PURE__*/ (0,jsx_runtime.jsx)("tr", {
                                            className: stackTraceRow,
                                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                colSpan: 8,
                                                className: stackTraceCell,
                                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("pre", {
                                                    className: stackTrace,
                                                    children: err.stackTrace || "No stack trace available."
                                                })
                                            })
                                        }, "".concat(err.fingerprint, "-trace"))
                                    ]
                                }, err.fingerprint))
                        })
                    ]
                })
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/analytics/usePageView.ts
var usePageView = __webpack_require__(90814);
;// ./src/app/errors/page.tsx
/* __next_internal_client_entry_do_not_use__ default auto */ 


function ErrorsPage() {
    (0,usePageView/* usePageView */.O)();
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(ErrorDashboard, {});
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
/******/ __webpack_require__.O(0, [1869,852,4202,9353,8165,3575,9691,3516,7358], () => (__webpack_exec__(4047)));
/******/ var __webpack_exports__ = __webpack_require__.O();
/******/ _N_E = __webpack_exports__;
/******/ }
]);
//# sourceMappingURL=page-9f78f63e54c0954f.js.map