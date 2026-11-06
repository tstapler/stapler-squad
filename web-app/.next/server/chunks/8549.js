"use strict";
exports.id = 8549;
exports.ids = [8549];
exports.modules = {

/***/ 45960:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  ie: () => (/* binding */ PR_STATUS_UNKNOWN_CLASS),
  n1: () => (/* binding */ PR_STATUS_UNKNOWN_ICON),
  C5: () => (/* binding */ PR_STATUS_UNKNOWN_LABEL),
  n4: () => (/* binding */ formatAgo),
  cw: () => (/* binding */ formatSinceUTC),
  CR: () => (/* binding */ formatStuckDuration),
  px: () => (/* binding */ getStuckReasonClass),
  qk: () => (/* binding */ getStuckReasonIcon),
  Lv: () => (/* binding */ getStuckReasonLabel),
  tR: () => (/* binding */ groupStuckItemsByItemId),
  lc: () => (/* binding */ isPrStatusUnknown),
  xj: () => (/* binding */ isRemediationParked),
  sh: () => (/* binding */ summarizeStuckItemGroup)
});

// UNUSED EXPORTS: MAX_REMEDIATION_ATTEMPTS, PR_STATUS_STALE_THRESHOLD_MS, STUCK_REASON_CLASS, STUCK_REASON_ICONS, STUCK_REASON_LABELS, STUCK_REASON_PRIORITY, selectPrimaryStuckItem

// EXTERNAL MODULE: ./src/gen/session/v1/backlog_pb.ts
var backlog_pb = __webpack_require__(34548);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog-stuck%2FstuckReason.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA73TS27DIBAG4L1PwbJZEBmSuA05TEQMwTQUENjxo-rdK6NWrV25zmq2M7_8MaPx9kzovtf3XY7eM4SEjt7wniFtjbYSX43sThlC3Ghlsa7lW2SolLaWYSwr7hm68_CE8Zkc5UDDQPvN2PFcCG0VQ9R388SQEhcXhAw4cKGbOP_KrkiZq7M1jnqQf_r8p99KraqaoSLPx1p6d_VVI9tDyhnZ4VgFbW8MpVRb6Vri6HkpGbKuDdyfso9s-70OktZx4eVNBddYMX_APfmlMy7MW-2v8RgivkPRGS1mqWYz8eiK1y97w0NeN_V2Kx4hyyChD4kkn5J74BEP8CMW8OQz8FZfgL0j_Eo58IgX-BFLeFIAb1UCe1dgTwF7FbCn10708M-JFhORLp3ofkq-wv8VN3jSrJBiWXwMVKP3CfJDEptZCgAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_stuck_2FstuckReason_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA73TS27DIBAG4L1PwbJZEBmSuA05TEQMwTQUENjxo_rdK6NWrV25zmq2M7_8MaPx9kzovtf3XY7eM4SEjt7wniFtjbYSX43sThlC3Ghlsa7lW2SolLaWYSwr7hm68_CE8Zkc5UDDQPvN2PFcCG0VQ9R388SQEhcXhAw4cKGbOP_KrkiZq7M1jnqQf_r8p99KraqaoSLPx1p6d_VVI9tDyhnZ4VgFbW8MpVRb6Vri6HkpGbKuDdyfso9s_70OktZx4eVNBddYMX_APfmlMy7MW_2v8RgivkPRGS1mqWYz8eiK1y97w0NeN_V2Kx4hyyChD4kkn5J74BEP8CMW8OQz8FZfgL0j_Eo58IgX_BFLeFIAb1UCe1dgTwF7FbCn10708M_JFhORLp3ofkq_wv8VN3jSrJBiWXwMVKP3CfJDEptZCgAA_22_7D = __webpack_require__(37252);
;// ./src/components/backlog-stuck/stuckReason.css.ts


var chip = '_124yiv30';
var chipAbandonedReview = '_124yiv32 _124yiv30';
var chipAutonomousStuck = '_124yiv38 _124yiv30';
var chipBlockedByClaim = '_124yiv3h _124yiv30';
var chipBlockedByDependency = '_124yiv3g _124yiv30';
var chipBouncing = '_124yiv35 _124yiv30';
var chipEscalated = '_124yiv3i _124yiv30';
var chipLikelyFlaky = '_124yiv3f _124yiv30';
var chipOrphanedTriage = '_124yiv37 _124yiv30';
var chipPlanNotApproved = '_124yiv3a _124yiv30';
var chipPrNeedsFix = '_124yiv3d _124yiv30';
var chipPrPendingNoPR = '_124yiv3b _124yiv30';
var chipPrReady = '_124yiv31 _124yiv30';
var chipPushFailed = '_124yiv36 _124yiv30';
var chipRespawnBlockedActive = '_124yiv3e _124yiv30';
var chipReworkBlockedStale = '_124yiv3c _124yiv30';
var chipReworkCap = '_124yiv33 _124yiv30';
var chipSpawnFailed = '_124yiv39 _124yiv30';
var chipStaleWork = '_124yiv34 _124yiv30';
var chipSteerFailed = '_124yiv3j _124yiv30';
var chipUnknown = '_124yiv3l _124yiv30';
var chipWorktreeInconsistent = '_124yiv3k _124yiv30';
;// ./src/components/backlog-stuck/stuckReason.ts
// Reason label/class/icon maps for the "Stuck Backlog Items" section
// (plan.md Task 4.1.2a). Direct analog of
// web-app/src/components/backlog/BacklogItemBadge.tsx's STATUS_CLASS +
// getStatusLabel pair.
//
// STUCK_REASON_LABELS/STUCK_REASON_CLASS/STUCK_REASON_ICONS are keyed as
// `Record<StuckReason, T>` (not a lookup function with a fallback) so that
// adding a new value to the generated `StuckReason` proto enum is a TypeScript
// compile error here — not a silently-blank chip at runtime.


