(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[12],{

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

/***/ 5382:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   H0: () => (/* binding */ SAVED_ANSWERS_CHANGED),
/* harmony export */   it: () => (/* binding */ useSavedAnswers)
/* harmony export */ });
/* unused harmony export SAVED_POLL_MS */
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(95444);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(66144);
/* harmony import */ var _gen_session_v1_guidance_request_pb__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(50207);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(21694);
/* harmony import */ var _lib_api_serialization__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(48443);
/* __next_internal_client_entry_do_not_use__ SAVED_POLL_MS,SAVED_ANSWERS_CHANGED,useSavedAnswers auto */ 





const SAVED_POLL_MS = 15000;
/** A card saw its answer become "saved for later": re-read the list now rather than at the next poll. */ const SAVED_ANSWERS_CHANGED = "ssq:saved-answers-changed";
/**
 * Answers stored for a stopped or archived session (agent-created requests only). Read while
 * `enabled` (the tray is open); a failed read keeps the last list, an empty list renders nothing.
 */ function useSavedAnswers(enabled) {
    const [rows, setRows] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)([]);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        if (!enabled) return;
        const ctrl = new AbortController();
        const read = ()=>Promise.resolve().then(()=>(0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__/* .createClient */ .UU)(_gen_session_v1_guidance_request_pb__WEBPACK_IMPORTED_MODULE_1__/* .GuidanceRequestService */ .h$, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_2__/* .getConnectTransport */ .qW)()).listSavedGuidanceAnswers((0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_guidance_request_pb__WEBPACK_IMPORTED_MODULE_1__/* .ListSavedGuidanceAnswersRequestSchema */ .zR, {}), {
                    signal: ctrl.signal
                })).then((res)=>setRows(res.requests.map((r)=>(0,_lib_api_serialization__WEBPACK_IMPORTED_MODULE_5__/* .toPlainObject */ .V)(r)))).catch(()=>{});
        void read();
        const timer = setInterval(()=>void read(), SAVED_POLL_MS);
        const onChanged = ()=>void read();
        window.addEventListener(SAVED_ANSWERS_CHANGED, onChanged);
        return ()=>{
            clearInterval(timer);
            window.removeEventListener(SAVED_ANSWERS_CHANGED, onChanged);
            ctrl.abort();
        };
    }, [
        enabled
    ]);
    return rows;
}


/***/ }),

/***/ 15562:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   s: () => (/* binding */ useClearResultLine)
/* harmony export */ });
/* unused harmony export keptLineText */
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _lib_hooks_useAnnounce__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(5647);
/* __next_internal_client_entry_do_not_use__ keptLineText,useClearResultLine auto */ 

/** The line shown (and announced once) when the server refused to delete rows; null when nothing was kept. */ function keptLineText(keptCount) {
    return keptCount > 0 ? "".concat(keptCount, " kept: still needs attention") : null;
}
/**
 * Shared by the tray and the Notifications page (Task 4.4f): the one place a
 * clear response's `kept` ids become a visible, once-announced line. The line
 * clears on the next user action via `clearKeptLine`; it carries no live role of
 * its own (the Announcer owns speech). The kept rows themselves never left local
 * history, so there is nothing to restore and no further RPC.
 */ function useClearResultLine() {
    const { announce } = (0,_lib_hooks_useAnnounce__WEBPACK_IMPORTED_MODULE_1__/* .useAnnounce */ .XE)();
    const [keptLine, setKeptLine] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(null);
    const applyClearResult = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)((result)=>{
        var _result_kept_length;
        const line = keptLineText((_result_kept_length = result === null || result === void 0 ? void 0 : result.kept.length) !== null && _result_kept_length !== void 0 ? _result_kept_length : 0);
        setKeptLine(line);
        if (line) announce(line, "polite", "clear-kept");
    }, [
        announce
    ]);
    const clearKeptLine = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>setKeptLine(null), []);
    return {
        keptLine,
        applyClearResult,
        clearKeptLine
    };
}


/***/ }),

/***/ 21411:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 28576:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Ui: () => (/* binding */ asOfLine),
/* harmony export */   Xc: () => (/* binding */ bannerButton),
/* harmony export */   vK: () => (/* binding */ banner)
/* harmony export */ });
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(65916);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FOfflineBanner_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xMzZkNjEzMCB7CiAgZGlzcGxheTogZmxleDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGp1c3RpZnktY29udGVudDogc3BhY2UtYmV0d2VlbjsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MnopOwogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MnopIDE2cHg7CiAgZm9udC1zaXplOiAwLjgxMjVyZW07CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnpkKTsKfQouXzEzNmQ2MTMxIHsKICBtaW4taGVpZ2h0OiA0NHB4OwogIHBhZGRpbmc6IDAgMTJweDsKICBjdXJzb3I6IHBvaW50ZXI7Cn0KLl8xMzZkNjEzMiB7CiAgbWFyZ2luOiAwOwogIHBhZGRpbmc6IDAgMTZweCB2YXIoLS1fMTllejJyejJ5KTsKICBmb250LXNpemU6IDAuNzVyZW07CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7Cn0_22_7D__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(44546);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FOfflineBanner_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xMzZkNjEzMCB7CiAgZGlzcGxheTogZmxleDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGp1c3RpZnktY29udGVudDogc3BhY2UtYmV0d2VlbjsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MnopOwogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MnopIDE2cHg7CiAgZm9udC1zaXplOiAwLjgxMjVyZW07CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnpkKTsKfQouXzEzNmQ2MTMxIHsKICBtaW4taGVpZ2h0OiA0NHB4OwogIHBhZGRpbmc6IDAgMTJweDsKICBjdXJzb3I6IHBvaW50ZXI7Cn0KLl8xMzZkNjEzMiB7CiAgbWFyZ2luOiAwOwogIHBhZGRpbmc6IDAgMTZweCB2YXIoLS1fMTllejJyejJ5KTsKICBmb250LXNpemU6IDAuNzVyZW07CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7Cn0_22_7D__WEBPACK_IMPORTED_MODULE_1___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FOfflineBanner_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xMzZkNjEzMCB7CiAgZGlzcGxheTogZmxleDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGp1c3RpZnktY29udGVudDogc3BhY2UtYmV0d2VlbjsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MnopOwogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MnopIDE2cHg7CiAgZm9udC1zaXplOiAwLjgxMjVyZW07CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnpkKTsKfQouXzEzNmQ2MTMxIHsKICBtaW4taGVpZ2h0OiA0NHB4OwogIHBhZGRpbmc6IDAgMTJweDsKICBjdXJzb3I6IHBvaW50ZXI7Cn0KLl8xMzZkNjEzMiB7CiAgbWFyZ2luOiAwOwogIHBhZGRpbmc6IDAgMTZweCB2YXIoLS1fMTllejJyejJ5KTsKICBmb250LXNpemU6IDAuNzVyZW07CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7Cn0_22_7D__WEBPACK_IMPORTED_MODULE_1__);


var asOfLine = '_136d6132';
var banner = '_136d6130';
var bannerButton = '_136d6131';

/***/ }),

/***/ 33741:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   RQ: () => (/* binding */ formatClockTime),
/* harmony export */   ZP: () => (/* binding */ ActionsDisabledReasonContext),
/* harmony export */   ky: () => (/* binding */ useAwaitingFresh),
/* harmony export */   lj: () => (/* binding */ useActionsDisabledReason),
/* harmony export */   op: () => (/* binding */ PendingNotice),
/* harmony export */   sO: () => (/* binding */ OfflineBanner),
/* harmony export */   y0: () => (/* binding */ actionsDisabledReason)
/* harmony export */ });
/* unused harmony exports BANNER_COPY, LOADING_DELAY_MS, STILL_LOADING_MS, CANT_REACH_MS, STILL_SENDING_MS, NO_RESPONSE_MS, STILL_SENDING_COPY, NO_RESPONSE_COPY, backoffDelayMs */
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(61574);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(55494);
/* harmony import */ var _OfflineBanner_css__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(28576);
/* __next_internal_client_entry_do_not_use__ BANNER_COPY,LOADING_DELAY_MS,STILL_LOADING_MS,CANT_REACH_MS,STILL_SENDING_MS,NO_RESPONSE_MS,STILL_SENDING_COPY,NO_RESPONSE_COPY,backoffDelayMs,formatClockTime,OfflineBanner,PendingNotice,ActionsDisabledReasonContext,useActionsDisabledReason,actionsDisabledReason,useAwaitingFresh auto */ 


const BANNER_COPY = {
    offline: "Offline. Showing last known state.",
    reconnecting: "Reconnecting to the server. Showing last known state.",
    cant_reach: "Can't reach the server.",
    loading: "Loading...",
    still_loading: "Still loading."
};
const LOADING_DELAY_MS = 150;
const STILL_LOADING_MS = 8000;
const CANT_REACH_MS = 30000;
const STILL_SENDING_MS = 10000;
const NO_RESPONSE_MS = 30000;
const BACKOFF_START_MS = 1000;
const BACKOFF_MAX_MS = 15000;
const STILL_SENDING_COPY = "Still sending... this is taking longer than usual.";
const NO_RESPONSE_COPY = "No response from the server. The action may or may not have been applied. Open the session to check.";
/** Auto-retry delay for reconnect attempt `attempt` (0-based): 1 s doubling to 15 s. */ function backoffDelayMs(attempt) {
    return Math.min(BACKOFF_START_MS * 2 ** attempt, BACKOFF_MAX_MS);
}
/** Clock time for "Showing data from" and "As of" lines. */ function formatClockTime(ms) {
    return new Date(ms).toLocaleTimeString([], {
        hour: "2-digit",
        minute: "2-digit"
    });
}
/** True once `active` has stayed true for `ms`; resets the moment it goes false. */ function useActiveFor(active, ms) {
    const [elapsed, setElapsed] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(false);
    (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        if (!active) {
            setElapsed(false);
            return;
        }
        const timer = setTimeout(()=>setElapsed(true), ms);
        return ()=>clearTimeout(timer);
    }, [
        active,
        ms
    ]);
    return active && elapsed;
}
/**
 * The one connectivity banner (ux.md section 9): a persistent text element, not a live
 * region. The panel's single polite region announces the state change.
 */ function OfflineBanner(param) {
    let { online, failed = false, loading = false, hasData = true, onRetry, showRetry = true, testId = "offline-banner" } = param;
    const coldLoading = online && !failed && loading && !hasData;
    const showLoading = useActiveFor(coldLoading, LOADING_DELAY_MS);
    const stillLoading = useActiveFor(coldLoading, STILL_LOADING_MS);
    const reconnecting = online && failed;
    const cantReach = useActiveFor(reconnecting, CANT_REACH_MS);
    const retryRef = (0,react__WEBPACK_IMPORTED_MODULE_1__.useRef)(onRetry);
    retryRef.current = onRetry;
    const autoRetry = reconnecting && !!onRetry;
    const [attempt, setAttempt] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(0);
    (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        if (!autoRetry) {
            setAttempt(0);
            return;
        }
        const timer = setTimeout(()=>{
            var _retryRef_current;
            (_retryRef_current = retryRef.current) === null || _retryRef_current === void 0 ? void 0 : _retryRef_current.call(retryRef);
            setAttempt((a)=>a + 1);
        }, backoffDelayMs(attempt));
        return ()=>clearTimeout(timer);
    }, [
        autoRetry,
        attempt
    ]);
    let state = null;
    if (!online) state = "offline";
    else if (reconnecting) state = cantReach ? "cant_reach" : "reconnecting";
    else if (stillLoading) state = "still_loading";
    else if (showLoading) state = "loading";
    if (!state) return null;
    const retryable = state === "cant_reach" || state === "still_loading";
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
        className: _OfflineBanner_css__WEBPACK_IMPORTED_MODULE_2__/* .banner */ .vK,
        "data-testid": testId,
        "data-state": state,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("span", {
                children: BANNER_COPY[state]
            }),
            retryable && showRetry && onRetry && // analytics-exempt
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("button", {
                type: "button",
                className: _OfflineBanner_css__WEBPACK_IMPORTED_MODULE_2__/* .bannerButton */ .Xc,
                onClick: onRetry,
                children: "Retry"
            })
        ]
    });
}
/** In-row notice for a request pending 10 s (not announced) and 30 s (Retry on the same idempotency key). */ function PendingNotice(param) {
    let { pending, onRetry } = param;
    const slow = useActiveFor(pending, STILL_SENDING_MS);
    const stuck = useActiveFor(pending, NO_RESPONSE_MS);
    if (!slow) return null;
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
        "data-testid": "pending-notice",
        "data-state": stuck ? "no_response" : "still_sending",
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("span", {
                children: stuck ? NO_RESPONSE_COPY : STILL_SENDING_COPY
            }),
            stuck && // analytics-exempt
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("button", {
                type: "button",
                className: _OfflineBanner_css__WEBPACK_IMPORTED_MODULE_2__/* .bannerButton */ .Xc,
                onClick: onRetry,
                children: "Retry"
            })
        ]
    });
}
const ActionsDisabledReasonContext = /*#__PURE__*/ (0,react__WEBPACK_IMPORTED_MODULE_1__.createContext)(undefined);
function useActionsDisabledReason() {
    return (0,react__WEBPACK_IMPORTED_MODULE_1__.useContext)(ActionsDisabledReasonContext);
}
function actionsDisabledReason(input) {
    if (!input.online) return "Offline";
    if (input.reconnecting) return "Reconnecting";
    if (input.awaitingFresh) return "Updating...";
    return undefined;
}
/**
 * After `unavailable` ends, stays true until `refreshedAt` moves past its value from when
 * the outage began, so a stale card cannot act on an item resolved meanwhile.
 */ function useAwaitingFresh(unavailable, refreshedAt) {
    const [outage, setOutage] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(null);
    if (unavailable && outage === null) setOutage({
        at: refreshedAt
    });
    const fresh = outage !== null && refreshedAt !== null && (outage.at === null || refreshedAt > outage.at);
    if (outage !== null && !unavailable && fresh) setOutage(null);
    return outage !== null && !unavailable && !fresh;
}


/***/ }),

/***/ 37349:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   b: () => (/* binding */ useApprovalResolution)
/* harmony export */ });
/* unused harmony export APPROVAL_RETRY_MESSAGE */
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(66144);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(45640);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__ = __webpack_require__(16294);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(21694);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(23575);
/* harmony import */ var _bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(95444);






