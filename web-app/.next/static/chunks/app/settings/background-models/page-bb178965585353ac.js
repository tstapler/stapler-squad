(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[2752],{

/***/ 2972:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   BackgroundModelsSettings: () => (/* binding */ BackgroundModelsSettings)
/* harmony export */ });
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(61574);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(55494);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(23575);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__ = __webpack_require__(66144);
/* harmony import */ var _connectrpc_connect_web__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(46564);
/* harmony import */ var _lib_config__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(51202);
/* harmony import */ var _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(6393);
/* __next_internal_client_entry_do_not_use__ BackgroundModelsSettings auto */ 
// +feature: background-models-settings
// BackgroundModelsSettings — Settings -> Background Models panel. Load -> form -> save
// over Get/UpdateBackgroundModels. A blank field means "use the built-in default"
// (shown as the placeholder); changes apply on the next call, no restart.






function BackgroundModelsSettings() {
    const [loading, setLoading] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(true);
    const [loadError, setLoadError] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(null);
    const [features, setFeatures] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)({});
    const [stages, setStages] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)({});
    const [effort, setEffort] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)("");
    const [featureDefaults, setFeatureDefaults] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)({});
    const [stageDefaults, setStageDefaults] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)({});
    const [effortLevels, setEffortLevels] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)([]);
    const [saving, setSaving] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(false);
    const [saveError, setSaveError] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(null);
    const [status, setStatus] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(null);
    const clientRef = (0,react__WEBPACK_IMPORTED_MODULE_1__.useRef)(null);
    const load = (0,react__WEBPACK_IMPORTED_MODULE_1__.useCallback)(async function() {
        let quiet = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : false;
        if (!clientRef.current) return;
        try {
            var _resp_settings, _resp_settings1, _resp_settings2;
            if (!quiet) setLoading(true);
            setLoadError(null);
            const resp = await clientRef.current.getBackgroundModels({});
            setFeatures({
                ...(_resp_settings = resp.settings) === null || _resp_settings === void 0 ? void 0 : _resp_settings.features
            });
            setStages({
                ...(_resp_settings1 = resp.settings) === null || _resp_settings1 === void 0 ? void 0 : _resp_settings1.stages
            });
            var _resp_settings_effort;
            setEffort((_resp_settings_effort = (_resp_settings2 = resp.settings) === null || _resp_settings2 === void 0 ? void 0 : _resp_settings2.effort) !== null && _resp_settings_effort !== void 0 ? _resp_settings_effort : "");
            setFeatureDefaults({
                ...resp.featureDefaults
            });
            setStageDefaults({
                ...resp.stageDefaults
            });
            setEffortLevels([
                ...resp.effortLevels
            ]);
        } catch (e) {
            setLoadError("Couldn't load background model settings.");
        } finally{
            setLoading(false);
        }
    }, []);
    (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        const transport = (0,_connectrpc_connect_web__WEBPACK_IMPORTED_MODULE_5__/* .createConnectTransport */ .w)({
            baseUrl: (0,_lib_config__WEBPACK_IMPORTED_MODULE_3__/* .getApiBaseUrl */ .h)()
        });
        clientRef.current = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_2__/* .SessionService */ .Qow, transport);
        load();
    }, [
        load
    ]);
    // Unlisted pins (hand-set in config.json) are kept server-side; sending them back is rejected.
    const onlyListed = (o, listed)=>Object.fromEntries(Object.entries(o).filter((param)=>{
            let [k] = param;
            return k in listed;
        }));
    async function handleSave() {
        if (!clientRef.current) return;
        setSaving(true);
        setSaveError(null);
        setStatus(null);
        try {
            await clientRef.current.updateBackgroundModels({
                settings: {
                    features: onlyListed(features, featureDefaults),
                    stages: onlyListed(stages, stageDefaults),
                    effort
                }
            });
            setStatus("Saved — applies to the next call, no restart needed.");
            await load(true);
        } catch (err) {
            setSaveError(err instanceof Error ? err.message : String(err));
        } finally{
            setSaving(false);
        }
    }
    const renderRows = (prefix, values, defaults, set)=>Object.keys(defaults).sort().map((key)=>{
            var _values_key;
            return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
                className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_4__/* .field */ .ZZ,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("label", {
                        htmlFor: "".concat(prefix, "-").concat(key),
                        className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_4__/* .label */ .Pf,
                        children: key
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("input", {
                        id: "".concat(prefix, "-").concat(key),
                        type: "text",
                        className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_4__/* .input */ .hF,
                        value: (_values_key = values[key]) !== null && _values_key !== void 0 ? _values_key : "",
                        onChange: (e)=>set({
                                ...values,
                                [key]: e.target.value
                            }),
                        placeholder: defaults[key] || "account default",
                        autoComplete: "off"
                    })
                ]
            }, key);
        });
    if (loading) {
        return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
            className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_4__/* .container */ .kL,
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("h2", {
                    className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_4__/* .heading */ .R_,
                    children: "Background Models"
                }),
                /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("p", {
                    className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_4__/* .loadingText */ .KQ,
                    children: "Loading…"
                })
            ]
        });
    }
    if (loadError) {
        return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
            className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_4__/* .container */ .kL,
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("h2", {
                    className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_4__/* .heading */ .R_,
                    children: "Background Models"
                }),
                /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("div", {
                    role: "alert",
                    children: loadError
                }),
                /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("div", {
                    className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_4__/* .actions */ .o1,
                    children: /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("button", {
                        type: "button",
                        className: "btn btn-primary",
                        onClick: ()=>load(),
                        children: "Retry"
                    })
                })
            ]
        });
    }
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
        className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_4__/* .container */ .kL,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("h2", {
                className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_4__/* .heading */ .R_,
                children: "Background Models"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("p", {
                className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_4__/* .description */ .h_,
                children: "Unattended LLM work is pinned to a cheaper model instead of the account default. Leave a field blank to use the built-in default shown in it. Opus is never a default — enter it here to opt in."
            }),
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
                className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_4__/* .form */ .Zd,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("h3", {
                        children: "Headless features"
                    }),
                    renderRows("bgm-feature", features, featureDefaults, setFeatures),
                    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("h3", {
                        children: "Backlog stages"
                    }),
                    renderRows("bgm-stage", stages, stageDefaults, setStages),
                    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
                        className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_4__/* .field */ .ZZ,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("label", {
                                htmlFor: "bgm-effort",
                                className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_4__/* .label */ .Pf,
                                children: "Effort (work sessions)"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("select", {
                                id: "bgm-effort",
                                className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_4__/* .input */ .hF,
                                value: effort,
                                onChange: (e)=>setEffort(e.target.value),
                                children: [
                                    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("option", {
                                        value: "",
                                        children: "default (medium)"
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("option", {
                                        value: "off",
                                        children: "off (CLI default)"
                                    }),
                                    effortLevels.map((l)=>/*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("option", {
                                            value: l,
                                            children: l
                                        }, l))
                                ]
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("p", {
                                className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_4__/* .hint */ .aK,
                                children: "Passed as --effort to claude work sessions only; other programs are unchanged."
                            })
                        ]
                    }),
                    saveError && /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("p", {
                        role: "alert",
                        className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_4__/* .saveError */ .Ek,
                        children: saveError
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
                        className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_4__/* .actions */ .o1,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("button", {
                                type: "button",
                                className: "btn btn-primary",
                                onClick: handleSave,
                                disabled: saving,
                                children: saving ? "Saving…" : "Save"
                            }),
                            status && /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("span", {
                                role: "status",
                                className: _TaggingClassifierSettings_css__WEBPACK_IMPORTED_MODULE_4__/* .saveStatus */ .i5,
                                children: status
                            })
                        ]
                    })
                ]
            })
        ]
    });
}


