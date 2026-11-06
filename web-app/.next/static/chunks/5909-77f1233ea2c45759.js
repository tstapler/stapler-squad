(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[5909],{

/***/ 2663:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   X: () => (/* binding */ useAbortableRequest)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* __next_internal_client_entry_do_not_use__ useAbortableRequest auto */ 
/**
 * Returns a `start()` function that hands back an AbortSignal for the next
 * request, automatically aborting whatever request this hook last started
 * (on the next call) and on unmount.
 *
 * A hook/component tied to a fast-changing key (session id, file path, ...)
 * that fires a request per render/effect without cancellation leaves every
 * prior request's promise — and everything it closes over — alive until it
 * resolves or times out. That's what happened in useSessionVcs: switching
 * sessions rapidly with no cancellation took the tab from ~44MB to ~154MB of
 * JS heap over 138 switches (measured 2026-09-04), each abandoned request
 * living until its own `deadline_exceeded`.
 *
 * Usage — one instance per independent request stream (a hook fetching two
 * unrelated things, like status and diff, uses two):
 *
 *   const startStatus = useAbortableRequest();
 *   const fetchStatus = useCallback(async () => {
 *     const signal = startStatus();
 *     try {
 *       const res = await client.getVCSStatus({ id }, { signal });
 *       if (signal.aborted) return;
 *       setStatus(res.vcsStatus);
 *     } catch (err) {
 *       if (signal.aborted) return;
 *       setError(err);
 *     } finally {
 *       if (!signal.aborted) setLoading(false);
 *     }
 *   }, [id, startStatus]);
 *
 * Every state update after the await must still be guarded by
 * `signal.aborted` — this hook only owns the controller's lifecycle, not
 * your component's state. The require-abort-signal ESLint rule
 * (web-app/eslint-plugin-rpc-lifecycle) enforces that every RPC client call
 * inside lib/hooks/** and lib/contexts/** passes a `{ signal }` at all.
 */ function useAbortableRequest() {
    const controllerRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    const start = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>{
        var _controllerRef_current;
        (_controllerRef_current = controllerRef.current) === null || _controllerRef_current === void 0 ? void 0 : _controllerRef_current.abort();
        const controller = new AbortController();
        controllerRef.current = controller;
        return controller.signal;
    }, []);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        return ()=>{
            var _controllerRef_current;
            return (_controllerRef_current = controllerRef.current) === null || _controllerRef_current === void 0 ? void 0 : _controllerRef_current.abort();
        };
    }, []);
    return start;
}


/***/ }),

