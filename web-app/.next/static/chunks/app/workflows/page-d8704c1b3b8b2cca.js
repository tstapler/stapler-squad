(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[7143],{

/***/ 3542:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   C: () => (/* binding */ fuzzyMatchModels),
/* harmony export */   i: () => (/* binding */ getModelLabel)
/* harmony export */ });
/* harmony import */ var fuse_js__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(66000);
/* harmony import */ var _programs__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(49051);


// Search both the human label ("Sonnet (latest)") and the raw value
// ("claude-sonnet-4-6") so typos against either surface a result.
const FUSE_OPTIONS = {
    keys: [
        {
            name: "label",
            weight: 0.6
        },
        {
            name: "value",
            weight: 0.4
        }
    ],
    includeScore: true,
    threshold: 0.4,
    minMatchCharLength: 1,
    ignoreLocation: true
};
const modelFuse = new fuse_js__WEBPACK_IMPORTED_MODULE_1__/* ["default"] */ .A(_programs__WEBPACK_IMPORTED_MODULE_0__/* .MODEL_AUTOCOMPLETE_OPTIONS */ .kg, FUSE_OPTIONS);
/**
 * Fuzzy/typo-tolerant filter for the Workflow model field's AutocompleteInput.
 * `suggestions` is the value list passed to AutocompleteInput (kept as the
 * signature contract so it can be used directly as a `filterFn`); results are
 * ranked best-match-first by Fuse.js and restricted to that value list.
 */ function fuzzyMatchModels(query, suggestions) {
    const trimmed = query.trim();
    if (!trimmed) return suggestions;
    const allowed = new Set(suggestions);
    return modelFuse.search(trimmed).map((r)=>r.item.value).filter((value)=>allowed.has(value));
}
/** Display label for a model suggestion value; falls back to the raw value. */ function getModelLabel(value) {
    var _MODEL_AUTOCOMPLETE_OPTIONS_find;
    var _MODEL_AUTOCOMPLETE_OPTIONS_find_label;
    return (_MODEL_AUTOCOMPLETE_OPTIONS_find_label = (_MODEL_AUTOCOMPLETE_OPTIONS_find = _programs__WEBPACK_IMPORTED_MODULE_0__/* .MODEL_AUTOCOMPLETE_OPTIONS */ .kg.find((o)=>o.value === value)) === null || _MODEL_AUTOCOMPLETE_OPTIONS_find === void 0 ? void 0 : _MODEL_AUTOCOMPLETE_OPTIONS_find.label) !== null && _MODEL_AUTOCOMPLETE_OPTIONS_find_label !== void 0 ? _MODEL_AUTOCOMPLETE_OPTIONS_find_label : value;
}


/***/ }),

/***/ 20188:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 65409));


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

/***/ 25562:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  U: () => (/* binding */ SlashCommandDropdown)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fui%2FSlashCommandDropdown.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA62UwXKjMAyG73kKHzcHZzC0aeM-TMfBCmhqbI9tGmCn796xk7RAyqTd7hFZI33_j6TNM9sNPRuKjPxdEaLQB-pDr4ATbTQ8rQhphKtQc5LFDyukRF1xcme7U6QRHa0Bqzpwkm8z28WgeQV3UOZIe05EG0yM7Y2T4OjehGAaTpjtiDcKJXkV7g-lz2wHQ-4GWKdkUb5UzrRa8tn7w_pp9bbaXLhZ4pborRI9JwcFCUAorDTFAI3npAQdwMVwJey8Xj6sJ8KuXmeRgqX8snXeOE6swUvx1oOjHhSU4eLeGJTX0ZWEu6yOzeTlP8wvUv7B6EA9DjBPL_YJPrpEfe1Qv5z_6xFlqDlhm_zeQRMjAbpAk41jA0ujouqZSVOGu0-Gg2hQ9Vf57YniBuWXvbIFAQ1qehaxSwpGQPffMuXLduyjHSdsPNmc1Cgl6A-rPh9AKbQeffK1xgDUW1GmjTo6YSdo2xtoYhktX3DifxCe1vrsZ_5Y1hPoh99CL8-zXI9OhRMSW39Vfjtd2HhIZk36BWtuzeRI42PSuHAVimzhKvzTjOVjzd85j2_vyPSCq7YFAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FSlashCommandDropdown_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA62UwXKjMAyG73kKHzcHZzC0aeM_TMfBCmhqbI9tGmCn796xk7RAyqTd7hFZI33_j6TNM9sNPRuKjPxdEaLQB_pDr4ATbTQ8rQhphKtQc5LFDyukRF1xcme7U6QRHa0Bqzpwkm8z28WgeQV3UOZIe05EG0yM7Y2T4OjehGAaTpjtiDcKJXkV7g_lz2wHQ_4GWKdkUb5UzrRa8tn7w_pp9bbaXLhZ4pborRI9JwcFCUAorDTFAI3npAQdwMVwJey8Xj6sJ8KuXmeRgqX8snXeOE6swUvx1oOjHhSU4eLeGJTX0ZWEu6yOzeTlP8wvUv7B6EA9DjBPL_YJPrpEfe1Qv5z_6xFlqDlhm_zeQRMjAbpAk41jA0ujouqZSVOGu0_Gg2hQ9Vf57YniBuWXvbIFAQ1qehaxSwpGQPffMuXLduyjHSdsPNmc1Cgl6A_rPh9AKbQeffK1xgDUW1GmjTo6YSdo2xtoYhktX3DifxCe1vrsZ_5Y1hPoh99CL8_zXI9OhRMSW39Vfjtd2HhIZk36BWtuzeRI42PSuHAVimzhKvzTjOVjzd85j2_vyPSCq7YFAAA_22_7D = __webpack_require__(5475);
;// ./src/components/ui/SlashCommandDropdown.css.ts


var description = '_19zy1z36';
var dropdown = '_19zy1z30';
var empty = '_19zy1z38';
var icon = '_19zy1z33';
var item = '_19zy1z31';
var itemSelected = '_19zy1z32';
var SlashCommandDropdown_css_name = '_19zy1z34';
var sourceBadge = '_19zy1z37';
var title = '_19zy1z35';
;// ./src/components/ui/SlashCommandDropdown.tsx
/* __next_internal_client_entry_do_not_use__ SlashCommandDropdown auto */ 


const SOURCE_LABELS = {
    project: "proj",
    user: "user",
    builtin: "built-in"
};
/**
 * Dropdown listing matching slash commands when the user types /command.
 * Follows the same controlled-index pattern as AtCommandDropdown.
 *
 * Usage:
 *   const { isActive, suggestions, complete, wordStart, wordEnd } =
 *     useSlashCommandSuggestions(value, cursorPos, commands);
 *   <SlashCommandDropdown suggestions={suggestions} selectedIndex={idx}
 *     onSelect={cmd => { const { newValue, newCursorPos } = complete(value, cmd); ... }} />
 */ function SlashCommandDropdown(param) {
    let { id = "slash-command-listbox", suggestions, selectedIndex, onSelect } = param;
    const selectedRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        var _selectedRef_current;
        (_selectedRef_current = selectedRef.current) === null || _selectedRef_current === void 0 ? void 0 : _selectedRef_current.scrollIntoView({
            block: "nearest"
        });
    }, [
        selectedIndex
    ]);
    if (suggestions.length === 0) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: empty,
            role: "status",
            "aria-live": "polite",
            children: "No matching commands"
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
        id: id,
        className: dropdown,
        role: "listbox",
        "aria-label": "Slash command suggestions",
        children: suggestions.map((cmd, i)=>{
            var _SOURCE_LABELS_cmd_source;
            return /*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                id: "".concat(id, "-option-").concat(i),
                ref: i === selectedIndex ? selectedRef : null,
                className: [
                    item,
                    i === selectedIndex ? itemSelected : ""
                ].filter(Boolean).join(" "),
                role: "option",
                "aria-selected": i === selectedIndex,
                onMouseDown: (e)=>{
                    e.preventDefault();
                    onSelect(cmd);
                },
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: icon,
                        "aria-hidden": "true",
                        children: "/"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: SlashCommandDropdown_css_name,
                        children: [
                            "/",
                            cmd.name
                        ]
                    }),
                    cmd.title && cmd.title !== cmd.name && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: title,
                        children: cmd.title
                    }),
                    cmd.description && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: description,
                        children: cmd.description
                    }),
                    cmd.source && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: sourceBadge,
                        children: (_SOURCE_LABELS_cmd_source = SOURCE_LABELS[cmd.source]) !== null && _SOURCE_LABELS_cmd_source !== void 0 ? _SOURCE_LABELS_cmd_source : cmd.source
                    })
                ]
            }, cmd.name);
        })
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

/***/ 49179:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   d: () => (/* binding */ useSlashCommands)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(66144);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(23575);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(21694);




/**
 * Fetches available slash commands for a given target directory.
 * Merges project (.claude/commands/), user (~/.claude/commands/), and built-in commands.
 * Re-fetches whenever targetDirectory changes.
 */ function useSlashCommands(targetDirectory) {
    const [commands, setCommands] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)([]);
    const [isLoading, setIsLoading] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(false);
    const abortRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        var _abortRef_current;
        (_abortRef_current = abortRef.current) === null || _abortRef_current === void 0 ? void 0 : _abortRef_current.abort();
        const abort = new AbortController();
        abortRef.current = abort;
        setIsLoading(true);
        const client = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_2__/* .getConnectTransport */ .qW)());
        client.listSlashCommands({
            targetDirectory
        }, {
            signal: abort.signal
        }).then((resp)=>{
            if (!abort.signal.aborted) {
                setCommands(resp.commands);
            }
        }).catch(()=>{
        /* ignore abort errors */ }).finally(()=>{
            if (!abort.signal.aborted) {
                setIsLoading(false);
            }
        });
        return ()=>{
            abort.abort();
        };
    }, [
        targetDirectory
    ]);
    return {
        commands,
        isLoading
    };
}


/***/ }),

