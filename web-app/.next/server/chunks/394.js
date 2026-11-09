"use strict";
exports.id = 394;
exports.ids = [394];
exports.modules = {

/***/ 2452:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   l: () => (/* binding */ useSessionVcsContext),
/* harmony export */   m: () => (/* binding */ SessionVcsProvider)
/* harmony export */ });
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(79147);
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(88782);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_1__);
/* harmony import */ var _lib_hooks_useSessionVcs__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(17518);
/* __next_internal_client_entry_do_not_use__ SessionVcsProvider,useSessionVcsContext auto */ 


const SessionVcsContext = /*#__PURE__*/ (0,react__WEBPACK_IMPORTED_MODULE_1__.createContext)(null);
/**
 * Provides a single shared VCS state (status + diff) for a SessionDetail and
 * all of its tabs (VcsPanel, FilesTab, DiffViewer). Without this provider all
 * three tabs fetched independently with no shared cache.
 *
 * Place this around the tab content in SessionDetail, not at the app root —
 * the data is scoped to a single session and should start/stop with it.
 */ function SessionVcsProvider({ sessionId, baseUrl, isActive, children }) {
    const value = (0,_lib_hooks_useSessionVcs__WEBPACK_IMPORTED_MODULE_2__/* .useSessionVcs */ .u)(sessionId, baseUrl, isActive);
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(SessionVcsContext.Provider, {
        value: value,
        children: children
    });
}
function useSessionVcsContext() {
    const ctx = (0,react__WEBPACK_IMPORTED_MODULE_1__.useContext)(SessionVcsContext);
    if (!ctx) throw new Error("useSessionVcsContext must be used within SessionVcsProvider");
    return ctx;
}


/***/ }),