/***/ }),

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

/***/ 6393:
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
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(65916);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2FsettingsPanelForm_css_ts_vanilla_css_22_2C_22source_22_3A_22LmFvZ2NhbTAgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IDFyZW07CiAgbWF4LXdpZHRoOiA2NDBweDsKfQouYW9nY2FtMSB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7CiAgZm9udC1zaXplOiAxLjI1cmVtOwogIGZvbnQtd2VpZ2h0OiA2MDA7CiAgbWFyZ2luOiAwOwp9Ci5hb2djYW0yIHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKICBmb250LXNpemU6IDAuODc1cmVtOwogIG1hcmdpbjogMDsKfQouYW9nY2FtMyB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7Cn0KLmFvZ2NhbTQgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IDEuMjVyZW07Cn0KLmFvZ2NhbTUgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IDAuNXJlbTsKfQouYW9nY2FtNiB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7CiAgZm9udC1zaXplOiAwLjg3NXJlbTsKICBmb250LXdlaWdodDogNjAwOwp9Ci5hb2djYW03IHsKICBkaXNwbGF5OiBmbGV4OwogIGdhcDogMC41cmVtOwogIGFsaWduLWl0ZW1zOiBjZW50ZXI7Cn0KLmFvZ2NhbTggewogIHBhZGRpbmc6IDAuNXJlbSAwLjc1cmVtOwogIGJhY2tncm91bmQtY29sb3I6IHZhcigtLV8xOWV6MnJ6MWEpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6ayk7CiAgYm9yZGVyLXJhZGl1czogNHB4OwogIGNvbG9yOiB2YXIoLS1fMTllejJyejFiKTsKICBmb250LXNpemU6IDAuODc1cmVtOwogIGZsZXg6IDE7Cn0KLmFvZ2NhbTg6Zm9jdXMgewogIG91dGxpbmU6IG5vbmU7CiAgYm9yZGVyLWNvbG9yOiB2YXIoLS1fMTllejJyemwpOwp9_22_7D__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(33545);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2FsettingsPanelForm_css_ts_vanilla_css_22_2C_22source_22_3A_22LmFvZ2NhbTAgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IDFyZW07CiAgbWF4LXdpZHRoOiA2NDBweDsKfQouYW9nY2FtMSB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7CiAgZm9udC1zaXplOiAxLjI1cmVtOwogIGZvbnQtd2VpZ2h0OiA2MDA7CiAgbWFyZ2luOiAwOwp9Ci5hb2djYW0yIHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKICBmb250LXNpemU6IDAuODc1cmVtOwogIG1hcmdpbjogMDsKfQouYW9nY2FtMyB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7Cn0KLmFvZ2NhbTQgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IDEuMjVyZW07Cn0KLmFvZ2NhbTUgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IDAuNXJlbTsKfQouYW9nY2FtNiB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7CiAgZm9udC1zaXplOiAwLjg3NXJlbTsKICBmb250LXdlaWdodDogNjAwOwp9Ci5hb2djYW03IHsKICBkaXNwbGF5OiBmbGV4OwogIGdhcDogMC41cmVtOwogIGFsaWduLWl0ZW1zOiBjZW50ZXI7Cn0KLmFvZ2NhbTggewogIHBhZGRpbmc6IDAuNXJlbSAwLjc1cmVtOwogIGJhY2tncm91bmQtY29sb3I6IHZhcigtLV8xOWV6MnJ6MWEpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6ayk7CiAgYm9yZGVyLXJhZGl1czogNHB4OwogIGNvbG9yOiB2YXIoLS1fMTllejJyejFiKTsKICBmb250LXNpemU6IDAuODc1cmVtOwogIGZsZXg6IDE7Cn0KLmFvZ2NhbTg6Zm9jdXMgewogIG91dGxpbmU6IG5vbmU7CiAgYm9yZGVyLWNvbG9yOiB2YXIoLS1fMTllejJyemwpOwp9_22_7D__WEBPACK_IMPORTED_MODULE_1___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2FsettingsPanelForm_css_ts_vanilla_css_22_2C_22source_22_3A_22LmFvZ2NhbTAgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IDFyZW07CiAgbWF4LXdpZHRoOiA2NDBweDsKfQouYW9nY2FtMSB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7CiAgZm9udC1zaXplOiAxLjI1cmVtOwogIGZvbnQtd2VpZ2h0OiA2MDA7CiAgbWFyZ2luOiAwOwp9Ci5hb2djYW0yIHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKICBmb250LXNpemU6IDAuODc1cmVtOwogIG1hcmdpbjogMDsKfQouYW9nY2FtMyB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7Cn0KLmFvZ2NhbTQgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IDEuMjVyZW07Cn0KLmFvZ2NhbTUgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IDAuNXJlbTsKfQouYW9nY2FtNiB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7CiAgZm9udC1zaXplOiAwLjg3NXJlbTsKICBmb250LXdlaWdodDogNjAwOwp9Ci5hb2djYW03IHsKICBkaXNwbGF5OiBmbGV4OwogIGdhcDogMC41cmVtOwogIGFsaWduLWl0ZW1zOiBjZW50ZXI7Cn0KLmFvZ2NhbTggewogIHBhZGRpbmc6IDAuNXJlbSAwLjc1cmVtOwogIGJhY2tncm91bmQtY29sb3I6IHZhcigtLV8xOWV6MnJ6MWEpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6ayk7CiAgYm9yZGVyLXJhZGl1czogNHB4OwogIGNvbG9yOiB2YXIoLS1fMTllejJyejFiKTsKICBmb250LXNpemU6IDAuODc1cmVtOwogIGZsZXg6IDE7Cn0KLmFvZ2NhbTg6Zm9jdXMgewogIG91dGxpbmU6IG5vbmU7CiAgYm9yZGVyLWNvbG9yOiB2YXIoLS1fMTllejJyemwpOwp9_22_7D__WEBPACK_IMPORTED_MODULE_1__);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fsettings_2FJulesSettings_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA62U33biIBDG732Kudy9wJNobZQ_TA_GMc4WgQIxxj199x6ibbJu_tieXgIzH9_8ZmD_vH5d_tdjAn9nALlRxnE4CveLsed0g_eFOy9_P80AdkYH5umMHJL5Ol2sHB7i_kG4gjSH5Gn2Nptf1dJhtXOjthX5S_FMqSXrjaovUcZJdBxSewJvFMmbqFMnijkhqfQcHuwp7lohJekiul1m0Swk88dP11_qZtFUI8lbJWoOO4XNDUJRoRkFPPjLJvNBuBCPCmGj8EW1FVoOY0l6IGe3_Q9T_VNY0_wuri_3cb1SzcahtvZX_RwbdJIc5oGM5rG_8qA7GJdZT3u6TuJKkQ_Mh1ohB200di9_vKOBOeqALm7_KX2gXc1yowPqwMFbkSPbYqgQ9X_tHUUyRRoHSbfus29NDYA5otspU3HYk5QX5wFPgbUHqBRZTz4eVXsKyJpaI8HKCdt1sW5c9L2qttip6Uvvmr40GRm_fuHxLyovnY9J1tBHj6eK5fsIqSnZWJFTqKPm5t8YSV5sFcrbsFX3Um0CE0qZCmU3ezPc1Orr_6340d97_6Nqef_ju76hz_EZeIzXgWPB9P6o8nsYWwEcFhidqrd3orAnkTkHAAA_22_7D__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(14217);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fsettings_2FJulesSettings_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA62U33biIBDG732Kudy9wJNobZQ_TA_GMc4WgQIxxj199x6ibbJu_tieXgIzH9_8ZmD_vH5d_tdjAn9nALlRxnE4CveLsed0g_eFOy9_P80AdkYH5umMHJL5Ol2sHB7i_kG4gjSH5Gn2Nptf1dJhtXOjthX5S_FMqSXrjaovUcZJdBxSewJvFMmbqFMnijkhqfQcHuwp7lohJekiul1m0Swk88dP11_qZtFUI8lbJWoOO4XNDUJRoRkFPPjLJvNBuBCPCmGj8EW1FVoOY0l6IGe3_Q9T_VNY0_wuri_3cb1SzcahtvZX_RwbdJIc5oGM5rG_8qA7GJdZT3u6TuJKkQ_Mh1ohB200di9_vKOBOeqALm7_KX2gXc1yowPqwMFbkSPbYqgQ9X_tHUUyRRoHSbfus29NDYA5otspU3HYk5QX5wFPgbUHqBRZTz4eVXsKyJpaI8HKCdt1sW5c9L2qttip6Uvvmr40GRm_fuHxLyovnY9J1tBHj6eK5fsIqSnZWJFTqKPm5t8YSV5sFcrbsFX3Um0CE0qZCmU3ezPc1Orr_6340d97_6Nqef_ju76hz_EZeIzXgWPB9P6o8nsYWwEcFhidqrd3orAnkTkHAAA_22_7D__WEBPACK_IMPORTED_MODULE_2___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fsettings_2FJulesSettings_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA62U33biIBDG732Kudy9wJNobZQ_TA_GMc4WgQIxxj199x6ibbJu_tieXgIzH9_8ZmD_vH5d_tdjAn9nALlRxnE4CveLsed0g_eFOy9_P80AdkYH5umMHJL5Ol2sHB7i_kG4gjSH5Gn2Nptf1dJhtXOjthX5S_FMqSXrjaovUcZJdBxSewJvFMmbqFMnijkhqfQcHuwp7lohJekiul1m0Swk88dP11_qZtFUI8lbJWoOO4XNDUJRoRkFPPjLJvNBuBCPCmGj8EW1FVoOY0l6IGe3_Q9T_VNY0_wuri_3cb1SzcahtvZX_RwbdJIc5oGM5rG_8qA7GJdZT3u6TuJKkQ_Mh1ohB200di9_vKOBOeqALm7_KX2gXc1yowPqwMFbkSPbYqgQ9X_tHUUyRRoHSbfus29NDYA5otspU3HYk5QX5wFPgbUHqBRZTz4eVXsKyJpaI8HKCdt1sW5c9L2qttip6Uvvmr40GRm_fuHxLyovnY9J1tBHj6eK5fsIqSnZWJFTqKPm5t8YSV5sFcrbsFX3Um0CE0qZCmU3ezPc1Orr_6340d97_6Nqef_ju76hz_EZeIzXgWPB9P6o8nsYWwEcFhidqrd3orAnkTkHAAA_22_7D__WEBPACK_IMPORTED_MODULE_2__);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fsettings_2FTaggingClassifierSettings_css_ts_vanilla_css_22_2C_22source_22_3A_22LnB1NzZwYzAgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IDAuNXJlbTsKfQoucHU3NnBjMSB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7CiAgZm9udC1zaXplOiAwLjg3NXJlbTsKICBmbGV4OiAxIDEgYXV0bzsKfQoucHU3NnBjMiB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MTApOwogIGJhY2tncm91bmQ6IHRyYW5zcGFyZW50OwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6ayk7CiAgYm9yZGVyLXJhZGl1czogNHB4OwogIHBhZGRpbmc6IDAuMjVyZW0gMC42MjVyZW07CiAgZm9udC1zaXplOiAwLjgxMjVyZW07CiAgY3Vyc29yOiBwb2ludGVyOwp9Ci5wdTc2cGMzIHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKICBmb250LXNpemU6IDAuODEyNXJlbTsKICBtYXJnaW46IDA7Cn0_22_7D__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(39058);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fsettings_2FTaggingClassifierSettings_css_ts_vanilla_css_22_2C_22source_22_3A_22LnB1NzZwYzAgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IDAuNXJlbTsKfQoucHU3NnBjMSB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7CiAgZm9udC1zaXplOiAwLjg3NXJlbTsKICBmbGV4OiAxIDEgYXV0bzsKfQoucHU3NnBjMiB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MTApOwogIGJhY2tncm91bmQ6IHRyYW5zcGFyZW50OwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6ayk7CiAgYm9yZGVyLXJhZGl1czogNHB4OwogIHBhZGRpbmc6IDAuMjVyZW0gMC42MjVyZW07CiAgZm9udC1zaXplOiAwLjgxMjVyZW07CiAgY3Vyc29yOiBwb2ludGVyOwp9Ci5wdTc2cGMzIHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKICBmb250LXNpemU6IDAuODEyNXJlbTsKICBtYXJnaW46IDA7Cn0_22_7D__WEBPACK_IMPORTED_MODULE_3___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fsettings_2FTaggingClassifierSettings_css_ts_vanilla_css_22_2C_22source_22_3A_22LnB1NzZwYzAgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IDAuNXJlbTsKfQoucHU3NnBjMSB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7CiAgZm9udC1zaXplOiAwLjg3NXJlbTsKICBmbGV4OiAxIDEgYXV0bzsKfQoucHU3NnBjMiB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MTApOwogIGJhY2tncm91bmQ6IHRyYW5zcGFyZW50OwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6ayk7CiAgYm9yZGVyLXJhZGl1czogNHB4OwogIHBhZGRpbmc6IDAuMjVyZW0gMC42MjVyZW07CiAgZm9udC1zaXplOiAwLjgxMjVyZW07CiAgY3Vyc29yOiBwb2ludGVyOwp9Ci5wdTc2cGMzIHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKICBmb250LXNpemU6IDAuODEyNXJlbTsKICBtYXJnaW46IDA7Cn0_22_7D__WEBPACK_IMPORTED_MODULE_3__);




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

