(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[9670],{

/***/ 228:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 2419:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  "default": () => (/* binding */ RulesPage)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/nav/index.ts
var nav = __webpack_require__(41078);
// EXTERNAL MODULE: ./src/lib/hooks/useApprovalRules.ts
var useApprovalRules = __webpack_require__(69670);
// EXTERNAL MODULE: ./src/lib/contexts/NotificationContext.tsx + 12 modules
var NotificationContext = __webpack_require__(62275);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ./src/gen/session/v1/session_pb.ts
var session_pb = __webpack_require__(23575);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/create.js
var create = __webpack_require__(95444);
// EXTERNAL MODULE: ./src/lib/api/transport.ts
var transport = __webpack_require__(21694);
;// ./src/lib/hooks/useApprovalAnalytics.ts
/* __next_internal_client_entry_do_not_use__ useApprovalAnalytics auto */ 





/**
 * React hook for loading approval analytics.
 *
 * Fetches a summary of auto-approval decisions over the requested time window
 * via `getApprovalAnalytics`.
 */ function useApprovalAnalytics() {
    let options = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : {};
    const { windowDays = 7 } = options;
    const [summary, setSummary] = (0,react.useState)(null);
    const [dailyBuckets, setDailyBuckets] = (0,react.useState)([]);
    const [loading, setLoading] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    const clientRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
    }, []);
    const fetchAnalytics = (0,react.useCallback)(async ()=>{
        if (!clientRef.current) return;
        setLoading(true);
        setError(null);
        try {
            const req = (0,create/* create */.v)(session_pb/* GetApprovalAnalyticsRequestSchema */.aZ7, {
                windowDays
            });
            const resp = await clientRef.current.getApprovalAnalytics(req);
            var _resp_summary;
            setSummary((_resp_summary = resp.summary) !== null && _resp_summary !== void 0 ? _resp_summary : null);
            var _resp_dailyBuckets;
            setDailyBuckets((_resp_dailyBuckets = resp.dailyBuckets) !== null && _resp_dailyBuckets !== void 0 ? _resp_dailyBuckets : []);
        } catch (err) {
            const e = err instanceof Error ? err : new Error("Failed to fetch analytics");
            setError(e);
            console.error("Failed to fetch approval analytics:", e);
        } finally{
            setLoading(false);
        }
    }, [
        windowDays
    ]);
    const refresh = (0,react.useCallback)(async ()=>{
        await fetchAnalytics();
    }, [
        fetchAnalytics
    ]);
    (0,react.useEffect)(()=>{
        refresh();
    }, [
        refresh
    ]);
    return {
        summary,
        dailyBuckets,
        loading,
        error,
        refresh
    };
}

// EXTERNAL MODULE: ./src/lib/hooks/useGenerateRule.ts
var useGenerateRule = __webpack_require__(20733);
;// ./src/lib/hooks/useExportRules.ts
/* __next_internal_client_entry_do_not_use__ useExportRules auto */ 




/**
 * Hook that calls ExportRules and triggers a browser file download.
 * - Creates a Blob from the returned YAML content.
 * - Triggers an <a download="rules.yaml"> click to start the download.
 * - Revokes the object URL after the click to release memory.
 */ function useExportRules() {
    const [loading, setLoading] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    // Initialize client inside useEffect to avoid SSR issues (Concern 3 from validation.md).
    const clientRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
    }, []);
    const exportRules = (0,react.useCallback)(async (ruleIds)=>{
        if (!clientRef.current) return;
        setLoading(true);
        setError(null);
        try {
            const req = (0,create/* create */.v)(session_pb/* ExportRulesRequestSchema */.qCD, {
                ruleIds: ruleIds !== null && ruleIds !== void 0 ? ruleIds : []
            });
            const resp = await clientRef.current.exportRules(req);
            const blob = new Blob([
                resp.yamlContent
            ], {
                type: "text/yaml"
            });
            const url = URL.createObjectURL(blob);
            const a = document.createElement("a");
            a.href = url;
            a.download = "rules.yaml";
            a.click();
            URL.revokeObjectURL(url);
        } catch (err) {
            setError(err instanceof Error ? err : new Error("Export failed"));
        } finally{
            setLoading(false);
        }
    }, []);
    return {
        exportRules,
        loading,
        error
    };
}

// EXTERNAL MODULE: ./src/gen/session/v1/types_pb.ts
var types_pb = __webpack_require__(81432);
// EXTERNAL MODULE: ./src/components/sessions/SuggestedRuleCard.tsx + 1 modules
var SuggestedRuleCard = __webpack_require__(10711);
// EXTERNAL MODULE: ./src/components/ui/Modal.tsx + 1 modules
var Modal = __webpack_require__(79898);
;// ./src/lib/hooks/useValidateRules.ts
/* __next_internal_client_entry_do_not_use__ useValidateRules auto */ 





/**
 * Debounced hook that calls ValidateRules on YAML input.
 * - Debounce delay is configurable (default 400ms).
 * - Clears results when yamlContent is empty.
 * - Cancels in-flight requests via AbortController when input changes or component unmounts.
 */ function useValidateRules(yamlContent) {
    let debounceMs = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : 400;
    const [results, setResults] = (0,react.useState)([]);
    const [loading, setLoading] = (0,react.useState)(false);
    const [validCount, setValidCount] = (0,react.useState)(0);
    const [errorCount, setErrorCount] = (0,react.useState)(0);
    const [error, setError] = (0,react.useState)(null);
    // Initialize client inside useEffect to avoid SSR issues.
    const clientRef = (0,react.useRef)(null);
    const abortRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
    }, []);
    (0,react.useEffect)(()=>{
        if (!yamlContent.trim()) {
            setResults([]);
            setValidCount(0);
            setErrorCount(0);
            setError(null);
            setLoading(false);
            return;
        }
        const timer = setTimeout(async ()=>{
            var // Cancel any in-flight request before starting a new one.
            _abortRef_current;
            if (!clientRef.current) return;
            (_abortRef_current = abortRef.current) === null || _abortRef_current === void 0 ? void 0 : _abortRef_current.abort();
            abortRef.current = new AbortController();
            setLoading(true);
            setError(null);
            try {
                const req = (0,create/* create */.v)(session_pb/* ValidateRulesRequestSchema */.wxB, {
                    yamlContent
                });
                const resp = await clientRef.current.validateRules(req, {
                    signal: abortRef.current.signal
                });
                var _resp_results;
                setResults((_resp_results = resp.results) !== null && _resp_results !== void 0 ? _resp_results : []);
                setValidCount(resp.validCount);
                setErrorCount(resp.errorCount);
            } catch (err) {
                if (err.name === "AbortError") return;
                setError(err instanceof Error ? err : new Error("Validation failed"));
                setResults([]);
                setValidCount(0);
                setErrorCount(0);
            } finally{
                setLoading(false);
            }
        }, debounceMs);
        return ()=>{
            var // Cancel in-flight request on unmount or when dependencies change.
            _abortRef_current;
            clearTimeout(timer);
            (_abortRef_current = abortRef.current) === null || _abortRef_current === void 0 ? void 0 : _abortRef_current.abort();
        };
    }, [
        yamlContent,
        debounceMs
    ]);
    return {
        results,
        loading,
        validCount,
        errorCount,
        error
    };
}

;// ./src/lib/hooks/useBulkUpsertRules.ts
/* __next_internal_client_entry_do_not_use__ useBulkUpsertRules auto */ 




/**
 * Hook that calls BulkUpsertRules and returns apply results.
 * - Returns created/updated/skipped counts and per-rule errors.
 * - Loading state is managed automatically.
 */ function useBulkUpsertRules() {
    const [loading, setLoading] = (0,react.useState)(false);
    const [result, setResult] = (0,react.useState)(null);
    const [error, setError] = (0,react.useState)(null);
    // Initialize client inside useEffect to avoid SSR issues.
    const clientRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
    }, []);
    const applyRules = (0,react.useCallback)(async (rules, overwriteDuplicates)=>{
        if (!clientRef.current) return {
            errors: []
        };
        setLoading(true);
        setError(null);
        setResult(null);
        try {
            const req = (0,create/* create */.v)(session_pb/* BulkUpsertRulesRequestSchema */.bL3, {
                rules,
                overwriteDuplicates
            });
            const resp = await clientRef.current.bulkUpsertRules(req);
            setResult(resp);
            return {
                errors: []
            };
        } catch (err) {
            const error = err instanceof Error ? err : new Error("Apply rules failed");
            setError(error);
            return {
                errors: [
                    error.message
                ]
            };
        } finally{
            setLoading(false);
        }
    }, []);
    return {
        applyRules,
        loading,
        result,
        error
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FImportRulesModal.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA7VX226jMBB9z1f4ZaXmgQhomlL3YyoDQ-KtsV1jEpLV_vsKk4sNGFDVfYkUY5-5nOOZ8eYjSkXxdg5D9GeF0Inm-oBRFIa_3lcIlZQHB6D7g8YoDkPZtIuF4DooSEnZGaNScFFJksH9S0UvgFEUd5slyXPK9y2kbO6rKck-90rUPMfoSNRTEHxEb3CJ1SUi63ZDJphQg2-p-ZYKlYPCKJINqgSjeW_bp7UrUCSndYXRrjOsoHPvCErTjLBuZ9M6bby8nkpF8776u9rck4MLkdWVSZGoNaMcMOKCg2Vp1GW2dnAigzB1oDTOC0kyqs8YhZvkxUGIDUJOK8nIGaOCQccJgybIqYJMU8Fxm7-65O2XPZEYJV30JWnudD7vrnSKI6iCiVNwxojUWjjmno05D4mjGb6uMihaI3eKtCK8kkQB19MCeF3EMbh53dp5bU2PJ7d2T70sOxWF6xnNRi7ubhluMwN7dlFfl6Hu-wJ6dVCScf0QRvc8oBrKCqMMuAbVF4-R2Em1K-2vg_pmUM31P131tQtD70XuEmpXi2fpXjji6i6WTU9fN9VtrZp0xYqspZ43GhodGCkWQpUY1VKCykgFjvG0y7SXmKO_RPUUls0gGeX4ql3oYuUzWGc_1MVFghmk3I8Uu0jF_6DJwt__WMyHH4uZTlRg-3pcb8_2VnrVnvJAi9uShfjbn0Wvv8miMlmMtUI_F-M6fFzWkaZvhfFpwrAD3TkzwLUnRLuh_XjKfu8eMNtMKrQWpcmZs6kcJ8kwcnPAU_MGZWkiL5ZFPmFx2mBWq6qFl4J2SxaqGJvKeu14ViulX9tfjox6E81NMMmQsK2_xg7CQV3zp91k8nAThZu4QtCvvgIf2pEEc6GfcE4rkjLI1zPXl697GLeD3cj26IZb20UudEAYEyfInePSnPIWg1GNmNZiGLa5vVOVyAa5te1rEJKb_sf_kYwu9C22fMshE4p0JNQ8B9VOsY6Prn9qUcX8VgXa9d4GiTVV-lrE6INjomCdDlRDYHZiJBU4o25zG3VH6pWVguox1rgJXTzpdJRMjB0IMdAalPG0Y2ETbqF8H9a4ftvQ33oKRP0ga7f7JMueaXODi4_z4dD3DxxWEHKCDgAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FImportRulesModal_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA7VX226jMBB9z1f4ZaXmgQhomlL3YyoDQ_KtsV1jEpLV_vsKk4sNGFDVfYkUY5_5nOOZ8eYjSkXxdg5D9GeF0Inm_oBRFIa_3lcIlZQHB6D7g8YoDkPZtIuF4DooSEnZGaNScFFJksH9S0UvgFEUd5slyXPK9y2kbO6rKck_90rUPMfoSNRTEHxEb3CJ1SUi63ZDJphQg2_p_ZYKlYPCKJINqgSjeW_bp7UrUCSndYXRrjOsoHPvCErTjLBuZ9M6bby8nkpF8776u9rck4MLkdWVSZGoNaMcMOKCg2Vp1GW2dnAigzB1oDTOC0kyqs8YhZvkxUGIDUJOK8nIGaOCQccJgybIqYJMU8Fxm7_65O2XPZEYJV30JWnudD7vrnSKI6iCiVNwxojUWjjmno05D4mjGb6uMihaI3eKtCK8kkQB19MCeF3EMbh53dp5bU2PJ7d2T70sOxWF6xnNRi7ubhluMwN7dlFfl6Hu_wJ6dVCScf0QRvc8oBrKCqMMuAbVF4_R2Em1K_2vg_pmUM31P131tQtD70XuEmpXi2fpXjji6i6WTU9fN9VtrZp0xYqspZ43GhodGCkWQpUY1VKCykgFjvG0y7SXmKO_RPUUls0gGeX4ql3oYuUzWGc_1MVFghmk3I8Uu0jF_6DJwt_WMyHH4uZTlRg_3pcb8_2VnrVnvJAi9uShfjbn0Wvv8miMlmMtUI_F_M6fFzWkaZvhfFpwrAD3TkzwLUnRLuh_XjKfu8eMNtMKrQWpcmZs6kcJ8kwcnPAU_MGZWkiL5ZFPmFx2mBWq6qFl4J2SxaqGJvKeu14ViulX9tfjox6E81NMMmQsK2_xg7CQV3zp91k8nAThZu4QtCvvgIf2pEEc6GfcE4rkjLI1zPXl697GLeD3cj26IZb20UudEAYEyfInePSnPIWg1GNmNZiGLa5vVOVyAa5te1rEJKb_sf_kYwu9C22fMshE4p0JNQ8B9VOsY6Prn9qUcX8VgXa9d4GiTVV_lrE6INjomCdDlRDYHZiJBU4o25zG3VH6pWVguox1rgJXTzpdJRMjB0IMdAalPG0Y2ETbqF8H9a4ftvQ33oKRP0ga7f7JMueaXODi4_z4dD3DxxWEHKCDgAA_22_7D = __webpack_require__(79116);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+recipes@0.5.7+6974e6bd53375d7a/node_modules/@vanilla-extract/recipes/dist/createRuntimeFn-62c9670f.esm.js
var createRuntimeFn_62c9670f_esm = __webpack_require__(8462);
;// ./src/components/sessions/ImportRulesModal.css.ts



var applyButton = '_1bof9y0o';
var decisionBadge = (0,createRuntimeFn_62c9670f_esm.c)({defaultClassName:'_1bof9y0a',variantClassNames:{decision:{allow:'_1bof9y0b',deny:'_1bof9y0c',escalate:'_1bof9y0d',unknown:'_1bof9y0e'}},defaultVariants:{decision:'unknown'},compoundVariants:[]});
var duplicateRadioGroup = '_1bof9y0m';
var duplicateRadioLabel = '_1bof9y0n';
var errorList = '_1bof9y0k';
var errorListItem = '_1bof9y0l';
var exampleBlock = '_1bof9y0r';
var exampleToggle = '_1bof9y0q';
var formStack = '_1bof9y0t';
var matchChip = '_1bof9y0j';
var matchChips = '_1bof9y0i';
var noValidRulesMessage = '_1bof9y0p';
var partialErrorBanner = '_1bof9y0u';
var previewList = '_1bof9y02';
var ruleCard = (0,createRuntimeFn_62c9670f_esm.c)({defaultClassName:'_1bof9y03',variantClassNames:{status:{valid:'_1bof9y04',error:'_1bof9y05',overwrite:'_1bof9y06',skip:'_1bof9y07'}},defaultVariants:{},compoundVariants:[]});
var ruleCardHeader = '_1bof9y08';
var ruleCardName = '_1bof9y09';
var sectionLabel = '_1bof9y0s';
var statusBadge = (0,createRuntimeFn_62c9670f_esm.c)({defaultClassName:'_1bof9y0f',variantClassNames:{type:{overwrite:'_1bof9y0g',skip:'_1bof9y0h'}},defaultVariants:{},compoundVariants:[]});
var yamlTextarea = '_1bof9y00';
var yamlTextareaLoading = '_1bof9y01 _1bof9y00';
;// ./src/components/sessions/ParsedRuleCard.tsx
/* __next_internal_client_entry_do_not_use__ ParsedRuleCard auto */ 


function decisionLabel(d) {
    switch(d){
        case types_pb/* AutoDecision */.Uz.ALLOW:
            return "Allow";
        case types_pb/* AutoDecision */.Uz.DENY:
            return "Deny";
        default:
            return "Escalate";
    }
}
function decisionVariant(d) {
    switch(d){
        case types_pb/* AutoDecision */.Uz.ALLOW:
            return "allow";
        case types_pb/* AutoDecision */.Uz.DENY:
            return "deny";
        case types_pb/* AutoDecision */.Uz.ESCALATE:
            return "escalate";
        default:
            return "unknown";
    }
}
function ParsedRuleCard(param) {
    let { result, status } = param;
    const rule = result.rule;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: ruleCard({
            status
        }),
        "data-testid": "parsed-rule-card-".concat(status),
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: ruleCardHeader,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: ruleCardName,
                        children: result.originalName || (rule === null || rule === void 0 ? void 0 : rule.name) || "(unnamed)"
                    }),
                    rule && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: decisionBadge({
                            decision: decisionVariant(rule.decision)
                        }),
                        children: decisionLabel(rule.decision)
                    }),
                    status === "overwrite" && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: statusBadge({
                            type: "overwrite"
                        }),
                        "data-testid": "overwrite-badge",
                        children: "will overwrite"
                    }),
                    status === "skip" && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: statusBadge({
                            type: "skip"
                        }),
                        "data-testid": "skip-badge",
                        children: "will skip"
                    })
                ]
            }),
            rule && (rule.toolName || rule.programs && rule.programs.length > 0 || rule.toolPattern || rule.commandPattern || rule.filePattern) && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: matchChips,
                children: [
                    rule.toolName && /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                        className: matchChip,
                        children: rule.toolName
                    }),
                    rule.toolPattern && /*#__PURE__*/ (0,jsx_runtime.jsxs)("code", {
                        className: matchChip,
                        children: [
                            "pattern: ",
                            rule.toolPattern
                        ]
                    }),
                    rule.programs && rule.programs.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("code", {
                        className: matchChip,
                        children: [
                            "programs: ",
                            rule.programs.join(", ")
                        ]
                    }),
                    rule.subcommands && rule.subcommands.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("code", {
                        className: matchChip,
                        children: [
                            "sub: ",
                            rule.subcommands.join(", ")
                        ]
                    }),
                    rule.commandPattern && /*#__PURE__*/ (0,jsx_runtime.jsxs)("code", {
                        className: matchChip,
                        children: [
                            "cmd: ",
                            rule.commandPattern
                        ]
                    }),
                    rule.filePattern && /*#__PURE__*/ (0,jsx_runtime.jsxs)("code", {
                        className: matchChip,
                        children: [
                            "file: ",
                            rule.filePattern
                        ]
                    })
                ]
            }),
            status === "error" && result.errors.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                className: errorList,
                "data-testid": "error-list",
                children: result.errors.map((err, i)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("li", {
                        className: errorListItem,
                        children: err
                    }, i))
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/components/sessions/ApprovalRulesPanel.css.ts
var ApprovalRulesPanel_css = __webpack_require__(696);
;// ./src/components/sessions/ImportRulesModal.tsx
/* __next_internal_client_entry_do_not_use__ ImportRulesModal auto */ 
// +feature: approval-rules:yaml-import







const EXAMPLE_YAML = "rules:\n- name: Allow git log\n  tool: Bash\n  programs:\n    - git\n  subcommands:\n    - log\n  decision: allow\n  reason: Read-only git history\n- name: Deny git push\n  tool: Bash\n  programs:\n    - git\n  subcommands:\n    - push\n  decision: deny\n";
/**
 * ImportRulesModal allows users to paste YAML rules, see live validation results,
 * and apply valid rules in bulk.
 */ function ImportRulesModal(param) {
    let { open, onClose, onApplied, existingRules } = param;
    const [yamlContent, setYamlContent] = (0,react.useState)("");
    const [duplicateMode, setDuplicateMode] = (0,react.useState)("skip");
    const [showExample, setShowExample] = (0,react.useState)(false);
    const [partialErrors, setPartialErrors] = (0,react.useState)([]);
    const { results, loading: validating, validCount, errorCount } = useValidateRules(yamlContent);
    const { applyRules, loading: applying, error: applyError } = useBulkUpsertRules();
    // Build a set of existing rule names for duplicate detection.
    const existingNames = new Set(existingRules.map((r)=>r.name));
    // Determine card status for each result.
    const cardStatus = (originalName, valid)=>{
        if (!valid) return "error";
        if (existingNames.has(originalName)) {
            return duplicateMode === "overwrite" ? "overwrite" : "skip";
        }
        return "valid";
    };
    // Count truly applicable valid rules (valid + not skipped).
    const applicableCount = results.filter((r)=>{
        if (!r.valid) return false;
        if (existingNames.has(r.originalName) && duplicateMode === "skip") return false;
        return true;
    }).length;
    const handleApply = async ()=>{
        setPartialErrors([]);
        const validRules = results.filter((r)=>{
            if (!r.valid || !r.rule) return false;
            if (existingNames.has(r.originalName) && duplicateMode === "skip") return false;
            return true;
        }).map((r)=>r.rule);
        const { errors } = await applyRules(validRules, duplicateMode === "overwrite");
        if (errors.length > 0) {
            setPartialErrors(errors);
            return;
        }
        onApplied();
        onClose();
        setYamlContent("");
        setPartialErrors([]);
    };
    const applyButtonLabel = ()=>{
        if (errorCount > 0 && validCount > 0) {
            const errLabel = errorCount === 1 ? "1 has errors" : "".concat(errorCount, " have errors");
            return "Apply ".concat(validCount, " rules (").concat(errLabel, ")");
        }
        return "Apply ".concat(validCount, " rules");
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(Modal/* Modal */.aF, {
        open: open,
        onOpenChange: (o)=>{
            if (!o) {
                onClose();
                setYamlContent("");
                setPartialErrors([]);
            }
        },
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)(Modal/* ModalContent */.$m, {
            showClose: false,
            className: ApprovalRulesPanel_css/* ruleModalContent */.RK,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: ApprovalRulesPanel_css/* modalHeader */._z,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: ApprovalRulesPanel_css/* modalTitleRow */.gn,
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(Modal/* ModalTitle */.wt, {
                                children: "Import Rules from YAML"
                            })
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)(Modal/* ModalClose */.Oj, {
                            className: ApprovalRulesPanel_css/* modalCloseButton */.iT,
                            "aria-label": "Close dialog",
                            children: "\xd7"
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: ApprovalRulesPanel_css/* modalBody */.S3,
                    children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: formStack,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("textarea", {
                                        className: validating ? yamlTextareaLoading : yamlTextarea,
                                        value: yamlContent,
                                        onChange: (e)=>setYamlContent(e.target.value),
                                        placeholder: "Paste your rules YAML here…",
                                        "aria-label": "YAML rules input",
                                        "data-testid": "yaml-input",
                                        rows: 10
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        type: "button",
                                        className: exampleToggle,
                                        onClick: ()=>setShowExample((v)=>!v),
                                        "data-testid": "show-example-toggle",
                                        children: showExample ? "Hide example" : "Show example YAML"
                                    }),
                                    showExample && /*#__PURE__*/ (0,jsx_runtime.jsx)("pre", {
                                        className: exampleBlock,
                                        children: EXAMPLE_YAML
                                    })
                                ]
                            }),
                            results.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: sectionLabel,
                                        children: [
                                            "Preview (",
                                            validCount,
                                            " valid, ",
                                            errorCount,
                                            " with errors)"
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                        className: previewList,
                                        "data-testid": "preview-list",
                                        children: results.map((result, i)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(ParsedRuleCard, {
                                                result: result,
                                                status: cardStatus(result.originalName, result.valid)
                                            }, i))
                                    })
                                ]
                            }),
                            results.length > 0 && validCount === 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                className: noValidRulesMessage,
                                "data-testid": "no-valid-rules-message",
                                children: "No valid rules to apply. Fix the errors above and try again."
                            }),
                            validCount > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                        className: sectionLabel,
                                        children: "Duplicate handling"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: duplicateRadioGroup,
                                        "data-testid": "duplicate-mode-group",
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                                className: duplicateRadioLabel,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                        type: "radio",
                                                        name: "duplicateMode",
                                                        value: "skip",
                                                        checked: duplicateMode === "skip",
                                                        onChange: ()=>setDuplicateMode("skip"),
                                                        "data-testid": "duplicate-mode-skip"
                                                    }),
                                                    "Skip existing (default)"
                                                ]
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                                className: duplicateRadioLabel,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                        type: "radio",
                                                        name: "duplicateMode",
                                                        value: "overwrite",
                                                        checked: duplicateMode === "overwrite",
                                                        onChange: ()=>setDuplicateMode("overwrite"),
                                                        "data-testid": "duplicate-mode-overwrite"
                                                    }),
                                                    "Overwrite existing"
                                                ]
                                            })
                                        ]
                                    })
                                ]
                            }),
                            (partialErrors.length > 0 || applyError) && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: partialErrorBanner,
                                "data-testid": "partial-error-banner",
                                children: partialErrors.length > 0 ? partialErrors.join(", ") : applyError === null || applyError === void 0 ? void 0 : applyError.message
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: applyButton,
                                disabled: applicableCount === 0 || applying || validating,
                                onClick: handleApply,
                                "data-testid": "apply-button",
                                children: applying ? "Applying…" : applyButtonLabel()
                            })
                        ]
                    })
                })
            ]
        })
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Frules%2FTagInput.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA41Uy46jMBC85yv6stLOwRGQmcms57J_supAA62AjWwzJFnl31eYgTUJeVywsIvu6qoyv_d0zA3WZIG7tm0pgr8rgOiHXwB06ypWJCFpDmB1xRl8ofkpxJ_4F50Sc6pfPkOg0HluyUmIm0N_cF4BxNG9cs6gsg0aUu5BpfNqPXCMfbWMbVPhUUJekUf0q-gMNhL6Z79V9C9zwsnRM24wy1gVC6eXOyeP32mTkfFsFnXYByhhMOPWXtbebAcMpvvC6FZll4AYPSBtjdVGgqODl6RmJUrionQSNh-DGlhxoQQ7qq2ElJQj8xkIJHOdtlZ07EpWXq1vZqmu-tLzvpVv-6TVi_ZMnRPfDBXX6FgrOaUqsUBoSejWQa5NhyazwXebuaWsfI_R2cVpn3K3n-WOn7eceveYXCsnLJ_o6nz3wMlBqkWx3_6XzrHm6ng1QusRXoDR9nj92u91JTsStsGUJCg9pHzS8HUw-iap9Dap5CWo83ZVR2lF4SUY38ekNppHTyblo177oCGrkgz7QOsGU3ZHCdF6-4zOcylCprLUX2Q836loCHj3R32M_P5wlzrOXCkhTqKB4OVU00WYxg6kuPhdLaoZ756Kz-MMzGIchXNJ2VSYUqmr7Hv-ZSZpaOx2UOMmLbwfEIAaTcFKOL18787_APkCOf5OBgAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Frules_2FTagInput_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA41Uy46jMBC85yv6stLOwRGQmcms57J_supAA62AjWwzJFnl31eYgTUJeVywsIvu6qoyv_d0zA3WZIG7tm0pgr8rgOiHXwB06ypWJCFpDmB1xRl8ofkpxJ_4F50Sc6pfPkOg0HluyUmIm0N_cF4BxNG9cs6gsg0aUu5BpfNqPXCMfbWMbVPhUUJekUf0q_gMNhL6Z79V9C9zwsnRM24wy1gVC6eXOyeP32mTkfFsFnXYByhhMOPWXtbebAcMpvvC6FZll4AYPSBtjdVGgqODl6RmJUrionQSNh_DGlhxoQQ7qq2ElJQj8xkIJHOdtlZ07EpWXq1vZqmu_tLzvpVv_6TVi_ZMnRPfDBXX6FgrOaUqsUBoSejWQa5NhyazwXebuaWsfI_R2cVpn3K3n_WOn7eceveYXCsnLJ_o6nz3wMlBqkWx3_6XzrHm6ng1QusRXoDR9nj92u91JTsStsGUJCg9pHzS8HUw_iap9Dap5CWo83ZVR2lF4SUY38ekNppHTyblo177oCGrkgz7QOsGU3ZHCdF6_4zOcylCprLUX2Q836loCHj3R32M_P5wlzrOXCkhTqKB4OVU00WYxg6kuPhdLaoZ756Kz_MMzGIchXNJ2VSYUqmr7Hv_ZSZpaOx2UOMmLbwfEIAaTcFKOL18787_APkCOf5OBgAA_22_7D = __webpack_require__(98216);
;// ./src/components/rules/TagInput.css.ts

var chip = 'iwuuue3';
var chipDisabled = 'iwuuue4';
var chipRemove = 'iwuuue5';
var container = 'iwuuue1';
var containerPrefilled = 'iwuuue2';
var helperText = 'iwuuue7';
var input = 'iwuuue6';
;// ./src/components/rules/TagInput.tsx
/* __next_internal_client_entry_do_not_use__ TagInput auto */ 


function TagInput(param) {
    let { value, onChange, placeholder, disabled, helperText: helper, isPrefilled } = param;
    const [text, setText] = (0,react.useState)("");
    const inputRef = (0,react.useRef)(null);
    const [showHighlight, setShowHighlight] = (0,react.useState)(false);
    (0,react.useEffect)(()=>{
        if (isPrefilled && value.length > 0) {
            setShowHighlight(true);
            const t = setTimeout(()=>setShowHighlight(false), 2100);
            return ()=>clearTimeout(t);
        }
    }, [
        isPrefilled,
        value.length
    ]);
    function addTag(raw) {
        const trimmed = raw.trim();
        if (!trimmed || value.includes(trimmed)) return;
        onChange([
            ...value,
            trimmed
        ]);
    }
    function removeTag(tag) {
        onChange(value.filter((t)=>t !== tag));
    }
    function handleKeyDown(e) {
        if ((e.key === "Enter" || e.key === ",") && text) {
            e.preventDefault();
            addTag(text);
            setText("");
        } else if (e.key === "Backspace" && !text && value.length > 0) {
            onChange(value.slice(0, -1));
        }
    }
    function handlePaste(e) {
        e.preventDefault();
        const pasted = e.clipboardData.getData("text");
        pasted.split(/[,\s]+/).forEach(addTag);
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: "".concat(container, " ").concat(showHighlight ? containerPrefilled : ""),
                onClick: ()=>{
                    var _inputRef_current;
                    return (_inputRef_current = inputRef.current) === null || _inputRef_current === void 0 ? void 0 : _inputRef_current.focus();
                },
                children: [
                    value.map((tag)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                            className: "".concat(chip, " ").concat(disabled ? chipDisabled : ""),
                            children: [
                                tag,
                                !disabled && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "button",
                                    className: chipRemove,
                                    onClick: (e)=>{
                                        e.stopPropagation();
                                        removeTag(tag);
                                    },
                                    "aria-label": "Remove ".concat(tag),
                                    children: "\xd7"
                                })
                            ]
                        }, tag)),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        ref: inputRef,
                        className: input,
                        value: text,
                        onChange: (e)=>setText(e.target.value),
                        onKeyDown: handleKeyDown,
                        onPaste: handlePaste,
                        onBlur: ()=>{
                            if (text) {
                                addTag(text);
                                setText("");
                            }
                        },
                        placeholder: value.length === 0 ? placeholder : "",
                        disabled: disabled,
                        "aria-label": placeholder
                    })
                ]
            }),
            helper && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: helperText,
                children: helper
            })
        ]
    });
}

