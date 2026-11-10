(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[4201],{

/***/ 11272:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   O: () => (/* binding */ useSessionRepoPaths)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _lib_store__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(93602);
/* harmony import */ var _lib_store_sessionsSlice__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(60795);
/* __next_internal_client_entry_do_not_use__ useSessionRepoPaths auto */ 


/** Returns deduplicated list of repo root paths from all known sessions. */ function useSessionRepoPaths() {
    const sessions = (0,_lib_store__WEBPACK_IMPORTED_MODULE_1__/* .useAppSelector */ .GV)(_lib_store_sessionsSlice__WEBPACK_IMPORTED_MODULE_2__/* .selectActiveSessionsSortedByUpdatedAt */ .od);
    return (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>{
        const seen = new Set();
        const paths = [];
        for (const s of sessions){
            const p = s.repoRoot;
            if (p && !seen.has(p)) {
                seen.add(p);
                paths.push(p);
            }
        }
        return paths;
    }, [
        sessions
    ]);
}


/***/ }),

/***/ 34609:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  E: () => (/* binding */ RepoPathInput)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/hooks/usePathCompletions.ts
var usePathCompletions = __webpack_require__(44762);
// EXTERNAL MODULE: ./src/lib/hooks/useSessionRepoPaths.ts
var useSessionRepoPaths = __webpack_require__(11272);
// EXTERNAL MODULE: ./src/lib/hooks/useGitHubEnterpriseHosts.ts
var useGitHubEnterpriseHosts = __webpack_require__(44635);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ./src/gen/session/v1/session_pb.ts
var session_pb = __webpack_require__(23575);
// EXTERNAL MODULE: ./src/lib/api/transport.ts
var transport = __webpack_require__(21694);
// EXTERNAL MODULE: ./src/lib/hooks/useAbortableEffect.ts
var useAbortableEffect = __webpack_require__(64343);
;// ./src/lib/hooks/useRepoPathSuggestions.ts
/* __next_internal_client_entry_do_not_use__ normalizePath,useRepoPathSuggestions auto */ 




const DEBOUNCE_MS = 150;
// Shorter than the server's own listWorktreesTimeout (20s, path_completion_service.go)
// so a genuinely stuck git subprocess surfaces as "unresolved" here well before
// the server would time it out itself.
const REQUEST_TIMEOUT_MS = 5000;
/** Strips a trailing slash so a candidate path matches its ListWorktrees entry regardless of how it was typed/stored. */ function normalizePath(p) {
    return p.length > 1 ? p.replace(/\/+$/, "") : p;
}
/** Merges one candidate path's resolved worktree family into `cache`. Returns true if anything new was added. */ function mergeWorktreeFamily(cache, worktrees) {
    const main = worktrees.find((w)=>w.isMain);
    if (!main) return false;
    const rootPath = normalizePath(main.path);
    let changed = false;
    for (const w of worktrees){
        const wp = normalizePath(w.path);
        if (!cache.has(wp)) {
            cache.set(wp, {
                rootPath,
                isMain: w.isMain,
                branch: w.branch
            });
            changed = true;
        }
    }
    return changed;
}
/**
 * Resolves one candidate, bounded by its own timeout so a stuck request
 * can't stay pending past REQUEST_TIMEOUT_MS regardless of what the outer
 * effect's shared `signal` does. Merges into `cache` and calls `onChange`
 * as soon as this specific request settles — independent of any sibling.
 */ async function resolveOnePath(client, repoPath, cache, outerSignal, onChange) {
    const controller = new AbortController();
    const onAbort = ()=>controller.abort();
    outerSignal.addEventListener("abort", onAbort);
    const timeoutTimer = setTimeout(()=>controller.abort(), REQUEST_TIMEOUT_MS);
    try {
        const result = await client.listWorktrees({
            repoPath
        }, {
            signal: controller.signal
        });
        if (mergeWorktreeFamily(cache, result.worktrees || [])) {
            onChange();
        }
    } catch (e) {
    // Aborted, timed out, or a network/server error — leave unresolved so
    // it's simply retried the next time it's a candidate.
    } finally{
        clearTimeout(timeoutTimer);
        outerSignal.removeEventListener("abort", onAbort);
    }
}
async function resolveUnresolvedPaths(candidates, cache, signal, onChange) {
    const unresolved = Array.from(new Set(candidates.map(normalizePath))).filter((p)=>p && !cache.has(p));
    if (unresolved.length === 0) return;
    const client = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
    await Promise.all(unresolved.map((repoPath)=>resolveOnePath(client, repoPath, cache, signal, onChange)));
}
function useRepoPathSuggestions(paths) {
    const [version, setVersion] = (0,react.useState)(0);
    const cacheRef = (0,react.useRef)(new Map());
    const [debounced, setDebounced] = (0,react.useState)([]);
    const key = paths.join("\n");
    (0,react.useEffect)(()=>{
        const timer = setTimeout(()=>setDebounced(paths), DEBOUNCE_MS);
        return ()=>clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        key
    ]);
    const debouncedKey = debounced.join("\n");
    (0,useAbortableEffect/* useAbortableEffect */.B)((signal)=>resolveUnresolvedPaths(debounced, cacheRef.current, signal, ()=>setVersion((v)=>v + 1)), // eslint-disable-next-line react-hooks/exhaustive-deps
    [
        debouncedKey
    ]);
    return {
        resolutions: cacheRef.current,
        version
    };
}

// EXTERNAL MODULE: ./src/components/ui/PathCompletionDropdown.tsx + 1 modules
var PathCompletionDropdown = __webpack_require__(81551);
// EXTERNAL MODULE: ./src/lib/github/urlParser.ts
var urlParser = __webpack_require__(11538);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fui%2FRepoPathInput.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61UwW7bMAy95yu4w4AEmw3JabZO_ZhCtmhbqCIKkpw4Hvbvg-Wkq521yaGwD4L4yEc96il_5o_dY2E8g98rAEdBR01WgEcjoz7g0wrgqFVsBXDGvj6t_qzySw5PObMogJNKadsIOEi_zrJn_guHwg_FsFnsbNlmxJeyemk8dVYtU7hMgIoM-atYOSWTV-gFcNdDIKPVAvbyBpV5qXQXlpW2PxOmJhuzoAe8ipf_4rXca3O6OlqXENRFoy0KsGSTbNFLe5Hz3EI6C7Cc7wKgDDh114_ESbMzrKQ-FcA-ZnRAXxs6CkBjtAs6zGcgaqq6kCbxlmTZpNm8crVSjeVY-grXL6UdBZkxCGdkhS0ZhT4R_X8k1U2hDsvKSgdZGlSpKjlZ6XgSwPIfafCdDyONpZhJY-iI6uMLo-bli5uicLaBL3rvyEdp4zz5Tl05u19YPu9v-76WxY07OTljL32jbRbJXeWf5lQP71PtP5lqt3hIZBnIdHFyxJheSVOtx9cCvo0ipfIG6yiAjUuvm_ayHjJtFfYCdozdY3e82-7zeT24Hvg4MN-Ucs2-w_nP-W6y9qsHW60U2o-vYfLPX3AcalpZBQAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FRepoPathInput_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61UwW7bMAy95yu4w4AEmw3JabZO_ZhCtmhbqCIKkpw4Hvbvg_Wkq521yaGwD4L4yEc96il_5o_dY2E8g98rAEdBR01WgEcjoz7g0wrgqFVsBXDGvj6t_qzySw5PObMogJNKadsIOEi_zrJn_guHwg_FsFnsbNlmxJeyemk8dVYtU7hMgIoM_atYOSWTV_gFcNdDIKPVAvbyBpV5qXQXlpW2PxOmJhuzoAe8ipf_4rXca3O6OlqXENRFoy0KsGSTbNFLe5Hz3EI6C7Cc7wKgDDh114_ESbMzrKQ_FcA_ZnRAXxs6CkBjtAs6zGcgaqq6kCbxlmTZpNm8crVSjeVY_grXL6UdBZkxCGdkhS0ZhT4R_X8k1U2hDsvKSgdZGlSpKjlZ6XgSwPIfafCdDyONpZhJY_iI6uMLo_bli5uicLaBL3rvyEdp4zz5Tl05u19YPu9v_76WxY07OTljL32jbRbJXeWf5lQP71PtP5lqt3hIZBnIdHFyxJheSVOtx9cCvo0ipfIG6yiAjUuvm_ayHjJtFfYCdozdY3e82_7zeT24Hvg4MN_Ucs2_w_nP_W6y9qsHW60U2o_vYfLPX3AcalpZBQAA_22_7D = __webpack_require__(12646);
;// ./src/components/ui/RepoPathInput.css.ts


var container = '_18u82lr0';
var dropdownWrapper = '_18u82lr5';
var githubHint = '_18u82lr4';
var RepoPathInput_css_hint = '_18u82lr3';
var input = '_18u82lr1';
var inputError = '_18u82lr2';
;// ./src/components/ui/RepoPathInput.tsx
/* __next_internal_client_entry_do_not_use__ RepoPathInput auto */ 