/** Text label for every StuckReason, paired with a color class — never color-only. */ const STUCK_REASON_LABELS = {
    [backlog_pb/* StuckReason */.Pwx.UNSPECIFIED]: "Unknown reason",
    [backlog_pb/* StuckReason */.Pwx.PR_READY_UNMERGED]: "PR ready to merge",
    [backlog_pb/* StuckReason */.Pwx.REWORK_CAP]: "Rework cap hit",
    [backlog_pb/* StuckReason */.Pwx.ABANDONED_REVIEW]: "Abandoned review",
    [backlog_pb/* StuckReason */.Pwx.STALE_WORK]: "Stale work session",
    [backlog_pb/* StuckReason */.Pwx.BOUNCING]: "Not converging",
    [backlog_pb/* StuckReason */.Pwx.PUSH_FAILED]: "Push/PR-create failed",
    [backlog_pb/* StuckReason */.Pwx.ORPHANED_TRIAGE]: "Triage session ended without finishing",
    [backlog_pb/* StuckReason */.Pwx.AUTONOMOUS_STUCK]: "Autonomous mode stopped without finishing",
    [backlog_pb/* StuckReason */.Pwx.SPAWN_FAILED]: "Rework session failed to start",
    [backlog_pb/* StuckReason */.Pwx.PLAN_NOT_APPROVED]: "Waiting on plan approval",
    [backlog_pb/* StuckReason */.Pwx.PR_PENDING_NO_PR]: "PR reference lost",
    [backlog_pb/* StuckReason */.Pwx.REWORK_BLOCKED_STALE]: "Rework blocked — session stalled",
    [backlog_pb/* StuckReason */.Pwx.PR_NEEDS_FIX]: "PR needs attention",
    [backlog_pb/* StuckReason */.Pwx.RESPAWN_BLOCKED_ACTIVE]: "Auto-respawn skipped — session active",
    [backlog_pb/* StuckReason */.Pwx.LIKELY_FLAKY]: "Possibly flaky — verify before assuming",
    [backlog_pb/* StuckReason */.Pwx.BLOCKED_BY_DEPENDENCY]: "Waiting on blocker item",
    [backlog_pb/* StuckReason */.Pwx.MULTIPLE_REASONS]: "Multiple reasons stuck",
    [backlog_pb/* StuckReason */.Pwx.BOUNCE_CAP_EXHAUSTED]: "Bounce cap exhausted",
    [backlog_pb/* StuckReason */.Pwx.STEER_FAILED]: "Steer attempt failed",
    [backlog_pb/* StuckReason */.Pwx.BLOCKED_BY_CLAIM]: "Blocked: claimed by another host",
    [backlog_pb/* StuckReason */.Pwx.WORKTREE_INCONSISTENT]: "Worktree state inconsistent",
    [backlog_pb/* StuckReason */.Pwx.REPEATED_NOOP_DISPATCH]: "Repeated no-op sessions",
    [backlog_pb/* StuckReason */.Pwx.MERGED_PR_UNVERIFIED]: "Merged PR not tied to item",
    [backlog_pb/* StuckReason */.Pwx.VERDICT_UNDELIVERED]: "Review verdict not delivered to session"
};
/** Decorative icon glyph for every StuckReason (never the sole signal — text label always accompanies it). */ const STUCK_REASON_ICONS = {
    [backlog_pb/* StuckReason */.Pwx.UNSPECIFIED]: "⚪",
    [backlog_pb/* StuckReason */.Pwx.PR_READY_UNMERGED]: "🟢",
    [backlog_pb/* StuckReason */.Pwx.REWORK_CAP]: "🔴",
    [backlog_pb/* StuckReason */.Pwx.ABANDONED_REVIEW]: "🟡",
    [backlog_pb/* StuckReason */.Pwx.STALE_WORK]: "🟠",
    [backlog_pb/* StuckReason */.Pwx.BOUNCING]: "🔁",
    [backlog_pb/* StuckReason */.Pwx.PUSH_FAILED]: "⛔",
    [backlog_pb/* StuckReason */.Pwx.ORPHANED_TRIAGE]: "🟡",
    [backlog_pb/* StuckReason */.Pwx.AUTONOMOUS_STUCK]: "🟡",
    [backlog_pb/* StuckReason */.Pwx.SPAWN_FAILED]: "⛔",
    [backlog_pb/* StuckReason */.Pwx.PLAN_NOT_APPROVED]: "🟡",
    [backlog_pb/* StuckReason */.Pwx.PR_PENDING_NO_PR]: "⛔",
    [backlog_pb/* StuckReason */.Pwx.REWORK_BLOCKED_STALE]: "🟥",
    [backlog_pb/* StuckReason */.Pwx.PR_NEEDS_FIX]: "🟡",
    [backlog_pb/* StuckReason */.Pwx.RESPAWN_BLOCKED_ACTIVE]: "🟡",
    [backlog_pb/* StuckReason */.Pwx.LIKELY_FLAKY]: "🟡",
    [backlog_pb/* StuckReason */.Pwx.BLOCKED_BY_DEPENDENCY]: "🟠",
    [backlog_pb/* StuckReason */.Pwx.MULTIPLE_REASONS]: "🔺",
    [backlog_pb/* StuckReason */.Pwx.BOUNCE_CAP_EXHAUSTED]: "🛑",
    [backlog_pb/* StuckReason */.Pwx.STEER_FAILED]: "⛔",
    [backlog_pb/* StuckReason */.Pwx.BLOCKED_BY_CLAIM]: "🟠",
    [backlog_pb/* StuckReason */.Pwx.WORKTREE_INCONSISTENT]: "⛔",
    [backlog_pb/* StuckReason */.Pwx.REPEATED_NOOP_DISPATCH]: "🔁",
    [backlog_pb/* StuckReason */.Pwx.MERGED_PR_UNVERIFIED]: "⛔",
    [backlog_pb/* StuckReason */.Pwx.VERDICT_UNDELIVERED]: "🟡"
};
/** vanilla-extract class per StuckReason (design/ux.md Surface 7 chip legend). */ const STUCK_REASON_CLASS = {
    [backlog_pb/* StuckReason */.Pwx.UNSPECIFIED]: chipUnknown,
    [backlog_pb/* StuckReason */.Pwx.PR_READY_UNMERGED]: chipPrReady,
    [backlog_pb/* StuckReason */.Pwx.REWORK_CAP]: chipReworkCap,
    [backlog_pb/* StuckReason */.Pwx.ABANDONED_REVIEW]: chipAbandonedReview,
    [backlog_pb/* StuckReason */.Pwx.STALE_WORK]: chipStaleWork,
    [backlog_pb/* StuckReason */.Pwx.BOUNCING]: chipBouncing,
    [backlog_pb/* StuckReason */.Pwx.PUSH_FAILED]: chipPushFailed,
    [backlog_pb/* StuckReason */.Pwx.ORPHANED_TRIAGE]: chipOrphanedTriage,
    [backlog_pb/* StuckReason */.Pwx.AUTONOMOUS_STUCK]: chipAutonomousStuck,
    [backlog_pb/* StuckReason */.Pwx.SPAWN_FAILED]: chipSpawnFailed,
    [backlog_pb/* StuckReason */.Pwx.PLAN_NOT_APPROVED]: chipPlanNotApproved,
    [backlog_pb/* StuckReason */.Pwx.PR_PENDING_NO_PR]: chipPrPendingNoPR,
    [backlog_pb/* StuckReason */.Pwx.REWORK_BLOCKED_STALE]: chipReworkBlockedStale,
    [backlog_pb/* StuckReason */.Pwx.PR_NEEDS_FIX]: chipPrNeedsFix,
    [backlog_pb/* StuckReason */.Pwx.RESPAWN_BLOCKED_ACTIVE]: chipRespawnBlockedActive,
    [backlog_pb/* StuckReason */.Pwx.LIKELY_FLAKY]: chipLikelyFlaky,
    [backlog_pb/* StuckReason */.Pwx.BLOCKED_BY_DEPENDENCY]: chipBlockedByDependency,
    [backlog_pb/* StuckReason */.Pwx.MULTIPLE_REASONS]: chipEscalated,
    [backlog_pb/* StuckReason */.Pwx.BOUNCE_CAP_EXHAUSTED]: chipEscalated,
    [backlog_pb/* StuckReason */.Pwx.STEER_FAILED]: chipSteerFailed,
    [backlog_pb/* StuckReason */.Pwx.BLOCKED_BY_CLAIM]: chipBlockedByClaim,
    [backlog_pb/* StuckReason */.Pwx.WORKTREE_INCONSISTENT]: chipWorktreeInconsistent,
    [backlog_pb/* StuckReason */.Pwx.REPEATED_NOOP_DISPATCH]: chipWorktreeInconsistent,
    [backlog_pb/* StuckReason */.Pwx.MERGED_PR_UNVERIFIED]: chipPrPendingNoPR,
    [backlog_pb/* StuckReason */.Pwx.VERDICT_UNDELIVERED]: chipAbandonedReview
};
/**
 * Display priority when a single backlog item has multiple simultaneous open
 * StuckReason rows — ListStuckBacklogItems can return more than one row per
 * item_id (e.g. BOUNCING + BOUNCE_CAP_EXHAUSTED + MULTIPLE_REASONS all open
 * at once). Lower number = shown as the primary reason first. Keyed as
 * `Record<StuckReason, number>` for the same exhaustiveness reason as the
 * maps above: a new StuckReason value with no priority entry is a TypeScript
 * compile error here, not a silent tie (every unmapped value defaulting to
 * `undefined`, which breaks the `<` comparison) at runtime.
 *
 * BacklogItemDetail and BacklogBoard/BacklogItemCard previously each
 * collapsed a multi-reason item to one reason independently (`.find()` vs.
 * `Map` construction order) with no shared order between them, so the same
 * item could show two different "the" reasons in two different views — see
 * BUG-105. Both call `selectPrimaryStuckItem`/`summarizeStuckItemGroup`
 * below instead of picking a reason themselves.
 *
 * Ordering rationale: the two synthetic "automated remediation has given up"
 * signals (BOUNCE_CAP_EXHAUSTED, then the generic MULTIPLE_REASONS escalation
 * flag) outrank every specific in-progress reason, since they mean "stop
 * auto-retrying, a human needs to look" — showing a lower-severity reason
 * like BOUNCING instead would read as "still auto-retrying" and could steer
 * an operator into the wrong action. Reasons with an available remediation
 * action outrank purely informational/positive ones (LIKELY_FLAKY,
 * PR_READY_UNMERGED), and UNSPECIFIED always sorts last.
 */ const STUCK_REASON_PRIORITY = {
    [backlog_pb/* StuckReason */.Pwx.BOUNCE_CAP_EXHAUSTED]: 0,
    [backlog_pb/* StuckReason */.Pwx.MULTIPLE_REASONS]: 1,
    [backlog_pb/* StuckReason */.Pwx.STEER_FAILED]: 2,
    [backlog_pb/* StuckReason */.Pwx.WORKTREE_INCONSISTENT]: 3,
    [backlog_pb/* StuckReason */.Pwx.REPEATED_NOOP_DISPATCH]: 3.5,
    [backlog_pb/* StuckReason */.Pwx.PUSH_FAILED]: 4,
    [backlog_pb/* StuckReason */.Pwx.SPAWN_FAILED]: 5,
    [backlog_pb/* StuckReason */.Pwx.PR_PENDING_NO_PR]: 6,
    // Same dead-end shape as PR_PENDING_NO_PR: needs a manual mark-done.
    [backlog_pb/* StuckReason */.Pwx.MERGED_PR_UNVERIFIED]: 6.5,
    [backlog_pb/* StuckReason */.Pwx.REWORK_BLOCKED_STALE]: 7,
    [backlog_pb/* StuckReason */.Pwx.PR_NEEDS_FIX]: 8,
    // The work session still has the verdict via its own polling fallback, so
    // this is strictly less urgent than PR_NEEDS_FIX (nothing auto-retries
    // that one) but still worth surfacing before ABANDONED_REVIEW.
    [backlog_pb/* StuckReason */.Pwx.VERDICT_UNDELIVERED]: 8.5,
    [backlog_pb/* StuckReason */.Pwx.ABANDONED_REVIEW]: 9,
    [backlog_pb/* StuckReason */.Pwx.REWORK_CAP]: 10,
    [backlog_pb/* StuckReason */.Pwx.RESPAWN_BLOCKED_ACTIVE]: 11,
    [backlog_pb/* StuckReason */.Pwx.ORPHANED_TRIAGE]: 12,
    [backlog_pb/* StuckReason */.Pwx.AUTONOMOUS_STUCK]: 13,
    [backlog_pb/* StuckReason */.Pwx.BLOCKED_BY_DEPENDENCY]: 14,
    // Sorts just after BLOCKED_BY_DEPENDENCY: both are by-design dequeue skips.
    [backlog_pb/* StuckReason */.Pwx.BLOCKED_BY_CLAIM]: 14.5,
    [backlog_pb/* StuckReason */.Pwx.PLAN_NOT_APPROVED]: 15,
    [backlog_pb/* StuckReason */.Pwx.STALE_WORK]: 16,
    [backlog_pb/* StuckReason */.Pwx.BOUNCING]: 17,
    [backlog_pb/* StuckReason */.Pwx.PR_READY_UNMERGED]: 18,
    [backlog_pb/* StuckReason */.Pwx.LIKELY_FLAKY]: 19,
    [backlog_pb/* StuckReason */.Pwx.UNSPECIFIED]: 20
};
/**
 * Picks the single highest-priority row (lowest `STUCK_REASON_PRIORITY`
 * value) from a list of open StuckBacklogItem rows for ONE backlog item —
 * the shared tie-break every rendering call site must use so two views never
 * disagree on "the" primary reason. Callers pre-filter to one item_id (or
 * pass an already-grouped list from `groupStuckItemsByItemId`); this
 * function does not filter by itemId itself.
 */ function selectPrimaryStuckItem(items) {
    return items.reduce((best, cur)=>{
        if (!best) return cur;
        return STUCK_REASON_PRIORITY[cur.reason] < STUCK_REASON_PRIORITY[best.reason] ? cur : best;
    }, undefined);
}
/** Groups open StuckBacklogItem rows by item_id — a board/list rendering many
 * cards builds this once (O(n)) instead of filtering the full array per card. */ function groupStuckItemsByItemId(items) {
    const map = new Map();
    for (const item of items){
        const group = map.get(item.itemId);
        if (group) group.push(item);
        else map.set(item.itemId, [
            item
        ]);
    }
    return map;
}
/** Resolves the primary reason + any additional open reasons for one item's
 * StuckBacklogItem group. Returns undefined for an empty group (item isn't stuck). */ function summarizeStuckItemGroup(items) {
    const primary = selectPrimaryStuckItem(items);
    if (!primary) return undefined;
    return {
        primary,
        otherReasons: items.filter((i)=>i !== primary).map((i)=>i.reason)
    };
}
/** Derived (not stored) reason label/class for a stale GitHub-status check (design/ux.md Surface 8). */ const PR_STATUS_UNKNOWN_LABEL = "Couldn't check PR status";
const PR_STATUS_UNKNOWN_ICON = "⚪";
const PR_STATUS_UNKNOWN_CLASS = chipUnknown;
/** How stale `last_checked_at` must be, for a `pr_ready_unmerged` item, before the UI treats the GitHub check as failed/unknown rather than trusting the last-known chip. */ const PR_STATUS_STALE_THRESHOLD_MS = 5 * 60 * 1000;
function getStuckReasonLabel(reason) {
    return STUCK_REASON_LABELS[reason] ?? STUCK_REASON_LABELS[backlog_pb/* StuckReason */.Pwx.UNSPECIFIED];
}
function getStuckReasonIcon(reason) {
    return STUCK_REASON_ICONS[reason] ?? STUCK_REASON_ICONS[backlog_pb/* StuckReason */.Pwx.UNSPECIFIED];
}
function getStuckReasonClass(reason) {
    return STUCK_REASON_CLASS[reason] ?? STUCK_REASON_CLASS[backlog_pb/* StuckReason */.Pwx.UNSPECIFIED];
}
function timestampToMs(ts) {
    if (!ts) return null;
    return Number(ts.seconds) * 1000 + Math.floor(ts.nanos / 1e6);
}
/**
 * pr_status_unknown is a derived, UI-only state (never a stored StuckReason):
 * a pr_ready_unmerged item whose last_checked_at is older than the staleness
 * threshold means the reconciler's GitHub poll is failing/stalled, and the
 * chip must never keep showing "PR ready to merge" on stale data.
 */ function isPrStatusUnknown(item, now = Date.now()) {
    if (item.reason !== backlog_pb/* StuckReason */.Pwx.PR_READY_UNMERGED) return false;
    const lastCheckedMs = timestampToMs(item.lastCheckedAt);
    if (lastCheckedMs === null) return true;
    return now - lastCheckedMs > PR_STATUS_STALE_THRESHOLD_MS;
}
/**
 * Mirrors session.MaxRemediationAttempts (session/backlog_remediation.go) —
 * the backoff schedule has 5 entries (30m/2h/8h/24h/72h), so a row with
 * remediation_attempts >= 5 is "parked". Not sourced from the proto response
 * itself since the cap is a backend policy constant, not per-item data.
 */ const MAX_REMEDIATION_ATTEMPTS = 5;
