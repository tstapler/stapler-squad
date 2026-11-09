(() => {
var exports = {};
exports.id = 8644;
exports.ids = [8644];
exports.modules = {

/***/ 1708:
/***/ ((module) => {

"use strict";
module.exports = require("node:process");

/***/ }),

/***/ 1917:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
__webpack_require__.r(__webpack_exports__);
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   "default": () => (/* binding */ TaggingClassifierSettingsPage),
/* harmony export */   metadata: () => (/* binding */ metadata)
/* harmony export */ });
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(12129);
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var _components_settings_TaggingClassifierSettings__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(40776);
/* harmony import */ var _components_analytics_PageViewTracker__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(7696);
// analytics-exempt
//
// PageViewTracker (rendered below) calls usePageView() internally, so this
// page is exempt from analytics/require-page-analytics's literal-call check
// -- same pattern as backlog-sources/page.tsx and remotes/page.tsx.



const metadata = {
    title: "Tag Classification - Settings - Stapler Squad",
    description: "Configure the LLM model hierarchy for session-tag classification."
};
function TaggingClassifierSettingsPage() {
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)(react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.Fragment, {
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(_components_analytics_PageViewTracker__WEBPACK_IMPORTED_MODULE_2__.PageViewTracker, {}),
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("main", {
                id: "main-content",
                children: /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(_components_settings_TaggingClassifierSettings__WEBPACK_IMPORTED_MODULE_1__.TaggingClassifierSettings, {})
            })
        ]
    });
}


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

/***/ 13366:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   TaggingClassifierSettings: () => (/* binding */ TaggingClassifierSettings)
/* harmony export */ });
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(79147);
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(88782);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_1__);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(31381);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__ = __webpack_require__(4435);
/* harmony import */ var _connectrpc_connect_web__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(14917);
/* harmony import */ var _lib_config__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(99706);
/* harmony import */ var _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(32647);
/* __next_internal_client_entry_do_not_use__ TaggingClassifierSettings auto */ 
// +feature: tagging-classifier-settings
// TaggingClassifierSettings — Settings -> Tag Classification panel. Load ->
// form -> save over Get/UpdateTaggingClassifierConfig, modeled on
// JulesSettings.tsx's structure. Covers the session-tag LLM fallback's model
// hierarchy: primary model plus ordered fallbacks (the free-proxy story —
// name the proxy's model as primary or fallback; ANTHROPIC_BASE_URL already
// flows to the headless subprocess).






