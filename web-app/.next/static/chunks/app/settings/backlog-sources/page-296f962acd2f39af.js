(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[4377],{

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

/***/ 6579:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 19892:
/***/ (() => {

// extracted by mini-css-extract-plugin

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

/***/ 50060:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  BacklogSourcesSettings: () => (/* binding */ BacklogSourcesSettings)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/nav/index.ts
var nav = __webpack_require__(41078);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ./src/lib/api/transport.ts
var transport = __webpack_require__(21694);
// EXTERNAL MODULE: ./src/gen/session/v1/backlog_pb.ts
var backlog_pb = __webpack_require__(79870);
;// ./src/lib/hooks/useBacklogSourcesService.ts
/* __next_internal_client_entry_do_not_use__ useBacklogSourcesService auto */ 



function tsToIso(ts) {
    return ts ? new Date(Number(ts.seconds) * 1000).toISOString() : undefined;
}
function mapItemSource(p) {
    return {
        id: p.id,
        pluginId: p.pluginId,
        displayName: p.displayName,
        enabled: p.enabled,
        tokenConfigured: p.tokenConfigured,
        lastSyncedAt: tsToIso(p.lastSyncedAt),
        forwardSyncEnabled: p.forwardSyncEnabled,
        backwardSyncEnabled: p.backwardSyncEnabled,
        forwardSyncCloseLabel: p.forwardSyncCloseLabel
    };
}
function mapSyncEvent(p) {
    return {
        id: p.id,
        startedAt: tsToIso(p.startedAt),
        finishedAt: tsToIso(p.finishedAt),
        itemsCreated: p.itemsCreated,
        itemsUpdated: p.itemsUpdated,
        itemsSkipped: p.itemsSkipped,
        itemsErrored: p.itemsErrored,
        errorMessage: p.errorMessage || undefined
    };
}
function useBacklogSourcesService() {
    const clientRef = (0,react.useRef)(null);
    const [lastError, setLastError] = (0,react.useState)(null);
    const clearError = (0,react.useCallback)(()=>setLastError(null), []);
    (0,react.useEffect)(()=>{
        clientRef.current = (0,promise_client/* createClient */.UU)(backlog_pb/* BacklogService */.jSM, (0,transport/* getConnectTransport */.qW)());
    }, []);
    const listItemSources = (0,react.useCallback)(async ()=>{
        if (!clientRef.current) return [];
        try {
            const resp = await clientRef.current.listItemSources({});
            setLastError(null);
            var _resp_sources;
            return ((_resp_sources = resp.sources) !== null && _resp_sources !== void 0 ? _resp_sources : []).map(mapItemSource);
        } catch (err) {
            console.error("[useBacklogSourcesService] listItemSources:", err);
            setLastError(err instanceof Error ? err : new Error(String(err)));
            return [];
        }
    }, []);
    const createItemSource = (0,react.useCallback)(async (data)=>{
        if (!clientRef.current) return null;
        try {
            setLastError(null);
            const resp = await clientRef.current.createItemSource({
                pluginId: data.pluginId,
                displayName: data.displayName,
                configJson: data.configJson,
                token: data.token
            });
            return resp.source ? mapItemSource(resp.source) : null;
        } catch (err) {
            console.error("[useBacklogSourcesService] createItemSource:", err);
            setLastError(err instanceof Error ? err : new Error(String(err)));
            return null;
        }
    }, []);
    const setItemSourceEnabled = (0,react.useCallback)(async (source, enabled)=>{
        if (!clientRef.current) return null;
        try {
            setLastError(null);
            const resp = await clientRef.current.updateItemSource({
                sourceId: source.id,
                displayName: source.displayName,
                enabled,
                token: "",
                forwardSyncEnabled: source.forwardSyncEnabled,
                backwardSyncEnabled: source.backwardSyncEnabled,
                forwardSyncCloseLabel: source.forwardSyncCloseLabel
            });
            return resp.source ? mapItemSource(resp.source) : null;
        } catch (err) {
            console.error("[useBacklogSourcesService] updateItemSource:", err);
            setLastError(err instanceof Error ? err : new Error(String(err)));
            return null;
        }
    }, []);
    // The three sync-direction setters below all round-trip the FULL current
    // source state through UpdateItemSource, only changing the one field the
    // caller intends to change — UpdateItemSource's fields are all
    // unconditionally overwritten (not partial-update), so passing anything
    // other than the current live value for every other field risks silently
    // reverting it (e.g. re-disabling `enabled` or clobbering the close
    // label) out from under a concurrent edit. Mirrors setItemSourceEnabled's
    // pattern above, generalized to take the whole ItemSource rather than a
    // narrow (id, displayName, enabled) tuple.
    const setForwardSyncEnabled = (0,react.useCallback)(async (source, enabled)=>{
        if (!clientRef.current) return null;
        try {
            setLastError(null);
            const resp = await clientRef.current.updateItemSource({
                sourceId: source.id,
                displayName: source.displayName,
                enabled: source.enabled,
                token: "",
                forwardSyncEnabled: enabled,
                backwardSyncEnabled: source.backwardSyncEnabled,
                forwardSyncCloseLabel: source.forwardSyncCloseLabel
            });
            return resp.source ? mapItemSource(resp.source) : null;
        } catch (err) {
            console.error("[useBacklogSourcesService] setForwardSyncEnabled:", err);
            setLastError(err instanceof Error ? err : new Error(String(err)));
            return null;
        }
    }, []);
    const setBackwardSyncEnabled = (0,react.useCallback)(async (source, enabled)=>{
        if (!clientRef.current) return null;
        try {
            setLastError(null);
            const resp = await clientRef.current.updateItemSource({
                sourceId: source.id,
                displayName: source.displayName,
                enabled: source.enabled,
                token: "",
                forwardSyncEnabled: source.forwardSyncEnabled,
                backwardSyncEnabled: enabled,
                forwardSyncCloseLabel: source.forwardSyncCloseLabel
            });
            return resp.source ? mapItemSource(resp.source) : null;
        } catch (err) {
            console.error("[useBacklogSourcesService] setBackwardSyncEnabled:", err);
            setLastError(err instanceof Error ? err : new Error(String(err)));
            return null;
        }
    }, []);
    const setForwardSyncCloseLabel = (0,react.useCallback)(async (source, closeLabel)=>{
        if (!clientRef.current) return null;
        try {
            setLastError(null);
            const resp = await clientRef.current.updateItemSource({
                sourceId: source.id,
                displayName: source.displayName,
                enabled: source.enabled,
                token: "",
                forwardSyncEnabled: source.forwardSyncEnabled,
                backwardSyncEnabled: source.backwardSyncEnabled,
                forwardSyncCloseLabel: closeLabel
            });
            return resp.source ? mapItemSource(resp.source) : null;
        } catch (err) {
            console.error("[useBacklogSourcesService] setForwardSyncCloseLabel:", err);
            setLastError(err instanceof Error ? err : new Error(String(err)));
            return null;
        }
    }, []);
    const deleteItemSource = (0,react.useCallback)(async (id)=>{
        if (!clientRef.current) return false;
        try {
            setLastError(null);
            await clientRef.current.deleteItemSource({
                sourceId: id
            });
            return true;
        } catch (err) {
            console.error("[useBacklogSourcesService] deleteItemSource:", err);
            setLastError(err instanceof Error ? err : new Error(String(err)));
            return false;
        }
    }, []);
    const triggerSync = (0,react.useCallback)(async (id)=>{
        if (!clientRef.current) return false;
        try {
            setLastError(null);
            await clientRef.current.triggerSync({
                sourceId: id
            });
            return true;
        } catch (err) {
            console.error("[useBacklogSourcesService] triggerSync:", err);
            setLastError(err instanceof Error ? err : new Error(String(err)));
            return false;
        }
    }, []);
    // previewBackwardSyncImpact returns null on any RPC failure (network,
    // upstream Fetch/rate-limit, etc.) rather than swallowing it into a
    // misleading itemCount: 0 — the caller (BacklogSourcesSettings) must be
    // able to tell "nothing to warn about" apart from "couldn't check" and
    // show a distinct inline error for the latter (Epic 4.4, Story 4.4.2).
    const previewBackwardSyncImpact = (0,react.useCallback)(async (sourceId)=>{
        if (!clientRef.current) return null;
        try {
            const resp = await clientRef.current.previewBackwardSyncImpact({
                sourceId
            });
            setLastError(null);
            var _resp_sampleTitles, _resp_possiblyIncomplete;
            return {
                itemCount: resp.itemCount,
                sampleTitles: (_resp_sampleTitles = resp.sampleTitles) !== null && _resp_sampleTitles !== void 0 ? _resp_sampleTitles : [],
                possiblyIncomplete: (_resp_possiblyIncomplete = resp.possiblyIncomplete) !== null && _resp_possiblyIncomplete !== void 0 ? _resp_possiblyIncomplete : false
            };
        } catch (err) {
            console.error("[useBacklogSourcesService] previewBackwardSyncImpact:", err);
            setLastError(err instanceof Error ? err : new Error(String(err)));
            return null;
        }
    }, []);
    const getSyncHistory = (0,react.useCallback)(async (id)=>{
        if (!clientRef.current) return {
            events: [],
            truncated: false
        };
        try {
            const resp = await clientRef.current.getSyncHistory({
                sourceId: id
            });
            setLastError(null);
            var _resp_events;
            return {
                events: ((_resp_events = resp.events) !== null && _resp_events !== void 0 ? _resp_events : []).map(mapSyncEvent),
                truncated: Boolean(resp.truncated)
            };
        } catch (err) {
            console.error("[useBacklogSourcesService] getSyncHistory:", err);
            setLastError(err instanceof Error ? err : new Error(String(err)));
            return {
                events: [],
                truncated: false
            };
        }
    }, []);
    return (0,react.useMemo)(()=>({
            listItemSources,
            createItemSource,
            setItemSourceEnabled,
            setForwardSyncEnabled,
            setBackwardSyncEnabled,
            setForwardSyncCloseLabel,
            deleteItemSource,
            triggerSync,
            getSyncHistory,
            previewBackwardSyncImpact,
            lastError,
            clearError
        }), [
        listItemSources,
        createItemSource,
        setItemSourceEnabled,
        setForwardSyncEnabled,
        setBackwardSyncEnabled,
        setForwardSyncCloseLabel,
        deleteItemSource,
        triggerSync,
        getSyncHistory,
        previewBackwardSyncImpact,
        lastError,
        clearError
    ]);
}

// EXTERNAL MODULE: ./src/lib/routes.ts + 1 modules
var routes = __webpack_require__(76514);
;// ./src/components/settings/backlogSourceSchemas.ts
// Adding a source type for a new plugin (e.g. Jira, Linear) means adding one entry
// here — the form in BacklogSourcesSettings.tsx renders fields from this schema
// instead of hardcoding owner/repo, so the UI stays in sync with the backend's
// pluggable ItemSourcePlugin registry (session/backlog_plugin.go) without
// per-plugin component changes. Kept in its own module (rather than inline in the
// component) so tests can mock it with a synthetic schema to prove the component
// is genuinely schema-driven rather than hardcoded.
const PLUGIN_SCHEMAS = [
    {
        id: "github_issues",
        label: "GitHub Issues",
        fields: [
            {
                key: "owner",
                label: "Owner",
                placeholder: "Owner (e.g. acme)"
            },
            {
                key: "repo",
                label: "Repo",
                placeholder: "Repo (e.g. widgets)"
            }
        ],
        requiresToken: true,
        tokenLabel: "GitHub personal access token",
        credentialsManagedExternally: true
    },
    {
        id: "linear",
        label: "Linear",
        fields: [
            {
                key: "team_key",
                label: "Team Key",
                placeholder: "Team key (e.g. ENG)"
            }
        ],
        requiresToken: true,
        tokenLabel: "Linear API key",
        credentialsManagedExternally: true,
        credentialsHelp: "Credentials are read from the server's keychain or the LINEAR_API_KEY environment variable."
    },
    {
        id: "jira",
        label: "JIRA",
        fields: [
            {
                key: "base_url",
                label: "Base URL",
                placeholder: "Base URL (e.g. https://acme.atlassian.net)"
            },
            {
                key: "project_key",
                label: "Project Key",
                placeholder: "Project key (e.g. ENG)"
            }
        ],
        requiresToken: true,
        tokenLabel: "JIRA API token",
        credentialsManagedExternally: true,
        credentialsHelp: "Credentials are read from the server's keychain or the JIRA_BASE_URL, JIRA_EMAIL and JIRA_API_TOKEN environment variables."
    },
    {
        id: "github_prs",
        label: "GitHub Pull Requests",
        fields: [
            {
                key: "owner",
                label: "Owner",
                placeholder: "Owner (e.g. acme)"
            },
            {
                key: "repo",
                label: "Repo",
                placeholder: "Repo (e.g. widgets)"
            }
        ],
        requiresToken: true,
        tokenLabel: "GitHub personal access token",
        credentialsManagedExternally: true
    }
];

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react-dom/index.js
var react_dom = __webpack_require__(80883);
// EXTERNAL MODULE: ./src/lib/hooks/useDialogFocusTrap.ts
var useDialogFocusTrap = __webpack_require__(82279);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2FmodalChrome.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA9VVTW-jMBC951fMpVKQ1pGhbZqQS__GnlYONuAWbNZ2UpJV__sKG1JwgGSl3cNKETH-ePPmzfPw-s5OqSIl01BvxbqkGH4tAFIlSzsAkBVJuDnFgHcLgM8FgJH-UuiWPhevPlzowxlFhE6lKmM3LIhh35cRrupgdyve-FnsH7ywWTkOkT1eSc0NlyKGlNeM7ixw1YYpWGraoeJZ3o330hhZdi8kec-UPAgaw5GoJUI_wi07R-pMLAPKdVWQUwxpwepmghQ8E4gbVuoYEiYMU83020Ebnp5QIoVhwvSXzogLyuoYQoxtTCJ4SRzrrjx4FWlgRLPdV4aPNsNpgtvAZaMoU0gRyg86hjCqLM2KUMpF1mZZkhp9cGryGJ6bqjRz7fsWP3Q7cuZU2uBjPpp7848oVyxx7BNZHErhWNRI54TKjxgwNCFg3TxUtidL_A3a3-oxGBUgbJauBHhyJe4yiZ6q2j3CdTvqCdBVNaxq0LLg1FPrLeghP1vkkqiMi4YwhgbWSpVKYZDmZxZDp5Sd-mjFWbsaJrKQyq8I7sdYD2N42GHLfhQn7OO8TBmdC83MlY3RKOK_M_OlPMOAj4McNp6Vx0n2DT1fyCvbe8E3gW9Kb0MZzHGPAu_SPG2GlyYhRbIMMX4ABJFi5bi609clI9W8YFvXYb_84m2mwZUxvR15MGvTgTMvYYkNO-nJOUaJDwpQcMEuTSVcPfcC7W2gK8XGdHFsryxpxWWCfkVFtvMPT0fnvqrJXTYsp5X7OXCokILd9uLLnNWiczDmg9tCz5U-cwkclG4yqCTvLq_9vrZtxDsjBjrFuTwydZda3sFUJgeNjlzzfcEsgDyYxggxRBMX2undbkMyTW1bs9-xCzCd4GIzqohiwsx209tthd3RVv7DUtI_KOWmL8HoDj6E_svF_g3PZClKtwoAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2FmodalChrome_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9VVTW_jMBC951fMpVKQ1pGhbZqQS_GnlYONuAWbNZ2UpJV_sKG1JwgGSl3cNKETH_ePPmzfPw_s5OqSIl01BvxbqkGH4tAFIlSzsAkBVJuDnFgHcLgM8FgJH_UuiWPhevPlzowxlFhE6lKmM3LIhh35cRrupgdyve_FnsH7ywWTkOkT1eSc0NlyKGlNeM7ixw1YYpWGraoeJZ3o330hhZdi8kec_UPAgaw5GoJUI_wi07R_pMLAPKdVWQUwxpwepmghQ8E4gbVuoYEiYMU83020Ebnp5QIoVhwvSXzogLyuoYQoxtTCJ4SRzrrjx4FWlgRLPdV4aPNsNpgtvAZaMoU0gRyg86hjCqLM2KUMpF1mZZkhp9cGryGJ6bqjRz7fsWP3Q7cuZU2uBjPpp7848oVyxx7BNZHErhWNRI54TKjxgwNCFg3TxUtidL_A3a3_oxGBUgbJauBHhyJe4yiZ6q2j3CdTvqCdBVNaxq0LLg1FPrLeghP1vkkqiMi4YwhgbWSpVKYZDmZxZDp5Sd_mjFWbsaJrKQyq8I7sdYD2N42GHLfhQn7OO8TBmdC83MlY3RKOK_M_OlPMOAj4McNp6Vx0n2DT1fyCvbe8E3gW9Kb0MZzHGPAu_SPG2GlyYhRbIMMX4ABJFi5bi609clI9W8YFvXYb_84m2mwZUxvR15MGvTgTMvYYkNO_nJOUaJDwpQcMEuTSVcPfcC7W2gK8XGdHFsryxpxWWCfkVFtvMPT0fnvqrJXTYsp5X7OXCokILd9uLLnNWiczDmg9tCz5U_cwkclG4yqCTvLq_9vrZtxDsjBjrFuTwydZda3sFUJgeNjlzzfcEsgDyYxggxRBMX2undbkMyTW1bs9_xCzCd4GIzqohiwsx209tthd3RVv7DUtI_KOWmL8HoDj6E_svF_g3PZClKtwoAAA_22_7D = __webpack_require__(6579);
;// ./src/components/settings/BackwardSyncConfirmDialog.css.ts


var actions = 'x9n6mdb';
var body = 'x9n6mda';
var dialog = 'x9n6md8';
var heading = 'x9n6md9';
var overlay = 'x9n6md7';
var primaryButton = 'x9n6mdc';
var secondaryButton = 'x9n6mdd';
;// ./src/components/settings/BackwardSyncConfirmDialog.tsx
/* __next_internal_client_entry_do_not_use__ BackwardSyncConfirmDialog auto */ 




/**
 * BackwardSyncConfirmDialog — gates the first enable of backward sync
 * (Epic 4.4, Story 4.4.2) with a preview of how many already-imported items
 * would immediately archive, and informed-consent copy stating the archive
 * isn't undone by re-disabling the toggle. Only flips the toggle on Confirm.
 *
 * No existing modal primitive in web-app/src/components/shared/ implements
 * BOTH a full Tab-cycling focus trap AND focus-return-to-trigger-on-close
 * (VaguenessPromptModal traps focus between two fixed buttons but never
 * returns focus; NewShellDialog closes on Escape but doesn't trap focus at
 * all) — this component is a new, minimal one built to this feature's exact
 * accessibility ACs rather than partially reusing either.
 */ function BackwardSyncConfirmDialog(param) {
    let { sourceDisplayName, itemCount, sampleTitles, possiblyIncomplete, onConfirm, onCancel } = param;
    const headingId = (0,react.useId)();
    const dialogRef = (0,react.useRef)(null);
    const confirmButtonRef = (0,react.useRef)(null);
    (0,useDialogFocusTrap/* useDialogFocusTrap */.L)({
        dialogRef,
        initialFocusRef: confirmButtonRef,
        onEscape: onCancel
    });
    const shown = sampleTitles.slice(0, 5);
    const remaining = itemCount - shown.length;
    const titlesText = shown.length > 0 ? "".concat(shown.join(", ")).concat(remaining > 0 ? ", and ".concat(remaining, " more") : "") : "";
    const content = /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: overlay,
        "data-testid": "backward-sync-confirm-overlay",
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            ref: dialogRef,
            role: "dialog",
            "aria-modal": "true",
            "aria-labelledby": headingId,
            className: dialog,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("h2", {
                    id: headingId,
                    className: heading,
                    children: [
                        "Reflect GitHub status back for ",
                        sourceDisplayName,
                        "?"
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                    className: body,
                    children: [
                        "Enabling this will immediately archive ",
                        itemCount,
                        " already-imported item",
                        itemCount === 1 ? "" : "s",
                        " whose linked GitHub issue is closed and can't be undone by disabling this toggle again",
                        titlesText ? ": ".concat(titlesText) : "",
                        ". Continue?"
                    ]
                }),
                possiblyIncomplete && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: body,
                    "data-testid": "backward-sync-confirm-incomplete-caveat",
                    children: "This repository has a large issue history — the count above may be incomplete. More already-closed items than shown could be archived once enabled."
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: actions,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            type: "button",
                            className: secondaryButton,
                            onClick: onCancel,
                            "data-testid": "backward-sync-confirm-cancel",
                            children: "Cancel"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            ref: confirmButtonRef,
                            type: "button",
                            className: primaryButton,
                            onClick: onConfirm,
                            "data-testid": "backward-sync-confirm-confirm",
                            children: "Confirm"
                        })
                    ]
                })
            ]
        })
    });
    // createPortal ensures the dialog renders at document.body, escaping any
    // ancestor CSS transform/filter/will-change that would break position:fixed
    // (ADR-009 / docs/reference/css-architecture.md).
    if (typeof document === "undefined") return null;
    return /*#__PURE__*/ (0,react_dom.createPortal)(content, document.body);
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsettings%2FBacklogSourcesSettings.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA91X23LbIBB991fwWD9IIymxE-OP6WCBJGoEFFBsudN_7wjFDrJuxHEmnTxpRixn2bNnlyUUu1Ju8gj8WQBQomNwoNgUEKwfI3ncLgDAVEuGaggyRuyP5htgqkhqqOAQpIJVJW9WciQheEHqRxD8jDfklKjTQ7LcLv4uwtZLbL18EDFyEROLmAluAk1PpGecLrfn9QOheWEgWEdR8y8VTKhr-8ial0jllEMQOY4eZhztlqOgySjo4x34SE4uH6t7IQIgEcaU54P8A7BD6T5XouL42uCpXRcKEwVBLI9AC0bxlRVxrAKFMK10z9PajWw9HBliNOcBNaTUEKSEG6K8iHpq89nwkitxgCDeeuT3XUK6-Hr-mHYuOBuL81qgSbhSpGw2Fq_nicPk_Gua100nQ1xwYv1XSjcHkIKeWRxPcps-KTRtBaUIQ4a-WCCjED__f0MAUZjoixZ1oSjfd6sB2fDGfZYuFztrLCRKqakhiMKVGwMXJkCMiQPBzp50WEKzWsF230g9JPVyuH7mte2hiYGc-BWWw6LNh0SKcOOnVwwL8ULUTDqeu1sw1WjHCL4hL6Tn6erIHlKd6LxzJF8SG_VTOylY4vA06D_ulG92j95cv4WUoZKyumdR-Slrttvkd7-cihvrjw726rF6PF3X4-yNFSOvK2vvXdbDYth5JOYtZpiJtNJtZbQuB1GZBRWVYZSTc4U4KFAylJJCMDwp1NT1_Wuy330Pfve3xfg1Pb2cIbgcZ2XVn1tW7dzSTk6asAy-tjiDlHEp8roGeIfVj1wDbFyffo28XTc1IxBQgxhNHfRypk1PCjj2EnDsL5L3Cs9T1bzbYSlvukIwMyv_d5w0WIMj1TgLqC_zK4vc5Ul8-jPpPASO8off-wCSM6-IeQqKieZplww5msDOXplQJQSVlESlSNuhixFjiAp0U9N2WgqjVfPauBzw911eaDeOLp0BduKmm0tL7aXq4xcUOnIj1F5imEj1ZK8037RXWgr_ARKDTuPyEgAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsettings_2FBacklogSourcesSettings_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA91X23LbIBB991fwWD9IIymxE_OP6WCBJGoEFFBsudN_7wjFDrJuxHEmnTxpRixn2bNnlyUUu1Ju8gj8WQBQomNwoNgUEKwfI3ncLgDAVEuGaggyRuyP5htgqkhqqOAQpIJVJW9WciQheEHqRxD8jDfklKjTQ7LcLv4uwtZLbL18EDFyEROLmAluAk1PpGecLrfn9QOheWEgWEdR8y8VTKhr_8ial0jllEMQOY4eZhztlqOgySjo4x34SE4uH6t7IQIgEcaU54P8A7BD6T5XouL42uCpXRcKEwVBLI9AC0bxlRVxrAKFMK10z9PajWw9HBliNOcBNaTUEKSEG6K8iHpq89nwkitxgCDeeuT3XUK6_Hr_mHYuOBuL81qgSbhSpGw2Fq_nicPk_Gua100nQ1xwYv1XSjcHkIKeWRxPcps_KTRtBaUIQ4a_WCCjED_f0MAUZjoixZ1oSjfd6sB2fDGfZYuFztrLCRKqakhiMKVGwMXJkCMiQPBzp50WEKzWsF230g9JPVyuH7mte2hiYGc_BWWw6LNh0SKcOOnVwwL8ULUTDqeu1sw1WjHCL4hL6Tn6erIHlKd6LxzJF8SG_VTOylY4vA06D_ulG92j95cv4WUoZKyumdR_Slrttvkd7_cihvrjw726rF6PF3X4_yNFSOvK2vvXdbDYth5JOYtZpiJtNJtZbQuB1GZBRWVYZSTc4U4KFAylJJCMDwp1NT1_Wuy330Pfve3xfg1Pb2cIbgcZ2XVn1tW7dzSTk6asAy_tjiDlHEp8roGeIfVj1wDbFyffo28XTc1IxBQgxhNHfRypk1PCjj2EnDsL5L3Cs9T1bzbYSlvukIwMyv_d5w0WIMj1TgLqC_zK4vc5Ul8_jPpPASO8off_wCSM6_IeQqKieZplww5msDOXplQJQSVlESlSNuhixFjiAp0U9N2WgqjVfPauBzw911eaDeOLp0BduKmm0tL7aXq4xcUOnIj1F5imEj1ZK8037RXWgr_ARKDTuPyEgAA_22_7D = __webpack_require__(19892);
;// ./src/components/settings/BacklogSourcesSettings.css.ts