/** Shown for any resolveApproval rejection that isn't the FailedPrecondition
 *  reconciliation-race case — a transient failure the user can retry. */ const APPROVAL_RETRY_MESSAGE = "Couldn't record your decision — try again.";
/**
 * Shared approval-resolution state/logic for the notification history views
 * (NotificationsPage and NotificationPanel): seeds resolved state from persisted
 * notification metadata, tracks in-flight/blocked approvals, and resolves an
 * approval via RPC.
 */ function useApprovalResolution(param) {
    let { notificationHistory, acknowledgeNotification } = param;
    const clientRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    const getClient = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>{
        if (!clientRef.current) {
            clientRef.current = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_2__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_1__/* .getConnectTransport */ .qW)());
        }
        return clientRef.current;
    }, []);
    const [resolvedApprovals, setResolvedApprovals] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)({});
    const [pendingApprovals, setPendingApprovals] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)({});
    const [blockedApprovals, setBlockedApprovals] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)({});
    const [failedApprovals, setFailedApprovals] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)({});
    // Seed resolvedApprovals from persisted metadata when history loads or updates.
    // The server stamps "approval_decision" on the notification record when an approval
    // is resolved, so this survives page refreshes.
    // "timeout" and "canceled" are server-side outcomes where the approval was not handled
    // via the UI; we map them to "expired" for display (same "Expired" badge).
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        const seeded = {};
        for (const n of notificationHistory){
            var _n_metadata, _n_metadata1;
            const decision = (_n_metadata = n.metadata) === null || _n_metadata === void 0 ? void 0 : _n_metadata["approval_decision"];
            const approvalId = (_n_metadata1 = n.metadata) === null || _n_metadata1 === void 0 ? void 0 : _n_metadata1["approval_id"];
            if (!approvalId || !decision) continue;
            if (decision === "allow" || decision === "deny") {
                seeded[approvalId] = decision;
            } else if (decision === "timeout" || decision === "canceled") {
                seeded[approvalId] = "expired";
            }
        }
        if (Object.keys(seeded).length > 0) {
            setResolvedApprovals((prev)=>({
                    ...seeded,
                    ...prev
                }));
        }
    }, [
        notificationHistory
    ]);
    const resolveApproval = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (approvalId, decision, notificationIds, overrideCiBlock)=>{
        setPendingApprovals((prev)=>({
                ...prev,
                [approvalId]: true
            }));
        try {
            // One-shot mutation triggered by an explicit user action (Approve/Deny
            // click), not tied to a mount/effect.
            // abort-signal-exempt
            await getClient().resolveApproval((0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_2__/* .ResolveApprovalRequestSchema */ .I6_, {
                approvalId,
                decision,
                overrideCiBlock
            }));
            setResolvedApprovals((prev)=>({
                    ...prev,
                    [approvalId]: decision
                }));
            setBlockedApprovals((prev)=>{
                const next = {
                    ...prev
                };
                delete next[approvalId];
                return next;
            });
            setFailedApprovals((prev)=>{
                const next = {
                    ...prev
                };
                delete next[approvalId];
                return next;
            });
            // Single call: marks as read in history AND closes the active toast
            acknowledgeNotification(notificationIds);
        } catch (err) {
            // AC5: CI-red block — show an inline explanation next to Approve/Deny instead of
            // collapsing into the generic "expired" state (not a silent no-op).
            if (err instanceof _connectrpc_connect__WEBPACK_IMPORTED_MODULE_5__/* .ConnectError */ .T && err.code === _connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__/* .Code */ .C.FailedPrecondition) {
                setBlockedApprovals((prev)=>({
                        ...prev,
                        [approvalId]: err.rawMessage || err.message
                    }));
                return;
            }
            console.error("Failed to resolve approval:", err);
            // Any other rejection (network error, RPC 5xx, etc.) is a transient
            // failure, not evidence the approval expired — keep it actionable so
            // the user can retry rather than dead-ending on a misleading "Expired" badge.
            setFailedApprovals((prev)=>({
                    ...prev,
                    [approvalId]: APPROVAL_RETRY_MESSAGE
                }));
        } finally{
            setPendingApprovals((prev)=>{
                const next = {
                    ...prev
                };
                delete next[approvalId];
                return next;
            });
        }
    }, [
        getClient,
        acknowledgeNotification
    ]);
    return {
        resolvedApprovals,
        pendingApprovals,
        blockedApprovals,
        failedApprovals,
        resolveApproval
    };
}


/***/ }),

/***/ 37865:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   J: () => (/* binding */ useFocusRestoreOnRemoval)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);

/**
 * Restores keyboard focus to a sensible sibling item when the currently
 * focused item disappears from `ids` (e.g. after auto-resolution or
 * dismissal), falling back to `fallbackRef` when no sibling remains.
 *
 * Callers track which item currently holds focus by wiring the returned ref
 * to each row's `onFocus`/`onBlur` handlers (see ReviewQueuePanel.tsx's
 * `renderQueueItem` and NotificationItem.tsx's `NeedsDecisionSection` for the
 * two call sites this was extracted from).
 *
 * `resolveElement` is looked up fresh inside the effect rather than cached,
 * so it should be stable across renders (e.g. wrapped in `useCallback`) to
 * avoid re-running the effect on every render.
 */ function useFocusRestoreOnRemoval(ids, resolveElement, fallbackRef) {
    const previousIdsRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(ids);
    const focusedIdRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        const currentIds = ids;
        const currentIdSet = new Set(currentIds);
        const previousIds = previousIdsRef.current;
        const focusedId = focusedIdRef.current;
        let rafId;
        if (focusedId && previousIds.includes(focusedId) && !currentIdSet.has(focusedId)) {
            const removedIndex = previousIds.indexOf(focusedId);
            const remainingIds = previousIds.filter((id)=>currentIdSet.has(id));
            var _remainingIds_removedIndex;
            const nextFocusId = (_remainingIds_removedIndex = remainingIds[removedIndex]) !== null && _remainingIds_removedIndex !== void 0 ? _remainingIds_removedIndex : remainingIds[removedIndex - 1];
            focusedIdRef.current = null;
            // Wait a tick for the removed row's DOM node to actually unmount.
            rafId = requestAnimationFrame(()=>{
                const el = nextFocusId ? resolveElement(nextFocusId) : null;
                if (el) {
                    el.focus();
                } else {
                    var _fallbackRef_current;
                    (_fallbackRef_current = fallbackRef.current) === null || _fallbackRef_current === void 0 ? void 0 : _fallbackRef_current.focus();
                }
            });
        }
        previousIdsRef.current = currentIds;
        return ()=>{
            if (rafId !== undefined) cancelAnimationFrame(rafId);
        };
    }, [
        ids,
        resolveElement,
        fallbackRef
    ]);
    return focusedIdRef;
}


/***/ }),

/***/ 44543:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  FB: () => (/* binding */ AutoHandledSection),
  BF: () => (/* binding */ NeedsDecisionSection),
  rn: () => (/* binding */ NotificationItem)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ./src/lib/nav/index.ts
var nav = __webpack_require__(41078);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/hooks/useFocusRestoreOnRemoval.ts
var useFocusRestoreOnRemoval = __webpack_require__(37865);
// EXTERNAL MODULE: ./src/lib/hooks/useSessionHidden.ts
var useSessionHidden = __webpack_require__(76468);
// EXTERNAL MODULE: ./src/lib/contexts/FeatureFlagsContext.tsx
var FeatureFlagsContext = __webpack_require__(63367);
// EXTERNAL MODULE: ./src/lib/hooks/useFeatureFlagValue.ts
var useFeatureFlagValue = __webpack_require__(43661);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/create.js
var create = __webpack_require__(95444);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ./src/gen/session/v1/guidance_request_pb.ts
var guidance_request_pb = __webpack_require__(50207);
// EXTERNAL MODULE: ./src/lib/api/transport.ts
var transport = __webpack_require__(21694);
// EXTERNAL MODULE: ./src/lib/api/serialization.ts
var serialization = __webpack_require__(48443);
;// ./src/lib/hooks/useGuidanceRow.ts
/* __next_internal_client_entry_do_not_use__ GUIDANCE_POLL_MS,isMoving,useGuidanceRow auto */ 





const GUIDANCE_POLL_MS = 3000;
/** Delivery states that still change on their own: pending, sent (acknowledgement or the 60 s decision). */ const MOVING = new Set([
    "pending",
    "sent"
]);
/** A row that can still change without the operator: open, or answered and not yet settled. */ function isMoving(row) {
    if (!row) return true;
    if (row.status === "pending") return true;
    var _row_deliveryState;
    return row.status === "answered" && MOVING.has((_row_deliveryState = row.deliveryState) !== null && _row_deliveryState !== void 0 ? _row_deliveryState : "");
}
/**
 * Reads one durable question and re-reads it every few seconds while it can still change (an
 * open card, or an answer waiting on acknowledgement). A failed read keeps the last row. Pass
 * `undefined` to skip, e.g. while the structured-question flag is off.
 */ function useGuidanceRow(id, keepFresh) {
    const [row, setRow] = (0,react.useState)(undefined);
    const live = keepFresh || isMoving(row);
    (0,react.useEffect)(()=>{
        if (!id) return;
        const ctrl = new AbortController();
        const read = ()=>Promise.resolve().then(()=>(0,promise_client/* createClient */.UU)(guidance_request_pb/* GuidanceRequestService */.h$, (0,transport/* getConnectTransport */.qW)()).getGuidanceRequest((0,create/* create */.v)(guidance_request_pb/* GetGuidanceRequestRequestSchema */.g3, {
                    id
                }), {
                    signal: ctrl.signal
                })).then((res)=>res.request && setRow((0,serialization/* toPlainObject */.V)(res.request))).catch(()=>{});
        void read();
        if (!live) return ()=>ctrl.abort();
        const timer = setInterval(()=>void read(), GUIDANCE_POLL_MS);
        return ()=>{
            clearInterval(timer);
            ctrl.abort();
        };
    }, [
        id,
        live
    ]);
    return id ? row : undefined;
}

// EXTERNAL MODULE: ./src/lib/hooks/useSavedAnswers.ts
var useSavedAnswers = __webpack_require__(5382);
// EXTERNAL MODULE: ./src/lib/contexts/notificationContexts.ts
var notificationContexts = __webpack_require__(82514);
// EXTERNAL MODULE: ./src/gen/session/v1/notification_action_pb.ts
var notification_action_pb = __webpack_require__(75449);
// EXTERNAL MODULE: ./src/lib/hooks/useAnnounce.ts
var useAnnounce = __webpack_require__(5647);
// EXTERNAL MODULE: ./src/lib/hooks/useEngagedCard.ts + 1 modules
var useEngagedCard = __webpack_require__(27924);
// EXTERNAL MODULE: ./src/lib/api/notificationActionApi.ts
var notificationActionApi = __webpack_require__(71208);
;// ./src/lib/hooks/useInvokeNotificationAction.ts
/* __next_internal_client_entry_do_not_use__ useInvokeNotificationAction auto */ 

const IDLE = {
    status: "idle"
};
function newKey() {
    var _globalThis_crypto_randomUUID, _globalThis_crypto;
    var _globalThis_crypto_randomUUID1;
    return (_globalThis_crypto_randomUUID1 = (_globalThis_crypto = globalThis.crypto) === null || _globalThis_crypto === void 0 ? void 0 : (_globalThis_crypto_randomUUID = _globalThis_crypto.randomUUID) === null || _globalThis_crypto_randomUUID === void 0 ? void 0 : _globalThis_crypto_randomUUID.call(_globalThis_crypto)) !== null && _globalThis_crypto_randomUUID1 !== void 0 ? _globalThis_crypto_randomUUID1 : "k-".concat(Date.now(), "-").concat(Math.random().toString(36).slice(2));
}
/**
 * Sends one inline action. `invoke` starts an attempt with a fresh idempotency key;
 * `retry` re-sends the last spec under the same key, so a duplicate write is impossible.
 * The row layer wraps `invoke` for any hold before dispatch (commit delay).
 */ function useInvokeNotificationAction() {
    const [state, setState] = (0,react.useState)(IDLE);
    const attempt = (0,react.useRef)(null);
    const mounted = (0,react.useRef)(true);
    (0,react.useEffect)(()=>{
        mounted.current = true;
        return ()=>{
            mounted.current = false;
        };
    }, []);
    const send = (0,react.useCallback)(async (spec, key)=>{
        attempt.current = {
            spec,
            key
        };
        setState({
            status: "pending",
            spec
        });
        try {
            var _spec_sessionId;
            const response = await (0,notificationActionApi/* invokeNotificationAction */.WQ)({
                actionKind: spec.kind,
                targetKeyType: spec.targetKeyType,
                targetKey: spec.targetKey,
                sessionId: (_spec_sessionId = spec.sessionId) !== null && _spec_sessionId !== void 0 ? _spec_sessionId : "",
                params: spec.params,
                idempotencyKey: key
            });
            if (mounted.current) setState({
                status: "settled",
                spec,
                response
            });
            return response;
        } catch (e) {
            if (mounted.current) setState({
                status: "failed",
                spec
            });
            return null;
        }
    }, []);
    const invoke = (0,react.useCallback)((spec)=>send(spec, newKey()), [
        send
    ]);
    const retry = (0,react.useCallback)(()=>{
        const last = attempt.current;
        return last ? send(last.spec, last.key) : Promise.resolve(null);
    }, [
        send
    ]);
    const reset = (0,react.useCallback)(()=>setState(IDLE), []);
    return {
        state,
        invoke,
        retry,
        reset
    };
}

// EXTERNAL MODULE: ./src/lib/contexts/deckViewportContext.ts
var deckViewportContext = __webpack_require__(44865);
// EXTERNAL MODULE: ./src/lib/utils/sessionDrafts.ts
var sessionDrafts = __webpack_require__(146);
// EXTERNAL MODULE: ./src/lib/copy/notificationCopy.ts
var notificationCopy = __webpack_require__(77448);
// EXTERNAL MODULE: ./src/lib/hooks/interactionTiming.ts
var interactionTiming = __webpack_require__(29901);
;// ./src/lib/hooks/useCommitDelay.ts
/* __next_internal_client_entry_do_not_use__ useCommitDelay auto */ 



