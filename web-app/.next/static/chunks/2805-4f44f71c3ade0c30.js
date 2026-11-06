"use strict";
(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[2805],{

/***/ 8570:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

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

/***/ 62428:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   AI: () => (/* binding */ Severity),
/* harmony export */   H5: () => (/* binding */ GetSessionTurnTimelineRequestSchema),
/* harmony export */   Mr: () => (/* binding */ ActivityType),
/* harmony export */   O5: () => (/* binding */ DismissFindingRequestSchema),
/* harmony export */   VJ: () => (/* binding */ InsightsService),
/* harmony export */   dQ: () => (/* binding */ WatchInsightsRequestSchema),
/* harmony export */   s7: () => (/* binding */ GetInsightsSummaryRequestSchema)
/* harmony export */ });
/* unused harmony exports file_session_v1_insights, WasteFindingSchema, SessionTokenSummarySchema, TopToolEntrySchema, DailyTokenBucketSchema, ModelBreakdownSchema, ActivityCostBreakdownSchema, ItemRoleCostSchema, RoleCostBreakdownSchema, TopEntrySchema, GetInsightsSummaryResponseSchema, ListSessionTokensRequestSchema, ListSessionTokensResponseSchema, InsightsEventSchema, TurnTokenStatSchema, GetSessionTurnTimelineResponseSchema, DismissFindingResponseSchema, ContextSampleSchema, CompactionEventSchema, ContextCeilingSummarySchema, GetContextHistoryRequestSchema, GetContextHistoryResponseSchema, ListSessionsByCeilingTimeRequestSchema, ListSessionsByCeilingTimeResponseSchema, GetCompactionStatsRequestSchema, GetCompactionStatsResponseSchema, SeveritySchema, FindingType, FindingTypeSchema, ActivityTypeSchema */
/* harmony import */ var _bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(1844);
/* harmony import */ var _bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(15033);
/* harmony import */ var _bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(95765);
/* harmony import */ var _bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(43963);
// @generated by protoc-gen-es v2.11.0 with parameter "target=ts,ts_nocheck=false,keep_empty_files=true"
// @generated from file session/v1/insights.proto (package session.v1, syntax proto3)
/* eslint-disable */ 

