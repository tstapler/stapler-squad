(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[8398],{

/***/ 2767:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 3456:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   FS: () => (/* binding */ GATE_KIND_LABELS),
/* harmony export */   _M: () => (/* binding */ useBacklogStages),
/* harmony export */   ll: () => (/* binding */ useBacklogStagesAdmin),
/* harmony export */   oI: () => (/* binding */ isBuiltInStage),
/* harmony export */   ou: () => (/* binding */ BUILTIN_BACKLOG_STAGES)
/* harmony export */ });
/* unused harmony export BUILT_IN_STAGE_SLUGS */
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(66144);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(21694);
/* harmony import */ var _gen_session_v1_backlog_pb__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(79870);
/* __next_internal_client_entry_do_not_use__ BUILTIN_BACKLOG_STAGES,useBacklogStages,BUILT_IN_STAGE_SLUGS,isBuiltInStage,GATE_KIND_LABELS,useBacklogStagesAdmin auto */ 



/**
 * The 9 built-in stages seeded by the backend on boot (session/workflow_stage_seed.go)
 * — used as the synchronous initial value so BacklogBoard/StageTracker render their
 * known columns/labels on first paint instead of flashing empty while ListStages is
 * in flight, and as the safe fallback on a fetch failure. Mirrors
 * CachingLivenessEngine/CachingPipelineEngine's cache-miss-falls-back-to-default
 * precedent (plan.md Pattern Decisions).
 */ const BUILTIN_BACKLOG_STAGES = [
    {
        slug: "idea",
        name: "Idea",
        enabled: true
    },
    {
        slug: "refining",
        name: "Refining",
        enabled: true
    },
    {
        slug: "ready",
        name: "Ready",
        enabled: true
    },
    {
        slug: "queued",
        name: "Queued",
        enabled: true
    },
    {
        slug: "in_progress",
        name: "In Progress",
        enabled: true
    },
    {
        slug: "review",
        name: "Review",
        enabled: true
    },
    {
        slug: "pr_pending",
        name: "PR Pending",
        enabled: true
    },
    {
        slug: "done",
        name: "Done",
        enabled: true
    },
    {
        slug: "archived",
        name: "Archived",
        enabled: true
    }
];
// Module-level cache shared across every hook instance in the page — once one
// mount's ListStages call succeeds, later mounts (e.g. navigating between
// board and item detail) start from that value instead of re-flashing the
// built-in defaults. Cleared only by a page reload, same lifetime as
// pipelineModeCache's in-memory cache.
let cachedStages = null;
function mapStage(p) {
    return {
        slug: p.slug,
        name: p.name,
        enabled: p.enabled
    };
}
/**
 * Fetches the live, operator-configured stage set (built-in + custom) via
 * ListStages, so BacklogBoard's columns and StageTracker's unrecognized-status
 * fallback reflect what's actually configured instead of a hardcoded list.
 *
 * Never blanks the result on a fetch failure — it keeps the last-known (or
 * built-in default) list, since a stage silently disappearing from the board
 * is exactly the BUG-037 failure mode this epic exists to prevent.
 */ function useBacklogStages() {
    const [stages, setStages] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(cachedStages !== null && cachedStages !== void 0 ? cachedStages : [
        ...BUILTIN_BACKLOG_STAGES
    ]);
    const [isLoading, setIsLoading] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(cachedStages === null);
    const [error, setError] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(null);
    const clientRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        let cancelled = false;
        if (!clientRef.current) {
            clientRef.current = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__/* .createClient */ .UU)(_gen_session_v1_backlog_pb__WEBPACK_IMPORTED_MODULE_2__/* .BacklogService */ .jSM, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_1__/* .getConnectTransport */ .qW)());
        }
        clientRef.current.listStages({}).then((resp)=>{
            if (cancelled) return;
            var _resp_items;
            const fetched = ((_resp_items = resp.items) !== null && _resp_items !== void 0 ? _resp_items : []).map(mapStage);
            cachedStages = fetched;
            setStages(fetched);
            setError(null);
        }).catch((err)=>{
            if (cancelled) return;
            console.error("[useBacklogStages] listStages:", err);
            setError(err instanceof Error ? err : new Error("Failed to load backlog stages"));
        }).finally(()=>{
            if (!cancelled) setIsLoading(false);
        });
        return ()=>{
            cancelled = true;
        };
    }, []);
    return {
        stages,
        isLoading,
        error
    };
}
// ---------------------------------------------------------------------------
// useBacklogStagesAdmin — full CRUD surface for the Epic 2.8 settings UI.
// Separate from useBacklogStages above (a read-only, cached list for board/
// detail rendering) since the two were built concurrently for different
// consumers and have incompatible shapes (subscribe-to-a-value vs.
// imperative action functions).
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Domain types exposed to UI (mapped from proto, but without Message<> noise)
// ---------------------------------------------------------------------------
/**
 * The 9 built-in BacklogStatus-derived stages seeded by
 * session.EnsureBuiltInWorkflowStages. Neither BacklogStage nor its proto
 * carries an explicit "is built-in" flag, so the frontend derives it from
 * slug membership in this closed set — matches useBacklogService.ts's
 * KnownBacklogStatus list.
 */ const BUILT_IN_STAGE_SLUGS = new Set([
    "idea",
    "refining",
    "ready",
    "queued",
    "in_progress",
    "review",
    "pr_pending",
    "done",
    "archived"
]);
function isBuiltInStage(slug) {
    return BUILT_IN_STAGE_SLUGS.has(slug);
}
/** Human-readable labels for each GateKind, shared by StageForm and StageGraphDiagram. */ const GATE_KIND_LABELS = {
    human_approval: "human approval",
    automated_review: "automated review",
    structural: "structural check",
    custom: "custom check"
};
function mapFullStage(s) {
    return {
        id: s.id,
        slug: s.slug,
        name: s.name,
        description: s.description,
        isEntry: s.isEntry,
        isTerminal: s.isTerminal,
        enabled: s.enabled
    };
}
function mapGate(g) {
    var _g_config;
    return {
        id: g.id,
        transitionId: g.transitionId,
        kind: g.kind,
        config: (_g_config = g.config) !== null && _g_config !== void 0 ? _g_config : {},
        stateful: g.stateful,
        orderIndex: g.orderIndex,
        enabled: g.enabled
    };
}
function mapTransition(t) {
    var _t_gates;
    return {
        id: t.id,
        fromStageSlug: t.fromStageSlug,
        toStageSlug: t.toStageSlug,
        enabled: t.enabled,
        gates: ((_t_gates = t.gates) !== null && _t_gates !== void 0 ? _t_gates : []).map(mapGate)
    };
}
/**
 * RPC-backed CRUD for the Epic 2.7 stage/transition/gate surface, feeding the
 * Epic 2.8 "Backlog Stages" settings UI (and, per plan.md's Epic 2.9, the
 * board/detail dynamic-stage rendering). Self-contained (its own ConnectRPC
 * client) rather than folded into useBacklogService.ts, so this and other
 * concurrently-built epics don't collide on the same shared hook file.
 */ function useBacklogStagesAdmin() {
    const clientRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        clientRef.current = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__/* .createClient */ .UU)(_gen_session_v1_backlog_pb__WEBPACK_IMPORTED_MODULE_2__/* .BacklogService */ .jSM, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_1__/* .getConnectTransport */ .qW)());
    }, []);
    const listStages = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async ()=>{
        if (!clientRef.current) return [];
        try {
            const resp = await clientRef.current.listStages({});
            var _resp_items;
            return ((_resp_items = resp.items) !== null && _resp_items !== void 0 ? _resp_items : []).map(mapFullStage);
        } catch (err) {
            console.error("[useBacklogStages] listStages:", err);
            throw err;
        }
    }, []);
    const createStage = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (data)=>{
        if (!clientRef.current) throw new Error("Backlog service not connected");
        try {
            var _data_description, _data_isEntry, _data_isTerminal, _data_enabled;
            const resp = await clientRef.current.createStage({
                slug: data.slug,
                name: data.name,
                description: (_data_description = data.description) !== null && _data_description !== void 0 ? _data_description : "",
                isEntry: (_data_isEntry = data.isEntry) !== null && _data_isEntry !== void 0 ? _data_isEntry : false,
                isTerminal: (_data_isTerminal = data.isTerminal) !== null && _data_isTerminal !== void 0 ? _data_isTerminal : false,
                enabled: (_data_enabled = data.enabled) !== null && _data_enabled !== void 0 ? _data_enabled : true
            });
            if (!resp.item) throw new Error("createStage: server returned no item");
            return mapFullStage(resp.item);
        } catch (err) {
            console.error("[useBacklogStages] createStage:", err);
            throw err;
        }
    }, []);
    const updateStage = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id, data)=>{
        if (!clientRef.current) throw new Error("Backlog service not connected");
        try {
            const resp = await clientRef.current.updateStage({
                id,
                name: data.name,
                description: data.description,
                isEntry: data.isEntry,
                isTerminal: data.isTerminal,
                enabled: data.enabled
            });
            if (!resp.item) throw new Error("updateStage: server returned no item");
            return mapFullStage(resp.item);
        } catch (err) {
            console.error("[useBacklogStages] updateStage:", err);
            throw err;
        }
    }, []);
    const deleteStage = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id, force)=>{
        if (!clientRef.current) return false;
        try {
            await clientRef.current.deleteStage({
                id,
                force: force !== null && force !== void 0 ? force : false
            });
            return true;
        } catch (err) {
            console.error("[useBacklogStages] deleteStage:", err);
            throw err;
        }
    }, []);
    const listStageTransitions = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (fromStageSlug)=>{
        if (!clientRef.current) return [];
        try {
            const resp = await clientRef.current.listStageTransitions({
                fromStageSlug
            });
            var _resp_items;
            return ((_resp_items = resp.items) !== null && _resp_items !== void 0 ? _resp_items : []).map(mapTransition);
        } catch (err) {
            console.error("[useBacklogStages] listStageTransitions:", err);
            throw err;
        }
    }, []);
    const createStageTransition = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (data)=>{
        if (!clientRef.current) throw new Error("Backlog service not connected");
        try {
            var _data_enabled;
            const resp = await clientRef.current.createStageTransition({
                fromStageSlug: data.fromStageSlug,
                toStageSlug: data.toStageSlug,
                enabled: (_data_enabled = data.enabled) !== null && _data_enabled !== void 0 ? _data_enabled : true
            });
            if (!resp.item) throw new Error("createStageTransition: server returned no item");
            var _resp_warnings;
            return {
                item: mapTransition(resp.item),
                warnings: (_resp_warnings = resp.warnings) !== null && _resp_warnings !== void 0 ? _resp_warnings : []
            };
        } catch (err) {
            console.error("[useBacklogStages] createStageTransition:", err);
            throw err;
        }
    }, []);
    const updateStageTransition = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id, data)=>{
        if (!clientRef.current) throw new Error("Backlog service not connected");
        try {
            const resp = await clientRef.current.updateStageTransition({
                id,
                enabled: data.enabled
            });
            if (!resp.item) throw new Error("updateStageTransition: server returned no item");
            var _resp_warnings;
            return {
                item: mapTransition(resp.item),
                warnings: (_resp_warnings = resp.warnings) !== null && _resp_warnings !== void 0 ? _resp_warnings : []
            };
        } catch (err) {
            console.error("[useBacklogStages] updateStageTransition:", err);
            throw err;
        }
    }, []);
    const deleteStageTransition = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id)=>{
        if (!clientRef.current) return false;
        try {
            await clientRef.current.deleteStageTransition({
                id
            });
            return true;
        } catch (err) {
            console.error("[useBacklogStages] deleteStageTransition:", err);
            throw err;
        }
    }, []);
    const createTransitionGate = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (data)=>{
        if (!clientRef.current) throw new Error("Backlog service not connected");
        try {
            var _data_config, _data_stateful, _data_orderIndex, _data_enabled;
            const resp = await clientRef.current.createTransitionGate({
                transitionId: data.transitionId,
                kind: data.kind,
                config: (_data_config = data.config) !== null && _data_config !== void 0 ? _data_config : {},
                stateful: (_data_stateful = data.stateful) !== null && _data_stateful !== void 0 ? _data_stateful : false,
                orderIndex: (_data_orderIndex = data.orderIndex) !== null && _data_orderIndex !== void 0 ? _data_orderIndex : 0,
                enabled: (_data_enabled = data.enabled) !== null && _data_enabled !== void 0 ? _data_enabled : true
            });
            if (!resp.item) throw new Error("createTransitionGate: server returned no item");
            return mapGate(resp.item);
        } catch (err) {
            console.error("[useBacklogStages] createTransitionGate:", err);
            throw err;
        }
    }, []);
    const updateTransitionGate = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id, data)=>{
        if (!clientRef.current) throw new Error("Backlog service not connected");
        try {
            var _data_config;
            const resp = await clientRef.current.updateTransitionGate({
                id,
                kind: data.kind,
                config: (_data_config = data.config) !== null && _data_config !== void 0 ? _data_config : {},
                stateful: data.stateful,
                orderIndex: data.orderIndex,
                enabled: data.enabled
            });
            if (!resp.item) throw new Error("updateTransitionGate: server returned no item");
            return mapGate(resp.item);
        } catch (err) {
            console.error("[useBacklogStages] updateTransitionGate:", err);
            throw err;
        }
    }, []);
    const deleteTransitionGate = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id)=>{
        if (!clientRef.current) return false;
        try {
            await clientRef.current.deleteTransitionGate({
                id
            });
            return true;
        } catch (err) {
            console.error("[useBacklogStages] deleteTransitionGate:", err);
            throw err;
        }
    }, []);
    return {
        listStages,
        createStage,
        updateStage,
        deleteStage,
        listStageTransitions,
        createStageTransition,
        updateStageTransition,
        deleteStageTransition,
        createTransitionGate,
        updateTransitionGate,
        deleteTransitionGate
    };
}


