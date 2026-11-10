"use strict";
exports.id = 108;
exports.ids = [108];
exports.modules = {

/***/ 17518:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   u: () => (/* binding */ useSessionVcs)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(88782);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(4435);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_7__ = __webpack_require__(22800);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_8__ = __webpack_require__(70792);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(31381);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_6__ = __webpack_require__(73396);
/* harmony import */ var _lib_store__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(92360);
/* harmony import */ var _lib_store_sessionsSlice__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(45985);
/* harmony import */ var _lib_hooks_useAbortableRequest__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(10937);
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
 */ function useSessionVcs(sessionId, baseUrl, isActive = true) {
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
            clientRef.current = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_5__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_6__/* .getConnectTransport */ .qW)());
        }
        return clientRef.current;
    }, []);
    // Cancel the in-flight status/diff request on the next call or on unmount —
    // this hook is instantiated per-SessionDetail, so switching sessions rapidly
    // without cancellation left every prior request's promise (and its closure
    // over sessionId/setState) alive until it resolved or hit its deadline,
    // growing heap under fast session switching (measured: ~44MB -> ~154MB
    // used JS heap over 138 switches in a profiling run).
    const startStatus = (0,_lib_hooks_useAbortableRequest__WEBPACK_IMPORTED_MODULE_4__/* .useAbortableRequest */ .X)();
    const startDiff = (0,_lib_hooks_useAbortableRequest__WEBPACK_IMPORTED_MODULE_4__/* .useAbortableRequest */ .X)();
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
                setStatus(response.vcsStatus ?? null);
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
    const sessionStatus = (0,_lib_store__WEBPACK_IMPORTED_MODULE_2__/* .useAppSelector */ .GV)((state)=>(0,_lib_store_sessionsSlice__WEBPACK_IMPORTED_MODULE_3__/* .selectAllSessions */ .rt)(state).find((s)=>s.id === sessionId)?.status);
    const sessionUpdatedAt = (0,_lib_store__WEBPACK_IMPORTED_MODULE_2__/* .useAppSelector */ .GV)((state)=>{
        const s = (0,_lib_store_sessionsSlice__WEBPACK_IMPORTED_MODULE_3__/* .selectAllSessions */ .rt)(state).find((s)=>s.id === sessionId);
        return s?.updatedAt?.seconds ?? null;
    });
    // VCS status: re-fetch on mount and whenever the session changes in Redux.
    // Event-driven is the primary trigger; the fallback interval catches git
    // commits that happen mid-session without changing the session's status
    // field. isActive controls the fallback's cadence, not whether it fires —
    // a backgrounded pane still needs a backstop, just not one tuned for a
    // user actively watching it.
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
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
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
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

/***/ 37894:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Q: () => (/* binding */ useVcsStatus),
/* harmony export */   U: () => (/* binding */ prefetchVcsStatus)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(88782);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(4435);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(22800);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__ = __webpack_require__(70792);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(31381);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(73396);
/* harmony import */ var _lib_hooks_useAbortableRequest__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(10937);
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
        const client = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_4__/* .getConnectTransport */ .qW)());
        // One-shot cache warm with no component lifecycle to cancel against;
        // the TTL cache above (not a signal) is what stops this from
        // compounding on rapid session switches.
        // abort-signal-exempt
        const response = await client.getVCSStatus({
            id: sessionId
        });
        vcsCache.set(sessionId, {
            data: response.vcsStatus ?? null,
            error: response.error || null,
            timestamp: Date.now()
        });
    } catch  {
    // Prefetch failures are silent – the hook will retry on mount.
    }
}
/**
 * Shared hook for VCS status with module-level caching and visibility-aware polling.
 * Multiple components using this hook for the same sessionId share one cache entry
 * and avoid redundant network requests.
 */ function useVcsStatus(sessionId, baseUrl, pollIntervalMs = 15000) {
    const hit = getCached(sessionId);
    const [data, setData] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(hit?.data ?? null);
    const [loading, setLoading] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(!hit);
    const [error, setError] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(hit?.error ?? null);
    // Set once GetVCSStatus reports the session no longer exists, so the
    // polling interval below stops calling a deleted session forever.
    const stoppedRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(false);
    // Cancel the in-flight request on the next call or on unmount — see
    // useSessionVcs.ts (the sibling hook this pattern was first fixed in) for
    // the measured impact of skipping this under rapid session switching.
    const startFetch = (0,_lib_hooks_useAbortableRequest__WEBPACK_IMPORTED_MODULE_2__/* .useAbortableRequest */ .X)();
    const fetchVcs = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (skipCache = false)=>{
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
            const client = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_4__/* .getConnectTransport */ .qW)());
            const response = await client.getVCSStatus({
                id: sessionId
            }, {
                signal
            });
            if (signal.aborted) return;
            const entry = {
                data: response.vcsStatus ?? null,
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
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
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

/***/ 66509:
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

/***/ 97108:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Nd: () => (/* binding */ getPrimaryCardAction),
/* harmony export */   tE: () => (/* binding */ getAvailableActions)
/* harmony export */ });
/* unused harmony export KNOWN_STATUS_MEMBERSHIP */
/* harmony import */ var _lib_backlog_planReviewStatus__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(66509);
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


/***/ })

};
;