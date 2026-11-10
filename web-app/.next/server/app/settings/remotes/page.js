(() => {
var exports = {};
exports.id = 1150;
exports.ids = [1150];
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

/***/ 3802:
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
const module4 = () => Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 84040));
const page5 = () => Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 34463));


// We inject the tree and pages here so that we can use them in the route
// module.
const tree = {
        children: [
        '',
        {
        children: [
        'settings',
        {
        children: [
        'remotes',
        {
        children: ['__PAGE__', {}, {
          page: [page5, "/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/settings/remotes/page.tsx"],
          
        }]
      },
        {
        
        
      }
      ]
      },
        {
        'layout': [module4, "/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/settings/layout.tsx"],
        
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
const pages = ["/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/settings/remotes/page.tsx"];


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
        page: "/settings/remotes/page",
        pathname: "/settings/remotes",
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

/***/ 16658:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  "default": () => (/* binding */ RemotesPage)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/connect-error.js
var connect_error = __webpack_require__(22800);
// EXTERNAL MODULE: ./src/lib/hooks/useRemotesService.ts
var useRemotesService = __webpack_require__(80888);
// EXTERNAL MODULE: ./src/components/analytics/PageViewTracker.tsx
var PageViewTracker = __webpack_require__(88486);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-dom.js
var react_dom = __webpack_require__(68051);
// EXTERNAL MODULE: ./src/lib/hooks/useDialogFocusTrap.ts
var useDialogFocusTrap = __webpack_require__(54183);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsettings%2FHostKeyTrustDialog.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA91Wy3LjIBC8-yvmslX2gZSkOI5DLvsnKSxGMgkCBZCteMv_viUk23rG2qqc9oQEDT3T0zwe3sKAfW4sC-DPAiDXVjihFYVElMhfFwBCWXQUgup7x-KP1OhCcRJrqQ2FAzNLQt7CFzxF5sRWFYoLm0v2RSGRWFYdTIpUEeEwsxRiVA5N1f1eWCeSLxJr5VC59tCJCMWxpBAGgWfOGedCpX3Cx3D1ujgvHi5ZhD6Le2G--DB32nA0FMK8BKul4D3UewtFDOOisAP6bYMpid0zro8DQLb6LvrIj2asJEfB3Z7CehvkXrLmP2YyXoZB8AsIRAazcX2rlnBhMK5rF2tZZKoaSVl-T7LIS5Zo5YgVJxzAuef040cU6d4NEHuPGFU6aBI0qVDeQy3iR088Oi28cY7GFPeXBZBCIdk3AYYPTx2qtae66pYa4a1dtcRhlkvmkNSqWQqscBrCxDRZFZkiYzrWuRl9HBuNTsMQp1xQL3QzbR8Qz7IrzrDrc7f0T3dKv7tb-nS69NE3pd_MIp7nKICjNpzsDLIPCr4hTMoO3_ONL2GZkF-DYIuuMtuuYS4bbdoGg7PM70lU_BYtcXrcJi3el1nnVzYtz2fHK0ornOeKSXdGp9XYAXJ_d86wTWFslUGuxeXcd4apyw3Um6Pqwgt13eXrdXVWttWje31AM0tD1ROeJjouLDkIK3YS_RK6cNWpQiGa2HJ1HRoY0Unib8qoFxWbiMfnmjODyk2WM_zRrf9fFJn9Q5G3bWFGEWLVW_ynbfA7Qy4YLFuX_CYI8nLl1-49XEYfYAASk8sLDMDUujR_O-2czq6_LZbq1VB3DjoaQeq5pFr96ppgDOEpu5BzO_htE_z4O4QYPKCxWE87_wUA5jxGcgoAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsettings_2FHostKeyTrustDialog_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA91Wy3LjIBC8_yvmslX2gZSkOI5DLvsnKSxGMgkCBZCteMv_viUk23rG2qqc9oQEDT3T0zwe3sKAfW4sC_DPAiDXVjihFYVElMhfFwBCWXQUgup7x_KP1OhCcRJrqQ2FAzNLQt7CFzxF5sRWFYoLm0v2RSGRWFYdTIpUEeEwsxRiVA5N1f1eWCeSLxJr5VC59tCJCMWxpBAGgWfOGedCpX3Cx3D1ujgvHi5ZhD6Le2G_DB32nA0FMK8BKul4D3UewtFDOOisAP6bYMpid0zro8DQLb6LvrIj2asJEfB3Z7CehvkXrLmP2YyXoZB8AsIRAazcX2rlnBhMK5rF2tZZKoaSVl_T7LIS5Zo5YgVJxzAuef040cU6d4NEHuPGFU6aBI0qVDeQy3iR088Oi28cY7GFPeXBZBCIdk3AYYPTx2qtae66pYa4a1dtcRhlkvmkNSqWQqscBrCxDRZFZkiYzrWuRl9HBuNTsMQp1xQL3QzbR8Qz7IrzrDrc7f0T3dKv7tb_nS69NE3pd_MIp7nKICjNpzsDLIPCr4hTMoO3_ONL2GZkF_DYIuuMtuuYS4bbdoGg7PM70lU_BYtcXrcJi3el1nnVzYtz2fHK0ornOeKSXdGp9XYAXJ_d86wTWFslUGuxeXcd4apyw3Um6Pqwgt13eXrdXVWttWje31AM0tD1ROeJjouLDkIK3YS_RK6cNWpQiGa2HJ1HRoY0Unib8qoFxWbiMfnmjODyk2WM_zRrf9fFJn9Q5G3bWFGEWLVW_ynbfA7Qy4YLFuX_CYI8nLl1_49XEYfYAASk8sLDMDUujR_O_2czq6_LZbq1VB3DjoaQeq5pFr96ppgDOEpu5BzO_htE_z4O4QYPKCxWE87_wUA5jxGcgoAAA_22_7D = __webpack_require__(99730);
;// ./src/components/settings/HostKeyTrustDialog.css.ts


var actions = '_10aq6sa8';
var body = '_10aq6sa3';
var detailList = '_10aq6sa4';
var detailTerm = '_10aq6sa5';
var detailValue = '_10aq6sa6';
var dialog = '_10aq6sa1';
var HostKeyTrustDialog_css_fingerprint = '_10aq6sa7';
var heading = '_10aq6sa2';
var overlay = '_10aq6sa0';
var primaryButton = '_10aq6sa9';
var secondaryButton = '_10aq6saa';
;// ./src/components/settings/HostKeyTrustDialog.tsx
/* __next_internal_client_entry_do_not_use__ HostKeyTrustDialog auto */ 




/**
 * HostKeyTrustDialog — the TOFU (trust-on-first-use) confirmation modal
 * shown when TestRemoteConnection reports host_key_unknown=true (Task
 * 6.1.1c). Per ux.md Surface 3 / research/ux.md §1's VS Code precedent this
 * is a MODAL, not an inline prompt or a silent auto-trust: the remote is not
 * persisted until "Trust and connect" is explicitly clicked, and focus
 * starts on Cancel (not Trust) so a stray Enter keypress can't silently
 * trust an unverified host.
 *
 * Built on the same minimal focus-trap pattern as BackwardSyncConfirmDialog
 * (no existing shared primitive implements both Tab-cycling AND
 * focus-return-to-trigger — see that component's doc comment), extended
 * with backdrop-click-equals-Cancel per this dialog's own AC7 (a stronger
 * requirement than BackwardSyncConfirmDialog's, which doesn't need it).
 */ function HostKeyTrustDialog({ host, port, fingerprint, onTrust, onCancel }) {
    const headingId = (0,react.useId)();
    const dialogRef = (0,react.useRef)(null);
    const cancelButtonRef = (0,react.useRef)(null);
    // Default focus lands on Cancel, not "Trust and connect" -- ux.md
    // Surface 3 interaction flow step 1.
    (0,useDialogFocusTrap/* useDialogFocusTrap */.L)({
        dialogRef,
        initialFocusRef: cancelButtonRef,
        onEscape: onCancel
    });
    const content = /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
        className: overlay,
        "data-testid": "host-key-trust-overlay",
        onClick: (e)=>{
            // Backdrop click == Cancel, never Trust (ux.md Surface 3 AC7). Only
            // fires when the click target IS the overlay itself, not a bubbled
            // click from inside the dialog.
            if (e.target === e.currentTarget) onCancel();
        },
        children: /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
            ref: dialogRef,
            role: "dialog",
            "aria-modal": "true",
            "aria-labelledby": headingId,
            className: dialog,
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("h2", {
                    id: headingId,
                    className: heading,
                    children: "Verify host identity"
                }),
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                    className: body,
                    children: "Stapler Squad has not connected to this host before. Verify the fingerprint matches what the remote's administrator (you) expects."
                }),
                /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("dl", {
                    className: detailList,
                    children: [
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("dt", {
                            className: detailTerm,
                            children: "Host"
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("dd", {
                            className: detailValue,
                            "data-testid": "host-key-trust-host",
                            children: [
                                host,
                                ":",
                                port || 22
                            ]
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("dt", {
                            className: detailTerm,
                            children: "Key type"
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("dd", {
                            className: detailValue,
                            children: "ED25519"
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("dt", {
                            className: detailTerm,
                            children: "Fingerprint"
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("dd", {
                            className: [
                                detailValue,
                                HostKeyTrustDialog_css_fingerprint
                            ].join(" "),
                            "data-testid": "host-key-trust-fingerprint",
                            children: fingerprint
                        })
                    ]
                }),
                /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                    className: actions,
                    children: [
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                            ref: cancelButtonRef,
                            type: "button",
                            className: secondaryButton,
                            onClick: onCancel,
                            "data-testid": "host-key-trust-cancel",
                            children: "Cancel"
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                            type: "button",
                            className: primaryButton,
                            onClick: onTrust,
                            "data-testid": "host-key-trust-confirm",
                            children: "Trust and connect"
                        })
                    ]
                })
            ]
        })
    });
    // createPortal escapes any ancestor CSS transform/filter/will-change that
    // would break position:fixed (ADR-009 / docs/reference/css-architecture.md).
    if (typeof document === "undefined") return null;
    return /*#__PURE__*/ (0,react_dom.createPortal)(content, document.body);
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsettings%2FAddRemoteForm.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA91WTY_aMBC98yt8qbQcvHJYCIu59J9Uxh4SC8eObAdCKv57hQk0EPJBtarUniLFM29m3jzP-H0n-HFVEfRzgpCQLlfsSNFWQbmeoPDFQlrgXhpNETeqyPT5JGE5RXtm3zD-Ea2gmtnqI5quJ6fJ-wUx-gLE2bGJOAuIW6M9drKCVvjNdH09P4BMUk9RTMj5HzfK2Ed70sT-GMBm006cWTjKmE2kpog0QOcBNGdCSJ203KrpYxASkDaM7xJrCi0eXaJLFhtjBViKorxEzigpHsx2DStsmZCFa9UTd9cTNXjs4TmTGqc1zfN5XjbrplvDCxeqr9N4GkkFIFN4JTVQpI2GOxQhHdsoEAHI5IxLf6SIvMch98K6M6Q2HjOlzAHEnTPNFeOQGiXABoDnxfKmChbPNcuUTDSWHjJHEQftwXYJtmqIAXvz3OAWL35RINHozg5178pebuS1nCFZZQPqzLoltWjfzMXlZvaoKKap2de9646q7_jslMxiWDLLf6kZ0GqGt0y7nFnQvnfk9XK-HMX5Z5Pzz57LNRsaaNGogRaR0VS_Omz7W3MrcvUVy6zqy3Bw-PNRVMEIppbNytiIkRfKdJ5Z31faDXJz2aUKSoqiq-AOUvg0rMea9C3LpDq2kIqBtvQs4r--PrvFFvbaHuxWmQMuKWKFN-d_h1R6wC5nHCjKbXPd8Rtr2KVW6l1N1svPh_9hIN1p5mFG8ddnlPjz111zodevu-8ZCMnQW8bKa4oxIXk5DVHunhJdIwJb2IN1cMY-_XaKa6caNSLk24PFssvi9AuE4-uPyQsAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsettings_2FAddRemoteForm_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA91WTY_aMBC98yt8qbQcvHJYCIu59J9Uxh4SC8eObAdCKv57hQk0EPJBtarUniLFM29m3jzP_H0n_HFVEfRzgpCQLlfsSNFWQbmeoPDFQlrgXhpNETeqyPT5JGE5RXtm3zD_Ea2gmtnqI5quJ6fJ_wUx_gLE2bGJOAuIW6M9drKCVvjNdH09P4BMUk9RTMj5HzfK2Ed70sT_GMBm006cWTjKmE2kpog0QOcBNGdCSJ203KrpYxASkDaM7xJrCi0eXaJLFhtjBViKorxEzigpHsx2DStsmZCFa9UTd9cTNXjs4TmTGqc1zfN5XjbrplvDCxeqr9N4GkkFIFN4JTVQpI2GOxQhHdsoEAHI5IxLf6SIvMch98K6M6Q2HjOlzAHEnTPNFeOQGiXABoDnxfKmChbPNcuUTDSWHjJHEQftwXYJtmqIAXvz3OAWL35RINHozg5178pebuS1nCFZZQPqzLoltWjfzMXlZvaoKKap2de9646q7_jslMxiWDLLf6kZ0GqGt0y7nFnQvnfk9XK_HMX5Z5Pzz57LNRsaaNGogRaR0VS_Omz7W3MrcvUVy6zqy3Bw_PNRVMEIppbNytiIkRfKdJ5Z31faDXJz2aUKSoqiq_AOUvg0rMea9C3LpDq2kIqBtvQs4r_PrvFFvbaHuxWmQMuKWKFN_d_h1R6wC5nHCjKbXPd8Rtr2KVW6l1N1svPh_9hIN1p5mFG8ddnlPjz111zodevu_8ZCMnQW8bKa4oxIXk5DVHunhJdIwJb2IN1cMY_XaKa6caNSLk24PFssvi9AuE4_uPyQsAAA_22_7D = __webpack_require__(20966);
;// ./src/components/settings/AddRemoteForm.css.ts


var actionRow = 'kdcy9z5';
var authorizedKeysBlock = 'kdcy9z9';
var authorizedKeysLine = 'kdcy9zb';
var authorizedKeysRow = 'kdcy9za';
var cancelBtn = 'kdcy9z7';
var caveat = 'kdcy9zd';
var copyBtn = 'kdcy9zc';
var errorMessage = 'kdcy9z8';
var fieldGroup = 'kdcy9z1';
var AddRemoteForm_css_form = 'kdcy9z0';
var hint = 'kdcy9z3';
var input = 'kdcy9z4';
var label = 'kdcy9z2';
var submitBtn = 'kdcy9z6';
;// ./src/components/settings/AddRemoteForm.tsx
/* __next_internal_client_entry_do_not_use__ AddRemoteForm auto */ 





/** Extracts a human-readable message from a failed RPC call. */ function AddRemoteForm_errorMessage(err) {
    if (err instanceof connect_error/* ConnectError */.T) return err.message;
    if (err instanceof Error) return err.message;
    return String(err);
}
/**
 * Add Remote form (Task 6.1.1b/e/f): name/host/user/port/base-path fields,
 * wired to TestRemoteConnection (draft mode) -> on unknown host key,
 * HostKeyTrustDialog -> TrustRemoteHostKey (draft mode) -> CreateRemote,
 * per ux.md Surface 2/3's interaction flow. The remote is never persisted
 * (CreateRemote is never called) until either an immediately-successful
 * test or an explicit "Trust and connect" click.
 */ function AddRemoteForm({ onSaved, onCancel }) {
    const { createRemote, deleteRemote, generateRemoteIdentity, testRemoteConnectionDraft, trustRemoteHostKeyDraft } = (0,useRemotesService/* useRemotesService */.I)();
    const [name, setName] = (0,react.useState)("");
    const [host, setHost] = (0,react.useState)("");
    const [user, setUser] = (0,react.useState)("");
    const [port, setPort] = (0,react.useState)("");
    const [basePath, setBasePath] = (0,react.useState)("");
    const [identity, setIdentity] = (0,react.useState)(null);
    const [testing, setTesting] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    const [copied, setCopied] = (0,react.useState)(false);
    // Set only while a host_key_unknown response is pending user action --
    // the dialog is rendered whenever this is non-empty.
    const [pendingFingerprint, setPendingFingerprint] = (0,react.useState)(null);
    const [trustBusy, setTrustBusy] = (0,react.useState)(false);
    // Guards against handleTrust acting on a stale result after the user has
    // already clicked Cancel while trustRemoteHostKeyDraft was still in
    // flight -- without this, the dialog visibly closes on Cancel but the
    // remote is still persisted (and onSaved still fires) once the pending
    // RPC resolves behind the user's back. A ref rather than a `cancelled`
    // local (useConfiguredRemotes.ts's equivalent idiom) because the
    // cancellation and the in-flight await live in two different callbacks
    // (handleTrustCancel vs. handleTrust), not one effect and its own cleanup.
    const trustCancelledRef = (0,react.useRef)(false);
    const canSubmit = Boolean(name.trim()) && Boolean(host.trim()) && Boolean(user.trim()) && Boolean(basePath.trim()) && !testing;
    // Once an identity has been generated, EVERY field locks (ux.md Surface 2:
    // "fields as above, now read-only until Cancel/retry") -- not just while
    // `testing` is true. Identity is generated once, keyed by `name`
    // (GenerateOrDescribeIdentity), and the displayed authorized_keys line is
    // that exact key. If fields (especially Name) stayed editable after
    // generation, a rename followed by Create would call CreateRemote with a
    // NEW name that has no stored identity -- its GenerateOrDescribeIdentity
    // fallback would then silently mint a FRESH, different keypair than the
    // one shown/pasted, and the saved remote's stored identity would never
    // match what's actually in the remote's authorized_keys. Fields stay
    // locked until the user explicitly cancels the whole form (which deletes
    // the orphaned identity) -- there is no "unlock to edit, then retry" path.
    const fieldsLocked = testing || Boolean(identity);
    const portNumber = port.trim() === "" ? 0 : Number(port);
    const currentDraft = (0,react.useCallback)(()=>({
            name: name.trim(),
            host: host.trim(),
            user: user.trim(),
            port: portNumber
        }), [
        name,
        host,
        user,
        portNumber
    ]);
    const persistRemote = (0,react.useCallback)(async ()=>{
        const saved = await createRemote({
            name: name.trim(),
            host: host.trim(),
            user: user.trim(),
            port: portNumber,
            basePath: basePath.trim()
        });
        onSaved(saved);
    }, [
        createRemote,
        name,
        host,
        user,
        portNumber,
        basePath,
        onSaved
    ]);
    const handleTestConnection = (0,react.useCallback)(async (e)=>{
        e.preventDefault();
        if (!canSubmit) return;
        setError(null);
        setTesting(true);
        try {
            let currentIdentity = identity;
            if (!currentIdentity) {
                currentIdentity = await generateRemoteIdentity(name.trim());
                setIdentity(currentIdentity);
            }
            const result = await testRemoteConnectionDraft(currentDraft());
            if (result.hostKeyUnknown) {
                setPendingFingerprint(result.fingerprint);
            } else if (result.success) {
                await persistRemote();
            } else {
                setError(result.errorMessage || `Couldn't reach ${name.trim()}. Check that the host is up, or pick a different remote.`);
            }
        } catch (err) {
            setError(AddRemoteForm_errorMessage(err));
        } finally{
            setTesting(false);
        }
    }, [
        canSubmit,
        identity,
        name,
        generateRemoteIdentity,
        testRemoteConnectionDraft,
        currentDraft,
        persistRemote
    ]);
    const handleTrust = (0,react.useCallback)(async ()=>{
        if (!pendingFingerprint) return;
        trustCancelledRef.current = false;
        setTrustBusy(true);
        setError(null);
        try {
            const trustResult = await trustRemoteHostKeyDraft(currentDraft(), pendingFingerprint);
            // Bail out if the user clicked Cancel while this RPC was still in
            // flight -- otherwise a resolve-after-cancel would silently persist
            // the remote (and fire onSaved) even though the dialog already closed.
            // See trustCancelledRef's declaration above.
            if (trustCancelledRef.current) return;
            if (trustResult.success) {
                setPendingFingerprint(null);
                await persistRemote();
            } else {
                setPendingFingerprint(null);
                setError(trustResult.errorMessage || "Trusting the host key failed.");
            }
        } catch (err) {
            if (trustCancelledRef.current) return;
            setPendingFingerprint(null);
            setError(AddRemoteForm_errorMessage(err));
        } finally{
            setTrustBusy(false);
        }
    }, [
        pendingFingerprint,
        trustRemoteHostKeyDraft,
        currentDraft,
        persistRemote
    ]);
    const handleTrustCancel = (0,react.useCallback)(()=>{
        // Signal any in-flight handleTrust call to bail out once its await
        // resolves, instead of silently persisting the remote after the user
        // already closed the dialog -- see handleTrust's trustCancelledRef check.
        trustCancelledRef.current = true;
        // Remote was never saved (CreateRemote is only called after a
        // successful Trust) -- closing the dialog just returns to the still-
        // editable Add Remote form. Per ux.md Surface 3 step 4, this is not a
        // dead end: the user can fix things on the remote host and retry Test
        // connection, which reuses the SAME generated identity (see
        // GenerateRemoteIdentity's idempotency).
        setPendingFingerprint(null);
    }, []);
    const handleFormCancel = (0,react.useCallback)(()=>{
        // If an identity was generated but the remote was never saved, clean up
        // the orphaned keychain entry (ux.md Surface 2, interaction flow step 3)
        // -- best-effort, a failure here shouldn't block closing the form.
        if (identity && name.trim()) {
            void deleteRemote(name.trim()).catch(()=>{});
        }
        onCancel();
    }, [
        identity,
        name,
        deleteRemote,
        onCancel
    ]);
    const handleCopyAuthorizedKeys = (0,react.useCallback)(()=>{
        if (!identity) return;
        void navigator.clipboard.writeText(identity.authorizedKeysLine).then(()=>{
            setCopied(true);
            window.setTimeout(()=>setCopied(false), 2000);
        });
    }, [
        identity
    ]);
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("form", {
                className: AddRemoteForm_css_form,
                onSubmit: handleTestConnection,
                "data-testid": "add-remote-form",
                children: [
                    error && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                        className: errorMessage,
                        role: "alert",
                        "data-testid": "add-remote-error",
                        children: error
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: fieldGroup,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("label", {
                                className: label,
                                htmlFor: "add-remote-name",
                                children: [
                                    "Name",
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                        "aria-hidden": "true",
                                        children: "*"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                                id: "add-remote-name",
                                type: "text",
                                className: input,
                                value: name,
                                onChange: (e)=>setName(e.target.value),
                                disabled: fieldsLocked,
                                required: true,
                                "aria-required": "true",
                                placeholder: "prod-box",
                                "data-testid": "add-remote-name"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: fieldGroup,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("label", {
                                className: label,
                                htmlFor: "add-remote-host",
                                children: [
                                    "Host",
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                        "aria-hidden": "true",
                                        children: "*"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                                id: "add-remote-host",
                                type: "text",
                                className: input,
                                value: host,
                                onChange: (e)=>setHost(e.target.value),
                                disabled: fieldsLocked,
                                required: true,
                                "aria-required": "true",
                                placeholder: "prod.example.com",
                                "data-testid": "add-remote-host"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: fieldGroup,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("label", {
                                className: label,
                                htmlFor: "add-remote-user",
                                children: [
                                    "User",
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                        "aria-hidden": "true",
                                        children: "*"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                                id: "add-remote-user",
                                type: "text",
                                className: input,
                                value: user,
                                onChange: (e)=>setUser(e.target.value),
                                disabled: fieldsLocked,
                                required: true,
                                "aria-required": "true",
                                placeholder: "tyler",
                                "data-testid": "add-remote-user"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: fieldGroup,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("label", {
                                className: label,
                                htmlFor: "add-remote-port",
                                children: "Port"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                                id: "add-remote-port",
                                type: "number",
                                inputMode: "numeric",
                                className: input,
                                value: port,
                                onChange: (e)=>setPort(e.target.value),
                                disabled: fieldsLocked,
                                placeholder: "22",
                                "data-testid": "add-remote-port"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: fieldGroup,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("label", {
                                className: label,
                                htmlFor: "add-remote-base-path",
                                children: [
                                    "Base path",
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                        "aria-hidden": "true",
                                        children: "*"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                                id: "add-remote-base-path",
                                type: "text",
                                className: input,
                                value: basePath,
                                onChange: (e)=>setBasePath(e.target.value),
                                disabled: fieldsLocked,
                                required: true,
                                "aria-required": "true",
                                placeholder: "/srv/workspaces",
                                "data-testid": "add-remote-base-path"
                            })
                        ]
                    }),
                    !identity && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                        className: hint,
                        children: "A new SSH keypair will be generated for this remote and stored in your OS keychain. You'll need to add the public key below to the remote's authorized_keys."
                    }),
                    identity && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: authorizedKeysBlock,
                        "data-testid": "add-remote-authorized-keys",
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                                className: label,
                                children: [
                                    "Add this to ~/.ssh/authorized_keys on ",
                                    host.trim() || "the remote",
                                    ":"
                                ]
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                className: authorizedKeysRow,
                                children: [
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("code", {
                                        className: authorizedKeysLine,
                                        children: identity.authorizedKeysLine
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                        type: "button",
                                        className: copyBtn,
                                        onClick: handleCopyAuthorizedKeys,
                                        "data-testid": "add-remote-copy-authorized-keys",
                                        children: copied ? "Copied!" : "Copy"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                                className: caveat,
                                children: "Stapler Squad cannot verify this line was applied — test the connection once it's in place."
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: actionRow,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                type: "button",
                                className: cancelBtn,
                                onClick: handleFormCancel,
                                "data-testid": "add-remote-cancel",
                                children: "Cancel"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                type: "submit",
                                className: submitBtn,
                                disabled: !canSubmit,
                                "data-testid": "add-remote-submit",
                                children: testing ? "Testing…" : "Test connection"
                            })
                        ]
                    })
                ]
            }),
            pendingFingerprint && /*#__PURE__*/ (0,react_jsx_runtime.jsx)(HostKeyTrustDialog, {
                host: host.trim(),
                port: portNumber,
                fingerprint: pendingFingerprint,
                onTrust: ()=>{
                    if (!trustBusy) void handleTrust();
                },
                onCancel: handleTrustCancel
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Fsettings%2Fremotes%2FRemotesPage.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA8VXy3LbIBTd5yvYdCZeKIPkV0I3_ZMMFtcSMQIVcGS5k3_vgO1WSNarzbQba8zj3HvPPRykp9f4--GwqpYY_XhAqKCnqOLM5gRtE1yevj4glAPPcktQjPEX91-9g94LVUU1QfRolRtj3JSC1gTtBfhN7hkxriG1XEmCUiWOhXQzGS0Jeqf6MYpe4xc4J_q8TBZupqSMcZndnf14eLplGvtMOxGp4JmMuIXCEJSCtKDd8NvRWL6vo1RJC9ISZEqaQrQDWwH0JoTDkIkPuVfSRoafobOc-fz9fHVla4OxG0uVULq9HvvlBdUZlwThINRyJNRu0QubDMCuPGwPw8l5cY8AhHZKM9CRpowfTSeVze-qh1I9auNyLRW_9eQCS1BcnpBRgrPWzuISnKaHTKujZG3kop-DdbcT60snCi6jm5RXKyftJj0kd7L2JPXHlaEq1veFOEv6yTnE3MzFDGTvlxlLtR0S9vBJGyJgu2i3j1GTQ7t_MEE827Du7axDPb8yPLuye8KcUtgmLOz5M0RSL37tybSqCIpvir6adXjYXyaQuaMGBJcwpMtryEq7efcbRKGTnGqWKTbQd3_ngw2kdASJ9iPFIRK7z-sIfSbXXB58j3oE3QgBf152i8D9oOPXbce_ZvyfHB86x9NqKk1JNUg7do92jD04Gh2v30_y-ucWm4RxQ3cCLhpQJU25rQnCT-tmyVLZiAqhKmDB9uwSbYSFGA9IsaXqbFIVcUvBeZBH0pfHcsQtrwum3sBbHPoT91kMvMNMMhVjawEEcUsFTwP8t378eOyK84xNbtWU4zL3pWu4-EaZh0-4Wpbx4LUZ_-Nr81sBjFP02PgO2WBcnha-1NYnwFCpLac1VoNNczfz0QTafhYQuwJ1Pjc8Mkh22fHxE7D_bRxwDQAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Fsettings_2Fremotes_2FRemotesPage_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA8VXy3LbIBTd5yvYdCZeKIPkV0I3_ZMMFtcSMQIVcGS5k3_vgO1WSNarzbQba8zj3HvPPRykp9f4_GwqpYY_XhAqKCnqOLM5gRtE1yevj4glAPPcktQjPEX91_9g94LVUU1QfRolRtj3JSC1gTtBfhN7hkxriG1XEmCUiWOhXQzGS0Jeqf6MYpe4xc4J_q8TBZupqSMcZndnf14eLplGvtMOxGp4JmMuIXCEJSCtKDd8NvRWL6vo1RJC9ISZEqaQrQDWwH0JoTDkIkPuVfSRoafobOc_fz9fHVla4OxG0uVULq9HvvlBdUZlwThINRyJNRu0QubDMCuPGwPw8l5cY8AhHZKM9CRpowfTSeVze_qh1I9auNyLRW_9eQCS1BcnpBRgrPWzuISnKaHTKujZG3kop_DdbcT60snCi6jm5RXKyftJj0kd7L2JPXHlaEq1veFOEv6yTnE3MzFDGTvlxlLtR0S9vBJGyJgu2i3j1GTQ7t_MEE827Du7axDPb8yPLuye8KcUtgmLOz5M0RSL37tybSqCIpvir6adXjYXyaQuaMGBJcwpMtryEq7efcbRKGTnGqWKTbQd3_ngw2kdASJ9iPFIRK7z_sIfSbXXB58j3oE3QgBf152i8D9oOPXbce_ZvyfHB86x9NqKk1JNUg7do92jD04Gh2v30_y_ucWm4RxQ3cCLhpQJU25rQnCT_tmyVLZiAqhKmDB9uwSbYSFGA9IsaXqbFIVcUvBeZBH0pfHcsQtrwum3sBbHPoT91kMvMNMMhVjawEEcUsFTwP8t378eOyK84xNbtWU4zL3pWu4_EaZh0_4Wpbx4LUZ_Nr81sBjFP02PgO2WBcnha_1NYnwFCpLac1VoNNczfz0QTafhYQuwJ1Pjc8Mkh22fHxE7D_bRxwDQAA_22_7D = __webpack_require__(39279);
;// ./src/app/settings/remotes/RemotesPage.css.ts


var RemotesPage_css_actionRow = '_1qkk4w3d';
var confirmDeleteBtn = '_1qkk4w3h _1qkk4w3f';
var confirmText = '_1qkk4w3e';
var container = '_1qkk4w30';
var deleteBtn = '_1qkk4w3g _1qkk4w3f';
var description = '_1qkk4w33';
var empty = '_1qkk4w3i';
var emptyState = '_1qkk4w36';
var RemotesPage_css_errorMessage = '_1qkk4w3j';
var formOverlay = '_1qkk4w3k';
var headerRow = '_1qkk4w31';
var list = '_1qkk4w35';
var listItem = '_1qkk4w37';
var listItemInfo = '_1qkk4w38';
var listItemMeta = '_1qkk4w3b';
var listItemName = '_1qkk4w3a';
var listItemNameRow = '_1qkk4w39';
var listItemStatus = '_1qkk4w3c';
var newBtn = '_1qkk4w34';
var smallBtn = '_1qkk4w3f';
var title = '_1qkk4w32';
;// ./src/app/settings/remotes/page.tsx
// analytics-exempt
/* __next_internal_client_entry_do_not_use__ default auto */ 
// +feature: settings-remotes
//
// PageViewTracker (rendered below) calls usePageView() internally, so this
// page is exempt from analytics/require-page-analytics's literal-call check
// -- same pattern as pipeline-modes/page.tsx.






function page_errorMessage(err) {
    if (err instanceof connect_error/* ConnectError */.T) return err.message;
    if (err instanceof Error) return err.message;
    return String(err);
}
/**
 * Settings -> Remotes: lists configured SSH remotes with row-level Test/
 * Delete actions, and an "Add remote" form (ssh-remote-workspaces Phase 6,
 * Epic 6.1, Task 6.1.1a). Covers requirements.md AC1's UI half.
 */ function RemotesPage() {
    const { listRemotes, deleteRemote, testRemoteConnectionSaved } = (0,useRemotesService/* useRemotesService */.I)();
    const [remotes, setRemotes] = (0,react.useState)([]);
    const [loading, setLoading] = (0,react.useState)(true);
    const [loadError, setLoadError] = (0,react.useState)(null);
    const [formOpen, setFormOpen] = (0,react.useState)(false);
    const [testingName, setTestingName] = (0,react.useState)(null);
    const [testStatus, setTestStatus] = (0,react.useState)({});
    const [confirmDeleteName, setConfirmDeleteName] = (0,react.useState)(null);
    const [deleting, setDeleting] = (0,react.useState)(false);
    const refresh = (0,react.useCallback)(async ()=>{
        setLoading(true);
        setLoadError(null);
        try {
            const list = await listRemotes();
            setRemotes(list);
        } catch (err) {
            setLoadError(page_errorMessage(err));
        } finally{
            setLoading(false);
        }
    }, [
        listRemotes
    ]);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        refresh();
    }, [
        refresh
    ]);
    const handleSaved = (0,react.useCallback)((remote)=>{
        setRemotes((prev)=>{
            const idx = prev.findIndex((r)=>r.name === remote.name);
            if (idx === -1) return [
                ...prev,
                remote
            ];
            const next = [
                ...prev
            ];
            next[idx] = remote;
            return next;
        });
        setFormOpen(false);
    }, []);
    const handleTest = (0,react.useCallback)(async (name)=>{
        setTestingName(name);
        setTestStatus((prev)=>({
                ...prev,
                [name]: ""
            }));
        try {
            const result = await testRemoteConnectionSaved(name);
            if (result.success) {
                setTestStatus((prev)=>({
                        ...prev,
                        [name]: "Connected"
                    }));
            } else if (result.hostKeyUnknown) {
                setTestStatus((prev)=>({
                        ...prev,
                        [name]: "Host key changed or not yet trusted — re-add this remote to re-trust it."
                    }));
            } else {
                setTestStatus((prev)=>({
                        ...prev,
                        [name]: result.errorMessage || "Connection failed."
                    }));
            }
        } catch (err) {
            setTestStatus((prev)=>({
                    ...prev,
                    [name]: page_errorMessage(err)
                }));
        } finally{
            setTestingName(null);
        }
    }, [
        testRemoteConnectionSaved
    ]);
    const handleDeleteConfirm = (0,react.useCallback)(async (name)=>{
        setDeleting(true);
        try {
            await deleteRemote(name);
            setRemotes((prev)=>prev.filter((r)=>r.name !== name));
            setConfirmDeleteName(null);
        } catch (err) {
            setLoadError(page_errorMessage(err));
        } finally{
            setDeleting(false);
        }
    }, [
        deleteRemote
    ]);
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(PageViewTracker.PageViewTracker, {}),
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("main", {
                id: "main-content",
                className: container,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: headerRow,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                children: [
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("h1", {
                                        className: title,
                                        children: "Remotes"
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                                        className: description,
                                        children: "Register a remote host to run sessions on a dedicated Linux box instead of this machine."
                                    })
                                ]
                            }),
                            !formOpen && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: newBtn,
                                onClick: ()=>setFormOpen(true),
                                "data-testid": "remotes-add-button",
                                children: "+ Add remote"
                            })
                        ]
                    }),
                    loadError && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                        className: RemotesPage_css_errorMessage,
                        role: "alert",
                        children: loadError
                    }),
                    formOpen && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                        className: formOverlay,
                        children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)(AddRemoteForm, {
                            onSaved: handleSaved,
                            onCancel: ()=>setFormOpen(false)
                        })
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                        className: list,
                        children: loading ? /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                            className: empty,
                            children: "Loading…"
                        }) : remotes.length === 0 ? !formOpen && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                            className: emptyState,
                            "data-testid": "remotes-empty-state",
                            children: [
                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                                    className: empty,
                                    children: "No remotes configured yet."
                                }),
                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                                    className: description,
                                    children: "Register a remote host to run sessions on a dedicated Linux box instead of this machine."
                                }),
                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                    className: newBtn,
                                    onClick: ()=>setFormOpen(true),
                                    children: "+ Add remote"
                                })
                            ]
                        }) : remotes.map((r)=>/*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                className: listItem,
                                "data-testid": `remote-row-${r.name}`,
                                children: [
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                        className: listItemInfo,
                                        children: [
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                                className: listItemNameRow,
                                                children: [
                                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                                        className: listItemName,
                                                        children: r.name
                                                    }),
                                                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                                                        className: listItemMeta,
                                                        children: [
                                                            r.user,
                                                            "@",
                                                            r.host,
                                                            r.port ? `:${r.port}` : ""
                                                        ]
                                                    })
                                                ]
                                            }),
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                                className: listItemMeta,
                                                children: r.basePath
                                            }),
                                            testStatus[r.name] && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                                className: listItemStatus,
                                                role: "status",
                                                "aria-live": "polite",
                                                "data-testid": `remote-status-${r.name}`,
                                                children: testStatus[r.name]
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                                        className: RemotesPage_css_actionRow,
                                        children: confirmDeleteName === r.name ? /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
                                            children: [
                                                /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                                                    className: confirmText,
                                                    children: [
                                                        "Remove ",
                                                        r.name,
                                                        "?"
                                                    ]
                                                }),
                                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                                    className: confirmDeleteBtn,
                                                    onClick: ()=>handleDeleteConfirm(r.name),
                                                    disabled: deleting,
                                                    "data-testid": `remote-confirm-delete-${r.name}`,
                                                    children: deleting ? "Removing…" : "Remove"
                                                }),
                                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                                    className: smallBtn,
                                                    onClick: ()=>setConfirmDeleteName(null),
                                                    disabled: deleting,
                                                    "data-testid": `remote-cancel-delete-${r.name}`,
                                                    children: "Cancel"
                                                })
                                            ]
                                        }) : /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
                                            children: [
                                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                                    className: smallBtn,
                                                    onClick: ()=>handleTest(r.name),
                                                    disabled: testingName === r.name,
                                                    "data-testid": `remote-test-${r.name}`,
                                                    children: testingName === r.name ? "Testing…" : "Test"
                                                }),
                                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                                    className: deleteBtn,
                                                    onClick: ()=>setConfirmDeleteName(r.name),
                                                    "aria-label": `Delete ${r.name}`,
                                                    "data-testid": `remote-delete-${r.name}`,
                                                    children: "Delete"
                                                })
                                            ]
                                        })
                                    })
                                ]
                            }, r.name))
                    })
                ]
            })
        ]
    });
}


/***/ }),

