(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[6173],{

/***/ 2816:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 6112:
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

/***/ 21791:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  I: () => (/* binding */ CollapsibleGroup),
  i: () => (/* binding */ CollapsibleSection)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@radix-ui+react-accordion@1.2.17+b721ee6ccd61acaa/node_modules/@radix-ui/react-accordion/dist/index.mjs + 14 modules
var dist = __webpack_require__(46252);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fui%2FCollapsible.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA5VU24rbMBB9z1cMC4XNgxbbm7RdhcL-RymLIo1tNbbGSOPcSv69WImT2EmW9s1ozjkzZy5-X-Eu96rGAB-pKxNCnsGfCUDuqY4fACXaomQJyWICcJgAMI0ia-WfhfDK2K1QWpM3lpzQ5BgdiyNqemQfJu-3KeePUv6H8J2ykj7lS58oiYiNNVxKSJPky-I6msbokrxBL5bETLWEtNlCoMqaUzEf6RvuM7_H6ZArKxVY6NJW5p6MI4cDQhZRxoamUjsJeYXbxbg2gNo60duZzZoIUZUtnLCMdZCg0TH67vl3G9jmu747EkKjNIol8gbRdYhCNXLkItvH9jXKGOuKcfQ1md7HL5VeFZ5aZySwVy40yqPjxdl3bxhAtz6Ql9CQ7SvVVHUvQ-UkCufkWAS7x5tS9CW-GWzHGVFGBOOWRWyRhApzHjZdlrRGfxzQlYeh0vfpiJSTboNY22CXFUYytVxZhxKyB-tRx2JOMEF5HpAliKwb4ZX263ALrIvwfhn-ddKXUMcUofTWrU77H8dj2ZI7jSonX0Pyks4DoAr4cCDZqAs_jWIlAivGH0_UoHv6BSMfZ30Jnjrg81tisBgKfT02cI0-r2gjobTGdPt5DbmXq2MpZ2t19HL5XV28CGr5EyFdUUDzWGr-mdS3yDofSnJ7GLfHs5gc_gLw6CcyYQUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FCollapsible_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA5VU24rbMBB9z1cMC4XNgxbbm7RdhcL_RymLIo1tNbbGSOPcSv69WImT2EmW9s1ozjkzZy5_X_Eu96rGAB_pKxNCnsGfCUDuqY4fACXaomQJyWICcJgAMI0ia_WfhfDK2K1QWpM3lpzQ5BgdiyNqemQfJu_3KeePUv6H8J2ykj7lS58oiYiNNVxKSJPky_I6msbokrxBL5bETLWEtNlCoMqaUzEf6RvuM7_H6ZArKxVY6NJW5p6MI4cDQhZRxoamUjsJeYXbxbg2gNo60duZzZoIUZUtnLCMdZCg0TH67vl3G9jmu747EkKjNIol8gbRdYhCNXLkItvH9jXKGOuKcfQ1md7HL5VeFZ5aZySwVy40yqPjxdl3bxhAtz6Ql9CQ7SvVVHUvQ_UkCufkWAS7x5tS9CW_GWzHGVFGBOOWRWyRhApzHjZdlrRGfxzQlYeh0vfpiJSTboNY22CXFUYytVxZhxKyB_tRx2JOMEF5HpAliKwb4ZX263ALrIvwfhn_ddKXUMcUofTWrU77H8dj2ZI7jSonX0Pyks4DoAr4cCDZqAs_jWIlAivGH0_UoHv6BSMfZ30Jnjrg81tisBgKfT02cI0_r2gjobTGdPt5DbmXq2MpZ2t19HL5XV28CGr5EyFdUUDzWGr_mdS3yDofSnJ7GLfHs5gc_gLw6CcyYQUAAA_22_7D = __webpack_require__(59936);
;// ./src/components/ui/Collapsible.css.ts


var chevron = '_1nh0oet3';
var content = '_1nh0oet6';
var contentInner = '_1nh0oet7';
var header = '_1nh0oet2';
var item = '_1nh0oet1';
var root = '_1nh0oet0';
;// ./src/components/ui/Collapsible.tsx
/* __next_internal_client_entry_do_not_use__ CollapsibleGroup,CollapsibleSection auto */ 



const CollapsibleGroupContext = /*#__PURE__*/ (0,react.createContext)({
    insideGroup: false,
    openKeys: undefined
});
/**
 * Wraps a single Accordion.Root (type="multiple") shared by every child
 * CollapsibleSection — render once around a group of sibling sections so
 * their headers share one Radix roving-tabindex context (Task 1.1.1c).
 */ function CollapsibleGroup(param) {
    let { children, className, defaultValue, value, onValueChange } = param;
    const rootClassName = className ? "".concat(root, " ").concat(className) : root;
    // The group's actual initial-open set for this render: the live
    // controlled `value` when present, else the uncontrolled `defaultValue`
    // Radix seeds its Root with. Either way, this is what determines each
    // section's real open/closed state — never a child's own defaultExpanded.
    const openKeys = value !== undefined ? value : defaultValue !== null && defaultValue !== void 0 ? defaultValue : [];
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(CollapsibleGroupContext.Provider, {
        value: {
            insideGroup: true,
            openKeys
        },
        children: value !== undefined ? /*#__PURE__*/ (0,jsx_runtime.jsx)(dist/* Root */.bL, {
            type: "multiple",
            className: rootClassName,
            value: value,
            onValueChange: onValueChange,
            children: children
        }) : /*#__PURE__*/ (0,jsx_runtime.jsx)(dist/* Root */.bL, {
            type: "multiple",
            className: rootClassName,
            defaultValue: defaultValue !== null && defaultValue !== void 0 ? defaultValue : [],
            onValueChange: onValueChange,
            children: children
        })
    });
}
function CollapsibleItem(param) {
    let { sectionKey, title, children, className } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(dist/* Item */.q7, {
        value: sectionKey,
        className: className ? "".concat(item, " ").concat(className) : item,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)(dist/* Header */.Y9, {
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)(dist/* Trigger */.l9, {
                    className: header,
                    "data-testid": "collapsible-header-".concat(sectionKey),
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            children: title
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: chevron,
                            "aria-hidden": "true",
                            children: "▸"
                        })
                    ]
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(dist/* Content */.UC, {
                className: content,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: contentInner,
                    children: children
                })
            })
        ]
    });
}
/**
 * A single progressive-disclosure section: a real `<button aria-expanded>`
 * header (never a `<div onClick>`) whose body is removed from the DOM (not
 * just visually hidden) while collapsed.
 *
 * Must be rendered either as a descendant of a `CollapsibleGroup` (to share
 * that group's single Radix Root and its cross-header keyboard nav), or
 * standalone — in which case it mounts its own implicit single-item
 * `Accordion.Root` so existing single-section call sites need no changes.
 */ function CollapsibleSection(param) {
    let { sectionKey, title, defaultExpanded = false, onExpandedChange, children, className } = param;
    const { insideGroup, openKeys } = (0,react.useContext)(CollapsibleGroupContext);
    if (insideGroup) {
        if (false) { var _openKeys_includes; }
        return /*#__PURE__*/ (0,jsx_runtime.jsx)(CollapsibleItem, {
            sectionKey: sectionKey,
            title: title,
            className: className,
            children: children
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(dist/* Root */.bL, {
        type: "multiple",
        className: root,
        defaultValue: defaultExpanded ? [
            sectionKey
        ] : [],
        onValueChange: (next)=>onExpandedChange === null || onExpandedChange === void 0 ? void 0 : onExpandedChange(next.includes(sectionKey)),
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(CollapsibleItem, {
            sectionKey: sectionKey,
            title: title,
            className: className,
            children: children
        })
    });
}


/***/ }),

/***/ 24031:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   X: () => (/* binding */ connectApi)
/* harmony export */ });
/* harmony import */ var _reduxjs_toolkit_query_react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(90453);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(45640);


// Generic base query for ConnectRPC unary calls.
// arg shape: { call: () => Promise<unknown> }
const connectBaseQuery = async (param)=>{
    let { call } = param;
    try {
        const result = await call();
        return {
            data: result
        };
    } catch (err) {
        if (err instanceof _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T) {
            // Store rawMessage, not message — .message is ConnectRPC's
            // `[code] message`-prefixed string, and callers reading this error
            // slice (e.g. BacklogItemDetail.tsx's lastError?.message reads)
            // display it verbatim to users.
            return {
                error: {
                    status: err.code,
                    error: err.rawMessage || err.message
                }
            };
        }
        const msg = err instanceof Error ? err.message : "Unknown error";
        return {
            error: {
                status: -1,
                error: msg
            }
        };
    }
};
const connectApi = (0,_reduxjs_toolkit_query_react__WEBPACK_IMPORTED_MODULE_1__/* .createApi */ .xP)({
    reducerPath: "connectApi",
    baseQuery: connectBaseQuery,
    tagTypes: [
        "Approvals",
        "GuidanceRequests"
    ],
    endpoints: ()=>({})
});


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