const MAX_FALLBACKS = 5;
function TaggingClassifierSettings() {
    const [loading, setLoading] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(true);
    const [loadError, setLoadError] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(null);
    const [model, setModel] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)("");
    const [fallbacks, setFallbacks] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)([]);
    const [fallbackInput, setFallbackInput] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)("");
    const [envOverrideActive, setEnvOverrideActive] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(false);
    const [saving, setSaving] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(false);
    const [saveError, setSaveError] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(null);
    const [saveStatusMessage, setSaveStatusMessage] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(null);
    const clientRef = (0,react__WEBPACK_IMPORTED_MODULE_1__.useRef)(null);
    const loadConfig = (0,react__WEBPACK_IMPORTED_MODULE_1__.useCallback)(async ()=>{
        if (!clientRef.current) return;
        try {
            setLoading(true);
            setLoadError(null);
            const response = await clientRef.current.getTaggingClassifierConfig({});
            const cfg = response.config;
            if (cfg) {
                setModel(cfg.model || "");
                setFallbacks([
                    ...cfg.fallbackModels
                ]);
            }
            setEnvOverrideActive(response.envOverrideActive);
        } catch  {
            setLoadError("Couldn't load tag classification settings.");
        } finally{
            setLoading(false);
        }
    }, []);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        const transport = (0,_connectrpc_connect_web__WEBPACK_IMPORTED_MODULE_4__/* .createConnectTransport */ .w)({
            baseUrl: (0,_lib_config__WEBPACK_IMPORTED_MODULE_5__/* .getApiBaseUrl */ .h)()
        });
        clientRef.current = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_2__/* .SessionService */ .Qow, transport);
        loadConfig();
    }, [
        loadConfig
    ]);
    function handleAddFallback() {
        const name = fallbackInput.trim();
        if (!name || fallbacks.includes(name) || fallbacks.length >= MAX_FALLBACKS) return;
        setFallbacks([
            ...fallbacks,
            name
        ]);
        setFallbackInput("");
    }
    function handleRemoveFallback(name) {
        setFallbacks(fallbacks.filter((f)=>f !== name));
    }
    async function handleSave() {
        if (!clientRef.current) return;
        setSaving(true);
        setSaveError(null);
        setSaveStatusMessage(null);
        try {
            const resp = await clientRef.current.updateTaggingClassifierConfig({
                config: {
                    model: model.trim(),
                    fallbackModels: fallbacks
                }
            });
            const saved = resp.config;
            if (saved) {
                setModel(saved.model || "");
                setFallbacks([
                    ...saved.fallbackModels
                ]);
            }
            setSaveStatusMessage("Saved — applies to the next classification, no restart needed.");
            await loadConfig();
        } catch (err) {
            setSaveError(err instanceof Error ? err.message : String(err));
        } finally{
            setSaving(false);
        }
    }
    if (loading) {
        return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
            className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_3__/* .container */ .kL,
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("h2", {
                    className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_3__/* .heading */ .R_,
                    children: "Tag Classification"
                }),
                /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("p", {
                    className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_3__/* .loadingText */ .KQ,
                    children: "Loading…"
                })
            ]
        });
    }
    if (loadError) {
        return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
            className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_3__/* .container */ .kL,
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("h2", {
                    className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_3__/* .heading */ .R_,
                    children: "Tag Classification"
                }),
                /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("div", {
                    role: "alert",
                    children: loadError
                }),
                /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("div", {
                    className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_3__/* .actions */ .o1,
                    children: /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("button", {
                        type: "button",
                        className: "btn btn-primary",
                        onClick: loadConfig,
                        children: "Retry"
                    })
                })
            ]
        });
    }
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
        className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_3__/* .container */ .kL,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("h2", {
                className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_3__/* .heading */ .R_,
                children: "Tag Classification"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("p", {
                className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_3__/* .description */ .h_,
                children: "Sessions the sync tagging rules can't confidently tag fall back to a cheap LLM call — batched, at most one call per poll tick, and each session is classified once then left alone (re-run one manually from its tag editor)."
            }),
            envOverrideActive && /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("p", {
                className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_3__/* .warningText */ .i4,
                role: "alert",
                children: "Environment overrides are active (STAPLER_SQUAD_TAGGING_MODEL or STAPLER_SQUAD_TAGGING_FALLBACK_MODELS). They win over this form — unset them for these saved values to take effect."
            }),
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
                className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_3__/* .form */ .Zd,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
                        className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_3__/* .field */ .ZZ,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("label", {
                                htmlFor: "tagging-model",
                                className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_3__/* .label */ .Pf,
                                children: "Primary model"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("input", {
                                id: "tagging-model",
                                type: "text",
                                className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_3__/* .input */ .hF,
                                value: model,
                                onChange: (e)=>setModel(e.target.value),
                                placeholder: "haiku",
                                "aria-describedby": "tagging-model-hint",
                                autoComplete: "off"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("p", {
                                id: "tagging-model-hint",
                                className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_3__/* .hint */ .aK,
                                children: "Passed as --model to the headless CLI. Blank resets to the default (haiku). To use a free proxy, point ANTHROPIC_BASE_URL at it and name its model here."
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
                        className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_3__/* .field */ .ZZ,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("span", {
                                className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_3__/* .label */ .Pf,
                                children: "Fallback models (tried in order)"
                            }),
                            fallbacks.length === 0 ? /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("p", {
                                className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_3__/* .emptyNote */ .St,
                                children: "None — a failed primary call degrades the batch to Unclassified."
                            }) : /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("ul", {
                                role: "list",
                                children: fallbacks.map((name)=>/*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("li", {
                                        className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_3__/* .fallbackRow */ .vL,
                                        role: "listitem",
                                        children: [
                                            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("span", {
                                                className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_3__/* .fallbackName */ .k5,
                                                children: name
                                            }),
                                            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("button", {
                                                type: "button",
                                                className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_3__/* .removeBtn */ .lZ,
                                                onClick: ()=>handleRemoveFallback(name),
                                                "aria-label": `Remove fallback model ${name}`,
                                                children: "Remove"
                                            })
                                        ]
                                    }, name))
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
                                className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_3__/* .inputRow */ .bd,
                                children: [
                                    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("input", {
                                        id: "tagging-fallback-input",
                                        type: "text",
                                        className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_3__/* .input */ .hF,
                                        value: fallbackInput,
                                        onChange: (e)=>setFallbackInput(e.target.value),
                                        onKeyDown: (e)=>{
                                            if (e.key === "Enter") {
                                                e.preventDefault();
                                                handleAddFallback();
                                            }
                                        },
                                        placeholder: "e.g. proxy-free",
                                        "aria-label": "Add a fallback model",
                                        autoComplete: "off"
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("button", {
                                        type: "button",
                                        className: "btn btn-secondary",
                                        onClick: handleAddFallback,
                                        disabled: !fallbackInput.trim() || fallbacks.length >= MAX_FALLBACKS,
                                        children: "Add"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("p", {
                                className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_3__/* .hint */ .aK,
                                children: [
                                    "First success wins; at most ",
                                    MAX_FALLBACKS,
                                    " fallbacks. The first model that answers keeps classifying even when the paid tier is unreachable."
                                ]
                            })
                        ]
                    }),
                    saveError && /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("p", {
                        role: "alert",
                        className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_3__/* .saveError */ .Ek,
                        children: saveError
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
                        className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_3__/* .actions */ .o1,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("button", {
                                type: "button",
                                className: "btn btn-primary",
                                onClick: handleSave,
                                disabled: saving,
                                children: saving ? "Saving…" : "Save"
                            }),
                            saveStatusMessage && /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("span", {
                                role: "status",
                                className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_3__/* .saveStatus */ .i5,
                                children: saveStatusMessage
                            })
                        ]
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

/***/ 32647:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Ek: () => (/* binding */ saveError),
/* harmony export */   KQ: () => (/* binding */ loadingText),
/* harmony export */   Pf: () => (/* binding */ label),
/* harmony export */   R_: () => (/* binding */ heading),
/* harmony export */   St: () => (/* binding */ emptyNote),
/* harmony export */   ZZ: () => (/* binding */ field),
/* harmony export */   Zd: () => (/* binding */ form),
/* harmony export */   aK: () => (/* binding */ hint),
/* harmony export */   bd: () => (/* binding */ inputRow),
/* harmony export */   hF: () => (/* binding */ input),
/* harmony export */   h_: () => (/* binding */ description),
/* harmony export */   i4: () => (/* binding */ warningText),
/* harmony export */   i5: () => (/* binding */ saveStatus),
/* harmony export */   k5: () => (/* binding */ fallbackName),
/* harmony export */   kL: () => (/* binding */ container),
/* harmony export */   lZ: () => (/* binding */ removeBtn),
/* harmony export */   o1: () => (/* binding */ actions),
/* harmony export */   vL: () => (/* binding */ fallbackRow)
/* harmony export */ });
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(13507);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2FsettingsPanelForm_css_ts_vanilla_css_22_2C_22source_22_3A_22LmFvZ2NhbTAgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IDFyZW07CiAgbWF4LXdpZHRoOiA2NDBweDsKfQouYW9nY2FtMSB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7CiAgZm9udC1zaXplOiAxLjI1cmVtOwogIGZvbnQtd2VpZ2h0OiA2MDA7CiAgbWFyZ2luOiAwOwp9Ci5hb2djYW0yIHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKICBmb250LXNpemU6IDAuODc1cmVtOwogIG1hcmdpbjogMDsKfQouYW9nY2FtMyB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7Cn0KLmFvZ2NhbTQgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IDEuMjVyZW07Cn0KLmFvZ2NhbTUgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IDAuNXJlbTsKfQouYW9nY2FtNiB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7CiAgZm9udC1zaXplOiAwLjg3NXJlbTsKICBmb250LXdlaWdodDogNjAwOwp9Ci5hb2djYW03IHsKICBkaXNwbGF5OiBmbGV4OwogIGdhcDogMC41cmVtOwogIGFsaWduLWl0ZW1zOiBjZW50ZXI7Cn0KLmFvZ2NhbTggewogIHBhZGRpbmc6IDAuNXJlbSAwLjc1cmVtOwogIGJhY2tncm91bmQtY29sb3I6IHZhcigtLV8xOWV6MnJ6MWEpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6ayk7CiAgYm9yZGVyLXJhZGl1czogNHB4OwogIGNvbG9yOiB2YXIoLS1fMTllejJyejFiKTsKICBmb250LXNpemU6IDAuODc1cmVtOwogIGZsZXg6IDE7Cn0KLmFvZ2NhbTg6Zm9jdXMgewogIG91dGxpbmU6IG5vbmU7CiAgYm9yZGVyLWNvbG9yOiB2YXIoLS1fMTllejJyemwpOwp9_22_7D__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(71918);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fsettings_2FJulesSettings_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA62U33biIBDG732Kudy9wJNobZQ_TA_GMc4WgQIxxj199x6ibbJu_tieXgIzH9_8ZmD_vH5d_tdjAn9nALlRxnE4CveLsed0g_eFOy9_P80AdkYH5umMHJL5Ol2sHB7i_kG4gjSH5Gn2Nptf1dJhtXOjthX5S_FMqSXrjaovUcZJdBxSewJvFMmbqFMnijkhqfQcHuwp7lohJekiul1m0Swk88dP11_qZtFUI8lbJWoOO4XNDUJRoRkFPPjLJvNBuBCPCmGj8EW1FVoOY0l6IGe3_Q9T_VNY0_wuri_3cb1SzcahtvZX_RwbdJIc5oGM5rG_8qA7GJdZT3u6TuJKkQ_Mh1ohB200di9_vKOBOeqALm7_KX2gXc1yowPqwMFbkSPbYqgQ9X_tHUUyRRoHSbfus29NDYA5otspU3HYk5QX5wFPgbUHqBRZTz4eVXsKyJpaI8HKCdt1sW5c9L2qttip6Uvvmr40GRm_fuHxLyovnY9J1tBHj6eK5fsIqSnZWJFTqKPm5t8YSV5sFcrbsFX3Um0CE0qZCmU3ezPc1Orr_6340d97_6Nqef_ju76hz_EZeIzXgWPB9P6o8nsYWwEcFhidqrd3orAnkTkHAAA_22_7D__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(74034);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fsettings_2FTaggingClassifierSettings_css_ts_vanilla_css_22_2C_22source_22_3A_22LnB1NzZwYzAgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IDAuNXJlbTsKfQoucHU3NnBjMSB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7CiAgZm9udC1zaXplOiAwLjg3NXJlbTsKICBmbGV4OiAxIDEgYXV0bzsKfQoucHU3NnBjMiB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MTApOwogIGJhY2tncm91bmQ6IHRyYW5zcGFyZW50OwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6ayk7CiAgYm9yZGVyLXJhZGl1czogNHB4OwogIHBhZGRpbmc6IDAuMjVyZW0gMC42MjVyZW07CiAgZm9udC1zaXplOiAwLjgxMjVyZW07CiAgY3Vyc29yOiBwb2ludGVyOwp9Ci5wdTc2cGMzIHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKICBmb250LXNpemU6IDAuODEyNXJlbTsKICBtYXJnaW46IDA7Cn0_22_7D__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(74809);




var actions = '_8q3sqvc';
var container = 'aogcam0';
var description = 'aogcam2';
var emptyNote = 'pu76pc3';
var fallbackName = 'pu76pc1';
var fallbackRow = 'pu76pc0';
var field = 'aogcam5';
var form = 'aogcam4';
var heading = 'aogcam1';
var hint = '_8q3sqv0';
var input = 'aogcam8';
var inputRow = 'aogcam7';
var label = 'aogcam6';
var loadingText = 'aogcam3';
var removeBtn = 'pu76pc2';
var saveError = '_8q3sqve';
var saveStatus = '_8q3sqvd';
var warningText = '_8q3sqv1';

/***/ }),

/***/ 33873:
/***/ ((module) => {

"use strict";
module.exports = require("path");

/***/ }),

/***/ 40776:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   TaggingClassifierSettings: () => (/* binding */ TaggingClassifierSettings)
/* harmony export */ });
/* harmony import */ var react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(70679);
/* harmony import */ var react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__);

const TaggingClassifierSettings = (0,react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__.registerClientReference)(
function() { throw new Error("Attempted to call TaggingClassifierSettings() from the server but TaggingClassifierSettings is on the client. It's not possible to invoke a client function from the server, it can only be rendered as a Component or passed to props of a Client Component."); },
"/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/components/settings/TaggingClassifierSettings.tsx",
"TaggingClassifierSettings",
);

/***/ }),

/***/ 49030:
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
const page5 = () => Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 1917));


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
        'tagging-classifier',
        {
        children: ['__PAGE__', {}, {
          page: [page5, "/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/settings/tagging-classifier/page.tsx"],
          
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
const pages = ["/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/settings/tagging-classifier/page.tsx"];


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
        page: "/settings/tagging-classifier/page",
        pathname: "/settings/tagging-classifier",
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

/***/ 50639:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 7696));
;
Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 40776));


