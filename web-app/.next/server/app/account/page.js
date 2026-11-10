(() => {
var exports = {};
exports.id = 1298;
exports.ids = [1298];
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

/***/ 8086:
/***/ ((module) => {

"use strict";
module.exports = require("module");

/***/ }),

/***/ 9273:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 42663));


/***/ }),

/***/ 10846:
/***/ ((module) => {

"use strict";
module.exports = require("next/dist/compiled/next-server/app-page.runtime.prod.js");

/***/ }),

/***/ 13488:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
__webpack_require__.r(__webpack_exports__);
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   "default": () => (/* binding */ AccountLayout),
/* harmony export */   metadata: () => (/* binding */ metadata)
/* harmony export */ });
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(12129);
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__);

const metadata = {
    title: "Account - Stapler Squad",
    description: "Manage your registered passkeys and add new devices."
};
function AccountLayout({ children }) {
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.Fragment, {
        children: children
    });
}


/***/ }),

/***/ 19121:
/***/ ((module) => {

"use strict";
module.exports = require("next/dist/server/app-render/action-async-storage.external.js");

/***/ }),

/***/ 21820:
/***/ ((module) => {

"use strict";
module.exports = require("os");

/***/ }),

/***/ 22425:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 57552));


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

/***/ 33873:
/***/ ((module) => {

"use strict";
module.exports = require("path");

/***/ }),

/***/ 42663:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
__webpack_require__.r(__webpack_exports__);
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   "default": () => (__WEBPACK_DEFAULT_EXPORT__)
/* harmony export */ });
/* harmony import */ var react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(70679);
/* harmony import */ var react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__);

/* harmony default export */ const __WEBPACK_DEFAULT_EXPORT__ = ((0,react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__.registerClientReference)(
function() { throw new Error("Attempted to call the default export of \"/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/account/page.tsx\" from the server, but it's on the client. It's not possible to invoke a client function from the server, it can only be rendered as a Component or passed to props of a Client Component."); },
"/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/account/page.tsx",
"default",
));


/***/ }),

/***/ 55591:
/***/ ((module) => {

"use strict";
module.exports = require("https");

/***/ }),