/***/ }),

/***/ 17376:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  "default": () => (/* binding */ BacklogStagesPage)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/nav/index.ts
var nav = __webpack_require__(41078);
// EXTERNAL MODULE: ./src/lib/hooks/useBacklogStages.ts
var useBacklogStages = __webpack_require__(3456);
// EXTERNAL MODULE: ./src/components/analytics/PageViewTracker.tsx
var PageViewTracker = __webpack_require__(3728);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/connect-error.js
var connect_error = __webpack_require__(45640);
// EXTERNAL MODULE: ./src/lib/hooks/useBacklogService.ts
var useBacklogService = __webpack_require__(8570);
// EXTERNAL MODULE: ./src/styles/a11y.css.ts
var a11y_css = __webpack_require__(26946);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FStageGraphDiagram.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xNmZob3Y4MCB7CiAgbWFyZ2luOiAwOwogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MnopOwp9Ci5fMTZmaG92ODEgewogIG92ZXJmbG93LXg6IGF1dG87CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnplKTsKICBib3JkZXItcmFkaXVzOiB2YXIoLS1fMTllejJyejM2KTsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejMwKTsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejcpOwp9Ci5fMTZmaG92ODIgewogIGZpbGw6IHZhcigtLV8xOWV6MnJ6Nyk7CiAgc3Ryb2tlOiB2YXIoLS1fMTllejJyemUpOwogIHN0cm9rZS13aWR0aDogMTsKfQouXzE2ZmhvdjgzIHsKICBmaWxsOiB2YXIoLS1fMTllejJyejApOwogIGZvbnQtc2l6ZTogMTFweDsKfQouXzE2Zmhvdjg0IHsKICBzdHJva2U6IHZhcigtLV8xOWV6MnJ6Mik7CiAgc3Ryb2tlLXdpZHRoOiAxOwogIGZpbGw6IG5vbmU7Cn0KLl8xNmZob3Y4NSB7CiAgZmlsbDogdmFyKC0tXzE5ZXoycnoyKTsKICBmb250LXNpemU6IDEwcHg7Cn0KLl8xNmZob3Y4NiB7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNhKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKfQouXzE2Zmhvdjg3IHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2IpOwogIGZvbnQtc3R5bGU6IGl0YWxpYzsKfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FStageGraphDiagram_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xNmZob3Y4MCB7CiAgbWFyZ2luOiAwOwogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MnopOwp9Ci5fMTZmaG92ODEgewogIG92ZXJmbG93LXg6IGF1dG87CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnplKTsKICBib3JkZXItcmFkaXVzOiB2YXIoLS1fMTllejJyejM2KTsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejMwKTsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejcpOwp9Ci5fMTZmaG92ODIgewogIGZpbGw6IHZhcigtLV8xOWV6MnJ6Nyk7CiAgc3Ryb2tlOiB2YXIoLS1fMTllejJyemUpOwogIHN0cm9rZS13aWR0aDogMTsKfQouXzE2ZmhvdjgzIHsKICBmaWxsOiB2YXIoLS1fMTllejJyejApOwogIGZvbnQtc2l6ZTogMTFweDsKfQouXzE2Zmhvdjg0IHsKICBzdHJva2U6IHZhcigtLV8xOWV6MnJ6Mik7CiAgc3Ryb2tlLXdpZHRoOiAxOwogIGZpbGw6IG5vbmU7Cn0KLl8xNmZob3Y4NSB7CiAgZmlsbDogdmFyKC0tXzE5ZXoycnoyKTsKICBmb250LXNpemU6IDEwcHg7Cn0KLl8xNmZob3Y4NiB7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNhKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKfQouXzE2Zmhvdjg3IHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2IpOwogIGZvbnQtc3R5bGU6IGl0YWxpYzsKfQ_22_7D = __webpack_require__(43412);
;// ./src/components/backlog/StageGraphDiagram.css.ts


var caption = '_16fhov86';
var edgeLine = '_16fhov84';
var empty = '_16fhov87';
var figure = '_16fhov80';
var gateBadgeText = '_16fhov85';
var nodeRect = '_16fhov82';
var nodeText = '_16fhov83';
var scrollArea = '_16fhov81';
;// ./src/components/backlog/StageGraphDiagram.tsx
/* __next_internal_client_entry_do_not_use__ StageGraphDiagram auto */ 




