(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[3116],{

/***/ 3542:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   C: () => (/* binding */ fuzzyMatchModels),
/* harmony export */   i: () => (/* binding */ getModelLabel)
/* harmony export */ });
/* harmony import */ var fuse_js__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(66000);
/* harmony import */ var _programs__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(49051);


// Search both the human label ("Sonnet (latest)") and the raw value
// ("claude-sonnet-4-6") so typos against either surface a result.
const FUSE_OPTIONS = {
    keys: [
        {
            name: "label",
            weight: 0.6
        },
        {
            name: "value",
            weight: 0.4
        }
    ],
    includeScore: true,
    threshold: 0.4,
    minMatchCharLength: 1,
    ignoreLocation: true
};
const modelFuse = new fuse_js__WEBPACK_IMPORTED_MODULE_1__/* ["default"] */ .A(_programs__WEBPACK_IMPORTED_MODULE_0__/* .MODEL_AUTOCOMPLETE_OPTIONS */ .kg, FUSE_OPTIONS);
/**
 * Fuzzy/typo-tolerant filter for the Workflow model field's AutocompleteInput.
 * `suggestions` is the value list passed to AutocompleteInput (kept as the
 * signature contract so it can be used directly as a `filterFn`); results are
 * ranked best-match-first by Fuse.js and restricted to that value list.
 */ function fuzzyMatchModels(query, suggestions) {
    const trimmed = query.trim();
    if (!trimmed) return suggestions;
    const allowed = new Set(suggestions);
    return modelFuse.search(trimmed).map((r)=>r.item.value).filter((value)=>allowed.has(value));
}
/** Display label for a model suggestion value; falls back to the raw value. */ function getModelLabel(value) {
    var _MODEL_AUTOCOMPLETE_OPTIONS_find;
    var _MODEL_AUTOCOMPLETE_OPTIONS_find_label;
    return (_MODEL_AUTOCOMPLETE_OPTIONS_find_label = (_MODEL_AUTOCOMPLETE_OPTIONS_find = _programs__WEBPACK_IMPORTED_MODULE_0__/* .MODEL_AUTOCOMPLETE_OPTIONS */ .kg.find((o)=>o.value === value)) === null || _MODEL_AUTOCOMPLETE_OPTIONS_find === void 0 ? void 0 : _MODEL_AUTOCOMPLETE_OPTIONS_find.label) !== null && _MODEL_AUTOCOMPLETE_OPTIONS_find_label !== void 0 ? _MODEL_AUTOCOMPLETE_OPTIONS_find_label : value;
}


/***/ }),

/***/ 39762:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 49051:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
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

/***/ 55963:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  P: () => (/* binding */ AutocompleteInput)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fui%2FAutocompleteInput.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA81V247aMBB95yumD5VAxZETspR1X_onlRMbYuHY1thZslT771UcsoTLZrfaPlSAZNlzOXPmzJDYfIV-Q-H3DMBZr4KyhgFKzYN6kj9mAAclQsUgpfTrj9nLLBk8lHFNiH4XFgCOC6HMjgFN1tkDyhposvneHbrXwqKQyCB1LXirlYAnjnNCfqWP8pjhUS7OVgS5UI1nsHZtd7u1JhCvjrKL_bh6jRmQmwH6ybO02iLQJPNLKGxLfMWFPcSLGJ-X-x3axgh2lT_lEUD0v36ji1sG2NaWjY882CZoZSQDY40cVXE3Vn2qc4DGgMbPyrWAu4LP6RLSNFtClq-WQJP0Tu5EIlqMuadSpfdw974j9BNQstXjEtab_ncfChPK80JL0TPheKnCc5RAZLNB3-EyNhCutT1IMd0FMU6RfqRC-KJqZzFwE0au2ZWweeGtbkLsTrCOQcl1Oe-UC98gd21sipbbwIB2R1S7ajjXvCWV7C-ynPaStE8St9oeyDMD3gQ7XdZ68dkRuOxS7truAUg6Us3pm6SLJVDIXBvN7prQHpBWPhAfnvVIuzXHnTKn0kcjnQ1DdyTKCNnGuadjyhkjB1nsVSC-RKt1wXG8JjZdIZPWJCAv933TJxVyQ1L-gdhVUxfvxK4-E5tVnSbeyZCN9b3qRTq5NocJclaZIPEtfFe78DX_6z5MH_zkentryZ7Bfqi-i_nN_8b44R9O7IjS8__P_zKcb7YhvW3DWQdBtt0KVTvDoJSDFG5m8WctheIwdyi3En3ffuLLStaSgeC4X0SeLzo0RU6vjZfZyx9GB_CzMggAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FAutocompleteInput_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81V247aMBB95yumD5VAxZETspR1X_onlRMbYuHY1thZslT771UcsoTLZrfaPlSAZNlzOXPmzJDYfIV_Q_H3DMBZr4KyhgFKzYN6kj9mAAclQsUgpfTrj9nLLBk8lHFNiH4XFgCOC6HMjgFN1tkDyhposvneHbrXwqKQyCB1LXirlYAnjnNCfqWP8pjhUS7OVgS5UI1nsHZtd7u1JhCvjrKL_bh6jRmQmwH6ybO02iLQJPNLKGxLfMWFPcSLGJ_X_x3axgh2lT_lEUD0v36ji1sG2NaWjY882CZoZSQDY40cVXE3Vn2qc4DGgMbPyrWAu4LP6RLSNFtClq_WQJP0Tu5EIlqMuadSpfdw974j9BNQstXjEtab_ncfChPK80JL0TPheKnCc5RAZLNB3_EyNhCutT1IMd0FMU6RfqRC_KJqZzFwE0au2ZWweeGtbkLsTrCOQcl1Oe_UC98gd21sipbbwIB2R1S7ajjXvCWV7C_ynPaStE8St9oeyDMD3gQ7XdZ68dkRuOxS7truAUg6Us3pm6SLJVDIXBvN7prQHpBWPhAfnvVIuzXHnTKn0kcjnQ1DdyTKCNnGuadjyhkjB1nsVSC_RKt1wXG8JjZdIZPWJCAv933TJxVyQ1L_gdhVUxfvxK4_E5tVnSbeyZCN9b3qRTq5NocJclaZIPEtfFe78DX_6z5MH_zkentryZ7Bfqi_i_nN_8b44R9O7IjS8_P_zKcb7YhvW3DWQdBtt0KVTvDoJSDFG5m8WctheIwdyi3En3ffuLLStaSgeC4X0SeLzo0RU6vjZfZyx9GB_CzMggAAA_22_7D = __webpack_require__(39762);
;// ./src/components/ui/AutocompleteInput.css.ts