;// ./src/lib/rulePreview.ts
// Client-side port of the key subset of CommandCriteria.Matches() from pkg/classifier.
// Skips: SafePythonImportsOnly (requires AST), RedirectionPattern (not in UI form).
// Programs whose subcommand extraction captures two tokens.
const DEEP_SUBCOMMAND_PROGRAMS = new Set([
    "gh",
    "docker",
    "kubectl",
    "aws",
    "gcloud",
    "az",
    "terraform",
    "ip",
    "git",
    "npm",
    "cargo",
    "go",
    "pip",
    "uv"
]);
function matchesProgram(programs, program) {
    if (programs.length === 0) return true;
    const lower = program.toLowerCase();
    return programs.some((p)=>{
        const pl = p.toLowerCase();
        return lower === pl || lower.startsWith(pl + ".");
    });
}
function extractSubcommand(program, args) {
    const filtered = args.filter((a)=>!a.startsWith("-") || a.includes(" "));
    const depth = DEEP_SUBCOMMAND_PROGRAMS.has(program.toLowerCase()) ? 2 : 1;
    return filtered.slice(0, depth).join(" ");
}
function parseSimpleCommand(cmd) {
    const parts = cmd.trim().split(/\s+/);
    var _parts_;
    return {
        program: (_parts_ = parts[0]) !== null && _parts_ !== void 0 ? _parts_ : "",
        args: parts.slice(1)
    };
}
function detectPythonMode(program, args) {
    if (!program.toLowerCase().startsWith("python") && !program.toLowerCase().startsWith("pypy")) {
        return "";
    }
    if (args.includes("-c")) return "inline";
    if (args.includes("-m")) return "module";
    if (args.includes("-V") || args.includes("--version")) return "version";
    const scriptArg = args.find((a)=>!a.startsWith("-") && a.endsWith(".py"));
    if (scriptArg) return "script";
    return "";
}
function matchesCriteria(criteria, cmd) {
    const { program, args } = parseSimpleCommand(cmd);
    if (!program) return false;
    if (!matchesProgram(criteria.programs, program)) return false;
    const sub = extractSubcommand(program, args);
    if (criteria.subcommands.length > 0) {
        const found = criteria.subcommands.some((s)=>sub === s || sub.startsWith(s + " "));
        if (!found) return false;
    }
    for (const bs of criteria.blockedSubcommands){
        if (sub === bs) return false;
    }
    if (criteria.requiredFlags.length > 0) {
        const found = criteria.requiredFlags.some((rf)=>args.includes(rf));
        if (!found) return false;
    }
    if (criteria.requiredFlagPrefixes.length > 0) {
        const found = criteria.requiredFlagPrefixes.some((prefix)=>args.some((a)=>a.startsWith(prefix)));
        if (!found) return false;
    }
    for (const ff of criteria.forbiddenFlags){
        if (args.includes(ff)) return false;
    }
    if (criteria.pythonModes.length > 0) {
        const mode = detectPythonMode(program, args);
        if (!criteria.pythonModes.includes(mode)) return false;
    }
    return true;
}
// Static example bank — one entry per (program, common-usage) pair.
const EXAMPLE_BANK = [
    // git
    "git log --oneline",
    "git status",
    "git diff HEAD",
    "git push origin main",
    "git push --force",
    "git reset --hard HEAD~1",
    "git checkout main",
    "git commit -m 'fix'",
    "git pull",
    "git branch -d old-branch",
    // python
    "python3 script.py",
    "python3 -m pytest",
    "python3 -c \"import json; print(json.dumps({}))\"",
    "python3 -c \"import requests; requests.get('http://example.com')\"",
    "python3 -V",
    "python script.py --arg value",
    // npm / yarn
    "npm install",
    "npm test",
    "npm publish",
    "npm run build",
    "yarn install",
    "npx jest",
    "pnpm install",
    // docker
    "docker ps",
    "docker logs my-container",
    "docker run -it ubuntu bash",
    "docker build -t my-image .",
    "docker exec -it container bash",
    // go
    "go build ./...",
    "go test ./...",
    "go run main.go",
    "go fmt ./...",
    // gh
    "gh pr list",
    "gh pr view 123",
    "gh pr create",
    "gh pr merge 123",
    "gh issue create",
    "gh api repos/owner/repo",
    "gh auth login",
    // curl
    "curl https://api.example.com",
    "curl -X POST https://api.example.com -d '{}'",
    "curl -o output.txt https://example.com",
    // misc
    "make build",
    "make test",
    "rm -rf /tmp/old",
    "sed -i 's/foo/bar/g' file.txt",
    "brew install jq",
    "pip install requests",
    "cargo build",
    "kubectl get pods",
    "terraform plan"
];
/**
 * Given criteria and real command stats, compute how many real decisions the rule
 * would cover, and which subcommands are not yet covered.
 */ function computeCoverage(criteria, stats) {
    if (criteria.programs.length === 0 || stats.length === 0) {
        return {
            covered: 0,
            total: 0,
            uncoveredSubcommands: []
        };
    }
    let covered = 0;
    let total = 0;
    const uncoveredSubcommands = [];
    for (const stat of stats){
        // Only consider stats whose program matches the criteria
        if (!matchesProgram(criteria.programs, stat.programName)) continue;
        total += stat.count;
        // Build a synthetic command string to test against full criteria
        const synthetic = stat.subcommand ? "".concat(stat.programName, " ").concat(stat.subcommand) : stat.programName;
        if (matchesCriteria(criteria, synthetic)) {
            covered += stat.count;
        } else if (criteria.subcommands.length > 0 && stat.subcommand && !uncoveredSubcommands.includes(stat.subcommand)) {
            uncoveredSubcommands.push(stat.subcommand);
        }
    }
    // Sort uncovered by frequency (most common first) using original stats
    uncoveredSubcommands.sort((a, b)=>{
        var _stats_find, _stats_find1;
        var _stats_find_count;
        const ca = (_stats_find_count = (_stats_find = stats.find((s)=>s.subcommand === a)) === null || _stats_find === void 0 ? void 0 : _stats_find.count) !== null && _stats_find_count !== void 0 ? _stats_find_count : 0;
        var _stats_find_count1;
        const cb = (_stats_find_count1 = (_stats_find1 = stats.find((s)=>s.subcommand === b)) === null || _stats_find1 === void 0 ? void 0 : _stats_find1.count) !== null && _stats_find_count1 !== void 0 ? _stats_find_count1 : 0;
        return cb - ca;
    });
    return {
        covered,
        total,
        uncoveredSubcommands: uncoveredSubcommands.slice(0, 8)
    };
}
function computePreview(criteria) {
    const hasAnyCriteria = criteria.programs.length > 0 || criteria.subcommands.length > 0 || criteria.blockedSubcommands.length > 0 || criteria.requiredFlags.length > 0 || criteria.forbiddenFlags.length > 0 || criteria.requiredFlagPrefixes.length > 0 || criteria.pythonModes.length > 0;
    if (!hasAnyCriteria) return {
        matches: [],
        nonMatches: []
    };
    const matches = [];
    const nonMatches = [];
    for (const cmd of EXAMPLE_BANK){
        if (matches.length >= 5 && nonMatches.length >= 3) break;
        if (matchesCriteria(criteria, cmd)) {
            if (matches.length < 5) matches.push(cmd);
        } else {
            if (nonMatches.length < 3) nonMatches.push(cmd);
        }
    }
    return {
        matches,
        nonMatches
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Frules%2FRulePreview.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61Wy27bMBC8-yt4KZAcaEhy6jTspX8S0OJK2oYiCZKKZRf-90KUnehlWXkcDAHmYGZ3drTi-jnWpfH8MSL_VoSU3OaoqNeGkVdu7yh9jp_gmNjjJr7_vSJkp60A2wJiUxOnJYoBNAtIw4VAlV_lOq3WF-04aGdaeerwCCP4LhCG8z1gXvgRogiIVEtth2dt3efGdtp7XQ4hybFfTxLqEeiM5AdGcoui4Wie1ENpJPdAUy2rUjlG4sw2v4Dg416jPvemz51JqENzEmoq0ELqUStGWvYrnMmhz_lwwz--0L95kwaiP4PopOVVH7m9jowH7jwu7yTjJcrDqMqqG76JHqZG_xZrywVWbqS6bTE8fcmtrpQYAtLr6YvC0b5AD9QZngIjSu8tN83f-hVsJvWekQKFgDBuD7Wn7wcgJRqHrufSr0UuTdaTvBvo_EECI-i5xLTH_9TdBdNp2LSNfW-O-ef7iqfL-Vs5j9mBplp5UJ6RMAO6A7-Hxu-O-C6IF-f3Y2vq27l4WpSL8ZQ7qmlPNY6iH_OU1YK4tmV5y5XDdgp7FL4g0TpxBLiDXgViZiE1QWXkEtdrEySES8wVRQ-lYyQF5cHe3CfDxQhfDnVTsSssqhdGoh539t1rJTH1ZxbJssC0HLe-rzOvQVpZ15wZjZdRSFRA32K2fujbw4ompMGk68WJboOT6ji_BzuK-Y3bxtnNL6RhYR-zG_9jX5HT6k8JAjm5K3lNwyvHyDaKTH0fuh1cL-buE434aXX6DyFWrL-dCQAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Frules_2FRulePreview_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61Wy27bMBC8_yt4KZAcaEhy6jTspX8S0OJK2oYiCZKKZRf_90KUnehlWXkcDAHmYGZ3drTi_jnWpfH8MSL_VoSU3OaoqNeGkVdu7yh9jp_gmNjjJr7_vSJkp60A2wJiUxOnJYoBNAtIw4VAlV_lOq3WF_04aGdaeerwCCP4LhCG8z1gXvgRogiIVEtth2dt3efGdtp7XQ4hybFfTxLqEeiM5AdGcoui4Wie1ENpJPdAUy2rUjlG4sw2v4Dg416jPvemz51JqENzEmoq0ELqUStGWvYrnMmhz_lwwz_0L95kwaiP4PopOVVH7m9jowH7jwu7yTjJcrDqMqqG76JHqZG_xZrywVWbqS6bTE8fcmtrpQYAtLr6YvC0b5AD9QZngIjSu8tN83f_hVsJvWekQKFgDBuD7Wn7wcgJRqHrufSr0UuTdaTvBvo_EECI_i5xLTH_9TdBdNp2LSNfW_O_ef7iqfL_Vs5j9mBplp5UJ6RMAO6A7_Hxu_O_C6IF_f3Y2vq27l4WpSL8ZQ7qmlPNY6iH_OU1YK4tmV5y5XDdgp7FL4g0TpxBLiDXgViZiE1QWXkEtdrEySES8wVRQ_lYyQF5cHe3CfDxQhfDnVTsSssqhdGoh539t1rJTH1ZxbJssC0HLe_rzOvQVpZ15wZjZdRSFRA32K2fujbw4ompMGk68WJboOT6ji_BzuK_Y3bxtnNL6RhYR_zG_9jX5HT6k8JAjm5K3lNwyvHyDaKTH0fuh1cL_buE434aXX6DyFWrL_dCQAA_22_7D = __webpack_require__(87433);
;// ./src/components/rules/RulePreview.css.ts

var colHeading = '_1ompta74';
var column = '_1ompta73';
var coverageBar = '_1ompta79';
var coverageFill = '_1ompta7c';
var coverageLabel = '_1ompta7a';
var coverageTrack = '_1ompta7b';
var empty = '_1ompta78';
var exampleRow = '_1ompta77';
var grid = '_1ompta72';
var heading = '_1ompta71';
var matchHead = '_1ompta75';
var noMatchHead = '_1ompta76';
var notice = '_1ompta7g';
var suggestionChip = '_1ompta7f';
var suggestionLabel = '_1ompta7e';
var suggestionsRow = '_1ompta7d';
var wrapper = '_1ompta70';
;// ./src/components/rules/RulePreview.tsx
/* __next_internal_client_entry_do_not_use__ RulePreview auto */ 



function RulePreview(param) {
    let { criteria, showSafePythonNotice, subcommandStats, onAddSubcommand } = param;
    const deferred = (0,react.useDeferredValue)(criteria);
    const preview = (0,react.useMemo)(()=>computePreview(deferred), [
        deferred
    ]);
    const coverage = (0,react.useMemo)(()=>subcommandStats ? computeCoverage(deferred, subcommandStats) : null, [
        deferred,
        subcommandStats
    ]);
    const hasAnyCriteria = deferred.programs.length > 0 || deferred.subcommands.length > 0 || deferred.blockedSubcommands.length > 0 || deferred.requiredFlags.length > 0 || deferred.forbiddenFlags.length > 0 || deferred.requiredFlagPrefixes.length > 0 || deferred.pythonModes.length > 0;
    if (!hasAnyCriteria) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: wrapper,
            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: "".concat(heading, " ").concat(empty),
                children: "Add criteria above to see matching examples."
            })
        });
    }
    const coveragePct = coverage && coverage.total > 0 ? Math.round(coverage.covered / coverage.total * 100) : null;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: wrapper,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: heading,
                children: "Preview"
            }),
            coverage && coverage.total > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: coverageBar,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: coverageLabel,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                children: [
                                    "Covers ",
                                    coverage.covered,
                                    " of ",
                                    coverage.total,
                                    " real decisions"
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                children: [
                                    coveragePct,
                                    "%"
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: coverageTrack,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: coverageFill,
                            style: {
                                width: "".concat(coveragePct !== null && coveragePct !== void 0 ? coveragePct : 0, "%")
                            }
                        })
                    })
                ]
            }),
            coverage && coverage.uncoveredSubcommands.length > 0 && onAddSubcommand && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: suggestionsRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: suggestionLabel,
                        children: "Also add:"
                    }),
                    coverage.uncoveredSubcommands.map((sub)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: suggestionChip,
                            type: "button",
                            title: 'Add "'.concat(sub, '" to subcommands'),
                            onClick: ()=>onAddSubcommand(sub),
                            children: sub
                        }, sub))
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: grid,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: column,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                className: "".concat(colHeading, " ").concat(matchHead),
                                children: "✓ Would match"
                            }),
                            preview.matches.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                className: empty,
                                children: "No examples found — check program spelling."
                            }) : preview.matches.map((cmd)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                                    className: exampleRow,
                                    title: cmd,
                                    children: cmd
                                }, cmd))
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: column,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                className: "".concat(colHeading, " ").concat(noMatchHead),
                                children: "✗ Would not match"
                            }),
                            preview.nonMatches.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                className: empty,
                                children: "All examples match."
                            }) : preview.nonMatches.map((cmd)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                                    className: exampleRow,
                                    title: cmd,
                                    children: cmd
                                }, cmd))
                        ]
                    })
                ]
            }),
            showSafePythonNotice && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: notice,
                children: "Note: “Safe stdlib imports only” filtering is not shown in preview — save the rule to test it."
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Frules%2FRuleBuilderForm.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA91Y23LbIBB9z1fwmMwUV1LqXJSX_kkGi5VMgkABZMvu5N87Qk2Crsi2Ztrpg6-s9uwe9gar53D3ph71S4B-XSG0IclrpmQpaIx2RF1j_Bw-wjFSx_ubp3pdKgoqRmFRIS05ox0pcKSwIpSVuqvo9sHKFIRSJrLeamhXKdMFJ4cYpRyq-o_6E1OmIDFMihglkpe5qFcyUgwqeb9affgWWt9SKQzW7Ag98cRi2vU9sGxrehJbK5FILlV3LbBLOVEZE3gjjZF5VyQ6tu2JrD09H60nwVI0Nxsmd6BSLvcx2jJKwTJGOMsE1sDTBhtrQ5RpWXhrLRzZo-hw0wULvhgcZHjTih4hBTx1os0oInRBFAgzSnUTG0mpdL1WSCYMKN_WZfYhq541sfMFi4JVqL81YPZ7m4R4W9PnSYyH9t7-8Ijn45G07miaBS_aD62HQ-u09Anc8DKyGA_E1M3lRnRAl2PfnScVycxUNFAZbDc1lSqPUVkUoBKibVxxMAYU1gVJbPgGq-AO8lHeoxkZfGi7cd-mOVOMWjIVo9hAXnBiADck6xiFqapfU3Q7uh-sbquq0RCjEH1HOGxJPS6w0dYrf9r6s2s0Wx2DyWklxdZMfyV8nV0Jx1MoJBNObOZTlJKc8UPPtbLDQ5zKpNRNWjd2DyLzpn6XhjMBMYpGCMhdMSzTVIOxdLVAN_8z-b2G4Dr-N9hOJvr7INE2a_eqXq_fW8poUzE5VDEKbaFiAu8ZNdsYhUFQVBPTVHQc6dTLDRjn9fAFSk5_CnB7POG839DpOQ0dvLFTjkdw6Yn-XRsr9WKFwUS6BL5c65TkzAtXjaNVHrBOx9wO50QzizIDuY5RAh9bOZEqvmJwwuDomMdmzSX-8WFgBOrODi_nQ83h3Je2d1PHr9FWMN-9V497U9sUzQngeXXptIro2M9PLd4T02_34JdPtuG-aaGHD89hYuDENYO5S2rz1pto-TmnmjymTJMNh6YZynqwN4d6sF-7eEIaTDiXe6Ctx8XFtM_vbMt11kvGHnFOp5OX1kAXqR1xX78H-vbnxgSfJzsKiVSkaeWloKDqeWvm3YV0fB-9sXHki6VOUo7Ot3-72ymn8fsO9xcU21vfQJK0WdMLXQ6cfco_nnW3sK7vFhw3zIKXA386yM8cKCPoOifVx-x_V8_-Nxaqcy0xBVXDvF-9_wbFCiU29xUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Frules_2FRuleBuilderForm_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA91Y23LbIBB9z1fwmMwUV1LqXJSX_kkGi5VMgkABZMvu5N87Qk2Crsi2Ztrpg6_s9uwe9gar53D3ph71S4B_XSG0IclrpmQpaIx2RF1j_Bw_wjFSx_ubp3pdKgoqRmFRIS05ox0pcKSwIpSVuqvo9sHKFIRSJrLeamhXKdMFJ4cYpRyq_o_6E1OmIDFMihglkpe5qFcyUgwqeb9affgWWt9SKQzW7Ag98cRi2vU9sGxrehJbK5FILlV3LbBLOVEZE3gjjZF5VyQ6tu2JrD09H60nwVI0Nxsmd6BSLvcx2jJKwTJGOMsE1sDTBhtrQ5RpWXhrLRzZo_hw0wULvhgcZHjTih4hBTx1os0oInRBFAgzSnUTG0mpdL1WSCYMKN_WZfYhq541sfMFi4JVqL81YPZ7m4R4W9PnSYyH9t7_8Ijn45G07miaBS_aD62HQ_u09Anc8DKyGA_E1M3lRnRAl2PfnScVycxUNFAZbDc1lSqPUVkUoBKibVxxMAYU1gVJbPgGq_AO8lHeoxkZfGi7cd_mOVOMWjIVo9hAXnBiADck6xiFqapfU3Q7uh_sbquq0RCjEH1HOGxJPS6w0dYrf9r6s2s0Wx2DyWklxdZMfyV8nV0Jx1MoJBNObOZTlJKc8UPPtbLDQ5zKpNRNWjd2DyLzpn6XhjMBMYpGCMhdMSzTVIOxdLVAN_8z_b2G4Dr_N9hOJvr7INE2a_eqXq_fW8poUzE5VDEKbaFiAu8ZNdsYhUFQVBPTVHQc6dTLDRjn9fAFSk5_CnB7POG839DpOQ0dvLFTjkdw6Yn_XRsr9WKFwUS6BL5c65TkzAtXjaNVHrBOx9wO50QzizIDuY5RAh9bOZEqvmJwwuDomMdmzSX_8WFgBOrODi_nQ83h3Je2d1PHr9FWMN_9V497U9sUzQngeXXptIro2M9PLd4T02_34JdPtuG_aaGHD89hYuDENYO5S2rz1pto_TmnmjymTJMNh6YZynqwN4d6sF_7eEIaTDiXe6Ctx8XFtM_vbMt11kvGHnFOp5OX1kAXqR1xX78H_vbnxgSfJzsKiVSkaeWloKDqeWvm3YV0fB_9sXHki6VOUo7Ot3_72ymn8fsO9xcU21vfQJK0WdMLXQ6cfco_nnW3sK7vFhw3zIKXA386yM8cKCPoOifVx_x_V8_Nxaqcy0xBVXDvF_9_wbFCiU29xUAAA_22_7D = __webpack_require__(62382);
;// ./src/components/rules/RuleBuilderForm.css.ts

var actions = '_1vqr9sjl';
var advancedToggle = '_1vqr9sjo';
var cancelBtn = '_1vqr9sjn';
var checkboxGrid = '_1vqr9sjt';
var checkboxRow = '_1vqr9sjh';
var decisionAllow = '_1vqr9sje';
var decisionBtn = '_1vqr9sjd';
var decisionDeny = '_1vqr9sjf';
var decisionEscalate = '_1vqr9sjg';
var decisionSegment = '_1vqr9sjc';
var errorBanner = '_1vqr9sjk';
var fieldInput = '_1vqr9sja';
var fieldLabel = '_1vqr9sj9';
var fieldSelect = '_1vqr9sjb';
var formGrid = '_1vqr9sj7';
var formGridFull = '_1vqr9sj8';
var formTitle = '_1vqr9sj1';
var formWrapper = '_1vqr9sj0';
var modeBtn = '_1vqr9sj3';
var modeBtnActive = '_1vqr9sj4';
var modeToggle = '_1vqr9sj2';
var priorityHint = '_1vqr9sji';
var priorityWarning = '_1vqr9sjj';
var pythonSection = '_1vqr9sjr';
var pythonSectionTitle = '_1vqr9sjs';
var radioGroup = '_1vqr9sjp';
var radioRow = '_1vqr9sjq';
var saveBtn = '_1vqr9sjm';
var section = '_1vqr9sj5';
var sectionTitle = '_1vqr9sj6';
;// ./src/components/rules/RuleBuilderForm.tsx
// +feature: approval-rule-min-session-idle-minutes
/* __next_internal_client_entry_do_not_use__ RuleBuilderForm auto */ 