const COLUMN_WIDTH = 160;
const ROW_HEIGHT = 56;
const NODE_WIDTH = 128;
const NODE_HEIGHT = 32;
const MARGIN = 16;
/**
 * BFS-rank layered layout (Surface 3, research/ux.md §1/§3): each stage's
 * column is its shortest hop-distance from any `IsEntry` stage. Deliberately
 * not a general graph-drawing algorithm — no manual positioning, no drag,
 * nothing to lay out — so a stage unreachable from any entry stage still gets
 * a defined column (placed one past the furthest reached rank) rather than
 * being silently omitted.
 */ function computeLayout(stages, transitions) {
    const bySlug = new Map(stages.map((s)=>[
            s.slug,
            s
        ]));
    const adjacency = new Map();
    for (const s of stages)adjacency.set(s.slug, []);
    for (const t of transitions){
        var _adjacency_get;
        if (!bySlug.has(t.fromStageSlug) || !bySlug.has(t.toStageSlug)) continue;
        (_adjacency_get = adjacency.get(t.fromStageSlug)) === null || _adjacency_get === void 0 ? void 0 : _adjacency_get.push(t.toStageSlug);
    }
    const rank = new Map();
    const entrySlugs = stages.filter((s)=>s.isEntry).map((s)=>s.slug);
    const queue = [
        ...entrySlugs
    ];
    for (const slug of entrySlugs)rank.set(slug, 0);
    while(queue.length > 0){
        const current = queue.shift();
        if (current === undefined) break;
        var _rank_get;
        const currentRank = (_rank_get = rank.get(current)) !== null && _rank_get !== void 0 ? _rank_get : 0;
        var _adjacency_get1;
        for (const next of (_adjacency_get1 = adjacency.get(current)) !== null && _adjacency_get1 !== void 0 ? _adjacency_get1 : []){
            if (!rank.has(next)) {
                rank.set(next, currentRank + 1);
                queue.push(next);
            }
        }
    }
    const maxReachedRank = Math.max(0, ...Array.from(rank.values()));
    const unreachedRank = maxReachedRank + 1;
    for (const s of stages){
        if (!rank.has(s.slug)) rank.set(s.slug, unreachedRank);
    }
    const columns = new Map();
    for (const s of stages){
        var _rank_get1;
        const r = (_rank_get1 = rank.get(s.slug)) !== null && _rank_get1 !== void 0 ? _rank_get1 : unreachedRank;
        var _columns_get;
        const col = (_columns_get = columns.get(r)) !== null && _columns_get !== void 0 ? _columns_get : [];
        col.push(s.slug);
        columns.set(r, col);
    }
    const nodes = [];
    for (const [col, slugs] of Array.from(columns.entries()).sort((a, b)=>a[0] - b[0])){
        slugs.forEach((slug, row)=>{
            const stage = bySlug.get(slug);
            if (!stage) return;
            nodes.push({
                slug,
                name: stage.name,
                x: MARGIN + col * COLUMN_WIDTH,
                y: MARGIN + row * ROW_HEIGHT
            });
        });
    }
    const edges = transitions.filter((t)=>bySlug.has(t.fromStageSlug) && bySlug.has(t.toStageSlug)).map((t)=>{
        var _bySlug_get, _bySlug_get1;
        var _bySlug_get_name, _bySlug_get_name1;
        return {
            fromSlug: t.fromStageSlug,
            toSlug: t.toStageSlug,
            fromName: (_bySlug_get_name = (_bySlug_get = bySlug.get(t.fromStageSlug)) === null || _bySlug_get === void 0 ? void 0 : _bySlug_get.name) !== null && _bySlug_get_name !== void 0 ? _bySlug_get_name : t.fromStageSlug,
            toName: (_bySlug_get_name1 = (_bySlug_get1 = bySlug.get(t.toStageSlug)) === null || _bySlug_get1 === void 0 ? void 0 : _bySlug_get1.name) !== null && _bySlug_get_name1 !== void 0 ? _bySlug_get_name1 : t.toStageSlug,
            gateCount: t.gates.length,
            gateKindLabels: t.gates.map((g)=>{
                var _GATE_KIND_LABELS_g_kind;
                return (_GATE_KIND_LABELS_g_kind = useBacklogStages/* GATE_KIND_LABELS */.FS[g.kind]) !== null && _GATE_KIND_LABELS_g_kind !== void 0 ? _GATE_KIND_LABELS_g_kind : g.kind;
            })
        };
    });
    return {
        nodes,
        edges
    };
}
/**
 * Surface 3's read-only generated graph diagram: computed inline SVG (no
 * graph/node-edge library — see `research/ux.md`'s build-vs-buy note) plus an
 * `sr-only` text-equivalent table, one row per edge, matching the SVG exactly.
 * The SVG is `aria-hidden` because the table — not an `aria-label` summary —
 * is the real text alternative (preserves per-edge gate-kind detail).
 */ function StageGraphDiagram(param) {
    let { stages, transitions } = param;
    const { nodes, edges } = (0,react.useMemo)(()=>computeLayout(stages, transitions), [
        stages,
        transitions
    ]);
    if (stages.length === 0) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
            className: empty,
            children: "No stages configured yet."
        });
    }
    const maxX = Math.max(NODE_WIDTH, ...nodes.map((n)=>n.x)) + NODE_WIDTH + MARGIN;
    const maxY = Math.max(NODE_HEIGHT, ...nodes.map((n)=>n.y)) + NODE_HEIGHT + MARGIN;
    const nodeCenter = (slug)=>{
        const n = nodes.find((node)=>node.slug === slug);
        if (!n) return undefined;
        return {
            x: n.x + NODE_WIDTH / 2,
            y: n.y + NODE_HEIGHT / 2
        };
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("figure", {
        className: figure,
        "aria-label": "Workflow stage graph",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: scrollArea,
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("svg", {
                    "aria-hidden": "true",
                    width: maxX,
                    height: maxY,
                    viewBox: "0 0 ".concat(maxX, " ").concat(maxY),
                    "data-testid": "stage-graph-svg",
                    children: [
                        edges.map((e, i)=>{
                            const from = nodeCenter(e.fromSlug);
                            const to = nodeCenter(e.toSlug);
                            if (!from || !to) return null;
                            return /*#__PURE__*/ (0,jsx_runtime.jsxs)("g", {
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("line", {
                                        x1: from.x,
                                        y1: from.y,
                                        x2: to.x,
                                        y2: to.y,
                                        className: edgeLine
                                    }),
                                    e.gateCount > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("text", {
                                        x: (from.x + to.x) / 2,
                                        y: (from.y + to.y) / 2 - 4,
                                        className: gateBadgeText,
                                        textAnchor: "middle",
                                        children: "\uD83D\uDD12".concat(e.gateCount)
                                    })
                                ]
                            }, "edge-".concat(i));
                        }),
                        nodes.map((n)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("g", {
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("rect", {
                                        x: n.x,
                                        y: n.y,
                                        width: NODE_WIDTH,
                                        height: NODE_HEIGHT,
                                        rx: 4,
                                        className: nodeRect
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("text", {
                                        x: n.x + NODE_WIDTH / 2,
                                        y: n.y + NODE_HEIGHT / 2 + 4,
                                        textAnchor: "middle",
                                        className: nodeText,
                                        children: n.name
                                    })
                                ]
                            }, n.slug))
                    ]
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: caption,
                children: "\uD83D\uDD12N badges mark an edge with N gate(s) attached"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("table", {
                className: a11y_css/* visuallyHidden */.Q,
                "data-testid": "stage-graph-sr-table",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("caption", {
                        children: "Stage transitions and gate counts"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("thead", {
                        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                    children: "From"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                    children: "To"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                    children: "Gates"
                                })
                            ]
                        })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("tbody", {
                        children: edges.map((e, i)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                        children: e.fromName
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                        children: e.toName
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                        children: e.gateCount === 0 ? "0" : "".concat(e.gateCount, " (").concat(e.gateKindLabels.join(", "), ")")
                                    })
                                ]
                            }, "row-".concat(i)))
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2FsettingsResourceForm.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA9WVwW6jMBCG73kKHzcHIkw2aeM8TDXYA7FqPKwxbUPVd1_Fm0jBhIVsK616QoKZbzw_v2dWT_xXWhUdpOx9wZjSTW3gKFhh8G2_YOGZKO1Qek1WMEmmrezpSwm1YC_gfiTJE99hl7luzZf7xcdidWHyL2Bmxz4zC8yCrE8a3eHgCPlyf_n-iro8eMG2aXp6J8mQi-PTPn09QYflKCnrk34GUg1KaVsOYrtlTE4DOQf5XDpqrYpT-J_SOTmFTjBev7GGjFZR2PNVVOJA6bYZNLEdb4JfyTcq73WXoiDZNqHXc9GbXBOw1HqjLQpmyWLEEbUBiQcyCl3A3T6f7NffhFCqQWp_FCxdbUNrrWtOuZZ8AsbQK6pe1va2L8Ho0ibaY9UIJtF6dGOm7PrHePgSIGMVuFLbxNNkxcc77cVn-2Lqal3ErUlfGpoyZTXh7WrckJvhdd6crvO1FOJAL2fTjNewkX5C6QZygyp20GaOg3bfSX4cyO8d2KYGh9bPnYy7WSo_9pMghJ9dbbDwgkHraf8vk_H_aMeHc3mOeDzaBjBLPc7jrE-ZNP826mdj6q-ntuL6rtHxEI2O_HMCy78sqmzq5HzWPufz9b_3n04s-d_wiHxRGgoAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2FsettingsResourceForm_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9WVwW6jMBCG73kKHzcHIkw2aeM8TDXYA7FqPKwxbUPVd1_Fm0jBhIVsK616QoKZbzw_v2dWT_xXWhUdpOx9wZjSTW3gKFhh8G2_YOGZKO1Qek1WMEmmrezpSwm1YC_gfiTJE99hl7luzZf7xcdidWHyL2Bmxz4zC8yCrE8a3eHgCPlyf_n_iro8eMG2aXp6J8mQi_PTPn09QYflKCnrk34GUg1KaVsOYrtlTE4DOQf5XDpqrYpT_J_SOTmFTjBev7GGjFZR2PNVVOJA6bYZNLEdb4JfyTcq73WXoiDZNqHXc9GbXBOw1HqjLQpmyWLEEbUBiQcyCl3A3T6f7NffhFCqQWp_FCxdbUNrrWtOuZZ8AsbQK6pe1va2L8Ho0ibaY9UIJtF6dGOm7PrHePgSIGMVuFLbxNNkxcc77cVn_2Lqal3ErUlfGpoyZTXh7WrckJvhdd6crvO1FOJAL2fTjNewkX5C6QZygyp20GaOg3bfSX4cyO8d2KYGh9bPnYy7WSo_9pMghJ9dbbDwgkHraf8vk_H_aMeHc3mOeDzaBjBLPc7jrE_ZNP826mdj6q_ntuL6rtHxEI2O_HMCy78sqmzq5HzWPufz9b_3n04s_d_wiHxRGgoAAA_22_7D = __webpack_require__(3339);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Fsettings%2Fbacklog-stages%2FStageForm.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA8VUTXOjMAy98yt8XA7uAN2mrfNjOsZWiHeJzdgiCXT63zs4yY75bLbLzF44YOlJ7-lJD28pZqem2CXkPSIkN1aCpcKUxjJy5PYHpW_pK7SZbdMk3kYf0cMtI_UZB6XpHlSxR0aeNtV5GxFiwakWGDmCRSV42UvLfJpUrip5w8iuBJ_DS1VoqhAOjhEBGsF2vwteDft4TOPupUukJ9u9d99ejce_qvGrdqh2DRVGI2hkxFVcAM0BTwDa1zIa6ZVTv5c8_vN-uqqwSZLu36SGAwl_TvfpqUllQaAymnVQ9UHPyTHAfFoBM2s9rYpLqXQxWfHmFUbS6kycKZUchEEQRS2XqnYjpM0lhovfhTW1lsOA5z63zb96xxMLAJ_XE-vAbaE0RTMOaPpFX9Zm8bo6ixJ2OBpWFppiOiR0xjUiW7BHwIF7DtOHJ4uXl5D3oXIPNWPerIkXm1406ld3QNTWdf1XRt3Gd-eWBBuAlmtXcQsa770jOdubI9jLCZ9dppd-krjc73DgvEazsPhj7a6u-U_apcm3xPN-CoSQa61PgAnzdh433X9v7rqu57uFnxtmO7MIXw1qlTvTO3YewCG3uI0-PgF4vo7EkggAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Fsettings_2Fbacklog_stages_2FStageForm_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA8VUTXOjMAy98yt8XA7uAN2mrfNjOsZWiHeJzdgiCXT63zs4yY75bLbLzF44YOlJ7_lJD28pZqem2CXkPSIkN1aCpcKUxjJy5PYHpW_pK7SZbdMk3kYf0cMtI_UZB6XpHlSxR0aeNtV5GxFiwakWGDmCRSV42UvLfJpUrip5w8iuBJ_DS1VoqhAOjhEBGsF2vwteDft4TOPupUukJ9u9d99ejce_qvGrdqh2DRVGI2hkxFVcAM0BTwDa1zIa6ZVTv5c8_vN_uqqwSZLu36SGAwl_TvfpqUllQaAymnVQ9UHPyTHAfFoBM2s9rYpLqXQxWfHmFUbS6kycKZUchEEQRS2XqnYjpM0lhovfhTW1lsOA5z63zb96xxMLAJ_XE_vAbaE0RTMOaPpFX9Zm8bo6ixJ2OBpWFppiOiR0xjUiW7BHwIF7DtOHJ4uXl5D3oXIPNWPerIkXm1406ld3QNTWdf1XRt3Gd_eWBBuAlmtXcQsa770jOdubI9jLCZ9dppd_krjc73DgvEazsPhj7a6u_U_apcm3xPN_CoSQa61PgAnzdh433X9v7rqu57uFnxtmO7MIXw1qlTvTO3YewCG3uI0_PgF4vo7EkggAAA_22_7D = __webpack_require__(2767);
;// ./src/app/settings/backlog-stages/StageForm.css.ts



var actionRow = '_1q0mfza7';
var addBtn = '_1t2wygfb';
var cancelBtn = '_1q0mfza9';
var checkboxRow = '_1q0mfza6';
var confirmDeleteBtn = '_1q0mfzab';
var deleteBtn = '_1q0mfzaa';
var errorMessage = '_1q0mfzac';
var fieldError = '_1t2wygfa';
var fieldGroup = '_1q0mfza1';
var flagsRow = '_1t2wygf2';
var StageForm_css_form = '_1q0mfza0';
var gateCheckboxRow = '_1t2wygf8';
var gateList = '_1t2wygf7';
var gateSubFields = '_1t2wygf9';
var graphSection = '_1t2wygfd';
var hint = '_1q0mfza3';
var input = '_1q0mfza4';
var inputDisabled = '_1q0mfza5';
var inputInvalid = '_1t2wygf0';
var StageForm_css_label = '_1q0mfza2';
var removeBtn = '_1t2wygfc';
var sectionHeading = '_1t2wygf3';
var submitBtn = '_1q0mfza8';
var StageForm_css_textarea = '_1t2wygf1 _1q0mfza4';
var transitionCard = '_1t2wygf5';
var transitionHeaderRow = '_1t2wygf6';
var transitionList = '_1t2wygf4';
var warningBanner = '_1t2wygfe';
;// ./src/app/settings/backlog-stages/TransitionRow.tsx
/* __next_internal_client_entry_do_not_use__ GATE_KINDS,emptyGateFormState,emptyTransitionFormState,gateConfigFor,validateTransition,GateCheckboxRow,TransitionRow auto */ 


// The 4 gate kinds, in display order, mirroring session.GateKind's sealed set
// (session/gate_status.go). Every transition row shows exactly these 4
// checkboxes — Task 2.8.2c1.
const GATE_KINDS = [
    {
        kind: "human_approval",
        label: "Human approval"
    },
    {
        kind: "automated_review",
        label: "Automated review"
    },
    {
        kind: "structural",
        label: "Structural check"
    },
    {
        kind: "custom",
        label: "Custom check"
    }
];
// Structural check's closed set of check IDs — mirrors
// session/gate_structural.go's structuralCheckEvaluators map. Kept in sync by
// hand (no RPC exposes this set); the server validates independently and
// fails closed on any ID outside it, so this list is a UI convenience, not
// the source of truth.
const STRUCTURAL_CHECK_OPTIONS = [
    {
        id: "ac_complete",
        label: "All acceptance criteria complete"
    },
    {
        id: "pr_green",
        label: "PR has passing CI and is mergeable"
    },
    {
        id: "no_open_blockers",
        label: "No open blocking dependencies"
    }
];
// Custom check's pre-registered skill allowlist — mirrors
// session/gate_config.go's registeredCustomCheckSkills (Story 2.4.4, ADR-003).
// Kept in sync by hand for the same reason as STRUCTURAL_CHECK_OPTIONS above.
const CUSTOM_CHECK_SKILL_OPTIONS = [
    {
        id: "review-feasibility",
        label: "review-feasibility"
    }
];
function emptyGateFormState() {
    return {
        checked: false,
        pipelineMode: "",
        requiresDiff: false,
        checkId: "",
        skill: ""
    };
}
function emptyTransitionFormState(key) {
    return {
        key,
        toStageSlug: "",
        gates: {
            human_approval: emptyGateFormState(),
            automated_review: emptyGateFormState(),
            structural: emptyGateFormState(),
            custom: emptyGateFormState()
        }
    };
}
function gateConfigFor(kind, state) {
    switch(kind){
        case "human_approval":
            return {};
        case "automated_review":
            return {
                pipeline_mode: state.pipelineMode,
                requires_diff: state.requiresDiff ? "true" : "false"
            };
        case "structural":
            return {
                check_id: state.checkId
            };
        case "custom":
            return {
                skill: state.skill
            };
        default:
            return {};
    }
}
/** Validates one transition row's checked gates; returns the row with any required-field errors filled in, and whether it's valid. */ function validateTransition(t) {
    let valid = true;
    const gates = {
        ...t.gates
    };
    const check = (kind, missing, message)=>{
        if (gates[kind].checked && missing) {
            gates[kind] = {
                ...gates[kind],
                fieldError: message
            };
            valid = false;
            return;
        }
        if (gates[kind].fieldError) gates[kind] = {
            ...gates[kind],
            fieldError: undefined
        };
    };
    check("automated_review", !gates.automated_review.pipelineMode, "Select a review prompt for this gate.");
    check("structural", !gates.structural.checkId, "Select a check for this gate.");
    check("custom", !gates.custom.skill, "Select a skill for this gate.");
    return {
        transition: {
            ...t,
            gates
        },
        valid
    };
}
/** Inline `role="alert"` message wired via `aria-describedby` to its field — shared by every gate-kind's sub-field (Task 2.8.2c4). */ function FieldError(param) {
    let { id, message } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
        id: id,
        className: fieldError,
        role: "alert",
        children: message
    });
}
function onGateChecked(state, onChange) {
    onChange({
        ...state,
        checked: true
    });
}
function onGateUnchecked(onChange) {
    // Unchecking hides AND clears the field — no orphaned hidden state that
    // resurfaces stale on next check (ux.md Surface 2 interaction flow #2).
    onChange(emptyGateFormState());
}
/**
 * One gate kind's checkbox plus (Tasks 2.8.2c2/c3) its progressive-disclosure
 * config field(s), shown only while checked.
 *
 * Task 2.8.2c4 (keyboard/label association): every control has a real
 * `<label htmlFor>`, sits in natural tab order (no manufactured tabIndex),
 * and each sub-field's error message is wired via `aria-describedby` so
 * screen readers announce it alongside the field, not just visually beside it.
 */ function GateCheckboxRow(param) {
    let { kind, label, state, onChange, pipelineModeOptions, idPrefix } = param;
    const checkboxId = "".concat(idPrefix, "-checkbox");
    const errorId = "".concat(idPrefix, "-error");
    const handleCheckedChange = (0,react.useCallback)((checked)=>checked ? onGateChecked(state, onChange) : onGateUnchecked(onChange), [
        onChange,
        state
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: gateCheckboxRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        id: checkboxId,
                        type: "checkbox",
                        checked: state.checked,
                        onChange: (e)=>handleCheckedChange(e.target.checked),
                        "data-testid": checkboxId
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                        className: StageForm_css_label,
                        htmlFor: checkboxId,
                        children: label
                    })
                ]
            }),
            state.checked && kind === "automated_review" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: gateSubFields,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: fieldGroup,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: StageForm_css_label,
                                htmlFor: "".concat(idPrefix, "-pipeline-mode"),
                                children: "Review prompt / mode"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                                id: "".concat(idPrefix, "-pipeline-mode"),
                                className: state.fieldError ? [
                                    input,
                                    inputInvalid
                                ].join(" ") : input,
                                value: state.pipelineMode,
                                onChange: (e)=>onChange({
                                        ...state,
                                        pipelineMode: e.target.value,
                                        fieldError: undefined
                                    }),
                                "aria-invalid": Boolean(state.fieldError),
                                "aria-describedby": state.fieldError ? errorId : undefined,
                                "data-testid": "".concat(idPrefix, "-pipeline-mode"),
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                        value: "",
                                        children: "Select a pipeline mode…"
                                    }),
                                    pipelineModeOptions.map((m)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                            value: m.slug,
                                            children: m.name
                                        }, m.slug))
                                ]
                            }),
                            state.fieldError && /*#__PURE__*/ (0,jsx_runtime.jsx)(FieldError, {
                                id: errorId,
                                message: state.fieldError
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: checkboxRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "".concat(idPrefix, "-requires-diff"),
                                type: "checkbox",
                                checked: state.requiresDiff,
                                onChange: (e)=>onChange({
                                        ...state,
                                        requiresDiff: e.target.checked
                                    }),
                                "data-testid": "".concat(idPrefix, "-requires-diff")
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: StageForm_css_label,
                                htmlFor: "".concat(idPrefix, "-requires-diff"),
                                children: "Requires diff"
                            })
                        ]
                    })
                ]
            }),
            state.checked && kind === "structural" && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: gateSubFields,
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: fieldGroup,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                            className: StageForm_css_label,
                            htmlFor: "".concat(idPrefix, "-check-id"),
                            children: "Check"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                            id: "".concat(idPrefix, "-check-id"),
                            className: state.fieldError ? [
                                input,
                                inputInvalid
                            ].join(" ") : input,
                            value: state.checkId,
                            onChange: (e)=>onChange({
                                    ...state,
                                    checkId: e.target.value,
                                    fieldError: undefined
                                }),
                            "aria-invalid": Boolean(state.fieldError),
                            "aria-describedby": state.fieldError ? errorId : undefined,
                            "data-testid": "".concat(idPrefix, "-check-id"),
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                    value: "",
                                    children: "Select a check…"
                                }),
                                STRUCTURAL_CHECK_OPTIONS.map((c)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                        value: c.id,
                                        children: c.label
                                    }, c.id))
                            ]
                        }),
                        state.fieldError && /*#__PURE__*/ (0,jsx_runtime.jsx)(FieldError, {
                            id: errorId,
                            message: state.fieldError
                        })
                    ]
                })
            }),
            state.checked && kind === "custom" && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: gateSubFields,
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: fieldGroup,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                            className: StageForm_css_label,
                            htmlFor: "".concat(idPrefix, "-skill"),
                            children: "Skill/command"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                            id: "".concat(idPrefix, "-skill"),
                            className: state.fieldError ? [
                                input,
                                inputInvalid
                            ].join(" ") : input,
                            value: state.skill,
                            onChange: (e)=>onChange({
                                    ...state,
                                    skill: e.target.value,
                                    fieldError: undefined
                                }),
                            "aria-invalid": Boolean(state.fieldError),
                            "aria-describedby": state.fieldError ? errorId : undefined,
                            "data-testid": "".concat(idPrefix, "-skill"),
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                    value: "",
                                    children: "Select a skill…"
                                }),
                                CUSTOM_CHECK_SKILL_OPTIONS.map((s)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                        value: s.id,
                                        children: s.label
                                    }, s.id))
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: hint,
                            children: "Pre-registered allowlist only — see ADR-003."
                        }),
                        state.fieldError && /*#__PURE__*/ (0,jsx_runtime.jsx)(FieldError, {
                            id: errorId,
                            message: state.fieldError
                        })
                    ]
                })
            })
        ]
    });
}
/** One "Outgoing transitions" sub-list row: target-stage select + its 4-gate checkbox group (Task 2.8.2b). */ function TransitionRow(param) {
    let { transition, allStages, pipelineModeOptions, onChange, onRemove, rowIndex } = param;
    const selectId = "stage-transition-".concat(rowIndex, "-to");
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: transitionCard,
        "data-testid": "stage-transition-row-".concat(rowIndex),
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: transitionHeaderRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: fieldGroup,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: StageForm_css_label,
                                htmlFor: selectId,
                                children: "To"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                                id: selectId,
                                className: input,
                                value: transition.toStageSlug,
                                onChange: (e)=>onChange({
                                        ...transition,
                                        toStageSlug: e.target.value
                                    }),
                                "data-testid": selectId,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                        value: "",
                                        children: "Select a stage…"
                                    }),
                                    allStages.map((s)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("option", {
                                            value: s.slug,
                                            children: [
                                                s.name,
                                                " (",
                                                s.slug,
                                                ")"
                                            ]
                                        }, s.slug))
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: removeBtn,
                        onClick: onRemove,
                        "data-testid": "stage-transition-remove-".concat(rowIndex),
                        children: "Remove"
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: gateList,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: StageForm_css_label,
                        children: "Gates:"
                    }),
                    GATE_KINDS.map((param)=>{
                        let { kind, label } = param;
                        return /*#__PURE__*/ (0,jsx_runtime.jsx)(GateCheckboxRow, {
                            kind: kind,
                            label: label,
                            state: transition.gates[kind],
                            onChange: (next)=>onChange({
                                    ...transition,
                                    gates: {
                                        ...transition.gates,
                                        [kind]: next
                                    }
                                }),
                            pipelineModeOptions: pipelineModeOptions,
                            idPrefix: "stage-transition-".concat(rowIndex, "-gate-").concat(kind)
                        }, kind);
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ./src/lib/api/transport.ts
var transport = __webpack_require__(21694);
// EXTERNAL MODULE: ./src/gen/session/v1/backlog_pb.ts
var backlog_pb = __webpack_require__(79870);
;// ./src/lib/hooks/useLivenessDefinitions.ts
/* __next_internal_client_entry_do_not_use__ useLivenessDefinitions auto */ 



function mapLiveness(p) {
    return {
        id: p.id,
        stageSlug: p.stageSlug,
        pipelineMode: p.pipelineMode,
        kind: p.kind,
        expectedDurationMs: Number(p.expectedDurationMs),
        stalenessMarginMs: Number(p.stalenessMarginMs),
        maxNoProgressDurationMs: Number(p.maxNoProgressDurationMs),
        cycleThreshold: p.cycleThreshold,
        cycleLookbackMs: Number(p.cycleLookbackMs),
        enabled: p.enabled
    };
}
/**
 * RPC-backed CRUD for the 5 LivenessDefinition RPCs, feeding the "Liveness
 * overrides" settings section. Self-contained client, mirroring
 * useBacklogService.ts's own PipelineMode CRUD split.
 */ function useLivenessDefinitions() {
    const clientRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        clientRef.current = (0,promise_client/* createClient */.UU)(backlog_pb/* BacklogService */.jSM, (0,transport/* getConnectTransport */.qW)());
    }, []);
    const listLivenessDefinitions = (0,react.useCallback)(async ()=>{
        if (!clientRef.current) return [];
        try {
            const resp = await clientRef.current.listLivenessDefinitions({});
            var _resp_items;
            return ((_resp_items = resp.items) !== null && _resp_items !== void 0 ? _resp_items : []).map(mapLiveness);
        } catch (err) {
            console.error("[useLivenessDefinitions] listLivenessDefinitions:", err);
            throw err;
        }
    }, []);
    const createLivenessDefinition = (0,react.useCallback)(async (data)=>{
        if (!clientRef.current) throw new Error("Backlog service not connected");
        try {
            var _data_expectedDurationMs, _data_stalenessMarginMs, _data_maxNoProgressDurationMs, _data_cycleThreshold, _data_cycleLookbackMs, _data_enabled;
            const resp = await clientRef.current.createLivenessDefinition({
                stageSlug: data.stageSlug,
                pipelineMode: data.pipelineMode || undefined,
                kind: data.kind,
                expectedDurationMs: BigInt(Math.trunc((_data_expectedDurationMs = data.expectedDurationMs) !== null && _data_expectedDurationMs !== void 0 ? _data_expectedDurationMs : 0)),
                stalenessMarginMs: BigInt(Math.trunc((_data_stalenessMarginMs = data.stalenessMarginMs) !== null && _data_stalenessMarginMs !== void 0 ? _data_stalenessMarginMs : 0)),
                maxNoProgressDurationMs: BigInt(Math.trunc((_data_maxNoProgressDurationMs = data.maxNoProgressDurationMs) !== null && _data_maxNoProgressDurationMs !== void 0 ? _data_maxNoProgressDurationMs : 0)),
                cycleThreshold: Math.trunc((_data_cycleThreshold = data.cycleThreshold) !== null && _data_cycleThreshold !== void 0 ? _data_cycleThreshold : 0),
                cycleLookbackMs: BigInt(Math.trunc((_data_cycleLookbackMs = data.cycleLookbackMs) !== null && _data_cycleLookbackMs !== void 0 ? _data_cycleLookbackMs : 0)),
                enabled: (_data_enabled = data.enabled) !== null && _data_enabled !== void 0 ? _data_enabled : true
            });
            if (!resp.item) throw new Error("createLivenessDefinition: server returned no item");
            return mapLiveness(resp.item);
        } catch (err) {
            console.error("[useLivenessDefinitions] createLivenessDefinition:", err);
            throw err;
        }
    }, []);
    const updateLivenessDefinition = (0,react.useCallback)(async (id, data)=>{
        if (!clientRef.current) throw new Error("Backlog service not connected");
        try {
            const resp = await clientRef.current.updateLivenessDefinition({
                id,
                expectedDurationMs: data.expectedDurationMs !== undefined ? BigInt(Math.trunc(data.expectedDurationMs)) : undefined,
                stalenessMarginMs: data.stalenessMarginMs !== undefined ? BigInt(Math.trunc(data.stalenessMarginMs)) : undefined,
                maxNoProgressDurationMs: data.maxNoProgressDurationMs !== undefined ? BigInt(Math.trunc(data.maxNoProgressDurationMs)) : undefined,
                cycleThreshold: data.cycleThreshold !== undefined ? Math.trunc(data.cycleThreshold) : undefined,
                cycleLookbackMs: data.cycleLookbackMs !== undefined ? BigInt(Math.trunc(data.cycleLookbackMs)) : undefined,
                enabled: data.enabled
            });
            if (!resp.item) throw new Error("updateLivenessDefinition: server returned no item");
            return mapLiveness(resp.item);
        } catch (err) {
            console.error("[useLivenessDefinitions] updateLivenessDefinition:", err);
            throw err;
        }
    }, []);
    const deleteLivenessDefinition = (0,react.useCallback)(async (id)=>{
        if (!clientRef.current) return false;
        try {
            await clientRef.current.deleteLivenessDefinition({
                id
            });
            return true;
        } catch (err) {
            console.error("[useLivenessDefinitions] deleteLivenessDefinition:", err);
            throw err;
        }
    }, []);
    return {
        listLivenessDefinitions,
        createLivenessDefinition,
        updateLivenessDefinition,
        deleteLivenessDefinition
    };
}