/***/ 24621:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Ay: () => (__WEBPACK_DEFAULT_EXPORT__),
/* harmony export */   N0: () => (/* binding */ toggleSelection),
/* harmony export */   Ub: () => (/* binding */ selectAll),
/* harmony export */   wr: () => (/* binding */ clearSelection),
/* harmony export */   xc: () => (/* binding */ selectionAfterBulk)
/* harmony export */ });
/* harmony import */ var _reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(73559);

const initialState = {
    selectedIds: [],
    selectMode: false
};
// Separate from bulkSelectionSlice (sessions) so selecting backlog rows never
// touches session selection.
const backlogBulkSelectionSlice = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_0__/* .createSlice */ .Z0)({
    name: "backlogBulkSelection",
    initialState,
    reducers: {
        toggleSelection (state, action) {
            const idx = state.selectedIds.indexOf(action.payload);
            if (idx >= 0) {
                state.selectedIds.splice(idx, 1);
            } else {
                state.selectedIds.push(action.payload);
            }
            state.selectMode = true;
        },
        selectAll (state, action) {
            state.selectedIds = action.payload;
            state.selectMode = action.payload.length > 0;
        },
        clearSelection (state) {
            state.selectedIds = [];
            state.selectMode = false;
        }
    }
});
const { toggleSelection, selectAll, clearSelection } = backlogBulkSelectionSlice.actions;
/** Action to apply after a bulk fan-out: clear on full success, otherwise keep only the failed ids selected. */ const selectionAfterBulk = (failedIds)=>failedIds.length === 0 ? clearSelection() : selectAll(failedIds);
/* harmony default export */ const __WEBPACK_DEFAULT_EXPORT__ = (backlogBulkSelectionSlice.reducer);


/***/ }),

/***/ 25337:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  M_: () => (/* binding */ store),
  jL: () => (/* binding */ useAppDispatch),
  GV: () => (/* binding */ useAppSelector)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@reduxjs+toolkit@2.11.2+71d2413d80c012e4/node_modules/@reduxjs/toolkit/dist/redux-toolkit.modern.mjs + 1 modules
var redux_toolkit_modern = __webpack_require__(73559);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-redux@9.2.0+5436edeee5f9f735/node_modules/react-redux/dist/react-redux.mjs
var react_redux = __webpack_require__(55324);
// EXTERNAL MODULE: ./src/lib/store/backlogItemsSlice.ts
var backlogItemsSlice = __webpack_require__(63939);
// EXTERNAL MODULE: ./src/lib/store/backlogBulkSelectionSlice.ts
var backlogBulkSelectionSlice = __webpack_require__(24621);
;// ./src/lib/store/bulkSelectionSlice.ts

const initialState = {
    selectedIds: []
};
const bulkSelectionSlice = (0,redux_toolkit_modern/* createSlice */.Z0)({
    name: 'bulkSelection',
    initialState,
    reducers: {
        toggleSelection (state, action) {
            const id = action.payload;
            const idx = state.selectedIds.indexOf(id);
            if (idx >= 0) {
                state.selectedIds.splice(idx, 1);
            } else {
                state.selectedIds.push(id);
            }
        },
        selectAll (state, action) {
            state.selectedIds = action.payload;
        },
        clearSelection (state) {
            state.selectedIds = [];
        }
    }
});
const { toggleSelection, selectAll, clearSelection } = bulkSelectionSlice.actions;
/* harmony default export */ const store_bulkSelectionSlice = (bulkSelectionSlice.reducer);

// EXTERNAL MODULE: ./src/lib/store/remotesSlice.ts
var remotesSlice = __webpack_require__(81921);
// EXTERNAL MODULE: ./src/lib/store/reviewQueueSlice.ts
var reviewQueueSlice = __webpack_require__(68453);
// EXTERNAL MODULE: ./src/lib/store/sessionsSlice.ts
var sessionsSlice = __webpack_require__(60795);
// EXTERNAL MODULE: ./src/lib/api/connectApi.ts
var connectApi = __webpack_require__(24031);
;// ./src/lib/store/store.ts









const store = (0,redux_toolkit_modern/* configureStore */.U1)({
    reducer: {
        backlogItems: backlogItemsSlice/* default */.Ay,
        backlogBulkSelection: backlogBulkSelectionSlice/* default */.Ay,
        bulkSelection: store_bulkSelectionSlice,
        remotes: remotesSlice/* default */.Ay,
        reviewQueue: reviewQueueSlice/* default */.Ay,
        sessions: sessionsSlice/* default */.Ay,
        [connectApi/* connectApi */.X.reducerPath]: connectApi/* connectApi */.X.reducer
    },
    middleware: (getDefaultMiddleware)=>getDefaultMiddleware().concat(connectApi/* connectApi */.X.middleware)
});
// Typed hooks for use throughout the app
const useAppDispatch = react_redux/* useDispatch */.wA.withTypes();
const useAppSelector = react_redux/* useSelector */.d4.withTypes();


/***/ }),

/***/ 31587:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  NotificationsPage: () => (/* binding */ NotificationsPage)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/store/index.ts
var store = __webpack_require__(93602);
// EXTERNAL MODULE: ./src/lib/store/sessionsSlice.ts
var sessionsSlice = __webpack_require__(60795);
// EXTERNAL MODULE: ./src/lib/contexts/NotificationContext.tsx + 12 modules
var NotificationContext = __webpack_require__(62275);
// EXTERNAL MODULE: ./src/lib/hooks/useAuditLog.ts
var useAuditLog = __webpack_require__(13867);
// EXTERNAL MODULE: ./src/lib/hooks/useApprovalResolution.ts
var useApprovalResolution = __webpack_require__(37349);
// EXTERNAL MODULE: ./src/lib/hooks/useClearResultLine.ts
var useClearResultLine = __webpack_require__(15562);
// EXTERNAL MODULE: ./src/lib/utils/notificationGrouping.ts
var notificationGrouping = __webpack_require__(65232);
// EXTERNAL MODULE: ./src/lib/utils/notificationMapping.ts
var notificationMapping = __webpack_require__(71249);
// EXTERNAL MODULE: ./src/components/ui/NotificationItem.tsx + 12 modules
var NotificationItem = __webpack_require__(44543);
// EXTERNAL MODULE: ./src/components/ui/Collapsible.tsx + 1 modules
var Collapsible = __webpack_require__(21791);
// EXTERNAL MODULE: ./src/components/ui/NotificationPanel.css.ts
var NotificationPanel_css = __webpack_require__(82254);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Fnotifications%2FNotificationsPage.css.ts.vanilla.css%22%2C%22source%22%3A%22LmhxNHRiMCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGhlaWdodDogMTAwJTsKICBvdmVyZmxvdy15OiBhdXRvOwogIG92ZXJmbG93LXg6IGhpZGRlbjsKICBtYXgtd2lkdGg6IDgwMHB4OwogIG1hcmdpbjogMCBhdXRvOwogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MzEpIHZhcigtLV8xOWV6MnJ6MzEpIDA7Cn0KQG1lZGlhIHNjcmVlbiBhbmQgKG1heC13aWR0aDogNzY4cHgpIHsKICAuaHE0dGIwIHsKICAgIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MnopIHZhcigtLV8xOWV6MnJ6MnopIDA7CiAgfQp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Fnotifications_2FNotificationsPage_css_ts_vanilla_css_22_2C_22source_22_3A_22LmhxNHRiMCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGhlaWdodDogMTAwJTsKICBvdmVyZmxvdy15OiBhdXRvOwogIG92ZXJmbG93LXg6IGhpZGRlbjsKICBtYXgtd2lkdGg6IDgwMHB4OwogIG1hcmdpbjogMCBhdXRvOwogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MzEpIHZhcigtLV8xOWV6MnJ6MzEpIDA7Cn0KQG1lZGlhIHNjcmVlbiBhbmQgKG1heC13aWR0aDogNzY4cHgpIHsKICAuaHE0dGIwIHsKICAgIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MnopIHZhcigtLV8xOWV6MnJ6MnopIDA7CiAgfQp9_22_7D = __webpack_require__(64006);
;// ./src/app/notifications/NotificationsPage.css.ts


var pageRoot = 'hq4tb0';
;// ./src/app/notifications/NotificationsPage.tsx
/* __next_internal_client_entry_do_not_use__ NotificationsPage auto */ 
// +feature: ui:notifications-page