/** Whether automated retries are exhausted and only a manual Reset can unstick this item. */ function isRemediationParked(item) {
    return item.remediationAttempts >= MAX_REMEDIATION_ATTEMPTS;
}
/**
 * Compact "stuck Nd"/"stuck Nh"/"stuck Nm" duration string (no "ago" suffix —
 * distinct from the "last checked Nm ago" phrasing used elsewhere), sourced
 * from a persisted timestamp (first_detected_at) so it survives restarts and
 * is never based on process uptime.
 */ function formatStuckDuration(since, now = Date.now()) {
    const sinceMs = timestampToMs(since);
    if (sinceMs === null) return "unknown";
    const diffMs = Math.max(0, now - sinceMs);
    const diffSec = Math.floor(diffMs / 1000);
    const diffMin = Math.floor(diffSec / 60);
    const diffHour = Math.floor(diffMin / 60);
    const diffDay = Math.floor(diffHour / 24);
    if (diffDay > 0) return `${diffDay}d`;
    if (diffHour > 0) return `${diffHour}h`;
    if (diffMin > 0) return `${diffMin}m`;
    return `${diffSec}s`;
}
/** "YYYY-MM-DD HH:MM UTC" — used for the detail view's "Since:" (first_detected_at) line. */ function formatSinceUTC(ts) {
    const ms = timestampToMs(ts);
    if (ms === null) return "unknown";
    const d = new Date(ms);
    const pad = (n)=>String(n).padStart(2, "0");
    return `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())} ${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())} UTC`;
}
/** "Nm ago" / "Nh ago" phrasing for last-checked / last-updated timestamps. */ function formatAgo(ts, now = Date.now()) {
    const ms = timestampToMs(ts);
    if (ms === null) return "unknown";
    const diffMs = Math.max(0, now - ms);
    const diffSec = Math.floor(diffMs / 1000);
    const diffMin = Math.floor(diffSec / 60);
    const diffHour = Math.floor(diffMin / 60);
    const diffDay = Math.floor(diffHour / 24);
    if (diffSec < 60) return "just now";
    if (diffMin < 60) return `${diffMin}m ago`;
    if (diffHour < 24) return `${diffHour}h ago`;
    return `${diffDay}d ago`;
}