var container = 'o43rs80';
var AutocompleteInput_css_error = 'o43rs81';
var highlighted = 'o43rs84';
var loading = 'o43rs85';
var suggestion = 'o43rs83';
var AutocompleteInput_css_suggestions = 'o43rs82';
;// ./src/components/ui/AutocompleteInput.tsx
/* __next_internal_client_entry_do_not_use__ AutocompleteInput auto */ 


/** Default filter: plain case-insensitive substring match (unchanged pre-existing behavior). */ function defaultFilterFn(query, suggestions) {
    return suggestions.filter((suggestion)=>suggestion.toLowerCase().includes(query.toLowerCase()));
}
function AutocompleteInput(param) {
    let { id, value, onChange, onBlur, placeholder, suggestions, isLoading = false, className = "", error = false, disabled = false, "data-testid": dataTestId, filterFn = defaultFilterFn, getLabel = (v)=>v } = param;
    const [isOpen, setIsOpen] = (0,react.useState)(false);
    const [highlightedIndex, setHighlightedIndex] = (0,react.useState)(-1);
    const inputRef = (0,react.useRef)(null);
    const listRef = (0,react.useRef)(null);
    // Filter (and, when filterFn is fuzzy, rank) suggestions based on input value
    const filteredSuggestions = filterFn(value, suggestions);
    // Close dropdown when clicking outside
    (0,react.useEffect)(()=>{
        const handleClickOutside = (event)=>{
            if (inputRef.current && !inputRef.current.contains(event.target) && listRef.current && !listRef.current.contains(event.target)) {
                setIsOpen(false);
            }
        };
        document.addEventListener("mousedown", handleClickOutside);
        return ()=>document.removeEventListener("mousedown", handleClickOutside);
    }, []);
    // Scroll highlighted item into view
    (0,react.useEffect)(()=>{
        if (listRef.current && highlightedIndex >= 0) {
            const highlightedElement = listRef.current.children[highlightedIndex];
            if (highlightedElement) {
                highlightedElement.scrollIntoView({
                    block: "nearest",
                    behavior: "smooth"
                });
            }
        }
    }, [
        highlightedIndex
    ]);
    const handleInputChange = (newValue)=>{
        onChange(newValue);
        setIsOpen(true);
        setHighlightedIndex(-1);
    };
    const handleSuggestionClick = (suggestion)=>{
        onChange(suggestion);
        setIsOpen(false);
        setHighlightedIndex(-1);
    };
    const handleKeyDown = (e)=>{
        if (!isOpen || filteredSuggestions.length === 0) {
            if (e.key === "ArrowDown" && filteredSuggestions.length > 0) {
                setIsOpen(true);
                setHighlightedIndex(0);
                e.preventDefault();
            }
            return;
        }
        switch(e.key){
            case "ArrowDown":
                e.preventDefault();
                setHighlightedIndex((prev)=>prev < filteredSuggestions.length - 1 ? prev + 1 : prev);
                break;
            case "ArrowUp":
                e.preventDefault();
                setHighlightedIndex((prev)=>prev > 0 ? prev - 1 : -1);
                break;
            case "Enter":
                e.preventDefault();
                if (highlightedIndex >= 0 && highlightedIndex < filteredSuggestions.length) {
                    handleSuggestionClick(filteredSuggestions[highlightedIndex]);
                }
                break;
            case "Escape":
                e.preventDefault();
                setIsOpen(false);
                setHighlightedIndex(-1);
                break;
            case "Tab":
                // Allow tab to close dropdown but don't prevent default
                setIsOpen(false);
                setHighlightedIndex(-1);
                break;
        }
    };
    const handleFocus = ()=>{
        if (filteredSuggestions.length > 0) {
            setIsOpen(true);
        }
    };
    const handleBlur = ()=>{
        onBlur === null || onBlur === void 0 ? void 0 : onBlur();
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                ref: inputRef,
                id: id,
                type: "text",
                value: value,
                onChange: (e)=>handleInputChange(e.target.value),
                onKeyDown: handleKeyDown,
                onFocus: handleFocus,
                onBlur: handleBlur,
                placeholder: placeholder,
                className: "".concat(className, " ").concat(error ? AutocompleteInput_css_error : ""),
                disabled: disabled,
                "data-testid": dataTestId,
                autoComplete: "off"
            }),
            isOpen && filteredSuggestions.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                ref: listRef,
                className: AutocompleteInput_css_suggestions,
                role: "listbox",
                children: filteredSuggestions.map((sugg, index)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("li", {
                        className: "".concat(suggestion, " ").concat(index === highlightedIndex ? highlighted : ""),
                        onClick: ()=>handleSuggestionClick(sugg),
                        onMouseEnter: ()=>setHighlightedIndex(index),
                        role: "option",
                        "aria-selected": index === highlightedIndex,
                        children: getLabel(sugg)
                    }, sugg))
            }),
            isOpen && isLoading && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: loading,
                children: "Loading suggestions..."
            })
        ]
    });
}


/***/ }),

/***/ 67109:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 83315:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 93656));


/***/ }),