/***/ 3339:
/***/ (() => {

// extracted by mini-css-extract-plugin

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

/***/ 8570:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   B5: () => (/* binding */ mapBacklogItem),
/* harmony export */   Kz: () => (/* binding */ GitHubAuthError),
/* harmony export */   Z3: () => (/* binding */ useBacklogSessionIndex),
/* harmony export */   gG: () => (/* binding */ useBacklogService)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(66144);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(21694);
/* harmony import */ var _bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(28765);
/* harmony import */ var _lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__ = __webpack_require__(8854);
/* harmony import */ var _lib_hooks_useAbortableEffect__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(64343);
/* harmony import */ var _gen_session_v1_backlog_pb__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(79870);
/* __next_internal_client_entry_do_not_use__ mapBacklogItem,GitHubAuthError,useBacklogService,useBacklogSessionIndex auto */ 






/** Mirrors the server's maxKnownRepoPaths (server/services/backlog_service_intent.go); the server is authoritative. */ const MAX_KNOWN_REPO_PATHS = 20;
// ---------------------------------------------------------------------------
// Proto ↔ domain mapping helpers
// ---------------------------------------------------------------------------
function toItemDependency(d) {
    return {
        itemId: d.itemId,
        publicId: d.publicId,
        title: d.title,
        status: d.status
    };
}
function mapAcCriterion(c) {
    return {
        index: c.index,
        text: c.text,
        status: c.status || "pending"
    };
}
function mapItemSession(s) {
    var _s_commitCountSinceSpawn, _s_estimatedCostUsd, _s_pipelineModeSnapshot, _s_pipelineModeSnapshotHash, _s_executorSnapshotHash;
    const session = {
        entityId: s.id,
        sessionId: s.sessionUuid,
        role: s.sessionRole,
        startedAt: s.startedAt ? new Date(Number(s.startedAt.seconds) * 1000).toISOString() : undefined,
        endedAt: s.endedAt ? new Date(Number(s.endedAt.seconds) * 1000).toISOString() : undefined,
        commitCountSinceSpawn: (_s_commitCountSinceSpawn = s.commitCountSinceSpawn) !== null && _s_commitCountSinceSpawn !== void 0 ? _s_commitCountSinceSpawn : 0,
        lastCommitAt: s.lastCommitAt ? (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_4__/* .timestampDate */ .FP)(s.lastCommitAt).toISOString() : undefined,
        lastCommitMessage: s.lastCommitMessage || undefined,
        lastFileTouchAt: s.lastFileTouchAt ? (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_4__/* .timestampDate */ .FP)(s.lastFileTouchAt).toISOString() : undefined,
        estimatedCostUsd: (_s_estimatedCostUsd = s.estimatedCostUsd) !== null && _s_estimatedCostUsd !== void 0 ? _s_estimatedCostUsd : 0,
        worktreeBranch: s.worktreeBranch || undefined,
        worktreePath: s.worktreePath || undefined,
        baseCommitSha: s.baseCommitSha || undefined,
        pipelineModeSnapshot: (_s_pipelineModeSnapshot = s.pipelineModeSnapshot) !== null && _s_pipelineModeSnapshot !== void 0 ? _s_pipelineModeSnapshot : "",
        pipelineModeSnapshotHash: (_s_pipelineModeSnapshotHash = s.pipelineModeSnapshotHash) !== null && _s_pipelineModeSnapshotHash !== void 0 ? _s_pipelineModeSnapshotHash : "",
        endReason: s.endReason || undefined,
        failureCapturePath: s.failureCapturePath || undefined,
        errorDetail: s.errorDetail || undefined,
        resolvedProgram: s.resolvedProgram || undefined,
        resolvedModel: s.resolvedModel || undefined,
        executorSnapshotHash: (_s_executorSnapshotHash = s.executorSnapshotHash) !== null && _s_executorSnapshotHash !== void 0 ? _s_executorSnapshotHash : "",
        configuredProgram: s.configuredProgram || undefined,
        executorFallbackReason: s.executorFallbackReason || undefined
    };
    // Map review verdict if present
    if (s.reviewVerdict) {
        const rv = s.reviewVerdict;
        const knownOutcomes = new Set([
            "PASS",
            "FAIL",
            "PARTIAL",
            "UNVERIFIABLE"
        ]);
        var _rv_perCriterion;
        session.reviewVerdict = {
            overallOutcome: knownOutcomes.has(rv.overallOutcome) ? rv.overallOutcome : rv.overallOutcome ? "PARTIAL" : "PENDING",
            summary: rv.summary,
            perCriterion: ((_rv_perCriterion = rv.perCriterion) !== null && _rv_perCriterion !== void 0 ? _rv_perCriterion : []).map((c)=>({
                    criterionIndex: c.criterionIndex,
                    outcome: c.outcome,
                    evidence: c.evidence
                }))
        };
    }
    // Map triage result if present
    if (s.triageResult) {
        const tr = s.triageResult;
        var _tr_suggestions, _tr_clarifyingQuestions, _tr_tasks;
        session.triageResult = {
            summary: tr.summary,
            suggestions: ((_tr_suggestions = tr.suggestions) !== null && _tr_suggestions !== void 0 ? _tr_suggestions : []).map((sg)=>({
                    text: sg.text,
                    rationale: sg.rationale
                })),
            clarifyingQuestions: (_tr_clarifyingQuestions = tr.clarifyingQuestions) !== null && _tr_clarifyingQuestions !== void 0 ? _tr_clarifyingQuestions : [],
            tasks: ((_tr_tasks = tr.tasks) !== null && _tr_tasks !== void 0 ? _tr_tasks : []).map((t)=>({
                    text: t.text,
                    estimate: t.estimate,
                    category: t.category
                })),
            iteration: tr.iteration,
            feedback: tr.feedback
        };
    }
    return session;
}
function mapStatusEvent(e) {
    return {
        id: e.id,
        fromStatus: e.fromStatus,
        toStatus: e.toStatus,
        triggeredBy: e.triggeredBy,
        createdAt: e.createdAt ? new Date(Number(e.createdAt.seconds) * 1000).toISOString() : undefined,
        note: e.note
    };
}
function mapProgressNote(n) {
    return {
        id: n.id,
        criterionIndex: n.criterionIndex,
        note: n.note,
        status: n.status,
        createdAt: n.createdAt ? new Date(Number(n.createdAt.seconds) * 1000).toISOString() : undefined
    };
}
function mapActivityNote(n) {
    return {
        id: n.id,
        message: n.message,
        authorSessionUuid: n.authorSessionUuid,
        authorSessionTitle: n.authorSessionTitle,
        createdAt: n.createdAt ? new Date(Number(n.createdAt.seconds) * 1000).toISOString() : undefined
    };
}
/** Strips the protobuf Message<> wrapper down to the plain {program, model} shape PipelineModeForm consumes. */ function mapStageExecutors(raw) {
    const result = {};
    if (!raw) return result;
    for (const [role, executor] of Object.entries(raw)){
        result[role] = {
            program: executor.program,
            model: executor.model
        };
    }
    return result;
}
function mapPipelineMode(p) {
    return {
        id: p.id,
        slug: p.slug,
        name: p.name,
        description: p.description,
        enabled: p.enabled,
        statusCommandTemplate: p.statusCommandTemplate,
        doneCommandTemplate: p.doneCommandTemplate,
        failCommandTemplate: p.failCommandTemplate,
        reviewCommandTemplate: p.reviewCommandTemplate,
        shipCommandTemplate: p.shipCommandTemplate,
        helpCommandTemplate: p.helpCommandTemplate,
        triagePromptTemplate: p.triagePromptTemplate,
        reviewPromptTemplate: p.reviewPromptTemplate,
        initialPromptTemplate: p.initialPromptTemplate,
        contentHash: p.contentHash,
        stageExecutors: mapStageExecutors(p.stageExecutors),
        stageExecutorHashes: {
            ...p.stageExecutorHashes
        }
    };
}
// Exported so other real-time consumers (useWatchBacklogItems) can convert
// the raw proto BacklogItem their stream/store deals in to this file's mapped
// domain BacklogItem — the shape BacklogItemCard/BacklogBoard/BacklogItemDetail
// actually render (acCriteria, gateVerdict, triageStatus, ISO date strings,
// etc., none of which exist on the raw proto message). Also exported for
// direct unit testing of triageStatus derivation — see
// useBacklogService.test.ts.
function mapBacklogItem(p) {
    var _mostRecentReviewSession_reviewVerdict;
    var _p_itemSessions;
    const linkedSessions = ((_p_itemSessions = p.itemSessions) !== null && _p_itemSessions !== void 0 ? _p_itemSessions : []).map(mapItemSession);
    // Extract gate verdict from the most recent session (for review status)
    let gateVerdict;
    let gateVerdictSummary;
    let gateCriteria;
    // Use the most recent review session's verdict — not the most recent session of any
    // role, which could be a work session (no verdict) after a reopen-for-revision cycle.
    const mostRecentReviewSession = linkedSessions.filter((s)=>s.role === "review").at(-1);
    if (mostRecentReviewSession === null || mostRecentReviewSession === void 0 ? void 0 : (_mostRecentReviewSession_reviewVerdict = mostRecentReviewSession.reviewVerdict) === null || _mostRecentReviewSession_reviewVerdict === void 0 ? void 0 : _mostRecentReviewSession_reviewVerdict.overallOutcome) {
        var _mostRecentReviewSession_reviewVerdict_perCriterion;
        gateVerdict = mostRecentReviewSession.reviewVerdict.overallOutcome;
        gateVerdictSummary = mostRecentReviewSession.reviewVerdict.summary;
        if ((_mostRecentReviewSession_reviewVerdict_perCriterion = mostRecentReviewSession.reviewVerdict.perCriterion) === null || _mostRecentReviewSession_reviewVerdict_perCriterion === void 0 ? void 0 : _mostRecentReviewSession_reviewVerdict_perCriterion.length) {
            gateCriteria = mostRecentReviewSession.reviewVerdict.perCriterion.map((c)=>({
                    label: c.evidence ? "".concat(c.outcome, ": ").concat(c.evidence) : "Criterion ".concat(c.criterionIndex, ": ").concat(c.outcome),
                    passed: c.outcome === "PASS" || c.outcome === "pass"
                }));
        }
    }
    // Derive triageStatus from linked sessions.
    // P12 fix: only mark "completed" if the session ended AND has a non-empty summary.
    // Orphan detection: a triage session without endedAt is only "running" while the item
    // is in "idea" status. If the item has advanced (ready, in_progress, etc.) the session
    // died without cleanly recording its end — treat it as "failed" so the UI doesn't show
    // a loading indicator for a session that no longer exists.
    const itemStatus = p.status || "idea";
    let triageStatus;
    const triageSession = linkedSessions.filter((s)=>s.role === "triage").at(-1);
    if (triageSession) {
        if (triageSession.endedAt) {
            var _triageSession_triageResult;
            triageStatus = ((_triageSession_triageResult = triageSession.triageResult) === null || _triageSession_triageResult === void 0 ? void 0 : _triageSession_triageResult.summary) ? "completed" : "failed";
        } else if (itemStatus === "idea") {
            triageStatus = "running";
        } else {
            triageStatus = "failed";
        }
    }
    const triageResult = triageSession === null || triageSession === void 0 ? void 0 : triageSession.triageResult;
    var _p_acceptanceCriteria, _p_statusEvents, _p_progressNotes, _p_activityNotes, _p_totalEstimatedCostUsd, _p_blockedBy, _p_blocking, _p_labels, _p_allowedTransitions;
    return {
        id: p.id,
        publicId: p.publicId || undefined,
        title: p.title,
        description: p.description || undefined,
        status: p.status || "idea",
        priority: p.priority || 3,
        repoPath: p.repoPath || undefined,
        skipPlanning: p.skipPlanning,
        skipReviewGate: p.skipReviewGate,
        autoSpawnSession: p.autoSpawnSession,
        autoCreatePR: p.autoCreatePr,
        autoApprovePlan: p.autoApprovePlan,
        planApproved: p.planApproved,
        planArtifactsPath: p.planArtifactsPath || undefined,
        planRejectionReason: p.planRejectionReason || undefined,
        // timestampDate, not a hand-rolled `Number(seconds) * 1000` — see the
        // createdAt/updatedAt comment above for why.
        planRejectedAt: p.planRejectedAt ? (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_4__/* .timestampDate */ .FP)(p.planRejectedAt).toISOString() : undefined,
        acCriteria: ((_p_acceptanceCriteria = p.acceptanceCriteria) !== null && _p_acceptanceCriteria !== void 0 ? _p_acceptanceCriteria : []).map(mapAcCriterion),
        linkedSessions,
        notes: p.notes || undefined,
        // timestampDate (not a hand-rolled `Number(seconds) * 1000`) — the
        // previous conversion silently dropped the sub-second `nanos` field
        // entirely, truncating to the whole second. Still only millisecond
        // precision (a JS Date's ceiling) for display purposes — updatedAtRaw
        // below is the lossless form used for exact-equality CAS checks.
        createdAt: p.createdAt ? (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_4__/* .timestampDate */ .FP)(p.createdAt).toISOString() : undefined,
        updatedAt: p.updatedAt ? (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_4__/* .timestampDate */ .FP)(p.updatedAt).toISOString() : undefined,
        updatedAtRaw: p.updatedAt,
        gateVerdict,
        gateVerdictSummary,
        gateCriteria,
        triageStatus,
        triageResult,
        statusEvents: ((_p_statusEvents = p.statusEvents) !== null && _p_statusEvents !== void 0 ? _p_statusEvents : []).map(mapStatusEvent),
        progressNotes: ((_p_progressNotes = p.progressNotes) !== null && _p_progressNotes !== void 0 ? _p_progressNotes : []).map(mapProgressNote),
        activityNotes: ((_p_activityNotes = p.activityNotes) !== null && _p_activityNotes !== void 0 ? _p_activityNotes : []).map(mapActivityNote),
        totalEstimatedCostUsd: (_p_totalEstimatedCostUsd = p.totalEstimatedCostUsd) !== null && _p_totalEstimatedCostUsd !== void 0 ? _p_totalEstimatedCostUsd : 0,
        prUrl: p.prUrl || undefined,
        prNumber: p.prNumber || undefined,
        pipelineMode: p.pipelineMode || undefined,
        category: p.category || undefined,
        reworkCapOverride: p.reworkCapOverride,
        costBudgetThresholdUsd: p.costBudgetThresholdUsd,
        duplicatePending: p.duplicatePending,
        duplicateRef: p.duplicateRef,
        blockedBy: ((_p_blockedBy = p.blockedBy) !== null && _p_blockedBy !== void 0 ? _p_blockedBy : []).map(toItemDependency),
        blocking: ((_p_blocking = p.blocking) !== null && _p_blocking !== void 0 ? _p_blocking : []).map(toItemDependency),
        externalId: p.externalId || undefined,
        pluginId: p.pluginId || undefined,
        hasPendingGuidanceRequests: p.hasPendingGuidanceRequests || undefined,
        externalUrl: p.externalUrl || undefined,
        labels: (_p_labels = p.labels) !== null && _p_labels !== void 0 ? _p_labels : [],
        allowedTransitions: (_p_allowedTransitions = p.allowedTransitions) !== null && _p_allowedTransitions !== void 0 ? _p_allowedTransitions : []
    };
}
function toProtoAcCriteria(criteria) {
    return criteria.map((c)=>({
            $typeName: "session.v1.AcCriterion",
            index: c.index,
            text: c.text,
            status: c.status
        }));
}
// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------
// ---------------------------------------------------------------------------
// GitHub picker domain types
// ---------------------------------------------------------------------------
function toClaimedElsewhere(claim) {
    return {
        externalUrl: claim.externalUrl,
        claimingHostId: claim.claimingHostId,
        itemDeepLink: claim.itemDeepLink,
        disputed: claim.disputed
    };
}
class GitHubAuthError extends Error {
    constructor(){
        super("No GitHub token configured");
        this.name = "GitHubAuthError";
    }
}
function useBacklogService() {
    const clientRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    const [lastError, setLastError] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(null);
    const clearError = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>setLastError(null), []);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        clientRef.current = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_5__/* .createClient */ .UU)(_gen_session_v1_backlog_pb__WEBPACK_IMPORTED_MODULE_3__/* .BacklogService */ .jSM, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_1__/* .getConnectTransport */ .qW)());
    }, []);
    const listBacklogItems = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (filter)=>{
        if (!clientRef.current) return [];
        try {
            var _filter_statuses, _filter_priorities, _filter_includeTerminal, _filter_includeArchived;
            const resp = await clientRef.current.listBacklogItems({
                status: (_filter_statuses = filter === null || filter === void 0 ? void 0 : filter.statuses) !== null && _filter_statuses !== void 0 ? _filter_statuses : [],
                priority: (_filter_priorities = filter === null || filter === void 0 ? void 0 : filter.priorities) !== null && _filter_priorities !== void 0 ? _filter_priorities : [],
                includeTerminal: (_filter_includeTerminal = filter === null || filter === void 0 ? void 0 : filter.includeTerminal) !== null && _filter_includeTerminal !== void 0 ? _filter_includeTerminal : false,
                includeArchived: (_filter_includeArchived = filter === null || filter === void 0 ? void 0 : filter.includeArchived) !== null && _filter_includeArchived !== void 0 ? _filter_includeArchived : false,
                sortBy: ""
            });
            var _resp_items;
            const items = ((_resp_items = resp.items) !== null && _resp_items !== void 0 ? _resp_items : []).map(mapBacklogItem);
            if (filter === null || filter === void 0 ? void 0 : filter.search) {
                const q = filter.search.toLowerCase();
                return items.filter((item)=>{
                    var _item_description;
                    return item.title.toLowerCase().includes(q) || ((_item_description = item.description) === null || _item_description === void 0 ? void 0 : _item_description.toLowerCase().includes(q));
                });
            }
            return items;
        } catch (err) {
            console.error("[useBacklogService] listBacklogItems:", err);
            return [];
        }
    }, []);
    const getBacklogItem = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id)=>{
        if (!clientRef.current) return null;
        try {
            const resp = await clientRef.current.getBacklogItem({
                itemId: id
            });
            return resp.item ? mapBacklogItem(resp.item) : null;
        } catch (err) {
            console.error("[useBacklogService] getBacklogItem:", err);
            return null;
        }
    }, []);
    const createBacklogItem = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (data)=>{
        if (!clientRef.current) return null;
        try {
            setLastError(null);
            var _data_description, _data_repoPath, _data_priority, _data_skipPlanning, _data_skipReviewGate, _data_autoSpawnSession, _data_autoCreatePR, _data_autoApprovePlan, _data_acCriteria, _data_notes, _data_skipTriage, _data_pipelineMode, _data_category;
            const resp = await clientRef.current.createBacklogItem({
                title: data.title,
                description: (_data_description = data.description) !== null && _data_description !== void 0 ? _data_description : "",
                repoPath: (_data_repoPath = data.repoPath) !== null && _data_repoPath !== void 0 ? _data_repoPath : "",
                priority: (_data_priority = data.priority) !== null && _data_priority !== void 0 ? _data_priority : 3,
                skipPlanning: (_data_skipPlanning = data.skipPlanning) !== null && _data_skipPlanning !== void 0 ? _data_skipPlanning : false,
                skipReviewGate: (_data_skipReviewGate = data.skipReviewGate) !== null && _data_skipReviewGate !== void 0 ? _data_skipReviewGate : false,
                autoSpawnSession: (_data_autoSpawnSession = data.autoSpawnSession) !== null && _data_autoSpawnSession !== void 0 ? _data_autoSpawnSession : false,
                autoCreatePr: (_data_autoCreatePR = data.autoCreatePR) !== null && _data_autoCreatePR !== void 0 ? _data_autoCreatePR : false,
                autoApprovePlan: (_data_autoApprovePlan = data.autoApprovePlan) !== null && _data_autoApprovePlan !== void 0 ? _data_autoApprovePlan : false,
                acceptanceCriteria: toProtoAcCriteria((_data_acCriteria = data.acCriteria) !== null && _data_acCriteria !== void 0 ? _data_acCriteria : []),
                notes: (_data_notes = data.notes) !== null && _data_notes !== void 0 ? _data_notes : "",
                skipTriage: (_data_skipTriage = data.skipTriage) !== null && _data_skipTriage !== void 0 ? _data_skipTriage : false,
                pipelineMode: (_data_pipelineMode = data.pipelineMode) !== null && _data_pipelineMode !== void 0 ? _data_pipelineMode : "",
                category: (_data_category = data.category) !== null && _data_category !== void 0 ? _data_category : ""
            });
            return resp.item ? {
                item: mapBacklogItem(resp.item),
                triageTriggered: resp.triageTriggered
            } : null;
        } catch (err) {
            console.error("[useBacklogService] createBacklogItem:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to create backlog item.")));
            return null;
        }
    }, []);
    const createBacklogItemFromChat = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (message, existingItemId)=>{
        if (!clientRef.current) return null;
        try {
            setLastError(null);
            const resp = await clientRef.current.createBacklogItemFromChat({
                message,
                existingItemId: existingItemId !== null && existingItemId !== void 0 ? existingItemId : ""
            });
            return resp.item ? {
                item: mapBacklogItem(resp.item),
                triageTriggered: resp.triageTriggered
            } : null;
        } catch (err) {
            console.error("[useBacklogService] createBacklogItemFromChat:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to create backlog item from chat.")));
            return null;
        }
    }, []);
    const parseBacklogItemIntent = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(// repo_path is intentionally omitted: the omnibar has no "current repo"
    // context available before parsing.
    async function(message) {
        let knownRepoPaths = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : [];
        if (!clientRef.current) return null;
        try {
            const resp = await clientRef.current.parseBacklogItemIntent({
                message,
                knownRepoPaths: knownRepoPaths.slice(0, MAX_KNOWN_REPO_PATHS)
            });
            if (resp.error || !resp.draft) return null;
            var _resp_draft_suggestedRepoPath;
            return {
                title: resp.draft.title,
                description: resp.draft.description,
                acceptanceCriteria: resp.draft.acceptanceCriteria,
                confidence: resp.draft.confidence,
                suggestedRepoPath: (_resp_draft_suggestedRepoPath = resp.draft.suggestedRepoPath) !== null && _resp_draft_suggestedRepoPath !== void 0 ? _resp_draft_suggestedRepoPath : ""
            };
        } catch (err) {
            console.error("[useBacklogService] parseBacklogItemIntent:", err);
            return null;
        }
    }, []);
    const updateBacklogItem = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id, data)=>{
        if (!clientRef.current) return null;
        try {
            setLastError(null);
            const resp = await clientRef.current.updateBacklogItem({
                itemId: id,
                title: data.title,
                description: data.description,
                repoPath: data.repoPath,
                priority: data.priority,
                skipPlanning: data.skipPlanning,
                skipReviewGate: data.skipReviewGate,
                autoSpawnSession: data.autoSpawnSession,
                autoCreatePr: data.autoCreatePR,
                autoApprovePlan: data.autoApprovePlan,
                acceptanceCriteria: data.acCriteria ? toProtoAcCriteria(data.acCriteria) : undefined,
                notes: data.notes,
                pipelineMode: data.pipelineMode,
                category: data.category,
                reworkCapOverride: data.reworkCapOverride,
                costBudgetThresholdUsd: data.costBudgetThresholdUsd,
                prUrl: data.prUrl,
                prNumber: data.prNumber
            });
            return resp.item ? mapBacklogItem(resp.item) : null;
        } catch (err) {
            console.error("[useBacklogService] updateBacklogItem:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to update backlog item.")));
            return null;
        }
    }, []);
    const archiveBacklogItem = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id)=>{
        if (!clientRef.current) return false;
        try {
            await clientRef.current.archiveBacklogItem({
                itemId: id
            });
            return true;
        } catch (err) {
            console.error("[useBacklogService] archiveBacklogItem:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to archive backlog item.")));
            throw err;
        }
    }, []);
    const unarchiveBacklogItem = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id)=>{
        if (!clientRef.current) return false;
        try {
            await clientRef.current.unarchiveBacklogItem({
                itemId: id
            });
            return true;
        } catch (err) {
            console.error("[useBacklogService] unarchiveBacklogItem:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to unarchive backlog item.")));
            throw err;
        }
    }, []);
    const deleteBacklogItem = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id)=>{
        if (!clientRef.current) return false;
        try {
            await clientRef.current.deleteBacklogItem({
                itemId: id
            });
            return true;
        } catch (err) {
            console.error("[useBacklogService] deleteBacklogItem:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to delete backlog item.")));
            throw err;
        }
    }, []);
    const transitionStatus = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id, toStatus, options)=>{
        if (!clientRef.current) return null;
        try {
            setLastError(null);
            var _options_expectedStatus, _options_overrideReason;
            const resp = await clientRef.current.transitionBacklogItemStatus({
                itemId: id,
                targetStatus: toStatus,
                expectedStatus: (_options_expectedStatus = options === null || options === void 0 ? void 0 : options.expectedStatus) !== null && _options_expectedStatus !== void 0 ? _options_expectedStatus : "",
                // Passed through verbatim — no Date round-trip. See the
                // expectedUpdatedAt doc comment above for why that matters.
                expectedUpdatedAt: options === null || options === void 0 ? void 0 : options.expectedUpdatedAt,
                overrideReason: (_options_overrideReason = options === null || options === void 0 ? void 0 : options.overrideReason) !== null && _options_overrideReason !== void 0 ? _options_overrideReason : ""
            });
            return resp.item ? mapBacklogItem(resp.item) : null;
        } catch (err) {
            console.error("[useBacklogService] transitionStatus:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to transition backlog item status.")));
            throw err;
        }
    }, []);
    const spawnSessionFromItem = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id, options)=>{
        if (!clientRef.current) return null;
        try {
            setLastError(null);
            var _options_autonomous, _options_force;
            const resp = await clientRef.current.spawnSessionFromItem({
                itemId: id,
                autonomous: (_options_autonomous = options === null || options === void 0 ? void 0 : options.autonomous) !== null && _options_autonomous !== void 0 ? _options_autonomous : false,
                force: (_options_force = options === null || options === void 0 ? void 0 : options.force) !== null && _options_force !== void 0 ? _options_force : false
            });
            return {
                sessionUuid: resp.sessionUuid,
                queued: resp.queued
            };
        } catch (err) {
            console.error("[useBacklogService] spawnSessionFromItem:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to spawn session from item.")));
            throw err;
        }
    }, []);
    const triggerTriage = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id, feedback)=>{
        if (!clientRef.current) return null;
        try {
            var _resp_itemSession;
            const resp = await clientRef.current.triggerTriage({
                itemId: id,
                feedback: feedback !== null && feedback !== void 0 ? feedback : ""
            });
            var _resp_itemSession_id;
            return {
                itemSessionId: (_resp_itemSession_id = (_resp_itemSession = resp.itemSession) === null || _resp_itemSession === void 0 ? void 0 : _resp_itemSession.id) !== null && _resp_itemSession_id !== void 0 ? _resp_itemSession_id : ""
            };
        } catch (err) {
            console.error("[useBacklogService] triggerTriage:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to trigger triage.")));
            throw err;
        }
    }, []);
    const cancelTriage = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id)=>{
        if (!clientRef.current) return false;
        try {
            const resp = await clientRef.current.cancelTriage({
                itemId: id
            });
            return resp.cancelled;
        } catch (err) {
            console.error("[useBacklogService] cancelTriage:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to cancel triage.")));
            throw err;
        }
    }, []);
    const approvePlan = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id)=>{
        if (!clientRef.current) return null;
        try {
            const resp = await clientRef.current.approvePlan({
                itemId: id
            });
            return resp.item ? mapBacklogItem(resp.item) : null;
        } catch (err) {
            console.error("[useBacklogService] approvePlan:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to approve plan.")));
            throw err;
        }
    }, []);
    const rejectPlan = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id, reason)=>{
        if (!clientRef.current) return null;
        try {
            const resp = await clientRef.current.rejectPlan({
                itemId: id,
                reason
            });
            return resp.item ? mapBacklogItem(resp.item) : null;
        } catch (err) {
            console.error("[useBacklogService] rejectPlan:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to reject plan.")));
            throw err;
        }
    }, []);
    const overrideVerdict = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id, overrideReason, toStatus)=>{
        if (!clientRef.current) return false;
        try {
            await clientRef.current.overrideVerdict({
                itemSessionId: id,
                overrideReason,
                toStatus: toStatus !== null && toStatus !== void 0 ? toStatus : "done"
            });
            return true;
        } catch (err) {
            console.error("[useBacklogService] overrideVerdict:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to override verdict.")));
            throw err;
        }
    }, []);
    const triggerReReview = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id)=>{
        if (!clientRef.current) return false;
        try {
            await clientRef.current.triggerReReview({
                itemId: id
            });
            return true;
        } catch (err) {
            console.error("[useBacklogService] triggerReReview:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to trigger re-review.")));
            throw err;
        }
    }, []);
    /**
   * Runs the same one-shot PR-creation prompt the opt-in AutoCreatePR policy
   * uses automatically, for an item in review with no PR yet — the
   * self-service "Ship PR" action on the item detail page. Can take a while
   * (the underlying RunOneShot call may run for several minutes), matching
   * ReviewQueuePanel's existing manual "Create PR" flow. Rethrows on failure
   * so the caller can show the specific error (e.g. "work session not
   * running") rather than a generic failure state.
   */ const triggerShipPR = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id)=>{
        if (!clientRef.current) return null;
        try {
            const resp = await clientRef.current.triggerShipPR({
                itemId: id
            });
            return {
                prUrl: resp.prUrl
            };
        } catch (err) {
            console.error("[useBacklogService] triggerShipPR:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to ship PR.")));
            throw err;
        }
    }, []);
    const submitManualReview = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id, overallOutcome, summary)=>{
        if (!clientRef.current) return null;
        try {
            setLastError(null);
            const resp = await clientRef.current.submitManualReview({
                itemId: id,
                overallOutcome,
                summary
            });
            return resp.item ? mapBacklogItem(resp.item) : null;
        } catch (err) {
            console.error("[useBacklogService] submitManualReview:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to submit manual review.")));
            throw err;
        }
    }, []);
    const listPipelineModes = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async ()=>{
        if (!clientRef.current) return [];
        try {
            const resp = await clientRef.current.listPipelineModes({});
            var _resp_items;
            return ((_resp_items = resp.items) !== null && _resp_items !== void 0 ? _resp_items : []).map(mapPipelineMode);
        } catch (err) {
            console.error("[useBacklogService] listPipelineModes:", err);
            throw err;
        }
    }, []);
    const getPipelineMode = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (slug)=>{
        if (!clientRef.current) return null;
        try {
            const resp = await clientRef.current.getPipelineMode({
                slug
            });
            return resp.item ? mapPipelineMode(resp.item) : null;
        } catch (err) {
            console.error("[useBacklogService] getPipelineMode:", err);
            throw err;
        }
    }, []);
    const createPipelineMode = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (data)=>{
        if (!clientRef.current) throw new Error("Backlog service not connected");
        try {
            var _data_description, _data_enabled, _data_statusCommandTemplate, _data_doneCommandTemplate, _data_failCommandTemplate, _data_reviewCommandTemplate, _data_shipCommandTemplate, _data_helpCommandTemplate, _data_triagePromptTemplate, _data_reviewPromptTemplate, _data_initialPromptTemplate, _data_stageExecutors, _data_forceUnknownModel;
            const resp = await clientRef.current.createPipelineMode({
                slug: data.slug,
                name: data.name,
                description: (_data_description = data.description) !== null && _data_description !== void 0 ? _data_description : "",
                enabled: (_data_enabled = data.enabled) !== null && _data_enabled !== void 0 ? _data_enabled : false,
                statusCommandTemplate: (_data_statusCommandTemplate = data.statusCommandTemplate) !== null && _data_statusCommandTemplate !== void 0 ? _data_statusCommandTemplate : "",
                doneCommandTemplate: (_data_doneCommandTemplate = data.doneCommandTemplate) !== null && _data_doneCommandTemplate !== void 0 ? _data_doneCommandTemplate : "",
                failCommandTemplate: (_data_failCommandTemplate = data.failCommandTemplate) !== null && _data_failCommandTemplate !== void 0 ? _data_failCommandTemplate : "",
                reviewCommandTemplate: (_data_reviewCommandTemplate = data.reviewCommandTemplate) !== null && _data_reviewCommandTemplate !== void 0 ? _data_reviewCommandTemplate : "",
                shipCommandTemplate: (_data_shipCommandTemplate = data.shipCommandTemplate) !== null && _data_shipCommandTemplate !== void 0 ? _data_shipCommandTemplate : "",
                helpCommandTemplate: (_data_helpCommandTemplate = data.helpCommandTemplate) !== null && _data_helpCommandTemplate !== void 0 ? _data_helpCommandTemplate : "",
                triagePromptTemplate: (_data_triagePromptTemplate = data.triagePromptTemplate) !== null && _data_triagePromptTemplate !== void 0 ? _data_triagePromptTemplate : "",
                reviewPromptTemplate: (_data_reviewPromptTemplate = data.reviewPromptTemplate) !== null && _data_reviewPromptTemplate !== void 0 ? _data_reviewPromptTemplate : "",
                initialPromptTemplate: (_data_initialPromptTemplate = data.initialPromptTemplate) !== null && _data_initialPromptTemplate !== void 0 ? _data_initialPromptTemplate : "",
                stageExecutors: (_data_stageExecutors = data.stageExecutors) !== null && _data_stageExecutors !== void 0 ? _data_stageExecutors : {},
                forceUnknownModel: (_data_forceUnknownModel = data.forceUnknownModel) !== null && _data_forceUnknownModel !== void 0 ? _data_forceUnknownModel : false
            });
            if (!resp.item) throw new Error("createPipelineMode: server returned no item");
            return mapPipelineMode(resp.item);
        } catch (err) {
            console.error("[useBacklogService] createPipelineMode:", err);
            throw err;
        }
    }, []);
    const updatePipelineMode = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id, data)=>{
        if (!clientRef.current) throw new Error("Backlog service not connected");
        try {
            const resp = await clientRef.current.updatePipelineMode({
                id,
                name: data.name,
                description: data.description,
                enabled: data.enabled,
                statusCommandTemplate: data.statusCommandTemplate,
                doneCommandTemplate: data.doneCommandTemplate,
                failCommandTemplate: data.failCommandTemplate,
                reviewCommandTemplate: data.reviewCommandTemplate,
                shipCommandTemplate: data.shipCommandTemplate,
                helpCommandTemplate: data.helpCommandTemplate,
                triagePromptTemplate: data.triagePromptTemplate,
                reviewPromptTemplate: data.reviewPromptTemplate,
                initialPromptTemplate: data.initialPromptTemplate,
                // undefined = leave stage executors untouched; present (even {}) = replace.
                stageExecutors: data.stageExecutors !== undefined ? {
                    values: data.stageExecutors
                } : undefined,
                forceUnknownModel: data.forceUnknownModel
            });
            if (!resp.item) throw new Error("updatePipelineMode: server returned no item");
            return mapPipelineMode(resp.item);
        } catch (err) {
            console.error("[useBacklogService] updatePipelineMode:", err);
            throw err;
        }
    }, []);
    const deletePipelineMode = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id)=>{
        if (!clientRef.current) return false;
        try {
            await clientRef.current.deletePipelineMode({
                id
            });
            return true;
        } catch (err) {
            console.error("[useBacklogService] deletePipelineMode:", err);
            throw err;
        }
    }, []);
    const checkCrossHostClaim = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (externalUrl, options)=>{
        if (!clientRef.current) return null;
        try {
            var _options_localOnly;
            const resp = await clientRef.current.checkCrossHostClaim({
                externalUrl,
                localOnly: (_options_localOnly = options === null || options === void 0 ? void 0 : options.localOnly) !== null && _options_localOnly !== void 0 ? _options_localOnly : false
            });
            return {
                enabled: resp.enabled,
                checked: resp.checked,
                claim: resp.claim ? toClaimedElsewhere(resp.claim) : undefined,
                claimedAtUnix: Number(resp.claimedAtUnix)
            };
        } catch (err) {
            console.warn("[useBacklogService] checkCrossHostClaim:", err);
            return null;
        }
    }, []);
    const listForeignClaims = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async ()=>{
        if (!clientRef.current) return [];
        try {
            const resp = await clientRef.current.listForeignClaims({});
            return resp.enabled ? resp.claims.map(toClaimedElsewhere) : [];
        } catch (err) {
            console.warn("[useBacklogService] listForeignClaims:", err);
            return [];
        }
    }, []);
    const resolveClaimDispute = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (externalUrl, reason)=>{
        if (!clientRef.current) throw new Error("Backlog service unavailable.");
        try {
            await clientRef.current.resolveClaimDispute({
                externalUrl,
                reason
            });
        } catch (err) {
            throw new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to resolve claim dispute."));
        }
    }, []);
    const importGitHubIssue = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (issueUrl, options)=>{
        if (!clientRef.current) return null;
        try {
            setLastError(null);
            var _options_repoPath, _options_skipPlanning, _options_overrideReason;
            const resp = await clientRef.current.importGitHubIssue({
                issueUrl,
                repoPath: (_options_repoPath = options === null || options === void 0 ? void 0 : options.repoPath) !== null && _options_repoPath !== void 0 ? _options_repoPath : "",
                skipPlanning: (_options_skipPlanning = options === null || options === void 0 ? void 0 : options.skipPlanning) !== null && _options_skipPlanning !== void 0 ? _options_skipPlanning : false,
                override: (options === null || options === void 0 ? void 0 : options.overrideReason) !== undefined,
                overrideReason: (_options_overrideReason = options === null || options === void 0 ? void 0 : options.overrideReason) !== null && _options_overrideReason !== void 0 ? _options_overrideReason : ""
            });
            if (resp.alreadyClaimedElsewhere) {
                const claim = resp.alreadyClaimedElsewhere;
                return {
                    alreadyClaimedElsewhere: {
                        externalUrl: claim.externalUrl,
                        claimingHostId: claim.claimingHostId,
                        itemDeepLink: claim.itemDeepLink,
                        disputed: claim.disputed
                    }
                };
            }
            return resp.item ? {
                item: mapBacklogItem(resp.item),
                triageTriggered: resp.triageTriggered,
                alreadyExisted: resp.alreadyExisted
            } : null;
        } catch (err) {
            console.error("[useBacklogService] importGitHubIssue:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to import GitHub issue.")));
            return null;
        }
    }, []);
    const searchGitHubRepos = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (query, limit)=>{
        if (!clientRef.current) return [];
        try {
            const resp = await clientRef.current.searchGitHubRepos({
                query,
                limit: limit !== null && limit !== void 0 ? limit : 30
            });
            return resp.repos.map((r)=>({
                    owner: r.owner,
                    repo: r.repo,
                    isLocal: r.isLocal,
                    localPath: r.localPath,
                    description: r.description,
                    host: r.host
                }));
        } catch (err) {
            if (err instanceof Error && err.message.toLowerCase().includes("token")) {
                throw new GitHubAuthError();
            }
            throw err;
        }
    }, []);
    const listGitHubIssues = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (owner, repo, options)=>{
        if (!clientRef.current) return [];
        try {
            var _options_state, _options_search, _options_limit, _options_host;
            const resp = await clientRef.current.listGitHubIssues({
                owner,
                repo,
                state: (_options_state = options === null || options === void 0 ? void 0 : options.state) !== null && _options_state !== void 0 ? _options_state : "open",
                search: (_options_search = options === null || options === void 0 ? void 0 : options.search) !== null && _options_search !== void 0 ? _options_search : "",
                limit: (_options_limit = options === null || options === void 0 ? void 0 : options.limit) !== null && _options_limit !== void 0 ? _options_limit : 30,
                host: (_options_host = options === null || options === void 0 ? void 0 : options.host) !== null && _options_host !== void 0 ? _options_host : ""
            });
            return resp.issues.map((i)=>{
                var _i_isPr;
                return {
                    number: i.number,
                    title: i.title,
                    body: i.body,
                    author: i.author,
                    state: i.state,
                    url: i.url,
                    labels: i.labels,
                    createdAt: i.createdAt ? new Date(Number(i.createdAt.seconds) * 1000).toISOString() : undefined,
                    updatedAt: i.updatedAt ? new Date(Number(i.updatedAt.seconds) * 1000).toISOString() : undefined,
                    isPR: (_i_isPr = i.isPr) !== null && _i_isPr !== void 0 ? _i_isPr : false,
                    host: i.host
                };
            });
        } catch (err) {
            if (err instanceof Error && err.message.toLowerCase().includes("token")) {
                throw new GitHubAuthError();
            }
            throw err;
        }
    }, []);
    // Stable object reference: all methods are useCallback(fn,[]) — only lastError changes.
    // Without useMemo, every render creates a new object, making callers' useCallback deps
    // fire on every render and causing infinite reload loops.
    return (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>({
            listBacklogItems,
            getBacklogItem,
            createBacklogItem,
            createBacklogItemFromChat,
            parseBacklogItemIntent,
            importGitHubIssue,
            checkCrossHostClaim,
            listForeignClaims,
            resolveClaimDispute,
            searchGitHubRepos,
            listGitHubIssues,
            updateBacklogItem,
            archiveBacklogItem,
            unarchiveBacklogItem,
            deleteBacklogItem,
            transitionStatus,
            spawnSessionFromItem,
            triggerTriage,
            cancelTriage,
            approvePlan,
            rejectPlan,
            overrideVerdict,
            triggerReReview,
            triggerShipPR,
            submitManualReview,
            listPipelineModes,
            getPipelineMode,
            createPipelineMode,
            updatePipelineMode,
            deletePipelineMode,
            lastError,
            clearError
        }), // eslint-disable-next-line react-hooks/exhaustive-deps
    [
        lastError
    ]);
}
/**
 * Fetches the full session→backlog index once on mount.
 * Returns a stable Map keyed by tmux session UUID.
 */ function useBacklogSessionIndex() {
    const [index, setIndex] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(new Map());
    const [loading, setLoading] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(true);
    (0,_lib_hooks_useAbortableEffect__WEBPACK_IMPORTED_MODULE_2__/* .useAbortableEffect */ .B)(async (signal)=>{
        const client = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_5__/* .createClient */ .UU)(_gen_session_v1_backlog_pb__WEBPACK_IMPORTED_MODULE_3__/* .BacklogService */ .jSM, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_1__/* .getConnectTransport */ .qW)());
        try {
            const resp = await client.getSessionBacklogIndex({}, {
                signal
            });
            if (signal.aborted) return;
            // Entries are ordered newest-first (see GetAllItemSessionsWithBacklogInfo's doc
            // comment in ent_repository_backlog.go) — keep only the first entry seen per
            // session UUID so a re-parented session (e.g. triage -> work) resolves to its
            // current role, not a stale one from an earlier row.
            const map = new Map();
            var _resp_entries;
            for (const e of (_resp_entries = resp.entries) !== null && _resp_entries !== void 0 ? _resp_entries : []){
                if (e.sessionUuid && !map.has(e.sessionUuid)) {
                    map.set(e.sessionUuid, {
                        itemId: e.itemId,
                        itemTitle: e.itemTitle,
                        itemStatus: e.itemStatus,
                        sessionRole: e.sessionRole
                    });
                }
            }
            setIndex(map);
        } catch (err) {
            if (signal.aborted) return;
            console.error("[useBacklogSessionIndex] failed:", err);
        } finally{
            if (!signal.aborted) setLoading(false);
        }
    }, []);
    return {
        index,
        loading
    };
}


