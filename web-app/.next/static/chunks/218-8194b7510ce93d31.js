"use strict";
(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[218],{

/***/ 391:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   u: () => (/* binding */ UnknownFlagsWarning)
/* harmony export */ });
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(61574);
/* harmony import */ var _ProbeStatusBadge__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(6990);
/* harmony import */ var _FlagNotes_css__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(31750);
// +feature: settings-programs



const MAX_NAMES = 3;
const MAX_NAME_LENGTH = 40;
const clip = (name)=>name.length > MAX_NAME_LENGTH ? "".concat(name.slice(0, MAX_NAME_LENGTH), "…") : name;
/** Soft warning: plain text referenced by aria-describedby, never role=alert or aria-invalid. */ function UnknownFlagsWarning(param) {
    let { id, testId, program, unknown } = param;
    if (unknown.length === 0) return null;
    const shown = unknown.slice(0, MAX_NAMES).map(clip).join(", ");
    const more = unknown.length - MAX_NAMES;
    const verb = unknown.length === 1 ? "is" : "are";
    const programName = program.split("/").pop() || program;
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
        id: id,
        className: _FlagNotes_css__WEBPACK_IMPORTED_MODULE_2__/* .warning */ .$e,
        "data-testid": testId,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(_ProbeStatusBadge__WEBPACK_IMPORTED_MODULE_1__/* .Icon */ .In, {
                name: "warning"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("span", {
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("code", {
                        children: shown
                    }),
                    more > 0 ? " and ".concat(more, " more") : "",
                    " ",
                    verb,
                    " not listed in ",
                    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("code", {
                        children: [
                            programName,
                            " --help"
                        ]
                    }),
                    ". It may still work (hidden or subcommand flags)."
                ]
            })
        ]
    });
}


/***/ }),

/***/ 3358:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   cf: () => (/* binding */ pickFanoutWinner),
/* harmony export */   mB: () => (/* binding */ fanOutSession),
/* harmony export */   tI: () => (/* binding */ listFanoutMembers),
/* harmony export */   w3: () => (/* binding */ fanoutComparisonPath),
/* harmony export */   wD: () => (/* binding */ MAX_FANOUT_VARIANTS)
/* harmony export */ });
/* unused harmony exports FANOUT_GROUP_TAG_PREFIX, FANOUT_VARIANT_TAG_PREFIX, fanoutGroupIdOf, fanoutVariantOf */
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(66144);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(23575);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(21694);



// Fan-out group membership is carried as session tags (see server/services/session_service_fanout.go).
const FANOUT_GROUP_TAG_PREFIX = "fanout:";
const FANOUT_VARIANT_TAG_PREFIX = "fanout-variant:";
const MAX_FANOUT_VARIANTS = 5;
function fanoutGroupIdOf(session) {
    const tag = session.tags.find((t)=>t.startsWith(FANOUT_GROUP_TAG_PREFIX));
    return tag === null || tag === void 0 ? void 0 : tag.slice(FANOUT_GROUP_TAG_PREFIX.length);
}
function fanoutVariantOf(session) {
    const tag = session.tags.find((t)=>t.startsWith(FANOUT_VARIANT_TAG_PREFIX));
    return tag ? Number.parseInt(tag.slice(FANOUT_VARIANT_TAG_PREFIX.length), 10) || 0 : 0;
}
function fanoutComparisonPath(groupId) {
    // Query param rather than /fanout/:groupId — the app is a static export, which cannot
    // pre-render dynamic path segments (same constraint as /sessions/summary?sessionId=).
    return "/fanout?groupId=".concat(encodeURIComponent(groupId));
}
function client() {
    return (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_2__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_0__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_1__/* .getConnectTransport */ .qW)());
}
async function fanOutSession(base, count) {
    let programOverrides = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : [];
    const res = await client().fanOutSession({
        base,
        count,
        programOverrides
    });
    return {
        groupId: res.groupId,
        sessions: res.sessions
    };
}
async function listFanoutMembers(groupId) {
    const res = await client().listSessions({
        includeArchived: true
    });
    return res.sessions.filter((s)=>fanoutGroupIdOf(s) === groupId).sort((a, b)=>fanoutVariantOf(a) - fanoutVariantOf(b));
}
async function pickFanoutWinner(groupId, winnerSessionId) {
    return client().pickFanoutWinner({
        groupId,
        winnerSessionId
    });
}


/***/ }),

/***/ 6990:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  In: () => (/* binding */ Icon),
  q5: () => (/* binding */ ProbeStatusBadge)
});

// UNUSED EXPORTS: CHECKING_ANNOUNCE_DELAY_MS, COULDNT_CHECK_TEXT, NOT_FOUND_TEXT

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fui%2FProbeStatusBadge.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA81Vy27qMBDd8xXeXAkWQQlQoOmm0v2QyoknyYjEtsYOr6v--5VNTJNAQeqqipQIz5zj8Zkz5n0Hp4J4A4Z9JHK_UVkRs38TxqzyH8YscWkKRU3KSFluYbpcxwLK2duEsc_J52QegIlHCDS65qeUFTUcXY77RgfiOmXu7ZZ4jaWM0EJjLnmRsZysC5Uub89pGkUfySucF3RenP1emguBsrwTHa0sY5-fKRJAEXGBrRmjlhufUyhpI4NnuIlnPt6gjA4obJWy-K1_1oU_a8bzXUmqlWKM33t4rmpF49BhNmBaPmHafs-UDJlWT5hO3zOdh0wvP-tkDtICPe6i40hZ4p4FQXMrMmNqD1TU6tBtxeXpUAHBoMK1r_BinYpQ7jpsx5Osta-4AiwrG3738Juf4IOnUrbQR2ZUjYLlLRFI-9ep2nOdVTrqlPYDpLnLumPLl_iP11Fiwy0qmfbmMJ5vDatRAieGskCJdqjC9ledolfYqy8sNDJlFQoB0gEtHG30FYC6Rm3Q-KortBAZzXNImVTBYg0_Bnsk8WgfPtjnkWGyL6kybtAEskd3AB9ORe4pnFuDoKvVRdCeg8PS9baKx6zJrN-D5FkPHt5f_WEfdajrG8oKCG046GAhb8m4HK0wDO6tQH0BUoGGZzUIr8QVTqokML6HSvMc7Sll8XwzwF4g4dRSSbi6QUCuqDN_KwWQs_y4GN5a5QjfGxDI2bQn-XoV6-PM8496dY_C_2n9B1MigUv3BgAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FProbeStatusBadge_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81Vy27qMBDd8xXeXAkWQQlQoOmm0v2QyoknyYjEtsYOr6v_5VNTJNAQeqqipQIz5zj8Zkz5n0Hp4J4A4Z9JHK_UVkRs38TxqzyH8YscWkKRU3KSFluYbpcxwLK2duEsc_J52QegIlHCDS65qeUFTUcXY77RgfiOmXu7ZZ4jaWM0EJjLnmRsZysC5Uub89pGkUfySucF3RenP1emguBsrwTHa0sY5_fKRJAEXGBrRmjlhufUyhpI4NnuIlnPt6gjA4obJWy_K1_1oU_a8bzXUmqlWKM33t4rmpF49BhNmBaPmHafs_UDJlWT5hO3zOdh0wvP_tkDtICPe6i40hZ4p4FQXMrMmNqD1TU6tBtxeXpUAHBoMK1r_BinYpQ7jpsx5Osta_4AiwrG3738Juf4IOnUrbQR2ZUjYLlLRFI_9ep2nOdVTrqlPYDpLnLumPLl_iP11Fiwy0qmfbmMJ5vDatRAieGskCJdqjC9ledolfYqy8sNDJlFQoB0gEtHG30FYC6Rm3Q_KortBAZzXNImVTBYg0_Bnsk8WgfPtjnkWGyL6kybtAEskd3AB9ORe4pnFuDoKvVRdCeg8PS9baKx6zJrN_D5FkPHt5f_WEfdajrG8oKCG046GAhb8m4HK0wDO6tQH0BUoGGZzUIr8QVTqokML6HSvMc7Sll8XwzwF4g4dRSSbi6QUCuqDN_KwWQs_y4GN5a5QjfGxDI2bQn_XoV6_PM8496dY_C_2n9B1MigUv3BgAA_22_7D = __webpack_require__(98659);
;// ./src/components/ui/ProbeStatusBadge.css.ts


var ProbeStatusBadge_css_button = '_1nv7obfc';
var detail = '_1nv7obfb';
var icon = '_1nv7obf6';
var path = '_1nv7obf9';
var pathExpanded = '_1nv7obfa';
var root = '_1nv7obf1';
var spinner = '_1nv7obf7';
var spinnerStatic = '_1nv7obf8';
var ProbeStatusBadge_css_status = '_1nv7obf5';
var toggle = '_1nv7obfd _1nv7obfc';
var tone = {success:'_1nv7obf2',neutral:'_1nv7obf3',warning:'_1nv7obf4'};
;// ./src/components/ui/ProbeStatusBadge.tsx
// +feature: settings-programs
/* __next_internal_client_entry_do_not_use__ CHECKING_ANNOUNCE_DELAY_MS,NOT_FOUND_TEXT,COULDNT_CHECK_TEXT,Icon,ProbeStatusBadge auto */ 


const CHECKING_ANNOUNCE_DELAY_MS = 300;
const NOT_FOUND_TEXT = "Not found as an executable on the server's PATH. Shell aliases and functions are not checked; the program may still work when launched from your shell.";
const COULDNT_CHECK_TEXT = "Couldn't check right now.";
const TRANSPORT_HINT = "The server may refuse checks when it listens on a non-loopback address without authentication.";
function describe(state, token) {
    switch(state.kind){
        case "checking":
            return {
                tone: "neutral",
                icons: [],
                text: "Checking..."
            };
        case "found":
            return {
                tone: "success",
                icons: [
                    "check"
                ],
                text: "Found: ".concat(state.path),
                path: state.path,
                flags: state.flagCount > 0 ? "".concat(state.flagCount, " flags detected") : undefined
            };
        case "noFlags":
            return {
                tone: "neutral",
                icons: [
                    "check"
                ],
                text: "Found: ".concat(state.path, ". Couldn't read flags from --help; flag suggestions unavailable."),
                path: state.path
            };
        case "needsConfirm":
            return {
                tone: "neutral",
                icons: [
                    "check",
                    "info"
                ],
                text: "Found: ".concat(state.path, ". Not checked for flags yet. Check runs `").concat(token, " --help` on this server."),
                path: state.path
            };
        case "timeout":
            return {
                tone: "neutral",
                icons: [
                    "check",
                    "info"
                ],
                text: "Found: ".concat(state.path, ". Timed out reading flags — try Check again."),
                path: state.path
            };
        case "wrapper":
            return {
                tone: "neutral",
                icons: [
                    "check",
                    "info"
                ],
                text: "Wrapper command (".concat(token, "): flags for the wrapped program are not checked.")
            };
        case "notFound":
            return {
                tone: "warning",
                icons: [
                    "warning"
                ],
                text: NOT_FOUND_TEXT
            };
        case "busyOrError":
            return {
                tone: "neutral",
                icons: [
                    "info"
                ],
                text: COULDNT_CHECK_TEXT,
                retry: true
            };
        case "transportError":
            return {
                tone: "warning",
                icons: [
                    "warning"
                ],
                text: COULDNT_CHECK_TEXT,
                extra: TRANSPORT_HINT,
                retry: true
            };
        default:
            return null;
    }
}
const ICON_PATHS = {
    check: "M3 8.5l3.2 3.2L13 4.8",
    info: "M8 7v5M8 4.2v.1",
    warning: "M8 2.5L14 13H2L8 2.5zM8 7v3M8 11.6v.1"
};
function Icon(param) {
    let { name } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("svg", {
        className: "".concat(icon),
        "data-icon": name,
        "aria-hidden": "true",
        viewBox: "0 0 16 16",
        fill: "none",
        stroke: "currentColor",
        strokeWidth: "1.6",
        strokeLinecap: "round",
        strokeLinejoin: "round",
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("path", {
            d: ICON_PATHS[name]
        })
    });
}
function prefersReducedMotion() {
    var _window_matchMedia, _window_matchMedia1, _window;
    return  true && !!((_window_matchMedia1 = (_window = window).matchMedia) === null || _window_matchMedia1 === void 0 ? void 0 : (_window_matchMedia = _window_matchMedia1.call(_window, "(prefers-reduced-motion: reduce)")) === null || _window_matchMedia === void 0 ? void 0 : _window_matchMedia.matches);
}
function ProbeStatusBadge(param) {
    let { state, checkedToken, onRetry, onConfirm, testId, id } = param;
    const checking = state.kind === "checking";
    const [announce, setAnnounce] = (0,react.useState)(false);
    const [checkShown, setCheckShown] = (0,react.useState)(false);
    const [expanded, setExpanded] = (0,react.useState)(false);
    (0,react.useEffect)(()=>{
        if (!checking) {
            setAnnounce(false);
            return;
        }
        const t = setTimeout(()=>setAnnounce(true), CHECKING_ANNOUNCE_DELAY_MS);
        return ()=>clearTimeout(t);
    }, [
        checking
    ]);
    (0,react.useEffect)(()=>{
        if (state.kind === "needsConfirm") setCheckShown(true);
        if (state.kind === "idle" || state.kind === "disabled") {
            setCheckShown(false);
            setExpanded(false);
        }
    }, [
        state.kind
    ]);
    const row = describe(state, checkedToken);
    if (!row) return null;
    const showCheck = checkShown || state.kind === "needsConfirm";
    const reduced = prefersReducedMotion();
    const isFound = row.tone === "success" || Boolean(row.path);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: "".concat(root, " ").concat(tone[row.tone]),
        "data-testid": testId,
        "data-tone": row.tone,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: "".concat(ProbeStatusBadge_css_status),
                role: "status",
                "aria-live": "polite",
                id: id,
                children: [
                    checking ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: "".concat(reduced ? spinnerStatic : spinner),
                        "data-icon": "spinner",
                        "data-static": reduced,
                        "aria-hidden": "true"
                    }) : null,
                    row.icons.map((name)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(Icon, {
                            name: name
                        }, name)),
                    checking && !announce ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        "aria-hidden": "true",
                        children: row.text
                    }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: row.path && !expanded && row.tone === "success" ? "".concat(path) : undefined,
                        children: row.text
                    }),
                    row.flags ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        children: row.flags
                    }) : null,
                    row.extra ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        children: row.extra
                    }) : null
                ]
            }),
            isFound && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: "".concat(detail),
                children: [
                    "Checked on this server only \xb7 Checked: ",
                    checkedToken,
                    expanded && row.path ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("code", {
                        className: "".concat(pathExpanded),
                        children: [
                            " ",
                            row.path
                        ]
                    }) : null
                ]
            }),
            row.path && // analytics-exempt
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                className: "".concat(toggle),
                "aria-expanded": expanded,
                onClick: ()=>setExpanded((v)=>!v),
                children: expanded ? "Hide full path" : "Show full path"
            }),
            row.retry && // analytics-exempt
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                className: "".concat(ProbeStatusBadge_css_button),
                onClick: onRetry,
                children: "Retry"
            }),
            showCheck && // analytics-exempt
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                className: "".concat(ProbeStatusBadge_css_button),
                onClick: onConfirm,
                disabled: checking,
                "aria-busy": checking,
                children: state.kind === "needsConfirm" ? "Check" : "Check again"
            })
        ]
    });
}


/***/ }),

/***/ 11804:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   N: () => (/* binding */ InputType),
/* harmony export */   P: () => (/* binding */ INPUT_TYPE_INFO)
/* harmony export */ });
/**
 * Input type detection types for the omnibar
 * Ported from Go implementation at ui/overlay/omnibar/types.go
 */ var InputType = /*#__PURE__*/ function(InputType) {
    InputType["Unknown"] = "unknown";
    InputType["Command"] = "command";
    InputType["LocalPath"] = "local_path";
    InputType["PathWithBranch"] = "path_with_branch";
    InputType["GitHubPR"] = "github_pr";
    InputType["GitHubBranch"] = "github_branch";
    InputType["GitHubRepo"] = "github_repo";
    InputType["GitHubShorthand"] = "github_shorthand";
    InputType["SessionSearch"] = "session_search";
    InputType["NewSession"] = "new_session";
    InputType["SpawnShell"] = "spawn_shell";
    InputType["Workflow"] = "workflow";
    InputType["Alias"] = "alias";
    InputType["AliasNotFound"] = "alias_not_found";
    InputType["AliasBrowse"] = "alias_browse";
    InputType["Preset"] = "preset";
    InputType["PresetNotFound"] = "preset_not_found";
    InputType["ChatBacklogItem"] = "chat_backlog_item";
    InputType["FanOut"] = "fan_out";
    return InputType;
}({});
const INPUT_TYPE_INFO = {
    ["unknown"]: {
        label: "Unknown",
        icon: "❓",
        description: "Enter a local path or GitHub URL"
    },
    ["command"]: {
        label: "Command",
        icon: "⌨️",
        description: "Run an omnibar command"
    },
    ["local_path"]: {
        label: "Local Path",
        icon: "📁",
        description: "Local filesystem path"
    },
    ["path_with_branch"]: {
        label: "Path + Branch",
        icon: "🌿",
        description: "Local path with branch (path@branch)"
    },
    ["github_pr"]: {
        label: "GitHub PR",
        icon: "🔀",
        description: "Pull request URL"
    },
    ["github_branch"]: {
        label: "GitHub Branch",
        icon: "🌱",
        description: "Branch URL"
    },
    ["github_repo"]: {
        label: "GitHub Repo",
        icon: "📦",
        description: "Repository URL"
    },
    ["github_shorthand"]: {
        label: "GitHub",
        icon: "📦",
        description: "owner/repo shorthand"
    },
    ["session_search"]: {
        label: "Search Sessions",
        icon: "🔍",
        description: "Search existing sessions"
    },
    ["new_session"]: {
        label: "New Session",
        icon: "✨",
        description: "Create a new session"
    },
    ["spawn_shell"]: {
        label: "Spawn Shell",
        icon: ">_",
        description: "Open a terminal session, optionally in a directory or running a command"
    },
    ["workflow"]: {
        label: "Workflow",
        icon: "⚡",
        description: "Quick workflow invocation"
    },
    ["alias"]: {
        label: "Alias",
        icon: "@",
        description: "Named session preset"
    },
    ["alias_not_found"]: {
        label: "Alias Not Found",
        icon: "@",
        description: "No alias with that name"
    },
    ["alias_browse"]: {
        label: "Alias Browse",
        icon: "@",
        description: "Browse or filter aliases"
    },
    ["preset"]: {
        label: "Preset",
        icon: "🚀",
        description: "Launcher preset"
    },
    ["preset_not_found"]: {
        label: "Preset Not Found",
        icon: "🚀",
        description: "No preset with that id"
    },
    ["chat_backlog_item"]: {
        label: "Backlog Item",
        icon: "📝",
        description: "Create a backlog item from a free-text message (backlog: <message>)"
    },
    ["fan_out"]: {
        label: "Run as N variants",
        icon: "🔀",
        description: "Run one prompt across N isolated sessions (fanout N: <prompt>)"
    }
};


/***/ }),

/***/ 15681:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Ds: () => (/* binding */ generateInvite),
/* harmony export */   Ic: () => (/* binding */ revokeCredential),
/* harmony export */   Jc: () => (/* binding */ listCredentials),
/* harmony export */   Pq: () => (/* binding */ getAuthStatus),
/* harmony export */   eA: () => (/* binding */ registerPasskey),
/* harmony export */   ip: () => (/* binding */ loginWithPasskey),
/* harmony export */   ri: () => (/* binding */ logout)
/* harmony export */ });
/* harmony import */ var _simplewebauthn_browser__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(84000);
/* provided dependency */ var process = __webpack_require__(36843);
/**
 * Passkey authentication client utilities.
 * Uses @simplewebauthn/browser to handle the browser WebAuthn ceremony.
 */ 
/** Returns the /auth base URL using the current origin. */ function authBase() {
    // Explicit override always wins, even in the browser (e.g. `next dev`
    // running on its own port against a separately-ported backend).
    const apiUrl = process.env.NEXT_PUBLIC_API_URL;
    if (apiUrl) {
        const origin = apiUrl.replace(/\/api\/?$/, "");
        return origin + "/auth";
    }
    if (true) {
        return window.location.origin + "/auth";
    }
    return "http://localhost:8543/auth";
}
/** Fetch the current auth status from the server. */ async function getAuthStatus() {
    const resp = await fetch("".concat(authBase(), "/status"), {
        credentials: "include"
    });
    if (!resp.ok) {
        throw new Error("auth status failed: ".concat(resp.statusText));
    }
    return resp.json();
}
/**
 * Register a new passkey.
 * If a setupToken is provided it is passed as a query param (first-device bootstrap).
 */ async function registerPasskey(setupToken) {
    const base = authBase();
    // 1. Begin registration – server returns ceremony key + WebAuthn options
    const beginParams = setupToken ? "?setup_token=".concat(setupToken) : "";
    const beginResp = await fetch("".concat(base, "/register/begin").concat(beginParams), {
        method: "POST",
        credentials: "include"
    });
    if (!beginResp.ok) {
        const text = await beginResp.text();
        throw new Error("begin registration failed: ".concat(text));
    }
    const { ceremony_key, options } = await beginResp.json();
    // go-webauthn wraps options as { publicKey: {...} } per the W3C spec.
    // @simplewebauthn/browser expects the flat inner PublicKeyCredentialCreationOptionsJSON.
    const credential = await (0,_simplewebauthn_browser__WEBPACK_IMPORTED_MODULE_0__/* .startRegistration */ .Ct)({
        optionsJSON: options.publicKey
    });
    // 3. Finish registration – send credential back to server
    const finishResp = await fetch("".concat(base, "/register/finish?ceremony_key=").concat(ceremony_key).concat(setupToken ? "&setup_token=".concat(setupToken) : ""), {
        method: "POST",
        headers: {
            "Content-Type": "application/json"
        },
        body: JSON.stringify(credential),
        credentials: "include"
    });
    if (!finishResp.ok) {
        const text = await finishResp.text();
        throw new Error("finish registration failed: ".concat(text));
    }
}
/** Authenticate with an existing passkey. */ async function loginWithPasskey() {
    const base = authBase();
    // 1. Begin login
    const beginResp = await fetch("".concat(base, "/login/begin"), {
        method: "POST",
        credentials: "include"
    });
    if (!beginResp.ok) {
        const text = await beginResp.text();
        throw new Error("begin login failed: ".concat(text));
    }
    const { ceremony_key, options } = await beginResp.json();
    // go-webauthn wraps options as { publicKey: {...} } per the W3C spec.
    // @simplewebauthn/browser expects the flat inner PublicKeyCredentialRequestOptionsJSON.
    const credential = await (0,_simplewebauthn_browser__WEBPACK_IMPORTED_MODULE_0__/* .startAuthentication */ .HS)({
        optionsJSON: options.publicKey
    });
    // 3. Finish login
    const finishResp = await fetch("".concat(base, "/login/finish?ceremony_key=").concat(ceremony_key), {
        method: "POST",
        headers: {
            "Content-Type": "application/json"
        },
        body: JSON.stringify(credential),
        credentials: "include"
    });
    if (!finishResp.ok) {
        const text = await finishResp.text();
        throw new Error("finish login failed: ".concat(text));
    }
}
/** Log out (revoke the current session cookie). */ async function logout() {
    await fetch("".concat(authBase(), "/logout"), {
        method: "POST",
        credentials: "include"
    });
}
/** Generate a new one-time invite (requires authenticated session). */ async function generateInvite(label) {
    const resp = await fetch("".concat(authBase(), "/invite/generate"), {
        method: "POST",
        credentials: "include",
        headers: {
            "Content-Type": "application/json"
        },
        body: JSON.stringify({
            label
        })
    });
    if (!resp.ok) {
        const text = await resp.text();
        throw new Error("generate invite failed: ".concat(text));
    }
    return resp.json();
}
/** List all registered passkeys (requires authenticated session). */ async function listCredentials() {
    const resp = await fetch("".concat(authBase(), "/credentials"), {
        credentials: "include"
    });
    if (!resp.ok) {
        const text = await resp.text();
        throw new Error("list credentials failed: ".concat(text));
    }
    const data = await resp.json();
    var _data_credentials;
    return (_data_credentials = data.credentials) !== null && _data_credentials !== void 0 ? _data_credentials : [];
}
/** Revoke a passkey by its hex ID (requires authenticated session). */ async function revokeCredential(id) {
    const resp = await fetch("".concat(authBase(), "/credentials/").concat(id, "/revoke"), {
        method: "POST",
        credentials: "include"
    });
    if (!resp.ok) {
        const text = await resp.text();
        throw new Error("revoke credential failed: ".concat(text));
    }
    return resp.json();
}


/***/ }),

/***/ 16244:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   _: () => (/* binding */ addRecentShellCommand),
/* harmony export */   e: () => (/* binding */ getRecentShellCommands)
/* harmony export */ });
const STORAGE_KEY = "ssq.recentShellCommands";
const MAX_ENTRIES = 8;
function getRecentShellCommands() {
    if (false) {}
    try {
        const raw = window.localStorage.getItem(STORAGE_KEY);
        if (!raw) return [];
        const parsed = JSON.parse(raw);
        return Array.isArray(parsed) ? parsed.filter((v)=>typeof v === "string") : [];
    } catch (e) {
        return [];
    }
}
function addRecentShellCommand(command) {
    if (false) {}
    const trimmed = command.trim();
    if (!trimmed) return;
    try {
        const existing = getRecentShellCommands().filter((c)=>c !== trimmed);
        const updated = [
            trimmed,
            ...existing
        ].slice(0, MAX_ENTRIES);
        window.localStorage.setItem(STORAGE_KEY, JSON.stringify(updated));
    } catch (e) {
    // localStorage unavailable (e.g. private browsing quota) — history is best-effort
    }
}


/***/ }),

/***/ 16446:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   I: () => (/* binding */ useRemotesService)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(66144);
/* harmony import */ var _bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(95444);
/* harmony import */ var _gen_session_v1_remote_pb__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(88149);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(21694);
/* __next_internal_client_entry_do_not_use__ useRemotesService,RemoteConfigProtoSchema auto */ 




/**
 * React hook wrapping the ConnectRPC RemoteService client: the Settings ->
 * Remotes list (ListRemotes/DeleteRemote) and the Add Remote form's
 * generate-identity -> test-connection -> trust -> create-remote flow
 * (ssh-remote-workspaces Phase 6, Epic 6.1). A fresh client is constructed
 * per hook instance from the shared transport singleton (getConnectTransport),
 * mirroring useGitHubEnterpriseHosts/useApprovalRules's pattern.
 */ function useRemotesService() {
    const client = (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>(0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__/* .createClient */ .UU)(_gen_session_v1_remote_pb__WEBPACK_IMPORTED_MODULE_1__/* .RemoteService */ .ef, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_2__/* .getConnectTransport */ .qW)()), []);
    // The whole returned API is wrapped in useMemo (keyed on the already-stable `client`) so its
    // methods keep a stable identity across re-renders. Without this, every call to
    // useRemotesService() returned brand-new function objects, which is a footgun for any
    // caller putting one of them in a useEffect/useCallback dependency array (exactly what
    // RemotesPage.tsx's `refresh` does, depending on `listRemotes`) -- a fresh function identity
    // every render makes that effect re-fire every render, which re-triggers the state update
    // that caused the render, which changes identity again: an infinite ListRemotes fetch loop.
    // Found while writing remote-workspaces.spec.ts (ssh-remote-workspaces Phase 6 Epic 6.3):
    // the live page was issuing thousands of ListRemotes calls per second and the remotes list
    // was re-rendering continuously, which is also why row buttons kept "detaching" mid-click in
    // that spec before this fix.
    return (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>{
        const listRemotes = async ()=>{
            const resp = await client.listRemotes((0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_remote_pb__WEBPACK_IMPORTED_MODULE_1__/* .ListRemotesRequestSchema */ .Cj, {}));
            return resp.remotes;
        };
        const createRemote = async (input)=>{
            const resp = await client.createRemote((0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_remote_pb__WEBPACK_IMPORTED_MODULE_1__/* .CreateRemoteRequestSchema */ .TW, {
                name: input.name,
                host: input.host,
                user: input.user,
                port: input.port,
                basePath: input.basePath
            }));
            if (!resp.remote) {
                // Defensive: the backend always populates this on success (a failure
                // surfaces as a thrown ConnectError instead) -- this only guards
                // against a future contract drift silently producing a null remote.
                throw new Error("CreateRemote succeeded but returned no remote");
            }
            return resp.remote;
        };
        const deleteRemote = async (name)=>{
            await client.deleteRemote((0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_remote_pb__WEBPACK_IMPORTED_MODULE_1__/* .DeleteRemoteRequestSchema */ .eB, {
                name
            }));
        };
        const generateRemoteIdentity = async (name)=>{
            const resp = await client.generateRemoteIdentity((0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_remote_pb__WEBPACK_IMPORTED_MODULE_1__/* .GenerateRemoteIdentityRequestSchema */ .cj, {
                name
            }));
            return {
                publicKeyText: resp.publicKeyText,
                authorizedKeysLine: resp.authorizedKeysLine
            };
        };
        const toDraftProto = (draft)=>(0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_remote_pb__WEBPACK_IMPORTED_MODULE_1__/* .DraftRemoteTargetSchema */ .ag, {
                name: draft.name,
                host: draft.host,
                user: draft.user,
                port: draft.port
            });
        const testRemoteConnectionDraft = async (draft)=>{
            const resp = await client.testRemoteConnection((0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_remote_pb__WEBPACK_IMPORTED_MODULE_1__/* .TestRemoteConnectionRequestSchema */ .Jt, {
                draft: toDraftProto(draft)
            }));
            return {
                success: resp.success,
                hostKeyUnknown: resp.hostKeyUnknown,
                fingerprint: resp.fingerprint,
                errorMessage: resp.errorMessage
            };
        };
        const testRemoteConnectionSaved = async (remoteName)=>{
            const resp = await client.testRemoteConnection((0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_remote_pb__WEBPACK_IMPORTED_MODULE_1__/* .TestRemoteConnectionRequestSchema */ .Jt, {
                remoteName
            }));
            return {
                success: resp.success,
                hostKeyUnknown: resp.hostKeyUnknown,
                fingerprint: resp.fingerprint,
                errorMessage: resp.errorMessage
            };
        };
        const trustRemoteHostKeyDraft = async (draft, fingerprint)=>{
            const resp = await client.trustRemoteHostKey((0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_remote_pb__WEBPACK_IMPORTED_MODULE_1__/* .TrustRemoteHostKeyRequestSchema */ .Ek, {
                draft: toDraftProto(draft),
                fingerprint
            }));
            return {
                success: resp.success,
                errorMessage: resp.errorMessage
            };
        };
        const trustRemoteHostKeySaved = async (remoteName, fingerprint)=>{
            const resp = await client.trustRemoteHostKey((0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_remote_pb__WEBPACK_IMPORTED_MODULE_1__/* .TrustRemoteHostKeyRequestSchema */ .Ek, {
                remoteName,
                fingerprint
            }));
            return {
                success: resp.success,
                errorMessage: resp.errorMessage
            };
        };
        return {
            listRemotes,
            createRemote,
            deleteRemote,
            generateRemoteIdentity,
            testRemoteConnectionDraft,
            testRemoteConnectionSaved,
            trustRemoteHostKeyDraft,
            trustRemoteHostKeySaved
        };
    }, [
        client
    ]);
}



/***/ }),

/***/ 22017:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   k: () => (/* binding */ validateFlags)
/* harmony export */ });
/** Splits on whitespace, honouring single/double quotes; quoted tokens are values, never flags. */ function tokenize(input) {
    const tokens = [];
    let text = "";
    let inToken = false;
    let quoted = false;
    let quote = "";
    const flush = ()=>{
        if (inToken) tokens.push({
            text,
            quoted
        });
        text = "";
        inToken = false;
        quoted = false;
    };
    for (const ch of input){
        if (quote) {
            if (ch === quote) quote = "";
            else text += ch;
        } else if (ch === '"' || ch === "'") {
            quote = ch;
            quoted = true;
            inToken = true;
        } else if (/\s/.test(ch)) {
            flush();
        } else {
            text += ch;
            inToken = true;
        }
    }
    flush();
    return tokens;
}
/**
 * Long flags in `input` that `flags` (a probe's parsed --help) does not list.
 * Precision-first: bundled or unknown single-dash tokens, values, `--x=value`
 * names that are known, `--no-<known>` and everything after `--` are never
 * reported. Returns [] when `flags` is empty.
 */ function validateFlags(input, flags) {
    if (flags.length === 0) return [];
    const byName = new Map();
    for (const f of flags){
        for (const n of [
            f.name,
            f.short,
            ...f.aliases
        ])if (n) byName.set(n, f);
    }
    const unknown = [];
    let expectsValue = false;
    for (const { text, quoted } of tokenize(input)){
        if (text === "--" && !quoted) break;
        if (expectsValue) {
            expectsValue = false;
            continue;
        }
        if (quoted || !text.startsWith("-") || text === "-") continue;
        const eq = text.indexOf("=");
        const name = eq === -1 ? text : text.slice(0, eq);
        const known = byName.get(name);
        if (known) {
            expectsValue = known.takesValue && eq === -1;
        } else if (name.startsWith("--no-") && byName.has("--".concat(name.slice(5)))) {
            continue;
        } else if (name.startsWith("--") && !unknown.includes(name)) {
            unknown.push(name);
        }
    }
    return unknown;
}


/***/ }),

/***/ 25562:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


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

/***/ 27980:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  _: () => (/* binding */ lazy_lazyComponent)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/api/app-dynamic.js
var app_dynamic = __webpack_require__(97202);
;// ./src/lib/lazy/impl.next.tsx

// ssr:false is a Next-only concern (xterm and recharts need the browser), so it lives here, not at call sites.
const lazyComponent = (loader, options)=>(0,app_dynamic["default"])(loader, {
        ssr: false,
        loading: options === null || options === void 0 ? void 0 : options.loading
    });

;// ./src/lib/lazy/index.ts
// `@/lib/lazy/impl` is aliased per build (next.config.ts, tsconfig paths, jest moduleNameMapper).

/** Code-split a component; the build-specific impl owns SSR and stale-chunk behaviour. */ const lazy_lazyComponent = lazyComponent;


/***/ }),

/***/ 31750:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   $e: () => (/* binding */ warning),
/* harmony export */   GI: () => (/* binding */ flagName),
/* harmony export */   I3: () => (/* binding */ disclosureToggle),
/* harmony export */   KQ: () => (/* binding */ flagItem),
/* harmony export */   zg: () => (/* binding */ flagList)
/* harmony export */ });
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(65916);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FFlagNotes_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xMXVmcGZ0MCB7CiAgZGlzcGxheTogZmxleDsKICBhbGlnbi1pdGVtczogZmxleC1zdGFydDsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MnopOwogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MnopIHZhcigtLV8xOWV6MnJ6MzApOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzcpOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnp5KTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnp6KTsKICBvdmVyZmxvdy13cmFwOiBhbnl3aGVyZTsKICBtaW4td2lkdGg6IDA7Cn0KLl8xMXVmcGZ0MSB7CiAgbWluLWhlaWdodDogNDRweDsKICBwYWRkaW5nOiAwIHZhcigtLV8xOWV6MnJ6MnopOwogIGJhY2tncm91bmQ6IHRyYW5zcGFyZW50OwogIGJvcmRlcjogbm9uZTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnowKTsKICBmb250OiBpbmhlcml0OwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgY3Vyc29yOiBwb2ludGVyOwogIHRleHQtYWxpZ246IGxlZnQ7Cn0KLl8xMXVmcGZ0MiB7CiAgbGlzdC1zdHlsZTogbm9uZTsKICBtYXJnaW46IDA7CiAgcGFkZGluZzogMDsKfQouXzExdWZwZnQzIHsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXgtd3JhcDogd3JhcDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeik7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKfQouXzExdWZwZnQ0IHsKICBmb250LWZhbWlseTogdmFyKC0tXzE5ZXoycnoydSk7CiAgb3ZlcmZsb3ctd3JhcDogYW55d2hlcmU7Cn0_22_7D__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(12403);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FFlagNotes_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xMXVmcGZ0MCB7CiAgZGlzcGxheTogZmxleDsKICBhbGlnbi1pdGVtczogZmxleC1zdGFydDsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MnopOwogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MnopIHZhcigtLV8xOWV6MnJ6MzApOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzcpOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnp5KTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnp6KTsKICBvdmVyZmxvdy13cmFwOiBhbnl3aGVyZTsKICBtaW4td2lkdGg6IDA7Cn0KLl8xMXVmcGZ0MSB7CiAgbWluLWhlaWdodDogNDRweDsKICBwYWRkaW5nOiAwIHZhcigtLV8xOWV6MnJ6MnopOwogIGJhY2tncm91bmQ6IHRyYW5zcGFyZW50OwogIGJvcmRlcjogbm9uZTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnowKTsKICBmb250OiBpbmhlcml0OwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgY3Vyc29yOiBwb2ludGVyOwogIHRleHQtYWxpZ246IGxlZnQ7Cn0KLl8xMXVmcGZ0MiB7CiAgbGlzdC1zdHlsZTogbm9uZTsKICBtYXJnaW46IDA7CiAgcGFkZGluZzogMDsKfQouXzExdWZwZnQzIHsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXgtd3JhcDogd3JhcDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeik7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKfQouXzExdWZwZnQ0IHsKICBmb250LWZhbWlseTogdmFyKC0tXzE5ZXoycnoydSk7CiAgb3ZlcmZsb3ctd3JhcDogYW55d2hlcmU7Cn0_22_7D__WEBPACK_IMPORTED_MODULE_1___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FFlagNotes_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xMXVmcGZ0MCB7CiAgZGlzcGxheTogZmxleDsKICBhbGlnbi1pdGVtczogZmxleC1zdGFydDsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MnopOwogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MnopIHZhcigtLV8xOWV6MnJ6MzApOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzcpOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnp5KTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnp6KTsKICBvdmVyZmxvdy13cmFwOiBhbnl3aGVyZTsKICBtaW4td2lkdGg6IDA7Cn0KLl8xMXVmcGZ0MSB7CiAgbWluLWhlaWdodDogNDRweDsKICBwYWRkaW5nOiAwIHZhcigtLV8xOWV6MnJ6MnopOwogIGJhY2tncm91bmQ6IHRyYW5zcGFyZW50OwogIGJvcmRlcjogbm9uZTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnowKTsKICBmb250OiBpbmhlcml0OwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgY3Vyc29yOiBwb2ludGVyOwogIHRleHQtYWxpZ246IGxlZnQ7Cn0KLl8xMXVmcGZ0MiB7CiAgbGlzdC1zdHlsZTogbm9uZTsKICBtYXJnaW46IDA7CiAgcGFkZGluZzogMDsKfQouXzExdWZwZnQzIHsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXgtd3JhcDogd3JhcDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeik7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKfQouXzExdWZwZnQ0IHsKICBmb250LWZhbWlseTogdmFyKC0tXzE5ZXoycnoydSk7CiAgb3ZlcmZsb3ctd3JhcDogYW55d2hlcmU7Cn0_22_7D__WEBPACK_IMPORTED_MODULE_1__);


var disclosureToggle = '_11ufpft1';
var flagItem = '_11ufpft3';
var flagList = '_11ufpft2';
var flagName = '_11ufpft4';
var warning = '_11ufpft0';

/***/ }),

/***/ 40218:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  U: () => (/* binding */ OmnibarProvider),
  W: () => (/* binding */ useOmnibar)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/nav/index.ts
var nav = __webpack_require__(41078);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/fuse.js@7.3.0/node_modules/fuse.js/dist/fuse.mjs
var dist_fuse = __webpack_require__(66000);
// EXTERNAL MODULE: ./src/lib/omnibar/types.ts
var types = __webpack_require__(11804);
;// ./src/lib/omnibar/detectors/CommandDetector.ts

/**
 * CommandDetector — detects VS Code-style `>command` prefix.
 * Priority 5 — runs before all other detectors.
 *
 * Recognized commands:
 *   >theme matrix | cyberpunk77 | wh40k | clean | light | dark
 *   >go sessions | review | history
 *   >shell [dir] [-- command]   — open a terminal session, optionally rooted at
 *     `dir` and/or running `command` as the session program instead of a plain shell.
 */ /** Splits the raw text after `>shell` into an optional directory and an optional `-- command` tail. */ function parseShellArgs(rawArgs) {
    const trimmed = (rawArgs !== null && rawArgs !== void 0 ? rawArgs : "").trim();
    if (!trimmed) return {};
    const tokens = trimmed.split(/\s+/);
    const dashIndex = tokens.indexOf("--");
    if (dashIndex === -1) return {
        dir: trimmed
    };
    const dir = tokens.slice(0, dashIndex).join(" ").trim();
    const command = tokens.slice(dashIndex + 1).join(" ").trim();
    return {
        dir: dir || undefined,
        command: command || undefined
    };
}
class CommandDetector {
    detect(input) {
        const trimmed = input.trim();
        if (!trimmed.startsWith(">")) return null;
        for (const cmd of CommandDetector.COMMANDS){
            const match = trimmed.match(cmd.pattern);
            if (match) {
                if (cmd.commandType === "spawn_shell") {
                    const { dir, command } = parseShellArgs(match[1]);
                    const suggestedName = command ? 'Run "'.concat(command, '"').concat(dir ? " in ".concat(dir) : "") : dir ? "Open terminal in ".concat(dir) : "Open terminal";
                    return {
                        type: types/* InputType */.N.SpawnShell,
                        confidence: 1.0,
                        parsedValue: trimmed,
                        suggestedName,
                        metadata: {
                            commandType: cmd.commandType,
                            shellDir: dir,
                            shellCommand: command
                        }
                    };
                }
                return {
                    type: types/* InputType */.N.Command,
                    confidence: 1.0,
                    parsedValue: trimmed,
                    suggestedName: cmd.suggestedName,
                    metadata: {
                        commandType: cmd.commandType,
                        commandArg: cmd.commandArg
                    }
                };
            }
        }
        // Unrecognized > command — still return a result with low confidence
        if (trimmed.length > 1) {
            return {
                type: types/* InputType */.N.Unknown,
                confidence: 0.3,
                parsedValue: trimmed,
                suggestedName: "Unknown command",
                metadata: {}
            };
        }
        return null;
    }
    constructor(){
        this.name = "CommandDetector";
        this.priority = 5;
    }
}
// Map of command string → type and argument
CommandDetector.COMMANDS = [
    {
        pattern: /^>theme\s+matrix$/i,
        commandType: "theme",
        commandArg: "matrix",
        suggestedName: "Switch to Matrix theme"
    },
    {
        pattern: /^>theme\s+cyberpunk77$/i,
        commandType: "theme",
        commandArg: "cyberpunk77",
        suggestedName: "Switch to Cyberpunk 77 theme"
    },
    {
        pattern: /^>theme\s+wh40k$/i,
        commandType: "theme",
        commandArg: "wh40k",
        suggestedName: "Switch to WH40K theme"
    },
    {
        pattern: /^>theme\s+clean$/i,
        commandType: "theme",
        commandArg: "clean",
        suggestedName: "Switch to Clean theme"
    },
    {
        pattern: /^>theme\s+light$/i,
        commandType: "theme",
        commandArg: "light",
        suggestedName: "Switch to Light theme"
    },
    {
        pattern: /^>theme\s+dark$/i,
        commandType: "theme",
        commandArg: "dark",
        suggestedName: "Switch to Dark theme"
    },
    {
        pattern: /^>go\s+sessions?$/i,
        commandType: "navigate",
        commandArg: "/",
        suggestedName: "Go to Sessions"
    },
    {
        pattern: /^>go\s+review$/i,
        commandType: "navigate",
        commandArg: "/review-queue",
        suggestedName: "Go to Review Queue"
    },
    {
        pattern: /^>go\s+history$/i,
        commandType: "navigate",
        commandArg: "/history",
        suggestedName: "Go to History"
    },
    {
        pattern: /^>shell(?:\s+(.+))?$/i,
        commandType: "spawn_shell",
        commandArg: "",
        suggestedName: "Spawn new shell"
    }
];

;// ./src/lib/omnibar/detectors/GitHubEnterpriseURLDetector.ts
/**
 * GitHubEnterpriseURLDetector matches GitHub PR/branch/repo URLs against a
 * caller-supplied list of configured GHES hostnames — the same URL shapes
 * the static GitHubPRDetector/GitHubBranchDetector/GitHubRepoDetector match
 * for github.com. Priority 15 — after GitHubPRDetector (10), before
 * GitHubBranchDetector (20).
 *
 * The host list starts empty and the detector is registered synchronously as
 * part of createDefaultRegistry() — see detector.ts — so the registry always
 * has an enterprise-detector slot. OmnibarContext calls setHosts() once the
 * enterprise host list loads (or changes) instead of swapping the detector
 * instance via unregister/register, which previously left a window (before
 * the async host-loading effect first ran) where no enterprise detector was
 * registered at all and GHES URLs fell through to the SessionSearch
 * catch-all.
 */ 
function escapeRegExp(value) {
    return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
class GitHubEnterpriseURLDetector {
    /** Replace the configured host list in place (called as hosts load/change). */ setHosts(hosts) {
        this.hosts = hosts.filter(Boolean);
    }
    detect(input) {
        const trimmed = input.trim();
        for (const host of this.hosts){
            const result = this.detectForHost(trimmed, host);
            if (result) return result;
        }
        return null;
    }
    detectForHost(trimmed, host) {
        const h = escapeRegExp(host);
        const prMatch = trimmed.match(new RegExp("^https?://".concat(h, "/([^/]+)/([^/]+)/pull/(\\d+)"), "i"));
        if (prMatch) {
            const [, owner, repo, prNumber] = prMatch;
            const gitHubRef = {
                owner,
                repo,
                prNumber: parseInt(prNumber, 10)
            };
            return {
                type: types/* InputType */.N.GitHubPR,
                confidence: 1.0,
                parsedValue: trimmed,
                suggestedName: "".concat(owner, "-").concat(repo, "-pr-").concat(prNumber),
                gitHubRef
            };
        }
        const branchMatch = trimmed.match(new RegExp("^https?://".concat(h, "/([^/]+)/([^/]+)/tree/(.+)"), "i"));
        if (branchMatch) {
            const [, owner, repo, branch] = branchMatch;
            const gitHubRef = {
                owner,
                repo,
                branch
            };
            return {
                type: types/* InputType */.N.GitHubBranch,
                confidence: 1.0,
                parsedValue: trimmed,
                suggestedName: "".concat(owner, "-").concat(repo, "-").concat(branch.replace(/\//g, "-")),
                branch,
                gitHubRef
            };
        }
        const repoMatch = trimmed.match(new RegExp("^https?://".concat(h, "/([^/]+)/([^/]+)/?$"), "i"));
        if (repoMatch) {
            const [, owner, repo] = repoMatch;
            const cleanRepo = repo.replace(/\.git$/, "");
            const gitHubRef = {
                owner,
                repo: cleanRepo
            };
            return {
                type: types/* InputType */.N.GitHubRepo,
                confidence: 1.0,
                parsedValue: trimmed,
                suggestedName: cleanRepo,
                gitHubRef
            };
        }
        return null;
    }
    constructor(hosts = []){
        this.name = "GitHubEnterpriseURL";
        this.priority = 15;
        this.hosts = hosts.filter(Boolean);
    }
}

;// ./src/lib/omnibar/slugify.ts
function toSessionSlug(input) {
    const slug = input.trim().toLowerCase().replace(/[^a-z0-9\s-]/g, "").replace(/\s+/g, "-").replace(/-{2,}/g, "-").replace(/^-|-$/g, "").slice(0, 60).replace(/-+$/, "");
    return slug;
}

;// ./src/lib/omnibar/detector.ts
/**
 * Input type detection engine for the omnibar
 * Ported from Go implementation at ui/overlay/omnibar/detector.go
 */ 



/**
 * GitHub PR URL detector
 * Matches: https://github.com/owner/repo/pull/123
 */ class GitHubPRDetector {
    detect(input) {
        const match = input.trim().match(this.pattern);
        if (!match) return null;
        const [, owner, repo, prNumber] = match;
        const gitHubRef = {
            owner,
            repo,
            prNumber: parseInt(prNumber, 10)
        };
        return {
            type: types/* InputType */.N.GitHubPR,
            confidence: 1.0,
            parsedValue: input.trim(),
            suggestedName: "".concat(owner, "-").concat(repo, "-pr-").concat(prNumber),
            gitHubRef
        };
    }
    constructor(){
        this.name = "GitHubPR";
        this.priority = 10;
        this.pattern = /^https?:\/\/github\.com\/([^/]+)\/([^/]+)\/pull\/(\d+)/i;
    }
}
/**
 * GitHub Branch URL detector
 * Matches: https://github.com/owner/repo/tree/branch-name
 */ class GitHubBranchDetector {
    detect(input) {
        const match = input.trim().match(this.pattern);
        if (!match) return null;
        const [, owner, repo, branch] = match;
        const gitHubRef = {
            owner,
            repo,
            branch
        };
        return {
            type: types/* InputType */.N.GitHubBranch,
            confidence: 1.0,
            parsedValue: input.trim(),
            suggestedName: "".concat(owner, "-").concat(repo, "-").concat(branch.replace(/\//g, "-")),
            branch,
            gitHubRef
        };
    }
    constructor(){
        this.name = "GitHubBranch";
        this.priority = 20;
        this.pattern = /^https?:\/\/github\.com\/([^/]+)\/([^/]+)\/tree\/(.+)/i;
    }
}
/**
 * GitHub Repository URL detector
 * Matches: https://github.com/owner/repo
 */ class GitHubRepoDetector {
    detect(input) {
        const match = input.trim().match(this.pattern);
        if (!match) return null;
        const [, owner, repo] = match;
        // Clean repo name (remove .git suffix if present)
        const cleanRepo = repo.replace(/\.git$/, "");
        const gitHubRef = {
            owner,
            repo: cleanRepo
        };
        return {
            type: types/* InputType */.N.GitHubRepo,
            confidence: 1.0,
            parsedValue: input.trim(),
            suggestedName: cleanRepo,
            gitHubRef
        };
    }
    constructor(){
        this.name = "GitHubRepo";
        this.priority = 30;
        this.pattern = /^https?:\/\/github\.com\/([^/]+)\/([^/]+)\/?$/i;
    }
}
/**
 * GitHub Shorthand detector
 * Matches: owner/repo or owner/repo:branch
 */ class GitHubShorthandDetector {
    detect(input) {
        const trimmed = input.trim();
        // Skip if it looks like a local path
        if (trimmed.startsWith("/") || trimmed.startsWith("~") || trimmed.startsWith(".")) {
            return null;
        }
        const match = trimmed.match(this.pattern);
        if (!match) return null;
        const [, owner, repo, branch] = match;
        const gitHubRef = {
            owner,
            repo,
            branch
        };
        const suggestedName = branch ? "".concat(repo, "-").concat(branch.replace(/\//g, "-")) : repo;
        return {
            type: types/* InputType */.N.GitHubShorthand,
            confidence: 0.9,
            parsedValue: trimmed,
            suggestedName,
            branch,
            gitHubRef
        };
    }
    constructor(){
        this.name = "GitHubShorthand";
        this.priority = 40;
        // Match owner/repo or owner/repo:branch (but not paths starting with / or ~)
        this.pattern = /^([a-zA-Z0-9_-]+)\/([a-zA-Z0-9_.-]+)(?::([a-zA-Z0-9_/.-]+))?$/;
    }
}
/**
 * Path with Branch detector
 * Matches: /path/to/repo@branch-name or ~/path@branch
 */ class PathWithBranchDetector {
    detect(input) {
        const trimmed = input.trim();
        // Must contain @ but not be a URL
        if (!trimmed.includes("@") || trimmed.includes("://")) {
            return null;
        }
        const match = trimmed.match(this.pattern);
        if (!match) return null;
        const [, path, branch] = match;
        // Validate branch name
        if (!this.isValidBranchName(branch)) {
            return null;
        }
        // Extract suggested name from path
        const pathParts = path.split("/").filter(Boolean);
        const dirName = pathParts[pathParts.length - 1] || "session";
        const suggestedName = "".concat(dirName, "-").concat(branch.replace(/\//g, "-"));
        return {
            type: types/* InputType */.N.PathWithBranch,
            confidence: 0.9,
            parsedValue: trimmed,
            suggestedName,
            localPath: path,
            branch
        };
    }
    isValidBranchName(name) {
        if (!name || name.length > 255) return false;
        if (name.startsWith(".") || name.startsWith("-")) return false;
        if (name.endsWith(".lock") || name.endsWith(".")) return false;
        if (name.includes("..") || name.includes("//")) return false;
        if (/[~^:?*\[\\\s]/.test(name)) return false;
        return true;
    }
    constructor(){
        this.name = "PathWithBranch";
        this.priority = 50;
        // Valid git branch name: alphanumeric, hyphens, underscores, forward slashes
        this.pattern = /^(.+)@([a-zA-Z0-9_/.-]+)$/;
    }
}
/**
 * Local Path detector (catch-all)
 * Matches: /path/to/dir, ~/path, ./path
 */ class LocalPathDetector {
    detect(input) {
        const trimmed = input.trim();
        if (!trimmed) return null;
        // Check for path-like patterns
        const isAbsolute = trimmed.startsWith("/");
        const hasTilde = trimmed.startsWith("~/");
        const isRelative = trimmed.startsWith("./") || trimmed.startsWith("../");
        // Also accept paths with multiple slashes that aren't GitHub shorthand
        const hasMultipleSlashes = (trimmed.match(/\//g) || []).length > 1;
        if (!isAbsolute && !hasTilde && !isRelative && !hasMultipleSlashes) {
            return null;
        }
        // Skip if contains @ (handled by PathWithBranch detector)
        if (trimmed.includes("@") && !trimmed.includes("://")) {
            return null;
        }
        // Skip URLs
        if (trimmed.includes("://")) {
            return null;
        }
        // Extract suggested name from path
        const pathParts = trimmed.split("/").filter(Boolean);
        const suggestedName = pathParts[pathParts.length - 1] || "session";
        return {
            type: types/* InputType */.N.LocalPath,
            confidence: 0.8,
            parsedValue: trimmed,
            suggestedName,
            localPath: trimmed
        };
    }
    constructor(){
        this.name = "LocalPath";
        this.priority = 100;
    }
}
/**
 * Chat Backlog Item detector — matches the "backlog:" trigger prefix for
 * free-text backlog item creation via chat. Priority 15 places it after
 * GitHubPR (10) but before GitHubBranch (20)/NewSession (35)/SessionSearch (200)
 * so it never shadows those detectors and is never shadowed by them.
 */ class ChatBacklogItemDetector {
    detect(input) {
        const trimmed = input.trim();
        if (!this.PREFIX_RE.test(trimmed)) return null;
        const message = trimmed.replace(this.PREFIX_RE, "").trim();
        if (!message) return null;
        return {
            type: types/* InputType */.N.ChatBacklogItem,
            confidence: 1.0,
            parsedValue: message,
            suggestedName: message
        };
    }
    constructor(){
        this.name = "ChatBacklogItem";
        this.priority = 15;
        this.PREFIX_RE = /^backlog:\s*/i;
    }
}
/**
 * Fan-out detector — matches "fanout N: <prompt>" (N in 2-5, the server's cap) so the omnibar
 * can prefill the variants count and first prompt. Priority 16 sits just after ChatBacklogItem (15).
 */ class FanOutDetector {
    detect(input) {
        const m = this.RE.exec(input.trim());
        if (!m) return null;
        const prompt = m[2].trim();
        if (!prompt) return null;
        return {
            type: types/* InputType */.N.FanOut,
            confidence: 1.0,
            parsedValue: prompt,
            suggestedName: prompt.slice(0, 40),
            metadata: {
                fanoutCount: Number(m[1])
            }
        };
    }
    constructor(){
        this.name = "FanOut";
        this.priority = 16;
        this.RE = RegExp("^fanout\\s+([2-5])\\s*:\\s*(.+)$", "is");
    }
}
/**
 * New Session detector — matches the "new/" prefix shortcut.
 * Priority 35 places it before GitHubShorthand (40) so "new/foo" is always
 * treated as a new-session command, not a GitHub owner/repo shorthand.
 * Accepts case-insensitive "new/" prefix; extracts the query after the slash.
 */ class NewSessionDetector {
    detect(input) {
        const trimmed = input.trim();
        if (!this.PREFIX_RE.test(trimmed)) return null;
        const query = trimmed.replace(this.PREFIX_RE, "");
        return {
            type: types/* InputType */.N.NewSession,
            confidence: 1.0,
            parsedValue: query,
            suggestedName: query
        };
    }
    constructor(){
        this.name = "NewSession";
        this.priority = 35;
        this.PREFIX_RE = /^new\//i;
    }
}
/**
 * Session Search detector — catch-all for bare-text queries.
 * Fires for any non-empty input that no other detector claimed.
 * Priority 200 ensures it runs last (after LocalPath at priority 100).
 */ class SessionSearchDetector {
    detect(input) {
        const trimmed = input.trim();
        if (!trimmed) return null;
        // All inputs that reach this point have already been rejected by
        // GitHub URL detectors (priorities 10-40), PathWithBranch (50),
        // and LocalPath (100). They are bare-text session search queries.
        return {
            type: types/* InputType */.N.SessionSearch,
            confidence: 0.5,
            parsedValue: trimmed,
            suggestedName: toSessionSlug(trimmed)
        };
    }
    constructor(){
        this.name = "SessionSearch";
        this.priority = 200;
    }
}
/**
 * DetectorRegistry manages detectors and orchestrates detection
 */ class DetectorRegistry {
    register(detector) {
        this.detectors.push(detector);
        // Sort by priority (lower = higher priority)
        this.detectors.sort((a, b)=>a.priority - b.priority);
    }
    /**
   * Remove a detector from the registry.
   * Safe to call when the detector is not registered (no-op).
   */ unregister(detector) {
        this.detectors = this.detectors.filter((d)=>d !== detector);
    }
    /** Look up a registered detector by name (e.g. to mutate its state in place). */ find(name) {
        return this.detectors.find((d)=>d.name === name);
    }
    detect(input) {
        for (const detector of this.detectors){
            try {
                const result = detector.detect(input);
                if (result) {
                    return result;
                }
            } catch (err) {
                // A single misbehaving detector must not prevent lower-priority
                // detectors (or the next debounce tick) from running.
                console.error('[DetectorRegistry] detector "'.concat(detector.name, '" threw during detect():'), err);
            }
        }
        return {
            type: types/* InputType */.N.Unknown,
            confidence: 0,
            parsedValue: input,
            suggestedName: ""
        };
    }
    detectAll(input) {
        const results = [];
        for (const detector of this.detectors){
            try {
                const result = detector.detect(input);
                if (result) {
                    results.push(result);
                }
            } catch (err) {
                console.error('[DetectorRegistry] detector "'.concat(detector.name, '" threw during detect():'), err);
            }
        }
        return results;
    }
    constructor(){
        this.detectors = [];
    }
}
// Create default registry with all detectors
function createDefaultRegistry() {
    const registry = new DetectorRegistry();
    registry.register(new CommandDetector());
    registry.register(new GitHubPRDetector());
    registry.register(new GitHubEnterpriseURLDetector());
    registry.register(new ChatBacklogItemDetector());
    registry.register(new FanOutDetector());
    registry.register(new GitHubBranchDetector());
    registry.register(new GitHubRepoDetector());
    registry.register(new NewSessionDetector());
    registry.register(new GitHubShorthandDetector());
    registry.register(new PathWithBranchDetector());
    registry.register(new LocalPathDetector());
    registry.register(new SessionSearchDetector());
    return registry;
}
// Singleton default registry
let defaultRegistry = null;
function getDefaultRegistry() {
    if (!defaultRegistry) {
        defaultRegistry = createDefaultRegistry();
    }
    return defaultRegistry;
}
/**
 * Reset the singleton registry to null so subsequent getDefaultRegistry() calls
 * return a fresh instance. Use in test afterEach blocks for test isolation.
 */ function resetDefaultRegistry() {
    defaultRegistry = null;
}
/**
 * Detect input type using the default registry
 */ function detect(input) {
    return getDefaultRegistry().detect(input);
}

;// ./src/lib/omnibar/index.ts



;// ./src/lib/omnibar/modes/useModeReducer.ts


const CREATION_TYPES = new Set([
    types/* InputType */.N.LocalPath,
    types/* InputType */.N.PathWithBranch,
    types/* InputType */.N.GitHubPR,
    types/* InputType */.N.GitHubBranch,
    types/* InputType */.N.GitHubRepo,
    types/* InputType */.N.GitHubShorthand,
    types/* InputType */.N.Alias
]);
function modeReducer(state, action) {
    switch(action.kind){
        case "detect":
            {
                if (CREATION_TYPES.has(action.detection.type)) {
                    return {
                        type: "creation",
                        detection: action.detection
                    };
                }
                return {
                    type: "discovery"
                };
            }
        case "select_repo":
            return {
                type: "creation_with_repo",
                path: action.path
            };
        case "open_creation_direct":
            return {
                type: "creation"
            };
        case "new_prefix_typed":
            var _action_query;
            return {
                type: "creation_with_repo",
                path: (_action_query = action.query) !== null && _action_query !== void 0 ? _action_query : ""
            };
        case "reset_to_discovery":
            return {
                type: "discovery"
            };
    }
}
function useModeReducer() {
    return (0,react.useReducer)(modeReducer, {
        type: "discovery"
    });
}

// EXTERNAL MODULE: ./src/lib/config.ts
var config = __webpack_require__(51202);
// EXTERNAL MODULE: ./src/lib/contexts/ThemeContext.tsx + 1 modules
var ThemeContext = __webpack_require__(45308);
// EXTERNAL MODULE: ./src/lib/hooks/usePathCompletions.ts
var usePathCompletions = __webpack_require__(44762);
;// ./src/lib/hooks/usePathHistory.ts
/* __next_internal_client_entry_do_not_use__ clearPathHistory,usePathHistory auto */ 
const STORAGE_KEY = "omnibar:path-history";
const MAX_ENTRIES = 50;
const MAX_RESULTS = 10;
function recencyScore(lastUsed) {
    const age = Date.now() - lastUsed;
    const hour = 3600000;
    const day = 86400000;
    const week = 7 * day;
    const month = 30 * day;
    if (age < hour) return 1.0;
    if (age < day) return 0.8;
    if (age < week) return 0.6;
    if (age < month) return 0.4;
    return 0.2;
}
function entryScore(e) {
    return recencyScore(e.lastUsed) + Math.log1p(e.count);
}
function loadFromStorage() {
    try {
        const raw = localStorage.getItem(STORAGE_KEY);
        if (!raw) return [];
        return JSON.parse(raw);
    } catch (e) {
        return [];
    }
}
function persistToStorage(entries) {
    try {
        localStorage.setItem(STORAGE_KEY, JSON.stringify(entries));
    } catch (e) {
    // Ignore storage errors (private browsing, quota exceeded)
    }
}
/** Remove all saved history (for testing / clear-history UX). */ function clearPathHistory() {
    try {
        localStorage.removeItem(STORAGE_KEY);
    } catch (e) {
    // ignore
    }
}
function usePathHistory() {
    const [entries, setEntries] = (0,react.useState)(()=>loadFromStorage());
    /**
   * Return stored paths that start with `prefix`, sorted by score desc.
   * Excludes exact matches (the user already typed that path).
   */ const getMatching = (0,react.useCallback)((prefix)=>{
        if (!prefix) return [];
        return entries.filter((e)=>e.path.startsWith(prefix) && e.path !== prefix).sort((a, b)=>entryScore(b) - entryScore(a)).slice(0, MAX_RESULTS);
    }, [
        entries
    ]);
    /**
   * Return all stored paths sorted by score desc, limited to `limit` entries.
   * Used for "Recent Repos" empty-state display when no query is active.
   */ const getAll = (0,react.useCallback)(function() {
        let limit = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : MAX_RESULTS;
        return [
            ...entries
        ].sort((a, b)=>entryScore(b) - entryScore(a)).slice(0, limit);
    }, [
        entries
    ]);
    /** Record a path submission. Increments count if already stored. */ const save = (0,react.useCallback)((path)=>{
        setEntries((prev)=>{
            const copy = prev.map((e)=>({
                    ...e
                }));
            const existing = copy.find((e)=>e.path === path);
            const now = Date.now();
            if (existing) {
                existing.count += 1;
                existing.lastUsed = now;
            } else {
                copy.push({
                    path,
                    count: 1,
                    lastUsed: now
                });
            }
            copy.sort((a, b)=>entryScore(b) - entryScore(a));
            const trimmed = copy.slice(0, MAX_ENTRIES);
            persistToStorage(trimmed);
            return trimmed;
        });
    }, []);
    return {
        getMatching,
        getAll,
        save
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ./src/gen/session/v1/session_pb.ts
var session_pb = __webpack_require__(23575);
// EXTERNAL MODULE: ./src/lib/api/transport.ts
var transport = __webpack_require__(21694);
;// ./src/lib/hooks/useWorktreeSuggestions.ts
/* __next_internal_client_entry_do_not_use__ useWorktreeSuggestions auto */ 



const DEBOUNCE_MS = 150;
// Above the server's own ListWorktrees request-scoped timeout so a genuine
// server-side timeout error surfaces before this client-side one fires.
const REQUEST_TIMEOUT_MS = 5000;
/**
 * Fetches git worktrees for the given repository path.
 * Used to populate the "Use Existing Worktree" dropdown in the Omnibar.
 *
 * Mirrors usePathCompletions' debounce + AbortController + timeout pattern so a
 * hung backend request surfaces a bounded error instead of loading forever.
 */ function useWorktreeSuggestions(repoPath) {
    let options = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : {};
    const { enabled = true } = options;
    const [worktrees, setWorktrees] = (0,react.useState)([]);
    const [isLoading, setIsLoading] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    const generationRef = (0,react.useRef)(0);
    (0,react.useEffect)(()=>{
        if (!enabled || !repoPath.trim()) {
            setWorktrees([]);
            setIsLoading(false);
            setError(null);
            return;
        }
        const generation = ++generationRef.current;
        setIsLoading(true);
        setError(null);
        const abortController = new AbortController();
        const debounceTimer = setTimeout(()=>{
            const timeoutTimer = setTimeout(()=>abortController.abort(), REQUEST_TIMEOUT_MS);
            const client = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
            client.listWorktrees({
                repoPath
            }, {
                signal: abortController.signal
            }).then((response)=>{
                if (generation !== generationRef.current) return;
                setWorktrees(response.worktrees || []);
                setIsLoading(false);
            }).catch((err)=>{
                if (generation !== generationRef.current) return;
                if (abortController.signal.aborted) {
                    setError("Fetching worktrees timed out");
                } else {
                    setError(err instanceof Error ? err.message : "Failed to fetch worktrees");
                }
                setWorktrees([]);
                setIsLoading(false);
            }).finally(()=>clearTimeout(timeoutTimer));
        }, DEBOUNCE_MS);
        return ()=>{
            clearTimeout(debounceTimer);
            abortController.abort();
        };
    }, [
        repoPath,
        enabled
    ]);
    return {
        worktrees,
        isLoading,
        error
    };
}

;// ./src/lib/hooks/useDestinationPathPreview.ts
/* __next_internal_client_entry_do_not_use__ useDestinationPathPreview auto */ 



const useDestinationPathPreview_DEBOUNCE_MS = 300;
const useDestinationPathPreview_REQUEST_TIMEOUT_MS = 3000;
/**
 * Fetches a live preview of where a session's checkout/worktree would be created,
 * without performing any git or filesystem mutation. Mirrors useWorktreeSuggestions'
 * debounce + AbortController + generation-counter pattern so stale responses (from a
 * fast-typing user) never clobber a newer one.
 */ function useDestinationPathPreview(params) {
    let options = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : {};
    const { enabled = true } = options;
    const [path, setPath] = (0,react.useState)(null);
    const [isExact, setIsExact] = (0,react.useState)(false);
    const [isLoading, setIsLoading] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    const generationRef = (0,react.useRef)(0);
    const mode = params === null || params === void 0 ? void 0 : params.mode;
    var _params_input;
    const input = (_params_input = params === null || params === void 0 ? void 0 : params.input) !== null && _params_input !== void 0 ? _params_input : "";
    var _params_repoPath;
    const repoPath = (_params_repoPath = params === null || params === void 0 ? void 0 : params.repoPath) !== null && _params_repoPath !== void 0 ? _params_repoPath : "";
    var _params_sessionName;
    const sessionName = (_params_sessionName = params === null || params === void 0 ? void 0 : params.sessionName) !== null && _params_sessionName !== void 0 ? _params_sessionName : "";
    (0,react.useEffect)(()=>{
        const paramsMissing = !mode || mode === "github_url" && !input.trim() || mode === "new_worktree" && (!repoPath.trim() || !sessionName.trim());
        if (!enabled || paramsMissing) {
            setPath(null);
            setIsExact(false);
            setIsLoading(false);
            setError(null);
            return;
        }
        const generation = ++generationRef.current;
        setIsLoading(true);
        setError(null);
        const abortController = new AbortController();
        const debounceTimer = setTimeout(()=>{
            const timeoutTimer = setTimeout(()=>abortController.abort(), useDestinationPathPreview_REQUEST_TIMEOUT_MS);
            const client = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
            client.previewDestinationPath({
                input,
                mode,
                repoPath,
                sessionName
            }, {
                signal: abortController.signal
            }).then((response)=>{
                if (generation !== generationRef.current) return;
                if (response.unresolvedReason) {
                    setPath(null);
                    setIsExact(false);
                } else {
                    setPath(response.path);
                    setIsExact(response.isExact);
                }
                setIsLoading(false);
            }).catch((err)=>{
                if (generation !== generationRef.current) return;
                if (abortController.signal.aborted) {
                    setError("Destination path preview timed out");
                } else {
                    setError(err instanceof Error ? err.message : "Failed to preview destination path");
                }
                setPath(null);
                setIsExact(false);
                setIsLoading(false);
            }).finally(()=>clearTimeout(timeoutTimer));
        }, useDestinationPathPreview_DEBOUNCE_MS);
        return ()=>{
            clearTimeout(debounceTimer);
            abortController.abort();
        };
    }, [
        mode,
        input,
        repoPath,
        sessionName,
        enabled
    ]);
    return {
        path,
        isExact,
        isLoading,
        error
    };
}

// EXTERNAL MODULE: ./src/gen/session/v1/types_pb.ts
var types_pb = __webpack_require__(81432);
// EXTERNAL MODULE: ./src/lib/store/index.ts
var store = __webpack_require__(93602);
// EXTERNAL MODULE: ./src/lib/store/sessionsSlice.ts
var sessionsSlice = __webpack_require__(60795);
;// ./src/lib/hooks/useSessionSearch.ts
/* __next_internal_client_entry_do_not_use__ useSessionSearch auto */ 




// Fuse.js config: multi-field weighted search
// title:0.5 > branch:0.3 > path:0.15 > tags:0.05
// threshold:0.4 — allow moderate fuzziness; tighten if too many false positives
// minMatchCharLength:1 — single character queries return results
const FUSE_OPTIONS = {
    keys: [
        {
            name: "title",
            weight: 0.5
        },
        {
            name: "branch",
            weight: 0.3
        },
        {
            name: "path",
            weight: 0.15
        },
        {
            name: "tags",
            weight: 0.05
        }
    ],
    includeScore: true,
    includeMatches: true,
    threshold: 0.4,
    minMatchCharLength: 1,
    ignoreLocation: true
};
/**
 * Client-side fuzzy session search using Fuse.js.
 * Returns ranked results for display in the Omnibar session section.
 * Empty query returns empty array (empty state shows recents instead).
 *
 * Filters out UNSPECIFIED sessions (status=0). All other statuses
 * (RUNNING, READY, LOADING, PAUSED, NEEDS_APPROVAL) are included.
 */ function useSessionSearch(query) {
    const sessions = (0,store/* useAppSelector */.GV)(sessionsSlice/* selectAllSessions */.rt);
    // Filter to active sessions only (exclude UNSPECIFIED)
    const activeSessions = (0,react.useMemo)(()=>sessions.filter((s)=>s.status !== types_pb/* SessionStatus */.Ph.UNSPECIFIED), [
        sessions
    ]);
    // Rebuild Fuse index when active session list changes
    const fuse = (0,react.useMemo)(()=>new dist_fuse/* default */.A(activeSessions, FUSE_OPTIONS), [
        activeSessions
    ]);
    return (0,react.useMemo)(()=>{
        const trimmed = query.trim();
        if (!trimmed) return [];
        const results = fuse.search(trimmed, {
            limit: 8
        });
        return results.map((r)=>{
            var _r_matches;
            var _r_score, _r_matches_map;
            return {
                session: r.item,
                score: (_r_score = r.score) !== null && _r_score !== void 0 ? _r_score : 1.0,
                matchedFields: (_r_matches_map = (_r_matches = r.matches) === null || _r_matches === void 0 ? void 0 : _r_matches.map((m)=>{
                    var _m_key;
                    return (_m_key = m.key) !== null && _m_key !== void 0 ? _m_key : "";
                })) !== null && _r_matches_map !== void 0 ? _r_matches_map : []
            };
        });
    }, [
        query,
        fuse
    ]);
}

// EXTERNAL MODULE: ./src/components/ui/PathCompletionDropdown.tsx + 1 modules
var PathCompletionDropdown = __webpack_require__(81551);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fui%2FAtCommandDropdown.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA62UTW7CMBBG95xilmVhlB9Ki3uYysSTZFTHtmynBKrevYoDBUIjStVlxiPP-14yWbym6zLZrqoEPmYAinxgPuwUctBG48sMoBGuIs0h6R-skJJ0xWFpu6HSiI7VSFUdOGTLxHZ90byjK5XZsh0H0QbT1zbGSXRsY0IwDYfUduCNIgnvwj0w9pqucZ-5Pc5jsyjeKmdaLfno_Gn-MvucLY7caeSW5K0SOw6lwgggFFWaUcDGcyhQB3R9uRJ2fF-2n18EuzodVfI09het88ZxsIaOl7ceHfOosAhHe-egvO6tRNzpdOkoXnZnfx77S6MD87THcXu-ifC9JeZrR_rt8F63JEPNIV1kjw6bvhKwCyxqPAk8G7Q8DSpFQ2p3Za4dRt1AKYzqNY5SrScwG9LsgPocOc-IHn8V_cd5yfc4Dun598uhJilRfws5HaBSZD35aK-mgMxbUcS92TphL9BWN9DENFo2YeI_CIflPfjMnov6AvopQk_sRZ5M7MWf_A8h7_hBfH4BBOM7irgEAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FAtCommandDropdown_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA62UTW7CMBBG95xilmVhlB9Ki3uYysSTZFTHtmynBKrevYoDBUIjStVlxiPP_14yWbym6zLZrqoEPmYAinxgPuwUctBG48sMoBGuIs0h6R_skJJ0xWFpu6HSiI7VSFUdOGTLxHZ90byjK5XZsh0H0QbT1zbGSXRsY0IwDYfUduCNIgnvwj0w9pqucZ_5Pc5jsyjeKmdaLfno_Gn_MvucLY7caeSW5K0SOw6lwgggFFWaUcDGcyhQB3R9uRJ2fF_2n18EuzodVfI09het88ZxsIaOl7ceHfOosAhHe_egvO6tRNzpdOkoXnZnfx77S6MD87THcXu_ifC9JeZrR_rt8F63JEPNIV1kjw6bvhKwCyxqPAk8G7Q8DSpFQ2p3Za4dRt1AKYzqNY5SrScwG9LsgPocOc_IHn8V_cd5yfc4Dun598uhJilRfws5HaBSZD35aK_mgMxbUcS92TphL9BWN9DENFo2YeI_CIflPfjMnov6AvopQk_sRZ5M7MWf_A8h7_hBfH4BBOM7irgEAAA_22_7D = __webpack_require__(46469);
;// ./src/components/ui/AtCommandDropdown.css.ts


var description = '_19f0w6g6';
var dropdown = '_19f0w6g0';
var empty = '_19f0w6g7';
var icon = '_19f0w6g3';
var item = '_19f0w6g1';
var itemSelected = '_19f0w6g2';
var AtCommandDropdown_css_name = '_19f0w6g5';
var slug = '_19f0w6g4';
;// ./src/components/ui/AtCommandDropdown.tsx
/* __next_internal_client_entry_do_not_use__ AtCommandDropdown auto */ 


/**
 * Dropdown that lists matching workflows when the user types @slug in an input.
 * Follows the same controlled-index pattern as PathCompletionDropdown.
 *
 * Usage:
 *   const { isAtCommand, suggestions, complete } = useAtCommandSuggestions(value, workflows);
 *   <AtCommandDropdown suggestions={suggestions} selectedIndex={idx} onSelect={wf => setValue(complete(wf))} />
 */ function AtCommandDropdown(param) {
    let { id = "at-command-listbox", suggestions, selectedIndex, onSelect } = param;
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
            children: "No matching workflows"
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
        id: id,
        className: dropdown,
        role: "listbox",
        "aria-label": "Workflow suggestions",
        children: suggestions.map((wf, i)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
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
                    onSelect(wf);
                },
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: icon,
                        "aria-hidden": "true",
                        children: "⚡"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: slug,
                        children: [
                            "@",
                            wf.slug
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: AtCommandDropdown_css_name,
                        children: wf.name
                    }),
                    wf.description && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: description,
                        children: wf.description
                    })
                ]
            }, wf.slug))
    });
}

;// ./src/lib/hooks/useAtCommandSuggestions.ts

// Matches "@slug" with no space — the slug is still being completed.
const AT_PREFIX = /^@([a-zA-Z0-9_-]*)$/;
/**
 * Given the current input value and a list of available workflows, returns
 * filtered @slug suggestions and a helper to produce the completed input string.
 *
 * Active when the input matches /^@slug-chars-only$/ (no space yet).
 * Calling complete(wf) returns "@slug " (with trailing space) ready to set as input.
 */ function useAtCommandSuggestions(value, workflows) {
    const match = value.match(AT_PREFIX);
    const isAtCommand = match !== null;
    const query = isAtCommand ? match[1].toLowerCase() : "";
    const suggestions = (0,react.useMemo)(()=>{
        if (!isAtCommand) return [];
        if (!query) return workflows;
        return workflows.filter((w)=>w.slug.toLowerCase().startsWith(query));
    }, [
        isAtCommand,
        query,
        workflows
    ]);
    function complete(wf) {
        return "@".concat(wf.slug, " ");
    }
    return {
        isAtCommand,
        query,
        suggestions,
        complete
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FOmnibarSessionResult.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61U0Y6bMBB8z1f4pVIj1RFwuVzO-ZiTgzewjWNbtjlIqvv3CgMBEhpS6Z6QvcN6dnZ3Vh9vwhSAEfmzIESgM5KfGTlIqHYLEr5UoIXUo1aMWF3W11xipih6OLkGS53n1tehjBtG4siE3w0XAlXGyNZUJE6ay7SwTltGjEblwdZXe20FWGq5wMIxsmmAJQqf18miH_VRovPU-bMERpRWEH7k6TGzulCCEW-5coZbUIFIOGJDu4eRaBU7AtzBbvG1WLXVx6H6YbJPbn9S-hG_wyWxl-1yiE6mtRqJkkJXW6sB9dow8mJ6WV1uUR0ZiYa5X0LutvJtg84Bs9xfjzdivTbq3KYcEEQlUQHdS50eh2-tZ6ouRlW_zqCrEXozgz6N0G8z6GSE3j43q6mWxUnVkRMq2koadUhG4mHO9_-a_9-F83g401QrD8oz4gxPge7BlwDqbhj23EHdgut-tJ0c8-rJ8EDmoJWnZdv7TRSYp1rWqzNWJ1ruOrjDCzASr5v8-hPsQeqSkRyFaIh5qDztAyAlGocu7FuOHmgopd6w0nLzpHr7QHiSXHJHLpnegW-jy6uObrwOPtTzTP_Nc33HM_5WEbnNUDUmEI9Jiabbd3JczTMxVWeJEzo-sMDGKcKDxGmJ4qZmWE74STs7D5o5YeDa8BT9uWU-tN420vvur1k3jlhea0vGAl2fiMfO_QxKtBnnXf7Bin39BVoEHWQsBwAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FOmnibarSessionResult_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61U0Y6bMBB8z1f4pVIj1RFwuVzO_ZiTgzewjWNbtjlIqvv3CgMBEhpS6Z6QvcN6dnZ3Vh9vwhSAEfmzIESgM5KfGTlIqHYLEr5UoIXUo1aMWF3W11xipih6OLkGS53n1tehjBtG4siE3w0XAlXGyNZUJE6ay7SwTltGjEblwdZXe20FWGq5wMIxsmmAJQqf18miH_VRovPU_bMERpRWEH7k6TGzulCCEW_5coZbUIFIOGJDu4eRaBU7AtzBbvG1WLXVx6H6YbJPbn9S_hG_wyWxl_1yiE6mtRqJkkJXW6sB9dow8mJ6WV1uUR0ZiYa5X0LutvJtg84Bs9xfjzdivTbq3KYcEEQlUQHdS50eh2_tZ6ouRlW_zqCrEXozgz6N0G8z6GSE3j43q6mWxUnVkRMq2koadUhG4mHO9_a_9_F83g401QrD8oz4gxPge7BlwDqbhj23EHdgut_tJ0c8_rJ8EDmoJWnZdv7TRSYp1rWqzNWJ1ruOrjDCzASr5v8_hPsQeqSkRyFaIh5qDztAyAlGocu7FuOHmgopd6w0nLzpHr7QHiSXHJHLpnegW_jy6uObrwOPtTzTP_Nc33HM_5WEbnNUDUmEI9Jiabbd3JczTMxVWeJEzo_sMDGKcKDxGmJ4qZmWE74STs7D5o5YeDa8BT9uWU_tN420vvur1k3jlhea0vGAl2fiMfO_QxKtBnnXf7Bin39BVoEHWQsBwAA_22_7D = __webpack_require__(81609);
;// ./src/components/sessions/OmnibarSessionResult.css.ts


var branch = '_7dpueib';
var cloneButton = '_7dpueid';
var content = '_7dpuei8';
var dot = '_7dpuei3';
var dotVariants = {running:'_7dpuei4',paused:'_7dpuei5',active:'_7dpuei6','default':'_7dpuei7'};
var dotWrapper = '_7dpuei2';
var path = '_7dpueic';
var row = '_7dpuei0';
var rowHighlighted = '_7dpuei1';
var title = '_7dpueia';
var titleRow = '_7dpuei9';
;// ./src/components/sessions/OmnibarSessionResult.tsx
/* __next_internal_client_entry_do_not_use__ OmnibarSessionResult auto */ 


function statusDotVariant(status) {
    switch(status){
        case types_pb/* SessionStatus */.Ph.RUNNING:
            return "running";
        case types_pb/* SessionStatus */.Ph.PAUSED:
            return "paused";
        case types_pb/* SessionStatus */.Ph.LOADING:
        case types_pb/* SessionStatus */.Ph.NEEDS_APPROVAL:
            return "active";
        default:
            return "default";
    }
}
function statusLabel(status) {
    switch(status){
        case types_pb/* SessionStatus */.Ph.RUNNING:
            return "Running";
        case types_pb/* SessionStatus */.Ph.PAUSED:
            return "Paused";
        case types_pb/* SessionStatus */.Ph.LOADING:
            return "Loading";
        case types_pb/* SessionStatus */.Ph.NEEDS_APPROVAL:
            return "Needs approval";
        case types_pb/* SessionStatus */.Ph.READY:
            return "Ready";
        default:
            return "Unknown";
    }
}
/**
 * Returns the last 2 path segments, e.g.:
 *   /Users/tyler/projects/auth  →  projects/auth
 *   /Users/tyler/auth           →  tyler/auth
 *   auth                        →  auth
 */ function shortPath(fullPath) {
    const parts = fullPath.split("/").filter(Boolean);
    if (parts.length <= 2) return parts.join("/");
    return parts.slice(-2).join("/");
}
function OmnibarSessionResult(param) {
    let { result, isHighlighted, id, onClick, onClone, onOpenInNewPane } = param;
    const { session } = result;
    const dotVariant = statusDotVariant(session.status);
    const rowClassName = [
        row,
        isHighlighted ? rowHighlighted : ""
    ].filter(Boolean).join(" ");
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
        id: id,
        className: rowClassName,
        role: "option",
        "aria-selected": isHighlighted,
        onMouseDown: (e)=>{
            e.preventDefault();
            if (e.altKey && onOpenInNewPane) {
                onOpenInNewPane(session);
            } else {
                onClick(session);
            }
        },
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: dotWrapper,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    className: "".concat(dot, " ").concat(dotVariants[dotVariant]),
                    "aria-label": statusLabel(session.status)
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                className: content,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: titleRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: title,
                                children: session.title
                            }),
                            session.branch && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: branch,
                                children: session.branch
                            })
                        ]
                    }),
                    session.existingDir && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: path,
                        children: shortPath(session.existingDir)
                    })
                ]
            }),
            onOpenInNewPane && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                className: cloneButton,
                onMouseDown: (e)=>{
                    e.preventDefault();
                    e.stopPropagation();
                    onOpenInNewPane(session);
                },
                "aria-label": "Open session ".concat(session.title, " in new pane"),
                tabIndex: isHighlighted ? 0 : -1,
                title: "Open in new pane (or Alt+click)",
                type: "button",
                children: "⊞"
            }),
            onClone && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                className: cloneButton,
                onMouseDown: (e)=>{
                    e.preventDefault();
                    e.stopPropagation();
                    onClone(session);
                },
                "aria-label": "Clone session ".concat(session.title),
                tabIndex: isHighlighted ? 0 : -1,
                title: "Clone this session",
                type: "button",
                children: "⊕"
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FOmnibarRepoResult.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA6VTy07DMBC89yv2CAdXcYFC3Y9BbrxNVnVsa-2QUsS_ozglLX2I13UyO7Mz60xbN2-2TQFvEwBDMVj9qmBtcbucAGhLlROUsIkKSnQJuYcrHRTIImRO0MaQqxQ8hS3I2QCWLUfPCoKnz6GVZ4MsWBtqo4L5QLQUk4jp1aIC5x32WBuRRUSLZTqAibWLlMg7BStdbir2rTNQTGXM6iOkBmrQjC4tJ--T6RBR5ojHvBfNN0I8ywXuZryTj7dH7Flm9z2IWDO5jYIiB_O2z_V1dHa7_HF7o8Pd6KBA9vSGnOjIpHpvdaaXtzHEWA41lN62jRsPkqsf5e9_cNGVjmhp6PcgAeBfkNfWdwpqMgazRVdTQhGDLvOlOtbhyO1hCONdEpF2qEDe7R_C9b4uuSTcJnH4gNZSiBSXp7e4UNi4y_wPu5weehR7vCaWoQ6pqpOCeXH9cRT_Dnu8z9PZPvKbcOPo4tejl36ARnNFTlhcJwW6Tf764_gA2fPZAFwEAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FOmnibarRepoResult_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA6VTy07DMBC89yv2CAdXcYFC3Y9BbrxNVnVsa_2QUsS_ozglLX2I13UyO7Mz60xbN2_2TQFvEwBDMVj9qmBtcbucAGhLlROUsIkKSnQJuYcrHRTIImRO0MaQqxQ8hS3I2QCWLUfPCoKnz6GVZ4MsWBtqo4L5QLQUk4jp1aIC5x32WBuRRUSLZTqAibWLlMg7BStdbir2rTNQTGXM6iOkBmrQjC4tJ_T6RBR5ojHvBfNN0I8ywXuZryTj7dH7Flm9z2IWDO5jYIiB_O2z_V1dHa7_HF7o8Pd6KBA9vSGnOjIpHpvdaaXtzHEWA41lN62jRsPkqsf5e9_cNGVjmhp6PcgAeBfkNfWdwpqMgazRVdTQhGDLvOlOtbhyO1hCONdEpF2qEDe7R_C9b4uuSTcJnH4gNZSiBSXp7e4UNi4y_wPu5weehR7vCaWoQ6pqpOCeXH9cRT_Dnu8z9PZPvKbcOPo4tejl36ARnNFTlhcJwW6Tf764_gA2fPZAFwEAAA_22_7D = __webpack_require__(91262);
;// ./src/components/sessions/OmnibarRepoResult.css.ts


var OmnibarRepoResult_css_content = 'un6mxm3';
var folderIcon = 'un6mxm2';
var OmnibarRepoResult_css_parentPath = 'un6mxm5';
var pathLine = 'un6mxm4';
var relativeTime = 'un6mxm9';
var OmnibarRepoResult_css_repoName = 'un6mxm7';
var OmnibarRepoResult_css_row = 'un6mxm0';
var OmnibarRepoResult_css_rowHighlighted = 'un6mxm1';
var separator = 'un6mxm6';
var OmnibarRepoResult_css_sessionCount = 'un6mxm8';
;// ./src/components/sessions/OmnibarRepoResult.tsx
/* __next_internal_client_entry_do_not_use__ OmnibarRepoResult auto */ 

function OmnibarRepoResult_relativeTime(timestamp) {
    const diffMs = Date.now() - timestamp;
    const diffMins = Math.floor(diffMs / 60000);
    if (diffMins < 1) return "just now";
    if (diffMins < 60) return "".concat(diffMins, "m ago");
    const diffHours = Math.floor(diffMins / 60);
    if (diffHours < 24) return "".concat(diffHours, "h ago");
    const diffDays = Math.floor(diffHours / 24);
    if (diffDays < 7) return "".concat(diffDays, "d ago");
    return "".concat(Math.floor(diffDays / 7), "w ago");
}
function OmnibarRepoResult(param) {
    let { entry, sessionCount, isHighlighted, id, onClick } = param;
    const parts = entry.path.split("/").filter(Boolean);
    var _parts_;
    const repoName = (_parts_ = parts[parts.length - 1]) !== null && _parts_ !== void 0 ? _parts_ : entry.path;
    const parentPath = parts.slice(-3, -1).join("/"); // up to 2 parent segments before the repo name
    const rowClass = [
        OmnibarRepoResult_css_row,
        isHighlighted ? OmnibarRepoResult_css_rowHighlighted : ""
    ].filter(Boolean).join(" ");
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
        role: "option",
        "aria-selected": isHighlighted,
        id: id,
        className: rowClass,
        onMouseDown: (e)=>{
            e.preventDefault(); // prevent focus loss on click
            onClick(entry.path);
        },
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: folderIcon,
                "aria-hidden": "true",
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("svg", {
                    width: "16",
                    height: "16",
                    viewBox: "0 0 16 16",
                    fill: "none",
                    xmlns: "http://www.w3.org/2000/svg",
                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)("path", {
                        d: "M1.5 3.5C1.5 2.948 1.948 2.5 2.5 2.5H6.086C6.35 2.5 6.602 2.605 6.789 2.793L7.5 3.5H13.5C14.052 3.5 14.5 3.948 14.5 4.5V12.5C14.5 13.052 14.052 13.5 13.5 13.5H2.5C1.948 13.5 1.5 13.052 1.5 12.5V3.5Z",
                        stroke: "currentColor",
                        strokeWidth: "1.25",
                        strokeLinejoin: "round"
                    })
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                className: OmnibarRepoResult_css_content,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: pathLine,
                        children: [
                            parentPath && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: OmnibarRepoResult_css_parentPath,
                                        children: parentPath
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: separator,
                                        children: "/"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: OmnibarRepoResult_css_repoName,
                                children: repoName
                            })
                        ]
                    }),
                    sessionCount !== undefined && sessionCount > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: OmnibarRepoResult_css_sessionCount,
                        children: [
                            sessionCount,
                            " ",
                            sessionCount === 1 ? "session" : "sessions"
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: relativeTime,
                children: OmnibarRepoResult_relativeTime(entry.lastUsed)
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FOmnibarResultList.css.ts.vanilla.css%22%2C%22source%22%3A%22LmVvenF0ODAgewogIGxpc3Qtc3R5bGU6IG5vbmU7CiAgbWFyZ2luOiAwOwogIHBhZGRpbmc6IDRweCAwOwogIHdpZHRoOiAxMDAlOwogIG1heC1oZWlnaHQ6IDMyMHB4OwogIG92ZXJmbG93LXk6IGF1dG87Cn0KLmVvenF0ODEgewogIHBhZGRpbmc6IDRweCAxMnB4OwogIGZvbnQtc2l6ZTogMTFweDsKICBmb250LXdlaWdodDogNjAwOwogIGxldHRlci1zcGFjaW5nOiAwLjA4ZW07CiAgdGV4dC10cmFuc2Zvcm06IHVwcGVyY2FzZTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKICB1c2VyLXNlbGVjdDogbm9uZTsKICBsaXN0LXN0eWxlOiBub25lOwp9Ci5lb3pxdDgyIHsKICBoZWlnaHQ6IDFweDsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyemUpOwogIG1hcmdpbjogNHB4IDA7CiAgbGlzdC1zdHlsZTogbm9uZTsKfQouZW96cXQ4MyB7CiAgZGlzcGxheTogZmxleDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGdhcDogOHB4OwogIHBhZGRpbmc6IDhweCAxMnB4OwogIGN1cnNvcjogcG9pbnRlcjsKICBsaXN0LXN0eWxlOiBub25lOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejIpOwogIGZvbnQtc3R5bGU6IGl0YWxpYzsKICBmb250LXNpemU6IDEzcHg7CiAgdHJhbnNpdGlvbjogYmFja2dyb3VuZCAwLjFzIGVhc2U7CiAgYm9yZGVyLXJhZGl1czogNnB4Owp9Ci5lb3pxdDg0IHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejgpOwp9Ci5lb3pxdDg1IHsKICBmb250LXdlaWdodDogNzAwOwogIGZvbnQtc3R5bGU6IG5vcm1hbDsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxKTsKICBmb250LXNpemU6IDE0cHg7Cn0%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FOmnibarResultList_css_ts_vanilla_css_22_2C_22source_22_3A_22LmVvenF0ODAgewogIGxpc3Qtc3R5bGU6IG5vbmU7CiAgbWFyZ2luOiAwOwogIHBhZGRpbmc6IDRweCAwOwogIHdpZHRoOiAxMDAlOwogIG1heC1oZWlnaHQ6IDMyMHB4OwogIG92ZXJmbG93LXk6IGF1dG87Cn0KLmVvenF0ODEgewogIHBhZGRpbmc6IDRweCAxMnB4OwogIGZvbnQtc2l6ZTogMTFweDsKICBmb250LXdlaWdodDogNjAwOwogIGxldHRlci1zcGFjaW5nOiAwLjA4ZW07CiAgdGV4dC10cmFuc2Zvcm06IHVwcGVyY2FzZTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKICB1c2VyLXNlbGVjdDogbm9uZTsKICBsaXN0LXN0eWxlOiBub25lOwp9Ci5lb3pxdDgyIHsKICBoZWlnaHQ6IDFweDsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyemUpOwogIG1hcmdpbjogNHB4IDA7CiAgbGlzdC1zdHlsZTogbm9uZTsKfQouZW96cXQ4MyB7CiAgZGlzcGxheTogZmxleDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGdhcDogOHB4OwogIHBhZGRpbmc6IDhweCAxMnB4OwogIGN1cnNvcjogcG9pbnRlcjsKICBsaXN0LXN0eWxlOiBub25lOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejIpOwogIGZvbnQtc3R5bGU6IGl0YWxpYzsKICBmb250LXNpemU6IDEzcHg7CiAgdHJhbnNpdGlvbjogYmFja2dyb3VuZCAwLjFzIGVhc2U7CiAgYm9yZGVyLXJhZGl1czogNnB4Owp9Ci5lb3pxdDg0IHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejgpOwp9Ci5lb3pxdDg1IHsKICBmb250LXdlaWdodDogNzAwOwogIGZvbnQtc3R5bGU6IG5vcm1hbDsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxKTsKICBmb250LXNpemU6IDE0cHg7Cn0_22_7D = __webpack_require__(64354);
;// ./src/components/sessions/OmnibarResultList.css.ts


var createNewHighlighted = 'eozqt84';
var createNewIcon = 'eozqt85';
var createNewItem = 'eozqt83';
var list = 'eozqt80';
var sectionHeader = 'eozqt81';
var OmnibarResultList_css_separator = 'eozqt82';
;// ./src/components/sessions/OmnibarResultList.tsx
/* __next_internal_client_entry_do_not_use__ getResultListItemCount,getHighlightedItemId,OmnibarResultList auto */ 




/**
 * Returns the total navigable item count for OmnibarResultList.
 * Sessions + repos + 1 for "+ New Session" + 1 for "Create Backlog Item (AI)".
 */ function getResultListItemCount(sessionCount, repoCount) {
    let hasBacklogOption = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : false;
    return sessionCount + repoCount + (hasBacklogOption ? 2 : 1);
}
/**
 * Returns the id of the currently highlighted item given the props used by
 * OmnibarResultList. The parent can use this to set aria-activedescendant on
 * the input element.
 */ function getHighlightedItemId(id, sessionResults, repoEntries, highlightedIndex) {
    if (highlightedIndex < 0) return undefined;
    if (highlightedIndex < sessionResults.length) {
        const session = sessionResults[highlightedIndex].session;
        return "".concat(id, "-session-").concat(session.id);
    }
    const repoIndex = highlightedIndex - sessionResults.length;
    if (repoIndex < repoEntries.length) {
        const entry = repoEntries[repoIndex];
        return "".concat(id, "-repo-").concat(encodeURIComponent(entry.path));
    }
    const actionIndex = repoIndex - repoEntries.length;
    if (actionIndex === 0) {
        return "".concat(id, "-create-new");
    }
    return "".concat(id, "-create-backlog");
}
function OmnibarResultList(param) {
    let { sessionResults, repoEntries, sessionCounts, onSessionSelect, onSessionOpenInNewPane, onRepoSelect, onCreateNew, onCreateBacklog, onCloneSession, highlightedIndex, id } = param;
    const createNewIndex = sessionResults.length + repoEntries.length;
    const createBacklogIndex = createNewIndex + 1;
    const isCreateNewHighlighted = highlightedIndex === createNewIndex;
    const isCreateBacklogHighlighted = highlightedIndex === createBacklogIndex;
    const listRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        if (highlightedIndex < 0 || !listRef.current) return;
        const highlightedId = getHighlightedItemId(id, sessionResults, repoEntries, highlightedIndex);
        if (!highlightedId) return;
        const el = listRef.current.querySelector("#".concat(CSS.escape(highlightedId)));
        if (el && typeof el.scrollIntoView === "function") {
            el.scrollIntoView({
                block: "nearest"
            });
        }
    }, [
        highlightedIndex,
        id,
        sessionResults,
        repoEntries
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("ul", {
        ref: listRef,
        role: "listbox",
        id: id,
        "aria-label": "Session search results",
        className: list,
        children: [
            sessionResults.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("li", {
                        role: "presentation",
                        "aria-hidden": "true",
                        className: sectionHeader,
                        children: "SESSIONS"
                    }),
                    sessionResults.map((result, i)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(OmnibarSessionResult, {
                            result: result,
                            isHighlighted: highlightedIndex === i,
                            id: "".concat(id, "-session-").concat(result.session.id),
                            onClick: onSessionSelect,
                            onOpenInNewPane: onSessionOpenInNewPane,
                            onClone: onCloneSession
                        }, result.session.id))
                ]
            }),
            repoEntries.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("li", {
                        role: "presentation",
                        "aria-hidden": "true",
                        className: sectionHeader,
                        children: "REPOS"
                    }),
                    repoEntries.map((entry, i)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(OmnibarRepoResult, {
                            entry: entry,
                            sessionCount: sessionCounts === null || sessionCounts === void 0 ? void 0 : sessionCounts[entry.path],
                            isHighlighted: highlightedIndex === sessionResults.length + i,
                            id: "".concat(id, "-repo-").concat(encodeURIComponent(entry.path)),
                            onClick: onRepoSelect
                        }, entry.path))
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("li", {
                role: "separator",
                "aria-hidden": "true",
                className: OmnibarResultList_css_separator
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                role: "option",
                id: "".concat(id, "-create-new"),
                "aria-selected": isCreateNewHighlighted,
                className: [
                    createNewItem,
                    isCreateNewHighlighted ? createNewHighlighted : ""
                ].filter(Boolean).join(" "),
                onMouseDown: (e)=>{
                    e.preventDefault();
                    onCreateNew();
                },
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: createNewIcon,
                        "aria-hidden": "true",
                        children: "+"
                    }),
                    "New Session"
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                role: "option",
                id: "".concat(id, "-create-backlog"),
                "aria-selected": isCreateBacklogHighlighted,
                className: [
                    createNewItem,
                    isCreateBacklogHighlighted ? createNewHighlighted : ""
                ].filter(Boolean).join(" "),
                onMouseDown: (e)=>{
                    e.preventDefault();
                    onCreateBacklog === null || onCreateBacklog === void 0 ? void 0 : onCreateBacklog();
                },
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: createNewIcon,
                        "aria-hidden": "true",
                        children: "\uD83D\uDCDD"
                    }),
                    "Create Backlog Item (AI)"
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FOmnibarModeBadge.css.ts.vanilla.css%22%2C%22source%22%3A%22LmZkdnVzZDAgewogIGRpc3BsYXk6IGZsZXg7CiAgYm9yZGVyLXJhZGl1czogdmFyKC0tXzE5ZXoycnozNik7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnplKTsKICBvdmVyZmxvdzogaGlkZGVuOwogIGZsZXgtc2hyaW5rOiAwOwp9Ci5mZHZ1c2QxIHsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejJ5KSB2YXIoLS1fMTllejJyejJ6KTsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2EpOwogIGZvbnQtd2VpZ2h0OiA1MDA7CiAgYmFja2dyb3VuZDogdHJhbnNwYXJlbnQ7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7CiAgY3Vyc29yOiBwb2ludGVyOwogIGJvcmRlcjogbm9uZTsKICBsaW5lLWhlaWdodDogMS41Owp9Ci5mZHZ1c2QxOmhvdmVyIHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejgpOwp9Ci5mZHZ1c2QyIHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyem0pOwogIGNvbG9yOiB2YXIoLS1fMTllejJyenEpOwogIGN1cnNvcjogZGVmYXVsdDsKfQouZmR2dXNkMjpob3ZlciB7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnptKTsKfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FOmnibarModeBadge_css_ts_vanilla_css_22_2C_22source_22_3A_22LmZkdnVzZDAgewogIGRpc3BsYXk6IGZsZXg7CiAgYm9yZGVyLXJhZGl1czogdmFyKC0tXzE5ZXoycnozNik7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnplKTsKICBvdmVyZmxvdzogaGlkZGVuOwogIGZsZXgtc2hyaW5rOiAwOwp9Ci5mZHZ1c2QxIHsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejJ5KSB2YXIoLS1fMTllejJyejJ6KTsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2EpOwogIGZvbnQtd2VpZ2h0OiA1MDA7CiAgYmFja2dyb3VuZDogdHJhbnNwYXJlbnQ7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7CiAgY3Vyc29yOiBwb2ludGVyOwogIGJvcmRlcjogbm9uZTsKICBsaW5lLWhlaWdodDogMS41Owp9Ci5mZHZ1c2QxOmhvdmVyIHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejgpOwp9Ci5mZHZ1c2QyIHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyem0pOwogIGNvbG9yOiB2YXIoLS1fMTllejJyenEpOwogIGN1cnNvcjogZGVmYXVsdDsKfQouZmR2dXNkMjpob3ZlciB7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnptKTsKfQ_22_7D = __webpack_require__(75294);
;// ./src/components/sessions/OmnibarModeBadge.css.ts

var badgeActive = 'fdvusd2';
var badgeButton = 'fdvusd1';
var badgeContainer = 'fdvusd0';
;// ./src/components/sessions/OmnibarModeBadge.tsx
/* __next_internal_client_entry_do_not_use__ OmnibarModeBadge auto */ 

function OmnibarModeBadge(param) {
    let { mode, onToggle } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: badgeContainer,
        role: "group",
        "aria-label": "Omnibar mode",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                className: [
                    badgeButton,
                    mode === "discovery" ? badgeActive : ""
                ].filter(Boolean).join(" "),
                onClick: mode !== "discovery" ? onToggle : undefined,
                "aria-pressed": mode === "discovery",
                title: "Jump to existing session (Cmd+K)",
                type: "button",
                children: "Jump"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                className: [
                    badgeButton,
                    mode === "creation" ? badgeActive : ""
                ].filter(Boolean).join(" "),
                onClick: mode !== "creation" ? onToggle : undefined,
                "aria-pressed": mode === "creation",
                title: "Create new session (Cmd+Shift+K)",
                type: "button",
                children: "Create"
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/hooks/useAvailablePrograms.ts
var useAvailablePrograms = __webpack_require__(56230);
// EXTERNAL MODULE: ./src/lib/constants/programs.ts
var programs = __webpack_require__(49051);
// EXTERNAL MODULE: ./src/lib/sessions/autoApprove.ts
var sessions_autoApprove = __webpack_require__(88242);
// EXTERNAL MODULE: ./src/lib/contexts/FeatureFlagsContext.tsx
var FeatureFlagsContext = __webpack_require__(63367);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FOmnibar.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA9VYy27bOhDd5yu4KWADlSHaseMom_5GVwUtjWw2FKmSVCznIv9-IZKSqKftpJuiQWFRQ86ZmTMP6scrXFJJMlDo1-bpclCbEP33gFAqRWZ-ICRyElN9iVD48oDQxwNCWvRfYfvq4-HH4Dw8fx5CWhKuUiGzyP5kRMPPRYB3eblEKiYMfi7C1fN2acVTyjTICB0kPZ40B6UW62WNbBd-c1pGxPCqFZwwYQpN2CDB0zDwctoL674XPD1SaKJhESZwHOIbym12nuTHw8pp2JgNuVBUU8EjlNISkhdzVO58zSDV7qdB7X4fhNYiqx9I_HqUouBJhN6IXATBL_wM72v5TozKhKqckUuEUgZltUAYPfKAasiUXQyUJlJXr34XStP0EsSCa-A6QjFwDbJ6lZMkofwYGHA4fDv5izWgel28gUyZOAeXCJFCi2rtPaA8gbISCsMXzw-Pxg_TZjwtrc0yARlIktBCRQivc2PMmSb6ZM78Vj1mpAzc0i4MrchBlIE6kUScIxSi9TYv0TbMSxRUZyB5PJBF-B25v9V2-R2F5h_Oyx6StaV0peQENh77GYubtTJCJ5okwF86AZfAiKZv4Dtja5wxH7NBUCJU5Z7npSYeeYmUYDTpGQLGjiPJa0e2AHZ3AZghjAvDxgWqdlj9nAquA0XfIUJ475YMF0-S8lfD7RbUk01GZujTJ71JuJxI4Lp1QYS44GCiUGhGObQLsWBC9kkWLvugdh7OlGSUXSJE-Qkk1R1oUZQzEsNJsASkwTmqoKpk7a69zf0mfBUTmxhOZsJ-eWeMW4XP3bBSXvkkuBJdQxCHqsH6WEENr0HFNmtHXZGNJfTjkBYbb-ns6LPt1g5ypXZcplGUHf8cegFxVg_ywHA0oRJim8OxYEXG22zadbMpHs-mK6f0Dkks-2_xzIStuGMrXHGaT7NrNaQXxT5ZKqI0tfrGxHNMGOStyXNXOp1acyIKV3irOgZGqYgLZc30JMeI6O_6TCang-DMWdvde_w3AxEXUlX7c0FtpfAs-pTnT3f1HJMirmUMoIxaMGmvD4HyvNAGSD1T7Lqdq36eMZ_6httJaTZoGZFHyq3kvhMxt7tXCX5_rTmrnMQQHECfwc4iI-4rFMhAAYNYtxF3OJuW05sZXv9ygWLzydctBc28jcLVulMG7OQ-nMfx3s3jrSj_VJ3u-0HMnHKW1Y7q_9HQGBngSZ_gUxPeDeSaryst6tygdiF21ww7wrYyf3wZeyvpi8huB937E81obRpJ1FG-DDnqh58wVjeA_uzXYlODQtsbGidJeVv59TRFp2rsj7jQiyihihwYJMub6vwNVUo1R5oT24v5auu7igsdEMbEubpPtrv1FRjZNIw_HRjaGnnlOLzvbfoK9mK6JOBh59r0kmfvNb8ZwHimk1ruVzfImtXN9c6m9kECeQ3OQnZwv93f2nC_d-_vuBx8bhA5T6W3h2J3V0KPNYDdZEL372yf5ihCGeVB967pmfmZ5OzR-PwlGpf30-Gxa8Plpq8lTdSqi-V40PySW99vM8GFGRJ8je_dwv7o8eBWtuFuGcPhX_7IsA67o1r9PNZiJj8yYDxdY4ou_vWVauSJbq7NMoTTjNhe1nx6xApVIzeRiPKUcqrhZeT2fmAifq2U_cggoQQtcgkpSBVISIoYkiAT9lwuAvsKeAyW792PkB6KgJMMGijui2_7NimkA9s0Xf-1plk1vKYFdxMTEAWBKEyT_fD0Ps7rxXN61-t79XouskWnGuWIVD1nnB2o7q2pJtMonQZp5ea2j4eP_wEz6x5GqhcAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FOmnibar_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9VYy27bOhDd5yu4KWADlSHaseMom_5GVwUtjWw2FKmSVCznIv9_IZKSqKftpJuiQWFRQ86ZmTMP6scrXFJJMlDo1_bpclCbEP33gFAqRWZ_ICRyElN9iVD48oDQxwNCWvRfYfvq4_HH4Dw8fx5CWhKuUiGzyP5kRMPPRYB3eblEKiYMfi7C1fN2acVTyjTICB0kPZ40B6UW62WNbBd_c1pGxPCqFZwwYQpN2CDB0zDwctoL674XPD1SaKJhESZwHOIbym12nuTHw8pp2JgNuVBUU8EjlNISkhdzVO58zSDV7qdB7X4fhNYiqx9I_HqUouBJhN6IXATBL_wM72v5TozKhKqckUuEUgZltUAYPfKAasiUXQyUJlJXr34XStP0EsSCa_A6QjFwDbJ6lZMkofwYGHA4fDv5izWgel28gUyZOAeXCJFCi2rtPaA8gbISCsMXzw_Pxg_TZjwtrc0yARlIktBCRQivc2PMmSb6ZM78Vj1mpAzc0i4MrchBlIE6kUScIxSi9TYv0TbMSxRUZyB5PJBF_B25v9V2_R2F5h_Oyx6StaV0peQENh77GYubtTJCJ5okwF86AZfAiKZv4Dtja5wxH7NBUCJU5Z7npSYeeYmUYDTpGQLGjiPJa0e2AHZ3AZghjAvDxgWqdlj9nAquA0XfIUJ475YMF0_S8lfD7RbUk01GZujTJ71JuJxI4Lp1QYS44GCiUGhGObQLsWBC9kkWLvugdh7OlGSUXSJE_Qkk1R1oUZQzEsNJsASkwTmqoKpk7a69zf0mfBUTmxhOZsJ_eWeMW4XP3bBSXvkkuBJdQxCHqsH6WEENr0HFNmtHXZGNJfTjkBYbb_ns6LPt1g5ypXZcplGUHf8cegFxVg_ywHA0oRJim8OxYEXG22zadbMpHs_mK6f0Dkks_2_xzIStuGMrXHGaT7NrNaQXxT5ZKqI0tfrGxHNMGOStyXNXOp1acyIKV3irOgZGqYgLZc30JMeI6O_6TCang_DMWdvde_w3AxEXUlX7c0FtpfAs_pTnT3f1HJMirmUMoIxaMGmvD4HyvNAGSD1T7Lqdq36eMZ_6httJaTZoGZFHyq3kvhMxt7tXCX5_rTmrnMQQHECfwc4iI_4rFMhAAYNYtxF3OJuW05sZXv9ygWLzydctBc28jcLVulMG7OQ_nMfx3s3jrSj_VJ3u_0HMnHKW1Y7q_9HQGBngSZ_gUxPeDeSaryst6tygdiF21ww7wrYyf3wZeyvpi8huB937E81obRpJ1FG_DDnqh58wVjeA_uzXYlODQtsbGidJeVv59TRFp2rsj7jQiyihihwYJMub6vwNVUo1R5oT24v5auu7igsdEMbEubpPtrv1FRjZNIw_HRjaGnnlOLzvbfoK9mK6JOBh59r0kmfvNb8ZwHimk1ruVzfImtXN9c6m9kECeQ3OQnZwv93f2nC_d_vuBx8bhA5T6W3h2J3V0KPNYDdZEL372yf5ihCGeVB967pmfmZ5OzR_PwlGpf30_Gxa8Plpq8lTdSqi_V40PySW99vM8GFGRJ8je_dwv7o8eBWtuFuGcPhX_7IsA67o1r9PNZiJj8yYDxdY4ou_vWVauSJbq7NMoTTjNhe1nx6xApVIzeRiPKUcqrhZeT2fmAifq2U_cggoQQtcgkpSBVISIoYkiAT9lwuAvsKeAyW792PkB6KgJMMGijui2_7NimkA9s0Xf_1plk1vKYFdxMTEAWBKEyT_fD0Ps7rxXN61_t79XouskWnGuWIVD1nnB2o7q2pJtMonQZp5ea2j4eP_wEz6x5GqhcAAA_22_7D = __webpack_require__(42688);
;// ./src/components/sessions/Omnibar.css.ts

var backlogButton = '_37ybs3p';
var body = '_37ybs3b';
var Omnibar_css_button = '_37ybs3r';
var buttonPrimary = '_37ybs3t';
var buttonSecondary = '_37ybs3s';
var Omnibar_css_checkbox = '_37ybs3h';
var collapsible = '_37ybs3i';
var collapsibleContent = '_37ybs3n';
var collapsibleHeader = '_37ybs3j';
var collapsibleIcon = '_37ybs3l';
var collapsibleTitle = '_37ybs3k';
var Omnibar_css_completionError = '_37ybs3z';
var createButton = '_37ybs3w';
var detectionBadge = '_37ybs39';
var detectionInfo = '_37ybs38';
var Omnibar_css_error = '_37ybs3u';
var expanded = '_37ybs3m';
var field = '_37ybs3c';
var fieldInput = '_37ybs3e';
var footer = '_37ybs3o';
var hint = '_37ybs3f';
var Omnibar_css_input = '_37ybs37';
var inputContainer = '_37ybs35';
var label = '_37ybs3d';
var modal = '_37ybs34';
var overlay = '_37ybs33';
var pathIndicator = '_37ybs310';
var pathIndicatorInvalid = '_37ybs312';
var pathIndicatorLoading = '_37ybs313';
var pathIndicatorValid = '_37ybs311';
var Omnibar_css_select = '_37ybs3g';
var shortcut = '_37ybs3x';
var shortcutKey = '_37ybs3y';
var shortcuts = '_37ybs3v';
var shortcutsBacklogButton = '_37ybs3q';
var typeIndicator = '_37ybs36';
var unknown = '_37ybs3a';
;// ./src/components/sessions/OmnibarBacklogButton.tsx
/* __next_internal_client_entry_do_not_use__ OmnibarBacklogButton auto */ 
function OmnibarBacklogButton(param) {
    let { onClick, disabled = false, className } = param;
    // The modal-wide handleKeyDown would otherwise also run handleSubmit on Enter (a focused
    // button still fires click, so Ctrl/Cmd+Enter would run both actions).
    const handleKeyDown = (e)=>{
        if (e.key === "Enter") e.stopPropagation();
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
        type: "button",
        "data-testid": "omnibar-create-backlog-button",
        className: className,
        onClick: onClick,
        onKeyDown: handleKeyDown,
        disabled: disabled,
        children: "Create Backlog Item"
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FOmnibarCreationPanel.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA81X227jIBB9z1fwslKiLZadm1v6MRU22KHB4AWcOFn131eGpDWOL4m6K60iNZFn5jBzZuaYBm8RiSt2iEPwewZAiQlhIkfggNUcwrfohZ6X6rw8LzpPVtHidQZAgtN9rmQlSDckdXapCFUwkcbIAoGorIGWnJGOc2adMykM1OxMu1irxNpTyaXq2qKv0AwXjJ8QKKSQusQpbSxHqQhMFMV7BOwXxJy_zj5mwbXyyFZOmC45PiGQcVo3gTkub2g4ucM4reFRNfbmrwe2HKXxdENjeFfl1n6kLN8ZBDZh2KJWYcIqfRO3bdE_zDu9aaJRWOgSKyrMBOVppXRjKyUThqrmkQ1mhkmBAOYchEGkAcWa-hShnTxQZYkanp_n9vz0psEWHdhMppWGB6ZZwqmFl5XhTFAElgMEFPaUixuUWaapsd4e9Goi12J4PH9NllH4ZazuYkdMwgofdu3mUl7boyjHhh38zmw6TjjRkleGurPcCqeYp_MoDH-Anw1PNg9OM4OAnUrlZtT-PkMmCK2biX1gGEfHOr741FDvMJFHBEIA12UNoqbDKk_wPHyynyDauN4eqMp447ljhFAxrlqxz9q2XxowZ7mAzNBCI5DS6_j3Kca5XzEAKLDKmYBG9se0koj_vaR0PPI7OvEfC0zsVggJaeaIMI0TTsnib-tN_AntpKbEKTMnBMJg085eSNO8ceSREi_82UYNdwmPvPJCP5MXf0oTLtP9-AhMgfvjue7IIXYF9yxWgWu4oy0JaDfsywjCYLlxjYOyMh54YsHbQNswLOtRsNUFiwkPKr1ANZXcdaNxOQ9s2iocuAPdyIPddsIUTV2uqeRVIcYV4j7Ze3TZ-q5kbRa_fJtVurTkyduClsEjl0zsUxQ_-uajd4itpVYbrMwAnavObmRuzWzYTjGxvwzmkRGzQyAKNooWzYPrtH09-cyECXs7mFD_90oblp1gKoWhwrRN4819mWjb5O3iIamP3e3RVvRZs0dZ3t-Hx6ba3ZQLJuCF6dA7Yzehfj2Jb13ivTx0ms6-oa2LW3aCtYf-_g305Y249vDWOms_tWWdynlL8qDqe7lf_4cZpf7jD4ErgTgXDgAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FOmnibarCreationPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81X227jIBB9z1fwslKiLZadm1v6MRU22KHB4AWcOFn131eGpDWOL4m6K60iNZFn5jBzZuaYBm8RiSt2iEPwewZAiQlhIkfggNUcwrfohZ6X6rw8LzpPVtHidQZAgtN9rmQlSDckdXapCFUwkcbIAoGorIGWnJGOc2adMykM1OxMu1irxNpTyaXq2qKv0AwXjJ8QKKSQusQpbSxHqQhMFMV7BOwXxJy_zj5mwbXyyFZOmC45PiGQcVo3gTkub2g4ucM4reFRNfbmrwe2HKXxdENjeFfl1n6kLN8ZBDZh2KJWYcIqfRO3bdE_zDu9aaJRWOgSKyrMBOVppXRjKyUThqrmkQ1mhkmBAOYchEGkAcWa_hShnTxQZYkanp_n9vz0psEWHdhMppWGB6ZZwqmFl5XhTFAElgMEFPaUixuUWaapsd4e9Goi12J4PH9NllH4ZazuYkdMwgofdu3mUl7boyjHhh38zmw6TjjRkleGurPcCqeYp_MoDH_Anw1PNg9OM4OAnUrlZtT_PkMmCK2biX1gGEfHOr741FDvMJFHBEIA12UNoqbDKk_wPHyynyDauN4eqMp447ljhFAxrlqxz9q2XxowZ7mAzNBCI5DS6_j3Kca5XzEAKLDKmYBG9se0koj_vaR0PPI7OvEfC0zsVggJaeaIMI0TTsnib_tN_AntpKbEKTMnBMJg085eSNO8ceSREi_82UYNdwmPvPJCP5MXf0oTLtP9_AhMgfvjue7IIXYF9yxWgWu4oy0JaDfsywjCYLlxjYOyMh54YsHbQNswLOtRsNUFiwkPKr1ANZXcdaNxOQ9s2iocuAPdyIPddsIUTV2uqeRVIcYV4j7Ze3TZ_q5kbRa_fJtVurTkyduClsEjl0zsUxQ_uajd4itpVYbrMwAnavObmRuzWzYTjGxvwzmkRGzQyAKNooWzYPrtH09_cyECXs7mFD_90oblp1gKoWhwrRN4819mWjb5O3iIamP3e3RVvRZs0dZ3t_Hx6ba3ZQLJuCF6dA7Yzehfj2Jb13ivTx0ms6_oa2LW3aCtYf_g305Y249vDWOms_tWWdynlL8qDqe7lf_4cZpf7jD4ErgTgXDgAA_22_7D = __webpack_require__(67710);
;// ./src/components/sessions/OmnibarCreationPanel.css.ts

var advancedSection = '_1d7uiv7a';
var advancedSectionOpen = '_1d7uiv7b';
var attachArea = '_1d7uiv76';
var attachButton = '_1d7uiv77';
var OmnibarCreationPanel_css_attachError = '_1d7uiv78';
var createRepoNotice = '_1d7uiv7c';
var createRepoNoticeActive = '_1d7uiv7d';
var createRepoNoticeBlocked = '_1d7uiv7j';
var createRepoNoticeBody = '_1d7uiv7g';
var createRepoNoticeDesc = '_1d7uiv7i';
var createRepoNoticeIcon = '_1d7uiv7f';
var createRepoNoticeIconError = '_1d7uiv7k';
var createRepoNoticeRow = '_1d7uiv7e';
var createRepoNoticeTitle = '_1d7uiv7h';
var pathDisplay = '_1d7uiv70';
var piApprovalWarning = '_1d7uiv79';
var radioBtn = '_1d7uiv72';
var radioBtnActive = '_1d7uiv73';
var radioGroup = '_1d7uiv71';
var remoteIcon = '_1d7uiv7l';
var slashDropdownWrapper = '_1d7uiv75';
var textareaWrapper = '_1d7uiv74';
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/image.mjs
var icons_image = __webpack_require__(31995);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/file-code.mjs
var file_code = __webpack_require__(43444);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/file-text.mjs
var file_text = __webpack_require__(30294);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/file-archive.mjs
var file_archive = __webpack_require__(42825);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/file.mjs
var file = __webpack_require__(68108);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FFileChipList.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA4WTQW6DMBBF9znFLNuFIyCtqjqHiRx7gGkcG9kmJKly98pOoYSAugHBfOZ_zzzWu1yWZeuyDL5XAIp8o8WFQ6nxvF1BurPOiYZDvMZXVXw4CffC2C7_xGvhrsX1NVaOwlVkWLDLgjOrkao6cMiLrEkW9oSu1LZjFw6iDXa7uq3Wfaz8MRYZTQZZn05oqgyjgEfPQaIJ6JYSXlKARihFppqpzgfeW6fQMScUtX761ebjrhHyUDnbGjUVyFEPDnlzBm81qYkKk6q0JjBPV3xy2ae6tNq6aS0bptqRCjWHIpsMlUNNSqF5GGqRhvosAQh4DuyvgFpT48nHUldTQOYbIZGDsT0NRzK9efZgskkmiR9fOzKHJAAYkt6D9jgMNOy_UAZWUuAgY5KZNRRROrJ6W7B6onmWl1Gj94VGv2BrLMMCWGMIjDU4Xnz_LFvn4w4bSz2pA4_JJrE9_B-LW8__ASZiNz4Ur-Mc09Hm-0WMbj84EkkHCgQAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FFileChipList_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA4WTQW6DMBBF9znFLNuFIyCtqjqHiRx7gGkcG9kmJKly98pOoYSAugHBfOZ_zzzWu1yWZeuyDL5XAIp8o8WFQ6nxvF1BurPOiYZDvMZXVXw4CffC2C7_xGvhrsX1NVaOwlVkWLDLgjOrkao6cMiLrEkW9oSu1LZjFw6iDXa7uq3Wfaz8MRYZTQZZn05oqgyjgEfPQaIJ6JYSXlKARihFppqpzgfeW6fQMScUtX761ebjrhHyUDnbGjUVyFEPDnlzBm81qYkKk6q0JjBPV3xy2ae6tNq6aS0bptqRCjWHIpsMlUNNSqF5GGqRhvosAQh4DuyvgFpT48nHUldTQOYbIZGDsT0NRzK9efZgskkmiR9fOzKHJAAYkt6D9jgMNOy_UAZWUuAgY5KZNRRROrJ6W7B6onmWl1Gj94VGv2BrLMMCWGMIjDU4Xnz_LFvn4w4bSz2pA4_JJrE9_B_LW8_ASZiNz4Ur_Mc09Hm_0WMbj84EkkHCgQAAA_22_7D = __webpack_require__(39792);
;// ./src/components/sessions/FileChipList.css.ts

var chip = '_1cffur01';
var chipIconWrapper = '_1cffur04';
var chipList = '_1cffur00';
var chipName = '_1cffur02';
var chipRemove = '_1cffur05';
var chipThumbnail = '_1cffur03';
;// ./src/components/sessions/FileChipList.tsx
// +feature: file-chip-list




function iconForMimeType(mimeType) {
    if (mimeType.startsWith("image/")) return icons_image/* default */.A;
    if (mimeType.startsWith("text/") || /\/(javascript|typescript|json|xml|yaml|toml)/.test(mimeType) || /x-(python|go|rust|c|java|ruby|sh)/.test(mimeType)) {
        return file_code/* default */.A;
    }
    if (mimeType === "application/pdf" || mimeType.includes("word") || mimeType.includes("document") || mimeType.includes("spreadsheet") || mimeType.includes("presentation")) {
        return file_text/* default */.A;
    }
    if (mimeType.includes("zip") || mimeType.includes("tar") || mimeType.includes("gzip") || mimeType.includes("bzip") || mimeType.includes("7z") || mimeType.includes("rar") || mimeType.includes("archive")) {
        return file_archive/* default */.A;
    }
    return file/* default */.A;
}
function FileChipList(param) {
    let { files, onRemove } = param;
    if (files.length === 0) return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: chipList,
        role: "list",
        "aria-label": "Attached files",
        children: files.map((f, i)=>{
            const isImage = f.file.type.startsWith("image/") && !!f.previewUrl;
            const Icon = iconForMimeType(f.file.type);
            return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: chip,
                role: "listitem",
                title: f.name,
                children: [
                    isImage ? // eslint-disable-next-line @next/next/no-img-element
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("img", {
                        src: f.previewUrl,
                        alt: f.name,
                        className: chipThumbnail
                    }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: chipIconWrapper,
                        "aria-hidden": "true",
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(Icon, {
                            size: 16
                        })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: chipName,
                        children: f.name
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: chipRemove,
                        onClick: ()=>onRemove(i),
                        "aria-label": "Remove ".concat(f.name),
                        children: "\xd7"
                    })
                ]
            }, f.path);
        })
    });
}

// EXTERNAL MODULE: ./src/components/ui/RadioGroup.tsx
var RadioGroup = __webpack_require__(98764);
// EXTERNAL MODULE: ./src/components/ui/RepoPathInput.tsx + 2 modules
var RepoPathInput = __webpack_require__(34609);
// EXTERNAL MODULE: ./src/components/ui/SlashCommandDropdown.tsx + 1 modules
var SlashCommandDropdown = __webpack_require__(25562);
// EXTERNAL MODULE: ./src/lib/hooks/useSlashCommands.ts
var useSlashCommands = __webpack_require__(49179);
// EXTERNAL MODULE: ./src/lib/hooks/useSlashCommandSuggestions.ts
var useSlashCommandSuggestions = __webpack_require__(64887);
// EXTERNAL MODULE: ./src/lib/hooks/useFanoutService.ts
var useFanoutService = __webpack_require__(3358);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FOmnibarPresetList.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA51T0XLCIBB89yvusT7gJNFqix_jYLgkNyXAADFqp__egWo1Wq3tI3fL7S57TFY5KpoGn8H7CECRD8yHnUIO2mhcjgBa4WrSHLJ4sEJK0vXh1Iota5DqJnAoZpndxqLZoKuU6dmOg-iCibW1cRIdh9xuwRtFEjbCPTG2yl9xX7g9jk8o5oSkzvMLyHQxXo4-RpOj4DwJluStEjsOlcLELhTVmlHA1nMoUQd0sVwLezmw2I8Hjq66lwKyhC87543jYA19DT_XxJvoPilbi_KtdqbT8nL0y9BI8QAcwHRBkUYOxY0nbM9hzFSVx8CBFTGUM7ZpYquMDqw_JDfPUpilUdHXcGo21DpLt39E5ok_xsAhP98DDg1JiTrWAm4DOzVQKbKefGz1DQVk3ooy7V7vhF0elVaiJbXj0BptEuK742mPV5uyHop-TqJv5HxM9Y6lR3nm_-Up_sazeIDn9jrl-R3D2YP_8Fe1n_JrvAlYBAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FOmnibarPresetList_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA51T0XLCIBB89yvusT7gJNFqix_jYLgkNyXAADFqp_egWo1Wq3tI3fL7S57TFY5KpoGn8H7CECRD8yHnUIO2mhcjgBa4WrSHLJ4sEJK0vXh1Iota5DqJnAoZpndxqLZoKuU6dmOg_iCibW1cRIdh9xuwRtFEjbCPTG2yl9xX7g9jk8o5oSkzvMLyHQxXo4_RpOj4DwJluStEjsOlcLELhTVmlHA1nMoUQd0sVwLezmw2I8Hjq66lwKyhC87543jYA19DT_XxJvoPilbi_KtdqbT8nL0y9BI8QAcwHRBkUYOxY0nbM9hzFSVx8CBFTGUM7ZpYquMDqw_JDfPUpilUdHXcGo21DpLt39E5ok_xsAhP98DDg1JiTrWAm4DOzVQKbKefGz1DQVk3ooy7V7vhF0elVaiJbXj0BptEuK742mPV5uyHop_TqJv5HxM9Y6lR3nm_Up_sazeIDn9jrl_R3D2YP_8Fe1n_JrvAlYBAAA_22_7D = __webpack_require__(15485);
;// ./src/components/sessions/OmnibarPresetList.css.ts


var emptyState = '_1eli3ts5';
var errorState = '_1eli3ts7';
var OmnibarPresetList_css_list = '_1eli3ts0';
var loadingState = '_1eli3ts6';
var OmnibarPresetList_css_row = '_1eli3ts1';
var rowArgv = '_1eli3ts4';
var rowLabel = '_1eli3ts3';
var rowSelected = '_1eli3ts2 _1eli3ts1';
;// ./src/components/sessions/OmnibarPresetList.tsx
// +feature: launcher-presets-list
/* __next_internal_client_entry_do_not_use__ OmnibarPresetList auto */ 

// OmnibarPresetList renders the "Presets" section of the creation panel: hand-edited,
// argv-based launch shortcuts from ~/.stapler-squad/launcher-presets.json. Structurally
// mirrors AliasPalette's a11y shape (listbox/option rows, status/alert states).
function OmnibarPresetList(param) {
    let { presets, loading, loadError, onSelect } = param;
    if (loadError) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            role: "alert",
            "aria-live": "assertive",
            className: errorState,
            "data-testid": "preset-config-error",
            children: loadError
        });
    }
    // Only the first fetch shows a loading state — once any presets have been shown once,
    // subsequent (e.g. Omnibar-reopen) refetches keep the last-known list visible rather than
    // flashing a loading state over already-rendered rows (avoids layout jank on every open).
    if (loading && presets.length === 0) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            role: "status",
            "aria-live": "polite",
            className: loadingState,
            "data-testid": "preset-list-loading",
            children: "Loading presets…"
        });
    }
    if (presets.length === 0) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            role: "status",
            className: emptyState,
            "data-testid": "preset-list-empty",
            children: [
                "No presets yet. Add one in ",
                /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                    children: "~/.stapler-squad/launcher-presets.json"
                }),
                "."
            ]
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
        role: "listbox",
        "aria-label": "Launcher presets",
        className: OmnibarPresetList_css_list,
        children: presets.map((preset)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(PresetRow, {
                preset: preset,
                onSelect: onSelect
            }, preset.id))
    });
}
function PresetRow(param) {
    let { preset, onSelect } = param;
    // Always derived from argv, never preset.program — program is presentation-only (per the
    // LauncherPresetProto proto comment and ADR-001 §3) and can disagree with what actually
    // launches if a preset author sets it to something other than argv[0]. Previewing argv
    // keeps "what you see is what launches" true even for a malformed/inconsistent preset.
    const argvPreview = preset.argv.join(" ");
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
        role: "option",
        "aria-selected": false,
        "aria-label": "".concat(preset.label, " — ").concat(argvPreview, " — selecting will replace the current program and extra args"),
        tabIndex: 0,
        className: OmnibarPresetList_css_row,
        onClick: ()=>onSelect(preset),
        onKeyDown: (e)=>{
            if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                onSelect(preset);
            }
        },
        "data-testid": "preset-row",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: rowLabel,
                children: preset.label
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: rowArgv,
                children: argvPreview
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/hooks/useProbeProgram.ts
var useProbeProgram = __webpack_require__(62182);
// EXTERNAL MODULE: ./src/components/ui/ProbeStatusBadge.tsx + 1 modules
var ProbeStatusBadge = __webpack_require__(6990);
// EXTERNAL MODULE: ./src/components/ui/UnknownFlagsWarning.tsx
var UnknownFlagsWarning = __webpack_require__(391);
// EXTERNAL MODULE: ./src/lib/flags/validateFlags.ts
var validateFlags = __webpack_require__(22017);
;// ./src/components/sessions/ProgramProbeSection.tsx
// +feature: session-create settings-programs
/* __next_internal_client_entry_do_not_use__ PROGRAM_PROBE_STATUS_ID,ProgramProbeSection auto */ 





const PROGRAM_PROBE_STATUS_ID = "omnibar-program-probe-status";
// Kept so the not-found variant stays addressable by its pre-probe testid.
const NOT_FOUND_TEST_ID = "preset-program-warning";
/** Probe status for the selected program. Selection only resolves; running --help needs an explicit Check. */ function ProgramProbeSection(param) {
    let { option } = param;
    var _option_command;
    const command = (_option_command = option === null || option === void 0 ? void 0 : option.command) !== null && _option_command !== void 0 ? _option_command : "";
    const { state, checkedToken, check } = (0,useProbeProgram/* useProbeProgram */.eT)(command, "picker");
    (0,react.useEffect)(()=>{
        check();
    }, [
        check
    ]);
    // Only the saved cli_flags are validated; alias extraFlags never reach the panel (Flagged Choice 2).
    const unknown = (0,react.useMemo)(()=>{
        var _option_cliFlags;
        return state.kind === "found" ? (0,validateFlags/* validateFlags */.k)((_option_cliFlags = option === null || option === void 0 ? void 0 : option.cliFlags) !== null && _option_cliFlags !== void 0 ? _option_cliFlags : "", state.flags) : [];
    }, [
        state,
        option === null || option === void 0 ? void 0 : option.cliFlags
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)(ProbeStatusBadge/* ProbeStatusBadge */.q5, {
                state: state,
                checkedToken: checkedToken,
                onRetry: ()=>check({
                        immediate: true
                    }),
                onConfirm: ()=>check({
                        explicit: true
                    }),
                testId: state.kind === "notFound" ? NOT_FOUND_TEST_ID : "program-probe-badge",
                id: PROGRAM_PROBE_STATUS_ID
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(UnknownFlagsWarning/* UnknownFlagsWarning */.u, {
                id: "omnibar-flags-warning-text",
                testId: "omnibar-flags-warning",
                program: checkedToken,
                unknown: unknown
            })
        ]
    });
}

;// ./src/components/sessions/OmnibarCreationPanel.tsx
/* __next_internal_client_entry_do_not_use__ SESSION_TYPES,AUTONOMOUS_MODE_HINT,OmnibarCreationPanel auto */ 
// +feature: session-image-attach pi-support




















// ─── Session Type Radio Group ────────────────────────────────────────────────
const SESSION_TYPES = [
    {
        value: "new_worktree",
        label: "New branch (isolated)",
        description: "Use this when you want to try something risky without touching your main branch — e.g. a refactor, a new feature, or a change you might abandon. Creates an isolated branch and working directory."
    },
    {
        value: "directory",
        label: "Existing folder",
        description: "Use this when you just want to work in a folder as-is — e.g. quick edits to a repo you already have checked out, or a folder with no git history."
    },
    {
        value: "existing_worktree",
        label: "Existing branch",
        description: "Use this when you want to resume work on a branch that's already checked out — e.g. picking up review feedback or continuing a session from earlier."
    },
    {
        value: "one_off",
        label: "Temporary (no git)",
        description: "Use this when you need scratch space for a quick experiment — e.g. testing a snippet or script. No path needed; a temporary directory is created automatically."
    },
    {
        value: "new_project",
        label: "New Project",
        description: "Use this when starting something brand new — e.g. a side project or prototype. Creates a directory, runs git init, and makes an initial commit."
    }
];
// Autonomous mode's hint text, shown when the "Autonomous mode" checkbox is checked.
// Not a session type itself — it's an orthogonal flag that composes with whichever
// type is selected above (see AUTONOMOUS_MODE_HINT usage below).
const AUTONOMOUS_MODE_HINT = "Hand off a well-defined task and walk away — e.g. a small bug fix or chore. An LLM reviewer approves risky tool calls instead of you; you'll be notified when it's done. To stop it, delete or hibernate the session.";
const PRIMARY_TYPES = SESSION_TYPES.slice(0, 2).concat([
    SESSION_TYPES[3]
]); // new_worktree, directory, one_off
const ADVANCED_TYPES = [
    SESSION_TYPES[2],
    SESSION_TYPES[4]
]; // existing_worktree, new_project
const ADVANCED_VALUES = new Set(ADVANCED_TYPES.map((t)=>t.value));
// Stable identity for callers that omit launcherPresets — a fresh `[]` literal as a default
// parameter value would get a new identity every render, retriggering the auto-open effect's
// dependency array unnecessarily.
const EMPTY_PRESETS = [];
// Stable identity for callers that omit remotes — see EMPTY_PRESETS above for why.
const EMPTY_REMOTES = [];
// Radio options for the "Open as" sub-selector inside New Project mode.
const NEW_PROJECT_OPEN_AS = [
    {
        value: "new_worktree",
        label: "New Worktree"
    },
    {
        value: "directory",
        label: "Directory"
    }
];
function SessionTypeRadioGroup(param) {
    let { value, onChange } = param;
    // Expand advanced section automatically if the current selection is an advanced type
    const [advancedOpen, setAdvancedOpen] = (0,react.useState)(()=>ADVANCED_VALUES.has(value));
    const visibleTypes = advancedOpen ? [
        ...PRIMARY_TYPES,
        ...ADVANCED_TYPES
    ] : PRIMARY_TYPES;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(RadioGroup/* RadioGroup */.z, {
        options: visibleTypes,
        value: value,
        onChange: onChange,
        groupLabel: "Session Type",
        groupLabelId: "omnibar-session-type-label",
        hintForValue: (v)=>{
            var _SESSION_TYPES_find;
            return (_SESSION_TYPES_find = SESSION_TYPES.find((t)=>t.value === v)) === null || _SESSION_TYPES_find === void 0 ? void 0 : _SESSION_TYPES_find.description;
        },
        trailingContent: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
            type: "button",
            tabIndex: -1,
            "aria-expanded": advancedOpen,
            onClick: ()=>setAdvancedOpen((o)=>!o),
            className: radioBtn,
            style: {
                opacity: 0.65,
                fontSize: "0.75em"
            },
            children: advancedOpen ? "▴ Less" : "▾ More"
        })
    });
}
// Helper: file → base64 string (strips data URL prefix).
function fileToBase64(file) {
    return new Promise((resolve, reject)=>{
        const reader = new FileReader();
        reader.onload = ()=>resolve(reader.result.split(",")[1]);
        reader.onerror = reject;
        reader.readAsDataURL(file);
    });
}
function OmnibarCreationPanel(param) {
    let { formState, setFormField, onSubmit, onCancel, onCreateBacklog, worktrees, isWorktreesLoading = false, worktreesError = null, isSubmitting, canSubmit, error, showAdvanced, onToggleAdvanced, path, uploadBaseUrl = "/api", onAttachedImagesChange, pathDoesNotExist, namePrefix = "", destinationPreviewPath = null, destinationPreviewIsExact = false, isDestinationPreviewLoading = false, onPresetSelect, launcherPresets: presets = EMPTY_PRESETS, launcherPresetsLoading: presetsLoading = false, launcherPresetsLoadError: presetsLoadError = null, remotes = EMPTY_REMOTES, piApprovalExtensionFailed = false } = param;
    const { sessionName, branch, program, category, autoYes, autoApprove, useTitleAsBranch, sessionType, existingWorktree, workingDir, parentDir, projectName, newProjectSessionType, createIfMissing, firstPrompt, autonomousMode, remoteName } = formState;
    // If the program changes to an unsupported agent after auto-approve was checked
    // (e.g. user picks "claude", checks the box, then switches to "codex"), force it back
    // off rather than silently submitting a checked-but-disabled checkbox's stale true value.
    (0,react.useEffect)(()=>{
        if (autoApprove && !(0,sessions_autoApprove/* isAutoApproveSupported */.$)(program)) {
            setFormField("autoApprove", false);
        }
    }, [
        program,
        autoApprove,
        setFormField
    ]);
    // Slash command autocomplete for the firstPrompt textarea.
    const firstPromptRef = (0,react.useRef)(null);
    const [firstPromptCursor, setFirstPromptCursor] = (0,react.useState)(0);
    const [slashSuggestIndex, setSlashSuggestIndex] = (0,react.useState)(-1);
    const { commands: slashCommands } = (0,useSlashCommands/* useSlashCommands */.d)(path !== null && path !== void 0 ? path : "");
    const slashState = (0,useSlashCommandSuggestions/* useSlashCommandSuggestions */.X)(firstPrompt, firstPromptCursor, slashCommands);
    const isSlashDropdownVisible = slashState.isActive && slashState.suggestions.length > 0;
    const handleSlashSelect = (0,react.useCallback)((cmd)=>{
        const { newValue, newCursorPos } = slashState.complete(firstPrompt, cmd);
        setFormField("firstPrompt", newValue);
        setSlashSuggestIndex(-1);
        requestAnimationFrame(()=>{
            if (firstPromptRef.current) {
                firstPromptRef.current.setSelectionRange(newCursorPos, newCursorPos);
                firstPromptRef.current.focus();
                setFirstPromptCursor(newCursorPos);
            }
        });
    }, [
        slashState,
        firstPrompt,
        setFormField
    ]);
    const handleFirstPromptKeyDown = (0,react.useCallback)((e)=>{
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
    // "Create new repository" affordance is only meaningful for session types
    // that operate on the path itself. existing_worktree expects a real parent
    // repo; we surface a different (blocking) message there.
    const showCreateRepoNotice = pathDoesNotExist === true && (sessionType === "directory" || sessionType === "new_worktree");
    const showExistingWorktreePathError = pathDoesNotExist === true && sessionType === "existing_worktree";
    // ─── Load default parentDir from config when new_project mode is first selected ──
    (0,react.useEffect)(()=>{
        if (sessionType !== "new_project" || parentDir) return;
        const load = async ()=>{
            try {
                var _resp_defaults;
                const client = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
                const resp = await client.getSessionDefaults({});
                const dir = (_resp_defaults = resp.defaults) === null || _resp_defaults === void 0 ? void 0 : _resp_defaults.newProjectBaseDir;
                if (dir && !parentDir) {
                    setFormField("parentDir", dir);
                }
            } catch (e) {
            // Non-critical: falls back to empty; user can type manually
            }
        };
        void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        sessionType
    ]);
    const availablePrograms = (0,useAvailablePrograms/* useAvailablePrograms */.U)();
    const piSupportEnabled = (0,FeatureFlagsContext/* useFeatureFlag */.u)(programs/* PI_SUPPORT_FLAG_NAME */.HN);
    // Story 3.1.1: "pi" is only offered in the rendered picker options when pi-support is on —
    // opt-in invisibility. availablePrograms (not the raw PROGRAMS constant) is the base list
    // here since it may also include extra programs detected on the host; getPickerPrograms
    // takes that list as a parameter so this call site and programs.ts share one filter rule.
    const pickerPrograms = (0,programs/* getPickerPrograms */.qG)(availablePrograms, piSupportEnabled);
    // Story 3.1.2: the capability warning is only even eligible to show for a program the pi
    // approval extension covers, and only while pi-support itself is on.
    const showPiApprovalWarning = piSupportEnabled && (0,sessions_autoApprove/* isApprovalExtensionSupported */.H)(program) && piApprovalExtensionFailed;
    // Default-expanded once there's something to show — either a loaded preset or a config
    // error. An error must never sit hidden behind a collapsed section: AC requires a malformed
    // config to "fail loudly", and a load_error with zero presets (the RPC's own contract) would
    // otherwise default to collapsed under the empty-state branch below, silently hiding it. A
    // truly empty, error-free state is the only case that stays collapsed (design/ux.md §5.1.1).
    // Auto-opens only once: a ref (not an ongoing effect) so a later background refetch can't
    // re-force the section open after the user has manually collapsed it.
    const shouldAutoOpenPresets = presets.length > 0 || Boolean(presetsLoadError);
    const [presetsOpen, setPresetsOpen] = (0,react.useState)(()=>shouldAutoOpenPresets);
    const hasAutoOpenedPresets = (0,react.useRef)(shouldAutoOpenPresets);
    (0,react.useEffect)(()=>{
        if (!hasAutoOpenedPresets.current && shouldAutoOpenPresets) {
            hasAutoOpenedPresets.current = true;
            setPresetsOpen(true);
        }
    }, [
        shouldAutoOpenPresets
    ]);
    // ─── File attachment state ────────────────────────────────────────────────
    const [attachedFiles, setAttachedFiles] = (0,react.useState)([]);
    // Mirror of attachedFiles kept in a ref so the unmount cleanup can revoke
    // object URLs without capturing stale closure values.
    const attachedFilesRef = (0,react.useRef)([]);
    const [attachError, setAttachError] = (0,react.useState)(null);
    const [isAttaching, setIsAttaching] = (0,react.useState)(false);
    const attachInputRef = (0,react.useRef)(null);
    // Keep ref in sync with state so the unmount cleanup always sees current files.
    (0,react.useEffect)(()=>{
        attachedFilesRef.current = attachedFiles;
        onAttachedImagesChange === null || onAttachedImagesChange === void 0 ? void 0 : onAttachedImagesChange(attachedFiles.map((f)=>f.path));
    }, [
        attachedFiles,
        onAttachedImagesChange
    ]);
    // Revoke object URLs on unmount via ref — avoids stale closure over empty array.
    (0,react.useEffect)(()=>{
        return ()=>{
            attachedFilesRef.current.forEach((f)=>{
                if (f.previewUrl) URL.revokeObjectURL(f.previewUrl);
            });
        };
    }, []);
    const handleAttachFiles = (0,react.useCallback)(async (e)=>{
        var _e_target_files;
        const files = Array.from((_e_target_files = e.target.files) !== null && _e_target_files !== void 0 ? _e_target_files : []);
        e.target.value = "";
        if (!files.length) return;
        const toUpload = files; // no cap; deduplication applied below
        setIsAttaching(true);
        setAttachError(null);
        const MAX_FILE_BYTES = 20 * 1024 * 1024; // 20 MB — must match backend maxUploadBytes
        // Size check — reject oversized files before FileReader allocation
        const oversized = toUpload.filter((f)=>f.size > MAX_FILE_BYTES);
        if (oversized.length > 0) {
            setAttachError("".concat(oversized.map((f)=>f.name).join(", "), ": exceeds 20 MB limit"));
        // Continue with remaining files (don't abort the whole batch)
        }
        const sizedOk = toUpload.filter((f)=>f.size <= MAX_FILE_BYTES);
        // Deduplication — skip files already attached (by name+size+lastModified)
        const existingKeys = new Set(attachedFiles.map((f)=>"".concat(f.file.name, "|").concat(f.file.size, "|").concat(f.file.lastModified)));
        const deduplicated = sizedOk.filter((f)=>!existingKeys.has("".concat(f.name, "|").concat(f.size, "|").concat(f.lastModified)));
        const results = [];
        for (const file of deduplicated){
            const isImage = file.type.startsWith("image/");
            const previewUrl = isImage ? URL.createObjectURL(file) : undefined;
            try {
                const base64 = await fileToBase64(file);
                const resp = await fetch("".concat(uploadBaseUrl, "/upload/file"), {
                    method: "POST",
                    headers: {
                        "Content-Type": "application/json"
                    },
                    body: JSON.stringify({
                        data: base64,
                        contentType: file.type,
                        originalFilename: file.name
                    })
                });
                if (!resp.ok) {
                    if (previewUrl) URL.revokeObjectURL(previewUrl);
                    setAttachError("Upload failed");
                    break;
                }
                const data = await resp.json();
                results.push({
                    file,
                    path: data.path,
                    previewUrl,
                    name: file.name,
                    size: file.size
                });
            } catch (e) {
                if (previewUrl) URL.revokeObjectURL(previewUrl);
                setAttachError("Upload failed");
                break;
            }
        }
        setAttachedFiles((prev)=>[
                ...prev,
                ...results
            ]);
        setIsAttaching(false);
    }, [
        attachedFiles,
        uploadBaseUrl
    ]);
    const removeFile = (0,react.useCallback)((index)=>{
        setAttachedFiles((prev)=>{
            const f = prev[index];
            if (f.previewUrl) URL.revokeObjectURL(f.previewUrl);
            return prev.filter((_, i)=>i !== index);
        });
    }, []);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            path && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: pathDisplay,
                children: path
            }),
            showCreateRepoNotice && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: [
                    createRepoNotice,
                    createIfMissing ? createRepoNoticeActive : ""
                ].filter(Boolean).join(" "),
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: createRepoNoticeRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: createRepoNoticeIcon,
                                "aria-hidden": "true",
                                children: "+"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: createRepoNoticeBody,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                        className: createRepoNoticeTitle,
                                        children: "Path doesn’t exist yet"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                        className: createRepoNoticeDesc,
                                        children: "Stapler Squad can create the directory and initialize a fresh git repository (with an initial commit) at this location before starting the session."
                                    })
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                        className: Omnibar_css_checkbox,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                type: "checkbox",
                                checked: createIfMissing,
                                onChange: (e)=>setFormField("createIfMissing", e.target.checked)
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                children: "Create a new git repository here"
                            })
                        ]
                    }),
                    !createIfMissing && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: createRepoNoticeBlocked,
                        role: "status",
                        children: "Check the box above to create the repository, or pick an existing path to continue."
                    })
                ]
            }),
            showExistingWorktreePathError && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: createRepoNotice,
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: createRepoNoticeRow,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: "".concat(createRepoNoticeIcon, " ").concat(createRepoNoticeIconError),
                            "aria-hidden": "true",
                            children: "!"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: createRepoNoticeBody,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: createRepoNoticeTitle,
                                    children: "Repository path doesn’t exist"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: createRepoNoticeDesc,
                                    children: "“Existing branch” needs a real parent repository. Switch to “Existing folder” or “New branch (isolated)” if you want to create a new repo here."
                                })
                            ]
                        })
                    ]
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: body,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: field,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: label,
                                htmlFor: "omnibar-name",
                                children: "Session Name *"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "omnibar-name",
                                type: "text",
                                className: fieldInput,
                                placeholder: "my-feature-session",
                                value: sessionName,
                                onChange: (e)=>setFormField("sessionName", e.target.value)
                            }),
                            !sessionName && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: hint,
                                style: {
                                    color: "var(--error)"
                                },
                                children: "Session name is empty — type a name above or use “name > prompt” syntax"
                            }),
                            namePrefix && sessionName === namePrefix && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: hint,
                                style: {
                                    color: "var(--warning)"
                                },
                                children: [
                                    "Type a label after the prefix to complete the session name (e.g. “",
                                    namePrefix,
                                    "my-feature”)"
                                ]
                            }),
                            firstPrompt && sessionName && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: hint,
                                children: [
                                    "Session name: ",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("strong", {
                                        children: sessionName
                                    }),
                                    " \xb7 First prompt will be typed into the session terminal automatically"
                                ]
                            })
                        ]
                    }),
                    destinationPreviewIsExact && destinationPreviewPath && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: hint,
                        style: {
                            marginTop: 0
                        },
                        children: [
                            "Will check out to: ",
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                                children: destinationPreviewPath
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: field,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionTypeRadioGroup, {
                            value: sessionType,
                            onChange: (v)=>setFormField("sessionType", v)
                        })
                    }),
                    remotes.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: field,
                        "data-testid": "remote-selector-field",
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: label,
                                htmlFor: "omnibar-remote",
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: remoteIcon,
                                        "aria-hidden": "true",
                                        children: "\uD83C\uDF10"
                                    }),
                                    " Remote host"
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                                id: "omnibar-remote",
                                "data-testid": "remote-selector",
                                className: Omnibar_css_select,
                                value: remoteName !== null && remoteName !== void 0 ? remoteName : "",
                                onChange: (e)=>setFormField("remoteName", e.target.value || undefined),
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                        value: "",
                                        children: "This machine"
                                    }),
                                    remotes.map((r)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                            value: r.name,
                                            children: r.name
                                        }, r.name))
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: hint,
                                children: remoteName ? 'Runs on "'.concat(remoteName, '" over SSH.') : "Runs locally by default — pick a configured remote to run this session over SSH instead."
                            })
                        ]
                    }),
                    sessionType !== "one_off" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: field,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: Omnibar_css_checkbox,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        type: "checkbox",
                                        checked: autonomousMode,
                                        onChange: (e)=>setFormField("autonomousMode", e.target.checked)
                                    }),
                                    "\uD83E\uDD16 Autonomous mode (Beta)"
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: hint,
                                children: AUTONOMOUS_MODE_HINT
                            })
                        ]
                    }),
                    sessionType === "one_off" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: hint,
                        style: {
                            marginTop: 0
                        },
                        children: [
                            "Directory will be created in your one-off base directory (default: ",
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                                children: "~/oneoff"
                            }),
                            ") with format ",
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                                children: "YYYYMMDD-word-word-NN"
                            }),
                            ". Configure in Settings → Defaults."
                        ]
                    }),
                    sessionType === "new_project" && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: field,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                        className: label,
                                        htmlFor: "omnibar-parent-dir",
                                        children: "Parent Directory *"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(RepoPathInput/* RepoPathInput */.E, {
                                        id: "omnibar-parent-dir",
                                        placeholder: "~/Projects",
                                        value: parentDir,
                                        onChange: (v)=>setFormField("parentDir", v),
                                        required: true,
                                        hint: "Recent paths below are existing project folders — pick one to use its parent, or type a new directory."
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: field,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                        className: label,
                                        htmlFor: "omnibar-project-name",
                                        children: "Project Name *"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        id: "omnibar-project-name",
                                        type: "text",
                                        className: fieldInput,
                                        placeholder: "my-awesome-project",
                                        value: projectName,
                                        onChange: (e)=>setFormField("projectName", e.target.value)
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: hint,
                                        children: "Name of the new project directory (no path separators)"
                                    })
                                ]
                            }),
                            parentDir.trim() && projectName.trim() && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: pathDisplay,
                                title: "".concat(parentDir.trim().replace(/\/$/, ""), "/").concat(projectName.trim()),
                                children: [
                                    parentDir.trim().replace(/\/$/, ""),
                                    "/",
                                    projectName.trim()
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: field,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                        className: label,
                                        id: "omnibar-open-as-label",
                                        children: "Open as"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                        role: "radiogroup",
                                        "aria-labelledby": "omnibar-open-as-label",
                                        className: radioGroup,
                                        children: NEW_PROJECT_OPEN_AS.map((opt)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                role: "radio",
                                                "aria-checked": newProjectSessionType === opt.value,
                                                tabIndex: newProjectSessionType === opt.value ? 0 : -1,
                                                type: "button",
                                                onClick: ()=>setFormField("newProjectSessionType", opt.value),
                                                className: [
                                                    radioBtn,
                                                    newProjectSessionType === opt.value ? radioBtnActive : ""
                                                ].filter(Boolean).join(" "),
                                                children: opt.label
                                            }, opt.value))
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: hint,
                                        children: newProjectSessionType === "new_worktree" ? "Creates an isolated git worktree for this session" : "Opens the project directory directly without worktree isolation"
                                    })
                                ]
                            }),
                            newProjectSessionType === "new_worktree" && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                        className: Omnibar_css_checkbox,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                type: "checkbox",
                                                checked: useTitleAsBranch,
                                                onChange: (e)=>setFormField("useTitleAsBranch", e.target.checked)
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                children: "Use session name as branch name"
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: field,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                                className: label,
                                                htmlFor: "omnibar-np-branch",
                                                children: [
                                                    "Git Branch ",
                                                    !useTitleAsBranch && "*"
                                                ]
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                id: "omnibar-np-branch",
                                                type: "text",
                                                className: fieldInput,
                                                placeholder: useTitleAsBranch ? sessionName || "Enter session name first" : "main",
                                                value: useTitleAsBranch ? sessionName : branch,
                                                onChange: (e)=>!useTitleAsBranch && setFormField("branch", e.target.value),
                                                disabled: useTitleAsBranch,
                                                style: {
                                                    opacity: useTitleAsBranch ? 0.6 : 1
                                                }
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: hint,
                                                children: useTitleAsBranch ? "Branch name will be: ".concat(sessionName || "(enter session name)") : "Branch to create for the new worktree"
                                            })
                                        ]
                                    })
                                ]
                            })
                        ]
                    }),
                    sessionType === "new_worktree" && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: Omnibar_css_checkbox,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        type: "checkbox",
                                        checked: useTitleAsBranch,
                                        onChange: (e)=>setFormField("useTitleAsBranch", e.target.checked)
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        children: "Use session name as branch name"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: field,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                        className: label,
                                        htmlFor: "omnibar-branch",
                                        children: [
                                            "Git Branch ",
                                            !useTitleAsBranch && "*"
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        id: "omnibar-branch",
                                        type: "text",
                                        className: fieldInput,
                                        placeholder: useTitleAsBranch ? sessionName || "Enter session name first" : "feature/my-feature",
                                        value: useTitleAsBranch ? sessionName : branch,
                                        onChange: (e)=>!useTitleAsBranch && setFormField("branch", e.target.value),
                                        disabled: useTitleAsBranch,
                                        style: {
                                            opacity: useTitleAsBranch ? 0.6 : 1
                                        }
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: hint,
                                        children: useTitleAsBranch ? "Branch name will be: ".concat(sessionName || "(enter session name)") : "Branch to create for the new worktree"
                                    }),
                                    !isDestinationPreviewLoading && destinationPreviewPath && !destinationPreviewIsExact && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                        className: hint,
                                        children: [
                                            "Will be created under: ",
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("code", {
                                                children: [
                                                    destinationPreviewPath,
                                                    "_<unique-id>"
                                                ]
                                            })
                                        ]
                                    })
                                ]
                            })
                        ]
                    }),
                    sessionType === "existing_worktree" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: field,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: label,
                                htmlFor: "omnibar-existing-worktree",
                                children: "Existing Worktree Path *"
                            }),
                            isWorktreesLoading ? /*#__PURE__*/ (0,jsx_runtime.jsx)("select", {
                                id: "omnibar-existing-worktree",
                                className: Omnibar_css_select,
                                disabled: true,
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                    children: "Loading worktrees…"
                                })
                            }) : worktrees.length > 0 ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                                id: "omnibar-existing-worktree",
                                className: Omnibar_css_select,
                                value: existingWorktree,
                                onChange: (e)=>setFormField("existingWorktree", e.target.value),
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                        value: "",
                                        children: "Select a worktree…"
                                    }),
                                    worktrees.map((wt)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                            value: wt.path,
                                            children: wt.branch ? "".concat(wt.branch, " (").concat(wt.path, ")") : wt.path
                                        }, wt.path))
                                ]
                            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)(RepoPathInput/* RepoPathInput */.E, {
                                id: "omnibar-existing-worktree",
                                placeholder: "/path/to/existing/worktree",
                                value: existingWorktree,
                                onChange: (v)=>setFormField("existingWorktree", v)
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: hint,
                                children: isWorktreesLoading ? "Scanning for git worktrees…" : worktreesError ? "".concat(worktreesError, " — enter the path manually below") : worktrees.length > 0 ? "Select an existing git worktree for this repository" : "Absolute path to an existing git worktree"
                            })
                        ]
                    }),
                    sessionType !== "one_off" && sessionType !== "new_project" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: field,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: label,
                                htmlFor: "omnibar-working-dir",
                                children: "Working Directory"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "omnibar-working-dir",
                                type: "text",
                                className: fieldInput,
                                placeholder: "src/api (optional)",
                                value: workingDir,
                                onChange: (e)=>setFormField("workingDir", e.target.value)
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: hint,
                                children: "Optional: Start in a subdirectory (relative path)"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: attachArea,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                ref: attachInputRef,
                                type: "file",
                                accept: "*/*",
                                multiple: true,
                                style: {
                                    display: "none"
                                },
                                onChange: handleAttachFiles,
                                "aria-hidden": "true"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: attachButton,
                                onClick: ()=>{
                                    var _attachInputRef_current;
                                    return (_attachInputRef_current = attachInputRef.current) === null || _attachInputRef_current === void 0 ? void 0 : _attachInputRef_current.click();
                                },
                                disabled: isAttaching,
                                "aria-label": "Attach files",
                                children: isAttaching ? "⏳ Uploading..." : "📎 Attach files"
                            }),
                            attachError && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: OmnibarCreationPanel_css_attachError,
                                children: attachError
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(FileChipList, {
                        files: attachedFiles,
                        onRemove: removeFile
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: field,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: label,
                                htmlFor: "omnibar-first-prompt",
                                children: [
                                    "First Prompt ",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        style: {
                                            fontWeight: "normal",
                                            opacity: 0.6
                                        },
                                        children: "(optional)"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: textareaWrapper,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("textarea", {
                                        ref: firstPromptRef,
                                        id: "omnibar-first-prompt",
                                        className: fieldInput,
                                        placeholder: "What should Claude do first? Type / for slash commands.",
                                        rows: 3,
                                        maxLength: 2000,
                                        value: formState.firstPrompt,
                                        onChange: (e)=>{
                                            setFormField("firstPrompt", e.target.value);
                                            var _e_target_selectionStart;
                                            setFirstPromptCursor((_e_target_selectionStart = e.target.selectionStart) !== null && _e_target_selectionStart !== void 0 ? _e_target_selectionStart : 0);
                                            setSlashSuggestIndex(-1);
                                        },
                                        onSelect: (e)=>{
                                            var _e_target_selectionStart;
                                            return setFirstPromptCursor((_e_target_selectionStart = e.target.selectionStart) !== null && _e_target_selectionStart !== void 0 ? _e_target_selectionStart : 0);
                                        },
                                        onKeyDown: handleFirstPromptKeyDown,
                                        style: {
                                            resize: "vertical",
                                            fontFamily: "inherit",
                                            fontSize: "inherit"
                                        }
                                    }),
                                    isSlashDropdownVisible && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                        className: slashDropdownWrapper,
                                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(SlashCommandDropdown/* SlashCommandDropdown */.U, {
                                            id: "omnibar-first-prompt-slash-listbox",
                                            suggestions: slashState.suggestions,
                                            selectedIndex: slashSuggestIndex,
                                            onSelect: handleSlashSelect
                                        })
                                    })
                                ]
                            }),
                            formState.firstPrompt.length > 1800 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: hint,
                                style: {
                                    color: "var(--warning)"
                                },
                                children: [
                                    2000 - formState.firstPrompt.length,
                                    " characters remaining"
                                ]
                            })
                        ]
                    }),
                    sessionType === "new_worktree" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: field,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: label,
                                htmlFor: "omnibar-fanout-count",
                                children: [
                                    "Run as variants ",
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                        style: {
                                            fontWeight: "normal",
                                            opacity: 0.6
                                        },
                                        children: [
                                            "(1 = single session, max ",
                                            useFanoutService/* MAX_FANOUT_VARIANTS */.wD,
                                            ")"
                                        ]
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "omnibar-fanout-count",
                                "data-testid": "omnibar-fanout-count",
                                type: "number",
                                min: 1,
                                max: useFanoutService/* MAX_FANOUT_VARIANTS */.wD,
                                className: fieldInput,
                                value: formState.fanoutCount,
                                onChange: (e)=>{
                                    const n = Math.max(1, Math.min(useFanoutService/* MAX_FANOUT_VARIANTS */.wD, Math.floor(Number(e.target.value)) || 1));
                                    setFormField("fanoutCount", n);
                                }
                            }),
                            formState.fanoutCount >= 2 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: hint,
                                role: "status",
                                "data-testid": "omnibar-fanout-cost-warning",
                                children: [
                                    formState.fanoutCount,
                                    " sessions will run simultaneously — monitor cost in Insights"
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: collapsible,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: collapsibleHeader,
                                onClick: ()=>setPresetsOpen((v)=>!v),
                                "aria-expanded": presetsOpen,
                                "data-testid": "preset-section-header",
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: collapsibleTitle,
                                        children: "Presets"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: "".concat(collapsibleIcon, " ").concat(presetsOpen ? expanded : ""),
                                        children: "▼"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: [
                                    advancedSection,
                                    presetsOpen ? advancedSectionOpen : ""
                                ].filter(Boolean).join(" "),
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: collapsibleContent,
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(OmnibarPresetList, {
                                        presets: presets,
                                        loading: presetsLoading,
                                        loadError: presetsLoadError,
                                        onSelect: (preset)=>onPresetSelect === null || onPresetSelect === void 0 ? void 0 : onPresetSelect(preset)
                                    })
                                })
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: collapsible,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: collapsibleHeader,
                                onClick: onToggleAdvanced,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: collapsibleTitle,
                                        children: "Advanced Options"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: "".concat(collapsibleIcon, " ").concat(showAdvanced ? expanded : ""),
                                        children: "▼"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: [
                                    advancedSection,
                                    showAdvanced ? advancedSectionOpen : ""
                                ].filter(Boolean).join(" "),
                                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: collapsibleContent,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: field,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                                    className: label,
                                                    htmlFor: "omnibar-program",
                                                    children: "Program"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("select", {
                                                    id: "omnibar-program",
                                                    className: Omnibar_css_select,
                                                    value: program,
                                                    "aria-describedby": PROGRAM_PROBE_STATUS_ID,
                                                    onChange: (e)=>setFormField("program", e.target.value),
                                                    children: pickerPrograms.map((p)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                            value: p.value,
                                                            children: p.label
                                                        }, p.value))
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)(ProgramProbeSection, {
                                                    option: availablePrograms.find((p)=>p.value === program)
                                                }),
                                                showPiApprovalWarning && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: piApprovalWarning,
                                                    role: "alert",
                                                    "data-testid": "pi-approval-extension-warning",
                                                    children: "Approval extension not loaded for pi — tool calls will run WITHOUT rule enforcement for this session."
                                                })
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: field,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                                    className: label,
                                                    htmlFor: "omnibar-category",
                                                    children: "Category"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                    id: "omnibar-category",
                                                    type: "text",
                                                    className: fieldInput,
                                                    placeholder: "e.g., Features, Bugfixes",
                                                    value: category,
                                                    onChange: (e)=>setFormField("category", e.target.value)
                                                })
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                            className: Omnibar_css_checkbox,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                    type: "checkbox",
                                                    checked: autoYes,
                                                    onChange: (e)=>setFormField("autoYes", e.target.checked)
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    children: "Auto-accept prompts (Enter-key fallback, experimental)"
                                                })
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                            className: Omnibar_css_checkbox,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                    type: "checkbox",
                                                    checked: autoApprove,
                                                    disabled: !(0,sessions_autoApprove/* isAutoApproveSupported */.$)(program),
                                                    onChange: (e)=>setFormField("autoApprove", e.target.checked)
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    children: "⚡ Auto-approve (skip permission prompts)"
                                                })
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: hint,
                                            children: (0,sessions_autoApprove/* isAutoApproveSupported */.$)(program) ? "Skips ALL permission/approval prompts for this agent. Risk of unintended file changes — use only in disposable/sandboxed workspaces." : 'Not supported for "'.concat(program || "this agent", '" yet.')
                                        })
                                    ]
                                })
                            })
                        ]
                    })
                ]
            }),
            error && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: Omnibar_css_error,
                role: "alert",
                "data-testid": "omnibar-create-error",
                children: error
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: footer,
                children: [
                    onCreateBacklog && /*#__PURE__*/ (0,jsx_runtime.jsx)(OmnibarBacklogButton, {
                        onClick: onCreateBacklog,
                        disabled: isSubmitting,
                        className: "".concat(Omnibar_css_button, " ").concat(buttonSecondary, " ").concat(backlogButton)
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: "".concat(Omnibar_css_button, " ").concat(buttonSecondary),
                        onClick: onCancel,
                        children: "Cancel"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        "data-testid": "omnibar-create-session-button",
                        className: "".concat(Omnibar_css_button, " ").concat(buttonPrimary),
                        onClick: onSubmit,
                        disabled: !canSubmit || isSubmitting,
                        children: isSubmitting ? "Creating..." : "Create Session"
                    })
                ]
            })
        ]
    });
}

;// ./src/lib/omnibar/parseSlashCommand.ts
const KNOWN_SLASH_COMMANDS = {
    oneoff: "one_off",
    worktree: "new_worktree",
    dir: "directory",
    existing: "existing_worktree",
    project: "new_project"
};
/**
 * Parses a `/command [remainder]` prefix from the omnibar input.
 * Returns null for any input that isn't a known slash command.
 * Unknown `/foo` inputs fall through to normal detection (e.g. LocalPathDetector).
 *
 * Must be called BEFORE detect() in the debounce effect so the slash prefix
 * is consumed before path detectors can misidentify it.
 */ function parseSlashCommand(input) {
    var _match_;
    const match = /^\/([a-z]+)(?:\s+(.*))?$/i.exec(input.trim());
    if (!match) return null;
    const cmd = match[1].toLowerCase();
    const sessionType = KNOWN_SLASH_COMMANDS[cmd];
    if (!sessionType) return null;
    var _match__trim;
    return {
        sessionType,
        remainder: (_match__trim = (_match_ = match[2]) === null || _match_ === void 0 ? void 0 : _match_.trim()) !== null && _match__trim !== void 0 ? _match__trim : ""
    };
}

;// ./src/lib/omnibar/parseInput.ts

/**
 * Splits bare-text omnibar input on the first `>` character.
 * Only call this when detection type is SessionSearch — never on path or URL inputs.
 *
 * "my feature > implement auth" → { name: "my-feature", firstPrompt: "implement auth" }
 * "my feature"                  → { name: "my-feature", firstPrompt: "" }
 * "> implement auth"            → { name: "", firstPrompt: "implement auth" }
 */ function parseInputWithSeparator(input) {
    const idx = input.indexOf(">");
    if (idx === -1) {
        return {
            name: toSessionSlug(input),
            firstPrompt: ""
        };
    }
    const rawName = input.slice(0, idx);
    const firstPrompt = input.slice(idx + 1).trim();
    return {
        name: toSessionSlug(rawName),
        firstPrompt
    };
}

// EXTERNAL MODULE: ./src/lib/lazy/index.ts + 1 modules
var lazy = __webpack_require__(27980);
// EXTERNAL MODULE: ./src/lib/hooks/useBacklogService.ts
var useBacklogService = __webpack_require__(8570);
// EXTERNAL MODULE: ./src/lib/contexts/notificationContexts.ts
var notificationContexts = __webpack_require__(82514);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect-web@2.1.1+0149f91553ca585e/node_modules/@connectrpc/connect-web/dist/esm/connect-transport.js + 8 modules
var connect_transport = __webpack_require__(46564);
// EXTERNAL MODULE: ./src/gen/session/v1/backlog_pb.ts
var backlog_pb = __webpack_require__(79870);
// EXTERNAL MODULE: ./src/lib/store/backlogItemsSlice.ts
var backlogItemsSlice = __webpack_require__(63939);
;// ./src/lib/hooks/useBacklogCreationNotifier.ts
/* __next_internal_client_entry_do_not_use__ useBacklogCreationNotifier auto */ 









const POLL_INTERVAL_MS = 4000;
// Give up on an item whose triage never settles (e.g. the service restarted mid-run).
const MAX_TRACK_MS = 30 * 60 * 1000;
// CreateBacklogItem publishes no live event, so an open list only learns of a
// new item once it is upserted into the shared store (as the backlog page does).
async function hydrateItemIntoStore(itemId, signal) {
    try {
        const client = (0,promise_client/* createClient */.UU)(backlog_pb/* BacklogService */.jSM, (0,connect_transport/* createConnectTransport */.w)({
            baseUrl: (0,config/* getApiBaseUrl */.h)(),
            interceptors: [
                (0,config/* createAuthInterceptor */._)()
            ]
        }));
        const resp = await client.getBacklogItem({
            itemId
        }, {
            signal
        });
        if (resp.item) store/* store */.M_.dispatch((0,backlogItemsSlice/* upsertItem */.vH)(resp.item));
    } catch (err) {
        console.error("[useBacklogCreationNotifier] failed to hydrate new item into the store:", err);
    }
}
/**
 * Follows backlog items created from the compose widget and raises a
 * notification when triage completes or the AI asks the user a question.
 * Only items passed to trackItem are watched, so items from import or the
 * manual form never notify. Tracking is in-memory and per-tab.
 */ function useBacklogCreationNotifier() {
    const router = (0,nav/* useRouter */.KA)();
    const { addNotification } = (0,notificationContexts/* useNotificationCommands */.RK)();
    const { getBacklogItem } = (0,useBacklogService/* useBacklogService */.gG)();
    const trackedRef = (0,react.useRef)(new Map());
    const timerRef = (0,react.useRef)(null);
    const inFlightRef = (0,react.useRef)(false);
    const mountedRef = (0,react.useRef)(true);
    const abortRef = (0,react.useRef)(null);
    const notify = (0,react.useCallback)((id, headline, title, type)=>{
        addNotification({
            sessionId: "",
            sessionName: "",
            title: "".concat(headline, " — ").concat(title),
            message: "Open the backlog item to review.",
            notificationType: type,
            onView: ()=>router.push("/backlog?item=".concat(id))
        });
    }, [
        addNotification,
        router
    ]);
    const pollOnce = (0,react.useCallback)(async ()=>{
        const tracked = trackedRef.current;
        for (const [id, t] of Array.from(tracked)){
            if (Date.now() - t.startedAt > MAX_TRACK_MS) {
                tracked.delete(id);
                continue;
            }
            let item;
            try {
                item = await getBacklogItem(id);
            } catch (e) {
                continue;
            }
            if (!mountedRef.current) return;
            if (!item) continue;
            if (item.hasPendingGuidanceRequests) {
                if (!t.notifiedGuidance) {
                    t.notifiedGuidance = true;
                    notify(id, "AI needs your input", t.title, "question");
                }
            } else {
                t.notifiedGuidance = false;
            }
            if (!t.notifiedComplete && item.triageStatus === "completed") {
                t.notifiedComplete = true;
                notify(id, "Triage complete", t.title, "task_complete");
            } else if (!t.notifiedComplete && item.triageStatus === "failed") {
                t.notifiedComplete = true;
                notify(id, "Triage failed", t.title, "error");
            }
            if (t.notifiedComplete && !item.hasPendingGuidanceRequests) tracked.delete(id);
        }
        if (tracked.size === 0 && timerRef.current) {
            clearInterval(timerRef.current);
            timerRef.current = null;
        }
    }, [
        getBacklogItem,
        notify
    ]);
    const pollRef = (0,react.useRef)(pollOnce);
    (0,react.useEffect)(()=>{
        pollRef.current = pollOnce;
    }, [
        pollOnce
    ]);
    const trackItem = (0,react.useCallback)((id, title)=>{
        if (!mountedRef.current) return;
        if (abortRef.current) void hydrateItemIntoStore(id, abortRef.current.signal);
        trackedRef.current.set(id, {
            title,
            startedAt: Date.now(),
            notifiedComplete: false,
            notifiedGuidance: false
        });
        if (timerRef.current) return;
        timerRef.current = setInterval(()=>{
            // A slow RPC must not let the next tick start an overlapping pass.
            if (inFlightRef.current) return;
            inFlightRef.current = true;
            void pollRef.current().finally(()=>{
                inFlightRef.current = false;
            });
        }, POLL_INTERVAL_MS);
    }, []);
    (0,react.useEffect)(()=>{
        mountedRef.current = true;
        abortRef.current = new AbortController();
        return ()=>{
            var _abortRef_current;
            mountedRef.current = false;
            (_abortRef_current = abortRef.current) === null || _abortRef_current === void 0 ? void 0 : _abortRef_current.abort();
            if (timerRef.current) clearInterval(timerRef.current);
            timerRef.current = null;
        };
    }, []);
    return {
        trackItem
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fui%2FAliasPalette.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA6VVXbObIBB9z6_gpTPNAxk1vUku98d0UFZlikABbz46_e8dIFFjorHto7B72N1zzropy3Rf2QT9WiGU0-JHZVQrGUGf1HzF-Hv6DpfMXPbrD3-vDANDUKpPyCrB2SgKBlHYUMZbOwbaRqSGnnANvKodQdss0Sd_qD7BlEId8Zkg2jrlz46cuZqgNEm-fKx-rzax3DSUK7h12LqzAIKkkhCBTcUlQYn_0JQxLqvw1eVmIZdxqwU9E1QKCI9TwSuJuYPGElSAdGD8cUX1uIXssr4Df7gdt5yE-KI1VhmCtOIRvK-I1L71FxQc1oOU7YJghFTrBJdAUDZBWDMMw6osLTiCcOb56N76Ft4qlXT4eKVsl4T5Fkr4ju4xY7cNl_jK3SG5g3sLcE9T05DqGSEoHSqCoJozBtKfOTg53F-AEFxbboNYau4AW02LoIijoXrw8O458dMMP60xizX6aVh-gQd95-tehlhA6W5a7urYTw-gGXJ8CHFTMjtPyGy6Mtrf33jcz_AYGw3TdoZKWyrTENRqDaagNphNgHNgwsCjzTbJGzSDHeCUnt8WfbOk5MY6XNRcsCjuAUR0dxf8PjeZbdoXHkw9tPOk7Dps-k9qj3TnyjnVPKFqgJ8H_NmEl0zmQ8DixSooFizl3YuNNh6o1_USsS1ps7fyyCfsRWPNtEl_3v2ubv-G_5nBlN0etvpi3mDxOro-NaX3eDs9pzRd-FvuSitnVvSDNtNN5j3fJVd_Z6AA2CXXffLMdp35eVwVF7bGMxv-Ad1nbc3wCAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FAliasPalette_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA6VVXbObIBB9z6_gpTPNAxk1vUku98d0UFZlikABbz46_e8dIFFjorHto7B72N1zzropy3Rf2QT9WiGU0_JHZVQrGUGf1HzF_Hv6DpfMXPbrD3_vDANDUKpPyCrB2SgKBlHYUMZbOwbaRqSGnnANvKodQdss0Sd_qD7BlEId8Zkg2jrlz46cuZqgNEm_fKx_rzax3DSUK7h12LqzAIKkkhCBTcUlQYn_0JQxLqvw1eVmIZdxqwU9E1QKCI9TwSuJuYPGElSAdGD8cUX1uIXssr4Df7gdt5yE_KI1VhmCtOIRvK_I1L71FxQc1oOU7YJghFTrBJdAUDZBWDMMw6osLTiCcOb56N76Ft4qlXT4eKVsl4T5Fkr4ju4xY7cNl_jK3SG5g3sLcE9T05DqGSEoHSqCoJozBtKfOTg53F_AEFxbboNYau4AW02LoIijoXrw8O458dMMP60xizX6aVh_gQd95_tehlhA6W5a7urYTw_gGXJ8CHFTMjtPyGy6Mtrf33jcz_AYGw3TdoZKWyrTENRqDaagNphNgHNgwsCjzTbJGzSDHeCUnt8WfbOk5MY6XNRcsCjuAUR0dxf8PjeZbdoXHkw9tPOk7Dps_k9qj3TnyjnVPKFqgJ8H_NmEl0zmQ8DixSooFizl3YuNNh6o1_USsS1ps7fyyCfsRWPNtEl_3v2ubv_G_5nBlN0etvpi3mDxOro_NaX3eDs9pzRd_FvuSitnVvSDNtNN5j3fJVd_Z6AA2CXXffLMdp35eVwVF7bGMxv_Ad1nbc3wCAAA_22_7D = __webpack_require__(55293);
;// ./src/components/ui/AliasPalette.css.ts


var copyButton = 'ff17gsd';
var emptyBody = 'ff17gsb';
var emptyExample = 'ff17gsc';
var AliasPalette_css_emptyState = 'ff17gs9';
var emptyTitle = 'ff17gsa';
var errorDetail = 'ff17gsh';
var errorIcon = 'ff17gsf';
var AliasPalette_css_errorState = 'ff17gse';
var errorTitle = 'ff17gsg';
var groupHeader = 'ff17gs8';
var AliasPalette_css_list = 'ff17gs1';
var palette = 'ff17gs0';
var AliasPalette_css_row = 'ff17gs2';
var rowDesc = 'ff17gs5';
var rowMeta = 'ff17gs6';
var rowName = 'ff17gs4';
var rowProgram = 'ff17gs7';
var AliasPalette_css_rowSelected = 'ff17gs3 ff17gs2';
;// ./src/components/ui/AliasPalette.tsx
/* __next_internal_client_entry_do_not_use__ AliasPalette auto */ 


function AliasPalette(param) {
    let { aliases, input, selectedIndex, onSelect, error } = param;
    const partial = input.startsWith("@") ? input.slice(1).toLowerCase() : "";
    const filtered = partial ? aliases.filter((a)=>a.name.toLowerCase().includes(partial) || a.description.toLowerCase().includes(partial) || a.group.toLowerCase().includes(partial)) : aliases;
    const isFiltering = partial.length > 0;
    if (error) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: palette,
            "data-testid": "alias-palette",
            children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                role: "alert",
                "aria-live": "assertive",
                className: AliasPalette_css_errorState,
                "data-testid": "alias-config-error",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: errorIcon,
                        children: "⚠"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: errorTitle,
                                children: "Alias config failed to load"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: errorDetail,
                                children: error.message
                            })
                        ]
                    })
                ]
            })
        });
    }
    if (filtered.length === 0) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: palette,
            "data-testid": "alias-palette",
            children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: AliasPalette_css_emptyState,
                "data-testid": "alias-palette-empty",
                role: "status",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: emptyTitle,
                        children: "No aliases yet"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: emptyBody,
                        children: "Add aliases in Settings → Aliases to launch sessions faster."
                    })
                ]
            })
        });
    }
    if (isFiltering) {
        const activeFiltered = filtered[selectedIndex];
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: palette,
            "data-testid": "alias-palette",
            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                role: "listbox",
                "aria-label": "Alias matches",
                "aria-activedescendant": activeFiltered ? "alias-option-".concat(activeFiltered.name) : undefined,
                className: AliasPalette_css_list,
                children: filtered.map((alias, i)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(AliasRow, {
                        alias: alias,
                        isSelected: i === selectedIndex,
                        onSelect: onSelect
                    }, alias.name))
            })
        });
    }
    const grouped = [];
    const ungrouped = [];
    for (const alias of filtered){
        if (!alias.group) {
            ungrouped.push(alias);
        } else {
            const existing = grouped.find((g)=>g.group === alias.group);
            if (existing) {
                existing.items.push(alias);
            } else {
                grouped.push({
                    group: alias.group,
                    items: [
                        alias
                    ]
                });
            }
        }
    }
    // Build a flat ordered list so selection index is derived explicitly, not via a mutable counter.
    const flatList = [
        ...ungrouped,
        ...grouped.flatMap((g)=>g.items)
    ];
    const activeAlias = flatList[selectedIndex];
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: palette,
        "data-testid": "alias-palette",
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("ul", {
            role: "listbox",
            "aria-label": "Alias palette",
            "aria-activedescendant": activeAlias ? "alias-option-".concat(activeAlias.name) : undefined,
            className: AliasPalette_css_list,
            children: [
                ungrouped.map((alias)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(AliasRow, {
                        alias: alias,
                        isSelected: flatList.indexOf(alias) === selectedIndex,
                        onSelect: onSelect
                    }, alias.name)),
                grouped.map((param)=>{
                    let { group, items } = param;
                    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(react.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("li", {
                                role: "presentation",
                                className: groupHeader,
                                "data-testid": "alias-group-header",
                                children: group.toUpperCase()
                            }),
                            items.map((alias)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(AliasRow, {
                                    alias: alias,
                                    isSelected: flatList.indexOf(alias) === selectedIndex,
                                    onSelect: onSelect
                                }, alias.name))
                        ]
                    }, group);
                })
            ]
        })
    });
}
function AliasRow(param) {
    let { alias, isSelected, onSelect } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
        id: "alias-option-".concat(alias.name),
        role: "option",
        "aria-selected": isSelected,
        "aria-label": "@".concat(alias.name).concat(alias.description ? " — ".concat(alias.description) : "").concat(alias.path ? ", path ".concat(alias.path) : "").concat(alias.program ? ", program ".concat(alias.program) : ""),
        tabIndex: isSelected ? 0 : -1,
        className: isSelected ? AliasPalette_css_rowSelected : AliasPalette_css_row,
        onClick: ()=>onSelect(alias),
        onKeyDown: (e)=>{
            if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                onSelect(alias);
            }
        },
        "data-testid": "alias-row",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                className: rowName,
                children: [
                    "@",
                    alias.name
                ]
            }),
            alias.description && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: rowDesc,
                children: alias.description
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                className: rowMeta,
                children: [
                    alias.path && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        children: alias.path
                    }),
                    alias.program && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: rowProgram,
                        children: [
                            "[",
                            alias.program,
                            "]"
                        ]
                    })
                ]
            })
        ]
    });
}

;// ./src/lib/hooks/useAliasSuggestions.ts

function useAliasSuggestions(input, aliases) {
    const isAliasBrowse = input === "@";
    const isAliasCompletion = /^@[\w-]+$/.test(input) && input.length > 1;
    const filteredAliases = (0,react.useMemo)(()=>{
        if (!isAliasBrowse && !isAliasCompletion) return [];
        const partial = input.slice(1).toLowerCase();
        if (!partial) return aliases;
        return aliases.filter((a)=>a.name.toLowerCase().includes(partial) || a.description.toLowerCase().includes(partial) || a.group.toLowerCase().includes(partial));
    }, [
        input,
        aliases,
        isAliasBrowse,
        isAliasCompletion
    ]);
    function complete(alias) {
        return "@" + alias.name + " ";
    }
    return {
        isAliasBrowse,
        isAliasCompletion,
        filteredAliases,
        complete
    };
}

;// ./src/lib/hooks/useAliases.ts
/* __next_internal_client_entry_do_not_use__ useAliases auto */ 



function useAliases() {
    const [aliases, setAliases] = (0,react.useState)([]);
    const [loading, setLoading] = (0,react.useState)(true);
    const [error, setError] = (0,react.useState)(null);
    const [fetchTick, setFetchTick] = (0,react.useState)(0);
    const clientRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
        let cancelled = false;
        async function fetchAliases() {
            try {
                const resp = await clientRef.current.listAliases({});
                if (!cancelled) {
                    var _resp_aliases;
                    setAliases(((_resp_aliases = resp.aliases) !== null && _resp_aliases !== void 0 ? _resp_aliases : []).map((a)=>{
                        var _a_tags;
                        return {
                            name: a.name,
                            group: a.group,
                            path: a.path,
                            description: a.description,
                            profile: a.profile,
                            program: a.program,
                            autoYes: a.autoYes,
                            tags: [
                                ...(_a_tags = a.tags) !== null && _a_tags !== void 0 ? _a_tags : []
                            ],
                            sessionType: a.sessionType,
                            namePrefix: a.namePrefix
                        };
                    }));
                }
            } catch (err) {
                if (!cancelled) {
                    setError(err instanceof Error ? err : new Error(String(err)));
                }
            } finally{
                if (!cancelled) {
                    setLoading(false);
                }
            }
        }
        void fetchAliases();
        return ()=>{
            cancelled = true;
        };
    }, [
        fetchTick
    ]);
    const refetch = (0,react.useCallback)(()=>setFetchTick((t)=>t + 1), []);
    return {
        aliases,
        loading,
        error,
        refetch
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react-dom/index.js
var react_dom = __webpack_require__(80883);
// EXTERNAL MODULE: ./src/styles/theme.css.ts
var theme_css = __webpack_require__(90377);
;// ./src/components/sessions/PathConfirmDialog.tsx
/* __next_internal_client_entry_do_not_use__ PathConfirmDialog auto */ 


// Directory mode with a non-existent path: confirm before creating it and running
// `git init`. Portaled to document.body, so it competes with the Omnibar overlay
// (zIndex.modal) at page level and must sit above it to be clickable.
function PathConfirmDialog(param) {
    let { path, onCancel, onConfirm } = param;
    return /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        role: "dialog",
        "aria-modal": "true",
        "aria-labelledby": "path-confirm-title",
        "data-testid": "path-confirm-dialog",
        style: {
            position: "fixed",
            inset: 0,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            background: "rgba(0,0,0,0.5)",
            zIndex: theme_css.zIndex.dialog,
            borderRadius: "inherit"
        },
        onClick: (e)=>e.stopPropagation(),
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            style: {
                background: "var(--card-background)",
                border: "1px solid var(--border-color)",
                borderRadius: "8px",
                padding: "24px",
                maxWidth: "420px",
                width: "100%",
                margin: "16px"
            },
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    id: "path-confirm-title",
                    style: {
                        fontWeight: 600,
                        fontSize: "1rem",
                        marginBottom: "8px"
                    },
                    children: "Create directory?"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    style: {
                        fontSize: "0.875rem",
                        color: "var(--text-secondary)",
                        marginBottom: "16px"
                    },
                    children: [
                        "The path",
                        " ",
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                            style: {
                                fontFamily: "monospace",
                                padding: "0 4px"
                            },
                            children: path
                        }),
                        " ",
                        "does not exist. Create it and initialize a git repository?"
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    style: {
                        display: "flex",
                        gap: "8px",
                        justifyContent: "flex-end"
                    },
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            type: "button",
                            style: {
                                padding: "6px 14px",
                                fontSize: "0.875rem",
                                borderRadius: "6px",
                                border: "1px solid var(--border-color)",
                                background: "transparent",
                                color: "var(--text-primary)",
                                cursor: "pointer"
                            },
                            onClick: onCancel,
                            children: "Cancel"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            type: "button",
                            style: {
                                padding: "6px 14px",
                                fontSize: "0.875rem",
                                borderRadius: "6px",
                                border: "none",
                                background: "var(--primary)",
                                color: "var(--primary-text)",
                                cursor: "pointer"
                            },
                            onClick: onConfirm,
                            children: "Create & Open"
                        })
                    ]
                })
            ]
        })
    }), document.body);
}

// EXTERNAL MODULE: ./src/lib/omnibar/recentShellCommands.ts
var recentShellCommands = __webpack_require__(16244);
;// ./src/components/sessions/Omnibar.tsx
/* __next_internal_client_entry_do_not_use__ Omnibar auto */ 



































// Stable identity for callers that omit the `workflows` prop. `workflows` is a
// dependency of the detection debounce effect below — a fresh `[]` literal as
// the default parameter value would get a new identity every render, which
// would restart the 150ms debounce on every render for such callers.
const EMPTY_WORKFLOWS = [];
const Omnibar_EMPTY_PRESETS = [];
const Omnibar_EMPTY_REMOTES = [];
// Lazy-load the backlog chat-compose/intent-review subtree. The app shell renders
// <Omnibar> on every route (src/app/Providers.tsx), so a static import of either
// component here would put their shared dependency, BacklogItemForm.tsx — and
// through it react-markdown/remark-gfm — in the Initial JS chunk, even though these
// views only render once the user actually drafts a backlog item from the omnibar.
const backlogComposeLoading = ()=>/*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        style: {
            padding: "24px",
            textAlign: "center",
            color: "var(--text-secondary)"
        },
        children: "Loading…"
    });
const BacklogItemIntentReview = (0,lazy/* lazyComponent */._)(()=>Promise.all(/* import() */[__webpack_require__.e(1869), __webpack_require__.e(935), __webpack_require__.e(1618), __webpack_require__.e(9231)]).then(__webpack_require__.bind(__webpack_require__, 16137)).then((m)=>({
            default: m.BacklogItemIntentReview
        })), {
    loading: backlogComposeLoading
});
const BacklogComposeWidget = (0,lazy/* lazyComponent */._)(()=>Promise.all(/* import() */[__webpack_require__.e(1869), __webpack_require__.e(7579)]).then(__webpack_require__.bind(__webpack_require__, 67579)).then((m)=>({
            default: m.BacklogComposeWidget
        })), {
    loading: backlogComposeLoading
});
const INITIAL_FORM_STATE = {
    sessionName: "",
    branch: "",
    program: "",
    category: "",
    autoYes: false,
    useTitleAsBranch: true,
    sessionType: "new_worktree",
    existingWorktree: "",
    workingDir: "",
    // New project mode defaults
    parentDir: "",
    projectName: "",
    newProjectSessionType: "new_worktree",
    createIfMissing: false,
    firstPrompt: "",
    fanoutCount: 1,
    autonomousMode: false,
    autoApprove: false,
    extraArgs: [],
    // undefined = local (this machine) — see OmnibarFormState.remoteName's doc comment.
    remoteName: undefined
};
// Validates a project name: no path separators, null bytes, or leading/trailing spaces/dots.
function isValidProjectName(name) {
    if (!name.trim()) return false;
    return !/[/\\<>:"|?*\x00]/.test(name) && !/^\.|\.$|^ | $/.test(name);
}
const RESULT_LISTBOX_ID = "omnibar-result-listbox";
function protoSessionTypeToFormString(st) {
    switch(st){
        case types_pb/* SessionType */.dF.DIRECTORY:
            return "directory";
        case types_pb/* SessionType */.dF.NEW_WORKTREE:
            return "new_worktree";
        case types_pb/* SessionType */.dF.EXISTING_WORKTREE:
            return "existing_worktree";
        case types_pb/* SessionType */.dF.ONE_OFF:
            return "one_off";
        default:
            return "new_worktree";
    }
}
function Omnibar(param) {
    let { isOpen, onClose, onCreateSession, onNavigateToSession, onNavigateToSessionInNewPane, onRunWorkflow, initialMode, initialInput, initialTitle, workflows = EMPTY_WORKFLOWS, launcherPresets = Omnibar_EMPTY_PRESETS, launcherPresetsLoading = false, launcherPresetsLoadError = null, remotes = Omnibar_EMPTY_REMOTES } = param;
    var _detection_metadata, _detection_metadata_alias, _detection_metadata1, _detection_metadata_alias1, _detection_metadata2;
    const router = (0,nav/* useRouter */.KA)();
    const { setTheme } = (0,ThemeContext/* useTheme */.DP)();
    const { createBacklogItemFromChat } = (0,useBacklogService/* useBacklogService */.gG)();
    const { trackItem } = useBacklogCreationNotifier();
    const { addNotification } = (0,notificationContexts/* useNotificationCommands */.RK)();
    const createBacklogItemInBackground = (0,react.useCallback)(async (text)=>{
        let result = null;
        try {
            result = await createBacklogItemFromChat(text);
        } catch (e) {
        // fall through to the failure notification
        }
        if (!result) {
            addNotification({
                sessionId: "",
                sessionName: "",
                title: "Failed to create backlog item",
                message: "Your text was not saved: ".concat(text.length > 200 ? "".concat(text.slice(0, 200), "…") : text),
                notificationType: "error"
            });
            return;
        }
        trackItem(result.item.id, result.item.title);
        addNotification({
            sessionId: "",
            sessionName: "",
            title: "Backlog item created — triage starting",
            message: result.item.title,
            notificationType: "info",
            onView: ()=>router.push("/backlog?item=".concat(result.item.id))
        });
    }, [
        createBacklogItemFromChat,
        trackItem,
        addNotification,
        router
    ]);
    // Input state
    const [input, setInput] = (0,react.useState)("");
    // Debounced copy of input — used for Fuse searches to avoid running O(n log n) work
    // on every keystroke. The visible input still uses the raw value for instant response.
    const [debouncedInput, setDebouncedInput] = (0,react.useState)("");
    const debounceFuseRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        if (debounceFuseRef.current) clearTimeout(debounceFuseRef.current);
        debounceFuseRef.current = setTimeout(()=>setDebouncedInput(input), 150);
        return ()=>{
            if (debounceFuseRef.current) clearTimeout(debounceFuseRef.current);
        };
    }, [
        input
    ]);
    const [detection, setDetection] = (0,react.useState)(null);
    // Consolidated form state
    const [formState, setFormState] = (0,react.useState)(INITIAL_FORM_STATE);
    const setFormField = (0,react.useCallback)((key, value)=>setFormState((prev)=>({
                ...prev,
                [key]: value
            })), []);
    // Consolidated UI state
    const [uiState, setUIState] = (0,react.useState)({
        showAdvanced: false,
        dropdownIndex: -1,
        dropdownDismissed: false,
        resultHighlightIndex: -1,
        atSuggestIndex: -1
    });
    const setUIField = (0,react.useCallback)((key, value)=>setUIState((prev)=>({
                ...prev,
                [key]: value
            })), []);
    const [activeDropdown, setActiveDropdown] = (0,react.useState)(null);
    const [aliasSuggestIndex, setAliasSuggestIndex] = (0,react.useState)(-1);
    // Submission state
    const [isSubmitting, setIsSubmitting] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    // Non-null while reviewing an LLM-parsed backlog item draft (the
    // "backlog: <message>" trigger) — holds the raw text passed to
    // BacklogItemIntentReview. Replaces the entire omnibar body with the
    // review UI until the user confirms or cancels; see the early return
    // near this component's bottom.
    const [backlogReviewText, setBacklogReviewText] = (0,react.useState)(null);
    // Dedicated multi-line compose mode opened by the "Create backlog item" button;
    // seeded from the current input so typed text is never discarded.
    const [backlogComposeText, setBacklogComposeText] = (0,react.useState)(null);
    // Synchronous double-submit guard: `isSubmitting` state only becomes visible to
    // handleSubmit's closure after React commits the re-render, which leaves a window
    // for a second rapid click (e.g. a fast double-click) to read the same stale
    // `isSubmitting === false` closure value. A ref is updated synchronously in the
    // same call stack as the first click, so a second click arriving before React
    // re-renders is still rejected.
    const isSubmittingRef = (0,react.useRef)(false);
    // Mode state machine
    const [modeState, dispatchMode] = useModeReducer();
    // Confirmation dialog state for Directory mode with non-existent path
    const [showPathConfirmation, setShowPathConfirmation] = (0,react.useState)(false);
    const [pendingSessionData, setPendingSessionData] = (0,react.useState)(null);
    // Convenience aliases for existing code
    // Destructure only fields needed for validation/submission logic in Omnibar.tsx
    const { sessionName, program, category, autoYes, autoApprove, sessionType, branch, useTitleAsBranch, existingWorktree, workingDir, parentDir, projectName, newProjectSessionType, createIfMissing } = formState;
    const { showAdvanced } = uiState;
    const { dropdownIndex, dropdownDismissed, resultHighlightIndex, atSuggestIndex } = uiState;
    // Used in detection auto-fill effects
    const setSessionName = (0,react.useCallback)((v)=>setFormField("sessionName", v), [
        setFormField
    ]);
    const setBranch = (0,react.useCallback)((v)=>setFormField("branch", v), [
        setFormField
    ]);
    // Selected launcher preset (list click or typed preset:<id>), drives the resolution chip.
    // Deliberately separate from `detection` state — a list click never touches the text input,
    // so it can't be derived from detection.type the way alias-resolution-chip is.
    const [selectedPreset, setSelectedPreset] = (0,react.useState)(null);
    const handlePresetSelect = (0,react.useCallback)((preset)=>{
        // Unconditional overwrite (no "only if empty" guard) — a discrete list click/typed
        // shorthand resolution is a one-shot action, not continuous re-detection on every
        // keystroke, so the thrashing concern that motivates the alias auto-fill guard doesn't
        // apply here (see plan.md Pattern Decisions).
        const [presetProgram, ...extraArgs] = preset.argv;
        setFormField("program", presetProgram);
        setFormField("extraArgs", extraArgs);
        if (preset.defaultPath) {
            setFormField("workingDir", preset.defaultPath);
        }
        setSelectedPreset(preset);
    }, [
        setFormField
    ]);
    const setDropdownIndex = (0,react.useCallback)((updater)=>{
        setUIState((prev)=>({
                ...prev,
                dropdownIndex: typeof updater === "function" ? updater(prev.dropdownIndex) : updater
            }));
    }, []);
    const setDropdownDismissed = (0,react.useCallback)((v)=>setUIField("dropdownDismissed", v), [
        setUIField
    ]);
    const setResultHighlightIndex = (0,react.useCallback)((updater)=>{
        setUIState((prev)=>({
                ...prev,
                resultHighlightIndex: typeof updater === "function" ? updater(prev.resultHighlightIndex) : updater
            }));
    }, []);
    // Refs
    const inputRef = (0,react.useRef)(null);
    const debounceRef = (0,react.useRef)(null);
    const lastSuggestedNameRef = (0,react.useRef)("");
    const prevDetectionTypeRef = (0,react.useRef)(null);
    // Tracks the input value as of the last time the detection effect actually
    // ran its body, so a reset_to_discovery dispatch only fires when input truly
    // transitioned away from empty — not merely because aliases/workflows finished
    // an async refetch while input was already empty (see reset_to_discovery guard below).
    const prevDetectionInputRef = (0,react.useRef)("");
    // Stable ref so handleKeyDown can always call the latest handleSubmit without
    // a circular declaration-order dependency (handleKeyDown is declared before handleSubmit).
    const handleSubmitRef = (0,react.useRef)(()=>{});
    // Holds the latest attached image paths from OmnibarCreationPanel without causing re-renders.
    const attachedImagePathsRef = (0,react.useRef)([]);
    // Refs for form fields read inside the detection effect — avoids re-triggering
    // the effect (and resetting to discovery mode) when only form fields change.
    const sessionNameRef = (0,react.useRef)(sessionName);
    const branchRef = (0,react.useRef)(branch);
    const programRef = (0,react.useRef)(program);
    (0,react.useEffect)(()=>{
        sessionNameRef.current = sessionName;
    }, [
        sessionName
    ]);
    (0,react.useEffect)(()=>{
        branchRef.current = branch;
    }, [
        branch
    ]);
    (0,react.useEffect)(()=>{
        programRef.current = program;
    }, [
        program
    ]);
    const lastSuggestedProgramRef = (0,react.useRef)("");
    // API base URL for pre-session image uploads — uses shared helper for SSR/dev consistency.
    const uploadBaseUrl = (0,config/* getApiBaseUrl */.h)();
    // Determine whether completions should be active.
    const isPathInput = (detection === null || detection === void 0 ? void 0 : detection.type) === types/* InputType */.N.LocalPath || (detection === null || detection === void 0 ? void 0 : detection.type) === types/* InputType */.N.PathWithBranch;
    // `>shell <dir>` is still typing its directory argument when no `-- command`
    // has been typed yet — offer the same directory completions as a normal path input.
    const shellMetadata = (detection === null || detection === void 0 ? void 0 : detection.type) === types/* InputType */.N.SpawnShell ? detection.metadata : undefined;
    const isShellDirInput = Boolean(shellMetadata && !shellMetadata.shellCommand);
    var _detection_localPath, _shellMetadata_shellDir;
    // Use the detected local path (strips branch suffix for PathWithBranch), or the
    // in-progress `>shell` directory argument.
    const completionPrefix = isPathInput ? (_detection_localPath = detection === null || detection === void 0 ? void 0 : detection.localPath) !== null && _detection_localPath !== void 0 ? _detection_localPath : input : isShellDirInput ? (_shellMetadata_shellDir = shellMetadata === null || shellMetadata === void 0 ? void 0 : shellMetadata.shellDir) !== null && _shellMetadata_shellDir !== void 0 ? _shellMetadata_shellDir : "" : "";
    const isPathCompletionActive = (isPathInput || isShellDirInput) && completionPrefix.length > 0;
    const { entries: completionEntries, baseDir: completionBaseDir, pathExists, isLoading: isCompletionLoading, error: completionError } = (0,usePathCompletions/* usePathCompletions */.C)(completionPrefix, {
        enabled: isPathCompletionActive,
        directoriesOnly: true
    });
    const { getMatching: getHistoryMatching, getAll: getAllHistory, save: saveHistory } = usePathHistory();
    var _detection_localPath1;
    // Worktree suggestions for the "Use Existing Worktree" mode.
    // Prefer the pre-selected repo path from creation_with_repo mode; fall back to the
    // live-detected local path when the user typed a path directly in the input.
    const repoPathForWorktrees = modeState.type === "creation_with_repo" && modeState.path ? modeState.path : isPathInput ? (_detection_localPath1 = detection === null || detection === void 0 ? void 0 : detection.localPath) !== null && _detection_localPath1 !== void 0 ? _detection_localPath1 : "" : "";
    const { worktrees, isLoading: isWorktreesLoading, error: worktreesError } = useWorktreeSuggestions(repoPathForWorktrees, {
        enabled: sessionType === "existing_worktree" && !!repoPathForWorktrees
    });
    // Destination path preview: github_url mode previews the exact clone destination for a
    // detected GitHub URL/PR/shorthand; new_worktree mode previews the deterministic prefix
    // of where the worktree will be created (the real path also gets a random suffix at
    // creation time — see PreviewWorktreePath's doc comment).
    const isGitHubUrlDetection = (detection === null || detection === void 0 ? void 0 : detection.type) === types/* InputType */.N.GitHubPR || (detection === null || detection === void 0 ? void 0 : detection.type) === types/* InputType */.N.GitHubBranch || (detection === null || detection === void 0 ? void 0 : detection.type) === types/* InputType */.N.GitHubRepo || (detection === null || detection === void 0 ? void 0 : detection.type) === types/* InputType */.N.GitHubShorthand;
    const destinationPreviewParams = (0,react.useMemo)(()=>{
        if (isGitHubUrlDetection && (detection === null || detection === void 0 ? void 0 : detection.parsedValue)) {
            return {
                mode: "github_url",
                input: detection.parsedValue
            };
        }
        if (sessionType === "new_worktree" && repoPathForWorktrees && sessionName.trim()) {
            return {
                mode: "new_worktree",
                input: "",
                repoPath: repoPathForWorktrees,
                sessionName: useTitleAsBranch ? sessionName : branch || sessionName
            };
        }
        return null;
    }, [
        isGitHubUrlDetection,
        detection === null || detection === void 0 ? void 0 : detection.parsedValue,
        sessionType,
        repoPathForWorktrees,
        sessionName,
        useTitleAsBranch,
        branch
    ]);
    const { path: destinationPreviewPath, isExact: destinationPreviewIsExact, isLoading: isDestinationPreviewLoading } = useDestinationPathPreview(destinationPreviewParams, {
        enabled: destinationPreviewParams !== null
    });
    // Convert live OS entries to CompletionEntry for type-safe downstream use.
    const liveEntries = (0,react.useMemo)(()=>completionEntries.map((e)=>({
                name: e.name,
                path: e.path,
                isDirectory: e.isDirectory
            })), [
        completionEntries
    ]);
    // History entries matching the current prefix.
    const historyMatches = (0,react.useMemo)(()=>isPathInput ? getHistoryMatching(completionPrefix).map((h)=>({
                name: h.path,
                path: h.path,
                isDirectory: true,
                isHistory: true
            })) : [], [
        isPathInput,
        completionPrefix,
        getHistoryMatching
    ]);
    // Merged entries: history first, then live (deduped against history).
    const mergedEntries = (0,react.useMemo)(()=>{
        const liveDeduped = liveEntries.filter((e)=>!historyMatches.some((h)=>h.path === e.path));
        return [
            ...historyMatches,
            ...liveDeduped
        ];
    }, [
        historyMatches,
        liveEntries
    ]);
    const historyCount = historyMatches.length;
    const isDropdownVisible = isPathCompletionActive && mergedEntries.length > 0 && !dropdownDismissed;
    // Discovery mode derived from modeState
    const isDiscoveryMode = modeState.type === "discovery";
    // @command autocomplete — active while user is typing "@slug" (no space yet)
    const { isAtCommand, suggestions: atSuggestions, complete: completeAtCommand } = useAtCommandSuggestions(input, workflows);
    const isAtDropdownVisible = isDiscoveryMode && isAtCommand;
    const { aliases, error: aliasError, refetch: refetchAliases } = useAliases();
    // Re-fetch aliases each time the omnibar opens so newly created aliases appear immediately.
    const prevIsOpenRef = (0,react.useRef)(false);
    (0,react.useEffect)(()=>{
        if (isOpen && !prevIsOpenRef.current) {
            refetchAliases();
        }
        prevIsOpenRef.current = isOpen;
    }, [
        isOpen,
        refetchAliases
    ]);
    const { isAliasBrowse, isAliasCompletion, filteredAliases, complete: completeAlias } = useAliasSuggestions(input, aliases);
    const isAliasPaletteVisible = isDiscoveryMode && (isAliasBrowse || isAliasCompletion);
    // Session search query uses the debounced input so Fuse only runs after typing pauses.
    const sessionSearchQuery = (0,react.useMemo)(()=>{
        if (!debouncedInput.trim()) return "";
        if ((detection === null || detection === void 0 ? void 0 : detection.type) === types/* InputType */.N.SessionSearch) return debouncedInput;
        // Eagerly treat as session search if input doesn't look like a path or URL
        if (!debouncedInput.startsWith("/") && !debouncedInput.startsWith("~") && !debouncedInput.startsWith("http")) {
            return debouncedInput;
        }
        return "";
    }, [
        debouncedInput,
        detection
    ]);
    const sessionResults = useSessionSearch(sessionSearchQuery);
    const allRepoEntries = (0,react.useMemo)(()=>getAllHistory(50), [
        getAllHistory
    ]);
    const repoFuse = (0,react.useMemo)(()=>new dist_fuse/* default */.A(allRepoEntries, {
            keys: [
                {
                    name: "path",
                    weight: 1.0
                }
            ],
            threshold: 0.4,
            ignoreLocation: true,
            minMatchCharLength: 1
        }), [
        allRepoEntries
    ]);
    const activeSortedSessions = (0,store/* useAppSelector */.GV)(sessionsSlice/* selectActiveSessionsSortedByUpdatedAt */.od);
    const displayedSessionResults = (0,react.useMemo)(()=>{
        if (!debouncedInput.trim()) {
            return activeSortedSessions.slice(0, 5).map((s)=>({
                    session: s,
                    score: 0,
                    matchedFields: []
                }));
        }
        return sessionResults;
    }, [
        debouncedInput,
        sessionResults,
        activeSortedSessions
    ]);
    const displayedRepoEntries = (0,react.useMemo)(()=>{
        if (!debouncedInput.trim()) {
            return allRepoEntries.slice(0, 5);
        }
        return repoFuse.search(debouncedInput).map((r)=>r.item).slice(0, 8);
    }, [
        debouncedInput,
        allRepoEntries,
        repoFuse
    ]);
    const totalResultCount = getResultListItemCount(displayedSessionResults.length, displayedRepoEntries.length, true);
    // Accept a completion entry: fill the input and continue for further completion.
    const handleCompletionSelect = (0,react.useCallback)((entry)=>{
        var _inputRef_current;
        const newPath = entry.isDirectory ? entry.path + "/" : entry.path;
        const newInput = isShellDirInput ? ">shell ".concat(newPath) : newPath;
        setInput(newInput);
        setDropdownIndex(-1);
        setDropdownDismissed(false);
        (_inputRef_current = inputRef.current) === null || _inputRef_current === void 0 ? void 0 : _inputRef_current.focus();
    }, [
        isShellDirInput,
        setDropdownIndex,
        setDropdownDismissed
    ]);
    // Detect input type with debouncing
    (0,react.useEffect)(()=>{
        if (debounceRef.current) {
            clearTimeout(debounceRef.current);
        }
        debounceRef.current = setTimeout(()=>{
            try {
                if (input.trim()) {
                    // Pre-process slash commands before detection so /oneoff etc. aren't
                    // misidentified as local paths by LocalPathDetector (priority 100).
                    const slashCmd = parseSlashCommand(input);
                    if (slashCmd) {
                        setFormField("sessionType", slashCmd.sessionType);
                    }
                    const detectInput = slashCmd ? slashCmd.remainder : input;
                    const result = detect(detectInput || input);
                    setDetection(result);
                    // Reset dropdown dismissed state when input type changes modes.
                    // Prevents session results from being suppressed after user dismisses
                    // path completion dropdown then backspaces to bare text.
                    if (result.type !== prevDetectionTypeRef.current) {
                        setDropdownDismissed(false);
                    }
                    prevDetectionTypeRef.current = result.type;
                    // Update mode based on detection type
                    if (result.type === types/* InputType */.N.NewSession) {
                        // "new/" prefix typed → creation_with_repo mode with query from parsedValue
                        dispatchMode({
                            kind: "new_prefix_typed",
                            query: result.parsedValue
                        });
                    } else {
                        dispatchMode({
                            kind: "detect",
                            detection: result
                        });
                        if (result.type === types/* InputType */.N.SessionSearch) {
                            setResultHighlightIndex(-1);
                        }
                    }
                    // Auto-fill session name (and firstPrompt for `>` separator) if:
                    // 1. Session name is empty, OR
                    // 2. Session name matches the last auto-suggested name (not manually edited)
                    // This allows suggestions to update as the user types the path (e.g., "~" → "sqlway")
                    if (result.type === types/* InputType */.N.SessionSearch && !slashCmd) {
                        // Derive-on-read: split on first `>` to populate name + firstPrompt
                        const parsed = parseInputWithSeparator(input);
                        const derivedName = parsed.name;
                        if (derivedName && (!sessionNameRef.current || sessionNameRef.current === lastSuggestedNameRef.current)) {
                            setSessionName(derivedName);
                            lastSuggestedNameRef.current = derivedName;
                        }
                        if (parsed.firstPrompt) {
                            setFormField("firstPrompt", parsed.firstPrompt);
                        }
                    } else if (result.suggestedName && result.type !== types/* InputType */.N.Alias && result.type !== types/* InputType */.N.Command && result.type !== types/* InputType */.N.SpawnShell) {
                        // Skip for Alias — the alias-specific block below handles name population
                        // (running both would reset lastSuggestedNameRef mid-effect and cause oscillation).
                        // Skip for Command/SpawnShell — their suggestedName is a human-readable label
                        // ("Switch to Matrix theme"), not a session-name candidate.
                        if (!sessionNameRef.current || sessionNameRef.current === lastSuggestedNameRef.current) {
                            // Detectors like LocalPath/PathWithBranch/NewSession/GitHub* build suggestedName
                            // from raw path segments, branch names, or free text — slug it here so every
                            // detector gets a valid kebab-case name for free instead of each one remembering to.
                            const slugged = toSessionSlug(result.suggestedName);
                            setSessionName(slugged);
                            lastSuggestedNameRef.current = slugged;
                        }
                    }
                    // Auto-fill branch if detected
                    if (result.branch && !branchRef.current) {
                        setBranch(result.branch);
                    }
                    // When an alias resolves, populate form fields from its configured defaults
                    // so the user can see (and optionally adjust) what will be created.
                    if (result.type === types/* InputType */.N.Alias) {
                        const aliasMeta = result.metadata;
                        const alias = aliasMeta === null || aliasMeta === void 0 ? void 0 : aliasMeta.alias;
                        if (alias) {
                            if (alias.program && (!programRef.current || programRef.current === lastSuggestedProgramRef.current)) {
                                setFormField("program", alias.program);
                                lastSuggestedProgramRef.current = alias.program;
                            }
                            // UNSPECIFIED means "Default (directory)" in the alias editor — always apply it.
                            // Skipping UNSPECIFIED left the form at its initial "new_worktree" default.
                            const resolvedSessionType = alias.sessionType !== types_pb/* SessionType */.dF.UNSPECIFIED ? protoSessionTypeToFormString(alias.sessionType) : "directory";
                            setFormField("sessionType", resolvedSessionType);
                            setFormField("autoYes", alias.autoYes);
                        }
                        // Populate branch from @alias:branch syntax so the user can see/edit it.
                        if ((aliasMeta === null || aliasMeta === void 0 ? void 0 : aliasMeta.branch) && !branchRef.current) {
                            setBranch(aliasMeta.branch);
                        }
                        // If the user typed a label after the alias name (e.g. "@ssq my-feature"),
                        // use it as the session name so they can see and edit it before submitting.
                        // If the alias defines a name_prefix, prepend it (e.g. prefix "ssq-" → "ssq-my-feature").
                        const typedLabel = aliasMeta === null || aliasMeta === void 0 ? void 0 : aliasMeta.label;
                        var _alias_namePrefix;
                        const namePrefix = (_alias_namePrefix = alias === null || alias === void 0 ? void 0 : alias.namePrefix) !== null && _alias_namePrefix !== void 0 ? _alias_namePrefix : "";
                        if (typedLabel && (!sessionNameRef.current || sessionNameRef.current === lastSuggestedNameRef.current)) {
                            // Slug the typed portion only — speech-to-text and free typing won't
                            // produce dashes on their own (e.g. "deck drainage" → "deck-drainage").
                            const suggested = namePrefix ? "".concat(namePrefix).concat(toSessionSlug(typedLabel)) : toSessionSlug(typedLabel);
                            setSessionName(suggested);
                            lastSuggestedNameRef.current = suggested;
                        } else if (namePrefix && (!sessionNameRef.current || sessionNameRef.current === lastSuggestedNameRef.current)) {
                            // No label yet but prefix defined: show just the prefix so the user knows what to complete.
                            setSessionName(namePrefix);
                            lastSuggestedNameRef.current = namePrefix;
                        }
                    } else if (result.type === types/* InputType */.N.Preset) {
                        // Typed preset:<id> shorthand resolves through the same handler as a list click,
                        // so both entry points produce identical form-state effects (single code path).
                        const presetMeta = result.metadata;
                        if (presetMeta === null || presetMeta === void 0 ? void 0 : presetMeta.preset) {
                            handlePresetSelect(presetMeta.preset);
                        }
                    }
                } else {
                    // This branch also runs when aliases/workflows finish an async refetch
                    // while input is (and already was) empty — that's an identity change in
                    // this effect's dep array, not a real input edit. Only reset to discovery
                    // when input actually transitioned from non-empty to empty, so it doesn't
                    // stomp a mode the user reached another way (e.g. Ctrl+Shift+K creation
                    // mode) before they've typed anything.
                    if (prevDetectionInputRef.current.trim()) {
                        setDetection(null);
                        dispatchMode({
                            kind: "reset_to_discovery"
                        });
                        setResultHighlightIndex(-1);
                    }
                }
                prevDetectionInputRef.current = input;
            } catch (err) {
                // Never let a thrown exception mid-update abandon UI state — leave the
                // last valid detection/canSubmit state in place and surface the failure.
                console.error("[Omnibar] input detection failed:", err);
                setError(err instanceof Error ? err.message : "Failed to process input");
            }
        }, 150); // 150ms debounce
        return ()=>{
            if (debounceRef.current) {
                clearTimeout(debounceRef.current);
            }
        };
    // aliases/workflows are included so detection re-evaluates once the async
    // alias/workflow lists resolve — otherwise a fast "@alias" typed before the
    // AliasDetector/WorkflowDetector registers permanently mis-detects (e.g. as
    // SessionSearch) with no keystroke left to trigger a re-run.
    }, [
        input,
        dispatchMode,
        setFormField,
        setBranch,
        setDropdownDismissed,
        setResultHighlightIndex,
        setSessionName,
        aliases,
        workflows,
        handlePresetSelect
    ]);
    // Clear a stale submission error whenever the input value itself changes —
    // keyed on `input` alone (not the detection-debounce effect's deps, which
    // also include aliases/workflows) so an unrelated async refetch can't wipe
    // a just-shown error. Covers every setInput() call site (typing, recent
    // shell command chips, alias/at-command completion, clone session), not
    // just the <input>'s own onChange.
    (0,react.useEffect)(()=>{
        setError(null);
    }, [
        input
    ]);
    // Focus input when opened
    (0,react.useEffect)(()=>{
        if (isOpen && inputRef.current) {
            inputRef.current.focus();
        }
    }, [
        isOpen
    ]);
    // Reset state when closed
    (0,react.useEffect)(()=>{
        if (!isOpen) {
            setInput("");
            setDetection(null);
            setFormState(INITIAL_FORM_STATE);
            setUIState({
                showAdvanced: false,
                dropdownIndex: -1,
                dropdownDismissed: false,
                resultHighlightIndex: -1,
                atSuggestIndex: -1
            });
            setError(null);
            setBacklogReviewText(null);
            setBacklogComposeText(null);
            // Defense-in-depth: handleSubmit's own finally blocks already reset this on
            // success/failure, but this instance never unmounts across open/close cycles, so
            // also clear it here in case onClose() didn't synchronously flip isOpen after a
            // submission (the original bug this guards against — see Omnibar.submitReset.test.tsx).
            setIsSubmitting(false);
            isSubmittingRef.current = false;
            lastSuggestedNameRef.current = "";
            prevDetectionTypeRef.current = null;
            dispatchMode({
                kind: "reset_to_discovery"
            });
        }
    }, [
        isOpen,
        dispatchMode
    ]);
    // Reset atSuggestIndex when leaving @ mode (user added space or cleared the @).
    (0,react.useEffect)(()=>{
        if (!isAtCommand) {
            setUIField("atSuggestIndex", -1);
        }
    }, [
        isAtCommand,
        setUIField
    ]);
    (0,react.useEffect)(()=>{
        if (isAliasPaletteVisible) {
            setActiveDropdown("alias");
        } else if (isAtDropdownVisible) {
            setActiveDropdown("workflow");
        } else {
            setActiveDropdown(null);
        }
    }, [
        isAliasPaletteVisible,
        isAtDropdownVisible
    ]);
    // On open: apply initialMode if provided
    (0,react.useEffect)(()=>{
        if (isOpen && initialMode === "creation") {
            dispatchMode({
                kind: "open_creation_direct"
            });
        }
    }, [
        isOpen,
        initialMode,
        dispatchMode
    ]);
    // On open: pre-populate input if initialInput is provided
    (0,react.useEffect)(()=>{
        if (isOpen && initialInput) {
            setInput(initialInput);
        }
    }, [
        isOpen,
        initialInput
    ]);
    // On open: pre-populate sessionName if initialTitle is provided
    (0,react.useEffect)(()=>{
        if (isOpen && initialTitle) {
            setSessionName(initialTitle);
        }
    }, [
        isOpen,
        initialTitle,
        setSessionName
    ]);
    // Session result selection handlers
    const handleSessionSelect = (0,react.useCallback)((session)=>{
        onNavigateToSession(session.id);
        onClose();
    }, [
        onNavigateToSession,
        onClose
    ]);
    const handleSessionSelectInNewPane = (0,react.useCallback)((session)=>{
        onNavigateToSessionInNewPane === null || onNavigateToSessionInNewPane === void 0 ? void 0 : onNavigateToSessionInNewPane(session.id);
        onClose();
    }, [
        onNavigateToSessionInNewPane,
        onClose
    ]);
    const handleCloneSession = (0,react.useCallback)((session)=>{
        // Pre-fill the input with the source session's repo (not its worktree, which
        // is unique to that session) and switch to creation mode.
        if (session.repoRoot) {
            var _inputRef_current;
            setInput(session.repoRoot);
            dispatchMode({
                kind: "select_repo",
                path: session.repoRoot
            });
            setResultHighlightIndex(-1);
            setDropdownDismissed(false);
            (_inputRef_current = inputRef.current) === null || _inputRef_current === void 0 ? void 0 : _inputRef_current.focus();
        }
    }, [
        dispatchMode,
        setDropdownDismissed,
        setResultHighlightIndex
    ]);
    const handleRepoSelect = (0,react.useCallback)((path)=>{
        var _inputRef_current;
        setInput(path + "/");
        dispatchMode({
            kind: "select_repo",
            path
        });
        setResultHighlightIndex(-1);
        setDropdownDismissed(false);
        (_inputRef_current = inputRef.current) === null || _inputRef_current === void 0 ? void 0 : _inputRef_current.focus();
    }, [
        dispatchMode,
        setDropdownDismissed,
        setResultHighlightIndex
    ]);
    // Seeds compose mode from the typed input, minus a leading "backlog:" trigger,
    // so the prefix never ends up in the created item.
    const openBacklogCompose = (0,react.useCallback)(()=>{
        setBacklogComposeText(input.trim().replace(/^backlog:\s*/i, ""));
        setResultHighlightIndex(-1);
    }, [
        input,
        setResultHighlightIndex
    ]);
    const dispatchHighlightedResultAction = (0,react.useCallback)((index)=>{
        if (index < displayedSessionResults.length) {
            handleSessionSelect(displayedSessionResults[index].session);
        } else {
            const repoIndex = index - displayedSessionResults.length;
            if (repoIndex < displayedRepoEntries.length) {
                handleRepoSelect(displayedRepoEntries[repoIndex].path);
            } else if (repoIndex === displayedRepoEntries.length) {
                dispatchMode({
                    kind: "open_creation_direct"
                });
                setResultHighlightIndex(-1);
            } else {
                openBacklogCompose();
            }
        }
    }, [
        displayedSessionResults,
        displayedRepoEntries,
        handleSessionSelect,
        handleRepoSelect,
        dispatchMode,
        setResultHighlightIndex,
        openBacklogCompose
    ]);
    // Handle keyboard shortcuts
    const handleKeyDown = (0,react.useCallback)((e)=>{
        // Alias palette keyboard navigation
        if (activeDropdown === "alias" && filteredAliases.length > 0) {
            if (e.key === "ArrowDown") {
                e.preventDefault();
                setAliasSuggestIndex((i)=>Math.min(i + 1, filteredAliases.length - 1));
                return;
            }
            if (e.key === "ArrowUp") {
                e.preventDefault();
                setAliasSuggestIndex((i)=>Math.max(i - 1, -1));
                return;
            }
            if (e.key === "Tab" || e.key === "Enter") {
                e.preventDefault();
                // Default to the top suggestion when nothing has been arrow-key-highlighted,
                // matching Tab's fallback so a bare Enter accepts the best match.
                const idx = aliasSuggestIndex >= 0 ? aliasSuggestIndex : 0;
                if (filteredAliases[idx]) {
                    setInput(completeAlias(filteredAliases[idx]));
                    setAliasSuggestIndex(-1);
                }
                return;
            }
            if (e.key === "Escape") {
                e.preventDefault();
                if (input.length > 1) {
                    setInput("@");
                } else {
                    setInput("");
                }
                setAliasSuggestIndex(-1);
                return;
            }
        }
        // @command autocomplete (highest priority when visible)
        if (isAtDropdownVisible) {
            if (e.key === "ArrowDown") {
                e.preventDefault();
                setUIField("atSuggestIndex", Math.min(atSuggestIndex + 1, atSuggestions.length - 1));
                return;
            }
            if (e.key === "ArrowUp") {
                e.preventDefault();
                setUIField("atSuggestIndex", Math.max(atSuggestIndex - 1, -1));
                return;
            }
            if (e.key === "Tab" || e.key === "Enter") {
                // Default to the top suggestion when nothing has been arrow-key-highlighted,
                // matching Tab's existing fallback so a bare Enter accepts the best match.
                const idx = atSuggestIndex >= 0 ? atSuggestIndex : 0;
                if (atSuggestions[idx]) {
                    e.preventDefault();
                    setInput(completeAtCommand(atSuggestions[idx]));
                    setUIField("atSuggestIndex", -1);
                }
                return;
            }
            if (e.key === "Escape") {
                e.nativeEvent.stopImmediatePropagation();
                setUIField("atSuggestIndex", -1);
                return;
            }
        }
        // Discovery mode navigation (before dropdown check)
        if (isDiscoveryMode && (resultHighlightIndex >= 0 || e.key === "ArrowDown")) {
            if (e.key === "ArrowDown") {
                e.preventDefault();
                setResultHighlightIndex((i)=>Math.min(i + 1, totalResultCount - 1));
                return;
            }
            if (e.key === "ArrowUp") {
                e.preventDefault();
                setResultHighlightIndex((i)=>Math.max(i - 1, -1));
                return;
            }
            if (e.key === "Enter" && !e.metaKey && resultHighlightIndex >= 0) {
                e.preventDefault();
                dispatchHighlightedResultAction(resultHighlightIndex);
                return;
            }
            if (e.key === "Escape" && resultHighlightIndex >= 0) {
                e.nativeEvent.stopImmediatePropagation();
                setResultHighlightIndex(-1);
                return;
            }
        }
        if (isDropdownVisible) {
            if (e.key === "ArrowDown") {
                e.preventDefault();
                setDropdownIndex((i)=>Math.min(i + 1, mergedEntries.length - 1));
                return;
            }
            if (e.key === "ArrowUp") {
                e.preventDefault();
                setDropdownIndex((i)=>Math.max(i - 1, -1));
                return;
            }
            if (e.key === "Tab") {
                e.preventDefault();
                if (dropdownIndex >= 0) {
                    // Explicit selection (including history entries) → accept it.
                    handleCompletionSelect(mergedEntries[dropdownIndex]);
                } else if (liveEntries.length === 1) {
                    handleCompletionSelect(liveEntries[0]);
                } else if (liveEntries.length > 1) {
                    // Extend input to longest common prefix of live entry names only.
                    const lcp = liveEntries.reduce((acc, entry)=>{
                        let i = 0;
                        while(i < acc.length && i < entry.name.length && acc[i] === entry.name[i])i++;
                        return acc.slice(0, i);
                    }, liveEntries[0].name);
                    if (lcp) {
                        const sep = completionBaseDir.endsWith("/") ? "" : "/";
                        const newPath = completionBaseDir + sep + lcp;
                        setInput(isShellDirInput ? ">shell ".concat(newPath) : newPath);
                        setDropdownDismissed(false);
                    }
                }
                return;
            }
            if (e.key === "Enter" && !e.metaKey && dropdownIndex >= 0) {
                e.preventDefault();
                handleCompletionSelect(mergedEntries[dropdownIndex]);
                return;
            }
            if (e.key === "Escape") {
                // Stop the native event so the global document listener doesn't
                // also call onClose() — first Escape dismisses the dropdown only.
                e.nativeEvent.stopImmediatePropagation();
                setDropdownDismissed(true);
                setDropdownIndex(-1);
                return;
            }
        }
        // Tab cycles session type when in creation mode and no dropdown is open
        if (e.key === "Tab" && !isDiscoveryMode && !isDropdownVisible) {
            e.preventDefault();
            const types = SESSION_TYPES.map((t)=>t.value);
            const idx = types.indexOf(sessionType);
            const next = types[(idx + 1) % types.length];
            setFormField("sessionType", next);
            return;
        }
        if (e.key === "Escape") {
            // Stop propagation so the global document listener doesn't call onClose() a second time.
            e.nativeEvent.stopImmediatePropagation();
            if (!isDiscoveryMode) {
                // Escape in creation mode: return to discovery rather than closing.
                // Second Escape (in discovery mode) will close.
                dispatchMode({
                    kind: "reset_to_discovery"
                });
                setInput("");
                setResultHighlightIndex(-1);
            } else {
                onClose();
            }
        } else if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
            // Cmd+Enter (Mac) / Ctrl+Enter (Linux/Windows) to submit.
            handleSubmitRef.current();
        } else if (e.key === "Enter") {
            // Plain Enter submits when the form is ready (creation mode), or fires a
            // fire-and-forget detection like @workflow invocation (discovery mode) —
            // reached here only when no result/dropdown entry consumed Enter above.
            // handleSubmit guards on canSubmit internally, so this is a no-op when the form is incomplete.
            handleSubmitRef.current();
        }
    }, [
        isDiscoveryMode,
        resultHighlightIndex,
        totalResultCount,
        dispatchHighlightedResultAction,
        isDropdownVisible,
        mergedEntries,
        liveEntries,
        completionBaseDir,
        isShellDirInput,
        dropdownIndex,
        handleCompletionSelect,
        onClose,
        dispatchMode,
        sessionType,
        setFormField,
        setDropdownDismissed,
        setDropdownIndex,
        setResultHighlightIndex,
        isAtDropdownVisible,
        atSuggestIndex,
        atSuggestions,
        completeAtCommand,
        setUIField,
        activeDropdown,
        aliasSuggestIndex,
        filteredAliases,
        completeAlias,
        input
    ]);
    // Leaving compose mode (cancel/Escape) remounts the input; put focus back on it.
    const wasComposingRef = (0,react.useRef)(false);
    (0,react.useEffect)(()=>{
        var _inputRef_current;
        const composing = backlogComposeText !== null;
        if (wasComposingRef.current && !composing && isOpen) (_inputRef_current = inputRef.current) === null || _inputRef_current === void 0 ? void 0 : _inputRef_current.focus();
        wasComposingRef.current = composing;
    }, [
        backlogComposeText,
        isOpen
    ]);
    // Global keyboard handler
    (0,react.useEffect)(()=>{
        const handleGlobalKeyDown = (e)=>{
            // Cmd+K or Ctrl+K to open (handled by parent)
            // Compose mode handles its own Escape (returns to the input, keeping the draft).
            if (isOpen && e.key === "Escape" && backlogComposeText === null) {
                onClose();
            }
        };
        document.addEventListener("keydown", handleGlobalKeyDown);
        return ()=>document.removeEventListener("keydown", handleGlobalKeyDown);
    }, [
        isOpen,
        onClose,
        backlogComposeText
    ]);
    // Get type info for display
    const typeInfo = (0,react.useMemo)(()=>{
        if (!detection) return types/* INPUT_TYPE_INFO */.P[types/* InputType */.N.Unknown];
        return types/* INPUT_TYPE_INFO */.P[detection.type];
    }, [
        detection
    ]);
    // True only after path completion has resolved and the path is missing.
    // Also requires that we're working with a local path (GitHub URLs are
    // resolved server-side, so existence isn't meaningful here).
    const pathDoesNotExist = isPathInput && !isCompletionLoading && pathExists === false;
    // Check if we can submit
    const canSubmit = (0,react.useMemo)(()=>{
        var _formState_firstPrompt, _detection_metadata;
        // One-off mode: only session name is required (no path needed).
        if (sessionType === "one_off") {
            return !!sessionName.trim();
        }
        // Autonomous mode composes with whichever sessionType is selected (checked below)
        // rather than replacing it — its only extra requirement is a goal, since that's
        // the orchestrator's sole signal of what to do.
        const autonomousNeedsGoal = formState.autonomousMode && !((_formState_firstPrompt = formState.firstPrompt) === null || _formState_firstPrompt === void 0 ? void 0 : _formState_firstPrompt.trim());
        // New project mode: requires parentDir + projectName (valid), no path detection.
        if (sessionType === "new_project") {
            if (!sessionName.trim()) return false;
            if (!parentDir.trim()) return false;
            if (!projectName.trim()) return false;
            if (!isValidProjectName(projectName)) return false;
            if (newProjectSessionType === "new_worktree" && !useTitleAsBranch && !branch.trim()) return false;
            return !autonomousNeedsGoal;
        }
        // Recognized commands (>theme ..., >go ...) and spawn_shell are always submittable
        if ((detection === null || detection === void 0 ? void 0 : detection.type) === types/* InputType */.N.Command && detection.confidence === 1.0) return true;
        if ((detection === null || detection === void 0 ? void 0 : detection.type) === types/* InputType */.N.SpawnShell && detection.confidence === 1.0) return true;
        // Chat backlog item creation (backlog: <message>) needs no sessionName/path.
        if ((detection === null || detection === void 0 ? void 0 : detection.type) === types/* InputType */.N.ChatBacklogItem) return true;
        // fanout N: <prompt> only prefills the variants count + first prompt; always submittable.
        if ((detection === null || detection === void 0 ? void 0 : detection.type) === types/* InputType */.N.FanOut) return true;
        // Workflow invocation (@slug [arg]) is fire-and-forget, no sessionName/path needed.
        if ((detection === null || detection === void 0 ? void 0 : detection.type) === types/* InputType */.N.Workflow && ((_detection_metadata = detection.metadata) === null || _detection_metadata === void 0 ? void 0 : _detection_metadata.workflowFound)) return true;
        if (!input.trim()) return false;
        if (!sessionName.trim()) return false;
        if (!detection || detection.type === types/* InputType */.N.Unknown || detection.type === types/* InputType */.N.Command || detection.type === types/* InputType */.N.SessionSearch || detection.type === types/* InputType */.N.AliasNotFound || detection.type === types/* InputType */.N.AliasBrowse) return false;
        if (detection.type === types/* InputType */.N.Alias) return true;
        // Validate session type specific requirements
        if (sessionType === "new_worktree") {
            // Branch is required unless using title as branch
            if (!useTitleAsBranch && !branch.trim()) return false;
        } else if (sessionType === "existing_worktree") {
            // Existing worktree path is required
            if (!existingWorktree.trim()) return false;
            // existing_worktree requires the parent repo path to actually exist
            if (pathDoesNotExist) return false;
        }
        // For directory / new_worktree: missing path requires explicit opt-in
        if (pathDoesNotExist && (sessionType === "directory" || sessionType === "new_worktree") && !createIfMissing) {
            return false;
        }
        return !autonomousNeedsGoal;
    }, [
        input,
        sessionName,
        detection,
        sessionType,
        branch,
        useTitleAsBranch,
        existingWorktree,
        pathDoesNotExist,
        createIfMissing,
        parentDir,
        projectName,
        newProjectSessionType,
        formState.autonomousMode,
        formState.firstPrompt
    ]);
    // Handle form submission
    const handleSubmit = (0,react.useCallback)(async ()=>{
        var _detection_metadata, _detection_metadata1;
        if (!canSubmit || isSubmitting || isSubmittingRef.current) return;
        isSubmittingRef.current = true;
        // Computed once and reused across every session-data branch below that carries a
        // selected preset's extraArgs through to the RPC.
        const extraArgsForSubmit = formState.extraArgs.length > 0 ? formState.extraArgs : undefined;
        // Execute omnibar commands (>theme ..., >go ...) immediately without entering
        // session-creation flow. These are fire-and-forget; no loading state needed.
        if ((detection === null || detection === void 0 ? void 0 : detection.type) === types/* InputType */.N.Command && detection.confidence === 1.0 && detection.metadata) {
            const { commandType, commandArg } = detection.metadata;
            if (commandType === "theme") {
                setTheme(commandArg);
            } else if (commandType === "navigate") {
                router.push(commandArg);
            }
            onClose();
            return;
        }
        // Chat backlog item creation (backlog: <message>) — opens the LLM-parsed
        // review UI instead of creating immediately; see backlogReviewText's
        // early-return render branch below. isSubmittingRef is cleared here (not
        // in a finally block) since control doesn't return to this function once
        // the review UI takes over.
        if ((detection === null || detection === void 0 ? void 0 : detection.type) === types/* InputType */.N.ChatBacklogItem) {
            isSubmittingRef.current = false;
            setBacklogComposeText(null);
            setBacklogReviewText(detection.parsedValue);
            return;
        }
        // fanout N: <prompt> — prefill the variants input and first prompt, then let the user
        // pick the repo path and submit through the normal creation flow.
        if ((detection === null || detection === void 0 ? void 0 : detection.type) === types/* InputType */.N.FanOut) {
            var _detection_metadata2;
            var _detection_metadata_fanoutCount;
            const count = Number((_detection_metadata_fanoutCount = (_detection_metadata2 = detection.metadata) === null || _detection_metadata2 === void 0 ? void 0 : _detection_metadata2.fanoutCount) !== null && _detection_metadata_fanoutCount !== void 0 ? _detection_metadata_fanoutCount : 2);
            setFormField("fanoutCount", count);
            setFormField("firstPrompt", detection.parsedValue);
            setFormField("sessionType", "new_worktree");
            setInput("");
            isSubmittingRef.current = false;
            return;
        }
        // Spawn shell command (>shell [dir] [-- command]) — creates a real terminal session
        // via the standard session-creation flow, rooted at `shellDir` (if given) and running
        // `shellCommand` as the session program (defaults to an interactive shell).
        if ((detection === null || detection === void 0 ? void 0 : detection.type) === types/* InputType */.N.SpawnShell && detection.confidence === 1.0) {
            var _detection_metadata3;
            const { shellDir, shellCommand } = (_detection_metadata3 = detection.metadata) !== null && _detection_metadata3 !== void 0 ? _detection_metadata3 : {};
            const sessionData = {
                title: shellCommand ? shellCommand : shellDir ? "Terminal: ".concat(shellDir) : "Terminal",
                path: shellDir !== null && shellDir !== void 0 ? shellDir : "",
                program: shellCommand !== null && shellCommand !== void 0 ? shellCommand : "bash",
                sessionType: shellDir ? "directory" : "one_off",
                createIfMissing: Boolean(shellDir),
                autoYes: false,
                autoApprove: false
            };
            setIsSubmitting(true);
            setError(null);
            try {
                await onCreateSession(sessionData);
                if (shellCommand) (0,recentShellCommands/* addRecentShellCommand */._)(shellCommand);
                onClose();
            } catch (err) {
                setError(err instanceof Error ? err.message : "Failed to create session");
            } finally{
                setIsSubmitting(false);
                isSubmittingRef.current = false;
            }
            return;
        }
        // Workflow invocation (@slug [arg]) — fire-and-forget, no session-creation flow.
        if ((detection === null || detection === void 0 ? void 0 : detection.type) === types/* InputType */.N.Workflow && ((_detection_metadata = detection.metadata) === null || _detection_metadata === void 0 ? void 0 : _detection_metadata.workflowFound)) {
            const { slug, workflowArg } = detection.metadata;
            void (onRunWorkflow === null || onRunWorkflow === void 0 ? void 0 : onRunWorkflow(slug, workflowArg !== null && workflowArg !== void 0 ? workflowArg : ""));
            onClose();
            return;
        }
        // Alias invocation (@aliasname [...]) — create session with alias context.
        if ((detection === null || detection === void 0 ? void 0 : detection.type) === types/* InputType */.N.Alias && ((_detection_metadata1 = detection.metadata) === null || _detection_metadata1 === void 0 ? void 0 : _detection_metadata1.aliasName)) {
            var _formState_firstPrompt;
            const aliasMeta = detection.metadata;
            const { aliasName, branch: aliasBranch, label, extraFlags } = aliasMeta;
            const sessionTitle = sessionName.trim() || (label === null || label === void 0 ? void 0 : label.trim()) || String(aliasName);
            // Apply useTitleAsBranch for new_worktree alias sessions, same as the regular path.
            let aliasFinalBranch = branch.trim() || (aliasBranch !== undefined ? String(aliasBranch) : "");
            if (sessionType === "new_worktree" && useTitleAsBranch && !aliasFinalBranch) {
                aliasFinalBranch = sessionTitle;
            }
            const firstPromptText = ((_formState_firstPrompt = formState.firstPrompt) === null || _formState_firstPrompt === void 0 ? void 0 : _formState_firstPrompt.trim()) || undefined;
            const sessionData = {
                title: sessionTitle,
                path: "",
                program: program || "",
                autoYes,
                autoApprove,
                aliasName: String(aliasName),
                branch: aliasFinalBranch || undefined,
                extraCliFlags: extraFlags !== undefined ? String(extraFlags) : undefined,
                extraArgs: extraArgsForSubmit,
                sessionType: sessionType,
                workingDir: workingDir.trim() || undefined,
                category: category.trim() || undefined,
                initialPrompt: firstPromptText
            };
            setIsSubmitting(true);
            setError(null);
            try {
                await onCreateSession(sessionData);
                onClose();
            } catch (err) {
                setError(err instanceof Error ? err.message : "Failed to create session");
            } finally{
                setIsSubmitting(false);
                isSubmittingRef.current = false;
            }
            return;
        }
        setIsSubmitting(true);
        setError(null);
        try {
            var _formState_firstPrompt1;
            // Determine final branch name
            let finalBranch = branch.trim();
            if ((sessionType === "new_worktree" || sessionType === "new_project" && newProjectSessionType === "new_worktree") && useTitleAsBranch) {
                finalBranch = sessionName.trim();
            }
            // Build prompt from attached image paths (pre-session uploads go to temp dir).
            const imagePaths = attachedImagePathsRef.current;
            const finalPrompt = imagePaths.length > 0 ? imagePaths.join(" ") : undefined;
            const firstPromptText = ((_formState_firstPrompt1 = formState.firstPrompt) === null || _formState_firstPrompt1 === void 0 ? void 0 : _formState_firstPrompt1.trim()) || undefined;
            let sessionData;
            if (sessionType === "new_project") {
                // New project mode: build the resolved path from parentDir + projectName
                const resolvedPath = "".concat(parentDir.trim().replace(/\/$/, ""), "/").concat(projectName.trim());
                sessionData = {
                    title: sessionName.trim(),
                    path: resolvedPath,
                    branch: newProjectSessionType === "new_worktree" ? finalBranch || undefined : undefined,
                    program,
                    category: category.trim() || undefined,
                    prompt: finalPrompt,
                    autoYes,
                    autoApprove,
                    sessionType: newProjectSessionType,
                    isNewProject: true,
                    initialPrompt: firstPromptText,
                    extraArgs: extraArgsForSubmit
                };
            } else {
                // Autonomous mode composes with sessionType (any type except one_off) rather
                // than replacing it — path/branch/worktree fields flow from the actual
                // selected type exactly as they would for a non-autonomous session.
                const isAutonomous = formState.autonomousMode;
                const isOneOff = sessionType === "one_off";
                sessionData = {
                    title: sessionName.trim(),
                    path: isOneOff ? "" : (detection === null || detection === void 0 ? void 0 : detection.localPath) || "",
                    branch: isOneOff ? undefined : finalBranch || undefined,
                    program,
                    category: category.trim() || undefined,
                    prompt: finalPrompt,
                    autoYes,
                    autoApprove,
                    sessionType,
                    existingWorktree: isOneOff ? undefined : existingWorktree.trim() || undefined,
                    workingDir: isOneOff ? undefined : workingDir.trim() || undefined,
                    autonomousMode: isAutonomous ? true : undefined,
                    permissionMode: isAutonomous ? "auto" : undefined,
                    // Remote target composes with sessionType exactly like autonomousMode above —
                    // passed through only when set (ADR-001: remote-as-orthogonal-flag).
                    remoteName: formState.remoteName || undefined,
                    // Only forward when relevant (non-existent path + opt-in checked).
                    createIfMissing: pathDoesNotExist && createIfMissing ? true : undefined,
                    initialPrompt: firstPromptText,
                    extraArgs: extraArgsForSubmit,
                    fanoutCount: sessionType === "new_worktree" && formState.fanoutCount >= 2 ? formState.fanoutCount : undefined
                };
                // Handle GitHub URLs - path will be resolved server-side
                if (!isOneOff && (detection === null || detection === void 0 ? void 0 : detection.gitHubRef)) {
                    sessionData.gitHubOwner = detection.gitHubRef.owner;
                    sessionData.gitHubRepo = detection.gitHubRef.repo;
                    sessionData.gitHubPRNumber = detection.gitHubRef.prNumber;
                    // For GitHub URLs, set path to the parsed value for server-side cloning
                    if (!sessionData.path) {
                        sessionData.path = detection.parsedValue;
                    }
                }
            }
            try {
                await onCreateSession(sessionData);
            } catch (err) {
                // R2: Directory mode with non-existent path → show confirmation dialog
                if (sessionType === "directory" && err instanceof Error && (err.message.includes("not found") || err.message.includes("CodeNotFound") || err.message.includes("path does not exist"))) {
                    setPendingSessionData(sessionData);
                    setShowPathConfirmation(true);
                    setIsSubmitting(false);
                    isSubmittingRef.current = false;
                    return;
                }
                throw err;
            }
            // Persist the chosen path to history for future completions.
            if (isPathInput && (detection === null || detection === void 0 ? void 0 : detection.localPath) && sessionType !== "one_off") {
                saveHistory(detection.localPath);
            }
            onClose();
        } catch (err) {
            const message = err instanceof Error ? err.message : "Failed to create session";
            setError(message);
        } finally{
            setIsSubmitting(false);
            isSubmittingRef.current = false;
        }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- pre-existing; adding formState.fanoutCount/setFormField would change re-render/callback-identity behavior, unverified here
    }, [
        canSubmit,
        isSubmitting,
        branch,
        sessionName,
        sessionType,
        useTitleAsBranch,
        detection,
        program,
        category,
        autoYes,
        autoApprove,
        existingWorktree,
        workingDir,
        parentDir,
        projectName,
        newProjectSessionType,
        pathDoesNotExist,
        createIfMissing,
        isPathInput,
        saveHistory,
        onCreateSession,
        onClose,
        onRunWorkflow,
        formState.firstPrompt,
        formState.autonomousMode,
        formState.extraArgs,
        formState.remoteName,
        router,
        setTheme
    ]);
    // Keep the ref in sync so handleKeyDown always dispatches the latest version.
    (0,react.useEffect)(()=>{
        handleSubmitRef.current = handleSubmit;
    }, [
        handleSubmit
    ]);
    if (!isOpen) return null;
    // Reviewing an LLM-parsed backlog item draft (backlog: <message>) replaces
    // the entire omnibar body — a different UI mode from session creation, not
    // another branch of the input/results form below.
    if (backlogReviewText !== null) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: overlay,
            onClick: onClose,
            role: "dialog",
            "aria-modal": "true",
            "aria-labelledby": "omnibar-title",
            "data-testid": "omnibar",
            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: modal,
                "data-testid": "omnibar-modal",
                onClick: (e)=>e.stopPropagation(),
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(BacklogItemIntentReview, {
                    initialText: backlogReviewText,
                    onDone: (item)=>{
                        setBacklogReviewText(null);
                        onClose();
                        router.push("/backlog?item=".concat(item.id));
                    },
                    onCancel: ()=>setBacklogReviewText(null)
                })
            })
        });
    }
    // Compose mode: dedicated textarea; Create closes the omnibar and creates the
    // item in the background, with triage progress reported via notifications.
    if (backlogComposeText !== null) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: overlay,
            // Backdrop clicks are ignored: the draft lives in the widget and only Cancel/Escape preserve it.
            role: "dialog",
            "aria-modal": "true",
            "aria-labelledby": "backlog-compose-title",
            "data-testid": "omnibar",
            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: modal,
                "data-testid": "omnibar-modal",
                onClick: (e)=>e.stopPropagation(),
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(BacklogComposeWidget, {
                    initialText: backlogComposeText,
                    onSubmit: (text)=>{
                        setBacklogComposeText(null);
                        onClose();
                        void createBacklogItemInBackground(text);
                    },
                    onCancel: (draft)=>{
                        setInput(draft);
                        setBacklogComposeText(null);
                    }
                })
            })
        });
    }
    const isMac = (()=>{
        try {
            return typeof navigator !== "undefined" && /Mac|iPhone|iPad|iPod/.test(navigator.platform);
        } catch (e) {
            return false;
        }
    })();
    var _detection_metadata_alias_namePrefix;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: overlay,
        onClick: onClose,
        role: "dialog",
        "aria-modal": "true",
        "aria-labelledby": "omnibar-title",
        "data-testid": "omnibar",
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: modal,
            "data-testid": "omnibar-modal",
            onClick: (e)=>e.stopPropagation(),
            onKeyDown: handleKeyDown,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: inputContainer,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: typeIndicator,
                            "aria-hidden": "true",
                            children: sessionType === "one_off" ? "⚡" : formState.autonomousMode ? "🤖" : typeInfo.icon
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                            ref: inputRef,
                            type: "text",
                            className: Omnibar_css_input,
                            placeholder: sessionType === "one_off" ? "Session title is the only thing needed…" : formState.autonomousMode ? "Session title (agent will run without human approval)…" : isDiscoveryMode ? "Jump to session, @alias, or search repos..." : "Enter path, GitHub URL, or owner/repo...",
                            value: input,
                            onChange: (e)=>{
                                setInput(e.target.value);
                                setDropdownDismissed(false);
                                setDropdownIndex(-1);
                            },
                            autoComplete: "off",
                            autoCorrect: "off",
                            autoCapitalize: "off",
                            spellCheck: false,
                            role: "combobox",
                            "aria-label": "Session source input",
                            "aria-autocomplete": "list",
                            "aria-expanded": isDiscoveryMode ? displayedSessionResults.length > 0 || displayedRepoEntries.length > 0 : isDropdownVisible,
                            "aria-controls": isDiscoveryMode ? RESULT_LISTBOX_ID : "path-completion-listbox",
                            "aria-activedescendant": isDiscoveryMode ? getHighlightedItemId(RESULT_LISTBOX_ID, displayedSessionResults, displayedRepoEntries, resultHighlightIndex) : isDropdownVisible && dropdownIndex >= 0 ? "path-completion-listbox-option-".concat(dropdownIndex) : undefined
                        }),
                        isPathInput && !isDiscoveryMode && input.trim() && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: pathIndicator,
                            "aria-live": "polite",
                            "aria-label": isCompletionLoading ? "Checking path" : pathExists ? "Path exists" : createIfMissing ? "New repository will be created" : "Path does not exist",
                            children: isCompletionLoading ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: pathIndicatorLoading,
                                "aria-hidden": "true",
                                children: "⟳"
                            }) : pathExists ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: pathIndicatorValid,
                                "aria-hidden": "true",
                                children: "✓"
                            }) : createIfMissing ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: pathIndicatorValid,
                                "aria-hidden": "true",
                                children: "+"
                            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: pathIndicatorInvalid,
                                "aria-hidden": "true",
                                children: "✗"
                            })
                        })
                    ]
                }),
                isAliasPaletteVisible && /*#__PURE__*/ (0,jsx_runtime.jsx)(AliasPalette, {
                    aliases: filteredAliases,
                    input: input,
                    selectedIndex: aliasSuggestIndex,
                    onSelect: (alias)=>{
                        setInput(completeAlias(alias));
                        setAliasSuggestIndex(-1);
                    },
                    error: aliasError
                }),
                isAtDropdownVisible && /*#__PURE__*/ (0,jsx_runtime.jsx)(AtCommandDropdown, {
                    id: "at-command-listbox",
                    suggestions: atSuggestions,
                    selectedIndex: atSuggestIndex,
                    onSelect: (wf)=>{
                        var _inputRef_current;
                        setInput(completeAtCommand(wf));
                        setUIField("atSuggestIndex", -1);
                        (_inputRef_current = inputRef.current) === null || _inputRef_current === void 0 ? void 0 : _inputRef_current.focus();
                    }
                }),
                (detection === null || detection === void 0 ? void 0 : detection.type) === types/* InputType */.N.Alias && detection.metadata && (()=>{
                    const m = detection.metadata;
                    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        role: "status",
                        "aria-live": "polite",
                        "data-testid": "alias-resolution-chip",
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                children: [
                                    "Alias resolved: @",
                                    String(m.aliasName)
                                ]
                            }),
                            m.branch ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                children: [
                                    " :",
                                    String(m.branch)
                                ]
                            }) : null,
                            m.label ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                children: [
                                    " \xb7 ",
                                    String(m.label)
                                ]
                            }) : null,
                            m.extraFlags ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                children: [
                                    " \xb7 ",
                                    String(m.extraFlags),
                                    " (appended)"
                                ]
                            }) : null
                        ]
                    });
                })(),
                (detection === null || detection === void 0 ? void 0 : detection.type) === types/* InputType */.N.Workflow && !!((_detection_metadata = detection.metadata) === null || _detection_metadata === void 0 ? void 0 : _detection_metadata.workflowFound) && (()=>{
                    const { workflow, workflowArg } = detection.metadata;
                    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        role: "status",
                        "aria-live": "polite",
                        "data-testid": "workflow-resolution-chip",
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                children: [
                                    "Workflow: @",
                                    workflow === null || workflow === void 0 ? void 0 : workflow.slug,
                                    (workflow === null || workflow === void 0 ? void 0 : workflow.name) ? " (".concat(workflow.name, ")") : ""
                                ]
                            }),
                            (workflow === null || workflow === void 0 ? void 0 : workflow.description) ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                children: [
                                    " \xb7 ",
                                    workflow.description
                                ]
                            }) : null,
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                children: [
                                    " \xb7 arg: ",
                                    workflowArg ? workflowArg : "(none)"
                                ]
                            })
                        ]
                    });
                })(),
                (detection === null || detection === void 0 ? void 0 : detection.type) === types/* InputType */.N.AliasNotFound && (()=>{
                    const m = detection.metadata;
                    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        role: "alert",
                        "aria-live": "assertive",
                        "data-testid": "alias-not-found",
                        children: [
                            "No alias '@",
                            String(m === null || m === void 0 ? void 0 : m.slug),
                            "'"
                        ]
                    });
                })(),
                selectedPreset && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    role: "status",
                    "aria-live": "polite",
                    "data-testid": "preset-resolution-chip",
                    children: [
                        "Preset applied: ",
                        selectedPreset.label,
                        " (",
                        program,
                        formState.extraArgs.length > 0 ? " ".concat(formState.extraArgs.join(" ")) : "",
                        ")"
                    ]
                }),
                (detection === null || detection === void 0 ? void 0 : detection.type) === types/* InputType */.N.PresetNotFound && (()=>{
                    const m = detection.metadata;
                    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        role: "alert",
                        "aria-live": "assertive",
                        "data-testid": "preset-not-found",
                        children: [
                            "No preset '",
                            m === null || m === void 0 ? void 0 : m.typedId,
                            "'"
                        ]
                    });
                })(),
                (detection === null || detection === void 0 ? void 0 : detection.type) === types/* InputType */.N.SpawnShell && (()=>{
                    var _detection_metadata;
                    const { shellDir, shellCommand } = (_detection_metadata = detection.metadata) !== null && _detection_metadata !== void 0 ? _detection_metadata : {};
                    const recentCommands = (0,recentShellCommands/* getRecentShellCommands */.e)();
                    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                role: "status",
                                "aria-live": "polite",
                                "data-testid": "spawn-shell-chip",
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        children: shellCommand ? 'Run "'.concat(shellCommand, '"') : "Open terminal"
                                    }),
                                    shellDir ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                        children: [
                                            " in ",
                                            shellDir
                                        ]
                                    }) : null
                                ]
                            }),
                            error && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: Omnibar_css_error,
                                role: "alert",
                                "aria-live": "assertive",
                                "data-testid": "spawn-shell-error",
                                children: error
                            }),
                            !shellCommand && !shellDir && recentCommands.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                "data-testid": "spawn-shell-recent-commands",
                                children: recentCommands.map((cmd)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        type: "button",
                                        "data-testid": "spawn-shell-recent-command",
                                        // eslint-disable-next-line analytics/require-on-click -- pre-existing, unrelated
                                        onClick: ()=>{
                                            var _inputRef_current;
                                            setInput(">shell -- ".concat(cmd));
                                            (_inputRef_current = inputRef.current) === null || _inputRef_current === void 0 ? void 0 : _inputRef_current.focus();
                                        },
                                        children: cmd
                                    }, cmd))
                            })
                        ]
                    });
                })(),
                isShellDirInput && isDropdownVisible && /*#__PURE__*/ (0,jsx_runtime.jsx)(PathCompletionDropdown/* PathCompletionDropdown */.h, {
                    id: "shell-dir-completion-listbox",
                    entries: mergedEntries,
                    historyCount: historyCount,
                    selectedIndex: dropdownIndex,
                    onSelect: handleCompletionSelect,
                    isLoading: isCompletionLoading
                }),
                isDiscoveryMode && !isAtDropdownVisible && /*#__PURE__*/ (0,jsx_runtime.jsx)(OmnibarResultList, {
                    id: RESULT_LISTBOX_ID,
                    sessionResults: displayedSessionResults,
                    repoEntries: displayedRepoEntries,
                    highlightedIndex: resultHighlightIndex,
                    onSessionSelect: handleSessionSelect,
                    onSessionOpenInNewPane: onNavigateToSessionInNewPane ? handleSessionSelectInNewPane : undefined,
                    onRepoSelect: handleRepoSelect,
                    onCloneSession: handleCloneSession,
                    onCreateNew: ()=>{
                        dispatchMode({
                            kind: "open_creation_direct"
                        });
                        setResultHighlightIndex(-1);
                    },
                    onCreateBacklog: openBacklogCompose
                }),
                !isDiscoveryMode && isDropdownVisible && sessionType !== "one_off" && /*#__PURE__*/ (0,jsx_runtime.jsx)(PathCompletionDropdown/* PathCompletionDropdown */.h, {
                    id: "path-completion-listbox",
                    entries: mergedEntries,
                    historyCount: historyCount,
                    selectedIndex: dropdownIndex,
                    onSelect: handleCompletionSelect,
                    isLoading: isCompletionLoading
                }),
                isPathInput && completionError && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: Omnibar_css_completionError,
                    "aria-live": "polite",
                    children: "Could not load completions"
                }),
                input.trim() && !isDiscoveryMode && sessionType !== "one_off" && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: detectionInfo,
                    children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: "".concat(detectionBadge, " ").concat((detection === null || detection === void 0 ? void 0 : detection.type) === types/* InputType */.N.Unknown ? unknown : ""),
                        children: [
                            typeInfo.icon,
                            " ",
                            typeInfo.label
                        ]
                    })
                }),
                !isDiscoveryMode && /*#__PURE__*/ (0,jsx_runtime.jsx)(OmnibarCreationPanel, {
                    formState: formState,
                    setFormField: setFormField,
                    onSubmit: handleSubmit,
                    onCancel: onClose,
                    onCreateBacklog: openBacklogCompose,
                    worktrees: worktrees,
                    isWorktreesLoading: isWorktreesLoading,
                    worktreesError: worktreesError,
                    isSubmitting: isSubmitting,
                    canSubmit: canSubmit,
                    error: error,
                    showAdvanced: showAdvanced,
                    onToggleAdvanced: ()=>setUIField("showAdvanced", !uiState.showAdvanced),
                    path: modeState.type === "creation_with_repo" ? modeState.path : (detection === null || detection === void 0 ? void 0 : detection.type) === types/* InputType */.N.Alias ? ((_detection_metadata1 = detection.metadata) === null || _detection_metadata1 === void 0 ? void 0 : (_detection_metadata_alias = _detection_metadata1.alias) === null || _detection_metadata_alias === void 0 ? void 0 : _detection_metadata_alias.path) || undefined : undefined,
                    uploadBaseUrl: uploadBaseUrl,
                    onAttachedImagesChange: (paths)=>{
                        attachedImagePathsRef.current = paths;
                    },
                    pathDoesNotExist: pathDoesNotExist,
                    destinationPreviewPath: destinationPreviewPath,
                    destinationPreviewIsExact: destinationPreviewIsExact,
                    isDestinationPreviewLoading: isDestinationPreviewLoading,
                    namePrefix: (detection === null || detection === void 0 ? void 0 : detection.type) === types/* InputType */.N.Alias ? (_detection_metadata_alias_namePrefix = (_detection_metadata2 = detection.metadata) === null || _detection_metadata2 === void 0 ? void 0 : (_detection_metadata_alias1 = _detection_metadata2.alias) === null || _detection_metadata_alias1 === void 0 ? void 0 : _detection_metadata_alias1.namePrefix) !== null && _detection_metadata_alias_namePrefix !== void 0 ? _detection_metadata_alias_namePrefix : "" : "",
                    onPresetSelect: handlePresetSelect,
                    launcherPresets: launcherPresets,
                    launcherPresetsLoading: launcherPresetsLoading,
                    launcherPresetsLoadError: launcherPresetsLoadError,
                    remotes: remotes
                }),
                showPathConfirmation && pendingSessionData && /*#__PURE__*/ (0,jsx_runtime.jsx)(PathConfirmDialog, {
                    path: pendingSessionData.path,
                    onCancel: ()=>{
                        setShowPathConfirmation(false);
                        setPendingSessionData(null);
                    },
                    onConfirm: async ()=>{
                        setShowPathConfirmation(false);
                        const retryData = {
                            ...pendingSessionData,
                            createIfMissing: true
                        };
                        setPendingSessionData(null);
                        setIsSubmitting(true);
                        setError(null);
                        try {
                            await onCreateSession(retryData);
                            onClose();
                        } catch (err) {
                            const message = err instanceof Error ? err.message : "Failed to create session";
                            setError(message);
                        } finally{
                            setIsSubmitting(false);
                            isSubmittingRef.current = false;
                        }
                    }
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: shortcuts,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)(OmnibarModeBadge, {
                            mode: isDiscoveryMode ? "discovery" : "creation",
                            onToggle: ()=>isDiscoveryMode ? dispatchMode({
                                    kind: "open_creation_direct"
                                }) : dispatchMode({
                                    kind: "reset_to_discovery"
                                })
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                            className: shortcut,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: shortcutKey,
                                    children: "Esc"
                                }),
                                " Close"
                            ]
                        }),
                        !isDiscoveryMode && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            type: "button",
                            className: createButton,
                            // eslint-disable-next-line analytics/require-on-click -- pre-existing, unrelated
                            onClick: handleSubmit,
                            disabled: !canSubmit || isSubmitting,
                            children: isSubmitting ? "Creating…" : "Create Session"
                        }),
                        isDiscoveryMode && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    className: shortcut,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: shortcutKey,
                                            children: "↑↓"
                                        }),
                                        " Navigate"
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    className: shortcut,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: shortcutKey,
                                            children: "↵"
                                        }),
                                        " Jump"
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)(OmnibarBacklogButton, {
                                    onClick: openBacklogCompose,
                                    className: "".concat(Omnibar_css_button, " ").concat(buttonSecondary, " ").concat(shortcutsBacklogButton)
                                })
                            ]
                        }),
                        !isDiscoveryMode && isDropdownVisible && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    className: shortcut,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: shortcutKey,
                                            children: "↑↓"
                                        }),
                                        " Navigate"
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    className: shortcut,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: shortcutKey,
                                            children: "Tab"
                                        }),
                                        " Complete"
                                    ]
                                })
                            ]
                        }),
                        !isDiscoveryMode && !isDropdownVisible && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                            className: shortcut,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: shortcutKey,
                                    children: "Tab"
                                }),
                                " Cycle type"
                            ]
                        })
                    ]
                })
            ]
        })
    });
}

// EXTERNAL MODULE: ./src/lib/hooks/useSessionService.ts + 1 modules
var useSessionService = __webpack_require__(73117);
// EXTERNAL MODULE: ./src/lib/hooks/useWorkflows.ts + 1 modules
var useWorkflows = __webpack_require__(79623);
// EXTERNAL MODULE: ./src/lib/contexts/AuthContext.tsx
var AuthContext = __webpack_require__(64792);
// EXTERNAL MODULE: ./src/lib/contexts/NotificationContext.tsx + 12 modules
var NotificationContext = __webpack_require__(62275);
// EXTERNAL MODULE: ./src/lib/utils/connectError.ts
var connectError = __webpack_require__(8854);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/create.js
var create = __webpack_require__(95444);
;// ./src/lib/omnibar/detectors/WorkflowDetector.ts
// +feature: workflow-detector
/**
 * WorkflowDetector detects @slug [arg] syntax for quick workflow invocation.
 * Priority 25 — after GitHub URL detectors (10/20/30) and before NewSessionDetector (35).
 */ 
/**
 * WorkflowDetector matches "@slug [arg]" input patterns.
 *
 * Registration: NOT in createDefaultRegistry(). Dynamically registered/unregistered
 * by OmnibarContext when workflows load, so the list is always current.
 *
 * Priority 25 is between GitHubRepoDetector (30) and NewSessionDetector (35).
 * "@" is unclaimed by any other detector.
 */ class WorkflowDetector {
    detect(input) {
        var _match_;
        const trimmed = input.trim();
        const match = trimmed.match(this.AT_PATTERN);
        if (!match) return null;
        const rawSlug = match[1];
        var _match__trim;
        const arg = (_match__trim = (_match_ = match[2]) === null || _match_ === void 0 ? void 0 : _match_.trim()) !== null && _match__trim !== void 0 ? _match__trim : "";
        const slug = rawSlug.toLowerCase();
        // Look up workflow by slug (case-insensitive).
        const wf = this.workflows.find((w)=>w.slug.toLowerCase() === slug);
        if (!wf) {
            // Unknown slug — return null so AliasDetector (priority 36) can claim @unknown input.
            // WorkflowDetector only claims slugs that match known workflows.
            return null;
        }
        var _wf_inputTemplate;
        // Known slug — interpolate prompt template if available.
        let interpolatedPrompt = (_wf_inputTemplate = wf.inputTemplate) !== null && _wf_inputTemplate !== void 0 ? _wf_inputTemplate : "";
        if (interpolatedPrompt && arg) {
            interpolatedPrompt = interpolatedPrompt.replace(/\{\{input\}\}/g, arg);
        } else if (!interpolatedPrompt) {
            interpolatedPrompt = arg;
        }
        return {
            type: types/* InputType */.N.Workflow,
            confidence: 1.0,
            parsedValue: trimmed,
            suggestedName: wf.name,
            metadata: {
                workflowFound: true,
                slug: wf.slug,
                workflowArg: arg,
                interpolatedPrompt,
                workflow: wf
            }
        };
    }
    constructor(workflows){
        this.name = "WorkflowDetector";
        this.priority = 25;
        this.AT_PATTERN = /^@([a-zA-Z0-9_-]+)(?:\s+(.+))?$/;
        this.workflows = workflows;
    }
}

// EXTERNAL MODULE: ./src/lib/omnibar/detectors/AliasDetector.ts
var AliasDetector = __webpack_require__(63251);
;// ./src/lib/hooks/useLauncherPresets.ts
/* __next_internal_client_entry_do_not_use__ useLauncherPresets auto */ 



// useLauncherPresets fetches ~/.stapler-squad/launcher-presets.json fresh on every call (the
// backend never caches — see GetLauncherPresets), exposing a domain-level loadError (malformed
// file) distinctly from a transport error (network/server failure), so the UI can render a
// specific, diagnosable message for the former without conflating it with the latter.
function useLauncherPresets() {
    const [presets, setPresets] = (0,react.useState)([]);
    const [loading, setLoading] = (0,react.useState)(true);
    const [error, setError] = (0,react.useState)(null);
    const [loadError, setLoadError] = (0,react.useState)(null);
    const [fetchTick, setFetchTick] = (0,react.useState)(0);
    const clientRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
        let cancelled = false;
        async function fetchPresets() {
            try {
                const resp = await clientRef.current.getLauncherPresets({});
                if (!cancelled) {
                    var _resp_presets;
                    setPresets(((_resp_presets = resp.presets) !== null && _resp_presets !== void 0 ? _resp_presets : []).map((p)=>{
                        var _p_argv;
                        return {
                            id: p.id,
                            label: p.label,
                            argv: [
                                ...(_p_argv = p.argv) !== null && _p_argv !== void 0 ? _p_argv : []
                            ],
                            program: p.program,
                            defaultPath: p.defaultPath
                        };
                    }));
                    setLoadError(resp.loadError || null);
                }
            } catch (err) {
                if (!cancelled) {
                    setError(err instanceof Error ? err : new Error(String(err)));
                }
            } finally{
                if (!cancelled) {
                    setLoading(false);
                }
            }
        }
        void fetchPresets();
        return ()=>{
            cancelled = true;
        };
    }, [
        fetchTick
    ]);
    const refetch = (0,react.useCallback)(()=>setFetchTick((t)=>t + 1), []);
    return {
        presets,
        loading,
        error,
        loadError,
        refetch
    };
}

;// ./src/lib/omnibar/detectors/PresetDetector.ts
// +feature: launcher-presets-detector
/**
 * PresetDetector detects preset:<id> shorthand, resolving directly to a configured
 * launcher preset. Priority 37 — after AliasDetector (36), before GitHubShorthandDetector (40).
 *
 * Simpler than AliasDetector by design (no browse mode, no label/flags suffix): presets are
 * authored once in a hand-edited file, not typed conversationally as often as @alias names.
 *
 * IMPORTANT: PresetDetector must claim ALL input starting with "preset:" and never return null
 * for such input (once an id is present), so SessionSearchDetector never claims it.
 */ 
class PresetDetector {
    detect(input) {
        if (!input.startsWith("preset:")) return null;
        const id = input.slice("preset:".length).trim();
        if (!id) return null;
        // Case-sensitive: presets are authored once in a file the user controls, not typed
        // conversationally as often as @alias names (unlike FindAlias's case-insensitive match).
        const found = this.presets.find((p)=>p.id === id);
        if (!found) {
            return {
                type: types/* InputType */.N.PresetNotFound,
                confidence: 1.0,
                parsedValue: input,
                suggestedName: id,
                metadata: {
                    typedId: input
                }
            };
        }
        return {
            type: types/* InputType */.N.Preset,
            confidence: 1.0,
            parsedValue: input,
            suggestedName: found.label,
            metadata: {
                preset: found
            }
        };
    }
    constructor(presets){
        this.name = "PresetDetector";
        this.priority = 37;
        this.presets = presets;
    }
}

// EXTERNAL MODULE: ./src/lib/hooks/useGitHubEnterpriseHosts.ts
var useGitHubEnterpriseHosts = __webpack_require__(44635);
// EXTERNAL MODULE: ./src/lib/hooks/useRemotesService.ts
var useRemotesService = __webpack_require__(16446);
;// ./src/lib/hooks/useConfiguredRemotes.ts
/* __next_internal_client_entry_do_not_use__ useConfiguredRemotes auto */ 

// useConfiguredRemotes fetches the configured-remotes list (ListRemotes) once on mount and
// maps it to the minimal RemoteOption[] shape the Omnibar's "Remote host" selector needs
// (ADR-001: remote-as-orthogonal-flag). This is the wiring OmnibarCreationPanel.tsx's and
// Omnibar.tsx's own "TODO(Phase 6): ... sourced from a remotesSlice ... populated by a
// ListRemotes RPC" comments called for -- until this hook existed, OmnibarContext.tsx never
// passed a `remotes` prop to <Omnibar> at all, so the selector never rendered in the actual
// running app regardless of what was configured in Settings -> Remotes (ssh-remote-workspaces
// Phase 6 Epic 6.3, found while wiring up remote-workspaces.spec.ts's e2e coverage of "create a
// session against a remote via the Omnibar").
//
// Mirrors useLauncherPresets' fetch-once-on-mount pattern (a fresh client per hook instance,
// no polling -- the list only changes when the user visits Settings -> Remotes, which is rare
// enough that a stale list for the lifetime of one page session is an acceptable tradeoff,
// matching launcher presets' same tradeoff).
function useConfiguredRemotes() {
    const { listRemotes } = (0,useRemotesService/* useRemotesService */.I)();
    const listRemotesRef = (0,react.useRef)(listRemotes);
    listRemotesRef.current = listRemotes;
    const [remotes, setRemotes] = (0,react.useState)([]);
    const [loading, setLoading] = (0,react.useState)(true);
    (0,react.useEffect)(()=>{
        let cancelled = false;
        async function fetchRemotes() {
            try {
                const list = await listRemotesRef.current();
                if (!cancelled) {
                    setRemotes(list.map((r)=>({
                            name: r.name
                        })));
                }
            } catch (e) {
            // Best-effort: a failed fetch just leaves the selector hidden (empty
            // list), matching today's no-remotes-configured behavior rather than
            // surfacing a fetch error in the session-creation UI.
            } finally{
                if (!cancelled) setLoading(false);
            }
        }
        void fetchRemotes();
        return ()=>{
            cancelled = true;
        };
    }, []);
    return {
        remotes,
        loading
    };
}

;// ./src/lib/contexts/OmnibarContext.tsx
/* __next_internal_client_entry_do_not_use__ useOmnibar,OmnibarProvider auto */ 




















const sessionTypeMap = {
    directory: types_pb/* SessionType */.dF.DIRECTORY,
    new_worktree: types_pb/* SessionType */.dF.NEW_WORKTREE,
    existing_worktree: types_pb/* SessionType */.dF.EXISTING_WORKTREE,
    one_off: types_pb/* SessionType */.dF.ONE_OFF,
    new_project: types_pb/* SessionType */.dF.NEW_PROJECT,
    autonomous: types_pb/* SessionType */.dF.DIRECTORY
};
const OmnibarContext = /*#__PURE__*/ (0,react.createContext)(null);
function useOmnibar() {
    const context = (0,react.useContext)(OmnibarContext);
    if (!context) {
        return {
            isOpen: false,
            open: ()=>{},
            openInCreationMode: ()=>{},
            openOmnibar: ()=>{},
            close: ()=>{},
            toggle: ()=>{}
        };
    }
    return context;
}
function OmnibarProvider(param) {
    let { children } = param;
    const [isOpen, setIsOpen] = (0,react.useState)(false);
    const [initialMode, setInitialMode] = (0,react.useState)("discovery");
    const [initialInput, setInitialInput] = (0,react.useState)(undefined);
    const [initialTitle, setInitialTitle] = (0,react.useState)(undefined);
    const router = (0,nav/* useRouter */.KA)();
    const { authEnabled, authenticated, loading: authLoading } = (0,AuthContext/* useAuth */.A)();
    const { createSession, runWorkflow: runWorkflowRPC } = (0,useSessionService/* useSessionService */.D)({
        enabled: !authLoading && (!authEnabled || authenticated)
    });
    const { workflows } = (0,useWorkflows/* useWorkflows */.m)();
    const { showActionToast } = (0,NotificationContext/* useNotifications */.E$)();
    // Lean WorkflowEntry[] for the detector and @ autocomplete dropdown.
    const workflowEntries = (0,react.useMemo)(()=>workflows.map((w)=>({
                slug: w.slug,
                name: w.name,
                description: w.description,
                targetDirectory: w.targetDirectory,
                sessionType: w.sessionType,
                inputTemplate: w.inputTemplate
            })), [
        workflows
    ]);
    // Dynamically register/unregister WorkflowDetector whenever the workflow list changes.
    // The detector is NOT in createDefaultRegistry() — it lives only in the singleton
    // returned by getDefaultRegistry() so it reflects the live DB state.
    const workflowDetectorRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        const registry = getDefaultRegistry();
        if (workflowDetectorRef.current) {
            registry.unregister(workflowDetectorRef.current);
        }
        const detector = new WorkflowDetector(workflowEntries);
        registry.register(detector);
        workflowDetectorRef.current = detector;
        return ()=>{
            registry.unregister(detector);
            workflowDetectorRef.current = null;
        };
    }, [
        workflowEntries
    ]);
    const { aliases } = useAliases();
    // Dynamically register/unregister AliasDetector whenever the alias list changes.
    const aliasDetectorRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        const registry = getDefaultRegistry();
        if (aliasDetectorRef.current) {
            registry.unregister(aliasDetectorRef.current);
        }
        const detector = new AliasDetector/* AliasDetector */.t(aliases);
        registry.register(detector);
        aliasDetectorRef.current = detector;
        return ()=>{
            registry.unregister(detector);
            aliasDetectorRef.current = null;
        };
    }, [
        aliases
    ]);
    // Single source of truth for launcher presets — both the PresetDetector registration below
    // and OmnibarPresetList's rendering (passed down through Omnibar -> OmnibarCreationPanel as
    // props) read from this one fetch. A second, independent useLauncherPresets() call in
    // OmnibarCreationPanel would fetch redundantly and — worse — this effect's refetch-on-open
    // would never reach it, since each hook call owns disconnected state.
    const { presets: launcherPresets, loading: launcherPresetsLoading, loadError: launcherPresetsLoadError, refetch: refetchLauncherPresets } = useLauncherPresets();
    // Configured remotes for the Omnibar's "Remote host" selector (ADR-001:
    // remote-as-orthogonal-flag) -- see useConfiguredRemotes' doc comment for why this wiring
    // was missing until ssh-remote-workspaces Phase 6 Epic 6.3.
    const { remotes: configuredRemotes } = useConfiguredRemotes();
    // Dynamically register/unregister PresetDetector whenever the preset list changes.
    const presetDetectorRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        const registry = getDefaultRegistry();
        if (presetDetectorRef.current) {
            registry.unregister(presetDetectorRef.current);
        }
        const detector = new PresetDetector(launcherPresets);
        registry.register(detector);
        presetDetectorRef.current = detector;
        return ()=>{
            registry.unregister(detector);
            presetDetectorRef.current = null;
        };
    }, [
        launcherPresets
    ]);
    const { hosts: enterpriseHosts, refetch: refetchEnterpriseHosts } = (0,useGitHubEnterpriseHosts/* useGitHubEnterpriseHosts */.P)();
    // Re-fetch presets and GHE hosts each time the omnibar opens so a hand-edited
    // launcher-presets.json change, or a GitHub Enterprise account added mid-session,
    // appears immediately without a server restart or page reload (Success Criterion 1;
    // same staleness problem for enterprise hosts as launcher presets).
    const prevOmnibarOpenRef = (0,react.useRef)(false);
    (0,react.useEffect)(()=>{
        if (isOpen && !prevOmnibarOpenRef.current) {
            refetchLauncherPresets();
            refetchEnterpriseHosts();
        }
        prevOmnibarOpenRef.current = isOpen;
    }, [
        isOpen,
        refetchLauncherPresets,
        refetchEnterpriseHosts
    ]);
    // GitHubEnterpriseURLDetector is registered synchronously (with an empty host
    // list) inside createDefaultRegistry(), so the registry always has a slot for
    // it — no window where a GHES URL falls through to SessionSearch. This effect
    // just keeps its host list in sync as the async RPC result changes.
    (0,react.useEffect)(()=>{
        const detector = getDefaultRegistry().find("GitHubEnterpriseURL");
        detector === null || detector === void 0 ? void 0 : detector.setHosts(enterpriseHosts);
    }, [
        enterpriseHosts
    ]);
    const open = (0,react.useCallback)(()=>{
        setInitialMode("discovery");
        setInitialInput(undefined);
        setInitialTitle(undefined);
        setIsOpen(true);
    }, []);
    const openInCreationMode = (0,react.useCallback)(()=>{
        setInitialMode("creation");
        setInitialInput(undefined);
        setInitialTitle(undefined);
        setIsOpen(true);
    }, []);
    const openOmnibar = (0,react.useCallback)((inputValue, titleValue)=>{
        setInitialMode(inputValue ? "creation" : "discovery");
        setInitialInput(inputValue);
        setInitialTitle(titleValue);
        setIsOpen(true);
    }, []);
    const close = (0,react.useCallback)(()=>{
        setIsOpen(false);
        setInitialInput(undefined);
        setInitialTitle(undefined);
    }, []);
    const toggle = (0,react.useCallback)(()=>setIsOpen((prev)=>!prev), []);
    // Global keyboard shortcut: Cmd+K or Ctrl+K (discovery), Cmd+Shift+K (creation)
    (0,react.useEffect)(()=>{
        const handleKeyDown = (e)=>{
            // Cmd+Shift+K (Mac) or Ctrl+Shift+K — open directly in creation mode
            if ((e.metaKey || e.ctrlKey) && e.shiftKey && e.key === "K") {
                e.preventDefault();
                openInCreationMode();
                return;
            }
            // Cmd+K (Mac) or Ctrl+K (Windows/Linux) — discovery mode toggle
            if ((e.metaKey || e.ctrlKey) && !e.shiftKey && e.key === "k") {
                e.preventDefault();
                toggle();
            }
            // Also support 'n' key when not in an input
            if (e.key === "n" && !isInputElement(e.target)) {
                e.preventDefault();
                open();
            }
        };
        document.addEventListener("keydown", handleKeyDown);
        return ()=>document.removeEventListener("keydown", handleKeyDown);
    }, [
        toggle,
        open,
        openInCreationMode
    ]);
    const handleNavigateToSession = (0,react.useCallback)((sessionId)=>{
        router.push("/?session=".concat(sessionId));
        close();
    }, [
        router,
        close
    ]);
    const handleNavigateToSessionInNewPane = (0,react.useCallback)((sessionId)=>{
        router.push("/?session=".concat(sessionId, "&newPane=true"));
        close();
    }, [
        router,
        close
    ]);
    // Handle session creation
    const handleCreateSession = (0,react.useCallback)(async (data)=>{
        // Determine effective session type. Every new_project variant — "open as
        // new_worktree" and "open as directory" alike — uses NEW_PROJECT: the
        // backend's SessionTypeNewProject case always bootstraps a missing path
        // (mkdir + git init + initial commit), then additionally creates a
        // worktree on it when a branch is present (Omnibar.tsx sets `branch` only
        // for "open as new_worktree"), or leaves it as a plain directory session
        // otherwise (see session/instance_worktree.go's setupFirstTimeWorktree).
        // NEW_WORKTREE is deliberately never used for new_project — it requires
        // an already-existing repo unconditionally, with no bootstrap exception.
        const effectiveSessionType = data.isNewProject ? types_pb/* SessionType */.dF.NEW_PROJECT : data.sessionType ? sessionTypeMap[data.sessionType] : undefined;
        if (data.fanoutCount && data.fanoutCount >= 2) {
            var _data_autoApprove, _data_permissionMode, _data_extraArgs;
            // Server validates 2-5 and rejects anything else with InvalidArgument.
            const { groupId } = await (0,useFanoutService/* fanOutSession */.mB)({
                title: data.title,
                path: data.path,
                branch: data.branch,
                program: data.program,
                category: data.category,
                autoYes: data.autoYes,
                autoApprove: (_data_autoApprove = data.autoApprove) !== null && _data_autoApprove !== void 0 ? _data_autoApprove : false,
                sessionType: effectiveSessionType,
                initialPrompt: data.initialPrompt,
                permissionMode: (_data_permissionMode = data.permissionMode) !== null && _data_permissionMode !== void 0 ? _data_permissionMode : "",
                extraArgs: (_data_extraArgs = data.extraArgs) !== null && _data_extraArgs !== void 0 ? _data_extraArgs : []
            }, data.fanoutCount);
            router.push((0,useFanoutService/* fanoutComparisonPath */.w3)(groupId));
            return;
        }
        var _data_autoApprove1, _data_createIfMissing, _data_autonomousMode, _data_permissionMode1, _data_aliasName, _data_extraCliFlags, _data_extraArgs1;
        // createSession throws on error, so no null check needed
        const session = await createSession({
            title: data.title,
            path: data.path,
            branch: data.branch,
            program: data.program,
            category: data.category,
            prompt: data.prompt,
            autoYes: data.autoYes,
            autoApprove: (_data_autoApprove1 = data.autoApprove) !== null && _data_autoApprove1 !== void 0 ? _data_autoApprove1 : false,
            workingDir: data.workingDir,
            existingWorktree: data.existingWorktree,
            sessionType: effectiveSessionType,
            createIfMissing: (_data_createIfMissing = data.createIfMissing) !== null && _data_createIfMissing !== void 0 ? _data_createIfMissing : false,
            initialPrompt: data.initialPrompt,
            autonomousMode: (_data_autonomousMode = data.autonomousMode) !== null && _data_autonomousMode !== void 0 ? _data_autonomousMode : false,
            // Remote target composes with sessionType (ADR-001: remote-as-orthogonal-flag) —
            // omitted entirely (not just an empty remote_name) when no remote is selected, so a
            // local session's CreateSessionRequest stays byte-identical to pre-change behavior.
            remote: data.remoteName ? (0,create/* create */.v)(session_pb/* RemoteTargetSchema */.BKV, {
                remoteName: data.remoteName
            }) : undefined,
            permissionMode: (_data_permissionMode1 = data.permissionMode) !== null && _data_permissionMode1 !== void 0 ? _data_permissionMode1 : "",
            aliasName: (_data_aliasName = data.aliasName) !== null && _data_aliasName !== void 0 ? _data_aliasName : "",
            cliFlags: (_data_extraCliFlags = data.extraCliFlags) !== null && _data_extraCliFlags !== void 0 ? _data_extraCliFlags : "",
            extraArgs: (_data_extraArgs1 = data.extraArgs) !== null && _data_extraArgs1 !== void 0 ? _data_extraArgs1 : []
        });
        if (session) {
            // Navigate to the new session so it auto-opens in the detail pane on mobile.
            router.push("/?session=".concat(session.id));
        }
    }, [
        createSession,
        router
    ]);
    // Handle run_workflow omnibar action — fires the workflow via RunWorkflow RPC,
    // then navigates to the newly created session so the user can see it running.
    const handleRunWorkflow = (0,react.useCallback)(async (slug, arg)=>{
        const toastKey = "run-workflow:".concat(slug);
        const wf = workflows.find((w)=>w.slug === slug);
        if (!wf) {
            // Slug matched at detection time but is gone from the (possibly stale) local
            // workflow list by the time Enter is pressed -- surface it rather than
            // swallowing the input silently. Root cause (list going stale) is tracked
            // separately via the in-flight WatchWorkflows streaming RPC.
            console.error("Unknown workflow slug:", slug);
            showActionToast("Workflow '@".concat(slug, "' not found — try reloading"), "error", toastKey);
            return;
        }
        try {
            // runWorkflowRPC (useSessionService.runWorkflow) never throws -- it catches
            // internally and returns null on failure -- so a falsy sessionId is the
            // actual failure signal, not just an absent-navigation no-op.
            const sessionId = await runWorkflowRPC({
                id: wf.id,
                arg
            });
            if (sessionId) {
                router.push("/?session=".concat(sessionId));
            } else {
                showActionToast("Failed to run workflow '@".concat(slug, "' — try again"), "error", toastKey);
            }
        } catch (err) {
            console.error("Failed to run workflow:", err);
            showActionToast((0,connectError/* getErrorMessage */.u)(err, "Failed to run workflow '@".concat(slug, "'")), "error", toastKey);
        }
    }, [
        runWorkflowRPC,
        workflows,
        router,
        showActionToast
    ]);
    const value = {
        isOpen,
        open,
        openInCreationMode,
        openOmnibar,
        close,
        toggle
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(OmnibarContext.Provider, {
        value: value,
        children: [
            children,
            /*#__PURE__*/ (0,jsx_runtime.jsx)(Omnibar, {
                isOpen: isOpen,
                onClose: close,
                onCreateSession: handleCreateSession,
                onNavigateToSession: handleNavigateToSession,
                onNavigateToSessionInNewPane: handleNavigateToSessionInNewPane,
                onRunWorkflow: handleRunWorkflow,
                initialMode: initialMode,
                initialInput: initialInput,
                initialTitle: initialTitle,
                workflows: workflowEntries,
                launcherPresets: launcherPresets,
                launcherPresetsLoading: launcherPresetsLoading,
                launcherPresetsLoadError: launcherPresetsLoadError,
                remotes: configuredRemotes
            })
        ]
    });
}
// Helper to check if target is an input element
function isInputElement(element) {
    if (!element) return false;
    const tagName = element.tagName.toLowerCase();
    return tagName === "input" || tagName === "textarea" || tagName === "select" || element instanceof HTMLElement && element.isContentEditable;
}


/***/ }),

/***/ 45308:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  NP: () => (/* binding */ ThemeProvider),
  DP: () => (/* binding */ useTheme)
});

// UNUSED EXPORTS: THEME_CLASSES

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/styles/theme.css.ts
var theme_css = __webpack_require__(90377);
;// ./src/styles/theme-contract.css.ts
var breakpoints = {sm:'640px',md:'768px',lg:'1024px',xl:'1280px',outer:'390px',fold:'600px',inner:'900px'};
var vars = {color:{textPrimary:'var(--_19ez2rz0)',textSecondary:'var(--_19ez2rz1)',textMuted:'var(--_19ez2rz2)',textDisabled:'var(--_19ez2rz3)',textTertiary:'var(--_19ez2rz4)',textInverse:'var(--_19ez2rz5)',background:'var(--_19ez2rz6)',cardBackground:'var(--_19ez2rz7)',hoverBackground:'var(--_19ez2rz8)',modalBackground:'var(--_19ez2rz9)',overlayBackground:'var(--_19ez2rza)',panelBgSecondary:'var(--_19ez2rzb)',surfaceSubtle:'var(--_19ez2rzc)',surfaceMuted:'var(--_19ez2rzd)',borderColor:'var(--_19ez2rze)',borderSubtle:'var(--_19ez2rzf)',borderMuted:'var(--_19ez2rzg)',borderStrong:'var(--_19ez2rzh)',borderHover:'var(--_19ez2rzi)',modalBorder:'var(--_19ez2rzj)',inputBorder:'var(--_19ez2rzk)',inputFocusBorder:'var(--_19ez2rzl)',primary:'var(--_19ez2rzm)',primaryHover:'var(--_19ez2rzn)',primaryActive:'var(--_19ez2rzo)',primaryDark:'var(--_19ez2rzp)',primaryText:'var(--_19ez2rzq)',onPrimaryFill:'var(--_19ez2rzr)',onSuccessFill:'var(--_19ez2rzs)',onErrorFill:'var(--_19ez2rzt)',success:'var(--_19ez2rzu)',successBg:'var(--_19ez2rzv)',successText:'var(--_19ez2rzw)',warning:'var(--_19ez2rzx)',warningBg:'var(--_19ez2rzy)',warningText:'var(--_19ez2rzz)',error:'var(--_19ez2rz10)',errorBg:'var(--_19ez2rz11)',errorText:'var(--_19ez2rz12)',errorDark:'var(--_19ez2rz13)',critical:'var(--_19ez2rz14)',criticalBg:'var(--_19ez2rz15)',criticalText:'var(--_19ez2rz16)',accentBg:'var(--_19ez2rz17)',accentHover:'var(--_19ez2rz18)',accentText:'var(--_19ez2rz19)',inputBackground:'var(--_19ez2rz1a)',inputText:'var(--_19ez2rz1b)',placeholderColor:'var(--_19ez2rz1c)',terminalBackground:'var(--_19ez2rz1d)',terminalForeground:'var(--_19ez2rz1e)',terminalBorder:'var(--_19ez2rz1f)',terminalHeaderBg:'var(--_19ez2rz1g)',terminalHeaderFg:'var(--_19ez2rz1h)',terminalTabsBg:'var(--_19ez2rz1i)',terminalTextMuted:'var(--_19ez2rz1j)',terminalHoverBg:'var(--_19ez2rz1k)',headerTextPrimary:'var(--_19ez2rz1l)',headerTextSecondary:'var(--_19ez2rz1m)',logError:'var(--_19ez2rz1n)',logWarn:'var(--_19ez2rz1o)',logInfo:'var(--_19ez2rz1p)',logDebug:'var(--_19ez2rz1q)',logTrace:'var(--_19ez2rz1r)',logOnDark:'var(--_19ez2rz1s)',logOnAmber:'var(--_19ez2rz1t)',logLive:'var(--_19ez2rz1u)',gitModified:'var(--_19ez2rz1v)',gitModifiedBg:'var(--_19ez2rz1w)',gitAddedBg:'var(--_19ez2rz1x)',gitDeletedBg:'var(--_19ez2rz1y)',gitAdded:'var(--_19ez2rz1z)',gitDeleted:'var(--_19ez2rz20)',gitRenamed:'var(--_19ez2rz21)',gitUntracked:'var(--_19ez2rz22)',gitConflict:'var(--_19ez2rz23)',glowPrimary:'var(--_19ez2rz24)',glowSecondary:'var(--_19ez2rz25)',scanlineColor:'var(--_19ez2rz26)',terminalCursor:'var(--_19ez2rz27)',statusDot:{running:'var(--_19ez2rz28)',paused:'var(--_19ez2rz29)',idle:'var(--_19ez2rz2a)'}},statusBadge:{approvalBg:'var(--_19ez2rz2b)',approvalFg:'var(--_19ez2rz2c)',approvalBorder:'var(--_19ez2rz2d)',inputBg:'var(--_19ez2rz2e)',inputFg:'var(--_19ez2rz2f)',inputBorder:'var(--_19ez2rz2g)',completeBg:'var(--_19ez2rz2h)',completeFg:'var(--_19ez2rz2i)',completeBorder:'var(--_19ez2rz2j)',uncommittedBg:'var(--_19ez2rz2k)',uncommittedFg:'var(--_19ez2rz2l)',uncommittedBorder:'var(--_19ez2rz2m)',idleBg:'var(--_19ez2rz2n)',idleFg:'var(--_19ez2rz2o)',idleBorder:'var(--_19ez2rz2p)',staleFg:'var(--_19ez2rz2q)',processingBg:'var(--_19ez2rz2r)',processingFg:'var(--_19ez2rz2s)',processingBorder:'var(--_19ez2rz2t)'},font:{mono:'var(--_19ez2rz2u)',sans:'var(--_19ez2rz2v)',display:'var(--_19ez2rz2w)'},space:{'0':'var(--_19ez2rz2x)','1':'var(--_19ez2rz2y)','2':'var(--_19ez2rz2z)','3':'var(--_19ez2rz30)','4':'var(--_19ez2rz31)','6':'var(--_19ez2rz32)','8':'var(--_19ez2rz33)','12':'var(--_19ez2rz34)','16':'var(--_19ez2rz35)'},radii:{sm:'var(--_19ez2rz36)',md:'var(--_19ez2rz37)',lg:'var(--_19ez2rz38)',full:'var(--_19ez2rz39)'},fontSize:{xs:'var(--_19ez2rz3a)',sm:'var(--_19ez2rz3b)',base:'var(--_19ez2rz3c)',lg:'var(--_19ez2rz3d)',xl:'var(--_19ez2rz3e)'},fontWeight:{normal:'var(--_19ez2rz3f)',medium:'var(--_19ez2rz3g)',semibold:'var(--_19ez2rz3h)',bold:'var(--_19ez2rz3i)'},shadow:{none:'var(--_19ez2rz3j)',sm:'var(--_19ez2rz3k)',md:'var(--_19ez2rz3l)',lg:'var(--_19ez2rz3m)'},transition:{fast:'var(--_19ez2rz3n)',base:'var(--_19ez2rz3o)',slow:'var(--_19ez2rz3p)'}};
var zIndex = {base:0,tableHeader:1,raised:10,terminalOverlay:15,terminalOverlayPill:20,terminalPaneChrome:25,terminalFilterPanel:26,header:100,dropdown:500,slideOver:700,modal:1000,bottomNavMoreBackdrop:1040,bottomNavMoreSheet:1045,bottomNav:1050,mobilePickerBackdrop:1060,mobilePickerSheet:1065,dialog:1070,toast:1080,floatingTerminalUI:1085,tooltip:1100};
;// ./src/lib/contexts/ThemeContext.tsx
/* __next_internal_client_entry_do_not_use__ THEME_CLASSES,useTheme,ThemeProvider auto */ 



const STORAGE_KEY = "stapler-theme";
/** Maps theme names to their vanilla-extract CSS class strings */ const THEME_CLASSES = {
    matrix: theme_css.matrixTheme,
    cyberpunk77: theme_css.cyberpunk77Theme,
    wh40k: theme_css.wh40kTheme,
    clean: theme_css.cleanTheme,
    light: theme_css.lightTheme,
    dark: theme_css.darkTheme
};
const ALL_THEME_CLASSES = Object.values(THEME_CLASSES);
const CUSTOM_PREFIX = "custom:";
/** Last applied custom theme (`{id, cls, props}`), read by the FOUC script in app/layout.tsx. */ const CUSTOM_CACHE_KEY = "stapler-theme-custom";
const isBuiltin = (name)=>Object.hasOwn(THEME_CLASSES, name);
function readCustomCache() {
    try {
        var _localStorage_getItem;
        return JSON.parse((_localStorage_getItem = localStorage.getItem(CUSTOM_CACHE_KEY)) !== null && _localStorage_getItem !== void 0 ? _localStorage_getItem : "null");
    } catch (e) {
        return null;
    }
}
const ThemeContext = /*#__PURE__*/ (0,react.createContext)(null);
function useTheme() {
    const ctx = (0,react.useContext)(ThemeContext);
    if (!ctx) {
        // Return a no-op fallback when used outside a ThemeProvider.
        // This allows components to be rendered in test environments or embedded contexts.
        return {
            theme: "clean",
            setTheme: ()=>{},
            availableThemes: [],
            customThemes: []
        };
    }
    return ctx;
}
function ThemeProvider(param) {
    let { children, initialTheme = "clean" } = param;
    const [theme, setThemeState] = (0,react.useState)(initialTheme);
    const [customThemes, setCustomThemes] = (0,react.useState)([]);
    const customThemesRef = (0,react.useRef)([]);
    // Latest selection, so the async /api/themes response never clobbers a choice made while it was in flight.
    const themeRef = (0,react.useRef)(initialTheme);
    const initialized = (0,react.useRef)(false);
    // On first mount, read localStorage and apply the persisted theme
    (0,react.useEffect)(()=>{
        if (initialized.current) return;
        initialized.current = true;
        let persisted = initialTheme;
        try {
            const stored = localStorage.getItem(STORAGE_KEY);
            if (stored && (isBuiltin(stored) || stored.startsWith(CUSTOM_PREFIX))) {
                persisted = stored;
            }
        } catch (e) {
        // localStorage unavailable
        }
        if (isBuiltin(persisted)) {
            applyTheme(persisted, []);
        } else {
            // A persisted custom theme is already painted by the FOUC script from its cache; adopt those
            // overrides so a later switch clears them, then confirm against /api/themes.
            const cached = readCustomCache();
            if ((cached === null || cached === void 0 ? void 0 : cached.id) === persisted) appliedOverrides = Object.keys(cached.props);
        }
        themeRef.current = persisted;
        setThemeState(persisted);
        if (typeof fetch !== "function") return;
        fetch("/api/themes").then((res)=>res.ok ? res.json() : {
                themes: []
            }).then((body)=>{
            var _body_themes;
            const themes = ((_body_themes = body.themes) !== null && _body_themes !== void 0 ? _body_themes : []).map((t)=>{
                var _t_tokens;
                return {
                    ...t,
                    tokens: (_t_tokens = t.tokens) !== null && _t_tokens !== void 0 ? _t_tokens : {}
                };
            });
            customThemesRef.current = themes;
            setCustomThemes(themes);
            const current = themeRef.current;
            if (!current.startsWith(CUSTOM_PREFIX)) return;
            if (themes.some((t)=>"".concat(CUSTOM_PREFIX).concat(t.id) === current)) {
                applyTheme(current, themes);
            } else {
                // Persisted custom theme no longer exists (file removed): fall back rather than show nothing selected.
                themeRef.current = initialTheme;
                applyTheme(initialTheme, []);
                setThemeState(initialTheme);
                try {
                    localStorage.setItem(STORAGE_KEY, initialTheme);
                    localStorage.removeItem(CUSTOM_CACHE_KEY);
                } catch (e) {
                // ignore
                }
            }
        }).catch(()=>{
        // server without /api/themes: built-in themes only
        });
    }, [
        initialTheme
    ]);
    const setTheme = (0,react.useCallback)((name)=>{
        themeRef.current = name;
        applyTheme(name, customThemesRef.current);
        setThemeState(name);
        try {
            localStorage.setItem(STORAGE_KEY, name);
        } catch (e) {
        // ignore
        }
    }, []);
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(ThemeContext.Provider, {
        value: {
            theme,
            setTheme,
            availableThemes: Object.keys(THEME_CLASSES),
            customThemes
        },
        children: children
    });
}
let appliedOverrides = [];
/** `var(--color-primary__x1)` -> `--color-primary__x1`, walking a dotted path through the contract. */ function cssVarName(path) {
    let node = vars;
    for (const key of path.split(".")){
        if (typeof node !== "object" || node === null || !(key in node)) return null;
        node = node[key];
    }
    const match = typeof node === "string" ? /^var\((--[^),\s]+)/.exec(node) : null;
    return match ? match[1] : null;
}
function applyTheme(name, custom) {
    if (typeof document === "undefined") return;
    const style = document.documentElement.style;
    appliedOverrides.forEach((prop)=>style.removeProperty(prop));
    appliedOverrides = [];
    const user = name.startsWith(CUSTOM_PREFIX) ? custom.find((t)=>t.id === name.slice(CUSTOM_PREFIX.length)) : undefined;
    if (!user) {
        applyThemeClass(isBuiltin(name) ? name : "clean");
        return;
    }
    const base = isBuiltin(user.base) ? user.base : "clean";
    applyThemeClass(base);
    const props = {};
    for (const [path, value] of Object.entries(user.tokens)){
        const prop = cssVarName(path);
        if (!prop) continue;
        style.setProperty(prop, value);
        props[prop] = value;
    }
    appliedOverrides = Object.keys(props);
    try {
        localStorage.setItem(CUSTOM_CACHE_KEY, JSON.stringify({
            id: name,
            cls: THEME_CLASSES[base],
            props
        }));
    } catch (e) {
    // ignore
    }
}
function applyThemeClass(name) {
    if (typeof document === "undefined") return;
    const html = document.documentElement;
    // Remove all known theme classes, add the new one
    html.classList.remove(...ALL_THEME_CLASSES);
    html.classList.add(THEME_CLASSES[name]);
}


/***/ }),

/***/ 49051:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

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

/***/ 56230:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

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

/***/ 62182:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   eT: () => (/* binding */ useProbeProgram)
/* harmony export */ });
/* unused harmony exports firstToken, toUiState */
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(45640);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(16294);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(66144);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(23575);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(21694);
// +feature: settings-programs
/* __next_internal_client_entry_do_not_use__ firstToken,toUiState,useProbeProgram auto */ 



function firstToken(command) {
    var _command_trim_split_;
    return (_command_trim_split_ = command.trim().split(/\s+/)[0]) !== null && _command_trim_split_ !== void 0 ? _command_trim_split_ : "";
}
/** Maps a response to UI state; never yields notFound for ERROR/BUSY. */ function toUiState(res) {
    if (res.isWrapper) return {
        kind: "wrapper"
    };
    const path = res.resolvedPath;
    switch(res.probeStatus){
        case _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .ProbeStatus */ .POv.NOT_FOUND:
            return {
                kind: "notFound"
            };
        case _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .ProbeStatus */ .POv.ERROR:
        case _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .ProbeStatus */ .POv.BUSY:
            return {
                kind: "busyOrError"
            };
        case _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .ProbeStatus */ .POv.NEEDS_CONFIRM:
            return {
                kind: "needsConfirm",
                path
            };
        case _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .ProbeStatus */ .POv.TIMEOUT:
            return {
                kind: "timeout",
                path
            };
        case _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .ProbeStatus */ .POv.FOUND_NO_FLAGS:
            return {
                kind: "noFlags",
                path
            };
        default:
            return res.found ? {
                kind: "found",
                path,
                flagCount: res.flags.length,
                flags: res.flags
            } : {
                kind: "notFound"
            };
    }
}
// Transient or non-final results are re-probed on the next check.
const UNSETTLED = new Set([
    "idle",
    "checking",
    "busyOrError",
    "transportError"
]);
const MEMOIZABLE = new Set([
    "found",
    "noFlags",
    "wrapper",
    "notFound"
]);
function toFailureState(err) {
    // Unimplemented = kill switch off. Anything else (network, PermissionDenied,
    // 403 from ProbeGuard) must read "couldn't check", never "not found".
    return _connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__/* .ConnectError */ .T.from(err).code === _connectrpc_connect__WEBPACK_IMPORTED_MODULE_4__/* .Code */ .C.Unimplemented ? {
        kind: "disabled"
    } : {
        kind: "transportError"
    };
}
function useProbeProgram(command, mode) {
    const [state, setState] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)({
        kind: "idle"
    });
    const client = (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>(0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_5__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_2__/* .getConnectTransport */ .qW)()), []);
    const stateRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(state);
    stateRef.current = state;
    const tokenRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(0);
    const controllerRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    const inFlightRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    const settledCommandRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    const memoRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(new Map());
    const cancel = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>{
        var _controllerRef_current;
        tokenRef.current += 1;
        (_controllerRef_current = controllerRef.current) === null || _controllerRef_current === void 0 ? void 0 : _controllerRef_current.abort();
        controllerRef.current = null;
        inFlightRef.current = null;
    }, []);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        settledCommandRef.current = null;
        setState({
            kind: "idle"
        });
        return cancel;
    }, [
        command,
        cancel
    ]);
    const check = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(function() {
        let opts = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : {};
        const cmd = command.trim();
        if (!cmd) {
            cancel();
            setState({
                kind: "idle"
            });
            return;
        }
        const explicit = opts.explicit === true;
        // An explicit Check supersedes an implicit probe of the same command (blur fires before the click).
        if (inFlightRef.current === cmd && !explicit) return;
        if (!explicit && !opts.immediate) {
            const kind = stateRef.current.kind;
            if (settledCommandRef.current === cmd && !UNSETTLED.has(kind)) return;
            const memo = memoRef.current.get(cmd);
            if (memo) {
                settledCommandRef.current = cmd;
                setState(memo);
                return;
            }
        }
        cancel();
        const token = tokenRef.current;
        const controller = new AbortController();
        controllerRef.current = controller;
        inFlightRef.current = cmd;
        setState({
            kind: "checking"
        });
        const request = {
            command: cmd,
            confirmExecute: explicit,
            resolveOnly: mode === "picker" && !explicit
        };
        client.probeProgram(request, {
            signal: controller.signal
        }).then((res)=>toUiState(res), (err)=>controller.signal.aborted ? null : toFailureState(err)).then((next)=>{
            if (next === null || token !== tokenRef.current) return;
            inFlightRef.current = null;
            settledCommandRef.current = cmd;
            if (MEMOIZABLE.has(next.kind)) memoRef.current.set(cmd, next);
            setState(next);
        });
    }, [
        client,
        command,
        mode,
        cancel
    ]);
    return {
        state,
        checkedToken: firstToken(command),
        check
    };
}


/***/ }),

/***/ 63251:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   P: () => (/* binding */ ALIAS_NAME_RE),
/* harmony export */   t: () => (/* binding */ AliasDetector)
/* harmony export */ });
/* harmony import */ var _types__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(11804);
// +feature: alias-detector
/**
 * AliasDetector detects @alias-name[:branch][ label][ --flags] syntax.
 * Priority 36 — after WorkflowDetector (25) and NewSessionDetector (35), before GitHubShorthandDetector (40).
 *
 * IMPORTANT: AliasDetector must claim ALL input starting with "@" and never return null
 * for such input. This prevents SessionSearchDetector from claiming "@foo" input.
 */ 
/** Name-only form of the alias regex (without the leading "@"). */ const ALIAS_NAME_RE = /^[\w-]+$/;
/**
 * AliasDetector matches the full alias grammar:
 *   @<name>[:<branch>][ <label text>][ --<extra-flags>]
 *
 * Three modes:
 *   - "@" alone or "@<partial>" (no space) → AliasBrowse (palette/completion mode)
 *   - "@<name> " (with space, known alias) → Alias (invocation mode)
 *   - "@<name> " (with space, unknown alias) → AliasNotFound
 */ class AliasDetector {
    detect(input) {
        // Only claim input that starts with "@"
        if (!input.startsWith("@")) return null;
        const trimmed = input.trimEnd();
        // hasSpace is true when the original input has a space after the "@name" portion,
        // signalling that the user has committed to a name (invocation mode).
        const hasSpace = input !== trimmed || /^@[\w-]+(?::[^\s]+)?\s/.test(input);
        // "@" alone → browse palette
        if (trimmed === "@") {
            return {
                type: _types__WEBPACK_IMPORTED_MODULE_0__/* .InputType */ .N.AliasBrowse,
                confidence: 1.0,
                parsedValue: trimmed,
                suggestedName: "",
                metadata: {
                    partial: ""
                }
            };
        }
        // "@<partial>" with no space → completion/browse mode
        if (/^@[\w-]+$/.test(trimmed) && !hasSpace) {
            const partial = trimmed.slice(1); // strip "@"
            return {
                type: _types__WEBPACK_IMPORTED_MODULE_0__/* .InputType */ .N.AliasBrowse,
                confidence: 1.0,
                parsedValue: trimmed,
                suggestedName: partial,
                metadata: {
                    partial
                }
            };
        }
        // "@<name> ..." with space → invocation mode; parse full grammar
        // Extract just the name+optional-branch prefix to identify the alias
        const nameMatch = trimmed.match(/^@([\w-]+)(?::([^\s]+))?(?:\s|$)/);
        if (!nameMatch) {
            // Starts with @ but doesn't match any recognizable pattern → AliasBrowse fallback
            return {
                type: _types__WEBPACK_IMPORTED_MODULE_0__/* .InputType */ .N.AliasBrowse,
                confidence: 0.5,
                parsedValue: trimmed,
                suggestedName: "",
                metadata: {
                    partial: trimmed.slice(1)
                }
            };
        }
        const aliasName = nameMatch[1];
        var _nameMatch_;
        const branch = (_nameMatch_ = nameMatch[2]) !== null && _nameMatch_ !== void 0 ? _nameMatch_ : undefined;
        // Find the alias (case-insensitive)
        const found = this.aliases.find((a)=>a.name.toLowerCase() === aliasName.toLowerCase());
        // Parse the rest of the string (everything after "@name[:branch] ")
        const afterPrefix = trimmed.slice(nameMatch[0].length).trim();
        let label;
        let extraFlags;
        if (afterPrefix) {
            const flagIdx = afterPrefix.indexOf(" --");
            if (flagIdx >= 0) {
                label = afterPrefix.slice(0, flagIdx).trim() || undefined;
                extraFlags = afterPrefix.slice(flagIdx + 1).trim() || undefined;
            } else if (afterPrefix.startsWith("--")) {
                extraFlags = afterPrefix;
            } else {
                label = afterPrefix || undefined;
            }
        }
        if (!found) {
            return {
                type: _types__WEBPACK_IMPORTED_MODULE_0__/* .InputType */ .N.AliasNotFound,
                confidence: 1.0,
                parsedValue: trimmed,
                suggestedName: aliasName,
                metadata: {
                    slug: aliasName
                }
            };
        }
        return {
            type: _types__WEBPACK_IMPORTED_MODULE_0__/* .InputType */ .N.Alias,
            confidence: 1.0,
            parsedValue: trimmed,
            suggestedName: found.name,
            metadata: {
                aliasName: found.name,
                branch,
                label,
                extraFlags,
                alias: found
            }
        };
    }
    constructor(aliases){
        this.name = "AliasDetector";
        this.priority = 36;
        this.aliases = aliases;
    }
}


/***/ }),

/***/ 64792:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   A: () => (/* binding */ useAuth),
/* harmony export */   AuthProvider: () => (/* binding */ AuthProvider)
/* harmony export */ });
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(61574);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(55494);
/* harmony import */ var _lib_auth_passkey__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(15681);
/* __next_internal_client_entry_do_not_use__ AuthProvider,useAuth auto */ 


const AuthContext = /*#__PURE__*/ (0,react__WEBPACK_IMPORTED_MODULE_1__.createContext)(null);
function AuthProvider(param) {
    let { children } = param;
    const [state, setState] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)({
        authEnabled: false,
        authenticated: false,
        hasCredentials: false,
        setupActive: false,
        loading: true
    });
    const refresh = (0,react__WEBPACK_IMPORTED_MODULE_1__.useCallback)(async ()=>{
        try {
            const status = await (0,_lib_auth_passkey__WEBPACK_IMPORTED_MODULE_2__/* .getAuthStatus */ .Pq)();
            setState({
                authEnabled: status.auth_enabled,
                authenticated: status.authenticated,
                hasCredentials: status.has_credentials,
                setupActive: status.setup_active,
                loading: false
            });
        } catch (e) {
            // Server unreachable or auth not configured – treat as not-authenticated
            setState((prev)=>({
                    ...prev,
                    loading: false,
                    authEnabled: false
                }));
        }
    }, []);
    const logout = (0,react__WEBPACK_IMPORTED_MODULE_1__.useCallback)(async ()=>{
        await (0,_lib_auth_passkey__WEBPACK_IMPORTED_MODULE_2__/* .logout */ .ri)();
        await refresh();
    }, [
        refresh
    ]);
    (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        refresh();
    }, [
        refresh
    ]);
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(AuthContext.Provider, {
        value: {
            ...state,
            refresh,
            logout
        },
        children: children
    });
}
function useAuth() {
    const ctx = (0,react__WEBPACK_IMPORTED_MODULE_1__.useContext)(AuthContext);
    if (!ctx) {
        throw new Error("useAuth must be used within AuthProvider");
    }
    return ctx;
}


/***/ }),

/***/ 64887:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

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

/***/ 88242:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   $: () => (/* binding */ isAutoApproveSupported),
/* harmony export */   H: () => (/* binding */ isApprovalExtensionSupported)
/* harmony export */ });
// Agents auto-approve (yolo mode) can inject a bypass flag for. Mirrors the backend's
// yoloFlagByAgent map (session/instance_tmux.go) -- keep in sync manually; small enough
// surface that a shared RPC isn't warranted for two agents.
const AUTO_APPROVE_SUPPORTED_AGENTS = [
    "claude",
    "aider"
];
// isAutoApproveSupported reports whether program is a recognized agent auto-approve can
// inject a bypass flag for. An empty program ("System default" at create time) is treated
// as supported -- the option's own label documents it resolves to claude, and the server
// re-validates against the actually-resolved program anyway (CreateSession's
// AutoApproveSupported guard), so this is only an optimistic UI default, not a
// correctness gap. A persisted session's program is always server-resolved and non-empty.
function isAutoApproveSupported(program) {
    var _program_trim_split_;
    if (program.trim() === "") return true;
    var _program_trim_split__split_pop;
    const base = (_program_trim_split__split_pop = (_program_trim_split_ = program.trim().split(/\s+/)[0]) === null || _program_trim_split_ === void 0 ? void 0 : _program_trim_split_.split("/").pop()) !== null && _program_trim_split__split_pop !== void 0 ? _program_trim_split__split_pop : "";
    return AUTO_APPROVE_SUPPORTED_AGENTS.includes(base);
}
// Programs the pi approval extension (ssq-approval.ts, Phase 4) covers. Deliberately
// narrower than AUTO_APPROVE_SUPPORTED_AGENTS above -- "claude" enforces approval rules
// via its own hook mechanism (not this extension), and "opencode" has its own out-of-scope
// hook, so neither belongs here even though both are legitimate agents elsewhere.
const APPROVAL_EXTENSION_SUPPORTED_AGENTS = [
    "pi"
];
// isApprovalExtensionSupported reports whether program is a recognized agent the pi
// approval extension applies to. Used to decide whether the creation panel's capability
// warning area is even eligible to render for the selected program (Story 3.1.2) -- the
// warning itself still depends on the program's live extension-health signal (Phase 4).
function isApprovalExtensionSupported(program) {
    var _program_trim_split_;
    var _program_trim_split__split_pop;
    const base = (_program_trim_split__split_pop = (_program_trim_split_ = program.trim().split(/\s+/)[0]) === null || _program_trim_split_ === void 0 ? void 0 : _program_trim_split_.split("/").pop()) !== null && _program_trim_split__split_pop !== void 0 ? _program_trim_split__split_pop : "";
    return APPROVAL_EXTENSION_SUPPORTED_AGENTS.includes(base);
}


/***/ }),

/***/ 90377:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

__webpack_require__.r(__webpack_exports__);
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   breakpoints: () => (/* binding */ breakpoints),
/* harmony export */   cleanTheme: () => (/* binding */ cleanTheme),
/* harmony export */   cyberpunk77Theme: () => (/* binding */ cyberpunk77Theme),
/* harmony export */   darkTheme: () => (/* binding */ darkTheme),
/* harmony export */   lightTheme: () => (/* binding */ lightTheme),
/* harmony export */   matrixTheme: () => (/* binding */ matrixTheme),
/* harmony export */   vars: () => (/* binding */ vars),
/* harmony export */   wh40kTheme: () => (/* binding */ wh40kTheme),
/* harmony export */   zIndex: () => (/* binding */ zIndex)
/* harmony export */ });
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(65916);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__);

var breakpoints = {sm:'640px',md:'768px',lg:'1024px',xl:'1280px',outer:'390px',fold:'600px',inner:'900px'};
var cleanTheme = '_1b1axoi5';
var cyberpunk77Theme = '_1b1axoi3';
var darkTheme = '_1b1axoi1';
var lightTheme = '_1b1axoi0';
var matrixTheme = '_1b1axoi2';
var vars = {color:{textPrimary:'var(--_19ez2rz0)',textSecondary:'var(--_19ez2rz1)',textMuted:'var(--_19ez2rz2)',textDisabled:'var(--_19ez2rz3)',textTertiary:'var(--_19ez2rz4)',textInverse:'var(--_19ez2rz5)',background:'var(--_19ez2rz6)',cardBackground:'var(--_19ez2rz7)',hoverBackground:'var(--_19ez2rz8)',modalBackground:'var(--_19ez2rz9)',overlayBackground:'var(--_19ez2rza)',panelBgSecondary:'var(--_19ez2rzb)',surfaceSubtle:'var(--_19ez2rzc)',surfaceMuted:'var(--_19ez2rzd)',borderColor:'var(--_19ez2rze)',borderSubtle:'var(--_19ez2rzf)',borderMuted:'var(--_19ez2rzg)',borderStrong:'var(--_19ez2rzh)',borderHover:'var(--_19ez2rzi)',modalBorder:'var(--_19ez2rzj)',inputBorder:'var(--_19ez2rzk)',inputFocusBorder:'var(--_19ez2rzl)',primary:'var(--_19ez2rzm)',primaryHover:'var(--_19ez2rzn)',primaryActive:'var(--_19ez2rzo)',primaryDark:'var(--_19ez2rzp)',primaryText:'var(--_19ez2rzq)',onPrimaryFill:'var(--_19ez2rzr)',onSuccessFill:'var(--_19ez2rzs)',onErrorFill:'var(--_19ez2rzt)',success:'var(--_19ez2rzu)',successBg:'var(--_19ez2rzv)',successText:'var(--_19ez2rzw)',warning:'var(--_19ez2rzx)',warningBg:'var(--_19ez2rzy)',warningText:'var(--_19ez2rzz)',error:'var(--_19ez2rz10)',errorBg:'var(--_19ez2rz11)',errorText:'var(--_19ez2rz12)',errorDark:'var(--_19ez2rz13)',critical:'var(--_19ez2rz14)',criticalBg:'var(--_19ez2rz15)',criticalText:'var(--_19ez2rz16)',accentBg:'var(--_19ez2rz17)',accentHover:'var(--_19ez2rz18)',accentText:'var(--_19ez2rz19)',inputBackground:'var(--_19ez2rz1a)',inputText:'var(--_19ez2rz1b)',placeholderColor:'var(--_19ez2rz1c)',terminalBackground:'var(--_19ez2rz1d)',terminalForeground:'var(--_19ez2rz1e)',terminalBorder:'var(--_19ez2rz1f)',terminalHeaderBg:'var(--_19ez2rz1g)',terminalHeaderFg:'var(--_19ez2rz1h)',terminalTabsBg:'var(--_19ez2rz1i)',terminalTextMuted:'var(--_19ez2rz1j)',terminalHoverBg:'var(--_19ez2rz1k)',headerTextPrimary:'var(--_19ez2rz1l)',headerTextSecondary:'var(--_19ez2rz1m)',logError:'var(--_19ez2rz1n)',logWarn:'var(--_19ez2rz1o)',logInfo:'var(--_19ez2rz1p)',logDebug:'var(--_19ez2rz1q)',logTrace:'var(--_19ez2rz1r)',logOnDark:'var(--_19ez2rz1s)',logOnAmber:'var(--_19ez2rz1t)',logLive:'var(--_19ez2rz1u)',gitModified:'var(--_19ez2rz1v)',gitModifiedBg:'var(--_19ez2rz1w)',gitAddedBg:'var(--_19ez2rz1x)',gitDeletedBg:'var(--_19ez2rz1y)',gitAdded:'var(--_19ez2rz1z)',gitDeleted:'var(--_19ez2rz20)',gitRenamed:'var(--_19ez2rz21)',gitUntracked:'var(--_19ez2rz22)',gitConflict:'var(--_19ez2rz23)',glowPrimary:'var(--_19ez2rz24)',glowSecondary:'var(--_19ez2rz25)',scanlineColor:'var(--_19ez2rz26)',terminalCursor:'var(--_19ez2rz27)',statusDot:{running:'var(--_19ez2rz28)',paused:'var(--_19ez2rz29)',idle:'var(--_19ez2rz2a)'}},statusBadge:{approvalBg:'var(--_19ez2rz2b)',approvalFg:'var(--_19ez2rz2c)',approvalBorder:'var(--_19ez2rz2d)',inputBg:'var(--_19ez2rz2e)',inputFg:'var(--_19ez2rz2f)',inputBorder:'var(--_19ez2rz2g)',completeBg:'var(--_19ez2rz2h)',completeFg:'var(--_19ez2rz2i)',completeBorder:'var(--_19ez2rz2j)',uncommittedBg:'var(--_19ez2rz2k)',uncommittedFg:'var(--_19ez2rz2l)',uncommittedBorder:'var(--_19ez2rz2m)',idleBg:'var(--_19ez2rz2n)',idleFg:'var(--_19ez2rz2o)',idleBorder:'var(--_19ez2rz2p)',staleFg:'var(--_19ez2rz2q)',processingBg:'var(--_19ez2rz2r)',processingFg:'var(--_19ez2rz2s)',processingBorder:'var(--_19ez2rz2t)'},font:{mono:'var(--_19ez2rz2u)',sans:'var(--_19ez2rz2v)',display:'var(--_19ez2rz2w)'},space:{'0':'var(--_19ez2rz2x)','1':'var(--_19ez2rz2y)','2':'var(--_19ez2rz2z)','3':'var(--_19ez2rz30)','4':'var(--_19ez2rz31)','6':'var(--_19ez2rz32)','8':'var(--_19ez2rz33)','12':'var(--_19ez2rz34)','16':'var(--_19ez2rz35)'},radii:{sm:'var(--_19ez2rz36)',md:'var(--_19ez2rz37)',lg:'var(--_19ez2rz38)',full:'var(--_19ez2rz39)'},fontSize:{xs:'var(--_19ez2rz3a)',sm:'var(--_19ez2rz3b)',base:'var(--_19ez2rz3c)',lg:'var(--_19ez2rz3d)',xl:'var(--_19ez2rz3e)'},fontWeight:{normal:'var(--_19ez2rz3f)',medium:'var(--_19ez2rz3g)',semibold:'var(--_19ez2rz3h)',bold:'var(--_19ez2rz3i)'},shadow:{none:'var(--_19ez2rz3j)',sm:'var(--_19ez2rz3k)',md:'var(--_19ez2rz3l)',lg:'var(--_19ez2rz3m)'},transition:{fast:'var(--_19ez2rz3n)',base:'var(--_19ez2rz3o)',slow:'var(--_19ez2rz3p)'}};
var wh40kTheme = '_1b1axoi4';
var zIndex = {base:0,tableHeader:1,raised:10,terminalOverlay:15,terminalOverlayPill:20,terminalPaneChrome:25,terminalFilterPanel:26,header:100,dropdown:500,slideOver:700,modal:1000,bottomNavMoreBackdrop:1040,bottomNavMoreSheet:1045,bottomNav:1050,mobilePickerBackdrop:1060,mobilePickerSheet:1065,dialog:1070,toast:1080,floatingTerminalUI:1085,tooltip:1100};

/***/ }),

/***/ 95923:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   AM: () => (/* binding */ groupLabel),
/* harmony export */   R$: () => (/* binding */ radioGroup),
/* harmony export */   U1: () => (/* binding */ radioBtn),
/* harmony export */   UE: () => (/* binding */ radioBtnActive),
/* harmony export */   ZZ: () => (/* binding */ field),
/* harmony export */   aK: () => (/* binding */ hint)
/* harmony export */ });
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FRadioGroup_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA6WTzW6DMBCE7zzFHpuDo0Bo1ToPEzl4Iasam65N_qq_e4WTqIGEUqkXkNjxtzMDzNcpv_r8mC_gMwHQ5BujjhJKg4dVAvEuNDEWgZyVUDjT1rabVKqRsFP8JMQ6fcNTxqfsOFslX8n8ykwjs3Q2CE8nHMqXm9nqOt8jVdtwp6iionDG8XCW9ndlj_2Pubwk23M376492DLCGqU12erB_aHNxU_QfwXdONbIgpWm1t9pXm40EtLmAN4Z0gMZnlWqeK_YtVZLCKysbxSjDb_2CVC07LtZ48gG5O5RPEznl6_MgcU89YDKY78wuXU75Fjb7er_ktfbkA9t0GyALV3RerEjTxuDEe_aYMiihGykgDpuuciEK0uPIap76HzCaz3_5X1Mxqj7MfI_tWMnsbaPfZ74vdR4hKwjfQPRcZsA_AMAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(91447);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FRadioGroup_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA6WTzW6DMBCE7zzFHpuDo0Bo1ToPEzl4Iasam65N_qq_e4WTqIGEUqkXkNjxtzMDzNcpv_r8mC_gMwHQ5BujjhJKg4dVAvEuNDEWgZyVUDjT1rabVKqRsFP8JMQ6fcNTxqfsOFslX8n8ykwjs3Q2CE8nHMqXm9nqOt8jVdtwp6iionDG8XCW9ndlj_2Pubwk23M376492DLCGqU12erB_aHNxU_QfwXdONbIgpWm1t9pXm40EtLmAN4Z0gMZnlWqeK_YtVZLCKysbxSjDb_2CVC07LtZ48gG5O5RPEznl6_MgcU89YDKY78wuXU75Fjb7er_ktfbkA9t0GyALV3RerEjTxuDEe_aYMiihGykgDpuuciEK0uPIap76HzCaz3_5X1Mxqj7MfI_tWMnsbaPfZ74vdR4hKwjfQPRcZsA_AMAAA_22_7D__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FRadioGroup_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA6WTzW6DMBCE7zzFHpuDo0Bo1ToPEzl4Iasam65N_qq_e4WTqIGEUqkXkNjxtzMDzNcpv_r8mC_gMwHQ5BujjhJKg4dVAvEuNDEWgZyVUDjT1rabVKqRsFP8JMQ6fcNTxqfsOFslX8n8ykwjs3Q2CE8nHMqXm9nqOt8jVdtwp6iionDG8XCW9ndlj_2Pubwk23M376492DLCGqU12erB_aHNxU_QfwXdONbIgpWm1t9pXm40EtLmAN4Z0gMZnlWqeK_YtVZLCKysbxSjDb_2CVC07LtZ48gG5O5RPEznl6_MgcU89YDKY78wuXU75Fjb7er_ktfbkA9t0GyALV3RerEjTxuDEe_aYMiihGykgDpuuciEK0uPIap76HzCaz3_5X1Mxqj7MfI_tWMnsbaPfZ74vdR4hKwjfQPRcZsA_AMAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__);

var field = '_1r8s4y40';
var groupLabel = '_1r8s4y41';
var hint = '_1r8s4y45';
var radioBtn = '_1r8s4y43';
var radioBtnActive = '_1r8s4y44';
var radioGroup = '_1r8s4y42';

/***/ }),

/***/ 98764:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   z: () => (/* binding */ RadioGroup)
/* harmony export */ });
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(61574);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(55494);
/* harmony import */ var _RadioGroup_css__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(95923);
/* __next_internal_client_entry_do_not_use__ RadioGroup auto */ 


/**
 * Generic ARIA radiogroup: role="radiogroup" + role="radio" + aria-checked per option,
 * roving tabindex, and arrow-key cycling where arrow keys both move focus AND select
 * (no separate Space/Enter confirmation step).
 */ function RadioGroup(param) {
    let { options, value, onChange, groupLabel, groupLabelId, hintForValue, trailingContent } = param;
    const autoLabelId = (0,react__WEBPACK_IMPORTED_MODULE_1__.useId)();
    const autoHintId = (0,react__WEBPACK_IMPORTED_MODULE_1__.useId)();
    const labelId = groupLabelId !== null && groupLabelId !== void 0 ? groupLabelId : autoLabelId;
    const currentIndex = options.findIndex((o)=>o.value === value);
    const hasSelection = currentIndex !== -1;
    const hintText = hintForValue === null || hintForValue === void 0 ? void 0 : hintForValue(value);
    function handleKeyDown(e) {
        const fromIndex = hasSelection ? currentIndex : 0;
        if (e.key === "ArrowRight" || e.key === "ArrowDown") {
            e.preventDefault();
            const next = (fromIndex + 1) % options.length;
            onChange(options[next].value);
        } else if (e.key === "ArrowLeft" || e.key === "ArrowUp") {
            e.preventDefault();
            const prev = (fromIndex - 1 + options.length) % options.length;
            onChange(options[prev].value);
        }
    }
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
        className: _RadioGroup_css__WEBPACK_IMPORTED_MODULE_2__/* .field */ .ZZ,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("span", {
                id: labelId,
                className: _RadioGroup_css__WEBPACK_IMPORTED_MODULE_2__/* .groupLabel */ .AM,
                children: groupLabel
            }),
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
                role: "radiogroup",
                "aria-labelledby": labelId,
                "aria-describedby": hintText ? autoHintId : undefined,
                className: _RadioGroup_css__WEBPACK_IMPORTED_MODULE_2__/* .radioGroup */ .R$,
                onKeyDown: handleKeyDown,
                children: [
                    options.map((option, idx)=>/*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("button", {
                            role: "radio",
                            "aria-checked": value === option.value,
                            tabIndex: value === option.value ? 0 : !hasSelection && idx === 0 ? 0 : -1,
                            type: "button",
                            onClick: ()=>onChange(option.value),
                            "data-testid": option.dataTestId,
                            className: [
                                _RadioGroup_css__WEBPACK_IMPORTED_MODULE_2__/* .radioBtn */ .U1,
                                value === option.value ? _RadioGroup_css__WEBPACK_IMPORTED_MODULE_2__/* .radioBtnActive */ .UE : ""
                            ].filter(Boolean).join(" "),
                            children: option.label
                        }, option.value)),
                    trailingContent
                ]
            }),
            hintText && /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("span", {
                id: autoHintId,
                className: _RadioGroup_css__WEBPACK_IMPORTED_MODULE_2__/* .hint */ .aK,
                children: hintText
            })
        ]
    });
}


/***/ })

}]);
//# sourceMappingURL=218-8194b7510ce93d31.js.map