/***/ 93656:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  "default": () => (/* binding */ PipelineModesPage)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/hooks/useBacklogService.ts
var useBacklogService = __webpack_require__(8570);
// EXTERNAL MODULE: ./src/components/analytics/PageViewTracker.tsx
var PageViewTracker = __webpack_require__(3728);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/connect-error.js
var connect_error = __webpack_require__(45640);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/code.js
var code = __webpack_require__(16294);
// EXTERNAL MODULE: ./src/components/ui/AutocompleteInput.tsx + 1 modules
var AutocompleteInput = __webpack_require__(55963);
// EXTERNAL MODULE: ./src/lib/constants/programs.ts
var programs = __webpack_require__(49051);
// EXTERNAL MODULE: ./src/lib/constants/modelFuzzyMatch.ts
var modelFuzzyMatch = __webpack_require__(3542);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Fa11y.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll85amk4YnAwIHsKICBwb3NpdGlvbjogYWJzb2x1dGU7CiAgd2lkdGg6IDFweDsKICBoZWlnaHQ6IDFweDsKICBwYWRkaW5nOiAwOwogIG1hcmdpbjogLTFweDsKICBvdmVyZmxvdzogaGlkZGVuOwogIGNsaXAtcGF0aDogaW5zZXQoNTAlKTsKICB3aGl0ZS1zcGFjZTogbm93cmFwOwogIGJvcmRlcjogMDsKfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Fa11y_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll85amk4YnAwIHsKICBwb3NpdGlvbjogYWJzb2x1dGU7CiAgd2lkdGg6IDFweDsKICBoZWlnaHQ6IDFweDsKICBwYWRkaW5nOiAwOwogIG1hcmdpbjogLTFweDsKICBvdmVyZmxvdzogaGlkZGVuOwogIGNsaXAtcGF0aDogaW5zZXQoNTAlKTsKICB3aGl0ZS1zcGFjZTogbm93cmFwOwogIGJvcmRlcjogMDsKfQ_22_7D = __webpack_require__(47439);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2FsettingsResourceForm.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA9WVwW6jMBCG73kKHzcHIkw2aeM8TDXYA7FqPKwxbUPVd1_Fm0jBhIVsK616QoKZbzw_v2dWT_xXWhUdpOx9wZjSTW3gKFhh8G2_YOGZKO1Qek1WMEmmrezpSwm1YC_gfiTJE99hl7luzZf7xcdidWHyL2Bmxz4zC8yCrE8a3eHgCPlyf_n-iro8eMG2aXp6J8mQi-PTPn09QYflKCnrk34GUg1KaVsOYrtlTE4DOQf5XDpqrYpT-J_SOTmFTjBev7GGjFZR2PNVVOJA6bYZNLEdb4JfyTcq73WXoiDZNqHXc9GbXBOw1HqjLQpmyWLEEbUBiQcyCl3A3T6f7NffhFCqQWp_FCxdbUNrrWtOuZZ8AsbQK6pe1va2L8Ho0ibaY9UIJtF6dGOm7PrHePgSIGMVuFLbxNNkxcc77cVn-2Lqal3ErUlfGpoyZTXh7WrckJvhdd6crvO1FOJAL2fTjNewkX5C6QZygyp20GaOg3bfSX4cyO8d2KYGh9bPnYy7WSo_9pMghJ9dbbDwgkHraf8vk_H_aMeHc3mOeDzaBjBLPc7jrE-ZNP826mdj6q-ntuL6rtHxEI2O_HMCy78sqmzq5HzWPufz9b_3n04s-d_wiHxRGgoAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2FsettingsResourceForm_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9WVwW6jMBCG73kKHzcHIkw2aeM8TDXYA7FqPKwxbUPVd1_Fm0jBhIVsK616QoKZbzw_v2dWT_xXWhUdpOx9wZjSTW3gKFhh8G2_YOGZKO1Qek1WMEmmrezpSwm1YC_gfiTJE99hl7luzZf7xcdidWHyL2Bmxz4zC8yCrE8a3eHgCPlyf_n_iro8eMG2aXp6J8mQi_PTPn09QYflKCnrk34GUg1KaVsOYrtlTE4DOQf5XDpqrYpT_J_SOTmFTjBev7GGjFZR2PNVVOJA6bYZNLEdb4JfyTcq73WXoiDZNqHXc9GbXBOw1HqjLQpmyWLEEbUBiQcyCl3A3T6f7NffhFCqQWp_FCxdbUNrrWtOuZZ8AsbQK6pe1va2L8Ho0ibaY9UIJtF6dGOm7PrHePgSIGMVuFLbxNNkxcc77cVn_2Lqal3ErUlfGpoyZTXh7WrckJvhdd6crvO1FOJAL2fTjNewkX5C6QZygyp20GaOg3bfSX4cyO8d2KYGh9bPnYy7WSo_9pMghJ9dbbDwgkHraf8vk_H_aMeHc3mOeDzaBjBLPc7jrE_ZNP826mdj6q_ntuL6rtHxEI2O_HMCy78sqmzq5HzWPufz9b_3n04s_d_wiHxRGgoAAA_22_7D = __webpack_require__(3339);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Fsettings%2Fpipeline-modes%2FPipelineModeForm.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA62T3Y7CIBCF7_sUc7PJeoGhddcf-jAG6bRlQ4FQam03vvum6Gat2jUmXkGAOfPNmWG-jVXTrL88he8IIDfak5xXUnUM9ty9E7KNN9gnrk-aWRoBVFKTEmVRegarxB6GM4e17JHBHp2Xgqs0OkbzX-E4CGeytop3DHKFIWZYSSYdCi-NZiCMaio93BTcXude0NlIM3mBZtKPNRd_BpyrGSPsQvnhvj3Xv6R0OBNGGXf9np7c4q6QmgEdpfoIqVqZ-ZJBTOnb8HRnXIaOCKMUtzUG-rBLH2JdSH8GaY8HT7iShWagMPeDhOVZJnVxY0M3u2fMRFXJpAnThHxMuHwZ4ZO9aEvpkdSWC2SgTeu4HYGtAthzEBfh6xB-ajjx5nbeukDx2qHd3P8IwVgiPVY1A4Hao_tP8FH3JmyNh2k4_gBsdDjcQgQAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Fsettings_2Fpipeline_modes_2FPipelineModeForm_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA62T3Y7CIBCF7_sUc7PJeoGhddcf_jAG6bRlQ4FQam03vvum6Gat2jUmXkGAOfPNmWG_jVXTrL88he8IIDfak5xXUnUM9ty9E7KNN9gnrk_aWRoBVFKTEmVRegarxB6GM4e17JHBHp2Xgqs0OkbzX_E4CGeytop3DHKFIWZYSSYdCi_NZiCMaio93BTcXude0NlIM3mBZtKPNRd_BpyrGSPsQvnhvj3Xv6R0OBNGGXf9np7c4q6QmgEdpfoIqVqZ_ZJBTOnb8HRnXIaOCKMUtzUG_rBLH2JdSH8GaY8HT7iShWagMPeDhOVZJnVxY0M3u2fMRFXJpAnThHxMuHwZ4ZO9aEvpkdSWC2SgTeu4HYGtAthzEBfh6xB_ajjx5nbeukDx2qHd3P8IwVgiPVY1A4Hao_tP8FH3JmyNh2k4_gBsdDjcQgQAAA_22_7D = __webpack_require__(67109);
;// ./src/app/settings/pipeline-modes/PipelineModeForm.css.ts




