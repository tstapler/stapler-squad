(() => {
var exports = {};
exports.id = 6173;
exports.ids = [6173];
exports.modules = {

/***/ 1708:
/***/ ((module) => {

"use strict";
module.exports = require("node:process");

/***/ }),

/***/ 3295:
/***/ ((module) => {

"use strict";
module.exports = require("next/dist/server/app-render/after-task-async-storage.external.js");

/***/ }),

/***/ 5856:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  NotificationsPage: () => (/* binding */ NotificationsPage)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
// EXTERNAL MODULE: ./src/lib/store/index.ts
var store = __webpack_require__(92360);
// EXTERNAL MODULE: ./src/lib/store/sessionsSlice.ts
var sessionsSlice = __webpack_require__(45985);
// EXTERNAL MODULE: ./src/lib/contexts/NotificationContext.tsx + 12 modules
var NotificationContext = __webpack_require__(46569);
// EXTERNAL MODULE: ./src/lib/hooks/useAuditLog.ts
var useAuditLog = __webpack_require__(19149);
// EXTERNAL MODULE: ./src/lib/hooks/useApprovalResolution.ts
var useApprovalResolution = __webpack_require__(86923);
// EXTERNAL MODULE: ./src/lib/hooks/useClearResultLine.ts
var useClearResultLine = __webpack_require__(35590);
// EXTERNAL MODULE: ./src/lib/utils/notificationGrouping.ts
var notificationGrouping = __webpack_require__(73992);
// EXTERNAL MODULE: ./src/lib/utils/notificationMapping.ts
var notificationMapping = __webpack_require__(96303);
// EXTERNAL MODULE: ./src/components/ui/NotificationItem.tsx + 12 modules
var NotificationItem = __webpack_require__(82202);
// EXTERNAL MODULE: ./src/components/ui/Collapsible.tsx + 1 modules
var Collapsible = __webpack_require__(70896);
// EXTERNAL MODULE: ./src/components/ui/NotificationPanel.css.ts
var NotificationPanel_css = __webpack_require__(31352);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Fnotifications%2FNotificationsPage.css.ts.vanilla.css%22%2C%22source%22%3A%22LmhxNHRiMCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGhlaWdodDogMTAwJTsKICBvdmVyZmxvdy15OiBhdXRvOwogIG92ZXJmbG93LXg6IGhpZGRlbjsKICBtYXgtd2lkdGg6IDgwMHB4OwogIG1hcmdpbjogMCBhdXRvOwogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MzEpIHZhcigtLV8xOWV6MnJ6MzEpIDA7Cn0KQG1lZGlhIHNjcmVlbiBhbmQgKG1heC13aWR0aDogNzY4cHgpIHsKICAuaHE0dGIwIHsKICAgIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MnopIHZhcigtLV8xOWV6MnJ6MnopIDA7CiAgfQp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Fnotifications_2FNotificationsPage_css_ts_vanilla_css_22_2C_22source_22_3A_22LmhxNHRiMCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGhlaWdodDogMTAwJTsKICBvdmVyZmxvdy15OiBhdXRvOwogIG92ZXJmbG93LXg6IGhpZGRlbjsKICBtYXgtd2lkdGg6IDgwMHB4OwogIG1hcmdpbjogMCBhdXRvOwogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MzEpIHZhcigtLV8xOWV6MnJ6MzEpIDA7Cn0KQG1lZGlhIHNjcmVlbiBhbmQgKG1heC13aWR0aDogNzY4cHgpIHsKICAuaHE0dGIwIHsKICAgIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MnopIHZhcigtLV8xOWV6MnJ6MnopIDA7CiAgfQp9_22_7D = __webpack_require__(84369);
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
    return minutes <= 0 ? "just now" : `${minutes}m`;
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
            items = items.filter((n)=>!n.metadata?.["item_id"]);
        }
        if (searchQuery.trim()) {
            const q = searchQuery.toLowerCase();
            items = items.filter((n)=>(n.sessionName || "").toLowerCase().includes(q) || (n.message || "").toLowerCase().includes(q) || (n.title || "").toLowerCase().includes(q) || (n.sourceProject || "").toLowerCase().includes(q) || (n.sourceWorkingDir || "").toLowerCase().includes(q) || (n.metadata?.["tool_name"] || "").toLowerCase().includes(q) || (n.metadata?.["tool_input_command"] || "").toLowerCase().includes(q) || (n.metadata?.["tool_input_file"] || "").toLowerCase().includes(q));
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
        !hasLoadedSessionsOnce || liveSessionIds.has(sessionId) || liveSessionTitles.has(sessionId.toLowerCase()) ? `/?session=${encodeURIComponent(sessionId)}` : `/sessions/summary?sessionId=${encodeURIComponent(sessionId)}`, [
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
    const renderRecentActivitySection = ()=>/*#__PURE__*/ (0,react_jsx_runtime.jsx)(Collapsible/* CollapsibleSection */.i, {
            sectionKey: "recent-activity",
            title: `Recent activity · ${recentActivity.length}`,
            children: /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: NotificationPanel_css/* list */.p_,
                children: [
                    (0,notificationGrouping/* groupNotifications */._Q)(recentActivity).map((group)=>/*#__PURE__*/ (0,react_jsx_runtime.jsx)(NotificationItem/* NotificationItem */.rn, {
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
                    historyHasMore && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                        className: NotificationPanel_css/* loadMore */.UX,
                        children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                            className: NotificationPanel_css/* loadMoreButton */.Qf,
                            onClick: loadMoreHistory,
                            disabled: historyLoading,
                            children: historyLoading ? "Loading..." : "Load more"
                        })
                    })
                ]
            })
        });
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: pageRoot,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: NotificationPanel_css/* header */.wx,
                "data-testid": "notifications-header",
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("h2", {
                        className: NotificationPanel_css/* title */.DD,
                        "data-testid": "notifications-title",
                        children: [
                            "Notifications",
                            unreadCount > 0 && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: NotificationPanel_css/* unreadBadge */.HN,
                                "data-testid": "notifications-unread-badge",
                                children: (0,notificationMapping/* capBadgeCount */.BK)(unreadCount)
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                        className: NotificationPanel_css/* headerActions */.$s,
                        children: notificationHistory.length > 0 && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
                            children: [
                                scopedMarkReadIds.length > 0 && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                    className: NotificationPanel_css/* markAllButton */.Vt,
                                    onClick: handleMarkActivityRead,
                                    "aria-label": "Mark activity as read",
                                    "data-testid": "notifications-mark-all-read",
                                    children: "Mark activity read"
                                }),
                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
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
            keptLine && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                "data-testid": "notifications-kept-line",
                children: keptLine
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: NotificationPanel_css/* filterBar */.kT,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: NotificationPanel_css/* searchRow */.Pc,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                                className: NotificationPanel_css/* searchInput */.Rt,
                                type: "search",
                                placeholder: "Search notifications…",
                                value: searchQuery,
                                onChange: (e)=>setSearchQuery(e.target.value),
                                "aria-label": "Search notifications"
                            }),
                            searchQuery && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: NotificationPanel_css/* searchClearButton */.aF,
                                onClick: ()=>setSearchQuery(""),
                                "aria-label": "Clear search",
                                type: "button",
                                children: "✕"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: NotificationPanel_css/* filterPills */.VS,
                        role: "group",
                        "aria-label": "Filter by type",
                        children: [
                            Object.keys(TYPE_FILTER_LABELS).map((filter)=>/*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                    className: `${NotificationPanel_css/* filterPill */._y} ${typeFilter === filter ? NotificationPanel_css/* filterPillActive */.s8 : ""}`,
                                    onClick: ()=>setTypeFilter(filter),
                                    "aria-pressed": typeFilter === filter,
                                    children: TYPE_FILTER_LABELS[filter]
                                }, filter)),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: `${NotificationPanel_css/* filterPill */._y} ${hideBacklogItems ? NotificationPanel_css/* filterPillExcludeActive */.uk : ""}`,
                                onClick: ()=>setHideBacklogItems((v)=>!v),
                                "aria-pressed": hideBacklogItems,
                                "aria-label": "Exclude backlog notifications",
                                children: "\uD83D\uDEAB Hide backlog"
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: NotificationPanel_css/* content */.Qs,
                "data-testid": "notifications-content",
                children: [
                    hasIncompleteSearch && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: NotificationPanel_css/* incompleteSearchNotice */.Xn,
                        "data-testid": "incomplete-search-notice",
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                children: "Showing results from loaded history only — load more to search older notifications."
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: NotificationPanel_css/* incompleteSearchNoticeButton */.F_,
                                onClick: loadMoreHistory,
                                disabled: historyLoading,
                                type: "button",
                                "data-testid": "incomplete-search-load-more",
                                children: historyLoading ? "Loading..." : "Load more"
                            })
                        ]
                    }),
                    historyLoading && notificationHistory.length === 0 ? /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: NotificationPanel_css/* empty */.Ie,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                                className: NotificationPanel_css/* emptyIcon */.Dw,
                                "aria-hidden": "true",
                                children: "⏳"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                                className: NotificationPanel_css/* emptyText */.BI,
                                children: "Loading notifications..."
                            })
                        ]
                    }) : totalActionableCount > 0 && needsDecision.length === 0 ? // Product Triad Review round-5 blocker fix: this branch must be
                    // reachable even when recentActivity is ALSO empty — see Task 3.1.2c.
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(NotificationItem/* NeedsDecisionSection */.BF, {
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
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: NotificationPanel_css/* empty */.Ie,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                                className: NotificationPanel_css/* emptyIcon */.Dw,
                                "aria-hidden": "true",
                                children: hasActiveFilter ? "🔍" : "🔔"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                                className: NotificationPanel_css/* emptyText */.BI,
                                children: hasActiveFilter ? "No matching notifications" : "No notifications yet"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                                className: NotificationPanel_css/* emptySubtext */.lP,
                                children: hasActiveFilter ? "Try adjusting your search or filter" : "You'll see notifications from your sessions here"
                            })
                        ]
                    }) : /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(NotificationItem/* NeedsDecisionSection */.BF, {
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
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(NotificationItem/* AutoHandledSection */.FB, {
                notifications: autoHandledNotifications,
                isOpen: autoHandledOpen,
                onToggle: ()=>setAutoHandledOpen((v)=>!v)
            })
        ]
    });
}


/***/ }),

/***/ 7696:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   PageViewTracker: () => (/* binding */ PageViewTracker)
/* harmony export */ });
/* harmony import */ var react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(70679);
/* harmony import */ var react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__);

const PageViewTracker = (0,react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__.registerClientReference)(
function() { throw new Error("Attempted to call PageViewTracker() from the server but PageViewTracker is on the client. It's not possible to invoke a client function from the server, it can only be rendered as a Component or passed to props of a Client Component."); },
"/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/components/analytics/PageViewTracker.tsx",
"PageViewTracker",
);

/***/ }),

/***/ 8086:
/***/ ((module) => {

"use strict";
module.exports = require("module");

/***/ }),

/***/ 10846:
/***/ ((module) => {

"use strict";
module.exports = require("next/dist/compiled/next-server/app-page.runtime.prod.js");

/***/ }),

/***/ 19121:
/***/ ((module) => {

"use strict";
module.exports = require("next/dist/server/app-render/action-async-storage.external.js");

/***/ }),

/***/ 19503:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 5856));
;
Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 88486));


/***/ }),

/***/ 21820:
/***/ ((module) => {

"use strict";
module.exports = require("os");

/***/ }),

/***/ 25692:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
__webpack_require__.r(__webpack_exports__);
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   "default": () => (/* binding */ Page),
/* harmony export */   metadata: () => (/* binding */ metadata)
/* harmony export */ });
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(12129);
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var _NotificationsPage__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(33792);
/* harmony import */ var _components_analytics_PageViewTracker__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(7696);
// analytics-exempt

// +feature: ui:notifications-page


const metadata = {
    title: "Notifications - Stapler Squad",
    description: "Session notifications, approvals, and alerts."
};
function Page() {
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)(react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.Fragment, {
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(_components_analytics_PageViewTracker__WEBPACK_IMPORTED_MODULE_2__.PageViewTracker, {}),
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(_NotificationsPage__WEBPACK_IMPORTED_MODULE_1__.NotificationsPage, {})
        ]
    });
}


/***/ }),

/***/ 27910:
/***/ ((module) => {

"use strict";
module.exports = require("stream");

/***/ }),

/***/ 28354:
/***/ ((module) => {

"use strict";
module.exports = require("util");

/***/ }),

/***/ 29021:
/***/ ((module) => {

"use strict";
module.exports = require("fs");

/***/ }),

/***/ 29294:
/***/ ((module) => {

"use strict";
module.exports = require("next/dist/server/app-render/work-async-storage.external.js");

/***/ }),

/***/ 33792:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   NotificationsPage: () => (/* binding */ NotificationsPage)
/* harmony export */ });
/* harmony import */ var react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(70679);
/* harmony import */ var react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__);

const NotificationsPage = (0,react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__.registerClientReference)(
function() { throw new Error("Attempted to call NotificationsPage() from the server but NotificationsPage is on the client. It's not possible to invoke a client function from the server, it can only be rendered as a Component or passed to props of a Client Component."); },
"/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/notifications/NotificationsPage.tsx",
"NotificationsPage",
);

/***/ }),

/***/ 33873:
/***/ ((module) => {

"use strict";
module.exports = require("path");

/***/ }),

/***/ 38408:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
__webpack_require__.r(__webpack_exports__);
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   GlobalError: () => (/* reexport default from dynamic */ next_dist_client_components_error_boundary__WEBPACK_IMPORTED_MODULE_2___default.a),
/* harmony export */   __next_app__: () => (/* binding */ __next_app__),
/* harmony export */   pages: () => (/* binding */ pages),
/* harmony export */   routeModule: () => (/* binding */ routeModule),
/* harmony export */   tree: () => (/* binding */ tree)
/* harmony export */ });
/* harmony import */ var next_dist_server_route_modules_app_page_module_compiled__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(54315);
/* harmony import */ var next_dist_server_route_modules_app_page_module_compiled__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(next_dist_server_route_modules_app_page_module_compiled__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var next_dist_server_route_kind__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(34788);
/* harmony import */ var next_dist_client_components_error_boundary__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(92838);
/* harmony import */ var next_dist_client_components_error_boundary__WEBPACK_IMPORTED_MODULE_2___default = /*#__PURE__*/__webpack_require__.n(next_dist_client_components_error_boundary__WEBPACK_IMPORTED_MODULE_2__);
/* harmony import */ var next_dist_server_app_render_entry_base__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(79793);
/* harmony import */ var next_dist_server_app_render_entry_base__WEBPACK_IMPORTED_MODULE_3___default = /*#__PURE__*/__webpack_require__.n(next_dist_server_app_render_entry_base__WEBPACK_IMPORTED_MODULE_3__);
/* harmony reexport (unknown) */ var __WEBPACK_REEXPORT_OBJECT__ = {};
/* harmony reexport (unknown) */ for(const __WEBPACK_IMPORT_KEY__ in next_dist_server_app_render_entry_base__WEBPACK_IMPORTED_MODULE_3__) if(["default","tree","pages","GlobalError","__next_app__","routeModule"].indexOf(__WEBPACK_IMPORT_KEY__) < 0) __WEBPACK_REEXPORT_OBJECT__[__WEBPACK_IMPORT_KEY__] = () => next_dist_server_app_render_entry_base__WEBPACK_IMPORTED_MODULE_3__[__WEBPACK_IMPORT_KEY__]
/* harmony reexport (unknown) */ __webpack_require__.d(__webpack_exports__, __WEBPACK_REEXPORT_OBJECT__);
const module0 = () => Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 36776));
const module1 = () => Promise.resolve(/* import() eager */).then(__webpack_require__.t.bind(__webpack_require__, 93250, 23));
const module2 = () => Promise.resolve(/* import() eager */).then(__webpack_require__.t.bind(__webpack_require__, 76963, 23));
const module3 = () => Promise.resolve(/* import() eager */).then(__webpack_require__.t.bind(__webpack_require__, 54552, 23));
const page4 = () => Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 25692));


// We inject the tree and pages here so that we can use them in the route
// module.
const tree = {
        children: [
        '',
        {
        children: [
        'notifications',
        {
        children: ['__PAGE__', {}, {
          page: [page4, "/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/notifications/page.tsx"],
          
        }]
      },
        {
        
        
      }
      ]
      },
        {
        'layout': [module0, "/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/layout.tsx"],
'not-found': [module1, "next/dist/client/components/not-found-error"],
'forbidden': [module2, "next/dist/client/components/forbidden-error"],
'unauthorized': [module3, "next/dist/client/components/unauthorized-error"],
        
      }
      ]
      }.children;
const pages = ["/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/notifications/page.tsx"];


const __next_app_require__ = __webpack_require__
const __next_app_load_chunk__ = () => Promise.resolve()
const __next_app__ = {
    require: __next_app_require__,
    loadChunk: __next_app_load_chunk__
};

// Create and export the route module that will be consumed.
const routeModule = new next_dist_server_route_modules_app_page_module_compiled__WEBPACK_IMPORTED_MODULE_0__.AppPageRouteModule({
    definition: {
        kind: next_dist_server_route_kind__WEBPACK_IMPORTED_MODULE_1__.RouteKind.APP_PAGE,
        page: "/notifications/page",
        pathname: "/notifications",
        // The following aren't used in production.
        bundlePath: '',
        filename: '',
        appPaths: []
    },
    userland: {
        loaderTree: tree
    }
});

//# sourceMappingURL=app-page.js.map

/***/ }),

/***/ 55591:
/***/ ((module) => {

"use strict";
module.exports = require("https");

/***/ }),

/***/ 63033:
/***/ ((module) => {

"use strict";
module.exports = require("next/dist/server/app-render/work-unit-async-storage.external.js");

/***/ }),

/***/ 70896:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  I: () => (/* binding */ CollapsibleGroup),
  i: () => (/* binding */ CollapsibleSection)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@radix-ui+react-accordion@1.2.17+b721ee6ccd61acaa/node_modules/@radix-ui/react-accordion/dist/index.mjs + 14 modules
var dist = __webpack_require__(87936);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fui%2FCollapsible.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA5VU24rbMBB9z1cMC4XNgxbbm7RdhcL-RymLIo1tNbbGSOPcSv69WImT2EmW9s1ozjkzZy5-X-Eu96rGAB-pKxNCnsGfCUDuqY4fACXaomQJyWICcJgAMI0ia-WfhfDK2K1QWpM3lpzQ5BgdiyNqemQfJu-3KeePUv6H8J2ykj7lS58oiYiNNVxKSJPky-I6msbokrxBL5bETLWEtNlCoMqaUzEf6RvuM7_H6ZArKxVY6NJW5p6MI4cDQhZRxoamUjsJeYXbxbg2gNo60duZzZoIUZUtnLCMdZCg0TH67vl3G9jmu747EkKjNIol8gbRdYhCNXLkItvH9jXKGOuKcfQ1md7HL5VeFZ5aZySwVy40yqPjxdl3bxhAtz6Ql9CQ7SvVVHUvQ-UkCufkWAS7x5tS9CW-GWzHGVFGBOOWRWyRhApzHjZdlrRGfxzQlYeh0vfpiJSTboNY22CXFUYytVxZhxKyB-tRx2JOMEF5HpAliKwb4ZX263ALrIvwfhn-ddKXUMcUofTWrU77H8dj2ZI7jSonX0Pyks4DoAr4cCDZqAs_jWIlAivGH0_UoHv6BSMfZ30Jnjrg81tisBgKfT02cI0-r2gjobTGdPt5DbmXq2MpZ2t19HL5XV28CGr5EyFdUUDzWGr-mdS3yDofSnJ7GLfHs5gc_gLw6CcyYQUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FCollapsible_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA5VU24rbMBB9z1cMC4XNgxbbm7RdhcL_RymLIo1tNbbGSOPcSv69WImT2EmW9s1ozjkzZy5_X_Eu96rGAB_pKxNCnsGfCUDuqY4fACXaomQJyWICcJgAMI0ia_WfhfDK2K1QWpM3lpzQ5BgdiyNqemQfJu_3KeePUv6H8J2ykj7lS58oiYiNNVxKSJPky_I6msbokrxBL5bETLWEtNlCoMqaUzEf6RvuM7_H6ZArKxVY6NJW5p6MI4cDQhZRxoamUjsJeYXbxbg2gNo60duZzZoIUZUtnLCMdZCg0TH67vl3G9jmu747EkKjNIol8gbRdYhCNXLkItvH9jXKGOuKcfQ1md7HL5VeFZ5aZySwVy40yqPjxdl3bxhAtz6Ql9CQ7SvVVHUvQ_UkCufkWAS7x5tS9CW_GWzHGVFGBOOWRWyRhApzHjZdlrRGfxzQlYeh0vfpiJSTboNY22CXFUYytVxZhxKyB_tRx2JOMEF5HpAliKwb4ZX263ALrIvwfhn_ddKXUMcUofTWrU77H8dj2ZI7jSonX0Pyks4DoAr4cCDZqAs_jWIlAivGH0_UoHv6BSMfZ30Jnjrg81tisBgKfT02cI0_r2gjobTGdPt5DbmXq2MpZ2t19HL5XV28CGr5EyFdUUDzWGr_mdS3yDofSnJ7GLfHs5gc_gLw6CcyYQUAAA_22_7D = __webpack_require__(97503);
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
 */ function CollapsibleGroup({ children, className, defaultValue, value, onValueChange }) {
    const rootClassName = className ? `${root} ${className}` : root;
    // The group's actual initial-open set for this render: the live
    // controlled `value` when present, else the uncontrolled `defaultValue`
    // Radix seeds its Root with. Either way, this is what determines each
    // section's real open/closed state — never a child's own defaultExpanded.
    const openKeys = value !== undefined ? value : defaultValue ?? [];
    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)(CollapsibleGroupContext.Provider, {
        value: {
            insideGroup: true,
            openKeys
        },
        children: value !== undefined ? /*#__PURE__*/ (0,react_jsx_runtime.jsx)(dist/* Root */.bL, {
            type: "multiple",
            className: rootClassName,
            value: value,
            onValueChange: onValueChange,
            children: children
        }) : /*#__PURE__*/ (0,react_jsx_runtime.jsx)(dist/* Root */.bL, {
            type: "multiple",
            className: rootClassName,
            defaultValue: defaultValue ?? [],
            onValueChange: onValueChange,
            children: children
        })
    });
}
function CollapsibleItem({ sectionKey, title, children, className }) {
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(dist/* Item */.q7, {
        value: sectionKey,
        className: className ? `${item} ${className}` : item,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(dist/* Header */.Y9, {
                children: /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(dist/* Trigger */.l9, {
                    className: header,
                    "data-testid": `collapsible-header-${sectionKey}`,
                    children: [
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                            children: title
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                            className: chevron,
                            "aria-hidden": "true",
                            children: "▸"
                        })
                    ]
                })
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(dist/* Content */.UC, {
                className: content,
                children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
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
 */ function CollapsibleSection({ sectionKey, title, defaultExpanded = false, onExpandedChange, children, className }) {
    const { insideGroup, openKeys } = (0,react.useContext)(CollapsibleGroupContext);
    if (insideGroup) {
        if (false) {}
        return /*#__PURE__*/ (0,react_jsx_runtime.jsx)(CollapsibleItem, {
            sectionKey: sectionKey,
            title: title,
            className: className,
            children: children
        });
    }
    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)(dist/* Root */.bL, {
        type: "multiple",
        className: root,
        defaultValue: defaultExpanded ? [
            sectionKey
        ] : [],
        onValueChange: (next)=>onExpandedChange?.(next.includes(sectionKey)),
        children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)(CollapsibleItem, {
            sectionKey: sectionKey,
            title: title,
            className: className,
            children: children
        })
    });
}


/***/ }),

/***/ 73136:
/***/ ((module) => {

"use strict";
module.exports = require("node:url");

/***/ }),

/***/ 73566:
/***/ ((module) => {

"use strict";
module.exports = require("worker_threads");

/***/ }),

/***/ 74075:
/***/ ((module) => {

"use strict";
module.exports = require("zlib");

/***/ }),

/***/ 76760:
/***/ ((module) => {

"use strict";
module.exports = require("node:path");

/***/ }),

/***/ 79551:
/***/ ((module) => {

"use strict";
module.exports = require("url");

/***/ }),

/***/ 81630:
/***/ ((module) => {

"use strict";
module.exports = require("http");

/***/ }),

/***/ 83997:
/***/ ((module) => {

"use strict";
module.exports = require("tty");

/***/ }),

/***/ 84369:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, ".hq4tb0 {\n  display: flex;\n  flex-direction: column;\n  height: 100%;\n  overflow-y: auto;\n  overflow-x: hidden;\n  max-width: 800px;\n  margin: 0 auto;\n  padding: var(--_19ez2rz31) var(--_19ez2rz31) 0;\n}\n@media screen and (max-width: 768px) {\n  .hq4tb0 {\n    padding: var(--_19ez2rz2z) var(--_19ez2rz2z) 0;\n  }\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 91479:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 33792));
;
Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 7696));


/***/ })

};
;

// load runtime
var __webpack_require__ = require("../../webpack-runtime.js");
__webpack_require__.C(exports);
var __webpack_exec__ = (moduleId) => (__webpack_require__(__webpack_require__.s = moduleId))
var __webpack_exports__ = __webpack_require__.X(0, [4655,2802,4164], () => (__webpack_exec__(38408)));
module.exports = __webpack_exports__;

})();