/**
 * Describes the file session/v1/insights.proto.
 */ const file_session_v1_insights = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_0__/* .fileDesc */ .w)("ChlzZXNzaW9uL3YxL2luc2lnaHRzLnByb3RvEgpzZXNzaW9uLnYxItIBCgxXYXN0ZUZpbmRpbmcSLQoMZmluZGluZ190eXBlGAEgASgOMhcuc2Vzc2lvbi52MS5GaW5kaW5nVHlwZRImCghzZXZlcml0eRgCIAEoDjIULnNlc3Npb24udjEuU2V2ZXJpdHkSGQoRZG9sbGFyX2ltcGFjdF91c2QYAyABKAESEgoKc2Vzc2lvbl9pZBgEIAEoCRIXCg9jb252ZXJzYXRpb25faWQYBSABKAkSDwoHbWVzc2FnZRgGIAEoCRISCgpmaW5kaW5nX2lkGAcgASgJIpEGChNTZXNzaW9uVG9rZW5TdW1tYXJ5EhIKCnNlc3Npb25faWQYASABKAkSFwoPY29udmVyc2F0aW9uX2lkGAIgASgJEhQKDHByb2plY3RfcGF0aBgDIAEoCRIVCg1wcmltYXJ5X21vZGVsGAQgASgJEhoKEnRvdGFsX2lucHV0X3Rva2VucxgFIAEoAxIbChN0b3RhbF9vdXRwdXRfdG9rZW5zGAYgASgDEh0KFWNhY2hlX2NyZWF0aW9uX3Rva2VucxgHIAEoAxIZChFjYWNoZV9yZWFkX3Rva2VucxgIIAEoAxIaChJlc3RpbWF0ZWRfY29zdF91c2QYCSABKAESFgoOY2FjaGVfaGl0X3JhdGUYCiABKAESFQoNbWVzc2FnZV9jb3VudBgLIAEoBRI0ChBmaXJzdF9tZXNzYWdlX2F0GAwgASgLMhouZ29vZ2xlLnByb3RvYnVmLlRpbWVzdGFtcBIzCg9sYXN0X21lc3NhZ2VfYXQYDSABKAsyGi5nb29nbGUucHJvdG9idWYuVGltZXN0YW1wEhEKCWlzX29ycGhhbhgOIAEoCBIZChFza2lsbF9hY3RpdmF0aW9ucxgPIAMoCRIrCgl0b3BfdG9vbHMYECADKAsyGC5zZXNzaW9uLnYxLlRvcFRvb2xFbnRyeRIXCg91bnByaWNlZF9tb2RlbHMYESADKAkSFQoNY2FjaGVfcm9pX3VzZBgSIAEoARIYCgt3YXN0ZV9zY29yZRgTIAEoAUgAiAEBEi8KDWFjdGl2aXR5X3R5cGUYFCABKA4yGC5zZXNzaW9uLnYxLkFjdGl2aXR5VHlwZRIMCgR0YWdzGBUgAygJEhQKDHNlc3Npb25fcm9sZRgWIAEoCRIWCg5pbnB1dF9jb3N0X3VzZBgXIAEoARIXCg9vdXRwdXRfY29zdF91c2QYGCABKAESHwoXY2FjaGVfY3JlYXRpb25fY29zdF91c2QYGSABKAESGwoTY2FjaGVfcmVhZF9jb3N0X3VzZBgaIAEoAUIOCgxfd2FzdGVfc2NvcmUikQEKDFRvcFRvb2xFbnRyeRIRCgl0b29sX25hbWUYASABKAkSEgoKY2FsbF9jb3VudBgCIAEoBRISCgptY3Bfc2VydmVyGAMgASgJEhAKCGNvc3RfdXNkGAQgASgBEh0KFWNvc3RfbWF5X2RvdWJsZV9jb3VudBgFIAEoCBIVCg1jb3N0X3VucHJpY2VkGAYgASgIItYDChBEYWlseVRva2VuQnVja2V0EigKBGRhdGUYASABKAsyGi5nb29nbGUucHJvdG9idWYuVGltZXN0YW1wEhoKEnRvdGFsX2lucHV0X3Rva2VucxgCIAEoAxIbChN0b3RhbF9vdXRwdXRfdG9rZW5zGAMgASgDEhkKEWNhY2hlX3JlYWRfdG9rZW5zGAQgASgDEhoKEmVzdGltYXRlZF9jb3N0X3VzZBgFIAEoARIVCg1zZXNzaW9uX2NvdW50GAYgASgFEkQKDWNvc3RfYnlfbW9kZWwYByADKAsyLS5zZXNzaW9uLnYxLkRhaWx5VG9rZW5CdWNrZXQuQ29zdEJ5TW9kZWxFbnRyeRJICg90b2tlbnNfYnlfbW9kZWwYCCADKAsyLy5zZXNzaW9uLnYxLkRhaWx5VG9rZW5CdWNrZXQuVG9rZW5zQnlNb2RlbEVudHJ5EhcKD3VucHJpY2VkX21vZGVscxgJIAMoCRoyChBDb3N0QnlNb2RlbEVudHJ5EgsKA2tleRgBIAEoCRINCgV2YWx1ZRgCIAEoAToCOAEaNAoSVG9rZW5zQnlNb2RlbEVudHJ5EgsKA2tleRgBIAEoCRINCgV2YWx1ZRgCIAEoAzoCOAEiygEKDk1vZGVsQnJlYWtkb3duEhQKDG1vZGVsX2ZhbWlseRgBIAEoCRIaChJ0b3RhbF9pbnB1dF90b2tlbnMYAiABKAMSGwoTdG90YWxfb3V0cHV0X3Rva2VucxgDIAEoAxIZChFjYWNoZV9yZWFkX3Rva2VucxgEIAEoAxIaChJlc3RpbWF0ZWRfY29zdF91c2QYBSABKAESFQoNc2Vzc2lvbl9jb3VudBgGIAEoBRIbChNwcmljaW5nX3VuYXZhaWxhYmxlGAcgASgIInsKFUFjdGl2aXR5Q29zdEJyZWFrZG93bhIvCg1hY3Rpdml0eV90eXBlGAEgASgOMhguc2Vzc2lvbi52MS5BY3Rpdml0eVR5cGUSGgoSZXN0aW1hdGVkX2Nvc3RfdXNkGAIgASgBEhUKDXNlc3Npb25fY291bnQYAyABKAUihgEKDEl0ZW1Sb2xlQ29zdBIPCgdpdGVtX2lkGAEgASgJEhIKCml0ZW1fdGl0bGUYAiABKAkSGgoSZXN0aW1hdGVkX2Nvc3RfdXNkGAMgASgBEhUKDXNlc3Npb25fY291bnQYBCABKAUSHgoWdW5wcmljZWRfc2Vzc2lvbl9jb3VudBgFIAEoBSKlAQoRUm9sZUNvc3RCcmVha2Rvd24SFAoMc2Vzc2lvbl9yb2xlGAEgASgJEhoKEmVzdGltYXRlZF9jb3N0X3VzZBgCIAEoARIVCg1zZXNzaW9uX2NvdW50GAMgASgFEicKBWl0ZW1zGAQgAygLMhguc2Vzc2lvbi52MS5JdGVtUm9sZUNvc3QSHgoWdW5wcmljZWRfc2Vzc2lvbl9jb3VudBgFIAEoBSJXCghUb3BFbnRyeRIMCgRuYW1lGAEgASgJEhMKC3Rva2VuX2NvdW50GAIgASgDEhgKEGFjdGl2YXRpb25fY291bnQYAyABKAVKBAgEEAVSCGNvc3RfdXNkIqgCChlHZXRJbnNpZ2h0c1N1bW1hcnlSZXF1ZXN0EigKBGZyb20YASABKAsyGi5nb29nbGUucHJvdG9idWYuVGltZXN0YW1wEiYKAnRvGAIgASgLMhouZ29vZ2xlLnByb3RvYnVmLlRpbWVzdGFtcBIZCgxtb2RlbF9maWx0ZXIYAyABKAlIAIgBARIeChFzZXNzaW9uX2lkX2ZpbHRlchgEIAEoCUgBiAEBEhcKD2luY2x1ZGVfb3JwaGFucxgFIAEoCBIjChZjb252ZXJzYXRpb25faWRfZmlsdGVyGAYgASgJSAKIAQFCDwoNX21vZGVsX2ZpbHRlckIUChJfc2Vzc2lvbl9pZF9maWx0ZXJCGQoXX2NvbnZlcnNhdGlvbl9pZF9maWx0ZXIijwUKGkdldEluc2lnaHRzU3VtbWFyeVJlc3BvbnNlEjEKCHNlc3Npb25zGAEgAygLMh8uc2Vzc2lvbi52MS5TZXNzaW9uVG9rZW5TdW1tYXJ5EhYKDnRvdGFsX2Nvc3RfdXNkGAIgASgBEhoKEnRvdGFsX2lucHV0X3Rva2VucxgDIAEoAxIbChN0b3RhbF9vdXRwdXRfdG9rZW5zGAQgASgDEh8KF3RvdGFsX2NhY2hlX3JlYWRfdG9rZW5zGAUgASgDEh4KFm92ZXJhbGxfY2FjaGVfaGl0X3JhdGUYBiABKAESKwoFZGFpbHkYByADKAsyHC5zZXNzaW9uLnYxLkRhaWx5VG9rZW5CdWNrZXQSKgoGbW9kZWxzGAggAygLMhouc2Vzc2lvbi52MS5Nb2RlbEJyZWFrZG93bhIoCgp0b3Bfc2tpbGxzGAkgAygLMhQuc2Vzc2lvbi52MS5Ub3BFbnRyeRInCgl0b3BfdG9vbHMYCiADKAsyFC5zZXNzaW9uLnYxLlRvcEVudHJ5EhIKCmlzX2xvYWRpbmcYCyABKAgSMQoNcHJpY2luZ19hc19vZhgMIAEoCzIaLmdvb2dsZS5wcm90b2J1Zi5UaW1lc3RhbXASFwoPdW5wcmljZWRfbW9kZWxzGA0gAygJEioKCGZpbmRpbmdzGA4gAygLMhguc2Vzc2lvbi52MS5XYXN0ZUZpbmRpbmcSPQoSYWN0aXZpdHlfYnJlYWtkb3duGA8gAygLMiEuc2Vzc2lvbi52MS5BY3Rpdml0eUNvc3RCcmVha2Rvd24SNQoOcm9sZV9icmVha2Rvd24YECADKAsyHS5zZXNzaW9uLnYxLlJvbGVDb3N0QnJlYWtkb3duIrcBChhMaXN0U2Vzc2lvblRva2Vuc1JlcXVlc3QSKAoEZnJvbRgBIAEoCzIaLmdvb2dsZS5wcm90b2J1Zi5UaW1lc3RhbXASJgoCdG8YAiABKAsyGi5nb29nbGUucHJvdG9idWYuVGltZXN0YW1wEg8KB3NvcnRfYnkYAyABKAkSEQoJc29ydF9kZXNjGAQgASgIEhEKCXBhZ2Vfc2l6ZRgFIAEoBRISCgpwYWdlX3Rva2VuGAYgASgJInwKGUxpc3RTZXNzaW9uVG9rZW5zUmVzcG9uc2USMQoIc2Vzc2lvbnMYASADKAsyHy5zZXNzaW9uLnYxLlNlc3Npb25Ub2tlblN1bW1hcnkSFwoPbmV4dF9wYWdlX3Rva2VuGAIgASgJEhMKC3RvdGFsX2NvdW50GAMgASgFImgKFFdhdGNoSW5zaWdodHNSZXF1ZXN0EigKBGZyb20YASABKAsyGi5nb29nbGUucHJvdG9idWYuVGltZXN0YW1wEiYKAnRvGAIgASgLMhouZ29vZ2xlLnByb3RvYnVmLlRpbWVzdGFtcCJ6Cg1JbnNpZ2h0c0V2ZW50EhIKCmV2ZW50X3R5cGUYASABKAkSNQoHc2Vzc2lvbhgCIAEoCzIfLnNlc3Npb24udjEuU2Vzc2lvblRva2VuU3VtbWFyeUgAiAEBEhIKCmFsbF9wYXJzZWQYAyABKAhCCgoIX3Nlc3Npb24iyAEKDVR1cm5Ub2tlblN0YXQSLQoJdGltZXN0YW1wGAEgASgLMhouZ29vZ2xlLnByb3RvYnVmLlRpbWVzdGFtcBINCgVtb2RlbBgCIAEoCRIUCgxpbnB1dF90b2tlbnMYAyABKAMSFQoNb3V0cHV0X3Rva2VucxgEIAEoAxIdChVjYWNoZV9jcmVhdGlvbl90b2tlbnMYBSABKAMSGQoRY2FjaGVfcmVhZF90b2tlbnMYBiABKAMSEgoKdG9vbF9uYW1lcxgHIAMoCSI4Ch1HZXRTZXNzaW9uVHVyblRpbWVsaW5lUmVxdWVzdBIXCg9jb252ZXJzYXRpb25faWQYASABKAkiSgoeR2V0U2Vzc2lvblR1cm5UaW1lbGluZVJlc3BvbnNlEigKBXR1cm5zGAEgAygLMhkuc2Vzc2lvbi52MS5UdXJuVG9rZW5TdGF0IocBChVEaXNtaXNzRmluZGluZ1JlcXVlc3QSEgoKZmluZGluZ19pZBgBIAEoCRISCgpzZXNzaW9uX2lkGAIgASgJEhcKD2NvbnZlcnNhdGlvbl9pZBgDIAEoCRItCgxmaW5kaW5nX3R5cGUYBCABKA4yFy5zZXNzaW9uLnYxLkZpbmRpbmdUeXBlIikKFkRpc21pc3NGaW5kaW5nUmVzcG9uc2USDwoHc3VjY2VzcxgBIAEoCCKPAQoNQ29udGV4dFNhbXBsZRISCgp0dXJuX2luZGV4GAEgASgFEi4KCnNhbXBsZWRfYXQYAiABKAsyGi5nb29nbGUucHJvdG9idWYuVGltZXN0YW1wEg0KBW1vZGVsGAMgASgJEhYKDmNvbnRleHRfdG9rZW5zGAQgASgDEhMKC2NvbnRleHRfbWF4GAUgASgDIsMBCg9Db21wYWN0aW9uRXZlbnQSFwoPY29udmVyc2F0aW9uX2lkGAEgASgJEhIKCnR1cm5faW5kZXgYAiABKAUSLwoLb2NjdXJyZWRfYXQYAyABKAsyGi5nb29nbGUucHJvdG9idWYuVGltZXN0YW1wEg8KB3RyaWdnZXIYBCABKAkSFQoNdG9rZW5zX2JlZm9yZRgFIAEoAxIUCgx0b2tlbnNfYWZ0ZXIYBiABKAMSFAoMdG9rZW5zX2ZyZWVkGAcgASgDIpYBChVDb250ZXh0Q2VpbGluZ1N1bW1hcnkSFwoPY29udmVyc2F0aW9uX2lkGAEgASgJEg0KBXR1cm5zGAIgASgFEhMKC3BlYWtfdG9rZW5zGAMgASgDEh8KF2ZyYWN0aW9uX2F0X29yX2Fib3ZlXzc1GAQgASgBEh8KF2ZyYWN0aW9uX2F0X29yX2Fib3ZlXzkwGAUgASgBIjMKGEdldENvbnRleHRIaXN0b3J5UmVxdWVzdBIXCg9jb252ZXJzYXRpb25faWQYASABKAkirQEKGUdldENvbnRleHRIaXN0b3J5UmVzcG9uc2USKgoHc2FtcGxlcxgBIAMoCzIZLnNlc3Npb24udjEuQ29udGV4dFNhbXBsZRIyCgdzdW1tYXJ5GAIgASgLMiEuc2Vzc2lvbi52MS5Db250ZXh0Q2VpbGluZ1N1bW1hcnkSMAoLY29tcGFjdGlvbnMYAyADKAsyGy5zZXNzaW9uLnYxLkNvbXBhY3Rpb25FdmVudCIxCiBMaXN0U2Vzc2lvbnNCeUNlaWxpbmdUaW1lUmVxdWVzdBINCgVsaW1pdBgBIAEoBSJYCiFMaXN0U2Vzc2lvbnNCeUNlaWxpbmdUaW1lUmVzcG9uc2USMwoIc2Vzc2lvbnMYASADKAsyIS5zZXNzaW9uLnYxLkNvbnRleHRDZWlsaW5nU3VtbWFyeSJGChlHZXRDb21wYWN0aW9uU3RhdHNSZXF1ZXN0EikKBXNpbmNlGAEgASgLMhouZ29vZ2xlLnByb3RvYnVmLlRpbWVzdGFtcCKxAQoaR2V0Q29tcGFjdGlvblN0YXRzUmVzcG9uc2USDQoFY291bnQYASABKAUSFAoMbWFudWFsX2NvdW50GAIgASgFEhIKCmF1dG9fY291bnQYAyABKAUSGgoSdG90YWxfdG9rZW5zX2ZyZWVkGAQgASgDEhwKFGF2ZXJhZ2VfdG9rZW5zX2ZyZWVkGAUgASgDEiAKGHNlc3Npb25zX3dpdGhfY29tcGFjdGlvbhgGIAEoBSphCghTZXZlcml0eRIYChRTRVZFUklUWV9VTlNQRUNJRklFRBAAEhEKDVNFVkVSSVRZX0lORk8QARIRCg1TRVZFUklUWV9XQVJOEAISFQoRU0VWRVJJVFlfQ1JJVElDQUwQAyqfAgoLRmluZGluZ1R5cGUSHAoYRklORElOR19UWVBFX1VOU1BFQ0lGSUVEEAASJwojRklORElOR19UWVBFX0NBQ0hFX0hJVF9GTE9PUl9CUkVBQ0gQARImCiJGSU5ESU5HX1RZUEVfU0VTU0lPTl9UT0tFTl9DRUlMSU5HEAISKAokRklORElOR19UWVBFX01PREVMX1NXSVRDSF9DQUNIRV9CVVNUEAMSKAokRklORElOR19UWVBFX09WRVJTSVpFRF9TVEFSVF9DT05URVhUEAQSHgoaRklORElOR19UWVBFX0xPV19DQUNIRV9ST0kQBRItCilGSU5ESU5HX1RZUEVfQ09OVEVYVF9HUk9XVEhfTk9fQ09NUEFDVElPThAGKsABCgxBY3Rpdml0eVR5cGUSHQoZQUNUSVZJVFlfVFlQRV9VTlNQRUNJRklFRBAAEhsKF0FDVElWSVRZX1RZUEVfREVCVUdHSU5HEAESHQoZQUNUSVZJVFlfVFlQRV9SRUZBQ1RPUklORxACEh0KGUFDVElWSVRZX1RZUEVfRkVBVFVSRV9ERVYQAxIdChlBQ1RJVklUWV9UWVBFX0VYUExPUkFUT1JZEAQSFwoTQUNUSVZJVFlfVFlQRV9PVEhFUhAFMsMGCg9JbnNpZ2h0c1NlcnZpY2USZQoSR2V0SW5zaWdodHNTdW1tYXJ5EiUuc2Vzc2lvbi52MS5HZXRJbnNpZ2h0c1N1bW1hcnlSZXF1ZXN0GiYuc2Vzc2lvbi52MS5HZXRJbnNpZ2h0c1N1bW1hcnlSZXNwb25zZSIAEmIKEUxpc3RTZXNzaW9uVG9rZW5zEiQuc2Vzc2lvbi52MS5MaXN0U2Vzc2lvblRva2Vuc1JlcXVlc3QaJS5zZXNzaW9uLnYxLkxpc3RTZXNzaW9uVG9rZW5zUmVzcG9uc2UiABJQCg1XYXRjaEluc2lnaHRzEiAuc2Vzc2lvbi52MS5XYXRjaEluc2lnaHRzUmVxdWVzdBoZLnNlc3Npb24udjEuSW5zaWdodHNFdmVudCIAMAEScQoWR2V0U2Vzc2lvblR1cm5UaW1lbGluZRIpLnNlc3Npb24udjEuR2V0U2Vzc2lvblR1cm5UaW1lbGluZVJlcXVlc3QaKi5zZXNzaW9uLnYxLkdldFNlc3Npb25UdXJuVGltZWxpbmVSZXNwb25zZSIAElkKDkRpc21pc3NGaW5kaW5nEiEuc2Vzc2lvbi52MS5EaXNtaXNzRmluZGluZ1JlcXVlc3QaIi5zZXNzaW9uLnYxLkRpc21pc3NGaW5kaW5nUmVzcG9uc2UiABJiChFHZXRDb250ZXh0SGlzdG9yeRIkLnNlc3Npb24udjEuR2V0Q29udGV4dEhpc3RvcnlSZXF1ZXN0GiUuc2Vzc2lvbi52MS5HZXRDb250ZXh0SGlzdG9yeVJlc3BvbnNlIgASegoZTGlzdFNlc3Npb25zQnlDZWlsaW5nVGltZRIsLnNlc3Npb24udjEuTGlzdFNlc3Npb25zQnlDZWlsaW5nVGltZVJlcXVlc3QaLS5zZXNzaW9uLnYxLkxpc3RTZXNzaW9uc0J5Q2VpbGluZ1RpbWVSZXNwb25zZSIAEmUKEkdldENvbXBhY3Rpb25TdGF0cxIlLnNlc3Npb24udjEuR2V0Q29tcGFjdGlvblN0YXRzUmVxdWVzdBomLnNlc3Npb24udjEuR2V0Q29tcGFjdGlvblN0YXRzUmVzcG9uc2UiAEKtAQoOY29tLnNlc3Npb24udjFCDUluc2lnaHRzUHJvdG9QAVpDZ2l0aHViLmNvbS90c3RhcGxlci9zdGFwbGVyLXNxdWFkL2dlbi9wcm90by9nby9zZXNzaW9uL3YxO3Nlc3Npb252MaICA1NYWKoCClNlc3Npb24uVjHKAgpTZXNzaW9uXFYx4gIWU2Vzc2lvblxWMVxHUEJNZXRhZGF0YeoCC1Nlc3Npb246OlYxYgZwcm90bzM", [
    _bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_1__/* .file_google_protobuf_timestamp */ .i
]);
/**
 * Describes the message session.v1.WasteFinding.
 * Use `create(WasteFindingSchema)` to create a new message.
 */ const WasteFindingSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 0)));