/***/ 9532:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  m: () => (/* binding */ CreatePullRequestModal)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-dom.js
var react_dom = __webpack_require__(68051);
// EXTERNAL MODULE: ./src/lib/hooks/useFocusTrap.ts
var useFocusTrap = __webpack_require__(99243);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Fanimations.css.ts.vanilla.css%22%2C%22source%22%3A%22QGtleWZyYW1lcyBfMXZvMmxuYjAgewogIDAlLCAxMDAlIHsKICAgIGJveC1zaGFkb3c6IDAgMCAwIDAgdHJhbnNwYXJlbnQ7CiAgfQogIDUwJSB7CiAgICBib3gtc2hhZG93OiAwIDAgOHB4IDJweCB2YXIoLS1fMTllejJyejI0KSwgMCAwIDE2cHggNHB4IHZhcigtLV8xOWV6MnJ6MjUpOwogIH0KfQpAa2V5ZnJhbWVzIF8xdm8ybG5iMSB7CiAgMCUsIDEwMCUgewogICAgdGV4dC1zaGFkb3c6IDAgMCA0cHggdmFyKC0tXzE5ZXoycnoyNCk7CiAgfQogIDUwJSB7CiAgICB0ZXh0LXNoYWRvdzogMCAwIDEycHggdmFyKC0tXzE5ZXoycnoyNCksIDAgMCAyNHB4IHZhcigtLV8xOWV6MnJ6MjUpOwogIH0KfQpAa2V5ZnJhbWVzIF8xdm8ybG5iMiB7CiAgZnJvbSB7CiAgICB0cmFuc2Zvcm06IHRyYW5zbGF0ZVkoMTZweCk7CiAgICBvcGFjaXR5OiAwOwogIH0KICB0byB7CiAgICB0cmFuc2Zvcm06IHRyYW5zbGF0ZVkoMCk7CiAgICBvcGFjaXR5OiAxOwogIH0KfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Fanimations_css_ts_vanilla_css_22_2C_22source_22_3A_22QGtleWZyYW1lcyBfMXZvMmxuYjAgewogIDAlLCAxMDAlIHsKICAgIGJveC1zaGFkb3c6IDAgMCAwIDAgdHJhbnNwYXJlbnQ7CiAgfQogIDUwJSB7CiAgICBib3gtc2hhZG93OiAwIDAgOHB4IDJweCB2YXIoLS1fMTllejJyejI0KSwgMCAwIDE2cHggNHB4IHZhcigtLV8xOWV6MnJ6MjUpOwogIH0KfQpAa2V5ZnJhbWVzIF8xdm8ybG5iMSB7CiAgMCUsIDEwMCUgewogICAgdGV4dC1zaGFkb3c6IDAgMCA0cHggdmFyKC0tXzE5ZXoycnoyNCk7CiAgfQogIDUwJSB7CiAgICB0ZXh0LXNoYWRvdzogMCAwIDEycHggdmFyKC0tXzE5ZXoycnoyNCksIDAgMCAyNHB4IHZhcigtLV8xOWV6MnJ6MjUpOwogIH0KfQpAa2V5ZnJhbWVzIF8xdm8ybG5iMiB7CiAgZnJvbSB7CiAgICB0cmFuc2Zvcm06IHRyYW5zbGF0ZVkoMTZweCk7CiAgICBvcGFjaXR5OiAwOwogIH0KICB0byB7CiAgICB0cmFuc2Zvcm06IHRyYW5zbGF0ZVkoMCk7CiAgICBvcGFjaXR5OiAxOwogIH0KfQ_22_7D = __webpack_require__(36278);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FSessionCard.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-0ca2_bOPJ7fwWvwGLjg-UT6WdUHFBgP-99PxwOC1qibTYSqVJUbOfQ_34QqQcpUQ87abYFFk0TWyKHw3lzZqTPT-R6EDghGYj2K7zPffC_DwAcBE_UBwB4ikMqrwHwP6nvUmCWHbhIAv0xxpL8-2GXXmbF_W8fAJC8PRUOTfXLid8-fG4jA4eRGVjMCQ61wbkRQn65mcH1hjYzDRs2gI7gEkvy4Efk2KVrd9xyY4z89mGhV1iq8XscPh0Fz1kUgGcsHjzvD_hIXpB42aoJey4iIgIA0wvIeEyj1ihijPIEjmietQEtd2pMiqOIsmPnLlR3EyyOlHl7LiVPOmM04cJcZFwEIOWUSSKKS2qzVFLOggqJkMdcAH-BMkBwRuZgzy9edsIRP5tXSx4Af7GsLtWUMy8m-OKdCD2eZHNVbYdX6woSY0mf1VXvTPZPVHoSp96JHk9xMVGjVEpCigVhUm2HM4kpI8KT15QEgLKYMuJl9IXYtxlOSAAykmWUs9-wiIrbmNEEFwiUt0sFtW9FucAaSX-xXGf2TUkTyo7eIWehHlNtrRlyoHHsJTwiBXXlqQWcxPgagBDH4UNC2YPmWYhF5FEWkcsc-LM5WM_A38HGT7LZJ0P2ghN_JkJLoMG2NuNpKV0VAwPgA5RewC69AHHc4wd_DsqfBZzNga_-FbJqw0Frc_WVWrfR3sWqFiWtN1mIY_LgLx5NHXBjCH0Hin6JpkIRLR_nYLPT__2FQgVUMuyRZ8JkFgDGGTFQXCsUS5XxYnKQAVij9GIM2YxSL9Go9Wo43JpU2Zrw9IqrHqVvA_Zogo8kAIX8YuEdCztAmHyQHIhCAebdhduXtjMTl12bQ9u1cfdRE6dWQLzPeJxLJbsacSWTHTsDPFB5IsnTAKz9X1qMN2y1t_Z_se3WriC_wwoZeFGW5vI_hTr_89fwRMKnPb_8-l-F7plG8hSAwn0UULRJab73Q8Vq-rh9rCfs1YSIZqnSz0NM1Apf8kzSw9Ur7AphMgBZikPi7Yk8E8KUasf0yDwqSZIFICSVjR1cGr2YS4cGrqVbPHAmlU0LAFxAtBYkqS-fSyJsfF9bPIcQW1uLvg_8ISXZTXKDyQQ3uLHFqTAQG818nstCdSorUAsL9AsJrXdP3IztYdsRp8PMOtjgSvdzB9TrkH9H1xmApZD3U1nTz8mgrxNI-zhriULr_n7WJxNjjE1Nih1_IIqF_RSDfyrFiEmx0_tT7EfYNh2JrUeYV8P5MgLn2g_nMhq6XMyVnu7HGJlwYn1saQjsLza1UTbB69hovZ6D5pe_WHZtpFMQplja0m_piMABpcY5MT2sigwcZrNf-F5GPIiFscZm2SNEl8mb63iB4q8XUUHKYD7kcZ6wWp02VuzIulzaNb5zwKjoKfIaF-cViWMaGlC5GbQ6dRR9Xx2V5CI9I5zL05SIUJ1qaiTTEUFHpwFJp6bYfB2DRAYgHUxIYgzS0wCk2ISU3W8wXiYFOpbRkGN4iwG8rQNhPgIJwgGLaRmg5zGc2ABO3IR0fi1O49SEVnR7eVfmXU2rJ7QerSwr8aJG9K5Xj4O-08vvYx4-jXjtG03pK4Oeze2WZV1Ylman8LUheJnKGnU2ypyfRQGg-G3igNw4dGY4LT9c3s2rKsKynftu6zxyrcuo6X7rOM1NWAnAerEm0Wdt3cw69av1zpLtlZEW0ZoV4exE2qp17EhuJ9s3FjrB9VT_OYkDmy4HpqAEJ1mSKefdx76krZ03NzmI49ido4XrDjsrbtrUM1PrxtADD_PMO1N5omzKDLgupzzTjO5jMjhyilSNHHAbcJtb0z1wO2APpgaEcOcG0pdEaAePpQg28B77fQfssbIAJJR5TerDRhD3A9QmtWDCIS4SsCcaRTqjpQLC5gaJY5pmVOXBzycqiadyYEXepW1k9_3LaVYq0BEJeZVhr3I3ro2ZMt6qT1iLGqLkXDp1L52ziIjCjpvAwskcneyTip0dcELjawASzrii3gRhiPp3lPcE881kMiBJ_ujsw-0Oe6WNrEmOTcfuOiOH8QM9PDYnrynZSdQw3H2uKTLesigkFLxQbsNf-GvDHTjRxRZOp2FdbQbSN84sV_UNh8gtfdPDqAGTMi_wy13WcCg9AJ_ap-V7mR-_bQIXJq1CSFOJbMYwO6jYpJdWqux1td3tSMS-7aeN3_YjcAFdgcy21I9uINEW_TI8UhFtVWCBlWOpLqwq_W6cTXXplmCSTXL7dizJW_w60AuJ2o5vN5r6fevKvF28LGqraOUqsOrK5Yuu7ioX7VtHhpKKN-heU1wva3bQ959PwANwUxbqKuftXQOAc8lNcqZvok52ha-Vo2rc-q1qMCXW7mhA4zxLo6_2EqiqZo8OWKWiARF2RNAwvUOG0yCiGd7HJGrXaNcmhowXyMf8XAh4M3sgtrHTOFBMO6zZBjbrGrtOFfhHtHwm313x40jni3VssuihiRgwLh9qvs0m8Hu8LcNcxJKICteIHHAeS_vAt9iYM6XNrpXhm9y56WbqqxOGE9o6jNXuoSNcji-znHoszG0if9-NWxzbTVDr5z7HpgI3_1PdmKE-lsnGsqymT7gj2SLsLnp0Is_G3Pd4gY73cvdRwU-9fVRoWhtVQ5yxtO57NdqhWeVyq7DHL5Wt_P6oXYgdDhQKCVa-Ix7Q8Nz0Q_30W95Ov9PS7vsAftesg7EWDyvQ7NiTC0itNTqnkQH4sN-cNwtcB4-x4G80SbmQWEcLLh_gsojWtGatF7PjqIoMarFQHG2HOW8QRb65JxxvPJrSAGoRRmf1dOTSbrqZ0ErX6fJbGnoBIZoDtFrqZkRjWdSqj9TJ9tGkxsBx0y2o6EUJarN0T8FiWhaoak7rmFkV0RNm-gCEBrPXL7Ma2hRhmhIbbe6JjSantl-dyUXonqCBjXpo1lrkFeE4Wv4sPLvfttzUt4OW7xAwo9VPT_XSRo32602tfKPV9wywzWVeoy1rK0IokgU-8B18qro6HQWJ26Rxc1OCYzwN6zxpTQ1MKzc5noOwRHI7IJB35rUHgrA8K6aQmISy01OPNkZKwTzgrO1BjsJfFSqgQZ9RDvP44ZARGQDPbthHW0dvXKWv7ZxlM0tX5upblW3oJsZ6srxW1ebj74TF_OMcfPydMxyqT7_xXFAiwL_I-eN8oK6zRU5cteoMaGk0cCDV4aVVi0sF8aoGhjMXkbcXBD8FQP0p1NIkzqObOHerTHPL3vqE1rgxPRpsnUP4Z9lIg_L-nVBGgyiP9e9v7TOxX-uFmV-tlm4216qeTmtc7uvLGCjtqSnPWFDMpMfyhAgaBkAWnbRYFBcyE6uBQuplrBSKbiykbu3Zh-7jQUNdrM3E92ua3405NDgQLb5F0zx-m-5x9H5d884z9ZQmutcTeSzncBOZyxyJs6nj5tYQI0VWWrWGNfRPZs1rm0PfX7Qv7jawhqRf_pL2e6S9IeDTXwS8kYCtEcduFNrpCEPxz0zlN2kdv88-jJA1GWmuwWPiMfkxH8SHW58rk7-yn9CtvlcUGj4DNg1LPU_9t_i41k94fk5IRDF4SAU5EJF5gkR5SCIv4VVnT_FdJ0Ss9zf0VVl8mChf5nhg39d3vk1YlnFP3yKstfrOfNtEmUDqvFLhU_luCushj27J6A_4zFHM9n4H4XpDqLMZZ9nIo8zjuWyPpZJoQF7I8-KoQdmBMirbCCLeg2H1To5-BP3FbhxF_Xz8jdg1XDLiku2meJLdZEhYom6Wucrjh7HDaNKoso_C3TfVriQ5rdXg8a7_yTP3qczGrZKio6CRp1uJAgDBP4AH2wTTOUWVB7JoVXZ-O1_EUr92w3zhhkV7tPFbtIeVNvR3OpWuoKy6mfvB5dyWnRQJjvXEOtOjH-7A7Ho-EUEMejVhbBjTtKPcOuQqsMEiI7P5iCTBrwPc__bh2_8BkD8AsRZIAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FSessionCard_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_0ca2_bOPJ7fwWvwGLjg_UT6WdUHFBgP_99PxwOC1qibTYSqVJUbOfQ_34QqQcpUQ87abYFFk0TWyKHw3lzZqTPT_R6EDghGYj2K7zPffC_DwAcBE_UBwB4ikMqrwHwP6nvUmCWHbhIAv0xxpL8_2GXXmbF_W8fAJC8PRUOTfXLid8_fG4jA4eRGVjMCQ61wbkRQn65mcH1hjYzDRs2gI7gEkvy4Efk2KVrd9xyY4z89mGhV1iq8XscPh0Fz1kUgGcsHjzvD_hIXpB42aoJey4iIgIA0wvIeEyj1ihijPIEjmietQEtd2pMiqOIsmPnLlR3EyyOlHl7LiVPOmM04cJcZFwEIOWUSSKKS2qzVFLOggqJkMdcAH_BMkBwRuZgzy9edsIRP5tXSx4Af7GsLtWUMy8m_OKdCD2eZHNVbYdX6woSY0mf1VXvTPZPVHoSp96JHk9xMVGjVEpCigVhUm2HM4kpI8KT15QEgLKYMuJl9IXYtxlOSAAykmWUs9_wiIrbmNEEFwiUt0sFtW9FucAaSX_xXGf2TUkTyo7eIWehHlNtrRlyoHHsJTwiBXXlqQWcxPgagBDH4UNC2YPmWYhF5FEWkcsc_LM5WM_A38HGT7LZJ0P2ghN_JkJLoMG2NuNpKV0VAwPgA5RewC69AHHc4wd_DsqfBZzNga_FbJqw0Frc_WVWrfR3sWqFiWtN1mIY_LgLx5NHXBjCH0Hin6JpkIRLR_nYLPT_2FQgVUMuyRZ8JkFgDGGTFQXCsUS5XxYnKQAVij9GIM2YxSL9Go9Wo43JpU2Zrw9IqrHqVvA_Zogo8kAIX8YuEdCztAmHyQHIhCAebdhduXtjMTl12bQ9u1cfdRE6dWQLzPeJxLJbsacSWTHTsDPFB5IsnTAKz9X1qMN2y1t_Z_se3WriC_wwoZeFGW5vI_hTr_89fwRMKnPb_8_l_F7plG8hSAwn0UULRJab73Q8Vq_rh9rCfs1YSIZqnSz0NM1Apf8kzSw9Ur7AphMgBZikPi7Yk8E8KUasf0yDwqSZIFICSVjR1cGr2YS4cGrqVbPHAmlU0LAFxAtBYkqS_fSyJsfF9bPIcQW1uLvg_8ISXZTXKDyQQ3uLHFqTAQG818nstCdSorUAsL9AsJrXdP3IztYdsRp8PMOtjgSvdzB9TrkH9H1xmApZD3U1nTz8mgrxNI_zhriULr_n7WJxNjjE1Nih1_IIqF_RSDfyrFiEmx0_tT7EfYNh2JrUeYV8P5MgLn2g_nMhq6XMyVnu7HGJlwYn1saQjsLza1UTbB69hovZ6D5pe_WHZtpFMQplja0m_piMABpcY5MT2sigwcZrNf_F5GPIiFscZm2SNEl8mb63iB4q8XUUHKYD7kcZ6wWp02VuzIulzaNb5zwKjoKfIaF_cViWMaGlC5GbQ6dRR9Xx2V5CI9I5zL05SIUJ1qaiTTEUFHpwFJp6bYfB2DRAYgHUxIYgzS0wCk2ISU3W8wXiYFOpbRkGN4iwG8rQNhPgIJwgGLaRmg5zGc2ABO3IR0fi1O49SEVnR7eVfmXU2rJ7QerSwr8aJG9K5Xj4O_08vvYx4_jXjtG03pK4Oeze2WZV1Ylman8LUheJnKGnU2ypyfRQGg_G3igNw4dGY4LT9c3s2rKsKynftu6zxyrcuo6X7rOM1NWAnAerEm0Wdt3cw69av1zpLtlZEW0ZoV4exE2qp17EhuJ9s3FjrB9VT_OYkDmy4HpqAEJ1mSKefdx76krZ03NzmI49ido4XrDjsrbtrUM1PrxtADD_PMO1N5omzKDLgupzzTjO5jMjhyilSNHHAbcJtb0z1wO2APpgaEcOcG0pdEaAePpQg28B77fQfssbIAJJR5TerDRhD3A9QmtWDCIS4SsCcaRTqjpQLC5gaJY5pmVOXBzycqiadyYEXepW1k9_3LaVYq0BEJeZVhr3I3ro2ZMt6qT1iLGqLkXDp1L52ziIjCjpvAwskcneyTip0dcELjawASzrii3gRhiPp3lPcE881kMiBJ_ujsw_0Oe6WNrEmOTcfuOiOH8QM9PDYnrynZSdQw3H2uKTLesigkFLxQbsNf_GvDHTjRxRZOp2FdbQbSN84sV_UNh8gtfdPDqAGTMi_wy13WcCg9AJ_ap_V7mR_bQIXJq1CSFOJbMYwO6jYpJdWqux1td3tSMS_7aeN3_YjcAFdgcy21I9uINEW_TI8UhFtVWCBlWOpLqwq_W6cTXXplmCSTXL7dizJW_w60AuJ2o5vN5r6fevKvF28LGqraOUqsOrK5Yuu7ioX7VtHhpKKN_heU1wva3bQ959PwANwUxbqKuftXQOAc8lNcqZvok52ha_Vo2rc_q1qMCXW7mhA4zxLo6_2EqiqZo8OWKWiARF2RNAwvUOG0yCiGd7HJGrXaNcmhowXyMf8XAh4M3sgtrHTOFBMO6zZBjbrGrtOFfhHtHwm313x40jni3VssuihiRgwLh9qvs0m8Hu8LcNcxJKICteIHHAeS_vAt9iYM6XNrpXhm9y56WbqqxOGE9o6jNXuoSNcji_znHoszG0if9_NWxzbTVDr5z7HpgI3_1PdmKE_lsnGsqymT7gj2SLsLnp0Is_G3Pd4gY73cvdRwU_9fVRoWhtVQ5yxtO57NdqhWeVyq7DHL5Wt_P6oXYgdDhQKCVa_Ix7Q8Nz0Q_30W95Ov9PS7vsAftesg7EWDyvQ7NiTC0itNTqnkQH4sN_cNwtcB4_x4G80SbmQWEcLLh_gsojWtGatF7PjqIoMarFQHG2HOW8QRb65JxxvPJrSAGoRRmf1dOTSbrqZ0ErX6fJbGnoBIZoDtFrqZkRjWdSqj9TJ9tGkxsBx0y2o6EUJarN0T8FiWhaoak7rmFkV0RNm_gCEBrPXL7Ma2hRhmhIbbe6JjSantl_dyUXonqCBjXpo1lrkFeE4Wv4sPLvfttzUt4OW7xAwo9VPT_XSRo32602tfKPV9wywzWVeoy1rK0IokgU_8B18qro6HQWJ26Rxc1OCYzwN6zxpTQ1MKzc5noOwRHI7IJB35rUHgrA8K6aQmISy01OPNkZKwTzgrO1BjsJfFSqgQZ9RDvP44ZARGQDPbthHW0dvXKWv7ZxlM0tX5upblW3oJsZ6srxW1ebj74TF_OMcfPydMxyqT7_xXFAiwL_I_eN8oK6zRU5cteoMaGk0cCDV4aVVi0sF8aoGhjMXkbcXBD8FQP0p1NIkzqObOHerTHPL3vqE1rgxPRpsnUP4Z9lIg_L_nVBGgyiP9e9v7TOxX_uFmV_tlm4216qeTmtc7uvLGCjtqSnPWFDMpMfyhAgaBkAWnbRYFBcyE6uBQuplrBSKbiykbu3Zh_7jQUNdrM3E92ua3405NDgQLb5F0zx_m_5x9H5d884z9ZQmutcTeSzncBOZyxyJs6nj5tYQI0VWWrWGNfRPZs1rm0PfX7Qv7jawhqRf_pL2e6S9IeDTXwS8kYCtEcduFNrpCEPxz0zlN2kdv88_jJA1GWmuwWPiMfkxH8SHW58rk7_yn9CtvlcUGj4DNg1LPU_9t_i41k94fk5IRDF4SAU5EJF5gkR5SCIv4VVnT_FdJ0Ss9zf0VVl8mChf5nhg39d3vk1YlnFP3yKstfrOfNtEmUDqvFLhU_luCushj27J6A_4zFHM9n4H4XpDqLMZZ9nIo8zjuWyPpZJoQF7I8_KoQdmBMirbCCLeg2H1To5_BP3FbhxF_Xz8jdg1XDLiku2meJLdZEhYom6Wucrjh7HDaNKoso_C3TfVriQ5rdXg8a7_yTP3qczGrZKio6CRp1uJAgDBP4AH2wTTOUWVB7JoVXZ_O1_EUr92w3zhhkV7tPFbtIeVNvR3OpWuoKy6mfvB5dyWnRQJjvXEOtOjH_7A7Ho_EUEMejVhbBjTtKPcOuQqsMEiI7P5iCTBrwPc_bh2_8BkD8AsRZIAAA_22_7D = __webpack_require__(61728);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FCreatePullRequestModal.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA9VUyY7jIBC95yvqMlJbChE4W4f-mBE2xGGCwQLcXkbz7yNIlHYcZ5H6MBr5YKkW6tWrV7WQ9Xa1W2H4PQNoJPcHCgTjHx8zgFJqdBCyOPhgE2WwWeFkLyh8CutlzlSwVYxzqYsQVLVA0qoN1sxYLiwFUrXgjJIcPpl9Q-gn2Yk-tb1IvqKQZVzWjo5Clu-nGJYfC2tqzccB2-jPjTJ27MLRtTfaoxNivHjfru2pi2jes1KqjoLUB2Gljx0zW0iNMuO9Kccvpn3yMfszW5wZo3uT1y7yZmqvpBYUtNFi0NUksPLcd4vcgXHTUMDxW1Yt2CJjb3gOhKRzSFfLOeAFua7KpWOZEvxUuGK59F1obhOZqK0LFbXxiCllGsEHySTmcOkqxToKmTL58QFJk-hJ8pyn7ov65iyfNcYDIOmU2v4_FT1XC4C3TDvppdH0ShWAF6kDwZwY8vJPNJV-R1PLmHNNEElfltAJ6GheJAF8s6Wl0cZVLB_ytboW9F6JqBmmZKGR9KJ0FHKhvbDB_Kt2Xu47lBvthfZDV8GqO_O7qHKEMT1jfNDhpGou2NcR-2R6c7s_G4xvntwtR0q8AYkjyEvJzTVdUgeBocsZeKCshwswOUN85qcxlqPMCnakEH9BQwNM2zs3aRJM_2SRu-SVk9G-fDLuDD_tk9t2RzSN_Flyf0zhuTCmvw4o34iLBwAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FCreatePullRequestModal_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9VUyY7jIBC95yvqMlJbChE4W4f_mBE2xGGCwQLcXkbz7yNIlHYcZ5H6MBr5YKkW6tWrV7WQ9Xa1W2H4PQNoJPcHCgTjHx8zgFJqdBCyOPhgE2WwWeFkLyh8CutlzlSwVYxzqYsQVLVA0qoN1sxYLiwFUrXgjJIcPpl9Q_gn2Yk_tb1IvqKQZVzWjo5Clu_nGJYfC2tqzccB2_jPjTJ27MLRtTfaoxNivHjfru2pi2jes1KqjoLUB2Gljx0zW0iNMuO9Kccvpn3yMfszW5wZo3uT1y7yZmqvpBYUtNFi0NUksPLcd4vcgXHTUMDxW1Yt2CJjb3gOhKRzSFfLOeAFua7KpWOZEvxUuGK59F1obhOZqK0LFbXxiCllGsEHySTmcOkqxToKmTL58QFJk_hJ8pyn7ov65iyfNcYDIOmU2v4_FT1XC4C3TDvppdH0ShWAF6kDwZwY8vJPNJV_R1PLmHNNEElfltAJ6GheJAF8s6Wl0cZVLB_ytboW9F6JqBmmZKGR9KJ0FHKhvbDB_Kt2Xu47lBvthfZDV8GqO_O7qHKEMT1jfNDhpGou2NcR_2R6c7s_G4xvntwtR0q8AYkjyEvJzTVdUgeBocsZeKCshwswOUN85qcxlqPMCnakEH9BQwNM2zs3aRJM_2SRu_SVk9G_fDLuDD_tk9t2RzSN_Flyf0zhuTCmvw4o34iLBwAA_22_7D = __webpack_require__(73045);
;// ./src/components/sessions/CreatePullRequestModal.css.ts