/***/ 19121:
/***/ ((module) => {

"use strict";
module.exports = require("next/dist/server/app-render/action-async-storage.external.js");

/***/ }),

/***/ 20966:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, ".kdcy9z0 {\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz31);\n}\n.kdcy9z1 {\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz2y);\n}\n.kdcy9z2 {\n  font-size: var(--_19ez2rz3b);\n  font-weight: 600;\n  color: var(--_19ez2rz0);\n}\n.kdcy9z3 {\n  font-size: var(--_19ez2rz3a);\n  color: var(--_19ez2rz2);\n  margin: 0;\n}\n.kdcy9z4 {\n  padding: var(--_19ez2rz2z) var(--_19ez2rz30);\n  background: var(--_19ez2rz1a);\n  border: 1px solid var(--_19ez2rzk);\n  border-radius: var(--_19ez2rz36);\n  color: var(--_19ez2rz1b);\n  font-size: var(--_19ez2rz3b);\n  min-height: 44px;\n}\n.kdcy9z4:focus {\n  border-color: var(--_19ez2rzl);\n  outline: none;\n}\n.kdcy9z4:disabled {\n  opacity: 0.6;\n  cursor: not-allowed;\n}\n.kdcy9z4::placeholder {\n  color: var(--_19ez2rz1c);\n}\n.kdcy9z5 {\n  display: flex;\n  align-items: center;\n  gap: var(--_19ez2rz2z);\n  margin-top: var(--_19ez2rz2z);\n}\n.kdcy9z6 {\n  padding: var(--_19ez2rz2z) var(--_19ez2rz31);\n  border-radius: var(--_19ez2rz36);\n  font-size: var(--_19ez2rz3b);\n  cursor: pointer;\n  border: 1px solid var(--_19ez2rzm);\n  background: var(--_19ez2rzm);\n  color: var(--_19ez2rz5);\n  font-weight: 500;\n  min-height: 44px;\n}\n.kdcy9z6:hover {\n  background: var(--_19ez2rzn);\n}\n.kdcy9z6:disabled {\n  opacity: 0.5;\n  cursor: not-allowed;\n}\n.kdcy9z7 {\n  padding: var(--_19ez2rz2z) var(--_19ez2rz31);\n  border-radius: var(--_19ez2rz36);\n  font-size: var(--_19ez2rz3b);\n  cursor: pointer;\n  border: 1px solid var(--_19ez2rze);\n  background: transparent;\n  color: var(--_19ez2rz0);\n  min-height: 44px;\n}\n.kdcy9z7:hover {\n  background: var(--_19ez2rz8);\n}\n.kdcy9z8 {\n  color: var(--_19ez2rz12);\n  background: var(--_19ez2rz11);\n  border: 1px solid var(--_19ez2rz10);\n  border-radius: var(--_19ez2rz36);\n  padding: var(--_19ez2rz2z) var(--_19ez2rz30);\n  font-size: var(--_19ez2rz3b);\n}\n.kdcy9z9 {\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz2z);\n  padding: var(--_19ez2rz30);\n  background: var(--_19ez2rzc);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: var(--_19ez2rz37);\n}\n.kdcy9za {\n  display: flex;\n  align-items: flex-start;\n  gap: var(--_19ez2rz2z);\n}\n.kdcy9zb {\n  flex: 1 1;\n  min-width: 0;\n  font-family: var(--_19ez2rz2u);\n  font-size: var(--_19ez2rz3a);\n  color: var(--_19ez2rz0);\n  background: var(--_19ez2rz1a);\n  border: 1px solid var(--_19ez2rzk);\n  border-radius: var(--_19ez2rz36);\n  padding: var(--_19ez2rz2z);\n  overflow-x: auto;\n  white-space: pre;\n}\n.kdcy9zc {\n  flex-shrink: 0;\n  padding: var(--_19ez2rz2z) var(--_19ez2rz30);\n  border-radius: var(--_19ez2rz36);\n  font-size: var(--_19ez2rz3b);\n  cursor: pointer;\n  border: 1px solid var(--_19ez2rze);\n  background: transparent;\n  color: var(--_19ez2rz0);\n  min-height: 44px;\n  min-width: 44px;\n}\n.kdcy9zc:hover {\n  background: var(--_19ez2rz8);\n}\n.kdcy9zd {\n  font-size: var(--_19ez2rz3a);\n  color: var(--_19ez2rzz);\n  margin: 0;\n}\n@media (max-width: 600px) {\n  .kdcy9z5 {\n    flex-direction: column-reverse;\n  }\n  .kdcy9z6 {\n    width: 100%;\n  }\n  .kdcy9z7 {\n    width: 100%;\n  }\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 21820:
/***/ ((module) => {

"use strict";
module.exports = require("os");

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

/***/ 34463:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
__webpack_require__.r(__webpack_exports__);
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   "default": () => (__WEBPACK_DEFAULT_EXPORT__)
/* harmony export */ });
/* harmony import */ var react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(70679);
/* harmony import */ var react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__);

