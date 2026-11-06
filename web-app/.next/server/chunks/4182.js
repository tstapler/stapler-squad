exports.id = 4182;
exports.ids = [4182];
exports.modules = {

/***/ 50779:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   b: () => (/* binding */ useGenerateRule)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(88782);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(4435);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(31381);
/* harmony import */ var _bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(7094);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(73396);
/* __next_internal_client_entry_do_not_use__ useGenerateRule auto */ 




/**
 * React hook encapsulating all state for the GenerateSuggestedRule RPC.
 *
 * Used by all four AI rule-generation surfaces (rules panel, review queue,
 * analytics gaps, command-sample form). Callers may also call clear() or
 * cancel() directly.
 */ function useGenerateRule() {
    const [suggestions, setSuggestions] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)([]);
    const [loading, setLoading] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(false);
    const [error, setError] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(null);
    const abortRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    const clientRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    // Abort any in-flight request on unmount to prevent state updates on unmounted components.
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        return ()=>{
            abortRef.current?.abort();
        };
    }, []);
    // Lazily initialise the client (mirrors useApprovalRules pattern).
    const getClient = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>{
        if (!clientRef.current) {
            clientRef.current = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_2__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_3__/* .getConnectTransport */ .qW)());
        }
        return clientRef.current;
    }, []);
    // Track whether the most recent abort was a user-initiated cancel (vs. timeout).
    const userCancelledRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(false);
    const generate = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (req)=>{
        // Cancel any previous in-flight request.
        userCancelledRef.current = false;
        abortRef.current?.abort();
        abortRef.current = new AbortController();
        // Reset previous state at the start of each call.
        setSuggestions([]);
        setError(null);
        setLoading(true);
        try {
            const client = getClient();
            const request = (0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .GenerateSuggestedRuleRequestSchema */ .pZy, req);
            const resp = await client.generateSuggestedRule(request, {
                signal: abortRef.current.signal,
                timeoutMs: 60000
            });
            setSuggestions(resp.suggestions ?? []);
        } catch (err) {
            const e = err instanceof Error ? err : new Error("GenerateSuggestedRule failed");
            if (e.name === "AbortError") {
                if (userCancelledRef.current) {
                // Manual cancel via cancel() — silently swallow. "Rule generation was cancelled."
                } else {
                    // Timeout-triggered abort — surface a friendly message.
                    setError(new Error("Rule generation timed out. Please try again."));
                }
            } else {
                setError(e);
            }
        } finally{
            setLoading(false);
        }
    }, [
        getClient
    ]);
    const cancel = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>{
        userCancelledRef.current = true;
        abortRef.current?.abort();
        setLoading(false);
    }, []);
    const clear = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>{
        setSuggestions([]);
        setError(null);
    }, []);
    return {
        suggestions,
        loading,
        error,
        generate,
        cancel,
        clear
    };
}


/***/ }),