const TYPE_FILTER_LABELS = {
    all: "All",
    approval_needed: "Approval",
    error: "Error",
    task_complete: "Task",
    info: "Info"
};
/** Minute-rounding elapsed-time helper for the staleness indicator (Task 3.1.2h) — a
 * one-line helper rather than a new dependency, matching Task 3.1.2h's guidance. */ function minutesAgoLabel(sinceMs) {
    const minutes = Math.round((Date.now() - sinceMs) / 60000);
    return minutes <= 0 ? "just now" : "".concat(minutes, "m");
}
function NotificationsPage() {
    const { notificationHistory, markAsRead, removeFromHistory, acknowledgeNotification, clearHistoryByIds, showActionToast, getUnreadCount, historyLoading, historyHasMore, historyError, historyLastUpdatedAt, loadMoreHistory, refreshHistory } = (0,NotificationContext/* useNotifications */.E$)();
    const auditLog = (0,useAuditLog/* useAuditLog */.r)();
    const { keptLine, applyClearResult, clearKeptLine } = (0,useClearResultLine/* useClearResultLine */.s)();
    // Epic 3.3 (session-completion-summary), Story 3.3.2: a notification's
    // sessionId may reference a session that's since been deleted from the
    // live list (e.g. after DeleteSession). In that case "View Session" falls
    // back to the durable standalone summary route instead of a dead/no-op
    // `/?session=<id>` link.
    const liveSessions = (0,store/* useAppSelector */.GV)(sessionsSlice/* selectAllSessions */.rt);
    const liveSessionIds = (0,react.useMemo)(()=>new Set(liveSessions.map((s)=>s.id)), [
        liveSessions
    ]);
    // Some notifications (e.g. SendNotification's poller-miss fallback in
    // notification_service.go) were recorded with the session's title rather than its
    // stable id — page.tsx's findSessionById already tolerates this by falling back to a
    // title match, so this liveness check has to match it or it wrongly reports a live
    // session as gone. See the reproduced case in NotificationsPage.test.tsx.
    const liveSessionTitles = (0,react.useMemo)(()=>new Set(liveSessions.map((s)=>s.title.toLowerCase())), [
        liveSessions
    ]);
    const hasLoadedSessionsOnce = (0,store/* useAppSelector */.GV)(sessionsSlice/* selectSessionsHasLoadedOnce */.Qr);
    const { resolvedApprovals, pendingApprovals, blockedApprovals, failedApprovals, resolveApproval } = (0,useApprovalResolution/* useApprovalResolution */.b)({
        notificationHistory,
        acknowledgeNotification
    });
    const [searchQuery, setSearchQuery] = (0,react.useState)("");
    const [typeFilter, setTypeFilter] = (0,react.useState)("all");
    const [hideBacklogItems, setHideBacklogItems] = (0,react.useState)(false);
    const [autoHandledOpen, setAutoHandledOpen] = (0,react.useState)(false);
    // Task 3.1.2c: resets exactly the three state variables hasActiveFilter (below)
    // already tracks, so "Clear filter" can never drift out of sync with what the
    // page itself considers "a filter is active."
    const clearFilters = (0,react.useCallback)(()=>{
        setSearchQuery("");
        setTypeFilter("all");
        setHideBacklogItems(false);
    }, []);
    const filteredNotifications = (0,react.useMemo)(()=>{
        // Task 3.1.3a: a rule-reconciled item is excluded from the main feed the
        // same way a live auto_approved item is — both surface only in AutoHandledSection.
        let items = notificationHistory.filter((n)=>n.notificationType !== "auto_approved" && !(0,notificationMapping/* isReconciledNotification */.yL)(n));
        if (typeFilter !== "all") {
            const allowed = new Set((0,notificationMapping/* notificationTypeFilter */.uI)(typeFilter, items.map((n)=>n.notificationType)));
            items = items.filter((n)=>allowed.has(n.notificationType));
        }
        if (hideBacklogItems) {
            items = items.filter((n)=>{
                var _n_metadata;
                return !((_n_metadata = n.metadata) === null || _n_metadata === void 0 ? void 0 : _n_metadata["item_id"]);
            });
        }
        if (searchQuery.trim()) {
            const q = searchQuery.toLowerCase();
            items = items.filter((n)=>{
                var _n_metadata, _n_metadata1, _n_metadata2;
                return (n.sessionName || "").toLowerCase().includes(q) || (n.message || "").toLowerCase().includes(q) || (n.title || "").toLowerCase().includes(q) || (n.sourceProject || "").toLowerCase().includes(q) || (n.sourceWorkingDir || "").toLowerCase().includes(q) || (((_n_metadata = n.metadata) === null || _n_metadata === void 0 ? void 0 : _n_metadata["tool_name"]) || "").toLowerCase().includes(q) || (((_n_metadata1 = n.metadata) === null || _n_metadata1 === void 0 ? void 0 : _n_metadata1["tool_input_command"]) || "").toLowerCase().includes(q) || (((_n_metadata2 = n.metadata) === null || _n_metadata2 === void 0 ? void 0 : _n_metadata2["tool_input_file"]) || "").toLowerCase().includes(q);
            });
        }
        return items;
    }, [
        notificationHistory,
        typeFilter,
        hideBacklogItems,
        searchQuery
    ]);
    // Story 3.1.2: split the filtered set into the always-expanded "needs a
    // decision" tier and everything else ("recent activity" — never
    // "informational", which is Task 3.2.1a's Review Queue tier name).
    const needsDecision = (0,react.useMemo)(()=>filteredNotifications.filter((n)=>n.isPendingDecision === true), [
        filteredNotifications
    ]);
    const recentActivity = (0,react.useMemo)(()=>filteredNotifications.filter((n)=>n.isPendingDecision !== true), [
        filteredNotifications
    ]);
    // The true unfiltered actionable count — deliberately computed over
    // notificationHistory, never filteredNotifications/needsDecision, so
    // NeedsDecisionSection can tell "nothing needs a decision" apart from "a
    // filter is hiding something that does" (Product Triad Review round-4 fix).
    const totalActionableCount = (0,react.useMemo)(()=>notificationHistory.filter((n)=>n.isPendingDecision === true).length, [
        notificationHistory
    ]);
    const hasActiveFilter = searchQuery.trim() !== "" || typeFilter !== "all" || hideBacklogItems;
    // The search box and "Hide backlog" toggle only filter over notificationHistory
    // that's already been paged into memory (see filteredNotifications above) — they
    // never re-query the server for older, not-yet-loaded history. When more history
    // remains (historyHasMore) and one of those two filters is active, surface that
    // instead of silently returning incomplete results.
    const hasIncompleteSearch = (searchQuery.trim() !== "" || hideBacklogItems) && historyHasMore;
    const autoHandledNotifications = (0,react.useMemo)(()=>notificationHistory.filter((n)=>n.notificationType === "auto_approved" || (0,notificationMapping/* isReconciledNotification */.yL)(n)), [
        notificationHistory
    ]);
    const unreadCount = getUnreadCount();
    // Task 3.1.2e: "Mark activity read" only ever touches Recent Activity +
    // Auto-handled — never an item in NeedsDecisionSection. Shared with
    // NotificationPanel's identical button (Task 3.1.5a) via computeScopedMarkReadIds
    // so the scoping rule is defined once.
    const scopedMarkReadIds = (0,react.useMemo)(()=>(0,notificationMapping/* computeScopedMarkReadIds */.f)([
            ...recentActivity,
            ...autoHandledNotifications
        ]), [
        recentActivity,
        autoHandledNotifications
    ]);
    const handleMarkActivityRead = (0,react.useCallback)(()=>{
        clearKeptLine();
        markAsRead(scopedMarkReadIds);
    }, [
        markAsRead,
        scopedMarkReadIds,
        clearKeptLine
    ]);
    const handleClearHistory = (0,react.useCallback)(()=>{
        // Irreversible, so gate behind a confirm. The ids are every read row the server field
        // says is not a pending decision; the server guards the same predicate again and lists
        // anything it kept, which the shared helper turns into the "N kept" line (Task 4.4f).
        if (!window.confirm("Clear read notifications? This can't be undone. Items still needing a decision won't be cleared.")) {
            return;
        }
        clearKeptLine();
        const ids = notificationHistory.filter((n)=>n.isRead && !n.isPendingDecision).map((n)=>n.id);
        clearHistoryByIds(ids).then(applyClearResult).catch(()=>showActionToast("Could not clear notifications", "error", "notifications-clear"));
    }, [
        notificationHistory,
        clearHistoryByIds,
        applyClearResult,
        clearKeptLine,
        showActionToast
    ]);
    const handleNotificationClick = (ids, onView, sessionId)=>{
        markAsRead(ids);
        const primaryId = Array.isArray(ids) ? ids[0] : ids;
        if (onView && sessionId) {
            auditLog.logNotificationSessionViewed(primaryId, sessionId);
            onView();
        } else if (onView) {
            auditLog.logNotificationViewed(primaryId, sessionId);
            onView();
        }
    };
    const getSessionHref = (0,react.useCallback)((sessionId)=>// Before the sessions store has ever loaded, an absent id means "haven't
        // heard yet," not "confirmed gone" — default to the live route so a fast
        // click right after page load doesn't race the first WatchSessions
        // snapshot and land on the summary route for a session that's actually live.
        // Also accept a title match (see liveSessionTitles above) since some
        // recorded notifications carry the title in place of the stable id.
        !hasLoadedSessionsOnce || liveSessionIds.has(sessionId) || liveSessionTitles.has(sessionId.toLowerCase()) ? "/?session=".concat(encodeURIComponent(sessionId)) : "/sessions/summary?sessionId=".concat(encodeURIComponent(sessionId)), [
        liveSessionIds,
        liveSessionTitles,
        hasLoadedSessionsOnce
    ]);
    // Task 3.1.2h (AC38): background fetch-failure staleness indicator — only
    // shown once a failure occurs, never implying the last-known list is fresher
    // than it is.
    const staleness = (0,react.useMemo)(()=>historyError && historyLastUpdatedAt !== null ? {
            label: minutesAgoLabel(historyLastUpdatedAt),
            onRetry: ()=>void refreshHistory()
        } : undefined, [
        historyError,
        historyLastUpdatedAt,
        refreshHistory
    ]);
    // "Recent activity" CollapsibleSection — identical between the two branches
    // below (one totalActionableCount>0-but-empty-needsDecision, one the normal
    // path); only the outer `recentActivity.length > 0` guard differs per call
    // site, so it's applied by each caller rather than baked in here (mirrors
    // ReviewQueuePanel.tsx's renderTierItems/groupTierItems local-function pattern).
    const renderRecentActivitySection = ()=>/*#__PURE__*/ (0,jsx_runtime.jsx)(Collapsible/* CollapsibleSection */.i, {
            sectionKey: "recent-activity",
            title: "Recent activity \xb7 ".concat(recentActivity.length),
            children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: NotificationPanel_css/* list */.p_,
                children: [
                    (0,notificationGrouping/* groupNotifications */._Q)(recentActivity).map((group)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(NotificationItem/* NotificationItem */.rn, {
                            group: group,
                            resolvedApprovals: resolvedApprovals,
                            pendingApprovals: pendingApprovals,
                            blockedApprovals: blockedApprovals,
                            failedApprovals: failedApprovals,
                            resolveApproval: resolveApproval,
                            removeFromHistory: removeFromHistory,
                            handleNotificationClick: handleNotificationClick,
                            getSessionHref: getSessionHref
                        }, group.notification.id)),
                    historyHasMore && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: NotificationPanel_css/* loadMore */.UX,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: NotificationPanel_css/* loadMoreButton */.Qf,
                            onClick: loadMoreHistory,
                            disabled: historyLoading,
                            children: historyLoading ? "Loading..." : "Load more"
                        })
                    })
                ]
            })
        });
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: pageRoot,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: NotificationPanel_css/* header */.wx,
                "data-testid": "notifications-header",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("h2", {
                        className: NotificationPanel_css/* title */.DD,
                        "data-testid": "notifications-title",
                        children: [
                            "Notifications",
                            unreadCount > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: NotificationPanel_css/* unreadBadge */.HN,
                                "data-testid": "notifications-unread-badge",
                                children: (0,notificationMapping/* capBadgeCount */.BK)(unreadCount)
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: NotificationPanel_css/* headerActions */.$s,
                        children: notificationHistory.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                            children: [
                                scopedMarkReadIds.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: NotificationPanel_css/* markAllButton */.Vt,
                                    onClick: handleMarkActivityRead,
                                    "aria-label": "Mark activity as read",
                                    "data-testid": "notifications-mark-all-read",
                                    children: "Mark activity read"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: NotificationPanel_css/* clearButton */.mt,
                                    onClick: handleClearHistory,
                                    "aria-label": "Clear notification history",
                                    "data-testid": "notifications-clear-all",
                                    children: "Clear history"
                                })
                            ]
                        })
                    })
                ]
            }),
            keptLine && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                "data-testid": "notifications-kept-line",
                children: keptLine
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: NotificationPanel_css/* filterBar */.kT,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: NotificationPanel_css/* searchRow */.Pc,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                className: NotificationPanel_css/* searchInput */.Rt,
                                type: "search",
                                placeholder: "Search notifications…",
                                value: searchQuery,
                                onChange: (e)=>setSearchQuery(e.target.value),
                                "aria-label": "Search notifications"
                            }),
                            searchQuery && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: NotificationPanel_css/* searchClearButton */.aF,
                                onClick: ()=>setSearchQuery(""),
                                "aria-label": "Clear search",
                                type: "button",
                                children: "✕"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: NotificationPanel_css/* filterPills */.VS,
                        role: "group",
                        "aria-label": "Filter by type",
                        children: [
                            Object.keys(TYPE_FILTER_LABELS).map((filter)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: "".concat(NotificationPanel_css/* filterPill */._y, " ").concat(typeFilter === filter ? NotificationPanel_css/* filterPillActive */.s8 : ""),
                                    onClick: ()=>setTypeFilter(filter),
                                    "aria-pressed": typeFilter === filter,
                                    children: TYPE_FILTER_LABELS[filter]
                                }, filter)),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: "".concat(NotificationPanel_css/* filterPill */._y, " ").concat(hideBacklogItems ? NotificationPanel_css/* filterPillExcludeActive */.uk : ""),
                                onClick: ()=>setHideBacklogItems((v)=>!v),
                                "aria-pressed": hideBacklogItems,
                                "aria-label": "Exclude backlog notifications",
                                children: "\uD83D\uDEAB Hide backlog"
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: NotificationPanel_css/* content */.Qs,
                "data-testid": "notifications-content",
                children: [
                    hasIncompleteSearch && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: NotificationPanel_css/* incompleteSearchNotice */.Xn,
                        "data-testid": "incomplete-search-notice",
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                children: "Showing results from loaded history only — load more to search older notifications."
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: NotificationPanel_css/* incompleteSearchNoticeButton */.F_,
                                onClick: loadMoreHistory,
                                disabled: historyLoading,
                                type: "button",
                                "data-testid": "incomplete-search-load-more",
                                children: historyLoading ? "Loading..." : "Load more"
                            })
                        ]
                    }),
                    historyLoading && notificationHistory.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: NotificationPanel_css/* empty */.Ie,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: NotificationPanel_css/* emptyIcon */.Dw,
                                "aria-hidden": "true",
                                children: "⏳"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                className: NotificationPanel_css/* emptyText */.BI,
                                children: "Loading notifications..."
                            })
                        ]
                    }) : totalActionableCount > 0 && needsDecision.length === 0 ? // Product Triad Review round-5 blocker fix: this branch must be
                    // reachable even when recentActivity is ALSO empty — see Task 3.1.2c.
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(NotificationItem/* NeedsDecisionSection */.BF, {
                                notifications: [],
                                totalActionableCount: totalActionableCount,
                                onClearFilter: clearFilters,
                                resolvedApprovals: resolvedApprovals,
                                pendingApprovals: pendingApprovals,
                                blockedApprovals: blockedApprovals,
                                failedApprovals: failedApprovals,
                                resolveApproval: resolveApproval,
                                handleNotificationClick: handleNotificationClick,
                                getSessionHref: getSessionHref,
                                staleness: staleness
                            }),
                            recentActivity.length > 0 && renderRecentActivitySection()
                        ]
                    }) : filteredNotifications.length === 0 ? // Reachable only when totalActionableCount === 0 — the branch above
                    // already caught every case where it's nonzero.
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: NotificationPanel_css/* empty */.Ie,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: NotificationPanel_css/* emptyIcon */.Dw,
                                "aria-hidden": "true",
                                children: hasActiveFilter ? "🔍" : "🔔"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                className: NotificationPanel_css/* emptyText */.BI,
                                children: hasActiveFilter ? "No matching notifications" : "No notifications yet"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                className: NotificationPanel_css/* emptySubtext */.lP,
                                children: hasActiveFilter ? "Try adjusting your search or filter" : "You'll see notifications from your sessions here"
                            })
                        ]
                    }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(NotificationItem/* NeedsDecisionSection */.BF, {
                                notifications: needsDecision,
                                totalActionableCount: totalActionableCount,
                                onClearFilter: clearFilters,
                                resolvedApprovals: resolvedApprovals,
                                pendingApprovals: pendingApprovals,
                                blockedApprovals: blockedApprovals,
                                failedApprovals: failedApprovals,
                                resolveApproval: resolveApproval,
                                handleNotificationClick: handleNotificationClick,
                                getSessionHref: getSessionHref,
                                staleness: staleness
                            }),
                            renderRecentActivitySection()
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(NotificationItem/* AutoHandledSection */.FB, {
                notifications: autoHandledNotifications,
                isOpen: autoHandledOpen,
                onToggle: ()=>setAutoHandledOpen((v)=>!v)
            })
        ]
    });
}