var actionRow = 'obmp9gc';
var addBtn = 'obmp9gk';
var authWarning = 'obmp9gn';
var bothDirectionsWarning = 'obmp9gr';
var container = 'obmp9g0';
var description = 'obmp9g3';
var empty = 'obmp9gl';
var errorMessage = 'obmp9gm';
var BacklogSourcesSettings_css_form = 'obmp9gg';
var formRow = 'obmp9gh';
var historyList = 'obmp9gf';
var input = 'obmp9gi';
var list = 'obmp9g4';
var listItem = 'obmp9g5';
var listItemHeader = 'obmp9g6';
var listItemMeta = 'obmp9g8';
var listItemName = 'obmp9g7';
var previewError = 'obmp9gt';
var previewPendingLabel = 'obmp9gs';
var removeBtn = 'obmp9ge';
var section = 'obmp9g1';
var sectionTitle = 'obmp9g2';
var BacklogSourcesSettings_css_select = 'obmp9gj';
var smallBtn = 'obmp9gd';
var subHeading = 'obmp9gp';
var syncDirectionGroup = 'obmp9go';
var syncDirectionRow = 'obmp9gq';
var toggle = 'obmp9g9';
var toggleOn = 'obmp9ga';
var togglePending = 'obmp9gb';
;// ./src/components/settings/BacklogSourcesSettings.tsx
/* __next_internal_client_entry_do_not_use__ BacklogSourcesSettings auto */ 