var bodyTextarea = 'iu74940';
var branchContext = 'iu74943';
var cancelButton = 'db4abu23';
var confirmDialog = 'db4abu1v';
var dialogActions = 'db4abu21';
var dialogContent = 'db4abu1x';
var errorMessage = 'db4abu20';
var fieldLabel = 'iu74941';
var loadingState = 'iu74944';
var prLink = 'iu74946';
var submitButton = 'db4abu22';
var successMessage = 'iu74945';
var textInput = 'iu74942';
var warningMessage = 'iu74947';
;// ./src/components/sessions/CreatePullRequestModal.tsx
/* __next_internal_client_entry_do_not_use__ CreatePullRequestModal auto */ 
// +feature: session-create-pr




const GENERIC_DRAFT_ERROR = "Couldn't load PR draft — try again.";
function CreatePullRequestModal({ session, isOpen, onClose, draftPullRequest, createPullRequest, triggerRef }) {
    const dialogRef = (0,react.useRef)(null);
    const titleInputRef = (0,react.useRef)(null);
    // Guards against a fast synthetic double-click racing the isSubmitting
    // re-render (ux.md criterion #8) — the disabled attribute alone isn't
    // enough because React state updates aren't synchronous.
    const submittingRef = (0,react.useRef)(false);
    const [title, setTitle] = (0,react.useState)("");
    const [body, setBody] = (0,react.useState)("");
    const [baseBranch, setBaseBranch] = (0,react.useState)("");
    const [isDrafting, setIsDrafting] = (0,react.useState)(false);
    const [isSubmitting, setIsSubmitting] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    const [existingPr, setExistingPr] = (0,react.useState)(null);
    const [success, setSuccess] = (0,react.useState)(null);
    const [hasDrafted, setHasDrafted] = (0,react.useState)(false);
    (0,useFocusTrap/* useFocusTrap */.r)(dialogRef, isOpen, triggerRef);
    const fetchDraft = async ()=>{
        setIsDrafting(true);
        setError(null);
        setExistingPr(null);
        setHasDrafted(false);
        try {
            const response = await draftPullRequest(session.id);
            if (!response) {
                setError(GENERIC_DRAFT_ERROR);
                return;
            }
            if (response.existingPrUrl) {
                setExistingPr({
                    url: response.existingPrUrl,
                    number: response.existingPrNumber
                });
                return;
            }
            setTitle(response.title);
            setBody(response.body);
            setBaseBranch(response.baseBranch);
            setHasDrafted(true);
        } catch  {
            setError(GENERIC_DRAFT_ERROR);
        } finally{
            setIsDrafting(false);
        }
    };
    // Reset all state when the modal opens and kick off the draft fetch.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (!isOpen) return;
        setTitle("");
        setBody("");
        setBaseBranch("");
        setExistingPr(null);
        setSuccess(null);
        setError(null);
        setHasDrafted(false);
        submittingRef.current = false;
        void fetchDraft();
    // Only re-run when the modal transitions to open, or for a different session.
    }, [
        isOpen,
        session.id
    ]);
    // Autofocus the title input once the draft resolves into the editable
    // form (Surface 4) — not on dialog open, and not the submit button, per
    // ux.md's explicit rationale that the body needs review first.
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (isOpen && hasDrafted && !isDrafting && !existingPr && !success) {
            titleInputRef.current?.focus();
        }
    }, [
        isOpen,
        hasDrafted,
        isDrafting,
        existingPr,
        success
    ]);
    if (!isOpen) return null;
    const handleClose = ()=>{
        onClose();
    };
    const handleSubmit = async ()=>{
        if (submittingRef.current) return;
        if (isSubmitting || isDrafting || !title.trim()) return;
        submittingRef.current = true;
        setIsSubmitting(true);
        setError(null);
        try {
            const response = await createPullRequest({
                sessionId: session.id,
                title,
                body,
                baseBranch
            });
            if (!response || !response.prUrl) {
                setError("Couldn't create the pull request — try again.");
                return;
            }
            setSuccess({
                prUrl: response.prUrl,
                prNumber: response.prNumber,
                alreadyExisted: response.alreadyExisted,
                persisted: response.persisted,
                persistError: response.persistError
            });
        } catch (err) {
            // AC6/criterion #4: surface the backend error verbatim, never wrapped.
            setError(err instanceof Error ? err.message : String(err));
        } finally{
            submittingRef.current = false;
            setIsSubmitting(false);
        }
    };
    const handleBackdropClick = ()=>{
        if (isSubmitting) return;
        handleClose();
    };
    const handleDialogKeyDown = (e)=>{
        if (e.key === "Escape" && !isSubmitting) {
            handleClose();
        }
    };
    const fieldsDisabled = isSubmitting;
    const submitDisabled = isSubmitting || isDrafting || !title.trim();
    return /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
        className: confirmDialog,
        onClick: (e)=>{
            e.stopPropagation();
            handleBackdropClick();
        },
        children: /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
            ref: dialogRef,
            role: "dialog",
            "aria-modal": "true",
            "aria-labelledby": "createPrDialogTitle",
            className: dialogContent,
            onClick: (e)=>e.stopPropagation(),
            onKeyDown: handleDialogKeyDown,
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("h3", {
                    id: "createPrDialogTitle",
                    children: "Create Pull Request"
                }),
                success ? /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
                    children: [
                        /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("p", {
                            className: successMessage,
                            children: [
                                "✅",
                                " ",
                                success.alreadyExisted ? "Updated" : "Created",
                                " PR #",
                                success.prNumber
                            ]
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("a", {
                            className: prLink,
                            href: success.prUrl,
                            target: "_blank",
                            rel: "noopener noreferrer",
                            "aria-label": `PR #${success.prNumber}`,
                            "data-testid": "github-pr-link",
                            children: success.prUrl
                        }),
                        !success.persisted && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("p", {
                            role: "alert",
                            className: warningMessage,
                            "data-testid": "create-pr-persist-warning",
                            children: [
                                "⚠",
                                " PR created but couldn't be saved to the session:",
                                " ",
                                success.persistError || "refresh to check."
                            ]
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                            className: dialogActions,
                            children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                onClick: handleClose,
                                className: submitButton,
                                "data-testid": "create-pr-close",
                                children: "Close"
                            })
                        })
                    ]
                }) : existingPr ? /* Surface 5: existing-PR / "View PR" dead end */ /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
                    children: [
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                            children: "This session already has a pull request."
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("a", {
                            className: prLink,
                            href: existingPr.url,
                            target: "_blank",
                            rel: "noopener noreferrer",
                            "aria-label": `PR #${existingPr.number}`,
                            "data-testid": "github-pr-link",
                            children: [
                                "View PR #",
                                existingPr.number
                            ]
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                            className: dialogActions,
                            children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                onClick: handleClose,
                                className: submitButton,
                                "data-testid": "create-pr-close",
                                children: "Close"
                            })
                        })
                    ]
                }) : isDrafting ? /* Surface 3: drafting/loading state */ /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
                    children: [
                        /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("p", {
                            className: loadingState,
                            "data-testid": "create-pr-loading",
                            children: [
                                "⏳",
                                " Drafting PR description…"
                            ]
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                            className: dialogActions,
                            children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                onClick: handleClose,
                                className: cancelButton,
                                "data-testid": "create-pr-cancel",
                                children: "Cancel"
                            })
                        })
                    ]
                }) : error && !hasDrafted ? /* Surface 8 Variant A: draft-fetch failure (form never rendered) */ /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
                    children: [
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                            role: "alert",
                            className: errorMessage,
                            "data-testid": "create-pr-error",
                            children: error
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                            className: dialogActions,
                            children: [
                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                    onClick: handleClose,
                                    className: cancelButton,
                                    "data-testid": "create-pr-close",
                                    children: "Close"
                                }),
                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                    onClick: ()=>void fetchDraft(),
                                    className: submitButton,
                                    "data-testid": "create-pr-retry",
                                    children: "Retry"
                                })
                            ]
                        })
                    ]
                }) : /* Surface 4/6/8B: editable form (idle, submitting, or submit-error) */ /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
                    children: [
                        (session.branch || baseBranch) && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("p", {
                            className: branchContext,
                            "data-testid": "create-pr-branch-context",
                            children: [
                                session.branch || "?",
                                " → ",
                                baseBranch || "?"
                            ]
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("label", {
                            className: fieldLabel,
                            htmlFor: "createPrTitle",
                            children: "Title"
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                            id: "createPrTitle",
                            ref: titleInputRef,
                            type: "text",
                            className: textInput,
                            value: title,
                            onChange: (e)=>setTitle(e.target.value),
                            disabled: fieldsDisabled,
                            "data-testid": "create-pr-title-input"
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("label", {
                            className: fieldLabel,
                            htmlFor: "createPrBody",
                            children: "Description"
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("textarea", {
                            id: "createPrBody",
                            className: bodyTextarea,
                            value: body,
                            onChange: (e)=>setBody(e.target.value),
                            disabled: fieldsDisabled,
                            "data-testid": "create-pr-body-input"
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("label", {
                            className: fieldLabel,
                            htmlFor: "createPrBaseBranch",
                            children: "Base branch"
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                            id: "createPrBaseBranch",
                            type: "text",
                            className: textInput,
                            value: baseBranch,
                            onChange: (e)=>setBaseBranch(e.target.value),
                            disabled: fieldsDisabled,
                            "data-testid": "create-pr-base-branch-select"
                        }),
                        error && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                            role: "alert",
                            className: errorMessage,
                            "data-testid": "create-pr-error",
                            children: error
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                            className: dialogActions,
                            children: [
                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                    onClick: handleClose,
                                    disabled: isSubmitting,
                                    className: cancelButton,
                                    "data-testid": "create-pr-cancel",
                                    children: "Cancel"
                                }),
                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                    onClick: ()=>void handleSubmit(),
                                    disabled: submitDisabled,
                                    className: submitButton,
                                    "data-testid": "create-pr-submit",
                                    children: isSubmitting ? "Creating PR…" : "Create PR"
                                })
                            ]
                        })
                    ]
                })
            ]
        })
    }), document.body);
}


/***/ }),

/***/ 15141:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   G: () => (/* binding */ isSessionStale),
/* harmony export */   w: () => (/* binding */ getLastActivityTimestamp)
/* harmony export */ });
/* harmony import */ var _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(29238);

/**
 * Returns the more recent of `lastMeaningfulOutput` and `lastTerminalUpdate`,
 * or `undefined` if neither is set. Extracted verbatim from the inline IIFE
 * that used to live in SessionCard.tsx's "Active Xm ago" header row.
 */ function getLastActivityTimestamp(session) {
    const moSecs = session.lastMeaningfulOutput?.seconds ?? BigInt(0);
    const tuSecs = session.lastTerminalUpdate?.seconds ?? BigInt(0);
    if (moSecs === BigInt(0) && tuSecs === BigInt(0)) {
        return undefined;
    }
    return moSecs >= tuSecs ? session.lastMeaningfulOutput : session.lastTerminalUpdate;
}
/**
 * A session is stale if it's ACTIVE but hasn't produced any recorded
 * activity within `thresholdMinutes`. Non-ACTIVE sessions (PAUSED, STOPPED,
 * HIBERNATED, CREATING, RESTORING, etc.) are never considered stale, and a
 * session with no recorded activity at all (e.g. brand new) is never
 * flagged either.
 */ function isSessionStale(session, thresholdMinutes) {
    if (session.status !== _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SessionStatus */ .Ph.ACTIVE) {
        return false;
    }
    const lastActivity = getLastActivityTimestamp(session);
    if (!lastActivity) {
        return false;
    }
    const lastActivityMs = Number(lastActivity.seconds) * 1000;
    return Date.now() - lastActivityMs > thresholdMinutes * 60000;
}