/***/ 57552:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  "default": () => (/* binding */ AccountPage)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
// EXTERNAL MODULE: ./src/lib/analytics/usePageView.ts
var usePageView = __webpack_require__(15116);
// EXTERNAL MODULE: ./src/lib/nav/index.ts
var nav = __webpack_require__(7220);
// EXTERNAL MODULE: ./src/lib/contexts/AuthContext.tsx
var AuthContext = __webpack_require__(58238);
// EXTERNAL MODULE: ./src/lib/routes.ts + 1 modules
var routes = __webpack_require__(65539);
// EXTERNAL MODULE: ./src/lib/auth/passkey.ts
var passkey = __webpack_require__(37889);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Faccount%2Faccount.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA71XyXLjIBS85yu4TFV8kEso8UY-JoUFskkQaADHy9T8-5SQZAvJaPEyJ1WxPB7d_R6t6SdMYh3NcQj-vACQ4kOwZ8RsEViEYXb4sGNqwwQCIcA7I_ORDBPCxAaBH6xeg-ATrugpUqe3t0lzBE7y9YTpjOMjAgmnNmT-DQhTNDZMCgRiyXepyGc2OLsS9uPl78u0yhTaTO-MCd2YkY2ZSGECzU60tZzYe9j5PWWbrUFgHob5WCy5VM314cQBroZZsJbGyLS5ITrZHWupCFXnNTA7AC05I43V1E3-7QGA2ARqMd-vx8ScbUTADE01AjEVhqp8-GunDUuOQSyFocIgoDMc02BNzZ5S0SWZ0COZNY6_N0ruBGluWdSQ6obojKfChO106-wi0gB1zB4B8LHQBBNVgYXOGfMeBcZtBc56FSh_qEq43COwZYQUTBh6MMFlgnLOMs10PrXfMkMDyx0CQu4VzpwcFz054ok3n8hFdGkjeUQRnUaLIvWfPHP0IqSgQ5Xhv-ja3w52Suc5ZJJVxWEUFpoVEpEZjpk5gnAKZ9pFBG1zUpCQ5hURpvGaUzKxMJWbEAiny1ljU7WyufB9Vs9GSBNgzuWeEmf_aiQNYYsGe7kMKyqMlwEYDipZZ9mtzGB_mXQzgzm_wsrKz4pfjLDRPlZ3soT_D0sP7qujqucejnDBkQWpoxf6-Vq6dK0LuGV1asIOOSEAMKGpKZ9zf7RCgaeACUIPCMCouNrNT-llyveGNuQW98hz2AP6NYDo5aQrrcjOlo8dDMNfH67BfF-WBvOxVo482crVjqLXfcG1JIswLXLtTalw6z15gsHdjE3Vkac9WhusjBN0W0BtJ7eKie-yOCrOpzNF03xgWwJ9GekW1ure537UA7F4UIXWkGFnZBCA92u86c6_ejTe4U_Gavz7Ps8HAGeCBjUB-I_iwzUKuzVawpsbWARaNjYdW18eIfhcfu0oYY-q6iEqG94ZjWqguxrmY17mzrZfy0zey6v9j7DIXK2AbDCZ5b_vVZBrAX83S-qm3OGlMhKcMn5sZbN72J-T37Mdm56t-v_3e0o8SATfA17tuSsE9WQhaL8xKx1_u0_Vtpvb0zv0YHocBOlhIKTjHXq7M9buvRvU4kfS0mXWegzEz-08wD4DDuGTMK7lv7f5F01GU540TM0_TSdpPAgVAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Faccount_2Faccount_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA71XyXLjIBS85yu4TFV8kEso8UY_JoUFskkQaADHy9T8_5SQZAvJaPEyJ1WxPB7d_R6t6SdMYh3NcQj_vACQ4kOwZ8RsEViEYXb4sGNqwwQCIcA7I_ORDBPCxAaBH6xeg_ATrugpUqe3t0lzBE7y9YTpjOMjAgmnNmT_DQhTNDZMCgRiyXepyGc2OLsS9uPl78u0yhTaTO_MCd2YkY2ZSGECzU60tZzYe9j5PWWbrUFgHob5WCy5VM314cQBroZZsJbGyLS5ITrZHWupCFXnNTA7AC05I43V1E3_7QGA2ARqMd_vx8ScbUTADE01AjEVhqp8_GunDUuOQSyFocIgoDMc02BNzZ5S0SWZ0COZNY6_N0ruBGluWdSQ6obojKfChO106_wi0gB1zB4B8LHQBBNVgYXOGfMeBcZtBc56FSh_qEq43COwZYQUTBh6MMFlgnLOMs10PrXfMkMDyx0CQu4VzpwcFz054ok3n8hFdGkjeUQRnUaLIvWfPHP0IqSgQ5Xhv_ja3w52Suc5ZJJVxWEUFpoVEpEZjpk5gnAKZ9pFBG1zUpCQ5hURpvGaUzKxMJWbEAiny1ljU7WyufB9Vs9GSBNgzuWeEmf_aiQNYYsGe7kMKyqMlwEYDipZZ9mtzGB_mXQzgzm_wsrKz4pfjLDRPlZ3soT_D0sP7qujqucejnDBkQWpoxf6_Vq6dK0LuGV1asIOOSEAMKGpKZ9zf7RCgaeACUIPCMCouNrNT_llyveGNuQW98hz2AP6NYDo5aQrrcjOlo8dDMNfH67BfF_WBvOxVo482crVjqLXfcG1JIswLXLtTalw6z15gsHdjE3Vkac9WhusjBN0W0BtJ7eKie_yOCrOpzNF03xgWwJ9GekW1ure537UA7F4UIXWkGFnZBCA92u86c6_ejTe4U_Gavz7Ps8HAGeCBjUB_I_iwzUKuzVawpsbWARaNjYdW18eIfhcfu0oYY_q6iEqG94ZjWqguxrmY17mzrZfy0zey6v9j7DIXK2AbDCZ5b_vVZBrAX83S_qm3OGlMhKcMn5sZbN72J_T37Mdm56t_v_3e0o8SATfA17tuSsE9WQhaL8xKx1_u0_Vtpvb0zv0YHocBOlhIKTjHXq7M9buvRvU4kfS0mXWegzEz_08wD4DDuGTMK7lv7f5F01GU540TM0_TSdpPAgVAAA_22_7D = __webpack_require__(75955);
;// ./src/app/account/account.css.ts