const MAX_HISTORY = 5;
function tildeAbbreviate(p) {
    const m = p.match(/^(\/(?:Users|home)\/[^/]+)(\/.*)?$/);
    var _m_;
    return m ? "~".concat((_m_ = m[2]) !== null && _m_ !== void 0 ? _m_ : "") : p;
}
function makeHistoryEntry(p, worktreeOf) {
    return {
        name: tildeAbbreviate(p),
        path: p,
        isDirectory: true,
        isHistory: true,
        ...worktreeOf ? {
            isWorktree: true,
            rootLabel: tildeAbbreviate(worktreeOf)
        } : {}
    };
}
function groupByResolvedRoot(paths, resolutions) {
    const groupOrder = [];
    const groups = new Map();
    for (const p of paths){
        const info = resolutions.get(normalizePath(p));
        const groupKey = info ? info.rootPath : normalizePath(p);
        let group = groups.get(groupKey);
        if (!group) {
            var _info_rootPath;
            group = {
                rootPath: null,
                discoveredRoot: (_info_rootPath = info === null || info === void 0 ? void 0 : info.rootPath) !== null && _info_rootPath !== void 0 ? _info_rootPath : null,
                members: []
            };
            groups.set(groupKey, group);
            groupOrder.push(groupKey);
        }
        if (!info || info.isMain) {
            group.rootPath = p;
        } else {
            group.members.push(p);
        }
    }
    return {
        groupOrder,
        groups
    };
}
function buildGroupedHistoryEntries(paths, resolutions) {
    const { groupOrder, groups } = groupByResolvedRoot(paths, resolutions);
    const entries = [];
    for (const groupKey of groupOrder){
        const group = groups.get(groupKey);
        if (!group) continue;
        if (group.rootPath) {
            entries.push(makeHistoryEntry(group.rootPath));
        } else if (group.discoveredRoot) {
            entries.push(makeHistoryEntry(group.discoveredRoot));
        }
        for (const member of group.members){
            const info = resolutions.get(normalizePath(member));
            entries.push(makeHistoryEntry(member, info === null || info === void 0 ? void 0 : info.rootPath));
        }
    }
    return entries;
}
function RepoPathInput(param) {
    let { id: idProp, value, onChange, disabled = false, error, placeholder = "/path/to/repo", required = false, hint, detectGitHubUrl = false, onSelect, "aria-label": ariaLabel, onEnter, "data-testid": testId } = param;
    const generatedId = (0,react.useId)();
    const id = idProp !== null && idProp !== void 0 ? idProp : generatedId;
    const listboxId = "".concat(id, "-listbox");
    const [open, setOpen] = (0,react.useState)(false);
    const [selectedIndex, setSelectedIndex] = (0,react.useState)(-1);
    const containerRef = (0,react.useRef)(null);
    const historyPaths = (0,useSessionRepoPaths/* useSessionRepoPaths */.O)();
    const { entries: fsEntries, isLoading } = (0,usePathCompletions/* usePathCompletions */.C)(value, {
        enabled: value.length > 0,
        directoriesOnly: true
    });
    const { hosts: enterpriseHosts } = (0,useGitHubEnterpriseHosts/* useGitHubEnterpriseHosts */.P)();
    const detectedRepo = (0,react.useMemo)(()=>{
        if (!detectGitHubUrl || !value.trim() || !(0,urlParser/* isGitHubRef */.Pu)(value, enterpriseHosts)) return null;
        return (0,urlParser/* parseGitHubRef */.GM)(value, enterpriseHosts);
    }, [
        detectGitHubUrl,
        value,
        enterpriseHosts
    ]);
    const historyCandidates = (0,react.useMemo)(()=>historyPaths.filter((p)=>value === "" || p.toLowerCase().includes(value.toLowerCase())).slice(0, MAX_HISTORY), [
        historyPaths,
        value
    ]);
    // `resolutions` is mutated in place as worktree families resolve in the
    // background, so `version` (not `resolutions` itself) is the signal that
    // grouping needs to be recomputed — see useRepoPathSuggestions' doc comment.
    const { resolutions, version } = useRepoPathSuggestions(historyCandidates);
    const { allEntries, historyCount } = (0,react.useMemo)(()=>{
        const historyEntries = buildGroupedHistoryEntries(historyCandidates, resolutions);
        const historySet = new Set(historyEntries.map((e)=>e.path));
        const fsCompletionEntries = fsEntries.filter((e)=>!historySet.has(e.path)).map((e)=>({
                name: e.name,
                path: e.path,
                isDirectory: e.isDirectory,
                isHistory: false
            }));
        return {
            allEntries: [
                ...historyEntries,
                ...fsCompletionEntries
            ],
            historyCount: historyEntries.length
        };
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        historyCandidates,
        resolutions,
        version,
        fsEntries
    ]);
    const showDropdown = open && (allEntries.length > 0 || isLoading);
    const handleSelect = (0,react.useCallback)((entry)=>{
        onChange(entry.path);
        onSelect === null || onSelect === void 0 ? void 0 : onSelect(entry);
        setOpen(false);
        setSelectedIndex(-1);
    }, [
        onChange,
        onSelect
    ]);
    const handleKeyDown = (0,react.useCallback)((e)=>{
        // Focus opens the list, so Tab must close it or it stays over the next control.
        if (e.key === "Tab") {
            setOpen(false);
            setSelectedIndex(-1);
            return;
        }
        if (e.key === "Enter" && onEnter && !e.nativeEvent.isComposing && e.keyCode !== 229 && // Safari fires the IME-confirm Enter after compositionend with keyCode 229
        !(open && selectedIndex >= 0 && selectedIndex < allEntries.length)) {
            e.preventDefault();
            setOpen(false);
            onEnter();
            return;
        }
        if (!open) {
            if (e.key === "ArrowDown" || e.key === "ArrowUp") {
                setOpen(true);
            }
            return;
        }
        switch(e.key){
            case "ArrowDown":
                e.preventDefault();
                setSelectedIndex((i)=>Math.min(i + 1, allEntries.length - 1));
                break;
            case "ArrowUp":
                e.preventDefault();
                setSelectedIndex((i)=>Math.max(i - 1, -1));
                break;
            case "Enter":
                if (selectedIndex >= 0 && selectedIndex < allEntries.length) {
                    e.preventDefault();
                    handleSelect(allEntries[selectedIndex]);
                }
                break;
            case "Escape":
                if (showDropdown) {
                    // Both calls are required: `e.stopPropagation()` sets React's own
                    // `isPropagationStopped` flag, which is what stops the synthetic bubble
                    // from reaching an ancestor's own `onKeyDown` (e.g. Omnibar's modal div);
                    // `e.nativeEvent.stopImmediatePropagation()` additionally stops the raw
                    // native event from reaching a real `document.addEventListener("keydown", ...)`
                    // ancestor (e.g. NewShellDialog's cancel-on-Escape listener). Neither call
                    // alone covers both ancestor shapes — see RepoPathInput.test.tsx's
                    // "does not bubble to a parent's own keydown handler" case. Note:
                    // stopImmediatePropagation also blocks any OTHER listener on this same
                    // input for this event, not just ancestors — fine today (only one keydown
                    // listener lives on this node) but worth remembering before adding a second.
                    e.stopPropagation();
                    e.nativeEvent.stopImmediatePropagation();
                    // Also cancel the default so a native <dialog> doesn't close on this Escape.
                    e.preventDefault();
                }
                setOpen(false);
                setSelectedIndex(-1);
                break;
        }
    }, [
        open,
        allEntries,
        selectedIndex,
        handleSelect,
        showDropdown,
        onEnter
    ]);
    (0,react.useEffect)(()=>{
        function handleClickOutside(e) {
            if (containerRef.current && !containerRef.current.contains(e.target)) {
                setOpen(false);
                setSelectedIndex(-1);
            }
        }
        document.addEventListener("mousedown", handleClickOutside);
        return ()=>document.removeEventListener("mousedown", handleClickOutside);
    }, []);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        ref: containerRef,
        className: container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                id: id,
                type: "text",
                className: [
                    input,
                    error ? inputError : ""
                ].filter(Boolean).join(" "),
                value: value,
                onChange: (e)=>{
                    onChange(e.target.value);
                    setOpen(true);
                    setSelectedIndex(-1);
                },
                onFocus: ()=>setOpen(true),
                onKeyDown: handleKeyDown,
                placeholder: placeholder,
                title: value || undefined,
                required: required,
                "aria-required": required || undefined,
                "aria-invalid": error ? true : undefined,
                role: "combobox",
                "aria-haspopup": "listbox",
                "aria-expanded": showDropdown,
                "aria-autocomplete": "list",
                "aria-controls": showDropdown ? listboxId : undefined,
                "aria-activedescendant": showDropdown && selectedIndex >= 0 ? "".concat(listboxId, "-option-").concat(selectedIndex) : undefined,
                "aria-describedby": error ? "".concat(id, "-error") : undefined,
                "aria-label": ariaLabel,
                disabled: disabled,
                "data-testid": testId,
                autoComplete: "off",
                spellCheck: false
            }),
            showDropdown && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: dropdownWrapper,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(PathCompletionDropdown/* PathCompletionDropdown */.h, {
                    id: listboxId,
                    entries: allEntries,
                    selectedIndex: selectedIndex,
                    onSelect: handleSelect,
                    isLoading: isLoading,
                    historyCount: historyCount
                })
            }),
            detectedRepo ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                className: githubHint,
                "data-testid": "repo-path-github-hint",
                children: [
                    "Will clone ",
                    (0,urlParser/* getRepoFullName */.p2)(detectedRepo),
                    " to",
                    " ",
                    "~/.stapler-squad/repos/".concat(detectedRepo.host || "github.com", "/").concat(detectedRepo.owner, "/").concat(detectedRepo.repo),
                    " ",
                    "when you save."
                ]
            }) : hint && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: RepoPathInput_css_hint,
                children: hint
            })
        ]
    });
}


/***/ }),

/***/ 44762:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   C: () => (/* binding */ usePathCompletions)
/* harmony export */ });
/* unused harmony export clearCompletionCache */
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(66144);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(23575);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(21694);
/* __next_internal_client_entry_do_not_use__ clearCompletionCache,usePathCompletions auto */ 



const DEBOUNCE_MS = 150;
const CACHE_MAX = 100;
const CACHE_TTL_MS = 30000;
const completionCache = new Map();
/** Clears all cached completions. Exposed for testing. */ function clearCompletionCache() {
    completionCache.clear();
}
function getCached(key) {
    const entry = completionCache.get(key);
    if (!entry) return null;
    if (Date.now() > entry.expiresAt) {
        completionCache.delete(key);
        return null;
    }
    // Refresh LRU position.
    completionCache.delete(key);
    completionCache.set(key, entry);
    return entry;
}
function setCached(key, value) {
    if (completionCache.size >= CACHE_MAX) {
        const firstKey = completionCache.keys().next().value;
        if (firstKey !== undefined) {
            completionCache.delete(firstKey);
        }
    }
    completionCache.set(key, value);
}
/**
 * Fetches real-time filesystem path completions for a given path prefix.
 *
 * Three-layer protection against redundant requests:
 *  1. 150ms debounce – coalesces rapid keystrokes.
 *  2. AbortController – cancels the in-flight HTTP request when the prefix changes.
 *  3. Generation counter – discards responses that arrive after a newer request fired.
 *
 * Results are cached in a module-level LRU (100 entries, 30s TTL) to avoid
 * redundant RPCs for burst typing that returns to the same prefix.
 */ function usePathCompletions(pathPrefix) {
    let options = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : {};
    const { enabled = true, directoriesOnly = true, maxResults = 50 } = options;
    const [result, setResult] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)({
        entries: [],
        baseDir: "",
        baseDirExists: false,
        pathExists: false,
        isLoading: false,
        error: null
    });
    const generationRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(0);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        if (!enabled || pathPrefix === "") {
            setResult((prev)=>({
                    ...prev,
                    entries: [],
                    baseDirExists: false,
                    pathExists: false,
                    isLoading: false,
                    error: null
                }));
            return;
        }
        const generation = ++generationRef.current;
        // Serve from cache immediately (no loading state needed).
        const cacheKey = "".concat(pathPrefix, "::").concat(directoriesOnly, "::").concat(maxResults);
        const cached = getCached(cacheKey);
        if (cached) {
            setResult({
                entries: cached.entries,
                baseDir: cached.baseDir,
                baseDirExists: cached.baseDirExists,
                pathExists: cached.pathExists,
                isLoading: false,
                error: null
            });
            return;
        }
        setResult((prev)=>({
                ...prev,
                isLoading: true,
                error: null
            }));
        const abortController = new AbortController();
        const debounceTimer = setTimeout(async ()=>{
            try {
                const client = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_2__/* .getConnectTransport */ .qW)());
                const response = await client.listPathCompletions({
                    pathPrefix,
                    directoriesOnly,
                    maxResults
                }, {
                    signal: abortController.signal
                });
                if (generation !== generationRef.current) return;
                const newResult = {
                    entries: response.entries,
                    baseDir: response.baseDir,
                    baseDirExists: response.baseDirExists,
                    pathExists: response.pathExists
                };
                setCached(cacheKey, {
                    ...newResult,
                    expiresAt: Date.now() + CACHE_TTL_MS
                });
                setResult({
                    ...newResult,
                    isLoading: false,
                    error: null
                });
            } catch (err) {
                if (generation !== generationRef.current) return;
                if (abortController.signal.aborted) return;
                const message = err instanceof Error ? err.message : "Failed to fetch completions";
                setResult((prev)=>({
                        ...prev,
                        isLoading: false,
                        error: message
                    }));
            }
        }, DEBOUNCE_MS);
        return ()=>{
            clearTimeout(debounceTimer);
            abortController.abort();
        };
    }, [
        pathPrefix,
        enabled,
        directoriesOnly,
        maxResults
    ]);
    return result;
}