;// ./src/app/settings/backlog-stages/LivenessRowEditor.tsx
/* __next_internal_client_entry_do_not_use__ KIND_OPTIONS,emptyRowForm,rowFormFromDefinition,validateRowForm,toMs,RowEditor auto */ 

const KIND_OPTIONS = [
    {
        value: "duration_budget",
        label: "Duration budget"
    },
    {
        value: "heartbeat",
        label: "Heartbeat"
    },
    {
        value: "cycle_frequency",
        label: "Cycle frequency"
    }
];
function emptyRowForm() {
    return {
        pipelineMode: "",
        kind: "duration_budget",
        expectedDurationMinutes: "",
        stalenessMarginMinutes: "",
        maxNoProgressMinutes: "",
        cycleThreshold: "",
        cycleLookbackMinutes: "",
        enabled: true
    };
}
function rowFormFromDefinition(def) {
    var _def_pipelineMode;
    return {
        id: def.id,
        pipelineMode: (_def_pipelineMode = def.pipelineMode) !== null && _def_pipelineMode !== void 0 ? _def_pipelineMode : "",
        kind: def.kind,
        expectedDurationMinutes: def.expectedDurationMs ? String(def.expectedDurationMs / 60000) : "",
        stalenessMarginMinutes: def.stalenessMarginMs ? String(def.stalenessMarginMs / 60000) : "",
        maxNoProgressMinutes: def.maxNoProgressDurationMs ? String(def.maxNoProgressDurationMs / 60000) : "",
        cycleThreshold: def.cycleThreshold ? String(def.cycleThreshold) : "",
        cycleLookbackMinutes: def.cycleLookbackMs ? String(def.cycleLookbackMs / 60000) : "",
        enabled: def.enabled
    };
}
function assertNever(x) {
    throw new Error("Unhandled LivenessKind: ".concat(JSON.stringify(x)));
}
/** Replicates LivenessDefinition.validate()'s shape rule plus positivity checks the server doesn't enforce (pitfalls.md §4). */ function validateRowForm(form) {
    const positiveInt = (raw)=>Number.isInteger(Number(raw)) && Number(raw) > 0;
    switch(form.kind){
        case "duration_budget":
            if (!positiveInt(form.expectedDurationMinutes)) return "Expected duration must be a positive number of minutes.";
            if (form.stalenessMarginMinutes && Number(form.stalenessMarginMinutes) < 0) return "Staleness margin cannot be negative.";
            return null;
        case "heartbeat":
            if (!positiveInt(form.maxNoProgressMinutes)) return "Max no-progress duration must be a positive number of minutes.";
            return null;
        case "cycle_frequency":
            if (!positiveInt(form.cycleThreshold)) return "Cycle threshold must be a positive whole number.";
            if (!positiveInt(form.cycleLookbackMinutes)) return "Cycle lookback must be a positive number of minutes.";
            return null;
        default:
            return assertNever(form.kind);
    }
}
function toMs(minutes) {
    return minutes ? Number(minutes) * 60000 : 0;
}
/** One labeled numeric input, shared by every kind-conditional duration/count field below. */ function NumberField(param) {
    let { id, label, min, value, onChange } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: fieldGroup,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                className: StageForm_css_label,
                htmlFor: id,
                children: label
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                id: id,
                type: "number",
                min: min,
                className: input,
                value: value,
                onChange: (e)=>onChange(e.target.value),
                "data-testid": id
            })
        ]
    });
}
function DurationBudgetFields(param) {
    let { form, onChange } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: gateSubFields,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)(NumberField, {
                id: "liveness-editor-expected",
                label: "Expected duration (minutes)",
                min: 1,
                value: form.expectedDurationMinutes,
                onChange: (v)=>onChange({
                        ...form,
                        expectedDurationMinutes: v
                    })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(NumberField, {
                id: "liveness-editor-margin",
                label: "Staleness margin (minutes)",
                min: 0,
                value: form.stalenessMarginMinutes,
                onChange: (v)=>onChange({
                        ...form,
                        stalenessMarginMinutes: v
                    })
            })
        ]
    });
}
function HeartbeatFields(param) {
    let { form, onChange } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: gateSubFields,
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(NumberField, {
            id: "liveness-editor-no-progress",
            label: "Max no-progress duration (minutes)",
            min: 1,
            value: form.maxNoProgressMinutes,
            onChange: (v)=>onChange({
                    ...form,
                    maxNoProgressMinutes: v
                })
        })
    });
}
function CycleFrequencyFields(param) {
    let { form, onChange } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: gateSubFields,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)(NumberField, {
                id: "liveness-editor-threshold",
                label: "Cycle threshold",
                min: 1,
                value: form.cycleThreshold,
                onChange: (v)=>onChange({
                        ...form,
                        cycleThreshold: v
                    })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(NumberField, {
                id: "liveness-editor-lookback",
                label: "Cycle lookback (minutes)",
                min: 1,
                value: form.cycleLookbackMinutes,
                onChange: (v)=>onChange({
                        ...form,
                        cycleLookbackMinutes: v
                    })
            })
        ]
    });
}
function DurationFields(props) {
    switch(props.form.kind){
        case "duration_budget":
            return /*#__PURE__*/ (0,jsx_runtime.jsx)(DurationBudgetFields, {
                ...props
            });
        case "heartbeat":
            return /*#__PURE__*/ (0,jsx_runtime.jsx)(HeartbeatFields, {
                ...props
            });
        case "cycle_frequency":
            return /*#__PURE__*/ (0,jsx_runtime.jsx)(CycleFrequencyFields, {
                ...props
            });
        default:
            return assertNever(props.form.kind);
    }
}
function ModeField(param) {
    let { form, isNew, pipelineModeOptions, onChange } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: fieldGroup,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                className: StageForm_css_label,
                htmlFor: "liveness-editor-mode",
                children: "Pipeline mode"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                id: "liveness-editor-mode",
                className: input,
                value: form.pipelineMode,
                disabled: !isNew,
                onChange: (e)=>onChange({
                        ...form,
                        pipelineMode: e.target.value
                    }),
                "data-testid": "liveness-editor-mode",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                        value: "",
                        children: "Default (all modes)"
                    }),
                    pipelineModeOptions.map((m)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                            value: m.slug,
                            children: m.name
                        }, m.slug))
                ]
            }),
            !isNew && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: hint,
                children: "Pipeline mode is immutable after creation."
            })
        ]
    });
}
function KindField(param) {
    let { form, isNew, onChange } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: fieldGroup,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                className: StageForm_css_label,
                htmlFor: "liveness-editor-kind",
                children: "Kind"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("select", {
                id: "liveness-editor-kind",
                className: input,
                value: form.kind,
                disabled: !isNew,
                onChange: (e)=>onChange({
                        ...form,
                        kind: e.target.value
                    }),
                "data-testid": "liveness-editor-kind",
                children: KIND_OPTIONS.map((k)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                        value: k.value,
                        children: k.label
                    }, k.value))
            }),
            !isNew && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: hint,
                children: "Kind is immutable after creation — delete and recreate to change shape."
            })
        ]
    });
}
/** Pipeline-mode + kind selects — both lock once a row is persisted (server-side immutable). */ function RowEditorHeader(props) {
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: transitionHeaderRow,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)(ModeField, {
                ...props
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(KindField, {
                ...props
            })
        ]
    });
}
/** Create/edit editor for one LivenessDefinition row. */ function RowEditor(param) {
    let { form, isNew, pipelineModeOptions, onChange, onSave, onCancel, saving } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: transitionCard,
        "data-testid": "liveness-row-editor",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)(RowEditorHeader, {
                form: form,
                isNew: isNew,
                pipelineModeOptions: pipelineModeOptions,
                onChange: onChange
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(DurationFields, {
                form: form,
                onChange: onChange
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: checkboxRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        id: "liveness-editor-enabled",
                        type: "checkbox",
                        checked: form.enabled,
                        onChange: (e)=>onChange({
                                ...form,
                                enabled: e.target.checked
                            }),
                        "data-testid": "liveness-editor-enabled"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                        className: StageForm_css_label,
                        htmlFor: "liveness-editor-enabled",
                        children: "Enabled"
                    })
                ]
            }),
            form.fieldError && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: fieldError,
                role: "alert",
                "data-testid": "liveness-editor-error",
                children: form.fieldError
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: transitionHeaderRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: addBtn,
                        onClick: onSave,
                        disabled: saving,
                        "data-testid": "liveness-editor-save",
                        children: saving ? "Saving…" : "Save"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: cancelBtn,
                        onClick: onCancel,
                        disabled: saving,
                        "data-testid": "liveness-editor-cancel",
                        children: "Cancel"
                    })
                ]
            })
        ]
    });
}