const PYTHON_PROGRAMS = new Set([
    "python",
    "python2",
    "python3",
    "pypy",
    "pypy3"
]);
function isPythonProgram(p) {
    return PYTHON_PROGRAMS.has(p.toLowerCase()) || p.toLowerCase().startsWith("python");
}
const PYTHON_MODES = [
    {
        value: "script",
        label: "Script (.py file)"
    },
    {
        value: "module",
        label: "Module (-m)"
    },
    {
        value: "inline",
        label: "Inline (-c)"
    },
    {
        value: "version",
        label: "Version (-V)"
    }
];
const TOOL_CATEGORIES = [
    {
        value: "",
        label: "— select category —"
    },
    {
        value: "builtin",
        label: "Built-in tools (Read, Edit, Bash…)"
    },
    {
        value: "builtin-agent",
        label: "Agent/planning tools"
    },
    {
        value: "mcp-read",
        label: "MCP read-only tools"
    },
    {
        value: "mcp-write",
        label: "MCP write tools"
    },
    {
        value: "mcp",
        label: "Any MCP tool"
    }
];
function defaultPriority(decision) {
    switch(decision){
        case types_pb/* AutoDecision */.Uz.DENY:
            return 950;
        case types_pb/* AutoDecision */.Uz.ALLOW:
            return 100;
        default:
            return 450;
    }
}
function decisionPrefix(decision) {
    switch(decision){
        case types_pb/* AutoDecision */.Uz.ALLOW:
            return "Allow";
        case types_pb/* AutoDecision */.Uz.DENY:
            return "Block";
        default:
            return "Escalate";
    }
}
function computeSuggestedName(toolTarget, toolName, toolCategory, toolPattern, programs, decision) {
    const prefix = decisionPrefix(decision);
    if (toolTarget === "name" && toolName.trim()) return "".concat(prefix, " ").concat(toolName.trim());
    if (toolTarget === "category" && toolCategory) {
        const cat = TOOL_CATEGORIES.find((c)=>c.value === toolCategory);
        return cat ? "".concat(prefix, " ").concat(cat.label) : "";
    }
    if (toolTarget === "pattern" && toolPattern.trim()) return "".concat(prefix, " ").concat(toolPattern.trim());
    if (programs.length > 0) return "".concat(prefix, " ").concat(programs[0]);
    return "";
}
function RuleBuilderForm(param) {
    let { editRule, prefill, templateSeed, onSave, onSaveToConfig, onCancel, onCmdGenerate, cmdSuggestions = [], cmdLoading = false, cmdClear, subcommandStats } = param;
    var _prefill_programs, _prefill_subcommands;
    const [mode, setMode] = (0,react.useState)("structured");
    const [pendingMode, setPendingMode] = (0,react.useState)(null);
    const [toolTarget, setToolTarget] = (0,react.useState)("name");
    const [name, setName] = (0,react.useState)("");
    const [toolName, setToolName] = (0,react.useState)("");
    const [toolCategory, setToolCategory] = (0,react.useState)("");
    const [toolPattern, setToolPattern] = (0,react.useState)("");
    const [programs, setPrograms] = (0,react.useState)([]);
    const [subcommands, setSubcommands] = (0,react.useState)([]);
    const [blockedSubcommands, setBlockedSubcommands] = (0,react.useState)([]);
    const [requiredFlags, setRequiredFlags] = (0,react.useState)([]);
    const [forbiddenFlags, setForbiddenFlags] = (0,react.useState)([]);
    const [requiredFlagPrefixes, setRequiredFlagPrefixes] = (0,react.useState)([]);
    const [pythonModes, setPythonModes] = (0,react.useState)([]);
    const [safePythonImportsOnly, setSafePythonImportsOnly] = (0,react.useState)(false);
    const [requireCiPassing, setRequireCiPassing] = (0,react.useState)(false);
    const [minSessionIdleMinutes, setMinSessionIdleMinutes] = (0,react.useState)(0);
    const [commandPattern, setCommandPattern] = (0,react.useState)("");
    const [filePattern, setFilePattern] = (0,react.useState)("");
    const [decision, setDecision] = (0,react.useState)(types_pb/* AutoDecision */.Uz.ALLOW);
    const [riskLevel, setRiskLevel] = (0,react.useState)("medium");
    const [reason, setReason] = (0,react.useState)("");
    const [alternative, setAlternative] = (0,react.useState)("");
    const [priority, setPriority] = (0,react.useState)(10);
    const [enabled, setEnabled] = (0,react.useState)(true);
    const [showAdvanced, setShowAdvanced] = (0,react.useState)(false);
    const [saving, setSaving] = (0,react.useState)(false);
    const [formError, setFormError] = (0,react.useState)(null);
    const [commandSampleText, setCommandSampleText] = (0,react.useState)("");
    const [destination, setDestination] = (0,react.useState)("db");
    // Track whether commandPattern was set by AI (to show ai-generated-badge)
    const [commandPatternIsAi, setCommandPatternIsAi] = (0,react.useState)(false);
    // Track whether user has manually edited commandPattern (to prevent AI overwrite)
    const commandPatternEditedByUser = (0,react.useRef)(false);
    const showPythonSection = programs.some(isPythonProgram);
    const inlineChecked = pythonModes.includes("inline");
    // Auto-suggest name from criteria when name is empty or still matches the last suggestion.
    // Uses a ref so the effect doesn't re-run on every name keystroke.
    const lastAutoName = (0,react.useRef)("");
    const nameRef = (0,react.useRef)(name);
    (0,react.useLayoutEffect)(()=>{
        nameRef.current = name;
    });
    (0,react.useEffect)(()=>{
        if (editRule) return;
        const suggested = computeSuggestedName(toolTarget, toolName, toolCategory, toolPattern, programs, decision);
        if (!suggested) return;
        if (nameRef.current === "" || nameRef.current === lastAutoName.current) {
            setName(suggested);
            lastAutoName.current = suggested;
        }
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        editRule,
        toolTarget,
        toolName,
        toolCategory,
        toolPattern,
        programs,
        decision
    ]);
    // Seed from template
    (0,react.useEffect)(()=>{
        if (!templateSeed) return;
        lastAutoName.current = "";
        setName("");
        if (templateSeed.toolCategory) {
            setToolTarget("category");
            setToolCategory(templateSeed.toolCategory);
        } else if (templateSeed.toolName) {
            setToolTarget("name");
            setToolName(templateSeed.toolName);
        } else if (templateSeed.commandPattern) {
            setMode("regex");
            setCommandPattern(templateSeed.commandPattern);
        } else setToolTarget("name");
        var _templateSeed_programs;
        setPrograms((_templateSeed_programs = templateSeed.programs) !== null && _templateSeed_programs !== void 0 ? _templateSeed_programs : []);
        var _templateSeed_subcommands;
        setSubcommands((_templateSeed_subcommands = templateSeed.subcommands) !== null && _templateSeed_subcommands !== void 0 ? _templateSeed_subcommands : []);
        var _templateSeed_blockedSubcommands;
        setBlockedSubcommands((_templateSeed_blockedSubcommands = templateSeed.blockedSubcommands) !== null && _templateSeed_blockedSubcommands !== void 0 ? _templateSeed_blockedSubcommands : []);
        var _templateSeed_requiredFlags;
        setRequiredFlags((_templateSeed_requiredFlags = templateSeed.requiredFlags) !== null && _templateSeed_requiredFlags !== void 0 ? _templateSeed_requiredFlags : []);
        var _templateSeed_forbiddenFlags;
        setForbiddenFlags((_templateSeed_forbiddenFlags = templateSeed.forbiddenFlags) !== null && _templateSeed_forbiddenFlags !== void 0 ? _templateSeed_forbiddenFlags : []);
        var _templateSeed_pythonModes;
        setPythonModes((_templateSeed_pythonModes = templateSeed.pythonModes) !== null && _templateSeed_pythonModes !== void 0 ? _templateSeed_pythonModes : []);
        var _templateSeed_safePythonImportsOnly;
        setSafePythonImportsOnly((_templateSeed_safePythonImportsOnly = templateSeed.safePythonImportsOnly) !== null && _templateSeed_safePythonImportsOnly !== void 0 ? _templateSeed_safePythonImportsOnly : false);
        var _templateSeed_requireCiPassing;
        setRequireCiPassing((_templateSeed_requireCiPassing = templateSeed.requireCiPassing) !== null && _templateSeed_requireCiPassing !== void 0 ? _templateSeed_requireCiPassing : false);
        setDecision(templateSeed.decision);
        setRiskLevel(templateSeed.riskLevel);
        setPriority(templateSeed.priority);
        var _templateSeed_reason;
        setReason((_templateSeed_reason = templateSeed.reason) !== null && _templateSeed_reason !== void 0 ? _templateSeed_reason : "");
        var _templateSeed_alternative;
        setAlternative((_templateSeed_alternative = templateSeed.alternative) !== null && _templateSeed_alternative !== void 0 ? _templateSeed_alternative : "");
        setEnabled(true);
    }, [
        templateSeed
    ]);
    // Seed from URL prefill (analytics deep-link)
    (0,react.useEffect)(()=>{
        var _prefill_programs, _prefill_subcommands;
        if (!prefill) return;
        if ((_prefill_programs = prefill.programs) === null || _prefill_programs === void 0 ? void 0 : _prefill_programs.length) setPrograms(prefill.programs);
        if ((_prefill_subcommands = prefill.subcommands) === null || _prefill_subcommands === void 0 ? void 0 : _prefill_subcommands.length) setSubcommands(prefill.subcommands);
        if (prefill.toolName) {
            setToolTarget("name");
            setToolName(prefill.toolName);
        }
        if (prefill.toolCategory) {
            setToolTarget("category");
            setToolCategory(prefill.toolCategory);
        }
        if (prefill.suggestedDecision !== undefined) {
            setDecision(prefill.suggestedDecision);
            setPriority(defaultPriority(prefill.suggestedDecision));
        }
        if (prefill.commandPattern && !commandPatternEditedByUser.current) {
            setCommandPattern(prefill.commandPattern);
            if (prefill.isAiGenerated) setCommandPatternIsAi(true);
        }
        // If an explicit name is provided, use it. Don't set lastAutoName to this value
        // — instead clear it so the auto-name check `nameRef.current === ""` is false
        // and auto-name won't overwrite the explicitly set name.
        if (prefill.initialName) {
            setName(prefill.initialName);
            lastAutoName.current = "__explicit_prefill__";
        }
    }, [
        prefill
    ]);
    // Pre-fill commandPattern from cmd AI suggestions (command-sample generation)
    (0,react.useEffect)(()=>{
        if (!cmdSuggestions || cmdSuggestions.length === 0) return;
        const suggestion = cmdSuggestions[0];
        if (suggestion.commandPattern && !commandPatternEditedByUser.current) {
            setCommandPattern(suggestion.commandPattern);
            setCommandPatternIsAi(true);
        }
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        cmdSuggestions
    ]);
    // Seed from edit rule
    (0,react.useEffect)(()=>{
        var _editRule_programs, _editRule_subcommands, _editRule_pythonModes;
        if (!editRule) return;
        var _editRule_programs_length, _editRule_subcommands_length, _editRule_pythonModes_length;
        const hasCriteria = ((_editRule_programs_length = (_editRule_programs = editRule.programs) === null || _editRule_programs === void 0 ? void 0 : _editRule_programs.length) !== null && _editRule_programs_length !== void 0 ? _editRule_programs_length : 0) > 0 || ((_editRule_subcommands_length = (_editRule_subcommands = editRule.subcommands) === null || _editRule_subcommands === void 0 ? void 0 : _editRule_subcommands.length) !== null && _editRule_subcommands_length !== void 0 ? _editRule_subcommands_length : 0) > 0 || ((_editRule_pythonModes_length = (_editRule_pythonModes = editRule.pythonModes) === null || _editRule_pythonModes === void 0 ? void 0 : _editRule_pythonModes.length) !== null && _editRule_pythonModes_length !== void 0 ? _editRule_pythonModes_length : 0) > 0;
        setMode(hasCriteria ? "structured" : editRule.commandPattern ? "regex" : "structured");
        setName(editRule.name);
        var _editRule_toolName;
        setToolName((_editRule_toolName = editRule.toolName) !== null && _editRule_toolName !== void 0 ? _editRule_toolName : "");
        var _editRule_toolCategory;
        setToolCategory((_editRule_toolCategory = editRule.toolCategory) !== null && _editRule_toolCategory !== void 0 ? _editRule_toolCategory : "");
        var _editRule_toolPattern;
        setToolPattern((_editRule_toolPattern = editRule.toolPattern) !== null && _editRule_toolPattern !== void 0 ? _editRule_toolPattern : "");
        if (editRule.toolCategory) setToolTarget("category");
        else if (editRule.toolPattern) setToolTarget("pattern");
        else setToolTarget("name");
        var _editRule_programs1;
        setPrograms((_editRule_programs1 = editRule.programs) !== null && _editRule_programs1 !== void 0 ? _editRule_programs1 : []);
        var _editRule_subcommands1;
        setSubcommands((_editRule_subcommands1 = editRule.subcommands) !== null && _editRule_subcommands1 !== void 0 ? _editRule_subcommands1 : []);
        var _editRule_blockedSubcommands;
        setBlockedSubcommands((_editRule_blockedSubcommands = editRule.blockedSubcommands) !== null && _editRule_blockedSubcommands !== void 0 ? _editRule_blockedSubcommands : []);
        var _editRule_requiredFlags;
        setRequiredFlags((_editRule_requiredFlags = editRule.requiredFlags) !== null && _editRule_requiredFlags !== void 0 ? _editRule_requiredFlags : []);
        var _editRule_forbiddenFlags;
        setForbiddenFlags((_editRule_forbiddenFlags = editRule.forbiddenFlags) !== null && _editRule_forbiddenFlags !== void 0 ? _editRule_forbiddenFlags : []);
        var _editRule_requiredFlagPrefixes;
        setRequiredFlagPrefixes((_editRule_requiredFlagPrefixes = editRule.requiredFlagPrefixes) !== null && _editRule_requiredFlagPrefixes !== void 0 ? _editRule_requiredFlagPrefixes : []);
        var _editRule_pythonModes1;
        setPythonModes((_editRule_pythonModes1 = editRule.pythonModes) !== null && _editRule_pythonModes1 !== void 0 ? _editRule_pythonModes1 : []);
        var _editRule_safePythonImportsOnly;
        setSafePythonImportsOnly((_editRule_safePythonImportsOnly = editRule.safePythonImportsOnly) !== null && _editRule_safePythonImportsOnly !== void 0 ? _editRule_safePythonImportsOnly : false);
        var _editRule_requireCiPassing;
        setRequireCiPassing((_editRule_requireCiPassing = editRule.requireCiPassing) !== null && _editRule_requireCiPassing !== void 0 ? _editRule_requireCiPassing : false);
        var _editRule_minSessionIdleMinutes;
        setMinSessionIdleMinutes((_editRule_minSessionIdleMinutes = editRule.minSessionIdleMinutes) !== null && _editRule_minSessionIdleMinutes !== void 0 ? _editRule_minSessionIdleMinutes : 0);
        var _editRule_commandPattern;
        setCommandPattern((_editRule_commandPattern = editRule.commandPattern) !== null && _editRule_commandPattern !== void 0 ? _editRule_commandPattern : "");
        var _editRule_filePattern;
        setFilePattern((_editRule_filePattern = editRule.filePattern) !== null && _editRule_filePattern !== void 0 ? _editRule_filePattern : "");
        var _editRule_decision;
        setDecision((_editRule_decision = editRule.decision) !== null && _editRule_decision !== void 0 ? _editRule_decision : types_pb/* AutoDecision */.Uz.ESCALATE);
        var _editRule_riskLevel;
        setRiskLevel((_editRule_riskLevel = editRule.riskLevel) !== null && _editRule_riskLevel !== void 0 ? _editRule_riskLevel : "medium");
        var _editRule_reason;
        setReason((_editRule_reason = editRule.reason) !== null && _editRule_reason !== void 0 ? _editRule_reason : "");
        var _editRule_alternative;
        setAlternative((_editRule_alternative = editRule.alternative) !== null && _editRule_alternative !== void 0 ? _editRule_alternative : "");
        var _editRule_priority;
        setPriority((_editRule_priority = editRule.priority) !== null && _editRule_priority !== void 0 ? _editRule_priority : 10);
        var _editRule_enabled;
        setEnabled((_editRule_enabled = editRule.enabled) !== null && _editRule_enabled !== void 0 ? _editRule_enabled : true);
    }, [
        editRule
    ]);
    function handleDecisionChange(d) {
        if (d === decision) return;
        // Only auto-default priority when the value equals the previous decision's default,
        // so manually-edited priorities are not overwritten.
        if (priority === defaultPriority(decision)) {
            setPriority(defaultPriority(d));
        }
        setDecision(d);
    }
    function togglePythonMode(m) {
        setPythonModes((prev)=>prev.includes(m) ? prev.filter((x)=>x !== m) : [
                ...prev,
                m
            ]);
    }
    function handleModeSwitch(next) {
        if (next === mode) return;
        const willLoseData = next === "regex" && (programs.length > 0 || subcommands.length > 0) || next === "structured" && !!commandPattern;
        if (willLoseData) {
            setPendingMode(next);
            return;
        }
        applyModeSwitch(next);
    }
    function applyModeSwitch(next) {
        if (next === "regex") {
            setPrograms([]);
            setSubcommands([]);
            setBlockedSubcommands([]);
            setRequiredFlags([]);
            setForbiddenFlags([]);
            setRequiredFlagPrefixes([]);
            setPythonModes([]);
            setSafePythonImportsOnly(false);
        }
        if (next === "structured") {
            setCommandPattern("");
        }
        setMode(next);
        setPendingMode(null);
    }
    async function handleSave() {
        if (!name.trim()) {
            setFormError("Name is required.");
            return;
        }
        const hasToolTarget = toolName || toolCategory || toolPattern;
        const hasCriteria = programs.length > 0 || subcommands.length > 0 || requiredFlags.length > 0 || forbiddenFlags.length > 0 || pythonModes.length > 0 || blockedSubcommands.length > 0;
        const hasPattern = commandPattern || filePattern;
        if (!hasToolTarget && !hasCriteria && !hasPattern) {
            setFormError("At least one match criterion is required (tool, program, or pattern).");
            return;
        }
        setFormError(null);
        setSaving(true);
        try {
            const rulePayload = {
                name: name.trim(),
                toolName: toolTarget === "name" ? toolName : "",
                toolCategory: toolTarget === "category" ? toolCategory : "",
                toolPattern: toolTarget === "pattern" ? toolPattern : "",
                commandPattern: mode === "regex" ? commandPattern : "",
                filePattern,
                decision,
                riskLevel,
                reason,
                alternative,
                priority,
                enabled,
                programs: mode === "structured" ? programs : [],
                subcommands: mode === "structured" ? subcommands : [],
                blockedSubcommands: mode === "structured" ? blockedSubcommands : [],
                requiredFlags: mode === "structured" ? requiredFlags : [],
                forbiddenFlags: mode === "structured" ? forbiddenFlags : [],
                requiredFlagPrefixes: mode === "structured" ? requiredFlagPrefixes : [],
                pythonModes: mode === "structured" ? pythonModes : [],
                safePythonImportsOnly: mode === "structured" ? safePythonImportsOnly : false,
                requireCiPassing,
                minSessionIdleMinutes
            };
            if (destination === "config" && onSaveToConfig && !editRule) {
                await onSaveToConfig(rulePayload);
            } else {
                var _editRule_id;
                const id = (_editRule_id = editRule === null || editRule === void 0 ? void 0 : editRule.id) !== null && _editRule_id !== void 0 ? _editRule_id : "user-".concat(Date.now());
                await onSave({
                    id,
                    ...rulePayload
                });
            }
        } catch (e) {
            setFormError(e instanceof Error ? e.message : "Failed to save rule.");
        } finally{
            setSaving(false);
        }
    }
    const previewCriteria = {
        programs,
        subcommands,
        blockedSubcommands,
        requiredFlags,
        forbiddenFlags,
        requiredFlagPrefixes,
        pythonModes
    };
    var _prefill_programs_length, _prefill_subcommands_length;
    const isPrefilled = !editRule && (((_prefill_programs_length = prefill === null || prefill === void 0 ? void 0 : (_prefill_programs = prefill.programs) === null || _prefill_programs === void 0 ? void 0 : _prefill_programs.length) !== null && _prefill_programs_length !== void 0 ? _prefill_programs_length : 0) > 0 || ((_prefill_subcommands_length = prefill === null || prefill === void 0 ? void 0 : (_prefill_subcommands = prefill.subcommands) === null || _prefill_subcommands === void 0 ? void 0 : _prefill_subcommands.length) !== null && _prefill_subcommands_length !== void 0 ? _prefill_subcommands_length : 0) > 0 || !!(prefill === null || prefill === void 0 ? void 0 : prefill.toolName) || !!(prefill === null || prefill === void 0 ? void 0 : prefill.toolCategory));
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: formWrapper,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                className: formTitle,
                children: editRule ? "Editing: ".concat(editRule.name) : "New Rule"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: modeToggle,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: "".concat(modeBtn, " ").concat(mode === "structured" ? modeBtnActive : ""),
                        onClick: ()=>handleModeSwitch("structured"),
                        children: "Structured"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: "".concat(modeBtn, " ").concat(mode === "regex" ? modeBtnActive : ""),
                        onClick: ()=>handleModeSwitch("regex"),
                        children: "Regex / Advanced"
                    })
                ]
            }),
            pendingMode && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: errorBanner,
                role: "alert",
                style: {
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    gap: 8
                },
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        children: pendingMode === "regex" ? "Switch to Regex? Structured criteria will be cleared." : "Switch to Structured? The regex pattern will be cleared."
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        style: {
                            display: "flex",
                            gap: 6,
                            flexShrink: 0
                        },
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: saveBtn,
                                style: {
                                    padding: "2px 10px"
                                },
                                onClick: ()=>applyModeSwitch(pendingMode),
                                children: "Confirm"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: cancelBtn,
                                style: {
                                    padding: "2px 10px"
                                },
                                onClick: ()=>setPendingMode(null),
                                children: "Keep"
                            })
                        ]
                    })
                ]
            }),
            formError && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: errorBanner,
                children: formError
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: section,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: sectionTitle,
                        children: "Tool"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: radioGroup,
                        children: [
                            "name",
                            "category",
                            "pattern"
                        ].map((t)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: radioRow,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        type: "radio",
                                        name: "toolTarget",
                                        value: t,
                                        checked: toolTarget === t,
                                        onChange: ()=>setToolTarget(t)
                                    }),
                                    t === "name" ? "Exact tool name" : t === "category" ? "Tool category" : "Tool pattern (regex)"
                                ]
                            }, t))
                    }),
                    toolTarget === "name" && /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        className: fieldInput,
                        "data-testid": "form-tool-name-input",
                        value: toolName,
                        onChange: (e)=>setToolName(e.target.value),
                        placeholder: "e.g. Bash, Read, Edit"
                    }),
                    toolTarget === "category" && /*#__PURE__*/ (0,jsx_runtime.jsx)("select", {
                        className: fieldSelect,
                        value: toolCategory,
                        onChange: (e)=>setToolCategory(e.target.value),
                        children: TOOL_CATEGORIES.map((c)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                value: c.value,
                                children: c.label
                            }, c.value))
                    }),
                    toolTarget === "pattern" && /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        className: fieldInput,
                        value: toolPattern,
                        onChange: (e)=>setToolPattern(e.target.value),
                        placeholder: "e.g. Read|Glob|Grep"
                    })
                ]
            }),
            mode === "structured" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: section,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: sectionTitle,
                        children: "Command Criteria"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: formGrid,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: fieldLabel,
                                children: [
                                    "Programs",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(TagInput, {
                                        value: programs,
                                        onChange: setPrograms,
                                        placeholder: "e.g. git, python3",
                                        isPrefilled: isPrefilled
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        type: "hidden",
                                        "data-testid": "form-criteria-programs-input",
                                        value: programs.join(","),
                                        readOnly: true
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: fieldLabel,
                                children: [
                                    "Subcommands (allow-list)",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(TagInput, {
                                        value: subcommands,
                                        onChange: setSubcommands,
                                        placeholder: "e.g. push, pull",
                                        isPrefilled: isPrefilled
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        type: "hidden",
                                        "data-testid": "form-criteria-subcommands-input",
                                        value: subcommands.join(","),
                                        readOnly: true
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: fieldLabel,
                                children: [
                                    "Blocked subcommands",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(TagInput, {
                                        value: blockedSubcommands,
                                        onChange: setBlockedSubcommands,
                                        placeholder: "e.g. reset, rebase"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: fieldLabel,
                                children: [
                                    "Required flags",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(TagInput, {
                                        value: requiredFlags,
                                        onChange: setRequiredFlags,
                                        placeholder: "e.g. --hard, -f"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: fieldLabel,
                                children: [
                                    "Forbidden flags",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(TagInput, {
                                        value: forbiddenFlags,
                                        onChange: setForbiddenFlags,
                                        placeholder: "e.g. --force, --no-verify"
                                    })
                                ]
                            })
                        ]
                    }),
                    showAdvanced && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: formGrid,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                            className: fieldLabel,
                            children: [
                                "Required flag prefixes",
                                /*#__PURE__*/ (0,jsx_runtime.jsx)(TagInput, {
                                    value: requiredFlagPrefixes,
                                    onChange: setRequiredFlagPrefixes,
                                    placeholder: "e.g. -i (matches -i, -i.bak)"
                                })
                            ]
                        })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: advancedToggle,
                        type: "button",
                        onClick: ()=>setShowAdvanced((v)=>!v),
                        children: showAdvanced ? "▲ Hide advanced" : "▼ Advanced options"
                    }),
                    showPythonSection && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: pythonSection,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                className: pythonSectionTitle,
                                children: "Python invocation mode"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: checkboxGrid,
                                children: PYTHON_MODES.map((m)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                        className: checkboxRow,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                type: "checkbox",
                                                checked: pythonModes.includes(m.value),
                                                onChange: ()=>togglePythonMode(m.value)
                                            }),
                                            m.label
                                        ]
                                    }, m.value))
                            }),
                            inlineChecked && /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: checkboxRow,
                                style: {
                                    marginTop: "8px"
                                },
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        type: "checkbox",
                                        checked: safePythonImportsOnly,
                                        onChange: (e)=>setSafePythonImportsOnly(e.target.checked)
                                    }),
                                    "Require safe stdlib imports only (json, os, sys, re…)"
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(RulePreview, {
                        criteria: previewCriteria,
                        showSafePythonNotice: safePythonImportsOnly,
                        subcommandStats: subcommandStats,
                        onAddSubcommand: (sub)=>setSubcommands((prev)=>prev.includes(sub) ? prev : [
                                    ...prev,
                                    sub
                                ])
                    })
                ]
            }),
            mode === "regex" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: section,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: sectionTitle,
                        children: "Regex patterns"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: formGrid,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                            className: fieldLabel,
                            children: [
                                "File pattern",
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                    className: fieldInput,
                                    value: filePattern,
                                    onChange: (e)=>setFilePattern(e.target.value),
                                    placeholder: "e.g. \\.env$"
                                })
                            ]
                        })
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                "data-testid": "advanced-regex-separator",
                style: {
                    borderTop: "1px solid var(--border-color)",
                    margin: "8px 0"
                }
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: section,
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                    className: fieldLabel,
                    children: [
                        "Command pattern (regex)",
                        commandPatternIsAi && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            "data-testid": "ai-generated-badge",
                            style: {
                                marginLeft: "8px",
                                fontSize: "0.75em",
                                color: "var(--primary)"
                            },
                            children: "✦ AI"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                            className: fieldInput,
                            "data-testid": "form-command-pattern-input",
                            value: commandPattern,
                            onChange: (e)=>{
                                commandPatternEditedByUser.current = true;
                                setCommandPatternIsAi(false);
                                setCommandPattern(e.target.value);
                            },
                            placeholder: "e.g. ^git push"
                        })
                    ]
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: section,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: sectionTitle,
                        children: "Decision"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: decisionSegment,
                        children: [
                            [
                                types_pb/* AutoDecision */.Uz.ALLOW,
                                "Auto-Allow",
                                decisionAllow
                            ],
                            [
                                types_pb/* AutoDecision */.Uz.DENY,
                                "Auto-Deny",
                                decisionDeny
                            ],
                            [
                                types_pb/* AutoDecision */.Uz.ESCALATE,
                                "Escalate",
                                decisionEscalate
                            ]
                        ].map((param)=>{
                            let [d, label, cls] = param;
                            return /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: "".concat(decisionBtn, " ").concat(decision === d ? cls : ""),
                                onClick: ()=>handleDecisionChange(d),
                                children: label
                            }, d);
                        })
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: section,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: sectionTitle,
                        children: "Details"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: formGrid,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: "".concat(fieldLabel, " ").concat(formGridFull),
                                children: [
                                    "Name *",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        className: fieldInput,
                                        "data-testid": "form-name-input",
                                        value: name,
                                        onChange: (e)=>setName(e.target.value),
                                        placeholder: "e.g. Allow git log"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: fieldLabel,
                                children: [
                                    "Risk level",
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                                        className: fieldSelect,
                                        value: riskLevel,
                                        onChange: (e)=>setRiskLevel(e.target.value),
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                value: "low",
                                                children: "Low"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                value: "medium",
                                                children: "Medium"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                value: "high",
                                                children: "High"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                value: "critical",
                                                children: "Critical"
                                            })
                                        ]
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: fieldLabel,
                                children: [
                                    "Priority",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        className: fieldInput,
                                        type: "number",
                                        min: 1,
                                        max: 9999,
                                        value: priority,
                                        onChange: (e)=>setPriority(Number(e.target.value))
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: priorityHint,
                                        children: "1000+ deny first \xb7 500+ escalate before allow \xb7 100–499 allow tier \xb7 1–99 after built-ins"
                                    }),
                                    priority < 100 && decision === types_pb/* AutoDecision */.Uz.DENY && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: priorityWarning,
                                        children: "Warning: priority below 100 means seed allow rules (at 100) will fire first."
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: "".concat(fieldLabel, " ").concat(formGridFull),
                                children: [
                                    "Reason (shown to Claude when denied/escalated)",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        className: fieldInput,
                                        value: reason,
                                        onChange: (e)=>setReason(e.target.value),
                                        placeholder: "e.g. This operation modifies remote state."
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: "".concat(fieldLabel, " ").concat(formGridFull),
                                children: [
                                    "Alternative suggestion",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        className: fieldInput,
                                        value: alternative,
                                        onChange: (e)=>setAlternative(e.target.value),
                                        placeholder: "e.g. Use the Edit tool instead."
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: checkboxRow,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        type: "checkbox",
                                        checked: enabled,
                                        onChange: (e)=>setEnabled(e.target.checked)
                                    }),
                                    "Rule enabled"
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: checkboxRow,
                                "data-testid": "require-ci-passing-checkbox",
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        type: "checkbox",
                                        checked: requireCiPassing,
                                        onChange: (e)=>setRequireCiPassing(e.target.checked)
                                    }),
                                    "Require CI passing on this branch"
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: fieldLabel,
                                htmlFor: "rule-min-session-idle-minutes-input",
                                children: [
                                    "Min. session idle minutes",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        id: "rule-min-session-idle-minutes-input",
                                        className: fieldInput,
                                        "data-testid": "min-session-idle-minutes-input",
                                        type: "number",
                                        min: 0,
                                        value: minSessionIdleMinutes,
                                        onChange: (e)=>setMinSessionIdleMinutes(Math.max(0, Number(e.target.value) || 0))
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: priorityHint,
                                        children: "0 = not applied. Otherwise requires the session to have been idle at least this many minutes to match; sessions with unknown idle time never match a nonzero value here (fails closed)."
                                    })
                                ]
                            })
                        ]
                    })
                ]
            }),
            onCmdGenerate && /*#__PURE__*/ (0,jsx_runtime.jsxs)("details", {
                "data-testid": "generate-from-command-details",
                style: {
                    marginBottom: "12px"
                },
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("summary", {
                        style: {
                            cursor: "pointer",
                            fontWeight: 600
                        },
                        children: "Generate pattern from command sample"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        style: {
                            paddingTop: "8px"
                        },
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("textarea", {
                                "data-testid": "command-sample-textarea",
                                value: commandSampleText,
                                onChange: (e)=>setCommandSampleText(e.target.value),
                                placeholder: "Paste a sample command here, e.g. git push origin main",
                                rows: 3,
                                style: {
                                    width: "100%",
                                    fontFamily: "monospace"
                                }
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                "data-testid": "command-sample-generate-button",
                                type: "button",
                                disabled: !commandSampleText.trim() || cmdLoading,
                                onClick: ()=>{
                                    if (commandSampleText.trim()) {
                                        void onCmdGenerate({
                                            source: types_pb/* SuggestionSource */.Ou.COMMAND_SAMPLE,
                                            commandSample: commandSampleText.trim()
                                        });
                                    }
                                },
                                children: cmdLoading ? "Generating…" : "Generate"
                            })
                        ]
                    })
                ]
            }),
            onSaveToConfig && !editRule && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                style: {
                    display: "flex",
                    alignItems: "center",
                    gap: 8,
                    fontSize: 13
                },
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        style: {
                            color: "var(--text-secondary)",
                            fontWeight: 500
                        },
                        children: "Save to:"
                    }),
                    [
                        "db",
                        "config"
                    ].map((d)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                            style: {
                                display: "flex",
                                alignItems: "center",
                                gap: 4,
                                cursor: "pointer"
                            },
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                    type: "radio",
                                    name: "destination",
                                    value: d,
                                    checked: destination === d,
                                    onChange: ()=>setDestination(d)
                                }),
                                d === "db" ? "Database" : "Config File"
                            ]
                        }, d)),
                    destination === "config" && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        style: {
                            fontSize: 11,
                            color: "var(--text-muted)",
                            fontFamily: "monospace"
                        },
                        children: "→ shared_rules.yaml"
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: actions,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: saveBtn,
                        onClick: handleSave,
                        disabled: saving,
                        children: saving ? "Saving…" : editRule ? "Update Rule" : destination === "config" ? "Save to Config File" : "Save Rule"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: cancelBtn,
                        onClick: onCancel,
                        children: "Cancel"
                    })
                ]
            })
        ]
    });
}

;// ./src/lib/ruleTemplates.ts

const RULE_TEMPLATES = [
    {
        id: "python-script-module",
        title: "Python script/module",
        description: "Allow python3 running a .py file or -m module. Inline -c still escalates.",
        icon: "🐍",
        decision: types_pb/* AutoDecision */.Uz.ALLOW,
        riskLevel: "low",
        priority: 100,
        programs: [
            "python",
            "python3",
            "pypy",
            "pypy3"
        ],
        pythonModes: [
            "script",
            "module",
            "version"
        ],
        reason: "Python running a project script or module."
    },
    {
        id: "python-inline-stdlib",
        title: "Python inline — stdlib only",
        description: "Allow python -c when all imports are from the safe stdlib (json, os, sys…).",
        icon: "🐍",
        decision: types_pb/* AutoDecision */.Uz.ALLOW,
        riskLevel: "low",
        priority: 100,
        programs: [
            "python",
            "python3",
            "pypy",
            "pypy3"
        ],
        pythonModes: [
            "inline"
        ],
        safePythonImportsOnly: true,
        reason: "Inline Python using only safe stdlib modules."
    },
    {
        id: "python-inline-any",
        title: "Python inline — any imports",
        description: "Escalate python -c with any imports for manual review.",
        icon: "🐍",
        decision: types_pb/* AutoDecision */.Uz.ESCALATE,
        riskLevel: "medium",
        priority: 50,
        programs: [
            "python",
            "python3",
            "pypy",
            "pypy3"
        ],
        pythonModes: [
            "inline"
        ],
        reason: "Inline Python with non-stdlib imports can make network calls or run arbitrary code.",
        alternative: "Write the code to a .py file and run it with python script.py instead."
    },
    {
        id: "git-read",
        title: "Git read-only",
        description: "Allow read-only git commands: log, status, diff, show, branch, etc.",
        icon: "📖",
        decision: types_pb/* AutoDecision */.Uz.ALLOW,
        riskLevel: "low",
        priority: 100,
        programs: [
            "git"
        ],
        subcommands: [
            "log",
            "status",
            "diff",
            "show",
            "branch",
            "remote",
            "fetch",
            "tag",
            "describe",
            "ls-files"
        ],
        reason: "Read-only git operations pose no risk."
    },
    {
        id: "git-push",
        title: "Git push",
        description: "Escalate all git push operations for manual review.",
        icon: "🚀",
        decision: types_pb/* AutoDecision */.Uz.ESCALATE,
        riskLevel: "high",
        priority: 50,
        programs: [
            "git"
        ],
        subcommands: [
            "push"
        ],
        reason: "git push modifies remote state and should be reviewed."
    },
    {
        id: "git-reset-hard",
        title: "Git reset --hard",
        description: "Deny git reset --hard — discards uncommitted changes irreversibly.",
        icon: "🚫",
        decision: types_pb/* AutoDecision */.Uz.DENY,
        riskLevel: "high",
        priority: 1000,
        programs: [
            "git"
        ],
        subcommands: [
            "reset"
        ],
        requiredFlags: [
            "--hard"
        ],
        reason: "git reset --hard discards uncommitted changes and cannot be undone.",
        alternative: "Use git stash to save changes, or git reset HEAD~1 to keep changes staged."
    },
    {
        id: "npm-install",
        title: "npm / yarn / pnpm install",
        description: "Allow standard package install operations.",
        icon: "📦",
        decision: types_pb/* AutoDecision */.Uz.ALLOW,
        riskLevel: "low",
        priority: 100,
        programs: [
            "npm",
            "npx",
            "yarn",
            "pnpm"
        ],
        reason: "Node.js package management and script execution."
    },
    {
        id: "npm-publish",
        title: "npm publish",
        description: "Escalate npm publish and credential operations for review.",
        icon: "📤",
        decision: types_pb/* AutoDecision */.Uz.ESCALATE,
        riskLevel: "high",
        priority: 500,
        programs: [
            "npm"
        ],
        subcommands: [
            "publish",
            "adduser",
            "login",
            "logout"
        ],
        reason: "npm publish/credential operations affect the public registry and should be reviewed."
    },
    {
        id: "docker-read",
        title: "Docker read-only",
        description: "Allow read-only Docker inspection: ps, logs, inspect, images.",
        icon: "🐳",
        decision: types_pb/* AutoDecision */.Uz.ALLOW,
        riskLevel: "low",
        priority: 100,
        programs: [
            "docker"
        ],
        subcommands: [
            "ps",
            "images",
            "logs",
            "inspect",
            "info",
            "version",
            "stats",
            "top"
        ],
        reason: "Read-only Docker inspection commands."
    },
    {
        id: "docker-write",
        title: "Docker write operations",
        description: "Escalate docker run, exec, build, rm and other mutating commands.",
        icon: "🐳",
        decision: types_pb/* AutoDecision */.Uz.ESCALATE,
        riskLevel: "medium",
        priority: 50,
        programs: [
            "docker"
        ],
        subcommands: [
            "run",
            "exec",
            "build",
            "push",
            "pull",
            "rm",
            "stop",
            "start",
            "restart",
            "compose"
        ],
        reason: "Docker operations that create, modify, or remove containers should be reviewed."
    },
    {
        id: "mcp-read-tools",
        title: "MCP read-only tools",
        description: "Allow all read-only MCP tools (context7, filesystem reads, etc.).",
        icon: "🔌",
        decision: types_pb/* AutoDecision */.Uz.ALLOW,
        riskLevel: "low",
        priority: 100,
        toolCategory: "mcp-read",
        reason: "Read-only MCP tools pose no risk."
    },
    {
        id: "file-editing-tools",
        title: "File editing tools",
        description: "Allow Claude Code's core file editing tools (Edit, Write, MultiEdit).",
        icon: "✏️",
        decision: types_pb/* AutoDecision */.Uz.ALLOW,
        riskLevel: "low",
        priority: 100,
        toolCategory: "builtin",
        reason: "Core Claude Code file editing tools."
    },
    {
        id: "escalate-unknown",
        title: "Escalate unknown program",
        description: "Catch-all: escalate any Bash command not matched by other rules.",
        icon: "❓",
        decision: types_pb/* AutoDecision */.Uz.ESCALATE,
        riskLevel: "medium",
        priority: 1,
        toolName: "Bash",
        commandPattern: ".*",
        reason: "No specific rule matched this program. Review before allowing."
    }
];

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Frules%2FTemplateLibrary.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA6VVwW6jMBC95yt8WalIdQQkaVP3sn9SGTyAU7CRbRKSVf99ZQNbQ0LIqidgZjzvMbw3rD-iXL1cDipEf1YI1VJzw6UgKOMtsPcVQlxoMASF9j6h6WeuZCMYQUeqnjD-iN7gEqsLDWz-grlg0BK0c-WM67qkZ4KyEloboCXPBeYGKk1QCsKAsuFDow3PzjiVwoAwfqqmjHGRT-E2UfC--lqtB_aRYz9P783RS6RioAiK6hZpWXI2qTp4VVhRxht9BbwP7tGKXfbEmSkIisLwl32saIv70OtLWLdDrACeF4agfXgsbEweQWWlPOEzQbQxsiPTYl1QJk9XWNV4BLEbQSaFwZpf4KqcOWouf-qBJxWFq0hlKdU0FwYdaZVzgRNpjKymJfF5zGezwCeZR4uX0aYC2Dq0f4LLFXfitVdsoKpLagCnsmwqoQlSUAM1T5tnFGXKgeW0voIIxxC7McSgaXvFjCtIO-N0IDM93ZDm5RM-JFN4QKavwX2_dvm0UdqOv5Z8MJyB1mDnU4JKyIyLKSqGvdDDuu-GwnW008-eSLvIeGyksMLuDOodnjKqggW9f04-B8lk2mh85JonJbj-sjElF0BQPDO6DqQvwzLL3G6LrSe91i9T6UbrnYLKHnXnBuNGo1OvDwn-Bwb0sPYLWHTJXOP3WG9H3d_GSufCVSelTD89YxpZf--pXtFd8Kbml8jeG0w-_zqTPUAddZ_hjb0xVvnws_G6JD_dXb7xhBTg23p4njMfg1Qq2rmtEQyUnf6YnmepO2r5XQHjFD15_599GNZt4A5OVufSroy_d-XX7d7b_X_3jjLVN_wLD9uwsIgIAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Frules_2FTemplateLibrary_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA6VVwW6jMBC95yt8WalIdQQkaVP3sn9SGTyAU7CRbRKSVf99ZQNbQ0LIqidgZjzvMbw3rD_iXL1cDipEf1YI1VJzw6UgKOMtsPcVQlxoMASF9j6h6WeuZCMYQUeqnjD_iN7gEqsLDWz_grlg0BK0c_WM67qkZ4KyEloboCXPBeYGKk1QCsKAsuFDow3PzjiVwoAwfqqmjHGRT_E2UfC_lqtB_aRYz9P783RS6RioAiK6hZpWXI2qTp4VVhRxht9BbwP7tGKXfbEmSkIisLwl32saIv70OtLWLdDrACeF4agfXgsbEweQWWlPOEzQbQxsiPTYl1QJk9XWNV4BLEbQSaFwZpf4KqcOWouf_qBJxWFq0hlKdU0FwYdaZVzgRNpjKymJfF5zGezwCeZR4uX0aYC2Dq0f4LLFXfitVdsoKpLagCnsmwqoQlSUAM1T5tnFGXKgeW0voIIxxC7McSgaXvFjCtIO_N0IDM93ZDm5RM_JFN4QKavwX2_dvm0UdqOv5Z8MJyB1mDnU4JKyIyLKSqGvdDDuu_GwnW008_eSLvIeGyksMLuDOodnjKqggW9f04_B8lk2mh85JonJbj_sjElF0BQPDO6DqQvwzLL3G6LrSe91i9T6UbrnYLKHnXnBuNGo1OvDwn_Bwb0sPYLWHTJXOP3WG9H3d_GSufCVSelTD89YxpZf_pXtFd8Kbml8jeG0w_zqTPUAddZ_hjb0xVvnws_G6JD_dXb7xhBTg23p4njMfg1Qq2rmtEQyUnf6YnmepO2r5XQHjFD15_599GNZt4A5OVufSroy_d_XX7d7b_X_3jjLVN_wLD9uwsIgIAAA_22_7D = __webpack_require__(228);
;// ./src/components/rules/TemplateLibrary.css.ts