/***/ 14217:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 24044:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Bl: () => (/* binding */ AnalyticsContextProvider),
/* harmony export */   Ig: () => (/* binding */ AnalyticsContext),
/* harmony export */   st: () => (/* binding */ useAnalytics)
/* harmony export */ });
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(61574);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(55494);
/* __next_internal_client_entry_do_not_use__ AnalyticsContext,useAnalytics,AnalyticsContextProvider auto */ 

const AnalyticsContext = /*#__PURE__*/ (0,react__WEBPACK_IMPORTED_MODULE_1__.createContext)(null);
function useAnalytics() {
    const context = (0,react__WEBPACK_IMPORTED_MODULE_1__.useContext)(AnalyticsContext);
    if (!context) {
        throw new Error("useAnalytics must be used within an AnalyticsContextProvider");
    }
    return context;
}
function AnalyticsContextProvider(param) {
    let { provider, children } = param;
    // Stable ref to the provider so identity doesn't change across parent re-renders
    const providerRef = (0,react__WEBPACK_IMPORTED_MODULE_1__.useRef)(provider);
    providerRef.current = provider;
    (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        var _p_initialize;
        const p = provider;
        providerRef.current = p;
        void ((_p_initialize = p.initialize) === null || _p_initialize === void 0 ? void 0 : _p_initialize.call(p));
        return ()=>{
            var _p_onClose;
            void ((_p_onClose = p.onClose) === null || _p_onClose === void 0 ? void 0 : _p_onClose.call(p));
        };
    }, [
        provider
    ]);
    const contextValue = (0,react__WEBPACK_IMPORTED_MODULE_1__.useMemo)(()=>({
            provider: providerRef.current,
            track: (event)=>providerRef.current.track(event)
        }), // track delegates through the ref so consumers don't re-render on provider swap
    // eslint-disable-next-line react-hooks/exhaustive-deps
    []);
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(AnalyticsContext.Provider, {
        value: contextValue,
        children: children
    });
}