;// ./src/app/settings/backlog-stages/LivenessSection.tsx
/* __next_internal_client_entry_do_not_use__ LivenessSection auto */ 
// +feature: settings-stage-liveness





const KIND_LABELS = Object.fromEntries(KIND_OPTIONS.map((o)=>[
        o.value,
        o.label
    ]));
/**
 * DefaultLivenessEngine's hardcoded built-in table (session/liveness_engine.go)
 * mirrored here for display only — no RPC surfaces the resolved-effective
 * value, and this 3-entry table is effectively static.
 */ const BUILTIN_DEFAULTS = {
    idea: "duration budget: 3h expected + 15m margin",
    in_progress: "heartbeat: 2h max no-progress",
    review: "cycle frequency: 3 cycles / 24h lookback"
};
function msToLabel(ms) {
    if (!ms) return "0m";
    const minutes = ms / 60000;
    if (minutes < 60) return "".concat(minutes, "m");
    const hours = minutes / 60;
    return "".concat(Number.isInteger(hours) ? hours : hours.toFixed(1), "h");
}
function LivenessSection_assertNever(x) {
    throw new Error("Unhandled LivenessKind: ".concat(JSON.stringify(x)));
}
function summaryFor(def) {
    switch(def.kind){
        case "duration_budget":
            return "expected ".concat(msToLabel(def.expectedDurationMs), " + margin ").concat(msToLabel(def.stalenessMarginMs));
        case "heartbeat":
            return "max no-progress ".concat(msToLabel(def.maxNoProgressDurationMs));
        case "cycle_frequency":
            return "".concat(def.cycleThreshold, " cycles / ").concat(msToLabel(def.cycleLookbackMs), " lookback");
        default:
            return LivenessSection_assertNever(def.kind);
    }
}
function toUpdatePayload(form) {
    return {
        expectedDurationMs: form.kind === "duration_budget" ? toMs(form.expectedDurationMinutes) : 0,
        stalenessMarginMs: form.kind === "duration_budget" ? toMs(form.stalenessMarginMinutes) : 0,
        maxNoProgressDurationMs: form.kind === "heartbeat" ? toMs(form.maxNoProgressMinutes) : 0,
        cycleThreshold: form.kind === "cycle_frequency" ? Number(form.cycleThreshold) : 0,
        cycleLookbackMs: form.kind === "cycle_frequency" ? toMs(form.cycleLookbackMinutes) : 0,
        enabled: form.enabled
    };
}
function RowActions(param) {
    let { confirmingDelete, deleting, rowKey, onEdit, onDeleteClick, onDeleteConfirm, onDeleteCancel } = param;
    if (confirmingDelete) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    type: "button",
                    className: confirmDeleteBtn,
                    onClick: onDeleteConfirm,
                    disabled: deleting,
                    "data-testid": "liveness-row-".concat(rowKey, "-confirm-delete"),
                    children: deleting ? "Deleting…" : "Confirm delete?"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    type: "button",
                    className: cancelBtn,
                    onClick: onDeleteCancel,
                    disabled: deleting,
                    "data-testid": "liveness-row-".concat(rowKey, "-cancel-delete"),
                    children: "Never mind"
                })
            ]
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                className: addBtn,
                onClick: onEdit,
                "data-testid": "liveness-row-".concat(rowKey, "-edit"),
                children: "Edit"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                className: removeBtn,
                onClick: onDeleteClick,
                "data-testid": "liveness-row-".concat(rowKey, "-delete"),
                children: "Delete"
            })
        ]
    });
}
function LivenessRow(param) {
    let { def, pipelineModeOptions, hasBaseRow, isEditing, editForm, confirmingDelete, deleting, onEdit, onEditChange, onEditSave, onEditCancel, saving, onDeleteClick, onDeleteConfirm, onDeleteCancel } = param;
    var _def_pipelineMode;
    const rowKey = (_def_pipelineMode = def.pipelineMode) !== null && _def_pipelineMode !== void 0 ? _def_pipelineMode : "default";
    if (isEditing && editForm) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)(RowEditor, {
            form: editForm,
            isNew: false,
            pipelineModeOptions: pipelineModeOptions,
            onChange: onEditChange,
            onSave: onEditSave,
            onCancel: onEditCancel,
            saving: saving
        });
    }
    const scopeLabel = def.pipelineMode ? "Mode: ".concat(def.pipelineMode) : "All modes (default)";
    const resolutionHint = def.pipelineMode ? 'Overrides mode "'.concat(def.pipelineMode, '" only — other modes without their own override ').concat(hasBaseRow ? "fall through to the stage default above" : "fall back to the built-in default", ".") : "Applies to any mode without its own override.";
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: transitionCard,
        "data-testid": "liveness-row-".concat(rowKey),
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: transitionHeaderRow,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: fieldGroup,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: StageForm_css_label,
                            children: scopeLabel
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                            className: hint,
                            children: [
                                KIND_LABELS[def.kind],
                                " — ",
                                summaryFor(def),
                                " — ",
                                def.enabled ? "enabled" : "disabled"
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: hint,
                            children: resolutionHint
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)(RowActions, {
                    confirmingDelete: confirmingDelete,
                    deleting: deleting,
                    rowKey: rowKey,
                    onEdit: onEdit,
                    onDeleteClick: onDeleteClick,
                    onDeleteConfirm: onDeleteConfirm,
                    onDeleteCancel: onDeleteCancel
                })
            ]
        })
    });
}
function fallbackNoteFor(stageSlug, hasBaseRow) {
    if (hasBaseRow) return "Modes without their own override use the stage-wide default row above.";
    const builtinDefault = BUILTIN_DEFAULTS[stageSlug];
    if (builtinDefault) {
        return "No stage-wide default configured — modes without an override fall back to the built-in default (".concat(builtinDefault, ").");
    }
    return "No stage-wide default configured — modes without an override have no timeout (no liveness check runs).";
}
/**
 * "Liveness overrides" sub-section nested in StageForm. Fetches the full
 * (unfiltered — no server-side filter RPC exists) ListLivenessDefinitions
 * response and filters to this stage client-side; acceptable at the
 * expected "dozens of rows" table size.
 */ function LivenessSection(param) {
    let { stageSlug, pipelineModeOptions } = param;
    const { listLivenessDefinitions, createLivenessDefinition, updateLivenessDefinition, deleteLivenessDefinition } = useLivenessDefinitions();
    const [definitions, setDefinitions] = (0,react.useState)([]);
    const [loading, setLoading] = (0,react.useState)(true);
    const [loadError, setLoadError] = (0,react.useState)(null);
    const [editingKey, setEditingKey] = (0,react.useState)(null); // "new" or a definition id
    const [editForm, setEditForm] = (0,react.useState)(null);
    const [saving, setSaving] = (0,react.useState)(false);
    const [confirmingDeleteId, setConfirmingDeleteId] = (0,react.useState)(null);
    const [deleting, setDeleting] = (0,react.useState)(false);
    (0,react.useEffect)(()=>{
        let cancelled = false;
        (async ()=>{
            try {
                const all = await listLivenessDefinitions();
                if (cancelled) return;
                setDefinitions(all.filter((d)=>d.stageSlug === stageSlug));
            } catch (err) {
                if (!cancelled) setLoadError(StageForm_errorMessage(err));
            } finally{
                if (!cancelled) setLoading(false);
            }
        })();
        return ()=>{
            cancelled = true;
        };
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        stageSlug
    ]);
    const handleAdd = (0,react.useCallback)(()=>{
        setEditingKey("new");
        setEditForm(emptyRowForm());
    }, []);
    const handleEdit = (0,react.useCallback)((def)=>{
        setEditingKey(def.id);
        setEditForm(rowFormFromDefinition(def));
    }, []);
    const handleEditCancel = (0,react.useCallback)(()=>{
        setEditingKey(null);
        setEditForm(null);
    }, []);
    const saveNew = (0,react.useCallback)(async (form)=>{
        const created = await createLivenessDefinition({
            stageSlug,
            pipelineMode: form.pipelineMode || undefined,
            kind: form.kind,
            ...toUpdatePayload(form)
        });
        setDefinitions((prev)=>[
                ...prev,
                created
            ]);
    }, [
        createLivenessDefinition,
        stageSlug
    ]);
    const saveExisting = (0,react.useCallback)(async (form)=>{
        const updated = await updateLivenessDefinition(form.id, toUpdatePayload(form));
        setDefinitions((prev)=>prev.map((d)=>d.id === updated.id ? updated : d));
    }, [
        updateLivenessDefinition
    ]);
    const handleEditSave = (0,react.useCallback)(async ()=>{
        if (!editForm) return;
        const validationError = validateRowForm(editForm);
        if (validationError) {
            setEditForm({
                ...editForm,
                fieldError: validationError
            });
            return;
        }
        setSaving(true);
        try {
            if (editForm.id) await saveExisting(editForm);
            else await saveNew(editForm);
            setEditingKey(null);
            setEditForm(null);
        } catch (err) {
            setEditForm({
                ...editForm,
                fieldError: StageForm_errorMessage(err)
            });
        } finally{
            setSaving(false);
        }
    }, [
        editForm,
        saveExisting,
        saveNew
    ]);
    const handleDeleteConfirm = (0,react.useCallback)(async (id)=>{
        setDeleting(true);
        try {
            await deleteLivenessDefinition(id);
            setDefinitions((prev)=>prev.filter((d)=>d.id !== id));
            setConfirmingDeleteId(null);
        } catch (err) {
            setLoadError(StageForm_errorMessage(err));
        } finally{
            setDeleting(false);
        }
    }, [
        deleteLivenessDefinition
    ]);
    const baseRow = definitions.find((d)=>!d.pipelineMode);
    const overrideRows = definitions.filter((d)=>d.pipelineMode);
    function renderRow(def) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)(LivenessRow, {
            def: def,
            pipelineModeOptions: pipelineModeOptions,
            hasBaseRow: Boolean(baseRow),
            isEditing: editingKey === def.id,
            editForm: editingKey === def.id ? editForm : null,
            confirmingDelete: confirmingDeleteId === def.id,
            deleting: deleting,
            onEdit: ()=>handleEdit(def),
            onEditChange: setEditForm,
            onEditSave: handleEditSave,
            onEditCancel: handleEditCancel,
            saving: saving,
            onDeleteClick: ()=>setConfirmingDeleteId(def.id),
            onDeleteConfirm: ()=>handleDeleteConfirm(def.id),
            onDeleteCancel: ()=>setConfirmingDeleteId(null)
        }, def.id);
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: sectionHeading,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        children: "Liveness overrides"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: addBtn,
                        onClick: handleAdd,
                        "data-testid": "liveness-add",
                        children: "+ Add override"
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: hint,
                children: "Changes apply immediately to every item currently sitting in this stage — there is no per-item snooze yet."
            }),
            loadError && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: errorMessage,
                role: "alert",
                "data-testid": "liveness-section-error",
                children: loadError
            }),
            loading ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: hint,
                children: "Loading liveness overrides…"
            }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: transitionList,
                children: [
                    editingKey === "new" && editForm && /*#__PURE__*/ (0,jsx_runtime.jsx)(RowEditor, {
                        form: editForm,
                        isNew: true,
                        pipelineModeOptions: pipelineModeOptions,
                        onChange: setEditForm,
                        onSave: handleEditSave,
                        onCancel: handleEditCancel,
                        saving: saving
                    }),
                    definitions.length === 0 && editingKey !== "new" && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: hint,
                        children: "No liveness overrides configured for this stage."
                    }),
                    baseRow && renderRow(baseRow),
                    overrideRows.map(renderRow),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: hint,
                        "data-testid": "liveness-fallback-note",
                        children: fallbackNoteFor(stageSlug, Boolean(baseRow))
                    })
                ]
            })
        ]
    });
}