/***/ }),

/***/ 8854:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   u: () => (/* binding */ getErrorMessage)
/* harmony export */ });
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(45640);

/**
 * Converts a thrown value from a ConnectRPC client call into a human-readable
 * message, using `ConnectError.rawMessage` (no `[code]` prefix) instead of the
 * `[code] message`-formatted `.message`. Non-ConnectError Errors fall back to
 * `.message`; anything else (including an empty `rawMessage`) falls back to
 * the caller-supplied fallback string.
 */ function getErrorMessage(e, fallback) {
    if (e instanceof _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T) return e.rawMessage || fallback;
    if (e instanceof Error) return e.message || fallback;
    return fallback;
}


/***/ }),

/***/ 27080:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 28765:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   FP: () => (/* binding */ timestampDate),
/* harmony export */   LI: () => (/* binding */ timestampFromDate),
/* harmony export */   R5: () => (/* binding */ timestampMs)
/* harmony export */ });
/* unused harmony exports timestampNow, timestampFromMs */
/* harmony import */ var _gen_google_protobuf_timestamp_pb_js__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(43963);
/* harmony import */ var _create_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(95444);
/* harmony import */ var _proto_int64_js__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(2952);
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
 * Create a google.protobuf.Timestamp for the current time.
 */
function timestampNow() {
    return timestampFromDate(new Date());
}
/**
 * Create a google.protobuf.Timestamp message from an ECMAScript Date.
 */