/* harmony default export */ const __WEBPACK_DEFAULT_EXPORT__ = ((0,react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__.registerClientReference)(
function() { throw new Error("Attempted to call the default export of \"/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/settings/remotes/page.tsx\" from the server, but it's on the client. It's not possible to invoke a client function from the server, it can only be rendered as a Component or passed to props of a Client Component."); },
"/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/settings/remotes/page.tsx",
"default",
));


/***/ }),

/***/ 39279:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "._1qkk4w30 {\n  max-width: 720px;\n  height: 100%;\n  overflow-y: auto;\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz32);\n  padding: var(--_19ez2rz32);\n}\n._1qkk4w31 {\n  display: flex;\n  align-items: center;\n  justify-content: space-between;\n  gap: var(--_19ez2rz30);\n}\n._1qkk4w32 {\n  font-size: var(--_19ez2rz3d);\n  font-weight: 600;\n  color: var(--_19ez2rz0);\n  margin: 0;\n}\n._1qkk4w33 {\n  font-size: var(--_19ez2rz3b);\n  color: var(--_19ez2rz2);\n  margin: 0;\n}\n._1qkk4w34 {\n  padding: var(--_19ez2rz2z) var(--_19ez2rz30);\n  border-radius: var(--_19ez2rz36);\n  font-size: var(--_19ez2rz3b);\n  cursor: pointer;\n  border: 1px solid var(--_19ez2rzm);\n  background: var(--_19ez2rzm);\n  color: var(--_19ez2rz5);\n  font-weight: 500;\n  min-height: 44px;\n}\n._1qkk4w34:hover {\n  background: var(--_19ez2rzn);\n}\n._1qkk4w35 {\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz2z);\n}\n._1qkk4w36 {\n  display: flex;\n  flex-direction: column;\n  align-items: flex-start;\n  gap: var(--_19ez2rz30);\n  padding: var(--_19ez2rz32);\n  background: var(--_19ez2rz7);\n  border: 1px dashed var(--_19ez2rze);\n  border-radius: var(--_19ez2rz37);\n}\n._1qkk4w37 {\n  display: flex;\n  align-items: center;\n  gap: var(--_19ez2rz30);\n  padding: var(--_19ez2rz30);\n  background: var(--_19ez2rz7);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: var(--_19ez2rz36);\n}\n._1qkk4w38 {\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz2y);\n  flex-grow: 1;\n  min-width: 0;\n}\n._1qkk4w39 {\n  display: flex;\n  align-items: baseline;\n  gap: var(--_19ez2rz2z);\n  flex-wrap: wrap;\n}\n._1qkk4w3a {\n  font-size: var(--_19ez2rz3b);\n  font-weight: 600;\n  color: var(--_19ez2rz0);\n}\n._1qkk4w3b {\n  font-size: var(--_19ez2rz3b);\n  color: var(--_19ez2rz2);\n}\n._1qkk4w3c {\n  font-size: var(--_19ez2rz3a);\n  color: var(--_19ez2rz1);\n}\n._1qkk4w3d {\n  display: flex;\n  gap: var(--_19ez2rz2z);\n  flex-shrink: 0;\n  align-items: center;\n}\n._1qkk4w3e {\n  font-size: var(--_19ez2rz3b);\n  color: var(--_19ez2rz0);\n}\n._1qkk4w3f {\n  padding: var(--_19ez2rz2y) var(--_19ez2rz2z);\n  border-radius: var(--_19ez2rz36);\n  font-size: var(--_19ez2rz3b);\n  cursor: pointer;\n  border: 1px solid var(--_19ez2rze);\n  background: transparent;\n  color: var(--_19ez2rz0);\n  min-height: 44px;\n  min-width: 44px;\n}\n._1qkk4w3f:hover {\n  background: var(--_19ez2rz8);\n}\n._1qkk4w3f:disabled {\n  opacity: 0.5;\n  cursor: not-allowed;\n}\n._1qkk4w3g {\n  border: 1px solid var(--_19ez2rz10);\n  color: var(--_19ez2rz12);\n}\n._1qkk4w3g:hover {\n  background: var(--_19ez2rz11);\n}\n._1qkk4w3h {\n  border: 2px solid var(--_19ez2rz13);\n  background: var(--_19ez2rz13);\n  color: var(--_19ez2rz5);\n  font-weight: 700;\n}\n._1qkk4w3i {\n  color: var(--_19ez2rz2);\n  font-size: var(--_19ez2rz3b);\n  font-style: italic;\n}\n._1qkk4w3j {\n  color: var(--_19ez2rz12);\n  background: var(--_19ez2rz11);\n  border: 1px solid var(--_19ez2rz10);\n  border-radius: var(--_19ez2rz36);\n  padding: var(--_19ez2rz2z) var(--_19ez2rz30);\n  font-size: var(--_19ez2rz3b);\n}\n._1qkk4w3k {\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz31);\n  padding: var(--_19ez2rz31);\n  background: var(--_19ez2rz7);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: var(--_19ez2rz36);\n}\n@media (max-width: 600px) {\n  ._1qkk4w31 {\n    flex-direction: column;\n    align-items: stretch;\n  }\n  ._1qkk4w37 {\n    flex-direction: column;\n    align-items: stretch;\n  }\n  ._1qkk4w3d {\n    justify-content: flex-end;\n  }\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 55289:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 34463));