/***/ 53532:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "._7df0zq0 {\n  display: inline-flex;\n  align-items: center;\n  gap: var(--_19ez2rz2y);\n  padding: 3px 10px;\n  border-radius: var(--_19ez2rz38);\n  font-size: var(--_19ez2rz3a);\n  font-weight: 600;\n  white-space: nowrap;\n}\n._7df0zq1 {\n  background: var(--_19ez2rz15);\n  color: var(--_19ez2rz16);\n  border: 1px solid var(--_19ez2rz14);\n}\n._7df0zq2 {\n  background: var(--_19ez2rz11);\n  color: var(--_19ez2rz12);\n}\n._7df0zq3 {\n  background: var(--_19ez2rzy);\n  color: var(--_19ez2rzz);\n}\n._7df0zq4 {\n  background: var(--_19ez2rzv);\n  color: var(--_19ez2rzw);\n}\n._7df0zq5 {\n  background: var(--_19ez2rzd);\n  color: var(--_19ez2rz2);\n}\n._7df0zq6 {\n  padding: 2px 6px;\n}\n._7df0zq8 {\n  font-size: var(--_19ez2rz3b);\n  line-height: 1;\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 55894:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  S: () => (/* binding */ SuggestedRuleCard)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
// EXTERNAL MODULE: ./src/lib/hooks/useApprovalRules.ts
var useApprovalRules = __webpack_require__(79254);
// EXTERNAL MODULE: ./src/gen/session/v1/types_pb.ts
var types_pb = __webpack_require__(29238);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FSuggestedRuleCard.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-1XzW7jIBC-9ym4rJRIJbLdpk3Zy75JRczYIcHgYpw4WfXdV4akNY7_1EZ76smSZ_iY-eaXxWsY6v3bcRugv3cIrWm8S7UqJSNoT_UM49fwBU6RPj3Pf9dypRlogsK8QoUSnLW0oKGFNWW8LNpADyurk1PGuEwJioK8qn8wXuSCHglKBNgf9RczriE2XEmCYiXKTNaSlOYEhY_1ufe7xcWD0HrwAcOl4BLwBY0KnkrMDWQFQTFIA9ozwzcyOs7bZgcTXHuxOomSBhf8BFdy-ik_AE835kpjYzUOG24AFzmNgSCpDprmnqvRSLD2FiVWQum2qJx7SA8jSMd-pMpHehxBCsN-qDDysZYWq5_H9QCUFdnQb84Uh4ulh_70PfQG0rNFiktd1Kq54hPSKhhLgbR1B9moPWh3U5dNga-_-vQuoRkXxysjyklZ2h_J9c1awfN8iKyTlWZUp1x2CFHguKzZSYQ64IogWhrVG7zgurZyDdhVF0IHpRlea6A7guwHUyE8al8stf3m9rSMkeIaI7KaTGSn16eRaK_99KE39vHWyTJQ9JN9XPuTItWc2bGiOcMGslxQA9iNm4IgDTlQM6tTCydciHuUcZnRahbVo-sehYmezz_HUtQaS7F_2cTp1h5ubFLPGmsr07oajHVyOimmu283gKshHE1J5uHq58xsCAqD4JezrqqB7OVnS9fKZx5IouKysJyo0tSTpZ7JEhredV6Y-awmP6w22bgRq-kPq6juRx_LztN5ndZwvhS04TH1B1l6I_Y33b3NNcKzIduyMDw54lhJA9I4LQySNe4yKh-bDWfenWrnDHKLQpeCDUXDbD6SNJmXMy1evrjKtPWnjKwpT4XOML050fVeajSVBXcTR-U05uaIgkW4LBDQAnyS3N5JpDIzwnhB1wLY3KWMO0lQsFgtW4cumm3FZdMiqUy9XKkDMO_4diQwq_-2eX4tXAPLyUgwqBB9gdg2HgADjW413E4agLvuqu15Idti7iw4W8j19kzQ1QtV2Dsuu3swTB77Tq475_5kwDhFRawBJKKSoVlGK3xuoU-PQV653G0tgb1bX5hY79_v3v8B_2UVuiMRAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FSuggestedRuleCard_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_1XzW7jIBC_9ym4rJRIJbLdpk3Zy75JRczYIcHgYpw4WfXdV4akNY7_1EZ76smSZ_iY_eaXxWsY6v3bcRugv3cIrWm8S7UqJSNoT_UM49fwBU6RPj3Pf9dypRlogsK8QoUSnLW0oKGFNWW8LNpADyurk1PGuEwJioK8qn8wXuSCHglKBNgf9RczriE2XEmCYiXKTNaSlOYEhY_1ufe7xcWD0HrwAcOl4BLwBY0KnkrMDWQFQTFIA9ozwzcyOs7bZgcTXHuxOomSBhf8BFdy_ik_AE835kpjYzUOG24AFzmNgSCpDprmnqvRSLD2FiVWQum2qJx7SA8jSMd_pMpHehxBCsN_qDDysZYWq5_H9QCUFdnQb84Uh4ulh_70PfQG0rNFiktd1Kq54hPSKhhLgbR1B9moPWh3U5dNga_vQuoRkXxysjyklZ2h_J9c1awfN8iKyTlWZUp1x2CFHguKzZSYQ64IogWhrVG7zgurZyDdhVF0IHpRlea6A7guwHUyE8al8stf3m9rSMkeIaI7KaTGSn16eRaK_99KE39vHWyTJQ9JN9XPuTItWc2bGiOcMGslxQA9iNm4IgDTlQM6tTCydciHuUcZnRahbVo_sehYmezz_HUtQaS7F_2cTp1h5ubFLPGmsr07oajHVyOimmu283gKshHE1J5uHq58xsCAqD4JezrqqB7OVnS9fKZx5IouKysJyo0tSTpZ7JEhredV6Y_awmP6w22bgRq_kPq6juRx_LztN5ndZwvhS04TH1B1l6I_Y33b3NNcKzIduyMDw54lhJA9I4LQySNe4yKh_bDWfenWrnDHKLQpeCDUXDbD6SNJmXMy1evrjKtPWnjKwpT4XOML050fVeajSVBXcTR_U05uaIgkW4LBDQAnyS3N5JpDIzwnhB1wLY3KWMO0lQsFgtW4cumm3FZdMiqUy9XKkDMO_4diQwq_2eX4tXAPLyUgwqBB9gdg2HgADjW413E4agLvuqu15Idti7iw4W8j19kzQ1QtV2Dsuu3swTB77Tq475_5kwDhFRawBJKKSoVlGK3xuoU_PQV653G0tgb1bX5hY79_v3v8B_2UVuiMRAAA_22_7D = __webpack_require__(75615);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+recipes@0.5.7+6974e6bd53375d7a/node_modules/@vanilla-extract/recipes/dist/createRuntimeFn-62c9670f.esm.js
var createRuntimeFn_62c9670f_esm = __webpack_require__(9196);
;// ./src/components/sessions/SuggestedRuleCard.css.ts