;// ./src/app/settings/backlog-stages/StageForm.tsx
/* __next_internal_client_entry_do_not_use__ errorMessage,StageForm auto */ 








function transitionToFormState(t, key) {
    const state = emptyTransitionFormState(key);
    state.id = t.id;
    state.toStageSlug = t.toStageSlug;
    for (const g of t.gates){
        if (g.kind !== "human_approval" && g.kind !== "automated_review" && g.kind !== "structural" && g.kind !== "custom") {
            continue;
        }
        var _g_config_pipeline_mode, _g_config_check_id, _g_config_skill;
        state.gates[g.kind] = {
            checked: true,
            id: g.id,
            pipelineMode: (_g_config_pipeline_mode = g.config.pipeline_mode) !== null && _g_config_pipeline_mode !== void 0 ? _g_config_pipeline_mode : "",
            requiresDiff: g.config.requires_diff === "true",
            checkId: (_g_config_check_id = g.config.check_id) !== null && _g_config_check_id !== void 0 ? _g_config_check_id : "",
            skill: (_g_config_skill = g.config.skill) !== null && _g_config_skill !== void 0 ? _g_config_skill : ""
        };
    }
    return state;
}
/** Extracts a human-readable message from a failure, preferring the ConnectError message — mirrors PipelineModeForm.tsx's errorMessage() helper. */ function StageForm_errorMessage(err) {
    if (err instanceof connect_error/* ConnectError */.T) return err.message;
    if (err instanceof Error) return err.message;
    return String(err);
}
/**
 * Create/edit form for a BacklogStage (Epic 2.8, Story 2.8.2): slug
 * (immutable on edit), name, description, entry/terminal/enabled flags, a
 * nested "Outgoing transitions" sub-list (each row: target stage + a 4-gate
 * checkbox group with progressive disclosure), a read-only graph preview, and
 * the two-step delete action. Mirrors PipelineModeForm.tsx's overlay/
 * slug-immutable/two-step-delete/inline-error shape (Task 2.8.2a).
 */ function StageForm(param) {
    let { stage, allStages, onSaved, onDeleted, onCancel } = param;
    const { createStage, updateStage, deleteStage, listStageTransitions, createStageTransition, deleteStageTransition, createTransitionGate, updateTransitionGate, deleteTransitionGate } = (0,useBacklogStages/* useBacklogStagesAdmin */.ll)();
    const { listPipelineModes } = (0,useBacklogService/* useBacklogService */.gG)();
    var _stage_slug;
    const [slug, setSlug] = (0,react.useState)((_stage_slug = stage === null || stage === void 0 ? void 0 : stage.slug) !== null && _stage_slug !== void 0 ? _stage_slug : "");
    var _stage_name;
    const [name, setName] = (0,react.useState)((_stage_name = stage === null || stage === void 0 ? void 0 : stage.name) !== null && _stage_name !== void 0 ? _stage_name : "");
    var _stage_description;
    const [description, setDescription] = (0,react.useState)((_stage_description = stage === null || stage === void 0 ? void 0 : stage.description) !== null && _stage_description !== void 0 ? _stage_description : "");
    var _stage_isEntry;
    const [isEntry, setIsEntry] = (0,react.useState)((_stage_isEntry = stage === null || stage === void 0 ? void 0 : stage.isEntry) !== null && _stage_isEntry !== void 0 ? _stage_isEntry : false);
    var _stage_isTerminal;
    const [isTerminal, setIsTerminal] = (0,react.useState)((_stage_isTerminal = stage === null || stage === void 0 ? void 0 : stage.isTerminal) !== null && _stage_isTerminal !== void 0 ? _stage_isTerminal : false);
    var _stage_enabled;
    const [enabled, setEnabled] = (0,react.useState)((_stage_enabled = stage === null || stage === void 0 ? void 0 : stage.enabled) !== null && _stage_enabled !== void 0 ? _stage_enabled : true);
    const [transitions, setTransitions] = (0,react.useState)([]);
    const [removedTransitionIds, setRemovedTransitionIds] = (0,react.useState)([]);
    const [allTransitions, setAllTransitions] = (0,react.useState)([]);
    const [pipelineModes, setPipelineModes] = (0,react.useState)([]);
    const [error, setError] = (0,react.useState)(null);
    const [warning, setWarning] = (0,react.useState)(null);
    // Holds the just-saved stage while a warning banner is up, so
    // acknowledging it reports the fresh save — not the stale `stage` prop —
    // to the caller.
    const [savedPendingAck, setSavedPendingAck] = (0,react.useState)(null);
    const [submitting, setSubmitting] = (0,react.useState)(false);
    const [confirmingDelete, setConfirmingDelete] = (0,react.useState)(false);
    const [deleting, setDeleting] = (0,react.useState)(false);
    const nextKeyRef = (0,react.useRef)(0);
    const newRowKey = (0,react.useCallback)(()=>"new-".concat(nextKeyRef.current++), []);
    // Load once on mount: the full graph (for the read-only preview) always,
    // this stage's own outgoing transitions only when editing, and pipeline
    // modes (for the automated-review gate's select). Deliberately run-once —
    // `stage`/`allStages` are fixed for this form instance, matching
    // PipelineModeForm.tsx's props-seed-initial-state-only pattern.
    (0,react.useEffect)(()=>{
        let cancelled = false;
        (async ()=>{
            try {
                const [all, modes, mine] = await Promise.all([
                    listStageTransitions(),
                    listPipelineModes(),
                    stage ? listStageTransitions(stage.slug) : Promise.resolve([])
                ]);
                if (cancelled) return;
                setAllTransitions(all);
                setPipelineModes(modes);
                if (stage) {
                    setTransitions(mine.map((t)=>transitionToFormState(t, newRowKey())));
                }
            } catch (err) {
                if (!cancelled) setError(StageForm_errorMessage(err));
            }
        })();
        return ()=>{
            cancelled = true;
        };
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
    const canSubmit = Boolean(slug.trim()) && Boolean(name.trim()) && !submitting;
    const handleAddTransition = (0,react.useCallback)(()=>{
        setTransitions((prev)=>[
                ...prev,
                emptyTransitionFormState(newRowKey())
            ]);
    }, [
        newRowKey
    ]);
    const handleRemoveTransition = (0,react.useCallback)((key)=>{
        setTransitions((prev)=>{
            const row = prev.find((t)=>t.key === key);
            if (row === null || row === void 0 ? void 0 : row.id) setRemovedTransitionIds((ids)=>[
                    ...ids,
                    row.id
                ]);
            return prev.filter((t)=>t.key !== key);
        });
    }, []);
    // One gate kind's persistence for an already-created transition: delete if
    // unchecked-but-previously-saved, else create or update depending on
    // whether it already has a persisted id.
    const persistGateForKind = (0,react.useCallback)(async (transitionId, kind, gate)=>{
        if (!gate.checked) {
            if (gate.id) await deleteTransitionGate(gate.id);
            return;
        }
        const config = gateConfigFor(kind, gate);
        if (gate.id) {
            await updateTransitionGate(gate.id, {
                kind,
                config,
                enabled: true
            });
        } else {
            await createTransitionGate({
                transitionId,
                kind,
                config,
                enabled: true
            });
        }
    }, [
        deleteTransitionGate,
        updateTransitionGate,
        createTransitionGate
    ]);
    // Persists one transition row: creates the StageTransition itself if new
    // (collecting the graph validator's warnings), then persists each of its 4
    // gate slots. Returns this row's warnings.
    const persistOneTransition = (0,react.useCallback)(async (fromSlug, t)=>{
        let transitionId = t.id;
        let warnings = [];
        if (!transitionId) {
            const result = await createStageTransition({
                fromStageSlug: fromSlug,
                toStageSlug: t.toStageSlug,
                enabled: true
            });
            transitionId = result.item.id;
            warnings = result.warnings;
        }
        for (const { kind } of GATE_KINDS){
            await persistGateForKind(transitionId, kind, t.gates[kind]);
        }
        return warnings;
    }, [
        createStageTransition,
        persistGateForKind
    ]);
    const persistTransitions = (0,react.useCallback)(async (fromSlug)=>{
        for (const id of removedTransitionIds){
            await deleteStageTransition(id);
        }
        const warnings = [];
        for (const t of transitions){
            warnings.push(...await persistOneTransition(fromSlug, t));
        }
        return warnings;
    }, [
        removedTransitionIds,
        transitions,
        deleteStageTransition,
        persistOneTransition
    ]);
    const handleSubmit = (0,react.useCallback)(async (e)=>{
        e.preventDefault();
        if (!canSubmit) return;
        setError(null);
        setWarning(null);
        // Client-side required-field check runs before any RPC call (ux.md
        // Surface 2 error table row 1): a checked gate with an empty required
        // sub-field is caught here, never submitted.
        let anyInvalid = false;
        const validated = transitions.map((t)=>{
            const { transition, valid } = validateTransition(t);
            if (!valid) anyInvalid = true;
            return transition;
        });
        if (anyInvalid) {
            setTransitions(validated);
            return;
        }
        setSubmitting(true);
        try {
            let saved;
            if (stage) {
                saved = await updateStage(stage.id, {
                    name: name.trim(),
                    description: description.trim(),
                    isEntry,
                    isTerminal,
                    enabled
                });
            } else {
                saved = await createStage({
                    slug: slug.trim(),
                    name: name.trim(),
                    description: description.trim(),
                    isEntry,
                    isTerminal,
                    enabled
                });
            }
            const warnings = await persistTransitions(saved.slug);
            setRemovedTransitionIds([]);
            if (warnings.length > 0) {
                // ux.md Surface 2: success-with-warnings stays open with an
                // acknowledged banner rather than closing immediately — silent
                // misconfiguration discovered later is worse than one extra click.
                setWarning(warnings.join(" "));
                setSavedPendingAck(saved);
                setSubmitting(false);
                return;
            }
            onSaved(saved);
        } catch (err) {
            setError(StageForm_errorMessage(err));
        } finally{
            setSubmitting(false);
        }
    }, [
        canSubmit,
        transitions,
        stage,
        updateStage,
        name,
        description,
        isEntry,
        isTerminal,
        enabled,
        createStage,
        slug,
        persistTransitions,
        onSaved
    ]);
    const handleWarningAcknowledged = (0,react.useCallback)(()=>{
        if (savedPendingAck) onSaved(savedPendingAck);
        setWarning(null);
        setSavedPendingAck(null);
    }, [
        savedPendingAck,
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
        if (!stage) return;
        setDeleting(true);
        setError(null);
        try {
            await deleteStage(stage.id);
            onDeleted(stage.id);
        } catch (err) {
            setError(StageForm_errorMessage(err));
            setConfirmingDelete(false);
        } finally{
            setDeleting(false);
        }
    }, [
        stage,
        deleteStage,
        onDeleted
    ]);
    const builtIn = stage ? (0,useBacklogStages/* isBuiltInStage */.oI)(stage.slug) : false;
    const deleteDisabled = builtIn || deleting;
    const deleteTooltip = builtIn ? "Built-in stage — disable it instead of deleting." : undefined;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("form", {
        className: StageForm_css_form,
        onSubmit: handleSubmit,
        "data-testid": "stage-form",
        children: [
            error && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: errorMessage,
                role: "alert",
                "data-testid": "stage-form-error",
                children: error
            }),
            warning && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: warningBanner,
                role: "status",
                "data-testid": "stage-form-warning",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        children: [
                            "Warning: ",
                            warning
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: cancelBtn,
                        onClick: handleWarningAcknowledged,
                        "data-testid": "stage-form-warning-ack",
                        children: "Got it"
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: fieldGroup,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                        className: StageForm_css_label,
                        htmlFor: "stage-form-slug",
                        children: "Slug"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        id: "stage-form-slug",
                        type: "text",
                        className: stage ? [
                            input,
                            inputDisabled
                        ].join(" ") : input,
                        value: slug,
                        onChange: (e)=>setSlug(e.target.value),
                        disabled: Boolean(stage),
                        placeholder: "design-review",
                        "data-testid": "stage-form-slug"
                    }),
                    stage && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: hint,
                        children: "Slugs are immutable after creation."
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: fieldGroup,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                        className: StageForm_css_label,
                        htmlFor: "stage-form-name",
                        children: "Name"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        id: "stage-form-name",
                        type: "text",
                        className: input,
                        value: name,
                        onChange: (e)=>setName(e.target.value),
                        placeholder: "Design Review",
                        "data-testid": "stage-form-name"
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: fieldGroup,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                        className: StageForm_css_label,
                        htmlFor: "stage-form-description",
                        children: "Description"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("textarea", {
                        id: "stage-form-description",
                        className: StageForm_css_textarea,
                        value: description,
                        onChange: (e)=>setDescription(e.target.value),
                        "data-testid": "stage-form-description"
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: flagsRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: checkboxRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "stage-form-is-entry",
                                type: "checkbox",
                                checked: isEntry,
                                onChange: (e)=>setIsEntry(e.target.checked),
                                "data-testid": "stage-form-is-entry"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: StageForm_css_label,
                                htmlFor: "stage-form-is-entry",
                                children: "Entry stage"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: checkboxRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "stage-form-is-terminal",
                                type: "checkbox",
                                checked: isTerminal,
                                onChange: (e)=>setIsTerminal(e.target.checked),
                                "data-testid": "stage-form-is-terminal"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: StageForm_css_label,
                                htmlFor: "stage-form-is-terminal",
                                children: "Terminal stage"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: checkboxRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "stage-form-enabled",
                                type: "checkbox",
                                checked: enabled,
                                onChange: (e)=>setEnabled(e.target.checked),
                                "data-testid": "stage-form-enabled"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: StageForm_css_label,
                                htmlFor: "stage-form-enabled",
                                children: "Enabled"
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: sectionHeading,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                children: "Outgoing transitions"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: addBtn,
                                onClick: handleAddTransition,
                                "data-testid": "stage-transition-add",
                                children: "+ Add transition"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: transitionList,
                        children: transitions.map((t, i)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(TransitionRow, {
                                transition: t,
                                allStages: allStages,
                                pipelineModeOptions: pipelineModes,
                                rowIndex: i,
                                onChange: (next)=>setTransitions((prev)=>prev.map((row)=>row.key === t.key ? next : row)),
                                onRemove: ()=>handleRemoveTransition(t.key)
                            }, t.key))
                    })
                ]
            }),
            stage && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(LivenessSection, {
                    stageSlug: stage.slug,
                    pipelineModeOptions: pipelineModes
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: graphSection,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: sectionHeading,
                        children: "Graph preview (read-only)"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(StageGraphDiagram, {
                        stages: allStages,
                        transitions: allTransitions
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: actionRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "submit",
                        className: submitBtn,
                        disabled: !canSubmit,
                        "data-testid": "stage-form-submit",
                        children: submitting ? "Saving…" : stage ? "Save changes" : "Create stage"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: cancelBtn,
                        onClick: onCancel,
                        "data-testid": "stage-form-cancel",
                        children: "Cancel"
                    }),
                    stage && (confirmingDelete ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: confirmDeleteBtn,
                                onClick: handleDeleteConfirm,
                                disabled: deleting,
                                "aria-label": "Confirm delete stage ".concat(stage.slug),
                                "data-testid": "stage-form-confirm-delete",
                                children: deleting ? "Deleting…" : "Confirm delete?"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: cancelBtn,
                                onClick: handleDeleteCancel,
                                disabled: deleting,
                                "data-testid": "stage-form-cancel-delete",
                                children: "Never mind"
                            })
                        ]
                    }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: deleteBtn,
                        onClick: handleDeleteClick,
                        disabled: deleteDisabled,
                        title: deleteTooltip,
                        "aria-label": "Delete stage ".concat(stage.slug),
                        "data-testid": "stage-form-delete",
                        children: "Delete"
                    }))
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2FsettingsListPage.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA71W25LaMAx95yv8WB6SSUKBxf2YHROLRItjZ2wHSDr9904MlFzIhe62T5mRoyPp-Eiy_x5akNUJA_JzQUjGLt4ZuU0p2UZBfvmxIISjyQUrKTkIcIb663HUEFtUkpJYiSKT9UnCckpOTH_zvPdwB1Wkq1W0rE9yxjnK5Onpr4V_zyJ0WfQiMoGJ9NBCZiiJQVrQtfmjMBYPpRcraUFaSkzOYvD2YM8AgwkF7ZCRC3lQ0noGK-j9zl3-7vwMmKSWkk0Q1LZYCaW7_wfu94zpBCUlQSvUaiLUfjkIG43AfnewAwxH1fIZAYTsleagPc04FqaXyuZR9ViqhTZ1rrnC-51cYSkJ8wsxSiDveGbX4Cw-JloVkneRs2EO1v2bWAcdKmiqTqAdIcMxZFsB6-eie0nmUdXG3Lwk5CGZDvdNMEHjdjnnMmCmEhqFbV1hKmcx2pKSwN-0jt--gsty-ccn0epMSejEj_I-m9qXvvss193LY7P69KWR0EDfP9APLENR9tIpJpqPjc-JRqx4ohI2JrKo7I4OR9SUYHYT0oTZ2fPPzcsGEjyXyJAWbuIzqUZ57Mrt4LBuSoz8tYas9khvSgj96G6axdOtRaWSMDRSJ6jMlcFrW2kQzOLJAVnN5N3-QCCBH5mJApOJ8Zm1uU1H18_faeifrR_oidPxlDMN0s7t4HTWlnnrOnE0bC-AdyfoulmLVNZjQqgz8JY7Oq-Rt8GscWVsKYAStExg3ML_GMYPo4mODsNZ2yac__B49TEzXnyjzOMX7KdVOLqaw_-7mn8D18pB-MILAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2FsettingsListPage_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA71W25LaMAx95yv8WB6SSUKBxf2YHROLRItjZ2wHSDr9904MlFzIhe62T5mRoyPp_Eiy_x5akNUJA_JzQUjGLt4ZuU0p2UZBfvmxIISjyQUrKTkIcIb663HUEFtUkpJYiSKT9UnCckpOTH_zvPdwB1Wkq1W0rE9yxjnK5Onpr4V_zyJ0WfQiMoGJ9NBCZiiJQVrQtfmjMBYPpRcraUFaSkzOYvD2YM8AgwkF7ZCRC3lQ0noGK_j9zl3_7vwMmKSWkk0Q1LZYCaW7_wfu94zpBCUlQSvUaiLUfjkIG43AfnewAwxH1fIZAYTsleagPc04FqaXyuZR9ViqhTZ1rrnC_51cYSkJ8wsxSiDveGbX4Cw_JloVkneRs2EO1v2bWAcdKmiqTqAdIcMxZFsB6_eie0nmUdXG3Lwk5CGZDvdNMEHjdjnnMmCmEhqFbV1hKmcx2pKSwN_0jt_gsty_ccn0epMSejEj_I_m9qXvvss193LY7P69KWR0EDfP9APLENR9tIpJpqPjc_JRqx4ohI2JrKo7I4OR9SUYHYT0oTZ2fPPzcsGEjyXyJAWbuIzqUZ57Mrt4LBuSoz8tYas9khvSgj96G6axdOtRaWSMDRSJ6jMlcFrW2kQzOLJAVnN5N3_QCCBH5mJApOJ8Zm1uU1H18_faeifrR_oidPxlDMN0s7t4HTWlnnrOnE0bC_AdyfoulmLVNZjQqgz8JY7Oq_Rt8GscWVsKYAStExg3ML_GMYPo4mODsNZ2yac_B49TEzXnyjzOMX7KdVOLqaw_7mn8D18pB_MILAAA_22_7D = __webpack_require__(27080);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Fsettings%2Fbacklog-stages%2Fpage.css.ts.vanilla.css%22%2C%22source%22%3A%22LngxdXdtdDAgewogIGFsaWduLWl0ZW1zOiBjZW50ZXI7Cn0KLngxdXdtdDEgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBqdXN0aWZ5LWNvbnRlbnQ6IHNwYWNlLWJldHdlZW47CiAgZ2FwOiB2YXIoLS1fMTllejJyejMwKTsKfQoueDF1d210MiB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MTIpOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYSk7CiAgbWFyZ2luLXRvcDogdmFyKC0tXzE5ZXoycnoyeSk7Cn0KLngxdXdtdDMgewogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MnkpIHZhcigtLV8xOWV6MnJ6MzApOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzYpOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgY3Vyc29yOiBwb2ludGVyOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6MTApOwogIGJhY2tncm91bmQ6IHRyYW5zcGFyZW50OwogIGNvbG9yOiB2YXIoLS1fMTllejJyejEyKTsKICBmbGV4LXNocmluazogMDsKfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Fsettings_2Fbacklog_stages_2Fpage_css_ts_vanilla_css_22_2C_22source_22_3A_22LngxdXdtdDAgewogIGFsaWduLWl0ZW1zOiBjZW50ZXI7Cn0KLngxdXdtdDEgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBqdXN0aWZ5LWNvbnRlbnQ6IHNwYWNlLWJldHdlZW47CiAgZ2FwOiB2YXIoLS1fMTllejJyejMwKTsKfQoueDF1d210MiB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MTIpOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYSk7CiAgbWFyZ2luLXRvcDogdmFyKC0tXzE5ZXoycnoyeSk7Cn0KLngxdXdtdDMgewogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MnkpIHZhcigtLV8xOWV6MnJ6MzApOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzYpOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgY3Vyc29yOiBwb2ludGVyOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6MTApOwogIGJhY2tncm91bmQ6IHRyYW5zcGFyZW50OwogIGNvbG9yOiB2YXIoLS1fMTllejJyejEyKTsKICBmbGV4LXNocmluazogMDsKfQ_22_7D = __webpack_require__(37048);
;// ./src/app/settings/backlog-stages/page.css.ts



