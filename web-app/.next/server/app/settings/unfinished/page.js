(() => {
var exports = {};
exports.id = 9426;
exports.ids = [9426];
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

/***/ 13081:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 7696));
;
Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 50894));


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

/***/ 23141:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  UnfinishedSourcesSettings: () => (/* binding */ UnfinishedSourcesSettings)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(4435);
// EXTERNAL MODULE: ./src/lib/api/transport.ts
var transport = __webpack_require__(73396);
// EXTERNAL MODULE: ./src/gen/session/v1/unfinished_pb.ts
var unfinished_pb = __webpack_require__(58432);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/create.js
var create = __webpack_require__(7094);
// EXTERNAL MODULE: ./src/lib/hooks/useAbortableRequest.ts
var useAbortableRequest = __webpack_require__(10937);
;// ./src/lib/hooks/useUnfinishedWorkConfig.ts
/* __next_internal_client_entry_do_not_use__ useUnfinishedWorkConfig auto */ 






function useUnfinishedWorkConfig() {
    const [config, setConfig] = (0,react.useState)(null);
    const [loading, setLoading] = (0,react.useState)(true);
    const client = (0,react.useMemo)(()=>(0,promise_client/* createClient */.UU)(unfinished_pb/* UnfinishedWorkService */.aC, (0,transport/* getConnectTransport */.qW)()), []);
    const startFetch = (0,useAbortableRequest/* useAbortableRequest */.X)();
    const fetchConfig = (0,react.useCallback)(async ()=>{
        setLoading(true);
        const signal = startFetch();
        try {
            const req = (0,create/* create */.v)(unfinished_pb/* GetUnfinishedWorkConfigRequestSchema */.JC, {});
            const res = await client.getUnfinishedWorkConfig(req, {
                signal
            });
            if (signal.aborted) return;
            if (res.config) setConfig(res.config);
        } catch  {
            if (signal.aborted) return;
        // ignore
        } finally{
            if (!signal.aborted) setLoading(false);
        }
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        startFetch
    ]);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        fetchConfig();
    }, [
        fetchConfig
    ]);
    const updateConfig = (0,react.useCallback)(async (patch)=>{
        if (!config) return;
        const merged = {
            ...config,
            ...patch
        };
        try {
            const req = (0,create/* create */.v)(unfinished_pb/* UpdateUnfinishedWorkConfigRequestSchema */.wR, {
                config: merged
            });
            // A one-shot mutation triggered by an explicit user action (a
            // settings toggle), not tied to a mount/effect that could re-fire
            // faster than this resolves.
            // abort-signal-exempt
            const res = await client.updateUnfinishedWorkConfig(req);
            if (res.config) setConfig(res.config);
        } catch  {
        // ignore
        }
    }, // eslint-disable-next-line react-hooks/exhaustive-deps
    [
        config
    ]);
    return {
        config,
        loading,
        updateConfig
    };
}