const SECOND_MS = 1000;
/**
 * The 3 s hold behind Approve, Send back and Mark done (design/ux.md 2.4). Leaving cancels and
 * never commits: panel close, an outside click, route change, Back, `visibilitychange` to hidden,
 * `pagehide`, going offline or unmounting mid-hold all send nothing. The countdown is plain text.
 */ function useCommitDelay(param) {
    let { onCommit, onCancel, insideSelector = "[data-notification-tray]" } = param;
    const [held, setHeld] = (0,react.useState)(null);
    const [secondsLeft, setSecondsLeft] = (0,react.useState)(0);
    const heldRef = (0,react.useRef)(null);
    const timers = (0,react.useRef)({});
    const handlers = (0,react.useRef)({
        onCommit,
        onCancel
    });
    handlers.current = {
        onCommit,
        onCancel
    };
    const tracker = (0,useEngagedCard/* useEngagement */.Tq)();
    const pathname = (0,nav/* usePathname */.a8)();
    const stop = (0,react.useCallback)(()=>{
        clearTimeout(timers.current.commit);
        clearInterval(timers.current.tick);
        timers.current = {};
        heldRef.current = null;
        setHeld(null);
    }, []);
    const cancel = (0,react.useCallback)((reason)=>{
        const current = heldRef.current;
        if (current === null) return;
        stop();
        handlers.current.onCancel(current, reason);
    }, [
        stop
    ]);
    const start = (0,react.useCallback)((next)=>{
        const delay = (0,interactionTiming/* getTiming */.Hl)().commitDelayMs;
        if (delay <= 0) {
            handlers.current.onCommit(next);
            return;
        }
        stop();
        heldRef.current = next;
        setHeld(next);
        setSecondsLeft(Math.ceil(delay / SECOND_MS));
        const startedAt = Date.now();
        timers.current.tick = setInterval(()=>setSecondsLeft(Math.max(1, Math.ceil((delay - (Date.now() - startedAt)) / SECOND_MS))), SECOND_MS);
        timers.current.commit = setTimeout(()=>{
            const committing = heldRef.current;
            stop();
            if (committing !== null) handlers.current.onCommit(committing);
        }, delay);
    }, [
        stop
    ]);
    const undo = (0,react.useCallback)(()=>cancel("undo"), [
        cancel
    ]);
    const leave = (0,react.useCallback)(()=>cancel("leave"), [
        cancel
    ]);
    (0,react.useEffect)(()=>{
        if (held === null) return;
        const onVisibility = ()=>document.visibilityState === "hidden" && leave();
        const onPointerDown = (e)=>{
            var _e_target_closest, _e_target;
            if (!((_e_target = e.target) === null || _e_target === void 0 ? void 0 : (_e_target_closest = _e_target.closest) === null || _e_target_closest === void 0 ? void 0 : _e_target_closest.call(_e_target, insideSelector))) leave();
        };
        document.addEventListener("visibilitychange", onVisibility);
        document.addEventListener("pointerdown", onPointerDown, true);
        window.addEventListener("pagehide", leave);
        window.addEventListener("offline", leave);
        window.addEventListener("popstate", leave);
        const offClose = tracker === null || tracker === void 0 ? void 0 : tracker.onClose(leave);
        return ()=>{
            document.removeEventListener("visibilitychange", onVisibility);
            document.removeEventListener("pointerdown", onPointerDown, true);
            window.removeEventListener("pagehide", leave);
            window.removeEventListener("offline", leave);
            window.removeEventListener("popstate", leave);
            offClose === null || offClose === void 0 ? void 0 : offClose();
        };
    }, [
        held,
        leave,
        tracker,
        insideSelector
    ]);
    // A route change mid-hold is leaving too.
    const firstPath = (0,react.useRef)(pathname);
    (0,react.useEffect)(()=>{
        if (pathname !== firstPath.current) {
            firstPath.current = pathname;
            leave();
        }
    }, [
        pathname,
        leave
    ]);
    // Unmounting mid-hold (the row left the list, the page closed) also cancels.
    (0,react.useEffect)(()=>leave, [
        leave
    ]);
    return {
        held,
        secondsLeft,
        start,
        undo
    };
}

// EXTERNAL MODULE: ./src/lib/contexts/AnalyticsContext.tsx
var AnalyticsContext = __webpack_require__(24044);
;// ./src/lib/hooks/useInlineActionTelemetry.ts
/* __next_internal_client_entry_do_not_use__ INLINE_ACTION_UNDO_EVENT,useInlineActionTelemetry auto */ 


/** The server counts this name into inline_action_undo_total{kind}. */ const INLINE_ACTION_UNDO_EVENT = "inline_action_undo";
/** Undo cancels during the commit delay, so only the client can report it. Inert without a provider. */ function useInlineActionTelemetry() {
    const analytics = (0,react.useContext)(AnalyticsContext/* AnalyticsContext */.Ig);
    const trackUndo = (0,react.useCallback)((kind)=>{
        var _ActionKind_kind;
        analytics === null || analytics === void 0 ? void 0 : analytics.track({
            name: INLINE_ACTION_UNDO_EVENT,
            category: "user_action",
            component: "ActionRow",
            labels: {
                kind: ((_ActionKind_kind = notification_action_pb/* ActionKind */.LN[kind]) !== null && _ActionKind_kind !== void 0 ? _ActionKind_kind : "unknown").toLowerCase()
            }
        });
    }, [
        analytics
    ]);
    return {
        trackUndo
    };
}

// EXTERNAL MODULE: ./src/components/ui/OfflineBanner.tsx
var OfflineBanner = __webpack_require__(33741);
// EXTERNAL MODULE: ./src/components/ui/notification-actions/ActionButton.tsx
var ActionButton = __webpack_require__(62590);
;// ./src/components/ui/notification-actions/actionSpec.ts


const OPTION_KINDS = new Set([
    notification_action_pb/* ActionKind */.LN.ANSWER_QUESTION,
    notification_action_pb/* ActionKind */.LN.ANSWER_GUIDANCE,
    notification_action_pb/* ActionKind */.LN.BACKLOG_DECISION
]);
const CANCEL_KINDS = new Set([
    notification_action_pb/* ActionKind */.LN.CANCEL_WAKE,
    notification_action_pb/* ActionKind */.LN.CANCEL_PEER_HELP
]);
/** Kinds this client can render; anything else degrades to Open only. */ const KNOWN_KINDS = new Set([
    notification_action_pb/* ActionKind */.LN.APPROVE,
    notification_action_pb/* ActionKind */.LN.DENY,
    ...OPTION_KINDS,
    ...CANCEL_KINDS
]);
const MERGE_CHOICE = "merge";
const kindSlug = (k)=>notification_action_pb/* ActionKind */.LN[k].toLowerCase();
const isMerge = (p)=>{
    var _p_option;
    return ((_p_option = p.option) === null || _p_option === void 0 ? void 0 : _p_option.value) === MERGE_CHOICE;
};
/** Cancel actions, merge and any `requires_confirm` descriptor run an inline confirm first. */ function needsConfirm(p) {
    return CANCEL_KINDS.has(p.descriptor.kind) || p.descriptor.requiresConfirm || isMerge(p);
}
function buildSpec(p) {
    let extra = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : {};
    const { descriptor: d, option } = p;
    const params = {
        ...extra
    };
    if (d.kind === notification_action_pb/* ActionKind */.LN.BACKLOG_DECISION) params.choice = option === null || option === void 0 ? void 0 : option.value;
    else if (OPTION_KINDS.has(d.kind)) params.answer = option === null || option === void 0 ? void 0 : option.label;
    return {
        kind: d.kind,
        targetKeyType: d.targetKeyType,
        targetKey: d.targetKey,
        sessionId: d.sessionId,
        params
    };
}
function unavailableReason(d) {
    return d.unavailableReason === "desktop_only" ? notificationCopy/* COPY */.sF.desktopOnly : notificationCopy/* COPY */.sF.forbidden;
}
function pendingVerb(kind) {
    if (kind === notification_action_pb/* ActionKind */.LN.APPROVE) return notificationCopy/* PENDING_VERB */.gH.approve;
    if (kind === notification_action_pb/* ActionKind */.LN.DENY) return notificationCopy/* PENDING_VERB */.gH.deny;
    return notificationCopy/* PENDING_VERB */.gH.other;
}
/** Approve, Send back (`rework`) and Mark done (`accept`) wait behind Undo; nothing else does. */ function holdKeyOf(spec) {
    var _spec_params, _spec_params1;
    if (spec.kind === notification_action_pb/* ActionKind */.LN.APPROVE) return "approve";
    if (spec.kind === notification_action_pb/* ActionKind */.LN.BACKLOG_DECISION && (((_spec_params = spec.params) === null || _spec_params === void 0 ? void 0 : _spec_params.choice) === "rework" || ((_spec_params1 = spec.params) === null || _spec_params1 === void 0 ? void 0 : _spec_params1.choice) === "accept")) return spec.params.choice;
    return null;
}

;// ./src/components/ui/notification-actions/layout.ts
/** Touch target below 900px (ux.md shared constants). */ const TOUCH_TARGET_PX = 44;
/** The column gap is 8px; Approve adds this so Deny-to-Approve is 24px on a phone (UX-AC65). */ const APPROVE_EXTRA_GAP_PX = 16;

// EXTERNAL MODULE: ./src/components/ui/notification-actions/ActionRow.css.ts
var ActionRow_css = __webpack_require__(48241);
;// ./src/components/ui/notification-actions/ActionButtons.tsx
/* __next_internal_client_entry_do_not_use__ ApprovalButtons,OptionGroup,CancelButton,OtherButtons auto */ 






function PrimaryButton(param) {
    let { d, ui } = param;
    var _ui_holding;
    const isApprove = d.kind === notification_action_pb/* ActionKind */.LN.APPROVE;
    const holdText = isApprove && ((_ui_holding = ui.holding) === null || _ui_holding === void 0 ? void 0 : _ui_holding.key) === "approve" ? ui.holding.text : undefined;
    const label = holdText !== null && holdText !== void 0 ? holdText : ui.pendingKind === d.kind ? (0,notificationCopy/* pendingLabel */.AE)(pendingVerb(d.kind)) : d.label;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionButton/* ActionButton */.r, {
        id: kindSlug(d.kind),
        label: label,
        variant: isApprove ? ActionRow_css/* approveVariant */.TE : ActionRow_css/* denyVariant */.Av,
        disabled: ui.blocked || !d.available,
        reasonId: ui.reasonFor(d),
        minHeight: ui.minHeight,
        style: ui.mobile && isApprove ? {
            marginBlockStart: APPROVE_EXTRA_GAP_PX
        } : undefined,
        onClick: ()=>ui.onPress({
                descriptor: d
            })
    });
}
/** Desktop: Approve, Deny. Phone: Deny first, so a thumb's first target is the safe one. */ function ApprovalButtons(param) {
    let { approve, deny, ui } = param;
    const a = approve && /*#__PURE__*/ (0,jsx_runtime.jsx)(PrimaryButton, {
        d: approve,
        ui: ui
    }, "a");
    const d = deny && /*#__PURE__*/ (0,jsx_runtime.jsx)(PrimaryButton, {
        d: deny,
        ui: ui
    }, "d");
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(jsx_runtime.Fragment, {
        children: ui.mobile ? [
            d,
            a
        ] : [
            a,
            d
        ]
    });
}
/** Answer and decision options, one button each inside a labelled group. */ function OptionGroup(param) {
    let { d, ui } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        role: "group",
        "aria-label": d.label,
        className: ActionRow_css/* buttons */.Uo,
        children: d.options.map((o, i)=>{
            var _ui_holding;
            return /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionButton/* ActionButton */.r, {
                id: "".concat(kindSlug(d.kind), "-").concat(o.value || i),
                label: d.kind === notification_action_pb/* ActionKind */.LN.BACKLOG_DECISION && ((_ui_holding = ui.holding) === null || _ui_holding === void 0 ? void 0 : _ui_holding.key) === o.value ? ui.holding.text : o.label,
                variant: ActionRow_css/* secondaryVariant */.OL,
                disabled: ui.blocked || !d.available,
                reasonId: ui.reasonFor(d),
                minHeight: ui.minHeight,
                option: true,
                onClick: ()=>ui.onPress({
                        descriptor: d,
                        option: o
                    })
            }, "".concat(o.value, "-").concat(o.label));
        })
    });
}
function CancelButton(param) {
    let { d, ui } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionButton/* ActionButton */.r, {
        id: kindSlug(d.kind),
        label: (0,notificationCopy/* cancelPrompt */.zl)(d.kind === notification_action_pb/* ActionKind */.LN.CANCEL_PEER_HELP ? "request" : "prompt"),
        variant: ActionRow_css/* secondaryVariant */.OL,
        disabled: ui.blocked || !d.available,
        reasonId: ui.reasonFor(d),
        minHeight: ui.minHeight,
        onClick: ()=>ui.onPress({
                descriptor: d
            })
    });
}
/** Every descriptor other than the approval pair, in server order. */ function OtherButtons(param) {
    let { descriptors, ui } = param;
    return descriptors.map((d)=>{
        if (OPTION_KINDS.has(d.kind)) return /*#__PURE__*/ (0,jsx_runtime.jsx)(OptionGroup, {
            d: d,
            ui: ui
        }, "".concat(d.kind, "-").concat(d.targetKey));
        if (CANCEL_KINDS.has(d.kind)) return /*#__PURE__*/ (0,jsx_runtime.jsx)(CancelButton, {
            d: d,
            ui: ui
        }, "".concat(d.kind, "-").concat(d.targetKey));
        return null;
    });
}

// EXTERNAL MODULE: ./src/lib/clipboard.ts
var clipboard = __webpack_require__(7202);
;// ./src/components/ui/notification-actions/ApprovalCommand.tsx
// +feature: notification-inline-actions
/* __next_internal_client_entry_do_not_use__ LONG_COMMAND_LINES,LONG_COMMAND_CHARS,isLongCommand,ApprovalCommand auto */ 