/***/ }),

/***/ 33545:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 39058:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 41078:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   KA: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.useRouter),
/* harmony export */   N_: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.Link),
/* harmony export */   a8: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.usePathname),
/* harmony export */   nQ: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.replaceHash),
/* harmony export */   ok: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.useSearchParams),
/* harmony export */   tk: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.pushHistoryMarker),
/* harmony export */   wV: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.useReplaceSearch),
/* harmony export */   x7: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.pushTrayEntry)
/* harmony export */ });
/* harmony import */ var _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(99863);
// `@/lib/nav/impl` is aliased per build (next.config.ts, tsconfig paths, jest moduleNameMapper); everything outside this directory imports from here.




/***/ }),

/***/ 43963:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   i: () => (/* binding */ file_google_protobuf_timestamp),
/* harmony export */   w: () => (/* binding */ TimestampSchema)
/* harmony export */ });
/* harmony import */ var _codegenv2_file_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(1844);
/* harmony import */ var _codegenv2_message_js__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(15033);
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.


/**
 * Describes the file google/protobuf/timestamp.proto.
 */
const file_google_protobuf_timestamp = /*@__PURE__*/ (0,_codegenv2_file_js__WEBPACK_IMPORTED_MODULE_0__/* .fileDesc */ .w)("Ch9nb29nbGUvcHJvdG9idWYvdGltZXN0YW1wLnByb3RvEg9nb29nbGUucHJvdG9idWYiKwoJVGltZXN0YW1wEg8KB3NlY29uZHMYASABKAMSDQoFbmFub3MYAiABKAVChQEKE2NvbS5nb29nbGUucHJvdG9idWZCDlRpbWVzdGFtcFByb3RvUAFaMmdvb2dsZS5nb2xhbmcub3JnL3Byb3RvYnVmL3R5cGVzL2tub3duL3RpbWVzdGFtcHBi+AEBogIDR1BCqgIeR29vZ2xlLlByb3RvYnVmLldlbGxLbm93blR5cGVzYgZwcm90bzM");
/**
 * Describes the message google.protobuf.Timestamp.
 * Use `create(TimestampSchema)` to create a new message.
 */
