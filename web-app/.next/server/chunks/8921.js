"use strict";
exports.id = 8921;
exports.ids = [8921];
exports.modules = {

/***/ 13759:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

__webpack_require__.r(__webpack_exports__);
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   BacklogItemIntentReview: () => (/* binding */ BacklogItemIntentReview),
/* harmony export */   extractImageMarkdown: () => (/* binding */ extractImageMarkdown)
/* harmony export */ });
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(79147);
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(88782);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_1__);
/* harmony import */ var _BacklogItemForm__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(47959);
/* harmony import */ var _lib_hooks_useBacklogService__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(78120);
/* harmony import */ var _lib_hooks_useSessionRepoPaths__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(50718);
/* __next_internal_client_entry_do_not_use__ extractImageMarkdown,BacklogItemIntentReview auto */ 
// +feature: backlog:intent-review




/** Truncates a message's first line for a client-side fallback title — mirrors
 * the server's deriveChatItemTitle (backlog_service_chat.go), used here only
 * while parsing is in flight or failed, never sent to the server directly. */ function fallbackTitle(message) {
    const firstLine = message.split("\n")[0].trim();
    return firstLine.length > 80 ? firstLine.slice(0, 80) : firstLine;
}
/** Image references in the user's text that the LLM may have dropped or rewritten. */ function extractImageMarkdown(text) {
    return Array.from(text.matchAll(/!\[[^\]]*\]\([^)]+\)/g), (m)=>m[0]);
}
/** Builds BacklogItemForm's initialValues from a parsed draft, or — when
 * draft is null (parse failed/timed out) — from the raw text, so the user's
 * typed message is never silently lost. */ function draftToInitialValues(draft, rawText, suggestedRepoPath) {
    if (!draft) {
        return {
            title: fallbackTitle(rawText),
            description: rawText
        };
    }
    let description = draft.description;
    const missingImages = extractImageMarkdown(rawText).filter((img)=>!description.includes(img));
    if (missingImages.length > 0) {
        description = `${description.trimEnd()}\n\n${missingImages.join("\n")}`;
    }
    return {
        title: draft.title,
        description,
        ...suggestedRepoPath ? {
            repoPath: suggestedRepoPath
        } : {},
        acCriteria: draft.acceptanceCriteria.map((text, index)=>({
                index,
                text,
                status: "pending"
            }))
    };
}
/** Returns the draft's suggested repo only if it is an exact member of the
 * list we sent — the LLM must never introduce a path of its own. */ function validatedSuggestion(draft, knownRepoPaths) {
    const suggested = draft?.suggestedRepoPath ?? "";
    return suggested && knownRepoPaths.includes(suggested) ? suggested : "";
}
/**
 * Wraps ParseBacklogItemIntent with the omnibar review UI's loading/result
 * state — mirrors the abandoned llm-omnibar design's useParseIntent.ts shape,
 * scoped down to this feature's needs.
 */ function useParseBacklogItemIntent(initialText) {
    const { parseBacklogItemIntent } = (0,_lib_hooks_useBacklogService__WEBPACK_IMPORTED_MODULE_3__/* .useBacklogService */ .gG)();
    const knownRepoPaths = (0,_lib_hooks_useSessionRepoPaths__WEBPACK_IMPORTED_MODULE_4__/* .useSessionRepoPaths */ .O)();
    const [suggestedRepoPath, setSuggestedRepoPath] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)("");
    const [phase, setPhase] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)("parsing");
    const [initialValues, setInitialValues] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)({});
    const [parseFailed, setParseFailed] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(false);
    // `active` guards against a stale parse response applying after the user
    // has already moved on (e.g. pressed Escape) before the in-flight RPC
    // resolves — mirrors the abandoned design's "abort-on-change" intent
    // without needing an AbortController, since the RPC has no side effects to
    // actually cancel.
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        let active = true;
        setPhase("parsing");
        parseBacklogItemIntent(initialText, knownRepoPaths).then((draft)=>{
            if (!active) return;
            const suggestion = validatedSuggestion(draft, knownRepoPaths);
            setParseFailed(!draft);
            setSuggestedRepoPath(suggestion);
            setInitialValues(draftToInitialValues(draft, initialText, suggestion));
            setPhase("review");
        });
        return ()=>{
            active = false;
        };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- run once per mount; initialText is fixed, and the known-repo list is a mount-time snapshot (an unhydrated session store just means no suggestion)
    }, []);
    return {
        phase,
        initialValues,
        parseFailed,
        suggestedRepoPath
    };
}
function ParsingIndicator() {
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("div", {
        role: "status",
        "aria-live": "polite",
        style: {
            padding: "24px",
            textAlign: "center",
            color: "var(--text-secondary)"
        },
        children: "Parsing your message into a backlog item…"
    });
}
function ParseFailedBanner() {
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("p", {
        role: "alert",
        "data-testid": "backlog-intent-review-parse-failed-banner",
        style: {
            margin: "0 0 12px",
            padding: "8px 12px",
            fontSize: "13px",
            borderRadius: "6px",
            background: "var(--warning-bg, rgba(234, 179, 8, 0.12))",
            color: "var(--warning-text, #b45309)"
        },
        children: "Couldn’t parse this into a structured item — review the fields below before creating."
    });
}
/**
 * Renders the review UI for a parsed "backlog: <message>" draft in place of
 * the old fire-and-close flow. Reuses BacklogItemForm for edit/create
 * (including its pipeline-mode and auto-spawn-session controls); falls back
 * to the raw text on parse failure so nothing is lost.
 */ function BacklogItemIntentReview({ initialText, onDone, onCancel }) {
    const { createBacklogItem } = (0,_lib_hooks_useBacklogService__WEBPACK_IMPORTED_MODULE_3__/* .useBacklogService */ .gG)();
    const { phase, initialValues, parseFailed, suggestedRepoPath } = useParseBacklogItemIntent(initialText);
    const handleSubmit = async (data)=>{
        const result = await createBacklogItem(data);
        if (result) onDone(result.item);
    };
    if (phase === "parsing") {
        return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(ParsingIndicator, {});
    }
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
        children: [
            parseFailed && /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(ParseFailedBanner, {}),
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(_BacklogItemForm__WEBPACK_IMPORTED_MODULE_2__/* .BacklogItemForm */ .T, {
                initialValues: initialValues,
                autoSuggestedRepoPath: suggestedRepoPath || undefined,
                onSubmit: handleSubmit,
                onCancel: onCancel
            })
        ]
    });
}