/** Past either limit the command moves into a labelled scroll region with a count and Copy. */ const LONG_COMMAND_LINES = 8;
const LONG_COMMAND_CHARS = 400;
function isLongCommand(text) {
    return text.split("\n").length > LONG_COMMAND_LINES || text.length > LONG_COMMAND_CHARS;
}
/** The whole command the operator is about to approve (ux.md 2.4); wrapped, never ellipsized. */ function ApprovalCommand(param) {
    let { tool, text } = param;
    const [copied, setCopied] = (0,react.useState)(false);
    if (!text) return null;
    const long = isLongCommand(text);
    const copyButton = // analytics-exempt
    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
        type: "button",
        className: ActionRow_css/* actionButton */.hP,
        onClick: ()=>{
            void (0,clipboard/* copyToClipboard */.l)(text).then(setCopied);
        },
        children: copied ? notificationCopy/* COPY */.sF.copied : notificationCopy/* COPY */.sF.copy
    });
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: ActionRow_css/* commandRegion */.s8,
        "data-testid": "approval-command",
        children: [
            tool && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                children: tool
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("pre", {
                className: "".concat(ActionRow_css/* commandText */.jt, " ").concat(long ? ActionRow_css/* commandScroll */.h3 : ""),
                // A keyboard user must be able to scroll the region.
                tabIndex: long ? 0 : undefined,
                "aria-label": long ? notificationCopy/* COPY */.sF.commandRegionLabel : undefined,
                role: long ? "region" : undefined,
                "data-testid": "approval-command-text",
                children: text
            }),
            long && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: ActionRow_css/* commandMeta */.Lu,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        "data-testid": "approval-command-count",
                        children: (0,notificationCopy/* characterCount */.Dq)(text.length)
                    }),
                    copyButton
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/guidance/deliveryView.ts
var deliveryView = __webpack_require__(35610);
// EXTERNAL MODULE: ./src/components/guidance/GuidanceDeliveryLine.tsx
var GuidanceDeliveryLine = __webpack_require__(26198);
// EXTERNAL MODULE: ./src/components/guidance/GuidanceQuestionForm.tsx
var GuidanceQuestionForm = __webpack_require__(33378);
;// ./src/components/ui/notification-actions/GuidanceBody.tsx
/* __next_internal_client_entry_do_not_use__ clientDeliveryOf,toFormQuestions,tabbedShape,liveAnswer,GuidanceBody auto */ 
// +feature: session:guidance-structured








const isLive = (d)=>(d === null || d === void 0 ? void 0 : d.kind) === notification_action_pb/* ActionKind */.LN.ANSWER_QUESTION;
const REASON_READ_ONLY = "read_only";
/** ActionResult.StaleReason (server/services/question_answer_sequence.go) when part of a multi-question answer was typed. */ const ANSWER_ABORTED_PARTIAL = "answer_aborted_partial";
/** What this browser knows about the last attempt, in the vocabulary of the delivery line. */ function clientDeliveryOf(state) {
    if (state.status === "pending") return {
        sending: true
    };
    if (state.status === "failed") return {
        clientFailure: true
    };
    if (state.status !== "settled") return {};
    const { outcome, detail, staleReason } = state.response;
    switch(outcome){
        case notification_action_pb/* ActionOutcome */.nd.APPLIED:
            return {
                applied: true
            };
        case notification_action_pb/* ActionOutcome */.nd.NOT_SENT:
            return {
                notSent: detail === "human_active" ? "human_active" : "plain"
            };
        case notification_action_pb/* ActionOutcome */.nd.SEND_INDETERMINATE:
            return {
                indeterminate: true
            };
        case notification_action_pb/* ActionOutcome */.nd.STALE:
            if (staleReason === ANSWER_ABORTED_PARTIAL) return {
                abortedPartial: true
            };
            return {
                staleReason: staleReason === "dialog_replaced" || staleReason === "session_ended" ? staleReason : "dialog_gone"
            };
        case notification_action_pb/* ActionOutcome */.nd.FORBIDDEN:
            return detail === "desktop_only" ? {
                desktopOnly: true
            } : {};
        case notification_action_pb/* ActionOutcome */.nd.ALREADY_RESOLVED:
            return detail ? {
                elsewhere: detail
            } : {};
        default:
            return {};
    }
}
const toFormQuestions = (g)=>{
    var _g_questions;
    return ((_g_questions = g.questions) !== null && _g_questions !== void 0 ? _g_questions : []).map((q)=>({
            text: q.question,
            header: q.header || undefined,
            multiSelect: q.multiSelect,
            options: q.options.map((o)=>({
                    label: o.label,
                    description: o.description || undefined
                }))
        }));
};
const hasOptions = (q)=>q.options.length > 0;
/** A single-select question: one digit answers it. */ const singleDigitShape = (qs)=>qs.length === 1 && !qs[0].multiSelect;
/**
 * The one two-question shape with a recorded key sequence (Spike S1c, mirrors session.MeasuredTabbedShape):
 * a single-select, then a multi-select. It is answered by one Submit carrying every pick.
 */ const tabbedShape = (qs)=>qs.length === 2 && !qs[0].multiSelect && qs[1].multiSelect && qs.every(hasOptions);
/** A keystroke route writes keys: only a single digit or the measured tabbed sequence is safe. */ const keystrokeShape = (qs)=>singleDigitShape(qs) || tabbedShape(qs);
/** The answer of a live route: the label for one digit, else one list of labels per question as JSON (EncodeSequencePicks). */ const liveAnswer = (answers)=>answers.length === 1 ? answers[0].selected[0] : JSON.stringify(answers.map((a)=>a.selected));
/**
 * The body of a question card (design/ux.md 3): the shared form, then the one delivery-state line
 * derived from the durable row and this browser's last attempt. Controls stay mounted while the
 * card flips to a final state, so focus is not moved when a terminal answer lands (UX-AC26).
 */ function GuidanceBody(props) {
    const { rowId, guidance, descriptor, state, disabledReason, minHeight, openControl, invoke, retry, reset, onMarkHandled, onDeliveryRow } = props;
    const { announce } = (0,useAnnounce/* useAnnounce */.XE)();
    const questions = (0,react.useMemo)(()=>toFormQuestions(guidance), [
        guidance
    ]);
    const client = (0,react.useMemo)(()=>clientDeliveryOf(state), [
        state
    ]);
    const view = (0,deliveryView/* deliveryView */.sH)((0,deliveryView/* snapshotOf */.eY)(guidance), client);
    const readOnlyReason = (descriptor === null || descriptor === void 0 ? void 0 : descriptor.unavailableReason) === REASON_READ_ONLY;
    const canAnswer = !!descriptor && (descriptor.available || descriptor.unavailableReason === "desktop_only") && !readOnlyReason && (!isLive(descriptor) || keystrokeShape(questions));
    // Once answerable, a card stays a form (locked) when it flips, so the focused control is not removed.
    const wasAnswerable = (0,react.useRef)(false);
    if (canAnswer) wasAnswerable.current = true;
    const answerable = canAnswer || wasAnswerable.current;
    const desktopOnly = (descriptor === null || descriptor === void 0 ? void 0 : descriptor.unavailableReason) === "desktop_only";
    const prevRow = (0,react.useRef)(undefined);
    (0,react.useEffect)(()=>{
        const text = (0,deliveryView/* announceTransition */.ho)(prevRow.current, view);
        if (text) announce(text, "polite", "delivery-".concat(rowId));
        var _view_row;
        prevRow.current = (_view_row = view === null || view === void 0 ? void 0 : view.row) !== null && _view_row !== void 0 ? _view_row : null;
        onDeliveryRow === null || onDeliveryRow === void 0 ? void 0 : onDeliveryRow(view);
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        view === null || view === void 0 ? void 0 : view.row
    ]);
    const send = (answers)=>{
        if (!descriptor) return;
        const text = isLive(descriptor) ? liveAnswer(answers) : (0,GuidanceQuestionForm/* formatAnswers */.BM)(answers);
        invoke(buildSpec({
            descriptor,
            option: (0,create/* create */.v)(notification_action_pb/* ActionOptionSchema */.lE, {
                label: text
            })
        }));
    };
    const onAction = (action)=>{
        if (action === "retry") retry();
        else if (action === "retryAnyway") {
            if (state.status === "settled") invoke({
                ...state.spec,
                params: {
                    ...state.spec.params,
                    confirm: true
                }
            });
        } else if (action === "markHandled") onMarkHandled === null || onMarkHandled === void 0 ? void 0 : onMarkHandled();
        else {
            reset();
            onMarkHandled === null || onMarkHandled === void 0 ? void 0 : onMarkHandled();
        }
    };
    const line = view && /*#__PURE__*/ (0,jsx_runtime.jsx)(GuidanceDeliveryLine/* GuidanceDeliveryLine */.V, {
        view: view,
        onAction: onAction,
        openControl: openControl,
        minHeight: minHeight,
        busy: state.status === "pending"
    });
    const locked = !answerable || desktopOnly || !!(view === null || view === void 0 ? void 0 : view.locked) || state.status === "pending";
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(GuidanceQuestionForm/* GuidanceQuestionForm */.hI, {
        id: guidance.id || rowId,
        questions: questions,
        mode: answerable ? "answer" : "readonly",
        canTypeText: !!descriptor && !isLive(descriptor),
        unrenderable: questions.length === 0,
        locked: locked,
        disabledReason: disabledReason,
        onSubmit: send,
        openControl: openControl,
        delivery: line !== null && line !== void 0 ? line : undefined,
        minHeight: minHeight
    });
}

;// ./src/components/ui/notification-actions/MoreMenu.tsx
/* __next_internal_client_entry_do_not_use__ MoreMenu auto */ 


/** Desktop-only menu holding Create rule..., kept out of the primary row (ux.md 2.1). */ function MoreMenu(param) {
    let { open, blocked, minHeight, onToggle, onCreateRule, buttonRef, firstItemRef } = param;
    const toggle = // analytics-exempt
    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
        ref: buttonRef,
        type: "button",
        className: ActionRow_css/* actionButton */.hP,
        "data-testid": "action-more",
        style: {
            minHeight
        },
        "aria-haspopup": "menu",
        "aria-expanded": open,
        "aria-disabled": blocked ? true : undefined,
        onClick: ()=>!blocked && onToggle(),
        onKeyDown: (e)=>{
            if (e.key === "ArrowDown" && !blocked && !open) {
                e.preventDefault();
                onToggle();
            }
        },
        children: notificationCopy/* COPY */.sF.more
    });
    const createRule = // analytics-exempt
    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
        ref: firstItemRef,
        type: "button",
        role: "menuitem",
        className: ActionRow_css/* actionButton */.hP,
        "data-testid": "action-create-rule",
        onClick: onCreateRule,
        children: notificationCopy/* COPY */.sF.createRuleMenu
    });
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            toggle,
            open && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                role: "menu",
                className: ActionRow_css/* menu */.MK,
                "aria-label": notificationCopy/* COPY */.sF.more,
                "data-testid": "more-menu",
                children: createRule
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/components/ui/notification-actions/SubSteps.tsx
var SubSteps = __webpack_require__(43943);
;// ./src/components/ui/notification-actions/actionOutcome.ts


const GONE = {
    settled: true,
    retry: "none"
};
const withRemaining = (text, ctx)=>ctx.remaining ? "".concat(text, ". ").concat((0,notificationCopy/* moreWaiting */.l7)(ctx.remaining)) : text;
function applied(ctx) {
    switch(ctx.kind){
        case notification_action_pb/* ActionKind */.LN.APPROVE:
            return {
                ...GONE,
                text: withRemaining(notificationCopy/* COPY */.sF.approved, ctx),
                announce: notificationCopy/* COPY */.sF.approved
            };
        case notification_action_pb/* ActionKind */.LN.DENY:
            return {
                ...GONE,
                text: withRemaining(notificationCopy/* COPY */.sF.denied, ctx),
                announce: notificationCopy/* COPY */.sF.denied
            };
        case notification_action_pb/* ActionKind */.LN.BACKLOG_DECISION:
            {
                var _ctx_chosenLabel;
                const text = (0,notificationCopy/* decided */.rc)((_ctx_chosenLabel = ctx.chosenLabel) !== null && _ctx_chosenLabel !== void 0 ? _ctx_chosenLabel : "");
                return {
                    ...GONE,
                    text,
                    announce: text
                };
            }
        case notification_action_pb/* ActionKind */.LN.ANSWER_QUESTION:
            var _ctx_chosenLabel1;
            // A live-pane answer is only "Sent": "Answered" waits for the agent's acknowledgement.
            return ctx.tier === notification_action_pb/* ActionTier */.hb.B ? {
                ...GONE,
                text: notificationCopy/* COPY */.sF.sentWaiting,
                announce: notificationCopy/* ANNOUNCE */.eV.answerSent
            } : {
                ...GONE,
                text: (0,notificationCopy/* answered */.oQ)((_ctx_chosenLabel1 = ctx.chosenLabel) !== null && _ctx_chosenLabel1 !== void 0 ? _ctx_chosenLabel1 : ""),
                announce: notificationCopy/* ANNOUNCE */.eV.answerSent
            };
        case notification_action_pb/* ActionKind */.LN.ANSWER_GUIDANCE:
            var _ctx_chosenLabel2;
            return {
                ...GONE,
                text: (0,notificationCopy/* answered */.oQ)((_ctx_chosenLabel2 = ctx.chosenLabel) !== null && _ctx_chosenLabel2 !== void 0 ? _ctx_chosenLabel2 : ""),
                announce: notificationCopy/* ANNOUNCE */.eV.answerSent
            };
        case notification_action_pb/* ActionKind */.LN.CANCEL_WAKE:
            return {
                ...GONE,
                text: notificationCopy/* COPY */.sF.promptCancelled,
                announce: notificationCopy/* COPY */.sF.promptCancelled
            };
        case notification_action_pb/* ActionKind */.LN.CANCEL_PEER_HELP:
            return {
                ...GONE,
                text: notificationCopy/* COPY */.sF.requestCancelled,
                announce: notificationCopy/* COPY */.sF.requestCancelled
            };
        default:
            return {
                ...GONE,
                text: notificationCopy/* COPY */.sF.approved,
                announce: notificationCopy/* COPY */.sF.approved
            };
    }
}
function stale(res, ctx) {
    const approvalKind = ctx.kind === notification_action_pb/* ActionKind */.LN.APPROVE || ctx.kind === notification_action_pb/* ActionKind */.LN.DENY;
    if (approvalKind && ctx.ruleName) return {
        ...GONE,
        text: (0,notificationCopy/* handledByRule */.Kx)(ctx.ruleName)
    };
    if (res.staleReason === "dialog_replaced") return {
        ...GONE,
        text: notificationCopy/* COPY */.sF.staleReplaced
    };
    if (res.staleReason === "session_ended") return {
        ...GONE,
        text: notificationCopy/* COPY */.sF.sessionEnded
    };
    return {
        ...GONE,
        text: notificationCopy/* COPY */.sF.staleGone
    };
}
/**
 * Maps a typed outcome to the row's copy and exits (design/ux.md 2.3 and 3.1). The
 * server never reports an expected condition as a Connect error, so this is total
 * over ActionOutcome; anything unknown reads as INVALID.
 */ function outcomeView(res, ctx) {
    switch(res.outcome){
        case notification_action_pb/* ActionOutcome */.nd.APPLIED:
            return applied(ctx);
        case notification_action_pb/* ActionOutcome */.nd.ALREADY_RESOLVED:
            return {
                ...GONE,
                text: (0,notificationCopy/* alreadyHandled */.B3)(res.detail || undefined)
            };
        case notification_action_pb/* ActionOutcome */.nd.STALE:
            return stale(res, ctx);
        case notification_action_pb/* ActionOutcome */.nd.NOT_SENT:
            return {
                settled: false,
                retry: "direct",
                text: res.detail === "human_active" ? notificationCopy/* COPY */.sF.notSentHumanActive : notificationCopy/* COPY */.sF.notSent,
                announce: notificationCopy/* ANNOUNCE */.eV.notDelivered
            };
        case notification_action_pb/* ActionOutcome */.nd.SEND_INDETERMINATE:
            return {
                settled: false,
                retry: "confirm",
                openPrimary: true,
                text: notificationCopy/* COPY */.sF.indeterminate
            };
        case notification_action_pb/* ActionOutcome */.nd.RATE_LIMITED:
            return {
                settled: false,
                retry: "countdown",
                text: "",
                countdownSeconds: Math.max(0, res.retryAfterSeconds)
            };
        case notification_action_pb/* ActionOutcome */.nd.FORBIDDEN:
            return {
                ...GONE,
                text: res.detail === "desktop_only" ? notificationCopy/* COPY */.sF.desktopOnly : notificationCopy/* COPY */.sF.forbidden
            };
        case notification_action_pb/* ActionOutcome */.nd.FLAG_OFF:
            return {
                ...GONE,
                text: notificationCopy/* COPY */.sF.flagOff
            };
        default:
            return {
                settled: false,
                retry: "none",
                refresh: true,
                text: notificationCopy/* COPY */.sF.invalid
            };
    }
}