var page_css_actionRow = 'x1uwmt0 _1tenzvie';
var badge = '_1tenzvic';
var container = '_1tenzvi0';
var description = '_1tenzvi3';
var page_css_empty = '_1tenzvii';
var page_css_errorMessage = 'x1uwmt1 _1tenzvij';
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
var retryBtn = 'x1uwmt3';
var rowErrorMessage = 'x1uwmt2';
var smallBtn = '_1tenzvih';
var title = '_1tenzvi2';
var toggle = '_1tenzvif';
var toggleOn = '_1tenzvig';
;// ./src/app/settings/backlog-stages/page.tsx
// analytics-exempt
/* __next_internal_client_entry_do_not_use__ default auto */ 
// +feature: settings-backlog-stages
//
// PageViewTracker (rendered below) calls usePageView() internally, so this
// page is exempt from analytics/require-page-analytics's literal-call check
// — same pattern as pipeline-modes/page.tsx.






// Note: `export const metadata` only applies to server components — this page
// is a client component (needs useState/useEffect for the list + form
// toggle), so metadata is intentionally omitted, matching pipeline-modes/page.tsx.
/**
 * Management page for backlog workflow stages (Epic 2.8, Story 2.8.1):
 * lists every stage (built-in and custom, enabled and disabled) with a quick
 * enable/disable toggle and create/edit actions. Mirrors
 * `pipeline-modes/page.tsx`'s list structure, per that story's precedent.
 *
 * Naming: this page and its copy say "Backlog Stages"/"Stage(s)" only, never
 * "Workflow(s)" — locked by research/ux.md §0's naming-collision finding.
 */ function BacklogStagesPageInner() {
    const { listStages, updateStage } = (0,useBacklogStages/* useBacklogStagesAdmin */.ll)();
    const searchParams = (0,nav/* useSearchParams */.ok)();
    const [stages, setStages] = (0,react.useState)([]);
    const [loading, setLoading] = (0,react.useState)(true);
    const [loadError, setLoadError] = (0,react.useState)(null);
    const [togglingId, setTogglingId] = (0,react.useState)(null);
    const [toggleErrors, setToggleErrors] = (0,react.useState)({});
    const [editingStage, setEditingStage] = (0,react.useState)(null);
    const [creating, setCreating] = (0,react.useState)(false);
    const refresh = (0,react.useCallback)(async ()=>{
        setLoading(true);
        setLoadError(null);
        try {
            const list = await listStages();
            setStages(list);
        } catch (err) {
            setLoadError(err instanceof Error ? err.message : String(err));
        } finally{
            setLoading(false);
        }
    }, [
        listStages
    ]);
    (0,react.useEffect)(()=>{
        refresh();
    }, [
        refresh
    ]);
    // ADR-005 Decision point 4: a `?editStage=<slug>` link (e.g. from the
    // item-detail gate checklist's "Fix in Stages settings" link) opens that
    // stage's edit form directly, instead of landing on the bare list. Read
    // once via useSearchParams on mount, matching this app's existing
    // search-param-driven state precedent (web-app/src/app/backlog/page.tsx's
    // `item` param). A no-op when the param is absent or matches no fetched
    // stage — never overrides a form the operator already has open by hand.
    const editStageAppliedRef = (0,react.useRef)(false);
    (0,react.useEffect)(()=>{
        if (editStageAppliedRef.current || loading || stages.length === 0) return;
        const slug = searchParams.get("editStage");
        if (!slug) {
            editStageAppliedRef.current = true;
            return;
        }
        const match = stages.find((s)=>s.slug === slug);
        if (match) {
            setCreating(false);
            setEditingStage(match);
        }
        editStageAppliedRef.current = true;
    }, [
        loading,
        stages,
        searchParams
    ]);
    const handleNewStage = (0,react.useCallback)(()=>{
        setEditingStage(null);
        setCreating(true);
    }, []);
    const handleEditStage = (0,react.useCallback)((stage)=>{
        setCreating(false);
        setEditingStage(stage);
    }, []);
    const closeForm = (0,react.useCallback)(()=>{
        setCreating(false);
        setEditingStage(null);
    }, []);
    // Success updates local state directly (no refetch) — matches
    // pipeline-modes/page.tsx's handleSaved, and ux.md Surface 2's "no
    // navigation, list updates in place" acceptance criterion.
    const handleSaved = (0,react.useCallback)((saved)=>{
        setStages((prev)=>{
            const idx = prev.findIndex((s)=>s.id === saved.id);
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
        setEditingStage(null);
    }, []);
    const handleDeleted = (0,react.useCallback)((id)=>{
        setStages((prev)=>prev.filter((s)=>s.id !== id));
        setCreating(false);
        setEditingStage(null);
    }, []);
    const handleToggleEnabled = (0,react.useCallback)(async (stage)=>{
        setTogglingId(stage.id);
        setToggleErrors((prev)=>{
            const { [stage.id]: _removed, ...rest } = prev;
            return rest;
        });
        try {
            const updated = await updateStage(stage.id, {
                enabled: !stage.enabled
            });
            setStages((prev)=>prev.map((s)=>s.id === updated.id ? updated : s));
        } catch (err) {
            // ux.md Surface 1 error table: the switch visually reverts (we never
            // applied the optimistic flip above) and the error is scoped to this
            // row, not a page-level banner.
            const message = err instanceof Error ? err.message : String(err);
            setToggleErrors((prev)=>({
                    ...prev,
                    [stage.id]: message
                }));
        } finally{
            setTogglingId(null);
        }
    }, [
        updateStage
    ]);
    const formOpen = creating || editingStage !== null;
    var _editingStage_id;
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
                                        children: "Backlog Stages"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                        className: description,
                                        children: "Configure workflow stages, the transitions between them, and the gates that must pass before a transition is allowed."
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: newBtn,
                                onClick: handleNewStage,
                                "data-testid": "backlog-stage-new",
                                children: "New Stage"
                            })
                        ]
                    }),
                    loadError && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: page_css_errorMessage,
                        role: "alert",
                        "data-testid": "backlog-stages-load-error",
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                children: [
                                    "Couldn't load stages — ",
                                    loadError
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: retryBtn,
                                onClick: refresh,
                                "data-testid": "backlog-stages-retry",
                                children: "Retry"
                            })
                        ]
                    }),
                    formOpen && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: formOverlay,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(StageForm, {
                            stage: editingStage,
                            allStages: stages,
                            onSaved: handleSaved,
                            onDeleted: handleDeleted,
                            onCancel: closeForm
                        }, (_editingStage_id = editingStage === null || editingStage === void 0 ? void 0 : editingStage.id) !== null && _editingStage_id !== void 0 ? _editingStage_id : "new")
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: list,
                        children: loading ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: page_css_empty,
                            children: "Loading…"
                        }) : stages.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: page_css_empty,
                            children: "No stages configured — the built-in 9-stage workflow is active by default."
                        }) : stages.map((s)=>{
                            const builtIn = (0,useBacklogStages/* isBuiltInStage */.oI)(s.slug);
                            return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: s.enabled ? listItem : [
                                    listItem,
                                    listItemDisabled
                                ].join(" "),
                                "data-testid": "backlog-stage-row-".concat(s.slug),
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: listItemInfo,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: listItemNameRow,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: listItemName,
                                                        children: s.name
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: listItemSlug,
                                                        children: s.slug
                                                    }),
                                                    builtIn && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                        className: badge,
                                                        children: [
                                                            "Built-in",
                                                            s.isTerminal ? ", terminal" : ""
                                                        ]
                                                    }),
                                                    !builtIn && !s.enabled && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: badge,
                                                        children: "disabled"
                                                    })
                                                ]
                                            }),
                                            s.description && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: listItemMeta,
                                                children: s.description
                                            }),
                                            toggleErrors[s.id] && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                className: rowErrorMessage,
                                                role: "alert",
                                                children: [
                                                    "Can't ",
                                                    s.enabled ? "disable" : "enable",
                                                    " '",
                                                    s.name,
                                                    "' — ",
                                                    toggleErrors[s.id]
                                                ]
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: page_css_actionRow,
                                        children: [
                                            !builtIn && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                role: "switch",
                                                "aria-checked": s.enabled,
                                                className: s.enabled ? [
                                                    toggle,
                                                    toggleOn
                                                ].join(" ") : toggle,
                                                onClick: ()=>handleToggleEnabled(s),
                                                disabled: togglingId === s.id,
                                                "aria-label": "".concat(s.enabled ? "Disable" : "Enable", " ").concat(s.name),
                                                "data-testid": "backlog-stage-toggle-".concat(s.slug)
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                className: smallBtn,
                                                onClick: ()=>handleEditStage(s),
                                                "data-testid": "backlog-stage-edit-".concat(s.slug),
                                                children: "Edit"
                                            })
                                        ]
                                    })
                                ]
                            }, s.id);
                        })
                    })
                ]
            })
        ]
    });
}
// useSearchParams requires a Suspense boundary — same wrapping pattern as
// web-app/src/app/backlog/page.tsx's BacklogPage/BacklogPageInner split.
function BacklogStagesPage() {
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(react.Suspense, {
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(BacklogStagesPageInner, {})
    });
}