/***/ }),

/***/ 21091:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   d: () => (/* binding */ useKeyboard)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(88782);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_0__);

/**
 * Hook for managing keyboard shortcuts and navigation
 */ function useKeyboard(handlers, options = {}) {
    const { enabled = true, ignoreElements = [
        "INPUT",
        "TEXTAREA",
        "SELECT"
    ], requireModifier = false } = options;
    const handleKeyDown = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)((event)=>{
        if (!enabled) return;
        // Ignore if we're in an input element -- tagName covers INPUT/TEXTAREA/SELECT, but a
        // contenteditable div (e.g. a rich-text omnibar) has no matching tagName, so it needs
        // its own check too. Matches the precedent in shortcutRegistry.ts's isInputElement and
        // OmnibarContext.tsx's equivalent guard.
        //
        // Also ignore a focused native button/link/role="button" element -- these have their
        // own default keyboard activation (Enter/Space) the browser dispatches on the focused
        // element itself, which a single-letter global shortcut has no business intercepting.
        // Without this, registering e.g. an unscoped `Enter` handler here (page.tsx does, for
        // "open the focused session row") calls preventDefault() on every Enter keydown on the
        // page -- even when its own handler body no-ops -- silently breaking Enter-activation
        // for every other focused button on the page (found via a real, reproducibly-failing
        // scroll-forward-blocked-toast-keyboard.spec.ts case: Enter never dismissed the toast's
        // "Got it" button, Space did, because Space isn't a registered shortcut key here).
        // event.target can be the bare Document node itself (real apps never
        // do this, but a test dispatching keydown directly on `document` does)
        // -- Document has no getAttribute, so that call is optional-chained
        // rather than assumed to exist on every EventTarget.
        const target = event.target;
        const tag = target.tagName;
        if (ignoreElements.includes(tag) || target.isContentEditable || tag === "BUTTON" || tag === "A" || target.getAttribute?.("role") === "button") {
            return;
        }
        // Skip events generated by IME composition (e.g. CJK input methods)
        if (event.isComposing || event.keyCode === 229) return;
        // Build key identifier with modifiers
        const key = event.key;
        const hasModifier = event.ctrlKey || event.altKey || event.metaKey || event.shiftKey;
        // If requireModifier is true, skip unless a modifier is pressed
        if (requireModifier && !hasModifier) {
            return;
        }
        // Check if we have a handler for this key
        if (handlers[key]) {
            event.preventDefault();
            handlers[key]();
        }
    }, [
        enabled,
        handlers,
        ignoreElements,
        requireModifier
    ]);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        if (!enabled) return;
        window.addEventListener("keydown", handleKeyDown);
        return ()=>{
            window.removeEventListener("keydown", handleKeyDown);
        };
    }, [
        enabled,
        handleKeyDown
    ]);
}


/***/ }),

/***/ 32689:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   J: () => (/* binding */ useSessionActions)
/* harmony export */ });
/* harmony import */ var _useSessionService__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(82250);
/* __next_internal_client_entry_do_not_use__ useSessionActions auto */ 
/**
 * Thin adapter that binds all session-mutating service calls to a specific
 * sessionId. Eliminates the need for each component to thread the id through
 * every call site, and provides a single place to add cross-cutting concerns
 * (optimistic updates, analytics, etc.) in the future.
 */ function useSessionActions(sessionId) {
    const { pauseSession, resumeSession, resumeCrashedSession, deleteSession, renameSession, restartSession, createCheckpoint, updateSession, cancelSessionCreation, retrySessionCreation } = (0,_useSessionService__WEBPACK_IMPORTED_MODULE_0__/* .useSessionService */ .D)();
    return {
        pause: ()=>pauseSession(sessionId),
        resume: (updates)=>resumeSession(sessionId, updates),
        resumeFromCrash: ()=>resumeCrashedSession(sessionId),
        delete: (force)=>deleteSession(sessionId, force),
        rename: (title)=>renameSession(sessionId, title),
        restart: ()=>restartSession(sessionId),
        cancelCreation: ()=>cancelSessionCreation(sessionId),
        retryCreation: ()=>retrySessionCreation(sessionId),
        createCheckpoint: (label)=>createCheckpoint(sessionId, label),
        updateTags: (tags)=>updateSession(sessionId, {
                tags
            }),
        update: (updates)=>updateSession(sessionId, updates)
    };
}


/***/ }),

/***/ 44965:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  Wh: () => (/* binding */ StatusBadge),
  dD: () => (/* binding */ getAttentionReasonInfo)
});

// UNUSED EXPORTS: getDetectedStatusInfo

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ./src/gen/session/v1/types_pb.ts
var types_pb = __webpack_require__(29238);
// EXTERNAL MODULE: ./src/lib/utils/assertNever.ts
var assertNever = __webpack_require__(20440);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FStatusBadge.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA73US27DIBAG4L1PMctmQWScNzlMhGGCaQhQcJpHlbtXcZpWtKpttZK3PyO-YSRmvKHnsxPqkMNbBiB19IafGWhrtEWyNXhaZwDcaGWJrnEfGQi0NYZbrLhnMPVNiedSaqsYTPwJaH4PSxckBhK41IfIgBb3eOtsTaK-IIN8vJgF3H-mR9SqqhnM8_yWHStdI4meC2Rg3TFwv86u2fjRNm3aLrnYqeAOVjJ45eGJkA1d4aUIl6Ic3a4Rzrjw40yMvnpkQP0JojNafi-To4QsukhsIbf9SJWSk-FfOR2enHWRtoV0_UifkvMusmohdT_yOSUXXeSuhTT9yH1KLv8z2Jc_DXbVRYYWMvYj65Tkw__LsoNc_C7SXiCmnmi8ZHsuafHYn83Grj72J11n13eQ5WWV3QUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FStatusBadge_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA73US27DIBAG4L1PMctmQWScNzlMhGGCaQhQcJpHlbtXcZpWtKpttZK3PyO_YSRmvKHnsxPqkMNbBiB19IafGWhrtEWyNXhaZwDcaGWJrnEfGQi0NYZbrLhnMPVNiedSaqsYTPwJaH4PSxckBhK41IfIgBb3eOtsTaK_IIN8vJgF3H_mR9SqqhnM8_yWHStdI4meC2Rg3TFwv86u2fjRNm3aLrnYqeAOVjJ45eGJkA1d4aUIl6Ic3a4Rzrjw40yMvnpkQP0JojNafi_To4QsukhsIbf9SJWSk_FfOR2enHWRtoV0_UifkvMusmohdT_yOSUXXeSuhTT9yH1KLv8z2Jc_DXbVRYYWMvYj65Tkw_LsoNc_C7SXiCmnmi8ZHsuafHYn83Grj72J11n13eQ5WWV3QUAAA_22_7D = __webpack_require__(44234);
;// ./src/components/sessions/StatusBadge.css.ts


var badge = '_1yyocgu0';
var icon = '_1yyocguc';
var reasonVariants = {approval:'_1yyocgu1',input:'_1yyocgu2',error:'_1yyocgu3',testsFailing:'_1yyocgu4',idle:'_1yyocgu5',complete:'_1yyocgu6',uncommitted:'_1yyocgu7',stale:'_1yyocgu8',processing:'_1yyocgu9',active:'_1yyocgua',unknown:'_1yyocgub'};
;// ./src/components/sessions/StatusBadge.tsx
/* __next_internal_client_entry_do_not_use__ getAttentionReasonInfo,getDetectedStatusInfo,StatusBadge auto */ 



function getAttentionReasonInfo(reason) {
    switch(reason){
        case types_pb/* AttentionReason */.X9.APPROVAL_PENDING:
            return {
                label: "Approval Pending",
                icon: "🔒",
                variant: "approval"
            };
        case types_pb/* AttentionReason */.X9.INPUT_REQUIRED:
            return {
                label: "Input Required",
                icon: "✏️",
                variant: "input"
            };
        case types_pb/* AttentionReason */.X9.ERROR_STATE:
            return {
                label: "Error",
                icon: "⚠️",
                variant: "error"
            };
        case types_pb/* AttentionReason */.X9.IDLE_TIMEOUT:
        case types_pb/* AttentionReason */.X9.IDLE:
            return {
                label: "Idle",
                icon: "⏰",
                variant: "idle"
            };
        case types_pb/* AttentionReason */.X9.TASK_COMPLETE:
            return {
                label: "Complete",
                icon: "✅",
                variant: "complete"
            };
        case types_pb/* AttentionReason */.X9.UNCOMMITTED_CHANGES:
            return {
                label: "Uncommitted Changes",
                icon: "📝",
                variant: "uncommitted"
            };
        case types_pb/* AttentionReason */.X9.STALE:
            return {
                label: "Stale",
                icon: "⌛",
                variant: "stale"
            };
        case types_pb/* AttentionReason */.X9.WAITING_FOR_USER:
            return {
                label: "Your Input Needed",
                icon: "✏️",
                variant: "input"
            };
        default:
            return {
                label: "Unknown",
                icon: "●",
                variant: "unknown"
            };
    }
}
function getDetectedStatusInfo(status) {
    switch(status){
        case types_pb/* DetectedStatus */.nB.READY:
            return {
                label: "Ready",
                icon: "✅",
                variant: "complete"
            };
        case types_pb/* DetectedStatus */.nB.PROCESSING:
            return {
                label: "Processing",
                icon: "⚙️",
                variant: "processing"
            };
        case types_pb/* DetectedStatus */.nB.NEEDS_APPROVAL:
            return {
                label: "Needs Approval",
                icon: "🔒",
                variant: "approval"
            };
        case types_pb/* DetectedStatus */.nB.INPUT_REQUIRED:
            return {
                label: "Input Required",
                icon: "✏️",
                variant: "input"
            };
        case types_pb/* DetectedStatus */.nB.ERROR:
            return {
                label: "Error",
                icon: "⚠️",
                variant: "error"
            };
        case types_pb/* DetectedStatus */.nB.TESTS_FAILING:
            return {
                label: "Tests Failing",
                icon: "❌",
                variant: "testsFailing"
            };
        case types_pb/* DetectedStatus */.nB.IDLE:
            return {
                label: "Idle",
                icon: "⏰",
                variant: "idle"
            };
        case types_pb/* DetectedStatus */.nB.EXECUTING:
            return {
                label: "Executing",
                icon: "⚡",
                variant: "active"
            };
        case types_pb/* DetectedStatus */.nB.SUCCESS:
            return {
                label: "Success",
                icon: "✅",
                variant: "complete"
            };
        case types_pb/* DetectedStatus */.nB.WAITING_FOR_AGENT:
            return {
                label: "Waiting for Agent",
                icon: "⏳",
                variant: "processing"
            };
        case types_pb/* DetectedStatus */.nB.COMPACTING:
            return {
                label: "Compacting",
                icon: "⟳",
                variant: "processing"
            };
        case types_pb/* DetectedStatus */.nB.UNKNOWN:
            return null;
        case types_pb/* DetectedStatus */.nB.UNSPECIFIED:
            return null;
        default:
            return (0,assertNever/* assertNever */.x)(status);
    }
}
function StatusBadge({ reason, detectedStatus, title, context }) {
    let info;
    if (reason !== undefined) {
        info = getAttentionReasonInfo(reason);
    } else if (detectedStatus !== undefined) {
        info = getDetectedStatusInfo(detectedStatus);
    } else {
        return null;
    }
    if (info === null) {
        return null;
    }
    const tooltipText = context || title || info.label;
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
        className: `${badge} ${reasonVariants[info.variant]}`,
        role: "status",
        "aria-label": info.label,
        title: tooltipText,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                className: icon,
                "aria-hidden": "true",
                children: info.icon
            }),
            info.label
        ]
    });
}


/***/ }),