;// ./src/components/ui/notification-actions/ActionRow.tsx
// +feature: notification-inline-actions
/* __next_internal_client_entry_do_not_use__ TOUCH_TARGET_PX,APPROVE_EXTRA_GAP_PX,ActionRow auto */ 






















/** Seconds left of a RATE_LIMITED hold; ticks once a second and calls `onDone` at zero. */ function useCountdown(seconds, onDone) {
    const [left, setLeft] = (0,react.useState)(seconds !== null && seconds !== void 0 ? seconds : 0);
    const doneRef = (0,react.useRef)(onDone);
    doneRef.current = onDone;
    (0,react.useEffect)(()=>{
        setLeft(seconds !== null && seconds !== void 0 ? seconds : 0);
        if (!seconds) return;
        let remaining = seconds;
        const timer = setInterval(()=>{
            remaining -= 1;
            setLeft(remaining);
            if (remaining <= 0) {
                clearInterval(timer);
                doneRef.current();
            }
        }, 1000);
        return ()=>clearInterval(timer);
    }, [
        seconds
    ]);
    return left;
}
function confirmCopy(p, prNumber) {
    var _p_option;
    const { kind } = p.descriptor;
    if (CANCEL_KINDS.has(kind)) {
        const word = kind === notification_action_pb/* ActionKind */.LN.CANCEL_PEER_HELP ? "request" : "prompt";
        const prompt = word === "request" ? notificationCopy/* COPY */.sF.cancelRequestQuestion : notificationCopy/* COPY */.sF.cancelPromptQuestion;
        return {
            prompt,
            confirmLabel: (0,notificationCopy/* cancelPrompt */.zl)(word),
            backLabel: notificationCopy/* COPY */.sF.keep
        };
    }
    var _p_option_label;
    const label = (_p_option_label = (_p_option = p.option) === null || _p_option === void 0 ? void 0 : _p_option.label) !== null && _p_option_label !== void 0 ? _p_option_label : p.descriptor.label;
    return {
        prompt: isMerge(p) ? (0,notificationCopy/* mergePrompt */.JE)(prNumber) : "".concat(label, "?"),
        confirmLabel: isMerge(p) ? notificationCopy/* COPY */.sF.confirmMerge : "Confirm ".concat(label.toLowerCase()),
        backLabel: notificationCopy/* COPY */.sF.back
    };
}
/**
 * The one inline action UI for a notification (ADR-002): renders only what the server's
 * descriptors allow, runs sub-steps inline (never a modal) and maps each typed outcome to
 * its copy. Every write goes through `dispatch`, the one place a hold before sending
 * (the Story 2.10 commit delay) can wrap.
 */ function ActionRow(props) {
    const { rowId, descriptors, approval, prNumber, openControl, onOpen, onRefresh, onCreateRule, localListener = true, onApplied } = props;
    const { isInnerScreen } = (0,deckViewportContext/* useDeckViewport */.T)();
    const mobile = !isInnerScreen;
    const minHeight = mobile ? TOUCH_TARGET_PX : undefined;
    const { announce } = (0,useAnnounce/* useAnnounce */.XE)();
    const contextReason = (0,OfflineBanner/* useActionsDisabledReason */.lj)();
    var _props_disabledReason;
    const offlineReason = (_props_disabledReason = props.disabledReason) !== null && _props_disabledReason !== void 0 ? _props_disabledReason : contextReason;
    const { state, invoke, retry, reset } = useInvokeNotificationAction();
    const uid = (0,react.useId)();
    const engagement = (0,useEngagedCard/* useCardEngagement */.j9)();
    const tracker = (0,useEngagedCard/* useEngagement */.Tq)();
    const statusRef = (0,react.useRef)(null);
    const focusWasInside = (0,react.useRef)(false);
    const [step, setStep] = (0,react.useState)("row");
    const [pick, setPick] = (0,react.useState)(null);
    const lastCtx = (0,react.useRef)(null);
    const draftKey = "deny:".concat(rowId);
    const [reason, setReason] = (0,react.useState)(()=>(0,sessionDrafts/* readDraft */.kF)(draftKey));
    const actionable = (0,react.useMemo)(()=>descriptors.filter((d)=>KNOWN_KINDS.has(d.kind)), [
        descriptors
    ]);
    const approve = actionable.find((d)=>d.kind === notification_action_pb/* ActionKind */.LN.APPROVE);
    const deny = actionable.find((d)=>d.kind === notification_action_pb/* ActionKind */.LN.DENY);
    const others = actionable.filter((d)=>d !== approve && d !== deny);
    const hasApproval = !!(approve || deny);
    const failClosed = hasApproval && (approval === null || approval === void 0 ? void 0 : approval.unrenderable) === true;
    const pending = state.status === "pending";
    const failed = state.status === "failed";
    const view = (0,react.useMemo)(()=>state.status === "settled" && lastCtx.current ? outcomeView(state.response, lastCtx.current) : null, [
        state
    ]);
    const recovering = failed || !!view && !view.settled && view.retry !== "countdown";
    const countdown = useCountdown((view === null || view === void 0 ? void 0 : view.retry) === "countdown" ? view.countdownSeconds : undefined, reset);
    const commit = (0,react.useCallback)((spec, ctx)=>{
        var _rootRef_current;
        lastCtx.current = ctx;
        focusWasInside.current = !!((_rootRef_current = rootRef.current) === null || _rootRef_current === void 0 ? void 0 : _rootRef_current.contains(document.activeElement));
        setStep("row");
        return invoke(spec);
    }, [
        invoke
    ]);
    // Approve, Send back and Mark done wait behind Undo; leaving cancels and sends nothing.
    const holdMarker = "hold:".concat(rowId);
    const [cancelledKey, setCancelledKey] = (0,react.useState)(()=>(0,sessionDrafts/* readDraft */.kF)(holdMarker) || null);
    const { trackUndo } = useInlineActionTelemetry();
    const delay = useCommitDelay({
        onCommit: (h)=>void commit(h.spec, h.ctx),
        onCancel: (h, reason)=>{
            announce(notificationCopy/* HOLD_COPY */.a6[h.key].undone, "polite", "hold-".concat(rowId));
            if (reason === "undo") trackUndo(h.spec.kind);
            if (reason === "leave") {
                (0,sessionDrafts/* writeDraft */.d$)(holdMarker, h.key);
                setCancelledKey(h.key);
            }
        }
    });
    const holding = delay.held;
    const blocked = pending || countdown > 0 || !!offlineReason || holding !== null;
    (0,react.useEffect)(()=>engagement.setHold(holding !== null), [
        engagement,
        holding
    ]);
    const undoRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        var _undoRef_current;
        if (holding) (_undoRef_current = undoRef.current) === null || _undoRef_current === void 0 ? void 0 : _undoRef_current.focus();
    }, [
        holding
    ]);
    const dispatch = (0,react.useCallback)((spec, ctx)=>{
        const key = holdKeyOf(spec);
        if (!key) return commit(spec, ctx);
        (0,sessionDrafts/* clearDraft */.nr)(holdMarker);
        setCancelledKey(null);
        setStep("row");
        announce(notificationCopy/* HOLD_COPY */.a6[key].pending, "polite", "hold-".concat(rowId));
        delay.start({
            spec,
            ctx,
            key
        });
        return Promise.resolve(null);
    }, [
        commit,
        delay,
        holdMarker,
        announce,
        rowId
    ]);
    // Sub-steps and drafts keep the card engaged; a final state releases it (design/ux.md 7.1).
    const draftOpen = reason.trim() !== "";
    (0,react.useEffect)(()=>{
        engagement.setSubStep(step !== "row" || draftOpen);
        return ()=>engagement.setSubStep(false);
    }, [
        engagement,
        step,
        draftOpen
    ]);
    const elsewhere = (view === null || view === void 0 ? void 0 : view.settled) ? undefined : props.elsewhere;
    const finalState = !!(view === null || view === void 0 ? void 0 : view.settled) || elsewhere !== undefined;
    (0,react.useEffect)(()=>{
        engagement.setResolved(finalState);
        return ()=>engagement.setResolved(false);
    }, [
        engagement,
        finalState
    ]);
    // The pressed button leaves with the row's controls; keep focus in the card on the result line.
    (0,react.useEffect)(()=>{
        if (finalState && focusWasInside.current) {
            focusWasInside.current = false;
            tracker === null || tracker === void 0 ? void 0 : tracker.focusProgrammatically(statusRef.current);
        }
    }, [
        finalState,
        tracker
    ]);
    // One short result per settled outcome, through the panel's single polite region.
    (0,react.useEffect)(()=>{
        var _lastCtx_current;
        if (state.status !== "settled" || !view) return;
        // A question card announces its own delivery transitions (GuidanceBody), once each.
        if (view.announce && !props.guidance) announce(view.announce, "polite", "action-".concat(rowId));
        var _lastCtx_current_chosenLabel;
        if (state.response.outcome === notification_action_pb/* ActionOutcome */.nd.APPLIED) onApplied === null || onApplied === void 0 ? void 0 : onApplied(state.spec.kind, (_lastCtx_current_chosenLabel = (_lastCtx_current = lastCtx.current) === null || _lastCtx_current === void 0 ? void 0 : _lastCtx_current.chosenLabel) !== null && _lastCtx_current_chosenLabel !== void 0 ? _lastCtx_current_chosenLabel : view.text);
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        state
    ]);
    const run = (p, opts)=>{
        var _p_option;
        return dispatch(buildSpec(p, opts), {
            kind: p.descriptor.kind,
            tier: p.descriptor.tier,
            chosenLabel: (_p_option = p.option) === null || _p_option === void 0 ? void 0 : _p_option.label,
            ruleName: approval === null || approval === void 0 ? void 0 : approval.ruleName,
            remaining: props.remaining
        });
    };
    const onPress = (p)=>{
        if (p.descriptor.kind === notification_action_pb/* ActionKind */.LN.DENY) {
            setPick(p);
            setStep("deny");
        } else if (needsConfirm(p)) {
            setPick(p);
            setStep("confirm");
        } else {
            void run(p);
        }
    };
    // Focus follows the operator's own sub-step; Back returns it to the control that opened it.
    const fieldRef = (0,react.useRef)(null);
    const confirmRef = (0,react.useRef)(null);
    const moreRef = (0,react.useRef)(null);
    const firstMenuItemRef = (0,react.useRef)(null);
    const rootRef = (0,react.useRef)(null);
    // The opener may unmount when the sub-step replaces the row, so remember it by test id.
    const opener = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        var _fieldRef_current, _firstMenuItemRef_current, _confirmRef_current;
        if (step === "deny") (_fieldRef_current = fieldRef.current) === null || _fieldRef_current === void 0 ? void 0 : _fieldRef_current.focus();
        else if (step === "more") (_firstMenuItemRef_current = firstMenuItemRef.current) === null || _firstMenuItemRef_current === void 0 ? void 0 : _firstMenuItemRef_current.focus();
        else if (step !== "row") (_confirmRef_current = confirmRef.current) === null || _confirmRef_current === void 0 ? void 0 : _confirmRef_current.focus();
    }, [
        step
    ]);
    const rememberOpener = ()=>{
        var _document_activeElement;
        var _document_activeElement_getAttribute;
        opener.current = (_document_activeElement_getAttribute = (_document_activeElement = document.activeElement) === null || _document_activeElement === void 0 ? void 0 : _document_activeElement.getAttribute("data-testid")) !== null && _document_activeElement_getAttribute !== void 0 ? _document_activeElement_getAttribute : null;
    };
    const open = (next)=>{
        rememberOpener();
        setStep(next);
    };
    const back = ()=>{
        const id = opener.current;
        setStep("row");
        requestAnimationFrame(()=>{
            var _rootRef_current_querySelector, _rootRef_current;
            if (id) (_rootRef_current = rootRef.current) === null || _rootRef_current === void 0 ? void 0 : (_rootRef_current_querySelector = _rootRef_current.querySelector('[data-testid="'.concat(id, '"]'))) === null || _rootRef_current_querySelector === void 0 ? void 0 : _rootRef_current_querySelector.focus();
        });
    };
    const onKeyDown = (e)=>{
        if (e.key !== "Escape" || step === "row" || e.nativeEvent.isComposing) return;
        // The innermost layer closes first; the panel's own Esc handler honours defaultPrevented.
        e.preventDefault();
        e.stopPropagation();
        back();
    };
    const reasonFor = (d)=>!d.available ? "".concat(uid, "-").concat(kindSlug(d.kind)) : offlineReason ? "".concat(uid, "-offline") : undefined;
    const ui = {
        blocked,
        mobile,
        minHeight,
        pendingKind: state.status === "pending" ? state.spec.kind : undefined,
        holding: holding ? {
            key: holding.key,
            text: (0,notificationCopy/* holdCountdown */.px)(notificationCopy/* HOLD_COPY */.a6[holding.key].verbing, delay.secondsLeft)
        } : undefined,
        reasonFor,
        onPress: (p)=>{
            rememberOpener();
            onPress(p);
        }
    };
    const openButton = openControl !== undefined ? openControl : /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionButton/* ActionButton */.r, {
        id: "open",
        label: notificationCopy/* COPY */.sF.openSession,
        variant: ActionRow_css/* secondaryVariant */.OL,
        minHeight: minHeight,
        onClick: ()=>onOpen === null || onOpen === void 0 ? void 0 : onOpen()
    });
    const statusLine = (text)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
            ref: statusRef,
            tabIndex: -1,
            className: ActionRow_css/* status */.h5,
            "data-testid": "action-status",
            children: text
        });
    const subStep = ()=>{
        const common = {
            onBack: back,
            minHeight,
            busy: pending
        };
        if (step === "deny" && pick) {
            return /*#__PURE__*/ (0,jsx_runtime.jsx)(SubSteps/* DenyStep */.JQ, {
                ...common,
                fieldId: "".concat(uid, "-reason"),
                fieldRef: fieldRef,
                reason: reason,
                onReason: (v)=>{
                    setReason(v);
                    (0,sessionDrafts/* writeDraft */.d$)(draftKey, v);
                },
                onSend: ()=>{
                    (0,sessionDrafts/* clearDraft */.nr)(draftKey);
                    void run(pick, {
                        reason: reason.trim() || undefined
                    });
                }
            });
        }
        if (step === "confirm" && pick) {
            return /*#__PURE__*/ (0,jsx_runtime.jsx)(SubSteps/* ConfirmStep */.Hb, {
                ...common,
                ...confirmCopy(pick, prNumber),
                confirmRef: confirmRef,
                onConfirm: ()=>void run(pick, {
                        confirm: true
                    })
            });
        }
        if (step === "retry-anyway" && state.status === "settled" && lastCtx.current) {
            const { spec } = state;
            const ctx = lastCtx.current;
            return /*#__PURE__*/ (0,jsx_runtime.jsx)(SubSteps/* ConfirmStep */.Hb, {
                ...common,
                prompt: notificationCopy/* COPY */.sF.retryAnywayPrompt,
                confirmLabel: notificationCopy/* COPY */.sF.retryAnywayConfirm,
                backLabel: notificationCopy/* COPY */.sF.back,
                confirmRef: confirmRef,
                confirmId: "retry-confirm",
                testId: "retry-anyway-step",
                onConfirm: ()=>void dispatch({
                        ...spec,
                        params: {
                            ...spec.params,
                            confirm: true
                        }
                    }, ctx)
            });
        }
        if (step === "create-rule") {
            var _approval_ruleScope, _ref;
            return /*#__PURE__*/ (0,jsx_runtime.jsx)(SubSteps/* ConfirmStep */.Hb, {
                ...common,
                ...(0,SubSteps/* createRuleStepProps */.py)((_ref = (_approval_ruleScope = approval === null || approval === void 0 ? void 0 : approval.ruleScope) !== null && _approval_ruleScope !== void 0 ? _approval_ruleScope : approval === null || approval === void 0 ? void 0 : approval.tool) !== null && _ref !== void 0 ? _ref : "this tool"),
                backLabel: notificationCopy/* COPY */.sF.back,
                confirmRef: confirmRef,
                onConfirm: ()=>{
                    setStep("row");
                    onCreateRule === null || onCreateRule === void 0 ? void 0 : onCreateRule();
                }
            });
        }
        return null;
    };
    const showMore = hasApproval && !failClosed && !!onCreateRule && isInnerScreen && localListener;
    const sub = subStep();
    let body;
    const answerDescriptor = actionable.find((d)=>d.kind === notification_action_pb/* ActionKind */.LN.ANSWER_QUESTION || d.kind === notification_action_pb/* ActionKind */.LN.ANSWER_GUIDANCE);
    if (props.guidance) {
        body = /*#__PURE__*/ (0,jsx_runtime.jsx)(GuidanceBody, {
            rowId: rowId,
            guidance: props.guidance,
            descriptor: answerDescriptor,
            state: state,
            disabledReason: offlineReason,
            minHeight: minHeight,
            openControl: openButton,
            invoke: (spec)=>{
                var _spec_params;
                var _answerDescriptor_tier;
                return void commit(spec, {
                    kind: spec.kind,
                    tier: (_answerDescriptor_tier = answerDescriptor === null || answerDescriptor === void 0 ? void 0 : answerDescriptor.tier) !== null && _answerDescriptor_tier !== void 0 ? _answerDescriptor_tier : notification_action_pb/* ActionTier */.hb.A,
                    chosenLabel: (_spec_params = spec.params) === null || _spec_params === void 0 ? void 0 : _spec_params.answer
                });
            },
            retry: ()=>void retry(),
            reset: reset,
            onMarkHandled: props.onMarkHandled,
            onDeliveryRow: props.onDeliveryRow
        });
    } else if (elsewhere !== undefined) {
        body = /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
            children: [
                statusLine((0,notificationCopy/* answeredElsewhere */.ul)(elsewhere)),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: ActionRow_css/* buttons */.Uo,
                    children: openButton
                })
            ]
        });
    } else if (view === null || view === void 0 ? void 0 : view.settled) {
        body = /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
            children: [
                statusLine(view.text),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: ActionRow_css/* buttons */.Uo,
                    children: openButton
                })
            ]
        });
    } else if (recovering) {
        const retryHere = failed || (view === null || view === void 0 ? void 0 : view.retry) === "direct";
        var _view_text;
        body = /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
            children: [
                statusLine(failed ? notificationCopy/* COPY */.sF.transient : (_view_text = view === null || view === void 0 ? void 0 : view.text) !== null && _view_text !== void 0 ? _view_text : ""),
                sub !== null && sub !== void 0 ? sub : /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: ActionRow_css/* buttons */.Uo,
                    children: [
                        (view === null || view === void 0 ? void 0 : view.openPrimary) && openButton,
                        retryHere && /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionButton/* ActionButton */.r, {
                            id: "retry",
                            label: notificationCopy/* COPY */.sF.retry,
                            minHeight: minHeight,
                            disabled: pending,
                            onClick: ()=>void retry()
                        }),
                        (view === null || view === void 0 ? void 0 : view.retry) === "confirm" && /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionButton/* ActionButton */.r, {
                            id: "retry-anyway",
                            label: notificationCopy/* COPY */.sF.retryAnyway,
                            minHeight: minHeight,
                            onClick: ()=>open("retry-anyway")
                        }),
                        (view === null || view === void 0 ? void 0 : view.refresh) && /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionButton/* ActionButton */.r, {
                            id: "refresh",
                            label: notificationCopy/* COPY */.sF.refresh,
                            minHeight: minHeight,
                            onClick: ()=>{
                                reset();
                                onRefresh === null || onRefresh === void 0 ? void 0 : onRefresh();
                            }
                        }),
                        failed && /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionButton/* ActionButton */.r, {
                            id: "dismiss",
                            label: notificationCopy/* COPY */.sF.dismiss,
                            minHeight: minHeight,
                            onClick: reset
                        }),
                        !(view === null || view === void 0 ? void 0 : view.openPrimary) && openButton
                    ]
                })
            ]
        });
    } else {
        const showRow = step === "row" || step === "more";
        body = /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
            children: [
                cancelledKey && !holding && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: ActionRow_css/* status */.h5,
                    "data-testid": "hold-cancelled",
                    children: notificationCopy/* HOLD_COPY */.a6[cancelledKey].cancelled
                }),
                hasApproval && !failClosed && /*#__PURE__*/ (0,jsx_runtime.jsx)(ApprovalCommand, {
                    tool: approval === null || approval === void 0 ? void 0 : approval.tool,
                    text: approval === null || approval === void 0 ? void 0 : approval.text
                }),
                failClosed && /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                    className: ActionRow_css/* status */.h5,
                    "data-testid": "approval-unrenderable",
                    children: [
                        (approval === null || approval === void 0 ? void 0 : approval.tool) ? "".concat(approval.tool, ". ") : "",
                        notificationCopy/* COPY */.sF.cantShowCommand
                    ]
                }),
                showRow ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: ActionRow_css/* buttons */.Uo,
                    role: "group",
                    "aria-label": "Actions",
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)(ApprovalButtons, {
                            approve: failClosed ? undefined : approve,
                            deny: deny,
                            ui: ui
                        }),
                        showMore && /*#__PURE__*/ (0,jsx_runtime.jsx)(MoreMenu, {
                            open: step === "more",
                            blocked: blocked,
                            minHeight: minHeight,
                            buttonRef: moreRef,
                            firstItemRef: firstMenuItemRef,
                            onToggle: ()=>step === "more" ? back() : open("more"),
                            onCreateRule: ()=>setStep("create-rule")
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)(OtherButtons, {
                            descriptors: others,
                            ui: ui
                        }),
                        holding && /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionButton/* ActionButton */.r, {
                            id: "undo",
                            label: notificationCopy/* UNDO_LABEL */.CT,
                            buttonRef: undoRef,
                            minHeight: TOUCH_TARGET_PX,
                            onClick: delay.undo
                        }),
                        openButton
                    ]
                }) : sub,
                countdown > 0 && statusLine((0,notificationCopy/* rateLimited */.xd)(countdown)),
                pending && /*#__PURE__*/ (0,jsx_runtime.jsx)(OfflineBanner/* PendingNotice */.op, {
                    pending: true,
                    onRetry: ()=>void retry()
                })
            ]
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        ref: rootRef,
        className: ActionRow_css/* root */.zr,
        "data-testid": "action-row",
        "aria-busy": pending ? true : undefined,
        onKeyDown: onKeyDown,
        children: [
            body,
            offlineReason && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                id: "".concat(uid, "-offline"),
                className: ActionRow_css/* status */.h5,
                "data-testid": "row-offline-reason",
                children: offlineReason
            }),
            actionable.filter((d)=>!d.available).map((d)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    id: "".concat(uid, "-").concat(kindSlug(d.kind)),
                    className: ActionRow_css/* status */.h5,
                    "data-testid": "action-unavailable-reason",
                    children: unavailableReason(d)
                }, "".concat(d.kind, "-").concat(d.targetKey)))
        ]
    });
}