/***/ }),

/***/ 56729:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   J: () => (/* binding */ GitHubBadge)
/* harmony export */ });
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(79147);
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(59634);
/* __next_internal_client_entry_do_not_use__ GitHubBadge auto */ 

/** Map PRPriority string → CSS module class name for the badge variant. */ function priorityClass(priority) {
    switch(priority){
        case "blocking":
            return _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .prBadgeBlocking */ .B2;
        case "ready":
            return _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .prBadgeReady */ .VG;
        case "pending":
            return _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .prBadgePending */ .ho;
        case "draft":
            return _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .prBadgeDraft */ .hC;
        case "complete":
            return _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .prBadgeComplete */ .L7;
        case "auth_error":
        case "error":
            return _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .prBadgeError */ .sv;
        default:
            return _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .prBadgeUnknown */ .o5; // gray: not yet loaded
    }
}
/** Human-readable label for each priority state. */ function priorityLabel(priority, isDraft) {
    if (isDraft) return "Draft";
    switch(priority){
        case "blocking":
            return "Blocked";
        case "ready":
            return "Ready";
        case "pending":
            return "Pending";
        case "draft":
            return "Draft";
        case "complete":
            return "Merged";
        case "auth_error":
            return "Auth Error";
        case "error":
            return "Error";
        default:
            return "";
    }
}
/**
 * Badge component that displays GitHub PR or repository information.
 *
 * Shows:
 * - PR badge with number, link, and priority status (when prNumber > 0)
 * - Repository badge with owner/repo (when no PR but has repo info)
 * - Nothing if no GitHub information is available
 */ function GitHubBadge({ prNumber, prUrl, owner, repo, sourceRef, host, prPriority, prState, isDraft, approvedCount, changesRequestedCount, checkConclusion, compact = false }) {
    // Don't render if no GitHub information
    const hasPR = prNumber && prNumber > 0;
    const hasRepo = owner && repo;
    if (!hasPR && !hasRepo) {
        return null;
    }
    // PR Badge
    if (hasPR) {
        // Construct URL from owner/repo when prUrl is missing (older sessions may not have it stored)
        const resolvedPrUrl = prUrl || (owner && repo ? `https://${host || "github.com"}/${owner}/${repo}/pull/${prNumber}` : undefined);
        const handleClick = (e)=>{
            e.stopPropagation();
            e.preventDefault(); // the anchor's own href+target would otherwise open a second tab
            if (resolvedPrUrl) {
                window.open(resolvedPrUrl, "_blank", "noopener,noreferrer");
            }
        };
        const priorityVariant = priorityClass(prPriority);
        const statusLabel = priorityLabel(prPriority, isDraft);
        // Build tooltip with full status detail
        const tooltipParts = [
            `GitHub PR #${prNumber}`
        ];
        if (prState) tooltipParts.push(`State: ${prState}`);
        if (approvedCount && approvedCount > 0) tooltipParts.push(`${approvedCount} approval${approvedCount > 1 ? "s" : ""}`);
        if (changesRequestedCount && changesRequestedCount > 0) tooltipParts.push(`${changesRequestedCount} change request${changesRequestedCount > 1 ? "s" : ""}`);
        if (checkConclusion) tooltipParts.push(`CI: ${checkConclusion}`);
        const tooltip = tooltipParts.join(" · ");
        return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("a", {
            href: resolvedPrUrl,
            target: "_blank",
            rel: "noopener noreferrer",
            className: `${_GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .badge */ .qS} ${_GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .prBadge */ .Eg} ${priorityVariant} ${compact ? _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .compact */ .oE : ""}`,
            onClick: handleClick,
            title: tooltip,
            "aria-label": `View GitHub Pull Request #${prNumber}${statusLabel ? `, status: ${statusLabel}` : ""}`,
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("svg", {
                    className: _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .icon */ .Kk,
                    viewBox: "0 0 16 16",
                    fill: "currentColor",
                    "aria-hidden": "true",
                    children: /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("path", {
                        d: "M1.5 3.25a2.25 2.25 0 1 1 3 2.122v5.256a2.251 2.251 0 1 1-1.5 0V5.372A2.25 2.25 0 0 1 1.5 3.25Zm5.677-.177L9.573.677A.25.25 0 0 1 10 .854V2.5h1A2.5 2.5 0 0 1 13.5 5v5.628a2.251 2.251 0 1 1-1.5 0V5a1 1 0 0 0-1-1h-1v1.646a.25.25 0 0 1-.427.177L7.177 3.427a.25.25 0 0 1 0-.354ZM3.75 2.5a.75.75 0 1 0 0 1.5.75.75 0 0 0 0-1.5Zm0 9.5a.75.75 0 1 0 0 1.5.75.75 0 0 0 0-1.5Zm8.25.75a.75.75 0 1 0 1.5 0 .75.75 0 0 0-1.5 0Z"
                    })
                }),
                /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("span", {
                    className: _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .text */ .Qq,
                    children: [
                        "#",
                        prNumber
                    ]
                }),
                statusLabel && /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("span", {
                    className: _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .priorityLabel */ .BS,
                    children: statusLabel
                })
            ]
        });
    }
    // Repository Badge (for non-PR GitHub sessions)
    if (hasRepo) {
        const repoUrl = `https://${host || "github.com"}/${owner}/${repo}`;
        const handleClick = (e)=>{
            e.stopPropagation();
            e.preventDefault(); // the anchor's own href+target would otherwise open a second tab
            window.open(repoUrl, "_blank", "noopener,noreferrer");
        };
        return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("a", {
            href: repoUrl,
            target: "_blank",
            rel: "noopener noreferrer",
            className: `${_GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .badge */ .qS} ${_GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .repoBadge */ .m2} ${compact ? _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .compact */ .oE : ""}`,
            onClick: handleClick,
            title: `GitHub Repository: ${owner}/${repo}${sourceRef ? ` (${sourceRef})` : ""}`,
            "aria-label": `View GitHub Repository ${owner}/${repo}`,
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("svg", {
                    className: _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .icon */ .Kk,
                    viewBox: "0 0 16 16",
                    fill: "currentColor",
                    "aria-hidden": "true",
                    children: /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("path", {
                        d: "M8 0c4.42 0 8 3.58 8 8a8.013 8.013 0 0 1-5.45 7.59c-.4.08-.55-.17-.55-.38 0-.27.01-1.13.01-2.2 0-.75-.25-1.23-.54-1.48 1.78-.2 3.65-.88 3.65-3.95 0-.88-.31-1.59-.82-2.15.08-.2.36-1.02-.08-2.12 0 0-.67-.22-2.2.82-.64-.18-1.32-.27-2-.27-.68 0-1.36.09-2 .27-1.53-1.03-2.2-.82-2.2-.82-.44 1.1-.16 1.92-.08 2.12-.51.56-.82 1.28-.82 2.15 0 3.06 1.86 3.75 3.64 3.95-.23.2-.44.55-.51 1.07-.46.21-1.61.55-2.33-.66-.15-.24-.6-.83-1.23-.82-.67.01-.27.38.01.53.34.19.73.9.82 1.13.16.45.68 1.31 2.69.94 0 .67.01 1.3.01 1.49 0 .21-.15.45-.55.38A7.995 7.995 0 0 1 0 8c0-4.42 3.58-8 8-8Z"
                    })
                }),
                /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("span", {
                    className: _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .text */ .Qq,
                    children: compact ? repo : `${owner}/${repo}`
                })
            ]
        });
    }
    return null;
}