/***/ }),

/***/ 55591:
/***/ ((module) => {

"use strict";
module.exports = require("https");

/***/ }),

/***/ 62722:
/***/ (() => {



/***/ }),

/***/ 63033:
/***/ ((module) => {

"use strict";
module.exports = require("next/dist/server/app-render/work-unit-async-storage.external.js");

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

/***/ 84040:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
__webpack_require__.r(__webpack_exports__);
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   "default": () => (/* binding */ SettingsLayout),
/* harmony export */   metadata: () => (/* binding */ metadata)
/* harmony export */ });
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(12129);
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__);

const metadata = {
    title: "Settings - Stapler Squad",
    description: "Configure session defaults, profiles, and directory rules."
};
function SettingsLayout({ children }) {
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.Fragment, {
        children: children
    });
}


/***/ }),

/***/ 91737:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 16658));


/***/ }),

/***/ 94266:
/***/ (() => {



/***/ }),

/***/ 99730:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "._10aq6sa0 {\n  position: fixed;\n  inset: 0;\n  background-color: var(--_19ez2rza);\n  display: flex;\n  align-items: center;\n  justify-content: center;\n  z-index: 1000;\n  padding: var(--_19ez2rz31);\n}\n._10aq6sa1 {\n  background-color: var(--_19ez2rz9);\n  border: 1px solid var(--_19ez2rzj);\n  border-radius: var(--_19ez2rz38);\n  box-shadow: var(--_19ez2rz3m);\n  padding: var(--_19ez2rz32);\n  max-width: 480px;\n  width: calc(100% - 2rem);\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz31);\n}\n._10aq6sa2 {\n  font-size: var(--_19ez2rz3d);\n  font-weight: var(--_19ez2rz3h);\n  color: var(--_19ez2rz0);\n  margin: 0;\n}\n._10aq6sa3 {\n  color: var(--_19ez2rz1);\n  font-size: var(--_19ez2rz3c);\n  margin: 0;\n  line-height: 1.5;\n}\n._10aq6sa4 {\n  display: grid;\n  grid-template-columns: auto 1fr;\n  grid-column-gap: var(--_19ez2rz30);\n  column-gap: var(--_19ez2rz30);\n  grid-row-gap: var(--_19ez2rz2z);\n  row-gap: var(--_19ez2rz2z);\n  margin: 0;\n  padding: var(--_19ez2rz30);\n  background: var(--_19ez2rzc);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: var(--_19ez2rz37);\n}\n._10aq6sa5 {\n  font-size: var(--_19ez2rz3b);\n  font-weight: var(--_19ez2rz3g);\n  color: var(--_19ez2rz2);\n  margin: 0;\n}\n._10aq6sa6 {\n  font-size: var(--_19ez2rz3b);\n  color: var(--_19ez2rz0);\n  margin: 0;\n  word-break: break-all;\n}\n._10aq6sa7 {\n  font-family: var(--_19ez2rz2u);\n}\n._10aq6sa8 {\n  display: flex;\n  gap: var(--_19ez2rz30);\n  justify-content: flex-end;\n  margin-top: var(--_19ez2rz2z);\n}\n._10aq6sa9 {\n  background-color: var(--_19ez2rzm);\n  color: var(--_19ez2rzq);\n  border: none;\n  border-radius: var(--_19ez2rz37);\n  padding: var(--_19ez2rz2z) var(--_19ez2rz31);\n  font-size: var(--_19ez2rz3c);\n  font-weight: var(--_19ez2rz3g);\n  cursor: pointer;\n  transition: var(--_19ez2rz3n);\n  min-height: 44px;\n}\n._10aq6sa9:hover {\n  background-color: var(--_19ez2rzn);\n}\n._10aq6sa9:focus-visible {\n  outline: 2px solid var(--_19ez2rzm);\n  outline-offset: 2px;\n}\n._10aq6saa {\n  background-color: transparent;\n  color: var(--_19ez2rz1);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: var(--_19ez2rz37);\n  padding: var(--_19ez2rz2z) var(--_19ez2rz31);\n  font-size: var(--_19ez2rz3c);\n  font-weight: var(--_19ez2rz3g);\n  cursor: pointer;\n  transition: var(--_19ez2rz3n);\n  min-height: 44px;\n}\n._10aq6saa:hover {\n  background-color: var(--_19ez2rz8);\n  border-color: var(--_19ez2rzi);\n}\n._10aq6saa:focus-visible {\n  outline: 2px solid var(--_19ez2rzm);\n  outline-offset: 2px;\n}\n@media (max-width: 600px) {\n  ._10aq6sa1 {\n    position: fixed;\n    left: 0;\n    right: 0;\n    bottom: 0;\n    max-width: 100%;\n    width: 100%;\n    border-bottom-left-radius: 0;\n    border-bottom-right-radius: 0;\n  }\n  ._10aq6sa8 {\n    flex-direction: column-reverse;\n  }\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ })

};
;

// load runtime
var __webpack_require__ = require("../../../webpack-runtime.js");
__webpack_require__.C(exports);
var __webpack_exec__ = (moduleId) => (__webpack_require__(__webpack_require__.s = moduleId))
var __webpack_exports__ = __webpack_require__.X(0, [4655,4164], () => (__webpack_exec__(3802)));
module.exports = __webpack_exports__;

})();