/***/ }),

/***/ 55591:
/***/ ((module) => {

"use strict";
module.exports = require("https");

/***/ }),

/***/ 58887:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 88486));
;
Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 13366));


/***/ }),

/***/ 62722:
/***/ (() => {



/***/ }),

/***/ 63033:
/***/ ((module) => {

"use strict";
module.exports = require("next/dist/server/app-render/work-unit-async-storage.external.js");

/***/ }),

/***/ 71918:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, ".aogcam0 {\n  display: flex;\n  flex-direction: column;\n  gap: 1rem;\n  max-width: 640px;\n}\n.aogcam1 {\n  color: var(--_19ez2rz0);\n  font-size: 1.25rem;\n  font-weight: 600;\n  margin: 0;\n}\n.aogcam2 {\n  color: var(--_19ez2rz2);\n  font-size: 0.875rem;\n  margin: 0;\n}\n.aogcam3 {\n  color: var(--_19ez2rz2);\n}\n.aogcam4 {\n  display: flex;\n  flex-direction: column;\n  gap: 1.25rem;\n}\n.aogcam5 {\n  display: flex;\n  flex-direction: column;\n  gap: 0.5rem;\n}\n.aogcam6 {\n  color: var(--_19ez2rz1);\n  font-size: 0.875rem;\n  font-weight: 600;\n}\n.aogcam7 {\n  display: flex;\n  gap: 0.5rem;\n  align-items: center;\n}\n.aogcam8 {\n  padding: 0.5rem 0.75rem;\n  background-color: var(--_19ez2rz1a);\n  border: 1px solid var(--_19ez2rzk);\n  border-radius: 4px;\n  color: var(--_19ez2rz1b);\n  font-size: 0.875rem;\n  flex: 1 1;\n}\n.aogcam8:focus {\n  outline: none;\n  border-color: var(--_19ez2rzl);\n}", ""]);
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