var acceptButton = '_11rvqyji';
var actions = '_11rvqyjh';
var card = '_11rvqyj0';
var cardHeader = '_11rvqyjk';
var cardTitle = '_11rvqyjl';
var confidenceBadge = (0,createRuntimeFn_62c9670f_esm.c)({defaultClassName:'_11rvqyj1',variantClassNames:{level:{high:'_11rvqyj2',medium:'_11rvqyj3',low:'_11rvqyj4'}},defaultVariants:{level:'medium'},compoundVariants:[]});
var conflictBanner = '_11rvqyj9';
var discardButton = '_11rvqyjj';
var explanationBlock = '_11rvqyj5';
var fieldGrid = '_11rvqyjb';
var fieldInput = '_11rvqyje';
var fieldLabel = '_11rvqyjd';
var fieldRow = '_11rvqyjc';
var fieldSelect = '_11rvqyjf';
var fieldTextarea = '_11rvqyjg';
var shadowBanner = '_11rvqyja';
var sourceCommandsDetails = '_11rvqyj6';
var sourceCommandsPre = '_11rvqyj8';
var sourceCommandsSummary = '_11rvqyj7';
;// ./src/components/sessions/SuggestedRuleCard.tsx
/* __next_internal_client_entry_do_not_use__ SuggestedRuleCard auto */ 