var card = '_1gr6zjr5';
var cardDecision = '_1gr6zjr9';
var cardDesc = '_1gr6zjr8';
var cardIcon = '_1gr6zjr6';
var cardTitle = '_1gr6zjr7';
var dialog = '_1gr6zjr1';
var dialogSubtitle = '_1gr6zjr3';
var dialogTitle = '_1gr6zjr2';
var footer = '_1gr6zjra';
var TemplateLibrary_css_grid = '_1gr6zjr4';
var overlay = '_1gr6zjr0';
var scratchLink = '_1gr6zjrb';
;// ./src/components/rules/TemplateLibrary.tsx
/* __next_internal_client_entry_do_not_use__ TemplateLibrary auto */ 




function TemplateLibrary_decisionLabel(d) {
    switch(d){
        case types_pb/* AutoDecision */.Uz.ALLOW:
            return "Auto-Allow";
        case types_pb/* AutoDecision */.Uz.DENY:
            return "Auto-Deny";
        default:
            return "Escalate";
    }
}
function TemplateLibrary(param) {
    let { open, onClose, onSelect } = param;
    const dialogRef = (0,react.useRef)(null);
    // Focus trap + Escape key
    (0,react.useEffect)(()=>{
        var _dialogRef_current;
        if (!open) return;
        (_dialogRef_current = dialogRef.current) === null || _dialogRef_current === void 0 ? void 0 : _dialogRef_current.focus();
        const handleKey = (e)=>{
            if (e.key === "Escape") onClose();
        };
        document.addEventListener("keydown", handleKey);
        return ()=>document.removeEventListener("keydown", handleKey);
    }, [
        open,
        onClose
    ]);
    if (!open) return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: overlay,
        onClick: (e)=>{
            if (e.target === e.currentTarget) onClose();
        },
        role: "dialog",
        "aria-modal": "true",
        "aria-label": "Choose a rule template",
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: dialog,
            ref: dialogRef,
            tabIndex: -1,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                    className: dialogTitle,
                    children: "Start from a template"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: dialogSubtitle,
                    children: "Select a starting point. All fields can be edited before saving."
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: TemplateLibrary_css_grid,
                    children: RULE_TEMPLATES.map((tpl)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                            className: card,
                            onClick: ()=>{
                                onSelect(tpl);
                                onClose();
                            },
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: cardIcon,
                                    children: tpl.icon
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: cardTitle,
                                    children: tpl.title
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: cardDesc,
                                    children: tpl.description
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: cardDecision,
                                    children: TemplateLibrary_decisionLabel(tpl.decision)
                                })
                            ]
                        }, tpl.id))
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: footer,
                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: scratchLink,
                        onClick: onClose,
                        children: "Start from scratch instead"
                    })
                })
            ]
        })
    });
}

;// ./src/lib/describeRule.ts
const TOOL_CATEGORY_LABELS = {
    "builtin": "built-in Claude Code",
    "builtin-agent": "agent/planning built-in",
    "mcp": "MCP",
    "mcp-read": "read-only MCP",
    "mcp-write": "write MCP"
};
function toolCategoryLabel(cat) {
    var _TOOL_CATEGORY_LABELS_cat;
    return (_TOOL_CATEGORY_LABELS_cat = TOOL_CATEGORY_LABELS[cat]) !== null && _TOOL_CATEGORY_LABELS_cat !== void 0 ? _TOOL_CATEGORY_LABELS_cat : cat;
}
function pythonModeDescription(modes) {
    const labels = {
        script: "a .py script",
        module: "-m module",
        inline: "inline -c code",
        version: "-V version check"
    };
    const mapped = modes.map((m)=>{
        var _labels_m;
        return (_labels_m = labels[m]) !== null && _labels_m !== void 0 ? _labels_m : m;
    });
    if (mapped.length === 0) return "any invocation";
    if (mapped.length === 1) return mapped[0];
    if (mapped.length === 2) return "".concat(mapped[0], " or ").concat(mapped[1]);
    return "".concat(mapped.slice(0, -1).join(", "), ", or ").concat(mapped[mapped.length - 1]);
}
function describeRule(rule) {
    var _rule_programs;
    const programs = (_rule_programs = rule.programs) !== null && _rule_programs !== void 0 ? _rule_programs : [];
    var _rule_subcommands;
    const subcommands = (_rule_subcommands = rule.subcommands) !== null && _rule_subcommands !== void 0 ? _rule_subcommands : [];
    var _rule_blockedSubcommands;
    const blocked = (_rule_blockedSubcommands = rule.blockedSubcommands) !== null && _rule_blockedSubcommands !== void 0 ? _rule_blockedSubcommands : [];
    var _rule_requiredFlags;
    const requiredFlags = (_rule_requiredFlags = rule.requiredFlags) !== null && _rule_requiredFlags !== void 0 ? _rule_requiredFlags : [];
    var _rule_forbiddenFlags;
    const forbiddenFlags = (_rule_forbiddenFlags = rule.forbiddenFlags) !== null && _rule_forbiddenFlags !== void 0 ? _rule_forbiddenFlags : [];
    var _rule_pythonModes;
    const pythonModes = (_rule_pythonModes = rule.pythonModes) !== null && _rule_pythonModes !== void 0 ? _rule_pythonModes : [];
    const hasStructured = programs.length > 0 || subcommands.length > 0 || blocked.length > 0 || requiredFlags.length > 0 || forbiddenFlags.length > 0 || pythonModes.length > 0 || rule.safePythonImportsOnly;
    if (hasStructured) {
        const progStr = programs.join("/") || "any program";
        let primary;
        if (pythonModes.length > 0) {
            primary = "".concat(progStr, " running ").concat(pythonModeDescription(pythonModes));
            if (rule.safePythonImportsOnly) {
                primary += " (stdlib imports only)";
            }
        } else if (subcommands.length > 0) {
            const subStr = subcommands.join(", ");
            const flagStr = requiredFlags.length > 0 ? " with ".concat(requiredFlags.join(" or ")) : "";
            primary = "".concat(progStr, " ").concat(subStr).concat(flagStr);
        } else if (requiredFlags.length > 0) {
            primary = "".concat(progStr, " with ").concat(requiredFlags.join(" or "));
        } else {
            primary = "".concat(progStr, " (any subcommand)");
        }
        let secondary;
        const parts = [];
        if (blocked.length > 0) parts.push("blocked: ".concat(blocked.join(", ")));
        if (forbiddenFlags.length > 0) parts.push("not: ".concat(forbiddenFlags.join(", ")));
        if (parts.length > 0) secondary = parts.join(" · ");
        return {
            primary,
            secondary,
            isStructured: true,
            isRegex: false
        };
    }
    if (rule.toolName) {
        return {
            primary: "Tool: ".concat(rule.toolName),
            isStructured: false,
            isRegex: false
        };
    }
    if (rule.toolCategory) {
        return {
            primary: "Any ".concat(toolCategoryLabel(rule.toolCategory), " tool"),
            isStructured: false,
            isRegex: false
        };
    }
    if (rule.toolPattern) {
        return {
            primary: "Tools matching: ".concat(rule.toolPattern),
            isStructured: false,
            isRegex: true
        };
    }
    if (rule.commandPattern) {
        return {
            primary: "Pattern: ".concat(rule.commandPattern),
            isStructured: false,
            isRegex: true
        };
    }
    if (rule.filePattern) {
        return {
            primary: "File pattern: ".concat(rule.filePattern),
            isStructured: false,
            isRegex: true
        };
    }
    return {
        primary: "(no match criteria)",
        isStructured: false,
        isRegex: false
    };
}

;// ./src/components/rules/MatchDescription.tsx


function MatchDescription(param) {
    let { rule, matchChipClass } = param;
    const desc = describeRule(rule);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        children: [
            desc.isRegex ? /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                className: matchChipClass,
                children: desc.primary
            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                style: {
                    fontSize: "0.8rem"
                },
                children: desc.primary
            }),
            desc.secondary && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                style: {
                    fontSize: "0.75rem",
                    opacity: 0.65,
                    marginTop: "2px"
                },
                children: desc.secondary
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/components/sessions/SeverityBadge.tsx + 1 modules
var SeverityBadge = __webpack_require__(55720);
// EXTERNAL MODULE: ./src/lib/hooks/useTaggingRuleNames.ts
var useTaggingRuleNames = __webpack_require__(48843);
;// ./src/lib/hooks/useTaggingRules.ts
/* __next_internal_client_entry_do_not_use__ useTaggingRules auto */ 







/** Builds a `TaggingRuleProto` from partial rule data, filling in defaults for unset fields. */ function buildTaggingRuleProto(ruleData) {
    var _ruleData_name, _ruleData_namePattern, _ruleData_branchPattern, _ruleData_pathPattern, _ruleData_programPattern, _ruleData_requiredTags, _ruleData_outputTag, _ruleData_priority, _ruleData_enabled;
    return (0,create/* create */.v)(types_pb/* TaggingRuleProtoSchema */.PW, {
        id: ruleData.id,
        name: (_ruleData_name = ruleData.name) !== null && _ruleData_name !== void 0 ? _ruleData_name : "",
        namePattern: (_ruleData_namePattern = ruleData.namePattern) !== null && _ruleData_namePattern !== void 0 ? _ruleData_namePattern : "",
        branchPattern: (_ruleData_branchPattern = ruleData.branchPattern) !== null && _ruleData_branchPattern !== void 0 ? _ruleData_branchPattern : "",
        pathPattern: (_ruleData_pathPattern = ruleData.pathPattern) !== null && _ruleData_pathPattern !== void 0 ? _ruleData_pathPattern : "",
        programPattern: (_ruleData_programPattern = ruleData.programPattern) !== null && _ruleData_programPattern !== void 0 ? _ruleData_programPattern : "",
        requiredTags: (_ruleData_requiredTags = ruleData.requiredTags) !== null && _ruleData_requiredTags !== void 0 ? _ruleData_requiredTags : [],
        outputTag: (_ruleData_outputTag = ruleData.outputTag) !== null && _ruleData_outputTag !== void 0 ? _ruleData_outputTag : "",
        priority: (_ruleData_priority = ruleData.priority) !== null && _ruleData_priority !== void 0 ? _ruleData_priority : 50,
        enabled: (_ruleData_enabled = ruleData.enabled) !== null && _ruleData_enabled !== void 0 ? _ruleData_enabled : true,
        source: "user"
    });
}
/**
 * React hook for managing session-tagging rules (the sibling rule system to
 * approval rules — see ApprovalRulesPanel/useApprovalRules for the equivalent
 * pattern). Loads rules via `listTaggingRules` and exposes upsert/delete
 * actions against the ConnectRPC SessionService.
 *
 * Only "user" source rules can be edited/deleted/toggled; seed rules are
 * read-only (mirrors approval rules' seed-rule convention).
 */ function useTaggingRules() {
    const [rules, setRules] = (0,react.useState)([]);
    const [loading, setLoading] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    const clientRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
    }, []);
    const fetchRules = (0,react.useCallback)(async ()=>{
        if (!clientRef.current) return;
        setLoading(true);
        setError(null);
        try {
            const req = (0,create/* create */.v)(session_pb/* ListTaggingRulesRequestSchema */.XyZ, {});
            const resp = await clientRef.current.listTaggingRules(req);
            var _resp_rules;
            setRules((_resp_rules = resp.rules) !== null && _resp_rules !== void 0 ? _resp_rules : []);
        } catch (err) {
            const e = err instanceof Error ? err : new Error("Failed to fetch tagging rules");
            setError(e);
            console.error("Failed to fetch tagging rules:", e);
        } finally{
            setLoading(false);
        }
    }, []);
    const refresh = (0,react.useCallback)(async ()=>{
        await fetchRules();
    }, [
        fetchRules
    ]);
    (0,react.useEffect)(()=>{
        refresh();
    }, [
        refresh
    ]);
    const upsertRule = (0,react.useCallback)(async (ruleData)=>{
        if (!clientRef.current) return;
        const rule = buildTaggingRuleProto(ruleData);
        const req = (0,create/* create */.v)(session_pb/* UpsertTaggingRuleRequestSchema */.Rf2, {
            rule
        });
        await clientRef.current.upsertTaggingRule(req);
        (0,useTaggingRuleNames/* invalidateTaggingRuleNamesCache */.$g)();
        await refresh();
    }, [
        refresh
    ]);
    const deleteRule = (0,react.useCallback)(async (id)=>{
        if (!clientRef.current) return;
        const req = (0,create/* create */.v)(session_pb/* DeleteTaggingRuleRequestSchema */.BPf, {
            id
        });
        await clientRef.current.deleteTaggingRule(req);
        (0,useTaggingRuleNames/* invalidateTaggingRuleNamesCache */.$g)();
        setRules((prev)=>prev.filter((r)=>r.id !== id));
    }, []);
    return {
        rules,
        loading,
        error,
        upsertRule,
        deleteRule,
        refresh
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Frules%2FTaggingRuleBuilderForm.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xZ3V6dmU1MCB7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNhKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxMik7CiAgbWFyZ2luOiAwOwp9Ci5fMWd1enZlNTEgewogIGJvcmRlci1jb2xvcjogdmFyKC0tXzE5ZXoycnoxMCk7Cn0%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Frules_2FTaggingRuleBuilderForm_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xZ3V6dmU1MCB7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNhKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxMik7CiAgbWFyZ2luOiAwOwp9Ci5fMWd1enZlNTEgewogIGJvcmRlci1jb2xvcjogdmFyKC0tXzE5ZXoycnoxMCk7Cn0_22_7D = __webpack_require__(87448);
;// ./src/components/rules/TaggingRuleBuilderForm.css.ts

var fieldError = '_1guzve50';
var invalidInput = '_1guzve51';
;// ./src/components/rules/TaggingRuleBuilderForm.tsx
/* __next_internal_client_entry_do_not_use__ PATTERN_ERROR_ID,TaggingRuleBuilderForm auto */ 




const PATTERN_ERROR_ID = "tagging-rule-pattern-error";
const MATCH_FIELDS = [
    {
        value: "name",
        label: "Name"
    },
    {
        value: "branch",
        label: "Branch"
    },
    {
        value: "path",
        label: "Path"
    },
    {
        value: "program",
        label: "Program"
    }
];
/** Validates a regex pattern, returning a human-readable error message or null if valid. */ function validatePattern(pattern) {
    if (!pattern) return null;
    try {
        // eslint-disable-next-line no-new
        new RegExp(pattern);
        return null;
    } catch (e) {
        return e instanceof Error ? e.message : "Invalid regex pattern";
    }
}
/** Reads whichever pattern field applies to `matchField` from an existing rule. */ function patternForField(rule, field) {
    switch(field){
        case "name":
            return rule.namePattern;
        case "branch":
            return rule.branchPattern;
        case "path":
            return rule.pathPattern;
        case "program":
            return rule.programPattern;
    }
}
/**
 * TaggingRuleBuilderForm — a new form built for the tagging-rule CRUD tab
 * (ux.md Surface 4/5), scoped to single-field matching (Task 6.1.1b: one
 * "match against" selector + one pattern field, not all four ANDed patterns
 * at once — multi-field rules remain editable via the API directly).
 *
 * Deliberately a separate component from RuleBuilderForm.tsx rather than an
 * extension of it: RuleBuilderForm is tightly coupled to ApprovalRuleProto's
 * shape (decision/risk/tool-target fields that don't apply here). It reuses
 * RuleBuilderForm.css's field-row classes for visual consistency instead.
 */ function TaggingRuleBuilderForm(param) {
    let { editRule, onSave, onCancel } = param;
    const [name, setName] = (0,react.useState)("");
    const [matchField, setMatchField] = (0,react.useState)("branch");
    const [pattern, setPattern] = (0,react.useState)("");
    const [requiredTags, setRequiredTags] = (0,react.useState)([]);
    const [outputTag, setOutputTag] = (0,react.useState)("");
    const [priority, setPriority] = (0,react.useState)(50);
    const [enabled, setEnabled] = (0,react.useState)(true);
    const [saving, setSaving] = (0,react.useState)(false);
    const [formError, setFormError] = (0,react.useState)(null);
    const [patternError, setPatternError] = (0,react.useState)(null);
    const [patternTouched, setPatternTouched] = (0,react.useState)(false);
    (0,react.useEffect)(()=>{
        if (!editRule) return;
        setName(editRule.name);
        var _find;
        const field = (_find = [
            "name",
            "branch",
            "path",
            "program"
        ].find((f)=>patternForField(editRule, f))) !== null && _find !== void 0 ? _find : "branch";
        setMatchField(field);
        setPattern(patternForField(editRule, field));
        var _editRule_requiredTags;
        setRequiredTags((_editRule_requiredTags = editRule.requiredTags) !== null && _editRule_requiredTags !== void 0 ? _editRule_requiredTags : []);
        var _editRule_outputTag;
        setOutputTag((_editRule_outputTag = editRule.outputTag) !== null && _editRule_outputTag !== void 0 ? _editRule_outputTag : "");
        var _editRule_priority;
        setPriority((_editRule_priority = editRule.priority) !== null && _editRule_priority !== void 0 ? _editRule_priority : 50);
        var _editRule_enabled;
        setEnabled((_editRule_enabled = editRule.enabled) !== null && _editRule_enabled !== void 0 ? _editRule_enabled : true);
    }, [
        editRule
    ]);
    function runPatternValidation(value) {
        const err = validatePattern(value);
        setPatternError(err);
        return err === null;
    }
    function handlePatternChange(value) {
        setPattern(value);
        if (patternTouched) runPatternValidation(value);
    }
    function handlePatternBlur() {
        setPatternTouched(true);
        runPatternValidation(pattern);
    }
    async function handleSave() {
        if (!name.trim()) {
            setFormError("Name is required.");
            return;
        }
        if (!outputTag.trim()) {
            setFormError("Output tag is required.");
            return;
        }
        setPatternTouched(true);
        if (!runPatternValidation(pattern)) {
            var _document_getElementById;
            setFormError(null);
            (_document_getElementById = document.getElementById("tagging-rule-pattern-input")) === null || _document_getElementById === void 0 ? void 0 : _document_getElementById.focus();
            return;
        }
        setFormError(null);
        setSaving(true);
        try {
            const rulePayload = {
                name: name.trim(),
                namePattern: matchField === "name" ? pattern : "",
                branchPattern: matchField === "branch" ? pattern : "",
                pathPattern: matchField === "path" ? pattern : "",
                programPattern: matchField === "program" ? pattern : "",
                requiredTags,
                outputTag: outputTag.trim(),
                priority,
                enabled
            };
            var _editRule_id;
            const id = (_editRule_id = editRule === null || editRule === void 0 ? void 0 : editRule.id) !== null && _editRule_id !== void 0 ? _editRule_id : "user-".concat(Date.now());
            await onSave({
                id,
                ...rulePayload
            });
        } catch (e) {
            setFormError(e instanceof Error ? e.message : "Failed to save rule.");
        } finally{
            setSaving(false);
        }
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: formWrapper,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                className: formTitle,
                children: editRule ? "Editing: ".concat(editRule.name) : "New Tagging Rule"
            }),
            formError && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: errorBanner,
                role: "alert",
                children: formError
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: section,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: sectionTitle,
                        children: "Match"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: formGrid,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: fieldLabel,
                                htmlFor: "tagging-rule-match-field-select",
                                children: [
                                    "Match against",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("select", {
                                        id: "tagging-rule-match-field-select",
                                        className: fieldSelect,
                                        "data-testid": "tagging-rule-match-field-select",
                                        value: matchField,
                                        onChange: (e)=>setMatchField(e.target.value),
                                        children: MATCH_FIELDS.map((f)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                value: f.value,
                                                children: f.label
                                            }, f.value))
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: fieldLabel,
                                htmlFor: "tagging-rule-pattern-input",
                                children: [
                                    "Pattern (regex)",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        id: "tagging-rule-pattern-input",
                                        className: "".concat(fieldInput, " ").concat(patternError ? invalidInput : ""),
                                        "data-testid": "tagging-rule-pattern-input",
                                        value: pattern,
                                        onChange: (e)=>handlePatternChange(e.target.value),
                                        onBlur: handlePatternBlur,
                                        "aria-invalid": patternError ? "true" : undefined,
                                        "aria-describedby": patternError ? PATTERN_ERROR_ID : undefined,
                                        placeholder: matchField === "branch" ? "e.g. ^(bugfix|fix)/" : "e.g. ^claude$"
                                    }),
                                    patternError && /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                                        id: PATTERN_ERROR_ID,
                                        className: fieldError,
                                        "data-testid": "tagging-rule-pattern-error",
                                        children: [
                                            "Invalid regex: ",
                                            patternError
                                        ]
                                    })
                                ]
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: section,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: sectionTitle,
                        children: "Output"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: formGrid,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: fieldLabel,
                                children: [
                                    "Requires tags (optional)",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(TagInput, {
                                        value: requiredTags,
                                        onChange: setRequiredTags,
                                        placeholder: "e.g. Bugfix"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: fieldLabel,
                                htmlFor: "tagging-rule-output-tag-input",
                                children: [
                                    "Output tag",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        id: "tagging-rule-output-tag-input",
                                        className: fieldInput,
                                        "data-testid": "tagging-rule-output-tag-input",
                                        value: outputTag,
                                        onChange: (e)=>setOutputTag(e.target.value),
                                        placeholder: "e.g. Bugfix"
                                    })
                                ]
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: section,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: sectionTitle,
                        children: "Details"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: formGrid,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: "".concat(fieldLabel, " ").concat(formGridFull),
                                children: [
                                    "Name *",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        className: fieldInput,
                                        "data-testid": "tagging-rule-name-input",
                                        value: name,
                                        onChange: (e)=>setName(e.target.value),
                                        placeholder: "e.g. Bugfix branch"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: fieldLabel,
                                children: [
                                    "Priority",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        className: fieldInput,
                                        type: "number",
                                        min: 1,
                                        max: 9999,
                                        value: priority,
                                        onChange: (e)=>setPriority(Number(e.target.value))
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: checkboxRow,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        type: "checkbox",
                                        checked: enabled,
                                        onChange: (e)=>setEnabled(e.target.checked)
                                    }),
                                    "Rule enabled"
                                ]
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: actions,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: saveBtn,
                        onClick: handleSave,
                        disabled: saving,
                        children: saving ? "Saving…" : "Save Rule"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: cancelBtn,
                        onClick: onCancel,
                        children: "Cancel"
                    })
                ]
            })
        ]
    });
}

;// ./src/components/sessions/TaggingRulesPanel.tsx
/* __next_internal_client_entry_do_not_use__ TaggingRulesPanel auto */ 




/** Describes which single field a rule matches against, for display purposes. */ function matchDescription(rule) {
    if (rule.namePattern) return "name: ".concat(rule.namePattern);
    if (rule.branchPattern) return "branch: ".concat(rule.branchPattern);
    if (rule.pathPattern) return "path: ".concat(rule.pathPattern);
    if (rule.programPattern) return "program: ".concat(rule.programPattern);
    return "—";
}
/**
 * TaggingRulesPanel — the "Tagging Rules" tab's content (ux.md Surface 4).
 * A sibling to ApprovalRulesPanel rather than an extension of it: tagging
 * rules have a different shape (single match-field pattern, output tag) than
 * approval rules (decision/risk/tool-target), so sharing one table/form would
 * require branching most of its rendering logic. Table styling is reused
 * verbatim from ApprovalRulesPanel.css for visual parity, per Task 6.1.1a.
 */ function TaggingRulesPanel() {
    const { rules, loading, upsertRule, deleteRule } = useTaggingRules();
    const [showBuilder, setShowBuilder] = (0,react.useState)(false);
    const [editingRule, setEditingRule] = (0,react.useState)(null);
    const [deleteError, setDeleteError] = (0,react.useState)(null);
    const handleSave = async (rule)=>{
        await upsertRule(rule);
        setShowBuilder(false);
        setEditingRule(null);
    };
    const handleCancel = ()=>{
        setShowBuilder(false);
        setEditingRule(null);
    };
    const handleEdit = (rule)=>{
        setEditingRule(rule);
        setShowBuilder(true);
    };
    const handleDelete = async (rule)=>{
        try {
            setDeleteError(null);
            await deleteRule(rule.id);
        } catch (err) {
            const message = err instanceof Error ? err.message : "Failed to delete tagging rule";
            setDeleteError(message);
            console.error("Failed to delete tagging rule:", err);
        }
    };
    const handleToggle = async (rule)=>{
        if (rule.source !== "user") return;
        await upsertRule({
            id: rule.id,
            name: rule.name,
            namePattern: rule.namePattern,
            branchPattern: rule.branchPattern,
            pathPattern: rule.pathPattern,
            programPattern: rule.programPattern,
            requiredTags: rule.requiredTags,
            outputTag: rule.outputTag,
            priority: rule.priority,
            enabled: !rule.enabled
        });
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        "data-testid": "tagging-rules-panel",
        children: [
            deleteError && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: ApprovalRulesPanel_css/* error */.z3,
                role: "alert",
                "data-testid": "tagging-rule-delete-error",
                children: [
                    "Failed to delete tagging rule: ",
                    deleteError
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: ApprovalRulesPanel_css/* tableWrapper */.wY,
                children: loading && rules.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: ApprovalRulesPanel_css/* loading */.Lq,
                    children: "Loading tagging rules…"
                }) : rules.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: ApprovalRulesPanel_css/* empty */.Ie,
                    "data-testid": "tagging-rules-empty-state",
                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        children: "No tagging rules configured. Sessions will rely on LLM classification only."
                    })
                }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("table", {
                    className: ApprovalRulesPanel_css/* table */.tp,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("thead", {
                            children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: ApprovalRulesPanel_css.th,
                                        children: "Name"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: ApprovalRulesPanel_css.th,
                                        children: "Match"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: ApprovalRulesPanel_css.th,
                                        children: "Output Tag"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: ApprovalRulesPanel_css.th,
                                        children: "Priority"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: ApprovalRulesPanel_css.th,
                                        "data-testid": "tagging-rule-fire-count-header",
                                        title: "Number of times this rule fired in the last 7 days",
                                        children: "Fires (7d)"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: ApprovalRulesPanel_css.th,
                                        children: "Enabled"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: ApprovalRulesPanel_css.th
                                    })
                                ]
                            })
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("tbody", {
                            children: rules.map((rule)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                    className: "".concat(ApprovalRulesPanel_css/* row */.nM, " ").concat(!rule.enabled ? ApprovalRulesPanel_css/* rowDisabled */.lr : ""),
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("td", {
                                            className: ApprovalRulesPanel_css.td,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: ApprovalRulesPanel_css/* ruleName */.oF,
                                                    children: rule.name || rule.id
                                                }),
                                                rule.source === "seed" && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: ApprovalRulesPanel_css/* sourceBadge */.xR,
                                                    title: "Built-in",
                                                    children: "ⓘ Built-in"
                                                })
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                            className: ApprovalRulesPanel_css.td,
                                            children: matchDescription(rule)
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                            className: ApprovalRulesPanel_css.td,
                                            children: rule.outputTag
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                            className: "".concat(ApprovalRulesPanel_css.td, " ").concat(ApprovalRulesPanel_css/* tdCenter */._S),
                                            children: rule.priority
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                            className: "".concat(ApprovalRulesPanel_css.td, " ").concat(ApprovalRulesPanel_css/* tdCenter */._S),
                                            children: rule.fireCount7d > 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: "".concat(ApprovalRulesPanel_css/* hitBadge */.nX, " ").concat(ApprovalRulesPanel_css/* hitBadgeActive */.pp),
                                                children: rule.fireCount7d.toLocaleString()
                                            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: ApprovalRulesPanel_css/* hitBadge */.nX,
                                                children: "—"
                                            })
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                            className: "".concat(ApprovalRulesPanel_css.td, " ").concat(ApprovalRulesPanel_css/* tdCenter */._S),
                                            children: rule.source === "user" ? /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                className: "".concat(ApprovalRulesPanel_css/* toggle */.L$, " ").concat(rule.enabled ? ApprovalRulesPanel_css/* toggleOn */._s : ApprovalRulesPanel_css/* toggleOff */.St),
                                                onClick: ()=>handleToggle(rule),
                                                "aria-label": "".concat(rule.enabled ? "Disable" : "Enable", " tagging rule"),
                                                children: rule.enabled ? "ON" : "OFF"
                                            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: ApprovalRulesPanel_css/* builtInBadge */.h$,
                                                title: "Built-in rules cannot be disabled",
                                                children: "Always on"
                                            })
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                            className: "".concat(ApprovalRulesPanel_css.td, " ").concat(ApprovalRulesPanel_css/* tdCenter */._S),
                                            style: {
                                                display: "flex",
                                                gap: "4px",
                                                alignItems: "center"
                                            },
                                            children: rule.source === "user" && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                        style: {
                                                            fontSize: "0.75rem",
                                                            padding: "2px 8px",
                                                            cursor: "pointer",
                                                            border: "1px solid var(--border-color)",
                                                            borderRadius: "4px",
                                                            background: "transparent",
                                                            color: "inherit"
                                                        },
                                                        onClick: ()=>handleEdit(rule),
                                                        "aria-label": "Edit tagging rule ".concat(rule.name),
                                                        children: "Edit"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                        className: ApprovalRulesPanel_css/* deleteButton */.G5,
                                                        onClick: ()=>{
                                                            void handleDelete(rule);
                                                        },
                                                        "aria-label": "Delete tagging rule ".concat(rule.name),
                                                        title: "Delete tagging rule",
                                                        children: "✕"
                                                    })
                                                ]
                                            })
                                        })
                                    ]
                                }, rule.id))
                        })
                    ]
                })
            }),
            rules.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: ApprovalRulesPanel_css/* rowCount */.oW,
                children: [
                    rules.length,
                    " tagging rule",
                    rules.length !== 1 ? "s" : ""
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: ApprovalRulesPanel_css/* formSection */.gy,
                id: "tagging-rule-builder",
                children: !showBuilder ? /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    "data-testid": "add-tagging-rule-button",
                    className: ApprovalRulesPanel_css/* addButton */.c9,
                    onClick: ()=>{
                        setEditingRule(null);
                        setShowBuilder(true);
                    },
                    children: "+ Add Tagging Rule"
                }) : /*#__PURE__*/ (0,jsx_runtime.jsx)(TaggingRuleBuilderForm, {
                    editRule: editingRule,
                    onSave: handleSave,
                    onCancel: handleCancel
                })
            })
        ]
    });
}

;// ./src/components/sessions/ApprovalRulesPanel.tsx
/* __next_internal_client_entry_do_not_use__ ApprovalRulesPanel auto */ 
// +feature: approval-rules-risk-column