/***/ }),

/***/ 75938:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  w: () => (/* binding */ DuplicatePendingBadge)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FDuplicatePendingBadge.css.ts.vanilla.css%22%2C%22source%22%3A%22LnI2NjdqMCB7CiAgZGlzcGxheTogaW5saW5lLWZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBib3JkZXItcmFkaXVzOiB2YXIoLS1fMTllejJyejM2KTsKICBwYWRkaW5nOiAxcHggdmFyKC0tXzE5ZXoycnoyeik7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNhKTsKICBmb250LXdlaWdodDogdmFyKC0tXzE5ZXoycnozaCk7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnoxMSk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MTIpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6MTApOwogIHdoaXRlLXNwYWNlOiBub3dyYXA7Cn0%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FDuplicatePendingBadge_css_ts_vanilla_css_22_2C_22source_22_3A_22LnI2NjdqMCB7CiAgZGlzcGxheTogaW5saW5lLWZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBib3JkZXItcmFkaXVzOiB2YXIoLS1fMTllejJyejM2KTsKICBwYWRkaW5nOiAxcHggdmFyKC0tXzE5ZXoycnoyeik7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNhKTsKICBmb250LXdlaWdodDogdmFyKC0tXzE5ZXoycnozaCk7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnoxMSk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MTIpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6MTApOwogIHdoaXRlLXNwYWNlOiBub3dyYXA7Cn0_22_7D = __webpack_require__(27883);
;// ./src/components/backlog/DuplicatePendingBadge.css.ts


var badge = 'r667j0';
;// ./src/components/backlog/DuplicatePendingBadge.tsx
/* __next_internal_client_entry_do_not_use__ DuplicatePendingBadge auto */ 
// +feature: backlog:duplicate-pending-badge

/** Marks an item in review whose report_duplicate claim awaits operator confirmation, so it reads differently from a real in-review PR. */ function DuplicatePendingBadge(param) {
    let { duplicateRef } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
        className: badge,
        "data-testid": "duplicate-pending-badge",
        title: duplicateRef ? "Claimed duplicate of ".concat(duplicateRef, " — awaiting confirmation") : "Duplicate claimed — awaiting confirmation",
        children: "Duplicate? awaiting confirmation"
    });
}


/***/ }),

/***/ 81551:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  h: () => (/* binding */ PathCompletionDropdown)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fui%2FPathCompletionDropdown.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll85YXB0bmYwIHsKICBsaXN0LXN0eWxlOiBub25lOwogIG1hcmdpbjogMDsKICBwYWRkaW5nOiA0cHggMDsKICBtYXgtaGVpZ2h0OiAyMDBweDsKICBvdmVyZmxvdy15OiBhdXRvOwogIGJvcmRlci1ib3R0b206IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6Nyk7Cn0KLl85YXB0bmYxIHsKICBkaXNwbGF5OiBmbGV4OwogIGFsaWduLWl0ZW1zOiBmbGV4LXN0YXJ0OwogIGdhcDogOHB4OwogIHBhZGRpbmc6IDdweCAxNnB4OwogIGN1cnNvcjogcG9pbnRlcjsKICBmb250LXNpemU6IDEzcHg7CiAgZm9udC1mYW1pbHk6IHZhcigtLV8xOWV6MnJ6MnUpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejApOwogIHVzZXItc2VsZWN0OiBub25lOwp9Ci5fOWFwdG5mMTpob3ZlciB7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnoxNyk7Cn0KLl85YXB0bmYyIHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejE3KTsKfQouXzlhcHRuZjMgewogIGZvbnQtc2l6ZTogMTRweDsKICBmbGV4LXNocmluazogMDsKICB3aWR0aDogMThweDsKICB0ZXh0LWFsaWduOiBjZW50ZXI7Cn0KLl85YXB0bmY0IHsKICBmbGV4OiAxOwogIG92ZXJmbG93LXdyYXA6IGFueXdoZXJlOwp9Ci5fOWFwdG5mNSB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7CiAgZmxleC1zaHJpbms6IDA7Cn0KLl85YXB0bmY2IHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKfQouXzlhcHRuZjcgewogIGhlaWdodDogMXB4OwogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6ZSk7CiAgbWFyZ2luOiA0cHggOHB4OwogIHBhZGRpbmc6IDA7CiAgbGlzdC1zdHlsZTogbm9uZTsKfQouXzlhcHRuZjggewogIHBhZGRpbmc6IDEwcHggMTZweDsKICBmb250LXNpemU6IDEzcHg7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7CiAgYm9yZGVyLWJvdHRvbTogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6ZSk7Cn0%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FPathCompletionDropdown_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll85YXB0bmYwIHsKICBsaXN0LXN0eWxlOiBub25lOwogIG1hcmdpbjogMDsKICBwYWRkaW5nOiA0cHggMDsKICBtYXgtaGVpZ2h0OiAyMDBweDsKICBvdmVyZmxvdy15OiBhdXRvOwogIGJvcmRlci1ib3R0b206IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6Nyk7Cn0KLl85YXB0bmYxIHsKICBkaXNwbGF5OiBmbGV4OwogIGFsaWduLWl0ZW1zOiBmbGV4LXN0YXJ0OwogIGdhcDogOHB4OwogIHBhZGRpbmc6IDdweCAxNnB4OwogIGN1cnNvcjogcG9pbnRlcjsKICBmb250LXNpemU6IDEzcHg7CiAgZm9udC1mYW1pbHk6IHZhcigtLV8xOWV6MnJ6MnUpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejApOwogIHVzZXItc2VsZWN0OiBub25lOwp9Ci5fOWFwdG5mMTpob3ZlciB7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnoxNyk7Cn0KLl85YXB0bmYyIHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejE3KTsKfQouXzlhcHRuZjMgewogIGZvbnQtc2l6ZTogMTRweDsKICBmbGV4LXNocmluazogMDsKICB3aWR0aDogMThweDsKICB0ZXh0LWFsaWduOiBjZW50ZXI7Cn0KLl85YXB0bmY0IHsKICBmbGV4OiAxOwogIG92ZXJmbG93LXdyYXA6IGFueXdoZXJlOwp9Ci5fOWFwdG5mNSB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7CiAgZmxleC1zaHJpbms6IDA7Cn0KLl85YXB0bmY2IHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKfQouXzlhcHRuZjcgewogIGhlaWdodDogMXB4OwogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6ZSk7CiAgbWFyZ2luOiA0cHggOHB4OwogIHBhZGRpbmc6IDA7CiAgbGlzdC1zdHlsZTogbm9uZTsKfQouXzlhcHRuZjggewogIHBhZGRpbmc6IDEwcHggMTZweDsKICBmb250LXNpemU6IDEzcHg7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7CiAgYm9yZGVyLWJvdHRvbTogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6ZSk7Cn0_22_7D = __webpack_require__(61128);
;// ./src/components/ui/PathCompletionDropdown.css.ts


var divider = '_9aptnf7';
var dropdown = '_9aptnf0';
var icon = '_9aptnf3';
var item = '_9aptnf1';
var itemHistory = '_9aptnf6';
var itemSelected = '_9aptnf2';
var loading = '_9aptnf8';
var PathCompletionDropdown_css_name = '_9aptnf4';
var suffix = '_9aptnf5';
;// ./src/components/ui/PathCompletionDropdown.tsx
/* __next_internal_client_entry_do_not_use__ PathCompletionDropdown auto */ 

function EntryItem(param) {
    let { entry, index, selectedIndex, onSelect, id } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
        id: "".concat(id, "-option-").concat(index),
        className: [
            item,
            index === selectedIndex ? itemSelected : "",
            entry.isHistory ? itemHistory : ""
        ].filter(Boolean).join(" "),
        role: "option",
        "aria-selected": index === selectedIndex,
        title: entry.path,
        onMouseDown: (e)=>{
            e.preventDefault();
            onSelect(entry);
        },
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: icon,
                "aria-hidden": "true",
                children: entry.isHistory ? "🕒" : entry.isDirectory ? "📁" : "📄"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: PathCompletionDropdown_css_name,
                children: entry.name
            }),
            entry.isWorktree && entry.rootLabel && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                className: suffix,
                children: [
                    "worktree of ",
                    entry.rootLabel
                ]
            }),
            entry.isDirectory && !entry.isHistory && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: suffix,
                "aria-hidden": "true",
                children: "/"
            })
        ]
    });
}
function PathCompletionDropdown(param) {
    let { entries, selectedIndex, onSelect, isLoading, historyCount = 0, id = "path-completion-listbox" } = param;
    if (isLoading && entries.length === 0) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: loading,
            children: "Loading completions…"
        });
    }
    if (entries.length === 0) return null;
    const showDivider = historyCount > 0 && historyCount < entries.length;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("ul", {
        id: id,
        className: dropdown,
        role: "listbox",
        "aria-label": "Path completions",
        "data-testid": "path-completion-dropdown",
        children: [
            entries.slice(0, historyCount).map((entry, i)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(EntryItem, {
                    entry: entry,
                    index: i,
                    selectedIndex: selectedIndex,
                    onSelect: onSelect,
                    id: id
                }, entry.path)),
            showDivider && /*#__PURE__*/ (0,jsx_runtime.jsx)("li", {
                className: divider,
                role: "presentation",
                "aria-hidden": "true"
            }),
            entries.slice(historyCount).map((entry, i)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(EntryItem, {
                    entry: entry,
                    index: historyCount + i,
                    selectedIndex: selectedIndex,
                    onSelect: onSelect,
                    id: id
                }, entry.path))
        ]
    });
}


/***/ }),

/***/ 85732:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 86781));


/***/ }),