function stateFromSuggestion(s) {
    return {
        name: s.name,
        toolName: s.toolName,
        toolPattern: s.toolPattern,
        commandPattern: s.commandPattern,
        filePattern: s.filePattern,
        decision: s.decision !== types_pb/* AutoDecision */.Uz.UNSPECIFIED ? s.decision : types_pb/* AutoDecision */.Uz.ALLOW,
        reason: s.reason,
        alternative: s.alternative,
        priority: s.priority > 0 ? s.priority : 100
    };
}
function confidenceLevel(c) {
    if (c >= 0.8) return "high";
    if (c >= 0.5) return "medium";
    return "low";
}
const CONFIDENCE_LABELS = {
    high: "High",
    medium: "Medium",
    low: "Low"
};
// ── Component ─────────────────────────────────────────────────────────────────
/**
 * SuggestedRuleCard renders a single AI-proposed rule with all fields editable.
 *
 * The user can review AI metadata (confidence, explanation, source commands,
 * conflict warnings), edit any field, then "Accept & Save" (calls upsertRule)
 * or "Discard" (calls onDiscard).
 */ function SuggestedRuleCard({ suggestion, onAccept, onDiscard, loading = false }) {
    const { upsertRule, rules } = (0,useApprovalRules/* useApprovalRules */.I)();
    // Keep a ref to the latest rules so handleAccept never uses a stale closure snapshot.
    const rulesRef = (0,react.useRef)(rules);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        rulesRef.current = rules;
    }, [
        rules
    ]);
    const [form, setForm] = (0,react.useState)(()=>stateFromSuggestion(suggestion));
    const [saving, setSaving] = (0,react.useState)(false);
    const [saveError, setSaveError] = (0,react.useState)(null);
    const setField = (0,react.useCallback)((key, value)=>{
        setForm((prev)=>({
                ...prev,
                [key]: value
            }));
    }, []);
    const handleAccept = (0,react.useCallback)(async ()=>{
        if (!form.name.trim()) return;
        setSaving(true);
        setSaveError(null);
        try {
            const id = `user-${Date.now()}`;
            await upsertRule({
                id,
                name: form.name,
                toolName: form.toolName,
                toolPattern: form.toolPattern,
                commandPattern: form.commandPattern,
                filePattern: form.filePattern,
                decision: form.decision,
                riskLevel: suggestion.riskLevel,
                reason: form.reason,
                alternative: form.alternative,
                priority: form.priority,
                enabled: true,
                source: "user"
            });
            // Find the just-upserted rule in the latest rules list (via ref to avoid stale closure).
            const saved = rulesRef.current.find((r)=>r.name === form.name && r.source === "user");
            if (saved) {
                onAccept(saved);
            } else {
                // Fallback: construct a minimal ApprovalRuleProto-shaped object.
                onAccept({
                    id,
                    name: form.name,
                    toolName: form.toolName,
                    toolPattern: form.toolPattern,
                    commandPattern: form.commandPattern,
                    filePattern: form.filePattern,
                    decision: form.decision,
                    riskLevel: suggestion.riskLevel,
                    reason: form.reason,
                    alternative: form.alternative,
                    priority: form.priority,
                    enabled: true,
                    source: "user"
                });
            }
        } catch (e) {
            setSaveError(e instanceof Error ? e.message : "Failed to save rule.");
        } finally{
            setSaving(false);
        }
    }, [
        form,
        suggestion.riskLevel,
        upsertRule,
        onAccept
    ]);
    const level = confidenceLevel(suggestion.confidence);
    const confidencePct = Math.round(suggestion.confidence * 100);
    const sourceCommandsToShow = suggestion.sourceCommands.slice(0, 5);
    if (loading) {
        return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
            className: card,
            "aria-busy": "true",
            children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                style: {
                    color: "inherit",
                    opacity: 0.5
                },
                children: "Generating suggestion…"
            })
        });
    }
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: card,
        "data-testid": "suggested-rule-card",
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: cardHeader,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("h3", {
                        className: cardTitle,
                        children: "AI Rule Suggestion"
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                        className: confidenceBadge({
                            level
                        }),
                        "data-testid": "confidence-badge",
                        "data-level": level,
                        "aria-label": `Confidence: ${CONFIDENCE_LABELS[level]} (${confidencePct}%)`,
                        children: [
                            CONFIDENCE_LABELS[level],
                            " — ",
                            confidencePct,
                            "% confidence"
                        ]
                    })
                ]
            }),
            suggestion.explanation && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                className: explanationBlock,
                children: suggestion.explanation
            }),
            sourceCommandsToShow.length > 0 && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("details", {
                className: sourceCommandsDetails,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("summary", {
                        className: sourceCommandsSummary,
                        children: [
                            "Source commands (",
                            sourceCommandsToShow.length,
                            ")"
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("pre", {
                        className: sourceCommandsPre,
                        children: sourceCommandsToShow.join("\n")
                    })
                ]
            }),
            suggestion.shadowedByRuleIds.length > 0 && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: conflictBanner,
                role: "alert",
                "data-testid": "conflict-banner",
                children: [
                    "May be shadowed by higher-priority rule(s):",
                    " ",
                    suggestion.shadowedByRuleIds.join(", ")
                ]
            }),
            suggestion.shadowsRuleIds.length > 0 && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: shadowBanner,
                role: "status",
                "data-testid": "shadow-banner",
                children: [
                    "May suppress lower-priority rule(s): ",
                    suggestion.shadowsRuleIds.join(", ")
                ]
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: fieldGrid,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("label", {
                        className: fieldRow,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: fieldLabel,
                                children: "Name *"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                                className: fieldInput,
                                type: "text",
                                value: form.name,
                                onChange: (e)=>setField("name", e.target.value),
                                required: true,
                                "aria-label": "Name",
                                "data-testid": "field-name"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("label", {
                        className: fieldRow,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: fieldLabel,
                                children: "Tool Name"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                                className: fieldInput,
                                type: "text",
                                value: form.toolName,
                                onChange: (e)=>setField("toolName", e.target.value),
                                "aria-label": "Tool Name",
                                "data-testid": "field-tool-name"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("label", {
                        className: fieldRow,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: fieldLabel,
                                children: "Tool Pattern"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                                className: fieldInput,
                                type: "text",
                                value: form.toolPattern,
                                onChange: (e)=>setField("toolPattern", e.target.value),
                                "aria-label": "Tool Pattern",
                                "data-testid": "field-tool-pattern"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("label", {
                        className: fieldRow,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: fieldLabel,
                                children: "Command Pattern"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                                className: fieldInput,
                                type: "text",
                                value: form.commandPattern,
                                onChange: (e)=>setField("commandPattern", e.target.value),
                                "aria-label": "Command Pattern",
                                "data-testid": "field-command-pattern"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("label", {
                        className: fieldRow,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: fieldLabel,
                                children: "File Pattern"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                                className: fieldInput,
                                type: "text",
                                value: form.filePattern,
                                onChange: (e)=>setField("filePattern", e.target.value),
                                "aria-label": "File Pattern",
                                "data-testid": "field-file-pattern"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("label", {
                        className: fieldRow,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: fieldLabel,
                                children: "Decision"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("select", {
                                className: fieldSelect,
                                value: form.decision,
                                onChange: (e)=>setField("decision", Number(e.target.value)),
                                "aria-label": "Decision",
                                "data-testid": "field-decision",
                                children: [
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("option", {
                                        value: types_pb/* AutoDecision */.Uz.ALLOW,
                                        children: "Auto-Allow"
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("option", {
                                        value: types_pb/* AutoDecision */.Uz.DENY,
                                        children: "Auto-Deny"
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("option", {
                                        value: types_pb/* AutoDecision */.Uz.ESCALATE,
                                        children: "Escalate"
                                    })
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("label", {
                        className: fieldRow,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: fieldLabel,
                                children: "Priority"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                                className: fieldInput,
                                type: "number",
                                min: 1,
                                max: 999,
                                value: form.priority,
                                onChange: (e)=>setField("priority", Math.max(1, Math.min(999, Number(e.target.value)))),
                                "aria-label": "Priority",
                                "data-testid": "field-priority"
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("label", {
                className: fieldRow,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                        className: fieldLabel,
                        children: "Reason"
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("textarea", {
                        className: fieldTextarea,
                        value: form.reason,
                        onChange: (e)=>setField("reason", e.target.value),
                        "aria-label": "Reason",
                        "data-testid": "field-reason"
                    })
                ]
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("label", {
                className: fieldRow,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                        className: fieldLabel,
                        children: "Alternative"
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("textarea", {
                        className: fieldTextarea,
                        value: form.alternative,
                        onChange: (e)=>setField("alternative", e.target.value),
                        "aria-label": "Alternative",
                        "data-testid": "field-alternative"
                    })
                ]
            }),
            saveError && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                role: "alert",
                style: {
                    color: "var(--error-text, red)",
                    fontSize: 13,
                    margin: 0
                },
                children: saveError
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: actions,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                        className: discardButton,
                        type: "button",
                        onClick: onDiscard,
                        disabled: saving,
                        "data-testid": "discard-rule",
                        children: "Discard"
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                        className: acceptButton,
                        type: "button",
                        onClick: handleAccept,
                        disabled: saving || !form.name.trim(),
                        "data-testid": "accept-rule",
                        children: saving ? "Saving…" : "Accept & Save"
                    })
                ]
            })
        ]
    });
}


/***/ }),

/***/ 62722:
/***/ (() => {



/***/ }),

/***/ 75615:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "._11rvqyj0 {\n  background: var(--_19ez2rz7);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: var(--_19ez2rz38);\n  padding: 20px;\n  display: flex;\n  flex-direction: column;\n  gap: 14px;\n}\n._11rvqyj1 {\n  display: inline-flex;\n  align-items: center;\n  padding: var(--_19ez2rz2y) var(--_19ez2rz30);\n  border-radius: var(--_19ez2rz39);\n  font-size: var(--_19ez2rz3a);\n  font-weight: var(--_19ez2rz3h);\n  white-space: nowrap;\n}\n._11rvqyj2 {\n  background: var(--_19ez2rzv);\n  color: var(--_19ez2rzu);\n}\n._11rvqyj3 {\n  background: var(--_19ez2rzy);\n  color: var(--_19ez2rzx);\n}\n._11rvqyj4 {\n  background: var(--_19ez2rz11);\n  color: var(--_19ez2rz12);\n}\n._11rvqyj5 {\n  font-size: var(--_19ez2rz3b);\n  color: var(--_19ez2rz1);\n  line-height: 1.5;\n}\n._11rvqyj6 {\n  font-size: var(--_19ez2rz3b);\n  color: var(--_19ez2rz1);\n}\n._11rvqyj7 {\n  cursor: pointer;\n  padding: var(--_19ez2rz2y) 0;\n  font-weight: var(--_19ez2rz3g);\n}\n._11rvqyj7:hover {\n  color: var(--_19ez2rz0);\n}\n._11rvqyj8 {\n  font-family: var(--_19ez2rz2u);\n  font-size: var(--_19ez2rz3a);\n  background: var(--_19ez2rzb);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: var(--_19ez2rz37);\n  padding: var(--_19ez2rz2z);\n  margin: var(--_19ez2rz2z) 0 0;\n  overflow-x: auto;\n  color: var(--_19ez2rz0);\n  white-space: pre-wrap;\n  word-break: break-all;\n}\n._11rvqyj9 {\n  padding: var(--_19ez2rz2z) var(--_19ez2rz30);\n  background: var(--_19ez2rzy);\n  border: 1px solid var(--_19ez2rzx);\n  border-radius: var(--_19ez2rz37);\n  color: var(--_19ez2rzz);\n  font-size: var(--_19ez2rz3b);\n}\n._11rvqyja {\n  padding: var(--_19ez2rz2z) var(--_19ez2rz30);\n  background: var(--_19ez2rzb);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: var(--_19ez2rz37);\n  color: var(--_19ez2rz1);\n  font-size: var(--_19ez2rz3b);\n}\n._11rvqyjb {\n  display: grid;\n  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));\n  grid-gap: 12px;\n  gap: 12px;\n}\n._11rvqyjc {\n  display: flex;\n  flex-direction: column;\n  gap: 4px;\n}\n._11rvqyjd {\n  font-size: var(--_19ez2rz3b);\n  font-weight: var(--_19ez2rz3g);\n  color: var(--_19ez2rz1);\n}\n._11rvqyje {\n  background: var(--_19ez2rz1a);\n  border: 1px solid var(--_19ez2rzk);\n  border-radius: var(--_19ez2rz37);\n  padding: var(--_19ez2rz2y) var(--_19ez2rz2z);\n  font-size: var(--_19ez2rz3b);\n  color: var(--_19ez2rz0);\n  width: 100%;\n  box-sizing: border-box;\n}\n._11rvqyje:focus {\n  outline: none;\n  border-color: var(--_19ez2rzm);\n}\n._11rvqyjf {\n  background: var(--_19ez2rz1a);\n  border: 1px solid var(--_19ez2rzk);\n  border-radius: var(--_19ez2rz37);\n  padding: var(--_19ez2rz2y) var(--_19ez2rz2z);\n  font-size: var(--_19ez2rz3b);\n  color: var(--_19ez2rz0);\n  width: 100%;\n  box-sizing: border-box;\n}\n._11rvqyjf:focus {\n  outline: none;\n  border-color: var(--_19ez2rzm);\n}\n._11rvqyjg {\n  background: var(--_19ez2rz1a);\n  border: 1px solid var(--_19ez2rzk);\n  border-radius: var(--_19ez2rz37);\n  padding: var(--_19ez2rz2y) var(--_19ez2rz2z);\n  font-size: var(--_19ez2rz3b);\n  color: var(--_19ez2rz0);\n  width: 100%;\n  box-sizing: border-box;\n  min-height: 60px;\n  resize: vertical;\n}\n._11rvqyjg:focus {\n  outline: none;\n  border-color: var(--_19ez2rzm);\n}\n._11rvqyjh {\n  display: flex;\n  gap: 10px;\n  justify-content: flex-end;\n  border-top: 1px solid var(--_19ez2rze);\n  padding-top: var(--_19ez2rz30);\n  margin-top: var(--_19ez2rz2z);\n}\n._11rvqyji {\n  background: var(--_19ez2rzm);\n  border: none;\n  border-radius: var(--_19ez2rz37);\n  padding: var(--_19ez2rz2z) var(--_19ez2rz31);\n  font-size: var(--_19ez2rz3b);\n  font-weight: var(--_19ez2rz3h);\n  color: var(--_19ez2rzq);\n  cursor: pointer;\n  transition: opacity 0.15s ease;\n}\n._11rvqyji:hover:not(:disabled) {\n  opacity: 0.85;\n}\n._11rvqyji:disabled {\n  opacity: 0.5;\n  cursor: not-allowed;\n}\n._11rvqyjj {\n  background: var(--_19ez2rz8);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: var(--_19ez2rz37);\n  padding: var(--_19ez2rz2z) var(--_19ez2rz31);\n  font-size: var(--_19ez2rz3b);\n  color: var(--_19ez2rz1);\n  cursor: pointer;\n  transition: all 0.15s ease;\n}\n._11rvqyjj:hover {\n  background: var(--_19ez2rz18);\n  color: var(--_19ez2rz0);\n}\n._11rvqyjk {\n  display: flex;\n  align-items: center;\n  gap: var(--_19ez2rz30);\n  flex-wrap: wrap;\n}\n._11rvqyjl {\n  margin: 0;\n  font-size: var(--_19ez2rz3d);\n  font-weight: var(--_19ez2rz3h);\n  color: var(--_19ez2rz0);\n}\n@media screen and (max-width: 640px) {\n  ._11rvqyjb {\n    grid-template-columns: 1fr;\n  }\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 79254:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   I: () => (/* binding */ useApprovalRules)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(88782);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(4435);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(31381);
/* harmony import */ var _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(29238);
/* harmony import */ var _bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(7094);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(73396);
/* __next_internal_client_entry_do_not_use__ useApprovalRules auto */ 






/**
 * Builds an `ApprovalRuleProto` from partial rule data, filling in defaults
 * for any unset fields. Shared by `upsertRule` and `bulkUpsertRules` so both
 * construct rules identically.
 */ function buildApprovalRuleProto(ruleData) {
    return (0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_3__/* .create */ .v)(_gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_2__/* .ApprovalRuleProtoSchema */ .nb, {
        id: ruleData.id,
        name: ruleData.name ?? "",
        toolName: ruleData.toolName ?? "",
        toolPattern: ruleData.toolPattern ?? "",
        toolCategory: ruleData.toolCategory ?? "",
        commandPattern: ruleData.commandPattern ?? "",
        filePattern: ruleData.filePattern ?? "",
        programs: ruleData.programs ?? [],
        subcommands: ruleData.subcommands ?? [],
        blockedSubcommands: ruleData.blockedSubcommands ?? [],
        requiredFlags: ruleData.requiredFlags ?? [],
        forbiddenFlags: ruleData.forbiddenFlags ?? [],
        requiredFlagPrefixes: ruleData.requiredFlagPrefixes ?? [],
        pythonModes: ruleData.pythonModes ?? [],
        safePythonImportsOnly: ruleData.safePythonImportsOnly ?? false,
        requireCiPassing: ruleData.requireCiPassing ?? false,
        minSessionIdleMinutes: ruleData.minSessionIdleMinutes ?? 0,
        decision: ruleData.decision ?? _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_2__/* .AutoDecision */ .Uz.ESCALATE,
        riskLevel: ruleData.riskLevel ?? "",
        reason: ruleData.reason ?? "",
        alternative: ruleData.alternative ?? "",
        priority: ruleData.priority ?? 10,
        enabled: ruleData.enabled ?? true,
        source: "user"
    });
}
/**
 * React hook for managing auto-approval rules.
 *
 * Loads all rules via `listApprovalRules` and exposes upsert/delete actions
 * that call `upsertApprovalRule` / `deleteApprovalRule` on the ConnectRPC SessionService.
 *
 * Only "user" source rules can be edited or deleted; seed and claude-settings
 * rules are read-only.
 */ function useApprovalRules(options = {}) {
    const { sourceFilter } = options;
    const [rules, setRules] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)([]);
    const [loading, setLoading] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(false);
    const [error, setError] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(null);
    const clientRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        clientRef.current = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_4__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_5__/* .getConnectTransport */ .qW)());
    }, []);
    const fetchRules = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async ()=>{
        if (!clientRef.current) return;
        setLoading(true);
        setError(null);
        try {
            const req = (0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_3__/* .create */ .v)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .ListApprovalRulesRequestSchema */ .FyJ, {});
            if (sourceFilter) {
                req.sourceFilter = sourceFilter;
            }
            const resp = await clientRef.current.listApprovalRules(req);
            setRules(resp.rules ?? []);
        } catch (err) {
            const e = err instanceof Error ? err : new Error("Failed to fetch approval rules");
            setError(e);
            console.error("Failed to fetch approval rules:", e);
        } finally{
            setLoading(false);
        }
    }, [
        sourceFilter
    ]);
    const refresh = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async ()=>{
        await fetchRules();
    }, [
        fetchRules
    ]);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        refresh();
    }, [
        refresh
    ]);
    const upsertRule = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (ruleData)=>{
        if (!clientRef.current) return;
        const rule = buildApprovalRuleProto(ruleData);
        const req = (0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_3__/* .create */ .v)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .UpsertApprovalRuleRequestSchema */ .zid, {
            rule
        });
        await clientRef.current.upsertApprovalRule(req);
        await refresh();
    }, [
        refresh
    ]);
    const bulkUpsertRules = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (rulesData)=>{
        if (!clientRef.current) return {
            created: 0,
            updated: 0,
            skipped: 0,
            errors: []
        };
        const protos = rulesData.map((ruleData)=>buildApprovalRuleProto(ruleData));
        const req = (0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_3__/* .create */ .v)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .BulkUpsertRulesRequestSchema */ .bL3, {
            rules: protos,
            overwriteDuplicates: false
        });
        const resp = await clientRef.current.bulkUpsertRules(req);
        await refresh();
        return resp;
    }, [
        refresh
    ]);
    const deleteRule = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id)=>{
        if (!clientRef.current) return;
        const req = (0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_3__/* .create */ .v)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .DeleteApprovalRuleRequestSchema */ .P0c, {
            id
        });
        await clientRef.current.deleteApprovalRule(req);
        // Optimistic update
        setRules((prev)=>prev.filter((r)=>r.id !== id));
    }, []);
    const reloadClaudeSettingsRules = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async ()=>{
        if (!clientRef.current) {
            return (0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_3__/* .create */ .v)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .ReloadClaudeSettingsRulesResponseSchema */ .qVG, {
                success: false,
                ruleCount: 0,
                message: "Client not ready"
            });
        }
        const req = (0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_3__/* .create */ .v)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .ReloadClaudeSettingsRulesRequestSchema */ .Ep4, {});
        const resp = await clientRef.current.reloadClaudeSettingsRules(req);
        await refresh();
        return resp;
    }, [
        refresh
    ]);
    return {
        rules,
        loading,
        error,
        upsertRule,
        bulkUpsertRules,
        deleteRule,
        refresh,
        reloadClaudeSettingsRules
    };
}


