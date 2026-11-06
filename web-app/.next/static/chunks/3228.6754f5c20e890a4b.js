"use strict";
(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[3228],{

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

/***/ 23275:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   K: () => (/* binding */ usePublishDeckTopOffset),
/* harmony export */   t: () => (/* binding */ usePublishStackTopOffset)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _lib_utils_toastDock__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(86130);
/* __next_internal_client_entry_do_not_use__ usePublishStackTopOffset,usePublishDeckTopOffset auto */ 

/**
 * Publishes `getBoundingClientRect().bottom` of the session tab row.
 *
 * The row's own size rarely changes when the memory banner above it appears,
 * wraps or is dismissed; its position does. So this re-measures on the row's and
 * the body's resize, on DOM insertions and removals, and on window resize,
 * coalescing bursts into one frame.
 */ function usePublishStackTopOffset(ref) {
    let enabled = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : true;
    usePublishBottomEdge(ref, undefined, enabled, _lib_utils_toastDock__WEBPACK_IMPORTED_MODULE_1__/* .setStackTopOffset */ .nu, false);
}
/**
 * Publishes the lowest bottom edge of the terminal toolbar and, on a phone, the
 * overflow row under it. A hidden toolbar (display: none, an inactive pane)
 * measures 0 tall and publishes nothing, so the deck falls back to the tab-row
 * anchor instead of jumping to the screen top. Pass `enabled = false` for a
 * terminal that is not the visible one.
 */ function usePublishDeckTopOffset(ref) {
    let enabled = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : true, extraRef = arguments.length > 2 ? arguments[2] : void 0;
    const publish = (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>{
        const owner = Symbol("deck-top-offset");
        return (px)=>(0,_lib_utils_toastDock__WEBPACK_IMPORTED_MODULE_1__/* .setDeckTopOffset */ .Gp)(px, owner);
    }, []);
    usePublishBottomEdge(ref, extraRef, enabled, publish, true);
}
function usePublishBottomEdge(ref, extraRef, enabled, publish, nullWhenHidden) {
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        const el = ref.current;
        if (!enabled || !el) return;
        let frame = 0;
        const measure = ()=>{
            var _extraRef_current;
            frame = 0;
            const rect = el.getBoundingClientRect();
            const extra = extraRef === null || extraRef === void 0 ? void 0 : (_extraRef_current = extraRef.current) === null || _extraRef_current === void 0 ? void 0 : _extraRef_current.getBoundingClientRect();
            const bottom = extra && extra.height > 0 ? Math.max(rect.bottom, extra.bottom) : rect.bottom;
            publish(nullWhenHidden && rect.height === 0 ? null : Math.round(bottom));
        };
        const schedule = ()=>{
            if (frame === 0) frame = requestAnimationFrame(measure);
        };
        const resizeObserver = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(schedule);
        resizeObserver === null || resizeObserver === void 0 ? void 0 : resizeObserver.observe(el);
        resizeObserver === null || resizeObserver === void 0 ? void 0 : resizeObserver.observe(document.body);
        const mutationObserver = new MutationObserver(schedule);
        mutationObserver.observe(document.body, {
            childList: true,
            subtree: true
        });
        window.addEventListener("resize", schedule);
        measure();
        return ()=>{
            if (frame !== 0) cancelAnimationFrame(frame);
            resizeObserver === null || resizeObserver === void 0 ? void 0 : resizeObserver.disconnect();
            mutationObserver.disconnect();
            window.removeEventListener("resize", schedule);
            publish(null);
        };
    }, [
        ref,
        extraRef,
        enabled,
        publish,
        nullWhenHidden
    ]);
}


/***/ }),

/***/ 40084:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  ResyncStaggerQueue: () => (/* binding */ ResyncStaggerQueue),
  SessionDetailView: () => (/* binding */ SessionDetailView),
  useResyncStaggerQueue: () => (/* binding */ useResyncStaggerQueue)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/shortcuts/useShortcut.ts
var useShortcut = __webpack_require__(21777);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/terminal.mjs
var terminal = __webpack_require__(27342);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/git-compare.mjs
var git_compare = __webpack_require__(79130);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/git-branch.mjs
var git_branch = __webpack_require__(46199);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/folder-open.mjs
var folder_open = __webpack_require__(58409);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/scroll-text.mjs
var scroll_text = __webpack_require__(59961);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/info.mjs
var info = __webpack_require__(17510);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/globe.mjs
var globe = __webpack_require__(12859);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/package.mjs
var icons_package = __webpack_require__(97448);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/file-text.mjs
var file_text = __webpack_require__(30294);
// EXTERNAL MODULE: ./src/lib/lazy/index.ts + 1 modules
var lazy = __webpack_require__(27980);
// EXTERNAL MODULE: ./src/gen/session/v1/types_pb.ts
var types_pb = __webpack_require__(81432);
// EXTERNAL MODULE: ./src/components/sessions/DiffViewer.tsx + 2 modules
var DiffViewer = __webpack_require__(81124);
// EXTERNAL MODULE: ./src/lib/contexts/SessionVcsContext.tsx
var SessionVcsContext = __webpack_require__(38590);
// EXTERNAL MODULE: ./src/lib/contexts/AnalyticsContext.tsx
var AnalyticsContext = __webpack_require__(24044);
// EXTERNAL MODULE: ./src/lib/vcs/adapters.ts
var adapters = __webpack_require__(85217);
// EXTERNAL MODULE: ./src/lib/vcs/githubRateLimit.ts
var githubRateLimit = __webpack_require__(7727);
// EXTERNAL MODULE: ./src/components/shared/VcsWidget.tsx + 20 modules
var VcsWidget = __webpack_require__(48704);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FVcsPanel.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA5WUwW7bMAyG73kKXgo0wBTYaZJ1yqXAHmRQJNrhakseJXVJhrz7IDmNE8_F0JMh_xT1_aSol1c8Vqxa9LDT_It3JfyZARQPX6Asioe8AHCd0hSOEsrtDOA8A1j_qxWLda-eZ4s-V5FDDPmuUUcJVYOHFJG-whCjDuSsBO2a2Nqk7JHqfZD56LTulDFkawnlpstb3Rty1bjf4ihBxeDSv53SrzW7aI2EN8WPQvwov-FpyafNPOnaNY7HUjHfDpzLz3LWqkuQPdMAuewOUNzkfcp5r6ZWffzOsUEWrAxFP-Z66pk_9vR8C76aBlcN1VZQwNZL0GgD8kC9HFNfSvvxkWU5H7gllN0BvGvIjMOK-YS95z77ZBPKuy6ss5nK2SA8nVDCMhf4qm-y3iquyYoGqzBcgauZTXe4Orw1FFhZ3ylGG6at6MhJ_J4wJ1ys7lyQ3SNTzqQj-_Src_Re5hsHPcrgQO7TBc4-_lPu656vn2rxz-gDVUehnQ1ow600nq3pjuQWBjwEkU8YEpxnLy0aUvDYMVbIXjCaqNGI1vXjYZ3oJbQa5xn7bg4AlKVWpWBhVYvy_cXZjkQTWfUpy8Xaj9VALdlaVNFexhKVR0FWuBjGsRSwTyW0i6kcZCuyFPDyUP0F9ThgfP4EAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FVcsPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA5WUwW7bMAyG73kKXgo0wBTYaZJ1yqXAHmRQJNrhakseJXVJhrz7IDmNE8_F0JMh_xT1_aSol1c8Vqxa9LDT_It3JfyZARQPX6Asioe8AHCd0hSOEsrtDOA8A1j_qxWLda_eZ4s_V5FDDPmuUUcJVYOHFJG_whCjDuSsBO2a2Nqk7JHqfZD56LTulDFkawnlpstb3Rty1bjf4ihBxeDSv53SrzW7aI2EN8WPQvwov_FpyafNPOnaNY7HUjHfDpzLz3LWqkuQPdMAuewOUNzkfcp5r6ZWffzOsUEWrAxFP_Z66pk_9vR8C76aBlcN1VZQwNZL0GgD8kC9HFNfSvvxkWU5H7gllN0BvGvIjMOK_YS95z77ZBPKuy6ss5nK2SA8nVDCMhf4qm_y3iquyYoGqzBcgauZTXe4Orw1FFhZ3ylGG6at6MhJ_J4wJ1ys7lyQ3SNTzqQj_Src_Re5hsHPcrgQO7TBc4_lPu656vn2rxz_gDVUehnQ1ow600nq3pjuQWBjwEkU8YEpxnLy0aUvDYMVbIXjCaqNGI1vXjYZ3oJbQa5xn7bg4AlKVWpWBhVYvy_cXZjkQTWfUpy8Xaj9VALdlaVNFexhKVR0FWuBjGsRSwTyW0i6kcZCuyFPDyUP0F9ThgfP4EAAA_22_7D = __webpack_require__(83705);
;// ./src/components/sessions/VcsPanel.css.ts


var container = 'bcrqrb0';
var empty = 'bcrqrb7';
var VcsPanel_css_error = 'bcrqrb4';
var errorIcon = 'bcrqrb5';
var retryButton = 'bcrqrb6';
var skeleton = 'bcrqrb2';
var skeletonBar = 'bcrqrb3';
;// ./src/components/sessions/VcsPanel.tsx
/* __next_internal_client_entry_do_not_use__ VcsPanel auto */ 






function VcsPanel(param) {
    let { onNavigateToFile, session, onBrowseFiles } = param;
    var _session_gitWorktree;
    const { status, statusLoading, error, refresh } = (0,SessionVcsContext/* useSessionVcsContext */.l)();
    const { track } = (0,AnalyticsContext/* useAnalytics */.st)();
    const handleRetry = ()=>{
        track({
            name: "toolbar_button_click",
            category: "user_action",
            component: "VcsPanel",
            labels: {
                button: "retry"
            }
        });
        refresh();
    };
    if (statusLoading && !status) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: container,
            children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: skeleton,
                role: "status",
                "aria-label": "Loading VCS status",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: skeletonBar,
                        style: {
                            width: "40%"
                        }
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: skeletonBar,
                        style: {
                            width: "70%"
                        }
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: skeletonBar,
                        style: {
                            width: "90%"
                        }
                    })
                ]
            })
        });
    }
    if (error) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: container,
            children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: VcsPanel_css_error,
                role: "status",
                "aria-live": "polite",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: errorIcon,
                        children: "⚠️"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        children: (0,githubRateLimit/* getGitHubRateLimitMessage */._)(error, error.message)
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: retryButton,
                        onClick: handleRetry,
                        children: "Retry"
                    })
                ]
            })
        });
    }
    if (!status) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: container,
            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: empty,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    children: "No VCS information available"
                })
            })
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: container,
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(VcsWidget/* VcsWidget */.D, {
            data: (0,adapters/* fromSessionVcs */.tS)(status, session),
            mode: "full",
            onNavigateToFile: onNavigateToFile,
            onRefresh: refresh,
            worktreePath: session === null || session === void 0 ? void 0 : (_session_gitWorktree = session.gitWorktree) === null || _session_gitWorktree === void 0 ? void 0 : _session_gitWorktree.worktreePath,
            onBrowseFiles: onBrowseFiles,
            sessionId: session === null || session === void 0 ? void 0 : session.id
        })
    });
}

// EXTERNAL MODULE: ./src/lib/hooks/useFocusTrap.ts
var useFocusTrap = __webpack_require__(63991);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect-web@2.1.1+0149f91553ca585e/node_modules/@connectrpc/connect-web/dist/esm/connect-transport.js + 8 modules
var connect_transport = __webpack_require__(46564);
// EXTERNAL MODULE: ./src/gen/session/v1/session_pb.ts
var session_pb = __webpack_require__(23575);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FWorkspaceSwitchModal.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA9VY3W7jKhC-71Nwc6RGOrbAbX5Kb_Y1ztWK2Dhhi8GLceJkte--MrgJxthOWm2lo1Ztgs38fPPNMMy3N3rKFSloBb6jbPMzZUcIfj0AkCtZmA8AyJKkTJ8wgK8PAPx-AEBL_xGyj34_fBsKRL5ArYiocqkKbD9youl_jxGCZbN4nVMZ3gz9jReD4nczEiOglBXTTAoMctbQrH2LiYrqTtWWpG87JWuRYXAg6jGKvqMXek7UmRgNGatKTk4Y5Jw27QLhbCcipmlRYZBSoalql3_UlWb5KUql0FRo99E5YiKjDQYIQqOTCFYQa9M1BjBGywpQUtFI1vrVdeTJODJu6cpYupUqoypSJGN1hQFKysYuN1G1J5k8YgBBAssGPLd_1G5LHuG_oPuNn4yQgjTRkWV6j8EKQiuh-_4C_3l_Y0_Zbq8x2MDDvl2TB6py3mrYsyyjIghc-z_KmKKpdT2VvC7ECB5oCo9ng8dAwSAEVUlSGm2pPlJr00jsSpJlTOwwQIoWAMVLRQsH0a3UWhYYoLIBleQs8-Cni551S8t-KXRUsTPFAMUoeRdplo8dfCtLh1RyqfyYwrvYtyMlBrAz3LFlNbQlGby0HtBLSEGvAFy_p7WqWlNLyQbYwfjiZNAhFCLps2WYyesuSwnnTuz7huJ9y7SZbNgsJkF15G0sOtzkpsvj6IQBqbXsk2MI78uMKeuQzxvrc490NrHUjokr24bqiB9NGK_vJpaNg6aNjpzKWpclVakBHABOtaYqatOnCy1cBm0MMG57tTEnBeMnX39SL159LzbrSep4YUuNinm4siFcm_vxgoug4xdBn01ROkhR_408XO7G62mn6pKPbslO3qv6kOuOyl1Y5Vz9hPHKKHXx8di_stoDlWSqCFxL0bUIm_dLoqjonw67m6rEus-p_czrqJfKQaIUfYlsRuJhVmDdF_jDpb2y0YRxj88jBHrz69y9yTef0I42PlGlJipS5xinuQ7mSTEUu5o8VidL3Hhcavf4OO6Zph7F0TvFn8Y43p1qcx4JN6BalthpPTp9_nKnyK7e1o7ID1aPgbnlDTXBSK00UdoD7f5yMEz4oZs3dw7lR2rCz2DCrZf989rEAl5aPGe_6qed86T6myfT0BD9sYzkTNDLuYHi557M-mt78FnG3xHXQ9j0jvfrAXxHe4-8ktnkP5ok9G3snbvSTYQ9kDG38GnZ1cbb0uZ4c8PtbGo-Adfl4jGqrAiW5y9Bo7FoYCH1I85YRbacZou5xsFH57LTbLzOPeKl64iQOiKcy2M7sXC2n-5qzCaGEpf4PM0VAkf7-d6zZKoXRv2jDoOkWzAHt9l53eMYgaBfzhI_Y5GdQIVdguP3D1dE8n_OetcROz0KgRq4hE4cB1aRPnGKAdOEs_Qm9XZY082QELRDJB_W2QbhryM7Ui7G7n09D3Eu09qe6bLW7bHpj1DGbwz-gK79ScomdP3o68QlJyndS551BTqo4tnbt7zjVjKOdh2aa6y8ucawVbrc1T9zdfZDhabHTt7lBK1mADjdBEDzFQD0m-mxthytZwcIaBPoN2dwO4-1gH8Ab_LhAkEYAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FWorkspaceSwitchModal_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9VY3W7jKhC_71Nwc6RGOrbAbX5Kb_Y1ztWK2Dhhi8GLceJkte_MrgJxthOWm2lo1Ztgs38fPPNMMy3N3rKFSloBb6jbPMzZUcIfj0AkCtZmA8AyJKkTJ8wgK8PAPx_AEBL_xGyj34_fBsKRL5ArYiocqkKbD9youl_jxGCZbN4nVMZ3gz9jReD4nczEiOglBXTTAoMctbQrH2LiYrqTtWWpG87JWuRYXAg6jGKvqMXek7UmRgNGatKTk4Y5Jw27QLhbCcipmlRYZBSoalql3_UlWb5KUql0FRo99E5YiKjDQYIQqOTCFYQa9M1BjBGywpQUtFI1vrVdeTJODJu6cpYupUqoypSJGN1hQFKysYuN1G1J5k8YgBBAssGPLd_1G5LHuG_oPuNn4yQgjTRkWV6j8EKQiuh_4C_3l_Y0_Zbq8x2MDDvl2TB6py3mrYsyyjIghc_z_KmKKpdT2VvC7ECB5oCo9ng8dAwSAEVUlSGm2pPlJr00jsSpJlTOwwQIoWAMVLRQsH0a3UWhYYoLIBleQs8_Cni551S8t_KXRUsTPFAMUoeRdplo8dfCtLh1RyqfyYwrvYtyMlBrAz3LFlNbQlGby0HtBLSEGvAFy_p7WqWlNLyQbYwfjiZNAhFCLps2WYyesuSwnnTuz7huJ9y7SZbNgsJkF15G0sOtzkpsvj6IQBqbXsk2MI78uMKeuQzxvrc490NrHUjokr24bqiB9NGK_vJpaNg6aNjpzKWpclVakBHABOtaYqatOnCy1cBm0MMG57tTEnBeMnX39SL159LzbrSep4YUuNinm4siFcm_vxgoug4xdBn01ROkhR_408XO7G62mn6pKPbslO3qv6kOuOyl1Y5Vz9hPHKKHXx8di_stoDlWSqCFxL0bUIm_dLoqjonw67m6rEus_p_czrqJfKQaIUfYlsRuJhVmDdF_jDpb2y0YRxj88jBHrz69y9yTef0I42PlGlJipS5xinuQ7mSTEUu5o8VidL3Hhcavf4OO6Zph7F0TvFn8Y43p1qcx4JN6BalthpPTp9_nKnyK7e1o7ID1aPgbnlDTXBSK00UdoD7f5yMEz4oZs3dw7lR2rCz2DCrZf989rEAl5aPGe_6qed86T6myfT0BD9sYzkTNDLuYHi557M_mt78FnG3xHXQ9j0jvfrAXxHe4_8ktnkP5ok9G3snbvSTYQ9kDG38GnZ1cbb0uZ4c8PtbGo_Adfl4jGqrAiW5y9Bo7FoYCH1I85YRbacZou5xsFH57LTbLzOPeKl64iQOiKcy2M7sXC2n_5qzCaGEpf4PM0VAkf7_d6zZKoXRv2jDoOkWzAHt9l53eMYgaBfzhI_Y5GdQIVdguP3D1dE8n_OetcROz0KgRq4hE4cB1aRPnGKAdOEs_Qm9XZY082QELRDJB_W2QbhryM7Ui7G7n09D3Eu09qe6bLW7bHpj1DGbwz_gK79ScomdP3o68QlJyndS551BTqo4tnbt7zjVjKOdh2aa6y8ucawVbrc1T9zdfZDhabHTt7lBK1mADjdBEDzFQD0m_mxthytZwcIaBPoN2dwO4_1gH8Ab_LhAkEYAAA_22_7D = __webpack_require__(78576);
;// ./src/components/sessions/WorkspaceSwitchModal.css.ts


var cancelButton = '_1d8qciww';
var closeButton = '_1d8qciw7';
var content = '_1d8qciw8';
var current = '_1d8qciwi';
var currentBadge = '_1d8qciwm';
var currentInfo = '_1d8qciw9';
var currentInfoTitle = '_1d8qciwa';
var currentInfoValue = '_1d8qciwb';
var emptyState = '_1d8qciw13';
var WorkspaceSwitchModal_css_error = '_1d8qciwz';
var WorkspaceSwitchModal_css_errorIcon = '_1d8qciw10';
var errorMessage = '_1d8qciw11';
var filterInput = '_1d8qciw14';
var footer = '_1d8qciwu';
var footerButtons = '_1d8qciwv';
var header = '_1d8qciw4';
var headerTitle = '_1d8qciw5';
var WorkspaceSwitchModal_css_loading = '_1d8qciwy';
var modal = '_1d8qciw3';
var modalOverlay = '_1d8qciw2';
var WorkspaceSwitchModal_css_retryButton = '_1d8qciw12';
var section = '_1d8qciwc';
var sectionIcon = '_1d8qciwe';
var sectionTitle = '_1d8qciwd';
var selected = '_1d8qciwh';
var strategyContent = '_1d8qciwr';
var strategyDescription = '_1d8qciwt';
var strategyLabel = '_1d8qciws';
var strategyOption = '_1d8qciwp';
var strategyOptions = '_1d8qciwo';
var strategyRadio = '_1d8qciwq';
var strategySection = '_1d8qciwn';
var successBanner = '_1d8qciw15';
var switchButton = '_1d8qciwx';
var targetItem = '_1d8qciwg';
var targetItemIcon = '_1d8qciwj';
var targetItemMeta = '_1d8qciwl';
var targetItemName = '_1d8qciwk';
var targetList = '_1d8qciwf';
var vcsIcon = '_1d8qciw6';
var warningBanner = '_1d8qciw16';
var warningIcon = '_1d8qciw17';
var warningText = '_1d8qciw18';
;// ./src/components/sessions/WorkspaceSwitchModal.tsx
/* __next_internal_client_entry_do_not_use__ WorkspaceSwitchModal auto */ 








function getVcsIcon(type) {
    switch(type){
        case types_pb/* VCSType */.Nz.VCS_TYPE_GIT:
            return "🌿";
        case types_pb/* VCSType */.Nz.VCS_TYPE_JUJUTSU:
            return "🔄";
        default:
            return "📁";
    }
}
function getVcsName(type) {
    switch(type){
        case VCSType.VCS_TYPE_GIT:
            return "Git";
        case VCSType.VCS_TYPE_JUJUTSU:
            return "Jujutsu";
        default:
            return "VCS";
    }
}
function formatTimestamp(timestamp) {
    if (!timestamp) return "";
    const now = new Date();
    const diff = now.getTime() - timestamp.getTime();
    const hours = Math.floor(diff / (1000 * 60 * 60));
    const days = Math.floor(hours / 24);
    if (days > 0) return "".concat(days, "d ago");
    if (hours > 0) return "".concat(hours, "h ago");
    const minutes = Math.floor(diff / (1000 * 60));
    return minutes > 0 ? "".concat(minutes, "m ago") : "just now";
}
function WorkspaceSwitchModal(param) {
    let { sessionId, sessionName, baseUrl, onClose, onSwitched, triggerRef } = param;
    const [targets, setTargets] = (0,react.useState)(null);
    const [vcsInfo, setVcsInfo] = (0,react.useState)(null);
    const [loading, setLoading] = (0,react.useState)(true);
    const [error, setError] = (0,react.useState)(null);
    const [filter, setFilter] = (0,react.useState)("");
    const [selectedTarget, setSelectedTarget] = (0,react.useState)(null);
    const [changeStrategy, setChangeStrategy] = (0,react.useState)(types_pb/* ChangeStrategy */.cB.KEEP_AS_WIP);
    const [isSwitching, setSwitching] = (0,react.useState)(false);
    const [switchError, setSwitchError] = (0,react.useState)(null);
    const [switchSuccess, setSwitchSuccess] = (0,react.useState)(false);
    const modalRef = (0,react.useRef)(null);
    (0,useFocusTrap/* useFocusTrap */.r)(modalRef, true, triggerRef);
    const client = (0,react.useMemo)(()=>(0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,connect_transport/* createConnectTransport */.w)({
            baseUrl
        })), [
        baseUrl
    ]);
    const fetchData = (0,react.useCallback)(async ()=>{
        setLoading(true);
        setError(null);
        try {
            const [infoResponse, targetsResponse] = await Promise.all([
                client.getWorkspaceInfo({
                    id: sessionId
                }),
                client.listWorkspaceTargets({
                    id: sessionId
                })
            ]);
            if (infoResponse.error) {
                setError(infoResponse.error);
                return;
            }
            if (targetsResponse.error) {
                setError(targetsResponse.error);
                return;
            }
            var _infoResponse_vcsInfo;
            setVcsInfo((_infoResponse_vcsInfo = infoResponse.vcsInfo) !== null && _infoResponse_vcsInfo !== void 0 ? _infoResponse_vcsInfo : null);
            var _targetsResponse_targets;
            setTargets((_targetsResponse_targets = targetsResponse.targets) !== null && _targetsResponse_targets !== void 0 ? _targetsResponse_targets : null);
        } catch (err) {
            setError(err instanceof Error ? err.message : "Failed to load workspace data");
            console.error("Error fetching workspace data:", err);
        } finally{
            setLoading(false);
        }
    }, [
        client,
        sessionId
    ]);
    (0,react.useEffect)(()=>{
        fetchData();
    }, [
        fetchData
    ]);
    // Filter targets based on search input
    const filteredBookmarks = (0,react.useMemo)(()=>{
        if (!(targets === null || targets === void 0 ? void 0 : targets.bookmarks)) return [];
        const lowerFilter = filter.toLowerCase();
        return targets.bookmarks.filter((b)=>b.name.toLowerCase().includes(lowerFilter));
    }, [
        targets === null || targets === void 0 ? void 0 : targets.bookmarks,
        filter
    ]);
    const filteredRevisions = (0,react.useMemo)(()=>{
        if (!(targets === null || targets === void 0 ? void 0 : targets.recentRevisions)) return [];
        const lowerFilter = filter.toLowerCase();
        return targets.recentRevisions.filter((r)=>r.shortId.toLowerCase().includes(lowerFilter) || r.description.toLowerCase().includes(lowerFilter) || r.author.toLowerCase().includes(lowerFilter));
    }, [
        targets === null || targets === void 0 ? void 0 : targets.recentRevisions,
        filter
    ]);
    const filteredWorktrees = (0,react.useMemo)(()=>{
        if (!(targets === null || targets === void 0 ? void 0 : targets.worktrees)) return [];
        const lowerFilter = filter.toLowerCase();
        return targets.worktrees.filter((w)=>w.name.toLowerCase().includes(lowerFilter) || w.path.toLowerCase().includes(lowerFilter));
    }, [
        targets === null || targets === void 0 ? void 0 : targets.worktrees,
        filter
    ]);
    const handleSwitch = async ()=>{
        if (!selectedTarget) return;
        setSwitching(true);
        setSwitchError(null);
        try {
            let switchType;
            switch(selectedTarget.type){
                case "bookmark":
                    switchType = types_pb/* WorkspaceSwitchType */.Uk.REVISION;
                    break;
                case "revision":
                    switchType = types_pb/* WorkspaceSwitchType */.Uk.REVISION;
                    break;
                case "worktree":
                    switchType = types_pb/* WorkspaceSwitchType */.Uk.WORKTREE;
                    break;
            }
            const response = await client.switchWorkspace({
                id: sessionId,
                switchType,
                target: selectedTarget.value,
                changeStrategy,
                createIfMissing: false
            });
            if (!response.success) {
                setSwitchError(response.message || "Failed to switch workspace");
                return;
            }
            // Show success message briefly before closing
            setSwitchSuccess(true);
            setTimeout(()=>{
                onSwitched === null || onSwitched === void 0 ? void 0 : onSwitched();
                onClose();
            }, 1500);
        } catch (err) {
            setSwitchError(err instanceof Error ? err.message : "Failed to switch workspace");
            console.error("Error switching workspace:", err);
        } finally{
            setSwitching(false);
        }
    };
    // Handle escape key via React synthetic event on the overlay instead of raw DOM listener
    const handleOverlayKeyDown = (0,react.useCallback)((e)=>{
        if (e.key === "Escape") {
            onClose();
        }
    }, [
        onClose
    ]);
    const renderContent = ()=>{
        if (loading) {
            return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: WorkspaceSwitchModal_css_loading,
                children: "Loading workspace information..."
            });
        }
        if (error) {
            return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: WorkspaceSwitchModal_css_error,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: WorkspaceSwitchModal_css_errorIcon,
                        children: "⚠️"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: errorMessage,
                        children: error
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: WorkspaceSwitchModal_css_retryButton,
                        onClick: fetchData,
                        children: "Retry"
                    })
                ]
            });
        }
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
            children: [
                vcsInfo && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: currentInfo,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: currentInfoTitle,
                            children: "Current Workspace"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: currentInfoValue,
                            children: vcsInfo.currentBookmark || vcsInfo.currentRevision || "(detached)"
                        })
                    ]
                }),
                (vcsInfo === null || vcsInfo === void 0 ? void 0 : vcsInfo.hasUncommittedChanges) && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: warningBanner,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: warningIcon,
                            children: "⚠️"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                            className: warningText,
                            children: [
                                "You have uncommitted changes (",
                                vcsInfo.modifiedFileCount,
                                " files). Choose how to handle them below."
                            ]
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                    type: "text",
                    className: filterInput,
                    placeholder: "Filter branches, revisions, worktrees...",
                    value: filter,
                    onChange: (e)=>setFilter(e.target.value),
                    autoFocus: true
                }),
                filteredBookmarks.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: section,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("h3", {
                            className: sectionTitle,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: sectionIcon,
                                    children: "⎇"
                                }),
                                (targets === null || targets === void 0 ? void 0 : targets.vcsType) === types_pb/* VCSType */.Nz.VCS_TYPE_JUJUTSU ? "Bookmarks" : "Branches"
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: targetList,
                            children: filteredBookmarks.map((bookmark)=>{
                                const isCurrent = bookmark.name === (vcsInfo === null || vcsInfo === void 0 ? void 0 : vcsInfo.currentBookmark);
                                const isSelected = (selectedTarget === null || selectedTarget === void 0 ? void 0 : selectedTarget.type) === "bookmark" && selectedTarget.value === bookmark.name;
                                return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: "".concat(targetItem, " ").concat(isSelected ? selected : "", " ").concat(isCurrent ? current : ""),
                                    onClick: ()=>!isCurrent && setSelectedTarget({
                                            type: "bookmark",
                                            value: bookmark.name
                                        }),
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: targetItemIcon,
                                            children: "⎇"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: targetItemName,
                                            children: bookmark.name
                                        }),
                                        bookmark.revisionId && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: targetItemMeta,
                                            children: bookmark.revisionId.substring(0, 8)
                                        }),
                                        isCurrent && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: currentBadge,
                                            children: "Current"
                                        })
                                    ]
                                }, bookmark.name);
                            })
                        })
                    ]
                }),
                filteredRevisions.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: section,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("h3", {
                            className: sectionTitle,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: sectionIcon,
                                    children: "\uD83D\uDCDD"
                                }),
                                "Recent Revisions"
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: targetList,
                            children: filteredRevisions.map((revision)=>{
                                const isCurrent = revision.isCurrent;
                                const isSelected = (selectedTarget === null || selectedTarget === void 0 ? void 0 : selectedTarget.type) === "revision" && selectedTarget.value === revision.id;
                                return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: "".concat(targetItem, " ").concat(isSelected ? selected : "", " ").concat(isCurrent ? current : ""),
                                    onClick: ()=>!isCurrent && setSelectedTarget({
                                            type: "revision",
                                            value: revision.id
                                        }),
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: targetItemIcon,
                                            children: "\uD83D\uDCDD"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: targetItemName,
                                            children: revision.shortId
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                            className: targetItemMeta,
                                            children: [
                                                revision.description.substring(0, 40),
                                                revision.description.length > 40 ? "..." : ""
                                            ]
                                        }),
                                        isCurrent && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: currentBadge,
                                            children: "Current"
                                        })
                                    ]
                                }, revision.id);
                            })
                        })
                    ]
                }),
                filteredWorktrees.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: section,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("h3", {
                            className: sectionTitle,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: sectionIcon,
                                    children: "\uD83D\uDCC2"
                                }),
                                "Worktrees"
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: targetList,
                            children: filteredWorktrees.map((worktree)=>{
                                const isCurrent = worktree.isCurrent;
                                const isSelected = (selectedTarget === null || selectedTarget === void 0 ? void 0 : selectedTarget.type) === "worktree" && selectedTarget.value === worktree.path;
                                return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: "".concat(targetItem, " ").concat(isSelected ? selected : "", " ").concat(isCurrent ? current : ""),
                                    onClick: ()=>!isCurrent && setSelectedTarget({
                                            type: "worktree",
                                            value: worktree.path
                                        }),
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: targetItemIcon,
                                            children: "\uD83D\uDCC2"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: targetItemName,
                                            children: worktree.name
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: targetItemMeta,
                                            children: worktree.bookmark
                                        }),
                                        isCurrent && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: currentBadge,
                                            children: "Current"
                                        })
                                    ]
                                }, worktree.path);
                            })
                        })
                    ]
                }),
                filteredBookmarks.length === 0 && filteredRevisions.length === 0 && filteredWorktrees.length === 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: emptyState,
                    children: filter ? "No matching targets found" : "No branches, revisions, or worktrees available"
                }),
                (vcsInfo === null || vcsInfo === void 0 ? void 0 : vcsInfo.hasUncommittedChanges) && selectedTarget && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: strategySection,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("h3", {
                            className: sectionTitle,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: sectionIcon,
                                    children: "⚙️"
                                }),
                                "Handle Uncommitted Changes"
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: strategyOptions,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: "".concat(strategyOption, " ").concat(changeStrategy === types_pb/* ChangeStrategy */.cB.KEEP_AS_WIP ? selected : ""),
                                    onClick: ()=>setChangeStrategy(types_pb/* ChangeStrategy */.cB.KEEP_AS_WIP),
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                            type: "radio",
                                            className: strategyRadio,
                                            checked: changeStrategy === types_pb/* ChangeStrategy */.cB.KEEP_AS_WIP,
                                            readOnly: true
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: strategyContent,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                                    className: strategyLabel,
                                                    children: "Keep as WIP"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                                    className: strategyDescription,
                                                    children: "Save changes as a separate WIP revision (JJ) or stash them (Git)"
                                                })
                                            ]
                                        })
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: "".concat(strategyOption, " ").concat(changeStrategy === types_pb/* ChangeStrategy */.cB.BRING_ALONG ? selected : ""),
                                    onClick: ()=>setChangeStrategy(types_pb/* ChangeStrategy */.cB.BRING_ALONG),
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                            type: "radio",
                                            className: strategyRadio,
                                            checked: changeStrategy === types_pb/* ChangeStrategy */.cB.BRING_ALONG,
                                            readOnly: true
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: strategyContent,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                                    className: strategyLabel,
                                                    children: "Bring Along"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                                    className: strategyDescription,
                                                    children: "Keep changes in the new location (JJ) or stash pop after switch (Git)"
                                                })
                                            ]
                                        })
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: "".concat(strategyOption, " ").concat(changeStrategy === types_pb/* ChangeStrategy */.cB.ABANDON ? selected : ""),
                                    onClick: ()=>setChangeStrategy(types_pb/* ChangeStrategy */.cB.ABANDON),
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                            type: "radio",
                                            className: strategyRadio,
                                            checked: changeStrategy === types_pb/* ChangeStrategy */.cB.ABANDON,
                                            readOnly: true
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: strategyContent,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                                    className: strategyLabel,
                                                    children: "Abandon"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                                    className: strategyDescription,
                                                    children: "Discard all uncommitted changes (cannot be undone!)"
                                                })
                                            ]
                                        })
                                    ]
                                })
                            ]
                        })
                    ]
                }),
                switchSuccess && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: successBanner,
                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        children: "✓ Workspace switched — conversation context preserved."
                    })
                }),
                !switchSuccess && switchError && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: WorkspaceSwitchModal_css_error,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: WorkspaceSwitchModal_css_errorIcon,
                            children: "❌"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: errorMessage,
                            children: switchError
                        })
                    ]
                })
            ]
        });
    };
    var _targets_vcsType;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: modalOverlay,
        onClick: onClose,
        onKeyDown: handleOverlayKeyDown,
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: modal,
            onClick: (e)=>e.stopPropagation(),
            role: "dialog",
            "aria-modal": "true",
            "aria-labelledby": "workspace-switch-title",
            ref: modalRef,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: header,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("h2", {
                            className: headerTitle,
                            id: "workspace-switch-title",
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: vcsIcon,
                                    children: getVcsIcon((_targets_vcsType = targets === null || targets === void 0 ? void 0 : targets.vcsType) !== null && _targets_vcsType !== void 0 ? _targets_vcsType : types_pb/* VCSType */.Nz.VCS_TYPE_UNSPECIFIED)
                                }),
                                "Switch Workspace - ",
                                sessionName
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: closeButton,
                            onClick: onClose,
                            children: "✕"
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: content,
                    children: renderContent()
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: footer,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {}),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: footerButtons,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: cancelButton,
                                    onClick: onClose,
                                    children: "Cancel"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: switchButton,
                                    disabled: !selectedTarget || isSwitching,
                                    onClick: handleSwitch,
                                    children: isSwitching ? "Switching..." : "Switch Workspace"
                                })
                            ]
                        })
                    ]
                })
            ]
        })
    });
}

// EXTERNAL MODULE: ./src/components/shared/LogViewer.tsx + 18 modules
var LogViewer = __webpack_require__(42692);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FSessionLogsTab.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61W226jMBB9z1f4ZdVGWkeQNr2439DHfa4MHoIV40G2aS6r_feVTUgdCiHt7gsXMz4zczxnhsXbna2MS8jvGSFC2lrxPSOFgt3LjIQ7FdJA7iRqRnJUTaX9lxLkunSMpEnyw79XUtNuLQlbUTta8EqqPSM3r6h5jjc_yc0raBUefmWNdg15Re1fK9Roa57Daa-VB2AkWTytDFQvsz-zRRtqOhwqV3KtqXRQWUZy0A6MX17z2oM8tiCE1FwIqddnaxkaAYZm6BxWjKT1jlhUUpB3bm4pfUuf4bA0h7SYnzjZGo_rr1FkyxBZ5OHeQEWSxUPnh-ebtcFGC5qjQsP6DsT8I5zxODbBahgBIgRquJCNZeS-3o2x2h7cVgpXMrJMksiyOzupSzDSxXmyAvPGhmyxcUpqYESjhsj3YHhqHqHcjbD1cSwTdD1exRaMs5W0nxpj_bcaZVc0X-HPGa6tbOXRj5gki3Rl45xZie9gQuZT6T3FZN2fk5UuTu5h52io_bjqBwGXMeCqB3iEG66qlqjJ8k2vOpAObZDjipu11JE6TwE_nAe8_Mf8HwOc1zIjqd_iz6VQuGWENw4HWtpp61PYetRM1_8-yl7x2kJoleHpbKMrgYs2EeyKxjqZb_YhF6yPzfO60j9QqcUxgTMnfWWtesqKSVNQuC80wVZOQQrbIzcPSXKR86O_IJQCTcVIU9dgcm4_N_v-qT-3UrkutrY_Twky1uPzd_XIp5rXBTa2pXRAw7DzbbM3RLIp5CHuL0PmU5BbNIJmBviGkXCjfiVCaGt2WlUwblfFdsW43S62W4_bhTZyMizPfwoyhfnm5bNKB6f_l8ZMb8wFyX5j9CzHRs9_GDJlW9RMo7tlQlqeKRDzq2v8wrCMXHS47U9AzXPp9qHVxHkJKHij_K_DX0uDdp1nCgAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FSessionLogsTab_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61W226jMBB9z1f4ZdVGWkeQNr2439DHfa4MHoIV40G2aS6r_feVTUgdCiHt7gsXMz4zczxnhsXbna2MS8jvGSFC2lrxPSOFgt3LjIQ7FdJA7iRqRnJUTaX9lxLkunSMpEnyw79XUtNuLQlbUTta8EqqPSM3r6h5jjc_yc0raBUefmWNdg15Re1fK9Roa57Daa_VB2AkWTytDFQvsz_zRRtqOhwqV3KtqXRQWUZy0A6MX17z2oM8tiCE1FwIqddnaxkaAYZm6BxWjKT1jlhUUpB3bm4pfUuf4bA0h7SYnzjZGo_rr1FkyxBZ5OHeQEWSxUPnh_ebtcFGC5qjQsP6DsT8I5zxODbBahgBIgRquJCNZeS_3o2x2h7cVgpXMrJMksiyOzupSzDSxXmyAvPGhmyxcUpqYESjhsj3YHhqHqHcjbD1cSwTdD1exRaMs5W0nxpj_bcaZVc0X_HPGa6tbOXRj5gki3Rl45xZie9gQuZT6T3FZN2fk5UuTu5h52io_bjqBwGXMeCqB3iEG66qlqjJ8k2vOpAObZDjipu11JE6TwE_nAe8_Mf8HwOc1zIjqd_iz6VQuGWENw4HWtpp61PYetRM1_8_yl7x2kJoleHpbKMrgYs2EeyKxjqZb_YhF6yPzfO60j9QqcUxgTMnfWWtesqKSVNQuC80wVZOQQrbIzcPSXKR86O_IJQCTcVIU9dgcm4_N_v_qT_3UrkutrY_Twky1uPzd_XIp5rXBTa2pXRAw7DzbbM3RLIp5CHuL0PmU5BbNIJmBviGkXCjfiVCaGt2WlUwblfFdsW43S62W4_bhTZyMizPfwoyhfnm5bNKB6f_l8ZMb8wFyX5j9CzHRs9_GDJlW9RMo7tlQlqeKRDzq2v8wrCMXHS47U9AzXPp9qHVxHkJKHij_K_DX0uDdp1nCgAA_22_7D = __webpack_require__(56900);
;// ./src/components/sessions/SessionLogsTab.css.ts


var SessionLogsTab_css_container = '_3smrt0';
var SessionLogsTab_css_empty = '_3smrt6';
var level = '_3smrtb';
var levelDebug = '_3smrtd';
var levelError = '_3smrtg';
var levelInfo = '_3smrte';
var levelWarning = '_3smrtf';
var loadMoreButton = '_3smrth';
var message = '_3smrtc';
var refreshButton = '_3smrt3';
var row = '_3smrt9';
var searchInput = '_3smrt2';
var SessionLogsTab_css_status = '_3smrt4';
var statusError = '_3smrt5';
var table = '_3smrt8';
var tableWrapper = '_3smrt7';
var timestamp = '_3smrta';
var toolbar = '_3smrt1';
;// ./src/components/sessions/SessionLogsTab.tsx
/* __next_internal_client_entry_do_not_use__ SessionLogsTab auto */ 


function SessionLogsTab(param) {
    let { sessionId } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: SessionLogsTab_css_container,
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            style: {
                flex: 1,
                minHeight: 0,
                display: "flex",
                flexDirection: "column",
                height: "100%"
            },
            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(LogViewer/* LogViewer */.r, {
                source: "session",
                sessionId: sessionId
            })
        })
    });
}

// EXTERNAL MODULE: ./src/lib/contexts/FeatureFlagsContext.tsx
var FeatureFlagsContext = __webpack_require__(63367);
;// ./src/lib/hooks/useFilesFlags.ts
/* __next_internal_client_entry_do_not_use__ useFilesFlags auto */ 

// Server defaults (server/services/feature_flag_service.go); editing and the ADR-036 flags fail closed.
const DEFAULTS = {
    editing: false,
    viewerV2: true,
    mergePhone: true,
    rangeRead: false,
    deltaWrite: false,
    editLarge: false
};
const REFRESH_THROTTLE_MS = 5000;
function useFilesFlags() {
    const { flags, isLoading, error, flagList, refresh } = (0,FeatureFlagsContext/* useFeatureFlags */.h)();
    const status = isLoading ? "loading" : error && flagList.length === 0 ? "error" : "ready";
    const lastRefreshAt = (0,react.useRef)(0);
    const latchedViewerV2 = (0,react.useRef)(null);
    const refreshNow = (0,react.useCallback)(async ()=>{
        lastRefreshAt.current = Date.now();
        await (refresh === null || refresh === void 0 ? void 0 : refresh());
    }, [
        refresh
    ]);
    (0,react.useEffect)(()=>{
        const throttled = ()=>{
            if (document.visibilityState === "hidden") return;
            if (Date.now() - lastRefreshAt.current >= REFRESH_THROTTLE_MS) void refreshNow();
        };
        window.addEventListener("focus", throttled);
        document.addEventListener("visibilitychange", throttled);
        return ()=>{
            window.removeEventListener("focus", throttled);
            document.removeEventListener("visibilitychange", throttled);
        };
    }, [
        refreshNow
    ]);
    const read = (name, fallback)=>{
        var _flags_name;
        return status === "error" ? fallback : (_flags_name = flags[name]) !== null && _flags_name !== void 0 ? _flags_name : fallback;
    };
    if (status === "ready" && latchedViewerV2.current === null) {
        latchedViewerV2.current = read("files:viewer-v2", DEFAULTS.viewerV2);
    }
    var _latchedViewerV2_current;
    return {
        status,
        editing: read("files:editing", DEFAULTS.editing),
        viewerV2: (_latchedViewerV2_current = latchedViewerV2.current) !== null && _latchedViewerV2_current !== void 0 ? _latchedViewerV2_current : DEFAULTS.viewerV2,
        mergePhone: read("files:merge-phone", DEFAULTS.mergePhone),
        rangeRead: read("files:range-read", DEFAULTS.rangeRead),
        deltaWrite: read("files:delta-write", DEFAULTS.deltaWrite),
        editLarge: read("files:edit-large", DEFAULTS.editLarge),
        onFlagOff: refreshNow
    };
}

// EXTERNAL MODULE: ./src/lib/files/analytics/filesAnalytics.ts
var filesAnalytics = __webpack_require__(72836);
;// ./src/lib/hooks/useAccessibleView.ts
/* __next_internal_client_entry_do_not_use__ useAccessibleView auto */ 

const STORAGE_KEY = "files.accessibleView";
const sizeBucket = (chars)=>chars < 10000 ? "lt10k" : chars < 100000 ? "10k-100k" : chars < 1000000 ? "100k-1m" : "gt1m";
function read() {
    try {
        return localStorage.getItem(STORAGE_KEY) === "1";
    } catch (e) {
        return false;
    }
}
function write(on) {
    try {
        localStorage.setItem(STORAGE_KEY, on ? "1" : "0");
    } catch (e) {}
}
/** The per-device choice between CodeMirror and the Accessible view (ux.md 8.10). */ function useAccessibleView(param) {
    let { textLength, swapView } = param;
    const analytics = (0,filesAnalytics/* useFilesAnalytics */.R)({
        component: "FileContentViewer"
    });
    const [on, setOn] = (0,react.useState)(false);
    // Read after mount so server and client render the same first frame.
    (0,react.useEffect)(()=>setOn(read()), []);
    const toggle = (0,react.useCallback)((via)=>{
        const to = !on;
        swapView();
        analytics.emit("files.a11y.accessible_view", {
            to: to ? "on" : "off",
            via,
            size: sizeBucket(textLength())
        });
        write(to);
        setOn(to);
    }, [
        on,
        swapView,
        analytics,
        textLength
    ]);
    return (0,react.useMemo)(()=>({
            on,
            toggle
        }), [
        on,
        toggle
    ]);
}

// EXTERNAL MODULE: ./src/lib/hooks/useFileTransport.ts + 5 modules
var useFileTransport = __webpack_require__(21427);
;// ./src/lib/hooks/useBufferFreshness.ts
/* __next_internal_client_entry_do_not_use__ compareTokens,useBufferFreshness auto */ 


const POLL_MS = 10000;
const BACKOFF_MS = 30000;
const CONFIRM_MS = 1000;
const BACKOFF_AFTER_FAILURES = 3;
/** Tokens are `v1:<size>:<mtime>:<hash>`; an equal hash with a different mtime is not a content change. */ function compareTokens(base, next) {
    if (base === next) return "same";
    const a = base.split(":");
    const b = next.split(":");
    const hashed = a.length === 4 && b.length === 4 && a[3] !== "-" && b[3] !== "-";
    return hashed && a[3] === b[3] ? "mtime_only" : "changed";
}
const canCheck = ()=>document.visibilityState === "visible" && navigator.onLine !== false;
/**
 * Detects on-disk changes to the active buffer: on focus, on becoming visible and every 10 s.
 * A change must be seen on two checks (one second apart) before the banner shows; it never touches the buffer.
 */ function useBufferFreshness(param) {
    let { active, enabled, sessionRunning, onAdoptToken } = param;
    const transport = (0,useFileTransport/* useFileTransport */.J7)();
    const analytics = (0,filesAnalytics/* useFilesAnalytics */.R)({
        component: "BannerStack"
    });
    const [stale, setStale] = (0,react.useState)(null);
    const live = (0,react.useRef)({
        active,
        sessionRunning,
        onAdoptToken,
        stale,
        analytics
    });
    live.current = {
        active,
        sessionRunning,
        onAdoptToken,
        stale,
        analytics
    };
    const pending = (0,react.useRef)(null);
    const dismissed = (0,react.useRef)(null);
    const loaded = (active === null || active === void 0 ? void 0 : active.status) === "loaded" ? active : null;
    var _loaded_path;
    const path = (_loaded_path = loaded === null || loaded === void 0 ? void 0 : loaded.path) !== null && _loaded_path !== void 0 ? _loaded_path : null;
    var _loaded_sessionId;
    const sessionId = (_loaded_sessionId = loaded === null || loaded === void 0 ? void 0 : loaded.sessionId) !== null && _loaded_sessionId !== void 0 ? _loaded_sessionId : null;
    var _loaded_token;
    const token = (_loaded_token = loaded === null || loaded === void 0 ? void 0 : loaded.token) !== null && _loaded_token !== void 0 ? _loaded_token : null;
    // A new baseline (save, reload, adopt) or another active buffer ends the current banner.
    (0,react.useEffect)(()=>{
        pending.current = null;
        dismissed.current = null;
        setStale(null);
    }, [
        path,
        token
    ]);
    const lastPath = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        if (!enabled || !path || !sessionId) return;
        const abort = new AbortController();
        let inflight = false;
        let failures = 0;
        let pollTimer;
        let confirmTimer;
        const show = (change)=>{
            const { active: cur, sessionRunning: running, analytics: a } = live.current;
            const info = {
                path,
                token: change,
                dirty: (cur === null || cur === void 0 ? void 0 : cur.status) === "loaded" && cur.dirty,
                actor: running ? "agent_likely" : "unknown"
            };
            if (!live.current.stale) a.emit("files.stale.shown", {
                dirty: String(info.dirty),
                actor: info.actor
            });
            setStale(info);
        };
        const handle = (next)=>{
            const cur = live.current.active;
            if ((cur === null || cur === void 0 ? void 0 : cur.status) !== "loaded" || cur.path !== path) return;
            const change = compareTokens(cur.token, next);
            if (change === "same") return;
            if (change === "mtime_only") {
                pending.current = null;
                live.current.onAdoptToken(path, next);
                return;
            }
            if (next === dismissed.current) return;
            if (live.current.stale) {
                if (live.current.stale.token !== next) show(next);
            } else if (pending.current === next) {
                pending.current = null;
                show(next);
            } else {
                pending.current = next;
                confirmTimer = setTimeout(()=>void check(), CONFIRM_MS);
            }
        };
        const check = async ()=>{
            if (inflight || abort.signal.aborted || !canCheck()) return;
            inflight = true;
            try {
                const r = await transport.stat({
                    sessionId,
                    path
                }, undefined, abort.signal);
                failures = 0;
                if (!abort.signal.aborted) handle(r.token);
            } catch (e) {
                if (!abort.signal.aborted) failures += 1;
            } finally{
                inflight = false;
            }
        };
        const schedule = ()=>{
            pollTimer = setTimeout(async ()=>{
                await check();
                if (!abort.signal.aborted) schedule();
            }, failures >= BACKOFF_AFTER_FAILURES ? BACKOFF_MS : POLL_MS);
        };
        const onWake = ()=>void check();
        const onVisibility = ()=>{
            if (document.visibilityState === "visible") onWake();
        };
        if (lastPath.current !== null && lastPath.current !== path) void check();
        lastPath.current = path;
        schedule();
        window.addEventListener("focus", onWake);
        document.addEventListener("visibilitychange", onVisibility);
        return ()=>{
            abort.abort();
            clearTimeout(pollTimer);
            clearTimeout(confirmTimer);
            window.removeEventListener("focus", onWake);
            document.removeEventListener("visibilitychange", onVisibility);
        };
    }, [
        enabled,
        path,
        sessionId,
        transport
    ]);
    const resolve = (0,react.useCallback)((action)=>{
        const cur = live.current.stale;
        live.current.analytics.emit("files.stale.action", {
            action
        });
        if (action === "dismiss" && cur) dismissed.current = cur.token;
        setStale(null);
    }, []);
    return {
        stale,
        resolve
    };
}

// EXTERNAL MODULE: ./src/components/sessions/FileContentViewer.tsx + 6 modules
var FileContentViewer = __webpack_require__(40415);
;// ./src/lib/files/deepLink.ts
// URL grammar for opening a file: `?file=<path>&line=<n>` (shareable) and `#file=<path>` (the
// selection mirror). Pure; useFilesDeepLink is the only code that writes it to history.
function parseDeepLink(search, hash) {
    const q = new URLSearchParams(search);
    const file = q.get("file") || new URLSearchParams(hash.replace(/^#/, "")).get("file");
    if (!file) return null;
    const line = Number(q.get("line"));
    return Number.isInteger(line) && line >= 1 ? {
        file,
        line
    } : {
        file
    };
}
const fileHash = (path)=>path ? "#file=".concat(encodeURIComponent(path)) : "";
/** Clamps a requested line into 1..lineCount; `clamped` feeds `files.nav.jump_line`. */ function clampLine(line, lineCount) {
    const target = Math.min(Math.max(1, Math.trunc(line)), Math.max(1, lineCount));
    return {
        line: target,
        clamped: target !== line
    };
}

;// ./src/lib/files/outline.ts
// Symbol outline without LSP or a parser: per-language declaration maps over lines. The editor's
// syntax tree is unreachable here (ADR-036 keeps @lezer/* inside the engine adapter), and
// declarations at line starts are all the outline lists.
/** Stand-in for the highlight-off threshold until Story 1.5.2 defines it. */ const OUTLINE_MAX_BYTES = 1024 * 1024;
/** Tries each regex in order; the first match wins and its builder names the symbol. */ function rules() {
    for(var _len = arguments.length, table = new Array(_len), _key = 0; _key < _len; _key++){
        table[_key] = arguments[_key];
    }
    return (line)=>{
        for (const [re, build] of table){
            const m = line.match(re);
            if (m) return build(m);
        }
        return null;
    };
}
const perLine = (rule)=>(lines)=>lines.flatMap((text, i)=>{
            const hit = rule(text);
            var _hit_depth;
            return hit ? [
                {
                    name: hit.name,
                    line: i + 1,
                    kind: hit.kind,
                    depth: (_hit_depth = hit.depth) !== null && _hit_depth !== void 0 ? _hit_depth : 0
                }
            ] : [];
        });
const ID = "[A-Za-z_$][\\w$]*";
const indentDepth = function(indent) {
    let unit = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : 4;
    return Math.floor(indent.replace(/\t/g, "    ").length / unit);
};
const go = perLine(rules([
    new RegExp("^func\\s+\\(\\s*(?:\\w+\\s+)?\\*?(".concat(ID, ")(?:\\[[^\\]]*\\])?\\s*\\)\\s*(").concat(ID, ")")),
    (m)=>({
            name: "".concat(m[1], ".").concat(m[2]),
            kind: "method"
        })
], [
    new RegExp("^func\\s+(".concat(ID, ")")),
    (m)=>({
            name: m[1],
            kind: "function"
        })
], [
    new RegExp("^type\\s+(".concat(ID, ")")),
    (m)=>({
            name: m[1],
            kind: "type"
        })
]));
const script = perLine(rules([
    new RegExp("^(?:export\\s+)?(?:default\\s+)?(?:async\\s+)?function\\*?\\s+(".concat(ID, ")")),
    (m)=>({
            name: m[1],
            kind: "function"
        })
], [
    new RegExp("^(?:export\\s+)?(?:default\\s+)?(?:abstract\\s+)?class\\s+(".concat(ID, ")")),
    (m)=>({
            name: m[1],
            kind: "class"
        })
], [
    new RegExp("^(?:export\\s+)?(?:interface|enum)\\s+(".concat(ID, ")")),
    (m)=>({
            name: m[1],
            kind: "type"
        })
], [
    new RegExp("^(?:export\\s+)?type\\s+(".concat(ID, ")\\b")),
    (m)=>({
            name: m[1],
            kind: "type"
        })
], [
    new RegExp("^(?:export\\s+)?const\\s+(".concat(ID, ")\\s*(?::[^=]+)?=\\s*(?:async\\s*)?(?:\\([^)]*\\)|").concat(ID, ")\\s*(?::[^=]+)?=>")),
    (m)=>({
            name: m[1],
            kind: "function"
        })
]));
const python = perLine(rules([
    new RegExp("^(\\s*)(?:async\\s+)?def\\s+(".concat(ID, ")")),
    (m)=>({
            name: m[2],
            kind: m[1] ? "method" : "function",
            depth: indentDepth(m[1])
        })
], [
    new RegExp("^(\\s*)class\\s+(".concat(ID, ")")),
    (m)=>({
            name: m[2],
            kind: "class",
            depth: indentDepth(m[1])
        })
]));
const VIS = "(?:pub(?:\\([^)]*\\))?\\s+)?";
const rust = perLine(rules([
    new RegExp("^(\\s*)".concat(VIS, "(?:const\\s+)?(?:async\\s+)?(?:unsafe\\s+)?fn\\s+(").concat(ID, ")")),
    (m)=>({
            name: m[2],
            kind: "function",
            depth: indentDepth(m[1])
        })
], [
    new RegExp("^(\\s*)".concat(VIS, "(?:struct|enum|trait|type|union)\\s+(").concat(ID, ")")),
    (m)=>({
            name: m[2],
            kind: "type",
            depth: indentDepth(m[1])
        })
], [
    new RegExp("^(\\s*)impl(?:<[^>]*>)?\\s+(?:[\\w:<>]+\\s+for\\s+)?(".concat(ID, ")")),
    (m)=>({
            name: "impl ".concat(m[2]),
            kind: "impl",
            depth: indentDepth(m[1])
        })
]));
const MOD = "(?:(?:public|private|protected|static|final|abstract|synchronized|default)\\s+)";
const java = perLine(rules([
    new RegExp("^(\\s*)".concat(MOD, "*(?:class|interface|enum|record)\\s+(").concat(ID, ")")),
    (m)=>({
            name: m[2],
            kind: "class",
            depth: indentDepth(m[1])
        })
], [
    new RegExp("^(\\s+)".concat(MOD, "+[\\w<>\\[\\], ?]+\\s+(").concat(ID, ")\\s*\\(")),
    (m)=>({
            name: m[2],
            kind: "method",
            depth: indentDepth(m[1])
        })
]));
const markdown = (lines)=>{
    let fenced = false;
    return lines.flatMap((text, i)=>{
        if (/^\s*(```|~~~)/.test(text)) {
            fenced = !fenced;
            return [];
        }
        const m = !fenced && text.match(/^(#{1,6})\s+(.+?)\s*#*\s*$/);
        return m ? [
            {
                name: m[2],
                line: i + 1,
                kind: "heading",
                depth: m[1].length - 1
            }
        ] : [];
    });
};
const SCANNERS = {
    go,
    typescript: script,
    tsx: script,
    javascript: script,
    jsx: script,
    python,
    rust,
    java,
    markdown
};
const outlineLanguages = ()=>Object.keys(SCANNERS);
/** Never throws: any failure is reported as `unavailable`. */ function buildOutline(text, language) {
    let opts = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : {};
    const scan = SCANNERS[language];
    if (!scan) return {
        kind: "unavailable",
        reason: "unsupported"
    };
    var _opts_size, _ref;
    if (((_ref = (_opts_size = opts.size) !== null && _opts_size !== void 0 ? _opts_size : text === null || text === void 0 ? void 0 : text.length) !== null && _ref !== void 0 ? _ref : 0) > OUTLINE_MAX_BYTES) return {
        kind: "unavailable",
        reason: "large"
    };
    try {
        return {
            kind: "symbols",
            symbols: scan(text.split("\n"))
        };
    } catch (e) {
        return {
            kind: "unavailable",
            reason: "error"
        };
    }
}

;// ./src/components/sessions/files/extensions/jumpToMatchNear.ts
/**
 * Nearest line (1-based) to `hitLine` that contains `needle`, searching outward up to `radius` lines;
 * on a tie the earlier line wins. Search hits come from disk, so a dirty buffer may have moved the text.
 */ function jumpToMatchNear(lines, hitLine, needle, radius) {
    if (!needle || lines.length === 0) return null;
    const center = Math.min(Math.max(1, Math.floor(hitLine)), lines.length);
    for(let d = 0; d <= radius; d++){
        for (const line of d === 0 ? [
            center
        ] : [
            center - d,
            center + d
        ]){
            if (line < 1 || line > lines.length) continue;
            const col = lines[line - 1].indexOf(needle);
            if (col >= 0) return {
                line,
                col
            };
        }
    }
    return null;
}

// EXTERNAL MODULE: ./src/lib/hooks/useFileService.ts
var useFileService = __webpack_require__(86793);
;// ./src/lib/hooks/useEditorBridge.ts
/* __next_internal_client_entry_do_not_use__ useEditorBridge auto */ 






const NEAR_RADIUS_LINES = 200;
const depthBucket = (n)=>n <= 1 ? "1" : n <= 5 ? "2-5" : "6+";
/**
 * Glue between the buffer reducer and the live editor: applies the reducer's pending cursor reveal
 * once the right editor is on screen, records the cursor before leaving a buffer, and owns go-to-line,
 * jumplist moves, outline text and reload-in-place.
 */ function useEditorBridge(buffers) {
    let { announce } = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : {};
    const analytics = (0,filesAnalytics/* useFilesAnalytics */.R)({
        component: "FileContentViewer"
    });
    const [editor, setEditor] = (0,react.useState)(null);
    const [reloadKey, setReloadKey] = (0,react.useState)(0);
    const [overlay, setOverlay] = (0,react.useState)(null);
    const [beyond, setBeyond] = (0,react.useState)(null);
    const pendingHit = (0,react.useRef)(null);
    const keepScroll = (0,react.useRef)(null);
    const { state, active, revealed, recordViewState } = buffers;
    const onEditorReady = (0,react.useCallback)((handle, path)=>setEditor({
            handle,
            path
        }), []);
    const { pendingReveal } = state;
    (0,react.useEffect)(()=>{
        if (!pendingReveal || !editor || editor.path !== pendingReveal.path) return;
        try {
            var _pendingHit_current;
            // The engine clamps; a hit past the loaded text (truncated read) says so instead of landing silently.
            // A windowed file loads on demand: its jump clamps itself, and the loaded text is not the whole file.
            const windowed = editor.handle.windowed !== undefined;
            const all = windowed ? [] : editor.handle.getText().split("\n");
            setBeyond(!windowed && pendingReveal.line > all.length ? {
                path: pendingReveal.path,
                line: pendingReveal.line
            } : null);
            editor.handle.gotoLine(pendingReveal.line);
            const hit = ((_pendingHit_current = pendingHit.current) === null || _pendingHit_current === void 0 ? void 0 : _pendingHit_current.path) === pendingReveal.path ? pendingHit.current : null;
            pendingHit.current = null;
            if (hit && (windowed || hit.line <= all.length)) {
                var _editor_handle_highlightRange, _editor_handle;
                const matched = hit.text.slice(hit.start, hit.end);
                let at = {
                    line: hit.line,
                    from: hit.start
                };
                var _all_;
                const here = (_all_ = all[hit.line - 1]) !== null && _all_ !== void 0 ? _all_ : "";
                if (hit.dirty && !here.includes(matched)) {
                    const near = jumpToMatchNear(all, hit.line, matched, NEAR_RADIUS_LINES);
                    if (near) {
                        at = {
                            line: near.line,
                            from: near.col
                        };
                        editor.handle.gotoLine(near.line);
                        announce === null || announce === void 0 ? void 0 : announce("Moved to line ".concat(near.line, " (buffer has unsaved changes)"));
                    } else {
                        announce === null || announce === void 0 ? void 0 : announce("Match not found in your unsaved version");
                        at = {
                            line: 0,
                            from: 0
                        };
                    }
                }
                if (at.line > 0) (_editor_handle_highlightRange = (_editor_handle = editor.handle).highlightRange) === null || _editor_handle_highlightRange === void 0 ? void 0 : _editor_handle_highlightRange.call(_editor_handle, {
                    line: at.line,
                    from: at.from,
                    to: at.from + matched.length
                });
            }
            const keep = keepScroll.current;
            if ((keep === null || keep === void 0 ? void 0 : keep.path) === pendingReveal.path) {
                editor.handle.restoreScroll(keep.snapshot);
                keepScroll.current = null;
            }
        } catch (e) {
            return; // the engine was torn down; the next onReady retries
        }
        revealed(pendingReveal.seq);
    }, [
        pendingReveal,
        editor,
        revealed,
        announce
    ]);
    /** Saves the cursor of the buffer on screen so leaving it and coming back lands on the same line. */ const capture = (0,react.useCallback)(()=>{
        if (!editor || !active || editor.path !== active.path) return;
        try {
            const { line, col } = editor.handle.getCursor();
            recordViewState(active.path, {
                ...active.viewState,
                line,
                col
            });
        } catch (e) {
        // engine gone: keep the last recorded position
        }
    }, [
        editor,
        active,
        recordViewState
    ]);
    const open = (0,react.useCallback)(function(path) {
        let opts = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : {};
        capture();
        buffers.open(path, opts);
    }, [
        capture,
        buffers
    ]);
    const activate = (0,react.useCallback)((path)=>{
        capture();
        buffers.activate(path);
    }, [
        capture,
        buffers
    ]);
    const jumpTo = (0,react.useCallback)(function(path, line) {
        let opts = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : {};
        const { hit, ...rest } = opts;
        capture();
        pendingHit.current = hit ? {
            ...hit,
            path
        } : null;
        buffers.jump(path, line, rest);
    }, [
        capture,
        buffers
    ]);
    const move = (0,react.useCallback)((delta, via)=>{
        const behind = state.navIndex;
        const depth = delta < 0 ? behind : state.nav.length - 1 - behind;
        if (depth <= 0) return;
        capture();
        analytics.emit(delta < 0 ? "files.nav.back" : "files.nav.forward", {
            via,
            depth: depthBucket(depth)
        });
        if (delta < 0) buffers.navBack();
        else buffers.navForward();
    }, [
        state.navIndex,
        state.nav.length,
        capture,
        analytics,
        buffers
    ]);
    const back = (0,react.useCallback)((via)=>move(-1, via), [
        move
    ]);
    const forward = (0,react.useCallback)((via)=>move(1, via), [
        move
    ]);
    const goToLine = (0,react.useCallback)((requested, via)=>{
        if (!editor || !active || editor.path !== active.path || !Number.isFinite(requested)) return;
        const windowed = editor.handle.windowed;
        const { line, clamped } = windowed ? {
            line: Math.max(1, Math.trunc(requested)),
            clamped: false
        } : clampLine(requested, editor.handle.getText().split("\n").length);
        analytics.emit("files.nav.jump_line", {
            via,
            clamped: String(clamped)
        });
        jumpTo(active.path, line);
    }, [
        editor,
        active,
        analytics,
        jumpTo
    ]);
    /** Fetches the file again in place: same cursor line and scroll once the new text is on screen. */ const reload = (0,react.useCallback)((newToken)=>{
        if (!active) return;
        const here = (editor === null || editor === void 0 ? void 0 : editor.path) === active.path ? editor.handle : null;
        let cursor = {
            line: active.viewState.line,
            col: active.viewState.col
        };
        try {
            if (here) {
                cursor = here.getCursor();
                keepScroll.current = {
                    path: active.path,
                    snapshot: here.scrollSnapshot()
                };
            }
        } catch (e) {
            keepScroll.current = null;
        }
        (0,useFileService/* invalidateFileContent */.JB)(active.sessionId, active.path);
        setEditor(null);
        setReloadKey((k)=>k + 1);
        buffers.adoptToken(active.path, newToken);
        buffers.jump(active.path, cursor.line, {
            col: cursor.col,
            chain: "step"
        });
    }, [
        active,
        editor,
        buffers
    ]);
    /** The read view is about to be swapped (CodeMirror <-> Accessible view): carry the line across to whichever mounts next. */ const swapView = (0,react.useCallback)(()=>{
        if (!active || (editor === null || editor === void 0 ? void 0 : editor.path) !== active.path) return;
        let cursor;
        try {
            cursor = editor.handle.getCursor();
        } catch (e) {
            return;
        }
        setEditor(null);
        buffers.jump(active.path, cursor.line, {
            col: cursor.col,
            chain: "step"
        });
    }, [
        active,
        editor,
        buffers
    ]);
    const outline = (0,react.useCallback)(()=>{
        if (!editor || !active || editor.path !== active.path) return {
            kind: "unavailable",
            reason: "error"
        };
        if (editor.handle.windowed) return {
            kind: "unavailable",
            reason: "large"
        };
        try {
            const text = editor.handle.getText();
            return buildOutline(text, (0,FileContentViewer/* detectLanguage */.od)(active.path), {
                size: text.length
            });
        } catch (e) {
            return {
                kind: "unavailable",
                reason: "error"
            };
        }
    }, [
        editor,
        active
    ]);
    /** Cursor and line endings of the buffer on screen, or null while its editor is not ready. */ const inspect = (0,react.useCallback)(()=>{
        if (!editor || !active || editor.path !== active.path) return null;
        try {
            const { line, col } = editor.handle.getCursor();
            return {
                line,
                col,
                eol: null
            };
        } catch (e) {
            return null;
        }
    }, [
        editor,
        active
    ]);
    const eol = (0,react.useCallback)(()=>{
        if (!editor || !active || editor.path !== active.path || editor.handle.windowed) return null;
        try {
            const text = editor.handle.getText();
            var _text_match;
            const crlf = ((_text_match = text.match(/\r\n/g)) !== null && _text_match !== void 0 ? _text_match : []).length;
            if (crlf === 0) return "LF";
            var _text_match1;
            return crlf === ((_text_match1 = text.match(/\n/g)) !== null && _text_match1 !== void 0 ? _text_match1 : []).length ? "CRLF" : "mixed";
        } catch (e) {
            return null;
        }
    }, [
        editor,
        active
    ]);
    const text = (0,react.useCallback)(()=>{
        if (!editor || !active || editor.path !== active.path) return null;
        try {
            return editor.handle.getText();
        } catch (e) {
            return null;
        }
    }, [
        editor,
        active
    ]);
    const focusEditor = (0,react.useCallback)(()=>{
        if (!editor || !active || editor.path !== active.path) return;
        try {
            editor.handle.focus();
        } catch (e) {}
    }, [
        editor,
        active
    ]);
    const dismissBeyond = (0,react.useCallback)(()=>setBeyond(null), []);
    var _editor_handle_windowed;
    const windowed = editor && active && editor.path === active.path ? (_editor_handle_windowed = editor.handle.windowed) !== null && _editor_handle_windowed !== void 0 ? _editor_handle_windowed : null : null;
    return {
        onEditorReady,
        windowed,
        focusEditor,
        beyond: beyond && active && beyond.path === active.path ? beyond : null,
        dismissBeyond,
        inspect,
        eol,
        text,
        reloadKey,
        overlay,
        setOverlay,
        open,
        activate,
        jumpTo,
        back,
        forward,
        goToLine,
        reload,
        swapView,
        outline
    };
}

;// ./src/lib/hooks/useFilesFeedback.ts
/* __next_internal_client_entry_do_not_use__ SUCCESS_MS,UNDO_MS,successText,useFilesFeedback auto */ 
/** ux.md 8.6: successes clear after 8 s, an Undo toast after 12 s; errors persist until dismissed. */ const SUCCESS_MS = 8000;
const UNDO_MS = 12000;
function successText(kind, name) {
    switch(kind){
        case "saved":
            return "Saved ".concat(name);
        case "merge_saved":
            return "Merged and saved ".concat(name);
        case "reloaded":
            return "Reloaded ".concat(name, " from disk");
        case "draft_restored":
            return "Restored your unsaved draft of ".concat(name);
        case "overwrote":
            return "Overwrote ".concat(name, " on disk with your version");
        case "reopened":
            return "Reopened ".concat(name);
    }
}
/**
 * Success and error messages for the status-bar region and the mobile toast. The status region holds
 * one message (an error is never replaced by a success); the toast follows the latest success.
 */ function useFilesFeedback() {
    const [message, setMessage] = (0,react.useState)(null);
    const [toast, setToast] = (0,react.useState)(null);
    const seq = (0,react.useRef)(0);
    const timer = (0,react.useRef)(null);
    const lifetime = (0,react.useRef)(SUCCESS_MS);
    const stop = (0,react.useCallback)(()=>{
        if (timer.current !== null) clearTimeout(timer.current);
        timer.current = null;
    }, []);
    const clearSuccess = (0,react.useCallback)(()=>{
        setToast(null);
        setMessage((m)=>(m === null || m === void 0 ? void 0 : m.kind) === "success" ? null : m);
    }, []);
    const start = (0,react.useCallback)((ms)=>{
        stop();
        lifetime.current = ms;
        timer.current = setTimeout(clearSuccess, ms);
    }, [
        stop,
        clearSuccess
    ]);
    (0,react.useEffect)(()=>stop, [
        stop
    ]);
    const announceSuccess = (0,react.useCallback)(function(kind, name) {
        let opts = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : {};
        const next = {
            id: ++seq.current,
            kind: "success",
            text: successText(kind, name),
            undo: opts.undo
        };
        setToast(next);
        setMessage((m)=>(m === null || m === void 0 ? void 0 : m.kind) === "error" ? m : next);
        start(opts.undo ? UNDO_MS : SUCCESS_MS);
    }, [
        start
    ]);
    /** A polite status-bar note with no toast (e.g. "Moved to line 15"); clears like a success. */ const announceInfo = (0,react.useCallback)((text)=>{
        setMessage((m)=>(m === null || m === void 0 ? void 0 : m.kind) === "error" ? m : {
                id: ++seq.current,
                kind: "success",
                text
            });
        start(SUCCESS_MS);
    }, [
        start
    ]);
    /** A wait in flight ("Checking the file on disk..."); the next success or error replaces it. */ const announceProgress = (0,react.useCallback)((text)=>{
        setMessage((m)=>(m === null || m === void 0 ? void 0 : m.kind) === "error" ? m : {
                id: ++seq.current,
                kind: "progress",
                text
            });
    }, []);
    const announceError = (0,react.useCallback)((text)=>{
        setMessage({
            id: ++seq.current,
            kind: "error",
            text
        });
    }, []);
    const dismiss = (0,react.useCallback)(()=>{
        stop();
        setToast(null);
        setMessage(null);
    }, [
        stop
    ]);
    const pause = (0,react.useCallback)(()=>stop(), [
        stop
    ]);
    /** Hover or focus left: the full window starts again. */ const resume = (0,react.useCallback)(()=>{
        if (toast || (message === null || message === void 0 ? void 0 : message.kind) === "success") start(lifetime.current);
    }, [
        toast,
        message,
        start
    ]);
    return {
        message,
        toast,
        announceSuccess,
        announceInfo,
        announceProgress,
        announceError,
        dismiss,
        pause,
        resume
    };
}

// EXTERNAL MODULE: ./src/components/sessions/files/PendingState.tsx + 1 modules
var PendingState = __webpack_require__(95272);
;// ./src/lib/files/bufferReducer.ts
// Pure tab/buffer state for the Files viewer (ADR-032). No React, no I/O, no file text:
// the reducer knows paths, flags and positions; text lives in the editor's EditorState.
const MOBILE_CAP = 10;
const DESKTOP_CAP = 30;
const CLOSED_STACK_LIMIT = 10;
const ORIGIN = {
    line: 1,
    col: 0,
    scrollTop: 0
};
function initialBufferState() {
    let cap = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : DESKTOP_CAP;
    return {
        buffers: [],
        activeIndex: -1,
        cap,
        tick: 0,
        nav: [],
        navIndex: -1,
        closed: [],
        notice: null,
        pendingReveal: null,
        revealSeq: 0
    };
}
// ---- predicates ----
const isDirty = (b)=>b.status === "loaded" && b.dirty;
const isSaving = (b)=>b.status === "loaded" && b.saving;
const isEditing = (b)=>b.status === "loaded" && b.mode === "edit";
/** The single preview slot may be taken over by the next single-open. */ const isReplaceablePreview = (b)=>!b.pinned && !isDirty(b) && !isSaving(b);
/** Cap eviction never takes a loading buffer. */ const isEvictable = (b)=>isReplaceablePreview(b) && b.status !== "loading";
const indexOf = (s, path)=>s.buffers.findIndex((b)=>b.path === path);
// ---- helpers ----
const windowedAt = (source, line)=>(source === null || source === void 0 ? void 0 : source.kind) === "windowed" ? {
        ...source,
        windowStartLine: line
    } : {
        kind: "full"
    };
function touched(s, index) {
    const tick = s.tick + 1;
    return {
        ...s,
        tick,
        buffers: s.buffers.map((b, i)=>i === index ? {
                ...b,
                usedAt: tick
            } : b)
    };
}
function mapBuffer(s, path, fn) {
    const i = indexOf(s, path);
    if (i < 0) return s;
    const next = fn(s.buffers[i]);
    return next === s.buffers[i] ? s : {
        ...s,
        buffers: s.buffers.map((b, j)=>j === i ? next : b)
    };
}
function syncCurrentNav(s) {
    const active = s.buffers[s.activeIndex];
    const cur = s.nav[s.navIndex];
    if (!active || !cur || cur.path !== active.path) return s;
    const { line, col } = active.viewState;
    if (cur.line === line && cur.col === col) return s;
    return {
        ...s,
        nav: s.nav.map((e, i)=>i === s.navIndex ? {
                ...e,
                line,
                col
            } : e)
    };
}
/** Records arrival at `path`; drops the forward half like a browser history. */ function pushNav(s, path, viewState) {
    var _head_;
    const synced = syncCurrentNav(s);
    const head = synced.nav.slice(0, synced.navIndex + 1);
    const entry = {
        path,
        line: viewState.line,
        col: viewState.col
    };
    const nav = ((_head_ = head[head.length - 1]) === null || _head_ === void 0 ? void 0 : _head_.path) === path ? [
        ...head.slice(0, -1),
        entry
    ] : [
        ...head,
        entry
    ];
    return {
        ...synced,
        nav,
        navIndex: nav.length - 1
    };
}
function pushClosed(s, b) {
    const closed = [
        ...s.closed,
        {
            path: b.path,
            viewState: b.viewState
        }
    ].slice(-CLOSED_STACK_LIMIT);
    return {
        ...s,
        closed
    };
}
function removeAt(s, index) {
    const removed = s.buffers[index];
    const buffers = s.buffers.filter((_, i)=>i !== index);
    let activeIndex = s.activeIndex;
    if (index < activeIndex) activeIndex -= 1;
    else if (index === activeIndex) {
        const mru = buffers.reduce((best, b, i)=>best < 0 || b.usedAt > buffers[best].usedAt ? i : best, -1);
        activeIndex = mru;
    }
    return pushClosed({
        ...s,
        buffers,
        activeIndex
    }, removed);
}
function newLoading(path, sessionId, pinned, viewState, usedAt) {
    return {
        status: "loading",
        path,
        sessionId,
        pinned,
        viewState,
        usedAt
    };
}
function lruEvictable(s) {
    let best = -1;
    s.buffers.forEach((b, i)=>{
        if (isEvictable(b) && (best < 0 || b.usedAt < s.buffers[best].usedAt)) best = i;
    });
    return best;
}
/** Adds a new Loading buffer applying the preview slot and the one cap rule. */ function openNew(s, path, sessionId, pinned, viewState) {
    let next = s;
    const atCap = next.buffers.length >= next.cap;
    const previewIndex = !pinned ? next.buffers.findIndex(isReplaceablePreview) : -1;
    let victim = previewIndex;
    if (victim < 0 && atCap) victim = lruEvictable(next);
    if (victim < 0 && atCap) return {
        ...next,
        notice: {
            kind: "refused",
            path,
            cap: next.cap
        }
    };
    let notice = next.notice;
    if (victim >= 0) {
        const gone = next.buffers[victim];
        next = pushClosed(next, gone);
        next = {
            ...next,
            buffers: next.buffers.filter((_, i)=>i !== victim),
            activeIndex: next.activeIndex > victim ? next.activeIndex - 1 : next.activeIndex
        };
        if (atCap) notice = {
            kind: "evicted",
            path: gone.path,
            cap: next.cap
        };
    }
    const tick = next.tick + 1;
    const buffers = [
        ...next.buffers,
        newLoading(path, sessionId, pinned, viewState, tick)
    ];
    return pushNav({
        ...next,
        buffers,
        activeIndex: buffers.length - 1,
        tick,
        notice
    }, path, viewState);
}
function openAction(from, a) {
    const s = syncCurrentNav(from);
    const existing = indexOf(s, a.path);
    var _a_pinned, _a_viewState;
    if (existing < 0) return openNew(s, a.path, a.sessionId, (_a_pinned = a.pinned) !== null && _a_pinned !== void 0 ? _a_pinned : false, (_a_viewState = a.viewState) !== null && _a_viewState !== void 0 ? _a_viewState : ORIGIN);
    let next = touched(s, existing);
    next = a.pinned ? mapBuffer(next, a.path, (b)=>b.pinned ? b : {
            ...b,
            pinned: true
        }) : next;
    next = {
        ...next,
        activeIndex: existing
    };
    return pushNav(next, a.path, next.buffers[existing].viewState);
}
function jumpAction(from, a) {
    const s = syncCurrentNav(from);
    const existing = indexOf(s, a.path);
    const target = (b)=>{
        var _b_viewState_scrollTop;
        return {
            line: a.line,
            col: a.col,
            scrollTop: (_b_viewState_scrollTop = b === null || b === void 0 ? void 0 : b.viewState.scrollTop) !== null && _b_viewState_scrollTop !== void 0 ? _b_viewState_scrollTop : 0
        };
    };
    let next;
    if (existing < 0) {
        var _next_notice;
        var _a_pinned;
        next = openNew(s, a.path, a.sessionId, (_a_pinned = a.pinned) !== null && _a_pinned !== void 0 ? _a_pinned : false, target());
        if (((_next_notice = next.notice) === null || _next_notice === void 0 ? void 0 : _next_notice.kind) === "refused") return next;
    } else {
        next = touched(s, existing);
        next = mapBuffer(next, a.path, (b)=>({
                ...b,
                pinned: b.pinned || !!a.pinned,
                viewState: target(b)
            }));
        next = pushNav({
            ...next,
            activeIndex: existing
        }, a.path, target(s.buffers[existing]));
    }
    if (a.chain !== "step" || s.navIndex < 0) return next;
    const nav = s.nav.map((e, i)=>i === s.navIndex ? {
            path: a.path,
            line: a.line,
            col: a.col
        } : e);
    return {
        ...next,
        nav,
        navIndex: s.navIndex
    };
}
function moveAction(s, path, delta) {
    var _s_buffers_s_activeIndex;
    const from = indexOf(s, path);
    const to = from + delta;
    if (from < 0 || to < 0 || to >= s.buffers.length) return s;
    const buffers = s.buffers.slice();
    [buffers[from], buffers[to]] = [
        buffers[to],
        buffers[from]
    ];
    const activePath = (_s_buffers_s_activeIndex = s.buffers[s.activeIndex]) === null || _s_buffers_s_activeIndex === void 0 ? void 0 : _s_buffers_s_activeIndex.path;
    return {
        ...s,
        buffers,
        activeIndex: buffers.findIndex((b)=>b.path === activePath)
    };
}
function bufferReducer_navigate(s, delta) {
    var _next_buffers_next_activeIndex, _opened_notice;
    const synced = syncCurrentNav(s);
    const target = synced.navIndex + delta;
    const entry = synced.nav[target];
    if (!entry) return s;
    const next = {
        ...synced,
        navIndex: target
    };
    const index = indexOf(next, entry.path);
    if (index >= 0) {
        const seen = mapBuffer(next, entry.path, (b)=>({
                ...b,
                viewState: {
                    ...b.viewState,
                    line: entry.line,
                    col: entry.col
                }
            }));
        return {
            ...touched(seen, index),
            activeIndex: index
        };
    }
    var _next_buffers_next_activeIndex_sessionId;
    const sessionId = (_next_buffers_next_activeIndex_sessionId = (_next_buffers_next_activeIndex = next.buffers[next.activeIndex]) === null || _next_buffers_next_activeIndex === void 0 ? void 0 : _next_buffers_next_activeIndex.sessionId) !== null && _next_buffers_next_activeIndex_sessionId !== void 0 ? _next_buffers_next_activeIndex_sessionId : "";
    const opened = openNew(next, entry.path, sessionId, false, {
        ...ORIGIN,
        line: entry.line,
        col: entry.col
    });
    if (((_opened_notice = opened.notice) === null || _opened_notice === void 0 ? void 0 : _opened_notice.kind) === "refused") return {
        ...s,
        notice: opened.notice
    };
    // openNew appended a nav entry; this jump is already in the list.
    return {
        ...opened,
        nav: next.nav,
        navIndex: target
    };
}
function setModeAction(s, path, mode) {
    return mapBuffer(s, path, (b)=>{
        if (b.status !== "loaded") return b;
        if (mode === "read") return b.mode === "read" ? b : {
            ...b,
            mode: "read"
        };
        if (b.readOnly !== "none" || b.mode === "edit") return b;
        return {
            ...b,
            mode: "edit",
            readOnly: "none",
            pinned: true
        };
    });
}
/** A notice describes the last action only; any later action replaces it. */ function bufferReducer(state, a) {
    const s = state.notice && a.type !== "clearNotice" ? {
        ...state,
        notice: null
    } : state;
    return withReveal(s, apply(s, a), a);
}
/** A change of active buffer (or an explicit jump) asks the editor to restore that buffer's cursor. */ function withReveal(prev, next, a) {
    var _next_notice, _activeBuffer;
    if (a.type === "revealed" || a.type === "clearNotice" || ((_next_notice = next.notice) === null || _next_notice === void 0 ? void 0 : _next_notice.kind) === "refused") return next;
    const active = activeBuffer(next);
    if (!active || a.type !== "jump" && ((_activeBuffer = activeBuffer(prev)) === null || _activeBuffer === void 0 ? void 0 : _activeBuffer.path) === active.path) return next;
    const { line, col } = active.viewState;
    if (a.type !== "jump" && line <= 1 && col === 0) return next.pendingReveal ? {
        ...next,
        pendingReveal: null
    } : next;
    const seq = next.revealSeq + 1;
    return {
        ...next,
        revealSeq: seq,
        pendingReveal: {
            path: active.path,
            line,
            col,
            seq
        }
    };
}
function apply(s, a) {
    switch(a.type){
        case "open":
            return openAction(s, a);
        case "restore":
            {
                const unique = a.tabs.filter((t, i)=>a.tabs.findIndex((u)=>u.path === t.path) === i);
                const buffers = unique.map((t, i)=>newLoading(t.path, a.sessionId, true, t.viewState, i + 1));
                const activeIndex = buffers.length === 0 ? -1 : Math.min(Math.max(a.activeIndex, 0), buffers.length - 1);
                const active = buffers[activeIndex];
                const nav = active ? [
                    {
                        path: active.path,
                        line: active.viewState.line,
                        col: active.viewState.col
                    }
                ] : [];
                return {
                    ...s,
                    buffers: buffers.slice(0, s.cap),
                    activeIndex: Math.min(activeIndex, s.cap - 1),
                    tick: buffers.length,
                    nav,
                    navIndex: nav.length - 1
                };
            }
        case "close":
            {
                const i = indexOf(s, a.path);
                if (i < 0 || isDirty(s.buffers[i]) && !a.discardConfirmed) return s;
                return removeAt(s, i);
            }
        case "jump":
            return jumpAction(s, a);
        case "move":
            return moveAction(s, a.path, a.delta);
        case "adoptToken":
            return mapBuffer(s, a.path, (b)=>b.status !== "loaded" || b.token === a.token ? b : {
                    ...b,
                    token: a.token
                });
        case "revealed":
            var _s_pendingReveal;
            return ((_s_pendingReveal = s.pendingReveal) === null || _s_pendingReveal === void 0 ? void 0 : _s_pendingReveal.seq) === a.seq ? {
                ...s,
                pendingReveal: null
            } : s;
        case "reopenClosed":
            {
                var _s_buffers_, _opened_notice;
                const top = s.closed[s.closed.length - 1];
                if (!top) return s;
                const rest = {
                    ...s,
                    closed: s.closed.slice(0, -1)
                };
                const already = indexOf(s, top.path);
                if (already >= 0) return {
                    ...touched(rest, already),
                    activeIndex: already
                };
                var _s_buffers__sessionId;
                const sessionId = (_s_buffers__sessionId = (_s_buffers_ = s.buffers[0]) === null || _s_buffers_ === void 0 ? void 0 : _s_buffers_.sessionId) !== null && _s_buffers__sessionId !== void 0 ? _s_buffers__sessionId : "";
                const opened = openNew({
                    ...s,
                    closed: s.closed.slice(0, -1)
                }, top.path, sessionId, true, top.viewState);
                return ((_opened_notice = opened.notice) === null || _opened_notice === void 0 ? void 0 : _opened_notice.kind) === "refused" ? {
                    ...s,
                    notice: opened.notice
                } : opened;
            }
        case "activate":
            {
                const i = indexOf(s, a.path);
                return i < 0 ? s : {
                    ...touched(s, i),
                    activeIndex: i
                };
            }
        case "pin":
            return mapBuffer(s, a.path, (b)=>b.pinned ? b : {
                    ...b,
                    pinned: true
                });
        case "unpin":
            // A dirty or editing buffer must stay pinned or the next single-open would replace it.
            return mapBuffer(s, a.path, (b)=>!b.pinned || isDirty(b) || isEditing(b) ? b : {
                    ...b,
                    pinned: false
                });
        case "navBack":
            return bufferReducer_navigate(s, -1);
        case "navForward":
            return bufferReducer_navigate(s, 1);
        case "recordViewState":
            return mapBuffer(s, a.path, (b)=>({
                    ...b,
                    viewState: a.viewState
                }));
        case "markLoaded":
            return mapBuffer(s, a.path, (b)=>{
                if (b.status === "failed") return b;
                const common = {
                    path: b.path,
                    sessionId: b.sessionId,
                    pinned: b.pinned,
                    viewState: b.viewState,
                    usedAt: b.usedAt
                };
                const loaded = {
                    ...common,
                    status: "loaded",
                    token: a.token,
                    dirty: false,
                    saving: false,
                    source: windowedAt(a.source, b.viewState.line)
                };
                return {
                    ...loaded,
                    mode: "read",
                    readOnly: a.readOnly
                };
            });
        case "markFailed":
            return mapBuffer(s, a.path, (b)=>b.status === "loaded" ? b : {
                    path: b.path,
                    sessionId: b.sessionId,
                    pinned: b.pinned,
                    viewState: b.viewState,
                    usedAt: b.usedAt,
                    status: "failed",
                    reason: a.reason
                });
        case "markDirty":
            return mapBuffer(s, a.path, (b)=>b.status !== "loaded" || b.dirty === a.dirty ? b : {
                    ...b,
                    dirty: a.dirty,
                    pinned: b.pinned || a.dirty
                });
        case "markSaving":
            return mapBuffer(s, a.path, (b)=>b.status !== "loaded" || b.saving === a.saving ? b : {
                    ...b,
                    saving: a.saving
                });
        case "setMode":
            return setModeAction(s, a.path, a.mode);
        case "flagOff":
            return {
                ...s,
                buffers: s.buffers.map((b)=>b.status === "loaded" && b.mode === "edit" ? {
                        ...b,
                        mode: "read"
                    } : b)
            };
        case "setCap":
            return {
                ...s,
                cap: a.cap
            };
        case "evict":
            {
                const i = lruEvictable(s);
                return i < 0 ? s : removeAt(s, i);
            }
        case "clearNotice":
            return s.notice ? {
                ...s,
                notice: null
            } : s;
    }
}
// ---- selectors ----
const activeBuffer = (s)=>{
    var _s_buffers_s_activeIndex;
    return (_s_buffers_s_activeIndex = s.buffers[s.activeIndex]) !== null && _s_buffers_s_activeIndex !== void 0 ? _s_buffers_s_activeIndex : null;
};
const selectedPath = (s)=>{
    var _activeBuffer;
    var _activeBuffer_path;
    return (_activeBuffer_path = (_activeBuffer = activeBuffer(s)) === null || _activeBuffer === void 0 ? void 0 : _activeBuffer.path) !== null && _activeBuffer_path !== void 0 ? _activeBuffer_path : null;
};
/** Most recently used first; feeds RecentFilesSection. */ const mruPaths = (s)=>[
        ...s.buffers
    ].sort((x, y)=>y.usedAt - x.usedAt).map((b)=>b.path);
const hasDirty = (s)=>s.buffers.some(isDirty);
const canGoBack = (s)=>s.navIndex > 0;
const canGoForward = (s)=>s.navIndex >= 0 && s.navIndex < s.nav.length - 1;

// EXTERNAL MODULE: ./src/lib/files/analytics/events.ts
var events = __webpack_require__(35136);
// EXTERNAL MODULE: ./src/lib/files/transport/fileTransport.ts
var fileTransport = __webpack_require__(87969);
// EXTERNAL MODULE: ./src/lib/files/windowedSource.ts
var windowedSource = __webpack_require__(78012);
;// ./src/lib/hooks/useOpenBuffers.ts
/* __next_internal_client_entry_do_not_use__ classifyFailure,storageKey,writePersisted,useOpenBuffers auto */ 







const MOBILE_WIDTH = 768;
const PERSIST_DEBOUNCE_MS = 300;
const STORAGE_VERSION = 1;
function classifyFailure(err) {
    if (!(err instanceof fileTransport/* Rejected */.HE)) return "rpc_error";
    return err.reason === "not_found" || err.reason === "too_large" || err.reason === "binary" ? err.reason : "rpc_error";
}
const formFactor = ()=> true && window.innerWidth < MOBILE_WIDTH ? "mobile" : "desktop";
const capFor = (form, coarse)=>form === "mobile" || coarse ? MOBILE_CAP : DESKTOP_CAP;
const coarsePointer = ()=> true && typeof window.matchMedia === "function" && window.matchMedia("(pointer: coarse)").matches;
const storageKey = (sessionId, device)=>"ssq.files.".concat(sessionId, ".").concat(device);
const isNum = (n)=>typeof n === "number" && Number.isFinite(n);
function readPersisted(sessionId) {
    try {
        const raw = window.localStorage.getItem(storageKey(sessionId, formFactor()));
        if (!raw) return {
            tabs: [],
            active: 0,
            outcome: null
        };
        const data = JSON.parse(raw);
        const rows = Array.isArray(data.tabs) ? data.tabs : [];
        const tabs = rows.filter((t)=>t && typeof t.path === "string" && t.path && isNum(t.line) && isNum(t.col) && isNum(t.scrollTop)).map((t)=>({
                path: t.path,
                viewState: {
                    line: t.line,
                    col: t.col,
                    scrollTop: t.scrollTop
                }
            }));
        const ok = data.v === STORAGE_VERSION && tabs.length === rows.length;
        return {
            tabs,
            active: isNum(data.active) ? data.active : 0,
            outcome: ok ? "ok" : "partial"
        };
    } catch (e) {
        return {
            tabs: [],
            active: 0,
            outcome: "storage_unavailable"
        };
    }
}
function writePersisted(sessionId, state) {
    try {
        const payload = {
            v: STORAGE_VERSION,
            active: state.activeIndex,
            tabs: state.buffers.map((b)=>({
                    path: b.path,
                    line: b.viewState.line,
                    col: b.viewState.col,
                    scrollTop: b.viewState.scrollTop
                }))
        };
        window.localStorage.setItem(storageKey(sessionId, formFactor()), JSON.stringify(payload));
        return true;
    } catch (e) {
        return false;
    }
}
function useOpenBuffers(sessionId) {
    let { initialSelectedPath, onSelectedPathChange } = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : {};
    var _restoredRef_current;
    const restoredRef = (0,react.useRef)(null);
    const [state, dispatchRaw] = (0,react.useReducer)(bufferReducer, undefined, ()=>{
        const base = initialBufferState(capFor(formFactor(), coarsePointer()));
        const restored =  false ? 0 : readPersisted(sessionId);
        restoredRef.current = restored;
        return restored.tabs.length ? bufferReducer(base, {
            type: "restore",
            sessionId,
            tabs: restored.tabs,
            activeIndex: restored.active
        }) : base;
    });
    const [storageFailed, setStorageFailed] = (0,react.useState)(((_restoredRef_current = restoredRef.current) === null || _restoredRef_current === void 0 ? void 0 : _restoredRef_current.outcome) === "storage_unavailable");
    const analytics = (0,filesAnalytics/* useFilesAnalytics */.R)({
        component: "FilesTab",
        sessionId
    });
    const transport = (0,useFileTransport/* useFileTransport */.J7)();
    const rangeRead = (0,react.useRef)(false);
    rangeRead.current = (0,FeatureFlagsContext/* useFeatureFlag */.u)("files:range-read");
    const dispatch = (0,react.useCallback)((a)=>dispatchRaw(a), []);
    // One restore event per mount, after the buffers exist.
    (0,react.useEffect)(()=>{
        const restored = restoredRef.current;
        if (restored === null || restored === void 0 ? void 0 : restored.outcome) analytics.emit("files.tab.restore", {
            outcome: restored.outcome,
            tabs: (0,events/* bucketTabs */.FP)(restored.tabs.length)
        });
        restoredRef.current = null;
    }, [
        analytics
    ]);
    // Viewport and pointer decide the tab cap; both are re-read on resize.
    (0,react.useEffect)(()=>{
        const update = ()=>dispatchRaw({
                type: "setCap",
                cap: capFor(formFactor(), coarsePointer())
            });
        window.addEventListener("resize", update);
        return ()=>window.removeEventListener("resize", update);
    }, []);
    // Where each newly opened path came from; consumed by `files.tab.open` once its stat lands.
    const sources = (0,react.useRef)(new Map());
    const stateRef = (0,react.useRef)(state);
    stateRef.current = state;
    const noteSource = (0,react.useCallback)((path, source, pinned)=>{
        if (!stateRef.current.buffers.some((b)=>b.path === path)) sources.current.set(path, {
            source,
            pinned
        });
    }, []);
    // Load each new buffer once; a stat is enough, the viewer owns the text.
    const inflight = (0,react.useRef)(new Map());
    (0,react.useEffect)(()=>{
        for (const b of state.buffers){
            if (b.status !== "loading") continue;
            const key = "".concat(b.sessionId, "\0").concat(b.path);
            if (inflight.current.has(key)) continue;
            const ctl = new AbortController();
            inflight.current.set(key, ctl);
            transport.stat({
                sessionId: b.sessionId,
                path: b.path
            }, undefined, ctl.signal).then((r)=>{
                if (ctl.signal.aborted) return;
                const { limits, ops } = transport.capabilities();
                const open = (0,windowedSource/* decideOpen */.R6)({
                    size: r.size,
                    readOnly: r.readOnly,
                    analysis: r.analysis,
                    ops,
                    hashCapBytes: limits.hashCapBytes,
                    rangeRead: rangeRead.current
                });
                dispatchRaw({
                    type: "markLoaded",
                    path: b.path,
                    token: r.token,
                    readOnly: open.readOnly,
                    source: open.source
                });
                const origin = sources.current.get(b.path);
                sources.current.delete(b.path);
                if (origin) analytics.emit("files.tab.open", {
                    kind: origin.pinned ? "pinned" : "preview",
                    source: origin.source,
                    ext_class: (0,events/* extClass */.KG)(b.path),
                    size: (0,events/* bucketBytes */.gi)(r.size)
                });
            }, (err)=>{
                if (!ctl.signal.aborted) dispatchRaw({
                    type: "markFailed",
                    path: b.path,
                    reason: classifyFailure(err)
                });
            }).finally(()=>inflight.current.delete(key));
        }
    }, [
        state.buffers,
        transport,
        analytics
    ]);
    (0,react.useEffect)(()=>{
        const map = inflight.current;
        return ()=>{
            map.forEach((c)=>c.abort());
            map.clear();
        };
    }, []);
    // Persist paths, active index and positions; text is never stored.
    const flush = (0,react.useCallback)(()=>{
        if (!writePersisted(sessionId, stateRef.current)) setStorageFailed(true);
    }, [
        sessionId
    ]);
    (0,react.useEffect)(()=>{
        const t = setTimeout(flush, PERSIST_DEBOUNCE_MS);
        return ()=>clearTimeout(t);
    }, [
        state.buffers,
        state.activeIndex,
        flush
    ]);
    (0,react.useEffect)(()=>{
        window.addEventListener("pagehide", flush);
        return ()=>{
            window.removeEventListener("pagehide", flush);
            flush();
        };
    }, [
        flush
    ]);
    // Back-compat with the single-path props (VCS cross-link).
    const selected = selectedPath(state);
    (0,react.useEffect)(()=>{
        if (initialSelectedPath && initialSelectedPath !== selected) {
            noteSource(initialSelectedPath, "deeplink", false);
            dispatchRaw({
                type: "open",
                path: initialSelectedPath,
                sessionId
            });
        }
    // Only a changed prop opens a file; selection changes must not re-open the old prop.
    }, [
        initialSelectedPath
    ]); // eslint-disable-line react-hooks/exhaustive-deps
    const notified = (0,react.useRef)(undefined);
    (0,react.useEffect)(()=>{
        if (notified.current === selected || notified.current === undefined && selected === null) return;
        notified.current = selected;
        onSelectedPathChange === null || onSelectedPathChange === void 0 ? void 0 : onSelectedPathChange(selected);
    }, [
        selected,
        onSelectedPathChange
    ]);
    const actions = (0,react.useMemo)(()=>({
            open: function(path) {
                let opts = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : {};
                var _opts_source;
                noteSource(path, (_opts_source = opts.source) !== null && _opts_source !== void 0 ? _opts_source : "tree", !!opts.pinned);
                dispatchRaw({
                    type: "open",
                    path,
                    sessionId,
                    pinned: opts.pinned
                });
            },
            jump: function(path, line) {
                let opts = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : {};
                var _opts_source;
                noteSource(path, (_opts_source = opts.source) !== null && _opts_source !== void 0 ? _opts_source : "jumplist", !!opts.pinned);
                var _opts_col;
                dispatchRaw({
                    type: "jump",
                    path,
                    sessionId,
                    line,
                    col: (_opts_col = opts.col) !== null && _opts_col !== void 0 ? _opts_col : 0,
                    pinned: opts.pinned,
                    chain: opts.chain
                });
            },
            move: (path, delta)=>dispatchRaw({
                    type: "move",
                    path,
                    delta
                }),
            adoptToken: (path, token)=>dispatchRaw({
                    type: "adoptToken",
                    path,
                    token
                }),
            revealed: (seq)=>dispatchRaw({
                    type: "revealed",
                    seq
                }),
            close: function(path) {
                let discardConfirmed = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : false;
                return dispatchRaw({
                    type: "close",
                    path,
                    discardConfirmed
                });
            },
            activate: (path)=>dispatchRaw({
                    type: "activate",
                    path
                }),
            pin: (path)=>dispatchRaw({
                    type: "pin",
                    path
                }),
            unpin: (path)=>dispatchRaw({
                    type: "unpin",
                    path
                }),
            navBack: ()=>dispatchRaw({
                    type: "navBack"
                }),
            navForward: ()=>dispatchRaw({
                    type: "navForward"
                }),
            recordViewState: (path, viewState)=>dispatchRaw({
                    type: "recordViewState",
                    path,
                    viewState
                }),
            setMode: (path, mode)=>dispatchRaw({
                    type: "setMode",
                    path,
                    mode
                }),
            markDirty: (path, dirty)=>dispatchRaw({
                    type: "markDirty",
                    path,
                    dirty
                }),
            reopenClosed: ()=>dispatchRaw({
                    type: "reopenClosed"
                }),
            clearNotice: ()=>dispatchRaw({
                    type: "clearNotice"
                })
        }), [
        sessionId,
        noteSource
    ]);
    return {
        state,
        dispatch,
        active: activeBuffer(state),
        selectedPath: selected,
        mru: (0,react.useMemo)(()=>mruPaths(state), [
            state
        ]),
        /** True once localStorage failed: show the one-time "Tab layout will not be remembered" status. */ storageUnavailable: storageFailed,
        ...actions
    };
}

;// ./src/lib/hooks/useStatusModel.ts
/* __next_internal_client_entry_do_not_use__ useStatusModel auto */ 
const CURSOR_POLL_MS = 250;
/** Cursor position and line endings for the status bar; the engine port has no change event, so it polls cheaply. */ function useStatusModel(param) {
    let { inspect, eol } = param;
    const [model, setModel] = (0,react.useState)({
        line: 1,
        col: 1,
        eol: null
    });
    (0,react.useEffect)(()=>{
        const read = ()=>{
            const at = inspect();
            if (!at) return;
            setModel((m)=>m.line === at.line && m.col === at.col + 1 ? m : {
                    ...m,
                    line: at.line,
                    col: at.col + 1
                });
        };
        setModel((m)=>({
                ...m,
                eol: eol()
            }));
        read();
        const id = setInterval(read, CURSOR_POLL_MS);
        return ()=>clearInterval(id);
    }, [
        inspect,
        eol
    ]);
    return model;
}

;// ./src/lib/hooks/useFilesV2.ts
/* __next_internal_client_entry_do_not_use__ useFilesV2 auto */ 








const RECENT_LIMIT = 8;
const baseName = (path)=>path.slice(path.lastIndexOf("/") + 1);
/** Everything the buffered (files:viewer-v2) Files tab needs, composed once so FilesTab stays a thin root. */ function useFilesV2(sessionId, sessionRunning, options) {
    const buffers = useOpenBuffers(sessionId, options);
    const feedback = useFilesFeedback();
    const bridge = useEditorBridge(buffers, {
        announce: feedback.announceInfo
    });
    const freshness = useBufferFreshness({
        active: buffers.active,
        enabled: true,
        sessionRunning,
        onAdoptToken: buffers.adoptToken
    });
    const { active, close } = buffers;
    const { stale, resolve } = freshness;
    const { back, forward, setOverlay, reload, swapView, text } = bridge;
    const accessible = useAccessibleView({
        textLength: (0,react.useCallback)(()=>{
            var _text;
            var _text_length;
            return (_text_length = (_text = text()) === null || _text === void 0 ? void 0 : _text.length) !== null && _text_length !== void 0 ? _text_length : 0;
        }, [
            text
        ]),
        swapView
    });
    const transport = (0,useFileTransport/* useFileTransport */.J7)();
    const status = useStatusModel(bridge);
    const { announceSuccess, announceProgress, announceError } = feedback;
    const activePathRef = (0,react.useRef)(undefined);
    activePathRef.current = active === null || active === void 0 ? void 0 : active.path;
    const shortcuts = (0,react.useMemo)(()=>({
            onBack: ()=>back("keyboard"),
            onForward: ()=>forward("keyboard"),
            onGoToLine: ()=>setOverlay("goto"),
            onCloseTab: ()=>{
                if (active) close(active.path);
            }
        }), [
        back,
        forward,
        setOverlay,
        active,
        close
    ]);
    const onStaleAction = (0,react.useCallback)((action)=>{
        if (action === "reload" && stale) {
            reload(stale.token);
            announceSuccess("reloaded", baseName(stale.path));
        }
        resolve(action);
    }, [
        stale,
        reload,
        resolve,
        announceSuccess
    ]);
    // The palette's "Reload from disk": stat first so the adopted token is the one the new text carries.
    // A failure never touches the buffer; it raises a banner with a way out (ux.md 8.5).
    const [reloadFailed, setReloadFailed] = (0,react.useState)(null);
    const reloadFromDisk = (0,react.useCallback)(()=>{
        if (!active || active.status !== "loaded") return;
        const { sessionId, path } = active;
        announceProgress("Checking the file on disk...");
        setReloadFailed(null);
        // The user may switch or close the tab while stat is in flight; a late result must not touch another tab.
        const stillActive = ()=>activePathRef.current === path;
        transport.stat({
            sessionId,
            path
        }).then((r)=>{
            if (stillActive()) {
                reload(r.token);
                announceSuccess("reloaded", baseName(path));
            }
        }, (err)=>{
            if (!stillActive()) return;
            announceError("Could not check the file: ".concat((0,PendingState/* failureClass */.fI)(classifyFailure(err))));
            setReloadFailed({
                path,
                seq: Date.now()
            });
        });
    }, [
        active,
        transport,
        reload,
        announceSuccess,
        announceProgress,
        announceError
    ]);
    const dirty = (active === null || active === void 0 ? void 0 : active.status) === "loaded" && active.dirty;
    const paletteCommands = (0,react.useMemo)(()=>[
            {
                id: "goto_line",
                label: "Go to line",
                shortcut: "Ctrl+G",
                disabledReason: active ? undefined : "no file open",
                run: ()=>setOverlay("goto")
            },
            {
                id: "toggle_accessible_view",
                label: accessible.on ? "Switch to code editor" : "Switch to accessible view",
                disabledReason: active ? undefined : "no file open",
                run: ()=>accessible.toggle("palette")
            },
            {
                id: "reload",
                label: "Reload from disk",
                disabledReason: (active === null || active === void 0 ? void 0 : active.status) === "loaded" ? undefined : "no file open",
                run: reloadFromDisk
            },
            {
                id: "save",
                label: "Save",
                shortcut: "Ctrl+S",
                disabledReason: dirty ? "saving is not available yet" : "no changes",
                run: ()=>undefined
            }
        ], [
        active,
        dirty,
        setOverlay,
        reloadFromDisk,
        accessible
    ]);
    const viewerSource = (active === null || active === void 0 ? void 0 : active.status) === "loaded" ? active.source : undefined;
    const recentPaths = (0,react.useMemo)(()=>buffers.mru.slice(0, RECENT_LIMIT), [
        buffers.mru
    ]);
    return {
        buffers,
        bridge,
        viewerSource,
        accessible,
        stale,
        sessionRunning,
        shortcuts,
        recentPaths,
        onStaleAction,
        feedback,
        status,
        paletteCommands,
        reloadFailed,
        reloadFromDisk,
        clearReloadFailed: ()=>setReloadFailed(null)
    };
}

// EXTERNAL MODULE: ./src/lib/hooks/filesChords.ts
var filesChords = __webpack_require__(89107);
;// ./src/lib/hooks/useFilesShortcuts.ts
/* __next_internal_client_entry_do_not_use__ useFilesShortcuts auto */ 

/**
 * Files-tab chords (the table and matcher live in filesChords.ts), active only while the tab is visible
 * (`offsetParent` is null under an inactive `display: none` panel). Ctrl/Cmd+F and +P need the tree pane;
 * the rest do not. A chord with no handler is left to the browser.
 */ function useFilesShortcuts(options) {
    const live = (0,react.useRef)(options);
    live.current = options;
    (0,react.useEffect)(()=>{
        const handleKeyDown = (e)=>{
            const o = live.current;
            const input = o.searchInputRef.current;
            if (!input || input.offsetParent === null) return;
            const chord = (0,filesChords/* matchChord */.sn)(e);
            if (!chord) return;
            const run = {
                back: o.onBack,
                forward: o.onForward,
                closeTab: o.onCloseTab,
                gotoLine: o.onGoToLine,
                search: o.onSearch,
                nextHit: o.onNextHit,
                prevHit: o.onPrevHit,
                treeFilter: o.collapsed ? undefined : ()=>input.focus(),
                quickOpen: o.collapsed ? undefined : o.onQuickOpen,
                save: undefined
            }[chord.id];
            if (!run) return;
            e.preventDefault();
            run();
        };
        window.addEventListener("keydown", handleKeyDown);
        return ()=>window.removeEventListener("keydown", handleKeyDown);
    }, []);
}

// EXTERNAL MODULE: ./src/lib/nav/index.ts
var nav = __webpack_require__(41078);
;// ./src/lib/hooks/useFilesDeepLink.ts
/* __next_internal_client_entry_do_not_use__ useFilesDeepLink auto */ 


/**
 * The only code that writes `#file=` or touches history for the Files tab. Selection changes use
 * `replaceState(history.state, ...)` so a pushed pane entry survives.
 */ function useFilesDeepLink(param) {
    let { initialSelectedPath, tree, selectedPath, onJump } = param;
    (0,react.useEffect)(()=>{
        var _tree_current;
        if (initialSelectedPath) (_tree_current = tree.current) === null || _tree_current === void 0 ? void 0 : _tree_current.revealPath(initialSelectedPath);
    }, [
        initialSelectedPath,
        tree
    ]);
    const jumped = (0,react.useRef)(false);
    (0,react.useEffect)(()=>{
        if (jumped.current || !onJump) return;
        jumped.current = true;
        const link = parseDeepLink(window.location.search, "");
        if (link) onJump(link.file, link.line);
    }, [
        onJump
    ]);
    const first = (0,react.useRef)(true);
    (0,react.useEffect)(()=>{
        const hash = fileHash(selectedPath);
        // Skip the mount pass when nothing is selected, so an unrelated hash is not erased.
        if (first.current && !selectedPath) {
            first.current = false;
            return;
        }
        first.current = false;
        if (window.location.hash === hash) return;
        (0,nav/* replaceHash */.nQ)(hash);
    }, [
        selectedPath
    ]);
}

;// ./src/lib/hooks/useResizablePanel.ts
/* __next_internal_client_entry_do_not_use__ useResizablePanel auto */ 
function readStoredNumber(key, fallback, min) {
    if (typeof localStorage === "undefined") return fallback;
    try {
        const v = localStorage.getItem(key);
        if (v !== null) {
            const n = Number(v);
            if (!isNaN(n)) return Math.max(min, n);
        }
    } catch (e) {
    // ignore
    }
    return fallback;
}
function readStoredBool(key, fallback) {
    if (typeof localStorage === "undefined") return fallback;
    try {
        const v = localStorage.getItem(key);
        if (v !== null) return v === "true";
    } catch (e) {
    // ignore
    }
    return fallback;
}
function useResizablePanel(param) {
    let { storageKey, defaultWidth, minWidth, maxWidthFraction, step = 16, largeStep = 64 } = param;
    const collapsedKey = storageKey + "Collapsed";
    const [width, setWidth] = (0,react.useState)(()=>readStoredNumber(storageKey, defaultWidth, minWidth));
    const [collapsed, setCollapsed] = (0,react.useState)(()=>readStoredBool(collapsedKey, false));
    const containerRef = (0,react.useRef)(null);
    // `aria-valuemax` reads the container width at render; the ref is empty on the first render, so re-render once
    // the container is measured and whenever it resizes.
    const [, remeasure] = (0,react.useReducer)((n)=>n + 1, 0);
    const isDraggingRef = (0,react.useRef)(false);
    const lastWidthRef = (0,react.useRef)(readStoredNumber(storageKey, defaultWidth, minWidth));
    const rafRef = (0,react.useRef)(null);
    const pendingWidthRef = (0,react.useRef)(null);
    // Persist width changes to localStorage
    (0,react.useEffect)(()=>{
        try {
            localStorage.setItem(storageKey, String(width));
        } catch (e) {
        // ignore
        }
    }, [
        storageKey,
        width
    ]);
    // Persist collapsed changes to localStorage
    (0,react.useEffect)(()=>{
        try {
            localStorage.setItem(collapsedKey, String(collapsed));
        } catch (e) {
        // ignore
        }
    }, [
        collapsedKey,
        collapsed
    ]);
    // Clamp width against the actual container size after mount — guards against
    // a stored value that exceeds maxWidthFraction on a narrower viewport.
    (0,react.useEffect)(()=>{
        const container = containerRef.current;
        if (!container) return;
        const maxWidth = container.clientWidth * maxWidthFraction;
        if (maxWidth > 0 && width > maxWidth) {
            setWidth(maxWidth);
        }
    }, [
        maxWidthFraction
    ]); // intentionally omits width — runs once on mount per fraction change
    (0,react.useEffect)(()=>{
        const container = containerRef.current;
        if (!container) return;
        remeasure();
        if (typeof ResizeObserver === "undefined") return;
        const observer = new ResizeObserver(()=>remeasure());
        observer.observe(container);
        return ()=>observer.disconnect();
    }, []);
    const onPointerDown = (0,react.useCallback)((e)=>{
        e.preventDefault();
        e.currentTarget.setPointerCapture(e.pointerId);
        isDraggingRef.current = true;
    }, []);
    const onPointerMove = (0,react.useCallback)((e)=>{
        if (!isDraggingRef.current) return;
        const container = containerRef.current;
        if (!container) return;
        const rect = container.getBoundingClientRect();
        const rawWidth = e.clientX - rect.left;
        const maxWidth = container.clientWidth * maxWidthFraction;
        const clamped = Math.max(minWidth, Math.min(maxWidth, rawWidth));
        pendingWidthRef.current = clamped;
        if (rafRef.current === null) {
            rafRef.current = requestAnimationFrame(()=>{
                if (pendingWidthRef.current !== null) {
                    setWidth(pendingWidthRef.current);
                    pendingWidthRef.current = null;
                }
                rafRef.current = null;
            });
        }
    }, [
        minWidth,
        maxWidthFraction
    ]);
    const onPointerUp = (0,react.useCallback)((e)=>{
        isDraggingRef.current = false;
        e.currentTarget.releasePointerCapture(e.pointerId);
        if (rafRef.current !== null) {
            cancelAnimationFrame(rafRef.current);
            rafRef.current = null;
        }
    }, []);
    const maxPx = ()=>{
        var _containerRef_current;
        var _containerRef_current_clientWidth;
        return ((_containerRef_current_clientWidth = (_containerRef_current = containerRef.current) === null || _containerRef_current === void 0 ? void 0 : _containerRef_current.clientWidth) !== null && _containerRef_current_clientWidth !== void 0 ? _containerRef_current_clientWidth : 0) * maxWidthFraction;
    };
    // The separator pattern: Arrow keys step, Shift+Arrow takes a large step, Home/End jump to the drag bounds.
    const onKeyDown = (0,react.useCallback)((e)=>{
        const max = maxPx();
        const dir = e.key === "ArrowRight" ? 1 : e.key === "ArrowLeft" ? -1 : 0;
        if (!dir && e.key !== "Home" && e.key !== "End") return;
        e.preventDefault();
        setWidth((w)=>{
            const raw = e.key === "Home" ? minWidth : e.key === "End" ? max : w + dir * (e.shiftKey ? largeStep : step);
            return Math.max(minWidth, max > 0 ? Math.min(max, raw) : raw);
        });
    }, // maxPx reads refs only
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [
        minWidth,
        maxWidthFraction,
        step,
        largeStep
    ]);
    const collapse = (0,react.useCallback)(()=>{
        lastWidthRef.current = width;
        setCollapsed(true);
    }, [
        width
    ]);
    const expand = (0,react.useCallback)(()=>{
        setCollapsed(false);
        setWidth(lastWidthRef.current || defaultWidth);
    }, [
        defaultWidth
    ]);
    return {
        width,
        collapsed,
        containerRef,
        handleProps: {
            onPointerDown,
            onPointerMove,
            onPointerUp,
            onPointerCancel: onPointerUp,
            onKeyDown,
            role: "separator",
            "aria-orientation": "vertical",
            "aria-label": "Resize file panel",
            tabIndex: 0,
            "aria-valuenow": Math.round(width),
            "aria-valuemin": minWidth,
            "aria-valuemax": Math.round(Math.max(maxPx(), minWidth))
        },
        collapse,
        expand
    };
}

;// ./src/lib/files/chooseFilesLayout.ts
const MOBILE_CONTAINER_MAX = 768;
const PHONE_SHORT_SIDE_MAX = 600;
/** ux.md section 0: mobile for a narrow container or a phone-class touch device, else desktop. */ function chooseFilesLayout(param) {
    let { containerWidth, pointerCoarse, screenShortSide } = param;
    if (containerWidth < MOBILE_CONTAINER_MAX) return "mobile";
    return pointerCoarse && screenShortSide < PHONE_SHORT_SIDE_MAX ? "mobile" : "desktop";
}

// EXTERNAL MODULE: ./src/lib/hooks/useCoarsePointer.ts
var useCoarsePointer = __webpack_require__(69250);
;// ./src/lib/hooks/useFilesLayout.ts
/* __next_internal_client_entry_do_not_use__ useFilesLayout auto */ 


const screenShortSide = ()=>{
    var _window_screen, _window_screen1;
    var _window_screen_width, _window_screen_height;
    return  false ? 0 : Math.min((_window_screen_width = (_window_screen = window.screen) === null || _window_screen === void 0 ? void 0 : _window_screen.width) !== null && _window_screen_width !== void 0 ? _window_screen_width : 0, (_window_screen_height = (_window_screen1 = window.screen) === null || _window_screen1 === void 0 ? void 0 : _window_screen1.height) !== null && _window_screen_height !== void 0 ? _window_screen_height : 0);
};
const initialWidth = ()=> false ? 0 : window.innerWidth;
/** The Files tab's layout and the container width it was chosen from (`screen` is read once; a phone's never changes). */ function useFilesLayout(containerRef) {
    const pointerCoarse = (0,useCoarsePointer/* useCoarsePointer */.O)();
    const [containerWidth, setContainerWidth] = (0,react.useState)(initialWidth);
    const [short] = (0,react.useState)(screenShortSide);
    (0,react.useLayoutEffect)(()=>{
        const el = containerRef.current;
        if (!el) return;
        // A zero width means "not laid out" (jsdom, display:none): keep the last answer.
        const measure = (w)=>{
            if (w > 0) setContainerWidth(w);
        };
        measure(el.getBoundingClientRect().width);
        if (typeof ResizeObserver === "undefined") return;
        const ro = new ResizeObserver((entries)=>measure(entries[entries.length - 1].contentRect.width));
        ro.observe(el);
        return ()=>ro.disconnect();
    }, [
        containerRef
    ]);
    return {
        layout: chooseFilesLayout({
            containerWidth,
            pointerCoarse,
            screenShortSide: short
        }),
        containerWidth
    };
}

;// ./src/lib/hooks/useMobilePane.ts
/* __next_internal_client_entry_do_not_use__ useMobilePane auto */ 


const KEY = "ssqFilesPane";
/** How many editor entries we pushed, as carried by a history state; 0 for any state that is not ours. */ const depthOf = (state)=>{
    const v = state === null || state === void 0 ? void 0 : state[KEY];
    return typeof v === "number" ? v : 0;
};
/**
 * The mobile nav-stack (ux.md 7.4). Entering Search or the editor pushes one history entry over the pane
 * it came from, so browser Back pops it exactly like the top-bar arrow and never leaves the session
 * (editor -> results -> files). The entry carries its depth, which tells a pop from a forward and from a
 * sheet's own entry (same depth); `stack[d]` is the pane shown at depth d and survives a pop so Forward
 * can replay it. If `pushState` throws (router race) the pane falls back to in-memory state. A keyboard
 * resize is not a navigation: nothing here listens to the viewport.
 */ function useMobilePane(active) {
    const analytics = (0,filesAnalytics/* useFilesAnalytics */.R)({
        component: "MobileFilesLayout"
    });
    const [pane, setPaneState] = (0,react.useState)("files");
    const paneRef = (0,react.useRef)("files");
    const origin = (0,react.useRef)("files");
    const depth = (0,react.useRef)(0);
    const stack = (0,react.useRef)([
        "files"
    ]);
    const viaOnPop = (0,react.useRef)(null);
    const show = (0,react.useCallback)((to, via)=>{
        const from = paneRef.current;
        if (from === to) return;
        analytics.emit("files.mobile.pane_switch", {
            from,
            to,
            via
        });
        paneRef.current = to;
        var _stack_current_;
        origin.current = (_stack_current_ = stack.current[depth.current - 1]) !== null && _stack_current_ !== void 0 ? _stack_current_ : "files";
        setPaneState(to);
    }, [
        analytics
    ]);
    const go = (0,react.useCallback)((to, via)=>{
        if (!active || to === paneRef.current) return;
        if (to === "files" && depth.current > 0) {
            viaOnPop.current = via;
            window.history.go(-depth.current);
            return;
        }
        if (to === "files") {
            show(to, via);
            return;
        }
        const below = paneRef.current;
        try {
            (0,nav/* pushHistoryMarker */.tk)(KEY, depth.current + 1);
            depth.current += 1;
            stack.current = [
                ...stack.current.slice(0, depth.current),
                to
            ];
        } catch (e) {
            // router race: stay on in-memory state
            origin.current = below;
        }
        show(to, via);
    }, [
        active,
        show
    ]);
    /** The top-bar arrow: pops this pane's entry and returns to the pane below it. */ const back = (0,react.useCallback)(function() {
        let via = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : "back_arrow";
        const here = paneRef.current;
        if (here === "files") return;
        if (depth.current > 0) {
            viaOnPop.current = via;
            window.history.back();
        } else show(here === "editor" ? origin.current : "files", via);
    }, [
        show
    ]);
    (0,react.useEffect)(()=>{
        if (!active) return;
        const onPopState = (e)=>{
            const next = depthOf(e.state);
            const before = depth.current;
            if (next === before) return;
            depth.current = next;
            var _viaOnPop_current;
            const via = (_viaOnPop_current = viaOnPop.current) !== null && _viaOnPop_current !== void 0 ? _viaOnPop_current : "system_back";
            viaOnPop.current = null;
            var _stack_current_next;
            show((_stack_current_next = stack.current[next]) !== null && _stack_current_next !== void 0 ? _stack_current_next : "files", via);
        };
        window.addEventListener("popstate", onPopState);
        return ()=>window.removeEventListener("popstate", onPopState);
    }, [
        active,
        show
    ]);
    return {
        pane,
        origin: origin.current,
        go,
        back
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FTreeResizeHandle.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll85eHZoMDMwIHsKICB3aWR0aDogOHB4OwogIG1hcmdpbi1sZWZ0OiAtNHB4OwogIG1hcmdpbi1yaWdodDogLTRweDsKICBmbGV4LXNocmluazogMDsKICBjdXJzb3I6IGNvbC1yZXNpemU7CiAgYmFja2dyb3VuZDogdHJhbnNwYXJlbnQ7CiAgdHJhbnNpdGlvbjogYmFja2dyb3VuZCAwLjE1czsKICBwb3NpdGlvbjogcmVsYXRpdmU7CiAgei1pbmRleDogMTsKfQouXzl4dmgwMzA6OmFmdGVyIHsKICBjb250ZW50OiAiIjsKICBwb3NpdGlvbjogYWJzb2x1dGU7CiAgdG9wOiAwOwogIGJvdHRvbTogMDsKICBsZWZ0OiA1MCU7CiAgdHJhbnNmb3JtOiB0cmFuc2xhdGVYKC01MCUpOwogIHdpZHRoOiAycHg7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnplKTsKICBvcGFjaXR5OiAwOwogIHRyYW5zaXRpb246IG9wYWNpdHkgMC4xNXM7Cn0KLl85eHZoMDMwOmZvY3VzLXZpc2libGU6OmFmdGVyIHsKICBvcGFjaXR5OiAxOwogIHdpZHRoOiAzcHg7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnpsKTsKfQouXzl4dmgwMzA6aG92ZXI6OmFmdGVyIHsKICBvcGFjaXR5OiAxOwp9Ci5fOXh2aDAzMDphY3RpdmU6OmFmdGVyIHsKICBvcGFjaXR5OiAxOwp9CkBtZWRpYSAobWF4LXdpZHRoOiA3NjhweCkgewogIC5fOXh2aDAzMCB7CiAgICBkaXNwbGF5OiBub25lOwogIH0KfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FTreeResizeHandle_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll85eHZoMDMwIHsKICB3aWR0aDogOHB4OwogIG1hcmdpbi1sZWZ0OiAtNHB4OwogIG1hcmdpbi1yaWdodDogLTRweDsKICBmbGV4LXNocmluazogMDsKICBjdXJzb3I6IGNvbC1yZXNpemU7CiAgYmFja2dyb3VuZDogdHJhbnNwYXJlbnQ7CiAgdHJhbnNpdGlvbjogYmFja2dyb3VuZCAwLjE1czsKICBwb3NpdGlvbjogcmVsYXRpdmU7CiAgei1pbmRleDogMTsKfQouXzl4dmgwMzA6OmFmdGVyIHsKICBjb250ZW50OiAiIjsKICBwb3NpdGlvbjogYWJzb2x1dGU7CiAgdG9wOiAwOwogIGJvdHRvbTogMDsKICBsZWZ0OiA1MCU7CiAgdHJhbnNmb3JtOiB0cmFuc2xhdGVYKC01MCUpOwogIHdpZHRoOiAycHg7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnplKTsKICBvcGFjaXR5OiAwOwogIHRyYW5zaXRpb246IG9wYWNpdHkgMC4xNXM7Cn0KLl85eHZoMDMwOmZvY3VzLXZpc2libGU6OmFmdGVyIHsKICBvcGFjaXR5OiAxOwogIHdpZHRoOiAzcHg7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnpsKTsKfQouXzl4dmgwMzA6aG92ZXI6OmFmdGVyIHsKICBvcGFjaXR5OiAxOwp9Ci5fOXh2aDAzMDphY3RpdmU6OmFmdGVyIHsKICBvcGFjaXR5OiAxOwp9CkBtZWRpYSAobWF4LXdpZHRoOiA3NjhweCkgewogIC5fOXh2aDAzMCB7CiAgICBkaXNwbGF5OiBub25lOwogIH0KfQ_22_7D = __webpack_require__(11683);
;// ./src/components/sessions/TreeResizeHandle.css.ts


var hitTarget = '_9xvh030';
;// ./src/components/sessions/TreeResizeHandle.tsx
/* __next_internal_client_entry_do_not_use__ TreeResizeHandle auto */ 

function TreeResizeHandle(props) {
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: hitTarget,
        style: {
            touchAction: "none"
        },
        ...props
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react-dom/index.js
var react_dom = __webpack_require__(80883);
// EXTERNAL MODULE: ./src/lib/utils/fileIcons.ts
var fileIcons = __webpack_require__(96426);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FQuickOpenPalette.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA71UzW7iMBC-8xRzWQmkukr4a-te9k0qE0_ILI5t2QZcVrz7yk4KCd2Wy2rFBY0n8_348zy-lVGZlkIBvycA1ngKZDSHmiLK1wkAaY-BQ5H-b0S12zqz15KD227EtHjIv8fVLB2fGGmJkUNZFLlfkrdKvHOoFcZUEIq2mlHA1ndF5oNwIR392vtA9TurjA6oA4cKdUCXjqyQkvSWBWM5tCJOn218ANSHqRc1MuFQsEwzdTzAs42z2evkPHn8EFdmcUP2B-GmjL2VL3iau1MpM_-NcRIdh9JG8EaRvGnDQRdzQtLe84SWqkeSoeHQkp6u1kXi91IcjvkDc0BXK3Pk0JCUqLshkflGyFQt0gxYzG0cm7oci5hnET1OWRQ_hpS10Zix9kGRxmuh945DmeaX645tbXRgnk7IoVx2paE7wQntrXCo89VURhnHgXSDjsLAg40JwbT3DYsJK7O4fBjH2ji3SlTYGCXRZaE96HjgfGzJIne2IrIGadsEDovk_dB09s5B7IMZebG0MQV6MGmZJ31MWfZDvs_vNaBbYS9BuIAU2fLs3975pMUa6r4YAvMmUb0X0N1Y-Oq_0v2W2dNslLuvotAO25ip67xVWGnHQVgPQz7vdQWMgWUtQxXd-mgc6R2_uc2nPCWH_Nh7tO420jD4i_4tqLyyvnqpGfx6gEqR9eTzm28oIPNWVPnBHZ2wIxrPVxo9ZNk7_FW2P6vq4n0x5Cbd_4joS7f8L6tinVbF_D7Xz2b-7arOk58tShIw7RPFoTLCeZxl2JsFNx7b76vz5PwHOalzJ6oGAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FQuickOpenPalette_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA71UzW7iMBC_8xRzWQmkukr4a_te9k0qE0_ILI5t2QZcVrz7yk4KCd2Wy2rFBY0n8_348zy_lVGZlkIBvycA1ngKZDSHmiLK1wkAaY_BQ5H_b0S12zqz15KD227EtHjIv8fVLB2fGGmJkUNZFLlfkrdKvHOoFcZUEIq2mlHA1ndF5oNwIR392vtA9TurjA6oA4cKdUCXjqyQkvSWBWM5tCJOn218ANSHqRc1MuFQsEwzdTzAs42z2evkPHn8EFdmcUP2B_GmjL2VL3iau1MpM_NcRIdh9JG8EaRvGnDQRdzQtLe84SWqkeSoeHQkp6u1kXi91IcjvkDc0BXK3Pk0JCUqLshkflGyFQt0gxYzG0cm7oci5hnET1OWRQ_hpS10Zix9kGRxmuh945DmeaX645tbXRgnk7IoVx2paE7wQntrXCo89VURhnHgXSDjsLAg40JwbT3DYsJK7O4fBjH2ji3SlTYGCXRZaE96HjgfGzJIne2IrIGadsEDovk_dB09s5B7IMZebG0MQV6MGmZJ31MWfZDvs_vNaBbYS9BuIAU2fLs3975pMUa6r4YAvMmUb0X0N1Y_Oq_0v2W2dNslLuvotAO25ip67xVWGnHQVgPQz7vdQWMgWUtQxXd_mgc6R2_uc2nPCWH_Nh7tO420jD4i_4tqLyyvnqpGfx6gEqR9eTzm28oIPNWVPnBHZ2wIxrPVxo9ZNk7_FW2P6vq4n0x5Cbd_4joS7f8L6tinVbF_D7Xz2b_7arOk58tShIw7RPFoTLCeZxl2JsFNx7b76vz5PwHOalzJ6oGAAA_22_7D = __webpack_require__(52073);
;// ./src/components/sessions/QuickOpenPalette.css.ts


var backdrop = '_1xlomit0';
var card = '_1xlomit1';
var QuickOpenPalette_css_emptyState = '_1xlomit9';
var resultIcon = '_1xlomit6';
var resultItem = '_1xlomit4';
var resultItemActive = '_1xlomit5';
var resultName = '_1xlomit7';
var resultPath = '_1xlomit8';
var resultsList = '_1xlomit3';
var QuickOpenPalette_css_searchInput = '_1xlomit2';
// EXTERNAL MODULE: ./src/styles/a11y.css.ts
var a11y_css = __webpack_require__(26946);
;// ./src/components/sessions/QuickOpenPalette.tsx
/* __next_internal_client_entry_do_not_use__ QuickOpenPalette,default auto */ 







// ---------------------------------------------------------------------------
// Fuse.js — optional progressive enhancement
// ---------------------------------------------------------------------------
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let FuseClass = null;
try {
    var _require_default;
    // Dynamic require so the bundle never fails when fuse.js is absent.
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    FuseClass = (_require_default = (__webpack_require__(59852)["default"])) !== null && _require_default !== void 0 ? _require_default : __webpack_require__(59852);
} catch (e) {
// fuse.js not installed — fall back to raw server results
}
// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------
function getFileIcon(node) {
    if (node.isDir) return "📁";
    return (0,fileIcons/* getFileIcon */.I)(node.name);
}
function dirPart(path) {
    const idx = path.lastIndexOf("/");
    return idx > 0 ? path.slice(0, idx) : "";
}
// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------
function QuickOpenPalette(param) {
    let { sessionId, baseUrl, recentPaths, onSelect, onClose, commands } = param;
    const [query, setQuery] = (0,react.useState)("");
    const [notice, setNotice] = (0,react.useState)("");
    const analytics = (0,filesAnalytics/* useFilesAnalytics */.R)({
        component: "QuickOpenPalette",
        sessionId
    });
    const commandMode = !!commands && query.startsWith(">");
    const listId = (0,react.useId)();
    const optionId = (i)=>"".concat(listId, "-opt-").concat(i);
    const matchedCommands = (0,react.useMemo)(()=>{
        if (!commandMode || !commands) return [];
        const needle = query.slice(1).trim().toLowerCase();
        return commands.filter((c)=>c.label.toLowerCase().includes(needle));
    }, [
        commandMode,
        commands,
        query
    ]);
    const [results, setResults] = (0,react.useState)(()=>recentPaths.map((p)=>{
            var _p_split_pop;
            return {
                path: p,
                name: (_p_split_pop = p.split("/").pop()) !== null && _p_split_pop !== void 0 ? _p_split_pop : p,
                isDir: false,
                size: BigInt(0),
                gitStatus: "",
                isSymlink: false,
                symlinkTarget: "",
                isIgnored: false
            };
        }));
    const [loading, setLoading] = (0,react.useState)(false);
    const [activeIndex, setActiveIndex] = (0,react.useState)(0);
    const inputRef = (0,react.useRef)(null);
    const prevFocusRef = (0,react.useRef)(null);
    const timerRef = (0,react.useRef)(null);
    const requestIdRef = (0,react.useRef)(0);
    const resultRefs = (0,react.useRef)([]);
    const cardRef = (0,react.useRef)(null);
    // Save and restore focus
    (0,react.useEffect)(()=>{
        var _inputRef_current;
        prevFocusRef.current = document.activeElement;
        (_inputRef_current = inputRef.current) === null || _inputRef_current === void 0 ? void 0 : _inputRef_current.focus();
        return ()=>{
            if (prevFocusRef.current && typeof prevFocusRef.current.focus === "function") {
                prevFocusRef.current.focus();
            }
        };
    }, []);
    // Debounced search
    (0,react.useEffect)(()=>{
        if (timerRef.current !== null) {
            clearTimeout(timerRef.current);
        }
        if (commandMode) {
            setActiveIndex(0);
            return;
        }
        if (query === "") {
            const recent = recentPaths.map((p)=>{
                var _p_split_pop;
                return {
                    path: p,
                    name: (_p_split_pop = p.split("/").pop()) !== null && _p_split_pop !== void 0 ? _p_split_pop : p,
                    isDir: false,
                    size: BigInt(0),
                    gitStatus: "",
                    isSymlink: false,
                    symlinkTarget: "",
                    isIgnored: false
                };
            });
            setResults(recent);
            setActiveIndex(0);
            return;
        }
        timerRef.current = setTimeout(()=>{
            const requestId = ++requestIdRef.current;
            setLoading(true);
            (0,useFileService/* searchFiles */._K)(sessionId, query, false, baseUrl).then((resp)=>{
                if (requestId !== requestIdRef.current) return;
                let files = resp.files;
                if (FuseClass && files.length > 0) {
                    const fuse = new FuseClass(files, {
                        keys: [
                            {
                                name: "name",
                                weight: 2
                            },
                            {
                                name: "path",
                                weight: 1
                            }
                        ],
                        threshold: 0.4,
                        ignoreLocation: true
                    });
                    files = fuse.search(query).map((r)=>r.item);
                }
                setResults(files);
                setActiveIndex(0);
            }).catch(()=>{
                if (requestId === requestIdRef.current) {
                    setResults([]);
                }
            }).finally(()=>{
                if (requestId === requestIdRef.current) {
                    setLoading(false);
                }
            });
        }, 300);
        return ()=>{
            if (timerRef.current !== null) clearTimeout(timerRef.current);
        };
    }, [
        query,
        commandMode,
        sessionId,
        baseUrl,
        recentPaths
    ]);
    // Clear stale DOM refs when the results list changes length
    (0,react.useEffect)(()=>{
        resultRefs.current = [];
    }, [
        results
    ]);
    // Scroll active item into view
    (0,react.useEffect)(()=>{
        const el = resultRefs.current[activeIndex];
        if (el && typeof el.scrollIntoView === "function") {
            el.scrollIntoView({
                block: "nearest"
            });
        }
    }, [
        activeIndex
    ]);
    const mode = commandMode ? "command" : "file";
    const rowCount = commandMode ? matchedCommands.length : results.length;
    const emit = (0,react.useCallback)(function(outcome) {
        let command = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : "none";
        return analytics.emit("files.nav.palette", {
            mode,
            outcome,
            command
        });
    }, [
        analytics,
        mode
    ]);
    const cancel = (0,react.useCallback)(()=>{
        emit("cancel");
        onClose();
    }, [
        emit,
        onClose
    ]);
    const runCommand = (0,react.useCallback)((cmd)=>{
        if (cmd.disabledReason) {
            setNotice("".concat(cmd.label, " is unavailable: ").concat(cmd.disabledReason));
            return;
        }
        emit("run", cmd.id);
        onClose();
        cmd.run();
    }, [
        emit,
        onClose
    ]);
    const pickFile = (0,react.useCallback)((path)=>{
        emit("run");
        onSelect(path);
    }, [
        emit,
        onSelect
    ]);
    const handleKeyDown = (0,react.useCallback)((e)=>{
        switch(e.key){
            case "ArrowDown":
                e.preventDefault();
                setActiveIndex((i)=>rowCount === 0 ? 0 : (i + 1) % rowCount);
                break;
            case "ArrowUp":
                e.preventDefault();
                setActiveIndex((i)=>rowCount === 0 ? 0 : (i - 1 + rowCount) % rowCount);
                break;
            case "Enter":
                e.preventDefault();
                if (commandMode) {
                    const cmd = matchedCommands[activeIndex];
                    if (cmd) runCommand(cmd);
                    else emit("empty");
                } else if (results[activeIndex]) {
                    pickFile(results[activeIndex].path);
                } else {
                    emit("empty");
                }
                break;
            case "Escape":
                e.preventDefault();
                cancel();
                break;
        }
    }, [
        rowCount,
        commandMode,
        matchedCommands,
        results,
        activeIndex,
        runCommand,
        pickFile,
        cancel,
        emit
    ]);
    const handleBackdropClick = (0,react.useCallback)((e)=>{
        if (e.target === e.currentTarget) {
            cancel();
        }
    }, [
        cancel
    ]);
    return /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: backdrop,
        onClick: handleBackdropClick,
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            ref: cardRef,
            className: card,
            onClick: (e)=>e.stopPropagation(),
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                    ref: inputRef,
                    className: QuickOpenPalette_css_searchInput,
                    type: "text",
                    role: "combobox",
                    "aria-expanded": "true",
                    "aria-controls": listId,
                    "aria-autocomplete": "list",
                    "aria-activedescendant": rowCount > 0 ? optionId(activeIndex) : undefined,
                    placeholder: loading ? "Searching…" : commands ? "Go to file… (> for commands)" : "Go to file…",
                    value: query,
                    onChange: (e)=>{
                        setQuery(e.target.value);
                        setNotice("");
                    },
                    onKeyDown: handleKeyDown,
                    "aria-label": "Quick open file",
                    autoComplete: "off",
                    spellCheck: false
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    "data-testid": "palette-count",
                    role: "status",
                    "aria-live": "polite",
                    className: a11y_css/* visuallyHidden */.Q,
                    children: notice || (commandMode ? "".concat(matchedCommands.length, " commands") : "".concat(results.length, " results"))
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    id: listId,
                    className: resultsList,
                    role: "listbox",
                    "aria-label": commandMode ? "Commands" : "File results",
                    children: [
                        commandMode && matchedCommands.length === 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: QuickOpenPalette_css_emptyState,
                            children: "No commands match"
                        }),
                        !commandMode && results.length === 0 && !loading && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: QuickOpenPalette_css_emptyState,
                            children: query ? "No files found" : "No recent files"
                        }),
                        commandMode && matchedCommands.map((cmd, i)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                id: optionId(i),
                                ref: (el)=>{
                                    resultRefs.current[i] = el;
                                },
                                className: i === activeIndex ? resultItemActive : resultItem,
                                role: "option",
                                "aria-selected": i === activeIndex,
                                "aria-disabled": cmd.disabledReason ? true : undefined,
                                onClick: ()=>runCommand(cmd),
                                onMouseEnter: ()=>setActiveIndex(i),
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: resultName,
                                        children: cmd.disabledReason ? "".concat(cmd.label, " (").concat(cmd.disabledReason, ")") : cmd.label
                                    }),
                                    cmd.shortcut && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: resultPath,
                                        children: cmd.shortcut
                                    })
                                ]
                            }, cmd.id)),
                        !commandMode && results.map((node, i)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                id: optionId(i),
                                ref: (el)=>{
                                    resultRefs.current[i] = el;
                                },
                                className: i === activeIndex ? resultItemActive : resultItem,
                                role: "option",
                                "aria-selected": i === activeIndex,
                                onClick: ()=>pickFile(node.path),
                                onMouseEnter: ()=>setActiveIndex(i),
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: resultIcon,
                                        children: getFileIcon(node)
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: resultName,
                                        children: node.name
                                    }),
                                    dirPart(node.path) && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: resultPath,
                                        title: node.path,
                                        children: dirPart(node.path)
                                    })
                                ]
                            }, node.path))
                    ]
                })
            ]
        })
    }), document.body);
}
/* harmony default export */ const sessions_QuickOpenPalette = ((/* unused pure expression or super */ null && (QuickOpenPalette)));

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2Ffiles%2FOutlinePanel.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA41V247bIBB9z1fwUm1cLRHOdZdVpf5HVVXYEJuGAAKcOKny7xVgJ-Sy3sgPFsNcz5kZJkU9pe0MgX8jALSy3HElMSCFVaJx7GMEgFMagxyhb_5geFU7DN50609HyCVlLQYz5I97Tl2NwWyK4vWWtLBm0WKJdrWXUW61IAcM1oIFJf-HlBtWxtClEs1W-puClJvKqEZSDHbEjCH8k7-z49Qc37NwrwxlBoNct8AqwemN1t9ECxpCeWNvHc1WnU4LbU2o2mOAwFy3IF_qFpiqIGP0Gr7JdJF9jE6jSQQsD4DdFUMEryTkjm0tBiWTjhkvrog-Y6YJpVxW3fnscBocrpV0cH-GLKAaZJYfGQb5XF8ws7XhcoMBSrzMohfhOckDBVzCjhZ0Fd0X2WX0FY6bJ3BcZsOM5SQolEooc3dXpMjOQwlqx8xaqD08YEAap2I7mYrL20JQWorg1kHrDoJhIJVkid_FY8ZSaq7YK4hlgkt2FWx5CVU2xvpatOI9zc9A9BABlAKw-EUMJ9AywUrH6I8XZxr28jukPwBw7GTVOJ_052Ru01DLS9N1DTbtinuU5TQ1XQXTSAkUbO0uPAV_O1-EdFA2W2Z4iYEjRSOI8QL7XIi3uJR67Adzy7PbUZldjdf71-wnHeVDdfHOHkgk4ItZqe7mwBkirSaGSTfI_3PtczW_OfqkFU-jn1tGOQFjv4K7-V8tV7rNQhWTdOune3_NW0Y_gjCs_Z7S8-JH8VQo59S2z3HDDoUihnbL_hUg3WZRMXZGZ9UlcnGaPhAlEeU4-ttxttfKuLO_HCG6qzPwHaDJKutTGELrLXsgQTGRU4JPB5l_doix7AqeWQeP36F9lvNuA1_32jIKTxfbxaDtJ89EYk8G7ZOt3gtPo9N_3i1uscwHAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2Ffiles_2FOutlinePanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA41V247bIBB9z1fwUm1cLRHOdZdVpf5HVVXYEJuGAAKcOKny7xVgJ_Sy3sgPFsNcz5kZJkU9pe0MgX8jALSy3HElMSCFVaJx7GMEgFMagxyhb_5geFU7DN50609HyCVlLQYz5I97Tl2NwWyK4vWWtLBm0WKJdrWXUW61IAcM1oIFJf_HlBtWxtClEs1W_puClJvKqEZSDHbEjCH8k7_z49Qc37NwrwxlBoNct8AqwemN1t9ECxpCeWNvHc1WnU4LbU2o2mOAwFy3IF_qFpiqIGP0Gr7JdJF9jE6jSQQsD4DdFUMEryTkjm0tBiWTjhkvrog_Y6YJpVxW3fnscBocrpV0cH_GLKAaZJYfGQb5XF8ws7XhcoMBSrzMohfhOckDBVzCjhZ0Fd0X2WX0FY6bJ3BcZsOM5SQolEooc3dXpMjOQwlqx8xaqD08YEAap2I7mYrL20JQWorg1kHrDoJhIJVkid_FY8ZSaq7YK4hlgkt2FWx5CVU2xvpatOI9zc9A9BABlAKw_EUMJ9AywUrH6I8XZxr28jukPwBw7GTVOJ_052Ru01DLS9N1DTbtinuU5TQ1XQXTSAkUbO0uPAV_O1_EdFA2W2Z4iYEjRSOI8QL7XIi3uJR67Adzy7PbUZldjdf71_wnHeVDdfHOHkgk4ItZqe7mwBkirSaGSTfI_3PtczW_OfqkFU_jn1tGOQFjv4K7_V8tV7rNQhWTdOune3_NW0Y_gjCs_Z7S8_JH8VQo59S2z3HDDoUihnbL_hUg3WZRMXZGZ9UlcnGaPhAlEeU4_ttxttfKuLO_HCG6qzPwHaDJKutTGELrLXsgQTGRU4JPB5l_doix7AqeWQeP36F9lvNuA1_32jIKTxfbxaDtJ89EYk8G7ZOt3gtPo9N_3i1uscwHAAA_22_7D = __webpack_require__(3119);
;// ./src/components/sessions/files/OutlinePanel.css.ts


var actions = 'bh2dx39';
var filter = 'bh2dx33';
var OutlinePanel_css_header = 'bh2dx31';
var heading = 'bh2dx32';
var list = 'bh2dx34';
var OutlinePanel_css_message = 'bh2dx38';
var OutlinePanel_css_option = 'bh2dx35';
var panel = 'bh2dx30';
var symbolKind = 'bh2dx36';
var symbolLine = 'bh2dx37';
var textButton = 'bh2dx3a';
;// ./src/components/sessions/files/GoToLineDialog.tsx
/* __next_internal_client_entry_do_not_use__ GoToLineDialog auto */ 


/** Ctrl+G prompt: a line number (out-of-range values are clamped by the caller). */ function GoToLineDialog(param) {
    let { onSubmit, onClose } = param;
    const [value, setValue] = (0,react.useState)("");
    const invoker = (0,react.useRef)(typeof document === "undefined" ? null : document.activeElement);
    (0,react.useEffect)(()=>{
        const node = invoker.current;
        return ()=>{
            var _node_focus;
            return node === null || node === void 0 ? void 0 : (_node_focus = node.focus) === null || _node_focus === void 0 ? void 0 : _node_focus.call(node);
        };
    }, []);
    const submit = (e)=>{
        e.preventDefault();
        const line = Number(value.trim().replace(/^:/, ""));
        if (Number.isInteger(line)) onSubmit(line);
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("form", {
        className: panel,
        role: "dialog",
        "aria-label": "Go to line",
        onSubmit: submit,
        onKeyDown: (e)=>{
            if (e.key === "Escape") {
                e.preventDefault();
                onClose();
            }
        },
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: OutlinePanel_css_header,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                    className: heading,
                    htmlFor: "files-goto-line",
                    children: "Go to line"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                    id: "files-goto-line",
                    className: filter,
                    inputMode: "numeric",
                    autoFocus: true,
                    value: value,
                    onChange: (e)=>setValue(e.target.value)
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    type: "submit",
                    className: textButton,
                    children: "Go"
                })
            ]
        })
    });
}

;// ./src/components/sessions/files/tabLabels.ts
const tabLabels_isDirty = (b)=>b.status === "loaded" && b.dirty;
const tabLabels_baseName = (path)=>path.slice(path.lastIndexOf("/") + 1);
/** Basename, with the parent directory added only when two open tabs would read the same. */ function labelOf(path, all) {
    const base = tabLabels_baseName(path);
    if (all.filter((b)=>tabLabels_baseName(b.path) === base).length < 2) return base;
    const parts = path.split("/");
    return parts.length > 1 ? "".concat(parts[parts.length - 2], "/").concat(base) : base;
}
/** State is in the name as words, never colour or italic alone (ux.md 5, 6). */ function accessibleName(b, all) {
    const label = labelOf(b.path, all);
    if (tabLabels_isDirty(b)) return "".concat(label, ", unsaved");
    return "".concat(label, ", ").concat(b.pinned ? "pinned" : "preview");
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2Ffiles%2FOpenTabsBar.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA91W227jOAx9z1fwZTE1sC5s51oVC-x_LBYDxaJjTRXJkOTcFv33hWS7kV07Sfs4yENriRIPDw9JPf9Ms1Jsik0C_80AGDeVoGcChcDT6wyACr6TMbe4NwSM1Wjz0q1vlWao462yVu0JpNUJjBKcwYHqpzj-mb7gJdMXjLwxzd92WtWSkcH-2u9XynDLlSSgUVDLD-hWHYTYlJrLNwLJ6-x99tyBTcfBur8EUvfvnsv4yJkt_VEAdUBdCHWMTwRobZVbM7lWQmyp7ixtyWXPT_YAKTlKi9ot72hFYFF5k4oyxuWOQOJWIIE0aTYcsBL5rrQE5lm7Rk8dhCxp7YbRA-S1NkoTqBTvPOZKuJU-p2kUJEg3nqbyU3jbQkkbG35BAum8cX8sucXYVDRHAlIdNa36zJBC5bWJD9zwrUDPk6qt4BIJZBPehPfWmsWqKAxaArFnIbz7H6o5jQ0KzC2yv35YXeOPf72PaSmtoklGkpYRRyhl6kiAS4MWEu8bkoH5PhqEWjr13HG_6R-aN4y0qiNQcsZQOhgWTza-bqAQvDLcDBhg1NK40njgePwgYHB7kzZ7FkiAWyp43rtkERg1uW2lpSqac3smkDyveyeW_kSrxE1j3Gm1_exkRRmvDYFl8sftAr9Ed0p51S8xLr027lTar9pYXpzjXEmL0oZbXR0t-vC77wa_k7TEkXj64OerT-3LaipNRTVKG8iNyxI1txNV6iPqgKT96B_SVjoQ17qxHw9FYHG34O9HNNZSgp7WSWkk2qDzzoeVvfHAr-2ebo0StfXwLzGXzPXvLLktqZcoTORUoL-i--ldf-oLTbdOV9UJ9G5Ln5I__e85W_YZWFybeRtrumkpsapypdYURpOMzYCGl9-Xhmw1RkM7hZJu2HXFsEwOZW86n7vpHLBF-x1iK1T-FlR658P3Vd8viKe9B9RF0mn2U918sRiaaTKi_JExer-9hIF-Z9DQ7w7i4I7tN585wRX5DUlPVcSHLtqvD-mny9ev4tkMnl1hwsfyMknvefoZcXmo4k4PVlxAHhvv518ZO179DHOlaZOAWjLUTgWPijXAgzeS2VVdf7y2n2PvnVzwyj3sc_uUgP9FNx6Yf--RcQpPLVICuaLaYOQRDR4NH2AWbR-64ulW3sNT6_ZU0K6uJ8N3-ehpGpy-achuGI46f5-9_w9ynfDqhw0AAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2Ffiles_2FOpenTabsBar_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA91W227jOAx9z1fwZTE1sC5s51oVC_x_LBYDxaJjTRXJkOTcFv33hWS7kV07Sfs4yENriRIPDw9JPf9Ms1Jsik0C_80AGDeVoGcChcDT6wyACr6TMbe4NwSM1Wjz0q1vlWao462yVu0JpNUJjBKcwYHqpzj_mb7gJdMXjLwxzd92WtWSkcH_2u9XynDLlSSgUVDLD_hWHYTYlJrLNwLJ6_x99tyBTcfBur8EUvfvnsv4yJkt_VEAdUBdCHWMTwRobZVbM7lWQmyp7ixtyWXPT_YAKTlKi9ot72hFYFF5k4oyxuWOQOJWIIE0aTYcsBL5rrQE5lm7Rk8dhCxp7YbRA_S1NkoTqBTvPOZKuJU_p2kUJEg3nqbyU3jbQkkbG35BAum8cX8sucXYVDRHAlIdNa36zJBC5bWJD9zwrUDPk6qt4BIJZBPehPfWmsWqKAxaArFnIbz7H6o5jQ0KzC2yv35YXeOPf72PaSmtoklGkpYRRyhl6kiAS4MWEu8bkoH5PhqEWjr13HG_6R_aN4y0qiNQcsZQOhgWTza_bqAQvDLcDBhg1NK40njgePwgYHB7kzZ7FkiAWyp43rtkERg1uW2lpSqac3smkDyveyeW_kSrxE1j3Gm1_exkRRmvDYFl8sftAr9Ed0p51S8xLr027lTar9pYXpzjXEmL0oZbXR0t_vC77wa_k7TEkXj64OerT_3LaipNRTVKG8iNyxI1txNV6iPqgKT96B_SVjoQ17qxHw9FYHG34O9HNNZSgp7WSWkk2qDzzoeVvfHAr_2ebo0StfXwLzGXzPXvLLktqZcoTORUoL_i_ldf_oLTbdOV9UJ9G5Ln5I_e85W_YZWFybeRtrumkpsapypdYURpOMzYCGl9_Xhmw1RkM7hZJu2HXFsEwOZW86n7vpHLBF_x1iK1T_FlR658P3Vd8viKe9B9RF0mn2U918sRiaaTKi_JExer_9hIF_Z9DQ7w7i4I7tN585wRX5DUlPVcSHLtqvD_mny9ev4tkMnl1hwsfyMknvefoZcXmo4k4PVlxAHhvv518ZO179DHOlaZOAWjLUTgWPijXAgzeS2VVdf7y2n2PvnVzwyj3sc_uUgP9FNx6Yf_RcQpPLVICuaLaYOQRDR4NH2AWbR_64ulW3sNT6_ZU0K6uJ8N3_ehpGpy_achuGI46f5_9_w9ynfDqhw0AAA_22_7D = __webpack_require__(67808);
;// ./src/components/sessions/files/OpenTabsBar.css.ts


var bar = '_12hl8f80';
var OpenTabsBar_css_closeButton = '_12hl8f86';
var dirtyDot = '_12hl8f85';
var listRow = '_12hl8f8b';
var menu = '_12hl8f88';
var menuItem = '_12hl8f8a';
var overflowButton = '_12hl8f87';
var pinGlyph = '_12hl8f84';
var srOnly = '_12hl8f8e';
var tab = '_12hl8f82';
var tabLabel = '_12hl8f83';
var tabList = '_12hl8f81';
var tabListDialog = '_12hl8f89';
var toast = '_12hl8f8c';
var toastAction = '_12hl8f8d';
;// ./src/components/sessions/files/OpenTabsBar.tsx
/* __next_internal_client_entry_do_not_use__ OpenTabsBar auto */ 
// +feature: files-tabs





const TOAST_MS = 8000;
function OpenTabsBar(p) {
    const { buffers, activeIndex, notice, panelId } = p;
    const analytics = (0,filesAnalytics/* useFilesAnalytics */.R)({
        component: "OpenTabsBar"
    });
    const tabRefs = (0,react.useRef)(new Map());
    const [menuState, setMenuState] = (0,react.useState)(null);
    const [listOpen, setListOpen] = (0,react.useState)(false);
    const [shown, setShown] = (0,react.useState)(null);
    const [paused, setPaused] = (0,react.useState)(false);
    const focusTab = (path)=>{
        var _tabRefs_current_get;
        return (_tabRefs_current_get = tabRefs.current.get(path)) === null || _tabRefs_current_get === void 0 ? void 0 : _tabRefs_current_get.focus();
    };
    const activate = (0,react.useCallback)((path, via)=>{
        analytics.emit("files.tab.switch", {
            via
        });
        p.onActivate(path);
    }, [
        analytics,
        p
    ]);
    const close = (0,react.useCallback)((path, method)=>{
        const b = buffers.find((x)=>x.path === path);
        analytics.emit("files.tab.close", {
            dirty: b && tabLabels_isDirty(b) ? "true" : "false",
            method,
            open_tabs: (0,events/* bucketTabs */.FP)(buffers.length)
        });
        p.onClose(path);
    }, [
        analytics,
        buffers,
        p
    ]);
    // The reducer clears its notice on the next action; the bar keeps showing it until dismissed or timed out.
    (0,react.useEffect)(()=>{
        if (!notice) return;
        setShown(notice);
        analytics.emit("files.tab.evict", {
            outcome: notice.kind === "evicted" ? "evicted_clean_preview" : "refused_pinned_dirty"
        });
        if (notice.kind === "refused") setListOpen(true);
    }, [
        notice,
        analytics
    ]);
    (0,react.useEffect)(()=>{
        if ((shown === null || shown === void 0 ? void 0 : shown.kind) !== "evicted" || paused) return;
        const t = setTimeout(()=>setShown(null), TOAST_MS);
        return ()=>clearTimeout(t);
    }, [
        shown,
        paused
    ]);
    const active = buffers[activeIndex];
    (0,react.useEffect)(()=>{
        var _tabRefs_current_get_scrollIntoView, _tabRefs_current_get;
        if (active) (_tabRefs_current_get = tabRefs.current.get(active.path)) === null || _tabRefs_current_get === void 0 ? void 0 : (_tabRefs_current_get_scrollIntoView = _tabRefs_current_get.scrollIntoView) === null || _tabRefs_current_get_scrollIntoView === void 0 ? void 0 : _tabRefs_current_get_scrollIntoView.call(_tabRefs_current_get, {
            block: "nearest",
            inline: "nearest"
        });
    }, [
        active === null || active === void 0 ? void 0 : active.path
    ]); // eslint-disable-line react-hooks/exhaustive-deps
    const onKeyDown = (e, b, index)=>{
        if (e.altKey && e.shiftKey && (e.key === "ArrowLeft" || e.key === "ArrowRight")) {
            e.preventDefault();
            p.onMove(b.path, e.key === "ArrowLeft" ? -1 : 1);
            return;
        }
        if (e.key === "ContextMenu" || e.shiftKey && e.key === "F10") {
            e.preventDefault();
            setMenuState({
                path: b.path,
                x: 8
            });
            return;
        }
        if (e.key === "Delete") {
            e.preventDefault();
            var _buffers_;
            const neighbor = (_buffers_ = buffers[index + 1]) !== null && _buffers_ !== void 0 ? _buffers_ : buffers[index - 1];
            close(b.path, "keyboard");
            if (neighbor) {
                focusTab(neighbor.path);
                if (index === activeIndex) p.onActivate(neighbor.path);
            }
            return;
        }
        const target = e.key === "ArrowRight" ? (index + 1) % buffers.length : e.key === "ArrowLeft" ? (index - 1 + buffers.length) % buffers.length : e.key === "Home" ? 0 : e.key === "End" ? buffers.length - 1 : -1;
        if (target < 0) return;
        e.preventDefault();
        focusTab(buffers[target].path);
        activate(buffers[target].path, "keyboard");
    };
    const openMenu = (e, path)=>{
        e.preventDefault();
        setMenuState({
            path,
            x: Math.max(8, e.clientX - e.currentTarget.getBoundingClientRect().left)
        });
    };
    const message = (shown === null || shown === void 0 ? void 0 : shown.kind) === "evicted" ? "Closed ".concat(tabLabels_baseName(shown.path), " to stay within ").concat(shown.cap, " tabs") : (shown === null || shown === void 0 ? void 0 : shown.kind) === "refused" ? "Too many tabs open - close one" : "";
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: bar,
        "data-testid": "files-tab-bar",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: tabList,
                role: "tablist",
                "aria-label": "Open files",
                children: buffers.map((b, i)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        ref: (el)=>{
                            if (el) tabRefs.current.set(b.path, el);
                            else tabRefs.current.delete(b.path);
                        },
                        role: "tab",
                        id: "files-tab-".concat(i),
                        className: tab,
                        "data-testid": "files-tab-".concat(b.path),
                        "data-preview": !b.pinned && !tabLabels_isDirty(b) ? "true" : undefined,
                        "aria-selected": i === activeIndex,
                        "aria-controls": panelId,
                        "aria-label": accessibleName(b, buffers),
                        title: b.path,
                        tabIndex: i === activeIndex || activeIndex < 0 && i === 0 ? 0 : -1,
                        onClick: ()=>activate(b.path, "strip"),
                        onDoubleClick: ()=>p.onPin(b.path),
                        onAuxClick: (e)=>{
                            if (e.button === 1) {
                                e.preventDefault();
                                close(b.path, "button");
                            }
                        },
                        onContextMenu: (e)=>openMenu(e, b.path),
                        onKeyDown: (e)=>onKeyDown(e, b, i),
                        children: [
                            b.pinned && !tabLabels_isDirty(b) && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: pinGlyph,
                                "aria-hidden": "true",
                                children: "\uD83D\uDCCC"
                            }),
                            tabLabels_isDirty(b) && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: dirtyDot,
                                "data-testid": "files-tab-dirty-dot-".concat(b.path),
                                "aria-hidden": "true"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: tabLabel,
                                "data-tab-label": true,
                                children: labelOf(b.path, buffers)
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: OpenTabsBar_css_closeButton,
                                tabIndex: -1,
                                "aria-label": "Close ".concat(labelOf(b.path, buffers)),
                                onClick: (e)=>{
                                    e.stopPropagation();
                                    close(b.path, "button");
                                },
                                children: "\xd7"
                            })
                        ]
                    }, b.path))
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                className: overflowButton,
                "aria-label": "List open tabs",
                "aria-haspopup": "dialog",
                "aria-expanded": listOpen,
                onClick: ()=>setListOpen((o)=>!o),
                children: "▾"
            }),
            menuState && /*#__PURE__*/ (0,jsx_runtime.jsx)(TabMenu, {
                path: menuState.path,
                x: menuState.x,
                buffers: buffers,
                onDismiss: ()=>{
                    setMenuState(null);
                    focusTab(menuState.path);
                },
                actions: {
                    close: ()=>close(menuState.path, "context_menu"),
                    closeOthers: ()=>p.onCloseOthers(menuState.path),
                    pin: ()=>p.onPin(menuState.path),
                    unpin: ()=>p.onUnpin(menuState.path),
                    move: (d)=>p.onMove(menuState.path, d),
                    copy: ()=>p.onCopyPath(menuState.path),
                    reveal: ()=>p.onReveal(menuState.path)
                }
            }),
            listOpen && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: tabListDialog,
                role: "dialog",
                "aria-label": "Open tabs",
                onKeyDown: (e)=>{
                    if (e.key === "Escape") setListOpen(false);
                },
                children: [
                    (shown === null || shown === void 0 ? void 0 : shown.kind) === "refused" && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        role: "note",
                        children: message
                    }),
                    buffers.map((b)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: listRow,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "button",
                                    className: menuItem,
                                    onClick: ()=>{
                                        activate(b.path, "strip");
                                        setListOpen(false);
                                    },
                                    children: accessibleName(b, buffers)
                                }),
                                b.pinned && !tabLabels_isDirty(b) && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                    type: "button",
                                    className: menuItem,
                                    onClick: ()=>p.onUnpin(b.path),
                                    children: [
                                        "Unpin ",
                                        labelOf(b.path, buffers)
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "button",
                                    className: menuItem,
                                    "aria-label": "Close ".concat(labelOf(b.path, buffers), " from list"),
                                    onClick: ()=>close(b.path, "button"),
                                    children: "\xd7"
                                })
                            ]
                        }, b.path))
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: srOnly,
                role: "status",
                "aria-live": "polite",
                children: message
            }),
            (shown === null || shown === void 0 ? void 0 : shown.kind) === "evicted" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: toast,
                "data-testid": "files-tab-toast",
                role: "group",
                "aria-label": "Tab closed",
                onMouseEnter: ()=>setPaused(true),
                onMouseLeave: ()=>setPaused(false),
                onFocus: ()=>setPaused(true),
                onBlur: ()=>setPaused(false),
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        children: message
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: toastAction,
                        onClick: ()=>{
                            analytics.emit("files.tab.evict_undo");
                            p.onUndoEvict();
                            setShown(null);
                        },
                        children: "Undo"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: toastAction,
                        "aria-label": "Dismiss",
                        onClick: ()=>setShown(null),
                        children: "\xd7"
                    })
                ]
            }),
            (shown === null || shown === void 0 ? void 0 : shown.kind) === "refused" && !listOpen && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: toast,
                role: "group",
                "aria-label": "Tab refused",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        children: message
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: toastAction,
                        "aria-label": "Dismiss",
                        onClick: ()=>setShown(null),
                        children: "\xd7"
                    })
                ]
            })
        ]
    });
}
function TabMenu(param) {
    let { path, x, buffers, onDismiss, actions } = param;
    const ref = (0,react.useRef)(null);
    const buffer = buffers.find((b)=>b.path === path);
    const index = buffers.findIndex((b)=>b.path === path);
    // onDismiss is an inline closure; reading it through a ref keeps the effects from re-running (and re-focusing) per render.
    const dismissRef = (0,react.useRef)(onDismiss);
    dismissRef.current = onDismiss;
    (0,react.useEffect)(()=>{
        var _ref_current_querySelector, _ref_current;
        (_ref_current = ref.current) === null || _ref_current === void 0 ? void 0 : (_ref_current_querySelector = _ref_current.querySelector("[role=menuitem]")) === null || _ref_current_querySelector === void 0 ? void 0 : _ref_current_querySelector.focus();
    }, [
        path
    ]);
    (0,react.useEffect)(()=>{
        const onDown = (e)=>{
            var _ref_current;
            if (!((_ref_current = ref.current) === null || _ref_current === void 0 ? void 0 : _ref_current.contains(e.target))) dismissRef.current();
        };
        document.addEventListener("mousedown", onDown);
        return ()=>document.removeEventListener("mousedown", onDown);
    }, []);
    if (!buffer) return null;
    const items = [
        [
            "Close",
            actions.close
        ],
        [
            "Close others",
            actions.closeOthers,
            buffers.length < 2
        ],
        [
            buffer.pinned ? "Unpin" : "Pin",
            buffer.pinned ? actions.unpin : actions.pin,
            tabLabels_isDirty(buffer) && buffer.pinned
        ],
        [
            "Move left",
            ()=>actions.move(-1),
            index === 0
        ],
        [
            "Move right",
            ()=>actions.move(1),
            index === buffers.length - 1
        ],
        [
            "Copy path",
            actions.copy
        ],
        [
            "Reveal in tree",
            actions.reveal
        ]
    ];
    const onKeyDown = (e)=>{
        var _ref_current, _nodes_;
        if (e.key === "Escape") {
            e.preventDefault();
            onDismiss();
            return;
        }
        if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
        e.preventDefault();
        var _ref_current_querySelectorAll;
        const nodes = [
            ...(_ref_current_querySelectorAll = (_ref_current = ref.current) === null || _ref_current === void 0 ? void 0 : _ref_current.querySelectorAll("[role=menuitem]:not([disabled])")) !== null && _ref_current_querySelectorAll !== void 0 ? _ref_current_querySelectorAll : []
        ];
        const at = nodes.indexOf(document.activeElement);
        (_nodes_ = nodes[(at + (e.key === "ArrowDown" ? 1 : -1) + nodes.length) % nodes.length]) === null || _nodes_ === void 0 ? void 0 : _nodes_.focus();
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        ref: ref,
        className: menu,
        style: {
            left: x
        },
        role: "menu",
        "aria-label": "".concat(tabLabels_baseName(path), " tab actions"),
        onKeyDown: onKeyDown,
        children: items.map((param)=>{
            let [label, run, disabled] = param;
            return /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                role: "menuitem",
                className: menuItem,
                disabled: disabled,
                onClick: ()=>{
                    run();
                    onDismiss();
                },
                children: label
            }, label);
        })
    });
}

;// ./src/components/sessions/files/OutlinePanel.tsx
/* __next_internal_client_entry_do_not_use__ OutlinePanel auto */ 




const UNAVAILABLE_TEXT = {
    large: "Outline unavailable for large files",
    unsupported: "No symbols found for this file type",
    error: "Could not read symbols"
};
/** Symbol picker: type-ahead listbox, or the specific unavailable message with its exits. */ function OutlinePanel(param) {
    let { result, onSelect, onClose, onGoToLine } = param;
    const analytics = (0,filesAnalytics/* useFilesAnalytics */.R)({
        component: "FileContentViewer"
    });
    const [query, setQuery] = (0,react.useState)("");
    const [cursor, setCursor] = (0,react.useState)(0);
    const invoker = (0,react.useRef)(typeof document === "undefined" ? null : document.activeElement);
    const done = (0,react.useRef)(false);
    const symbols = result.kind === "symbols" ? result.symbols : [];
    const shown = symbols.filter((s)=>s.name.toLowerCase().includes(query.toLowerCase()));
    const bucket = (0,events/* bucketSymbols */.Ow)(symbols.length);
    const message_ = result.kind === "unavailable" ? UNAVAILABLE_TEXT[result.reason] : symbols.length === 0 ? UNAVAILABLE_TEXT.unsupported : null;
    // Unavailable and empty outlines are terminal on open; a real list ends on select or close.
    (0,react.useEffect)(()=>{
        if (message_ === null) return;
        done.current = true;
        analytics.emit("files.nav.outline", {
            outcome: result.kind === "unavailable" && result.reason === "large" ? "unavailable_large" : "empty",
            symbols: bucket
        });
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
    (0,react.useEffect)(()=>{
        const node = invoker.current;
        return ()=>{
            var _node_focus;
            return node === null || node === void 0 ? void 0 : (_node_focus = node.focus) === null || _node_focus === void 0 ? void 0 : _node_focus.call(node);
        };
    }, []);
    const finish = (outcome)=>{
        if (!done.current) analytics.emit("files.nav.outline", {
            outcome,
            symbols: bucket
        });
        done.current = true;
    };
    const choose = (s)=>{
        finish("jumped");
        onSelect(s);
    };
    const close = ()=>{
        finish("closed");
        onClose();
    };
    const onKeyDown = (e)=>{
        if (e.key === "Escape") {
            e.preventDefault();
            close();
        } else if (e.key === "ArrowDown") {
            e.preventDefault();
            setCursor((c)=>Math.min(c + 1, shown.length - 1));
        } else if (e.key === "ArrowUp") {
            e.preventDefault();
            setCursor((c)=>Math.max(c - 1, 0));
        } else if (e.key === "Enter" && shown[cursor]) {
            e.preventDefault();
            choose(shown[cursor]);
        }
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: panel,
        role: "dialog",
        "aria-label": "Outline",
        onKeyDown: onKeyDown,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: OutlinePanel_css_header,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: heading,
                        children: [
                            "Outline",
                            symbols.length ? " (".concat(symbols.length, ")") : ""
                        ]
                    }),
                    message_ === null && /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        className: filter,
                        type: "text",
                        "aria-label": "Filter symbols",
                        autoFocus: true,
                        value: query,
                        "aria-controls": "outline-symbols",
                        "aria-activedescendant": shown[cursor] ? "outline-opt-".concat(cursor) : undefined,
                        onChange: (e)=>{
                            setQuery(e.target.value);
                            setCursor(0);
                        }
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: textButton,
                        onClick: close,
                        children: "Close"
                    })
                ]
            }),
            message_ !== null ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: OutlinePanel_css_message,
                        role: "status",
                        children: message_
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: actions,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            type: "button",
                            className: textButton,
                            onClick: ()=>{
                                finish("closed");
                                onGoToLine();
                            },
                            children: "Go to line"
                        })
                    })
                ]
            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                className: list,
                id: "outline-symbols",
                role: "listbox",
                "aria-label": "Symbols",
                children: shown.map((s, i)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                        id: "outline-opt-".concat(i),
                        role: "option",
                        "aria-selected": i === cursor,
                        className: OutlinePanel_css_option,
                        style: {
                            paddingLeft: 8 + s.depth * 12
                        },
                        onMouseEnter: ()=>setCursor(i),
                        onClick: ()=>choose(s),
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                children: s.name
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: symbolKind,
                                children: s.kind
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: symbolLine,
                                children: s.line
                            })
                        ]
                    }, "".concat(s.line, ":").concat(s.name)))
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/components/sessions/files/BannerStack.tsx + 2 modules
var BannerStack = __webpack_require__(89038);
// EXTERNAL MODULE: ./src/components/sessions/files/StaleBanner.css.ts
var StaleBanner_css = __webpack_require__(31597);
;// ./src/components/sessions/files/StaleBanner.tsx
/* __next_internal_client_entry_do_not_use__ staleBannerSpec,StaleBanner auto */ 


/** The stale-on-disk banner as a stack entry; "stale with edits" outranks a clean stale buffer. */ function staleBannerSpec(stale, sessionRunning, onAction) {
    const suffix = sessionRunning ? " (session is running; this may be the agent)" : " (source unknown)";
    return {
        id: "stale:".concat(stale.path),
        kind: stale.dirty ? "stale_dirty" : "stale_clean",
        seq: 1,
        testId: "files-stale-banner",
        onDismiss: ()=>onAction("dismiss"),
        body: /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
            children: [
                "Changed on disk -",
                " ",
                stale.dirty ? /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    type: "button",
                    className: StaleBanner_css/* bannerAction */.X,
                    onClick: ()=>onAction("review"),
                    children: "review diff"
                }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    type: "button",
                    className: StaleBanner_css/* bannerAction */.X,
                    onClick: ()=>onAction("reload"),
                    children: "Reload"
                }),
                suffix
            ]
        })
    };
}
/** A status line, never a dialog: it must not take focus from wherever the user is typing. */ function StaleBanner(param) {
    let { stale, sessionRunning, onAction } = param;
    return /*#__PURE__*/ _jsx(BannerItem, {
        spec: staleBannerSpec(stale, sessionRunning, onAction)
    });
}

;// ./src/components/sessions/files/useTabActions.ts

/** Tab commands shared by the desktop strip and the mobile sheet. */ function useTabActions(v2) {
    return {
        closeOthers: (keep)=>v2 === null || v2 === void 0 ? void 0 : v2.buffers.state.buffers.forEach((b)=>{
                if (b.path !== keep) v2.buffers.close(b.path);
            }),
        copyPath: (path)=>{
            var _navigator_clipboard;
            return void ((_navigator_clipboard = navigator.clipboard) === null || _navigator_clipboard === void 0 ? void 0 : _navigator_clipboard.writeText(path));
        },
        reopen: ()=>{
            if (!v2) return;
            const top = v2.buffers.state.closed[v2.buffers.state.closed.length - 1];
            v2.buffers.reopenClosed();
            if (top) v2.feedback.announceSuccess("reopened", tabLabels_baseName(top.path));
        }
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2Ffiles%2FFilesV2Chrome.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xN3B6b2N2MCB7CiAgcG9zaXRpb246IHJlbGF0aXZlOwogIGZsZXgtc2hyaW5rOiAwOwp9Ci5fMTdwem9jdjEgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IDRweDsKICBwYWRkaW5nOiAycHggOHB4OwogIGJvcmRlci1ib3R0b206IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6Nyk7Cn0KLl8xN3B6b2N2MiB7CiAgYm9yZGVyOiBub25lOwogIGJhY2tncm91bmQ6IHRyYW5zcGFyZW50OwogIGNvbG9yOiB2YXIoLS1fMTllejJyejEpOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzYpOwogIHBhZGRpbmc6IDJweCA4cHg7CiAgY3Vyc29yOiBwb2ludGVyOwogIGZvbnQtc2l6ZTogMTNweDsKfQouXzE3cHpvY3YyOmhvdmVyIHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejgpOwp9Ci5fMTdwem9jdjJbYXJpYS1kaXNhYmxlZD0ndHJ1ZSddIHsKICBvcGFjaXR5OiAwLjQ7CiAgY3Vyc29yOiBkZWZhdWx0Owp9Ci5fMTdwem9jdjMgewogIGZsZXg6IDE7Cn0KLl8xN3B6b2N2NCB7CiAgZmxleDogMTsKICBtaW4taGVpZ2h0OiAwOwogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKfQpAbWVkaWEgKHBvaW50ZXI6IGNvYXJzZSkgewogIC5fMTdwem9jdjIgewogICAgbWluLWhlaWdodDogNDRweDsKICAgIG1pbi13aWR0aDogNDRweDsKICB9Cn0%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2Ffiles_2FFilesV2Chrome_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xN3B6b2N2MCB7CiAgcG9zaXRpb246IHJlbGF0aXZlOwogIGZsZXgtc2hyaW5rOiAwOwp9Ci5fMTdwem9jdjEgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IDRweDsKICBwYWRkaW5nOiAycHggOHB4OwogIGJvcmRlci1ib3R0b206IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6Nyk7Cn0KLl8xN3B6b2N2MiB7CiAgYm9yZGVyOiBub25lOwogIGJhY2tncm91bmQ6IHRyYW5zcGFyZW50OwogIGNvbG9yOiB2YXIoLS1fMTllejJyejEpOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzYpOwogIHBhZGRpbmc6IDJweCA4cHg7CiAgY3Vyc29yOiBwb2ludGVyOwogIGZvbnQtc2l6ZTogMTNweDsKfQouXzE3cHpvY3YyOmhvdmVyIHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejgpOwp9Ci5fMTdwem9jdjJbYXJpYS1kaXNhYmxlZD0ndHJ1ZSddIHsKICBvcGFjaXR5OiAwLjQ7CiAgY3Vyc29yOiBkZWZhdWx0Owp9Ci5fMTdwem9jdjMgewogIGZsZXg6IDE7Cn0KLl8xN3B6b2N2NCB7CiAgZmxleDogMTsKICBtaW4taGVpZ2h0OiAwOwogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKfQpAbWVkaWEgKHBvaW50ZXI6IGNvYXJzZSkgewogIC5fMTdwem9jdjIgewogICAgbWluLWhlaWdodDogNDRweDsKICAgIG1pbi13aWR0aDogNDRweDsKICB9Cn0_22_7D = __webpack_require__(54941);
;// ./src/components/sessions/files/FilesV2Chrome.css.ts


var chrome = '_17pzocv0';
var navButton = '_17pzocv2';
var navRow = '_17pzocv1';
var spacer = '_17pzocv3';
var tabPanel = '_17pzocv4';
;// ./src/components/sessions/files/FilesV2Chrome.tsx
/* __next_internal_client_entry_do_not_use__ FilesV2Chrome auto */ 










/** Tab strip, back/forward/outline/go-to-line row, stale banner and their overlays. */ function FilesV2Chrome(param) {
    let { v2, fileTreeRef, panelId, mobile = false, onQuickOpen, onSearch } = param;
    const { buffers, bridge, stale, sessionRunning } = v2;
    const { state } = buffers;
    const back = canGoBack(state);
    const forward = canGoForward(state);
    const { closeOthers, copyPath, reopen } = useTabActions(v2);
    // Touch sizing follows the pointer until the resolved InputProfile (ADR-033) lands; 44px touch, 24px with a hardware keyboard.
    const target = (0,useCoarsePointer/* useCoarsePointer */.O)() ? 44 : 24;
    const active = buffers.active;
    const banners = [
        ...stale ? [
            staleBannerSpec(stale, sessionRunning, v2.onStaleAction)
        ] : [],
        ...v2.reloadFailed && (active === null || active === void 0 ? void 0 : active.status) === "loaded" && active.path === v2.reloadFailed.path ? [
            (0,PendingState/* reloadFailedBanner */.v_)({
                path: active.path,
                reason: "rpc_error",
                dirty: active.dirty,
                seq: v2.reloadFailed.seq,
                onRetry: v2.reloadFromDisk,
                onCopy: ()=>{
                    var _navigator_clipboard;
                    const text = bridge.text();
                    if (text !== null) void ((_navigator_clipboard = navigator.clipboard) === null || _navigator_clipboard === void 0 ? void 0 : _navigator_clipboard.writeText(text));
                },
                onKeep: v2.clearReloadFailed
            })
        ] : []
    ];
    if (bridge.beyond) {
        banners.push({
            id: "beyond:".concat(bridge.beyond.path),
            kind: "beyond_loaded",
            seq: 2,
            body: "Line ".concat(bridge.beyond.line, " is beyond the loaded portion"),
            onDismiss: bridge.dismissBeyond
        });
    }
    const jumpToSymbol = (line)=>{
        if (buffers.active) bridge.jumpTo(buffers.active.path, line, {
            source: "outline"
        });
        bridge.setOverlay(null);
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: chrome,
        children: [
            !mobile && /*#__PURE__*/ (0,jsx_runtime.jsx)(OpenTabsBar, {
                buffers: state.buffers,
                activeIndex: state.activeIndex,
                notice: state.notice,
                panelId: panelId,
                onActivate: bridge.activate,
                onClose: (path)=>buffers.close(path),
                onPin: buffers.pin,
                onUnpin: buffers.unpin,
                onMove: buffers.move,
                onCloseOthers: closeOthers,
                onCopyPath: copyPath,
                onReveal: (path)=>{
                    var _fileTreeRef_current;
                    return (_fileTreeRef_current = fileTreeRef.current) === null || _fileTreeRef_current === void 0 ? void 0 : _fileTreeRef_current.revealPath(path);
                },
                onUndoEvict: reopen
            }),
            !mobile && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: navRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: navButton,
                        "aria-label": "Back",
                        "aria-disabled": !back,
                        title: back ? "Back (Alt+[)" : "No earlier location",
                        onClick: ()=>bridge.back("button"),
                        children: "←"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: navButton,
                        "aria-label": "Forward",
                        "aria-disabled": !forward,
                        title: forward ? "Forward (Alt+])" : "No later location",
                        onClick: ()=>bridge.forward("button"),
                        children: "→"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: spacer
                    }),
                    onQuickOpen && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: navButton,
                        style: {
                            minHeight: target,
                            minWidth: target
                        },
                        "aria-haspopup": "dialog",
                        title: "Quick open (Ctrl+P)",
                        onClick: onQuickOpen,
                        children: "Quick open"
                    }),
                    onSearch && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: navButton,
                        style: {
                            minHeight: target,
                            minWidth: target
                        },
                        title: "Search (Ctrl+Shift+F)",
                        onClick: onSearch,
                        children: "Search"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: navButton,
                        style: {
                            minHeight: target,
                            minWidth: target
                        },
                        "aria-pressed": v2.accessible.on,
                        disabled: !buffers.active,
                        title: "Accessible view: plain paged text for screen readers",
                        onClick: ()=>v2.accessible.toggle("toolbar"),
                        children: "Accessible view"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: navButton,
                        style: {
                            minHeight: target,
                            minWidth: target
                        },
                        "aria-haspopup": "dialog",
                        disabled: !buffers.active,
                        onClick: ()=>bridge.setOverlay("outline"),
                        children: "Outline"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: navButton,
                        style: {
                            minHeight: target,
                            minWidth: target
                        },
                        "aria-haspopup": "dialog",
                        disabled: !buffers.active,
                        title: "Go to line (Ctrl+G)",
                        onClick: ()=>bridge.setOverlay("goto"),
                        children: "Go to line"
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(BannerStack/* BannerStack */.j, {
                banners: banners
            }),
            bridge.overlay === "outline" && /*#__PURE__*/ (0,jsx_runtime.jsx)(OutlinePanel, {
                result: bridge.outline(),
                onSelect: (s)=>jumpToSymbol(s.line),
                onClose: ()=>bridge.setOverlay(null),
                onGoToLine: ()=>bridge.setOverlay("goto")
            }),
            bridge.overlay === "goto" && /*#__PURE__*/ (0,jsx_runtime.jsx)(GoToLineDialog, {
                onSubmit: (line)=>{
                    bridge.setOverlay(null);
                    bridge.goToLine(line, "palette");
                },
                onClose: ()=>bridge.setOverlay(null)
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/files/useWindowedState.ts
var useWindowedState = __webpack_require__(50163);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2Ffiles%2FStatusBar.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA9VV227bMAx9z1fwpWiCVYXtJk6iYkA_Y8AwDIxFO1ptSZDkxM2Qfx8sO41z6zbsafBLTFHM4TmH9MsrveUWK3LQLHZLN5_DzxFAbnUVfgBog5n0bxyi5xHAfgTg9flR3B3tR49dkSgkCOlMiW8c8pKaNgNLWSgmPVWOQ0bKk23DBRoOcWJCjkEhpCo4JKaBOOqClVRsTbJYew5J2sVyrTxzckfHuyttBVnmdVvPNOB0KQVs0I4Z-x4vaZfYHU1CJmavhdW1EvzsfB7OM11qe34UT56PLcYdTy2IDVqJyjNVV2RlxsHjqi7RtgEXoJbUMLe2Ur0GGt-LJKFIB_s25GIyaM6ikLU7h_aUXrTlLSpn0JLyg46kWpOV_kDgSeCd-gh6jrPauvaW0fIg1s1enkIvRjvppVYccOV0WXvqsHuvKw5xFN217yXlnsOiFxcbtpXCrzk8JdGZCxYDF9wWbTlg6DaPP_6Axw_kj_rrbfso9LbladrCS00DtljhOHoIz2MyC6nbtfTEnMGMOChtKywHfE0DXxXaQirWEYK1189_OzjpcUR6FoeqzMK_XLdzMvRzevTztp-0NBpWWpzpm8uGxFHMWadscF2ubdUbsERPX8ZsFt1NhkbIsMzGcTu38AlIbcYOc2JoCZlUjjzrEh8gMs0k3NwxqQQ1HKbRCUVKK_odRYtrpkr-R1Md1fgq0COrdCvG5_scBd1_CwqhkhV2Eh02ejyLKgeEjpiu_aDK8mT_HKj8ty3iqfFMUKZtj6JWgmwpu9pXNsrAudPp5b7vYvvRS0VCIowH62Kezk0zCT2cfHgA9IZsXuota45TdTGQW4vm8E07MfmVCdwPEPTYOWQaraMTAElf4LKDUOMXz8Ipbm8HAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2Ffiles_2FStatusBar_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9VV227bMAx9z1fwpWiCVYXtJk6iYkA_Y8AwDIxFO1ptSZDkxM2Qfx8sO41z6zbsafBLTFHM4TmH9MsrveUWK3LQLHZLN5_DzxFAbnUVfgBog5n0bxyi5xHAfgTg9flR3B3tR49dkSgkCOlMiW8c8pKaNgNLWSgmPVWOQ0bKk23DBRoOcWJCjkEhpCo4JKaBOOqClVRsTbJYew5J2sVyrTxzckfHuyttBVnmdVvPNOB0KQVs0I4Z_x4vaZfYHU1CJmavhdW1EvzsfB7OM11qe34UT56PLcYdTy2IDVqJyjNVV2RlxsHjqi7RtgEXoJbUMLe2Ur0GGt_LJKFIB_s25GIyaM6ikLU7h_aUXrTlLSpn0JLyg46kWpOV_kDgSeCd_gh6jrPauvaW0fIg1s1enkIvRjvppVYccOV0WXvqsHuvKw5xFN217yXlnsOiFxcbtpXCrzk8JdGZCxYDF9wWbTlg6DaPP_6Axw_kj_rrbfso9LbladrCS00DtljhOHoIz2MyC6nbtfTEnMGMOChtKywHfE0DXxXaQirWEYK1189_OzjpcUR6FoeqzMK_XLdzMvRzevTztp_0NBpWWpzpm8uGxFHMWadscF2ubdUbsERPX8ZsFt1NhkbIsMzGcTu38AlIbcYOc2JoCZlUjjzrEh8gMs0k3NwxqQQ1HKbRCUVKK_odRYtrpkr_R1Md1fgq0COrdCvG5_scBd1_CwqhkhV2Eh02ejyLKgeEjpiu_aDK8mT_HKj8ty3iqfFMUKZtj6JWgmwpu9pXNsrAudPp5b7vYvvRS0VCIowH62Kezk0zCT2cfHgA9IZsXuota45TdTGQW4vm8E07MfmVCdwPEPTYOWQaraMTAElf4LKDUOMXz8Ipbm8HAAA_22_7D = __webpack_require__(43799);
;// ./src/components/sessions/files/StatusBar.css.ts


var StatusBar_css_bar = 'x8z9s70';
var chip = 'x8z9s72';
var StatusBar_css_dirtyDot = 'x8z9s76';
var errorText = 'x8z9s75';
var popover = 'x8z9s73';
var region = 'x8z9s74';
var segment = 'x8z9s71';
var toastBox = 'x8z9s78';
var toastButton = 'x8z9s79';
;// ./src/components/sessions/files/FeedbackToast.tsx
/* __next_internal_client_entry_do_not_use__ FeedbackToast auto */ 

const reducedMotion = ()=> true && typeof window.matchMedia === "function" && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
/** Mobile cue for a success; the status-bar region is the announced copy, so this one is not live. */ function FeedbackToast(param) {
    let { toast, onDismiss, onPause, onResume } = param;
    if (!toast) return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: toastBox,
        role: "group",
        "aria-label": "Notification",
        "data-testid": "files-toast",
        "data-motion": reducedMotion() ? "none" : "fade",
        onMouseEnter: onPause,
        onMouseLeave: onResume,
        onFocus: onPause,
        onBlur: onResume,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                children: toast.text
            }),
            toast.undo && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                className: toastButton,
                onClick: ()=>{
                    var _toast_undo;
                    (_toast_undo = toast.undo) === null || _toast_undo === void 0 ? void 0 : _toast_undo.call(toast);
                    onDismiss();
                },
                children: "Undo"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                className: toastButton,
                "aria-label": "Dismiss",
                onClick: onDismiss,
                children: "\xd7"
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/components/sessions/files/LargeFileStates.tsx + 1 modules
var LargeFileStates = __webpack_require__(47558);
;// ./src/components/sessions/files/StatusBar.tsx
/* __next_internal_client_entry_do_not_use__ readOnlyChipText,StatusBar auto */ 




const REASONS = {
    flag_off: {
        chip: "editing is turned off",
        why: "Editing is turned off for this installation, so the file can only be read."
    },
    binary: {
        chip: "binary file",
        why: "Binary files are shown as a placeholder and cannot be edited."
    },
    truncated: {
        chip: "showing the first part only",
        why: "Only the first part of this file is loaded, so it cannot be edited."
    },
    too_large: {
        chip: "file too large to edit",
        why: "This file is larger than the editable limit and cannot be edited."
    },
    non_utf8: {
        chip: "unsupported encoding",
        why: "This file is not UTF-8 text and cannot be edited without corrupting it."
    },
    mixed_eol: {
        chip: "mixed line endings",
        why: "This file mixes line endings, so saving could rewrite them; it cannot be edited."
    },
    symlink: {
        chip: "symbolic link",
        why: "Symbolic links are not edited through the viewer."
    },
    worktree_missing: {
        chip: "worktree unavailable",
        why: "The session's worktree is not on disk (the session may be paused), so the file cannot be edited."
    },
    hidden_session: {
        chip: "session is hidden",
        why: "Hidden sessions cannot be edited."
    },
    git_internal: {
        chip: "git internals",
        why: "Files inside .git cannot be edited."
    }
};
function readOnlyChipText(reason, detail) {
    return "Read-only: ".concat(detail ? "showing ".concat(detail) : REASONS[reason].chip);
}
const positionText = (line, col, w)=>{
    const ln = line.toLocaleString("en-US");
    if (!w) return "Ln ".concat(line, ", Col ").concat(col);
    return "Ln ".concat(ln, " of ").concat(w.exact ? w.lineCount.toLocaleString("en-US") : "about ".concat((0,windowedSource/* approxLines */.Yd)(w.lineCount)), ", Col ").concat(col);
};
/** Position, format and state of the active buffer, plus the polite region for success and error messages. */ function StatusBar(p) {
    var _p_windowed, _p_message, _p_message1;
    const [open, setOpen] = (0,react.useState)(false);
    const chipRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        if (!open) return;
        const onKey = (e)=>{
            var _chipRef_current;
            if (e.key !== "Escape") return;
            e.stopPropagation();
            setOpen(false);
            (_chipRef_current = chipRef.current) === null || _chipRef_current === void 0 ? void 0 : _chipRef_current.focus();
        };
        document.addEventListener("keydown", onKey, true);
        return ()=>document.removeEventListener("keydown", onKey, true);
    }, [
        open
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: StatusBar_css_bar,
        "data-testid": "files-status-bar",
        style: {
            position: "relative"
        },
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: segment,
                children: positionText(p.line, p.col, p.windowed)
            }),
            ((_p_windowed = p.windowed) === null || _p_windowed === void 0 ? void 0 : _p_windowed.indexing) && /*#__PURE__*/ (0,jsx_runtime.jsx)(LargeFileStates/* IndexingChip */.DC, {}),
            p.eol && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: segment,
                children: p.eol
            }),
            p.encoding && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: segment,
                children: p.encoding
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: segment,
                children: p.language
            }),
            p.readOnly !== "none" && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                ref: chipRef,
                type: "button",
                className: chip,
                "aria-expanded": open,
                onClick: ()=>setOpen((o)=>!o),
                children: readOnlyChipText(p.readOnly, p.readOnlyDetail)
            }),
            open && p.readOnly !== "none" && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: popover,
                role: "note",
                children: REASONS[p.readOnly].why
            }),
            p.dirty && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: "".concat(segment, " ").concat(StatusBar_css_dirtyDot),
                children: "● Modified"
            }),
            p.offline && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: segment,
                children: "Offline"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: region,
                onMouseEnter: p.onPause,
                onMouseLeave: p.onResume,
                onFocus: p.onPause,
                onBlur: p.onResume,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        role: "status",
                        "aria-live": "polite",
                        "data-testid": "files-status-message",
                        className: ((_p_message = p.message) === null || _p_message === void 0 ? void 0 : _p_message.kind) === "error" ? errorText : undefined,
                        children: (_p_message1 = p.message) === null || _p_message1 === void 0 ? void 0 : _p_message1.text
                    }),
                    p.message && p.onDismissMessage && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: chip,
                        "aria-label": "Dismiss message",
                        onClick: p.onDismissMessage,
                        children: "\xd7"
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@tanstack+react-virtual@3.13.25+498059a1009c1789/node_modules/@tanstack/react-virtual/dist/esm/index.js + 3 modules
var esm = __webpack_require__(28396);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/connect-error.js
var connect_error = __webpack_require__(45640);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/code.js
var code = __webpack_require__(16294);
// EXTERNAL MODULE: ./src/lib/api/transport.ts
var transport = __webpack_require__(21694);
;// ./src/lib/files/searchClient.ts
// Streaming content search over plain Connect fetch streaming (ADR-030 decision 4, path a).
// Hooks depend on the SearchStream port so tests drive a hand-written stream.



/** The default per-file scan cap (ADR-030); a hit at or past this offset can only come from a large-file search. */ const SEARCH_FILE_CAP_BYTES = 2 * 1024 * 1024;
/** `byteOffset` is the only size signal a hit carries, so files over the cap with an early match show no badge. */ const isLargeFileHit = (hit)=>hit.byteOffset >= SEARCH_FILE_CAP_BYTES;
/** Longer patterns are refused by the server; a shorter one is never sent. */ const MIN_QUERY_LENGTH = 2;
/** Per-browser-tab id: it only keys newest-wins on the server, never identity. */ function newClientId() {
    return typeof crypto !== "undefined" && "randomUUID" in crypto ? crypto.randomUUID() : "c".concat(Math.random().toString(36).slice(2)).concat(Date.now().toString(36));
}
function connectSearchStream(sessionId, clientId) {
    return async function*(q, signal) {
        const client = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
        const request = {
            sessionId,
            clientId,
            options: {
                pattern: q.pattern,
                matchMode: q.regex ? session_pb/* SearchMatchMode */.xUZ.REGEX : session_pb/* SearchMatchMode */.xUZ.LITERAL,
                caseMode: q.caseSensitive ? session_pb/* SearchCaseMode */.wCy.SENSITIVE : session_pb/* SearchCaseMode */.wCy.INSENSITIVE,
                wordMode: q.wholeWord ? session_pb/* SearchWordMode */.SjT.WHOLE : session_pb/* SearchWordMode */.SjT.ANY,
                scope: {
                    includeGlobs: q.include,
                    excludeGlobs: q.exclude,
                    includeIgnored: q.includeIgnored
                },
                largeFiles: q.largeFiles
            }
        };
        for await (const msg of client.searchContent(request, {
            signal
        })){
            const m = msg.message;
            if (m.case === "queued") yield {
                type: "queued",
                ahead: m.value.ahead
            };
            else if (m.case === "batch") {
                yield {
                    type: "batch",
                    matches: m.value.matches.map((x)=>({
                            path: x.path,
                            line: x.line,
                            text: x.text,
                            start: x.startUtf16,
                            end: x.endUtf16,
                            byteOffset: Number(x.byteOffset)
                        }))
                };
            } else if (m.case === "stats") {
                const s = m.value;
                yield {
                    type: "stats",
                    filesScanned: s.filesScanned,
                    binarySkipped: s.binarySkipped,
                    truncated: s.truncated,
                    durationMs: Number(s.durationMs),
                    largeFilesSkipped: s.largeFilesSkipped
                };
            }
        }
    };
}
/** Maps a stream failure to a UI state; `message` is only set where the server text is meant for the user. */ function classifySearchError(err) {
    if (!(err instanceof connect_error/* ConnectError */.T)) return {
        kind: "error"
    };
    switch(err.code){
        case code/* Code */.C.ResourceExhausted:
            return {
                kind: "busy"
            };
        case code/* Code */.C.InvalidArgument:
            return {
                kind: "invalid",
                message: err.rawMessage
            };
        case code/* Code */.C.FailedPrecondition:
            {
                if (err.findDetails(session_pb/* ReadOnlyReasonDetailSchema */.j1A).some((d)=>d.reason === session_pb/* ReadOnlyReason */.VNo.FLAG_OFF)) return {
                    kind: "flag_off"
                };
                return err.rawMessage.includes("WorktreeMissing") ? {
                    kind: "worktree"
                } : {
                    kind: "error"
                };
            }
        case code/* Code */.C.Canceled:
            return {
                kind: "cancelled"
            };
        default:
            return {
                kind: "error"
            };
    }
}

;// ./src/lib/hooks/useFirstTapActivation.ts
/* __next_internal_client_entry_do_not_use__ TAP_SLOP_PX,useFirstTapActivation auto */ 
/** Beyond this the gesture is a scroll, not a tap (ux.md 3.5). */ const TAP_SLOP_PX = 10;
const CLICK_SWALLOW_MS = 600;
const rowKeyOf = (target)=>{
    var _target_closest, _target_closest1;
    return target === null || target === void 0 ? void 0 : (_target_closest1 = target.closest) === null || _target_closest1 === void 0 ? void 0 : (_target_closest = _target_closest1.call(target, "[data-row-key]")) === null || _target_closest === void 0 ? void 0 : _target_closest.dataset.rowKey;
};
/**
 * One tap opens a row even while the soft keyboard is up: the row is captured on pointerdown (the input is
 * blurred then, so the keyboard closes), and activated on pointerup without hit-testing again, because the
 * viewport relayout between the two can move a different row under the finger. A pointer that travels more
 * than 10 px is a scroll. A click with no pointer sequence (assistive technology) still activates.
 * Spread the result on the element containing rows that carry `data-row-key`.
 */ function useFirstTapActivation(onActivate) {
    const down = (0,react.useRef)(null);
    const swallowClick = (0,react.useRef)(false);
    const swallowTimer = (0,react.useRef)(null);
    const onPointerDown = (0,react.useCallback)((e)=>{
        const key = rowKeyOf(e.target);
        if (!key) return;
        down.current = {
            key,
            x: e.clientX,
            y: e.clientY,
            pointerId: e.pointerId
        };
        const focused = document.activeElement;
        if (focused instanceof HTMLInputElement || focused instanceof HTMLTextAreaElement) focused.blur();
    }, []);
    const onPointerMove = (0,react.useCallback)((e)=>{
        const d = down.current;
        if (d && d.pointerId === e.pointerId && Math.hypot(e.clientX - d.x, e.clientY - d.y) > TAP_SLOP_PX) down.current = null;
    }, []);
    const onPointerUp = (0,react.useCallback)((e)=>{
        const d = down.current;
        down.current = null;
        if (!d || d.pointerId !== e.pointerId || Math.hypot(e.clientX - d.x, e.clientY - d.y) > TAP_SLOP_PX) return;
        swallowClick.current = true;
        if (swallowTimer.current !== null) clearTimeout(swallowTimer.current);
        swallowTimer.current = setTimeout(()=>{
            swallowClick.current = false;
        }, CLICK_SWALLOW_MS);
        onActivate(d.key);
    }, [
        onActivate
    ]);
    const onPointerCancel = (0,react.useCallback)(()=>{
        down.current = null;
    }, []);
    const onClick = (0,react.useCallback)((e)=>{
        if (swallowClick.current) {
            swallowClick.current = false;
            return;
        }
        const key = rowKeyOf(e.target);
        if (key) onActivate(key);
    }, [
        onActivate
    ]);
    return {
        onPointerDown,
        onPointerMove,
        onPointerUp,
        onPointerCancel,
        onClick
    };
}

;// ./src/components/sessions/files/searchRows.ts

const groupKey = (path)=>"g:".concat(path);
const hitKey = (hitIndex)=>"h:".concat(hitIndex);
const DIRTY_NOTE = "buffer has unsaved changes; line may differ";
const LARGE_FILE_BADGE = "large file";
/** Files in first-seen order, each followed by its hits unless collapsed; hit numbering ignores collapsing. */ function buildRows(hits, collapsed) {
    const groups = new Map();
    hits.forEach((h, i)=>{
        const list = groups.get(h.path);
        if (list) list.push(i);
        else groups.set(h.path, [
            i
        ]);
    });
    const rows = [];
    let g = 0;
    for (const [path, indexes] of groups){
        g += 1;
        const isCollapsed = collapsed.has(path);
        rows.push({
            kind: "group",
            key: groupKey(path),
            path,
            count: indexes.length,
            posinset: g,
            setsize: groups.size,
            collapsed: isCollapsed
        });
        if (isCollapsed) continue;
        indexes.forEach((hitIndex, k)=>{
            rows.push({
                kind: "hit",
                key: hitKey(hitIndex),
                hit: hits[hitIndex],
                hitIndex,
                posinset: k + 1,
                setsize: indexes.length,
                result: hitIndex + 1,
                total: hits.length
            });
        });
    }
    return rows;
}
/** The name a screen reader gets; the `<mark>` stays inside the visible text (ux.md 3.5). */ function searchRows_accessibleName(row) {
    let dirty = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : false, collapsed = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : false;
    if (row.kind === "group") {
        const noun = row.count === 1 ? "match" : "matches";
        return "".concat(row.path, ", ").concat(row.count, " ").concat(noun, ", ").concat(collapsed || row.collapsed ? "collapsed" : "expanded");
    }
    const large = isLargeFileHit(row.hit) ? ", ".concat(LARGE_FILE_BADGE) : "";
    const base = "".concat(row.hit.path, " line ").concat(row.hit.line, ", ").concat(row.hit.text, ", match ").concat(row.posinset, " of ").concat(row.setsize, ", result ").concat(row.result, " of ").concat(row.total).concat(large);
    return dirty ? "".concat(base, ", ").concat(DIRTY_NOTE) : base;
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2Ffiles%2FSearchPanel.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA8VY646bOBT-P0_hP1WDWkZAruNqpX2P1apy8ElwAza1TSZJNe9eYSBgh0uSrrQaKZMYn9t3znd8zOv3ecwStQzQrxeEKFN5Ss4Y7VI4fXtB5r9PmYRYM8ExikVaZLx5glFYfs0Y9xNg-0RjFDQL74zqxPz-eHmtjYT9RvYkxyjKzfecUMr4HqNFfkKram0rJAXpb4XWIsMozE9IiZRRdCRy5vvfwze4RPICXtdWZGxVohhxwcGoIvFhL0XBKUZaEq5yIoHr8lEsUiGxozP0TKyCa4wYT0Ay3Sz4il0Ao3De43gYVItxIVWpNBeMa5A9wUTXYCx3OnH8QyQjvoIUYg30r89aFvD5XxNdr8uBd2PG792YWXjNHy0Ak7WVE_ym_N0qXYwkfFGJkpTtuc80ZAqjGCqYWg1Lo8Eptra2OqaX-aky32Z9qFIOXYgkoaxQLjjzlefWi1MZxBuumu2dZdPGuXoSqTpD77LcWH52la4tDgyhsX8KjXvYE0zDUNPeLO1IxtKzqyQqPCfv0capuqjtFTeM66BRMSmXoJRDpJE0r7voDPJoqBDeLI5t_pOedD-qGk7apxALSSr2FpyCTFllt6c5WYgu7Ap9e6pBOEpIReeOq2Gdt75IIwu97QhFVuMUuYVmsNu39mJjr7evP6JwyOXNs6ymtl_RM37ZyMKfR2or3NkKwyEf549jd8-B1Dqy_98c6UuvEVGaSOuET_qOOGueEkeQu1S8-2eMSKGFoapQrDIsISWaHQ2nRaFLejfNpTXCjBGRk5jpM0bB63LZffyjwqlXZ911wyD41BU5OCJkq0RaaCOiRV67nsKuicLSU7bBU4m-yct1Wjk9xpctUdC0s7sJcdv2RtpsAz1GCaMUuIXAyGg2eaJcMzV0LGfdbb7Y7RRojPzQLvADTkoXJ2xuLHambRN-r8tsFQTj407s3TGsZVPD2i2a9SHVPoA0ZbliylRMwjT4KiexqWi3EfLhITi6npHXEBdViGbtWCaOa58XGUgWY6TJtkiJLBdU14S4y8SEOguHeZxcozZwYiRLB68tRSWS8YNzc8qnkJ0eoCwwcwkP56P15udEuZ2HJ6JL37Dp3v0C5FS5HE5D2FH44Izr3tyCZtLumU-GU6OGfbt4D2vT_UPOwJTQeweb6H81CKZFj12lxxK8Hhl5rVZTPDnxOnOttTA2twbNAPB3BpQRNKs3lcczkQo844_9jsA-cBd1DXx09i1H9tkYr26F16PCHR73Wd78ifChFh4onpv-KjOSujp0raM953fsBLSSl5VDTWWVia7easQkjWerco78goAfZ4rswCcSiM-4Al2_lviKgvzkeZXoxWeclu1tvmx03UPpqYvr-rqrJByhZUsLrhRBcr8ls-Cr-XuNlp4bftHB30F6qGw-fgPHgLSbUxMAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2Ffiles_2FSearchPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA8VY646bOBT_P0_hP1WDWkZAruNqpX2P1apy8ElwAza1TSZJNe9eYSBgh0uSrrQaKZMYn9t3znd8zOv3ecwStQzQrxeEKFN5Ss4Y7VI4fXtB5r9PmYRYM8ExikVaZLx5glFYfs0Y9xNg_0RjFDQL74zqxPz_eHmtjYT9RvYkxyjKzfecUMr4HqNFfkKram0rJAXpb4XWIsMozE9IiZRRdCRy5vvfwze4RPICXtdWZGxVohhxwcGoIvFhL0XBKUZaEq5yIoHr8lEsUiGxozP0TKyCa4wYT0Ay3Sz4il0Ao3De43gYVItxIVWpNBeMa5A9wUTXYCx3OnH8QyQjvoIUYg30r89aFvD5XxNdr8uBd2PG792YWXjNHy0Ak7WVE_ym_N0qXYwkfFGJkpTtuc80ZAqjGCqYWg1Lo8Eptra2OqaX_aky32Z9qFIOXYgkoaxQLjjzlefWi1MZxBuumu2dZdPGuXoSqTpD77LcWH52la4tDgyhsX8KjXvYE0zDUNPeLO1IxtKzqyQqPCfv0capuqjtFTeM66BRMSmXoJRDpJE0r7voDPJoqBDeLI5t_pOedD_qGk7apxALSSr2FpyCTFllt6c5WYgu7Ap9e6pBOEpIReeOq2Gdt75IIwu97QhFVuMUuYVmsNu39mJjr7evP6JwyOXNs6ymtl_RM37ZyMKfR2or3NkKwyEf549jd8_B1Dqy_98c6UuvEVGaSOuET_qOOGueEkeQu1S8_2eMSKGFoapQrDIsISWaHQ2nRaFLejfNpTXCjBGRk5jpM0bB63LZffyjwqlXZ911wyD41BU5OCJkq0RaaCOiRV67nsKuicLSU7bBU4m_yct1Wjk9xpctUdC0s7sJcdv2RtpsAz1GCaMUuIXAyGg2eaJcMzV0LGfdbb7Y7RRojPzQLvADTkoXJ2xuLHambRN_r8tsFQTj407s3TGsZVPD2i2a9SHVPoA0ZbliylRMwjT4KiexqWi3EfLhITi6npHXEBdViGbtWCaOa58XGUgWY6TJtkiJLBdU14S4y8SEOguHeZxcozZwYiRLB68tRSWS8YNzc8qnkJ0eoCwwcwkP56P15udEuZ2HJ6JL37Dp3v0C5FS5HE5D2FH44Izr3tyCZtLumU_GU6OGfbt4D2vT_UPOwJTQeweb6H81CKZFj12lxxK8Hhl5rVZTPDnxOnOttTA2twbNAPB3BpQRNKs3lcczkQo844_9jsA_cBd1DXx09i1H9tkYr26F16PCHR73Wd78ifChFh4onpv_KjOSujp0raM953fsBLSSl5VDTWWVia7easQkjWerco78goAfZ4rswCcSiM_4Al2_lviKgvzkeZXoxWeclu1tvmx03UPpqYvr_rqrJByhZUsLrhRBcr8ls_Cr_XuNlp4bftHB30F6qGw_fgPHgLSbUxMAAA_22_7D = __webpack_require__(12507);
;// ./src/components/sessions/files/SearchPanel.css.ts


var checkLabel = '_3cihs5b';
var SearchPanel_css_chip = '_3cihs5t';
var chipButton = '_3cihs5u';
var count = '_3cihs5n';
var dirtyNote = '_3cihs5s';
var emptyMessage = '_3cihs5g';
var fieldLabel = '_3cihs5a';
var filters = '_3cihs59';
var SearchPanel_css_form = '_3cihs53';
var groupPath = '_3cihs5m';
var groupRow = '_3cihs5l';
var SearchPanel_css_header = '_3cihs51';
var headerTab = '_3cihs52';
var helper = '_3cihs5f';
var hitText = '_3cihs5p';
var inlineError = '_3cihs5d';
var largeBadge = '_3cihs5r';
var lineNo = '_3cihs5o';
var mark = '_3cihs5q';
var note = '_3cihs5e';
var SearchPanel_css_panel = '_3cihs50';
var queryInput = '_3cihs55';
var queryRow = '_3cihs54';
var rowBase = '_3cihs5k';
var SearchPanel_css_status = '_3cihs5c';
var SearchPanel_css_textButton = '_3cihs58';
var toggle = '_3cihs57';
var toggleRow = '_3cihs56';
var tree = '_3cihs5h';
var treeDimmed = '_3cihs5i';
var treeInner = '_3cihs5j';
;// ./src/components/sessions/files/SearchResults.tsx
/* __next_internal_client_entry_do_not_use__ rowDomId,SearchResults,QuickfixChip auto */ 






const rowDomId = (treeId, key)=>"".concat(treeId, "-").concat(key);
/** Splits `text` around the match range (UTF-16 indexes, as the server sends them). */ function Highlighted(param) {
    let { hit } = param;
    const { text, start, end } = hit;
    if (start < 0 || end > text.length || start >= end) return /*#__PURE__*/ (0,jsx_runtime.jsx)(jsx_runtime.Fragment, {
        children: text
    });
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            text.slice(0, start),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("mark", {
                className: mark,
                children: text.slice(start, end)
            }),
            text.slice(end)
        ]
    });
}
/** Virtualized `tree` of file groups and hit rows; every row carries explicit level, setsize and posinset. */ const SearchResults = /*#__PURE__*/ (0,react.forwardRef)(function SearchResults(p, ref) {
    const scrollRef = (0,react.useRef)(null);
    const rowHeight = p.coarse ? 44 : 24;
    const virtualizer = (0,esm/* useVirtualizer */.Te)({
        count: p.rows.length,
        getScrollElement: ()=>scrollRef.current,
        estimateSize: ()=>rowHeight,
        overscan: 8,
        getItemKey: (i)=>{
            var _p_rows_i;
            var _p_rows_i_key;
            return (_p_rows_i_key = (_p_rows_i = p.rows[i]) === null || _p_rows_i === void 0 ? void 0 : _p_rows_i.key) !== null && _p_rows_i_key !== void 0 ? _p_rows_i_key : i;
        }
    });
    // A pane hidden with display:none loses its scroll offset; remember it and put it back when the list shows again.
    const savedTop = (0,react.useRef)(0);
    (0,react.useEffect)(()=>{
        const el = scrollRef.current;
        if (!el || typeof ResizeObserver === "undefined") return;
        let hidden = false;
        const ro = new ResizeObserver((entries)=>{
            const height = entries[entries.length - 1].contentRect.height;
            if (height > 0 && hidden) el.scrollTop = savedTop.current;
            hidden = height === 0;
        });
        ro.observe(el);
        return ()=>ro.disconnect();
    }, []);
    (0,react.useImperativeHandle)(ref, ()=>({
            scrollToKey (key) {
                const i = p.rows.findIndex((r)=>r.key === key);
                if (i >= 0) virtualizer.scrollToIndex(i, {
                    align: "auto"
                });
            }
        }), [
        p.rows,
        virtualizer
    ]);
    const activateKey = (0,react.useCallback)((key, via)=>{
        const row = p.rows.find((r)=>r.key === key);
        if (!row) return;
        if (row.kind === "group") p.onToggleGroup(row.path);
        else p.onActivate(row.hit, row.hitIndex, via);
    }, [
        p
    ]);
    const tap = useFirstTapActivation((0,react.useCallback)((key)=>activateKey(key, "tap"), [
        activateKey
    ]));
    // Rows are focusable on touch devices, where a hardware keyboard may still be attached.
    const onKeyDown = (e)=>{
        var _e_target_closest;
        if (e.key !== "Enter" && e.key !== " ") return;
        const key = (_e_target_closest = e.target.closest("[data-row-key]")) === null || _e_target_closest === void 0 ? void 0 : _e_target_closest.dataset.rowKey;
        if (!key) return;
        e.preventDefault();
        activateKey(key, "enter");
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        ref: scrollRef,
        className: "".concat(tree, " ").concat(p.stale ? treeDimmed : ""),
        id: p.treeId,
        role: "tree",
        "aria-label": "Search results",
        "aria-busy": p.busy,
        ...tap,
        onScroll: (e)=>{
            if (e.currentTarget.clientHeight > 0) savedTop.current = e.currentTarget.scrollTop;
        },
        onKeyDown: onKeyDown,
        "data-testid": "search-results",
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: treeInner,
            style: {
                height: virtualizer.getTotalSize()
            },
            children: virtualizer.getVirtualItems().map((v)=>{
                const row = p.rows[v.index];
                if (!row) return null;
                const style = {
                    transform: "translateY(".concat(v.start, "px)"),
                    height: v.size,
                    ...p.coarse ? {
                        minHeight: 44
                    } : {}
                };
                const common = {
                    id: rowDomId(p.treeId, row.key),
                    "data-row-key": row.key,
                    role: "treeitem",
                    "aria-setsize": row.setsize,
                    "aria-posinset": row.posinset,
                    "aria-selected": p.activeKey === row.key,
                    tabIndex: p.coarse ? -1 : undefined,
                    style
                };
                if (row.kind === "group") {
                    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        ...common,
                        className: "".concat(rowBase, " ").concat(groupRow),
                        "aria-level": 1,
                        "aria-expanded": !row.collapsed,
                        "aria-label": searchRows_accessibleName(row),
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: groupPath,
                                children: [
                                    row.collapsed ? "▸" : "▾",
                                    " ",
                                    row.path
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: count,
                                children: row.count
                            })
                        ]
                    }, row.key);
                }
                const dirty = p.dirtyPaths.has(row.hit.path);
                return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    ...common,
                    className: rowBase,
                    "aria-level": 2,
                    "aria-label": searchRows_accessibleName(row, dirty),
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: lineNo,
                            children: row.hit.line
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: hitText,
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(Highlighted, {
                                hit: row.hit
                            })
                        }),
                        isLargeFileHit(row.hit) && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: largeBadge,
                            children: LARGE_FILE_BADGE
                        }),
                        dirty && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: dirtyNote,
                            children: DIRTY_NOTE
                        })
                    ]
                }, row.key);
            })
        })
    });
});
/** Next/previous-hit chip: a status segment on desktop, a floating 44 px control on touch (ux.md S2). */ function QuickfixChip(param) {
    let { view, stacked = false } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: SearchPanel_css_chip,
        style: stacked ? {
            position: "static"
        } : undefined,
        role: "group",
        "aria-label": "Search hits",
        "data-testid": "quickfix-chip",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                children: [
                    "Hit ",
                    view.position,
                    "/",
                    view.total
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                className: chipButton,
                "aria-label": "Previous hit",
                onClick: ()=>view.step(-1),
                children: "↑"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                className: chipButton,
                "aria-label": "Next hit",
                onClick: ()=>view.step(1),
                children: "↓"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                className: chipButton,
                "aria-label": "Dismiss hit navigation",
                onClick: view.dismiss,
                children: "\xd7"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                role: "status",
                "aria-live": "polite",
                children: view.note
            })
        ]
    });
}

;// ./src/components/sessions/files/FilesV2Footer.tsx
/* __next_internal_client_entry_do_not_use__ FilesV2Footer auto */ 





const languageName = (id)=>id === "text" ? "Plain Text" : id.charAt(0).toUpperCase() + id.slice(1);
/** Status bar and the mobile toast for the active buffer. */ function FilesV2Footer(param) {
    let { v2, quickfix } = param;
    const { buffers, status, feedback } = v2;
    const active = buffers.active;
    const file = v2.bridge.windowed;
    const windowed = (0,useWindowedState/* useWindowedState */.O)(file);
    if (!active) return null;
    const loaded = active.status === "loaded" ? active : null;
    var _loaded_readOnly;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            quickfix && /*#__PURE__*/ (0,jsx_runtime.jsx)(QuickfixChip, {
                view: quickfix
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(StatusBar, {
                line: status.line,
                col: status.col,
                eol: status.eol,
                encoding: loaded ? "UTF-8" : null,
                language: languageName((0,FileContentViewer/* detectLanguage */.od)(active.path)),
                readOnly: (_loaded_readOnly = loaded === null || loaded === void 0 ? void 0 : loaded.readOnly) !== null && _loaded_readOnly !== void 0 ? _loaded_readOnly : "none",
                windowed: file ? windowed : null,
                dirty: !!(loaded === null || loaded === void 0 ? void 0 : loaded.dirty),
                message: feedback.message,
                onDismissMessage: feedback.dismiss,
                onPause: feedback.pause,
                onResume: feedback.resume
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(FeedbackToast, {
                toast: feedback.toast,
                onDismiss: feedback.dismiss,
                onPause: feedback.pause,
                onResume: feedback.resume
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/utils/gitStatus.ts
var gitStatus = __webpack_require__(41852);
// EXTERNAL MODULE: ./src/components/sessions/FileTree.tsx + 1 modules
var FileTree = __webpack_require__(23471);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FFilesTab.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA8VXy3KbMBTd-yvURcfxAg9gx0mU8V90l8lkBLoY1ULSSMKPdPLvHR6xAQtj0mm7sxGSzj3n3Afzt2AZKp1kPvo1QYgyozg5YpRwODxPEEqBbVKLUeD734v_cgc64XKPUcooBVE8i0i83WiZC4rRjug7z3sLnuA91O8BnRUvxFJYwgRozx4VYMQEZwI8w96hvSxIBhgljIP5QaLnycdkfsL3QoklHidHmdv1NJMR4zB9LVEXYD3KNMSWSYFRLHmeidb2wB1e3856xaSaiS1Gfhmm1BS0p2tC1AEZyRnthAwzN003hNIBu2fUphXz6BvLlNSWCHuJREgBn4gxCoqfGRPep3L-LXdXTxURsJ4CZVbqSzwn8qoLG4eGTbx-B6zTMc4ACtQ9h-yYYRHjzB6djC7ONmhJVp8WLtXhfO35wjGK3mByLnV3za-Wcm2KNSWZsKDHmWExQP5yYP2q4nGqZQbrKYcNiY_Na5fuhCGcbYTHLGQGoxiqcBDaEIVRzbIilDKxwehRHVAQ1tQ36LOaCKOIhqabz6pcsNXDbVZym0hhy0qCUbBoyuxF0lqZXde1wdP9yUN1ErmMe3MBGRYAC2nvXHk3Qx1Mo7XtHtCoJNerw2q86KuO6PfqUAg_Uoer-cVmroL8McFaSlszAIn1UiIo0Eb0K2d30HLvadiBNm0yH7oGaNajAYhk1nTyQKR11SGU5eaUNU6LB3Dh8bDD9-LMt8xt0VcdPnnAiYxzUwZYX9-bUq1tWHESQyo5BV3udm4L29sex7uoztxmoMEVXkJ3WXW2gJRZ8IwicUnMXhPVBovTItH7gwugGx0TKrcvxSiznsYpxNtIHupJxAGJxEWsN_H91NPGTmKH6oAeLqla3FxjRzlz8RUFysnuNDXOl-0AK7LL0ocpMyTiQGeVLfuTa9vfX7viPJ1OLQ-VisTlzODP75t4KSQk57a1lfSQP86TQ3aL_nGXORfDeKCX0L8zR9BRxeBnbixLjl7xSQDCNpf-31ix-pOxAlodWDWCN8ATjIzVYOP0en8BZwdEKCN6w0RVF0pGGhcnX_niGfx-SF4qm7463fIb-aXst04OAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FFilesTab_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA8VXy3KbMBTd_yvURcfxAg9gx0mU8V90l8lkBLoY1ULSSMKPdPLvHR6xAQtj0mm7sxGSzj3n3Afzt2AZKp1kPvo1QYgyozg5YpRwODxPEEqBbVKLUeD734v_cgc64XKPUcooBVE8i0i83WiZC4rRjug7z3sLnuA91O8BnRUvxFJYwgRozx4VYMQEZwI8w96hvSxIBhgljIP5QaLnycdkfsL3QoklHidHmdv1NJMR4zB9LVEXYD3KNMSWSYFRLHmeidb2wB1e3856xaSaiS1Gfhmm1BS0p2tC1AEZyRnthAwzN003hNIBu2fUphXz6BvLlNSWCHuJREgBn4gxCoqfGRPep3L_LXdXTxURsJ4CZVbqSzwn8qoLG4eGTbx_B6zTMc4ACtQ9h_yYYRHjzB6djC7ONmhJVp8WLtXhfO35wjGK3mByLnV3za_Wcm2KNSWZsKDHmWExQP5yYP2q4nGqZQbrKYcNiY_Na5fuhCGcbYTHLGQGoxiqcBDaEIVRzbIilDKxwehRHVAQ1tQ36LOaCKOIhqabz6pcsNXDbVZym0hhy0qCUbBoyuxF0lqZXde1wdP9yUN1ErmMe3MBGRYAC2nvXHk3Qx1Mo7XtHtCoJNerw2q86KuO6PfqUAg_Uoer_cVmroL8McFaSlszAIn1UiIo0Eb0K2d30HLvadiBNm0yH7oGaNajAYhk1nTyQKR11SGU5eaUNU6LB3Dh8bDD9_LMt8xt0VcdPnnAiYxzUwZYX9_bUq1tWHESQyo5BV3udm4L29sex7uoztxmoMEVXkJ3WXW2gJRZ8IwicUnMXhPVBovTItH7gwugGx0TKrcvxSiznsYpxNtIHupJxAGJxEWsN_H91NPGTmKH6oAeLqla3FxjRzlz8RUFysnuNDXOl_0AK7LL0ocpMyTiQGeVLfuTa9vfX7viPJ1OLQ_VisTlzODP75t4KSQk57a1lfSQP86TQ3aL_nGXORfDeKCX0L8zR9BRxeBnbixLjl7xSQDCNpf_31ix_pOxAlodWDWCN8ATjIzVYOP0en8BZwdEKCN6w0RVF0pGGhcnX_niGfx_SF4qm7463fIb_aXst04OAAA_22_7D = __webpack_require__(55100);
;// ./src/components/sessions/FilesTab.css.ts


var FilesTab_css_container = '_142prfm0';
var contentPane = '_142prfm5';
var expandStrip = '_142prfm3';
var leftView = '_142prfmf';
var mobileBackButton = '_142prfm4';
var mobileSearchButton = '_142prfmd';
var searchCount = '_142prfma';
var FilesTab_css_searchInput = '_142prfm7';
var FilesTab_css_toolbar = '_142prfm6';
var toolbarButton = '_142prfm9';
var toolbarButtonMobileHidden = '_142prfmc';
var toolbarDivider = '_142prfme';
var toolbarLabel = '_142prfm8';
var treePane = '_142prfm1';
var treePaneCollapsed = '_142prfm2';
var treeWrapper = '_142prfmb';
;// ./src/components/sessions/FilesTreeToolbar.tsx
/* __next_internal_client_entry_do_not_use__ FilesTreeToolbar auto */ 

const desktopOnly = "".concat(toolbarButton, " ").concat(toolbarButtonMobileHidden);
/** The Explorer pane's search box and option row. */ function FilesTreeToolbar(p) {
    const nextSort = p.sortBy === "name" ? "type" : "name";
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: FilesTab_css_toolbar,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                ref: p.searchInputRef,
                type: "search",
                className: FilesTab_css_searchInput,
                placeholder: "Search files…",
                value: p.searchTerm,
                onChange: (e)=>p.onSearchTerm(e.target.value),
                onKeyDown: (e)=>{
                    if (e.key === "Escape") {
                        var _p_searchInputRef_current;
                        p.onSearchTerm("");
                        (_p_searchInputRef_current = p.searchInputRef.current) === null || _p_searchInputRef_current === void 0 ? void 0 : _p_searchInputRef_current.blur();
                    }
                },
                enterKeyHint: "search",
                "aria-label": "Search files"
            }),
            p.searchResultCount !== null && p.searchTerm.length >= 2 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                className: searchCount,
                title: p.searchResultTruncated ? "Results truncated at 500" : undefined,
                "aria-live": "polite",
                "aria-atomic": "true",
                children: [
                    p.searchResultCount,
                    p.searchResultTruncated ? "+" : "",
                    " match",
                    p.searchResultCount !== 1 ? "es" : ""
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                className: toolbarLabel,
                title: "Show gitignored files",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        type: "checkbox",
                        checked: p.includeIgnored,
                        onChange: (e)=>p.onIncludeIgnored(e.target.checked)
                    }),
                    "Ignored"
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                className: toolbarLabel,
                title: "Show only files with git changes",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        type: "checkbox",
                        checked: p.filterChangedOnly,
                        onChange: (e)=>p.onFilterChangedOnly(e.target.checked)
                    }),
                    "Changed"
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                className: desktopOnly,
                onClick: p.onToggleSort,
                title: "Sort by ".concat(nextSort),
                "aria-label": "Sort by ".concat(nextSort),
                children: [
                    "Sort: ",
                    p.sortBy === "name" ? "Name" : "Type"
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                className: desktopOnly,
                onClick: ()=>{
                    var _p_fileTreeRef_current;
                    return (_p_fileTreeRef_current = p.fileTreeRef.current) === null || _p_fileTreeRef_current === void 0 ? void 0 : _p_fileTreeRef_current.collapseAll();
                },
                title: "Collapse all directories",
                "aria-label": "Collapse all directories",
                children: "⊟"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: toolbarDivider
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                className: desktopOnly,
                onClick: p.collapsed ? p.onExpand : p.onCollapse,
                title: p.collapsed ? "Expand file tree panel" : "Collapse file tree panel",
                "aria-label": p.collapsed ? "Expand file tree panel" : "Collapse file tree panel",
                children: p.collapsed ? "⊞" : "⊠"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                className: toolbarButton,
                onClick: p.onRefreshStatus,
                title: "Refresh git status",
                "aria-label": "Refresh git status",
                disabled: p.vcsLoading,
                children: p.vcsLoading ? "⟳" : "↺"
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FRecentFilesSection.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA82U247bIBCG7_MUc1NpI4UI23HakodZYXtso2BAgNduqr57BT4kTtuVWlVVLxnm8M_HMMfXZMyGmucUvu4ACm0rtKTQ3uuOQWJGcFqKCt64fSHkNfmMt9TecH_ZARheVUI1q_fJjMHccdsItbV-2x2XQkksVAlnJP_CoJYYo7gUjSLCY-cYlKg82mBuuFkTD6LyLYOE0g_hWGvliRM3ZJAkk4fH0RNvuXK1th2D3hi0JXf4IDemg09mhHQKKrXUlj11mMYOJXqPljjDyxhJjzTHLtz0LthRYukZKK1ihbK3LqQyWiz6C15eG6t7VTGIwgy3qPxlZX2PjuIjBgYSa7-hlkZqARZxrRXqyoA-Q6BTP1IoJC2KpvUMkk2W7PfZn3_KfmWZTSyDbSmZzue_TWOdTssr0bt1LOb3E6pFK_wzlOxp_DLW6je007Q_yNk-f3Ldb6JO_xG4Sek0e0IrUjQHsE3BX070EOocIM3zA9Bjmu_3_xLt6Y_Q5r-Y7AXc-eFrz6LvpN-Tc54SB49hJnymMXVQWUs9MGhFVaFa898vUEphnHBRSSs8xiWAAeJgubnMkp8_2Md7ze1memfJ_Nj6vEHD2yz78zvmbqW_qAUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FRecentFilesSection_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA82U247bIBCG7_MUc1NpI4UI23HakodZYXtso2BAgNduqr57BT4kTtuVWlVVLxnm8M_HMMfXZMyGmucUvu4ACm0rtKTQ3uuOQWJGcFqKCt64fSHkNfmMt9TecH_ZARheVUI1q_fJjMHccdsItbV_2x2XQkksVAlnJP_CoJYYo7gUjSLCY_cYlKg82mBuuFkTD6LyLYOE0g_hWGvliRM3ZJAkk4fH0RNvuXK1th2D3hi0JXf4IDemg09mhHQKKrXUlj11mMYOJXqPljjDyxhJjzTHLtz0LthRYukZKK1ihbK3LqQyWiz6C15eG6t7VTGIwgy3qPxlZX2PjuIjBgYSa7_hlkZqARZxrRXqyoA_Q6BTP1IoJC2KpvUMkk2W7PfZn3_KfmWZTSyDbSmZzue_TWOdTssr0bt1LOb3E6pFK_wzlOxp_DLW6je007Q_yNk_f3Ldb6JO_xG4Sek0e0IrUjQHsE3BX070EOocIM3zA9Bjmu_3_xLt6Y_Q5r_Y7AXc_eFrz6LvpN_Tc54SB49hJnymMXVQWUs9MGhFVaFa898vUEphnHBRSSs8xiWAAeJgubnMkp8_2Md7ze1memfJ_Nj6vEHD2yz78zvmbqW_qAUAAA_22_7D = __webpack_require__(60859);
;// ./src/components/sessions/RecentFilesSection.css.ts


var chevron = '_1x3wfa52';
var RecentFilesSection_css_container = '_1x3wfa50';
var entry = '_1x3wfa53';
var entryDir = '_1x3wfa57';
var entryIcon = '_1x3wfa55';
var entryName = '_1x3wfa56';
var entrySelected = '_1x3wfa54';
var RecentFilesSection_css_heading = '_1x3wfa51';
;// ./src/components/sessions/RecentFilesSection.tsx




function RecentFilesSection(param) {
    let { paths, selectedPath, onSelect } = param;
    const [collapsed, setCollapsed] = (0,react.useState)(()=>{
        try {
            return localStorage.getItem("filesTab.recentCollapsed") === "true";
        } catch (e) {
            return false;
        }
    });
    const toggle = ()=>{
        setCollapsed((v)=>{
            const next = !v;
            try {
                localStorage.setItem("filesTab.recentCollapsed", String(next));
            } catch (e) {}
            return next;
        });
    };
    if (paths.length === 0) {
        return null;
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: RecentFilesSection_css_container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                className: RecentFilesSection_css_heading,
                onClick: toggle,
                "aria-expanded": !collapsed,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: chevron,
                        children: collapsed ? "▸" : "▾"
                    }),
                    "Recent"
                ]
            }),
            !collapsed && paths.map((path)=>{
                var _path_split_pop;
                const basename = (_path_split_pop = path.split("/").pop()) !== null && _path_split_pop !== void 0 ? _path_split_pop : path;
                var _path_split_slice_;
                const parentDir = path.includes("/") ? (_path_split_slice_ = path.split("/").slice(-2, -1)[0]) !== null && _path_split_slice_ !== void 0 ? _path_split_slice_ : "" : "";
                const icon = (0,fileIcons/* getFileIcon */.I)(basename);
                const isSelected = path === selectedPath;
                return /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                    className: isSelected ? entrySelected : entry,
                    title: path,
                    onClick: ()=>onSelect(path),
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: entryIcon,
                            children: icon
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: entryName,
                            children: basename
                        }),
                        parentDir && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: entryDir,
                            children: parentDir
                        })
                    ]
                }, path);
            })
        ]
    });
}

;// ./src/lib/files/analytics/findFlowMeter.ts
/** Counts discrete user actions from opening Search to opening a hit (ADR-035 section 5). */ class FindFlowMeter {
    /** Search panel opened: starts a flow with the open itself as interaction 1. */ open() {
        this.count = 1;
    }
    /** One tap or chord (typing a query is one); ignored while no flow is running. */ interact() {
        if (this.count !== null) this.count += 1;
    }
    /** The hit tap counts too. Returns the total and resets, or null if no flow was running. */ finish() {
        if (this.count === null) return null;
        const total = this.count + 1;
        this.count = null;
        return total;
    }
    /** Cancel or panel close: discard without emitting. */ cancel() {
        this.count = null;
    }
    constructor(){
        this.count = null;
    }
}

;// ./src/lib/files/largeFilesPref.ts
/* __next_internal_client_entry_do_not_use__ readLargeFiles,writeLargeFiles,useLargeFiles auto */ 
const largeFilesPref_KEY = "ssq.files.search.largeFiles";
const CHANGE = "ssq-files-large-files-change";
/** Set only while storage rejects writes, so the toggle still applies for this session. */ let unsaved = null;
/** Per-device "Include large files" search toggle; off until the user turns it on. */ function readLargeFiles() {
    if (unsaved !== null) return unsaved;
    try {
        return window.localStorage.getItem(largeFilesPref_KEY) === "1";
    } catch (e) {
        return false; // storage unavailable: default
    }
}
function writeLargeFiles(on) {
    try {
        window.localStorage.setItem(largeFilesPref_KEY, on ? "1" : "0");
        unsaved = null;
    } catch (e) {
        unsaved = on;
    }
    window.dispatchEvent(new Event(CHANGE));
}
const subscribe = (cb)=>{
    window.addEventListener(CHANGE, cb);
    window.addEventListener("storage", cb);
    return ()=>{
        window.removeEventListener(CHANGE, cb);
        window.removeEventListener("storage", cb);
    };
};
const useLargeFiles = ()=>(0,react.useSyncExternalStore)(subscribe, readLargeFiles, ()=>false);

;// ./src/lib/hooks/useContentSearch.ts
/* __next_internal_client_entry_do_not_use__ SEARCH_DEBOUNCE_MS,HIDDEN_ABORT_MS,useContentSearch auto */ 




const SEARCH_DEBOUNCE_MS = 300;
const HIDDEN_ABORT_MS = 30000;
const INITIAL = {
    status: "idle",
    ahead: 0,
    hits: [],
    stale: false,
    stats: null,
    message: null,
    answered: null
};
const now = ()=>typeof performance !== "undefined" ? performance.now() : Date.now();
const distinctFiles = (hits)=>new Set(hits.map((h)=>h.path)).size;
/** Debounced, abortable content search: one live stream, request-id guarded, never auto-retried. */ function useContentSearch(param) {
    let { sessionId, enabled = true, stream } = param;
    const analytics = (0,filesAnalytics/* useFilesAnalytics */.R)({
        component: "SearchPanel",
        sessionId
    });
    const { onFlagOff } = useFilesFlags();
    const clientId = (0,react.useRef)("");
    if (!clientId.current) clientId.current = newClientId();
    const run = (0,react.useMemo)(()=>stream !== null && stream !== void 0 ? stream : connectSearchStream(sessionId, clientId.current), [
        stream,
        sessionId
    ]);
    const [state, setState] = (0,react.useState)(INITIAL);
    const live = (0,react.useRef)({
        state,
        analytics,
        run,
        onFlagOff
    });
    live.current = {
        state,
        analytics,
        run,
        onFlagOff
    };
    const requestId = (0,react.useRef)(0);
    const controller = (0,react.useRef)(null);
    const debounce = (0,react.useRef)(null);
    const hiddenTimer = (0,react.useRef)(null);
    const lastQuery = (0,react.useRef)(null);
    const mounted = (0,react.useRef)(true);
    const abort = (0,react.useCallback)(()=>{
        var _controller_current;
        (_controller_current = controller.current) === null || _controller_current === void 0 ? void 0 : _controller_current.abort();
        controller.current = null;
        requestId.current += 1;
    }, []);
    const clearDebounce = (0,react.useCallback)(()=>{
        if (debounce.current !== null) clearTimeout(debounce.current);
        debounce.current = null;
    }, []);
    const start = (0,react.useCallback)((q)=>{
        abort();
        const id = requestId.current;
        const ctl = new AbortController();
        controller.current = ctl;
        lastQuery.current = q;
        const startedAt = now();
        const { analytics: a } = live.current;
        a.emit("files.search.run", {
            query_len: (0,events/* bucketQueryLen */.yo)(q.pattern.length),
            opts: (0,events/* searchOpts */.nO)({
                regex: q.regex,
                caseSensitive: q.caseSensitive,
                wholeWord: q.wholeWord,
                globs: q.include.length + q.exclude.length > 0,
                ignored: q.includeIgnored,
                large: q.largeFiles
            })
        });
        setState((s)=>({
                ...s,
                status: "searching",
                ahead: 0,
                message: null,
                stale: s.hits.length > 0
            }));
        let received = [];
        let firstMs = null;
        let queued = false;
        const finish = (outcome, stats)=>{
            var _stats_filesScanned;
            const scale = (0,events/* bucketScale */.v4)((_stats_filesScanned = stats === null || stats === void 0 ? void 0 : stats.filesScanned) !== null && _stats_filesScanned !== void 0 ? _stats_filesScanned : 0);
            const labels = {
                results: (0,events/* bucketResults */.Du)(received.length),
                files: (0,events/* bucketFiles */.QO)(distinctFiles(received)),
                queued: String(queued)
            };
            a.emit("files.search.results", {
                outcome,
                ...labels
            });
            if (firstMs !== null) a.emitTimed("files.perf.search_first_result_ms", firstMs, {
                scale,
                queued: labels.queued
            });
            const total = outcome === "ok" || outcome === "truncated" ? outcome : outcome === "empty" ? "ok" : "error";
            a.emitTimed("files.perf.search_total_ms", now() - startedAt, {
                outcome: total,
                scale
            });
        };
        const isLive = ()=>requestId.current === id && !ctl.signal.aborted && mounted.current;
        void (async ()=>{
            try {
                for await (const ev of live.current.run(q, ctl.signal)){
                    if (!isLive()) return;
                    if (ev.type === "queued") {
                        queued = true;
                        a.emit("files.search.queued", {
                            ahead: ev.ahead >= 2 ? "2+" : "1"
                        });
                        setState((s)=>({
                                ...s,
                                status: "queued",
                                ahead: ev.ahead
                            }));
                    } else if (ev.type === "batch") {
                        const first = received.length === 0;
                        if (firstMs === null) firstMs = now() - startedAt;
                        received = received.concat(ev.matches);
                        setState((s)=>({
                                ...s,
                                status: "searching",
                                ahead: 0,
                                hits: first ? ev.matches : s.hits.concat(ev.matches),
                                stale: false,
                                answered: q
                            }));
                    } else {
                        const hits = received;
                        setState((s)=>({
                                ...s,
                                status: "done",
                                ahead: 0,
                                hits,
                                stale: false,
                                stats: ev,
                                answered: q
                            }));
                        finish(ev.truncated ? "truncated" : hits.length === 0 ? "empty" : "ok", ev);
                    }
                }
            } catch (err) {
                if (!isLive()) return;
                const { kind, message } = classifySearchError(err);
                if (kind === "cancelled") return;
                if (kind === "flag_off") void live.current.onFlagOff();
                const status = kind === "worktree" ? "worktree_unavailable" : kind === "flag_off" ? "disabled" : kind;
                setState((s)=>({
                        ...s,
                        status,
                        ahead: 0,
                        message: message !== null && message !== void 0 ? message : null
                    }));
                finish(kind === "invalid" ? "invalid_regex" : kind === "worktree" ? "worktree_unavailable" : kind === "busy" ? "busy" : kind === "flag_off" ? "flag_off" : "error", null);
            }
        })();
    }, [
        abort
    ]);
    const submit = (0,react.useCallback)((q)=>{
        if (q.pattern.length < MIN_QUERY_LENGTH) {
            abort();
            setState((s)=>({
                    ...s,
                    status: "idle",
                    ahead: 0,
                    stale: s.hits.length > 0
                }));
            return;
        }
        start(q);
    }, [
        abort,
        start
    ]);
    /** Typing: coalesces into one request 300 ms after the last change. */ const search = (0,react.useCallback)((q)=>{
        clearDebounce();
        debounce.current = setTimeout(()=>{
            debounce.current = null;
            submit(q);
        }, SEARCH_DEBOUNCE_MS);
    }, [
        clearDebounce,
        submit
    ]);
    /** Enter or a toggle: no debounce. */ const runNow = (0,react.useCallback)((q)=>{
        clearDebounce();
        submit(q);
    }, [
        clearDebounce,
        submit
    ]);
    const retry = (0,react.useCallback)(()=>{
        if (lastQuery.current) runNow(lastQuery.current);
    }, [
        runNow
    ]);
    const cancel = (0,react.useCallback)(()=>{
        clearDebounce();
        const { state: s, analytics: a } = live.current;
        if (s.status !== "queued" && s.status !== "searching") return;
        a.emit("files.search.cancel", {
            phase: s.status === "queued" ? "queued" : "running",
            results: (0,events/* bucketResults */.Du)(s.hits.length)
        });
        abort();
        // A queued search never produced anything of its own, so it returns to idle with the prior results.
        setState((cur)=>({
                ...cur,
                status: cur.status === "queued" ? "idle" : "cancelled",
                ahead: 0
            }));
    }, [
        abort,
        clearDebounce
    ]);
    const clear = (0,react.useCallback)(()=>{
        clearDebounce();
        abort();
        lastQuery.current = null;
        setState(INITIAL);
    }, [
        abort,
        clearDebounce
    ]);
    // Panel closed or unmounted: nothing keeps a connection open.
    (0,react.useEffect)(()=>{
        mounted.current = true;
        if (!enabled) {
            clearDebounce();
            abort();
            // A search cut off by closing the panel keeps its partial results and stops claiming to run.
            setState((cur)=>cur.status === "queued" || cur.status === "searching" ? {
                    ...cur,
                    status: "idle",
                    ahead: 0,
                    stale: cur.hits.length > 0
                } : cur);
        }
        return ()=>{
            mounted.current = false;
        };
    }, [
        enabled,
        abort,
        clearDebounce
    ]);
    (0,react.useEffect)(()=>()=>{
            clearDebounce();
            abort();
        }, [
        abort,
        clearDebounce
    ]);
    // A hidden document releases its connection after 30 s; the user re-runs on return.
    (0,react.useEffect)(()=>{
        const onVisibility = ()=>{
            if (hiddenTimer.current !== null) clearTimeout(hiddenTimer.current);
            hiddenTimer.current = null;
            if (document.visibilityState !== "hidden") return;
            hiddenTimer.current = setTimeout(()=>{
                hiddenTimer.current = null;
                const st = live.current.state.status;
                if (st !== "queued" && st !== "searching") return;
                abort();
                setState((cur)=>({
                        ...cur,
                        status: "cancelled",
                        ahead: 0
                    }));
            }, HIDDEN_ABORT_MS);
        };
        document.addEventListener("visibilitychange", onVisibility);
        return ()=>{
            document.removeEventListener("visibilitychange", onVisibility);
            if (hiddenTimer.current !== null) clearTimeout(hiddenTimer.current);
        };
    }, [
        abort
    ]);
    return {
        ...state,
        search,
        runNow,
        retry,
        cancel,
        clear,
        lastQuery: lastQuery.current
    };
}

;// ./src/components/sessions/files/searchForm.ts
const EMPTY = {
    pattern: "",
    regex: false,
    caseSensitive: false,
    wholeWord: false,
    include: "",
    exclude: "",
    includeIgnored: false
};
const globs = (raw)=>raw.split(",").map((g)=>g.trim()).filter(Boolean);
/** `largeFiles` is a per-device preference, so it rides beside the form and survives "Clear filters". */ const toQuery = (f, largeFiles)=>({
        pattern: f.pattern,
        regex: f.regex,
        caseSensitive: f.caseSensitive,
        wholeWord: f.wholeWord,
        include: globs(f.include),
        exclude: globs(f.exclude),
        includeIgnored: f.includeIgnored,
        largeFiles
    });
/** "(case-sensitive, regex)" style list of the toggles that are on. */ function activeToggles(q) {
    const on = [
        q.caseSensitive && "case-sensitive",
        q.wholeWord && "whole word",
        q.regex && "regex",
        q.includeIgnored && "ignored files included",
        q.largeFiles && "large files included"
    ].filter(Boolean);
    return on.length ? " (".concat(on.join(", "), ")") : "";
}

;// ./src/components/sessions/files/SearchPanel.tsx
/* __next_internal_client_entry_do_not_use__ SearchPaneTabs,SearchIdleState,SearchPanel,hitKey auto */ 












/** The Explorer | Search header above the left pane. */ function SearchPaneTabs(param) {
    let { pane, onPane } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: SearchPanel_css_header,
        role: "tablist",
        "aria-label": "Left panel",
        children: [
            "explorer",
            "search"
        ].map((id)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                role: "tab",
                id: "files-left-".concat(id),
                "aria-selected": pane === id,
                "data-testid": "files-left-".concat(id),
                className: headerTab,
                onClick: ()=>onPane(id),
                children: id === "explorer" ? "Explorer" : "Search"
            }, id))
    });
}
/** Never a blank pane: what to type and what the toggles mean (ux.md 3.4). */ function SearchIdleState(param) {
    let { sessionName } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: helper,
        "data-testid": "search-idle",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                children: [
                    "Search file contents in ",
                    sessionName !== null && sessionName !== void 0 ? sessionName : "this session"
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                children: "Type at least 2 characters"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                children: "Aa matches case, ab whole words, .* regular expressions. Ignored and binary files are skipped."
            })
        ]
    });
}
function resultsText(count, files) {
    return "".concat(count, " ").concat(count === 1 ? "result" : "results", " in ").concat(files, " ").concat(files === 1 ? "file" : "files");
}
/** Query, toggles, filters and the grouped results of the content search. */ const SearchPanel = /*#__PURE__*/ (0,react.forwardRef)(function SearchPanel(p, ref) {
    var _search_stats, _search_answered;
    const analytics = (0,filesAnalytics/* useFilesAnalytics */.R)({
        component: "SearchPanel",
        sessionId: p.sessionId
    });
    const search = useContentSearch({
        sessionId: p.sessionId,
        enabled: p.visible,
        stream: p.stream
    });
    const coarse = (0,useCoarsePointer/* useCoarsePointer */.O)();
    const largeFiles = useLargeFiles();
    const treeId = (0,react.useId)();
    const [form, setForm] = (0,react.useState)(EMPTY);
    const [showFilters, setShowFilters] = (0,react.useState)(false);
    const [collapsed, setCollapsed] = (0,react.useState)(new Set());
    const [activeKey, setActiveKey] = (0,react.useState)(null);
    const [includeFocusRequest, setIncludeFocusRequest] = (0,react.useState)(0);
    const [cursor, setCursor] = (0,react.useState)(null);
    const [wrapNote, setWrapNote] = (0,react.useState)(null);
    const inputRef = (0,react.useRef)(null);
    const includeRef = (0,react.useRef)(null);
    const resultsRef = (0,react.useRef)(null);
    const meter = (0,react.useRef)(new FindFlowMeter());
    const dirtyEdit = (0,react.useRef)(false);
    const rows = (0,react.useMemo)(()=>buildRows(search.hits, collapsed), [
        search.hits,
        collapsed
    ]);
    const files = (0,react.useMemo)(()=>new Set(search.hits.map((h)=>h.path)).size, [
        search.hits
    ]);
    const slow = (0,PendingState/* usePendingDelay */.oQ)(search.status === "searching", 1000);
    // Opening the panel starts a find flow; closing it discards the flow without an event.
    (0,react.useEffect)(()=>{
        if (p.visible) meter.current.open();
        else meter.current.cancel();
    }, [
        p.visible
    ]);
    (0,react.useEffect)(()=>{
        var _inputRef_current;
        if (p.focusToken > 0) (_inputRef_current = inputRef.current) === null || _inputRef_current === void 0 ? void 0 : _inputRef_current.focus();
    }, [
        p.focusToken
    ]);
    // Streamed batches grow `hits`; only a new query (or cleared results) should drop the selection.
    (0,react.useEffect)(()=>{
        setActiveKey(null);
    }, [
        search.answered
    ]);
    (0,react.useEffect)(()=>{
        var _includeRef_current;
        if (includeFocusRequest > 0) (_includeRef_current = includeRef.current) === null || _includeRef_current === void 0 ? void 0 : _includeRef_current.focus();
    }, [
        includeFocusRequest
    ]);
    (0,react.useEffect)(()=>{
        var _resultsRef_current;
        if (activeKey) (_resultsRef_current = resultsRef.current) === null || _resultsRef_current === void 0 ? void 0 : _resultsRef_current.scrollToKey(activeKey);
    }, [
        activeKey
    ]);
    const change = (0,react.useCallback)(function(next, immediate) {
        let large = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : largeFiles;
        setForm(next);
        dirtyEdit.current = true;
        const q = toQuery(next, large);
        if (immediate) search.runNow(q);
        else search.search(q);
    }, [
        search,
        largeFiles
    ]);
    const setLargeFiles = (on)=>{
        writeLargeFiles(on);
        change(form, true, on);
    };
    // One interaction per edit that leads to a request (typing a query is one, not one per key).
    const requested = search.lastQuery;
    (0,react.useEffect)(()=>{
        if (requested && dirtyEdit.current) {
            meter.current.interact();
            dirtyEdit.current = false;
        }
    }, [
        requested
    ]);
    // Callbacks below stay stable across renders (the quickfix effect depends on `step`), so props go through a ref.
    const props = (0,react.useRef)(p);
    props.current = p;
    const activate = (0,react.useCallback)((hit, hitIndex, via)=>{
        const reused = props.current.openHit(hit, via);
        setCursor(hitIndex);
        setWrapNote(null);
        const interactions = meter.current.finish();
        if (interactions !== null) {
            analytics.emit("files.search.open_hit", {
                rank: (0,events/* bucketRank */.Zc)(hitIndex + 1),
                interactions: (0,events/* bucketInteractions */.yL)(interactions),
                via,
                reused_tab: String(reused)
            });
        }
    }, [
        analytics
    ]);
    const hitsRef = (0,react.useRef)(search.hits);
    hitsRef.current = search.hits;
    const cursorRef = (0,react.useRef)(cursor);
    cursorRef.current = cursor;
    const step = (0,react.useCallback)((delta)=>{
        const n = hitsRef.current.length;
        if (n === 0) return;
        const at = cursorRef.current;
        const next = at === null ? delta > 0 ? 0 : n - 1 : (at + delta + n) % n;
        const wrapped = at !== null && (delta > 0 ? next < at : next > at);
        activate(hitsRef.current[next], next, "quickfix");
        if (wrapped) setWrapNote(delta > 0 ? "Wrapped to first hit" : "Wrapped to last hit");
    }, [
        activate
    ]);
    (0,react.useImperativeHandle)(ref, ()=>({
            focusQuery: ()=>{
                var _inputRef_current;
                return (_inputRef_current = inputRef.current) === null || _inputRef_current === void 0 ? void 0 : _inputRef_current.focus();
            },
            step
        }), [
        step
    ]);
    // A new search or cleared results end the walk through the old hits.
    const answered = search.answered;
    (0,react.useEffect)(()=>{
        setCursor(null);
        setWrapNote(null);
    }, [
        answered
    ]);
    const { onQuickfix } = p;
    const total = search.hits.length;
    (0,react.useEffect)(()=>{
        onQuickfix === null || onQuickfix === void 0 ? void 0 : onQuickfix(cursor === null || total === 0 ? null : {
            position: cursor + 1,
            total,
            note: wrapNote,
            step,
            dismiss: ()=>{
                setCursor(null);
                setWrapNote(null);
            }
        });
    }, [
        cursor,
        total,
        wrapNote,
        step,
        onQuickfix
    ]);
    const toggleGroup = (0,react.useCallback)((path)=>{
        setCollapsed((cur)=>{
            const next = new Set(cur);
            if (!next.delete(path)) next.add(path);
            return next;
        });
    }, []);
    const onKeyDown = (e)=>{
        if (e.key === "Escape") {
            e.preventDefault();
            p.onEscape();
            return;
        }
        if (e.target !== inputRef.current) return;
        const keys = rows.map((r)=>r.key);
        const at = activeKey ? keys.indexOf(activeKey) : -1;
        const move = (to)=>{
            e.preventDefault();
            var _keys_Math_max;
            setActiveKey((_keys_Math_max = keys[Math.max(0, Math.min(keys.length - 1, to))]) !== null && _keys_Math_max !== void 0 ? _keys_Math_max : null);
        };
        if (e.key === "ArrowDown") move(at + 1);
        else if (e.key === "ArrowUp") move(at - 1);
        else if (e.key === "Home" && e.ctrlKey) move(0);
        else if (e.key === "End" && e.ctrlKey) move(keys.length - 1);
        else if (e.key === "Enter") {
            e.preventDefault();
            const row = rows.find((r)=>r.key === activeKey);
            if ((row === null || row === void 0 ? void 0 : row.kind) === "group") toggleGroup(row.path);
            else if ((row === null || row === void 0 ? void 0 : row.kind) === "hit") activate(row.hit, row.hitIndex, "enter");
            else if (search.hits.length > 0 && !activeKey) activate(search.hits[0], 0, "enter");
            else search.runNow(toQuery(form, largeFiles));
        }
    };
    var _search_answered1;
    const q = (_search_answered1 = search.answered) !== null && _search_answered1 !== void 0 ? _search_answered1 : toQuery(form, largeFiles);
    const typed = form.pattern.length;
    const st = search.status;
    const hasResults = search.hits.length > 0;
    const worktreeGone = st === "worktree_unavailable";
    const disabled = st === "disabled";
    const busyStream = st === "searching" || st === "queued";
    let line = null;
    if (st === "queued") line = "Waiting for other searches (".concat(search.ahead, " ahead)...");
    else if (st === "searching") line = slow ? "Searching... ".concat(search.hits.length, " results so far") : "Searching...";
    else if (st === "cancelled") line = "Search cancelled - ".concat(search.hits.length, " results so far");
    else if (st === "done" && hasResults) line = resultsText(search.hits.length, files);
    const idleCopy = typed < 2 && !busyStream && !worktreeGone && !disabled;
    const retained = st === "idle" && hasResults;
    const zero = st === "done" && !hasResults;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: SearchPanel_css_panel,
        "data-testid": "search-panel",
        onKeyDown: onKeyDown,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: SearchPanel_css_form,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: queryRow,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                            ref: inputRef,
                            type: "search",
                            className: queryInput,
                            "aria-label": "Search file contents",
                            "aria-controls": treeId,
                            "aria-activedescendant": activeKey ? rowDomId(treeId, activeKey) : undefined,
                            inputMode: "search",
                            autoComplete: "off",
                            spellCheck: false,
                            placeholder: "Search",
                            value: form.pattern,
                            "data-testid": "search-query",
                            onChange: (e)=>change({
                                    ...form,
                                    pattern: e.target.value
                                }, false)
                        })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: toggleRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: toggle,
                                "aria-pressed": form.caseSensitive,
                                "data-testid": "search-toggle-case",
                                "aria-label": "Match case",
                                title: "Match case",
                                onClick: ()=>change({
                                        ...form,
                                        caseSensitive: !form.caseSensitive
                                    }, true),
                                children: "Aa"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: toggle,
                                "aria-pressed": form.wholeWord,
                                "data-testid": "search-toggle-word",
                                "aria-label": "Match whole word",
                                title: "Match whole word",
                                onClick: ()=>change({
                                        ...form,
                                        wholeWord: !form.wholeWord
                                    }, true),
                                children: "ab"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: toggle,
                                "aria-pressed": form.regex,
                                "data-testid": "search-toggle-regex",
                                "aria-label": "Use regular expression",
                                title: "Use regular expression",
                                onClick: ()=>change({
                                        ...form,
                                        regex: !form.regex
                                    }, true),
                                children: ".*"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: SearchPanel_css_textButton,
                                "aria-expanded": showFilters,
                                onClick: ()=>setShowFilters((v)=>!v),
                                children: "Filters"
                            })
                        ]
                    }),
                    showFilters && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: filters,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: fieldLabel,
                                htmlFor: "".concat(treeId, "-inc"),
                                children: "Files to include"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "".concat(treeId, "-inc"),
                                ref: includeRef,
                                className: queryInput,
                                value: form.include,
                                placeholder: "**/*.go",
                                onChange: (e)=>change({
                                        ...form,
                                        include: e.target.value
                                    }, false)
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: fieldLabel,
                                htmlFor: "".concat(treeId, "-exc"),
                                children: "Files to exclude"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "".concat(treeId, "-exc"),
                                className: queryInput,
                                value: form.exclude,
                                placeholder: "vendor/**",
                                onChange: (e)=>change({
                                        ...form,
                                        exclude: e.target.value
                                    }, false)
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: checkLabel,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        type: "checkbox",
                                        checked: form.includeIgnored,
                                        onChange: (e)=>change({
                                                ...form,
                                                includeIgnored: e.target.checked
                                            }, true)
                                    }),
                                    "Include ignored files"
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: checkLabel,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        type: "checkbox",
                                        checked: largeFiles,
                                        onChange: (e)=>setLargeFiles(e.target.checked)
                                    }),
                                    "Include large files"
                                ]
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: SearchPanel_css_status,
                role: "status",
                "aria-live": "polite",
                "data-testid": "search-status",
                children: [
                    line,
                    busyStream && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: SearchPanel_css_textButton,
                        "data-testid": "search-cancel",
                        onClick: search.cancel,
                        children: "Cancel"
                    })
                ]
            }),
            st === "invalid" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: inlineError,
                children: [
                    "Invalid regular expression: ",
                    search.message
                ]
            }),
            st === "error" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: emptyMessage,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        children: "Search failed - check your connection"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: SearchPanel_css_textButton,
                        onClick: search.retry,
                        children: "Retry"
                    })
                ]
            }),
            st === "busy" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: emptyMessage,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        children: "Too many searches are running - try again in a moment"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: SearchPanel_css_textButton,
                        onClick: search.retry,
                        children: "Retry"
                    })
                ]
            }),
            disabled && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: emptyMessage,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    children: "Search is disabled"
                })
            }),
            worktreeGone && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: emptyMessage,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        children: "Session paused; worktree unavailable"
                    }),
                    p.onResumeSession && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: SearchPanel_css_textButton,
                        onClick: p.onResumeSession,
                        children: "Resume session"
                    })
                ]
            }),
            zero && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: emptyMessage,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        children: "No matches for `".concat(q.pattern, "`").concat(activeToggles(q))
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: SearchPanel_css_textButton,
                        onClick: ()=>change({
                                ...EMPTY,
                                pattern: form.pattern
                            }, true),
                        children: "Clear filters"
                    })
                ]
            }),
            st === "done" && ((_search_stats = search.stats) === null || _search_stats === void 0 ? void 0 : _search_stats.truncated) && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: note,
                children: [
                    "Showing first 1000 - refine your query",
                    " ",
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: SearchPanel_css_textButton,
                        onClick: ()=>{
                            setShowFilters(true);
                            setIncludeFocusRequest((n)=>n + 1);
                        },
                        children: "Add filters"
                    })
                ]
            }),
            idleCopy && !hasResults && /*#__PURE__*/ (0,jsx_runtime.jsx)(SearchIdleState, {
                sessionName: p.sessionName
            }),
            retained && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: note,
                "data-testid": "search-retained",
                children: [
                    "Results for `",
                    (_search_answered = search.answered) === null || _search_answered === void 0 ? void 0 : _search_answered.pattern,
                    "` - edit to search again"
                ]
            }),
            hasResults && !worktreeGone && /*#__PURE__*/ (0,jsx_runtime.jsx)(SearchResults, {
                ref: resultsRef,
                rows: rows,
                stale: search.stale || retained,
                busy: busyStream,
                activeKey: activeKey,
                dirtyPaths: p.dirtyPaths,
                coarse: coarse,
                treeId: treeId,
                onToggleGroup: toggleGroup,
                onActivate: activate
            }),
            st === "done" && search.stats && search.stats.largeFilesSkipped > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: note,
                children: [
                    search.stats.largeFilesSkipped,
                    " large ",
                    search.stats.largeFilesSkipped === 1 ? "file was" : "files were",
                    " too big or too slow to search"
                ]
            }),
            st === "done" && search.stats && search.stats.binarySkipped > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: note,
                children: [
                    "Skipped ",
                    search.stats.binarySkipped,
                    " binary files"
                ]
            })
        ]
    });
});


;// ./src/components/sessions/files/FilesLeftPane.tsx
/* __next_internal_client_entry_do_not_use__ FilesLeftPane auto */ 








/** The Explorer (toolbar, recent files, tree) and, with files:viewer-v2, the Search panel; both stay mounted. */ function FilesLeftPane(p) {
    const { v2, search } = p;
    const [includeIgnored, setIncludeIgnored] = (0,react.useState)(false);
    const [sortBy, setSortBy] = (0,react.useState)("name");
    const [filterChangedOnly, setFilterChangedOnly] = (0,react.useState)(false);
    const [searchTerm, setSearchTerm] = (0,react.useState)("");
    const [resultCount, setResultCount] = (0,react.useState)(null);
    const [resultTruncated, setResultTruncated] = (0,react.useState)(false);
    // VCS status comes from shared context, with no independent fetch.
    const { status, statusLoading, refreshStatus } = (0,SessionVcsContext/* useSessionVcsContext */.l)();
    const changedFiles = (0,react.useMemo)(()=>{
        if (!status) return [];
        return [
            ...status.stagedFiles,
            ...status.unstagedFiles,
            ...status.untrackedFiles
        ];
    }, [
        status
    ]);
    const gitStatusMap = (0,react.useMemo)(()=>(0,gitStatus/* buildGitStatusMap */.lT)(changedFiles), [
        changedFiles
    ]);
    const lineStatsMap = (0,react.useMemo)(()=>(0,gitStatus/* buildLineStatsMap */.Ec)(changedFiles), [
        changedFiles
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            v2 && !p.mobile && /*#__PURE__*/ (0,jsx_runtime.jsx)(SearchPaneTabs, {
                pane: search.leftPane,
                onPane: search.chooseLeftPane
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: leftView,
                hidden: search.leftPane === "search",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(FilesTreeToolbar, {
                        searchInputRef: p.searchInputRef,
                        fileTreeRef: p.fileTreeRef,
                        searchTerm: searchTerm,
                        onSearchTerm: setSearchTerm,
                        searchResultCount: resultCount,
                        searchResultTruncated: resultTruncated,
                        includeIgnored: includeIgnored,
                        onIncludeIgnored: setIncludeIgnored,
                        filterChangedOnly: filterChangedOnly,
                        onFilterChangedOnly: setFilterChangedOnly,
                        sortBy: sortBy,
                        onToggleSort: ()=>setSortBy((prev)=>prev === "name" ? "type" : "name"),
                        collapsed: p.panel.collapsed,
                        onCollapse: p.panel.collapse,
                        onExpand: p.panel.expand,
                        vcsLoading: statusLoading,
                        onRefreshStatus: refreshStatus
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(RecentFilesSection, {
                        paths: p.recentPaths,
                        selectedPath: p.selectedPath,
                        onSelect: (path)=>p.onSelect(path, "recent")
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: treeWrapper,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(FileTree/* FileTree */.hr, {
                            ref: p.fileTreeRef,
                            sessionId: p.sessionId,
                            baseUrl: p.baseUrl,
                            onFileSelect: p.onSelect,
                            gitStatusMap: gitStatusMap,
                            lineStatsMap: lineStatsMap,
                            sortBy: sortBy,
                            filterChangedOnly: filterChangedOnly,
                            selectedPath: p.selectedPath,
                            includeIgnored: includeIgnored,
                            searchTerm: searchTerm,
                            onSearchResults: (count, truncated)=>{
                                setResultCount(count);
                                setResultTruncated(truncated);
                            }
                        })
                    })
                ]
            }),
            v2 && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: leftView,
                hidden: search.leftPane !== "search",
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(SearchPanel, {
                    ref: p.searchRef,
                    onQuickfix: p.onQuickfix,
                    sessionId: p.sessionId,
                    sessionName: p.sessionName,
                    visible: search.leftPane === "search",
                    focusToken: search.focusToken,
                    dirtyPaths: search.dirtyPaths,
                    openHit: search.openHit,
                    onEscape: v2.bridge.focusEditor,
                    onResumeSession: p.onResumeSession ? ()=>{
                        var _p_onResumeSession;
                        return (_p_onResumeSession = p.onResumeSession) === null || _p_onResumeSession === void 0 ? void 0 : _p_onResumeSession.call(p, document.activeElement);
                    } : undefined
                })
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/files/fontSize.ts
var fontSize = __webpack_require__(19216);
// EXTERNAL MODULE: ./src/components/providers/ViewportProvider.tsx
var ViewportProvider = __webpack_require__(55765);
// EXTERNAL MODULE: ./src/lib/hooks/usePublishStackTopOffset.ts
var usePublishStackTopOffset = __webpack_require__(23275);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2Ffiles%2FMobileFilesLayout.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA81Wy27bMBC85yt4KRIXZUA5seMwSH8kKApKXNlMaVIgV7bjov9ekJJlSZaUoD200ElcPmZndoe8NavXFHNGfl4RIpUvtHjjJNdweLoiRGi1NlQhbD0nGRgEF4ZDmPqNU-YHJyyMpCL7sXa2NJKTnXA3lH5PHuE4d8eHWYhnVlvXD7EY2ipDN6DWG-Tk_r6I56bWSXA0tYh2y0lSHIi3WsneBhA3KKxXqKzhxIEWqHbwdPXr6rZKLPlPElt1EkNbvJuVkFKZdcMBmN2NFzlQ4UBQZTxgHftCWHF4n4l5ZCKA2iuJmzPZ4wJwYqyBPg3ohPGFcGCwxYAyG3AqjuTW4MUA9eoInCR31QFZ6XxYVlh1Ir8RSRmtDNCTVlEUqRxkVW6Z1eXWTKj4WnpU-RvNrEEIUM6htaiIbzHMCSMx6zNTPLdZ6elOeZVqiLzZEgMoTuYjqumoQD2N2jz3gJzQeXfnF-GUoFJ5kWqQz9foSrj-Vp1QiEzhGyfs9r7NkIRclBovd8lKFzTobJLaUL9C2n2g0QMSFhGzHtrtrLXfXVwaaOYkeeoWCWvNu-_Pu2irkYWLamGrCpIOLctpAB_v3yjvcljeLoBFa2hfF_-StUE_VKrswOU60LlRUkIsO4QD0nMAtFaFVz6E9huFQH0hMgjNs3eiqPA31etQN3vELDjRkOMEe6sIpA7UPnLq166tOCFV6TlZsE_T3nWcDbldc-JjPPHsJiL1VpcYneBIlZFBqAVjPdBzxio4F3KNt3Cj0sl4RjE_ztrGNGaer7MBPrpT7h7qOedWiSVCkmVxIG6dihv2JX6380W7Tx5fpEBBc2e3z9doi7rnKjNnFeeu9tFOfXcWVqbd9Gt9w00tF41z9216qIpadZ_M_9TNh-6zC1sf8PBp4pdtNgXfhB6qaBhVfdVZ8kH7XEz7ZzpR3q7XV5U89W9T-8mwKw2XOXWwA-fhwrfidDCyca76mJpPCjsw6E-yteB_JZ_rFLoTRYm2NTHr5Xl-FAy9dE611TYCOcFUqLcG8l_RNmXmq56ZhxZN2KWf371nH2_jz7Xjh5zl8EFnaciDf_fgir4gIbNOVMKVRoILL5PBvm0g5xN610kk3Quo_h26JjOtilB0Gd4wEr_Z6BX56zdukSc9iAwAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2Ffiles_2FMobileFilesLayout_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81Wy27bMBC85yt4KRIXZUA5seMwSH8kKApKXNlMaVIgV7bjov9ekJJlSZaUoD200ElcPmZndoe8NavXFHNGfl4RIpUvtHjjJNdweLoiRGi1NlQhbD0nGRgEF4ZDmPqNU_YHJyyMpCL7sXa2NJKTnXA3lH5PHuE4d8eHWYhnVlvXD7EY2ipDN6DWG_Tk_r6I56bWSXA0tYh2y0lSHIi3WsneBhA3KKxXqKzhxIEWqHbwdPXr6rZKLPlPElt1EkNbvJuVkFKZdcMBmN2NFzlQ4UBQZTxgHftCWHF4n4l5ZCKA2iuJmzPZ4wJwYqyBPg3ohPGFcGCwxYAyG3AqjuTW4MUA9eoInCR31QFZ6XxYVlh1Ir8RSRmtDNCTVlEUqRxkVW6Z1eXWTKj4WnpU_RvNrEEIUM6htaiIbzHMCSMx6zNTPLdZ6elOeZVqiLzZEgMoTuYjqumoQD2N2jz3gJzQeXfnF_GUoFJ5kWqQz9foSrj_Vp1QiEzhGyfs9r7NkIRclBovd8lKFzTobJLaUL9C2n2g0QMSFhGzHtrtrLXfXVwaaOYkeeoWCWvNu_Pu2irkYWLamGrCpIOLctpAB_v3yjvcljeLoBFa2hfF_StUE_VKrswOU60LlRUkIsO4QD0nMAtFaFVz6E9huFQH0hMgjNs3eiqPA31etQN3vELDjRkOMEe6sIpA7UPnLq166tOCFV6TlZsE_T3nWcDbldc_JjPPHsJiL1VpcYneBIlZFBqAVjPdBzxio4F3KNt3Cj0sl4RjE_ztrGNGaer7MBPrpT7h7qOedWiSVCkmVxIG6dihv2JX6380W7Tx5fpEBBc2e3z9doi7rnKjNnFeeu9tFOfXcWVqbd9Gt9w00tF41z9216qIpadZ_M_9TNh_6zC1sf8PBp4pdtNgXfhB6qaBhVfdVZ8kH7XEz7ZzpR3q7XV5U89W9T_8mwKw2XOXWwA_fhwrfidDCyca76mJpPCjsw6E_yteB_JZ_rFLoTRYm2NTHr5Xl_FAy9dE611TYCOcFUqLcG8l_RNmXmq56ZhxZN2KWf371nH2_jz7Xjh5zl8EFnaciDf_fgir4gIbNOVMKVRoILL5PBvm0g5xN610kk3Quo_h26JjOtilB0Gd4wEr_Z6BX56zdukSc9iAwAAA_22_7D = __webpack_require__(82062);
;// ./src/components/sessions/files/MobileFilesLayout.css.ts


var barButton = 'n8jbtf2';
var barLabel = 'n8jbtf5';
var bottomBar = 'n8jbtf1';
var bottomItem = 'n8jbtf3';
var MobileFilesLayout_css_dirtyDot = 'n8jbtf8';
var fileName = 'n8jbtf7';
var floatingStack = 'n8jbtfb';
var MobileFilesLayout_css_popover = 'n8jbtf9';
var popoverItem = 'n8jbtfa';
var split = 'n8jbtf4';
var MobileFilesLayout_css_srOnly = 'n8jbtff';
var stackHost = 'n8jbtfc';
var title = 'n8jbtf6';
var MobileFilesLayout_css_toast = 'n8jbtfd';
var MobileFilesLayout_css_toastButton = 'n8jbtfe';
var topBar = 'n8jbtf0';
;// ./src/components/sessions/files/MobileBars.tsx
/* __next_internal_client_entry_do_not_use__ NARROW_BAR_PX,MobileTopBar,MobileBottomBar auto */ 



const NARROW_BAR_PX = 360;
/** A button that opens a small menu; Esc and outside taps close it and return focus to the button. */ function MenuButton(param) {
    let { label, glyph, items, from } = param;
    const [open, setOpen] = (0,react.useState)(false);
    const button = (0,react.useRef)(null);
    const menu = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        var _menu_current_querySelector, _menu_current;
        if (!open) return;
        (_menu_current = menu.current) === null || _menu_current === void 0 ? void 0 : (_menu_current_querySelector = _menu_current.querySelector("[role=menuitem]:not([aria-disabled=true])")) === null || _menu_current_querySelector === void 0 ? void 0 : _menu_current_querySelector.focus();
        const onDown = (e)=>{
            var _menu_current;
            if (!((_menu_current = menu.current) === null || _menu_current === void 0 ? void 0 : _menu_current.contains(e.target)) && e.target !== button.current) setOpen(false);
        };
        document.addEventListener("pointerdown", onDown);
        return ()=>document.removeEventListener("pointerdown", onDown);
    }, [
        open
    ]);
    const close = ()=>{
        var _button_current;
        setOpen(false);
        (_button_current = button.current) === null || _button_current === void 0 ? void 0 : _button_current.focus();
    };
    const onKey = (e)=>{
        if (e.key === "Escape") {
            e.stopPropagation();
            close();
        }
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                ref: button,
                type: "button",
                className: barButton,
                "aria-label": label,
                "aria-haspopup": "menu",
                "aria-expanded": open,
                onClick: ()=>setOpen((o)=>!o),
                children: glyph
            }),
            open && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                ref: menu,
                className: MobileFilesLayout_css_popover,
                "data-from": from,
                role: "menu",
                "aria-label": label,
                onKeyDown: onKey,
                children: items.map((it)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        role: "menuitem",
                        className: popoverItem,
                        "aria-disabled": it.disabled || undefined,
                        onClick: ()=>{
                            if (it.disabled) return;
                            setOpen(false);
                            it.run();
                        },
                        children: it.label
                    }, it.label))
            })
        ]
    });
}
function MobileTopBar(param) {
    let { pane, origin, path, dirty, tabCount, overflow, onBack, onTabs } = param;
    const heading = pane === "files" ? "Files" : pane === "search" ? "Search" : path !== null && path !== void 0 ? path : "No file open";
    const backLabel = pane === "search" ? "Back to Files" : origin === "search" ? "Back to results" : "Back to Files";
    // The phone toast deck docks under the session tab row by default, which is where this bar sits; dock it below the bar instead.
    const barRef = (0,react.useRef)(null);
    (0,usePublishStackTopOffset/* usePublishDeckTopOffset */.K)(barRef);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        ref: barRef,
        className: topBar,
        "data-testid": "mobile-top-bar",
        children: [
            pane !== "files" && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                className: barButton,
                "aria-label": backLabel,
                onClick: ()=>onBack(),
                children: "←"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: title,
                style: pane === "files" ? {
                    paddingLeft: 12
                } : undefined,
                children: [
                    pane === "editor" && dirty && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: MobileFilesLayout_css_dirtyDot,
                        "aria-hidden": "true"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: fileName,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("bdi", {
                            children: heading
                        })
                    }),
                    pane === "editor" && dirty && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: MobileFilesLayout_css_srOnly,
                        children: "unsaved"
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                type: "button",
                className: barButton,
                "aria-label": "Open files, ".concat(tabCount),
                "aria-haspopup": "dialog",
                onClick: onTabs,
                children: [
                    "[",
                    tabCount,
                    "]"
                ]
            }),
            pane === "editor" && overflow.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)(MenuButton, {
                label: "More actions",
                glyph: "⋯",
                items: overflow,
                from: "top"
            })
        ]
    });
}
function MobileBottomBar(p) {
    const narrow = p.width < NARROW_BAR_PX;
    const step = (label, glyph, enabled, run, why)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
            type: "button",
            className: barButton,
            style: {
                flex: 1
            },
            "aria-label": label,
            "aria-disabled": !enabled,
            title: enabled ? label : why,
            onClick: ()=>{
                if (enabled) run();
            },
            children: glyph
        });
    const item = function(label, glyph, run) {
        let extra = arguments.length > 3 && arguments[3] !== void 0 ? arguments[3] : {};
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
            type: "button",
            className: "".concat(barButton, " ").concat(bottomItem),
            "aria-label": label,
            "aria-current": extra.current ? "true" : undefined,
            "aria-disabled": extra.disabled || undefined,
            onClick: ()=>{
                if (!extra.disabled) run();
            },
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    "aria-hidden": "true",
                    children: glyph
                }),
                !narrow && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    className: barLabel,
                    "aria-hidden": "true",
                    children: label
                })
            ]
        });
    };
    const back = step("Previous location", "◀", p.canBack, p.onBack, "No earlier location");
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("nav", {
        className: bottomBar,
        "aria-label": "Files actions",
        "data-testid": "mobile-bottom-bar",
        children: [
            item("Files", "▤", p.onFiles, {
                current: p.pane === "files"
            }),
            item("Search", "⌕", p.onSearch, {
                current: p.pane === "search"
            }),
            item("Quick open", "⌘", p.onQuickOpen),
            narrow ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: split,
                        children: back
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(MenuButton, {
                        label: "More",
                        glyph: "⋯",
                        from: "bottom",
                        items: [
                            {
                                label: "Go to line",
                                run: p.onGoToLine,
                                disabled: !p.hasFile
                            },
                            {
                                label: "Next location",
                                run: p.onForward,
                                disabled: !p.canForward
                            }
                        ]
                    })
                ]
            }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    item("Go to line", "#", p.onGoToLine, {
                        disabled: !p.hasFile
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: split,
                        style: {
                            flex: 2
                        },
                        role: "group",
                        "aria-label": "Location history",
                        children: [
                            back,
                            step("Next location", "▶", p.canForward, p.onForward, "No later location")
                        ]
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/hooks/useTrayDismissal.ts + 1 modules
var useTrayDismissal = __webpack_require__(61423);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2Ffiles%2FTabSwitcherSheet.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA71XXY_aOBR951f4pSpIdeUwDMN4VKl_Y1WtKhPfJO44dmQ7EFjNf1_ZTsBJgSnqbsVLMDe-55577gdfX-FQGFaDRd8zZuRWPhD0zwyhwug6PCDkDFO20Kam8VEyB3_NM0I-LF5mCL3NEHL6pi3pDd9mnwcvWXih0VY4oRVFheiAeyuhLDiKiH8-YqE4dBRl5CkcbFn-WhrdKk6RKbdsTj6Fz-fVY3DBhW0kO1BUSOj8AZOiVFg4qG08xKD4SwpkGYDsBXeV90M--Ndq1uEKRFk5ijaE76qLl4f7uDCQxxhyLdtaTXHumJlj_D17huPSHJ8Dzq02HAx2uqEoazpktRR8YvkjtTSMi9ZOL3t4Wlw4IZG8hnEuVIm32jldUwRqN7esAMwMMBxo7n_7hEjTBW9MiZrFYM5yyDaktgiYBaxbNyLvIZB3m_UclAPjj3-01onigHOtHChHkW1YDngLbg-gEswUEbRpOkRQtm66hIYhlmucwWIEbxXg1cyUQvWSKrRy2IojUJQ9xrvD0b7P9poEs1xLbaZ0k_Htj-F2KazD1h0kUKS0gpepx3NM_pvegSmk3uMDRax1enTjelIUBiRzYgfvS9s6Ay6v7qCqGAfzFFxf1-1mbL6JTUKG4gwhC4X7IiJ3FstPsjgrxkHncAiUIgmF82clayhaxtSduF03HcqWqVbO2UiDCk2pYQaUu5nmqAqKhKrAiGjbGuuNGy1O8HSbV5j1ETVM4cOIpee7yiMEtk40OZbpoByKKsF5rJfAz_kHkFI0Vlj_074SDnCoME_F3rBmhI3FDCZ-evoucrJc_FcQPB8nERgnL6c5Abq9C2g21mmedvdNfO_U2EetZeiwj3ECXK-E4-IkZ1sZoV6D4hOfPPW5WkUvvj4Gx8PZ7wo1W1zRZUpWEFSKjhY6by3eCSu2EgJW3TopFISyutgsZEx_NMO6KMKMxsvp3d84cwxXoqykD_XLR2da-Pj3rzmpLztZ3cj2pCPHaE5NISO-K1yoqIee_6s5PiyuujymY_m9JtuN8RVjfMtVgu-OhhkaxVAFP2841jFzWzQJpDKOyAvqTGbxeApfD7f8hZVlvfg_e3ISWTUZpmxrtWxdqDTTh9q3gLCH9WEnO-edaUlGYLYhExJX70kuXQx_dymMNr4_Me6bMkGD0sZb8_JxLAZxVQyXBnEqkD8-et8XWRoXrfyEum_FEd-4sGwrgff9q2G5cAeK_L-NFBWHgrUyDK2vNXDB0LwxUICx2ABvc-C41sNC578vwn2TfyCj5Xvgr_9DJcanb7O3fwHgUVjquA0AAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2Ffiles_2FTabSwitcherSheet_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA71XXY_aOBR951f4pSpIdeUwDMN4VKl_Y1WtKhPfJO44dmQ7EFjNf1_ZTsBJgSnqbsVLMDe_55577gdfX_FQGFaDRd8zZuRWPhD0zwyhwug6PCDkDFO20Kam8VEyB3_NM0I_LF5mCL3NEHL6pi3pDd9mnwcvWXih0VY4oRVFheiAeyuhLDiKiH8_YqE4dBRl5CkcbFn_WhrdKk6RKbdsTj6Fz_fVY3DBhW0kO1BUSOj8AZOiVFg4qG08xKD4SwpkGYDsBXeV90M_Ndq1uEKRFk5ijaE76qLl4f7uDCQxxhyLdtaTXHumJlj_D17huPSHJ8Dzq02HAx2uqEoazpktRR8YvkjtTSMi9ZOL3t4Wlw4IZG8hnEuVIm32jldUwRqN7esAMwMMBxo7n_7hEjTBW9MiZrFYM5yyDaktgiYBaxbNyLvIZB3m_UclAPjj3_01onigHOtHChHkW1YDngLbg_gEswUEbRpOkRQtm66hIYhlmucwWIEbxXg1cyUQvWSKrRy2IojUJQ9xrvD0b7P9poEs1xLbaZ0k_Htj_F2KazD1h0kUKS0gpepx3NM_pvegSmk3uMDRax1enTjelIUBiRzYgfvS9s6Ay6v7qCqGAfzFFxf1_1mbL6JTUKG4gwhC4X7IiJ3FstPsjgrxkHncAiUIgmF82clayhaxtSduF03HcqWqVbO2UiDCk2pYQaUu5nmqAqKhKrAiGjbGuuNGy1O8HSbV5j1ETVM4cOIpee7yiMEtk40OZbpoByKKsF5rJfAz_kHkFI0Vlj_074SDnCoME_F3rBmhI3FDCZ_evoucrJc_FcQPB8nERgnL6c5Abq9C2g21mmedvdNfO_U2EetZeiwj3ECXK_E4_IkZ1sZoV6D4hOfPPW5WkUvvj4Gx8PZ7wo1W1zRZUpWEFSKjhY6by3eCSu2EgJW3TopFISyutgsZEx_NMO6KMKMxsvp3d84cwxXoqykD_XLR2da_Pj3rzmpLztZ3cj2pCPHaE5NISO_K1yoqIee_6s5PiyuujymY_m9JtuN8RVjfMtVgu_OhhkaxVAFP2841jFzWzQJpDKOyAvqTGbxeApfD7f8hZVlvfg_e3ISWTUZpmxrtWxdqDTTh9q3gLCH9WEnO_edaUlGYLYhExJX70kuXQx_dymMNr4_Me6bMkGD0sZb8_JxLAZxVQyXBnEqkD8_et8XWRoXrfyEum_FEd_4sGwrgff9q2G5cAeK_L_NFBWHgrUyDK2vNXDB0LwxUICx2ABvc_C41sNC578vwn2TfyCj5Xvgr_9DJcanb7O3fwHgUVjquA0AAA_22_7D = __webpack_require__(33171);
;// ./src/components/sessions/files/TabSwitcherSheet.css.ts


var TabSwitcherSheet_css_backdrop = '_1arlbl31';
var dirtyMark = '_1arlbl3c';
var TabSwitcherSheet_css_empty = '_1arlbl3f';
var TabSwitcherSheet_css_heading = '_1arlbl34';
var iconButton = '_1arlbl3d';
var TabSwitcherSheet_css_list = '_1arlbl35';
var TabSwitcherSheet_css_menuItem = '_1arlbl3i';
var notice = '_1arlbl3e';
var TabSwitcherSheet_css_row = '_1arlbl36';
var rowActive = '_1arlbl37';
var rowMain = '_1arlbl38';
var rowMenu = '_1arlbl3h';
var rowPath = '_1arlbl3a';
var rowTitle = '_1arlbl39';
var sheet = '_1arlbl32';
var sheetHeader = '_1arlbl33';
var stateWord = '_1arlbl3b';
var TabSwitcherSheet_css_textButton = '_1arlbl3g';
;// ./src/components/sessions/files/TabSwitcherSheet.tsx
/* __next_internal_client_entry_do_not_use__ TARGET_PX,TabSwitcherSheet auto */ 





const TARGET_PX = 44;
const LONG_PRESS_MS = 500;
const stateOf = (b)=>tabLabels_isDirty(b) ? "unsaved" : b.pinned ? "pinned" : "preview";
/** Full-width bottom sheet listing the open buffers; it owns one history entry so system Back dismisses it. */ function TabSwitcherSheet(p) {
    const ref = (0,react.useRef)(null);
    const invoker = (0,react.useRef)(typeof document === "undefined" ? null : document.activeElement);
    const [menuFor, setMenuFor] = (0,react.useState)(null);
    const pressTimer = (0,react.useRef)(null);
    const longPressed = (0,react.useRef)(false);
    (0,useTrayDismissal/* useTrayDismissal */.l)({
        isOpen: true,
        isSheet: true,
        modal: false,
        trayRef: ref,
        close: p.onDismiss
    });
    (0,useFocusTrap/* useFocusTrap */.r)(ref, true, invoker);
    (0,react.useEffect)(()=>()=>{
            if (pressTimer.current) clearTimeout(pressTimer.current);
        }, []);
    const startPress = (path)=>{
        longPressed.current = false;
        pressTimer.current = setTimeout(()=>{
            longPressed.current = true;
            setMenuFor(path);
        }, LONG_PRESS_MS);
    };
    const endPress = ()=>{
        if (pressTimer.current) clearTimeout(pressTimer.current);
    };
    const onMenuKey = (e)=>{
        if (e.key !== "Escape" || !menuFor) return;
        e.stopPropagation();
        setMenuFor(null);
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: TabSwitcherSheet_css_backdrop,
        "data-testid": "tab-switcher-backdrop",
        onClick: (e)=>{
            if (e.target === e.currentTarget) p.onDismiss();
        },
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            ref: ref,
            className: sheet,
            role: "dialog",
            "aria-modal": "true",
            "aria-label": "Open files",
            "data-testid": "tab-switcher-sheet",
            onKeyDown: onMenuKey,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: sheetHeader,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("h2", {
                            className: TabSwitcherSheet_css_heading,
                            children: [
                                "Open files (",
                                p.buffers.length,
                                ")"
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            type: "button",
                            className: iconButton,
                            style: {
                                minWidth: TARGET_PX
                            },
                            "aria-label": "Close tab list",
                            onClick: p.onDismiss,
                            children: "\xd7"
                        })
                    ]
                }),
                p.refusal && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: notice,
                    role: "note",
                    children: p.refusal
                }),
                p.buffers.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: TabSwitcherSheet_css_empty,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            children: "No open files - open one from Files"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            type: "button",
                            className: TabSwitcherSheet_css_textButton,
                            onClick: p.onGoToFiles,
                            children: "Go to Files"
                        })
                    ]
                }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                    className: TabSwitcherSheet_css_list,
                    "aria-label": "Open files",
                    children: p.buffers.map((b, i)=>{
                        const label = labelOf(b.path, p.buffers);
                        const closable = !tabLabels_isDirty(b);
                        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                            className: "".concat(TabSwitcherSheet_css_row, " ").concat(i === p.activeIndex ? rowActive : ""),
                            "data-testid": "tab-sheet-row-".concat(b.path),
                            style: {
                                minHeight: TARGET_PX
                            },
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                    type: "button",
                                    className: rowMain,
                                    style: {
                                        minHeight: TARGET_PX
                                    },
                                    "aria-label": accessibleName(b, p.buffers),
                                    "aria-current": i === p.activeIndex ? "true" : undefined,
                                    onPointerDown: ()=>startPress(b.path),
                                    onPointerUp: endPress,
                                    onPointerLeave: endPress,
                                    onPointerCancel: endPress,
                                    onContextMenu: (e)=>{
                                        e.preventDefault();
                                        setMenuFor(b.path);
                                    },
                                    onClick: ()=>{
                                        if (longPressed.current) {
                                            longPressed.current = false;
                                            return;
                                        }
                                        p.onSelect(b.path);
                                    },
                                    onDoubleClick: ()=>p.onPin(b.path),
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                            className: rowTitle,
                                            "aria-hidden": "true",
                                            children: [
                                                tabLabels_isDirty(b) && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: dirtyMark
                                                }),
                                                b.pinned && !tabLabels_isDirty(b) && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    children: "\uD83D\uDCCC"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    children: label
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                    className: stateWord,
                                                    children: [
                                                        "(",
                                                        stateOf(b),
                                                        ")"
                                                    ]
                                                })
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: rowPath,
                                            "aria-hidden": "true",
                                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("bdi", {
                                                children: b.path
                                            })
                                        })
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "button",
                                    className: iconButton,
                                    style: {
                                        minHeight: TARGET_PX
                                    },
                                    "aria-label": "Actions for ".concat(label),
                                    "aria-haspopup": "menu",
                                    "aria-expanded": menuFor === b.path,
                                    onClick: ()=>setMenuFor(menuFor === b.path ? null : b.path),
                                    children: "⋯"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "button",
                                    className: iconButton,
                                    style: {
                                        minHeight: TARGET_PX
                                    },
                                    "data-highlight": p.refusal && closable ? "true" : undefined,
                                    "aria-label": p.refusal && closable ? "Close ".concat(label, ", frees a slot") : "Close ".concat(label),
                                    onClick: ()=>p.onClose(b.path, "button"),
                                    children: "\xd7"
                                }),
                                menuFor === b.path && /*#__PURE__*/ (0,jsx_runtime.jsx)(RowMenu, {
                                    buffer: b,
                                    count: p.buffers.length,
                                    onDone: ()=>setMenuFor(null),
                                    actions: {
                                        pin: p.onPin,
                                        unpin: p.onUnpin,
                                        close: (path)=>p.onClose(path, "context_menu"),
                                        closeOthers: p.onCloseOthers,
                                        copy: p.onCopyPath
                                    },
                                    label: tabLabels_baseName(b.path)
                                })
                            ]
                        }, b.path);
                    })
                })
            ]
        })
    });
}
function RowMenu(param) {
    let { buffer, count, label, onDone, actions } = param;
    const ref = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        var _ref_current_querySelector, _ref_current;
        (_ref_current = ref.current) === null || _ref_current === void 0 ? void 0 : (_ref_current_querySelector = _ref_current.querySelector("[role=menuitem]")) === null || _ref_current_querySelector === void 0 ? void 0 : _ref_current_querySelector.focus();
    }, []);
    const items = [
        [
            buffer.pinned ? "Unpin" : "Pin",
            ()=>(buffer.pinned ? actions.unpin : actions.pin)(buffer.path),
            buffer.pinned && tabLabels_isDirty(buffer)
        ],
        [
            "Close",
            ()=>actions.close(buffer.path)
        ],
        [
            "Close others",
            ()=>actions.closeOthers(buffer.path),
            count < 2
        ],
        [
            "Copy path",
            ()=>actions.copy(buffer.path)
        ]
    ];
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        ref: ref,
        className: rowMenu,
        role: "menu",
        "aria-label": "".concat(label, " actions"),
        children: items.map((param)=>{
            let [name, run, disabled] = param;
            return /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                role: "menuitem",
                className: TabSwitcherSheet_css_menuItem,
                style: {
                    minHeight: TARGET_PX
                },
                disabled: disabled,
                onClick: ()=>{
                    run();
                    onDone();
                },
                children: name
            }, name);
        })
    });
}

;// ./src/components/sessions/files/MobileFilesLayout.tsx
/* __next_internal_client_entry_do_not_use__ MobileFilesLayout auto */ 













const MobileFilesLayout_TOAST_MS = 8000;
function noticeText(n) {
    if (!n) return null;
    return n.kind === "evicted" ? "Closed ".concat(tabLabels_baseName(n.path), " to stay within ").concat(n.cap, " tabs") : "Too many tabs open - close one";
}
/** Top bar, floating controls and bottom action bar around the three panes (ux.md 7.1). */ function MobileFilesLayout(p) {
    const { v2 } = p;
    const on = p.active && !!v2;
    const { isVirtualKeyboardOpen } = (0,ViewportProvider/* useViewport */.k)();
    const coarse = (0,useCoarsePointer/* useCoarsePointer */.O)();
    const analytics = (0,filesAnalytics/* useFilesAnalytics */.R)({
        component: "MobileFilesLayout"
    });
    const [sheetOpen, setSheetOpen] = (0,react.useState)(false);
    const [shown, setShown] = (0,react.useState)(null);
    var _v2_buffers_state_notice;
    const notice = (_v2_buffers_state_notice = v2 === null || v2 === void 0 ? void 0 : v2.buffers.state.notice) !== null && _v2_buffers_state_notice !== void 0 ? _v2_buffers_state_notice : null;
    // The reducer clears its notice on the next action; the layout keeps what it showed until dismissed or timed out.
    (0,react.useEffect)(()=>{
        if (!notice) return;
        setShown(notice);
        if (notice.kind === "refused") setSheetOpen(true);
    }, [
        notice
    ]);
    (0,react.useEffect)(()=>{
        if ((shown === null || shown === void 0 ? void 0 : shown.kind) !== "evicted") return;
        const t = setTimeout(()=>setShown(null), MobileFilesLayout_TOAST_MS);
        return ()=>clearTimeout(t);
    }, [
        shown
    ]);
    const tabActions = useTabActions(v2);
    const overflow = (0,react.useMemo)(()=>{
        if (!v2) return [];
        const active = v2.buffers.active;
        var _p_overflowItems;
        return [
            {
                label: "Outline",
                run: ()=>v2.bridge.setOverlay("outline"),
                disabled: !active
            },
            {
                label: v2.accessible.on ? "Switch to code editor" : "Switch to accessible view",
                run: ()=>v2.accessible.toggle("overflow"),
                disabled: !active
            },
            {
                label: "Copy path",
                run: ()=>active && tabActions.copyPath(active.path),
                disabled: !active
            },
            {
                label: "Text larger (A+)",
                run: ()=>void (0,fontSize/* stepFontPx */.Es)(1, coarse)
            },
            {
                label: "Text smaller (A-)",
                run: ()=>void (0,fontSize/* stepFontPx */.Es)(-1, coarse)
            },
            ...(_p_overflowItems = p.overflowItems) !== null && _p_overflowItems !== void 0 ? _p_overflowItems : []
        ];
    }, [
        v2,
        p.overflowItems,
        tabActions,
        coarse
    ]);
    if (!on || !v2) return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            null,
            p.children,
            null
        ]
    });
    const { buffers, bridge } = v2;
    const { state } = buffers;
    const active = buffers.active;
    const emitClose = (closing, method)=>{
        const dirty = closing.some(tabLabels_isDirty);
        analytics.emit("files.tab.close", {
            dirty: dirty ? "true" : "false",
            method,
            open_tabs: (0,events/* bucketTabs */.FP)(state.buffers.length)
        });
    };
    const closeFromSheet = (path, method)=>{
        emitClose(state.buffers.filter((x)=>x.path === path), method);
        buffers.close(path);
    };
    // One event per gesture, not per closed tab.
    const closeOthersFromSheet = (keep)=>{
        emitClose(state.buffers.filter((x)=>x.path !== keep), "close_others");
        tabActions.closeOthers(keep);
    };
    const selectFromSheet = (path)=>{
        analytics.emit("files.tab.switch", {
            via: "sheet"
        });
        bridge.activate(path);
        setSheetOpen(false);
        // The sheet's own history entry pops first (async); only then may the editor pane push its entry.
        if (p.pane !== "editor") window.addEventListener("popstate", ()=>p.onPane("editor", "bottom_bar"), {
            once: true
        });
    };
    const message = noticeText(shown);
    var _active_path;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)(MobileTopBar, {
                pane: p.pane,
                origin: p.origin,
                path: (_active_path = active === null || active === void 0 ? void 0 : active.path) !== null && _active_path !== void 0 ? _active_path : null,
                dirty: !!active && tabLabels_isDirty(active),
                tabCount: state.buffers.length,
                overflow: overflow,
                onBack: p.onBack,
                onTabs: ()=>setSheetOpen(true)
            }),
            p.children,
            !isVirtualKeyboardOpen && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: stackHost,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: floatingStack,
                                "data-testid": "floating-controls",
                                children: p.quickfix && p.pane === "editor" && /*#__PURE__*/ (0,jsx_runtime.jsx)(QuickfixChip, {
                                    view: p.quickfix,
                                    stacked: true
                                })
                            }),
                            (shown === null || shown === void 0 ? void 0 : shown.kind) === "evicted" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: MobileFilesLayout_css_toast,
                                "data-testid": "files-tab-toast",
                                role: "group",
                                "aria-label": "Tab closed",
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        children: message
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        type: "button",
                                        className: MobileFilesLayout_css_toastButton,
                                        onClick: ()=>{
                                            tabActions.reopen();
                                            setShown(null);
                                        },
                                        children: "Undo"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        type: "button",
                                        className: MobileFilesLayout_css_toastButton,
                                        "aria-label": "Dismiss",
                                        onClick: ()=>setShown(null),
                                        children: "\xd7"
                                    })
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(MobileBottomBar, {
                        pane: p.pane,
                        width: p.containerWidth,
                        hasFile: !!active,
                        canBack: canGoBack(state),
                        canForward: canGoForward(state),
                        onFiles: ()=>p.onPane("files", "bottom_bar"),
                        onSearch: p.onSearch,
                        onQuickOpen: p.onQuickOpen,
                        onGoToLine: ()=>bridge.setOverlay("goto"),
                        onBack: ()=>bridge.back("button"),
                        onForward: ()=>bridge.forward("button")
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: MobileFilesLayout_css_srOnly,
                role: "status",
                "aria-live": "polite",
                children: message
            }),
            sheetOpen && /*#__PURE__*/ (0,jsx_runtime.jsx)(TabSwitcherSheet, {
                buffers: state.buffers,
                activeIndex: state.activeIndex,
                refusal: (shown === null || shown === void 0 ? void 0 : shown.kind) === "refused" ? message : null,
                onSelect: selectFromSheet,
                onClose: closeFromSheet,
                onPin: buffers.pin,
                onUnpin: buffers.unpin,
                onCloseOthers: closeOthersFromSheet,
                onCopyPath: tabActions.copyPath,
                onDismiss: ()=>setSheetOpen(false),
                onGoToFiles: ()=>{
                    setSheetOpen(false);
                    p.onPane("files", "bottom_bar");
                }
            })
        ]
    });
}

;// ./src/components/sessions/files/ActiveBufferWait.tsx
/* __next_internal_client_entry_do_not_use__ useActiveBufferWait auto */ 



const ActiveBufferWait_baseName = (path)=>path.slice(path.lastIndexOf("/") + 1);
/**
 * The open wait of ux.md 8.5 for the active buffer: after 150 ms a pending panel ("Opening b.go..."),
 * on failure a message with Retry and Close tab (not found also offers Reveal in tree). The viewer stays
 * mounted underneath and is only hidden, so its own load is never restarted.
 */ function useActiveBufferWait(v2, fileTreeRef) {
    const analytics = (0,filesAnalytics/* useFilesAnalytics */.R)({
        component: "FileContentViewer"
    });
    var _v2_buffers_active;
    const active = (_v2_buffers_active = v2 === null || v2 === void 0 ? void 0 : v2.buffers.active) !== null && _v2_buffers_active !== void 0 ? _v2_buffers_active : null;
    var _v2_buffers_state_buffers_filter_length;
    const loadingCount = (_v2_buffers_state_buffers_filter_length = v2 === null || v2 === void 0 ? void 0 : v2.buffers.state.buffers.filter((b)=>b.status === "loading").length) !== null && _v2_buffers_state_buffers_filter_length !== void 0 ? _v2_buffers_state_buffers_filter_length : 0;
    const loading = (active === null || active === void 0 ? void 0 : active.status) === "loading" || loadingCount > 1;
    const showPending = (0,PendingState/* usePendingDelay */.oQ)(loading, PendingState/* PENDING_MS */.ls);
    const failed = (active === null || active === void 0 ? void 0 : active.status) === "failed" ? active : null;
    const failureKey = failed ? "".concat(failed.path, ":").concat(failed.reason) : null;
    // One event per failure shown, enum reason only.
    (0,react.useEffect)(()=>{
        if (failed) analytics.emit("files.open.failed", {
            reason: failed.reason
        });
    // failureKey identifies the failure; `failed` itself changes identity on unrelated state updates.
    }, [
        failureKey
    ]); // eslint-disable-line react-hooks/exhaustive-deps
    if (!v2 || !active) return {
        node: null,
        hideViewer: false
    };
    const name = ActiveBufferWait_baseName(active.path);
    if (failed) {
        const { buffers, bridge } = v2;
        const actions = [
            {
                label: "Retry",
                onClick: ()=>{
                    buffers.close(failed.path);
                    bridge.open(failed.path);
                }
            },
            {
                label: "Close tab",
                onClick: ()=>buffers.close(failed.path)
            },
            ...failed.reason === "not_found" ? [
                {
                    label: "Reveal in tree",
                    onClick: ()=>{
                        var _fileTreeRef_current;
                        return (_fileTreeRef_current = fileTreeRef.current) === null || _fileTreeRef_current === void 0 ? void 0 : _fileTreeRef_current.revealPath(failed.path);
                    }
                }
            ] : []
        ];
        return {
            node: /*#__PURE__*/ (0,jsx_runtime.jsx)(PendingState/* WaitState */.Dt, {
                kind: "open",
                name: name,
                failure: {
                    reason: failed.reason,
                    actions
                }
            }),
            hideViewer: true
        };
    }
    // Several tabs loading at once is a session restore ("Restoring 3 tabs"); a failure lands per tab above.
    if (showPending) return {
        node: loadingCount > 1 ? /*#__PURE__*/ (0,jsx_runtime.jsx)(PendingState/* WaitState */.Dt, {
            kind: "restore",
            count: loadingCount
        }) : /*#__PURE__*/ (0,jsx_runtime.jsx)(PendingState/* WaitState */.Dt, {
            kind: "open",
            name: name
        }),
        hideViewer: active.status === "loading"
    };
    return {
        node: null,
        hideViewer: false
    };
}

;// ./src/lib/hooks/useSearchPane.ts
/* __next_internal_client_entry_do_not_use__ useSearchPane auto */ 
/** Which left view shows (Explorer or Search), and the glue between a search hit and the buffers. */ function useSearchPane(v2, param) {
    let { expandPanel, showTreePane, showContentPane } = param;
    const [leftPane, setLeftPane] = (0,react.useState)("explorer");
    const [focusToken, setFocusToken] = (0,react.useState)(0);
    const chooseLeftPane = (0,react.useCallback)((pane)=>{
        setLeftPane(pane);
        if (pane === "search") setFocusToken((n)=>n + 1);
    }, []);
    const openSearch = (0,react.useCallback)(()=>{
        expandPanel();
        showTreePane();
        chooseLeftPane("search");
    }, [
        expandPanel,
        showTreePane,
        chooseLeftPane
    ]);
    const buffers = v2 === null || v2 === void 0 ? void 0 : v2.buffers.state.buffers;
    const jumpTo = v2 === null || v2 === void 0 ? void 0 : v2.bridge.jumpTo;
    const openHit = (0,react.useCallback)((hit)=>{
        if (!jumpTo) return false;
        const reused = !!(buffers === null || buffers === void 0 ? void 0 : buffers.some((b)=>b.path === hit.path));
        const dirty = !!(buffers === null || buffers === void 0 ? void 0 : buffers.some((b)=>b.path === hit.path && b.status === "loaded" && b.dirty));
        jumpTo(hit.path, hit.line, {
            source: "search",
            hit: {
                line: hit.line,
                text: hit.text,
                start: hit.start,
                end: hit.end,
                dirty
            }
        });
        showContentPane();
        return reused;
    }, [
        jumpTo,
        buffers,
        showContentPane
    ]);
    const dirtyPaths = (0,react.useMemo)(()=>new Set((buffers !== null && buffers !== void 0 ? buffers : []).flatMap((b)=>b.status === "loaded" && b.dirty ? [
                b.path
            ] : [])), [
        buffers
    ]);
    return {
        leftPane,
        showLeftPane: setLeftPane,
        chooseLeftPane,
        focusToken,
        openSearch,
        openHit,
        dirtyPaths
    };
}

// EXTERNAL MODULE: ./src/components/sessions/files/AccessibleView.tsx + 1 modules
var AccessibleView = __webpack_require__(42529);
;// ./src/components/sessions/FilesTab.tsx
/* __next_internal_client_entry_do_not_use__ FilesTab auto */ 






















// ---- Component ----
const FilesTab_RECENT_LIMIT = 8;
const PANEL_ID = "files-editor-panel";
function FilesTab(props) {
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(useFileTransport/* FileTransportProvider */.Bw, {
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(FilesTabGate, {
            ...props
        })
    });
}
function FilesTabGate(props) {
    const { status, viewerV2 } = useFilesFlags();
    if (status === "loading") return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: FilesTab_css_container,
        "data-testid": "files-tab-skeleton",
        "aria-busy": "true"
    });
    return viewerV2 ? /*#__PURE__*/ (0,jsx_runtime.jsx)(BufferedFilesTab, {
        ...props
    }) : /*#__PURE__*/ (0,jsx_runtime.jsx)(LegacyFilesTab, {
        ...props
    });
}
/** files:viewer-v2 on: selection is the active buffer of the open-buffers reducer. */ function BufferedFilesTab(param) {
    let { sessionId, initialSelectedPath, onSelectedPathChange, sessionRunning = false, ...rest } = param;
    const v2 = useFilesV2(sessionId, sessionRunning, {
        initialSelectedPath,
        onSelectedPathChange
    });
    const { open } = v2.bridge;
    const onOpen = (0,react.useCallback)((path, source)=>open(path, {
            source
        }), [
        open
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(FilesTabView, {
        ...rest,
        sessionId: sessionId,
        initialSelectedPath: initialSelectedPath,
        selectedPath: v2.buffers.selectedPath,
        recentPaths: v2.recentPaths,
        onOpen: onOpen,
        v2: v2
    });
}
/** files:viewer-v2 off: the pre-revamp single-selection behavior. */ function LegacyFilesTab(param) {
    let { initialSelectedPath, onSelectedPathChange, ...rest } = param;
    const [selectedPath, setSelectedPath] = (0,react.useState)(initialSelectedPath !== null && initialSelectedPath !== void 0 ? initialSelectedPath : null);
    const [recentPaths, setRecentPaths] = (0,react.useState)([]);
    const onOpen = (0,react.useCallback)((path)=>{
        setSelectedPath(path);
        onSelectedPathChange === null || onSelectedPathChange === void 0 ? void 0 : onSelectedPathChange(path);
        setRecentPaths((prev)=>[
                path,
                ...prev.filter((p)=>p !== path)
            ].slice(0, FilesTab_RECENT_LIMIT));
    }, [
        onSelectedPathChange
    ]);
    (0,react.useEffect)(()=>{
        if (initialSelectedPath !== undefined) setSelectedPath(initialSelectedPath);
    }, [
        initialSelectedPath
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(FilesTabView, {
        ...rest,
        initialSelectedPath: initialSelectedPath,
        selectedPath: selectedPath,
        recentPaths: recentPaths,
        onOpen: onOpen
    });
}
function FilesTabView(param) {
    let { sessionId, baseUrl, initialSelectedPath, selectedPath, recentPaths, onOpen, v2, sessionName, onResumeSession } = param;
    var _v2_buffers_active;
    const [isQuickOpenOpen, setIsQuickOpenOpen] = (0,react.useState)(false);
    const searchInputRef = (0,react.useRef)(null);
    const fileTreeRef = (0,react.useRef)(null);
    const searchRef = (0,react.useRef)(null);
    const [quickfix, setQuickfix] = (0,react.useState)(null);
    const panel = useResizablePanel({
        storageKey: "filestab.treeWidth",
        defaultWidth: 260,
        minWidth: 160,
        maxWidthFraction: 0.5
    });
    const { layout, containerWidth } = useFilesLayout(panel.containerRef);
    const mobile = layout === "mobile";
    const nav = useMobilePane(mobile);
    const { go } = nav;
    const { diff } = (0,SessionVcsContext/* useSessionVcsContext */.l)();
    const handleFileSelect = (0,react.useCallback)((path, source)=>{
        onOpen(path, source);
        go("editor", "tap_hit");
    }, [
        onOpen,
        go
    ]);
    const jumpTo = v2 === null || v2 === void 0 ? void 0 : v2.bridge.jumpTo;
    const onDeepLink = (0,react.useMemo)(()=>jumpTo && function(path) {
            let line = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : 1;
            jumpTo(path, line, {
                pinned: true,
                source: "deeplink"
            });
            go("editor", "tap_hit");
        }, [
        jumpTo,
        go
    ]);
    useFilesDeepLink({
        initialSelectedPath,
        tree: fileTreeRef,
        selectedPath,
        onJump: onDeepLink
    });
    const openQuickOpen = (0,react.useCallback)(()=>setIsQuickOpenOpen(true), []);
    const search = useSearchPane(v2, {
        expandPanel: panel.expand,
        showTreePane: (0,react.useCallback)(()=>go("search", "bottom_bar"), [
            go
        ]),
        showContentPane: (0,react.useCallback)(()=>go("editor", "tap_hit"), [
            go
        ])
    });
    // On mobile the Files and Search panes pick which left view shows; the editor pane leaves it as it was.
    const { showLeftPane } = search;
    (0,react.useEffect)(()=>{
        if (mobile && nav.pane !== "editor") showLeftPane(nav.pane === "search" ? "search" : "explorer");
    }, [
        mobile,
        nav.pane,
        showLeftPane
    ]);
    useFilesShortcuts({
        searchInputRef,
        collapsed: panel.collapsed,
        onQuickOpen: openQuickOpen,
        onSearch: v2 ? search.openSearch : undefined,
        onNextHit: v2 ? ()=>{
            var _searchRef_current;
            return (_searchRef_current = searchRef.current) === null || _searchRef_current === void 0 ? void 0 : _searchRef_current.step(1);
        } : undefined,
        onPrevHit: v2 ? ()=>{
            var _searchRef_current;
            return (_searchRef_current = searchRef.current) === null || _searchRef_current === void 0 ? void 0 : _searchRef_current.step(-1);
        } : undefined,
        ...v2 === null || v2 === void 0 ? void 0 : v2.shortcuts
    });
    const wait = useActiveBufferWait(v2, fileTreeRef);
    const active = v2 === null || v2 === void 0 ? void 0 : v2.buffers.active;
    const treePaneClasses = [
        treePane,
        panel.collapsed && !mobile ? treePaneCollapsed : ""
    ].filter(Boolean).join(" ");
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: FilesTab_css_container,
        ref: panel.containerRef,
        "data-testid": "files-tab",
        "data-layout": layout,
        "data-pane": nav.pane,
        "data-chrome": v2 ? "v2" : "legacy",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)(MobileFilesLayout, {
                active: mobile,
                v2: v2,
                pane: nav.pane,
                origin: nav.origin,
                containerWidth: containerWidth,
                onPane: nav.go,
                onBack: nav.back,
                onQuickOpen: openQuickOpen,
                onSearch: search.openSearch,
                quickfix: quickfix,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: treePaneClasses,
                        "data-testid": "files-tree-pane",
                        style: {
                            width: panel.collapsed ? 0 : panel.width
                        },
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(FilesLeftPane, {
                            sessionId: sessionId,
                            baseUrl: baseUrl,
                            v2: v2,
                            selectedPath: selectedPath,
                            recentPaths: recentPaths,
                            onSelect: handleFileSelect,
                            searchInputRef: searchInputRef,
                            fileTreeRef: fileTreeRef,
                            searchRef: searchRef,
                            panel: panel,
                            search: search,
                            onQuickfix: setQuickfix,
                            sessionName: sessionName,
                            onResumeSession: onResumeSession,
                            mobile: mobile
                        })
                    }),
                    !mobile && !panel.collapsed && /*#__PURE__*/ (0,jsx_runtime.jsx)(TreeResizeHandle, {
                        ...panel.handleProps
                    }),
                    !mobile && panel.collapsed && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: expandStrip,
                        onClick: panel.expand,
                        title: "Expand file tree panel",
                        "aria-label": "Expand file tree panel",
                        children: "⊞"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: contentPane,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: mobileBackButton,
                                onClick: ()=>nav.back(),
                                children: "← Files"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: mobileSearchButton,
                                onClick: openQuickOpen,
                                "aria-label": "Search files",
                                children: "\uD83D\uDD0D"
                            }),
                            v2 && /*#__PURE__*/ (0,jsx_runtime.jsx)(FilesV2Chrome, {
                                v2: v2,
                                fileTreeRef: fileTreeRef,
                                panelId: PANEL_ID,
                                mobile: mobile,
                                onQuickOpen: openQuickOpen,
                                onSearch: search.openSearch
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: tabPanel,
                                role: v2 ? "tabpanel" : undefined,
                                id: PANEL_ID,
                                "aria-labelledby": v2 ? "files-tab-".concat(v2.buffers.state.activeIndex) : undefined,
                                children: [
                                    v2 && active && /*#__PURE__*/ (0,jsx_runtime.jsx)(AccessibleView/* AccessibleSkipLink */.o, {
                                        on: v2.accessible.on,
                                        onToggle: ()=>v2.accessible.toggle("skip_link")
                                    }),
                                    wait.node,
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                        hidden: wait.hideViewer,
                                        style: wait.hideViewer ? undefined : {
                                            display: "contents"
                                        },
                                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(FileContentViewer/* FileContentViewer */.Ol, {
                                            sessionId: sessionId,
                                            filePath: selectedPath,
                                            baseUrl: baseUrl,
                                            diffContent: diff === null || diff === void 0 ? void 0 : diff.content,
                                            reloadKey: v2 === null || v2 === void 0 ? void 0 : v2.bridge.reloadKey,
                                            onEditorReady: v2 === null || v2 === void 0 ? void 0 : v2.bridge.onEditorReady,
                                            accessible: v2 === null || v2 === void 0 ? void 0 : v2.accessible.on,
                                            dirty: (v2 === null || v2 === void 0 ? void 0 : (_v2_buffers_active = v2.buffers.active) === null || _v2_buffers_active === void 0 ? void 0 : _v2_buffers_active.status) === "loaded" && v2.buffers.active.dirty,
                                            source: v2 === null || v2 === void 0 ? void 0 : v2.viewerSource
                                        })
                                    })
                                ]
                            }),
                            v2 && /*#__PURE__*/ (0,jsx_runtime.jsx)(FilesV2Footer, {
                                v2: v2,
                                quickfix: mobile ? null : quickfix
                            })
                        ]
                    })
                ]
            }),
            isQuickOpenOpen && /*#__PURE__*/ (0,jsx_runtime.jsx)(QuickOpenPalette, {
                sessionId: sessionId,
                baseUrl: baseUrl,
                recentPaths: recentPaths,
                commands: v2 === null || v2 === void 0 ? void 0 : v2.paletteCommands,
                onSelect: (path)=>{
                    var _fileTreeRef_current;
                    setIsQuickOpenOpen(false);
                    handleFileSelect(path, "palette");
                    (_fileTreeRef_current = fileTreeRef.current) === null || _fileTreeRef_current === void 0 ? void 0 : _fileTreeRef_current.revealPath(path);
                },
                onClose: ()=>setIsQuickOpenOpen(false)
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/create.js
var create = __webpack_require__(95444);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FBrowserTab.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA92WUW_bIBDH3_MpUKVKrTQix22alGhP-xKTpmnC5hKzYkCAEztTv_sEjl3qxmm97WnKg2PfwfH_3XEw39Z6DQ8J-jVDiHGrBW0I2gqoNzMUnphxA7njShKUK1GV0lsK4LvCEbRIkmv_rpXlrY8BQR3fg_-q9mC2Qh0IKjhjIDez59m8jbgIEX0AghZTZkCo5BJ38ZNoyvS8CCr4TmLuoLQE5SAdGP_5Z2Ud3zY4V9KBdLFpKG4cw45qgvbU3GD8Y_EIx9Qc0-Ott-RKKPPGFkxbJR22_AhD810W7A5qh8Oq40VpyhiXuzdjFrcRgruA4AUlzawSlQsoubQwIHZ_wd2pMWkCto6gZcvGGSrtVpmStH8FdfD1Bi-T6-B7xFyykONkMyE541xHMKTN7RBMEvwzZRgYbCjjlX0D77H1ofnTzqhKsqFDM57L44dymaka24IyX8EJSnWN1rpGZpfRm-RT-M3botCKe_EY9iCdJYhWTkWZWraZ6sT7ic7iuSz3IfIhaKFrZJXgbOAGb6CEzGpqQLqIB5cFGO4-giGvjPVDTipjYaTwezzIG8_DOi7yh0n7fLyUum2ervVLt7OF4fIpbJOId3KRdqacU-VloP3qV69Xz6XgEvAfiGhe74f_pyRW00tiRRi3NBPAwiilac5dQ1AyX8axpPKNVagDsHjwN2o4xdqAtcA-XzlTwdX3D4Tv2Z7U55XxPL74t2j-9XtdNtSa6Q80P21bUOHlwJkrCLpLk7ZM-5aaJvEJiZuuaUw6yCd32q6vjpJZxWTas-JyWV2ulB7j498d71bTHHAG7gBwUXkfkIaAJa3xKQXdfeBf7ag-UnaKZHZcDlvP2dz4RfW3lPWpMF7uSl0dBLJbWnLREFQqqQKEAfNkvloaKEOpFdwBDk4EaQMTWsO7SHqxeRB7HmmuhKDaQqjQ8G-6jCiQY6-PTa_hvoW1B-N4TkV30XJKBwLKMJwZoE8EhYfvF5vZ828eALdyKAsAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FBrowserTab_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA92WUW_bIBDH3_MpUKVKrTQix22alGhP_xKTpmnC5hKzYkCAEztTv_sEjl3qxmm97WnKg2PfwfH_3XEw39Z6DQ8J_jVDiHGrBW0I2gqoNzMUnphxA7njShKUK1GV0lsK4LvCEbRIkmv_rpXlrY8BQR3fg_q9mC2Qh0IKjhjIDez59m8jbgIEX0AghZTZkCo5BJ38ZNoyvS8CCr4TmLuoLQE5SAdGP_5Z2Ud3zY4V9KBdLFpKG4cw45qgvbU3GD8Y_EIx9Qc0_Ott_RKKPPGFkxbJR22_AhD810W7A5qh8Oq40VpyhiXuzdjFrcRgruA4AUlzawSlQsoubQwIHZ_wd2pMWkCto6gZcvGGSrtVpmStH8FdfD1Bi_T6_B7xFyykONkMyE541xHMKTN7RBMEvwzZRgYbCjjlX0D77H1ofnTzqhKsqFDM57L44dymaka24IyX8EJSnWN1rpGZpfRm_RT_M3botCKe_EY9iCdJYhWTkWZWraZ6sT7ic7iuSz3IfIhaKFrZJXgbOAGb6CEzGpqQLqIB5cFGO4_giGvjPVDTipjYaTwezzIG8_DOi7yh0n7fLyUum2ervVLt7OF4fIpbJOId3KRdqacU_VloP3qV69Xz6XgEvAfiGhe74f_pyRW00tiRRi3NBPAwiilac5dQ1AyX8axpPKNVagDsHjwN2o4xdqAtcA_XzlTwdX3D4Tv2Z7U55XxPL74t2j_9XtdNtSa6Q80P21bUOHlwJkrCLpLk7ZM_5aaJvEJiZuuaUw6yCd32q6vjpJZxWTas_JyWV2ulB7j498d71bTHHAG7gBwUXkfkIaAJa3xKQXdfeBf7ag_UnaKZHZcDlvP2dz4RfW3lPWpMF7uSl0dBLJbWnLREFQqqQKEAfNkvloaKEOpFdwBDk4EaQMTWsO7SHqxeRB7HmmuhKDaQqjQ8G_6jCiQY6_PTa_hvoW1B_N4TkV30XJKBwLKMJwZoE8EhYfvF5vZ828eALdyKAsAAA_22_7D = __webpack_require__(52176);
;// ./src/components/sessions/BrowserTab.css.ts

var browserTabContainer = 'fxp8e60';
var canvasWrapper = 'fxp8e63';
var cssTable = 'fxp8e6c';
var inspectButton = 'fxp8e67';
var placeholderOverlay = 'fxp8e62';
var reconnectButton = 'fxp8e65';
var reconnectingBanner = 'fxp8e64';
var resultCode = 'fxp8e6b';
var resultHeader = 'fxp8e69';
var resultPanel = 'fxp8e68';
var resultScreenshot = 'fxp8e6a';
var BrowserTab_css_toolbar = 'fxp8e66';
var viewerArea = 'fxp8e61';
;// ./src/components/sessions/CDPViewer.tsx
/* __next_internal_client_entry_do_not_use__ default auto */ 

// Approximate hover box (Chrome pixels). The exact element box is only known
// after a click, so this is a cursor cue, not a selector.
const HOVER_BOX_W = 120;
const HOVER_BOX_H = 60;
// CDPViewer connects to a WebSocket that streams JPEG frames (binary messages)
// and renders them to a canvas. Mouse and keyboard events are forwarded back
// to the server as CDP Input.dispatch* JSON messages.
function CDPViewer(param) {
    let { wsUrl, isVisible, onConnected, onDisconnected, inspectMode = false, onInspectClick } = param;
    const canvasRef = (0,react.useRef)(null);
    const overlayRef = (0,react.useRef)(null);
    const wsRef = (0,react.useRef)(null);
    const reconnectTimerRef = (0,react.useRef)(null);
    const unmountedRef = (0,react.useRef)(false);
    // Keep latest callbacks in a ref so they can be used in stable event handlers
    const onConnectedRef = (0,react.useRef)(onConnected);
    const onDisconnectedRef = (0,react.useRef)(onDisconnected);
    const isVisibleRef = (0,react.useRef)(isVisible);
    const [connectionStatusText, setConnectionStatusText] = (0,react.useState)('Browser disconnected');
    (0,react.useEffect)(()=>{
        onConnectedRef.current = onConnected;
    }, [
        onConnected
    ]);
    (0,react.useEffect)(()=>{
        onDisconnectedRef.current = onDisconnected;
    }, [
        onDisconnected
    ]);
    (0,react.useEffect)(()=>{
        isVisibleRef.current = isVisible;
    }, [
        isVisible
    ]);
    // Leaving inspect mode clears any hover rectangle left on the overlay.
    const wasInspectingRef = (0,react.useRef)(false);
    (0,react.useEffect)(()=>{
        if (wasInspectingRef.current && !inspectMode) clearOverlay();
        wasInspectingRef.current = inspectMode;
    }, [
        inspectMode
    ]);
    // ------------------------------------------------------------------
    // WebSocket lifecycle
    // ------------------------------------------------------------------
    (0,react.useEffect)(()=>{
        unmountedRef.current = false;
        // renderFrame only accesses canvasRef (a stable ref) so it does not need
        // to be listed in the effect's dependency array.
        async function renderFrame(data) {
            const canvas = canvasRef.current;
            if (!canvas) return;
            const ctx = canvas.getContext('2d');
            if (!ctx) return;
            if (typeof createImageBitmap !== 'undefined') {
                // Fast path: use createImageBitmap (supported in modern browsers)
                const blob = new Blob([
                    data
                ], {
                    type: 'image/jpeg'
                });
                try {
                    const bitmap = await createImageBitmap(blob);
                    canvas.width = bitmap.width;
                    canvas.height = bitmap.height;
                    ctx.drawImage(bitmap, 0, 0);
                    bitmap.close();
                } catch (e) {
                // Ignore decode errors on individual frames
                }
            } else {
                // Fallback: use an Image element
                const blob = new Blob([
                    data
                ], {
                    type: 'image/jpeg'
                });
                const url = URL.createObjectURL(blob);
                const img = new Image();
                img.onload = ()=>{
                    canvas.width = img.naturalWidth;
                    canvas.height = img.naturalHeight;
                    ctx.drawImage(img, 0, 0);
                    URL.revokeObjectURL(url);
                };
                img.onerror = ()=>URL.revokeObjectURL(url);
                img.src = url;
            }
        }
        function connect() {
            if (unmountedRef.current) return;
            const ws = new WebSocket(wsUrl);
            ws.binaryType = 'arraybuffer';
            wsRef.current = ws;
            ws.addEventListener('open', ()=>{
                var _onConnectedRef_current;
                if (unmountedRef.current) {
                    ws.close();
                    return;
                }
                setConnectionStatusText('Browser connected');
                (_onConnectedRef_current = onConnectedRef.current) === null || _onConnectedRef_current === void 0 ? void 0 : _onConnectedRef_current.call(onConnectedRef);
            });
            ws.addEventListener('message', (event)=>{
                if (event.data instanceof ArrayBuffer) {
                    void renderFrame(event.data);
                }
            // Text messages from server (status, etc.) are intentionally ignored
            });
            ws.addEventListener('close', ()=>{
                var _onDisconnectedRef_current;
                wsRef.current = null;
                setConnectionStatusText('Reconnecting to browser…');
                (_onDisconnectedRef_current = onDisconnectedRef.current) === null || _onDisconnectedRef_current === void 0 ? void 0 : _onDisconnectedRef_current.call(onDisconnectedRef);
                if (!unmountedRef.current) {
                    // Reconnect with 2 s backoff
                    reconnectTimerRef.current = setTimeout(connect, 2000);
                }
            });
            ws.addEventListener('error', ()=>{
            // Error is always followed by a close event; let close handle reconnection
            });
        }
        connect();
        return ()=>{
            unmountedRef.current = true;
            if (reconnectTimerRef.current !== null) {
                clearTimeout(reconnectTimerRef.current);
                reconnectTimerRef.current = null;
            }
            if (wsRef.current) {
                wsRef.current.close();
                wsRef.current = null;
            }
        };
    }, [
        wsUrl
    ]);
    // ------------------------------------------------------------------
    // Input event helpers
    // ------------------------------------------------------------------
    function sendJson(obj) {
        const ws = wsRef.current;
        if (ws && ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify(obj));
        }
    }
    function getCanvasCoords(e) {
        const canvas = canvasRef.current;
        if (!canvas) return {
            x: 0,
            y: 0
        };
        const rect = canvas.getBoundingClientRect();
        // Scale CSS pixels → Chrome device pixels using canvas intrinsic size
        const scaleX = canvas.width / rect.width;
        const scaleY = canvas.height / rect.height;
        return {
            x: Math.round((e.clientX - rect.left) * scaleX),
            y: Math.round((e.clientY - rect.top) * scaleY)
        };
    }
    function getModifiers(e) {
        // CDP modifier bitmask: Alt=1, Ctrl=2, Meta/Cmd=4, Shift=8
        let m = 0;
        if (e.altKey) m |= 1;
        if (e.ctrlKey) m |= 2;
        if (e.metaKey) m |= 4;
        if (e.shiftKey) m |= 8;
        return m;
    }
    function cdpButton(e) {
        switch(e.button){
            case 0:
                return 'left';
            case 1:
                return 'middle';
            case 2:
                return 'right';
            default:
                return 'none';
        }
    }
    // ------------------------------------------------------------------
    // Mouse handlers
    // ------------------------------------------------------------------
    function clearOverlay() {
        var _overlay_getContext;
        const overlay = overlayRef.current;
        overlay === null || overlay === void 0 ? void 0 : (_overlay_getContext = overlay.getContext('2d')) === null || _overlay_getContext === void 0 ? void 0 : _overlay_getContext.clearRect(0, 0, overlay.width, overlay.height);
    }
    function drawHoverBox(x, y) {
        const canvas = canvasRef.current;
        const overlay = overlayRef.current;
        const ctx = overlay === null || overlay === void 0 ? void 0 : overlay.getContext('2d');
        if (!canvas || !overlay || !ctx) return;
        if (overlay.width !== canvas.width) overlay.width = canvas.width;
        if (overlay.height !== canvas.height) overlay.height = canvas.height;
        ctx.clearRect(0, 0, overlay.width, overlay.height);
        ctx.fillStyle = 'rgba(66, 133, 244, 0.2)';
        ctx.strokeStyle = 'rgba(66, 133, 244, 0.9)';
        ctx.lineWidth = 2;
        ctx.fillRect(x - HOVER_BOX_W / 2, y - HOVER_BOX_H / 2, HOVER_BOX_W, HOVER_BOX_H);
        ctx.strokeRect(x - HOVER_BOX_W / 2, y - HOVER_BOX_H / 2, HOVER_BOX_W, HOVER_BOX_H);
    }
    function handleMouseDown(e) {
        if (!isVisibleRef.current) return;
        const { x, y } = getCanvasCoords(e);
        if (inspectMode) {
            if (e.button === 0) onInspectClick === null || onInspectClick === void 0 ? void 0 : onInspectClick(x, y);
            return;
        }
        sendJson({
            method: 'Input.dispatchMouseEvent',
            params: {
                type: 'mousePressed',
                x,
                y,
                button: cdpButton(e),
                clickCount: 1,
                modifiers: getModifiers(e)
            }
        });
    }
    function handleMouseUp(e) {
        if (!isVisibleRef.current || inspectMode) return;
        const { x, y } = getCanvasCoords(e);
        sendJson({
            method: 'Input.dispatchMouseEvent',
            params: {
                type: 'mouseReleased',
                x,
                y,
                button: cdpButton(e),
                clickCount: 1,
                modifiers: getModifiers(e)
            }
        });
    }
    function handleMouseMove(e) {
        if (!isVisibleRef.current) return;
        const { x, y } = getCanvasCoords(e);
        if (inspectMode) {
            drawHoverBox(x, y);
            return;
        }
        sendJson({
            method: 'Input.dispatchMouseEvent',
            params: {
                type: 'mouseMoved',
                x,
                y,
                button: 'none',
                clickCount: 0,
                modifiers: getModifiers(e)
            }
        });
    }
    // ------------------------------------------------------------------
    // Keyboard handlers
    // ------------------------------------------------------------------
    function handleKeyDown(e) {
        if (!isVisibleRef.current) return;
        e.preventDefault();
        sendJson({
            method: 'Input.dispatchKeyEvent',
            params: {
                type: 'keyDown',
                key: e.key,
                code: e.code,
                modifiers: getModifiers(e)
            }
        });
    }
    function handleKeyUp(e) {
        if (!isVisibleRef.current) return;
        sendJson({
            method: 'Input.dispatchKeyEvent',
            params: {
                type: 'keyUp',
                key: e.key,
                code: e.code,
                modifiers: getModifiers(e)
            }
        });
    }
    // ------------------------------------------------------------------
    // Wheel handler
    // ------------------------------------------------------------------
    function handleWheel(e) {
        if (!isVisibleRef.current) return;
        e.preventDefault();
        const canvas = canvasRef.current;
        if (!canvas) return;
        const rect = canvas.getBoundingClientRect();
        const scaleX = canvas.width / rect.width;
        const scaleY = canvas.height / rect.height;
        const x = Math.round((e.clientX - rect.left) * scaleX);
        const y = Math.round((e.clientY - rect.top) * scaleY);
        sendJson({
            method: 'Input.dispatchMouseEvent',
            params: {
                type: 'mouseWheel',
                x,
                y,
                deltaX: e.deltaX,
                deltaY: e.deltaY,
                modifiers: getModifiers(e)
            }
        });
    }
    // ------------------------------------------------------------------
    // Touch handlers
    // ------------------------------------------------------------------
    function handleTouchStart(e) {
        if (!isVisibleRef.current || inspectMode) return;
        e.preventDefault();
        const touch = e.touches[0];
        if (!touch) return;
        const canvas = canvasRef.current;
        if (!canvas) return;
        const rect = canvas.getBoundingClientRect();
        const scaleX = canvas.width / rect.width;
        const scaleY = canvas.height / rect.height;
        const x = Math.round((touch.clientX - rect.left) * scaleX);
        const y = Math.round((touch.clientY - rect.top) * scaleY);
        sendJson({
            method: 'Input.dispatchMouseEvent',
            params: {
                type: 'mousePressed',
                x,
                y,
                button: 'left',
                clickCount: 1
            }
        });
    }
    function handleTouchMove(e) {
        if (!isVisibleRef.current || inspectMode) return;
        e.preventDefault();
        const touch = e.touches[0];
        if (!touch) return;
        const canvas = canvasRef.current;
        if (!canvas) return;
        const rect = canvas.getBoundingClientRect();
        const scaleX = canvas.width / rect.width;
        const scaleY = canvas.height / rect.height;
        const x = Math.round((touch.clientX - rect.left) * scaleX);
        const y = Math.round((touch.clientY - rect.top) * scaleY);
        sendJson({
            method: 'Input.dispatchMouseEvent',
            params: {
                type: 'mouseMoved',
                x,
                y
            }
        });
    }
    function handleTouchEnd(e) {
        if (!isVisibleRef.current || inspectMode) return;
        e.preventDefault();
        const canvas = canvasRef.current;
        if (!canvas) return;
        // changedTouches has the touch point that was lifted
        const touch = e.changedTouches[0];
        if (!touch) return;
        const rect = canvas.getBoundingClientRect();
        const scaleX = canvas.width / rect.width;
        const scaleY = canvas.height / rect.height;
        const x = Math.round((touch.clientX - rect.left) * scaleX);
        const y = Math.round((touch.clientY - rect.top) * scaleY);
        sendJson({
            method: 'Input.dispatchMouseEvent',
            params: {
                type: 'mouseReleased',
                x,
                y,
                button: 'left',
                clickCount: 1
            }
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        style: {
            position: 'relative',
            width: '100%',
            height: '100%'
        },
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("canvas", {
                ref: canvasRef,
                role: "img",
                "aria-label": "Remote browser display",
                tabIndex: 0,
                style: {
                    width: '100%',
                    height: '100%',
                    display: 'block',
                    cursor: inspectMode ? 'crosshair' : undefined
                },
                onMouseDown: handleMouseDown,
                onMouseUp: handleMouseUp,
                onMouseMove: handleMouseMove,
                onKeyDown: handleKeyDown,
                onKeyUp: handleKeyUp,
                onWheel: handleWheel,
                onTouchStart: handleTouchStart,
                onTouchMove: handleTouchMove,
                onTouchEnd: handleTouchEnd,
                onMouseLeave: inspectMode ? clearOverlay : undefined
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("canvas", {
                ref: overlayRef,
                "data-testid": "cdp-inspect-overlay",
                "aria-hidden": "true",
                style: {
                    position: 'absolute',
                    top: 0,
                    left: 0,
                    width: '100%',
                    height: '100%',
                    pointerEvents: 'none',
                    display: inspectMode ? 'block' : 'none'
                }
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                role: "status",
                "aria-live": "polite",
                style: {
                    position: 'absolute',
                    width: 1,
                    height: 1,
                    overflow: 'hidden',
                    clip: 'rect(0,0,0,0)'
                },
                children: connectionStatusText
            })
        ]
    });
}

;// ./src/components/sessions/BrowserTab.tsx
/* __next_internal_client_entry_do_not_use__ VNCStatus,buildInspectUrl,formatInspectContext,buildWsUrl,BrowserTab auto */ 









/** REST URL for the inspect endpoint; baseUrl ends in /api (see BrowserTab props). */ function buildInspectUrl(baseUrl, sessionId) {
    return "".concat(baseUrl.replace(/\/+$/, ''), "/sessions/").concat(sessionId, "/inspect-element");
}
// Page content is untrusted: a CR would submit the line and ESC could inject
// terminal escape sequences, so control characters never reach the PTY.
// eslint-disable-next-line no-control-regex
const CONTROL_CHARS = /[\u0000-\u001f\u007f-\u009f]+/g;
function stripControlChars(s) {
    return s.replace(CONTROL_CHARS, ' ');
}
/** Text injected into the terminal by "Send to Agent" (no trailing newline: the user confirms). */ function formatInspectContext(x, y, r) {
    var _r_computedCss;
    const css = ((_r_computedCss = r.computedCss) !== null && _r_computedCss !== void 0 ? _r_computedCss : []).map((p)=>"".concat(p.name, ": ").concat(p.value)).join('; ');
    return "[Browser Inspector] Element at (".concat(x, ", ").concat(y, "):\nHTML: ").concat(stripControlChars(r.html), "\nCSS: ").concat(stripControlChars(css));
}
function buildWsUrl(baseUrl, sessionId) {
    // Resolve protocol-relative (//host/path) and relative (/path) URLs to absolute
    // before doing scheme substitution, so we never produce a broken WebSocket URL.
    let absolute = baseUrl;
    if (absolute.startsWith('//')) {
        // Protocol-relative: prepend the current page protocol.
        absolute = "".concat(window.location.protocol).concat(absolute);
    } else if (absolute.startsWith('/') || !absolute.startsWith('http://') && !absolute.startsWith('https://') && !absolute.startsWith('ws://') && !absolute.startsWith('wss://')) {
        // Relative path or no scheme: make it absolute using the current origin.
        absolute = "".concat(window.location.protocol, "//").concat(window.location.host).concat(absolute.startsWith('/') ? '' : '/').concat(absolute);
    }
    // If already a WebSocket URL, use as-is (no double-substitution).
    if (absolute.startsWith('ws://') || absolute.startsWith('wss://')) {
        const origin = absolute.replace(/\/+$/, '').replace(/\/api$/, '');
        return "".concat(origin, "/api/sessions/").concat(sessionId, "/cdp-stream");
    }
    // Normalize: strip trailing slashes, then strip /api suffix if present.
    // Handles cases where baseUrl ends with '/' or lacks the /api suffix,
    // which would otherwise produce a doubled path like wss://host/api/api/sessions/...
    const origin = absolute.replace(/\/+$/, '').replace(/\/api$/, '');
    const wsScheme = origin.startsWith('https') ? 'wss' : 'ws';
    const host = origin.replace(/^https?:\/\//, '');
    return "".concat(wsScheme, "://").concat(host, "/api/sessions/").concat(sessionId, "/cdp-stream");
}
function BrowserTab(param) {
    let { sessionId, baseUrl, isVisible, vncState } = param;
    var _vncState_status;
    const status = (_vncState_status = vncState === null || vncState === void 0 ? void 0 : vncState.status) !== null && _vncState_status !== void 0 ? _vncState_status : types_pb/* VNCStatus */.wL.VNC_STATUS_UNSPECIFIED;
    const isUnavailable = status === types_pb/* VNCStatus */.wL.VNC_STATUS_UNAVAILABLE || status === types_pb/* VNCStatus */.wL.VNC_STATUS_UNSPECIFIED;
    const isWaiting = !isUnavailable && (status === types_pb/* VNCStatus */.wL.VNC_STATUS_STARTING || status === types_pb/* VNCStatus */.wL.VNC_STATUS_NO_BROWSER || !(vncState === null || vncState === void 0 ? void 0 : vncState.browserWindowDetected));
    const isReady = status === types_pb/* VNCStatus */.wL.VNC_STATUS_READY && (vncState === null || vncState === void 0 ? void 0 : vncState.browserWindowDetected) === true;
    // Track if we've ever been ready — keeps CDPViewer mounted once connected
    // so the WebSocket connection survives tab switches even if isReady temporarily flickers.
    // This avoids a premature WebSocket handshake during STARTING/NO_BROWSER states.
    const hasBeenReadyRef = (0,react.useRef)(false);
    if (isReady) hasBeenReadyRef.current = true;
    const shouldMountViewer = hasBeenReadyRef.current;
    const wsUrl = buildWsUrl(baseUrl, sessionId);
    // Connection state for reconnecting banner and manual reconnect
    const [connectionState, setConnectionState] = (0,react.useState)('disconnected');
    // Incrementing key forces CDPViewer to re-mount on manual reconnect
    const [viewerKey, setViewerKey] = (0,react.useState)(0);
    // Inspect mode state. The toggle is only usable while the stream is live (isReady).
    const [inspectMode, setInspectMode] = (0,react.useState)(false);
    const [inspecting, setInspecting] = (0,react.useState)(false);
    const [inspectError, setInspectError] = (0,react.useState)(null);
    const [inspectResult, setInspectResult] = (0,react.useState)(null);
    const abortRef = (0,react.useRef)(null);
    const closeInspector = (0,react.useCallback)(()=>{
        var _abortRef_current;
        (_abortRef_current = abortRef.current) === null || _abortRef_current === void 0 ? void 0 : _abortRef_current.abort();
        setInspectMode(false);
        setInspecting(false);
        setInspectError(null);
        setInspectResult(null);
    }, []);
    (0,react.useEffect)(()=>()=>{
            var _abortRef_current;
            return (_abortRef_current = abortRef.current) === null || _abortRef_current === void 0 ? void 0 : _abortRef_current.abort();
        }, []);
    // Stream dropped while inspecting: leave inspect mode.
    (0,react.useEffect)(()=>{
        if (!isReady) closeInspector();
    }, [
        isReady,
        closeInspector
    ]);
    (0,react.useEffect)(()=>{
        if (!inspectMode && !inspectResult) return;
        const onKey = (e)=>{
            if (e.key === 'Escape') closeInspector();
        };
        document.addEventListener('keydown', onKey);
        return ()=>document.removeEventListener('keydown', onKey);
    }, [
        inspectMode,
        inspectResult,
        closeInspector
    ]);
    async function handleInspectClick(x, y) {
        var _abortRef_current;
        (_abortRef_current = abortRef.current) === null || _abortRef_current === void 0 ? void 0 : _abortRef_current.abort();
        const controller = new AbortController();
        abortRef.current = controller;
        setInspecting(true);
        setInspectError(null);
        try {
            const res = await fetch(buildInspectUrl(baseUrl, sessionId), {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    x,
                    y
                }),
                signal: controller.signal
            });
            const body = await res.json().catch(()=>({}));
            var _body_error;
            if (!res.ok) throw new Error((_body_error = body === null || body === void 0 ? void 0 : body.error) !== null && _body_error !== void 0 ? _body_error : "Inspection failed (".concat(res.status, ")"));
            setInspectResult({
                x,
                y,
                data: body
            });
        } catch (err) {
            if (controller.signal.aborted) return;
            setInspectError(err instanceof Error ? err.message : 'Inspection failed');
        } finally{
            if (abortRef.current === controller) setInspecting(false);
        }
    }
    async function handleSendToAgent() {
        if (!inspectResult) return;
        try {
            const client = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
            await client.writeToSession((0,create/* create */.v)(session_pb/* WriteToSessionRequestSchema */.XSj, {
                sessionId,
                input: formatInspectContext(inspectResult.x, inspectResult.y, inspectResult.data),
                pressEnter: false
            }));
        } catch (err) {
            setInspectError(err instanceof Error ? err.message : 'Failed to send to agent');
        }
    }
    function handleReconnect() {
        setConnectionState('disconnected');
        setViewerKey((k)=>k + 1);
    }
    var _inspectResult_data_computedCss;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: browserTabContainer,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: BrowserTab_css_toolbar,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: inspectButton,
                        "data-testid": "inspect-toggle",
                        "aria-pressed": inspectMode,
                        disabled: !isReady,
                        title: isReady ? 'Pick an element to send to the agent (Esc to exit)' : 'Inspect is available once the browser stream is live',
                        onClick: ()=>inspectMode ? closeInspector() : setInspectMode(true),
                        children: "Inspect"
                    }),
                    inspecting && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        role: "status",
                        children: "Inspecting…"
                    }),
                    inspectError && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        role: "alert",
                        children: inspectError
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: viewerArea,
                children: [
                    isUnavailable && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: placeholderOverlay,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                children: "Browser passthrough unavailable on this host"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                style: {
                                    fontSize: '0.85em',
                                    opacity: 0.7
                                },
                                children: "Requires a running Chrome/Chromium instance with CDP enabled"
                            })
                        ]
                    }),
                    isWaiting && !isUnavailable && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: placeholderOverlay,
                        children: status === types_pb/* VNCStatus */.wL.VNC_STATUS_STARTING ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            children: "Starting virtual display..."
                        }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    children: "No browser open yet"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    style: {
                                        fontSize: '0.85em',
                                        opacity: 0.7
                                    },
                                    children: "Run a browser in this session to see it here"
                                })
                            ]
                        })
                    }),
                    shouldMountViewer && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: canvasWrapper,
                        style: {
                            visibility: isReady ? 'visible' : 'hidden',
                            pointerEvents: isReady ? 'auto' : 'none'
                        },
                        children: [
                            connectionState === 'reconnecting' && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: reconnectingBanner,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        children: "Reconnecting…"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        className: reconnectButton,
                                        onClick: handleReconnect,
                                        children: "Reconnect"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(CDPViewer, {
                                wsUrl: wsUrl,
                                isVisible: isVisible && isReady,
                                onConnected: ()=>setConnectionState('connected'),
                                onDisconnected: ()=>setConnectionState('reconnecting'),
                                inspectMode: inspectMode,
                                onInspectClick: handleInspectClick
                            }, viewerKey)
                        ]
                    }),
                    inspectResult && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: resultPanel,
                        "data-testid": "inspect-result-panel",
                        role: "dialog",
                        "aria-label": "Inspected element",
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: resultHeader,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("strong", {
                                        children: [
                                            "Element at (",
                                            inspectResult.x,
                                            ", ",
                                            inspectResult.y,
                                            ")"
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        type: "button",
                                        className: inspectButton,
                                        onClick: closeInspector,
                                        children: "✕ Close"
                                    })
                                ]
                            }),
                            inspectResult.data.notFound ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                children: "No element found at this position."
                            }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                children: [
                                    inspectResult.data.screenshotBase64 ? // eslint-disable-next-line @next/next/no-img-element
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("img", {
                                        className: resultScreenshot,
                                        alt: "Element screenshot",
                                        src: "data:image/jpeg;base64,".concat(inspectResult.data.screenshotBase64)
                                    }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        children: "Screenshot unavailable"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("pre", {
                                        className: resultCode,
                                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                                            children: inspectResult.data.html
                                        })
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("table", {
                                        className: cssTable,
                                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("tbody", {
                                            children: ((_inspectResult_data_computedCss = inspectResult.data.computedCss) !== null && _inspectResult_data_computedCss !== void 0 ? _inspectResult_data_computedCss : []).map((p)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                                    children: [
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                            children: p.name
                                                        }),
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                            children: p.value
                                                        })
                                                    ]
                                                }, p.name))
                                        })
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        type: "button",
                                        className: inspectButton,
                                        onClick: handleSendToAgent,
                                        children: "Send to Agent"
                                    })
                                ]
                            })
                        ]
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FArtifactsTab.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA6WTTW7DIBCF9zkFm0rNggg7TdqQw1TYHjuowFgDTutUvXsFaWSn-a26QjBi3sd7w-w1K95W2TwI9jlhbAO62QTJMiEe1hPGcAtUG3znvWSqCxjPWlVV2jWSbRU9cv6arWCX026eTWO10r41qpesNvARD-LKK01QBo1OshJNZ12sNKo9aZJP15OvyexAlSWqS4p5UizRIP2u7WECfASujG6iLrgA9EfEdJfrANaPO5whz3fH5PNEXqML3OsdnLAXCTDV339MXwpxz3sCKedrJCtZ17ZApfIQiwZCAOK-VWUyS8zEAmysWEWNdrzAENDeAn9K4Eb7wH3oDUjm0MFR8GJo-rP5X-p5f4ywSAhnfbCDDxWUSGrf_oB43e-xhNzE2U5CJ906VwEZHVuOriyHRGtltekls-gw-n2H9oX3iOkoINrPwfV8nm8Mlroslg8zd-YFI42XpFGo8q0h7Fw1GFwgVUDDvuzIR6EW9eFzXMntlkUXPnreT-OYjQHvje8b3coPMOAEAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FArtifactsTab_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA6WTTW7DIBCF9zkFm0rNggg7TdqQw1TYHjuowFgDTutUvXsFaWSn_a26QjBi3sd7w_w1K95W2TwI9jlhbAO62QTJMiEe1hPGcAtUG3znvWSqCxjPWlVV2jWSbRU9cv6arWCX026eTWO10r41qpesNvARD_LKK01QBo1OshJNZ12sNKo9aZJP15OvyexAlSWqS4p5UizRIP2u7WECfASujG6iLrgA9EfEdJfrANaPO5whz3fH5PNEXqML3OsdnLAXCTDV339MXwpxz3sCKedrJCtZ17ZApfIQiwZCAOK_VWUyS8zEAmysWEWNdrzAENDeAn9K4Eb7wH3oDUjm0MFR8GJo_rP5X_p5f4ywSAhnfbCDDxWUSGrf_oB43e_xhNzE2U5CJ906VwEZHVuOriyHRGtltekls_gw_n2H9oX3iOkoINrPwfV8nm8Mlroslg8zd_YFI42XpFGo8q0h7Fw1GFwgVUDDvuzIR6EW9eFzXMntlkUXPnreT_OYjQHvje8b3coPMOAEAAA_22_7D = __webpack_require__(14205);
;// ./src/components/sessions/ArtifactsTab.css.ts

var ArtifactsTab_css_container = '_1bk913t0';
var ArtifactsTab_css_emptyState = '_1bk913t1';
var ArtifactsTab_css_link = '_1bk913t5';
var ArtifactsTab_css_list = '_1bk913t4';
var ArtifactsTab_css_section = '_1bk913t2';
var ArtifactsTab_css_sectionTitle = '_1bk913t3';
var ArtifactsTab_css_sha = '_1bk913t6';
var shaFull = '_1bk913t7';
var urlToggleButton = '_1bk913t8';
;// ./src/components/sessions/ArtifactsTab.tsx
// +feature: session-artifacts
/* __next_internal_client_entry_do_not_use__ ArtifactsTab auto */ 


function parsePRDisplay(url) {
    const m = url.match(/github\.com\/([\w.-]+\/[\w.-]+)\/pull\/(\d+)/);
    return m ? "".concat(m[1], "#").concat(m[2]) : url;
}
// safeHref ensures only http/https URLs are used as link targets (N-5 fix).
const safeHref = (url)=>/^https?:\/\//i.test(url) ? url : "#";
function ArtifactsTab(param) {
    let { session } = param;
    var _artifacts_prUrls, _artifacts_commitShas, _artifacts_externalUrls, _artifacts_prUrls1, _artifacts_commitShas1, _artifacts_externalUrls1;
    const [urlsExpanded, setUrlsExpanded] = (0,react.useState)(false);
    const artifacts = session.artifacts;
    // Sub-state 1: scan not yet run (server omits the field entirely)
    if (!artifacts) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: ArtifactsTab_css_emptyState,
            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                children: "Extraction pending — will populate automatically once the session starts."
            })
        });
    }
    var _artifacts_prUrls_length, _artifacts_commitShas_length, _artifacts_externalUrls_length;
    const hasContent = ((_artifacts_prUrls_length = (_artifacts_prUrls = artifacts.prUrls) === null || _artifacts_prUrls === void 0 ? void 0 : _artifacts_prUrls.length) !== null && _artifacts_prUrls_length !== void 0 ? _artifacts_prUrls_length : 0) > 0 || ((_artifacts_commitShas_length = (_artifacts_commitShas = artifacts.commitShas) === null || _artifacts_commitShas === void 0 ? void 0 : _artifacts_commitShas.length) !== null && _artifacts_commitShas_length !== void 0 ? _artifacts_commitShas_length : 0) > 0 || ((_artifacts_externalUrls_length = (_artifacts_externalUrls = artifacts.externalUrls) === null || _artifacts_externalUrls === void 0 ? void 0 : _artifacts_externalUrls.length) !== null && _artifacts_externalUrls_length !== void 0 ? _artifacts_externalUrls_length : 0) > 0;
    // Sub-state 2: scan ran but found nothing
    if (!hasContent) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: ArtifactsTab_css_emptyState,
            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                children: "No artifacts found in this session's conversation history."
            })
        });
    }
    var _artifacts_prUrls_length1, _artifacts_commitShas_length1, _artifacts_externalUrls_length1;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: ArtifactsTab_css_container,
        children: [
            ((_artifacts_prUrls_length1 = (_artifacts_prUrls1 = artifacts.prUrls) === null || _artifacts_prUrls1 === void 0 ? void 0 : _artifacts_prUrls1.length) !== null && _artifacts_prUrls_length1 !== void 0 ? _artifacts_prUrls_length1 : 0) > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
                className: ArtifactsTab_css_section,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: ArtifactsTab_css_sectionTitle,
                        children: "Pull Requests"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                        className: ArtifactsTab_css_list,
                        children: artifacts.prUrls.map((url)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("li", {
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                                    href: safeHref(url),
                                    target: "_blank",
                                    rel: "noopener noreferrer",
                                    className: ArtifactsTab_css_link,
                                    children: parsePRDisplay(url)
                                })
                            }, url))
                    })
                ]
            }),
            ((_artifacts_commitShas_length1 = (_artifacts_commitShas1 = artifacts.commitShas) === null || _artifacts_commitShas1 === void 0 ? void 0 : _artifacts_commitShas1.length) !== null && _artifacts_commitShas_length1 !== void 0 ? _artifacts_commitShas_length1 : 0) > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
                className: ArtifactsTab_css_section,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: ArtifactsTab_css_sectionTitle,
                        children: "Commits"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                        className: ArtifactsTab_css_list,
                        children: artifacts.commitShas.map((sha)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                                        className: ArtifactsTab_css_sha,
                                        children: sha.slice(0, 7)
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: shaFull,
                                        children: sha
                                    })
                                ]
                            }, sha))
                    })
                ]
            }),
            ((_artifacts_externalUrls_length1 = (_artifacts_externalUrls1 = artifacts.externalUrls) === null || _artifacts_externalUrls1 === void 0 ? void 0 : _artifacts_externalUrls1.length) !== null && _artifacts_externalUrls_length1 !== void 0 ? _artifacts_externalUrls_length1 : 0) > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
                className: ArtifactsTab_css_section,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: ArtifactsTab_css_sectionTitle,
                        children: "External URLs"
                    }),
                    urlsExpanded ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                                className: ArtifactsTab_css_list,
                                children: artifacts.externalUrls.map((url)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("li", {
                                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                                            href: safeHref(url),
                                            target: "_blank",
                                            rel: "noopener noreferrer",
                                            className: ArtifactsTab_css_link,
                                            children: url.length > 60 ? url.slice(0, 60) + "…" : url
                                        })
                                    }, url))
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: urlToggleButton,
                                onClick: ()=>setUrlsExpanded(false),
                                children: "Hide URLs"
                            })
                        ]
                    }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                        className: urlToggleButton,
                        onClick: ()=>setUrlsExpanded(true),
                        children: [
                            "Show ",
                            artifacts.externalUrls.length,
                            " external URL",
                            artifacts.externalUrls.length !== 1 ? "s" : ""
                        ]
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/components/sessions/SessionSummaryPanel.tsx + 3 modules
var SessionSummaryPanel = __webpack_require__(64164);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FCheckpointList.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61V2Y7bMAx8z1fosQGqwPJ2k13lYxayRcfsypIhyZuj6L8XkuP4yOUWfQps0uRwOJysPhgrUsbWCfm1IESiq5U4clIoOGwXJP5SiRZyj0ZzkhvVVDpEdqLmJNkufi9WXQ0WaxRGe-rwBJx8CfuN0g_2DqfUnl6y5baL7wF3pedknSThXW6UsdN8FtMrYXeoaWa8N9U0JT3FHA8HT70V2hXGVpw0dQ02Fw5CUIH3YKmrRY56x0mySl6hGgFPZwG_CTKNoVpIGYtfwSPJZWbnjwo4QS8U5qP-L7G_QndJ0kZDPz1vq1y6xKe_3RWrD6OmP25vXCjcaYoeKte-pM4L60PoZ-M8FkeaG-1Be04Cp0Az8HuAvtHNFT1gaEJ2EvMzkX_urGm0nH6St3FjJVhqhcTGXS1sM8iJkxNnFMpJWtGKJ-gGW8r6riRZMUcgamhIGi_NF9hI3X2Ib8vRR6__dFsToo7tNaCme5S-PIsg1OCEjdqt_-8ZtvsIUxfK7DkpUcp22_Hs-gAohbVDF0L7Ej3Em4ti3ltRjzBuZogvB-3BPpZV5DBU5-Sqx9sTHsTjqx5Ueh-jRa1QA30C-qL4IL-by7wvILa5j43N0P-6X_GD0WO8EBWq4xUBzfKOd6YT7xQty9EoSov6c2pP89i68pY-dNZ7mtSxQHnWbPc8ZDHeci0saD80gM5N55D2wOXzxroQqw122AYUMxuYCTaugXYg2RyH-d42HVrO2OyHdM8yIMYe6CcZizuLtc5_s948NfBkSvqY3f75ZvfqiTKze0xHs5GQGytaJhstwQayx9MM-LkJQIfp_wBTXpcZ9wgAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FCheckpointList_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61V2Y7bMAx8z1fosQGqwPJ2k13lYxayRcfsypIhyZuj6L8XkuP4yOUWfQps0uRwOJysPhgrUsbWCfm1IESiq5U4clIoOGwXJP5SiRZyj0ZzkhvVVDpEdqLmJNkufi9WXQ0WaxRGe_rwBJx8CfuN0g_2DqfUnl6y5baL7wF3pedknSThXW6UsdN8FtMrYXeoaWa8N9U0JT3FHA8HT70V2hXGVpw0dQ02Fw5CUIH3YKmrRY56x0mySl6hGgFPZwG_CTKNoVpIGYtfwSPJZWbnjwo4QS8U5qP_L7G_QndJ0kZDPz1vq1y6xKe_3RWrD6OmP25vXCjcaYoeKte_pM4L60PoZ_M8FkeaG_1Be04Cp0Az8HuAvtHNFT1gaEJ2EvMzkX_urGm0nH6St3FjJVhqhcTGXS1sM8iJkxNnFMpJWtGKJ_gGW8r6riRZMUcgamhIGi_NF9hI3X2Ib8vRR6_dFsToo7tNaCme5S_PIsg1OCEjdqt_8ZtvsIUxfK7DkpUcp22_Hs_gAohbVDF0L7Ej3Em4ti3ltRjzBuZogvB_3BPpZV5DBU5_Sqx9sTHsTjqx5Ueh_jRa1QA30C_qL4IL_by7wvILa5j43N0P_6X_GD0WO8EBWq4xUBzfKOd6YT7xQty9EoSov6c2pP89i68pY_dNZ7mtSxQHnWbPc8ZDHeci0saD80gM5N55D2wOXzxroQqw122AYUMxuYCTaugXYg2RyH_d42HVrO2OyHdM8yIMYe6CcZizuLtc5_s948NfBkSvqY3f75ZvfqiTKze0xHs5GQGytaJhstwQayx9MM_LkJQIfp_wBTXpcZ9wgAAA_22_7D = __webpack_require__(20734);
;// ./src/components/sessions/CheckpointList.css.ts


var CheckpointList_css_container = '_11f21160';
var deleteButton = '_11f2116a';
var CheckpointList_css_emptyState = '_11f21162';
var CheckpointList_css_header = '_11f21161';
var item = '_11f21164';
var itemInfo = '_11f21165';
var itemLabel = '_11f21166';
var itemMeta = '_11f21167';
var CheckpointList_css_list = '_11f21163';
var pill = '_11f21169';
var showMoreButton = '_11f2116b';
var CheckpointList_css_timestamp = '_11f21168';
;// ./src/components/sessions/RetryHistoryList.tsx
/* __next_internal_client_entry_do_not_use__ RetryHistoryList auto */ 


const MAX_VISIBLE = 10;
function formatRelativeTime(timestamp) {
    if (!timestamp || timestamp.seconds === BigInt(0)) return "Unknown time";
    const now = Date.now();
    const date = new Date(Number(timestamp.seconds) * 1000);
    const seconds = Math.floor((now - date.getTime()) / 1000);
    if (seconds < 60) return "".concat(seconds, "s ago");
    if (seconds < 3600) return "".concat(Math.floor(seconds / 60), "m ago");
    if (seconds < 86400) return "".concat(Math.floor(seconds / 3600), "h ago");
    return "".concat(Math.floor(seconds / 86400), "d ago");
}
/**
 * Retry attempt history for a session (session-retry-backoff, AC5). Clones
 * CheckpointList's shape verbatim (newest-first, MAX_VISIBLE + show-all
 * toggle, formatRelativeTime, empty state).
 */ function RetryHistoryList(param) {
    let { history = [], isLoading = false } = param;
    const [showAll, setShowAll] = (0,react.useState)(false);
    if (isLoading) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: CheckpointList_css_container,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: CheckpointList_css_header,
                    children: "Retry History"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: CheckpointList_css_emptyState,
                    "aria-hidden": "true",
                    children: "Loading retry history…"
                })
            ]
        });
    }
    // Newest first — the proto field is append-ordered (oldest first).
    const sorted = [
        ...history
    ].sort((a, b)=>b.attempt - a.attempt);
    if (sorted.length === 0) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: CheckpointList_css_container,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: CheckpointList_css_header,
                    children: "Retry History"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: CheckpointList_css_emptyState,
                    children: "No retries yet"
                })
            ]
        });
    }
    const visible = showAll ? sorted : sorted.slice(0, MAX_VISIBLE);
    const hiddenCount = sorted.length - MAX_VISIBLE;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: CheckpointList_css_container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: CheckpointList_css_header,
                children: "Retry History"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                className: CheckpointList_css_list,
                "aria-label": "Retry attempt history",
                children: visible.map((rec)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("li", {
                        className: item,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: itemInfo,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    className: itemLabel,
                                    children: [
                                        "Attempt ",
                                        rec.attempt
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: itemMeta,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: CheckpointList_css_timestamp,
                                            children: formatRelativeTime(rec.timestamp)
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: pill,
                                            title: "Failure reason: ".concat(rec.reason),
                                            children: rec.reason
                                        })
                                    ]
                                })
                            ]
                        })
                    }, rec.attempt))
            }),
            !showAll && hiddenCount > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                className: showMoreButton,
                onClick: (e)=>{
                    e.stopPropagation();
                    setShowAll(true);
                },
                type: "button",
                children: [
                    "Show all (",
                    sorted.length,
                    ")"
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/components/ui/ActionBar.tsx + 1 modules
var ActionBar = __webpack_require__(93321);
// EXTERNAL MODULE: ./src/lib/config.ts
var config = __webpack_require__(51202);
// EXTERNAL MODULE: ./src/lib/constants/programs.ts
var programs = __webpack_require__(49051);
// EXTERNAL MODULE: ./src/lib/hooks/useAvailablePrograms.ts
var useAvailablePrograms = __webpack_require__(56230);
// EXTERNAL MODULE: ./src/components/ui/Modal.tsx + 1 modules
var Modal = __webpack_require__(79898);
// EXTERNAL MODULE: ./src/components/sessions/ResumeSessionModal.tsx + 2 modules
var ResumeSessionModal = __webpack_require__(61053);
// EXTERNAL MODULE: ./src/components/sessions/ExportBundleModal.tsx + 3 modules
var ExportBundleModal = __webpack_require__(34892);
// EXTERNAL MODULE: ./src/components/sessions/TagEditor.tsx + 1 modules
var TagEditor = __webpack_require__(95580);
// EXTERNAL MODULE: ./src/components/ui/AppLink.tsx
var AppLink = __webpack_require__(71732);
// EXTERNAL MODULE: ./src/lib/hooks/useBacklogService.ts
var useBacklogService = __webpack_require__(8570);
// EXTERNAL MODULE: ./src/lib/hooks/useWatchBacklogItems.ts
var useWatchBacklogItems = __webpack_require__(10414);
// EXTERNAL MODULE: ./src/lib/store/index.ts
var store = __webpack_require__(93602);
// EXTERNAL MODULE: ./src/lib/store/backlogItemsSlice.ts
var backlogItemsSlice = __webpack_require__(63939);
// EXTERNAL MODULE: ./src/gen/session/v1/backlog_pb.ts
var backlog_pb = __webpack_require__(79870);
// EXTERNAL MODULE: ./src/components/common/InlineNotice.tsx + 1 modules
var InlineNotice = __webpack_require__(96307);
// EXTERNAL MODULE: ./src/components/backlog/ConnectionIndicator.tsx + 1 modules
var ConnectionIndicator = __webpack_require__(52454);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FBacklogItemPanel.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA81X23KbMBB9z1fopTPxTPEA8S3kpZ_Rt4wMC1YsJCJEbNzpv3ckwJa42k7a6UtiJNize3b3aPVjD2UscAo5evX28Lw4eC769YBQLHiqfyAkBWZ5zEUaVD8plvDz0XPdb7MX_QLPcEhkGSBXPf9-QEjy0W_d9ode9eHvh3njhacNRCTPKC4DFFM4qnfUfyciAkJJOAtQyGmRMrWzA5LsZICUX-p5y0UEwqEQq8XsiHJOSYQ-sHh0nFfvGU6-OIF2ZIvDfSJ4waKgtb_W-_wDREz5IUA7EkXAXkxH_X5HMSUJc4iENA9QCEyCUMtvRS5JXDohZxKYNLcSnLXh_ZPGP5BI7gL05GfadEqY01pqgjdfaa9lOIoIS-osVfQEiHEGbQ50pjIsgEm1FXLKRcc17VlYiFztZZw0YcScSScnJ2h_8YRn5_1D7dvK1c5oQHLOKBfInfv5d8MpvWCkVdTJHsmrmaNgp3KoM9UbjTtRBxvb3lPVIpdI3flmvRSQKiuUMDiTrwv7uuIw7C-69ld-Y7-PwJEUUZAShJOrVtPZn7tLZciAW1ZwFI61w2b5uGYHOGWAcCF5b1TDvXkuvVZFVLT3FH69gxlJcWXsok6qEhDgHBxeSCuO1Ve1YltLRugdLvetXTPrf-qc597m3eYm74alStfAQah99dfCeLYxCNOdcj8Rhg76G1sH_UVb89ruljNTTXBEirzD0WpCFcLZ3cUxrIWNMI8fWYcdkaCbGpSGd7jG93B9YWuu1aY_xZ_jLB3m7P1uzqbY2N7DxjVxTtXXTcGsq2D0WoxTQstOVRUzq8rdqsq7JWOd4AYR4fARWEwk7sNM_qAFAysaxjpOYJWTWEcbC4axPH8CzPMm0bSaGnDxMNwU2noSLLGxkvYs0CPkQ6HPuvPIfGFZ311l_eqRwx1AREjCUToRhFzUB3ozePaMkP3joO331ERX6UwHtWARCOWfZY3cetkYOgANm28TzA6r2Uhd6XiMO1WRZSBCnMOVk95eO0VJLp1clhQuaUixSAirxz37qvBJXkqbF3rFpKEBcomFnJg2_lJXpF81qxkjireyR5Tm2Zryz5eIEYnuK5mKsJ0gbF8nrcqnI3kWIH0BNMJj_2d4_uz2ULh9dTF2sq_qaduB5vZTt0i12Ht5qTVev3DlPfX9djVejmjGgAZ-Tnnfp5Q3m1beP-_god52EgAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FBacklogItemPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81X23KbMBB9z1fopTPxTPEA8S3kpZ_Rt4wMC1YsJCJEbNzpv3ckwJa42k7a6UtiJNize3b3aPVjD2UscAo5evX28Lw4eC769YBQLHiqfyAkBWZ5zEUaVD8plvDz0XPdb7MX_QLPcEhkGSBXPf9_QEjy0W_d9ode9eHvh3njhacNRCTPKC4DFFM4qnfUfyciAkJJOAtQyGmRMrWzA5LsZICUX_p5y0UEwqEQq8XsiHJOSYQ_sHh0nFfvGU6_OIF2ZIvDfSJ4waKgtb_W_wDREz5IUA7EkXAXkxH_X5HMSUJc4iENA9QCEyCUMtvRS5JXDohZxKYNLcSnLXh_ZPGP5BI7gL05GfadEqY01pqgjdfaa9lOIoIS_osVfQEiHEGbQ50pjIsgEm1FXLKRcc17VlYiFztZZw0YcScSScnJ2h_8YRn5_1D7dvK1c5oQHLOKBfInfv5d8MpvWCkVdTJHsmrmaNgp3KoM9UbjTtRBxvb3lPVIpdI3flmvRSQKiuUMDiTrwv7uuIw7C_69ld_Y7_PwJEUUZAShJOrVtPZn7tLZciAW1ZwFI61w2b5uGYHOGWAcCF5b1TDvXkuvVZFVLT3FH69gxlJcWXsok6qEhDgHBxeSCuO1Ve1YltLRugdLvetXTPrf_qc597m3eYm74alStfAQah99dfCeLYxCNOdcj8Rhg76G1sH_UVb89ruljNTTXBEirzD0WpCFcLZ3cUxrIWNMI8fWYcdkaCbGpSGd7jG93B9YWuu1aY_xZ_jLB3m7P1uzqbY2N7DxjVxTtXXTcGsq2D0WoxTQstOVRUzq8rdqsq7JWOd4AYR4fARWEwk7sNM_qAFAysaxjpOYJWTWEcbC4axPH8CzPMm0bSaGnDxMNwU2noSLLGxkvYs0CPkQ6HPuvPIfGFZ311l_eqRwx1AREjCUToRhFzUB3ozePaMkP3joO331ERX6UwHtWARCOWfZY3cetkYOgANm28TzA6r2Uhd6XiMO1WRZSBCnMOVk95eO0VJLp1clhQuaUixSAirxz37qvBJXkqbF3rFpKEBcomFnJg2_lJXpF81qxkjireyR5Tm2Zryz5eIEYnuK5mKsJ0gbF8nrcqnI3kWIH0BNMJj_2d4_uz2ULh9dTF2sq_qaduB5vZTt0i12Ht5qTVev3DlPfX9djVejmjGgAZ_Tnnfp5Q3m1beP_god52EgAA_22_7D = __webpack_require__(922);
;// ./src/components/backlog/BacklogItemPanel.css.ts


var actionLink = '_1ke94w1q';
var BacklogItemPanel_css_actions = '_1ke94w1p';
var BacklogItemPanel_css_content = '_1ke94w15';
var criteriaHeader = '_1ke94w1j';
var criteriaList = '_1ke94w1k';
var criteriaSection = '_1ke94w1i';
var criterionDone = '_1ke94w1m';
var criterionPending = '_1ke94w1n';
var criterionRow = '_1ke94w1l';
var criterionText = '_1ke94w1o';
var error = '_1ke94w17';
var BacklogItemPanel_css_header = '_1ke94w18';
var BacklogItemPanel_css_loading = '_1ke94w16';
var BacklogItemPanel_css_panel = '_1ke94w11';
var priorityBadge = '_1ke94w19';
var statusChip = '_1ke94w1a';
var BacklogItemPanel_css_title = '_1ke94w1h';
var BacklogItemPanel_css_toggle = '_1ke94w12';
var toggleIcon = '_1ke94w13';
var toggleLabel = '_1ke94w14';
var verdictBadgeFail = '_1ke94w1e _1ke94w1b';
var verdictBadgePartial = '_1ke94w1d _1ke94w1b';
var verdictBadgePass = '_1ke94w1c _1ke94w1b';
var verdictBadgeUnverifiable = '_1ke94w1f _1ke94w1b';
var verdictSummary = '_1ke94w1g';
;// ./src/components/backlog/BacklogItemPanel.tsx
/* __next_internal_client_entry_do_not_use__ BacklogItemPanel auto */ 
// +feature: backlog:item-panel













// Sweep fix (backlog-event-driven-updates Phase 5 compliance sweep,
// 2026-07-22): Story 5.4.1's own acceptance criterion ("a verdict recorded
// shortly after ... BacklogItemPanel reflects the new status and verdict")
// and ux.md UX AC #14 both require the verdict to surface here — the
// original implementation only ever rendered status/priority/AC criteria,
// never gateVerdict. Mirrors BacklogItemCard.tsx's VERDICT_BADGE_CONFIG;
// PENDING is intentionally left unbadged there too (it just means a review
// is running, not a signal worth a badge).
const VERDICT_BADGE_CONFIG = {
    PASS: {
        label: "✓ PASS",
        className: verdictBadgePass
    },
    PARTIAL: {
        label: "◑ PARTIAL",
        className: verdictBadgePartial
    },
    FAIL: {
        label: "✗ FAIL",
        className: verdictBadgeFail
    },
    UNVERIFIABLE: {
        label: "? UNVERIFIABLE",
        className: verdictBadgeUnverifiable
    }
};
function BacklogItemPanel(param) {
    let { backlogItemId, sessionId } = param;
    const { getBacklogItem } = (0,useBacklogService/* useBacklogService */.gG)();
    const [open, setOpen] = (0,react.useState)(()=>{
        if (false) {}
        return localStorage.getItem("backlog-panel-".concat(sessionId)) === "open";
    });
    const [item, setItem] = (0,react.useState)(null);
    const [loading, setLoading] = (0,react.useState)(false);
    const [loadError, setLoadError] = (0,react.useState)(false);
    const loadItem = (0,react.useCallback)(async ()=>{
        setLoading(true);
        setLoadError(false);
        try {
            const result = await getBacklogItem(backlogItemId);
            setItem(result);
        } catch (err) {
            console.error("Failed to load backlog item:", err);
            setLoadError(true);
        } finally{
            setLoading(false);
        }
    }, [
        getBacklogItem,
        backlogItemId
    ]);
    // Initial load only — Epic 5.4 (Story 5.4.1) replaces the old
    // exponential-backoff poll below with the live subscription instead. A
    // direct fetch is still needed here for the first paint: the shared
    // store's own initial snapshot (useWatchBacklogItems's REST refresh)
    // excludes terminal/archived items, so a linked item that's already
    // "done" or archived when this panel mounts would otherwise never appear.
    (0,react.useEffect)(()=>{
        void loadItem();
    }, [
        loadItem
    ]);
    // Epic 5.4 (Story 5.4.1 / Task 5.4.1b): live updates replace the old poll
    // entirely. Subscribed unfiltered (no status/category filter) so this
    // panel keeps reflecting the linked item's current state the same way
    // BacklogItemDetail does (Task 5.3.1b) — the hook's own return value is
    // unused here; it exists only to keep the shared store hydrated/connected.
    // This panel reads the single item it cares about straight off the store
    // below via selectBacklogItemById, so unrelated item updates elsewhere
    // never cause this panel (or the surrounding SessionDetail it's embedded
    // in) to re-render.
    // Task 6.2.1c: connectionState is captured here (previously discarded) to
    // mount this panel's own ConnectionIndicator. Task 5.4.1a's discovery pass
    // (plan.md) confirmed SessionDetail/SessionDetailView/SessionDetailBar have
    // no existing session-level "Live" indicator to reuse instead, so this
    // panel needs its own (ux.md §4 UX AC #15/#20).
    const { connectionState } = (0,useWatchBacklogItems/* useWatchBacklogItems */.E)();
    const liveRawItem = (0,store/* useAppSelector */.GV)((state)=>(0,backlogItemsSlice/* selectBacklogItemById */.zc)(state, backlogItemId));
    (0,react.useEffect)(()=>{
        if (!liveRawItem) return;
        setItem((0,useBacklogService/* mapBacklogItem */.B5)(liveRawItem));
    }, [
        liveRawItem
    ]);
    // Terminal-state (Task 5.4.1c): set when an ArchivedEvent/RemovedEvent
    // arrives for this item from the separate raw watch below. Mirrors
    // BacklogItemDetail's Task 5.3.1c mechanism exactly — see that file for
    // the full rationale (useWatchBacklogItems.ts intentionally never
    // dispatches itemArchived/removal into the normalized store, and there is
    // no server-side item-id filter on WatchBacklogItemsRequest, so this
    // watches unfiltered and matches events against `backlogItemId`
    // client-side).
    const [terminalState, setTerminalState] = (0,react.useState)(null);
    (0,react.useEffect)(()=>{
        setTerminalState(null);
        const abortController = new AbortController();
        const watchTerminal = async ()=>{
            try {
                const transport = (0,connect_transport/* createConnectTransport */.w)({
                    baseUrl: (0,config/* getApiBaseUrl */.h)(),
                    interceptors: [
                        (0,config/* createAuthInterceptor */._)()
                    ]
                });
                const client = (0,promise_client/* createClient */.UU)(backlog_pb/* BacklogService */.jSM, transport);
                const stream = client.watchBacklogItems({
                    statusFilter: [],
                    categoryFilter: [],
                    afterSeq: 0n
                }, {
                    signal: abortController.signal
                });
                for await (const event of stream){
                    if (event.event.case === "itemArchived" && event.event.value.itemId === backlogItemId) {
                        setTerminalState("archived");
                    } else if (event.event.case === "itemRemoved" && event.event.value.itemId === backlogItemId) {
                        setTerminalState("removed");
                    }
                }
            } catch (err) {
                if (err instanceof Error && err.name === "AbortError") return;
                if (abortController.signal.aborted) return;
                console.error("[BacklogItemPanel] terminal-state watch stream error:", err);
            }
        };
        void watchTerminal();
        return ()=>{
            abortController.abort();
        };
    }, [
        backlogItemId
    ]);
    const toggleOpen = ()=>{
        const next = !open;
        setOpen(next);
        localStorage.setItem("backlog-panel-".concat(sessionId), next ? "open" : "closed");
    };
    const statusIcon = (status)=>{
        if (status === "done") return "✓";
        if (status === "in_progress") return "●";
        return "○";
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: BacklogItemPanel_css_panel,
        "data-testid": "backlog-panel",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                className: BacklogItemPanel_css_toggle,
                onClick: toggleOpen,
                "data-testid": "backlog-panel-toggle",
                "aria-expanded": open,
                "aria-label": open ? "Collapse backlog panel" : "Expand backlog panel",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: toggleIcon,
                        children: open ? "▼" : "▶"
                    }),
                    !open && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: toggleLabel,
                        children: "Task"
                    })
                ]
            }),
            open && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: BacklogItemPanel_css_content,
                children: loading && !item ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: BacklogItemPanel_css_loading,
                    children: "Loading..."
                }) : item ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: BacklogItemPanel_css_header,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    className: priorityBadge,
                                    children: [
                                        "P",
                                        item.priority
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: statusChip,
                                    children: item.status.replace(/_/g, " ")
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)(ConnectionIndicator/* ConnectionIndicator */._, {
                                    connectionState: connectionState
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)(AppLink/* AppLink */.E, {
                            href: "/backlog?item=".concat(item.id),
                            className: BacklogItemPanel_css_title,
                            "data-testid": "backlog-panel-title",
                            children: item.title
                        }),
                        item.gateVerdict && VERDICT_BADGE_CONFIG[item.gateVerdict] && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: BacklogItemPanel_css_header,
                            "data-testid": "backlog-panel-verdict",
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: VERDICT_BADGE_CONFIG[item.gateVerdict].className,
                                    title: "Last review result",
                                    children: VERDICT_BADGE_CONFIG[item.gateVerdict].label
                                }),
                                item.gateVerdictSummary && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: verdictSummary,
                                    children: item.gateVerdictSummary
                                })
                            ]
                        }),
                        item.acCriteria.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: criteriaSection,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: criteriaHeader,
                                    children: "Acceptance Criteria"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                                    className: criteriaList,
                                    children: item.acCriteria.map((c)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                                            className: criterionRow,
                                            "data-testid": "backlog-panel-criterion-".concat(c.index),
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: c.status === "done" ? criterionDone : criterionPending,
                                                    children: statusIcon(c.status)
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: criterionText,
                                                    children: c.text
                                                })
                                            ]
                                        }, c.index))
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: BacklogItemPanel_css_actions,
                            children: terminalState ? /*#__PURE__*/ (0,jsx_runtime.jsx)(InlineNotice/* InlineNotice */.w, {
                                message: terminalState === "archived" ? "This item was archived elsewhere." : "This item was removed elsewhere.",
                                "data-testid": "backlog-panel-terminal-notice"
                            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)(AppLink/* AppLink */.E, {
                                href: "/backlog?item=".concat(item.id),
                                className: actionLink,
                                "data-testid": "backlog-panel-view-full",
                                children: "View full item →"
                            })
                        })
                    ]
                }) : loadError ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: error,
                    children: "Failed to load item"
                }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: error,
                    children: "Item not found"
                })
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/components/shared/BacklogOriginBadge.tsx + 1 modules
var BacklogOriginBadge = __webpack_require__(97443);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FGoalPanel.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61VXXPiIBR976_gcX3ASWJrK_tjOgSuCYrAANboTv97BzROMGqym31M7uWc-33mn_nKZMtNlqE_LwiV2nKwBOWmQU5LwdEXtb8w_sxXcCrsCWa_r17YUi72jty4LN6jj6GcC1X1rFm07qithMJem7sO3y_zNq48xsWFM5IeCVpLaAIAlaJSWHjYOYIYKA82_K5oD7A4RUa2t05bgowWrfPegcUOJDBPkNIKws-1Vh47cYJeYOXsaj-AqGrf86iihxTOY-ePElrUbjqE4AOUW-ExB0-FdHhH7RZsmmbvXRHtTMuQQ0p7U7BFiiSUFArwQN2u3SpMgz5MM9zl1ex5tehgterocaiFB-wMZbFcB0tNks7reS4p21ZW7xXvtVed23uvMoXujOvjoS5MWsG3IUr7hNKNo_Qp5XKIsnxCycZR8pTyfYiyfkIpxlFuUsqPSPl49-OqdvxX0X9oIx8uRVhFBbi-jF8-f0vQaboo7VqWOk5_-xkhmKQ7Q1DRNZS6wdoKUGGywXrBqAx2_QV2LfWBoFpwDirhLAcyoo8z2p1L3ulXe7LaNrTfd07ddb-zzvWVsO5tZXFMW1CSOiQUw_bQhKvFtKVeaEXQXnGwoUDJE_bvOeYD4nAbHY9U_YPbYlzyvZv9iPmD_6Q9yXXNhlWmE8L6-d1HMSiC8uRRFR9dWO-2efG3la4nqUo4EMsxqrKcrCrVKFURU1Wlg7WZKhcdrO3Uo9zBklM15fsHp-45ZB0KAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FGoalPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61VXXPiIBR976_gcX3ASWJrK_tjOgSuCYrAANboTv97BzROMGqym31M7uWc_33mn_nKZMtNlqE_LwiV2nKwBOWmQU5LwdEXtb8w_sxXcCrsCWa_r17YUi72jty4LN6jj6GcC1X1rFm07qithMJem7sO3y_zNq48xsWFM5IeCVpLaAIAlaJSWHjYOYIYKA82_K5oD7A4RUa2t05bgowWrfPegcUOJDBPkNIKws_1Vh47cYJeYOXsaj_AqGrf86iihxTOY_ePElrUbjqE4AOUW_ExB0_FdHhH7RZsmmbvXRHtTMuQQ0p7U7BFiiSUFArwQN2u3SpMgz5MM9zl1ex5tehgterocaiFB_wMZbFcB0tNks7reS4p21ZW7xXvtVed23uvMoXujOvjoS5MWsG3IUr7hNKNo_Qp5XKIsnxCycZR8pTyfYiyfkIpxlFuUsqPSPl49_OqdvxX0X9oIx8uRVhFBbi_jF8_f0vQaboo7VqWOk5_xkhmKQ7Q1DRNZS6wdoKUGGywXrBqAx2_QV2LfWBoFpwDirhLAcyoo8z2p1L3ulXe7LaNrTfd07ddb_zzvWVsO5tZXFMW1CSOiQUw_bQhKvFtKVeaEXQXnGwoUDJE_bvOeYD4nAbHY9U_YPbYlzyvZv9iPmD_6Q9yXXNhlWmE8L6_d1HMSiC8uRRFR9dWO_2efG3la4nqUo4EMsxqrKcrCrVKFURU1Wlg7WZKhcdrO3Uo9zBklM15fsHp_45ZB0KAAA_22_7D = __webpack_require__(68486);
;// ./src/components/sessions/GoalPanel.css.ts


var body = '_19p06j08';
var expandButton = '_19p06j0b';
var goalText = '_19p06j09';
var goalTextClamped = '_19p06j0a';
var panelContainer = '_19p06j00';
var statusChipBase = '_19p06j03';
var statusChipVariants = {idle:'_19p06j04',working:'_19p06j05',blocked:'_19p06j06',done:'_19p06j07'};
var summary = '_19p06j01';
var summaryLabel = '_19p06j02';
var taskChildren = '_19p06j0g';
var taskFraction = '_19p06j0c';
var taskItem = '_19p06j0e';
var taskList = '_19p06j0d';
var taskStatusChipBase = '_19p06j0h';
var taskStatusVariants = {pending:'_19p06j0i',in_progress:'_19p06j0j',done:'_19p06j0k',blocked:'_19p06j0l'};
var taskTitle = '_19p06j0f';
;// ./src/components/sessions/GoalPanel.tsx
/* __next_internal_client_entry_do_not_use__ parseTasksJson,GoalPanel auto */ 
// +feature: session-goal-panel


/**
 * Parses a JSON string into a TaskNode array. Returns [] on any parse failure.
 * Exported for direct unit testing.
 */ function parseTasksJson(json) {
    if (!json) return [];
    try {
        const parsed = JSON.parse(json);
        if (!Array.isArray(parsed)) return [];
        return parsed;
    } catch (e) {
        return [];
    }
}
/** Goal status label map */ const STATUS_LABELS = {
    idle: "Idle",
    working: "Working",
    blocked: "Blocked",
    done: "Done"
};
/** Task status label map */ const TASK_STATUS_LABELS = {
    pending: "Pending",
    in_progress: "In Progress",
    done: "Done",
    blocked: "Blocked"
};
const MAX_GOAL_DISPLAY_LENGTH = 120;
/** Single task node rendered with status badge and optional children. */ function TaskTreeNode(param) {
    let { node, depth } = param;
    if (depth > 3) return null; // guard against malformed data
    var _taskStatusVariants_node_status;
    const statusClass = (_taskStatusVariants_node_status = taskStatusVariants[node.status]) !== null && _taskStatusVariants_node_status !== void 0 ? _taskStatusVariants_node_status : taskStatusVariants["pending"];
    var _TASK_STATUS_LABELS_node_status;
    const statusLabel = (_TASK_STATUS_LABELS_node_status = TASK_STATUS_LABELS[node.status]) !== null && _TASK_STATUS_LABELS_node_status !== void 0 ? _TASK_STATUS_LABELS_node_status : node.status;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
        className: taskItem,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: "".concat(taskStatusChipBase, " ").concat(statusClass),
                "aria-label": "Status: ".concat(statusLabel),
                children: statusLabel
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: taskTitle,
                children: node.title
            }),
            node.children && node.children.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                className: taskChildren,
                role: "list",
                children: node.children.map((child)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(TaskTreeNode, {
                        node: child,
                        depth: depth + 1
                    }, child.id))
            })
        ]
    });
}
/**
 * GoalPanel renders the session goal and task tree in the session detail view.
 * Returns null when no goal is set.
 */ function GoalPanel(param) {
    let { goal } = param;
    const [expanded, setExpanded] = (0,react.useState)(false);
    const tasks = (0,react.useMemo)(()=>parseTasksJson(goal === null || goal === void 0 ? void 0 : goal.tasksJson), [
        goal === null || goal === void 0 ? void 0 : goal.tasksJson
    ]);
    if (!goal || !goal.goalText) return null;
    const isLong = goal.goalText.length > MAX_GOAL_DISPLAY_LENGTH;
    const displayText = !isLong || expanded ? goal.goalText : goal.goalText.slice(0, MAX_GOAL_DISPLAY_LENGTH) + "…";
    var _statusChipVariants_goal_status;
    const chipClass = (_statusChipVariants_goal_status = statusChipVariants[goal.status]) !== null && _statusChipVariants_goal_status !== void 0 ? _statusChipVariants_goal_status : statusChipVariants["idle"];
    var _STATUS_LABELS_goal_status;
    const statusLabel = (_STATUS_LABELS_goal_status = STATUS_LABELS[goal.status]) !== null && _STATUS_LABELS_goal_status !== void 0 ? _STATUS_LABELS_goal_status : goal.status;
    var _goal_tasksTotal;
    const hasFraction = ((_goal_tasksTotal = goal.tasksTotal) !== null && _goal_tasksTotal !== void 0 ? _goal_tasksTotal : 0) > 0;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("details", {
        className: panelContainer,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("summary", {
                className: summary,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: "".concat(statusChipBase, " ").concat(chipClass),
                        "aria-label": "Goal status: ".concat(statusLabel),
                        children: statusLabel
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: summaryLabel,
                        children: "Goal & Tasks"
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: body,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                        className: "".concat(goalText, " ").concat(!expanded && isLong ? goalTextClamped : ""),
                        children: [
                            displayText,
                            isLong && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: expandButton,
                                onClick: ()=>setExpanded((v)=>!v),
                                "aria-label": expanded ? "Show less goal text" : "Show full goal text",
                                children: expanded ? "less" : "more"
                            })
                        ]
                    }),
                    tasks.length > 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                        className: taskList,
                        role: "list",
                        "aria-label": "Task list",
                        children: tasks.map((t)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(TaskTreeNode, {
                                node: t,
                                depth: 1
                            }, t.id))
                    }) : hasFraction ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                        className: taskFraction,
                        "aria-label": "Task progress",
                        children: [
                            goal.tasksDone,
                            "/",
                            goal.tasksTotal,
                            " done"
                        ]
                    }) : null
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-markdown@10.1.0+5bba37e96b694e48/node_modules/react-markdown/lib/index.js + 114 modules
var lib = __webpack_require__(55206);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/remark-gfm@4.0.1+759ce506b1ed1a42/node_modules/remark-gfm/lib/index.js + 56 modules
var remark_gfm_lib = __webpack_require__(69896);
// EXTERNAL MODULE: ./src/components/backlog/markdownBody.css.ts
var markdownBody_css = __webpack_require__(14613);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FNotePanel.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA81WTXPjIAy951dw2ZnNgYztpmlCf0wHg-KwweAB3MTZ6X_v-KOJsZ3a9eawV_SEpCc9weptVzAdpgH6u0Ao1oaDISjMzshqKTh6p-Y3xm_hDi6RucDy9YrChnKRW9KBPL1UmIxyLlTSswaVNaUmEQo7nQ0CPharJq2wSosLm0laELSXcC79qRSJwsJBaglioByY8jihvfuiSxWQ5cZqQ1CmxRc4t2CwBQnMEaS0gvJwr5XDVlygl1e8vNpPIJKD6yGSCiGFddi6QsLtVqZlGdzHdwolBJ8gPgqHOTgqpMUpNUcwPgH1jTe3qDLfp7Mq_wZ_quBjRQ6mG7UbF2vndDoSbD0_WFrPGWXHxOhc8RuVj5jQzXcTGhXLySOUCoUPzTCs19nZq54c9HvTvnYh_t1bj7HnCn0S3B0ICoPgVzdIGAXZefKc7mkqZNErML_Pexj3iO8A6HJKF47_3IWacwNNjWCcYFR6ZJG9ZrmtKNO5k0JBd0zwYJHS43wzMqV0oiSGxFd4kV6Gd9n9pTVZ1tv5SnseaXjq9btD72P01bwJk_RVnzUK6UpuO0lyyqeOcGFpLIHXg5RRJlxBULDatJNS2mEqpT4Bb3vv5hMf_E8r7mEt2M3YetTXhVCllPHIU_8nt07sC8y0cqBc2zRLy-MvzvUx75N05TmYJeQfs0xbLDs4l18Gpg11QiuCcsXBlBS2PeL5Wy4cXXP-MmKTNPHdR-owppo6l4E8UPC6-PgE_GPqOtMKAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FNotePanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81WTXPjIAy951dw2ZnNgYztpmlCf0wHg_KwweAB3MTZ6X_v_KOJsZ3a9eawV_SEpCc9weptVzAdpgH6u0Ao1oaDISjMzshqKTh6p_Y3xm_hDi6RucDy9YrChnKRW9KBPL1UmIxyLlTSswaVNaUmEQo7nQ0CPharJq2wSosLm0laELSXcC79qRSJwsJBaglioByY8jihvfuiSxWQ5cZqQ1CmxRc4t2CwBQnMEaS0gvJwr5XDVlygl1e8vNpPIJKD6yGSCiGFddi6QsLtVqZlGdzHdwolBJ8gPgqHOTgqpMUpNUcwPgH1jTe3qDLfp7Mq_wZ_quBjRQ6mG7UbF2vndDoSbD0_WFrPGWXHxOhc8RuVj5jQzXcTGhXLySOUCoUPzTCs19nZq54c9HvTvnYh_t1bj7HnCn0S3B0ICoPgVzdIGAXZefKc7mkqZNErML_Pexj3iO8A6HJKF47_3IWacwNNjWCcYFR6ZJG9ZrmtKNO5k0JBd0zwYJHS43wzMqV0oiSGxFd4kV6Gd9n9pTVZ1tv5SnseaXjq9btD72P01bwJk_RVnzUK6UpuO0lyyqeOcGFpLIHXg5RRJlxBULDatJNS2mEqpT4Bb3vv5hMf_E8r7mEt2M3YetTXhVCllPHIU_8nt07sC8y0cqBc2zRLy_MvzvUx75N05TmYJeQfs0xbLDs4l18Gpg11QiuCcsXBlBS2PeL5Wy4cXXP_MmKTNPHdR_owppo6l4E8UPC6_PgE_GPqOtMKAAA_22_7D = __webpack_require__(49337);
;// ./src/components/sessions/NotePanel.css.ts


var actionsRow = '_9yco1m7';
var addButton = '_9yco1m4';
var NotePanel_css_body = '_9yco1m2';
var NotePanel_css_cancelButton = '_9yco1m9';
var editButton = '_9yco1ma';
var emptyText = '_9yco1m3';
var NotePanel_css_errorText = '_9yco1mb';
var hint = '_9yco1m6';
var NotePanel_css_panelContainer = '_9yco1m0';
var renderedHeading = '_9yco1mc';
var saveButton = '_9yco1m8';
var NotePanel_css_summary = '_9yco1m1';
var NotePanel_css_textarea = '_9yco1m5';
;// ./src/components/sessions/NotePanel.tsx
/* __next_internal_client_entry_do_not_use__ NotePanel auto */ 
// +feature: session-notes-panel





// Cross-reference: matches session.MaxNoteLength (session/instance.go) and the ent
// schema's field.Text("note").MaxLen(10000) (session/ent/schema/session.go) — kept in
// sync manually across all three; see plan.md's Unresolved Question 1.
const NOTE_MAX_LENGTH = 10000;
const noteEncoder = new TextEncoder();
// A user-typed `# Heading` must never become a real page-level heading tag — even
// a remapped one (e.g. h1->h5) still creates a heading-order skip against this
// page's actual h2 session title, tripping the repo's axe/Lighthouse heading-order
// CI gate. Render every markdown heading level as a styled non-heading element instead.
function RenderedHeading(param) {
    let { children } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: renderedHeading,
        children: children
    });
}
const headingComponents = {
    h1: RenderedHeading,
    h2: RenderedHeading,
    h3: RenderedHeading,
    h4: RenderedHeading,
    h5: RenderedHeading,
    h6: RenderedHeading
};
/**
 * NotePanel renders a session's free-form markdown note, with an edit/read-mode
 * toggle mirroring GoalPanel's file structure. Explicit Save/Cancel — no
 * autosave-on-blur convention exists elsewhere in this codebase.
 */ function NotePanel(param) {
    let { note, onSave } = param;
    const [isEditing, setIsEditing] = (0,react.useState)(false);
    const [draftValue, setDraftValue] = (0,react.useState)(note);
    const [saving, setSaving] = (0,react.useState)(false);
    const [saveError, setSaveError] = (0,react.useState)(null);
    // Restores focus to the Edit/Add-note button on exiting edit mode (Save or
    // Cancel), per design/ux.md Surface 3 AC11 — focus must never fall back to
    // <body>. Same ref serves both buttons since only one renders at a time.
    const editButtonRef = (0,react.useRef)(null);
    // Only re-sync local state from the note prop when not editing, so a
    // stream-driven update from another tab doesn't clobber an in-progress edit —
    // mirrors the guard at SessionDetailView.tsx's isEditingCategory sync.
    (0,react.useEffect)(()=>{
        if (!isEditing) {
            setDraftValue(note);
        }
    }, [
        note,
        isEditing
    ]);
    // Restore focus to the Edit/Add-note button after exiting edit mode (Save or
    // Cancel) — design/ux.md Surface 3 AC11. Runs after render so the button has
    // already remounted; wasEditingRef guards against firing on initial mount.
    const wasEditingRef = (0,react.useRef)(false);
    (0,react.useEffect)(()=>{
        if (wasEditingRef.current && !isEditing) {
            var _editButtonRef_current;
            (_editButtonRef_current = editButtonRef.current) === null || _editButtonRef_current === void 0 ? void 0 : _editButtonRef_current.focus();
        }
        wasEditingRef.current = isEditing;
    }, [
        isEditing
    ]);
    const startEditing = ()=>{
        setDraftValue(note);
        setSaveError(null);
        setIsEditing(true);
    };
    const cancel = ()=>{
        setDraftValue(note);
        setSaveError(null);
        setIsEditing(false);
    };
    // The backend caps by UTF-8 byte length (session.MaxNoteLength), but the
    // textarea's native maxLength counts UTF-16 code units — a multi-byte-heavy
    // note (CJK, emoji) can pass the char cap yet still fail server-side. Track
    // bytes so the hint and the save guard below are both byte-accurate.
    const noteByteLength = noteEncoder.encode(draftValue).length;
    const save = async ()=>{
        if (noteByteLength > NOTE_MAX_LENGTH) {
            setSaveError("Note is too long (".concat(noteByteLength, "/").concat(NOTE_MAX_LENGTH, " bytes) — some characters take more than one byte"));
            return;
        }
        setSaving(true);
        setSaveError(null);
        try {
            await onSave(draftValue);
            setIsEditing(false);
        } catch (err) {
            setSaveError(err instanceof Error ? err.message : "Failed to save note");
        } finally{
            setSaving(false);
        }
    };
    const hasNote = note.trim().length > 0;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("details", {
        open: true,
        className: NotePanel_css_panelContainer,
        "data-testid": "session-note-panel",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("summary", {
                className: NotePanel_css_summary,
                children: "Notes"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: NotePanel_css_body,
                children: isEditing ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("textarea", {
                            className: NotePanel_css_textarea,
                            value: draftValue,
                            onChange: (e)=>setDraftValue(e.target.value),
                            maxLength: NOTE_MAX_LENGTH,
                            autoFocus: true,
                            "aria-label": "Session note (markdown)",
                            "aria-describedby": saveError ? "session-note-hint session-note-error" : "session-note-hint",
                            "data-testid": "session-note-textarea"
                        }),
                        saveError && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                            id: "session-note-error",
                            className: NotePanel_css_errorText,
                            role: "alert",
                            "aria-live": "assertive",
                            children: saveError
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                            id: "session-note-hint",
                            className: hint,
                            children: [
                                "Markdown supported. ",
                                noteByteLength,
                                "/",
                                NOTE_MAX_LENGTH
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: actionsRow,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: saveButton,
                                    onClick: save,
                                    disabled: saving,
                                    "data-testid": "session-note-save-button",
                                    children: "Save"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: NotePanel_css_cancelButton,
                                    onClick: cancel,
                                    disabled: saving,
                                    children: "Cancel"
                                })
                            ]
                        })
                    ]
                }) : hasNote ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: markdownBody_css/* markdownBody */.e,
                            "data-testid": "session-note-rendered",
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(lib/* Markdown */.oz, {
                                remarkPlugins: [
                                    remark_gfm_lib/* default */.A
                                ],
                                components: headingComponents,
                                children: note
                            })
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            ref: editButtonRef,
                            className: editButton,
                            onClick: startEditing,
                            children: "Edit"
                        })
                    ]
                }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                            className: emptyText,
                            children: "No notes yet — leave yourself a reminder about this session."
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            ref: editButtonRef,
                            className: addButton,
                            onClick: startEditing,
                            children: "Add note"
                        })
                    ]
                })
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/store/sessionsSlice.ts
var sessionsSlice = __webpack_require__(60795);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FWorkspacePeersPanel.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA71V23KbMBB9z1fosX5QxihuYuOPyQi0hm1BUqUl2GT67x2EQ_CFSy_TJ0bsYc_Zs7vosdmkP3KzZu8PjCXGKXAxi-yReVOgYm_SfeH8NdpBI1wDq32P4k4qrHx8BXl6CRgrlUKd3UTXIVpKl6HmZOxdwM-Hx05VFFQp9LaQp5gdCji2n8sCM82RoPQxS0ETuPb1t8oTHk48NZpAU8y8lSnwBKgG0C0ikzeEohkqSgyRKe9ielEiiDoYTdxjAzcVJCFhiNeAWU43iCwgUlMYdx27KP_p78r_DNWoKI-Z2NiQID_L-jiXqPkVpO_fej-YC200hLNMv2fOVFrFjJzU3koHmkarEl3BlfNtzBr8EDbuolwwas8XZsW5eQMXLBu19lL6ZXw7zLa5b3375AodpIRGxy1RVc6MVoGeuKdTAZ8GduN2dnfodS_g678QcJraxbO8eYsXm_b8W_M65lif7uW_Ldp2hkmO5xHDPLs_zxMN88hLI1EXqIHP-Nm3Wdgj23ZrPN3c3WrJDk4ZnAdEnSMBD3_bdsRrJ-2gmKS7WkZHSLgJd_1qv-BeEjR0L50jTCYI02WEakioZgijaKLvYhFhFOb1F4e1L2atBwAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FWorkspacePeersPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA71V23KbMBB9z1fosX5QxihuYuOPyQi0hm1BUqUl2GT67x2EQ_CFSy_TJ0bsYc_Zs7vosdmkP3KzZu8PjCXGKXAxi_yReVOgYm_SfeH8NdpBI1wDq32P4k4qrHx8BXl6CRgrlUKd3UTXIVpKl6HmZOxdwM_Hx05VFFQp9LaQp5gdCji2n8sCM82RoPQxS0ETuPb1t8oTHk48NZpAU8y8lSnwBKgG0C0ikzeEohkqSgyRKe9ielEiiDoYTdxjAzcVJCFhiNeAWU43iCwgUlMYdx27KP_p78r_DNWoKI_Z2NiQID_L_jiXqPkVpO_fej_YC200hLNMv2fOVFrFjJzU3koHmkarEl3BlfNtzBr8EDbuolwwas8XZsW5eQMXLBu19lL6ZXw7zLa5b3375AodpIRGxy1RVc6MVoGeuKdTAZ8GduN2dnfodS_g678QcJraxbO8eYsXm_b8W_M65lif7uW_Ldp2hkmO5xHDPLs_zxMN88hLI1EXqIHP_Nm3Wdgj23ZrPN3c3WrJDk4ZnAdEnSMBD3_bdsRrJ_2gmKS7WkZHSLgJd_1qv_BeEjR0L50jTCYI02WEakioZgijaKLvYhFhFOb1F4e1L2atBwAA_22_7D = __webpack_require__(58598);
;// ./src/components/sessions/WorkspacePeersPanel.css.ts


var dismissButton = 'z4cqho3';
var WorkspacePeersPanel_css_heading = 'z4cqho2';
var headingRow = 'z4cqho1';
var lifecycleChipBase = 'z4cqhoa';
var lifecycleChipVariants = {active:'z4cqhob',stuck:'z4cqhoc',gone:'z4cqhod'};
var WorkspacePeersPanel_css_panelContainer = 'z4cqho0';
var peerGoal = 'z4cqho9';
var peerItem = 'z4cqho5';
var peerList = 'z4cqho4';
var peerMeta = 'z4cqho8';
var peerTitle = 'z4cqho7';
var peerTitleRow = 'z4cqho6';
;// ./src/components/sessions/WorkspacePeersPanel.tsx
/* __next_internal_client_entry_do_not_use__ GOAL_STALE_THRESHOLD_MS,peerLifecycle,WorkspacePeersPanel auto */ 
// +feature: workspace-peers-panel





// Mirrors session.goalStaleThreshold (session/workspace_peers.go) — a goal is
// considered stale (peer "stuck") after this long without an update. Exported for direct
// unit testing of the boundary in peerLifecycle.
const GOAL_STALE_THRESHOLD_MS = 30 * 60 * 1000;
/**
 * Derives a peer's lifecycle from two independent signals: whether the session process
 * is confirmed dead (status === STOPPED, ponytail: a proxy for tmux liveness — this repo's
 * orphan-sweep reconciliation keeps Status roughly in sync with reality; a fully
 * authoritative check would need a server round-trip via list_workspace_peers) and whether
 * its goal hasn't been touched in a while. Exported for direct unit testing.
 */ function peerLifecycle(peer, nowMs) {
    var _peer_goal;
    if (peer.status === types_pb/* SessionStatus */.Ph.STOPPED || peer.status === types_pb/* SessionStatus */.Ph.PERMANENTLY_FAILED) return "gone";
    const updatedAt = (_peer_goal = peer.goal) === null || _peer_goal === void 0 ? void 0 : _peer_goal.updatedAt;
    if (updatedAt) {
        const updatedMs = Number(updatedAt.seconds) * 1000;
        if (nowMs - updatedMs > GOAL_STALE_THRESHOLD_MS) return "stuck";
    }
    return "active";
}
const LIFECYCLE_LABELS = {
    active: "Active",
    stuck: "Stuck",
    gone: "Gone"
};
// ponytail: per-session dismiss, mirrors BacklogItemPanel's
// `backlog-panel-${sessionId}` localStorage key.
const dismissedKey = (sessionId)=>"workspace-peers-dismissed-".concat(sessionId);
/**
 * WorkspacePeersPanel lists other active sessions in this exact working directory
 * (session.activeDir), live-updated via the existing WatchSessions Redux store — no
 * extra polling or RPC needed. Scoped to the literal active directory, not workspaceKey
 * (which also matches sibling worktrees/branches of the same repo) — a peer editing a
 * different worktree isn't touching this directory's files. activeDir is populated
 * regardless of session state, so stopped sessions with an already-cleaned-up worktree
 * still compare correctly instead of collapsing onto the shared repo root. Renders
 * nothing when the session has no active dir, no peers, or the user dismissed it.
 */ function WorkspacePeersPanel(param) {
    let { session, now } = param;
    const allSessions = (0,store/* useAppSelector */.GV)(sessionsSlice/* selectAllSessions */.rt);
    const nowMs = now !== null && now !== void 0 ? now : Date.now();
    const [dismissed, setDismissed] = (0,react.useState)(()=>{
        if (false) {}
        return localStorage.getItem(dismissedKey(session.id)) === "1";
    });
    const peers = (0,react.useMemo)(()=>{
        if (!session.activeDir) return [];
        return allSessions.filter((s)=>s.id !== session.id && s.activeDir === session.activeDir);
    }, [
        allSessions,
        session.activeDir,
        session.id
    ]);
    if (!session.activeDir || peers.length === 0 || dismissed) return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: WorkspacePeersPanel_css_panelContainer,
        "data-testid": "workspace-peers-panel",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: headingRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: WorkspacePeersPanel_css_heading,
                        children: "Other Sessions in This Directory"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: dismissButton,
                        "aria-label": "Dismiss",
                        "data-testid": "workspace-peers-dismiss",
                        onClick: ()=>{
                            localStorage.setItem(dismissedKey(session.id), "1");
                            setDismissed(true);
                        },
                        children: "✕"
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                className: peerList,
                role: "list",
                children: peers.map((peer)=>{
                    var _peer_goal;
                    const lifecycle = peerLifecycle(peer, nowMs);
                    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                        className: peerItem,
                        "data-testid": "workspace-peer-item",
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: peerTitleRow,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: peerTitle,
                                        children: peer.title
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: "".concat(lifecycleChipBase, " ").concat(lifecycleChipVariants[lifecycle]),
                                        "aria-label": "Peer status: ".concat(LIFECYCLE_LABELS[lifecycle]),
                                        children: LIFECYCLE_LABELS[lifecycle]
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: peerMeta,
                                children: peer.branch || peer.existingDir
                            }),
                            ((_peer_goal = peer.goal) === null || _peer_goal === void 0 ? void 0 : _peer_goal.goalText) && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: peerGoal,
                                children: peer.goal.goalText
                            })
                        ]
                    }, peer.id);
                })
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/codegenv2/file.js
var file = __webpack_require__(1844);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/codegenv2/service.js
var service = __webpack_require__(95765);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/gen/google/protobuf/timestamp_pb.js
var timestamp_pb = __webpack_require__(43963);
;// ./src/gen/session/v1/handoff_summary_pb.ts
// @generated by protoc-gen-es v2.11.0 with parameter "target=ts,ts_nocheck=false,keep_empty_files=true"
// @generated from file session/v1/handoff_summary.proto (package session.v1, syntax proto3)
/* eslint-disable */ 

/**
 * Describes the file session/v1/handoff_summary.proto.
 */ const file_session_v1_handoff_summary = /*@__PURE__*/ (0,file/* fileDesc */.w)("CiBzZXNzaW9uL3YxL2hhbmRvZmZfc3VtbWFyeS5wcm90bxIKc2Vzc2lvbi52MSLaAgoTSGFuZG9mZlN1bW1hcnlQcm90bxISCgpzZXNzaW9uX2lkGAEgASgJEhUKDXNlc3Npb25fdGl0bGUYAiABKAkSMAoGc3RhdHVzGAMgASgOMiAuc2Vzc2lvbi52MS5IYW5kb2ZmU3VtbWFyeVN0YXR1cxITCgthY3RpdmVfdGFzaxgEIAEoCRIUCgxzdW1tYXJ5X3RleHQYBSABKAkSIgoabWlkZGxlX21lc3NhZ2VzX3N1bW1hcml6ZWQYBiABKAUSFQoNZXJyb3JfbWVzc2FnZRgHIAEoCRITCgtlcnJvcl9zdGFnZRgIIAEoCRIwCgxnZW5lcmF0ZWRfYXQYCSABKAsyGi5nb29nbGUucHJvdG9idWYuVGltZXN0YW1wEjkKFWdlbmVyYXRpb25fc3RhcnRlZF9hdBgKIAEoCzIaLmdvb2dsZS5wcm90b2J1Zi5UaW1lc3RhbXAiLgoYR2V0SGFuZG9mZlN1bW1hcnlSZXF1ZXN0EhIKCnNlc3Npb25faWQYASABKAkiXgoZR2V0SGFuZG9mZlN1bW1hcnlSZXNwb25zZRI1CgdzdW1tYXJ5GAEgASgLMh8uc2Vzc2lvbi52MS5IYW5kb2ZmU3VtbWFyeVByb3RvSACIAQFCCgoIX3N1bW1hcnkiMgocVHJpZ2dlckhhbmRvZmZTdW1tYXJ5UmVxdWVzdBISCgpzZXNzaW9uX2lkGAEgASgJIlEKHVRyaWdnZXJIYW5kb2ZmU3VtbWFyeVJlc3BvbnNlEjAKB3N1bW1hcnkYASABKAsyHy5zZXNzaW9uLnYxLkhhbmRvZmZTdW1tYXJ5UHJvdG8qzQEKFEhhbmRvZmZTdW1tYXJ5U3RhdHVzEiYKIkhBTkRPRkZfU1VNTUFSWV9TVEFUVVNfVU5TUEVDSUZJRUQQABIiCh5IQU5ET0ZGX1NVTU1BUllfU1RBVFVTX1BFTkRJTkcQARIlCiFIQU5ET0ZGX1NVTU1BUllfU1RBVFVTX0dFTkVSQVRJTkcQAhIgChxIQU5ET0ZGX1NVTU1BUllfU1RBVFVTX1JFQURZEAMSIAocSEFORE9GRl9TVU1NQVJZX1NUQVRVU19FUlJPUhAEMusBChVIYW5kb2ZmU3VtbWFyeVNlcnZpY2USYgoRR2V0SGFuZG9mZlN1bW1hcnkSJC5zZXNzaW9uLnYxLkdldEhhbmRvZmZTdW1tYXJ5UmVxdWVzdBolLnNlc3Npb24udjEuR2V0SGFuZG9mZlN1bW1hcnlSZXNwb25zZSIAEm4KFVRyaWdnZXJIYW5kb2ZmU3VtbWFyeRIoLnNlc3Npb24udjEuVHJpZ2dlckhhbmRvZmZTdW1tYXJ5UmVxdWVzdBopLnNlc3Npb24udjEuVHJpZ2dlckhhbmRvZmZTdW1tYXJ5UmVzcG9uc2UiAEKzAQoOY29tLnNlc3Npb24udjFCE0hhbmRvZmZTdW1tYXJ5UHJvdG9QAVpDZ2l0aHViLmNvbS90c3RhcGxlci9zdGFwbGVyLXNxdWFkL2dlbi9wcm90by9nby9zZXNzaW9uL3YxO3Nlc3Npb252MaICA1NYWKoCClNlc3Npb24uVjHKAgpTZXNzaW9uXFYx4gIWU2Vzc2lvblxWMVxHUEJNZXRhZGF0YeoCC1Nlc3Npb246OlYxYgZwcm90bzM", [
    timestamp_pb/* file_google_protobuf_timestamp */.i
]);
/**
 * Describes the message session.v1.HandoffSummaryProto.
 * Use `create(HandoffSummaryProtoSchema)` to create a new message.
 */ const HandoffSummaryProtoSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_handoff_summary, 0)));
/**
 * Describes the message session.v1.GetHandoffSummaryRequest.
 * Use `create(GetHandoffSummaryRequestSchema)` to create a new message.
 */ const GetHandoffSummaryRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_handoff_summary, 1)));
/**
 * Describes the message session.v1.GetHandoffSummaryResponse.
 * Use `create(GetHandoffSummaryResponseSchema)` to create a new message.
 */ const GetHandoffSummaryResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_handoff_summary, 2)));
/**
 * Describes the message session.v1.TriggerHandoffSummaryRequest.
 * Use `create(TriggerHandoffSummaryRequestSchema)` to create a new message.
 */ const TriggerHandoffSummaryRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_handoff_summary, 3)));
/**
 * Describes the message session.v1.TriggerHandoffSummaryResponse.
 * Use `create(TriggerHandoffSummaryResponseSchema)` to create a new message.
 */ const TriggerHandoffSummaryResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_handoff_summary, 4)));
/**
 * HandoffSummaryStatus indicates the generation state of a session handoff summary.
 *
 * @generated from enum session.v1.HandoffSummaryStatus
 */ var HandoffSummaryStatus = /*#__PURE__*/ function(HandoffSummaryStatus) {
    /**
   * @generated from enum value: HANDOFF_SUMMARY_STATUS_UNSPECIFIED = 0;
   */ HandoffSummaryStatus[HandoffSummaryStatus["UNSPECIFIED"] = 0] = "UNSPECIFIED";
    /**
   * @generated from enum value: HANDOFF_SUMMARY_STATUS_PENDING = 1;
   */ HandoffSummaryStatus[HandoffSummaryStatus["PENDING"] = 1] = "PENDING";
    /**
   * @generated from enum value: HANDOFF_SUMMARY_STATUS_GENERATING = 2;
   */ HandoffSummaryStatus[HandoffSummaryStatus["GENERATING"] = 2] = "GENERATING";
    /**
   * @generated from enum value: HANDOFF_SUMMARY_STATUS_READY = 3;
   */ HandoffSummaryStatus[HandoffSummaryStatus["READY"] = 3] = "READY";
    /**
   * @generated from enum value: HANDOFF_SUMMARY_STATUS_ERROR = 4;
   */ HandoffSummaryStatus[HandoffSummaryStatus["ERROR"] = 4] = "ERROR";
    return HandoffSummaryStatus;
}({});
/**
 * Describes the enum session.v1.HandoffSummaryStatus.
 */ const HandoffSummaryStatusSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_session_v1_handoff_summary, 0)));
/**
 * HandoffSummaryService provides read and regenerate access to restart-handoff
 * summaries: a condensed briefing generated for a session so a new session can
 * be seeded from it (Restart-with-Summary), independent of SessionService's
 * live-instance machinery.
 *
 * @generated from service session.v1.HandoffSummaryService
 */ const HandoffSummaryService = /*@__PURE__*/ (0,service/* serviceDesc */.Z)(file_session_v1_handoff_summary, 0);

// EXTERNAL MODULE: ./src/lib/hooks/useAbortableRequest.ts
var useAbortableRequest = __webpack_require__(2663);
;// ./src/lib/hooks/useHandoffSummary.ts
/* __next_internal_client_entry_do_not_use__ isGenerating,useHandoffSummary auto */ 





const POLL_INTERVAL_MS = 2000;
/**
 * Cap on consecutive `nil` GetHandoffSummary reads before giving up
 * (~10 * 2s = 20s). Mirrors useSessionSummary's MAX_NULL_POLL_ATTEMPTS — a
 * sessionId that will never produce a row (typo'd URL, excluded session)
 * should not poll forever.
 */ const MAX_NULL_POLL_ATTEMPTS = 10;
/**
 * Single canonical definition of "still generating," mirroring
 * useSessionSummary's isGenerating — proto3's zero value (`UNSPECIFIED`) is
 * not a state the backend intentionally sets; it only appears when a row is
 * read before its terminal status has ever been assigned, so it is treated
 * as generating rather than terminal.
 */ function isGenerating(status) {
    return status === HandoffSummaryStatus.UNSPECIFIED || status === HandoffSummaryStatus.PENDING || status === HandoffSummaryStatus.GENERATING;
}
/**
 * Fetches a session's restart-handoff summary, polling every 2s while
 * generation is in flight. A `nil` response (no row exists yet) polls
 * identically to PENDING/GENERATING — there is no user-visible distinction
 * between "not started yet" and "in progress." Polling stops once `status`
 * reaches READY/ERROR, or once `MAX_NULL_POLL_ATTEMPTS` consecutive `nil`
 * reads have occurred, at which point `neverResolved` is set.
 *
 * Structural copy of useSessionSummary.ts (Story 3.1.1) — shared data layer
 * for RestartWithSummaryButton and HandoffSummarySection.
 */ function useHandoffSummary(sessionId) {
    const [data, setData] = (0,react.useState)(null);
    const [loading, setLoading] = (0,react.useState)(!!sessionId);
    const [error, setError] = (0,react.useState)(null);
    const [neverResolved, setNeverResolved] = (0,react.useState)(false);
    // Consecutive `nil` GetHandoffSummary reads. Only counts consecutive
    // nulls — reset to 0 the moment a non-null summary is observed (even if
    // still GENERATING).
    const nullAttemptsRef = (0,react.useRef)(0);
    const intervalRef = (0,react.useRef)(null);
    // Holds the latest fetchSummary closure so the interval tick (set up once
    // per sessionId) always calls the current version rather than a stale one.
    const fetchSummaryRef = (0,react.useRef)(async ()=>{});
    // The sessionId this hook instance is *currently* rendering for. Updated
    // synchronously every render (not via an effect) so it reflects the latest
    // prop before any async callback scheduled during this render can resolve
    // — guards against a fetch/trigger call started for an old sessionId
    // clobbering freshly-fetched state for a new one.
    const activeSessionIdRef = (0,react.useRef)(sessionId);
    activeSessionIdRef.current = sessionId;
    // True while a getHandoffSummary request (initial fetch, refetch, or poll
    // tick) is in flight. Guards against overlapping poll requests: if a
    // single call takes longer than POLL_INTERVAL_MS, the next interval tick
    // skips issuing a new request rather than firing a second one whose
    // out-of-order response could leave stale data displayed.
    const pollInFlightRef = (0,react.useRef)(false);
    // Shared between fetchSummary and trigger (safe: pollInFlightRef already
    // keeps them from firing concurrently) so either one cancels the other's
    // in-flight request, and both are cancelled on sessionId change/unmount.
    const startRequest = (0,useAbortableRequest/* useAbortableRequest */.X)();
    const stopPolling = (0,react.useCallback)(()=>{
        if (intervalRef.current !== null) {
            clearInterval(intervalRef.current);
            intervalRef.current = null;
        }
    }, []);
    const startPolling = (0,react.useCallback)(()=>{
        if (intervalRef.current !== null) return;
        intervalRef.current = setInterval(()=>{
            if (pollInFlightRef.current) return;
            fetchSummaryRef.current();
        }, POLL_INTERVAL_MS);
    }, []);
    const fetchSummary = (0,react.useCallback)(async ()=>{
        if (!sessionId) {
            setLoading(false);
            return;
        }
        pollInFlightRef.current = true;
        const signal = startRequest();
        try {
            const client = (0,promise_client/* createClient */.UU)(HandoffSummaryService, (0,transport/* getConnectTransport */.qW)());
            const response = await client.getHandoffSummary({
                sessionId
            }, {
                signal
            });
            if (signal.aborted) return;
            // A newer sessionId may have become active while this request was in
            // flight — discard this response rather than overwriting the fresher
            // session's state.
            if (activeSessionIdRef.current !== sessionId) return;
            var _response_summary;
            const summary = (_response_summary = response.summary) !== null && _response_summary !== void 0 ? _response_summary : null;
            setData(summary);
            setError(null);
            if (summary === null) {
                nullAttemptsRef.current += 1;
            } else {
                nullAttemptsRef.current = 0;
            }
            const exceededNullAttempts = summary === null && nullAttemptsRef.current >= MAX_NULL_POLL_ATTEMPTS;
            if (exceededNullAttempts) {
                setNeverResolved(true);
            }
            const shouldPoll = !exceededNullAttempts && (summary === null || isGenerating(summary.status));
            if (shouldPoll) {
                startPolling();
            } else {
                stopPolling();
            }
        } catch (err) {
            if (signal.aborted || activeSessionIdRef.current !== sessionId) return;
            setError(err instanceof Error ? err : new Error("Failed to load handoff summary"));
        } finally{
            if (!signal.aborted && activeSessionIdRef.current === sessionId) {
                setLoading(false);
            }
            pollInFlightRef.current = false;
        }
    }, [
        sessionId,
        startPolling,
        stopPolling,
        startRequest
    ]);
    (0,react.useEffect)(()=>{
        fetchSummaryRef.current = fetchSummary;
    }, [
        fetchSummary
    ]);
    (0,react.useEffect)(()=>{
        nullAttemptsRef.current = 0;
        setNeverResolved(false);
        stopPolling();
        fetchSummary();
        return ()=>stopPolling();
    // fetchSummary/stopPolling are recreated only when sessionId changes, so
    // this intentionally keys off sessionId alone to avoid re-running on
    // every fetch.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        sessionId
    ]);
    const trigger = (0,react.useCallback)(async ()=>{
        if (!sessionId) return;
        pollInFlightRef.current = true;
        const signal = startRequest();
        try {
            const client = (0,promise_client/* createClient */.UU)(HandoffSummaryService, (0,transport/* getConnectTransport */.qW)());
            const response = await client.triggerHandoffSummary({
                sessionId
            }, {
                signal
            });
            if (signal.aborted) return;
            // See fetchSummary's identical guard — the sessionId this call was
            // issued for may no longer be the active one by the time it resolves.
            if (activeSessionIdRef.current !== sessionId) return;
            var _response_summary;
            const summary = (_response_summary = response.summary) !== null && _response_summary !== void 0 ? _response_summary : null;
            setData(summary);
            setError(null);
            nullAttemptsRef.current = 0;
            setNeverResolved(false);
            const shouldPoll = summary === null || isGenerating(summary.status);
            if (shouldPoll) {
                startPolling();
            } else {
                stopPolling();
            }
        } catch (err) {
            if (signal.aborted) return;
            const e = err instanceof Error ? err : new Error("Failed to trigger handoff summary");
            if (activeSessionIdRef.current === sessionId) {
                setError(e);
            }
            // Rethrow so callers awaiting trigger() can distinguish success from
            // failure via the promise itself.
            throw e;
        } finally{
            pollInFlightRef.current = false;
        }
    }, [
        sessionId,
        startPolling,
        stopPolling,
        startRequest
    ]);
    return {
        data,
        loading,
        error,
        neverResolved,
        trigger,
        refetch: fetchSummary
    };
}

// EXTERNAL MODULE: ./src/components/ui/Collapsible.tsx + 1 modules
var Collapsible = __webpack_require__(21791);
// EXTERNAL MODULE: ./src/lib/hooks/useSessionService.ts + 1 modules
var useSessionService = __webpack_require__(73117);
// EXTERNAL MODULE: ./src/lib/routes.ts + 1 modules
var routes = __webpack_require__(76514);
// EXTERNAL MODULE: ./src/components/ui/LiveRegion.css.ts
var LiveRegion_css = __webpack_require__(76863);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FRestartWithSummaryButton.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA7WS227CMAyG7_sUvhwXQRTGpoWHQaZxW4s0ztKU08S7TwSh0XLYxbSrSP5_fz7E42Ve1vjxtp7AVwZguPUW9xpKS7tFBulVhgMVkcVpKMR2jTspFXoNGwwvSi3zDzpMw2F6GC2yYza-MPM-k51lR-qCRsuVUxypaTUU5CKFR9z96KR4NIZddafqIDKbJP9KgqGgAhru2mHW7D15SnFRtXygG331o2-JqzreOKrkKLrQStDghS8jnAtryP0OWrFsBpnNuT0s1lWQzpkh-awXYk_cvvQ52LCuZUMh7fkxz12v4y7WDbGGW1xZMoksHguOew2T8fx6ZCdRobWyJdNLn_7DLc0S87fvujtcPu2jXv-A6pPmidRgqNipKI8ON5UqsWG719CIk9ZjQc_PD583AbCtOZJKJA0-kNoG9EmQYNQqEK41pEedIovs-A3xK8v07AMAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FRestartWithSummaryButton_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA7WS227CMAyG7_sUvhwXQRTGpoWHQaZxW4s0ztKU08S7TwSh0XLYxbSrSP5_fz7E42Ve1vjxtp7AVwZguPUW9xpKS7tFBulVhgMVkcVpKMR2jTspFXoNGwwvSi3zDzpMw2F6GC2yYza_MPM_k51lR_qCRsuVUxypaTUU5CKFR9z96KR4NIZddafqIDKbJP9KgqGgAhru2mHW7D15SnFRtXygG331o2_JqzreOKrkKLrQStDghS8jnAtryP0OWrFsBpnNuT0s1lWQzpkh_awXYk_cvvQ52LCuZUMh7fkxz12v4y7WDbGGW1xZMoksHguOew2T8fx6ZCdRobWyJdNLn_7DLc0S87fvujtcPu2jXv_A6pPmidRgqNipKI8ON5UqsWG719CIk9ZjQc_PD583AbCtOZJKJA0_kNoG9EmQYNQqEK41pEedIovs_A3xK8v07AMAAA_22_7D = __webpack_require__(93893);
;// ./src/components/sessions/RestartWithSummaryButton.css.ts


var RestartWithSummaryButton_css_button = '_1fha96k1';
var RestartWithSummaryButton_css_container = '_1fha96k0';
var errorContainer = '_1fha96k2';
var errorDetails = '_1fha96k4';
var errorRawText = '_1fha96k5';
var RestartWithSummaryButton_css_errorText = '_1fha96k3';
;// ./src/components/sessions/RestartWithSummaryButton.tsx
/* __next_internal_client_entry_do_not_use__ RestartWithSummaryButton auto */ 










/**
 * Maps a backend `HandoffSummary.error_stage` to a plain-language message,
 * per design/ux.md's UX acceptance criterion #3 ("every error state has a
 * plain-language message, not a raw stage string"). The raw `errorMessage`
 * (which can be a technical string like a Go error's `.Error()` text) is
 * shown separately, behind a disclosure -- never as the primary text.
 */ const STAGE_MESSAGES = {
    transcript: "Couldn't read this session's conversation history.",
    generation: "Failed while generating the handoff summary.",
    persist: "Generated the summary but couldn't save it.",
    stale: "Generation didn't complete (the server may have restarted)."
};
function friendlyStageMessage(stage) {
    if (!stage) return "Something went wrong while generating this summary.";
    var _STAGE_MESSAGES_stage;
    return (_STAGE_MESSAGES_stage = STAGE_MESSAGES[stage]) !== null && _STAGE_MESSAGES_stage !== void 0 ? _STAGE_MESSAGES_stage : "Something went wrong while generating this summary.";
}
/**
 * Plain-language reason for a failed `createSession` restart call, per
 * design/ux.md's "restart-session-creation failure" flow -- a gap the plan
 * left undesigned. `CodeNotFound` means `restart_from_session_id` pointed at
 * a source session that no longer exists (e.g. archived/deleted between
 * generating the summary and clicking restart); anything else collapses to
 * a generic retry prompt rather than surfacing a raw transport/RPC message.
 */ function restartFailureReason(err) {
    if (err instanceof connect_error/* ConnectError */.T && err.code === code/* Code */.C.NotFound) {
        return "The original session no longer exists.";
    }
    return "Something went wrong — try again.";
}
/**
 * Drives the trigger -> poll -> create-session restart flow for a session's
 * handoff summary. Renders `null` when the backend reports the feature
 * disabled -- there is no dedicated feature-flag read RPC in this plan's
 * scope, so a `TriggerHandoffSummary` call that fails with
 * `Code.FailedPrecondition` is treated as the disabled signal (see Story
 * 3.2.1 task notes).
 */ function RestartWithSummaryButton(param) {
    let { sessionId, handoff } = param;
    const { data, neverResolved, trigger } = handoff;
    const { createSession } = (0,useSessionService/* useSessionService */.D)();
    const { track } = (0,AnalyticsContext/* useAnalytics */.st)();
    const router = (0,nav/* useRouter */.KA)();
    const [featureDisabled, setFeatureDisabled] = (0,react.useState)(false);
    const [triggering, setTriggering] = (0,react.useState)(false);
    const [triggerErrorMessage, setTriggerErrorMessage] = (0,react.useState)(null);
    const [restarting, setRestarting] = (0,react.useState)(false);
    const [restartErrorMessage, setRestartErrorMessage] = (0,react.useState)(null);
    // Finding 1: the still-live-source guard (connect.CodeFailedPrecondition;
    // see resolveRestartSource in server/services/session_service.go) must be
    // a real, visible confirmation -- not silently retried -- since two live
    // CLI processes sharing the same working directory (SESSION_TYPE_DIRECTORY,
    // no worktree isolation) is a real git-corruption risk. True once the
    // first attempt is rejected; the user must click "Restart anyway" to
    // proceed.
    const [needsLiveSourceConfirm, setNeedsLiveSourceConfirm] = (0,react.useState)(false);
    const [liveMessage, setLiveMessage] = (0,react.useState)("");
    const prevPhaseRef = (0,react.useRef)(null);
    const handleTrigger = (0,react.useCallback)(async ()=>{
        setTriggering(true);
        setTriggerErrorMessage(null);
        try {
            await trigger();
        } catch (err) {
            if (err instanceof connect_error/* ConnectError */.T && err.code === code/* Code */.C.FailedPrecondition) {
                setFeatureDisabled(true);
            } else {
                setTriggerErrorMessage(err instanceof Error ? err.message : "Failed to generate summary");
            }
        } finally{
            setTriggering(false);
        }
    }, [
        trigger
    ]);
    const handleRestart = (0,react.useCallback)(async ()=>{
        if (!data) return;
        track({
            name: "session_restart_with_summary",
            category: "user_action",
            component: "RestartWithSummaryButton",
            sessionId
        });
        setRestarting(true);
        setRestartErrorMessage(null);
        setLiveMessage("Starting new session…");
        const baseRequest = {
            prompt: data.summaryText,
            restartFromSessionId: sessionId
        };
        try {
            const session = await createSession(baseRequest);
            if (session) {
                router.push(routes/* routes */.J.sessionDetail(session.id));
            }
        } catch (err) {
            // The source session is normally still open when this button is
            // clicked -- that's the entire point of the feature (the source is
            // degrading mid-session) -- so the backend's still-live-source guard
            // (connect.CodeFailedPrecondition; see resolveRestartSource in
            // server/services/session_service.go) fires on essentially every real
            // click. This is NOT auto-retried: two live CLI processes sharing the
            // same working directory can corrupt git state, so proceeding needs a
            // real, visible confirmation click ("Restart anyway"), not the
            // original click reinterpreted as consent.
            if (err instanceof connect_error/* ConnectError */.T && err.code === code/* Code */.C.FailedPrecondition) {
                setNeedsLiveSourceConfirm(true);
                setLiveMessage("The source session is still running. Restarting shares its working directory, which can corrupt git state. Confirmation required.");
            } else {
                const reason = restartFailureReason(err);
                setRestartErrorMessage("Couldn't start the new session. ".concat(reason));
                setLiveMessage("Couldn't start the new session: ".concat(reason));
            }
        } finally{
            setRestarting(false);
        }
    }, [
        createSession,
        data,
        router,
        sessionId,
        track
    ]);
    // Fires only from the "Restart anyway" button below, i.e. only after the
    // user has explicitly seen and dismissed the live-source warning -- this
    // IS the confirmation the still-live-source guard exists to require.
    const handleConfirmRestart = (0,react.useCallback)(async ()=>{
        if (!data) return;
        track({
            name: "session_restart_with_summary_confirmed",
            category: "user_action",
            component: "RestartWithSummaryButton",
            sessionId
        });
        setRestarting(true);
        setRestartErrorMessage(null);
        setLiveMessage("Starting new session…");
        try {
            const session = await createSession({
                prompt: data.summaryText,
                restartFromSessionId: sessionId,
                confirmRestartWithLiveSource: true
            });
            if (session) {
                setNeedsLiveSourceConfirm(false);
                router.push(routes/* routes */.J.sessionDetail(session.id));
            }
        } catch (err) {
            setNeedsLiveSourceConfirm(false);
            const reason = restartFailureReason(err);
            setRestartErrorMessage("Couldn't start the new session. ".concat(reason));
            setLiveMessage("Couldn't start the new session: ".concat(reason));
        } finally{
            setRestarting(false);
        }
    }, [
        createSession,
        data,
        router,
        sessionId,
        track
    ]);
    let phase;
    if (triggerErrorMessage !== null || data && data.status === HandoffSummaryStatus.ERROR) {
        phase = "error";
    } else if (triggering || data && !neverResolved && isGenerating(data.status)) {
        phase = "generating";
    } else if (data && !neverResolved && data.status === HandoffSummaryStatus.READY) {
        phase = "ready";
    } else {
        phase = "idle";
    }
    // Announces phase transitions through the single shared aria-live region
    // below, mirroring SessionSummaryPanel.tsx's pattern (design/ux.md Surface
    // 3's Accessibility section): idle->generating and generating->ready/error
    // are driven off `phase` here; ready->creating and creating->error are
    // announced imperatively inside handleRestart above, since "creating" is
    // not one of this button's four render phases.
    (0,react.useEffect)(()=>{
        if (prevPhaseRef.current === phase) return;
        prevPhaseRef.current = phase;
        if (phase === "generating") {
            setLiveMessage("Generating handoff summary…");
        } else if (phase === "ready") {
            setLiveMessage("Handoff summary ready.");
        } else if (phase === "error") {
            const message = triggerErrorMessage || friendlyStageMessage(data === null || data === void 0 ? void 0 : data.errorStage);
            setLiveMessage("Couldn't generate handoff summary: ".concat(message));
        }
    // triggerErrorMessage/data are read for their current-render value only
    // within the "error" branch above -- intentionally not dependencies so
    // this doesn't re-run on every poll tick (mirrors SessionSummaryPanel's
    // identical phase-transition effect).
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        phase
    ]);
    if (featureDisabled) {
        return null;
    }
    const liveRegion = /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        role: "status",
        "aria-live": "polite",
        "aria-atomic": "true",
        className: LiveRegion_css/* srOnly */.s,
        children: liveMessage
    });
    if (phase === "idle") {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    type: "button",
                    className: RestartWithSummaryButton_css_button,
                    onClick: handleTrigger,
                    "data-testid": "restart-with-summary-button",
                    children: "Generate restart summary"
                }),
                liveRegion
            ]
        });
    }
    if (phase === "generating") {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    type: "button",
                    className: RestartWithSummaryButton_css_button,
                    disabled: true,
                    "data-testid": "restart-with-summary-button",
                    children: "Generating summary..."
                }),
                liveRegion
            ]
        });
    }
    if (phase === "error") {
        // triggerErrorMessage is a client-side catch (e.g. a network failure
        // calling trigger()) with no backend stage, so it's already plain text.
        // A data-driven ERROR row does have a stage -- map it to a friendly
        // message and keep the raw errorMessage behind a disclosure, never as
        // the primary text (design/ux.md UX AC #3).
        const errorMessage = triggerErrorMessage || friendlyStageMessage(data === null || data === void 0 ? void 0 : data.errorStage);
        const rawDetail = !triggerErrorMessage ? data === null || data === void 0 ? void 0 : data.errorMessage : undefined;
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: errorContainer,
            "data-testid": "restart-with-summary-error",
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: RestartWithSummaryButton_css_errorText,
                    "data-testid": "restart-with-summary-error-message",
                    children: errorMessage
                }),
                rawDetail && /*#__PURE__*/ (0,jsx_runtime.jsxs)("details", {
                    className: errorDetails,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("summary", {
                            children: "Details"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: errorRawText,
                            children: rawDetail
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    type: "button",
                    className: RestartWithSummaryButton_css_button,
                    onClick: handleTrigger,
                    "data-testid": "restart-with-summary-retry",
                    children: "Try again"
                }),
                liveRegion
            ]
        });
    }
    // phase === "ready"
    if (needsLiveSourceConfirm) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: RestartWithSummaryButton_css_container,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: errorContainer,
                    "data-testid": "restart-with-summary-live-source-warning",
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: RestartWithSummaryButton_css_errorText,
                            children: "The source session is still running. Restarting shares its working directory — concurrent writes can corrupt git state."
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            type: "button",
                            className: RestartWithSummaryButton_css_button,
                            onClick: handleConfirmRestart,
                            disabled: restarting,
                            "data-testid": "restart-with-summary-confirm-live-source",
                            children: restarting ? "Starting session..." : "Restart anyway"
                        })
                    ]
                }),
                restartErrorMessage && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: RestartWithSummaryButton_css_errorText,
                    "data-testid": "restart-with-summary-restart-error",
                    children: restartErrorMessage
                }),
                liveRegion
            ]
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: RestartWithSummaryButton_css_container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                className: RestartWithSummaryButton_css_button,
                onClick: handleRestart,
                disabled: restarting,
                "data-testid": "restart-with-summary-button",
                children: restarting ? "Starting session..." : "Start new session from this summary"
            }),
            restartErrorMessage && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: RestartWithSummaryButton_css_errorText,
                "data-testid": "restart-with-summary-restart-error",
                children: restartErrorMessage
            }),
            liveRegion
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FHandoffSummarySection.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61U0XKCMBB85yvusT7EAW21ho_pxCTCjSGhIRSx4793CB1EFHFaX8gMt7eX3Wwyz0quP8MQvgMAgUWuWE1hp-QhDsCvRKCV3KHRFLhRZaabSsJyCl_MvhDyEW3kcWGPi-MsDk7BvGWMPOPOaEcKPMoheLmdNTTcKGOviHwpZ0KgTm5MgTDuqF2tJAV0TCHvTV88Xc_yWYx3lQ1MCj1-y_g-sabUYtjC27qxQlpimcCyuDJ63cNQiPIDFEahGMB2fa2vt7UyhYkm6GRWUOBSO2nvC_XmVLapN9_ehLeH0qFQS5JKTFJHIeq1rx5q9_Xqt32ASMfjF_atWE9MYvdj3PG8e56buLKP24zjoouNsXHgoY_bXp4lam_qxJF2CW3iMlBVT2QyWo9bEj2Q19X57EYd79Txv78y7V4yZhPUtHlSOlLxP9KOR3qedgJx5vqW1GepO5ahqilkRpsiZ1xOm3BXV5Wik8QzUcitJO0FBKiMFWRrJdtT8Atp_sTB6QeV4CWtCgYAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FHandoffSummarySection_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61U0XKCMBB85yvusT7EAW21ho_pxCTCjSGhIRSx4793CB1EFHFaX8gMt7eX3Wwyz0quP8MQvgMAgUWuWE1hp_QhDsCvRKCV3KHRFLhRZaabSsJyCl_MvhDyEW3kcWGPi_MsDk7BvGWMPOPOaEcKPMoheLmdNTTcKGOviHwpZ0KgTm5MgTDuqF2tJAV0TCHvTV88Xc_yWYx3lQ1MCj1_y_g_sabUYtjC27qxQlpimcCyuDJ63cNQiPIDFEahGMB2fa2vt7UyhYkm6GRWUOBSO2nvC_XmVLapN9_ehLeH0qFQS5JKTFJHIeq1rx5q9_Xqt32ASMfjF_atWE9MYvdj3PG8e56buLKP24zjoouNsXHgoY_bXp4lam_qxJF2CW3iMlBVT2QyWo9bEj2Q19X57EYd79Txv78y7V4yZhPUtHlSOlLxP9KOR3qedgJx5vqW1GepO5ahqilkRpsiZ1xOm3BXV5Wik8QzUcitJO0FBKiMFWRrJdtT8Atp_sTB6QeV4CWtCgYAAA_22_7D = __webpack_require__(84399);
;// ./src/components/sessions/HandoffSummarySection.css.ts


var activeTask = 'mucnq0c';
var HandoffSummarySection_css_emptyText = 'mucnq01';
var HandoffSummarySection_css_list = 'mucnq02';
var HandoffSummarySection_css_pill = 'mucnq0b';
var previewDetails = 'mucnq0d';
var previewText = 'mucnq0e';
var HandoffSummarySection_css_row = 'mucnq03';
var rowHeader = 'mucnq04';
var HandoffSummarySection_css_section = 'mucnq00';
var HandoffSummarySection_css_statusError = 'mucnq09';
var statusGenerating = 'mucnq0a';
var statusIcon = 'mucnq05';
var statusLabel = 'mucnq06';
var statusReady = 'mucnq08';
var HandoffSummarySection_css_timestamp = 'mucnq07';
;// ./src/components/sessions/HandoffSummarySection.tsx
/* __next_internal_client_entry_do_not_use__ HandoffSummarySection auto */ 






/**
 * Mirrors CheckpointList.tsx's formatRelativeTime helper (same
 * `{seconds, nanos}` Timestamp shape as HandoffSummaryProto.generatedAt) --
 * reused verbatim per this story's task notes rather than re-derived.
 */ function relativeTimeFromSeconds(seconds) {
    if (seconds < 60) return "".concat(seconds, "s ago");
    if (seconds < 3600) return "".concat(Math.floor(seconds / 60), "m ago");
    if (seconds < 86400) return "".concat(Math.floor(seconds / 3600), "h ago");
    return "".concat(Math.floor(seconds / 86400), "d ago");
}
function HandoffSummarySection_formatRelativeTime(timestamp) {
    if (!timestamp || timestamp.seconds === BigInt(0)) return "Unknown time";
    const now = Date.now();
    const date = new Date(Number(timestamp.seconds) * 1000);
    const seconds = Math.floor((now - date.getTime()) / 1000);
    return relativeTimeFromSeconds(Math.max(seconds, 0));
}
const STATUS_CONFIG = {
    [HandoffSummaryStatus.UNSPECIFIED]: {
        label: "Pending",
        icon: "⏳",
        className: statusGenerating
    },
    [HandoffSummaryStatus.PENDING]: {
        label: "Pending",
        icon: "⏳",
        className: statusGenerating
    },
    [HandoffSummaryStatus.GENERATING]: {
        label: "Generating",
        icon: "⏳",
        className: statusGenerating
    },
    [HandoffSummaryStatus.READY]: {
        label: "Ready",
        icon: "✓",
        className: statusReady
    },
    [HandoffSummaryStatus.ERROR]: {
        label: "Error",
        icon: "⚠",
        className: HandoffSummarySection_css_statusError
    }
};
function statusConfig(status) {
    var _STATUS_CONFIG_status;
    return (_STATUS_CONFIG_status = STATUS_CONFIG[status]) !== null && _STATUS_CONFIG_status !== void 0 ? _STATUS_CONFIG_status : STATUS_CONFIG[HandoffSummaryStatus.UNSPECIFIED];
}
/**
 * A single point-in-time record row for this session's handoff summary.
 * Embeds RestartWithSummaryButton (when the feature is enabled), fed this
 * component's own `handoff` result -- it derives idle/generating/ready/error
 * phase from that shared data rather than polling independently, so
 * rendering it unconditionally here covers READY (the "start new session"
 * action), GENERATING (disabled button), and ERROR (its own error text +
 * "Try again" retry) without this component branching on status itself for
 * that part.
 */ function HandoffSummaryRow(param) {
    let { sessionId, summary, featureEnabled, handoff } = param;
    const { label, icon, className } = statusConfig(summary.status);
    let timestampText;
    if (summary.status === HandoffSummaryStatus.READY) {
        timestampText = "ready ".concat(HandoffSummarySection_formatRelativeTime(summary.generatedAt));
    } else if (summary.status === HandoffSummaryStatus.ERROR) {
        timestampText = "last attempt: ".concat(HandoffSummarySection_formatRelativeTime(summary.generatedAt));
    } else {
        timestampText = "started ".concat(HandoffSummarySection_formatRelativeTime(summary.generationStartedAt));
    }
    const isReady = summary.status === HandoffSummaryStatus.READY;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: HandoffSummarySection_css_row,
        role: "listitem",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: rowHeader,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: "".concat(statusIcon, " ").concat(className),
                        "aria-hidden": "true",
                        children: icon
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: statusLabel,
                        children: label
                    }),
                    isReady && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: HandoffSummarySection_css_pill,
                        children: [
                            summary.middleMessagesSummarized,
                            " turns summarized"
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: HandoffSummarySection_css_timestamp,
                        children: timestampText
                    })
                ]
            }),
            isReady && summary.activeTask && /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                className: activeTask,
                children: [
                    "Active task: ",
                    summary.activeTask
                ]
            }),
            isReady && summary.summaryText && /*#__PURE__*/ (0,jsx_runtime.jsxs)("details", {
                className: previewDetails,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("summary", {
                        children: "Preview full handoff text"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("pre", {
                        className: previewText,
                        children: summary.summaryText
                    })
                ]
            }),
            featureEnabled && /*#__PURE__*/ (0,jsx_runtime.jsx)(RestartWithSummaryButton, {
                sessionId: sessionId,
                handoff: handoff
            })
        ]
    });
}
/**
 * Always-rendered, collapsible record of this session's handoff-summary
 * state in the Info tab -- `role="list"`/`"listitem"`, never `"status"`,
 * since this is a reviewed record, not a live announcement, with an
 * explicit empty state rather than omitting the section. Defaults to
 * expanded (unlike WorkflowHistorySection): its single row is small and
 * carries the primary restart action, so there's little cost to showing it.
 */ function HandoffSummarySection(param) {
    let { sessionId } = param;
    // Single poll instance for this session, shared with every
    // RestartWithSummaryButton mounted below (Finding 2) -- previously each
    // button ran its own independent useHandoffSummary(sessionId) call, i.e.
    // two competing 2s poll loops against the same session.
    const handoff = useHandoffSummary(sessionId);
    const { data } = handoff;
    // The backend defaults this feature to enabled (HandoffSummaryConfig's
    // EnabledOrDefault()), but useFeatureFlag defaults to `false` while the
    // flag list is still loading -- so treating `false` as "disabled" during
    // that window would flash a false disabled message on every page load.
    // Guard: only treat the feature as disabled once loading has finished.
    const { isLoading: flagsLoading } = (0,FeatureFlagsContext/* useFeatureFlags */.h)();
    const flagEnabled = (0,FeatureFlagsContext/* useFeatureFlag */.u)("handoff-summary");
    const featureEnabled = flagsLoading || flagEnabled;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(Collapsible/* CollapsibleSection */.i, {
        sectionKey: "handoff-summary",
        title: "Handoff Summary",
        defaultExpanded: true,
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: HandoffSummarySection_css_section,
            children: !featureEnabled && data === null ? /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: HandoffSummarySection_css_emptyText,
                children: "Restart-with-summary is disabled for this workspace."
            }) : data === null ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: HandoffSummarySection_css_emptyText,
                        children: "No handoff summary generated for this session."
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(RestartWithSummaryButton, {
                        sessionId: sessionId,
                        handoff: handoff
                    })
                ]
            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: HandoffSummarySection_css_list,
                role: "list",
                "aria-label": "Handoff summary",
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(HandoffSummaryRow, {
                    sessionId: sessionId,
                    summary: data,
                    featureEnabled: featureEnabled,
                    handoff: handoff
                })
            })
        })
    });
}

// EXTERNAL MODULE: ./src/components/guidance/GuidanceRequestPanel.tsx + 3 modules
var GuidanceRequestPanel = __webpack_require__(73090);
// EXTERNAL MODULE: ./src/lib/transport/watch-ws-transport.ts
var watch_ws_transport = __webpack_require__(90062);
// EXTERNAL MODULE: ./src/lib/telemetry/rpcTiming.ts
var rpcTiming = __webpack_require__(77665);
;// ./src/lib/hooks/useShells.ts
/* __next_internal_client_entry_do_not_use__ useShells auto */ // +feature: shell-tabs








function shellToTab(shell) {
    let status = "running";
    if (shell.status === types_pb/* ShellStatus */.d5.STOPPED) status = "stopped";
    else if (shell.status === types_pb/* ShellStatus */.d5.ERROR) status = "error";
    return {
        id: shell.id,
        name: shell.name || shell.command || "shell",
        command: shell.command,
        status,
        exitCode: shell.exitCode !== 0 ? shell.exitCode : undefined
    };
}
function useShells(sessionId) {
    const [shells, setShells] = (0,react.useState)([]);
    const [isLoading, setIsLoading] = (0,react.useState)(false);
    // Use context directly so useShells works in test environments without AnalyticsContextProvider
    const analyticsCtx = (0,react.useContext)(AnalyticsContext/* AnalyticsContext */.Ig);
    const clientRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        // Guard: createAuthInterceptor may be undefined in test environments where @/lib/config
        // is partially mocked (e.g. only getApiBaseUrl is exported).
        const authInterceptor = typeof config/* createAuthInterceptor */._ === "function" ? (0,config/* createAuthInterceptor */._)() : null;
        const interceptors = analyticsCtx ? [
            authInterceptor,
            (0,rpcTiming/* createRpcTimingInterceptor */.r)(analyticsCtx)
        ].filter(Boolean) : [
            authInterceptor
        ].filter(Boolean);
        const transport = (0,watch_ws_transport/* createSessionWatchTransport */.ZX)({
            baseUrl: (0,config/* getApiBaseUrl */.h)(),
            interceptors: interceptors
        });
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, transport);
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
    const refetch = (0,react.useCallback)(async ()=>{
        if (!clientRef.current || !sessionId) return;
        setIsLoading(true);
        try {
            const response = await clientRef.current.listShells({
                sessionId
            });
            setShells(response.shells.map(shellToTab));
        } catch (e) {
        // Silently ignore — shells are additive
        } finally{
            setIsLoading(false);
        }
    }, [
        sessionId
    ]);
    (0,react.useEffect)(()=>{
        refetch();
    }, [
        refetch
    ]);
    const spawnShell = (0,react.useCallback)(async (req)=>{
        if (!clientRef.current || !sessionId) return null;
        try {
            var _req_name, _req_command, _req_workingDir;
            const response = await clientRef.current.spawnShell({
                sessionId,
                name: (_req_name = req.name) !== null && _req_name !== void 0 ? _req_name : "",
                command: (_req_command = req.command) !== null && _req_command !== void 0 ? _req_command : "",
                workingDir: (_req_workingDir = req.workingDir) !== null && _req_workingDir !== void 0 ? _req_workingDir : ""
            });
            if (!response.shell) return null;
            const tab = shellToTab(response.shell);
            setShells((prev)=>[
                    ...prev,
                    tab
                ]);
            return tab;
        } catch (err) {
            throw err;
        }
    }, [
        sessionId
    ]);
    const stopShell = (0,react.useCallback)(async (shellId)=>{
        if (!clientRef.current || !sessionId) return false;
        try {
            const response = await clientRef.current.stopShell({
                sessionId,
                shellId
            });
            if (response.success) {
                setShells((prev)=>prev.map((s)=>s.id === shellId ? {
                            ...s,
                            status: "stopped"
                        } : s));
            }
            return response.success;
        } catch (e) {
            return false;
        }
    }, [
        sessionId
    ]);
    const restartShell = (0,react.useCallback)(async (shellId)=>{
        if (!clientRef.current || !sessionId) return false;
        try {
            const response = await clientRef.current.restartShell({
                sessionId,
                shellId
            });
            if (response.success) {
                setShells((prev)=>prev.map((s)=>s.id === shellId ? {
                            ...s,
                            status: "running",
                            exitCode: undefined
                        } : s));
            }
            return response.success;
        } catch (e) {
            return false;
        }
    }, [
        sessionId
    ]);
    const deleteShell = (0,react.useCallback)(async (shellId)=>{
        if (!clientRef.current || !sessionId) return false;
        try {
            const response = await clientRef.current.deleteShell({
                sessionId,
                shellId
            });
            if (response.success) {
                setShells((prev)=>prev.filter((s)=>s.id !== shellId));
            }
            return response.success;
        } catch (e) {
            return false;
        }
    }, [
        sessionId
    ]);
    const updateShellStatus = (0,react.useCallback)((shellId, status, exitCode)=>{
        setShells((prev)=>prev.map((s)=>s.id === shellId ? {
                    ...s,
                    status,
                    exitCode: exitCode !== undefined ? exitCode : s.exitCode
                } : s));
    }, []);
    return {
        shells,
        isLoading,
        spawnShell,
        stopShell,
        restartShell,
        deleteShell,
        updateShellStatus,
        refetch
    };
}

// EXTERNAL MODULE: ./src/lib/contexts/NotificationContext.tsx + 12 modules
var NotificationContext = __webpack_require__(62275);
// EXTERNAL MODULE: ./src/lib/utils/viewedSessions.ts
var viewedSessions = __webpack_require__(76991);
// EXTERNAL MODULE: ./src/lib/telemetry/active-session.ts
var active_session = __webpack_require__(73483);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FShellTab.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA6VT0W7CMAx85yvyuD4E0bIxKB8zhcZtPUISOSktTPv3KemAisHotMf44vPpfJ52qanS7Yx9TBiT6KwSh5yVCrr1hDGhsNIcPexczgrQHiiUK2Fzthf0xPlbuoJjRsfsmASkcUDcgYLC50wbDevJ52Taz0jjjNJozx0e4ZphLiJDYZShayydRSzI4q4m1NuczQbUWaRuUfo6Z0sbtdeAVe3Pz40hCcRJSGzcj9mr3_nnkX8jim1FptGS31TZJIOW51Et2bDlZVRL9OLcs3hg6mZoKuoaCH2o7ETHv_1Ks1lvkdkDlcq0OatRStCh5qHz_AKAUmgdugC1NXrgzooCwq5bEnYg7PXfiTokvVCqUHMFpb_54zxw-aeB743zWB54YbQH7YfQyZarHKWjgrRILn9OF3Brp56EdlYQaH839Vm_u4ZcwKzBk0IrpERdxYw-uqiLP3kd9jgqY8v7pxjT9wUsrTDaNgQAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FShellTab_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA6VT0W7CMAx85yvyuD4E0bIxKB8zhcZtPUISOSktTPv3KemAisHotMf44vPpfJ52qanS7Yx9TBiT6KwSh5yVCrr1hDGhsNIcPexczgrQHiiUK2Fzthf0xPlbuoJjRsfsmASkcUDcgYLC50wbDevJ52Taz0jjjNJozx0e4ZphLiJDYZShayydRSzI4q4m1NuczQbUWaRuUfo6Z0sbtdeAVe3Pz40hCcRJSGzcj9mr3_nnkX8jim1FptGS31TZJIOW51Et2bDlZVRL9OLcs3hg6mZoKuoaCH2o7ETHv_1Ks1lvkdkDlcq0OatRStCh5qHz_AKAUmgdugC1NXrgzooCwq5bEnYg7PXfiTokvVCqUHMFpb_54zxw_aeB743zWB54YbQH7YfQyZarHKWjgrRILn9OF3Brp56EdlYQaH839Vm_u4ZcwKzBk0IrpERdxYw_uqiLP3kd9jgqY8v7pxjT9wUsrTDaNgQAAA_22_7D = __webpack_require__(22189);
;// ./src/components/sessions/ShellTab.css.ts


var actionButton = 'x1og1k8';
var ShellTab_css_actions = 'x1og1k7';
var errorIndicator = 'x1og1k1';
var statusDot = {running:'x1og1k3 x1og1k2',stopped:'x1og1k4 x1og1k2',error:'x1og1k5 x1og1k2'};
var ShellTab_css_tabLabel = 'x1og1k0';
var tabName = 'x1og1k6';
;// ./src/components/sessions/ShellTab.tsx
/* __next_internal_client_entry_do_not_use__ ShellTabLabel auto */ 
// +feature: shell-tabs


function ShellTabLabel(param) {
    let { shell, onStop, onRestart, onClose } = param;
    const handleStop = (0,react.useCallback)((e)=>{
        e.stopPropagation();
        onStop(shell.id);
    }, [
        onStop,
        shell.id
    ]);
    const handleRestart = (0,react.useCallback)((e)=>{
        e.stopPropagation();
        onRestart(shell.id);
    }, [
        onRestart,
        shell.id
    ]);
    const handleClose = (0,react.useCallback)((e)=>{
        e.stopPropagation();
        onClose(shell.id);
    }, [
        onClose,
        shell.id
    ]);
    const hasError = shell.status === "error" || shell.status !== "running" && shell.exitCode != null && shell.exitCode !== 0;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: ShellTab_css_tabLabel,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: statusDot[shell.status],
                title: shell.status,
                "aria-label": "Shell status: ".concat(shell.status)
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: tabName,
                title: shell.name || shell.command || "shell",
                children: shell.name || shell.command || "shell"
            }),
            hasError && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: errorIndicator,
                title: shell.exitCode != null ? "Exited with code ".concat(shell.exitCode) : "Shell errored",
                "aria-label": shell.exitCode != null ? "Exit code ".concat(shell.exitCode) : "Shell errored",
                children: "!"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: ShellTab_css_actions,
                children: [
                    shell.status === "running" && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: actionButton,
                        onClick: handleStop,
                        title: "Stop shell",
                        "aria-label": "Stop shell ".concat(shell.name),
                        children: "■"
                    }),
                    shell.status !== "running" && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: actionButton,
                        onClick: handleRestart,
                        title: "Restart shell",
                        "aria-label": "Restart shell ".concat(shell.name),
                        children: "↺"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: actionButton,
                        onClick: handleClose,
                        title: "Delete shell",
                        "aria-label": "Delete shell ".concat(shell.name),
                        children: "\xd7"
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/components/ui/RepoPathInput.tsx + 2 modules
var RepoPathInput = __webpack_require__(34609);
// EXTERNAL MODULE: ./src/lib/omnibar/recentShellCommands.ts
var recentShellCommands = __webpack_require__(16244);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FNewShellDialog.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA9WVUXPiIBDH3_0UvNxMfcAhsb0q_TAdAiSiCBwQTdLpd7-BeDZqTOJ0-nAv6rC77O6P_66LHVnRcovAxwwAo53wQisMclFx9jYDQCjHPQYo_M4I3RVWl4pBqqW2GByIfYLwPVnzJrUNmQcvJpyRpMYgl7wKB0SKQkHh-d5hQLny3Ibjbem8yGtItfJc-a6pgUIxXmGQIITeZp-zRVtlEqscK2Mdy8i0ZdxikJgKOC0Fu_LadrygJUyU7vqi5Sr6GMKYUMWNNY3Wo2B-g8Fzikzsdk8qeDqjRNKnBKHDEUCwTE3Vzyd8QyYspy17qmW5V8FSEHOTNZl3gKQRSK6Vh040_MaZxYzRfuSi2Pgbj0306OWI5m0_thAqKuCcdhnTfq-RtO428jzSSDbaSHG_kQtkL5M0lJCB67JJAttNENjrkMDSZn7tj74w9GKi0a5LL4XiGCiteEejCUK_2pqqEB5TnurLdNVlhLGRhPKNlozbCKyfBL0gi3NNS9fyba_tjZLdoN_9UrpZDlFbXLF7amo6YoVe9zucs77eEYG3RDlDLFd-UE3jr89_4PWTkdefPCSldaEvo0W7cb-44I0-nF58bERWXaCrSSH7-0P154LqP-H-b_xWD_BTF_wwE45kkrMYqw2hwtcYoMVLN6PSHhIp9TH8OZ-D1wMTmk7o-XwRGdjrRxumLnwOLfTBEbzY-Nlkkf34HK4HdVQ_vIWzEZVkD6hkZGQ-_wK4CXdPwAkAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FNewShellDialog_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9WVUXPiIBDH3_0UvNxMfcAhsb0q_TAdAiSiCBwQTdLpd7_BeDZqTOJ0_nAv6rC77O6P_66LHVnRcovAxwwAo53wQisMclFx9jYDQCjHPQYo_M4I3RVWl4pBqqW2GByIfYLwPVnzJrUNmQcvJpyRpMYgl7wKB0SKQkHh_d5hQLny3Ibjbem8yGtItfJc_a6pgUIxXmGQIITeZp_zRVtlEqscK2Mdy8i0ZdxikJgKOC0Fu_LadrygJUyU7vqi5Sr6GMKYUMWNNY3Wo2B_g8Fzikzsdk8qeDqjRNKnBKHDEUCwTE3Vzyd8QyYspy17qmW5V8FSEHOTNZl3gKQRSK6Vh040_MaZxYzRfuSi2Pgbj0306OWI5m0_thAqKuCcdhnTfq_RtO428jzSSDbaSHG_kQtkL5M0lJCB67JJAttNENjrkMDSZn7tj74w9GKi0a5LL4XiGCiteEejCUK_2pqqEB5TnurLdNVlhLGRhPKNlozbCKyfBL0gi3NNS9fyba_tjZLdoN_9UrpZDlFbXLF7amo6YoVe9zucs77eEYG3RDlDLFd_UE3jr89_4PWTkdefPCSldaEvo0W7cb_44I0_nF58bERWXaCrSSH7_0P154LqP_H_b_xWD_BTF_wwE45kkrMYqw2hwtcYoMVLN6PSHhIp9TH8OZ_D1wMTmk7o_XwRGdjrRxumLnwOLfTBEbzY_Nlkkf34HK4HdVQ_vIWzEZVkD6hkZGQ_wK4CXdPwAkAAA_22_7D = __webpack_require__(92521);
;// ./src/components/sessions/NewShellDialog.css.ts


var NewShellDialog_css_actions = 'ka8cuj6';
var NewShellDialog_css_cancelButton = 'ka8cuj7';
var dialog = 'ka8cuj1';
var NewShellDialog_css_errorText = 'ka8cuj9';
var fieldGroup = 'ka8cuj3';
var input = 'ka8cuj5';
var label = 'ka8cuj4';
var overlay = 'ka8cuj0';
var submitButton = 'ka8cuj8';
var suggestionChip = 'ka8cujb';
var suggestions = 'ka8cuja';
var NewShellDialog_css_title = 'ka8cuj2';
;// ./src/components/sessions/NewShellDialog.tsx
/* __next_internal_client_entry_do_not_use__ NewShellDialog auto */ 
// +feature: shell-tabs





function NewShellDialog(param) {
    let { onSubmit, onCancel, defaultWorkingDir = "" } = param;
    const [name, setName] = (0,react.useState)("");
    const [command, setCommand] = (0,react.useState)("");
    const [workingDir, setWorkingDir] = (0,react.useState)(defaultWorkingDir);
    const [isSubmitting, setIsSubmitting] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    const [recentCommands] = (0,react.useState)(()=>(0,recentShellCommands/* getRecentShellCommands */.e)());
    const nameInputRef = (0,react.useRef)(null);
    // Focus name input on mount
    (0,react.useEffect)(()=>{
        var _nameInputRef_current;
        (_nameInputRef_current = nameInputRef.current) === null || _nameInputRef_current === void 0 ? void 0 : _nameInputRef_current.focus();
    }, []);
    // Close on Escape key
    (0,react.useEffect)(()=>{
        const handler = (e)=>{
            if (e.key === "Escape") onCancel();
        };
        document.addEventListener("keydown", handler);
        return ()=>document.removeEventListener("keydown", handler);
    }, [
        onCancel
    ]);
    const handleSubmit = (0,react.useCallback)(async (e)=>{
        e.preventDefault();
        setIsSubmitting(true);
        setError(null);
        try {
            const trimmedCommand = command.trim();
            if (trimmedCommand) (0,recentShellCommands/* addRecentShellCommand */._)(trimmedCommand);
            await onSubmit({
                name: name.trim() || undefined,
                command: trimmedCommand || undefined,
                workingDir: workingDir.trim() || undefined
            });
        } catch (err) {
            setError(err instanceof Error ? err.message : "Failed to spawn shell");
        } finally{
            setIsSubmitting(false);
        }
    }, [
        onSubmit,
        name,
        command,
        workingDir
    ]);
    const handleOverlayClick = (0,react.useCallback)((e)=>{
        if (e.target === e.currentTarget) onCancel();
    }, [
        onCancel
    ]);
    const content = /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: overlay,
        onClick: handleOverlayClick,
        role: "dialog",
        "aria-modal": "true",
        "aria-labelledby": "new-shell-dialog-title",
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: dialog,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                    id: "new-shell-dialog-title",
                    className: NewShellDialog_css_title,
                    children: "New Shell"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("form", {
                    onSubmit: handleSubmit,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: fieldGroup,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                    htmlFor: "shell-name",
                                    className: label,
                                    children: "Name (optional)"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                    ref: nameInputRef,
                                    id: "shell-name",
                                    type: "text",
                                    className: input,
                                    value: name,
                                    onChange: (e)=>setName(e.target.value),
                                    placeholder: "e.g. dev server",
                                    autoComplete: "off"
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: fieldGroup,
                            style: {
                                marginTop: "12px"
                            },
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                    htmlFor: "shell-command",
                                    className: label,
                                    children: "Command (optional)"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                    id: "shell-command",
                                    type: "text",
                                    className: input,
                                    value: command,
                                    onChange: (e)=>setCommand(e.target.value),
                                    placeholder: "Defaults to $SHELL",
                                    autoComplete: "off"
                                }),
                                recentCommands.length > 0 && !command && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: suggestions,
                                    "data-testid": "new-shell-recent-commands",
                                    children: recentCommands.map((cmd)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                            type: "button",
                                            className: suggestionChip,
                                            "data-testid": "new-shell-recent-command",
                                            onClick: ()=>setCommand(cmd),
                                            children: cmd
                                        }, cmd))
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: fieldGroup,
                            style: {
                                marginTop: "12px"
                            },
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                    htmlFor: "shell-working-dir",
                                    className: label,
                                    children: "Working Directory (optional)"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)(RepoPathInput/* RepoPathInput */.E, {
                                    id: "shell-working-dir",
                                    value: workingDir,
                                    onChange: setWorkingDir,
                                    placeholder: "Defaults to session directory"
                                })
                            ]
                        }),
                        error && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                            className: NewShellDialog_css_errorText,
                            style: {
                                marginTop: "8px"
                            },
                            children: error
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: NewShellDialog_css_actions,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "button",
                                    className: NewShellDialog_css_cancelButton,
                                    onClick: onCancel,
                                    disabled: isSubmitting,
                                    children: "Cancel"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "submit",
                                    className: submitButton,
                                    disabled: isSubmitting,
                                    children: isSubmitting ? "Spawning..." : "Spawn Shell"
                                })
                            ]
                        })
                    ]
                })
            ]
        })
    });
    return /*#__PURE__*/ (0,react_dom.createPortal)(content, document.body);
}

// EXTERNAL MODULE: ./src/lib/hooks/useWorkflows.ts + 1 modules
var useWorkflows = __webpack_require__(79623);
// EXTERNAL MODULE: ./src/components/sessions/TriggersPanel.css.ts
var TriggersPanel_css = __webpack_require__(61823);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FSessionDetail.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-1a64rjNhj9v08hCoUERsHK5DLxUNjXKKUU2ZYTbWTLKyuXmTLvXiTLjiRfMzNlKVtYyKyty3f_jo789UheUoEzUoK_0JWdLzQIwN9fAEgFz_QfAPACx1S-hCB41v-XAudlykUWVn8yLMnvs1Vxnav3b18AkNyfioamBmbi25dFLQTSCyS0LBh-CUHKyFWNUb8woYLEkvI8BDFnpyxXbw6E7g8yBDFm8eyMxQzCMyWXggsJq3cPAAVBcj7MAQTVgAPBCRHm9VwLkdEc1ktpdfmZiJTxSwgONEmI3ivC8XEv-ClPQrPSX2hHXpfiFSXzZ1uNpVajXnBYLGfiY7f-mNF9DqkkWRmCmOSSCPX426mUNH2BMc8lyWUIygLHBEZEXkglcoGThOb7EASLx-1akAwEC_2r9eFC2SHiUvIsBKi4gpIzmnjKkfmw8lv9vuAlrZxTShofX551PBTGnK-Q5gm5hgAFjT_Lg6D50QywHbAKiqtnTs9AllpLVytr1koPzbDY09zskvJcwpK-EjVz99hYQj-_mO03gR4bc8aFr2sVsl3BIclVwtsLwhgtSlqqV5cDlQRq34Qg5xeBiz71KpkdMZ_amq3dKKE5ozmBI8Fi2SzY1FZ79INB4ISeyhDsdrtdcfVs5triEc_7bNfhX-0GyEgqQ3ff3riK571uWM5vIg8HrmW1zV25tcdFE17OMlu9jC22LmkFFiSXtlg5z4lnQLRARu34JEqlV8FpveOAph3xfrOgdn6dO7rgaoFoUyq5UPPKB0to_aDD7Z6HN01xvNBEHkLwuKyCwk7X-tm7q1b9yjZyeFC5pE09kIj9wfPk-v7pA74f2iabj3h8PGXWptxMDIjvfQExLRy8EHi4tWYQLNDEkLANazmq30zFvJGjhQIg0gjCWRPHkp6JXrQfOlgzdn7Z7DT7tARDHbVxMMymJVFnEwDgVBIBS8JILOv4sfTCP12tcVIj4iz5QQUIf3IBwmFCSxwxkuglb9h68Wi7KOcSYsb4hSTO9OheXDxU2ZAHr-I26NjUodMfJxm-1k7ZGgd8Bi7qirHhJEl-uiT5AfmQjOUDGksIhNyMqMr77dggCMOq6nfhR2ta2p0Jn3SW2XjetfqqAQWoQQUDQu5dIVshe3gPHLkFmBV93We6W6wPpMVAePd30ugeANMb9pWzbo86PLhsPOiJ3R0xY2esw5R67oxPeXwq4ZmWNGJVrPKTVKlty9aFBs0wyNO0JDIEcOnVW9ovhY0njSlg70BryW9-CUc9tciac9RzBgawalGmz-2DLAl8CQE-Se4EZy3C5K5lbZ15x_zu7mJNyEdlbXQMgl_vEgsAnNMMVw9vbJmKW0BwSSA_SRBxeXAF-iPBEsMKyf72S4pZSX75c6QucFftpbGg7qG6ONhlYTR_5QsjIaASMxo72xSuGHtBNcxSv1CSrFAYG1bqlyrNCoLlTPkXplQ-KNNm-Dp7DILi-gBQKioOrcIWRmSnZSv5HQG-3wtnTAnsKIATQPp2AkjfvodREJOOHZPJpcp32tnWqedUFETEuNRljhEplWEVfDRcztrjJ8oRoaL-s1DVwi9cJDASBB9DoH8UIu2r-nf2sBabIu8Fbx1txgdjz30UmoO7N36HMu9MQ7JltHqHzWxbQ07vaei1UAbKVYXJWvTs17QuNrdD4U6Hj4X2cfJJdgDiDZyzkYUcUpxRponLAxHULQ3nqvG6DbdBMxZw8Ddgbm5efKK4sdhm2QuWJmjfEX93oSVTAO7lZEYYqF6qyLbIv0DVXO6maq7_Wb9MaTK9dd22wCQ_PA2XaWu9l0_iBnqPgW3sZIOrZRBUx01T1dqnlHfCGEvFV7-nLRcdFX3tzEHB4P0Psk9yd17_3LbculuiwS2nN-IWw7KqjezCdV_l5YeokMYe_3Mhg1wIWn4yOYh6rn1H8PDKqFtdrVWXrW2Ehcw9qGWUtTGKxXOgTfXIzlF1WecHiRk2CUx_CNQN2HWqL3trOfIAHlpNLsnuLKv5TZ-2HiHR7LHVnWVPP9QeH2a66lL0VFw9igptextxi0waYdM-hBkH-mbw3sbe7_i15_jtuxz_NDLecKED93euR521uw4bwWLnjtqNSDCCCocE2E2GifYs4gu8ep5wrYFSjwTGUcnZSeoor8LbIA5hmJym73tflLhFcNlBFPTizI9F7rQcGYjvMfrAQgEZzWfLJ8267ILzxeyvSGicKF4s0MykynWxj_AseADm3wKtu3PJ9oVHWpsOqBPma0YSikEZC0JygPMEzCyxntSnInM92_tOx3HAyqsupr8pqta-_mlfv3eQeW_2Xiuzl_t1j7NLCBBADUXpoIb2ept6vdZXLM6wtP7YrSZBr6G1g3OHYKlSKVf9XcaCMxZhUctSF3h3nzCEFxIdqYTNBLO1xyR6E_f-qIjx-GjE60w5-5MtFVrNZUr1QFgeaGiLZVC_NlEFyVnFzU0oN4kUIsQC7lXWkVzOJK_WfbBBwkPrXmbeUu_QDrE6w2--t0PCiryeq0fvO7RVrVkLcfRiDkfCb12BeSOjnLGsrU33wMIM7GFrUSqcLOpe5Ht7t1tZdEYi3B7qf3LSm8mCX-At4N11IyvL_PTsMdiA37qungcvn0dqAIqnFQGUtMzj5vtdF5NvX97-AeHIFHyaKwAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FSessionDetail_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_1a64rjNhj9v08hCoUERsHK5DLxUNjXKKUU2ZYTbWTLKyuXmTLvXiTLjiRfMzNlKVtYyKyty3f_jo789UheUoEzUoK_0JWdLzQIwN9fAEgFz_QfAPACx1S_hCB41v_XAudlykUWVn8yLMnvs1Vxnav3b18AkNyfioamBmbi25dFLQTSCyS0LBh_CUHKyFWNUb8woYLEkvI8BDFnpyxXbw6E7g8yBDFm8eyMxQzCMyWXggsJq3cPAAVBcj7MAQTVgAPBCRHm9VwLkdEc1ktpdfmZiJTxSwgONEmI3ivC8XEv_ClPQrPSX2hHXpfiFSXzZ1uNpVajXnBYLGfiY7f_mNF9DqkkWRmCmOSSCPX426mUNH2BMc8lyWUIygLHBEZEXkglcoGThOb7EASLx_1akAwEC_2r9eFC2SHiUvIsBKi4gpIzmnjKkfmw8lv9vuAlrZxTShofX551PBTGnK_Q5gm5hgAFjT_Lg6D50QywHbAKiqtnTs9AllpLVytr1koPzbDY09zskvJcwpK_EjVz99hYQj_mO03gR4bc8aFr2sVsl3BIclVwtsLwhgtSlqqV5cDlQRq34Qg5xeBiz71KpkdMZ_amq3dKKE5ozmBI8Fi2SzY1FZ79INB4ISeyhDsdrtdcfVs5triEc_7bNfhX_0GyEgqQ3ff3riK571uWM5vIg8HrmW1zV25tcdFE17OMlu9jC22LmkFFiSXtlg5z4lnQLRARu34JEqlV8FpveOAph3xfrOgdn6dO7rgaoFoUyq5UPPKB0to_aDD7Z6HN01xvNBEHkLwuKyCwk7X_tm7q1b9yjZyeFC5pE09kIj9wfPk_v7pA74f2iabj3h8PGXWptxMDIjvfQExLRy8EHi4tWYQLNDEkLANazmq30zFvJGjhQIg0gjCWRPHkp6JXrQfOlgzdn7Z7DT7tARDHbVxMMymJVFnEwDgVBIBS8JILOv4sfTCP12tcVIj4iz5QQUIf3IBwmFCSxwxkuglb9h68Wi7KOcSYsb4hSTO9OheXDxU2ZAHr_I26NjUodMfJxm_1k7ZGgd8Bi7qirHhJEl_uiT5AfmQjOUDGksIhNyMqMr77dggCMOq6nfhR2ta2p0Jn3SW2XjetfqqAQWoQQUDQu5dIVshe3gPHLkFmBV93We6W6wPpMVAePd30ugeANMb9pWzbo86PLhsPOiJ3R0xY2esw5R67oxPeXwq4ZmWNGJVrPKTVKlty9aFBs0wyNO0JDIEcOnVW9ovhY0njSlg70BryW9_CUc9tciac9RzBgawalGmz_2DLAl8CQE_Se4EZy3C5K5lbZ15x_zu7mJNyEdlbXQMgl_vEgsAnNMMVw9vbJmKW0BwSSA_SRBxeXAF_iPBEsMKyf72S4pZSX75c6QucFftpbGg7qG6ONhlYTR_5QsjIaASMxo72xSuGHtBNcxSv1CSrFAYG1bqlyrNCoLlTPkXplQ_KNNm_Dp7DILi_gBQKioOrcIWRmSnZSv5HQG_3wtnTAnsKIATQPp2AkjfvodREJOOHZPJpcp32tnWqedUFETEuNRljhEplWEVfDRcztrjJ8oRoaL_s1DVwi9cJDASBB9DoH8UIu2r_nf2sBabIu8Fbx1txgdjz30UmoO7N36HMu9MQ7JltHqHzWxbQ07vaei1UAbKVYXJWvTs17QuNrdD4U6Hj4X2cfJJdgDiDZyzkYUcUpxRponLAxHULQ3nqvG6DbdBMxZw8Ddgbm5efKK4sdhm2QuWJmjfEX93oSVTAO7lZEYYqF6qyLbIv0DVXO6maq7_Wb9MaTK9dd22wCQ_PA2XaWu9l0_iBnqPgW3sZIOrZRBUx01T1dqnlHfCGEvFV7_nLRcdFX3tzEHB4P0Psk9yd17_3LbculuiwS2nN_IWw7KqjezCdV_l5YeokMYe_3Mhg1wIWn4yOYh6rn1H8PDKqFtdrVWXrW2Ehcw9qGWUtTGKxXOgTfXIzlF1WecHiRk2CUx_CNQN2HWqL3trOfIAHlpNLsnuLKv5TZ_2HiHR7LHVnWVPP9QeH2a66lL0VFw9igptextxi0waYdM_hBkH_mbw3sbe7_i15_jtuxz_NDLecKED93euR521uw4bwWLnjtqNSDCCCocE2E2GifYs4gu8ep5wrYFSjwTGUcnZSeoor8LbIA5hmJym73tflLhFcNlBFPTizI9F7rQcGYjvMfrAQgEZzWfLJ8267ILzxeyvSGicKF4s0MykynWxj_AseADm3wKtu3PJ9oVHWpsOqBPma0YSikEZC0JygPMEzCyxntSnInM92_tOx3HAyqsupr8pqta_mlfv3eQeW_2Xiuzl_t1j7NLCBBADUXpoIb2ept6vdZXLM6wtP7YrSZBr6G1g3OHYKlSKVf9XcaCMxZhUctSF3h3nzCEFxIdqYTNBLO1xyR6E_f_qIjx_GjE60w5_5MtFVrNZUr1QFgeaGiLZVC_NlEFyVnFzU0oN4kUIsQC7lXWkVzOJK_WfbBBwkPrXmbeUu_QDrE6w2_t0PCiryeq0fvO7RVrVkLcfRiDkfCb12BeSOjnLGsrU33wMIM7GFrUSqcLOpe5Ht7t1tZdEYi3B7qf3LSm8mCX_At4N11IyvL_PTsMdiA37qungcvn0dqAIqnFQGUtMzj5vtdF5NvX97_AeHIFHyaKwAA_22_7D = __webpack_require__(24783);
;// ./src/components/sessions/SessionDetail.css.ts


var SessionDetail_css_actionButton = '_1xlvwi017';
var actionButtonDanger = '_1xlvwi018';
var actionButtonSave = '_1xlvwi019';
var actionDivider = '_1xlvwi016';
var actionSheet = '_1xlvwi013';
var actionSheetItem = '_1xlvwi014';
var actionSheetItemDestructive = '_1xlvwi015';
var active = '_1xlvwi0i';
var SessionDetail_css_cancelButton = '_1xlvwi0x';
var SessionDetail_css_closeButton = '_1xlvwi0d';
var SessionDetail_css_container = '_1xlvwi01';
var SessionDetail_css_content = '_1xlvwi0l';
var SessionDetail_css_disabledTabHint = '_1xlvwi01f';
var SessionDetail_css_editButton = '_1xlvwi0t';
var editContainer = '_1xlvwi0u';
var editInput = '_1xlvwi0v';
var fullscreen = '_1xlvwi02';
var fullscreenButton = '_1xlvwi07';
var fullscreenContent = '_1xlvwi0m';
var fullscreenMobileHeader = '_1xlvwi01a';
var fullscreenMobileHeaderActions = '_1xlvwi01c';
var fullscreenMobileTabs = '_1xlvwi01d';
var fullscreenMobileTitle = '_1xlvwi01b';
var SessionDetail_css_header = '_1xlvwi03';
var headerActions = '_1xlvwi06';
var infoGrid = '_1xlvwi0p';
var infoItem = '_1xlvwi0q';
var infoLabel = '_1xlvwi0r';
var infoValue = '_1xlvwi0s';
var moreActionsButton = '_1xlvwi012';
var SessionDetail_css_navButton = '_1xlvwi0a';
var navButtonWithLabel = '_1xlvwi0b';
var navSessionLabel = '_1xlvwi0c';
var noTerminalIcon = '_1xlvwi0z';
var noTerminalPlaceholder = '_1xlvwi0y';
var noTerminalSubtext = '_1xlvwi011';
var noTerminalText = '_1xlvwi010';
var placeholder = '_1xlvwi0o';
var SessionDetail_css_queuePosition = '_1xlvwi09';
var SessionDetail_css_saveButton = '_1xlvwi0w';
var statusBadge = '_1xlvwi05';
var switchWorkspaceButton = '_1xlvwi08';
var SessionDetail_css_tab = '_1xlvwi0h';
var tabContent = '_1xlvwi0n';
var tabDisabled = '_1xlvwi01e';
var tabIcon = '_1xlvwi0j';
var SessionDetail_css_tabLabel = '_1xlvwi0k';
var SessionDetail_css_tabs = '_1xlvwi0f';
var tabsFade = '_1xlvwi0g';
var tabsWrapper = '_1xlvwi0e';
var SessionDetail_css_title = '_1xlvwi04';
var workflowSection = '_1xlvwi01g';
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FSessionDetailView.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA51U21KsMBB836_Io_sQa2G9lPFjrJAMMBoymMuKWP67lajnLCy4lk9Q6U7PTE_D5UNxje2-qHfsbcOYIkNOsIN0F5w_FHcwlm6M2_vN--bym1msM7sps8zMnjwGJCuYrDyZGOB-wxhaD0GwXXrX6HsjXwWrDQzpID25Rgfq86IiEzubEGmwsRwDdF4wBTaAS8eP0QesX7kiG8CGY6iR_bzNfbFNSCXVU-MoWj0nyIyPHK2GQbAid1mR0-C4kxqjP5G8nY6-z6PXZAP3OMIJHXKFRRPHqdLV35WKcip1fUZKZ6mMvwA2bThhtOvFdhnqpGvQ5r0eFb45U7j6YYa5LGMBhsBzEI733MmBv6AOrWD7m10_TBq4zQ38yxlagxb4d9xWQrWQnHLM7fRSa7TNAjqfrDyTtG598uft71L3k7Hqlxv9rCKYJZu_ThWdTx31hP_9TSvggZZNOTZbtHQAly1fn9zOL9WkoucH9FgZyJcphrQnwcp-YJ4M6kX3vmic6jr_U8q8_A_7OC8N2gQAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FSessionDetailView_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA51U21KsMBB836_Io_sQa2G9lPFjrJAMMBoymMuKWP67lajnLCy4lk9Q6U7PTE_D5UNxje2_qHfsbcOYIkNOsIN0F5w_FHcwlm6M2_vN_bym1msM7sps8zMnjwGJCuYrDyZGOB_wxhaD0GwXXrX6HsjXwWrDQzpID25Rgfq86IiEzubEGmwsRwDdF4wBTaAS8eP0QesX7kiG8CGY6iR_bzNfbFNSCXVU_MoWj0nyIyPHK2GQbAid1mR0_C4kxqjP5G8nY6_z6PXZAP3OMIJHXKFRRPHqdLV35WKcip1fUZKZ6mMvwA2bThhtOvFdhnqpGvQ5r0eFb45U7j6YYa5LGMBhsBzEI733MmBv6AOrWD7m10_TBq4zQ38yxlagxb4d9xWQrWQnHLM7fRSa7TNAjqfrDyTtG598uft71L3k7Hqlxv9rCKYJZu_ThWdTx31hP_9TSvggZZNOTZbtHQAly1fn9zOL9WkoucH9FgZyJcphrQnwcp_YJ4M6kX3vmic6jr_U8q8_A_7OC8N2gQAAA_22_7D = __webpack_require__(83411);
;// ./src/components/sessions/SessionDetailView.css.ts


var crashedOverlayIcon = '_15ih31f4';
var diffAdded = '_15ih31f0';
var pausedOverlay = '_15ih31f2';
var pausedOverlayButton = '_15ih31f7';
var pausedOverlayIcon = '_15ih31f3';
var pausedOverlayReason = '_15ih31f6';
var pausedOverlayTitle = '_15ih31f5';
var restartedFromLink = '_15ih31f1';
// EXTERNAL MODULE: ./src/lib/sessions/formatPauseReason.ts
var formatPauseReason = __webpack_require__(31205);
;// ./src/components/sessions/SessionDetailView.tsx
/* __next_internal_client_entry_do_not_use__ ResyncStaggerQueue,useResyncStaggerQueue,SessionDetailView auto */ 
// +feature: session-trigger-attribution-badge













































// Epic 6.1 (terminal:resync-stagger), Task 6.1.1.1/6.1.1.5 — stagger queue
// spreading simultaneous multi-instance visibility/focus resyncs (e.g. a
// pooled tmux session with several `TerminalOutput` instances all becoming
// visible within the same tick) across a small jitter window instead of
// firing every `CurrentPaneRequest` in the same event-loop turn.
//
// Deliberately scoped *per-SessionDetailView instance*, not cross-session —
// resolved engineering-triad decision (plan.md Task 6.1.1.5): (1) the
// primary reported failure mode is single-session multi-instance resync,
// which this queue fixes; (2) a cross-session synchronized burst is a
// narrower/rarer case requiring multiple sessions open, sharing a tmux
// socket, AND becoming visible at the same tab-focus instant; (3) a
// cross-session coordinator would need a materially different architecture
// (global mutable scheduling state instead of a component-local queue).
// See `staggerCoordinator_should_NotCoordinateAcrossMultipleSessionDetailViews_When_TwoSessionsBecomeVisibleSimultaneously`
// in the test file for a passing regression test documenting this known gap
// — it should be filed as its own tracked backlog follow-up, not silently
// left undocumented.
//
// Interface Pollution checklist #4 (forwarding-only wrapper): kept as a
// plain class co-located here rather than split into its own file, since
// its only consumer is this component.
const RESYNC_STAGGER_JITTER_MAX_MS = 300;
class ResyncStaggerQueue {
    /**
   * Reconfigures batching on a live queue instead of requiring a fresh
   * instance. `terminal:resync-batching` is a `useFeatureFlag` value (see
   * `FeatureFlagsContext.tsx`), which can flip while a `SessionDetailView` is
   * already mounted (e.g. toggled from `/settings/features` without a
   * remount) — `useResyncStaggerQueue` used to bake `batchingEnabled` into
   * the constructor only, so a live toggle was silently ignored for the rest
   * of that view's lifetime. `schedule()`/`scheduleBatched()` read
   * `this.batchingEnabled` fresh on every call, so flipping this field takes
   * effect on the next `schedule()` call — it never touches entries already
   * queued under the old mode, so nothing in flight is dropped or
   * duplicated. */ setBatchingEnabled(batchingEnabled) {
        this.batchingEnabled = batchingEnabled;
    }
    /** Enqueues `fire` with a random 0-300ms jitter delay, unless `preempt` is
   * set — in which case it fires immediately (Task 6.1.1.3: "newly-focused
   * preempts queued"), without clearing other already-queued entries. */ schedule(instanceId, fire, opts) {
        this.clear(instanceId);
        if (opts.preempt) {
            fire();
            return;
        }
        if (this.batchingEnabled) {
            this.scheduleBatched(instanceId, fire);
            return;
        }
        const delay = Math.random() * RESYNC_STAGGER_JITTER_MAX_MS;
        const timer = setTimeout(()=>{
            this.entries.delete(instanceId);
            fire();
        }, delay);
        this.entries.set(instanceId, {
            instanceId,
            fire,
            timer
        });
        // Task 7.1.1.1 (Epic 7.1 observability) — this.entries.size right after
        // insertion is the current burst size: how many resync requests are
        // queued together (across every instance sharing this per-view queue) at
        // the moment a new one joins. Logged on every enqueue rather than only
        // once per burst so the size is visible mid-burst, not just at its start.
        console.debug("[resync-stagger] burst size=".concat(this.entries.size, " instanceId=").concat(instanceId));
    }
    /**
   * Task 5.2.1.3 — client-side half of the `terminal:resync-batching` go/no-go
   * (ADR-006; requirements.md Unresolved Question #1). All non-preempt
   * entries scheduled while a coalescing window is open share that window's
   * single timer, so they fire together in one synchronous pass instead of
   * being spread across independent per-instance jitter delays — the
   * "coalesce same-tick resync requests" behavior Story 5.2.1 asks for.
   *
   * Known limitation, intentionally left for a follow-up: `fire` here is
   * still the same opaque `() => void` used by the non-batching path, and
   * each instance's `fire` independently constructs and sends its own
   * `CurrentPaneRequest` deep inside `useTerminalFlowControl`/
   * `useTerminalStream` (outside Epic 5.2's file scope, which is limited to
   * this stagger coordinator plus the proto/server-side dispatch in
   * `connectrpc_websocket.go`). So today this coalesces *when* sibling
   * resyncs fire, not yet *how many wire messages* go out — turning a batch
   * into one actual `BatchedCurrentPaneRequest` send requires those hooks to
   * expose a non-side-effecting "build the request, don't send it" path that
   * this queue can collect into a single batched send. That wiring, and the
   * decision to default this flag on, are both deferred pending Epic 5.1's
   * compression benchmark numbers, per ADR-006 and requirements.md's
   * Unresolved Question #1 — not resolved by this story.
   */ scheduleBatched(instanceId, fire) {
        if (this.batchTimer === null) {
            this.batchTimer = setTimeout(()=>{
                const fires = Array.from(this.batchEntries.values());
                this.batchEntries.clear();
                this.batchTimer = null;
                for (const pending of fires){
                    this.entries.delete(pending.instanceId);
                    pending.fire();
                }
            }, Math.random() * RESYNC_STAGGER_JITTER_MAX_MS);
        }
        const entry = {
            instanceId,
            fire,
            timer: this.batchTimer
        };
        this.entries.set(instanceId, entry);
        this.batchEntries.set(instanceId, entry);
        // Task 7.1.1.1 (Epic 7.1 observability) — batching-path counterpart to the
        // non-batching log in schedule() above: this.batchEntries.size is the
        // burst that will actually fire together in one synchronous pass once the
        // shared coalescing timer elapses.
        console.debug("[resync-stagger] burst size=".concat(this.batchEntries.size, " instanceId=").concat(instanceId, " batched=true"));
    }
    /** Cancels a single instance's pending entry, if any (e.g. re-scheduling,
   * or that instance unmounting while still queued). A batched entry's timer
   * is shared with its whole coalescing window, so it's only cleared once
   * every sibling in that window has also been removed — clearing one
   * instance's entry must never cancel its still-pending siblings' fire. */ clear(instanceId) {
        const existing = this.entries.get(instanceId);
        if (!existing) return;
        this.entries.delete(instanceId);
        const wasBatched = this.batchEntries.delete(instanceId);
        if (wasBatched) {
            // Only cancel the shared timer once every sibling sharing it has also
            // been cleared — otherwise the remaining siblings would never fire.
            if (this.batchEntries.size === 0) {
                clearTimeout(existing.timer);
                this.batchTimer = null;
            }
        } else {
            clearTimeout(existing.timer);
        }
    }
    /** Task 6.1.1.6 — cancels every pending entry, e.g. on SessionDetailView
   * unmount, so no stagger callback fires after the component is gone. */ clearAll() {
        for (const entry of this.entries.values()){
            clearTimeout(entry.timer);
        }
        this.entries.clear();
        this.batchEntries.clear();
        this.batchTimer = null;
    }
    /**
   * Epic 5.2 (`terminal:resync-batching`, default off — go/no-go
   * deliberately deferred, see the doc comment above `schedule()`'s batching
   * branch below). When true, entries scheduled without `preempt` share one
   * timer per coalescing window instead of each getting its own independent
   * jittered timer, so simultaneously-queued resyncs fire together in one
   * synchronous batch rather than spread across the jitter window. Defaults
   * to false so `terminal:resync-stagger`-only callers (batching flag off)
   * are byte-for-byte unaffected — see
   * `staggerCoordinator_should_SendNSeparateRequests_When_BatchingFlagOff`.
   */ constructor(batchingEnabled = false){
        this.batchingEnabled = batchingEnabled;
        this.entries = new Map();
        this.batchTimer = null;
        this.batchEntries = new Map();
    }
}
/**
 * Task 6.1.1.1/6.1.1.2 — builds a per-instance `scheduleResync` callback
 * factory, gated by the `terminal:resync-stagger` flag. Flag off returns
 * `undefined` for every instance, so `TerminalOutput`/`useVisibilityResync`
 * see no `scheduleResync` at all and fall back to their pre-Epic-6.1
 * synchronous-fire behavior (AC7 flag-off parity).
 *
 * `batchingEnabled` (Task 5.2.1.3, `terminal:resync-batching`, default off)
 * additionally coalesces same-tick entries onto one shared timer instead of
 * independent per-instance jitter timers — see `ResyncStaggerQueue`'s
 * constructor and `scheduleBatched` doc comments for exactly what this does
 * and does not yet cover, and `staggerCoordinator_should_SendNSeparateRequests_When_BatchingFlagOff`
 * for the flag-off parity this must never regress (AC6b).
 */ function useResyncStaggerQueue(enabled) {
    let batchingEnabled = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : false;
    const queueRef = (0,react.useRef)(null);
    if (enabled && !queueRef.current) {
        queueRef.current = new ResyncStaggerQueue(batchingEnabled);
    }
    // `batchingEnabled` comes from `useFeatureFlag("terminal:resync-batching")`,
    // which can change while this component stays mounted (e.g. toggled live
    // from /settings/features) — the lazy construction above only reads it
    // once, at first-enable time. Keep the live queue's batching mode in sync
    // on every render where the flag value has changed, instead of silently
    // running with whatever `batchingEnabled` happened to be at construction.
    (0,react.useEffect)(()=>{
        var _queueRef_current;
        (_queueRef_current = queueRef.current) === null || _queueRef_current === void 0 ? void 0 : _queueRef_current.setBatchingEnabled(batchingEnabled);
    }, [
        batchingEnabled
    ]);
    // Task 6.1.1.6 — clear every pending stagger timeout on unmount (and when
    // the flag flips off), so no queued resync fires after the component
    // holding the queue is gone.
    (0,react.useEffect)(()=>{
        return ()=>{
            var _queueRef_current;
            (_queueRef_current = queueRef.current) === null || _queueRef_current === void 0 ? void 0 : _queueRef_current.clearAll();
        };
    }, []);
    return (0,react.useCallback)((instanceId)=>{
        if (!enabled || !queueRef.current) return undefined;
        const queue = queueRef.current;
        return (fire, opts)=>{
            queue.schedule(instanceId, fire, opts);
        };
    }, [
        enabled
    ]);
}
// Dynamically import TerminalOutput with SSR disabled (xterm.js requires browser environment)
const TerminalOutput = (0,lazy/* lazyComponent */._)(()=>Promise.all(/* import() */[__webpack_require__.e(1869), __webpack_require__.e(5857), __webpack_require__.e(6262)]).then(__webpack_require__.bind(__webpack_require__, 36262)).then((mod)=>({
            default: mod.TerminalOutput
        })), {
    loading: ()=>/*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            style: {
                padding: "20px",
                textAlign: "center"
            },
            children: "Loading terminal..."
        })
});
// Terminal per the SessionStatus doc comment: "Session has been stopped
// (terminal state, cannot transition further)." Used to gate the Summary
// tab, which only has content once the session has ended.
function isSessionTerminal(status) {
    // PermanentlyFailed is terminal-like (session-retry-backoff): the retry
    // policy has given up for this episode, so the Summary tab's content is
    // just as final as a plain Stopped session's, until "Retry now" re-arms it.
    return status === types_pb/* SessionStatus */.Ph.STOPPED || status === types_pb/* SessionStatus */.Ph.PERMANENTLY_FAILED;
}
function getStatusLabel(status) {
    switch(status){
        case types_pb/* SessionStatus */.Ph.RUNNING:
            return "Running";
        case types_pb/* SessionStatus */.Ph.READY:
            return "Ready";
        case types_pb/* SessionStatus */.Ph.LOADING:
            return "Loading";
        case types_pb/* SessionStatus */.Ph.PAUSED:
            return "Paused";
        case types_pb/* SessionStatus */.Ph.NEEDS_APPROVAL:
            return "Needs Approval";
        case types_pb/* SessionStatus */.Ph.CREATING:
            return "Creating";
        case types_pb/* SessionStatus */.Ph.STOPPED:
            return "Stopped";
        case types_pb/* SessionStatus */.Ph.PERMANENTLY_FAILED:
            return "Failed";
        default:
            return "Unknown";
    }
}
function getSessionTypeLabel(type) {
    switch(type){
        case types_pb/* SessionType */.dF.DIRECTORY:
            return "Directory";
        case types_pb/* SessionType */.dF.NEW_WORKTREE:
            return "New Worktree";
        case types_pb/* SessionType */.dF.EXISTING_WORKTREE:
            return "Existing Worktree";
        default:
            return "Unknown";
    }
}
const POOL_PANE_BASE = {
    position: "absolute",
    top: 0,
    left: 0,
    right: 0,
    bottom: 0
};
function SessionDetailView(param) {
    let { session, allSessions, actions, onClose, onFullscreenChange, onTabChange, initialTab = "info", embedded = false, onNext, onPrevious, showNavigation = false, onApprovalResolved: _onApprovalResolved, onDismissFromQueue, queuePosition, queueTotal, nextSessionName, previousSessionName, onBack, canGoBack, backlogItemId, backlogEntry, isActive = true } = param;
    var _session_externalMetadata, _session_externalMetadata1, _session_externalMetadata2, _session_claudeSession, _session_claudeSession1, _session_goal;
    // activeTabId is either a static SessionDetailTab or a shell tab id "shell:<shellId>"
    const [activeTabId, setActiveTabId] = (0,react.useState)(initialTab);
    // Epic 6.1 (terminal:resync-stagger), Task 6.1.1.1/6.1.1.2 — per-view
    // stagger queue for this SessionDetailView's pooled TerminalOutput
    // instances. See ResyncStaggerQueue above for why this is scoped
    // per-instance rather than cross-session.
    const resyncStaggerEnabled = (0,FeatureFlagsContext/* useFeatureFlag */.u)("terminal:resync-stagger");
    const resyncBatchingEnabled = (0,FeatureFlagsContext/* useFeatureFlag */.u)("terminal:resync-batching");
    const getScheduleResync = useResyncStaggerQueue(resyncStaggerEnabled, resyncBatchingEnabled);
    // Reason text for a disabled tab, surfaced on tap since touch devices have no
    // hover to reveal the `title` attribute. Cleared after a few seconds or on next tap.
    const [disabledTabHint, setDisabledTabHint] = (0,react.useState)(null);
    const disabledTabHintTimeoutRef = (0,react.useRef)(null);
    const showDisabledTabHint = (reason)=>{
        if (disabledTabHintTimeoutRef.current) clearTimeout(disabledTabHintTimeoutRef.current);
        setDisabledTabHint(reason);
        disabledTabHintTimeoutRef.current = setTimeout(()=>setDisabledTabHint(null), 3000);
    };
    (0,react.useEffect)(()=>()=>{
            if (disabledTabHintTimeoutRef.current) clearTimeout(disabledTabHintTimeoutRef.current);
        }, []);
    // Sync active tab when the pane's controlled tab changes (e.g. PaneHeader tab click)
    (0,react.useEffect)(()=>{
        setActiveTabId(initialTab);
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        initialTab
    ]);
    // Backward-compat alias for code that still references activeTab
    const activeTab = activeTabId;
    // Trigger attribution (webhook-triggers Epic 7.4, AC6): resolve session.workflowId
    // to its Workflow row so we can show "Triggered by: {slug} ({trigger_type})" for
    // automated (non-"manual") trigger-created sessions, distinguishing them from a
    // plain @slug manually-invoked Workflow session.
    const { workflows: attributionWorkflows } = (0,useWorkflows/* useWorkflows */.m)();
    const triggerWorkflow = session.workflowId ? attributionWorkflows.find((w)=>w.id === session.workflowId) : undefined;
    const isAutomatedTrigger = !!triggerWorkflow && [
        "cron",
        "github_push",
        "webhook"
    ].includes(triggerWorkflow.triggerType);
    // Restart lineage (context-compression, UX acceptance criterion #10): resolve
    // session.restartedFromSessionId against the live session list so the "Restarted
    // from:" row can link to the source session by title when it's still around, and
    // gracefully fall back to a plain, non-clickable "(no longer available)" label when
    // it's been archived/deleted since the restart — mirrors the trigger-attribution
    // lookup above.
    const restartSourceSession = session.restartedFromSessionId ? allSessions.find((s)=>s.id === session.restartedFromSessionId) : undefined;
    // Shell tabs
    const { shells, spawnShell, stopShell, restartShell, deleteShell, updateShellStatus } = useShells(session.id);
    const [showNewShellDialog, setShowNewShellDialog] = (0,react.useState)(false);
    const { addNotification } = (0,NotificationContext/* useNotifications */.E$)();
    // Tell the notification deck which session is on screen so it can skip redundant non-pinned toasts.
    (0,react.useEffect)(()=>(0,viewedSessions/* markSessionViewed */.O)(session.id), [
        session.id
    ]);
    // Attach the active session UUID to all OTel spans created while this view
    // is mounted, so traces are queryable by session. Cleared on unmount so
    // spans from unrelated pages do not carry a stale session UUID.
    (0,react.useEffect)(()=>{
        (0,active_session/* setActiveSessionUUID */.u)(session.id);
        return ()=>{
            (0,active_session/* setActiveSessionUUID */.u)(null);
        };
    }, [
        session.id
    ]);
    // The mobile toast deck docks directly under this tab row.
    const tabRowRef = (0,react.useRef)(null);
    (0,usePublishStackTopOffset/* usePublishStackTopOffset */.t)(tabRowRef);
    // Fire a toast notification when a shell exits with a non-zero exit code.
    // Track which shell IDs we've already notified to avoid duplicate toasts.
    const notifiedShellExitsRef = (0,react.useRef)(new Set());
    (0,react.useEffect)(()=>{
        for (const shell of shells){
            if (shell.status !== "running" && shell.exitCode != null && shell.exitCode !== 0 && !notifiedShellExitsRef.current.has(shell.id)) {
                notifiedShellExitsRef.current.add(shell.id);
                addNotification({
                    sessionId: session.id,
                    sessionName: session.title,
                    message: 'Shell "'.concat(shell.name || shell.command || "shell", '" exited with code ').concat(shell.exitCode),
                    priority: "medium",
                    notificationType: "error"
                });
            }
        }
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        shells
    ]);
    const [isFullscreen, setIsFullscreen] = (0,react.useState)(false);
    const [copiedField, setCopiedField] = (0,react.useState)(null);
    // The path is remembered with its session so a switch to another session never inherits it.
    const [filesSelection, setFilesSelection] = (0,react.useState)(()=>{
        const hash =  false ? 0 : window.location.hash;
        const path = hash.startsWith("#file=") ? decodeURIComponent(hash.slice(6)) || null : null;
        return {
            sessionId: session.id,
            path
        };
    });
    const filesSelectedPath = filesSelection.sessionId === session.id ? filesSelection.path : null;
    const setFilesSelectedPath = (0,react.useCallback)((path)=>setFilesSelection({
            sessionId: session.id,
            path
        }), [
        session.id
    ]);
    const [showWorkspaceSwitchModal, setShowWorkspaceSwitchModal] = (0,react.useState)(false);
    // The action sheet's own opener buttons unmount when the sheet closes, so
    // any modal launched from the sheet restores focus to the persistent
    // "More actions" button instead of the transient sheet item.
    const moreActionsButtonRef = (0,react.useRef)(null);
    const workspaceSwitchTriggerRef = (0,react.useRef)(null);
    const tagEditorTriggerRef = (0,react.useRef)(null);
    const availablePrograms = (0,useAvailablePrograms/* useAvailablePrograms */.U)();
    const [isEditingProgram, setIsEditingProgram] = (0,react.useState)(false);
    const [programValue, setProgramValue] = (0,react.useState)(session.program || "");
    // Re-sync when server pushes an update (WatchSessions), but only while not actively editing.
    react.useEffect(()=>{
        if (!isEditingProgram) setProgramValue(session.program || "");
    }, [
        session.program,
        isEditingProgram
    ]);
    const [isEditingWorkingDir, setIsEditingWorkingDir] = (0,react.useState)(false);
    const [workingDirValue, setWorkingDirValue] = (0,react.useState)(session.workingDir || "");
    // Action sheet state
    const [actionSheetOpen, setActionSheetOpen] = (0,react.useState)(false);
    const [exportBundleOpen, setExportBundleOpen] = (0,react.useState)(false);
    const [showDeleteConfirm, setShowDeleteConfirm] = (0,react.useState)(false);
    const [showResumeModal, setShowResumeModal] = (0,react.useState)(false);
    const resumeTriggerRef = (0,react.useRef)(null);
    // Rename state
    const [showRenameModal, setShowRenameModal] = (0,react.useState)(false);
    const [renameValue, setRenameValue] = (0,react.useState)(session.title);
    const [renameError, setRenameError] = (0,react.useState)(null);
    // Tag editor state
    const [showTagEditor, setShowTagEditor] = (0,react.useState)(false);
    const [showRestartConfirm, setShowRestartConfirm] = (0,react.useState)(false);
    const [showCheckpointModal, setShowCheckpointModal] = (0,react.useState)(false);
    const [checkpointLabel, setCheckpointLabel] = (0,react.useState)("");
    // Category/Tags inline editing state
    const [isEditingCategory, setIsEditingCategory] = (0,react.useState)(false);
    const [categoryValue, setCategoryValue] = (0,react.useState)(session.category || "");
    const [isEditingTags, setIsEditingTags] = (0,react.useState)(false);
    var _session_tags;
    const [tagsInputValue, setTagsInputValue] = (0,react.useState)(((_session_tags = session.tags) !== null && _session_tags !== void 0 ? _session_tags : []).join(", "));
    // Launch command display state
    const [launchCmdExpanded, setLaunchCmdExpanded] = (0,react.useState)(false);
    const [copiedLaunchCmd, setCopiedLaunchCmd] = (0,react.useState)(false);
    // Measure click-to-render latency when the detail panel first mounts for a session
    (0,react.useEffect)(()=>{
        if (typeof performance === "undefined") return;
        try {
            performance.measure("session:click-to-render", "session:click");
        } catch (e) {
        // mark may be absent (e.g. deep-link navigation with no prior click)
        }
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        session.id
    ]);
    // Terminal instance pool: keeps up to 8 session terminals alive (LRU, oldest first)
    // Pool entries are either session IDs or "shell:<shellId>" strings.
    const [pooledSessionIds, setPooledSessionIds] = (0,react.useState)([]);
    // Pool ids whose session was hidden when it was current, so a pooled terminal stays
    // read-only while another session is in front (Story 5.3).
    const hiddenPoolIdsRef = (0,react.useRef)(new Set());
    if (session.hidden) hiddenPoolIdsRef.current.add(session.id);
    else hiddenPoolIdsRef.current.delete(session.id);
    const [pooledMuxPaths, setPooledMuxPaths] = (0,react.useState)([]);
    // Separate pool for shell PTY terminals, keyed by "shell:<shellId>"
    const [pooledShellKeys, setPooledShellKeys] = (0,react.useState)([]);
    (0,react.useEffect)(()=>{
        setPooledSessionIds((prev)=>{
            if (prev.includes(session.id)) return prev;
            const updated = [
                ...prev,
                session.id
            ];
            if (updated.length > 8) return updated.slice(-8);
            return updated;
        });
    }, [
        session.id
    ]);
    (0,react.useEffect)(()=>{
        var _session_externalMetadata;
        const muxPath = (_session_externalMetadata = session.externalMetadata) === null || _session_externalMetadata === void 0 ? void 0 : _session_externalMetadata.muxSocketPath;
        if (!muxPath) return;
        setPooledMuxPaths((prev)=>{
            if (prev.includes(muxPath)) return prev;
            const updated = [
                ...prev,
                muxPath
            ];
            if (updated.length > 8) return updated.slice(-8);
            return updated;
        });
    }, [
        (_session_externalMetadata = session.externalMetadata) === null || _session_externalMetadata === void 0 ? void 0 : _session_externalMetadata.muxSocketPath
    ]);
    // Add shell terminal to the pool when its tab becomes active
    (0,react.useEffect)(()=>{
        if (!activeTabId.startsWith("shell:")) return;
        const shellKey = activeTabId; // already "shell:<shellId>"
        setPooledShellKeys((prev)=>{
            if (prev.includes(shellKey)) return prev;
            const updated = [
                ...prev,
                shellKey
            ];
            if (updated.length > 8) return updated.slice(-8);
            return updated;
        });
    }, [
        activeTabId
    ]);
    // Notify parent of fullscreen state changes
    (0,react.useEffect)(()=>{
        onFullscreenChange === null || onFullscreenChange === void 0 ? void 0 : onFullscreenChange(isFullscreen);
    }, [
        isFullscreen,
        onFullscreenChange
    ]);
    const vncState = session.vncState;
    var _vncState_status;
    const vncStatus = (_vncState_status = vncState === null || vncState === void 0 ? void 0 : vncState.status) !== null && _vncState_status !== void 0 ? _vncState_status : types_pb/* VNCStatus */.wL.VNC_STATUS_UNSPECIFIED;
    const isBrowserAvailable = vncStatus !== types_pb/* VNCStatus */.wL.VNC_STATUS_UNAVAILABLE && vncStatus !== types_pb/* VNCStatus */.wL.VNC_STATUS_UNSPECIFIED;
    const tabs = [
        {
            id: "terminal",
            label: "Terminal",
            icon: terminal/* default */.A
        },
        {
            id: "diff",
            label: "Diff",
            icon: git_compare/* default */.A
        },
        {
            id: "vcs",
            label: "VCS",
            icon: git_branch/* default */.A
        },
        {
            id: "files",
            label: "Files",
            icon: folder_open/* default */.A
        },
        {
            id: "logs",
            label: "Logs",
            icon: scroll_text/* default */.A
        },
        {
            id: "info",
            label: "Info",
            icon: info/* default */.A
        },
        {
            id: "browser",
            label: "Browser",
            icon: globe/* default */.A,
            disabled: !isBrowserAvailable
        },
        {
            id: "artifacts",
            label: "Artifacts",
            icon: icons_package/* default */.A
        },
        {
            id: "summary",
            label: "Summary",
            icon: file_text/* default */.A,
            disabled: !isSessionTerminal(session.status)
        }
    ];
    const handleTabChange = (tabId)=>{
        setActiveTabId(tabId);
        // Only propagate static tabs to external callers
        if (!tabId.startsWith("shell:")) {
            onTabChange === null || onTabChange === void 0 ? void 0 : onTabChange(tabId);
        }
    };
    const handleOpenNewShellDialog = (0,react.useCallback)(()=>setShowNewShellDialog(true), []);
    (0,useShortcut/* useShortcut */.w)("terminal.spawn-shell", {
        key: "t",
        modifiers: {
            ctrl: true
        },
        label: "Spawn new shell",
        context: "terminal",
        action: handleOpenNewShellDialog
    }, isActive);
    const handleSpawnShell = (0,react.useCallback)(async (params)=>{
        const tab = await spawnShell(params);
        if (tab) {
            setShowNewShellDialog(false);
            handleTabChange("shell:".concat(tab.id));
        }
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        spawnShell
    ]);
    const handleCopy = (field, value)=>{
        navigator.clipboard.writeText(value).then(()=>{
            setCopiedField(field);
            setTimeout(()=>setCopiedField(null), 1500);
        }).catch((err)=>{
            console.warn("[SessionDetailView] clipboard write failed", err);
        });
    };
    // Keyboard shortcuts: Escape to exit fullscreen, Shift+Arrow for navigation
    (0,react.useEffect)(()=>{
        const handleKeyDown = (e)=>{
            if (e.key === "Escape" && isFullscreen) {
                setIsFullscreen(false);
            }
            // Review queue navigation shortcuts (Shift+Arrow)
            if (showNavigation && e.shiftKey && !e.ctrlKey && !e.altKey && !e.metaKey) {
                if (e.key === "ArrowRight" && onNext) {
                    e.preventDefault();
                    onNext();
                } else if (e.key === "ArrowLeft" && onPrevious) {
                    e.preventDefault();
                    onPrevious();
                }
            }
        };
        window.addEventListener("keydown", handleKeyDown);
        return ()=>window.removeEventListener("keydown", handleKeyDown);
    }, [
        isFullscreen,
        showNavigation,
        onNext,
        onPrevious
    ]);
    function makeStringFieldEditor(editValue, originalValue, setValue, setEditing, updateFn) {
        return {
            handleSave: async ()=>{
                if (editValue !== originalValue) await updateFn(editValue);
                setEditing(false);
            },
            handleCancel: ()=>{
                setValue(originalValue);
                setEditing(false);
            }
        };
    }
    const { handleSave: handleSaveProgram, handleCancel: handleCancelProgramEdit } = makeStringFieldEditor(programValue, session.program || "", setProgramValue, setIsEditingProgram, (v)=>actions.update({
            program: v
        }));
    const { handleSave: handleSaveWorkingDir, handleCancel: handleCancelWorkingDirEdit } = makeStringFieldEditor(workingDirValue, session.workingDir || "", setWorkingDirValue, setIsEditingWorkingDir, (v)=>actions.update({
            workingDir: v
        }));
    const { handleSave: handleSaveCategory, handleCancel: handleCancelCategoryEdit } = makeStringFieldEditor(categoryValue, session.category || "", setCategoryValue, setIsEditingCategory, (v)=>actions.update({
            category: v
        }));
    const handleSaveTags = async ()=>{
        const newTags = tagsInputValue.split(",").map((t)=>t.trim()).filter(Boolean);
        var _session_tags;
        const current = (_session_tags = session.tags) !== null && _session_tags !== void 0 ? _session_tags : [];
        if (JSON.stringify(newTags) !== JSON.stringify(current)) {
            await actions.update({
                tags: newTags
            });
        }
        setIsEditingTags(false);
    };
    const handleCancelTagsEdit = ()=>{
        var _session_tags;
        setTagsInputValue(((_session_tags = session.tags) !== null && _session_tags !== void 0 ? _session_tags : []).join(", "));
        setIsEditingTags(false);
    };
    // Action sheet handlers
    const handlePauseResume = async (triggerEl)=>{
        if (session.status === types_pb/* SessionStatus */.Ph.PAUSED) {
            // The action sheet's own opener buttons unmount when the sheet closes, so
            // fall back to the persistent "More actions" button in that case (see
            // tagEditorTriggerRef/workspaceSwitchTriggerRef below for the same pattern).
            resumeTriggerRef.current = actionSheetOpen ? moreActionsButtonRef.current : triggerEl;
            setActionSheetOpen(false);
            setShowResumeModal(true);
        } else {
            await actions.pause();
            setActionSheetOpen(false);
        }
    };
    const handleResumeFromCrash = async ()=>{
        await actions.resumeFromCrash();
    };
    const handleDeleteClick = ()=>{
        if (session.status === types_pb/* SessionStatus */.Ph.RUNNING || session.status === types_pb/* SessionStatus */.Ph.NEEDS_APPROVAL) {
            setShowDeleteConfirm(true);
        } else {
            handleConfirmDelete();
        }
    };
    const handleConfirmDelete = async ()=>{
        setShowDeleteConfirm(false);
        setActionSheetOpen(false);
        await actions.delete();
        onClose();
    };
    const handleRenameSave = async ()=>{
        const trimmed = renameValue.trim();
        if (!trimmed) {
            setRenameError("Session name cannot be empty");
            return;
        }
        setRenameError(null);
        await actions.rename(trimmed);
        setShowRenameModal(false);
    };
    var _session_tags1, _session_note;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: "".concat(SessionDetail_css_container, " ").concat(isFullscreen ? fullscreen : ""),
        children: [
            !embedded && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: "".concat(SessionDetail_css_header, " ").concat(isFullscreen ? fullscreenMobileHeader : ""),
                "data-testid": "session-header",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("h2", {
                        className: "".concat(SessionDetail_css_title, " ").concat(isFullscreen ? fullscreenMobileTitle : ""),
                        title: session.title,
                        "data-testid": "session-header-title",
                        children: [
                            session.title,
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: statusBadge,
                                "data-testid": "session-status-badge",
                                children: getStatusLabel(session.status)
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(BacklogOriginBadge/* BacklogOriginBadge */.J, {
                                entry: backlogEntry,
                                compact: true
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)(ActionBar/* ActionBar */.E, {
                        gap: "sm",
                        justify: "end",
                        scroll: true,
                        className: "".concat(headerActions, " ").concat(isFullscreen ? fullscreenMobileHeaderActions : ""),
                        children: [
                            (activeTab === "terminal" || activeTab === "diff" || activeTab === "vcs") && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: fullscreenButton,
                                onClick: ()=>setIsFullscreen(!isFullscreen),
                                "aria-label": isFullscreen ? "Exit fullscreen" : "Enter fullscreen",
                                title: isFullscreen ? "Exit fullscreen (Esc)" : "Enter fullscreen",
                                children: isFullscreen ? "⊗" : "⛶"
                            }),
                            canGoBack && onBack && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: SessionDetail_css_navButton,
                                onClick: onBack,
                                "aria-label": "Go back to previous session",
                                title: "Back (previously visited)",
                                children: "↩"
                            }),
                            showNavigation && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: navButtonWithLabel,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                className: SessionDetail_css_navButton,
                                                onClick: onPrevious,
                                                "aria-label": previousSessionName ? "Previous session: ".concat(previousSessionName) : "Previous session",
                                                title: previousSessionName ? "Previous: ".concat(previousSessionName, " (Shift+←)") : "Previous session (Shift+←)",
                                                children: "←"
                                            }),
                                            previousSessionName && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: navSessionLabel,
                                                "aria-hidden": "true",
                                                children: previousSessionName
                                            })
                                        ]
                                    }),
                                    queuePosition !== undefined && queuePosition > 0 && queueTotal !== undefined && queueTotal > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                        className: SessionDetail_css_queuePosition,
                                        "aria-live": "polite",
                                        children: [
                                            queuePosition,
                                            " of ",
                                            queueTotal
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: navButtonWithLabel,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                className: SessionDetail_css_navButton,
                                                onClick: onNext,
                                                "aria-label": nextSessionName ? "Next session: ".concat(nextSessionName) : "Next session",
                                                title: nextSessionName ? "Next: ".concat(nextSessionName, " (Shift+→)") : "Next session (Shift+→)",
                                                children: "→"
                                            }),
                                            nextSessionName && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: navSessionLabel,
                                                "aria-hidden": "true",
                                                children: nextSessionName
                                            })
                                        ]
                                    })
                                ]
                            }),
                            onDismissFromQueue && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: SessionDetail_css_navButton,
                                onClick: onDismissFromQueue,
                                "aria-label": "Clear from queue and advance",
                                title: "Clear from queue and advance to next",
                                children: "⏭"
                            }),
                            session.instanceType !== types_pb/* InstanceType */.y9.EXTERNAL && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: switchWorkspaceButton,
                                onClick: (event)=>{
                                    workspaceSwitchTriggerRef.current = event.currentTarget;
                                    setShowWorkspaceSwitchModal(true);
                                },
                                "aria-label": "Switch workspace",
                                title: "Switch branch, bookmark, or worktree",
                                children: "⎇ Switch"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                ref: moreActionsButtonRef,
                                className: moreActionsButton,
                                onClick: ()=>setActionSheetOpen(true),
                                "aria-label": "Session actions",
                                "data-testid": "more-actions-button",
                                children: "⋯"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: SessionDetail_css_closeButton,
                                onClick: onClose,
                                "aria-label": "Close",
                                children: "✕"
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: tabsWrapper,
                ref: tabRowRef,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: "".concat(SessionDetail_css_tabs, " ").concat(isFullscreen ? fullscreenMobileTabs : ""),
                        role: "tablist",
                        onKeyDown: (e)=>{
                            const currentIndex = tabs.findIndex((t)=>t.id === activeTabId);
                            if (e.key === "ArrowRight") {
                                e.preventDefault();
                                const nextIndex = (currentIndex + 1) % tabs.length;
                                handleTabChange(tabs[nextIndex].id);
                                const el = e.currentTarget.querySelectorAll('[role="tab"]')[nextIndex];
                                el === null || el === void 0 ? void 0 : el.focus();
                                el === null || el === void 0 ? void 0 : el.scrollIntoView({
                                    block: "nearest",
                                    inline: "nearest"
                                });
                            } else if (e.key === "ArrowLeft") {
                                e.preventDefault();
                                const prevIndex = (currentIndex - 1 + tabs.length) % tabs.length;
                                handleTabChange(tabs[prevIndex].id);
                                const el = e.currentTarget.querySelectorAll('[role="tab"]')[prevIndex];
                                el === null || el === void 0 ? void 0 : el.focus();
                                el === null || el === void 0 ? void 0 : el.scrollIntoView({
                                    block: "nearest",
                                    inline: "nearest"
                                });
                            }
                        },
                        children: [
                            tabs.map((tab)=>{
                                const Icon = tab.icon;
                                const disabledReason = tab.disabled && tab.id === "browser" ? "Browser passthrough requires Linux with Xvfb, x11vnc, and xdotool" : tab.disabled && tab.id === "summary" ? "Summary is generated after the session ends." : undefined;
                                return /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                    id: "tab-".concat(tab.id),
                                    role: "tab",
                                    "aria-selected": activeTab === tab.id,
                                    "aria-disabled": tab.disabled,
                                    "aria-label": disabledReason ? "".concat(tab.label, ": ").concat(disabledReason) : undefined,
                                    className: "".concat(SessionDetail_css_tab, " ").concat(activeTab === tab.id ? active : "", " ").concat(tab.disabled ? tabDisabled : ""),
                                    onClick: ()=>{
                                        if (tab.disabled) {
                                            // title (hover) is unreachable on touch — surface the reason on tap instead.
                                            if (disabledReason) showDisabledTabHint(disabledReason);
                                            return;
                                        }
                                        handleTabChange(tab.id);
                                    },
                                    title: disabledReason,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: tabIcon,
                                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(Icon, {
                                                size: 16
                                            })
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: SessionDetail_css_tabLabel,
                                            children: tab.label
                                        })
                                    ]
                                }, tab.id);
                            }),
                            shells.map((shell)=>{
                                const shellTabId = "shell:".concat(shell.id);
                                return /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    id: "tab-".concat(shellTabId),
                                    role: "tab",
                                    "aria-selected": activeTabId === shellTabId,
                                    className: "".concat(SessionDetail_css_tab, " ").concat(activeTabId === shellTabId ? active : ""),
                                    onClick: ()=>handleTabChange(shellTabId),
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ShellTabLabel, {
                                        shell: shell,
                                        onStop: (id)=>stopShell(id),
                                        onRestart: (id)=>restartShell(id),
                                        onClose: async (id)=>{
                                            await deleteShell(id);
                                            // If we were viewing this shell, go back to terminal tab
                                            if (activeTabId === "shell:".concat(id)) {
                                                handleTabChange("terminal");
                                            }
                                        }
                                    })
                                }, shellTabId);
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: SessionDetail_css_tab,
                                onClick: ()=>setShowNewShellDialog(true),
                                "aria-label": "Spawn new shell",
                                title: "Spawn new shell",
                                style: {
                                    padding: "0 8px",
                                    fontSize: "16px"
                                },
                                children: "+"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: tabsFade,
                        "data-testid": "tab-scroll-fade",
                        "aria-hidden": "true"
                    }),
                    disabledTabHint && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: SessionDetail_css_disabledTabHint,
                        "data-testid": "disabled-tab-hint",
                        role: "status",
                        "aria-live": "polite",
                        onClick: ()=>setDisabledTabHint(null),
                        children: disabledTabHint
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                style: {
                    display: "flex",
                    flexDirection: "row",
                    flex: 1,
                    minHeight: 0
                },
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: "".concat(SessionDetail_css_content, " ").concat(isFullscreen ? fullscreenContent : ""),
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: tabContent,
                                role: "tabpanel",
                                "aria-labelledby": "tab-terminal",
                                "aria-hidden": activeTab !== "terminal",
                                style: {
                                    display: activeTab === "terminal" ? "flex" : "none"
                                },
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    style: {
                                        flex: 1,
                                        minHeight: 0,
                                        display: "flex",
                                        flexDirection: "column"
                                    },
                                    children: session.instanceType === types_pb/* InstanceType */.y9.EXTERNAL && !((_session_externalMetadata1 = session.externalMetadata) === null || _session_externalMetadata1 === void 0 ? void 0 : _session_externalMetadata1.muxSocketPath) ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: noTerminalPlaceholder,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: noTerminalIcon,
                                                children: "⛓️"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                                className: noTerminalText,
                                                children: "Terminal not available"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                                className: noTerminalSubtext,
                                                children: "This session is running in an external terminal. Use Approve / Deny above to respond to pending requests."
                                            })
                                        ]
                                    }) : session.instanceType === types_pb/* InstanceType */.y9.EXTERNAL && ((_session_externalMetadata2 = session.externalMetadata) === null || _session_externalMetadata2 === void 0 ? void 0 : _session_externalMetadata2.muxSocketPath) ? // External mux sessions cannot be paused; no overlay needed.
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                        style: {
                                            position: 'relative',
                                            flex: 1,
                                            minHeight: 0
                                        },
                                        children: pooledMuxPaths.map((poolPath)=>{
                                            var _session_externalMetadata, _session_externalMetadata1, _session_externalMetadata2, _session_externalMetadata3;
                                            return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                                style: {
                                                    ...POOL_PANE_BASE,
                                                    visibility: poolPath === ((_session_externalMetadata = session.externalMetadata) === null || _session_externalMetadata === void 0 ? void 0 : _session_externalMetadata.muxSocketPath) ? "visible" : "hidden",
                                                    pointerEvents: poolPath === ((_session_externalMetadata1 = session.externalMetadata) === null || _session_externalMetadata1 === void 0 ? void 0 : _session_externalMetadata1.muxSocketPath) ? "auto" : "none"
                                                },
                                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(TerminalOutput, {
                                                    sessionId: poolPath,
                                                    baseUrl: (0,config/* getApiBaseUrl */.h)(),
                                                    isExternal: true,
                                                    tmuxSessionName: (_session_externalMetadata2 = session.externalMetadata) === null || _session_externalMetadata2 === void 0 ? void 0 : _session_externalMetadata2.tmuxSessionName,
                                                    isVisible: poolPath === ((_session_externalMetadata3 = session.externalMetadata) === null || _session_externalMetadata3 === void 0 ? void 0 : _session_externalMetadata3.muxSocketPath),
                                                    scheduleResync: getScheduleResync(poolPath)
                                                })
                                            }, poolPath);
                                        })
                                    }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        style: {
                                            position: 'relative',
                                            flex: 1,
                                            minHeight: 0
                                        },
                                        children: [
                                            pooledSessionIds.map((poolId)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                                    style: {
                                                        ...POOL_PANE_BASE,
                                                        visibility: poolId === session.id ? "visible" : "hidden",
                                                        pointerEvents: poolId === session.id ? "auto" : "none"
                                                    },
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(TerminalOutput, {
                                                        sessionId: poolId,
                                                        baseUrl: (0,config/* getApiBaseUrl */.h)(),
                                                        isVisible: poolId === session.id,
                                                        scheduleResync: getScheduleResync(poolId),
                                                        readOnly: hiddenPoolIdsRef.current.has(poolId)
                                                    })
                                                }, poolId)),
                                            session.status === types_pb/* SessionStatus */.Ph.PAUSED && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: pausedOverlay,
                                                role: "status",
                                                "aria-live": "polite",
                                                "aria-label": "Session is paused",
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: pausedOverlayIcon,
                                                        "aria-hidden": "true",
                                                        children: "⏸"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                                        className: pausedOverlayTitle,
                                                        children: "This session is paused"
                                                    }),
                                                    session.pauseReason && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                                        className: pausedOverlayReason,
                                                        children: (0,formatPauseReason/* formatPauseReason */.w)(session.pauseReason)
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                        className: pausedOverlayButton,
                                                        onClick: (e)=>{
                                                            e.stopPropagation();
                                                            handlePauseResume(e.currentTarget);
                                                        },
                                                        "aria-label": "Resume this session",
                                                        children: "▶ Resume Session"
                                                    })
                                                ]
                                            }),
                                            session.status === types_pb/* SessionStatus */.Ph.CRASHED && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: pausedOverlay,
                                                role: "status",
                                                "aria-live": "polite",
                                                "aria-label": "Session has crashed",
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: crashedOverlayIcon,
                                                        "aria-hidden": "true",
                                                        children: "⚠️"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                                        className: pausedOverlayTitle,
                                                        children: "This session has crashed"
                                                    }),
                                                    session.exitReason && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                                        className: pausedOverlayReason,
                                                        children: session.exitReason
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                        className: pausedOverlayButton,
                                                        onClick: (e)=>{
                                                            e.stopPropagation();
                                                            handleResumeFromCrash();
                                                        },
                                                        "aria-label": "Resume this session",
                                                        children: "▶ Resume Session"
                                                    })
                                                ]
                                            })
                                        ]
                                    })
                                })
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: tabContent,
                                role: "tabpanel",
                                "aria-labelledby": "tab-browser",
                                "aria-hidden": activeTab !== "browser",
                                style: {
                                    display: activeTab === "browser" ? undefined : 'none'
                                },
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(BrowserTab, {
                                    sessionId: session.id,
                                    baseUrl: (0,config/* getApiBaseUrl */.h)(),
                                    isVisible: activeTab === "browser",
                                    vncState: vncState
                                })
                            }),
                            pooledShellKeys.map((shellKey)=>{
                                const shellId = shellKey.replace(/^shell:/, "");
                                const shell = shells.find((s)=>s.id === shellId);
                                return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: tabContent,
                                    role: "tabpanel",
                                    "aria-labelledby": "tab-".concat(shellKey),
                                    "aria-hidden": activeTabId !== shellKey,
                                    style: {
                                        display: activeTabId === shellKey ? "flex" : "none",
                                        flexDirection: "column"
                                    },
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)(TerminalOutput, {
                                            sessionId: session.id,
                                            baseUrl: (0,config/* getApiBaseUrl */.h)(),
                                            shellId: shellId,
                                            isVisible: activeTabId === shellKey,
                                            scheduleResync: getScheduleResync(shellKey),
                                            readOnly: session.hidden,
                                            onShellStatusChange: (status, exitCode)=>{
                                                updateShellStatus(shellId, status, exitCode);
                                            }
                                        }),
                                        shell && shell.status !== "running" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            style: {
                                                padding: "8px 16px",
                                                fontSize: "13px",
                                                color: "var(--text-secondary)"
                                            },
                                            children: [
                                                "Shell ",
                                                shell.status === "stopped" ? "stopped" : "errored",
                                                shell.exitCode !== undefined ? " (exit ".concat(shell.exitCode, ")") : "",
                                                "."
                                            ]
                                        })
                                    ]
                                }, shellKey);
                            }),
                            showNewShellDialog && /*#__PURE__*/ (0,jsx_runtime.jsx)(NewShellDialog, {
                                onSubmit: handleSpawnShell,
                                onCancel: ()=>setShowNewShellDialog(false),
                                defaultWorkingDir: session.workingDir
                            }),
                            activeTab === "diff" && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: tabContent,
                                role: "tabpanel",
                                "aria-labelledby": "tab-diff",
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(DiffViewer/* DiffViewer */.M, {
                                    session: session
                                })
                            }),
                            activeTab === "vcs" && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: tabContent,
                                role: "tabpanel",
                                "aria-labelledby": "tab-vcs",
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(VcsPanel, {
                                    session: session,
                                    onNavigateToFile: (path)=>{
                                        setFilesSelectedPath(path);
                                        handleTabChange("files");
                                    },
                                    onBrowseFiles: ()=>handleTabChange("files")
                                })
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: tabContent,
                                role: "tabpanel",
                                "aria-labelledby": "tab-files",
                                "aria-hidden": activeTab !== "files",
                                "data-active": activeTab === "files",
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(FilesTab, {
                                    sessionId: session.id,
                                    baseUrl: (0,config/* getApiBaseUrl */.h)(),
                                    initialSelectedPath: filesSelectedPath,
                                    onSelectedPathChange: setFilesSelectedPath,
                                    sessionRunning: session.status === types_pb/* SessionStatus */.Ph.RUNNING,
                                    sessionName: session.title,
                                    onResumeSession: session.status === types_pb/* SessionStatus */.Ph.PAUSED ? handlePauseResume : undefined
                                }, session.id)
                            }),
                            activeTab === "logs" && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: tabContent,
                                role: "tabpanel",
                                "aria-labelledby": "tab-logs",
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionLogsTab, {
                                    sessionId: session.id
                                })
                            }),
                            activeTab === "info" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: tabContent,
                                role: "tabpanel",
                                "aria-labelledby": "tab-info",
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: infoGrid,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: infoItem,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoLabel,
                                                        children: "Instance ID:"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                        className: infoValue,
                                                        style: {
                                                            fontFamily: 'monospace',
                                                            fontSize: '0.9em'
                                                        },
                                                        children: [
                                                            session.id,
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                onClick: ()=>handleCopy('instanceId', session.id),
                                                                className: SessionDetail_css_editButton,
                                                                title: "Copy to clipboard",
                                                                children: copiedField === 'instanceId' ? '✓' : '📋'
                                                            })
                                                        ]
                                                    })
                                                ]
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: infoItem,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoLabel,
                                                        children: "Status:"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoValue,
                                                        children: getStatusLabel(session.status)
                                                    })
                                                ]
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: infoItem,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoLabel,
                                                        children: "Session Type:"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoValue,
                                                        children: getSessionTypeLabel(session.sessionType)
                                                    })
                                                ]
                                            }),
                                            session.instanceType === types_pb/* InstanceType */.y9.EXTERNAL && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: infoItem,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoLabel,
                                                        children: "Instance Type:"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoValue,
                                                        children: "External"
                                                    })
                                                ]
                                            }),
                                            session.restartedFromSessionId && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: infoItem,
                                                "data-testid": "restarted-from-row",
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoLabel,
                                                        children: "Restarted from:"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoValue,
                                                        children: restartSourceSession ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("a", {
                                                            href: routes/* routes */.J.sessionDetail(restartSourceSession.id),
                                                            className: restartedFromLink,
                                                            "data-testid": "restarted-from-link",
                                                            children: [
                                                                "↗ ",
                                                                restartSourceSession.title
                                                            ]
                                                        }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                            "data-testid": "restarted-from-unavailable",
                                                            children: [
                                                                session.restartedFromSessionId,
                                                                " (no longer available)"
                                                            ]
                                                        })
                                                    })
                                                ]
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: infoItem,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoLabel,
                                                        children: "Created:"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoValue,
                                                        children: session.createdAt ? new Date(Number(session.createdAt.seconds) * 1000).toLocaleString() : "N/A"
                                                    })
                                                ]
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: infoItem,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoLabel,
                                                        children: "Updated:"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoValue,
                                                        children: session.updatedAt ? new Date(Number(session.updatedAt.seconds) * 1000).toLocaleString() : "N/A"
                                                    })
                                                ]
                                            }),
                                            session.branch && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: infoItem,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoLabel,
                                                        children: "Branch:"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoValue,
                                                        style: {
                                                            fontFamily: 'monospace'
                                                        },
                                                        children: session.branch
                                                    })
                                                ]
                                            }),
                                            session.path && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: infoItem,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoLabel,
                                                        children: "Workspace Path:"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoValue,
                                                        style: {
                                                            fontFamily: 'monospace',
                                                            wordBreak: 'break-all'
                                                        },
                                                        children: session.path
                                                    })
                                                ]
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: infoItem,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoLabel,
                                                        children: "Working Directory:"
                                                    }),
                                                    isEditingWorkingDir ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                        className: editContainer,
                                                        children: [
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                                type: "text",
                                                                value: workingDirValue,
                                                                onChange: (e)=>setWorkingDirValue(e.target.value),
                                                                onKeyDown: (e)=>{
                                                                    if (e.key === "Enter") handleSaveWorkingDir();
                                                                    else if (e.key === "Escape") handleCancelWorkingDirEdit();
                                                                },
                                                                autoFocus: true,
                                                                className: editInput,
                                                                placeholder: session.path || "",
                                                                style: {
                                                                    fontFamily: 'monospace',
                                                                    minWidth: '20ch'
                                                                }
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                onClick: handleSaveWorkingDir,
                                                                className: SessionDetail_css_saveButton,
                                                                children: "✓"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                onClick: handleCancelWorkingDirEdit,
                                                                className: SessionDetail_css_cancelButton,
                                                                children: "✕"
                                                            })
                                                        ]
                                                    }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                        className: infoValue,
                                                        style: {
                                                            fontFamily: 'monospace',
                                                            wordBreak: 'break-all'
                                                        },
                                                        children: [
                                                            session.workingDir || /*#__PURE__*/ (0,jsx_runtime.jsx)("em", {
                                                                style: {
                                                                    opacity: 0.5
                                                                },
                                                                children: session.path
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                onClick: ()=>{
                                                                    setWorkingDirValue(session.workingDir || "");
                                                                    setIsEditingWorkingDir(true);
                                                                },
                                                                className: SessionDetail_css_editButton,
                                                                title: "Edit working directory",
                                                                children: "✏️"
                                                            })
                                                        ]
                                                    })
                                                ]
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: infoItem,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoLabel,
                                                        children: "Category:"
                                                    }),
                                                    isEditingCategory ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                        className: editContainer,
                                                        children: [
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                                type: "text",
                                                                value: categoryValue,
                                                                onChange: (e)=>setCategoryValue(e.target.value),
                                                                onKeyDown: (e)=>{
                                                                    if (e.key === "Enter") handleSaveCategory();
                                                                    else if (e.key === "Escape") handleCancelCategoryEdit();
                                                                },
                                                                autoFocus: true,
                                                                className: editInput,
                                                                placeholder: "e.g. Work/Frontend"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                onClick: handleSaveCategory,
                                                                className: SessionDetail_css_saveButton,
                                                                children: "✓"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                onClick: handleCancelCategoryEdit,
                                                                className: SessionDetail_css_cancelButton,
                                                                children: "✕"
                                                            })
                                                        ]
                                                    }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                        className: infoValue,
                                                        children: [
                                                            session.category || /*#__PURE__*/ (0,jsx_runtime.jsx)("em", {
                                                                style: {
                                                                    opacity: 0.5
                                                                },
                                                                children: "None"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                onClick: ()=>{
                                                                    setCategoryValue(session.category || "");
                                                                    setIsEditingCategory(true);
                                                                },
                                                                className: SessionDetail_css_editButton,
                                                                title: "Edit category",
                                                                children: "✏️"
                                                            })
                                                        ]
                                                    })
                                                ]
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: infoItem,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoLabel,
                                                        children: "Tags:"
                                                    }),
                                                    isEditingTags ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                        className: editContainer,
                                                        children: [
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                                type: "text",
                                                                value: tagsInputValue,
                                                                onChange: (e)=>setTagsInputValue(e.target.value),
                                                                onKeyDown: (e)=>{
                                                                    if (e.key === "Enter") handleSaveTags();
                                                                    else if (e.key === "Escape") handleCancelTagsEdit();
                                                                },
                                                                autoFocus: true,
                                                                className: editInput,
                                                                placeholder: "tag1, tag2, tag3"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                onClick: handleSaveTags,
                                                                className: SessionDetail_css_saveButton,
                                                                children: "✓"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                onClick: handleCancelTagsEdit,
                                                                className: SessionDetail_css_cancelButton,
                                                                children: "✕"
                                                            })
                                                        ]
                                                    }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                        className: infoValue,
                                                        children: [
                                                            ((_session_tags1 = session.tags) !== null && _session_tags1 !== void 0 ? _session_tags1 : []).length > 0 ? session.tags.join(", ") : /*#__PURE__*/ (0,jsx_runtime.jsx)("em", {
                                                                style: {
                                                                    opacity: 0.5
                                                                },
                                                                children: "None"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                onClick: ()=>{
                                                                    var _session_tags;
                                                                    setTagsInputValue(((_session_tags = session.tags) !== null && _session_tags !== void 0 ? _session_tags : []).join(", "));
                                                                    setIsEditingTags(true);
                                                                },
                                                                className: SessionDetail_css_editButton,
                                                                title: "Edit tags",
                                                                children: "✏️"
                                                            })
                                                        ]
                                                    })
                                                ]
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: infoItem,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoLabel,
                                                        children: "Auto Yes:"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoValue,
                                                        children: session.autoYes ? "Enabled" : "Disabled"
                                                    })
                                                ]
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: infoItem,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoLabel,
                                                        children: "Program:"
                                                    }),
                                                    isEditingProgram ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                        className: editContainer,
                                                        children: [
                                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                                                                value: programValue,
                                                                onChange: (e)=>setProgramValue(e.target.value),
                                                                autoFocus: true,
                                                                className: editInput,
                                                                children: [
                                                                    availablePrograms.map((p)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                                            value: p.value,
                                                                            children: p.label
                                                                        }, p.value)),
                                                                    !(0,programs/* isKnownProgram */.oV)(programValue) && /*#__PURE__*/ (0,jsx_runtime.jsxs)("option", {
                                                                        value: programValue,
                                                                        children: [
                                                                            "Custom: ",
                                                                            programValue
                                                                        ]
                                                                    })
                                                                ]
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                onClick: handleSaveProgram,
                                                                className: SessionDetail_css_saveButton,
                                                                children: "✓"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                onClick: handleCancelProgramEdit,
                                                                className: SessionDetail_css_cancelButton,
                                                                children: "✕"
                                                            })
                                                        ]
                                                    }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                        className: infoValue,
                                                        children: [
                                                            session.program ? (0,programs/* getProgramDisplay */.pw)(session.program) : /*#__PURE__*/ (0,jsx_runtime.jsx)("em", {
                                                                style: {
                                                                    opacity: 0.5
                                                                },
                                                                children: "None"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                onClick: ()=>{
                                                                    setProgramValue(session.program || "");
                                                                    setIsEditingProgram(true);
                                                                },
                                                                className: SessionDetail_css_editButton,
                                                                title: "Edit program",
                                                                children: "✏️"
                                                            })
                                                        ]
                                                    })
                                                ]
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: infoItem,
                                                style: {
                                                    alignItems: 'flex-start'
                                                },
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoLabel,
                                                        style: {
                                                            paddingTop: '2px'
                                                        },
                                                        children: "Launch Command:"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                                        style: {
                                                            flex: 1,
                                                            minWidth: 0
                                                        },
                                                        children: session.launchCommand ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                                            children: [
                                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("pre", {
                                                                    style: {
                                                                        fontFamily: 'monospace',
                                                                        fontSize: '0.82em',
                                                                        margin: 0,
                                                                        padding: '6px 8px',
                                                                        background: 'var(--terminal-background, #1a1a2e)',
                                                                        color: 'var(--terminal-foreground, #e0e0e0)',
                                                                        borderRadius: '4px',
                                                                        overflowX: launchCmdExpanded ? 'auto' : 'hidden',
                                                                        whiteSpace: launchCmdExpanded ? 'pre' : 'nowrap',
                                                                        textOverflow: launchCmdExpanded ? 'clip' : 'ellipsis',
                                                                        maxHeight: launchCmdExpanded ? '200px' : undefined,
                                                                        overflowY: launchCmdExpanded ? 'auto' : undefined,
                                                                        cursor: 'default'
                                                                    },
                                                                    children: session.launchCommand
                                                                }),
                                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                                    style: {
                                                                        display: 'flex',
                                                                        gap: '8px',
                                                                        marginTop: '4px'
                                                                    },
                                                                    children: [
                                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                            onClick: ()=>setLaunchCmdExpanded((v)=>!v),
                                                                            className: SessionDetail_css_editButton,
                                                                            title: launchCmdExpanded ? "Collapse" : "Expand full command",
                                                                            style: {
                                                                                fontSize: '0.75em'
                                                                            },
                                                                            children: launchCmdExpanded ? "▲ collapse" : "▼ expand"
                                                                        }),
                                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                            onClick: ()=>{
                                                                                navigator.clipboard.writeText(session.launchCommand).then(()=>{
                                                                                    setCopiedLaunchCmd(true);
                                                                                    setTimeout(()=>setCopiedLaunchCmd(false), 2000);
                                                                                }).catch((err)=>{
                                                                                    console.warn("[SessionDetailView] clipboard write failed", err);
                                                                                });
                                                                            },
                                                                            className: SessionDetail_css_editButton,
                                                                            title: "Copy to clipboard",
                                                                            style: {
                                                                                fontSize: '0.75em'
                                                                            },
                                                                            children: copiedLaunchCmd ? "✓ copied" : "⎘ copy"
                                                                        })
                                                                    ]
                                                                })
                                                            ]
                                                        }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("em", {
                                                            style: {
                                                                opacity: 0.5
                                                            },
                                                            children: "Not started yet"
                                                        })
                                                    })
                                                ]
                                            }),
                                            session.tmuxPrefix && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: infoItem,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoLabel,
                                                        children: "Tmux Prefix:"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoValue,
                                                        style: {
                                                            fontFamily: 'monospace'
                                                        },
                                                        children: session.tmuxPrefix
                                                    })
                                                ]
                                            }),
                                            ((_session_claudeSession = session.claudeSession) === null || _session_claudeSession === void 0 ? void 0 : _session_claudeSession.sessionId) && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: infoItem,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoLabel,
                                                        children: "Claude Conversation UUID:"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                        className: infoValue,
                                                        style: {
                                                            fontFamily: 'monospace',
                                                            fontSize: '0.9em'
                                                        },
                                                        children: [
                                                            session.claudeSession.sessionId,
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                onClick: ()=>handleCopy('claudeUuid', session.claudeSession.sessionId),
                                                                className: SessionDetail_css_editButton,
                                                                title: "Copy to clipboard",
                                                                children: copiedField === 'claudeUuid' ? '✓' : '📋'
                                                            })
                                                        ]
                                                    })
                                                ]
                                            }),
                                            ((_session_claudeSession1 = session.claudeSession) === null || _session_claudeSession1 === void 0 ? void 0 : _session_claudeSession1.projectName) && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: infoItem,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoLabel,
                                                        children: "Claude Project:"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoValue,
                                                        children: session.claudeSession.projectName
                                                    })
                                                ]
                                            }),
                                            session.historyFilePath && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: infoItem,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoLabel,
                                                        children: "History File:"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoValue,
                                                        style: {
                                                            fontFamily: 'monospace',
                                                            wordBreak: 'break-all',
                                                            fontSize: '0.85em'
                                                        },
                                                        children: session.historyFilePath
                                                    })
                                                ]
                                            }),
                                            session.gitWorktree && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                                children: [
                                                    session.gitWorktree.repoPath && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                        className: infoItem,
                                                        children: [
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                className: infoLabel,
                                                                children: "Repo Path:"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                className: infoValue,
                                                                style: {
                                                                    fontFamily: 'monospace',
                                                                    wordBreak: 'break-all'
                                                                },
                                                                children: session.gitWorktree.repoPath
                                                            })
                                                        ]
                                                    }),
                                                    session.gitWorktree.worktreePath && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                        className: infoItem,
                                                        children: [
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                className: infoLabel,
                                                                children: "Worktree Path:"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                className: infoValue,
                                                                style: {
                                                                    fontFamily: 'monospace',
                                                                    wordBreak: 'break-all'
                                                                },
                                                                children: session.gitWorktree.worktreePath
                                                            })
                                                        ]
                                                    }),
                                                    session.gitWorktree.branchName && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                        className: infoItem,
                                                        children: [
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                className: infoLabel,
                                                                children: "Worktree Branch:"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                className: infoValue,
                                                                style: {
                                                                    fontFamily: 'monospace'
                                                                },
                                                                children: session.gitWorktree.branchName
                                                            })
                                                        ]
                                                    }),
                                                    session.gitWorktree.baseCommitSha && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                        className: infoItem,
                                                        children: [
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                className: infoLabel,
                                                                children: "Base Commit:"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                                className: infoValue,
                                                                style: {
                                                                    fontFamily: 'monospace',
                                                                    fontSize: '0.85em'
                                                                },
                                                                children: [
                                                                    session.gitWorktree.baseCommitSha,
                                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                        onClick: ()=>handleCopy('baseCommit', session.gitWorktree.baseCommitSha),
                                                                        className: SessionDetail_css_editButton,
                                                                        title: "Copy to clipboard",
                                                                        children: copiedField === 'baseCommit' ? '✓' : '📋'
                                                                    })
                                                                ]
                                                            })
                                                        ]
                                                    })
                                                ]
                                            }),
                                            session.diffStats && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: infoItem,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoLabel,
                                                        children: "Diff Stats:"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                        className: infoValue,
                                                        children: [
                                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                                className: diffAdded,
                                                                children: [
                                                                    "+",
                                                                    session.diffStats.added
                                                                ]
                                                            }),
                                                            " / ",
                                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                                style: {
                                                                    color: 'var(--color-error, #ef4444)'
                                                                },
                                                                children: [
                                                                    "-",
                                                                    session.diffStats.removed
                                                                ]
                                                            })
                                                        ]
                                                    })
                                                ]
                                            }),
                                            session.githubPrUrl && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: infoItem,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoLabel,
                                                        children: "GitHub PR:"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoValue,
                                                        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("a", {
                                                            href: session.githubPrUrl,
                                                            target: "_blank",
                                                            rel: "noopener noreferrer",
                                                            style: {
                                                                color: 'var(--color-link, #3b82f6)'
                                                            },
                                                            children: [
                                                                "#",
                                                                session.githubPrNumber,
                                                                " ",
                                                                session.githubPrState && "(".concat(session.githubPrState, ")"),
                                                                session.githubPrIsDraft && " [Draft]"
                                                            ]
                                                        })
                                                    })
                                                ]
                                            }),
                                            session.githubOwner && session.githubRepo && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: infoItem,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoLabel,
                                                        children: "GitHub Repo:"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                        className: infoValue,
                                                        style: {
                                                            fontFamily: 'monospace'
                                                        },
                                                        children: [
                                                            session.githubOwner,
                                                            "/",
                                                            session.githubRepo
                                                        ]
                                                    })
                                                ]
                                            }),
                                            session.githubSourceRef && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: infoItem,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoLabel,
                                                        children: "Source Ref:"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoValue,
                                                        style: {
                                                            fontFamily: 'monospace'
                                                        },
                                                        children: session.githubSourceRef
                                                    })
                                                ]
                                            }),
                                            (session.githubApprovedCount > 0 || session.githubChangesReqCount > 0) && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: infoItem,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoLabel,
                                                        children: "Reviews:"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                        className: infoValue,
                                                        children: [
                                                            session.githubApprovedCount > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                                className: diffAdded,
                                                                children: [
                                                                    session.githubApprovedCount,
                                                                    " approved"
                                                                ]
                                                            }),
                                                            session.githubApprovedCount > 0 && session.githubChangesReqCount > 0 && " · ",
                                                            session.githubChangesReqCount > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                                style: {
                                                                    color: 'var(--color-error, #ef4444)'
                                                                },
                                                                children: [
                                                                    session.githubChangesReqCount,
                                                                    " changes requested"
                                                                ]
                                                            })
                                                        ]
                                                    })
                                                ]
                                            }),
                                            session.githubCheckConclusion && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: infoItem,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoLabel,
                                                        children: "CI Status:"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoValue,
                                                        children: session.githubCheckConclusion
                                                    })
                                                ]
                                            }),
                                            session.clonedRepoPath && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: infoItem,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoLabel,
                                                        children: "Cloned Repo:"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoValue,
                                                        style: {
                                                            fontFamily: 'monospace',
                                                            wordBreak: 'break-all'
                                                        },
                                                        children: session.clonedRepoPath
                                                    })
                                                ]
                                            }),
                                            session.instanceType === types_pb/* InstanceType */.y9.EXTERNAL && session.externalMetadata && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                                children: [
                                                    session.externalMetadata.sourceTerminal && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                        className: infoItem,
                                                        children: [
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                className: infoLabel,
                                                                children: "Source Terminal:"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                className: infoValue,
                                                                children: session.externalMetadata.sourceTerminal
                                                            })
                                                        ]
                                                    }),
                                                    session.externalMetadata.muxEnabled && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                        className: infoItem,
                                                        children: [
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                className: infoLabel,
                                                                children: "Mux Enabled:"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                className: infoValue,
                                                                children: "Yes"
                                                            })
                                                        ]
                                                    }),
                                                    session.externalMetadata.tmuxSessionName && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                        className: infoItem,
                                                        children: [
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                className: infoLabel,
                                                                children: "Tmux Session:"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                className: infoValue,
                                                                style: {
                                                                    fontFamily: 'monospace'
                                                                },
                                                                children: session.externalMetadata.tmuxSessionName
                                                            })
                                                        ]
                                                    })
                                                ]
                                            }),
                                            session.prompt && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: infoItem,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoLabel,
                                                        children: "Launch Prompt:"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoValue,
                                                        children: session.prompt
                                                    })
                                                ]
                                            }),
                                            session.initialPrompt && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: infoItem,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoLabel,
                                                        children: "Terminal Prompt:"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: infoValue,
                                                        style: {
                                                            whiteSpace: "pre-wrap",
                                                            fontFamily: "monospace",
                                                            fontSize: "0.85em"
                                                        },
                                                        children: session.initialPrompt
                                                    })
                                                ]
                                            }),
                                            session.workflowId && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: workflowSection,
                                                "data-testid": "workflow-metadata-section",
                                                children: [
                                                    isAutomatedTrigger && triggerWorkflow && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                        className: infoItem,
                                                        children: [
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                className: infoLabel,
                                                                children: "Trigger:"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("a", {
                                                                className: TriggersPanel_css/* attributionBadge */.fZ,
                                                                href: "/triggers",
                                                                "data-testid": "trigger-attribution-badge",
                                                                "aria-label": "Triggered by ".concat(triggerWorkflow.slug, " (").concat(triggerWorkflow.triggerType, ") — view in Triggers"),
                                                                children: [
                                                                    "Triggered by: ",
                                                                    triggerWorkflow.slug,
                                                                    " (",
                                                                    triggerWorkflow.triggerType,
                                                                    ")"
                                                                ]
                                                            })
                                                        ]
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                        className: infoItem,
                                                        children: [
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                className: infoLabel,
                                                                children: "Workflow:"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                className: infoValue,
                                                                children: session.workflowName || session.workflowId
                                                            })
                                                        ]
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                        className: infoItem,
                                                        children: [
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                className: infoLabel,
                                                                children: "Workflow ID:"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                                className: infoValue,
                                                                style: {
                                                                    fontFamily: "monospace",
                                                                    fontSize: "0.85em"
                                                                },
                                                                children: [
                                                                    session.workflowId,
                                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                        onClick: ()=>handleCopy("workflowId", session.workflowId),
                                                                        className: SessionDetail_css_editButton,
                                                                        title: "Copy to clipboard",
                                                                        children: copiedField === "workflowId" ? "✓" : "📋"
                                                                    })
                                                                ]
                                                            })
                                                        ]
                                                    })
                                                ]
                                            })
                                        ]
                                    }),
                                    ((_session_goal = session.goal) === null || _session_goal === void 0 ? void 0 : _session_goal.goalText) && /*#__PURE__*/ (0,jsx_runtime.jsx)(GoalPanel, {
                                        goal: session.goal
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(NotePanel, {
                                        note: (_session_note = session.note) !== null && _session_note !== void 0 ? _session_note : "",
                                        onSave: async (v)=>{
                                            // actions.update resolves to null (never rejects) on RPC failure — NotePanel's
                                            // save-error UI (aria-live assertive message, textarea preserved) only fires on
                                            // a rejected promise, so a null result must be converted into a throw here.
                                            const result = await actions.update({
                                                note: v
                                            });
                                            if (!result) throw new Error("Failed to save note");
                                        }
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(GuidanceRequestPanel/* GuidanceRequestPanel */.w, {
                                        scope: "session",
                                        scopeKey: session.id
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(WorkspacePeersPanel, {
                                        session: session
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(HandoffSummarySection, {
                                        sessionId: session.id
                                    })
                                ]
                            }),
                            activeTab === "artifacts" && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: tabContent,
                                role: "tabpanel",
                                "aria-labelledby": "tab-artifacts",
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ArtifactsTab, {
                                    session: session
                                })
                            }),
                            activeTab === "summary" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: tabContent,
                                role: "tabpanel",
                                "aria-labelledby": "tab-summary",
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionSummaryPanel/* SessionSummaryPanel */.L, {
                                        sessionId: session.id
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(RetryHistoryList, {
                                        history: session.retryHistory
                                    })
                                ]
                            })
                        ]
                    }),
                    backlogItemId && /*#__PURE__*/ (0,jsx_runtime.jsx)(BacklogItemPanel, {
                        backlogItemId: backlogItemId,
                        sessionId: session.id
                    })
                ]
            }),
            exportBundleOpen && /*#__PURE__*/ (0,jsx_runtime.jsx)(ExportBundleModal/* ExportBundleModal */.s, {
                open: exportBundleOpen,
                onOpenChange: setExportBundleOpen,
                sessionId: session.id,
                sessionTitle: session.title,
                repoPath: session.path
            }),
            showWorkspaceSwitchModal && /*#__PURE__*/ (0,jsx_runtime.jsx)(WorkspaceSwitchModal, {
                sessionId: session.id,
                sessionName: session.title,
                baseUrl: (0,config/* getApiBaseUrl */.h)(),
                onClose: ()=>setShowWorkspaceSwitchModal(false),
                onSwitched: ()=>{
                    // The session will be updated via the event bus
                    setShowWorkspaceSwitchModal(false);
                },
                triggerRef: workspaceSwitchTriggerRef
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(Modal/* Modal */.aF, {
                open: actionSheetOpen,
                onOpenChange: setActionSheetOpen,
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)(Modal/* ModalContent */.$m, {
                    fallbackTitle: session.title,
                    "data-testid": "action-sheet",
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)(Modal/* ModalTitle */.wt, {
                            children: session.title
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: actionSheet,
                            children: [
                                session.instanceType !== types_pb/* InstanceType */.y9.EXTERNAL && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: actionSheetItem,
                                    onClick: (e)=>handlePauseResume(e.currentTarget),
                                    "data-testid": "action-pause",
                                    children: session.status === types_pb/* SessionStatus */.Ph.PAUSED ? '▶ Resume' : '⏸ Pause'
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: actionSheetItem,
                                    onClick: ()=>{
                                        setActionSheetOpen(false);
                                        setRenameValue(session.title);
                                        setShowRenameModal(true);
                                    },
                                    "data-testid": "action-rename",
                                    children: "✏️ Rename"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: actionSheetItem,
                                    onClick: ()=>{
                                        setActionSheetOpen(false);
                                        tagEditorTriggerRef.current = moreActionsButtonRef.current;
                                        setShowTagEditor(true);
                                    },
                                    "data-testid": "action-edit-tags",
                                    children: "\uD83C\uDFF7 Edit Tags"
                                }),
                                session.instanceType !== types_pb/* InstanceType */.y9.EXTERNAL && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: actionSheetItem,
                                    onClick: ()=>{
                                        setActionSheetOpen(false);
                                        setCheckpointLabel("");
                                        setShowCheckpointModal(true);
                                    },
                                    "data-testid": "action-checkpoint",
                                    children: "\uD83D\uDCF8 Create Checkpoint"
                                }),
                                session.instanceType !== types_pb/* InstanceType */.y9.EXTERNAL && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: actionSheetItem,
                                    onClick: ()=>{
                                        setActionSheetOpen(false);
                                        workspaceSwitchTriggerRef.current = moreActionsButtonRef.current;
                                        setShowWorkspaceSwitchModal(true);
                                    },
                                    children: "⎇ Switch Workspace"
                                }),
                                session.instanceType !== types_pb/* InstanceType */.y9.EXTERNAL && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: actionSheetItem,
                                    onClick: ()=>{
                                        setActionSheetOpen(false);
                                        setShowRestartConfirm(true);
                                    },
                                    "data-testid": "action-restart",
                                    children: "\uD83D\uDD04 Restart"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: actionSheetItem,
                                    onClick: ()=>{
                                        setActionSheetOpen(false);
                                        setShowNewShellDialog(true);
                                    },
                                    "data-testid": "action-spawn-shell",
                                    children: ">_ Spawn new shell"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: actionSheetItem,
                                    onClick: ()=>{
                                        setActionSheetOpen(false);
                                        setExportBundleOpen(true);
                                    },
                                    "data-testid": "export-diagnostics-action",
                                    children: "\uD83D\uDCE6 Export Diagnostics"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("hr", {
                                    className: actionDivider
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: "".concat(actionSheetItem, " ").concat(actionSheetItemDestructive),
                                    onClick: handleDeleteClick,
                                    "data-testid": "action-delete",
                                    children: "\uD83D\uDDD1 Delete"
                                })
                            ]
                        })
                    ]
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(Modal/* Modal */.aF, {
                open: showDeleteConfirm,
                onOpenChange: setShowDeleteConfirm,
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)(Modal/* ModalContent */.$m, {
                    fallbackTitle: "Confirm delete",
                    "data-testid": "delete-confirm-dialog",
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)(Modal/* ModalTitle */.wt, {
                            children: "Delete session?"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                            style: {
                                color: 'var(--text-secondary)',
                                marginBottom: '1rem'
                            },
                            children: "This session is currently running. Stop and delete it?"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)(Modal/* ModalFooter */.jl, {
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: SessionDetail_css_actionButton,
                                    onClick: ()=>setShowDeleteConfirm(false),
                                    children: "Cancel"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: "".concat(SessionDetail_css_actionButton, " ").concat(actionButtonDanger),
                                    onClick: handleConfirmDelete,
                                    "data-testid": "delete-confirm",
                                    children: "Delete"
                                })
                            ]
                        })
                    ]
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(Modal/* Modal */.aF, {
                open: showRenameModal,
                onOpenChange: setShowRenameModal,
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)(Modal/* ModalContent */.$m, {
                    fallbackTitle: "Rename session",
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)(Modal/* ModalTitle */.wt, {
                            children: "Rename session"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                            type: "text",
                            value: renameValue,
                            onChange: (e)=>{
                                setRenameValue(e.target.value);
                                setRenameError(null);
                            },
                            onKeyDown: (e)=>{
                                if (e.key === 'Enter') handleRenameSave();
                                else if (e.key === 'Escape') setShowRenameModal(false);
                            },
                            autoFocus: true,
                            className: editInput,
                            "data-testid": "rename-input"
                        }),
                        renameError && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                            style: {
                                color: 'var(--error)',
                                fontSize: '0.875rem',
                                marginTop: '0.25rem'
                            },
                            children: renameError
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)(Modal/* ModalFooter */.jl, {
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: SessionDetail_css_actionButton,
                                    onClick: ()=>setShowRenameModal(false),
                                    children: "Cancel"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: "".concat(SessionDetail_css_actionButton, " ").concat(actionButtonSave),
                                    onClick: handleRenameSave,
                                    "data-testid": "rename-save",
                                    children: "Save"
                                })
                            ]
                        })
                    ]
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(Modal/* Modal */.aF, {
                open: showRestartConfirm,
                onOpenChange: setShowRestartConfirm,
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)(Modal/* ModalContent */.$m, {
                    fallbackTitle: "Restart session",
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)(Modal/* ModalTitle */.wt, {
                            children: "Restart session?"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                            style: {
                                color: 'var(--text-secondary)',
                                marginBottom: '1rem'
                            },
                            children: "This will stop and restart the session process."
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)(Modal/* ModalFooter */.jl, {
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: SessionDetail_css_actionButton,
                                    onClick: ()=>setShowRestartConfirm(false),
                                    children: "Cancel"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: "".concat(SessionDetail_css_actionButton, " ").concat(actionButtonDanger),
                                    onClick: async ()=>{
                                        setShowRestartConfirm(false);
                                        await actions.restart();
                                    },
                                    "data-testid": "restart-confirm",
                                    children: "Restart"
                                })
                            ]
                        })
                    ]
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(Modal/* Modal */.aF, {
                open: showCheckpointModal,
                onOpenChange: setShowCheckpointModal,
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)(Modal/* ModalContent */.$m, {
                    fallbackTitle: "Create checkpoint",
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)(Modal/* ModalTitle */.wt, {
                            children: "Create checkpoint"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                            type: "text",
                            value: checkpointLabel,
                            onChange: (e)=>setCheckpointLabel(e.target.value),
                            onKeyDown: (e)=>{
                                if (e.key === 'Enter') {
                                    setShowCheckpointModal(false);
                                    actions.createCheckpoint(checkpointLabel.trim());
                                } else if (e.key === 'Escape') setShowCheckpointModal(false);
                            },
                            placeholder: "Optional label...",
                            autoFocus: true,
                            className: editInput,
                            "data-testid": "checkpoint-input"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)(Modal/* ModalFooter */.jl, {
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: SessionDetail_css_actionButton,
                                    onClick: ()=>setShowCheckpointModal(false),
                                    children: "Cancel"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: "".concat(SessionDetail_css_actionButton, " ").concat(actionButtonSave),
                                    onClick: ()=>{
                                        setShowCheckpointModal(false);
                                        actions.createCheckpoint(checkpointLabel.trim());
                                    },
                                    "data-testid": "checkpoint-save",
                                    children: "Save"
                                })
                            ]
                        })
                    ]
                })
            }),
            showTagEditor && /*#__PURE__*/ (0,jsx_runtime.jsx)(TagEditor/* TagEditor */.y, {
                tags: session.tags || [],
                sessionTitle: session.title,
                onSave: (tags)=>{
                    actions.updateTags(tags);
                    setShowTagEditor(false);
                },
                onCancel: ()=>setShowTagEditor(false),
                triggerRef: tagEditorTriggerRef
            }),
            showResumeModal && /*#__PURE__*/ (0,jsx_runtime.jsx)(ResumeSessionModal/* ResumeSessionModal */.h, {
                session: session,
                sessions: allSessions,
                onConfirm: async (updates)=>{
                    await actions.resume({
                        title: updates.title,
                        tags: updates.tags
                    });
                    setShowResumeModal(false);
                },
                onCancel: ()=>setShowResumeModal(false),
                triggerRef: resumeTriggerRef
            })
        ]
    });
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

/***/ 61423:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  l: () => (/* binding */ useTrayDismissal)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/nav/index.ts
var nav = __webpack_require__(41078);
;// ./src/lib/hooks/usePanelHistory.ts
/* __next_internal_client_entry_do_not_use__ usePanelHistory auto */ 

/**
 * One history entry per open panel (ux.md 11.3): Back closes it, closing it any other way
 * removes the entry, and a deep link replaces the current entry so Back leaves the page.
 * `releaseHistoryEntry` lets a navigation that closes the panel keep its own new entry.
 */ function usePanelHistory(param) {
    let { active, onBack, replace = false } = param;
    const backRef = (0,react.useRef)(onBack);
    backRef.current = onBack;
    const ownsEntry = (0,react.useRef)(false);
    // A navigation that closes the panel must not also history.back(): that pops the page it just opened.
    const releaseHistoryEntry = (0,react.useRef)(()=>{
        ownsEntry.current = false;
    }).current;
    (0,react.useEffect)(()=>{
        if (!active) return;
        (0,nav/* pushTrayEntry */.x7)(replace);
        ownsEntry.current = !replace;
        const onPopState = ()=>{
            ownsEntry.current = false;
            backRef.current();
        };
        window.addEventListener("popstate", onPopState);
        return ()=>{
            window.removeEventListener("popstate", onPopState);
            if (ownsEntry.current) {
                ownsEntry.current = false;
                window.history.back();
            }
        };
    }, [
        active,
        replace
    ]);
    return {
        releaseHistoryEntry
    };
}

;// ./src/lib/hooks/useTrayDismissal.ts
/* __next_internal_client_entry_do_not_use__ useTrayDismissal auto */ 

/**
 * Every way the open tray goes away that is not a button: Esc (from inside the
 * tray or the page body, never while focus is in the terminal, TY-4), hardware
 * Back on a bottom sheet (TS-8; the entry keeps the URL and Next's own state), and
 * the modal sheet's inert background.
 */ function useTrayDismissal(param) {
    let { isOpen, isSheet, modal, trayRef, close } = param;
    (0,react.useEffect)(()=>{
        if (!isOpen) return;
        const onKeyDown = (e)=>{
            var _trayRef_current;
            if (e.key !== "Escape" || e.defaultPrevented) return;
            const target = e.target;
            const inTray = !!target && !!((_trayRef_current = trayRef.current) === null || _trayRef_current === void 0 ? void 0 : _trayRef_current.contains(target));
            const onBody = target === document.body || target === document.documentElement || target === null;
            if (inTray || onBody) close();
        };
        document.addEventListener("keydown", onKeyDown);
        return ()=>document.removeEventListener("keydown", onKeyDown);
    }, [
        isOpen,
        trayRef,
        close
    ]);
    const { releaseHistoryEntry } = usePanelHistory({
        active: isSheet && isOpen,
        onBack: close
    });
    (0,react.useEffect)(()=>{
        if (!modal) return;
        const main = document.getElementById("main-content");
        main === null || main === void 0 ? void 0 : main.setAttribute("inert", "");
        return ()=>main === null || main === void 0 ? void 0 : main.removeAttribute("inert");
    }, [
        modal
    ]);
    return {
        releaseHistoryEntry
    };
}


/***/ }),

/***/ 61823:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   $: () => (/* binding */ titleRow),
/* harmony export */   AP: () => (/* binding */ typeBadge),
/* harmony export */   B7: () => (/* binding */ mobileAddFab),
/* harmony export */   C7: () => (/* binding */ typeGithubPush),
/* harmony export */   DD: () => (/* binding */ title),
/* harmony export */   Db: () => (/* binding */ historyCounter),
/* harmony export */   FP: () => (/* binding */ emptyStateLink),
/* harmony export */   Ft: () => (/* binding */ lastFired),
/* harmony export */   Ie: () => (/* binding */ empty),
/* harmony export */   L$: () => (/* binding */ toggle),
/* harmony export */   Lq: () => (/* binding */ loading),
/* harmony export */   MM: () => (/* binding */ cardTop),
/* harmony export */   Mf: () => (/* binding */ tab),
/* harmony export */   Mn: () => (/* binding */ statusFiredFailed),
/* harmony export */   Nr: () => (/* binding */ card),
/* harmony export */   O6: () => (/* binding */ triggerSlug),
/* harmony export */   Od: () => (/* binding */ statusBadge),
/* harmony export */   PI: () => (/* binding */ historyList),
/* harmony export */   Pn: () => (/* binding */ statusNoMatch),
/* harmony export */   Pz: () => (/* binding */ headerButtons),
/* harmony export */   Qg: () => (/* binding */ visuallyHidden),
/* harmony export */   SA: () => (/* binding */ statusFiredSuccess),
/* harmony export */   St: () => (/* binding */ toggleOff),
/* harmony export */   V9: () => (/* binding */ historyError),
/* harmony export */   VA: () => (/* binding */ subtitle),
/* harmony export */   ZU: () => (/* binding */ historyTimestamp),
/* harmony export */   _S: () => (/* binding */ tdCenter),
/* harmony export */   _c: () => (/* binding */ refreshButton),
/* harmony export */   _s: () => (/* binding */ toggleOn),
/* harmony export */   am: () => (/* binding */ triggerName),
/* harmony export */   bK: () => (/* binding */ historyToggle),
/* harmony export */   bX: () => (/* binding */ historySessionLink),
/* harmony export */   c9: () => (/* binding */ addButton),
/* harmony export */   cm: () => (/* binding */ rowActions),
/* harmony export */   df: () => (/* binding */ neverFired),
/* harmony export */   fZ: () => (/* binding */ attributionBadge),
/* harmony export */   ft: () => (/* binding */ retryButton),
/* harmony export */   gb: () => (/* binding */ iconButton),
/* harmony export */   hL: () => (/* binding */ cardList),
/* harmony export */   k: () => (/* binding */ headerButtonsHiddenOnMobile),
/* harmony export */   lr: () => (/* binding */ rowDisabled),
/* harmony export */   nM: () => (/* binding */ row),
/* harmony export */   nd: () => (/* binding */ panel),
/* harmony export */   o0: () => (/* binding */ cardMeta),
/* harmony export */   oW: () => (/* binding */ rowCount),
/* harmony export */   qN: () => (/* binding */ typeWebhook),
/* harmony export */   qR: () => (/* binding */ historyWrapper),
/* harmony export */   rP: () => (/* binding */ statusRejected),
/* harmony export */   sH: () => (/* binding */ showNoMatchToggle),
/* harmony export */   td: () => (/* binding */ td),
/* harmony export */   th: () => (/* binding */ th),
/* harmony export */   tp: () => (/* binding */ table),
/* harmony export */   vR: () => (/* binding */ tabs),
/* harmony export */   vT: () => (/* binding */ toggleError),
/* harmony export */   wY: () => (/* binding */ tableWrapper),
/* harmony export */   wx: () => (/* binding */ header),
/* harmony export */   x6: () => (/* binding */ typeCron),
/* harmony export */   xg: () => (/* binding */ historyEntry),
/* harmony export */   yR: () => (/* binding */ tabActive),
/* harmony export */   z3: () => (/* binding */ error)
/* harmony export */ });
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(65916);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Fa11y_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll85amk4YnAwIHsKICBwb3NpdGlvbjogYWJzb2x1dGU7CiAgd2lkdGg6IDFweDsKICBoZWlnaHQ6IDFweDsKICBwYWRkaW5nOiAwOwogIG1hcmdpbjogLTFweDsKICBvdmVyZmxvdzogaGlkZGVuOwogIGNsaXAtcGF0aDogaW5zZXQoNTAlKTsKICB3aGl0ZS1zcGFjZTogbm93cmFwOwogIGJvcmRlcjogMDsKfQ_22_7D__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(47439);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Fa11y_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll85amk4YnAwIHsKICBwb3NpdGlvbjogYWJzb2x1dGU7CiAgd2lkdGg6IDFweDsKICBoZWlnaHQ6IDFweDsKICBwYWRkaW5nOiAwOwogIG1hcmdpbjogLTFweDsKICBvdmVyZmxvdzogaGlkZGVuOwogIGNsaXAtcGF0aDogaW5zZXQoNTAlKTsKICB3aGl0ZS1zcGFjZTogbm93cmFwOwogIGJvcmRlcjogMDsKfQ_22_7D__WEBPACK_IMPORTED_MODULE_1___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Fa11y_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll85amk4YnAwIHsKICBwb3NpdGlvbjogYWJzb2x1dGU7CiAgd2lkdGg6IDFweDsKICBoZWlnaHQ6IDFweDsKICBwYWRkaW5nOiAwOwogIG1hcmdpbjogLTFweDsKICBvdmVyZmxvdzogaGlkZGVuOwogIGNsaXAtcGF0aDogaW5zZXQoNTAlKTsKICB3aGl0ZS1zcGFjZTogbm93cmFwOwogIGJvcmRlcjogMDsKfQ_22_7D__WEBPACK_IMPORTED_MODULE_1__);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FTriggersPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81ZUW_jOBB_31_hl5Ma3RFhCknKvtw_WTngJG6MTY2TkJz6309gCDbYQLLt6VRp2zX2eGb8zczn8fJXcMXrN_aDf34AsEXJcS_4iaUxOCPx4nm_4Bu_BeK2XvysvnORYhEDmJeg4JSkvVlYm_UJlJJTEQMY5GU1nKM0JWwfg8BXAykpcoquMdhRXA9Uv72UCJxIwlkMEk5PGau_7FEeA7hS6zLCvAMm_4OMgX9fVxwEYcd65PPHsjEL1mY9ulFY7dMJCexC3k_FJLurl3AmMZMxKHKUYG_L5QXjWhiiZM88InFWxCDBTGKhy321y61VMP0a3BYz5IW1vAyJPWGtZziTXkFuOAZBqLxXD10a9639elrCKRf9Tf2FLjwaFQ5flXCrIGgIWk0gbfM80jY9oK3yEkBfM7tRtsFRchJFpW3OiXLliCMAkAKxgii8IEqBvwwKgFGBDePiAz9jETMuX_KUFGhLcbqYsBhuTAfdF9breI4SIq8x8JeRrjXj0kOU8gtO9dXretHdBW3wjewOZzkc_iMetx97sHCgZAB5K7DbqPfNaNxMedO0h3GGLYqHPai85uXdmJmwaDyi2xeYqr6ZZ9FGoMRldXRkz3RjnaFjbtHLTej7t9gOHG76tPu_23OEHbAg8q5aihMukAqmE0uxoESJuFtyzzDa4k6nxDR7UwX69yJ99RjSe0hIR1J9I7muSRdRjVT_6qvx96XMVS8OIs2Ttvw_NzYcKRNGw5yJVc6ckyRnVardhKRM94NVXube6sPYaq8y9BmLHeUXr4wBOkn_Ux_7dmMZKu_8JWpJ0POFrlPjUKtxIak8VPnS_8M0kKK8wDXJqf_yHm0njPQKiK8hQk8rFO_kgE_sRvhEE33Oc9nqlm65lDybcsvlQCT2at5VZSEVNwDkvEVdIUlyvNaq87xJKjePsBSXMYC62e8jZp_xkCRBtDVd8vxhVWcA91irYEvd3RwaU1RILzkQmgKp0kpPD5WNuyVZO09jEmGkz2BmdtpSnhwHRxv5BrPmY2saZEEtj_xQRuh1QGxPbucovCjG6dXH10uquakCYVUR8e6adEWxq_12FuDSuYdqK9o6dT4m8s523NRO0O_kwsyQVExIuroFlYYgaXX1HPrWJ1r_0WGcJiw_uy0_GZafv8qFl3m81U01O1Hl80gLDEHXWtCAt8xC6e2htTpDkleKY0AkoiQxLuu_ls6eYe5T0Glvu0MSM8ZUVMOhqa5hOOxBBL1yDH3zBtful_IdOtG6aJqXOm0p_CrcwmAWaZxx0HCsT9G4w5EBRjgtDPv48ZfrSODMONxQu8HrLn8NnHz0KUIVDu8ONSRyJDCTjovMLHYLw1n01mwBwOgBDuuMlH47pL4lrYahYmsMjRG5j8WMKGoQ7vJKpHlFC4aNGQ0rE3l9ZgM3E9_fjITyWNfS73mvTW_PNhM3Q2gNs7WmM_qSbqNqiUokpKmIttF2pDeqx6x5TXVF_lM1BCZPFBGdIIsKrdoxKZ646lmafkEHwxZ_Fr0NnqM3YxwdD01LPOxp_NWPMWn3cMW_U_j2ftD3zv6pHnkDB0qKe0VvnWg0ifWGjrbpwb7pGJO0FPm_01dOcrmaisJ5Dxw7E77qrjxFeEavPhbOMkFt4bt7V0snCvaO_hePUQ1fAjBdDLOfqs39epo58PJCLCEdRMUVRfD_tqim_VMBc_PzemSNXy5y_UFgAMSm2mOWkcLn9nOCUIFInAmAHEUvBSdcYagr0K_bxULyZml21Q5wH41CY1XMGSU_R_0I6U1XNJNdimwwTR5EXZrIY8hs4N0fwLrINKmz9r3rRQC4Xyasuk9IaS34ie4G2jvGpYlAbnHqkO373lFwWtKl17sR2xvADab0VOUFq4h_7REluVwtWDLEr5pW_ka_uCKbLYP14NXwPKOJiLEknOuJlc77TjIotBkSCKX_zlW7QYrFrPgtnGDTN3xTSesiqBn_8CeXkeangfAAA_22_7D__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(6470);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FTriggersPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81ZUW_jOBB_31_hl5Ma3RFhCknKvtw_WTngJG6MTY2TkJz6309gCDbYQLLt6VRp2zX2eGb8zczn8fJXcMXrN_aDf34AsEXJcS_4iaUxOCPx4nm_4Bu_BeK2XvysvnORYhEDmJeg4JSkvVlYm_UJlJJTEQMY5GU1nKM0JWwfg8BXAykpcoquMdhRXA9Uv72UCJxIwlkMEk5PGau_7FEeA7hS6zLCvAMm_4OMgX9fVxwEYcd65PPHsjEL1mY9ulFY7dMJCexC3k_FJLurl3AmMZMxKHKUYG_L5QXjWhiiZM88InFWxCDBTGKhy321y61VMP0a3BYz5IW1vAyJPWGtZziTXkFuOAZBqLxXD10a9639elrCKRf9Tf2FLjwaFQ5flXCrIGgIWk0gbfM80jY9oK3yEkBfM7tRtsFRchJFpW3OiXLliCMAkAKxgii8IEqBvwwKgFGBDePiAz9jETMuX_KUFGhLcbqYsBhuTAfdF9breI4SIq8x8JeRrjXj0kOU8gtO9dXretHdBW3wjewOZzkc_iMetx97sHCgZAB5K7DbqPfNaNxMedO0h3GGLYqHPai85uXdmJmwaDyi2xeYqr6ZZ9FGoMRldXRkz3RjnaFjbtHLTej7t9gOHG76tPu_23OEHbAg8q5aihMukAqmE0uxoESJuFtyzzDa4k6nxDR7UwX69yJ99RjSe0hIR1J9I7muSRdRjVT_6qvx96XMVS8OIs2Ttvw_NzYcKRNGw5yJVc6ckyRnVardhKRM94NVXube6sPYaq8y9BmLHeUXr4wBOkn_Ux_7dmMZKu_8JWpJ0POFrlPjUKtxIak8VPnS_8M0kKK8wDXJqf_yHm0njPQKiK8hQk8rFO_kgE_sRvhEE33Oc9nqlm65lDybcsvlQCT2at5VZSEVNwDkvEVdIUlyvNaq87xJKjePsBSXMYC62e8jZp_xkCRBtDVd8vxhVWcA91irYEvd3RwaU1RILzkQmgKp0kpPD5WNuyVZO09jEmGkz2BmdtpSnhwHRxv5BrPmY2saZEEtj_xQRuh1QGxPbucovCjG6dXH10uquakCYVUR8e6adEWxq_12FuDSuYdqK9o6dT4m8s523NRO0O_kwsyQVExIuroFlYYgaXX1HPrWJ1r_0WGcJiw_uy0_GZafv8qFl3m81U01O1Hl80gLDEHXWtCAt8xC6e2htTpDkleKY0AkoiQxLuu_ls6eYe5T0Glvu0MSM8ZUVMOhqa5hOOxBBL1yDH3zBtful_IdOtG6aJqXOm0p_CrcwmAWaZxx0HCsT9G4w5EBRjgtDPv48ZfrSODMONxQu8HrLn8NnHz0KUIVDu8ONSRyJDCTjovMLHYLw1n01mwBwOgBDuuMlH47pL4lrYahYmsMjRG5j8WMKGoQ7vJKpHlFC4aNGQ0rE3l9ZgM3E9_fjITyWNfS73mvTW_PNhM3Q2gNs7WmM_qSbqNqiUokpKmIttF2pDeqx6x5TXVF_lM1BCZPFBGdIIsKrdoxKZ646lmafkEHwxZ_Fr0NnqM3YxwdD01LPOxp_NWPMWn3cMW_U_j2ftD3zv6pHnkDB0qKe0VvnWg0ifWGjrbpwb7pGJO0FPm_01dOcrmaisJ5Dxw7E77qrjxFeEavPhbOMkFt4bt7V0snCvaO_hePUQ1fAjBdDLOfqs39epo58PJCLCEdRMUVRfD_tqim_VMBc_PzemSNXy5y_UFgAMSm2mOWkcLn9nOCUIFInAmAHEUvBSdcYagr0K_bxULyZml21Q5wH41CY1XMGSU_R_0I6U1XNJNdimwwTR5EXZrIY8hs4N0fwLrINKmz9r3rRQC4Xyasuk9IaS34ie4G2jvGpYlAbnHqkO373lFwWtKl17sR2xvADab0VOUFq4h_7REluVwtWDLEr5pW_ka_uCKbLYP14NXwPKOJiLEknOuJlc77TjIotBkSCKX_zlW7QYrFrPgtnGDTN3xTSesiqBn_8CeXkeangfAAA_22_7D__WEBPACK_IMPORTED_MODULE_2___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FTriggersPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81ZUW_jOBB_31_hl5Ma3RFhCknKvtw_WTngJG6MTY2TkJz6309gCDbYQLLt6VRp2zX2eGb8zczn8fJXcMXrN_aDf34AsEXJcS_4iaUxOCPx4nm_4Bu_BeK2XvysvnORYhEDmJeg4JSkvVlYm_UJlJJTEQMY5GU1nKM0JWwfg8BXAykpcoquMdhRXA9Uv72UCJxIwlkMEk5PGau_7FEeA7hS6zLCvAMm_4OMgX9fVxwEYcd65PPHsjEL1mY9ulFY7dMJCexC3k_FJLurl3AmMZMxKHKUYG_L5QXjWhiiZM88InFWxCDBTGKhy321y61VMP0a3BYz5IW1vAyJPWGtZziTXkFuOAZBqLxXD10a9639elrCKRf9Tf2FLjwaFQ5flXCrIGgIWk0gbfM80jY9oK3yEkBfM7tRtsFRchJFpW3OiXLliCMAkAKxgii8IEqBvwwKgFGBDePiAz9jETMuX_KUFGhLcbqYsBhuTAfdF9breI4SIq8x8JeRrjXj0kOU8gtO9dXretHdBW3wjewOZzkc_iMetx97sHCgZAB5K7DbqPfNaNxMedO0h3GGLYqHPai85uXdmJmwaDyi2xeYqr6ZZ9FGoMRldXRkz3RjnaFjbtHLTej7t9gOHG76tPu_23OEHbAg8q5aihMukAqmE0uxoESJuFtyzzDa4k6nxDR7UwX69yJ99RjSe0hIR1J9I7muSRdRjVT_6qvx96XMVS8OIs2Ttvw_NzYcKRNGw5yJVc6ckyRnVardhKRM94NVXube6sPYaq8y9BmLHeUXr4wBOkn_Ux_7dmMZKu_8JWpJ0POFrlPjUKtxIak8VPnS_8M0kKK8wDXJqf_yHm0njPQKiK8hQk8rFO_kgE_sRvhEE33Oc9nqlm65lDybcsvlQCT2at5VZSEVNwDkvEVdIUlyvNaq87xJKjePsBSXMYC62e8jZp_xkCRBtDVd8vxhVWcA91irYEvd3RwaU1RILzkQmgKp0kpPD5WNuyVZO09jEmGkz2BmdtpSnhwHRxv5BrPmY2saZEEtj_xQRuh1QGxPbucovCjG6dXH10uquakCYVUR8e6adEWxq_12FuDSuYdqK9o6dT4m8s523NRO0O_kwsyQVExIuroFlYYgaXX1HPrWJ1r_0WGcJiw_uy0_GZafv8qFl3m81U01O1Hl80gLDEHXWtCAt8xC6e2htTpDkleKY0AkoiQxLuu_ls6eYe5T0Glvu0MSM8ZUVMOhqa5hOOxBBL1yDH3zBtful_IdOtG6aJqXOm0p_CrcwmAWaZxx0HCsT9G4w5EBRjgtDPv48ZfrSODMONxQu8HrLn8NnHz0KUIVDu8ONSRyJDCTjovMLHYLw1n01mwBwOgBDuuMlH47pL4lrYahYmsMjRG5j8WMKGoQ7vJKpHlFC4aNGQ0rE3l9ZgM3E9_fjITyWNfS73mvTW_PNhM3Q2gNs7WmM_qSbqNqiUokpKmIttF2pDeqx6x5TXVF_lM1BCZPFBGdIIsKrdoxKZ646lmafkEHwxZ_Fr0NnqM3YxwdD01LPOxp_NWPMWn3cMW_U_j2ftD3zv6pHnkDB0qKe0VvnWg0ifWGjrbpwb7pGJO0FPm_01dOcrmaisJ5Dxw7E77qrjxFeEavPhbOMkFt4bt7V0snCvaO_hePUQ1fAjBdDLOfqs39epo58PJCLCEdRMUVRfD_tqim_VMBc_PzemSNXy5y_UFgAMSm2mOWkcLn9nOCUIFInAmAHEUvBSdcYagr0K_bxULyZml21Q5wH41CY1XMGSU_R_0I6U1XNJNdimwwTR5EXZrIY8hs4N0fwLrINKmz9r3rRQC4Xyasuk9IaS34ie4G2jvGpYlAbnHqkO373lFwWtKl17sR2xvADab0VOUFq4h_7REluVwtWDLEr5pW_ka_uCKbLYP14NXwPKOJiLEknOuJlc77TjIotBkSCKX_zlW7QYrFrPgtnGDTN3xTSesiqBn_8CeXkeangfAAA_22_7D__WEBPACK_IMPORTED_MODULE_2__);



var addButton = '_2ye79n15';
var attributionBadge = '_2ye79n1m';
var card = '_2ye79n19';
var cardList = '_2ye79n18';
var cardMeta = '_2ye79n1b';
var cardTop = '_2ye79n1a';
var empty = '_2ye79na';
var emptyStateLink = '_2ye79nb';
var error = '_2ye79n7';
var header = '_2ye79n1';
var headerButtons = '_2ye79n3';
var headerButtonsHiddenOnMobile = '_2ye79n17';
var historyCounter = '_2ye79n1f';
var historyEntry = '_2ye79n1h';
var historyError = '_2ye79n1j';
var historyList = '_2ye79n1g';
var historySessionLink = '_2ye79n1k';
var historyTimestamp = '_2ye79n1i';
var historyToggle = '_2ye79n1d';
var historyWrapper = '_2ye79n1e';
var iconButton = '_2ye79n14';
var lastFired = '_2ye79ny';
var loading = '_2ye79n9';
var mobileAddFab = '_2ye79n16';
var neverFired = '_2ye79nz';
var panel = '_2ye79n0';
var refreshButton = '_2ye79n6';
var retryButton = '_2ye79n8';
var row = '_2ye79nl';
var rowActions = '_2ye79n13';
var rowCount = '_2ye79n1c';
var rowDisabled = '_2ye79nm';
var showNoMatchToggle = '_2ye79n1l';
var statusBadge = '_2ye79nt';
var statusFiredFailed = '_2ye79nv';
var statusFiredSuccess = '_2ye79nu';
var statusNoMatch = '_2ye79nx';
var statusRejected = '_2ye79nw';
var subtitle = '_2ye79n5';
var tab = '_2ye79ne';
var tabActive = '_2ye79nf';
var table = '_2ye79nh';
var tableWrapper = '_2ye79ng';
var tabs = '_2ye79nd';
var td = '_2ye79nj';
var tdCenter = '_2ye79nk';
var th = '_2ye79ni';
var title = '_2ye79n4';
var titleRow = '_2ye79n2';
var toggle = '_2ye79n10';
var toggleError = '_2ye79nc';
var toggleOff = '_2ye79n12';
var toggleOn = '_2ye79n11';
var triggerName = '_2ye79nn';
var triggerSlug = '_2ye79no';
var typeBadge = '_2ye79np';
var typeCron = '_2ye79nq';
var typeGithubPush = '_2ye79nr';
var typeWebhook = '_2ye79ns';
var visuallyHidden = '_9ji8bp0';

/***/ }),

/***/ 71732:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   E: () => (/* reexport safe */ _lib_nav__WEBPACK_IMPORTED_MODULE_0__.N_)
/* harmony export */ });
/* harmony import */ var _lib_nav__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(41078);



/***/ }),

/***/ 73483:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   E: () => (/* binding */ getActiveSessionUUID),
/* harmony export */   u: () => (/* binding */ setActiveSessionUUID)
/* harmony export */ });
// Lightweight session UUID state shared between otel-init.ts (SpanProcessor)
// and any component that needs to read it (e.g. WebVitalsReporter).
//
// Kept in its own file so components can import these functions WITHOUT
// pulling in the full OTel SDK initialisation, which runs module-level code
// (new WebTracerProvider / new OTLPTraceExporter) that fails during Next.js
// SSR prerendering when the relative trace URL can't be parsed.
let activeSessionUUID = null;
function setActiveSessionUUID(uuid) {
    activeSessionUUID = uuid;
}
function getActiveSessionUUID() {
    return activeSessionUUID;
}


/***/ }),

/***/ 81124:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  M: () => (/* binding */ DiffViewer)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ./src/lib/contexts/SessionVcsContext.tsx
var SessionVcsContext = __webpack_require__(38590);
// EXTERNAL MODULE: ./src/components/shared/DiffRenderer.tsx + 1 modules
var DiffRenderer = __webpack_require__(78516);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/timestamp.js
var timestamp = __webpack_require__(28765);
// EXTERNAL MODULE: ./src/lib/utils/datetime.ts
var datetime = __webpack_require__(3335);
// EXTERNAL MODULE: ./src/components/shared/GitHubBadge.css.ts
var GitHubBadge_css = __webpack_require__(74512);
;// ./src/components/sessions/CIStatusBadge.tsx
// +feature: session:ci-status-badge
/* __next_internal_client_entry_do_not_use__ CIStatusBadge auto */ 



/** Map a GitHub CI check conclusion to the badge's display state (AC1). */ function statusInfo(checkConclusion) {
    switch(checkConclusion){
        case "success":
            return {
                label: "Passing",
                icon: "✅",
                variant: GitHubBadge_css/* prBadgeReady */.VG
            };
        case "failure":
            return {
                label: "Failing",
                icon: "❌",
                variant: GitHubBadge_css/* prBadgeBlocking */.B2
            };
        case "pending":
            return {
                label: "Pending",
                icon: "⏳",
                variant: GitHubBadge_css/* prBadgePending */.ho
            };
        default:
            return {
                label: "No checks",
                icon: "⬤",
                variant: GitHubBadge_css/* prBadgeUnknown */.o5
            };
    }
}
/**
 * Read-only CI status badge for the diff viewer header. Purely presentational over
 * already-delivered Session data (AC3) — renders nothing when the session has no
 * associated PR (AC7).
 */ function CIStatusBadge(param) {
    let { checkConclusion, prUrl, prNumber, lastChecked } = param;
    if (!prNumber) {
        return null;
    }
    const info = statusInfo(checkConclusion);
    const href = prUrl ? "".concat(prUrl, "/checks") : undefined;
    const tooltipParts = [
        "CI status: ".concat(info.label)
    ];
    if (lastChecked) {
        tooltipParts.push("checked ".concat((0,datetime/* formatRelativeTime */.fw)((0,timestamp/* timestampDate */.FP)(lastChecked).getTime())));
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("a", {
        href: href,
        target: "_blank",
        rel: "noopener noreferrer",
        className: "".concat(GitHubBadge_css/* badge */.qS, " ").concat(GitHubBadge_css/* prBadge */.Eg, " ").concat(info.variant),
        role: "status",
        "aria-label": "CI status: ".concat(info.label),
        title: tooltipParts.join(" · "),
        "data-testid": "ci-status-badge",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: "".concat(GitHubBadge_css/* icon */.Kk),
                "aria-hidden": "true",
                children: info.icon
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: "".concat(GitHubBadge_css/* text */.Qq),
                children: info.label
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FDiffViewer.css.ts.vanilla.css%22%2C%22source%22%3A%22LmU0MzE5dDAgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejJ6KSB2YXIoLS1fMTllejJyejMwKTsKfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FDiffViewer_css_ts_vanilla_css_22_2C_22source_22_3A_22LmU0MzE5dDAgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejJ6KSB2YXIoLS1fMTllejJyejMwKTsKfQ_22_7D = __webpack_require__(58252);
;// ./src/components/sessions/DiffViewer.css.ts


var diffHeader = 'e4319t0';
;// ./src/components/sessions/DiffViewer.tsx
/* __next_internal_client_entry_do_not_use__ DiffViewer auto */ 




/** Session-aware diff viewer — reads from SessionVcsContext. */ function DiffViewer(param) {
    let { session } = param;
    const { diff: rawDiff, diffLoading: loading, refreshDiff } = (0,SessionVcsContext/* useSessionVcsContext */.l)();
    var _rawDiff_content, _rawDiff_added, _rawDiff_removed;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            (session === null || session === void 0 ? void 0 : session.githubPrNumber) ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: diffHeader,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(CIStatusBadge, {
                    checkConclusion: session.githubCheckConclusion,
                    prUrl: session.githubPrUrl,
                    prNumber: session.githubPrNumber,
                    lastChecked: session.lastPrStatusCheck
                })
            }) : null,
            /*#__PURE__*/ (0,jsx_runtime.jsx)(DiffRenderer/* DiffRenderer */.F, {
                content: (_rawDiff_content = rawDiff === null || rawDiff === void 0 ? void 0 : rawDiff.content) !== null && _rawDiff_content !== void 0 ? _rawDiff_content : "",
                added: (_rawDiff_added = rawDiff === null || rawDiff === void 0 ? void 0 : rawDiff.added) !== null && _rawDiff_added !== void 0 ? _rawDiff_added : 0,
                removed: (_rawDiff_removed = rawDiff === null || rawDiff === void 0 ? void 0 : rawDiff.removed) !== null && _rawDiff_removed !== void 0 ? _rawDiff_removed : 0,
                loading: loading,
                onRefresh: refreshDiff
            })
        ]
    });
}


/***/ }),

/***/ 89107:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   sk: () => (/* binding */ FILES_CHORDS),
/* harmony export */   sn: () => (/* binding */ matchChord)
/* harmony export */ });
/* unused harmony export chordSignature */
// The Files tab's registered chords (ux.md 14.1). One table drives the matcher, the shortcut overlay and
// the reservedChords test, so a chord cannot be documented without being matched, or the reverse.
const FILES_CHORDS = [
    {
        id: "quickOpen",
        label: "Quick open",
        mod: true,
        key: "p",
        equivalent: "Search files button (data-testid files-quick-open)"
    },
    {
        id: "search",
        label: "Search in files",
        mod: true,
        shift: true,
        key: "f",
        equivalent: "Search tab in the Explorer | Search header"
    },
    {
        id: "gotoLine",
        label: "Go to line",
        mod: true,
        key: "g",
        equivalent: "Go to line button in the navigation row"
    },
    {
        id: "treeFilter",
        label: "Filter the file tree",
        mod: true,
        key: "f",
        equivalent: "File tree filter field"
    },
    {
        id: "back",
        label: "Jumplist back",
        alt: true,
        code: "BracketLeft",
        equivalent: "Back button in the navigation row"
    },
    {
        id: "forward",
        label: "Jumplist forward",
        alt: true,
        code: "BracketRight",
        equivalent: "Forward button in the navigation row"
    },
    {
        id: "closeTab",
        label: "Close tab",
        alt: true,
        code: "KeyW",
        equivalent: "Close button on the tab"
    },
    {
        id: "save",
        label: "Save",
        mod: true,
        key: "s",
        equivalent: "Save button in the status bar"
    },
    {
        id: "nextHit",
        label: "Next search hit",
        key: "F4",
        equivalent: "Next hit chevron on the quickfix chip"
    },
    {
        id: "prevHit",
        label: "Previous search hit",
        shift: true,
        key: "F4",
        equivalent: "Previous hit chevron on the quickfix chip"
    }
];
/** AltGr is reported as Ctrl+Alt on many Windows layouts; a character typed with it is never a chord. */ const isAltGr = (e)=>e.altKey && e.ctrlKey || typeof e.getModifierState === "function" && e.getModifierState("AltGraph");
/** The chord a keydown is, or null; the caller decides whether it is allowed to act on it. */ function matchChord(e) {
    var _e_key;
    if (isAltGr(e)) return null;
    const mod = e.ctrlKey || e.metaKey;
    const key = (_e_key = e.key) === null || _e_key === void 0 ? void 0 : _e_key.toLowerCase();
    var _FILES_CHORDS_find;
    return (_FILES_CHORDS_find = FILES_CHORDS.find((c)=>{
        var _c_key;
        if (!!c.mod !== mod || !!c.alt !== e.altKey || !!c.shift !== e.shiftKey) return false;
        if (c.code) return e.code === c.code;
        return key === ((_c_key = c.key) === null || _c_key === void 0 ? void 0 : _c_key.toLowerCase());
    })) !== null && _FILES_CHORDS_find !== void 0 ? _FILES_CHORDS_find : null;
}
/** Canonical form used to compare a chord against the denylist, e.g. `Mod+Shift+F`. */ function chordSignature(c) {
    var _c_key, _c_code;
    const keyName = (_c_code = c.code) !== null && _c_code !== void 0 ? _c_code : ((_c_key = c.key) !== null && _c_key !== void 0 ? _c_key : "").toUpperCase();
    return [
        c.mod && "Mod",
        c.alt && "Alt",
        c.shift && "Shift",
        keyName
    ].filter(Boolean).join("+");
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

/***/ })

}]);
//# sourceMappingURL=3228.6754f5c20e890a4b.js.map