var actionRow = '_1q0mfza7';
var cancelBtn = '_1q0mfza9';
var checkboxRow = '_1q0mfza6';
var confirmDeleteBtn = '_1q0mfzab';
var deleteBtn = '_1q0mfzaa';
var errorMessage = '_1q0mfzac';
var fieldGroup = '_1q0mfza1';
var forceOverrideLabel = '_1luu8jt9';
var PipelineModeForm_css_form = '_1q0mfza0';
var hint = '_1q0mfza3';
var input = '_1q0mfza4';
var inputDisabled = '_1q0mfza5';
var label = '_1q0mfza2';
var sectionHeading = '_1luu8jt3';
var stageExecutorCell = '_1luu8jt7';
var stageExecutorHeaderCell = '_1luu8jt5';
var stageExecutorRoleCell = '_1luu8jt6';
var stageExecutorSection = '_1luu8jt2';
var stageExecutorTable = '_1luu8jt4';
var stageFieldError = '_1luu8jt8 _1q0mfzac';
var submitBtn = '_1q0mfza8';
var templateFieldsGrid = '_1luu8jt1';
var PipelineModeForm_css_textarea = '_1luu8jt0 _1q0mfza4';
var visuallyHidden = '_9ji8bp0';
;// ./src/app/settings/pipeline-modes/PipelineModeForm.tsx
/* __next_internal_client_entry_do_not_use__ PipelineModeForm auto */ 