// ── helpers ──────────────────────────────────────────────────────────────────
/** Escape all regex metacharacters in a literal string for safe interpolation into patterns. */ function escapeRegex(s) {
    return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
function ApprovalRulesPanel_decisionLabel(d) {
    switch(d){
        case types_pb/* AutoDecision */.Uz.ALLOW:
            return "Auto-Allow";
        case types_pb/* AutoDecision */.Uz.DENY:
            return "Auto-Deny";
        default:
            return "Escalate";
    }
}
function decisionClass(d) {
    switch(d){
        case types_pb/* AutoDecision */.Uz.ALLOW:
            return ApprovalRulesPanel_css/* decisionAllow */._8;
        case types_pb/* AutoDecision */.Uz.DENY:
            return ApprovalRulesPanel_css/* decisionDeny */.bx;
        default:
            return ApprovalRulesPanel_css/* decisionEscalate */.b6;
    }
}
function sourceLabel(s) {
    switch(s){
        case "user":
            return "Custom";
        case "seed":
            return "Built-in";
        case "claude-settings":
            return "Claude Settings";
        case "config":
            return "Config File";
        default:
            return s;
    }
}
/**
 * ApprovalRulesPanel shows the list of auto-approval rules and lets users
 * create, edit, toggle, and delete custom rules via the structured rule builder.
 */ function ApprovalRulesPanel(param) {
    let { prefill } = param;
    const { rules, loading, error, upsertRule, deleteRule, refresh, reloadClaudeSettingsRules } = (0,useApprovalRules/* useApprovalRules */.I)();
    const { showActionToast } = (0,NotificationContext/* useNotifications */.E$)();
    const [reloadingClaudeSettings, setReloadingClaudeSettings] = (0,react.useState)(false);
    const { summary, loading: analyticsLoading } = useApprovalAnalytics({
        windowDays: 7
    });
    const { exportRules, loading: exporting, error: exportError } = useExportRules();
    // ── Epic 3: panel-level "Generate Suggestions" hook ─────────────────────
    const { suggestions, loading: genLoading, error: genError, generate, cancel, clear } = (0,useGenerateRule/* useGenerateRule */.b)();
    // ── Epic 6: command-sample generate hook (for the in-form "Generate from command" section) ─
    const { suggestions: cmdSuggestions, loading: cmdLoading, generate: cmdGenerate, cancel: cmdCancel, clear: cmdClear } = (0,useGenerateRule/* useGenerateRule */.b)();
    const [cmdSampleText, setCmdSampleText] = (0,react.useState)("");
    const [sourceFilter, setSourceFilter] = (0,react.useState)("all");
    // ux.md Surface 4: the "Tagging Rules" tab lives alongside the existing
    // approval-rule source-filter tabs but renders a wholly separate panel
    // (TaggingRulesPanel), rather than filtering the same table.
    const [showTaggingRulesTab, setShowTaggingRulesTab] = (0,react.useState)(false);
    const [searchQuery, setSearchQuery] = (0,react.useState)("");
    const [sortKey, setSortKey] = (0,react.useState)("priority");
    const [sortDir, setSortDir] = (0,react.useState)("desc");
    const [importModalOpen, setImportModalOpen] = (0,react.useState)(false);
    // ── URL param pre-fill ───────────────────────────────────────────────────
    const urlPrefill = (0,react.useMemo)(()=>{
        if (false) {}
        const p = new URLSearchParams(window.location.search);
        const tool = p.get("tool");
        const program = p.get("program");
        const subcommand = p.get("subcommand");
        const open = p.get("open");
        if (tool) return {
            toolName: tool,
            initialName: "Allow ".concat(tool)
        };
        if (program) {
            const name = subcommand ? "Allow ".concat(program, " ").concat(subcommand) : "Allow ".concat(program);
            return {
                programs: [
                    program
                ],
                subcommands: subcommand ? [
                    subcommand
                ] : undefined,
                initialName: name
            };
        }
        if (open) return {};
        return null;
    }, []);
    const hasUrlParams = urlPrefill !== null;
    const [showBuilder, setShowBuilder] = (0,react.useState)(!!prefill || hasUrlParams);
    const [editingRule, setEditingRule] = (0,react.useState)(null);
    const [templateSeed, setTemplateSeed] = (0,react.useState)(null);
    const [showTemplates, setShowTemplates] = (0,react.useState)(false);
    // ── Escape key to close builder ──────────────────────────────────────────
    (0,react.useEffect)(()=>{
        if (!showBuilder) return;
        const handler = (e)=>{
            if (e.key === "Escape") {
                setShowBuilder(false);
                setEditingRule(null);
                setTemplateSeed(null);
            }
        };
        document.addEventListener("keydown", handler);
        return ()=>document.removeEventListener("keydown", handler);
    }, [
        showBuilder
    ]);
    var _cmdSuggestions_;
    // Merge prop prefill, URL prefill, and cmd suggestions into effective prefill
    const cmdSuggestion = (_cmdSuggestions_ = cmdSuggestions[0]) !== null && _cmdSuggestions_ !== void 0 ? _cmdSuggestions_ : null;
    const effectivePrefill = (()=>{
        const base = prefill !== null && prefill !== void 0 ? prefill : urlPrefill;
        if (!cmdSuggestion) return base;
        return {
            ...base,
            commandPattern: cmdSuggestion.commandPattern,
            isAiGenerated: true
        };
    })();
    // ── hit counts from analytics ─────────────────────────────────────────────
    const hitCountByRuleId = (0,react.useMemo)(()=>{
        const m = new Map();
        var _summary_topTriggeredRules;
        for (const r of (_summary_topTriggeredRules = summary === null || summary === void 0 ? void 0 : summary.topTriggeredRules) !== null && _summary_topTriggeredRules !== void 0 ? _summary_topTriggeredRules : []){
            var _m_get;
            m.set(r.ruleId, ((_m_get = m.get(r.ruleId)) !== null && _m_get !== void 0 ? _m_get : 0) + r.count);
        }
        return m;
    }, [
        summary
    ]);
    // ── sort toggle ───────────────────────────────────────────────────────────
    function handleSort(key) {
        if (sortKey === key) {
            setSortDir((d)=>d === "asc" ? "desc" : "asc");
        } else {
            setSortKey(key);
            setSortDir(key === "priority" || key === "hits" ? "desc" : "asc");
        }
    }
    function sortIcon(key) {
        if (sortKey !== key) return " ↕";
        return sortDir === "asc" ? " ↑" : " ↓";
    }
    // ── filter + sort ─────────────────────────────────────────────────────────
    const visibleRules = (0,react.useMemo)(()=>{
        const q = searchQuery.trim().toLowerCase();
        let filtered = sourceFilter === "all" ? rules : rules.filter((r)=>r.source === sourceFilter);
        if (q) {
            filtered = filtered.filter((r)=>r.name.toLowerCase().includes(q) || r.reason.toLowerCase().includes(q) || r.toolName.toLowerCase().includes(q) || r.toolPattern.toLowerCase().includes(q) || r.commandPattern.toLowerCase().includes(q) || r.programs.some((p)=>p.toLowerCase().includes(q)));
        }
        return [
            ...filtered
        ].sort((a, b)=>{
            let cmp = 0;
            switch(sortKey){
                case "name":
                    cmp = a.name.localeCompare(b.name);
                    break;
                case "decision":
                    cmp = a.decision - b.decision;
                    break;
                case "priority":
                    cmp = a.priority - b.priority;
                    break;
                case "hits":
                    var _hitCountByRuleId_get, _hitCountByRuleId_get1;
                    cmp = ((_hitCountByRuleId_get = hitCountByRuleId.get(a.id)) !== null && _hitCountByRuleId_get !== void 0 ? _hitCountByRuleId_get : 0) - ((_hitCountByRuleId_get1 = hitCountByRuleId.get(b.id)) !== null && _hitCountByRuleId_get1 !== void 0 ? _hitCountByRuleId_get1 : 0);
                    break;
            }
            return sortDir === "asc" ? cmp : -cmp;
        });
    }, [
        rules,
        sourceFilter,
        searchQuery,
        sortKey,
        sortDir,
        hitCountByRuleId
    ]);
    // ── save ──────────────────────────────────────────────────────────────────
    const handleSave = async (rule)=>{
        await upsertRule(rule);
        setShowBuilder(false);
        setEditingRule(null);
        setTemplateSeed(null);
    };
    const handleCancel = ()=>{
        setShowBuilder(false);
        setEditingRule(null);
        setTemplateSeed(null);
        cmdClear();
        setCmdSampleText("");
    };
    const handleEdit = (rule)=>{
        setEditingRule(rule);
        setTemplateSeed(null);
        setShowBuilder(true);
        setTimeout(()=>{
            var _document_getElementById;
            (_document_getElementById = document.getElementById("rule-builder")) === null || _document_getElementById === void 0 ? void 0 : _document_getElementById.scrollIntoView({
                behavior: "smooth"
            });
        }, 50);
    };
    const handleTemplateSelect = (tpl)=>{
        setTemplateSeed(tpl);
        setEditingRule(null);
        setShowBuilder(true);
    };
    // ── toggle enabled ────────────────────────────────────────────────────────
    const handleToggle = async (rule)=>{
        if (rule.source !== "user") return;
        try {
            await upsertRule({
                id: rule.id,
                name: rule.name,
                toolName: rule.toolName,
                toolPattern: rule.toolPattern,
                toolCategory: rule.toolCategory,
                commandPattern: rule.commandPattern,
                filePattern: rule.filePattern,
                decision: rule.decision,
                riskLevel: rule.riskLevel,
                reason: rule.reason,
                alternative: rule.alternative,
                priority: rule.priority,
                enabled: !rule.enabled,
                programs: rule.programs,
                subcommands: rule.subcommands,
                blockedSubcommands: rule.blockedSubcommands,
                requiredFlags: rule.requiredFlags,
                forbiddenFlags: rule.forbiddenFlags,
                requiredFlagPrefixes: rule.requiredFlagPrefixes,
                pythonModes: rule.pythonModes,
                safePythonImportsOnly: rule.safePythonImportsOnly
            });
        } catch (e) {
            console.error("Failed to toggle rule:", e);
        }
    };
    const handleReloadClaudeSettings = async ()=>{
        if (reloadingClaudeSettings) return; // guards against a rapid double-click firing two RPCs
        setReloadingClaudeSettings(true);
        try {
            const resp = await reloadClaudeSettingsRules();
            showActionToast(resp.message || (resp.success ? "Reloaded ".concat(resp.ruleCount, " claude-settings rule(s).") : "Failed to reload Claude settings rules — previous rules still active."), resp.success ? "success" : "error", "claude-settings-reload");
        } catch (e) {
            console.error("Failed to reload claude-settings rules:", e);
            showActionToast("Could not reach the server to reload rules. Try again.", "error", "claude-settings-reload");
        } finally{
            setReloadingClaudeSettings(false);
        }
    };
    // ── Epic 3: handle suggestion cards ──────────────────────────────────────
    const [dismissedIndices, setDismissedIndices] = (0,react.useState)(new Set());
    const handleSuggestionAccept = (_savedRule)=>{
        clear();
        refresh();
        setDismissedIndices(new Set());
    };
    const handleSuggestionDiscard = (idx)=>{
        setDismissedIndices((prev)=>new Set([
                ...prev,
                idx
            ]));
    };
    const visibleSuggestions = suggestions.filter((_, idx)=>!dismissedIndices.has(idx));
    // ── aria-live announcement for generate state changes ─────────────────────
    const genLiveMessage = (0,react.useMemo)(()=>{
        if (genLoading) return "Generating rule suggestions…";
        if (suggestions.length > 0) return "Rule suggestions ready.";
        return "";
    }, [
        genLoading,
        suggestions.length
    ]);
    // ── analytics summary bar ─────────────────────────────────────────────────
    const autoAllowRate = summary ? Math.round(summary.autoApproveRate * 100) : null;
    const manualRate = summary ? Math.round(summary.manualReviewRate * 100) : null;
    const total = summary ? summary.totalDecisions : null;
    // ── Approval Rules tab content (everything below the tab bar that is specific to it) ──────
    // Extracted so the tab-bar branch below is a single `showTaggingRulesTab ? <TaggingRulesPanel
    // /> : renderApprovalRulesContent()` instead of repeating `!showTaggingRulesTab &&` on every
    // block (config/claude-settings hints, error banner, rules table, row count, mobile FAB, rule
    // builder). A plain function (not a nested component) so it isn't remounted on every render.
    function renderApprovalRulesContent() {
        var _summary_commandSubcommandStats;
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
            children: [
                sourceFilter === "config" && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: ApprovalRulesPanel_css/* configFileHint */.aE,
                    children: "Stored in ~/.config/stapler-squad/shared_rules.yaml"
                }),
                sourceFilter === "claude-settings" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: ApprovalRulesPanel_css/* configFileHint */.aE,
                    children: [
                        "Loaded from ~/.claude/settings.json",
                        " ",
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: ApprovalRulesPanel_css/* retryButton */.ft,
                            onClick: handleReloadClaudeSettings,
                            disabled: reloadingClaudeSettings,
                            children: reloadingClaudeSettings ? "Reloading…" : "Reload rules"
                        })
                    ]
                }),
                error && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: ApprovalRulesPanel_css/* error */.z3,
                    children: [
                        "Failed to load rules: ",
                        error.message,
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            onClick: refresh,
                            className: ApprovalRulesPanel_css/* retryButton */.ft,
                            children: "Retry"
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: ApprovalRulesPanel_css/* tableWrapper */.wY,
                    children: loading && visibleRules.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: ApprovalRulesPanel_css/* loading */.Lq,
                        children: "Loading rules…"
                    }) : visibleRules.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: ApprovalRulesPanel_css/* empty */.Ie,
                        "data-testid": "empty-state",
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                children: "Approval rules let you automatically allow or deny tool calls from Claude without manual review."
                            }),
                            (sourceFilter === "all" || sourceFilter === "user") && /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        style: {
                                            background: "none",
                                            border: "none",
                                            cursor: "pointer",
                                            color: "inherit",
                                            textDecoration: "underline",
                                            padding: 0
                                        },
                                        onClick: ()=>{
                                            setTemplateSeed(null);
                                            setEditingRule(null);
                                            setShowBuilder(true);
                                        },
                                        children: "Add Rule"
                                    }),
                                    " ",
                                    "using the builder below or",
                                    " ",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        style: {
                                            background: "none",
                                            border: "none",
                                            cursor: "pointer",
                                            color: "inherit",
                                            textDecoration: "underline",
                                            padding: 0
                                        },
                                        onClick: ()=>setImportModalOpen(true),
                                        children: "Import YAML"
                                    }),
                                    " ",
                                    "to import from a file."
                                ]
                            }),
                            sourceFilter === "seed" && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                children: "No built-in rules are available in this workspace."
                            }),
                            sourceFilter === "claude-settings" && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                children: "No rules from your ~/.claude/settings.json file were found."
                            }),
                            sourceFilter === "config" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                                children: [
                                    'No rules in your config file yet. Use the "→ Config" button on a custom rule to copy it to ',
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                                        children: "~/.config/stapler-squad/shared_rules.yaml"
                                    }),
                                    ', or create a new rule and select "Save to Config File".'
                                ]
                            })
                        ]
                    }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("table", {
                        className: ApprovalRulesPanel_css/* table */.tp,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("thead", {
                                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("th", {
                                            className: "".concat(ApprovalRulesPanel_css.th, " ").concat(ApprovalRulesPanel_css/* thSortable */.r5),
                                            onClick: ()=>handleSort("name"),
                                            children: [
                                                "Name",
                                                sortIcon("name")
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                            className: ApprovalRulesPanel_css.th,
                                            children: "Match"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("th", {
                                            className: "".concat(ApprovalRulesPanel_css.th, " ").concat(ApprovalRulesPanel_css/* thSortable */.r5),
                                            onClick: ()=>handleSort("decision"),
                                            children: [
                                                "Decision",
                                                sortIcon("decision")
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                            className: ApprovalRulesPanel_css.th,
                                            children: "Risk"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                            className: ApprovalRulesPanel_css.th,
                                            children: "Source"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("th", {
                                            className: "".concat(ApprovalRulesPanel_css.th, " ").concat(ApprovalRulesPanel_css/* thSortable */.r5),
                                            title: "Higher priority fires first. Custom rules default to 10; built-in rules default to 100–1000.",
                                            onClick: ()=>handleSort("priority"),
                                            children: [
                                                "Priority ⓘ",
                                                sortIcon("priority")
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("th", {
                                            className: "".concat(ApprovalRulesPanel_css.th, " ").concat(ApprovalRulesPanel_css/* thSortable */.r5),
                                            title: "Number of times this rule fired in the last 7 days",
                                            onClick: ()=>handleSort("hits"),
                                            children: [
                                                "Hits (7d)",
                                                sortIcon("hits")
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                            className: ApprovalRulesPanel_css.th,
                                            children: "Enabled"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                            className: ApprovalRulesPanel_css.th
                                        })
                                    ]
                                })
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("tbody", {
                                children: visibleRules.map((rule)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                        className: "".concat(ApprovalRulesPanel_css/* row */.nM, " ").concat(!rule.enabled ? ApprovalRulesPanel_css/* rowDisabled */.lr : ""),
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("td", {
                                                className: ApprovalRulesPanel_css.td,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: ApprovalRulesPanel_css/* ruleName */.oF,
                                                        children: rule.name || rule.id
                                                    }),
                                                    rule.reason && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: ApprovalRulesPanel_css/* ruleReason */.JJ,
                                                        children: rule.reason
                                                    }),
                                                    rule.alternative && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                        className: ApprovalRulesPanel_css/* ruleAlt */.ax,
                                                        children: [
                                                            "Alt: ",
                                                            rule.alternative
                                                        ]
                                                    })
                                                ]
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                className: ApprovalRulesPanel_css.td,
                                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(MatchDescription, {
                                                    rule: rule,
                                                    matchChipClass: ApprovalRulesPanel_css/* matchChip */.UH
                                                })
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                className: ApprovalRulesPanel_css.td,
                                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: "".concat(ApprovalRulesPanel_css/* decisionBadge */.g, " ").concat(decisionClass(rule.decision)),
                                                    children: ApprovalRulesPanel_decisionLabel(rule.decision)
                                                })
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                className: ApprovalRulesPanel_css.td,
                                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(SeverityBadge/* SeverityBadge */.P, {
                                                    riskLevel: rule.riskLevel,
                                                    compact: true
                                                })
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                className: ApprovalRulesPanel_css.td,
                                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: rule.source === "config" ? ApprovalRulesPanel_css/* configFileBadge */.cJ : ApprovalRulesPanel_css/* sourceBadge */.xR,
                                                    title: rule.source === "seed" ? "These rules ship with stapler-squad and cannot be deleted" : rule.source === "claude-settings" ? "These rules come from your ~/.claude/settings.json file" : rule.source === "config" ? "This rule is stored in ~/.config/stapler-squad/shared_rules.yaml and can be shared" : undefined,
                                                    children: sourceLabel(rule.source)
                                                })
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                className: "".concat(ApprovalRulesPanel_css.td, " ").concat(ApprovalRulesPanel_css/* tdCenter */._S),
                                                children: rule.priority
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                className: "".concat(ApprovalRulesPanel_css.td, " ").concat(ApprovalRulesPanel_css/* tdCenter */._S),
                                                children: (()=>{
                                                    var _hitCountByRuleId_get;
                                                    const hits = (_hitCountByRuleId_get = hitCountByRuleId.get(rule.id)) !== null && _hitCountByRuleId_get !== void 0 ? _hitCountByRuleId_get : 0;
                                                    return hits > 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: "".concat(ApprovalRulesPanel_css/* hitBadge */.nX, " ").concat(ApprovalRulesPanel_css/* hitBadgeActive */.pp),
                                                        children: hits.toLocaleString()
                                                    }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: ApprovalRulesPanel_css/* hitBadge */.nX,
                                                        children: "—"
                                                    });
                                                })()
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                className: "".concat(ApprovalRulesPanel_css.td, " ").concat(ApprovalRulesPanel_css/* tdCenter */._S),
                                                children: rule.source === "user" ? /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                    className: "".concat(ApprovalRulesPanel_css/* toggle */.L$, " ").concat(rule.enabled ? ApprovalRulesPanel_css/* toggleOn */._s : ApprovalRulesPanel_css/* toggleOff */.St),
                                                    onClick: ()=>handleToggle(rule),
                                                    "aria-label": rule.enabled ? "Disable rule" : "Enable rule",
                                                    children: rule.enabled ? "ON" : "OFF"
                                                }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: ApprovalRulesPanel_css/* builtInBadge */.h$,
                                                    title: "Built-in rules cannot be disabled",
                                                    children: "Always on"
                                                })
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                className: "".concat(ApprovalRulesPanel_css.td, " ").concat(ApprovalRulesPanel_css/* tdCenter */._S),
                                                style: {
                                                    display: "flex",
                                                    gap: "4px",
                                                    alignItems: "center"
                                                },
                                                children: rule.source === "user" && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                                    children: [
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                            style: {
                                                                fontSize: "0.75rem",
                                                                padding: "2px 8px",
                                                                cursor: "pointer",
                                                                border: "1px solid var(--border-color)",
                                                                borderRadius: "4px",
                                                                background: "transparent",
                                                                color: "inherit"
                                                            },
                                                            onClick: ()=>handleEdit(rule),
                                                            "aria-label": "Edit rule ".concat(rule.name),
                                                            children: "Edit"
                                                        }),
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                            className: ApprovalRulesPanel_css/* deleteButton */.G5,
                                                            onClick: ()=>deleteRule(rule.id),
                                                            "aria-label": "Delete rule ".concat(rule.name),
                                                            title: "Delete rule",
                                                            children: "✕"
                                                        })
                                                    ]
                                                })
                                            })
                                        ]
                                    }, rule.id))
                            })
                        ]
                    })
                }),
                visibleRules.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: ApprovalRulesPanel_css/* rowCount */.oW,
                    children: [
                        visibleRules.length,
                        " rule",
                        visibleRules.length !== 1 ? "s" : "",
                        (sourceFilter !== "all" || searchQuery.trim()) && " (filtered from ".concat(rules.length, " total)")
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    className: ApprovalRulesPanel_css/* mobileAddFab */.B7,
                    onClick: ()=>{
                        setTemplateSeed(null);
                        setEditingRule(null);
                        setShowBuilder(true);
                    },
                    "aria-label": "Add rule",
                    "data-testid": "add-rule-fab",
                    children: "+"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: ApprovalRulesPanel_css/* formSection */.gy,
                    id: "rule-builder",
                    ...showBuilder ? {
                        role: "dialog"
                    } : {},
                    children: !showBuilder ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        style: {
                            display: "flex",
                            gap: "8px",
                            flexWrap: "wrap"
                        },
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                "data-testid": "add-rule-button",
                                className: ApprovalRulesPanel_css/* addButton */.c9,
                                onClick: ()=>{
                                    setTemplateSeed(null);
                                    setEditingRule(null);
                                    setShowBuilder(true);
                                },
                                children: "+ Add Custom Rule"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: ApprovalRulesPanel_css/* addButton */.c9,
                                style: {
                                    background: "transparent",
                                    border: "1px solid var(--border-color)",
                                    color: "inherit"
                                },
                                onClick: ()=>setShowTemplates(true),
                                children: "Start from Template"
                            })
                        ]
                    }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                style: {
                                    display: "flex",
                                    justifyContent: "flex-end",
                                    marginBottom: 8
                                },
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    "aria-label": "Close dialog",
                                    onClick: handleCancel,
                                    style: {
                                        background: "none",
                                        border: "none",
                                        cursor: "pointer",
                                        fontSize: 18,
                                        color: "var(--text-secondary)",
                                        lineHeight: 1
                                    },
                                    children: "\xd7"
                                })
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("details", {
                                "data-testid": "generate-from-command-details",
                                style: {
                                    marginBottom: 12
                                },
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("summary", {
                                        style: {
                                            cursor: "pointer",
                                            fontSize: 13,
                                            color: "var(--text-secondary)"
                                        },
                                        children: "Generate from command sample"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        style: {
                                            marginTop: 8,
                                            display: "flex",
                                            flexDirection: "column",
                                            gap: 8
                                        },
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("textarea", {
                                                "data-testid": "command-sample-textarea",
                                                value: cmdSampleText,
                                                onChange: (e)=>setCmdSampleText(e.target.value),
                                                placeholder: "Paste a command you ran, e.g. git push origin main",
                                                rows: 3,
                                                style: {
                                                    width: "100%",
                                                    fontFamily: "monospace",
                                                    fontSize: 12,
                                                    padding: "6px 8px",
                                                    borderRadius: 6,
                                                    border: "1px solid var(--border-color)",
                                                    background: "var(--input-background)",
                                                    color: "var(--input-text)",
                                                    resize: "vertical"
                                                }
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                style: {
                                                    display: "flex",
                                                    gap: 8
                                                },
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                        "data-testid": "command-sample-generate-button",
                                                        disabled: !cmdSampleText.trim() || cmdLoading,
                                                        onClick: ()=>{
                                                            void cmdGenerate({
                                                                source: types_pb/* SuggestionSource */.Ou.COMMAND_SAMPLE,
                                                                commandSample: cmdSampleText
                                                            });
                                                        },
                                                        style: {
                                                            padding: "4px 12px",
                                                            fontSize: 12,
                                                            borderRadius: 6,
                                                            border: "none",
                                                            cursor: cmdSampleText.trim() ? "pointer" : "default",
                                                            background: "var(--primary)",
                                                            color: "var(--primary-text)"
                                                        },
                                                        children: cmdLoading ? "Generating…" : "Generate"
                                                    }),
                                                    cmdLoading && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                        onClick: cmdCancel,
                                                        style: {
                                                            padding: "4px 12px",
                                                            fontSize: 12,
                                                            borderRadius: 6,
                                                            border: "1px solid var(--border-color)",
                                                            background: "none",
                                                            cursor: "pointer"
                                                        },
                                                        children: "Cancel"
                                                    })
                                                ]
                                            })
                                        ]
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(RuleBuilderForm, {
                                editRule: editingRule,
                                prefill: showBuilder ? effectivePrefill : null,
                                templateSeed: templateSeed,
                                onSave: handleSave,
                                onCancel: handleCancel,
                                subcommandStats: (_summary_commandSubcommandStats = summary === null || summary === void 0 ? void 0 : summary.commandSubcommandStats) !== null && _summary_commandSubcommandStats !== void 0 ? _summary_commandSubcommandStats : []
                            })
                        ]
                    })
                })
            ]
        });
    }
    // ── render ────────────────────────────────────────────────────────────────
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: ApprovalRulesPanel_css/* panel */.nd,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: ApprovalRulesPanel_css/* header */.wx,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: ApprovalRulesPanel_css/* titleRow */.$,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                                className: ApprovalRulesPanel_css/* title */.DD,
                                children: "Approval Rules"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: ApprovalRulesPanel_css/* generateButtonRow */.u1,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        onClick: ()=>{
                                            setDismissedIndices(new Set());
                                            void generate({
                                                source: types_pb/* SuggestionSource */.Ou.ANALYTICS_GAPS
                                            });
                                        },
                                        className: "".concat(ApprovalRulesPanel_css/* generateButton */.OH, " ").concat(ApprovalRulesPanel_css/* headerButtonsHiddenOnMobile */.k),
                                        disabled: genLoading,
                                        "data-testid": "generate-suggestions",
                                        children: genLoading ? "Generating…" : "Generate Suggestions"
                                    }),
                                    genLoading && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        onClick: cancel,
                                        className: "".concat(ApprovalRulesPanel_css/* cancelGenerateButton */.Oe, " ").concat(ApprovalRulesPanel_css/* headerButtonsHiddenOnMobile */.k),
                                        "data-testid": "cancel-generate-button",
                                        children: "Cancel"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        className: "".concat(ApprovalRulesPanel_css/* addButton */.c9, " ").concat(ApprovalRulesPanel_css/* headerButtonsHiddenOnMobile */.k),
                                        onClick: ()=>void exportRules(),
                                        disabled: exporting,
                                        "data-testid": "export-yaml-button",
                                        children: exporting ? "Exporting…" : "Export YAML"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        className: "".concat(ApprovalRulesPanel_css/* addButton */.c9, " ").concat(ApprovalRulesPanel_css/* headerButtonsHiddenOnMobile */.k),
                                        onClick: ()=>setImportModalOpen(true),
                                        "data-testid": "import-yaml-button",
                                        children: "Import YAML"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        onClick: refresh,
                                        className: ApprovalRulesPanel_css/* refreshButton */._c,
                                        disabled: loading,
                                        "aria-label": "Refresh rules",
                                        title: "Refresh rules",
                                        children: loading ? "⟳" : "↻"
                                    })
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: ApprovalRulesPanel_css/* subtitle */.VA,
                        children: "Rules are evaluated in priority order before requests reach the manual review queue."
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                "aria-live": "polite",
                "aria-atomic": "true",
                style: {
                    position: "absolute",
                    width: 1,
                    height: 1,
                    padding: 0,
                    overflow: "hidden",
                    clip: "rect(0,0,0,0)",
                    whiteSpace: "nowrap",
                    border: 0
                },
                children: genLiveMessage
            }),
            genError && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: ApprovalRulesPanel_css/* generateErrorBanner */.a9,
                role: "alert",
                "data-testid": "generate-error-banner",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        children: [
                            "Failed to generate suggestions: ",
                            genError.message
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: ApprovalRulesPanel_css/* dismissErrorButton */.ru,
                        onClick: clear,
                        "aria-label": "Dismiss error",
                        "data-testid": "dismiss-error-button",
                        children: "\xd7"
                    })
                ]
            }),
            exportError && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: ApprovalRulesPanel_css/* generateErrorBanner */.a9,
                role: "alert",
                "data-testid": "export-error-banner",
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                    children: [
                        "Export failed: ",
                        exportError.message
                    ]
                })
            }),
            !analyticsLoading && summary && total !== null && total > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: ApprovalRulesPanel_css/* analyticsBar */.ch,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: ApprovalRulesPanel_css/* analyticsTotal */.BX,
                        children: [
                            total.toLocaleString(),
                            " decisions"
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        style: {
                            color: "var(--text-muted)",
                            fontSize: 12
                        },
                        children: "last 7 days"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: "".concat(ApprovalRulesPanel_css/* analyticsRate */.Vw, " ").concat(ApprovalRulesPanel_css/* rateAllow */.iC),
                        children: [
                            autoAllowRate,
                            "% auto-allowed"
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: "".concat(ApprovalRulesPanel_css/* analyticsRate */.Vw, " ").concat(ApprovalRulesPanel_css/* rateManual */.hU),
                        children: [
                            manualRate,
                            "% manual review"
                        ]
                    }),
                    summary.topTools.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: ApprovalRulesPanel_css/* analyticsTopTool */.mm,
                        children: [
                            "Top tool: ",
                            summary.topTools[0].toolName
                        ]
                    })
                ]
            }),
            visibleSuggestions.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: ApprovalRulesPanel_css/* suggestionsContainer */.RO,
                "data-testid": "suggestions-container",
                children: visibleSuggestions.map((suggestion, i)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(SuggestedRuleCard/* SuggestedRuleCard */.S, {
                        suggestion: suggestion,
                        onAccept: handleSuggestionAccept,
                        onDiscard: ()=>handleSuggestionDiscard(suggestions.indexOf(suggestion))
                    }, i))
            }),
            !showTaggingRulesTab && /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                className: ApprovalRulesPanel_css/* searchBar */.ON,
                type: "search",
                placeholder: "Search by name, tool, program, pattern…",
                value: searchQuery,
                onChange: (e)=>setSearchQuery(e.target.value),
                "aria-label": "Search rules"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: ApprovalRulesPanel_css/* tabs */.vR,
                children: [
                    [
                        "all",
                        "user",
                        "config",
                        "seed",
                        "claude-settings"
                    ].map((src)=>{
                        const count = src === "all" ? rules.length : rules.filter((r)=>r.source === src).length;
                        const fullLabel = src === "all" ? "All" : sourceLabel(src);
                        const shortLabel = src === "claude-settings" ? "Settings" : src === "config" ? "Config" : fullLabel;
                        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                            className: "".concat(ApprovalRulesPanel_css/* tab */.Mf, " ").concat(!showTaggingRulesTab && sourceFilter === src ? ApprovalRulesPanel_css/* tabActive */.yR : ""),
                            onClick: ()=>{
                                setShowTaggingRulesTab(false);
                                setSourceFilter(src);
                            },
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: ApprovalRulesPanel_css/* tabLabelFull */.TY,
                                    children: fullLabel
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: ApprovalRulesPanel_css/* tabLabelShort */.sd,
                                    children: shortLabel
                                }),
                                " ",
                                "(",
                                count,
                                ")"
                            ]
                        }, src);
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: "".concat(ApprovalRulesPanel_css/* tab */.Mf, " ").concat(showTaggingRulesTab ? ApprovalRulesPanel_css/* tabActive */.yR : ""),
                        onClick: ()=>setShowTaggingRulesTab(true),
                        "data-testid": "tagging-rules-tab",
                        children: "Tagging Rules"
                    })
                ]
            }),
            showTaggingRulesTab ? /*#__PURE__*/ (0,jsx_runtime.jsx)(TaggingRulesPanel, {}) : renderApprovalRulesContent(),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(TemplateLibrary, {
                open: showTemplates,
                onClose: ()=>setShowTemplates(false),
                onSelect: handleTemplateSelect
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(ImportRulesModal, {
                open: importModalOpen,
                onClose: ()=>setImportModalOpen(false),
                onApplied: ()=>{
                    void refresh();
                },
                existingRules: rules
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/ruleBuilderPrefill.ts
var ruleBuilderPrefill = __webpack_require__(21235);
;// ./src/lib/hooks/useProgramAnalytics.ts
/* __next_internal_client_entry_do_not_use__ useProgramAnalytics auto */ 





function useProgramAnalytics(program, windowDays) {
    const clientRef = (0,react.useRef)(null);
    const [data, setData] = (0,react.useState)(null);
    const [isLoading, setIsLoading] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    (0,react.useEffect)(()=>{
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
    }, []);
    const fetch = (0,react.useCallback)(()=>{
        if (!program) {
            setData(null);
            setIsLoading(false);
            return ()=>{};
        }
        if (!clientRef.current) {
            return ()=>{};
        }
        const controller = new AbortController();
        setIsLoading(true);
        setError(null);
        const req = (0,create/* create */.v)(session_pb/* GetProgramAnalyticsRequestSchema */.nE9, {
            program,
            windowDays
        });
        clientRef.current.getProgramAnalytics(req, {
            signal: controller.signal
        }).then((resp)=>{
            setData(resp);
            setIsLoading(false);
        }).catch((err)=>{
            if (err instanceof Error && err.name === "AbortError") return;
            setData(null);
            setError(err instanceof Error ? err : new Error(String(err)));
            setIsLoading(false);
        });
        return ()=>controller.abort();
    }, [
        program,
        windowDays
    ]);
    (0,react.useEffect)(()=>{
        const cleanup = fetch();
        return cleanup;
    }, [
        fetch
    ]);
    return {
        data,
        isLoading,
        error,
        refresh: fetch
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FProgramDetailPanel.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA6VW25KiMBB99yvyslXrQ6YIc3OYj5kKpMHMhIQNQcWt_fetRNAAQnTmwVIgdp8-ffo0Dx8kT3cx-xOhvyuEUpp9FVo1kiVoR_VvjD_IGxxjfXxdv9vnSjPQCSLVAdVKcDY6Bd4prCnjTT0O9LhxZyrKGJfF5ClxT0uqCy6xUdXkQLR-X_1bPfS4icPNeF0J2iYoF3CwAajghcTcQFknKANpQNvbn01teN7iTEkD0iSormgGOAWzB5Be5lQZo8qryc_19WeWyegKnYkYH4flxK6cXEmDa36ECQDmYrrne-DF1iToJYrsvUwJpcfnR2Q9TposlQS_sf111ujahqsU77m7miBeambc3iCHlxHEZKt2oB3Q2ZKWhLoZxnsKEJreR-hJngYOBhtNZZ0rXSaoqSrQGa0ddQKMAY2tshwn0UP0DGVQW04Ji8onw8qeXWV7zsw2QSSKfnlcZ0oIWtWQoP7Xe5AFL_SLC-2qdIOUIAG5We709XIWRHPXFA0a9Gwb5MF9dXDvhXZX_luma-NgCF4bXJtWwGWaztCiS4u7i4lz2W_MuIbMcCVdA5vSWVNBJ5pwE-ZBeLuoPaclF22CSiWVc7llBdDAWBE2zwOBgAssNmDBFxCyXpALtceHBNHGKHtvv-UG3Hw5iveaVgMS6HAlcCm4BBzYDCFmzsVZnfzc5tLAut3Ns90MI2WBSO18pMMwEgtEImRBAqNZgO_0YE7g4e5cxVRevJpBpjQ9DdRpKH2s3sqZnG4kA23BD_6SXy0vFSr7sjm3nVGRuHL1llzizqi7O_Msl9fERE7_UnanmNbulNcBnuKGtyBnLCDZmekOSg_2sbueG-PjGg1td7tMglf003eLdp_o5JSzxfPAil-QSBzYuWN__XSppqbkHfmaf3WJA1pOb3gz9pdyPzledjGfnUQ_Sh967-rc4QYX_A-bFLlxdAwAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FProgramDetailPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA6VW25KiMBB99yvyslXrQ6YIc3OYj5kKpMHMhIQNQcWt_fetRNAAQnTmwVIgdp8_ffo0Dx8kT3cx_xOhvyuEUpp9FVo1kiVoR_VvjD_IGxxjfXxdv9vnSjPQCSLVAdVKcDY6Bd4prCnjTT0O9LhxZyrKGJfF5ClxT0uqCy6xUdXkQLR_X_1bPfS4icPNeF0J2iYoF3CwAajghcTcQFknKANpQNvbn01teN7iTEkD0iSormgGOAWzB5Be5lQZo8qryc_19WeWyegKnYkYH4flxK6cXEmDa36ECQDmYrrne_DF1iToJYrsvUwJpcfnR2Q9TposlQS_sf111ujahqsU77m7miBeambc3iCHlxHEZKt2oB3Q2ZKWhLoZxnsKEJreR_hJngYOBhtNZZ0rXSaoqSrQGa0ddQKMAY2tshwn0UP0DGVQW04Ji8onw8qeXWV7zsw2QSSKfnlcZ0oIWtWQoP7Xe5AFL_SLC_2qdIOUIAG5We709XIWRHPXFA0a9Gwb5MF9dXDvhXZX_luma_NgCF4bXJtWwGWaztCiS4u7i4lz2W_MuIbMcCVdA5vSWVNBJ5pwE_ZBeLuoPaclF22CSiWVc7llBdDAWBE2zwOBgAssNmDBFxCyXpALtceHBNHGKHtvv_UG3Hw5iveaVgMS6HAlcCm4BBzYDCFmzsVZnfzc5tLAut3Ns90MI2WBSO18pMMwEgtEImRBAqNZgO_0YE7g4e5cxVRevJpBpjQ9DdRpKH2s3sqZnG4kA23BD_6SXy0vFSr7sjm3nVGRuHL1llzizqi7O_Msl9fERE7_UnanmNbulNcBnuKGtyBnLCDZmekOSg_2sbueG_PjGg1td7tMglf003eLdp_o5JSzxfPAil_QSBzYuWN_XSppqbkHfmaf3WJA1pOb3gz9pdyPzledjGfnUQ_Sh967_rc4QYX_A_bFLlxdAwAAA_22_7D = __webpack_require__(15242);
;// ./src/components/sessions/ProgramDetailPanel.css.ts


var addRuleLink = '_1fbv2dqe';
var breakdownTable = '_1fbv2dq5';
var closeButton = '_1fbv2dq3';
var coverageNo = '_1fbv2dqd _1fbv2dqa';
var coveragePartial = '_1fbv2dqc _1fbv2dqa';
var coverageYes = '_1fbv2dqb _1fbv2dqa';
var errorState = '_1fbv2dql';
var exampleItem = '_1fbv2dq9';
var examplesList = '_1fbv2dq8';
var loadingState = '_1fbv2dqk';
var panel = '_1fbv2dq0';
var panelHeader = '_1fbv2dq1';
var panelTitle = '_1fbv2dq2';
var ProgramDetailPanel_css_sectionTitle = '_1fbv2dq4';
var sparklineBar = '_1fbv2dqf';
var td = '_1fbv2dq7';
var th = '_1fbv2dq6';
var trendBar = '_1fbv2dqh';
var trendDate = '_1fbv2dqi';
var trendRow = '_1fbv2dqg';
var trendSection = '_1fbv2dqj';
;// ./src/components/sessions/ProgramDetailPanel.tsx
// +feature: analytics-drill-down
/* __next_internal_client_entry_do_not_use__ ProgramDetailPanel auto */ 



function ProgramDetailPanel(param) {
    let { program, windowDays, onClose } = param;
    const { data, isLoading, error } = useProgramAnalytics(program, windowDays);
    const maxTotal = (0,react.useMemo)(()=>{
        var _data_subcommands;
        if (!(data === null || data === void 0 ? void 0 : (_data_subcommands = data.subcommands) === null || _data_subcommands === void 0 ? void 0 : _data_subcommands.length)) return 1;
        return Math.max(...data.subcommands.map((s)=>s.total), 1);
    }, [
        data === null || data === void 0 ? void 0 : data.subcommands
    ]);
    const totalForProgram = (0,react.useMemo)(()=>{
        var _data_subcommands;
        if (!(data === null || data === void 0 ? void 0 : (_data_subcommands = data.subcommands) === null || _data_subcommands === void 0 ? void 0 : _data_subcommands.length)) return 0;
        return data.subcommands.reduce((sum, s)=>sum + s.total, 0);
    }, [
        data === null || data === void 0 ? void 0 : data.subcommands
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: panel,
        "data-testid": "program-detail-panel",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: panelHeader,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: panelTitle,
                        children: [
                            program,
                            (data === null || data === void 0 ? void 0 : data.category) ? " \xb7 ".concat(data.category) : ""
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: closeButton,
                        onClick: onClose,
                        "aria-label": "Close program detail panel",
                        children: "✕"
                    })
                ]
            }),
            isLoading && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: loadingState,
                children: "Loading…"
            }),
            error && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: errorState,
                role: "alert",
                children: [
                    "Failed to load analytics: ",
                    error.message
                ]
            }),
            data && !isLoading && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: ProgramDetailPanel_css_sectionTitle,
                        children: "Subcommand Breakdown"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("table", {
                        className: breakdownTable,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("thead", {
                                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                            className: th,
                                            children: "Subcommand"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                            className: th,
                                            children: "Count"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                            className: th,
                                            children: "%"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                            className: th,
                                            children: "Allow"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                            className: th,
                                            children: "Deny"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                            className: th,
                                            children: "Manual"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                            className: th,
                                            children: "Coverage"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                            className: th
                                        })
                                    ]
                                })
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("tbody", {
                                children: [
                                    data.subcommands.map((row)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(SubcommandRow, {
                                            row: row,
                                            program: program,
                                            totalForProgram: totalForProgram,
                                            maxTotal: maxTotal
                                        }, row.subcommand || "(none)")),
                                    data.subcommands.length === 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("tr", {
                                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                            className: td,
                                            colSpan: 8,
                                            children: "No subcommand data for this program in the selected window."
                                        })
                                    })
                                ]
                            })
                        ]
                    }),
                    data.recentExamples.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: ProgramDetailPanel_css_sectionTitle,
                                children: "Recent Examples"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                                className: examplesList,
                                children: data.recentExamples.map((cmd, i)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("li", {
                                        className: exampleItem,
                                        children: cmd
                                    }, i))
                            })
                        ]
                    }),
                    data.trend.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: ProgramDetailPanel_css_sectionTitle,
                                children: [
                                    "Daily Activity (last ",
                                    data.trend.length,
                                    " days)"
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(TrendSparkline, {
                                buckets: data.trend
                            })
                        ]
                    })
                ]
            })
        ]
    });
}
function TrendSparkline(param) {
    let { buckets } = param;
    var _buckets_, _buckets_1;
    const maxTotal = Math.max(...buckets.map((b)=>b.total), 1);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: trendSection,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: trendRow,
                "aria-label": "Daily activity trend sparkline",
                children: buckets.map((b)=>{
                    const heightPct = Math.round(b.total / maxTotal * 100);
                    return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: trendBar,
                        style: {
                            height: "".concat(heightPct, "%")
                        },
                        title: "".concat(b.date, ": ").concat(b.total, " commands"),
                        "aria-hidden": "true"
                    }, b.date);
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: trendDate,
                children: [
                    (_buckets_ = buckets[0]) === null || _buckets_ === void 0 ? void 0 : _buckets_.date,
                    " – ",
                    (_buckets_1 = buckets[buckets.length - 1]) === null || _buckets_1 === void 0 ? void 0 : _buckets_1.date
                ]
            })
        ]
    });
}
function SubcommandRow(param) {
    let { row, program, totalForProgram, maxTotal } = param;
    const label = row.subcommand || "(none)";
    const pct = totalForProgram > 0 ? (row.total / totalForProgram * 100).toFixed(1) : "0.0";
    const barWidth = maxTotal > 0 ? Math.round(row.total / maxTotal * 80) : 0;
    const manual = row.escalate + row.manualAllow + row.manualDeny;
    // Determine three-state coverage: covered / partial / gap
    const hasEscalated = row.escalate > 0;
    let coverageCell;
    if (row.hasRuleCoverage && !hasEscalated) {
        coverageCell = /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
            className: coverageYes,
            "data-testid": "coverage-covered",
            children: "✓ covered"
        });
    } else if (row.hasRuleCoverage && hasEscalated) {
        coverageCell = /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
            className: coveragePartial,
            "data-testid": "coverage-partial",
            children: "⚠ partial"
        });
    } else {
        coverageCell = /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
            className: coverageNo,
            "data-testid": "coverage-gap",
            children: "✗ gap"
        });
    }
    // AC-13: "Add rule →" link pre-populates the rule form
    const addRuleHref = "/rules?program=".concat(encodeURIComponent(program), "&subcommand=").concat(encodeURIComponent(row.subcommand));
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                className: td,
                children: label
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("td", {
                className: td,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: sparklineBar,
                        style: {
                            width: "".concat(barWidth, "px")
                        },
                        "aria-hidden": "true"
                    }),
                    " ",
                    row.total
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("td", {
                className: td,
                children: [
                    pct,
                    "%"
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                className: td,
                children: row.autoAllow
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                className: td,
                children: row.autoDeny
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                className: td,
                children: manual
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                className: td,
                children: coverageCell
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                className: td,
                children: !row.hasRuleCoverage && /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                    href: addRuleHref,
                    className: addRuleLink,
                    children: "Add rule →"
                })
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/sessions/riskLevel.ts
var riskLevel = __webpack_require__(18969);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FApprovalAnalyticsPanel.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA91a227jKBi-n6fgZqVGGlcm53ofZkQMSZhicDFOnK723Vc-pYAx2GmrkfamqRzznw_f_5PnX_AY728pjsE_PwA4oPT1JEXJcQIuSD5F0S_4Qt6X8n23-Lv-XkhMZAJgXoFCMIqtt4j2ViQRpmWRALjMq_pxjjCm_JSAZdw-wLTIGbol4MhI86D-jDCVJFVU8ASkgpUZr785obw_9--P515o2Ag9l8zaorJ0U_ldFooeb1EquCJcJaDIUUqiA1FXQhpqiNETj6giWZGAlHBF5J1Jr3Qjy1XWj-q_BuNVwzhD8kR5AuLmdcFVVNB3koDluqNQP7oSejqrBOzi5rVUMCFtF8ULg_raSx2uWupOStCktAnExv7x2NhbobHNKwBjTfFO2m0nbSmLWtxc0N7ao6YAQEnEC9rGAGIMxM_LAhBUEFO75CwuRCZcqKcE0wIdGMGLgMpwb5nofrI5KHKUUnVLQPy80eXmQkWIMXEl2Di-dYdgE0nbcCDtvs9BW8tBm9pB66GDVvMcBEccBDcOD-1aD01xybTU2AdIZbolnASzcV5vJq-Xhtfdfn1d8OgBJ3kLxp58ctt8uRjx2iDwvJXNrsIo5BhTIS44cUi-tgJtlVd3baYGVTxQcGnJejAT7SQpbhSTFEeKZDlDikRtvygSIElOkHpCpRLRkTL2E2SUZ6h6gqs4r34CeJSLhVXyNWZpwDCHT_TU2DJXnZP3Mjm7sVqCt0XMF__lwh_EFzMJSJBgH87-KNJIHoMkqwDFm0nw1BDUG_D-gQYMAKOcROfudWjZ1eI2alvDdlMOwdg2z5RTlXnobJvgjmJ0E2w8JmhTvQUdkRIOtEUHPGAAiRjtdGcQ-20W11UnriJV3WfpievVy8NAF6crRKYbn7cG39c_xJeNOWjUQpmJAkHshFcbh5u306Emn4fDtZOihUsXIo9MXKMqAXWx_Ryc1MjnDfkrxepc18z4L7OvM5QXpJGr-c_ZHTVqb6bXX-qK6_A8I0c1y5owUKj0PhEdhFIiC9nleqaKRM3EUnfcAWJswZQutazlNN4pxtWdJYunVl6IVDRFrBcioxgzE_ypMUnDSpaG9_upM6M86p7tbShzSRgqVJSeKcNAGW2w17WFL9qZq7-Su-JAO10FSnrg-M1f2wOn32fXYn34Dq0MIHalqI3z6tjZDiuS3niOKKPsZpNflpPxPoQDRX3jr326XRD0NXkfAvAHj9p9pUvAmWJMzGIIVwYjq15ZlPTZqQlnED-v-uHJiHIb3cF1wHHLlb_rwtBCoAyc3wbOV4HzoXl3GYANMDQAwotNYG8SeDE7HuVN2z4wkb7akQwdAb8zyqgN6r9mYvC0mlDhhMizj-iHDGNGbFp9oZA0Wwhs56362wRAANs1lBabZj2CqZuvnXoTUytsyUEvsMCCLhueFPRm8urnSSjm4gCB46SsGT8_GHCc6L4DKh_L2o85fHa43sYDsZoUyZUXJdwHjRpxDcEfnDbO_I8UHs5W363wZVzhcpLC5acU_j2mcHg4bSl2tWwCDoWvrlnibsSdho7n4Ixv2g91enW69ti187IoVd2u9FVcVcvbKHKHuwOUoe9axla18DVJcoZSchYMdyvbRwZ7-JocRVoWwSVPZmG10QnZs8ZthgtMUiFRq2lvF2efNGHBxmKvrarvr0Enk5JjIms_mBQydwv0bWQHacH1UF3Z12_1HUts3LS49hY6PeGlFxtrBfcOZEqC5fM1H6xB91oauoP_W9JtPy_x7Unq7SNoH9i-QPnptd1boBJlhmlGNvgbyxdrzReOFb5RWu6cRwuLnHQJxC3TFF-00dQ5NoLnSBKuHg4Yp63iEVsZCmlmmBQcSlsAfixsjD3OY8HuH2XLWckcvmv_GJ9i7QbyS3dmjux9OCUvngFq75ifXFX3Ora0-LNprd8Afz6tr4_cvttZfh29fd_qUmJyRCWzJtRqkpXXf7xCbHzV1FBoboW4zYJMpQ6iZWcdG4AMF4s-kt-Ig123ymO_JdF_EhQ_MFhMiIA-xYzZa_y3LZYy9n3EMg65ur0V_A_6MOhf5SUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FApprovalAnalyticsPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA91a227jKBi_n6fgZqVGGlcm53ofZkQMSZhicDFOnK723Vc_pYAx2GmrkfamqRzznw_f_5PnX_AY728pjsE_PwA4oPT1JEXJcQIuSD5F0S_4Qt6X8n23_Lv_XkhMZAJgXoFCMIqtt4j2ViQRpmWRALjMq_pxjjCm_JSAZdw_wLTIGbol4MhI86D_jDCVJFVU8ASkgpUZr785obw_9_P515o2Ag9l8zaorJ0U_ldFooeb1EquCJcJaDIUUqiA1FXQhpqiNETj6giWZGAlHBF5J1Jr3Qjy1XWj_q_BuNVwzhD8kR5AuLmdcFVVNB3koDluqNQP7oSejqrBOzi5rVUMCFtF8ULg_raSx2uWupOStCktAnExv7x2NhbobHNKwBjTfFO2m0nbSmLWtxc0N7ao6YAQEnEC9rGAGIMxM_LAhBUEFO75CwuRCZcqKcE0wIdGMGLgMpwb5nofrI5KHKUUnVLQPy80eXmQkWIMXEl2Di_dYdgE0nbcCDtvs9BW8tBm9pB66GDVvMcBEccBDcOD_1aD01xybTU2AdIZbolnASzcV5vJq_Xhtfdfn1d8OgBJ3kLxp58ctt8uRjx2iDwvJXNrsIo5BhTIS44cUi_tgJtlVd3baYGVTxQcGnJejAT7SQpbhSTFEeKZDlDikRtvygSIElOkHpCpRLRkTL2E2SUZ6h6gqs4r34CeJSLhVXyNWZpwDCHT_TU2DJXnZP3Mjm7sVqCt0XMF_lwh_EFzMJSJBgH87_KNJIHoMkqwDFm0nw1BDUG_D_gQYMAKOcROfudWjZ1eI2alvDdlMOwdg2z5RTlXnobJvgjmJ0E2w8JmhTvQUdkRIOtEUHPGAAiRjtdGcQ_20W11UnriJV3WfpievVy8NAF6crRKYbn7cG39c_xJeNOWjUQpmJAkHshFcbh5u306Emn4fDtZOihUsXIo9MXKMqAXWx_Ryc1MjnDfkrxepc18z4L7OvM5QXpJGr_c_ZHTVqb6bXX_qK6_A8I0c1y5owUKj0PhEdhFIiC9nleqaKRM3EUnfcAWJswZQutazlNN4pxtWdJYunVl6IVDRFrBcioxgzE_ypMUnDSpaG9_upM6M86p7tbShzSRgqVJSeKcNAGW2w17WFL9qZq7_Su_JAO10FSnrg_M1f2wOn32fXYn34Dq0MIHalqI3z6tjZDiuS3niOKKPsZpNflpPxPoQDRX3jr326XRD0NXkfAvAHj9p9pUvAmWJMzGIIVwYjq15ZlPTZqQlnED_v_uHJiHIb3cF1wHHLlb_rwtBCoAyc3wbOV4HzoXl3GYANMDQAwotNYG8SeDE7HuVN2z4wkb7akQwdAb8zyqgN6r9mYvC0mlDhhMizj_iHDGNGbFp9oZA0Wwhs56362wRAANs1lBabZj2CqZuvnXoTUytsyUEvsMCCLhueFPRm8urnSSjm4gCB46SsGT8_GHCc6L4DKh_L2o85fHa43sYDsZoUyZUXJdwHjRpxDcEfnDbO_I8UHs5W363wZVzhcpLC5acU_j2mcHg4bSl2tWwCDoWvrlnibsSdho7n4Ixv2g91enW69ti187IoVd2u9FVcVcvbKHKHuwOUoe9axla18DVJcoZSchYMdyvbRwZ7_JocRVoWwSVPZmG10QnZs8ZthgtMUiFRq2lvF2efNGHBxmKvrarvr0Enk5JjIms_mBQydwv0bWQHacH1UF3Z12_1HUts3LS49hY6PeGlFxtrBfcOZEqC5fM1H6xB91oauoP_W9JtPy_x7Unq7SNoH9i_QPnptd1boBJlhmlGNvgbyxdrzReOFb5RWu6cRwuLnHQJxC3TFF_00dQ5NoLnSBKuHg4Yp63iEVsZCmlmmBQcSlsAfixsjD3OY8HuH2XLWckcvmv_GJ9i7QbyS3dmjux9OCUvngFq75ifXFX3Ora0_LNprd8Afz6tr4_cvttZfh29fd_qUmJyRCWzJtRqkpXXf7xCbHzV1FBoboW4zYJMpQ6iZWcdG4AMF4s_kt_Ig123ymO_JdF_EhQ_MFhMiIA_xYzZa_y3LZYy9n3EMg65ur0V_A_6MOhf5SUAAA_22_7D = __webpack_require__(32675);
;// ./src/components/sessions/ApprovalAnalyticsPanel.css.ts


var ApprovalAnalyticsPanel_css_addRuleLink = '_1f08ycd1l';
var allowCount = '_1f08ycdw';
var barCmd = '_1f08ycd16';
var barFill = '_1f08ycd13';
var barGap = '_1f08ycd18';
var barPython = '_1f08ycd17';
var barRule = '_1f08ycd15';
var barTool = '_1f08ycd14';
var barTrack = '_1f08ycd12';
var bulkActionBar = '_1f08ycd1p';
var bulkActionCount = '_1f08ycd1q';
var bulkAddBtn = '_1f08ycd1r';
var bulkClearBtn = '_1f08ycd1s';
var bulkDiscardBtn = '_1f08ycd1x';
var bulkResultMsg = '_1f08ycd1y';
var bulkReviewActions = '_1f08ycd1v';
var bulkReviewHeader = '_1f08ycd1u';
var bulkReviewPanel = '_1f08ycd1t';
var bulkSaveBtn = '_1f08ycd1w';
var ApprovalAnalyticsPanel_css_card = '_1f08ycdc';
var cardAllow = '_1f08ycdd';
var cardDeny = '_1f08ycde';
var cardLabel = '_1f08ycdh';
var cardManual = '_1f08ycdf';
var cardSub = '_1f08ycdi';
var cardValue = '_1f08ycdg';
var cards = '_1f08ycdb';
var categoryBadge = '_1f08ycd19';
var checkboxTd = '_1f08ycd1o';
var checkboxTh = '_1f08ycd1n';
var decisionSelect = '_1f08ycd1z';
var denyCount = '_1f08ycdx';
var ApprovalAnalyticsPanel_css_empty = '_1f08ycdk';
var emptyHint = '_1f08ycdl';
var ApprovalAnalyticsPanel_css_error = '_1f08ycd9';
var filterInput = '_1f08ycd1k';
var gapBadgeDesc = '_1f08ycd1j';
var gapBadgeHigh = '_1f08ycd1g';
var gapBadgeLow = '_1f08ycd1i';
var gapBadgeMed = '_1f08ycd1h';
var header = '_1f08ycd1';
var ApprovalAnalyticsPanel_css_loading = '_1f08ycdj';
var manualCount = '_1f08ycdy';
var ApprovalAnalyticsPanel_css_panel = '_1f08ycd0';
var pctLabel = '_1f08ycdz';
var refreshButton = '_1f08ycd5';
var removeEntryBtn = '_1f08ycd20';
var retryButton = '_1f08ycda';
var row = '_1f08ycdv';
var rowActions = '_1f08ycd1m';
var ruleName = '_1f08ycd11';
var ApprovalAnalyticsPanel_css_sectionTitle = '_1f08ycdm';
var stackedAllow = '_1f08ycd1d';
var stackedBarTrack = '_1f08ycd1c';
var stackedDeny = '_1f08ycd1e';
var stackedManual = '_1f08ycd1f';
var subtitle = '_1f08ycd4';
var table = '_1f08ycdp';
var tableSection = '_1f08ycdn';
var tableWrapper = '_1f08ycdo';
var ApprovalAnalyticsPanel_css_td = '_1f08ycds';
var tdBar = '_1f08ycdu';
var tdRight = '_1f08ycdt';
var ApprovalAnalyticsPanel_css_th = '_1f08ycdq';
var thRight = '_1f08ycdr';
var title = '_1f08ycd3';
var titleRow = '_1f08ycd2';
var toolName = '_1f08ycd10';
var twoColCell = '_1f08ycd1b';
var twoColGrid = '_1f08ycd1a';
var windowBtn = '_1f08ycd7';
var windowBtnActive = '_1f08ycd8';
var windowSelector = '_1f08ycd6';
;// ./src/components/sessions/ApprovalAnalyticsPanel.tsx
/* __next_internal_client_entry_do_not_use__ ApprovalAnalyticsPanel auto */ 
// +feature: approval-analytics-risk-breakdown










// ── helpers ───────────────────────────────────────────────────────────────────
function pct(count, total) {
    if (total === 0) return 0;
    return Math.round(count / total * 100);
}
function formatDate(iso) {
    // "2006-01-02" → "Jan 15"
    try {
        const d = new Date(iso + "T00:00:00");
        return d.toLocaleDateString(undefined, {
            month: "short",
            day: "numeric"
        });
    } catch (e) {
        return iso;
    }
}
// Simple inline bar component — no charting library required.
function Bar(param) {
    let { value, max, className } = param;
    const width = max === 0 ? 0 : Math.round(value / max * 100);
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: barTrack,
        "aria-hidden": "true",
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: "".concat(barFill, " ").concat(className),
            style: {
                width: "".concat(width, "%")
            }
        })
    });
}
// Stacked bar showing allow/deny/manual composition scaled to max total.
function StackedBar(param) {
    let { allow, deny, manual, total } = param;
    const rowTotal = allow + deny + manual;
    const scale = total === 0 ? 0 : rowTotal / total;
    const ap = Math.round(scale * (allow / (rowTotal || 1)) * 100);
    const dp = Math.round(scale * (deny / (rowTotal || 1)) * 100);
    const mp = Math.round(scale * (manual / (rowTotal || 1)) * 100);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: stackedBarTrack,
        "aria-hidden": "true",
        children: [
            ap > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: stackedAllow,
                style: {
                    width: "".concat(ap, "%")
                }
            }),
            dp > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: stackedDeny,
                style: {
                    width: "".concat(dp, "%")
                }
            }),
            mp > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: stackedManual,
                style: {
                    width: "".concat(mp, "%")
                }
            })
        ]
    });
}
// ── component ─────────────────────────────────────────────────────────────────
const WINDOW_OPTIONS = [
    {
        label: "7 days",
        value: 7
    },
    {
        label: "14 days",
        value: 14
    },
    {
        label: "30 days",
        value: 30
    },
    {
        label: "90 days",
        value: 90
    }
];
// Maps `escalation_category` values (see EscalationCategory in the backend classifier) to the
// human-readable labels shown in the "Escalation Reasons" table. Callers must fall back to the
// raw category string for unmapped keys — see the `?? category` usage below — so an unrecognized
// category never renders the literal string "undefined".
const ESCALATION_CATEGORY_LABELS = {
    "no-match": "No auto-approval rule matched",
    "explicit-rule": "Rule explicitly flagged for review",
    "domain-age": "Newly-registered domain",
    "secret-scan": "Plaintext secret detected",
    "unclassifiable": "Shell expansion — couldn't classify",
    "unexpected": "Internal classification error"
};
// Maps `risk_level` values (classifier.RiskLevel string form) to the human-readable labels
// shown in the "Risk Level Breakdown" table, sourced from riskLevel.ts's shared base labels
// (also consumed by SeverityBadge.tsx). Same fallback contract as ESCALATION_CATEGORY_LABELS
// above — unmapped keys render the raw string, never "undefined" — which is why this is typed
// as Record<string, string> here rather than the narrower Record<RiskLevel, string>.
const RISK_LEVEL_LABELS = riskLevel/* RISK_LEVEL_LABELS */.p;
/**
 * ApprovalAnalyticsPanel displays time-series and aggregate data for
 * auto-approval classification decisions.
 *
 * Shows:
 * - Window selector (7 / 14 / 30 / 90 days)
 * - Summary cards: total, auto-allow rate, manual review rate, avg/day
 * - Day-by-day breakdown table with inline bar charts
 * - Top tools and top triggered rules
 */ function ApprovalAnalyticsPanel() {
    var _escalationReasonRows_, _riskLevelRows_;
    const [windowDays, setWindowDays] = (0,react.useState)(7);
    const [selectedProgram, setSelectedProgram] = (0,react.useState)(null);
    const { summary, dailyBuckets, loading, error, refresh } = useApprovalAnalytics({
        windowDays
    });
    const { bulkUpsertRules } = (0,useApprovalRules/* useApprovalRules */.I)();
    var _summary_totalDecisions;
    const total = (_summary_totalDecisions = summary === null || summary === void 0 ? void 0 : summary.totalDecisions) !== null && _summary_totalDecisions !== void 0 ? _summary_totalDecisions : 0;
    var _summary_decisionCounts_auto_allow;
    const autoAllowCount = (_summary_decisionCounts_auto_allow = summary === null || summary === void 0 ? void 0 : summary.decisionCounts["auto_allow"]) !== null && _summary_decisionCounts_auto_allow !== void 0 ? _summary_decisionCounts_auto_allow : 0;
    var _summary_decisionCounts_auto_deny;
    const autoDenyCount = (_summary_decisionCounts_auto_deny = summary === null || summary === void 0 ? void 0 : summary.decisionCounts["auto_deny"]) !== null && _summary_decisionCounts_auto_deny !== void 0 ? _summary_decisionCounts_auto_deny : 0;
    var _summary_decisionCounts_escalate, _summary_decisionCounts_manual_allow, _summary_decisionCounts_manual_deny;
    const escalateCount = ((_summary_decisionCounts_escalate = summary === null || summary === void 0 ? void 0 : summary.decisionCounts["escalate"]) !== null && _summary_decisionCounts_escalate !== void 0 ? _summary_decisionCounts_escalate : 0) + ((_summary_decisionCounts_manual_allow = summary === null || summary === void 0 ? void 0 : summary.decisionCounts["manual_allow"]) !== null && _summary_decisionCounts_manual_allow !== void 0 ? _summary_decisionCounts_manual_allow : 0) + ((_summary_decisionCounts_manual_deny = summary === null || summary === void 0 ? void 0 : summary.decisionCounts["manual_deny"]) !== null && _summary_decisionCounts_manual_deny !== void 0 ? _summary_decisionCounts_manual_deny : 0);
    const autoAllowRate = pct(autoAllowCount, total);
    const autoDenyRate = pct(autoDenyCount, total);
    const manualRate = pct(escalateCount, total);
    const avgPerDay = dailyBuckets.length > 0 ? Math.round(total / windowDays) : 0;
    var _summary_decisionCounts_manual_allow1;
    const manualAllow = (_summary_decisionCounts_manual_allow1 = summary === null || summary === void 0 ? void 0 : summary.decisionCounts["manual_allow"]) !== null && _summary_decisionCounts_manual_allow1 !== void 0 ? _summary_decisionCounts_manual_allow1 : 0;
    var _summary_decisionCounts_manual_deny1;
    const manualDeny = (_summary_decisionCounts_manual_deny1 = summary === null || summary === void 0 ? void 0 : summary.decisionCounts["manual_deny"]) !== null && _summary_decisionCounts_manual_deny1 !== void 0 ? _summary_decisionCounts_manual_deny1 : 0;
    const manualTotal = manualAllow + manualDeny;
    const manualAllowPct = manualTotal > 0 ? Math.round(manualAllow / manualTotal * 100) : null;
    // Max total across days — used to scale inline bars.
    const maxDayTotal = dailyBuckets.reduce((m, b)=>Math.max(m, b.total), 0);
    // Non-zero escalation-category counts, sorted descending — drives the "Escalation Reasons" table.
    const escalationReasonRows = (0,react.useMemo)(()=>{
        var _summary_escalationReasonCounts;
        const counts = (_summary_escalationReasonCounts = summary === null || summary === void 0 ? void 0 : summary.escalationReasonCounts) !== null && _summary_escalationReasonCounts !== void 0 ? _summary_escalationReasonCounts : {};
        return Object.entries(counts).filter((param)=>{
            let [, count] = param;
            return count > 0;
        }).sort((a, b)=>b[1] - a[1]);
    }, [
        summary === null || summary === void 0 ? void 0 : summary.escalationReasonCounts
    ]);
    var _escalationReasonRows__;
    const escalationReasonMaxCount = (_escalationReasonRows__ = (_escalationReasonRows_ = escalationReasonRows[0]) === null || _escalationReasonRows_ === void 0 ? void 0 : _escalationReasonRows_[1]) !== null && _escalationReasonRows__ !== void 0 ? _escalationReasonRows__ : 1;
    // Non-zero risk-level counts, sorted descending — drives the "Risk Level Breakdown" table.
    // Scoped to escalations only, matching escalationReasonCounts (see analytics_store.go's
    // ComputeSummary) — the two tables share a denominator.
    const riskLevelRows = (0,react.useMemo)(()=>{
        var _summary_riskLevelCounts;
        const counts = (_summary_riskLevelCounts = summary === null || summary === void 0 ? void 0 : summary.riskLevelCounts) !== null && _summary_riskLevelCounts !== void 0 ? _summary_riskLevelCounts : {};
        return Object.entries(counts).filter((param)=>{
            let [, count] = param;
            return count > 0;
        }).sort((a, b)=>b[1] - a[1]);
    }, [
        summary === null || summary === void 0 ? void 0 : summary.riskLevelCounts
    ]);
    var _riskLevelRows__;
    const riskLevelMaxCount = (_riskLevelRows__ = (_riskLevelRows_ = riskLevelRows[0]) === null || _riskLevelRows_ === void 0 ? void 0 : _riskLevelRows_[1]) !== null && _riskLevelRows__ !== void 0 ? _riskLevelRows__ : 1;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: ApprovalAnalyticsPanel_css_panel,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: titleRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                        className: title,
                        children: "Approval Analytics"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        style: {
                            display: "flex",
                            alignItems: "center",
                            gap: 8
                        },
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: windowSelector,
                                role: "group",
                                "aria-label": "Time window",
                                children: WINDOW_OPTIONS.map((opt)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        className: "".concat(windowBtn, " ").concat(windowDays === opt.value ? windowBtnActive : ""),
                                        onClick: ()=>setWindowDays(opt.value),
                                        "aria-pressed": windowDays === opt.value,
                                        children: opt.label
                                    }, opt.value))
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                onClick: refresh,
                                className: refreshButton,
                                disabled: loading,
                                "aria-label": "Refresh analytics",
                                children: loading ? "⟳" : "↻"
                            })
                        ]
                    })
                ]
            }),
            error && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: ApprovalAnalyticsPanel_css_error,
                role: "alert",
                children: [
                    "Failed to load analytics: ",
                    error.message,
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        onClick: refresh,
                        className: retryButton,
                        children: "Retry"
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: cards,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: ApprovalAnalyticsPanel_css_card,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: cardValue,
                                children: total
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: cardLabel,
                                children: "Total decisions"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: cardSub,
                                children: [
                                    avgPerDay,
                                    "/day avg"
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: "".concat(ApprovalAnalyticsPanel_css_card, " ").concat(cardAllow),
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: cardValue,
                                children: [
                                    autoAllowRate,
                                    "%"
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: cardLabel,
                                children: "Auto-allowed"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: cardSub,
                                children: [
                                    autoAllowCount,
                                    " requests"
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: "".concat(ApprovalAnalyticsPanel_css_card, " ").concat(cardDeny),
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: cardValue,
                                children: [
                                    autoDenyRate,
                                    "%"
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: cardLabel,
                                children: "Auto-denied"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: cardSub,
                                children: [
                                    autoDenyCount,
                                    " requests"
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: "".concat(ApprovalAnalyticsPanel_css_card, " ").concat(cardManual),
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: cardValue,
                                children: [
                                    manualRate,
                                    "%"
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: cardLabel,
                                children: "Manual review"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: cardSub,
                                children: [
                                    escalateCount,
                                    " requests",
                                    manualAllowPct !== null && " \xb7 ".concat(manualAllowPct, "% allowed")
                                ]
                            })
                        ]
                    })
                ]
            }),
            loading && dailyBuckets.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: ApprovalAnalyticsPanel_css_loading,
                children: "Loading analytics…"
            }) : dailyBuckets.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: ApprovalAnalyticsPanel_css_empty,
                children: [
                    "No data for the last ",
                    windowDays,
                    " days.",
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("br", {}),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: emptyHint,
                        children: "Analytics are recorded when Claude Code sends hook requests."
                    })
                ]
            }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: tableSection,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: ApprovalAnalyticsPanel_css_sectionTitle,
                        children: "Daily Breakdown"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: tableWrapper,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("table", {
                            className: table,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("thead", {
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                className: ApprovalAnalyticsPanel_css_th,
                                                children: "Date"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                className: "".concat(ApprovalAnalyticsPanel_css_th, " ").concat(thRight),
                                                children: "Total"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                className: "".concat(ApprovalAnalyticsPanel_css_th, " ").concat(thRight),
                                                children: "Allow"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                className: "".concat(ApprovalAnalyticsPanel_css_th, " ").concat(thRight),
                                                children: "Deny"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                className: "".concat(ApprovalAnalyticsPanel_css_th, " ").concat(thRight),
                                                children: "Manual"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                className: ApprovalAnalyticsPanel_css_th,
                                                children: "Volume"
                                            })
                                        ]
                                    })
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("tbody", {
                                    children: [
                                        ...dailyBuckets
                                    ].reverse().map((b)=>{
                                        const manualTotal = b.escalate + b.manualAllow + b.manualDeny;
                                        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                            className: row,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: ApprovalAnalyticsPanel_css_td,
                                                    children: formatDate(b.date)
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: "".concat(ApprovalAnalyticsPanel_css_td, " ").concat(tdRight),
                                                    children: b.total
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("td", {
                                                    className: "".concat(ApprovalAnalyticsPanel_css_td, " ").concat(tdRight),
                                                    children: [
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                            className: allowCount,
                                                            children: b.autoAllow
                                                        }),
                                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                            className: pctLabel,
                                                            children: [
                                                                " ",
                                                                pct(b.autoAllow, b.total),
                                                                "%"
                                                            ]
                                                        })
                                                    ]
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("td", {
                                                    className: "".concat(ApprovalAnalyticsPanel_css_td, " ").concat(tdRight),
                                                    children: [
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                            className: denyCount,
                                                            children: b.autoDeny
                                                        }),
                                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                            className: pctLabel,
                                                            children: [
                                                                " ",
                                                                pct(b.autoDeny, b.total),
                                                                "%"
                                                            ]
                                                        })
                                                    ]
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("td", {
                                                    className: "".concat(ApprovalAnalyticsPanel_css_td, " ").concat(tdRight),
                                                    children: [
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                            className: manualCount,
                                                            children: manualTotal
                                                        }),
                                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                            className: pctLabel,
                                                            children: [
                                                                " ",
                                                                pct(manualTotal, b.total),
                                                                "%"
                                                            ]
                                                        })
                                                    ]
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: "".concat(ApprovalAnalyticsPanel_css_td, " ").concat(tdBar),
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(StackedBar, {
                                                        allow: b.autoAllow,
                                                        deny: b.autoDeny,
                                                        manual: manualTotal,
                                                        total: maxDayTotal
                                                    })
                                                })
                                            ]
                                        }, b.date);
                                    })
                                })
                            ]
                        })
                    })
                ]
            }),
            summary && (summary.topTools.length > 0 || summary.topTriggeredRules.length > 0) && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: twoColGrid,
                children: [
                    summary.topTools.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: "".concat(tableSection, " ").concat(twoColCell),
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                                className: ApprovalAnalyticsPanel_css_sectionTitle,
                                children: "Top Tools"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: tableWrapper,
                                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("table", {
                                    className: table,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("thead", {
                                            children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                        className: ApprovalAnalyticsPanel_css_th,
                                                        children: "Tool"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                        className: "".concat(ApprovalAnalyticsPanel_css_th, " ").concat(thRight),
                                                        children: "Requests"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                        className: ApprovalAnalyticsPanel_css_th,
                                                        children: "Share"
                                                    })
                                                ]
                                            })
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("tbody", {
                                            children: summary.topTools.map((t)=>{
                                                var _summary_topTools_;
                                                var _summary_topTools__count;
                                                return /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                                    className: row,
                                                    children: [
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                            className: ApprovalAnalyticsPanel_css_td,
                                                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                                                                className: toolName,
                                                                children: t.toolName
                                                            })
                                                        }),
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                            className: "".concat(ApprovalAnalyticsPanel_css_td, " ").concat(tdRight),
                                                            children: t.count
                                                        }),
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                            className: "".concat(ApprovalAnalyticsPanel_css_td, " ").concat(tdBar),
                                                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(Bar, {
                                                                value: t.count,
                                                                max: (_summary_topTools__count = (_summary_topTools_ = summary.topTools[0]) === null || _summary_topTools_ === void 0 ? void 0 : _summary_topTools_.count) !== null && _summary_topTools__count !== void 0 ? _summary_topTools__count : 1,
                                                                className: barTool
                                                            })
                                                        })
                                                    ]
                                                }, t.toolName);
                                            })
                                        })
                                    ]
                                })
                            })
                        ]
                    }),
                    summary.topTriggeredRules.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: "".concat(tableSection, " ").concat(twoColCell),
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                                className: ApprovalAnalyticsPanel_css_sectionTitle,
                                children: "Top Triggered Rules"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: tableWrapper,
                                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("table", {
                                    className: table,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("thead", {
                                            children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                        className: ApprovalAnalyticsPanel_css_th,
                                                        children: "Rule"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                        className: "".concat(ApprovalAnalyticsPanel_css_th, " ").concat(thRight),
                                                        children: "Triggers"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                        className: ApprovalAnalyticsPanel_css_th,
                                                        children: "Frequency"
                                                    })
                                                ]
                                            })
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("tbody", {
                                            children: summary.topTriggeredRules.map((r)=>{
                                                var _summary_topTriggeredRules_;
                                                var _summary_topTriggeredRules__count;
                                                return /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                                    className: row,
                                                    children: [
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                            className: ApprovalAnalyticsPanel_css_td,
                                                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                className: ruleName,
                                                                children: r.ruleName || r.ruleId
                                                            })
                                                        }),
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                            className: "".concat(ApprovalAnalyticsPanel_css_td, " ").concat(tdRight),
                                                            children: r.count
                                                        }),
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                            className: "".concat(ApprovalAnalyticsPanel_css_td, " ").concat(tdBar),
                                                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(Bar, {
                                                                value: r.count,
                                                                max: (_summary_topTriggeredRules__count = (_summary_topTriggeredRules_ = summary.topTriggeredRules[0]) === null || _summary_topTriggeredRules_ === void 0 ? void 0 : _summary_topTriggeredRules_.count) !== null && _summary_topTriggeredRules__count !== void 0 ? _summary_topTriggeredRules__count : 1,
                                                                className: barRule
                                                            })
                                                        })
                                                    ]
                                                }, r.ruleId);
                                            })
                                        })
                                    ]
                                })
                            })
                        ]
                    })
                ]
            }),
            summary && (escalationReasonRows.length > 0 ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: tableSection,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: ApprovalAnalyticsPanel_css_sectionTitle,
                        children: "Escalation Reasons"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: tableWrapper,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("table", {
                            className: table,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("thead", {
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                className: ApprovalAnalyticsPanel_css_th,
                                                children: "Reason"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                className: "".concat(ApprovalAnalyticsPanel_css_th, " ").concat(thRight),
                                                children: "Count"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                className: ApprovalAnalyticsPanel_css_th,
                                                children: "Frequency"
                                            })
                                        ]
                                    })
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("tbody", {
                                    children: escalationReasonRows.map((param)=>{
                                        let [category, count] = param;
                                        var _ESCALATION_CATEGORY_LABELS_category;
                                        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                            className: row,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: ApprovalAnalyticsPanel_css_td,
                                                    children: (_ESCALATION_CATEGORY_LABELS_category = ESCALATION_CATEGORY_LABELS[category]) !== null && _ESCALATION_CATEGORY_LABELS_category !== void 0 ? _ESCALATION_CATEGORY_LABELS_category : category
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: "".concat(ApprovalAnalyticsPanel_css_td, " ").concat(tdRight),
                                                    children: count
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: "".concat(ApprovalAnalyticsPanel_css_td, " ").concat(tdBar),
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(Bar, {
                                                        value: count,
                                                        max: escalationReasonMaxCount,
                                                        className: barRule
                                                    })
                                                })
                                            ]
                                        }, category);
                                    })
                                })
                            ]
                        })
                    })
                ]
            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: ApprovalAnalyticsPanel_css_empty,
                children: "No escalations in this window."
            })),
            summary && (riskLevelRows.length > 0 ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: tableSection,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: ApprovalAnalyticsPanel_css_sectionTitle,
                        children: "Risk Level Breakdown"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: tableWrapper,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("table", {
                            className: table,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("thead", {
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                className: ApprovalAnalyticsPanel_css_th,
                                                children: "Risk Level"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                className: "".concat(ApprovalAnalyticsPanel_css_th, " ").concat(thRight),
                                                children: "Count"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                className: ApprovalAnalyticsPanel_css_th,
                                                children: "Frequency"
                                            })
                                        ]
                                    })
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("tbody", {
                                    children: riskLevelRows.map((param)=>{
                                        let [level, count] = param;
                                        var _RISK_LEVEL_LABELS_level;
                                        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                            className: row,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: ApprovalAnalyticsPanel_css_td,
                                                    children: (_RISK_LEVEL_LABELS_level = RISK_LEVEL_LABELS[level]) !== null && _RISK_LEVEL_LABELS_level !== void 0 ? _RISK_LEVEL_LABELS_level : level
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: "".concat(ApprovalAnalyticsPanel_css_td, " ").concat(tdRight),
                                                    children: count
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: "".concat(ApprovalAnalyticsPanel_css_td, " ").concat(tdBar),
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(Bar, {
                                                        value: count,
                                                        max: riskLevelMaxCount,
                                                        className: barRule
                                                    })
                                                })
                                            ]
                                        }, level);
                                    })
                                })
                            ]
                        })
                    })
                ]
            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: ApprovalAnalyticsPanel_css_empty,
                children: "No escalations in this window."
            })),
            summary && summary.topPythonImports.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: tableSection,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: ApprovalAnalyticsPanel_css_sectionTitle,
                        children: "Top Python Imports"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: tableWrapper,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("table", {
                            className: table,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("thead", {
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                className: ApprovalAnalyticsPanel_css_th,
                                                children: "Module"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                className: "".concat(ApprovalAnalyticsPanel_css_th, " ").concat(thRight),
                                                children: "Uses"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                className: ApprovalAnalyticsPanel_css_th,
                                                children: "Share"
                                            })
                                        ]
                                    })
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("tbody", {
                                    children: summary.topPythonImports.map((imp)=>{
                                        var _summary_topPythonImports_;
                                        var _summary_topPythonImports__count;
                                        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                            className: row,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: ApprovalAnalyticsPanel_css_td,
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                                                        className: toolName,
                                                        children: imp.module
                                                    })
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: "".concat(ApprovalAnalyticsPanel_css_td, " ").concat(tdRight),
                                                    children: imp.count
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: "".concat(ApprovalAnalyticsPanel_css_td, " ").concat(tdBar),
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(Bar, {
                                                        value: imp.count,
                                                        max: (_summary_topPythonImports__count = (_summary_topPythonImports_ = summary.topPythonImports[0]) === null || _summary_topPythonImports_ === void 0 ? void 0 : _summary_topPythonImports_.count) !== null && _summary_topPythonImports__count !== void 0 ? _summary_topPythonImports__count : 1,
                                                        className: barPython
                                                    })
                                                })
                                            ]
                                        }, imp.module);
                                    })
                                })
                            ]
                        })
                    })
                ]
            }),
            summary && (summary.commandSubcommandStats.length > 0 || summary.topTools.length > 0 || summary.topUncoveredTools.length > 0) && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: tableSection,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        style: {
                            display: "flex",
                            alignItems: "baseline",
                            gap: 4,
                            marginBottom: 12,
                            flexWrap: "wrap"
                        },
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                                className: ApprovalAnalyticsPanel_css_sectionTitle,
                                style: {
                                    margin: 0
                                },
                                children: "Activity"
                            }),
                            summary.coverageGapCount > 0 && (()=>{
                                const rounded = Math.round(summary.coverageGapRate);
                                const cls = rounded >= 30 ? gapBadgeHigh : rounded >= 10 ? gapBadgeMed : gapBadgeLow;
                                return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                            className: cls,
                                            children: [
                                                rounded,
                                                "% uncovered"
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                            className: gapBadgeDesc,
                                            children: [
                                                summary.coverageGapCount,
                                                " of ",
                                                total,
                                                " decisions had no matching rule"
                                            ]
                                        })
                                    ]
                                });
                            })()
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(UnifiedActivityTable, {
                        commandStats: summary.commandSubcommandStats,
                        uncoveredToolNames: new Set(summary.topUncoveredTools.map((t)=>t.toolName)),
                        uncoveredProgramNames: new Set(summary.topUncoveredPrograms.map((p)=>p.programName)),
                        allToolNames: summary.topTools,
                        windowDays: windowDays,
                        selectedProgram: selectedProgram,
                        onSelectProgram: setSelectedProgram,
                        bulkUpsert: bulkUpsertRules,
                        onRuleAccepted: refresh
                    })
                ]
            })
        ]
    });
}
// ── BulkReviewPanel ───────────────────────────────────────────────────────────
function BulkReviewPanel(param) {
    let { entries, onEntriesChange, bulkUpsert, onDone } = param;
    const [saving, setSaving] = (0,react.useState)(false);
    const [result, setResult] = (0,react.useState)("");
    const setDecision = (0,react.useCallback)((key, decision)=>{
        onEntriesChange(entries.map((e)=>e.key === key ? {
                ...e,
                decision
            } : e));
    }, [
        entries,
        onEntriesChange
    ]);
    const removeEntry = (0,react.useCallback)((key)=>{
        const next = entries.filter((e)=>e.key !== key);
        if (next.length === 0) {
            onDone();
            return;
        }
        onEntriesChange(next);
    }, [
        entries,
        onEntriesChange,
        onDone
    ]);
    const handleSave = (0,react.useCallback)(async ()=>{
        setSaving(true);
        setResult("");
        try {
            const rules = entries.map((e)=>({
                    id: "",
                    name: e.subcommand ? "".concat(e.program, " ").concat(e.subcommand) : e.program,
                    programs: [
                        e.program
                    ],
                    subcommands: e.subcommand ? [
                        e.subcommand
                    ] : [],
                    decision: e.decision
                }));
            const resp = await bulkUpsert(rules);
            setResult("✓ Created ".concat(resp.created, ", updated ").concat(resp.updated));
        } catch (err) {
            setResult("Error: ".concat(err instanceof Error ? err.message : "unknown error"));
        } finally{
            setSaving(false);
        }
    }, [
        entries,
        bulkUpsert,
        onDone
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: bulkReviewPanel,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: bulkReviewHeader,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        children: [
                            "Review ",
                            entries.length,
                            " new rule",
                            entries.length !== 1 ? "s" : ""
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: bulkReviewActions,
                        children: [
                            result && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: bulkResultMsg,
                                role: "status",
                                children: result
                            }),
                            result ? /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: bulkSaveBtn,
                                onClick: onDone,
                                children: "Done"
                            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: bulkSaveBtn,
                                onClick: handleSave,
                                disabled: saving,
                                children: saving ? "Saving…" : "Save all"
                            }),
                            !result && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: bulkDiscardBtn,
                                onClick: onDone,
                                disabled: saving,
                                children: "Cancel"
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: tableWrapper,
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("table", {
                    className: table,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("thead", {
                            children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: ApprovalAnalyticsPanel_css_th,
                                        children: "Program"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: ApprovalAnalyticsPanel_css_th,
                                        children: "Subcommand"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: ApprovalAnalyticsPanel_css_th,
                                        children: "Decision"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: ApprovalAnalyticsPanel_css_th
                                    })
                                ]
                            })
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("tbody", {
                            children: entries.map((e)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                    className: row,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                            className: ApprovalAnalyticsPanel_css_td,
                                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                                                className: toolName,
                                                children: e.program
                                            })
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                            className: ApprovalAnalyticsPanel_css_td,
                                            children: e.subcommand ? /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                                                className: toolName,
                                                children: e.subcommand
                                            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                style: {
                                                    color: "var(--text-muted)"
                                                },
                                                children: "any"
                                            })
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                            className: ApprovalAnalyticsPanel_css_td,
                                            children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                                                className: decisionSelect,
                                                value: e.decision,
                                                onChange: (ev)=>setDecision(e.key, Number(ev.target.value)),
                                                "aria-label": "Decision for ".concat(e.program, " ").concat(e.subcommand),
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                        value: types_pb/* AutoDecision */.Uz.ALLOW,
                                                        children: "Allow"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                        value: types_pb/* AutoDecision */.Uz.DENY,
                                                        children: "Deny"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                        value: types_pb/* AutoDecision */.Uz.ESCALATE,
                                                        children: "Escalate (manual)"
                                                    })
                                                ]
                                            })
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                            className: ApprovalAnalyticsPanel_css_td,
                                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                className: removeEntryBtn,
                                                onClick: ()=>removeEntry(e.key),
                                                "aria-label": "Remove",
                                                children: "✕"
                                            })
                                        })
                                    ]
                                }, e.key))
                        })
                    ]
                })
            })
        ]
    });
}
const FILTER_LABELS = {
    all: "All",
    "needs-rule": "Needs rule",
    "has-manual": "Has manual"
};
function UnifiedActivityTable(param) {
    let { commandStats, uncoveredToolNames, uncoveredProgramNames, allToolNames, windowDays, selectedProgram, onSelectProgram, bulkUpsert, onRuleAccepted } = param;
    const [filter, setFilter] = (0,react.useState)("all");
    const [search, setSearch] = (0,react.useState)("");
    const [selected, setSelected] = (0,react.useState)(new Set());
    const [reviewEntries, setReviewEntries] = (0,react.useState)(null);
    const [activeRowKey, setActiveRowKey] = (0,react.useState)(null);
    const { suggestions, loading: genLoading, error: genError, generate, clear: clearSuggestion } = (0,useGenerateRule/* useGenerateRule */.b)();
    // Build unified rows from both tools and commands
    const allRows = react.useMemo(()=>{
        const rows = [];
        // Tool rows — all tools from topTools, mark uncovered ones
        for (const t of allToolNames){
            var _t_manualAllow, _t_manualDeny;
            rows.push({
                key: "tool:".concat(t.toolName),
                type: "tool",
                name: t.toolName,
                subcommand: "",
                category: "tool",
                count: t.count,
                manualCount: ((_t_manualAllow = t.manualAllow) !== null && _t_manualAllow !== void 0 ? _t_manualAllow : 0) + ((_t_manualDeny = t.manualDeny) !== null && _t_manualDeny !== void 0 ? _t_manualDeny : 0),
                isUncovered: uncoveredToolNames.has(t.toolName),
                addRuleHref: (0,ruleBuilderPrefill/* buildPrefillHref */.gd)({
                    toolName: t.toolName
                }),
                bulkEntry: {
                    key: "tool:".concat(t.toolName),
                    program: t.toolName,
                    subcommand: "",
                    decision: types_pb/* AutoDecision */.Uz.ALLOW
                }
            });
        }
        // Add uncovered tools not already present in topTools
        for (const name of uncoveredToolNames){
            if (!allToolNames.find((t)=>t.toolName === name)) {
                rows.push({
                    key: "tool:".concat(name),
                    type: "tool",
                    name,
                    subcommand: "",
                    category: "tool",
                    count: 0,
                    manualCount: 0,
                    isUncovered: true,
                    addRuleHref: (0,ruleBuilderPrefill/* buildPrefillHref */.gd)({
                        toolName: name
                    }),
                    bulkEntry: {
                        key: "tool:".concat(name),
                        program: name,
                        subcommand: "",
                        decision: types_pb/* AutoDecision */.Uz.ALLOW
                    }
                });
            }
        }
        // Command rows from commandSubcommandStats
        const seenPrograms = new Set();
        for (const s of commandStats){
            const manualCount = s.manualAllow + s.manualDeny;
            seenPrograms.add(s.programName);
            rows.push({
                key: "cmd:".concat(s.programName, ":").concat(s.subcommand),
                type: "command",
                name: s.programName,
                subcommand: s.subcommand,
                category: s.category,
                count: s.count,
                manualCount,
                isUncovered: uncoveredProgramNames.has(s.programName),
                programName: s.programName,
                addRuleHref: (0,ruleBuilderPrefill/* buildPrefillHref */.gd)({
                    programs: [
                        s.programName
                    ],
                    subcommands: s.subcommand ? [
                        s.subcommand
                    ] : []
                }),
                bulkEntry: {
                    key: "cmd:".concat(s.programName, ":").concat(s.subcommand),
                    program: s.programName,
                    subcommand: s.subcommand,
                    decision: types_pb/* AutoDecision */.Uz.ALLOW
                }
            });
        }
        // Also show uncovered programs not already present in commandStats
        for (const p of uncoveredProgramNames){
            if (!seenPrograms.has(p)) {
                rows.push({
                    key: "cmd:".concat(p, ":"),
                    type: "command",
                    name: p,
                    subcommand: "",
                    category: "",
                    count: 0,
                    manualCount: 0,
                    isUncovered: true,
                    programName: p,
                    addRuleHref: (0,ruleBuilderPrefill/* buildPrefillHref */.gd)({
                        programs: [
                            p
                        ]
                    }),
                    bulkEntry: {
                        key: "cmd:".concat(p, ":"),
                        program: p,
                        subcommand: "",
                        decision: types_pb/* AutoDecision */.Uz.ALLOW
                    }
                });
            }
        }
        return rows;
    }, [
        commandStats,
        uncoveredToolNames,
        uncoveredProgramNames,
        allToolNames
    ]);
    const filteredRows = react.useMemo(()=>{
        let rows = allRows;
        if (filter === "needs-rule") rows = rows.filter((r)=>r.isUncovered);
        if (filter === "has-manual") rows = rows.filter((r)=>r.manualCount > 0);
        if (search) {
            const lc = search.toLowerCase();
            rows = rows.filter((r)=>r.name.toLowerCase().includes(lc) || r.subcommand.toLowerCase().includes(lc));
        }
        return rows;
    }, [
        allRows,
        filter,
        search
    ]);
    const maxCount = filteredRows.reduce((m, r)=>Math.max(m, r.count), 1);
    const allFilteredKeys = filteredRows.map((r)=>r.key);
    const allChecked = allFilteredKeys.length > 0 && allFilteredKeys.every((k)=>selected.has(k));
    const toggleAll = ()=>{
        if (allChecked) {
            setSelected((prev)=>{
                const next = new Set(prev);
                allFilteredKeys.forEach((k)=>next.delete(k));
                return next;
            });
        } else {
            setSelected((prev)=>new Set([
                    ...prev,
                    ...allFilteredKeys
                ]));
        }
    };
    const toggleRow = (key)=>{
        setSelected((prev)=>{
            const next = new Set(prev);
            if (next.has(key)) next.delete(key);
            else next.add(key);
            return next;
        });
    };
    const openReview = ()=>{
        const entries = allRows.filter((r)=>selected.has(r.key)).map((r)=>({
                ...r.bulkEntry,
                decision: types_pb/* AutoDecision */.Uz.ALLOW
            }));
        setReviewEntries(entries);
    };
    const closeReview = ()=>{
        setReviewEntries(null);
        setSelected(new Set());
    };
    // Counts for filter badges
    const needsRuleCount = allRows.filter((r)=>r.isUncovered).length;
    const hasManualCount = allRows.filter((r)=>r.manualCount > 0).length;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                style: {
                    display: "flex",
                    alignItems: "center",
                    gap: 8,
                    flexWrap: "wrap",
                    marginBottom: 8
                },
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: windowSelector,
                        role: "group",
                        "aria-label": "Activity filter",
                        children: [
                            "all",
                            "needs-rule",
                            "has-manual"
                        ].map((f)=>{
                            const badge = f === "needs-rule" ? needsRuleCount : f === "has-manual" ? hasManualCount : allRows.length;
                            return /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                className: "".concat(windowBtn, " ").concat(filter === f ? windowBtnActive : ""),
                                onClick: ()=>setFilter(f),
                                "aria-pressed": filter === f,
                                children: [
                                    FILTER_LABELS[f],
                                    badge > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                        style: {
                                            marginLeft: 4,
                                            opacity: 0.7,
                                            fontSize: "0.85em"
                                        },
                                        children: [
                                            "(",
                                            badge,
                                            ")"
                                        ]
                                    })
                                ]
                            }, f);
                        })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        type: "text",
                        placeholder: "Search…",
                        value: search,
                        onChange: (e)=>setSearch(e.target.value),
                        className: filterInput,
                        style: {
                            flex: 1,
                            minWidth: 120
                        },
                        "aria-label": "Search activity"
                    })
                ]
            }),
            selected.size > 0 && !reviewEntries && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: bulkActionBar,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: bulkActionCount,
                        children: [
                            selected.size,
                            " selected"
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: bulkAddBtn,
                        onClick: openReview,
                        children: "Add rules for selected →"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: bulkClearBtn,
                        onClick: ()=>setSelected(new Set()),
                        children: "Clear"
                    })
                ]
            }),
            reviewEntries && /*#__PURE__*/ (0,jsx_runtime.jsx)(BulkReviewPanel, {
                entries: reviewEntries,
                onEntriesChange: setReviewEntries,
                bulkUpsert: bulkUpsert,
                onDone: closeReview
            }),
            filteredRows.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                style: {
                    color: "var(--text-muted)",
                    fontSize: "0.85em",
                    padding: "12px 0"
                },
                children: filter === "needs-rule" ? "No uncovered executions — all activity has a matching rule." : filter === "has-manual" ? "No manual reviews in this window." : "No activity."
            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: tableWrapper,
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("table", {
                    className: table,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("thead", {
                            children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: checkboxTh,
                                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                            type: "checkbox",
                                            checked: allChecked,
                                            onChange: toggleAll,
                                            "aria-label": "Select all"
                                        })
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: ApprovalAnalyticsPanel_css_th,
                                        children: "Name"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: ApprovalAnalyticsPanel_css_th,
                                        children: "Type"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: ApprovalAnalyticsPanel_css_th,
                                        children: "Category"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: "".concat(ApprovalAnalyticsPanel_css_th, " ").concat(thRight),
                                        children: "Count"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: "".concat(ApprovalAnalyticsPanel_css_th, " ").concat(thRight),
                                        children: "Manual"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: ApprovalAnalyticsPanel_css_th,
                                        children: "Volume"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: ApprovalAnalyticsPanel_css_th
                                    })
                                ]
                            })
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("tbody", {
                            children: filteredRows.map((r)=>{
                                const isDrillOpen = r.type === "command" && selectedProgram === r.programName;
                                const colSpan = 8;
                                return /*#__PURE__*/ (0,jsx_runtime.jsxs)(react.Fragment, {
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                            className: row,
                                            style: r.type === "command" && r.programName ? {
                                                cursor: "pointer"
                                            } : undefined,
                                            tabIndex: r.type === "command" && r.programName ? 0 : undefined,
                                            role: r.type === "command" && r.programName ? "button" : undefined,
                                            "aria-expanded": r.type === "command" && r.programName ? isDrillOpen : undefined,
                                            onClick: r.type === "command" && r.programName ? ()=>onSelectProgram(isDrillOpen ? null : r.programName) : undefined,
                                            onKeyDown: r.type === "command" && r.programName ? (e)=>{
                                                if (e.key === "Enter" || e.key === " ") {
                                                    e.preventDefault();
                                                    onSelectProgram(isDrillOpen ? null : r.programName);
                                                }
                                            } : undefined,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: checkboxTd,
                                                    onClick: (e)=>e.stopPropagation(),
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                        type: "checkbox",
                                                        checked: selected.has(r.key),
                                                        onChange: ()=>toggleRow(r.key),
                                                        "aria-label": "Select ".concat(r.name, " ").concat(r.subcommand)
                                                    })
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("td", {
                                                    className: ApprovalAnalyticsPanel_css_td,
                                                    children: [
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                                                            className: toolName,
                                                            children: r.name
                                                        }),
                                                        r.subcommand && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                                            children: [
                                                                " ",
                                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                                                                    className: toolName,
                                                                    style: {
                                                                        opacity: 0.7
                                                                    },
                                                                    children: r.subcommand
                                                                })
                                                            ]
                                                        }),
                                                        r.isUncovered && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                            className: gapBadgeLow,
                                                            style: {
                                                                marginLeft: 6
                                                            },
                                                            children: "no rule"
                                                        })
                                                    ]
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: ApprovalAnalyticsPanel_css_td,
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: categoryBadge,
                                                        children: r.type
                                                    })
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: ApprovalAnalyticsPanel_css_td,
                                                    children: r.category !== "tool" && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: categoryBadge,
                                                        children: r.category
                                                    })
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: "".concat(ApprovalAnalyticsPanel_css_td, " ").concat(tdRight),
                                                    children: r.count
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: "".concat(ApprovalAnalyticsPanel_css_td, " ").concat(tdRight),
                                                    children: r.manualCount > 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: manualCount,
                                                        children: r.manualCount
                                                    }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        style: {
                                                            opacity: 0.3
                                                        },
                                                        children: "—"
                                                    })
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: "".concat(ApprovalAnalyticsPanel_css_td, " ").concat(tdBar),
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(Bar, {
                                                        value: r.count,
                                                        max: maxCount,
                                                        className: r.isUncovered ? barGap : barCmd
                                                    })
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                    className: ApprovalAnalyticsPanel_css_td,
                                                    onClick: (e)=>e.stopPropagation(),
                                                    children: r.isUncovered ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                        style: {
                                                            display: "flex",
                                                            flexDirection: "column",
                                                            gap: 2
                                                        },
                                                        children: [
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                "data-testid": r.type === "tool" ? "suggest-rule-tool-".concat(r.name) : "suggest-rule-program-".concat(r.name),
                                                                className: ApprovalAnalyticsPanel_css_addRuleLink,
                                                                style: {
                                                                    background: "none",
                                                                    border: "none",
                                                                    cursor: genLoading ? "not-allowed" : "pointer",
                                                                    padding: 0,
                                                                    font: "inherit"
                                                                },
                                                                disabled: genLoading,
                                                                onClick: ()=>{
                                                                    setActiveRowKey(r.key);
                                                                    void generate({
                                                                        source: types_pb/* SuggestionSource */.Ou.ANALYTICS_GAPS,
                                                                        windowDays,
                                                                        ...r.type === "tool" ? {
                                                                            toolNameFilter: r.name
                                                                        } : {
                                                                            programNameFilter: r.name
                                                                        }
                                                                    });
                                                                },
                                                                children: genLoading && activeRowKey === r.key ? "Generating…" : "Suggest rule →"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                                                                href: r.addRuleHref,
                                                                className: ApprovalAnalyticsPanel_css_addRuleLink,
                                                                style: {
                                                                    fontSize: "0.8em",
                                                                    opacity: 0.7
                                                                },
                                                                children: "or add manually →"
                                                            })
                                                        ]
                                                    }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                                                        href: r.addRuleHref,
                                                        className: ApprovalAnalyticsPanel_css_addRuleLink,
                                                        title: "Add a rule for ".concat(r.name),
                                                        children: "Add rule →"
                                                    })
                                                })
                                            ]
                                        }),
                                        genError && activeRowKey === r.key && /*#__PURE__*/ (0,jsx_runtime.jsx)("tr", {
                                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                colSpan: colSpan,
                                                style: {
                                                    color: "var(--error)",
                                                    fontSize: "0.85em",
                                                    padding: "4px 8px"
                                                },
                                                children: genError.message
                                            })
                                        }),
                                        !genError && activeRowKey === r.key && suggestions.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("tr", {
                                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                colSpan: colSpan,
                                                onClick: (e)=>e.stopPropagation(),
                                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(SuggestedRuleCard/* SuggestedRuleCard */.S, {
                                                    suggestion: suggestions[0],
                                                    onAccept: ()=>{
                                                        clearSuggestion();
                                                        setActiveRowKey(null);
                                                        onRuleAccepted === null || onRuleAccepted === void 0 ? void 0 : onRuleAccepted();
                                                    },
                                                    onDiscard: ()=>{
                                                        clearSuggestion();
                                                        setActiveRowKey(null);
                                                    }
                                                })
                                            })
                                        }),
                                        isDrillOpen && /*#__PURE__*/ (0,jsx_runtime.jsx)("tr", {
                                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                colSpan: colSpan,
                                                onClick: (e)=>e.stopPropagation(),
                                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ProgramDetailPanel, {
                                                    program: r.programName,
                                                    windowDays: windowDays,
                                                    onClose: ()=>onSelectProgram(null)
                                                })
                                            })
                                        })
                                    ]
                                }, r.key);
                            })
                        })
                    ]
                })
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect-web@2.1.1+0149f91553ca585e/node_modules/@connectrpc/connect-web/dist/esm/connect-transport.js + 8 modules
var connect_transport = __webpack_require__(46564);
// EXTERNAL MODULE: ./src/lib/config.ts
var config = __webpack_require__(51202);
// EXTERNAL MODULE: ./src/lib/analytics/index.ts
var analytics = __webpack_require__(10671);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FHookStatusPanel.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61SQW7CMBC884o9koNRHAhQ9zHIxEtw66xT2wFC1b9XSUCQKC1U6snSeGd2ZjWzDd9mx129iOFzArCV2XvubEVKwEG6KWMb_oLnxJ1X0Wvzb51CJ4CXJ_DWaDWYwrsp5qTSlRfAk_LUwKVUSlMuIIk7QGlfGlkL2BlsgeZlSjvMgrYkILOmKqj5yWUpgC8a3tdkdjXNW9N_lRmqJK1KIV2uSUDcKlgKzOszCuDrzmwLHVHn-yBgFbdjmTXWDS8VRz31-e_q8059VIn3lRbjaaXROTEdsPAdyHyQLtyuFt8FeGLrwH_abn3G3_Lfkq7Gk75VPuhdzTJLASlc0iKpHnn9oMlF9KONj17JyRKO1Hk9aPOyPF2aOZq315v00pvK-WZ7aTUFdA0UnCSvu77evEM846kHlB77GcXeHtAJsmEqlPZya1BFD4JTNNC4ElueLWWmQy0gnqX3HskGJo2xR2zO_A1T-r5FMAQAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FHookStatusPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61SQW7CMBC884o9koNRHAhQ9zHIxEtw66xT2wFC1b9XSUCQKC1U6snSeGd2ZjWzDd9mx129iOFzArCV2XvubEVKwEG6KWMb_oLnxJ1X0Wvzb51CJ4CXJ_DWaDWYwrsp5qTSlRfAk_LUwKVUSlMuIIk7QGlfGlkL2BlsgeZlSjvMgrYkILOmKqj5yWUpgC8a3tdkdjXNW9N_lRmqJK1KIV2uSUDcKlgKzOszCuDrzmwLHVHn_yBgFbdjmTXWDS8VRz31_e_q8059VIn3lRbjaaXROTEdsPAdyHyQLtyuFt8FeGLrwH_abn3G3_Lfkq7Gk75VPuhdzTJLASlc0iKpHnn9oMlF9KONj17JyRKO1Hk9aPOyPF2aOZq315v00pvK_WZ7aTUFdA0UnCSvu77evEM846kHlB77GcXeHtAJsmEqlPZya1BFD4JTNNC4ElueLWWmQy0gnqX3HskGJo2xR2zO_A1T_r5FMAQAAA_22_7D = __webpack_require__(73658);
;// ./src/components/sessions/HookStatusPanel.css.ts