/**
 * Describes the message session.v1.SessionTokenSummary.
 * Use `create(SessionTokenSummarySchema)` to create a new message.
 */ const SessionTokenSummarySchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 1)));
/**
 * Describes the message session.v1.TopToolEntry.
 * Use `create(TopToolEntrySchema)` to create a new message.
 */ const TopToolEntrySchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 2)));
/**
 * Describes the message session.v1.DailyTokenBucket.
 * Use `create(DailyTokenBucketSchema)` to create a new message.
 */ const DailyTokenBucketSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 3)));
/**
 * Describes the message session.v1.ModelBreakdown.
 * Use `create(ModelBreakdownSchema)` to create a new message.
 */ const ModelBreakdownSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 4)));
/**
 * Describes the message session.v1.ActivityCostBreakdown.
 * Use `create(ActivityCostBreakdownSchema)` to create a new message.
 */ const ActivityCostBreakdownSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 5)));
/**
 * Describes the message session.v1.ItemRoleCost.
 * Use `create(ItemRoleCostSchema)` to create a new message.
 */ const ItemRoleCostSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 6)));
/**
 * Describes the message session.v1.RoleCostBreakdown.
 * Use `create(RoleCostBreakdownSchema)` to create a new message.
 */ const RoleCostBreakdownSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 7)));