/***/ }),

/***/ 35369:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 31587));
;
Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 3728));


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

/***/ 48443:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   V: () => (/* binding */ toPlainObject)
/* harmony export */ });
// Converts a protobuf Message to a plain JSON-serializable object.
// Used as the serialization boundary so Redux never stores protobuf class instances.
// In @bufbuild/protobuf v2, Message is a plain TypeScript type (not a class),
// so we use JSON round-trip to strip any non-serializable values.
function toPlainObject(msg) {
    // BigInt replacer: protobuf v2 represents int64/uint64 as bigint, which JSON.stringify rejects.
    // Timestamp seconds fit safely in Number (epoch seconds won't exceed 2^53 for millennia).
    return JSON.parse(JSON.stringify(msg, (_, v)=>typeof v === "bigint" ? Number(v) : v));
}


/***/ }),

/***/ 52162:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 60795:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   AY: () => (/* binding */ selectSessionsError),
/* harmony export */   Ay: () => (__WEBPACK_DEFAULT_EXPORT__),
/* harmony export */   Hx: () => (/* binding */ selectDetectedStatusMap),
/* harmony export */   Nh: () => (/* binding */ setError),
/* harmony export */   OS: () => (/* binding */ selectSessionsLoading),
/* harmony export */   QA: () => (/* binding */ setErrorCode),
/* harmony export */   Qr: () => (/* binding */ selectSessionsHasLoadedOnce),
/* harmony export */   YI: () => (/* binding */ setSessions),
/* harmony export */   aA: () => (/* binding */ setConnectionState),
/* harmony export */   dF: () => (/* binding */ removeSession),
/* harmony export */   ih: () => (/* binding */ upsertSession),
/* harmony export */   mN: () => (/* binding */ selectConnectionState),
/* harmony export */   od: () => (/* binding */ selectActiveSessionsSortedByUpdatedAt),
/* harmony export */   pu: () => (/* binding */ removeDetectedStatus),
/* harmony export */   r1: () => (/* binding */ setLoading),
/* harmony export */   rt: () => (/* binding */ selectAllSessions)
/* harmony export */ });
/* unused harmony exports selectSessionById, selectSessionIds, selectSessionsTotal, selectSessionsErrorCode */
/* harmony import */ var _reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(73559);
/* harmony import */ var _reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(13009);
/* harmony import */ var _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(81432);