var checkboxLabel = '_1bcwfy45';
var HookStatusPanel_css_checkboxRow = '_1bcwfy44';
var HookStatusPanel_css_footer = '_1bcwfy47';
var HookStatusPanel_css_header = '_1bcwfy41';
var installButton = '_1bcwfy48';
var message = '_1bcwfy46';
var HookStatusPanel_css_panel = '_1bcwfy40';
var HookStatusPanel_css_subtitle = '_1bcwfy43';
var HookStatusPanel_css_title = '_1bcwfy42';
;// ./src/components/sessions/HookStatusPanel.tsx
// +feature: rules-hook-status
/* __next_internal_client_entry_do_not_use__ HookStatusPanel auto */ 







function HookStatusPanel() {
    const { track } = (0,analytics/* useAnalytics */.st)();
    const [installRules, setInstallRules] = (0,react.useState)(true);
    const [installAgyRules, setInstallAgyRules] = (0,react.useState)(true);
    const [installGeminiRules, setInstallGeminiRules] = (0,react.useState)(true);
    const [installNotifications, setInstallNotifications] = (0,react.useState)(true);
    const [rulesAvailable, setRulesAvailable] = (0,react.useState)(true);
    const [notificationsAvailable, setNotificationsAvailable] = (0,react.useState)(true);
    const [rulesInstalled, setRulesInstalled] = (0,react.useState)(false);
    const [agyRulesInstalled, setAgyRulesInstalled] = (0,react.useState)(false);
    const [geminiRulesInstalled, setGeminiRulesInstalled] = (0,react.useState)(false);
    const [notificationsInstalled, setNotificationsInstalled] = (0,react.useState)(false);
    const [hookBusy, setHookBusy] = (0,react.useState)(false);
    const [hookMessage, setHookMessage] = (0,react.useState)(null);
    const client = (0,react.useMemo)(()=>(0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,connect_transport/* createConnectTransport */.w)({
            baseUrl: (0,config/* getApiBaseUrl */.h)()
        })), []);
    const mountedRef = (0,react.useRef)(true);
    const seededRef = (0,react.useRef)(false);
    (0,react.useEffect)(()=>{
        mountedRef.current = true;
        return ()=>{
            mountedRef.current = false;
        };
    }, []);
    const refreshHookStatus = (0,react.useCallback)(async ()=>{
        try {
            const res = await client.getHookStatus({});
            if (!mountedRef.current) return;
            setRulesInstalled(res.rulesInstalled);
            setAgyRulesInstalled(res.agyRulesInstalled);
            setGeminiRulesInstalled(res.geminiRulesInstalled);
            setNotificationsInstalled(res.notificationsInstalled);
            setRulesAvailable(res.rulesAvailable);
            setNotificationsAvailable(res.notificationsAvailable);
            if (!seededRef.current) {
                seededRef.current = true;
                setInstallRules(res.rulesAvailable && !res.rulesInstalled);
                setInstallAgyRules(res.rulesAvailable && !res.agyRulesInstalled);
                setInstallGeminiRules(res.rulesAvailable && !res.geminiRulesInstalled);
                setInstallNotifications(res.notificationsAvailable && !res.notificationsInstalled);
            }
        } catch (e) {
            if (mountedRef.current) setHookMessage("Could not check hook status.");
        }
    }, [
        client
    ]);
    (0,react.useEffect)(()=>{
        void refreshHookStatus();
    }, [
        refreshHookStatus
    ]);
    const handleInstallHooks = async ()=>{
        track({
            name: "hook_status_install",
            category: "user_action",
            component: "HookStatusPanel",
            labels: {
                installRules: String(installRules),
                installAgyRules: String(installAgyRules),
                installGeminiRules: String(installGeminiRules),
                installNotifications: String(installNotifications)
            }
        });
        setHookBusy(true);
        setHookMessage(null);
        try {
            const res = await client.installHooks({
                installRules,
                installAgyRules,
                installGeminiRules,
                installNotifications
            });
            if (!mountedRef.current) return;
            if (res.status) {
                setRulesInstalled(res.status.rulesInstalled);
                setAgyRulesInstalled(res.status.agyRulesInstalled);
                setGeminiRulesInstalled(res.status.geminiRulesInstalled);
                setNotificationsInstalled(res.status.notificationsInstalled);
            }
            setHookMessage(res.messages.join(" ") || "Hooks updated.");
        } catch (e) {
            if (mountedRef.current) {
                setHookMessage("Failed to install hooks.");
            }
        } finally{
            if (mountedRef.current) setHookBusy(false);
        }
    };
    const hasPendingInstalls = installRules && !rulesInstalled || installAgyRules && !agyRulesInstalled || installGeminiRules && !geminiRulesInstalled || installNotifications && !notificationsInstalled;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
        className: HookStatusPanel_css_panel,
        "data-testid": "hook-status-panel",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: HookStatusPanel_css_header,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                        className: HookStatusPanel_css_title,
                        children: "CLI Agent Hooks & Rule Enforcement"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: HookStatusPanel_css_subtitle,
                        children: "Enforce your approval rules across Claude Code, Antigravity CLI, and Gemini CLI."
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                className: HookStatusPanel_css_checkboxRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        type: "checkbox",
                        checked: installRules,
                        disabled: !rulesAvailable || rulesInstalled || hookBusy,
                        onChange: (e)=>setInstallRules(e.target.checked)
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: checkboxLabel,
                        children: [
                            "Claude Code rule enforcement",
                            rulesInstalled ? " (already installed)" : !rulesAvailable ? " (ssq-hooks not installed — run `make install`)" : " — gate Claude tool calls through your rules"
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                className: HookStatusPanel_css_checkboxRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        type: "checkbox",
                        checked: installAgyRules,
                        disabled: !rulesAvailable || agyRulesInstalled || hookBusy,
                        onChange: (e)=>setInstallAgyRules(e.target.checked)
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: checkboxLabel,
                        children: [
                            "Antigravity CLI (agy) rule enforcement",
                            agyRulesInstalled ? " (already installed)" : !rulesAvailable ? " (ssq-hooks not installed — run `make install`)" : " — gate Antigravity tool calls through your rules"
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                className: HookStatusPanel_css_checkboxRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        type: "checkbox",
                        checked: installGeminiRules,
                        disabled: !rulesAvailable || geminiRulesInstalled || hookBusy,
                        onChange: (e)=>setInstallGeminiRules(e.target.checked)
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: checkboxLabel,
                        children: [
                            "Gemini CLI rule enforcement",
                            geminiRulesInstalled ? " (already installed)" : !rulesAvailable ? " (ssq-hooks not installed — run `make install`)" : " — gate Gemini tool calls through your rules"
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                className: HookStatusPanel_css_checkboxRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        type: "checkbox",
                        checked: installNotifications,
                        disabled: !notificationsAvailable || notificationsInstalled || hookBusy,
                        onChange: (e)=>setInstallNotifications(e.target.checked)
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: checkboxLabel,
                        children: [
                            "Enable notifications",
                            notificationsInstalled ? " (already installed)" : !notificationsAvailable ? " (ssq-hook-handler not found)" : " — chimes and alerts when agents need you"
                        ]
                    })
                ]
            }),
            hookMessage && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: message,
                children: hookMessage
            }),
            hasPendingInstalls && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: HookStatusPanel_css_footer,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    className: installButton,
                    onClick: handleInstallHooks,
                    disabled: hookBusy,
                    "data-testid": "hook-status-install-button",
                    children: hookBusy ? "Installing…" : "Install"
                })
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Frules%2Fpage.css.ts.vanilla.css%22%2C%22source%22%3A%22LnlhNGNjYTAgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBtaW4taGVpZ2h0OiBjYWxjKHZhcigtLXZpZXdwb3J0LWhlaWdodCwgMTAwZHZoKSAtIHZhcigtLWhlYWRlci1oZWlnaHQpKTsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejYpOwp9Ci55YTRjY2ExIHsKICBmbGV4OiAxOwogIG1heC13aWR0aDogMTIwMHB4OwogIHdpZHRoOiAxMDAlOwogIG1hcmdpbjogMCBhdXRvOwogIHBhZGRpbmc6IDJyZW07CiAgb3ZlcmZsb3cteTogYXV0bzsKICBvdmVyZmxvdy14OiBoaWRkZW47CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogMnJlbTsKfQpAbWVkaWEgc2NyZWVuIGFuZCAobWF4LXdpZHRoOiA3NjhweCkgewogIC55YTRjY2ExIHsKICAgIHBhZGRpbmc6IDFyZW07CiAgfQp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Frules_2Fpage_css_ts_vanilla_css_22_2C_22source_22_3A_22LnlhNGNjYTAgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBtaW4taGVpZ2h0OiBjYWxjKHZhcigtLXZpZXdwb3J0LWhlaWdodCwgMTAwZHZoKSAtIHZhcigtLWhlYWRlci1oZWlnaHQpKTsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejYpOwp9Ci55YTRjY2ExIHsKICBmbGV4OiAxOwogIG1heC13aWR0aDogMTIwMHB4OwogIHdpZHRoOiAxMDAlOwogIG1hcmdpbjogMCBhdXRvOwogIHBhZGRpbmc6IDJyZW07CiAgb3ZlcmZsb3cteTogYXV0bzsKICBvdmVyZmxvdy14OiBoaWRkZW47CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogMnJlbTsKfQpAbWVkaWEgc2NyZWVuIGFuZCAobWF4LXdpZHRoOiA3NjhweCkgewogIC55YTRjY2ExIHsKICAgIHBhZGRpbmc6IDFyZW07CiAgfQp9_22_7D = __webpack_require__(26678);
;// ./src/app/rules/page.css.ts