/***/ 45583:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  G: () => (/* binding */ SessionDetail)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
// EXTERNAL MODULE: ./src/lib/lazy/index.ts + 1 modules
var lazy = __webpack_require__(76937);
// EXTERNAL MODULE: ./src/lib/hooks/useSessionActions.ts
var useSessionActions = __webpack_require__(32689);
// EXTERNAL MODULE: ./src/lib/contexts/SessionVcsContext.tsx
var SessionVcsContext = __webpack_require__(2452);
// EXTERNAL MODULE: ./src/lib/hooks/useVcsStatus.ts
var useVcsStatus = __webpack_require__(37894);
// EXTERNAL MODULE: ./src/lib/config.ts
var config = __webpack_require__(99706);
// EXTERNAL MODULE: ./src/lib/store/index.ts
var store = __webpack_require__(92360);
// EXTERNAL MODULE: ./src/lib/store/sessionsSlice.ts
var sessionsSlice = __webpack_require__(45985);
// EXTERNAL MODULE: ./src/lib/contexts/notificationContexts.ts
var notificationContexts = __webpack_require__(50012);
// EXTERNAL MODULE: ./src/lib/contexts/FeatureFlagsContext.tsx
var FeatureFlagsContext = __webpack_require__(79577);
// EXTERNAL MODULE: ./src/lib/reply/replyQuestion.ts
var replyQuestion = __webpack_require__(83601);
;// ./src/lib/hooks/useStickyReplyQuestion.ts
/* __next_internal_client_entry_do_not_use__ useStickyReplyQuestion auto */ 
/**
 * Keeps the question a Reply card is showing after its notification turns read.
 * Sending a reply marks the notification read, which would otherwise unmount the
 * card and its receipt in the same render (RP-4: the receipt stays until the
 * question's state changes or the view closes). A different unread question
 * replaces it, and a different session clears it.
 */ function useStickyReplyQuestion(unread, sessionId) {
    const [held, setHeld] = (0,react.useState)({
        sessionId,
        question: unread
    });
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        setHeld((prev)=>{
            if (prev.sessionId !== sessionId) return {
                sessionId,
                question: unread
            };
            if (unread && unread.id !== prev.question?.id) return {
                sessionId,
                question: unread
            };
            return prev;
        });
    }, [
        unread,
        sessionId
    ]);
    if (held.sessionId !== sessionId) return unread;
    return unread ?? held.question;
}

// EXTERNAL MODULE: ./src/lib/hooks/useAnnounce.ts
var useAnnounce = __webpack_require__(88305);
;// ./src/components/sessions/readOnlyCopy.ts
// Copy for the hidden-session read-only view (plan Story 5.3, design/ux.md Surface 12).
const READ_ONLY_BANNER_PRIMARY = "Background session - read-only";
// One constant (ADV-N36): PR 5u added the unary guards (WriteToSession, the UpdateSession
// text fields, the steer branch, Restart and SwitchWorkspace), so the view may now say
// that nothing can be typed. Until PR 5u the copy read "Terminal input is disabled in
// this view." because only the stream was guarded.
const READ_ONLY_SECONDARY = "You can read output but not type.";
const READ_ONLY_SECONDARY_WITH_REPLY = "You can read output. Reply to Claude's question below; nothing else can be typed.";
function readOnlySecondaryText(replyCardPresent) {
    return replyCardPresent ? READ_ONLY_SECONDARY_WITH_REPLY : READ_ONLY_SECONDARY;
}
const READ_ONLY_REFUSAL_TOAST = "This session is read-only";
/** True for the server's guard refusal ("this session is a background session and is read-only"). */ function isReadOnlyRefusal(message) {
    return !!message && /background session.*read-only/i.test(message);
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FReadOnlyBanner.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll84cW9tbjIwIHsKICBmbGV4OiAwIDAgYXV0bzsKICBwb3NpdGlvbjogc3RpY2t5OwogIHRvcDogMDsKICB6LWluZGV4OiAxOwogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MnopOwogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MnopIHZhcigtLV8xOWV6MnJ6MzApOwogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6ZCk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7CiAgYm9yZGVyLWJvdHRvbTogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6Zyk7Cn0KLl84cW9tbjIxIHsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2MpOwogIGZsZXg6IDAgMCBhdXRvOwp9Ci5fOHFvbW4yMiB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIG1pbi13aWR0aDogMDsKfQouXzhxb21uMjMgewogIGZvbnQtd2VpZ2h0OiB2YXIoLS1fMTllejJyejNoKTsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2IpOwp9Ci5fOHFvbW4yNCB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FReadOnlyBanner_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll84cW9tbjIwIHsKICBmbGV4OiAwIDAgYXV0bzsKICBwb3NpdGlvbjogc3RpY2t5OwogIHRvcDogMDsKICB6LWluZGV4OiAxOwogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MnopOwogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MnopIHZhcigtLV8xOWV6MnJ6MzApOwogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6ZCk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7CiAgYm9yZGVyLWJvdHRvbTogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6Zyk7Cn0KLl84cW9tbjIxIHsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2MpOwogIGZsZXg6IDAgMCBhdXRvOwp9Ci5fOHFvbW4yMiB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIG1pbi13aWR0aDogMDsKfQouXzhxb21uMjMgewogIGZvbnQtd2VpZ2h0OiB2YXIoLS1fMTllejJyejNoKTsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2IpOwp9Ci5fOHFvbW4yNCB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKfQ_22_7D = __webpack_require__(19520);
;// ./src/components/sessions/ReadOnlyBanner.css.ts


var banner = '_8qomn20';
var icon = '_8qomn21';
var primary = '_8qomn23';
var ReadOnlyBanner_css_secondary = '_8qomn24';
var ReadOnlyBanner_css_text = '_8qomn22';
;// ./src/components/sessions/ReadOnlyBanner.tsx
/* __next_internal_client_entry_do_not_use__ ReadOnlyBanner auto */ 




/**
 * Banner for a hidden session's read-only view. Announced once on mount through
 * the Announcer; the element carries no live role of its own (RO-8).
 */ function ReadOnlyBanner({ replyCardPresent = false }) {
    const { announce } = (0,useAnnounce/* useAnnounce */.XE)();
    const secondary = readOnlySecondaryText(replyCardPresent);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        announce(`${READ_ONLY_BANNER_PRIMARY}. ${secondary}`, "polite", "readonly-banner");
    // On mount only: a copy change while open (Reply card appearing) is not re-announced.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: banner,
        "data-testid": "readonly-banner",
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                className: icon,
                "aria-hidden": "true",
                children: "\uD83D\uDC41"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: ReadOnlyBanner_css_text,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                        className: primary,
                        children: READ_ONLY_BANNER_PRIMARY
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                        className: ReadOnlyBanner_css_secondary,
                        "data-testid": "readonly-banner-secondary",
                        children: secondary
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/connect-error.js
var connect_error = __webpack_require__(22800);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/code.js
var esm_code = __webpack_require__(70792);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(4435);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/create.js
var create = __webpack_require__(7094);
// EXTERNAL MODULE: ./src/gen/session/v1/session_pb.ts
var session_pb = __webpack_require__(31381);
// EXTERNAL MODULE: ./src/lib/api/transport.ts
var transport = __webpack_require__(73396);
// EXTERNAL MODULE: ./src/lib/hooks/useNotificationConnectivity.ts + 1 modules
var useNotificationConnectivity = __webpack_require__(34607);
;// ./src/lib/hooks/useReplyToQuestion.ts
/* __next_internal_client_entry_do_not_use__ CHOOSING,REPLY_COPY,phaseForOutcome,phaseForError,announcementFor,useReplyToQuestion auto */ 






const CHOOSING = {
    phase: "choosing"
};
const REPLY_COPY = {
    noPending: "This question is no longer waiting. You may have answered it elsewhere.",
    stalePrompt: "This question is no longer on screen. You may have answered it already. Check the terminal.",
    notWaiting: "The question isn't showing yet - try again in a moment",
    invalid: "That option is not available. Check the terminal.",
    retry: "Could not send - Retry",
    indeterminate: "Sent? Check the terminal output to confirm",
    refused: "This device is not allowed to reply",
    offline: "Offline - reconnect to reply",
    footer: "Sent to the background session's terminal once. This reply is logged."
};
/** Maps a server outcome to the next phase. `now` is injected so tests do not read a clock. */ function phaseForOutcome(outcome, option, retryAfterSeconds, now) {
    switch(outcome){
        case session_pb/* ReplyOutcome */.jTN.SENT:
            return {
                phase: "sent",
                option,
                at: now
            };
        case session_pb/* ReplyOutcome */.jTN.SEND_INDETERMINATE:
            return {
                phase: "indeterminate",
                option
            };
        case session_pb/* ReplyOutcome */.jTN.NO_PENDING:
            return {
                phase: "noPending"
            };
        case session_pb/* ReplyOutcome */.jTN.STALE_PROMPT:
            return {
                phase: "stalePrompt"
            };
        case session_pb/* ReplyOutcome */.jTN.NOT_WAITING:
            return {
                phase: "notWaiting",
                option
            };
        case session_pb/* ReplyOutcome */.jTN.RATE_LIMITED:
            return {
                phase: "rateLimited",
                option,
                seconds: Math.max(1, retryAfterSeconds)
            };
        case session_pb/* ReplyOutcome */.jTN.NOT_HIDDEN:
        case session_pb/* ReplyOutcome */.jTN.DISABLED:
            return {
                phase: "hidden"
            };
        case session_pb/* ReplyOutcome */.jTN.NOT_SENT:
        default:
            return {
                phase: "retryable",
                option
            };
    }
}
/** Maps a thrown RPC error to a phase: only InvalidArgument and PermissionDenied are special. */ function phaseForError(err, option) {
    const code = connect_error/* ConnectError */.T.from(err).code;
    if (code === esm_code/* Code */.C.InvalidArgument) return {
        phase: "invalid"
    };
    if (code === esm_code/* Code */.C.PermissionDenied) return {
        phase: "refused"
    };
    return {
        phase: "retryable",
        option
    };
}
/** The assertive-or-polite announcement of a phase (the card has no live role). */ function announcementFor(p, optionLabel, time) {
    switch(p.phase){
        case "sent":
            return {
                text: `Sent: ${p.option}. ${optionLabel(p.option)} at ${time(p.at)} - the question closed`,
                politeness: "polite"
            };
        case "indeterminate":
            return {
                text: REPLY_COPY.indeterminate,
                politeness: "assertive"
            };
        case "retryable":
            return {
                text: REPLY_COPY.retry,
                politeness: "assertive"
            };
        case "noPending":
            return {
                text: REPLY_COPY.noPending,
                politeness: "polite"
            };
        case "stalePrompt":
            return {
                text: REPLY_COPY.stalePrompt,
                politeness: "assertive"
            };
        case "notWaiting":
            return {
                text: REPLY_COPY.notWaiting,
                politeness: "assertive"
            };
        case "invalid":
            return {
                text: REPLY_COPY.invalid,
                politeness: "assertive"
            };
        case "refused":
            return {
                text: REPLY_COPY.refused,
                politeness: "assertive"
            };
        case "rateLimited":
            return {
                text: `Please wait ${p.seconds} seconds`,
                politeness: "polite"
            };
        default:
            return null;
    }
}
const reducer = (_, a)=>a.phase;
function useReplyToQuestion(args) {
    const { sessionId, questionId, options, onSent } = args;
    const now = args.now ?? Date.now;
    const [state, dispatch] = (0,react.useReducer)(reducer, CHOOSING);
    const { isOffline } = (0,useNotificationConnectivity/* useNotificationConnectivity */.yO)();
    const { announce } = (0,useAnnounce/* useAnnounce */.XE)();
    const inFlight = (0,react.useRef)(false);
    const attempt = (0,react.useRef)(null);
    const announced = (0,react.useRef)(null);
    const send = args.sendReply ?? (async (req)=>{
        // Built on the first tap, so mounting the card creates no transport.
        const client = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
        // One-shot write from an explicit tap. It is deliberately not tied to the card's
        // lifetime: dropping the request on unmount could not take the digit back, and the
        // server answers a retried reply_id from its cached result.
        // abort-signal-exempt
        const res = await client.replyToPendingQuestion((0,create/* create */.v)(session_pb/* ReplyToPendingQuestionRequestSchema */.VeS, req));
        return {
            outcome: res.outcome,
            retryAfterSeconds: res.retryAfterSeconds
        };
    });
    const newId = args.newReplyId ?? (()=>crypto.randomUUID());
    const submit = (0,react.useCallback)(async (option, replyId)=>{
        inFlight.current = true;
        dispatch({
            type: "set",
            phase: {
                phase: "sending",
                option
            }
        });
        let next;
        try {
            const res = await send({
                sessionId,
                questionId,
                replyText: String(option),
                replyId
            });
            next = phaseForOutcome(res.outcome, option, res.retryAfterSeconds, now());
        } catch (err) {
            next = phaseForError(err, option);
        } finally{
            inFlight.current = false;
        }
        dispatch({
            type: "set",
            phase: next
        });
        if (next.phase === "sent") onSent?.();
    }, // send/newId close over stable args; the deps below are the ones that identify the question
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [
        sessionId,
        questionId,
        onSent
    ]);
    const choose = (0,react.useCallback)((option)=>{
        if (inFlight.current || isOffline) return; // the first tap disables every button (RP-3, RP-18)
        if (option < 1 || option > options.length) return;
        if (!attempt.current || attempt.current.option !== option) attempt.current = {
            replyId: newId(),
            option
        };
        void submit(option, attempt.current.replyId);
    }, // eslint-disable-next-line react-hooks/exhaustive-deps
    [
        isOffline,
        options.length,
        submit
    ]);
    const retry = (0,react.useCallback)(()=>{
        if (inFlight.current || isOffline || !attempt.current) return;
        void submit(attempt.current.option, attempt.current.replyId);
    }, [
        isOffline,
        submit
    ]);
    // Rate limited: keep the options, re-enable them when the wait ends.
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (state.phase !== "rateLimited") return;
        const t = setTimeout(()=>dispatch({
                type: "set",
                phase: CHOOSING
            }), state.seconds * 1000);
        return ()=>clearTimeout(t);
    }, [
        state
    ]);
    // One announcement per phase change, through the Announcer (the card has no live role).
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (announced.current === state) return;
        announced.current = state;
        const a = announcementFor(state, (n)=>options[n - 1] ?? "", (ms)=>new Date(ms).toLocaleTimeString([], {
                hour: "2-digit",
                minute: "2-digit"
            }));
        if (a) announce(a.text, a.politeness, `reply:${questionId}`);
    }, [
        state,
        options,
        questionId,
        announce
    ]);
    return {
        state,
        offline: isOffline,
        choose,
        retry
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FReplyCard.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61T247aMBB95yvmbZeqRg4LbNdov6SqViYeEquO7R07JFD136uYFBGgK6Lu61zOzDlnZvaWzZcrVVkOvyYAW4OtAA4cZB3degJQyZaVqIsyCsilyR93kh4Z22lsvKPY575CxrnalVP4Any2nHadboe0Na5he3FCUzp4I_ciDVr3A5nShHnUzgrInakr22UK6QUch71lL3iY02F-SMBeKqVtcZl94tPrSFe_kfnPglxt1WXLc8rnzji6TPWtjhQS27gYXSUg8y0EZ7S6KMbpevJ7MvurZZa0rCQV2grgiaezkQV9wKut8-kp3_Q6X1SUQ_T5GPRNQm9KHZEFL3MU4AlZQ9IPPOoCAqTdNyUSDuY9pXnS6MKygGZ7NI-FKCmmC9H2dCGLhW8HFnHI5sfQuQuRpA1eEtp4lwEfK3-yiaTSdRCwOk7MawodrnfaRqQBqUUi9QnneIa5PBrzkRwZ9y1kfTRiG1nSVYDBbbzHyX_f8st9t_z5Ui6F0kFuDKrE33mZ67gXwGer81brOrLGNaiG7d8lack8YQioXh8i1fjwI0H1u9zkVI19m9XYtznrfR7_cjeXzoaw3_7zs1bXn3VLpJurvA9uwjqLY9z_A2ZDnlw3BgAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FReplyCard_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61T247aMBB95yvmbZeqRg4LbNdov6SqViYeEquO7R07JFD136uYFBGgK6Lu61zOzDlnZvaWzZcrVVkOvyYAW4OtAA4cZB3degJQyZaVqIsyCsilyR93kh4Z22lsvKPY575CxrnalVP4Any2nHadboe0Na5he3FCUzp4I_ciDVr3A5nShHnUzgrInakr22UK6QUch71lL3iY02F_SMBeKqVtcZl94tPrSFe_kfnPglxt1WXLc8rnzji6TPWtjhQS27gYXSUg8y0EZ7S6KMbpevJ7MvurZZa0rCQV2grgiaezkQV9wKut8_kp3_Q6X1SUQ_T5GPRNQm9KHZEFL3MU4AlZQ9IPPOoCAqTdNyUSDuY9pXnS6MKygGZ7NI_FKCmmC9H2dCGLhW8HFnHI5sfQuQuRpA1eEtp4lwEfK3_yiaTSdRCwOk7MawodrnfaRqQBqUUi9QnneIa5PBrzkRwZ9y1kfTRiG1nSVYDBbbzHyX_f8st9t_z5Ui6F0kFuDKrE33mZ67gXwGer81brOrLGNaiG7d8lack8YQioXh8i1fjwI0H1u9zkVI19m9XYtznrfR7_cjeXzoaw3_7zs1bXn3VLpJurvA9uwjqLY9z_A2ZDnlw3BgAA_22_7D = __webpack_require__(59471);
;// ./src/components/sessions/ReplyCard.css.ts


var actionButton = '_1256dmn8';
var card = '_1256dmn0';
var disclosure = '_1256dmn3';
var footer = '_1256dmn7';
var heading = '_1256dmn1';
var ReplyCard_css_option = '_1256dmn5';
var ReplyCard_css_options = '_1256dmn4';
var ReplyCard_css_prompt = '_1256dmn2';
var ReplyCard_css_status = '_1256dmn6';
;// ./src/components/sessions/ReplyCard.tsx
// +feature: session-reply-card
/* __next_internal_client_entry_do_not_use__ ReplyCard auto */ 




const NEEDS_VIEW_OUTPUT = new Set([
    "indeterminate",
    "stalePrompt",
    "invalid"
]);
/**
 * Audited one-tap answer to a hidden session's pending single-select question
 * (ADR-010). One numbered button per option label; no text field, so no soft
 * keyboard, IME or Enter path exists (RP-2, RP-12, RP-18). Anything the server
 * cannot answer renders "Answer in the terminal".
 */ function ReplyCard({ sessionId, notification, onSent, onViewOutput, hookOverrides }) {
    const availability = (0,replyQuestion/* replyAvailability */.wR)(notification);
    if (!availability) return null;
    if (availability.kind === "unavailable") {
        return /*#__PURE__*/ (0,react_jsx_runtime.jsx)(UnavailableCard, {
            copy: (0,replyQuestion/* unavailableCopy */.MN)(availability.cause),
            onViewOutput: onViewOutput
        });
    }
    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)(ReplyChoices, {
        sessionId: sessionId,
        questionId: availability.questionId,
        options: availability.options,
        prompt: availability.prompt,
        onSent: onSent,
        onViewOutput: onViewOutput,
        hookOverrides: hookOverrides
    });
}
function UnavailableCard({ copy, onViewOutput }) {
    const headingId = (0,react.useId)();
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("section", {
        className: card,
        role: "group",
        "aria-labelledby": headingId,
        "data-testid": "reply-card",
        "data-state": "unavailable",
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("h2", {
                id: headingId,
                className: heading,
                children: "Claude has a question"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                className: ReplyCard_css_status,
                "data-testid": "reply-unavailable",
                children: copy
            }),
            onViewOutput && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                type: "button",
                className: actionButton,
                onClick: onViewOutput,
                "data-testid": "reply-view-output",
                children: "View output"
            })
        ]
    });
}
function ReplyChoices({ sessionId, questionId, options, prompt, onSent, onViewOutput, hookOverrides }) {
    const headingId = (0,react.useId)();
    const [expanded, setExpanded] = (0,react.useState)(false);
    const reply = useReplyToQuestion({
        sessionId,
        questionId,
        options,
        onSent,
        ...hookOverrides
    });
    const { state } = reply;
    const { shown, truncated } = (0,replyQuestion/* truncatePrompt */.rb)(prompt);
    if (state.phase === "hidden") return null; // NOT_HIDDEN and DISABLED: the card goes away
    const choosing = state.phase === "choosing" || state.phase === "retryable" || state.phase === "notWaiting" || state.phase === "rateLimited";
    const settled = state.phase === "sent" || state.phase === "indeterminate";
    const buttonsDisabled = state.phase === "sending" || state.phase === "rateLimited" || reply.offline;
    const chosen = "option" in state ? state.option : undefined;
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("section", {
        className: card,
        role: "group",
        "aria-labelledby": headingId,
        "aria-busy": state.phase === "sending" ? "true" : undefined,
        "data-testid": "reply-card",
        "data-state": state.phase,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("h2", {
                id: headingId,
                className: heading,
                children: "Claude has a question"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("p", {
                className: ReplyCard_css_prompt,
                "data-testid": "reply-prompt",
                children: [
                    expanded ? prompt : shown,
                    truncated && !expanded ? "…" : ""
                ]
            }),
            truncated && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                type: "button",
                className: disclosure,
                "aria-expanded": expanded,
                onClick: ()=>setExpanded((v)=>!v),
                "data-testid": "reply-show-full",
                children: expanded ? "Hide full prompt" : "Show full prompt"
            }),
            (choosing || state.phase === "sending" || settled) && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                className: ReplyCard_css_options,
                children: options.map((label, i)=>{
                    const n = i + 1;
                    if (state.phase === "sent" && n !== chosen) return null; // buttons are removed on success
                    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("button", {
                        type: "button",
                        className: ReplyCard_css_option,
                        "data-testid": `reply-option-${n}`,
                        disabled: buttonsDisabled || settled,
                        "aria-pressed": chosen === n ? "true" : undefined,
                        onClick: ()=>reply.choose(n),
                        children: [
                            n,
                            ". ",
                            label
                        ]
                    }, n);
                })
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(Status, {
                state: state,
                options: options,
                offline: reply.offline,
                onRetry: reply.retry
            }),
            onViewOutput && NEEDS_VIEW_OUTPUT.has(state.phase) && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                type: "button",
                className: actionButton,
                onClick: onViewOutput,
                "data-testid": "reply-view-output",
                children: "View output"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                className: footer,
                children: REPLY_COPY.footer
            })
        ]
    });
}
function Status({ state, options, offline, onRetry }) {
    const retryButton = /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
        type: "button",
        className: actionButton,
        onClick: onRetry,
        disabled: offline,
        "data-testid": "reply-retry",
        children: "Retry"
    });
    if (offline && state.phase !== "sent" && state.phase !== "indeterminate") {
        return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
            className: ReplyCard_css_status,
            "data-testid": "reply-status",
            children: REPLY_COPY.offline
        });
    }
    switch(state.phase){
        case "sent":
            return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("p", {
                className: ReplyCard_css_status,
                "data-testid": "reply-receipt",
                tabIndex: -1,
                ref: (el)=>el?.focus(),
                children: [
                    "Sent: ",
                    state.option,
                    ". ",
                    options[state.option - 1],
                    " at",
                    " ",
                    new Date(state.at).toLocaleTimeString([], {
                        hour: "2-digit",
                        minute: "2-digit"
                    }),
                    " - the question closed"
                ]
            });
        case "indeterminate":
            return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                className: ReplyCard_css_status,
                "data-testid": "reply-status",
                children: REPLY_COPY.indeterminate
            });
        case "noPending":
            return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                className: ReplyCard_css_status,
                "data-testid": "reply-status",
                children: REPLY_COPY.noPending
            });
        case "stalePrompt":
            return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                className: ReplyCard_css_status,
                "data-testid": "reply-status",
                children: REPLY_COPY.stalePrompt
            });
        case "invalid":
            return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                className: ReplyCard_css_status,
                "data-testid": "reply-status",
                children: REPLY_COPY.invalid
            });
        case "refused":
            return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                className: ReplyCard_css_status,
                "data-testid": "reply-status",
                children: REPLY_COPY.refused
            });
        case "notWaiting":
            return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                        className: ReplyCard_css_status,
                        "data-testid": "reply-status",
                        children: REPLY_COPY.notWaiting
                    }),
                    retryButton
                ]
            });
        case "retryable":
            return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                        className: ReplyCard_css_status,
                        "data-testid": "reply-status",
                        children: REPLY_COPY.retry.replace(" - Retry", "")
                    }),
                    retryButton
                ]
            });
        case "rateLimited":
            return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("p", {
                className: ReplyCard_css_status,
                "data-testid": "reply-status",
                children: [
                    "Please wait ",
                    state.seconds,
                    " seconds"
                ]
            });
        default:
            return null;
    }
}