// EXTERNAL MODULE: ./src/components/ui/RepoPathInput.tsx + 2 modules
var RepoPathInput = __webpack_require__(79659);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsettings%2FUnfinishedSourcesSettings.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61Wy3LiMBC88xU6LgdTthPYRXxMSpbG9lRkSSvJvLby71uWQ0DGgFPhRKEZ9Ty6W7B4y_6u1kWepeTfjJCG7ZMdCl9TsnpNzX4zI0SgM5IdKCklhIPuMxFogXvUihKuZduoLlIxQ8mW2V9J8pat4Zjb40s-38w-ZotTnSzU-SFmGmPmAbPUyicOj3CVzuebU3wHWNWeklWadmdcS22H-WlIb5itUFGSRqVeHpQq5jdh8zuwr-NbYRIrlaCHxlHCQXmwE1eyDICfVOaLpYWmu1l_jp8t8tNRoa0Am1gmsHVXsOv5OYcSpRWE-VrrugGNxlNPBePvldWtEkMMCBBGO-yptSCZx20A8pap0_kZgaSL3EXjrMI4t2s08fS_nyCy_Bhj_vkpRQGQEMOEQFWNRMcovTf074ibzOyJ0xLF6Prvs7w6G6RkDcrDVXftOeO7uh8oc907qKOisnpHSdbd1Fuwpey-1igEBFI87H1yDoCUaBy6LrSr0UPiDOPQqXJnmYmKsCu9BKUZZkH5iYq-Y-JHe_jiOB1eP8y_hOhqi-p9-BIwWncjh_5HO8gGDyqjpeatS7bosJAQLurWS1RASX5DFDK08ZmW6LJ04IOEpmnlonwxbosJhuKjQniePTI2yR_vk_0xTkfxFO9c7qUntFdw39Zo5YjDk5IjHGok41BrKe4Kisf1RUj9Ng9TH5h7z8fIz8oD9poHImhuM7e8_lewTGMzigsz3q6hBvt7siEjaIGOFRJ6jrRhHP2BknSxvNyf0j5hUuodiOg63BbBtHetj_uDBErQM4l8M_v4Dyj3y6ZDCgAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsettings_2FUnfinishedSourcesSettings_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61Wy3LiMBC88xU6LgdTthPYRXxMSpbG9lRkSSvJvLby71uWQ0DGgFPhRKEZ9Ty6W7B4y_6u1kWepeTfjJCG7ZMdCl9TsnpNzX4zI0SgM5IdKCklhIPuMxFogXvUihKuZduoLlIxQ8mW2V9J8pat4Zjb40s_38w_ZotTnSzU_SFmGmPmAbPUyicOj3CVzuebU3wHWNWeklWadmdcS22H_WlIb5itUFGSRqVeHpQq5jdh8zuwr_NbYRIrlaCHxlHCQXmwE1eyDICfVOaLpYWmu1l_jp8t8tNRoa0Am1gmsHVXsOv5OYcSpRWE_VrrugGNxlNPBePvldWtEkMMCBBGO_yptSCZx20A8pap0_kZgaSL3EXjrMI4t2s08fS_nyCy_Bhj_vkpRQGQEMOEQFWNRMcovTf074ibzOyJ0xLF6Prvs7w6G6RkDcrDVXftOeO7uh8oc907qKOisnpHSdbd1Fuwpey_1igEBFI87H1yDoCUaBy6LrSr0UPiDOPQqXJnmYmKsCu9BKUZZkH5iYq_Y_JHe_jiOB1eP8y_hOhqi_p9_BIwWncjh_5HO8gGDyqjpeatS7bosJAQLurWS1RASX5DFDK08ZmW6LJ04IOEpmnlonwxbosJhuKjQniePTI2yR_vk_0xTkfxFO9c7qUntFdw39Zo5YjDk5IjHGok41BrKe4Kisf1RUj9Ng9TH5h7z8fIz8oD9poHImhuM7e8_lewTGMzigsz3q6hBvt7siEjaIGOFRJ6jrRhHP2BknSxvNyf0j5hUuodiOg63BbBtHetj_uDBErQM4l8M_v4Dyj3y6ZDCgAA_22_7D = __webpack_require__(65057);
;// ./src/components/settings/UnfinishedSourcesSettings.css.ts


var addBtn = '_1q69b21d';
var addRow = '_1q69b21b';
var container = '_1q69b210';
var description = '_1q69b213';
var empty = '_1q69b21e';
var input = '_1q69b21c';
var list = '_1q69b217';
var listItem = '_1q69b218';
var listItemPath = '_1q69b219';
var removeBtn = '_1q69b21a';
var section = '_1q69b211';
var sectionTitle = '_1q69b212';
var toggle = '_1q69b215';
var toggleOn = '_1q69b216';
var toggleRow = '_1q69b214';
;// ./src/components/settings/UnfinishedSourcesSettings.tsx
/* __next_internal_client_entry_do_not_use__ UnfinishedSourcesSettings auto */ 