/**
 * Describes the message session.v1.TopEntry.
 * Use `create(TopEntrySchema)` to create a new message.
 */ const TopEntrySchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 8)));
/**
 * Describes the message session.v1.GetInsightsSummaryRequest.
 * Use `create(GetInsightsSummaryRequestSchema)` to create a new message.
 */ const GetInsightsSummaryRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_insights, 9);
/**
 * Describes the message session.v1.GetInsightsSummaryResponse.
 * Use `create(GetInsightsSummaryResponseSchema)` to create a new message.
 */ const GetInsightsSummaryResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 10)));
/**
 * Describes the message session.v1.ListSessionTokensRequest.
 * Use `create(ListSessionTokensRequestSchema)` to create a new message.
 */ const ListSessionTokensRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 11)));
/**
 * Describes the message session.v1.ListSessionTokensResponse.
 * Use `create(ListSessionTokensResponseSchema)` to create a new message.
 */ const ListSessionTokensResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 12)));
/**
 * Describes the message session.v1.WatchInsightsRequest.
 * Use `create(WatchInsightsRequestSchema)` to create a new message.
 */ const WatchInsightsRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_insights, 13);
/**
 * Describes the message session.v1.InsightsEvent.
 * Use `create(InsightsEventSchema)` to create a new message.
 */ const InsightsEventSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 14)));