// EXTERNAL MODULE: ./src/components/ui/notification-actions/inlineActions.ts
var inlineActions = __webpack_require__(57642);
// EXTERNAL MODULE: ./src/lib/reply/replyQuestion.ts
var replyQuestion = __webpack_require__(15203);
// EXTERNAL MODULE: ./src/components/ui/BackgroundChip.tsx + 1 modules
var BackgroundChip = __webpack_require__(36475);
// EXTERNAL MODULE: ./src/lib/utils/notificationGrouping.ts
var notificationGrouping = __webpack_require__(65232);
// EXTERNAL MODULE: ./src/lib/utils/datetime.ts
var datetime = __webpack_require__(3335);
// EXTERNAL MODULE: ./src/styles/a11y.css.ts
var a11y_css = __webpack_require__(26946);
// EXTERNAL MODULE: ./src/lib/utils/notificationMapping.ts
var notificationMapping = __webpack_require__(71249);
// EXTERNAL MODULE: ./src/components/ui/NotificationPanel.css.ts
var NotificationPanel_css = __webpack_require__(82254);
;// ./src/components/ui/NotificationItem.tsx
/* __next_internal_client_entry_do_not_use__ NotificationItem,AutoHandledSection,NeedsDecisionSection auto */ 



















const GENERIC_TITLES = new Set([
    "Claude Notification",
    "Notification",
    "Alert",
    "claude notification"
]);
function getContextString(notification) {
    const projectName = notification.sourceProject;
    const workingDirName = notification.sourceWorkingDir ? notification.sourceWorkingDir.split("/").pop() : null;
    const contextName = projectName || workingDirName;
    const parts = [];
    if (contextName) parts.push(contextName);
    if (notification.sourceApp) parts.push("via ".concat(notification.sourceApp));
    return parts.join(" ");
}
/** Delivery rows after which the card has left Needs you: acknowledged, handled in the terminal, replaced, session ended. */ const RETIRING_ROWS = new Set([
    4,
    11,
    12,
    17
]);
/** Delivery rows 8 and 9: the answer was stored for a stopped or archived session. */ const SAVED_ROWS = new Set([
    8,
    9
]);
const defaultSessionHref = (sessionId)=>"/?session=".concat(encodeURIComponent(sessionId));
// notification=<id> lets the deleted-session card show this record's own text (Task 5.3e).
const hiddenSessionHref = (sessionId, notificationId)=>"/?session=".concat(encodeURIComponent(sessionId), "&tab=terminal&notification=").concat(encodeURIComponent(notificationId));
/**
 * Renders a single notification card: type label, count badge, remove button,
 * subtitle/context/message, approval-needed metadata, and the footer action
 * buttons (approve/deny — including the CI-blocked "Approve anyway" variant,
 * focus-window, and View in Backlog / View Session links).
 *
 * Shared between NotificationsPage (full history) and NotificationPanel
 * (compact dropdown) — see NotificationsPage.test.tsx for the session-link
 * fallback behavior `getSessionHref` exists to preserve.
 */ function NotificationItem(param) {
    let { group, resolvedApprovals, pendingApprovals, blockedApprovals, failedApprovals, resolveApproval, removeFromHistory, handleNotificationClick, getSessionHref = defaultSessionHref, onNavigate, offlineReason, lookupHidden = true, onRefreshActions, onActionApplied, answeredElsewhere, onCreateRule } = param;
    var _notification_metadata, _notification_metadata1, _guidanceRow_questions, _notification_metadata2, _notification_metadata3, _replyAvailability, _notification_metadata4, _notification_metadata5;
    const notification = group.notification;
    const contextString = getContextString(notification);
    const hasSourceApp = notification.sourceApp || notification.sourceBundleId;
    // A hidden session opens read-only (Story 5.3): chip and "View output" instead of "View Session".
    const sessionHidden = (0,useSessionHidden/* useSessionHidden */.o)(notification.sessionId, lookupHidden) === true;
    const replyEnabled = (0,FeatureFlagsContext/* useFeatureFlag */.u)("hidden_session_reply");
    // Always show the session name as the primary title so users know which
    // session generated the notification. If the stored title is a generic
    // placeholder (e.g. "Claude Notification") or absent, fall back to the
    // session name; otherwise show the specific title.
    const primaryTitle = notification.sessionName || (notification.title && !GENERIC_TITLES.has(notification.title) ? notification.title : null) || notification.sessionId || "Notification";
    const subtitleText = notification.title && !GENERIC_TITLES.has(notification.title) && notification.title !== primaryTitle ? notification.title : null;
    const navigate = (ids, onView, sessionId)=>{
        handleNotificationClick(ids, onView, sessionId);
        onNavigate === null || onNavigate === void 0 ? void 0 : onNavigate();
    };
    var _notification_actions;
    // Descriptor-driven actions (ADR-002) replace the legacy buttons when the flag is on and the
    // server sent something beyond Open; a resolved or CI-blocked approval keeps the legacy badge.
    const descriptors = (_notification_actions = notification.actions) !== null && _notification_actions !== void 0 ? _notification_actions : [];
    const approvalKey = notification.notificationType === "approval_needed" ? (_notification_metadata = notification.metadata) === null || _notification_metadata === void 0 ? void 0 : _notification_metadata.approval_id : undefined;
    const legacyOnly = !!approvalKey && !!(resolvedApprovals[approvalKey] || blockedApprovals[approvalKey]);
    const hasInlineActions = descriptors.some((d)=>d.kind !== notification_action_pb/* ActionKind */.LN.OPEN);
    // A question mirrored to a durable row renders as the shared question card (render stage of the structured flag).
    const guidanceId = (_notification_metadata1 = notification.metadata) === null || _notification_metadata1 === void 0 ? void 0 : _notification_metadata1["guidance_request_id"];
    const structuredOn = (0,useFeatureFlagValue/* useFeatureFlagValue */.Rw)(inlineActions/* STRUCTURED_QUESTION_FLAG */.O9, inlineActions/* STRUCTURED_RENDER_SCOPE */.iw, false, !!guidanceId && !legacyOnly);
    const guidanceRow = useGuidanceRow(structuredOn ? guidanceId : undefined, lookupHidden && !notification.isRead);
    var _guidanceRow_questions_length;
    const guidance = guidanceRow && ((_guidanceRow_questions_length = (_guidanceRow_questions = guidanceRow.questions) === null || _guidanceRow_questions === void 0 ? void 0 : _guidanceRow_questions.length) !== null && _guidanceRow_questions_length !== void 0 ? _guidanceRow_questions_length : 0) > 0 ? guidanceRow : undefined;
    const inlineFlag = (0,useFeatureFlagValue/* useFeatureFlagValue */.Rw)(inlineActions/* INLINE_ACTIONS_FLAG */.j_, undefined, false, (hasInlineActions || !!guidance) && !legacyOnly);
    const rowOn = inlineFlag && (hasInlineActions || !!guidance) && !legacyOnly;
    const { markAnswerHandled } = (0,notificationContexts/* useNotificationCommands */.RK)();
    // The server reopens an Unconfirmed answer and retires a settled one; history is not pushed for either.
    const lastRow = (0,react.useRef)(undefined);
    const onDeliveryRow = (0,react.useCallback)((view)=>{
        if (!view || view.row === lastRow.current) return;
        lastRow.current = view.row;
        if (SAVED_ROWS.has(view.row)) window.dispatchEvent(new Event(useSavedAnswers/* SAVED_ANSWERS_CHANGED */.H0));
        const reopened = view.needsYou && notification.isRead;
        const retired = !view.needsYou && RETIRING_ROWS.has(view.row) && !notification.isRead;
        if (reopened || retired) onRefreshActions === null || onRefreshActions === void 0 ? void 0 : onRefreshActions();
    }, [
        notification.isRead,
        onRefreshActions
    ]);
    // One history row can stand for several pending approvals of a session (the "x20" row). Deciding it
    // decides one; the rest stay pending, so the row must not be marked read here. The server hands it on
    // to the next approval and the history refresh brings that one back.
    const stillPending = approvalKey && group.allIds.length === 1 ? Math.max(group.count - 1, 0) : 0;
    const decidedIds = stillPending > 0 ? [] : group.allIds;
    const approval = rowOn && approvalKey ? (0,inlineActions/* approvalDisplay */.cp)(notification) : undefined;
    const focusControl = hasSourceApp && notification.onFocusWindow && !sessionHidden && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
        className: NotificationPanel_css/* focusButton */.Np,
        onClick: notification.onFocusWindow,
        title: "Focus the source application window",
        children: "\uD83D\uDD17 Focus"
    });
    const openLinks = /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            ((_notification_metadata2 = notification.metadata) === null || _notification_metadata2 === void 0 ? void 0 : _notification_metadata2["item_id"]) && /*#__PURE__*/ (0,jsx_runtime.jsx)(nav/* Link */.N_, {
                href: "/backlog?item=".concat(encodeURIComponent(notification.metadata["item_id"])),
                className: NotificationPanel_css/* viewButton */.qM,
                onClick: ()=>navigate(group.allIds, undefined, notification.sessionId),
                "data-testid": "notification-view-backlog",
                children: "View in Backlog"
            }),
            !((_notification_metadata3 = notification.metadata) === null || _notification_metadata3 === void 0 ? void 0 : _notification_metadata3["item_id"]) && notification.sessionId && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(nav/* Link */.N_, {
                        href: sessionHidden ? hiddenSessionHref(notification.sessionId, notification.id) : getSessionHref(notification.sessionId),
                        className: NotificationPanel_css/* viewButton */.qM,
                        onClick: ()=>navigate(group.allIds, notification.onView, notification.sessionId),
                        "data-testid": sessionHidden ? "notification-view-output" : "notification-view-session",
                        children: sessionHidden ? "View output" : "View Session"
                    }),
                    sessionHidden && replyEnabled && ((_replyAvailability = (0,replyQuestion/* replyAvailability */.wR)(notification)) === null || _replyAvailability === void 0 ? void 0 : _replyAvailability.kind) === "replyable" && /*#__PURE__*/ (0,jsx_runtime.jsx)(nav/* Link */.N_, {
                        href: "".concat(hiddenSessionHref(notification.sessionId, notification.id), "&reply=1"),
                        className: NotificationPanel_css/* viewButton */.qM,
                        onClick: ()=>navigate(group.allIds, notification.onView, notification.sessionId),
                        "data-testid": "notification-reply",
                        children: "Reply"
                    })
                ]
            })
        ]
    });
    const legacyActions = /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            notification.notificationType === "approval_needed" && ((_notification_metadata4 = notification.metadata) === null || _notification_metadata4 === void 0 ? void 0 : _notification_metadata4.approval_id) && (()=>{
                const approvalId = notification.metadata.approval_id;
                const resolved = resolvedApprovals[approvalId];
                const isPending = !!pendingApprovals[approvalId];
                const blockedMessage = blockedApprovals[approvalId];
                const failedMessage = failedApprovals[approvalId];
                if (resolved === "allow" || resolved === "deny") {
                    if ((0,notificationMapping/* isReconciledNotification */.yL)(notification)) {
                        var _notification_metadata;
                        var _notification_metadata_classifier_rule_name;
                        const ruleName = (_notification_metadata_classifier_rule_name = (_notification_metadata = notification.metadata) === null || _notification_metadata === void 0 ? void 0 : _notification_metadata["classifier_rule_name"]) !== null && _notification_metadata_classifier_rule_name !== void 0 ? _notification_metadata_classifier_rule_name : "a rule";
                        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                            className: NotificationPanel_css/* resolvedBadge */.uw,
                            "data-decision": "".concat(resolved, "-reconciled"),
                            title: "Re-evaluated with current context (e.g. CI status, session idle time) at resolution time, which may have changed since this was first escalated.",
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    "aria-hidden": "true",
                                    children: resolved === "allow" ? "✓" : "✗"
                                }),
                                " Auto-resolved by rule: ",
                                ruleName
                            ]
                        });
                    }
                    return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: NotificationPanel_css/* resolvedBadge */.uw,
                        "data-decision": resolved,
                        children: resolved === "allow" ? "✓ Approved" : "✗ Denied"
                    });
                }
                if (resolved === "expired") return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    className: NotificationPanel_css/* resolvedBadge */.uw,
                    "data-decision": "expired",
                    children: "Expired"
                });
                if (blockedMessage) {
                    // AC5/Story 2.2.4: visible inline explanation (not a silent no-op or
                    // disabled button) plus an audited "Approve anyway" override.
                    const { text: blockedText, checksUrl } = (0,notificationMapping/* splitCIBlockMessage */.rK)(blockedMessage);
                    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: NotificationPanel_css/* ciBlockedRow */.TZ,
                        "data-testid": "ci-block-message",
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: NotificationPanel_css/* ciBlockedText */.mU,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        "aria-hidden": "true",
                                        children: "⚠️ "
                                    }),
                                    blockedText
                                ]
                            }),
                            checksUrl && /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                                href: checksUrl,
                                target: "_blank",
                                rel: "noopener noreferrer",
                                className: NotificationPanel_css/* ciBlockedLink */.Xi,
                                "data-testid": "ci-block-view-run-link",
                                children: "View CI run"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: NotificationPanel_css/* itemActions */.TN,
                                children: [
                                    checksUrl && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        className: NotificationPanel_css/* approveButton */.Wz,
                                        onClick: ()=>resolveApproval(approvalId, "allow", decidedIds, true),
                                        disabled: isPending || !!offlineReason,
                                        title: offlineReason !== null && offlineReason !== void 0 ? offlineReason : "Approve despite failing CI",
                                        children: isPending ? "…" : "Approve anyway"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        className: NotificationPanel_css/* denyButton */.$O,
                                        onClick: ()=>resolveApproval(approvalId, "deny", decidedIds),
                                        disabled: isPending || !!offlineReason,
                                        title: offlineReason !== null && offlineReason !== void 0 ? offlineReason : "Deny this tool use",
                                        children: isPending ? "…" : "✗ Deny"
                                    })
                                ]
                            })
                        ]
                    });
                }
                return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                    children: [
                        offlineReason && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: NotificationPanel_css/* ciBlockedText */.mU,
                            "data-testid": "row-offline-reason",
                            children: offlineReason
                        }),
                        failedMessage && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: NotificationPanel_css/* ciBlockedText */.mU,
                            "data-testid": "approval-retry-message",
                            children: failedMessage
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: NotificationPanel_css/* approveButton */.Wz,
                            onClick: ()=>resolveApproval(approvalId, "allow", decidedIds),
                            disabled: isPending || !!offlineReason,
                            title: offlineReason !== null && offlineReason !== void 0 ? offlineReason : "Approve this tool use",
                            children: isPending ? "…" : "✓ Approve"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: NotificationPanel_css/* denyButton */.$O,
                            onClick: ()=>resolveApproval(approvalId, "deny", decidedIds),
                            disabled: isPending || !!offlineReason,
                            title: offlineReason !== null && offlineReason !== void 0 ? offlineReason : "Deny this tool use",
                            children: isPending ? "…" : "✗ Deny"
                        })
                    ]
                });
            })(),
            focusControl,
            openLinks
        ]
    });
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: "".concat(NotificationPanel_css/* item */.AS, " ").concat(notification.isRead ? NotificationPanel_css/* read */.LF : NotificationPanel_css/* unread */.gy),
        style: {
            "--priority-color": (0,notificationMapping/* priorityColor */.yA)(notification.priority)
        },
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: NotificationPanel_css/* itemHeader */.Li,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: NotificationPanel_css/* itemTitle */.wp,
                        children: [
                            !notification.isRead && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: NotificationPanel_css/* unreadDot */.Zm,
                                role: "img",
                                "aria-label": "Unread"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: NotificationPanel_css/* typeIcon */.A1,
                                "aria-hidden": "true",
                                children: (0,notificationMapping/* notificationTypeIcon */.Ds)(notification.notificationType)
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("strong", {
                                children: primaryTitle
                            }),
                            sessionHidden && /*#__PURE__*/ (0,jsx_runtime.jsx)(BackgroundChip/* BackgroundChip */.l, {}),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: NotificationPanel_css/* typeLabel */.Dd,
                                style: {
                                    backgroundColor: (0,notificationMapping/* priorityColor */.yA)(notification.priority)
                                },
                                children: (0,notificationMapping/* notificationTypeLabel */.MD)(notification.notificationType)
                            }),
                            group.count > 1 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: NotificationPanel_css/* countBadge */.z2,
                                "aria-label": "".concat(group.count, " occurrences"),
                                children: [
                                    "x",
                                    group.count
                                ]
                            })
                        ]
                    }),
                    removeFromHistory && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: NotificationPanel_css/* removeButton */.DT,
                        onClick: ()=>!offlineReason && removeFromHistory(notification.id),
                        "aria-label": "Remove notification",
                        "aria-disabled": offlineReason ? true : undefined,
                        title: offlineReason,
                        children: "✕"
                    })
                ]
            }),
            subtitleText && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: NotificationPanel_css/* itemSubtitle */.EC,
                children: subtitleText
            }),
            contextString && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: NotificationPanel_css/* itemContext */.Nc,
                children: contextString
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: NotificationPanel_css/* itemMessage */.hm,
                children: notification.message
            }),
            notification.notificationType === "approval_needed" && notification.metadata && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: NotificationPanel_css/* approvalDetails */.MT,
                children: [
                    !rowOn && notification.metadata.tool_name && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: NotificationPanel_css/* approvalTool */.ie,
                        children: [
                            "\uD83D\uDD27 ",
                            notification.metadata.tool_name
                        ]
                    }),
                    !rowOn && notification.metadata.tool_input_command && /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                        className: NotificationPanel_css/* approvalCommand */.jY,
                        children: notification.metadata.tool_input_command
                    }),
                    !rowOn && notification.metadata.tool_input_file && !notification.metadata.tool_input_command && /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                        className: NotificationPanel_css/* approvalCommand */.jY,
                        children: notification.metadata.tool_input_file
                    }),
                    notification.metadata.cwd && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: NotificationPanel_css/* approvalCwd */.ig,
                        title: notification.metadata.cwd,
                        children: [
                            "\uD83D\uDCC1 ",
                            notification.metadata.cwd.split("/").slice(-2).join("/")
                        ]
                    })
                ]
            }),
            notification.sourceWorkingDir && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: NotificationPanel_css/* itemWorkingDir */.e,
                title: notification.sourceWorkingDir,
                children: [
                    "\uD83D\uDCC1 ",
                    notification.sourceWorkingDir.split("/").slice(-2).join("/")
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: NotificationPanel_css/* itemFooter */.$T,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: NotificationPanel_css/* timestamp */.vE,
                        children: (0,datetime/* formatRelativeTime */.fw)(notification.timestamp)
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: NotificationPanel_css/* itemActions */.TN,
                        children: rowOn ? focusControl : legacyActions
                    })
                ]
            }),
            rowOn && /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionRow, {
                rowId: notification.id,
                descriptors: descriptors,
                approval: approval,
                prNumber: (_notification_metadata5 = notification.metadata) === null || _notification_metadata5 === void 0 ? void 0 : _notification_metadata5["pr_number"],
                openControl: openLinks,
                onRefresh: onRefreshActions,
                onApplied: (kind, summary)=>{
                    if (stillPending > 0 && (kind === notification_action_pb/* ActionKind */.LN.APPROVE || kind === notification_action_pb/* ActionKind */.LN.DENY)) onRefreshActions === null || onRefreshActions === void 0 ? void 0 : onRefreshActions();
                    else onActionApplied === null || onActionApplied === void 0 ? void 0 : onActionApplied(group.allIds, summary);
                },
                remaining: stillPending,
                guidance: guidance,
                onMarkHandled: ()=>markAnswerHandled(group.allIds),
                onDeliveryRow: onDeliveryRow,
                elsewhere: group.allIds.map((id)=>answeredElsewhere === null || answeredElsewhere === void 0 ? void 0 : answeredElsewhere[id]).find((v)=>v !== undefined),
                onCreateRule: onCreateRule ? ()=>onCreateRule(notification) : undefined,
                localListener: (0,inlineActions/* isLocalListener */.qq)(),
                disabledReason: offlineReason
            })
        ]
    }, notification.id);
}
/**
 * Collapsible section listing auto-handled (classifier-resolved) notifications,
 * shown below the main notification list.
 */ function AutoHandledSection(param) {
    let { notifications, isOpen, onToggle } = param;
    if (notifications.length === 0) return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: NotificationPanel_css/* autoHandledSection */.j8,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                className: NotificationPanel_css/* autoHandledHeader */.zt,
                onClick: onToggle,
                "aria-expanded": isOpen,
                "aria-controls": "auto-handled-list",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: NotificationPanel_css/* autoHandledHeaderLeft */.Oo,
                        children: [
                            "Auto-handled",
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: NotificationPanel_css/* autoHandledBadge */.L,
                                children: notifications.length
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: "".concat(NotificationPanel_css/* autoHandledChevron */.PY, " ").concat(isOpen ? NotificationPanel_css/* autoHandledChevronOpen */.P0 : ""),
                        children: "▼"
                    })
                ]
            }),
            isOpen && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                id: "auto-handled-list",
                className: NotificationPanel_css/* autoHandledList */.gb,
                children: notifications.map((n)=>{
                    var _n_metadata, _n_metadata1, _n_metadata2;
                    var _n_metadata_approval_decision;
                    const decision = (_n_metadata_approval_decision = (_n_metadata = n.metadata) === null || _n_metadata === void 0 ? void 0 : _n_metadata["approval_decision"]) !== null && _n_metadata_approval_decision !== void 0 ? _n_metadata_approval_decision : "allow";
                    const ruleName = (_n_metadata1 = n.metadata) === null || _n_metadata1 === void 0 ? void 0 : _n_metadata1["classifier_rule_name"];
                    var _n_metadata_tool_name;
                    const toolName = (_n_metadata_tool_name = (_n_metadata2 = n.metadata) === null || _n_metadata2 === void 0 ? void 0 : _n_metadata2["tool_name"]) !== null && _n_metadata_tool_name !== void 0 ? _n_metadata_tool_name : n.title;
                    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: NotificationPanel_css/* autoHandledItem */.nc,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: NotificationPanel_css/* autoHandledDecision */.E0,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        "aria-hidden": "true",
                                        children: decision === "deny" ? "✗" : "✓"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: a11y_css/* visuallyHidden */.Q,
                                        children: decision === "deny" ? "Denied" : "Approved"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: NotificationPanel_css/* autoHandledContent */.ZS,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                        className: NotificationPanel_css/* autoHandledTitle */.k_,
                                        children: toolName
                                    }),
                                    (n.message || ruleName) && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: NotificationPanel_css/* autoHandledMeta */.$5,
                                        children: [
                                            n.message && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                children: n.message
                                            }),
                                            ruleName && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                children: [
                                                    "\xb7 ",
                                                    ruleName
                                                ]
                                            })
                                        ]
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: NotificationPanel_css/* autoHandledTimestamp */._O,
                                children: (0,datetime/* formatRelativeTime */.fw)(n.timestamp)
                            })
                        ]
                    }, n.id);
                })
            })
        ]
    });
}
/**
 * "Needs a decision" — always-expanded top tier of unread actionable
 * notifications (approval_needed/question/error/task_failed/warning). See
 * design/ux.md Surface 1 and Surface 4.
 *
 * Deliberately does not pass `removeFromHistory` to its `NotificationItem`s —
 * an item here leaves only by being resolved (Approve/Deny/Open session) or
 * by its underlying state resolving elsewhere, never via the ✕ control.
 */ function NeedsDecisionSection(param) {
    let { notifications, totalActionableCount, onClearFilter, resolvedApprovals, pendingApprovals, blockedApprovals, failedApprovals, resolveApproval, handleNotificationClick, getSessionHref, staleness } = param;
    // Focus management on removal (design/ux.md AC34): when the item that held
    // keyboard focus leaves the section (resolved elsewhere or acted on),
    // move focus to the item now at the same list position, or to the
    // section heading if none remain. Mirrors ReviewQueuePanel.tsx's identical
    // pattern for the Review Queue's own needs-decision tier (Task 3.2.1's
    // Surface 9 focus handling).
    const headingRef = (0,react.useRef)(null);
    const notificationIds = (0,react.useMemo)(()=>notifications.map((n)=>n.id), [
        notifications
    ]);
    const resolveNeedsDecisionElement = (0,react.useCallback)((id)=>document.querySelector('[data-testid="needs-decision-item-'.concat(id, '"]')), []);
    const focusedIdRef = (0,useFocusRestoreOnRemoval/* useFocusRestoreOnRemoval */.J)(notificationIds, resolveNeedsDecisionElement, headingRef);
    const count = notifications.length;
    const announcement = count === 0 ? "" : count === 1 ? "1 item needs a decision" : "".concat(count, " items need a decision");
    const heading = /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: NotificationPanel_css/* needsDecisionHeadingRow */.w4,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("h3", {
                className: NotificationPanel_css/* needsDecisionHeading */.gM,
                ref: headingRef,
                tabIndex: -1,
                "data-testid": "needs-decision-heading",
                children: [
                    "Needs a decision \xb7 ",
                    count
                ]
            }),
            staleness && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                className: NotificationPanel_css/* stalenessIndicator */.M2,
                role: "status",
                "data-testid": "needs-decision-staleness",
                children: [
                    "Last updated ",
                    staleness.label,
                    " ago \xb7",
                    " ",
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: NotificationPanel_css/* stalenessRetry */.Lj,
                        onClick: staleness.onRetry,
                        children: "Retry"
                    })
                ]
            })
        ]
    });
    // Filter-aware empty state (Product Triad Review round-4 blocker fix):
    // never render the calm "All caught up" copy when a filter — not a lack
    // of actionable items — is what emptied the filtered list.
    if (count === 0) {
        const hiddenByFilter = totalActionableCount > 0;
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: NotificationPanel_css/* needsDecisionSection */.ro,
            "data-testid": "needs-decision-section",
            children: [
                heading,
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    "aria-live": "polite",
                    className: a11y_css/* visuallyHidden */.Q,
                    "data-testid": "needs-decision-announcement",
                    children: hiddenByFilter ? totalActionableCount === 1 ? "1 item needs a decision, but is hidden by your filter" : "".concat(totalActionableCount, " items need a decision, but are hidden by your filter") : "All caught up"
                }),
                hiddenByFilter ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: NotificationPanel_css/* needsDecisionEmpty */.rt,
                    "data-testid": "needs-decision-hidden-by-filter",
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: NotificationPanel_css/* needsDecisionEmptyIcon */.i6,
                            "aria-hidden": "true",
                            children: "⚠️"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                            className: NotificationPanel_css/* needsDecisionEmptyText */._J,
                            children: totalActionableCount === 1 ? "1 item needs a decision, but is hidden by your filter" : "".concat(totalActionableCount, " items need a decision, but are hidden by your filter")
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            type: "button",
                            className: NotificationPanel_css/* needsDecisionClearFilterButton */.p9,
                            onClick: onClearFilter,
                            children: "Clear filter"
                        })
                    ]
                }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: NotificationPanel_css/* needsDecisionEmpty */.rt,
                    "data-testid": "needs-decision-empty",
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: NotificationPanel_css/* needsDecisionEmptyIcon */.i6,
                            "aria-hidden": "true",
                            children: "✓"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                            className: NotificationPanel_css/* needsDecisionEmptyText */._J,
                            children: "All caught up"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                            className: NotificationPanel_css/* needsDecisionEmptySubtext */.C3,
                            children: "Nothing needs your attention right now"
                        })
                    ]
                })
            ]
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: NotificationPanel_css/* needsDecisionSection */.ro,
        "data-testid": "needs-decision-section",
        children: [
            heading,
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                "aria-live": "polite",
                className: a11y_css/* visuallyHidden */.Q,
                "data-testid": "needs-decision-announcement",
                children: announcement
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: NotificationPanel_css/* list */.p_,
                children: (0,notificationGrouping/* groupNotifications */._Q)(notifications).map((group)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        "data-testid": "needs-decision-item-".concat(group.notification.id),
                        tabIndex: -1,
                        onFocus: ()=>{
                            focusedIdRef.current = group.notification.id;
                        },
                        onBlur: ()=>{
                            if (focusedIdRef.current === group.notification.id) focusedIdRef.current = null;
                        },
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(NotificationItem, {
                            group: group,
                            resolvedApprovals: resolvedApprovals,
                            pendingApprovals: pendingApprovals,
                            blockedApprovals: blockedApprovals,
                            failedApprovals: failedApprovals,
                            resolveApproval: resolveApproval,
                            handleNotificationClick: handleNotificationClick,
                            getSessionHref: getSessionHref
                        })
                    }, group.notification.id))
            })
        ]
    });
}