/***/ 86781:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  "default": () => (/* binding */ BacklogBoardPage)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/nav/index.ts
var nav = __webpack_require__(41078);
// EXTERNAL MODULE: ./src/lib/hooks/useWatchBacklogItems.ts
var useWatchBacklogItems = __webpack_require__(10414);
// EXTERNAL MODULE: ./src/lib/hooks/useBacklogFilters.ts
var useBacklogFilters = __webpack_require__(3598);
// EXTERNAL MODULE: ./src/lib/hooks/useBacklogStages.ts
var useBacklogStages = __webpack_require__(3456);
// EXTERNAL MODULE: ./src/components/backlog-stuck/stuckReason.ts + 1 modules
var stuckReason = __webpack_require__(91216);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/circle-dot.mjs
var circle_dot = __webpack_require__(65248);
// EXTERNAL MODULE: ./src/lib/backlog/status.ts
var backlog_status = __webpack_require__(85546);
// EXTERNAL MODULE: ./src/lib/backlog/sourceBadge.ts
var sourceBadge = __webpack_require__(82488);
// EXTERNAL MODULE: ./src/lib/backlog/itemActions.ts
var itemActions = __webpack_require__(5658);
// EXTERNAL MODULE: ./src/components/backlog/BlockerChip.tsx + 1 modules
var BlockerChip = __webpack_require__(82496);
// EXTERNAL MODULE: ./src/lib/backlog/dependencies.ts
var dependencies = __webpack_require__(17213);
// EXTERNAL MODULE: ./src/components/backlog/DuplicatePendingBadge.tsx + 1 modules
var DuplicatePendingBadge = __webpack_require__(75938);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/triangle-alert.mjs
var triangle_alert = __webpack_require__(18719);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/link-2.mjs
var link_2 = __webpack_require__(71231);
// EXTERNAL MODULE: ./src/components/backlog/ClaimedElsewhereNotice.tsx + 1 modules
var ClaimedElsewhereNotice = __webpack_require__(88627);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FBacklogItemCard.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-VYXW-rOBB976_wy0qttK6AfqSlL5X2h6wMHpJpjI1sk4-u-t9XNoSYAIHbe1f3YV-iEA9zxmdmjid-38Kx0KwEQ7YMXpNjRP65IaTQqvRfCLGaSVMoXaZEK8ss3EYc1ndvN4R83RBi1aTdw3Ng-XXzfgn14N-M_mgdZCzfrrWqJae5EkqnZMf0LaV_x6_wmejP-KUDjaPFb606_PsGNfYvcjSVYMeUoBQogWZC5VtnuUduNymJk-rgHjeA6409P2dKc9ApSaoDMUogJ3mtNUj7lwM_W1CrqlNAnpmKOavAQDOOtbkM9-HVx8sklsyikmmXluh-ZYiLlWmCskCJFpypqliO9pg6A_dcCDhQs9EotymJ3s4bT_obd3adPUcNeYOXK1GX0q2sWXUZXvLpwzuTPk73iaW4Y6lvBXfzRDSeKsY5yvVgNfKrea2NY7hSKC14-j3Z2GylBfBpINF9_GQIMAN_kkw5ihhX--DnkKp0o3agPWGhk8swsN3HydsgzO1dz2uh8trQHRrMBHjvqrYup2FB9V2UHqI1o6ooDFhvHTh-Gs8sE7iWFC2UJm3LwjJtr6f2ozYWiyPNlbQgbUpMxXKgGdg9gAxAnxuhUNLSfdskF5vfeIfewuAnDNazJodjxDbp9VvuOvD-0TOxA10IR_UGOQdfqN3O6R6yLVqaKU_A6dH7yQUrq5Qk4YLLnNLo97kDbTFn4tQTKYmD3a5GVWOU6RxOtTjg8rx0vfaf-7UfXabqOMMsO69P5AbPFgUrURwH5VDfjakJISVK2opk8tgXySRqRXJSHuLVdM7LRdKxDjvq5VyD3-RhPR1PMkGAhYOlwWlXVxXo3AsIIQKsBU1d0zSpu4-eoAxCfv1OIY2163GBhv73ddTwt7wA-DTf8eL8tzngkCvdnpJSSbh6-r0Gkn5FcubUvvPHFmjuFSW4UNU28L12iXaf10S6iyH7oRimNT9o6ZCyfFF3XeudGXnpkPh3muJ3l_5SCR00x6DCewNiRwpM12o9M4XtZmu5R38xjXSYQTrOIh1CpPU0UpzMQMXxLFYchWCbabA5rNUsVO8cwt8o6gMP4z3-Cw7J4bC9TKx_ZhzojfOdHze1n2f5_pA_GOYxHOanQ3mZTXnZS3nK0bBMQKNdwb-wx5AsqSxlQqg98ODlj59T1s7P9hf5ETPkJJsrjnCWuOSjV0AcClaLUOrEohS1UcwidW7L_8OsVbLD6fB21yLD83xevkep_Fw0jvXUXTatMPIPzc9r5wUQAiuDxl-4bNCCH5jBtUsz_HzdvJfAkZHbSkMB2lANvM6B01KdBj7aLIHM4c4Dt1E8tjdD3R0KlayE00XKw9vFIq9PM2TyFJXmctliiXJNi1q29yNOXaiq7aVhgULQUnFISaH0nmluupuvmb00z6ObWHwp9vUvprFVgsoTAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FBacklogItemCard_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_VYXW_rOBB976_wy0qttK6AfqSlL5X2h6wMHpJpjI1sk4_u_t9XNoSYAIHbe1f3YV_iEA9zxmdmjid_38Kx0KwEQ7YMXpNjRP65IaTQqvRfCLGaSVMoXaZEK8ss3EYc1ndvN4R83RBi1aTdw3Ng_XXzfgn14N_M_mgdZCzfrrWqJae5EkqnZMf0LaV_x6_wmejP_KUDjaPFb606_PsGNfYvcjSVYMeUoBQogWZC5VtnuUduNymJk_rgHjeA6409P2dKc9ApSaoDMUogJ3mtNUj7lwM_W1CrqlNAnpmKOavAQDOOtbkM9_HVx8sklsyikmmXluh_ZYiLlWmCskCJFpypqliO9pg6A_dcCDhQs9EotymJ3s4bT_obd3adPUcNeYOXK1GX0q2sWXUZXvLpwzuTPk73iaW4Y6lvBXfzRDSeKsY5yvVgNfKrea2NY7hSKC14_j3Z2GylBfBpINF9_GQIMAN_kkw5ihhX_DnkKp0o3agPWGhk8swsN3HydsgzO1dz2uh8trQHRrMBHjvqrYup2FB9V2UHqI1o6ooDFhvHTh_Gs8sE7iWFC2UJm3LwjJtr6f2ozYWiyPNlbQgbUpMxXKgGdg9gAxAnxuhUNLSfdskF5vfeIfewuAnDNazJodjxDbp9VvuOvD_0TOxA10IR_UGOQdfqN3O6R6yLVqaKU_A6dH7yQUrq5Qk4YLLnNLo97kDbTFn4tQTKYmD3a5GVWOU6RxOtTjg8rx0vfaf_7UfXabqOMMsO69P5AbPFgUrURwH5VDfjakJISVK2opk8tgXySRqRXJSHuLVdM7LRdKxDjvq5VyD3_RhPR1PMkGAhYOlwWlXVxXo3AsIIQKsBU1d0zSpu4_eoAxCfv1OIY2163GBhv73ddTwt7wA_DTf8eL8tzngkCvdnpJSSbh6_r0Gkn5FcubUvvPHFmjuFSW4UNU28L12iXaf10S6iyH7oRimNT9o6ZCyfFF3XeudGXnpkPh3muJ3l_5SCR00x6DCewNiRwpM12o9M4XtZmu5R38xjXSYQTrOIh1CpPU0UpzMQMXxLFYchWCbabA5rNUsVO8cwt8o6gMP4z3_Cw7J4bC9TKx_ZhzojfOdHze1n2f5_pA_GOYxHOanQ3mZTXnZS3nK0bBMQKNdwb_wx5AsqSxlQqg98ODlj59T1s7P9hf5ETPkJJsrjnCWuOSjV0AcClaLUOrEohS1UcwidW7L_8OsVbLD6fB21yLD83xevkep_Fw0jvXUXTatMPIPzc9r5wUQAiuDxl_4bNCCH5jBtUsz_HzdvJfAkZHbSkMB2lANvM6B01KdBj7aLIHM4c4Dt1E8tjdD3R0KlayE00XKw9vFIq9PM2TyFJXmctliiXJNi1q29yNOXaiq7aVhgULQUnFISaH0nmluupuvmb00z6ObWHwp9vUvprFVgsoTAAA_22_7D = __webpack_require__(72925);
;// ./src/components/backlog/BacklogItemCard.css.ts


var acSummary = 'kae92yc';
var actionButton = 'kae92yi';
var actionButtonDone = 'kae92yl';
var blockedByLabel = 'kae92yk';
var buttonSpinner = 'kae92y1';
var card = 'kae92y2';
var cardFooter = 'kae92ya';
var cardHeader = 'kae92y5';
var claimChip = 'kae92ym';
var claimChipLabel = 'kae92yn';
var BacklogItemCard_css_disabledReason = 'kae92yj';
var footerLeft = 'kae92yb';
var BacklogItemCard_css_justChanged = 'kae92y4';
var priorityBadge = 'kae92y7';
var provenanceBadge = 'kae92y9';
var statusLabel = 'kae92y8';
var title = 'kae92y6';
var verdictBadgeFail = 'kae92yg kae92yd';
var verdictBadgePartial = 'kae92yf kae92yd';
var verdictBadgePass = 'kae92ye kae92yd';
var verdictBadgeUnverifiable = 'kae92yh kae92yd';
;// ./src/components/backlog/ClaimChip.tsx
/* __next_internal_client_entry_do_not_use__ ClaimChip auto */ 
// +feature: backlog:claim-chip



