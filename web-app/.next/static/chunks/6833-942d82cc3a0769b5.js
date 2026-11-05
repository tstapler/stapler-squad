"use strict";
(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[6833],{

/***/ 3456:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

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

/***/ 3598:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   KH: () => (/* binding */ useBacklogFilters),
/* harmony export */   nY: () => (/* binding */ ALL_STATUSES),
/* harmony export */   py: () => (/* binding */ BACKLOG_FILTER_FIELDS),
/* harmony export */   ub: () => (/* binding */ filterBacklogItems)
/* harmony export */ });
/* harmony import */ var _lib_hooks_usePersistedViewState__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(88253);
/* harmony import */ var _lib_backlog_sourceBadge__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(82488);


// ---------------------------------------------------------------------------
// Shared backlog filter state — used by both the list view (page.tsx) and the
// board view (BacklogBoard.tsx) so that filtering a status/priority chip in
// one view is reflected in the other. Sort and group-by remain list-only
// (see .backlog-context.md AC 6) and are NOT part of this shared state.
// ---------------------------------------------------------------------------
const ALL_STATUSES = [
    "idea",
    "refining",
    "ready",
    "queued",
    "in_progress",
    "review",
    "pr_pending",
    "done",
    "archived"
];
// Keys and validators must exactly match the fields previously inlined in
// page.tsx's BACKLOG_VIEW_FIELDS so both views read/write the same
// localStorage entries (AC 3).
const BACKLOG_FILTER_FIELDS = {
    search: {
        key: "stapler-squad-backlog-search",
        defaultValue: ""
    },
    statusFilter: {
        key: "stapler-squad-backlog-status-filter",
        defaultValue: [],
        isValid: (v)=>Array.isArray(v) && v.every((s)=>ALL_STATUSES.includes(s))
    },
    priorityFilter: {
        key: "stapler-squad-backlog-priority-filter",
        defaultValue: [],
        isValid: (v)=>Array.isArray(v) && v.every((n)=>typeof n === "number")
    },
    sourceFilter: {
        key: "stapler-squad-backlog-source-filter",
        defaultValue: [],
        isValid: (v)=>Array.isArray(v) && v.every(_lib_backlog_sourceBadge__WEBPACK_IMPORTED_MODULE_1__/* .isFilterableSourceId */ .Zf)
    },
    showArchived: {
        key: "stapler-squad-backlog-show-archived",
        defaultValue: false,
        isValid: (v)=>typeof v === "boolean"
    }
};
function useBacklogFilters() {
    return (0,_lib_hooks_usePersistedViewState__WEBPACK_IMPORTED_MODULE_0__/* .usePersistedViewState */ .P)(BACKLOG_FILTER_FIELDS);
}
function filterBacklogItems(items, filters) {
    const q = filters.search.trim().toLowerCase();
    return items.filter((item)=>{
        if (!filters.showArchived && item.status === "archived") return false;
        if (filters.statusFilter.length > 0 && !filters.statusFilter.includes(item.status)) return false;
        if (filters.priorityFilter.length > 0 && !filters.priorityFilter.includes(item.priority)) return false;
        var _resolveSourceId;
        if (filters.sourceFilter.length > 0 && !filters.sourceFilter.includes((_resolveSourceId = (0,_lib_backlog_sourceBadge__WEBPACK_IMPORTED_MODULE_1__/* .resolveSourceId */ .TE)(item)) !== null && _resolveSourceId !== void 0 ? _resolveSourceId : "")) return false;
        var _item_description;
        if (q && !item.title.toLowerCase().includes(q) && !((_item_description = item.description) !== null && _item_description !== void 0 ? _item_description : "").toLowerCase().includes(q)) {
            return false;
        }
        return true;
    });
}


/***/ }),

/***/ 5658:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Nd: () => (/* binding */ getPrimaryCardAction),
/* harmony export */   tE: () => (/* binding */ getAvailableActions)
/* harmony export */ });
/* unused harmony export KNOWN_STATUS_MEMBERSHIP */
/* harmony import */ var _lib_backlog_planReviewStatus__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(57557);
// Single source of truth for "what actions does this backlog item's CURRENT
// state expose" — status + gate flags in, an action-id set out.
//
// Why this exists (docs/tasks/backlog-feature-improvement.md's 2026-08-03
// entry, item be676dab): action visibility in ActionsSection.tsx and
// TriageReviewPanel/PlanningSection used to be keyed off INCIDENTAL data that
// is supposed to correlate with the item's real state but isn't guaranteed
// to — `item.triageResult` truthy, `item.planArtifactsPath` truthy — instead
// of the actual state-machine gate condition
// (`!item.skipPlanning && !item.planApproved`, mirroring
// session/domain's ErrPlanRequired / DequeueNextQueuedItems' planning gate).
// When a triage session ran and produced nothing usable, `triageResult` and
// `planArtifactsPath` are BOTH empty even though the item is still very much
// gated on plan approval — the old conditions rendered nothing at all in
// that case, a silent dead end with no retry affordance. This module fixes
// that by deriving action availability from the gate condition directly, and
// makes the derivation exhaustive over every `KnownBacklogStatus` so a future
// new status is a compile error here (see the `never` check in the
// `default` arm) rather than a silently-missing branch.

// KNOWN_STATUS_MEMBERSHIP, not a hand-typed Set<KnownBacklogStatus>, is what makes
// asKnownStatus's runtime routing gate stay in sync with the KnownBacklogStatus union at
// COMPILE time. A plain `new Set<KnownBacklogStatus>([...])` looks type-safe but isn't: TS
// only checks that each listed literal belongs to the union, never that every union member
// is listed — add a 10th status to KnownBacklogStatus (useBacklogService.ts) without adding
// it here, and the switch below still forces you to add a `case` for it (its own `never`
// check catches that), but nothing forces this object to grow too. With that desync,
// asKnownStatus silently treats real items in the new status as "unknown" (status: undefined
// below), routing them to the delete-only branch instead of the case you just wrote —
// `tsc --strict` exits 0 the whole time. The exhaustiveness guarantee only ever covered the
// switch, not the gate that lets values reach it. `satisfies Record<KnownBacklogStatus, true>`
// closes that gap: TS errors here on both a missing key (a new status not yet added) and an
// extra key (a status removed from the union), so the object's key set can never silently
// drift from KnownBacklogStatus.
// Exported so itemActions.test.ts can assert its key count against a literal
// reference list — a cheap runtime tripwire on top of the `satisfies` check
// above (which is the actual compile-time guarantee; this just makes the
// invariant visible in test output too).
const KNOWN_STATUS_MEMBERSHIP = {
    idea: true,
    refining: true,
    ready: true,
    queued: true,
    in_progress: true,
    review: true,
    pr_pending: true,
    done: true,
    archived: true
};
function asKnownStatus(status) {
    return Object.prototype.hasOwnProperty.call(KNOWN_STATUS_MEMBERSHIP, status) ? status : undefined;
}
/** Statuses with an earlier stage to return to — mirrors the original
 * ActionsSection.tsx array, now sourced from one place instead of two
 * separately-maintained string arrays. */ const CAN_SEND_BACK_IDEA = new Set([
    "refining",
    "ready",
    "in_progress",
    "review",
    "pr_pending",
    "done"
]);
const CAN_SEND_BACK_READY = new Set([
    "in_progress",
    "review",
    "pr_pending",
    "done"
]);
/**
 * getAvailableActions is the single source of truth for which actions a
 * backlog item's current state exposes. Exhaustive over `KnownBacklogStatus`
 * via a switch with a `never` check in `default` — adding a new
 * `BacklogStatus` value without a case here is a compile error, not a silent
 * gap.
 */ function getAvailableActions(item) {
    const isGatedOnPlanApproval = !item.skipPlanning && !item.planApproved;
    const hasPlan = !!item.planArtifactsPath;
    const actions = new Set();
    const status = asKnownStatus(item.status);
    switch(status){
        case "idea":
            actions.add("mark_ready");
            actions.add("trigger_triage");
            break;
        case "refining":
            break;
        case "ready":
            actions.add("trigger_triage");
            actions.add("spawn_session");
            actions.add("spawn_session_autonomous");
            if (isGatedOnPlanApproval) {
                if (hasPlan) {
                    actions.add("approve_plan");
                } else if (item.triageStatus === "failed") {
                    // A ready item can also be reached via "Mark Ready" straight from idea,
                    // with no plan and no triage session ever having run (triageStatus
                    // undefined) — showing "Retry Triage" there would duplicate the
                    // "Trigger Triage" button above for a case with nothing to retry. Only
                    // surface it when there's actual evidence a triage attempt happened
                    // and produced nothing usable, matching the triage-failed banner's own
                    // condition in BacklogItemDetail.tsx.
                    actions.add("retry_triage");
                }
            }
            break;
        case "queued":
            if (isGatedOnPlanApproval) {
                if (hasPlan) {
                    actions.add("approve_plan");
                } else if (item.triageStatus === "failed") {
                    actions.add("retry_triage");
                }
            }
            break;
        case "in_progress":
            if (item.linkedSessions.length > 0) {
                actions.add("view_session");
                actions.add("restart_session");
            }
            break;
        case "review":
            if (!item.prUrl) actions.add("ship_pr");
            actions.add("override_done");
            actions.add("re_review");
            actions.add("manual_review");
            actions.add("restart_session");
            break;
        case "pr_pending":
            break;
        case "done":
            actions.add("archive");
            actions.add("reopen");
            break;
        case "archived":
            // ActionsSection replaces everything with an informational notice once
            // terminalState fires from a *live* archive event, but an item can also
            // simply BE archived on first load (e.g. opened directly from a "Show
            // Archived" list) without that watch ever having fired. Exposing
            // "unarchive" here — checked in ActionsSection's non-terminal branch —
            // covers that case; the terminal branch has its own Unarchive button
            // for the live-event path.
            actions.add("unarchive");
            break;
        case undefined:
            break;
        default:
            {
                const _exhaustive = status;
                return _exhaustive;
            }
    }
    if (status && CAN_SEND_BACK_IDEA.has(status)) {
        actions.add("send_back_idea");
    }
    if (status && CAN_SEND_BACK_READY.has(status)) {
        actions.add("send_back_ready");
    }
    actions.add("delete");
    return {
        actions,
        isGatedOnPlanApproval,
        hasPlan
    };
}
/**
 * getPrimaryCardAction picks the single action a board card's action button
 * should surface, built on `getAvailableActions` and `derivePlanReviewStatus`
 * rather than a second, independently-maintained status->action mapping (see
 * this module's header comment for the class of bug that divergence causes).
 *
 * For `ready`/`queued`, the candidate action set from `getAvailableActions`
 * can contain more than one entry (e.g. `trigger_triage` and `retry_triage`
 * both present), so precedence is an explicit if/else chain rather than a
 * read off `Set` iteration order, which is not guaranteed to match desired
 * priority.
 *
 * Exhaustive over `KnownBacklogStatus` via `asKnownStatus` + a switch with a
 * `never` check in `default`, mirroring `getAvailableActions` above — adding
 * a new status without a case here is a compile error, not a silent
 * fallthrough to the generic `{ label: item.status, ... }` placeholder.
 */ function getPrimaryCardAction(item) {
    const status = asKnownStatus(item.status);
    switch(status){
        case "idea":
            return {
                label: "Mark Ready",
                action: "mark_ready",
                disabled: item.acCriteria.length === 0,
                disabledReason: item.acCriteria.length === 0 ? "Add at least one AC criterion first" : undefined
            };
        case "refining":
            return {
                label: "Refining…",
                action: "refining",
                isDone: true
            };
        case "ready":
        case "queued":
            {
                const { actions } = getAvailableActions(item);
                const planStatus = (0,_lib_backlog_planReviewStatus__WEBPACK_IMPORTED_MODULE_0__/* .derivePlanReviewStatus */ .V)(item);
                const canSpawnSession = actions.has("spawn_session") && (planStatus === "approved" || planStatus === "skipped");
                if (actions.has("approve_plan")) {
                    return {
                        label: "Approve Plan",
                        action: "approve_plan"
                    };
                }
                if (actions.has("retry_triage")) {
                    return {
                        label: "Retry Triage",
                        action: "retry_triage"
                    };
                }
                if (canSpawnSession) {
                    return {
                        label: "Spawn Session",
                        action: "spawn_session"
                    };
                }
                if (status === "ready") {
                    return {
                        label: "Trigger Triage",
                        action: "trigger_triage",
                        disabled: !item.repoPath,
                        disabledReason: !item.repoPath ? "Set repository path first" : undefined
                    };
                }
                return {
                    label: "Queued",
                    action: "queued",
                    isDone: true,
                    disabled: true
                };
            }
        case "in_progress":
            return {
                label: "View Session",
                action: "view_session",
                disabled: item.linkedSessions.length === 0,
                disabledReason: item.linkedSessions.length === 0 ? "No linked session yet" : undefined
            };
        case "review":
            // "view_review" is a deliberate BacklogActionId (see that type's
            // definition above) distinct from ActionsSection's review-status
            // actions (ship_pr/override_done/re_review/manual_review) — it's a
            // pure navigation shim, mirroring "view_session", that board/page.tsx's
            // handleAction special-cases identically to "view_session".
            return {
                label: "View Review",
                action: "view_review"
            };
        case "pr_pending":
            // No status-specific primary action today, same as getAvailableActions'
            // "pr_pending" case — preserves the pre-existing fallback rendering.
            return {
                label: item.status,
                action: item.status,
                isDone: true
            };
        case "done":
            return {
                label: "Done ✓",
                action: "done",
                isDone: true
            };
        case "archived":
            return {
                label: "Archived",
                action: "archived",
                isDone: true
            };
        case undefined:
            // Unknown/forward-compatible status string — same generic fallback
            // getAvailableActions uses for a status outside KnownBacklogStatus.
            return {
                label: item.status,
                action: item.status,
                isDone: true
            };
        default:
            {
                const _exhaustive = status;
                return _exhaustive;
            }
    }
}


/***/ }),

/***/ 10001:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   DD: () => (/* binding */ title),
/* harmony export */   HN: () => (/* binding */ thRight),
/* harmony export */   IU: () => (/* binding */ errorState),
/* harmony export */   Nr: () => (/* binding */ card),
/* harmony export */   Pz: () => (/* binding */ tdRight),
/* harmony export */   lN: () => (/* binding */ emphasis),
/* harmony export */   td: () => (/* binding */ td),
/* harmony export */   th: () => (/* binding */ th),
/* harmony export */   tp: () => (/* binding */ table)
/* harmony export */ });
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fapp_2Finsights_2FItemStageCostTable_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA62T0W6DIBiF730KbpbMCxrArbb6MA0KIhsVg3TtWPruS0lMEKcu2e6M53j4_sPv7oTz7K0_7hH4SgCoaP0ujL50rAAf1DxDeMJH7ohxeVo_dG0YNwXA_Q0MWkkWuXjggoYyeRnioOzgPT1lTHZipqI0foPTMrknuxEUe9BGdxYO0vFZQOXjvX7lUrR25mi9o9ZKm1hDXjpTI2QHK22tPscW4qY8xPNcJbNtATBCT0EDtVaK9gMvwPhUbrIH0ZmPtvxmIVVSdAVQvLEr7ZHPuD1PuzAsSddp6GaTIrzusa6l3Wimw73MhjOPMyaeV_5l2HRH1D3S6gLZ0Ubm_vPf3S6Xy1rkHVYzsJo4z6rtR_PuBSgMrl_A6OkUvgRBAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(26628);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fapp_2Finsights_2FItemStageCostTable_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA62T0W6DIBiF730KbpbMCxrArbb6MA0KIhsVg3TtWPruS0lMEKcu2e6M53j4_sPv7oTz7K0_7hH4SgCoaP0ujL50rAAf1DxDeMJH7ohxeVo_dG0YNwXA_Q0MWkkWuXjggoYyeRnioOzgPT1lTHZipqI0foPTMrknuxEUe9BGdxYO0vFZQOXjvX7lUrR25mi9o9ZKm1hDXjpTI2QHK22tPscW4qY8xPNcJbNtATBCT0EDtVaK9gMvwPhUbrIH0ZmPtvxmIVVSdAVQvLEr7ZHPuD1PuzAsSddp6GaTIrzusa6l3Wimw73MhjOPMyaeV_5l2HRH1D3S6gLZ0Ubm_vPf3S6Xy1rkHVYzsJo4z6rtR_PuBSgMrl_A6OkUvgRBAAA_22_7D__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fapp_2Finsights_2FItemStageCostTable_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA62T0W6DIBiF730KbpbMCxrArbb6MA0KIhsVg3TtWPruS0lMEKcu2e6M53j4_sPv7oTz7K0_7hH4SgCoaP0ujL50rAAf1DxDeMJH7ohxeVo_dG0YNwXA_Q0MWkkWuXjggoYyeRnioOzgPT1lTHZipqI0foPTMrknuxEUe9BGdxYO0vFZQOXjvX7lUrR25mi9o9ZKm1hDXjpTI2QHK22tPscW4qY8xPNcJbNtATBCT0EDtVaK9gMvwPhUbrIH0ZmPtvxmIVVSdAVQvLEr7ZHPuD1PuzAsSddp6GaTIrzusa6l3Wimw73MhjOPMyaeV_5l2HRH1D3S6gLZ0Ubm_vPf3S6Xy1rkHVYzsJo4z6rtR_PuBSgMrl_A6OkUvgRBAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__);

var card = '_173jp960';
var emphasis = '_173jp967';
var errorState = '_173jp968';
var table = '_173jp962';
var td = '_173jp965';
var tdRight = '_173jp966 _173jp965';
var th = '_173jp963';
var thRight = '_173jp964 _173jp963';
var title = '_173jp961';

/***/ }),

/***/ 17213:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   WY: () => (/* binding */ blockedByLabel),
/* harmony export */   g8: () => (/* binding */ isBlockerUnresolved)
/* harmony export */ });
/* unused harmony export unresolvedBlockers */
// Mirrors the server's resolved definition (session.TerminalStatusStrings):
// a blocker is resolved once done or archived; deletion removes the edge.
const RESOLVED_STATUSES = new Set([
    "done",
    "archived"
]);
function isBlockerUnresolved(dep) {
    return !RESOLVED_STATUSES.has(dep.status);
}
function unresolvedBlockers(deps) {
    return (deps !== null && deps !== void 0 ? deps : []).filter(isBlockerUnresolved);
}
/** "blocked by <title>" / "blocked by <title> +N", or undefined when nothing blocks the item. */ function blockedByLabel(deps) {
    const open = unresolvedBlockers(deps);
    if (open.length === 0) return undefined;
    const first = open[0].title || open[0].publicId || open[0].itemId;
    return open.length === 1 ? "blocked by ".concat(first) : "blocked by ".concat(first, " +").concat(open.length - 1);
}


/***/ }),

/***/ 22479:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Y: () => (/* binding */ useDebouncedCallback),
/* harmony export */   d: () => (/* binding */ useDebounce)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);

/**
 * Hook that debounces a value by the specified delay
 * @param value - The value to debounce
 * @param delay - Delay in milliseconds
 * @returns The debounced value
 */ function useDebounce(value, delay) {
    const [debouncedValue, setDebouncedValue] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(value);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        const timer = setTimeout(()=>{
            setDebouncedValue(value);
        }, delay);
        return ()=>{
            clearTimeout(timer);
        };
    }, [
        value,
        delay
    ]);
    return debouncedValue;
}
/**
 * Hook that returns a debounced callback function
 * @param callback - The callback to debounce
 * @param delay - Delay in milliseconds
 * @returns Debounced callback function
 */ function useDebouncedCallback(callback, delay) {
    const timeoutIdRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    const debouncedCallback = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(function() {
        for(var _len = arguments.length, args = new Array(_len), _key = 0; _key < _len; _key++){
            args[_key] = arguments[_key];
        }
        if (timeoutIdRef.current) {
            clearTimeout(timeoutIdRef.current);
        }
        timeoutIdRef.current = setTimeout(()=>{
            timeoutIdRef.current = null;
            callback(...args);
        }, delay);
    }, [
        callback,
        delay
    ]);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        return ()=>{
            if (timeoutIdRef.current) {
                clearTimeout(timeoutIdRef.current);
            }
        };
    }, []);
    return debouncedCallback;
}


/***/ }),

/***/ 23522:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  l: () => (/* binding */ BacklogFilterBar)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ./src/lib/analytics/index.ts
var analytics = __webpack_require__(10671);
// EXTERNAL MODULE: ./src/lib/backlog/status.ts
var backlog_status = __webpack_require__(85546);
// EXTERNAL MODULE: ./src/lib/hooks/useBacklogFilters.ts
var useBacklogFilters = __webpack_require__(3598);
// EXTERNAL MODULE: ./src/lib/backlog/sourceBadge.ts
var sourceBadge = __webpack_require__(82488);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FBacklogFilterBar.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA91V247bIBB9z1fw2Eglsp1uk2U_ZkUMtkfBgADHSar99wqSbH0N23QrVX3xw8wZM-fMYVi9pnVha94m6McCIQZWC3oiqBD8-LJAiAooJQbHa0tQzqXjxodLqgk6UPMF49f0mZ8zc87OS5_RlDGQ5TC7TpbDSBbwO2UYN3innFM1Qak-IqsEsAG6CGDfFbaVAbknKHmPtMa3478vi7fF6kYpDZQ8gqDUg2uQuAXmKoLSbaIDwZoeb7F1do3NcMhOIw7JhQPN96VRjWTDkpQGQK6EMqPcriPAPPN9VyZDGTR2JO7moo6SDls481F-9ytf0BrEaUTtEBCqcQIkJ0gqyftakkLljQ2KXnuZJCWWgzKiBc15pQTjJlRPa5H367K-G0H6tvADpjx9SJnpnpYXf5gSJBa8cATRxqlen-t_ps-8MdbntILbOW0FjmOraR4mOrof30LzHzb79YL_N2YfKdaR5umRuf6Jks5QaTU1XLrIoGMS8s-VsOVQVm6EKH_LdOjCDxwoSTq8UbJKLeLU9pfNE6nU4bou5u227RKdFKzqL5Xv00_cnet453HZfO7Nf_dOpo8zbrk70efIROnjE40ZroysBRZbXTFvfO0NecYzm7_jmW3kh-lmnl0dPcwj3n4CTESRDIMJAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FBacklogFilterBar_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA91V247bIBB9z1fw2Eglsp1uk2U_ZkUMtkfBgADHSar99wqSbH0N23QrVX3xw8wZM_fMYVi9pnVha94m6McCIQZWC3oiqBD8_LJAiAooJQbHa0tQzqXjxodLqgk6UPMF49f0mZ8zc87OS5_RlDGQ5TC7TpbDSBbwO2UYN3innFM1Qak_IqsEsAG6CGDfFbaVAbknKHmPtMa3478vi7fF6kYpDZQ8gqDUg2uQuAXmKoLSbaIDwZoeb7F1do3NcMhOIw7JhQPN96VRjWTDkpQGQK6EMqPcriPAPPN9VyZDGTR2JO7moo6SDls481F_9ytf0BrEaUTtEBCqcQIkJ0gqyftakkLljQ2KXnuZJCWWgzKiBc15pQTjJlRPa5H367K_G0H6tvADpjx9SJnpnpYXf5gSJBa8cATRxqlen_t_ps_8MdbntILbOW0FjmOraR4mOrof30LzHzb79YL_N2YfKdaR5umRuf6Jks5QaTU1XLrIoGMS8s_VsOVQVm6EKH_LdOjCDxwoSTq8UbJKLeLU9pfNE6nU4bou5u227RKdFKzqL5Xv00_cnet453HZfO7Nf_dOpo8zbrk70efIROnjE40ZroysBRZbXTFvfO0NecYzm7_jmW3kh_lmnl0dPcwj3n4CTESRDIMJAAA_22_7D = __webpack_require__(1442);
;// ./src/components/backlog/BacklogFilterBar.css.ts


var filterBar = '_1mfsmew0';
var filterChip = '_1mfsmew7';
var filterChipActive = '_1mfsmew8';
var filterChipGroup = '_1mfsmew6';
var groupByLabel = '_1mfsmew2';
var groupBySelect = '_1mfsmew4';
var resetViewButton = '_1mfsmew5';
var searchInput = '_1mfsmew1';
var showArchivedLabel = '_1mfsmew3';
;// ./src/components/backlog/BacklogFilterBar.tsx
/* __next_internal_client_entry_do_not_use__ BacklogFilterBar auto */ 





function StatusFilterChips(param) {
    let { selected, onChange } = param;
    const { track } = (0,analytics/* useAnalytics */.st)();
    const toggle = (status)=>{
        const next = selected.includes(status) ? selected.filter((s)=>s !== status) : [
            ...selected,
            status
        ];
        onChange(next);
    };
    // Exclude "archived" from default chips (too noisy)
    const displayStatuses = useBacklogFilters/* ALL_STATUSES */.nY.filter((s)=>s !== "archived");
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: filterChipGroup,
        role: "group",
        "aria-label": "Filter by status",
        children: displayStatuses.map((status)=>{
            const active = selected.includes(status);
            return /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                className: "".concat(filterChip, " ").concat(active ? filterChipActive : ""),
                onClick: ()=>{
                    track({
                        name: "backlog_filter_status",
                        category: "user_action",
                        component: "BacklogFilterBar",
                        labels: {
                            status,
                            active: String(!selected.includes(status))
                        }
                    });
                    toggle(status);
                },
                "aria-pressed": active,
                "data-testid": "backlog-filter-status-".concat(status),
                children: (0,backlog_status/* getStatusLabel */.p)(status)
            }, status);
        })
    });
}
function PriorityFilterChips(param) {
    let { selected, onChange } = param;
    const { track } = (0,analytics/* useAnalytics */.st)();
    const toggle = (p)=>{
        const next = selected.includes(p) ? selected.filter((x)=>x !== p) : [
            ...selected,
            p
        ];
        onChange(next);
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: filterChipGroup,
        role: "group",
        "aria-label": "Filter by priority",
        children: [
            1,
            2,
            3,
            4,
            5
        ].map((p)=>{
            const active = selected.includes(p);
            return /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                type: "button",
                className: "".concat(filterChip, " ").concat(active ? filterChipActive : ""),
                onClick: ()=>{
                    track({
                        name: "backlog_filter_priority",
                        category: "user_action",
                        component: "BacklogFilterBar",
                        labels: {
                            priority: String(p),
                            active: String(!selected.includes(p))
                        }
                    });
                    toggle(p);
                },
                "aria-pressed": active,
                "data-testid": "backlog-filter-priority-".concat(p),
                children: [
                    "P",
                    p
                ]
            }, p);
        })
    });
}
function SourceFilterChips(param) {
    let { selected, onChange } = param;
    const toggle = (id)=>onChange(selected.includes(id) ? selected.filter((s)=>s !== id) : [
            ...selected,
            id
        ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: filterChipGroup,
        role: "group",
        "aria-label": "Filter by issue source",
        children: sourceBadge/* FILTERABLE_SOURCE_IDS */.is.map((id)=>{
            const active = selected.includes(id);
            return /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                className: "".concat(filterChip, " ").concat(active ? filterChipActive : ""),
                onClick: ()=>toggle(id),
                "aria-pressed": active,
                "data-testid": "backlog-filter-source-".concat(id),
                children: sourceBadge/* SOURCE_BADGE_CONFIG */.rg[id].label
            }, id);
        })
    });
}
function BacklogFilterBar(param) {
    let { search, onSearchChange, statusFilter, onStatusFilterChange, priorityFilter, onPriorityFilterChange, sourceFilter = [], onSourceFilterChange, showArchived, onShowArchivedChange, onResetView, showSortGroupControls, groupBy, onGroupByChange, showArchivedControl = true, doneCount = 0, onArchiveDone, archiveDoneBusy = false } = param;
    const { track } = (0,analytics/* useAnalytics */.st)();
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: filterBar,
        role: "search",
        "aria-label": "Filter backlog items",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                type: "search",
                className: searchInput,
                placeholder: "Search by title…",
                value: search,
                onChange: (e)=>onSearchChange(e.target.value),
                "aria-label": "Search backlog items",
                "data-testid": "backlog-search-input"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(StatusFilterChips, {
                selected: statusFilter,
                onChange: onStatusFilterChange
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(PriorityFilterChips, {
                selected: priorityFilter,
                onChange: onPriorityFilterChange
            }),
            onSourceFilterChange && /*#__PURE__*/ (0,jsx_runtime.jsx)(SourceFilterChips, {
                selected: sourceFilter,
                onChange: onSourceFilterChange
            }),
            showArchivedControl && /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                className: showArchivedLabel,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        type: "checkbox",
                        checked: showArchived,
                        onChange: (e)=>onShowArchivedChange(e.target.checked),
                        "aria-label": "Show archived items",
                        "data-testid": "backlog-show-archived-toggle"
                    }),
                    "Show Archived"
                ]
            }),
            showSortGroupControls && groupBy !== undefined && onGroupByChange && /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                className: groupByLabel,
                children: [
                    "Group by:",
                    " ",
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                        className: groupBySelect,
                        value: groupBy,
                        onChange: (e)=>onGroupByChange(e.target.value),
                        "aria-label": "Group by",
                        "data-testid": "backlog-group-by-select",
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                value: "none",
                                children: "None"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                value: "repoPath",
                                children: "Repository"
                            })
                        ]
                    })
                ]
            }),
            onArchiveDone && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                type: "button",
                className: resetViewButton,
                disabled: doneCount === 0 || archiveDoneBusy,
                onClick: ()=>{
                    track({
                        name: "backlog_archive_done",
                        category: "user_action",
                        component: "BacklogFilterBar",
                        labels: {
                            count: String(doneCount)
                        }
                    });
                    onArchiveDone();
                },
                "data-testid": "backlog-archive-done-button",
                children: [
                    "Archive done (",
                    doneCount,
                    ")"
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                className: resetViewButton,
                onClick: onResetView,
                "aria-label": "Reset view",
                "data-testid": "backlog-reset-view-button",
                children: "Reset view"
            })
        ]
    });
}


/***/ }),

/***/ 34243:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   a: () => (/* binding */ safeExternalHref),
/* harmony export */   q: () => (/* binding */ parseExternalRef)
/* harmony export */ });
const ISSUE_OR_PULL_PATH = /^\/([^/]+)\/([^/]+)\/(?:issues|pull)\/(\d+)\/?$/;
/**
 * Parses a GitHub (or GHE) issue/PR URL into its repo and number. Returns
 * undefined for anything that does not have the `/<owner>/<repo>/(issues|pull)/<n>`
 * shape so callers can fall back to the bare external id.
 */ function parseExternalRef(externalUrl) {
    if (!externalUrl) return undefined;
    let url;
    try {
        url = new URL(externalUrl);
    } catch (e) {
        return undefined;
    }
    if (url.protocol !== "https:" && url.protocol !== "http:") return undefined;
    const match = ISSUE_OR_PULL_PATH.exec(url.pathname);
    if (!match) return undefined;
    return {
        repo: "".concat(match[1], "/").concat(match[2]),
        number: match[3]
    };
}
/**
 * Returns the normalized href only for http(s) URLs, so a tracker-supplied
 * `javascript:`/`data:` URL is never rendered as a link target.
 */ function safeExternalHref(url) {
    if (!url) return undefined;
    try {
        const parsed = new URL(url);
        return parsed.protocol === "https:" || parsed.protocol === "http:" ? parsed.href : undefined;
    } catch (e) {
        return undefined;
    }
}


/***/ }),

/***/ 41062:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  Uw: () => (/* binding */ BacklogItemDetail)
});

// UNUSED EXPORTS: STEER_AUDIT_UNAVAILABLE_MESSAGE, steerFailureMessage

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect-web@2.1.1+0149f91553ca585e/node_modules/@connectrpc/connect-web/dist/esm/connect-transport.js + 8 modules
var connect_transport = __webpack_require__(46564);
// EXTERNAL MODULE: ./src/lib/hooks/useBacklogService.ts
var useBacklogService = __webpack_require__(8570);
// EXTERNAL MODULE: ./src/lib/hooks/useSessionService.ts + 1 modules
var useSessionService = __webpack_require__(73117);
// EXTERNAL MODULE: ./src/lib/contexts/NotificationContext.tsx + 12 modules
var NotificationContext = __webpack_require__(62275);
// EXTERNAL MODULE: ./src/lib/analytics/index.ts
var analytics = __webpack_require__(10671);
;// ./src/lib/backlog/currentWorkSession.ts

/**
 * The single source of truth for "the current work session" — the most
 * recent `role === "work"` LinkedSession on an item, or `undefined` if none
 * exists. Extracted verbatim from BacklogItemDetail.tsx's 4 independent
 * inline re-derivations (D3) so they can never drift out of sync.
 */ function getLatestWorkSession(item) {
    var _item_linkedSessions;
    return [
        ...(_item_linkedSessions = item === null || item === void 0 ? void 0 : item.linkedSessions) !== null && _item_linkedSessions !== void 0 ? _item_linkedSessions : []
    ].reverse().find((s)=>s.role === "work");
}
/** Memoized wrapper around getLatestWorkSession, keyed on item.linkedSessions. */ function useCurrentWorkSession(item) {
    return (0,react.useMemo)(()=>getLatestWorkSession(item), [
        item
    ]);
}

// EXTERNAL MODULE: ./src/lib/hooks/useStuckBacklogItems.ts
var useStuckBacklogItems = __webpack_require__(77717);
// EXTERNAL MODULE: ./src/components/backlog-stuck/stuckReason.ts + 1 modules
var stuckReason = __webpack_require__(91216);
;// ./src/lib/backlog/pipelineModeDisplay.ts
function resolvePipelineModeDisplay(session, modes) {
    var _session_pipelineModeSnapshot;
    const snapshot = (_session_pipelineModeSnapshot = session.pipelineModeSnapshot) !== null && _session_pipelineModeSnapshot !== void 0 ? _session_pipelineModeSnapshot : "";
    if (snapshot === "") {
        return {
            kind: "resolved",
            name: "default",
            drifted: false
        };
    }
    const match = modes.find((m)=>m.slug === snapshot);
    if (!match) {
        return {
            kind: "unrecognized",
            slug: snapshot
        };
    }
    var _session_pipelineModeSnapshotHash;
    const snapshotHash = (_session_pipelineModeSnapshotHash = session.pipelineModeSnapshotHash) !== null && _session_pipelineModeSnapshotHash !== void 0 ? _session_pipelineModeSnapshotHash : "";
    const drifted = snapshotHash !== "" && snapshotHash !== match.contentHash;
    return {
        kind: "resolved",
        name: match.name,
        drifted
    };
}
/**
 * Resolves a session's executor-provenance facts (fallback + drift) against
 * `mode` — the currently-fetched PipelineMode this session's
 * pipelineModeSnapshot resolves to (or undefined if unresolved/default,
 * matching resolvePipelineModeDisplay's own precedent of only comparing
 * drift when a live mode match exists).
 *
 * `drifted` compares session.executorSnapshotHash against
 * `mode.stageExecutorHashes[session.role]` — Task 5.2.4a's now-DENSE map, so
 * an unconfigured role's session (whose own hash is
 * ComputeExecutorHash("", "")) matches the mode's own
 * ComputeExecutorHash("", "") entry for that role by construction, and never
 * falsely reads as drifted. An empty snapshotHash (a session predating this
 * feature) or a missing mode entry is treated as "no signal" — never
 * drifted.
 */ function resolveExecutorProvenance(session, mode) {
    var _mode_stageExecutorHashes;
    var _session_executorFallbackReason;
    const reason = (_session_executorFallbackReason = session.executorFallbackReason) !== null && _session_executorFallbackReason !== void 0 ? _session_executorFallbackReason : "";
    var _session_configuredProgram, _session_resolvedProgram, _session_resolvedModel;
    const fallback = reason ? {
        configuredProgram: (_session_configuredProgram = session.configuredProgram) !== null && _session_configuredProgram !== void 0 ? _session_configuredProgram : "",
        resolvedProgram: (_session_resolvedProgram = session.resolvedProgram) !== null && _session_resolvedProgram !== void 0 ? _session_resolvedProgram : "",
        resolvedModel: (_session_resolvedModel = session.resolvedModel) !== null && _session_resolvedModel !== void 0 ? _session_resolvedModel : "",
        reason
    } : null;
    var _session_executorSnapshotHash;
    const snapshotHash = (_session_executorSnapshotHash = session.executorSnapshotHash) !== null && _session_executorSnapshotHash !== void 0 ? _session_executorSnapshotHash : "";
    var _mode_stageExecutorHashes_session_role;
    const modeHash = (_mode_stageExecutorHashes_session_role = mode === null || mode === void 0 ? void 0 : (_mode_stageExecutorHashes = mode.stageExecutorHashes) === null || _mode_stageExecutorHashes === void 0 ? void 0 : _mode_stageExecutorHashes[session.role]) !== null && _mode_stageExecutorHashes_session_role !== void 0 ? _mode_stageExecutorHashes_session_role : "";
    const drifted = snapshotHash !== "" && modeHash !== "" && snapshotHash !== modeHash;
    return {
        fallback,
        drifted
    };
}

;// ./src/lib/backlog/formatDate.ts
/**
 * Formats an ISO date string for display, e.g. "Jul 21, 2026, 03:45 PM".
 * Returns "—" when `iso` is missing.
 *
 * Deliberately distinct from `lib/utils/datetime.ts` (takes `Date | number`)
 * and `lib/utils/timestamp.ts`'s `formatDate` (takes a proto `Timestamp`) —
 * this one takes the raw ISO string BacklogItem fields carry and owns the
 * "—" empty-state fallback those two don't have.
 *
 * Extracted from BacklogItemDetail.tsx so it can be shared with the
 * detail/*Section.tsx components that were split out from it (Epic 3), which
 * had each been copy-pasting this same helper.
 */ function formatDate(iso) {
    if (!iso) return "—";
    return new Date(iso).toLocaleString(undefined, {
        year: "numeric",
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit"
    });
}

// EXTERNAL MODULE: ./src/lib/hooks/useVcsStatus.ts
var useVcsStatus = __webpack_require__(93758);
// EXTERNAL MODULE: ./src/gen/session/v1/backlog_pb.ts
var backlog_pb = __webpack_require__(79870);
// EXTERNAL MODULE: ./src/lib/api/transport.ts
var transport = __webpack_require__(21694);
// EXTERNAL MODULE: ./src/lib/hooks/useAbortableEffect.ts
var useAbortableEffect = __webpack_require__(64343);
;// ./src/lib/hooks/useBacklogItemShipStatus.ts
/* __next_internal_client_entry_do_not_use__ useBacklogItemShipStatus auto */ 




/**
 * Fetches whether a backlog item's code actually shipped to main, from
 * durable evidence (repo_path + the most recent work session's commit)
 * rather than a live per-session worktree. Unlike useVcsStatus, this works
 * even after a session's worktree has been cleaned up — the normal end
 * state for a done item — so it's the fallback for the Version Control
 * section once the live VCSStatus widget has nothing to show.
 *
 * No polling: shipped status doesn't change on its own once a session has
 * ended, so a one-shot fetch (plus manual refetch) is enough.
 */ function useBacklogItemShipStatus(itemId) {
    const [data, setData] = (0,react.useState)(null);
    const [loading, setLoading] = (0,react.useState)(!!itemId);
    const [refetchToken, setRefetchToken] = (0,react.useState)(0);
    (0,useAbortableEffect/* useAbortableEffect */.B)(async (signal)=>{
        if (!itemId) {
            setLoading(false);
            return;
        }
        setLoading(true);
        try {
            const client = (0,promise_client/* createClient */.UU)(backlog_pb/* BacklogService */.jSM, (0,transport/* getConnectTransport */.qW)());
            const response = await client.getBacklogItemShipStatus({
                itemId
            }, {
                signal
            });
            if (signal.aborted) return;
            var _response_status;
            setData((_response_status = response.status) !== null && _response_status !== void 0 ? _response_status : null);
        } catch (e) {
            if (signal.aborted) return;
            setData(null);
        } finally{
            if (!signal.aborted) setLoading(false);
        }
    }, [
        itemId,
        refetchToken
    ]);
    return {
        data,
        loading,
        refetch: ()=>setRefetchToken((t)=>t + 1)
    };
}

// EXTERNAL MODULE: ./src/lib/hooks/useWatchBacklogItems.ts
var useWatchBacklogItems = __webpack_require__(10414);
// EXTERNAL MODULE: ./src/lib/config.ts
var config = __webpack_require__(51202);
// EXTERNAL MODULE: ./src/components/guidance/GuidanceRequestPanel.tsx + 3 modules
var GuidanceRequestPanel = __webpack_require__(73090);
// EXTERNAL MODULE: ./src/gen/session/v1/session_pb.ts
var session_pb = __webpack_require__(23575);
// EXTERNAL MODULE: ./src/lib/store/index.ts
var store = __webpack_require__(93602);
// EXTERNAL MODULE: ./src/lib/store/store.ts + 1 modules
var store_store = __webpack_require__(25337);
// EXTERNAL MODULE: ./src/lib/store/backlogItemsSlice.ts
var backlogItemsSlice = __webpack_require__(63939);
// EXTERNAL MODULE: ./src/lib/store/sessionsSlice.ts
var sessionsSlice = __webpack_require__(60795);
// EXTERNAL MODULE: ./src/lib/vcs/adapters.ts
var adapters = __webpack_require__(85217);
;// ./src/lib/hooks/useSectionExpandState.ts

function storageKey(itemId, sectionKey) {
    return "backlog-detail-section-".concat(itemId, "-").concat(sectionKey);
}
/**
 * Per-item, per-section expand/collapse boolean persisted to localStorage
 * (key `backlog-detail-section-${itemId}-${sectionKey}`). Reads/writes are
 * wrapped in try/catch, matching RecentFilesSection.tsx's defensive
 * localStorage pattern — a throwing localStorage (private-browsing quota,
 * disabled storage, etc.) falls back to `defaultExpanded` instead of
 * crashing the component.
 */ function useSectionExpandState(itemId, sectionKey, defaultExpanded) {
    const [expanded, setExpandedState] = (0,react.useState)(()=>{
        try {
            const stored = localStorage.getItem(storageKey(itemId, sectionKey));
            if (stored === null) return defaultExpanded;
            return stored === "true";
        } catch (e) {
            return defaultExpanded;
        }
    });
    const setExpanded = (0,react.useCallback)((next)=>{
        setExpandedState(next);
        try {
            localStorage.setItem(storageKey(itemId, sectionKey), String(next));
        } catch (e) {
        // Persistence is best-effort only — in-memory state still updates.
        }
    }, [
        itemId,
        sectionKey
    ]);
    return [
        expanded,
        setExpanded
    ];
}

// EXTERNAL MODULE: ./src/lib/clipboard.ts
var clipboard = __webpack_require__(7202);
// EXTERNAL MODULE: ./src/lib/utils/connectError.ts
var connectError = __webpack_require__(8854);
;// ./src/components/backlog/detail/SendBackError.ts

/**
 * Thrown by handleSendBackWithFeedback (Task 2.2.1a) so SendBackFeedbackBox
 * can render distinct recovery copy per which of the 3 chained calls failed
 * (ux.md Surface 7) instead of one generic message for every failure.
 * itemStatusAfterFailure is the item's real status re-fetched after the
 * failure — needed because triggerTriage's own internal ready→idea CAS
 * (backlog_service_trigger_triage.go:249-260) can commit before a LATER
 * step in that same call fails, leaving the item at "idea" rather than
 * "ready" (pre-mortem P1 #1).
 */ class SendBackError extends Error {
    constructor(failedAt, itemStatusAfterFailure, cause){
        super(cause instanceof Error ? cause.message : String(cause), {
            cause
        }), this.failedAt = failedAt, this.itemStatusAfterFailure = itemStatusAfterFailure;
        this.name = "SendBackError";
    }
}
/**
 * Maps a handleSubmit failure to the recovery copy SendBackFeedbackBox
 * renders (ux.md Surface 6/7). A plain rejection with no SendBackError
 * wrapper falls back to the generic Surface 6 copy. `retryable` is true only
 * for the "landed at idea" case: idea→ready is itself a valid transition
 * (session/domain/backlog.go's validTransitions), so unlike the other two
 * branches — where resubmitting would replay transitionStatus against an
 * already-"ready" item and fail — retrying from "idea" is a genuine recovery
 * path, not a misleading affordance.
 */ const RETRIAGE_HEADLINE = "Sent back, but retriage didn't start";
function deriveSendBackErrorCopy(err) {
    if (!(err instanceof SendBackError) || err.failedAt === "transition") {
        return {
            headline: "Failed to send back",
            message: (0,connectError/* getErrorMessage */.u)(err, "Failed to send back.")
        };
    }
    // triggerTriage's own internal ready→idea CAS can commit before a later
    // step in that same call fails (pre-mortem P1 #1), landing the item at
    // "idea" rather than "ready".
    if (err.itemStatusAfterFailure === "idea") {
        return {
            headline: RETRIAGE_HEADLINE,
            message: 'The item moved back to "idea" before retriage could start. Your ' + "feedback is still in this box — click Retry to send it back to " + '"ready" and start retriage again.',
            retryable: true
        };
    }
    if (err.failedAt === "reject") {
        return {
            headline: RETRIAGE_HEADLINE,
            message: 'The item already moved to "ready." Close this form and use the ' + '"Request Changes" button below to record your feedback and retry ' + "— do not resubmit this box."
        };
    }
    // rejectPlan succeeded, so point at "Regenerate Plan with This Feedback"
    // (PlanVerdictBox), not the plain "Trigger Triage" button — that one sends
    // no feedback argument and the backend never reads the already-persisted
    // PlanRejectionReason on its own, so it would silently discard it.
    return {
        headline: RETRIAGE_HEADLINE,
        message: 'The item already moved to "ready" and your feedback was recorded. ' + 'Close this form and use the "Regenerate Plan with This Feedback" ' + "button below to retry — do not resubmit this box."
    };
}

// EXTERNAL MODULE: ./src/components/ui/Collapsible.tsx + 1 modules
var Collapsible = __webpack_require__(21791);
// EXTERNAL MODULE: ./src/components/common/InlineNotice.tsx + 1 modules
var InlineNotice = __webpack_require__(96307);
// EXTERNAL MODULE: ./src/components/backlog/ConnectionIndicator.tsx + 1 modules
var ConnectionIndicator = __webpack_require__(52454);
// EXTERNAL MODULE: ./src/lib/utils/datetime.ts
var datetime = __webpack_require__(3335);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/triangle-alert.mjs
var triangle_alert = __webpack_require__(18719);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/info.mjs
var info = __webpack_require__(17510);
// EXTERNAL MODULE: ./src/components/backlog/ClaimOverrideForm.tsx + 1 modules
var ClaimOverrideForm = __webpack_require__(55468);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2FDeepLinkErrorBanner.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA91SQW6DMBC88wofm4MjTNpGJY-JDN7AKsRGa5MQqv69stNEBhqlh5x6QTKzu7Mzs8utKCSUGlL2mTCm0LaNPOds10C_SRiTDVaao4ODvfzk1klyHqpkm7OjpBfOt-IDhoyGbFh4pDCkgDhJhZ2d1qzWoaaVSqGuZmga0J3RjlscYIYXEUPORNszaxpUkzJxGVPIcl-R6bSazhEiFJSmMTTDssUm-UqWV2vEv7Kmf-DM-b4xw9iXLPjyU4m6BsKg_uJFTaj3OUtHLavQEiScAKvazUTUY47X0HCQVKGeWXpesNQTPDZlumTE8PZ7uvcjDPJO5HH_3dz248783hOxvQe22H1tNMS5Xd9lR9bv3BrUDuhuKIc_XcXtpoJdDnrHFZSGpEOjc9ZpBdSgZ46XzWtzBIpjHg_XY3HrJ4iL7-h5mr4BnK_i7ucEAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2FDeepLinkErrorBanner_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA91SQW6DMBC88wofm4MjTNpGJY_JDN7AKsRGa5MQqv69stNEBhqlh5x6QTKzu7Mzs8utKCSUGlL2mTCm0LaNPOds10C_SRiTDVaao4ODvfzk1klyHqpkm7OjpBfOt_IDhoyGbFh4pDCkgDhJhZ2d1qzWoaaVSqGuZmga0J3RjlscYIYXEUPORNszaxpUkzJxGVPIcl_R6bSazhEiFJSmMTTDssUm_UqWV2vEv7Kmf_DM_b4xw9iXLPjyU4m6BsKg_uJFTaj3OUtHLavQEiScAKvazUTUY47X0HCQVKGeWXpesNQTPDZlumTE8PZ7uvcjDPJO5HH_3dz248783hOxvQe22H1tNMS5Xd9lR9bv3BrUDuhuKIc_XcXtpoJdDnrHFZSGpEOjc9ZpBdSgZ46XzWtzBIpjHg_XY3HrJ4iL7_h5mr4BnK_i7ucEAAA_22_7D = __webpack_require__(24058);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FClaimConflictBanner.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll84b2g0ODQwIHsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXgtZGlyZWN0aW9uOiBjb2x1bW47CiAgbWluLXdpZHRoOiAwOwogIGZsZXg6IDE7Cn0KLl84b2g0ODQxIHsKICBtYXJnaW4tdG9wOiB2YXIoLS1fMTllejJyejJ6KTsKfQouXzhvaDQ4NDIgewogIHBvc2l0aW9uOiBhYnNvbHV0ZTsKICB3aWR0aDogMXB4OwogIGhlaWdodDogMXB4OwogIG92ZXJmbG93OiBoaWRkZW47CiAgY2xpcDogcmVjdCgwIDAgMCAwKTsKICB3aGl0ZS1zcGFjZTogbm93cmFwOwp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FClaimConflictBanner_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll84b2g0ODQwIHsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXgtZGlyZWN0aW9uOiBjb2x1bW47CiAgbWluLXdpZHRoOiAwOwogIGZsZXg6IDE7Cn0KLl84b2g0ODQxIHsKICBtYXJnaW4tdG9wOiB2YXIoLS1fMTllejJyejJ6KTsKfQouXzhvaDQ4NDIgewogIHBvc2l0aW9uOiBhYnNvbHV0ZTsKICB3aWR0aDogMXB4OwogIGhlaWdodDogMXB4OwogIG92ZXJmbG93OiBoaWRkZW47CiAgY2xpcDogcmVjdCgwIDAgMCAwKTsKICB3aGl0ZS1zcGFjZTogbm93cmFwOwp9_22_7D = __webpack_require__(2417);
;// ./src/components/backlog/ClaimConflictBanner.css.ts



var actionButton = '_1baecne6';
var actions = '_1baecne5';
var ClaimConflictBanner_css_body = '_1baecne4';
var container = '_1baecne1';
var content = '_8oh4840';
var formSlot = '_8oh4841';
var ClaimConflictBanner_css_headline = '_1baecne3';
var icon = '_1baecne2';
var srOnly = '_8oh4842';
;// ./src/components/backlog/ClaimConflictBanner.tsx
/* __next_internal_client_entry_do_not_use__ ClaimConflictBanner auto */ 
// +feature: backlog:claim-conflict-banner




const COPIED_RESET_MS = 1500;
/**
 * Claim-conflict banner for the item detail (design/ux.md Surface 1). Always
 * role="status"/aria-live="polite": the current link is not a dead end, so this
 * never uses role="alert".
 */ function ClaimConflictBanner(param) {
    let { hostname, itemDeepLink, lastSeenAt, disputed = false, indeterminate = false, onCopyDeepLink, onOverride, onResolveDispute, onRetry } = param;
    const [copied, setCopied] = (0,react.useState)(false);
    const [formOpen, setFormOpen] = (0,react.useState)(false);
    const [busy, setBusy] = (0,react.useState)(false);
    const [formError, setFormError] = (0,react.useState)(null);
    const timerRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>()=>{
            if (timerRef.current) clearTimeout(timerRef.current);
        }, []);
    const handleCopy = (0,react.useCallback)(async ()=>{
        try {
            await navigator.clipboard.writeText(itemDeepLink);
        } catch (e) {
            return;
        }
        setCopied(true);
        onCopyDeepLink === null || onCopyDeepLink === void 0 ? void 0 : onCopyDeepLink();
        if (timerRef.current) clearTimeout(timerRef.current);
        timerRef.current = setTimeout(()=>setCopied(false), COPIED_RESET_MS);
    }, [
        itemDeepLink,
        onCopyDeepLink
    ]);
    const submit = disputed ? onResolveDispute : onOverride;
    const handleConfirm = (0,react.useCallback)(async (reason)=>{
        if (!submit) return;
        setBusy(true);
        setFormError(null);
        try {
            await submit(reason);
            setFormOpen(false);
        } catch (err) {
            setFormError(err instanceof Error ? err.message : "Action failed.");
        } finally{
            setBusy(false);
        }
    }, [
        submit
    ]);
    const Icon = disputed ? triangle_alert/* default */.A : info/* default */.A;
    const headline = indeterminate ? "Couldn't confirm this isn't claimed elsewhere" : disputed ? "Claim disputed — two hosts both claim this issue" : 'This issue is already claimed by "'.concat(hostname, '"');
    const body = indeterminate ? "None of the known hosts responded to the claim check. Proceeding is allowed, but check back if this seems off." : disputed ? '"'.concat(hostname, '" and another host have each claimed this. Nothing was auto-resolved; pick which claim is correct.') : '"'.concat(hostname, '" claimed this').concat(lastSeenAt ? " ".concat(lastSeenAt) : "", ". Working on it here risks a duplicate PR.");
    const actionLabel = disputed ? "Resolve — work on it here" : "Override — work on it here";
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: container,
        role: "status",
        "aria-live": "polite",
        "data-testid": "claim-conflict-banner",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)(Icon, {
                className: icon,
                size: 16,
                "aria-hidden": "true"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: content,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: ClaimConflictBanner_css_headline,
                        children: headline
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: ClaimConflictBanner_css_body,
                        children: body
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: actions,
                        children: indeterminate ? onRetry && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            type: "button",
                            className: actionButton,
                            onClick: onRetry,
                            "data-testid": "claim-banner-retry",
                            children: "Retry check"
                        }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "button",
                                    className: actionButton,
                                    onClick: ()=>void handleCopy(),
                                    "data-testid": "claim-banner-copy",
                                    children: copied ? "✓ Copied" : 'Copy link to "'.concat(hostname, "\"'s item")
                                }),
                                submit && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "button",
                                    className: actionButton,
                                    "aria-expanded": formOpen,
                                    onClick: ()=>setFormOpen((open)=>!open),
                                    "data-testid": "claim-banner-action",
                                    children: actionLabel
                                })
                            ]
                        })
                    }),
                    formOpen && submit && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: formSlot,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ClaimOverrideForm/* ClaimOverrideForm */.V, {
                            label: disputed ? "Reason for resolving this dispute (required)" : "Reason for working on this despite the claim (required)",
                            confirmLabel: disputed ? "Resolve" : "Override",
                            onConfirm: handleConfirm,
                            onCancel: ()=>setFormOpen(false),
                            busy: busy,
                            errorMessage: formError
                        })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: srOnly,
                        "aria-live": "polite",
                        "data-testid": "claim-banner-copy-status",
                        children: copied ? "Link to ".concat(hostname, "'s item copied to clipboard") : ""
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/components/backlog/ClaimedElsewhereNotice.tsx + 1 modules
var ClaimedElsewhereNotice = __webpack_require__(88627);
;// ./src/components/backlog/ItemClaimBanner.tsx
/* __next_internal_client_entry_do_not_use__ ItemClaimBanner auto */ 
// +feature: backlog:claim-conflict-banner





/**
 * Runs the cross-host claim check for an item's external URL in the
 * background and renders ClaimConflictBanner only when there is something to
 * say. Renders nothing while loading, when the feature is off, when the URL is
 * unclaimed, or when the RPC fails (design/ux.md Surface 1 error table).
 */ function ItemClaimBanner(param) {
    let { externalUrl } = param;
    const { checkCrossHostClaim, resolveClaimDispute } = (0,useBacklogService/* useBacklogService */.gG)();
    const [status, setStatus] = (0,react.useState)(null);
    const [dismissed, setDismissed] = (0,react.useState)(false);
    const runCheck = (0,react.useCallback)(async ()=>{
        // Optional call: tests that stub useBacklogService without this method
        // must keep rendering the detail view.
        const result = await (checkCrossHostClaim === null || checkCrossHostClaim === void 0 ? void 0 : checkCrossHostClaim(externalUrl));
        setStatus(result !== null && result !== void 0 ? result : null);
    }, [
        checkCrossHostClaim,
        externalUrl
    ]);
    (0,react.useEffect)(()=>{
        let cancelled = false;
        void (async ()=>{
            const result = await (checkCrossHostClaim === null || checkCrossHostClaim === void 0 ? void 0 : checkCrossHostClaim(externalUrl));
            if (!cancelled) setStatus(result !== null && result !== void 0 ? result : null);
        })();
        return ()=>{
            cancelled = true;
        };
    }, [
        checkCrossHostClaim,
        externalUrl
    ]);
    if (!status || !status.enabled || dismissed) return null;
    if (!status.claim) {
        if (status.checked) return null;
        return /*#__PURE__*/ (0,jsx_runtime.jsx)(ClaimConflictBanner, {
            hostname: "",
            itemDeepLink: "",
            indeterminate: true,
            onRetry: ()=>void runCheck()
        });
    }
    const claim = status.claim;
    const hostname = (0,ClaimedElsewhereNotice/* claimingHostLabel */.$)(claim);
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(ClaimConflictBanner, {
        hostname: hostname,
        itemDeepLink: claim.itemDeepLink,
        lastSeenAt: status.claimedAtUnix > 0 ? (0,datetime/* formatRelativeTime */.fw)(status.claimedAtUnix * 1000) : undefined,
        disputed: claim.disputed,
        onOverride: (reason)=>{
            // No server-side block exists for an item that has not been dequeued;
            // the override is recorded client-side and hides the banner for this view.
            console.info("[claim] override acknowledged", {
                externalUrl,
                claimingHost: claim.claimingHostId,
                reason
            });
            setDismissed(true);
        },
        onResolveDispute: async (reason)=>{
            await resolveClaimDispute(externalUrl, reason);
            await runCheck();
        }
    });
}

// EXTERNAL MODULE: ./src/components/backlog/BacklogItemForm.tsx + 1 modules
var BacklogItemForm = __webpack_require__(91618);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FAcCriteriaList.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61Vy5KjIBTd5ytYjgtS0X4kTX9MFwGCt4NgAXZHp-bfu0TLJlFDFrOy4F7OOffp9mu_L87lDv3dIMTB1Yq2BJ2UuLxvUPhiDlYwD0YTxIxqKt1bJK0J-qL2D8Yf-ZvoCtsVbdZbKmolaIJ2_aGmnIOW40mB89j5VgmCtNHiffNvsx0E5MsCqAKpMXhRueESO0-tX1PQBQUnoz120Ilb-9MxG2RogUsBsvQE5duXK6GzmHrpk8ziWibogLWolgnthe2vPxvn4dRiZrQX2semb-C-JCg_1AFgEjWej8ZyYbGlHBo3i-Y1-_UhKK8vyBkF_MZNZlMlXWlBn8dihCydaAWqnUXdJPJIo0pjb-rAHqXpKaTpSNlZWtNofgvAw3tmlLEz7CgovOjRBzQxPSeYivM6VaGSXEUVk72kyMo7ZJAm-4zJXgPZsmP2fwo4b4oKNB57stgNPejFxePQ2QTZvj0jjft1jbuJgaA8enJIhRX4uGDG0mHphBHzpTWNLCOgtwCUCvGRCfpdUssbbWV2vsdhvcGUCzvmcDUdNJWC-4Mz4Rzv4KgEUJiLCYndQYIUUhkj8Ye6dvwHgKcK2CMre209d8N6_gFaWUQyxgYAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FAcCriteriaList_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61Vy5KjIBTd5ytYjgtS0X4kTX9MFwGCt4NgAXZHp_bfu0TLJlFDFrOy4F7OOffp9mu_L87lDv3dIMTB1Yq2BJ2UuLxvUPhiDlYwD0YTxIxqKt1bJK0J_qL2D8Yf_ZvoCtsVbdZbKmolaIJ2_aGmnIOW40mB89j5VgmCtNHiffNvsx0E5MsCqAKpMXhRueESO0_tX1PQBQUnoz120Ilb_9MxG2RogUsBsvQE5duXK6GzmHrpk8ziWibogLWolgnthe2vPxvn4dRiZrQX2semb_C_JCg_1AFgEjWej8ZyYbGlHBo3i_Y1_UhKK8vyBkF_MZNZlMlXWlBn8dihCydaAWqnUXdJPJIo0pjb_rAHqXpKaTpSNlZWtNofgvAw3tmlLEz7CgovOjRBzQxPSeYivM6VaGSXEUVk72kyMo7ZJAm_4zJXgPZsmP2fwo4b4oKNB57stgNPejFxePQ2QTZvj0jjft1jbuJgaA8enJIhRX4uGDG0mHphBHzpTWNLCOgtwCUCvGRCfpdUssbbWV2vsdhvcGUCzvmcDUdNJWC_4Mz4Rzv4KgEUJiLCYndQYIUUhkj8Ye6dvwHgKcK2CMre209d8N6_gFaWUQyxgYAAA_22_7D = __webpack_require__(71575);
;// ./src/components/backlog/AcCriteriaList.css.ts


var AcCriteriaList_css_checkbox = 'v772kh2';
var checkboxDone = 'v772kh5';
var checkboxInProgress = 'v772kh4';
var checkboxPending = 'v772kh3';
var criterionIndex = 'v772kh6';
var criterionText = 'v772kh7';
var criterionTextDone = 'v772kh8';
var empty = 'v772khd';
var item = 'v772kh1';
var list = 'v772kh0';
var statusLabel = 'v772kh9';
var statusLabelDone = 'v772khc';
var statusLabelInProgress = 'v772khb';
var statusLabelPending = 'v772kha';
;// ./src/components/backlog/AcCriteriaList.tsx
/* __next_internal_client_entry_do_not_use__ AcCriteriaList auto */ 

function CheckboxIcon(param) {
    let { status } = param;
    const cls = status === "done" ? checkboxDone : status === "in_progress" ? checkboxInProgress : checkboxPending;
    const symbol = status === "done" ? "✓" : status === "in_progress" ? "~" : " ";
    const ariaLabel = status === "done" ? "Done" : status === "in_progress" ? "In progress" : "Pending";
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
        className: "".concat(AcCriteriaList_css_checkbox, " ").concat(cls),
        "aria-label": ariaLabel,
        role: "img",
        children: symbol
    });
}
const STATUS_LABEL = {
    pending: "Pending",
    in_progress: "In Progress",
    done: "Done"
};
const STATUS_LABEL_CLASS = {
    pending: statusLabelPending,
    in_progress: statusLabelInProgress,
    done: statusLabelDone
};
function AcCriteriaList(param) {
    let { criteria, readonly = true } = param;
    if (criteria.length === 0) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
            className: empty,
            children: "No acceptance criteria defined."
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("ol", {
        className: list,
        "aria-label": "Acceptance criteria",
        children: criteria.map((criterion)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                className: item,
                "data-testid": "ac-criterion-item",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(CheckboxIcon, {
                        status: criterion.status
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: criterionIndex,
                        "aria-hidden": "true",
                        children: [
                            criterion.index + 1,
                            "."
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: "".concat(criterionText, " ").concat(criterion.status === "done" ? criterionTextDone : ""),
                        children: criterion.text
                    }),
                    !readonly && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: "".concat(statusLabel, " ").concat(STATUS_LABEL_CLASS[criterion.status]),
                        "aria-label": "Status: ".concat(STATUS_LABEL[criterion.status]),
                        children: STATUS_LABEL[criterion.status]
                    })
                ]
            }, criterion.index))
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2FstatusBadgeVariants.css.ts.vanilla.css%22%2C%22source%22%3A%22LmVwejgyZzAgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6ZCk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnpnKTsKfQouZXB6ODJnMSB7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnoyZSk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MmYpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6MmcpOwp9Ci5lcHo4MmcyIHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejJrKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoybCk7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnoybSk7Cn0KLmVwejgyZzMgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6MmIpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejJjKTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyejJkKTsKfQouZXB6ODJnNCB7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnoyaCk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MmkpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6MmopOwp9Ci5lcHo4Mmc1IHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyemQpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejMpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6Zyk7Cn0KLmVwejgyZzYgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6eSk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6eik7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnp4KTsKfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2FstatusBadgeVariants_css_ts_vanilla_css_22_2C_22source_22_3A_22LmVwejgyZzAgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6ZCk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnpnKTsKfQouZXB6ODJnMSB7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnoyZSk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MmYpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6MmcpOwp9Ci5lcHo4MmcyIHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejJrKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoybCk7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnoybSk7Cn0KLmVwejgyZzMgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6MmIpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejJjKTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyejJkKTsKfQouZXB6ODJnNCB7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnoyaCk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MmkpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6MmopOwp9Ci5lcHo4Mmc1IHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyemQpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejMpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6Zyk7Cn0KLmVwejgyZzYgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6eSk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6eik7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnp4KTsKfQ_22_7D = __webpack_require__(23401);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FBacklogItemDetail.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA91bW2-jOBR-76-wVhqpXS0VkKRJ6ctI-yv2aeTAIbgBzBinTbLqf19xSbAJxiaQdnZeZhrw5fj4O_fD9y0cQoYTyNGP2Rw2R2yjf-8QChlNyj8Q4gyneUhZ4iFGOeZwbweweXi5Q-jjDiFOleNmT8LIj7vHegennBCQPIvxwUMkjUkK1jqm_rYY-k4CHnnIcbN98TMCsol483tNWQDMQ262RzmNSYD8HWOQ8r9pTFkzwuI0s_zimVeRluFilDCA4YDscg-9YXZvWT-cZzi67Dh7LgnGKUkwJzT1GsbYj8scFcRihkgakpRwKMbSDPuEH7xiQPE7jGFv5REj6dZD9otwdFc-ejHwPCEgDPxqR5_GuySVTm_b30rSsb_dMLpLgzbdFdn0DVgY03cPRSQIIBU3n1U3G8PeQ4442Dp4CO84LZ5lOAhIurngivNw-eRl2Fk2OOtYVqRwfqZQZJ-aKLuDKJnhi26G45hsUotwSHKv3o9jVoLjdZdzEh4sn6YcUu6hPMM-WGvg7wCqc7jHhx463eMFnfZD_206FXdr_Lbe1ZN7UfxUjglpyq2cHOHi_fpBFCbnLEytrWbS_TyV3BSp7pQsD6U0BR39_o7lxcuMkpQD6yfXL6eUiuIsEhLH7X7BWw4VPPUlm8PzUmZc8ebWlHOaqJkfSrxfDUZyl7xVrNdhvNn1eVq-lbfWrI4reVfeOjQgfq9vvTWCqMXE7oDM40zcfm3AUh9O4NQg4p0V74t_xS38Tlun2clEss-YK-xgN1VKtmItW6NyBIc9twSzvssyYD7OS9GOgXNgVoGcSgAf7QUk54VDnJD4cMGwnYTp4P_JHfJgcsxe9b5U4zYx0swbiZGgESSs3s41OkyzVXh7odk0p-nl74jz7vICuxCDz70S56XnSVlgrRngrYfK_ywcxyJh0TV4vVC2zauEpGfdNJ9X_m3xrHaBT49UbsWhbV9q1ppIiAm-ek19t2kfbtn14rbpNLvCrXhR4cle-CbyMiuRNZ3ER_22pNmQTCkCnUd6vQZoXRsdev3SCwDNzF3LXwZA69EAev1sAG0n1iO1vpi1QuaZFDJ_xXW6w68z0FzWdsBlGVxFPJWD2yyZaIyxHrA9QHJu6ZmdNcU5NulUJ1fFL-ktueJexZXVUK5wqoqoDPjWsIIascI8qHkqdUBEOJTHAw9lDKy2V5WVu-p8wJwfYvAQ4Tgmvl7xNsv_7BEk0c0zkSD2OeZPmZbp1ZdLo8yK3h51aMXhSnhoWFFKB6l0WrMOsh-dHAHO4S_JwDXPpdsx0sCO3l4m8qV7AcnxOoYqKhSSqnORXSnlhVtO3yEQZ-e61FQP8PtolOOsfKqju6b2iev2Mk4Udg-RE3zc7HyOfl35FLvprezbFEteqR-UnFka-VIwUf62Ycb72IhkJt_XfqoI5_oI_WRRYwgvNGjLnh7Ulk2G91E9UC5EOPY1FuiMI0eVdTLyxJXoOqh5djQC3n506quyQJKvkdK2p-F0V_hMUOS2Mh6_AicDnEfwVax0v5qVz78LKGeT6EipgCrk6uxzABKAT1ldOm7VwUgaASNcIms-StMo89tTxPylczDS-fjkXKAzFz0YcwfFmSu9z4WB9-ksDFPWPTG9li2Xhfumyl9jr3kBcUyynOQmkvFkFhmqSWsH3j7OygDyKMUNzrLfThsiw6hK4VTl0lMvSd09MWF9vsd_ccxq69tpY00FW97KEXTHC7XSKCQG9XrAOPFx_NKqRqzsU-pwX-xcMu2c9tnLnPZC6u9yrcTF8gUpKstqXzLBbEPSrhxMyx10qrLyLaom41WXGlKJGlE_JUCdjUpbtYosMArkUplv6y_j25i4XZcu6WPT2jiZLEzyb1medDQoCYYVdAbckbb66QRXhdxii0x3NCfRWI1wjXLKDgylaFi7zNDy2akNjJ5SbAxizMkbDInlHfC8NYSUQW0u63LPH3_Ia-N1TuMdr5PbBdN8HPv3loP-7LBvViEllbEuFOiiMoiCzS7_jDGHf-6thf2tgkRlPldyWWnVWVVa6BoUQ0lmVRf8LPPCpNtgjXMorJvebW8loiXvQNhU6D8Y0U1xkUlXbReNdb963GGiXVs999WA9xP0TsoVFMVbNax8I1MQGqjBpQy97XQJxnac2NEK58RjQXdVTVAgILk609BuU7lR0qavuuE8f1IWQVfGNAtU-ms0qizCxGFfKjg_F3vu0gBYpVWFKbrKpQakwkrZqLSHzhD_6oW8ZHQhr_Nyu5qTdRU_-VbM61vCrJ8NKk6MerJt5QFsneGp9q_jvcq1OX2eMMJK6sQhv9a7FdbgX1CUknWrSVPGAF-UezHOueVHJA7ECP-0TaWahAmKSp86rFeI-hQ65k3EkSpvcDNlXqFyYp39bnYkNXjMnalxEf0EMiAce692Y1t53MMo_MnurAnGjoOqCj09fCr_d24WbOhl2bWvbzuqshJdbUc1FLv7nlpVZdeZ0KVWi4DcJ-AqvrkbEkJqq9lrXZ7AHqiiRfoHqmD34js_A7lx50Plpucjx4WRlzhRDeuaFlDXxCFwnyZy9b8yLjL_mGSAS5ng_SmcXbGqh3KQmRN4vBwGlQ4OHj4DKpIv2q9uVrcMEAdT8zxW52v0bG9vrIzuckBfS5RANh7TTDrT1VO1hx5aJ9QbwWmcLpMsl8DFYd83Tuub1OV_Y-fE1zi2miyhY8DDQc15mha_kZ-9t4xvMLw3Xh9gV7xs2-bBFRO9d-6G5irvRP6ih_wRnzgIRG16i5nXtX6bSeFWZ6_1vQNa3aWuabrR73zwrr4FyZZ2tFi5kdChcNEJYd6y4Cq--1P3Kn9PICAY3QsO07zorHgoF5KbbTut28fdx39dY27dokUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FBacklogItemDetail_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA91bW2_jOBR_76_wVhqpXS0VkKRJ6ctI_yv2aeTAIbgBzBinTbLqf19xSbAJxiaQdnZeZhrw5fj4O_fD9y0cQoYTyNGP2Rw2R2yjf_8QChlNyj8Q4gyneUhZ4iFGOeZwbweweXi5Q_jjDiFOleNmT8LIj7vHegennBCQPIvxwUMkjUkK1jqm_rYY_k4CHnnIcbN98TMCsol483tNWQDMQ262RzmNSYD8HWOQ8r9pTFkzwuI0s_zimVeRluFilDCA4YDscg_9YXZvWT_cZzi67Dh7LgnGKUkwJzT1GsbYj8scFcRihkgakpRwKMbSDPuEH7xiQPE7jGFv5REj6dZD9otwdFc_ejHwPCEgDPxqR5_GuySVTm_b30rSsb_dMLpLgzbdFdn0DVgY03cPRSQIIBU3n1U3G8PeQ4442Dp4CO84LZ5lOAhIurngivNw_eRl2Fk2OOtYVqRwfqZQZJ_aKLuDKJnhi26G45hsUotwSHKv3o9jVoLjdZdzEh4sn6YcUu6hPMM_WGvg7wCqc7jHhx463eMFnfZD_206FXdr_Lbe1ZN7UfxUjglpyq2cHOHi_fpBFCbnLEytrWbS_TyV3BSp7pQsD6U0BR39_o7lxcuMkpQD6yfXL6eUiuIsEhLH7X7BWw4VPPUlm8PzUmZc8ebWlHOaqJkfSrxfDUZyl7xVrNdhvNn1eVq_lbfWrI4reVfeOjQgfq9vvTWCqMXE7oDM40zcfm3AUh9O4NQg4p0V74t_xS38Tlun2clEss_YK_xgN1VKtmItW6NyBIc9twSzvssyYD7OS9GOgXNgVoGcSgAf7QUk54VDnJD4cMGwnYTp4P_JHfJgcsxe9b5U4zYx0swbiZGgESSs3s41OkyzVXh7odk0p_nl74jz7vICuxCDz70S56XnSVlgrRngrYfK_ywcxyJh0TV4vVC2zauEpGfdNJ9X_m3xrHaBT49UbsWhbV9q1ppIiAm_ek19t2kfbtn14rbpNLvCrXhR4cle_CbyMiuRNZ3ER_22pNmQTCkCnUd6vQZoXRsdev3SCwDNzF3LXwZA69EAev1sAG0n1iO1vpi1QuaZFDJ_xXW6w68z0FzWdsBlGVxFPJWD2yyZaIyxHrA9QHJu6ZmdNcU5NulUJ1fFL_ktueJexZXVUK5wqoqoDPjWsIIascI8qHkqdUBEOJTHAw9lDKy2V5WVu_p8wJwfYvAQ4Tgmvl7xNsv_7BEk0c0zkSD2OeZPmZbp1ZdLo8yK3h51aMXhSnhoWFFKB6l0WrMOsh_dHAHO4S_JwDXPpdsx0sCO3l4m8qV7AcnxOoYqKhSSqnORXSnlhVtO3yEQZ_e61FQP8PtolOOsfKqju6b2iev2Mk4Udg_RE3zc7HyOfl35FLvprezbFEteqR_UnFka_VIwUf62Ycb72IhkJt_XfqoI5_oI_WRRYwgvNGjLnh7Ulk2G91E9UC5EOPY1FuiMI0eVdTLyxJXoOqh5djQC3n506quyQJKvkdK2p_F0V_hMUOS2Mh6_AicDnEfwVax0v5qVz78LKGeT6EipgCrk6uxzABKAT1ldOm7VwUgaASNcIms_StMo89tTxPylczDS_fjkXKAzFz0YcwfFmSu9z4WB9_ksDFPWPTG9li2Xhfumyl9jr3kBcUyynOQmkvFkFhmqSWsH3j7OygDyKMUNzrLfThsiw6hK4VTl0lMvSd09MWF9vsd_ccxq69tpY00FW97KEXTHC7XSKCQG9XrAOPFx_NKqRqzsU_pwX_xcMu2c9tnLnPZC6u9yrcTF8gUpKstqXzLBbEPSrhxMyx10qrLyLaom41WXGlKJGlE_JUCdjUpbtYosMArkUplv6y_j25i4XZcu6WPT2jiZLEzyb1medDQoCYYVdAbckbb66QRXhdxii0x3NCfRWI1wjXLKDgylaFi7zNDy2akNjJ5SbAxizMkbDInlHfC8NYSUQW0u63LPH3_Ia_N1TuMdr5PbBdN8HPv3loP_7LBvViEllbEuFOiiMoiCzS7_jDGHf_6thf2tgkRlPldyWWnVWVVa6BoUQ0lmVRf8LPPCpNtgjXMorJvebW8loiXvQNhU6D8Y0U1xkUlXbReNdb963GGiXVs999WA9xP0TsoVFMVbNax8I1MQGqjBpQy97XQJxnac2NEK58RjQXdVTVAgILk609BuU7lR0qavuuE8f1IWQVfGNAtU_ms0qizCxGFfKjg_F3vu0gBYpVWFKbrKpQakwkrZqLSHzhD_6oW8ZHQhr_Nyu5qTdRU_VbM61vCrJ8NKk6MerJt5QFsneGp9q_jvcq1OX2eMMJK6sQhv9a7FdbgX1CUknWrSVPGAF_UezHOueVHJA7ECP_0TaWahAmKSp86rFeI_hQ65k3EkSpvcDNlXqFyYp39bnYkNXjMnalxEf0EMiAce692Y1t53MMo_MnurAnGjoOqCj09fCr_d24WbOhl2bWvbzuqshJdbUc1FLv7nlpVZdeZ0KVWi4DcJ_AqvrkbEkJqq9lrXZ7AHqiiRfoHqmD34js_A7lx50Plpucjx4WRlzhRDeuaFlDXxCFwnyZy9b8yLjL_mGSAS5ng_SmcXbGqh3KQmRN4vBwGlQ4OHj4DKpIv2q9uVrcMEAdT8zxW52v0bG9vrIzuckBfS5RANh7TTDrT1VO1hx5aJ9QbwWmcLpMsl8DFYd83Tuub1OV_Y_fE1zi2miyhY8DDQc15mha_kZ_9t4xvMLw3Xh9gV7xs2_bBFRO9d_6G5irvRP6ih_wRnzgIRG16i5nXtX6bSeFWZ6_1vQNa3aWuabrR73zwrr4FyZZ2tFi5kdChcNEJYd6y4Cq_1P3Kn9PICAY3QsO07zorHgoF5KbbTut28fdx39dY27dokUAAA_22_7D = __webpack_require__(67880);
;// ./src/components/backlog/BacklogItemDetail.css.ts



var BacklogItemDetail_css_actionButton = '_34egzar';
var actionButtonDanger = '_34egzat';
var actionButtonSecondary = '_34egzas';
var actionsPanel = '_34egzaq';
var artifactsPath = '_34egza1c';
var autonomousHealthStrip = '_34egza2a';
var bannerBar = '_34egza4';
var branchBadge = '_34egza26';
var buttonSpinner = '_34egza1';
var cancelNotesButton = '_34egza1b';
var closeButton = '_34egzak';
var commitDetail = '_34egza1v';
var BacklogItemDetail_css_container = '_34egza2';
var copyButton = '_34egzah';
var dateMeta = '_34egzae';
var description = '_34egzao';
var editButton = '_34egzaj';
var emptyText = '_34egzap';
var errorBanner = '_34egza5';
var errorBannerDismiss = '_34egza6';
var errorState = '_34egza2b';
var executorDriftBadge = '_34egza12';
var executorFallbackBadge = '_34egza11';
var header = '_34egza7';
var headerActions = '_34egzai';
var headerRow = '_34egza8';
var idRow = '_34egzaf';
var idText = '_34egzag';
var inlineEditActions = '_34egza19';
var itemTitle = '_34egzaa';
var julesProvenanceMarker = '_34egza1m';
var loadingState = '_34egza1z';
var manualReviewActions = '_34egza2i';
var manualReviewForm = '_34egza2c';
var manualReviewLabel = '_34egza2f';
var manualReviewRow = '_34egza2e';
var manualReviewSelect = '_34egza2g';
var manualReviewTextarea = '_34egza2h';
var manualReviewTitle = '_34egza2d';
var metaRow = '_34egzab';
var notesTextarea = '_34egza18';
var pipelineDriftBadge = '_34egza10';
var pipelineGroup = '_34egzax';
var pipelineLabel = '_34egzay';
var pipelineValue = '_34egzaz';
var planSummary = '_34egza20';
var planTask = '_34egza22';
var planTaskBadge = '_34egza25';
var planTaskList = '_34egza21';
var planTaskMeta = '_34egza24';
var planTaskText = '_34egza23';
var priorityBadge = '_34egzad';
var progressNoteItem = '_34egza1t';
var progressNoteList = '_34egza1s';
var progressNoteMeta = '_34egza1u';
var reviewContextBox = '_34egza1j';
var reviewContextDate = '_34egza1o';
var reviewContextInfo = '_34egza1k';
var reviewContextLabel = '_34egza1l';
var reviewContextSessionId = '_34egza1n';
var saveNotesButton = '_34egza1a';
var scrollArea = '_34egza3';
var section = '_34egzal';
var sectionGroupLabel = '_34egzan';
var sectionTitle = '_34egzam';
var sessionCost = '_34egza28';
var sessionDate = '_34egza17';
var sessionDeleteBtn = '_34egza14';
var sessionEndedBadge = '_34egza27';
var sessionId = '_34egza15';
var sessionLink = '_34egza13';
var sessionList = '_34egzau';
var sessionRole = '_34egza16';
var sessionRow = '_34egzav';
var sessionRowMain = '_34egzaw';
var sessionTotalCost = '_34egza29';
var statusArchived = 'epz82g5';
var statusBadge = '_34egzac';
var statusDone = 'epz82g4';
var statusIdea = 'epz82g0';
var statusInProgress = 'epz82g2';
var statusReady = 'epz82g1';
var statusRefining = 'epz82g6';
var statusReview = 'epz82g3';
var titleGroup = '_34egza9';
var verdictCriterion = '_34egza1y';
var verdictDetail = '_34egza1w';
var verdictSummary = '_34egza1x';
var viewChangesButton = '_34egza1p';
var workflowEvent = '_34egza1e';
var workflowEventArrow = '_34egza1i';
var workflowEventFrom = '_34egza1h';
var workflowEventMeta = '_34egza1r';
var workflowEventNote = '_34egza1g';
var workflowEventRow = '_34egza1f';
var workflowEventTo = '_34egza1q';
var workflowTimeline = '_34egza1d';
;// ./src/components/backlog/detail/AcCriteriaSection.tsx
/* __next_internal_client_entry_do_not_use__ AcCriteriaSection auto */ 



/** Acceptance criteria as a CollapsibleGroup member; the header carries the done/total count. */ function AcCriteriaSection(param) {
    let { item, defaultExpanded } = param;
    const done = item.acCriteria.filter((c)=>c.status === "done").length;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(Collapsible/* CollapsibleSection */.i, {
        sectionKey: "ac-criteria",
        title: "Acceptance Criteria (".concat(done, "/").concat(item.acCriteria.length, ")"),
        defaultExpanded: defaultExpanded,
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: section,
            "data-testid": "ac-criteria-section",
            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(AcCriteriaList, {
                criteria: item.acCriteria
            })
        })
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FInlineError.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA71Uy47CMAy88xU5LocgWthFhI9ZhcZtvRSnclMorPj3VcIrPMUB7aVV7fE4M3YzyDuTLNxQ_PaEMNjUld4okVfQzXpC6AoLkuhg2SiRATlgHy50rcRK84eU38kUtilv023fZ2ptDFKhRFp3V4jRMCDmlg2wEkndicZWaK5gSQyTrA22zXWz0XSP0dmiYNuSuQYkSQBktrJ8k0tDLrfkZINbuCGf92e9XW-wNya5b4x_S4MMmUNLyndql_Tcm7con1zafN_if_AlDb4ciJBKYHQnY5qSkRZKDKOCUSgI7GvAonQ3_GXMPz7DHx7mmZAT0ef9AT4eU1CwZp_3Tx9aai6QpLO3NZu411foFdtPliCe_fE7a7nxJ68tHn-qu1qWL8wkWoeh_3LQOWkgs6z3y9mSAa7Qdz4fVZV2BRxP8ZKaYmGTtwt7Zdkur5NoEBXkTgndOuuDXpksDzuVBBiSXKNxpRLp-FCJdMIcY69edz9t4zDfyMySA3Ln1O4Pjvvuyj4FAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FInlineError_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA71Uy47CMAy88xU5LocgWthFhI9ZhcZtvRSnclMorPj3VcIrPMUB7aVV7fE4M3YzyDuTLNxQ_PaEMNjUld4okVfQzXpC6AoLkuhg2SiRATlgHy50rcRK84eU38kUtilv023fZ2ptDFKhRFp3V4jRMCDmlg2wEkndicZWaK5gSQyTrA22zXWz0XSP0dmiYNuSuQYkSQBktrJ8k0tDLrfkZINbuCGf92e9XW_wNya5b4x_S4MMmUNLyndql_Tcm7con1zafN_if_AlDb4ciJBKYHQnY5qSkRZKDKOCUSgI7GvAonQ3_GXMPz7DHx7mmZAT0ef9AT4eU1CwZp_3Tx9aai6QpLO3NZu411foFdtPliCe_fE7a7nxJ68tHn_qu1qWL8wkWoeh_3LQOWkgs6z3y9mSAa7Qdz4fVZV2BRxP8ZKaYmGTtwt7Zdkur5NoEBXkTgndOuuDXpksDzuVBBiSXKNxpRLp_FCJdMIcY69edz9t4zDfyMySA3Ln1O4Pjvvuyj4FAAA_22_7D = __webpack_require__(24589);
;// ./src/components/backlog/InlineError.css.ts


var InlineError_css_actionButton = 'fxd1kt6';
var InlineError_css_actions = 'fxd1kt5';
var blockContainer = 'fxd1kt1';
var body = 'fxd1kt4';
var dismissButton = 'fxd1kt7';
var headline = 'fxd1kt3';
var InlineError_css_icon = 'fxd1kt2';
var pillContainer = 'fxd1kt0';
;// ./src/components/backlog/InlineError.tsx
/* __next_internal_client_entry_do_not_use__ InlineError auto */ 
// +feature: backlog:inline-error

const COPY = {
    transient: {
        headline: "Triage failed",
        body: "Network error. The request could not be completed."
    },
    timeout: {
        headline: "Triage timed out",
        body: "The triage session did not complete within 3 minutes."
    },
    permanent: {
        headline: "Triage failed",
        body: "The triage session exited unexpectedly (exit code 1). Check the session logs for details."
    }
};
function InlineError(param) {
    let { type, onRetry, onDismiss, logsSessionId, customMessage, headline: headlineOverride, retryAriaLabel = "Retry triage" } = param;
    const copy = COPY[type];
    const bodyText = customMessage !== null && customMessage !== void 0 ? customMessage : copy.body;
    const headlineText = headlineOverride !== null && headlineOverride !== void 0 ? headlineOverride : copy.headline;
    if (type === "permanent") {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: blockContainer,
            role: "alert",
            "aria-live": "assertive",
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    style: {
                        display: "flex",
                        alignItems: "center",
                        gap: "inherit"
                    },
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: InlineError_css_icon,
                            "aria-hidden": "true",
                            children: "✕"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: headline,
                            children: headlineText
                        }),
                        onDismiss && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: dismissButton,
                            onClick: onDismiss,
                            "aria-label": "Dismiss error",
                            type: "button",
                            children: "\xd7"
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: body,
                    style: {
                        margin: 0
                    },
                    children: bodyText
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: InlineError_css_actions,
                    children: [
                        logsSessionId && /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                            className: InlineError_css_actionButton,
                            href: "/sessions/".concat(logsSessionId, "/logs"),
                            target: "_blank",
                            rel: "noopener noreferrer",
                            "aria-label": "View session logs (opens in new tab)",
                            children: "View session logs"
                        }),
                        onRetry && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: InlineError_css_actionButton,
                            onClick: onRetry,
                            "aria-label": retryAriaLabel,
                            type: "button",
                            children: "Retry ↺"
                        })
                    ]
                })
            ]
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: pillContainer,
        role: "alert",
        "aria-live": "assertive",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: InlineError_css_icon,
                "aria-hidden": "true",
                children: "✕"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: headline,
                        children: headlineText
                    }),
                    " — ",
                    bodyText
                ]
            }),
            onRetry && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                className: InlineError_css_actionButton,
                onClick: onRetry,
                "aria-label": retryAriaLabel,
                type: "button",
                children: "Retry ↺"
            }),
            onDismiss && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                className: dismissButton,
                onClick: onDismiss,
                "aria-label": "Dismiss error",
                type: "button",
                children: "\xd7"
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/components/backlog/TriageLoadingIndicator.tsx + 1 modules
var TriageLoadingIndicator = __webpack_require__(90080);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react-dom/index.js
var react_dom = __webpack_require__(80883);
;// ./src/lib/backlog/composeQuestionAnswerFeedback.ts
/**
 * Composes a Q:/A: feedback string for a single answered triage question,
 * preserving the question↔answer link without requiring a stable question
 * ID (none exists — see architecture.md §2.1). Handed as-is to the
 * existing triggerTriage(id, feedback) call.
 */ function composeQuestionAnswerFeedback(questionText, answerText) {
    return "Q: ".concat(questionText.trim(), "\nA: ").concat(answerText.trim());
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FTriageDiffSection.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA9VXyXLjIBC9-yu4TFVywCV5S0Iu8ydTSLQkKggUQN6m8u9TAo0HWZKjLE5qDi4deqH79XsNnqdRwZ_jCP2eIcS4qQQ9EJRrzh5nyH2xhbIS1AJOlahLaQiKM938nAetCNpSfYPxr_gBjgt9XMa3jaWkOucSW9VzWBxvH2cvs7k_Ou4enQnYN-HNFzOuIbVcSYL84SNHLg5hxoXLmClpseFH6NVHXX3OvgOeF7bnUTiPVAmle0c5k4W9xVZTaTKlS4LqqgKdUgONUYC1oLGpaMplTlA0j9ZQBpgkylpVXm5i-UoTyWsl-lB7EEAQt1TwNMi-GgZ9DFqEKsqYa6ZnHRouQglNn3KtasnwYI1b76U0A401Zbw2vRY3QR9vhCByJsEl4KIdcTxfBwCsvxuAOP4eBFruMkiVpl5azskWWtV5EUC08QxsdGgKzeUTQdEU4ZwS3LkE43ugXRQflH5nmdxPUs07pR_uNYfF6diH94s1vg677iaz64JI6Kev5sRlpILnEhsQGWn5Zam2DlwuT7WsVtV-uFe3eCuqQdpRXMsAAIKkkvAenD9yieR-6LU2TXWV4tKCuzNd-dxjdxYjO2CRQm1BO8h6oq0lA93MrhOQqbQ2eMsNTwS4QFXbxougRbVHRgnOBoFq3bDKMgPWeQeJ088gwvEi04-DEzhVwFwFO85sQVAcRT_O2bKJPFvG83e51FMMvaDRpEOmeATJpwmau5u00Z09oyUXh14j_ubU0IaDtjylIoTKs6A7_b8CaKsbbFOEgMP0C7IzqMwv_Uk6HiLioOl5UMxTkJ4s-GU0fTLXEnwWCP41tM4CGTc0EeBlopqXpz00L891WJJUFlMh1A5YJ_hKWyN_AxWmrPR4kgphsgr_K27kb-DGfTfwA9zIr8WN4qtfTMN_h36WwDhFJtUAElHJ0E1J97i9ZzarqNrfuko7W_HSzffyzzlrnc9urcAjH_N4-QNlOtUdmA8AAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FTriageDiffSection_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9VXyXLjIBC9_yu4TFVywCV5S0Iu8ydTSLQkKggUQN6m8u9TAo0HWZKjLE5qDi4deqH79XsNnqdRwZ_jCP2eIcS4qQQ9EJRrzh5nyH2xhbIS1AJOlahLaQiKM938nAetCNpSfYPxr_gBjgt9XMa3jaWkOucSW9VzWBxvH2cvs7k_Ou4enQnYN_HNFzOuIbVcSYL84SNHLg5hxoXLmClpseFH6NVHXX3OvgOeF7bnUTiPVAmle0c5k4W9xVZTaTKlS4LqqgKdUgONUYC1oLGpaMplTlA0j9ZQBpgkylpVXm5i_UoTyWsl_lB7EEAQt1TwNMi_GgZ9DFqEKsqYa6ZnHRouQglNn3KtasnwYI1b76U0A401Zbw2vRY3QR9vhCByJsEl4KIdcTxfBwCsvxuAOP4eBFruMkiVpl5azskWWtV5EUC08QxsdGgKzeUTQdEU4ZwS3LkE43ugXRQflH5nmdxPUs07pR_uNYfF6diH94s1vg677iaz64JI6Kev5sRlpILnEhsQGWn5Zam2DlwuT7WsVtV_uFe3eCuqQdpRXMsAAIKkkvAenD9yieR_6LU2TXWV4tKCuzNd_dxjdxYjO2CRQm1BO8h6oq0lA93MrhOQqbQ2eMsNTwS4QFXbxougRbVHRgnOBoFq3bDKMgPWeQeJ088gwvEi04_DEzhVwFwFO85sQVAcRT_O2bKJPFvG83e51FMMvaDRpEOmeATJpwmau5u00Z09oyUXh14j_ubU0IaDtjylIoTKs6A7_b8CaKsbbFOEgMP0C7IzqMwv_Uk6HiLioOl5UMxTkJ4s_GU0fTLXEnwWCP41tM4CGTc0EeBlopqXpz00L891WJJUFlMh1A5YJ_hKWyN_AxWmrPR4kgphsgr_K27kb_DGfTfwA9zIr8WN4qtfTMN_h36WwDhFJtUAElHJ0E1J97i9ZzarqNrfuko7W_HSzffyzzlrnc9urcAjH_N4_QNlOtUdmA8AAA_22_7D = __webpack_require__(20982);
;// ./src/components/backlog/TriageDiffSection.css.ts


var addedItem = 'c0hiq14';
var answerActions = 'c0hiq1e';
var answerCancelButton = 'c0hiq1g';
var answerForm = 'c0hiq1c';
var answerSubmitButton = 'c0hiq1f';
var answerTextarea = 'c0hiq1d';
var answerToggle = 'c0hiq1b';
var answeredMarker = 'c0hiq1h';
var columnHeader = 'c0hiq12';
var diffColumn = 'c0hiq11';
var diffContainer = 'c0hiq10';
var diffPrefix = 'c0hiq16';
var emptyState = 'c0hiq13';
var questionItem = 'c0hiq19';
var questionRow = 'c0hiq1a';
var questionsHeading = 'c0hiq18';
var questionsSection = 'c0hiq17';
var removedItem = 'c0hiq15';
;// ./src/components/backlog/TriageDiffSection.tsx
/* __next_internal_client_entry_do_not_use__ TriageDiffSection auto */ 
// +feature: backlog-triage-question-answer





/**
 * TriageDiffSection — two-column diff showing current AC vs suggested AC.
 * Filters out suggestions with rationale === "question" (those render in a
 * separate "Triage Questions" section below the diff).
 * Uses text comparison for simple set diff (v1 — no LCS needed).
 */ function TriageDiffSection(param) {
    let { currentCriteria, suggestedSuggestions, onAnswerQuestion } = param;
    // Filter out question suggestions
    const acSuggestions = suggestedSuggestions.filter((s)=>s.rationale !== "question");
    const questionSuggestions = suggestedSuggestions.filter((s)=>s.rationale === "question");
    const currentTexts = new Set(currentCriteria.map((c)=>c.text.trim()));
    const suggestedTexts = new Set(acSuggestions.map((s)=>s.text.trim()));
    // Added: in suggested but not in current
    const added = acSuggestions.filter((s)=>!currentTexts.has(s.text.trim()));
    // Removed: in current but not in suggested
    const removed = currentCriteria.filter((c)=>!suggestedTexts.has(c.text.trim()));
    // Retained: in both
    const retained = currentCriteria.filter((c)=>suggestedTexts.has(c.text.trim()));
    // Per-question answer form state (Task 1.1.1c). Keyed by the question's
    // index within questionSuggestions — no stable question ID exists.
    const [openIndex, setOpenIndex] = (0,react.useState)(null);
    const [answerDrafts, setAnswerDrafts] = (0,react.useState)({});
    const [answeredIndices, setAnsweredIndices] = (0,react.useState)(new Set());
    const [submittingIndex, setSubmittingIndex] = (0,react.useState)(null);
    const [errorIndex, setErrorIndex] = (0,react.useState)(null);
    const [errorMessage, setErrorMessage] = (0,react.useState)();
    // Set right before closing a form (Cancel/Escape/submit-success) so the
    // effect below can return focus to the toggle button only after the DOM
    // has re-rendered with the toggle visible again (it's `hidden` while its
    // form is open, so focusing it synchronously in the click handler — before
    // that re-render commits — would silently no-op).
    const [focusToggleIndex, setFocusToggleIndex] = (0,react.useState)(null);
    const toggleRefs = (0,react.useRef)({});
    const textareaRefs = (0,react.useRef)({});
    (0,react.useEffect)(()=>{
        if (openIndex !== null) {
            var _textareaRefs_current_openIndex;
            (_textareaRefs_current_openIndex = textareaRefs.current[openIndex]) === null || _textareaRefs_current_openIndex === void 0 ? void 0 : _textareaRefs_current_openIndex.focus();
        }
    }, [
        openIndex
    ]);
    (0,react.useEffect)(()=>{
        if (focusToggleIndex !== null) {
            var _toggleRefs_current_focusToggleIndex;
            (_toggleRefs_current_focusToggleIndex = toggleRefs.current[focusToggleIndex]) === null || _toggleRefs_current_focusToggleIndex === void 0 ? void 0 : _toggleRefs_current_focusToggleIndex.focus();
            setFocusToggleIndex(null);
        }
    }, [
        focusToggleIndex,
        openIndex
    ]);
    const handleCancel = (i)=>{
        setOpenIndex(null);
        setErrorIndex(null);
        setErrorMessage(undefined);
        setFocusToggleIndex(i);
    };
    const handleSubmit = async (i, questionText)=>{
        var _answerDrafts_i;
        const draft = ((_answerDrafts_i = answerDrafts[i]) !== null && _answerDrafts_i !== void 0 ? _answerDrafts_i : "").trim();
        if (!draft || submittingIndex === i || !onAnswerQuestion) return;
        setSubmittingIndex(i);
        setErrorIndex(null);
        setErrorMessage(undefined);
        try {
            const composed = composeQuestionAnswerFeedback(questionText, draft);
            await onAnswerQuestion(composed);
            // Keep the draft in state (rather than clearing it) — the
            // "✓ Answered: {draft}" marker below needs the submitted text to
            // display. Closing the form (openIndex -> null) already removes the
            // editable textarea, which is what "clears the draft" means in
            // practice: no stale editable input left behind.
            setAnsweredIndices((prev)=>new Set(prev).add(i));
            setOpenIndex(null);
        } catch (err) {
            setErrorIndex(i);
            setErrorMessage((0,connectError/* getErrorMessage */.u)(err, "Failed to submit answer. Please try again."));
        } finally{
            setSubmittingIndex(null);
        }
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: diffContainer,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: diffColumn,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: columnHeader,
                                children: [
                                    "Current (",
                                    currentCriteria.length,
                                    " ",
                                    currentCriteria.length === 1 ? "criterion" : "criteria",
                                    ")"
                                ]
                            }),
                            currentCriteria.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: emptyState,
                                children: "(none)"
                            }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                children: [
                                    retained.map((c)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: addedItem,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: diffPrefix,
                                                    "aria-hidden": "true",
                                                    children: " "
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    children: c.text
                                                })
                                            ]
                                        }, c.index)),
                                    removed.map((c)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: removedItem,
                                            "aria-label": "Removed: ".concat(c.text),
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: diffPrefix,
                                                    "aria-hidden": "true",
                                                    children: "−"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    children: c.text
                                                })
                                            ]
                                        }, c.index))
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: diffColumn,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: columnHeader,
                                children: [
                                    "Suggested (",
                                    acSuggestions.length,
                                    " ",
                                    acSuggestions.length === 1 ? "criterion" : "criteria",
                                    ")"
                                ]
                            }),
                            acSuggestions.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: emptyState,
                                children: "(no changes suggested)"
                            }) : acSuggestions.map((s, i)=>{
                                const isNew = !currentTexts.has(s.text.trim());
                                return isNew ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: addedItem,
                                    "aria-label": "Added: ".concat(s.text),
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: diffPrefix,
                                            "aria-hidden": "true",
                                            children: "+"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            children: s.text
                                        })
                                    ]
                                }, i) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: addedItem,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: diffPrefix,
                                            "aria-hidden": "true",
                                            children: " "
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            children: s.text
                                        })
                                    ]
                                }, i);
                            })
                        ]
                    })
                ]
            }),
            questionSuggestions.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: questionsSection,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h4", {
                        className: questionsHeading,
                        children: "Triage Questions"
                    }),
                    questionSuggestions.map((q, i)=>{
                        const isOpen = openIndex === i;
                        const isAnswered = answeredIndices.has(i);
                        const isSubmitting = submittingIndex === i;
                        var _answerDrafts_i;
                        const draft = (_answerDrafts_i = answerDrafts[i]) !== null && _answerDrafts_i !== void 0 ? _answerDrafts_i : "";
                        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: questionRow,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: questionItem,
                                    children: q.text
                                }),
                                isAnswered ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    role: "status",
                                    "aria-live": "polite",
                                    className: answeredMarker,
                                    children: [
                                        "✓ Answered: ",
                                        draft
                                    ]
                                }) : onAnswerQuestion && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                            type: "button",
                                            ref: (el)=>{
                                                toggleRefs.current[i] = el;
                                            },
                                            "aria-expanded": isOpen,
                                            "aria-controls": "triage-question-answer-input-".concat(i),
                                            "data-testid": "triage-question-answer-toggle-".concat(i),
                                            className: answerToggle,
                                            onClick: ()=>setOpenIndex(isOpen ? null : i),
                                            hidden: isOpen,
                                            children: "Answer ▸"
                                        }),
                                        isOpen && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: answerForm,
                                            role: "form",
                                            "aria-label": "Answer: ".concat(q.text),
                                            children: [
                                                errorIndex === i && errorMessage && // Real retry: re-attempts submit with the same
                                                // draft (still in answerDrafts, untouched on
                                                // failure). Dismiss closes the form entirely,
                                                // matching the prior "Skip without applying"
                                                // behavior (discards the in-progress answer).
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)(InlineError, {
                                                    type: "transient",
                                                    headline: "Failed to submit answer",
                                                    retryAriaLabel: "Retry submitting answer",
                                                    customMessage: errorMessage,
                                                    onRetry: ()=>void handleSubmit(i, q.text),
                                                    onDismiss: ()=>handleCancel(i)
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("textarea", {
                                                    id: "triage-question-answer-input-".concat(i),
                                                    "data-testid": "triage-question-answer-input-".concat(i),
                                                    ref: (el)=>{
                                                        textareaRefs.current[i] = el;
                                                    },
                                                    rows: 3,
                                                    className: answerTextarea,
                                                    value: draft,
                                                    disabled: isSubmitting,
                                                    onChange: (e)=>setAnswerDrafts((prev)=>({
                                                                ...prev,
                                                                [i]: e.target.value
                                                            })),
                                                    onKeyDown: (e)=>{
                                                        if (e.key === "Escape") {
                                                            handleCancel(i);
                                                        }
                                                    }
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                    className: answerActions,
                                                    children: [
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                            type: "button",
                                                            className: answerSubmitButton,
                                                            "data-testid": "triage-question-answer-submit-".concat(i),
                                                            "aria-disabled": !draft.trim() || isSubmitting,
                                                            "aria-busy": isSubmitting,
                                                            disabled: !draft.trim() || isSubmitting,
                                                            onClick: ()=>{
                                                                if (!draft.trim() || isSubmitting) return;
                                                                void handleSubmit(i, q.text);
                                                            },
                                                            children: isSubmitting ? "Submitting…" : "Submit"
                                                        }),
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                            type: "button",
                                                            className: answerCancelButton,
                                                            "data-testid": "triage-question-answer-cancel-".concat(i),
                                                            disabled: isSubmitting,
                                                            onClick: ()=>handleCancel(i),
                                                            children: "Cancel"
                                                        })
                                                    ]
                                                })
                                            ]
                                        })
                                    ]
                                })
                            ]
                        }, i);
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FTriageErrorBanner.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA9VTTW-DMAy991fkuB5SFVrtI_sxUyAmtRZimgTWMu2_T4RpolBWJu2yUyT7-cV-z940rdQnv2XvK8Yymb9qR7VVPCdDTrBGujvOX5InaFPXJsn6uYORU-AES6oT82RQjWHbAYw7qbD2Y6rdQ8RUUim0epLtGRT6ysizYIWBUxfoXq7QQR6QrGA5mbq0XUbLakyStpGklE6j5RmFQOXVjz5Wm16FJKpQkA3cYwsTcBYJr0uTDj4TbDtgTSPrZJa5jr_rdos8Ked7Ol64ZcnCbVvuf7IlPa-vG3VLsph_A9SHMEHofoDa-W6CitAGcF0oOGk99kaPauyFTuJADbhFao0KC8przxv0mBmIBFQHgxYES2eWu9f7C8apKDyEiB4Q72d6iRNV0oEN84u06MRgwYX9Qyv3v7Dy8bLwj638BPAu7UgYBQAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FTriageErrorBanner_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9VTTW_DMAy991fkuB5SFVrtI_sxUyAmtRZimgTWMu2_T4RpolBWJu2yUyT7_cV_z940rdQnv2XvK8Yymb9qR7VVPCdDTrBGujvOX5InaFPXJsn6uYORU_AES6oT82RQjWHbAYw7qbD2Y6rdQ8RUUim0epLtGRT6ysizYIWBUxfoXq7QQR6QrGA5mbq0XUbLakyStpGklE6j5RmFQOXVjz5Wm16FJKpQkA3cYwsTcBYJr0uTDj4TbDtgTSPrZJa5jr_rdos8Ked7Ol64ZcnCbVvuf7IlPa_vG3VLsph_A9SHMEHofoDa_W6CitAGcF0oOGk99kaPauyFTuJADbhFao0KC8przxv0mBmIBFQHgxYES2eWu9f7C8apKDyEiB4Q72d6iRNV0oEN84u06MRgwYX9Qyv3v7Dy8bLwj638BPAu7UgYBQAA_22_7D = __webpack_require__(801);
;// ./src/components/backlog/TriageErrorBanner.css.ts


var TriageErrorBanner_css_actions = 'vzagxs2';
var banner = 'vzagxs0';
var TriageErrorBanner_css_message = 'vzagxs1';
var reloadButton = 'vzagxs3';
var skipButton = 'vzagxs4';
;// ./src/components/backlog/TriageErrorBanner.tsx
/* __next_internal_client_entry_do_not_use__ TriageErrorBanner auto */ 

/**
 * TriageErrorBanner — inline error state for the TriageReviewPanel.
 * Renders above the diff so context is preserved during error recovery.
 * Uses role="alert" so screen readers announce it immediately.
 */ function TriageErrorBanner(param) {
    let { message, onReload, onSkip } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        role: "alert",
        className: banner,
        "data-testid": "triage-error-banner",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: TriageErrorBanner_css_message,
                children: message
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: TriageErrorBanner_css_actions,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: reloadButton,
                        onClick: onReload,
                        "data-testid": "triage-error-reload",
                        children: "Reload item"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: skipButton,
                        onClick: onSkip,
                        "data-testid": "triage-error-skip",
                        children: "Skip without applying"
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/hooks/useHistoryFullTextSearch.ts
var useHistoryFullTextSearch = __webpack_require__(86082);
// EXTERNAL MODULE: ./src/lib/hooks/useDebounce.ts
var useDebounce = __webpack_require__(22479);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FTriageRelatedWorkSection.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA7WVzW6DMAzH730KXyath0xA99XsYaqUGGotJCgJrDDt3aemrQa0rKjrTkix_XMc-28eVvGyVk0dwecMQJIrlWg4ZAq3bzMIXybJYurJaA6pUVWhd5ZclBxqYe8ZW8VLbBPbJu38bfY1ezgg44D8IOk3HOIoutuFlUJK0vlJaDM_BwPIjPbMUYvDiMU62FOjjB3aomBai_Q9t6bSkp31isXezViJlkNcbsEZRXLg9t7xYlZIqtzJXZ77hfPMpJVjNTlaKwzPYCqvSCOHZCRNEdIc3JjJMoc-eHfJya26BKDIeeZ8o5CDNhp3Z4WwOWkOUa9TUfcGi1vcoJn_NgrtpO69TGoeTmwegMetZxJTY8W-hOOjHLKT3qAlH04q63ZHpSHt0YZoK7SjfeAAr3uzseAbU6MNr3ipxNdB5H9N1WNgXVJasH8g5Rt_4pH_rsWfVE8XUolxUNIDPV8Pinugl0nFj4J6ovmhvl5PTTp7by9P8kJR2sUvz6tQKMo1I4-F45DicTjHl8B1Zfc7IUZmOUiiFBa1H0UVk0RcTBbx7f4uYurM_2EXiKt3wfqPMvoG1WFOrPYHAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FTriageRelatedWorkSection_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA7WVzW6DMAzH730KXyath0xA99XsYaqUGGotJCgJrDDt3aemrQa0rKjrTkix_XMc_28eVvGyVk0dwecMQJIrlWg4ZAq3bzMIXybJYurJaA6pUVWhd5ZclBxqYe8ZW8VLbBPbJu38bfY1ezgg44D8IOk3HOIoutuFlUJK0vlJaDM_BwPIjPbMUYvDiMU62FOjjB3aomBai_Q9t6bSkp31isXezViJlkNcbsEZRXLg9t7xYlZIqtzJXZ77hfPMpJVjNTlaKwzPYCqvSCOHZCRNEdIc3JjJMoc_eHfJya26BKDIeeZ8o5CDNhp3Z4WwOWkOUa9TUfcGi1vcoJn_NgrtpO69TGoeTmwegMetZxJTY8W_hOOjHLKT3qAlH04q63ZHpSHt0YZoK7SjfeAAr3uzseAbU6MNr3ipxNdB5H9N1WNgXVJasH8g5Rt_4pH_rsWfVE8XUolxUNIDPV8Pinugl0nFj4J6ovmhvl5PTTp7by9P8kJR2sUvz6tQKMo1I4_F45DicTjHl8B1Zfc7IUZmOUiiFBa1H0UVk0RcTBbx7f4uYurM_2EXiKt3wfqPMvoG1WFOrPYHAAA_22_7D = __webpack_require__(81458);
;// ./src/components/backlog/TriageRelatedWorkSection.css.ts


var TriageRelatedWorkSection_css_emptyState = '_19vlyv8';
var TriageRelatedWorkSection_css_errorState = '_19vlyv9';
var hintText = '_19vlyvb';
var input = '_19vlyv1';
var moreMatchesText = '_19vlyv6';
var resultCard = '_19vlyv3';
var resultList = '_19vlyv2';
var resultMeta = '_19vlyv5';
var resultTitle = '_19vlyv4';
var retryButton = '_19vlyva';
var TriageRelatedWorkSection_css_section = '_19vlyv0';
var snippetText = '_19vlyv7';
;// ./src/components/backlog/TriageRelatedWorkSection.tsx
/* __next_internal_client_entry_do_not_use__ TriageRelatedWorkSection auto */ 
// +feature: backlog:triage-related-work




function TriageRelatedWorkSection_formatDate(date) {
    if (!date) return "";
    return date.toLocaleDateString();
}
function SessionHitCard(param) {
    let { hit } = param;
    var _hit_snippets_;
    const snippet = (_hit_snippets_ = hit.snippets[0]) === null || _hit_snippets_ === void 0 ? void 0 : _hit_snippets_.text;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("li", {
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("a", {
            href: "/history?sessionId=".concat(encodeURIComponent(hit.sessionId), "&messageIndex=").concat(hit.messageIndex),
            target: "_blank",
            rel: "noopener noreferrer",
            className: resultCard,
            "data-testid": "triage-related-work-hit-".concat(hit.sessionId),
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    className: resultTitle,
                    children: hit.sessionName
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    className: resultMeta,
                    children: TriageRelatedWorkSection_formatDate(hit.metadata.createdAt)
                }),
                snippet && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: snippetText,
                    children: snippet.length > 200 ? "".concat(snippet.slice(0, 200), "…") : snippet
                }),
                hit.moreMatchesInSessionCount > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                    className: moreMatchesText,
                    children: [
                        "+",
                        hit.moreMatchesInSessionCount,
                        " more ",
                        hit.moreMatchesInSessionCount === 1 ? "match" : "matches",
                        " in this session"
                    ]
                })
            ]
        })
    });
}
/**
 * TriageRelatedWorkSection — "Find related past work" search box inside
 * TriageReviewPanel. Pre-populated with the backlog item's title, auto-
 * searching on mount, scoped to the item's repo, session-deduped.
 */ function TriageRelatedWorkSection(param) {
    let { itemTitle, repoPath } = param;
    const { results, loading, error, search, clearSearch } = (0,useHistoryFullTextSearch/* useHistoryFullTextSearch */.c)({
        autoSearch: false
    });
    const [query, setQuery] = (0,react.useState)(itemTitle);
    const debouncedQuery = (0,useDebounce/* useDebounce */.d)(query, 300);
    const runSearch = (q)=>{
        // includeContext is deliberately omitted: v1 ships snippet-only cards
        // (see project_plans/session-search-fts5/design/ux.md's fallback
        // recommendation) — SessionHitCard never renders contextWindow/
        // bookendFirst/bookendLast, so requesting it would only cost the server
        // an extra full-conversation-file read per hit on every debounced
        // keystroke for data nothing displays. Re-add if a future card design
        // actually surfaces the context window.
        search({
            query: q,
            project: repoPath,
            groupBySession: true,
            excludeAutomationSessions: true,
            limit: 5
        });
    };
    (0,react.useEffect)(()=>{
        if (!debouncedQuery.trim()) {
            clearSearch();
            return;
        }
        runSearch(debouncedQuery);
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        debouncedQuery,
        repoPath
    ]);
    const retry = ()=>runSearch(debouncedQuery);
    const hasCompletedSearch = !loading && debouncedQuery.trim().length > 0;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: TriageRelatedWorkSection_css_section,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: hintText,
                "aria-hidden": "true",
                children: "Find related past work"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                type: "search",
                "aria-label": "Search past sessions for ".concat(itemTitle || "this item"),
                value: query,
                onChange: (e)=>setQuery(e.target.value),
                "data-testid": "triage-related-work-input",
                className: input
            }),
            error && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                role: "alert",
                className: TriageRelatedWorkSection_css_errorState,
                "data-testid": "triage-related-work-error",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        children: "Search failed —"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: retryButton,
                        onClick: retry,
                        children: "Retry"
                    })
                ]
            }),
            !error && loading && results.length === 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: hintText,
                children: "Searching…"
            }),
            !error && hasCompletedSearch && results.length === 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: TriageRelatedWorkSection_css_emptyState,
                "data-testid": "triage-related-work-empty",
                children: "No related past sessions found — this looks like new territory."
            }),
            results.length > 0 && // aria-live="off": this list re-renders on every debounced keystroke;
            // TriageReviewPanel's ancestor <section aria-live="polite"> exists to
            // announce triage-completion state, not this search box's per-edit
            // churn, so results are explicitly opted out of that ambient region.
            /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                className: resultList,
                "data-testid": "triage-related-work-results",
                "aria-live": "off",
                children: results.map((hit)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(SessionHitCard, {
                        hit: hit
                    }, hit.sessionId))
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FTriageReviewPanel.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA8VYyXKjMBC95yt0SVVyUApw7CTkR-aWEqgBjYVEhPDC1Pz7FJLjsIONU3OyC1q9vtfq5unDpSX5TDcO-nOHUEDCbaxkISgOJZfKRzuiHjD-cN-g9FT58vheSUlFQWEOkfbRKjugXHJGW6KHuqgilBV5W9vKqssIpUzEnbeueZsSFTOBA6m1TAdkKMszTo4-ijgcqgfVL6ZMQaiZFD4KJS9SUb2JSdZR4jy-3_29e_pKhWtS0dH5u8g1i444lEKD0D7KMxICDkDvAYxuwlksMNOQ5j4KQWhQDcWeURxJoXHOSuj4EZpgzPs9sDjRHYnESPSWxqlly0dOw_BqoLhaEZFnRIHQg3q9Wh19JKSA6bpuxurqHR_bT8rvwHsTE9iwC5VX_mWS2dQiGwCzNW6dEc2irvxE7kCZRIzkb4oAr22tkQyLHO9YzgIORrssNGcCfOQNMCM1lk5iWEZRDtpIN1Q_96PwImR7x6a76wkAkiUAtEDRcNDYlCWSKvVRkWWgQpIb1HDQGhSuiGOA4Tw5a0iHcbuZ8DcY9sZt0wEhk-7kFJf7tGmYerEUsbDWMvORO1C-aIRor7coWlnve8aV8aq-zWrd6XCuPq-g-Gjr9so2xU-Neknvi4eawJ5RnfjIdZz7y3rCW60nTKWvc5SynAQcqOV8hWh9rBC9rnsppMaEc7kH2jr-Y32DLOj1bgMIQwSApTf7FfAI_gM8yAXweG0fXQQP8nPwCG7VoWo6w1ltekbxhjBZs2UT2qppysS5sb-YiMew17znO_bIiC_BLIJsZxNkVs4ikjJ-7ASyMxIKTsdBaRYS3kyWxVETP63-3hsobyYdZs0NI1PBVO1rtqLr73yve-dbPfrIwUdME87ChrHYGOMsPwt9Zaeh5YwlZ_m20SZP0k_Ixi5hDOSaKD0-M1w9KdX8YTb_xmKimNieoq7PIyeKDRahpu73WZ2P3J4pbN2Q3g4Yn4LeuUAVJXumpXkLyxSvoyno7ROmwYy3Bkl7RbLJwZPPvl_m1S9dvOtdOsI_VyN8zQNhPMjk1y0bsUN1v1X57d3hrVX7NWFdu6StdfOXEw2_HvDauTeyJWaCGkQ5Tj8lh9b8gSX9si8ftxiNAllBnFC57wjwH5mdRpbeXtjWCioXQCrt3S2-G2o_W5fMEsngx4IK1xRCqYgFZiEoqIqczWinvhS0xkR562HtH1ROUsCUEwAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FTriageReviewPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA8VYyXKjMBC95yt0SVVyUApw7CTkR_aWEqgBjYVEhPDC1Pz7FJLjsIONU3OyC1q9vtfq5unDpSX5TDcO_nOHUEDCbaxkISgOJZfKRzuiHjD_cN_g9FT58vheSUlFQWEOkfbRKjugXHJGW6KHuqgilBV5W9vKqssIpUzEnbeueZsSFTOBA6m1TAdkKMszTo4_ijgcqgfVL6ZMQaiZFD4KJS9SUb2JSdZR4jy_3_29e_pKhWtS0dH5u8g1i444lEKD0D7KMxICDkDvAYxuwlksMNOQ5j4KQWhQDcWeURxJoXHOSuj4EZpgzPs9sDjRHYnESPSWxqlly0dOw_BqoLhaEZFnRIHQg3q9Wh19JKSA6bpuxurqHR_bT8rvwHsTE9iwC5VX_mWS2dQiGwCzNW6dEc2irvxE7kCZRIzkb4oAr22tkQyLHO9YzgIORrssNGcCfOQNMCM1lk5iWEZRDtpIN1Q_96PwImR7x6a76wkAkiUAtEDRcNDYlCWSKvVRkWWgQpIb1HDQGhSuiGOA4Tw5a0iHcbuZ8DcY9sZt0wEhk_7kFJf7tGmYerEUsbDWMvORO1C_aIRor7coWlnve8aV8aq_zWrd6XCuPq_g_Gjr9so2xU_Neknvi4eawJ5RnfjIdZz7y3rCW60nTKWvc5SynAQcqOV8hWh9rBC9rnsppMaEc7kH2jr_Y32DLOj1bgMIQwSApTf7FfAI_gM8yAXweG0fXQQP8nPwCG7VoWo6w1ltekbxhjBZs2UT2qppysS5sb_YiMew17znO_bIiC_BLIJsZxNkVs4ikjJ_7ASyMxIKTsdBaRYS3kyWxVETP63_3hsobyYdZs0NI1PBVO1rtqLr73yve_dbPfrIwUdME87ChrHYGOMsPwt9Zaeh5YwlZ_m20SZP0k_Ixi5hDOSaKD0_M1w9KdX8YTb_xmKimNieoq7PIyeKDRahpu73WZ2P3J4pbN2Q3g4Yn4LeuUAVJXumpXkLyxSvoyno7ROmwYy3Bkl7RbLJwZPPvl_m1S9dvOtdOsI_VyN8zQNhPMjk1y0bsUN1v1X57d3hrVX7NWFdu6StdfOXEw2_HvDauTeyJWaCGkQ5Tj8lh9b8gSX9si8ftxiNAllBnFC57wjwH5mdRpbeXtjWCioXQCrt3S2_G2o_W5fMEsngx4IK1xRCqYgFZiEoqIqczWinvhS0xkR562HtH1ROUsCUEwAA_22_7D = __webpack_require__(68540);
;// ./src/components/backlog/TriageReviewPanel.css.ts


var TriageReviewPanel_css_actions = '_1dzaqm68';
var applyButton = '_1dzaqm69';
var TriageReviewPanel_css_dismissButton = '_1dzaqm63';
var divider = '_1dzaqm67';
var heading = '_1dzaqm62';
var iterationBadge = '_1dzaqm6e';
var noSuggestionsText = '_1dzaqm6f';
var panel = '_1dzaqm60';
var panelHeader = '_1dzaqm61';
var refineForm = '_1dzaqm6b';
var refineLabel = '_1dzaqm6c';
var refineTextarea = '_1dzaqm6d';
var sectionLabel = '_1dzaqm65';
var TriageReviewPanel_css_skipButton = '_1dzaqm6a';
var summarySection = '_1dzaqm64';
var summaryText = '_1dzaqm66';
var taskBadge = '_1dzaqm6k';
var taskBullet = '_1dzaqm6i';
var taskCategoryBadge = '_1dzaqm6m _1dzaqm6k';
var taskEstimateBadge = '_1dzaqm6l _1dzaqm6k';
var taskItem = '_1dzaqm6h';
var taskList = '_1dzaqm6g';
var taskText = '_1dzaqm6j';
var undoButton = '_1dzaqm6o';
var undoToastOverlay = '_1dzaqm6n';
;// ./src/components/backlog/TriageReviewPanel.tsx
/* __next_internal_client_entry_do_not_use__ TriageReviewPanel auto */ 








const DISMISSED_KEY = (id)=>"triage-panel-dismissed-".concat(id);
function isDismissed(itemId) {
    if (false) {}
    return Boolean(localStorage.getItem(DISMISSED_KEY(itemId)));
}
function setDismissed(itemId) {
    if (true) {
        localStorage.setItem(DISMISSED_KEY(itemId), "1");
    }
}
function isReadOnlyProps(props) {
    return props.readOnly === true;
}
/**
 * TriageReviewPanel — inline triage diff/review panel inside BacklogItemDetail.
 * Shows when triageStatus === "completed" AND item.status === "idea" AND not dismissed.
 *
 * Per UX spec Section 3.1 and Section 7.2.
 */ function TriageReviewPanel(props) {
    var _triageResult_tasks;
    const { item, triageResult } = props;
    const readOnly = isReadOnlyProps(props);
    const onApply = isReadOnlyProps(props) ? undefined : props.onApply;
    const onUndoApply = isReadOnlyProps(props) ? undefined : props.onUndoApply;
    const onSkip = isReadOnlyProps(props) ? undefined : props.onSkip;
    const onRefine = isReadOnlyProps(props) ? undefined : props.onRefine;
    const onAnswerQuestion = isReadOnlyProps(props) ? undefined : props.onAnswerQuestion;
    // readOnly panels ignore the interactive dismissed flag entirely — it's
    // keyed by item.id, which a readOnly historical-record render (Story
    // 4.1.2) shares with the live interactive panel for the same item, so
    // respecting it here would incorrectly hide a headless diagnostic
    // session's record just because the user once dismissed the live prompt.
    // A historical record shouldn't be dismissible in the first place, so
    // isDismissed/setDismissed become no-ops in this mode.
    const [dismissed, setDismissedState] = (0,react.useState)(()=>readOnly ? false : isDismissed(item.id));
    const [applyState, setApplyState] = (0,react.useState)("idle");
    const [applyError, setApplyError] = (0,react.useState)();
    const [showUndoToast, setShowUndoToast] = (0,react.useState)(false);
    const [preApplyCriteria, setPreApplyCriteria] = (0,react.useState)();
    const [isMounted, setIsMounted] = (0,react.useState)(false);
    const [showRefineForm, setShowRefineForm] = (0,react.useState)(false);
    const [refineFeedback, setRefineFeedback] = (0,react.useState)("");
    const [refineState, setRefineState] = (0,react.useState)("idle");
    const [refineError, setRefineError] = (0,react.useState)();
    (0,react.useEffect)(()=>{
        setIsMounted(true);
    }, []);
    // Re-check dismissed state when item changes (skipped in readOnly mode).
    (0,react.useEffect)(()=>{
        if (readOnly) return;
        setDismissedState(isDismissed(item.id));
    }, [
        item.id,
        readOnly
    ]);
    const handleDismiss = (0,react.useCallback)(()=>{
        if (readOnly || !onSkip) return;
        setDismissed(item.id);
        setDismissedState(true);
        onSkip();
    }, [
        item.id,
        onSkip,
        readOnly
    ]);
    const handleApply = (0,react.useCallback)(async ()=>{
        if (!onApply) return;
        // Cache pre-apply criteria for undo
        const cached = [
            ...item.acCriteria
        ];
        setPreApplyCriteria(cached);
        setApplyState("applying");
        setApplyError(undefined);
        try {
            await onApply(cached);
            setDismissed(item.id);
            setDismissedState(true);
            setShowUndoToast(true);
            // Auto-dismiss undo toast after 7s
            setTimeout(()=>setShowUndoToast(false), 7000);
        } catch (err) {
            setApplyError((0,connectError/* getErrorMessage */.u)(err, "Failed to apply suggestions. The item may have been updated by another process. Reload and try again."));
            setApplyState("error");
        }
    }, [
        item.acCriteria,
        item.id,
        onApply
    ]);
    const handleRefineSubmit = (0,react.useCallback)(async ()=>{
        const feedback = refineFeedback.trim();
        if (!feedback || !onRefine) return;
        setRefineState("submitting");
        setRefineError(undefined);
        try {
            await onRefine(feedback);
            setShowRefineForm(false);
            setRefineFeedback("");
            setRefineState("idle");
        } catch (err) {
            setRefineError((0,connectError/* getErrorMessage */.u)(err, "Failed to submit feedback. Please try again."));
            setRefineState("error");
        }
    }, [
        refineFeedback,
        onRefine
    ]);
    // Undo toast rendered via portal so it appears at the bottom of the viewport.
    // Built BEFORE the dismissed guard so the toast persists after the panel hides itself.
    const undoToast = showUndoToast && preApplyCriteria && isMounted ? /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: undoToastOverlay,
        role: "status",
        "data-testid": "triage-undo-toast",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                children: "Triage applied — item is now ready."
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                className: undoButton,
                onClick: ()=>{
                    setShowUndoToast(false);
                    if (onUndoApply && preApplyCriteria) {
                        void onUndoApply(preApplyCriteria);
                    }
                },
                "data-testid": "triage-undo-button",
                children: "Undo"
            })
        ]
    }), document.body) : null;
    if (dismissed) return undoToast;
    const acSuggestions = triageResult.suggestions.filter((s)=>s.rationale !== "question");
    const hasSuggestions = acSuggestions.length > 0;
    var _triageResult_tasks_length;
    const hasTasks = ((_triageResult_tasks_length = (_triageResult_tasks = triageResult.tasks) === null || _triageResult_tasks === void 0 ? void 0 : _triageResult_tasks.length) !== null && _triageResult_tasks_length !== void 0 ? _triageResult_tasks_length : 0) > 0;
    const isApplying = applyState === "applying";
    const isRefining = refineState === "submitting";
    var _triageResult_iteration, _triageResult_tasks1;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
                className: panel,
                "aria-live": "polite",
                "data-testid": "triage-review-panel",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: panelHeader,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("h3", {
                                className: heading,
                                children: [
                                    "Triage Ready",
                                    ((_triageResult_iteration = triageResult.iteration) !== null && _triageResult_iteration !== void 0 ? _triageResult_iteration : 1) > 1 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                        className: iterationBadge,
                                        children: [
                                            " \xb7 Iteration ",
                                            triageResult.iteration
                                        ]
                                    })
                                ]
                            }),
                            !readOnly && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: TriageReviewPanel_css_dismissButton,
                                onClick: handleDismiss,
                                "aria-label": "Dismiss triage review",
                                "data-testid": "triage-dismiss-button",
                                disabled: isApplying,
                                children: "Skip \xd7"
                            })
                        ]
                    }),
                    applyState === "error" && applyError && /*#__PURE__*/ (0,jsx_runtime.jsx)(TriageErrorBanner, {
                        message: applyError,
                        onReload: ()=>{
                            setApplyState("idle");
                            setApplyError(undefined);
                        },
                        onSkip: handleDismiss
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: summarySection,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                className: sectionLabel,
                                children: "Summary"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                className: summaryText,
                                children: triageResult.summary
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(GuidanceRequestPanel/* GuidanceRequestPanel */.w, {
                        scope: "backlog-item",
                        scopeKey: item.id,
                        title: "Triage questions"
                    }),
                    !readOnly && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("hr", {
                                className: divider,
                                "aria-hidden": "true"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(TriageRelatedWorkSection, {
                                itemTitle: item.title,
                                repoPath: item.repoPath
                            })
                        ]
                    }),
                    hasSuggestions && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("hr", {
                                className: divider,
                                "aria-hidden": "true"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                        className: sectionLabel,
                                        children: "Suggested Acceptance Criteria"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(TriageDiffSection, {
                                        currentCriteria: item.acCriteria,
                                        suggestedSuggestions: triageResult.suggestions,
                                        onAnswerQuestion: onAnswerQuestion
                                    })
                                ]
                            })
                        ]
                    }),
                    !hasSuggestions && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: noSuggestionsText,
                        children: "No AC changes suggested. You can mark this item ready manually."
                    }),
                    hasTasks && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("hr", {
                                className: divider,
                                "aria-hidden": "true"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                        className: sectionLabel,
                                        children: "Implementation plan"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                                        className: taskList,
                                        "data-testid": "triage-task-list",
                                        children: ((_triageResult_tasks1 = triageResult.tasks) !== null && _triageResult_tasks1 !== void 0 ? _triageResult_tasks1 : []).map((task, i)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                                                className: taskItem,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: taskBullet,
                                                        "aria-hidden": "true",
                                                        children: "•"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: taskText,
                                                        children: task.text
                                                    }),
                                                    task.estimate && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: taskEstimateBadge,
                                                        children: task.estimate
                                                    }),
                                                    task.category && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: taskCategoryBadge,
                                                        children: task.category
                                                    })
                                                ]
                                            }, i))
                                    })
                                ]
                            })
                        ]
                    }),
                    !readOnly && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: TriageReviewPanel_css_actions,
                        children: [
                            hasSuggestions ? /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: applyButton,
                                onClick: handleApply,
                                disabled: isApplying,
                                "aria-label": "Apply triage suggestions — replaces acceptance criteria and marks item ready",
                                "aria-busy": isApplying,
                                "data-testid": "triage-apply-button",
                                children: isApplying ? "Applying…" : "Apply suggestions"
                            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: applyButton,
                                onClick: handleApply,
                                disabled: isApplying,
                                "aria-busy": isApplying,
                                "data-testid": "triage-mark-ready-button",
                                children: isApplying ? "Applying…" : "Mark ready"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: TriageReviewPanel_css_skipButton,
                                onClick: handleDismiss,
                                disabled: isApplying,
                                "data-testid": "triage-skip-button",
                                children: "Skip — review later"
                            }),
                            onRefine && !showRefineForm && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: TriageReviewPanel_css_skipButton,
                                onClick: ()=>setShowRefineForm(true),
                                disabled: isApplying,
                                "data-testid": "triage-refine-toggle-button",
                                children: "Not quite — give feedback"
                            })
                        ]
                    }),
                    !readOnly && onRefine && showRefineForm && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: refineForm,
                        role: "form",
                        "aria-label": "Refine triage with feedback",
                        children: [
                            refineState === "error" && refineError && /*#__PURE__*/ (0,jsx_runtime.jsx)(TriageErrorBanner, {
                                message: refineError,
                                onReload: ()=>{
                                    setRefineState("idle");
                                    setRefineError(undefined);
                                },
                                onSkip: ()=>setShowRefineForm(false)
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                htmlFor: "triage-refine-feedback",
                                className: refineLabel,
                                children: "What should change?"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("textarea", {
                                id: "triage-refine-feedback",
                                rows: 3,
                                placeholder: "e.g. missed the mobile case, re-check the auth approach",
                                value: refineFeedback,
                                onChange: (e)=>setRefineFeedback(e.target.value),
                                className: refineTextarea,
                                "data-testid": "triage-refine-textarea",
                                disabled: isRefining
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: TriageReviewPanel_css_actions,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        type: "button",
                                        className: applyButton,
                                        onClick: ()=>void handleRefineSubmit(),
                                        disabled: isRefining || !refineFeedback.trim(),
                                        "aria-busy": isRefining,
                                        "data-testid": "triage-refine-submit-button",
                                        children: isRefining ? "Refining…" : "Refine triage"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        type: "button",
                                        className: TriageReviewPanel_css_skipButton,
                                        onClick: ()=>{
                                            setShowRefineForm(false);
                                            setRefineFeedback("");
                                        },
                                        disabled: isRefining,
                                        "data-testid": "triage-refine-cancel-button",
                                        children: "Cancel"
                                    })
                                ]
                            })
                        ]
                    })
                ]
            }),
            undoToast
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FChatRefinementPanel.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA62UwY6bMBCG73kKH5uDI0y27K73YSpjD8Ra46G2WRKqvnuFqVQ3kICinJDsn39mvhnPQb0EMEVGfu0IKYX8rB12VlGJBh0nX8J9o_QHe4chd8Pr_mNUoVPgqIEqcHJsz8Sj0epK2qRSJ5Tu_LXbcbJrhVLa1rNbFm8b4WptaYkhYHNDo7RvjbhwUhk4jwfjlyrtQAaNlhOJpmvseFOLdmaS7T92v3eHCQSLICq0gXo9wEwsY8R434OuT2GmOEXFIr8sKYmTLAmbb-J_2QC1uAs1ewKwfEiBHVeAlY_geFkxFbdN8zS579HnCeWOaZ7p6W_HWeGgGQ_xC1xlsKcXTkQXMAldTB19vFf5sF_u3hrr_qQDUN8KCZy0DmjvRJtk9rpp1t6ilzC6ttSDqSZ6FKxKrN42WRXJ2HLCbmwMWA7og3AhCfm-3NLVORXTSBk4c8Iewb6llWs1fk6qFV7sznizcsMM_CuaVyg7n47ioquJptgFoy1wYtFC4lJu6nJzO-mf_8GZ3J_9Ntimt3FvcddTAZ3zYwUtahvAjUfBCev1tCqu_rEp7ZIr7UVpQEVg2Aqpw4WT7FCkzhYDFcZgD-lTko_vUZbPFukfgHexz9cHAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FChatRefinementPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA62UwY6bMBCG73kKH5uDI0y27K73YSpjD8Ra46G2WRKqvnuFqVQ3kICinJDsn39mvhnPQb0EMEVGfu0IKYX8rB12VlGJBh0nX8J9o_QHe4chd8Pr_mNUoVPgqIEqcHJsz8Sj0epK2qRSJ5Tu_LXbcbJrhVLa1rNbFm8b4WptaYkhYHNDo7RvjbhwUhk4jwfjlyrtQAaNlhOJpmvseFOLdmaS7T92v3eHCQSLICq0gXo9wEwsY8R434OuT2GmOEXFIr8sKYmTLAmbb_J_2QC1uAs1ewKwfEiBHVeAlY_geFkxFbdN8zS579HnCeWOaZ7p6W_HWeGgGQ_xC1xlsKcXTkQXMAldTB19vFf5sF_u3hrr_qQDUN8KCZy0DmjvRJtk9rpp1t6ilzC6ttSDqSZ6FKxKrN42WRXJ2HLCbmwMWA7og3AhCfm_3NLVORXTSBk4c8Iewb6llWs1fk6qFV7sznizcsMM_CuaVyg7n47ioquJptgFoy1wYtFC4lJu6nJzO_mf_8GZ3J_9Ntimt3FvcddTAZ3zYwUtahvAjUfBCev1tCqu_rEp7ZIr7UVpQEVg2Aqpw4WT7FCkzhYDFcZgD_lTko_vUZbPFukfgHexz9cHAAA_22_7D = __webpack_require__(61201);
;// ./src/components/backlog/ChatRefinementPanel.css.ts


var errorText = 'd4tel6c';
var ChatRefinementPanel_css_heading = 'd4tel61';
var ChatRefinementPanel_css_input = 'd4tel6a';
var inputRow = 'd4tel69';
var ChatRefinementPanel_css_panel = 'd4tel60';
var questionBanner = 'd4tel62';
var questionMeta = 'd4tel64';
var questionText = 'd4tel63';
var sendButton = 'd4tel6b';
var ChatRefinementPanel_css_transcript = 'd4tel65';
var turn = 'd4tel66';
var turnAssistant = 'd4tel68 d4tel66';
var turnUser = 'd4tel67 d4tel66';
;// ./src/components/backlog/ChatRefinementPanel.tsx
// +feature: backlog:chat-refinement-panel




/**
 * Additive chat surface for multi-turn backlog item refinement — sits
 * alongside TriageReviewPanel's structured refine-feedback form on the
 * Backlog page's item detail view, without modifying that panel.
 */ function ChatRefinementPanel(param) {
    let { clarifyingQuestions, onSend } = param;
    const [transcript, setTranscript] = (0,react.useState)([]);
    const [input, setInput] = (0,react.useState)("");
    const [sending, setSending] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    const currentQuestion = clarifyingQuestions.length > 0 ? clarifyingQuestions[0] : null;
    const handleSend = (0,react.useCallback)(async ()=>{
        const message = input.trim();
        if (!message || sending) return;
        setSending(true);
        setError(null);
        const answeringQuestion = currentQuestion;
        setTranscript((prev)=>[
                ...prev,
                {
                    role: "user",
                    text: message
                }
            ]);
        setInput("");
        try {
            await onSend(message);
            setTranscript((prev)=>[
                    ...prev,
                    {
                        role: "assistant",
                        text: answeringQuestion ? "Thanks — re-running triage with your answer. This item will update once it's done." : "Got it — re-running triage with your message. This item will update once it's done."
                    }
                ]);
        } catch (err) {
            setError((0,connectError/* getErrorMessage */.u)(err, "Failed to send message"));
        } finally{
            setSending(false);
        }
    }, [
        input,
        sending,
        currentQuestion,
        onSend
    ]);
    const handleKeyDown = (0,react.useCallback)((e)=>{
        if (e.key === "Enter" && !e.shiftKey) {
            e.preventDefault();
            void handleSend();
        }
    }, [
        handleSend
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: ChatRefinementPanel_css_panel,
        "data-testid": "chat-refinement-panel",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("h4", {
                className: ChatRefinementPanel_css_heading,
                children: "Refine via chat"
            }),
            currentQuestion && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: questionBanner,
                "data-testid": "chat-refinement-question",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: questionMeta,
                        children: clarifyingQuestions.length > 1 ? "Clarifying question (".concat(clarifyingQuestions.length, " pending)") : "Clarifying question"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: questionText,
                        children: currentQuestion
                    })
                ]
            }),
            transcript.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: ChatRefinementPanel_css_transcript,
                "data-testid": "chat-refinement-transcript",
                children: transcript.map((turn, i)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: turn.role === "user" ? turnUser : turnAssistant,
                        "data-testid": "chat-turn-".concat(turn.role),
                        children: turn.text
                    }, i))
            }),
            error && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: errorText,
                children: error
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: inputRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        className: ChatRefinementPanel_css_input,
                        type: "text",
                        value: input,
                        onChange: (e)=>setInput(e.target.value),
                        onKeyDown: handleKeyDown,
                        placeholder: currentQuestion ? "Answer the question above…" : "Ask for a change, or add more detail…",
                        disabled: sending,
                        "aria-label": "Chat refinement message",
                        "data-testid": "chat-refinement-input"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: sendButton,
                        onClick: ()=>void handleSend(),
                        disabled: sending || !input.trim(),
                        "data-testid": "chat-refinement-send",
                        children: sending ? "Sending…" : "Send"
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/components/shared/DiffRenderer.tsx + 1 modules
var DiffRenderer = __webpack_require__(78516);
// EXTERNAL MODULE: ./src/lib/hooks/useFocusTrap.ts
var useFocusTrap = __webpack_require__(63991);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FReviewChangesModal.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA51UwZKiMBC9-xV91KplCnDWBeYyv7GnqZg0kjEkqSQquuW_byWIIoq6Wx6k85rXL92v-VzjvjSkRgtfSZpmpK5i-DMBKI2qwwOA0oRyty8g_pgAHCcATg2hpIWOk89bwuQxIYAzRNpSmboAS4nAafyWL2btsSAOf08z3cyeFL9Dk1xxxLNO41unLA1MWlnuuJIFlLxB5rO4tOhO-paErldGbSQrwKyWZBr_CL-3RSA8RFwybArI89zHRPKatHSXjiZpXFtAYvGjX38-Vt8pXcB8W_lngaXzwc4Hhq-qS7RUzqn6nHmWksRxkM641YLsCygFNv7A_0eMG6RtQarEppbDW26JmUbRV5LjITWHfNaWMgxNAYluwCrB2SDru5cVGcL4xg6J5tkpp4lsRZja3STUIUFt0ZTCwxVnDOVIVxNI3u929T109ebuRPCVjLjD2hZAUTo0_nhF9FBHegg6NGGMy9UddKg77l--G8pYp8rZeRK2Mlyug896-n-2-6Kkiyw_4E2Xli2Bx3fY-mGQUYUMqoQyQywebbHDxkUXAIXg2nLroV3FHUZWE4oFSLUzRHd3CNvXE794Ip6MS0tnj41IXzJi-YIRF9fz9VR3HfBgSL_-_56ty0O7GVJlTr6WSuKzokXlBxRK37y_kQyN4PJ6GbJ_WobvjXW83EdUSYfS9aEdZ64qIM10IKhOzuviVxr-4uh6Bgifb00MSvfENnRjrMe04p3i8eHQZ9PNeo0eN2T2eM16fHnrlm5f7i_gy1_r419WOYjMtgcAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FReviewChangesModal_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA51UwZKiMBC9_xV91KplCnDWBeYyv7GnqZg0kjEkqSQquuW_byWIIoq6Wx6k85rXL92v_VzjvjSkRgtfSZpmpK5i_DMBKI2qwwOA0oRyty8g_pgAHCcATg2hpIWOk89bwuQxIYAzRNpSmboAS4nAafyWL2btsSAOf08z3cyeFL9Dk1xxxLNO41unLA1MWlnuuJIFlLxB5rO4tOhO_paErldGbSQrwKyWZBr_CL_3RSA8RFwybArI89zHRPKatHSXjiZpXFtAYvGjX38_Vt8pXcB8W_lngaXzwc4Hhq_qS7RUzqn6nHmWksRxkM641YLsCygFNv7A_0eMG6RtQarEppbDW26JmUbRV5LjITWHfNaWMgxNAYluwCrB2SDru5cVGcL4xg6J5tkpp4lsRZja3STUIUFt0ZTCwxVnDOVIVxNI3u929T109ebuRPCVjLjD2hZAUTo0_nhF9FBHegg6NGGMy9UddKg77l_G8pYp8rZeRK2Mlyug896_n_2_6Kkiyw_4E2Xli2Bx3fY_mGQUYUMqoQyQywebbHDxkUXAIXg2nLroV3FHUZWE4oFSLUzRHd3CNvXE794Ip6MS0tnj41IXzJi_YIRF9fz9VR3HfBgSL_56ty0O7GVJlTr6WSuKzokXlBxRK37y_kQyN4PJ6GbJ_WobvjXW83EdUSYfS9aEdZ64qIM10IKhOzuviVxr_4uh6Bgifb00MSvfENnRjrMe04p3i8eHQZ9PNeo0eN2T2eM16fHnrlm5f7i_gy1_r419WOYjMtgcAAA_22_7D = __webpack_require__(10307);
;// ./src/components/backlog/ReviewChangesModal.css.ts

var backdrop = '_1228amh2';
var ReviewChangesModal_css_closeButton = '_1228amh8';
var modal = '_1228amh3';
var modalBody = '_1228amh9';
var modalHeader = '_1228amh4';
var modalLabel = '_1228amh6';
var modalTitle = '_1228amh5';
var openTerminalLink = '_1228amh7';
;// ./src/components/backlog/ReviewChangesModal.tsx
/* __next_internal_client_entry_do_not_use__ ReviewChangesModal auto */ 
// +feature: backlog:review-changes-modal










function ReviewChangesModal(param) {
    let { itemId, sessionId, sessionTitle, onClose, triggerRef } = param;
    const modalRef = (0,react.useRef)(null);
    (0,useFocusTrap/* useFocusTrap */.r)(modalRef, true, triggerRef);
    const [diff, setDiff] = (0,react.useState)(null);
    const [loading, setLoading] = (0,react.useState)(true);
    // Distinct from a genuinely empty diff — a fetch failure must not render as
    // "No changes to display", which looks identical to real emptiness and hides
    // exactly the case a reviewer most needs to notice (see
    // docs/tasks/backlog-feature-improvement.md, Manual Gates section).
    const [fetchError, setFetchError] = (0,react.useState)(null);
    const fetchDiff = (0,react.useCallback)(()=>{
        setLoading(true);
        setFetchError(null);
        const transport = (0,connect_transport/* createConnectTransport */.w)({
            baseUrl: (0,config/* getApiBaseUrl */.h)(),
            interceptors: [
                (0,config/* createAuthInterceptor */._)()
            ]
        });
        const client = (0,promise_client/* createClient */.UU)(backlog_pb/* BacklogService */.jSM, transport);
        client.getBacklogItemDiff({
            itemId
        }).then((resp)=>{
            setDiff({
                content: resp.diff,
                added: resp.added,
                removed: resp.removed
            });
        }).catch((err)=>{
            setFetchError((0,connectError/* getErrorMessage */.u)(err, "Could not reach the server."));
        }).finally(()=>setLoading(false));
    }, [
        itemId
    ]);
    (0,react.useEffect)(()=>{
        fetchDiff();
    }, [
        fetchDiff
    ]);
    (0,react.useEffect)(()=>{
        function handleKeyDown(e) {
            if (e.key === "Escape") {
                e.stopPropagation();
                onClose();
            }
        }
        window.addEventListener("keydown", handleKeyDown, {
            capture: true
        });
        return ()=>window.removeEventListener("keydown", handleKeyDown, {
                capture: true
            });
    }, [
        onClose
    ]);
    if (typeof document === "undefined") return null;
    var _diff_content, _diff_added, _diff_removed;
    return /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: backdrop,
                onClick: onClose,
                "aria-hidden": "true"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                ref: modalRef,
                className: modal,
                role: "dialog",
                "aria-modal": "true",
                "aria-labelledby": "review-changes-title",
                "data-testid": "review-changes-modal",
                tabIndex: -1,
                onClick: (e)=>e.stopPropagation(),
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: modalHeader,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                id: "review-changes-title",
                                className: modalTitle,
                                children: sessionTitle !== null && sessionTitle !== void 0 ? sessionTitle : itemId
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: modalLabel,
                                children: "Changes"
                            }),
                            sessionId && /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                                className: openTerminalLink,
                                href: "/?session=".concat(sessionId),
                                title: "Open session in terminal",
                                children: "Open in Terminal ↗"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: ReviewChangesModal_css_closeButton,
                                onClick: onClose,
                                "aria-label": "Close changes viewer",
                                children: "✕"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: modalBody,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(DiffRenderer/* DiffRenderer */.F, {
                            content: (_diff_content = diff === null || diff === void 0 ? void 0 : diff.content) !== null && _diff_content !== void 0 ? _diff_content : "",
                            added: (_diff_added = diff === null || diff === void 0 ? void 0 : diff.added) !== null && _diff_added !== void 0 ? _diff_added : 0,
                            removed: (_diff_removed = diff === null || diff === void 0 ? void 0 : diff.removed) !== null && _diff_removed !== void 0 ? _diff_removed : 0,
                            loading: loading,
                            error: fetchError,
                            onRefresh: fetchDiff
                        })
                    })
                ]
            })
        ]
    }), document.body);
}

// EXTERNAL MODULE: ./src/components/sessions/FileTree.tsx + 1 modules
var FileTree = __webpack_require__(23471);
// EXTERNAL MODULE: ./src/components/sessions/FileContentViewer.tsx + 6 modules
var FileContentViewer = __webpack_require__(40415);
// EXTERNAL MODULE: ./src/lib/hooks/useSessionVcs.ts
var useSessionVcs = __webpack_require__(60976);
// EXTERNAL MODULE: ./src/lib/utils/gitStatus.ts
var gitStatus = __webpack_require__(41852);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FBacklogFileBrowserModal.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll83eHlxeHowIHsKICBmbGV4OiAxOwogIG92ZXJmbG93OiBoaWRkZW47CiAgZGlzcGxheTogZmxleDsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejFkKTsKfQouXzd4eXF4ejEgewogIHdpZHRoOiAyODBweDsKICBmbGV4LXNocmluazogMDsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXgtZGlyZWN0aW9uOiBjb2x1bW47CiAgb3ZlcmZsb3c6IGhpZGRlbjsKICBib3JkZXItcmlnaHQ6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemYpOwp9Ci5fN3h5cXh6MiB7CiAgZmxleDogMTsKICBvdmVyZmxvdzogaGlkZGVuOwogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBtaW4td2lkdGg6IDA7Cn0KQG1lZGlhIChtYXgtd2lkdGg6IDc2OHB4KSB7CiAgLl83eHlxeHoxIHsKICAgIHdpZHRoOiAxNjBweDsKICB9Cn0%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FBacklogFileBrowserModal_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll83eHlxeHowIHsKICBmbGV4OiAxOwogIG92ZXJmbG93OiBoaWRkZW47CiAgZGlzcGxheTogZmxleDsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejFkKTsKfQouXzd4eXF4ejEgewogIHdpZHRoOiAyODBweDsKICBmbGV4LXNocmluazogMDsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXgtZGlyZWN0aW9uOiBjb2x1bW47CiAgb3ZlcmZsb3c6IGhpZGRlbjsKICBib3JkZXItcmlnaHQ6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemYpOwp9Ci5fN3h5cXh6MiB7CiAgZmxleDogMTsKICBvdmVyZmxvdzogaGlkZGVuOwogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBtaW4td2lkdGg6IDA7Cn0KQG1lZGlhIChtYXgtd2lkdGg6IDc2OHB4KSB7CiAgLl83eHlxeHoxIHsKICAgIHdpZHRoOiAxNjBweDsKICB9Cn0_22_7D = __webpack_require__(97516);
;// ./src/components/backlog/BacklogFileBrowserModal.css.ts


var BacklogFileBrowserModal_css_backdrop = '_1228amh2';
var BacklogFileBrowserModal_css_closeButton = '_1228amh8';
var contentPane = '_7xyqxz2';
var BacklogFileBrowserModal_css_modal = '_1228amh3';
var BacklogFileBrowserModal_css_modalBody = '_7xyqxz0';
var BacklogFileBrowserModal_css_modalHeader = '_1228amh4';
var BacklogFileBrowserModal_css_modalLabel = '_1228amh6';
var BacklogFileBrowserModal_css_modalTitle = '_1228amh5';
var BacklogFileBrowserModal_css_openTerminalLink = '_1228amh7';
var treePane = '_7xyqxz1';
;// ./src/components/backlog/BacklogFileBrowserModal.tsx
/* __next_internal_client_entry_do_not_use__ BacklogFileBrowserModal auto */ 
// +feature: backlog:file-browser-modal









/**
 * Lets a user click through a backlog work session's worktree files and
 * directories in-app, rather than only copying the worktree path. Embeds
 * FileTree + FileContentViewer directly (not FilesTab) since those two have
 * no dependency on SessionVcsContext/session tab-switching state — see
 * FileTree.tsx / FileContentViewer.tsx prop signatures.
 */ function BacklogFileBrowserModal(param) {
    let { sessionId, sessionTitle, onClose, triggerRef } = param;
    const modalRef = (0,react.useRef)(null);
    (0,useFocusTrap/* useFocusTrap */.r)(modalRef, true, triggerRef);
    const [selectedPath, setSelectedPath] = (0,react.useState)(null);
    const baseUrl = (0,config/* getApiBaseUrl */.h)();
    // Own lightweight VCS status fetch — this modal has no SessionVcsProvider ancestor.
    const { status } = (0,useSessionVcs/* useSessionVcs */.u)(sessionId, baseUrl);
    const gitStatusMap = (0,react.useMemo)(()=>{
        if (!status) return new Map();
        const { stagedFiles, unstagedFiles, untrackedFiles } = status;
        return (0,gitStatus/* buildGitStatusMap */.lT)([
            ...stagedFiles,
            ...unstagedFiles,
            ...untrackedFiles
        ]);
    }, [
        status
    ]);
    (0,react.useEffect)(()=>{
        function handleKeyDown(e) {
            if (e.key === "Escape") {
                e.stopPropagation();
                onClose();
            }
        }
        window.addEventListener("keydown", handleKeyDown, {
            capture: true
        });
        return ()=>window.removeEventListener("keydown", handleKeyDown, {
                capture: true
            });
    }, [
        onClose
    ]);
    if (typeof document === "undefined") return null;
    return /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: BacklogFileBrowserModal_css_backdrop,
                onClick: onClose,
                "aria-hidden": "true"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                ref: modalRef,
                className: BacklogFileBrowserModal_css_modal,
                role: "dialog",
                "aria-modal": "true",
                "aria-labelledby": "file-browser-title",
                "data-testid": "file-browser-modal",
                tabIndex: -1,
                onClick: (e)=>e.stopPropagation(),
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: BacklogFileBrowserModal_css_modalHeader,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                id: "file-browser-title",
                                className: BacklogFileBrowserModal_css_modalTitle,
                                children: sessionTitle !== null && sessionTitle !== void 0 ? sessionTitle : sessionId
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: BacklogFileBrowserModal_css_modalLabel,
                                children: "Files"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                                className: BacklogFileBrowserModal_css_openTerminalLink,
                                href: "/?session=".concat(sessionId),
                                title: "Open session in terminal",
                                children: "Open in Terminal ↗"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: BacklogFileBrowserModal_css_closeButton,
                                onClick: onClose,
                                "aria-label": "Close file browser",
                                children: "✕"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: BacklogFileBrowserModal_css_modalBody,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: treePane,
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(FileTree/* FileTree */.hr, {
                                    sessionId: sessionId,
                                    baseUrl: baseUrl,
                                    selectedPath: selectedPath,
                                    onFileSelect: setSelectedPath,
                                    gitStatusMap: gitStatusMap
                                })
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: contentPane,
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(FileContentViewer/* FileContentViewer */.Ol, {
                                    sessionId: sessionId,
                                    filePath: selectedPath,
                                    baseUrl: baseUrl
                                })
                            })
                        ]
                    })
                ]
            })
        ]
    }), document.body);
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2FmodalChrome.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA9VVTW-jMBC951fMpVKQ1pGhbZqQS__GnlYONuAWbNZ2UpJV__sKG1JwgGSl3cNKETH-ePPmzfPw-s5OqSIl01BvxbqkGH4tAFIlSzsAkBVJuDnFgHcLgM8FgJH-UuiWPhevPlzowxlFhE6lKmM3LIhh35cRrupgdyve-FnsH7ywWTkOkT1eSc0NlyKGlNeM7ixw1YYpWGraoeJZ3o330hhZdi8kec-UPAgaw5GoJUI_wi07R-pMLAPKdVWQUwxpwepmghQ8E4gbVuoYEiYMU83020Ebnp5QIoVhwvSXzogLyuoYQoxtTCJ4SRzrrjx4FWlgRLPdV4aPNsNpgtvAZaMoU0gRyg86hjCqLM2KUMpF1mZZkhp9cGryGJ6bqjRz7fsWP3Q7cuZU2uBjPpp7848oVyxx7BNZHErhWNRI54TKjxgwNCFg3TxUtidL_A3a3-oxGBUgbJauBHhyJe4yiZ6q2j3CdTvqCdBVNaxq0LLg1FPrLeghP1vkkqiMi4YwhgbWSpVKYZDmZxZDp5Sd-mjFWbsaJrKQyq8I7sdYD2N42GHLfhQn7OO8TBmdC83MlY3RKOK_M_OlPMOAj4McNp6Vx0n2DT1fyCvbe8E3gW9Kb0MZzHGPAu_SPG2GlyYhRbIMMX4ABJFi5bi609clI9W8YFvXYb_84m2mwZUxvR15MGvTgTMvYYkNO-nJOUaJDwpQcMEuTSVcPfcC7W2gK8XGdHFsryxpxWWCfkVFtvMPT0fnvqrJXTYsp5X7OXCokILd9uLLnNWiczDmg9tCz5U-cwkclG4yqCTvLq_9vrZtxDsjBjrFuTwydZda3sFUJgeNjlzzfcEsgDyYxggxRBMX2undbkMyTW1bs9-xCzCd4GIzqohiwsx209tthd3RVv7DUtI_KOWmL8HoDj6E_svF_g3PZClKtwoAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2FmodalChrome_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9VVTW_jMBC951fMpVKQ1pGhbZqQS_GnlYONuAWbNZ2UpJV_sKG1JwgGSl3cNKETH_ePPmzfPw_s5OqSIl01BvxbqkGH4tAFIlSzsAkBVJuDnFgHcLgM8FgJH_UuiWPhevPlzowxlFhE6lKmM3LIhh35cRrupgdyve_FnsH7ywWTkOkT1eSc0NlyKGlNeM7ixw1YYpWGraoeJZ3o330hhZdi8kec_UPAgaw5GoJUI_wi07R_pMLAPKdVWQUwxpwepmghQ8E4gbVuoYEiYMU83020Ebnp5QIoVhwvSXzogLyuoYQoxtTCJ4SRzrrjx4FWlgRLPdV4aPNsNpgtvAZaMoU0gRyg86hjCqLM2KUMpF1mZZkhp9cGryGJ6bqjRz7fsWP3Q7cuZU2uBjPpp7848oVyxx7BNZHErhWNRI54TKjxgwNCFg3TxUtidL_A3a3_oxGBUgbJauBHhyJe4yiZ6q2j3CdTvqCdBVNaxq0LLg1FPrLeghP1vkkqiMi4YwhgbWSpVKYZDmZxZDp5Sd_mjFWbsaJrKQyq8I7sdYD2N42GHLfhQn7OO8TBmdC83MlY3RKOK_M_OlPMOAj4McNp6Vx0n2DT1fyCvbe8E3gW9Kb0MZzHGPAu_SPG2GlyYhRbIMMX4ABJFi5bi609clI9W8YFvXYb_84m2mwZUxvR15MGvTgTMvYYkNO_nJOUaJDwpQcMEuTSVcPfcC7W2gK8XGdHFsryxpxWWCfkVFtvMPT0fnvqrJXTYsp5X7OXCokILd9uLLnNWiczDmg9tCz5U_cwkclG4yqCTvLq_9vrZtxDsjBjrFuTwydZda3sFUJgeNjlzzfcEsgDyYxggxRBMX2undbkMyTW1bs9_xCzCd4GIzqohiwsx209tthd3RVv7DUtI_KOWmL8HoDj6E_svF_g3PZClKtwoAAA_22_7D = __webpack_require__(6579);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FJulesDispatchDialog.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61U0W7jIBB8z1fwclLzQAVukzT0Y04EsL0tBgtwYvt0_34ytlIrsWNXvack7GRnd2bgme-M4K8E_dkgVFoPAaxhKIVayfcNQmC8CgyR7vuJi8_M2cpILKy2jqEzd08Y_6ZH1Sau5dsOJcGXmjcMpVrV3QHXkBkMQRWeIaFMUK47_qh8gLTBwpqgTBiXWgxGqpohSkhkLrmUYLJbwhe6fd_83Tz3O9C4w9KQxzjkyTqpHEO0rJG3GuQN6mOEwo5LqPwd-duAqbHPubSXO0CxfTR7EqsFr_EFZMgZ2iWkjIINvwXX4okS8gthlDhVXPG5giwPDB3JOe_O7Fm5VNsLbhjiVbCTLnSfWIJTondYWF0VpqtkvHwsbBKFTa0J2EOr7sAyThbrl2G0G0QeEZN-kGEtl4GJObvSvkTany2StPPMfWkpL82qvNQr8nJ4GAfyJeKkyKexI6_T0szctDldru12_0vpa8d97DgpKF2x6HKarkyH_uWaVjVpt9M6L7lO-SrbP1fYvp9PID19x_QDS62ofNzXVkGDUQwZa9RohkkaPe7yFv_v1MCnXADBdbyDYL6eln3_FMXRUl6AbhgCkysHYdTsOG9zsrAan733_EF2knXu0VXuUbLavu_mayne473_Ad4w2VmDBwAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FJulesDispatchDialog_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61U0W7jIBB8z1fwclLzQAVukzT0Y04EsL0tBgtwYvt0_34ytlIrsWNXvack7GRnd2bgme_M4K8E_dkgVFoPAaxhKIVayfcNQmC8CgyR7vuJi8_M2cpILKy2jqEzd08Y_6ZH1Sau5dsOJcGXmjcMpVrV3QHXkBkMQRWeIaFMUK47_qh8gLTBwpqgTBiXWgxGqpohSkhkLrmUYLJbwhe6fd_83Tz3O9C4w9KQxzjkyTqpHEO0rJG3GuQN6mOEwo5LqPwd_duAqbHPubSXO0CxfTR7EqsFr_EFZMgZ2iWkjIINvwXX4okS8gthlDhVXPG5giwPDB3JOe_O7Fm5VNsLbhjiVbCTLnSfWIJTondYWF0VpqtkvHwsbBKFTa0J2EOr7sAyThbrl2G0G0QeEZN_kGEtl4GJObvSvkTany2StPPMfWkpL82qvNQr8nJ4GAfyJeKkyKexI6_T0szctDldru12_0vpa8d97DgpKF2x6HKarkyH_uWaVjVpt9M6L7lO_SrbP1fYvp9PID19x_QDS62ofNzXVkGDUQwZa9RohkkaPe7yFv_v1MCnXADBdbyDYL6eln3_FMXRUl6AbhgCkysHYdTsOG9zsrAan733_EF2knXu0VXuUbLavu_mayne473_Ad4w2VmDBwAA_22_7D = __webpack_require__(5365);
;// ./src/components/backlog/JulesDispatchDialog.css.ts



var JulesDispatchDialog_css_actions = 'x9n6mdb';
var dialog = 'a5nca41';
var egressBlock = 'a5nca43';
var egressCheckboxRow = 'a5nca44';
var JulesDispatchDialog_css_errorBanner = 'a5nca4a';
var field = 'a5nca45';
var JulesDispatchDialog_css_heading = 'a5nca42';
var hint = 'a5nca49';
var JulesDispatchDialog_css_input = 'a5nca47';
var label = 'a5nca46';
var overlay = 'a5nca40';
var primaryButton = 'x9n6mdc';
var secondaryButton = 'x9n6mdd';
var JulesDispatchDialog_css_textarea = 'a5nca48 a5nca47';
;// ./src/components/backlog/JulesDispatchDialog.tsx
/* __next_internal_client_entry_do_not_use__ JulesDispatchDialog auto */ 
// +feature: jules-dispatch
// JulesDispatchDialog — Dispatch UX (google-jules-integration Epic 3.2,
// Story 3.2.1). Opened from ActionsSection's gated "Dispatch to Jules"
// button (BacklogItemDetail.tsx). Asks for the branch + prompt and, the
// first time a given repo is dispatched to, requires an explicit egress
// confirmation before any code leaves the machine. See
// project_plans/google-jules-integration/design/ux.md §3 for the full
// wireframe/interaction spec this implements.










// shortRepoLabel mirrors JulesSettings.tsx's own local helper (not exported,
// so duplicated here) — derives a display-friendly "owner/repo" from the
// full filesystem path stored in RepoPath/EgressAcknowledgedRepos.
function shortRepoLabel(repoPath) {
    const segments = repoPath.split("/").filter(Boolean);
    if (segments.length < 2) return repoPath;
    return segments.slice(-2).join("/");
}
// buildDefaultPrompt seeds the Prompt textarea from the item's own title +
// acceptance criteria, the same inputs a local spawn's triage prompt is
// built from — kept intentionally simple (no server round-trip) since the
// user can freely edit it before dispatch.
function buildDefaultPrompt(title, acCriteria) {
    if (acCriteria.length === 0) return title;
    const criteriaText = acCriteria.map((c)=>"- ".concat(c.text)).join("\n");
    return "".concat(title, "\n\nAcceptance criteria:\n").concat(criteriaText);
}
function JulesDispatchDialog(param) {
    let { itemId, itemTitle, acCriteria, repoPath, initialBranch, egressAcknowledged, onClose, triggerRef } = param;
    const headingId = (0,react.useId)();
    const dialogRef = (0,react.useRef)(null);
    (0,useFocusTrap/* useFocusTrap */.r)(dialogRef, true, triggerRef);
    const [branch, setBranch] = (0,react.useState)(initialBranch);
    const [prompt, setPrompt] = (0,react.useState)(()=>buildDefaultPrompt(itemTitle, acCriteria));
    const [consentChecked, setConsentChecked] = (0,react.useState)(false);
    const [submitting, setSubmitting] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    const repoLabel = shortRepoLabel(repoPath);
    const clientsRef = (0,react.useRef)(null);
    if (!clientsRef.current) {
        const transport = (0,connect_transport/* createConnectTransport */.w)({
            baseUrl: (0,config/* getApiBaseUrl */.h)(),
            interceptors: [
                (0,config/* createAuthInterceptor */._)()
            ]
        });
        clientsRef.current = {
            backlog: (0,promise_client/* createClient */.UU)(backlog_pb/* BacklogService */.jSM, transport),
            session: (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, transport)
        };
    }
    // Escape closes the dialog, mirroring ReviewChangesModal/
    // BacklogFileBrowserModal — useFocusTrap only handles Tab-cycling, not Esc.
    // Ignored while a dispatch is in flight, same guard as the disabled
    // Cancel button below (prevents an in-flight submit from being abandoned
    // mid-request).
    (0,react.useEffect)(()=>{
        function handleKeyDown(e) {
            if (e.key === "Escape" && !submitting) {
                e.stopPropagation();
                onClose();
            }
        }
        window.addEventListener("keydown", handleKeyDown, {
            capture: true
        });
        return ()=>window.removeEventListener("keydown", handleKeyDown, {
                capture: true
            });
    }, [
        onClose,
        submitting
    ]);
    const dispatchDisabled = submitting || branch.trim() === "" || prompt.trim() === "" || !egressAcknowledged && !consentChecked;
    async function handleDispatch() {
        if (dispatchDisabled || !clientsRef.current) return;
        setSubmitting(true);
        setError(null);
        try {
            if (!egressAcknowledged) {
                await clientsRef.current.session.confirmEgressConsent({
                    repoPath
                });
            }
            await clientsRef.current.backlog.dispatchToJules({
                itemId,
                branch: branch.trim(),
                prompt: prompt.trim()
            });
            onClose();
        } catch (err) {
            setError((0,connectError/* getErrorMessage */.u)(err, "Couldn't reach Jules. Try again."));
        } finally{
            setSubmitting(false);
        }
    }
    const content = /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: overlay,
        "data-testid": "jules-dispatch-overlay",
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            ref: dialogRef,
            role: "dialog",
            "aria-modal": "true",
            "aria-labelledby": headingId,
            className: dialog,
            "data-testid": "jules-dispatch-dialog",
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                    id: headingId,
                    className: JulesDispatchDialog_css_heading,
                    children: "Dispatch to Jules"
                }),
                !egressAcknowledged && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: egressBlock,
                    "data-testid": "jules-dispatch-egress-block",
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                            children: [
                                "The contents of ",
                                repoLabel,
                                " will be sent to Google's cloud VM to run this session."
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                            className: egressCheckboxRow,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                    type: "checkbox",
                                    checked: consentChecked,
                                    onChange: (e)=>setConsentChecked(e.target.checked),
                                    "data-testid": "jules-dispatch-egress-checkbox"
                                }),
                                "I understand and want to continue"
                            ]
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: field,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                            htmlFor: "jules-dispatch-branch",
                            className: label,
                            children: "Branch"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                            id: "jules-dispatch-branch",
                            type: "text",
                            className: JulesDispatchDialog_css_input,
                            value: branch,
                            onChange: (e)=>setBranch(e.target.value),
                            "aria-describedby": "jules-dispatch-branch-hint",
                            "data-testid": "jules-dispatch-branch"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                            id: "jules-dispatch-branch-hint",
                            className: hint,
                            children: "Jules starts from a branch already pushed to GitHub — local-only branches won't work."
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: field,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                            htmlFor: "jules-dispatch-prompt",
                            className: label,
                            children: "Prompt"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("textarea", {
                            id: "jules-dispatch-prompt",
                            className: JulesDispatchDialog_css_textarea,
                            value: prompt,
                            onChange: (e)=>setPrompt(e.target.value),
                            rows: 6,
                            "data-testid": "jules-dispatch-prompt"
                        })
                    ]
                }),
                error && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    role: "alert",
                    className: JulesDispatchDialog_css_errorBanner,
                    "data-testid": "jules-dispatch-error",
                    children: error
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: JulesDispatchDialog_css_actions,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            type: "button",
                            className: secondaryButton,
                            onClick: onClose,
                            disabled: submitting,
                            "data-testid": "jules-dispatch-cancel",
                            children: "Cancel"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            type: "button",
                            className: primaryButton,
                            onClick: ()=>{
                                void handleDispatch();
                            },
                            disabled: dispatchDisabled,
                            "data-testid": "jules-dispatch-submit",
                            children: submitting ? "Dispatching…" : "Dispatch"
                        })
                    ]
                })
            ]
        })
    });
    if (typeof document === "undefined") return null;
    return /*#__PURE__*/ (0,react_dom.createPortal)(content, document.body);
}

// EXTERNAL MODULE: ./src/lib/routes.ts + 1 modules
var routes = __webpack_require__(76514);
// EXTERNAL MODULE: ./src/lib/backlog/formatReworkCapOverride.ts
var formatReworkCapOverride = __webpack_require__(13333);
// EXTERNAL MODULE: ./src/components/shared/GitHubBadge.tsx
var GitHubBadge = __webpack_require__(20621);
// EXTERNAL MODULE: ./src/components/backlog/BlockerChip.tsx + 1 modules
var BlockerChip = __webpack_require__(82496);
// EXTERNAL MODULE: ./src/components/backlog/detail/StageTracker.tsx + 1 modules
var StageTracker = __webpack_require__(54821);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/timestamp.js
var timestamp = __webpack_require__(28765);
;// ./src/components/backlog/detail/LivenessLine.tsx



function parseDate(iso) {
    if (!iso) return undefined;
    const d = new Date(iso);
    return Number.isNaN(d.getTime()) ? undefined : d;
}
/**
 * A session's own "most recent activity" timestamp: the max of commit/
 * file-touch telemetry and endedAt/startedAt (lifecycle timestamps),
 * whichever is most recent. Falls back to undefined when none are set
 * (older sessions predating this telemetry, or sessions with no activity yet).
 */ function sessionActivity(session) {
    const candidates = [
        parseDate(session.lastCommitAt),
        parseDate(session.lastFileTouchAt),
        parseDate(session.endedAt),
        parseDate(session.startedAt)
    ];
    const known = candidates.filter((d)=>d !== undefined);
    if (known.length === 0) {
        return undefined;
    }
    return known.reduce((max, d)=>d.getTime() > max.getTime() ? d : max);
}
/**
 * Picks the max timestamp across the item's linked sessions, status events,
 * and progress notes, falling back to the item's own createdAt when none of
 * those sources have a timestamp (a brand-new item with no activity yet).
 */ function deriveLastActivity(item) {
    var _item_linkedSessions, _item_statusEvents, _item_progressNotes;
    const candidates = [
        ...((_item_linkedSessions = item.linkedSessions) !== null && _item_linkedSessions !== void 0 ? _item_linkedSessions : []).map(sessionActivity),
        ...((_item_statusEvents = item.statusEvents) !== null && _item_statusEvents !== void 0 ? _item_statusEvents : []).map((e)=>parseDate(e.createdAt)),
        ...((_item_progressNotes = item.progressNotes) !== null && _item_progressNotes !== void 0 ? _item_progressNotes : []).map((n)=>parseDate(n.createdAt))
    ];
    const known = candidates.filter((d)=>d !== undefined);
    if (known.length === 0) {
        return parseDate(item.createdAt);
    }
    return known.reduce((max, d)=>d.getTime() > max.getTime() ? d : max);
}
/**
 * Plain, non-`aria-live` static text ("Last activity Nm ago"). Deliberately
 * not wrapped in its own aria-live/role="status" region — re-announcing this
 * on every 5-second poll tick would be noise, not help (design/ux.md
 * Surface 1/2 "D. 5-second poll refresh" note, Accessibility AC 20).
 */ function LivenessLine(param) {
    let { item } = param;
    const lastActivity = deriveLastActivity(item);
    const label = lastActivity ? (0,stuckReason/* formatAgo */.n4)((0,timestamp/* timestampFromDate */.LI)(lastActivity)) : "unknown";
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
        "data-testid": "liveness-line",
        children: [
            "Last activity ",
            label
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2Fdetail%2FLifecycleSummary.css.ts.vanilla.css%22%2C%22source%22%3A%22LmFkNTk1ajAgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC13cmFwOiB3cmFwOwogIGFsaWduLWl0ZW1zOiBjZW50ZXI7CiAgZ2FwOiB2YXIoLS1fMTllejJyejMwKTsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejJ6KSAwOwp9Ci5hZDU5NWoxIHsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2EpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyem0pOwogIHdoaXRlLXNwYWNlOiBub3dyYXA7Cn0KLmFkNTk1ajIgewogIGRpc3BsYXk6IGlubGluZS1mbGV4OwogIGFsaWduLWl0ZW1zOiBjZW50ZXI7CiAgZ2FwOiB2YXIoLS1fMTllejJyejJ5KTsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejJ5KSB2YXIoLS1fMTllejJyejJ6KTsKICBib3JkZXItcmFkaXVzOiB2YXIoLS1fMTllejJyejM5KTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemspOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYSk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7CiAgd2hpdGUtc3BhY2U6IG5vd3JhcDsKfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2Fdetail_2FLifecycleSummary_css_ts_vanilla_css_22_2C_22source_22_3A_22LmFkNTk1ajAgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC13cmFwOiB3cmFwOwogIGFsaWduLWl0ZW1zOiBjZW50ZXI7CiAgZ2FwOiB2YXIoLS1fMTllejJyejMwKTsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejJ6KSAwOwp9Ci5hZDU5NWoxIHsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2EpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyem0pOwogIHdoaXRlLXNwYWNlOiBub3dyYXA7Cn0KLmFkNTk1ajIgewogIGRpc3BsYXk6IGlubGluZS1mbGV4OwogIGFsaWduLWl0ZW1zOiBjZW50ZXI7CiAgZ2FwOiB2YXIoLS1fMTllejJyejJ5KTsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejJ5KSB2YXIoLS1fMTllejJyejJ6KTsKICBib3JkZXItcmFkaXVzOiB2YXIoLS1fMTllejJyejM5KTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemspOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYSk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7CiAgd2hpdGUtc3BhY2U6IG5vd3JhcDsKfQ_22_7D = __webpack_require__(13744);
;// ./src/components/backlog/detail/LifecycleSummary.css.ts


var LifecycleSummary_css_container = 'ad595j0';
var pipelineBadge = 'ad595j2';
var unfinishedLink = 'ad595j1';
;// ./src/components/backlog/detail/LifecycleSummary.tsx
/* __next_internal_client_entry_do_not_use__ LifecycleSummary auto */ 







/** BlockerChip + "View in Unfinished" link, rendered only while the item is
 * currently flagged stuck. */ function StuckBlock(param) {
    let { itemId, stuckItem, otherStuckReasons, onTriggerRemediationNow, onOverrideClaimBlock } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)(BlockerChip/* BlockerChip */.B, {
                variant: "full",
                item: stuckItem,
                otherReasons: otherStuckReasons,
                onTriggerRemediationNow: onTriggerRemediationNow,
                onOverrideClaimBlock: onOverrideClaimBlock
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                href: routes/* routes */.J.unfinishedItem(itemId),
                className: unfinishedLink,
                "data-testid": "lifecycle-unfinished-link",
                children: "View in Unfinished"
            })
        ]
    });
}
/**
 * PR link for the sticky header. Omitted for pr_pending because
 * PullRequestSection already renders the link expanded in the scroll area.
 * Falls back to a plain link when prNumber is unknown (GitHubBadge needs it).
 */ function PrBadge(param) {
    let { item } = param;
    if (!item.prUrl || item.status === "pr_pending") return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
        "data-testid": "lifecycle-pr-badge",
        children: item.prNumber && item.prNumber > 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)(GitHubBadge/* GitHubBadge */.J, {
            prNumber: item.prNumber,
            prUrl: item.prUrl
        }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
            href: item.prUrl,
            target: "_blank",
            rel: "noopener noreferrer",
            className: pipelineBadge,
            children: "Pull request"
        })
    });
}
/** Rework-cap override badge — hidden entirely when no override is set. */ function ReworkCapBadge(param) {
    let { reworkCapOverride } = param;
    const reworkCap = (0,formatReworkCapOverride/* resolveReworkCapOverride */.H)(reworkCapOverride);
    if (reworkCap.kind === "unset") return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
        className: pipelineBadge,
        "data-testid": "lifecycle-rework-cap-badge",
        children: [
            "Rework cap: ",
            reworkCap.kind === "unlimited" ? "unlimited" : reworkCap.rounds
        ]
    });
}
/**
 * Compact, read-only automation-profile chips (CONFIGURABILITY GAP fix, UX
 * audit 2026-09-11): Pipeline mode / skip planning / skip review gate /
 * auto-spawn / auto-create PR were previously only visible by opening Edit
 * (BacklogItemForm.tsx). Each chip only renders when it deviates from the
 * default (off, or default pipeline), keeping the common unconfigured item
 * uncluttered.
 */ function AutomationProfileBadges(param) {
    let { item, showPipelineBadge, configuredPipelineModeName } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            !showPipelineBadge && configuredPipelineModeName && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                className: pipelineBadge,
                "data-testid": "lifecycle-configured-pipeline-badge",
                children: [
                    "Pipeline: ",
                    configuredPipelineModeName
                ]
            }),
            item.skipPlanning && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: pipelineBadge,
                "data-testid": "lifecycle-skip-planning-badge",
                children: "Skip planning"
            }),
            item.skipReviewGate && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: pipelineBadge,
                "data-testid": "lifecycle-skip-review-badge",
                children: "Skip review gate"
            }),
            item.autoSpawnSession && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: pipelineBadge,
                "data-testid": "lifecycle-auto-spawn-badge",
                children: "Auto-spawn"
            }),
            item.autoCreatePR && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: pipelineBadge,
                "data-testid": "lifecycle-auto-create-pr-badge",
                children: "Auto-create PR"
            })
        ]
    });
}
/**
 * Always-visible header region — Stage Tracker + Blocker Chip + Pipeline
 * badge + Liveness Line — replacing the old standalone status badge (D1).
 * The single authoritative place lifecycle status is shown.
 */ function LifecycleSummary(param) {
    let { item, pipelineDisplay, configuredPipelineModeName, stuckItem, otherStuckReasons, onTriggerRemediationNow, onOverrideClaimBlock } = param;
    // Only a "resolved" mode with a non-default name is glanceable-worthy —
    // the common default-pipeline case renders no badge at all (Task 3.1.4g).
    const showPipelineBadge = (pipelineDisplay === null || pipelineDisplay === void 0 ? void 0 : pipelineDisplay.kind) === "resolved" && pipelineDisplay.name !== "default";
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: LifecycleSummary_css_container,
        "data-testid": "lifecycle-summary",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)(StageTracker/* StageTracker */.zG, {
                status: item.status
            }),
            stuckItem && /*#__PURE__*/ (0,jsx_runtime.jsx)(StuckBlock, {
                itemId: item.id,
                stuckItem: stuckItem,
                otherStuckReasons: otherStuckReasons,
                onTriggerRemediationNow: onTriggerRemediationNow,
                onOverrideClaimBlock: onOverrideClaimBlock
            }),
            item.category && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                className: pipelineBadge,
                "data-testid": "lifecycle-category-badge",
                children: [
                    "Category: ",
                    item.category
                ]
            }),
            showPipelineBadge && (pipelineDisplay === null || pipelineDisplay === void 0 ? void 0 : pipelineDisplay.kind) === "resolved" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                className: pipelineBadge,
                "data-testid": "lifecycle-pipeline-badge",
                children: [
                    "Pipeline: ",
                    pipelineDisplay.name
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(AutomationProfileBadges, {
                item: item,
                showPipelineBadge: showPipelineBadge,
                configuredPipelineModeName: configuredPipelineModeName
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(ReworkCapBadge, {
                reworkCapOverride: item.reworkCapOverride
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(PrBadge, {
                item: item
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(LivenessLine, {
                item: item
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/backlog/dependencies.ts
var dependencies = __webpack_require__(17213);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2Fdetail%2FDependenciesSection.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xdG04ZTB6MCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeik7CiAgcGFkZGluZzogdmFyKC0tXzE5ZXoycnozMCk7CiAgYm9yZGVyLXJhZGl1czogdmFyKC0tXzE5ZXoycnozNyk7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnpkKTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwp9Ci5fMXRtOGUwejEgewogIG1hcmdpbjogMDsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2IpOwogIGZvbnQtd2VpZ2h0OiB2YXIoLS1fMTllejJyejNoKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnowKTsKfQouXzF0bThlMHoyIHsKICBtYXJnaW46IDA7CiAgcGFkZGluZy1sZWZ0OiB2YXIoLS1fMTllejJyejMxKTsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2IpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejEpOwp9Ci5fMXRtOGUwejMgewogIG1hcmdpbi1sZWZ0OiB2YXIoLS1fMTllejJyejJ5KTsKICBvcGFjaXR5OiAwLjc1Owp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2Fdetail_2FDependenciesSection_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xdG04ZTB6MCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeik7CiAgcGFkZGluZzogdmFyKC0tXzE5ZXoycnozMCk7CiAgYm9yZGVyLXJhZGl1czogdmFyKC0tXzE5ZXoycnozNyk7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnpkKTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwp9Ci5fMXRtOGUwejEgewogIG1hcmdpbjogMDsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2IpOwogIGZvbnQtd2VpZ2h0OiB2YXIoLS1fMTllejJyejNoKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnowKTsKfQouXzF0bThlMHoyIHsKICBtYXJnaW46IDA7CiAgcGFkZGluZy1sZWZ0OiB2YXIoLS1fMTllejJyejMxKTsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2IpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejEpOwp9Ci5fMXRtOGUwejMgewogIG1hcmdpbi1sZWZ0OiB2YXIoLS1fMTllejJyejJ5KTsKICBvcGFjaXR5OiAwLjc1Owp9_22_7D = __webpack_require__(80520);
;// ./src/components/backlog/detail/DependenciesSection.css.ts


var DependenciesSection_css_heading = '_1tm8e0z1';
var DependenciesSection_css_list = '_1tm8e0z2';
var DependenciesSection_css_section = '_1tm8e0z0';
var DependenciesSection_css_status = '_1tm8e0z3';
;// ./src/components/backlog/detail/DependenciesSection.tsx
/* __next_internal_client_entry_do_not_use__ DependenciesSection auto */ 


function DependencyList(param) {
    let { label, deps, testId } = param;
    if (deps.length === 0) return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        "data-testid": testId,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("h4", {
                className: DependenciesSection_css_heading,
                children: label
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                className: DependenciesSection_css_list,
                children: deps.map((d)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                        children: [
                            d.title || d.publicId || d.itemId,
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: DependenciesSection_css_status,
                                children: [
                                    "(",
                                    d.status,
                                    (0,dependencies/* isBlockerUnresolved */.g8)(d) ? "" : ", resolved",
                                    ")"
                                ]
                            })
                        ]
                    }, d.itemId))
            })
        ]
    });
}
/**
 * Item-detail view of the dependency edges: what this item is blocked by and
 * what is waiting on it. Renders nothing for an item with no edges.
 */ function DependenciesSection(param) {
    let { item } = param;
    var _item_blockedBy;
    const blockedBy = (_item_blockedBy = item.blockedBy) !== null && _item_blockedBy !== void 0 ? _item_blockedBy : [];
    var _item_blocking;
    const blocking = (_item_blocking = item.blocking) !== null && _item_blocking !== void 0 ? _item_blocking : [];
    if (blockedBy.length === 0 && blocking.length === 0) return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
        className: DependenciesSection_css_section,
        "aria-label": "Dependencies",
        "data-testid": "backlog-dependencies",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)(DependencyList, {
                label: "Blocked by",
                deps: blockedBy,
                testId: "backlog-blocked-by"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(DependencyList, {
                label: "Blocking",
                deps: blocking,
                testId: "backlog-blocking"
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/contexts/FeatureFlagsContext.tsx
var FeatureFlagsContext = __webpack_require__(63367);
;// ./src/components/backlog/detail/PlanningSection.tsx
/* __next_internal_client_entry_do_not_use__ PlanningSection auto */ 


/**
 * Read-only triage-result record for items past `idea` status — extracted
 * verbatim from BacklogItemDetail.tsx (Story 3.1.2, Task 3.1.2a). Always
 * visible when relevant (primary content, not progressive disclosure) —
 * matches the pre-extraction behavior exactly.
 */ function PlanningSection(param) {
    let { item } = param;
    // TriageResultPanel renders the same data when its flag is on.
    const replacedByTriagePanel = (0,FeatureFlagsContext/* useFeatureFlag */.u)("backlog-triage-results-panel");
    if (replacedByTriagePanel || !item.triageResult || item.status === "idea") return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: section,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                className: sectionTitle,
                children: "Planning"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: planSummary,
                children: item.triageResult.summary
            }),
            item.triageResult.tasks && item.triageResult.tasks.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: planTaskList,
                children: item.triageResult.tasks.map((t, i)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: planTask,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: planTaskText,
                                children: t.text
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: planTaskMeta,
                                children: [
                                    t.estimate && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: planTaskBadge,
                                        children: t.estimate
                                    }),
                                    t.category && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: planTaskBadge,
                                        children: t.category
                                    })
                                ]
                            })
                        ]
                    }, i))
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FGateVerdictBox.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA71Y23LiMAx95yv8sjPtg3eSAKUNL_snO06sBC-OnToOt53--04cLjbk4ha6LxRG8pEs6Uhyf61hnylSQIWK5DBfRQH6O0EoU7IwXxDSiogqk6qIkZKaaHgKKOTPywlCHxOEtOzVm75Ymh-Tn62B0OhTVpWc7GOUcdg1Gs1fTJmCVDMpYpRKXheikeSkjNGGqCeMf4dvcIjUYRo-Ly-IUeuyFBpX7AA3yolxwci3wPKVvtFYGY1UcqmuZZERadhpbN2wLktQKamgEXLQGhSuSpIykcco-BnMoWgkBVE5EziRWsviBnlvX2JqLpFIRUFhRSirqxs3F8aXklBq7FxJAyM9InDIdIxm5Q5VkjO6vDvk0cH2dmZ729jCncGrW5dIus6VrAW9lm9szLkf5m4E04nqix9mGIyAhk7BLfxQoxHQhY35-g2Yb91UI5zlAjMNRRWjFIQG5ZX0xMANJHqIYsxGSvuRdrYe7dczKTsrQr9iGzEiWEGaMseCFBCfmp0rorUiLRXCypVoVjCR46wWR65wJoAoV4lpaM_jVNZCx4iJjAmmwWEfE81ZnHCZrq0bZMM3OOvlI82OeGXiyx3NAGekYHx_42Zt-7kaLpazHvMshT8DpeAEaO0ZSO6pV3hNl27HbBxxd-K-eUppOcJ_-YDJbYbeoMlWgbNK40rvOcRISAGXQzEKnCkYWB6WD2l4Y9k-m3v3rHHl28aqR_nfpGSrGnnzaVnQroVjMxoxVDCBV8eanM3K3dJzUxklTa2qJiSlZK2ps5u1cbNn04kOz1374NBYLPqp826tTZdaG-JhbqesjldyA22G--0L9whlFUk4tONNNozU-4aRczsqQmpMOJdboNbhzd2ROV3xdOXwtCReHc2HG9snYrSxY9TWTSfsQINzaLJ9cHUc17_vKI-tV3mEU_vM7gtlsb-xcJ3m0-8b0g2OlLF5ZUYOhVSeNqdaUFBNU_Fq851d_PCIF2IwNmec0RYG98TvnqfnKMuGI9wZwPAhb2yHcuH9j-yRm15MtU_hLaN6FaMwCH5cz6BF1M6g_g4wwncy4EriEL6vOa7tx_ZdQ3Boo94YDQXH46A0Swl3YhVnMq3bvUHWuimL69Lt7rXcCfnMazkdX5bDeXfljb4tw5fHkd7nHyRjY88ns_0FRj_XfBb_j1uvX3_NmET_A90-1rIwFAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FGateVerdictBox_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA71Y23LiMAx95yv8sjPtg3eSAKUNL_snO06sBC_OnToOt53_04cLjbk4ha6LxRG8pEs6Uhyf61hnylSQIWK5DBfRQH6O0EoU7IwXxDSiogqk6qIkZKaaHgKKOTPywlCHxOEtOzVm75Ymh_Tn62B0OhTVpWc7GOUcdg1Gs1fTJmCVDMpYpRKXheikeSkjNGGqCeMf4dvcIjUYRo_Ly_IUeuyFBpX7AA3yolxwci3wPKVvtFYGY1UcqmuZZERadhpbN2wLktQKamgEXLQGhSuSpIykcco_BnMoWgkBVE5EziRWsviBnlvX2JqLpFIRUFhRSirqxs3F8aXklBq7FxJAyM9InDIdIxm5Q5VkjO6vDvk0cH2dmZ729jCncGrW5dIus6VrAW9lm9szLkf5m4E04nqix9mGIyAhk7BLfxQoxHQhY35_g2Yb91UI5zlAjMNRRWjFIQG5ZX0xMANJHqIYsxGSvuRdrYe7dczKTsrQr9iGzEiWEGaMseCFBCfmp0rorUiLRXCypVoVjCR46wWR65wJoAoV4lpaM_jVNZCx4iJjAmmwWEfE81ZnHCZrq0bZMM3OOvlI82OeGXiyx3NAGekYHx_42Zt_7kaLpazHvMshT8DpeAEaO0ZSO6pV3hNl27HbBxxd_K_eUppOcJ_YDJbYbeoMlWgbNK40rvOcRISAGXQzEKnCkYWB6WD2l4Y9k_m3v3rHHl28aqR_nfpGSrGnnzaVnQroVjMxoxVDCBV8eanM3K3dJzUxklTa2qJiSlZK2ps5u1cbNn04kOz1374NBYLPqp826tTZdaG_JhbqesjldyA22G_0L9whlFUk4tONNNozU_4aRczsqQmpMOJdboNbhzd2ROV3xdOXwtCReHc2HG9snYrSxY9TWTSfsQINzaLJ9cHUc17_vKI_tV3mEU_vM7gtlsb_xcJ3m0_8b0g2OlLF5ZUYOhVSeNqdaUFBNU_Fq851d_PCIF2IwNmec0RYG98TvnqfnKMuGI9wZwPAhb2yHcuH9j_yRm15MtU_hLaN6FaMwCH5cz6BF1M6g_g4wwncy4EriEL6vOa7tx_ZdQ3Boo94YDQXH46A0Swl3YhVnMq3bvUHWuimL69Lt7rXcCfnMazkdX5bDeXfljb4tw5fHkd7nHyRjY88ns_0FRj_XfBb_j1uvX3_NmET_A90_1rIwFAAA_22_7D = __webpack_require__(51372);
;// ./src/components/backlog/GateVerdictBox.css.ts


var GateVerdictBox_css_actions = 'mbz5h2s';
var criteriaHeading = 'mbz5h2n';
var criteriaIconFail = 'mbz5h2r';
var criteriaIconPass = 'mbz5h2q';
var criteriaItem = 'mbz5h2p';
var criteriaList = 'mbz5h2o';
var dangerButton = 'mbz5h2w mbz5h2t';
var disabledButton = 'mbz5h2x';
var formActions = 'mbz5h215';
var formHint = 'mbz5h214';
var formLabel = 'mbz5h212';
var formTextarea = 'mbz5h213';
var overrideForm = 'mbz5h211';
var overrideSection = 'mbz5h2z';
var overrideToggle = 'mbz5h210';
var GateVerdictBox_css_primaryButton = 'mbz5h2u mbz5h2t';
var GateVerdictBox_css_secondaryButton = 'mbz5h2v mbz5h2t';
var GateVerdictBox_css_section = 'mbz5h21';
var GateVerdictBox_css_sectionTitle = 'mbz5h22';
var skipGateBody = 'mbz5h218';
var skipGateConfirmation = 'mbz5h216';
var skipGateWarning = 'mbz5h217';
var skipLink = 'mbz5h2y';
var spinKeyframes = 'mbz5h20';
var verdictCard = 'mbz5h23';
var verdictCardFail = 'mbz5h26 mbz5h23';
var verdictCardPartial = 'mbz5h25 mbz5h23';
var verdictCardPass = 'mbz5h24 mbz5h23';
var verdictCardPending = 'mbz5h27 mbz5h23';
var verdictCardUnverifiable = 'mbz5h28 mbz5h23';
var verdictHeader = 'mbz5h29';
var verdictIcon = 'mbz5h2a';
var verdictIconFail = 'mbz5h2d';
var verdictIconPartial = 'mbz5h2c';
var verdictIconPass = 'mbz5h2b';
var verdictIconPending = 'mbz5h2e';
var verdictIconUnverifiable = 'mbz5h2f';
var verdictLabel = 'mbz5h2g';
var verdictLabelFail = 'mbz5h2j';
var verdictLabelPartial = 'mbz5h2i';
var verdictLabelPass = 'mbz5h2h';
var verdictLabelPending = 'mbz5h2k';
var verdictLabelUnverifiable = 'mbz5h2l';
var GateVerdictBox_css_verdictSummary = 'mbz5h2m';
;// ./src/components/backlog/GateVerdictBox.tsx
/* __next_internal_client_entry_do_not_use__ GateVerdictBox auto */ 
// +feature: backlog:gate-verdict




function GateVerdictBox_isReadOnlyProps(props) {
    return props.readOnly === true;
}
const MIN_OVERRIDE_REASON_LENGTH = 5;
const VERDICT_CONFIG = {
    PASS: {
        icon: "✓",
        label: "PASSED",
        cardClass: verdictCardPass,
        iconClass: verdictIconPass,
        labelClass: verdictLabelPass
    },
    PARTIAL: {
        icon: "◑",
        label: "PARTIAL",
        cardClass: verdictCardPartial,
        iconClass: verdictIconPartial,
        labelClass: verdictLabelPartial
    },
    FAIL: {
        icon: "✗",
        label: "FAILED",
        cardClass: verdictCardFail,
        iconClass: verdictIconFail,
        labelClass: verdictLabelFail
    },
    PENDING: {
        icon: "◌",
        label: "PENDING",
        cardClass: verdictCardPending,
        iconClass: verdictIconPending,
        labelClass: verdictLabelPending
    },
    UNVERIFIABLE: {
        icon: "?",
        label: "UNVERIFIABLE",
        cardClass: verdictCardUnverifiable,
        iconClass: verdictIconUnverifiable,
        labelClass: verdictLabelUnverifiable
    }
};
function GateVerdictBox(props) {
    const { verdict, summary, criteria, elapsedSeconds, actionPending = false } = props;
    const readOnly = GateVerdictBox_isReadOnlyProps(props);
    const onApprove = GateVerdictBox_isReadOnlyProps(props) ? undefined : props.onApprove;
    const onReopen = GateVerdictBox_isReadOnlyProps(props) ? undefined : props.onReopen;
    const onOverride = GateVerdictBox_isReadOnlyProps(props) ? undefined : props.onOverride;
    const onSkipGate = GateVerdictBox_isReadOnlyProps(props) ? undefined : props.onSkipGate;
    const onReReview = GateVerdictBox_isReadOnlyProps(props) ? undefined : props.onReReview;
    const [showOverride, setShowOverride] = (0,react.useState)(false);
    const [overrideReason, setOverrideReason] = (0,react.useState)("");
    const [showReopen, setShowReopen] = (0,react.useState)(false);
    const [reopenFeedback, setReopenFeedback] = (0,react.useState)("");
    const [showSkipConfirm, setShowSkipConfirm] = (0,react.useState)(false);
    const [localPending, setLocalPending] = (0,react.useState)(false);
    const [actionError, setActionError] = (0,react.useState)(null);
    const isPending = localPending || actionPending;
    const config = VERDICT_CONFIG[verdict];
    const overrideToggleRef = (0,react.useRef)(null);
    const skipLinkRef = (0,react.useRef)(null);
    const skipConfirmDialogRef = (0,react.useRef)(null);
    // skipLinkRef's button and the dialog occupy the same position in a
    // ternary, so React unmounts/remounts a brand-new button element on every
    // toggle — useFocusTrap's own triggerRef restore would capture a DOM node
    // that's already disconnected by the time its cleanup runs. Restore focus
    // to the (new) button ourselves once it has actually remounted instead.
    (0,useFocusTrap/* useFocusTrap */.r)(skipConfirmDialogRef, showSkipConfirm);
    const wasSkipConfirmOpenRef = (0,react.useRef)(false);
    (0,react.useEffect)(()=>{
        if (wasSkipConfirmOpenRef.current && !showSkipConfirm) {
            var _skipLinkRef_current;
            (_skipLinkRef_current = skipLinkRef.current) === null || _skipLinkRef_current === void 0 ? void 0 : _skipLinkRef_current.focus();
        }
        wasSkipConfirmOpenRef.current = showSkipConfirm;
    }, [
        showSkipConfirm
    ]);
    // Focus override reason textarea when form opens
    (0,react.useEffect)(()=>{
        if (showOverride) {
            const el = document.getElementById("override-reason");
            if (el) {
                el.focus();
            }
        }
    }, [
        showOverride
    ]);
    // Focus reopen feedback textarea when form opens
    (0,react.useEffect)(()=>{
        if (showReopen) {
            const el = document.getElementById("reopen-feedback");
            if (el) {
                el.focus();
            }
        }
    }, [
        showReopen
    ]);
    function handleKeyDown(e) {
        if (readOnly) return;
        if (e.key === "Enter" && e.ctrlKey) {
            if (verdict === "PASS") {
                void handleApprove();
            } else if (verdict === "PARTIAL" || verdict === "FAIL") {
                setShowReopen(true);
            }
        }
    }
    async function handleApprove() {
        if (!onApprove) return;
        setLocalPending(true);
        try {
            await onApprove();
        } catch (err) {
            setActionError("Action failed. Please try again.");
            console.error(err);
        } finally{
            setLocalPending(false);
        }
    }
    async function handleReReview() {
        if (!onReReview) return;
        setLocalPending(true);
        try {
            await onReReview();
        } catch (err) {
            setActionError("Action failed. Please try again.");
            console.error(err);
        } finally{
            setLocalPending(false);
        }
    }
    async function handleReopenSubmit() {
        if (!onReopen) return;
        setLocalPending(true);
        try {
            await onReopen(reopenFeedback);
            setShowReopen(false);
            setReopenFeedback("");
        } catch (err) {
            setActionError("Action failed. Please try again.");
            console.error(err);
        } finally{
            setLocalPending(false);
        }
    }
    async function handleOverrideSubmit() {
        if (!onOverride) return;
        setLocalPending(true);
        try {
            await onOverride(overrideReason);
        } catch (err) {
            setActionError("Action failed. Please try again.");
            console.error(err);
        } finally{
            setLocalPending(false);
        }
    }
    async function handleSkipGateConfirm() {
        if (!onSkipGate) return;
        setLocalPending(true);
        try {
            await onSkipGate();
        } catch (err) {
            setActionError("Action failed. Please try again.");
            console.error(err);
        } finally{
            setLocalPending(false);
        }
    }
    function handleSkipConfirmKeyDown(e) {
        if (e.key === "Escape") {
            setShowSkipConfirm(false);
        }
    }
    function handleOverrideFormKeyDown(e) {
        if (e.key === "Escape") {
            var _overrideToggleRef_current;
            setShowOverride(false);
            setOverrideReason("");
            (_overrideToggleRef_current = overrideToggleRef.current) === null || _overrideToggleRef_current === void 0 ? void 0 : _overrideToggleRef_current.focus();
        }
    }
    const showCriteria = criteria && criteria.length > 0;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
        role: "status",
        "aria-live": "polite",
        "aria-atomic": "true",
        "aria-label": "Gate verdict",
        className: GateVerdictBox_css_section,
        onKeyDown: handleKeyDown,
        tabIndex: -1,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: GateVerdictBox_css_sectionTitle,
                children: "Gate Verdict"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: config.cardClass,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: verdictHeader,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: config.iconClass,
                                "aria-hidden": "true",
                                children: config.icon
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: "".concat(verdictLabel, " ").concat(config.labelClass),
                                children: config.label
                            }),
                            verdict === "PENDING" && elapsedSeconds !== undefined && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: GateVerdictBox_css_verdictSummary,
                                "aria-label": "Elapsed: ".concat(elapsedSeconds, " seconds"),
                                children: [
                                    elapsedSeconds,
                                    "s"
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: GateVerdictBox_css_verdictSummary,
                        children: summary
                    }),
                    showCriteria && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                className: criteriaHeading,
                                children: "Review outcome per criterion"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                                className: criteriaList,
                                "aria-label": "Criteria results",
                                children: criteria.map((c, i)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                                        className: criteriaItem,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: c.passed ? criteriaIconPass : criteriaIconFail,
                                                "aria-hidden": "true",
                                                children: c.passed ? "✓" : "✗"
                                            }),
                                            c.label
                                        ]
                                    }, i))
                            })
                        ]
                    })
                ]
            }),
            !readOnly && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: GateVerdictBox_css_actions,
                children: [
                    verdict === "PASS" && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: GateVerdictBox_css_primaryButton,
                                onClick: ()=>void handleApprove(),
                                disabled: isPending,
                                children: "Approve — Mark Done"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: GateVerdictBox_css_secondaryButton,
                                onClick: ()=>setShowReopen(true),
                                disabled: isPending,
                                children: "Reopen for Revision"
                            })
                        ]
                    }),
                    (verdict === "PARTIAL" || verdict === "FAIL") && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: GateVerdictBox_css_primaryButton,
                        onClick: ()=>setShowReopen(true),
                        disabled: isPending || readOnly,
                        children: "Reopen for Revision"
                    }),
                    verdict === "PENDING" && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: GateVerdictBox_css_primaryButton,
                                "aria-disabled": "true",
                                disabled: true,
                                title: "Wait for gate result or use Skip Gate below",
                                children: "Approve — Mark Done"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: GateVerdictBox_css_secondaryButton,
                                onClick: ()=>setShowReopen(true),
                                disabled: isPending,
                                children: "Reopen for Revision"
                            })
                        ]
                    }),
                    verdict === "UNVERIFIABLE" && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            onReReview && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: GateVerdictBox_css_primaryButton,
                                onClick: ()=>void handleReReview(),
                                disabled: isPending,
                                children: "Re-run Gate"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: GateVerdictBox_css_secondaryButton,
                                onClick: ()=>setShowReopen(true),
                                disabled: isPending,
                                children: "Reopen for Revision"
                            })
                        ]
                    })
                ]
            }),
            !readOnly && showReopen && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                role: "form",
                "aria-label": "Reopen for revision",
                className: overrideForm,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                        htmlFor: "reopen-feedback",
                        className: formLabel,
                        children: "Feedback for the agent (optional)"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("textarea", {
                        id: "reopen-feedback",
                        rows: 3,
                        placeholder: "What should the agent fix or improve?",
                        value: reopenFeedback,
                        onChange: (e)=>setReopenFeedback(e.target.value),
                        className: formTextarea
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: formActions,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: GateVerdictBox_css_secondaryButton,
                                onClick: ()=>{
                                    setShowReopen(false);
                                    setReopenFeedback("");
                                },
                                children: "Cancel"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: GateVerdictBox_css_primaryButton,
                                disabled: isPending || readOnly,
                                onClick: ()=>void handleReopenSubmit(),
                                children: "Reopen & Spawn Session"
                            })
                        ]
                    })
                ]
            }),
            actionError && /*#__PURE__*/ (0,jsx_runtime.jsx)(InlineError, {
                type: "transient",
                onRetry: ()=>setActionError(null),
                onDismiss: ()=>setActionError(null),
                customMessage: actionError
            }),
            !readOnly && (verdict === "PARTIAL" || verdict === "FAIL" || verdict === "UNVERIFIABLE") && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: overrideSection,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                        ref: overrideToggleRef,
                        className: overrideToggle,
                        "aria-expanded": showOverride,
                        onClick: ()=>setShowOverride((prev)=>!prev),
                        children: [
                            "Override: Mark done anyway ",
                            showOverride ? "▾" : "▸"
                        ]
                    }),
                    showOverride && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        role: "form",
                        "aria-label": "Override gate verdict",
                        id: "override-form",
                        className: overrideForm,
                        onKeyDown: handleOverrideFormKeyDown,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                htmlFor: "override-reason",
                                className: formLabel,
                                children: "Reason for override (required)"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("textarea", {
                                id: "override-reason",
                                rows: 3,
                                placeholder: "Explain why this item should be marked done despite...",
                                "aria-describedby": "override-hint",
                                value: overrideReason,
                                onChange: (e)=>setOverrideReason(e.target.value),
                                className: formTextarea
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                id: "override-hint",
                                className: formHint,
                                children: "Enter at least 5 characters to continue."
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: formActions,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        className: GateVerdictBox_css_secondaryButton,
                                        onClick: ()=>{
                                            setShowOverride(false);
                                            setOverrideReason("");
                                        },
                                        children: "Cancel"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        className: dangerButton,
                                        "aria-disabled": overrideReason.trim().length < MIN_OVERRIDE_REASON_LENGTH,
                                        disabled: overrideReason.trim().length < MIN_OVERRIDE_REASON_LENGTH,
                                        onClick: ()=>void handleOverrideSubmit(),
                                        children: "Mark Done — Override"
                                    })
                                ]
                            })
                        ]
                    })
                ]
            }),
            !readOnly && (showSkipConfirm ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                ref: skipConfirmDialogRef,
                role: "alertdialog",
                "aria-labelledby": "skip-gate-warning",
                "aria-modal": "true",
                className: skipGateConfirmation,
                onKeyDown: handleSkipConfirmKeyDown,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        id: "skip-gate-warning",
                        className: skipGateWarning,
                        children: "Skip gate and mark done without review"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: skipGateBody,
                        children: "The acceptance criteria will not be evaluated. This cannot be undone."
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: formActions,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: GateVerdictBox_css_secondaryButton,
                                onClick: ()=>setShowSkipConfirm(false),
                                children: "Cancel"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: dangerButton,
                                onClick: ()=>void handleSkipGateConfirm(),
                                children: "Confirm — Skip Gate"
                            })
                        ]
                    })
                ]
            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                ref: skipLinkRef,
                className: skipLink,
                onClick: ()=>setShowSkipConfirm(true),
                children: "Skip gate and mark done without review"
            }))
        ]
    });
}

;// ./src/components/backlog/detail/ReviewingSection.tsx
/* __next_internal_client_entry_do_not_use__ ReviewingSection auto */ 



/**
 * The "Reviewing" work-session context box + GateVerdictBox — only rendered
 * while `item.status === "review"` (guard preserved verbatim from the
 * pre-extraction inline block, Story 3.1.2 Task 3.1.2b). Wrapped in a
 * Collapsible, default-expanded only while the item is actually in review.
 */ function ReviewingSection(param) {
    let { item, workSession, actionLoading, defaultExpanded, onViewChanges, onGateApprove, onGateReopen, onGateOverride, onGateSkip, onReReview, readOnly = false } = param;
    const activeReviewSession = [
        ...item.linkedSessions
    ].reverse().find((s)=>s.role === "review" && !s.endedAt && !s.sessionId.startsWith("headless-") && !s.sessionId.startsWith("review-blocked-"));
    var _item_gateVerdict, _item_gateVerdict1;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(Collapsible/* CollapsibleSection */.i, {
        sectionKey: "reviewing",
        title: "Reviewing",
        defaultExpanded: defaultExpanded,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: section,
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: reviewContextBox,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: reviewContextInfo,
                            children: [
                                workSession ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: reviewContextLabel,
                                            children: "Work session"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                                            className: reviewContextSessionId,
                                            href: "/?session=".concat(workSession.sessionId),
                                            title: "Open in terminal",
                                            children: workSession.sessionId
                                        }),
                                        workSession.endedAt && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                            className: reviewContextDate,
                                            children: [
                                                "Completed ",
                                                new Date(workSession.endedAt).toLocaleString()
                                            ]
                                        })
                                    ]
                                }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: reviewContextLabel,
                                    children: "No work session found"
                                }),
                                activeReviewSession && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: reviewContextLabel,
                                            children: "Review session"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                                            className: reviewContextSessionId,
                                            href: "/?session=".concat(activeReviewSession.sessionId),
                                            title: "Open review session in terminal",
                                            children: activeReviewSession.sessionId
                                        })
                                    ]
                                })
                            ]
                        }),
                        workSession && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: viewChangesButton,
                            onClick: onViewChanges,
                            "data-testid": "backlog-review-view-changes",
                            children: "View Changes ↗"
                        })
                    ]
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: section,
                children: readOnly ? /*#__PURE__*/ (0,jsx_runtime.jsx)(GateVerdictBox, {
                    readOnly: true,
                    verdict: (_item_gateVerdict = item.gateVerdict) !== null && _item_gateVerdict !== void 0 ? _item_gateVerdict : "PENDING",
                    summary: item.gateVerdictSummary || "Review in progress",
                    criteria: item.gateCriteria,
                    elapsedSeconds: undefined
                }) : /*#__PURE__*/ (0,jsx_runtime.jsx)(GateVerdictBox, {
                    verdict: (_item_gateVerdict1 = item.gateVerdict) !== null && _item_gateVerdict1 !== void 0 ? _item_gateVerdict1 : "PENDING",
                    summary: item.gateVerdictSummary || "Review in progress",
                    criteria: item.gateCriteria,
                    elapsedSeconds: undefined,
                    onApprove: onGateApprove,
                    onReopen: onGateReopen,
                    onOverride: onGateOverride,
                    onSkipGate: onGateSkip,
                    onReReview: onReReview,
                    actionPending: actionLoading !== null
                })
            })
        ]
    });
}

;// ./src/components/backlog/detail/LastReviewResultSection.tsx
/* __next_internal_client_entry_do_not_use__ LastReviewResultSection auto */ 



/**
 * Read-only historical record of the item's most recent review verdict,
 * shown once the item has bounced back to a non-review status (e.g.
 * in_progress after a FAIL triggered auto-reopen). item.gateVerdict/
 * gateCriteria are derived from the most recent review session regardless of
 * current status, but ReviewingSection only ever renders while
 * status === "review" — without this section, a verdict that caused a
 * rework round silently disappeared the moment the item left review.
 */ function LastReviewResultSection(param) {
    let { item, defaultExpanded } = param;
    var _item_gateVerdict;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(Collapsible/* CollapsibleSection */.i, {
        sectionKey: "last-review-result",
        title: "Last Review Result",
        defaultExpanded: defaultExpanded,
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: section,
            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(GateVerdictBox, {
                readOnly: true,
                verdict: (_item_gateVerdict = item.gateVerdict) !== null && _item_gateVerdict !== void 0 ? _item_gateVerdict : "PENDING",
                summary: item.gateVerdictSummary || "Review in progress",
                criteria: item.gateCriteria,
                elapsedSeconds: undefined
            })
        })
    });
}

;// ./src/components/backlog/detail/ActionButtonLabel.tsx
/* __next_internal_client_entry_do_not_use__ ActionButtonLabel auto */ 

/**
 * Renders a button's label, swapping in a spinner + "Running…" while
 * `pending`. Shared by ActionsSection, PullRequestSection, and NotesSection
 * — each had copy-pasted this same helper after the Epic 3 extraction from
 * BacklogItemDetail.tsx.
 */ function ActionButtonLabel(param) {
    let { pending, label } = param;
    if (!pending) return /*#__PURE__*/ (0,jsx_runtime.jsx)(jsx_runtime.Fragment, {
        children: label
    });
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: buttonSpinner,
                "aria-hidden": "true"
            }),
            "Running…"
        ]
    });
}

;// ./src/lib/backlog/sessionKind.ts
/**
 * Classifies a LinkedSession's kind from its role and sessionId prefix.
 * Single source of truth for the ad hoc prefix-string checks that used to be
 * scattered across BacklogItemDetail.tsx's Sessions section — fixes the
 * pre-existing dead-link bug where a `manual-review-*` or `diff-error-*`
 * session (role "review", no recognized prefix in the old inline check)
 * fell through to a clickable `<a href="/?session=...">` even though it was
 * never Instance-backed.
 *
 * Check order matters: role "triage" or a "headless-" sessionId prefix wins
 * first, then the two blocked-guardrail prefixes, then the manual-review
 * prefix, then role "review", with "work" as the final fallback.
 */ function classifySessionKind(session) {
    if (session.role === "triage" || session.sessionId.startsWith("headless-")) {
        return "headless_diagnostic";
    }
    if (session.sessionId.startsWith("review-blocked-") || session.sessionId.startsWith("diff-error-")) {
        return "blocked_guardrail";
    }
    if (session.sessionId.startsWith("manual-review-")) {
        return "manual_review_marker";
    }
    if (session.role === "review") {
        return "review";
    }
    return "work";
}
/**
 * A LinkedSession is steerable iff it is Instance-backed ("work"/"review",
 * per classifySessionKind) and has not ended. Synthetic session kinds
 * (headless triage/review, blocked-guardrail, manual-review-marker) are
 * never steerable — no session.Instance was ever created for them. See
 * ADR-002 (project_plans/backlog-operator-feedback-loop/decisions/).
 */ function isSteerable(session) {
    const kind = classifySessionKind(session);
    return (kind === "work" || kind === "review") && !session.endedAt;
}

;// ./src/components/backlog/detail/PullRequestSection.tsx
/* __next_internal_client_entry_do_not_use__ prStatusLabel,PullRequestSection auto */ 






/** PR status line derived from open stuck reasons; falls back to the neutral "waiting for merge". */ function prStatusLabel() {
    let stuckReasons = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : [];
    if (stuckReasons.includes(backlog_pb/* StuckReason */.Pwx.PR_NEEDS_FIX)) {
        return "needs fixes (failing checks, conflicts or review feedback)";
    }
    if (stuckReasons.includes(backlog_pb/* StuckReason */.Pwx.PR_READY_UNMERGED)) {
        return "ready to merge";
    }
    return "waiting for merge";
}
/**
 * "Pull Request" block — only rendered while `item.status === "pr_pending"`
 * (guard preserved verbatim, Story 3.1.2 Task 3.1.2c). Only rendered when
 * relevant, so always-expanded by default is correct here. This is the sole
 * data source for the PR URL/number text (D4) — VersionControlSection
 * (Story 3.1.4) opts its own VcsWidget out of the duplicate PR link via
 * `showPrLink={false}` when this section is also rendering.
 */ function PullRequestSection(param) {
    let { item, actionLoading, onMarkDone, readOnly = false, stuckReasons } = param;
    // Story 3.3.2, Task 3.3.2b: linkedSessions is in creation order (same
    // convention BacklogItemDetail.tsx's Jules dispatch-gate branch prefill
    // relies on). The newest linked session isn't necessarily the one that
    // opened this PR -- a review-gate session links *after* the work session
    // it reviews (see julesDispatchGate.ts's skipReviewGate flow), so `.at(-1)`
    // can return a "review" session and misattribute (or drop) the badge.
    // Only sessionKind "work" (classifySessionKind) actually produces a PR --
    // filter to those before taking the last one.
    const producingSession = item.linkedSessions.filter((s)=>classifySessionKind(s) === "work").at(-1);
    const producedByJules = (producingSession === null || producingSession === void 0 ? void 0 : producingSession.role) === "jules_work";
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(Collapsible/* CollapsibleSection */.i, {
        sectionKey: "pull-request",
        title: "Pull Request",
        defaultExpanded: true,
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: section,
            children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: reviewContextBox,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: reviewContextInfo,
                        children: item.prUrl ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    className: reviewContextLabel,
                                    children: [
                                        "PR #",
                                        item.prNumber,
                                        " — ",
                                        prStatusLabel(stuckReasons)
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: sessionRowMain,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)(GitHubBadge/* GitHubBadge */.J, {
                                            prNumber: item.prNumber,
                                            prUrl: item.prUrl
                                        }),
                                        producedByJules && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                            className: julesProvenanceMarker,
                                            "aria-label": "Opened by Jules",
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    "aria-hidden": "true",
                                                    children: "☁"
                                                }),
                                                " Jules"
                                            ]
                                        })
                                    ]
                                })
                            ]
                        }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: reviewContextLabel,
                            children: "PR pending — no URL recorded yet"
                        })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: BacklogItemDetail_css_actionButton,
                        onClick: onMarkDone,
                        disabled: actionLoading !== null || readOnly,
                        "aria-busy": actionLoading === "mark_done",
                        title: "Mark done manually (if PR already merged)",
                        "data-testid": "backlog-action-mark-done",
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionButtonLabel, {
                            pending: actionLoading === "mark_done",
                            label: "Mark Done"
                        })
                    })
                ]
            })
        })
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/circle-dot.mjs
var circle_dot = __webpack_require__(65248);
// EXTERNAL MODULE: ./src/components/backlog/externalRef.ts
var externalRef = __webpack_require__(34243);
// EXTERNAL MODULE: ./src/lib/backlog/sourceBadge.ts
var sourceBadge = __webpack_require__(82488);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2Fdetail%2FSourceSection.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xMzdkcW5tMCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeik7Cn0KLl8xMzdkcW5tMSB7CiAgZGlzcGxheTogaW5saW5lLWZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MnkpOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgZm9udC13ZWlnaHQ6IHZhcigtLV8xOWV6MnJ6M2cpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyem0pOwogIHRleHQtZGVjb3JhdGlvbjogbm9uZTsKICBhbGlnbi1zZWxmOiBmbGV4LXN0YXJ0Owp9Ci5fMTM3ZHFubTE6aG92ZXIgewogIHRleHQtZGVjb3JhdGlvbjogdW5kZXJsaW5lOwp9Ci5fMTM3ZHFubTIgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC13cmFwOiB3cmFwOwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeSk7Cn0KLl8xMzdkcW5tMyB7CiAgZGlzcGxheTogaW5saW5lLWJsb2NrOwogIHBhZGRpbmc6IDFweCB2YXIoLS1fMTllejJyejJ5KTsKICBib3JkZXItcmFkaXVzOiB2YXIoLS1fMTllejJyejM2KTsKICBmb250LXNpemU6IDEwcHg7CiAgZm9udC13ZWlnaHQ6IDUwMDsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejgpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejEpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6ZSk7CiAgbWF4LXdpZHRoOiA4MHB4OwogIG92ZXJmbG93OiBoaWRkZW47CiAgdGV4dC1vdmVyZmxvdzogZWxsaXBzaXM7CiAgd2hpdGUtc3BhY2U6IG5vd3JhcDsKfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2Fdetail_2FSourceSection_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xMzdkcW5tMCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeik7Cn0KLl8xMzdkcW5tMSB7CiAgZGlzcGxheTogaW5saW5lLWZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MnkpOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgZm9udC13ZWlnaHQ6IHZhcigtLV8xOWV6MnJ6M2cpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyem0pOwogIHRleHQtZGVjb3JhdGlvbjogbm9uZTsKICBhbGlnbi1zZWxmOiBmbGV4LXN0YXJ0Owp9Ci5fMTM3ZHFubTE6aG92ZXIgewogIHRleHQtZGVjb3JhdGlvbjogdW5kZXJsaW5lOwp9Ci5fMTM3ZHFubTIgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC13cmFwOiB3cmFwOwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeSk7Cn0KLl8xMzdkcW5tMyB7CiAgZGlzcGxheTogaW5saW5lLWJsb2NrOwogIHBhZGRpbmc6IDFweCB2YXIoLS1fMTllejJyejJ5KTsKICBib3JkZXItcmFkaXVzOiB2YXIoLS1fMTllejJyejM2KTsKICBmb250LXNpemU6IDEwcHg7CiAgZm9udC13ZWlnaHQ6IDUwMDsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejgpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejEpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6ZSk7CiAgbWF4LXdpZHRoOiA4MHB4OwogIG92ZXJmbG93OiBoaWRkZW47CiAgdGV4dC1vdmVyZmxvdzogZWxsaXBzaXM7CiAgd2hpdGUtc3BhY2U6IG5vd3JhcDsKfQ_22_7D = __webpack_require__(83665);
;// ./src/components/backlog/detail/SourceSection.css.ts


var labelBadge = '_137dqnm3';
var SourceSection_css_labels = '_137dqnm2';
var SourceSection_css_link = '_137dqnm1';
var SourceSection_css_section = '_137dqnm0';
;// ./src/components/backlog/detail/SourceSection.tsx
/* __next_internal_client_entry_do_not_use__ SourceSection auto */ 
// +feature: backlog:item-detail-source-section
// lucide-react (this repo's pinned v1.14) ships no brand "Github" glyph —
// CircleDot is the closest available icon to GitHub's own issue-tracker
// mark and is used here instead of the `Github` icon plan.md's snippet
// assumed exists (see BacklogItemCard.tsx for the same substitution).





/**
 * "Source" section — provenance for a backlog item imported from an
 * external tracker (currently GitHub issues). Guard-at-call-site, matching
 * PullRequestSection's pattern: the parent (BacklogItemDetail.tsx) decides
 * whether to render this at all (`item.externalUrl` truthy), this component
 * never re-derives that guard itself.
 *
 * externalId is still optional in the type — nothing enforces that it always
 * accompanies a real externalUrl — so the issue number portion is rendered
 * defensively: only shown when externalId is actually present, to avoid a
 * literal "Issue #undefined".
 */ function SourceSection(param) {
    let { externalUrl, externalId, pluginId, labels, defaultExpanded } = param;
    const source = (0,sourceBadge/* getSourceBadgeConfig */.T2)(pluginId);
    const refLabel = externalId ? (0,sourceBadge/* buildExternalRefLabels */.c_)(pluginId, externalUrl, externalId).fullRef : "";
    const href = (0,externalRef/* safeExternalHref */.a)(externalUrl);
    const content = /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)(circle_dot/* default */.A, {
                "aria-hidden": "true",
                size: 14
            }),
            refLabel ? "Issue ".concat(refLabel) : "Issue"
        ]
    });
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(Collapsible/* CollapsibleSection */.i, {
        sectionKey: "source",
        title: "Source",
        defaultExpanded: defaultExpanded,
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: SourceSection_css_section,
            children: [
                href ? /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                    href: href,
                    target: "_blank",
                    rel: "noopener noreferrer",
                    className: SourceSection_css_link,
                    title: source.openLabel,
                    children: content
                }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    className: SourceSection_css_link,
                    children: content
                }),
                labels.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: SourceSection_css_labels,
                    children: labels.map((label)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: labelBadge,
                            title: label,
                            children: label
                        }, label))
                })
            ]
        })
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-markdown@10.1.0+5bba37e96b694e48/node_modules/react-markdown/lib/index.js + 114 modules
var lib = __webpack_require__(55206);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/remark-gfm@4.0.1+759ce506b1ed1a42/node_modules/remark-gfm/lib/index.js + 56 modules
var remark_gfm_lib = __webpack_require__(69896);
// EXTERNAL MODULE: ./src/components/backlog/markdownBody.css.ts
var markdownBody_css = __webpack_require__(14613);
;// ./src/components/backlog/detail/DescriptionSection.tsx
/* __next_internal_client_entry_do_not_use__ DescriptionSection auto */ 





/**
 * The item's markdown description — the field a user actually fills in, so
 * it seeds expanded by default. `defaultExpanded` seeds the initial
 * `useSectionExpandState`-backed value only; a stored per-item preference
 * can still collapse it.
 */ function DescriptionSection(param) {
    let { item, defaultExpanded } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(Collapsible/* CollapsibleSection */.i, {
        sectionKey: "description",
        title: "Description",
        defaultExpanded: defaultExpanded,
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: section,
            children: item.description ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: markdownBody_css/* markdownBody */.e,
                "data-testid": "backlog-description-rendered",
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(lib/* Markdown */.oz, {
                    remarkPlugins: [
                        remark_gfm_lib/* default */.A
                    ],
                    children: item.description
                })
            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: emptyText,
                children: "No description."
            })
        })
    });
}

// EXTERNAL MODULE: ./src/lib/backlog/itemActions.ts
var itemActions = __webpack_require__(5658);
// EXTERNAL MODULE: ./src/lib/backlog/planReviewStatus.ts
var planReviewStatus = __webpack_require__(57557);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FsharedFormStyles.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61Uy26DMBC88xW-VCoHR0CSpnU-JjLYwLbGpsbkQZV_rzCJAgaSSO0Jybs7uzOaYbELE_n5nq0D9OMhxKAqBT0RBFKA5DgV_Lj1EKICMonB8KIiKOHScN0-FyBxziHLDUGrVWlbY6UZ11hTBnVF0J7qV4x34QdvIt0sN37bkyppcAUNH9VjW09qXSlNUKnguqqkjIHM3IGo8V2I8LbicLnN6cj8rXf2FlfqoaUe0-Qr06qWjCCpJL9xISgsj6hSApiDlHXXKtEeOyyFzg6Sqz3X3aZOocmxfB4xcBEZVDQWnFlQVdIEzImgYLHuSyiVwVQIdeBsMB6NSA-3FfOHfPt9bTqt-sh9qrP40neG_sRmOXTv1bbtFzPQPDGgJGnp1IVsKxktR7ZwBF5ZzEdWfeSz5-yxtrsOwExOUBgEL266NlGXrvkYbO_KHdI7t8T-M2b_8v8r3rae0gLEaURkbzs0v4xzbSChYigWSVVSV51RatP-qNzITodLDEV_mzbNhDWsvOdfhpNsSCwFAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FsharedFormStyles_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61Uy26DMBC88xW_VCoHR0CSpnU_JjLYwLbGpsbkQZV_rzCJAgaSSO0Jybs7uzOaYbELE_n5nq0D9OMhxKAqBT0RBFKA5DgV_Lj1EKICMonB8KIiKOHScN0_FyBxziHLDUGrVWlbY6UZ11hTBnVF0J7qV4x34QdvIt0sN37bkyppcAUNH9VjW09qXSlNUKnguqqkjIHM3IGo8V2I8LbicLnN6cj8rXf2FlfqoaUe0_Qr06qWjCCpJL9xISgsj6hSApiDlHXXKtEeOyyFzg6Sqz3X3aZOocmxfB4xcBEZVDQWnFlQVdIEzImgYLHuSyiVwVQIdeBsMB6NSA_3FfOHfPt9bTqt_sh9qrP40neG_sRmOXTv1bbtFzPQPDGgJGnp1IVsKxktR7ZwBF5ZzEdWfeSz5_yxtrsOwExOUBgEL266NlGXrvkYbO_KHdI7t8T_M2b_8v8r3rae0gLEaURkbzs0v4xzbSChYigWSVVSV51RatP_qNzITodLDEV_mzbNhDWsvOdfhpNsSCwFAAA_22_7D = __webpack_require__(10782);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2Fdetail%2FSendBackFeedbackBox.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xOXNzc215MCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeik7Cn0%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2Fdetail_2FSendBackFeedbackBox_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xOXNzc215MCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeik7Cn0_22_7D = __webpack_require__(73709);
;// ./src/components/backlog/detail/SendBackFeedbackBox.css.ts



var SendBackFeedbackBox_css_form = '_1cnj8g53';
var SendBackFeedbackBox_css_formActions = '_1cnj8g56';
var SendBackFeedbackBox_css_formLabel = '_1cnj8g54';
var SendBackFeedbackBox_css_formTextarea = '_1cnj8g55';
var SendBackFeedbackBox_css_secondaryButton = '_1cnj8g51 _1cnj8g50';
var SendBackFeedbackBox_css_section = '_19sssmy0';
var submitButton = '_1cnj8g52 _1cnj8g50';
;// ./src/components/backlog/detail/SendBackFeedbackBox.tsx
/* __next_internal_client_entry_do_not_use__ SendBackFeedbackBox auto */ 
// +feature: backlog:send-back-feedback





// Renders null only when the action is ineligible (visible=false) AND
// there's no pending actionError AND no submit (incl. a Retry) is in
// flight — so a partial-failure error set just before a parent re-render
// still gets shown even after `visible` flips false, and a Retry launched
// from a `visible=false`/no-error state stays rendered for the duration of
// its own pending phase instead of blanking. See the early-return guard below.
function SendBackFeedbackBox(param) {
    let { visible, activeWorkSessionCount, disabled = false, actionPending = false, onSubmit } = param;
    const [showForm, setShowForm] = (0,react.useState)(false);
    const [feedback, setFeedback] = (0,react.useState)("");
    const [localPending, setLocalPending] = (0,react.useState)(false);
    const [actionError, setActionError] = (0,react.useState)(null);
    const isPending = localPending || actionPending;
    const canSubmit = feedback.trim().length > 0 && !isPending;
    const toggleRef = (0,react.useRef)(null);
    const textareaRef = (0,react.useRef)(null);
    // Mirrors BacklogItemDetail.tsx's mountedRef convention: guards the four
    // setState calls below handleSubmit's `await onSubmit(...)` against firing
    // after this box has unmounted mid-request.
    const isMountedRef = (0,react.useRef)(true);
    (0,react.useEffect)(()=>{
        return ()=>{
            isMountedRef.current = false;
        };
    }, []);
    (0,react.useEffect)(()=>{
        var _textareaRef_current;
        if (showForm) (_textareaRef_current = textareaRef.current) === null || _textareaRef_current === void 0 ? void 0 : _textareaRef_current.focus();
    }, [
        showForm
    ]);
    // Stays mounted while `isPending` too (not just while there's an
    // actionError): the row-4 Retry clears actionError before onSubmit
    // resolves, while `visible` can still be false at that instant — without
    // this, the guard would unmount the in-flight Retry UI.
    if (!visible && !actionError && !isPending) return null;
    function handleCancel() {
        var _toggleRef_current;
        // Gated on isPending, matching Submit's existing gating: without this,
        // a click or Escape mid-submit visually aborts the form while the
        // in-flight request keeps running unseen.
        if (isPending) return;
        setShowForm(false);
        setFeedback("");
        setActionError(null); // lets the component unmount on its next render once !visible (see the guard above)
        (_toggleRef_current = toggleRef.current) === null || _toggleRef_current === void 0 ? void 0 : _toggleRef_current.focus();
    }
    function handleTextareaKeyDown(e) {
        if (e.key === "Escape") handleCancel();
    }
    // Retry re-invokes the exact same submit path: handleSubmit already reads
    // the current `feedback` state and delegates to `onSubmit`, which reads
    // the parent's current item status — no separate retry codepath needed.
    function handleRetry() {
        void handleSubmit();
    }
    async function handleSubmit() {
        if (!canSubmit) return;
        setLocalPending(true);
        // Clear any stale error from a prior attempt before this one starts —
        // load-bearing for Retry, which calls handleSubmit directly.
        setActionError(null);
        try {
            await onSubmit(feedback);
            if (!isMountedRef.current) return;
            setShowForm(false);
            setFeedback("");
        } catch (err) {
            if (isMountedRef.current) setActionError(deriveSendBackErrorCopy(err));
            console.error(err);
        } finally{
            if (isMountedRef.current) setLocalPending(false);
        }
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: SendBackFeedbackBox_css_section,
        children: [
            visible && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                ref: toggleRef,
                className: SendBackFeedbackBox_css_secondaryButton,
                "aria-expanded": showForm,
                disabled: disabled || isPending,
                onClick: ()=>setShowForm((prev)=>!prev),
                "data-testid": "backlog-action-send-back-feedback",
                children: "↩ Send back for re-planning"
            }),
            showForm && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                role: "form",
                "aria-label": "Send back for re-planning",
                className: SendBackFeedbackBox_css_form,
                children: [
                    activeWorkSessionCount > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)(InlineNotice/* InlineNotice */.w, {
                        message: "This item has an active session — submitting this will stop it, and its work may be superseded once re-planning completes.",
                        "data-testid": "send-back-active-session-notice"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                        htmlFor: "send-back-feedback",
                        className: SendBackFeedbackBox_css_formLabel,
                        children: "What should change? (required)"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("textarea", {
                        id: "send-back-feedback",
                        ref: textareaRef,
                        "data-testid": "send-back-feedback-textarea",
                        rows: 3,
                        placeholder: "e.g. missed the mobile case, re-check the auth approach",
                        value: feedback,
                        onChange: (e)=>setFeedback(e.target.value),
                        onKeyDown: handleTextareaKeyDown,
                        className: SendBackFeedbackBox_css_formTextarea
                    }),
                    actionError && /*#__PURE__*/ (0,jsx_runtime.jsx)(InlineError, {
                        type: "transient",
                        headline: actionError.headline,
                        onDismiss: ()=>setActionError(null),
                        customMessage: actionError.message,
                        // Wired only for the retryable=true (idea-landing) case — the
                        // other branches never set retryable, preserving ux.md's
                        // "no misleading retry" discipline for those.
                        onRetry: actionError.retryable ? handleRetry : undefined,
                        retryAriaLabel: "Retry send-back with this feedback"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: SendBackFeedbackBox_css_formActions,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: SendBackFeedbackBox_css_secondaryButton,
                                onClick: handleCancel,
                                disabled: isPending,
                                "aria-disabled": isPending,
                                children: "Cancel"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: submitButton,
                                "aria-disabled": !canSubmit,
                                disabled: !canSubmit,
                                "aria-busy": isPending,
                                onClick: ()=>void handleSubmit(),
                                "data-testid": "backlog-action-send-back-feedback-submit",
                                children: isPending ? "Sending…" : "Submit"
                            })
                        ]
                    })
                ]
            })
        ]
    });
}

;// ./src/components/backlog/detail/ActionsSection.tsx
/* __next_internal_client_entry_do_not_use__ ActionsSection auto */ 






/**
 * The full status-conditional action-button block, including the inline
 * manual-review form — extracted verbatim from BacklogItemDetail.tsx
 * (Story 3.1.3, Task 3.1.3b). Always visible (primary content, not
 * progressive disclosure) — no Collapsible wrapper. Every data-testid is
 * preserved exactly: manual-review-form, manual-review-outcome,
 * manual-review-summary, manual-review-submit, backlog-action-ship-pr,
 * backlog-action-override-done, backlog-action-re-review,
 * backlog-action-manual-review, backlog-action-restart-session, and the
 * rest of the per-status action buttons.
 */ function ActionsSection(param) {
    let { item, actionLoading, latestWorkSession, showManualReview, manualReviewOutcome, manualReviewSummary, onAction, onManualReviewOutcomeChange, onManualReviewSummaryChange, onManualReviewSubmit, onManualReviewCancel, terminalState, julesDispatchGate, onDispatchToJulesClick, activeWorkSessionCount, onSendBackWithFeedback } = param;
    // getAvailableActions (web-app/src/lib/backlog/itemActions.ts) is the single
    // source of truth for which actions this item's current status + gate flags
    // expose — see its doc comment for why this replaced a scattered set of
    // per-status JSX conditionals, some of which (approve_plan) used to key off
    // incidental data (planArtifactsPath) instead of the real gate condition
    // (docs/tasks/backlog-feature-improvement.md's 2026-08-03 entry, item
    // be676dab). Everything below this line is transient per-click UI state
    // (actionLoading, AC-criteria-empty tooltips, etc.) that stays local.
    const { actions } = (0,itemActions/* getAvailableActions */.tE)(item);
    // Autonomous mode does its own planning — no plan-approval gate needed,
    // just the ready-status check `actions.has("spawn_session_autonomous")`
    // already encodes.
    const canRunAutonomously = actions.has("spawn_session_autonomous");
    // Task 4.3.1c: reuse the single derivePlanReviewStatus source of truth
    // (also used by PlanVerdictBox) instead of re-deriving the raw
    // skipPlanning/planApproved check here — pure internal-logic refactor,
    // behaviorally identical for every case that mattered before (a rejected
    // plan was never approved, so it already blocked spawn; adding
    // "changes_requested" as its own state doesn't loosen or tighten the gate).
    const planStatus = (0,planReviewStatus/* derivePlanReviewStatus */.V)(item);
    const canSpawnSession = actions.has("spawn_session") && (planStatus === "skipped" || planStatus === "approved");
    // Self-service "Ship PR" action: only makes sense for an item sitting in
    // review with no PR yet — the exact gap this closes (see
    // docs/tasks/backlog-feature-improvement.md, 2026-07-18 update). All AC
    // criteria must be complete before shipping; a gate verdict of PASS is
    // encouraged (via the button's title) but not required — same
    // human-override philosophy as the existing "Override → Done" action.
    const acAllComplete = item.acCriteria.length > 0 && item.acCriteria.every((c)=>c.status === "done");
    var _julesDispatchGate_reason;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: section,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                className: sectionTitle,
                children: "Actions"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: actionsPanel,
                role: "group",
                "aria-label": "Item actions",
                children: terminalState ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)(InlineNotice/* InlineNotice */.w, {
                            message: terminalState === "archived" ? "This item was archived elsewhere." : "This item was removed elsewhere.",
                            "data-testid": "backlog-detail-terminal-notice"
                        }),
                        terminalState === "archived" && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: BacklogItemDetail_css_actionButton,
                            onClick: ()=>onAction("unarchive"),
                            disabled: actionLoading !== null,
                            "aria-busy": actionLoading === "unarchive",
                            title: "Restores the item to the Idea column. Its git worktree, if any, was deleted at archive time and cannot be recreated.",
                            "data-testid": "backlog-action-unarchive",
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionButtonLabel, {
                                pending: actionLoading === "unarchive",
                                label: "Unarchive"
                            })
                        })
                    ]
                }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                    children: [
                        actions.has("mark_ready") && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: BacklogItemDetail_css_actionButton,
                            onClick: ()=>onAction("mark_ready"),
                            disabled: actionLoading !== null || item.acCriteria.length === 0,
                            "aria-disabled": item.acCriteria.length === 0,
                            "aria-busy": actionLoading === "mark_ready",
                            title: item.acCriteria.length === 0 ? "Add at least one AC criterion first" : undefined,
                            "data-testid": "backlog-action-mark-ready",
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionButtonLabel, {
                                pending: actionLoading === "mark_ready",
                                label: "Mark Ready"
                            })
                        }),
                        actions.has("trigger_triage") && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: BacklogItemDetail_css_actionButton,
                            onClick: ()=>onAction("trigger_triage"),
                            disabled: actionLoading !== null || !item.repoPath,
                            "aria-disabled": !item.repoPath,
                            "aria-busy": actionLoading === "trigger_triage",
                            title: !item.repoPath ? "Set repository path first" : undefined,
                            "data-testid": "backlog-action-trigger-triage",
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionButtonLabel, {
                                pending: actionLoading === "trigger_triage",
                                label: "Trigger Triage"
                            })
                        }),
                        actions.has("spawn_session") && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: BacklogItemDetail_css_actionButton,
                            onClick: ()=>onAction("spawn_session"),
                            disabled: actionLoading !== null || !canSpawnSession,
                            "aria-disabled": !canSpawnSession,
                            "aria-busy": actionLoading === "spawn_session",
                            title: !canSpawnSession ? "Approve the plan or enable skip_planning to spawn a session" : undefined,
                            "data-testid": "backlog-action-spawn-session",
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionButtonLabel, {
                                pending: actionLoading === "spawn_session",
                                label: "Spawn Session"
                            })
                        }),
                        actions.has("spawn_session_autonomous") && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: BacklogItemDetail_css_actionButton,
                            onClick: ()=>onAction("spawn_session_autonomous"),
                            disabled: actionLoading !== null || !canRunAutonomously,
                            "aria-disabled": !canRunAutonomously,
                            "aria-busy": actionLoading === "spawn_session_autonomous",
                            title: !canRunAutonomously ? "Item must be in Ready status to run autonomously" : "Run the agent without human approval for tool calls",
                            "data-testid": "backlog-action-run-autonomously",
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionButtonLabel, {
                                pending: actionLoading === "spawn_session_autonomous",
                                label: "Run Autonomously"
                            })
                        }),
                        julesDispatchGate && !julesDispatchGate.hidden && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: BacklogItemDetail_css_actionButton,
                                    onClick: onDispatchToJulesClick,
                                    disabled: actionLoading !== null || julesDispatchGate.disabled,
                                    "aria-describedby": julesDispatchGate.reason ? "jules-dispatch-gate-reason" : undefined,
                                    title: (_julesDispatchGate_reason = julesDispatchGate.reason) !== null && _julesDispatchGate_reason !== void 0 ? _julesDispatchGate_reason : undefined,
                                    "data-testid": "dispatch-to-jules",
                                    children: "Dispatch to Jules"
                                }),
                                julesDispatchGate.reason && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                    id: "jules-dispatch-gate-reason",
                                    className: emptyText,
                                    "data-testid": "dispatch-to-jules-reason",
                                    children: julesDispatchGate.reason
                                })
                            ]
                        }),
                        actions.has("approve_plan") && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: BacklogItemDetail_css_actionButton,
                            onClick: ()=>onAction("approve_plan"),
                            disabled: actionLoading !== null,
                            "aria-busy": actionLoading === "approve_plan",
                            title: item.status === "queued" ? "Queued items can't be dequeued until their plan is approved (or skip_planning is set)." : undefined,
                            "data-testid": "backlog-action-approve-plan",
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionButtonLabel, {
                                pending: actionLoading === "approve_plan",
                                label: "Approve Plan"
                            })
                        }),
                        actions.has("retry_triage") && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: BacklogItemDetail_css_actionButton,
                            onClick: ()=>onAction("retry_triage"),
                            disabled: actionLoading !== null,
                            "aria-busy": actionLoading === "retry_triage",
                            title: "This item's most recent triage session ended without producing a usable plan — retry to generate one.",
                            "data-testid": "backlog-action-retry-triage",
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionButtonLabel, {
                                pending: actionLoading === "retry_triage",
                                label: "Retry Triage"
                            })
                        }),
                        actions.has("view_session") && /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                            className: BacklogItemDetail_css_actionButton,
                            href: "/?session=".concat((latestWorkSession !== null && latestWorkSession !== void 0 ? latestWorkSession : item.linkedSessions[item.linkedSessions.length - 1]).sessionId),
                            "data-testid": "backlog-action-view-session",
                            children: "View Session"
                        }),
                        actions.has("ship_pr") && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: BacklogItemDetail_css_actionButton,
                            onClick: ()=>onAction("ship_pr"),
                            disabled: actionLoading !== null || !acAllComplete,
                            "aria-disabled": !acAllComplete,
                            "aria-busy": actionLoading === "ship_pr",
                            title: !acAllComplete ? "All acceptance criteria must be complete before shipping a PR." : "Ask the agent to push the branch and open a pull request for this item.",
                            "data-testid": "backlog-action-ship-pr",
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionButtonLabel, {
                                pending: actionLoading === "ship_pr",
                                label: "\uD83D\uDE80 Ship PR"
                            })
                        }),
                        actions.has("override_done") && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: "".concat(BacklogItemDetail_css_actionButton, " ").concat(actionButtonDanger),
                            onClick: ()=>onAction("override_done"),
                            disabled: actionLoading !== null,
                            "aria-busy": actionLoading === "override_done",
                            "data-testid": "backlog-action-override-done",
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionButtonLabel, {
                                pending: actionLoading === "override_done",
                                label: "Override → Done"
                            })
                        }),
                        actions.has("re_review") && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: BacklogItemDetail_css_actionButton,
                            onClick: ()=>onAction("re_review"),
                            disabled: actionLoading !== null,
                            "aria-busy": actionLoading === "re_review",
                            "data-testid": "backlog-action-re-review",
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionButtonLabel, {
                                pending: actionLoading === "re_review",
                                label: "Re-review"
                            })
                        }),
                        actions.has("manual_review") && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: BacklogItemDetail_css_actionButton,
                            onClick: ()=>onAction("manual_review"),
                            disabled: actionLoading !== null,
                            "data-testid": "backlog-action-manual-review",
                            children: "Submit Review"
                        }),
                        actions.has("restart_session") && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: BacklogItemDetail_css_actionButton,
                            onClick: ()=>onAction("restart_session"),
                            disabled: actionLoading !== null,
                            "aria-busy": actionLoading === "restart_session",
                            title: item.status === "review" ? "Stop the review session and restart work from scratch in a fresh git worktree" : "Stop the current session and re-spawn it in a fresh git worktree",
                            "data-testid": "backlog-action-restart-session",
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionButtonLabel, {
                                pending: actionLoading === "restart_session",
                                label: "Restart"
                            })
                        }),
                        showManualReview && item.status === "review" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: manualReviewForm,
                            "data-testid": "manual-review-form",
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("h4", {
                                    className: manualReviewTitle,
                                    children: "Submit Review"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: manualReviewRow,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                            className: manualReviewLabel,
                                            children: "Verdict"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                                            className: manualReviewSelect,
                                            value: manualReviewOutcome,
                                            onChange: (e)=>onManualReviewOutcomeChange(e.target.value),
                                            "data-testid": "manual-review-outcome",
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                    value: "PASS",
                                                    children: "PASS — meets all criteria"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                    value: "FAIL",
                                                    children: "FAIL — does not meet criteria"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                    value: "PARTIAL",
                                                    children: "PARTIAL — partially meets criteria"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                    value: "UNVERIFIABLE",
                                                    children: "UNVERIFIABLE — cannot verify"
                                                })
                                            ]
                                        })
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: manualReviewRow,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                            className: manualReviewLabel,
                                            children: "Summary"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("textarea", {
                                            className: manualReviewTextarea,
                                            placeholder: "Describe your findings…",
                                            value: manualReviewSummary,
                                            onChange: (e)=>onManualReviewSummaryChange(e.target.value),
                                            rows: 4,
                                            "data-testid": "manual-review-summary"
                                        })
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: manualReviewActions,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                            className: BacklogItemDetail_css_actionButton,
                                            disabled: !manualReviewSummary.trim() || actionLoading !== null,
                                            "aria-busy": actionLoading === "manual_review_submit",
                                            onClick: onManualReviewSubmit,
                                            "data-testid": "manual-review-submit",
                                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionButtonLabel, {
                                                pending: actionLoading === "manual_review_submit",
                                                label: "Submit"
                                            })
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                            className: actionButtonSecondary,
                                            onClick: onManualReviewCancel,
                                            "data-testid": "manual-review-cancel",
                                            children: "Cancel"
                                        })
                                    ]
                                })
                            ]
                        }),
                        actions.has("archive") && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: BacklogItemDetail_css_actionButton,
                            onClick: ()=>onAction("archive"),
                            disabled: actionLoading !== null,
                            "aria-busy": actionLoading === "archive",
                            "data-testid": "backlog-action-archive",
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionButtonLabel, {
                                pending: actionLoading === "archive",
                                label: "Archive"
                            })
                        }),
                        actions.has("reopen") && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: BacklogItemDetail_css_actionButton,
                            onClick: ()=>onAction("reopen"),
                            disabled: actionLoading !== null,
                            "aria-busy": actionLoading === "reopen",
                            "data-testid": "backlog-action-reopen",
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionButtonLabel, {
                                pending: actionLoading === "reopen",
                                label: "Re-open to Review"
                            })
                        }),
                        actions.has("unarchive") && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: BacklogItemDetail_css_actionButton,
                            onClick: ()=>onAction("unarchive"),
                            disabled: actionLoading !== null,
                            "aria-busy": actionLoading === "unarchive",
                            title: "Restores the item to the Idea column. Its git worktree, if any, was deleted at archive time and cannot be recreated.",
                            "data-testid": "backlog-action-unarchive",
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionButtonLabel, {
                                pending: actionLoading === "unarchive",
                                label: "Unarchive"
                            })
                        }),
                        actions.has("send_back_idea") && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: "".concat(BacklogItemDetail_css_actionButton, " ").concat(actionButtonSecondary),
                            onClick: ()=>onAction("send_back_idea"),
                            disabled: actionLoading !== null,
                            "aria-busy": actionLoading === "send_back_idea",
                            title: "Reset to Idea and clear plan approval so triage can re-run",
                            "data-testid": "backlog-action-send-back-idea",
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionButtonLabel, {
                                pending: actionLoading === "send_back_idea",
                                label: "↩ Return to Triage"
                            })
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)(SendBackFeedbackBox, {
                            visible: actions.has("send_back_ready"),
                            activeWorkSessionCount: activeWorkSessionCount,
                            disabled: actionLoading !== null && actionLoading !== "send_back_ready",
                            actionPending: actionLoading === "send_back_ready",
                            onSubmit: onSendBackWithFeedback
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: actionButtonDanger,
                            onClick: ()=>onAction("delete"),
                            disabled: actionLoading !== null,
                            "aria-busy": actionLoading === "delete",
                            "data-testid": "backlog-action-delete",
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionButtonLabel, {
                                pending: actionLoading === "delete",
                                label: "Delete"
                            })
                        })
                    ]
                })
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FPlanVerdictBox.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA62Vy27DIBRE9_kKls2CKHZeDf6YCJtrmxYDApyHq_57ZZRIwU5i1HblxYzODJeHF4fkKEFIvkRfM4QYt1rQC0GlgHM2Q_6LGTdQOK4kQYUSbSN7paKaoCM1bxgfkj10qelWyTybfc8WN2bimaWSDlvewciez7ObfgJe1W7kqL2jUEKZoZZ6ycHZYWeotKUyDUGt1mAKaqEXBTgHBltNCy4rgpaL5QaaXmmoqbjEuXJONSPyJVxG6peRK8PAYEMZb-2o6M630ZQxnzRQl169EgSUjqC1PiOrBGfZn8eedmHf1X3fPg2_GGBOi8_KqFayob4Lqes4ajNBTQbYTRy2ncAeQ-o2jpqsp9puQu5uxLXuIoAgRm0NLIsJrScyWRj5_vhiUsEribmDxhJUgHRgIo_H3gOfHok7J33uTPahNX9ubSdvOQ9ZxYvYbWhl0WuBibeIRrX89YPjwSVtuLiMirZh0zJ6TVX8_tSv9-fOyeOn_xHd9DPqT_A41EunmjvwgwWCtAF8MlQHEeJf7sn16e3pBF0zfgBxizg1JAcAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FPlanVerdictBox_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA62Vy27DIBRE9_kKls2CKHZeDf6YCJtrmxYDApyHq_57ZZRIwU5i1HblxYzODJeHF4fkKEFIvkRfM4QYt1rQC0GlgHM2Q_6LGTdQOK4kQYUSbSN7paKaoCM1bxgfkj10qelWyTybfc8WN2bimaWSDlvewciez7ObfgJe1W7kqL2jUEKZoZZ6ycHZYWeotKUyDUGt1mAKaqEXBTgHBltNCy4rgpaL5QaaXmmoqbjEuXJONSPyJVxG6peRK8PAYEMZb_2o6M630ZQxnzRQl169EgSUjqC1PiOrBGfZn8eedmHf1X3fPg2_GGBOi8_KqFayob4Lqes4ajNBTQbYTRy2ncAeQ_o2jpqsp9puQu5uxLXuIoAgRm0NLIsJrScyWRj5_vhiUsEribmDxhJUgHRgIo_H3gOfHok7J33uTPahNX9ubSdvOQ9ZxYvYbWhl0WuBibeIRrX89YPjwSVtuLiMirZh0zJ6TVX8_tSv9_fOyeOn_xHd9DPqT_A41EunmjvwgwWCtAF8MlQHEeJf7sn16e3pBF0zfgBxizg1JAcAAA_22_7D = __webpack_require__(67000);
;// ./src/components/backlog/PlanVerdictBox.css.ts



var PlanVerdictBox_css_actions = '_1vnelnil';
var cardApproved = '_1vnelni5 _1vnelni2';
var cardChangesRequested = '_1vnelni6 _1vnelni2';
var cardHeader = '_1vnelni8';
var cardNoPlan = '_1vnelni3 _1vnelni2';
var cardPendingReview = '_1vnelni4 _1vnelni2';
var cardSkipped = '_1vnelni7 _1vnelni2';
var PlanVerdictBox_css_form = '_1cnj8g53';
var PlanVerdictBox_css_formActions = '_1cnj8g56';
var PlanVerdictBox_css_formLabel = '_1cnj8g54';
var PlanVerdictBox_css_formTextarea = '_1cnj8g55';
var iconApproved = '_1vnelnib';
var iconChangesRequested = '_1vnelnic';
var iconNoPlan = '_1vnelni9';
var iconPendingReview = '_1vnelnia';
var iconSkipped = '_1vnelnid';
var PlanVerdictBox_css_label = '_1vnelnie';
var labelApproved = '_1vnelnih';
var labelChangesRequested = '_1vnelnii';
var labelNoPlan = '_1vnelnif';
var labelPendingReview = '_1vnelnig';
var labelSkipped = '_1vnelnij';
var PlanVerdictBox_css_primaryButton = '_1cnj8g52 _1cnj8g50';
var reasonText = '_1vnelnik';
var PlanVerdictBox_css_secondaryButton = '_1cnj8g51 _1cnj8g50';
var PlanVerdictBox_css_section = '_1vnelni0';
var PlanVerdictBox_css_sectionTitle = '_1vnelni1';
;// ./src/components/backlog/PlanVerdictBox.tsx
/* __next_internal_client_entry_do_not_use__ PlanVerdictBox auto */ 
// +feature: backlog-plan-verdict-box



const STATUS_CONFIG = {
    no_plan: {
        icon: "○",
        label: "No plan yet",
        cardClass: cardNoPlan,
        iconClass: iconNoPlan,
        labelClass: labelNoPlan
    },
    pending_review: {
        icon: "◌",
        label: "Pending review",
        cardClass: cardPendingReview,
        iconClass: iconPendingReview,
        labelClass: labelPendingReview
    },
    approved: {
        icon: "✓",
        label: "Plan approved",
        cardClass: cardApproved,
        iconClass: iconApproved,
        labelClass: labelApproved
    },
    changes_requested: {
        icon: "✎",
        label: "Revisions requested",
        cardClass: cardChangesRequested,
        iconClass: iconChangesRequested,
        labelClass: labelChangesRequested
    },
    skipped: {
        icon: "⊘",
        label: "Planning skipped",
        cardClass: cardSkipped,
        iconClass: iconSkipped,
        labelClass: labelSkipped
    }
};
/**
 * Story 4.2.1: read-only status card + reject-with-reason form for the
 * plan-review lifecycle. Modeled on GateVerdictBox's toggle/form/focus
 * pattern. Does NOT render an Approve action — ActionsSection already owns
 * that button (avoid duplicating the same action in two places); this
 * component's own write action is "Request Changes" (and, once rejected,
 * "Regenerate Plan with This Feedback" per ADR-002).
 */ function PlanVerdictBox(param) {
    let { status, rejectionReason, readOnly = false, actionPending = false, onReject, onRegenerateWithFeedback } = param;
    const config = STATUS_CONFIG[status];
    const [showReject, setShowReject] = (0,react.useState)(false);
    const [reason, setReason] = (0,react.useState)("");
    const [localPending, setLocalPending] = (0,react.useState)(false);
    const [regeneratePending, setRegeneratePending] = (0,react.useState)(false);
    const [actionError, setActionError] = (0,react.useState)(null);
    const [actionErrorHeadline, setActionErrorHeadline] = (0,react.useState)("Action failed");
    const isPending = localPending || actionPending;
    const canSubmit = reason.trim().length > 0 && !isPending;
    const toggleRef = (0,react.useRef)(null);
    const textareaRef = (0,react.useRef)(null);
    // Focus the reject-reason textarea when the form opens — mirrors
    // GateVerdictBox's override-form focus pattern.
    (0,react.useEffect)(()=>{
        if (showReject) {
            var _textareaRef_current;
            (_textareaRef_current = textareaRef.current) === null || _textareaRef_current === void 0 ? void 0 : _textareaRef_current.focus();
        }
    }, [
        showReject
    ]);
    function handleCancel() {
        var _toggleRef_current;
        setShowReject(false);
        setReason("");
        (_toggleRef_current = toggleRef.current) === null || _toggleRef_current === void 0 ? void 0 : _toggleRef_current.focus();
    }
    function handleTextareaKeyDown(e) {
        if (e.key === "Escape") {
            handleCancel();
        }
    }
    async function handleSubmit() {
        if (!onReject || !canSubmit) return;
        setLocalPending(true);
        try {
            await onReject(reason);
            setShowReject(false);
            setReason("");
        } catch (err) {
            setActionErrorHeadline("Failed to request changes");
            setActionError("Action failed. Please try again.");
            console.error(err);
        } finally{
            setLocalPending(false);
        }
    }
    async function handleRegenerate() {
        if (!onRegenerateWithFeedback) return;
        setRegeneratePending(true);
        try {
            await onRegenerateWithFeedback();
        } catch (err) {
            setActionErrorHeadline("Failed to regenerate plan");
            setActionError("Action failed. Please try again.");
            console.error(err);
        } finally{
            setRegeneratePending(false);
        }
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
        role: "status",
        "aria-live": "polite",
        "aria-atomic": "true",
        "aria-label": "Plan review status",
        className: PlanVerdictBox_css_section,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: PlanVerdictBox_css_sectionTitle,
                children: "Plan Review"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: config.cardClass,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: cardHeader,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: config.iconClass,
                                "aria-hidden": "true",
                                children: config.icon
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: "".concat(PlanVerdictBox_css_label, " ").concat(config.labelClass),
                                children: config.label
                            })
                        ]
                    }),
                    status === "changes_requested" && rejectionReason && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: reasonText,
                        children: rejectionReason
                    })
                ]
            }),
            !readOnly && status === "changes_requested" && onRegenerateWithFeedback && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: PlanVerdictBox_css_actions,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    className: PlanVerdictBox_css_primaryButton,
                    onClick: ()=>void handleRegenerate(),
                    disabled: regeneratePending,
                    "aria-busy": regeneratePending,
                    "data-testid": "backlog-action-regenerate-plan",
                    children: regeneratePending ? "Regenerating…" : "Regenerate Plan with This Feedback"
                })
            }),
            actionError && // Dismiss-only: neither reject-plan nor regenerate-plan failure has
            // a wireable retry here (the user re-triggers the action via the
            // still-open form/button below), so offering a "Retry" that just
            // clears state would misrepresent what the button does.
            /*#__PURE__*/ (0,jsx_runtime.jsx)(InlineError, {
                type: "transient",
                headline: actionErrorHeadline,
                onDismiss: ()=>setActionError(null),
                customMessage: actionError
            }),
            !readOnly && onReject && (status === "pending_review" || status === "changes_requested") && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        ref: toggleRef,
                        className: PlanVerdictBox_css_secondaryButton,
                        "aria-expanded": showReject,
                        onClick: ()=>setShowReject((prev)=>!prev),
                        "data-testid": "backlog-action-reject-plan",
                        children: "Request Changes"
                    }),
                    showReject && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        role: "form",
                        "aria-label": "Request changes to plan",
                        className: PlanVerdictBox_css_form,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                htmlFor: "plan-reject-reason",
                                className: PlanVerdictBox_css_formLabel,
                                children: "What should change? (required)"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("textarea", {
                                id: "plan-reject-reason",
                                ref: textareaRef,
                                "data-testid": "plan-reject-reason",
                                rows: 3,
                                placeholder: "Explain what should be different about this plan…",
                                value: reason,
                                onChange: (e)=>setReason(e.target.value),
                                onKeyDown: handleTextareaKeyDown,
                                className: PlanVerdictBox_css_formTextarea
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: PlanVerdictBox_css_formActions,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        className: PlanVerdictBox_css_secondaryButton,
                                        onClick: handleCancel,
                                        children: "Cancel"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        className: PlanVerdictBox_css_primaryButton,
                                        "aria-disabled": !canSubmit,
                                        disabled: !canSubmit,
                                        "aria-busy": isPending,
                                        onClick: ()=>void handleSubmit(),
                                        "data-testid": "backlog-action-reject-plan-submit",
                                        children: isPending ? "Requesting…" : "Submit"
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

;// ./src/lib/backlog/julesDispatchGate.ts
/**
 * Single source of truth for Story 3.2.2's "Dispatch to Jules" gating,
 * resolved entirely over GetJulesConfig + the item's already-loaded
 * sessions — no new RPC field. Called once from BacklogItemDetail.tsx and
 * passed down to the presentational ActionsSection, which does no gating
 * computation of its own (mirrors derivePlanReviewStatus's role for the
 * plan-review gate).
 *
 * Precedence order from ux.md §3.1 — only the first matching condition
 * applies, so e.g. "no key" and "no branch" both being true always reads as
 * "no key" alone, never both at once: feature off (hidden) -> no key ->
 * Jules session already open -> no known branch -> enabled. `branch` is the
 * newest non-empty worktreeBranch across the item's sessions, the same
 * value SessionsSection's per-row branch badge reads (§4.1) — sessions is
 * in creation order, so `.at(-1)` on the filtered list is the newest.
 */ function resolveJulesDispatchGate(julesConfig, sessions) {
    var _sessions_filter_at;
    if (!julesConfig || !julesConfig.enabled) return {
        hidden: true,
        disabled: true,
        reason: null,
        branch: ""
    };
    var _sessions_filter_at_worktreeBranch;
    const branch = (_sessions_filter_at_worktreeBranch = (_sessions_filter_at = sessions.filter((s)=>!!s.worktreeBranch).at(-1)) === null || _sessions_filter_at === void 0 ? void 0 : _sessions_filter_at.worktreeBranch) !== null && _sessions_filter_at_worktreeBranch !== void 0 ? _sessions_filter_at_worktreeBranch : "";
    if (!julesConfig.hasApiKey) {
        return {
            hidden: false,
            disabled: true,
            reason: "Add a Jules API key in Settings to enable cloud sessions.",
            branch
        };
    }
    const hasOpenJulesSession = sessions.some((s)=>s.role === "jules_work" && !s.endedAt);
    if (hasOpenJulesSession) {
        return {
            hidden: false,
            disabled: true,
            reason: "A Jules session is already running for this item.",
            branch
        };
    }
    if (!branch) {
        return {
            hidden: false,
            disabled: true,
            reason: "This item has no branch yet — spawn a local session (or push a branch) before dispatching to Jules.",
            branch
        };
    }
    return {
        hidden: false,
        disabled: false,
        reason: null,
        branch
    };
}

;// ./src/components/backlog/detail/PlanArtifactsSection.tsx
/* __next_internal_client_entry_do_not_use__ PlanArtifactsSection auto */ 


/** Collapsed by default (secondary info) — Story 3.1.4, Task 3.1.4a. */ function PlanArtifactsSection(param) {
    let { item, defaultExpanded } = param;
    if (!item.planArtifactsPath) return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(Collapsible/* CollapsibleSection */.i, {
        sectionKey: "plan-artifacts",
        title: "Plan Artifacts",
        defaultExpanded: defaultExpanded,
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: section,
            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                className: artifactsPath,
                children: item.planArtifactsPath
            })
        })
    });
}

// EXTERNAL MODULE: ./src/components/shared/VcsWidget.tsx + 20 modules
var VcsWidget = __webpack_require__(48704);
;// ./src/components/backlog/detail/VersionControlSection.tsx
/* __next_internal_client_entry_do_not_use__ VersionControlSection auto */ 



/**
 * Live VCS state for the most recent work session, falling back to the
 * durable ship-status check once the live worktree is gone. Extracted
 * verbatim from BacklogItemDetail.tsx (Story 3.1.4, Task 3.1.4b).
 *
 * D4 fix: `PullRequestSection` is the single data source for the PR URL
 * text when it's also rendering for the current status (`pr_pending`) —
 * this widget opts its own PR link out via `showPrLink={false}` only in
 * that case, so the link is still visible via this section alone for every
 * other status.
 *
 * When the widget has no GitHub block (ship-status missing or stale) the
 * item's own prUrl is rendered as a plain link rather than synthesizing PR
 * state the widget would present as fact (merged / CI pending).
 */ function VersionControlSection(param) {
    let { item, widgetData, activeSessionCount, worktreePath, defaultExpanded, onViewDiff, onBrowseFiles } = param;
    const showPrLink = item.status !== "pr_pending";
    const showItemPrLink = showPrLink && !!item.prUrl && !(widgetData === null || widgetData === void 0 ? void 0 : widgetData.github);
    if (!widgetData && !showItemPrLink) return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(Collapsible/* CollapsibleSection */.i, {
        sectionKey: "version-control",
        title: "Version Control",
        defaultExpanded: defaultExpanded,
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: section,
            children: [
                widgetData && /*#__PURE__*/ (0,jsx_runtime.jsx)(VcsWidget/* VcsWidget */.D, {
                    data: widgetData,
                    mode: "full",
                    onViewDiff: onViewDiff,
                    activeSessionCount: activeSessionCount,
                    worktreePath: worktreePath,
                    onBrowseFiles: onBrowseFiles,
                    showPrLink: showPrLink
                }),
                showItemPrLink && /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                    href: item.prUrl,
                    target: "_blank",
                    rel: "noopener noreferrer",
                    "data-testid": "vcs-section-pr-link",
                    children: item.prNumber ? "Pull request #".concat(item.prNumber) : "Pull request"
                })
            ]
        })
    });
}

;// ./src/lib/hooks/useShowMore.ts

function useShowMore_storageKey(itemId, sectionKey) {
    return "backlog-detail-showmore-".concat(itemId, "-").concat(sectionKey);
}
/**
 * Blocker C fix (research/features.md finding #5) + pre-mortem finding #2:
 * caps a long list's default rendering to the most recent `cap` entries,
 * revealing the rest via `showAll()`. The "show all" choice is
 * `localStorage`-backed (key `backlog-detail-showmore-${itemId}-${sectionKey}`,
 * same pattern/convention as `useSectionExpandState`) — NOT a plain
 * `useState` that resets to the capped view on every mount, since that
 * would make a heavily-cycled, chronically-stuck item (exactly the case
 * this project exists to make inspectable) re-pay the "Show N more" click
 * on every single re-open (Story 3.1.4, Task 3.1.4c2).
 *
 * Reads/writes are wrapped in try/catch, matching
 * `useSectionExpandState`'s defensive localStorage pattern.
 */ function useShowMore(itemId, sectionKey, items, cap) {
    const [showingAll, setShowingAll] = (0,react.useState)(()=>{
        try {
            return localStorage.getItem(useShowMore_storageKey(itemId, sectionKey)) === "true";
        } catch (e) {
            return false;
        }
    });
    const showAll = (0,react.useCallback)(()=>{
        setShowingAll(true);
        try {
            localStorage.setItem(useShowMore_storageKey(itemId, sectionKey), "true");
        } catch (e) {
        // Persistence is best-effort only — in-memory state still updates.
        }
    }, [
        itemId,
        sectionKey
    ]);
    // Defends against callers (and test mocks) that pass an undefined/null
    // list — e.g. a BacklogItem fixture that omits an optional notes field.
    const safeItems = items !== null && items !== void 0 ? items : [];
    const hasMore = safeItems.length > cap && !showingAll;
    const remaining = Math.max(safeItems.length - cap, 0);
    const visible = (0,react.useMemo)(()=>showingAll || safeItems.length <= cap ? safeItems : safeItems.slice(-cap), [
        safeItems,
        cap,
        showingAll
    ]);
    return {
        visible,
        hasMore,
        remaining,
        showAll
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/connect-error.js
var connect_error = __webpack_require__(45640);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/code.js
var code = __webpack_require__(16294);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FSessionMonitor.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA9VY227iMBB95yv8UmkrrVFCS2ndl_2TlYkniYsvWccphFX_fWWHhBBICJc-7BMKM0xmzhyfGfNrBWVsqIQcLVfBm5wF6O8EoeDhJwqD4ME_IKQzGnFbEhS-TxD6miA0P7YF0-fK-jWZVrFC78J4nglaEhQL2DgP94kZNxBZrhVBkRaFVM6S0IygT2p-YPw7fIPtzGxn20dnWWrDwBAUZhuUa8FZxw1aXthQxou8G-lp4X30J5hY6DVBKWcM1Ps-39npfKngicLcgswJikBZMMPJZpQxrpIT1m5KQZU2jVaJ0YVi3Z-wdllLba2Wwxg0tTz5WmKtLM75Fo7AoD6yt6-BJ6k98ki9R6SFNl1b6E0WNhZbQ1UeayMJKrIMTERzcEYB1oLBuWOHgyKYBnOQzTtjKrkojxAqHmuCeK411Tz7atac2ZSgl8y3Jd2lvXvs9H4ePAxDK5tX4Tw1XK0ICny3FZfUERMrKoHUx-LQxApDK_KG03l-aLNccpXguFA7fgPNAXOFdWEPPbmFKgyOdKEsQVzFXHELrcrnh5zkSnAF-ApqloPULLvUPDh3fSfqZdTZTI5I7kmTUQPK9nJsVtGvMLmzZZrXxY3i9AC_9tCS1ImBB7ifJ6_9xyBoR3s5Eydc9AeSbax7PZpXLa7hxP_W-XNt9urDINL1SVRatQ_O4q7dffVxasmZPQeV6NTDBJcE0cLq-6r_op3B217Qh8XzDG69tSK0TrkFL9lAUGYArw3NvEEbhpcG6Iog_4GpEF7lHfFqVMLp3H0nqUm48mraJE_vtQk0EZf3iFhpoqQbvBstbudpvSS60xA937WrZmmTJ_N5nhcO6PcL237xqLoHmHSKFxewy30zZjH6Dk0bPT6Si7bFjyK3PC5xpJUFN-v3pganoNpZbhDFym5LAQRxSwWPWgmnlx6Zy0vp34lHFRkG46XfJzdiwD0d6Dj3GIzmzQmJ7kywcyNwV9IY5t0CydGetK949BxsfvJxmih3vPBUaFidnb3RDQ7HHoo2hazGF1KevA40kcRlvPnWHeqUEg5c075hhR4C6h5r1_AunLa5KqtRVd8Yr-hR_8Y-MObC5ajmrS4bOze1RBfWjd2jRViSWEdFXnVkAFbRhlVdp5MX8L3K8vwN_WSuf25ids-ulgwzW41itjpAkTCe06WAakHr_lVW56-0G2dCr4G9T77-AQ9k5zqTEwAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FSessionMonitor_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9VY227iMBB95yv8UmkrrVFCS2ndl_2TlYkniYsvWccphFX_fWWHhBBICJc_7BMKM0xmzhyfGfNrBWVsqIQcLVfBm5wF6O8EoeDhJwqD4ME_IKQzGnFbEhS_TxD6miA0P7YF0_fK_jWZVrFC78J4nglaEhQL2DgP94kZNxBZrhVBkRaFVM6S0IygT2p_YPw7fIPtzGxn20dnWWrDwBAUZhuUa8FZxw1aXthQxou8G_lp4X30J5hY6DVBKWcM1Ps_39npfKngicLcgswJikBZMMPJZpQxrpIT1m5KQZU2jVaJ0YVi3Z_wdllLba2Wwxg0tTz5WmKtLM75Fo7AoD6yt6_BJ6k98ki9R6SFNl1b6E0WNhZbQ1UeayMJKrIMTERzcEYB1oLBuWOHgyKYBnOQzTtjKrkojxAqHmuCeK411Tz7atac2ZSgl8y3Jd2lvXvs9H4ePAxDK5tX4Tw1XK0ICny3FZfUERMrKoHUx_LQxApDK_KG03l_aLNccpXguFA7fgPNAXOFdWEPPbmFKgyOdKEsQVzFXHELrcrnh5zkSnAF_ApqloPULLvUPDh3fSfqZdTZTI5I7kmTUQPK9nJsVtGvMLmzZZrXxY3i9AC_9tCS1ImBB7ifJ6_9xyBoR3s5Eydc9AeSbax7PZpXLa7hxP_W_XNt9urDINL1SVRatQ_O4q7dffVxasmZPQeV6NTDBJcE0cLq_6r_op3B217Qh8XzDG69tSK0TrkFL9lAUGYArw3NvEEbhpcG6Iog_4GpEF7lHfFqVMLp3H0nqUm48mraJE_vtQk0EZf3iFhpoqQbvBstbudpvSS60xA937WrZmmTJ_N5nhcO6PcL237xqLoHmHSKFxewy30zZjH6Dk0bPT6Si7bFjyK3PC5xpJUFN_v3pganoNpZbhDFym5LAQRxSwWPWgmnlx6Zy0vp34lHFRkG46XfJzdiwD0d6Dj3GIzmzQmJ7kywcyNwV9IY5t0CydGetK949BxsfvJxmih3vPBUaFidnb3RDQ7HHoo2hazGF1KevA40kcRlvPnWHeqUEg5c075hhR4C6h5r1_AunLa5KqtRVd8Yr_hR_8Y_MObC5ajmrS4bOze1RBfWjd2jRViSWEdFXnVkAFbRhlVdp5MX8L3K8vwN_WSuf25ids_ulgwzW41itjpAkTCe06WAakHr_lVW56_0G2dCr4G9T77_AQ9k5zqTEwAA_22_7D = __webpack_require__(27122);
;// ./src/components/backlog/SessionMonitor.css.ts


var SessionMonitor_css_container = 'bk09m21';
var SessionMonitor_css_emptyState = 'bk09m2g';
var errorRetryButton = 'bk09m2i';
var SessionMonitor_css_errorState = 'bk09m2h';
var SessionMonitor_css_inputRow = 'bk09m2j';
var liveIndicator = 'bk09m24';
var message = 'bk09m2b';
var messageContent = 'bk09m2f';
var messageList = 'bk09m2a';
var messageRole = 'bk09m2c';
var messageRoleAssistant = 'bk09m2e';
var messageRoleUser = 'bk09m2d';
var openLink = 'bk09m27';
var outputArea = 'bk09m28';
var quickActionButton = 'bk09m2l';
var quickActions = 'bk09m2k';
var SessionMonitor_css_sendButton = 'bk09m2n';
var SessionMonitor_css_terminalOutput = 'bk09m29';
var textInput = 'bk09m2m';
var toolbar = 'bk09m22';
var toolbarTitle = 'bk09m23';
var viewToggle = 'bk09m25';
var viewToggleActive = 'bk09m26';
;// ./src/components/backlog/SessionMonitor.tsx
/* __next_internal_client_entry_do_not_use__ SessionMonitor auto */ 
// +feature: backlog:session-monitor





const QUICK_ACTIONS = [
    "1",
    "2",
    "3",
    "y",
    "n"
];
const POLL_INTERVAL_MS = 5000;
const CONVERSATION_LIMIT = 30;
const TERMINAL_LINES = 60;
function SessionMonitor(param) {
    let { sessionId, sessionRole, isRunning } = param;
    const { getTerminalSnapshot, writeToSession, getConversationMessages } = (0,useSessionService/* useSessionService */.D)();
    const [view, setView] = (0,react.useState)("terminal");
    const [terminalOutput, setTerminalOutput] = (0,react.useState)("");
    const [messages, setMessages] = (0,react.useState)([]);
    const [inputValue, setInputValue] = (0,react.useState)("");
    const [sending, setSending] = (0,react.useState)(false);
    // Distinct from a genuinely empty terminal/history — a fetch failure must not
    // render as "No output yet…"/"No conversation history yet…", which looks
    // identical to real emptiness and hides the failure from the viewer.
    const [terminalError, setTerminalError] = (0,react.useState)(null);
    const [conversationError, setConversationError] = (0,react.useState)(null);
    const outputRef = (0,react.useRef)(null);
    const pollRef = (0,react.useRef)(null);
    // A NotFound response means the underlying session/history is gone for good —
    // retrying every POLL_INTERVAL_MS forever just hammers the server with a lookup
    // that can never succeed. Stop polling entirely once we see one.
    const goneRef = (0,react.useRef)(false);
    const stopPolling = (0,react.useCallback)(()=>{
        goneRef.current = true;
        if (pollRef.current) {
            clearInterval(pollRef.current);
            pollRef.current = null;
        }
    }, []);
    const fetchTerminal = (0,react.useCallback)(async ()=>{
        try {
            const output = await getTerminalSnapshot(sessionId, TERMINAL_LINES);
            setTerminalError(null);
            if (output) setTerminalOutput(output);
        } catch (err) {
            setTerminalError((0,connectError/* getErrorMessage */.u)(err, "Could not reach the server."));
            if (err instanceof connect_error/* ConnectError */.T && err.code === code/* Code */.C.NotFound) stopPolling();
        }
    }, [
        sessionId,
        getTerminalSnapshot,
        stopPolling
    ]);
    const fetchConversation = (0,react.useCallback)(async ()=>{
        try {
            const msgs = await getConversationMessages(sessionId, CONVERSATION_LIMIT);
            setConversationError(null);
            setMessages(msgs);
        } catch (err) {
            setConversationError((0,connectError/* getErrorMessage */.u)(err, "Could not reach the server."));
            if (err instanceof connect_error/* ConnectError */.T && err.code === code/* Code */.C.NotFound) stopPolling();
        }
    }, [
        sessionId,
        getConversationMessages,
        stopPolling
    ]);
    const refresh = (0,react.useCallback)(()=>{
        void fetchTerminal();
        void fetchConversation();
    }, [
        fetchTerminal,
        fetchConversation
    ]);
    // Reset stale state when sessionId changes
    (0,react.useEffect)(()=>{
        setTerminalOutput("");
        setMessages([]);
        setTerminalError(null);
        setConversationError(null);
        goneRef.current = false;
    }, [
        sessionId
    ]);
    // Initial load + polling while running
    (0,react.useEffect)(()=>{
        void refresh();
        if (isRunning && !goneRef.current) {
            pollRef.current = setInterval(refresh, POLL_INTERVAL_MS);
        }
        return ()=>{
            if (pollRef.current) clearInterval(pollRef.current);
        };
    }, [
        isRunning,
        refresh
    ]);
    // Auto-scroll output to bottom
    (0,react.useEffect)(()=>{
        if (outputRef.current) {
            outputRef.current.scrollTop = outputRef.current.scrollHeight;
        }
    }, [
        terminalOutput,
        messages
    ]);
    const handleSend = (0,react.useCallback)(async (text)=>{
        if (!text.trim() || sending) return;
        setSending(true);
        try {
            await writeToSession(sessionId, text, true);
            setInputValue("");
            // Refresh output after a short delay to capture response
            setTimeout(()=>void refresh(), 800);
        } finally{
            setSending(false);
        }
    }, [
        sessionId,
        writeToSession,
        sending,
        refresh
    ]);
    const handleKeyDown = (e)=>{
        if (e.key === "Enter") {
            e.preventDefault();
            void handleSend(inputValue);
        }
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: SessionMonitor_css_container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: toolbar,
                children: [
                    isRunning && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: liveIndicator,
                        "aria-hidden": "true"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: toolbarTitle,
                        children: [
                            sessionRole ? "".concat(sessionRole, " session") : "session",
                            " \xb7 ",
                            isRunning ? "running" : "ended"
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: "".concat(viewToggle, " ").concat(view === "terminal" ? viewToggleActive : ""),
                        onClick: ()=>setView("terminal"),
                        title: "Show terminal output",
                        children: "Terminal"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: "".concat(viewToggle, " ").concat(view === "history" ? viewToggleActive : ""),
                        onClick: ()=>setView("history"),
                        title: "Show conversation history",
                        children: "History"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                        className: openLink,
                        href: "/?session=".concat(sessionId),
                        title: "Open full terminal",
                        "aria-label": "Open session in full terminal view",
                        children: "↗ Open"
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: outputArea,
                ref: outputRef,
                "aria-label": "Session output",
                "aria-live": "polite",
                children: view === "history" ? conversationError ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: SessionMonitor_css_errorState,
                    "data-testid": "session-monitor-conversation-error",
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                            children: [
                                "Failed to load conversation history: ",
                                conversationError
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: errorRetryButton,
                            onClick: ()=>void fetchConversation(),
                            "data-testid": "session-monitor-retry-conversation",
                            children: "Retry"
                        })
                    ]
                }) : messages.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: SessionMonitor_css_emptyState,
                    children: "No conversation history yet…"
                }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: messageList,
                    children: messages.map((msg, i)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: message,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: "".concat(messageRole, " ").concat(msg.role === "user" ? messageRoleUser : messageRoleAssistant),
                                    children: msg.role
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: messageContent,
                                    children: truncateContent(msg.content)
                                })
                            ]
                        }, i))
                }) : terminalError ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: SessionMonitor_css_errorState,
                    "data-testid": "session-monitor-terminal-error",
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                            children: [
                                "Failed to load terminal output: ",
                                terminalError
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: errorRetryButton,
                            onClick: ()=>void fetchTerminal(),
                            "data-testid": "session-monitor-retry-terminal",
                            children: "Retry"
                        })
                    ]
                }) : terminalOutput ? /*#__PURE__*/ (0,jsx_runtime.jsx)("pre", {
                    className: SessionMonitor_css_terminalOutput,
                    children: terminalOutput
                }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: SessionMonitor_css_emptyState,
                    children: "No output yet…"
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: SessionMonitor_css_inputRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: quickActions,
                        "aria-label": "Quick inputs",
                        children: QUICK_ACTIONS.map((action)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: quickActionButton,
                                onClick: ()=>void handleSend(action),
                                disabled: sending || !isRunning,
                                "aria-label": 'Send "'.concat(action, '"'),
                                title: 'Send "'.concat(action, '"'),
                                children: action
                            }, action))
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        type: "text",
                        className: textInput,
                        value: inputValue,
                        onChange: (e)=>setInputValue(e.target.value),
                        onKeyDown: handleKeyDown,
                        placeholder: "Send input to session…",
                        disabled: sending || !isRunning,
                        "aria-label": "Session input",
                        "data-testid": "session-monitor-input"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: SessionMonitor_css_sendButton,
                        onClick: ()=>void handleSend(inputValue),
                        disabled: sending || !inputValue.trim() || !isRunning,
                        "data-testid": "session-monitor-send",
                        children: "Send"
                    })
                ]
            })
        ]
    });
}
// Cap message content at 800 chars to avoid massive tool-use blobs in the monitor
function truncateContent(content) {
    if (!content) return "";
    try {
        // If content is JSON (tool use), show a compact summary
        const parsed = JSON.parse(content);
        if (Array.isArray(parsed)) {
            return parsed.map((block)=>{
                var _block_text;
                var _block_text_slice;
                if (block.type === "text") return (_block_text_slice = (_block_text = block.text) === null || _block_text === void 0 ? void 0 : _block_text.slice(0, 400)) !== null && _block_text_slice !== void 0 ? _block_text_slice : "";
                if (block.type === "tool_use") return "[tool: ".concat(block.name, "]");
                if (block.type === "tool_result") return "[tool result]";
                return JSON.stringify(block).slice(0, 100);
            }).filter(Boolean).join("\n");
        }
    } catch (e) {
    // not JSON — plain text
    }
    return content.length > 800 ? content.slice(0, 800) + "…" : content;
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2Fdetail%2FBlockedNotice.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xb2ExNXQ1MCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeik7CiAgcGFkZGluZzogdmFyKC0tXzE5ZXoycnozMCk7CiAgYm9yZGVyLXJhZGl1czogdmFyKC0tXzE5ZXoycnozNyk7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnpkKTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwp9Ci5fMW9hMTV0NTEgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MnopOwogIG1hcmdpbjogMDsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2IpOwogIGZvbnQtd2VpZ2h0OiB2YXIoLS1fMTllejJyejNoKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnowKTsKfQouXzFvYTE1dDUyIHsKICBtYXJnaW46IDA7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxKTsKICB3aGl0ZS1zcGFjZTogcHJlLXdyYXA7Cn0%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2Fdetail_2FBlockedNotice_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xb2ExNXQ1MCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeik7CiAgcGFkZGluZzogdmFyKC0tXzE5ZXoycnozMCk7CiAgYm9yZGVyLXJhZGl1czogdmFyKC0tXzE5ZXoycnozNyk7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnpkKTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwp9Ci5fMW9hMTV0NTEgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MnopOwogIG1hcmdpbjogMDsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2IpOwogIGZvbnQtd2VpZ2h0OiB2YXIoLS1fMTllejJyejNoKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnowKTsKfQouXzFvYTE1dDUyIHsKICBtYXJnaW46IDA7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxKTsKICB3aGl0ZS1zcGFjZTogcHJlLXdyYXA7Cn0_22_7D = __webpack_require__(82362);
;// ./src/components/backlog/detail/BlockedNotice.css.ts


var BlockedNotice_css_label = '_1oa15t51';
var notice = '_1oa15t50';
var BlockedNotice_css_summaryText = '_1oa15t52';
;// ./src/components/backlog/detail/BlockedNotice.tsx
/* __next_internal_client_entry_do_not_use__ BlockedNotice auto */ 

const KIND_CONFIG = {
    blocked_guardrail: {
        icon: "🚫",
        label: "Blocked before starting",
        fallbackText: "No summary recorded."
    },
    manual_review_marker: {
        icon: "✍️",
        label: "Manual review",
        fallbackText: "No summary recorded."
    },
    missing_diagnostic_data: {
        icon: "⚠️",
        label: "No diagnostic data",
        fallbackText: "No diagnostic data recorded."
    }
};
/**
 * Blocked-Before-Start Notice (ux.md Surface 4 & 5): the plain-text
 * explanation shown for a Blocked Guardrail or Manual Review Marker
 * synthetic session — there was never a session to open, so this never
 * renders an "open session" affordance of any kind (Story 4.1.3).
 *
 * `role="status"`, not `role="log"`: a single current-state announcement,
 * not a scrolling transcript — no raw transcript exists for these DB-only
 * rows (plan.md's "Structured Diagnostic" glossary entry).
 *
 * Text is rendered verbatim from the backend. `session.reviewVerdict.summary`
 * for a "blocked_guardrail" kind comes from exactly two backend code paths
 * (see classifySessionKind in sessionKind.ts, and session/review_gate.go):
 *
 *  - "review-blocked-*": built from RunPreGateSecurityCheck's error string
 *    (session/review_gate.go ~line 228). Confirmed safe by Story 4.1.1's
 *    security review (plan.md's Unresolved Question #1) and covered by
 *    TestRunPreGateSecurityCheck_should_NeverEmbedRawSecretSubstringInErrorString_When_SecretDetectedInDiff
 *    in session/backlog_review_test.go — the error string only ever contains
 *    a fixed pattern name, never the matched secret or diff content.
 *  - "diff-error-*": built from GetGitDiffRef's wrapped command error
 *    (session/review_gate.go ~line 191, session/backlog_review.go's
 *    GetGitDiffRef). Currently benign — the wrapped error is `exec.Cmd`'s
 *    own Error() string (e.g. "exit status 128") plus the range arg and
 *    directory path, never command stderr or diff content, per
 *    TestGetGitDiffRefError_should_NeverEmbedCommandStderr_When_DiffCommandFails
 *    in session/backlog_review_test.go. This path has NOT had the same
 *    security review as RunPreGateSecurityCheck — if GetGitDiffRef's error
 *    wrapping is ever changed to include cmd.Stderr, that regression test
 *    is what would catch it before it reaches this UI surface.
 *
 * A "missing_diagnostic_data" notice most commonly means a headless
 * triage/review call itself errored or produced output that failed to parse
 * (see classifyHeadlessCallError, server/services/backlog_service_triage.go) —
 * `session.endReason`/`session.failureCapturePath` are populated in exactly
 * that case, so this renders that instead of the generic fallback text when
 * present, giving a concrete reason and a server-local path to the full raw
 * LLM output instead of an unexplained "no diagnostic data."
 */ function BlockedNotice(param) {
    let { kind, session } = param;
    var _session_reviewVerdict;
    const config = KIND_CONFIG[kind];
    const summary = (_session_reviewVerdict = session.reviewVerdict) === null || _session_reviewVerdict === void 0 ? void 0 : _session_reviewVerdict.summary;
    const headlessFailureText = session.endReason ? "Headless call failed (".concat(session.endReason).concat(session.errorDetail ? ": ".concat(session.errorDetail) : "", ").").concat(session.failureCapturePath ? " Full raw output captured on the server at: ".concat(session.failureCapturePath) : "") : undefined;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: notice,
        role: "status",
        "data-testid": "blocked-notice",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                className: BlockedNotice_css_label,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        "aria-hidden": "true",
                        children: config.icon
                    }),
                    " ",
                    config.label
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: BlockedNotice_css_summaryText,
                children: summary || headlessFailureText || config.fallbackText
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2Fdetail%2FSessionDiagnosticPanel.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xNDMya2JlMCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogdmFyKC0tXzE5ZXoycnozMCk7Cn0KLl8xNDMya2JlMSB7CiAgbWFyZ2luOiAwOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgZm9udC13ZWlnaHQ6IHZhcigtLV8xOWV6MnJ6M2gpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejEpOwp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2Fdetail_2FSessionDiagnosticPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xNDMya2JlMCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogdmFyKC0tXzE5ZXoycnozMCk7Cn0KLl8xNDMya2JlMSB7CiAgbWFyZ2luOiAwOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgZm9udC13ZWlnaHQ6IHZhcigtLV8xOWV6MnJ6M2gpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejEpOwp9_22_7D = __webpack_require__(50707);
;// ./src/components/backlog/detail/SessionDiagnosticPanel.css.ts


var SessionDiagnosticPanel_css_panel = '_1432kbe0';
var stateSummary = '_1432kbe1';
;// ./src/components/backlog/detail/SessionDiagnosticPanel.tsx
/* __next_internal_client_entry_do_not_use__ SessionDiagnosticPanel auto */ 





function summarizeTriage(triageResult) {
    const suggestionCount = triageResult.suggestions.filter((s)=>s.rationale !== "question").length;
    return "Triage completed — ".concat(suggestionCount, " suggestion").concat(suggestionCount === 1 ? "" : "s");
}
function summarizeReReview(reviewVerdict) {
    var _reviewVerdict_overallOutcome;
    return "Re-review completed — ".concat((_reviewVerdict_overallOutcome = reviewVerdict.overallOutcome) !== null && _reviewVerdict_overallOutcome !== void 0 ? _reviewVerdict_overallOutcome : "UNVERIFIABLE");
}
/**
 * Dispatches a Synthetic Session (Story 1.1.3's classifySessionKind) to the
 * correct read-only presentation — never a raw transcript/log viewer, per
 * plan.md's "Structured Diagnostic" glossary entry: no scrollback exists for
 * these DB-only rows, only already-fetched structured JSON.
 *
 *  - "headless_diagnostic" with `triageResult` populated → TriageReviewPanel
 *    readOnly (a `headless-triage-*` row).
 *  - "headless_diagnostic" with `reviewVerdict` populated → GateVerdictBox
 *    readOnly (a `headless-re-review-*` row).
 *  - "headless_diagnostic" with NEITHER populated (malformed/partial data) →
 *    falls back to BlockedNotice rather than rendering nothing, so this
 *    edge case can't reproduce the original inert-row bug.
 *  - "blocked_guardrail" / "manual_review_marker" → BlockedNotice (ux.md
 *    Surface 4 & 5) — identical treatment, distinct icon/label per kind.
 */ function SessionDiagnosticPanel(param) {
    let { session, item } = param;
    const kind = classifySessionKind(session);
    if (kind === "headless_diagnostic") {
        if (session.triageResult) {
            return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: SessionDiagnosticPanel_css_panel,
                "data-testid": "session-diagnostic-panel",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        role: "status",
                        className: stateSummary,
                        children: summarizeTriage(session.triageResult)
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(TriageReviewPanel, {
                        item: item,
                        triageResult: session.triageResult,
                        readOnly: true
                    })
                ]
            });
        }
        if (session.reviewVerdict) {
            var _reviewVerdict_perCriterion;
            const { reviewVerdict } = session;
            var _reviewVerdict_overallOutcome;
            return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: SessionDiagnosticPanel_css_panel,
                "data-testid": "session-diagnostic-panel",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        role: "status",
                        className: stateSummary,
                        children: summarizeReReview(reviewVerdict)
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(GateVerdictBox, {
                        verdict: (_reviewVerdict_overallOutcome = reviewVerdict.overallOutcome) !== null && _reviewVerdict_overallOutcome !== void 0 ? _reviewVerdict_overallOutcome : "UNVERIFIABLE",
                        summary: reviewVerdict.summary || "No summary recorded.",
                        criteria: (_reviewVerdict_perCriterion = reviewVerdict.perCriterion) === null || _reviewVerdict_perCriterion === void 0 ? void 0 : _reviewVerdict_perCriterion.map((c)=>({
                                label: "#".concat(c.criterionIndex, " ").concat(c.outcome).concat(c.evidence ? " — ".concat(c.evidence) : ""),
                                passed: c.outcome === "PASS"
                            })),
                        readOnly: true
                    })
                ]
            });
        }
        // Defensive edge case (architecture review finding): a headless_diagnostic
        // row with neither field populated must not render nothing — fall back to
        // the Blocked-Before-Start treatment instead.
        return /*#__PURE__*/ (0,jsx_runtime.jsx)(BlockedNotice, {
            kind: "missing_diagnostic_data",
            session: session
        });
    }
    if (kind === "blocked_guardrail" || kind === "manual_review_marker") {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)(BlockedNotice, {
            kind: kind,
            session: session
        });
    }
    // Defensive: SessionDiagnosticPanel is only meant to be used for Synthetic
    // Sessions. If a caller ever passes a "work"/"review" (Real Session) by
    // mistake, fail safe with the same Blocked-Before-Start treatment rather
    // than rendering nothing.
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(BlockedNotice, {
        kind: "missing_diagnostic_data",
        session: session
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FJulesStatusBadge.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA6VUzW6jMBC-5ynmmJWWCpNuunEvfZPK4AFGMTZrOyGh6ruvcGkKJCzVRuKAmeH7Gc_Myx7PuRUVOnhledn8YY9P8LYCyK2pwguAt0K73NiKgzVeeFzHEosfzyuA9xWAN7N5m-0g83318EkRhz8kuVqJMwfSijRGucJTlyoUFToij5XjkKH2aLvPhag5HIVdR9Er22Gb2DY5B-xaSEm6uBEFFtcBMzVWoo2skHRw08TNLsDkRvvIUYtX8fQr3iAVpeewjePnoSUWLKUi2xfWHLScYrCngJEZZexVrOMfYCX3Yn365cDqEzijSE7SqjHjZoHxPE_YjpEeF5CO80jNGOnXUhXYP6qQfKsKLB5Tbu8rwzLhacz3--YcpMpk-w4tnMq-4dikRSYgQlMlPBnNB4PMHhIXUIQF0jlp8jiC2YV_K2EL0pHC3F_NULswGmK-JMnYrLib68YYTjKKeTVVCHk8-UhiZmxfrIOWaLsSjbSm44u53Eiv3pu5VfRfhWK3pWkzVcVLc0QbtH3TSBaSa-PoI0ukzqhD1wUADUlfhl7tTpc--zhedmr85ZtD1Ec7GbkyDYeSpEQdrCmqOVjM_Dr-Cf0TnDUleYxcLTLsXDVW1IOd3Mvo1ulf0VwK_44GAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FJulesStatusBadge_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA6VUzW6jMBC_5ynmmJWWCpNuunEvfZPK4AFGMTZrOyGh6ruvcGkKJCzVRuKAmeH7Gc_Myx7PuRUVOnhledn8YY9P8LYCyK2pwguAt0K73NiKgzVeeFzHEosfzyuA9xWAN7N5m_0g83318EkRhz8kuVqJMwfSijRGucJTlyoUFToij5XjkKH2aLvPhag5HIVdR9Er22Gb2DY5B_xaSEm6uBEFFtcBMzVWoo2skHRw08TNLsDkRvvIUYtX8fQr3iAVpeewjePnoSUWLKUi2xfWHLScYrCngJEZZexVrOMfYCX3Yn365cDqEzijSE7SqjHjZoHxPE_YjpEeF5CO80jNGOnXUhXYP6qQfKsKLB5Tbu8rwzLhacz3_YcpMpk_w4tnMq_4dikRSYgQlMlPBnNB4PMHhIXUIQF0jlp8jiC2YV_K2EL0pHC3F_NULswGmK_JMnYrLib68YYTjKKeTVVCHk8_UhiZmxfrIOWaLsSjbSm44u53Eiv3pu5VfRfhWK3pWkzVcVLc0QbtH3TSBaSa_PoI0ukzqhD1wUADUlfhl7tTpc_zhedmr85ZtD1Ec7GbkyDYeSpEQdrCmqOVjM_Dr_Cf0TnDUleYxcLTLsXDVW1IOd3Mvo1ulf0VwK_44GAAA_22_7D = __webpack_require__(25418);
;// ./src/components/backlog/JulesStatusBadge.css.ts


var ariaLiveRegion = '_1fhwq14c';
var badge = '_1fhwq140';
var JulesStatusBadge_css_icon = '_1fhwq148';
var phaseDone = '_1fhwq144';
var phaseFailed = '_1fhwq145';
var phaseNeedsReview = '_1fhwq143';
var phaseQueued = '_1fhwq141';
var phaseReconnectRequired = '_1fhwq146';
var phaseRunning = '_1fhwq142';
var secondaryText = '_1fhwq149';
var updateKeyLink = '_1fhwq14a';
var webLink = '_1fhwq14b';
;// ./src/components/backlog/JulesStatusBadge.tsx
/* __next_internal_client_entry_do_not_use__ JulesStatusBadge auto */ 
// +feature: jules-status-badge


const PHASE_LABEL = {
    queued: "Jules: Queued",
    running: "Jules: Running",
    "needs-review": "Jules: Needs Review",
    done: "Jules: Done",
    failed: "Jules: Failed",
    "reconnect-required": "Jules: Reconnect required"
};
// Aria-hidden glyph per phase — distinguishes phases by shape/content, not
// color alone (ux.md §4's "no color-only signal" requirement, plan.md Task
// 3.3.1a/validation.md's "never color-alone" criterion).
const PHASE_ICON = {
    queued: "⏳",
    running: "↻",
    "needs-review": "👁",
    done: "✓",
    failed: "✕",
    "reconnect-required": "🔑"
};
const PHASE_CLASS = {
    queued: phaseQueued,
    running: phaseRunning,
    "needs-review": phaseNeedsReview,
    done: phaseDone,
    failed: phaseFailed,
    "reconnect-required": phaseReconnectRequired
};
// Polite announcement text for every phase except "failed", which is
// announced through the separate role="alert" region instead (assertive) —
// mirrors RemoteConnectionIndicator's POLITE_ANNOUNCE/ALERT_ANNOUNCE split.
const POLITE_ANNOUNCE = {
    queued: "Jules session queued",
    running: "Jules session running",
    "needs-review": "Jules session needs review",
    done: "Jules session done",
    "reconnect-required": "Jules: Reconnect required — update your API key"
};
const ALERT_ANNOUNCE = "Jules session failed";
// Staleness text (ux.md §4.2 step 5) only makes sense while the session is
// still open and actively being polled — a poller hiccup must never be
// confused with the terminal Done/Failed states or the account-wide
// Reconnect-required condition, which already has its own explanation.
const STALE_PHASES = new Set([
    "queued",
    "running",
    "needs-review"
]);
function minutesAgo(lastPolledAt) {
    const ts = typeof lastPolledAt === "string" ? new Date(lastPolledAt) : lastPolledAt;
    return Math.max(0, Math.floor((Date.now() - ts.getTime()) / 60000));
}
function JulesStatusBadge(param) {
    let { phase, lastPolledAt, pollHealthy = true, julesWebUrl } = param;
    const prevPhaseRef = (0,react.useRef)(phase);
    const [politeAnnouncement, setPoliteAnnouncement] = (0,react.useState)("");
    (0,react.useEffect)(()=>{
        if (prevPhaseRef.current !== phase) {
            const text = phase ? POLITE_ANNOUNCE[phase] : undefined;
            if (text) setPoliteAnnouncement(text);
            prevPhaseRef.current = phase;
        }
    }, [
        phase
    ]);
    // No real state known yet — never render a neutral/optimistic placeholder
    // chip (validation.md's "nothing renders before a real state is known").
    if (!phase) {
        return null;
    }
    const label = PHASE_LABEL[phase];
    const showStale = !pollHealthy && !!lastPolledAt && STALE_PHASES.has(phase);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: ariaLiveRegion,
                role: "status",
                "aria-live": "polite",
                "aria-atomic": "true",
                children: politeAnnouncement
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                className: "".concat(badge, " ").concat(PHASE_CLASS[phase]),
                role: "img",
                "aria-label": label,
                "data-testid": "jules-status-badge",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: JulesStatusBadge_css_icon,
                        "aria-hidden": "true",
                        "data-testid": "jules-status-icon",
                        children: PHASE_ICON[phase]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        children: label
                    })
                ]
            }),
            showStale && lastPolledAt && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: secondaryText,
                "data-testid": "jules-status-stale",
                children: "Last updated ".concat(minutesAgo(lastPolledAt), "m ago, retrying…")
            }),
            phase === "reconnect-required" && /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                className: updateKeyLink,
                href: "/settings/jules",
                children: "Update key"
            }),
            phase === "failed" && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                role: "alert",
                className: ariaLiveRegion,
                children: ALERT_ANNOUNCE
            }),
            julesWebUrl && /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                className: webLink,
                href: julesWebUrl,
                target: "_blank",
                rel: "noreferrer",
                children: "View this session on jules.google.com"
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2Fdetail%2FdetailShared.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xOTducXlyMCB7CiAgbWFyZ2luLXRvcDogdmFyKC0tXzE5ZXoycnoyeik7CiAgbWluLWhlaWdodDogNDRweDsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejJ6KSB2YXIoLS1fMTllejJyejMwKTsKICBiYWNrZ3JvdW5kOiBub25lOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6ayk7CiAgYm9yZGVyLXJhZGl1czogdmFyKC0tXzE5ZXoycnozNik7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6bSk7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKICBmb250LXdlaWdodDogdmFyKC0tXzE5ZXoycnozZyk7CiAgY3Vyc29yOiBwb2ludGVyOwogIGFsaWduLXNlbGY6IGZsZXgtc3RhcnQ7Cn0KLl8xOTducXlyMDpob3ZlciB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6bik7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycno4KTsKfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2Fdetail_2FdetailShared_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xOTducXlyMCB7CiAgbWFyZ2luLXRvcDogdmFyKC0tXzE5ZXoycnoyeik7CiAgbWluLWhlaWdodDogNDRweDsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejJ6KSB2YXIoLS1fMTllejJyejMwKTsKICBiYWNrZ3JvdW5kOiBub25lOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6ayk7CiAgYm9yZGVyLXJhZGl1czogdmFyKC0tXzE5ZXoycnozNik7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6bSk7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKICBmb250LXdlaWdodDogdmFyKC0tXzE5ZXoycnozZyk7CiAgY3Vyc29yOiBwb2ludGVyOwogIGFsaWduLXNlbGY6IGZsZXgtc3RhcnQ7Cn0KLl8xOTducXlyMDpob3ZlciB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6bik7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycno4KTsKfQ_22_7D = __webpack_require__(68271);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2Fdetail%2FSessionsSection.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-VWyZLaMBC98xW6pGp8EGUbBhhzyZ9MCUu2O8iSI8lgSPHvKS_M2MILFJnkkKv79evtqdvzdy_1FscVcdGvGUIRZ0WAvO0MoRQEPgI1SYDc7ewym1-RXoWkoDNOTkHlsm1cMQXFQgNSBCiUPE9FaYlJFqADUS8Yv3tv7Oyrs39ytneG8_vDEQ6xwGBYqgMUMmGYGoi1cJ0O4eKPENoZt4ppxVp2Y4HgIBieCPkj1waiEw6lMEyYtqmMmTCIExOg5TIrrDyunzJCKYi4p-v9peykokxhRSjk-qbcVQsTIC8rkJYcqAVjNYqE-1jJXFCbZl3ZQ8mlsk1ebcqVLm2ZhGu5kRQGazizm5yI82E_Nv2wEPGHwrBOFIi9LaxlkMgDU9WEmgb0ZpcOJ546FiMFTXac0YpUZiQEcwqQO39tFyikwYRzeWTUco9kmGt8AA07zmqO3JSSCZA_0PY6uwaGZRRpZip0h_r1Wc37Z2dMWGdbWBW-lcBqfL30K3s42rjUPDKitd1dat7f8S7WzrhGd5_2iKTATzeFHKwm1QLoDl5IwbYTEuU2z1M6XP-FpfU5vIcf3c_OAK32jI7q0bV412hH10_PTjOKCA31jbR8xNTKWrdX1kQHhWO5PqeJr9tNm38it2oMGVFMmIm7dOfh-x_0t3lAfxvHcv0yAb1VZP0TnGok6WZJKqqBOS1qPRhWlG8EYtFW3dSsetPzq-jfU0aBIB0qxgQigqKXlBTX67haulnhVHlZ53zsn9t6ONooZsKktFzaROuGqInlue63G8xmCHP5Dfw2gJw9DAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2Fdetail_2FSessionsSection_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_VWyZLaMBC98xW6pGp8EGUbBhhzyZ9MCUu2O8iSI8lgSPHvKS_M2MILFJnkkKv79evtqdvzdy_1FscVcdGvGUIRZ0WAvO0MoRQEPgI1SYDc7ewym1_RXoWkoDNOTkHlsm1cMQXFQgNSBCiUPE9FaYlJFqADUS8Yv3tv7Oyrs39ytneG8_vDEQ6xwGBYqgMUMmGYGoi1cJ0O4eKPENoZt4ppxVp2Y4HgIBieCPkj1waiEw6lMEyYtqmMmTCIExOg5TIrrDyunzJCKYi4p_v9peykokxhRSjk_qbcVQsTIC8rkJYcqAVjNYqE_1jJXFCbZl3ZQ8mlsk1ebcqVLm2ZhGu5kRQGazizm5yI82E_Nv2wEPGHwrBOFIi9LaxlkMgDU9WEmgb0ZpcOJ546FiMFTXac0YpUZiQEcwqQO39tFyikwYRzeWTUco9kmGt8AA07zmqO3JSSCZA_0PY6uwaGZRRpZip0h_r1Wc37Z2dMWGdbWBW_lcBqfL30K3s42rjUPDKitd1dat7f8S7WzrhGd5_2iKTATzeFHKwm1QLoDl5IwbYTEuU2z1M6XP_FpfU5vIcf3c_OAK32jI7q0bV412hH10_PTjOKCA31jbR8xNTKWrdX1kQHhWO5PqeJr9tNm38it2oMGVFMmIm7dOfh_x_0t3lAfxvHcv0yAb1VZP0TnGok6WZJKqqBOS1qPRhWlG8EYtFW3dSsetPzq_jfU0aBIB0qxgQigqKXlBTX67haulnhVHlZ53zsn9t6ONooZsKktFzaROuGqInlue63G8xmCHP5Dfw2gJw9DAAA_22_7D = __webpack_require__(2928);
;// ./src/components/backlog/detail/SessionsSection.css.ts



var diagnosticRowTitle = '_1m13w6a3';
var diagnosticRowWrapper = '_1m13w6a0';
var SessionsSection_css_emptyState = '_1m13w6aa';
var julesRowContent = '_1m13w6a1';
var julesRowMeta = '_1m13w6a2';
var sessionSteerBtn = '_1m13w6a4';
var showMoreButton = '_197nqyr0';
var steerCancelButton = '_1m13w6a8';
var steerComposer = '_1m13w6a5';
var SessionsSection_css_steerError = '_1m13w6a9';
var steerInput = '_1m13w6a6';
var steerSubmitButton = '_1m13w6a7';
;// ./src/components/backlog/detail/SessionsSection.tsx
/* __next_internal_client_entry_do_not_use__ SessionsSection auto */ 
// +feature: backlog:session-steer












// Jules end reasons that represent a successful outcome (session/jules_session_poller.go's
// applyCompletedState) -- everything else ("jules_failed", "jules_session_missing",
// "jules_timed_out", "dispatch_incomplete") is a failure. Kept local to this file
// since ItemSession carries no dedicated Jules-outcome field, only the generic
// end_reason string already used for headless triage/review buckets.
const JULES_SUCCESS_END_REASONS = new Set([
    "jules_completed",
    "jules_completed_no_pr"
]);
/**
 * Computes a jules_work row's badge phase from fields already on the wire
 * (role/endedAt/endReason) -- there is no live Jules API state on
 * ItemSession itself, only the coarse durable outcome (Story 3.3.2, Task
 * 3.3.2a). An open row is always "running": SessionsSection has no signal to
 * distinguish Queued/Planning/AwaitingPlanApproval, all of which the poller
 * treats identically (session/jules_session_poller.go's applyNonTerminalState).
 * authReconnectRequired overrides every open row account-wide (ux.md §4.2
 * step 6/plan.md Story 2.3.4) -- a closed row's phase is never overridden.
 */ function computeJulesPhase(session, authReconnectRequired) {
    if (!session.endedAt) {
        return authReconnectRequired ? "reconnect-required" : "running";
    }
    var _session_endReason;
    return JULES_SUCCESS_END_REASONS.has((_session_endReason = session.endReason) !== null && _session_endReason !== void 0 ? _session_endReason : "") ? "done" : "failed";
}
// Jules doesn't persist a web URL on ItemSession (only the ephemeral API
// response carries s.URL, session/jules_session_poller.go's
// julesSessionWebURL) -- the durable session_uuid is stored as
// "jules-sessions/<id>" (julesSessionNameFromUUID's inverse), which maps
// onto the same "https://jules.google.com/session/<id>" shape the backend's
// own failure notes already link to (plan.md:894).
const JULES_SESSION_UUID_PREFIX = "jules-sessions/";
function julesWebUrlFor(session) {
    const id = session.sessionId.startsWith(JULES_SESSION_UUID_PREFIX) ? session.sessionId.slice(JULES_SESSION_UUID_PREFIX.length) : session.sessionId;
    return "https://jules.google.com/session/".concat(id);
}
// Partial (not a full Record<SessionKind, ...>) because "work"/"review" are
// Real Sessions rendered via the plain <a> branch below and never look this
// map up — only the 3 Synthetic Session kinds need an icon here.
const SYNTHETIC_KIND_ICON = {
    headless_diagnostic: "🩺",
    blocked_guardrail: "🚫",
    manual_review_marker: "✍️"
};
function firstLine(message) {
    const newlineIndex = message.indexOf("\n");
    return newlineIndex === -1 ? message : message.slice(0, newlineIndex);
}
const SHOW_MORE_CAP = 5;
/**
 * Linked-sessions list, total cost, and the active-session monitor —
 * extracted verbatim from BacklogItemDetail.tsx (Story 3.1.4, Task 3.1.4c).
 * Default-expanded (primary operational info, per requirements' emphasis
 * on session inspectability). Caps its default rendering to the 5 most
 * recent sessions via `useShowMore` (Task 3.1.4c2, Blocker C fix) — a
 * single already-expanded section could otherwise reproduce the
 * "everything visible, nothing prioritized" problem this project exists
 * to fix, one level down, for heavily-cycled items.
 */ function SessionsSection(param) {
    let { item, pipelineModes, latestWorkSession, deletingSessionId, defaultExpanded, onDeleteSession, onSteerSession, steeringSessionId, authReconnectRequired = false } = param;
    const { visible, hasMore, remaining, showAll } = useShowMore(item.id, "sessions", item.linkedSessions, SHOW_MORE_CAP);
    // Steer composer state (Story 2.2.2). Mirrors TriageDiffSection's
    // openIndex/draft/toggleRefs shape (Gap 1's same inline-disclosure
    // pattern) — keyed by sessionId rather than array index since sessions
    // aren't positionally stable across a "show more" expansion.
    //
    // steerDrafts is keyed per-sessionId (Record<string, string>), not a
    // single shared string — matching TriageDiffSection's answerDrafts
    // (Record<number, string>) for the same "multiple independent inline
    // forms" problem. A single shared draft let an unsent draft typed for
    // session A survive into session B's composer when the operator switched
    // Steer targets without sending, risking the draft being sent to the
    // wrong session (code review finding, PR #457).
    const [openSteerFor, setOpenSteerFor] = (0,react.useState)(null);
    const [steerDrafts, setSteerDrafts] = (0,react.useState)({});
    const [steerError, setSteerError] = (0,react.useState)(null);
    const steerToggleRefs = (0,react.useRef)({});
    const steerInputRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        if (openSteerFor !== null) {
            var _steerInputRef_current;
            (_steerInputRef_current = steerInputRef.current) === null || _steerInputRef_current === void 0 ? void 0 : _steerInputRef_current.focus();
        }
    }, [
        openSteerFor
    ]);
    const clearSteerDraft = (sessionId)=>{
        setSteerDrafts((prev)=>{
            if (!(sessionId in prev)) return prev;
            const next = {
                ...prev
            };
            delete next[sessionId];
            return next;
        });
    };
    const handleSteerCancel = ()=>{
        const sessionId = openSteerFor;
        const toggle = sessionId ? steerToggleRefs.current[sessionId] : null;
        setOpenSteerFor(null);
        if (sessionId) clearSteerDraft(sessionId);
        setSteerError(null);
        toggle === null || toggle === void 0 ? void 0 : toggle.focus();
    };
    const handleSteerSubmit = async (s)=>{
        var _steerDrafts_s_sessionId;
        const message = ((_steerDrafts_s_sessionId = steerDrafts[s.sessionId]) !== null && _steerDrafts_s_sessionId !== void 0 ? _steerDrafts_s_sessionId : "").trim();
        if (!message) return;
        // Belt-and-suspenders re-check (pre-mortem failure #2, P2): the Send
        // button's disabled state already re-derives isSteerable(s) live on
        // every render, but a stale click event could still slip through if the
        // session ended between render and click — don't make the network call.
        if (!isSteerable(s)) {
            setSteerError("Session has ended — steering is unavailable.");
            return;
        }
        setSteerError(null);
        try {
            var _steerToggleRefs_current_s_sessionId;
            await onSteerSession(s, message);
            clearSteerDraft(s.sessionId);
            setOpenSteerFor(null);
            (_steerToggleRefs_current_s_sessionId = steerToggleRefs.current[s.sessionId]) === null || _steerToggleRefs_current_s_sessionId === void 0 ? void 0 : _steerToggleRefs_current_s_sessionId.focus();
        } catch (err) {
            // Keep the composer open and surface the error inline — don't close
            // optimistically on a failed RPC call.
            setSteerError((0,connectError/* getErrorMessage */.u)(err, "Failed to steer session."));
        }
    };
    // AC0 fix: previously `return null` here, which made a genuinely
    // zero-session item and a live populated→empty transition (blanked-out
    // event data, see WatchBacklogItems root cause) both look like the whole
    // "Sessions" block vanished — indistinguishable from a UI bug. An explicit
    // empty state (matching BacklogEmptyState.tsx's FooterNudge pattern) makes
    // "no sessions" a legible, intentional state instead.
    if (item.linkedSessions.length === 0) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)(Collapsible/* CollapsibleSection */.i, {
            sectionKey: "sessions",
            title: "Sessions (0)",
            defaultExpanded: defaultExpanded,
            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                role: "status",
                "aria-live": "polite",
                className: SessionsSection_css_emptyState,
                children: "No sessions yet for this item."
            })
        });
    }
    const statusToRole = {
        idea: "triage",
        in_progress: "work",
        review: "review"
    };
    const expectedRole = statusToRole[item.status];
    // For the "work" role, source from the same current-work-session value as
    // every other call site (Story 1.1.2, D3) instead of re-scanning
    // independently; other roles keep their own lookup since "current work
    // session" isn't what they're asking for.
    const active = expectedRole === "work" ? latestWorkSession && !latestWorkSession.endedAt ? latestWorkSession : undefined : [
        ...item.linkedSessions
    ].reverse().find((s)=>!s.endedAt && s.role === expectedRole && (classifySessionKind(s) === "work" || classifySessionKind(s) === "review"));
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(Collapsible/* CollapsibleSection */.i, {
        sectionKey: "sessions",
        title: "Sessions (".concat(item.linkedSessions.length, ")"),
        defaultExpanded: defaultExpanded,
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: section,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)(Collapsible/* CollapsibleGroup */.I, {
                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: sessionList,
                        role: "list",
                        "aria-label": "Linked sessions",
                        children: visible.map((s)=>{
                            var _s_reviewVerdict_perCriterion, _s_reviewVerdict_perCriterion1;
                            // A session without endedAt that isn't in the active phase for
                            // this item's current status is a stale/orphaned record — label
                            // it ended.
                            const isOrphan = !s.endedAt && s.role !== statusToRole[item.status];
                            const pipelineDisplay = resolvePipelineModeDisplay(s, pipelineModes);
                            // Story 5.2.4: the currently-fetched mode this session's frozen
                            // snapshot resolves to (or undefined for the default mode / an
                            // unrecognized slug) — resolveExecutorProvenance only computes
                            // drift when a live mode match exists, mirroring
                            // resolvePipelineModeDisplay's own precedent.
                            const matchedMode = pipelineModes.find((m)=>{
                                var _s_pipelineModeSnapshot;
                                return m.slug === ((_s_pipelineModeSnapshot = s.pipelineModeSnapshot) !== null && _s_pipelineModeSnapshot !== void 0 ? _s_pipelineModeSnapshot : "");
                            });
                            const provenance = resolveExecutorProvenance(s, matchedMode);
                            const kind = classifySessionKind(s);
                            const isSynthetic = kind !== "work" && kind !== "review";
                            // A Jules cloud session has no PTY/tmux Instance behind it --
                            // never Steerable, never Instance-backed -- so it's routed to
                            // its own row content (badge instead of branch chip, no
                            // SessionMonitor) ahead of the synthetic/work-review split
                            // above, which doesn't know about this kind (Story 3.3.2,
                            // Task 3.3.2a).
                            const isJulesWork = s.role === "jules_work";
                            var _s_entityId, _SYNTHETIC_KIND_ICON_kind, _steerDrafts_s_sessionId, _steerDrafts_s_sessionId1, _s_commitCountSinceSpawn, _s_reviewVerdict_perCriterion_length, _s_entityId1;
                            return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: sessionRow,
                                role: "listitem",
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: sessionRowMain,
                                        children: [
                                            isJulesWork ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: julesRowContent,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(JulesStatusBadge, {
                                                        phase: computeJulesPhase(s, authReconnectRequired),
                                                        julesWebUrl: julesWebUrlFor(s)
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                        className: julesRowMeta,
                                                        children: [
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                className: sessionId,
                                                                title: s.sessionId,
                                                                children: s.sessionId
                                                            }),
                                                            s.startedAt && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                className: sessionDate,
                                                                children: formatDate(s.startedAt)
                                                            }),
                                                            s.estimatedCostUsd > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                                className: sessionCost,
                                                                title: "Estimated session cost",
                                                                children: [
                                                                    "$",
                                                                    s.estimatedCostUsd.toFixed(4)
                                                                ]
                                                            })
                                                        ]
                                                    })
                                                ]
                                            }) : isSynthetic ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                                className: diagnosticRowWrapper,
                                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(Collapsible/* CollapsibleSection */.i, {
                                                    sectionKey: "session-".concat((_s_entityId = s.entityId) !== null && _s_entityId !== void 0 ? _s_entityId : s.sessionId),
                                                    defaultExpanded: false,
                                                    title: /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                        className: diagnosticRowTitle,
                                                        children: [
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                "aria-hidden": "true",
                                                                children: (_SYNTHETIC_KIND_ICON_kind = SYNTHETIC_KIND_ICON[kind]) !== null && _SYNTHETIC_KIND_ICON_kind !== void 0 ? _SYNTHETIC_KIND_ICON_kind : "🔍"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                className: sessionId,
                                                                title: s.sessionId,
                                                                children: s.sessionId
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                className: sessionRole,
                                                                children: s.role
                                                            }),
                                                            s.startedAt && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                className: sessionDate,
                                                                children: formatDate(s.startedAt)
                                                            }),
                                                            s.estimatedCostUsd > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                                className: sessionCost,
                                                                title: "Estimated session cost",
                                                                children: [
                                                                    "$",
                                                                    s.estimatedCostUsd.toFixed(4)
                                                                ]
                                                            })
                                                        ]
                                                    }),
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionDiagnosticPanel, {
                                                        session: s,
                                                        item: item
                                                    })
                                                })
                                            }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("a", {
                                                className: sessionLink,
                                                href: "/?session=".concat(s.sessionId),
                                                title: "Open in terminal",
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: sessionId,
                                                        title: s.sessionId,
                                                        children: s.sessionId
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: sessionRole,
                                                        children: s.role
                                                    }),
                                                    s.worktreeBranch && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: branchBadge,
                                                        title: "Git branch for this work session",
                                                        children: s.worktreeBranch
                                                    }),
                                                    s.baseCommitSha && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                        className: branchBadge,
                                                        title: "Branched from ".concat(s.baseCommitSha),
                                                        children: [
                                                            "from ",
                                                            s.baseCommitSha.slice(0, 7)
                                                        ]
                                                    }),
                                                    s.startedAt && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: sessionDate,
                                                        children: formatDate(s.startedAt)
                                                    }),
                                                    s.estimatedCostUsd > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                        className: sessionCost,
                                                        title: "Estimated session cost",
                                                        children: [
                                                            "$",
                                                            s.estimatedCostUsd.toFixed(4)
                                                        ]
                                                    }),
                                                    isOrphan && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: sessionEndedBadge,
                                                        children: "ended"
                                                    })
                                                ]
                                            }),
                                            !isSynthetic && !isJulesWork && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                type: "button",
                                                ref: (el)=>{
                                                    steerToggleRefs.current[s.sessionId] = el;
                                                },
                                                className: sessionSteerBtn,
                                                disabled: !isSteerable(s) || steeringSessionId === s.sessionId,
                                                "aria-disabled": !isSteerable(s),
                                                "aria-expanded": openSteerFor === s.sessionId,
                                                "aria-controls": "session-steer-composer-".concat(s.sessionId),
                                                title: !isSteerable(s) && s.endedAt ? "Session has ended — steering is unavailable" : undefined,
                                                "aria-label": "Steer session ".concat(s.sessionId),
                                                "data-testid": "session-steer-toggle-".concat(s.sessionId),
                                                onClick: (e)=>{
                                                    e.preventDefault();
                                                    setSteerError(null);
                                                    setOpenSteerFor(s.sessionId);
                                                },
                                                children: "Steer"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                className: sessionDeleteBtn,
                                                disabled: deletingSessionId === s.sessionId,
                                                "aria-label": "Delete session",
                                                onClick: (e)=>{
                                                    e.preventDefault();
                                                    onDeleteSession(s);
                                                },
                                                children: deletingSessionId === s.sessionId ? "…" : "Delete"
                                            })
                                        ]
                                    }),
                                    !isSynthetic && !isJulesWork && openSteerFor === s.sessionId && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        id: "session-steer-composer-".concat(s.sessionId),
                                        "data-testid": "session-steer-composer-".concat(s.sessionId),
                                        className: steerComposer,
                                        role: "form",
                                        "aria-label": "Steer session ".concat(s.sessionId),
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                ref: steerInputRef,
                                                type: "text",
                                                className: steerInput,
                                                value: (_steerDrafts_s_sessionId = steerDrafts[s.sessionId]) !== null && _steerDrafts_s_sessionId !== void 0 ? _steerDrafts_s_sessionId : "",
                                                onChange: (e)=>setSteerDrafts((prev)=>({
                                                            ...prev,
                                                            [s.sessionId]: e.target.value
                                                        })),
                                                onKeyDown: (e)=>{
                                                    if (e.key === "Enter") void handleSteerSubmit(s);
                                                    if (e.key === "Escape") handleSteerCancel();
                                                },
                                                placeholder: "Steering message…",
                                                "data-testid": "session-steer-input-".concat(s.sessionId),
                                                disabled: steeringSessionId === s.sessionId
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                type: "button",
                                                className: steerSubmitButton,
                                                onClick: ()=>void handleSteerSubmit(s),
                                                // Re-derives isSteerable(s) from the current s prop on every
                                                // render rather than closing over the value from when the
                                                // composer was opened — a session that ends while the
                                                // composer is open must disable Send without requiring
                                                // close/reopen (pre-mortem failure #2, P2).
                                                disabled: steeringSessionId === s.sessionId || !((_steerDrafts_s_sessionId1 = steerDrafts[s.sessionId]) !== null && _steerDrafts_s_sessionId1 !== void 0 ? _steerDrafts_s_sessionId1 : "").trim() || !isSteerable(s),
                                                "aria-busy": steeringSessionId === s.sessionId,
                                                "data-testid": "session-steer-submit-".concat(s.sessionId),
                                                children: steeringSessionId === s.sessionId ? "Sending…" : "Send"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                type: "button",
                                                className: steerCancelButton,
                                                onClick: handleSteerCancel,
                                                "data-testid": "session-steer-cancel-".concat(s.sessionId),
                                                children: "Cancel"
                                            }),
                                            steerError && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: SessionsSection_css_steerError,
                                                role: "alert",
                                                children: steerError
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: pipelineGroup,
                                        role: "group",
                                        "aria-label": "Pipeline",
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: pipelineLabel,
                                                children: "Pipeline:"
                                            }),
                                            " ",
                                            pipelineDisplay.kind === "unrecognized" ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: pipelineValue,
                                                children: "custom (unrecognized mode: '".concat(pipelineDisplay.slug, "')")
                                            }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: pipelineValue,
                                                        children: pipelineDisplay.name
                                                    }),
                                                    pipelineDisplay.drifted && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                                        children: [
                                                            " ",
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                className: pipelineDriftBadge,
                                                                children: "(content since changed)"
                                                            })
                                                        ]
                                                    })
                                                ]
                                            }),
                                            provenance.fallback && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                className: executorFallbackBadge,
                                                "aria-label": "Fell back to Claude: ".concat(provenance.fallback.reason),
                                                children: [
                                                    " ",
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        "aria-hidden": "true",
                                                        children: "↩"
                                                    }),
                                                    " Ran on different program"
                                                ]
                                            }),
                                            provenance.drifted && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                className: executorDriftBadge,
                                                "aria-label": "Executor config changed since this session ran",
                                                children: [
                                                    " ",
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        "aria-hidden": "true",
                                                        children: "⚙"
                                                    }),
                                                    " (executor config since changed)"
                                                ]
                                            })
                                        ]
                                    }),
                                    !isSynthetic && ((_s_commitCountSinceSpawn = s.commitCountSinceSpawn) !== null && _s_commitCountSinceSpawn !== void 0 ? _s_commitCountSinceSpawn : 0) > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: commitDetail,
                                        title: s.lastCommitMessage,
                                        children: [
                                            s.commitCountSinceSpawn,
                                            " commit",
                                            s.commitCountSinceSpawn === 1 ? "" : "s",
                                            s.lastCommitMessage && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                                children: [
                                                    " — ",
                                                    firstLine(s.lastCommitMessage)
                                                ]
                                            })
                                        ]
                                    }),
                                    !isSynthetic && s.reviewVerdict && (s.reviewVerdict.summary || ((_s_reviewVerdict_perCriterion_length = (_s_reviewVerdict_perCriterion = s.reviewVerdict.perCriterion) === null || _s_reviewVerdict_perCriterion === void 0 ? void 0 : _s_reviewVerdict_perCriterion.length) !== null && _s_reviewVerdict_perCriterion_length !== void 0 ? _s_reviewVerdict_perCriterion_length : 0) > 0) && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: verdictDetail,
                                        "aria-label": "Review verdict detail",
                                        children: [
                                            s.reviewVerdict.summary && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                className: verdictSummary,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("strong", {
                                                        children: [
                                                            s.reviewVerdict.overallOutcome,
                                                            ":"
                                                        ]
                                                    }),
                                                    " ",
                                                    s.reviewVerdict.summary
                                                ]
                                            }),
                                            (_s_reviewVerdict_perCriterion1 = s.reviewVerdict.perCriterion) === null || _s_reviewVerdict_perCriterion1 === void 0 ? void 0 : _s_reviewVerdict_perCriterion1.map((c)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                    className: verdictCriterion,
                                                    children: [
                                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                            children: [
                                                                "#",
                                                                c.criterionIndex,
                                                                " ",
                                                                c.outcome
                                                            ]
                                                        }),
                                                        c.evidence && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                            children: [
                                                                "— ",
                                                                c.evidence
                                                            ]
                                                        })
                                                    ]
                                                }, c.criterionIndex))
                                        ]
                                    })
                                ]
                            }, (_s_entityId1 = s.entityId) !== null && _s_entityId1 !== void 0 ? _s_entityId1 : s.sessionId);
                        })
                    })
                }),
                hasMore && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                    type: "button",
                    className: showMoreButton,
                    onClick: showAll,
                    "data-testid": "sessions-show-more",
                    children: [
                        "Show ",
                        remaining,
                        " more"
                    ]
                }),
                item.totalEstimatedCostUsd > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                    className: sessionTotalCost,
                    children: [
                        "Total estimated cost: ",
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("strong", {
                            children: [
                                "$",
                                item.totalEstimatedCostUsd.toFixed(4)
                            ]
                        })
                    ]
                }),
                active && active.role !== "jules_work" && /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionMonitor, {
                    sessionId: active.sessionId,
                    sessionRole: active.role,
                    isRunning: true
                })
            ]
        })
    });
}

;// ./src/components/backlog/detail/AutonomousHealthStrip.tsx



function reworkAttemptCount(item) {
    return item.linkedSessions.filter((s)=>s.role === "work").length;
}
function AutonomousHealthStrip(param) {
    let { item } = param;
    const attempts = reworkAttemptCount(item);
    const cap = item.reworkCapOverride;
    const showCost = item.totalEstimatedCostUsd > 0;
    if (!showCost && attempts === 0 && cap === undefined) {
        return null;
    }
    const capLabel = cap === undefined ? "default" : cap === 0 ? "unlimited" : String(cap);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: autonomousHealthStrip,
        "data-testid": "autonomous-health-strip",
        children: [
            showCost && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                children: [
                    "Estimated cost: ",
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("strong", {
                        children: [
                            "$",
                            item.totalEstimatedCostUsd.toFixed(4)
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                children: [
                    "Rework attempts: ",
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("strong", {
                        children: attempts
                    }),
                    " / ",
                    capLabel
                ]
            })
        ]
    });
}

;// ./src/components/backlog/detail/WorkflowHistorySection.css.ts


var WorkflowHistorySection_css_showMoreButton = '_197nqyr0';
;// ./src/components/backlog/detail/WorkflowHistorySection.tsx
/* __next_internal_client_entry_do_not_use__ WorkflowHistorySection auto */ 





const WorkflowHistorySection_SHOW_MORE_CAP = 8;
/**
 * Status-transition audit trail — extracted verbatim from
 * BacklogItemDetail.tsx (Story 3.1.4, Task 3.1.4d), collapsed by default.
 * Caps its default rendering to the 8 most recent events via `useShowMore`
 * (Task 3.1.4d2, Blocker C fix).
 *
 * Always renders (never hides itself for zero events) — items created before
 * the status-change audit-trail fix (#198) have no BacklogStatusEvent rows,
 * and hiding the section entirely made the Workflow feature look broken
 * rather than showing an explicit "no history" state.
 */ function WorkflowHistorySection(param) {
    let { item, defaultExpanded } = param;
    const { visible, hasMore, remaining, showAll } = useShowMore(item.id, "workflow", item.statusEvents, WorkflowHistorySection_SHOW_MORE_CAP);
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(Collapsible/* CollapsibleSection */.i, {
        sectionKey: "workflow",
        title: "Workflow",
        defaultExpanded: defaultExpanded,
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: section,
            children: item.statusEvents.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: emptyText,
                children: "No status history recorded."
            }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: workflowTimeline,
                        role: "list",
                        "aria-label": "Status history",
                        children: visible.map((ev)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: workflowEvent,
                                role: "listitem",
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: workflowEventRow,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: workflowEventFrom,
                                                children: ev.fromStatus.replace("_", " ")
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: workflowEventArrow,
                                                children: "→"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: workflowEventTo,
                                                children: ev.toStatus.replace("_", " ")
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                className: workflowEventMeta,
                                                children: [
                                                    ev.createdAt ? formatDate(ev.createdAt) : "",
                                                    " · ",
                                                    ev.triggeredBy
                                                ]
                                            })
                                        ]
                                    }),
                                    ev.note && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: workflowEventNote,
                                        children: ev.note
                                    })
                                ]
                            }, ev.id))
                    }),
                    hasMore && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                        type: "button",
                        className: WorkflowHistorySection_css_showMoreButton,
                        onClick: showAll,
                        "data-testid": "workflow-show-more",
                        children: [
                            "Show ",
                            remaining,
                            " more"
                        ]
                    })
                ]
            })
        })
    });
}

;// ./src/components/backlog/detail/ProgressHistorySection.css.ts


var ProgressHistorySection_css_showMoreButton = '_197nqyr0';
;// ./src/components/backlog/detail/ProgressHistorySection.tsx
/* __next_internal_client_entry_do_not_use__ ProgressHistorySection auto */ 





const ProgressHistorySection_SHOW_MORE_CAP = 8;
/**
 * The implementer's report_progress audit trail — extracted verbatim from
 * BacklogItemDetail.tsx (Story 3.1.4, Task 3.1.4e), collapsed by default.
 * Caps its default rendering to the 8 most recent notes via `useShowMore`
 * (Task 3.1.4e2, Blocker C fix).
 */ function ProgressHistorySection(param) {
    let { item, defaultExpanded } = param;
    const { visible, hasMore, remaining, showAll } = useShowMore(item.id, "progress-history", item.progressNotes, ProgressHistorySection_SHOW_MORE_CAP);
    if (item.progressNotes.length === 0) return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(Collapsible/* CollapsibleSection */.i, {
        sectionKey: "progress-history",
        title: "Progress History",
        defaultExpanded: defaultExpanded,
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: section,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: progressNoteList,
                    role: "list",
                    "aria-label": "Implementer progress history",
                    children: visible.map((n)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: progressNoteItem,
                            role: "listitem",
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: progressNoteMeta,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                            children: [
                                                "Criterion #",
                                                n.criterionIndex
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            children: "\xb7"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            children: n.status
                                        }),
                                        n.createdAt && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    children: "\xb7"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    children: formatDate(n.createdAt)
                                                })
                                            ]
                                        })
                                    ]
                                }),
                                n.note && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    children: n.note
                                })
                            ]
                        }, n.id))
                }),
                hasMore && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                    type: "button",
                    className: ProgressHistorySection_css_showMoreButton,
                    onClick: showAll,
                    "data-testid": "progress-history-show-more",
                    children: [
                        "Show ",
                        remaining,
                        " more"
                    ]
                })
            ]
        })
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FResultPanel.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61W226jMBB9z1f4pVIjrSug6c196Z-sDB7CtGAj2xTIKv--wg4uCQvprlaREpkZn5k5M3PI2wf0ueYVGPLRvzRRvSe_NoRENz9IHEU37kCIqnmGtmckutu9bgg5bgh5mFtjbztu7jxW5BwEmrrkPSN5Cd3gMfxSgRoyi0oykqmyqeRg2fOakU-ubyn9Gb_AIdGH5LAdLBVK2qKwBSPRcK65ECj3l973kfNOlRagGYnrjhhVorhwyydeVHOBjZkhPXkfnn3stWqkuHTw9lxJSw0eYHY_dXb1CTovVUtbPdTGZd8WoOH1i6R4hSR_afgeHvES95KihcowkoG0oIfH742xmPc0U9KCtIyYmmdAU7AtwCqtIYfE5VBxvUd5Ini5sOyr8BZwX1hGHqNognbv0JYB-Pasg0nd_bnn6_15vNKfZ2fPVKn0rHhnagu0QB1ZjEjlWQ5F7FwRy_Cfy_DtlNqHKzhxvAwUJ1OkxytI_TLQWbOf_qbZfKHZK8yGQM8uUInGUmP70rEs4fUyeBgEd_q2WoQwL_-2QCk3UKLPZ1l3FnQm6bc-3dOMpspaVa0LzlfCrOTG0qzAUvienoN4loI79-tUQsdIPHwSDdVcE4N_Gvxpyg0a5pT8yjKsT0DAzhz2Cs6ks-GS-IYeBGfwERpthhC1wlHmhmqL0wjudnV3Niwoh1bSsfcLSnl9ZPP5bixNwOm9xLuxByPLQfI7RnhjVaA35xWW_QyouSJk4ptiGIooXBEjV7Eflv8hplxixf0Wjn8W4rudIcANUJRUNZagzFGinQ4wunxOLO0GkoLpfWp68qa3CgRycltryEEbqkE0GQhaKR_an7fu5lnFZwmOUnPcHH8DdjcHOuQIAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FResultPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61W226jMBB9z1f4pVIjrSug6c196Z_sDB7CtGAj2xTIKv_wg4uCQvprlaREpkZn5k5M3PI2wf0ueYVGPLRvzRRvSe_NoRENz9IHEU37kCIqnmGtmckutu9bgg5bgh5mFtjbztu7jxW5BwEmrrkPSN5Cd3gMfxSgRoyi0oykqmyqeRg2fOakU_ubyn9Gb_AIdGH5LAdLBVK2qKwBSPRcK65ECj3l973kfNOlRagGYnrjhhVorhwyydeVHOBjZkhPXkfnn3stWqkuHTw9lxJSw0eYHY_dXb1CTovVUtbPdTGZd8WoOH1i6R4hSR_afgeHvES95KihcowkoG0oIfH742xmPc0U9KCtIyYmmdAU7AtwCqtIYfE5VBxvUd5Ini5sOyr8BZwX1hGHqNognbv0JYB_Pasg0nd_bnn6_15vNKfZ2fPVKn0rHhnagu0QB1ZjEjlWQ5F7FwRy_Cfy_DtlNqHKzhxvAwUJ1OkxytI_TLQWbOf_qbZfKHZK8yGQM8uUInGUmP70rEs4fUyeBgEd_q2WoQwL_2QCk3UKLPZ1l3FnQm6bc_3dOMpspaVa0LzlfCrOTG0qzAUvienoN4loI79_tUQsdIPHwSDdVcE4N_Gvxpyg0a5pT8yjKsT0DAzhz2Cs6ks_GS_IYeBGfwERpthhC1wlHmhmqL0wjudnV3Niwoh1bSsfcLSnl9ZPP5bixNwOm9xLuxByPLQfI7RnhjVaA35xWW_QyouSJk4ptiGIooXBEjV7Eflv8hplxixf0Wjn8W4rudIcANUJRUNZagzFGinQ4wunxOLO0GkoLpfWp68qa3CgRycltryEEbqkE0GQhaKR_an7fu5lnFZwmOUnPcHH8DdjcHOuQIAAA_22_7D = __webpack_require__(96907);
;// ./src/components/backlog/ResultPanel.css.ts


var ResultPanel_css_badge = 'ky9u0p3';
var badgeFail = 'ky9u0p5 ky9u0p3';
var badgePass = 'ky9u0p4 ky9u0p3';
var badgeWarn = 'ky9u0p6 ky9u0p3';
var ResultPanel_css_container = 'ky9u0p0';
var disclosure = 'ky9u0pd';
var disclosureSummary = 'ky9u0pe';
var evidence = 'ky9u0pb';
var ResultPanel_css_header = 'ky9u0p1';
var ResultPanel_css_list = 'ky9u0p8';
var mutedText = 'ky9u0pc';
var rawJson = 'ky9u0pf';
var row = 'ky9u0p9';
var rowText = 'ky9u0pa';
var ResultPanel_css_sectionLabel = 'ky9u0p7';
var skeletonLine = 'ky9u0ph';
var skeletonMedium = 'ky9u0pj ky9u0ph';
var skeletonShort = 'ky9u0pi ky9u0ph';
var title = 'ky9u0p2';
;// ./src/components/backlog/DiagnoseResultCard.tsx
/* __next_internal_client_entry_do_not_use__ parseDiagnosisNote,DiagnoseResultCard auto */ 

const DIAGNOSIS_PREFIX = "Diagnosis result: ";
/** Splits a diagnose activity note into outcome + summary; null if it isn't one. */ function parseDiagnosisNote(message) {
    if (!message.startsWith(DIAGNOSIS_PREFIX)) return null;
    const rest = message.slice(DIAGNOSIS_PREFIX.length);
    const sep = rest.indexOf("\n\n");
    const outcome = (sep === -1 ? rest : rest.slice(0, sep)).trim();
    if (!/^[A-Z_]+$/.test(outcome)) return null;
    return {
        outcome,
        summary: sep === -1 ? "" : rest.slice(sep + 2).trim()
    };
}
function DiagnoseResultCard(param) {
    let { message } = param;
    const parsed = parseDiagnosisNote(message);
    // Parse-error fallback: unrecognized shape renders as the original plain text.
    if (!parsed) return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
        children: message
    });
    const badge = parsed.outcome === "BUG_FILED" ? badgeFail : parsed.outcome === "NO_ACTION" ? ResultPanel_css_badge : badgeWarn;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: ResultPanel_css_container,
        "data-testid": "diagnose-result-card",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: ResultPanel_css_header,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h4", {
                        className: title,
                        children: "Diagnosis"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: badge,
                        children: parsed.outcome
                    })
                ]
            }),
            parsed.summary && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: mutedText,
                children: parsed.summary
            })
        ]
    });
}

;// ./src/components/backlog/detail/ActivityLogSection.tsx
/* __next_internal_client_entry_do_not_use__ ActivityLogSection auto */ 






const ActivityLogSection_SHOW_MORE_CAP = 8;
/**
 * The ungated, free-form activity log posted via post_backlog_update
 * (backlog-item-activity-log, ADR-001's sibling table to
 * BacklogProgressNote) — structurally cloned from ProgressHistorySection.tsx
 * but deliberately renders a visually distinct meta-line ("<author> ·
 * <date>", never "Criterion #N · status") so an informal note is never
 * confusable with an official progress mark. Collapsed by default; caps its
 * default rendering to the 8 most recent notes via useShowMore.
 */ function ActivityLogSection(param) {
    let { item, defaultExpanded } = param;
    const { visible, hasMore, remaining, showAll } = useShowMore(item.id, "activity-log", item.activityNotes, ActivityLogSection_SHOW_MORE_CAP);
    var _item_activityNotes;
    if (((_item_activityNotes = item.activityNotes) !== null && _item_activityNotes !== void 0 ? _item_activityNotes : []).length === 0) return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(Collapsible/* CollapsibleSection */.i, {
        sectionKey: "activity-log",
        title: "Activity Log",
        defaultExpanded: defaultExpanded,
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: section,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: progressNoteList,
                    role: "list",
                    "aria-label": "Backlog item activity log",
                    children: visible.map((n)=>{
                        const author = n.authorSessionTitle || (n.authorSessionUuid ? n.authorSessionUuid.slice(0, 8) : "") || "manual";
                        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: progressNoteItem,
                            role: "listitem",
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: progressNoteMeta,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            children: author
                                        }),
                                        n.createdAt && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    children: "\xb7"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    children: formatDate(n.createdAt)
                                                })
                                            ]
                                        })
                                    ]
                                }),
                                n.message && /*#__PURE__*/ (0,jsx_runtime.jsx)(DiagnoseResultCard, {
                                    message: n.message
                                })
                            ]
                        }, n.id);
                    })
                }),
                hasMore && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                    type: "button",
                    className: ProgressHistorySection_css_showMoreButton,
                    onClick: showAll,
                    "data-testid": "activity-log-show-more",
                    children: [
                        "Show ",
                        remaining,
                        " more"
                    ]
                })
            ]
        })
    });
}

;// ./src/components/backlog/RawJsonDisclosure.tsx
/* __next_internal_client_entry_do_not_use__ RawJsonDisclosure auto */ 

/** "View raw JSON" toggle shared by the formatted result panels. */ function RawJsonDisclosure(param) {
    let { value, testId } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("details", {
        className: disclosure,
        "data-testid": testId,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("summary", {
                className: disclosureSummary,
                children: "View raw JSON"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("pre", {
                className: rawJson,
                children: JSON.stringify(value, null, 2)
            })
        ]
    });
}

;// ./src/components/backlog/TriageResultPanel.tsx
/* __next_internal_client_entry_do_not_use__ TRIAGE_RESULTS_PANEL_FLAG,TriageResultPanel auto */ 



const TRIAGE_RESULTS_PANEL_FLAG = "backlog-triage-results-panel";
function isMalformed(r) {
    var _r_suggestions, _r_tasks;
    return typeof r.summary !== "string" || !Array.isArray((_r_suggestions = r.suggestions) !== null && _r_suggestions !== void 0 ? _r_suggestions : []) || !Array.isArray((_r_tasks = r.tasks) !== null && _r_tasks !== void 0 ? _r_tasks : []);
}
function TriageResultPanel(param) {
    let { triageResult, priority, loading } = param;
    const enabled = (0,FeatureFlagsContext/* useFeatureFlag */.u)(TRIAGE_RESULTS_PANEL_FLAG);
    if (!enabled) return null;
    if (loading) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
            className: ResultPanel_css_container,
            "aria-busy": "true",
            "aria-label": "Triage Results",
            "data-testid": "triage-result-skeleton",
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: skeletonShort
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: skeletonLine
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: skeletonMedium
                })
            ]
        });
    }
    if (!triageResult) return null;
    if (isMalformed(triageResult)) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
            className: ResultPanel_css_container,
            "aria-label": "Triage Results",
            "data-testid": "triage-result-panel",
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                    className: title,
                    children: "Triage Results"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: mutedText,
                    children: "Triage result could not be displayed."
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)(RawJsonDisclosure, {
                    value: triageResult,
                    testId: "triage-raw-json"
                })
            ]
        });
    }
    var _triageResult_tasks;
    const tasks = (_triageResult_tasks = triageResult.tasks) !== null && _triageResult_tasks !== void 0 ? _triageResult_tasks : [];
    var _triageResult_suggestions;
    const suggestions = ((_triageResult_suggestions = triageResult.suggestions) !== null && _triageResult_suggestions !== void 0 ? _triageResult_suggestions : []).filter((s)=>s.rationale !== "question");
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
        className: ResultPanel_css_container,
        "aria-label": "Triage Results",
        "data-testid": "triage-result-panel",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: ResultPanel_css_header,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: title,
                        children: "Triage Results"
                    }),
                    priority != null && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: ResultPanel_css_badge,
                        children: [
                            "P",
                            priority
                        ]
                    })
                ]
            }),
            triageResult.summary && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                children: triageResult.summary
            }),
            tasks.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("h4", {
                        className: ResultPanel_css_sectionLabel,
                        children: [
                            "Tasks (",
                            tasks.length,
                            ")"
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                        className: ResultPanel_css_list,
                        children: tasks.map((t, i)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                                className: row,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: rowText,
                                        children: t.text
                                    }),
                                    t.estimate && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: ResultPanel_css_badge,
                                        children: t.estimate
                                    }),
                                    t.category && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: ResultPanel_css_badge,
                                        children: t.category
                                    })
                                ]
                            }, i))
                    })
                ]
            }),
            suggestions.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("h4", {
                        className: ResultPanel_css_sectionLabel,
                        children: [
                            "Suggestions (",
                            suggestions.length,
                            ")"
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                        className: ResultPanel_css_list,
                        children: suggestions.map((s, i)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                                className: row,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: rowText,
                                        children: s.text
                                    }),
                                    s.rationale && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: evidence,
                                        children: s.rationale
                                    })
                                ]
                            }, i))
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(RawJsonDisclosure, {
                value: triageResult,
                testId: "triage-raw-json"
            })
        ]
    });
}

;// ./src/components/backlog/ReviewVerdictPanel.tsx
/* __next_internal_client_entry_do_not_use__ REVIEW_RESULTS_PANEL_FLAG,latestReviewVerdict,ReviewVerdictPanel auto */ 



const REVIEW_RESULTS_PANEL_FLAG = "backlog-review-results-panel";
/** Verdict of the most recent review session that has one (linkedSessions is oldest-first). */ function latestReviewVerdict(sessions) {
    var _sessions_filter_at;
    return (_sessions_filter_at = sessions.filter((s)=>s.role === "review" && s.reviewVerdict).at(-1)) === null || _sessions_filter_at === void 0 ? void 0 : _sessions_filter_at.reviewVerdict;
}
function outcomeBadge(outcome) {
    const o = (outcome !== null && outcome !== void 0 ? outcome : "").toUpperCase();
    if (o === "PASS") return badgePass;
    if (o === "FAIL") return badgeFail;
    return badgeWarn;
}
function ReviewVerdictPanel(param) {
    let { verdict, loading } = param;
    const enabled = (0,FeatureFlagsContext/* useFeatureFlag */.u)(REVIEW_RESULTS_PANEL_FLAG);
    if (!enabled) return null;
    if (loading) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
            className: ResultPanel_css_container,
            "aria-busy": "true",
            "aria-label": "Review Results",
            "data-testid": "review-verdict-skeleton",
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: skeletonShort
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: skeletonLine
                })
            ]
        });
    }
    if (!verdict) return null;
    var _verdict_perCriterion;
    if (!Array.isArray((_verdict_perCriterion = verdict.perCriterion) !== null && _verdict_perCriterion !== void 0 ? _verdict_perCriterion : [])) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
            className: ResultPanel_css_container,
            "aria-label": "Review Results",
            "data-testid": "review-verdict-panel",
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                    className: title,
                    children: "Review Results"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: mutedText,
                    children: "Review result could not be displayed."
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)(RawJsonDisclosure, {
                    value: verdict,
                    testId: "review-raw-json"
                })
            ]
        });
    }
    var _verdict_perCriterion1;
    const criteria = (_verdict_perCriterion1 = verdict.perCriterion) !== null && _verdict_perCriterion1 !== void 0 ? _verdict_perCriterion1 : [];
    var _verdict_overallOutcome;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
        className: ResultPanel_css_container,
        "aria-label": "Review Results",
        "data-testid": "review-verdict-panel",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: ResultPanel_css_header,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: title,
                        children: "Review Results"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: outcomeBadge(verdict.overallOutcome),
                        "data-testid": "review-overall",
                        children: (_verdict_overallOutcome = verdict.overallOutcome) !== null && _verdict_overallOutcome !== void 0 ? _verdict_overallOutcome : "PENDING"
                    })
                ]
            }),
            verdict.summary && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                children: verdict.summary
            }),
            criteria.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                className: ResultPanel_css_list,
                "aria-label": "Per-criterion results",
                children: criteria.map((c)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                        className: row,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: rowText,
                                children: [
                                    "Criterion ",
                                    c.criterionIndex + 1
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: outcomeBadge(c.outcome),
                                children: c.outcome
                            }),
                            c.evidence && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: evidence,
                                children: c.evidence
                            })
                        ]
                    }, c.criterionIndex))
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(RawJsonDisclosure, {
                value: verdict,
                testId: "review-raw-json"
            })
        ]
    });
}

;// ./src/components/backlog/BacklogResultPanels.tsx
/* __next_internal_client_entry_do_not_use__ BacklogResultPanels auto */ 


/** Formatted triage/review results; each panel self-gates on its feature flag and hides when its data is absent. */ function BacklogResultPanels(param) {
    let { item } = param;
    const verdict = latestReviewVerdict(item.linkedSessions);
    // Skeleton only while a review session is actually running, not for a review item with no session.
    const reviewRunning = !verdict && item.linkedSessions.some((s)=>s.role === "review" && !s.endedAt);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)(TriageResultPanel, {
                triageResult: item.triageResult,
                priority: item.priority,
                loading: item.triageStatus === "running"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(ReviewVerdictPanel, {
                verdict: verdict,
                loading: reviewRunning
            })
        ]
    });
}

;// ./src/components/backlog/detail/NotesSection.tsx
/* __next_internal_client_entry_do_not_use__ NotesSection auto */ 



/**
 * Free-text notes with inline editing — extracted verbatim from
 * BacklogItemDetail.tsx (Story 3.1.4, Task 3.1.4f). Default-expanded only
 * when `item.notes` is already non-empty (secondary content worth
 * surfacing by default when there's something to show).
 */ function NotesSection(param) {
    let { item, actionLoading, editingNotes, notesValue, defaultExpanded, onNotesValueChange, onStartEditing, onSave, onCancel } = param;
    var _item_notes;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(Collapsible/* CollapsibleSection */.i, {
        sectionKey: "notes",
        title: "Notes",
        defaultExpanded: defaultExpanded,
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: section,
            children: editingNotes ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("textarea", {
                        className: notesTextarea,
                        value: notesValue,
                        onChange: (e)=>onNotesValueChange(e.target.value),
                        "aria-label": "Notes",
                        "data-testid": "backlog-notes-textarea"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: inlineEditActions,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: saveNotesButton,
                                onClick: onSave,
                                disabled: actionLoading !== null,
                                "aria-busy": actionLoading === "save_notes",
                                "data-testid": "backlog-notes-save",
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionButtonLabel, {
                                    pending: actionLoading === "save_notes",
                                    label: "Save"
                                })
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: cancelNotesButton,
                                onClick: onCancel,
                                "data-testid": "backlog-notes-cancel",
                                children: "Cancel"
                            })
                        ]
                    })
                ]
            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: item.notes ? description : emptyText,
                onClick: onStartEditing,
                role: "button",
                tabIndex: 0,
                onKeyDown: (e)=>{
                    if (e.key === "Enter" || e.key === " ") onStartEditing();
                },
                "aria-label": "Click to edit notes",
                "data-testid": "backlog-notes-display",
                children: (_item_notes = item.notes) !== null && _item_notes !== void 0 ? _item_notes : "Click to add notes…"
            })
        })
    });
}

;// ./src/components/backlog/detail/ManualOverrideSection.tsx
/* __next_internal_client_entry_do_not_use__ ManualOverrideSection auto */ 
// +feature: backlog:manual-override





const ManualOverrideSection_MIN_OVERRIDE_REASON_LENGTH = 5;
/**
 * Operator escape hatch for a backlog item whose automation has gotten
 * wedged — force a status transition, or manually link a PR that shipped
 * out-of-band (no item_sessions link, so report_pr_created was never
 * callable). Always rendered regardless of status, deliberately separate
 * from the automated pipeline's status-conditional action buttons
 * (GateVerdictBox, PullRequestSection) — reuses that same override styling
 * (dangerButton/overrideForm/formTextarea) so it reads as "the same kind of
 * dangerous, deliberate action," not a new visual language.
 */ function ManualOverrideSection(param) {
    let { item, defaultExpanded, readOnly = false, onOverrideStatus, onAssociatePR } = param;
    const [toStatus, setToStatus] = (0,react.useState)("");
    const [reason, setReason] = (0,react.useState)("");
    const [statusPending, setStatusPending] = (0,react.useState)(false);
    const [statusError, setStatusError] = (0,react.useState)(null);
    const [prUrl, setPrUrl] = (0,react.useState)("");
    const [prNumber, setPrNumber] = (0,react.useState)("");
    const [prPending, setPrPending] = (0,react.useState)(false);
    const [prError, setPrError] = (0,react.useState)(null);
    const canSubmitStatus = toStatus !== "" && reason.trim().length >= ManualOverrideSection_MIN_OVERRIDE_REASON_LENGTH;
    const canAssociatePR = item.status === "review";
    const canSubmitPR = prUrl.trim() !== "" && /^\d+$/.test(prNumber.trim());
    async function handleOverrideSubmit() {
        setStatusPending(true);
        setStatusError(null);
        try {
            await onOverrideStatus(toStatus, reason.trim());
            setToStatus("");
            setReason("");
        } catch (err) {
            setStatusError((0,connectError/* getErrorMessage */.u)(err, "Status override failed. Please try again."));
        } finally{
            setStatusPending(false);
        }
    }
    async function handleAssociatePRSubmit() {
        setPrPending(true);
        setPrError(null);
        try {
            await onAssociatePR(prUrl.trim(), parseInt(prNumber, 10));
            setPrUrl("");
            setPrNumber("");
        } catch (err) {
            setPrError((0,connectError/* getErrorMessage */.u)(err, "PR association failed. Please try again."));
        } finally{
            setPrPending(false);
        }
    }
    var _item_allowedTransitions;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(Collapsible/* CollapsibleSection */.i, {
        sectionKey: "manual-override",
        title: "Manual Overrides",
        defaultExpanded: defaultExpanded,
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: overrideSection,
            "data-testid": "manual-override-section",
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: formHint,
                    children: "Operator escape hatch for recovering a wedged item. Every action here bypasses the automated pipeline and is recorded to the item's audit trail."
                }),
                canAssociatePR && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    role: "form",
                    "aria-label": "Associate existing PR",
                    className: overrideForm,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                            htmlFor: "manual-override-pr-url",
                            className: formLabel,
                            children: "Link an existing PR"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                            id: "manual-override-pr-url",
                            type: "text",
                            placeholder: "https://github.com/org/repo/pull/123",
                            value: prUrl,
                            onChange: (e)=>setPrUrl(e.target.value),
                            className: formTextarea,
                            "data-testid": "manual-override-pr-url-input"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                            id: "manual-override-pr-number",
                            type: "number",
                            min: 1,
                            placeholder: "PR number",
                            value: prNumber,
                            onChange: (e)=>setPrNumber(e.target.value),
                            className: formTextarea,
                            "data-testid": "manual-override-pr-number-input"
                        }),
                        prError && /*#__PURE__*/ (0,jsx_runtime.jsx)(InlineError, {
                            type: "transient",
                            onRetry: ()=>setPrError(null),
                            onDismiss: ()=>setPrError(null),
                            customMessage: prError
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: formActions,
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: dangerButton,
                                disabled: prPending || readOnly || !canSubmitPR,
                                "aria-busy": prPending,
                                onClick: ()=>void handleAssociatePRSubmit(),
                                "data-testid": "manual-override-pr-submit",
                                children: "Link PR"
                            })
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    role: "form",
                    "aria-label": "Override status",
                    className: overrideForm,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                            htmlFor: "manual-override-status-select",
                            className: formLabel,
                            children: "Force status to"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                            id: "manual-override-status-select",
                            value: toStatus,
                            onChange: (e)=>setToStatus(e.target.value),
                            className: formTextarea,
                            "data-testid": "manual-override-status-select",
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                    value: "",
                                    children: "Select a status…"
                                }),
                                ((_item_allowedTransitions = item.allowedTransitions) !== null && _item_allowedTransitions !== void 0 ? _item_allowedTransitions : []).map((s)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                        value: s,
                                        children: s
                                    }, s))
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                            htmlFor: "manual-override-reason",
                            className: formLabel,
                            children: "Reason (required)"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("textarea", {
                            id: "manual-override-reason",
                            rows: 3,
                            placeholder: "Explain why this item's status is being manually overridden…",
                            "aria-describedby": "manual-override-reason-hint",
                            value: reason,
                            onChange: (e)=>setReason(e.target.value),
                            className: formTextarea,
                            "data-testid": "manual-override-reason-textarea"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                            id: "manual-override-reason-hint",
                            className: formHint,
                            children: [
                                "Enter at least ",
                                ManualOverrideSection_MIN_OVERRIDE_REASON_LENGTH,
                                " characters to continue."
                            ]
                        }),
                        statusError && /*#__PURE__*/ (0,jsx_runtime.jsx)(InlineError, {
                            type: "transient",
                            onRetry: ()=>setStatusError(null),
                            onDismiss: ()=>setStatusError(null),
                            customMessage: statusError
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: formActions,
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: dangerButton,
                                disabled: statusPending || readOnly || !canSubmitStatus,
                                "aria-busy": statusPending,
                                onClick: ()=>void handleOverrideSubmit(),
                                "data-testid": "manual-override-status-submit",
                                children: "Override Status"
                            })
                        })
                    ]
                })
            ]
        })
    });
}

;// ./src/lib/hooks/useGateChecklist.ts
/* __next_internal_client_entry_do_not_use__ useGateChecklist,useGateApproval auto */ 




function mapGateStatus(g) {
    return {
        gateId: g.gateId,
        kind: g.kind,
        satisfied: g.satisfied,
        description: g.description || undefined,
        actionHint: g.actionHint || undefined,
        configError: g.configError || undefined
    };
}
/**
 * ADR-005 Decision point 2: resolves "which transition(s) is the item-detail
 * gate checklist about" as every non-`archived` entry in `allowedTransitions`
 * that has at least one configured gate — not a single "next" transition,
 * since the transition graph has no unambiguous one. `archived` is excluded
 * unconditionally: it's this codebase's universal operator escape hatch
 * (OverrideVerdict bypasses gate checks the same way), so a "what's blocking
 * X -> Archived?" row would contradict that.
 *
 * Calls GetPendingGates(itemId, to) once per remaining candidate and keeps
 * only the candidates whose response is non-empty — a target with zero gates
 * renders nothing, which is what collapses the common built-in case (most
 * edges have no configured gates) to "section does not render" without a
 * special case.
 */ async function fetchCandidates(client, itemId, targets) {
    const results = await Promise.all(targets.map(async (to)=>{
        const resp = await client.getPendingGates({
            itemId,
            toStatus: to
        });
        var _resp_gates;
        return {
            toStatus: to,
            gates: ((_resp_gates = resp.gates) !== null && _resp_gates !== void 0 ? _resp_gates : []).map(mapGateStatus)
        };
    }));
    return results.filter((c)=>c.gates.length > 0);
}
/** Runs one fetch-and-apply cycle for the effect below; returns its cleanup (cancel) function. */ function runGateFetch(itemId, targets, clientRef, param) {
    let { setCandidates, setIsLoading, setError } = param;
    if (!itemId || targets.length === 0) {
        setCandidates([]);
        setIsLoading(false);
        setError(null);
        return ()=>{};
    }
    let cancelled = false;
    if (!clientRef.current) {
        clientRef.current = (0,promise_client/* createClient */.UU)(backlog_pb/* BacklogService */.jSM, (0,transport/* getConnectTransport */.qW)());
    }
    setIsLoading(true);
    setError(null);
    fetchCandidates(clientRef.current, itemId, targets).then((results)=>{
        if (!cancelled) setCandidates(results);
    }).catch((err)=>{
        if (cancelled) return;
        console.error("[useGateChecklist] getPendingGates:", err);
        setError((0,connectError/* getErrorMessage */.u)(err, "Couldn't load pending gates"));
    }).finally(()=>{
        if (!cancelled) setIsLoading(false);
    });
    return ()=>{
        cancelled = true;
    };
}
function useGateChecklist(itemId, allowedTransitions) {
    const [candidates, setCandidates] = (0,react.useState)([]);
    const [isLoading, setIsLoading] = (0,react.useState)(true);
    const [error, setError] = (0,react.useState)(null);
    const [reloadToken, setReloadToken] = (0,react.useState)(0);
    const clientRef = (0,react.useRef)(null);
    const targets = allowedTransitions.filter((t)=>t !== "archived");
    // Stable string key so this effect doesn't refire just because the caller
    // passed a fresh array literal this render — same idiom as
    // useWatchBacklogItems.ts's statusFilterKey/categoryFilterKey.
    const targetsKey = targets.join(",");
    (0,react.useEffect)(()=>{
        return runGateFetch(itemId, targets, clientRef, {
            setCandidates,
            setIsLoading,
            setError
        });
    // targets/targetsKey intentionally paired: targetsKey is the real dep,
    // targets itself is recomputed fresh every render from allowedTransitions.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        itemId,
        targetsKey,
        reloadToken
    ]);
    const refetch = (0,react.useCallback)(()=>setReloadToken((t)=>t + 1), []);
    return {
        candidates,
        isLoading,
        error,
        refetch
    };
}
/**
 * Thin client wrapper for BacklogService.RecordGateApproval (Epic 2.4.1,
 * ADR-006 Part A), following useBacklogStagesAdmin's self-contained-client
 * convention rather than folding into useBacklogService.ts.
 */ function useGateApproval() {
    const clientRef = (0,react.useRef)(null);
    const recordApproval = (0,react.useCallback)(async (itemId, gateId, approved)=>{
        if (!clientRef.current) {
            clientRef.current = (0,promise_client/* createClient */.UU)(backlog_pb/* BacklogService */.jSM, (0,transport/* getConnectTransport */.qW)());
        }
        try {
            await clientRef.current.recordGateApproval({
                itemId,
                gateId,
                satisfiedBy: "",
                approved
            });
        } catch (err) {
            console.error("[useGateApproval] recordGateApproval:", err);
            throw new Error((0,connectError/* getErrorMessage */.u)(err, approved ? "Couldn't record approval" : "Couldn't record rejection"));
        }
    }, []);
    return {
        recordApproval
    };
}

// EXTERNAL MODULE: ./src/lib/hooks/useBacklogStages.ts
var useBacklogStages = __webpack_require__(3456);
// EXTERNAL MODULE: ./src/lib/backlog/status.ts
var backlog_status = __webpack_require__(85546);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FGateChecklist.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA7VV7Y6jIBT936fg55gsjTrtdEsfZkIFlSkCA1jbbvbdN0Ldiq2r6cwmJkbv5dxzvw7L92T7Zj7zjxj8WgBAmFEcnxHIOT3tFsC9IWGaZpZJgUAmeV2J1lJghcAR6xcI35MtvaT68hpHu8XvxbLDTBxmLoWFhl3onfs-2nX2hrKitHcepfPIJJd6aEudydKThVZjYXKpKwRqpajOsKGtkVNrqYZG4YyJAoF4Ga9pFVBMvyHt9OKocGYsNPbMKQJCCsegwrpgAoG4_VCYEM8joPDqKChpmI-F90by2rrzDSO2RCBRjlV5LdL1s493CwWvVnmkOueyQaBkhFBHPuNMIdBm9RL_ANfHkW9KZqmrlGPfaKza33upCdVDxqvvK5oPADUmrDZ37d9EQZ4Ppu0vAqe5RWClTsBIzkjAd-349hzhw4mqPR7ODoWWtSBD-zGc7rd5qOkE6iZE3cxDTeIJ2CQJcX_Owz1NwJ5D1O3jScCcFQIySyuDQEaFpfpfY9ADxDfJGJEEFh7YuwOj_ex5ZuOeaehJxj2TgcTRcddT6Jn_TzEcsCpmxXqcYIhUPm7w-EI7EWjlAwEvIj0wFoIxwZmgcGJoKiZgJ3yrlde2L6hGeomG_smt8k92pvD1rLVpC6ok8-R7qX_49Rvdq2q8I59RX4n9zdJHRmUr9RP4IhocIszgPad-1GV7Q9pze0Ou-5kIaSHmXDY0FNTDXbTuwutoJp0QD3gUsyfv0M_L9_vhsflbcfha0vz5papmTVAvVvV8rHRi-ufuz_gskZbrH6mXWwU5CgAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FGateChecklist_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA7VV7Y6jIBT936fg55gsjTrtdEsfZkIFlSkCA1jbbvbdN0Ldiq2r6cwmJkbv5dxzvw7L92T7Zj7zjxj8WgBAmFEcnxHIOT3tFsC9IWGaZpZJgUAmeV2J1lJghcAR6xcI35MtvaT68hpHu8XvxbLDTBxmLoWFhl3onfs_2nX2hrKitHcepfPIJJd6aEudydKThVZjYXKpKwRqpajOsKGtkVNrqYZG4YyJAoF4Ga9pFVBMvyHt9OKocGYsNPbMKQJCCsegwrpgAoG4_VCYEM8joPDqKChpmI_F90by2rrzDSO2RCBRjlV5LdL1s493CwWvVnmkOueyQaBkhFBHPuNMIdBm9RL_ANfHkW9KZqmrlGPfaKza33upCdVDxqvvK5oPADUmrDZ37d9EQZ4Ppu0vAqe5RWClTsBIzkjAd_349hzhw4mqPR7ODoWWtSBD_zGc7rd5qOkE6iZE3cxDTeIJ2CQJcX_Owz1NwJ5D1O3jScCcFQIySyuDQEaFpfpfY9ADxDfJGJEEFh7YuwOj_ex5ZuOeaehJxj2TgcTRcddT6Jn_TzEcsCpmxXqcYIhUPm7w_EI7EWjlAwEvIj0wFoIxwZmgcGJoKiZgJ3yrlde2L6hGeomG_smt8k92pvD1rLVpC6ok8_R7qX_49Rvdq2q8I59RX4n9zdJHRmUr9RP4IhocIszgPad_1GV7Q9pze0Ou_5kIaSHmXDY0FNTDXbTuwutoJp0QD3gUsyfv0M_L9_vhsflbcfha0vz5papmTVAvVvV8rHRi_ufuz_gskZbrH6mXWwU5CgAA_22_7D = __webpack_require__(53214);
;// ./src/components/backlog/GateChecklist.css.ts


var approveButton = '_196sqfjj _196sqfji';
var fixLink = '_196sqfjl';
var GateChecklist_css_list = '_196sqfj2';
var rejectButton = '_196sqfjk _196sqfji';
var GateChecklist_css_row = '_196sqfj4';
var rowActions = '_196sqfjh';
var rowBlocked = '_196sqfj7 _196sqfj4';
var rowConfigError = '_196sqfj8 _196sqfj4';
var rowDescription = '_196sqfjg';
var rowHeader = '_196sqfj9';
var rowIcon = '_196sqfja';
var rowIconBlocked = '_196sqfjd';
var rowIconConfigError = '_196sqfje';
var rowIconPending = '_196sqfjc';
var rowIconSatisfied = '_196sqfjb';
var rowLabel = '_196sqfjf';
var rowPending = '_196sqfj6 _196sqfj4';
var rowSatisfied = '_196sqfj5 _196sqfj4';
var GateChecklist_css_section = '_196sqfj0';
var GateChecklist_css_sectionTitle = '_196sqfj1';
var GateChecklist_css_srOnly = '_196sqfj3';
var staleStageNotice = '_196sqfjm';
;// ./src/components/backlog/GateChecklist.tsx
/* __next_internal_client_entry_do_not_use__ GateChecklist auto */ 
// +feature: backlog:gate-checklist



const GATE_KIND_CONFIG = {
    human_approval: {
        label: "Human approval"
    },
    automated_review: {
        label: "Automated review"
    },
    structural: {
        label: "Structural check"
    },
    custom: {
        label: "Custom check"
    }
};
function resolveRowState(gate, locallySatisfied) {
    if (gate.configError) return "configError";
    if (gate.satisfied || locallySatisfied.has(gate.gateId)) return "satisfied";
    return gate.kind === "human_approval" ? "pending" : "blocked";
}
const ROW_ICON = {
    satisfied: "✓",
    pending: "○",
    blocked: "✗",
    configError: "⚠"
};
const ROW_CLASS = {
    satisfied: rowSatisfied,
    pending: rowPending,
    blocked: rowBlocked,
    configError: rowConfigError
};
const ROW_ICON_CLASS = {
    satisfied: rowIconSatisfied,
    pending: rowIconPending,
    blocked: rowIconBlocked,
    configError: rowIconConfigError
};
/** One `<li>` — a single gate's status, and its Approve/Reject affordance when it's an unsatisfied human-approval gate. */ function GateRow(param) {
    let { gate, state, transitionLabel, isPending, rowError, onApprove, onReject, onDismissError, stagesSettingsHref } = param;
    const kindLabel = GATE_KIND_CONFIG[gate.kind].label;
    var _gate_description, _gate_description1;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
        role: "status",
        "aria-label": "".concat(kindLabel, " gate"),
        className: ROW_CLASS[state],
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: rowHeader,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: "".concat(rowIcon, " ").concat(ROW_ICON_CLASS[state]),
                        "aria-hidden": "true",
                        children: ROW_ICON[state]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: rowLabel,
                        children: kindLabel
                    })
                ]
            }),
            state === "configError" && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: rowDescription,
                        children: "Configuration error — this gate can't be evaluated (".concat(gate.configError, ")")
                    }),
                    stagesSettingsHref && /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                        className: fixLink,
                        href: stagesSettingsHref,
                        children: "Fix in Stages settings →"
                    })
                ]
            }),
            (state === "satisfied" || state === "blocked") && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: rowDescription,
                children: (_gate_description = gate.description) !== null && _gate_description !== void 0 ? _gate_description : state === "satisfied" ? "Satisfied" : "Blocked"
            }),
            state === "pending" && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: rowDescription,
                        children: (_gate_description1 = gate.description) !== null && _gate_description1 !== void 0 ? _gate_description1 : "Pending — click Approve below"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: rowActions,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: approveButton,
                                disabled: isPending,
                                "aria-label": "Approve human-approval gate for transition ".concat(transitionLabel),
                                onClick: onApprove,
                                type: "button",
                                children: "Approve"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: rejectButton,
                                disabled: isPending,
                                "aria-label": "Reject human-approval gate for transition ".concat(transitionLabel),
                                onClick: onReject,
                                type: "button",
                                children: "Reject"
                            })
                        ]
                    })
                ]
            }),
            rowError && /*#__PURE__*/ (0,jsx_runtime.jsx)(InlineError, {
                type: "transient",
                headline: "Couldn't record approval",
                customMessage: "".concat(rowError, "."),
                retryAriaLabel: "Try approval again",
                onRetry: onDismissError
            })
        ]
    });
}
/**
 * Story 2.10.1: generalizes GateVerdictBox.tsx's single-verdict-card pattern
 * (a config map keyed by state driving icon/label/class) into one row per
 * independently-satisfiable gate — the GitHub-branch-protection shape
 * design/ux.md's Surface 4 calls for, instead of one collapsed "Blocked"
 * state.
 *
 * aria-live lives on the `<ul>`, not on each `<li>` (each row is still
 * `role="status"` per-row for semantic labeling): a per-row live region would
 * re-announce every row on every render of a multi-gate list, which
 * design/ux.md's accessibility section explicitly calls out as the
 * over-announcement problem GateVerdictBox's single-card `aria-live` never
 * had to face.
 */ function GateChecklist(param) {
    let { gates, transitionLabel, onApprove, onReject, stagesSettingsHref } = param;
    const [pendingGateIds, setPendingGateIds] = (0,react.useState)(new Set());
    const [locallySatisfied, setLocallySatisfied] = (0,react.useState)(new Set());
    const [rowErrors, setRowErrors] = (0,react.useState)({});
    const [announcement, setAnnouncement] = (0,react.useState)("");
    if (gates.length === 0) {
        return null;
    }
    function markGatePending(gateId) {
        setPendingGateIds((prev)=>new Set(prev).add(gateId));
    }
    function clearGatePending(gateId) {
        setPendingGateIds((prev)=>{
            const next = new Set(prev);
            next.delete(gateId);
            return next;
        });
    }
    function remainingCountAfter(satisfiedGateId) {
        return gates.filter((g)=>{
            if (g.gateId === satisfiedGateId) return false;
            if (g.configError) return true;
            return !(g.satisfied || locallySatisfied.has(g.gateId));
        }).length;
    }
    async function handleApprove(gate) {
        setRowErrors((prev)=>({
                ...prev,
                [gate.gateId]: undefined
            }));
        markGatePending(gate.gateId);
        try {
            await onApprove(gate.gateId);
            const remaining = remainingCountAfter(gate.gateId);
            setLocallySatisfied((prev)=>new Set(prev).add(gate.gateId));
            const kindLabel = GATE_KIND_CONFIG[gate.kind].label;
            setAnnouncement("".concat(kindLabel, " satisfied. ").concat(remaining, " gate").concat(remaining === 1 ? "" : "s", " remaining."));
        } catch (err) {
            setRowErrors((prev)=>({
                    ...prev,
                    [gate.gateId]: err instanceof Error ? err.message : "Please try again"
                }));
        } finally{
            clearGatePending(gate.gateId);
        }
    }
    async function handleReject(gate) {
        setRowErrors((prev)=>({
                ...prev,
                [gate.gateId]: undefined
            }));
        markGatePending(gate.gateId);
        try {
            await onReject(gate.gateId);
            const kindLabel = GATE_KIND_CONFIG[gate.kind].label;
            setAnnouncement("".concat(kindLabel, " rejected."));
        } catch (err) {
            setRowErrors((prev)=>({
                    ...prev,
                    [gate.gateId]: err instanceof Error ? err.message : "Please try again"
                }));
        } finally{
            clearGatePending(gate.gateId);
        }
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
        className: GateChecklist_css_section,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                className: GateChecklist_css_sectionTitle,
                children: [
                    "What’s blocking ",
                    transitionLabel,
                    "?"
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("ul", {
                role: "list",
                "aria-label": "Pending gates",
                "aria-live": "polite",
                "aria-atomic": "true",
                className: GateChecklist_css_list,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("li", {
                        className: GateChecklist_css_srOnly,
                        children: announcement
                    }),
                    gates.map((gate)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(GateRow, {
                            gate: gate,
                            state: resolveRowState(gate, locallySatisfied),
                            transitionLabel: transitionLabel,
                            isPending: pendingGateIds.has(gate.gateId),
                            rowError: rowErrors[gate.gateId],
                            onApprove: ()=>void handleApprove(gate),
                            onReject: ()=>void handleReject(gate),
                            onDismissError: ()=>setRowErrors((prev)=>({
                                        ...prev,
                                        [gate.gateId]: undefined
                                    })),
                            stagesSettingsHref: stagesSettingsHref
                        }, gate.gateId))
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FGateBlockingSection.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xZ2JuN3BwMCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogdmFyKC0tXzE5ZXoycnozMCk7Cn0%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FGateBlockingSection_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xZ2JuN3BwMCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogdmFyKC0tXzE5ZXoycnozMCk7Cn0_22_7D = __webpack_require__(79423);
;// ./src/components/backlog/GateBlockingSection.css.ts


var GateBlockingSection_css_container = '_1gbn7pp0';
;// ./src/components/backlog/GateBlockingSection.tsx
/* __next_internal_client_entry_do_not_use__ GateBlockingSection auto */ 
// +feature: backlog:gate-blocking-section








/**
 * Exact copy from design/ux.md Surface 4 / ADR-005 Decision point 3.
 */ const FROZEN_SNAPSHOT_NOTICE = "This item is on a stage no longer in the current configuration. Transitions shown reflect the configuration when it entered this stage.";
/**
 * ADR-005 Decision point 3: must check `.enabled`, not merely slug presence —
 * useBacklogStages()'s listStages({}) returns disabled stages too, but the
 * backend's stageConfigCache treats a disabled stage identically to a
 * deleted one (ListEnabledStages/ListEnabledTransitions).
 */ function isFrozenSnapshot(stages, status) {
    return !stages.some((s)=>s.slug === status && s.enabled);
}
/** Builds the Approve/Reject callbacks GateChecklist needs from useGateApproval's single decision RPC (ADR-006 Part A). */ function useGateDecisionHandlers(recordApproval, itemId) {
    const handleApprove = (0,react.useCallback)((gateId)=>recordApproval(itemId, gateId, true), [
        recordApproval,
        itemId
    ]);
    const handleReject = (0,react.useCallback)((gateId)=>recordApproval(itemId, gateId, false), [
        recordApproval,
        itemId
    ]);
    return {
        handleApprove,
        handleReject
    };
}
function GateBlockingSectionError(param) {
    let { error, onRetry } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: GateBlockingSection_css_container,
        "data-testid": "gate-blocking-section-error",
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(InlineError, {
            type: "transient",
            headline: "Couldn't load pending gates",
            customMessage: "".concat(error, "."),
            retryAriaLabel: "Retry loading pending gates",
            onRetry: onRetry
        })
    });
}
/**
 * ADR-005: renders the item-detail "what's blocking this transition"
 * checklist — one `GateChecklist` per non-`archived` candidate transition
 * that has at least one configured gate (useGateChecklist), plus the
 * frozen-config-snapshot notice when the item's current stage is disabled or
 * absent from the live stage list (useBacklogStages).
 *
 * `[Approve]`/`[Reject]` both call ADR-006 Part A's `RecordGateApproval(itemId,
 * gateId, approved)` — approving is permanently one-shot, rejecting is
 * reversible by a later approve.
 */ function GateBlockingSection(param) {
    let { item } = param;
    var _item_allowedTransitions;
    const { candidates, isLoading, error, refetch } = useGateChecklist(item.id, (_item_allowedTransitions = item.allowedTransitions) !== null && _item_allowedTransitions !== void 0 ? _item_allowedTransitions : []);
    const { stages } = (0,useBacklogStages/* useBacklogStages */._M)();
    const { recordApproval } = useGateApproval();
    const { handleApprove, handleReject } = useGateDecisionHandlers(recordApproval, item.id);
    if (isLoading) return null;
    if (error) return /*#__PURE__*/ (0,jsx_runtime.jsx)(GateBlockingSectionError, {
        error: error,
        onRetry: refetch
    });
    if (candidates.length === 0) return null;
    const onFrozenSnapshot = isFrozenSnapshot(stages, item.status);
    // Transitions/gates are edited on the origin stage's form (StageForm.tsx
    // lists outgoing transitions via listStageTransitions(stage.slug)), so the
    // deep link targets item.status, not any candidate's toStatus.
    const stagesSettingsHref = "/settings/backlog-stages?editStage=".concat(item.status);
    const fromLabel = (0,backlog_status/* getStatusLabel */.p)(item.status);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: GateBlockingSection_css_container,
        "data-testid": "gate-blocking-section",
        children: [
            onFrozenSnapshot && /*#__PURE__*/ (0,jsx_runtime.jsx)(InlineNotice/* InlineNotice */.w, {
                message: FROZEN_SNAPSHOT_NOTICE,
                "data-testid": "gate-blocking-frozen-snapshot-notice"
            }),
            candidates.map((candidate)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(GateChecklist, {
                    gates: candidate.gates,
                    transitionLabel: "".concat(fromLabel, " → ").concat((0,backlog_status/* getStatusLabel */.p)(candidate.toStatus)),
                    onApprove: handleApprove,
                    onReject: handleReject,
                    stagesSettingsHref: stagesSettingsHref
                }, candidate.toStatus))
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Finsights%2FItemBudgetWarning.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xbTNsNDV5MCB7CiAgZGlzcGxheTogZmxleDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeik7CiAgcGFkZGluZzogdmFyKC0tXzE5ZXoycnoyeikgdmFyKC0tXzE5ZXoycnozMCk7CiAgYm9yZGVyLXJhZGl1czogdmFyKC0tXzE5ZXoycnozNik7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnp4KTsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyenkpOwp9Ci5fMW0zbDQ1eTEgewogIGZsZXgtc2hyaW5rOiAwOwp9Ci5fMW0zbDQ1eTIgewogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6eik7CiAgZm9udC13ZWlnaHQ6IHZhcigtLV8xOWV6MnJ6M2cpOwp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Finsights_2FItemBudgetWarning_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xbTNsNDV5MCB7CiAgZGlzcGxheTogZmxleDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeik7CiAgcGFkZGluZzogdmFyKC0tXzE5ZXoycnoyeikgdmFyKC0tXzE5ZXoycnozMCk7CiAgYm9yZGVyLXJhZGl1czogdmFyKC0tXzE5ZXoycnozNik7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnp4KTsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyenkpOwp9Ci5fMW0zbDQ1eTEgewogIGZsZXgtc2hyaW5rOiAwOwp9Ci5fMW0zbDQ1eTIgewogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6eik7CiAgZm9udC13ZWlnaHQ6IHZhcigtLV8xOWV6MnJ6M2cpOwp9_22_7D = __webpack_require__(30375);
;// ./src/app/insights/ItemBudgetWarning.css.ts

var ItemBudgetWarning_css_banner = '_1m3l45y0';
var ItemBudgetWarning_css_icon = '_1m3l45y1';
var ItemBudgetWarning_css_text = '_1m3l45y2';
// EXTERNAL MODULE: ./src/app/insights/insightsFormatters.ts
var insightsFormatters = __webpack_require__(97900);
;// ./src/app/insights/ItemBudgetWarning.tsx
// +feature: backlog:item-budget-warning
/* __next_internal_client_entry_do_not_use__ ItemBudgetWarning auto */ 


/**
 * Per-item budget warning banner (design/ux.md Surface D2) — visually
 * consistent with ProjectedCostCard's warning styling but a genuinely
 * separate component: it answers "is THIS item over budget," not "is total
 * monthly spend over budget." Renders null (no empty box) whenever no
 * threshold is configured, or the item hasn't crossed it yet. Advisory only:
 * no dismiss action, since dismissing wouldn't change the underlying fact.
 */ function ItemBudgetWarning(param) {
    let { thresholdUsd, totalCostUsd } = param;
    if (thresholdUsd === undefined) return null;
    if (totalCostUsd < thresholdUsd) return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: ItemBudgetWarning_css_banner,
        role: "status",
        "data-testid": "item-budget-warning",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: ItemBudgetWarning_css_icon,
                "aria-hidden": "true",
                children: "⚠"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: ItemBudgetWarning_css_text,
                children: "Over budget: ".concat((0,insightsFormatters/* fmtCost */.F0)(totalCostUsd), " spent, threshold ").concat((0,insightsFormatters/* fmtCost */.F0)(thresholdUsd))
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/app/insights/ItemStageCostTable.css.ts
var ItemStageCostTable_css = __webpack_require__(10001);
;// ./src/app/insights/ItemStageCostTable.tsx
// +feature: insights-dashboard



function roleLabel(role) {
    if (!role) return "Unattributed";
    return role.charAt(0).toUpperCase() + role.slice(1);
}
/**
 * Story 5.2.3: one backlog item's cost broken down by pipeline stage,
 * sourced from GetInsightsSummaryResponse.role_breakdown[].items filtered to
 * this item's ID (no new RPC — see BacklogItemDetail.tsx's wiring). Renders
 * nothing (not an empty table) when `rows` is empty — an item with no cost
 * data anywhere is legitimately absent from this surface, distinct from a
 * failed fetch, which the caller renders as its own error state.
 */ function ItemStageCostTable(param) {
    let { rows } = param;
    if (rows.length === 0) return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: ItemStageCostTable_css/* card */.Nr,
        "data-testid": "item-stage-cost-table",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: ItemStageCostTable_css/* title */.DD,
                children: "Cost by Stage"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("table", {
                className: ItemStageCostTable_css/* table */.tp,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("thead", {
                        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                    className: ItemStageCostTable_css.th,
                                    scope: "col",
                                    children: "Stage"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                    className: ItemStageCostTable_css/* thRight */.HN,
                                    scope: "col",
                                    children: "Cost"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                    className: ItemStageCostTable_css/* thRight */.HN,
                                    scope: "col",
                                    children: "Runs"
                                })
                            ]
                        })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("tbody", {
                        children: rows.map((r)=>{
                            const isRework = r.sessionCount > 1;
                            const runsLabel = "".concat(r.sessionCount, " run").concat(r.sessionCount === 1 ? "" : "s");
                            return /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("td", {
                                        className: ItemStageCostTable_css.td,
                                        children: [
                                            roleLabel(r.role),
                                            r.unpricedSessionCount > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                className: ItemStageCostTable_css/* emphasis */.lN,
                                                "aria-label": "".concat(r.unpricedSessionCount, " unpriced session").concat(r.unpricedSessionCount === 1 ? "" : "s"),
                                                children: [
                                                    " ",
                                                    "⚠ ",
                                                    r.unpricedSessionCount,
                                                    " unpriced"
                                                ]
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                        className: ItemStageCostTable_css/* tdRight */.Pz,
                                        children: (0,insightsFormatters/* fmtCost */.F0)(r.costUsd)
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                        className: ItemStageCostTable_css/* tdRight */.Pz,
                                        children: isRework ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                            className: ItemStageCostTable_css/* emphasis */.lN,
                                            "aria-label": "".concat(runsLabel, ", rework indicator"),
                                            children: [
                                                "⚠ ",
                                                runsLabel
                                            ]
                                        }) : runsLabel
                                    })
                                ]
                            }, r.role);
                        })
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/hooks/useInsightsService.ts
var useInsightsService = __webpack_require__(81240);
// EXTERNAL MODULE: ./src/components/ui/Skeleton.tsx + 1 modules
var Skeleton = __webpack_require__(95025);
;// ./src/components/backlog/BacklogItemDetail.tsx
/* __next_internal_client_entry_do_not_use__ STEER_AUDIT_UNAVAILABLE_MESSAGE,steerFailureMessage,BacklogItemDetail auto */ 
// +feature: backlog:item-detail






































































const PRIORITY_LABELS = {
    1: "P1",
    2: "P2",
    3: "P3",
    4: "P4",
    5: "P5"
};
const ACTION_SUCCESS_MESSAGES = {
    mark_ready: "Marked ready.",
    trigger_triage: "Triage started.",
    spawn_session: "Session started.",
    spawn_session_autonomous: "Autonomous session started.",
    restart_session: "Session restarted.",
    approve_plan: "Plan approved.",
    retry_triage: "Triage re-triggered.",
    mark_done: "Marked done.",
    override_done: "Overridden to done.",
    re_review: "Re-review triggered.",
    ship_pr: "PR created.",
    archive: "Archived.",
    unarchive: "Unarchived — back in the idea column. Needs a fresh session.",
    reopen: "Reopened for review.",
    send_back_idea: "Sent back to triage."
};
/**
 * The server refuses a steer to a hidden review session when it cannot write
 * the audit line first (Internal, "audit log unavailable"). Show that as one
 * explicit sentence instead of the raw RPC error.
 */ const STEER_AUDIT_UNAVAILABLE_MESSAGE = "Audit log unavailable, steer not sent";
function steerFailureMessage(raw) {
    return /audit log unavailable/i.test(raw) ? STEER_AUDIT_UNAVAILABLE_MESSAGE : raw;
}
/** Inert divider between accordion groups: aria-hidden, and not an AccordionItem, so it stays out of keyboard nav. */ function GroupLabel(param) {
    let { children } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: sectionGroupLabel,
        "aria-hidden": "true",
        "data-testid": "section-group-label-".concat(children.toLowerCase().replace(/ /g, "-")),
        children: children
    });
}
function BacklogItemDetail(param) {
    let { itemId, onClose } = param;
    var _pipelineModes_find;
    const { track } = (0,analytics/* useAnalytics */.st)();
    const { getBacklogItem, transitionStatus, triggerTriage, createBacklogItemFromChat, cancelTriage, spawnSessionFromItem, approvePlan, rejectPlan, overrideVerdict, triggerReReview, triggerShipPR, submitManualReview, archiveBacklogItem, unarchiveBacklogItem, deleteBacklogItem, updateBacklogItem, listPipelineModes, lastError } = (0,useBacklogService/* useBacklogService */.gG)();
    const { deleteSession, updateSession } = (0,useSessionService/* useSessionService */.D)();
    const { showActionToast } = (0,NotificationContext/* useNotifications */.E$)();
    const [item, setItem] = (0,react.useState)(null);
    const [loading, setLoading] = (0,react.useState)(true);
    const [error, setError] = (0,react.useState)(null);
    // Guards the Edit action against the #146 loading-gate's other half: `item`
    // can become non-null (and the full detail view, including Edit, render)
    // from the shared backlogItemsSlice store's synchronous hydration (the
    // liveRawItem effect below) *before* this item's own authoritative
    // getBacklogItem() fetch has resolved even once. If that store entry is a
    // stale/incomplete snapshot (e.g. cached before triage assigned a
    // category), opening Edit against it seeds BacklogItemForm's local state
    // from stale data — and, worse, a correction that lands afterward while
    // editMode is true only gets buffered (Story 5.3.2), never applied to the
    // open form. Blocking Edit specifically (not the read-only view) until at
    // least one authoritative response has confirmed `item` closes that
    // window without reintroducing #146's remount/loading-flash regression.
    const [itemConfirmed, setItemConfirmed] = (0,react.useState)(false);
    /** The action key currently in flight (e.g. "mark_ready"), or null when idle. */ const [actionLoading, setActionLoading] = (0,react.useState)(null);
    const [editMode, setEditMode] = (0,react.useState)(false);
    const [deletingSessionId, setDeletingSessionId] = (0,react.useState)(null);
    const [steeringSessionId, setSteeringSessionId] = (0,react.useState)(null);
    const [copiedField, setCopiedField] = (0,react.useState)(null);
    // Epic 3.4 "what ran" surface: the currently-fetched mode list, used only
    // to resolve a session's frozen pipelineModeSnapshot slug to a
    // human-readable name (and to detect content drift). Fetch failure
    // degrades every session's "Pipeline" group to the unrecognized-mode
    // fallback rather than blocking the rest of the item detail view.
    const [pipelineModes, setPipelineModes] = (0,react.useState)([]);
    // Mirrors `item` on every render (assignment during render, not in an
    // effect, so it's always current by the time an in-flight load() resolves)
    // — lets load() compare a freshly-fetched result against whatever is
    // currently displayed without needing `item` in its own dependency array.
    const itemRef = (0,react.useRef)(null);
    itemRef.current = item;
    // Monotonic guard against out-of-order load() resolution (pitfall #4):
    // each call captures its own sequence number, and only the response
    // matching the most-recently-issued call is applied.
    const loadSeqRef = (0,react.useRef)(0);
    const mountedRef = (0,react.useRef)(true);
    (0,react.useEffect)(()=>{
        mountedRef.current = true;
        return ()=>{
            mountedRef.current = false;
        };
    }, []);
    // copyTimerRef cancels a still-pending prior confirmation timeout so
    // clicking Copy ID then Copy Link within the 1.5s window doesn't let the
    // first timer clear the second button's confirmation state early.
    const copyTimerRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        return ()=>{
            if (copyTimerRef.current) clearTimeout(copyTimerRef.current);
        };
    }, []);
    const handleCopy = (0,react.useCallback)((field, value)=>{
        void (0,clipboard/* copyToClipboard */.l)(value).then((ok)=>{
            if (!ok || !mountedRef.current) return;
            if (copyTimerRef.current) clearTimeout(copyTimerRef.current);
            setCopiedField(field);
            copyTimerRef.current = setTimeout(()=>{
                if (mountedRef.current) setCopiedField(null);
                copyTimerRef.current = null;
            }, 1500);
        });
    }, []);
    // Review changes modal
    const [showChangesModal, setShowChangesModal] = (0,react.useState)(false);
    const changesModalTriggerRef = (0,react.useRef)(null);
    // File browser modal
    const [showFileBrowser, setShowFileBrowser] = (0,react.useState)(false);
    const fileBrowserTriggerRef = (0,react.useRef)(null);
    // Jules dispatch dialog (Story 3.2.2)
    const [showJulesDispatch, setShowJulesDispatch] = (0,react.useState)(false);
    const julesDispatchTriggerRef = (0,react.useRef)(null);
    const [julesConfig, setJulesConfig] = (0,react.useState)(null);
    // Manual review form
    const [showManualReview, setShowManualReview] = (0,react.useState)(false);
    const [manualReviewOutcome, setManualReviewOutcome] = (0,react.useState)("PASS");
    const [manualReviewSummary, setManualReviewSummary] = (0,react.useState)("");
    // Notes inline editing
    const [editingNotes, setEditingNotes] = (0,react.useState)(false);
    const [notesValue, setNotesValue] = (0,react.useState)("");
    // Triage progress tracking
    const [triageElapsedSeconds, setTriageElapsedSeconds] = (0,react.useState)(0);
    // Called once here (not inside LifecycleSummary) so this component owns the
    // single fetch/poll — mirrors board/page.tsx calling it once at the page
    // level and passing the resolved StuckBacklogItem down as a prop, rather
    // than LifecycleSummary standing up its own transport/client and 60s poll
    // on every remount (this component remounts via `key={selectedItemId}` on
    // every backlog item click — see stapler-squad PR #208 review).
    const { items: stuckItems, triggerRemediationNow, overrideClaimBlock } = (0,useStuckBacklogItems/* useStuckBacklogItems */.d)();
    // BUG-105: an item can have several simultaneous open StuckBacklogItem rows
    // (e.g. BOUNCING + BOUNCE_CAP_EXHAUSTED + MULTIPLE_REASONS all open at
    // once) — `summarizeStuckItemGroup` resolves the SAME shared-priority
    // primary reason BacklogBoard/BacklogItemCard resolve for the same item,
    // instead of this component picking array order 0 (`.find()`) on its own.
    const stuckItemGroup = item ? stuckItems.filter((i)=>i.itemId === item.id) : [];
    const stuckSummary = (0,stuckReason/* summarizeStuckItemGroup */.sh)(stuckItemGroup);
    const stuckItem = stuckSummary === null || stuckSummary === void 0 ? void 0 : stuckSummary.primary;
    // Version control state for the most recent work session's worktree.
    const latestWorkSession = useCurrentWorkSession(item);
    var _item_linkedSessions;
    // Surfaces the "most recent work session" heuristic's ambiguity when more than
    // one work session is currently active — the heuristic above is unchanged, this
    // only makes it visible via VcsWidgetHeader's "N active sessions" indicator.
    const activeWorkSessionCount = ((_item_linkedSessions = item === null || item === void 0 ? void 0 : item.linkedSessions) !== null && _item_linkedSessions !== void 0 ? _item_linkedSessions : []).filter((s)=>s.role === "work" && !s.endedAt).length;
    var _item_linkedSessions1;
    // Story 3.2.2: "Dispatch to Jules" gate — see resolveJulesDispatchGate's
    // own doc comment for the precedence order. Extracted to
    // lib/backlog/julesDispatchGate.ts (mirrors derivePlanReviewStatus) so it
    // is unit-testable without rendering this whole component.
    const julesDispatchGate = resolveJulesDispatchGate(julesConfig, (_item_linkedSessions1 = item === null || item === void 0 ? void 0 : item.linkedSessions) !== null && _item_linkedSessions1 !== void 0 ? _item_linkedSessions1 : []);
    var _latestWorkSession_sessionId;
    const { data: vcsStatus } = (0,useVcsStatus/* useVcsStatus */.Q)((_latestWorkSession_sessionId = latestWorkSession === null || latestWorkSession === void 0 ? void 0 : latestWorkSession.sessionId) !== null && _latestWorkSession_sessionId !== void 0 ? _latestWorkSession_sessionId : "", (0,config/* getApiBaseUrl */.h)());
    // Fallback for once the live session's worktree has been cleaned up (the normal
    // state for a done item) — vcsStatus above comes back null in that case since
    // useVcsStatus needs a live in-memory Instance to query.
    const { data: shipStatus } = useBacklogItemShipStatus(!vcsStatus && item ? item.id : "");
    const vcsWidgetData = vcsStatus ? (0,adapters/* fromSessionVcs */.tS)(vcsStatus) : shipStatus ? (0,adapters/* fromShipStatus */.fP)(shipStatus) : null;
    // D6 fix: the current work session's resolved pipeline mode, surfaced as
    // a compact badge in LifecycleSummary (Task 3.1.4g) — same value the
    // Sessions section resolves per-row, just also glanceable at the top.
    const pipelineDisplay = latestWorkSession ? resolvePipelineModeDisplay(latestWorkSession, pipelineModes) : undefined;
    var _pipelineModes_find_name;
    // CONFIGURABILITY GAP fix (UX audit 2026-09-11): the item's own configured
    // pipeline mode, resolved to a display name for LifecycleSummary's
    // automation-profile chips. Distinct from pipelineDisplay above — that
    // reflects what actually ran in the latest work session (or is undefined
    // before any session exists), while this reflects the item's current
    // configuration regardless of session history, so it's the only glanceable
    // signal available before a session has ever spawned.
    const configuredPipelineModeName = (item === null || item === void 0 ? void 0 : item.pipelineMode) ? (_pipelineModes_find_name = (_pipelineModes_find = pipelineModes.find((m)=>m.slug === item.pipelineMode)) === null || _pipelineModes_find === void 0 ? void 0 : _pipelineModes_find.name) !== null && _pipelineModes_find_name !== void 0 ? _pipelineModes_find_name : item.pipelineMode : undefined;
    // Story 5.2.3: this item's cost broken down by stage, sourced from
    // GetInsightsSummaryResponse.role_breakdown[].items filtered to item.id —
    // no new RPC (per plan.md), reusing the same Insights fetch the
    // dashboard's own StageCostChart consumes. This is BacklogItemDetail.tsx's
    // first Insights-summary fetch (ux.md Surface I), so its own
    // loading/error states are handled explicitly below rather than assumed
    // to piggyback on an existing fetch.
    const { summary: insightsSummary, loading: insightsLoading, error: insightsError } = (0,useInsightsService/* useInsightsSummary */.VT)();
    var _insightsSummary_roleBreakdown;
    const itemStageCostRows = ((_insightsSummary_roleBreakdown = insightsSummary === null || insightsSummary === void 0 ? void 0 : insightsSummary.roleBreakdown) !== null && _insightsSummary_roleBreakdown !== void 0 ? _insightsSummary_roleBreakdown : []).map((bucket)=>{
        const entry = bucket.items.find((i)=>i.itemId === (item === null || item === void 0 ? void 0 : item.id));
        return entry ? {
            role: bucket.sessionRole,
            costUsd: entry.estimatedCostUsd,
            sessionCount: entry.sessionCount,
            unpricedSessionCount: entry.unpricedSessionCount
        } : null;
    }).filter((row)=>row !== null);
    // Epic 5.3 (Story 5.3.1, backlog-event-driven-updates): live updates
    // replace the old 5s poll entirely. Subscribed unfiltered (no status/
    // category filter) so this panel keeps showing the item's current state
    // even if a list/board filter elsewhere would have excluded it (design/
    // ux.md §3). The hook's own `items` return value is unused here — it
    // exists only to keep the shared store hydrated/connected; this panel
    // reads the single item it cares about straight off the store below so
    // unrelated item updates elsewhere never cause this component to re-run
    // (selectBacklogItemById only changes reference when THIS item's store
    // entry changes, unlike the hook's fully-remapped `items` array).
    // Task 6.2.1c: connectionState is captured here to mount the
    // ConnectionIndicator in this panel's header (ux.md §3 wireframe, UX AC #20).
    const { connectionState } = (0,useWatchBacklogItems/* useWatchBacklogItems */.E)();
    const liveRawItem = (0,store/* useAppSelector */.GV)((state)=>(0,backlogItemsSlice/* selectBacklogItemById */.zc)(state, itemId));
    // Buffered-update state (Story 5.3.2): while editMode is true, an incoming
    // live update is NOT applied to the visible item/form — it's stashed here
    // and offered via an InlineNotice "Reload" action instead, so it can't
    // silently clobber an in-progress edit.
    const [bufferedItem, setBufferedItem] = (0,react.useState)(null);
    // A Save was attempted while a buffered update was pending — show the
    // warn-before-overwrite confirmation instead of calling the save RPC.
    const [saveConfirmPending, setSaveConfirmPending] = (0,react.useState)(false);
    const [pendingSaveData, setPendingSaveData] = (0,react.useState)(null);
    // Terminal-state (Task 5.3.1c): set when an ArchivedEvent/RemovedEvent
    // arrives for this item from the separate raw watch below.
    const [terminalState, setTerminalState] = (0,react.useState)(null);
    (0,react.useEffect)(()=>{
        if (!liveRawItem) return;
        const mapped = (0,useBacklogService/* mapBacklogItem */.B5)(liveRawItem);
        if (editMode) {
            setBufferedItem(mapped);
            return;
        }
        // Guard against a stale live-store entry stomping more-current state
        // (pitfall #2's other half, alongside load()'s matching guard below):
        // backlogItemsSlice's own upsertItem guard only protects the 2nd+ write
        // for a given item id — the *first* live event this component observes
        // for an item (e.g. a "created" event delivered, after a connection
        // delay, later than a separately-issued load()/GetBacklogItem fetch
        // already completed) has no `existing` entry to compare against there,
        // so it's accepted into the store unconditionally and would otherwise
        // unconditionally overwrite this component's already-fresher `item`.
        const current = itemRef.current;
        const currentMs = (current === null || current === void 0 ? void 0 : current.updatedAt) ? new Date(current.updatedAt).getTime() : 0;
        const mappedMs = mapped.updatedAt ? new Date(mapped.updatedAt).getTime() : 0;
        if (current && mappedMs < currentMs) return;
        setItem(mapped);
        var _mapped_notes;
        setNotesValue((_mapped_notes = mapped.notes) !== null && _mapped_notes !== void 0 ? _mapped_notes : "");
    // editMode is read from the closure at the time this effect actually
    // fires (i.e. whenever liveRawItem changes) — it deliberately does NOT
    // belong in the dependency array. Depending on it would re-run this
    // effect on every editMode toggle even when no new live item arrived,
    // which would re-apply a possibly-stale `liveRawItem` right as edit mode
    // closes and momentarily stomp a just-completed save.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        liveRawItem
    ]);
    // Once editMode flips back to false (Cancel, or a completed Save), apply
    // any update that was buffered while editing — design/ux.md §6's "or exits
    // edit mode" path.
    const prevEditModeRef = (0,react.useRef)(editMode);
    (0,react.useEffect)(()=>{
        if (prevEditModeRef.current && !editMode && bufferedItem) {
            setItem(bufferedItem);
            var _bufferedItem_notes;
            setNotesValue((_bufferedItem_notes = bufferedItem.notes) !== null && _bufferedItem_notes !== void 0 ? _bufferedItem_notes : "");
            setBufferedItem(null);
        }
        prevEditModeRef.current = editMode;
    }, [
        editMode,
        bufferedItem
    ]);
    // Task 5.3.1c: a separate, item-scoped raw event subscription dedicated to
    // detecting BacklogItemArchivedEvent/BacklogItemRemovedEvent for this item.
    // useWatchBacklogItems.ts intentionally does NOT dispatch itemArchived into
    // backlogItemsSlice (see that hook's file header, note 1) — item_archived
    // carries no full BacklogItem payload, so there is nothing meaningful to
    // upsert, and the design defers this to component-level handling. There is
    // also no server-side item-id filter on WatchBacklogItemsRequest, so this
    // watches unfiltered and matches events against `itemId` client-side. This
    // is a lightweight, single-purpose stream — unlike useWatchBacklogItems.ts
    // it does not implement exponential-backoff reconnect/afterSeq replay; a
    // dropped connection simply stops detecting further archive/removal for
    // this item until the component remounts.
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
                    if (event.event.case === "itemArchived" && event.event.value.itemId === itemId) {
                        setTerminalState("archived");
                    } else if (event.event.case === "itemRemoved" && event.event.value.itemId === itemId) {
                        setTerminalState("removed");
                    }
                }
            } catch (err) {
                if (err instanceof Error && err.name === "AbortError") return;
                if (abortController.signal.aborted) return;
                console.error("[BacklogItemDetail] terminal-state watch stream error:", err);
            }
        };
        void watchTerminal();
        return ()=>{
            abortController.abort();
        };
    }, [
        itemId
    ]);
    // Task 3.1.4i: every sibling CollapsibleSection (Reviewing/Pull Request
    // from Story 3.1.2, Description from 3.1.3, the rest from 3.1.4) shares
    // one CollapsibleGroup so their headers get Home/End/Arrow roving-
    // tabindex nav. Persistence (Story 1.1.1's contract) is threaded through
    // as a controlled `value`/`onValueChange` on the group, backed by
    // useSectionExpandState per section key — the same
    // `backlog-detail-section-${itemId}-${sectionKey}` localStorage
    // convention used everywhere else.
    //
    // `item` is still null on this component's very first render (load()
    // resolves asynchronously), so a status-derived default computed here
    // would always evaluate against a not-yet-loaded item and get locked in
    // as false by useSectionExpandState's one-time useState initializer —
    // every status-dependent section would default to false regardless of
    // the actual loaded status. Sections with a static default (not
    // status-dependent) are unaffected and get it directly.
    const [reviewingExpanded, setReviewingExpanded] = useSectionExpandState(itemId, "reviewing", false);
    const [lastReviewResultExpanded, setLastReviewResultExpanded] = useSectionExpandState(itemId, "last-review-result", false);
    const [pullRequestExpanded, setPullRequestExpanded] = useSectionExpandState(itemId, "pull-request", true);
    const [planArtifactsExpanded, setPlanArtifactsExpanded] = useSectionExpandState(itemId, "plan-artifacts", false);
    const [versionControlExpanded, setVersionControlExpanded] = useSectionExpandState(itemId, "version-control", false);
    const [sessionsExpanded, setSessionsExpanded] = useSectionExpandState(itemId, "sessions", true);
    const [workflowExpanded, setWorkflowExpanded] = useSectionExpandState(itemId, "workflow", false);
    const [progressHistoryExpanded, setProgressHistoryExpanded] = useSectionExpandState(itemId, "progress-history", false);
    const [activityLogExpanded, setActivityLogExpanded] = useSectionExpandState(itemId, "activity-log", false);
    const [notesExpanded, setNotesExpanded] = useSectionExpandState(itemId, "notes", false);
    const [descriptionExpanded, setDescriptionExpanded] = useSectionExpandState(itemId, "description", true);
    const [sourceExpanded, setSourceExpanded] = useSectionExpandState(itemId, "source", false);
    // Status-dependent default (review/pr_pending) is applied in the initial-expand effect below.
    const [acCriteriaExpanded, setAcCriteriaExpanded] = useSectionExpandState(itemId, "ac-criteria", false);
    const [manualOverrideExpanded, setManualOverrideExpanded] = useSectionExpandState(itemId, "manual-override", false);
    // Story 3.1.5: applies each status-dependent section's real default
    // exactly once, the first time `item` becomes available after this
    // instance's initial load — never again afterward (guarded by the ref),
    // so a later live update's status change can never retroactively re-open a
    // section the user has since collapsed. Combined with Story 3.1.1's
    // `key={itemId}` remount at the call site, this ref naturally resets per
    // item with no extra plumbing. Only opens a section when the user has no
    // existing persisted preference for it (a direct localStorage read,
    // bypassing useSectionExpandState's own already-applied fallback) — it
    // must never clobber a preference from a previous visit to this same
    // item.
    const initialExpandAppliedRef = (0,react.useRef)(false);
    (0,react.useEffect)(()=>{
        if (initialExpandAppliedRef.current || !item) return;
        initialExpandAppliedRef.current = true;
        const hasStoredPreference = (sectionKey)=>{
            try {
                return localStorage.getItem("backlog-detail-section-".concat(itemId, "-").concat(sectionKey)) !== null;
            } catch (e) {
                return false;
            }
        };
        if (item.status === "review" && !hasStoredPreference("reviewing")) {
            setReviewingExpanded(true);
        }
        if (item.status !== "review" && item.gateVerdict && !hasStoredPreference("last-review-result")) {
            setLastReviewResultExpanded(true);
        }
        // A set prUrl expands it for every status (done/archived included) so the
        // PR is visible; a stored manual choice still wins.
        if (([
            "in_progress",
            "review",
            "pr_pending"
        ].includes(item.status) || !!item.prUrl) && !hasStoredPreference("version-control")) {
            setVersionControlExpanded(true);
        }
        if ([
            "review",
            "pr_pending"
        ].includes(item.status) && !hasStoredPreference("ac-criteria")) {
            setAcCriteriaExpanded(true);
        }
        if (item.notes && !hasStoredPreference("notes")) {
            setNotesExpanded(true);
        }
    // Deliberately omits setExpanded functions from deps — they're stable
    // (useCallback in useSectionExpandState) and including them would
    // invite a lint-driven "fix" that reintroduces a re-run per render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        item,
        itemId
    ]);
    const sectionExpandEntries = [
        [
            "reviewing",
            reviewingExpanded,
            setReviewingExpanded
        ],
        [
            "last-review-result",
            lastReviewResultExpanded,
            setLastReviewResultExpanded
        ],
        [
            "pull-request",
            pullRequestExpanded,
            setPullRequestExpanded
        ],
        [
            "description",
            descriptionExpanded,
            setDescriptionExpanded
        ],
        [
            "plan-artifacts",
            planArtifactsExpanded,
            setPlanArtifactsExpanded
        ],
        [
            "version-control",
            versionControlExpanded,
            setVersionControlExpanded
        ],
        [
            "sessions",
            sessionsExpanded,
            setSessionsExpanded
        ],
        [
            "workflow",
            workflowExpanded,
            setWorkflowExpanded
        ],
        [
            "progress-history",
            progressHistoryExpanded,
            setProgressHistoryExpanded
        ],
        [
            "activity-log",
            activityLogExpanded,
            setActivityLogExpanded
        ],
        [
            "notes",
            notesExpanded,
            setNotesExpanded
        ],
        [
            "source",
            sourceExpanded,
            setSourceExpanded
        ],
        [
            "ac-criteria",
            acCriteriaExpanded,
            setAcCriteriaExpanded
        ],
        [
            "manual-override",
            manualOverrideExpanded,
            setManualOverrideExpanded
        ]
    ];
    const openSectionKeys = sectionExpandEntries.filter((param)=>{
        let [, expanded] = param;
        return expanded;
    }).map((param)=>{
        let [key] = param;
        return key;
    });
    const handleGroupValueChange = (next)=>{
        const nextSet = new Set(next);
        for (const [key, expanded, setExpanded] of sectionExpandEntries){
            const shouldBeExpanded = nextSet.has(key);
            if (shouldBeExpanded !== expanded) setExpanded(shouldBeExpanded);
        }
    };
    const load = (0,react.useCallback)(async ()=>{
        const seq = ++loadSeqRef.current;
        setLoading(true);
        setError(null);
        try {
            const result = await getBacklogItem(itemId);
            // Drop this response if it's not the most recently issued load() call
            // (pitfall #4 — resolution order isn't guaranteed to match call order)
            // or the component has since unmounted.
            if (!mountedRef.current || seq !== loadSeqRef.current) return;
            if (!result) {
                setError("Item not found.");
            } else {
                // Guard against stomping a fresher live-watch/optimistic update with
                // an older fetched result (pitfall #2) — `>=` is intentional: a
                // same-millisecond result from the user's own just-triggered
                // mutation must still apply, only a strictly *older* one is dropped.
                const current = itemRef.current;
                const currentMs = (current === null || current === void 0 ? void 0 : current.updatedAt) ? new Date(current.updatedAt).getTime() : 0;
                const resultMs = result.updatedAt ? new Date(result.updatedAt).getTime() : 0;
                if (!current || resultMs >= currentMs) {
                    setItem(result);
                    var _result_notes;
                    setNotesValue((_result_notes = result.notes) !== null && _result_notes !== void 0 ? _result_notes : "");
                }
                // Set regardless of whether this particular result "won" the
                // staleness comparison above: either way, the server has now been
                // consulted at least once for this itemId, so whatever `item` holds
                // has been reconciled against it — safe to allow editing.
                setItemConfirmed(true);
            }
        } catch (e) {
            if (mountedRef.current) setError((0,connectError/* getErrorMessage */.u)(e, "Failed to load item."));
        } finally{
            if (mountedRef.current) setLoading(false);
        }
    }, [
        itemId,
        getBacklogItem
    ]);
    // Re-arms the itemConfirmed gate whenever the viewed item changes, so a
    // second item opened in the same mounted component instance can't inherit
    // the previous item's already-confirmed state.
    (0,react.useEffect)(()=>{
        setItemConfirmed(false);
    }, [
        itemId
    ]);
    (0,react.useEffect)(()=>{
        void load();
    }, [
        load
    ]);
    // Extracted verbatim from the inline session-delete onClick handler
    // (Story 3.1.4, Task 3.1.4c) so SessionsSection stays a props-down/
    // callbacks-up presentational component.
    const handleDeleteSession = (0,react.useCallback)(async (s)=>{
        if (!item) return;
        if (!confirm("Delete this session? This cannot be undone.")) return;
        const toastKey = "".concat(item.id, ":delete_session:").concat(s.sessionId);
        setDeletingSessionId(s.sessionId);
        try {
            if (s.role === "triage") {
                await cancelTriage(item.id);
            } else {
                track({
                    name: "backlog_delete_session",
                    category: "user_action",
                    component: "BacklogItemDetail",
                    labels: {
                        role: s.role
                    }
                });
                await deleteSession(s.sessionId, true);
            }
            showActionToast("Session deleted.", "success", toastKey);
            await load();
        } catch (err) {
            const msg = (0,connectError/* getErrorMessage */.u)(err, "Failed to delete session.");
            if (mountedRef.current) setError(msg);
            showActionToast(msg, "error", toastKey);
        } finally{
            if (mountedRef.current) setDeletingSessionId(null);
        }
    }, [
        item,
        cancelTriage,
        track,
        deleteSession,
        showActionToast,
        load
    ]);
    // Epic 2.2 (Story 2.2.2, ADR-002): steers a live work/review session via
    // the same widened UpdateSession RPC/hook the general session list's own
    // Steer dialog uses (AC7). Mirrors handleDeleteSession's shape.
    // updateSession() itself swallows RPC errors and returns null rather than
    // throwing (it dispatches the real failure message to the shared
    // session-service redux error slice instead — see useSessionService.ts's
    // updateSession) — so failure is re-thrown here, letting SessionsSection's
    // inline composer (which awaits onSteerSession) catch it, keep itself
    // open, and surface the error instead of closing optimistically. Read the
    // real message via store.getState() rather than a memoized selector value
    // (e.g. useAppSelector) — updateSession's dispatch happens synchronously
    // before it resolves, but a selector captured in this callback's closure
    // would still reflect the pre-call render and lag one render behind.
    const handleSteerSession = (0,react.useCallback)(async (s, message)=>{
        const toastKey = "".concat(s.sessionId, ":steer");
        setSteeringSessionId(s.sessionId);
        try {
            const result = await updateSession(s.sessionId, {
                steerMessage: message
            });
            if (!result) {
                var _selectSessionsError;
                throw new Error((_selectSessionsError = (0,sessionsSlice/* selectSessionsError */.AY)(store_store/* store */.M_.getState())) !== null && _selectSessionsError !== void 0 ? _selectSessionsError : "Failed to steer session.");
            }
            showActionToast("Steering message sent.", "success", toastKey);
        } catch (err) {
            const msg = steerFailureMessage((0,connectError/* getErrorMessage */.u)(err, "Failed to steer session."));
            showActionToast(msg, "error", toastKey);
            throw err instanceof Error ? err : new Error(msg);
        } finally{
            setSteeringSessionId(null);
        }
    }, [
        updateSession,
        showActionToast
    ]);
    // Epic 3.4: fetch the current mode list once, for resolving each linked
    // session's frozen pipelineModeSnapshot to a name/drift state. Not
    // filtered by `enabled` — a since-disabled (but not deleted) mode is
    // still a "found" match, not "unrecognized".
    (0,react.useEffect)(()=>{
        let cancelled = false;
        listPipelineModes().then((modes)=>{
            if (!cancelled) setPipelineModes(modes);
        }).catch((e)=>{
            console.warn("[BacklogItemDetail] listPipelineModes failed", e);
        });
        return ()=>{
            cancelled = true;
        };
    }, [
        listPipelineModes
    ]);
    // Story 3.2.2: fetch Jules config once, for the "Dispatch to Jules" gate
    // below (ux.md §3.1's precedence order). Fetched here rather than reusing
    // JulesSettings.tsx's own load (that panel isn't mounted on this page) —
    // same raw createClient(SessionService, ...) pattern that file uses,
    // since GetJulesConfig has no wrapper on useBacklogService/useSessionService.
    // A fetch failure leaves julesConfig null, which resolveJulesDispatchGate
    // below treats the same as "feature off" (button hidden) rather than
    // surfacing a load error on an otherwise-unrelated page — deliberately
    // silent (no console.warn), unlike listPipelineModes below, since this
    // page's own test suites assert zero console.warn calls across a wide
    // range of unrelated scenarios (BacklogItemDetail.test.tsx's Collapsible
    // regression suite) and their `createClient` mocks predate this RPC.
    // Wrapped in try/catch (not just .catch()) because such a mock throws
    // synchronously when getJulesConfig doesn't exist on it, before a
    // `.then/.catch` chain would ever attach.
    (0,react.useEffect)(()=>{
        let cancelled = false;
        (async ()=>{
            try {
                const transport = (0,connect_transport/* createConnectTransport */.w)({
                    baseUrl: (0,config/* getApiBaseUrl */.h)(),
                    interceptors: [
                        (0,config/* createAuthInterceptor */._)()
                    ]
                });
                const client = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, transport);
                const resp = await client.getJulesConfig({});
                if (cancelled || !resp.config) return;
                setJulesConfig({
                    enabled: resp.config.enabled,
                    hasApiKey: resp.config.hasApiKey,
                    egressAcknowledgedRepos: resp.config.egressAcknowledgedRepos,
                    authReconnectRequired: resp.config.authReconnectRequired
                });
            } catch (e) {
            // Silent — see comment above.
            }
        })();
        return ()=>{
            cancelled = true;
        };
    }, []);
    // Track triage progress: increment elapsed time while triageStatus === "running"
    (0,react.useEffect)(()=>{
        if ((item === null || item === void 0 ? void 0 : item.triageStatus) !== "running") {
            return;
        }
        const interval = setInterval(()=>{
            setTriageElapsedSeconds((prev)=>prev + 1);
        }, 1000);
        return ()=>clearInterval(interval);
    }, [
        item === null || item === void 0 ? void 0 : item.triageStatus
    ]);
    // Reset triage timer when status changes away from triage
    (0,react.useEffect)(()=>{
        if ((item === null || item === void 0 ? void 0 : item.triageStatus) !== "running") {
            setTriageElapsedSeconds(0);
        }
    }, [
        item === null || item === void 0 ? void 0 : item.triageStatus
    ]);
    // retriggerTriageCore is shared by the standalone "Retry" button (InlineError,
    // shown for the triage-failed banner) and the "retry_triage" action dispatched
    // from ActionsSection's approve-plan-replacement affordance for a queued/ready
    // item that's gated on plan approval with no usable triage result (see
    // itemActions.ts — docs/tasks/backlog-feature-improvement.md's 2026-08-03
    // entry, item be676dab). TriggerTriage only ever accepts idea/ready, so a
    // queued item needs the same reset-to-idea step the manual "Return to Triage"
    // action performs first — mirrors AutoRespawnTriage's server-side handling of
    // the identical generalized case (server/services/backlog_service_triage.go).
    const retriggerTriageCore = (0,react.useCallback)(async (targetItemId, currentStatus)=>{
        if (currentStatus !== "idea" && currentStatus !== "ready") {
            await transitionStatus(targetItemId, "idea");
        }
        await triggerTriage(targetItemId);
    }, [
        transitionStatus,
        triggerTriage
    ]);
    const handleAction = (0,react.useCallback)(async (action)=>{
        if (!item) return;
        setActionLoading(action);
        const toastKey = "".concat(item.id, ":").concat(action);
        var _ACTION_SUCCESS_MESSAGES_action;
        let successMessage = (_ACTION_SUCCESS_MESSAGES_action = ACTION_SUCCESS_MESSAGES[action]) !== null && _ACTION_SUCCESS_MESSAGES_action !== void 0 ? _ACTION_SUCCESS_MESSAGES_action : "Done.";
        try {
            switch(action){
                case "mark_ready":
                    await transitionStatus(item.id, "ready");
                    break;
                case "trigger_triage":
                    await triggerTriage(item.id);
                    break;
                case "spawn_session":
                    {
                        const resp = await spawnSessionFromItem(item.id);
                        if (resp === null || resp === void 0 ? void 0 : resp.queued) successMessage = "At capacity — item queued, will start automatically.";
                        break;
                    }
                case "spawn_session_autonomous":
                    {
                        const resp = await spawnSessionFromItem(item.id, {
                            autonomous: true
                        });
                        if (resp === null || resp === void 0 ? void 0 : resp.queued) successMessage = "At capacity — item queued, will start automatically.";
                        break;
                    }
                case "restart_session":
                    await spawnSessionFromItem(item.id, {
                        force: true
                    });
                    break;
                case "approve_plan":
                    await approvePlan(item.id);
                    break;
                case "retry_triage":
                    await retriggerTriageCore(item.id, item.status);
                    break;
                case "mark_done":
                    await transitionStatus(item.id, "done");
                    break;
                case "override_done":
                    {
                        const reviewSession = item.linkedSessions.filter((s)=>s.role === "review").at(-1);
                        if (reviewSession) {
                            await overrideVerdict(reviewSession.entityId, "Manual override to done", "done");
                        } else {
                            if (mountedRef.current) setError("No review session found — cannot override verdict.");
                            return;
                        }
                        break;
                    }
                case "re_review":
                    await triggerReReview(item.id);
                    break;
                case "ship_pr":
                    await triggerShipPR(item.id);
                    break;
                case "manual_review":
                    setShowManualReview(true);
                    return;
                case "archive":
                    if (!confirm("Archive this item? It will be hidden from the default view. Its git worktree (if any) will be deleted from disk and cannot be recreated by unarchiving.")) return;
                    await archiveBacklogItem(item.id);
                    break;
                case "unarchive":
                    await unarchiveBacklogItem(item.id);
                    break;
                case "delete":
                    if (!confirm("Permanently delete this item and all its history? This cannot be undone.")) return;
                    await deleteBacklogItem(item.id);
                    onClose === null || onClose === void 0 ? void 0 : onClose();
                    return;
                case "reopen":
                    await transitionStatus(item.id, "review");
                    break;
                case "send_back_idea":
                    await transitionStatus(item.id, "idea");
                    break;
                default:
                    return;
            }
            showActionToast(successMessage, "success", toastKey);
            await load();
        } catch (e) {
            const msg = (0,connectError/* getErrorMessage */.u)(e, "Action failed.");
            if (mountedRef.current) setError(msg);
            showActionToast(msg, "error", toastKey);
        } finally{
            if (mountedRef.current) setActionLoading(null);
        }
    }, [
        item,
        transitionStatus,
        triggerTriage,
        retriggerTriageCore,
        spawnSessionFromItem,
        approvePlan,
        overrideVerdict,
        triggerReReview,
        triggerShipPR,
        archiveBacklogItem,
        unarchiveBacklogItem,
        deleteBacklogItem,
        onClose,
        load,
        showActionToast
    ]);
    // Extracted verbatim from the inline manual-review-submit onClick handler
    // (Story 3.1.3, Task 3.1.3b) so ActionsSection stays a props-down/
    // callbacks-up presentational component.
    const handleManualReviewSubmit = (0,react.useCallback)(async ()=>{
        if (!item) return;
        const toastKey = "".concat(item.id, ":manual_review_submit");
        setActionLoading("manual_review_submit");
        try {
            await submitManualReview(item.id, manualReviewOutcome, manualReviewSummary.trim());
            showActionToast("Review submitted.", "success", toastKey);
            setShowManualReview(false);
            setManualReviewSummary("");
            setManualReviewOutcome("PASS");
            await load();
        } catch (e) {
            const msg = (0,connectError/* getErrorMessage */.u)(e, "Submit failed.");
            if (mountedRef.current) setError(msg);
            showActionToast(msg, "error", toastKey);
        } finally{
            if (mountedRef.current) setActionLoading(null);
        }
    }, [
        item,
        manualReviewOutcome,
        manualReviewSummary,
        submitManualReview,
        showActionToast,
        load
    ]);
    // The backend writes skipPlanning/skipReviewGate/autoSpawnSession/autoCreatePR/
    // autoApprovePlan unconditionally on every UpdateBacklogItem call (they're plain
    // proto bools, not optional — no "unset" wire representation), so any partial
    // update that omits them silently resets them to false. Every partial
    // updateBacklogItem call below must spread these current values.
    const currentFlags = (0,react.useCallback)(()=>{
        var _item_skipPlanning, _item_skipReviewGate, _item_autoSpawnSession, _item_autoCreatePR, _item_autoApprovePlan;
        return {
            skipPlanning: (_item_skipPlanning = item === null || item === void 0 ? void 0 : item.skipPlanning) !== null && _item_skipPlanning !== void 0 ? _item_skipPlanning : false,
            skipReviewGate: (_item_skipReviewGate = item === null || item === void 0 ? void 0 : item.skipReviewGate) !== null && _item_skipReviewGate !== void 0 ? _item_skipReviewGate : false,
            autoSpawnSession: (_item_autoSpawnSession = item === null || item === void 0 ? void 0 : item.autoSpawnSession) !== null && _item_autoSpawnSession !== void 0 ? _item_autoSpawnSession : false,
            autoCreatePR: (_item_autoCreatePR = item === null || item === void 0 ? void 0 : item.autoCreatePR) !== null && _item_autoCreatePR !== void 0 ? _item_autoCreatePR : false,
            autoApprovePlan: (_item_autoApprovePlan = item === null || item === void 0 ? void 0 : item.autoApprovePlan) !== null && _item_autoApprovePlan !== void 0 ? _item_autoApprovePlan : false
        };
    }, [
        item
    ]);
    const handleSaveNotes = (0,react.useCallback)(async ()=>{
        if (!item) return;
        setActionLoading("save_notes");
        try {
            const updated = await updateBacklogItem(item.id, {
                ...currentFlags(),
                notes: notesValue
            });
            if (!mountedRef.current) return;
            if (updated) setItem(updated);
            setEditingNotes(false);
        } finally{
            if (mountedRef.current) setActionLoading(null);
        }
    }, [
        item,
        notesValue,
        updateBacklogItem,
        currentFlags
    ]);
    const handleUpdateItem = (0,react.useCallback)(async (data)=>{
        if (!item) return;
        const updated = await updateBacklogItem(item.id, data);
        if (updated) {
            setItem(updated);
            var _updated_notes;
            setNotesValue((_updated_notes = updated.notes) !== null && _updated_notes !== void 0 ? _updated_notes : "");
        }
        setEditMode(false);
    }, [
        item,
        updateBacklogItem
    ]);
    // Task 5.3.2c: the form's Save button submits through this wrapper instead
    // of calling handleUpdateItem directly. If a live update landed while
    // editing (bufferedItem is set), the save is intercepted — the real save
    // RPC is not called until the user explicitly picks "Save Anyway" or
    // "Reload" on the warn-before-overwrite banner below.
    const handleFormSubmit = (0,react.useCallback)(async (data)=>{
        if (bufferedItem) {
            setPendingSaveData(data);
            setSaveConfirmPending(true);
            return;
        }
        await handleUpdateItem(data);
    }, [
        bufferedItem,
        handleUpdateItem
    ]);
    /** "Save Anyway" on the warn-before-overwrite banner: proceeds with the original save, discarding the buffered server-side change. */ const handleSaveAnyway = (0,react.useCallback)(async ()=>{
        if (!pendingSaveData) return;
        const data = pendingSaveData;
        setPendingSaveData(null);
        setSaveConfirmPending(false);
        setBufferedItem(null);
        await handleUpdateItem(data);
    }, [
        pendingSaveData,
        handleUpdateItem
    ]);
    /** Plain "Reload" on the buffered-update banner (Task 5.3.2b) — applies the buffered update but stays in edit mode with the refreshed values. */ const handleReloadBuffered = (0,react.useCallback)(()=>{
        if (!bufferedItem) return;
        setItem(bufferedItem);
        var _bufferedItem_notes;
        setNotesValue((_bufferedItem_notes = bufferedItem.notes) !== null && _bufferedItem_notes !== void 0 ? _bufferedItem_notes : "");
        setBufferedItem(null);
    }, [
        bufferedItem
    ]);
    /** "Reload" on the warn-before-overwrite banner (Task 5.3.2c) — discards the in-progress edit, applies the buffered update, and returns to view mode without saving. */ const handleConfirmReload = (0,react.useCallback)(()=>{
        if (bufferedItem) {
            setItem(bufferedItem);
            var _bufferedItem_notes;
            setNotesValue((_bufferedItem_notes = bufferedItem.notes) !== null && _bufferedItem_notes !== void 0 ? _bufferedItem_notes : "");
        }
        setBufferedItem(null);
        setPendingSaveData(null);
        setSaveConfirmPending(false);
        setEditMode(false);
    }, [
        bufferedItem
    ]);
    const handleCancelTriage = (0,react.useCallback)(async ()=>{
        if (!item) return;
        const toastKey = "".concat(item.id, ":cancel_triage");
        setActionLoading("cancel_triage");
        try {
            await cancelTriage(item.id);
            showActionToast("Triage cancelled.", "success", toastKey);
            await load();
        } catch (e) {
            const msg = (0,connectError/* getErrorMessage */.u)(e, "Cancel failed.");
            if (mountedRef.current) setError(msg);
            showActionToast(msg, "error", toastKey);
        } finally{
            if (mountedRef.current) setActionLoading(null);
        }
    }, [
        item,
        cancelTriage,
        load,
        showActionToast
    ]);
    const handleRetriggerTriage = (0,react.useCallback)(async ()=>{
        if (!item) return;
        const toastKey = "".concat(item.id, ":retrigger_triage");
        setActionLoading("retrigger_triage");
        try {
            await retriggerTriageCore(item.id, item.status);
            showActionToast("Triage re-triggered.", "success", toastKey);
            await load();
        } catch (e) {
            console.error("[BacklogItemDetail] retrigger triage failed", e);
            const msg = (0,connectError/* getErrorMessage */.u)(e, "Triage re-trigger failed.");
            if (mountedRef.current) setError(msg);
            showActionToast(msg, "error", toastKey);
        } finally{
            if (mountedRef.current) setActionLoading(null);
        }
    }, [
        item,
        retriggerTriageCore,
        load,
        showActionToast
    ]);
    const handleRefineTriage = (0,react.useCallback)(async (feedback)=>{
        if (!item) return;
        await triggerTriage(item.id, feedback);
        await load();
    }, [
        item,
        triggerTriage,
        load
    ]);
    // Chat-based refinement (additive to handleRefineTriage's structured-form
    // path): delegates to CreateBacklogItemFromChat, which internally calls the
    // same TriggerTriage/feedback path — same data shape, different front door.
    const handleChatRefine = (0,react.useCallback)(async (message)=>{
        if (!item) return;
        const result = await createBacklogItemFromChat(message, item.id);
        if (!result) {
            var _lastError_message;
            throw new Error((_lastError_message = lastError === null || lastError === void 0 ? void 0 : lastError.message) !== null && _lastError_message !== void 0 ? _lastError_message : "Failed to send chat message");
        }
        await load();
    }, [
        item,
        createBacklogItemFromChat,
        load,
        lastError
    ]);
    // Story 4.3.1: RejectPlan only persists state (ADR-002) — it does not
    // itself trigger regeneration. handleRegeneratePlanWithFeedback is the
    // separate, explicit follow-up action PlanVerdictBox renders once an item
    // is in the changes_requested state.
    const handleRejectPlan = (0,react.useCallback)(async (reason)=>{
        if (!item) return;
        const toastKey = "".concat(item.id, ":reject_plan");
        setActionLoading("reject_plan");
        try {
            await rejectPlan(item.id, reason);
            showActionToast("Revisions requested.", "success", toastKey);
            await load();
        } catch (e) {
            showActionToast((0,connectError/* getErrorMessage */.u)(e, "Reject failed."), "error", toastKey);
            throw e;
        } finally{
            if (mountedRef.current) setActionLoading(null);
        }
    }, [
        item,
        rejectPlan,
        load,
        showActionToast
    ]);
    // Story 2.2.1: chains transitionStatus -> rejectPlan -> triggerTriage as
    // one operator action. Each call is wrapped in its own try/catch (not one
    // shared try) so the outer catch knows exactly which of the three failed
    // (resolves BLOCKER 5.2) — SendBackFeedbackBox renders distinct recovery
    // copy per failedAt. See ux.md Surface 7 and pre-mortem P1 #1.
    const handleSendBackWithFeedback = (0,react.useCallback)(async (feedback)=>{
        if (!item) return;
        const toastKey = "".concat(item.id, ":send_back_ready");
        setActionLoading("send_back_ready");
        try {
            try {
                await transitionStatus(item.id, "ready", {
                    expectedStatus: item.status,
                    expectedUpdatedAt: item.updatedAtRaw,
                    overrideReason: feedback
                });
            } catch (e) {
                // Nothing has changed server-side — item.status is still accurate.
                throw new SendBackError("transition", item.status, e);
            }
            try {
                await rejectPlan(item.id, feedback);
            } catch (e) {
                // transitionStatus committed; the item is "ready" even though this
                // call failed (rejectPlan doesn't change status).
                throw new SendBackError("reject", "ready", e);
            }
            try {
                await triggerTriage(item.id, feedback);
            } catch (e) {
                // Can't assume "ready" here (pre-mortem P1 #1): triggerTriage's own
                // internal CAS may have already moved the item to "idea" before
                // this failure. Re-fetch directly (not via load(), whose result
                // isn't returned to this scope) to learn the real status.
                const fresh = await getBacklogItem(item.id);
                var _fresh_status;
                throw new SendBackError("triage", (_fresh_status = fresh === null || fresh === void 0 ? void 0 : fresh.status) !== null && _fresh_status !== void 0 ? _fresh_status : "ready", e);
            }
            showActionToast("Feedback sent — retriage started.", "success", toastKey);
            await load();
        } catch (e) {
            // Re-fetch unconditionally: once transitionStatus (call 1) commits, a
            // stale local `item` would replay it with an outdated CAS precondition
            // on retry, failing again with a confusing ErrPreconditionFailed.
            await load();
            // Partial failure (call 1 committed, call 2/3 failed) gets a neutral
            // toast so it doesn't contradict SendBackFeedbackBox's more specific
            // in-form message (ux.md Surface 7); only a true call-1 failure keeps
            // the "Failed to send back." framing.
            const toastMessage = e instanceof SendBackError && e.failedAt !== "transition" ? "Send-back needs attention — see details below." : (0,connectError/* getErrorMessage */.u)(e, "Failed to send back.");
            showActionToast(toastMessage, "error", toastKey);
            throw e; // still a SendBackError (or the original error) — SendBackFeedbackBox's catch reads it
        } finally{
            if (mountedRef.current) setActionLoading(null);
        }
    }, [
        item,
        transitionStatus,
        rejectPlan,
        triggerTriage,
        getBacklogItem,
        load,
        showActionToast
    ]);
    const handleRegeneratePlanWithFeedback = (0,react.useCallback)(async ()=>{
        if (!(item === null || item === void 0 ? void 0 : item.planRejectionReason)) return;
        await triggerTriage(item.id, item.planRejectionReason);
        await load();
    }, [
        item,
        triggerTriage,
        load
    ]);
    const handleApplyTriageSuggestions = (0,react.useCallback)(async (preApplyCriteria)=>{
        var _item_triageResult;
        if (!item) return;
        var _item_triageResult_suggestions_filter;
        // Step 1: Update AC with suggestions
        const acSuggestions = (_item_triageResult_suggestions_filter = (_item_triageResult = item.triageResult) === null || _item_triageResult === void 0 ? void 0 : _item_triageResult.suggestions.filter((s)=>s.rationale !== "question")) !== null && _item_triageResult_suggestions_filter !== void 0 ? _item_triageResult_suggestions_filter : [];
        const newAcCriteria = acSuggestions.map((s, i)=>({
                index: i,
                text: s.text,
                status: "pending"
            }));
        const updated = await updateBacklogItem(item.id, {
            ...currentFlags(),
            acCriteria: newAcCriteria
        });
        if (!updated) {
            throw new Error("Failed to apply suggestions — item may have been modified by another process. Reload and try again.");
        }
        // Step 2: Transition to ready
        const transitioned = await transitionStatus(item.id, "ready", {
            expectedStatus: "idea"
        });
        if (!transitioned) {
            throw new Error("Failed to mark item ready — please try again.");
        }
        await load();
        // Store pre-apply criteria for undo (returned via throw on error, used by panel)
        void preApplyCriteria; // captured in panel for undo
    }, [
        item,
        updateBacklogItem,
        transitionStatus,
        load,
        currentFlags
    ]);
    const handleUndoTriageSuggestions = (0,react.useCallback)(async (preApplyCriteria)=>{
        if (!item) return;
        // Called via `void onUndoApply(...)` from TriageReviewPanel — nothing upstream
        // awaits this, so failures must be caught and surfaced here, not thrown.
        try {
            // Revert AC to the pre-apply snapshot
            const updated = await updateBacklogItem(item.id, {
                ...currentFlags(),
                acCriteria: preApplyCriteria
            });
            if (!updated) {
                throw new Error("Failed to undo — item may have been modified. Reload and try again.");
            }
            // Revert status back to idea
            await transitionStatus(item.id, "idea");
            showActionToast("Undo applied.", "success", "".concat(item.id, ":undo_triage_apply"));
            await load();
        } catch (e) {
            const msg = (0,connectError/* getErrorMessage */.u)(e, "Undo failed.");
            if (mountedRef.current) setError(msg);
            showActionToast(msg, "error", "".concat(item.id, ":undo_triage_apply"));
        }
    }, [
        item,
        updateBacklogItem,
        transitionStatus,
        load,
        currentFlags,
        showActionToast
    ]);
    // The four handlers below are called from GateVerdictBox, which already wraps each
    // call in its own try/catch and shows a local InlineError on failure. We add a toast
    // here too (rethrowing so GateVerdictBox's own handling still runs) since the inline
    // banner there is easy to miss.
    const handleGateApprove = (0,react.useCallback)(async ()=>{
        if (!item) return;
        const toastKey = "".concat(item.id, ":gate_approve");
        setActionLoading("gate_approve");
        try {
            const ok = await transitionStatus(item.id, "done");
            if (!ok) {
                var _lastError_message;
                const msg = (_lastError_message = lastError === null || lastError === void 0 ? void 0 : lastError.message) !== null && _lastError_message !== void 0 ? _lastError_message : "Failed to approve — please try again.";
                if (mountedRef.current) setError(msg);
                showActionToast(msg, "error", toastKey);
                return;
            }
            showActionToast("Approved.", "success", toastKey);
            await load();
        } catch (e) {
            showActionToast((0,connectError/* getErrorMessage */.u)(e, "Approve failed."), "error", toastKey);
            throw e;
        } finally{
            if (mountedRef.current) setActionLoading(null);
        }
    }, [
        item,
        transitionStatus,
        lastError,
        load,
        showActionToast
    ]);
    const handleGateReopen = (0,react.useCallback)(async (feedback)=>{
        if (!item) return;
        const toastKey = "".concat(item.id, ":gate_reopen");
        setActionLoading("gate_reopen");
        try {
            // Append feedback to notes so the next work session sees it in its prompt.
            if (feedback.trim()) {
                const timestamp = new Date().toISOString().slice(0, 16).replace("T", " ");
                const note = "\n\n[Revision feedback ".concat(timestamp, "]\n").concat(feedback.trim());
                var _item_notes;
                await updateBacklogItem(item.id, {
                    ...currentFlags(),
                    notes: ((_item_notes = item.notes) !== null && _item_notes !== void 0 ? _item_notes : "") + note
                });
            }
            await transitionStatus(item.id, "in_progress");
            // Spawn a new work session immediately — the backend now accepts in_progress.
            await spawnSessionFromItem(item.id);
            showActionToast("Reopened — new session started.", "success", toastKey);
            await load();
        } catch (e) {
            showActionToast((0,connectError/* getErrorMessage */.u)(e, "Reopen failed."), "error", toastKey);
            throw e;
        } finally{
            if (mountedRef.current) setActionLoading(null);
        }
    }, [
        item,
        transitionStatus,
        spawnSessionFromItem,
        updateBacklogItem,
        load,
        currentFlags,
        showActionToast
    ]);
    const handleGateOverride = (0,react.useCallback)(async (reason)=>{
        if (!item) return;
        const toastKey = "".concat(item.id, ":gate_override");
        setActionLoading("gate_override");
        try {
            const reviewSession = item.linkedSessions.filter((s)=>s.role === "review").at(-1);
            if (!reviewSession) {
                const msg = "No review session found — cannot override verdict.";
                if (mountedRef.current) setError(msg);
                showActionToast(msg, "error", toastKey);
                return;
            }
            await overrideVerdict(reviewSession.entityId, reason, "done");
            showActionToast("Overridden to done.", "success", toastKey);
            await load();
        } catch (e) {
            const msg = (0,connectError/* getErrorMessage */.u)(e, "Override failed.");
            if (mountedRef.current) setError(msg);
            showActionToast(msg, "error", toastKey);
            throw e;
        } finally{
            if (mountedRef.current) setActionLoading(null);
        }
    }, [
        item,
        overrideVerdict,
        load,
        showActionToast
    ]);
    // Manual overrides (ManualOverrideSection) — the operator escape hatch for
    // an item whose automation has gotten wedged. Unlike the four handlers
    // above (called from GateVerdictBox, status-conditional on being in
    // review), these are always reachable regardless of status.
    const handleManualStatusOverride = (0,react.useCallback)(async (toStatus, reason)=>{
        if (!item) return;
        const toastKey = "".concat(item.id, ":manual_override_status");
        setActionLoading("manual_override_status");
        try {
            // expectedUpdatedAt uses item.updatedAtRaw (the undecoded protobuf
            // Timestamp), not item.updatedAt (a display-oriented ISO string) —
            // an earlier version of this code built the precondition from the
            // ISO string via `new Date(...)` + timestampFromDate, which is only
            // millisecond-precision and can never exactly match the server's
            // nanosecond-precision updated_at column (ent's CAS check is exact
            // equality), so every write spuriously failed. Passing the raw
            // Timestamp through verbatim avoids the round-trip entirely.
            const ok = await transitionStatus(item.id, toStatus, {
                expectedStatus: item.status,
                expectedUpdatedAt: item.updatedAtRaw,
                overrideReason: reason
            });
            if (!ok) {
                var _lastError_message;
                const msg = (_lastError_message = lastError === null || lastError === void 0 ? void 0 : lastError.message) !== null && _lastError_message !== void 0 ? _lastError_message : "Someone else changed this item — reload and try again.";
                showActionToast(msg, "error", toastKey);
                throw new Error(msg);
            }
            showActionToast("Status manually overridden to ".concat(toStatus, "."), "success", toastKey);
            await load();
        } catch (e) {
            showActionToast((0,connectError/* getErrorMessage */.u)(e, "Status override failed."), "error", toastKey);
            throw e;
        } finally{
            if (mountedRef.current) setActionLoading(null);
        }
    }, [
        item,
        transitionStatus,
        lastError,
        load,
        showActionToast
    ]);
    const handleManualAssociatePR = (0,react.useCallback)(async (prUrl, prNumber)=>{
        if (!item) return;
        const toastKey = "".concat(item.id, ":manual_associate_pr");
        setActionLoading("manual_associate_pr");
        try {
            const updated = await updateBacklogItem(item.id, {
                ...currentFlags(),
                prUrl,
                prNumber
            });
            if (!updated) {
                var _lastError_message;
                const msg = (_lastError_message = lastError === null || lastError === void 0 ? void 0 : lastError.message) !== null && _lastError_message !== void 0 ? _lastError_message : "Failed to link PR — please try again.";
                showActionToast(msg, "error", toastKey);
                throw new Error(msg);
            }
            showActionToast("PR #".concat(prNumber, " linked — item moved to pr_pending."), "success", toastKey);
            await load();
        } catch (e) {
            showActionToast((0,connectError/* getErrorMessage */.u)(e, "PR association failed."), "error", toastKey);
            throw e;
        } finally{
            if (mountedRef.current) setActionLoading(null);
        }
    }, [
        item,
        updateBacklogItem,
        lastError,
        load,
        currentFlags,
        showActionToast
    ]);
    const handleGateSkip = (0,react.useCallback)(async ()=>{
        if (!item) return;
        const toastKey = "".concat(item.id, ":gate_skip");
        setActionLoading("gate_skip");
        try {
            const reviewSession = item.linkedSessions.filter((s)=>s.role === "review").at(-1);
            if (reviewSession) {
                await overrideVerdict(reviewSession.entityId, "Gate skipped by user", "done");
            } else {
                // No review session yet — direct transition (item.skipReviewGate path)
                const ok = await transitionStatus(item.id, "done");
                if (!ok) {
                    var _lastError_message;
                    const msg = (_lastError_message = lastError === null || lastError === void 0 ? void 0 : lastError.message) !== null && _lastError_message !== void 0 ? _lastError_message : "Failed to skip gate — please try again.";
                    if (mountedRef.current) setError(msg);
                    showActionToast(msg, "error", toastKey);
                    return;
                }
            }
            showActionToast("Gate skipped — marked done.", "success", toastKey);
            await load();
        } catch (e) {
            const msg = (0,connectError/* getErrorMessage */.u)(e, "Skip gate failed.");
            if (mountedRef.current) setError(msg);
            showActionToast(msg, "error", toastKey);
            throw e;
        } finally{
            if (mountedRef.current) setActionLoading(null);
        }
    }, [
        item,
        overrideVerdict,
        transitionStatus,
        lastError,
        load,
        showActionToast
    ]);
    // Only show the full-screen loader on the INITIAL load (no item yet). Background
    // refreshes must NOT unmount the detail view / edit form, or in-progress edits
    // like unsaved acceptance criteria get discarded when the form remounts.
    // See stapler-squad#146.
    if (loading && !item) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("article", {
            className: BacklogItemDetail_css_container,
            "data-testid": "backlog-item-detail",
            children: [
                onClose && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: header,
                    children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: headerRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {}),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: headerActions,
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: closeButton,
                                    onClick: onClose,
                                    "aria-label": "Close",
                                    children: "\xd7"
                                })
                            })
                        ]
                    })
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: loadingState,
                    role: "status",
                    "aria-label": "Loading backlog item",
                    children: "Loading…"
                })
            ]
        });
    }
    if (!item) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("article", {
            className: BacklogItemDetail_css_container,
            "data-testid": "backlog-item-detail",
            children: [
                onClose && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: header,
                    children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: headerRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {}),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: headerActions,
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: closeButton,
                                    onClick: onClose,
                                    "aria-label": "Close",
                                    children: "\xd7"
                                })
                            })
                        ]
                    })
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: errorState,
                    role: "alert",
                    children: error !== null && error !== void 0 ? error : "Item not found."
                })
            ]
        });
    }
    if (editMode) {
        var _item_updatedAt;
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("article", {
            className: BacklogItemDetail_css_container,
            "data-testid": "backlog-item-detail",
            "aria-label": "Edit backlog item: ".concat(item.title),
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: header,
                    children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: headerRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: titleGroup,
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                                    className: itemTitle,
                                    children: item.title
                                })
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: headerActions,
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: closeButton,
                                    onClick: ()=>setEditMode(false),
                                    "aria-label": "Cancel editing",
                                    "data-testid": "backlog-detail-cancel-edit",
                                    children: "\xd7"
                                })
                            })
                        ]
                    })
                }),
                saveConfirmPending ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: bannerBar,
                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(InlineNotice/* InlineNotice */.w, {
                        message: "Saving will overwrite a change made elsewhere — Reload first?",
                        actions: [
                            {
                                label: "Save Anyway",
                                onClick: ()=>void handleSaveAnyway(),
                                variant: "primary"
                            },
                            {
                                label: "Reload",
                                onClick: handleConfirmReload
                            }
                        ],
                        "data-testid": "backlog-detail-save-conflict-notice"
                    })
                }) : bufferedItem ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: bannerBar,
                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(InlineNotice/* InlineNotice */.w, {
                        message: "This item changed elsewhere.",
                        actions: [
                            {
                                label: "Reload",
                                onClick: handleReloadBuffered
                            }
                        ],
                        onDismiss: ()=>setBufferedItem(null),
                        "data-testid": "backlog-detail-buffered-update-notice"
                    })
                }) : null,
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: scrollArea,
                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(BacklogItemForm/* BacklogItemForm */.T, {
                        initialValues: item,
                        onSubmit: handleFormSubmit,
                        onCancel: ()=>setEditMode(false)
                    }, "".concat(item.id, ":").concat((_item_updatedAt = item.updatedAt) !== null && _item_updatedAt !== void 0 ? _item_updatedAt : ""))
                })
            ]
        });
    }
    var _PRIORITY_LABELS_item_priority, _PRIORITY_LABELS_item_priority1, _item_repoPath, _julesDispatchGate_branch, _item_repoPath1, _item_labels, _julesConfig_authReconnectRequired;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("article", {
        className: BacklogItemDetail_css_container,
        "data-testid": "backlog-item-detail",
        "aria-label": "Backlog item: ".concat(item.title),
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: header,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: headerRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: titleGroup,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                                        className: itemTitle,
                                        children: item.title
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: metaRow,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: priorityBadge,
                                                "aria-label": "Priority: ".concat((_PRIORITY_LABELS_item_priority = PRIORITY_LABELS[item.priority]) !== null && _PRIORITY_LABELS_item_priority !== void 0 ? _PRIORITY_LABELS_item_priority : "Unknown"),
                                                children: (_PRIORITY_LABELS_item_priority1 = PRIORITY_LABELS[item.priority]) !== null && _PRIORITY_LABELS_item_priority1 !== void 0 ? _PRIORITY_LABELS_item_priority1 : "P?"
                                            }),
                                            item.createdAt && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                className: dateMeta,
                                                children: [
                                                    "Created ",
                                                    formatDate(item.createdAt)
                                                ]
                                            }),
                                            item.updatedAt && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                className: dateMeta,
                                                children: [
                                                    "\xb7 Updated ",
                                                    formatDate(item.updatedAt)
                                                ]
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: idRow,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: idText,
                                                "data-testid": "backlog-item-id",
                                                children: item.publicId || item.id
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                type: "button",
                                                className: copyButton,
                                                onClick: ()=>handleCopy("id", item.publicId || item.id),
                                                "aria-label": copiedField === "id" ? "Copied item ID to clipboard" : "Copy item ID",
                                                "data-testid": "copy-item-id-button",
                                                children: copiedField === "id" ? "✓ Copied" : "Copy ID"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                type: "button",
                                                className: copyButton,
                                                onClick: ()=>handleCopy("link", "ssq://".concat(window.location.host, "/backlog/v1/").concat(item.publicId || item.id)),
                                                "aria-label": copiedField === "link" ? "Copied link to clipboard" : "Copy shareable link",
                                                "data-testid": "copy-item-link-button",
                                                children: copiedField === "link" ? "✓ Copied" : "Copy Link"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                "aria-live": "polite",
                                                role: "status",
                                                "data-testid": "copy-status-announcement",
                                                style: {
                                                    position: "absolute",
                                                    width: "1px",
                                                    height: "1px",
                                                    overflow: "hidden",
                                                    clip: "rect(0 0 0 0)",
                                                    whiteSpace: "nowrap"
                                                },
                                                children: [
                                                    copiedField === "id" && "Item ID copied to clipboard",
                                                    copiedField === "link" && "Link copied to clipboard"
                                                ]
                                            })
                                        ]
                                    }),
                                    item.externalUrl && /*#__PURE__*/ (0,jsx_runtime.jsx)(ItemClaimBanner, {
                                        externalUrl: item.externalUrl
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: headerActions,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(ConnectionIndicator/* ConnectionIndicator */._, {
                                        connectionState: connectionState
                                    }),
                                    !terminalState && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        className: editButton,
                                        onClick: ()=>setEditMode(true),
                                        disabled: !itemConfirmed,
                                        title: itemConfirmed ? undefined : "Confirming latest data before editing…",
                                        "aria-label": "Edit item",
                                        "data-testid": "backlog-detail-edit",
                                        children: "Edit"
                                    }),
                                    onClose && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        className: closeButton,
                                        onClick: onClose,
                                        "aria-label": "Close item detail",
                                        "data-testid": "backlog-detail-close",
                                        children: "\xd7"
                                    })
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(LifecycleSummary, {
                        item: item,
                        pipelineDisplay: pipelineDisplay,
                        configuredPipelineModeName: configuredPipelineModeName,
                        stuckItem: stuckItem,
                        otherStuckReasons: stuckSummary === null || stuckSummary === void 0 ? void 0 : stuckSummary.otherReasons,
                        onTriggerRemediationNow: triggerRemediationNow,
                        onOverrideClaimBlock: overrideClaimBlock
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(ItemBudgetWarning, {
                        thresholdUsd: item.costBudgetThresholdUsd,
                        totalCostUsd: item.totalEstimatedCostUsd
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: scrollArea,
                children: [
                    (item.status === "ready" || item.status === "queued" || (0,planReviewStatus/* derivePlanReviewStatus */.V)(item) !== "no_plan") && /*#__PURE__*/ (0,jsx_runtime.jsx)(PlanVerdictBox, {
                        status: (0,planReviewStatus/* derivePlanReviewStatus */.V)(item),
                        rejectionReason: item.planRejectionReason,
                        readOnly: terminalState !== null,
                        actionPending: actionLoading === "reject_plan",
                        onReject: handleRejectPlan,
                        onRegenerateWithFeedback: handleRegeneratePlanWithFeedback
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(ActionsSection, {
                        item: item,
                        actionLoading: actionLoading,
                        latestWorkSession: latestWorkSession,
                        showManualReview: showManualReview,
                        manualReviewOutcome: manualReviewOutcome,
                        manualReviewSummary: manualReviewSummary,
                        onAction: handleAction,
                        onManualReviewOutcomeChange: setManualReviewOutcome,
                        onManualReviewSummaryChange: setManualReviewSummary,
                        onManualReviewSubmit: ()=>{
                            void handleManualReviewSubmit();
                        },
                        onManualReviewCancel: ()=>{
                            setShowManualReview(false);
                            setManualReviewSummary("");
                        },
                        terminalState: terminalState,
                        julesDispatchGate: julesDispatchGate,
                        onDispatchToJulesClick: (event)=>{
                            julesDispatchTriggerRef.current = event.currentTarget;
                            setShowJulesDispatch(true);
                        },
                        activeWorkSessionCount: activeWorkSessionCount,
                        onSendBackWithFeedback: handleSendBackWithFeedback
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(GateBlockingSection, {
                        item: item
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(DependenciesSection, {
                        item: item
                    }),
                    !(item.triageStatus === "completed" && item.status === "idea" && item.triageResult) && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: section,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(GuidanceRequestPanel/* GuidanceRequestPanel */.w, {
                            scope: "backlog-item",
                            scopeKey: item.id
                        })
                    }),
                    error && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: errorBanner,
                        role: "alert",
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                children: error
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: errorBannerDismiss,
                                onClick: ()=>setError(null),
                                "aria-label": "Dismiss error",
                                children: "\xd7"
                            })
                        ]
                    }),
                    (item.status === "idea" || item.status === "ready") && item.triageStatus === "running" && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: section,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(TriageLoadingIndicator/* TriageLoadingIndicator */.i, {
                            elapsedSeconds: triageElapsedSeconds,
                            context: "item",
                            onCancel: actionLoading !== null ? ()=>{} : handleCancelTriage,
                            compact: false
                        })
                    }),
                    item.triageStatus === "completed" && item.status === "idea" && item.triageResult && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: section,
                        "aria-live": "polite",
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(TriageReviewPanel, {
                            item: item,
                            triageResult: item.triageResult,
                            onApply: handleApplyTriageSuggestions,
                            onUndoApply: handleUndoTriageSuggestions,
                            // Dismissing the panel is a purely client-local state change
                            // (localStorage + local React state, no server mutation) —
                            // nothing to refresh (pitfall #3). A load() here was a
                            // needless, unguarded race source that could stomp
                            // more-current live-store state with a redundant read.
                            onSkip: ()=>{},
                            onRefine: handleRefineTriage,
                            onAnswerQuestion: handleRefineTriage
                        })
                    }),
                    item.triageStatus === "completed" && item.status === "idea" && item.triageResult && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: section,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ChatRefinementPanel, {
                            clarifyingQuestions: item.triageResult.clarifyingQuestions,
                            onSend: handleChatRefine
                        })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(PlanningSection, {
                        item: item
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(BacklogResultPanels, {
                        item: item
                    }),
                    item.triageStatus === "failed" && !item.skipPlanning && !item.planApproved && (item.status === "idea" || item.status === "queued") && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: section,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(InlineError, {
                            type: "permanent",
                            customMessage: item.status === "queued" ? "This item's most recent triage session ended without producing a usable plan. Retry triage to generate one." : undefined,
                            // InlineError itself has no disabled/loading state (unmodified,
                            // pre-existing component) — guard the same way
                            // TriageLoadingIndicator's onCancel does above, swapping in a
                            // no-op while a retry is already in flight. Without this, a
                            // double-click fires two concurrent retriggerTriageCore calls,
                            // each doing transitionStatus(id,"idea") + triggerTriage(id) —
                            // worse for a queued item (two competing status resets) than the
                            // pre-existing idea-only exposure of this same button.
                            onRetry: actionLoading !== null ? ()=>{} : handleRetriggerTriage
                        })
                    }),
                    showChangesModal && /*#__PURE__*/ (0,jsx_runtime.jsx)(ReviewChangesModal, {
                        itemId: item.id,
                        sessionId: latestWorkSession === null || latestWorkSession === void 0 ? void 0 : latestWorkSession.sessionId,
                        sessionTitle: item.title,
                        onClose: ()=>setShowChangesModal(false),
                        triggerRef: changesModalTriggerRef
                    }),
                    showFileBrowser && latestWorkSession && /*#__PURE__*/ (0,jsx_runtime.jsx)(BacklogFileBrowserModal, {
                        sessionId: latestWorkSession.sessionId,
                        sessionTitle: item.title,
                        onClose: ()=>setShowFileBrowser(false),
                        triggerRef: fileBrowserTriggerRef
                    }),
                    showJulesDispatch && julesConfig && /*#__PURE__*/ (0,jsx_runtime.jsx)(JulesDispatchDialog, {
                        itemId: item.id,
                        itemTitle: item.title,
                        acCriteria: item.acCriteria,
                        repoPath: (_item_repoPath = item.repoPath) !== null && _item_repoPath !== void 0 ? _item_repoPath : "",
                        initialBranch: (_julesDispatchGate_branch = julesDispatchGate === null || julesDispatchGate === void 0 ? void 0 : julesDispatchGate.branch) !== null && _julesDispatchGate_branch !== void 0 ? _julesDispatchGate_branch : "",
                        egressAcknowledged: julesConfig.egressAcknowledgedRepos.includes((_item_repoPath1 = item.repoPath) !== null && _item_repoPath1 !== void 0 ? _item_repoPath1 : ""),
                        onClose: ()=>setShowJulesDispatch(false),
                        triggerRef: julesDispatchTriggerRef
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)(Collapsible/* CollapsibleGroup */.I, {
                        value: openSectionKeys,
                        onValueChange: handleGroupValueChange,
                        children: [
                            (item.status === "review" || item.status === "pr_pending" || !!item.gateVerdict) && /*#__PURE__*/ (0,jsx_runtime.jsx)(GroupLabel, {
                                children: "Decision"
                            }),
                            item.status === "review" && /*#__PURE__*/ (0,jsx_runtime.jsx)(ReviewingSection, {
                                item: item,
                                workSession: latestWorkSession,
                                actionLoading: actionLoading,
                                defaultExpanded: reviewingExpanded,
                                onViewChanges: (event)=>{
                                    changesModalTriggerRef.current = event.currentTarget;
                                    setShowChangesModal(true);
                                },
                                onGateApprove: handleGateApprove,
                                onGateReopen: handleGateReopen,
                                onGateOverride: handleGateOverride,
                                onGateSkip: handleGateSkip,
                                onReReview: ()=>triggerReReview(item.id).then(()=>load()),
                                readOnly: terminalState !== null
                            }),
                            item.status !== "review" && item.gateVerdict && /*#__PURE__*/ (0,jsx_runtime.jsx)(LastReviewResultSection, {
                                item: item,
                                defaultExpanded: lastReviewResultExpanded
                            }),
                            item.status === "pr_pending" && /*#__PURE__*/ (0,jsx_runtime.jsx)(PullRequestSection, {
                                item: item,
                                actionLoading: actionLoading,
                                onMarkDone: ()=>handleAction("mark_done"),
                                readOnly: terminalState !== null,
                                stuckReasons: stuckItemGroup.map((i)=>i.reason)
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(GroupLabel, {
                                children: "Reference"
                            }),
                            item.externalUrl && /*#__PURE__*/ (0,jsx_runtime.jsx)(SourceSection, {
                                externalUrl: item.externalUrl,
                                externalId: item.externalId,
                                pluginId: item.pluginId,
                                labels: (_item_labels = item.labels) !== null && _item_labels !== void 0 ? _item_labels : [],
                                defaultExpanded: sourceExpanded
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(DescriptionSection, {
                                item: item,
                                defaultExpanded: descriptionExpanded
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(AcCriteriaSection, {
                                item: item,
                                defaultExpanded: acCriteriaExpanded
                            }),
                            item.planArtifactsPath && /*#__PURE__*/ (0,jsx_runtime.jsx)(PlanArtifactsSection, {
                                item: item,
                                defaultExpanded: planArtifactsExpanded
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(GroupLabel, {
                                children: "Monitoring"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(VersionControlSection, {
                                item: item,
                                widgetData: vcsWidgetData,
                                activeSessionCount: activeWorkSessionCount,
                                worktreePath: latestWorkSession === null || latestWorkSession === void 0 ? void 0 : latestWorkSession.worktreePath,
                                defaultExpanded: versionControlExpanded,
                                onViewDiff: (event)=>{
                                    changesModalTriggerRef.current = event.currentTarget;
                                    setShowChangesModal(true);
                                },
                                onBrowseFiles: (event)=>{
                                    fileBrowserTriggerRef.current = event.currentTarget;
                                    setShowFileBrowser(true);
                                }
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(AutonomousHealthStrip, {
                                item: item
                            }),
                            insightsLoading && !insightsSummary ? /*#__PURE__*/ (0,jsx_runtime.jsx)(Skeleton/* Skeleton */.E, {
                                variant: "rectangular",
                                width: "100%",
                                height: 96
                            }) : insightsError ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: ItemStageCostTable_css/* errorState */.IU,
                                "data-testid": "item-stage-cost-error",
                                children: "Couldn't load cost data."
                            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)(ItemStageCostTable, {
                                rows: itemStageCostRows
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(GroupLabel, {
                                children: "Audit Trail"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionsSection, {
                                item: item,
                                pipelineModes: pipelineModes,
                                latestWorkSession: latestWorkSession,
                                deletingSessionId: deletingSessionId,
                                defaultExpanded: sessionsExpanded,
                                onDeleteSession: handleDeleteSession,
                                onSteerSession: handleSteerSession,
                                steeringSessionId: steeringSessionId,
                                authReconnectRequired: (_julesConfig_authReconnectRequired = julesConfig === null || julesConfig === void 0 ? void 0 : julesConfig.authReconnectRequired) !== null && _julesConfig_authReconnectRequired !== void 0 ? _julesConfig_authReconnectRequired : false
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(WorkflowHistorySection, {
                                item: item,
                                defaultExpanded: workflowExpanded
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(ProgressHistorySection, {
                                item: item,
                                defaultExpanded: progressHistoryExpanded
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(ActivityLogSection, {
                                item: item,
                                defaultExpanded: activityLogExpanded
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(GroupLabel, {
                                children: "Admin"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(NotesSection, {
                                item: item,
                                actionLoading: actionLoading,
                                editingNotes: editingNotes,
                                notesValue: notesValue,
                                defaultExpanded: notesExpanded,
                                onNotesValueChange: setNotesValue,
                                onStartEditing: ()=>setEditingNotes(true),
                                onSave: handleSaveNotes,
                                onCancel: ()=>{
                                    var _item_notes;
                                    setNotesValue((_item_notes = item.notes) !== null && _item_notes !== void 0 ? _item_notes : "");
                                    setEditingNotes(false);
                                }
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(ManualOverrideSection, {
                                item: item,
                                defaultExpanded: manualOverrideExpanded,
                                readOnly: terminalState !== null,
                                onOverrideStatus: handleManualStatusOverride,
                                onAssociatePR: handleManualAssociatePR
                            })
                        ]
                    })
                ]
            })
        ]
    });
}


/***/ }),

/***/ 48443:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   V: () => (/* binding */ toPlainObject)
/* harmony export */ });
// Converts a protobuf Message to a plain JSON-serializable object.
// Used as the serialization boundary so Redux never stores protobuf class instances.
// In @bufbuild/protobuf v2, Message is a plain TypeScript type (not a class),
// so we use JSON round-trip to strip any non-serializable values.
function toPlainObject(msg) {
    // BigInt replacer: protobuf v2 represents int64/uint64 as bigint, which JSON.stringify rejects.
    // Timestamp seconds fit safely in Number (epoch seconds won't exceed 2^53 for millennia).
    return JSON.parse(JSON.stringify(msg, (_, v)=>typeof v === "bigint" ? Number(v) : v));
}


/***/ }),

/***/ 54821:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  zG: () => (/* binding */ StageTracker),
  GF: () => (/* binding */ deriveStageDisplay)
});

// UNUSED EXPORTS: STAGE_ORDER

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ./src/lib/hooks/useBacklogStages.ts
var useBacklogStages = __webpack_require__(3456);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2Fdetail%2FStageTracker.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA62U3XKjMAyF73kKX7YXzoSkSVvyMB0HK6DW2F5bNMBO333HTv-AFtju3sBgCZ3DJ6HVQ1rufyl9s2a_E8as8UhodMYcKEH4DIeEMYneKtFm7KSgCQfhzs9O2IyFazgSCgvNkaDyGctBE7hD8pKs3uqnsf6PKzFWhJxn4a44f0jvodu4btNeh0glXIE6Y-vwYIWUqIvXJ4WeuKdWQca00dCztOlbQq1QA39z9tc23pVH0eFJF_OPxklw3AmJtR--tb2POSejiXvsYBQXH_EzYFHSKKOIGecSCbi3Io8ILpA_QdhGCEeRPxXO1FqO7OtYJjfKuFHMfPqQjKW2Yd4olMM0e92TvJmTdBOSfpkkzeIp-6Z2c6bKCVO4zNRjX3L_k_l7n7KNbb6ew-m52v_zXF1ATJB6miCllpGqFs3u7WBniaM3qqa4s1B7oNctMNo637B9rD3hqeW50QSavsT-X37u_QxC-T3B7Uz_jgv7p4AIXIR7WZer9Q6qECFoiJMT2p-MqzJWWwsuFx6GnZPClzBs3WXrdBy1hCZj6brXsbvYMRNEqQ2i290hefkDr-LGToUGAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2Fdetail_2FStageTracker_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA62U3XKjMAyF73kKX7YXzoSkSVvyMB0HK6DW2F5bNMBO333HTv_AFtju3sBgCZ3DJ6HVQ1rufyl9s2a_E8as8UhodMYcKEH4DIeEMYneKtFm7KSgCQfhzs9O2IyFazgSCgvNkaDyGctBE7hD8pKs3uqnsf6PKzFWhJxn4a44f0jvodu4btNeh0glXIE6Y_vwYIWUqIvXJ4WeuKdWQca00dCztOlbQq1QA39z9tc23pVH0eFJF_OPxklw3AmJtR_tb2POSejiXvsYBQXH_EzYFHSKKOIGecSCbi3Io8ILpA_QdhGCEeRPxXO1FqO7OtYJjfKuFHMfPqQjKW2Yd4olMM0e92TvJmTdBOSfpkkzeIp_6Z2c6bKCVO4zNRjX3L_k_l7n7KNbb6ew_m52v_zXF1ATJB6miCllpGqFs3u7WBniaM3qqa4s1B7oNctMNo637B9rD3hqeW50QSavsT_X37u_QxC_T3B7Uz_jgv7p4AIXIR7WZer9Q6qECFoiJMT2p_MqzJWWwsuFx6GnZPClzBs3WXrdBy1hCZj6brXsbvYMRNEqQ2i290hefkDr_LGToUGAAA_22_7D = __webpack_require__(55943);
;// ./src/components/backlog/detail/StageTracker.css.ts


var archivedRibbon = '_1h6qln47';
var container = '_1h6qln40';
var modifierBadge = '_1h6qln46';
var nodeVariants = {pending:'_1h6qln43 _1h6qln42',active:'_1h6qln44 _1h6qln42',done:'_1h6qln45 _1h6qln42'};
var track = '_1h6qln41';
var trackDimmed = '_1h6qln48';
;// ./src/components/backlog/detail/StageTracker.tsx



const STAGE_ORDER = [
    "idea",
    "ready",
    "in_progress",
    "review",
    "done"
];
const STAGE_LABELS = {
    idea: "Idea",
    ready: "Ready",
    in_progress: "In Progress",
    review: "Review",
    done: "Done"
};
/**
 * Pure derivation from item.status to the tracker's 5-node display. `queued`
 * and `pr_pending` never add a 6th node — they render as a modifier badge on
 * an existing node (In Progress / Review respectively). `refining` folds
 * into Idea. `archived` never guesses which stage the item was archived
 * from — the tracker renders as a dimmed neutral state with an "Archived"
 * ribbon overlay instead (see StageTracker's render logic below).
 *
 * `stages` (Epic 2.9's `useBacklogStages()`, defaulted to the built-in set)
 * is consulted only for a status that matches none of the fixed cases below:
 * a status matching a real, currently-configured custom stage folds into the
 * Idea node with a modifier badge naming that stage (the same "no 6th node,
 * use a modifier instead" treatment `queued`/`pr_pending` already get) rather
 * than being lumped in with genuinely unresolvable data. A status matching
 * nothing in `stages` at all (a deleted/unknown stage) keeps today's dimmed
 * "Archived"-styled fallback unchanged.
 */ function deriveStageDisplay(status) {
    let stages = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : [];
    switch(status){
        case "idea":
        case "refining":
            return {
                activeStage: "idea",
                archived: false
            };
        case "ready":
            return {
                activeStage: "ready",
                archived: false
            };
        case "queued":
            return {
                activeStage: "in_progress",
                modifier: "Queued",
                archived: false
            };
        case "in_progress":
            return {
                activeStage: "in_progress",
                archived: false
            };
        case "review":
            return {
                activeStage: "review",
                archived: false
            };
        case "pr_pending":
            return {
                activeStage: "review",
                modifier: "PR pending",
                archived: false
            };
        case "done":
            return {
                activeStage: "done",
                archived: false
            };
        case "archived":
            // The pre-archive stage isn't reconstructable from status alone — the
            // caller renders a neutral/dimmed tracker with the ribbon overlay
            // rather than guessing a stage (Story 2.1.1 AC).
            return {
                activeStage: "idea",
                archived: true
            };
        default:
            {
                const customStage = stages.find((s)=>s.slug === status);
                if (customStage) {
                    return {
                        activeStage: "idea",
                        modifier: customStage.name,
                        archived: false
                    };
                }
                // Unknown/future server-defined status with no matching configured
                // stage either: don't crash, don't guess a stage — same defensive
                // posture as getStatusClass's archived fallback.
                return {
                    activeStage: "idea",
                    archived: true
                };
            }
    }
}
/** Compact horizontal stepper bound strictly to item.status — always exactly 5 nodes. */ function StageTracker(param) {
    let { status } = param;
    const { stages } = (0,useBacklogStages/* useBacklogStages */._M)();
    const { activeStage, modifier, archived } = deriveStageDisplay(status, stages);
    const activeIndex = STAGE_ORDER.indexOf(activeStage);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: container,
        "data-testid": "stage-tracker",
        "aria-label": archived ? "Lifecycle stage: Archived" : "Lifecycle stage: ".concat(STAGE_LABELS[activeStage]),
        role: "status",
        "aria-live": "polite",
        "aria-atomic": "true",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("ol", {
                className: "".concat(track, " ").concat(archived ? trackDimmed : ""),
                children: STAGE_ORDER.map((stage, index)=>{
                    const nodeState = archived ? "pending" : index < activeIndex ? "done" : index === activeIndex ? "active" : "pending";
                    const isActive = !archived && stage === activeStage;
                    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                        className: nodeVariants[nodeState],
                        "data-testid": "stage-node-".concat(stage),
                        "aria-current": isActive ? "step" : undefined,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                children: STAGE_LABELS[stage]
                            }),
                            isActive && modifier && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: modifierBadge,
                                "data-testid": "stage-modifier-badge",
                                children: modifier
                            })
                        ]
                    }, stage);
                })
            }),
            archived && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: archivedRibbon,
                "data-testid": "stage-archived-ribbon",
                children: "Archived"
            })
        ]
    });
}


/***/ }),

/***/ 57557:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   V: () => (/* binding */ derivePlanReviewStatus)
/* harmony export */ });
/**
 * Single source of truth for the 5-state plan-review status — never
 * persisted server-side, always derived. Both `PlanVerdictBox` (status card)
 * and `ActionsSection` (spawn-gate button visibility) call this rather than
 * each re-deriving state from raw fields, avoiding drift between the two
 * surfaces. See project_plans/plan-approval-ux/decisions/ADR-001.
 *
 * Precedence matters: `skipped` is checked first (a categorically different
 * meaning than "no plan yet"), then `changes_requested` — a non-empty
 * `planRejectionReason` wins even if `planApproved` is also (defensively,
 * should-never-happen) true, since the two are supposed to be mutually
 * exclusive post-symmetry-fix but the derivation must still resolve
 * deterministically if they ever coexist.
 */ function derivePlanReviewStatus(item) {
    if (item.skipPlanning) return "skipped";
    if (item.planRejectionReason) return "changes_requested";
    if (item.planApproved) return "approved";
    if (item.planArtifactsPath) return "pending_review";
    return "no_plan";
}


/***/ }),

/***/ 60976:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   u: () => (/* binding */ useSessionVcs)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__ = __webpack_require__(66144);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_7__ = __webpack_require__(45640);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_8__ = __webpack_require__(16294);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(23575);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(21694);
/* harmony import */ var _lib_store__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(93602);
/* harmony import */ var _lib_store_sessionsSlice__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(60795);
/* harmony import */ var _lib_hooks_useAbortableRequest__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(2663);
/* __next_internal_client_entry_do_not_use__ useSessionVcs auto */ 






/** Fallback poll interval for the currently-focused/visible session. */ const ACTIVE_STATUS_POLL_MS = 60000;
/**
 * Fallback poll interval for a session whose pane is open but not focused
 * (e.g. a background split-pane, or a peeked session behind the active
 * one). Event-driven refetches (Redux `sessionUpdatedAt`) still fire
 * regardless of focus, so this is just the backstop for state changes the
 * push stream might miss — 10x looser than the active interval is enough
 * for a pane the user isn't looking at, and cuts N-1 background panes'
 * contribution to GetVCSStatus/server load by the same factor.
 */ const INACTIVE_STATUS_POLL_MS = 600000;
/**
 * Single source of truth for a session's VCS status and diff data.
 *
 * Intended to be instantiated once per SessionDetail via SessionVcsProvider
 * and consumed by VcsPanel, FilesTab, and DiffViewer through
 * useSessionVcsContext(). This eliminates the 3 independent, uncached fetches
 * those components previously made independently.
 *
 * @param isActive - Whether this session's pane currently has focus.
 * Defaults to true (matches prior always-poll-every-60s behavior) so every
 * caller except a multi-pane layout is unaffected. A multi-pane layout
 * should pass the focused pane's own focus state so unfocused panes fall
 * back to INACTIVE_STATUS_POLL_MS instead of polling as if the user were
 * watching them.
 */ function useSessionVcs(sessionId, baseUrl) {
    let isActive = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : true;
    const [status, setStatus] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(null);
    const [diff, setDiff] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(null);
    const [statusLoading, setStatusLoading] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(true);
    const [diffLoading, setDiffLoading] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(true);
    const [error, setError] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(null);
    // Set once GetVCSStatus reports the session no longer exists, so the 60s
    // fallback interval below stops calling a deleted session forever.
    const stoppedRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(false);
    // Stable client reference — initialized once from the shared transport singleton.
    const clientRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    const getClient = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>{
        if (!clientRef.current) {
            clientRef.current = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_2__/* .getConnectTransport */ .qW)());
        }
        return clientRef.current;
    }, []);
    // Cancel the in-flight status/diff request on the next call or on unmount —
    // this hook is instantiated per-SessionDetail, so switching sessions rapidly
    // without cancellation left every prior request's promise (and its closure
    // over sessionId/setState) alive until it resolved or hit its deadline,
    // growing heap under fast session switching (measured: ~44MB -> ~154MB
    // used JS heap over 138 switches in a profiling run).
    const startStatus = (0,_lib_hooks_useAbortableRequest__WEBPACK_IMPORTED_MODULE_5__/* .useAbortableRequest */ .X)();
    const startDiff = (0,_lib_hooks_useAbortableRequest__WEBPACK_IMPORTED_MODULE_5__/* .useAbortableRequest */ .X)();
    const fetchStatus = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async ()=>{
        if (stoppedRef.current) {
            setStatusLoading(false);
            return;
        }
        const signal = startStatus();
        try {
            const response = await getClient().getVCSStatus({
                id: sessionId
            }, {
                signal
            });
            if (signal.aborted) return;
            if (response.error) {
                setError(new Error(response.error));
                setStatus(null);
            } else {
                var _response_vcsStatus;
                setStatus((_response_vcsStatus = response.vcsStatus) !== null && _response_vcsStatus !== void 0 ? _response_vcsStatus : null);
                setError(null);
            }
        } catch (err) {
            if (signal.aborted) return;
            if (err instanceof _connectrpc_connect__WEBPACK_IMPORTED_MODULE_7__/* .ConnectError */ .T && err.code === _connectrpc_connect__WEBPACK_IMPORTED_MODULE_8__/* .Code */ .C.NotFound) {
                stoppedRef.current = true;
                setError(err);
                setStatus(null);
                return;
            }
            setError(err instanceof Error ? err : new Error("Failed to load VCS status"));
        } finally{
            if (!signal.aborted) setStatusLoading(false);
        }
    }, [
        sessionId,
        getClient,
        startStatus
    ]);
    const fetchDiff = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async ()=>{
        if (stoppedRef.current) {
            setDiffLoading(false);
            return;
        }
        const signal = startDiff();
        setDiffLoading(true);
        try {
            const response = await getClient().getSessionDiff({
                id: sessionId
            }, {
                signal
            });
            if (signal.aborted) return;
            if (response.diffStats) {
                setDiff({
                    content: response.diffStats.content,
                    added: response.diffStats.added,
                    removed: response.diffStats.removed
                });
            } else {
                setDiff(null);
            }
        } catch (err) {
            if (signal.aborted) return;
            if (err instanceof _connectrpc_connect__WEBPACK_IMPORTED_MODULE_7__/* .ConnectError */ .T && err.code === _connectrpc_connect__WEBPACK_IMPORTED_MODULE_8__/* .Code */ .C.NotFound) {
                stoppedRef.current = true;
            }
            // Diff errors are non-fatal — status error is the primary signal.
            console.error("useSessionVcs: failed to load diff:", err);
        } finally{
            if (!signal.aborted) setDiffLoading(false);
        }
    }, [
        sessionId,
        getClient,
        startDiff
    ]);
    const refresh = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>{
        fetchStatus();
        fetchDiff();
    }, [
        fetchStatus,
        fetchDiff
    ]);
    // Watch session state from the Redux store so VCS data re-fetches whenever
    // the session is updated by the server-pushed watchSessions stream, instead
    // of polling on a fixed interval.
    const sessionStatus = (0,_lib_store__WEBPACK_IMPORTED_MODULE_3__/* .useAppSelector */ .GV)((state)=>{
        var _selectAllSessions_find;
        return (_selectAllSessions_find = (0,_lib_store_sessionsSlice__WEBPACK_IMPORTED_MODULE_4__/* .selectAllSessions */ .rt)(state).find((s)=>s.id === sessionId)) === null || _selectAllSessions_find === void 0 ? void 0 : _selectAllSessions_find.status;
    });
    const sessionUpdatedAt = (0,_lib_store__WEBPACK_IMPORTED_MODULE_3__/* .useAppSelector */ .GV)((state)=>{
        var _s_updatedAt;
        const s = (0,_lib_store_sessionsSlice__WEBPACK_IMPORTED_MODULE_4__/* .selectAllSessions */ .rt)(state).find((s)=>s.id === sessionId);
        var _s_updatedAt_seconds;
        return (_s_updatedAt_seconds = s === null || s === void 0 ? void 0 : (_s_updatedAt = s.updatedAt) === null || _s_updatedAt === void 0 ? void 0 : _s_updatedAt.seconds) !== null && _s_updatedAt_seconds !== void 0 ? _s_updatedAt_seconds : null;
    });
    // VCS status: re-fetch on mount and whenever the session changes in Redux.
    // Event-driven is the primary trigger; the fallback interval catches git
    // commits that happen mid-session without changing the session's status
    // field. isActive controls the fallback's cadence, not whether it fires —
    // a backgrounded pane still needs a backstop, just not one tuned for a
    // user actively watching it.
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        stoppedRef.current = false;
        setStatusLoading(true);
        fetchStatus();
        const pollMs = isActive ? ACTIVE_STATUS_POLL_MS : INACTIVE_STATUS_POLL_MS;
        const fallback = setInterval(()=>{
            if (stoppedRef.current) {
                clearInterval(fallback);
                return;
            }
            if (!document.hidden) fetchStatus();
        }, pollMs);
        return ()=>clearInterval(fallback);
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        fetchStatus,
        sessionStatus,
        sessionUpdatedAt,
        isActive
    ]);
    // Diff: fetch once on mount; consumers call refreshDiff() when needed.
    // Cancellation on unmount/re-run is handled by startDiff's useAbortableRequest.
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        fetchDiff();
    }, [
        fetchDiff
    ]);
    return {
        status,
        diff,
        statusLoading,
        diffLoading,
        loading: statusLoading,
        error,
        refreshStatus: fetchStatus,
        refreshDiff: fetchDiff,
        refresh
    };
}


/***/ }),

/***/ 82488:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   T2: () => (/* binding */ getSourceBadgeConfig),
/* harmony export */   TE: () => (/* binding */ resolveSourceId),
/* harmony export */   Zf: () => (/* binding */ isFilterableSourceId),
/* harmony export */   c_: () => (/* binding */ buildExternalRefLabels),
/* harmony export */   is: () => (/* binding */ FILTERABLE_SOURCE_IDS),
/* harmony export */   rg: () => (/* binding */ SOURCE_BADGE_CONFIG)
/* harmony export */ });
/* harmony import */ var _components_backlog_externalRef__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(34243);

const SOURCE_BADGE_CONFIG = {
    github_issues: {
        label: "GitHub",
        idPrefix: "#",
        openLabel: "Open on GitHub"
    },
    linear: {
        label: "Linear",
        idPrefix: "",
        openLabel: "Open in Linear"
    },
    jira: {
        label: "JIRA",
        idPrefix: "",
        openLabel: "Open in JIRA"
    }
};
const EXTERNAL_BADGE_CONFIG = {
    label: "External",
    idPrefix: "",
    openLabel: "Open externally"
};
function getSourceBadgeConfig(pluginId) {
    if (!pluginId) return SOURCE_BADGE_CONFIG.github_issues;
    return Object.hasOwn(SOURCE_BADGE_CONFIG, pluginId) ? SOURCE_BADGE_CONFIG[pluginId] : EXTERNAL_BADGE_CONFIG;
}
/**
 * The source id an item is badged and filtered under: its own plugin id when
 * known, github_issues for legacy imports (no plugin id, has an externalUrl),
 * otherwise undefined.
 */ function resolveSourceId(item) {
    if (item.pluginId && Object.hasOwn(SOURCE_BADGE_CONFIG, item.pluginId)) return item.pluginId;
    return !item.pluginId && item.externalUrl ? "github_issues" : undefined;
}
/**
 * Display refs for an imported item: repo-qualified for GitHub URLs
 * ("acme/widgets#22"; short form drops the owner), bare id otherwise since
 * Linear/JIRA ids (ENG-123) are already unambiguous.
 */ function buildExternalRefLabels(pluginId, externalUrl, externalId) {
    const ref = pluginId && pluginId !== "github_issues" ? undefined : (0,_components_backlog_externalRef__WEBPACK_IMPORTED_MODULE_0__/* .parseExternalRef */ .q)(externalUrl);
    const fullRef = ref ? "".concat(ref.repo, "#").concat(ref.number) : "".concat(getSourceBadgeConfig(pluginId).idPrefix).concat(externalId);
    const shortRef = ref ? "".concat(ref.repo.split("/")[1], "#").concat(ref.number) : fullRef;
    return {
        fullRef,
        shortRef
    };
}
/** Plugin ids offered in the source filter (issue trackers only). */ const FILTERABLE_SOURCE_IDS = [
    "github_issues",
    "linear",
    "jira"
];
function isFilterableSourceId(id) {
    return typeof id === "string" && FILTERABLE_SOURCE_IDS.includes(id);
}


/***/ }),

/***/ 82496:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  B: () => (/* binding */ BlockerChip)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/gen/session/v1/backlog_pb.ts
var backlog_pb = __webpack_require__(79870);
// EXTERNAL MODULE: ./src/lib/utils/connectError.ts
var connectError = __webpack_require__(8854);
// EXTERNAL MODULE: ./src/components/backlog-stuck/stuckReason.ts + 1 modules
var stuckReason = __webpack_require__(91216);
// EXTERNAL MODULE: ./src/components/backlog/ClaimOverrideForm.tsx + 1 modules
var ClaimOverrideForm = __webpack_require__(55468);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FBlockerChip.css.ts.vanilla.css%22%2C%22source%22%3A%22LmVmbmMwIHsKICBvcGFjaXR5OiAwLjc1OwogIGZvbnQtd2VpZ2h0OiB2YXIoLS1fMTllejJyejNmKTsKICBtYXJnaW4tbGVmdDogdmFyKC0tXzE5ZXoycnoyeSk7Cn0KLmVmbmMxIHsKICBkaXNwbGF5OiBpbmxpbmUtZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGFsaWduLWl0ZW1zOiBmbGV4LXN0YXJ0OwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeSk7Cn0KLmVmbmMyIHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxMik7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKfQouZWZuYzMgewogIGRpc3BsYXk6IGlubGluZS1mbGV4OwogIGFsaWduLWl0ZW1zOiBjZW50ZXI7CiAgbWFyZ2luLWxlZnQ6IHZhcigtLV8xOWV6MnJ6MnkpOwogIHBhZGRpbmc6IDFweCB2YXIoLS1fMTllejJyejJ5KTsKICBib3JkZXItcmFkaXVzOiB2YXIoLS1fMTllejJyejM2KTsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2EpOwogIGZvbnQtd2VpZ2h0OiB2YXIoLS1fMTllejJyejNmKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyemQpOwogIGN1cnNvcjogaGVscDsKfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FBlockerChip_css_ts_vanilla_css_22_2C_22source_22_3A_22LmVmbmMwIHsKICBvcGFjaXR5OiAwLjc1OwogIGZvbnQtd2VpZ2h0OiB2YXIoLS1fMTllejJyejNmKTsKICBtYXJnaW4tbGVmdDogdmFyKC0tXzE5ZXoycnoyeSk7Cn0KLmVmbmMxIHsKICBkaXNwbGF5OiBpbmxpbmUtZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGFsaWduLWl0ZW1zOiBmbGV4LXN0YXJ0OwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeSk7Cn0KLmVmbmMyIHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxMik7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKfQouZWZuYzMgewogIGRpc3BsYXk6IGlubGluZS1mbGV4OwogIGFsaWduLWl0ZW1zOiBjZW50ZXI7CiAgbWFyZ2luLWxlZnQ6IHZhcigtLV8xOWV6MnJ6MnkpOwogIHBhZGRpbmc6IDFweCB2YXIoLS1fMTllejJyejJ5KTsKICBib3JkZXItcmFkaXVzOiB2YXIoLS1fMTllejJyejM2KTsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2EpOwogIGZvbnQtd2VpZ2h0OiB2YXIoLS1fMTllejJyejNmKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyemQpOwogIGN1cnNvcjogaGVscDsKfQ_22_7D = __webpack_require__(49413);
;// ./src/components/backlog/BlockerChip.css.ts


var duration = 'efnc0';
var errorText = 'efnc2';
var moreCount = 'efnc3';
var wrapper = 'efnc1';
;// ./src/components/backlog/BlockerChip.tsx
/* __next_internal_client_entry_do_not_use__ BlockerChip auto */ 






const PARKED_LABEL = "Retry unavailable — max attempts reached";
/**
 * "+N more" indicator for any other currently-open StuckReason on this same
 * item — never let `item.reason` render as if it were the only open reason
 * when `otherReasons` is non-empty (BUG-105). The full list is available via
 * the native `title` tooltip rather than dropped silently.
 */ function MoreReasonsBadge(param) {
    let { otherReasons } = param;
    if (otherReasons.length === 0) return null;
    const title = "Also stuck for: ".concat(otherReasons.map(stuckReason/* getStuckReasonLabel */.Lv).join(", "));
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
        className: moreCount,
        "data-testid": "blocker-chip-more",
        title: title,
        children: [
            "+",
            otherReasons.length,
            " more"
        ]
    });
}
/** Retry-now state machine for the interactive "full" variant. */ function useRetryNow(item, onTriggerRemediationNow) {
    const [retryState, setRetryState] = (0,react.useState)("idle");
    const [retryErrorMessage, setRetryErrorMessage] = (0,react.useState)(null);
    const handleRetryNow = (0,react.useCallback)(async ()=>{
        if (!onTriggerRemediationNow || retryState === "pending") return;
        setRetryState("pending");
        setRetryErrorMessage(null);
        try {
            await onTriggerRemediationNow(item.itemId, item.reason);
            setRetryState("idle");
        } catch (err) {
            setRetryState("error");
            setRetryErrorMessage((0,connectError/* getErrorMessage */.u)(err, "Retry failed"));
        }
    }, [
        onTriggerRemediationNow,
        retryState,
        item.itemId,
        item.reason
    ]);
    return {
        retryState,
        retryErrorMessage,
        handleRetryNow
    };
}
/** Read-only chip — the "compact" board-card variant, and the "full" variant
 * when no `onTriggerRemediationNow` handler was supplied. */ function NonInteractiveChip(param) {
    let { item, variant, icon, label, chipClass, otherReasons } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
        className: chipClass,
        "aria-label": label,
        "data-testid": "blocker-chip",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                "aria-hidden": "true",
                children: icon
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                children: label
            }),
            variant === "full" && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: duration,
                "data-testid": "blocker-chip-duration",
                children: (0,stuckReason/* formatStuckDuration */.CR)(item.firstDetectedAt)
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(MoreReasonsBadge, {
                otherReasons: otherReasons
            })
        ]
    });
}
/** Clickable "full" variant — retries the reason's remediation action. */ function InteractiveChip(param) {
    let { item, icon, label, chipClass, otherReasons, onTriggerRemediationNow } = param;
    const parked = (0,stuckReason/* isRemediationParked */.xj)(item);
    const { retryState, retryErrorMessage, handleRetryNow } = useRetryNow(item, onTriggerRemediationNow);
    const ariaLabel = parked ? PARKED_LABEL : retryState === "pending" ? "".concat(label, " — retrying") : "".concat(label, " — retry now");
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
        className: wrapper,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                type: "button",
                className: chipClass,
                "aria-label": ariaLabel,
                title: parked ? PARKED_LABEL : undefined,
                "data-testid": "blocker-chip-retry",
                disabled: parked || retryState === "pending",
                onClick: handleRetryNow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        "aria-hidden": "true",
                        children: icon
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        children: label
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: duration,
                        "data-testid": "blocker-chip-duration",
                        children: retryState === "pending" ? "Retrying…" : (0,stuckReason/* formatStuckDuration */.CR)(item.firstDetectedAt)
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(MoreReasonsBadge, {
                otherReasons: otherReasons
            }),
            retryState === "error" && retryErrorMessage && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: errorText,
                "data-testid": "blocker-chip-error",
                role: "alert",
                children: retryErrorMessage
            })
        ]
    });
}
/** BLOCKED_BY_CLAIM chip: toggles a reason form rather than acting immediately. */ function ClaimBlockChip(param) {
    let { item, icon, label, chipClass, otherReasons, onOverrideClaimBlock } = param;
    const [open, setOpen] = (0,react.useState)(false);
    const [pending, setPending] = (0,react.useState)(false);
    const [errorMessage, setErrorMessage] = (0,react.useState)(null);
    const handleConfirm = (0,react.useCallback)(async (reason)=>{
        setPending(true);
        setErrorMessage(null);
        try {
            await onOverrideClaimBlock(item.itemId, reason);
            setOpen(false);
        } catch (err) {
            setErrorMessage((0,connectError/* getErrorMessage */.u)(err, "Override failed"));
        } finally{
            setPending(false);
        }
    }, [
        onOverrideClaimBlock,
        item.itemId
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
        className: wrapper,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                type: "button",
                className: chipClass,
                "aria-label": "".concat(label, " — override"),
                "aria-expanded": open,
                title: item.context || undefined,
                "data-testid": "blocker-chip-claim-override",
                onClick: ()=>setOpen((prev)=>!prev),
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        "aria-hidden": "true",
                        children: icon
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        children: label
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: duration,
                        "data-testid": "blocker-chip-duration",
                        children: (0,stuckReason/* formatStuckDuration */.CR)(item.firstDetectedAt)
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(MoreReasonsBadge, {
                otherReasons: otherReasons
            }),
            open && /*#__PURE__*/ (0,jsx_runtime.jsx)(ClaimOverrideForm/* ClaimOverrideForm */.V, {
                label: "Reason for working on this despite the other host's claim (required)",
                confirmLabel: "Override claim block",
                onConfirm: handleConfirm,
                onCancel: ()=>setOpen(false),
                busy: pending,
                errorMessage: errorMessage
            })
        ]
    });
}
/**
 * Derived (never stored) "waiting on X" indicator, sourced from
 * useStuckBacklogItems()/StuckBacklogItem.reason. Reuses
 * stuckReason.ts's icon/label/duration formatting and color-class mapping
 * verbatim — one source of truth shared by the detail view and board card,
 * instead of two independent implementations drifting apart.
 */ function BlockerChip(param) {
    let { item, variant, onTriggerRemediationNow, onOverrideClaimBlock, otherReasons = [] } = param;
    const visual = {
        item,
        icon: (0,stuckReason/* getStuckReasonIcon */.qk)(item.reason),
        label: (0,stuckReason/* getStuckReasonLabel */.Lv)(item.reason),
        chipClass: (0,stuckReason/* getStuckReasonClass */.px)(item.reason),
        otherReasons
    };
    if (variant === "full" && item.reason === backlog_pb/* StuckReason */.Pwx.BLOCKED_BY_CLAIM && onOverrideClaimBlock) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)(ClaimBlockChip, {
            ...visual,
            onOverrideClaimBlock: onOverrideClaimBlock
        });
    }
    if (variant === "full" && onTriggerRemediationNow) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)(InteractiveChip, {
            ...visual,
            onTriggerRemediationNow: onTriggerRemediationNow
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(NonInteractiveChip, {
        ...visual,
        variant: variant
    });
}


/***/ }),

/***/ 85546:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   p: () => (/* binding */ getStatusLabel)
/* harmony export */ });
/* unused harmony export STATUS_LABELS */
// Shared status display helpers for backlog items.
// CSS class mappings are intentionally kept per-component (vanilla-extract
// generates scoped class names), but labels and the fallback formatter are shared.
const STATUS_LABELS = {
    idea: "Idea",
    refining: "Refining",
    ready: "Ready",
    queued: "Queued",
    in_progress: "In Progress",
    review: "Review",
    done: "Done",
    archived: "Archived"
};
/** Human-readable label for any status string, including unknown server-defined ones. */ const getStatusLabel = (s)=>{
    var _STATUS_LABELS_s;
    return (_STATUS_LABELS_s = STATUS_LABELS[s]) !== null && _STATUS_LABELS_s !== void 0 ? _STATUS_LABELS_s : s.replace(/_/g, " ");
};


/***/ }),

/***/ 86082:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   c: () => (/* binding */ useHistoryFullTextSearch)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(66144);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__ = __webpack_require__(45640);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_7__ = __webpack_require__(16294);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(23575);
/* harmony import */ var _bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(28765);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(21694);
/* harmony import */ var _useDebounce__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(22479);
/* __next_internal_client_entry_do_not_use__ useHistoryFullTextSearch,default auto */ 





/**
 * Hook for full-text search across Claude conversation history.
 * Provides debounced search with pagination support.
 */ function useHistoryFullTextSearch() {
    let options = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : {};
    const { debounceMs = 300, autoSearch = true } = options;
    const [state, setState] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)({
        results: [],
        totalMatches: 0,
        queryTimeMs: 0,
        hasMore: false,
        loading: false,
        error: null
    });
    const [query, setQuery] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)("");
    const debouncedQuery = (0,_useDebounce__WEBPACK_IMPORTED_MODULE_3__/* .useDebounce */ .d)(query, debounceMs);
    const clientRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    const abortControllerRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    // Initialize ConnectRPC client
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        clientRef.current = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_4__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_2__/* .getConnectTransport */ .qW)());
    }, []);
    const toSearchMessageItems = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)((msgs)=>msgs.map((m)=>({
                role: m.role,
                content: m.content,
                timestamp: m.timestamp ? (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_5__/* .timestampDate */ .FP)(m.timestamp) : null,
                model: m.model
            })), []);
    // Convert protobuf SearchResult to our interface
    const convertResult = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)((result)=>{
        var _result_metadata, _result_metadata1, _result_metadata2, _result_metadata3;
        var _result_metadata_isMetadataMatch, _result_metadata_matchSource, _result_metadata_model, _result_moreMatchesInSessionCount;
        return {
            sessionId: result.sessionId,
            sessionName: result.sessionName,
            project: result.project,
            messageIndex: result.messageIndex,
            score: result.score,
            snippets: result.snippets.map((snippet)=>({
                    text: snippet.text,
                    highlightRanges: snippet.highlightRanges.map((hr)=>({
                            start: hr.start,
                            end: hr.end
                        })),
                    messageRole: snippet.messageRole,
                    messageTime: snippet.messageTime ? (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_5__/* .timestampDate */ .FP)(snippet.messageTime) : null
                })),
            metadata: {
                isMetadataMatch: (_result_metadata_isMetadataMatch = (_result_metadata = result.metadata) === null || _result_metadata === void 0 ? void 0 : _result_metadata.isMetadataMatch) !== null && _result_metadata_isMetadataMatch !== void 0 ? _result_metadata_isMetadataMatch : false,
                matchSource: (_result_metadata_matchSource = (_result_metadata1 = result.metadata) === null || _result_metadata1 === void 0 ? void 0 : _result_metadata1.matchSource) !== null && _result_metadata_matchSource !== void 0 ? _result_metadata_matchSource : "",
                model: (_result_metadata_model = (_result_metadata2 = result.metadata) === null || _result_metadata2 === void 0 ? void 0 : _result_metadata2.model) !== null && _result_metadata_model !== void 0 ? _result_metadata_model : "",
                createdAt: ((_result_metadata3 = result.metadata) === null || _result_metadata3 === void 0 ? void 0 : _result_metadata3.createdAt) ? (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_5__/* .timestampDate */ .FP)(result.metadata.createdAt) : null
            },
            moreMatchesInSessionCount: (_result_moreMatchesInSessionCount = result.moreMatchesInSessionCount) !== null && _result_moreMatchesInSessionCount !== void 0 ? _result_moreMatchesInSessionCount : 0,
            contextWindow: toSearchMessageItems(result.contextWindow),
            bookendFirst: toSearchMessageItems(result.bookendFirst),
            bookendLast: toSearchMessageItems(result.bookendLast)
        };
    }, [
        toSearchMessageItems
    ]);
    // Execute search
    const search = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (searchOptions)=>{
        if (!clientRef.current) return;
        if (!searchOptions.query.trim()) {
            setState((prev)=>({
                    ...prev,
                    results: [],
                    totalMatches: 0,
                    hasMore: false,
                    loading: false,
                    error: null
                }));
            return;
        }
        // Cancel any pending request
        if (abortControllerRef.current) {
            abortControllerRef.current.abort();
        }
        abortControllerRef.current = new AbortController();
        setState((prev)=>({
                ...prev,
                loading: true,
                error: null
            }));
        try {
            var _searchOptions_limit, _searchOptions_offset, _searchOptions_project, _searchOptions_model, _searchOptions_groupBySession, _searchOptions_includeContext, _searchOptions_excludeAutomationSessions;
            const response = await clientRef.current.searchClaudeHistory({
                query: searchOptions.query,
                limit: (_searchOptions_limit = searchOptions.limit) !== null && _searchOptions_limit !== void 0 ? _searchOptions_limit : 20,
                offset: (_searchOptions_offset = searchOptions.offset) !== null && _searchOptions_offset !== void 0 ? _searchOptions_offset : 0,
                project: (_searchOptions_project = searchOptions.project) !== null && _searchOptions_project !== void 0 ? _searchOptions_project : "",
                model: (_searchOptions_model = searchOptions.model) !== null && _searchOptions_model !== void 0 ? _searchOptions_model : "",
                startTime: searchOptions.startTime ? (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_5__/* .timestampFromDate */ .LI)(searchOptions.startTime) : undefined,
                endTime: searchOptions.endTime ? (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_5__/* .timestampFromDate */ .LI)(searchOptions.endTime) : undefined,
                groupBySession: (_searchOptions_groupBySession = searchOptions.groupBySession) !== null && _searchOptions_groupBySession !== void 0 ? _searchOptions_groupBySession : false,
                includeContext: (_searchOptions_includeContext = searchOptions.includeContext) !== null && _searchOptions_includeContext !== void 0 ? _searchOptions_includeContext : false,
                excludeAutomationSessions: (_searchOptions_excludeAutomationSessions = searchOptions.excludeAutomationSessions) !== null && _searchOptions_excludeAutomationSessions !== void 0 ? _searchOptions_excludeAutomationSessions : false
            }, {
                signal: abortControllerRef.current.signal
            });
            const newResults = response.results.map(convertResult);
            setState((prev)=>({
                    // Append results if append flag is set, otherwise replace
                    results: searchOptions.append ? [
                        ...prev.results,
                        ...newResults
                    ] : newResults,
                    totalMatches: response.totalMatches,
                    queryTimeMs: Number(response.queryTimeMs),
                    hasMore: response.hasMore,
                    loading: false,
                    error: null
                }));
        } catch (err) {
            // Comprehensive abort/cancel error detection
            // 1. Native AbortError
            if (err instanceof Error && err.name === "AbortError") {
                return;
            }
            // 2. DOMException AbortError
            if (err instanceof DOMException && err.name === "AbortError") {
                return;
            }
            // 3. ConnectRPC error codes for cancellation
            if (err instanceof _connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__/* .ConnectError */ .T && (err.code === _connectrpc_connect__WEBPACK_IMPORTED_MODULE_7__/* .Code */ .C.Canceled || err.code === _connectrpc_connect__WEBPACK_IMPORTED_MODULE_7__/* .Code */ .C.Aborted)) {
                return;
            }
            // 4. Check error message for abort/cancel indicators (fallback)
            if (err instanceof Error) {
                const msg = err.message.toLowerCase();
                if (msg.includes("abort") || msg.includes("cancel")) {
                    return;
                }
            }
            // Debug: log unexpected errors to help diagnose
            console.debug("[FullTextSearch] Search error:", err, "type:", typeof err, "name:", err === null || err === void 0 ? void 0 : err.name, "message:", err === null || err === void 0 ? void 0 : err.message);
            const error = err instanceof Error ? err : new Error("Search failed");
            setState((prev)=>({
                    ...prev,
                    loading: false,
                    error
                }));
        }
    }, [
        convertResult
    ]);
    // Load more results (pagination)
    const loadMore = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async ()=>{
        if (!state.hasMore || state.loading) return;
        await search({
            query: debouncedQuery,
            offset: state.results.length,
            append: true
        });
    }, [
        search,
        debouncedQuery,
        state.hasMore,
        state.loading,
        state.results.length
    ]);
    // Clear search results
    const clearSearch = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>{
        setQuery("");
        setState({
            results: [],
            totalMatches: 0,
            queryTimeMs: 0,
            hasMore: false,
            loading: false,
            error: null
        });
    }, []);
    // Auto-search when debounced query changes
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        if (autoSearch && debouncedQuery) {
            search({
                query: debouncedQuery
            });
        } else if (!debouncedQuery) {
            setState((prev)=>({
                    ...prev,
                    results: [],
                    totalMatches: 0,
                    hasMore: false
                }));
        }
    }, [
        autoSearch,
        debouncedQuery,
        search
    ]);
    // Cleanup on unmount
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        return ()=>{
            if (abortControllerRef.current) {
                abortControllerRef.current.abort();
            }
        };
    }, []);
    return {
        // State
        ...state,
        query,
        // Actions
        setQuery,
        search,
        loadMore,
        clearSearch
    };
}
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (useHistoryFullTextSearch)));


/***/ }),

/***/ 88253:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   P: () => (/* binding */ usePersistedViewState)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* __next_internal_client_entry_do_not_use__ usePersistedViewState auto */ 
function readRaw(key) {
    if (false) {}
    try {
        return window.localStorage.getItem(key);
    } catch (e) {
        return null;
    }
}
function writeRaw(key, value) {
    if (false) {}
    try {
        window.localStorage.setItem(key, value);
    } catch (e) {
    // Ignore storage errors (private browsing / quota exceeded) — state
    // still lives in memory, it just won't survive a reload.
    }
}
function removeRaw(key) {
    if (false) {}
    try {
        window.localStorage.removeItem(key);
    } catch (e) {
    // Ignore storage errors
    }
}
function loadField(config) {
    const raw = readRaw(config.key);
    if (raw === null) return config.defaultValue;
    let parsed;
    try {
        parsed = JSON.parse(raw);
    } catch (e) {
        return config.defaultValue;
    }
    const value = config.deserialize ? config.deserialize(parsed) : parsed;
    if (config.isValid && !config.isValid(value)) return config.defaultValue;
    return value;
}
function saveField(config, value) {
    const serialized = config.serialize ? config.serialize(value) : value;
    writeRaw(config.key, JSON.stringify(serialized));
}
function buildDefaults(fields) {
    const out = {};
    Object.keys(fields).forEach((name)=>{
        out[name] = fields[name].defaultValue;
    });
    return out;
}
/**
 * Generic hydration-safe localStorage persistence for a page/component's view
 * state (search, filters, sort, grouping, etc). State always starts at the
 * hardcoded defaults so SSR/static-export first paint can never mismatch a
 * client-only localStorage read; a mount-only effect then loads persisted
 * values and flips `isHydrated`.
 */ function usePersistedViewState(fields) {
    const fieldsRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(fields);
    fieldsRef.current = fields;
    const defaults = (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>buildDefaults(fields), [
        fields
    ]);
    const defaultsRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(defaults);
    defaultsRef.current = defaults;
    const [state, setState] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(defaults);
    const [isHydrated, setIsHydrated] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(false);
    // Skips the persistence effect's next run once, so resetToDefaults's
    // removeRaw calls aren't immediately undone by re-saving the new (default)
    // state back into localStorage.
    const skipPersistRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(false);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        const loaded = {};
        Object.keys(fieldsRef.current).forEach((name)=>{
            loaded[name] = loadField(fieldsRef.current[name]);
        });
        setState(loaded);
        setIsHydrated(true);
    // Mount-only: hydration must run exactly once regardless of prop identity.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        if (!isHydrated) return;
        if (skipPersistRef.current) {
            skipPersistRef.current = false;
            return;
        }
        Object.keys(fieldsRef.current).forEach((name)=>{
            saveField(fieldsRef.current[name], state[name]);
        });
    }, [
        state,
        isHydrated
    ]);
    const setters = (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>{
        const out = {};
        Object.keys(fields).forEach((name)=>{
            out[name] = (value)=>{
                setState((prev)=>({
                        ...prev,
                        [name]: typeof value === "function" ? value(prev[name]) : value
                    }));
            };
        });
        return out;
    }, [
        fields
    ]);
    const clearStorage = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>{
        Object.keys(fieldsRef.current).forEach((name)=>{
            removeRaw(fieldsRef.current[name].key);
        });
    }, []);
    const resetToDefaults = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>{
        skipPersistRef.current = true;
        clearStorage();
        setState(defaultsRef.current);
    }, [
        clearStorage
    ]);
    return {
        state,
        setters,
        isHydrated,
        resetToDefaults,
        clearStorage
    };
}


/***/ }),

/***/ 88627:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  E: () => (/* binding */ ClaimedElsewhereNotice),
  $: () => (/* binding */ claimingHostLabel)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/components/backlog/ClaimOverrideForm.tsx + 1 modules
var ClaimOverrideForm = __webpack_require__(55468);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FClaimedElsewhereNotice.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xcnhtNThoMCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeik7CiAgbWFyZ2luLXRvcDogdmFyKC0tXzE5ZXoycnoyeik7CiAgcGFkZGluZzogdmFyKC0tXzE5ZXoycnozMCk7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnp4KTsKICBib3JkZXItcmFkaXVzOiB2YXIoLS1fMTllejJyejM3KTsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyenkpOwp9Ci5fMXJ4bTU4aDEgewogIG1hcmdpbjogMDsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2IpOwogIGZvbnQtd2VpZ2h0OiB2YXIoLS1fMTllejJyejNnKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnp6KTsKfQouXzFyeG01OGgyIHsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXgtZGlyZWN0aW9uOiBjb2x1bW47CiAgZ2FwOiB2YXIoLS1fMTllejJyejJ5KTsKfQouXzFyeG01OGgzIHsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2IpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejApOwogIHdvcmQtYnJlYWs6IGJyZWFrLXdvcmQ7Cn0KLl8xcnhtNThoNCB7CiAgZGlzcGxheTogZmxleDsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MnopOwp9Ci5fMXJ4bTU4aDUgewogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MnkpIHZhcigtLV8xOWV6MnJ6MzApOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzcpOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgY3Vyc29yOiBwb2ludGVyOwogIGJhY2tncm91bmQ6IG5vbmU7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnpnKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxKTsKfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FClaimedElsewhereNotice_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xcnhtNThoMCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeik7CiAgbWFyZ2luLXRvcDogdmFyKC0tXzE5ZXoycnoyeik7CiAgcGFkZGluZzogdmFyKC0tXzE5ZXoycnozMCk7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnp4KTsKICBib3JkZXItcmFkaXVzOiB2YXIoLS1fMTllejJyejM3KTsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyenkpOwp9Ci5fMXJ4bTU4aDEgewogIG1hcmdpbjogMDsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2IpOwogIGZvbnQtd2VpZ2h0OiB2YXIoLS1fMTllejJyejNnKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnp6KTsKfQouXzFyeG01OGgyIHsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXgtZGlyZWN0aW9uOiBjb2x1bW47CiAgZ2FwOiB2YXIoLS1fMTllejJyejJ5KTsKfQouXzFyeG01OGgzIHsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2IpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejApOwogIHdvcmQtYnJlYWs6IGJyZWFrLXdvcmQ7Cn0KLl8xcnhtNThoNCB7CiAgZGlzcGxheTogZmxleDsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MnopOwp9Ci5fMXJ4bTU4aDUgewogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MnkpIHZhcigtLV8xOWV6MnJ6MzApOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzcpOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgY3Vyc29yOiBwb2ludGVyOwogIGJhY2tncm91bmQ6IG5vbmU7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnpnKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxKTsKfQ_22_7D = __webpack_require__(34819);
;// ./src/components/backlog/ClaimedElsewhereNotice.css.ts


var actionButton = '_1rxm58h5';
var container = '_1rxm58h0';
var ClaimedElsewhereNotice_css_entry = '_1rxm58h2';
var entryActions = '_1rxm58h4';
var entryText = '_1rxm58h3';
var heading = '_1rxm58h1';
;// ./src/components/backlog/ClaimedElsewhereNotice.tsx
/* __next_internal_client_entry_do_not_use__ claimingHostLabel,ClaimedElsewhereNotice auto */ 



/** The claiming host's name for display: the deep link's authority, else its opaque ID. */ function claimingHostLabel(claim) {
    try {
        const host = new URL(claim.itemDeepLink).hostname;
        if (host) return host;
    } catch (e) {
    // fall through to the opaque host ID
    }
    return claim.claimingHostId;
}
/**
 * Third import-result bucket, distinct from "already imported" (same host) and
 * "import failed": another stapler-squad host already claimed the issue. Offers
 * Copy link and an audited "Import anyway" (reason >= 5 characters).
 */ function ClaimedElsewhereNotice(param) {
    let { entries, onImportAnyway } = param;
    const [openUrl, setOpenUrl] = (0,react.useState)(null);
    const [busyUrl, setBusyUrl] = (0,react.useState)(null);
    const [copiedUrl, setCopiedUrl] = (0,react.useState)(null);
    const [errorUrl, setErrorUrl] = (0,react.useState)(null);
    const copyLink = (0,react.useCallback)(async (entry)=>{
        try {
            await navigator.clipboard.writeText(entry.claim.itemDeepLink);
            setCopiedUrl(entry.issueUrl);
        } catch (e) {
            setCopiedUrl(null);
        }
    }, []);
    const confirm = (0,react.useCallback)(async (entry, reason)=>{
        setBusyUrl(entry.issueUrl);
        setErrorUrl(null);
        try {
            await onImportAnyway(entry, reason);
            setOpenUrl(null);
        } catch (err) {
            setErrorUrl({
                url: entry.issueUrl,
                message: err instanceof Error ? err.message : "Import failed."
            });
        } finally{
            setBusyUrl(null);
        }
    }, [
        onImportAnyway
    ]);
    if (entries.length === 0) return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: container,
        role: "status",
        "data-testid": "claimed-elsewhere-notice",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: heading,
                children: entries.length === 1 ? "1 issue is already claimed by another host" : "".concat(entries.length, " issues are already claimed by another host")
            }),
            entries.map((entry)=>{
                const host = claimingHostLabel(entry.claim);
                return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: ClaimedElsewhereNotice_css_entry,
                    "data-testid": "claimed-elsewhere-entry",
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                            className: entryText,
                            children: [
                                entry.issueUrl,
                                " — ",
                                entry.claim.disputed ? "disputed between hosts, currently ".concat(host) : "claimed by ".concat(host)
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: entryActions,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "button",
                                    className: actionButton,
                                    onClick: ()=>void copyLink(entry),
                                    "data-testid": "claimed-elsewhere-copy",
                                    children: copiedUrl === entry.issueUrl ? "Copied" : "Copy link to ".concat(host, "'s item")
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "button",
                                    className: actionButton,
                                    "aria-expanded": openUrl === entry.issueUrl,
                                    onClick: ()=>setOpenUrl((prev)=>prev === entry.issueUrl ? null : entry.issueUrl),
                                    "data-testid": "claimed-elsewhere-import-anyway",
                                    children: "Import anyway"
                                })
                            ]
                        }),
                        openUrl === entry.issueUrl && /*#__PURE__*/ (0,jsx_runtime.jsx)(ClaimOverrideForm/* ClaimOverrideForm */.V, {
                            label: "Reason for importing anyway (required)",
                            confirmLabel: "Import anyway",
                            onConfirm: (reason)=>confirm(entry, reason),
                            onCancel: ()=>setOpenUrl(null),
                            busy: busyUrl === entry.issueUrl,
                            errorMessage: (errorUrl === null || errorUrl === void 0 ? void 0 : errorUrl.url) === entry.issueUrl ? errorUrl.message : null
                        })
                    ]
                }, entry.issueUrl);
            })
        ]
    });
}


/***/ }),

/***/ 90080:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  i: () => (/* binding */ TriageLoadingIndicator)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FTriageLoadingIndicator.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-VUwW7iMBC98xVzbA-uksACdS_7J5WJJ8ks9jiynW5g1X9fJQERKBSkstqV9mb7PY9n3rzx9zVuCq8sBnhdsLWaEvg1ASi8s_0CIHrFoXDeSvAuqogPicby8WUC8D4BiO4ibzofMd8nT7sX0v6CplAbtZFQGGw7ijJUsqCINkjIkSP67rhUtYQ35R-EeE2fcZv5bbbtY9ZKa-LyArpS-br0rmF9SlgMuPMavYS0biE4Q_qEVY5YwitNTTgNNB0iFY6jCLTFD_iqx3NnnD_F0seXkSTZsSTEhhjF15XJ6vbvqfN8L3WmZ9VZGZevuwg_ScdKQjqre7UqpLKKh_2-lOyWUqKrxdmM7M0FKyarIjmWh5FKnpYBuqyVB-KCmCL24hhsRag88VpCMq559h_W_G34egy2EtIbvTEf7nQmK5Qls_kwEs0VG6rLNsx6yCpfEguDRZSgmujG7y_698fjxI7xXiM0H3JrfOiSqx3tZ_-TdK9Vev1zOO0QgCUWO8vNdhbrjva2y5bD2a1_-o8mRCo2IncckeMBGukqK_eGvlf3bLHJWL6zjOrIKMurjTref274f6ole_n_cEuWd2rJb9TR-MF0CAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FTriageLoadingIndicator_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_VUwW7iMBC98xVzbA_uksACdS_7J5WJJ8ks9jiynW5g1X9fJQERKBSkstqV9mb7PY9n3rzx9zVuCq8sBnhdsLWaEvg1ASi8s_0CIHrFoXDeSvAuqogPicby8WUC8D4BiO4ibzofMd8nT7sX0v6CplAbtZFQGGw7ijJUsqCINkjIkSP67rhUtYQ35R_EeE2fcZv5bbbtY9ZKa_LyArpS_br0rmF9SlgMuPMavYS0biE4Q_qEVY5YwitNTTgNNB0iFY6jCLTFD_iqx3NnnD_F0seXkSTZsSTEhhjF15XJ6vbvqfN8L3WmZ9VZGZevuwg_ScdKQjqre7UqpLKKh_2_lOyWUqKrxdmM7M0FKyarIjmWh5FKnpYBuqyVB_KCmCL24hhsRag88VpCMq559h_W_G34egy2EtIbvTEf7nQmK5Qls_kwEs0VG6rLNsx6yCpfEguDRZSgmujG7y_698fjxI7xXiM0H3JrfOiSqx3tZ_TdK9Vev1zOO0QgCUWO8vNdhbrjva2y5bD2a1_o8mRCo2IncckeMBGukqK_eGvlf3bLHJWL6zjOrIKMurjTref274f6ole_n_cEuWd2rJb9TR_MF0CAAA_22_7D = __webpack_require__(97510);
;// ./src/components/backlog/TriageLoadingIndicator.css.ts


var cancelButton = '_7nmmdi7';
var cancelButtonCompact = '_7nmmdi8';
var compactContainer = '_7nmmdi2';
var container = '_7nmmdi1';
var elapsed = '_7nmmdi6';
var label = '_7nmmdi5';
var spinKeyframes = '_7nmmdi0';
var spinner = '_7nmmdi3';
var spinnerHidden = '_7nmmdi4';
;// ./src/components/backlog/TriageLoadingIndicator.tsx
/* __next_internal_client_entry_do_not_use__ TriageLoadingIndicator auto */ 
// +feature: backlog:triage-loading-indicator

const TRIAGE_TIMEOUT_SECONDS = 180;
const TRIAGE_SLOW_THRESHOLD_SECONDS = 60;
const ARIA_LABEL_INTERVAL_SECONDS = 30;
function getLabel(context, elapsedSeconds) {
    if (context === "item") {
        if (elapsedSeconds < TRIAGE_SLOW_THRESHOLD_SECONDS) return "Thinking about acceptance criteria...";
        return "Still thinking — up to 3 min";
    }
    // list / compact context
    if (elapsedSeconds < TRIAGE_SLOW_THRESHOLD_SECONDS) return "Thinking...";
    return "Still working — up to 3 min";
}
function TriageLoadingIndicator(param) {
    let { elapsedSeconds, context, onCancel, compact = false } = param;
    if (elapsedSeconds >= TRIAGE_TIMEOUT_SECONDS) {
        return null;
    }
    const ariaElapsed = Math.floor(elapsedSeconds / ARIA_LABEL_INTERVAL_SECONDS) * ARIA_LABEL_INTERVAL_SECONDS;
    const ariaLabel = "Triage in progress, ".concat(ariaElapsed, " seconds elapsed");
    const labelText = getLabel(context, elapsedSeconds);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: compact ? compactContainer : container,
        role: "status",
        "aria-live": "polite",
        "aria-label": ariaLabel,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: spinnerHidden,
                "aria-hidden": "true"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: label,
                children: labelText
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                className: elapsed,
                children: [
                    elapsedSeconds,
                    "s"
                ]
            }),
            compact ? /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                "aria-label": "Cancel triage",
                onClick: onCancel,
                className: cancelButtonCompact,
                children: "\xd7"
            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                "aria-label": "Cancel triage",
                onClick: onCancel,
                className: cancelButton,
                children: "Stop"
            })
        ]
    });
}


/***/ }),

/***/ 93758:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Q: () => (/* binding */ useVcsStatus),
/* harmony export */   U: () => (/* binding */ prefetchVcsStatus)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(66144);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(45640);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__ = __webpack_require__(16294);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(23575);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(21694);
/* harmony import */ var _lib_hooks_useAbortableRequest__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(2663);
/* __next_internal_client_entry_do_not_use__ prefetchVcsStatus,useVcsStatus auto */ 




// Module-level cache shared across all hook instances.
// Both VcsPanel and FilesTab read from the same cache, eliminating duplicate requests.
const vcsCache = new Map();
const VCS_CACHE_TTL_MS = 15000;
function getCached(sessionId) {
    const entry = vcsCache.get(sessionId);
    if (entry && Date.now() - entry.timestamp < VCS_CACHE_TTL_MS) return entry;
    return null;
}
/**
 * Warm the VCS cache for a session without rendering anything.
 * Call from SessionDetail when a session is selected.
 */ async function prefetchVcsStatus(sessionId, baseUrl) {
    if (getCached(sessionId)) return;
    try {
        const client = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_4__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_2__/* .getConnectTransport */ .qW)());
        // One-shot cache warm with no component lifecycle to cancel against;
        // the TTL cache above (not a signal) is what stops this from
        // compounding on rapid session switches.
        // abort-signal-exempt
        const response = await client.getVCSStatus({
            id: sessionId
        });
        var _response_vcsStatus;
        vcsCache.set(sessionId, {
            data: (_response_vcsStatus = response.vcsStatus) !== null && _response_vcsStatus !== void 0 ? _response_vcsStatus : null,
            error: response.error || null,
            timestamp: Date.now()
        });
    } catch (e) {
    // Prefetch failures are silent – the hook will retry on mount.
    }
}
/**
 * Shared hook for VCS status with module-level caching and visibility-aware polling.
 * Multiple components using this hook for the same sessionId share one cache entry
 * and avoid redundant network requests.
 */ function useVcsStatus(sessionId, baseUrl) {
    let pollIntervalMs = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : 15000;
    const hit = getCached(sessionId);
    var _hit_data;
    const [data, setData] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)((_hit_data = hit === null || hit === void 0 ? void 0 : hit.data) !== null && _hit_data !== void 0 ? _hit_data : null);
    const [loading, setLoading] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(!hit);
    var _hit_error;
    const [error, setError] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)((_hit_error = hit === null || hit === void 0 ? void 0 : hit.error) !== null && _hit_error !== void 0 ? _hit_error : null);
    // Set once GetVCSStatus reports the session no longer exists, so the
    // polling interval below stops calling a deleted session forever.
    const stoppedRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(false);
    // Cancel the in-flight request on the next call or on unmount — see
    // useSessionVcs.ts (the sibling hook this pattern was first fixed in) for
    // the measured impact of skipping this under rapid session switching.
    const startFetch = (0,_lib_hooks_useAbortableRequest__WEBPACK_IMPORTED_MODULE_3__/* .useAbortableRequest */ .X)();
    const fetchVcs = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async function() {
        let skipCache = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : false;
        if (!sessionId || stoppedRef.current) {
            setLoading(false);
            return;
        }
        if (!skipCache) {
            const cached = getCached(sessionId);
            if (cached) {
                setData(cached.data);
                setError(cached.error);
                setLoading(false);
                return;
            }
        }
        const signal = startFetch();
        try {
            const client = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_4__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_2__/* .getConnectTransport */ .qW)());
            const response = await client.getVCSStatus({
                id: sessionId
            }, {
                signal
            });
            if (signal.aborted) return;
            var _response_vcsStatus;
            const entry = {
                data: (_response_vcsStatus = response.vcsStatus) !== null && _response_vcsStatus !== void 0 ? _response_vcsStatus : null,
                error: response.error || null,
                timestamp: Date.now()
            };
            vcsCache.set(sessionId, entry);
            setData(entry.data);
            setError(entry.error);
        } catch (err) {
            if (signal.aborted) return;
            if (err instanceof _connectrpc_connect__WEBPACK_IMPORTED_MODULE_5__/* .ConnectError */ .T && err.code === _connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__/* .Code */ .C.NotFound) {
                stoppedRef.current = true;
                vcsCache.delete(sessionId);
                setError(err.message);
                setData(null);
                return;
            }
            setError(err instanceof Error ? err.message : "Failed to load VCS status");
        } finally{
            if (!signal.aborted) setLoading(false);
        }
    }, [
        sessionId,
        baseUrl,
        startFetch
    ] // eslint-disable-line react-hooks/exhaustive-deps
    );
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        stoppedRef.current = false;
        fetchVcs();
        const interval = setInterval(()=>{
            if (stoppedRef.current) {
                clearInterval(interval);
                return;
            }
            if (!document.hidden) fetchVcs();
        }, pollIntervalMs);
        return ()=>clearInterval(interval);
    }, [
        sessionId,
        baseUrl
    ]); // eslint-disable-line react-hooks/exhaustive-deps
    return {
        data,
        loading,
        error,
        refetch: ()=>fetchVcs(true)
    };
}


/***/ }),

/***/ 95025:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  E: () => (/* binding */ Skeleton)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fui%2FSkeleton.css.ts.vanilla.css%22%2C%22source%22%3A%22QGtleWZyYW1lcyBwNjN1MGgwIHsKICAwJSB7CiAgICBiYWNrZ3JvdW5kLXBvc2l0aW9uOiAyMDAlIDA7CiAgfQogIDEwMCUgewogICAgYmFja2dyb3VuZC1wb3NpdGlvbjogLTIwMCUgMDsKICB9Cn0KLnA2M3UwaDEgewogIGJhY2tncm91bmQ6IGxpbmVhci1ncmFkaWVudCg5MGRlZywgdmFyKC0tXzE5ZXoycnpmKSAyNSUsIHZhcigtLV8xOWV6MnJ6ZykgNTAlLCB2YXIoLS1fMTllejJyemYpIDc1JSk7CiAgYmFja2dyb3VuZC1zaXplOiAyMDAlIDEwMCU7Cn0KLnA2M3UwaDIgewogIGJvcmRlci1yYWRpdXM6IDRweDsKfQoucDYzdTBoMyB7CiAgYm9yZGVyLXJhZGl1czogNTAlOwp9Ci5wNjN1MGg0IHsKICBib3JkZXItcmFkaXVzOiA0cHg7CiAgaGVpZ2h0OiAxZW07Cn0KQG1lZGlhIChwcmVmZXJzLXJlZHVjZWQtbW90aW9uOiBuby1wcmVmZXJlbmNlKSB7CiAgLnA2M3UwaDEgewogICAgYW5pbWF0aW9uOiBwNjN1MGgwIDEuNXMgaW5maW5pdGU7CiAgfQp9CkBtZWRpYSAocHJlZmVycy1jb2xvci1zY2hlbWU6IGRhcmspIHsKICAucDYzdTBoMSB7CiAgICBiYWNrZ3JvdW5kOiBsaW5lYXItZ3JhZGllbnQoOTBkZWcsIHZhcigtLV8xOWV6MnJ6ZCkgMjUlLCB2YXIoLS1fMTllejJyemcpIDUwJSwgdmFyKC0tXzE5ZXoycnpkKSA3NSUpOwogICAgYmFja2dyb3VuZC1zaXplOiAyMDAlIDEwMCU7CiAgfQp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FSkeleton_css_ts_vanilla_css_22_2C_22source_22_3A_22QGtleWZyYW1lcyBwNjN1MGgwIHsKICAwJSB7CiAgICBiYWNrZ3JvdW5kLXBvc2l0aW9uOiAyMDAlIDA7CiAgfQogIDEwMCUgewogICAgYmFja2dyb3VuZC1wb3NpdGlvbjogLTIwMCUgMDsKICB9Cn0KLnA2M3UwaDEgewogIGJhY2tncm91bmQ6IGxpbmVhci1ncmFkaWVudCg5MGRlZywgdmFyKC0tXzE5ZXoycnpmKSAyNSUsIHZhcigtLV8xOWV6MnJ6ZykgNTAlLCB2YXIoLS1fMTllejJyemYpIDc1JSk7CiAgYmFja2dyb3VuZC1zaXplOiAyMDAlIDEwMCU7Cn0KLnA2M3UwaDIgewogIGJvcmRlci1yYWRpdXM6IDRweDsKfQoucDYzdTBoMyB7CiAgYm9yZGVyLXJhZGl1czogNTAlOwp9Ci5wNjN1MGg0IHsKICBib3JkZXItcmFkaXVzOiA0cHg7CiAgaGVpZ2h0OiAxZW07Cn0KQG1lZGlhIChwcmVmZXJzLXJlZHVjZWQtbW90aW9uOiBuby1wcmVmZXJlbmNlKSB7CiAgLnA2M3UwaDEgewogICAgYW5pbWF0aW9uOiBwNjN1MGgwIDEuNXMgaW5maW5pdGU7CiAgfQp9CkBtZWRpYSAocHJlZmVycy1jb2xvci1zY2hlbWU6IGRhcmspIHsKICAucDYzdTBoMSB7CiAgICBiYWNrZ3JvdW5kOiBsaW5lYXItZ3JhZGllbnQoOTBkZWcsIHZhcigtLV8xOWV6MnJ6ZCkgMjUlLCB2YXIoLS1fMTllejJyemcpIDUwJSwgdmFyKC0tXzE5ZXoycnpkKSA3NSUpOwogICAgYmFja2dyb3VuZC1zaXplOiAyMDAlIDEwMCU7CiAgfQp9_22_7D = __webpack_require__(46377);
;// ./src/components/ui/Skeleton.css.ts


var circular = 'p63u0h3';
var rectangular = 'p63u0h2';
var skeleton = 'p63u0h1';
var Skeleton_css_text = 'p63u0h4';
;// ./src/components/ui/Skeleton.tsx


function Skeleton(param) {
    let { className = "", width, height, variant = "rectangular", style: customStyle } = param;
    const style = {
        ...customStyle
    };
    if (width) {
        style.width = typeof width === "number" ? "".concat(width, "px") : width;
    }
    if (height) {
        style.height = typeof height === "number" ? "".concat(height, "px") : height;
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: "".concat(skeleton, " ").concat({
            rectangular: rectangular,
            circular: circular,
            text: Skeleton_css_text
        }[variant], " ").concat(className),
        style: style
    });
}


/***/ }),

/***/ 97900:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   F0: () => (/* binding */ fmtCost),
/* harmony export */   L0: () => (/* binding */ computeCacheHitRate),
/* harmony export */   jd: () => (/* binding */ roleDisplayLabel),
/* harmony export */   m_: () => (/* binding */ shortId),
/* harmony export */   pT: () => (/* binding */ fmtPct),
/* harmony export */   qw: () => (/* binding */ fmtDate),
/* harmony export */   uk: () => (/* binding */ pathBasename),
/* harmony export */   yj: () => (/* binding */ fmtTokens)
/* harmony export */ });
// +feature: insights-dashboard
// Shared formatting helpers for all insights components.
/** Format a USD cost with adaptive decimal precision. */ function fmtCost(usd) {
    if (usd < 0.01) return "$".concat(usd.toFixed(4));
    if (usd < 1) return "$".concat(usd.toFixed(3));
    return "$".concat(usd.toFixed(2));
}
/** Format a token count with M/K abbreviations. */ function fmtTokens(n) {
    const num = Number(n);
    if (num >= 1000000) return "".concat((num / 1000000).toFixed(1), "M");
    if (num >= 1000) return "".concat((num / 1000).toFixed(1), "K");
    return num.toString();
}
/** Format a cache-hit rate (0–1) as a percentage with 1 decimal place. */ function fmtPct(rate) {
    return "".concat((rate * 100).toFixed(1), "%");
}
/** Cache-hit rate: cacheRead / (input + cacheRead), 0-guarded. */ function computeCacheHitRate(input, cacheRead) {
    const denom = input + cacheRead;
    return denom === 0 ? 0 : cacheRead / denom;
}
/** Format a protobuf Timestamp as a short human-readable date. */ function fmtDate(ts) {
    if (!ts) return "—";
    return new Date(Number(ts.seconds) * 1000).toLocaleDateString(undefined, {
        month: "short",
        day: "numeric",
        year: "numeric"
    });
}
/** Return the first 8 characters of an ID followed by an ellipsis. */ function shortId(id) {
    return id.length > 8 ? id.slice(0, 8) + "…" : id;
}
/**
 * Display text for a SessionRole/session_role value ("" for a stapler-squad
 * session with no backlog attribution). The raw "" must stay "" everywhere it's
 * used as data (filter state, StageCostChart's dataKey, SessionTokenSummary
 * comparisons) — this is for text shown to a person only.
 */ function roleDisplayLabel(role) {
    return role || "unattributed";
}
// Session worktree dirs are `<title>_<16-hex id>`; the transcript's project path
// decoding turns every "_" and "-" into "/", so the id is the segment after the title.
const WORKTREE_PATH = /\/worktrees\/(.+?)\/[0-9a-f]{16}(?:\/|$)/;
/**
 * Return a project path's display name: the session title for a stapler-squad
 * worktree path (whose last segment is just the opaque id), else its final path
 * segment. The title is approximate: original "-", "_" and "." are indistinguishable.
 */ function pathBasename(p) {
    var _WORKTREE_PATH_exec;
    const title = (_WORKTREE_PATH_exec = WORKTREE_PATH.exec(p)) === null || _WORKTREE_PATH_exec === void 0 ? void 0 : _WORKTREE_PATH_exec[1];
    return title ? title.replace(/\//g, "-") : p.split("/").pop() || p;
}


/***/ })

}]);
//# sourceMappingURL=6833-942d82cc3a0769b5.js.map