const STAGE_ROLES = [
    {
        role: "triage",
        label: "Triage"
    },
    {
        role: "review",
        label: "Review"
    },
    {
        role: "work",
        label: "Work"
    }
];
function emptyStageExecutors(mode) {
    const values = {};
    for (const { role } of STAGE_ROLES){
        var _mode_stageExecutors;
        const existing = mode === null || mode === void 0 ? void 0 : (_mode_stageExecutors = mode.stageExecutors) === null || _mode_stageExecutors === void 0 ? void 0 : _mode_stageExecutors[role];
        var _existing_program, _existing_model;
        values[role] = {
            program: (_existing_program = existing === null || existing === void 0 ? void 0 : existing.program) !== null && _existing_program !== void 0 ? _existing_program : "",
            model: (_existing_model = existing === null || existing === void 0 ? void 0 : existing.model) !== null && _existing_model !== void 0 ? _existing_model : ""
        };
    }
    return values;
}
// Static list, not useAvailablePrograms()'s host-detected one: a pipeline
// mode's stage executor can target a different machine than the browser's,
// so "installed here" isn't the right filter (per plan.md Task 5.1.1b).
const STAGE_PROGRAM_SUGGESTIONS = programs/* PROGRAMS */.i.filter((p)=>p.value).map((p)=>p.value);
const STAGE_MODEL_SUGGESTIONS = programs/* MODEL_AUTOCOMPLETE_OPTIONS */.kg.map((m)=>m.value);
/**
 * Finds which stage row a CodeInvalidArgument message refers to. The backend
 * always names the offending role in its error text (Tasks 1.3.1b/c2/e's
 * acceptance criteria), so a case-insensitive substring check is sufficient;
 * returns null when no role is confidently identifiable, so the caller can
 * fall back to the generic top-of-form banner instead of guessing.
 */ function matchStageFromError(message) {
    var _STAGE_ROLES_find;
    const lower = message.toLowerCase();
    var _STAGE_ROLES_find_role;
    return (_STAGE_ROLES_find_role = (_STAGE_ROLES_find = STAGE_ROLES.find((param)=>{
        let { role } = param;
        return lower.includes(role);
    })) === null || _STAGE_ROLES_find === void 0 ? void 0 : _STAGE_ROLES_find.role) !== null && _STAGE_ROLES_find_role !== void 0 ? _STAGE_ROLES_find_role : null;
}
function isForceOverrideEligible(message) {
    return message.toLowerCase().includes("force_unknown_model");
}
// The 9 content-template fields, in the same fixed order the backend uses for
// content_hash computation (see server/services/backlog_service_pipeline_mode.go
// pipelineModeToProto), each labeled with the file/prompt it drives — per
// plan.md Story 3.3.2's acceptance criteria.
const CONTENT_FIELDS = [
    {
        key: "statusCommandTemplate",
        label: "status.md content",
        hint: "Written to .claude/commands/backlog/status.md"
    },
    {
        key: "doneCommandTemplate",
        label: "done-N.md template",
        hint: "Written to .claude/commands/backlog/done-N.md"
    },
    {
        key: "failCommandTemplate",
        label: "fail-N.md template",
        hint: "Written to .claude/commands/backlog/fail-N.md"
    },
    {
        key: "reviewCommandTemplate",
        label: "review.md content",
        hint: "Written to .claude/commands/backlog/review.md"
    },
    {
        key: "shipCommandTemplate",
        label: "ship.md content",
        hint: "Written to .claude/commands/backlog/ship.md"
    },
    {
        key: "helpCommandTemplate",
        label: "help.md content",
        hint: "Written to .claude/commands/backlog/help.md"
    },
    {
        key: "triagePromptTemplate",
        label: "Triage prompt",
        hint: "Headless triage LLM call prompt"
    },
    {
        key: "reviewPromptTemplate",
        label: "Review prompt",
        hint: "Headless review-gate LLM call prompt"
    },
    {
        key: "initialPromptTemplate",
        label: "Initial prompt",
        hint: "Session's opening/interactive prompt"
    }
];
function emptyContentFields(mode) {
    const values = {};
    for (const f of CONTENT_FIELDS){
        var _mode_f_key;
        values[f.key] = (_mode_f_key = mode === null || mode === void 0 ? void 0 : mode[f.key]) !== null && _mode_f_key !== void 0 ? _mode_f_key : "";
    }
    return values;
}
/** Extracts a human-readable message from a create/update failure, preferring the ConnectError message (e.g. Story 2.3.1's CodeInvalidArgument text) over a generic fallback. */ function PipelineModeForm_errorMessage(err) {
    if (err instanceof connect_error/* ConnectError */.T) return err.message;
    if (err instanceof Error) return err.message;
    return String(err);
}
/**
 * Classifies a create/update failure for display: a save-time CodeInvalidArgument
 * naming a specific stage (Story 1.3.1) becomes a StageFieldError rendered next to
 * that row (Task 5.1.1g); everything else falls back to the generic top-of-form
 * banner `PipelineModeForm` already used pre-Epic-5.1 (Story 3.3.2).
 */ function classifySaveError(err) {
    const message = PipelineModeForm_errorMessage(err);
    const role = err instanceof connect_error/* ConnectError */.T && err.code === code/* Code */.C.InvalidArgument ? matchStageFromError(message) : null;
    if (role) {
        return {
            bannerMessage: null,
            stageError: {
                role,
                message,
                offersForceOverride: isForceOverrideEligible(message)
            }
        };
    }
    return {
        bannerMessage: message,
        stageError: null
    };
}
/** One stage's Program/Model cells, plus its inline save-error row when `fieldError` names this role (Tasks 5.1.1g/h). */ function StageExecutorRow(param) {
    let { role, stageLabel, value, onFieldChange, fieldError, forceUnknownModel, onForceUnknownModelChange } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(react.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                        className: stageExecutorRoleCell,
                        scope: "row",
                        children: stageLabel
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("td", {
                        className: stageExecutorCell,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: visuallyHidden,
                                htmlFor: "pipeline-mode-stage-".concat(role, "-program"),
                                children: "".concat(stageLabel, " stage program")
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(AutocompleteInput/* AutocompleteInput */.P, {
                                id: "pipeline-mode-stage-".concat(role, "-program"),
                                value: value.program,
                                onChange: (v)=>onFieldChange(role, "program", v),
                                placeholder: "System default",
                                suggestions: STAGE_PROGRAM_SUGGESTIONS,
                                getLabel: programs/* getProgramDisplay */.pw,
                                className: input,
                                "data-testid": "pipeline-mode-stage-".concat(role, "-program")
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("td", {
                        className: stageExecutorCell,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: visuallyHidden,
                                htmlFor: "pipeline-mode-stage-".concat(role, "-model"),
                                children: "".concat(stageLabel, " stage model")
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(AutocompleteInput/* AutocompleteInput */.P, {
                                id: "pipeline-mode-stage-".concat(role, "-model"),
                                value: value.model,
                                onChange: (v)=>onFieldChange(role, "model", v),
                                placeholder: "System default",
                                suggestions: STAGE_MODEL_SUGGESTIONS,
                                filterFn: modelFuzzyMatch/* fuzzyMatchModels */.C,
                                getLabel: modelFuzzyMatch/* getModelLabel */.i,
                                className: input,
                                "data-testid": "pipeline-mode-stage-".concat(role, "-model")
                            })
                        ]
                    })
                ]
            }),
            (fieldError === null || fieldError === void 0 ? void 0 : fieldError.role) === role && /*#__PURE__*/ (0,jsx_runtime.jsx)("tr", {
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                    className: stageExecutorCell,
                    colSpan: 3,
                    children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: stageFieldError,
                        role: "alert",
                        "data-testid": "pipeline-mode-stage-".concat(role, "-error"),
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                children: fieldError.message
                            }),
                            fieldError.offersForceOverride && /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: forceOverrideLabel,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        type: "checkbox",
                                        checked: forceUnknownModel,
                                        onChange: (e)=>onForceUnknownModelChange(e.target.checked),
                                        "data-testid": "pipeline-mode-force-unknown-model"
                                    }),
                                    "I know this model isn't in the pricing table yet — save anyway"
                                ]
                            })
                        ]
                    })
                })
            })
        ]
    });
}
/** The 3-row (Triage/Review/Work) × {Program, Model} table from Task 5.1.1b. Extracted from PipelineModeForm's render to keep that function's body a reasonable size. */ function StageExecutorTable(param) {
    let { values, onFieldChange, fieldError, forceUnknownModel, onForceUnknownModelChange } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: stageExecutorSection,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                className: sectionHeading,
                children: "Stage execution"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: hint,
                children: "Different models per stage means no prompt cache carries over between stages."
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("table", {
                className: stageExecutorTable,
                "data-testid": "pipeline-mode-stage-executor-table",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("thead", {
                        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                    className: stageExecutorHeaderCell,
                                    scope: "col",
                                    children: "Stage"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                    className: stageExecutorHeaderCell,
                                    scope: "col",
                                    children: "Program"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                    className: stageExecutorHeaderCell,
                                    scope: "col",
                                    children: "Model"
                                })
                            ]
                        })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("tbody", {
                        children: STAGE_ROLES.map((param)=>{
                            let { role, label: stageLabel } = param;
                            return /*#__PURE__*/ (0,jsx_runtime.jsx)(StageExecutorRow, {
                                role: role,
                                stageLabel: stageLabel,
                                value: values[role],
                                onFieldChange: onFieldChange,
                                fieldError: fieldError,
                                forceUnknownModel: forceUnknownModel,
                                onForceUnknownModelChange: onForceUnknownModelChange
                            }, role);
                        })
                    })
                ]
            })
        ]
    });
}
/**
 * Create/edit form for a PipelineMode: slug (immutable on edit), name,
 * description, enabled toggle, and the 9 labeled content-template textareas.
 * Also owns the delete-with-confirm action when editing an existing mode.
 */ function PipelineModeForm(param) {
    let { mode, onSaved, onDeleted, onCancel } = param;
    const { createPipelineMode, updatePipelineMode, deletePipelineMode } = (0,useBacklogService/* useBacklogService */.gG)();
    var _mode_slug;
    const [slug, setSlug] = (0,react.useState)((_mode_slug = mode === null || mode === void 0 ? void 0 : mode.slug) !== null && _mode_slug !== void 0 ? _mode_slug : "");
    var _mode_name;
    const [name, setName] = (0,react.useState)((_mode_name = mode === null || mode === void 0 ? void 0 : mode.name) !== null && _mode_name !== void 0 ? _mode_name : "");
    var _mode_description;
    const [description, setDescription] = (0,react.useState)((_mode_description = mode === null || mode === void 0 ? void 0 : mode.description) !== null && _mode_description !== void 0 ? _mode_description : "");
    var _mode_enabled;
    const [enabled, setEnabled] = (0,react.useState)((_mode_enabled = mode === null || mode === void 0 ? void 0 : mode.enabled) !== null && _mode_enabled !== void 0 ? _mode_enabled : true);
    const [contentFields, setContentFields] = (0,react.useState)(()=>emptyContentFields(mode));
    const [stageExecutors, setStageExecutors] = (0,react.useState)(()=>emptyStageExecutors(mode));
    const [forceUnknownModel, setForceUnknownModel] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    const [stageFieldError, setStageFieldError] = (0,react.useState)(null);
    const [submitting, setSubmitting] = (0,react.useState)(false);
    const [confirmingDelete, setConfirmingDelete] = (0,react.useState)(false);
    const [deleting, setDeleting] = (0,react.useState)(false);
    const setField = (0,react.useCallback)((key, value)=>{
        setContentFields((prev)=>({
                ...prev,
                [key]: value
            }));
    }, []);
    const setStageExecutorField = (0,react.useCallback)((role, field, value)=>{
        setStageExecutors((prev)=>({
                ...prev,
                [role]: {
                    ...prev[role],
                    [field]: value
                }
            }));
    }, []);
    const canSubmit = Boolean(slug.trim()) && Boolean(name.trim()) && !submitting;
    const handleSubmit = (0,react.useCallback)(async (e)=>{
        e.preventDefault();
        if (!canSubmit) return;
        setError(null);
        setStageFieldError(null);
        setSubmitting(true);
        try {
            let saved;
            if (mode) {
                saved = await updatePipelineMode(mode.id, {
                    name: name.trim(),
                    description: description.trim(),
                    enabled,
                    ...contentFields,
                    stageExecutors,
                    forceUnknownModel
                });
            } else {
                saved = await createPipelineMode({
                    slug: slug.trim(),
                    name: name.trim(),
                    description: description.trim(),
                    enabled,
                    ...contentFields,
                    stageExecutors,
                    forceUnknownModel
                });
            }
            onSaved(saved);
            setForceUnknownModel(false);
        } catch (err) {
            // Story 3.3.2 acceptance criteria: on CodeInvalidArgument (or any
            // other failure) the error is displayed inline — no navigation, no
            // list refresh, and the form's in-progress edits are preserved.
            const { bannerMessage, stageError } = classifySaveError(err);
            setError(bannerMessage);
            setStageFieldError(stageError);
        } finally{
            setSubmitting(false);
        }
    }, [
        canSubmit,
        mode,
        name,
        description,
        enabled,
        contentFields,
        stageExecutors,
        forceUnknownModel,
        slug,
        createPipelineMode,
        updatePipelineMode,
        onSaved
    ]);
    const handleDeleteClick = (0,react.useCallback)(()=>{
        setError(null);
        setConfirmingDelete(true);
    }, []);
    const handleDeleteCancel = (0,react.useCallback)(()=>{
        setConfirmingDelete(false);
    }, []);
    const handleDeleteConfirm = (0,react.useCallback)(async ()=>{
        if (!mode) return;
        setDeleting(true);
        setError(null);
        try {
            await deletePipelineMode(mode.id);
            onDeleted(mode.id);
        } catch (err) {
            setError(PipelineModeForm_errorMessage(err));
            setConfirmingDelete(false);
        } finally{
            setDeleting(false);
        }
    }, [
        mode,
        deletePipelineMode,
        onDeleted
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("form", {
        className: PipelineModeForm_css_form,
        onSubmit: handleSubmit,
        "data-testid": "pipeline-mode-form",
        children: [
            error && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: errorMessage,
                role: "alert",
                "data-testid": "pipeline-mode-error",
                children: error
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: fieldGroup,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                        className: label,
                        htmlFor: "pipeline-mode-slug",
                        children: "Slug"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        id: "pipeline-mode-slug",
                        type: "text",
                        className: mode ? [
                            input,
                            inputDisabled
                        ].join(" ") : input,
                        value: slug,
                        onChange: (e)=>setSlug(e.target.value),
                        disabled: Boolean(mode),
                        placeholder: "quick",
                        "data-testid": "pipeline-mode-slug"
                    }),
                    mode && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: hint,
                        children: "Slugs are immutable after creation."
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: fieldGroup,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                        className: label,
                        htmlFor: "pipeline-mode-name",
                        children: "Name"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        id: "pipeline-mode-name",
                        type: "text",
                        className: input,
                        value: name,
                        onChange: (e)=>setName(e.target.value),
                        placeholder: "Quick Fix",
                        "data-testid": "pipeline-mode-name"
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: fieldGroup,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                        className: label,
                        htmlFor: "pipeline-mode-description",
                        children: "Description"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("textarea", {
                        id: "pipeline-mode-description",
                        className: input,
                        value: description,
                        onChange: (e)=>setDescription(e.target.value),
                        placeholder: "Fast, low-ceremony pipeline for small fixes",
                        "data-testid": "pipeline-mode-description"
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: checkboxRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        id: "pipeline-mode-enabled",
                        type: "checkbox",
                        checked: enabled,
                        onChange: (e)=>setEnabled(e.target.checked),
                        "data-testid": "pipeline-mode-enabled"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                        className: label,
                        htmlFor: "pipeline-mode-enabled",
                        children: "Enabled"
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(StageExecutorTable, {
                values: stageExecutors,
                onFieldChange: setStageExecutorField,
                fieldError: stageFieldError,
                forceUnknownModel: forceUnknownModel,
                onForceUnknownModelChange: setForceUnknownModel
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: templateFieldsGrid,
                children: CONTENT_FIELDS.map((f)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: fieldGroup,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: label,
                                htmlFor: "pipeline-mode-field-".concat(f.key),
                                children: f.label
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: hint,
                                children: f.hint
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("textarea", {
                                id: "pipeline-mode-field-".concat(f.key),
                                className: PipelineModeForm_css_textarea,
                                value: contentFields[f.key],
                                onChange: (e)=>setField(f.key, e.target.value),
                                "data-testid": "pipeline-mode-field-".concat(f.key)
                            })
                        ]
                    }, f.key))
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: actionRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "submit",
                        className: submitBtn,
                        disabled: !canSubmit,
                        "data-testid": "pipeline-mode-submit",
                        children: submitting ? "Saving…" : mode ? "Save changes" : "Create mode"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: cancelBtn,
                        onClick: onCancel,
                        "data-testid": "pipeline-mode-cancel",
                        children: "Cancel"
                    }),
                    mode && (confirmingDelete ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: confirmDeleteBtn,
                                onClick: handleDeleteConfirm,
                                disabled: deleting,
                                "aria-label": "Confirm delete pipeline mode ".concat(mode.slug),
                                "data-testid": "pipeline-mode-confirm-delete",
                                children: deleting ? "Deleting…" : "Confirm delete?"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: cancelBtn,
                                onClick: handleDeleteCancel,
                                disabled: deleting,
                                "data-testid": "pipeline-mode-cancel-delete",
                                children: "Never mind"
                            })
                        ]
                    }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: deleteBtn,
                        onClick: handleDeleteClick,
                        "aria-label": "Delete pipeline mode ".concat(mode.slug),
                        "data-testid": "pipeline-mode-delete",
                        children: "Delete"
                    }))
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2FsettingsListPage.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA71W25LaMAx95yv8WB6SSUKBxf2YHROLRItjZ2wHSDr9904MlFzIhe62T5mRoyPp-Eiy_x5akNUJA_JzQUjGLt4ZuU0p2UZBfvmxIISjyQUrKTkIcIb663HUEFtUkpJYiSKT9UnCckpOTH_zvPdwB1Wkq1W0rE9yxjnK5Onpr4V_zyJ0WfQiMoGJ9NBCZiiJQVrQtfmjMBYPpRcraUFaSkzOYvD2YM8AgwkF7ZCRC3lQ0noGK-j9zl3-7vwMmKSWkk0Q1LZYCaW7_wfu94zpBCUlQSvUaiLUfjkIG43AfnewAwxH1fIZAYTsleagPc04FqaXyuZR9ViqhTZ1rrnC-51cYSkJ8wsxSiDveGbX4Cw-JloVkneRs2EO1v2bWAcdKmiqTqAdIcMxZFsB6-eie0nmUdXG3Lwk5CGZDvdNMEHjdjnnMmCmEhqFbV1hKmcx2pKSwN-0jt--gsty-ccn0epMSejEj_I-m9qXvvss193LY7P69KWR0EDfP9APLENR9tIpJpqPjc-JRqx4ohI2JrKo7I4OR9SUYHYT0oTZ2fPPzcsGEjyXyJAWbuIzqUZ57Mrt4LBuSoz8tYas9khvSgj96G6axdOtRaWSMDRSJ6jMlcFrW2kQzOLJAVnN5N3-QCCBH5mJApOJ8Zm1uU1H18_faeifrR_oidPxlDMN0s7t4HTWlnnrOnE0bC-AdyfoulmLVNZjQqgz8JY7Oq-Rt8GscWVsKYAStExg3ML_GMYPo4mODsNZ2yac__B49TEzXnyjzOMX7KdVOLqaw_-7mn8D18pB-MILAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2FsettingsListPage_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA71W25LaMAx95yv8WB6SSUKBxf2YHROLRItjZ2wHSDr9904MlFzIhe62T5mRoyPp_Eiy_x5akNUJA_JzQUjGLt4ZuU0p2UZBfvmxIISjyQUrKTkIcIb663HUEFtUkpJYiSKT9UnCckpOTH_zvPdwB1Wkq1W0rE9yxjnK5Onpr4V_zyJ0WfQiMoGJ9NBCZiiJQVrQtfmjMBYPpRcraUFaSkzOYvD2YM8AgwkF7ZCRC3lQ0noGK_j9zl3_7vwMmKSWkk0Q1LZYCaW7_wfu94zpBCUlQSvUaiLUfjkIG43AfnewAwxH1fIZAYTsleagPc04FqaXyuZR9ViqhTZ1rrnC_51cYSkJ8wsxSiDveGbX4Cw_JloVkneRs2EO1v2bWAcdKmiqTqAdIcMxZFsB6_eie0nmUdXG3Lwk5CGZDvdNMEHjdjnnMmCmEhqFbV1hKmcx2pKSwN_0jt_gsty_ccn0epMSejEj_I_m9qXvvss193LY7P69KWR0EDfP9APLENR9tIpJpqPjc_JRqx4ohI2JrKo7I4OR9SUYHYT0oTZ2fPPzcsGEjyXyJAWbuIzqUZ57Mrt4LBuSoz8tYas9khvSgj96G6axdOtRaWSMDRSJ6jMlcFrW2kQzOLJAVnN5N3_QCCBH5mJApOJ8Zm1uU1H18_faeifrR_oidPxlDMN0s7t4HTWlnnrOnE0bC_AdyfoulmLVNZjQqgz8JY7Oq_Rt8GscWVsKYAStExg3ML_GMYPo4mODsNZ2yac_B49TEzXnyjzOMX7KdVOLqaw_7mn8D18pB_MILAAA_22_7D = __webpack_require__(27080);
;// ./src/app/settings/pipeline-modes/page.css.ts