/**
 * Compact board-card indicator that another host claimed this item's external
 * URL (design/ux.md Surface 2). A heads-up, not a control: clicking the card
 * opens the detail view where ClaimConflictBanner offers the actions.
 */ function ClaimChip(param) {
    let { claim } = param;
    const host = (0,ClaimedElsewhereNotice/* claimingHostLabel */.$)(claim);
    const Icon = claim.disputed ? triangle_alert/* default */.A : link_2/* default */.A;
    const label = claim.disputed ? "Disputed" : "Claimed: ".concat(host);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
        className: claimChip,
        title: claim.disputed ? "Claim disputed between hosts, currently ".concat(host) : "Claimed by ".concat(host),
        "data-testid": "claim-chip",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)(Icon, {
                "aria-hidden": "true",
                size: 12
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: claimChipLabel,
                children: label
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/components/backlog/TriageLoadingIndicator.tsx + 1 modules
var TriageLoadingIndicator = __webpack_require__(90080);
// EXTERNAL MODULE: ./src/components/backlog/externalRef.ts
var externalRef = __webpack_require__(34243);
;// ./src/components/backlog/BacklogItemCard.tsx
/* __next_internal_client_entry_do_not_use__ BacklogItemCard auto */ 
// +feature: backlog:item-card

// lucide-react (this repo's pinned v1.14) ships no brand "Github" glyph —
// CircleDot is the closest available icon to GitHub's own issue-tracker
// mark and is used here instead (plan.md's snippet assumed `Github` exists;
// it doesn't in the installed version).











// The bare externalId is only an issue number, so repo-qualify it from the URL
// (stapler-mcp#22 vs stapler-squad#22); unparseable URLs keep the bare "#N".
function ProvenanceBadge(param) {
    let { externalUrl, externalId, pluginId } = param;
    const source = (0,sourceBadge/* getSourceBadgeConfig */.T2)(pluginId);
    const { fullRef, shortRef } = (0,sourceBadge/* buildExternalRefLabels */.c_)(pluginId, externalUrl, externalId);
    const href = (0,externalRef/* safeExternalHref */.a)(externalUrl);
    if (!href) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
            className: provenanceBadge,
            title: fullRef,
            "data-action-button": "true",
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)(circle_dot/* default */.A, {
                    "aria-hidden": "true",
                    size: 12
                }),
                shortRef
            ]
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("a", {
        href: href,
        target: "_blank",
        rel: "noopener noreferrer",
        className: provenanceBadge,
        "aria-label": "Imported from ".concat(source.label, " issue ").concat(fullRef),
        title: fullRef,
        "data-action-button": "true",
        onClick: (e)=>e.stopPropagation(),
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)(circle_dot/* default */.A, {
                "aria-hidden": "true",
                size: 12
            }),
            shortRef
        ]
    });
}
function AcSummary(param) {
    let { item } = param;
    if (item.acCriteria.length === 0) return null;
    const done = item.acCriteria.filter((c)=>c.status === "done").length;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
        className: acSummary,
        "aria-label": "".concat(done, " of ").concat(item.acCriteria.length, " criteria done"),
        children: [
            done,
            "/",
            item.acCriteria.length,
            " done"
        ]
    });
}
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
// Last review result, at a glance — most useful on in_progress cards, where a
// FAIL/PARTIAL verdict that triggered rework is otherwise invisible until the
// item is opened (PENDING is left unbadged: it just means a review is running,
// not a signal worth a card badge).
function VerdictBadge(param) {
    let { item } = param;
    if (!item.gateVerdict) return null;
    const config = VERDICT_BADGE_CONFIG[item.gateVerdict];
    if (!config) return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
        className: config.className,
        title: "Last review result",
        children: config.label
    });
}
const PRIORITY_LABELS = {
    1: "P1",
    2: "P2",
    3: "P3",
    4: "P4",
    5: "P5"
};
// Epic 6.1 (backlog-event-driven-updates): how long the `.justChanged` flash
// class stays applied. ux.md §1 calls for "~250ms, fading" (Linear/Jira-
// style); Story 6.1.1's Given/When/Then uses the same figure.
const FLASH_DURATION_MS = 250;
const BacklogItemCard = /*#__PURE__*/ (0,react.memo)(function BacklogItemCard(param) {
    let { item, onAction, onClick, pendingAction = null, forceJustChanged = false, stuckItem, otherStuckReasons, claim } = param;
    const actionSpec = (0,itemActions/* getPrimaryCardAction */.Nd)(item);
    const blockedByText = (0,dependencies/* blockedByLabel */.WY)(item.blockedBy);
    const isTriageRunning = item.triageStatus === "running";
    const isActionPending = pendingAction === actionSpec.action;
    // Transient disable reasons (spinner + "Running…"/"Triage in progress") are
    // already explained via the button label/TriageLoadingIndicator — never
    // surface the underlying domain condition's reason on top of those, even
    // if it's also unmet.
    const disabledReason = isTriageRunning || pendingAction !== null ? undefined : actionSpec.disabled ? actionSpec.disabledReason : undefined;
    const disabledReasonId = "".concat(item.id, "-disabled-reason");
    // `item.liveVersion` only advances for a genuine live (non-snapshot)
    // BacklogItemEvent (see useWatchBacklogItems.ts / backlogItemsSlice.ts) —
    // never for the initial snapshot, a reconnect resync, or a forced-
    // is_snapshot replay copy (pre-mortem #4). Comparing against the
    // previously-seen value (not just "did the item prop change") is what
    // keeps a resnapshot from flashing even though the item's fields did
    // change while disconnected — and comparing to a ref instead of keying off
    // `item` reference identity means this never fires on first mount.
    const prevLiveVersionRef = (0,react.useRef)(item.liveVersion);
    const [justChanged, setJustChanged] = (0,react.useState)(false);
    (0,react.useEffect)(()=>{
        const prev = prevLiveVersionRef.current;
        const next = item.liveVersion;
        prevLiveVersionRef.current = next;
        if (prev === undefined || next === undefined || next === prev) return;
        setJustChanged(true);
        const timer = setTimeout(()=>setJustChanged(false), FLASH_DURATION_MS);
        return ()=>clearTimeout(timer);
    }, [
        item.liveVersion
    ]);
    const handleCardClick = (e)=>{
        // Don't open detail if the action button was clicked
        if (e.target.closest("[data-action-button]")) return;
        onClick(item.id);
    };
    const handleKeyDown = (e)=>{
        // Ignore keydowns that bubbled up from a nested focusable child (the
        // provenance badge link, the action button) — otherwise Enter on those
        // elements both triggers their own behavior AND this card's onClick,
        // and for the badge `<a>` specifically, this handler's preventDefault()
        // stops the anchor from navigating at all.
        if (e.target !== e.currentTarget) return;
        if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            onClick(item.id);
        }
    };
    const handleCancelTriage = (0,react.useCallback)(()=>onAction("cancel_triage", item.id), [
        onAction,
        item.id
    ]);
    const showFlash = justChanged || forceJustChanged;
    var _PRIORITY_LABELS_item_priority, _PRIORITY_LABELS_item_priority1, _item_duplicateRef;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: showFlash ? "".concat(card, " ").concat(BacklogItemCard_css_justChanged) : card,
        role: "article",
        tabIndex: 0,
        "data-testid": "backlog-item-card",
        "data-item-id": item.id,
        "aria-label": "Backlog item: ".concat(item.title),
        onClick: handleCardClick,
        onKeyDown: handleKeyDown,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: cardHeader,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: title,
                        children: item.title
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: priorityBadge,
                        "aria-label": "Priority: ".concat((_PRIORITY_LABELS_item_priority = PRIORITY_LABELS[item.priority]) !== null && _PRIORITY_LABELS_item_priority !== void 0 ? _PRIORITY_LABELS_item_priority : "P?"),
                        children: (_PRIORITY_LABELS_item_priority1 = PRIORITY_LABELS[item.priority]) !== null && _PRIORITY_LABELS_item_priority1 !== void 0 ? _PRIORITY_LABELS_item_priority1 : "P?"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: statusLabel,
                        "data-testid": "backlog-item-card-status",
                        children: (0,backlog_status/* getStatusLabel */.p)(item.status)
                    }),
                    item.duplicatePending && /*#__PURE__*/ (0,jsx_runtime.jsx)(DuplicatePendingBadge/* DuplicatePendingBadge */.w, {
                        duplicateRef: (_item_duplicateRef = item.duplicateRef) !== null && _item_duplicateRef !== void 0 ? _item_duplicateRef : ""
                    })
                ]
            }),
            isTriageRunning && /*#__PURE__*/ (0,jsx_runtime.jsx)(TriageLoadingIndicator/* TriageLoadingIndicator */.i, {
                elapsedSeconds: 0,
                context: "list",
                onCancel: pendingAction !== null ? ()=>{} : handleCancelTriage,
                compact: true
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: cardFooter,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: footerLeft,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(AcSummary, {
                                item: item
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(VerdictBadge, {
                                item: item
                            })
                        ]
                    }),
                    item.externalUrl && item.externalId && /*#__PURE__*/ (0,jsx_runtime.jsx)(ProvenanceBadge, {
                        externalUrl: item.externalUrl,
                        externalId: item.externalId,
                        pluginId: item.pluginId
                    }),
                    claim && /*#__PURE__*/ (0,jsx_runtime.jsx)(ClaimChip, {
                        claim: claim
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: "".concat(actionButton, " ").concat(actionSpec.isDone ? actionButtonDone : ""),
                        disabled: actionSpec.disabled || actionSpec.isDone || isTriageRunning || pendingAction !== null,
                        "aria-label": isActionPending ? "Running…" : isTriageRunning ? "Triage in progress" : actionSpec.label,
                        "aria-describedby": disabledReason ? disabledReasonId : undefined,
                        title: disabledReason,
                        "data-action-button": "true",
                        "data-testid": "backlog-action-".concat(actionSpec.action),
                        onClick: (e)=>{
                            e.stopPropagation();
                            if (!actionSpec.disabled && !actionSpec.isDone && !isTriageRunning && pendingAction === null) {
                                onAction(actionSpec.action, item.id);
                            }
                        },
                        children: isActionPending ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: buttonSpinner,
                                    "aria-hidden": "true"
                                }),
                                "Running…"
                            ]
                        }) : actionSpec.label
                    }),
                    stuckItem && /*#__PURE__*/ (0,jsx_runtime.jsx)(BlockerChip/* BlockerChip */.B, {
                        variant: "compact",
                        item: stuckItem,
                        otherReasons: otherStuckReasons
                    }),
                    blockedByText && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: blockedByLabel,
                        "data-testid": "backlog-card-blocked-by",
                        children: blockedByText
                    })
                ]
            }),
            disabledReason && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                id: disabledReasonId,
                className: BacklogItemCard_css_disabledReason,
                "data-testid": "backlog-action-disabled-reason",
                children: disabledReason
            })
        ]
    });
});