function formatDate(iso) {
    return iso ? new Date(iso).toLocaleString() : "never";
}
/**
 * Settings panel for configuring backlog item sources (e.g. GitHub repos to
 * sync issues/PRs from): add a source, enable/disable, sync now, view history.
 */ // Non-transient sync failures (auth revoked/expired) warrant a persistent,
// row-level warning that doesn't require expanding history to notice
// (Story 4.3.2) — a rate limit or a one-off network blip does not.
//
// A bare "403" match used to be enough, but GitHub's rate-limit response is
// ALSO a 403 (session/backlog_plugin_github.go's Fetch/CloseIssue render it
// as "github_issues: rate limited (status 403)" / "... rate limited closing
// issue ..."), so a bare "403" false-positives on transient rate-limiting.
// Match on 401 (GitHub never uses 401 for rate limiting) and specific auth
// phrasing instead, and explicitly exclude anything already identified as a
// rate limit.
function isAuthFailure(errorMessage) {
    if (!errorMessage) return false;
    const lower = errorMessage.toLowerCase();
    if (lower.includes("rate limit")) return false;
    return lower.includes("401") || lower.includes("403") || lower.includes("bad credentials") || lower.includes("revoked") || lower.includes("requires authentication");
}
function BacklogSourcesSettings() {
    const { listItemSources, createItemSource, setItemSourceEnabled, setForwardSyncEnabled, setBackwardSyncEnabled, setForwardSyncCloseLabel, deleteItemSource, triggerSync, getSyncHistory, previewBackwardSyncImpact, lastError, clearError } = useBacklogSourcesService();
    const [sources, setSources] = (0,react.useState)([]);
    const [loading, setLoading] = (0,react.useState)(true);
    const [syncingId, setSyncingId] = (0,react.useState)(null);
    // Per-source in-flight guards for the enabled/forward-sync toggles — same
    // pattern as backwardSyncPreviewPendingId below, added to close a
    // stale-closure double-click bug: without this, a rapid second click
    // reads the same (still-current) `source.enabled` / `source.forwardSyncEnabled`
    // prop and sends the same target value twice instead of toggling back.
    const [enabledTogglePendingId, setEnabledTogglePendingId] = (0,react.useState)(null);
    const [forwardSyncTogglePendingId, setForwardSyncTogglePendingId] = (0,react.useState)(null);
    const [historyBySource, setHistoryBySource] = (0,react.useState)({});
    const [expandedId, setExpandedId] = (0,react.useState)(null);
    // Local in-progress edits to the close-label input, keyed by source id —
    // committed to the backend on blur rather than per keystroke.
    const [closeLabelDrafts, setCloseLabelDrafts] = (0,react.useState)({});
    // Epic 4.4: first-enable-of-backward-sync confirmation. sourceId of the
    // toggle currently awaiting PreviewBackwardSyncImpact — shows a
    // pending/disabled toggle state while true (UX round 2: visibility of
    // system status).
    const [backwardSyncPreviewPendingId, setBackwardSyncPreviewPendingId] = (0,react.useState)(null);
    // Distinct, per-row inline error when the preview RPC itself fails — kept
    // separate from the shared top-level `lastError` banner so it's directly
    // testable and scoped to the row that triggered it.
    const [backwardSyncPreviewErrors, setBackwardSyncPreviewErrors] = (0,react.useState)({});
    // Pending confirmation dialog state: set once a preview returns a nonzero
    // itemCount; the toggle only actually flips (setBackwardSyncEnabled) on
    // Confirm — never as a side effect of the preview call itself.
    const [backwardSyncConfirm, setBackwardSyncConfirm] = (0,react.useState)(null);
    const [pluginId, setPluginId] = (0,react.useState)(PLUGIN_SCHEMAS[0].id);
    const [displayName, setDisplayName] = (0,react.useState)("");
    const [fieldValues, setFieldValues] = (0,react.useState)({});
    const [token, setToken] = (0,react.useState)("");
    const [submitting, setSubmitting] = (0,react.useState)(false);
    var _PLUGIN_SCHEMAS_find;
    const schema = (_PLUGIN_SCHEMAS_find = PLUGIN_SCHEMAS.find((s)=>s.id === pluginId)) !== null && _PLUGIN_SCHEMAS_find !== void 0 ? _PLUGIN_SCHEMAS_find : PLUGIN_SCHEMAS[0];
    const handlePluginChange = (id)=>{
        setPluginId(id);
        setFieldValues({});
        setToken("");
    };
    const setField = (key, value)=>{
        setFieldValues((prev)=>({
                ...prev,
                [key]: value
            }));
    };
    const refresh = (0,react.useCallback)(async ()=>{
        const list = await listItemSources();
        setSources(list);
        setLoading(false);
        // Eagerly load each source's sync history so the row-level
        // non-transient-failure warning (Story 4.3.2) can render without the
        // user needing to expand history first.
        const histories = await Promise.all(list.map((s)=>getSyncHistory(s.id)));
        setHistoryBySource((prev)=>{
            const next = {
                ...prev
            };
            list.forEach((s, i)=>{
                next[s.id] = histories[i];
            });
            return next;
        });
    }, [
        listItemSources,
        getSyncHistory
    ]);
    (0,react.useEffect)(()=>{
        refresh();
    }, [
        refresh
    ]);
    const canSubmit = Boolean(displayName.trim()) && schema.fields.every((f)=>{
        var _fieldValues_f_key;
        return Boolean((_fieldValues_f_key = fieldValues[f.key]) === null || _fieldValues_f_key === void 0 ? void 0 : _fieldValues_f_key.trim());
    }) && (!schema.requiresToken || schema.credentialsManagedExternally || Boolean(token.trim())) && !submitting;
    const handleAddSource = async ()=>{
        if (!canSubmit) return;
        setSubmitting(true);
        clearError();
        const config = {};
        for (const f of schema.fields){
            var _fieldValues_f_key;
            var _fieldValues_f_key_trim;
            config[f.key] = (_fieldValues_f_key_trim = (_fieldValues_f_key = fieldValues[f.key]) === null || _fieldValues_f_key === void 0 ? void 0 : _fieldValues_f_key.trim()) !== null && _fieldValues_f_key_trim !== void 0 ? _fieldValues_f_key_trim : "";
        }
        const created = await createItemSource({
            pluginId,
            displayName: displayName.trim(),
            configJson: JSON.stringify(config),
            token: schema.requiresToken && !schema.credentialsManagedExternally ? token.trim() : ""
        });
        setSubmitting(false);
        if (created) {
            setDisplayName("");
            setFieldValues({});
            setToken("");
            await refresh();
        }
    };
    const handleToggleEnabled = async (source)=>{
        if (enabledTogglePendingId === source.id) return;
        setEnabledTogglePendingId(source.id);
        try {
            const updated = await setItemSourceEnabled(source, !source.enabled);
            if (updated) await refresh();
        } finally{
            setEnabledTogglePendingId(null);
        }
    };
    const handleToggleForwardSync = async (source)=>{
        if (forwardSyncTogglePendingId === source.id) return;
        setForwardSyncTogglePendingId(source.id);
        try {
            const updated = await setForwardSyncEnabled(source, !source.forwardSyncEnabled);
            if (updated) await refresh();
        } finally{
            setForwardSyncTogglePendingId(null);
        }
    };
    // Turning backward sync OFF still flips directly on click — only turning
    // it ON routes through PreviewBackwardSyncImpact + a confirmation dialog
    // (Epic 4.4), since only enabling can immediately bulk-archive items.
    const handleToggleBackwardSync = async (source)=>{
        if (source.backwardSyncEnabled) {
            const updated = await setBackwardSyncEnabled(source, false);
            if (updated) await refresh();
            return;
        }
        setBackwardSyncPreviewErrors((prev)=>{
            const next = {
                ...prev
            };
            delete next[source.id];
            return next;
        });
        setBackwardSyncPreviewPendingId(source.id);
        const preview = await previewBackwardSyncImpact(source.id);
        setBackwardSyncPreviewPendingId(null);
        if (preview === null) {
            setBackwardSyncPreviewErrors((prev)=>({
                    ...prev,
                    [source.id]: "Couldn't check impact — try again"
                }));
            return;
        }
        if (preview.itemCount === 0 && !preview.possiblyIncomplete) {
            // No impact to preview, and the fetch was exhaustive — flip
            // immediately, no dialog (don't add friction where there's nothing to
            // warn about). If possiblyIncomplete is true, itemCount: 0 is not
            // trustworthy as "nothing to warn about" — the underlying fetch hit
            // its page cap, so older closed issues could exist beyond what was
            // sampled (the exact under-reporting failure mode PreviewBackwardSyncImpact
            // exists to prevent) — fall through to the confirmation dialog instead
            // of silently skipping it.
            const updated = await setBackwardSyncEnabled(source, true);
            if (updated) await refresh();
            return;
        }
        setBackwardSyncConfirm({
            source,
            itemCount: preview.itemCount,
            sampleTitles: preview.sampleTitles,
            possiblyIncomplete: preview.possiblyIncomplete
        });
    };
    const handleConfirmBackwardSync = async ()=>{
        if (!backwardSyncConfirm) return;
        const { source } = backwardSyncConfirm;
        setBackwardSyncConfirm(null);
        const updated = await setBackwardSyncEnabled(source, true);
        if (updated) await refresh();
    };
    const handleCancelBackwardSync = ()=>{
        setBackwardSyncConfirm(null);
    };
    const handleCloseLabelChange = async (source, closeLabel)=>{
        const updated = await setForwardSyncCloseLabel(source, closeLabel);
        if (updated) {
            await refresh();
            // Clear the local draft now that the server has the committed value —
            // otherwise closeLabelValue's `closeLabelDrafts[source.id] ?? ...`
            // fallback keeps pinning to this stale local value forever and the
            // input never reconciles with server truth again (e.g. after another
            // client changes it).
            setCloseLabelDrafts((prev)=>{
                const next = {
                    ...prev
                };
                delete next[source.id];
                return next;
            });
        }
    };
    const handleDelete = async (source)=>{
        if (await deleteItemSource(source.id)) await refresh();
    };
    const handleSyncNow = async (source)=>{
        setSyncingId(source.id);
        const ok = await triggerSync(source.id);
        setSyncingId(null);
        if (ok) {
            await refresh();
            if (expandedId === source.id) {
                setHistoryBySource((prev)=>({
                        ...prev,
                        [source.id]: {
                            events: [],
                            truncated: false
                        }
                    }));
                const result = await getSyncHistory(source.id);
                setHistoryBySource((prev)=>({
                        ...prev,
                        [source.id]: result
                    }));
            }
        }
    };
    const handleToggleHistory = async (source)=>{
        if (expandedId === source.id) {
            setExpandedId(null);
            return;
        }
        setExpandedId(source.id);
        if (!historyBySource[source.id]) {
            const result = await getSyncHistory(source.id);
            setHistoryBySource((prev)=>({
                    ...prev,
                    [source.id]: result
                }));
        }
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
                className: section,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: sectionTitle,
                        children: "Backlog Sources"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: description,
                        children: "Sync backlog items from external sources like GitHub issues and pull requests."
                    }),
                    lastError && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: errorMessage,
                        children: lastError.message
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: list,
                        children: loading ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: empty,
                            children: "Loading…"
                        }) : sources.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: empty,
                            children: "No sources configured."
                        }) : sources.map((source)=>{
                            var _historyBySource_source_id_events_, _historyBySource_source_id_events, _historyBySource_source_id, _historyBySource_source_id1;
                            const mostRecentSyncError = (_historyBySource_source_id = historyBySource[source.id]) === null || _historyBySource_source_id === void 0 ? void 0 : (_historyBySource_source_id_events = _historyBySource_source_id.events) === null || _historyBySource_source_id_events === void 0 ? void 0 : (_historyBySource_source_id_events_ = _historyBySource_source_id_events[0]) === null || _historyBySource_source_id_events_ === void 0 ? void 0 : _historyBySource_source_id_events_.errorMessage;
                            var _closeLabelDrafts_source_id;
                            const closeLabelValue = (_closeLabelDrafts_source_id = closeLabelDrafts[source.id]) !== null && _closeLabelDrafts_source_id !== void 0 ? _closeLabelDrafts_source_id : source.forwardSyncCloseLabel;
                            var _historyBySource_source_id_events1;
                            return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: listItem,
                                "data-testid": "source-row-".concat(source.id),
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: listItemHeader,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: listItemName,
                                                children: source.displayName
                                            }),
                                            isAuthFailure(mostRecentSyncError) && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: authWarning,
                                                "data-testid": "source-row-".concat(source.id, "-auth-warning"),
                                                role: "alert",
                                                children: "⚠ Sync failing — check credentials"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: listItemMeta,
                                                children: source.pluginId
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                role: "switch",
                                                "aria-checked": source.enabled,
                                                "aria-busy": enabledTogglePendingId === source.id,
                                                disabled: enabledTogglePendingId === source.id,
                                                className: "".concat(toggle, " ").concat(source.enabled ? toggleOn : "", " ").concat(enabledTogglePendingId === source.id ? togglePending : ""),
                                                onClick: ()=>handleToggleEnabled(source),
                                                "aria-label": "".concat(source.enabled ? "Disable" : "Enable", " ").concat(source.displayName)
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                className: removeBtn,
                                                onClick: ()=>handleDelete(source),
                                                "aria-label": "Remove ".concat(source.displayName),
                                                children: "✕"
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                        className: listItemMeta,
                                        children: [
                                            "Last synced: ",
                                            formatDate(source.lastSyncedAt)
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: syncDirectionGroup,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: subHeading,
                                                children: "Sync with GitHub"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: syncDirectionRow,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                        role: "switch",
                                                        "aria-checked": source.forwardSyncEnabled,
                                                        "aria-busy": forwardSyncTogglePendingId === source.id,
                                                        disabled: forwardSyncTogglePendingId === source.id,
                                                        className: "".concat(toggle, " ").concat(source.forwardSyncEnabled ? toggleOn : "", " ").concat(forwardSyncTogglePendingId === source.id ? togglePending : ""),
                                                        onClick: ()=>handleToggleForwardSync(source),
                                                        "aria-label": "".concat(source.forwardSyncEnabled ? "Disable" : "Enable", " closing GitHub issues when done")
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        children: "Close GitHub issues when I finish here"
                                                    })
                                                ]
                                            }),
                                            source.forwardSyncEnabled && /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                type: "text",
                                                className: input,
                                                placeholder: "Label to apply on close (optional)",
                                                "aria-label": "Close label for ".concat(source.displayName),
                                                value: closeLabelValue,
                                                onChange: (e)=>setCloseLabelDrafts((prev)=>({
                                                            ...prev,
                                                            [source.id]: e.target.value
                                                        })),
                                                onBlur: ()=>handleCloseLabelChange(source, closeLabelValue)
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: syncDirectionRow,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                        role: "switch",
                                                        "aria-checked": source.backwardSyncEnabled,
                                                        "aria-busy": backwardSyncPreviewPendingId === source.id,
                                                        disabled: backwardSyncPreviewPendingId === source.id,
                                                        className: "".concat(toggle, " ").concat(source.backwardSyncEnabled ? toggleOn : "", " ").concat(backwardSyncPreviewPendingId === source.id ? togglePending : ""),
                                                        onClick: ()=>handleToggleBackwardSync(source),
                                                        "aria-label": "".concat(source.backwardSyncEnabled ? "Disable" : "Enable", " reflecting GitHub status back")
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        children: "Reflect GitHub status back here"
                                                    }),
                                                    backwardSyncPreviewPendingId === source.id && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: previewPendingLabel,
                                                        children: "Checking impact…"
                                                    })
                                                ]
                                            }),
                                            backwardSyncPreviewErrors[source.id] && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                                className: previewError,
                                                role: "alert",
                                                "data-testid": "source-row-".concat(source.id, "-preview-error"),
                                                children: backwardSyncPreviewErrors[source.id]
                                            }),
                                            source.forwardSyncEnabled && source.backwardSyncEnabled && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                                className: bothDirectionsWarning,
                                                children: "Both directions are enabled — closing this item's issue may be observed and re-applied by backward sync. Verify this doesn't create a loop for items you also edit manually."
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: actionRow,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                className: smallBtn,
                                                onClick: ()=>handleSyncNow(source),
                                                disabled: syncingId === source.id || !source.enabled,
                                                title: !source.enabled ? "Enable this source to sync it" : undefined,
                                                children: syncingId === source.id ? "Syncing…" : "Sync now"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                className: smallBtn,
                                                onClick: ()=>handleToggleHistory(source),
                                                children: expandedId === source.id ? "Hide history" : "View history"
                                            })
                                        ]
                                    }),
                                    expandedId === source.id && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                        className: historyList,
                                        children: ((_historyBySource_source_id_events1 = (_historyBySource_source_id1 = historyBySource[source.id]) === null || _historyBySource_source_id1 === void 0 ? void 0 : _historyBySource_source_id1.events) !== null && _historyBySource_source_id_events1 !== void 0 ? _historyBySource_source_id_events1 : []).length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: empty,
                                            children: "No sync runs yet."
                                        }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                            children: [
                                                historyBySource[source.id].events.map((ev)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                        children: [
                                                            formatDate(ev.startedAt),
                                                            " — created ",
                                                            ev.itemsCreated,
                                                            ", updated ",
                                                            ev.itemsUpdated,
                                                            ", skipped",
                                                            " ",
                                                            ev.itemsSkipped,
                                                            ev.itemsErrored > 0 && ", errored ".concat(ev.itemsErrored),
                                                            ev.errorMessage && " (".concat(ev.errorMessage, ")")
                                                        ]
                                                    }, ev.id)),
                                                historyBySource[source.id].truncated && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                    className: empty,
                                                    children: [
                                                        "Older sync history exists but is not shown (limited to the most recent",
                                                        " ",
                                                        historyBySource[source.id].events.length,
                                                        " runs)."
                                                    ]
                                                })
                                            ]
                                        })
                                    })
                                ]
                            }, source.id);
                        })
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
                className: section,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: sectionTitle,
                        children: "Add a Source"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: BacklogSourcesSettings_css_form,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("select", {
                                className: BacklogSourcesSettings_css_select,
                                value: pluginId,
                                onChange: (e)=>handlePluginChange(e.target.value),
                                children: PLUGIN_SCHEMAS.map((s)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                        value: s.id,
                                        children: s.label
                                    }, s.id))
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                type: "text",
                                className: input,
                                placeholder: "Display name (e.g. My Repo Issues)",
                                value: displayName,
                                onChange: (e)=>setDisplayName(e.target.value)
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: formRow,
                                children: schema.fields.map((f)=>{
                                    var _fieldValues_f_key;
                                    return /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        type: "text",
                                        className: input,
                                        placeholder: f.placeholder,
                                        "aria-label": f.label,
                                        value: (_fieldValues_f_key = fieldValues[f.key]) !== null && _fieldValues_f_key !== void 0 ? _fieldValues_f_key : "",
                                        onChange: (e)=>setField(f.key, e.target.value)
                                    }, f.key);
                                })
                            }),
                            schema.requiresToken && schema.credentialsHelp ? /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                className: description,
                                children: schema.credentialsHelp
                            }) : schema.requiresToken && schema.credentialsManagedExternally ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                                className: description,
                                children: [
                                    "This source authenticates using a connected GitHub account. Connect one under",
                                    " ",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(nav/* Link */.N_, {
                                        href: routes/* routes */.J.unfinished,
                                        children: "GitHub accounts"
                                    }),
                                    " if you haven't already."
                                ]
                            }) : schema.requiresToken && /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                type: "password",
                                className: input,
                                placeholder: schema.tokenLabel,
                                value: token,
                                onChange: (e)=>setToken(e.target.value)
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: addBtn,
                                onClick: handleAddSource,
                                disabled: !canSubmit,
                                children: submitting ? "Adding…" : "Add Source"
                            })
                        ]
                    })
                ]
            }),
            backwardSyncConfirm && /*#__PURE__*/ (0,jsx_runtime.jsx)(BackwardSyncConfirmDialog, {
                sourceDisplayName: backwardSyncConfirm.source.displayName,
                itemCount: backwardSyncConfirm.itemCount,
                sampleTitles: backwardSyncConfirm.sampleTitles,
                possiblyIncomplete: backwardSyncConfirm.possiblyIncomplete,
                onConfirm: handleConfirmBackwardSync,
                onCancel: handleCancelBackwardSync
            })
        ]
    });
}