/***/ 55963:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  P: () => (/* binding */ AutocompleteInput)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fui%2FAutocompleteInput.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA81V247aMBB95yumD5VAxZETspR1X_onlRMbYuHY1thZslT771UcsoTLZrfaPlSAZNlzOXPmzJDYfIV-Q-H3DMBZr4KyhgFKzYN6kj9mAAclQsUgpfTrj9nLLBk8lHFNiH4XFgCOC6HMjgFN1tkDyhposvneHbrXwqKQyCB1LXirlYAnjnNCfqWP8pjhUS7OVgS5UI1nsHZtd7u1JhCvjrKL_bh6jRmQmwH6ybO02iLQJPNLKGxLfMWFPcSLGJ-X-x3axgh2lT_lEUD0v36ji1sG2NaWjY882CZoZSQDY40cVXE3Vn2qc4DGgMbPyrWAu4LP6RLSNFtClq-WQJP0Tu5EIlqMuadSpfdw974j9BNQstXjEtab_ncfChPK80JL0TPheKnCc5RAZLNB3-EyNhCutT1IMd0FMU6RfqRC-KJqZzFwE0au2ZWweeGtbkLsTrCOQcl1Oe-UC98gd21sipbbwIB2R1S7ajjXvCWV7C-ynPaStE8St9oeyDMD3gQ7XdZ68dkRuOxS7truAUg6Us3pm6SLJVDIXBvN7prQHpBWPhAfnvVIuzXHnTKn0kcjnQ1DdyTKCNnGuadjyhkjB1nsVSC-RKt1wXG8JjZdIZPWJCAv933TJxVyQ1L-gdhVUxfvxK4-E5tVnSbeyZCN9b3qRTq5NocJclaZIPEtfFe78DX_6z5MH_zkentryZ7Bfqi-i_nN_8b44R9O7IjS8__P_zKcb7YhvW3DWQdBtt0KVTvDoJSDFG5m8WctheIwdyi3En3ffuLLStaSgeC4X0SeLzo0RU6vjZfZyx9GB_CzMggAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FAutocompleteInput_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81V247aMBB95yumD5VAxZETspR1X_onlRMbYuHY1thZslT771UcsoTLZrfaPlSAZNlzOXPmzJDYfIV_Q_H3DMBZr4KyhgFKzYN6kj9mAAclQsUgpfTrj9nLLBk8lHFNiH4XFgCOC6HMjgFN1tkDyhposvneHbrXwqKQyCB1LXirlYAnjnNCfqWP8pjhUS7OVgS5UI1nsHZtd7u1JhCvjrKL_bh6jRmQmwH6ybO02iLQJPNLKGxLfMWFPcSLGJ_X_x3axgh2lT_lEUD0v36ji1sG2NaWjY882CZoZSQDY40cVXE3Vn2qc4DGgMbPyrWAu4LP6RLSNFtClq_WQJP0Tu5EIlqMuadSpfdw974j9BNQstXjEtab_ncfChPK80JL0TPheKnCc5RAZLNB3_EyNhCutT1IMd0FMU6RfqRC_KJqZzFwE0au2ZWweeGtbkLsTrCOQcl1Oe_UC98gd21sipbbwIB2R1S7ajjXvCWV7C_ynPaStE8St9oeyDMD3gQ7XdZ68dkRuOxS7truAUg6Us3pm6SLJVDIXBvN7prQHpBWPhAfnvVIuzXHnTKn0kcjnQ1DdyTKCNnGuadjyhkjB1nsVSC_RKt1wXG8JjZdIZPWJCAv933TJxVyQ1L_gdhVUxfvxK4_E5tVnSbeyZCN9b3qRTq5NocJclaZIPEtfFe78DX_6z5MH_zkentryZ7Bfqi_i_nN_8b44R9O7IjS8_P_zKcb7YhvW3DWQdBtt0KVTvDoJSDFG5m8WctheIwdyi3En3ffuLLStaSgeC4X0SeLzo0RU6vjZfZyx9GB_CzMggAAA_22_7D = __webpack_require__(39762);
;// ./src/components/ui/AutocompleteInput.css.ts


var container = 'o43rs80';
var AutocompleteInput_css_error = 'o43rs81';
var highlighted = 'o43rs84';
var loading = 'o43rs85';
var suggestion = 'o43rs83';
var AutocompleteInput_css_suggestions = 'o43rs82';
;// ./src/components/ui/AutocompleteInput.tsx
/* __next_internal_client_entry_do_not_use__ AutocompleteInput auto */ 


/** Default filter: plain case-insensitive substring match (unchanged pre-existing behavior). */ function defaultFilterFn(query, suggestions) {
    return suggestions.filter((suggestion)=>suggestion.toLowerCase().includes(query.toLowerCase()));
}
function AutocompleteInput(param) {
    let { id, value, onChange, onBlur, placeholder, suggestions, isLoading = false, className = "", error = false, disabled = false, "data-testid": dataTestId, filterFn = defaultFilterFn, getLabel = (v)=>v } = param;
    const [isOpen, setIsOpen] = (0,react.useState)(false);
    const [highlightedIndex, setHighlightedIndex] = (0,react.useState)(-1);
    const inputRef = (0,react.useRef)(null);
    const listRef = (0,react.useRef)(null);
    // Filter (and, when filterFn is fuzzy, rank) suggestions based on input value
    const filteredSuggestions = filterFn(value, suggestions);
    // Close dropdown when clicking outside
    (0,react.useEffect)(()=>{
        const handleClickOutside = (event)=>{
            if (inputRef.current && !inputRef.current.contains(event.target) && listRef.current && !listRef.current.contains(event.target)) {
                setIsOpen(false);
            }
        };
        document.addEventListener("mousedown", handleClickOutside);
        return ()=>document.removeEventListener("mousedown", handleClickOutside);
    }, []);
    // Scroll highlighted item into view
    (0,react.useEffect)(()=>{
        if (listRef.current && highlightedIndex >= 0) {
            const highlightedElement = listRef.current.children[highlightedIndex];
            if (highlightedElement) {
                highlightedElement.scrollIntoView({
                    block: "nearest",
                    behavior: "smooth"
                });
            }
        }
    }, [
        highlightedIndex
    ]);
    const handleInputChange = (newValue)=>{
        onChange(newValue);
        setIsOpen(true);
        setHighlightedIndex(-1);
    };
    const handleSuggestionClick = (suggestion)=>{
        onChange(suggestion);
        setIsOpen(false);
        setHighlightedIndex(-1);
    };
    const handleKeyDown = (e)=>{
        if (!isOpen || filteredSuggestions.length === 0) {
            if (e.key === "ArrowDown" && filteredSuggestions.length > 0) {
                setIsOpen(true);
                setHighlightedIndex(0);
                e.preventDefault();
            }
            return;
        }
        switch(e.key){
            case "ArrowDown":
                e.preventDefault();
                setHighlightedIndex((prev)=>prev < filteredSuggestions.length - 1 ? prev + 1 : prev);
                break;
            case "ArrowUp":
                e.preventDefault();
                setHighlightedIndex((prev)=>prev > 0 ? prev - 1 : -1);
                break;
            case "Enter":
                e.preventDefault();
                if (highlightedIndex >= 0 && highlightedIndex < filteredSuggestions.length) {
                    handleSuggestionClick(filteredSuggestions[highlightedIndex]);
                }
                break;
            case "Escape":
                e.preventDefault();
                setIsOpen(false);
                setHighlightedIndex(-1);
                break;
            case "Tab":
                // Allow tab to close dropdown but don't prevent default
                setIsOpen(false);
                setHighlightedIndex(-1);
                break;
        }
    };
    const handleFocus = ()=>{
        if (filteredSuggestions.length > 0) {
            setIsOpen(true);
        }
    };
    const handleBlur = ()=>{
        onBlur === null || onBlur === void 0 ? void 0 : onBlur();
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                ref: inputRef,
                id: id,
                type: "text",
                value: value,
                onChange: (e)=>handleInputChange(e.target.value),
                onKeyDown: handleKeyDown,
                onFocus: handleFocus,
                onBlur: handleBlur,
                placeholder: placeholder,
                className: "".concat(className, " ").concat(error ? AutocompleteInput_css_error : ""),
                disabled: disabled,
                "data-testid": dataTestId,
                autoComplete: "off"
            }),
            isOpen && filteredSuggestions.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                ref: listRef,
                className: AutocompleteInput_css_suggestions,
                role: "listbox",
                children: filteredSuggestions.map((sugg, index)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("li", {
                        className: "".concat(suggestion, " ").concat(index === highlightedIndex ? highlighted : ""),
                        onClick: ()=>handleSuggestionClick(sugg),
                        onMouseEnter: ()=>setHighlightedIndex(index),
                        role: "option",
                        "aria-selected": index === highlightedIndex,
                        children: getLabel(sugg)
                    }, sugg))
            }),
            isOpen && isLoading && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: loading,
                children: "Loading suggestions..."
            })
        ]
    });
}


/***/ }),

/***/ 56230:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   U: () => (/* binding */ useAvailablePrograms)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _lib_constants_programs__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(49051);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(66144);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(23575);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(21694);





function useAvailablePrograms() {
    const [programs, setPrograms] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(_lib_constants_programs__WEBPACK_IMPORTED_MODULE_1__/* .PROGRAMS */ .i);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        // Guard against environments (e.g. tests) where fetch or connect transport is not configured
        if (typeof fetch !== "function") return;
        let mounted = true;
        try {
            const client = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_4__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_2__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_3__/* .getConnectTransport */ .qW)());
            // abort-signal-exempt
            client.listProgramsConfig({}).then((res)=>{
                if (!mounted) return;
                if (res.programs && res.programs.length > 0) {
                    const list = res.programs.map((p)=>({
                            value: p.id,
                            label: p.label,
                            description: p.description || p.command,
                            command: p.command,
                            cliFlags: p.cliFlags
                        }));
                    setPrograms(list);
                }
            }).catch(()=>{
                // Fallback to /api/server-info if ListProgramsConfig fails
                fetch("/api/server-info").then((r)=>r.json()).then((data)=>{
                    if (!mounted || !Array.isArray(data.programs)) return;
                    const staticValues = new Set(_lib_constants_programs__WEBPACK_IMPORTED_MODULE_1__/* .PROGRAMS */ .i.map((p)=>p.value));
                    const extra = [];
                    for (const fullPath of data.programs){
                        var _fullPath_split_pop;
                        const basename = (_fullPath_split_pop = fullPath.split("/").pop()) !== null && _fullPath_split_pop !== void 0 ? _fullPath_split_pop : fullPath;
                        if (!staticValues.has(fullPath) && !staticValues.has(basename)) {
                            extra.push({
                                value: basename,
                                label: basename,
                                description: fullPath
                            });
                        }
                    }
                    if (extra.length > 0) {
                        setPrograms([
                            ..._lib_constants_programs__WEBPACK_IMPORTED_MODULE_1__/* .PROGRAMS */ .i,
                            ...extra
                        ]);
                    }
                }).catch(()=>{});
            });
        } catch (e) {
        // Fall back silently
        }
        return ()=>{
            mounted = false;
        };
    }, []);
    return programs;
}