var page_css_actionRow = '_1tenzvie';
var badge = '_1tenzvic';
var container = '_1tenzvi0';
var description = '_1tenzvi3';
var empty = '_1tenzvii';
var page_css_errorMessage = '_1tenzvij';
var formOverlay = '_1tenzvik';
var headerRow = '_1tenzvi1';
var list = '_1tenzvi5';
var listItem = '_1tenzvi6';
var listItemDisabled = '_1tenzvi7';
var listItemInfo = '_1tenzvi8';
var listItemMeta = '_1tenzvid';
var listItemName = '_1tenzvia';
var listItemNameRow = '_1tenzvi9';
var listItemSlug = '_1tenzvib';
var newBtn = '_1tenzvi4';
var smallBtn = '_1tenzvih';
var title = '_1tenzvi2';
var toggle = '_1tenzvif';
var toggleOn = '_1tenzvig';
;// ./src/app/settings/pipeline-modes/page.tsx
// analytics-exempt
/* __next_internal_client_entry_do_not_use__ default auto */ 
// +feature: settings-pipeline-modes
//
// PageViewTracker (rendered below) calls usePageView() internally, so this
// page is exempt from analytics/require-page-analytics's literal-call check
// — same pattern as backlog/board/page.tsx and workflows/page.tsx.