/***/ }),

/***/ 60215:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   H: () => (/* binding */ resolveReworkCapOverride)
/* harmony export */ });
/**
 * A rework-cap override value is tri-state: `undefined` means no override is
 * set (global default applies), `0` explicitly means unlimited retries for
 * this item, and any other value is this item's own cap. Resolving that
 * tri-state must use `=== undefined` / `=== 0` checks, never `||`/truthy
 * coercion (which would misreport an explicit 0 as unset) — this type and
 * resolver centralize that check so StuckItemDetail.tsx (the stuck-items
 * panel, which spells out a full sentence) and LifecycleSummary.tsx (the
 * backlog item detail page's compact badge) can't drift into inconsistent
 * tri-state handling even though they render different strings from it.
 */ function resolveReworkCapOverride(value) {
    if (value === undefined) return {
        kind: "unset"
    };
    if (value === 0) return {
        kind: "unlimited"
    };
    return {
        kind: "capped",
        rounds: value
    };
}


/***/ }),

/***/ 90796:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  V: () => (/* binding */ ClaimOverrideForm)
});

// UNUSED EXPORTS: MIN_OVERRIDE_REASON_LENGTH

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FClaimOverrideForm.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA81TwXKCMBC98xW5dKYe4gBWncaP6YQk4NaYTUNQoeO_d4y2BQT10ENPOezb3ffevkzt5r3AOCafESESSqt5zUiu1WEVkfBSCU4JD2gYEairrTlVCm4Z2XH3TOlb8qqa1DVpMzlVttwVYKjHYcAxmp5XJmFljsbTEhrVB8-yMC3U9wqKtb9CFAEhUKPr15L2pjRs2oP0a0aSOH4KNMHQ9WXufGGDXMulBFOMCMu42BQOKyOvtvEbTM46MnRSOUYSeyAlapA92KaFoo5LqMorvctfR-45lvMt6PpKyC4gnLq0K-dBcN22iuUoqjIYhpXXYBQjBo1qsRuUqduGz-6c9oZdaXvOy0MRGba9M2g-HO-xEP_0LULfWCzqSZ9Q_GdXFJUrT6osgvHK9fPXPcl4rB79Isv_KrTHOe58pm8XBvV9dPQxCSXPtJLnaFsuwNeMxNN5m4RBT7nWuFdyFR2_ACdzixUcBQAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FClaimOverrideForm_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81TwXKCMBC98xW5dKYe4gBWncaP6YQk4NaYTUNQoeO_d4y2BQT10ENPOezb3ffevkzt5r3AOCafESESSqt5zUiu1WEVkfBSCU4JD2gYEairrTlVCm4Z2XH3TOlb8qqa1DVpMzlVttwVYKjHYcAxmp5XJmFljsbTEhrVB8_yMC3U9wqKtb9CFAEhUKPr15L2pjRs2oP0a0aSOH4KNMHQ9WXufGGDXMulBFOMCMu42BQOKyOvtvEbTM46MnRSOUYSeyAlapA92KaFoo5LqMorvctfR_45lvMt6PpKyC4gnLq0K_dBcN22iuUoqjIYhpXXYBQjBo1qsRuUqduGz_6c9oZdaXvOy0MRGba9M2g_HO_xEP_0LULfWCzqSZ9Q_GdXFJUrT6osgvHK9fPXPcl4rB79Isv_KrTHOe58pm8XBvV9dPQxCSXPtJLnaFsuwNeMxNN5m4RBT7nWuFdyFR2_ACdzixUcBQAA_22_7D = __webpack_require__(32615);
;// ./src/components/backlog/ClaimOverrideForm.css.ts