/***/ }),

/***/ 64887:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   X: () => (/* binding */ useSlashCommandSuggestions)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);

// Matches a /word at the cursor — the command name is still being typed (no space yet).
// Works both for whole-input (omnibar) and cursor-word (textarea) detection.
const SLASH_WORD = /\/([a-zA-Z0-9:_-]*)$/;
/**
 * Detects /command autocomplete at the cursor in any text input or textarea.
 *
 * @param value      Current full text value.
 * @param cursorPos  Cursor position within value (use input.selectionStart).
 * @param commands   Available slash commands from useSlashCommands.
 */ function useSlashCommandSuggestions(value, cursorPos, commands) {
    return (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>{
        const textUpToCursor = value.slice(0, cursorPos);
        const match = textUpToCursor.match(SLASH_WORD);
        if (!match) {
            return {
                isActive: false,
                query: "",
                suggestions: [],
                wordStart: 0,
                wordEnd: 0,
                complete
            };
        }
        const query = match[1].toLowerCase();
        const wordStart = cursorPos - match[0].length;
        const wordEnd = cursorPos;
        const suggestions = query ? commands.filter((c)=>c.name.toLowerCase().startsWith(query)) : commands;
        function complete(v, cmd) {
            const insertion = "/".concat(cmd.name, " ");
            const newValue = v.slice(0, wordStart) + insertion + v.slice(wordEnd);
            return {
                newValue,
                newCursorPos: wordStart + insertion.length
            };
        }
        return {
            isActive: true,
            query,
            suggestions,
            wordStart,
            wordEnd,
            complete
        };
    }, [
        value,
        cursorPos,
        commands
    ]);
}


/***/ }),