/**
 * Describes the message session.v1.TurnTokenStat.
 * Use `create(TurnTokenStatSchema)` to create a new message.
 */ const TurnTokenStatSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 15)));
/**
 * Describes the message session.v1.GetSessionTurnTimelineRequest.
 * Use `create(GetSessionTurnTimelineRequestSchema)` to create a new message.
 */ const GetSessionTurnTimelineRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_insights, 16);
/**
 * Describes the message session.v1.GetSessionTurnTimelineResponse.
 * Use `create(GetSessionTurnTimelineResponseSchema)` to create a new message.
 */ const GetSessionTurnTimelineResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 17)));
/**
 * Describes the message session.v1.DismissFindingRequest.
 * Use `create(DismissFindingRequestSchema)` to create a new message.
 */ const DismissFindingRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_insights, 18);
/**
 * Describes the message session.v1.DismissFindingResponse.
 * Use `create(DismissFindingResponseSchema)` to create a new message.
 */ const DismissFindingResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 19)));
/**
 * Describes the message session.v1.ContextSample.
 * Use `create(ContextSampleSchema)` to create a new message.
 */ const ContextSampleSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 20)));
/**
 * Describes the message session.v1.CompactionEvent.
 * Use `create(CompactionEventSchema)` to create a new message.
 */ const CompactionEventSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 21)));
/**
 * Describes the message session.v1.ContextCeilingSummary.
 * Use `create(ContextCeilingSummarySchema)` to create a new message.
 */ const ContextCeilingSummarySchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 22)));