/***/ }),

/***/ 94266:
/***/ (() => {



/***/ }),

/***/ 98619:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  P: () => (/* binding */ SeverityBadge),
  i: () => (/* binding */ getRiskLevelInfo)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ./src/lib/sessions/riskLevel.ts
var sessions_riskLevel = __webpack_require__(22309);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FSeverityBadge.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll83ZGYwenEwIHsKICBkaXNwbGF5OiBpbmxpbmUtZmxleDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeSk7CiAgcGFkZGluZzogM3B4IDEwcHg7CiAgYm9yZGVyLXJhZGl1czogdmFyKC0tXzE5ZXoycnozOCk7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNhKTsKICBmb250LXdlaWdodDogNjAwOwogIHdoaXRlLXNwYWNlOiBub3dyYXA7Cn0KLl83ZGYwenExIHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejE1KTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxNik7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnoxNCk7Cn0KLl83ZGYwenEyIHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejExKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxMik7Cn0KLl83ZGYwenEzIHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyenkpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyenopOwp9Ci5fN2RmMHpxNCB7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnp2KTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnp3KTsKfQouXzdkZjB6cTUgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6ZCk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7Cn0KLl83ZGYwenE2IHsKICBwYWRkaW5nOiAycHggNnB4Owp9Ci5fN2RmMHpxOCB7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKICBsaW5lLWhlaWdodDogMTsKfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FSeverityBadge_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll83ZGYwenEwIHsKICBkaXNwbGF5OiBpbmxpbmUtZmxleDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeSk7CiAgcGFkZGluZzogM3B4IDEwcHg7CiAgYm9yZGVyLXJhZGl1czogdmFyKC0tXzE5ZXoycnozOCk7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNhKTsKICBmb250LXdlaWdodDogNjAwOwogIHdoaXRlLXNwYWNlOiBub3dyYXA7Cn0KLl83ZGYwenExIHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejE1KTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxNik7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnoxNCk7Cn0KLl83ZGYwenEyIHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejExKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxMik7Cn0KLl83ZGYwenEzIHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyenkpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyenopOwp9Ci5fN2RmMHpxNCB7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnp2KTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnp3KTsKfQouXzdkZjB6cTUgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6ZCk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7Cn0KLl83ZGYwenE2IHsKICBwYWRkaW5nOiAycHggNnB4Owp9Ci5fN2RmMHpxOCB7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKICBsaW5lLWhlaWdodDogMTsKfQ_22_7D = __webpack_require__(53532);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+recipes@0.5.7+6974e6bd53375d7a/node_modules/@vanilla-extract/recipes/dist/createRuntimeFn-62c9670f.esm.js
var createRuntimeFn_62c9670f_esm = __webpack_require__(9196);
;// ./src/components/sessions/SeverityBadge.css.ts