const TimestampSchema = /*@__PURE__*/ (0,_codegenv2_message_js__WEBPACK_IMPORTED_MODULE_1__/* .messageDesc */ .Z)(file_google_protobuf_timestamp, 0);


/***/ }),

/***/ 65916:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 71959:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 3728));
;
Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 2972));


/***/ }),

/***/ 90814:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   O: () => (/* binding */ usePageView)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _lib_nav__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(41078);
/* harmony import */ var _lib_contexts_AnalyticsContext__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(24044);
/* __next_internal_client_entry_do_not_use__ usePageView auto */ 


function usePageView() {
    const pathname = (0,_lib_nav__WEBPACK_IMPORTED_MODULE_1__/* .usePathname */ .a8)();
    const { track } = (0,_lib_contexts_AnalyticsContext__WEBPACK_IMPORTED_MODULE_2__/* .useAnalytics */ .st)();
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        track({
            name: "page_view",
            category: "navigation",
            page: pathname
        });
    }, [
        pathname,
        track
    ]);
}


/***/ }),

/***/ 99863:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  Link: () => (/* binding */ Link),
  Redirect: () => (/* binding */ Redirect),
  pushHistoryMarker: () => (/* binding */ pushHistoryMarker),
  pushTrayEntry: () => (/* binding */ pushTrayEntry),
  replaceHash: () => (/* binding */ replaceHash),
  usePathname: () => (/* binding */ usePathname),
  useReplaceSearch: () => (/* binding */ useReplaceSearch),
  useRouter: () => (/* binding */ useRouter),
  useSearchParams: () => (/* binding */ useSearchParams)
});