function timestampFromDate(date) {
    return timestampFromMs(date.getTime());
}
/**
 * Convert a google.protobuf.Timestamp message to an ECMAScript Date.
 */
function timestampDate(timestamp) {
    return new Date(timestampMs(timestamp));
}
/**
 * Create a google.protobuf.Timestamp message from a Unix timestamp in milliseconds.
 */
function timestampFromMs(timestampMs) {
    const seconds = Math.floor(timestampMs / 1000);
    return (0,_create_js__WEBPACK_IMPORTED_MODULE_0__/* .create */ .v)(_gen_google_protobuf_timestamp_pb_js__WEBPACK_IMPORTED_MODULE_1__/* .TimestampSchema */ .w, {
        seconds: _proto_int64_js__WEBPACK_IMPORTED_MODULE_2__/* .protoInt64 */ .M.parse(seconds),
        nanos: (timestampMs - seconds * 1000) * 1000000,
    });
}
/**
 * Convert a google.protobuf.Timestamp to a Unix timestamp in milliseconds.
 */
function timestampMs(timestamp) {
    return (Number(timestamp.seconds) * 1000 + Math.round(timestamp.nanos / 1000000));
}


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

/***/ 47439:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 64343:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   B: () => (/* binding */ useAbortableEffect)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _lib_hooks_useAbortableRequest__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(2663);
/* __next_internal_client_entry_do_not_use__ useAbortableEffect auto */ 