var main = 'ya4cca1';
var page = 'ya4cca0';
;// ./src/app/rules/page.tsx
// analytics-exempt
/* __next_internal_client_entry_do_not_use__ default auto */ 
// +feature: approval-rules rules-management







function RulesPageInner() {
    const searchParams = (0,nav/* useSearchParams */.ok)();
    const router = (0,nav/* useRouter */.KA)();
    const rawPrefill = searchParams.get("prefill");
    const prefill = rawPrefill ? (0,ruleBuilderPrefill/* decodePrefill */.Tv)(rawPrefill) : null;
    (0,react.useEffect)(()=>{
        if (prefill) {
            setTimeout(()=>{
                var _document_getElementById;
                (_document_getElementById = document.getElementById("rule-builder")) === null || _document_getElementById === void 0 ? void 0 : _document_getElementById.scrollIntoView({
                    behavior: "smooth"
                });
            }, 100);
        }
    }, [
        prefill
    ]);
    function clearPrefill() {
        router.replace("/rules");
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: page,
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("main", {
            id: "main-content",
            className: main,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)(HookStatusPanel, {}),
                /*#__PURE__*/ (0,jsx_runtime.jsx)(ApprovalRulesPanel, {
                    prefill: prefill
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)(ApprovalAnalyticsPanel, {})
            ]
        })
    });
}
function RulesPage() {
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(react.Suspense, {
        fallback: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            style: {
                padding: "2rem"
            },
            children: "Loading…"
        }),
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(RulesPageInner, {})
    });
}