/***/ }),

/***/ 26946:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Q: () => (/* binding */ visuallyHidden)
/* harmony export */ });
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Fa11y_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll85amk4YnAwIHsKICBwb3NpdGlvbjogYWJzb2x1dGU7CiAgd2lkdGg6IDFweDsKICBoZWlnaHQ6IDFweDsKICBwYWRkaW5nOiAwOwogIG1hcmdpbjogLTFweDsKICBvdmVyZmxvdzogaGlkZGVuOwogIGNsaXAtcGF0aDogaW5zZXQoNTAlKTsKICB3aGl0ZS1zcGFjZTogbm93cmFwOwogIGJvcmRlcjogMDsKfQ_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(47439);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Fa11y_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll85amk4YnAwIHsKICBwb3NpdGlvbjogYWJzb2x1dGU7CiAgd2lkdGg6IDFweDsKICBoZWlnaHQ6IDFweDsKICBwYWRkaW5nOiAwOwogIG1hcmdpbjogLTFweDsKICBvdmVyZmxvdzogaGlkZGVuOwogIGNsaXAtcGF0aDogaW5zZXQoNTAlKTsKICB3aGl0ZS1zcGFjZTogbm93cmFwOwogIGJvcmRlcjogMDsKfQ_22_7D__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Fa11y_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll85amk4YnAwIHsKICBwb3NpdGlvbjogYWJzb2x1dGU7CiAgd2lkdGg6IDFweDsKICBoZWlnaHQ6IDFweDsKICBwYWRkaW5nOiAwOwogIG1hcmdpbjogLTFweDsKICBvdmVyZmxvdzogaGlkZGVuOwogIGNsaXAtcGF0aDogaW5zZXQoNTAlKTsKICB3aGl0ZS1zcGFjZTogbm93cmFwOwogIGJvcmRlcjogMDsKfQ_22_7D__WEBPACK_IMPORTED_MODULE_0__);

var visuallyHidden = '_9ji8bp0';

/***/ }),

/***/ 35937:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 17376));


/***/ }),

/***/ 37048:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 43412:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ })

},
/******/ __webpack_require__ => { // webpackRuntimeModules
/******/ var __webpack_exec__ = (moduleId) => (__webpack_require__(__webpack_require__.s = moduleId))
/******/ __webpack_require__.O(0, [1869,852,4202,9353,8165,8890,5909,9691,3516,7358], () => (__webpack_exec__(35937)));
/******/ var __webpack_exports__ = __webpack_require__.O();
/******/ _N_E = __webpack_exports__;
/******/ }
]);
//# sourceMappingURL=page-4dfe605820539ba0.js.map