/**
 * Runs `effect(signal)` on mount and whenever `deps` change, cancelling the
 * previous run first (and on unmount) via useAbortableRequest — the common
 * "declare an AbortController ref, wrap the fetch in useCallback, call it
 * from useEffect" boilerplate collapsed into one call.
 *
 * `effect` still must check `signal.aborted` before every state update
 * after an `await` — this hook only owns cancellation timing, not your
 * component's state (same contract as useAbortableRequest). It intentionally
 * does not manage data/loading/error state itself: hooks in this codebase
 * differ too much in error handling (NotFound stops polling, some errors
 * are non-fatal, some fall back to a cached value) for one generic shape to
 * fit without fighting those differences — see useAbortableRequest's doc
 * comment for the lower-level primitive this builds on.
 *
 * Usage:
 *
 *   useAbortableEffect(async (signal) => {
 *     setLoading(true);
 *     try {
 *       const res = await client.getThing({ id }, { signal });
 *       if (signal.aborted) return;
 *       setData(res.thing);
 *     } catch (err) {
 *       if (signal.aborted) return;
 *       setError(err);
 *     } finally {
 *       if (!signal.aborted) setLoading(false);
 *     }
 *   }, [id]);
 */ function useAbortableEffect(effect, deps) {
    const start = (0,_lib_hooks_useAbortableRequest__WEBPACK_IMPORTED_MODULE_1__/* .useAbortableRequest */ .X)();
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        const signal = start();
        // effect() is documented to handle its own errors; this catch only
        // guards against a caller that doesn't, so a stray rejection doesn't
        // surface as an unhandled promise rejection.
        void effect(signal).catch(()=>{});
    // effect and start are intentionally excluded: callers list their own
    // fetch dependencies in `deps`, same convention as every hook this
    // replaces (e.g. useSessionVcs.ts's fetchStatus effect).
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, deps);
}


/***/ }),

/***/ 65916:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ })

}]);
//# sourceMappingURL=5909-77f1233ea2c45759.js.map