;// ./src/components/sessions/SessionDetail.tsx
/* __next_internal_client_entry_do_not_use__ SessionDetail auto */ 















// Dynamically import SessionDetailView (and its heavy transitive deps: CodeMirror,
// XtermTerminal, syntax-highlight packs, WASM) so they are NOT in the initial bundle.
// This defers ~2MB of JS parse/eval until the user actually opens a session.
const SessionDetailView = (0,lazy/* lazyComponent */._)(()=>Promise.all(/* import() */[__webpack_require__.e(3746), __webpack_require__.e(2802), __webpack_require__.e(3258), __webpack_require__.e(9394), __webpack_require__.e(3595), __webpack_require__.e(1911), __webpack_require__.e(8809), __webpack_require__.e(8130), __webpack_require__.e(8332), __webpack_require__.e(3370), __webpack_require__.e(1692), __webpack_require__.e(201), __webpack_require__.e(673), __webpack_require__.e(1953)]).then(__webpack_require__.bind(__webpack_require__, 11953)).then((m)=>({
            default: m.SessionDetailView
        })), {
    loading: ()=>/*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
            style: {
                padding: "20px",
                textAlign: "center",
                color: "var(--text-secondary)"
            },
            children: "Loading..."
        })
});
function SessionDetail({ session, onClose, onFullscreenChange, onTabChange, initialTab = "info", embedded = false, onNext, onPrevious, showNavigation = false, onApprovalResolved, onDismissFromQueue, queuePosition, queueTotal, nextSessionName, previousSessionName, onBack, canGoBack, backlogItemId, backlogEntry, isActive }) {
    const actions = (0,useSessionActions/* useSessionActions */.J)(session.id);
    const allSessions = (0,store/* useAppSelector */.GV)(sessionsSlice/* selectAllSessions */.rt);
    const sessionsError = (0,store/* useAppSelector */.GV)(sessionsSlice/* selectSessionsError */.AY);
    const dispatch = (0,store/* useAppDispatch */.jL)();
    const { showActionToast, markAsRead } = (0,notificationContexts/* useNotificationCommands */.RK)();
    const { notificationHistory } = (0,notificationContexts/* useNotificationState */.IB)();
    // Kill switch: the server's hidden_session_reply flag (a missing value reads as off).
    const replyEnabled = (0,FeatureFlagsContext/* useFeatureFlag */.u)("hidden_session_reply");
    const unreadQuestion = session.hidden && replyEnabled ? (0,replyQuestion/* pendingQuestionFor */.xb)(notificationHistory, session.id) : undefined;
    // Marking the notification read after a send must not unmount the card holding its receipt.
    const stickyQuestion = useStickyReplyQuestion(unreadQuestion, session.id);
    const pendingQuestion = session.hidden && replyEnabled ? stickyQuestion : undefined;
    // A write the server refused on a hidden session reads as one plain line, never the raw RPC error.
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (!session.hidden || !isReadOnlyRefusal(sessionsError)) return;
        showActionToast(READ_ONLY_REFUSAL_TOAST, "error", "readonly-refusal");
        dispatch((0,sessionsSlice/* setError */.Nh)(null));
    }, [
        session.hidden,
        sessionsError,
        showActionToast,
        dispatch
    ]);
    // Prefetch VCS data as soon as a session is selected so tabs load instantly.
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        const baseUrl = (0,config/* getApiBaseUrl */.h)();
        (0,useVcsStatus/* prefetchVcsStatus */.U)(session.id, baseUrl);
    }, [
        session.id
    ]);
    const detailView = /*#__PURE__*/ (0,react_jsx_runtime.jsx)(SessionDetailView, {
        session: session,
        allSessions: allSessions,
        actions: actions,
        onClose: onClose,
        onFullscreenChange: onFullscreenChange,
        onTabChange: onTabChange,
        initialTab: initialTab,
        embedded: embedded,
        onNext: onNext,
        onPrevious: onPrevious,
        showNavigation: showNavigation,
        onApprovalResolved: onApprovalResolved,
        onDismissFromQueue: onDismissFromQueue,
        queuePosition: queuePosition,
        queueTotal: queueTotal,
        nextSessionName: nextSessionName,
        previousSessionName: previousSessionName,
        onBack: onBack,
        canGoBack: canGoBack,
        backlogItemId: backlogItemId,
        isActive: isActive,
        backlogEntry: backlogEntry
    });
    // Read-only is decided from session.hidden (the server enforces the same rule, Story 5.1).
    // The wrapper exists only for hidden sessions so a visible session's layout is untouched.
    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)(SessionVcsContext/* SessionVcsProvider */.m, {
        sessionId: session.id,
        baseUrl: (0,config/* getApiBaseUrl */.h)(),
        isActive: isActive,
        children: session.hidden ? /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
            style: {
                display: "flex",
                flexDirection: "column",
                flex: 1,
                height: "100%",
                minHeight: 0
            },
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)(ReadOnlyBanner, {
                    replyCardPresent: pendingQuestion !== undefined
                }),
                pendingQuestion && /*#__PURE__*/ (0,react_jsx_runtime.jsx)(ReplyCard, {
                    sessionId: session.id,
                    notification: pendingQuestion,
                    onSent: ()=>void markAsRead(pendingQuestion.id),
                    onViewOutput: ()=>onTabChange?.("terminal")
                }, pendingQuestion.id),
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                    style: {
                        flex: 1,
                        minHeight: 0,
                        display: "flex",
                        flexDirection: "column"
                    },
                    children: detailView
                })
            ]
        }) : detailView
    });
}