var countdown = '_1fcs26ar';
var countdownExpired = '_1fcs26as';
var credentialInfo = '_1fcs26a5';
var credentialList = '_1fcs26a3';
var credentialMeta = '_1fcs26a7';
var credentialName = '_1fcs26a6';
var credentialRow = '_1fcs26a4';
var dangerButton = '_1fcs26a9';
var emptyState = '_1fcs26au';
var errorText = '_1fcs26av';
var ghostButton = '_1fcs26aa';
var modal = '_1fcs26ac';
var modalActions = '_1fcs26ae';
var modalOverlay = '_1fcs26ab';
var modalTitle = '_1fcs26ad';
var page = '_1fcs26a0';
var primaryButton = '_1fcs26a8';
var qrBlock = '_1fcs26am';
var qrImage = '_1fcs26an';
var qrLabel = '_1fcs26ao';
var qrRow = '_1fcs26al';
var section = '_1fcs26a1';
var sectionTitle = '_1fcs26a2';
var step = '_1fcs26ag';
var stepContent = '_1fcs26ai';
var stepDesc = '_1fcs26ak';
var stepList = '_1fcs26af';
var stepNumber = '_1fcs26ah';
var stepTitle = '_1fcs26aj';
var urlRow = '_1fcs26ap';
var urlShowButton = '_1fcs26aw';
var urlText = '_1fcs26aq';
var warningText = '_1fcs26at';
;// ./src/app/account/page.tsx
/* __next_internal_client_entry_do_not_use__ default auto */ 