/***/ }),

/***/ 44546:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 46085:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 57642:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   O9: () => (/* binding */ STRUCTURED_QUESTION_FLAG),
/* harmony export */   OK: () => (/* binding */ rulePrefillFor),
/* harmony export */   cp: () => (/* binding */ approvalDisplay),
/* harmony export */   iw: () => (/* binding */ STRUCTURED_RENDER_SCOPE),
/* harmony export */   j_: () => (/* binding */ INLINE_ACTIONS_FLAG),
/* harmony export */   qq: () => (/* binding */ isLocalListener)
/* harmony export */ });
/** Gates descriptor-driven rendering; flag-off keeps the legacy Approve/Deny path. */ const INLINE_ACTIONS_FLAG = "notification_inline_actions";
/** Gates the question card; its render stage chooses between the card and the plain option buttons. */ const STRUCTURED_QUESTION_FLAG = "askuserquestion_structured";
const STRUCTURED_RENDER_SCOPE = "stage:render";
/** Approval fields the server stamps on the notification's metadata. */ function approvalDisplay(n) {
    var _md_tool_input_command;
    var _n_metadata;
    const md = (_n_metadata = n.metadata) !== null && _n_metadata !== void 0 ? _n_metadata : {};
    const text = md.tool_input_command || md.tool_input_file || undefined;
    const program = (_md_tool_input_command = md.tool_input_command) === null || _md_tool_input_command === void 0 ? void 0 : _md_tool_input_command.trim().split(/\s+/)[0];
    return {
        tool: md.tool_name || undefined,
        text,
        unrenderable: md.tool_input_unrenderable === "true",
        ruleName: md.classifier_rule_name || undefined,
        ruleScope: [
            md.tool_name,
            program
        ].filter(Boolean).join(" ") || undefined
    };
}
/**
 * The local listener binds loopback; the remote one is reached by another host name.
 * Only gates the More menu, so a wrong answer cannot widen what the server allows.
 */ function isLocalListener() {
    if (false) {}
    return [
        "localhost",
        "127.0.0.1",
        "[::1]",
        "::1"
    ].includes(window.location.hostname);
}
/** Seeds the rule builder from an approval notification. */ function rulePrefillFor(n) {
    var _n_metadata_tool_input_command, _n_metadata, _n_metadata1;
    const program = (_n_metadata = n.metadata) === null || _n_metadata === void 0 ? void 0 : (_n_metadata_tool_input_command = _n_metadata.tool_input_command) === null || _n_metadata_tool_input_command === void 0 ? void 0 : _n_metadata_tool_input_command.trim().split(/\s+/)[0];
    return {
        toolName: (_n_metadata1 = n.metadata) === null || _n_metadata1 === void 0 ? void 0 : _n_metadata1.tool_name,
        programs: program ? [
            program
        ] : undefined
    };
}