/***/ }),

/***/ 56193:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  b: () => (/* binding */ ReviewQueueBadge)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ./src/gen/session/v1/types_pb.ts
var types_pb = __webpack_require__(29238);
// EXTERNAL MODULE: ./src/components/sessions/StatusBadge.tsx + 1 modules
var StatusBadge = __webpack_require__(44965);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FReviewQueueBadge.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA71UwW7qMBC88xV7qQTSM3JSCtS99E8qE28SPxzbsp2SUPXfn-xAGygFTu8S2buTmdlx4tct9qXjDXrwjwseegofEwD68AcySh_SBsBYXsjQM8heJgCfE4Cnnz06Xw3dz8l84MoSREhvFe8ZlAq7iKi4ZbC2ac2VrDSRARvPoEAd0MVyhJKdi8D4fPnmzE85tdGYXjA6EC_3yCCjA3Uq7VBWdWCwojTWFIaAjvjoWVfRM82xGdE_ntJLraRGcnT-i9u_rQ-y7ElhdEAdxq2dFKFmkC8GT_XBznG_MU6gI44L2XoWUz0f5gAsWueNY1CjsnEfHNdeBmk0G9alcQ3Qee4BucfxRKw27-jSXF9IBr7gCqfZPJ-NsIuEslyIlM7CdpDlF50ey2Or-YXcl0PuAbtARuqtteiKZBRgV8uA6UwwHujZgT9d_oh-OYr0baXMvgiWiWDDi23lTKsFg3fupoS8Zc-4z90-38xSwkbFgM96xex7eAaZ7cAbJcU5TIxTXN0S3F4RVPcJNmPB9S1BvCJY3idYjQWfbwnqK4LmPkE7FuT_TfC1QSE5TBupyeHnXS3XtpslByeX0I974nD9_QPEHF_8VAUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FReviewQueueBadge_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA71UwW7qMBC88xV7qQTSM3JSCtS99E8qE28SPxzbsp2SUPXfn_xAGygFTu8S2buTmdlx4tct9qXjDXrwjwseegofEwD68AcySh_SBsBYXsjQM8heJgCfE4Cnnz06Xw3dz8l84MoSREhvFe8ZlAq7iKi4ZbC2ac2VrDSRARvPoEAd0MVyhJKdi8D4fPnmzE85tdGYXjA6EC_3yCCjA3Uq7VBWdWCwojTWFIaAjvjoWVfRM82xGdE_ntJLraRGcnT_i9u_rQ_y7ElhdEAdxq2dFKFmkC8GT_XBznG_MU6gI44L2XoWUz0f5gAsWueNY1CjsnEfHNdeBmk0G9alcQ3Qee4BucfxRKw27_jSXF9IBr7gCqfZPJ_NsIuEslyIlM7CdpDlF50ey2Or_YXcl0PuAbtARuqtteiKZBRgV8uA6UwwHujZgT9d_oh_OYr0baXMvgiWiWDDi23lTKsFg3fupoS8Zc_4z90_38xSwkbFgM96xex7eAaZ7cAbJcU5TIxTXN0S3F4RVPcJNmPB9S1BvCJY3idYjQWfbwnqK4LmPkE7FuT_TfC1QSE5TBupyeHnXS3XtpslByeX0I974nD9_QPEHF_8VAUAAA_22_7D = __webpack_require__(12647);
;// ./src/components/sessions/ReviewQueueBadge.css.ts


var badge = 's34aty1';
var badgeCompact = 's34aty3';
var ReviewQueueBadge_css_priority = 's34aty5 s34aty4';
var priorityAbbr = 's34aty2';
var priorityHigh = 's34aty7';
var priorityLow = 's34aty9';
var priorityMedium = 's34aty8';
var priorityUnspecified = 's34atya';
var priorityUrgent = 's34aty6';
;// ./src/components/sessions/ReviewQueueBadge.tsx
/* __next_internal_client_entry_do_not_use__ ReviewQueueBadge auto */ 



/**
 * Badge component that displays priority and attention reason for review queue items.
 *
 * Shows visual indicators using emojis and colors to communicate urgency and context.
 */ function ReviewQueueBadge({ priority, reason, compact = false }) {
    const getPriorityEmoji = (p)=>{
        switch(p){
            case types_pb/* Priority */.bx.URGENT:
                return "🔴";
            case types_pb/* Priority */.bx.HIGH:
                return "🟡";
            case types_pb/* Priority */.bx.MEDIUM:
                return "🔵";
            case types_pb/* Priority */.bx.LOW:
                return "⚪";
            default:
                return "⚫";
        }
    };
    const getPriorityAbbr = (p)=>{
        switch(p){
            case types_pb/* Priority */.bx.URGENT:
                return "URG";
            case types_pb/* Priority */.bx.HIGH:
                return "HIGH";
            case types_pb/* Priority */.bx.MEDIUM:
                return "MED";
            case types_pb/* Priority */.bx.LOW:
                return "LOW";
            default:
                return "";
        }
    };
    const getPriorityClass = (p)=>{
        switch(p){
            case types_pb/* Priority */.bx.URGENT:
                return priorityUrgent;
            case types_pb/* Priority */.bx.HIGH:
                return priorityHigh;
            case types_pb/* Priority */.bx.MEDIUM:
                return priorityMedium;
            case types_pb/* Priority */.bx.LOW:
                return priorityLow;
            default:
                return priorityUnspecified;
        }
    };
    const getPriorityText = (p)=>{
        switch(p){
            case types_pb/* Priority */.bx.URGENT:
                return "Urgent";
            case types_pb/* Priority */.bx.HIGH:
                return "High";
            case types_pb/* Priority */.bx.MEDIUM:
                return "Medium";
            case types_pb/* Priority */.bx.LOW:
                return "Low";
            default:
                return "Unknown";
        }
    };
    const reasonLabel = (0,StatusBadge/* getAttentionReasonInfo */.dD)(reason).label;
    if (compact) {
        const abbr = getPriorityAbbr(priority);
        return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
            className: `${badgeCompact} ${getPriorityClass(priority)}`,
            title: `${getPriorityText(priority)}: ${reasonLabel}`,
            "aria-label": `${getPriorityText(priority)} priority: ${reasonLabel}`,
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                    "aria-hidden": "true",
                    children: getPriorityEmoji(priority)
                }),
                abbr && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                    className: priorityAbbr,
                    "aria-hidden": "true",
                    children: abbr
                })
            ]
        });
    }
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: badge,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                className: `${ReviewQueueBadge_css_priority} ${getPriorityClass(priority)}`,
                "aria-label": `Priority: ${getPriorityText(priority)}`,
                children: [
                    getPriorityEmoji(priority),
                    " ",
                    getPriorityText(priority)
                ]
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(StatusBadge/* StatusBadge */.Wh, {
                reason: reason
            })
        ]
    });
}


/***/ }),

/***/ 65637:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Bv: () => (/* binding */ PINNED_GROUP_KEY),
/* harmony export */   Ln: () => (/* binding */ groupSessions),
/* harmony export */   O3: () => (/* binding */ groupWithPinned),
/* harmony export */   YK: () => (/* binding */ GroupingStrategyLabels),
/* harmony export */   j$: () => (/* binding */ GroupingStrategy),
/* harmony export */   nd: () => (/* binding */ cycleGroupingStrategy)
/* harmony export */ });
/* harmony import */ var _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(29238);
/* harmony import */ var _session_staleness__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(15141);