/**
 * Describes the message session.v1.GetContextHistoryRequest.
 * Use `create(GetContextHistoryRequestSchema)` to create a new message.
 */ const GetContextHistoryRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 23)));
/**
 * Describes the message session.v1.GetContextHistoryResponse.
 * Use `create(GetContextHistoryResponseSchema)` to create a new message.
 */ const GetContextHistoryResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 24)));
/**
 * Describes the message session.v1.ListSessionsByCeilingTimeRequest.
 * Use `create(ListSessionsByCeilingTimeRequestSchema)` to create a new message.
 */ const ListSessionsByCeilingTimeRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 25)));
/**
 * Describes the message session.v1.ListSessionsByCeilingTimeResponse.
 * Use `create(ListSessionsByCeilingTimeResponseSchema)` to create a new message.
 */ const ListSessionsByCeilingTimeResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 26)));
/**
 * Describes the message session.v1.GetCompactionStatsRequest.
 * Use `create(GetCompactionStatsRequestSchema)` to create a new message.
 */ const GetCompactionStatsRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 27)));
/**
 * Describes the message session.v1.GetCompactionStatsResponse.
 * Use `create(GetCompactionStatsResponseSchema)` to create a new message.
 */ const GetCompactionStatsResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 28)));
/**
 * Severity classifies how urgently a WasteFinding should be acted on.
 *
 * @generated from enum session.v1.Severity
 */ var Severity = /*#__PURE__*/ function(Severity) {
    /**
   * @generated from enum value: SEVERITY_UNSPECIFIED = 0;
   */ Severity[Severity["UNSPECIFIED"] = 0] = "UNSPECIFIED";
    /**
   * @generated from enum value: SEVERITY_INFO = 1;
   */ Severity[Severity["INFO"] = 1] = "INFO";
    /**
   * @generated from enum value: SEVERITY_WARN = 2;
   */ Severity[Severity["WARN"] = 2] = "WARN";
    /**
   * @generated from enum value: SEVERITY_CRITICAL = 3;
   */ Severity[Severity["CRITICAL"] = 3] = "CRITICAL";
    return Severity;
}({});
/**
 * Describes the enum session.v1.Severity.
 */ const SeveritySchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_session_v1_insights, 0)));
/**
 * FindingType identifies which waste-pattern heuristic produced a WasteFinding.
 * 4 non-zero values shipped in v1 — see plan.md's "Detector scope cut" note.
 * FINDING_TYPE_LOW_CACHE_ROI (5) shipped later, promoting ComputeCacheROI from
 * a tooltip-only metric to a first-class finding — see tstapler/stapler-squad#879.
 *
 * @generated from enum session.v1.FindingType
 */ var FindingType = /*#__PURE__*/ function(FindingType) {
    /**
   * @generated from enum value: FINDING_TYPE_UNSPECIFIED = 0;
   */ FindingType[FindingType["UNSPECIFIED"] = 0] = "UNSPECIFIED";
    /**
   * @generated from enum value: FINDING_TYPE_CACHE_HIT_FLOOR_BREACH = 1;
   */ FindingType[FindingType["CACHE_HIT_FLOOR_BREACH"] = 1] = "CACHE_HIT_FLOOR_BREACH";
    /**
   * @generated from enum value: FINDING_TYPE_SESSION_TOKEN_CEILING = 2;
   */ FindingType[FindingType["SESSION_TOKEN_CEILING"] = 2] = "SESSION_TOKEN_CEILING";
    /**
   * @generated from enum value: FINDING_TYPE_MODEL_SWITCH_CACHE_BUST = 3;
   */ FindingType[FindingType["MODEL_SWITCH_CACHE_BUST"] = 3] = "MODEL_SWITCH_CACHE_BUST";
    /**
   * @generated from enum value: FINDING_TYPE_OVERSIZED_START_CONTEXT = 4;
   */ FindingType[FindingType["OVERSIZED_START_CONTEXT"] = 4] = "OVERSIZED_START_CONTEXT";
    /**
   * @generated from enum value: FINDING_TYPE_LOW_CACHE_ROI = 5;
   */ FindingType[FindingType["LOW_CACHE_ROI"] = 5] = "LOW_CACHE_ROI";
    /**
   * @generated from enum value: FINDING_TYPE_CONTEXT_GROWTH_NO_COMPACTION = 6;
   */ FindingType[FindingType["CONTEXT_GROWTH_NO_COMPACTION"] = 6] = "CONTEXT_GROWTH_NO_COMPACTION";
    return FindingType;
}({});
/**
 * Describes the enum session.v1.FindingType.
 */ const FindingTypeSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_session_v1_insights, 1)));
/**
 * ActivityType classifies the kind of work a session performed. Defined now
 * (Epic 1.1) but not populated until Epic 1.2's ClassifyActivity lands.
 *
 * @generated from enum session.v1.ActivityType
 */ var ActivityType = /*#__PURE__*/ function(ActivityType) {
    /**
   * @generated from enum value: ACTIVITY_TYPE_UNSPECIFIED = 0;
   */ ActivityType[ActivityType["UNSPECIFIED"] = 0] = "UNSPECIFIED";
    /**
   * @generated from enum value: ACTIVITY_TYPE_DEBUGGING = 1;
   */ ActivityType[ActivityType["DEBUGGING"] = 1] = "DEBUGGING";
    /**
   * @generated from enum value: ACTIVITY_TYPE_REFACTORING = 2;
   */ ActivityType[ActivityType["REFACTORING"] = 2] = "REFACTORING";
    /**
   * @generated from enum value: ACTIVITY_TYPE_FEATURE_DEV = 3;
   */ ActivityType[ActivityType["FEATURE_DEV"] = 3] = "FEATURE_DEV";
    /**
   * @generated from enum value: ACTIVITY_TYPE_EXPLORATORY = 4;
   */ ActivityType[ActivityType["EXPLORATORY"] = 4] = "EXPLORATORY";
    /**
   * @generated from enum value: ACTIVITY_TYPE_OTHER = 5;
   */ ActivityType[ActivityType["OTHER"] = 5] = "OTHER";
    return ActivityType;
}({});
/**
 * Describes the enum session.v1.ActivityType.
 */ const ActivityTypeSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_session_v1_insights, 2)));