const sessionsAdapter = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_1__/* .createEntityAdapter */ .pU)({
    selectId: (session)=>session.id
});
const initialState = sessionsAdapter.getInitialState({
    loading: false,
    hasLoadedOnce: false,
    error: null,
    detectedStatusMap: {},
    connectionState: "disconnected",
    deletedIds: {}
});
const sessionsSlice = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_1__/* .createSlice */ .Z0)({
    name: "sessions",
    initialState,
    reducers: {
        setSessions (state, action) {
            state.hasLoadedOnce = true;
            const filtered = action.payload.filter((s)=>!state.deletedIds[s.id]);
            // Preserve existing entity references when a session's data is unchanged
            // (same updatedAt) so React.memo on SessionRowWrapper can skip re-rendering
            // rows untouched by this snapshot/poll — mirrors the guard in upsertSession.
            const merged = filtered.map((incoming)=>{
                const existing = state.entities[incoming.id];
                if (existing && existing.updatedAt !== undefined && incoming.updatedAt !== undefined && existing.updatedAt.seconds === incoming.updatedAt.seconds && existing.updatedAt.nanos === incoming.updatedAt.nanos) {
                    return existing;
                }
                return incoming;
            });
            sessionsAdapter.setAll(state, merged);
        },
        upsertSession (state, action) {
            // Don't resurrect a deleted session via an in-flight update event
            if (!state.deletedIds[action.payload.id]) {
                // Skip no-op upserts: if updatedAt matches the existing record the data
                // hasn't changed and we'd just cause a spurious re-render. This prevents
                // the N-render storm from WatchSessions initial-snapshot events that
                // duplicate sessions already loaded by the preceding listSessions() call.
                const existing = state.entities[action.payload.id];
                const incoming = action.payload;
                if (existing && existing.updatedAt !== undefined && incoming.updatedAt !== undefined && existing.updatedAt.seconds === incoming.updatedAt.seconds && existing.updatedAt.nanos === incoming.updatedAt.nanos) {
                    return;
                }
                sessionsAdapter.upsertOne(state, action.payload);
                // Sync detectedStatusMap from the session's proto fields
                const session = action.payload;
                if (session.status !== _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SessionStatus */ .Ph.ACTIVE) {
                    // Non-active: clear badge unconditionally
                    delete state.detectedStatusMap[session.id];
                } else if (session.detectedStatus !== undefined && session.detectedStatus !== _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.UNSPECIFIED) {
                    var _session_detectedContext;
                    // Active with typed detection: update map from proto field
                    state.detectedStatusMap[session.id] = {
                        detectedStatus: session.detectedStatus,
                        detectedContext: (_session_detectedContext = session.detectedContext) !== null && _session_detectedContext !== void 0 ? _session_detectedContext : ""
                    };
                } else {
                    // Active + UNSPECIFIED: clear
                    delete state.detectedStatusMap[session.id];
                }
            }
        },
        removeSession (state, action) {
            sessionsAdapter.removeOne(state, action.payload);
            state.deletedIds[action.payload] = true;
        },
        setLoading (state, action) {
            state.loading = action.payload;
        },
        setError (state, action) {
            state.error = action.payload;
            // A fresh error/clear always invalidates the previously-classified code — callers that
            // need a code alongside the message dispatch setErrorCode immediately after setError.
            state.errorCode = undefined;
        },
        setErrorCode (state, action) {
            state.errorCode = action.payload;
        },
        setConnectionState (state, action) {
            state.connectionState = action.payload;
        },
        removeDetectedStatus (state, action) {
            delete state.detectedStatusMap[action.payload];
        }
    }
});
const { setSessions, upsertSession, removeSession, setLoading, setError, setErrorCode, setConnectionState, removeDetectedStatus } = sessionsSlice.actions;
// Use the adapter's built-in selectors scoped to the sessions slice
const adapterSelectors = sessionsAdapter.getSelectors((state)=>state.sessions);
const selectAllSessions = adapterSelectors.selectAll;
const selectSessionById = adapterSelectors.selectById;
// Memoized selector: active sessions pre-sorted by updatedAt descending.
// Avoids repeated O(n log n) sort inside render functions that show recent sessions.
const selectActiveSessionsSortedByUpdatedAt = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_2__/* .createSelector */ .Mz)(selectAllSessions, (sessions)=>sessions.filter((s)=>s.status !== _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SessionStatus */ .Ph.UNSPECIFIED).sort((a, b)=>{
        var _b_updatedAt, _a_updatedAt, _b_createdAt, _a_createdAt;
        var _b_updatedAt_seconds, _a_updatedAt_seconds;
        const byUpdated = Number((_b_updatedAt_seconds = (_b_updatedAt = b.updatedAt) === null || _b_updatedAt === void 0 ? void 0 : _b_updatedAt.seconds) !== null && _b_updatedAt_seconds !== void 0 ? _b_updatedAt_seconds : 0) - Number((_a_updatedAt_seconds = (_a_updatedAt = a.updatedAt) === null || _a_updatedAt === void 0 ? void 0 : _a_updatedAt.seconds) !== null && _a_updatedAt_seconds !== void 0 ? _a_updatedAt_seconds : 0);
        if (byUpdated !== 0) return byUpdated;
        var _b_createdAt_seconds, _a_createdAt_seconds;
        const byCreated = Number((_b_createdAt_seconds = (_b_createdAt = b.createdAt) === null || _b_createdAt === void 0 ? void 0 : _b_createdAt.seconds) !== null && _b_createdAt_seconds !== void 0 ? _b_createdAt_seconds : 0) - Number((_a_createdAt_seconds = (_a_createdAt = a.createdAt) === null || _a_createdAt === void 0 ? void 0 : _a_createdAt.seconds) !== null && _a_createdAt_seconds !== void 0 ? _a_createdAt_seconds : 0);
        if (byCreated !== 0) return byCreated;
        return a.id.localeCompare(b.id);
    }));
const selectSessionIds = adapterSelectors.selectIds;
const selectSessionsTotal = adapterSelectors.selectTotal;
const selectSessionsLoading = (state)=>state.sessions.loading;
const selectSessionsHasLoadedOnce = (state)=>state.sessions.hasLoadedOnce;
const selectSessionsError = (state)=>state.sessions.error;
const selectSessionsErrorCode = (state)=>state.sessions.errorCode;
const selectDetectedStatusMap = (state)=>state.sessions.detectedStatusMap;
const selectConnectionState = (state)=>state.sessions.connectionState;
/* harmony default export */ const __WEBPACK_DEFAULT_EXPORT__ = (sessionsSlice.reducer);