/***/ 65409:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  "default": () => (/* binding */ WorkflowsPage)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react-dom/index.js
var react_dom = __webpack_require__(80883);
// EXTERNAL MODULE: ./src/lib/nav/index.ts
var nav = __webpack_require__(41078);
// EXTERNAL MODULE: ./src/gen/session/v1/types_pb.ts
var types_pb = __webpack_require__(81432);
// EXTERNAL MODULE: ./src/lib/hooks/useWorkflows.ts + 1 modules
var useWorkflows = __webpack_require__(79623);
// EXTERNAL MODULE: ./src/lib/hooks/useSessionService.ts + 1 modules
var useSessionService = __webpack_require__(73117);
// EXTERNAL MODULE: ./src/components/ui/RepoPathInput.tsx + 2 modules
var RepoPathInput = __webpack_require__(34609);
// EXTERNAL MODULE: ./src/components/ui/SlashCommandDropdown.tsx + 1 modules
var SlashCommandDropdown = __webpack_require__(25562);
// EXTERNAL MODULE: ./src/components/ui/AutocompleteInput.tsx + 1 modules
var AutocompleteInput = __webpack_require__(55963);
// EXTERNAL MODULE: ./src/lib/hooks/useSlashCommands.ts
var useSlashCommands = __webpack_require__(49179);
// EXTERNAL MODULE: ./src/lib/hooks/useSlashCommandSuggestions.ts
var useSlashCommandSuggestions = __webpack_require__(64887);
// EXTERNAL MODULE: ./src/lib/hooks/useAvailablePrograms.ts
var useAvailablePrograms = __webpack_require__(56230);
// EXTERNAL MODULE: ./src/lib/constants/programs.ts
var programs = __webpack_require__(49051);
// EXTERNAL MODULE: ./src/lib/constants/modelFuzzyMatch.ts
var modelFuzzyMatch = __webpack_require__(3542);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/cronstrue@3.24.0/node_modules/cronstrue/dist/cronstrue.js
var cronstrue = __webpack_require__(76556);
var cronstrue_default = /*#__PURE__*/__webpack_require__.n(cronstrue);
;// ./src/lib/cron/validateCron.ts
/**
 * Validates a cron expression against the exact grammar the backend accepts:
 * robfig/cron/v3 configured with `cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow`
 * (server/workflows/scheduler.go). That means: exactly 5 whitespace-separated fields, no
 * seconds field, no `@` descriptors (the Descriptor parser option is not enabled server-side),
 * and no Quartz `?`/`L`/`W`/`#` extensions.
 */ const MINUTE = {
    min: 0,
    max: 59
};
const HOUR = {
    min: 0,
    max: 23
};
const DOM = {
    min: 1,
    max: 31
};
const MONTH = {
    min: 1,
    max: 12,
    names: {
        jan: 1,
        feb: 2,
        mar: 3,
        apr: 4,
        may: 5,
        jun: 6,
        jul: 7,
        aug: 8,
        sep: 9,
        oct: 10,
        nov: 11,
        dec: 12
    }
};
const DOW = {
    min: 0,
    max: 6,
    names: {
        sun: 0,
        mon: 1,
        tue: 2,
        wed: 3,
        thu: 4,
        fri: 5,
        sat: 6
    }
};
const FIELD_NAMES = [
    "minute",
    "hour",
    "day-of-month",
    "month",
    "day-of-week"
];
function parseIntOrName(token, names) {
    if (names) {
        const named = names[token.toLowerCase()];
        if (named !== undefined) return named;
    }
    if (!/^\d+$/.test(token)) return null;
    return parseInt(token, 10);
}
function validateRangeExpr(expr, bounds) {
    const slashParts = expr.split("/");
    if (slashParts.length > 2) return 'too many slashes: "'.concat(expr, '"');
    const [rangePart, stepPart] = slashParts;
    const hyphenParts = rangePart.split("-");
    if (hyphenParts.length > 2) return 'too many hyphens: "'.concat(expr, '"');
    let start;
    let end;
    if (hyphenParts[0] === "*") {
        start = bounds.min;
        end = bounds.max;
    } else {
        const s = parseIntOrName(hyphenParts[0], bounds.names);
        if (s === null) return 'invalid value "'.concat(hyphenParts[0], '" in "').concat(expr, '"');
        start = s;
        if (hyphenParts.length === 2) {
            const e = parseIntOrName(hyphenParts[1], bounds.names);
            if (e === null) return 'invalid value "'.concat(hyphenParts[1], '" in "').concat(expr, '"');
            end = e;
        } else {
            end = start;
        }
    }
    if (stepPart !== undefined) {
        if (!/^\d+$/.test(stepPart)) return 'invalid step "'.concat(stepPart, '" in "').concat(expr, '"');
        const step = parseInt(stepPart, 10);
        if (step === 0) return 'step must be a positive number: "'.concat(expr, '"');
        if (hyphenParts.length === 1 && hyphenParts[0] !== "*") {
            // robfig/cron: "N/step" means "N-max/step"
            end = bounds.max;
        }
    }
    if (start < bounds.min) return '"'.concat(expr, '" starts below the minimum of ').concat(bounds.min);
    if (end > bounds.max) return '"'.concat(expr, '" ends above the maximum of ').concat(bounds.max);
    if (start > end) return '"'.concat(expr, '" has a start greater than its end');
    return null;
}
function validateField(field, bounds) {
    const items = field.split(",");
    for (const item of items){
        if (item === "") return "empty range expression";
        const err = validateRangeExpr(item, bounds);
        if (err) return err;
    }
    return null;
}
function validateCron(expr) {
    const trimmed = expr.trim();
    if (trimmed === "") return {
        valid: false,
        error: "Expression is empty"
    };
    // The backend parser strips an optional TZ=/CRON_TZ= prefix before parsing fields,
    // regardless of which fields are configured — mirror that so we don't over-reject.
    const tzMatch = trimmed.match(/^(?:TZ|CRON_TZ)=\S+\s+(.+)$/);
    const spec = tzMatch ? tzMatch[1] : trimmed;
    if (spec.startsWith("@")) {
        return {
            valid: false,
            error: "Descriptors like @daily aren't supported by this server's cron parser — use 5 numeric fields instead"
        };
    }
    const fields = spec.split(/\s+/).filter(Boolean);
    if (fields.length !== 5) {
        return {
            valid: false,
            error: "Expected exactly 5 fields (minute hour day-of-month month day-of-week), found ".concat(fields.length)
        };
    }
    for (const field of fields){
        if (/[?LW#]/i.test(field)) {
            return {
                valid: false,
                error: 'Unsupported syntax "'.concat(field, "\" — Quartz extensions (?, L, W, #) aren't supported")
            };
        }
    }
    const bounds = [
        MINUTE,
        HOUR,
        DOM,
        MONTH,
        DOW
    ];
    for(let i = 0; i < 5; i++){
        const err = validateField(fields[i], bounds[i]);
        if (err) return {
            valid: false,
            error: "".concat(FIELD_NAMES[i], " field: ").concat(err)
        };
    }
    return {
        valid: true
    };
}

;// ./src/lib/cron/explainCron.ts


/**
 * Human-readable explanation of a 5-field cron expression, or a status/error
 * message suitable for direct display in the UI.
 */ function explainCron(expr) {
    const trimmed = expr.trim();
    if (trimmed === "") return "Enter a schedule above";
    const fields = trimmed.split(/\s+/).filter(Boolean);
    if (!trimmed.startsWith("@") && fields.length < 5) {
        // User is still mid-keystroke; don't flash a hard error on every partial input.
        return "Still typing…";
    }
    const validation = validateCron(trimmed);
    if (!validation.valid) {
        return "Invalid: ".concat(validation.error);
    }
    const [minute, hour, dom, month, dow] = fields;
    try {
        if (dom !== "*" && dow !== "*") {
            // ponytail: cronstrue always joins a restricted day-of-month + day-of-week with
            // "and" (confirmed via its logicalAndDayFields option — neither setting produces
            // "or"). Standard cron (and robfig/cron/v3) semantics are OR when both are
            // restricted, so build the explanation from two single-field expressions and
            // join them ourselves. Ceiling: phrasing can get wordy when combined with
            // step/range hour or month fields — good enough for the common cases this
            // widget is meant to cover.
            const domText = cronstrue_default().toString("".concat(minute, " ").concat(hour, " ").concat(dom, " ").concat(month, " *"));
            const dowText = cronstrue_default().toString("".concat(minute, " ").concat(hour, " * ").concat(month, " ").concat(dow));
            const [, ...dowRestParts] = dowText.split(", ");
            const dowClause = (dowRestParts.join(", ") || dowText).replace(/^only /, "");
            return "".concat(domText, ", or ").concat(dowClause);
        }
        return cronstrue_default().toString(trimmed);
    } catch (e) {
        return "Unable to explain this expression";
    }
}

;// ./src/lib/cron/buildCronFromSimple.ts
const DEFAULT_SIMPLE_SCHEDULE = {
    frequency: "daily",
    hour: 9,
    minute: 0
};
/** Builds the cron string the backend expects from a Simple-mode selection. */ function buildCronFromSimple(s) {
    const { hour, minute } = s;
    switch(s.frequency){
        case "daily":
            return "".concat(minute, " ").concat(hour, " * * *");
        case "weekdays":
            return "".concat(minute, " ").concat(hour, " * * 1-5");
        case "weekly":
            var _s_dayOfWeek;
            return "".concat(minute, " ").concat(hour, " * * ").concat((_s_dayOfWeek = s.dayOfWeek) !== null && _s_dayOfWeek !== void 0 ? _s_dayOfWeek : 1);
        case "monthly":
            var _s_dayOfMonth;
            return "".concat(minute, " ").concat(hour, " ").concat((_s_dayOfMonth = s.dayOfMonth) !== null && _s_dayOfMonth !== void 0 ? _s_dayOfMonth : 1, " * *");
    }
}
/**
 * Recovers a SimpleSchedule from a cron string only if it exactly matches a shape the
 * builder itself produces. Returns null (not an error) for anything else — step values,
 * ranges other than the exact weekday "1-5", lists, multi-field restrictions, etc. — so
 * the caller can fall back to a raw-editor notice instead of guessing.
 */ function parseCronToSimple(expr) {
    const fields = expr.trim().split(/\s+/).filter(Boolean);
    if (fields.length !== 5) return null;
    const [minuteStr, hourStr, dom, month, dow] = fields;
    if (!/^\d{1,2}$/.test(minuteStr) || !/^\d{1,2}$/.test(hourStr)) return null;
    const minute = parseInt(minuteStr, 10);
    const hour = parseInt(hourStr, 10);
    if (minute > 59 || hour > 23) return null;
    if (month !== "*") return null;
    if (dom === "*" && dow === "1-5") {
        return {
            frequency: "weekdays",
            hour,
            minute
        };
    }
    if (dom === "*" && dow === "*") {
        return {
            frequency: "daily",
            hour,
            minute
        };
    }
    if (dom === "*" && /^[0-6]$/.test(dow)) {
        return {
            frequency: "weekly",
            hour,
            minute,
            dayOfWeek: parseInt(dow, 10)
        };
    }
    if (dow === "*" && /^([1-9]|[12]\d|3[01])$/.test(dom)) {
        return {
            frequency: "monthly",
            hour,
            minute,
            dayOfMonth: parseInt(dom, 10)
        };
    }
    return null;
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fworkflows%2FCronScheduleInput.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61UUW-CMBh851d8L0vmA4bi3LT-mKW0hX2xtKQtE1n870vBEUQ0y-YLJBx3vbscLNneHzBP4CsCEOgqxY4UciWbXQTdPRZoJfdoNAVuVF3qgBSsovDJ7HMcv5OtbFPbpu1iF52iZa9I5hVneKtkzEvneUxhoWP0snQUuNRe2kEuWa6sLDu_RvvYYSuvzsgWAedGGTvFSA_V1gWsMtiLD45WnaOKCYG6mAk9FwcgY3xfWFNrMaUcb1tpeqqxQtrYMoG1u0rytriflI3bfHmwd8Lu9JiN3FMgVQPOKBST1_YPyNifZGqvUEsK2mgZHnjLtMN-qhOGvmiF5obXruvmbGQ2kRqT1nc-kYMNOwzXX30br51UYFIgQIDV3gRiiTo-oPAfFDbdogfGW8c4Y-tLbDPGJrztGCNJ8tR334Rau0Wc42emGRrPWYnqSKE02riKcTnSY73zu-O7MY903ED2dx1y8bvg_zP0Q_VHJSmgZwr5Ljp9AzpkfVgWBQAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fworkflows_2FCronScheduleInput_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61UUW_CMBh851d8L0vmA4bi3LT_mKW0hX2xtKQtE1n870vBEUQ0y_YLJBx3vbscLNneHzBP4CsCEOgqxY4UciWbXQTdPRZoJfdoNAVuVF3qgBSsovDJ7HMcv5OtbFPbpu1iF52iZa9I5hVneKtkzEvneUxhoWP0snQUuNRe2kEuWa6sLDu_RvvYYSuvzsgWAedGGTvFSA_V1gWsMtiLD45WnaOKCYG6mAk9FwcgY3xfWFNrMaUcb1tpeqqxQtrYMoG1u0rytriflI3bfHmwd8Lu9JiN3FMgVQPOKBST1_YPyNifZGqvUEsK2mgZHnjLtMN_qhOGvmiF5obXruvmbGQ2kRqT1nc_kYMNOwzXX30br51UYFIgQIDV3gRiiTo_oPAfFDbdogfGW8c4Y_tLbDPGJrztGCNJ8tR334Rau0Wc42emGRrPWYnqSKE02riKcTnSY73zu_O7MY903ED2dx1y8bvg_zP0Q_VHJSmgZwr5Ljp9AzpkfVgWBQAA_22_7D = __webpack_require__(78463);
;// ./src/components/workflows/CronScheduleInput.css.ts


var error = 'aktwifb';
var CronScheduleInput_css_explanation = 'aktwifa';
var modeOption = 'aktwif2';
var modeToggle = 'aktwif1';
var notice = 'aktwif3';
var numberInput = 'aktwif7 aktwif4';
var rawInput = 'aktwif9 aktwif4';
var CronScheduleInput_css_select = 'aktwif6 aktwif4';
var simpleRow = 'aktwif5';
var timeInput = 'aktwif8 aktwif4';
var timezoneNote = 'aktwifc';
var wrapper = 'aktwif0';
;// ./src/components/workflows/CronScheduleInput.tsx
/* __next_internal_client_entry_do_not_use__ CronScheduleInput auto */ 
// +feature: workflow-cron-schedule-input





const DAY_LABELS = [
    "Sunday",
    "Monday",
    "Tuesday",
    "Wednesday",
    "Thursday",
    "Friday",
    "Saturday"
];
function timeToFields(t) {
    const m = /^(\d{2}):(\d{2})$/.exec(t);
    if (!m) return null;
    return {
        hour: parseInt(m[1], 10),
        minute: parseInt(m[2], 10)
    };
}
function fieldsToTime(hour, minute) {
    return "".concat(String(hour).padStart(2, "0"), ":").concat(String(minute).padStart(2, "0"));
}
function CronScheduleInput(param) {
    let { id, labelId, value, onChange } = param;
    const uid = (0,react.useId)();
    const explanationId = "".concat(id, "-explanation");
    const errorId = "".concat(id, "-error");
    const initialParsed = parseCronToSimple(value);
    const [mode, setMode] = (0,react.useState)(value.trim() === "" || initialParsed ? "simple" : "advanced");
    const [simple, setSimple] = (0,react.useState)(initialParsed !== null && initialParsed !== void 0 ? initialParsed : DEFAULT_SIMPLE_SCHEDULE);
    const [fallbackNotice, setFallbackNotice] = (0,react.useState)(false);
    // In Simple mode, preview the schedule the dropdowns currently describe rather than
    // the committed `value` — so an untouched form (e.g. editing an existing manual
    // workflow) shows an honest preview without onChange firing before the user acts,
    // which would otherwise silently write a cron string into an unrelated form save.
    const explanation = mode === "simple" ? explainCron(buildCronFromSimple(simple)) : explainCron(value);
    const validation = validateCron(value.trim());
    const showError = value.trim() !== "" && !validation.valid;
    function emitSimple(next) {
        setSimple(next);
        onChange(buildCronFromSimple(next));
    }
    function handleModeChange(next) {
        if (next === "advanced") {
            setFallbackNotice(false);
            setMode("advanced");
            return;
        }
        const parsed = parseCronToSimple(value);
        if (!parsed && value.trim() !== "") {
            // Not representable by the builder — stay on Advanced with a raw-editor notice
            // rather than silently discarding step values/ranges/lists.
            setFallbackNotice(true);
            return;
        }
        setFallbackNotice(false);
        setSimple(parsed !== null && parsed !== void 0 ? parsed : DEFAULT_SIMPLE_SCHEDULE);
        if (!parsed) onChange(buildCronFromSimple(DEFAULT_SIMPLE_SCHEDULE));
        setMode("simple");
    }
    var _simple_dayOfWeek, _simple_dayOfMonth;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        id: id,
        className: wrapper,
        role: "group",
        "aria-labelledby": labelId,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: modeToggle,
                role: "radiogroup",
                "aria-label": "Schedule entry mode",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                        className: modeOption,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                type: "radio",
                                name: "".concat(uid, "-cron-mode"),
                                checked: mode === "simple",
                                onChange: ()=>handleModeChange("simple")
                            }),
                            "Simple"
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                        className: modeOption,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                type: "radio",
                                name: "".concat(uid, "-cron-mode"),
                                checked: mode === "advanced",
                                onChange: ()=>handleModeChange("advanced")
                            }),
                            "Advanced"
                        ]
                    })
                ]
            }),
            fallbackNotice && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: notice,
                role: "status",
                children: "This expression uses step values, ranges, or lists the Simple builder can't represent — edit it directly below."
            }),
            mode === "simple" ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: simpleRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                        className: CronScheduleInput_css_select,
                        "aria-label": "Frequency",
                        value: simple.frequency,
                        onChange: (e)=>emitSimple({
                                ...simple,
                                frequency: e.target.value
                            }),
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                value: "daily",
                                children: "Daily"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                value: "weekdays",
                                children: "Every weekday"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                value: "weekly",
                                children: "Weekly"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                value: "monthly",
                                children: "Monthly"
                            })
                        ]
                    }),
                    simple.frequency === "weekly" && /*#__PURE__*/ (0,jsx_runtime.jsx)("select", {
                        className: CronScheduleInput_css_select,
                        "aria-label": "Day of week",
                        value: (_simple_dayOfWeek = simple.dayOfWeek) !== null && _simple_dayOfWeek !== void 0 ? _simple_dayOfWeek : 1,
                        onChange: (e)=>emitSimple({
                                ...simple,
                                dayOfWeek: parseInt(e.target.value, 10)
                            }),
                        children: DAY_LABELS.map((label, idx)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                value: idx,
                                children: label
                            }, label))
                    }),
                    simple.frequency === "monthly" && /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        className: numberInput,
                        type: "number",
                        "aria-label": "Day of month",
                        min: 1,
                        max: 31,
                        value: (_simple_dayOfMonth = simple.dayOfMonth) !== null && _simple_dayOfMonth !== void 0 ? _simple_dayOfMonth : 1,
                        onChange: (e)=>emitSimple({
                                ...simple,
                                dayOfMonth: Math.min(31, Math.max(1, parseInt(e.target.value, 10) || 1))
                            })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        className: timeInput,
                        type: "time",
                        "aria-label": "Time",
                        value: fieldsToTime(simple.hour, simple.minute),
                        onChange: (e)=>{
                            const t = timeToFields(e.target.value);
                            if (t) emitSimple({
                                ...simple,
                                hour: t.hour,
                                minute: t.minute
                            });
                        }
                    })
                ]
            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                className: rawInput,
                type: "text",
                "aria-label": "Advanced cron expression",
                value: value,
                onChange: (e)=>onChange(e.target.value),
                placeholder: "0 9 * * 1-5",
                "aria-describedby": showError ? "".concat(explanationId, " ").concat(errorId) : explanationId,
                "aria-invalid": showError
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                id: explanationId,
                className: CronScheduleInput_css_explanation,
                "aria-live": "polite",
                children: explanation
            }),
            showError && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                id: errorId,
                role: "alert",
                "data-testid": "wf-cron-error",
                className: error,
                children: validation.error
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: timezoneNote,
                children: "Runs in the server's local timezone (not your browser's)"
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fworkflows%2FWorkflowForm.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-1W227iMBB95yv8shJo1yhJC7Tuy_5J5diT4K1jW7ZTAqv--8oObEkggbI8rhACJXM5c854xvPXlK7Mzi4T9HuCEBfOSLolqJDQvExQ_MVcWGBeaEUQ07KuVHhTUkPQO7VTjF_TZ9hldveQzl4mH5P5IWYaY1bUlkLhXHuvq75Ltuu6ZNGl0MpjJ3ZwkoHPXg7vNyDKtSdolSThGdNS2759Es1bBAQlnVQPN1WczB8sVJ1Ajxcw56eYFyOYeyQuYvTzlsflYQmFD_iyPr5ljGAo50KVZwTo422j5pS9lVbXip-kpbNh8G2xubYcLEGpaZDTUvCe2duRFbaUi9qd0Lb6pG2EVl17KRQQpLSC8GAjuF8TlCbJtzZHE9xj5ft8uY5Ce0uVE63IvdCqK8GSFJrVLtK4j3G2eNl348LRXAKPntpQJvw2SLSM_NXWhRhKe0yl1BvgHffVf9lukA0hC_u8YL1gVMYjIhRe78_eU2KavxALWgm5JajSSjtDGXQluE34p1Y5fYBpQVIv3ruxn3tGNHda1j6S4bUhiFHJpoEP9B1lpom17c94LLOtJv7fYaE4NGGuXCMjXC1jlGFNud4QlKBH06A0Mw2yZU6nyY_4maeLVtF3sIUMhmvBOahOsfTCiBxpzex4xkViQsr-jMvPD3MqRamw8FA5ghgoD3Zgd8VFdLlvByf253k2WrR5juCxLrzSCh5xWMGxh8pI6gG3m8YRlBY2fIe2bNLtNn7vOZGOVJrcaQIc4Yfz0v2qnRfFFjOtPCjfvsCg-Bgt3UYZv2oUX-QtPeEtjiJDLSh_oTfudyJvumb0O_Mry68g63C02xk42DRPwz3Ta9fyn2nvGlTDmRcd7g-b5u4sL-_AcnkVyydOt1wwflbABUWOWQCFqOJoWtEG7xfwY1iPsxivN73GxlVI9zH5-APgb0y4TgwAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fworkflows_2FWorkflowForm_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_1W227iMBB95yv8shJo1yhJC7Tuy_5J5diT4K1jW7ZTAqv_8oObEkggbI8rhACJXM5c854xvPXlK7Mzi4T9HuCEBfOSLolqJDQvExQ_MVcWGBeaEUQ07KuVHhTUkPQO7VTjF_TZ9hldveQzl4mH5P5IWYaY1bUlkLhXHuvq75Ltuu6ZNGl0MpjJ3ZwkoHPXg7vNyDKtSdolSThGdNS2759Es1bBAQlnVQPN1WczB8sVJ1Ajxcw56eYFyOYeyQuYvTzlsflYQmFD_iyPr5ljGAo50KVZwTo422j5pS9lVbXip_kpbNh8G2xubYcLEGpaZDTUvCe2duRFbaUi9qd0Lb6pG2EVl17KRQQpLSC8GAjuF8TlCbJtzZHE9xj5ft8uY5Ce0uVE63IvdCqK8GSFJrVLtK4j3G2eNl348LRXAKPntpQJvw2SLSM_NXWhRhKe0yl1BvgHffVf9lukA0hC_u8YL1gVMYjIhRe78_eU2KavxALWgm5JajSSjtDGXQluE34p1Y5fYBpQVIv3ruxn3tGNHda1j6S4bUhiFHJpoEP9B1lpom17c94LLOtJv7fYaE4NGGuXCMjXC1jlGFNud4QlKBH06A0Mw2yZU6nyY_4maeLVtF3sIUMhmvBOahOsfTCiBxpzex4xkViQsr_jMvPD3MqRamw8FA5ghgoD3Zgd8VFdLlvByf253k2WrR5juCxLrzSCh5xWMGxh8pI6gG3m8YRlBY2fIe2bNLtNn7vOZGOVJrcaQIc4Yfz0v2qnRfFFjOtPCjfvsCg_Bgt3UYZv2oUX_QtPeEtjiJDLSh_oTfudyJvumb0O_Mry68g63C02xk42DRPwz3Ta9fyn2nvGlTDmRcd7g_b5u4sL_AcnkVyydOt1wwflbABUWOWQCFqOJoWtEG7xfwY1iPsxivN73GxlVI9zH5_APgb0y4TgwAAA_22_7D = __webpack_require__(49351);
;// ./src/components/workflows/WorkflowForm.css.ts