/**
 * InsightsService provides token usage analytics derived from JSONL transcripts.
 *
 * @generated from service session.v1.InsightsService
 */ const InsightsService = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_3__/* .serviceDesc */ .Z)(file_session_v1_insights, 0);


/***/ }),

/***/ 81240:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   HH: () => (/* binding */ useDismissFinding),
/* harmony export */   Ih: () => (/* binding */ useSessionTurnTimeline),
/* harmony export */   VT: () => (/* binding */ useInsightsSummary),
/* harmony export */   xk: () => (/* binding */ useSessionDetail)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(66144);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(21694);
/* harmony import */ var _gen_session_v1_insights_pb__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(62428);
/* harmony import */ var _bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(95444);
/* harmony import */ var _bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(28765);
// +feature: insights-dashboard
/* __next_internal_client_entry_do_not_use__ useInsightsSummary,useSessionDetail,useSessionTurnTimeline,useDismissFinding auto */ 






/** Hook that fetches the InsightsSummary and subscribes to live updates via WatchInsights. */ function useInsightsSummary() {
    let filters = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : {};
    const [summary, setSummary] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(null);
    const [loading, setLoading] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(true);
    const [isLiveUpdating, setIsLiveUpdating] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(false);
    const [error, setError] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(null);
    const abortWatchRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    const fetchCountRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(0);
    const client = (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>(0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__/* .createClient */ .UU)(_gen_session_v1_insights_pb__WEBPACK_IMPORTED_MODULE_2__/* .InsightsService */ .VJ, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_1__/* .getWatchTransport */ .VZ)()), []);
    const fetchSummary = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async ()=>{
        const fetchId = ++fetchCountRef.current;
        setLoading(true);
        setError(null);
        try {
            var _filters_includeOrphans;
            const req = (0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_insights_pb__WEBPACK_IMPORTED_MODULE_2__/* .GetInsightsSummaryRequestSchema */ .s7, {
                includeOrphans: (_filters_includeOrphans = filters.includeOrphans) !== null && _filters_includeOrphans !== void 0 ? _filters_includeOrphans : true,
                ...filters.modelFilter && {
                    modelFilter: filters.modelFilter
                },
                ...filters.sessionIdFilter && {
                    sessionIdFilter: filters.sessionIdFilter
                },
                ...filters.from && {
                    from: (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_5__/* .timestampFromDate */ .LI)(filters.from)
                },
                ...filters.to && {
                    to: (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_5__/* .timestampFromDate */ .LI)(filters.to)
                }
            });
            const res = await client.getInsightsSummary(req);
            if (fetchId === fetchCountRef.current) {
                setSummary(res);
            }
        } catch (err) {
            if (fetchId === fetchCountRef.current) {
                setError(err instanceof Error ? err.message : "Failed to load insights");
            }
        } finally{
            if (fetchId === fetchCountRef.current) {
                setLoading(false);
            }
        }
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        filters.includeOrphans,
        filters.modelFilter,
        filters.sessionIdFilter,
        filters.from,
        filters.to
    ]);
    const INITIAL_RETRY_DELAY_MS = 1000;
    const MAX_RETRY_DELAY_MS = 30000;
    /** Subscribe to WatchInsights with exponential-backoff reconnect. */ const startWatch = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>{
        if (abortWatchRef.current) {
            abortWatchRef.current.abort();
        }
        const abort = new AbortController();
        abortWatchRef.current = abort;
        (async ()=>{
            let retryDelay = INITIAL_RETRY_DELAY_MS;
            while(!abort.signal.aborted){
                try {
                    const req = (0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_insights_pb__WEBPACK_IMPORTED_MODULE_2__/* .WatchInsightsRequestSchema */ .dQ, {
                        ...filters.from && {
                            from: (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_5__/* .timestampFromDate */ .LI)(filters.from)
                        },
                        ...filters.to && {
                            to: (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_5__/* .timestampFromDate */ .LI)(filters.to)
                        }
                    });
                    const stream = client.watchInsights(req, {
                        signal: abort.signal
                    });
                    retryDelay = INITIAL_RETRY_DELAY_MS;
                    for await (const event of stream){
                        if (abort.signal.aborted) break;
                        if (event.eventType === "update" && event.session) {
                            setIsLiveUpdating(true);
                            setSummary((prev)=>{
                                if (!prev) return prev;
                                // If time filter is active, skip sessions outside the filter window
                                if (filters.from || filters.to) {
                                    const lastTs = event.session.lastMessageAt;
                                    if (lastTs) {
                                        const ts = Number(lastTs.seconds) * 1000;
                                        if (filters.from && ts < filters.from.getTime()) return prev;
                                        if (filters.to && ts > filters.to.getTime()) return prev;
                                    }
                                }
                                const key = event.session.conversationId || event.session.sessionId;
                                const idx = prev.sessions.findIndex((s)=>(s.conversationId || s.sessionId) === key);
                                if (idx === -1) {
                                    return {
                                        ...prev,
                                        sessions: [
                                            ...prev.sessions,
                                            event.session
                                        ]
                                    };
                                }
                                const sessions = [
                                    ...prev.sessions
                                ];
                                sessions[idx] = event.session;
                                return {
                                    ...prev,
                                    sessions
                                };
                            });
                            setIsLiveUpdating(false);
                        } else if (event.eventType === "parse_complete") {
                            setIsLiveUpdating(true);
                            await fetchSummary();
                            setIsLiveUpdating(false);
                        }
                    }
                } catch (err) {
                    if (!abort.signal.aborted) {
                        console.error("[WatchInsights] stream error, reconnecting:", err);
                    }
                }
                if (!abort.signal.aborted) {
                    await new Promise((r)=>setTimeout(r, retryDelay));
                    retryDelay = Math.min(retryDelay * 2, MAX_RETRY_DELAY_MS);
                }
            }
        })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        fetchSummary,
        filters.from,
        filters.to
    ]);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        fetchSummary();
        startWatch();
        return ()=>{
            var _abortWatchRef_current;
            (_abortWatchRef_current = abortWatchRef.current) === null || _abortWatchRef_current === void 0 ? void 0 : _abortWatchRef_current.abort();
        };
    }, [
        fetchSummary,
        startWatch
    ]);
    return {
        summary,
        loading,
        isLiveUpdating,
        error,
        refetch: fetchSummary
    };
}
/**
 * Hook that fetches a single session's SessionTokenSummary by ID, for the
 * deep-linkable /insights/session-detail?sessionId= route (Epic 1.4, Story
 * 1.4.3).
 * Sets both sessionIdFilter and conversationIdFilter to the same value so an
 * orphan session (empty sessionId) is still reachable, and includeOrphans so
 * it isn't filtered out. Deliberately omits from/to — the route must resolve
 * regardless of the dashboard's global date range at the time the link was
 * created (bookmarkability, per research/features.md §5).
 */ function useSessionDetail(sessionId) {
    const [summary, setSummary] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(null);
    const [loading, setLoading] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(true);
    const [error, setError] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(null);
    const client = (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>(0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__/* .createClient */ .UU)(_gen_session_v1_insights_pb__WEBPACK_IMPORTED_MODULE_2__/* .InsightsService */ .VJ, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_1__/* .getConnectTransport */ .qW)()), []);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        let cancelled = false;
        if (!sessionId) {
            // An empty sessionId (e.g. "?sessionId=" absent) is not "no filter" to
            // this hook, unlike the server's GetInsightsSummary RPC, which treats an
            // empty sessionIdFilter as unset and would return every session,
            // making res.sessions[0] an arbitrary session's data. Short-circuit to
            // a not-found state instead of issuing the request.
            setSummary(null);
            setLoading(false);
            setError(null);
            return;
        }
        setLoading(true);
        setError(null);
        (async ()=>{
            try {
                const req = (0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_insights_pb__WEBPACK_IMPORTED_MODULE_2__/* .GetInsightsSummaryRequestSchema */ .s7, {
                    sessionIdFilter: sessionId,
                    conversationIdFilter: sessionId,
                    includeOrphans: true
                });
                const res = await client.getInsightsSummary(req);
                var _res_sessions_;
                if (!cancelled) setSummary((_res_sessions_ = res.sessions[0]) !== null && _res_sessions_ !== void 0 ? _res_sessions_ : null);
            } catch (err) {
                if (!cancelled) {
                    setError(err instanceof Error ? err.message : "Failed to load session");
                }
            } finally{
                if (!cancelled) setLoading(false);
            }
        })();
        return ()=>{
            cancelled = true;
        };
    }, [
        sessionId,
        client
    ]);
    return {
        summary,
        loading,
        error
    };
}
/**
 * Hook that lazily fetches per-turn token stats for a single session, keyed on
 * conversationId. No-ops (empty turns, not loading) when conversationId is
 * falsy — used by SessionDetailDrawer, fetched only when the drawer is open
 * for a given session, never batched with the list RPCs.
 */ function useSessionTurnTimeline(conversationId) {
    const [turns, setTurns] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)([]);
    const [loading, setLoading] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(false);
    const [error, setError] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(null);
    const client = (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>(0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__/* .createClient */ .UU)(_gen_session_v1_insights_pb__WEBPACK_IMPORTED_MODULE_2__/* .InsightsService */ .VJ, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_1__/* .getConnectTransport */ .qW)()), []);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        if (!conversationId) {
            setTurns([]);
            setLoading(false);
            return;
        }
        let cancelled = false;
        setLoading(true);
        setError(null);
        (async ()=>{
            try {
                const res = await client.getSessionTurnTimeline((0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_insights_pb__WEBPACK_IMPORTED_MODULE_2__/* .GetSessionTurnTimelineRequestSchema */ .H5, {
                    conversationId
                }));
                if (!cancelled) setTurns(res.turns);
            } catch (err) {
                if (!cancelled) {
                    console.error("[GetSessionTurnTimeline] fetch error:", err);
                    setError(err instanceof Error ? err.message : "Failed to load turn timeline");
                }
            } finally{
                if (!cancelled) setLoading(false);
            }
        })();
        return ()=>{
            cancelled = true;
        };
    }, [
        conversationId,
        client
    ]);
    return {
        turns,
        loading,
        error
    };
}
/**
 * Hook that dismisses a WasteFinding by its stable finding_id (see
 * ComputeFindingID, session/tokens/findings.go), so it's excluded from future
 * GetInsightsSummary responses. The caller (FindingsPanel) is responsible for
 * removing the card from its own rendered list — this hook only performs the
 * RPC call.
 */ function useDismissFinding() {
    const client = (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>(0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__/* .createClient */ .UU)(_gen_session_v1_insights_pb__WEBPACK_IMPORTED_MODULE_2__/* .InsightsService */ .VJ, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_1__/* .getConnectTransport */ .qW)()), []);
    const dismissFinding = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (findingId, sessionId, conversationId, findingType)=>{
        await client.dismissFinding((0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_insights_pb__WEBPACK_IMPORTED_MODULE_2__/* .DismissFindingRequestSchema */ .O5, {
            findingId,
            sessionId,
            conversationId,
            findingType
        }));
    }, [
        client
    ]);
    return {
        dismissFinding
    };
}


/***/ })

}]);
//# sourceMappingURL=2805-4f44f71c3ade0c30.js.map