/**
 * Settings panel for configuring Unfinished Work sources:
 * - Auto-spider toggle
 * - Watch dirs (add/remove)
 * - Pinned repos (add/remove)
 */ function UnfinishedSourcesSettings() {
    const { config, loading, updateConfig } = useUnfinishedWorkConfig();
    const [newWatchDir, setNewWatchDir] = (0,react.useState)("");
    const [newPinnedRepo, setNewPinnedRepo] = (0,react.useState)("");
    if (loading) return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
        children: "Loading…"
    });
    if (!config) return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
        children: "Failed to load config."
    });
    const watchDirs = config.watchDirs ?? [];
    const pinnedRepos = config.pinnedRepos ?? [];
    const autoSpider = config.autoSpiderSessions ?? true;
    const handleToggleAutoSpider = ()=>{
        updateConfig({
            autoSpiderSessions: !autoSpider
        });
    };
    const handleRemoveWatchDir = (dir)=>{
        updateConfig({
            watchDirs: watchDirs.filter((d)=>d !== dir)
        });
    };
    const handleAddWatchDir = ()=>{
        const trimmed = newWatchDir.trim();
        if (!trimmed || watchDirs.includes(trimmed)) return;
        updateConfig({
            watchDirs: [
                ...watchDirs,
                trimmed
            ]
        });
        setNewWatchDir("");
    };
    const handleRemovePinnedRepo = (repo)=>{
        updateConfig({
            pinnedRepos: pinnedRepos.filter((r)=>r !== repo)
        });
    };
    const handleAddPinnedRepo = ()=>{
        const trimmed = newPinnedRepo.trim();
        if (!trimmed || pinnedRepos.includes(trimmed)) return;
        updateConfig({
            pinnedRepos: [
                ...pinnedRepos,
                trimmed
            ]
        });
        setNewPinnedRepo("");
    };
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: container,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("section", {
                className: section,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("h3", {
                        className: sectionTitle,
                        children: "Auto-Spider Sessions"
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                        className: description,
                        children: "Automatically scan the git repos of active Stapler Squad sessions for unfinished work."
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: toggleRow,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                role: "switch",
                                "aria-checked": autoSpider,
                                className: `${toggle} ${autoSpider ? toggleOn : ""}`,
                                onClick: handleToggleAutoSpider,
                                "aria-label": "Toggle auto-spider sessions"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                children: autoSpider ? "Enabled" : "Disabled"
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("section", {
                className: section,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("h3", {
                        className: sectionTitle,
                        children: "Watch Directories"
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                        className: description,
                        children: "Stapler Squad will scan all git repos found within these directories (depth ≤ 5)."
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                        className: list,
                        children: watchDirs.length === 0 ? /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                            className: empty,
                            children: "No watch directories configured."
                        }) : watchDirs.map((dir)=>/*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                className: listItem,
                                children: [
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                        className: listItemPath,
                                        children: dir
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                        className: removeBtn,
                                        onClick: ()=>handleRemoveWatchDir(dir),
                                        "aria-label": `Remove watch directory ${dir}`,
                                        children: "✕"
                                    })
                                ]
                            }, dir))
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: addRow,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(RepoPathInput/* RepoPathInput */.E, {
                                value: newWatchDir,
                                onChange: setNewWatchDir,
                                onEnter: handleAddWatchDir,
                                "aria-label": "New watch directory path",
                                placeholder: "/Users/you/code"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: addBtn,
                                onClick: handleAddWatchDir,
                                disabled: !newWatchDir.trim(),
                                children: "Add"
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("section", {
                className: section,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("h3", {
                        className: sectionTitle,
                        children: "Pinned Repositories"
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                        className: description,
                        children: "Explicitly add a specific git repository to always scan."
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                        className: list,
                        children: pinnedRepos.length === 0 ? /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                            className: empty,
                            children: "No pinned repositories."
                        }) : pinnedRepos.map((repo)=>/*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                className: listItem,
                                children: [
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                        className: listItemPath,
                                        children: repo
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                        className: removeBtn,
                                        onClick: ()=>handleRemovePinnedRepo(repo),
                                        "aria-label": `Remove pinned repo ${repo}`,
                                        children: "✕"
                                    })
                                ]
                            }, repo))
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: addRow,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(RepoPathInput/* RepoPathInput */.E, {
                                value: newPinnedRepo,
                                onChange: setNewPinnedRepo,
                                onEnter: handleAddPinnedRepo,
                                "aria-label": "New pinned repository path",
                                placeholder: "/Users/you/my-project",
                                "data-testid": "pinned-repo-input"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: addBtn,
                                onClick: handleAddPinnedRepo,
                                disabled: !newPinnedRepo.trim(),
                                children: "Add"
                            })
                        ]
                    })
                ]
            })
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

/***/ 33873:
/***/ ((module) => {

"use strict";
module.exports = require("path");

/***/ }),

/***/ 43358:
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
const page5 = () => Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 45667));


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
        'unfinished',
        {
        children: ['__PAGE__', {}, {
          page: [page5, "/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/settings/unfinished/page.tsx"],
          
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
const pages = ["/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/settings/unfinished/page.tsx"];


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
        page: "/settings/unfinished/page",
        pathname: "/settings/unfinished",
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

/***/ 45667:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
__webpack_require__.r(__webpack_exports__);
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   "default": () => (/* binding */ UnfinishedSettingsPage),
/* harmony export */   metadata: () => (/* binding */ metadata)
/* harmony export */ });
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(12129);
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var _components_settings_UnfinishedSourcesSettings__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(50894);
/* harmony import */ var _components_analytics_PageViewTracker__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(7696);
// analytics-exempt



const metadata = {
    title: "Unfinished Sources - Settings - Stapler Squad",
    description: "Configure watch directories and pinned repositories for unfinished work scanning."
};
function UnfinishedSettingsPage() {
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)(react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.Fragment, {
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(_components_analytics_PageViewTracker__WEBPACK_IMPORTED_MODULE_2__.PageViewTracker, {}),
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(_components_settings_UnfinishedSourcesSettings__WEBPACK_IMPORTED_MODULE_1__.UnfinishedSourcesSettings, {})
        ]
    });
}


/***/ }),

/***/ 49529:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 88486));
;
Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 23141));


/***/ }),