// EXTERNAL MODULE: ./src/components/backlog/ConnectionIndicator.tsx + 1 modules
var ConnectionIndicator = __webpack_require__(52454);
// EXTERNAL MODULE: ./src/components/backlog/detail/StageTracker.tsx + 1 modules
var StageTracker = __webpack_require__(54821);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FBacklogBoard.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61Wy3KjMBC8-yt0SVU4yMUjdmJyyZ-kZDTYs9aDlURsvOV_30KADRjW2aqcKOjRTE9PD_BxgCo3TIIln6tE_T64N_JnQUj45C-E6IJl6KqURO_-3hmmbK6NTInNmIDnKKiBy4KQKLw_Fc6cCpebdXvwsli2pUN_mqMtBKtSkgs41SH1lXI0kDnUKiWZFqVUHdIyk6joHnC3d77oLWk0nfRXaR3mFc20cqBcA1BQvAYLxjmqXUq-mHmm9DPawDk25_gcjJ4kUTCsFk9X27FinCxphNNfYHKhj_SUElY6PSgfTpSbzNLv_iUMi9NIHSZwpyg6kLbt1DpmXJ958r_iT_UUXtkckbt9SuKXloxkp-5Z0hHsYt56hKndG1SH0RBfpskN2spAOTAzzOJz8GCw3qlbbTgYutXOaZmSqDgRqwXyUXwe9MmtPLlcK0ctnuFOlK0v7fFjO6NRxN5HZFpoM8aa-To4OdpborIowGTMQg0KcA4MtfXOedcswxXIa82cSRTVXctlMPDIrZ31UGtUAhXQB5LfrdMN6ruhHXRn1e6-ld0wjqW9U2fjmW5ZdtgZXSo-DuDz6sU36SdHwx6OBoPvCTm7s3E1MMvrD6xZa-aJ2W189kJjLT6FL1DOpkRpBf0o5qMmBUseCNbzsnWVgJSgYwKzq0m9P_rjn1m5JB6_ynxXN5JbT3J-6q_BzTnzmwrBY3-9_uvV0L7SfmRgt6_ict1vNfOtdksRxd9ZivW3loIplKwhWJTCAomWK0uAWaCoqC4dQZWjQjfwB_d82p1dh0819iGBIyPPhYEcjKUGeJkBp1I36ZWmDQQqg8AnGJqyR4YqJiG9_nC8j1BempZzHIbSjmGHEtWO5qVqpfft6NKNA3MUgkrNISW5NkdmuO3-OB5109xPtzH8I7osLn8BgWyAjkQJAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FBacklogBoard_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61Wy3KjMBC8_yt0SVU4yMUjdmJyyZ_kZDTYs9aDlURsvOV_30KADRjW2aqcKOjRTE9PD_BxgCo3TIIln6tE_T64N_JnQUj45C_E6IJl6KqURO_3hmmbK6NTInNmIDnKKiBy4KQKLw_Fc6cCpebdXvwsli2pUN_mqMtBKtSkgs41SH1lXI0kDnUKiWZFqVUHdIyk6joHnC3d77oLWk0nfRXaR3mFc20cqBcA1BQvAYLxjmqXUq_mHmm9DPawDk25_gcjJ4kUTCsFk9X27FinCxphNNfYHKhj_SUElY6PSgfTpSbzNLv_iUMi9NIHSZwpyg6kLbt1DpmXJ958r_iT_UUXtkckbt9SuKXloxkp_5Z0hHsYt56hKndG1SH0RBfpskN2spAOTAzzOJz8GCw3qlbbTgYutXOaZmSqDgRqwXyUXwe9MmtPLlcK0ctnuFOlK0v7fFjO6NRxN5HZFpoM8aa_To4OdpborIowGTMQg0KcA4MtfXOedcswxXIa82cSRTVXctlMPDIrZ31UGtUAhXQB5LfrdMN6ruhHXRn1e6_ld0wjqW9U2fjmW5ZdtgZXSo_DuDz6sU36SdHwx6OBoPvCTm7s3E1MMvrD6xZa_aJ2W189kJjLT6FL1DOpkRpBf0o5qMmBUseCNbzsnWVgJSgYwKzq0m9P_rjn1m5JB6_ynxXN5JbT3J_6q_BzTnzmwrBY3_9_uvV0L7SfmRgt6_ict1vNfOtdksRxd9ZivW3loIplKwhWJTCAomWK0uAWaCoqC4dQZWjQjfwB_d82p1dh0819iGBIyPPhYEcjKUGeJkBp1I36ZWmDQQqg8AnGJqyR4YqJiG9_nC8j1BempZzHIbSjmGHEtWO5qVqpfft6NKNA3MUgkrNISW5NkdmuO3_OB5109xPtzH8I7osLn8BgWyAjkQJAAA_22_7D = __webpack_require__(20666);
;// ./src/components/backlog/BacklogBoard.css.ts


var board = '_53nqkt2';
var boardToolbar = '_53nqkt1';
var boardWrapper = '_53nqkt0';
var cardExiting = '_53nqkt9';
var BacklogBoard_css_column = '_53nqkt3';
var columnCards = '_53nqkt7';
var columnCount = '_53nqkt6';
var columnHeader = '_53nqkt4';
var columnTitle = '_53nqkt5';
var emptyColumn = '_53nqkta';
var skeletonCard = '_53nqktb';
var skeletonLine = '_53nqktc';
var skeletonLineShort = '_53nqktd';
;// ./src/components/backlog/BacklogBoard.tsx
/* __next_internal_client_entry_do_not_use__ BacklogBoard auto */ 
// +feature: backlog:board