/***/ }),

/***/ 63367:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   RequireFeatureFlag: () => (/* binding */ RequireFeatureFlag),
/* harmony export */   h: () => (/* binding */ useFeatureFlags),
/* harmony export */   j$: () => (/* binding */ FeatureFlagsProvider),
/* harmony export */   u: () => (/* binding */ useFeatureFlag)
/* harmony export */ });
/* unused harmony export readbackMismatch */
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(61574);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(55494);
/* harmony import */ var _lib_nav__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(41078);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__ = __webpack_require__(66144);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(21694);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(23575);
/* harmony import */ var _featureFlagScopes__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(80070);
/* __next_internal_client_entry_do_not_use__ readbackMismatch,FeatureFlagsProvider,useFeatureFlags,useFeatureFlag,RequireFeatureFlag auto */ 






function scopesOf(flag) {
    return Object.fromEntries(flag.scopes.map((s)=>[
            s.scope,
            s.enabled
        ]));
}
function metaOf(f) {
    return {
        name: f.name,
        enabled: f.enabled,
        description: f.description,
        statusDetail: f.statusDetail,
        scopes: scopesOf(f)
    };
}
/** What the server must have persisted for `change`, or null when the read-back agrees. */ function readbackMismatch(change, flag) {
    if (change.mutation === "reset") return null;
    if (change.scope === _featureFlagScopes__WEBPACK_IMPORTED_MODULE_5__/* .GLOBAL_SCOPE */ .N) {
        return change.mutation === "set" && flag.enabled !== change.enabled ? "global value" : null;
    }
    const entry = flag.scopes.find((s)=>s.scope === change.scope);
    if (change.mutation === "clear") return entry ? change.scope : null;
    return (entry === null || entry === void 0 ? void 0 : entry.enabled) === change.enabled ? null : change.scope;
}
const FeatureFlagsContext = /*#__PURE__*/ (0,react__WEBPACK_IMPORTED_MODULE_1__.createContext)({
    flags: {},
    flagList: [],
    isLoading: true,
    error: null,
    setFlag: async ()=>{},
    refresh: async ()=>{}
});
function FeatureFlagsProvider(param) {
    let { children } = param;
    const [flags, setFlags] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)({});
    const [flagList, setFlagList] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)([]);
    const [isLoading, setIsLoading] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(true);
    const [error, setError] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(null);
    const client = (0,react__WEBPACK_IMPORTED_MODULE_1__.useMemo)(()=>(0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_4__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_3__/* .getConnectTransport */ .qW)()), []);
    const fetchFlags = (0,react__WEBPACK_IMPORTED_MODULE_1__.useCallback)(async ()=>{
        try {
            const res = await client.getFeatureFlags({});
            const map = {};
            const list = [];
            for (const f of res.flags){
                map[f.name] = f.enabled;
                list.push(metaOf(f));
            }
            setFlags(map);
            setFlagList(list);
            setError(null);
        } catch (e) {
            setError("Failed to load feature flags");
        } finally{
            setIsLoading(false);
        }
    }, [
        client
    ]);
    (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        fetchFlags();
    }, [
        fetchFlags
    ]);
    // The displayed state is always the server's read-back, never an optimistic
    // value: a failed or ignored change leaves the previous state and re-reads it.
    const setFlag = (0,react__WEBPACK_IMPORTED_MODULE_1__.useCallback)(async (name, change)=>{
        const req = typeof change === "boolean" ? {
            name,
            enabled: change
        } : change.mutation === "set" ? {
            name,
            scope: change.scope,
            mutation: change.enabled ? _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_4__/* .FlagMutation */ .s34.SET_ENABLED : _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_4__/* .FlagMutation */ .s34.SET_DISABLED
        } : change.mutation === "clear" ? {
            name,
            scope: change.scope,
            mutation: _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_4__/* .FlagMutation */ .s34.CLEAR_SCOPE
        } : {
            name,
            mutation: _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_4__/* .FlagMutation */ .s34.RESET_GLOBAL
        };
        try {
            const res = await client.updateFeatureFlag(req);
            const flag = res.flag;
            if (!flag) throw new Error("response carried no flag");
            if (typeof change !== "boolean") {
                const bad = readbackMismatch(change, flag);
                if (bad) {
                    // An older server that ignores `scope` answers with success and no entry.
                    throw new Error("server read-back does not show the change for ".concat(bad));
                }
            }
            setFlags((prev)=>({
                    ...prev,
                    [flag.name]: flag.enabled
                }));
            setFlagList((prev)=>prev.map((f)=>f.name === flag.name ? {
                        ...f,
                        ...metaOf(flag)
                    } : f));
        } catch (err) {
            console.error("Failed to update feature flag", name, err);
            // Re-read first: a successful fetch clears the error, and this one must outlive it.
            await fetchFlags();
            setError("Failed to update feature flag");
        }
    }, [
        client,
        fetchFlags
    ]);
    const value = (0,react__WEBPACK_IMPORTED_MODULE_1__.useMemo)(()=>({
            flags,
            flagList,
            isLoading,
            error,
            setFlag,
            refresh: fetchFlags
        }), [
        flags,
        flagList,
        isLoading,
        error,
        setFlag,
        fetchFlags
    ]);
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(FeatureFlagsContext.Provider, {
        value: value,
        children: children
    });
}
function useFeatureFlags() {
    return (0,react__WEBPACK_IMPORTED_MODULE_1__.useContext)(FeatureFlagsContext);
}
function useFeatureFlag(name) {
    const { flags } = (0,react__WEBPACK_IMPORTED_MODULE_1__.useContext)(FeatureFlagsContext);
    var _flags_name;
    return (_flags_name = flags[name]) !== null && _flags_name !== void 0 ? _flags_name : false;
}
/**
 * Gates `children` behind a feature flag: renders nothing (and redirects to
 * `redirectTo`) while the flag is disabled or still loading; renders
 * `children` once the flag is confirmed enabled.
 *
 * Intentionally usable from more than one place in a route's render tree
 * (e.g. a segment's `layout.tsx` AND its `page.tsx`) — that's not
 * duplication, it's defense in depth: the layout keeps a disabled flag from
 * ever mounting the page's client bundle, and the same check inside the
 * page guarantees the page's own render output can never show gated content
 * even if it's ever rendered outside that layout (unit tests, future route
 * restructuring). Using a Server Component `page.tsx` to render this Client
 * Component also lets the page keep a static `metadata` export, which a
 * fully-client page cannot do.
 */ function RequireFeatureFlag(param) {
    let { flag, redirectTo = "/", children } = param;
    const { flags, isLoading } = useFeatureFlags();
    const router = (0,_lib_nav__WEBPACK_IMPORTED_MODULE_2__/* .useRouter */ .KA)();
    (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        if (!isLoading && !flags[flag]) {
            router.replace(redirectTo);
        }
    }, [
        isLoading,
        flags,
        flag,
        redirectTo,
        router
    ]);
    if (isLoading) return null;
    if (!flags[flag]) return null;
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.Fragment, {
        children: children
    });
}