/***/ }),

/***/ 63939:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Ai: () => (/* binding */ removeItem),
/* harmony export */   Ay: () => (__WEBPACK_DEFAULT_EXPORT__),
/* harmony export */   Bi: () => (/* binding */ selectAllBacklogItems),
/* harmony export */   Ky: () => (/* binding */ selectBacklogItemsLiveVersionMap),
/* harmony export */   Se: () => (/* binding */ appendActivityNote),
/* harmony export */   vH: () => (/* binding */ upsertItem),
/* harmony export */   zc: () => (/* binding */ selectBacklogItemById)
/* harmony export */ });
/* unused harmony exports selectBacklogItemsMap, selectBacklogItemsTotal, makeSelectBacklogItemsByStatus */
/* harmony import */ var _reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(73559);
/* harmony import */ var _reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(13009);
/* harmony import */ var _bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(28765);


const initialState = {
    items: {},
    liveVersion: {}
};
/** Returns the epoch-ms value of a proto Timestamp, or 0 if unset (oldest possible). */ function timestampMs(ts) {
    return ts ? (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_0__/* .timestampDate */ .FP)(ts).getTime() : 0;
}
const backlogItemsSlice = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_1__/* .createSlice */ .Z0)({
    name: "backlogItems",
    initialState,
    reducers: {
        /**
     * Upserts a backlog item, guarded by a real `updatedAt`-based staleness
     * check (not sessionsSlice.upsertSession's equal-only check): an incoming
     * item strictly older than the currently-stored item for the same id is
     * dropped, so out-of-order event delivery (e.g. concurrent publishers,
     * stream replay) can never regress the store to older data.
     *
     * `isSnapshot` (default `true`) drives `liveVersion` bookkeeping only —
     * pass `false` exclusively for a genuine live `BacklogItemEvent` (i.e.
     * `event.isSnapshot === false`) so Epic 6.1's flash treatment can tell a
     * real-time change apart from a snapshot/replay/poll refresh.
     */ upsertItem: {
            reducer (state, action) {
                const { item: incoming, isSnapshot } = action.payload;
                const existing = state.items[incoming.id];
                if (existing && timestampMs(incoming.updatedAt) < timestampMs(existing.updatedAt)) {
                    return;
                }
                // Backstop for a partially-loaded event push (item_sessions/activity_notes/
                // allowed_transitions dropped by a publishItemChanged call site or a
                // non-eager-loaded snapshot query) racing a fully-loaded one: never let an
                // empty itemSessions, activityNotes, or allowedTransitions clobber data we
                // already have for this item. A genuine all-removed update would still
                // arrive with a newer updatedAt and populated (even if empty-by-intent)
                // data from a call site that *did* eager-load, so this only masks the
                // known-bad partial-load case, not real removals.
                //
                // activityNotes specifically: every BacklogChangeKind other than
                // ChangeActivityNoteAdded publishes via publishItemChanged ->
                // attachItemSessionsForPublish (session/ent_repository_backlog.go), which
                // re-populates only ItemSessions, never ActivityNotes — so their wire
                // events embed a full protoItem whose activity_notes is empty. Without
                // this guard, the very next status transition/verdict/session-attach would
                // wipe out any notes accumulated via the dedicated appendActivityNote
                // reducer below (ADR-002, Blocker 1 fix).
                //
                // allowedTransitions specifically: a pure function of status, so it's
                // never legitimately empty for a real item — an empty value is always a
                // sparse-DTO/resync artifact (e.g. a stale ListBacklogItems shape).
                //
                // planArtifactsPath specifically: unlike the three fields above, it CAN
                // legitimately go from set back to empty (TransitionBacklogItemStatus
                // resets it when an item moves back to idea/refining, e.g. re-triage) —
                // so this is gated on the item's status being unchanged. Every real
                // reset path changes status in the same write (server-side
                // backlog_service_lifecycle.go), so "same status, plan vanished" is
                // never a legitimate transition and is always a sparse-DTO artifact
                // (ListBacklogItems' backlogItemSummaryToProto omitted this field
                // entirely until the fix alongside this guard — getPrimaryCardAction/
                // itemActions.ts derives a ready item's card action directly from it,
                // so a sparse resync could flip "Approve Plan" to "Trigger Triage").
                let nextItem = incoming;
                if (existing) {
                    var _existing_itemSessions, _incoming_itemSessions, _existing_activityNotes, _incoming_activityNotes, _existing_allowedTransitions, _incoming_allowedTransitions, _existing_blocking, _incoming_blocking;
                    const patch = {};
                    var _existing_itemSessions_length, _incoming_itemSessions_length;
                    if (((_existing_itemSessions_length = (_existing_itemSessions = existing.itemSessions) === null || _existing_itemSessions === void 0 ? void 0 : _existing_itemSessions.length) !== null && _existing_itemSessions_length !== void 0 ? _existing_itemSessions_length : 0) > 0 && ((_incoming_itemSessions_length = (_incoming_itemSessions = incoming.itemSessions) === null || _incoming_itemSessions === void 0 ? void 0 : _incoming_itemSessions.length) !== null && _incoming_itemSessions_length !== void 0 ? _incoming_itemSessions_length : 0) === 0) {
                        patch.itemSessions = existing.itemSessions;
                    }
                    var _existing_activityNotes_length, _incoming_activityNotes_length;
                    if (((_existing_activityNotes_length = (_existing_activityNotes = existing.activityNotes) === null || _existing_activityNotes === void 0 ? void 0 : _existing_activityNotes.length) !== null && _existing_activityNotes_length !== void 0 ? _existing_activityNotes_length : 0) > 0 && ((_incoming_activityNotes_length = (_incoming_activityNotes = incoming.activityNotes) === null || _incoming_activityNotes === void 0 ? void 0 : _incoming_activityNotes.length) !== null && _incoming_activityNotes_length !== void 0 ? _incoming_activityNotes_length : 0) === 0) {
                        patch.activityNotes = existing.activityNotes;
                    }
                    var _existing_allowedTransitions_length, _incoming_allowedTransitions_length;
                    if (((_existing_allowedTransitions_length = (_existing_allowedTransitions = existing.allowedTransitions) === null || _existing_allowedTransitions === void 0 ? void 0 : _existing_allowedTransitions.length) !== null && _existing_allowedTransitions_length !== void 0 ? _existing_allowedTransitions_length : 0) > 0 && ((_incoming_allowedTransitions_length = (_incoming_allowedTransitions = incoming.allowedTransitions) === null || _incoming_allowedTransitions === void 0 ? void 0 : _incoming_allowedTransitions.length) !== null && _incoming_allowedTransitions_length !== void 0 ? _incoming_allowedTransitions_length : 0) === 0) {
                        patch.allowedTransitions = existing.allowedTransitions;
                    }
                    var _existing_blocking_length, _incoming_blocking_length;
                    // blocking is only populated by GetBacklogItem/Add/Remove; Watch
                    // payloads never carry it, so a live event must not erase it.
                    if (((_existing_blocking_length = (_existing_blocking = existing.blocking) === null || _existing_blocking === void 0 ? void 0 : _existing_blocking.length) !== null && _existing_blocking_length !== void 0 ? _existing_blocking_length : 0) > 0 && ((_incoming_blocking_length = (_incoming_blocking = incoming.blocking) === null || _incoming_blocking === void 0 ? void 0 : _incoming_blocking.length) !== null && _incoming_blocking_length !== void 0 ? _incoming_blocking_length : 0) === 0) {
                        patch.blocking = existing.blocking;
                    }
                    if (existing.status === incoming.status && existing.planArtifactsPath && !incoming.planArtifactsPath) {
                        patch.planArtifactsPath = existing.planArtifactsPath;
                    }
                    if (Object.keys(patch).length > 0) {
                        nextItem = {
                            ...incoming,
                            ...patch
                        };
                    }
                }
                state.items[incoming.id] = nextItem;
                if (!isSnapshot) {
                    var _state_liveVersion_incoming_id;
                    state.liveVersion[incoming.id] = ((_state_liveVersion_incoming_id = state.liveVersion[incoming.id]) !== null && _state_liveVersion_incoming_id !== void 0 ? _state_liveVersion_incoming_id : 0) + 1;
                }
            },
            prepare (item) {
                let isSnapshot = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : true;
                return {
                    payload: {
                        item,
                        isSnapshot
                    }
                };
            }
        },
        /** Deletes an item from the map entirely (BacklogItemRemovedEvent — permanent delete, not upsert). */ removeItem (state, action) {
            delete state.items[action.payload];
        },
        /**
     * Targeted single-note append for the dedicated activityNoteAdded live
     * event (ADR-002, backlog-item-activity-log) — appends `note` to
     * `state.items[itemId].activityNotes` without touching any other field,
     * never a call to upsertItem/a wholesale item replace. No-op if the item
     * isn't in the store yet (matches removeItem's "item not present"
     * tolerance) — the next GetBacklogItem/ListBacklogItems refresh will
     * include it.
     */ appendActivityNote (state, action) {
            const { itemId, note } = action.payload;
            const existing = state.items[itemId];
            if (!existing) return;
            var _existing_activityNotes;
            state.items[itemId] = {
                ...existing,
                activityNotes: [
                    ...(_existing_activityNotes = existing.activityNotes) !== null && _existing_activityNotes !== void 0 ? _existing_activityNotes : [],
                    note
                ]
            };
        }
    }
});
const { upsertItem, removeItem, appendActivityNote } = backlogItemsSlice.actions;
// Base selectors
const selectBacklogItemsMap = (state)=>state.backlogItems.items;
const selectBacklogItemById = (state, id)=>state.backlogItems.items[id];
/**
 * Raw `liveVersion` map — returned directly (like `selectBacklogItemsMap`)
 * rather than via `createSelector`: Immer already gives it the same
 * structural-sharing guarantee (unrelated keys keep their prior value
 * across an unrelated item's update), so no extra memoization is needed.
 */ const selectBacklogItemsLiveVersionMap = (state)=>state.backlogItems.liveVersion;