// The board's 5 columns are exactly StageTracker's 5 stages — reuse the same
// pure status->stage mapping the item-detail pipeline stepper already uses,
// so a "queued"/"pr_pending"/"refining" item folds into its mapped column
// (In Progress / Review / Idea respectively) instead of silently matching no
// column at all (BUG-037: those items rendered nowhere on the board — no
// error, no count, no way to tell they existed short of opening item
// detail). Archived items are excluded from the board, same as before this
// fix (archived never matched any of the 5 literal statuses either). This is
// also the defined behavior for the shared `showArchived` filter (AC 7):
// toggling it changes what filterBacklogItems() lets through, but an
// archived item still resolves to no Stage here, so it never renders in any
// column regardless of the toggle — there is no archived column on the
// board, by design, not by oversight.
function stageOf(status) {
    const { activeStage, archived } = (0,StageTracker/* deriveStageDisplay */.GF)(status);
    return archived ? null : activeStage;
}
const COLUMNS = [
    {
        status: "idea",
        label: "Idea"
    },
    {
        status: "ready",
        label: "Ready"
    },
    {
        status: "in_progress",
        label: "In Progress"
    },
    {
        status: "review",
        label: "Review"
    },
    {
        status: "done",
        label: "Done"
    }
];
// Slugs already represented by COLUMNS above (directly or folded via
// stageOf()) — a fetched BacklogStage matching one of these is a built-in,
// not a custom stage, and must not get a second, duplicate column (Task
// 2.9.1a).
const BUILTIN_STATUS_SLUGS = new Set(useBacklogStages/* BUILTIN_BACKLOG_STAGES */.ou.map((s)=>s.slug));
// Sentinel status for the trailing overflow column (Task 2.9.1b): any live
// item whose status matches neither a built-in fold destination nor a
// fetched custom stage lands here instead of disappearing off the board
// entirely — this is BUG-037's exact failure mode (an item silently
// unrenderable anywhere), now guarded a third time for a third root cause
// (unrecognized/deleted custom stage, after the original status/column
// mismatch and the later archived-item edge case).
const UNRECOGNIZED_STAGE_STATUS = "__unrecognized_stage__";
const UNRECOGNIZED_COLUMN = {
    status: UNRECOGNIZED_STAGE_STATUS,
    label: "Unrecognized stage"
};
// The overflow column never has an exiting/animating card of its own
// (Task 2.9.1b keeps this out of scope of the Epic 6.4 animation tracker,
// see BacklogBoard()'s comment above unrecognizedItems) — a stable shared
// empty Set avoids allocating a new one on every render.
const EMPTY_ID_SET = new Set();
// Epic 6.4 (backlog-event-driven-updates): how long a card's exit fade plays
// in its origin column before it's removed from the DOM, and how long the
// "just changed" flash is forced on for a card that just entered a new
// column (ux.md §7 — "~200ms" fade, paired with the flash on entry). Under
// `prefers-reduced-motion: reduce` both collapse to 0ms at the call site.
const EXIT_TRANSITION_MS = 200;
const ENTER_FLASH_MS = 250;
function SkeletonCard() {
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: skeletonCard,
        "aria-hidden": "true",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: skeletonLine
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: "".concat(skeletonLine, " ").concat(skeletonLineShort)
            })
        ]
    });
}
function BoardColumn(param) {
    let { column, items, exitingIds, enteringIds, onAction, onItemClick, isLoading, pending, stuckItemsById, claimsByUrl, isEmptyDueToFilter } = param;
    // The column count badge should reflect genuinely present items, not a
    // still-fading departure that's only rendered for the exit transition.
    const settledCount = items.filter((item)=>!exitingIds.has(item.id)).length;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
        className: BacklogBoard_css_column,
        "aria-label": "".concat(column.label, " column"),
        "data-testid": "backlog-column-".concat(column.status),
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: columnHeader,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: columnTitle,
                        children: column.label
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: columnCount,
                        "aria-label": "".concat(settledCount, " items"),
                        children: settledCount
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: columnCards,
                role: "list",
                "aria-label": "".concat(column.label, " items"),
                children: isLoading ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)(SkeletonCard, {}),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)(SkeletonCard, {})
                    ]
                }) : items.length === 0 && isEmptyDueToFilter ? /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: emptyColumn,
                    "data-testid": "backlog-column-empty-filtered",
                    children: "No items match filter"
                }) : items.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: emptyColumn,
                    children: "No items"
                }) : items.map((item)=>{
                    const isExiting = exitingIds.has(item.id);
                    const isEntering = !isExiting && enteringIds.has(item.id);
                    const stuckGroup = stuckItemsById.get(item.id);
                    const stuckSummary = stuckGroup ? (0,stuckReason/* summarizeStuckItemGroup */.sh)(stuckGroup) : undefined;
                    var _pending_item_id;
                    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        role: "listitem",
                        className: isExiting ? cardExiting : undefined,
                        "aria-hidden": isExiting || undefined,
                        "data-testid": isExiting ? "backlog-card-exiting" : undefined,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(BacklogItemCard, {
                            item: item,
                            onAction: onAction,
                            onClick: onItemClick,
                            pendingAction: (_pending_item_id = pending[item.id]) !== null && _pending_item_id !== void 0 ? _pending_item_id : null,
                            forceJustChanged: isEntering,
                            stuckItem: stuckSummary === null || stuckSummary === void 0 ? void 0 : stuckSummary.primary,
                            otherStuckReasons: stuckSummary === null || stuckSummary === void 0 ? void 0 : stuckSummary.otherReasons,
                            claim: item.externalUrl ? claimsByUrl.get(item.externalUrl) : undefined
                        })
                    }, item.id);
                })
            })
        ]
    });
}
function BacklogBoard(param) {
    let { onAction, onItemClick, pending = {}, stuckItems = [], filters, foreignClaims } = param;
    const claimsByUrl = (0,react.useMemo)(()=>new Map((foreignClaims !== null && foreignClaims !== void 0 ? foreignClaims : []).map((claim)=>[
                claim.externalUrl,
                claim
            ])), [
        foreignClaims
    ]);
    // Epic 5.2 (backlog-event-driven-updates): the board subscribes to the
    // same live stream/normalized store as the list page (ux.md §2, "no
    // board-specific fetch") rather than receiving items as props — a status-
    // change event moves an item's column membership purely by this filter
    // re-evaluating on the updated item, no board-specific refetch involved.
    const { items, connectionState } = (0,useWatchBacklogItems/* useWatchBacklogItems */.E)();
    // Only show the skeleton on a genuinely empty first paint — a disconnect/
    // reconnect must keep showing last-known state, not blank/spinner-out
    // (ux.md §1 "Error / edge cases", shared by this surface per §2).
    const isLoading = connectionState === "connecting" && items.length === 0;
    // Task 2.9.1a: the board's columns are the fixed 5 built-in stages above
    // plus one column per enabled *custom* stage the operator has configured
    // (useBacklogStages() falls back to the built-in set synchronously, so
    // this never regresses to zero columns while the fetch is in flight).
    const { stages } = (0,useBacklogStages/* useBacklogStages */._M)();
    const customColumns = (0,react.useMemo)(()=>stages.filter((s)=>s.enabled && !BUILTIN_STATUS_SLUGS.has(s.slug)).map((s)=>({
                status: s.slug,
                label: s.name
            })), [
        stages
    ]);
    const customColumnStatuses = (0,react.useMemo)(()=>new Set(customColumns.map((c)=>c.status)), [
        customColumns
    ]);
    const columns = (0,react.useMemo)(()=>[
            ...COLUMNS,
            ...customColumns
        ], [
        customColumns
    ]);
    // Epic 6.4 (backlog-event-driven-updates): when a genuine live status
    // change (gated on `item.liveVersion` advancing, same signal as
    // BacklogItemCard's own flash and the list view's Epic 6.3 exit fade)
    // moves an item from one board column to another, briefly keep rendering
    // it in its origin column with a fade-out ("exiting") while the freshly
    // mounted card in its destination column force-flashes ("entering") — the
    // same event driving one continuous "moved from X to Y" (ux.md §7,
    // UX AC #8), not two independent, uncorrelated animations. A bulk
    // resnapshot on reconnect never advances `liveVersion` for its items (see
    // backlogItemsSlice.ts), so it falls straight through to an ordinary,
    // un-animated re-render — matching the list view's Epic 6.3 guard.
    const [exitingItems, setExitingItems] = (0,react.useState)(new Map());
    const [enteringIds, setEnteringIds] = (0,react.useState)(new Set());
    const exitingMapRef = (0,react.useRef)(new Map());
    const enteringSetRef = (0,react.useRef)(new Set());
    const prevStatusRef = (0,react.useRef)(new Map());
    const prevLiveVersionRef = (0,react.useRef)(new Map());
    const exitTimersRef = (0,react.useRef)(new Map());
    const enterTimersRef = (0,react.useRef)(new Map());
    const reducedMotionRef = (0,react.useRef)(false);
    (0,react.useEffect)(()=>{
        if ( false || !window.matchMedia) return;
        const mq = window.matchMedia("(prefers-reduced-motion: reduce)");
        reducedMotionRef.current = mq.matches;
        const onChange = ()=>{
            reducedMotionRef.current = mq.matches;
        };
        mq.addEventListener("change", onChange);
        return ()=>mq.removeEventListener("change", onChange);
    }, []);
    // useLayoutEffect (not useEffect): runs before the browser paints, so a
    // departing card is re-added to the exiting map within the same commit it
    // was excluded from its old column — no visible blank frame in between.
    (0,react.useLayoutEffect)(()=>{
        const exitingMap = exitingMapRef.current;
        const enteringSet = enteringSetRef.current;
        let exitingChanged = false;
        let enteringChanged = false;
        // Flap protection first pass: if an item with a pending exit has
        // re-matched the column it was fading out of (before that fade
        // completed), cancel the exit rather than let it finish unmounting and
        // immediately remount (ux.md §7 "Error / edge cases"). This settling
        // move itself must not then be treated as a *new* transition to
        // animate (pass 2 below skips these ids) — otherwise a fast
        // review -> in_progress -> review flap would fade a card out of
        // "review" (cancelled here) only to immediately start a fresh
        // exit/enter cycle for the reverse hop, still flickering.
        const flappedIds = new Set();
        for (const item of items){
            const pendingExit = exitingMap.get(item.id);
            if (pendingExit && stageOf(pendingExit.fromStatus) === stageOf(item.status)) {
                const timer = exitTimersRef.current.get(item.id);
                if (timer) clearTimeout(timer);
                exitTimersRef.current.delete(item.id);
                exitingMap.delete(item.id);
                exitingChanged = true;
                flappedIds.add(item.id);
                const enterTimer = enterTimersRef.current.get(item.id);
                if (enterTimer) clearTimeout(enterTimer);
                enterTimersRef.current.delete(item.id);
                if (enteringSet.delete(item.id)) enteringChanged = true;
            }
        }
        for (const item of items){
            const prevStatus = prevStatusRef.current.get(item.id);
            const prevVersion = prevLiveVersionRef.current.get(item.id);
            const isGenuineLiveChange = item.liveVersion !== undefined && item.liveVersion !== prevVersion;
            if (!flappedIds.has(item.id) && isGenuineLiveChange && prevStatus !== undefined && stageOf(prevStatus) !== stageOf(item.status)) {
                if (stageOf(prevStatus) !== null && !exitingMap.has(item.id)) {
                    exitingMap.set(item.id, {
                        item: {
                            ...item,
                            status: prevStatus
                        },
                        fromStatus: prevStatus
                    });
                    exitingChanged = true;
                    const duration = reducedMotionRef.current ? 0 : EXIT_TRANSITION_MS;
                    const timer = setTimeout(()=>{
                        if (exitingMapRef.current.delete(item.id)) {
                            setExitingItems(new Map(exitingMapRef.current));
                        }
                        exitTimersRef.current.delete(item.id);
                    }, duration);
                    exitTimersRef.current.set(item.id, timer);
                }
                if (stageOf(item.status) !== null) {
                    enteringSet.add(item.id);
                    enteringChanged = true;
                    const existingTimer = enterTimersRef.current.get(item.id);
                    if (existingTimer) clearTimeout(existingTimer);
                    const duration = reducedMotionRef.current ? 0 : ENTER_FLASH_MS;
                    const timer = setTimeout(()=>{
                        if (enteringSetRef.current.delete(item.id)) {
                            setEnteringIds(new Set(enteringSetRef.current));
                        }
                        enterTimersRef.current.delete(item.id);
                    }, duration);
                    enterTimersRef.current.set(item.id, timer);
                }
            }
            prevStatusRef.current.set(item.id, item.status);
            prevLiveVersionRef.current.set(item.id, item.liveVersion);
        }
        if (exitingChanged) setExitingItems(new Map(exitingMap));
        if (enteringChanged) setEnteringIds(new Set(enteringSet));
    }, [
        items
    ]);
    // Clear any in-flight timers on unmount.
    (0,react.useEffect)(()=>{
        return ()=>{
            for (const timer of exitTimersRef.current.values())clearTimeout(timer);
            for (const timer of enterTimersRef.current.values())clearTimeout(timer);
        };
    }, []);
    // BUG-105: an item can have several simultaneous open StuckBacklogItem
    // rows (e.g. BOUNCING + BOUNCE_CAP_EXHAUSTED + MULTIPLE_REASONS all open at
    // once) — grouping here (instead of the old `new Map(stuckItems.map(s =>
    // [s.itemId, s]))`, which silently kept whichever row happened to be last
    // in the array) lets each card resolve the SAME shared-priority primary
    // reason BacklogItemDetail resolves for the same item.
    const stuckItemsById = (0,stuckReason/* groupStuckItemsByItemId */.tR)(stuckItems);
    // Task 2.9.1b: an item whose status matches neither a built-in fold
    // destination (stageOf()) nor a fetched custom stage column must still
    // render somewhere — this is BUG-037's exact failure mode (silently
    // unrenderable) recurring for a third root cause (a deleted/unrecognized
    // custom stage). "archived" is excluded deliberately: it has its own
    // defined "no column" behavior (stageOf's comment above), not an
    // unrecognized one.
    const knownColumnStatuses = (0,react.useMemo)(()=>new Set(columns.map((c)=>c.status)), [
        columns
    ]);
    const unrecognizedItems = (0,react.useMemo)(()=>{
        const unfiltered = items.filter((i)=>stageOf(i.status) === null && i.status !== "archived" && !knownColumnStatuses.has(i.status));
        return filters ? (0,useBacklogFilters/* filterBacklogItems */.ub)(unfiltered, filters) : unfiltered;
    }, [
        items,
        knownColumnStatuses,
        filters
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: boardWrapper,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: boardToolbar,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ConnectionIndicator/* ConnectionIndicator */._, {
                    connectionState: connectionState
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: board,
                role: "region",
                "aria-label": "Backlog board",
                "data-testid": "backlog-board",
                children: [
                    columns.map((column)=>{
                        // A custom stage (Task 2.9.1a) has no fold mapping in stageOf() —
                        // it matches an item by its raw status/slug equality instead.
                        const isCustomColumn = customColumnStatuses.has(column.status);
                        const columnMatches = (status)=>isCustomColumn ? status === column.status : stageOf(status) === column.status;
                        // Filtering (AC 0, 1, 2) applies only to this column's settled
                        // items — `items` itself stays unfiltered so the exit/enter
                        // animation tracking above keeps seeing every status transition
                        // regardless of the active filter. Sort/group-by are list-only
                        // (AC 6) and never reach the board.
                        const unfilteredBaseItems = items.filter((i)=>columnMatches(i.status));
                        const baseItems = filters ? (0,useBacklogFilters/* filterBacklogItems */.ub)(unfilteredBaseItems, filters) : unfilteredBaseItems;
                        const isEmptyDueToFilter = baseItems.length === 0 && unfilteredBaseItems.length > 0;
                        const baseIds = new Set(baseItems.map((i)=>i.id));
                        const exitingForColumn = Array.from(exitingItems.values()).filter((e)=>columnMatches(e.fromStatus) && !baseIds.has(e.item.id));
                        const displayItems = exitingForColumn.length === 0 ? baseItems : [
                            ...baseItems,
                            ...exitingForColumn.map((e)=>e.item)
                        ];
                        const exitingIdsForColumn = new Set(exitingForColumn.map((e)=>e.item.id));
                        return /*#__PURE__*/ (0,jsx_runtime.jsx)(BoardColumn, {
                            column: column,
                            items: displayItems,
                            exitingIds: exitingIdsForColumn,
                            enteringIds: enteringIds,
                            onAction: onAction,
                            onItemClick: onItemClick,
                            isLoading: isLoading,
                            pending: pending,
                            stuckItemsById: stuckItemsById,
                            claimsByUrl: claimsByUrl,
                            isEmptyDueToFilter: isEmptyDueToFilter
                        }, column.status);
                    }),
                    unrecognizedItems.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)(BoardColumn, {
                        column: UNRECOGNIZED_COLUMN,
                        items: unrecognizedItems,
                        exitingIds: EMPTY_ID_SET,
                        enteringIds: enteringIds,
                        onAction: onAction,
                        onItemClick: onItemClick,
                        isLoading: isLoading,
                        pending: pending,
                        stuckItemsById: stuckItemsById,
                        claimsByUrl: claimsByUrl,
                        isEmptyDueToFilter: false
                    }, UNRECOGNIZED_COLUMN.status)
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/components/backlog/BacklogFilterBar.tsx + 1 modules
var BacklogFilterBar = __webpack_require__(23522);
// EXTERNAL MODULE: ./src/components/backlog/BacklogItemDetail.tsx + 89 modules
var BacklogItemDetail = __webpack_require__(41062);
// EXTERNAL MODULE: ./src/lib/hooks/useBacklogService.ts
var useBacklogService = __webpack_require__(8570);
// EXTERNAL MODULE: ./src/lib/contexts/NotificationContext.tsx + 12 modules
var NotificationContext = __webpack_require__(62275);
// EXTERNAL MODULE: ./src/lib/hooks/useStuckBacklogItems.ts
var useStuckBacklogItems = __webpack_require__(77717);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Fpane%2FdetailPaneShell.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8yanFmdmwwIHsKICBtaW4td2lkdGg6IDI0MHB4OwogIG1heC13aWR0aDogODAwcHg7CiAgYm9yZGVyLWxlZnQ6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGZsZXgtc2hyaW5rOiAwOwogIG92ZXJmbG93OiBoaWRkZW47CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwp9CkBtZWRpYSAobWF4LXdpZHRoOiA3NjhweCkgewogIC5fMmpxZnZsMCB7CiAgICBwb3NpdGlvbjogZml4ZWQ7CiAgICB0b3A6IDA7CiAgICBsZWZ0OiAwOwogICAgcmlnaHQ6IDA7CiAgICBib3R0b206IHZhcigtLWJvdHRvbS1uYXYtaGVpZ2h0LCA3MnB4KTsKICAgIHdpZHRoOiAxMDAlICFpbXBvcnRhbnQ7CiAgICB6LWluZGV4OiA1MDA7CiAgICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejkpOwogIH0KfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Fpane_2FdetailPaneShell_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8yanFmdmwwIHsKICBtaW4td2lkdGg6IDI0MHB4OwogIG1heC13aWR0aDogODAwcHg7CiAgYm9yZGVyLWxlZnQ6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGZsZXgtc2hyaW5rOiAwOwogIG92ZXJmbG93OiBoaWRkZW47CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwp9CkBtZWRpYSAobWF4LXdpZHRoOiA3NjhweCkgewogIC5fMmpxZnZsMCB7CiAgICBwb3NpdGlvbjogZml4ZWQ7CiAgICB0b3A6IDA7CiAgICBsZWZ0OiAwOwogICAgcmlnaHQ6IDA7CiAgICBib3R0b206IHZhcigtLWJvdHRvbS1uYXYtaGVpZ2h0LCA3MnB4KTsKICAgIHdpZHRoOiAxMDAlICFpbXBvcnRhbnQ7CiAgICB6LWluZGV4OiA1MDA7CiAgICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejkpOwogIH0KfQ_22_7D = __webpack_require__(37341);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Fbacklog%2Fboard%2Fboard.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll85eTAwbGswIHsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXgtZGlyZWN0aW9uOiBjb2x1bW47CiAgaGVpZ2h0OiAxMDAlOwogIG92ZXJmbG93OiBoaWRkZW47Cn0KLl85eTAwbGsxIHsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXg6IDE7CiAgbWluLWhlaWdodDogMDsKICBvdmVyZmxvdzogaGlkZGVuOwp9Ci5fOXkwMGxrMiB7CiAgd2lkdGg6IDQyMHB4Owp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Fbacklog_2Fboard_2Fboard_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll85eTAwbGswIHsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXgtZGlyZWN0aW9uOiBjb2x1bW47CiAgaGVpZ2h0OiAxMDAlOwogIG92ZXJmbG93OiBoaWRkZW47Cn0KLl85eTAwbGsxIHsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXg6IDE7CiAgbWluLWhlaWdodDogMDsKICBvdmVyZmxvdzogaGlkZGVuOwp9Ci5fOXkwMGxrMiB7CiAgd2lkdGg6IDQyMHB4Owp9_22_7D = __webpack_require__(83309);
;// ./src/app/backlog/board/board.css.ts



var contentArea = '_9y00lk1';
var detailPane = '_9y00lk2 _2jqfvl0';
var pageWrapper = '_9y00lk0';
;// ./src/app/backlog/board/page.tsx
// analytics-exempt
/* __next_internal_client_entry_do_not_use__ default auto */ 
// +feature: backlog:board-page










const ACTION_SUCCESS_MESSAGES = {
    mark_ready: "Marked ready.",
    trigger_triage: "Triage started.",
    spawn_session: "Session started.",
    cancel_triage: "Triage cancelled."
};
function BacklogBoardPageInner() {
    const { transitionStatus, triggerTriage, spawnSessionFromItem, cancelTriage, listForeignClaims } = (0,useBacklogService/* useBacklogService */.gG)();
    // One local-only call for the whole board (never per card); claims that have
    // not gossiped here yet simply show no chip.
    const [foreignClaims, setForeignClaims] = (0,react.useState)([]);
    (0,react.useEffect)(()=>{
        let cancelled = false;
        void listForeignClaims().then((claims)=>{
            if (!cancelled) setForeignClaims(claims);
        });
        return ()=>{
            cancelled = true;
        };
    }, [
        listForeignClaims
    ]);
    const { showActionToast } = (0,NotificationContext/* useNotifications */.E$)();
    const router = (0,nav/* useRouter */.KA)();
    const searchParams = (0,nav/* useSearchParams */.ok)();
    const selectedItemId = searchParams.get("item");
    // Shared filter state with the list view (AC 3, 4) — same localStorage
    // keys via useBacklogFilters. Sort/group-by remain list-only (AC 6).
    const filterState = (0,useBacklogFilters/* useBacklogFilters */.KH)();
    const { search, statusFilter, priorityFilter, sourceFilter, showArchived } = filterState.state;
    const { search: setSearch, statusFilter: setStatusFilter, priorityFilter: setPriorityFilter, sourceFilter: setSourceFilter, showArchived: setShowArchived } = filterState.setters;
    const resetView = filterState.resetToDefaults;
    /** itemId -> action key currently in flight for that card. */ const [pending, setPending] = (0,react.useState)({});
    // Called once here (not per-card) so every card shares one poll instead of
    // N independent 60s polls — see plan.md Task 5.1.1a.
    const { items: stuckItems } = (0,useStuckBacklogItems/* useStuckBacklogItems */.d)();
    const mountedRef = (0,react.useRef)(true);
    (0,react.useEffect)(()=>{
        mountedRef.current = true;
        return ()=>{
            mountedRef.current = false;
        };
    }, []);
    const handleAction = (0,react.useCallback)(async (action, itemId)=>{
        if (action === "view_session" || action === "view_review") {
            const params = new URLSearchParams(searchParams.toString());
            params.set("item", itemId);
            router.push("/backlog/board?".concat(params.toString()));
            return;
        }
        setPending((prev)=>({
                ...prev,
                [itemId]: action
            }));
        const toastKey = "".concat(itemId, ":").concat(action);
        var _ACTION_SUCCESS_MESSAGES_action;
        let successMessage = (_ACTION_SUCCESS_MESSAGES_action = ACTION_SUCCESS_MESSAGES[action]) !== null && _ACTION_SUCCESS_MESSAGES_action !== void 0 ? _ACTION_SUCCESS_MESSAGES_action : "Done.";
        try {
            switch(action){
                case "mark_ready":
                    await transitionStatus(itemId, "ready");
                    break;
                case "trigger_triage":
                    await triggerTriage(itemId);
                    break;
                case "spawn_session":
                    {
                        const resp = await spawnSessionFromItem(itemId);
                        if (resp === null || resp === void 0 ? void 0 : resp.queued) successMessage = "At capacity — item queued, will start automatically.";
                        break;
                    }
                case "cancel_triage":
                    await cancelTriage(itemId);
                    break;
                default:
                    return;
            }
            // No manual refetch: the mutation's server-side effect publishes a
            // BacklogItemEvent that the board's live useWatchBacklogItems stream
            // picks up and applies to the shared store, so the card/column
            // updates itself once that event arrives (Epic 5.2).
            showActionToast(successMessage, "success", toastKey);
        } catch (e) {
            const msg = e instanceof Error ? e.message : "Action failed.";
            showActionToast(msg, "error", toastKey);
        } finally{
            if (mountedRef.current) {
                setPending((prev)=>{
                    const next = {
                        ...prev
                    };
                    delete next[itemId];
                    return next;
                });
            }
        }
    }, [
        transitionStatus,
        triggerTriage,
        spawnSessionFromItem,
        cancelTriage,
        router,
        searchParams,
        showActionToast
    ]);
    const handleItemClick = (0,react.useCallback)((itemId)=>{
        const params = new URLSearchParams(searchParams.toString());
        params.set("item", itemId);
        router.push("/backlog/board?".concat(params.toString()));
    }, [
        router,
        searchParams
    ]);
    const handleDetailClose = (0,react.useCallback)(()=>{
        const params = new URLSearchParams(searchParams.toString());
        params.delete("item");
        const qs = params.toString();
        router.push(qs ? "/backlog/board?".concat(qs) : "/backlog/board");
    }, [
        router,
        searchParams
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: pageWrapper,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)(BacklogFilterBar/* BacklogFilterBar */.l, {
                search: search,
                onSearchChange: setSearch,
                statusFilter: statusFilter,
                onStatusFilterChange: setStatusFilter,
                priorityFilter: priorityFilter,
                onPriorityFilterChange: setPriorityFilter,
                sourceFilter: sourceFilter,
                onSourceFilterChange: setSourceFilter,
                showArchived: showArchived,
                onShowArchivedChange: setShowArchived,
                onResetView: resetView,
                showSortGroupControls: false,
                showArchivedControl: false
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: contentArea,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(BacklogBoard, {
                        onAction: handleAction,
                        onItemClick: handleItemClick,
                        pending: pending,
                        stuckItems: stuckItems,
                        foreignClaims: foreignClaims,
                        filters: {
                            search,
                            statusFilter,
                            priorityFilter,
                            sourceFilter,
                            showArchived
                        }
                    }),
                    selectedItemId && /*#__PURE__*/ (0,jsx_runtime.jsx)("aside", {
                        className: detailPane,
                        "aria-label": "Item detail",
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(BacklogItemDetail/* BacklogItemDetail */.Uw, {
                            itemId: selectedItemId,
                            onClose: handleDetailClose
                        }, selectedItemId)
                    })
                ]
            })
        ]
    });
}
function BacklogBoardPage() {
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(react.Suspense, {
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(BacklogBoardPageInner, {})
    });
}


/***/ }),

/***/ 90377:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
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

"use strict";
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

"use strict";
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

},
/******/ __webpack_require__ => { // webpackRuntimeModules
/******/ var __webpack_exec__ = (moduleId) => (__webpack_require__(__webpack_require__.s = moduleId))
/******/ __webpack_require__.O(0, [1869,4202,9353,8182,7924,935,2265,2921,7333,8165,3575,8890,2513,2275,3117,4298,2805,3793,6925,1618,3467,6833,9691,3516,7358], () => (__webpack_exec__(85732)));
/******/ var __webpack_exports__ = __webpack_require__.O();
/******/ _N_E = __webpack_exports__;
/******/ }
]);
//# sourceMappingURL=page-d1465385a90222db.js.map