/***/ }),

/***/ 10671:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Or: () => (/* reexport safe */ _usePageView__WEBPACK_IMPORTED_MODULE_0__.O),
/* harmony export */   st: () => (/* reexport safe */ _lib_contexts_AnalyticsContext__WEBPACK_IMPORTED_MODULE_1__.st)
/* harmony export */ });
/* harmony import */ var _usePageView__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(90814);
/* harmony import */ var _lib_contexts_AnalyticsContext__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(24044);






/***/ }),

/***/ 15242:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 21235:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Tv: () => (/* binding */ decodePrefill),
/* harmony export */   gd: () => (/* binding */ buildPrefillHref)
/* harmony export */ });
/* unused harmony export encodePrefill */
function encodePrefill(payload) {
    return btoa(JSON.stringify(payload));
}
const VALID_DECISIONS = new Set([
    0,
    1,
    2,
    3
]); // AutoDecision enum values
function decodePrefill(encoded) {
    try {
        const obj = JSON.parse(atob(encoded));
        if (typeof obj !== "object" || obj === null) return null;
        const result = {};
        if (Array.isArray(obj.programs)) result.programs = obj.programs.filter((v)=>typeof v === "string");
        if (Array.isArray(obj.subcommands)) result.subcommands = obj.subcommands.filter((v)=>typeof v === "string");
        if (typeof obj.toolName === "string") result.toolName = obj.toolName;
        if (typeof obj.toolCategory === "string") result.toolCategory = obj.toolCategory;
        if (typeof obj.suggestedDecision === "number" && VALID_DECISIONS.has(obj.suggestedDecision)) {
            result.suggestedDecision = obj.suggestedDecision;
        }
        return result;
    } catch (e) {
        return null;
    }
}
function buildPrefillHref(payload) {
    return "/rules?prefill=".concat(encodePrefill(payload));
}


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

/***/ 26678:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 32675:
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

/***/ 48843:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   $g: () => (/* binding */ invalidateTaggingRuleNamesCache),
/* harmony export */   Lk: () => (/* binding */ useTaggingRuleNames)
/* harmony export */ });
/* unused harmony export _resetTaggingRuleNamesCacheForTesting */
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(66144);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(23575);
/* harmony import */ var _bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(95444);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(21694);
/* __next_internal_client_entry_do_not_use__ _resetTaggingRuleNamesCacheForTesting,invalidateTaggingRuleNamesCache,useTaggingRuleNames auto */ 





/**
 * Module-level shared cache: SessionCard renders one instance of
 * useTaggingRuleNames per card, potentially dozens at once in a session
 * list. Sharing a single in-flight/resolved fetch (instead of one RPC per
 * card) is a cheap dedup that costs nothing in correctness — rule names
 * change rarely, and this hook is a tooltip nicety, not a source of truth
 * any save path depends on.
 */ let cachedNamesPromise = null;
/**
 * Mounted `useTaggingRuleNames` instances register a refetch callback here so
 * `invalidateTaggingRuleNamesCache` can push fresh data to them directly —
 * without this, invalidation would only affect the *next* mount (empty-deps
 * `useEffect`), leaving already-mounted SessionCards stuck on stale names.
 */ const cacheListeners = new Set();
async function fetchRuleNames() {
    try {
        const client = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_2__/* .getConnectTransport */ .qW)());
        // This fetch is a module-level cache shared across every mounted SessionCard
        // (see cachedNamesPromise above) — aborting it on any one card's unmount
        // would break the fetch for every other still-mounted consumer.
        // abort-signal-exempt
        const resp = await client.listTaggingRules((0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .ListTaggingRulesRequestSchema */ .XyZ, {}));
        const names = {};
        var _resp_rules;
        for (const rule of (_resp_rules = resp.rules) !== null && _resp_rules !== void 0 ? _resp_rules : []){
            names[rule.id] = rule.name;
        }
        return names;
    } catch (err) {
        console.error("Failed to fetch tagging rule names:", err);
        // Don't cache a failed fetch: a transient RPC error would otherwise leave
        // every tooltip blank for the rest of the page's life with no retry.
        cachedNamesPromise = null;
        return {};
    }
}
/** Resets the module-level cache — for tests only. */ function _resetTaggingRuleNamesCacheForTesting() {
    cachedNamesPromise = null;
}
/**
 * Invalidates the module-level rule-names cache so the next render refetches
 * fresh data. Call after a tagging rule mutation (upsert/delete) so already-
 * mounted SessionCards' provenance tooltips pick up the new name/removal
 * instead of showing stale data until a full page reload.
 */ function invalidateTaggingRuleNamesCache() {
    cachedNamesPromise = null;
    for (const listener of cacheListeners)listener();
}
/**
 * Returns a map of TaggingRule.ID -> rule name, for resolving a tag's
 * provenance (`RuleTagProvenance[tag]`, a rule ID or the "llm" sentinel) into
 * human-readable tooltip text (ux.md Surface 1). Falls back to an empty map
 * on any fetch failure — callers should treat an unresolved ID the same as a
 * deleted rule (ux.md's "originating rule no longer exists" edge case).
 */ function useTaggingRuleNames() {
    const [names, setNames] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)({});
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        let cancelled = false;
        const applyResolved = (resolved)=>{
            if (cancelled) return;
            setNames(resolved);
        };
        const load = ()=>{
            if (!cachedNamesPromise) {
                cachedNamesPromise = fetchRuleNames();
            }
            cachedNamesPromise.then(applyResolved);
        };
        load();
        cacheListeners.add(load);
        return ()=>{
            cancelled = true;
            cacheListeners.delete(load);
        };
    }, []);
    return names;
}


/***/ }),

/***/ 60795:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   AY: () => (/* binding */ selectSessionsError),
/* harmony export */   Ay: () => (__WEBPACK_DEFAULT_EXPORT__),
/* harmony export */   Hx: () => (/* binding */ selectDetectedStatusMap),
/* harmony export */   Nh: () => (/* binding */ setError),
/* harmony export */   OS: () => (/* binding */ selectSessionsLoading),
/* harmony export */   QA: () => (/* binding */ setErrorCode),
/* harmony export */   Qr: () => (/* binding */ selectSessionsHasLoadedOnce),
/* harmony export */   YI: () => (/* binding */ setSessions),
/* harmony export */   aA: () => (/* binding */ setConnectionState),
/* harmony export */   dF: () => (/* binding */ removeSession),
/* harmony export */   ih: () => (/* binding */ upsertSession),
/* harmony export */   mN: () => (/* binding */ selectConnectionState),
/* harmony export */   od: () => (/* binding */ selectActiveSessionsSortedByUpdatedAt),
/* harmony export */   pu: () => (/* binding */ removeDetectedStatus),
/* harmony export */   r1: () => (/* binding */ setLoading),
/* harmony export */   rt: () => (/* binding */ selectAllSessions)
/* harmony export */ });
/* unused harmony exports selectSessionById, selectSessionIds, selectSessionsTotal, selectSessionsErrorCode */
/* harmony import */ var _reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(73559);
/* harmony import */ var _reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(13009);
/* harmony import */ var _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(81432);


const sessionsAdapter = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_1__/* .createEntityAdapter */ .pU)({
    selectId: (session)=>session.id
});
const initialState = sessionsAdapter.getInitialState({
    loading: false,
    hasLoadedOnce: false,
    error: null,
    detectedStatusMap: {},
    connectionState: "disconnected",
    deletedIds: {}
});
const sessionsSlice = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_1__/* .createSlice */ .Z0)({
    name: "sessions",
    initialState,
    reducers: {
        setSessions (state, action) {
            state.hasLoadedOnce = true;
            const filtered = action.payload.filter((s)=>!state.deletedIds[s.id]);
            // Preserve existing entity references when a session's data is unchanged
            // (same updatedAt) so React.memo on SessionRowWrapper can skip re-rendering
            // rows untouched by this snapshot/poll — mirrors the guard in upsertSession.
            const merged = filtered.map((incoming)=>{
                const existing = state.entities[incoming.id];
                if (existing && existing.updatedAt !== undefined && incoming.updatedAt !== undefined && existing.updatedAt.seconds === incoming.updatedAt.seconds && existing.updatedAt.nanos === incoming.updatedAt.nanos) {
                    return existing;
                }
                return incoming;
            });
            sessionsAdapter.setAll(state, merged);
        },
        upsertSession (state, action) {
            // Don't resurrect a deleted session via an in-flight update event
            if (!state.deletedIds[action.payload.id]) {
                // Skip no-op upserts: if updatedAt matches the existing record the data
                // hasn't changed and we'd just cause a spurious re-render. This prevents
                // the N-render storm from WatchSessions initial-snapshot events that
                // duplicate sessions already loaded by the preceding listSessions() call.
                const existing = state.entities[action.payload.id];
                const incoming = action.payload;
                if (existing && existing.updatedAt !== undefined && incoming.updatedAt !== undefined && existing.updatedAt.seconds === incoming.updatedAt.seconds && existing.updatedAt.nanos === incoming.updatedAt.nanos) {
                    return;
                }
                sessionsAdapter.upsertOne(state, action.payload);
                // Sync detectedStatusMap from the session's proto fields
                const session = action.payload;
                if (session.status !== _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SessionStatus */ .Ph.ACTIVE) {
                    // Non-active: clear badge unconditionally
                    delete state.detectedStatusMap[session.id];
                } else if (session.detectedStatus !== undefined && session.detectedStatus !== _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.UNSPECIFIED) {
                    var _session_detectedContext;
                    // Active with typed detection: update map from proto field
                    state.detectedStatusMap[session.id] = {
                        detectedStatus: session.detectedStatus,
                        detectedContext: (_session_detectedContext = session.detectedContext) !== null && _session_detectedContext !== void 0 ? _session_detectedContext : ""
                    };
                } else {
                    // Active + UNSPECIFIED: clear
                    delete state.detectedStatusMap[session.id];
                }
            }
        },
        removeSession (state, action) {
            sessionsAdapter.removeOne(state, action.payload);
            state.deletedIds[action.payload] = true;
        },
        setLoading (state, action) {
            state.loading = action.payload;
        },
        setError (state, action) {
            state.error = action.payload;
            // A fresh error/clear always invalidates the previously-classified code — callers that
            // need a code alongside the message dispatch setErrorCode immediately after setError.
            state.errorCode = undefined;
        },
        setErrorCode (state, action) {
            state.errorCode = action.payload;
        },
        setConnectionState (state, action) {
            state.connectionState = action.payload;
        },
        removeDetectedStatus (state, action) {
            delete state.detectedStatusMap[action.payload];
        }
    }
});
const { setSessions, upsertSession, removeSession, setLoading, setError, setErrorCode, setConnectionState, removeDetectedStatus } = sessionsSlice.actions;
// Use the adapter's built-in selectors scoped to the sessions slice
const adapterSelectors = sessionsAdapter.getSelectors((state)=>state.sessions);
const selectAllSessions = adapterSelectors.selectAll;
const selectSessionById = adapterSelectors.selectById;
// Memoized selector: active sessions pre-sorted by updatedAt descending.
// Avoids repeated O(n log n) sort inside render functions that show recent sessions.
const selectActiveSessionsSortedByUpdatedAt = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_2__/* .createSelector */ .Mz)(selectAllSessions, (sessions)=>sessions.filter((s)=>s.status !== _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SessionStatus */ .Ph.UNSPECIFIED).sort((a, b)=>{
        var _b_updatedAt, _a_updatedAt, _b_createdAt, _a_createdAt;
        var _b_updatedAt_seconds, _a_updatedAt_seconds;
        const byUpdated = Number((_b_updatedAt_seconds = (_b_updatedAt = b.updatedAt) === null || _b_updatedAt === void 0 ? void 0 : _b_updatedAt.seconds) !== null && _b_updatedAt_seconds !== void 0 ? _b_updatedAt_seconds : 0) - Number((_a_updatedAt_seconds = (_a_updatedAt = a.updatedAt) === null || _a_updatedAt === void 0 ? void 0 : _a_updatedAt.seconds) !== null && _a_updatedAt_seconds !== void 0 ? _a_updatedAt_seconds : 0);
        if (byUpdated !== 0) return byUpdated;
        var _b_createdAt_seconds, _a_createdAt_seconds;
        const byCreated = Number((_b_createdAt_seconds = (_b_createdAt = b.createdAt) === null || _b_createdAt === void 0 ? void 0 : _b_createdAt.seconds) !== null && _b_createdAt_seconds !== void 0 ? _b_createdAt_seconds : 0) - Number((_a_createdAt_seconds = (_a_createdAt = a.createdAt) === null || _a_createdAt === void 0 ? void 0 : _a_createdAt.seconds) !== null && _a_createdAt_seconds !== void 0 ? _a_createdAt_seconds : 0);
        if (byCreated !== 0) return byCreated;
        return a.id.localeCompare(b.id);
    }));
const selectSessionIds = adapterSelectors.selectIds;
const selectSessionsTotal = adapterSelectors.selectTotal;
const selectSessionsLoading = (state)=>state.sessions.loading;
const selectSessionsHasLoadedOnce = (state)=>state.sessions.hasLoadedOnce;
const selectSessionsError = (state)=>state.sessions.error;
const selectSessionsErrorCode = (state)=>state.sessions.errorCode;
const selectDetectedStatusMap = (state)=>state.sessions.detectedStatusMap;
const selectConnectionState = (state)=>state.sessions.connectionState;
/* harmony default export */ const __WEBPACK_DEFAULT_EXPORT__ = (sessionsSlice.reducer);


/***/ }),

/***/ 62382:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 65916:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 73658:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 79116:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 86207:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 2419));


/***/ }),

/***/ 87433:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 87448:
/***/ (() => {

// extracted by mini-css-extract-plugin

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

/***/ 98216:
/***/ (() => {

// extracted by mini-css-extract-plugin

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
/******/ __webpack_require__.O(0, [1869,852,4202,9353,8182,4069,8165,3575,2275,4826,2648,9691,3516,7358], () => (__webpack_exec__(86207)));
/******/ var __webpack_exports__ = __webpack_require__.O();
/******/ _N_E = __webpack_exports__;
/******/ }
]);
//# sourceMappingURL=page-ac338ae335798062.js.map