var badge = (0,createRuntimeFn_62c9670f_esm.c)({defaultClassName:'_7df0zq0',variantClassNames:{level:{critical:'_7df0zq1',high:'_7df0zq2',medium:'_7df0zq3',low:'_7df0zq4',unknown:'_7df0zq5'},compact:{true:'_7df0zq6',false:'_7df0zq7'}},defaultVariants:{level:'unknown',compact:false},compoundVariants:[]});
var icon = '_7df0zq8';
;// ./src/components/sessions/SeverityBadge.tsx
/* __next_internal_client_entry_do_not_use__ getRiskLevelInfo,SeverityBadge auto */ 
// +feature: severity-badge


// getRiskLevelInfo maps a classifier.RiskLevel string ("low"/"medium"/"high"/"critical") to
// display info. "" or any unrecognized value renders the distinct "not recorded" state — never
// falls back to "Low" (ADR-001: under-communicating risk is worse than over-communicating it).
function getRiskLevelInfo(riskLevel) {
    switch(riskLevel){
        case "critical":
            return {
                label: sessions_riskLevel/* RISK_LEVEL_LABELS */.p.critical,
                abbr: "CRIT",
                icon: "⛔",
                variant: "critical",
                testIdSuffix: "critical"
            };
        case "high":
            return {
                label: sessions_riskLevel/* RISK_LEVEL_LABELS */.p.high,
                abbr: "HIGH",
                icon: "🔴",
                variant: "high",
                testIdSuffix: "high"
            };
        case "medium":
            return {
                label: sessions_riskLevel/* RISK_LEVEL_LABELS */.p.medium,
                abbr: "MED",
                icon: "🟠",
                variant: "medium",
                testIdSuffix: "medium"
            };
        case "low":
            return {
                label: sessions_riskLevel/* RISK_LEVEL_LABELS */.p.low,
                abbr: "LOW",
                icon: "🟢",
                variant: "low",
                testIdSuffix: "low"
            };
        default:
            return {
                label: "Severity not recorded",
                abbr: "N/A",
                icon: "⚪",
                variant: "unknown",
                testIdSuffix: "unrecorded"
            };
    }
}
function SeverityBadge({ riskLevel, compact = false }) {
    const info = getRiskLevelInfo(riskLevel);
    const ariaLabel = info.variant === "unknown" ? info.label : `${info.label} risk`;
    return(// role="img" (not "status"): this badge is static once mounted, not a live region.
    // role="status" implies aria-live="polite" — with many badges on one page (every
    // ReviewQueuePanel row, every ApprovalRulesPanel row), that risks a burst of
    // screen-reader announcements on initial render/re-sort instead of announcing only
    // genuine changes. "img" still exposes aria-label as a single readable unit.
    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
        className: badge({
            level: info.variant,
            compact
        }),
        role: "img",
        "aria-label": ariaLabel,
        title: ariaLabel,
        "data-testid": `severity-badge-${info.testIdSuffix}`,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                className: icon,
                "aria-hidden": "true",
                children: info.icon
            }),
            compact ? info.abbr : info.label
        ]
    }));
}


/***/ })

};
;