/**
 * Grouping strategy options for organizing sessions.
 * Mirrors the TUI grouping strategies in ui/list.go
 */ var GroupingStrategy = /*#__PURE__*/ function(GroupingStrategy) {
    GroupingStrategy["Category"] = "category";
    GroupingStrategy["Tag"] = "tag";
    GroupingStrategy["Branch"] = "branch";
    GroupingStrategy["Path"] = "path";
    GroupingStrategy["Program"] = "program";
    GroupingStrategy["Status"] = "status";
    GroupingStrategy["SessionType"] = "session_type";
    GroupingStrategy["Project"] = "project";
    GroupingStrategy["Workflow"] = "workflow";
    GroupingStrategy["Stale"] = "stale";
    GroupingStrategy["None"] = "none";
    return GroupingStrategy;
}({});
const GroupingStrategyLabels = {
    ["category"]: "Category",
    ["tag"]: "Tags",
    ["branch"]: "Branch",
    ["path"]: "Path",
    ["program"]: "Program",
    ["status"]: "Status",
    ["session_type"]: "Session Type",
    ["project"]: "Project",
    ["workflow"]: "Workflow",
    ["stale"]: "Stale",
    ["none"]: "None (Flat List)"
};
const PINNED_GROUP_KEY = "__pinned__";
/**
 * Groups sessions by strategy, with pinned sessions lifted into a leading
 * "Pinned" group (above every strategy) and removed from their normal groups.
 * Groups left empty by the removal are dropped.
 */ function groupWithPinned(sessions, group) {
    const isPinned = (s)=>s.pinned && !s.archivedAt;
    const pinned = sessions.filter(isPinned);
    if (pinned.length === 0) return group(sessions);
    const rest = sessions.filter((s)=>!isPinned(s));
    return [
        {
            groupKey: PINNED_GROUP_KEY,
            displayName: "Pinned",
            sessions: pinned
        },
        ...group(rest)
    ];
}
function groupSessions(sessions, strategy, options) {
    // Early exit: No grouping returns single flat group
    if (strategy === "none") {
        return [
            {
                groupKey: "all",
                displayName: "All Sessions",
                sessions: [
                    ...sessions
                ]
            }
        ];
    }
    // Map for O(1) group lookups and membership checks
    const grouped = new Map();
    // Phase 1: Extract group keys and build membership map
    sessions.forEach((session)=>{
        let groupKeys = [];
        // Extract group keys based on selected strategy
        // Most strategies return single group (single-membership)
        // Tag strategy returns multiple groups (multi-membership)
        switch(strategy){
            case "category":
                // Single-membership: One category per session
                groupKeys = [
                    session.category || "Uncategorized"
                ];
                break;
            case "tag":
                // Multi-membership: Session appears in ALL its tag groups
                // Example: session with tags ["Frontend", "React"] appears in both groups
                // This enables multi-dimensional organization without duplication
                if (session.tags && session.tags.length > 0) {
                    groupKeys = session.tags;
                } else {
                    groupKeys = [
                        "Untagged"
                    ];
                }
                break;
            case "branch":
                // Single-membership: One branch per session
                groupKeys = [
                    session.branch || "No Branch"
                ];
                break;
            case "path":
                // Single-membership: One path per session. activeDir, not path, so
                // worktree sessions group by where they actually run.
                groupKeys = [
                    session.activeDir || "No Path"
                ];
                break;
            case "program":
                // Single-membership: One program per session
                groupKeys = [
                    session.program || "No Program"
                ];
                break;
            case "status":
                // Single-membership: One status per session
                groupKeys = [
                    getStatusDisplayName(session.status)
                ];
                break;
            case "session_type":
                // Single-membership: One type per session
                groupKeys = [
                    getSessionTypeDisplayName(session.sessionType)
                ];
                break;
            case "project":
                // Single-membership: One project per session (empty project_id = No Project)
                groupKeys = [
                    session.projectId || "No Project"
                ];
                break;
            case "workflow":
                // Single-membership: One workflow per session; manual sessions go into "Manual Sessions"
                if (session.workflowId) {
                    const wfName = options?.workflowIdToName?.get(session.workflowId) ?? session.workflowId;
                    groupKeys = [
                        wfName
                    ];
                } else {
                    groupKeys = [
                        "Manual Sessions"
                    ];
                }
                break;
            case "stale":
                // Single-membership: "Stale" only for ACTIVE sessions past the idle threshold;
                // everything else (including non-ACTIVE sessions) falls into "Not Stale" rather
                // than a misleading "Active" bucket.
                groupKeys = [
                    (0,_session_staleness__WEBPACK_IMPORTED_MODULE_1__/* .isSessionStale */ .G)(session, options?.thresholdMinutes ?? 30) ? "Stale" : "Not Stale"
                ];
                break;
            default:
                // Fallback for unknown strategies
                groupKeys = [
                    "Uncategorized"
                ];
        }
        // Phase 2: Add session to all its groups (enables multi-membership)
        // For most strategies, this adds to single group
        // For Tag strategy, this adds to multiple groups
        groupKeys.forEach((key)=>{
            if (!grouped.has(key)) {
                grouped.set(key, []);
            }
            // Session reference is shared across groups (no cloning for efficiency)
            grouped.get(key).push(session);
        });
    });
    // Phase 3: Convert Map to sorted array
    // Sort logic ensures consistent ordering with special groups at the end
    const sortedGroups = Array.from(grouped.keys()).sort((a, b)=>{
        // Special groups (empty/missing fields) always appear at the end
        const specialGroups = [
            "Uncategorized",
            "Untagged",
            "No Branch",
            "No Path",
            "No Program",
            "No Project",
            "Manual Sessions"
        ];
        if (specialGroups.includes(a)) return 1;
        if (specialGroups.includes(b)) return -1;
        // For Status grouping: enforce preferred order so active groups appear first.
        // "Paused" appears after active/working groups, before terminal/unknown states.
        const statusOrder = {
            "Active": 0,
            "Ready": 1,
            "Loading": 2,
            "Creating": 2,
            "Needs Approval": 3,
            "Paused": 4,
            "Stopped": 5,
            "Hibernated": 6,
            "Unknown": 7
        };
        const aOrder = statusOrder[a] ?? 99;
        const bOrder = statusOrder[b] ?? 99;
        if (aOrder !== bOrder) return aOrder - bOrder;
        // Regular groups sorted alphabetically as fallback
        return a.localeCompare(b);
    });
    // Phase 4: Build final result array with metadata
    return sortedGroups.map((key)=>({
            groupKey: key,
            displayName: key,
            sessions: grouped.get(key)
        }));
}
/**
 * Get human-readable status display name
 */ function getStatusDisplayName(status) {
    switch(status){
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SessionStatus */ .Ph.ACTIVE:
            return "Active"; // 1 (covers legacy RUNNING alias)
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SessionStatus */ .Ph.READY:
            return "Ready"; // 2 (deprecated)
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SessionStatus */ .Ph.LOADING:
            return "Loading"; // 3 (deprecated)
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SessionStatus */ .Ph.PAUSED:
            return "Paused"; // 4
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SessionStatus */ .Ph.NEEDS_APPROVAL:
            return "Needs Approval"; // 5 (deprecated)
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SessionStatus */ .Ph.CREATING:
            return "Creating"; // 6
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SessionStatus */ .Ph.STOPPED:
            return "Stopped"; // 7
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SessionStatus */ .Ph.HIBERNATED:
            return "Hibernated"; // 8
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SessionStatus */ .Ph.CRASHED:
            return "Crashed"; // 10
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SessionStatus */ .Ph.PERMANENTLY_FAILED:
            return "Failed"; // 11
        default:
            return "Unknown";
    }
}
/**
 * Get human-readable session type display name
 */ function getSessionTypeDisplayName(sessionType) {
    switch(sessionType){
        case 1:
            return "Directory";
        case 2:
            return "New Worktree";
        case 3:
            return "Existing Worktree";
        default:
            return "Unknown";
    }
}
/**
 * Get the next grouping strategy in the cycle.
 * Mirrors the TUI CycleGroupingStrategy() method in ui/list.go
 */ function cycleGroupingStrategy(current) {
    const strategies = Object.values(GroupingStrategy);
    const currentIndex = strategies.indexOf(current);
    const nextIndex = (currentIndex + 1) % strategies.length;
    return strategies[nextIndex];
}


/***/ }),

/***/ 88649:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  aF: () => (/* binding */ Modal),
  Oj: () => (/* binding */ ModalClose),
  $m: () => (/* binding */ ModalContent),
  mw: () => (/* binding */ ModalDescription),
  jl: () => (/* binding */ ModalFooter),
  wt: () => (/* binding */ ModalTitle)
});

// UNUSED EXPORTS: ModalTrigger

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@radix-ui+react-dialog@1.1.15+b721ee6ccd61acaa/node_modules/@radix-ui/react-dialog/dist/index.mjs + 25 modules
var dist = __webpack_require__(97866);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fui%2FModal.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA41U226jMBB9z1fMS6VEW1dAljRxX_onKwcG4hY8yDaEZJV_X2EgoQSSfUFjn2HOXM748xtPiRY5GkiPG8wPHvxdACSacmcAUCEiaU8cvI8FwGUBYGkM-S10WXyOw_mPwwFYLZRJSOe8NTNhcclC7-UVIpFFzoRfsC3q1epJAg-DNd9Vn-Zbm1zgwhRkpJWkOCSyxrjxkcqg7VLci-g71VSqmEMl9JKxP_4Oz4E-CxdPKJmL9v--g96bHxpAYbBxODOpYqw5-L7nfdzY13PslgoOoffS2Bkm9nr4n_Lm0921OOkYNQe_qMFQJuOR19fAi2kRy9KMA623zqcQcSxVeocGDj3K2B447Lzq2BxzUbPuKtx4Rd3fHVCmB8thG1aH5o4q1ElGR3biIEpLkx32H3TYH6TG9mQt5bxhWt5l-QqoqqURCTKhUTA39O6XRmvXQf1uNUzKMiPPeFdw7Ap2-LErZ-M57USUkR77e6u2eJ1Kdc3wp0twHvKHT_j3q1mu4DnX2h9ybRxXLE2RiROHJEM3qq_SWJmcWETKorItwFA5vaaimCzgSuwEPamSflBTHv5Qic5hTrM4rOB9tFZibygrLV43a5JGt3ObxO66ITKZKiYt5oZDhMqinmzSDeqkvw5a5feq78-P1-39x-IqUjhedPcaFEKjsk-kEJXaNFhBss9tICx_2-RzayU_NPvoGjr_rmzn5ef9GAxPKCoNq6SR-wxdVCptJhVyCGZGm7vgnRujJHEPs2vb5R-k0MMDvAYAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FModal_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA41U226jMBB9z1fMS6VEW1dAljRxX_onKwcG4hY8yDaEZJV_X2EgoQSSfUFjn2HOXM748xtPiRY5GkiPG8wPHvxdACSacmcAUCEiaU8cvI8FwGUBYGkM_S10WXyOw_mPwwFYLZRJSOe8NTNhcclC7_UVIpFFzoRfsC3q1epJAg_DNd9Vn_Zbm1zgwhRkpJWkOCSyxrjxkcqg7VLci_g71VSqmEMl9JKxP_4Oz4E_CxdPKJmL9v_g96bHxpAYbBxODOpYqw5_L7nfdzY13PslgoOoffS2Bkm9nr4n_Lm0921OOkYNQe_qMFQJuOR19fAi2kRy9KMA623zqcQcSxVeocGDj3K2B447Lzq2BxzUbPuKtx4Rd3fHVCmB8thG1aH5o4q1ElGR3biIEpLkx32H3TYH6TG9mQt5bxhWt5l_QqoqqURCTKhUTA39O6XRmvXQf1uNUzKMiPPeFdw7Ap2_LErZ_M57USUkR77e6u2eJ1Kdc3wp0twHvKHT_j3q1mu4DnX2h9ybRxXLE2RiROHJEM3qq_SWJmcWETKorItwFA5vaaimCzgSuwEPamSflBTHv5Qic5hTrM4rOB9tFZibygrLV43a5JGt3ObxO66ITKZKiYt5oZDhMqinmzSDeqkvw5a5feq78_P1_39x_IqUjhedPcaFEKjsk_kEJXaNFhBss9tICx_2_RzayU_NPvoGjr_rmzn5ef9GAxPKCoNq6SR_wxdVCptJhVyCGZGm7vgnRujJHEPs2vb5R_k0MMDvAYAAA_22_7D = __webpack_require__(45186);
;// ./src/components/ui/Modal.css.ts


var closeButton = 'gw6emh7';
var content = 'gw6emh3';
var description = 'gw6emh5';
var footer = 'gw6emh6';
var overlay = 'gw6emh2';
var title = 'gw6emh4';
;// ./src/components/ui/Modal.tsx
/* __next_internal_client_entry_do_not_use__ Modal,ModalTrigger,ModalContent,ModalTitle,ModalDescription,ModalClose,ModalFooter auto */ 


const Modal = dist/* Root */.bL;
const ModalTrigger = dist/* Trigger */.l9;
const srOnly = {
    position: "absolute",
    width: 1,
    height: 1,
    padding: 0,
    margin: -1,
    overflow: "hidden",
    clipPath: "inset(50%)",
    whiteSpace: "nowrap",
    border: 0
};
function ModalContent({ children, showClose = true, fallbackTitle, className, ...props }) {
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(dist/* Portal */.ZL, {
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(dist/* Overlay */.hJ, {
                className: overlay
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(dist/* Content */.UC, {
                className: [
                    content,
                    className
                ].filter(Boolean).join(" "),
                ...props,
                children: [
                    fallbackTitle != null && /*#__PURE__*/ (0,react_jsx_runtime.jsx)(dist/* Title */.hE, {
                        style: srOnly,
                        children: fallbackTitle
                    }),
                    showClose && /*#__PURE__*/ (0,react_jsx_runtime.jsx)(dist/* Close */.bm, {
                        className: closeButton,
                        "aria-label": "Close",
                        children: "\xd7"
                    }),
                    children
                ]
            })
        ]
    });
}
function ModalTitle({ children, ...props }) {
    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)(dist/* Title */.hE, {
        className: title,
        ...props,
        children: children
    });
}
function ModalDescription({ children, ...props }) {
    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)(dist/* Description */.VY, {
        className: description,
        ...props,
        children: children
    });
}
const ModalClose = dist/* Close */.bm;
function ModalFooter({ children, ...props }) {
    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
        className: footer,
        ...props,
        children: children
    });
}


/***/ })

};
;