/***/ 74034:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "._8q3sqv0 {\n  color: var(--_19ez2rz2);\n  font-size: 0.8125rem;\n  margin: 0;\n}\n._8q3sqv1 {\n  color: var(--_19ez2rzz);\n  background-color: var(--_19ez2rzy);\n  border: 1px solid var(--_19ez2rzx);\n  border-radius: 4px;\n  padding: 0.375rem 0.625rem;\n  font-size: 0.8125rem;\n  margin: 0;\n}\n._8q3sqv2 {\n  display: flex;\n  align-items: flex-start;\n  gap: 0.5rem;\n}\n._8q3sqv3 {\n  color: var(--_19ez2rz0);\n  font-size: 0.875rem;\n}\n._8q3sqv4 {\n  color: var(--_19ez2rz0);\n  background-color: var(--_19ez2rz17);\n  border: 1px solid var(--_19ez2rzk);\n  border-radius: 4px;\n  padding: 0.5rem 0.75rem;\n  font-size: 0.8125rem;\n}\n._8q3sqv5 {\n  display: flex;\n  flex-direction: column;\n  gap: 0.375rem;\n  margin: 0;\n  padding: 0;\n  list-style: none;\n}\n._8q3sqv6 {\n  display: flex;\n  align-items: center;\n  justify-content: space-between;\n  gap: 0.5rem;\n  padding: 0.5rem 0.75rem;\n  border: 1px solid var(--_19ez2rze);\n  border-radius: 4px;\n}\n._8q3sqv7 {\n  color: var(--_19ez2rz0);\n  font-size: 0.875rem;\n  overflow: hidden;\n  text-overflow: ellipsis;\n  white-space: nowrap;\n}\n._8q3sqv8 {\n  padding: 0.375rem 0.75rem;\n  background-color: var(--_19ez2rz11);\n  border: 1px solid var(--_19ez2rz10);\n  border-radius: 4px;\n  color: var(--_19ez2rz12);\n  font-size: 0.8125rem;\n  cursor: pointer;\n  white-space: nowrap;\n}\n._8q3sqv8:hover {\n  opacity: 0.9;\n}\n._8q3sqv8:disabled {\n  opacity: 0.5;\n  cursor: not-allowed;\n}\n._8q3sqv9 {\n  color: var(--_19ez2rzw);\n  font-size: 0.8125rem;\n  margin: 0;\n}\n._8q3sqva {\n  color: var(--_19ez2rz2);\n  font-size: 0.8125rem;\n  margin: 0;\n}\n._8q3sqvb {\n  color: var(--_19ez2rz2);\n  font-size: 0.8125rem;\n  margin: 0;\n}\n._8q3sqvc {\n  display: flex;\n  gap: 0.75rem;\n  align-items: center;\n  padding-top: 0.5rem;\n}\n._8q3sqvd {\n  color: var(--_19ez2rzw);\n  font-size: 0.8125rem;\n}\n._8q3sqve {\n  color: var(--_19ez2rz12);\n  font-size: 0.8125rem;\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 74075:
/***/ ((module) => {

"use strict";
module.exports = require("zlib");

/***/ }),

/***/ 74809:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, ".pu76pc0 {\n  display: flex;\n  align-items: center;\n  gap: 0.5rem;\n}\n.pu76pc1 {\n  color: var(--_19ez2rz0);\n  font-size: 0.875rem;\n  flex: 1 1 auto;\n}\n.pu76pc2 {\n  color: var(--_19ez2rz10);\n  background: transparent;\n  border: 1px solid var(--_19ez2rzk);\n  border-radius: 4px;\n  padding: 0.25rem 0.625rem;\n  font-size: 0.8125rem;\n  cursor: pointer;\n}\n.pu76pc3 {\n  color: var(--_19ez2rz2);\n  font-size: 0.8125rem;\n  margin: 0;\n}", ""]);
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
var __webpack_exports__ = __webpack_require__.X(0, [4655,4164], () => (__webpack_exec__(49030)));
module.exports = __webpack_exports__;

})();