// UNUSED EXPORTS: nextImpl

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/client/app-dir/link.js
var app_dir_link = __webpack_require__(6024);
var link_default = /*#__PURE__*/__webpack_require__.n(app_dir_link);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/api/navigation.js
var navigation = __webpack_require__(45853);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
;// ./src/lib/nav/history.ts
// The only place outside an impl that touches raw History; every impl shares it so foreign state survives the same way.
const TRAY_MARKER = "ssqTray";
function replaceSearchInHistory(search) {
    const query = search === "" || search.startsWith("?") ? search : "?".concat(search);
    window.history.replaceState(window.history.state, "", query === "" ? window.location.pathname : query);
}
function replaceHashInHistory(hash) {
    const url = hash ? "".concat(window.location.pathname).concat(window.location.search).concat(hash.startsWith("#") ? hash : "#".concat(hash)) : window.location.pathname + window.location.search;
    window.history.replaceState(window.history.state, "", url);
}
function pushHistoryMarkerInHistory(key, value) {
    let replace = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : false;
    var _window_history_state;
    const state = {
        ...(_window_history_state = window.history.state) !== null && _window_history_state !== void 0 ? _window_history_state : {},
        [key]: value
    };
    if (replace) window.history.replaceState(state, "");
    else window.history.pushState(state, "");
}
function pushTrayEntryInHistory() {
    let replace = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : false;
    pushHistoryMarkerInHistory(TRAY_MARKER, true, replace);
}