/***/ }),

/***/ 49623:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   e: () => (/* binding */ markdownBody)
/* harmony export */ });
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(13507);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FmarkdownBody_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA52T0W7CMAxF3_sVfpm0SkuVgmBb_JgpNGkbkcZdmkLHtH_fmgILAca0t0i2r8_9crKqrGRJ4TMBKFCjZbDl9pGQt_xV7md2T9NVAlCicaRTexmX52tf18pIUktV1Y5Bni1XyVeSHaRbL95wWynDgAKNJGb7FOjZANO8c6SolRbBLFmjc9iw82Z_G73xaE4OjghZoOVOoWHQGyHtCBzKqKY6rBrITglXM8gpfRgF1miFtMRyofruwv9z_tPDIG8H6FArEbWVabisQCH9Nh9ryRulP2LhWR8HT7OX54Vs_DpebCqLvRHx2ITTciGUqcaZfCEboNn8OPmrmeU_zLR28nKP6Z6qTP8a9sldVJ1OFbfSlhp3ZGDAe4cx6yn8ENhZbrqWW2lcyGpwvJIwz1Ct109wfKP2modGomXpLvhmni865ou_EG5Yayw27z26A_EUziQ_v5FjE2Z0HWQK6uqfOWOchkfP3zdHlT4nBAAA_22_7D__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(52298);


var markdownBody = 'gfgef0';

/***/ }),