// Note: `export const metadata` only applies to server components — this page
// is a client component (it needs useState/useEffect for the list + form
// toggle, mirroring BacklogItemForm's direct useBacklogService() usage), so
// metadata is intentionally omitted here rather than silently ignored. The
// document <title> is set by the nearest server-rendered layout instead.
/**
 * Management page for pipeline modes: lists all modes (enabled AND disabled —
 * this is the operator-facing management view, unlike the item-form selector
 * which only offers enabled modes), with quick enable/disable + create/edit/
 * delete actions. Mirrors BacklogSourcesSettings.tsx's structure.
 */ function PipelineModesPage() {
    const { listPipelineModes, updatePipelineMode } = (0,useBacklogService/* useBacklogService */.gG)();
    const [modes, setModes] = (0,react.useState)([]);
    const [loading, setLoading] = (0,react.useState)(true);
    const [loadError, setLoadError] = (0,react.useState)(null);
    const [togglingId, setTogglingId] = (0,react.useState)(null);
    // null = form hidden; undefined-as-"create" is represented by the sentinel
    // object below rather than `undefined`, so "creating" and "hidden" are
    // unambiguous states.
    const [editingMode, setEditingMode] = (0,react.useState)(null);
    const [creating, setCreating] = (0,react.useState)(false);
    const refresh = (0,react.useCallback)(async ()=>{
        setLoading(true);
        setLoadError(null);
        try {
            const list = await listPipelineModes();
            setModes(list);
        } catch (err) {
            setLoadError(err instanceof Error ? err.message : String(err));
        } finally{
            setLoading(false);
        }
    }, [
        listPipelineModes
    ]);
    (0,react.useEffect)(()=>{
        refresh();
    }, [
        refresh
    ]);
    const handleNewMode = (0,react.useCallback)(()=>{
        setEditingMode(null);
        setCreating(true);
    }, []);
    const handleEditMode = (0,react.useCallback)((mode)=>{
        setCreating(false);
        setEditingMode(mode);
    }, []);
    const closeForm = (0,react.useCallback)(()=>{
        setCreating(false);
        setEditingMode(null);
    }, []);
    // Story 3.3.2 acceptance criteria: on success the new/updated mode appears
    // in the list without a page reload — update local state directly rather
    // than refetching.
    const handleSaved = (0,react.useCallback)((saved)=>{
        setModes((prev)=>{
            const idx = prev.findIndex((m)=>m.id === saved.id);
            if (idx === -1) return [
                ...prev,
                saved
            ];
            const next = [
                ...prev
            ];
            next[idx] = saved;
            return next;
        });
        setCreating(false);
        setEditingMode(null);
    }, []);
    const handleDeleted = (0,react.useCallback)((id)=>{
        setModes((prev)=>prev.filter((m)=>m.id !== id));
        setCreating(false);
        setEditingMode(null);
    }, []);
    const handleToggleEnabled = (0,react.useCallback)(async (m)=>{
        setTogglingId(m.id);
        try {
            const updated = await updatePipelineMode(m.id, {
                enabled: !m.enabled
            });
            setModes((prev)=>prev.map((x)=>x.id === updated.id ? updated : x));
        } catch (err) {
            setLoadError(err instanceof Error ? err.message : String(err));
        } finally{
            setTogglingId(null);
        }
    }, [
        updatePipelineMode
    ]);
    const formOpen = creating || editingMode !== null;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)(PageViewTracker.PageViewTracker, {}),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: container,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: headerRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h1", {
                                        className: title,
                                        children: "Pipeline Modes"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                        className: description,
                                        children: "Named, runtime-definable pipeline definitions — which slash-commands and prompt content a backlog item's triage/work/review pipeline uses."
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: newBtn,
                                onClick: handleNewMode,
                                "data-testid": "pipeline-mode-new",
                                children: "New Mode"
                            })
                        ]
                    }),
                    loadError && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: page_css_errorMessage,
                        role: "alert",
                        children: loadError
                    }),
                    formOpen && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: formOverlay,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(PipelineModeForm, {
                            mode: editingMode,
                            onSaved: handleSaved,
                            onDeleted: handleDeleted,
                            onCancel: closeForm
                        })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: list,
                        children: loading ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: empty,
                            children: "Loading…"
                        }) : modes.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: empty,
                            children: "No pipeline modes defined yet."
                        }) : modes.map((m)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: m.enabled ? listItem : [
                                    listItem,
                                    listItemDisabled
                                ].join(" "),
                                "data-testid": "pipeline-mode-row-".concat(m.slug),
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: listItemInfo,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: listItemNameRow,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: listItemName,
                                                        children: m.name
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: listItemSlug,
                                                        children: m.slug
                                                    }),
                                                    !m.enabled && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: badge,
                                                        children: "disabled"
                                                    })
                                                ]
                                            }),
                                            m.description && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: listItemMeta,
                                                children: m.description
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: page_css_actionRow,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                role: "switch",
                                                "aria-checked": m.enabled,
                                                className: m.enabled ? [
                                                    toggle,
                                                    toggleOn
                                                ].join(" ") : toggle,
                                                onClick: ()=>handleToggleEnabled(m),
                                                disabled: togglingId === m.id,
                                                "aria-label": "".concat(m.enabled ? "Disable" : "Enable", " ").concat(m.name),
                                                "data-testid": "pipeline-mode-toggle-".concat(m.slug)
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                className: smallBtn,
                                                onClick: ()=>handleEditMode(m),
                                                "data-testid": "pipeline-mode-edit-".concat(m.slug),
                                                children: "Edit"
                                            })
                                        ]
                                    })
                                ]
                            }, m.id))
                    })
                ]
            })
        ]
    });
}


/***/ })

},
/******/ __webpack_require__ => { // webpackRuntimeModules
/******/ var __webpack_exec__ = (moduleId) => (__webpack_require__(__webpack_require__.s = moduleId))
/******/ __webpack_require__.O(0, [1869,852,4202,9353,6000,8165,8890,5909,9691,3516,7358], () => (__webpack_exec__(83315)));
/******/ var __webpack_exports__ = __webpack_require__.O();
/******/ _N_E = __webpack_exports__;
/******/ }
]);
//# sourceMappingURL=page-481ff8a19afa0a35.js.map