// ─── Countdown ────────────────────────────────────────────────────────────────
function Countdown({ ttlSeconds }) {
    const [left, setLeft] = (0,react.useState)(ttlSeconds);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (left <= 0) return;
        const id = setInterval(()=>{
            setLeft((n)=>{
                if (n <= 1) {
                    clearInterval(id);
                    return 0;
                }
                return n - 1;
            });
        }, 1000);
        return ()=>clearInterval(id);
    }, []); // eslint-disable-line react-hooks/exhaustive-deps
    if (left <= 0) {
        return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
            className: `${countdown} ${countdownExpired}`,
            children: "Invite expired — generate a new one"
        });
    }
    const m = Math.floor(left / 60);
    const sec = left % 60;
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("p", {
        className: countdown,
        children: [
            "Expires in ",
            m,
            ":",
            String(sec).padStart(2, "0")
        ]
    });
}
// ─── Add Device Modal ─────────────────────────────────────────────────────────
function AddDeviceModal({ onClose, onSuccess }) {
    const [label, setLabel] = (0,react.useState)("");
    const [invite, setInvite] = (0,react.useState)(null);
    const [showUrl, setShowUrl] = (0,react.useState)(false);
    const [loading, setLoading] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)("");
    const [copied, setCopied] = (0,react.useState)(false);
    const generate = async ()=>{
        setLoading(true);
        setError("");
        try {
            const data = await (0,passkey/* generateInvite */.Ds)(label || "New Device");
            setInvite(data);
        } catch (e) {
            setError(e instanceof Error ? e.message : String(e));
        } finally{
            setLoading(false);
        }
    };
    const copyUrl = async ()=>{
        if (!invite) return;
        await navigator.clipboard.writeText(invite.registration_url);
        setCopied(true);
        setTimeout(()=>setCopied(false), 2000);
    };
    if (!invite) {
        return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
            className: modalOverlay,
            onClick: (e)=>e.target === e.currentTarget && onClose(),
            children: /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: modal,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("h2", {
                        className: modalTitle,
                        children: "Add New Device"
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                        className: stepDesc,
                        children: "Give this device a name so you can identify it later (e.g. “iPhone 15”)."
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                        type: "text",
                        placeholder: "Device name (optional)",
                        "aria-label": "Device name (optional)",
                        value: label,
                        onChange: (e)=>setLabel(e.target.value),
                        onKeyDown: (e)=>e.key === "Enter" && generate(),
                        style: {
                            width: "100%",
                            padding: "0.5rem 0.75rem",
                            borderRadius: "6px",
                            border: "1px solid var(--border-color)",
                            background: "var(--input-background)",
                            color: "var(--input-text)",
                            fontSize: "0.875rem",
                            boxSizing: "border-box"
                        }
                    }),
                    error && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                        className: errorText,
                        children: error
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: modalActions,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: ghostButton,
                                onClick: onClose,
                                children: "Cancel"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: primaryButton,
                                onClick: generate,
                                disabled: loading,
                                children: loading ? "Generating…" : "Generate Invite"
                            })
                        ]
                    })
                ]
            })
        });
    }
    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
        className: modalOverlay,
        onClick: (e)=>e.target === e.currentTarget && onClose(),
        children: /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
            className: modal,
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("h2", {
                    className: modalTitle,
                    children: "Add New Device"
                }),
                /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("ol", {
                    className: stepList,
                    children: [
                        /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("li", {
                            className: step,
                            children: [
                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                    className: stepNumber,
                                    children: "1"
                                }),
                                /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                    className: stepContent,
                                    children: [
                                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                                            className: stepTitle,
                                            children: "Install the CA certificate on the new device"
                                        }),
                                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                                            className: stepDesc,
                                            children: "Scan the left QR code to download the certificate, then install it:"
                                        }),
                                        /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("p", {
                                            className: stepDesc,
                                            children: [
                                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("strong", {
                                                    children: "iOS"
                                                }),
                                                ": Download → Settings → General → VPN & Device Management → [cert] → Install. Then Settings → General → About → Certificate Trust Settings → enable."
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("p", {
                                            className: stepDesc,
                                            children: [
                                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("strong", {
                                                    children: "Android"
                                                }),
                                                ": Download → Settings → Security → Encryption & credentials → Install a certificate → CA Certificate."
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("p", {
                                            className: stepDesc,
                                            children: [
                                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("strong", {
                                                    children: "macOS / Windows"
                                                }),
                                                ": Download and double-click the .pem file to add it to the trust store."
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                                            className: qrRow,
                                            children: /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                                className: qrBlock,
                                                children: [
                                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("img", {
                                                        src: invite.ca_qr_data_url,
                                                        alt: "CA cert QR code",
                                                        className: qrImage
                                                    }),
                                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                                        className: qrLabel,
                                                        children: "CA Certificate"
                                                    })
                                                ]
                                            })
                                        })
                                    ]
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("li", {
                            className: step,
                            children: [
                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                    className: stepNumber,
                                    children: "2"
                                }),
                                /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                    className: stepContent,
                                    children: [
                                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                                            className: stepTitle,
                                            children: "Register a passkey on the new device"
                                        }),
                                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                                            className: stepDesc,
                                            children: "Scan the QR code with the new device’s camera, then follow the browser prompt."
                                        }),
                                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                                            className: qrRow,
                                            children: /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                                className: qrBlock,
                                                children: [
                                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("img", {
                                                        src: invite.reg_qr_data_url,
                                                        alt: "Registration QR code",
                                                        className: qrImage
                                                    }),
                                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                                        className: qrLabel,
                                                        children: "Registration"
                                                    })
                                                ]
                                            })
                                        }),
                                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)(Countdown, {
                                            ttlSeconds: invite.ttl_seconds
                                        }),
                                        showUrl ? /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                            className: urlRow,
                                            children: [
                                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                                    className: urlText,
                                                    title: invite.registration_url,
                                                    children: invite.registration_url
                                                }),
                                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                                    className: ghostButton,
                                                    onClick: copyUrl,
                                                    children: copied ? "Copied!" : "Copy"
                                                })
                                            ]
                                        }) : /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                            className: `${ghostButton} ${urlShowButton}`,
                                            onClick: ()=>setShowUrl(true),
                                            children: "Can’t scan? Show URL"
                                        })
                                    ]
                                })
                            ]
                        })
                    ]
                }),
                /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                    className: modalActions,
                    children: [
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                            className: ghostButton,
                            onClick: ()=>{
                                setInvite(null);
                                setLabel("");
                            },
                            children: "Regenerate"
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                            className: primaryButton,
                            onClick: ()=>{
                                onSuccess();
                                onClose();
                            },
                            children: "Done"
                        })
                    ]
                })
            ]
        })
    });
}
// ─── Revoke Confirm Dialog ────────────────────────────────────────────────────
function RevokeConfirmDialog({ credential, isLast, onConfirm, onCancel }) {
    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
        className: modalOverlay,
        onClick: (e)=>e.target === e.currentTarget && onCancel(),
        children: /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
            className: modal,
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("h2", {
                    className: modalTitle,
                    children: "Revoke passkey?"
                }),
                /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("p", {
                    className: stepDesc,
                    children: [
                        "Remove ",
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("strong", {
                            children: credential.display_name || "this passkey"
                        }),
                        "? The device will no longer be able to sign in."
                    ]
                }),
                isLast && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                    className: warningText,
                    children: "This is your last passkey. Revoking it will sign out all active sessions and lock you out of remote access until you use the CLI to register a new passkey."
                }),
                /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                    className: modalActions,
                    children: [
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                            className: ghostButton,
                            onClick: onCancel,
                            children: "Cancel"
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                            className: dangerButton,
                            onClick: onConfirm,
                            children: isLast ? "Revoke and lock out" : "Revoke"
                        })
                    ]
                })
            ]
        })
    });
}
// ─── Credential Row ───────────────────────────────────────────────────────────
function CredentialRow({ cred, isLast, onRevoke }) {
    const createdAt = cred.created_at ? new Date(cred.created_at).toLocaleDateString(undefined, {
        year: "numeric",
        month: "short",
        day: "numeric"
    }) : "Unknown";
    const lastUsed = cred.last_used_at ? new Date(cred.last_used_at).toLocaleDateString(undefined, {
        year: "numeric",
        month: "short",
        day: "numeric"
    }) : "Never";
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: credentialRow,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: credentialInfo,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                        className: credentialName,
                        children: cred.display_name || "Passkey"
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                        className: credentialMeta,
                        children: [
                            "Registered ",
                            createdAt,
                            " \xb7 Last used ",
                            lastUsed
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                className: dangerButton,
                onClick: ()=>onRevoke(cred),
                title: "Revoke this passkey",
                children: "Revoke"
            })
        ]
    });
}
// ─── Account Page ─────────────────────────────────────────────────────────────
function AccountPage() {
    (0,usePageView/* usePageView */.O)();
    const router = (0,nav/* useRouter */.KA)();
    const { authEnabled, authenticated, loading: authLoading } = (0,AuthContext/* useAuth */.A)();
    const [credentials, setCredentials] = (0,react.useState)([]);
    const [credsLoading, setCredsLoading] = (0,react.useState)(true);
    const [credsError, setCredsError] = (0,react.useState)("");
    const [showAddModal, setShowAddModal] = (0,react.useState)(false);
    const [revoking, setRevoking] = (0,react.useState)(null);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (!authLoading && authEnabled && !authenticated) {
            router.replace(routes/* routes */.J.login);
        }
    }, [
        authLoading,
        authEnabled,
        authenticated,
        router
    ]);
    const loadCredentials = (0,react.useCallback)(async ()=>{
        setCredsLoading(true);
        setCredsError("");
        try {
            const data = await (0,passkey/* listCredentials */.Jc)();
            setCredentials(data);
        } catch (e) {
            setCredsError(e instanceof Error ? e.message : String(e));
        } finally{
            setCredsLoading(false);
        }
    }, []);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (!authLoading && authEnabled && authenticated) {
            loadCredentials();
        }
    }, [
        authLoading,
        authEnabled,
        authenticated,
        loadCredentials
    ]);
    const handleRevoke = async (cred)=>{
        try {
            const result = await (0,passkey/* revokeCredential */.Ic)(cred.id);
            setRevoking(null);
            if (result.last_credential) {
                // Session cookie was invalidated server-side; redirect to login.
                router.replace(routes/* routes */.J.login);
                return;
            }
            await loadCredentials();
        } catch (e) {
            setCredsError(e instanceof Error ? e.message : String(e));
            setRevoking(null);
        }
    };
    if (authLoading) {
        return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("main", {
            id: "main-content",
            className: page,
            children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                className: emptyState,
                children: "Loading…"
            })
        });
    }
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("main", {
        id: "main-content",
        className: page,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("section", {
                className: section,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("h2", {
                        className: sectionTitle,
                        children: "Your Passkeys"
                    }),
                    credsLoading ? /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                        className: emptyState,
                        children: "Loading passkeys…"
                    }) : credsError ? /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                        className: errorText,
                        children: credsError
                    }) : credentials.length === 0 ? /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                        className: emptyState,
                        children: "No passkeys registered."
                    }) : /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                        className: credentialList,
                        children: credentials.map((cred)=>/*#__PURE__*/ (0,react_jsx_runtime.jsx)(CredentialRow, {
                                cred: cred,
                                isLast: credentials.length === 1,
                                onRevoke: (c)=>setRevoking(c)
                            }, cred.id))
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                        children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                            className: primaryButton,
                            onClick: ()=>setShowAddModal(true),
                            children: "Add New Device"
                        })
                    })
                ]
            }),
            showAddModal && /*#__PURE__*/ (0,react_jsx_runtime.jsx)(AddDeviceModal, {
                onClose: ()=>setShowAddModal(false),
                onSuccess: loadCredentials
            }),
            revoking && /*#__PURE__*/ (0,react_jsx_runtime.jsx)(RevokeConfirmDialog, {
                credential: revoking,
                isLast: credentials.length === 1,
                onConfirm: ()=>handleRevoke(revoking),
                onCancel: ()=>setRevoking(null)
            })
        ]
    });
}