// Memoized selectors: list-shaped reads off backlogItemsSlice must not allocate
// a new array/object on every call, or every consumer (e.g. BacklogItemCard)
// re-renders whenever ANY item in the store changes, not just its own.
const selectAllBacklogItems = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_2__/* .createSelector */ .Mz)(selectBacklogItemsMap, (items)=>Object.values(items));
const selectBacklogItemsTotal = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_2__/* .createSelector */ .Mz)(selectBacklogItemsMap, (items)=>Object.keys(items).length);
/** Shallow array equality: same length and same element references, in order. */ function shallowArrayEqual(a, b) {
    if (a === b) return true;
    if (a.length !== b.length) return false;
    for(let i = 0; i < a.length; i++){
        if (a[i] !== b[i]) return false;
    }
    return true;
}
/**
 * Selector factory for a status-filtered view. Use one instance per
 * component (e.g. via `useMemo(makeSelectBacklogItemsByStatus, [])`) rather
 * than a single shared selector — a shared selector's single-entry cache
 * would thrash (and stop memoizing) if multiple components filter by
 * different statuses simultaneously.
 *
 * `resultEqualityCheck: shallowArrayEqual` is what actually delivers on the
 * "unrelated item update shouldn't re-render this list" guarantee: without
 * it, the filtered array is a brand-new reference every time ANY item in the
 * store changes (because `selectAllBacklogItems` itself must return a new
 * array whenever the underlying map changes), even if none of the matching
 * items actually differ. With it, the selector still recomputes, but returns
 * the previous array reference when the filtered contents are unchanged.
 */ const makeSelectBacklogItemsByStatus = ()=>createSelector([
        selectAllBacklogItems,
        (_state, status)=>status
    ], (items, status)=>items.filter((item)=>item.status === status), {
        memoizeOptions: {
            resultEqualityCheck: shallowArrayEqual
        }
    });
/* harmony default export */ const __WEBPACK_DEFAULT_EXPORT__ = (backlogItemsSlice.reducer);


/***/ }),

/***/ 64006:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 66485:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 68453:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Ai: () => (/* binding */ removeItem),
/* harmony export */   Ay: () => (__WEBPACK_DEFAULT_EXPORT__),
/* harmony export */   B5: () => (/* binding */ addItem),
/* harmony export */   K6: () => (/* binding */ selectReviewQueueLoading),
/* harmony export */   KS: () => (/* binding */ selectReviewQueue),
/* harmony export */   Nh: () => (/* binding */ setError),
/* harmony export */   _W: () => (/* binding */ selectReviewQueueItemsWithLiveStatus),
/* harmony export */   fk: () => (/* binding */ updateItem),
/* harmony export */   r1: () => (/* binding */ setLoading),
/* harmony export */   s6: () => (/* binding */ setReviewQueue),
/* harmony export */   wk: () => (/* binding */ selectReviewQueueLastUpdatedAt),
/* harmony export */   wq: () => (/* binding */ selectReviewQueueError)
/* harmony export */ });
/* unused harmony exports setReviewQueueStats, selectReviewQueueItems, selectReviewQueueStats, selectWaitingItems, selectQueueCountsByWorkingState */
/* harmony import */ var _reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(73559);
/* harmony import */ var _reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(13009);
/* harmony import */ var _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(81432);
/* harmony import */ var _lib_utils_deriveWorkingState__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(81801);



const initialState = {
    reviewQueue: null,
    loading: false,
    error: null,
    lastUpdatedAt: null,
    stats: {
        totalItems: 0,
        byPriority: {},
        byReason: {},
        averageAgeSeconds: "0",
        oldestItemId: "",
        oldestAgeSeconds: "0"
    }
};
const reviewQueueSlice = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_2__/* .createSlice */ .Z0)({
    name: "reviewQueue",
    initialState,
    reducers: {
        setReviewQueue (state, action) {
            state.reviewQueue = action.payload;
            if (action.payload) {
                state.stats.totalItems = action.payload.totalItems;
            }
            state.lastUpdatedAt = Date.now();
        },
        setReviewQueueStats (state, action) {
            state.stats = action.payload;
        },
        setLoading (state, action) {
            state.loading = action.payload;
        },
        setError (state, action) {
            state.error = action.payload;
        },
        removeItem (state, action) {
            if (!state.reviewQueue) return;
            const before = state.reviewQueue.items.length;
            state.reviewQueue.items = state.reviewQueue.items.filter((item)=>item.sessionId !== action.payload);
            const removed = before - state.reviewQueue.items.length;
            state.reviewQueue.totalItems = Math.max(0, state.reviewQueue.totalItems - removed);
            state.stats.totalItems = Math.max(0, state.stats.totalItems - removed);
        },
        addItem (state, action) {
            if (!state.reviewQueue) return;
            const exists = state.reviewQueue.items.some((i)=>i.sessionId === action.payload.sessionId);
            if (!exists) {
                state.reviewQueue.items.push(action.payload);
                state.reviewQueue.totalItems += 1;
                state.stats.totalItems += 1;
            }
        },
        updateItem (state, action) {
            if (!state.reviewQueue) return;
            const item = state.reviewQueue.items.find((i)=>i.sessionId === action.payload.sessionId);
            if (item) {
                Object.assign(item, action.payload.updates);
            }
        }
    }
});
const { setReviewQueue, setReviewQueueStats, setLoading, setError, removeItem, addItem, updateItem } = reviewQueueSlice.actions;
// Selectors
const selectReviewQueue = (state)=>state.reviewQueue.reviewQueue;
const selectReviewQueueItems = (state)=>{
    var _state_reviewQueue_reviewQueue;
    var _state_reviewQueue_reviewQueue_items;
    return (_state_reviewQueue_reviewQueue_items = (_state_reviewQueue_reviewQueue = state.reviewQueue.reviewQueue) === null || _state_reviewQueue_reviewQueue === void 0 ? void 0 : _state_reviewQueue_reviewQueue.items) !== null && _state_reviewQueue_reviewQueue_items !== void 0 ? _state_reviewQueue_reviewQueue_items : [];
};
const selectReviewQueueStats = (state)=>state.reviewQueue.stats;
const selectReviewQueueLoading = (state)=>state.reviewQueue.loading;
const selectReviewQueueError = (state)=>state.reviewQueue.error;
const selectReviewQueueLastUpdatedAt = (state)=>state.reviewQueue.lastUpdatedAt;
// selectReviewQueueItemsWithLiveStatus joins review queue items with live session state from
// sessionsSlice. It overrides each item's workingState using the live detectedStatus from
// the WatchSessions stream, so the review queue panel always reflects the current session
// state even when the WatchReviewQueue stream hasn't pushed a fresh snapshot yet.
const selectReviewQueueItemsWithLiveStatus = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_3__/* .createSelector */ .Mz)([
    (state)=>{
        var _state_reviewQueue_reviewQueue;
        var _state_reviewQueue_reviewQueue_items;
        return (_state_reviewQueue_reviewQueue_items = (_state_reviewQueue_reviewQueue = state.reviewQueue.reviewQueue) === null || _state_reviewQueue_reviewQueue === void 0 ? void 0 : _state_reviewQueue_reviewQueue.items) !== null && _state_reviewQueue_reviewQueue_items !== void 0 ? _state_reviewQueue_reviewQueue_items : [];
    },
    (state)=>state.sessions.detectedStatusMap
], (items, detectedStatusMap)=>items.map((item)=>{
        const liveStatus = detectedStatusMap[item.sessionId];
        if (!liveStatus) return item;
        const liveWorkingState = (0,_lib_utils_deriveWorkingState__WEBPACK_IMPORTED_MODULE_1__/* .deriveWorkingState */ .m)({
            subStatus: item.subStatus,
            detectedStatus: liveStatus.detectedStatus
        });
        return {
            ...item,
            workingState: liveWorkingState
        };
    }));
