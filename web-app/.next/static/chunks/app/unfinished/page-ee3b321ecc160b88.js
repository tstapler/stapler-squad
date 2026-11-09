(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[4570],{

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

/***/ 3335:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   CC: () => (/* binding */ formatTimestampShort),
/* harmony export */   Ey: () => (/* binding */ formatTimestamp),
/* harmony export */   dB: () => (/* binding */ getUserTimezone),
/* harmony export */   fw: () => (/* binding */ formatRelativeTime),
/* harmony export */   jX: () => (/* binding */ TIME_RANGE_PRESETS)
/* harmony export */ });
/* unused harmony export formatTime */
/**
 * Utility functions for date and time formatting with timezone support
 */ /**
 * Format a timestamp for display with full date, time, and timezone
 * @param date - Date object or timestamp to format
 * @returns Formatted string in user's local timezone (e.g., "1/5/2025, 3:45:30 PM PST")
 */ function formatTimestamp(date) {
    const dateObj = typeof date === 'number' ? new Date(date) : date;
    return dateObj.toLocaleString(undefined, {
        year: 'numeric',
        month: 'numeric',
        day: 'numeric',
        hour: 'numeric',
        minute: '2-digit',
        second: '2-digit',
        timeZoneName: 'short'
    });
}
/**
 * Format a timestamp for display with date and time (no timezone)
 * @param date - Date object or timestamp to format
 * @returns Formatted string in user's local timezone (e.g., "1/5/2025, 3:45:30 PM")
 */ function formatTimestampShort(date) {
    const dateObj = typeof date === 'number' ? new Date(date) : date;
    return dateObj.toLocaleString(undefined, {
        year: 'numeric',
        month: 'numeric',
        day: 'numeric',
        hour: 'numeric',
        minute: '2-digit',
        second: '2-digit'
    });
}
/**
 * Format a timestamp for display with just time (no date)
 * @param date - Date object or timestamp to format
 * @returns Formatted string in user's local timezone (e.g., "3:45:30 PM")
 */ function formatTime(date) {
    const dateObj = typeof date === 'number' ? new Date(date) : date;
    return dateObj.toLocaleTimeString(undefined, {
        hour: 'numeric',
        minute: '2-digit',
        second: '2-digit'
    });
}
/**
 * Format a timestamp as relative time (e.g., "5m ago", "2h ago")
 * @param timestamp - Unix timestamp in milliseconds
 * @returns Relative time string
 */ function formatRelativeTime(timestamp) {
    const date = new Date(timestamp);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);
    const diffDays = Math.floor(diffMs / 86400000);
    if (diffMins < 1) return "Just now";
    if (diffMins < 60) return "".concat(diffMins, "m ago");
    if (diffHours < 24) return "".concat(diffHours, "h ago");
    if (diffDays < 7) return "".concat(diffDays, "d ago");
    return date.toLocaleDateString();
}
/**
 * Get the user's timezone abbreviation (e.g., "PST", "EST", "UTC")
 */ function getUserTimezone() {
    const date = new Date();
    const timeZoneName = date.toLocaleString(undefined, {
        timeZoneName: 'short'
    }).split(' ').pop();
    return timeZoneName || Intl.DateTimeFormat().resolvedOptions().timeZone;
}
/**
 * Standard time range presets for log filtering
 */ const TIME_RANGE_PRESETS = [
    {
        label: 'Last 5 minutes',
        value: '5m',
        getRange: ()=>({
                start: new Date(Date.now() - 5 * 60 * 1000),
                end: new Date()
            })
    },
    {
        label: 'Last 15 minutes',
        value: '15m',
        getRange: ()=>({
                start: new Date(Date.now() - 15 * 60 * 1000),
                end: new Date()
            })
    },
    {
        label: 'Last 1 hour',
        value: '1h',
        getRange: ()=>({
                start: new Date(Date.now() - 60 * 60 * 1000),
                end: new Date()
            })
    },
    {
        label: 'Last 4 hours',
        value: '4h',
        getRange: ()=>({
                start: new Date(Date.now() - 4 * 60 * 60 * 1000),
                end: new Date()
            })
    },
    {
        label: 'Last 24 hours',
        value: '24h',
        getRange: ()=>({
                start: new Date(Date.now() - 24 * 60 * 60 * 1000),
                end: new Date()
            })
    },
    {
        label: 'Last 7 days',
        value: '7d',
        getRange: ()=>({
                start: new Date(Date.now() - 7 * 24 * 60 * 60 * 1000),
                end: new Date()
            })
    },
    {
        label: 'All time',
        value: 'all',
        getRange: ()=>({
                start: new Date(0),
                end: new Date()
            })
    }
];


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

/***/ 13798:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   $Y: () => (/* binding */ AddGitHubAccountWithTokenRequestSchema),
/* harmony export */   AY: () => (/* binding */ ClosePRDirectRequestSchema),
/* harmony export */   Al: () => (/* binding */ ListGitHubAccountsRequestSchema),
/* harmony export */   Ex: () => (/* binding */ PRDraftToggleRequestSchema),
/* harmony export */   Gs: () => (/* binding */ WatchUserPRsRequestSchema),
/* harmony export */   LV: () => (/* binding */ RevokeGitHubTokenRequestSchema),
/* harmony export */   Rx: () => (/* binding */ ListGitHubCLIHostsRequestSchema),
/* harmony export */   SH: () => (/* binding */ StartGitHubDeviceAuthRequestSchema),
/* harmony export */   Tn: () => (/* binding */ NudgeSessionForPRRequestSchema),
/* harmony export */   UX: () => (/* binding */ NudgeOutcome),
/* harmony export */   WJ: () => (/* binding */ MergePRDirectRequestSchema),
/* harmony export */   X_: () => (/* binding */ DeviceAuthStatus),
/* harmony export */   aI: () => (/* binding */ GitHubUserService),
/* harmony export */   it: () => (/* binding */ AddGitHubAccountFromCLIRequestSchema),
/* harmony export */   m$: () => (/* binding */ AccountPollState),
/* harmony export */   nQ: () => (/* binding */ PollGitHubDeviceAuthRequestSchema),
/* harmony export */   xE: () => (/* binding */ PRKeySchema)
/* harmony export */ });
/* unused harmony exports file_session_v1_github_user, PRActionResponseSchema, GitHubAccountSchema, GitHubAuthStateSchema, ListUserPRsRequestSchema, ListUserPRsResponseSchema, AccountPollStatusSchema, UserPREventSchema, GetGitHubAuthStateRequestSchema, GetGitHubAuthStateResponseSchema, StartGitHubDeviceAuthResponseSchema, PollGitHubDeviceAuthResponseSchema, RevokeGitHubTokenResponseSchema, ListGitHubAccountsResponseSchema, AddGitHubAccountWithTokenResponseSchema, GitHubCLIHostSchema, ListGitHubCLIHostsResponseSchema, NudgeSessionForPRResponseSchema, AccountPollStateSchema, DeviceAuthStatusSchema, NudgeOutcomeSchema, NudgeReason, NudgeReasonSchema */
/* harmony import */ var _bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(1844);
/* harmony import */ var _bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(15033);
/* harmony import */ var _bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(95765);
/* harmony import */ var _types_pb__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(81432);
// @generated by protoc-gen-es v2.11.0 with parameter "target=ts,ts_nocheck=false,keep_empty_files=true"
// @generated from file session/v1/github_user.proto (package session.v1, syntax proto3)
/* eslint-disable */ 

/**
 * Describes the file session/v1/github_user.proto.
 */ const file_session_v1_github_user = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_1__/* .fileDesc */ .w)("ChxzZXNzaW9uL3YxL2dpdGh1Yl91c2VyLnByb3RvEgpzZXNzaW9uLnYxInEKFE1lcmdlUFJEaXJlY3RSZXF1ZXN0Eg0KBW93bmVyGAEgASgJEgwKBHJlcG8YAiABKAkSDgoGbnVtYmVyGAMgASgFEgwKBGhvc3QYBCABKAkSEwoGbWV0aG9kGAUgASgJSACIAQFCCQoHX21ldGhvZCJRChRDbG9zZVBSRGlyZWN0UmVxdWVzdBINCgVvd25lchgBIAEoCRIMCgRyZXBvGAIgASgJEg4KBm51bWJlchgDIAEoBRIMCgRob3N0GAQgASgJIlEKFFBSRHJhZnRUb2dnbGVSZXF1ZXN0Eg0KBW93bmVyGAEgASgJEgwKBHJlcG8YAiABKAkSDgoGbnVtYmVyGAMgASgFEgwKBGhvc3QYBCABKAkiNAoQUFJBY3Rpb25SZXNwb25zZRIPCgdzdWNjZXNzGAEgASgIEg8KB21lc3NhZ2UYAiABKAkiRQoNR2l0SHViQWNjb3VudBIQCgh1c2VybmFtZRgBIAEoCRIUCgxpc19lbnZfdG9rZW4YAiABKAgSDAoEaG9zdBgDIAEoCSJ6Cg9HaXRIdWJBdXRoU3RhdGUSEQoJYXZhaWxhYmxlGAEgASgIEhAKCHVzZXJuYW1lGAIgASgJEhUKDWVycm9yX21lc3NhZ2UYAyABKAkSKwoIYWNjb3VudHMYBCADKAsyGS5zZXNzaW9uLnYxLkdpdEh1YkFjY291bnQiFAoSTGlzdFVzZXJQUnNSZXF1ZXN0IqABChNMaXN0VXNlclBSc1Jlc3BvbnNlEh8KA3BycxgBIAMoCzISLnNlc3Npb24udjEuVXNlclBSEi8KCmF1dGhfc3RhdGUYAiABKAsyGy5zZXNzaW9uLnYxLkdpdEh1YkF1dGhTdGF0ZRI3ChBhY2NvdW50X3N0YXR1c2VzGAMgAygLMh0uc2Vzc2lvbi52MS5BY2NvdW50UG9sbFN0YXR1cyJ1ChFBY2NvdW50UG9sbFN0YXR1cxIMCgRob3N0GAEgASgJEhUKDWFjY291bnRfbG9naW4YAiABKAkSKwoFc3RhdGUYAyABKA4yHC5zZXNzaW9uLnYxLkFjY291bnRQb2xsU3RhdGUSDgoGZGV0YWlsGAQgASgJIhUKE1dhdGNoVXNlclBSc1JlcXVlc3QirAEKC1VzZXJQUkV2ZW50EhIKCmV2ZW50X3R5cGUYASABKAkSHwoDcHJzGAIgAygLMhIuc2Vzc2lvbi52MS5Vc2VyUFISLwoKYXV0aF9zdGF0ZRgDIAEoCzIbLnNlc3Npb24udjEuR2l0SHViQXV0aFN0YXRlEjcKEGFjY291bnRfc3RhdHVzZXMYBCADKAsyHS5zZXNzaW9uLnYxLkFjY291bnRQb2xsU3RhdHVzIhsKGUdldEdpdEh1YkF1dGhTdGF0ZVJlcXVlc3QiTQoaR2V0R2l0SHViQXV0aFN0YXRlUmVzcG9uc2USLwoKYXV0aF9zdGF0ZRgBIAEoCzIbLnNlc3Npb24udjEuR2l0SHViQXV0aFN0YXRlIiwKHFN0YXJ0R2l0SHViRGV2aWNlQXV0aFJlcXVlc3QSDAoEaG9zdBgBIAEoCSKHAQodU3RhcnRHaXRIdWJEZXZpY2VBdXRoUmVzcG9uc2USEwoLZGV2aWNlX2NvZGUYASABKAkSEQoJdXNlcl9jb2RlGAIgASgJEhgKEHZlcmlmaWNhdGlvbl91cmkYAyABKAkSEgoKZXhwaXJlc19pbhgEIAEoBRIQCghpbnRlcnZhbBgFIAEoBSIyChtQb2xsR2l0SHViRGV2aWNlQXV0aFJlcXVlc3QSEwoLZGV2aWNlX2NvZGUYASABKAkijAEKHFBvbGxHaXRIdWJEZXZpY2VBdXRoUmVzcG9uc2USLAoGc3RhdHVzGAEgASgOMhwuc2Vzc2lvbi52MS5EZXZpY2VBdXRoU3RhdHVzEg0KBWVycm9yGAIgASgJEi8KCmF1dGhfc3RhdGUYAyABKAsyGy5zZXNzaW9uLnYxLkdpdEh1YkF1dGhTdGF0ZSI6ChhSZXZva2VHaXRIdWJUb2tlblJlcXVlc3QSEAoIdXNlcm5hbWUYASABKAkSDAoEaG9zdBgCIAEoCSIbChlSZXZva2VHaXRIdWJUb2tlblJlc3BvbnNlIhsKGUxpc3RHaXRIdWJBY2NvdW50c1JlcXVlc3QiYwoaTGlzdEdpdEh1YkFjY291bnRzUmVzcG9uc2USKwoIYWNjb3VudHMYASADKAsyGS5zZXNzaW9uLnYxLkdpdEh1YkFjY291bnQSGAoQZW50ZXJwcmlzZV9ob3N0cxgCIAMoCSI/CiBBZGRHaXRIdWJBY2NvdW50V2l0aFRva2VuUmVxdWVzdBIMCgRob3N0GAEgASgJEg0KBXRva2VuGAIgASgJIlQKIUFkZEdpdEh1YkFjY291bnRXaXRoVG9rZW5SZXNwb25zZRIvCgphdXRoX3N0YXRlGAEgASgLMhsuc2Vzc2lvbi52MS5HaXRIdWJBdXRoU3RhdGUiRgoNR2l0SHViQ0xJSG9zdBIMCgRob3N0GAEgASgJEhAKCHVzZXJuYW1lGAIgASgJEhUKDWFscmVhZHlfYWRkZWQYAyABKAgiGwoZTGlzdEdpdEh1YkNMSUhvc3RzUmVxdWVzdCJcChpMaXN0R2l0SHViQ0xJSG9zdHNSZXNwb25zZRIoCgVob3N0cxgBIAMoCzIZLnNlc3Npb24udjEuR2l0SHViQ0xJSG9zdBIUCgxnaF9hdmFpbGFibGUYAiABKAgiLgoeQWRkR2l0SHViQWNjb3VudEZyb21DTElSZXF1ZXN0EgwKBGhvc3QYASABKAkiQgoFUFJLZXkSDAoEaG9zdBgBIAEoCRINCgVvd25lchgCIAEoCRIMCgRyZXBvGAMgASgJEg4KBm51bWJlchgEIAEoBSJNChhOdWRnZVNlc3Npb25Gb3JQUlJlcXVlc3QSHQoCcHIYASABKAsyES5zZXNzaW9uLnYxLlBSS2V5EhIKCnNlc3Npb25faWQYAiABKAkilAEKGU51ZGdlU2Vzc2lvbkZvclBSUmVzcG9uc2USKQoHb3V0Y29tZRgBIAEoDjIYLnNlc3Npb24udjEuTnVkZ2VPdXRjb21lEigKB3JlYXNvbnMYAiADKA4yFy5zZXNzaW9uLnYxLk51ZGdlUmVhc29uEhIKCnNlc3Npb25faWQYAyABKAkSDgoGZGV0YWlsGAQgASgJKrkBChBBY2NvdW50UG9sbFN0YXRlEiIKHkFDQ09VTlRfUE9MTF9TVEFURV9VTlNQRUNJRklFRBAAEhkKFUFDQ09VTlRfUE9MTF9TVEFURV9PSxABEiMKH0FDQ09VTlRfUE9MTF9TVEFURV9VTkFVVEhPUklaRUQQAhIjCh9BQ0NPVU5UX1BPTExfU1RBVEVfUkFURV9MSU1JVEVEEAMSHAoYQUNDT1VOVF9QT0xMX1NUQVRFX0VSUk9SEAQqtQEKEERldmljZUF1dGhTdGF0dXMSIgoeREVWSUNFX0FVVEhfU1RBVFVTX1VOU1BFQ0lGSUVEEAASHgoaREVWSUNFX0FVVEhfU1RBVFVTX1BFTkRJTkcQARIfChtERVZJQ0VfQVVUSF9TVEFUVVNfQ09NUExFVEUQAhIeChpERVZJQ0VfQVVUSF9TVEFUVVNfRVhQSVJFRBADEhwKGERFVklDRV9BVVRIX1NUQVRVU19FUlJPUhAEKqACCgxOdWRnZU91dGNvbWUSHQoZTlVER0VfT1VUQ09NRV9VTlNQRUNJRklFRBAAEhsKF05VREdFX09VVENPTUVfREVMSVZFUkVEEAESFgoSTlVER0VfT1VUQ09NRV9CVVNZEAISGAoUTlVER0VfT1VUQ09NRV9QQVVTRUQQAxIbChdOVURHRV9PVVRDT01FX0RVUExJQ0FURRAEEiAKHE5VREdFX09VVENPTUVfTk9USElOR19UT19GSVgQBRIkCiBOVURHRV9PVVRDT01FX1NFU1NJT05fTk9UX0xJTktFRBAGEh4KGk5VREdFX09VVENPTUVfUFJfTk9UX0ZPVU5EEAcSHQoZTlVER0VfT1VUQ09NRV9OT1RfUlVOTklORxAIKpIBCgtOdWRnZVJlYXNvbhIcChhOVURHRV9SRUFTT05fVU5TUEVDSUZJRUQQABIfChtOVURHRV9SRUFTT05fRkFJTElOR19DSEVDS1MQARIjCh9OVURHRV9SRUFTT05fVU5SRVNPTFZFRF9USFJFQURTEAISHwobTlVER0VfUkVBU09OX01FUkdFX0NPTkZMSUNUEAMyzgsKEUdpdEh1YlVzZXJTZXJ2aWNlElAKC0xpc3RVc2VyUFJzEh4uc2Vzc2lvbi52MS5MaXN0VXNlclBSc1JlcXVlc3QaHy5zZXNzaW9uLnYxLkxpc3RVc2VyUFJzUmVzcG9uc2UiABJMCgxXYXRjaFVzZXJQUnMSHy5zZXNzaW9uLnYxLldhdGNoVXNlclBSc1JlcXVlc3QaFy5zZXNzaW9uLnYxLlVzZXJQUkV2ZW50IgAwARJlChJHZXRHaXRIdWJBdXRoU3RhdGUSJS5zZXNzaW9uLnYxLkdldEdpdEh1YkF1dGhTdGF0ZVJlcXVlc3QaJi5zZXNzaW9uLnYxLkdldEdpdEh1YkF1dGhTdGF0ZVJlc3BvbnNlIgASbgoVU3RhcnRHaXRIdWJEZXZpY2VBdXRoEiguc2Vzc2lvbi52MS5TdGFydEdpdEh1YkRldmljZUF1dGhSZXF1ZXN0Gikuc2Vzc2lvbi52MS5TdGFydEdpdEh1YkRldmljZUF1dGhSZXNwb25zZSIAEmsKFFBvbGxHaXRIdWJEZXZpY2VBdXRoEicuc2Vzc2lvbi52MS5Qb2xsR2l0SHViRGV2aWNlQXV0aFJlcXVlc3QaKC5zZXNzaW9uLnYxLlBvbGxHaXRIdWJEZXZpY2VBdXRoUmVzcG9uc2UiABJiChFSZXZva2VHaXRIdWJUb2tlbhIkLnNlc3Npb24udjEuUmV2b2tlR2l0SHViVG9rZW5SZXF1ZXN0GiUuc2Vzc2lvbi52MS5SZXZva2VHaXRIdWJUb2tlblJlc3BvbnNlIgASZQoSTGlzdEdpdEh1YkFjY291bnRzEiUuc2Vzc2lvbi52MS5MaXN0R2l0SHViQWNjb3VudHNSZXF1ZXN0GiYuc2Vzc2lvbi52MS5MaXN0R2l0SHViQWNjb3VudHNSZXNwb25zZSIAEnoKGUFkZEdpdEh1YkFjY291bnRXaXRoVG9rZW4SLC5zZXNzaW9uLnYxLkFkZEdpdEh1YkFjY291bnRXaXRoVG9rZW5SZXF1ZXN0Gi0uc2Vzc2lvbi52MS5BZGRHaXRIdWJBY2NvdW50V2l0aFRva2VuUmVzcG9uc2UiABJlChJMaXN0R2l0SHViQ0xJSG9zdHMSJS5zZXNzaW9uLnYxLkxpc3RHaXRIdWJDTElIb3N0c1JlcXVlc3QaJi5zZXNzaW9uLnYxLkxpc3RHaXRIdWJDTElIb3N0c1Jlc3BvbnNlIgASdgoXQWRkR2l0SHViQWNjb3VudEZyb21DTEkSKi5zZXNzaW9uLnYxLkFkZEdpdEh1YkFjY291bnRGcm9tQ0xJUmVxdWVzdBotLnNlc3Npb24udjEuQWRkR2l0SHViQWNjb3VudFdpdGhUb2tlblJlc3BvbnNlIgASYgoRTnVkZ2VTZXNzaW9uRm9yUFISJC5zZXNzaW9uLnYxLk51ZGdlU2Vzc2lvbkZvclBSUmVxdWVzdBolLnNlc3Npb24udjEuTnVkZ2VTZXNzaW9uRm9yUFJSZXNwb25zZSIAElEKDU1lcmdlUFJEaXJlY3QSIC5zZXNzaW9uLnYxLk1lcmdlUFJEaXJlY3RSZXF1ZXN0Ghwuc2Vzc2lvbi52MS5QUkFjdGlvblJlc3BvbnNlIgASUQoNQ2xvc2VQUkRpcmVjdBIgLnNlc3Npb24udjEuQ2xvc2VQUkRpcmVjdFJlcXVlc3QaHC5zZXNzaW9uLnYxLlBSQWN0aW9uUmVzcG9uc2UiABJUChBDb252ZXJ0UFJUb0RyYWZ0EiAuc2Vzc2lvbi52MS5QUkRyYWZ0VG9nZ2xlUmVxdWVzdBocLnNlc3Npb24udjEuUFJBY3Rpb25SZXNwb25zZSIAEk8KC01hcmtQUlJlYWR5EiAuc2Vzc2lvbi52MS5QUkRyYWZ0VG9nZ2xlUmVxdWVzdBocLnNlc3Npb24udjEuUFJBY3Rpb25SZXNwb25zZSIAQq8BCg5jb20uc2Vzc2lvbi52MUIPR2l0aHViVXNlclByb3RvUAFaQ2dpdGh1Yi5jb20vdHN0YXBsZXIvc3RhcGxlci1zcXVhZC9nZW4vcHJvdG8vZ28vc2Vzc2lvbi92MTtzZXNzaW9udjGiAgNTWFiqAgpTZXNzaW9uLlYxygIKU2Vzc2lvblxWMeICFlNlc3Npb25cVjFcR1BCTWV0YWRhdGHqAgtTZXNzaW9uOjpWMWIGcHJvdG8z", [
    _types_pb__WEBPACK_IMPORTED_MODULE_0__/* .file_session_v1_types */ .m$
]);
/**
 * Describes the message session.v1.MergePRDirectRequest.
 * Use `create(MergePRDirectRequestSchema)` to create a new message.
 */ const MergePRDirectRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_github_user, 0);
/**
 * Describes the message session.v1.ClosePRDirectRequest.
 * Use `create(ClosePRDirectRequestSchema)` to create a new message.
 */ const ClosePRDirectRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_github_user, 1);
/**
 * Describes the message session.v1.PRDraftToggleRequest.
 * Use `create(PRDraftToggleRequestSchema)` to create a new message.
 */ const PRDraftToggleRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_github_user, 2);
/**
 * Describes the message session.v1.PRActionResponse.
 * Use `create(PRActionResponseSchema)` to create a new message.
 */ const PRActionResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 3)));
/**
 * Describes the message session.v1.GitHubAccount.
 * Use `create(GitHubAccountSchema)` to create a new message.
 */ const GitHubAccountSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 4)));
/**
 * Describes the message session.v1.GitHubAuthState.
 * Use `create(GitHubAuthStateSchema)` to create a new message.
 */ const GitHubAuthStateSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 5)));
/**
 * Describes the message session.v1.ListUserPRsRequest.
 * Use `create(ListUserPRsRequestSchema)` to create a new message.
 */ const ListUserPRsRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 6)));
/**
 * Describes the message session.v1.ListUserPRsResponse.
 * Use `create(ListUserPRsResponseSchema)` to create a new message.
 */ const ListUserPRsResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 7)));
/**
 * Describes the message session.v1.AccountPollStatus.
 * Use `create(AccountPollStatusSchema)` to create a new message.
 */ const AccountPollStatusSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 8)));
/**
 * Describes the message session.v1.WatchUserPRsRequest.
 * Use `create(WatchUserPRsRequestSchema)` to create a new message.
 */ const WatchUserPRsRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_github_user, 9);
/**
 * Describes the message session.v1.UserPREvent.
 * Use `create(UserPREventSchema)` to create a new message.
 */ const UserPREventSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 10)));
/**
 * Describes the message session.v1.GetGitHubAuthStateRequest.
 * Use `create(GetGitHubAuthStateRequestSchema)` to create a new message.
 */ const GetGitHubAuthStateRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 11)));
/**
 * Describes the message session.v1.GetGitHubAuthStateResponse.
 * Use `create(GetGitHubAuthStateResponseSchema)` to create a new message.
 */ const GetGitHubAuthStateResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 12)));
/**
 * Describes the message session.v1.StartGitHubDeviceAuthRequest.
 * Use `create(StartGitHubDeviceAuthRequestSchema)` to create a new message.
 */ const StartGitHubDeviceAuthRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_github_user, 13);
/**
 * Describes the message session.v1.StartGitHubDeviceAuthResponse.
 * Use `create(StartGitHubDeviceAuthResponseSchema)` to create a new message.
 */ const StartGitHubDeviceAuthResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 14)));
/**
 * Describes the message session.v1.PollGitHubDeviceAuthRequest.
 * Use `create(PollGitHubDeviceAuthRequestSchema)` to create a new message.
 */ const PollGitHubDeviceAuthRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_github_user, 15);
/**
 * Describes the message session.v1.PollGitHubDeviceAuthResponse.
 * Use `create(PollGitHubDeviceAuthResponseSchema)` to create a new message.
 */ const PollGitHubDeviceAuthResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 16)));
/**
 * Describes the message session.v1.RevokeGitHubTokenRequest.
 * Use `create(RevokeGitHubTokenRequestSchema)` to create a new message.
 */ const RevokeGitHubTokenRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_github_user, 17);
/**
 * Describes the message session.v1.RevokeGitHubTokenResponse.
 * Use `create(RevokeGitHubTokenResponseSchema)` to create a new message.
 */ const RevokeGitHubTokenResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 18)));
/**
 * Describes the message session.v1.ListGitHubAccountsRequest.
 * Use `create(ListGitHubAccountsRequestSchema)` to create a new message.
 */ const ListGitHubAccountsRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_github_user, 19);
/**
 * Describes the message session.v1.ListGitHubAccountsResponse.
 * Use `create(ListGitHubAccountsResponseSchema)` to create a new message.
 */ const ListGitHubAccountsResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 20)));
/**
 * Describes the message session.v1.AddGitHubAccountWithTokenRequest.
 * Use `create(AddGitHubAccountWithTokenRequestSchema)` to create a new message.
 */ const AddGitHubAccountWithTokenRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_github_user, 21);
/**
 * Describes the message session.v1.AddGitHubAccountWithTokenResponse.
 * Use `create(AddGitHubAccountWithTokenResponseSchema)` to create a new message.
 */ const AddGitHubAccountWithTokenResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 22)));
/**
 * Describes the message session.v1.GitHubCLIHost.
 * Use `create(GitHubCLIHostSchema)` to create a new message.
 */ const GitHubCLIHostSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 23)));
/**
 * Describes the message session.v1.ListGitHubCLIHostsRequest.
 * Use `create(ListGitHubCLIHostsRequestSchema)` to create a new message.
 */ const ListGitHubCLIHostsRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_github_user, 24);
/**
 * Describes the message session.v1.ListGitHubCLIHostsResponse.
 * Use `create(ListGitHubCLIHostsResponseSchema)` to create a new message.
 */ const ListGitHubCLIHostsResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 25)));
/**
 * Describes the message session.v1.AddGitHubAccountFromCLIRequest.
 * Use `create(AddGitHubAccountFromCLIRequestSchema)` to create a new message.
 */ const AddGitHubAccountFromCLIRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_github_user, 26);
/**
 * Describes the message session.v1.PRKey.
 * Use `create(PRKeySchema)` to create a new message.
 */ const PRKeySchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_github_user, 27);
/**
 * Describes the message session.v1.NudgeSessionForPRRequest.
 * Use `create(NudgeSessionForPRRequestSchema)` to create a new message.
 */ const NudgeSessionForPRRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_github_user, 28);
/**
 * Describes the message session.v1.NudgeSessionForPRResponse.
 * Use `create(NudgeSessionForPRResponseSchema)` to create a new message.
 */ const NudgeSessionForPRResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_github_user, 29)));
/**
 * @generated from enum session.v1.AccountPollState
 */ var AccountPollState = /*#__PURE__*/ function(AccountPollState) {
    /**
   * @generated from enum value: ACCOUNT_POLL_STATE_UNSPECIFIED = 0;
   */ AccountPollState[AccountPollState["UNSPECIFIED"] = 0] = "UNSPECIFIED";
    /**
   * @generated from enum value: ACCOUNT_POLL_STATE_OK = 1;
   */ AccountPollState[AccountPollState["OK"] = 1] = "OK";
    /**
   * @generated from enum value: ACCOUNT_POLL_STATE_UNAUTHORIZED = 2;
   */ AccountPollState[AccountPollState["UNAUTHORIZED"] = 2] = "UNAUTHORIZED";
    /**
   * @generated from enum value: ACCOUNT_POLL_STATE_RATE_LIMITED = 3;
   */ AccountPollState[AccountPollState["RATE_LIMITED"] = 3] = "RATE_LIMITED";
    /**
   * @generated from enum value: ACCOUNT_POLL_STATE_ERROR = 4;
   */ AccountPollState[AccountPollState["ERROR"] = 4] = "ERROR";
    return AccountPollState;
}({});
/**
 * Describes the enum session.v1.AccountPollState.
 */ const AccountPollStateSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_session_v1_github_user, 0)));
/**
 * DeviceAuthStatus describes the outcome of a single poll attempt.
 *
 * @generated from enum session.v1.DeviceAuthStatus
 */ var DeviceAuthStatus = /*#__PURE__*/ function(DeviceAuthStatus) {
    /**
   * @generated from enum value: DEVICE_AUTH_STATUS_UNSPECIFIED = 0;
   */ DeviceAuthStatus[DeviceAuthStatus["UNSPECIFIED"] = 0] = "UNSPECIFIED";
    /**
   * waiting for user to authorize
   *
   * @generated from enum value: DEVICE_AUTH_STATUS_PENDING = 1;
   */ DeviceAuthStatus[DeviceAuthStatus["PENDING"] = 1] = "PENDING";
    /**
   * token received and stored in keychain
   *
   * @generated from enum value: DEVICE_AUTH_STATUS_COMPLETE = 2;
   */ DeviceAuthStatus[DeviceAuthStatus["COMPLETE"] = 2] = "COMPLETE";
    /**
   * device code expired; restart the flow
   *
   * @generated from enum value: DEVICE_AUTH_STATUS_EXPIRED = 3;
   */ DeviceAuthStatus[DeviceAuthStatus["EXPIRED"] = 3] = "EXPIRED";
    /**
   * unexpected error
   *
   * @generated from enum value: DEVICE_AUTH_STATUS_ERROR = 4;
   */ DeviceAuthStatus[DeviceAuthStatus["ERROR"] = 4] = "ERROR";
    return DeviceAuthStatus;
}({});
/**
 * Describes the enum session.v1.DeviceAuthStatus.
 */ const DeviceAuthStatusSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_session_v1_github_user, 1)));
/**
 * NudgeOutcome is the typed result of NudgeSessionForPR.
 *
 * @generated from enum session.v1.NudgeOutcome
 */ var NudgeOutcome = /*#__PURE__*/ function(NudgeOutcome) {
    /**
   * @generated from enum value: NUDGE_OUTCOME_UNSPECIFIED = 0;
   */ NudgeOutcome[NudgeOutcome["UNSPECIFIED"] = 0] = "UNSPECIFIED";
    /**
   * @generated from enum value: NUDGE_OUTCOME_DELIVERED = 1;
   */ NudgeOutcome[NudgeOutcome["DELIVERED"] = 1] = "DELIVERED";
    /**
   * @generated from enum value: NUDGE_OUTCOME_BUSY = 2;
   */ NudgeOutcome[NudgeOutcome["BUSY"] = 2] = "BUSY";
    /**
   * @generated from enum value: NUDGE_OUTCOME_PAUSED = 3;
   */ NudgeOutcome[NudgeOutcome["PAUSED"] = 3] = "PAUSED";
    /**
   * @generated from enum value: NUDGE_OUTCOME_DUPLICATE = 4;
   */ NudgeOutcome[NudgeOutcome["DUPLICATE"] = 4] = "DUPLICATE";
    /**
   * @generated from enum value: NUDGE_OUTCOME_NOTHING_TO_FIX = 5;
   */ NudgeOutcome[NudgeOutcome["NOTHING_TO_FIX"] = 5] = "NOTHING_TO_FIX";
    /**
   * @generated from enum value: NUDGE_OUTCOME_SESSION_NOT_LINKED = 6;
   */ NudgeOutcome[NudgeOutcome["SESSION_NOT_LINKED"] = 6] = "SESSION_NOT_LINKED";
    /**
   * @generated from enum value: NUDGE_OUTCOME_PR_NOT_FOUND = 7;
   */ NudgeOutcome[NudgeOutcome["PR_NOT_FOUND"] = 7] = "PR_NOT_FOUND";
    /**
   * The linked session no longer exists or is not running (deleted or crashed),
   * as opposed to PAUSED, which Open session can resume.
   *
   * @generated from enum value: NUDGE_OUTCOME_NOT_RUNNING = 8;
   */ NudgeOutcome[NudgeOutcome["NOT_RUNNING"] = 8] = "NOT_RUNNING";
    return NudgeOutcome;
}({});
/**
 * Describes the enum session.v1.NudgeOutcome.
 */ const NudgeOutcomeSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_session_v1_github_user, 2)));
/**
 * NudgeReason is one kind of problem a nudge prompt covers.
 *
 * @generated from enum session.v1.NudgeReason
 */ var NudgeReason = /*#__PURE__*/ function(NudgeReason) {
    /**
   * @generated from enum value: NUDGE_REASON_UNSPECIFIED = 0;
   */ NudgeReason[NudgeReason["UNSPECIFIED"] = 0] = "UNSPECIFIED";
    /**
   * @generated from enum value: NUDGE_REASON_FAILING_CHECKS = 1;
   */ NudgeReason[NudgeReason["FAILING_CHECKS"] = 1] = "FAILING_CHECKS";
    /**
   * @generated from enum value: NUDGE_REASON_UNRESOLVED_THREADS = 2;
   */ NudgeReason[NudgeReason["UNRESOLVED_THREADS"] = 2] = "UNRESOLVED_THREADS";
    /**
   * @generated from enum value: NUDGE_REASON_MERGE_CONFLICT = 3;
   */ NudgeReason[NudgeReason["MERGE_CONFLICT"] = 3] = "MERGE_CONFLICT";
    return NudgeReason;
}({});
/**
 * Describes the enum session.v1.NudgeReason.
 */ const NudgeReasonSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_session_v1_github_user, 3)));
/**
 * GitHubUserService provides access to the authenticated user's GitHub pull
 * requests across all repos. Data is served from the in-process UserPRCache
 * (which polls the GitHub GraphQL API in the background). No subprocess calls.
 *
 * @generated from service session.v1.GitHubUserService
 */ const GitHubUserService = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_3__/* .serviceDesc */ .Z)(file_session_v1_github_user, 0);


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

/***/ 36796:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 78140));
;
Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 3728));


/***/ }),

/***/ 43860:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   AO: () => (/* binding */ GetWorktreeAISummaryRequestSchema),
/* harmony export */   AQ: () => (/* binding */ WatchUnfinishedWorkRequestSchema),
/* harmony export */   I_: () => (/* binding */ ScanUnfinishedWorkRequestSchema),
/* harmony export */   JC: () => (/* binding */ GetUnfinishedWorkConfigRequestSchema),
/* harmony export */   P7: () => (/* binding */ QuickCommitPushRequestSchema),
/* harmony export */   aC: () => (/* binding */ UnfinishedWorkService),
/* harmony export */   eq: () => (/* binding */ SnoozeWorktreeRequestSchema),
/* harmony export */   gp: () => (/* binding */ DismissWorktreeRequestSchema),
/* harmony export */   wR: () => (/* binding */ UpdateUnfinishedWorkConfigRequestSchema)
/* harmony export */ });
/* unused harmony exports file_session_v1_unfinished, ListUnfinishedWorkRequestSchema, ListUnfinishedWorkResponseSchema, UnfinishedWorkEventSchema, ScanCompletedSchema, ScanUnfinishedWorkResponseSchema, DismissWorktreeResponseSchema, UndismissWorktreeRequestSchema, UndismissWorktreeResponseSchema, SnoozeWorktreeResponseSchema, GetWorktreeAISummaryResponseSchema, QuickCommitPushResponseSchema, GetUnfinishedWorkConfigResponseSchema, UpdateUnfinishedWorkConfigResponseSchema, GetWorktreeDiffRequestSchema, GetWorktreeDiffResponseSchema */
/* harmony import */ var _bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(1844);
/* harmony import */ var _bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(15033);
/* harmony import */ var _bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(95765);
/* harmony import */ var _bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(43963);
/* harmony import */ var _types_pb__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(81432);
// @generated by protoc-gen-es v2.11.0 with parameter "target=ts,ts_nocheck=false,keep_empty_files=true"
// @generated from file session/v1/unfinished.proto (package session.v1, syntax proto3)
/* eslint-disable */ 


/**
 * Describes the file session/v1/unfinished.proto.
 */ const file_session_v1_unfinished = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_1__/* .fileDesc */ .w)("ChtzZXNzaW9uL3YxL3VuZmluaXNoZWQucHJvdG8SCnNlc3Npb24udjEiGwoZTGlzdFVuZmluaXNoZWRXb3JrUmVxdWVzdCJ+ChpMaXN0VW5maW5pc2hlZFdvcmtSZXNwb25zZRIxCgl3b3JrdHJlZXMYASADKAsyHi5zZXNzaW9uLnYxLlVuZmluaXNoZWRXb3JrdHJlZRItCglsYXN0X3NjYW4YAiABKAsyGi5nb29nbGUucHJvdG9idWYuVGltZXN0YW1wIhwKGldhdGNoVW5maW5pc2hlZFdvcmtSZXF1ZXN0Is0BChNVbmZpbmlzaGVkV29ya0V2ZW50EjoKEHdvcmt0cmVlX3VwZGF0ZWQYASABKAsyHi5zZXNzaW9uLnYxLlVuZmluaXNoZWRXb3JrdHJlZUgAEjoKEHdvcmt0cmVlX3JlbW92ZWQYAiABKAsyHi5zZXNzaW9uLnYxLlVuZmluaXNoZWRXb3JrdHJlZUgAEjMKDnNjYW5fY29tcGxldGVkGAMgASgLMhkuc2Vzc2lvbi52MS5TY2FuQ29tcGxldGVkSABCCQoHcGF5bG9hZCJBCg1TY2FuQ29tcGxldGVkEjAKDGNvbXBsZXRlZF9hdBgBIAEoCzIaLmdvb2dsZS5wcm90b2J1Zi5UaW1lc3RhbXAiGwoZU2NhblVuZmluaXNoZWRXb3JrUmVxdWVzdCJRChpTY2FuVW5maW5pc2hlZFdvcmtSZXNwb25zZRIzCg9zY2FuX3N0YXJ0ZWRfYXQYASABKAsyGi5nb29nbGUucHJvdG9idWYuVGltZXN0YW1wIjsKFkRpc21pc3NXb3JrdHJlZVJlcXVlc3QSEQoJcmVwb19wYXRoGAEgASgJEg4KBmJyYW5jaBgCIAEoCSIZChdEaXNtaXNzV29ya3RyZWVSZXNwb25zZSI9ChhVbmRpc21pc3NXb3JrdHJlZVJlcXVlc3QSEQoJcmVwb19wYXRoGAEgASgJEg4KBmJyYW5jaBgCIAEoCSIbChlVbmRpc21pc3NXb3JrdHJlZVJlc3BvbnNlIjoKFVNub296ZVdvcmt0cmVlUmVxdWVzdBIRCglyZXBvX3BhdGgYASABKAkSDgoGYnJhbmNoGAIgASgJIhgKFlNub296ZVdvcmt0cmVlUmVzcG9uc2UiQAobR2V0V29ya3RyZWVBSVN1bW1hcnlSZXF1ZXN0EhEKCXJlcG9fcGF0aBgBIAEoCRIOCgZicmFuY2gYAiABKAkiQwocR2V0V29ya3RyZWVBSVN1bW1hcnlSZXNwb25zZRIPCgdzdW1tYXJ5GAEgASgJEhIKCmZyb21fY2FjaGUYAiABKAgiUwoWUXVpY2tDb21taXRQdXNoUmVxdWVzdBIRCglyZXBvX3BhdGgYASABKAkSDgoGYnJhbmNoGAIgASgJEhYKDmNvbW1pdF9tZXNzYWdlGAMgASgJIkEKF1F1aWNrQ29tbWl0UHVzaFJlc3BvbnNlEg8KB3N1Y2Nlc3MYASABKAgSFQoNZXJyb3JfbWVzc2FnZRgCIAEoCSIgCh5HZXRVbmZpbmlzaGVkV29ya0NvbmZpZ1JlcXVlc3QiUwofR2V0VW5maW5pc2hlZFdvcmtDb25maWdSZXNwb25zZRIwCgZjb25maWcYASABKAsyIC5zZXNzaW9uLnYxLlVuZmluaXNoZWRXb3JrQ29uZmlnIlUKIVVwZGF0ZVVuZmluaXNoZWRXb3JrQ29uZmlnUmVxdWVzdBIwCgZjb25maWcYASABKAsyIC5zZXNzaW9uLnYxLlVuZmluaXNoZWRXb3JrQ29uZmlnIlYKIlVwZGF0ZVVuZmluaXNoZWRXb3JrQ29uZmlnUmVzcG9uc2USMAoGY29uZmlnGAEgASgLMiAuc2Vzc2lvbi52MS5VbmZpbmlzaGVkV29ya0NvbmZpZyI7ChZHZXRXb3JrdHJlZURpZmZSZXF1ZXN0EhEKCXJlcG9fcGF0aBgBIAEoCRIOCgZicmFuY2gYAiABKAkiUwoXR2V0V29ya3RyZWVEaWZmUmVzcG9uc2USKQoKZGlmZl9zdGF0cxgBIAEoCzIVLnNlc3Npb24udjEuRGlmZlN0YXRzEg0KBWVycm9yGAIgASgJMoQJChVVbmZpbmlzaGVkV29ya1NlcnZpY2USZQoSTGlzdFVuZmluaXNoZWRXb3JrEiUuc2Vzc2lvbi52MS5MaXN0VW5maW5pc2hlZFdvcmtSZXF1ZXN0GiYuc2Vzc2lvbi52MS5MaXN0VW5maW5pc2hlZFdvcmtSZXNwb25zZSIAEmIKE1dhdGNoVW5maW5pc2hlZFdvcmsSJi5zZXNzaW9uLnYxLldhdGNoVW5maW5pc2hlZFdvcmtSZXF1ZXN0Gh8uc2Vzc2lvbi52MS5VbmZpbmlzaGVkV29ya0V2ZW50IgAwARJlChJTY2FuVW5maW5pc2hlZFdvcmsSJS5zZXNzaW9uLnYxLlNjYW5VbmZpbmlzaGVkV29ya1JlcXVlc3QaJi5zZXNzaW9uLnYxLlNjYW5VbmZpbmlzaGVkV29ya1Jlc3BvbnNlIgASXAoPRGlzbWlzc1dvcmt0cmVlEiIuc2Vzc2lvbi52MS5EaXNtaXNzV29ya3RyZWVSZXF1ZXN0GiMuc2Vzc2lvbi52MS5EaXNtaXNzV29ya3RyZWVSZXNwb25zZSIAEmIKEVVuZGlzbWlzc1dvcmt0cmVlEiQuc2Vzc2lvbi52MS5VbmRpc21pc3NXb3JrdHJlZVJlcXVlc3QaJS5zZXNzaW9uLnYxLlVuZGlzbWlzc1dvcmt0cmVlUmVzcG9uc2UiABJZCg5Tbm9vemVXb3JrdHJlZRIhLnNlc3Npb24udjEuU25vb3plV29ya3RyZWVSZXF1ZXN0GiIuc2Vzc2lvbi52MS5Tbm9vemVXb3JrdHJlZVJlc3BvbnNlIgASawoUR2V0V29ya3RyZWVBSVN1bW1hcnkSJy5zZXNzaW9uLnYxLkdldFdvcmt0cmVlQUlTdW1tYXJ5UmVxdWVzdBooLnNlc3Npb24udjEuR2V0V29ya3RyZWVBSVN1bW1hcnlSZXNwb25zZSIAElwKD0dldFdvcmt0cmVlRGlmZhIiLnNlc3Npb24udjEuR2V0V29ya3RyZWVEaWZmUmVxdWVzdBojLnNlc3Npb24udjEuR2V0V29ya3RyZWVEaWZmUmVzcG9uc2UiABJcCg9RdWlja0NvbW1pdFB1c2gSIi5zZXNzaW9uLnYxLlF1aWNrQ29tbWl0UHVzaFJlcXVlc3QaIy5zZXNzaW9uLnYxLlF1aWNrQ29tbWl0UHVzaFJlc3BvbnNlIgASdAoXR2V0VW5maW5pc2hlZFdvcmtDb25maWcSKi5zZXNzaW9uLnYxLkdldFVuZmluaXNoZWRXb3JrQ29uZmlnUmVxdWVzdBorLnNlc3Npb24udjEuR2V0VW5maW5pc2hlZFdvcmtDb25maWdSZXNwb25zZSIAEn0KGlVwZGF0ZVVuZmluaXNoZWRXb3JrQ29uZmlnEi0uc2Vzc2lvbi52MS5VcGRhdGVVbmZpbmlzaGVkV29ya0NvbmZpZ1JlcXVlc3QaLi5zZXNzaW9uLnYxLlVwZGF0ZVVuZmluaXNoZWRXb3JrQ29uZmlnUmVzcG9uc2UiAEKvAQoOY29tLnNlc3Npb24udjFCD1VuZmluaXNoZWRQcm90b1ABWkNnaXRodWIuY29tL3RzdGFwbGVyL3N0YXBsZXItc3F1YWQvZ2VuL3Byb3RvL2dvL3Nlc3Npb24vdjE7c2Vzc2lvbnYxogIDU1hYqgIKU2Vzc2lvbi5WMcoCClNlc3Npb25cVjHiAhZTZXNzaW9uXFYxXEdQQk1ldGFkYXRh6gILU2Vzc2lvbjo6VjFiBnByb3RvMw", [
    _bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_2__/* .file_google_protobuf_timestamp */ .i,
    _types_pb__WEBPACK_IMPORTED_MODULE_0__/* .file_session_v1_types */ .m$
]);
/**
 * Describes the message session.v1.ListUnfinishedWorkRequest.
 * Use `create(ListUnfinishedWorkRequestSchema)` to create a new message.
 */ const ListUnfinishedWorkRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_unfinished, 0)));
/**
 * Describes the message session.v1.ListUnfinishedWorkResponse.
 * Use `create(ListUnfinishedWorkResponseSchema)` to create a new message.
 */ const ListUnfinishedWorkResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_unfinished, 1)));
/**
 * Describes the message session.v1.WatchUnfinishedWorkRequest.
 * Use `create(WatchUnfinishedWorkRequestSchema)` to create a new message.
 */ const WatchUnfinishedWorkRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_3__/* .messageDesc */ .Z)(file_session_v1_unfinished, 2);
/**
 * Describes the message session.v1.UnfinishedWorkEvent.
 * Use `create(UnfinishedWorkEventSchema)` to create a new message.
 */ const UnfinishedWorkEventSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_unfinished, 3)));
/**
 * Describes the message session.v1.ScanCompleted.
 * Use `create(ScanCompletedSchema)` to create a new message.
 */ const ScanCompletedSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_unfinished, 4)));
/**
 * Describes the message session.v1.ScanUnfinishedWorkRequest.
 * Use `create(ScanUnfinishedWorkRequestSchema)` to create a new message.
 */ const ScanUnfinishedWorkRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_3__/* .messageDesc */ .Z)(file_session_v1_unfinished, 5);
/**
 * Describes the message session.v1.ScanUnfinishedWorkResponse.
 * Use `create(ScanUnfinishedWorkResponseSchema)` to create a new message.
 */ const ScanUnfinishedWorkResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_unfinished, 6)));
/**
 * Describes the message session.v1.DismissWorktreeRequest.
 * Use `create(DismissWorktreeRequestSchema)` to create a new message.
 */ const DismissWorktreeRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_3__/* .messageDesc */ .Z)(file_session_v1_unfinished, 7);
/**
 * Describes the message session.v1.DismissWorktreeResponse.
 * Use `create(DismissWorktreeResponseSchema)` to create a new message.
 */ const DismissWorktreeResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_unfinished, 8)));
/**
 * Describes the message session.v1.UndismissWorktreeRequest.
 * Use `create(UndismissWorktreeRequestSchema)` to create a new message.
 */ const UndismissWorktreeRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_unfinished, 9)));
/**
 * Describes the message session.v1.UndismissWorktreeResponse.
 * Use `create(UndismissWorktreeResponseSchema)` to create a new message.
 */ const UndismissWorktreeResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_unfinished, 10)));
/**
 * Describes the message session.v1.SnoozeWorktreeRequest.
 * Use `create(SnoozeWorktreeRequestSchema)` to create a new message.
 */ const SnoozeWorktreeRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_3__/* .messageDesc */ .Z)(file_session_v1_unfinished, 11);
/**
 * Describes the message session.v1.SnoozeWorktreeResponse.
 * Use `create(SnoozeWorktreeResponseSchema)` to create a new message.
 */ const SnoozeWorktreeResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_unfinished, 12)));
/**
 * Describes the message session.v1.GetWorktreeAISummaryRequest.
 * Use `create(GetWorktreeAISummaryRequestSchema)` to create a new message.
 */ const GetWorktreeAISummaryRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_3__/* .messageDesc */ .Z)(file_session_v1_unfinished, 13);
/**
 * Describes the message session.v1.GetWorktreeAISummaryResponse.
 * Use `create(GetWorktreeAISummaryResponseSchema)` to create a new message.
 */ const GetWorktreeAISummaryResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_unfinished, 14)));
/**
 * Describes the message session.v1.QuickCommitPushRequest.
 * Use `create(QuickCommitPushRequestSchema)` to create a new message.
 */ const QuickCommitPushRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_3__/* .messageDesc */ .Z)(file_session_v1_unfinished, 15);
/**
 * Describes the message session.v1.QuickCommitPushResponse.
 * Use `create(QuickCommitPushResponseSchema)` to create a new message.
 */ const QuickCommitPushResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_unfinished, 16)));
/**
 * Describes the message session.v1.GetUnfinishedWorkConfigRequest.
 * Use `create(GetUnfinishedWorkConfigRequestSchema)` to create a new message.
 */ const GetUnfinishedWorkConfigRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_3__/* .messageDesc */ .Z)(file_session_v1_unfinished, 17);
/**
 * Describes the message session.v1.GetUnfinishedWorkConfigResponse.
 * Use `create(GetUnfinishedWorkConfigResponseSchema)` to create a new message.
 */ const GetUnfinishedWorkConfigResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_unfinished, 18)));
/**
 * Describes the message session.v1.UpdateUnfinishedWorkConfigRequest.
 * Use `create(UpdateUnfinishedWorkConfigRequestSchema)` to create a new message.
 */ const UpdateUnfinishedWorkConfigRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_3__/* .messageDesc */ .Z)(file_session_v1_unfinished, 19);
/**
 * Describes the message session.v1.UpdateUnfinishedWorkConfigResponse.
 * Use `create(UpdateUnfinishedWorkConfigResponseSchema)` to create a new message.
 */ const UpdateUnfinishedWorkConfigResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_unfinished, 20)));
/**
 * Describes the message session.v1.GetWorktreeDiffRequest.
 * Use `create(GetWorktreeDiffRequestSchema)` to create a new message.
 */ const GetWorktreeDiffRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_unfinished, 21)));
/**
 * Describes the message session.v1.GetWorktreeDiffResponse.
 * Use `create(GetWorktreeDiffResponseSchema)` to create a new message.
 */ const GetWorktreeDiffResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_unfinished, 22)));
/**
 * UnfinishedWorkService manages detection, display, and dismissal of
 * unfinished git worktrees across all configured sources.
 *
 * @generated from service session.v1.UnfinishedWorkService
 */ const UnfinishedWorkService = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_4__/* .serviceDesc */ .Z)(file_session_v1_unfinished, 0);


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

/***/ 70562:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   w: () => (/* binding */ useNowTicker)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* __next_internal_client_entry_do_not_use__ useNowTicker auto */ 
/** Returns `Date.now()`, re-read every `intervalMs`. One call per panel = one shared ticker. */ function useNowTicker() {
    let intervalMs = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : 30000;
    const [now, setNow] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(()=>Date.now());
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        setNow(Date.now());
        const id = setInterval(()=>setNow(Date.now()), intervalMs);
        return ()=>clearInterval(id);
    }, [
        intervalMs
    ]);
    return now;
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

/***/ 78140:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  UnfinishedTab: () => (/* binding */ UnfinishedTab)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/nav/index.ts
var nav = __webpack_require__(41078);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect-web@2.1.1+0149f91553ca585e/node_modules/@connectrpc/connect-web/dist/esm/connect-transport.js + 8 modules
var connect_transport = __webpack_require__(46564);
// EXTERNAL MODULE: ./src/gen/session/v1/unfinished_pb.ts
var unfinished_pb = __webpack_require__(43860);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/create.js
var create = __webpack_require__(95444);
// EXTERNAL MODULE: ./src/lib/config.ts
var config = __webpack_require__(51202);
// EXTERNAL MODULE: ./src/lib/routes.ts + 1 modules
var routes = __webpack_require__(76514);
;// ./src/lib/nav/search-params.ts
// Pure parsers over a query string; no router import so they run under any impl.
function stringParam(params, key) {
    const value = params.get(key);
    return value === null || value === "" ? undefined : value;
}
/** Base-10 integer, or undefined when absent or not a whole number. */ function intParam(params, key) {
    const value = params.get(key);
    if (value === null || !/^-?\d+$/.test(value)) return undefined;
    return Number(value);
}
function enumParam(params, key, allowed) {
    const value = params.get(key);
    return allowed.find((candidate)=>candidate === value);
}

// EXTERNAL MODULE: ./src/lib/hooks/useUnfinishedWork.ts
var useUnfinishedWork = __webpack_require__(82476);
// EXTERNAL MODULE: ./src/gen/session/v1/types_pb.ts
var types_pb = __webpack_require__(81432);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react-dom/index.js
var react_dom = __webpack_require__(80883);
// EXTERNAL MODULE: ./src/lib/hooks/useFocusTrap.ts
var useFocusTrap = __webpack_require__(63991);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Funfinished%2FCommitPushModal.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA8VWTW_iMBC98yt8WYlINXLSQql72X9SOfEEpnXsyHYgsOK_r-KkJQESuKz2BPJM5uO9N2MvPpLlkTnDyJ8ZIaVx6NFoTnKsQb7PCEHtwHPCmv-pyL421lRacrITdk7pR_wGx8QeRdTYJbpSiQMnuYK6ORAKN5qih8JxkoH2YJvjz8p5zA80M9qD9n3TkaKWUHMSMxZymh3YXJk9PXAiKm-as1JIiXrDSWyheJ-dZouuizh0MV7mWygzNVaC5SQua-KMQnnh9dnzolZIrNxloOd1NCjkwpoE6x6l33JSoJ6_rFlZP5E3ttsHUyFqugXcbJvmhcrmbYAdwr401ne2pwYFudtGhJLEQhGNAZKamjo8hlK6ulNT36Sk-aUSLWQt0ZlRVaEby0aUV43EUR_eJMCbG-2bbHDlLUOBwb7vmlu1LGZGGXvpzzoo7AZ1UNg50_OdTGk0GjXuRaWp8d4Uly7JYSjXVJnsq5_-JaTv-IsZ-xVCov7hrKFzQgDJMZqel1g8pMSvB5S4moAiPROSiwLV4arQ6uwxAbSFzgjWYybUlOZ6KPLcZJVrZ7L1uFmmanVdeYUaONFGwzAML5XIYGuUBBui3e42G4h1GTyv9H9D5R1bV1spjApo2Y-6avfkKO23BugxBu-KvbKuabo0-L0s78kHrlTordCuFBa0nxygXsN822ycO4t1ffFNIJ7u0GGqIHz7Q28yUuxABdTkebh34nIgqdf_Bv_NrTZOSSviKciK8bldBlPgqruOz2EIW8SJG2DCJTqRKpAt0KXI0B84YYtlv0htPBVKmT3I4df_jKtWOVwbP_8pMbojJD0Q0npi2pOHaHtcKuzeyn5cS_fmMmahy98FSBRk3jwGurvmdbUu6xajxeBl1uskSO_U84g7j4v7anhj9U57b49vmV71xfqnZ7_T7PQXACTLWjIKAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Funfinished_2FCommitPushModal_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA8VWTW_iMBC98yt8WYlINXLSQql72X9SOfEEpnXsyHYgsOK_r_KkJQESuKz2BPJM5uO9N2MvPpLlkTnDyJ8ZIaVx6NFoTnKsQb7PCEHtwHPCmv_pyL421lRacrITdk7pR_wGx8QeRdTYJbpSiQMnuYK6ORAKN5qih8JxkoH2YJvjz8p5zA80M9qD9n3TkaKWUHMSMxZymh3YXJk9PXAiKm_as1JIiXrDSWyheJ_dZouuizh0MV7mWygzNVaC5SQua_KMQnnh9dnzolZIrNxloOd1NCjkwpoE6x6l33JSoJ6_rFlZP5E3ttsHUyFqugXcbJvmhcrmbYAdwr401ne2pwYFudtGhJLEQhGNAZKamjo8hlK6ulNT36Sk_aUSLWQt0ZlRVaEby0aUV43EUR_eJMCbG_2bbHDlLUOBwb7vmlu1LGZGGXvpzzoo7AZ1UNg50_OdTGk0GjXuRaWp8d4Uly7JYSjXVJnsq5_JaTv_IsZ_xVCov7hrKFzQgDJMZqel1g8pMSvB5S4moAiPROSiwLV4arQ6uwxAbSFzgjWYybUlOZ6KPLcZJVrZ7L1uFmmanVdeYUaONFGwzAML5XIYGuUBBui3e42G4h1GTyv9H9D5R1bV1spjApo2Y_6avfkKO23BugxBu_KvbKuabo0_L0s78kHrlTordCuFBa0nxygXsN822ycO4t1ffFNIJ7u0GGqIHz7Q28yUuxABdTkebh34nIgqdf_Bv_NrTZOSSviKciK8bldBlPgqruOz2EIW8SJG2DCJTqRKpAt0KXI0B84YYtlv0htPBVKmT3I4df_jKtWOVwbP_8pMbojJD0Q0npi2pOHaHtcKuzeyn5cS_fmMmahy98FSBRk3jwGurvmdbUu6xajxeBl1uskSO_U84g7j4v7anhj9U57b49vmV71xfqnZ7_T7PQXACTLWjIKAAA_22_7D = __webpack_require__(80036);
;// ./src/components/unfinished/CommitPushModal.css.ts


var btnCancel = '_25z0so6';
var btnSubmit = '_25z0so7';
var buttonRow = '_25z0so5';
var errorMsg = '_25z0so8';
var label = '_25z0so3';
var modal = '_25z0so1';
var overlay = '_25z0so0';
var CommitPushModal_css_textarea = '_25z0so4';
var title = '_25z0so2';
;// ./src/components/unfinished/CommitPushModal.tsx
/* __next_internal_client_entry_do_not_use__ CommitPushModal auto */ 










/**
 * Modal for staging all changes, entering a commit message, and pushing.
 */ function CommitPushModal(param) {
    let { repoPath, branch, onClose } = param;
    const [message, setMessage] = (0,react.useState)("");
    const [loading, setLoading] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    const dialogRef = (0,react.useRef)(null);
    (0,useFocusTrap/* useFocusTrap */.r)(dialogRef, true);
    const transport = (0,connect_transport/* createConnectTransport */.w)({
        baseUrl: (0,config/* getApiBaseUrl */.h)(),
        interceptors: [
            (0,config/* createAuthInterceptor */._)()
        ]
    });
    const client = (0,promise_client/* createClient */.UU)(unfinished_pb/* UnfinishedWorkService */.aC, transport);
    const handleSubmit = (0,react.useCallback)(async ()=>{
        const trimmed = message.trim();
        if (!trimmed) return;
        setLoading(true);
        setError(null);
        try {
            const req = (0,create/* create */.v)(unfinished_pb/* QuickCommitPushRequestSchema */.P7, {
                repoPath,
                branch,
                commitMessage: trimmed
            });
            const res = await client.quickCommitPush(req);
            if (res.success) {
                onClose();
            } else {
                setError(res.errorMessage || "Operation failed");
            }
        } catch (err) {
            setError(err instanceof Error ? err.message : "Unexpected error");
        } finally{
            setLoading(false);
        }
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        message,
        repoPath,
        branch,
        onClose
    ]);
    const handleKeyDown = (e)=>{
        if (e.key === "Escape") onClose();
        if ((e.metaKey || e.ctrlKey) && e.key === "Enter") handleSubmit();
    };
    const modalContent = /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        ref: dialogRef,
        className: overlay,
        onClick: (e)=>{
            if (e.target === e.currentTarget) onClose();
        },
        onKeyDown: handleKeyDown,
        role: "dialog",
        "aria-modal": "true",
        "aria-labelledby": "commit-push-title",
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: modal,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("h2", {
                    id: "commit-push-title",
                    className: title,
                    children: [
                        "Commit & Push — ",
                        branch
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                            className: label,
                            htmlFor: "commit-message",
                            children: "Commit message"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("textarea", {
                            id: "commit-message",
                            className: CommitPushModal_css_textarea,
                            value: message,
                            onChange: (e)=>setMessage(e.target.value),
                            placeholder: "Describe your changes…",
                            disabled: loading
                        })
                    ]
                }),
                error && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: errorMsg,
                    children: error
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: buttonRow,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: btnCancel,
                            onClick: onClose,
                            disabled: loading,
                            children: "Cancel"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: btnSubmit,
                            onClick: handleSubmit,
                            disabled: !message.trim() || loading,
                            "aria-label": "Stage all, commit, and push",
                            children: loading ? "Pushing…" : "Commit & Push"
                        })
                    ]
                })
            ]
        })
    });
    return /*#__PURE__*/ (0,react_dom.createPortal)(modalContent, document.body);
}

// EXTERNAL MODULE: ./src/components/shared/DiffRenderer.tsx + 1 modules
var DiffRenderer = __webpack_require__(78516);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Funfinished%2FWorktreeDiffModal.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA7VUTY_iMAy991f4shJIE9TCwA6Zy_6TUWgMNZMmURJoYcV_XyWFmVIW2Mte2sp2_fH8_CYfhdkaf6hy-J0BWOMpkNEc1tSifM8ASHsMHPL4vRLl58aZnZYc9sKNGPsolnicuqMYR78kb5U4cFgrbKNBKNpoRgFrz8EHh6Gson2784HWB1YaHVAHDiXqgC66joy0xJZDkeepqBVSkt5wmDmsobg83rNTNrl0X6Tu77e3TO2tjJPoOBS2BW8UyUHUthfFnJC088NEs7e_DxrfTJLDsoOvNGpX6-hpSIaKQ016tMxz277Acr5vUpZatKxC2lQRAKHKUVdrT9hY48LZ9xKRkPtqDAxeHdbpV7NHt1am4VCRlKiv0JgmNB4v4xvwm114K0pkKwwNor7awACLfDy0FH0EVyYEUz-DOyHnK0f6M7GsN8cszbE2OjBPR7ypX3YJor85w7joOFMaZdwwPj-D7jakz4S-hREgYBvYtwOVIuvJp11WFJAlfDho0zhhrxp-fdLwany3uel_b25-cyPBCe2tcKhD_zy00fikz96IxWQ6T-cIUO6cj39YQxdy3aHO9JDSKNL4dQHF8-NbPGPMnFcRnDTpAw7cF4p43r18i26hKunRvZ38sxicsl81ShIwipd_FoafizfbjlOdybUY99BLfDj1Q4pLyK1iX2v2lwIVef6jM_R157HkdPGDpeR96zddTtnpD6_mztVOBgAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Funfinished_2FWorktreeDiffModal_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA7VUTY_iMAy991f4shJIE9TCwA6Zy_6TUWgMNZMmURJoYcV_XyWFmVIW2Mte2sp2_fH8_CYfhdkaf6hy_J0BWOMpkNEc1tSifM8ASHsMHPL4vRLl58aZnZYc9sKNGPsolnicuqMYR78kb5U4cFgrbKNBKNpoRgFrz8EHh6Gson2784HWB1YaHVAHDiXqgC66joy0xJZDkeepqBVSkt5wmDmsobg83rNTNrl0X6Tu77e3TO2tjJPoOBS2BW8UyUHUthfFnJC088NEs7e_DxrfTJLDsoOvNGpX6_hpSIaKQ016tMxz277Acr5vUpZatKxC2lQRAKHKUVdrT9hY48LZ9xKRkPtqDAxeHdbpV7NHt1am4VCRlKiv0JgmNB4v4xvwm114K0pkKwwNor7awACLfDy0FH0EVyYEUz_DOyHnK0f6M7GsN8cszbE2OjBPR7ypX3YJor85w7joOFMaZdwwPj_D7jakz4S_hREgYBvYtwOVIuvJp11WFJAlfDho0zhhrxp_fdLwany3uel_b25_cyPBCe2tcKhD_zy00fikz96IxWQ6T_cIUO6cj39YQxdy3aHO9JDSKNL4dQHF8_NbPGPMnFcRnDTpAw7cF4p43r18i26hKunRvZ38sxicsl81ShIwipd_FoafizfbjlOdybUY99BLfDj1Q4pLyK1iX2v2lwIVef6jM_R157HkdPGDpeR96zddTtnpD6_mztVOBgAA_22_7D = __webpack_require__(5125);
;// ./src/components/unfinished/WorktreeDiffModal.css.ts


var body = '_1ojosyh6';
var closeButton = '_1ojosyh5';
var header = '_1ojosyh2';
var WorktreeDiffModal_css_modal = '_1ojosyh1';
var WorktreeDiffModal_css_overlay = '_1ojosyh0';
var subtitle = '_1ojosyh4';
var WorktreeDiffModal_css_title = '_1ojosyh3';
;// ./src/components/unfinished/WorktreeDiffModal.tsx
/* __next_internal_client_entry_do_not_use__ WorktreeDiffModal auto */ 









function WorktreeDiffModal(param) {
    let { repoPath, branch, repoName, onClose } = param;
    const [content, setContent] = (0,react.useState)("");
    const [added, setAdded] = (0,react.useState)(0);
    const [removed, setRemoved] = (0,react.useState)(0);
    const [loading, setLoading] = (0,react.useState)(true);
    const [fetchError, setFetchError] = (0,react.useState)(null);
    const fetchDiff = (0,react.useCallback)(async ()=>{
        setLoading(true);
        setFetchError(null);
        try {
            const client = (0,promise_client/* createClient */.UU)(unfinished_pb/* UnfinishedWorkService */.aC, (0,connect_transport/* createConnectTransport */.w)({
                baseUrl: (0,config/* getApiBaseUrl */.h)(),
                interceptors: [
                    (0,config/* createAuthInterceptor */._)()
                ]
            }));
            const res = await client.getWorktreeDiff({
                repoPath,
                branch
            });
            if (res.error) {
                setFetchError(res.error);
            } else if (res.diffStats) {
                setContent(res.diffStats.content);
                setAdded(res.diffStats.added);
                setRemoved(res.diffStats.removed);
            } else {
                setContent("");
            }
        } catch (err) {
            setFetchError(err instanceof Error ? err.message : "Failed to load diff");
        } finally{
            setLoading(false);
        }
    }, [
        repoPath,
        branch
    ]);
    (0,react.useEffect)(()=>{
        fetchDiff();
    }, [
        fetchDiff
    ]);
    (0,react.useEffect)(()=>{
        const handler = (e)=>{
            if (e.key === "Escape") onClose();
        };
        document.addEventListener("keydown", handler);
        return ()=>document.removeEventListener("keydown", handler);
    }, [
        onClose
    ]);
    const dialogRef = (0,react.useRef)(null);
    (0,useFocusTrap/* useFocusTrap */.r)(dialogRef, true);
    const modalContent = /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        ref: dialogRef,
        className: WorktreeDiffModal_css_overlay,
        onClick: (e)=>{
            if (e.target === e.currentTarget) onClose();
        },
        role: "dialog",
        "aria-modal": "true",
        "aria-label": "Diff for ".concat(repoName, " / ").concat(branch),
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: WorktreeDiffModal_css_modal,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: header,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            style: {
                                overflow: "hidden"
                            },
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                                    className: WorktreeDiffModal_css_title,
                                    children: [
                                        repoName,
                                        " — ",
                                        branch
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                    className: subtitle,
                                    children: repoPath
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: closeButton,
                            onClick: onClose,
                            "aria-label": "Close diff",
                            children: "✕"
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: body,
                    children: fetchError ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        style: {
                            padding: "2rem",
                            color: "var(--error)",
                            textAlign: "center"
                        },
                        children: fetchError
                    }) : /*#__PURE__*/ (0,jsx_runtime.jsx)(DiffRenderer/* DiffRenderer */.F, {
                        content: content,
                        added: added,
                        removed: removed,
                        loading: loading,
                        onRefresh: fetchDiff
                    })
                })
            ]
        })
    });
    return /*#__PURE__*/ (0,react_dom.createPortal)(modalContent, document.body);
}

// EXTERNAL MODULE: ./src/components/shared/VcsWidget.tsx + 20 modules
var VcsWidget = __webpack_require__(48704);
// EXTERNAL MODULE: ./src/lib/vcs/adapters.ts
var adapters = __webpack_require__(85217);
// EXTERNAL MODULE: ./src/components/shared/GitHubBadge.tsx
var GitHubBadge = __webpack_require__(20621);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Funfinished%2FUnfinishedItemDetail.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61W227iMBB95yvmpRJo1yhJC23dl_2TlRNPgreOHdkOt1X_fWUHUlMSoCuEFJF45sz9jH-94640rEYLe2Mbo1_g7wSgNLoOfwCcYcqW2tQUjHbM4TThWM3eJgAfEwCnR-Uel5Hkx2TeGUiCfM6K98roVnEKa2amhPxOX3Gfmf1zUMi14WgopM0WrJaCf5HCSIo43VBQWmH0LdfO6ZpILB0xjIvWfjX0GFs6yhtRra4oNIxzoaqz03T29hllGqLkwjaS7SiUErdel0lRKSIc1rb7SKxjxvmjijVnkEkwWDNTCXXwcEQmYG2Mx_DPyJPsBk8KVA7NiBfZLg7scRhuSG8_7FkfUCjbQDS9radTW0JJoZD8ZwSjhcv2s-GcHjpjpBmWXXRaOWLFHs_O88_zDfquorBIEv-taI3VhkKjxdHpG9v9ytAUWnrc06M0HIXpFE5oRSMYSOZpZn92it1LnH660ms0V-b1Zdz0aTFpqYvWkrWwIpcYUHXrfEkpZCNxywB-ECO6LC26kKUIeHHFwXrcwUVc50GJOg5hcVNC1GV7Pdoy4FyZhXG6Sa60RHobkZZ3a_UL3ReqtzpMQTpfRll4DlkY1s3idL0OkkEudfHuTWwEdysKKdb-rbfVvR6TMNZmX7fJeC-cpWqRPAQyUqJm3Xgd92gyX9oQOTMgVCmUcBjFw0I8jT5OpUHJnFiHHTZMeb1q_kWV5VbL1gXV0EgFk8U0TZIH-AFPzbarAZaOQiCgPRGK47YnpLsu44s9lOstsSvG9YZC4l2D1NfEVDmbJj_Db552U1kLRY5FfUmaQPl--krplVeCc1RRTopvrbk_rXWi3JFCK4fKUbANK5Dk6DboYT_7Kenq-93dcee90F9uokoFSm-YQeUuMjCAw60jIRM0tEGctps5PlK5G5GT7ITJC6q0m1LJrCPFSkg-6_yKb2iXmayH4t011me5ZLWQu7PKtVfqxMZJLYuvZd1cDVx9Pv4BXYM7sFwLAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Funfinished_2FUnfinishedItemDetail_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61W227iMBB95yvmpRJo1yhJC23dl_2TlRNPgreOHdkOt1X_fWUHUlMSoCuEFJF45sz9jH_94640rEYLe2Mbo1_g7wSgNLoOfwCcYcqW2tQUjHbM4TThWM3eJgAfEwCnR_Uel5Hkx2TeGUiCfM6K98roVnEKa2amhPxOX3Gfmf1zUMi14WgopM0WrJaCf5HCSIo43VBQWmH0LdfO6ZpILB0xjIvWfjX0GFs6yhtRra4oNIxzoaqz03T29hllGqLkwjaS7SiUErdel0lRKSIc1rb7SKxjxvmjijVnkEkwWDNTCXXwcEQmYG2Mx_DPyJPsBk8KVA7NiBfZLg7scRhuSG8_7FkfUCjbQDS9radTW0JJoZD8ZwSjhcv2s_GcHjpjpBmWXXRaOWLFHs_O88_zDfquorBIEv_taI3VhkKjxdHpG9v9ytAUWnrc06M0HIXpFE5oRSMYSOZpZn92it1LnH660ms0V_b1Zdz0aTFpqYvWkrWwIpcYUHXrfEkpZCNxywB_ECO6LC26kKUIeHHFwXrcwUVc50GJOg5hcVNC1GV7Pdoy4FyZhXG6Sa60RHobkZZ3a_UL3ReqtzpMQTpfRll4DlkY1s3idL0OkkEudfHuTWwEdysKKdb_rbfVvR6TMNZmX7fJeC_cpWqRPAQyUqJm3Xgd92gyX9oQOTMgVCmUcBjFw0I8jT5OpUHJnFiHHTZMeb1q_kWV5VbL1gXV0EgFk8U0TZIH_AFPzbarAZaOQiCgPRGK47YnpLsu44s9lOstsSvG9YZC4l2D1NfEVDmbJj_Db552U1kLRY5FfUmaQPl_krplVeCc1RRTopvrbk_rXWi3JFCK4fKUbANK5Dk6DboYT_7Kenq_93dcee90F9uokoFSm_YQeUuMjCAw60jIRM0tEGctps5PlK5G5GT7ITJC6q0m1LJrCPFSkg_6_yKb2iXmayH4t011me5ZLWQu7PKtVfqxMZJLYuvZd1cDVx9Pv4BXYM7sFwLAAA_22_7D = __webpack_require__(93255);
;// ./src/components/unfinished/UnfinishedItemDetail.css.ts


var actionRow = 'zrspro3';
var btn = 'zrspro4';
var btnPrimary = 'zrspro5 zrspro4';
var detail = 'zrspro0';
var sessionBtnWrapper = 'zrsproa';
var sessionPicker = 'zrsprob';
var sessionPickerIdHint = 'zrsprod';
var sessionPickerItem = 'zrsproc';
var spinner = 'zrspro9';
var statItem = 'zrspro2';
var summaryBox = 'zrspro6';
var summaryError = 'zrspro7';
var vcsRow = 'zrspro1';
;// ./src/components/unfinished/UnfinishedItemDetail.tsx
/* __next_internal_client_entry_do_not_use__ UnfinishedItemDetail auto */ 















/**
 * Expanded accordion detail panel for an UnfinishedItem.
 * Shows diff stats, commit messages, and action buttons.
 */ function UnfinishedItemDetail(param) {
    let { worktree } = param;
    const router = (0,nav/* useRouter */.KA)();
    const [aiSummary, setAiSummary] = (0,react.useState)(null);
    const [aiLoading, setAiLoading] = (0,react.useState)(false);
    const [aiError, setAiError] = (0,react.useState)(false);
    const [showCommitModal, setShowCommitModal] = (0,react.useState)(false);
    const [showDiffModal, setShowDiffModal] = (0,react.useState)(false);
    const [showSessionPicker, setShowSessionPicker] = (0,react.useState)(false);
    const pickerRef = (0,react.useRef)(null);
    // Close picker on outside click
    (0,react.useEffect)(()=>{
        if (!showSessionPicker) return;
        function handleClick(e) {
            if (pickerRef.current && !pickerRef.current.contains(e.target)) {
                setShowSessionPicker(false);
            }
        }
        document.addEventListener("mousedown", handleClick);
        return ()=>document.removeEventListener("mousedown", handleClick);
    }, [
        showSessionPicker
    ]);
    const transport = (0,connect_transport/* createConnectTransport */.w)({
        baseUrl: (0,config/* getApiBaseUrl */.h)(),
        interceptors: [
            (0,config/* createAuthInterceptor */._)()
        ]
    });
    const client = (0,promise_client/* createClient */.UU)(unfinished_pb/* UnfinishedWorkService */.aC, transport);
    const handleOpenSession = (0,react.useCallback)(()=>{
        if (worktree.sessionIds.length > 1) {
            setShowSessionPicker((v)=>!v);
        } else if (worktree.sessionIds.length === 1) {
            router.push(routes/* routes */.J.sessionDetail(worktree.sessionIds[0]));
        } else {
            router.push(routes/* routes */.J.newSessionFromWorktree(worktree.worktreePath, worktree.branch, worktree.branch));
        }
    }, [
        router,
        worktree
    ]);
    const handleSummarize = (0,react.useCallback)(async ()=>{
        if (aiSummary !== null) return; // already cached
        setAiLoading(true);
        setAiError(false);
        try {
            const req = (0,create/* create */.v)(unfinished_pb/* GetWorktreeAISummaryRequestSchema */.AO, {
                repoPath: worktree.repoPath,
                branch: worktree.branch
            });
            const res = await client.getWorktreeAISummary(req);
            setAiSummary(res.summary);
        } catch (e) {
            setAiError(true);
        } finally{
            setAiLoading(false);
        }
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        worktree.repoPath,
        worktree.branch,
        aiSummary
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: detail,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: vcsRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(VcsWidget/* VcsWidget */.D, {
                        data: (0,adapters/* fromUnfinishedWorktree */.FM)(worktree),
                        mode: "compact"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(GitHubBadge/* GitHubBadge */.J, {
                        prNumber: worktree.githubPrNumber,
                        prUrl: worktree.githubPrUrl,
                        prState: worktree.githubPrState,
                        prPriority: worktree.githubPrPriority,
                        compact: true
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: actionRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: sessionBtnWrapper,
                        ref: pickerRef,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: btnPrimary,
                                onClick: handleOpenSession,
                                "aria-expanded": showSessionPicker,
                                children: worktree.sessionIds.length > 1 ? "Reattach Session (".concat(worktree.sessionIds.length, ")") : worktree.sessionIds.length === 1 ? "Reattach Session" : "Open Session"
                            }),
                            showSessionPicker && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: sessionPicker,
                                role: "listbox",
                                "aria-label": "Select session",
                                children: worktree.sessionIds.map((id, i)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                        className: sessionPickerItem,
                                        role: "option",
                                        "aria-selected": false,
                                        onClick: ()=>{
                                            setShowSessionPicker(false);
                                            router.push(routes/* routes */.J.sessionDetail(id));
                                        },
                                        children: [
                                            "Session ",
                                            i + 1,
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: sessionPickerIdHint,
                                                children: id.slice(0, 8)
                                            })
                                        ]
                                    }, id))
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: btn,
                        onClick: ()=>setShowDiffModal(true),
                        children: "View Diff"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: btn,
                        onClick: ()=>setShowCommitModal(true),
                        children: "Commit & Push"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: btn,
                        onClick: handleSummarize,
                        disabled: aiLoading,
                        "aria-label": "Generate AI summary of changes",
                        children: aiLoading ? "Summarizing…" : "Summarize"
                    })
                ]
            }),
            aiLoading && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: summaryBox,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: spinner,
                        "aria-label": "Generating summary"
                    }),
                    " Generating summary…"
                ]
            }),
            aiError && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: "".concat(summaryBox, " ").concat(summaryError),
                children: "Summary unavailable — try again."
            }),
            aiSummary !== null && !aiLoading && !aiError && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: summaryBox,
                children: aiSummary
            }),
            showDiffModal && /*#__PURE__*/ (0,jsx_runtime.jsx)(WorktreeDiffModal, {
                repoPath: worktree.repoPath,
                branch: worktree.branch,
                repoName: worktree.repoName || worktree.branch,
                onClose: ()=>setShowDiffModal(false)
            }),
            showCommitModal && /*#__PURE__*/ (0,jsx_runtime.jsx)(CommitPushModal, {
                repoPath: worktree.repoPath,
                branch: worktree.branch,
                onClose: ()=>setShowCommitModal(false)
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Funfinished%2FUnfinishedItem.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61W226jMBB9z1f4sZHiCNNN2tKX_ZPKwQZGNTayTUKyyr-vbCjhFkLVKhJSPOM54zlz236Qc7UjTATo3wqhA40_U61KySJ0pPoJ4w_yxi-hvrys351cacZ1hEhRIaMEsIEW72hhTRmUZmjoubZUUMZApiNpsB6eEK8fl9ooHaFCgbRcuyOrqTRgQcmo4zgKtmRnNl9OxEooXZ-5O6q0AiSPkFSSv6-uq20bgChTR64fhOG1-0Bve6gB64HZRMWlwUcwcBC8Nj9zXTQAFTYZZeoUocD_wqKa0OwAka7pg7JW5VjwxLY0BO8juYY0m1OYCFPoYRiYQtBzhBLBK3eNCkglBstzE6GYf1GU0mKCYSdxF_FJO7n79jCePUaipMUJzUGchzbCsrbhNAxc-AjjUOfMVICD29UTd--P0D4IWpdMpkF--mB0PPrzY4_ofY_CW0BS7Rgn7n8OEp-A2awhxiVnIpw4A8a49BXAK4tvAi4EFAZ8op8ysBybgsY-2Ucx3k3zOEFYeFnPMDzH476PAdKVHn6QMm1jGGd848l8e9kvImKKf-9d1pyR7a73lpcHjeF8n9_LotZZ9cv59QHe8T5euQiv7OO9PcAjL_cB80WAeR-QPgIk9wFJuAiRBH3Iw3dzPqc6BenbaIRoaZUvxILGYM9NWXaHUCP5mjYTw-XmygaNRsQJbAYSDdxt4UjPYjwKn_ekoJpL-1ujer9eVpE_aHsTc31ch3OzPjQb1A75cGLwh30q4m_M-ZkB8o0VIJ5YAdo9JLxDT70HNGpYJYnh1pPZM80WPWZxKU0_pymjvzlnQNGTR6w3g7UHHiRsP2URuq6u_wFnvyvDZgoAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Funfinished_2FUnfinishedItem_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61W226jMBB9z1f4sZHiCNNN2tKX_ZPKwQZGNTayTUKyyr_vbCjhFkLVKhJSPOM54zlz236Qc7UjTATo3wqhA40_U61KySJ0pPoJ4w_yxi_hvrys351cacZ1hEhRIaMEsIEW72hhTRmUZmjoubZUUMZApiNpsB6eEK8fl9ooHaFCgbRcuyOrqTRgQcmo4zgKtmRnNl9OxEooXZ_5O6q0AiSPkFSSv6_uq20bgChTR64fhOG1_0Bve6gB64HZRMWlwUcwcBC8Nj9zXTQAFTYZZeoUocD_wqKa0OwAka7pg7JW5VjwxLY0BO8juYY0m1OYCFPoYRiYQtBzhBLBK3eNCkglBstzE6GYf1GU0mKCYSdxF_FJO7n79jCePUaipMUJzUGchzbCsrbhNAxc_AjjUOfMVICD29UTd_P0D4IWpdMpkF_mB0PPrzY4_ofY_CW0BS7Rgn7n8OEp_A2awhxiVnIpw4A8a49BXAK4tvAi4EFAZ8op8ysBybgsY_2Ucx3k3zOEFYeFnPMDzH476PAdKVHn6QMm1jGGd848l8e9kvImKKf_9d1pyR7a73lpcHjeF8n9_LotZZ9cv59QHe8T5euQiv7OO9PcAjL_cB80WAeR_QPgIk9wFJuAiRBH3Iw3dzPqc6BenbaIRoaZUvxILGYM9NWXaHUCP5mjYTw_XmygaNRsQJbAYSDdxt4UjPYjwKn_ekoJpL_1ujer9eVpE_aHsTc31ch3OzPjQb1A75cGLwh30q4m_M_ZkB8o0VIJ5YAdo9JLxDT70HNGpYJYnh1pPZM80WPWZxKU0_pymjvzlnQNGTR6w3g7UHHiRsP2URuq6u_wFnvyvDZgoAAA_22_7D = __webpack_require__(21774);
;// ./src/components/unfinished/UnfinishedItem.css.ts


var actionBtn = '_1yx51dlc';
var actions = '_1yx51dlb';
var branch = '_1yx51dl3';
var card = '_1yx51dl0';
var cardExpanded = '_1yx51dl1';
var chip = '_1yx51dl6';
var chipAhead = '_1yx51dl8 _1yx51dl6';
var chipBehind = '_1yx51dl9 _1yx51dl6';
var chipTimeout = '_1yx51dla _1yx51dl6';
var chipUncommitted = '_1yx51dl7 _1yx51dl6';
var chips = '_1yx51dl5';
var dismissBtn = '_1yx51dld _1yx51dlc';
var UnfinishedItem_css_header = '_1yx51dl2';
var path = '_1yx51dl4';
;// ./src/components/unfinished/UnfinishedItem.tsx
/* __next_internal_client_entry_do_not_use__ UnfinishedItem auto */ 




/**
 * Card representing a single git worktree with unfinished work.
 * Shows branch name, abbreviated path, status chips, and hover-reveal action buttons.
 */ function UnfinishedItem(param) {
    let { worktree, isExpanded, onToggleExpand, onDismiss, onSnooze } = param;
    const isTimeout = worktree.scanStatus === types_pb/* ScanStatus */.Sd.TIMEOUT || worktree.scanStatus === types_pb/* ScanStatus */.Sd.ERROR;
    const handleKeyDown = (0,react.useCallback)((e)=>{
        if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            onToggleExpand();
        }
        if (e.key === "Escape" && isExpanded) {
            onToggleExpand();
        }
    }, [
        isExpanded,
        onToggleExpand
    ]);
    const handleDismissClick = (e)=>{
        e.stopPropagation();
        onDismiss(worktree.repoPath, worktree.branch);
    };
    const handleSnoozeClick = (e)=>{
        e.stopPropagation();
        onSnooze(worktree.repoPath, worktree.branch);
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                role: "button",
                tabIndex: 0,
                "aria-expanded": isExpanded,
                className: "".concat(card, " ").concat(isExpanded ? cardExpanded : ""),
                onClick: onToggleExpand,
                onKeyDown: handleKeyDown,
                "data-testid": "unfinished-item",
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: UnfinishedItem_css_header,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: branch,
                            children: worktree.branch
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: path,
                            children: worktree.displayPath || worktree.worktreePath
                        }),
                        isTimeout ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: chips,
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: chipTimeout,
                                "aria-label": "scan timed out",
                                title: worktree.scanErrorMsg || "Scan timed out",
                                children: "⚠ Timeout"
                            })
                        }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: chips,
                            children: [
                                worktree.hasUncommitted && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: chipUncommitted,
                                    children: "Uncommitted"
                                }),
                                worktree.commitsAhead > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    className: chipAhead,
                                    children: [
                                        "↑",
                                        worktree.commitsAhead
                                    ]
                                }),
                                worktree.commitsBehind > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    className: chipBehind,
                                    children: [
                                        "↓",
                                        worktree.commitsBehind
                                    ]
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: actions,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: dismissBtn,
                                    onClick: handleDismissClick,
                                    "aria-label": "Dismiss this worktree",
                                    title: "Dismiss",
                                    children: "\xd7"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: actionBtn,
                                    onClick: handleSnoozeClick,
                                    "aria-label": "Snooze until next change",
                                    title: "Snooze until next git change",
                                    children: "Snooze"
                                })
                            ]
                        })
                    ]
                })
            }),
            isExpanded && /*#__PURE__*/ (0,jsx_runtime.jsx)(UnfinishedItemDetail, {
                worktree: worktree
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Funfinished%2FUnfinishedRepoGroup.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA6VT227CMAx95yvyuD0EtTDYCB8zpYlbLNK4clJuE_8-NRWMFpgmTX1IZec45xzb08-cF9G8N5n4mghRa67Qy4JipFqJneYXKT_zFZxmfJrnr-vJeTK9QPIEsRgap49KlA4O64kQ2mHlJUaogxIGfATuwpVuxgVnp9cu02hr0VcPsmMCWbpfEFtgydpiG-5ILtMd03IgVqIhvBBoA7AM4MBEJTx56ILURoceLoFbdWpDO-CksdBmWzG13o6f-xhZokoybZA7DFg46MF0kGGjLe2VyNI3aw6jMm5YZpaAJfkoA57gTqPuNZLrJI5cS6nI2geMSF71_yVxLbJpvgjr256h79TLwpHZDhjME4MrVAmmqCO8rDIL1ZDs2w_ZPWC1iUoss2z9q4DiuYC-xwla6hrd8U5hO3x_cTu5Dsr4ZMz-Yefz_ps_TORqOOX33U8EbxQtHy9Wd0qLDKZvrCHX1v5Pu_XQl7TPV-ciPa5x_gZZtQEIIwQAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Funfinished_2FUnfinishedRepoGroup_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA6VT227CMAx95yvyuD0EtTDYCB8zpYlbLNK4clJuE_8_NRWMFpgmTX1IZec45xzb08_cF9G8N5n4mghRa67Qy4JipFqJneYXKT_zFZxmfJrnr_vJeTK9QPIEsRgap49KlA4O64kQ2mHlJUaogxIGfATuwpVuxgVnp9cu02hr0VcPsmMCWbpfEFtgydpiG_5ILtMd03IgVqIhvBBoA7AM4MBEJTx56ILURoceLoFbdWpDO_CksdBmWzG13o6f_xhZokoybZA7DFg46MF0kGGjLe2VyNI3aw6jMm5YZpaAJfkoA57gTqPuNZLrJI5cS6nI2geMSF71_yVxLbJpvgjr256h79TLwpHZDhjME4MrVAmmqCO8rDIL1ZDs2w_ZPWC1iUoss2z9q4DiuYC_xwla6hrd8U5hO3x_cTu5Dsr4ZMz_Yefz_ps_TORqOOX33U8EbxQtHy9Wd0qLDKZvrCHX1v5Pu_XQl7TPV_ciPa5x_gZZtQEIIwQAAA_22_7D = __webpack_require__(37498);
;// ./src/components/unfinished/UnfinishedRepoGroup.css.ts


var chevron = '_1r5tc7p2';
var chevronExpanded = '_1r5tc7p3';
var count = '_1r5tc7p5';
var group = '_1r5tc7p0';
var UnfinishedRepoGroup_css_header = '_1r5tc7p1';
var itemList = '_1r5tc7p6';
var UnfinishedRepoGroup_css_repoName = '_1r5tc7p4';
;// ./src/components/unfinished/UnfinishedRepoGroup.tsx
/* __next_internal_client_entry_do_not_use__ UnfinishedRepoGroup auto */ 



/**
 * Collapsible group showing all worktrees for a single repository.
 */ function UnfinishedRepoGroup(param) {
    let { repoName, worktrees, onDismiss, onSnooze } = param;
    const [isOpen, setIsOpen] = (0,react.useState)(true);
    const [expandedKey, setExpandedKey] = (0,react.useState)(null);
    const toggleGroup = (0,react.useCallback)(()=>setIsOpen((v)=>!v), []);
    const handleKeyDown = (e)=>{
        if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            toggleGroup();
        }
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
        className: group,
        "aria-label": "Repository: ".concat(repoName),
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                role: "button",
                tabIndex: 0,
                className: UnfinishedRepoGroup_css_header,
                onClick: toggleGroup,
                onKeyDown: handleKeyDown,
                "aria-expanded": isOpen,
                "aria-controls": "repo-group-".concat(repoName),
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: "".concat(chevron, " ").concat(isOpen ? chevronExpanded : ""),
                        "aria-hidden": "true",
                        children: "▶"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: UnfinishedRepoGroup_css_repoName,
                        children: repoName
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: count,
                        children: worktrees.length
                    })
                ]
            }),
            isOpen && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                id: "repo-group-".concat(repoName),
                className: itemList,
                children: worktrees.map((wt)=>{
                    const key = "".concat(wt.repoPath, "|").concat(wt.branch);
                    return /*#__PURE__*/ (0,jsx_runtime.jsx)(UnfinishedItem, {
                        worktree: wt,
                        isExpanded: expandedKey === key,
                        onToggleExpand: ()=>setExpandedKey((prev)=>prev === key ? null : key),
                        onDismiss: onDismiss,
                        onSnooze: onSnooze
                    }, key);
                })
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/hooks/useNowTicker.ts
var useNowTicker = __webpack_require__(70562);
;// ./src/lib/unfinished/prAttention.ts
// Rollup states come from normalizeCheckState (github/user_pr_cache.go), which
// folds FAILURE and ERROR into "failure"; pending is deliberately not failing.
// Per-check failure is whatever the server itemised (mapFailingChecks), so the
// button never promises more than the nudge prompt will contain.
const ROLLUP_FAILING = "failure";
function prAttention(pr) {
    const failingChecks = pr.failingChecks.length;
    const checksFailingUnlisted = failingChecks === 0 && pr.checkConclusion === ROLLUP_FAILING;
    const changesRequested = pr.changesReqCount > 0;
    const threadsUnknown = pr.unresolvedThreadCount === undefined;
    var _pr_unresolvedThreadCount;
    const unresolvedThreads = (_pr_unresolvedThreadCount = pr.unresolvedThreadCount) !== null && _pr_unresolvedThreadCount !== void 0 ? _pr_unresolvedThreadCount : 0;
    const conflictUnknown = pr.hasMergeConflict === undefined;
    const mergeConflict = pr.hasMergeConflict === true;
    const nudgeable = !pr.isDraft && (failingChecks > 0 || unresolvedThreads > 0 || mergeConflict);
    const needsAttention = !pr.isDraft && (nudgeable || changesRequested || checksFailingUnlisted);
    return {
        failingChecks,
        changesRequested,
        unresolvedThreads,
        mergeConflict,
        threadsUnknown,
        conflictUnknown,
        needsAttention,
        nudgeable,
        checksFailingUnlisted
    };
}
const DEGRADED_ATTENTION_TEXT = "Review-thread counts are not loaded yet, so the real number may be higher.";
function summarizeAttention(prs) {
    return {
        count: prs.filter((pr)=>prAttention(pr).needsAttention).length,
        degraded: prs.some((pr)=>pr.detailsLoaded === false)
    };
}

;// ./src/lib/unfinished/relativeTime.ts
/** "just now" under 60 s, then "N min ago" / "N h ago" / "N d ago". Pure; callers inject `now`. */ function formatRelativeTime(thenMs, nowMs) {
    const seconds = Math.max(0, Math.floor((nowMs - thenMs) / 1000));
    if (seconds < 60) return "just now";
    const minutes = Math.floor(seconds / 60);
    if (minutes < 60) return "".concat(minutes, " min ago");
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return "".concat(hours, " h ago");
    return "".concat(Math.floor(hours / 24), " d ago");
}

;// ./src/lib/unfinished/tabStats.ts
const TAB_STATS_STORAGE_KEY = "up-next-tab-stats";
const LEFT_PRS_WINDOW_MS = 5000;
function emptyTabStats() {
    return {
        visits: 0,
        landed: {
            prs: 0,
            stuck: 0,
            worktrees: 0,
            queue: 0
        },
        leftPrsWithin5s: 0,
        firstPrCardMs: null,
        openSessionClicks: 0
    };
}
/** Pure reducer; counters only, never PR content. */ function recordTabEvent(stats, event) {
    switch(event.type){
        case "visit":
            return {
                ...stats,
                visits: stats.visits + 1,
                landed: {
                    ...stats.landed,
                    [event.landedOn]: stats.landed[event.landedOn] + 1
                }
            };
        case "click":
            {
                const leftPrsQuickly = event.firstClickOfVisit && event.from === "prs" && event.to !== "prs" && event.msSinceVisit <= LEFT_PRS_WINDOW_MS;
                return leftPrsQuickly ? {
                    ...stats,
                    leftPrsWithin5s: stats.leftPrsWithin5s + 1
                } : stats;
            }
        case "firstPrCard":
            return {
                ...stats,
                firstPrCardMs: event.ms
            };
        case "openSession":
            return {
                ...stats,
                openSessionClicks: stats.openSessionClicks + 1
            };
    }
}
function readStats() {
    try {
        var _window_localStorage_getItem;
        const parsed = JSON.parse((_window_localStorage_getItem = window.localStorage.getItem(TAB_STATS_STORAGE_KEY)) !== null && _window_localStorage_getItem !== void 0 ? _window_localStorage_getItem : "null");
        if (parsed && typeof parsed === "object") {
            const base = emptyTabStats();
            var _parsed_landed;
            return {
                visits: Number(parsed.visits) || 0,
                landed: {
                    ...base.landed,
                    ...(_parsed_landed = parsed.landed) !== null && _parsed_landed !== void 0 ? _parsed_landed : {}
                },
                leftPrsWithin5s: Number(parsed.leftPrsWithin5s) || 0,
                firstPrCardMs: typeof parsed.firstPrCardMs === "number" ? parsed.firstPrCardMs : null,
                openSessionClicks: Number(parsed.openSessionClicks) || 0
            };
        }
    } catch (e) {
    // Missing, malformed or blocked storage: start from zero.
    }
    return emptyTabStats();
}
function writeStats(stats) {
    try {
        window.localStorage.setItem(TAB_STATS_STORAGE_KEY, JSON.stringify(stats));
    } catch (e) {
    // Metrics are best-effort.
    }
}
/** Read-modify-write convenience for call sites outside the pure reducer. */ function emitTabEvent(event) {
    writeStats(recordTabEvent(readStats(), event));
}

// EXTERNAL MODULE: ./src/gen/session/v1/github_user_pb.ts
var github_user_pb = __webpack_require__(13798);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2FcollapsibleSection.css.ts.vanilla.css%22%2C%22source%22%3A%22Lnc4cDF6MDAgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MzApOwp9Ci53OHAxejAxIHsKICBkaXNwbGF5OiBmbGV4OwogIGFsaWduLWl0ZW1zOiBjZW50ZXI7CiAgZ2FwOiB2YXIoLS1fMTllejJyejJ6KTsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejJ6KSB2YXIoLS1fMTllejJyejMwKTsKICBib3JkZXItcmFkaXVzOiB2YXIoLS1fMTllejJyejM2KTsKICBjdXJzb3I6IHBvaW50ZXI7CiAgdXNlci1zZWxlY3Q6IG5vbmU7CiAgb3V0bGluZTogbm9uZTsKfQoudzhwMXowMTpob3ZlciB7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycno4KTsKfQoudzhwMXowMTpmb2N1cy12aXNpYmxlIHsKICBib3gtc2hhZG93OiAwIDAgMCAycHggdmFyKC0tXzE5ZXoycnpsKTsKfQoudzhwMXowMiB7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNhKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKICB0cmFuc2l0aW9uOiB0cmFuc2Zvcm0gMC4xNXM7CiAgZGlzcGxheTogaW5saW5lLWJsb2NrOwp9Ci53OHAxejAzIHsKICB0cmFuc2Zvcm06IHJvdGF0ZSg5MGRlZyk7Cn0KLnc4cDF6MDQgewogIGZvbnQtd2VpZ2h0OiA2MDA7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnowKTsKfQoudzhwMXowNSB7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNhKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyemMpOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzkpOwogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MnkpIHZhcigtLV8xOWV6MnJ6MnopOwp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2FcollapsibleSection_css_ts_vanilla_css_22_2C_22source_22_3A_22Lnc4cDF6MDAgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MzApOwp9Ci53OHAxejAxIHsKICBkaXNwbGF5OiBmbGV4OwogIGFsaWduLWl0ZW1zOiBjZW50ZXI7CiAgZ2FwOiB2YXIoLS1fMTllejJyejJ6KTsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejJ6KSB2YXIoLS1fMTllejJyejMwKTsKICBib3JkZXItcmFkaXVzOiB2YXIoLS1fMTllejJyejM2KTsKICBjdXJzb3I6IHBvaW50ZXI7CiAgdXNlci1zZWxlY3Q6IG5vbmU7CiAgb3V0bGluZTogbm9uZTsKfQoudzhwMXowMTpob3ZlciB7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycno4KTsKfQoudzhwMXowMTpmb2N1cy12aXNpYmxlIHsKICBib3gtc2hhZG93OiAwIDAgMCAycHggdmFyKC0tXzE5ZXoycnpsKTsKfQoudzhwMXowMiB7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNhKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKICB0cmFuc2l0aW9uOiB0cmFuc2Zvcm0gMC4xNXM7CiAgZGlzcGxheTogaW5saW5lLWJsb2NrOwp9Ci53OHAxejAzIHsKICB0cmFuc2Zvcm06IHJvdGF0ZSg5MGRlZyk7Cn0KLnc4cDF6MDQgewogIGZvbnQtd2VpZ2h0OiA2MDA7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnowKTsKfQoudzhwMXowNSB7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNhKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyemMpOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzkpOwogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MnkpIHZhcigtLV8xOWV6MnJ6MnopOwp9_22_7D = __webpack_require__(63255);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Funfinished%2FGitHubPRsSection.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA91aW2-jOhB-769AKx3p9IEKk7RpqfaXrI5WDhjw1tgc2ySQo_3vR0Au2NzshOxWq0p9iC_j-Wbmm_GYp-9gs1_xbeY5_z04TgZ5gqlLUCwDZwf53677Hbyhg88P_uHx_eHnw9NpAWgW5DCKME30ySvw2P_l_cFxQkYY723dDMWMSlfgA-pttlUl-5OSPRvJh2ZoC8OPhLOCRvp41Y4zHiEeOCAvHcEIjrRZZWeWy2GEC9E71sZAx0n8GzU6KKwaFCIscgKrwIkJKhsJBJVuhDkKJWY0qNUuMlqPJDAfAOv9Cih_HV4WeKwbPOYAHvWCzk7Pw8hCghPqYokyETghohLxZcHbGYFXLAbelWgVl6V7hJNUBs6L5ykIvkxFqH_oIePNIJONn-ZZAY0yiubheTFSvq-h44QFF_UZcoZP9m_dQiASt67iCgm5VOEIUrZDvAFlXEmqeuFmEsPKAEPJIRU55IjKGeqd8zlk4HNzoMLHQQC7GndhamUNnhmPe4PGCa_358ixRDdu581ikFtz5Nv1HAk6sgJHDXdoRZhDGB_pqLbLntfj9X9Fxvb6o7fsIVEp3QiFjMPW6i1XdEV0_K83u6AR4gRrS8LLqWKYYVL14quYCYuwzzWblmsIkhJxV-QwbBzPe_JRNqrkHIWGCwe6HTF1IItmDAnN6kRZERQ4WEKCQ2V_dKsz-odhZ7TRWYu7WD0TprUjuVccrVKx9_Ny-PDjfgDM6IcYOMKbGeMPWZJclk7GzT7FEjUhgOqA7bFC8nmANQuw5L64-gvhmupB6j29Prfsw2o-klX900ZZg9WCZRzCBSAyKzbmcZgzp0mR8eMaH_xRCInjyg0ZlYjK7tBECaeXucfjYZoijuVIgXq2SJtV6sOlx0wDTFDsGPxFVbyTL8-TgDLlw9Qn5nWu3SWCIkW_JaQGcJ2LoQ_DcnbicpOpnkYWKGd1puvfzdTaLrstXXd2ohOnV_PsWFLubMaucKzPmwiv8C4WRFjALUGRGn9NkF52jFAMC6JeR_NbK7DRFPDvQhcBu0JrGffkN9eNlbqhuMCsVfYmWUXardbCo7CUrWGx-yw19DnytkxKlo2Hady9Cp8mT9tnb6WkIU1pMsq2q47pOeWu13lpWGouwDmKBzyP9rAMS7KrekuDRKbXIVplUS7eCTrMNN-UbDAsjBhdZ8ZaosC7l7tpHSBYSKZKBrfy_VwuAv7d3Nys0Lf336GW3ETfayBmWCFrHx7oIwHfwn3VdTELC-HusMBbgo7rS1ekMGL7wPGavz6SRNtnNWqODJbuHkcyDRzgeX_NWOjUU_4EFgITTtqeUrOH45z0XHu17go-QZATGKKUkehoJpMKAaxa-8zataWDo3zf81rsG8rEbYnejNUdPaFKWC9R4R9s28LA7sXLIO-B6Reg0deLPzjlKegsnt_A629PcG-dLwhqrupZeKRGXKB2B3YvEGbJdfrhZ-AVUzfJVgVkWtmJN8cxzlNkDaSOMyP6k3fb4zSXxbFAspmt7h3eWj8M1CdtejpSZKNmHRAxYfujYSCt9iniWmaN5gsNU6qxjGxgk8aW6l3OvT4r0NzRA6JvkGPonjoOX79IXqAv_7SZ83QozhKOhJhsEAC71xmL3tAl0AwvltZfJyzwec_dv0EyCqJ4vOIB_lwdZhYHQOen5JcTtN0naCbfw_z8H0kye10tJwAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Funfinished_2FGitHubPRsSection_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA91aW2_jOhB_769AKx3p9IEKk7RpqfaXrI5WDhjw1tgc2ySQo_3vR0Au2NzshOxWq0p9iC_j_Wbmm_GYp_9gs1_xbeY5_z04TgZ5gqlLUCwDZwf53677Hbyhg88P_uHx_eHnw9NpAWgW5DCKME30ySvw2P_l_cFxQkYY723dDMWMSlfgA_pttlUl_5OSPRvJh2ZoC8OPhLOCRvp41Y4zHiEeOCAvHcEIjrRZZWeWy2GEC9E71sZAx0n8GzU6KKwaFCIscgKrwIkJKhsJBJVuhDkKJWY0qNUuMlqPJDAfAOv9Cih_HV4WeKwbPOYAHvWCzk7Pw8hCghPqYokyETghohLxZcHbGYFXLAbelWgVl6V7hJNUBs6L5ykIvkxFqH_oIePNIJONn_ZZAY0yiubheTFSvq_h44QFF_UZcoZP9m_dQiASt67iCgm5VOEIUrZDvAFlXEmqeuFmEsPKAEPJIRU55IjKGeqd8zlk4HNzoMLHQQC7GndhamUNnhmPe4PGCa_358ixRDdu581ikFtz5Nv1HAk6sgJHDXdoRZhDGB_pqLbLntfj9X9Fxvb6o7fsIVEp3QiFjMPW6i1XdEV0_K83u6AR4gRrS8LLqWKYYVL14quYCYuwzzWblmsIkhJxV_QwbBzPe_JRNqrkHIWGCwe6HTF1IItmDAnN6kRZERQ4WEKCQ2V_dKsz_odhZ7TRWYu7WD0TprUjuVccrVKx9_Ny_PDjfgDM6IcYOMKbGeMPWZJclk7GzT7FEjUhgOqA7bFC8nmANQuw5L64_gvhmupB6j29Prfsw2o_klX900ZZg9WCZRzCBSAyKzbmcZgzp0mR8eMaH_xRCInjyg0ZlYjK7tBECaeXucfjYZoijuVIgXq2SJtV6sOlx0wDTFDsGPxFVbyTL8_TgDLlw9Qn5nWu3SWCIkW_JaQGcJ2LoQ_DcnbicpOpnkYWKGd1puvfzdTaLrstXXd2ohOnV_PsWFLubMaucKzPmwiv8C4WRFjALUGRGn9NkF52jFAMC6JeR_NbK7DRFPDvQhcBu0JrGffkN9eNlbqhuMCsVfYmWUXardbCo7CUrWGx_yw19DnytkxKlo2Hady9Cp8mT9tnb6WkIU1pMsq2q47pOeWu13lpWGouwDmKBzyP9rAMS7KrekuDRKbXIVplUS7eCTrMNN_UbDAsjBhdZ8ZaosC7l7tpHSBYSKZKBrfy_VwuAv7d3Nys0Lf336GW3ETfayBmWCFrHx7oIwHfwn3VdTELC_HusMBbgo7rS1ekMGL7wPGavz6SRNtnNWqODJbuHkcyDRzgeX_NWOjUU_4EFgITTtqeUrOH45z0XHu17go_QZATGKKUkehoJpMKAaxa_8zataWDo3zf81rsG8rEbYnejNUdPaFKWC9R4R9s28LA7sXLIO_B6Reg0deLPzjlKegsnt_A629PcG_dLwhqrupZeKRGXKB2B3YvEGbJdfrhZ_AVUzfJVgVkWtmJN8cxzlNkDaSOMyP6k3fb4zSXxbFAspmt7h3eWj8M1CdtejpSZKNmHRAxYfujYSCt9iniWmaN5gsNU6qxjGxgk8aW6l3OvT4r0NzRA6JvkGPonjoOX79IXqAv_7SZ83QozhKOhJhsEAC71xmL3tAl0AwvltZfJyzwec_dv0EyCqJ4vOIB_lwdZhYHQOen5JcTtN0naCbfw_z8H0kye10tJwAA_22_7D = __webpack_require__(12972);
;// ./src/components/unfinished/GitHubPRsSection.css.ts



var accountChip = '_17w3rbmf';
var accountChipEnv = '_17w3rbmg';
var accountsRow = '_17w3rbme';
var addAccountButton = '_17w3rbmk';
var addAccountPanel = '_17w3rbm14';
var attentionNote = '_17w3rbm19';
var authBanner = '_17w3rbm3';
var authBannerSuccess = '_17w3rbm5';
var authBannerText = '_17w3rbm4';
var authError = '_17w3rbm2';
var authTab = '_17w3rbm17 _17w3rbm16';
var authTabActive = '_17w3rbm18 _17w3rbm16';
var authTabs = '_17w3rbm15';
var badge = '_17w3rbm0 w8p1z05';
var banner = '_17w3rbm1e';
var bannerError = '_17w3rbm1f';
var cancelButton = '_17w3rbm7';
var GitHubPRsSection_css_chevron = 'w8p1z02';
var GitHubPRsSection_css_chevronExpanded = 'w8p1z03';
var cliImportDivider = '_17w3rbmp';
var cliImportHostButton = '_17w3rbmo';
var cliImportHostList = '_17w3rbmn';
var cliImportLabel = '_17w3rbmm';
var cliImportSection = '_17w3rbml';
var connectButton = '_17w3rbm6';
var deviceFlowCard = '_17w3rbm8';
var deviceFlowInstructions = '_17w3rbm9';
var deviceFlowRow = '_17w3rbma';
var disconnectAccountButton = '_17w3rbmj';
var empty = '_17w3rbm1';
var emptyState = '_17w3rbm1g';
var filterBar = '_17w3rbmv';
var filterChip = '_17w3rbmy _17w3rbmx';
var filterChipActive = '_17w3rbmz _17w3rbmx';
var filterChipGroup = '_17w3rbmw';
var freshness = '_17w3rbm1c';
var hostBadge = '_17w3rbmh';
var hostInput = '_17w3rbmi';
var panelButton = '_17w3rbm1d';
var panelHeader = '_17w3rbm1a';
var panelHeading = '_17w3rbm1b';
var pollingIndicator = '_17w3rbmd';
var searchInput = '_17w3rbm13';
var section = 'w8p1z00';
var sectionHeader = 'w8p1z01';
var sectionTitle = 'w8p1z04';
var sortGroup = '_17w3rbm10';
var sortLabel = '_17w3rbm11';
var sortSelect = '_17w3rbm12';
var statCount = '_17w3rbms';
var statCountError = '_17w3rbmu';
var statCountWarning = '_17w3rbmt';
var GitHubPRsSection_css_statItem = '_17w3rbmr';
var statsBar = '_17w3rbmq';
var userCode = '_17w3rbmc';
var verificationLink = '_17w3rbmb';
;// ./src/lib/unfinished/prOrdering.ts

function prKey(pr) {
    return "".concat(repoKey(pr), "#").concat(pr.number);
}
function repoKey(pr) {
    return "".concat((pr.host || "github.com").toLowerCase(), "/").concat(pr.owner, "/").concat(pr.repo);
}
/** 3 = failing checks or conflict, 2 = unresolved threads, 1 = changes requested only, 0 = none/draft. */ function severityRank(pr) {
    if (pr.isDraft) return 0;
    const a = prAttention(pr);
    if (a.failingChecks > 0 || a.checksFailingUnlisted || a.mergeConflict) return 3;
    if (a.unresolvedThreads > 0) return 2;
    if (a.changesRequested) return 1;
    return 0;
}
/** Lower = worse. */ function ciRank(pr) {
    if (pr.checkConclusion === "failure" || pr.checkConclusion === "error") return 0;
    if (pr.changesReqCount > 0) return 1;
    if (pr.checkConclusion === "success") return 3;
    return 2;
}
function updatedSeconds(pr) {
    var _pr_updatedAt;
    var _pr_updatedAt_seconds;
    return Number((_pr_updatedAt_seconds = (_pr_updatedAt = pr.updatedAt) === null || _pr_updatedAt === void 0 ? void 0 : _pr_updatedAt.seconds) !== null && _pr_updatedAt_seconds !== void 0 ? _pr_updatedAt_seconds : 0n);
}
const byUpdatedDesc = (a, b)=>updatedSeconds(b) - updatedSeconds(a);
const byUpdatedAsc = (a, b)=>updatedSeconds(a) - updatedSeconds(b);
const then = function() {
    for(var _len = arguments.length, cmps = new Array(_len), _key = 0; _key < _len; _key++){
        cmps[_key] = arguments[_key];
    }
    return (a, b)=>{
        for (const c of cmps){
            const r = c(a, b);
            if (r !== 0) return r;
        }
        return 0;
    };
};
const maxOf = (g, f)=>Math.max(...g.prs.map(f));
const minOf = (g, f)=>Math.min(...g.prs.map(f));
const keyAsc = (a, b)=>a.key.localeCompare(b.key);
const RULES = {
    "attention-first": {
        card: then((a, b)=>severityRank(b) - severityRank(a), byUpdatedDesc),
        group: (a, b)=>maxOf(b, severityRank) - maxOf(a, severityRank) || maxOf(b, updatedSeconds) - maxOf(a, updatedSeconds) || keyAsc(a, b)
    },
    repo: {
        card: then((a, b)=>severityRank(b) - severityRank(a), byUpdatedDesc),
        group: keyAsc
    },
    "updated-desc": {
        card: byUpdatedDesc,
        group: (a, b)=>maxOf(b, updatedSeconds) - maxOf(a, updatedSeconds) || keyAsc(a, b)
    },
    "updated-asc": {
        card: byUpdatedAsc,
        group: (a, b)=>minOf(a, updatedSeconds) - minOf(b, updatedSeconds) || keyAsc(a, b)
    },
    "ci-status": {
        card: then((a, b)=>ciRank(a) - ciRank(b), byUpdatedDesc),
        group: (a, b)=>minOf(a, ciRank) - minOf(b, ciRank) || keyAsc(a, b)
    }
};
/** Groups PRs by repo and orders groups and cards per the sort's documented rule. Pure. */ function orderPRs(prs, sortBy) {
    const rule = RULES[sortBy];
    const map = new Map();
    for (const pr of prs){
        const key = repoKey(pr);
        const list = map.get(key);
        if (list) list.push(pr);
        else map.set(key, [
            pr
        ]);
    }
    const groups = [];
    for (const [key, members] of map){
        groups.push({
            key,
            prs: [
                ...members
            ].sort(rule.card)
        });
    }
    return groups.sort(rule.group);
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/connect-error.js
var connect_error = __webpack_require__(45640);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/code.js
var code = __webpack_require__(16294);
// EXTERNAL MODULE: ./src/lib/api/transport.ts
var api_transport = __webpack_require__(21694);
;// ./src/lib/hooks/useNudgePR.ts
/* __next_internal_client_entry_do_not_use__ NUDGE_STATE_TTL_MS,useNudgePR auto */ 




/** Message and "Requested just now" lifetime; matches the server's duplicate window. */ const NUDGE_STATE_TTL_MS = 60000;
const IDLE = {
    status: "idle"
};
function toErrorState(sessionId, err) {
    const e = connect_error/* ConnectError */.T.from(err);
    const message = e.rawMessage || e.message;
    if (e.code === code/* Code */.C.ResourceExhausted) return {
        status: "error",
        sessionId,
        kind: "rate_limit",
        message
    };
    if (e.code === code/* Code */.C.FailedPrecondition) return {
        status: "error",
        sessionId,
        kind: "failed_precondition",
        message
    };
    return {
        status: "error",
        sessionId,
        kind: "network",
        message: "Could not send request: ".concat(message)
    };
}
/**
 * One card's nudge lifecycle. State lives in component memory only (the server's duplicate
 * answer is the source of truth after a reload). A `done` result clears after 60 s; errors
 * stay until the next action.
 */ function useNudgePR() {
    let { client } = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : {};
    const [state, setState] = (0,react.useState)(IDLE);
    const timerRef = (0,react.useRef)(null);
    const clientRef = (0,react.useRef)(client);
    clientRef.current = client;
    const clearTimer = ()=>{
        if (timerRef.current !== null) clearTimeout(timerRef.current);
        timerRef.current = null;
    };
    (0,react.useEffect)(()=>clearTimer, []);
    const dismiss = (0,react.useCallback)(()=>{
        clearTimer();
        setState(IDLE);
    }, []);
    const nudge = (0,react.useCallback)(async (pr, sessionId)=>{
        clearTimer();
        setState({
            status: "pending",
            sessionId
        });
        var _clientRef_current;
        const c = (_clientRef_current = clientRef.current) !== null && _clientRef_current !== void 0 ? _clientRef_current : (0,promise_client/* createClient */.UU)(github_user_pb/* GitHubUserService */.aI, (0,api_transport/* getConnectTransport */.qW)());
        const req = (0,create/* create */.v)(github_user_pb/* NudgeSessionForPRRequestSchema */.Tn, {
            pr: (0,create/* create */.v)(github_user_pb/* PRKeySchema */.xE, {
                host: pr.host || "github.com",
                owner: pr.owner,
                repo: pr.repo,
                number: pr.number
            }),
            sessionId
        });
        try {
            const res = await c.nudgeSessionForPR(req);
            const done = {
                status: "done",
                sessionId,
                outcome: res.outcome,
                detail: res.detail
            };
            setState(done);
            timerRef.current = setTimeout(()=>setState(IDLE), NUDGE_STATE_TTL_MS);
            return done;
        } catch (err) {
            const failed = toErrorState(sessionId, err);
            setState(failed);
            return failed;
        }
    }, []);
    return {
        state,
        nudge,
        dismiss
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Funfinished%2Fprs%2FNudgeButton.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA7VV227iMBB95yusPsGDUQKUUFcr7X-sVqshHhKrjh1NnCZhxb-vYgoy4ap2-0IkzmjOmTMXT7Pn1CzaiP0dMSZVVWroBNtobF9HzH-5VISpU9YIllpdF6ZHMigFewcac_4nfsHtjLazbtIjhTK8UdLlgkWvo91oumeILzOAVpnhymFRCZaicUhH4oZ6kv73GuN2EjDMThmU0cogv0N0LStjG2scr9QWh_gcPJ5abWmIxaGguRfU-5GjynIn2GJRtgOPDn-VIKUy2QVT_6O8yENrSN8ysrWRQzzZ45YkkmBx2bLKaiUHURhEcQKp6upMxDI0YvEtRszvVRMn153QD1WqH6z0fjs83nxUv4wiL6ymqldWWnUYR_uOtNG2-Zh9MF2TI2Ho5S8gBVyqCtYa5Y8nRzU-_fYO2xJS5TrBoukyJDDWcdDaNijDTMJYN76cbiLyXorPesPgVdjl57NoR2CqEgiNu7kx9xsRn5u42JsY1JwEYpb7kQPKlPGH6AsLfdaWNSG88cZSaGfyXQfuS7tua9efwX4GTDhGicCidN2p5kHMyqOXbZmFnX_5zOV98Bzc2N7znToqgs8octg6LjG1BPvn7uDHzwKlAlalhGgYGMnGBbRHpauobCee8OStu_V8DjRUjtCleY_sRrt_IEfLCZQHAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Funfinished_2Fprs_2FNudgeButton_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA7VV227iMBB95yusPsGDUQKUUFcr7X_sVqshHhKrjh1NnCZhxb_vYgoy4ap2_0IkzmjOmTMXT7Pn1CzaiP0dMSZVVWroBNtobF9HzH_5VISpU9YIllpdF6ZHMigFewcac_4nfsHtjLazbtIjhTK8UdLlgkWvo91oumeILzOAVpnhymFRCZaicUhH4oZ6kv73GuN2EjDMThmU0cogv0N0LStjG2scr9QWh_gcPJ5abWmIxaGguRfU_5GjynIn2GJRtgOPDn_VIKUy2QVT_6O8yENrSN8ysrWRQzzZ45YkkmBx2bLKaiUHURhEcQKp6upMxDI0YvEtRszvVRMn153QD1WqH6z0fjs83nxUv4wiL6ymqldWWnUYR_uOtNG2_Zh9MF2TI2Ho5S8gBVyqCtYa5Y8nRzU_fYO2xJS5TrBoukyJDDWcdDaNijDTMJYN76cbiLyXorPesPgVdjl57NoR2CqEgiNu7kx9xsRn5u42JsY1JwEYpb7kQPKlPGH6AsLfdaWNSG88cZSaGfyXQfuS7tua9efwX4GTDhGicCidN2p5kHMyqOXbZmFnX_5zOV98Bzc2N7znToqgs8octg6LjG1BPvn7uDHzwKlAlalhGgYGMnGBbRHpauobCee8OStu_V8DjRUjtCleY_sRrt_IEfLCZQHAAA_22_7D = __webpack_require__(71097);
;// ./src/components/unfinished/prs/NudgeButton.css.ts


var alertRegion = 'g5cn4x8 g5cn4x7';
var askButton = 'g5cn4x4';
var askButtonSecondary = 'g5cn4x5 g5cn4x4';
var controls = 'g5cn4x1';
var hint = 'g5cn4x6';
var NudgeButton_css_nudge = 'g5cn4x0';
var NudgeButton_css_openSessionLink = 'g5cn4x9';
var primaryOpenLink = 'g5cn4xa g5cn4x4';
var NudgeButton_css_select = 'g5cn4x3';
var selectLabel = 'g5cn4x2';
var statusRegion = 'g5cn4x7';
;// ./src/components/unfinished/prs/NudgeButton.tsx
// +feature: up-next-tabs
/* __next_internal_client_entry_do_not_use__ NUDGE_HINT,NOT_IDLE_HINT,SLOW_SEND_MS,outcomeMessage,stateMessage,NudgeButton auto */ 








const NUDGE_HINT = "Sends this session a message listing the failing checks, unresolved review threads and merge conflict for this PR, as links. Comment text is not included.";
const PAUSED_TEXT = "Session paused. Open it to resume";
const NOT_RUNNING_TEXT = "Session is not running or was deleted. Open it to restart it.";
const NOT_IDLE_TEXT = "Session isn't idle, so it can't take a request. Open it to continue.";
const BUSY_FALLBACK_TEXT = "Session can't take a request right now. Open it to continue.";
const NOT_IDLE_HINT = "None of this PR's sessions are confirmed idle, so a request could interrupt them. Open the session to continue.";
const SLOW_SEND_MS = 3000;
function sessionSignature(s) {
    return "".concat(s.status, "|").concat(s.steerReady);
}
/** Why a session cannot take a request, or undefined when it can. */ function blockedReason(status) {
    if (status === types_pb/* LinkedSessionStatus */.Wd.PAUSED) return PAUSED_TEXT;
    if (status === types_pb/* LinkedSessionStatus */.Wd.STOPPED) return "Session stopped. Open it to restart it.";
    return undefined;
}
/** Copy for each outcome; the `never` default fails the build when the proto gains an outcome. */ function outcomeMessage(outcome, detail, sessionId) {
    const status = function(text) {
        let withLink = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : false;
        return {
            text,
            role: "status",
            openSessionId: withLink ? sessionId : undefined
        };
    };
    switch(outcome){
        case github_user_pb/* NudgeOutcome */.UX.DELIVERED:
            return status("Request sent to ".concat(sessionId), true);
        case github_user_pb/* NudgeOutcome */.UX.BUSY:
            return status(detail || BUSY_FALLBACK_TEXT, true);
        case github_user_pb/* NudgeOutcome */.UX.PAUSED:
            return status(detail || PAUSED_TEXT, true);
        case github_user_pb/* NudgeOutcome */.UX.NOT_RUNNING:
            return status(detail || NOT_RUNNING_TEXT, true);
        case github_user_pb/* NudgeOutcome */.UX.DUPLICATE:
            return status("Already requested in the last minute", true);
        case github_user_pb/* NudgeOutcome */.UX.NOTHING_TO_FIX:
            return status("Nothing to fix right now");
        case github_user_pb/* NudgeOutcome */.UX.SESSION_NOT_LINKED:
            return status("Session no longer linked");
        case github_user_pb/* NudgeOutcome */.UX.PR_NOT_FOUND:
            return status("PR not found (closed or moved?)");
        case github_user_pb/* NudgeOutcome */.UX.UNSPECIFIED:
            return status("Unexpected response. Try again.");
        default:
            {
                const unhandled = outcome;
                return status("Unexpected response (".concat(String(unhandled), ")"));
            }
    }
}
function stateMessage(state) {
    if (state.status === "done") return outcomeMessage(state.outcome, state.detail, state.sessionId);
    if (state.status === "error") return {
        text: state.message,
        role: "alert"
    };
    return undefined;
}
function fixNoun(pr) {
    const a = prAttention(pr);
    const reasons = [
        a.failingChecks > 0,
        a.unresolvedThreads > 0,
        a.mergeConflict
    ].filter(Boolean).length;
    if (reasons > 1) return "PR #".concat(pr.number);
    if (a.failingChecks > 0) return "CI on PR #".concat(pr.number);
    if (a.unresolvedThreads > 0) return "comments on PR #".concat(pr.number);
    return "conflicts on PR #".concat(pr.number);
}
function NudgeButton(param) {
    let { pr, sessions, nudgeable, client, onNothingToFix } = param;
    const { state, nudge, dismiss } = useNudgePR({
        client
    });
    const [selected, setSelected] = (0,react.useState)();
    const [blockedIds, setBlockedIds] = (0,react.useState)(new Map());
    const block = (id, reason)=>{
        const s = sessions.find((x)=>x.sessionId === id);
        setBlockedIds((prev)=>new Map(prev).set(id, {
                reason,
                sig: s ? sessionSignature(s) : ""
            }));
    };
    // A block reflects the state the session was in when the server refused; once its status or
    // readiness changes (e.g. the user resumed it) the refusal is stale, so let the poll decide again.
    (0,react.useEffect)(()=>{
        setBlockedIds((prev)=>{
            let changed = false;
            const next = new Map(prev);
            for (const [id, entry] of prev){
                const s = sessions.find((x)=>x.sessionId === id);
                if (!s || sessionSignature(s) !== entry.sig) {
                    next.delete(id);
                    changed = true;
                }
            }
            return changed ? next : prev;
        });
    }, [
        sessions
    ]);
    const [slow, setSlow] = (0,react.useState)(false);
    const [hiddenByNothingToFix, setHiddenByNothingToFix] = (0,react.useState)(false);
    const [landingNote, setLandingNote] = (0,react.useState)(false);
    const hadFocus = (0,react.useRef)(false);
    const regionRef = (0,react.useRef)(null);
    const hintId = (0,react.useId)();
    const selectId = (0,react.useId)();
    // A poll hands over a fresh `pr` object each time; only a change in what needs attention may unhide.
    const attention = prAttention(pr);
    const attentionSignature = "".concat(pr.checkConclusion, "|").concat(attention.failingChecks, "|").concat(attention.unresolvedThreads, "|").concat(attention.mergeConflict);
    (0,react.useEffect)(()=>setHiddenByNothingToFix(false), [
        attentionSignature
    ]);
    const pending = state.status === "pending";
    (0,react.useEffect)(()=>{
        if (!pending) {
            setSlow(false);
            return;
        }
        const t = setTimeout(()=>setSlow(true), SLOW_SEND_MS);
        return ()=>clearTimeout(t);
    }, [
        pending
    ]);
    // A send in flight or finished keeps its session ready even if the next poll reports it busy.
    const isReady = (s)=>s.steerReady || state.status !== "idle" && state.sessionId === s.sessionId;
    const isBlocked = (s)=>blockedIds.has(s.sessionId) || blockedReason(s.status) !== undefined || !isReady(s);
    const reasonFor = (s)=>{
        var _blockedIds_get;
        var _blockedReason, _ref;
        return (_ref = (_blockedReason = blockedReason(s.status)) !== null && _blockedReason !== void 0 ? _blockedReason : (_blockedIds_get = blockedIds.get(s.sessionId)) === null || _blockedIds_get === void 0 ? void 0 : _blockedIds_get.reason) !== null && _ref !== void 0 ? _ref : isReady(s) ? PAUSED_TEXT : NOT_IDLE_TEXT;
    };
    const runnable = sessions.filter((s)=>!isBlocked(s));
    var _runnable_find;
    const target = (_runnable_find = runnable.find((s)=>s.sessionId === selected)) !== null && _runnable_find !== void 0 ? _runnable_find : runnable[0];
    const label = target !== null && target !== void 0 ? target : sessions[0];
    var _sessions_find;
    const openTarget = (_sessions_find = sessions.find((s)=>s.sessionId === selected)) !== null && _sessions_find !== void 0 ? _sessions_find : sessions[0];
    const showControls = nudgeable && sessions.length > 0 && !hiddenByNothingToFix;
    // The focused button is removed when the PR stops being nudgeable: keep focus in the card.
    (0,react.useEffect)(()=>{
        var _regionRef_current;
        if (showControls || !hadFocus.current) return;
        hadFocus.current = false;
        setLandingNote(true);
        (_regionRef_current = regionRef.current) === null || _regionRef_current === void 0 ? void 0 : _regionRef_current.focus();
    }, [
        showControls
    ]);
    (0,react.useEffect)(()=>{
        if (showControls) setLandingNote(false);
    }, [
        showControls
    ]);
    const forTarget = state.status !== "idle" && target && state.sessionId === target.sessionId;
    const outcome = state.status === "done" ? state.outcome : undefined;
    const holdsOff = forTarget && (outcome === github_user_pb/* NudgeOutcome */.UX.DELIVERED || outcome === github_user_pb/* NudgeOutcome */.UX.DUPLICATE);
    const disabled = pending || holdsOff || !target || outcome === github_user_pb/* NudgeOutcome */.UX.PR_NOT_FOUND;
    let visibleText = label ? "Ask ".concat(label.sessionId, " to fix") : "";
    let accessibleName = label ? "".concat(visibleText, " ").concat(fixNoun(pr)) : undefined;
    if (pending) {
        visibleText = slow ? "Still sending..." : "Sending...";
        accessibleName = undefined;
    } else if (forTarget && outcome === github_user_pb/* NudgeOutcome */.UX.DELIVERED) {
        visibleText = "Sent";
        accessibleName = undefined;
    } else if (forTarget && outcome === github_user_pb/* NudgeOutcome */.UX.DUPLICATE) {
        visibleText = "Already requested in the last minute";
        accessibleName = undefined;
    }
    const onClick = async ()=>{
        if (disabled || !target) return;
        const result = await nudge(pr, target.sessionId);
        if (result.status === "error" && result.kind === "failed_precondition") {
            block(result.sessionId, result.message);
        } else if (result.status === "done") {
            if (result.outcome === github_user_pb/* NudgeOutcome */.UX.PAUSED) {
                block(result.sessionId, result.detail || PAUSED_TEXT);
            } else if (result.outcome === github_user_pb/* NudgeOutcome */.UX.NOT_RUNNING) {
                block(result.sessionId, result.detail || NOT_RUNNING_TEXT);
            } else if (result.outcome === github_user_pb/* NudgeOutcome */.UX.SESSION_NOT_LINKED) {
                block(result.sessionId, "No longer linked");
            } else if (result.outcome === github_user_pb/* NudgeOutcome */.UX.NOTHING_TO_FIX) {
                setHiddenByNothingToFix(true);
                onNothingToFix === null || onNothingToFix === void 0 ? void 0 : onNothingToFix();
            }
        }
    };
    const onSelect = (sessionId)=>{
        dismiss();
        setSelected(sessionId);
    };
    var _stateMessage;
    const message = (_stateMessage = stateMessage(state)) !== null && _stateMessage !== void 0 ? _stateMessage : landingNote ? {
        text: "Nothing to fix right now",
        role: "status"
    } : undefined;
    const statusMessage = (message === null || message === void 0 ? void 0 : message.role) === "status" ? message : undefined;
    const alertMessage = (message === null || message === void 0 ? void 0 : message.role) === "alert" ? message : undefined;
    const openSessionLink = (id)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(nav/* Link */.N_, {
            href: "/?session=".concat(encodeURIComponent(id)),
            className: NudgeButton_css_openSessionLink,
            "aria-label": "Open session ".concat(id),
            onClick: ()=>emitTabEvent({
                    type: "openSession"
                }),
            children: "Open session"
        });
    const blockedList = sessions.filter(isBlocked);
    const allBlocked = showControls && runnable.length === 0;
    const allNotIdle = allBlocked && sessions.every((s)=>!isReady(s));
    var _target_sessionId;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: NudgeButton_css_nudge,
        children: [
            showControls && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: controls,
                        children: [
                            sessions.length > 1 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: selectLabel,
                                htmlFor: selectId,
                                children: [
                                    "Session",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("select", {
                                        id: selectId,
                                        className: NudgeButton_css_select,
                                        value: (_target_sessionId = target === null || target === void 0 ? void 0 : target.sessionId) !== null && _target_sessionId !== void 0 ? _target_sessionId : "",
                                        onChange: (e)=>onSelect(e.target.value),
                                        children: sessions.map((s)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                value: s.sessionId,
                                                disabled: isBlocked(s),
                                                children: isBlocked(s) ? "".concat(s.sessionId, " (").concat(reasonFor(s), ")") : s.sessionId
                                            }, s.sessionId))
                                    })
                                ]
                            }),
                            allNotIdle && /*#__PURE__*/ (0,jsx_runtime.jsx)(nav/* Link */.N_, {
                                href: "/?session=".concat(encodeURIComponent(openTarget.sessionId)),
                                className: primaryOpenLink,
                                "aria-label": "Open session ".concat(openTarget.sessionId),
                                "data-testid": "pr-open-session-".concat(pr.number),
                                onClick: ()=>emitTabEvent({
                                        type: "openSession"
                                    }),
                                children: "Open session"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: allNotIdle ? askButtonSecondary : askButton,
                                "aria-label": accessibleName,
                                "aria-disabled": disabled || undefined,
                                "aria-busy": pending || undefined,
                                "aria-describedby": hintId,
                                "data-testid": "pr-nudge-".concat(pr.number),
                                onFocus: ()=>{
                                    hadFocus.current = true;
                                },
                                onBlur: ()=>{
                                    hadFocus.current = false;
                                },
                                onClick: onClick,
                                children: visibleText
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        id: hintId,
                        className: hint,
                        children: allNotIdle ? NOT_IDLE_HINT : NUDGE_HINT
                    }),
                    blockedList.map((s)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                            className: hint,
                            "data-testid": "nudge-blocked-reason",
                            children: [
                                s.sessionId,
                                ": ",
                                reasonFor(s),
                                " ",
                                allBlocked && !allNotIdle && blockedList.length === 1 && openSessionLink(s.sessionId)
                            ]
                        }, s.sessionId))
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                ref: regionRef,
                role: "status",
                tabIndex: -1,
                className: statusRegion,
                "data-testid": "pr-nudge-status-".concat(pr.number),
                children: statusMessage && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            children: statusMessage.text
                        }),
                        statusMessage.openSessionId && openSessionLink(statusMessage.openSessionId)
                    ]
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                role: "alert",
                className: alertRegion,
                children: alertMessage && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    children: alertMessage.text
                })
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/contexts/notificationContexts.ts
var notificationContexts = __webpack_require__(82514);
;// ./src/lib/hooks/usePRActions.ts
/* __next_internal_client_entry_do_not_use__ usePRActions auto */ 




const target = (pr)=>({
        owner: pr.owner,
        repo: pr.repo,
        number: pr.number,
        host: pr.host || "github.com"
    });
/**
 * One card's PR actions. `inflight` names the running action so every button can disable
 * itself; `run` resolves true on success and sets `error` (cleared on the next run) otherwise.
 */ function usePRActions(client) {
    const [inflight, setInflight] = (0,react.useState)(null);
    const [error, setError] = (0,react.useState)(null);
    const clientRef = (0,react.useRef)(client);
    clientRef.current = client;
    const run = (0,react.useCallback)(async function(name, pr) {
        let method = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : "merge";
        setError(null);
        setInflight(name);
        var _clientRef_current;
        const c = (_clientRef_current = clientRef.current) !== null && _clientRef_current !== void 0 ? _clientRef_current : (0,promise_client/* createClient */.UU)(github_user_pb/* GitHubUserService */.aI, (0,api_transport/* getConnectTransport */.qW)());
        try {
            switch(name){
                case "merge":
                    await c.mergePRDirect((0,create/* create */.v)(github_user_pb/* MergePRDirectRequestSchema */.WJ, {
                        ...target(pr),
                        method
                    }));
                    break;
                case "close":
                    await c.closePRDirect((0,create/* create */.v)(github_user_pb/* ClosePRDirectRequestSchema */.AY, target(pr)));
                    break;
                case "toDraft":
                    await c.convertPRToDraft((0,create/* create */.v)(github_user_pb/* PRDraftToggleRequestSchema */.Ex, target(pr)));
                    break;
                case "toReady":
                    await c.markPRReady((0,create/* create */.v)(github_user_pb/* PRDraftToggleRequestSchema */.Ex, target(pr)));
                    break;
            }
            return true;
        } catch (err) {
            const e = connect_error/* ConnectError */.T.from(err);
            setError(e.rawMessage || e.message);
            return false;
        } finally{
            setInflight(null);
        }
    }, []);
    return {
        inflight,
        error,
        run
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Funfinished%2Fprs%2FPRCard.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA91Zy3KbMBTd5yu0rBfKIOI4Kd30TzoyCFuNkKgkjO1O_r2DwI4QL0HsLLpJJnC5575fefyFnjbPyctrAP4-ALDF8dtOioInEThg-Q3CX-g7OYfy_LL6Ub0XMiEyAig_AiUYTRwqYlFBiRNaKJfRU80px0lC-a7zNli5T5ChT6jKGT5FIGXkWD2ofsOEShJrKngEYsGKjFdvdjh32YZnw0RLzBWt6RshY8GEBMEjelY_Ht4fHq_2iPbiQGRtFYvU5UxXrc-Q-aAjLGZ0xyHVJFP1Q6g0lvqqRykrmaufAwo8BW2c0OCkgmuo6Jl0yLdGX_O-JHS31xHYBEH1rFeLoDYPOWqYkFhIXNuIC06uMu6kKCOAqr8zymFJE72PgOFZWSplomz0wPxU7ok0nzLKCdw3IqDHdVsLy8gd8IInRFafD0qdtW3y5GH7mHBN5JiRezxiYaw_7J7ijLJTJ9KKD8v3egavBvUJV7OMa4n1fE-x0GKxNv0eGU7QAV_15EiG5Y5yyEiqI4ALLboyWoK8tAWh3ITlRIQMlKjw5JaoRvjxsrfxckBfxjop9Fw9K_dUE6hyHJMqTTtx-jpRzuMpd09VetTOve9fjIcn8A7DeKUXXtnG207gnYbxzl545zZePIGH0IhBQz-Lhm3IxEBWKXSJtfU6N_kRF1JVMLmgl9wYbBVOYMoMszZKlIq4UPBAFd0yYjBFoasYj0A4ICozCjVkUKSpItpQt1jXzBhVGip9YuRDqLpYNIXrmtbBrLnCAkqXVJM-w87oo4sqt9Mgd19loP2sTuw3ATVFts-Mi7uaJTL17KBL2uBvw9tHireJsc5fH1Zns-3XTzR_R8utJPgNlkImLcxsufTLRx9-z9EnG5uLbSE8B1nrE3HHHKlnIy26BKd2lOT9QszbrRyefwzPWg9FWOouPH0ZPHvUshqjWepyLAnXnjPGda1ZMIx1eqGtuRUFPvVY3jRHW503lwReAsUni9RIIEyHXT39zGosFra-oRkstsW9EszCOAzOTN4R3Syd91oexuL1ECVU4S0j9egnchxTfYpAUO8Yl0-50BAzJkrSLvdlZ0adn4pjhyQL6rgMKvAbhJ3bysnrFDZyR5mp3PlW8e_ogYJbVPfzxLnO28KfjfGtox26Xe4Nr1kjvmZeqrOvze6RzWzmvjJ1YUDIvpAOW_DVcVt4F7fdtvL8nw4LPU_amfdCiQZOrp-oMM1Fr_d_AIPnPVSfZT-xPk_vDyNrS5-jR4I5sAJsK7QW2XA0pn4D_c-MJBQDFUtCOMA8Ad8yfLwYa_0a5MeVMZHzj4oxV73b5JuGvHV2DTpkYpKrE75KS6Ljfc3o_R-CB_KDjhoAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Funfinished_2Fprs_2FPRCard_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA91Zy3KbMBTd5yu0rBfKIOI4Kd30TzoyCFuNkKgkjO1O_r2DwI4QL0HsLLpJJnC5575fefyFnjbPyctrAP4_ALDF8dtOioInEThg_Q3CX_g7OYfy_LL6Ub0XMiEyAig_AiUYTRwqYlFBiRNaKJfRU80px0lC_a7zNli5T5ChT6jKGT5FIGXkWD2ofsOEShJrKngEYsGKjFdvdjh32YZnw0RLzBWt6RshY8GEBMEjelY_Ht4fHq_2iPbiQGRtFYvU5UxXrc_Q_aAjLGZ0xyHVJFP1Q6g0lvqqRykrmaufAwo8BW2c0OCkgmuo6Jl0yLdGX_O_JHS31xHYBEH1rFeLoDYPOWqYkFhIXNuIC06uMu6kKCOAqr8zymFJE72PgOFZWSplomz0wPxU7ok0nzLKCdw3IqDHdVsLy8gd8IInRFafD0qdtW3y5GH7mHBN5JiRezxiYaw_7J7ijLJTJ9KKD8v3egavBvUJV7OMa4n1fE_x0GKxNv0eGU7QAV_15EiG5Y5yyEiqI4ALLboyWoK8tAWh3ITlRIQMlKjw5JaoRvjxsrfxckBfxjop9Fw9K_dUE6hyHJMqTTtx_jpRzuMpd09VetTOve9fjIcn8A7DeKUXXtnG207gnYbxzl545zZePIGH0IhBQz_Lhm3IxEBWKXSJtfU6N_kRF1JVMLmgl9wYbBVOYMoMszZKlIq4UPBAFd0yYjBFoasYj0A4ICozCjVkUKSpItpQt1jXzBhVGip9YuRDqLpYNIXrmtbBrLnCAkqXVJM_w87oo4sqt9Mgd19loP2sTuw3ATVFts_Mi7uaJTL17KBL2uBvw9tHireJsc5fH1Zns_3XTzR_R8utJPgNlkImLcxsufTLRx9_z9EnG5uLbSE8B1nrE3HHHKlnIy26BKd2lOT9QszbrRyefwzPWg9FWOouPH0ZPHvUshqjWepyLAnXnjPGda1ZMIx1eqGtuRUFPvVY3jRHW503lwReAsUni9RIIEyHXT39zGosFra_oRkstsW9EszCOAzOTN4R3Syd91oexuL1ECVU4S0j9egnchxTfYpAUO8Yl0_50BAzJkrSLvdlZ0adn4pjhyQL6rgMKvAbhJ3bysnrFDZyR5mp3PlW8e_ogYJbVPfzxLnO28KfjfGtox26Xe4Nr1kjvmZeqrOvze6RzWzmvjJ1YUDIvpAOW_DVcVt4F7fdtvL8nw4LPU_amfdCiQZOrp_oMM1Fr_d_AIPnPVSfZT_xPk_vDyNrS5_jR4I5sAJsK7QW2XA0pn4D_c_MJBQDFUtCOMA8Ad8yfLwYa_0a5MeVMZHzj4oxV73b5JuGvHV2DTpkYpKrE75KS6Ljfc3o_R_CB_KDjhoAAA_22_7D = __webpack_require__(56204);
;// ./src/components/unfinished/prs/PRCard.css.ts


var actionButton = '_1365d78w _1365d78v';
var actionError = '_1365d78z';
var PRCard_css_actionRow = '_1365d78u';
var bodyPreview = '_1365d78r';
var changesRequestedNote = '_1365d78l';
var checkLink = '_1365d78f';
var checkList = '_1365d78e';
var chipDraft = '_1365d788 _1365d787';
var chipError = '_1365d78c _1365d787';
var chipInteractive = '_1365d78d';
var chipNeutral = '_1365d789 _1365d787';
var chipSuccess = '_1365d78a _1365d787';
var chipWarning = '_1365d78b _1365d787';
var PRCard_css_chips = '_1365d786';
var confirmDialog = '_1365d7810';
var createSessionButton = '_1365d7812';
var dangerActionButton = '_1365d78x _1365d78v';
var expandToggle = '_1365d78q';
var expandedInfo = '_1365d78p';
var hostAccountLabel = '_1365d78m';
var labelList = '_1365d78s';
var methodSelect = '_1365d78y _1365d78v';
var noSessionText = '_1365d78k';
var openSessionButton = '_1365d7811';
var prActions = '_1365d78o';
var prBranch = '_1365d785';
var prCard = '_1365d780';
var prHeader = '_1365d781';
var prMeta = '_1365d783';
var prRepo = '_1365d784';
var prTitle = '_1365d782';
var repoGroupHeader = '_1365d7814';
var repoGroupSection = '_1365d7813';
var sessionList = '_1365d78g';
var sessionName = '_1365d78i';
var sessionRow = '_1365d78h';
var sessionStatus = '_1365d78j';
var statLine = '_1365d78t';
var worktreeLink = '_1365d78n';
;// ./src/components/unfinished/prs/PRActionBar.tsx
// +feature: unfinished-github-prs
/* __next_internal_client_entry_do_not_use__ PRActionBar auto */ 





const METHOD_KEY = "pr-panel-merge-method";
const METHODS = [
    "merge",
    "squash",
    "rebase"
];
function readMethod() {
    try {
        const v = window.localStorage.getItem(METHOD_KEY);
        if (METHODS.includes(v)) return v;
    } catch (e) {
    // storage unavailable: fall back to the default
    }
    return "merge";
}
function PRActionBar(param) {
    let { pr, client, onGone, onChanged } = param;
    const { inflight, error, run } = usePRActions(client);
    const { addNotification } = (0,notificationContexts/* useNotificationCommands */.RK)();
    const [method, setMethod] = (0,react.useState)(readMethod);
    const [confirmingClose, setConfirmingClose] = (0,react.useState)(false);
    const dialogRef = (0,react.useRef)(null);
    const closeTriggerRef = (0,react.useRef)(null);
    (0,useFocusTrap/* useFocusTrap */.r)(dialogRef, confirmingClose, closeTriggerRef);
    const busy = inflight !== null;
    const toast = (message)=>addNotification({
            sessionId: "",
            sessionName: "",
            message,
            notificationType: "info"
        });
    const changeMethod = (m)=>{
        setMethod(m);
        try {
            window.localStorage.setItem(METHOD_KEY, m);
        } catch (e) {
        // not persisted; the selection still applies for this card
        }
    };
    const merge = async ()=>{
        if (await run("merge", pr, method)) {
            toast("PR #".concat(pr.number, " merged"));
            onGone();
        }
    };
    const close = async ()=>{
        // Stay mounted until the RPC settles so focus returns to a re-enabled trigger on failure.
        const ok = await run("close", pr);
        setConfirmingClose(false);
        if (ok) {
            toast("PR #".concat(pr.number, " closed"));
            onGone();
        }
    };
    const toggleDraft = async ()=>{
        if (await run(pr.isDraft ? "toReady" : "toDraft", pr)) onChanged === null || onChanged === void 0 ? void 0 : onChanged();
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            error && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                role: "alert",
                className: actionError,
                "data-testid": "pr-action-error",
                children: error
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: PRCard_css_actionRow,
                children: [
                    !pr.isDraft && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("select", {
                                className: methodSelect,
                                "aria-label": "Merge method",
                                value: method,
                                disabled: busy,
                                onChange: (e)=>changeMethod(e.target.value),
                                children: METHODS.map((m)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                        value: m,
                                        children: m
                                    }, m))
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: actionButton,
                                disabled: busy,
                                onClick: merge,
                                "data-testid": "pr-merge-button",
                                children: inflight === "merge" ? "Merging…" : "Merge (".concat(method, ")")
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: actionButton,
                        disabled: busy || !pr.nodeId,
                        onClick: toggleDraft,
                        "data-testid": "pr-draft-toggle-button",
                        children: pr.isDraft ? "Mark as ready" : "Convert to draft"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        ref: closeTriggerRef,
                        type: "button",
                        className: dangerActionButton,
                        disabled: busy,
                        onClick: ()=>setConfirmingClose(true),
                        "data-testid": "pr-close-button",
                        children: "Close"
                    })
                ]
            }),
            confirmingClose && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                ref: dialogRef,
                role: "dialog",
                "aria-modal": "true",
                "aria-labelledby": "close-pr-title-".concat(pr.number),
                tabIndex: -1,
                className: confirmDialog,
                onKeyDown: (e)=>{
                    if (e.key === "Escape") setConfirmingClose(false);
                },
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                        id: "close-pr-title-".concat(pr.number),
                        children: [
                            "Close PR #",
                            pr.number,
                            " without merging?"
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: PRCard_css_actionRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                autoFocus: true,
                                className: actionButton,
                                disabled: busy,
                                onClick: ()=>setConfirmingClose(false),
                                children: "Cancel"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: dangerActionButton,
                                disabled: busy,
                                onClick: close,
                                "data-testid": "pr-close-confirm",
                                children: "Close PR"
                            })
                        ]
                    })
                ]
            })
        ]
    });
}

;// ./src/components/unfinished/prs/PRExpandedInfo.tsx
// +feature: unfinished-github-prs
/* __next_internal_client_entry_do_not_use__ PRExpandedInfo auto */ 


const BODY_PREVIEW_CHARS = 300;
function PRExpandedInfo(param) {
    let { pr } = param;
    const [open, setOpen] = (0,react.useState)(false);
    const panelId = (0,react.useId)();
    const chars = Array.from(pr.body);
    const body = chars.length > BODY_PREVIEW_CHARS ? "".concat(chars.slice(0, BODY_PREVIEW_CHARS).join(""), "…") : pr.body;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: expandedInfo,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                className: expandToggle,
                "aria-expanded": open,
                "aria-controls": panelId,
                onClick: ()=>setOpen((v)=>!v),
                "data-testid": "pr-details-toggle",
                children: open ? "Hide details" : "Show details"
            }),
            open && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                id: panelId,
                "data-testid": "pr-details",
                children: [
                    body && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: bodyPreview,
                        children: body
                    }),
                    pr.labels.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                        className: labelList,
                        "aria-label": "Labels",
                        children: pr.labels.map((l)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("li", {
                                className: chipNeutral,
                                children: l
                            }, l))
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                        className: statLine,
                        "data-testid": "pr-stats",
                        children: [
                            "+",
                            pr.additions,
                            " / -",
                            pr.deletions,
                            " / ",
                            pr.changedFiles,
                            " files"
                        ]
                    }),
                    pr.requestedReviewerLogins.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                        className: statLine,
                        "data-testid": "pr-reviewers",
                        children: [
                            "Awaiting review: ",
                            pr.requestedReviewerLogins.join(", ")
                        ]
                    })
                ]
            })
        ]
    });
}

;// ./src/components/unfinished/prs/PRCard.tsx
// +feature: unfinished-github-prs
/* __next_internal_client_entry_do_not_use__ PRCard auto */ 









const cx = function() {
    for(var _len = arguments.length, names = new Array(_len), _key = 0; _key < _len; _key++){
        names[_key] = arguments[_key];
    }
    return names.join(" ");
};
const EXTERNAL = {
    target: "_blank",
    rel: "noopener noreferrer"
};
function ciChip(pr, a) {
    if (pr.isDraft) return null;
    if (a.failingChecks > 0) return null; // rendered by FailingChecks
    if (a.checksFailingUnlisted) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
            className: chipError,
            "data-testid": "ci-failing-unlisted",
            children: "CI failing (checks not listed)"
        });
    }
    const conclusion = pr.checkConclusion;
    if (conclusion === "success" || conclusion === "completed") {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
            className: chipSuccess,
            children: "✓ CI passing"
        });
    }
    if (conclusion === "pending" || conclusion === "in_progress" || conclusion === "queued") {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
            className: chipNeutral,
            children: "CI pending"
        });
    }
    return null;
}
function FailingChecks(param) {
    let { pr, count } = param;
    const [open, setOpen] = (0,react.useState)(false);
    const listId = (0,react.useId)();
    const capped = pr.failingChecks.length >= 10;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                type: "button",
                className: cx(chipError, chipInteractive),
                "aria-expanded": open,
                "aria-controls": listId,
                onClick: ()=>setOpen((v)=>!v),
                "data-testid": "failing-checks-toggle",
                children: [
                    "CI: ",
                    count,
                    " failing"
                ]
            }),
            open && /*#__PURE__*/ (0,jsx_runtime.jsxs)("ul", {
                id: listId,
                className: checkList,
                "data-testid": "failing-checks-list",
                children: [
                    pr.failingChecks.map((c)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("li", {
                            children: c.url ? /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                                className: checkLink,
                                href: c.url,
                                ...EXTERNAL,
                                children: c.name
                            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: checkLink,
                                children: c.name
                            })
                        }, "".concat(c.name, "|").concat(c.url))),
                    capped && /*#__PURE__*/ (0,jsx_runtime.jsx)("li", {
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                            className: checkLink,
                            href: "".concat(pr.htmlUrl, "/checks"),
                            ...EXTERNAL,
                            children: "and more on GitHub"
                        })
                    })
                ]
            })
        ]
    });
}
function ThreadsChip(param) {
    let { pr, a } = param;
    const unknown = a.threadsUnknown || !pr.detailsLoaded && a.unresolvedThreads === 0;
    if (unknown) return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
        className: chipNeutral,
        children: "? threads"
    });
    if (a.unresolvedThreads === 0) return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("a", {
        className: cx(chipWarning, chipInteractive),
        href: pr.htmlUrl,
        ...EXTERNAL,
        children: [
            a.unresolvedThreads,
            pr.unresolvedThreadsTruncated ? "+" : "",
            " unresolved"
        ]
    });
}
function sessionStatusText(status) {
    switch(status){
        case types_pb/* LinkedSessionStatus */.Wd.RUNNING:
            return "running";
        case types_pb/* LinkedSessionStatus */.Wd.PAUSED:
            return "paused";
        case types_pb/* LinkedSessionStatus */.Wd.STOPPED:
            return "stopped";
        default:
            return "status unknown";
    }
}
const lastActive = (s)=>{
    var _s_lastActiveAt;
    var _s_lastActiveAt_seconds;
    return Number((_s_lastActiveAt_seconds = (_s_lastActiveAt = s.lastActiveAt) === null || _s_lastActiveAt === void 0 ? void 0 : _s_lastActiveAt.seconds) !== null && _s_lastActiveAt_seconds !== void 0 ? _s_lastActiveAt_seconds : 0n);
};
/** Server-linked sessions, most recently active first; falls back to bare `sessionIds`. */ function linkedSessionsOf(pr) {
    if (pr.linkedSessions.length > 0) {
        return [
            ...pr.linkedSessions
        ].sort((x, y)=>lastActive(y) - lastActive(x));
    }
    return pr.sessionIds.map((sessionId)=>({
            sessionId,
            status: types_pb/* LinkedSessionStatus */.Wd.UNSPECIFIED,
            lastActiveAt: undefined,
            steerReady: false
        }));
}
function PRCard(param) {
    let { pr, showHostAccount = false, nudgeClient, onNothingToFix, actionClient } = param;
    const [gone, setGone] = (0,react.useState)(false);
    const a = prAttention(pr);
    const sessions = linkedSessionsOf(pr);
    const conflictChip = a.mergeConflict ? "Merge conflict" : a.conflictUnknown && !pr.isDraft && !a.needsAttention ? "? conflict" : null;
    if (gone) return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: prCard,
        "data-testid": "github-pr-card",
        "data-pr-key": prKey(pr),
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: prHeader,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                        className: prTitle,
                        href: pr.htmlUrl,
                        target: "_blank",
                        rel: "noreferrer",
                        "aria-label": "PR #".concat(pr.number, ": ").concat(pr.title),
                        children: pr.title
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: PRCard_css_chips,
                        children: [
                            pr.isDraft && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: chipDraft,
                                children: "Draft"
                            }),
                            !pr.isDraft && a.failingChecks > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)(FailingChecks, {
                                pr: pr,
                                count: a.failingChecks
                            }),
                            ciChip(pr, a),
                            a.changesRequested && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: chipError,
                                children: "Changes requested"
                            }),
                            pr.approvedCount > 0 && !a.changesRequested && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: chipSuccess,
                                children: [
                                    pr.approvedCount,
                                    " approved"
                                ]
                            }),
                            !pr.isDraft && /*#__PURE__*/ (0,jsx_runtime.jsx)(ThreadsChip, {
                                pr: pr,
                                a: a
                            }),
                            conflictChip && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: a.mergeConflict ? chipError : chipNeutral,
                                children: conflictChip
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: prMeta,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: prRepo,
                        children: [
                            pr.owner,
                            "/",
                            pr.repo,
                            " #",
                            pr.number
                        ]
                    }),
                    showHostAccount && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: hostAccountLabel,
                        "data-testid": "pr-host-account",
                        children: [
                            pr.host || "github.com",
                            " - ",
                            pr.accountLogin
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: prBranch,
                        children: [
                            pr.headRef,
                            " → ",
                            pr.baseRef
                        ]
                    }),
                    pr.localWorktreePath && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: worktreeLink,
                        title: pr.localWorktreePath,
                        children: pr.localWorktreePath.split("/").slice(-2).join("/")
                    })
                ]
            }),
            pr.detailsLoaded && /*#__PURE__*/ (0,jsx_runtime.jsx)(PRExpandedInfo, {
                pr: pr
            }),
            a.needsAttention && !a.nudgeable && a.changesRequested && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: changesRequestedNote,
                children: "A reviewer asked for changes. Open the PR on GitHub to read them; no automatic request is available."
            }),
            sessions.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                className: sessionList,
                "aria-label": "Linked sessions",
                children: sessions.map((s, i)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                        className: sessionRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: sessionName,
                                children: s.sessionId
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: sessionStatus,
                                children: [
                                    "(",
                                    sessionStatusText(s.status),
                                    i === 0 ? ", default" : "",
                                    ")"
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(nav/* Link */.N_, {
                                href: "/?session=".concat(encodeURIComponent(s.sessionId)),
                                className: openSessionButton,
                                "aria-label": "Open session ".concat(s.sessionId),
                                "data-testid": "open-session-link",
                                children: "Open session"
                            })
                        ]
                    }, "".concat(i, "-").concat(s.sessionId)))
            }),
            pr.linkedSessions.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)(NudgeButton, {
                pr: pr,
                sessions: sessions,
                nudgeable: a.nudgeable,
                client: nudgeClient,
                onNothingToFix: onNothingToFix
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: prActions,
                children: [
                    a.needsAttention && !a.nudgeable && a.changesRequested && /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                        className: createSessionButton,
                        href: pr.htmlUrl,
                        ...EXTERNAL,
                        "data-testid": "open-pr-on-github",
                        children: "Open PR on GitHub"
                    }),
                    sessions.length === 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: noSessionText,
                                children: "No session on this branch"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(nav/* Link */.N_, {
                                href: "/?pr=".concat(encodeURIComponent(pr.htmlUrl)),
                                className: createSessionButton,
                                "data-testid": "create-session-button",
                                children: "+ Session"
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(PRActionBar, {
                pr: pr,
                client: actionClient,
                onGone: ()=>{
                    setGone(true);
                    onNothingToFix === null || onNothingToFix === void 0 ? void 0 : onNothingToFix();
                },
                onChanged: onNothingToFix
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Funfinished%2Fprs%2FPRGroupedList.css.ts.vanilla.css%22%2C%22source%22%3A%22LnF1NHVidzAgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBmbGV4LXdyYXA6IHdyYXA7CiAgZ2FwOiB2YXIoLS1fMTllejJyejJ6KTsKICBtaW4taGVpZ2h0OiAwOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7Cn0KLnF1NHVidzEgewogIG1pbi1oZWlnaHQ6IDQ0cHg7CiAgcGFkZGluZzogdmFyKC0tXzE5ZXoycnoyeSkgdmFyKC0tXzE5ZXoycnozMCk7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnoxNyk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MTkpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6bCk7CiAgYm9yZGVyLXJhZGl1czogdmFyKC0tXzE5ZXoycnozNik7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNhKTsKICBmb250LXdlaWdodDogNjAwOwogIGN1cnNvcjogcG9pbnRlcjsKfQoucXU0dWJ3MTpmb2N1cy12aXNpYmxlIHsKICBvdXRsaW5lOiAycHggc29saWQgdmFyKC0tXzE5ZXoycnpsKTsKICBvdXRsaW5lLW9mZnNldDogMnB4Owp9Ci5xdTR1YncyIHsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXgtZGlyZWN0aW9uOiBjb2x1bW47CiAgZ2FwOiB2YXIoLS1fMTllejJyejJ6KTsKICBtaW4td2lkdGg6IDA7Cn0%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Funfinished_2Fprs_2FPRGroupedList_css_ts_vanilla_css_22_2C_22source_22_3A_22LnF1NHVidzAgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBmbGV4LXdyYXA6IHdyYXA7CiAgZ2FwOiB2YXIoLS1fMTllejJyejJ6KTsKICBtaW4taGVpZ2h0OiAwOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7Cn0KLnF1NHVidzEgewogIG1pbi1oZWlnaHQ6IDQ0cHg7CiAgcGFkZGluZzogdmFyKC0tXzE5ZXoycnoyeSkgdmFyKC0tXzE5ZXoycnozMCk7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnoxNyk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MTkpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6bCk7CiAgYm9yZGVyLXJhZGl1czogdmFyKC0tXzE5ZXoycnozNik7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNhKTsKICBmb250LXdlaWdodDogNjAwOwogIGN1cnNvcjogcG9pbnRlcjsKfQoucXU0dWJ3MTpmb2N1cy12aXNpYmxlIHsKICBvdXRsaW5lOiAycHggc29saWQgdmFyKC0tXzE5ZXoycnpsKTsKICBvdXRsaW5lLW9mZnNldDogMnB4Owp9Ci5xdTR1YncyIHsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXgtZGlyZWN0aW9uOiBjb2x1bW47CiAgZ2FwOiB2YXIoLS1fMTllejJyejJ6KTsKICBtaW4td2lkdGg6IDA7Cn0_22_7D = __webpack_require__(64352);
;// ./src/components/unfinished/prs/PRGroupedList.css.ts


var changesLine = 'qu4ubw0';
var groupList = 'qu4ubw2';
var refreshListButton = 'qu4ubw1';
;// ./src/components/unfinished/prs/PRGroupedList.tsx
// +feature: unfinished-github-prs
/* __next_internal_client_entry_do_not_use__ PRGroupedList auto */ 





const freeze = (groups, stateKey)=>({
        stateKey,
        groups: groups.map((g)=>({
                key: g.key,
                prs: g.prs
            }))
    });
const flatKeys = (groups)=>groups.flatMap((g)=>g.prs.map(prKey));
const countPRs = (groups)=>groups.reduce((n, g)=>n + g.prs.length, 0);
/** PRs added, removed, or at a different position between the frozen order and the live one. */ function changedCount(frozen, live) {
    const before = flatKeys(frozen.groups);
    const after = flatKeys(live);
    const beforeSet = new Set(before);
    const afterSet = new Set(after);
    const removed = before.filter((k)=>!afterSet.has(k)).length;
    const added = after.filter((k)=>!beforeSet.has(k)).length;
    const beforeCommon = before.filter((k)=>afterSet.has(k));
    const afterCommon = after.filter((k)=>beforeSet.has(k));
    const moved = beforeCommon.filter((k, i)=>afterCommon[i] !== k).length;
    return removed + added + moved;
}
function groupTitle(key) {
    return key.startsWith("github.com/") ? key.slice("github.com/".length) : key;
}
function spansMultipleHostsOrAccounts(prs) {
    const hosts = new Set(prs.map((p)=>(p.host || "github.com").toLowerCase()));
    const accounts = new Set(prs.map((p)=>"".concat(p.host, "|").concat(p.accountLogin)));
    return hosts.size > 1 || accounts.size > 1;
}
/**
 * Renders PRs grouped by repo. After the first paint of a given `stateKey` the ORDER and
 * MEMBERSHIP are frozen so the list does not shuffle under the user; card content still
 * updates in place, and a status line offers "Refresh list" to apply the new order.
 */ function PRGroupedList(param) {
    let { prs, sortBy, stateKey, fallbackFocusRef, onNothingToFix } = param;
    const liveGroups = (0,react.useMemo)(()=>orderPRs(prs, sortBy), [
        prs,
        sortBy
    ]);
    const liveByKey = (0,react.useMemo)(()=>new Map(prs.map((p)=>[
                prKey(p),
                p
            ])), [
        prs
    ]);
    const showHostAccount = (0,react.useMemo)(()=>spansMultipleHostsOrAccounts(prs), [
        prs
    ]);
    const [applied, setApplied] = (0,react.useState)(()=>freeze(liveGroups, stateKey));
    // An empty first paint has nothing to protect; follow live data until there is something.
    const followLive = applied.stateKey !== stateKey || countPRs(applied.groups) === 0;
    const frozen = followLive ? freeze(liveGroups, stateKey) : applied;
    if (followLive && countPRs(frozen.groups) > 0) setApplied(frozen);
    const containerRef = (0,react.useRef)(null);
    const lastFocusedKey = (0,react.useRef)(null);
    const pendingFocus = (0,react.useRef)(false);
    const onFocusCapture = (0,react.useCallback)((e)=>{
        const card = e.target.closest("[data-pr-key]");
        var _card_dataset_prKey;
        if (card) lastFocusedKey.current = (_card_dataset_prKey = card.dataset.prKey) !== null && _card_dataset_prKey !== void 0 ? _card_dataset_prKey : null;
    }, []);
    const applyLiveOrder = (0,react.useCallback)(()=>{
        pendingFocus.current = true;
        setApplied(freeze(liveGroups, stateKey));
    }, [
        liveGroups,
        stateKey
    ]);
    (0,react.useLayoutEffect)(()=>{
        var _containerRef_current, _this;
        if (!pendingFocus.current) return;
        pendingFocus.current = false;
        const key = lastFocusedKey.current;
        var _containerRef_current_querySelectorAll;
        const card = key ? Array.from((_containerRef_current_querySelectorAll = (_containerRef_current = containerRef.current) === null || _containerRef_current === void 0 ? void 0 : _containerRef_current.querySelectorAll("[data-pr-key]")) !== null && _containerRef_current_querySelectorAll !== void 0 ? _containerRef_current_querySelectorAll : []).find((el)=>el.dataset.prKey === key) : undefined;
        var _card_querySelector;
        (_this = (_card_querySelector = card === null || card === void 0 ? void 0 : card.querySelector("a")) !== null && _card_querySelector !== void 0 ? _card_querySelector : fallbackFocusRef === null || fallbackFocusRef === void 0 ? void 0 : fallbackFocusRef.current) === null || _this === void 0 ? void 0 : _this.focus();
    }, [
        applied,
        fallbackFocusRef
    ]);
    const changed = changedCount(frozen, liveGroups);
    const groups = frozen.groups;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        ref: containerRef,
        className: groupList,
        onFocusCapture: onFocusCapture,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                role: "status",
                className: changesLine,
                "data-testid": "pr-list-changes",
                children: changed > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                            children: [
                                changed,
                                " ",
                                changed === 1 ? "PR" : "PRs",
                                " changed."
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            type: "button",
                            className: refreshListButton,
                            onClick: applyLiveOrder,
                            children: "Refresh list"
                        })
                    ]
                })
            }),
            groups.map((group)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: repoGroupSection,
                    children: [
                        groups.length > 1 && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: repoGroupHeader,
                            children: groupTitle(group.key)
                        }),
                        group.prs.map((frozenPR)=>{
                            const key = prKey(frozenPR);
                            var _liveByKey_get;
                            return /*#__PURE__*/ (0,jsx_runtime.jsx)(PRCard, {
                                pr: (_liveByKey_get = liveByKey.get(key)) !== null && _liveByKey_get !== void 0 ? _liveByKey_get : frozenPR,
                                showHostAccount: showHostAccount,
                                onNothingToFix: onNothingToFix
                            }, key);
                        })
                    ]
                }, group.key))
        ]
    });
}

;// ./src/components/unfinished/GitHubPRsSection.tsx
// +feature: unfinished-github-prs
/* __next_internal_client_entry_do_not_use__ PRCard,PRGroupedList,GitHubPRsSection auto */ 














function useGitHubUserClient() {
    return (0,react.useMemo)(()=>{
        const transport = (0,connect_transport/* createConnectTransport */.w)({
            baseUrl: (0,config/* getApiBaseUrl */.h)(),
            interceptors: [
                (0,config/* createAuthInterceptor */._)()
            ]
        });
        return (0,promise_client/* createClient */.UU)(github_user_pb/* GitHubUserService */.aI, transport);
    }, []);
}
function StatsBar(param) {
    let { prs } = param;
    const ciFailures = prs.filter((p)=>p.checkConclusion === "failure" || p.checkConclusion === "error").length;
    const needsReview = prs.filter((p)=>p.changesReqCount > 0).length;
    const withSessions = prs.filter((p)=>p.sessionIds.length > 0).length;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: statsBar,
        "data-testid": "github-prs-stats",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                className: GitHubPRsSection_css_statItem,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: statCount,
                        children: prs.length
                    }),
                    " open"
                ]
            }),
            ciFailures > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                className: GitHubPRsSection_css_statItem,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: statCountError,
                        children: ciFailures
                    }),
                    " CI failing"
                ]
            }),
            needsReview > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                className: GitHubPRsSection_css_statItem,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: statCountWarning,
                        children: needsReview
                    }),
                    " changes requested"
                ]
            }),
            withSessions > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                className: GitHubPRsSection_css_statItem,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: statCount,
                        children: withSessions
                    }),
                    " with sessions"
                ]
            })
        ]
    });
}
function AccountsBar(param) {
    let { accounts, onDisconnect, onAddAccount } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: accountsRow,
        "data-testid": "github-accounts-row",
        children: [
            accounts.map((acc)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                    className: acc.isEnvToken ? accountChipEnv : accountChip,
                    title: acc.isEnvToken ? "Sourced from environment variable" : undefined,
                    children: [
                        "@",
                        acc.username,
                        acc.host && acc.host !== "github.com" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                            className: hostBadge,
                            children: [
                                "(",
                                acc.host,
                                ")"
                            ]
                        }),
                        !acc.isEnvToken && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: disconnectAccountButton,
                            onClick: ()=>onDisconnect(acc.username, acc.host),
                            "aria-label": "Disconnect ".concat(acc.username),
                            title: "Disconnect this account",
                            children: "\xd7"
                        })
                    ]
                }, "".concat(acc.host || "github.com", ":").concat(acc.username))),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                className: addAccountButton,
                onClick: onAddAccount,
                "data-testid": "add-github-account-button",
                children: "+ Add account"
            })
        ]
    });
}
function DeviceAuthBanner(param) {
    let { errorMessage, onAuthComplete, onCancel } = param;
    const client = useGitHubUserClient();
    const [flow, setFlow] = (0,react.useState)({
        kind: "idle"
    });
    const [host, setHost] = (0,react.useState)("");
    const pollTimerRef = (0,react.useRef)(null);
    const abortRef = (0,react.useRef)(false);
    (0,react.useEffect)(()=>{
        abortRef.current = false;
        return ()=>{
            abortRef.current = true;
            if (pollTimerRef.current) clearTimeout(pollTimerRef.current);
        };
    }, []);
    const handleConnect = (0,react.useCallback)(async ()=>{
        setFlow({
            kind: "starting"
        });
        try {
            const res = await client.startGitHubDeviceAuth((0,create/* create */.v)(github_user_pb/* StartGitHubDeviceAuthRequestSchema */.SH, {
                host: host.trim()
            }));
            if (abortRef.current) return;
            setFlow({
                kind: "waiting",
                userCode: res.userCode,
                verificationUri: res.verificationUri,
                deviceCode: res.deviceCode
            });
        } catch (err) {
            if (!abortRef.current) {
                setFlow({
                    kind: "error",
                    message: String(err)
                });
            }
        }
    }, [
        client,
        host
    ]);
    const schedulePoll = (0,react.useCallback)((deviceCode, userCode, verificationUri, intervalMs)=>{
        if (abortRef.current) return;
        pollTimerRef.current = setTimeout(async ()=>{
            if (abortRef.current) return;
            try {
                const res = await client.pollGitHubDeviceAuth((0,create/* create */.v)(github_user_pb/* PollGitHubDeviceAuthRequestSchema */.nQ, {
                    deviceCode
                }));
                if (abortRef.current) return;
                if (res.status === github_user_pb/* DeviceAuthStatus */.X_.COMPLETE) {
                    setFlow({
                        kind: "complete"
                    });
                    onAuthComplete();
                } else if (res.status === github_user_pb/* DeviceAuthStatus */.X_.EXPIRED) {
                    setFlow({
                        kind: "expired"
                    });
                } else if (res.status === github_user_pb/* DeviceAuthStatus */.X_.ERROR) {
                    setFlow({
                        kind: "error",
                        message: res.error || "Unknown error"
                    });
                } else {
                    setFlow({
                        kind: "polling",
                        userCode,
                        verificationUri,
                        deviceCode
                    });
                    schedulePoll(deviceCode, userCode, verificationUri, intervalMs);
                }
            } catch (err) {
                if (!abortRef.current) {
                    setFlow({
                        kind: "error",
                        message: String(err)
                    });
                }
            }
        }, intervalMs);
    }, [
        client,
        onAuthComplete
    ]);
    const handleStartPolling = (0,react.useCallback)(()=>{
        if (flow.kind !== "waiting") return;
        const { deviceCode, userCode, verificationUri } = flow;
        setFlow({
            kind: "polling",
            userCode,
            verificationUri,
            deviceCode
        });
        schedulePoll(deviceCode, userCode, verificationUri, 5000);
    }, [
        flow,
        schedulePoll
    ]);
    const handleReset = (0,react.useCallback)(()=>{
        if (pollTimerRef.current) clearTimeout(pollTimerRef.current);
        setFlow({
            kind: "idle"
        });
        onCancel === null || onCancel === void 0 ? void 0 : onCancel();
    }, [
        onCancel
    ]);
    if (flow.kind === "idle") {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: authBanner,
            "data-testid": "github-auth-banner",
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    className: authBannerText,
                    children: errorMessage || "Connect GitHub to see your open PRs"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                    className: hostInput,
                    value: host,
                    onChange: (e)=>setHost(e.target.value),
                    placeholder: "github.com",
                    "aria-label": "GitHub host",
                    "data-testid": "github-host-input"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    className: connectButton,
                    onClick: handleConnect,
                    "data-testid": "github-connect-button",
                    children: "Connect GitHub"
                })
            ]
        });
    }
    if (flow.kind === "starting") {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: authBanner,
            "data-testid": "github-auth-banner",
            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: authBannerText,
                children: "Starting device auth…"
            })
        });
    }
    if (flow.kind === "waiting" || flow.kind === "polling") {
        const isPolling = flow.kind === "polling";
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: deviceFlowCard,
            "data-testid": "github-device-flow",
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: deviceFlowInstructions,
                    children: isPolling ? "Waiting for authorization…" : "Open the link below and enter your code:"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: deviceFlowRow,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                            href: flow.verificationUri,
                            target: "_blank",
                            rel: "noreferrer",
                            className: verificationLink,
                            "data-testid": "github-verification-link",
                            children: flow.verificationUri
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: userCode,
                            "data-testid": "github-user-code",
                            children: flow.userCode
                        })
                    ]
                }),
                !isPolling && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    className: connectButton,
                    onClick: handleStartPolling,
                    "data-testid": "github-authorized-button",
                    children: "I've authorized — continue"
                }),
                isPolling && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    className: pollingIndicator,
                    children: "Checking…"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    className: cancelButton,
                    onClick: handleReset,
                    children: "Cancel"
                })
            ]
        });
    }
    if (flow.kind === "complete") {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: authBannerSuccess,
            "data-testid": "github-auth-success",
            children: "GitHub connected successfully."
        });
    }
    if (flow.kind === "expired") {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: authBanner,
            "data-testid": "github-auth-banner",
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    className: authBannerText,
                    children: "Device code expired."
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    className: connectButton,
                    onClick: handleReset,
                    children: "Try again"
                })
            ]
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: authBanner,
        "data-testid": "github-auth-banner",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                className: authBannerText,
                children: [
                    "Auth error: ",
                    flow.kind === "error" ? flow.message : ""
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                className: connectButton,
                onClick: handleReset,
                children: "Try again"
            })
        ]
    });
}
function AddAccountPanel(param) {
    let { errorMessage, onAuthComplete, onCancel } = param;
    const [mode, setMode] = (0,react.useState)("device");
    const [cliAvailable, setCliAvailable] = (0,react.useState)(false);
    const [manualExpanded, setManualExpanded] = (0,react.useState)(false);
    const modeChosenRef = (0,react.useRef)(false);
    const handleCliAvailabilityChange = (0,react.useCallback)((available)=>{
        setCliAvailable(available);
        // Once CLI credentials are importable, prefer the token tab over device flow —
        // there's no reason to walk through device flow when credentials are already local.
        if (available && !modeChosenRef.current) {
            setMode("token");
        }
    }, []);
    const showManualAuth = !cliAvailable || manualExpanded;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: addAccountPanel,
        "data-testid": "github-add-account-panel",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)(CLIImportSection, {
                onAuthComplete: onAuthComplete,
                onAvailabilityChange: handleCliAvailabilityChange
            }),
            cliAvailable && !manualExpanded && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                className: cancelButton,
                onClick: ()=>setManualExpanded(true),
                "data-testid": "github-add-account-manual-toggle",
                children: "Set up manually instead"
            }),
            showManualAuth && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: authTabs,
                        role: "tablist",
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                role: "tab",
                                "aria-selected": mode === "device",
                                className: mode === "device" ? authTabActive : authTab,
                                onClick: ()=>{
                                    modeChosenRef.current = true;
                                    setMode("device");
                                },
                                "data-testid": "github-auth-tab-device",
                                children: "Device flow"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                role: "tab",
                                "aria-selected": mode === "token",
                                className: mode === "token" ? authTabActive : authTab,
                                onClick: ()=>{
                                    modeChosenRef.current = true;
                                    setMode("token");
                                },
                                "data-testid": "github-auth-tab-token",
                                children: "Personal access token"
                            })
                        ]
                    }),
                    mode === "device" ? /*#__PURE__*/ (0,jsx_runtime.jsx)(DeviceAuthBanner, {
                        errorMessage: errorMessage,
                        onAuthComplete: onAuthComplete,
                        onCancel: onCancel
                    }) : /*#__PURE__*/ (0,jsx_runtime.jsx)(TokenAuthForm, {
                        onAuthComplete: onAuthComplete,
                        onCancel: onCancel
                    })
                ]
            })
        ]
    });
}
function CLIImportSection(param) {
    let { onAuthComplete, onAvailabilityChange } = param;
    const client = useGitHubUserClient();
    const [state, setState] = (0,react.useState)({
        kind: "loading"
    });
    const [importingHost, setImportingHost] = (0,react.useState)(null);
    (0,react.useEffect)(()=>{
        let cancelled = false;
        client.listGitHubCLIHosts((0,create/* create */.v)(github_user_pb/* ListGitHubCLIHostsRequestSchema */.Rx, {})).then((res)=>{
            if (cancelled) return;
            if (!res.ghAvailable || res.hosts.length === 0) {
                setState({
                    kind: "unavailable"
                });
                onAvailabilityChange === null || onAvailabilityChange === void 0 ? void 0 : onAvailabilityChange(false);
            } else {
                setState({
                    kind: "ready",
                    hosts: res.hosts
                });
                onAvailabilityChange === null || onAvailabilityChange === void 0 ? void 0 : onAvailabilityChange(true);
            }
        }).catch(()=>{
            if (!cancelled) {
                setState({
                    kind: "unavailable"
                });
                onAvailabilityChange === null || onAvailabilityChange === void 0 ? void 0 : onAvailabilityChange(false);
            }
        });
        return ()=>{
            cancelled = true;
        };
    // onAvailabilityChange is expected to be a stable setState-style callback from the parent
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        client
    ]);
    const handleImport = (0,react.useCallback)(async (host)=>{
        setImportingHost(host);
        try {
            await client.addGitHubAccountFromCLI((0,create/* create */.v)(github_user_pb/* AddGitHubAccountFromCLIRequestSchema */.it, {
                host
            }));
            onAuthComplete();
        } catch (err) {
            setState({
                kind: "error",
                message: String(err)
            });
        } finally{
            setImportingHost(null);
        }
    }, [
        client,
        onAuthComplete
    ]);
    if (state.kind === "loading" || state.kind === "unavailable") {
        return null;
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: cliImportSection,
        "data-testid": "github-cli-import-section",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: cliImportLabel,
                children: "Already logged in via gh CLI:"
            }),
            state.kind === "ready" && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: cliImportHostList,
                children: state.hosts.map((h)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: cliImportHostButton,
                        disabled: h.alreadyAdded || importingHost === h.host,
                        onClick: ()=>handleImport(h.host),
                        "data-testid": "github-cli-import-host-".concat(h.host),
                        children: h.alreadyAdded ? "".concat(h.host).concat(h.username ? " (".concat(h.username, ")") : "", " — connected") : importingHost === h.host ? "Importing ".concat(h.host, "…") : "Import ".concat(h.host).concat(h.username ? " (".concat(h.username, ")") : "")
                    }, h.host))
            }),
            state.kind === "error" && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: authError,
                "data-testid": "github-cli-import-error",
                children: state.message
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: cliImportDivider,
                children: "or paste a token manually"
            })
        ]
    });
}
function TokenAuthForm(param) {
    let { onAuthComplete, onCancel } = param;
    const client = useGitHubUserClient();
    const [host, setHost] = (0,react.useState)("");
    const [token, setToken] = (0,react.useState)("");
    const [status, setStatus] = (0,react.useState)({
        kind: "idle"
    });
    const handleSubmit = (0,react.useCallback)(async (e)=>{
        e.preventDefault();
        setStatus({
            kind: "submitting"
        });
        try {
            await client.addGitHubAccountWithToken((0,create/* create */.v)(github_user_pb/* AddGitHubAccountWithTokenRequestSchema */.$Y, {
                host: host.trim(),
                token: token.trim()
            }));
            onAuthComplete();
        } catch (err) {
            setStatus({
                kind: "error",
                message: String(err)
            });
        }
    }, [
        client,
        host,
        token,
        onAuthComplete
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("form", {
        className: deviceFlowCard,
        onSubmit: handleSubmit,
        "data-testid": "github-token-auth-form",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                className: hostInput,
                value: host,
                onChange: (e)=>setHost(e.target.value),
                placeholder: "github.com",
                "aria-label": "GitHub host",
                "data-testid": "github-token-host-input"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                type: "password",
                className: hostInput,
                value: token,
                onChange: (e)=>setToken(e.target.value),
                placeholder: "ghp_… or a GHES personal access token",
                "aria-label": "Personal access token",
                "data-testid": "github-token-input"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "submit",
                className: connectButton,
                disabled: status.kind === "submitting" || !token.trim(),
                "data-testid": "github-token-submit-button",
                children: status.kind === "submitting" ? "Validating…" : "Connect with token"
            }),
            onCancel && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                className: cancelButton,
                onClick: onCancel,
                children: "Cancel"
            }),
            status.kind === "error" && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: authError,
                "data-testid": "github-token-auth-error",
                children: status.message
            })
        ]
    });
}
// --- Filter / sort bar ---
const STATUS_FILTERS = [
    {
        value: "all",
        label: "All"
    },
    {
        value: "ci-failing",
        label: "CI failing"
    },
    {
        value: "changes-requested",
        label: "Changes req"
    },
    {
        value: "with-session",
        label: "Has session"
    },
    {
        value: "draft",
        label: "Draft"
    },
    {
        value: "needs-attention",
        label: "Needs attention"
    }
];
const SORT_OPTIONS = [
    {
        value: "updated-desc",
        label: "Updated down"
    },
    {
        value: "updated-asc",
        label: "Updated up"
    },
    {
        value: "repo",
        label: "Repo A-Z"
    },
    {
        value: "ci-status",
        label: "CI status"
    },
    {
        value: "attention-first",
        label: "Attention first"
    }
];
function FilterBar(param) {
    let { filter, sort, search, searchRef, clearActive, onFilter, onSort, onSearch, onClear } = param;
    const sortId = (0,react.useId)();
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: filterBar,
        "data-testid": "github-prs-filter-bar",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: filterChipGroup,
                role: "group",
                "aria-label": "Filter PRs",
                children: STATUS_FILTERS.map((f)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: filter === f.value ? filterChipActive : filterChip,
                        onClick: ()=>onFilter(f.value),
                        "aria-pressed": filter === f.value,
                        "data-testid": "filter-chip-".concat(f.value),
                        children: f.label
                    }, f.value))
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: sortGroup,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        ref: searchRef,
                        type: "search",
                        className: searchInput,
                        placeholder: "Search PRs…",
                        value: search,
                        onChange: (e)=>onSearch(e.target.value),
                        "aria-label": "Search pull requests",
                        "data-testid": "github-prs-search"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                        className: sortLabel,
                        htmlFor: sortId,
                        children: "Sort"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("select", {
                        id: sortId,
                        className: sortSelect,
                        value: sort,
                        onChange: (e)=>onSort(e.target.value),
                        "data-testid": "github-prs-sort",
                        children: SORT_OPTIONS.map((o)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                value: o.value,
                                children: o.label
                            }, o.value))
                    }),
                    clearActive && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: panelButton,
                        onClick: onClear,
                        "data-testid": "clear-filters",
                        children: "Clear filters"
                    })
                ]
            })
        ]
    });
}
// --- Account status banners ---
const accountLabel = (st)=>"".concat(st.accountLogin, " on ").concat(st.host);
/** One banner per failed account; healthy accounts' cards still render. Never the "not connected" state. */ function AccountBanners(param) {
    let { statuses, onReconnect } = param;
    const failed = statuses.filter((st)=>st.state !== github_user_pb/* AccountPollState */.m$.OK && st.state !== github_user_pb/* AccountPollState */.m$.UNSPECIFIED);
    if (failed.length === 0) return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(jsx_runtime.Fragment, {
        children: failed.map((st)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                role: "status",
                className: banner,
                "data-testid": "github-account-banner",
                children: [
                    st.state === github_user_pb/* AccountPollState */.m$.UNAUTHORIZED && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                children: [
                                    "GitHub sign-in expired for ",
                                    accountLabel(st),
                                    ". "
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: panelButton,
                                onClick: onReconnect,
                                children: "Reconnect"
                            })
                        ]
                    }),
                    st.state === github_user_pb/* AccountPollState */.m$.RATE_LIMITED && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        children: [
                            "GitHub rate limit reached for ",
                            accountLabel(st),
                            "."
                        ]
                    }),
                    st.state === github_user_pb/* AccountPollState */.m$.ERROR && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        children: [
                            "Could not refresh ",
                            accountLabel(st),
                            st.detail ? ": ".concat(st.detail) : "."
                        ]
                    })
                ]
            }, "".concat(st.host, "|").concat(st.accountLogin)))
    });
}
/**
 * Displays the authenticated GitHub user's open pull requests.
 * Shows connected accounts, aggregated stats, and PRs grouped by repo.
 */ function GitHubPRsSection(param) {
    let { prs, authState, accountStatuses, lastUpdatedAt, error, refreshing, refresh, filters, attention } = param;
    const client = useGitHubUserClient();
    const [isOpen, setIsOpen] = (0,react.useState)(true);
    const [addingAccount, setAddingAccount] = (0,react.useState)(false);
    const { filterStatus, sortBy, searchQuery, setFilterStatus, setSortBy, setSearchQuery } = filters;
    const now = (0,useNowTicker/* useNowTicker */.w)();
    const headingRef = (0,react.useRef)(null);
    const searchRef = (0,react.useRef)(null);
    const retryPendingRef = (0,react.useRef)(false);
    const mountedAtRef = (0,react.useRef)(performance.now());
    const firstCardEmittedRef = (0,react.useRef)(false);
    const visiblePRs = (0,react.useMemo)(()=>filters.apply(prs), [
        filters,
        prs
    ]);
    (0,react.useEffect)(()=>{
        if (!firstCardEmittedRef.current && isOpen && visiblePRs.length > 0) {
            firstCardEmittedRef.current = true;
            emitTabEvent({
                type: "firstPrCard",
                ms: Math.round(performance.now() - mountedAtRef.current)
            });
        }
    }, [
        isOpen,
        visiblePRs.length
    ]);
    // Retry succeeded (error cleared) => land on the list heading, never on <body>.
    (0,react.useEffect)(()=>{
        if (!error && retryPendingRef.current) {
            var _headingRef_current;
            retryPendingRef.current = false;
            (_headingRef_current = headingRef.current) === null || _headingRef_current === void 0 ? void 0 : _headingRef_current.focus();
        }
    }, [
        error
    ]);
    const handleRefresh = (0,react.useCallback)(()=>{
        if (!refreshing) refresh();
    }, [
        refreshing,
        refresh
    ]);
    const handleRetry = (0,react.useCallback)(()=>{
        retryPendingRef.current = true;
        refresh();
    }, [
        refresh
    ]);
    const handleClearFilters = (0,react.useCallback)(()=>{
        var _searchRef_current;
        filters.clear();
        (_searchRef_current = searchRef.current) === null || _searchRef_current === void 0 ? void 0 : _searchRef_current.focus();
    }, [
        filters
    ]);
    const toggleOpen = (0,react.useCallback)(()=>setIsOpen((v)=>!v), []);
    const handleKeyDown = (e)=>{
        if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            toggleOpen();
        }
    };
    const handleDisconnect = (0,react.useCallback)(async (username, host)=>{
        await client.revokeGitHubToken((0,create/* create */.v)(github_user_pb/* RevokeGitHubTokenRequestSchema */.LV, {
            username,
            host
        }));
        refresh();
    }, [
        client,
        refresh
    ]);
    const handleAddAccount = (0,react.useCallback)(()=>{
        setAddingAccount(true);
    }, []);
    const handleAddAccountComplete = (0,react.useCallback)(()=>{
        setAddingAccount(false);
        refresh();
    }, [
        refresh
    ]);
    const handleAddAccountCancel = (0,react.useCallback)(()=>{
        setAddingAccount(false);
    }, []);
    const authUnavailable = authState && !authState.available;
    var _authState_accounts;
    const accounts = (_authState_accounts = authState === null || authState === void 0 ? void 0 : authState.accounts) !== null && _authState_accounts !== void 0 ? _authState_accounts : [];
    var _authState_errorMessage;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
        className: section,
        "aria-label": "GitHub Pull Requests",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                role: "button",
                tabIndex: 0,
                className: sectionHeader,
                onClick: toggleOpen,
                onKeyDown: handleKeyDown,
                "aria-expanded": isOpen,
                "aria-controls": "github-prs-list",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: "".concat(GitHubPRsSection_css_chevron, " ").concat(isOpen ? GitHubPRsSection_css_chevronExpanded : ""),
                        "aria-hidden": "true",
                        children: "▶"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: sectionTitle,
                        children: "GitHub Pull Requests"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: badge,
                        children: prs.length
                    })
                ]
            }),
            attention.count > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                className: attentionNote,
                "data-testid": "github-prs-attention",
                children: [
                    attention.count,
                    attention.degraded ? "+" : "",
                    " need attention",
                    attention.degraded && " ".concat(DEGRADED_ATTENTION_TEXT)
                ]
            }),
            isOpen && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                id: "github-prs-list",
                children: authUnavailable && !addingAccount ? /*#__PURE__*/ (0,jsx_runtime.jsx)(AddAccountPanel, {
                    errorMessage: (_authState_errorMessage = authState === null || authState === void 0 ? void 0 : authState.errorMessage) !== null && _authState_errorMessage !== void 0 ? _authState_errorMessage : "",
                    onAuthComplete: refresh
                }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: panelHeader,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                                    className: panelHeading,
                                    tabIndex: -1,
                                    ref: headingRef,
                                    children: "Open pull requests"
                                }),
                                lastUpdatedAt !== undefined && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    className: freshness,
                                    "data-testid": "github-prs-updated",
                                    children: [
                                        "Updated ",
                                        formatRelativeTime(lastUpdatedAt, now)
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "button",
                                    className: panelButton,
                                    "aria-disabled": refreshing,
                                    "aria-busy": refreshing,
                                    onClick: handleRefresh,
                                    "data-testid": "github-prs-refresh",
                                    children: refreshing ? "Refreshing..." : "Refresh"
                                })
                            ]
                        }),
                        error && prs.length === 0 && lastUpdatedAt === undefined && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            role: "alert",
                            className: "".concat(banner, " ").concat(bannerError),
                            "data-testid": "github-prs-error",
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    children: [
                                        "Could not load PRs: ",
                                        error
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "button",
                                    className: panelButton,
                                    onClick: handleRetry,
                                    children: "Retry"
                                })
                            ]
                        }),
                        error && (prs.length > 0 || lastUpdatedAt !== undefined) && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            role: "status",
                            className: "".concat(banner, " ").concat(bannerError),
                            "data-testid": "github-prs-error",
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    children: [
                                        lastUpdatedAt !== undefined ? "Showing data from ".concat(formatRelativeTime(lastUpdatedAt, now), ". ") : "",
                                        "Could not refresh: ",
                                        error
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "button",
                                    className: panelButton,
                                    onClick: handleRetry,
                                    children: "Retry"
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)(AccountBanners, {
                            statuses: accountStatuses,
                            onReconnect: handleAddAccount
                        }),
                        accounts.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)(AccountsBar, {
                            accounts: accounts,
                            onDisconnect: handleDisconnect,
                            onAddAccount: handleAddAccount
                        }),
                        addingAccount && /*#__PURE__*/ (0,jsx_runtime.jsx)(AddAccountPanel, {
                            errorMessage: "",
                            onAuthComplete: handleAddAccountComplete,
                            onCancel: handleAddAccountCancel
                        }),
                        prs.length === 0 && !addingAccount ? authState === undefined && !error ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: empty,
                            children: "Connecting to GitHub…"
                        }) : authState === undefined ? null : /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: empty,
                            children: "No open PRs"
                        }) : prs.length > 0 ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)(StatsBar, {
                                    prs: prs
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)(FilterBar, {
                                    filter: filterStatus,
                                    sort: sortBy,
                                    search: searchQuery,
                                    searchRef: searchRef,
                                    clearActive: filters.isActive,
                                    onFilter: setFilterStatus,
                                    onSort: setSortBy,
                                    onSearch: setSearchQuery,
                                    onClear: handleClearFilters
                                }),
                                visiblePRs.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: emptyState,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            children: "No PRs match your filters"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                            type: "button",
                                            className: panelButton,
                                            onClick: handleClearFilters,
                                            "data-testid": "clear-filters-empty",
                                            children: "Clear filters"
                                        })
                                    ]
                                }) : /*#__PURE__*/ (0,jsx_runtime.jsx)(PRGroupedList, {
                                    prs: visiblePRs,
                                    sortBy: sortBy,
                                    stateKey: "".concat(filterStatus, "|").concat(sortBy, "|").concat(searchQuery),
                                    fallbackFocusRef: headingRef,
                                    onNothingToFix: refresh
                                })
                            ]
                        }) : null
                    ]
                })
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/hooks/useBacklogService.ts
var useBacklogService = __webpack_require__(8570);
// EXTERNAL MODULE: ./src/components/backlog/GitHubIssuePicker.tsx + 3 modules
var GitHubIssuePicker = __webpack_require__(85529);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Funfinished%2FBacklogQueueSection.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61Wy3KjMBC8-yt02ar4IJeEHT-Uj9mS0QDKgkRJwmBv5d-3BIRgDJjK5ohmmGn1dA9ssl1WnSVBf1cIZdzEUuEUIscQL5x-WyGUcyGkihm6cPOC8W96gltgbsF1PTjZkrXPP_PwT2x0oQRDznBlc25AOR8KdarNsBBt3tJGgGGI5hWyOpVikAW9LGy4kIUdFtru65xIK4etvMFDnH_FS5Bx4hjaE1IDK4z1yHItlQPjj8pEOsA25yEwpHRpeP62-lhtWr5Yoi9gatZaUKOXy9aT9_ahrh6tKwlp85RfGYpSqGqsKVRYSAOhk1oxX6jIlI_EPH-YyG3dm1c7xQEFtN80GG_KUxkrLB1klqEQPgkZ6Tgy8fuEw4_N9rCe0eKWPGixUZWDymEBoTa8IVBpBXXAC1M2Z_35IbKhr7bP0cJByz6x2zp9WojnaSGO1W5YrrUQG10yRHu9dk96zYh-rFfwZKLhD7u1m2iQV-N6njfi6_LrRzyT6fXhwsW0RYMFAPY1gClh0glhzrSbl03X9zDbd8oQdxt-OTL6TBZ02RanZLHVF9JwbGjQn2aOZAXCvy-VBccQucc9bt5GJEsX4XthnYyuONTKgXL90A1LJaBiiBLvsQ7lqdkfT2CcFnH4voDCY5tTYZtw4VfGICGbXadBq5UKl1K4hKHdkeQ1Je1zyNPwhRLyC2EUGMi6_KRdMUdySfyZ351Rqkt8_fqh-L-vXP3N6YjlT-wffmfTNiaptfPxDwqp9AMgCQAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Funfinished_2FBacklogQueueSection_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61Wy3KjMBC8_yt02ar4IJeEHT_Uj9mS0QDKgkRJwmBv5d_3BIRgDJjK5ohmmGn1dA9ssl1WnSVBf1cIZdzEUuEUIscQL5x_WyGUcyGkihm6cPOC8W96gltgbsF1PTjZkrXPP_PwT2x0oQRDznBlc25AOR8KdarNsBBt3tJGgGGI5hWyOpVikAW9LGy4kIUdFtru65xIK4etvMFDnH_FS5Bx4hjaE1IDK4z1yHItlQPjj8pEOsA25yEwpHRpeP62_lhtWr5Yoi9gatZaUKOXy9aT9_ahrh6tKwlp85RfGYpSqGqsKVRYSAOhk1oxX6jIlI_EPH_YyG3dm1c7xQEFtN80GG_KUxkrLB1klqEQPgkZ6Tgy8fuEw4_N9rCe0eKWPGixUZWDymEBoTa8IVBpBXXAC1M2Z_35IbKhr7bP0cJByz6x2zp9WojnaSGO1W5YrrUQG10yRHu9dk96zYh_rFfwZKLhD7u1m2iQV_N6njfi6_LrRzyT6fXhwsW0RYMFAPY1gClh0glhzrSbl03X9zDbd8oQdxt_OTL6TBZ02RanZLHVF9JwbGjQn2aOZAXCvy_VBccQucc9bt5GJEsX4XthnYyuONTKgXL90A1LJaBiiBLvsQ7lqdkfT2CcFnH4voDCY5tTYZtw4VfGICGbXadBq5UKl1K4hKHdkeQ1Je1zyNPwhRLyC2EUGMi6_KRdMUdySfyZ351Rqkt8_fqh_L_vXP3N6YjlT_wffmfTNiaptfPxDwqp9AMgCQAA_22_7D = __webpack_require__(54345);
;// ./src/components/unfinished/BacklogQueueSection.css.ts



var BacklogQueueSection_css_badge = 'w8p1z05';
var BacklogQueueSection_css_card = 'm4mxbi2';
var cardTitle = 'm4mxbi3';
var BacklogQueueSection_css_chevron = 'w8p1z02';
var BacklogQueueSection_css_chevronExpanded = 'w8p1z03';
var dialog = 'm4mxbi9';
var dialogHeading = 'm4mxbia';
var BacklogQueueSection_css_empty = 'm4mxbi6';
var errorBox = 'm4mxbi7';
var importButton = 'm4mxbi0';
var list = 'm4mxbi1';
var BacklogQueueSection_css_overlay = 'm4mxbi8';
var priorityChip = 'm4mxbi5';
var BacklogQueueSection_css_section = 'w8p1z00';
var BacklogQueueSection_css_sectionHeader = 'w8p1z01';
var BacklogQueueSection_css_sectionTitle = 'w8p1z04';
var statusChip = 'm4mxbi4';
;// ./src/components/unfinished/BacklogQueueSection.tsx
// +feature: unfinished-backlog-queue
/* __next_internal_client_entry_do_not_use__ BacklogQueueSection auto */ 







const QUEUED_STATUSES = [
    "idea",
    "refining",
    "ready"
];
function statusLabel(status) {
    switch(status){
        case "idea":
            return "Idea";
        case "refining":
            return "Refining";
        case "ready":
            return "Ready";
        default:
            return status;
    }
}
function QueueCard(param) {
    let { item } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(nav/* Link */.N_, {
        href: "/backlog?item=".concat(encodeURIComponent(item.id)),
        className: BacklogQueueSection_css_card,
        "data-testid": "backlog-queue-card",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: cardTitle,
                children: item.title
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                className: priorityChip,
                children: [
                    "P",
                    item.priority
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: statusChip,
                children: statusLabel(item.status)
            })
        ]
    });
}
/**
 * "Up Next" section on the Unfinished page: surfaces queued backlog items
 * (idea/refining/ready) and lets the user import a GitHub issue as a new
 * task, mirroring the collapsible-section pattern used by GitHubPRsSection.
 */ function BacklogQueueSection() {
    const { listBacklogItems, importGitHubIssue } = (0,useBacklogService/* useBacklogService */.gG)();
    const [items, setItems] = (0,react.useState)([]);
    const [loading, setLoading] = (0,react.useState)(true);
    const [error, setError] = (0,react.useState)(null);
    const [isOpen, setIsOpen] = (0,react.useState)(true);
    const [showImport, setShowImport] = (0,react.useState)(false);
    const headingId = (0,react.useId)();
    const importDialogRef = (0,react.useRef)(null);
    // No Escape-to-close: pre-existing behavior, unchanged by this fix (which
    // scopes to wiring the Tab trap, not adding new dismiss affordances) — use
    // the dialog's own Cancel button to close it.
    (0,useFocusTrap/* useFocusTrap */.r)(importDialogRef, showImport);
    // ponytail: request-id guard, not AbortController — listBacklogItems has no signal param
    const loadRequestIdRef = (0,react.useRef)(0);
    const load = (0,react.useCallback)(async ()=>{
        const requestId = ++loadRequestIdRef.current;
        setLoading(true);
        setError(null);
        try {
            const result = await listBacklogItems({
                statuses: [
                    ...QUEUED_STATUSES
                ]
            });
            if (loadRequestIdRef.current === requestId) setItems(result);
        } catch (e) {
            if (loadRequestIdRef.current === requestId) setError("Failed to load queued backlog items.");
        } finally{
            if (loadRequestIdRef.current === requestId) setLoading(false);
        }
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
    (0,react.useEffect)(()=>{
        load();
    }, [
        load
    ]);
    const toggleOpen = (0,react.useCallback)(()=>setIsOpen((v)=>!v), []);
    const handleKeyDown = (e)=>{
        if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            toggleOpen();
        }
    };
    // ponytail: generic failure message, same pattern as app/backlog/page.tsx's
    // handlePickerSelect — the hook's `lastError` state updates asynchronously,
    // so reading it synchronously right after this await would race with it.
    const handlePickerSelect = (0,react.useCallback)(async (owner, repo, issues)=>{
        setShowImport(false);
        let successCount = 0;
        let claimedCount = 0;
        for (const issue of issues){
            const url = issue.url || "https://".concat(issue.host || "github.com", "/").concat(owner, "/").concat(repo, "/issues/").concat(issue.number);
            const result = await importGitHubIssue(url);
            if (result === null || result === void 0 ? void 0 : result.alreadyClaimedElsewhere) claimedCount++;
            else if (result) successCount++;
        }
        if (successCount > 0) await load();
        if (claimedCount > 0) {
            setError("".concat(claimedCount, " issue").concat(claimedCount === 1 ? " is" : "s are", " already claimed by another host. Use Backlog › Import from GitHub Issue to import anyway."));
        } else if (successCount < issues.length) {
            const failed = issues.length - successCount;
            setError(issues.length === 1 ? "Failed to import GitHub issue. Pull requests can't be imported as backlog items." : "Imported ".concat(successCount, " of ").concat(issues.length, " issues — ").concat(failed, " failed. Pull requests can't be imported as backlog items."));
        }
    }, [
        importGitHubIssue,
        load
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
        className: BacklogQueueSection_css_section,
        "aria-label": "Up Next",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                role: "button",
                tabIndex: 0,
                className: BacklogQueueSection_css_sectionHeader,
                onClick: toggleOpen,
                onKeyDown: handleKeyDown,
                "aria-expanded": isOpen,
                "aria-controls": "backlog-queue-list",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: "".concat(BacklogQueueSection_css_chevron, " ").concat(isOpen ? BacklogQueueSection_css_chevronExpanded : ""),
                        "aria-hidden": "true",
                        children: "▶"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: BacklogQueueSection_css_sectionTitle,
                        children: "Up Next"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: BacklogQueueSection_css_badge,
                        children: items.length
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: importButton,
                        onClick: (e)=>{
                            e.stopPropagation();
                            setShowImport(true);
                        },
                        onKeyDown: (e)=>e.stopPropagation(),
                        "data-testid": "import-github-issue-button",
                        children: "+ Import GitHub Issue"
                    })
                ]
            }),
            isOpen && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                id: "backlog-queue-list",
                children: error ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: errorBox,
                    children: error
                }) : loading ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: BacklogQueueSection_css_empty,
                    children: "Loading…"
                }) : items.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: BacklogQueueSection_css_empty,
                    children: "No queued backlog items. Import a GitHub issue to get started."
                }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: list,
                    children: items.map((item)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(QueueCard, {
                            item: item
                        }, item.id))
                })
            }),
            showImport && typeof document !== "undefined" && /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: BacklogQueueSection_css_overlay,
                "data-testid": "backlog-queue-import-modal",
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    ref: importDialogRef,
                    role: "dialog",
                    "aria-modal": "true",
                    "aria-labelledby": headingId,
                    className: dialog,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                            id: headingId,
                            className: dialogHeading,
                            children: "Import GitHub Issue"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)(GitHubIssuePicker/* GitHubIssuePicker */.n, {
                            onSelect: handlePickerSelect,
                            onCancel: ()=>setShowImport(false)
                        })
                    ]
                })
            }), document.body)
        ]
    });
}

// EXTERNAL MODULE: ./src/gen/session/v1/backlog_pb.ts
var backlog_pb = __webpack_require__(79870);
// EXTERNAL MODULE: ./src/lib/hooks/useStuckBacklogItems.ts
var useStuckBacklogItems = __webpack_require__(77717);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/codegenv2/file.js
var file = __webpack_require__(1844);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/codegenv2/message.js
var message = __webpack_require__(15033);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/codegenv2/service.js
var service = __webpack_require__(95765);
;// ./src/gen/session/v1/diagnose_pb.ts
// @generated by protoc-gen-es v2.11.0 with parameter "target=ts,ts_nocheck=false,keep_empty_files=true"
// @generated from file session/v1/diagnose.proto (package session.v1, syntax proto3)
/* eslint-disable */ 

/**
 * Describes the file session/v1/diagnose.proto.
 */ const file_session_v1_diagnose = /*@__PURE__*/ (0,file/* fileDesc */.w)("ChlzZXNzaW9uL3YxL2RpYWdub3NlLnByb3RvEgpzZXNzaW9uLnYxIjIKH0Fzc2VtYmxlRGlhZ25vc3RpY0J1bmRsZVJlcXVlc3QSDwoHaXRlbV9pZBgBIAEoCSJwCiBBc3NlbWJsZURpYWdub3N0aWNCdW5kbGVSZXNwb25zZRIPCgdpdGVtX2lkGAEgASgJEg4KBnByb21wdBgCIAEoCRIRCgljb21wYWN0ZWQYAyABKAgSGAoQZXN0aW1hdGVkX3Rva2VucxgEIAEoBSJ8ChdEaXNwYXRjaERpYWdub3NlUmVxdWVzdBIPCgdpdGVtX2lkGAEgASgJEiEKGW51ZGdlX3RhcmdldF9zZXNzaW9uX3V1aWQYAiABKAkSLQoMc3R1Y2tfcmVhc29uGAMgASgOMhcuc2Vzc2lvbi52MS5TdHVja1JlYXNvbiJzChhEaXNwYXRjaERpYWdub3NlUmVzcG9uc2USHwoXZGlhZ25vc3RpY19zZXNzaW9uX3V1aWQYASABKAkSFQoNbnVkZ2VfYWxsb3dlZBgCIAEoCBIfChdudWRnZV9kaXNhbGxvd2VkX3JlYXNvbhgDIAEoCTLtAQoRRGlhZ25vc3RpY1NlcnZpY2USdwoYQXNzZW1ibGVEaWFnbm9zdGljQnVuZGxlEisuc2Vzc2lvbi52MS5Bc3NlbWJsZURpYWdub3N0aWNCdW5kbGVSZXF1ZXN0Giwuc2Vzc2lvbi52MS5Bc3NlbWJsZURpYWdub3N0aWNCdW5kbGVSZXNwb25zZSIAEl8KEERpc3BhdGNoRGlhZ25vc2USIy5zZXNzaW9uLnYxLkRpc3BhdGNoRGlhZ25vc2VSZXF1ZXN0GiQuc2Vzc2lvbi52MS5EaXNwYXRjaERpYWdub3NlUmVzcG9uc2UiAEKtAQoOY29tLnNlc3Npb24udjFCDURpYWdub3NlUHJvdG9QAVpDZ2l0aHViLmNvbS90c3RhcGxlci9zdGFwbGVyLXNxdWFkL2dlbi9wcm90by9nby9zZXNzaW9uL3YxO3Nlc3Npb252MaICA1NYWKoCClNlc3Npb24uVjHKAgpTZXNzaW9uXFYx4gIWU2Vzc2lvblxWMVxHUEJNZXRhZGF0YeoCC1Nlc3Npb246OlYxYgZwcm90bzM", [
    backlog_pb/* file_session_v1_backlog */.aa7
]);
/**
 * Describes the message session.v1.AssembleDiagnosticBundleRequest.
 * Use `create(AssembleDiagnosticBundleRequestSchema)` to create a new message.
 */ const AssembleDiagnosticBundleRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_diagnose, 0)));
/**
 * Describes the message session.v1.AssembleDiagnosticBundleResponse.
 * Use `create(AssembleDiagnosticBundleResponseSchema)` to create a new message.
 */ const AssembleDiagnosticBundleResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_diagnose, 1)));
/**
 * Describes the message session.v1.DispatchDiagnoseRequest.
 * Use `create(DispatchDiagnoseRequestSchema)` to create a new message.
 */ const DispatchDiagnoseRequestSchema = /*@__PURE__*/ (0,message/* messageDesc */.Z)(file_session_v1_diagnose, 2);
/**
 * Describes the message session.v1.DispatchDiagnoseResponse.
 * Use `create(DispatchDiagnoseResponseSchema)` to create a new message.
 */ const DispatchDiagnoseResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_diagnose, 3)));
/**
 * DiagnosticService backs the "Diagnose & Nudge" backlog action (backlog
 * item 68964304): dispatch a context-bounded agent to investigate a stuck
 * (or any) backlog item, file a bug on a genuine defect, or safely nudge an
 * idle linked session. Read (AssembleDiagnosticBundle) and write
 * (DispatchDiagnose) are split, mirroring GuidanceRequestService's
 * registration shape (proto/session/v1/guidance_request.proto).
 *
 * @generated from service session.v1.DiagnosticService
 */ const DiagnosticService = /*@__PURE__*/ (0,service/* serviceDesc */.Z)(file_session_v1_diagnose, 0);

;// ./src/lib/hooks/useDiagnoseAction.ts
/* __next_internal_client_entry_do_not_use__ useDiagnoseAction auto */ 





/**
 * Dispatches the "Diagnose & Nudge" backlog action (backlog item 68964304).
 * A separate hook from useStuckBacklogItems (rather than adding another
 * method there) since this dispatch is meaningful from BacklogItemDetail.tsx
 * too, on items that aren't flagged stuck at all — bundling it into the
 * stuck-items-only hook would force every consumer through that hook's
 * polling/context machinery for a single one-shot action call.
 */ function useDiagnoseAction() {
    const transport = (0,react.useMemo)(()=>(0,api_transport/* getConnectTransport */.qW)(), []);
    const client = (0,react.useMemo)(()=>(0,promise_client/* createClient */.UU)(DiagnosticService, transport), [
        transport
    ]);
    const dispatchDiagnose = (0,react.useCallback)(async function(itemId) {
        let reason = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : backlog_pb/* StuckReason */.Pwx.UNSPECIFIED;
        const req = (0,create/* create */.v)(DispatchDiagnoseRequestSchema, {
            itemId,
            stuckReason: reason
        });
        return client.dispatchDiagnose(req);
    }, [
        client
    ]);
    return {
        dispatchDiagnose
    };
}

// EXTERNAL MODULE: ./src/components/backlog-stuck/stuckReason.ts + 1 modules
var stuckReason = __webpack_require__(91216);
// EXTERNAL MODULE: ./src/lib/analytics/index.ts
var analytics = __webpack_require__(10671);
// EXTERNAL MODULE: ./src/lib/backlog/formatReworkCapOverride.ts
var formatReworkCapOverride = __webpack_require__(13333);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog-stuck%2FStuckItemDetail.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA81U226jMBB9z1dYfdo8OAJya1ztl6xWlcETsGpsNDZNYNV_X9lkFTeEJlX7sC8gzeWcmTMzXjwvj6JbPybkz4yQnBcvJZpWC0ZeOf6g9DndQZ9hv50_eb9BAchI2hyJNUqKiyiIoqgzDSPaaIhsuXHO1FTB3lHkQrb2kmgZM_2LR1lWNxIaLoTU5cibBq-QtlG8Y2Sv4OgN_k-FRCicNJqRwqi21t5T8uYSJOsDyN5oR63sYUSSz59mb7PFScs0aDminAbmSpaaSge1ZSTnFpQcVAtVHtAn-m9MkgWSwiiDI9RztQfwyjGySRJvq6WmBylcxcguac5C2AqlfmEkiRmW0wzpu35X04HJuJR18o5l_QHLLmQ7ODoqoDDIh1kNOxVBsMq8AgagUXCrBeCg5zljc5d41nUKGJGOK1nE6dsvDbgA7QBvjPcxUJxmtc2GWfnxVScZV6vBNrH3WTe_XsrpsCZuafOZM598LFIeAm5txB2ntAsyfKbv_rLvZfJtfdc3-q6n217f0fb1my1atB6vMXLYm0idXxwlp0JanisQPx8ctvDwO2hmGl5I1zGSLDYxjDaOcqXMAUQMxf8roccL5pBr23AE7b60Wh_qyb9Pz_y-F-Zamf543v4CreTePZgHAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_stuck_2FStuckItemDetail_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81U226jMBB9z1dYfdo8OAJya1ztl6xWlcETsGpsNDZNYNV_X9lkFTeEJlX7sC8gzeWcmTMzXjwvj6JbPybkz4yQnBcvJZpWC0ZeOf6g9DndQZ9hv50_eb9BAchI2hyJNUqKiyiIoqgzDSPaaIhsuXHO1FTB3lHkQrb2kmgZM_2LR1lWNxIaLoTU5cibBq_QtlG8Y2Sv4OgN_k_FRCicNJqRwqi21t5T8uYSJOsDyN5oR63sYUSSz59mb7PFScs0aDminAbmSpaaSge1ZSTnFpQcVAtVHtAn_m9MkgWSwiiDI9RztQfwyjGySRJvq6WmBylcxcguac5C2AqlfmEkiRmW0wzpu35X04HJuJR18o5l_QHLLmQ7ODoqoDDIh1kNOxVBsMq8AgagUXCrBeCg5zljc5d41nUKGJGOK1nE6dsvDbgA7QBvjPcxUJxmtc2GWfnxVScZV6vBNrH3WTe_XsrpsCZuafOZM598LFIeAm5txB2ntAsyfKbv_rLvZfJtfdc3_q6n217f0fb1my1atB6vMXLYm0idXxwlp0JanisQPx8ctvDwO2hmGl5I1zGSLDYxjDaOcqXMAUQMxf8roccL5pBr23AE7b60Wh_qyb9Pz_y_F_Zamf543v4CreTePZgHAAA_22_7D = __webpack_require__(91484);
;// ./src/components/backlog-stuck/StuckItemDetail.css.ts


var actionCopy = '_3xdy584';
var StuckItemDetail_css_detail = '_3xdy580';
var StuckItemDetail_css_label = '_3xdy582';
var overrideButton = '_3xdy589';
var overrideForm = '_3xdy587';
var overrideInput = '_3xdy588';
var overrideStatus = '_3xdy58b';
var overrideUnlimitedButton = '_3xdy58a';
var prLink = '_3xdy585';
var row = '_3xdy581';
var unknownNote = '_3xdy586';
var value = '_3xdy583';
;// ./src/components/backlog-stuck/StuckItemDetail.tsx
// +feature: backlog-diagnose-nudge
/* __next_internal_client_entry_do_not_use__ StuckItemDetail auto */ 








/** Read-only "Repo auto-merge: on/off/unknown" line (Story 4.1.4). `allowAutoMerge` is
 * unset ("not fetched / unknown", never treated as "disabled") whenever the
 * server's best-effort fetch hasn't populated it. */ function autoMergeLine(allowAutoMerge) {
    if (allowAutoMerge === undefined) return "Repo auto-merge: unknown";
    return "Repo auto-merge: ".concat(allowAutoMerge ? "on" : "off", " (allow_auto_merge: ").concat(allowAutoMerge, ")");
}
/**
 * Read-only "current rework cap override" line. `currentReworkCapOverride`
 * is `undefined` both before the parent's fetch-on-expand has resolved and
 * when it resolves to a genuinely-unset override — those two states are
 * otherwise visually indistinguishable, so `loaded` (StuckItemsSection's
 * `reworkCapOverrides.has(item.itemId)`) must gate the "No override set"
 * copy specifically, rather than being inferred from `value`.
 */ function StuckItemDetail_formatReworkCapOverride(value, loaded) {
    if (!loaded) return "Checking current override…";
    const resolved = (0,formatReworkCapOverride/* resolveReworkCapOverride */.H)(value);
    if (resolved.kind === "unset") return "No override set (using global default)";
    if (resolved.kind === "unlimited") return "Unlimited";
    return "".concat(resolved.rounds, " rounds");
}
/**
 * Expanded accordion detail panel for a StuckItem. Renders inline beneath the
 * card (no portal/modal), mirroring UnfinishedItemDetail.tsx.
 */ function StuckItemDetail(param) {
    let { item, onReworkCapOverride, currentReworkCapOverride, reworkCapOverrideLoaded = false, onApprovePlan, onDiagnose } = param;
    var _item_context;
    const { track } = (0,analytics/* useAnalytics */.st)();
    const unknown = (0,stuckReason/* isPrStatusUnknown */.lc)(item);
    const isPrReady = item.reason === backlog_pb/* StuckReason */.Pwx.PR_READY_UNMERGED;
    const isReworkCap = item.reason === backlog_pb/* StuckReason */.Pwx.REWORK_CAP;
    const isAutonomousStuck = item.reason === backlog_pb/* StuckReason */.Pwx.AUTONOMOUS_STUCK;
    const isPlanNotApprovedReason = item.reason === backlog_pb/* StuckReason */.Pwx.PLAN_NOT_APPROVED;
    // hasPlan gates the actionable "Approve Plan" affordance on a real plan
    // existing — `reason` alone lags actual PlanArtifactsPath state until the
    // next ReconcileStuck tick (see research/pitfalls.md #1), so without this
    // check the button could be shown with nothing behind it.
    const hasPlan = item.planArtifactsPath !== "";
    const isPlanNotApproved = isPlanNotApprovedReason && hasPlan;
    const why = ((_item_context = item.context) === null || _item_context === void 0 ? void 0 : _item_context.trim()) ? item.context : "No additional context recorded";
    // Pre-fill with this item's existing explicit cap (>0) so the input
    // reflects reality instead of a generic default; 0 (unlimited) and
    // undefined (no override) have no positive number to show in this
    // min=1 numeric field, so both fall back to the "3" starting suggestion.
    const [moreRounds, setMoreRounds] = (0,react.useState)(currentReworkCapOverride !== undefined && currentReworkCapOverride > 0 ? String(currentReworkCapOverride) : "3");
    // currentReworkCapOverride is fetched by the parent (StuckItemsSection)
    // *after* this component has already mounted with it `undefined` (the
    // panel expands, then the getBacklogItem fetch resolves later) — so the
    // useState initializer above almost always runs against the unresolved
    // value and the input is stuck showing "3". This ref-guarded one-time sync
    // re-applies the real value once it arrives.
    //
    // hasSyncedOverride alone only tracks "has the sync fired once" — it does
    // NOT track "has the user already edited the field since mount". Without
    // userEditedRounds, a user who types into the field before the parent's
    // fetch resolves would have their input silently clobbered the moment the
    // fetch lands (the effect's guard was still false, so it fires and
    // overwrites whatever they typed). userEditedRounds, set in the input's
    // onChange handler, makes the sync a no-op once the user has started
    // typing, regardless of fetch timing.
    const hasSyncedOverride = (0,react.useRef)(false);
    const userEditedRounds = (0,react.useRef)(false);
    (0,react.useEffect)(()=>{
        if (!hasSyncedOverride.current && !userEditedRounds.current && currentReworkCapOverride !== undefined) {
            hasSyncedOverride.current = true;
            if (currentReworkCapOverride > 0) {
                setMoreRounds(String(currentReworkCapOverride));
            }
        }
    }, [
        currentReworkCapOverride
    ]);
    const [overrideState, setOverrideState] = (0,react.useState)("idle");
    const [approveState, setApproveState] = (0,react.useState)("idle");
    const [approveError, setApproveError] = (0,react.useState)(null);
    const [diagnoseState, setDiagnoseState] = (0,react.useState)("idle");
    const [diagnoseError, setDiagnoseError] = (0,react.useState)(null);
    // Each computed once and reused for both a button's aria-disabled and its
    // click guard below, so the two conditions can't drift apart.
    const isOverridePending = overrideState === "pending";
    const isSetCapDisabled = isOverridePending || !Number(moreRounds) || Number(moreRounds) <= 0;
    const isApprovePending = approveState === "pending";
    const isDiagnosePending = diagnoseState === "pending" || diagnoseState === "dispatched";
    // The dispatched diagnostic session's UUID (onDiagnose's resolved value),
    // used to render a direct "View diagnostic session" link below — see
    // onDiagnose's doc comment for why this link matters (the session is
    // hidden from the main session list).
    const [dispatchedSessionUuid, setDispatchedSessionUuid] = (0,react.useState)(null);
    async function submitOverride(override) {
        // These buttons use aria-disabled instead of disabled (so they stay
        // focusable while busy), which doesn't block clicks on its own — guard
        // here against a rapid double click/Enter re-dispatching before the
        // first request settles.
        if (!onReworkCapOverride || isOverridePending) return;
        setOverrideState("pending");
        const ok = await onReworkCapOverride(item.itemId, override);
        setOverrideState(ok ? "idle" : "error");
    }
    async function submitApprovePlan() {
        if (!onApprovePlan || isApprovePending) return;
        setApproveState("pending");
        setApproveError(null);
        try {
            await onApprovePlan(item.itemId);
            setApproveState("idle");
        } catch (err) {
            setApproveState("error");
            setApproveError(err instanceof Error ? err.message : "Failed to approve — try again.");
        }
    }
    async function submitDiagnose() {
        if (!onDiagnose || isDiagnosePending) return;
        setDiagnoseState("pending");
        setDiagnoseError(null);
        try {
            const uuid = await onDiagnose(item.itemId, item.reason);
            setDispatchedSessionUuid(uuid);
            setDiagnoseState("dispatched");
        } catch (err) {
            setDiagnoseState("error");
            setDiagnoseError(err instanceof Error ? err.message : "Failed to dispatch — try again.");
        }
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: StuckItemDetail_css_detail,
        "data-testid": "stuck-item-detail",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: row,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: StuckItemDetail_css_label,
                        children: "Why:"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: value,
                        "data-testid": "stuck-item-why",
                        children: why
                    })
                ]
            }),
            isPrReady && !unknown && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: actionCopy,
                "data-testid": "stuck-item-action-copy",
                children: "This PR is ready — merge it on GitHub when you're ready."
            }),
            unknown && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: actionCopy,
                "data-testid": "stuck-item-no-action-copy",
                children: "Couldn't check this PR's status — no action available."
            }),
            isReworkCap && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: actionCopy,
                        "data-testid": "stuck-item-rework-cap-copy",
                        children: 'Hit the auto-rework cap after repeated failed reviews. Raise this item\'s own cap below to let it keep retrying automatically, or click "Reopen for Revision" on the item to try one more round manually.'
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: row,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: StuckItemDetail_css_label,
                                children: "Current override:"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: value,
                                "data-testid": "stuck-item-rework-cap-current",
                                children: StuckItemDetail_formatReworkCapOverride(currentReworkCapOverride, reworkCapOverrideLoaded)
                            })
                        ]
                    }),
                    onReworkCapOverride && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: overrideForm,
                        "data-testid": "stuck-item-rework-cap-override-form",
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                type: "number",
                                min: 1,
                                value: moreRounds,
                                onChange: (e)=>{
                                    userEditedRounds.current = true;
                                    setMoreRounds(e.target.value);
                                },
                                className: overrideInput,
                                "aria-label": "This item's new rework cap",
                                "data-testid": "stuck-item-rework-cap-rounds-input",
                                disabled: isOverridePending
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                type: "button",
                                className: overrideButton,
                                "aria-disabled": isSetCapDisabled,
                                onClick: ()=>{
                                    if (isSetCapDisabled) return;
                                    track({
                                        name: "stuck_item_rework_cap_override",
                                        category: "user_action",
                                        component: "StuckItemDetail",
                                        labels: {
                                            unlimited: "false"
                                        }
                                    });
                                    void submitOverride(Number(moreRounds));
                                },
                                "data-testid": "stuck-item-rework-cap-allow-rounds",
                                children: [
                                    "Set this item's cap to ",
                                    moreRounds || 0,
                                    " & resume"
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: overrideUnlimitedButton,
                                "aria-disabled": isOverridePending,
                                onClick: ()=>{
                                    if (isOverridePending) return;
                                    track({
                                        name: "stuck_item_rework_cap_override",
                                        category: "user_action",
                                        component: "StuckItemDetail",
                                        labels: {
                                            unlimited: "true"
                                        }
                                    });
                                    void submitOverride(0);
                                },
                                "data-testid": "stuck-item-rework-cap-unlimited",
                                children: "Remove cap for this item & resume"
                            }),
                            overrideState === "error" && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: overrideStatus,
                                role: "alert",
                                children: "Failed to apply — try again."
                            })
                        ]
                    })
                ]
            }),
            isAutonomousStuck && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: actionCopy,
                "data-testid": "stuck-item-autonomous-stuck-copy",
                children: 'Autonomous mode stopped without a completion signal. Open the session to see what it accomplished, then either give it a manual instruction or use "Reopen for Revision" / re-trigger triage to let it try again.'
            }),
            isPlanNotApprovedReason && !hasPlan && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: actionCopy,
                "data-testid": "stuck-item-no-action-copy",
                children: "This item is flagged as waiting on plan approval, but no plan has been generated yet — run triage first. This will clear on the next check once a plan exists."
            }),
            isPlanNotApproved && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: actionCopy,
                        "data-testid": "stuck-item-plan-not-approved-copy",
                        children: "This item is queued but can't be dequeued until its plan is approved (or skip_planning is set). Approve the plan below to unblock it."
                    }),
                    onApprovePlan && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: overrideForm,
                        "data-testid": "stuck-item-approve-plan-form",
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: overrideButton,
                                "aria-disabled": isApprovePending,
                                onClick: ()=>{
                                    if (isApprovePending) return;
                                    track({
                                        name: "stuck_item_approve_plan",
                                        category: "user_action",
                                        component: "StuckItemDetail"
                                    });
                                    void submitApprovePlan();
                                },
                                "data-testid": "stuck-item-approve-plan",
                                children: approveState === "pending" ? "Approving…" : "Approve Plan"
                            }),
                            approveState === "error" && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: overrideStatus,
                                role: "alert",
                                "data-testid": "stuck-item-approve-plan-error",
                                children: approveError
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: row,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: StuckItemDetail_css_label,
                        children: "Since:"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: value,
                        children: [
                            (0,stuckReason/* formatSinceUTC */.cw)(item.firstDetectedAt),
                            " (first detected)"
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: row,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: StuckItemDetail_css_label,
                        children: "Last check:"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: value,
                        "data-testid": "stuck-item-last-check",
                        children: (0,stuckReason/* formatAgo */.n4)(item.lastCheckedAt)
                    })
                ]
            }),
            isPrReady && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: row,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    className: "".concat(item.allowAutoMerge === undefined ? unknownNote : value),
                    "data-testid": "stuck-item-auto-merge",
                    children: autoMergeLine(item.allowAutoMerge)
                })
            }),
            item.prNumber > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: row,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: StuckItemDetail_css_label,
                        children: "PR:"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("a", {
                        className: prLink,
                        href: item.prUrl,
                        target: "_blank",
                        rel: "noreferrer",
                        "data-testid": "stuck-item-pr-link",
                        "aria-label": "View PR #".concat(item.prNumber, " on GitHub"),
                        children: [
                            "\uD83D\uDD17 View PR #",
                            item.prNumber,
                            " on GitHub"
                        ]
                    })
                ]
            }),
            onDiagnose && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: overrideForm,
                "data-testid": "stuck-item-diagnose-form",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: overrideButton,
                        "aria-disabled": isDiagnosePending,
                        onClick: ()=>{
                            if (isDiagnosePending) return;
                            track({
                                name: "stuck_item_diagnose",
                                category: "user_action",
                                component: "StuckItemDetail",
                                labels: {
                                    reason: String(item.reason)
                                }
                            });
                            void submitDiagnose();
                        },
                        "data-testid": "stuck-item-diagnose",
                        children: diagnoseState === "pending" ? "Dispatching…" : diagnoseState === "dispatched" ? "Diagnostic session dispatched" : "Diagnose & Nudge"
                    }),
                    diagnoseState === "error" && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: overrideStatus,
                        role: "alert",
                        "data-testid": "stuck-item-diagnose-error",
                        children: diagnoseError
                    }),
                    diagnoseState === "dispatched" && dispatchedSessionUuid && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: overrideStatus,
                        "data-testid": "stuck-item-diagnose-dispatched-detail",
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(nav/* Link */.N_, {
                                className: prLink,
                                href: routes/* routes */.J.sessionDetail(dispatchedSessionUuid),
                                "data-testid": "stuck-item-diagnose-session-link",
                                "aria-label": "Open the dispatched diagnostic session's transcript",
                                children: "View diagnostic session →"
                            }),
                            " ",
                            'for its live log. Its conclusion (bug filed, note, or nudge) posts to this item\'s Activity Log — see "Open item detail" below.'
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: row,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(nav/* Link */.N_, {
                    className: prLink,
                    href: "".concat(routes/* routes */.J.backlog, "?item=").concat(encodeURIComponent(item.itemId)),
                    "data-testid": "stuck-item-open-detail-link",
                    "aria-label": "Open this item's full detail page",
                    children: "Open item detail →"
                })
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/components/backlog/DuplicatePendingBadge.tsx + 1 modules
var DuplicatePendingBadge = __webpack_require__(75938);
// EXTERNAL MODULE: ./src/components/backlog/ClaimOverrideForm.tsx + 1 modules
var ClaimOverrideForm = __webpack_require__(55468);
;// ./src/components/backlog-stuck/useStuckItemCardActions.ts



const SNOOZE_DURATION_MS = {
    "1h": 60 * 60 * 1000,
    "1d": 24 * 60 * 60 * 1000,
    "3d": 3 * 24 * 60 * 60 * 1000
};
/**
 * Snooze popover state (open/duration/confirm) for one StuckItem card.
 * Extracted from StuckItem.tsx — same behavior, isolated as its own concern.
 */ function useSnoozeControl(itemId, reason, onSnooze, containerRef) {
    const [snoozeOpen, setSnoozeOpen] = (0,react.useState)(false);
    const [snoozeDuration, setSnoozeDuration] = (0,react.useState)("1d");
    const [snoozeState, setSnoozeState] = (0,react.useState)("idle");
    // Surface 10: clicking outside the open picker closes it with no request sent.
    (0,react.useEffect)(()=>{
        if (!snoozeOpen) return;
        const handleClickOutside = (e)=>{
            if (containerRef.current && !containerRef.current.contains(e.target)) {
                setSnoozeOpen(false);
                setSnoozeState("idle");
            }
        };
        document.addEventListener("mousedown", handleClickOutside);
        return ()=>document.removeEventListener("mousedown", handleClickOutside);
    }, [
        snoozeOpen,
        containerRef
    ]);
    const handleSnoozeTriggerClick = (0,react.useCallback)((e)=>{
        e.stopPropagation();
        setSnoozeOpen((open)=>!open);
        setSnoozeState("idle");
    }, []);
    const handleSnoozeCancel = (0,react.useCallback)((e)=>{
        e.stopPropagation();
        setSnoozeOpen(false);
        setSnoozeState("idle");
    }, []);
    const handleSnoozeConfirm = (0,react.useCallback)(async (e)=>{
        e.stopPropagation();
        if (!onSnooze) return;
        setSnoozeState("pending");
        const until = new Date(Date.now() + SNOOZE_DURATION_MS[snoozeDuration]);
        const applied = await onSnooze(itemId, reason, until);
        if (applied) {
            // Success: the hook refetches and this card is removed from the
            // parent's list on the next render — nothing further to do here.
            setSnoozeOpen(false);
            setSnoozeState("idle");
        } else {
            setSnoozeState("error");
        }
    }, [
        onSnooze,
        itemId,
        reason,
        snoozeDuration
    ]);
    const handleSnoozePickerKeyDown = (0,react.useCallback)((e)=>{
        e.stopPropagation();
        if (e.key === "Escape") {
            setSnoozeOpen(false);
            setSnoozeState("idle");
        }
    }, []);
    return {
        snoozeOpen,
        snoozeDuration,
        setSnoozeDuration,
        snoozeState,
        handleSnoozeTriggerClick,
        handleSnoozeCancel,
        handleSnoozeConfirm,
        handleSnoozePickerKeyDown
    };
}
/**
 * "Retry now" (TriggerRemediationNow) state for one StuckItem card.
 * Extracted from StuckItem.tsx — same behavior, isolated as its own concern.
 */ function useRetryRemediation(itemId, reason, remediationAttempts, onTriggerRemediationNow) {
    const [retryState, setRetryState] = (0,react.useState)("idle");
    const [retryErrorMessage, setRetryErrorMessage] = (0,react.useState)(null);
    const isParked = (0,stuckReason/* isRemediationParked */.xj)({
        remediationAttempts
    });
    const handleRetryNow = (0,react.useCallback)(async (e)=>{
        e.stopPropagation();
        if (!onTriggerRemediationNow) return;
        setRetryState("pending");
        setRetryErrorMessage(null);
        try {
            await onTriggerRemediationNow(itemId, reason);
            // Success: the hook refetches; this item's remediation_attempts will
            // reflect the new attempt on the next render. No local "success"
            // state needed beyond clearing pending.
            setRetryState("idle");
        } catch (err) {
            setRetryState("error");
            setRetryErrorMessage(err instanceof Error ? err.message : "Retry failed");
        }
    }, [
        onTriggerRemediationNow,
        itemId,
        reason
    ]);
    return {
        retryState,
        retryErrorMessage,
        handleRetryNow,
        isParked
    };
}
/**
 * "Override claim" (OverrideClaimBlock) form state for one StuckItem card.
 * Extracted from StuckItem.tsx — same behavior, isolated as its own concern.
 */ function useClaimOverrideControl(itemId, reason, onOverrideClaimBlock) {
    const [overrideOpen, setOverrideOpen] = (0,react.useState)(false);
    const [overrideBusy, setOverrideBusy] = (0,react.useState)(false);
    const [overrideError, setOverrideError] = (0,react.useState)(null);
    const canOverrideClaim = reason === backlog_pb/* StuckReason */.Pwx.BLOCKED_BY_CLAIM && onOverrideClaimBlock !== undefined;
    const handleOverrideConfirm = (0,react.useCallback)(async (overrideReason)=>{
        if (!onOverrideClaimBlock) return;
        setOverrideBusy(true);
        setOverrideError(null);
        try {
            await onOverrideClaimBlock(itemId, overrideReason);
            setOverrideOpen(false);
        } catch (err) {
            setOverrideError(err instanceof Error ? err.message : "Override failed");
        } finally{
            setOverrideBusy(false);
        }
    }, [
        onOverrideClaimBlock,
        itemId
    ]);
    return {
        canOverrideClaim,
        overrideOpen,
        setOverrideOpen,
        overrideBusy,
        overrideError,
        handleOverrideConfirm
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog-stuck%2FStuckItem.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-1X227iMBB971f4sUg1SlKgNLzsn1RO7JApjp21HW6r_vsqDhDnCl2q9mVVCame8Yznds5k-hYc5u_Fu4f-PCAUkXizVrIQNERboh4xfvNf2TFQx5fJqpRLRZkKkZ_vkZYcaEuLOVpYEQqFbht6rizlhFIQ647Um7RPfKsfF0pLFaJcgjBMlUdGEaHBgBSh83DkTf25fjo_IpZcquqsvCMLw0GwEAkp2Orh42F6jj9M5ZapK1lYuvFZ020NmDStJjIuNN6ChoizyvrIbX6yv8c6JVTuQuTZvyDf92jWfnzXciSNkRnmLDGXGnirjlzBOh1T6OYosF5kTmIwhxB504UrfbZSCjrn5BCihLN9aZNwWAsMhmU6RDE7F29N8p7al5LyIt6pUl7-ui5m1kUihcEajqxjIKpapS-zJ9vl1R0rIw_R3PMu_taqTLZvW2TLVMLLf1OglAnbamxvcC1gnEOuwXbULgXDsM5JbLuqejJCGQi8A2rSEC29fO9GMa-jSEgG_NB-a1DUj-2NkwzHGdQ51KkCsbGlrZ0v7q1ScLQeMqLWILCRXYXD3a-_nprhHnm5mt3tsP8m1lCWkIIb1_rSWh8Gr9UofIx4vlbyqA9ZS2jooGs_3n4KuW1V29P_2sFGC8A5UUyYryKHRZMcurh3ar87uquHSUpKwOkJFvzpvG-EXOBrs89JcqGeBhkF-gldWCjoYaZA9xFRM-0X334fvezApCBuuvLaw0iupkORwUAdK5I6qWGZJJoZW_Wmn9v5dASwb6da0hOKA8KzWb4_H50rfT4bbibaZY0Xr4Gn0f-p-ImpiD4_FWNXom-aiui7pyIKKWgScUbbu9vMrbqQBhPO5Y5R93Y8MFPnpgiWzfDoXdwb3NDDta-qSCNryPHKd8Y4Vcdf_L3T2bjsJFFQLK6mJZa8yMT40jWymCT9e93nvBwaGV5_0ap4x77eQaX6demVb4ERxLOdVluC_jjfC20gOeBYCsOEqQSYCToWbW31_QeIYdYhhmfv34lh4MPbCfFmNKvvbK5oZ8Pv-X3TSGbfmbbh3Gxuyo1o5mYQrBcjYP0rYxQIerT-qrV9Yu8318E2jn84GtGAxsdfl74hlR4SAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_stuck_2FStuckItem_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_1X227iMBB971f4sUg1SlKgNLzsn1RO7JApjp21HW6r_vsqDhDnCl2q9mVVCame8Yznds5k_hYc5u_Fu4f_PCAUkXizVrIQNERboh4xfvNf2TFQx5fJqpRLRZkKkZ_vkZYcaEuLOVpYEQqFbht6rizlhFIQ647Um7RPfKsfF0pLFaJcgjBMlUdGEaHBgBSh83DkTf25fjo_IpZcquqsvCMLw0GwEAkp2Orh42F6jj9M5ZapK1lYuvFZ020NmDStJjIuNN6ChoizyvrIbX6yv8c6JVTuQuTZvyDf92jWfnzXciSNkRnmLDGXGnirjlzBOh1T6OYosF5kTmIwhxB504UrfbZSCjrn5BCihLN9aZNwWAsMhmU6RDE7F29N8p7al5LyIt6pUl7_ui5m1kUihcEajqxjIKpapS_zJ9vl1R0rIw_R3PMu_taqTLZvW2TLVMLLf1OglAnbamxvcC1gnEOuwXbULgXDsM5JbLuqejJCGQi8A2rSEC29fO9GMa_jSEgG_NB_a1DUj_2NkwzHGdQ51KkCsbGlrZ0v7q1ScLQeMqLWILCRXYXD3a_nprhHnm5mt3tsP8m1lCWkIIb1_rSWh8Gr9UofIx4vlbyqA9ZS2jooGs_3n4KuW1V29P_2sFGC8A5UUyYryKHRZMcurh3ar87uquHSUpKwOkJFvzpvG_EXOBrs89JcqGeBhkF_gldWCjoYaZA9xFRM_0X334fvezApCBuuvLaw0iupkORwUAdK5I6qWGZJJoZW_Wmn9v5dASwb6da0hOKA8KzWb4_H50rfT4bbibaZY0Xr4Gn0f_p_ImpiD4_FWNXom_aiui7pyIKKWgScUbbu9vMrbqQBhPO5Y5R93Y8MFPnpgiWzfDoXdwb3NDDta_qSCNryPHKd8Y4Vcdf_L3T2bjsJFFQLK6mJZa8yMT40jWymCT9e93nvBwaGV5_0ap4x77eQaX6demVb4ERxLOdVluC_jjfC20gOeBYCsOEqQSYCToWbW31_QeIYdYhhmfv34lh4MPbCfFmNKvvbK5oZ8Pv_X3TSGbfmbbh3Gxuyo1o5mYQrBcjYP0rYxQIerT_qrV9Yu8318E2jn84GtGAxsdfl74hlR4SAAA_22_7D = __webpack_require__(93282);
;// ./src/components/backlog-stuck/StuckItem.css.ts


var StuckItem_css_card = '_2y5juj0';
var StuckItem_css_cardExpanded = '_2y5juj1';
var cardResolved = '_2y5juj2';
var duration = '_2y5juj5';
var StuckItem_css_header = '_2y5juj3';
var metaRow = '_2y5juj6';
var otherReasonsBadge = '_2y5juj7';
var resolvedBanner = '_2y5juj8';
var retryBtn = '_2y5jujb';
var retryBtnAlwaysOn = '_2y5jujc';
var retryError = '_2y5jujd';
var snoozeActions = '_2y5juji';
var snoozeBtn = '_2y5juj9';
var snoozeBtnAlwaysOn = '_2y5juja';
var snoozeCancelBtn = '_2y5jujj';
var snoozeConfirmBtn = '_2y5jujk';
var snoozeErrorRow = '_2y5jujh';
var snoozeOptionLabel = '_2y5jujg';
var snoozeOptions = '_2y5jujf';
var snoozePicker = '_2y5juje';
var StuckItem_css_title = '_2y5juj4';
;// ./src/components/backlog-stuck/StuckItem.tsx
/* __next_internal_client_entry_do_not_use__ StuckItem auto */ 







/** Extracts "owner/repo" from a GitHub PR URL, for the glance-level identity line. */ function repoFromPrUrl(prUrl) {
    const match = prUrl.match(/github\.com\/([^/]+\/[^/]+)\/pull\//);
    return match ? match[1] : null;
}
const SNOOZE_DURATION_LABELS = {
    "1h": "1 hour",
    "1d": "1 day",
    "3d": "3 days"
};
/**
 * Detects a `(hover: none), (pointer: coarse)` pointer — the media query the
 * rest of the app already uses to gate hover-only chrome (design/ux.md
 * Surface 7). Returns false during SSR / before the effect runs, matching a
 * hover-capable desktop default.
 */ function useHoverUnavailable() {
    const [hoverUnavailable, setHoverUnavailable] = (0,react.useState)(false);
    (0,react.useEffect)(()=>{
        if ( false || typeof window.matchMedia !== "function") return;
        const mql = window.matchMedia("(hover: none), (pointer: coarse)");
        setHoverUnavailable(mql.matches);
        const listener = (e)=>setHoverUnavailable(e.matches);
        if (mql.addEventListener) {
            mql.addEventListener("change", listener);
            return ()=>mql.removeEventListener("change", listener);
        }
        // Safari <14 fallback.
        mql.addListener(listener);
        return ()=>mql.removeListener(listener);
    }, []);
    return hoverUnavailable;
}
/**
 * Card representing a single stuck backlog item (one BacklogStuckState row).
 * Verbatim copy of UnfinishedItem.tsx's expand/keyboard/aria-expanded shape.
 */ function StuckItem(param) {
    let { item, isExpanded, onToggleExpand, otherReasonsCount = 0, otherReasonLabels = [], justResolved = false, resolvedMessage, resolvedTrailingMessage, onSnooze, onReworkCapOverride, currentReworkCapOverride, reworkCapOverrideLoaded = false, onTriggerRemediationNow, onApprovePlan, onOverrideClaimBlock, onDiagnose, focusItemId } = param;
    const cardRef = (0,react.useRef)(null);
    const containerRef = (0,react.useRef)(null);
    const wasExpandedRef = (0,react.useRef)(isExpanded);
    const hoverUnavailable = useHoverUnavailable();
    const { snoozeOpen, snoozeDuration, setSnoozeDuration, snoozeState, handleSnoozeTriggerClick, handleSnoozeCancel, handleSnoozeConfirm, handleSnoozePickerKeyDown } = useSnoozeControl(item.itemId, item.reason, onSnooze, containerRef);
    const { retryState, retryErrorMessage, handleRetryNow, isParked } = useRetryRemediation(item.itemId, item.reason, item.remediationAttempts, onTriggerRemediationNow);
    const { canOverrideClaim, overrideOpen, setOverrideOpen, overrideBusy, overrideError, handleOverrideConfirm } = useClaimOverrideControl(item.itemId, item.reason, onOverrideClaimBlock);
    // AC 29: when this card collapses (Escape, re-click, or a parent-driven
    // toggle), keyboard focus returns to the card's own toggle control — it
    // must never be dropped to <body>.
    (0,react.useEffect)(()=>{
        if (wasExpandedRef.current && !isExpanded) {
            var _cardRef_current;
            (_cardRef_current = cardRef.current) === null || _cardRef_current === void 0 ? void 0 : _cardRef_current.focus();
        }
        wasExpandedRef.current = isExpanded;
    }, [
        isExpanded
    ]);
    // /unfinished?item=<itemId> deep link (routes.unfinishedItem): scroll the
    // matching card into view once it's expanded and rendered.
    (0,react.useEffect)(()=>{
        if (focusItemId && item.itemId === focusItemId && isExpanded) {
            var _cardRef_current;
            (_cardRef_current = cardRef.current) === null || _cardRef_current === void 0 ? void 0 : _cardRef_current.scrollIntoView({
                block: "center"
            });
        }
    }, [
        focusItemId,
        item.itemId,
        isExpanded
    ]);
    const handleKeyDown = (0,react.useCallback)((e)=>{
        if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            onToggleExpand();
        }
        if (e.key === "Escape" && isExpanded) {
            onToggleExpand();
        }
    }, [
        isExpanded,
        onToggleExpand
    ]);
    const unknown = (0,stuckReason/* isPrStatusUnknown */.lc)(item);
    const chipLabel = unknown ? stuckReason/* PR_STATUS_UNKNOWN_LABEL */.C5 : (0,stuckReason/* getStuckReasonLabel */.Lv)(item.reason);
    const chipIcon = unknown ? stuckReason/* PR_STATUS_UNKNOWN_ICON */.n1 : (0,stuckReason/* getStuckReasonIcon */.qk)(item.reason);
    const chipClass = unknown ? stuckReason/* PR_STATUS_UNKNOWN_CLASS */.ie : (0,stuckReason/* getStuckReasonClass */.px)(item.reason);
    const repo = item.prNumber > 0 ? repoFromPrUrl(item.prUrl) : null;
    const identity = item.prNumber > 0 ? "PR #".concat(item.prNumber).concat(repo ? " \xb7 ".concat(repo) : "") : "item ".concat(item.itemId.slice(0, 8));
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        ref: containerRef,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                ref: cardRef,
                role: "button",
                tabIndex: 0,
                "aria-expanded": isExpanded,
                className: "".concat(StuckItem_css_card, " ").concat(isExpanded ? StuckItem_css_cardExpanded : "", " ").concat(justResolved ? cardResolved : ""),
                onClick: onToggleExpand,
                onKeyDown: handleKeyDown,
                "data-testid": "stuck-item",
                "data-reason": item.reason,
                "data-item-id": item.itemId,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: StuckItem_css_header,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: "".concat(chipClass),
                                "aria-label": chipLabel,
                                "data-testid": "stuck-item-chip",
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        "aria-hidden": "true",
                                        children: chipIcon
                                    }),
                                    chipLabel
                                ]
                            }),
                            item.duplicatePending && /*#__PURE__*/ (0,jsx_runtime.jsx)(DuplicatePendingBadge/* DuplicatePendingBadge */.w, {
                                duplicateRef: item.duplicateRef
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: StuckItem_css_title,
                                title: item.title,
                                children: item.title
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: duration,
                                "data-testid": "stuck-item-duration",
                                children: [
                                    "stuck ",
                                    (0,stuckReason/* formatStuckDuration */.CR)(item.firstDetectedAt)
                                ]
                            }),
                            onTriggerRemediationNow && // analytics-exempt
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: "".concat(retryBtn, " ").concat(hoverUnavailable ? retryBtnAlwaysOn : ""),
                                onClick: handleRetryNow,
                                disabled: isParked || retryState === "pending",
                                "aria-label": isParked ? "Retry now (disabled — remediation attempts exhausted; use Reset to try again)" : "Retry remediation now",
                                title: isParked ? "Automated retries have been exhausted for this item — use Reset to try again" : "Immediately retry the automated fix, skipping the wait",
                                "data-testid": "stuck-item-retry-now",
                                children: retryState === "pending" ? "Retrying…" : "Retry now"
                            }),
                            canOverrideClaim && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: "".concat(retryBtn, " ").concat(retryBtnAlwaysOn),
                                "aria-expanded": overrideOpen,
                                onClick: (e)=>{
                                    e.stopPropagation();
                                    setOverrideOpen((open)=>!open);
                                },
                                "data-testid": "stuck-item-override-claim",
                                children: "Override claim"
                            }),
                            onSnooze && // analytics-exempt
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: "".concat(snoozeBtn, " ").concat(hoverUnavailable ? snoozeBtnAlwaysOn : ""),
                                onClick: handleSnoozeTriggerClick,
                                "aria-haspopup": "true",
                                "aria-expanded": snoozeOpen,
                                "aria-label": hoverUnavailable ? "Snooze options" : "Snooze this item",
                                title: "Snooze",
                                "data-testid": "stuck-item-snooze-trigger",
                                children: hoverUnavailable ? "⋮" : "Snooze"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: metaRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                children: identity
                            }),
                            unknown && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                "data-testid": "stuck-item-last-checked",
                                children: [
                                    "\xb7 last checked ",
                                    (0,stuckReason/* formatAgo */.n4)(item.lastCheckedAt),
                                    " (check failing)"
                                ]
                            }),
                            otherReasonsCount > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: otherReasonsBadge,
                                title: otherReasonLabels.length > 0 ? otherReasonLabels.join(", ") : undefined,
                                "data-testid": "stuck-item-other-reasons-badge",
                                children: [
                                    "\xb7 also stuck for ",
                                    otherReasonsCount,
                                    " other reason",
                                    otherReasonsCount !== 1 ? "s" : "",
                                    " ⓘ"
                                ]
                            })
                        ]
                    }),
                    retryState === "error" && retryErrorMessage && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: retryError,
                        "data-testid": "stuck-item-retry-error",
                        children: [
                            "Retry failed: ",
                            retryErrorMessage
                        ]
                    })
                ]
            }),
            overrideOpen && canOverrideClaim && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                onClick: (e)=>e.stopPropagation(),
                onKeyDown: (e)=>e.stopPropagation(),
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ClaimOverrideForm/* ClaimOverrideForm */.V, {
                    label: "Reason for working on this despite the other host's claim (required)",
                    confirmLabel: "Override claim block",
                    onConfirm: handleOverrideConfirm,
                    onCancel: ()=>setOverrideOpen(false),
                    busy: overrideBusy,
                    errorMessage: overrideError
                })
            }),
            snoozeOpen && onSnooze && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: snoozePicker,
                role: "group",
                "aria-label": "Snooze duration",
                "data-testid": "stuck-item-snooze-picker",
                onClick: (e)=>e.stopPropagation(),
                onKeyDown: handleSnoozePickerKeyDown,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: snoozeOptions,
                        children: Object.keys(SNOOZE_DURATION_LABELS).map((d)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: snoozeOptionLabel,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        type: "radio",
                                        name: "snooze-duration-".concat(item.itemId, "-").concat(item.reason),
                                        value: d,
                                        checked: snoozeDuration === d,
                                        onChange: ()=>setSnoozeDuration(d),
                                        "data-testid": "stuck-item-snooze-option-".concat(d)
                                    }),
                                    SNOOZE_DURATION_LABELS[d]
                                ]
                            }, d))
                    }),
                    snoozeState === "error" && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: snoozeErrorRow,
                        "data-testid": "stuck-item-snooze-error",
                        children: "Couldn't snooze — try again"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: snoozeActions,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: snoozeCancelBtn,
                                onClick: handleSnoozeCancel,
                                "data-testid": "stuck-item-snooze-cancel",
                                children: "Cancel"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: snoozeConfirmBtn,
                                onClick: handleSnoozeConfirm,
                                disabled: snoozeState === "pending",
                                "data-testid": "stuck-item-snooze-confirm",
                                children: snoozeState === "pending" ? "Snoozing…" : snoozeState === "error" ? "Retry" : "Confirm"
                            })
                        ]
                    })
                ]
            }),
            justResolved && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: resolvedBanner,
                "data-testid": "stuck-item-resolved-banner",
                children: [
                    "✓ ",
                    resolvedMessage || "This item was just resolved.",
                    " ",
                    resolvedTrailingMessage || "It will be removed from this list shortly."
                ]
            }),
            isExpanded && !justResolved && /*#__PURE__*/ (0,jsx_runtime.jsx)(StuckItemDetail, {
                item: item,
                onReworkCapOverride: onReworkCapOverride,
                currentReworkCapOverride: currentReworkCapOverride,
                reworkCapOverrideLoaded: reworkCapOverrideLoaded,
                onApprovePlan: onApprovePlan,
                onDiagnose: onDiagnose
            })
        ]
    });
}

;// ./src/components/backlog-stuck/stuckItemsSectionShared.ts

function itemKey(item) {
    return "".concat(item.itemId, "::").concat(item.reason);
}
// multiple_reasons / bounce_cap_exhausted are synthetic aggregate rows over an
// item's *other* stuck reasons, not reasons themselves — exclude them from
// "other reasons" counting/labeling (mirrors the backend's own self-exclusion
// in reconcileMultiReasonEscalation), or a 2-reason item's own escalation row
// double-counts as a 3rd (plan.md Task 2.1.1c).
function isEscalationReason(reason) {
    return reason === backlog_pb/* StuckReason */.Pwx.MULTIPLE_REASONS || reason === backlog_pb/* StuckReason */.Pwx.BOUNCE_CAP_EXHAUSTED;
}

;// ./src/components/backlog-stuck/useResolvedGhosts.ts



const GHOST_LIFETIME_MS = 2800;
/**
 * Builds the "just resolved" copy for one ghost row.
 *
 * Only MULTIPLE_REASONS gets de-escalation copy, not BOUNCE_CAP_EXHAUSTED —
 * intentional, not an oversight: bounce_cap_exhausted can only ever coexist
 * with an open bouncing row (backend invariant, see reconcileBouncingItems'
 * paired resolve), so it never resolves independently while the item still
 * has open non-escalation reasons. If that invariant ever changes, this
 * condition needs the same OR as isEscalationReason.
 */ function buildGhostCopy(item, items, nextItemIds) {
    const isDeescalation = item.reason === backlog_pb/* StuckReason */.Pwx.MULTIPLE_REASONS && nextItemIds.has(item.itemId);
    if (!isDeescalation) {
        const message = item.reason === backlog_pb/* StuckReason */.Pwx.PR_READY_UNMERGED && item.prNumber > 0 ? "PR #".concat(item.prNumber, " was merged.") : "This item was just resolved.";
        return {
            message
        };
    }
    const remainingReasons = items.filter((i)=>i.itemId === item.itemId && !isEscalationReason(i.reason)).length;
    return {
        message: "No longer critical — down to ".concat(remainingReasons, " open reason").concat(remainingReasons !== 1 ? "s" : "", "."),
        trailingMessage: "This card will be removed shortly; the item itself is still open elsewhere in the list."
    };
}
/** Adds one ghost entry per newly-missing item that isn't already tracked. */ function addGhostEntries(prev, newlyMissingExpanded, items, nextItemIds, scheduleGhostExpiry) {
    const next = new Map(prev);
    const toAdd = newlyMissingExpanded.filter((item)=>!next.has(itemKey(item)));
    for (const item of toAdd){
        const key = itemKey(item);
        next.set(key, {
            item,
            ...buildGhostCopy(item, items, nextItemIds)
        });
        scheduleGhostExpiry(key);
    }
    return next;
}
/**
 * Surface 12: an item that resolves while its card is expanded gets a brief
 * "was just resolved" confirmation instead of being yanked out immediately.
 * Extracted from StuckItemsSection.tsx — same behavior, isolated as its own concern.
 *
 * Task 2.1.4a: `itemKey` is per-(itemId, reason), so a `multiple_reasons` row
 * de-escalating (resolving while the item itself remains open under other
 * reasons) is *already* caught by this same "row disappeared while expanded"
 * comparison — each reason is its own list entry. The only extension needed
 * is distinguishing that case (item still present under another reason) from
 * true full-item resolution, so the copy doesn't falsely claim the whole item
 * is going away.
 */ function useResolvedGhosts(items, expandedKeys, setExpandedKeys) {
    const [resolvedGhosts, setResolvedGhosts] = (0,react.useState)(new Map());
    const prevItemsRef = (0,react.useRef)([]);
    const ghostTimersRef = (0,react.useRef)(new Map());
    // Expires one ghost row after GHOST_LIFETIME_MS: drops it from resolvedGhosts
    // and finally collapses the (now-gone) card out of expandedKeys.
    const scheduleGhostExpiry = (key)=>{
        const timer = setTimeout(()=>{
            setResolvedGhosts((p)=>{
                const n = new Map(p);
                n.delete(key);
                return n;
            });
            setExpandedKeys((p)=>{
                const n = new Set(p);
                n.delete(key);
                return n;
            });
            ghostTimersRef.current.delete(key);
        }, GHOST_LIFETIME_MS);
        ghostTimersRef.current.set(key, timer);
    };
    (0,react.useEffect)(()=>{
        const prevItems = prevItemsRef.current;
        prevItemsRef.current = items;
        const nextKeys = new Set(items.map(itemKey));
        const newlyMissingExpanded = prevItems.filter((p)=>expandedKeys.has(itemKey(p)) && !nextKeys.has(itemKey(p)));
        if (newlyMissingExpanded.length === 0) return;
        const nextItemIds = new Set(items.map((i)=>i.itemId));
        setResolvedGhosts((prev)=>addGhostEntries(prev, newlyMissingExpanded, items, nextItemIds, scheduleGhostExpiry));
    // scheduleGhostExpiry is recreated every render but only reads from refs/setters
    // that are themselves stable, so omitting it keeps this effect's dependency set
    // focused on the data that actually drives it.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        items,
        expandedKeys,
        setExpandedKeys
    ]);
    (0,react.useEffect)(()=>{
        const timers = ghostTimersRef.current;
        return ()=>{
            for (const t of timers.values())clearTimeout(t);
        };
    }, []);
    return resolvedGhosts;
}

;// ./src/components/backlog-stuck/useReworkCapOverrides.ts



/**
 * Fetches the current reworkCapOverride for newly-expanded REWORK_CAP items
 * so StuckItemDetail can show it instead of guessing blind. Extracted from
 * StuckItemsSection.tsx — same behavior, isolated as its own concern. Skips
 * items already present in the map (including a resolved `undefined`) so
 * re-expanding doesn't re-fetch.
 *
 * reworkCapOverrides is deliberately NOT a dependency of the effect below
 * even though it's read inside (via reworkCapFetchStartedRef.current
 * instead) — it used to be, but every successful per-item fetch changed the
 * map reference and re-fired the effect while the previous invocation's
 * for-loop was still mid-flight on later items, causing O(N^2) duplicate
 * getBacklogItem calls when several REWORK_CAP items were expanded at once.
 * reworkCapFetchStartedRef (a ref, so mutating it can't trigger a
 * re-render/re-run) tracks "already fetched or in-flight" instead, updated
 * synchronously before the async fetch starts so an overlapping effect
 * invocation (from items/expandedKeys changing mid-batch) can't double-fetch
 * the same item either.
 */ function useReworkCapOverrides(items, expandedKeys, getBacklogItem) {
    const [reworkCapOverrides, setReworkCapOverrides] = (0,react.useState)(new Map());
    // Mirrors reworkCapOverrides synchronously so the effect's cleanup can check
    // "was this item actually committed?" without depending on a stale closure
    // over the state value — the effect deliberately excludes reworkCapOverrides
    // from its dependency array, so its own closure only ever sees the map as of
    // when the effect instance started, not later commits made mid-batch.
    const reworkCapOverridesRef = (0,react.useRef)(new Map());
    // Tracks itemIds already fetched-or-in-flight. Deliberately NOT derived from
    // reworkCapOverrides state — see the doc comment above.
    const reworkCapFetchStartedRef = (0,react.useRef)(new Set());
    (0,react.useEffect)(()=>{
        // Captured once per effect instance (not re-read from the ref inside the
        // cleanup) purely to satisfy react-hooks/exhaustive-deps's "ref value may
        // have changed by cleanup time" check — reworkCapFetchStartedRef.current
        // is a single long-lived Set that's only ever mutated in place, never
        // reassigned, so this alias and `.current` always point at the same Set.
        const fetchStarted = reworkCapFetchStartedRef.current;
        const toFetch = items.filter((item)=>item.reason === backlog_pb/* StuckReason */.Pwx.REWORK_CAP && expandedKeys.has(itemKey(item)) && !reworkCapOverrides.has(item.itemId) && !fetchStarted.has(item.itemId));
        if (toFetch.length === 0) return;
        for (const item of toFetch)fetchStarted.add(item.itemId);
        let cancelled = false;
        void (async ()=>{
            for (const item of toFetch){
                const full = await getBacklogItem(item.itemId);
                if (cancelled) return;
                if (full === null) {
                    // getBacklogItem swallows RPC errors (and "client not ready yet")
                    // as null (see useBacklogService.ts's getBacklogItem) — caching
                    // that as a resolved `undefined` would be indistinguishable from a
                    // genuinely-unset override and StuckItemDetail would confidently
                    // (and wrongly) render "No override set" after a transient
                    // network failure. Leave it out of the map entirely so it's
                    // absent from `reworkCapOverrides`, and release the started-marker
                    // so a later effect invocation (e.g. the item being re-expanded)
                    // retries instead of being stuck permanently unfetched.
                    fetchStarted.delete(item.itemId);
                    continue;
                }
                setReworkCapOverrides((prev)=>{
                    if (prev.has(item.itemId)) return prev;
                    const next = new Map(prev);
                    next.set(item.itemId, full.reworkCapOverride);
                    reworkCapOverridesRef.current = next;
                    return next;
                });
            }
        })();
        return ()=>{
            cancelled = true;
            // This invocation was interrupted (items/expandedKeys changed) before
            // finishing its whole batch — release the started-marker for anything
            // it didn't get to commit, so a later invocation can still fetch it
            // instead of leaving it stuck marked "in flight" forever.
            // reworkCapOverridesRef (not the possibly-stale `reworkCapOverrides`
            // closed over by this effect instance) reflects every commit made up
            // to this exact moment, including ones from this same batch.
            for (const item of toFetch){
                if (!reworkCapOverridesRef.current.has(item.itemId)) {
                    fetchStarted.delete(item.itemId);
                }
            }
        };
    // reworkCapOverrides is deliberately excluded; see the doc comment above this hook.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        items,
        expandedKeys,
        getBacklogItem
    ]);
    return reworkCapOverrides;
}

;// ./src/components/backlog-stuck/useBulkResetParked.ts


/**
 * "Reset parked" state shared by the global "Reset all parked" button and
 * each per-reason-group "Reset parked (N)" button. Extracted from
 * StuckItemsSection.tsx — same behavior, isolated as its own concern. Reason
 * omitted resets across every reason (unchanged existing behavior), reason
 * set scopes the reset (and the resulting message) to just that bucket.
 */ function useBulkResetParked(bulkResetParkedRemediation) {
    const [bulkResetState, setBulkResetState] = (0,react.useState)("idle");
    const [bulkResetMessage, setBulkResetMessage] = (0,react.useState)(null);
    // Which single reason's "Reset parked (N)" button is mid-flight — kept
    // separate from bulkResetState (the global "Reset all parked" button's own
    // pending flag) so each button can show its own label/spinner, but both are
    // combined below into anyResetPending so the two families still can't fire
    // overlapping resets that would clobber the single shared bulkResetMessage.
    const [resettingReason, setResettingReason] = (0,react.useState)(null);
    // True while either the global or a per-reason reset is in flight — gates
    // both button families so an operator can't fire two overlapping
    // BulkResetStuckRemediation calls whose resulting toasts would clobber the
    // single shared bulkResetMessage state.
    const anyResetPending = bulkResetState === "pending" || resettingReason !== null;
    const handleBulkResetParked = (0,react.useCallback)(async (reason)=>{
        if (anyResetPending) {
            return;
        }
        if (reason !== undefined) {
            setResettingReason(reason);
        } else {
            setBulkResetState("pending");
        }
        setBulkResetMessage(null);
        try {
            const n = await bulkResetParkedRemediation(reason);
            setBulkResetState("idle");
            const scope = reason !== undefined ? " in ".concat((0,stuckReason/* getStuckReasonLabel */.Lv)(reason)) : "";
            setBulkResetMessage(n > 0 ? "Reset ".concat(n, " parked item").concat(n !== 1 ? "s" : "").concat(scope, ".") : "No parked items to reset.");
        } catch (err) {
            setBulkResetState("error");
            setBulkResetMessage(err instanceof Error ? err.message : "Bulk reset failed");
        } finally{
            setResettingReason(null);
        }
    }, [
        anyResetPending,
        bulkResetParkedRemediation
    ]);
    return {
        bulkResetState,
        bulkResetMessage,
        resettingReason,
        anyResetPending,
        handleBulkResetParked
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog-stuck%2FStuckItemsSection.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA81X247bKBi-n6fgZqVGWiLb03gaerNSH6TCNrZpbH4v4Mlh1XdfAUkGj03iyWTaKtJkAv-J7z_w8c-G7UtJW6ZQJiRdyQr994BQKaG1_yCkJRWqBNkSJEFTzT5FBasWXx8Q-vmAkIag3GPqSf58WDoHkZXPQBZMEhR3O6Sg4QV6pvITxt_jNTsk8sCsmpPCkha8V-SVyOOTleloUXBRjXZju1tw1TV0T1DZsJ1ZMN-44JLlmoMgKIemb4XZqWg3MhItvr5EHtvIRxZpwyuBuWatIihnQjNpln_0SvNyj3MQmglNkOpoznDG9Jaxiw6PQW6l2Td_vRgSlx4QGit-YCP9wumb_S3jVa0Jeoois5ZDA_K1vHPXUllxQVDkOXq84ihbBI0m4xhWkW_78zSQE3gkh9d4CHCIIATPTJYNbPGOINprMGsql9A0GZV4ywtdG3HBzAbesmzDNT4rOUlbOBr6vPbDI-QkfrY3jNhZPSus7G6gEJP9YjrJl4t7_QLiJfx7qUwCOuCnupvZWTTfVBJ6URDXuR2VTOhgTl03WUnuGufFAoqWcaL-doruhxHe1lwzbKveT5vNpaolF5thxa1IbbLjxoMX3jCQL-G6GzTripSQ9wo_c8Wzhlmr0OuGC0ZQEsCmscaPYhjKUjFtkfQMp1cChHCA__p5n5QA_wjpLEBE2N_Kt_Y0Y3rZ3DBR3Dy_xv36an59mTVWBrMjvTC_4sH8GsWCIvPxbgmcgdbQTvSol5mTTKiDyvMZCYq9s629my3U1umtbbu7qW0PQTiDbbieVXX7xUDlg3ptTQquaNawwtnsaM71nqBomfowCtCYNg1sWeEp07dN5eRwZeJSV2tc4PoIZfJ5EG023WFvYhw2irPF_O4WHZCa7QxivBI-Y7lwmYdI1uPCtVcYtdz3zu7rPbnqPbsDCTwWxiTNO5-sfBM3DHu5RqgunDJMhAf9XQ8j5cL2X9ZAvrG3tuNN0XLNWvP7VOvnhdOgCvX1gLlr6KYvunaKAq2ivyxogrf0SDKOb5JomSpkAqUScVFywbVPwPgvY-ahcXK4ieSlo6E-nrIXJ_u8W-PqdRu8rn_8bmCPz7j3wRjHYRzjZBaQcfROJDcf8EZI7_dGyHtpiMQ3A9E8osFFzSTXb-YXm48iC81NCF9G72y8vf05HPt2xDvsJL4h-LPr6dqbc7j_NBM9uOsTEe5dif8DZpoL_k0TAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_stuck_2FStuckItemsSection_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81X247bKBi_n6fgZqVGWiLb03gaerNSH6TCNrZpbH4v4Mlh1XdfAUkGj03iyWTaKtJkAv_J7z_w8c_G7UtJW6ZQJiRdyQr994BQKaG1_yCkJRWqBNkSJEFTzT5FBasWXx8Q_vmAkIag3GPqSf58WDoHkZXPQBZMEhR3O6Sg4QV6pvITxt_jNTsk8sCsmpPCkha8V_SVyOOTleloUXBRjXZju1tw1TV0T1DZsJ1ZMN_44JLlmoMgKIemb4XZqWg3MhItvr5EHtvIRxZpwyuBuWatIihnQjNpln_0SvNyj3MQmglNkOpoznDG9Jaxiw6PQW6l2Td_vRgSlx4QGit_YCP9wumb_S3jVa0Jeoois5ZDA_K1vHPXUllxQVDkOXq84ihbBI0m4xhWkW_78zSQE3gkh9d4CHCIIATPTJYNbPGOINprMGsql9A0GZV4ywtdG3HBzAbesmzDNT4rOUlbOBr6vPbDI_QkfrY3jNhZPSus7G6gEJP9YjrJl4t7_QLiJfx7qUwCOuCnupvZWTTfVBJ6URDXuR2VTOhgTl03WUnuGufFAoqWcaL_doruhxHe1lwzbKveT5vNpaolF5thxa1IbbLjxoMX3jCQL_G6GzTripSQ9wo_c8Wzhlmr0OuGC0ZQEsCmscaPYhjKUjFtkfQMp1cChHCA_p5n5QA_wjpLEBE2N_Kt_Y0Y3rZ3DBR3Dy_xv36an59mTVWBrMjvTC_4sH8GsWCIvPxbgmcgdbQTvSol5mTTKiDyvMZCYq9s629my3U1umtbbu7qW0PQTiDbbieVXX7xUDlg3ptTQquaNawwtnsaM71nqBomfowCtCYNg1sWeEp07dN5eRwZeJSV2tc4PoIZfJ5EG023WFvYhw2irPF_O4WHZCa7QxivBI_Y7lwmYdI1uPCtVcYtdz3zu7rPbnqPbsDCTwWxiTNO5_sfBM3DHu5RqgunDJMhAf9XQ8j5cL2X9ZAvrG3tuNN0XLNWvP7VOvnhdOgCvX1gLlr6KYvunaKAq2ivyxogrf0SDKOb5JomSpkAqUScVFywbVPwPgvY_ahcXK4ieSlo6E_nrIXJ_u8W_PqdRu8rn_8bmCPz7j3wRjHYRzjZBaQcfROJDcf8EZI7_dGyHtpiMQ3A9E8osFFzSTXb_YXm48iC81NCF9G72y8vf05HPt2xDvsJL4h_LPr6dqbc7j_NBM9uOsTEe5dif8DZpoL_k0TAAA_22_7D = __webpack_require__(63975);
;// ./src/components/backlog-stuck/StuckItemsSection.css.ts


var StuckItemsSection_css_chip = 'bnra5r5';
var chipActive = 'bnra5r6';
var clearFilterBtn = 'bnra5ro';
var countRegion = 'bnra5r3';
var StuckItemsSection_css_empty = 'bnra5rd';
var errorBanner = 'bnra5ri';
var errorBannerFullBody = 'bnra5rj';
var filterRow = 'bnra5r4';
var filteredEmpty = 'bnra5re';
var StuckItemsSection_css_group = 'bnra5rb';
var groupHeading = 'bnra5r8';
var groupHeadingRow = 'bnra5r7';
var StuckItemsSection_css_itemList = 'bnra5rc';
var loading = 'bnra5rf';
var resetParkedBtn = 'bnra5rl bnra5r9';
var resetParkedMessage = 'bnra5rm';
var resetParkedMessageError = 'bnra5rn';
var resetParkedReasonBtn = 'bnra5ra bnra5r9';
var StuckItemsSection_css_retryBtn = 'bnra5rk';
var StuckItemsSection_css_section = 'bnra5r0';
var StuckItemsSection_css_sectionHeader = 'bnra5r1';
var StuckItemsSection_css_sectionTitle = 'bnra5r2';
var StuckItemsSection_css_spinner = 'bnra5rh';
;// ./src/components/backlog-stuck/StuckItemsSection.tsx
// +feature: backlog-stuck-items
/* __next_internal_client_entry_do_not_use__ StuckItemsSection auto */ 












// Fixed, deliberate display order — by typical actionability, NOT severity.
// pr_ready_unmerged leads (one known next step: merge it); the remaining
// reasons that need a decision or investigation follow. This must never be
// read as a danger/severity ranking (design/ux.md Surface 2).
//
// IMPORTANT: this list must be kept in sync with every StuckReason value
// (stuckReason.ts's STUCK_REASON_LABELS/ICONS/CLASS maps are `Record<StuckReason,
// T>` and so are compile-checked exhaustive, but this array is not — a
// reason present in `grouped` (below) yet absent here is silently never
// rendered even though it still counts toward the total/badge, which is
// exactly the kind of count-vs-list mismatch this feature exists to avoid).
// plan_not_approved, spawn_failed, pr_pending_no_pr, rework_blocked_stale,
// pr_needs_fix, and respawn_blocked_active were all previously missing here
// (backlog/plan-approval-flicker fix, 2026-08) — found via the e2e test for
// the plan-approval flicker fix never being able to find a seeded
// plan_not_approved item's card despite the section's own count showing 1.
const GROUP_ORDER = [
    backlog_pb/* StuckReason */.Pwx.PR_READY_UNMERGED,
    backlog_pb/* StuckReason */.Pwx.PLAN_NOT_APPROVED,
    backlog_pb/* StuckReason */.Pwx.ABANDONED_REVIEW,
    backlog_pb/* StuckReason */.Pwx.STALE_WORK,
    backlog_pb/* StuckReason */.Pwx.ORPHANED_TRIAGE,
    backlog_pb/* StuckReason */.Pwx.REWORK_CAP,
    backlog_pb/* StuckReason */.Pwx.AUTONOMOUS_STUCK,
    backlog_pb/* StuckReason */.Pwx.BOUNCING,
    backlog_pb/* StuckReason */.Pwx.MULTIPLE_REASONS,
    backlog_pb/* StuckReason */.Pwx.BOUNCE_CAP_EXHAUSTED,
    backlog_pb/* StuckReason */.Pwx.PUSH_FAILED,
    backlog_pb/* StuckReason */.Pwx.SPAWN_FAILED,
    backlog_pb/* StuckReason */.Pwx.STEER_FAILED,
    backlog_pb/* StuckReason */.Pwx.VERDICT_UNDELIVERED,
    backlog_pb/* StuckReason */.Pwx.PR_PENDING_NO_PR,
    backlog_pb/* StuckReason */.Pwx.REWORK_BLOCKED_STALE,
    backlog_pb/* StuckReason */.Pwx.PR_NEEDS_FIX,
    backlog_pb/* StuckReason */.Pwx.RESPAWN_BLOCKED_ACTIVE,
    backlog_pb/* StuckReason */.Pwx.LIKELY_FLAKY,
    backlog_pb/* StuckReason */.Pwx.BLOCKED_BY_DEPENDENCY,
    backlog_pb/* StuckReason */.Pwx.BLOCKED_BY_CLAIM,
    backlog_pb/* StuckReason */.Pwx.WORKTREE_INCONSISTENT,
    backlog_pb/* StuckReason */.Pwx.REPEATED_NOOP_DISPATCH,
    backlog_pb/* StuckReason */.Pwx.MERGED_PR_UNVERIFIED
];
function firstDetectedMs(item) {
    const ts = item.firstDetectedAt;
    if (!ts) return 0;
    return Number(ts.seconds) * 1000;
}
/**
 * "Stuck Backlog Items" section — grouped-by-reason, filterable list mounted
 * on /unfinished, directly below the existing filter-chip row and above
 * GitHubPRsSection (design/ux.md Surface 2).
 */ /** Mirrors StuckItem.tsx's MAX_REMEDIATION_ATTEMPTS — see that constant's doc comment. */ const MAX_REMEDIATION_ATTEMPTS = 5;
function StuckItemsSection() {
    let { focusItemId } = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : {};
    const { items, isLoading, error, lastFetched, refetch, snooze, bulkResetParkedRemediation, triggerRemediationNow, overrideClaimBlock } = (0,useStuckBacklogItems/* useStuckBacklogItems */.d)();
    const { updateBacklogItem, transitionStatus, spawnSessionFromItem, approvePlan, getBacklogItem } = (0,useBacklogService/* useBacklogService */.gG)();
    const { dispatchDiagnose } = useDiagnoseAction();
    const [filter, setFilter] = (0,react.useState)("all");
    const [expandedKeys, setExpandedKeys] = (0,react.useState)(new Set());
    const resolvedGhosts = useResolvedGhosts(items, expandedKeys, setExpandedKeys);
    // Populated lazily on expand for REWORK_CAP items only — StuckBacklogItem
    // (the list-fetch shape) doesn't carry reworkCapOverride, so the current
    // value has to be fetched via getBacklogItem (full BacklogItem) on demand.
    const reworkCapOverrides = useReworkCapOverrides(items, expandedKeys, getBacklogItem);
    const { bulkResetState, bulkResetMessage, resettingReason, anyResetPending, handleBulkResetParked } = useBulkResetParked(bulkResetParkedRemediation);
    const appliedFocusItemIdRef = (0,react.useRef)(undefined);
    const toggleExpand = (0,react.useCallback)((key)=>{
        setExpandedKeys((prev)=>{
            const next = new Set(prev);
            if (next.has(key)) {
                next.delete(key);
            } else {
                next.add(key);
            }
            return next;
        });
    }, []);
    const handleClearFilter = (0,react.useCallback)(()=>setFilter("all"), []);
    // /unfinished?item=<itemId> deep link (routes.unfinishedItem): pre-expand
    // every card matching this itemId (a bare itemId can match multiple
    // composite keys, since one item can appear under several reasons), and
    // fall back to the "all" filter if the currently active reason filter
    // would otherwise hide every matching card.
    //
    // Applied at most once per focusItemId (tracked via appliedFocusItemIdRef),
    // not on every `items` change: useStuckBacklogItems() hands back a fresh
    // array reference on every ~60s poll tick regardless of whether the data
    // actually changed, and focusItemId never clears itself from the URL. An
    // unguarded effect would re-run on each poll and silently re-expand a card
    // the user had just manually collapsed, or re-reset a filter they'd since
    // changed away from "all".
    (0,react.useEffect)(()=>{
        if (!focusItemId || appliedFocusItemIdRef.current === focusItemId) return;
        const matches = items.filter((i)=>i.itemId === focusItemId);
        if (matches.length === 0) return;
        appliedFocusItemIdRef.current = focusItemId;
        setExpandedKeys((prev)=>{
            const next = new Set(prev);
            for (const item of matches)next.add(itemKey(item));
            return next;
        });
        setFilter((prevFilter)=>{
            if (prevFilter === "all") return prevFilter;
            const stillVisible = matches.some((i)=>i.reason === prevFilter);
            return stillVisible ? prevFilter : "all";
        });
    }, [
        focusItemId,
        items
    ]);
    // rework_cap "continue automatically" action: sets the item's per-item
    // override then immediately reopens it — mirrors BacklogItemDetail.tsx's
    // handleGateReopen exactly (transition to in_progress, spawn a fresh work
    // session), so the item starts working again in the same click instead of
    // requiring a separate "Reopen for Revision" click elsewhere. On success,
    // future automatic rework/re-review rounds for this item use the raised
    // cap instead of the global default (see effectiveReworkCap on the backend).
    const handleReworkCapOverride = (0,react.useCallback)(async (itemId, override)=>{
        try {
            const updated = await updateBacklogItem(itemId, {
                reworkCapOverride: override
            });
            if (!updated) return false;
            await transitionStatus(itemId, "in_progress");
            await spawnSessionFromItem(itemId);
            await refetch();
            return true;
        } catch (err) {
            console.error("[StuckItemsSection] reworkCapOverride reopen failed:", err);
            return false;
        }
    }, [
        updateBacklogItem,
        transitionStatus,
        spawnSessionFromItem,
        refetch
    ]);
    // BUG-038 follow-up: the only "Approve Plan" UI action lived inside the
    // item-detail page's `status === "ready"` block, but items this reason
    // flags are stuck in `status === "queued"` — so that button was never
    // reachable from here. Approve directly from the stuck-item card instead.
    //
    // Deliberately NOT try/catch-swallowed (unlike the other handlers in this
    // file): useBacklogService's approvePlan rethrows the backend's
    // FailedPrecondition message verbatim (e.g. "no plan artifacts found — run
    // TriggerTriage first"), and StuckItemDetail needs that specific message
    // rather than a generic failure, in case the hasPlan gate is ever stale.
    const handleApprovePlan = (0,react.useCallback)(async (itemId)=>{
        // approvePlan resolves null (without throwing) if the RPC client isn't
        // ready yet — must not let that silently read as success, which would
        // reintroduce the "looks approved but isn't" flicker (see
        // backlog/plan-approval-flicker).
        const updated = await approvePlan(itemId);
        if (!updated) throw new Error("Approve plan did not return an updated item.");
        await refetch();
    }, [
        approvePlan,
        refetch
    ]);
    // Diagnose & Nudge (backlog item 68964304): dispatches a diagnostic agent
    // scoped to this card's own StuckReason. Deliberately NOT
    // try/catch-swallowed, same rationale as handleApprovePlan above — the
    // card needs the specific dispatch failure, not a generic one. Does not
    // refetch(): dispatching a diagnostic session doesn't change this item's
    // stuck-state row by itself (a nudge, if the agent performs one, is what
    // eventually resolves the condition on a later poll tick). Returns the
    // dispatched session's UUID so StuckItemDetail can link straight to it —
    // see onDiagnose's doc comment (StuckItem.tsx) for why that link matters.
    const handleDiagnose = (0,react.useCallback)(async (itemId, reason)=>{
        const resp = await dispatchDiagnose(itemId, reason);
        return resp.diagnosticSessionUuid;
    }, [
        dispatchDiagnose
    ]);
    // Visible items: the filtered set actually rendered. Cross-reference badges
    // are computed from this set so they auto-suppress once a filter narrows an
    // item to a single visible card.
    //
    // A row that just resolved (tracked in resolvedGhosts) has, by definition,
    // already disappeared from `items` — grouping/rendering purely off `items`
    // would make its ghost/confirmation card (justResolved banner) never
    // actually appear, since it wouldn't be in any group to render at all. Keep
    // it visible for its fade-out window by re-including it here (deduped
    // against `items`, and still respecting the active filter) until its ghost
    // timer clears it from resolvedGhosts.
    const visibleItems = (0,react.useMemo)(()=>{
        const base = filter === "all" ? items : items.filter((i)=>i.reason === filter);
        if (resolvedGhosts.size === 0) return base;
        const baseKeys = new Set(base.map(itemKey));
        const ghostOnlyItems = Array.from(resolvedGhosts.values()).map((g)=>g.item).filter((item)=>!baseKeys.has(itemKey(item)) && (filter === "all" || item.reason === filter));
        return ghostOnlyItems.length > 0 ? [
            ...base,
            ...ghostOnlyItems
        ] : base;
    }, [
        items,
        filter,
        resolvedGhosts
    ]);
    // Counts only non-escalation reasons per item — the two escalation reasons
    // are aggregate summaries of the other rows, not additional "other reasons"
    // in their own right (see isEscalationReason's doc comment).
    const itemIdCounts = (0,react.useMemo)(()=>{
        const counts = new Map();
        for (const item of visibleItems){
            if (isEscalationReason(item.reason)) continue;
            var _counts_get;
            counts.set(item.itemId, ((_counts_get = counts.get(item.itemId)) !== null && _counts_get !== void 0 ? _counts_get : 0) + 1);
        }
        return counts;
    }, [
        visibleItems
    ]);
    const otherReasonLabelsFor = (0,react.useCallback)((item)=>visibleItems.filter((i)=>i.itemId === item.itemId && i.reason !== item.reason && !isEscalationReason(i.reason)).map((i)=>(0,stuckReason/* getStuckReasonLabel */.Lv)(i.reason)), [
        visibleItems
    ]);
    const grouped = (0,react.useMemo)(()=>{
        const map = new Map();
        for (const reason of GROUP_ORDER)map.set(reason, []);
        for (const item of visibleItems){
            const list = map.get(item.reason);
            if (list) {
                list.push(item);
            } else {
                map.set(item.reason, [
                    item
                ]);
            }
        }
        for (const list of map.values()){
            list.sort((a, b)=>firstDetectedMs(a) - firstDetectedMs(b));
        }
        return map;
    }, [
        visibleItems
    ]);
    const countsByReason = (0,react.useMemo)(()=>{
        const counts = new Map();
        for (const item of items){
            var _counts_get;
            counts.set(item.reason, ((_counts_get = counts.get(item.reason)) !== null && _counts_get !== void 0 ? _counts_get : 0) + 1);
        }
        return counts;
    }, [
        items
    ]);
    const totalCount = items.length;
    const activeFilterLabel = filter === "all" ? null : (0,stuckReason/* getStuckReasonLabel */.Lv)(filter);
    const filteredCount = visibleItems.length;
    const parkedCount = (0,react.useMemo)(()=>items.filter((i)=>i.remediationAttempts >= MAX_REMEDIATION_ATTEMPTS).length, [
        items
    ]);
    const parkedCountByReason = (0,react.useMemo)(()=>{
        const counts = new Map();
        for (const item of items){
            if (item.remediationAttempts >= MAX_REMEDIATION_ATTEMPTS) {
                var _counts_get;
                counts.set(item.reason, ((_counts_get = counts.get(item.reason)) !== null && _counts_get !== void 0 ? _counts_get : 0) + 1);
            }
        }
        return counts;
    }, [
        items
    ]);
    const chips = [
        {
            value: "all",
            label: "All",
            count: totalCount
        },
        ...GROUP_ORDER.map((reason)=>{
            var _countsByReason_get;
            return {
                value: reason,
                label: (0,stuckReason/* getStuckReasonLabel */.Lv)(reason),
                count: (_countsByReason_get = countsByReason.get(reason)) !== null && _countsByReason_get !== void 0 ? _countsByReason_get : 0
            };
        })
    ];
    const showFirstLoadError = error !== null && lastFetched === null;
    const showStaleBanner = error !== null && lastFetched !== null;
    const showInitialLoading = isLoading && lastFetched === null && !error;
    let body;
    if (showFirstLoadError) {
        body = /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: errorBannerFullBody,
            "data-testid": "stuck-items-error-full",
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    children: "⚠ Couldn't check for stuck items right now."
                }),
                // analytics-exempt
                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    className: StuckItemsSection_css_retryBtn,
                    onClick: refetch,
                    "data-testid": "stuck-items-retry",
                    children: "Retry"
                })
            ]
        });
    } else if (showInitialLoading) {
        body = /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: loading,
            "data-testid": "stuck-items-loading",
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    className: StuckItemsSection_css_spinner,
                    "aria-label": "Checking"
                }),
                "Checking for stuck items…"
            ]
        });
    } else if (totalCount === 0) {
        body = /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: StuckItemsSection_css_empty,
            "data-testid": "stuck-items-empty",
            children: "✓ Nothing stuck — all backlog items are progressing."
        });
    } else if (filteredCount === 0) {
        body = /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: filteredEmpty,
            "data-testid": "stuck-items-filtered-empty",
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                    children: [
                        'No stuck items match "',
                        activeFilterLabel,
                        '".'
                    ]
                }),
                // analytics-exempt
                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    className: clearFilterBtn,
                    onClick: handleClearFilter,
                    "data-testid": "stuck-items-clear-filter",
                    children: "Clear filter"
                })
            ]
        });
    } else {
        body = /*#__PURE__*/ (0,jsx_runtime.jsx)(jsx_runtime.Fragment, {
            children: GROUP_ORDER.filter((reason)=>{
                var _grouped_get;
                var _grouped_get_length;
                return ((_grouped_get_length = (_grouped_get = grouped.get(reason)) === null || _grouped_get === void 0 ? void 0 : _grouped_get.length) !== null && _grouped_get_length !== void 0 ? _grouped_get_length : 0) > 0;
            }).map((reason)=>{
                var _grouped_get;
                const groupItems = (_grouped_get = grouped.get(reason)) !== null && _grouped_get !== void 0 ? _grouped_get : [];
                var _parkedCountByReason_get;
                const parkedInGroup = (_parkedCountByReason_get = parkedCountByReason.get(reason)) !== null && _parkedCountByReason_get !== void 0 ? _parkedCountByReason_get : 0;
                return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: StuckItemsSection_css_group,
                    "data-testid": "stuck-group-".concat(reason),
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: groupHeadingRow,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("h3", {
                                    className: groupHeading,
                                    children: [
                                        (0,stuckReason/* getStuckReasonLabel */.Lv)(reason),
                                        " (",
                                        groupItems.length,
                                        ")"
                                    ]
                                }),
                                parkedInGroup > 0 && // analytics-exempt
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "button",
                                    className: resetParkedReasonBtn,
                                    onClick: ()=>handleBulkResetParked(reason),
                                    disabled: anyResetPending,
                                    title: "Clear the automated-retry counters on every ".concat((0,stuckReason/* getStuckReasonLabel */.Lv)(reason), " item that has exhausted its 5 automated attempts, so they get a fresh shot"),
                                    "data-testid": "stuck-group-reset-parked-".concat(reason),
                                    children: resettingReason === reason ? "Resetting…" : "Reset parked (".concat(parkedInGroup, ")")
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: StuckItemsSection_css_itemList,
                            children: groupItems.map((item)=>{
                                const key = itemKey(item);
                                var _itemIdCounts_get, _itemIdCounts_get1;
                                // Escalation-reason cards (multiple_reasons/bounce_cap_exhausted) aren't
                                // themselves counted in itemIdCounts (see isEscalationReason), so they don't
                                // subtract 1 for "self" the way an ordinary reason card does.
                                const otherCount = isEscalationReason(item.reason) ? (_itemIdCounts_get = itemIdCounts.get(item.itemId)) !== null && _itemIdCounts_get !== void 0 ? _itemIdCounts_get : 0 : ((_itemIdCounts_get1 = itemIdCounts.get(item.itemId)) !== null && _itemIdCounts_get1 !== void 0 ? _itemIdCounts_get1 : 1) - 1;
                                const ghost = resolvedGhosts.get(key);
                                return /*#__PURE__*/ (0,jsx_runtime.jsx)(StuckItem, {
                                    item: item,
                                    isExpanded: expandedKeys.has(key),
                                    onToggleExpand: ()=>toggleExpand(key),
                                    otherReasonsCount: otherCount,
                                    otherReasonLabels: otherReasonLabelsFor(item),
                                    justResolved: ghost !== undefined,
                                    resolvedMessage: ghost === null || ghost === void 0 ? void 0 : ghost.message,
                                    resolvedTrailingMessage: ghost === null || ghost === void 0 ? void 0 : ghost.trailingMessage,
                                    onSnooze: snooze,
                                    onReworkCapOverride: handleReworkCapOverride,
                                    currentReworkCapOverride: reworkCapOverrides.get(item.itemId),
                                    reworkCapOverrideLoaded: reworkCapOverrides.has(item.itemId),
                                    onTriggerRemediationNow: triggerRemediationNow,
                                    onOverrideClaimBlock: overrideClaimBlock,
                                    onApprovePlan: handleApprovePlan,
                                    onDiagnose: handleDiagnose,
                                    focusItemId: focusItemId
                                }, key);
                            })
                        })
                    ]
                }, reason);
            })
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
        className: StuckItemsSection_css_section,
        "aria-label": "Stuck Backlog Items",
        "data-testid": "stuck-items-section",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: StuckItemsSection_css_sectionHeader,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                        className: StuckItemsSection_css_sectionTitle,
                        children: "Stuck Backlog Items"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: countRegion,
                        "aria-live": "polite",
                        "data-testid": "stuck-items-count",
                        children: [
                            totalCount,
                            " stuck"
                        ]
                    }),
                    parkedCount > 0 && // analytics-exempt
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: resetParkedBtn,
                        onClick: ()=>handleBulkResetParked(),
                        disabled: anyResetPending,
                        title: "Clear the automated-retry counters on every item that has exhausted its 5 automated attempts, so they get a fresh shot",
                        "data-testid": "stuck-items-reset-parked",
                        children: bulkResetState === "pending" ? "Resetting…" : "Reset all parked (".concat(parkedCount, ")")
                    })
                ]
            }),
            bulkResetMessage && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: bulkResetState === "error" ? resetParkedMessageError : resetParkedMessage,
                "aria-live": "polite",
                "data-testid": "stuck-items-reset-parked-message",
                children: bulkResetMessage
            }),
            showStaleBanner && lastFetched && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: errorBanner,
                "data-testid": "stuck-items-stale-banner",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        children: [
                            "Couldn't refresh stuck items (last updated",
                            " ",
                            Math.max(0, Math.floor((Date.now() - lastFetched.getTime()) / 60000)),
                            "m ago)."
                        ]
                    }),
                    // analytics-exempt
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: StuckItemsSection_css_retryBtn,
                        onClick: refetch,
                        "data-testid": "stuck-items-retry",
                        children: "Retry"
                    })
                ]
            }),
            !showFirstLoadError && !showInitialLoading && totalCount > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: filterRow,
                role: "group",
                "aria-label": "Filter stuck items by reason",
                children: chips.map((param)=>{
                    let { value, label, count } = param;
                    return(// analytics-exempt
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                        className: "".concat(StuckItemsSection_css_chip, " ").concat(filter === value ? chipActive : ""),
                        onClick: ()=>setFilter(value),
                        "aria-pressed": filter === value,
                        "data-testid": "stuck-filter-chip-".concat(value),
                        children: [
                            label,
                            " (",
                            count,
                            ")"
                        ]
                    }, String(value)));
                })
            }),
            body
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@radix-ui+react-tabs@1.1.13+b721ee6ccd61acaa/node_modules/@radix-ui/react-tabs/dist/index.mjs + 3 modules
var dist = __webpack_require__(51586);
;// ./src/lib/unfinished/upNextTab.ts
const UP_NEXT_TABS = [
    "prs",
    "stuck",
    "worktrees",
    "queue"
];
const UP_NEXT_TAB_STORAGE_KEY = "up-next-tab";
function parseUpNextTab(raw) {
    return UP_NEXT_TABS.includes(raw !== null && raw !== void 0 ? raw : "") ? raw : null;
}
/** Tab implied by the URL alone (`?item=` means Stuck), or null when the URL has no opinion. */ function tabFromUrl(param) {
    let { item, tab } = param;
    if (item) return "stuck";
    return parseUpNextTab(tab);
}
/** Never throws: storage can be blocked (SecurityError) or unavailable (SSR). */ function readStoredTab() {
    try {
        return parseUpNextTab(window.localStorage.getItem(UP_NEXT_TAB_STORAGE_KEY));
    } catch (e) {
        return null;
    }
}
function writeStoredTab(tab) {
    try {
        window.localStorage.setItem(UP_NEXT_TAB_STORAGE_KEY, tab);
    } catch (e) {
    // Storage unavailable: tab switching still works via the URL.
    }
}
/** Precedence: ?item= (Stuck) > valid ?tab= > stored tab > "prs". */ function resolveInitialTab(params, stored) {
    var _tabFromUrl, _ref;
    return (_ref = (_tabFromUrl = tabFromUrl(params)) !== null && _tabFromUrl !== void 0 ? _tabFromUrl : parseUpNextTab(stored)) !== null && _ref !== void 0 ? _ref : "prs";
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Funfinished%2FUpNextTabs.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61Uy27bMBC8-ysWOcUHCpJstzCDfkkQFJS4khahSYKkbNdF_r0QZT38iJNDoYsojnZnhrNMqN2bzTqFvwsASd4q8YdDpfD4sgCoheWQdm-FcRIdK0wIZschs0fwRpGEvXDPjP3OtnjK3QmXHdjs0VXKHNiRg2iDeVl8LJK-UXbZiLQijWzoJxTVmlHAnedQog7oJhrJxuGuW-5IswapbgKH9drGP62QknTdwX7kHRCyM7oyOjBPJ-RXXFfFctw_nMtdIeqIKI0y7novj1uFKN9rZ1otOWijcbLqej1al4_WBSe0t8KhDlGWcDXpEceyXlnZOt-1t4YGPw4NBWTeihK7NgcnbPc51qNARt8I0cv5GfCmO6F4Ene1pRfoV-FIMI8Ky4Dy11NwLT69Pf77sa3N8sYXdrfU7pJ2ZcrWsz15KhRGAqYNXYDmrt5WGGHMVJXHwIHlnbdj5fxuKAtlyvchbweSoeGQJfmQwilxkCarn8PnsyonJLWew3a77Y_x8xiKb_oVSQ25nxEJeAwsTs58Zu76md2m9gqQzR1fRV-sGUIlCm9UG2KqB0N6dSOtftlHecrw5NX8euDQkJSoI11FloPDMjynEJ_lg6QPI5bOyK57sn0nFozl5ytgQvy_AF3mZxNLDaLTO8OcJmNAPg-C_GYQvp7ZTa_0UmF_H338A_mou87yBQAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Funfinished_2FUpNextTabs_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61Uy27bMBC8_ysWOcUHCpJstzCDfkkQFJS4khahSYKkbNdF_r0QZT38iJNDoYsojnZnhrNMqN2bzTqFvwsASd4q8YdDpfD4sgCoheWQdm_FcRIdK0wIZschs0fwRpGEvXDPjP3OtnjK3QmXHdjs0VXKHNiRg2iDeVl8LJK_UXbZiLQijWzoJxTVmlHAnedQog7oJhrJxuGuW_5IswapbgKH9drGP62QknTdwX7kHRCyM7oyOjBPJ_RXXFfFctw_nMtdIeqIKI0y7novj1uFKN9rZ1otOWijcbLqej1al4_WBSe0t8KhDlGWcDXpEceyXlnZOt_1t4YGPw4NBWTeihK7NgcnbPc51qNARt8I0cv5GfCmO6F4Ene1pRfoV_FIMI8Ky4Dy11NwLT69Pf77sa3N8sYXdrfU7pJ2ZcrWsz15KhRGAqYNXYDmrt5WGGHMVJXHwIHlnbdj5fxuKAtlyvchbweSoeGQJfmQwilxkCarn8PnsyonJLWew3a77Y_x8xiKb_oVSQ25nxEJeAwsTs58Zu76md2m9gqQzR1fRV_sGUIlCm9UG2KqB0N6dSOtftlHecrw5NX8euDQkJSoI11FloPDMjynEJ_lg6QPI5bOyK57sn0nFozl5ytgQvy_AF3mZxNLDaLTO8OcJmNAPg_C_GYQvp7ZTa_0UmF_H338A_mou87yBQAA_22_7D = __webpack_require__(65550);
;// ./src/components/unfinished/UpNextTabs.css.ts


var UpNextTabs_css_badge = 'iuvo542';
var panel = 'iuvo544';
var UpNextTabs_css_panelHeading = 'iuvo545';
var UpNextTabs_css_tab = 'iuvo541';
var tabList = 'iuvo540';
var visuallyHidden = 'iuvo543';
;// ./src/components/unfinished/UpNextTabs.tsx
/* __next_internal_client_entry_do_not_use__ UpNextTabs auto */ 



const TAB_LABELS = {
    prs: "PRs",
    stuck: "Stuck",
    worktrees: "Worktrees",
    queue: "Queue"
};
const PANEL_HEADINGS = {
    prs: "Open pull requests",
    stuck: "Stuck backlog items",
    worktrees: "Worktrees",
    queue: "Backlog queue"
};
function tabName(tab, count, isDegraded) {
    const label = TAB_LABELS[tab];
    if (count <= 0) return label;
    return "".concat(label, ", ").concat(count).concat(isDegraded ? " or more" : "", " need attention");
}
function UpNextTabs(param) {
    let { value, onValueChange, badges = {}, degraded = {}, descriptions = {}, panels } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(dist/* Root */.bL, {
        value: value,
        onValueChange: (next)=>onValueChange(next),
        activationMode: "manual",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)(dist/* List */.B8, {
                className: tabList,
                "aria-label": "Up next sections",
                children: UP_NEXT_TABS.map((tab)=>{
                    var _badges_tab;
                    const count = (_badges_tab = badges[tab]) !== null && _badges_tab !== void 0 ? _badges_tab : 0;
                    const isDegraded = degraded[tab] === true;
                    const description = descriptions[tab];
                    const descriptionId = description ? "up-next-tab-desc-".concat(tab) : undefined;
                    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(dist/* Trigger */.l9, {
                        value: tab,
                        className: UpNextTabs_css_tab,
                        "data-testid": "up-next-tab-".concat(tab),
                        "aria-label": tabName(tab, count, isDegraded),
                        "aria-describedby": descriptionId,
                        title: description,
                        children: [
                            TAB_LABELS[tab],
                            count > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: UpNextTabs_css_badge,
                                "aria-hidden": "true",
                                "data-testid": "up-next-tab-badge-".concat(tab),
                                children: [
                                    count,
                                    isDegraded ? "+" : ""
                                ]
                            }),
                            descriptionId && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                id: descriptionId,
                                className: visuallyHidden,
                                children: description
                            })
                        ]
                    }, tab);
                })
            }),
            UP_NEXT_TABS.map((tab)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)(dist/* Content */.UC, {
                    value: tab,
                    className: panel,
                    tabIndex: 0,
                    "data-testid": "up-next-panel-".concat(tab),
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                            className: UpNextTabs_css_panelHeading,
                            tabIndex: -1,
                            children: PANEL_HEADINGS[tab]
                        }),
                        panels[tab]
                    ]
                }, tab))
        ]
    });
}

;// ./src/lib/hooks/useGitHubPRs.ts
/* __next_internal_client_entry_do_not_use__ useGitHubPRs auto */ 





const RECONNECT_DELAY_MS = 5000;
/**
 * Subscribes to WatchUserPRs server-streaming RPC.
 * Replaces the full PR list on each snapshot event.
 * Reconnects automatically on disconnect.
 */ function useGitHubPRs() {
    let { now = Date.now } = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : {};
    const [prs, setPrs] = (0,react.useState)([]);
    const [authState, setAuthState] = (0,react.useState)(undefined);
    const [accountStatuses, setAccountStatuses] = (0,react.useState)([]);
    const [lastUpdatedAt, setLastUpdatedAt] = (0,react.useState)(undefined);
    const [error, setError] = (0,react.useState)(undefined);
    const [refreshing, setRefreshing] = (0,react.useState)(false);
    const nowRef = (0,react.useRef)(now);
    nowRef.current = now;
    const abortRef = (0,react.useRef)(null);
    const reconnectTimerRef = (0,react.useRef)(null);
    const startWatch = (0,react.useCallback)(()=>{
        if (abortRef.current) abortRef.current.abort();
        const abort = new AbortController();
        abortRef.current = abort;
        const client = (0,promise_client/* createClient */.UU)(github_user_pb/* GitHubUserService */.aI, (0,api_transport/* getWatchTransport */.VZ)());
        (async ()=>{
            try {
                const req = (0,create/* create */.v)(github_user_pb/* WatchUserPRsRequestSchema */.Gs, {});
                const stream = client.watchUserPRs(req, {
                    signal: abort.signal
                });
                for await (const event of stream){
                    if (abort.signal.aborted) break;
                    if (event.authState) setAuthState(event.authState);
                    if (event.eventType === "snapshot" || event.accountStatuses.length > 0) {
                        setAccountStatuses(event.accountStatuses);
                    }
                    setPrs(event.prs);
                    setLastUpdatedAt(nowRef.current());
                    setError(undefined);
                    setRefreshing(false);
                }
            } catch (err) {
                if (abort.signal.aborted) return;
                setError(err instanceof Error ? err.message : String(err));
                setRefreshing(false);
                reconnectTimerRef.current = setTimeout(()=>{
                    if (!abort.signal.aborted) startWatch();
                }, RECONNECT_DELAY_MS);
            }
        })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
    (0,react.useEffect)(()=>{
        startWatch();
        return ()=>{
            var _abortRef_current;
            (_abortRef_current = abortRef.current) === null || _abortRef_current === void 0 ? void 0 : _abortRef_current.abort();
            if (reconnectTimerRef.current) clearTimeout(reconnectTimerRef.current);
        };
    }, [
        startWatch
    ]);
    const refresh = (0,react.useCallback)(()=>{
        setRefreshing(true);
        startWatch();
    }, [
        startWatch
    ]);
    return {
        prs,
        authState,
        accountStatuses,
        lastUpdatedAt,
        error,
        refreshing,
        refresh
    };
}

;// ./src/lib/hooks/usePRListFilters.ts
/* __next_internal_client_entry_do_not_use__ DEFAULT_FILTER,DEFAULT_SORT,applyFilter,usePRListFilters auto */ 

const DEFAULT_FILTER = "all";
const DEFAULT_SORT = "attention-first";
/** Filters and searches only; ordering is `orderPRs`' job so the list can freeze it. */ function applyFilter(prs, filter, search) {
    let result = prs;
    if (search.trim()) {
        const q = search.trim().toLowerCase();
        result = result.filter((p)=>p.title.toLowerCase().includes(q) || p.headRef.toLowerCase().includes(q) || "".concat(p.owner, "/").concat(p.repo).toLowerCase().includes(q) || String(p.number).includes(q));
    }
    switch(filter){
        case "ci-failing":
            result = result.filter((p)=>p.checkConclusion === "failure" || p.checkConclusion === "error");
            break;
        case "changes-requested":
            result = result.filter((p)=>p.changesReqCount > 0);
            break;
        case "with-session":
            result = result.filter((p)=>p.sessionIds.length > 0 || p.linkedSessions.length > 0);
            break;
        case "draft":
            result = result.filter((p)=>p.isDraft);
            break;
        case "needs-attention":
            result = result.filter((p)=>prAttention(p).needsAttention);
            break;
    }
    return result;
}
/**
 * PR filter/sort/search state, owned by the page (not the PRs panel) so it survives tab
 * switches. Deliberately component state, not URL state: lost on reload and not shareable.
 */ function usePRListFilters() {
    const [filterStatus, setFilterStatus] = (0,react.useState)(DEFAULT_FILTER);
    const [sortBy, setSortBy] = (0,react.useState)(DEFAULT_SORT);
    const [searchQuery, setSearchQuery] = (0,react.useState)("");
    const clear = (0,react.useCallback)(()=>{
        setFilterStatus(DEFAULT_FILTER);
        setSortBy(DEFAULT_SORT);
        setSearchQuery("");
    }, []);
    return (0,react.useMemo)(()=>({
            filterStatus,
            sortBy,
            searchQuery,
            setFilterStatus,
            setSortBy,
            setSearchQuery,
            clear,
            isActive: filterStatus !== DEFAULT_FILTER || sortBy !== DEFAULT_SORT || searchQuery.trim() !== "",
            apply: (prs)=>applyFilter(prs, filterStatus, searchQuery)
        }), [
        filterStatus,
        sortBy,
        searchQuery,
        clear
    ]);
}

;// ./src/lib/hooks/useUpNextTab.ts
/* __next_internal_client_entry_do_not_use__ useUpNextTab auto */ 




// useLayoutEffect warns during SSR/static export; fall back to useEffect there.
const useIsomorphicLayoutEffect =  true ? react.useLayoutEffect : 0;
/**
 * Tab state: URL (`?item=`, `?tab=`) wins and is read synchronously on first render; only the
 * localStorage fallback is read post-mount (layout effect, before paint) so SSR/hydration agree.
 * Only user clicks write storage; deep links never do.
 */ function useUpNextTab() {
    let now = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : ()=>performance.now();
    const searchParams = (0,nav/* useSearchParams */.ok)();
    const replaceSearch = (0,nav/* useReplaceSearch */.wV)();
    const urlTab = tabFromUrl({
        item: stringParam(searchParams, "item"),
        tab: stringParam(searchParams, "tab")
    });
    const [storedTab, setStoredTab] = (0,react.useState)(null);
    // A click wins over the URL until the URL itself changes: history.replaceState does not always
    // re-render useSearchParams on the static export.
    const [clickedTab, setClickedTab] = (0,react.useState)(null);
    var _ref, _ref1;
    const tab = (_ref1 = (_ref = clickedTab !== null && clickedTab !== void 0 ? clickedTab : urlTab) !== null && _ref !== void 0 ? _ref : storedTab) !== null && _ref1 !== void 0 ? _ref1 : "prs";
    const tabRef = (0,react.useRef)(tab);
    tabRef.current = tab;
    const visitStartRef = (0,react.useRef)(null);
    const clickedRef = (0,react.useRef)(false);
    useIsomorphicLayoutEffect(()=>{
        const stored = urlTab === null ? readStoredTab() : null;
        if (stored) setStoredTab(stored);
        visitStartRef.current = now();
        var _ref;
        emitTabEvent({
            type: "visit",
            landedOn: (_ref = urlTab !== null && urlTab !== void 0 ? urlTab : stored) !== null && _ref !== void 0 ? _ref : "prs"
        });
    // Mount-only: later URL changes are handled by derivation, not re-reading storage.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
    (0,react.useEffect)(()=>setClickedTab(null), [
        urlTab
    ]);
    const setTab = (0,react.useCallback)((next)=>{
        var _visitStartRef_current;
        emitTabEvent({
            type: "click",
            from: tabRef.current,
            to: next,
            msSinceVisit: now() - ((_visitStartRef_current = visitStartRef.current) !== null && _visitStartRef_current !== void 0 ? _visitStartRef_current : now()),
            firstClickOfVisit: !clickedRef.current
        });
        clickedRef.current = true;
        writeStoredTab(next);
        setStoredTab(next);
        setClickedTab(next);
        const params = new URLSearchParams(searchParams.toString());
        params.delete("item");
        params.set("tab", next);
        // Not router.replace: on the static export that hard-reloads the document, dropping focus and state.
        replaceSearch("?".concat(params.toString()));
    }, [
        searchParams,
        now,
        replaceSearch
    ]);
    return {
        tab,
        setTab
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Funfinished%2FUnfinishedTab.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA71W0W7iMBB85yv8clKRzsgJBYr7cn9SOckmcUnsyHYa4NR_P8VuIA4JlGvvhATBWc-uZ8dj_9rBIVWsBI1egtf966Y4rtDvGUKpkqV9QMgoJnQqVUmRkoYZeCAJZPPnGULvM4SMnIxbrnuR77NFl4LYGQ1PTE5RQMiPNiIHnuXm_F--gUoL2eADRaw20hvbU5TzJAHRjpZsjz_AtmtS7d2YyrigiJzmVixJuMgoemPqAeOXYAvHUB2XwfxypI1PuK4KdqAoLcBCtr844Qpiw6WgKJZFXdoCMlaNwD73lxzYJV9gsoJnAnMDpaYoBmFATQGS-amIRrXv228vR_idOTIlG4oCL8HSSUMKgzU_wsV0cNPb981HNzeEtGOxLKQaxrtsp055qR5vpGLzSdhwvH2fpyE8-r1b-7xyUXABOCpkvHs-65gsNiso-0o-j0RSJaAoCqs90rLgySCjI85FYSMrPLqwsh-lWMJrTdHK7RYmeMmcLs8bmSzWGrXFMoW4SLngBryVbb6qmCFVTxZwYquFx4utRsaWNIhZn1U1KoXISaFWuqWskryrt2M9uM46i3eZkrVIhsibaY05i7COxx3rZxhEFkGofVpo3lqXJWc639N0PjKgmaYyrjV-45pHBVhcWZu21dMiKyz8RxiWaarBWHI86O24JKZ67zmSkM6TfKPuDFjHShZFxFRn1kIKaF_gBqIdN_g0yUVaBRlZx7lfIKXdhBOiX7PD7U1hV1V5-CtVbv-fKq3MKqZAmC8I8qeb2KkToSbnBrCuWAz91tl-6lxxsRuaMvtmGbN_J-PoRpHldJGrfvcnjbif61O0iOsZe3ixRTKwN9g6cd-Dr5x4Uxec5RyR61qN_fzJuAXcd_8JfUy4cZ5H910d3IotRb1rZ11VoGKmrasUYAwoq2_LClmQR3cadzeOy9OJtB-v7vQ7uBjIPrvr1H2tteHpAcdSGBCGIrtjcQSmAbia8v6D2DGDI2mMLCcwP-lfvnmu3a38pl1OmsYfT_0P2KkMAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Funfinished_2FUnfinishedTab_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA71W0W7iMBB85yv8clKRzsgJBYr7cn9SOckmcUnsyHYa4NR_P8VuIA4JlGvvhATBWc_uZ8dj_9rBIVWsBI1egtf966Y4rtDvGUKpkqV9QMgoJnQqVUmRkoYZeCAJZPPnGULvM4SMnIxbrnuR77NFl4LYGQ1PTE5RQMiPNiIHnuXm_F_gUoL2eADRaw20hvbU5TzJAHRjpZsjz_AtmtS7d2YyrigiJzmVixJuMgoemPqAeOXYAvHUB2XwfxypI1PuK4KdqAoLcBCtr844Qpiw6WgKJZFXdoCMlaNwD73lxzYJV9gsoJnAnMDpaYoBmFATQGS_amIRrXv228vR_idOTIlG4oCL8HSSUMKgzU_wsV0cNPb981HNzeEtGOxLKQaxrtsp055qR5vpGLzSdhwvH2fpyE8_r1b_7xyUXABOCpkvHs_65gsNiso_0o_j0RSJaAoCqs90rLgySCjI85FYSMrPLqwsh_lWMJrTdHK7RYmeMmcLs8bmSzWGrXFMoW4SLngBryVbb6qmCFVTxZwYquFx4utRsaWNIhZn1U1KoXISaFWuqWskryrt2M9uM46i3eZkrVIhsibaY05i7COxx3rZxhEFkGofVpo3lqXJWc639N0PjKgmaYyrjV_45pHBVhcWZu21dMiKyz8RxiWaarBWHI86O24JKZ67zmSkM6TfKPuDFjHShZFxFRn1kIKaF_gBqIdN_g0yUVaBRlZx7lfIKXdhBOiX7PD7U1hV1V5_CtVbv_fKq3MKqZAmC8I8qeb2KkToSbnBrCuWAz91tl_6lxxsRuaMvtmGbN_J_PoRpHldJGrfvcnjbif61O0iOsZe3ixRTKwN9g6cd_Dr5x4Uxec5RyR61qN_fzJuAXcd_8JfUy4cZ5H910d3IotRb1rZ11VoGKmrasUYAwoq2_LClmQR3cadzeOy9OJtB_v7vQ7uBjIPrvr1H2tteHpAcdSGBCGIrtjcQSmAbia8v6D2DGDI2mMLCcwP_lfvnmu3a38pl1OmsYfT_0P2KkMAAA_22_7D = __webpack_require__(87678);
;// ./src/app/unfinished/UnfinishedTab.css.ts


var UnfinishedTab_css_btn = '_1jxj7lz8';
var UnfinishedTab_css_chip = '_1jxj7lza';
var UnfinishedTab_css_chipActive = '_1jxj7lzb';
var container = '_1jxj7lz0';
var UnfinishedTab_css_empty = '_1jxj7lzc';
var UnfinishedTab_css_filterRow = '_1jxj7lz9';
var UnfinishedTab_css_group = '_1jxj7lzf';
var UnfinishedTab_css_groupHeading = '_1jxj7lze';
var notice = '_1jxj7lzg';
var repoList = '_1jxj7lzd';
var scanInfo = '_1jxj7lz4';
var UnfinishedTab_css_spinner = '_1jxj7lz6';
var UnfinishedTab_css_title = '_1jxj7lz3';
var toolbar = '_1jxj7lz1';
var toolbarLeft = '_1jxj7lz2';
var toolbarRight = '_1jxj7lz7';
;// ./src/app/unfinished/UnfinishedTab.tsx
/* __next_internal_client_entry_do_not_use__ UnfinishedTab auto */ 
// +feature: up-next-tabs






















const PR_TAB_DESCRIPTION = "PRs with failing CI, conflicts, unresolved threads or requested changes. The sidebar badge uses its own count.";
/**
 * Main Unfinished Work tab component.
 * Groups worktrees by repo, supports filter chips, and handles dismiss/snooze.
 */ function UnfinishedTab() {
    const { worktrees, lastScanTime, isScanning, triggerScan } = (0,useUnfinishedWork/* useUnfinishedWork */.c)();
    const [filter, setFilter] = (0,react.useState)("all");
    const [secondsAgo, setSecondsAgo] = (0,react.useState)(0);
    const searchParams = (0,nav/* useSearchParams */.ok)();
    const replaceSearch = (0,nav/* useReplaceSearch */.wV)();
    const { tab, setTab } = useUpNextTab();
    // Lifted here so the tab badge and the PRs panel share one WatchUserPRs stream.
    const gitHubPRs = useGitHubPRs();
    const prFilters = usePRListFilters();
    // Shared poll via StuckBacklogItemsProvider; no second poller for the badge/notice.
    const { items: stuckItems, lastFetched: stuckLastFetched } = (0,useStuckBacklogItems/* useStuckBacklogItems */.d)();
    // routes.unfinishedItem(itemId) deep link — pre-expands/filters the
    // matching Stuck Backlog Items card (see StuckItemsSection's focusItemId).
    const urlItemId = stringParam(searchParams, "item");
    // history.replaceState does not always re-render useSearchParams on the static export, so
    // "Show all stuck items" records the item it dismissed instead of waiting for the URL to update.
    const [dismissedItemId, setDismissedItemId] = (0,react.useState)();
    const focusItemId = urlItemId === dismissedItemId ? undefined : urlItemId;
    (0,react.useEffect)(()=>{
        if (urlItemId !== dismissedItemId) setDismissedItemId(undefined);
    }, [
        urlItemId,
        dismissedItemId
    ]);
    const transport = (0,react.useMemo)(()=>(0,connect_transport/* createConnectTransport */.w)({
            baseUrl: (0,config/* getApiBaseUrl */.h)(),
            interceptors: [
                (0,config/* createAuthInterceptor */._)()
            ]
        }), []);
    const client = (0,react.useMemo)(()=>(0,promise_client/* createClient */.UU)(unfinished_pb/* UnfinishedWorkService */.aC, transport), [
        transport
    ]);
    // Update "last scanned N seconds ago" counter every second
    (0,react.useEffect)(()=>{
        if (!lastScanTime) return;
        const tick = ()=>{
            setSecondsAgo(Math.floor((Date.now() - lastScanTime.getTime()) / 1000));
        };
        tick();
        const id = setInterval(tick, 1000);
        return ()=>clearInterval(id);
    }, [
        lastScanTime
    ]);
    const handleDismiss = (0,react.useCallback)(async (repoPath, branch)=>{
        try {
            const req = (0,create/* create */.v)(unfinished_pb/* DismissWorktreeRequestSchema */.gp, {
                repoPath,
                branch
            });
            await client.dismissWorktree(req);
        } catch (e) {
        // ignore
        }
    }, // eslint-disable-next-line react-hooks/exhaustive-deps
    []);
    const handleSnooze = (0,react.useCallback)(async (repoPath, branch)=>{
        try {
            const req = (0,create/* create */.v)(unfinished_pb/* SnoozeWorktreeRequestSchema */.eq, {
                repoPath,
                branch
            });
            await client.snoozeWorktree(req);
        } catch (e) {
        // ignore
        }
    }, // eslint-disable-next-line react-hooks/exhaustive-deps
    []);
    // Apply client-side filter
    const filtered = worktrees.filter((wt)=>{
        if (filter === "uncommitted") return wt.hasUncommitted;
        if (filter === "ahead") return wt.commitsAhead > 0;
        if (filter === "behind") return wt.commitsBehind > 0;
        return true;
    });
    // Group by repoName
    const groups = new Map();
    for (const wt of filtered){
        const name = wt.repoName || wt.repoPath;
        if (!groups.has(name)) groups.set(name, []);
        groups.get(name).push(wt);
    }
    const chips = [
        {
            label: "All",
            value: "all"
        },
        {
            label: "Uncommitted",
            value: "uncommitted"
        },
        {
            label: "Ahead",
            value: "ahead"
        },
        {
            label: "Behind",
            value: "behind"
        }
    ];
    const attention = summarizeAttention(gitHubPRs.prs);
    const stuckLoaded = stuckLastFetched !== null;
    const itemMissing = focusItemId !== undefined && stuckLoaded && !stuckItems.some((i)=>i.itemId === focusItemId);
    const showAllStuck = (0,react.useCallback)(()=>{
        var _document_querySelector;
        const params = new URLSearchParams(searchParams.toString());
        params.delete("item");
        params.set("tab", "stuck");
        // Not router.replace: that hard-reloads the static export, which would drop the focus moved below.
        replaceSearch("?".concat(params.toString()));
        setDismissedItemId(urlItemId);
        (_document_querySelector = document.querySelector('[data-testid="up-next-panel-stuck"] h2')) === null || _document_querySelector === void 0 ? void 0 : _document_querySelector.focus();
    }, [
        searchParams,
        urlItemId,
        replaceSearch
    ]);
    const prsDescription = attention.degraded ? "At least ".concat(attention.count, " PRs need attention. ").concat(DEGRADED_ATTENTION_TEXT, " ").concat(PR_TAB_DESCRIPTION) : PR_TAB_DESCRIPTION;
    const panels = {
        prs: /*#__PURE__*/ (0,jsx_runtime.jsx)(GitHubPRsSection, {
            ...gitHubPRs,
            filters: prFilters,
            attention: attention
        }),
        stuck: /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
            children: [
                itemMissing && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: notice,
                    role: "status",
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                            children: [
                                "Item ",
                                focusItemId,
                                " was not found. It may have been completed or removed."
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: UnfinishedTab_css_btn,
                            onClick: showAllStuck,
                            children: "Show all stuck items"
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)(StuckItemsSection, {
                    focusItemId: focusItemId
                })
            ]
        }),
        worktrees: /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: UnfinishedTab_css_filterRow,
                    role: "group",
                    "aria-label": "Filter worktrees",
                    children: chips.map((param)=>{
                        let { label, value } = param;
                        return /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: "".concat(UnfinishedTab_css_chip, " ").concat(filter === value ? UnfinishedTab_css_chipActive : ""),
                            onClick: ()=>setFilter(value),
                            "aria-pressed": filter === value,
                            children: label
                        }, value);
                    })
                }),
                groups.size === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: UnfinishedTab_css_empty,
                    children: worktrees.length === 0 ? "No unfinished work found. All repos are clean." : "No items match the current filter."
                }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: repoList,
                    children: Array.from(groups.entries()).map((param)=>{
                        let [repoName, wts] = param;
                        return /*#__PURE__*/ (0,jsx_runtime.jsx)(UnfinishedRepoGroup, {
                            repoName: repoName,
                            worktrees: wts,
                            onDismiss: handleDismiss,
                            onSnooze: handleSnooze
                        }, repoName);
                    })
                })
            ]
        }),
        queue: /*#__PURE__*/ (0,jsx_runtime.jsx)(BacklogQueueSection, {})
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: toolbar,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: toolbarLeft,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("h1", {
                                className: UnfinishedTab_css_title,
                                children: "Up Next"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: scanInfo,
                                children: isScanning ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: UnfinishedTab_css_spinner,
                                            "aria-label": "Scanning"
                                        }),
                                        "Scanning…"
                                    ]
                                }) : lastScanTime ? "Last scanned ".concat(secondsAgo, "s ago") : "Not yet scanned"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: toolbarRight,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(nav/* Link */.N_, {
                                href: routes/* routes */.J.settingsUnfinished,
                                className: UnfinishedTab_css_btn,
                                "aria-label": "Configure scan sources",
                                children: "Sources"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: UnfinishedTab_css_btn,
                                onClick: triggerScan,
                                disabled: isScanning,
                                "aria-label": "Refresh — trigger an immediate scan",
                                children: "Refresh"
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(UpNextTabs, {
                value: tab,
                onValueChange: setTab,
                badges: {
                    prs: attention.count,
                    stuck: stuckLoaded ? stuckItems.length : 0
                },
                degraded: {
                    prs: attention.degraded
                },
                descriptions: {
                    prs: prsDescription
                },
                panels: panels
            })
        ]
    });
}


/***/ }),

/***/ 82476:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   c: () => (/* binding */ useUnfinishedWork)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(66144);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(21694);
/* harmony import */ var _gen_session_v1_unfinished_pb__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(43860);
/* harmony import */ var _bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(95444);
/* __next_internal_client_entry_do_not_use__ useUnfinishedWork auto */ 





/**
 * Hook that subscribes to WatchUnfinishedWork streaming RPC.
 * Maintains a local Map<key, UnfinishedWorktree> and exposes a sorted array.
 * Reconnects automatically on disconnect.
 */ function useUnfinishedWork() {
    const [worktreeMap, setWorktreeMap] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(new Map());
    const [lastScanTime, setLastScanTime] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(null);
    const [isScanning, setIsScanning] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(false);
    const abortRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    const reconnectTimerRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    const client = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__/* .createClient */ .UU)(_gen_session_v1_unfinished_pb__WEBPACK_IMPORTED_MODULE_2__/* .UnfinishedWorkService */ .aC, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_1__/* .getWatchTransport */ .VZ)());
    const worktreeKey = (wt)=>"".concat(wt.repoPath, "|").concat(wt.branch);
    const startWatch = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>{
        if (abortRef.current) {
            abortRef.current.abort();
        }
        const abort = new AbortController();
        abortRef.current = abort;
        (async ()=>{
            try {
                const req = (0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_unfinished_pb__WEBPACK_IMPORTED_MODULE_2__/* .WatchUnfinishedWorkRequestSchema */ .AQ, {});
                const stream = client.watchUnfinishedWork(req, {
                    signal: abort.signal
                });
                for await (const event of stream){
                    if (abort.signal.aborted) break;
                    if (event.payload.case === "worktreeUpdated") {
                        const wt = event.payload.value;
                        const key = worktreeKey(wt);
                        setWorktreeMap((prev)=>{
                            const next = new Map(prev);
                            next.set(key, wt);
                            return next;
                        });
                    } else if (event.payload.case === "worktreeRemoved") {
                        const wt = event.payload.value;
                        const key = worktreeKey(wt);
                        setWorktreeMap((prev)=>{
                            const next = new Map(prev);
                            next.delete(key);
                            return next;
                        });
                    } else if (event.payload.case === "scanCompleted") {
                        const completedAt = event.payload.value.completedAt;
                        if (completedAt) {
                            const ts = completedAt;
                            setLastScanTime(new Date(Number(ts.seconds) * 1000));
                        }
                        setIsScanning(false);
                    }
                }
            } catch (err) {
                if (abort.signal.aborted) return;
                // Reconnect after 3s on error
                reconnectTimerRef.current = setTimeout(()=>{
                    if (!abort.signal.aborted) {
                        startWatch();
                    }
                }, 3000);
            }
        })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        startWatch();
        return ()=>{
            if (abortRef.current) abortRef.current.abort();
            if (reconnectTimerRef.current) clearTimeout(reconnectTimerRef.current);
        };
    }, [
        startWatch
    ]);
    const triggerScan = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async ()=>{
        setIsScanning(true);
        try {
            const req = (0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_unfinished_pb__WEBPACK_IMPORTED_MODULE_2__/* .ScanUnfinishedWorkRequestSchema */ .I_, {});
            // One-shot mutation triggered by an explicit user action ("scan now"),
            // not tied to a mount/effect.
            // abort-signal-exempt
            await client.scanUnfinishedWork(req);
        } catch (e) {
            setIsScanning(false);
        }
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
    // Sort worktrees by lastModified descending
    const worktrees = Array.from(worktreeMap.values()).sort((a, b)=>{
        const ta = a.lastModified ? Number(a.lastModified.seconds) : 0;
        const tb = b.lastModified ? Number(b.lastModified.seconds) : 0;
        return tb - ta;
    });
    return {
        worktrees,
        lastScanTime,
        isScanning,
        triggerScan
    };
}


/***/ }),

/***/ 82514:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   E$: () => (/* binding */ useNotifications),
/* harmony export */   IB: () => (/* binding */ useNotificationState),
/* harmony export */   RK: () => (/* binding */ useNotificationCommands),
/* harmony export */   _J: () => (/* binding */ NotificationStateContext),
/* harmony export */   jo: () => (/* binding */ NotificationCommandsContext)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* __next_internal_client_entry_do_not_use__ NotificationStateContext,NotificationCommandsContext,useNotificationState,useNotificationCommands,useNotifications auto */ 
const NotificationStateContext = (0,react__WEBPACK_IMPORTED_MODULE_0__.createContext)(null);
const NotificationCommandsContext = (0,react__WEBPACK_IMPORTED_MODULE_0__.createContext)(null);
const noop = ()=>{};
const OUTSIDE_PROVIDER_STATE = {
    notifications: [],
    notificationHistory: [],
    isPanelOpen: false,
    historyLoading: false,
    historyHasMore: false,
    historyError: null,
    historyLastUpdatedAt: null,
    unreadCount: 0,
    movedToTray: null,
    answeredElsewhere: {},
    quietMode: false
};
/**
 * Outside a NotificationProvider these return no-ops, so components render in
 * tests or embedded views without a full provider tree.
 */ const OUTSIDE_PROVIDER_COMMANDS = {
    addNotification: noop,
    addToHistoryOnly: noop,
    removeNotification: noop,
    removeToastByApprovalId: noop,
    acknowledgeNotification: noop,
    broadcastActionResolved: noop,
    markAnswerHandled: noop,
    markBackgroundReviewed: noop,
    clearAll: noop,
    dismissToast: noop,
    moveAllToTray: ()=>[],
    undoMoveToTray: noop,
    showSessionNotification: noop,
    togglePanel: noop,
    markAsRead: noop,
    showActionToast: ()=>""
};
/** Toast and history state, without the commands. */ function useNotificationState() {
    var _useContext;
    return (_useContext = (0,react__WEBPACK_IMPORTED_MODULE_0__.useContext)(NotificationStateContext)) !== null && _useContext !== void 0 ? _useContext : OUTSIDE_PROVIDER_STATE;
}
/** Stable commands only; consumers using just these never re-render on notification state. */ function useNotificationCommands() {
    var _useContext;
    return (_useContext = (0,react__WEBPACK_IMPORTED_MODULE_0__.useContext)(NotificationCommandsContext)) !== null && _useContext !== void 0 ? _useContext : OUTSIDE_PROVIDER_COMMANDS;
}
/** Both halves merged: the original `useNotifications()` surface. */ function useNotifications() {
    const state = useNotificationState();
    const commands = useNotificationCommands();
    return (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>({
            ...state,
            ...commands,
            getUnreadCount: ()=>state.unreadCount
        }), [
        state,
        commands
    ]);
}


/***/ }),

/***/ 85529:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  n: () => (/* binding */ GitHubIssuePicker)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/hooks/useBacklogService.ts
var useBacklogService = __webpack_require__(8570);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-redux@9.2.0+5436edeee5f9f735/node_modules/react-redux/dist/react-redux.mjs
var react_redux = __webpack_require__(55324);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/reselect@5.1.1/node_modules/reselect/dist/reselect.mjs
var reselect = __webpack_require__(13009);
// EXTERNAL MODULE: ./src/lib/store/sessionsSlice.ts
var sessionsSlice = __webpack_require__(60795);
;// ./src/lib/utils/issuePickerCache.ts
// TTL cache for the GitHub issue picker — keyed under the page's origin so
// multiple instances (dev / prod) never share stale data.
const TTL_MS = 5 * 60 * 1000; // 5 minutes
function cacheKey(suffix) {
    if (false) {}
    return "ssq:".concat(window.location.origin, ":").concat(suffix);
}
function readEntry(key) {
    try {
        const raw = localStorage.getItem(key);
        if (!raw) return null;
        const entry = JSON.parse(raw);
        if (Date.now() > entry.expiry) {
            localStorage.removeItem(key);
            return null;
        }
        return entry.data;
    } catch (e) {
        return null;
    }
}
function writeEntry(key, data) {
    try {
        const entry = {
            data,
            expiry: Date.now() + TTL_MS
        };
        localStorage.setItem(key, JSON.stringify(entry));
    } catch (e) {
    // localStorage full or unavailable — silently skip
    }
}
// ─── Repo list cache ────────────────────────────────────────────────────────
const REPOS_KEY = ()=>cacheKey("github:repos");
function getCachedRepos() {
    return readEntry(REPOS_KEY());
}
function setCachedRepos(repos) {
    writeEntry(REPOS_KEY(), repos);
}
// ─── Issue list cache ───────────────────────────────────────────────────────
function issuesKey(owner, repo, state) {
    return cacheKey("github:issues:".concat(owner, "/").concat(repo, ":").concat(state));
}
function getCachedIssues(owner, repo, state) {
    return readEntry(issuesKey(owner, repo, state));
}
function setCachedIssues(owner, repo, state, issues) {
    writeEntry(issuesKey(owner, repo, state), issues);
}
// ─── Recent repos (multi-entry history, up to 5) ───────────────────────────
const RECENT_REPOS_KEY = ()=>cacheKey("github:recentRepos");
const RECENT_REPOS_MAX = 5;
function getRecentRepos() {
    if (false) {}
    try {
        const raw = localStorage.getItem(RECENT_REPOS_KEY());
        if (!raw) return [];
        return JSON.parse(raw);
    } catch (e) {
        return [];
    }
}
function addRecentRepo(owner, repo) {
    if (false) {}
    try {
        const existing = getRecentRepos();
        const key = "".concat(owner, "/").concat(repo);
        const filtered = existing.filter((r)=>"".concat(r.owner, "/").concat(r.repo) !== key);
        const updated = [
            {
                owner,
                repo
            },
            ...filtered
        ].slice(0, RECENT_REPOS_MAX);
        localStorage.setItem(RECENT_REPOS_KEY(), JSON.stringify(updated));
    } catch (e) {
    // ignore
    }
}

;// ./src/lib/hooks/useGitHubIssuePicker.ts
/* __next_internal_client_entry_do_not_use__ useGitHubIssuePicker auto */ 





// ─── Hook ────────────────────────────────────────────────────────────────────
const ISSUE_DEBOUNCE_MS = 150;
// Derives local-path repos from the session list. Memoized with createSelector
// (module-level, not inline in the hook — an inline selector would rebuild the
// memoization cache every render, defeating the point): selectAllSessions's
// result only changes reference when the sessions slice actually changes, so
// this only recomputes then, returning the SAME array reference on every other
// render instead of a fresh one. An inline `useSelector((state) => { ... build
// a new array ... })` — the previous shape — returns a new array reference on
// every invocation regardless of whether the input changed, which is the
// classic react-redux anti-pattern that produces the "The result of
// getSnapshot should be cached" class of bug: useSyncExternalStore treats each
// new reference as a genuine store change and can loop re-rendering trying to
// reach a stable snapshot, intermittently manifesting as a crash under CPU
// contention (jsdom + React 19's stricter consistency checks) without ever
// reproducing reliably in isolation — see useGitHubIssuePicker.multiselect.test.ts.
const selectLocalRepos = (0,reselect/* createSelector */.Mz)([
    sessionsSlice/* selectAllSessions */.rt
], (sessions)=>{
    const seen = new Set();
    const results = [];
    for (const s of sessions){
        if (!s.repoRoot) continue;
        const parts = s.repoRoot.split("/");
        var _parts_;
        const repo = (_parts_ = parts[parts.length - 1]) !== null && _parts_ !== void 0 ? _parts_ : "";
        var _parts_1;
        const owner = (_parts_1 = parts[parts.length - 2]) !== null && _parts_1 !== void 0 ? _parts_1 : "";
        const key = "".concat(owner, "/").concat(repo);
        if (seen.has(key)) continue;
        seen.add(key);
        var _s_title;
        results.push({
            owner,
            repo,
            isLocal: true,
            localPath: s.repoRoot,
            description: (_s_title = s.title) !== null && _s_title !== void 0 ? _s_title : "",
            host: ""
        });
    }
    return results;
});
function useGitHubIssuePicker(param) {
    let { searchGitHubRepos, listGitHubIssues, onSelect } = param;
    const [phase, setPhase] = (0,react.useState)("repo");
    const [repoQuery, setRepoQuery] = (0,react.useState)("");
    const [repos, setRepos] = (0,react.useState)([]);
    const [reposLoading, setReposLoading] = (0,react.useState)(false);
    const [selectedRepo, setSelectedRepo] = (0,react.useState)(null);
    const [issueSearch, setIssueSearch] = (0,react.useState)("");
    const [issueState, setIssueState] = (0,react.useState)("open");
    const [issues, setIssues] = (0,react.useState)([]);
    const [issuesLoading, setIssuesLoading] = (0,react.useState)(false);
    const [authError, setAuthError] = (0,react.useState)(false);
    // In-memory cache of ALL fetched repos — filtered client-side as user types.
    const allReposRef = (0,react.useRef)([]);
    const issueGenRef = (0,react.useRef)(0);
    // Pull local-path repos from Redux session list for the "local repos" tier.
    const localRepos = (0,react_redux/* useSelector */.d4)(selectLocalRepos);
    // ─── Repo phase: one-time eager fetch, then filter client-side ─────────────
    // Step 1: fetch full repo list once on mount (or from localStorage cache).
    (0,react.useEffect)(()=>{
        // Seed from localStorage cache immediately so the list appears before the
        // network response arrives.
        const cached = getCachedRepos();
        if (cached && cached.length > 0) {
            const mapped = cached.map((r)=>{
                var _r_host;
                return {
                    ...r,
                    isLocal: false,
                    localPath: "",
                    host: (_r_host = r.host) !== null && _r_host !== void 0 ? _r_host : ""
                };
            });
            allReposRef.current = mapped;
            setRepos(mapped);
        }
        let cancelled = false;
        setReposLoading(true);
        searchGitHubRepos("", 100).then((results)=>{
            if (cancelled) return;
            allReposRef.current = results;
            setRepos(results);
            setCachedRepos(results.map((r)=>({
                    owner: r.owner,
                    repo: r.repo,
                    description: r.description,
                    host: r.host
                })));
            setAuthError(false);
        }).catch((err)=>{
            if (cancelled) return;
            if (err instanceof useBacklogService/* GitHubAuthError */.Kz) setAuthError(true);
        }).finally(()=>{
            if (!cancelled) setReposLoading(false);
        });
        return ()=>{
            cancelled = true;
        };
    // Run once on mount — searchGitHubRepos identity is stable from the service hook.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
    // ─── Issue phase: fetch on search/state/repo change ────────────────────────
    (0,react.useEffect)(()=>{
        if (phase !== "issue" || !selectedRepo) return;
        const gen = ++issueGenRef.current;
        const { owner, repo, host } = selectedRepo;
        // Check cache (only for no-search case).
        if (issueSearch === "") {
            const cached = getCachedIssues(owner, repo, issueState);
            if (cached) {
                if (gen === issueGenRef.current) {
                    setIssues(cached.map((c)=>{
                        var _c_isPR, _c_host;
                        return {
                            ...c,
                            isPR: (_c_isPR = c.isPR) !== null && _c_isPR !== void 0 ? _c_isPR : false,
                            host: (_c_host = c.host) !== null && _c_host !== void 0 ? _c_host : ""
                        };
                    }));
                }
                return;
            }
        }
        let timer = null;
        timer = setTimeout(async ()=>{
            if (gen !== issueGenRef.current) return;
            setIssuesLoading(true);
            try {
                const results = await listGitHubIssues(owner, repo, {
                    state: issueState,
                    search: issueSearch,
                    limit: 50,
                    host
                });
                if (gen !== issueGenRef.current) return;
                setIssues(results);
                if (issueSearch === "") {
                    setCachedIssues(owner, repo, issueState, results);
                }
                setAuthError(false);
            } catch (err) {
                if (gen !== issueGenRef.current) return;
                if (err instanceof useBacklogService/* GitHubAuthError */.Kz) {
                    setAuthError(true);
                }
            } finally{
                if (gen === issueGenRef.current) setIssuesLoading(false);
            }
        }, ISSUE_DEBOUNCE_MS);
        return ()=>{
            if (timer !== null) clearTimeout(timer);
        };
    }, [
        phase,
        selectedRepo,
        issueSearch,
        issueState,
        listGitHubIssues
    ]);
    // ─── Actions ────────────────────────────────────────────────────────────────
    const selectRepo = (0,react.useCallback)((repo)=>{
        setSelectedRepo(repo);
        addRecentRepo(repo.owner, repo.repo);
        setIssues([]);
        setIssueSearch("");
        setPhase("issue");
    }, []);
    const selectIssues = (0,react.useCallback)((issues)=>{
        if (!selectedRepo || issues.length === 0) return;
        onSelect(selectedRepo.owner, selectedRepo.repo, issues);
    }, [
        selectedRepo,
        onSelect
    ]);
    const goBack = (0,react.useCallback)(()=>{
        setPhase("repo");
        setSelectedRepo(null);
        setIssues([]);
        setIssueSearch("");
    }, []);
    const reset = (0,react.useCallback)(()=>{
        setPhase("repo");
        setRepoQuery("");
        setRepos(allReposRef.current);
        setSelectedRepo(null);
        setIssues([]);
        setIssueSearch("");
        setIssueState("open");
        setAuthError(false);
        issueGenRef.current = 0;
    }, []);
    // Build the full merged + filtered + history-tiered repo list.
    // Returns [repos, historyCount] — historyCount is the number of leading history entries.
    const mergedRepos = (0,react.useCallback)((fetched)=>{
        // Enrich fetched repos with local path info from session list.
        const localMap = new Map(localRepos.map((r)=>[
                "".concat(r.owner, "/").concat(r.repo),
                r.localPath
            ]));
        const enrichedFetched = fetched.map((r)=>{
            const localPath = localMap.get("".concat(r.owner, "/").concat(r.repo));
            return localPath ? {
                ...r,
                isLocal: true,
                localPath
            } : r;
        });
        // Local-only: session repos not returned by the GitHub API.
        const fetchedKeys = new Set(fetched.map((r)=>"".concat(r.owner, "/").concat(r.repo)));
        const localOnly = localRepos.filter((r)=>!fetchedKeys.has("".concat(r.owner, "/").concat(r.repo)));
        // Full candidate list.
        const all = [
            ...enrichedFetched,
            ...localOnly
        ];
        // Apply query filter.
        const q = repoQuery.trim().toLowerCase();
        const filtered = q === "" ? all : all.filter((r)=>{
            var _r_description;
            return "".concat(r.owner, "/").concat(r.repo).toLowerCase().includes(q) || ((_r_description = r.description) === null || _r_description === void 0 ? void 0 : _r_description.toLowerCase().includes(q));
        });
        // History tier: recent repos (in recency order) that appear in filtered results.
        const recentList = getRecentRepos();
        const historyRepos = recentList.map((recent)=>filtered.find((f)=>f.owner === recent.owner && f.repo === recent.repo)).filter((r)=>r !== undefined);
        const historyKeySet = new Set(historyRepos.map((r)=>"".concat(r.owner, "/").concat(r.repo)));
        const otherRepos = filtered.filter((r)=>!historyKeySet.has("".concat(r.owner, "/").concat(r.repo)));
        return [
            [
                ...historyRepos,
                ...otherRepos
            ],
            historyRepos.length
        ];
    }, [
        localRepos,
        repoQuery
    ]);
    const [computedRepos, repoHistoryCount] = mergedRepos(repos);
    return {
        phase,
        repoQuery,
        repos: computedRepos,
        repoHistoryCount,
        reposLoading,
        selectedRepo,
        setRepoQuery,
        selectRepo,
        issueSearch,
        issueState,
        issues,
        issuesLoading,
        setIssueSearch,
        setIssueState,
        selectIssues,
        goBack,
        authError,
        reset
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FGitHubIssuePicker.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA8VZ3W6jOhC-71P4XBxpq5UrSLZJ6-o8yWp1ZGCSeGNsakzzc7TvfoQJ4IBNIMl2ryJlhvH8fjNjP2kQ4p0H6L8HhBKWZ5weCFpx2L89IPOLE6Yg1kwKgmLJi1SUlDXNCPqg6gvG_4avcJyp4zx4fHv49fBUSQzdEilna4GZhjQnKAahQXnEzY62uNnN4g6PJSWjScLE2kF1HY9QROPtWslCJARpRUWeUQVCl6RYcqm6gsLqK6kSUASF2R7lkrOkwwUWF1Y0YUXe8-bS8Kyk0DhnR-jRI0OPC5WXWmSSVca3HiMb-QHK-M224lzMy6PXlLNwzs_9zwRnAvAfCcM5w_KTHW7oO2DrjSboOQjGue-bcd-OJXpDUBgEfw854dh1gqmsISeE1B_GMBrloe3dPCQLXSYHQUIKqITuS35j6umASO5t75CVjIu8StWKwWkKt136PBWyUrrHm1PgZt-CzLCXJbLicocPBNFCy_sm03DaNqYs7oKV0xOqBx4j8MaX6qgNrdYyHfZeaznhNNc43jCe2OGvhVQpZLGPxbT2k-9UMYpz4BBrSP7RqoAfUwQsDbOr7Ms4ERSisMmcOpsI2rAkAZN2GvYatwTgnGU5y0vSbsM04DyjsamWnaKZdfBLe7AzIEM1_3gvbWorAxQ0VpaFdIKy5xLJGo1fr9d41kLLRxkwobEoUlAsJkjTqOBUlX_kLo2a86mzSUVcxtuz8ihT09mWhmt60YW_8AQiruS4quVOmR6sKLx0wOwu6Re5QcmPPk64asTF48VdGhE-cyBo9K_g6XPHx7aBTkpLd9V5B0W42bJzhtSf3--fZ16nKhdlVTYmr4ZMnldBMIVjctpuvpfAy9s6m7PXg2f7Jj1QSiocrfu-rSiltneb4HzFY46ybdl0Ab9GxBFQew0-zkaj9Lj693YSNiJId06Qmq4PHAhimnIWWxr9vDizoL9YmkmlaYkvzXdb853MaMz0gaDg6dkulwRWtOBn_J4R6wy9Gm5u-OqpOqyC71ey7lxqzYQLaZqxCucbxcSWILtq0162nQ7sjyhde80g0KhpyRT96W7Zb-BjoDvtxVBIlVJuHSavWaJ_FrlmqwOOpdAgtE2yXPHqmEROhnDQGpTp8yaZg6dgBun9BqJ-uDPFUqoO_To-EVq0cgyXTNRjzewEJqbQTAhWUqUEFVkGKqa5vRlkv8-1TZY5fPu7PDgRC6finMPNjSffbx7iHQtDWfCYw0q3W_aYWb9RSk1d9q9ZRtW0ZbS6r-hb65-09FW3rDMb5Oxt04qhDZPF1MH9hli7No7hteLDaDcw_Po3bUef3nliMADe3elzwLor7kcaQAhccNDqbXXZy7eHe4-Vl5QxwJlALBWtMqu22WqPywla7y2tGxF2Nz3c4Rmhu_51VZoHjo0ExZTHX_qMXw26oa_9L0YAb2PWccCsMr0Jqi9LfPa0zyLVS0s9AgVX119VLGe1linAtSLNvWalX6SAbvFOquStcxMaLrw3oa3S0x5zem3V6Icj0DsAb9jPnnvC2W1dqBU0n7bdXrxqv_N2e8Wjw-L06OBtM-GcJCynEYdkcPgXstxfuNxB8vbw63_kBboIChwAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FGitHubIssuePicker_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA8VZ3W6jOhC_71P4XBxpq5UrSLZJ6_o8yWp1ZGCSeGNsakzzc7TvfoQJ4IBNIMl2ryJlhvH8fjNjP2kQ4p0H6L8HhBKWZ5weCFpx2L89IPOLE6Yg1kwKgmLJi1SUlDXNCPqg6gvG_4avcJyp4zx4fHv49fBUSQzdEilna4GZhjQnKAahQXnEzY62uNnN4g6PJSWjScLE2kF1HY9QROPtWslCJARpRUWeUQVCl6RYcqm6gsLqK6kSUASF2R7lkrOkwwUWF1Y0YUXe8_bS8Kyk0DhnR_jRI0OPC5WXWmSSVca3HiMb_QHK_M224lzMy6PXlLNwzs_9zwRnAvAfCcM5w_KTHW7oO2DrjSboOQjGue_bcd_OJXpDUBgEfw854dh1gqmsISeE1B_GMBrloe3dPCQLXSYHQUIKqITuS35j6umASO5t75CVjIu8StWKwWkKt136PBWyUrrHm1PgZt_CzLCXJbLicocPBNFCy_sm03DaNqYs7oKV0xOqBx4j8MaX6qgNrdYyHfZeaznhNNc43jCe2OGvhVQpZLGPxbT2k_9UMYpz4BBrSP7RqoAfUwQsDbOr7Ms4ERSisMmcOpsI2rAkAZN2GvYatwTgnGU5y0vSbsM04DyjsamWnaKZdfBLe7AzIEM1_3gvbWorAxQ0VpaFdIKy5xLJGo1fr9d41kLLRxkwobEoUlAsJkjTqOBUlX_kLo2a86mzSUVcxtuz8ihT09mWhmt60YW_8AQiruS4quVOmR6sKLx0wOwu6Re5QcmPPk64asTF48VdGhE_cyBo9K_g6XPHx7aBTkpLd9V5B0W42bJzhtSf3_fZ16nKhdlVTYmr4ZMnldBMIVjctpuvpfAy9s6m7PXg2f7Jj1QSiocrfu_rSiltneb4HzFY46ybdl0Ab9GxBFQew0_zkaj9Lj693YSNiJId06Qmq4PHAhimnIWWxr9vDizoL9YmkmlaYkvzXdb853MaMz0gaDg6dkulwRWtOBn_J4R6wy9Gm5u_OqpOqyC71ey7lxqzYQLaZqxCucbxcSWILtq0162nQ7sjyhde80g0KhpyRT96W7Zb_BjoDvtxVBIlVJuHSavWaJ_FrlmqwOOpdAgtE2yXPHqmEROhnDQGpTp8yaZg6dgBun9BqJ_uDPFUqoO_To_EVq0cgyXTNRjzewEJqbQTAhWUqUEFVkGKqa5vRlkv8_1TZY5fPu7PDgRC6finMPNjSffbx7iHQtDWfCYw0q3W_aYWb9RSk1d9q9ZRtW0ZbS6r_hb65_09FW3rDMb5Oxt04qhDZPF1MH9hli7No7hteLDaDcw_Po3bUef3nliMADe3elzwLor7kcaQAhccNDqbXXZy7eHe4_Vl5QxwJlALBWtMqu22WqPywla7y2tGxF2Nz3c4Rmhu_51VZoHjo0ExZTHX_qMXw26oa_9L0YAb2PWccCsMr0Jqi9LfPa0zyLVS0s9AgVX119VLGe1linAtSLNvWalX6SAbvFOquStcxMaLrw3oa3S0x5zem3V6Icj0DsAb9jPnnvC2W1dqBU0n7bdXrxqv_N2e8Wjw_L06OBtM_GcJCynEYdkcPgXstxfuNxB8vbw63_kBboIChwAAA_22_7D = __webpack_require__(10301);
;// ./src/components/backlog/GitHubIssuePicker.css.ts


var authErrorBox = 'tennqlg';
var backButton = 'tennql2';
var container = 'tennql0';
var emptyState = 'tennqlf';
var expandToggle = 'tennqlw';
var filterBar = 'tennqlb';
var historyDivider = 'tennqll';
var historyIcon = 'tennqlm';
var importBar = 'tennql11';
var importBarHint = 'tennql12';
var importButton = 'tennql13';
var issueAuthor = 'tennqlv';
var issueCheckbox = 'tennqls';
var issueExpandedBody = 'tennql10';
var issueExpandedLabels = 'tennqlz';
var issueExpandedPanel = 'tennqly';
var issueMainCol = 'tennqlt';
var issueNumber = 'tennql9';
var issueRowWrapper = 'tennqlr';
var issueSubMeta = 'tennqlu';
var issueTypeBadge = 'tennqlp';
var labelBadge = 'tennqla';
var listContainer = 'tennql5';
var listItem = 'tennql6';
var listItemDisabled = 'tennqlk';
var listItemMeta = 'tennql8';
var listItemName = 'tennql7';
var listItemSelected = 'tennqlj';
var loadingText = 'tennqli';
var localBadge = 'tennqlh';
var matchHighlight = 'tennqln';
var openLink = 'tennqlx';
var phaseHeader = 'tennql1';
var prTypeBadge = 'tennqlo';
var relativeDate = 'tennqlq';
var repoChip = 'tennql3';
var searchInput = 'tennql4';
var stateButton = {inactive:'tennqld',active:'tennqle'};
var stateToggle = 'tennqlc';
;// ./src/components/backlog/GitHubIssuePicker.tsx
// +feature: backlog:github-issue-picker
/* __next_internal_client_entry_do_not_use__ GitHubIssuePicker auto */ 




// ─── Helpers ─────────────────────────────────────────────────────────────────
function highlightMatch(text, query) {
    const q = query.trim().toLowerCase();
    if (!q) return text;
    const idx = text.toLowerCase().indexOf(q);
    if (idx === -1) return text;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            text.slice(0, idx),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("mark", {
                className: matchHighlight,
                children: text.slice(idx, idx + q.length)
            }),
            text.slice(idx + q.length)
        ]
    });
}
function relativeTime(iso) {
    if (!iso) return "";
    const diff = Date.now() - new Date(iso).getTime();
    const s = Math.floor(diff / 1000);
    if (s < 60) return "".concat(s, "s ago");
    const m = Math.floor(s / 60);
    if (m < 60) return "".concat(m, "m ago");
    const h = Math.floor(m / 60);
    if (h < 24) return "".concat(h, "h ago");
    const d = Math.floor(h / 24);
    if (d < 30) return "".concat(d, "d ago");
    const mo = Math.floor(d / 30);
    if (mo < 12) return "".concat(mo, "mo ago");
    return "".concat(Math.floor(mo / 12), "y ago");
}
// ─── Component ────────────────────────────────────────────────────────────────
function GitHubIssuePicker(param) {
    let { onSelect, onCancel, importing, importProgress } = param;
    const { searchGitHubRepos, listGitHubIssues } = (0,useBacklogService/* useBacklogService */.gG)();
    const picker = useGitHubIssuePicker({
        searchGitHubRepos,
        listGitHubIssues,
        onSelect
    });
    const searchRef = (0,react.useRef)(null);
    // Focus input when phase changes.
    (0,react.useEffect)(()=>{
        var _searchRef_current;
        (_searchRef_current = searchRef.current) === null || _searchRef_current === void 0 ? void 0 : _searchRef_current.focus();
    }, [
        picker.phase
    ]);
    // Two-level Escape: issue → repo → onCancel.
    const handleKeyDown = (0,react.useCallback)((e)=>{
        if (e.key !== "Escape") return;
        e.preventDefault();
        if (picker.phase === "issue") {
            picker.goBack();
        } else {
            onCancel();
        }
    }, [
        picker,
        onCancel
    ]);
    if (picker.authError) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: container,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: authErrorBox,
                    children: [
                        "No GitHub token configured. Set ",
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                            children: "GITHUB_TOKEN"
                        }),
                        " or ",
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                            children: "GH_TOKEN"
                        }),
                        " to enable the GitHub issue picker."
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    style: {
                        display: "flex",
                        justifyContent: "flex-end"
                    },
                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        onClick: onCancel,
                        className: backButton,
                        children: "Close"
                    })
                })
            ]
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: container,
        onKeyDown: handleKeyDown,
        children: [
            picker.phase === "repo" ? /*#__PURE__*/ (0,jsx_runtime.jsx)(RepoPhase, {
                picker: picker,
                searchRef: searchRef
            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)(IssuePhase, {
                picker: picker,
                searchRef: searchRef,
                importing: importing,
                importProgress: importProgress
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                style: {
                    display: "flex",
                    justifyContent: "flex-end"
                },
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    type: "button",
                    onClick: onCancel,
                    className: backButton,
                    disabled: importing,
                    children: "Cancel"
                })
            })
        ]
    });
}
// ─── Repo phase ───────────────────────────────────────────────────────────────
function RepoPhase(param) {
    let { picker, searchRef } = param;
    const [selectedIndex, setSelectedIndex] = (0,react.useState)(-1);
    const listRef = (0,react.useRef)(null);
    const repos = picker.repos;
    // Reset selection when the repo list changes (query update or initial load).
    (0,react.useEffect)(()=>{
        setSelectedIndex(-1);
    }, [
        repos
    ]);
    // Scroll keyboard-selected item into view.
    (0,react.useEffect)(()=>{
        var _items_selectedIndex;
        if (selectedIndex < 0 || !listRef.current) return;
        const items = listRef.current.querySelectorAll('[role="option"]');
        (_items_selectedIndex = items[selectedIndex]) === null || _items_selectedIndex === void 0 ? void 0 : _items_selectedIndex.scrollIntoView({
            block: "nearest"
        });
    }, [
        selectedIndex
    ]);
    const handleKeyDown = (e)=>{
        if (repos.length === 0) return;
        if (e.key === "ArrowDown") {
            e.preventDefault();
            setSelectedIndex((i)=>Math.min(i + 1, repos.length - 1));
        } else if (e.key === "ArrowUp") {
            e.preventDefault();
            setSelectedIndex((i)=>Math.max(i - 1, 0));
        } else if (e.key === "Enter") {
            e.preventDefault();
            const idx = selectedIndex >= 0 ? selectedIndex : 0;
            const repo = repos[idx];
            if (repo) picker.selectRepo(repo);
        }
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: phaseHeader,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    style: {
                        fontSize: "13px",
                        fontWeight: 600
                    },
                    children: "Select a repository"
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                ref: searchRef,
                className: searchInput,
                type: "text",
                placeholder: "Search repos…",
                value: picker.repoQuery,
                onChange: (e)=>picker.setRepoQuery(e.target.value),
                onKeyDown: handleKeyDown,
                "aria-label": "Search GitHub repositories",
                "aria-autocomplete": "list",
                "aria-controls": "repo-list",
                autoComplete: "off",
                autoFocus: true
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                id: "repo-list",
                ref: listRef,
                role: "listbox",
                "aria-label": "GitHub repositories",
                className: listContainer,
                children: picker.reposLoading ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: loadingText,
                    children: "Loading…"
                }) : repos.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: emptyState,
                    children: picker.repoQuery ? "No repos found." : "No repos available."
                }) : repos.map((repo, idx)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)(react.Fragment, {
                        children: [
                            idx === picker.repoHistoryCount && picker.repoHistoryCount > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: historyDivider,
                                role: "separator"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                role: "option",
                                "aria-selected": idx === selectedIndex,
                                className: idx === selectedIndex ? "".concat(listItem, " ").concat(listItemSelected) : listItem,
                                onMouseDown: (e)=>{
                                    e.preventDefault();
                                    picker.selectRepo(repo);
                                },
                                onMouseEnter: ()=>setSelectedIndex(idx),
                                children: [
                                    idx < picker.repoHistoryCount && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: historyIcon,
                                        children: "\uD83D\uDD52"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: listItemName,
                                        children: highlightMatch("".concat(repo.owner, "/").concat(repo.repo), picker.repoQuery)
                                    }),
                                    repo.isLocal && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: localBadge,
                                        children: "local"
                                    }),
                                    repo.host && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: localBadge,
                                        children: repo.host
                                    }),
                                    repo.description && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: listItemMeta,
                                        children: highlightMatch(repo.description, picker.repoQuery)
                                    })
                                ]
                            })
                        ]
                    }, "".concat(repo.owner, "/").concat(repo.repo)))
            })
        ]
    });
}
// ─── Issue phase ──────────────────────────────────────────────────────────────
function IssuePhase(param) {
    let { picker, searchRef, importing, importProgress } = param;
    var _picker_selectedRepo, _picker_selectedRepo1, _picker_selectedRepo2;
    const states = [
        "open",
        "closed",
        "all"
    ];
    const [selected, setSelected] = (0,react.useState)(new Set());
    const [expanded, setExpanded] = (0,react.useState)(new Set());
    // Selections/expansions don't carry across a repo or filter change.
    (0,react.useEffect)(()=>{
        setSelected(new Set());
        setExpanded(new Set());
    }, [
        picker.selectedRepo,
        picker.issueState,
        picker.issueSearch
    ]);
    const toggleSelected = (0,react.useCallback)((number)=>{
        setSelected((prev)=>{
            const next = new Set(prev);
            if (next.has(number)) next.delete(number);
            else next.add(number);
            return next;
        });
    }, []);
    const toggleExpanded = (0,react.useCallback)((number)=>{
        setExpanded((prev)=>{
            const next = new Set(prev);
            if (next.has(number)) next.delete(number);
            else next.add(number);
            return next;
        });
    }, []);
    const selectedIssues = picker.issues.filter((i)=>selected.has(i.number));
    const handleImport = (0,react.useCallback)(()=>{
        if (selectedIssues.length > 0) picker.selectIssues(selectedIssues);
    }, [
        picker,
        selectedIssues
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: phaseHeader,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: backButton,
                        onClick: picker.goBack,
                        "aria-label": "Back to repository selection",
                        children: "← Back"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: repoChip,
                        "aria-label": "Selected repository",
                        children: [
                            (_picker_selectedRepo = picker.selectedRepo) === null || _picker_selectedRepo === void 0 ? void 0 : _picker_selectedRepo.owner,
                            "/",
                            (_picker_selectedRepo1 = picker.selectedRepo) === null || _picker_selectedRepo1 === void 0 ? void 0 : _picker_selectedRepo1.repo,
                            ((_picker_selectedRepo2 = picker.selectedRepo) === null || _picker_selectedRepo2 === void 0 ? void 0 : _picker_selectedRepo2.host) ? " (".concat(picker.selectedRepo.host, ")") : ""
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: filterBar,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        ref: searchRef,
                        className: searchInput,
                        type: "search",
                        placeholder: "Search issues…",
                        value: picker.issueSearch,
                        onChange: (e)=>picker.setIssueSearch(e.target.value),
                        "aria-label": "Search issues",
                        autoComplete: "off",
                        style: {
                            flex: 1
                        },
                        autoFocus: true
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: stateToggle,
                        role: "group",
                        "aria-label": "Issue state filter",
                        children: states.map((s)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: picker.issueState === s ? stateButton.active : stateButton.inactive,
                                onMouseDown: (e)=>{
                                    e.preventDefault();
                                    picker.setIssueState(s);
                                },
                                children: s
                            }, s))
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                role: "listbox",
                "aria-label": "GitHub issues",
                "aria-multiselectable": "true",
                className: listContainer,
                children: picker.issuesLoading ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: loadingText,
                    children: "Loading…"
                }) : picker.issues.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: emptyState,
                    children: picker.issueSearch ? "No issues match your search." : "No issues found."
                }) : picker.issues.map((issue)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(IssueRow, {
                        issue: issue,
                        selected: selected.has(issue.number),
                        expanded: expanded.has(issue.number),
                        onToggleSelected: ()=>toggleSelected(issue.number),
                        onToggleExpanded: ()=>toggleExpanded(issue.number)
                    }, issue.number))
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: importBar,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: importBarHint,
                        children: importing && importProgress ? "Importing ".concat(importProgress.done, " of ").concat(importProgress.total, "…") : selectedIssues.length > 0 ? "".concat(selectedIssues.length, " selected") : "Check issues to import, then Import."
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: importButton,
                        disabled: selectedIssues.length === 0 || importing,
                        onClick: handleImport,
                        children: importing ? "Importing…" : "Import".concat(selectedIssues.length > 0 ? " (".concat(selectedIssues.length, ")") : "")
                    })
                ]
            })
        ]
    });
}
// ─── Issue row ────────────────────────────────────────────────────────────────
function IssueRow(param) {
    let { issue, selected, expanded, onToggleSelected, onToggleExpanded } = param;
    var _issue_body;
    const age = relativeTime(issue.updatedAt || issue.createdAt);
    const bodyPreview = (_issue_body = issue.body) === null || _issue_body === void 0 ? void 0 : _issue_body.trim();
    // ImportGitHubIssue only accepts real issue URLs (ParseGitHubRef categorizes
    // /pull/N URLs as RefTypePR, not RefTypeIssue) — a PR can never be imported
    // as a backlog item, so don't offer a selection that's guaranteed to fail.
    const importable = !issue.isPR;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: issueRowWrapper,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                role: "option",
                "aria-selected": selected,
                "aria-disabled": !importable,
                className: (selected ? "".concat(listItem, " ").concat(listItemSelected) : listItem) + (importable ? "" : " ".concat(listItemDisabled)),
                title: importable ? undefined : "Pull requests can't be imported as backlog items — open it on GitHub instead.",
                onClick: importable ? onToggleSelected : undefined,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        type: "checkbox",
                        checked: selected,
                        disabled: !importable,
                        onChange: onToggleSelected,
                        onClick: (e)=>e.stopPropagation(),
                        "aria-label": importable ? "Select issue #".concat(issue.number, ": ").concat(issue.title) : "Issue #".concat(issue.number, " is a pull request and can't be imported: ").concat(issue.title),
                        className: issueCheckbox
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: issue.isPR ? prTypeBadge : issueTypeBadge,
                        children: issue.isPR ? "PR" : "#"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: issueNumber,
                        children: issue.number
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: issueMainCol,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: listItemName,
                                children: issue.title
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: issueSubMeta,
                                children: [
                                    issue.author && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                        className: issueAuthor,
                                        children: [
                                            "@",
                                            issue.author
                                        ]
                                    }),
                                    age && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        children: age
                                    }),
                                    issue.labels.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                        children: [
                                            issue.labels.length,
                                            " label",
                                            issue.labels.length > 1 ? "s" : ""
                                        ]
                                    })
                                ]
                            })
                        ]
                    }),
                    bodyPreview && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: expandToggle,
                        "aria-label": expanded ? "Collapse details" : "Expand details",
                        "aria-expanded": expanded,
                        onClick: (e)=>{
                            e.stopPropagation();
                            onToggleExpanded();
                        },
                        children: expanded ? "▾" : "▸"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                        href: issue.url,
                        target: "_blank",
                        rel: "noopener noreferrer",
                        className: openLink,
                        "aria-label": "Open issue #".concat(issue.number, " on GitHub"),
                        title: "Open on GitHub",
                        onClick: (e)=>e.stopPropagation(),
                        children: "\uD83D\uDC41"
                    })
                ]
            }),
            expanded && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: issueExpandedPanel,
                children: [
                    issue.labels.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: issueExpandedLabels,
                        children: issue.labels.map((label)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: labelBadge,
                                title: label,
                                children: label
                            }, label))
                    }),
                    bodyPreview && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: issueExpandedBody,
                        children: bodyPreview
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
/******/ __webpack_require__.O(0, [1869,852,4202,9353,8182,2265,5047,8165,3575,8890,3793,3467,9691,3516,7358], () => (__webpack_exec__(36796)));
/******/ var __webpack_exports__ = __webpack_require__.O();
/******/ _N_E = __webpack_exports__;
/******/ }
]);
//# sourceMappingURL=page-ee3b321ecc160b88.js.map