/***/ 50894:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   UnfinishedSourcesSettings: () => (/* binding */ UnfinishedSourcesSettings)
/* harmony export */ });
/* harmony import */ var react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(70679);
/* harmony import */ var react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__);

const UnfinishedSourcesSettings = (0,react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__.registerClientReference)(
function() { throw new Error("Attempted to call UnfinishedSourcesSettings() from the server but UnfinishedSourcesSettings is on the client. It's not possible to invoke a client function from the server, it can only be rendered as a Component or passed to props of a Client Component."); },
"/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/components/settings/UnfinishedSourcesSettings.tsx",
"UnfinishedSourcesSettings",
);

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

/***/ 65057:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "._1q69b210 {\n  max-width: 640px;\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz32);\n}\n._1q69b211 {\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz30);\n}\n._1q69b212 {\n  font-size: var(--_19ez2rz3c);\n  font-weight: 600;\n  color: var(--_19ez2rz0);\n  margin: 0;\n}\n._1q69b213 {\n  font-size: var(--_19ez2rz3b);\n  color: var(--_19ez2rz2);\n  margin: 0;\n}\n._1q69b214 {\n  display: flex;\n  align-items: center;\n  gap: var(--_19ez2rz30);\n}\n._1q69b215 {\n  width: 2.5rem;\n  height: 1.25rem;\n  border-radius: var(--_19ez2rz39);\n  border: none;\n  cursor: pointer;\n  background: var(--_19ez2rze);\n  position: relative;\n  transition: background 0.2s;\n}\n._1q69b216 {\n  background: var(--_19ez2rzm);\n}\n._1q69b217 {\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz2z);\n}\n._1q69b218 {\n  display: flex;\n  align-items: center;\n  gap: var(--_19ez2rz2z);\n  padding: var(--_19ez2rz2z) var(--_19ez2rz30);\n  background: var(--_19ez2rz7);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: var(--_19ez2rz36);\n  font-family: var(--_19ez2rz2u);\n  font-size: var(--_19ez2rz3b);\n  color: var(--_19ez2rz0);\n}\n._1q69b219 {\n  flex-grow: 1;\n  overflow: hidden;\n  text-overflow: ellipsis;\n  white-space: nowrap;\n}\n._1q69b21a {\n  background: transparent;\n  border: none;\n  cursor: pointer;\n  color: var(--_19ez2rz2);\n  font-size: var(--_19ez2rz3b);\n  padding: 0 var(--_19ez2rz2y);\n  flex-shrink: 0;\n}\n._1q69b21a:hover {\n  color: var(--_19ez2rz12);\n}\n._1q69b21a:focus-visible {\n  outline: 2px solid var(--_19ez2rzl);\n  outline-offset: 1px;\n  border-radius: var(--_19ez2rz36);\n}\n._1q69b21b {\n  display: flex;\n  gap: var(--_19ez2rz2z);\n}\n._1q69b21c {\n  flex-grow: 1;\n  padding: var(--_19ez2rz2z) var(--_19ez2rz30);\n  background: var(--_19ez2rz1a);\n  border: 1px solid var(--_19ez2rzk);\n  border-radius: var(--_19ez2rz36);\n  color: var(--_19ez2rz1b);\n  font-family: var(--_19ez2rz2u);\n  font-size: var(--_19ez2rz3b);\n}\n._1q69b21c:focus {\n  border-color: var(--_19ez2rzl);\n  outline: none;\n}\n._1q69b21c::placeholder {\n  color: var(--_19ez2rz1c);\n}\n._1q69b21d {\n  padding: var(--_19ez2rz2z) var(--_19ez2rz30);\n  border-radius: var(--_19ez2rz36);\n  font-size: var(--_19ez2rz3b);\n  cursor: pointer;\n  border: 1px solid var(--_19ez2rzm);\n  background: var(--_19ez2rzm);\n  color: var(--_19ez2rz5);\n  font-weight: 500;\n}\n._1q69b21d:hover {\n  background: var(--_19ez2rzn);\n}\n._1q69b21d:focus-visible {\n  outline: 2px solid var(--_19ez2rzl);\n  outline-offset: 1px;\n}\n._1q69b21d:disabled {\n  opacity: 0.5;\n  cursor: not-allowed;\n}\n._1q69b21e {\n  color: var(--_19ez2rz2);\n  font-size: var(--_19ez2rz3b);\n  font-style: italic;\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


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

/***/ 94266:
/***/ (() => {



/***/ })

};
;

// load runtime
var __webpack_require__ = require("../../../webpack-runtime.js");
__webpack_require__.C(exports);
var __webpack_exec__ = (moduleId) => (__webpack_require__(__webpack_require__.s = moduleId))
var __webpack_exports__ = __webpack_require__.X(0, [4655,4164], () => (__webpack_exec__(43358)));
module.exports = __webpack_exports__;

})();