var buttonRow = '_1a7pzr6e';
var cancelButton = '_1a7pzr6f';
var checkboxRow = '_1a7pzr6b';
var errorBanner = '_1a7pzr6d';
var fieldGroup = '_1a7pzr63';
var WorkflowForm_css_form = '_1a7pzr60';
var formHeader = '_1a7pzr61';
var formTitle = '_1a7pzr62';
var hint = '_1a7pzr6a';
var input = '_1a7pzr66';
var label = '_1a7pzr64';
var required = '_1a7pzr65';
var row = '_1a7pzr6c';
var slashDropdownWrapper = '_1a7pzr69';
var submitButton = '_1a7pzr6g';
var WorkflowForm_css_textarea = '_1a7pzr67';
var textareaWrapper = '_1a7pzr68';
;// ./src/components/workflows/WorkflowForm.tsx
/* __next_internal_client_entry_do_not_use__ WorkflowForm auto */ 












const EMPTY = {
    slug: "",
    name: "",
    description: "",
    command: "",
    targetDirectory: "",
    inputTemplate: "",
    sessionType: "directory",
    model: "",
    agentType: "",
    cronExpression: "",
    cronEnabled: false,
    keepSessions: 0,
    archiveAfterHours: 0
};
function protoToFormData(w) {
    var _w_keepSessions, _w_archiveAfterHours;
    return {
        slug: w.slug,
        name: w.name,
        description: w.description,
        command: w.command,
        targetDirectory: w.targetDirectory,
        inputTemplate: w.inputTemplate,
        sessionType: w.sessionType || "directory",
        model: w.model,
        agentType: w.agentType,
        cronExpression: w.cronExpression,
        cronEnabled: w.cronEnabled,
        keepSessions: (_w_keepSessions = w.keepSessions) !== null && _w_keepSessions !== void 0 ? _w_keepSessions : 0,
        archiveAfterHours: (_w_archiveAfterHours = w.archiveAfterHours) !== null && _w_archiveAfterHours !== void 0 ? _w_archiveAfterHours : 0
    };
}
function WorkflowForm(param) {
    let { existing, onSubmit, onCancel } = param;
    const [formData, setFormData] = (0,react.useState)(existing ? protoToFormData(existing) : EMPTY);
    const [submitting, setSubmitting] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    // Slash command autocomplete for the command textarea.
    const commandRef = (0,react.useRef)(null);
    const [commandCursorPos, setCommandCursorPos] = (0,react.useState)(0);
    const [slashSuggestIndex, setSlashSuggestIndex] = (0,react.useState)(-1);
    const { commands: slashCommands } = (0,useSlashCommands/* useSlashCommands */.d)(formData.targetDirectory);
    const availablePrograms = (0,useAvailablePrograms/* useAvailablePrograms */.U)();
    const modelSuggestions = programs/* MODEL_AUTOCOMPLETE_OPTIONS */.kg.map((m)=>m.value);
    const agentTypeSuggestions = availablePrograms.map((p)=>p.value).filter(Boolean);
    const slashState = (0,useSlashCommandSuggestions/* useSlashCommandSuggestions */.X)(formData.command, commandCursorPos, slashCommands);
    const isSlashDropdownVisible = slashState.isActive && slashState.suggestions.length > 0;
    const handleSlashSelect = (0,react.useCallback)((cmd)=>{
        const { newValue, newCursorPos } = slashState.complete(formData.command, cmd);
        setField("command", newValue);
        setSlashSuggestIndex(-1);
        // Restore cursor after React re-renders the textarea value.
        requestAnimationFrame(()=>{
            if (commandRef.current) {
                commandRef.current.setSelectionRange(newCursorPos, newCursorPos);
                commandRef.current.focus();
                setCommandCursorPos(newCursorPos);
            }
        });
    }, [
        slashState,
        formData.command,
        setField
    ]);
    const handleCommandKeyDown = (0,react.useCallback)((e)=>{
        if (!isSlashDropdownVisible) return;
        if (e.key === "ArrowDown") {
            e.preventDefault();
            setSlashSuggestIndex((i)=>Math.min(i + 1, slashState.suggestions.length - 1));
        } else if (e.key === "ArrowUp") {
            e.preventDefault();
            setSlashSuggestIndex((i)=>Math.max(i - 1, -1));
        } else if (e.key === "Tab") {
            e.preventDefault();
            const idx = slashSuggestIndex >= 0 ? slashSuggestIndex : 0;
            if (slashState.suggestions[idx]) handleSlashSelect(slashState.suggestions[idx]);
        } else if (e.key === "Enter" && slashSuggestIndex >= 0) {
            if (slashState.suggestions[slashSuggestIndex]) {
                e.preventDefault();
                handleSlashSelect(slashState.suggestions[slashSuggestIndex]);
            }
        } else if (e.key === "Escape") {
            setSlashSuggestIndex(-1);
        }
    }, [
        isSlashDropdownVisible,
        slashState,
        slashSuggestIndex,
        handleSlashSelect
    ]);
    const isEdit = !!existing;
    function setField(key, value) {
        setFormData((prev)=>({
                ...prev,
                [key]: value
            }));
    }
    async function handleSubmit(e) {
        e.preventDefault();
        var _formData_cronExpression;
        const cron = ((_formData_cronExpression = formData.cronExpression) !== null && _formData_cronExpression !== void 0 ? _formData_cronExpression : "").trim();
        if (cron !== "") {
            const validation = validateCron(cron);
            if (!validation.valid) {
                setError("Cron expression: ".concat(validation.error));
                return;
            }
        }
        setSubmitting(true);
        setError(null);
        try {
            await onSubmit(formData);
        } catch (err) {
            setError(err instanceof Error ? err.message : "Failed to save workflow");
            setSubmitting(false);
        }
    }
    var _formData_description, _formData_inputTemplate, _formData_model, _formData_agentType, _formData_cronExpression, _formData_keepSessions, _formData_archiveAfterHours;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("form", {
        className: WorkflowForm_css_form,
        onSubmit: handleSubmit,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: formHeader,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                    className: formTitle,
                    children: isEdit ? "Edit Workflow" : "New Workflow"
                })
            }),
            error && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: errorBanner,
                children: error
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: row,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: fieldGroup,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: label,
                                htmlFor: "wf-slug",
                                children: [
                                    "Slug ",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: required,
                                        children: "*"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "wf-slug",
                                className: input,
                                type: "text",
                                value: formData.slug,
                                onChange: (e)=>setField("slug", e.target.value),
                                placeholder: "my-workflow",
                                required: true,
                                disabled: isEdit,
                                pattern: "[a-z0-9]+(-[a-z0-9]+)*",
                                minLength: 2,
                                maxLength: 64,
                                title: "2-64 lowercase chars, hyphens allowed (no consecutive or leading/trailing hyphens)"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: hint,
                                children: "Type @slug in the omnibar to invoke"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: fieldGroup,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: label,
                                htmlFor: "wf-name",
                                children: [
                                    "Name ",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: required,
                                        children: "*"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "wf-name",
                                className: input,
                                type: "text",
                                value: formData.name,
                                onChange: (e)=>setField("name", e.target.value),
                                placeholder: "My Workflow",
                                required: true
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: fieldGroup,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                        className: label,
                        htmlFor: "wf-description",
                        children: "Description"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        id: "wf-description",
                        className: input,
                        type: "text",
                        value: (_formData_description = formData.description) !== null && _formData_description !== void 0 ? _formData_description : "",
                        onChange: (e)=>setField("description", e.target.value),
                        placeholder: "What does this workflow do?"
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: fieldGroup,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                        className: label,
                        htmlFor: "wf-command",
                        children: [
                            "Command / Prompt ",
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: required,
                                children: "*"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: textareaWrapper,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("textarea", {
                                ref: commandRef,
                                id: "wf-command",
                                className: WorkflowForm_css_textarea,
                                value: formData.command,
                                onChange: (e)=>{
                                    setField("command", e.target.value);
                                    var _e_target_selectionStart;
                                    setCommandCursorPos((_e_target_selectionStart = e.target.selectionStart) !== null && _e_target_selectionStart !== void 0 ? _e_target_selectionStart : 0);
                                    setSlashSuggestIndex(-1);
                                },
                                onSelect: (e)=>{
                                    var _e_target_selectionStart;
                                    return setCommandCursorPos((_e_target_selectionStart = e.target.selectionStart) !== null && _e_target_selectionStart !== void 0 ? _e_target_selectionStart : 0);
                                },
                                onKeyDown: handleCommandKeyDown,
                                placeholder: "Describe what the agent should do… (type / for slash commands)",
                                required: true
                            }),
                            isSlashDropdownVisible && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: slashDropdownWrapper,
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(SlashCommandDropdown/* SlashCommandDropdown */.U, {
                                    id: "wf-command-slash-listbox",
                                    suggestions: slashState.suggestions,
                                    selectedIndex: slashSuggestIndex,
                                    onSelect: handleSlashSelect
                                })
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: hint,
                        children: [
                            "Use ",
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                                children: "{{input}}"
                            }),
                            " to inject the argument typed after @slug. Type ",
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                                children: "/"
                            }),
                            " for slash commands."
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: fieldGroup,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                        className: label,
                        htmlFor: "wf-target-dir",
                        children: [
                            "Target Directory ",
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: required,
                                children: "*"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(RepoPathInput/* RepoPathInput */.E, {
                        id: "wf-target-dir",
                        value: formData.targetDirectory,
                        onChange: (v)=>setField("targetDirectory", v),
                        placeholder: "/path/to/project",
                        required: true
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: fieldGroup,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                        className: label,
                        htmlFor: "wf-input-template",
                        children: "Input Template"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("textarea", {
                        id: "wf-input-template",
                        className: WorkflowForm_css_textarea,
                        value: (_formData_inputTemplate = formData.inputTemplate) !== null && _formData_inputTemplate !== void 0 ? _formData_inputTemplate : "",
                        onChange: (e)=>setField("inputTemplate", e.target.value),
                        placeholder: "Optional: template wrapping the user-supplied {{input}}"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: hint,
                        children: [
                            "If set, ",
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                                children: "{{input}}"
                            }),
                            " is replaced with the argument before passing to the agent."
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: row,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: fieldGroup,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: label,
                                htmlFor: "wf-model",
                                children: "Model"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(AutocompleteInput/* AutocompleteInput */.P, {
                                id: "wf-model",
                                value: (_formData_model = formData.model) !== null && _formData_model !== void 0 ? _formData_model : "",
                                onChange: (v)=>setField("model", v),
                                placeholder: "claude-sonnet-4-6",
                                suggestions: modelSuggestions,
                                filterFn: modelFuzzyMatch/* fuzzyMatchModels */.C,
                                getLabel: modelFuzzyMatch/* getModelLabel */.i,
                                className: input
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: fieldGroup,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: label,
                                htmlFor: "wf-agent-type",
                                children: "Program"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(AutocompleteInput/* AutocompleteInput */.P, {
                                id: "wf-agent-type",
                                value: (_formData_agentType = formData.agentType) !== null && _formData_agentType !== void 0 ? _formData_agentType : "",
                                onChange: (v)=>setField("agentType", v),
                                placeholder: "claude",
                                suggestions: agentTypeSuggestions,
                                className: input
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: row,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: fieldGroup,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: label,
                                id: "wf-cron-label",
                                children: "Cron Expression"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(CronScheduleInput, {
                                id: "wf-cron",
                                labelId: "wf-cron-label",
                                value: (_formData_cronExpression = formData.cronExpression) !== null && _formData_cronExpression !== void 0 ? _formData_cronExpression : "",
                                onChange: (v)=>setField("cronExpression", v)
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: fieldGroup,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: label,
                                children: "\xa0"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: checkboxRow,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        type: "checkbox",
                                        checked: formData.cronEnabled,
                                        onChange: (e)=>setField("cronEnabled", e.target.checked)
                                    }),
                                    "Enable scheduled runs"
                                ]
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: row,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: fieldGroup,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: label,
                                htmlFor: "wf-keep-sessions",
                                children: "Keep Sessions"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "wf-keep-sessions",
                                className: input,
                                type: "number",
                                min: 0,
                                value: (_formData_keepSessions = formData.keepSessions) !== null && _formData_keepSessions !== void 0 ? _formData_keepSessions : 0,
                                onChange: (e)=>setField("keepSessions", parseInt(e.target.value, 10) || 0),
                                "data-testid": "keep-sessions-input"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: hint,
                                children: "Keep only the N most recent sessions (0 = keep all)"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: fieldGroup,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: label,
                                htmlFor: "wf-archive-after-hours",
                                children: "Archive After Hours"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "wf-archive-after-hours",
                                className: input,
                                type: "number",
                                min: 0,
                                value: (_formData_archiveAfterHours = formData.archiveAfterHours) !== null && _formData_archiveAfterHours !== void 0 ? _formData_archiveAfterHours : 0,
                                onChange: (e)=>setField("archiveAfterHours", parseInt(e.target.value, 10) || 0),
                                "data-testid": "archive-after-hours-input"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: hint,
                                children: "Auto-archive completed sessions after N hours (0 = disabled)"
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: buttonRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: cancelButton,
                        onClick: onCancel,
                        disabled: submitting,
                        children: "Cancel"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "submit",
                        className: submitButton,
                        disabled: submitting,
                        children: submitting ? "Saving…" : isEdit ? "Save Changes" : "Create Workflow"
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fworkflows%2FWorkflowsPanel.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA8VY226jMBB971f4ZaVGWiJDm6SlH1M5eAhujE2NSUhW_fcVJjTmTmi3q0ptCuO5nBnPmcny1Q1XmwCfMPpzhxBlacLJyUchh_zlDpm_DmUKAs2k8FEgeRaL4s2OJD5ylysF8cvdx92yUuR2KyKc7YTDNMSpjwIQGlTx-C1LNQtPTiCFBqF9lCYkAGcL-gggPj04qsJa8fvTNF5uWra9WUHgpdfS9GA0hVJoJ2Vn8NGBqHvHeXWf4eyp8wMsXqr3R2C7SPtog3HxLJBcqqY8NuIxUTsmfIRrph5HTG0XvWrdAbWrm_JwAeLR4IBQQihlYte0550XTe9KF7Yk2O-UzARtHon7vV-VR6WioHwkpIDr_44ilGVpC4zNFfcBsGp5WV_ykqm08CGRrApaKyJSVtZEQ49Y1MH0I3kAZSDtj7VxaG3Ee6B8wDdD6boDlYAX3wOe5f9m0P8Ho0xDrh1TU3Y1dbro3Wb96eetX4vRTXJESRoBbcjBBJif6oE8m0CK8gm5PDq5j0im5TQ1dY9SyVmXQ5YxYowdGdWRj1yMf1l2Ask5SVIwDdB8uikh21nlbKeIQ6h7r2d_f-u_EdTOxlZqLeNhpBA6RkyDY1im6DklpVhRBrOiHGj6E70LjfABlGYB4RViMaOUQ81BOqkVNWoQrhwTkpjxk49iKaSBYbgIyKQUfOLlJXmbM8ZrfT3GcWNpC-tsxwRnApxppOc1SW9mEM8jLYa0uWmF66S9G0nroR-nrJ7xaEQT7dfk1TWx7kHiAt7qAl4nxJaWt6F75Z1a9wr_7Fyxvj133zFXvM2ZK_ZfhtI4mBAFQo_13gn08w-wXX0DtvvJjXKgf1v6-M_AjifhPmne-0_A80nAm2nWOhWX8mVAWiZjVHnbimIZEl9ZjAbS2Ww4HRiOpWJgcv0Mt04YwsK69xpb8nLWhtyq9evaWaaqgy0to8ntiHfS7yz4GgC890MVXwdWCoFUpMSiymc1wfsoYpSWX04Y4esL4JwlKUunTCzvVuZaNjNBQRUjTO2I6hxytlwG-1qJuO355TSnXeDluqL3LuYzXptLEEoV-yhLElABKdeKsfDTspvKqqWELAdaHGQiBW3KvH7b1G5L7vFv87MsKf7sMEEhN4uOkZ_9rdP1Vd_k3ygjPdLd1hP3xH6D5bVvbHIxyZ3Lo9UaJ3n1LLpk5hkfIrtWnZO9beZOGhFaFGrDVlH5H38BQA2l5wsUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fworkflows_2FWorkflowsPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA8VY226jMBB971f4ZaVGWiJDm6SlH1M5eAhujE2NSUhW_fcVJjTmTmi3q0ptCuO5nBnPmcny1Q1XmwCfMPpzhxBlacLJyUchh_zlDpm_DmUKAs2k8FEgeRaL4s2OJD5ylysF8cvdx92yUuR2KyKc7YTDNMSpjwIQGlTx_C1LNQtPTiCFBqF9lCYkAGcL_gggPj04qsJa8fvTNF5uWra9WUHgpdfS9GA0hVJoJ2Vn8NGBqHvHeXWf4eyp8wMsXqr3R2C7SPtog3HxLJBcqqY8NuIxUTsmfIRrph5HTG0XvWrdAbWrm_JwAeLR4IBQQihlYte0550XTe9KF7Yk2O_UzARtHon7vV_VR6WioHwkpIDr_44ilGVpC4zNFfcBsGp5WV_ykqm08CGRrApaKyJSVtZEQ49Y1MH0I3kAZSDtj7VxaG3Ee6B8wDdD6boDlYAX3wOe5f9m0P8Ho0xDrh1TU3Y1dbro3Wb96eetX4vRTXJESRoBbcjBBJif6oE8m0CK8gm5PDq5j0im5TQ1dY9SyVmXQ5YxYowdGdWRj1yMf1l2Ask5SVIwDdB8uikh21nlbKeIQ6h7r2d_f_u_EdTOxlZqLeNhpBA6RkyDY1im6DklpVhRBrOiHGj6E70LjfABlGYB4RViMaOUQ81BOqkVNWoQrhwTkpjxk49iKaSBYbgIyKQUfOLlJXmbM8ZrfT3GcWNpC_tsxwRnApxppOc1SW9mEM8jLYa0uWmF66S9G0nroR_nrJ7xaEQT7dfk1TWx7kHiAt7qAl4nxJaWt6F75Z1a9wr_7Fyxvj133zFXvM2ZK_ZfhtI4mBAFQo_13gn08w_wXX0DtvvJjXKgf1v6_M_AjifhPmne_0_A80nAm2nWOhWX8mVAWiZjVHnbimIZEl9ZjAbS2Ww4HRiOpWJgcv0Mt04YwsK69xpb8nLWhtyq9evaWaaqgy0to8ntiHfS7yz4GgC890MVXwdWCoFUpMSiymc1wfsoYpSWX04Y4esL4JwlKUunTCzvVuZaNjNBQRUjTO2I6hxytlwG_1qJuO355TSnXeDluqL3LuYzXptLEEoV_yhLElABKdeKsfDTspvKqqWELAdaHGQiBW3KvH7b1G5L7vFv87MsKf7sMEEhN4uOkZ_9rdP1Vd_k3ygjPdLd1hP3xH6D5bVvbHIxyZ3Lo9UaJ3n1LLpk5hkfIrtWnZO9beZOGhFaFGrDVlH5H38BQA2l5wsUAAA_22_7D = __webpack_require__(26491);
;// ./src/components/workflows/WorkflowsPanel.css.ts


var actions = '_1f57c0yi';
var addButton = '_1f57c0y5';
var cronBadge = '_1f57c0yf';
var cronDisabled = '_1f57c0yh';
var cronEnabled = '_1f57c0yg';
var deleteButton = '_1f57c0yl';
var editButton = '_1f57c0yk';
var empty = '_1f57c0y8';
var WorkflowsPanel_css_error = '_1f57c0y6';
var formCard = '_1f57c0yt';
var formOverlay = '_1f57c0ys';
var header = '_1f57c0y1';
var WorkflowsPanel_css_loading = '_1f57c0y7';
var panel = '_1f57c0y0';
var WorkflowsPanel_css_row = '_1f57c0yd';
var runButton = '_1f57c0yj';
var runLink = '_1f57c0yq';
var runRow = '_1f57c0yp';
var runsAccordion = '_1f57c0ym';
var runsList = '_1f57c0yo';
var runsToggle = '_1f57c0yn';
var slugCell = '_1f57c0ye';
var statusBadge = '_1f57c0yr';
var subtitle = '_1f57c0y4';
var table = '_1f57c0ya';
var tableWrapper = '_1f57c0y9';
var td = '_1f57c0yc';
var th = '_1f57c0yb';
var title = '_1f57c0y3';
var titleRow = '_1f57c0y2';
;// ./src/components/workflows/WorkflowsPanel.tsx
/* __next_internal_client_entry_do_not_use__ WorkflowsPanel auto */ 
// +feature: workflows-management








function statusLabel(status) {
    switch(status){
        case types_pb/* SessionStatus */.Ph.ACTIVE:
            return "active";
        case types_pb/* SessionStatus */.Ph.STOPPED:
            return "stopped";
        case types_pb/* SessionStatus */.Ph.PAUSED:
            return "paused";
        case types_pb/* SessionStatus */.Ph.HIBERNATED:
            return "hibernated";
        default:
            return "unknown";
    }
}
function statusColor(status) {
    switch(status){
        case types_pb/* SessionStatus */.Ph.ACTIVE:
            return "var(--success)";
        case types_pb/* SessionStatus */.Ph.STOPPED:
            return "var(--text-muted)";
        case types_pb/* SessionStatus */.Ph.PAUSED:
            return "var(--warning)";
        default:
            return "var(--text-muted)";
    }
}
function RecentRuns(param) {
    let { workflowId } = param;
    const [expanded, setExpanded] = (0,react.useState)(false);
    const [runs, setRuns] = (0,react.useState)([]);
    const [loading, setLoading] = (0,react.useState)(false);
    const { listSessionsByWorkflow } = (0,useSessionService/* useSessionService */.D)();
    const load = (0,react.useCallback)(async ()=>{
        if (runs.length > 0) return;
        setLoading(true);
        const sessions = await listSessionsByWorkflow(workflowId, true);
        setRuns(sessions.slice(-5).reverse());
        setLoading(false);
    }, [
        workflowId,
        runs.length,
        listSessionsByWorkflow
    ]);
    function toggle() {
        if (!expanded) void load();
        setExpanded((v)=>!v);
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: runsAccordion,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                className: runsToggle,
                onClick: toggle,
                children: [
                    expanded ? "▾" : "▸",
                    " Recent Runs"
                ]
            }),
            expanded && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: runsList,
                children: [
                    loading && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        style: {
                            fontSize: "0.7rem",
                            color: "var(--text-muted)"
                        },
                        children: "Loading…"
                    }),
                    !loading && runs.length === 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        style: {
                            fontSize: "0.7rem",
                            color: "var(--text-muted)"
                        },
                        children: "No runs yet."
                    }),
                    runs.map((s)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: runRow,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: statusBadge,
                                    style: {
                                        background: statusColor(s.status) + "33",
                                        color: statusColor(s.status)
                                    },
                                    children: statusLabel(s.status)
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)(nav/* Link */.N_, {
                                    href: "/?session=".concat(s.id),
                                    className: runLink,
                                    children: s.title
                                }),
                                s.updatedAt && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    style: {
                                        color: "var(--text-muted)",
                                        flexShrink: 0
                                    },
                                    children: new Date(Number(s.updatedAt.seconds) * 1000).toLocaleString(undefined, {
                                        month: "short",
                                        day: "numeric",
                                        hour: "2-digit",
                                        minute: "2-digit"
                                    })
                                })
                            ]
                        }, s.id))
                ]
            })
        ]
    });
}
function WorkflowsPanel() {
    const { workflows, loading, error, createWorkflow, updateWorkflow, deleteWorkflow, archiveWorkflowSessions, deleteWorkflowFailedSessions } = (0,useWorkflows/* useWorkflows */.m)();
    const { runWorkflow } = (0,useSessionService/* useSessionService */.D)();
    const router = (0,nav/* useRouter */.KA)();
    const [showForm, setShowForm] = (0,react.useState)(false);
    const [editTarget, setEditTarget] = (0,react.useState)(null);
    const [confirmDeleteId, setConfirmDeleteId] = (0,react.useState)(null);
    const [runningId, setRunningId] = (0,react.useState)(null);
    const [confirmArchiveId, setConfirmArchiveId] = (0,react.useState)(null);
    const [confirmDeleteFailedId, setConfirmDeleteFailedId] = (0,react.useState)(null);
    const [archivingId, setArchivingId] = (0,react.useState)(null);
    const [deletingFailedId, setDeletingFailedId] = (0,react.useState)(null);
    function openCreate() {
        setEditTarget(null);
        setShowForm(true);
    }
    function openEdit(wf) {
        setEditTarget(wf);
        setShowForm(true);
    }
    function closeForm() {
        setShowForm(false);
        setEditTarget(null);
    }
    async function handleFormSubmit(data) {
        if (editTarget) {
            await updateWorkflow(editTarget.id, data);
        } else {
            await createWorkflow(data);
        }
        closeForm();
    }
    function handleDeleteClick(wf) {
        setConfirmDeleteId(wf.id);
    }
    async function handleDeleteConfirm() {
        if (!confirmDeleteId) return;
        await deleteWorkflow(confirmDeleteId);
        setConfirmDeleteId(null);
    }
    function handleDeleteCancel() {
        setConfirmDeleteId(null);
    }
    async function handleArchiveConfirm() {
        if (!confirmArchiveId) return;
        setArchivingId(confirmArchiveId);
        setConfirmArchiveId(null);
        try {
            await archiveWorkflowSessions(confirmArchiveId);
        } finally{
            setArchivingId(null);
        }
    }
    async function handleDeleteFailedConfirm() {
        if (!confirmDeleteFailedId) return;
        setDeletingFailedId(confirmDeleteFailedId);
        setConfirmDeleteFailedId(null);
        try {
            await deleteWorkflowFailedSessions(confirmDeleteFailedId);
        } finally{
            setDeletingFailedId(null);
        }
    }
    async function handleRun(wf) {
        setRunningId(wf.id);
        const sessionId = await runWorkflow({
            id: wf.id
        });
        setRunningId(null);
        if (sessionId) {
            router.push("/?session=".concat(sessionId));
        }
    }
    const formModal = showForm ? /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: formOverlay,
        onClick: (e)=>{
            if (e.target === e.currentTarget) closeForm();
        },
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: formCard,
            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(WorkflowForm, {
                existing: editTarget !== null && editTarget !== void 0 ? editTarget : undefined,
                onSubmit: handleFormSubmit,
                onCancel: closeForm
            })
        })
    }), document.body) : null;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: panel,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: header,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: titleRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("h1", {
                                className: title,
                                children: "Workflows"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                                className: subtitle,
                                children: [
                                    "Define reusable agent workflows. Invoke them from the omnibar with",
                                    " ",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                                        children: "@slug [optional-arg]"
                                    }),
                                    "."
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: addButton,
                        onClick: openCreate,
                        children: "+ New Workflow"
                    })
                ]
            }),
            error && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: WorkflowsPanel_css_error,
                children: error.message
            }),
            loading && workflows.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: WorkflowsPanel_css_loading,
                children: "Loading workflows…"
            }) : workflows.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: empty,
                children: [
                    "No workflows yet. Click ",
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("strong", {
                        children: "+ New Workflow"
                    }),
                    " to create one."
                ]
            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: tableWrapper,
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("table", {
                    className: table,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("thead", {
                            children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: th,
                                        children: "Slug"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: th,
                                        children: "Name"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: th,
                                        children: "Target Directory"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: th,
                                        children: "Schedule"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: th,
                                        children: "Actions"
                                    })
                                ]
                            })
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("tbody", {
                            children: workflows.map((wf)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                            className: WorkflowsPanel_css_row,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: td,
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                        className: slugCell,
                                                        children: [
                                                            "@",
                                                            wf.slug
                                                        ]
                                                    })
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("td", {
                                                    className: td,
                                                    children: [
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                                            children: wf.name
                                                        }),
                                                        wf.description && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                                            style: {
                                                                fontSize: "0.75rem",
                                                                opacity: 0.7
                                                            },
                                                            children: wf.description
                                                        })
                                                    ]
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: td,
                                                    children: wf.targetDirectory || "—"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: td,
                                                    children: wf.cronExpression ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                        className: [
                                                            cronBadge,
                                                            wf.cronEnabled ? cronEnabled : cronDisabled
                                                        ].join(" "),
                                                        children: [
                                                            wf.cronEnabled ? "⏰" : "⏸",
                                                            " ",
                                                            wf.cronExpression
                                                        ]
                                                    }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: cronDisabled + " " + cronBadge,
                                                        children: "Manual"
                                                    })
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: td,
                                                    children: confirmDeleteId === wf.id ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                        className: actions,
                                                        children: [
                                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                                style: {
                                                                    fontSize: "0.75rem",
                                                                    color: "var(--text-secondary)"
                                                                },
                                                                children: [
                                                                    'Delete "',
                                                                    wf.name,
                                                                    '"?'
                                                                ]
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                className: deleteButton,
                                                                onClick: ()=>void handleDeleteConfirm(),
                                                                children: "Yes, delete"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                className: editButton,
                                                                onClick: handleDeleteCancel,
                                                                children: "Cancel"
                                                            })
                                                        ]
                                                    }) : confirmArchiveId === wf.id ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                        className: actions,
                                                        children: [
                                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                                style: {
                                                                    fontSize: "0.75rem",
                                                                    color: "var(--text-secondary)"
                                                                },
                                                                children: [
                                                                    'Archive all sessions for "',
                                                                    wf.name,
                                                                    '"?'
                                                                ]
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                className: deleteButton,
                                                                onClick: ()=>void handleArchiveConfirm(),
                                                                "data-testid": "confirm-archive-sessions",
                                                                children: "Yes, archive"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                className: editButton,
                                                                onClick: ()=>setConfirmArchiveId(null),
                                                                children: "Cancel"
                                                            })
                                                        ]
                                                    }) : confirmDeleteFailedId === wf.id ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                        className: actions,
                                                        children: [
                                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                                style: {
                                                                    fontSize: "0.75rem",
                                                                    color: "var(--text-secondary)"
                                                                },
                                                                children: [
                                                                    'Delete failed sessions for "',
                                                                    wf.name,
                                                                    '"?'
                                                                ]
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                className: deleteButton,
                                                                onClick: ()=>void handleDeleteFailedConfirm(),
                                                                "data-testid": "confirm-delete-failed-sessions",
                                                                children: "Yes, delete"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                className: editButton,
                                                                onClick: ()=>setConfirmDeleteFailedId(null),
                                                                children: "Cancel"
                                                            })
                                                        ]
                                                    }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                        className: actions,
                                                        children: [
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                className: runButton,
                                                                disabled: runningId === wf.id,
                                                                onClick: ()=>void handleRun(wf),
                                                                children: runningId === wf.id ? "…" : "▶ Run"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                className: editButton,
                                                                onClick: ()=>openEdit(wf),
                                                                children: "Edit"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                className: editButton,
                                                                disabled: archivingId === wf.id,
                                                                onClick: ()=>setConfirmArchiveId(wf.id),
                                                                "data-testid": "archive-sessions-button",
                                                                children: archivingId === wf.id ? "…" : "Archive Sessions"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                className: editButton,
                                                                disabled: deletingFailedId === wf.id,
                                                                onClick: ()=>setConfirmDeleteFailedId(wf.id),
                                                                "data-testid": "delete-failed-sessions-button",
                                                                children: deletingFailedId === wf.id ? "…" : "Delete Failed"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                className: deleteButton,
                                                                onClick: ()=>handleDeleteClick(wf),
                                                                children: "Delete"
                                                            })
                                                        ]
                                                    })
                                                })
                                            ]
                                        }, wf.id),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("tr", {
                                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                colSpan: 5,
                                                style: {
                                                    padding: 0,
                                                    borderBottom: "1px solid var(--border-color)"
                                                },
                                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(RecentRuns, {
                                                    workflowId: wf.id
                                                })
                                            })
                                        }, wf.id + "-runs")
                                    ]
                                }))
                        })
                    ]
                })
            }),
            formModal
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Fworkflows%2Fpage.css.ts.vanilla.css%22%2C%22source%22%3A%22LnV5Z3c2OTAgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBtaW4taGVpZ2h0OiBjYWxjKHZhcigtLXZpZXdwb3J0LWhlaWdodCwgMTAwZHZoKSAtIHZhcigtLWhlYWRlci1oZWlnaHQpKTsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejYpOwp9Ci51eWd3NjkxIHsKICBmbGV4OiAxOwogIG1heC13aWR0aDogMTIwMHB4OwogIHdpZHRoOiAxMDAlOwogIG1hcmdpbjogMCBhdXRvOwogIHBhZGRpbmc6IDJyZW07CiAgb3ZlcmZsb3cteTogYXV0bzsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXgtZGlyZWN0aW9uOiBjb2x1bW47CiAgZ2FwOiAycmVtOwp9CkBtZWRpYSBzY3JlZW4gYW5kIChtYXgtd2lkdGg6IDc2OHB4KSB7CiAgLnV5Z3c2OTEgewogICAgcGFkZGluZzogMXJlbTsKICB9Cn0%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Fworkflows_2Fpage_css_ts_vanilla_css_22_2C_22source_22_3A_22LnV5Z3c2OTAgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBtaW4taGVpZ2h0OiBjYWxjKHZhcigtLXZpZXdwb3J0LWhlaWdodCwgMTAwZHZoKSAtIHZhcigtLWhlYWRlci1oZWlnaHQpKTsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejYpOwp9Ci51eWd3NjkxIHsKICBmbGV4OiAxOwogIG1heC13aWR0aDogMTIwMHB4OwogIHdpZHRoOiAxMDAlOwogIG1hcmdpbjogMCBhdXRvOwogIHBhZGRpbmc6IDJyZW07CiAgb3ZlcmZsb3cteTogYXV0bzsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXgtZGlyZWN0aW9uOiBjb2x1bW47CiAgZ2FwOiAycmVtOwp9CkBtZWRpYSBzY3JlZW4gYW5kIChtYXgtd2lkdGg6IDc2OHB4KSB7CiAgLnV5Z3c2OTEgewogICAgcGFkZGluZzogMXJlbTsKICB9Cn0_22_7D = __webpack_require__(16319);
;// ./src/app/workflows/page.css.ts


var main = 'uygw691';
var page = 'uygw690';
;// ./src/app/workflows/page.tsx
// analytics-exempt
/* __next_internal_client_entry_do_not_use__ default auto */ 
// +feature: workflows-management



function WorkflowsPageInner() {
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: page,
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("main", {
            id: "main-content",
            className: main,
            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(WorkflowsPanel, {})
        })
    });
}
function WorkflowsPage() {
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(react.Suspense, {
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(WorkflowsPageInner, {})
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
/******/ __webpack_require__.O(0, [1869,852,4202,9353,8182,7924,6000,2099,7888,8165,3575,2513,2275,3117,4247,9691,3516,7358], () => (__webpack_exec__(20188)));
/******/ var __webpack_exports__ = __webpack_require__.O();
/******/ _N_E = __webpack_exports__;
/******/ }
]);
//# sourceMappingURL=page-d8704c1b3b8b2cca.js.map