/***/ 50013:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "._15lm3oa0 {\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz31);\n}\n._15lm3oa1 {\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz2y);\n}\n._15lm3oa2 {\n  font-size: var(--_19ez2rz3b);\n  font-weight: var(--_19ez2rz3g);\n  color: var(--_19ez2rz1);\n  display: flex;\n  align-items: center;\n  gap: var(--_19ez2rz2y);\n}\n._15lm3oa3 {\n  color: var(--_19ez2rz10);\n  font-size: var(--_19ez2rz3a);\n  line-height: 1;\n}\n._15lm3oa4 {\n  width: 100%;\n  padding: var(--_19ez2rz2z) var(--_19ez2rz30);\n  background: var(--_19ez2rz1a);\n  color: var(--_19ez2rz1b);\n  border: 1px solid var(--_19ez2rzk);\n  border-radius: var(--_19ez2rz37);\n  font-size: var(--_19ez2rz3b);\n  font-family: var(--_19ez2rz2v);\n  outline: none;\n  transition: border-color 0.15s ease;\n  box-sizing: border-box;\n}\n._15lm3oa4:focus {\n  border-color: var(--_19ez2rzl);\n  box-shadow: 0 0 0 2px var(--_19ez2rz17);\n}\n._15lm3oa4::placeholder {\n  color: var(--_19ez2rz1c);\n}\n._15lm3oa5 {\n  width: 100%;\n  padding: var(--_19ez2rz2z) var(--_19ez2rz30);\n  background: var(--_19ez2rz1a);\n  color: var(--_19ez2rz1b);\n  border: 1px solid var(--_19ez2rzk);\n  border-radius: var(--_19ez2rz37);\n  font-size: var(--_19ez2rz3b);\n  font-family: var(--_19ez2rz2v);\n  outline: none;\n  resize: vertical;\n  min-height: 80px;\n  transition: border-color 0.15s ease;\n  box-sizing: border-box;\n}\n._15lm3oa5:focus {\n  border-color: var(--_19ez2rzl);\n  box-shadow: 0 0 0 2px var(--_19ez2rz17);\n}\n._15lm3oa5::placeholder {\n  color: var(--_19ez2rz1c);\n}\n._15lm3oa6 {\n  width: 100%;\n  padding: var(--_19ez2rz2z) var(--_19ez2rz30);\n  background: var(--_19ez2rz1a);\n  color: var(--_19ez2rz1b);\n  border: 1px solid var(--_19ez2rzk);\n  border-radius: var(--_19ez2rz37);\n  font-size: var(--_19ez2rz3b);\n  font-family: var(--_19ez2rz2v);\n  outline: none;\n  cursor: pointer;\n  box-sizing: border-box;\n}\n._15lm3oa6:focus {\n  border-color: var(--_19ez2rzl);\n}\n._15lm3oa7 {\n  display: flex;\n  align-items: center;\n  gap: var(--_19ez2rz2z);\n  cursor: pointer;\n}\n._15lm3oa8 {\n  width: 16px;\n  height: 16px;\n  cursor: pointer;\n  accent-color: var(--_19ez2rzm);\n}\n._15lm3oa9 {\n  font-size: var(--_19ez2rz3b);\n  color: var(--_19ez2rz0);\n  cursor: pointer;\n}\n._15lm3oaa {\n  font-size: var(--_19ez2rz3a);\n  color: var(--_19ez2rz2);\n  margin-left: var(--_19ez2rz32);\n}\n._15lm3oab {\n  font-size: var(--_19ez2rz3a);\n  color: var(--_19ez2rz2);\n}\n._15lm3oac {\n  color: var(--_19ez2rzm);\n  text-decoration: none;\n}\n._15lm3oac:hover {\n  text-decoration: underline;\n}\n._15lm3oad {\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz2z);\n}\n._15lm3oae {\n  display: flex;\n  align-items: center;\n  justify-content: space-between;\n}\n._15lm3oaf {\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz2z);\n}\n._15lm3oag {\n  display: flex;\n  align-items: center;\n  gap: var(--_19ez2rz2z);\n}\n._15lm3oah {\n  flex: 1 1;\n  padding: var(--_19ez2rz2y) var(--_19ez2rz2z);\n  background: var(--_19ez2rz1a);\n  color: var(--_19ez2rz1b);\n  border: 1px solid var(--_19ez2rzk);\n  border-radius: var(--_19ez2rz36);\n  font-size: var(--_19ez2rz3b);\n  font-family: var(--_19ez2rz2v);\n  outline: none;\n}\n._15lm3oah:focus {\n  border-color: var(--_19ez2rzl);\n}\n._15lm3oai {\n  padding: var(--_19ez2rz2y) var(--_19ez2rz2z);\n  background: var(--_19ez2rz1a);\n  color: var(--_19ez2rz1b);\n  border: 1px solid var(--_19ez2rzk);\n  border-radius: var(--_19ez2rz36);\n  font-size: var(--_19ez2rz3a);\n  font-family: var(--_19ez2rz2u);\n  outline: none;\n  cursor: pointer;\n  min-width: 100px;\n}\n._15lm3oaj {\n  display: inline-flex;\n  align-items: center;\n  justify-content: center;\n  width: 24px;\n  height: 24px;\n  border-radius: var(--_19ez2rz36);\n  border: 1px solid var(--_19ez2rzg);\n  background: transparent;\n  color: var(--_19ez2rz2);\n  cursor: pointer;\n  font-size: var(--_19ez2rz3b);\n  flex-shrink: 0;\n}\n._15lm3oaj:hover {\n  background: var(--_19ez2rz11);\n  color: var(--_19ez2rz10);\n  border-color: var(--_19ez2rz10);\n}\n._15lm3oak {\n  display: inline-flex;\n  align-items: center;\n  gap: var(--_19ez2rz2y);\n  padding: var(--_19ez2rz2y) var(--_19ez2rz2z);\n  background: transparent;\n  color: var(--_19ez2rzm);\n  border: 1px dashed var(--_19ez2rzg);\n  border-radius: var(--_19ez2rz36);\n  font-size: var(--_19ez2rz3b);\n  cursor: pointer;\n  align-self: flex-start;\n}\n._15lm3oak:hover {\n  background: var(--_19ez2rz17);\n  border-color: var(--_19ez2rzm);\n}\n._15lm3oal {\n  display: flex;\n  gap: var(--_19ez2rz2z);\n  justify-content: flex-end;\n  padding-top: var(--_19ez2rz2z);\n  border-top: 1px solid var(--_19ez2rzf);\n}\n._15lm3oam {\n  display: inline-flex;\n  align-items: center;\n  gap: var(--_19ez2rz2y);\n  padding: var(--_19ez2rz2z) var(--_19ez2rz31);\n  background: var(--_19ez2rzm);\n  color: var(--_19ez2rzq);\n  border: none;\n  border-radius: var(--_19ez2rz37);\n  font-size: var(--_19ez2rz3b);\n  font-weight: var(--_19ez2rz3g);\n  cursor: pointer;\n  transition: background 0.1s ease;\n}\n._15lm3oam:hover {\n  background: var(--_19ez2rzn);\n}\n._15lm3oam:disabled {\n  opacity: 0.5;\n  cursor: not-allowed;\n}\n._15lm3oan {\n  display: inline-flex;\n  align-items: center;\n  padding: var(--_19ez2rz2z) var(--_19ez2rz31);\n  background: transparent;\n  color: var(--_19ez2rz1);\n  border: 1px solid var(--_19ez2rzg);\n  border-radius: var(--_19ez2rz37);\n  font-size: var(--_19ez2rz3b);\n  font-weight: var(--_19ez2rz3g);\n  cursor: pointer;\n}\n._15lm3oan:hover {\n  background: var(--_19ez2rz8);\n  border-color: var(--_19ez2rzh);\n}\n._15lm3oao {\n  color: var(--_19ez2rz10);\n  font-size: var(--_19ez2rz3a);\n  margin-top: var(--_19ez2rz2y);\n}\n._15lm3oap {\n  display: flex;\n  align-items: center;\n  justify-content: space-between;\n  gap: var(--_19ez2rz2z);\n  flex-wrap: wrap;\n}\n._15lm3oaq {\n  display: flex;\n  align-items: center;\n  gap: var(--_19ez2rz2y);\n}\n._15lm3oar {\n  padding: var(--_19ez2rz2y) var(--_19ez2rz2z);\n  background: transparent;\n  color: var(--_19ez2rz2);\n  border: 1px solid transparent;\n  border-radius: var(--_19ez2rz36);\n  font-size: var(--_19ez2rz3a);\n  cursor: pointer;\n}\n._15lm3oar:hover {\n  color: var(--_19ez2rz0);\n}\n._15lm3oas {\n  color: var(--_19ez2rz0);\n  background: var(--_19ez2rz17);\n  border-color: var(--_19ez2rzg);\n}\n._15lm3oat {\n  padding: var(--_19ez2rz2y) var(--_19ez2rz2z);\n  background: transparent;\n  color: var(--_19ez2rzm);\n  border: 1px solid var(--_19ez2rzg);\n  border-radius: var(--_19ez2rz36);\n  font-size: var(--_19ez2rz3a);\n  cursor: pointer;\n}\n._15lm3oat:hover {\n  background: var(--_19ez2rz17);\n}\n._15lm3oat:disabled {\n  opacity: 0.5;\n  cursor: not-allowed;\n}\n._15lm3oau {\n  display: none;\n}\n._15lm3oav {\n  min-height: 80px;\n  padding: var(--_19ez2rz2z) var(--_19ez2rz30);\n  background: var(--_19ez2rz1a);\n  border: 1px solid var(--_19ez2rzk);\n  border-radius: var(--_19ez2rz37);\n  box-sizing: border-box;\n}\n._15lm3oaw {\n  color: var(--_19ez2rz2);\n  font-size: var(--_19ez2rz3b);\n  font-style: italic;\n}\n._15lm3oax {\n  display: grid;\n  grid-template-columns: 2fr 1fr;\n  grid-gap: var(--_19ez2rz30);\n  gap: var(--_19ez2rz30);\n}\n._15lm3oay {\n  display: inline-flex;\n  align-items: center;\n  align-self: flex-start;\n  gap: var(--_19ez2rz2y);\n  margin-top: var(--_19ez2rz2y);\n  padding: var(--_19ez2rz2y) var(--_19ez2rz2z);\n  font-size: var(--_19ez2rz3a);\n  color: var(--_19ez2rz1b);\n  background: var(--_19ez2rz17);\n  border: 1px solid var(--_19ez2rzk);\n  border-radius: var(--_19ez2rz39);\n}\n._15lm3oaz {\n  min-width: 24px;\n  min-height: 24px;\n  padding: 0;\n  border: none;\n  background: transparent;\n  color: inherit;\n  cursor: pointer;\n}\n@media (max-width: 480px) {\n  ._15lm3oax {\n    grid-template-columns: 1fr;\n  }\n}\n@media (pointer: coarse) {\n  ._15lm3oaz {\n    min-width: 44px;\n    min-height: 44px;\n  }\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 52298:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, ".gfgef0 {\n  color: var(--_19ez2rz0);\n  font-size: var(--_19ez2rz3b);\n  line-height: 1.6;\n}\n.gfgef0 p {\n  margin: 0 0 var(--_19ez2rz2z) 0;\n}\n.gfgef0 p:last-child {\n  margin-bottom: 0;\n}\n.gfgef0 a {\n  color: var(--_19ez2rzm);\n  text-decoration: underline;\n}\n.gfgef0 img {\n  max-width: 100%;\n  border-radius: var(--_19ez2rz37);\n  border: 1px solid var(--_19ez2rzf);\n}\n.gfgef0 code {\n  font-family: var(--_19ez2rz2u);\n  font-size: 0.875em;\n  background: var(--_19ez2rz7);\n  padding: 0.15em 0.35em;\n  border-radius: var(--_19ez2rz36);\n  border: 1px solid var(--_19ez2rzf);\n}\n.gfgef0 pre {\n  background: var(--_19ez2rz7);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: var(--_19ez2rz37);\n  padding: var(--_19ez2rz30);\n  overflow-x: auto;\n}\n.gfgef0 pre code {\n  background: transparent;\n  border: none;\n  padding: 0;\n}\n.gfgef0 ul, .gfgef0 ol {\n  padding-left: var(--_19ez2rz32);\n  margin-bottom: var(--_19ez2rz2z);\n}\n.gfgef0 blockquote {\n  border-left: 3px solid var(--_19ez2rzm);\n  padding-left: var(--_19ez2rz30);\n  color: var(--_19ez2rz2);\n  margin-left: 0;\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ })

};
;