;// ./src/lib/nav/normalize.ts
/** Strips trailing slashes (Next's `trailingSlash` export reports `/x/`) but keeps the root `/`. */ function normalizePathname(pathname) {
    const trimmed = pathname.replace(/\/+$/, "");
    return trimmed === "" ? "/" : trimmed;
}

;// ./src/lib/nav/impl.next.tsx
/* __next_internal_client_entry_do_not_use__ useRouter,useSearchParams,usePathname,Link,Redirect,useReplaceSearch,replaceHash,pushTrayEntry,pushHistoryMarker,nextImpl auto */ 





const toNextOptions = (options)=>(options === null || options === void 0 ? void 0 : options.resetScroll) === false ? {
        scroll: false
    } : undefined;
function useRouter() {
    const router = (0,navigation.useRouter)();
    return (0,react.useMemo)(()=>({
            push: (href, options)=>router.push(href, toNextOptions(options)),
            replace: (href, options)=>router.replace(href, toNextOptions(options)),
            back: ()=>router.back(),
            forward: ()=>router.forward()
        }), [
        router
    ]);
}
function useSearchParams() {
    return (0,navigation.useSearchParams)();
}
function usePathname() {
    var _useNextPathname;
    return normalizePathname((_useNextPathname = (0,navigation.usePathname)()) !== null && _useNextPathname !== void 0 ? _useNextPathname : "/");
}
/** Prefetch stays off: it triggers CSS preload warnings (the reason the NavLink this replaces existed). */ const Link = /*#__PURE__*/ (0,react.forwardRef)(function Link(param, ref) {
    let { resetScroll, ...props } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)((link_default()), {
        ref: ref,
        prefetch: false,
        scroll: resetScroll === false ? false : undefined,
        ...props
    });
});
/** Next's render-time `redirect()`: the behaviour the two redirect-only pages have today. */ function Redirect(param) {
    let { to } = param;
    (0,navigation.redirect)(to);
}
function useReplaceSearch() {
    return (0,react.useCallback)((search)=>replaceSearchInHistory(search), []);
}
const replaceHash = replaceHashInHistory;
const pushTrayEntry = pushTrayEntryInHistory;
const pushHistoryMarker = pushHistoryMarkerInHistory;
const nextImpl = {
    useRouter,
    useSearchParams,
    usePathname,
    Link,
    Redirect,
    useReplaceSearch,
    replaceHash,
    pushTrayEntry,
    pushHistoryMarker
};


/***/ })

},
/******/ __webpack_require__ => { // webpackRuntimeModules
/******/ var __webpack_exec__ = (moduleId) => (__webpack_require__(__webpack_require__.s = moduleId))
/******/ __webpack_require__.O(0, [1869,852,4202,9353,8165,3575,9691,3516,7358], () => (__webpack_exec__(71959)));
/******/ var __webpack_exports__ = __webpack_require__.O();
/******/ _N_E = __webpack_exports__;
/******/ }
]);
//# sourceMappingURL=page-bb178965585353ac.js.map