/***/ }),

/***/ 71208:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   WQ: () => (/* binding */ invokeNotificationAction)
/* harmony export */ });
/* unused harmony exports ACTION_CLIENT_HEADER, ACTION_CLIENT_WEB_UI */
/* harmony import */ var _bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(95444);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(66144);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(21694);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(23575);
/* harmony import */ var _gen_session_v1_notification_action_pb__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(75449);





/** Labels the audit channel (web_local / web_remote); the server treats any other caller as unknown. */ const ACTION_CLIENT_HEADER = "X-Stapler-Client";
const ACTION_CLIENT_WEB_UI = "web-ui";
function invokeNotificationAction(init) {
    const client = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_0__/* .getConnectTransport */ .qW)());
    // One-shot mutation from an explicit click; aborting on unmount would leave its outcome unknown.
    // abort-signal-exempt
    return client.invokeNotificationAction((0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_notification_action_pb__WEBPACK_IMPORTED_MODULE_2__/* .InvokeNotificationActionRequestSchema */ .EY, {
        ...init,
        params: init.params && (0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_notification_action_pb__WEBPACK_IMPORTED_MODULE_2__/* .NotificationActionParamsSchema */ .F4, init.params)
    }), {
        headers: {
            [ACTION_CLIENT_HEADER]: ACTION_CLIENT_WEB_UI
        }
    });
}


/***/ })

}]);
//# sourceMappingURL=12-3d3d125c94cd20f9.js.map