/***/ }),

/***/ 65916:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 76514:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  J: () => (/* reexport */ routes)
});

;// ./src/lib/nav/paths.ts
/**
 * Centralized route definitions for type-safe navigation (re-exported from `@/lib/routes`)
 */ const routes = {
    home: "/",
    sessionCreate: "/?new=true",
    reviewQueue: "/review-queue",
    unfinished: "/unfinished",
    rules: "/rules",
    history: "/history",
    logs: "/logs",
    errors: "/errors",
    config: "/config",
    notifications: "/notifications",
    settings: "/settings",
    settingsDefaults: "/settings/defaults",
    help: "/help",
    changelog: "/changelog",
    settingsUnfinished: "/settings/unfinished",
    insights: "/insights",
    settingsFeatures: "/settings/features",
    settingsBacklogSources: "/settings/backlog-sources",
    settingsBacklogStages: "/settings/backlog-stages",
    settingsPipelineModes: "/settings/pipeline-modes",
    settingsRemotes: "/settings/remotes",
    settingsJules: "/settings/jules",
    settingsTaggingClassifier: "/settings/tagging-classifier",
    settingsLlmBackends: "/settings/llm-backends",
    settingsBackgroundModels: "/settings/background-models",
    backlog: "/backlog",
    backlogBoard: "/backlog/board",
    sessionsImport: "/sessions/import",
    workflows: "/workflows",
    triggers: "/triggers",
    login: "/login",
    account: "/account",
    escapeAnalytics: "/analytics/escape",
    files: "/files",
    sessionDetail: (id)=>"/?session=".concat(id),
    newSessionFromWorktree: (worktreePath, branch, title)=>{
        const params = new URLSearchParams({
            worktree: worktreePath,
            branch
        });
        if (title) params.set("title", title);
        return "/?".concat(params.toString());
    },
    unfinishedItem: (itemId)=>"/unfinished?item=".concat(encodeURIComponent(itemId)),
    unfinishedTab: (tab)=>"/unfinished?tab=".concat(encodeURIComponent(tab))
};
/** Every `page.tsx` under src/app, one literal each; `route-meta.test.ts` fails when this list drifts from the directory. */ const ROUTE_PATHS = (/* unused pure expression or super */ null && ([
    "/",
    "/account",
    "/analytics/escape",
    "/backlog",
    "/backlog/board",
    "/changelog",
    "/config",
    "/debug/escape-codes",
    "/errors",
    "/fanout",
    "/files",
    "/help",
    "/history",
    "/insights",
    "/insights/session-detail",
    "/login",
    "/logs",
    "/notifications",
    "/resolve",
    "/review-queue",
    "/rules",
    "/sessions/import",
    "/sessions/new",
    "/sessions/summary",
    "/settings",
    "/settings/background-models",
    "/settings/backlog-sources",
    "/settings/backlog-stages",
    "/settings/defaults",
    "/settings/features",
    "/settings/jules",
    "/settings/llm-backends",
    "/settings/pipeline-modes",
    "/settings/remotes",
    "/settings/tagging-classifier",
    "/settings/unfinished",
    "/spike-files",
    "/spike-lazydoc",
    "/test/escape-codes",
    "/test/layout-overlap",
    "/test/terminal-stress",
    "/triggers",
    "/unfinished",
    "/workflows"
]));