/***/ }),

/***/ 62722:
/***/ (() => {



/***/ }),

/***/ 63033:
/***/ ((module) => {

"use strict";
module.exports = require("next/dist/server/app-render/work-unit-async-storage.external.js");

/***/ }),

/***/ 69242:
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
const module4 = () => Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 13488));
const page5 = () => Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 42663));


// We inject the tree and pages here so that we can use them in the route
// module.
const tree = {
        children: [
        '',
        {
        children: [
        'account',
        {
        children: ['__PAGE__', {}, {
          page: [page5, "/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/account/page.tsx"],
          
        }]
      },
        {
        'layout': [module4, "/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/account/layout.tsx"],
        
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
const pages = ["/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/account/page.tsx"];


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
        page: "/account/page",
        pathname: "/account",
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

/***/ 75955:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "._1fcs26a0 {\n  max-width: 700px;\n  margin: 0 auto;\n  padding: var(--_19ez2rz33) var(--_19ez2rz31);\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz33);\n}\n._1fcs26a1 {\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz31);\n}\n._1fcs26a2 {\n  font-size: var(--_19ez2rz3d);\n  font-weight: 600;\n  color: var(--_19ez2rz0);\n  margin: 0;\n  padding-bottom: var(--_19ez2rz2z);\n  border-bottom: 1px solid var(--_19ez2rze);\n}\n._1fcs26a3 {\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz2z);\n}\n._1fcs26a4 {\n  display: flex;\n  align-items: center;\n  justify-content: space-between;\n  padding: var(--_19ez2rz30) var(--_19ez2rz31);\n  background: var(--_19ez2rz7);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: var(--_19ez2rz37);\n  gap: var(--_19ez2rz31);\n}\n._1fcs26a5 {\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz2y);\n  min-width: 0;\n}\n._1fcs26a6 {\n  font-size: var(--_19ez2rz3c);\n  font-weight: 500;\n  color: var(--_19ez2rz0);\n  overflow: hidden;\n  text-overflow: ellipsis;\n  white-space: nowrap;\n}\n._1fcs26a7 {\n  font-size: var(--_19ez2rz3a);\n  color: var(--_19ez2rz2);\n}\n._1fcs26a8 {\n  padding: var(--_19ez2rz2z) var(--_19ez2rz31);\n  background: var(--_19ez2rzm);\n  color: var(--_19ez2rz5);\n  border: none;\n  border-radius: var(--_19ez2rz37);\n  font-size: var(--_19ez2rz3b);\n  font-weight: 600;\n  cursor: pointer;\n  transition: opacity 0.15s;\n}\n._1fcs26a8:hover:not(:disabled) {\n  opacity: 0.85;\n}\n._1fcs26a8:disabled {\n  opacity: 0.45;\n  cursor: not-allowed;\n}\n._1fcs26a9 {\n  padding: var(--_19ez2rz2z) var(--_19ez2rz30);\n  background: transparent;\n  color: var(--_19ez2rz10);\n  border: 1px solid var(--_19ez2rz10);\n  border-radius: var(--_19ez2rz37);\n  font-size: var(--_19ez2rz3a);\n  font-weight: 500;\n  cursor: pointer;\n  transition: all 0.15s;\n}\n._1fcs26a9:hover:not(:disabled) {\n  background: var(--_19ez2rz11);\n}\n._1fcs26a9:disabled {\n  opacity: 0.45;\n  cursor: not-allowed;\n}\n._1fcs26aa {\n  padding: var(--_19ez2rz2z) var(--_19ez2rz30);\n  background: transparent;\n  color: var(--_19ez2rz1);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: var(--_19ez2rz37);\n  font-size: var(--_19ez2rz3b);\n  font-weight: 500;\n  cursor: pointer;\n  transition: all 0.15s;\n}\n._1fcs26aa:hover {\n  color: var(--_19ez2rz0);\n  background: var(--_19ez2rz8);\n}\n._1fcs26ab {\n  position: fixed;\n  inset: 0;\n  background: var(--_19ez2rza);\n  z-index: 1200;\n  display: flex;\n  align-items: center;\n  justify-content: center;\n  padding: var(--_19ez2rz31);\n}\n._1fcs26ac {\n  background: var(--_19ez2rz7);\n  border: 1px solid var(--_19ez2rzj);\n  border-radius: var(--_19ez2rz38);\n  padding: var(--_19ez2rz32);\n  width: 100%;\n  max-width: 480px;\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz31);\n}\n._1fcs26ad {\n  font-size: var(--_19ez2rz3d);\n  font-weight: 600;\n  color: var(--_19ez2rz0);\n  margin: 0;\n}\n._1fcs26ae {\n  display: flex;\n  gap: var(--_19ez2rz30);\n  justify-content: flex-end;\n}\n._1fcs26af {\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz31);\n}\n._1fcs26ag {\n  display: flex;\n  gap: var(--_19ez2rz30);\n  align-items: flex-start;\n}\n._1fcs26ah {\n  flex-shrink: 0;\n  width: 1.5rem;\n  height: 1.5rem;\n  border-radius: var(--_19ez2rz39);\n  background: var(--_19ez2rzm);\n  color: var(--_19ez2rz5);\n  font-size: var(--_19ez2rz3a);\n  font-weight: 700;\n  display: flex;\n  align-items: center;\n  justify-content: center;\n}\n._1fcs26ai {\n  flex: 1 1;\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz2z);\n}\n._1fcs26aj {\n  font-size: var(--_19ez2rz3b);\n  font-weight: 600;\n  color: var(--_19ez2rz0);\n  margin: 0;\n}\n._1fcs26ak {\n  font-size: var(--_19ez2rz3a);\n  color: var(--_19ez2rz2);\n  line-height: 1.5;\n  margin: 0;\n}\n._1fcs26al {\n  display: flex;\n  gap: var(--_19ez2rz31);\n  align-items: flex-start;\n  flex-wrap: wrap;\n}\n._1fcs26am {\n  display: flex;\n  flex-direction: column;\n  align-items: center;\n  gap: var(--_19ez2rz2y);\n}\n._1fcs26an {\n  width: 120px;\n  height: 120px;\n  border-radius: var(--_19ez2rz36);\n  border: 1px solid var(--_19ez2rze);\n  background: var(--_19ez2rz7);\n}\n._1fcs26ao {\n  font-size: var(--_19ez2rz3a);\n  color: var(--_19ez2rz2);\n  text-align: center;\n}\n._1fcs26ap {\n  display: flex;\n  gap: var(--_19ez2rz2z);\n  align-items: center;\n}\n._1fcs26aq {\n  flex: 1 1;\n  font-size: var(--_19ez2rz3a);\n  color: var(--_19ez2rz1);\n  font-family: var(--_19ez2rz2u);\n  overflow: hidden;\n  text-overflow: ellipsis;\n  white-space: nowrap;\n  padding: var(--_19ez2rz2y) var(--_19ez2rz2z);\n  background: var(--_19ez2rz1a);\n  border: 1px solid var(--_19ez2rzk);\n  border-radius: var(--_19ez2rz36);\n}\n._1fcs26ar {\n  font-size: var(--_19ez2rz3a);\n  color: var(--_19ez2rz2);\n  text-align: center;\n}\n._1fcs26as {\n  color: var(--_19ez2rz10);\n  font-weight: 600;\n}\n._1fcs26at {\n  font-size: var(--_19ez2rz3a);\n  color: var(--_19ez2rzx);\n  background: var(--_19ez2rzy);\n  border: 1px solid var(--_19ez2rzx);\n  border-radius: var(--_19ez2rz36);\n  padding: var(--_19ez2rz2z) var(--_19ez2rz30);\n  line-height: 1.5;\n}\n._1fcs26au {\n  font-size: var(--_19ez2rz3b);\n  color: var(--_19ez2rz2);\n  text-align: center;\n  padding: var(--_19ez2rz32) var(--_19ez2rz31);\n}\n._1fcs26av {\n  font-size: var(--_19ez2rz3a);\n  color: var(--_19ez2rz10);\n  background: var(--_19ez2rz11);\n  border-radius: var(--_19ez2rz36);\n  padding: var(--_19ez2rz2z) var(--_19ez2rz30);\n}\n._1fcs26aw {\n  align-self: flex-start;\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


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

/***/ 94266:
/***/ (() => {



/***/ })

};
;

// load runtime
var __webpack_require__ = require("../../webpack-runtime.js");
__webpack_require__.C(exports);
var __webpack_exec__ = (moduleId) => (__webpack_require__(__webpack_require__.s = moduleId))
var __webpack_exports__ = __webpack_require__.X(0, [4655,4164], () => (__webpack_exec__(69242)));
module.exports = __webpack_exports__;

})();