// selectWaitingItems returns only items that are NOT actively working,
// so the queue shows sessions that genuinely need user attention.
const selectWaitingItems = (state)=>{
    var _state_reviewQueue_reviewQueue;
    var _state_reviewQueue_reviewQueue_items;
    const items = (_state_reviewQueue_reviewQueue_items = (_state_reviewQueue_reviewQueue = state.reviewQueue.reviewQueue) === null || _state_reviewQueue_reviewQueue === void 0 ? void 0 : _state_reviewQueue_reviewQueue.items) !== null && _state_reviewQueue_reviewQueue_items !== void 0 ? _state_reviewQueue_reviewQueue_items : [];
    return items.filter((item)=>{
        const ws = deriveWorkingState(item);
        return ws !== WorkingState.ACTIVE && ws !== WorkingState.PROCESSING;
    });
};
// selectQueueCountsByWorkingState returns per-category counts for the header badge.
const selectQueueCountsByWorkingState = (state)=>{
    var _state_reviewQueue_reviewQueue;
    var _state_reviewQueue_reviewQueue_items;
    const items = (_state_reviewQueue_reviewQueue_items = (_state_reviewQueue_reviewQueue = state.reviewQueue.reviewQueue) === null || _state_reviewQueue_reviewQueue === void 0 ? void 0 : _state_reviewQueue_reviewQueue.items) !== null && _state_reviewQueue_reviewQueue_items !== void 0 ? _state_reviewQueue_reviewQueue_items : [];
    let waiting = 0;
    let working = 0;
    let stuck = 0;
    for (const item of items){
        const ws = deriveWorkingState(item);
        if (ws === WorkingState.ACTIVE || ws === WorkingState.PROCESSING) {
            working++;
        } else if (ws === WorkingState.WAITING) {
            stuck++;
        } else {
            waiting++;
        }
    }
    return {
        waiting,
        working,
        stuck
    };
};
/* harmony default export */ const __WEBPACK_DEFAULT_EXPORT__ = (reviewQueueSlice.reducer);


/***/ }),

/***/ 81801:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   m: () => (/* binding */ deriveWorkingState)
/* harmony export */ });
/* harmony import */ var _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(81432);
/* harmony import */ var _lib_utils_assertNever__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(92418);


/**
 * Derives the effective WorkingState for a session from its SubStatus and DetectedStatus.
 *
 * This is the frontend equivalent of the removed server-side MapDetectedStatusToWorkingState.
 * SubStatus is the primary signal; DetectedStatus is a fallback for cases where SubStatus
 * is UNSPECIFIED (e.g. non-Active sessions or legacy items).
 *
 * Mapping:
 *   SubStatus.PROCESSING, NEEDS_APPROVAL, INPUT_REQUIRED, ERROR, TESTS_FAILING, RATE_LIMITED, WAITING_FOR_AGENT, COMPACTING
 *     → WorkingState.PROCESSING  (actively doing something or blocked on user)
 *   SubStatus.IDLE, READY
 *     → WorkingState.IDLE        (at prompt, ready for next instruction)
 *   SubStatus.SUCCESS
 *     → WorkingState.IDLE        (task finished, prompt available)
 *   SubStatus.UNSPECIFIED → fall through to detectedStatus
 *     DetectedStatus.EXECUTING, WAITING_FOR_AGENT, COMPACTING → WorkingState.ACTIVE
 *     DetectedStatus.PROCESSING                                → WorkingState.PROCESSING
 *     default                                                  → WorkingState.UNSPECIFIED
 */ function deriveWorkingState(session) {
    switch(session.subStatus){
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.PROCESSING:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.NEEDS_APPROVAL:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.INPUT_REQUIRED:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.ERROR:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.TESTS_FAILING:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.RATE_LIMITED:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.WAITING_FOR_AGENT:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.COMPACTING:
            return _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .WorkingState */ .RA.PROCESSING;
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.IDLE:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.READY:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.SUCCESS:
            return _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .WorkingState */ .RA.IDLE;
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.UNSPECIFIED:
            break;
    }
    // detectedStatus-based fallback (used when subStatus is UNSPECIFIED)
    switch(session.detectedStatus){
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.EXECUTING:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.WAITING_FOR_AGENT:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.COMPACTING:
            return _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .WorkingState */ .RA.ACTIVE;
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.PROCESSING:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.NEEDS_APPROVAL:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.INPUT_REQUIRED:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.ERROR:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.TESTS_FAILING:
            return _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .WorkingState */ .RA.PROCESSING;
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.IDLE:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.READY:
            return _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .WorkingState */ .RA.IDLE;
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.SUCCESS:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.UNKNOWN:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.UNSPECIFIED:
        case undefined:
            return _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .WorkingState */ .RA.UNSPECIFIED;
        default:
            return (0,_lib_utils_assertNever__WEBPACK_IMPORTED_MODULE_1__/* .assertNever */ .x)(session.detectedStatus);
    }
}


/***/ }),

/***/ 81921:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Ay: () => (__WEBPACK_DEFAULT_EXPORT__),
/* harmony export */   lj: () => (/* binding */ selectRemoteHealthEntry),
/* harmony export */   ng: () => (/* binding */ remoteHealthChanged)
/* harmony export */ });
/* unused harmony export selectRemoteConnectionState */
/* harmony import */ var _reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(73559);
/* harmony import */ var _gen_session_v1_remote_pb__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(88149);


const initialState = {
    byName: {}
};
const remotesSlice = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_1__/* .createSlice */ .Z0)({
    name: "remotes",
    initialState,
    reducers: {
        remoteHealthChanged (state, action) {
            const { remoteName, state: connState, previousState } = action.payload;
            if (!remoteName) return;
            state.byName[remoteName] = {
                state: connState,
                previousState,
                updatedAt: Date.now()
            };
        }
    }
});
const { remoteHealthChanged } = remotesSlice.actions;
// selectRemoteConnectionState returns a selector for a single remote's
// current connection state, defaulting to UNSPECIFIED (never RemoteConnectionState's
// CONNECTED) when no health-change event has been observed yet -- consumers
// (RemoteConnectionIndicator) must treat UNSPECIFIED as "unknown," not as a
// synonym for disconnected or connected.
const selectRemoteConnectionState = (remoteName)=>(state)=>{
        var _state_remotes_byName_remoteName;
        var _state_remotes_byName_remoteName_state;
        return (_state_remotes_byName_remoteName_state = (_state_remotes_byName_remoteName = state.remotes.byName[remoteName]) === null || _state_remotes_byName_remoteName === void 0 ? void 0 : _state_remotes_byName_remoteName.state) !== null && _state_remotes_byName_remoteName_state !== void 0 ? _state_remotes_byName_remoteName_state : RemoteConnectionState.UNSPECIFIED;
    };
// selectRemoteHealthEntry exposes the full stored entry (state, previousState,
// updatedAt). RemoteConnectionIndicator currently only reads .state from this
// (it tracks its own prior-state via a local ref to decide polite vs. assertive
// announcements, not this entry's previousState) -- exposed as the full entry
// rather than just .state for any future consumer that needs updatedAt/previousState
// directly from the store instead of re-deriving it.
const selectRemoteHealthEntry = (remoteName)=>(state)=>state.remotes.byName[remoteName];
/* harmony default export */ const __WEBPACK_DEFAULT_EXPORT__ = (remotesSlice.reducer);


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

/***/ 92418:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   x: () => (/* binding */ assertNever)
/* harmony export */ });
function assertNever(x) {
    throw new Error("Unhandled case: ".concat(String(x)));
}


/***/ }),

/***/ 93602:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   GV: () => (/* reexport safe */ _store__WEBPACK_IMPORTED_MODULE_0__.GV),
/* harmony export */   M_: () => (/* reexport safe */ _store__WEBPACK_IMPORTED_MODULE_0__.M_),
/* harmony export */   jL: () => (/* reexport safe */ _store__WEBPACK_IMPORTED_MODULE_0__.jL)
/* harmony export */ });
/* harmony import */ var _store__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(25337);



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
/******/ __webpack_require__.O(0, [1869,852,4202,9353,8182,7924,2265,8165,3575,2275,4298,12,9691,3516,7358], () => (__webpack_exec__(35369)));
/******/ var __webpack_exports__ = __webpack_require__.O();
/******/ _N_E = __webpack_exports__;
/******/ }
]);
//# sourceMappingURL=page-d29a4ea924bf3b27.js.map