;// ./src/lib/routes.ts
/**
 * Centralized route definitions for type-safe navigation.
 * The data lives in `@/lib/nav/paths`; this path stays for existing importers.
 */ 


/***/ }),

/***/ 80070:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   N: () => (/* binding */ GLOBAL_SCOPE)
/* harmony export */ });
/** The scope literal a client sends for a flag's global value (mirrors the server's globalScopeLiteral). */ const GLOBAL_SCOPE = "global";


/***/ }),

/***/ 82279:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   L: () => (/* binding */ useDialogFocusTrap)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);

const FOCUSABLE_SELECTOR = 'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';
/**
 * Shared modal focus-trap behavior: focuses `initialFocusRef` on mount,
 * restores focus to whatever was focused before the dialog opened once it
 * unmounts, and cycles Tab/Shift+Tab within `dialogRef` while routing
 * Escape to `onEscape`.
 *
 * Extracted from BackwardSyncConfirmDialog and HostKeyTrustDialog, which
 * both implemented this identically apart from which button receives
 * initial focus (Confirm vs Cancel).
 */ function useDialogFocusTrap(param) {
    let { dialogRef, initialFocusRef, onEscape } = param;
    // Capture whatever had focus when the dialog mounted (the element that
    // triggered it) so focus can be restored on close.
    const previouslyFocusedRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(typeof document !== "undefined" ? document.activeElement : null);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        var _initialFocusRef_current;
        (_initialFocusRef_current = initialFocusRef.current) === null || _initialFocusRef_current === void 0 ? void 0 : _initialFocusRef_current.focus();
        return ()=>{
            var _previouslyFocusedRef_current;
            (_previouslyFocusedRef_current = previouslyFocusedRef.current) === null || _previouslyFocusedRef_current === void 0 ? void 0 : _previouslyFocusedRef_current.focus();
        };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- run once on mount/unmount only
    }, []);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        const handleKeyDown = (e)=>{
            if (e.key === "Escape") {
                e.preventDefault();
                onEscape();
                return;
            }
            if (e.key === "Tab" && dialogRef.current) {
                const focusable = Array.from(dialogRef.current.querySelectorAll(FOCUSABLE_SELECTOR));
                if (focusable.length === 0) return;
                const first = focusable[0];
                const last = focusable[focusable.length - 1];
                if (e.shiftKey && document.activeElement === first) {
                    e.preventDefault();
                    last.focus();
                } else if (!e.shiftKey && document.activeElement === last) {
                    e.preventDefault();
                    first.focus();
                }
            }
        };
        document.addEventListener("keydown", handleKeyDown);
        return ()=>document.removeEventListener("keydown", handleKeyDown);
    }, [
        dialogRef,
        onEscape
    ]);
}


/***/ }),

/***/ 85987:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 3728));
;
Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 50060));
;
Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 63367));


/***/ })

},
/******/ __webpack_require__ => { // webpackRuntimeModules
/******/ var __webpack_exec__ = (moduleId) => (__webpack_require__(__webpack_require__.s = moduleId))
/******/ __webpack_require__.O(0, [1869,852,4202,9353,8165,3575,8890,9691,3516,7358], () => (__webpack_exec__(85987)));
/******/ var __webpack_exports__ = __webpack_require__.O();
/******/ _N_E = __webpack_exports__;
/******/ }
]);
//# sourceMappingURL=page-296f962acd2f39af.js.map