var actions = 'pkjgo05';
var cancelButton = 'pkjgo06';
var confirmButton = 'pkjgo07';
var error = 'pkjgo04';
var ClaimOverrideForm_css_form = 'pkjgo00';
var hint = 'pkjgo03';
var ClaimOverrideForm_css_label = 'pkjgo01';
var ClaimOverrideForm_css_textarea = 'pkjgo02';
;// ./src/components/backlog/ClaimOverrideForm.tsx
/* __next_internal_client_entry_do_not_use__ MIN_OVERRIDE_REASON_LENGTH,ClaimOverrideForm auto */ 


/** Matches the server-side minimum (backlog_service_claim.go) and GateVerdictBox. */ const MIN_OVERRIDE_REASON_LENGTH = 5;
/**
 * Reason-capture form for overriding a cross-host claim block. Confirm stays
 * disabled until the trimmed reason reaches MIN_OVERRIDE_REASON_LENGTH,
 * mirroring GateVerdictBox's override form.
 */ function ClaimOverrideForm({ label, confirmLabel, onConfirm, onCancel, busy = false, errorMessage }) {
    const [reason, setReason] = (0,react.useState)("");
    const fieldId = (0,react.useId)();
    const tooShort = reason.trim().length < MIN_OVERRIDE_REASON_LENGTH;
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        role: "form",
        "aria-label": label,
        className: ClaimOverrideForm_css_form,
        "data-testid": "claim-override-form",
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("label", {
                htmlFor: fieldId,
                className: ClaimOverrideForm_css_label,
                children: label
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("textarea", {
                id: fieldId,
                rows: 2,
                value: reason,
                onChange: (e)=>setReason(e.target.value),
                className: ClaimOverrideForm_css_textarea,
                "data-testid": "claim-override-reason"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                className: hint,
                children: [
                    "Enter at least ",
                    MIN_OVERRIDE_REASON_LENGTH,
                    " characters to continue. The reason is audit-logged."
                ]
            }),
            errorMessage && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                className: error,
                role: "alert",
                "data-testid": "claim-override-error",
                children: errorMessage
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: actions,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                        type: "button",
                        className: cancelButton,
                        onClick: onCancel,
                        "data-testid": "claim-override-cancel",
                        children: "Cancel"
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                        type: "button",
                        className: confirmButton,
                        disabled: tooShort || busy,
                        onClick: ()=>void onConfirm(reason.trim()),
                        "data-testid": "claim-override-confirm",
                        children: confirmLabel
                    })
                ]
            })
        ]
    });
}


/***/ })

};
;