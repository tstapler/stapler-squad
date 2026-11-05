(() => {
var exports = {};
exports.id = 1764;
exports.ids = [1764];
exports.modules = {

/***/ 201:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 11645));


/***/ }),

/***/ 1708:
/***/ ((module) => {

"use strict";
module.exports = require("node:process");

/***/ }),

/***/ 2452:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   l: () => (/* binding */ useSessionVcsContext),
/* harmony export */   m: () => (/* binding */ SessionVcsProvider)
/* harmony export */ });
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(79147);
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(88782);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_1__);
/* harmony import */ var _lib_hooks_useSessionVcs__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(17518);
/* __next_internal_client_entry_do_not_use__ SessionVcsProvider,useSessionVcsContext auto */ 


const SessionVcsContext = /*#__PURE__*/ (0,react__WEBPACK_IMPORTED_MODULE_1__.createContext)(null);
/**
 * Provides a single shared VCS state (status + diff) for a SessionDetail and
 * all of its tabs (VcsPanel, FilesTab, DiffViewer). Without this provider all
 * three tabs fetched independently with no shared cache.
 *
 * Place this around the tab content in SessionDetail, not at the app root —
 * the data is scoped to a single session and should start/stop with it.
 */ function SessionVcsProvider({ sessionId, baseUrl, isActive, children }) {
    const value = (0,_lib_hooks_useSessionVcs__WEBPACK_IMPORTED_MODULE_2__/* .useSessionVcs */ .u)(sessionId, baseUrl, isActive);
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(SessionVcsContext.Provider, {
        value: value,
        children: children
    });
}
function useSessionVcsContext() {
    const ctx = (0,react__WEBPACK_IMPORTED_MODULE_1__.useContext)(SessionVcsContext);
    if (!ctx) throw new Error("useSessionVcsContext must be used within SessionVcsProvider");
    return ctx;
}


/***/ }),

/***/ 3295:
/***/ ((module) => {

"use strict";
module.exports = require("next/dist/server/app-render/after-task-async-storage.external.js");

/***/ }),

/***/ 4196:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "._1lykch20 {\n  display: flex;\n  flex-direction: column;\n  height: 100%;\n  background: var(--_19ez2rz1d);\n  color: var(--_19ez2rz1e);\n  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', 'source-code-pro', monospace;\n}\n._1lykch21 {\n  display: flex;\n  align-items: center;\n  justify-content: space-between;\n  padding: 0.75rem 1rem;\n  background: var(--_19ez2rz1g);\n  border-bottom: 1px solid var(--_19ez2rz1f);\n  flex-shrink: 0;\n}\n:root[data-left-handed] ._1lykch21 {\n  flex-direction: row-reverse;\n}\n._1lykch22 {\n  display: flex;\n  align-items: center;\n  gap: 1rem;\n  font-size: 0.875rem;\n}\n._1lykch23 {\n  color: var(--_19ez2rz1e);\n}\n._1lykch24 {\n  color: var(--_19ez2rzu);\n  font-weight: 500;\n}\n._1lykch25 {\n  color: var(--_19ez2rz10);\n  font-weight: 500;\n}\n._1lykch26 {\n  display: flex;\n  gap: 0.25rem;\n  background: var(--_19ez2rz1k);\n  border-radius: 4px;\n  padding: 0.25rem;\n}\n._1lykch27 {\n  padding: 0.4rem 0.75rem;\n  background: transparent;\n  border: none;\n  border-radius: 3px;\n  color: var(--_19ez2rz1e);\n  font-size: 0.875rem;\n  cursor: pointer;\n  transition: background 0.2s, color 0.2s;\n}\n._1lykch27:hover {\n  background: var(--_19ez2rz1k);\n}\n._1lykch28 {\n  background: var(--_19ez2rzm);\n  color: var(--_19ez2rzq);\n}\n._1lykch29 {\n  display: flex;\n  flex: 1 1;\n  min-height: 0;\n}\n._1lykch2a {\n  flex-shrink: 0;\n  width: 260px;\n  overflow-y: auto;\n  border-right: 1px solid var(--_19ez2rz1f);\n  background: var(--_19ez2rz1g);\n}\n._1lykch2b {\n  display: flex;\n  align-items: center;\n  justify-content: space-between;\n  gap: 0.5rem;\n  width: 100%;\n  padding: 0.5rem 0.75rem;\n  background: transparent;\n  border: none;\n  border-left: 3px solid transparent;\n  color: var(--_19ez2rz1e);\n  font-size: 0.8rem;\n  text-align: left;\n  cursor: pointer;\n}\n._1lykch2b:hover {\n  background: var(--_19ez2rz1k);\n}\n._1lykch2c {\n  background: var(--_19ez2rz1k);\n  border-left-color: var(--_19ez2rzm);\n}\n._1lykch2d {\n  overflow: hidden;\n  text-overflow: ellipsis;\n  white-space: nowrap;\n  word-break: break-all;\n}\n._1lykch2e {\n  display: flex;\n  gap: 0.4rem;\n  flex-shrink: 0;\n  font-size: 0.75rem;\n}\n._1lykch2f {\n  flex: 1 1;\n  overflow-y: auto;\n  padding: 1rem;\n}\n._1lykch2f::-webkit-scrollbar {\n  width: 12px;\n  height: 12px;\n}\n._1lykch2f::-webkit-scrollbar-track {\n  background: var(--_19ez2rz1d);\n}\n._1lykch2f::-webkit-scrollbar-thumb {\n  background: var(--_19ez2rz1k);\n  border-radius: 6px;\n  border: 2px solid var(--_19ez2rz1d);\n}\n._1lykch2f::-webkit-scrollbar-thumb:hover {\n  background: var(--_19ez2rz1f);\n}\n._1lykch2g {\n  margin-bottom: 1.5rem;\n  border: 1px solid var(--_19ez2rz1f);\n  border-radius: 6px;\n  overflow: hidden;\n}\n._1lykch2h {\n  display: flex;\n  align-items: center;\n  justify-content: space-between;\n  padding: 0.75rem 1rem;\n  background: var(--_19ez2rz1g);\n  border-bottom: 1px solid var(--_19ez2rz1f);\n}\n._1lykch2i {\n  font-weight: 600;\n  color: var(--_19ez2rz1e);\n}\n._1lykch2j {\n  display: flex;\n  gap: 0.75rem;\n  font-size: 0.875rem;\n}\n._1lykch2k {\n  border-top: 1px solid var(--_19ez2rz1f);\n}\n._1lykch2l {\n  padding: 0.5rem 1rem;\n  background: var(--_19ez2rz1k);\n  color: var(--_19ez2rz1j);\n  font-size: 0.875rem;\n  font-family: inherit;\n}\n._1lykch2m {\n  background: var(--_19ez2rz1d);\n}\n._1lykch2n {\n  display: flex;\n  align-items: center;\n  font-size: 13px;\n  line-height: 1.6;\n  border-left: 3px solid transparent;\n}\n._1lykch2o {\n  background: var(--_19ez2rzv);\n  border-left-color: var(--_19ez2rzu);\n}\n._1lykch2p {\n  background: var(--_19ez2rz11);\n  border-left-color: var(--_19ez2rz10);\n}\n._1lykch2q {\n  background: var(--_19ez2rz1d);\n}\n._1lykch2r {\n  display: inline-block;\n  width: 50px;\n  padding: 0 0.75rem;\n  text-align: right;\n  color: var(--_19ez2rz1j);\n  -webkit-user-select: none;\n     -moz-user-select: none;\n          user-select: none;\n  flex-shrink: 0;\n}\n._1lykch2s {\n  padding: 0 1rem;\n  flex: 1 1;\n  white-space: pre;\n  overflow-x: auto;\n}\n._1lykch2s::-webkit-scrollbar {\n  width: 12px;\n  height: 12px;\n}\n._1lykch2s::-webkit-scrollbar-track {\n  background: var(--_19ez2rz1d);\n}\n._1lykch2s::-webkit-scrollbar-thumb {\n  background: var(--_19ez2rz1k);\n  border-radius: 6px;\n  border: 2px solid var(--_19ez2rz1d);\n}\n._1lykch2s::-webkit-scrollbar-thumb:hover {\n  background: var(--_19ez2rz1f);\n}\n._1lykch2t {\n  display: flex;\n  flex-direction: column;\n  align-items: center;\n  justify-content: center;\n  height: 100%;\n  padding: 2rem;\n  text-align: center;\n}\n._1lykch2u {\n  display: flex;\n  flex-direction: column;\n  align-items: center;\n  justify-content: center;\n  height: 100%;\n  padding: 2rem;\n  text-align: center;\n  color: var(--_19ez2rz1j);\n}\n._1lykch2v {\n  margin-top: 0.5rem;\n  font-size: 0.875rem;\n  color: var(--_19ez2rz1j);\n}\n._1lykch2w {\n  display: flex;\n  flex-direction: column;\n  align-items: center;\n  justify-content: center;\n  height: 100%;\n  padding: 2rem;\n  text-align: center;\n  color: var(--_19ez2rz10);\n}\n._1lykch2x {\n  margin-top: 1rem;\n  padding: 0.4rem 1rem;\n  background: transparent;\n  border: 1px solid var(--_19ez2rz10);\n  border-radius: 4px;\n  color: var(--_19ez2rz10);\n  font-size: 0.875rem;\n  cursor: pointer;\n  transition: background 0.2s;\n}\n._1lykch2x:hover {\n  background: var(--_19ez2rz1k);\n}\n@media (max-width: 768px) {\n  ._1lykch21 {\n    flex-direction: column;\n    align-items: stretch;\n    gap: 0.75rem;\n    padding: 0.5rem 0.75rem;\n  }\n  ._1lykch22 {\n    justify-content: center;\n  }\n  ._1lykch26 {\n    justify-content: center;\n  }\n  ._1lykch29 {\n    flex-direction: column;\n  }\n  ._1lykch2a {\n    width: 100%;\n    max-height: 160px;\n    border-right: none;\n    border-bottom: 1px solid var(--_19ez2rz1f);\n  }\n  ._1lykch2f {\n    padding: 0.75rem;\n  }\n  ._1lykch2h {\n    flex-direction: column;\n    align-items: flex-start;\n    gap: 0.5rem;\n  }\n  ._1lykch2r {\n    width: 40px;\n    padding: 0 0.5rem;\n  }\n  ._1lykch2s {\n    padding: 0 0.5rem;\n    font-size: 12px;\n  }\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 8086:
/***/ ((module) => {

"use strict";
module.exports = require("module");

/***/ }),

/***/ 10846:
/***/ ((module) => {

"use strict";
module.exports = require("next/dist/compiled/next-server/app-page.runtime.prod.js");

/***/ }),

/***/ 11645:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
__webpack_require__.r(__webpack_exports__);
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   "default": () => (__WEBPACK_DEFAULT_EXPORT__)
/* harmony export */ });
/* harmony import */ var react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(70679);
/* harmony import */ var react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__);

/* harmony default export */ const __WEBPACK_DEFAULT_EXPORT__ = ((0,react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__.registerClientReference)(
function() { throw new Error("Attempted to call the default export of \"/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/fanout/page.tsx\" from the server, but it's on the client. It's not possible to invoke a client function from the server, it can only be rendered as a Component or passed to props of a Client Component."); },
"/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/fanout/page.tsx",
"default",
));


/***/ }),

/***/ 17518:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
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

/***/ 19121:
/***/ ((module) => {

"use strict";
module.exports = require("next/dist/server/app-render/action-async-storage.external.js");

/***/ }),

/***/ 21820:
/***/ ((module) => {

"use strict";
module.exports = require("os");

/***/ }),

/***/ 22053:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "._1bdf1i10 {\n  display: flex;\n  flex-direction: column;\n  gap: 12px;\n  padding: 16px;\n  height: 100%;\n  min-height: 0;\n}\n._1bdf1i11 {\n  display: flex;\n  align-items: center;\n  gap: 12px;\n  justify-content: space-between;\n}\n._1bdf1i12 {\n  display: grid;\n  grid-auto-flow: column;\n  grid-auto-columns: minmax(320px, 1fr);\n  grid-gap: 12px;\n  gap: 12px;\n  flex: 1 1;\n  min-height: 0;\n  overflow-x: auto;\n}\n._1bdf1i13 {\n  display: flex;\n  flex-direction: column;\n  gap: 8px;\n  min-height: 0;\n  border: 1px solid var(--_19ez2rze);\n  border-radius: 8px;\n  padding: 8px;\n}\n._1bdf1i14 {\n  display: flex;\n  align-items: center;\n  justify-content: space-between;\n  gap: 8px;\n}\n._1bdf1i15 {\n  flex: 1 1;\n  min-height: 0;\n  overflow: auto;\n}\n._1bdf1i16 {\n  font-size: 12px;\n  padding: 2px 8px;\n  border-radius: 999px;\n  border: 1px solid var(--_19ez2rze);\n}\n._1bdf1i17 {\n  color: var(--_19ez2rz10);\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 27910:
/***/ ((module) => {

"use strict";
module.exports = require("stream");

/***/ }),

/***/ 28354:
/***/ ((module) => {

"use strict";
module.exports = require("util");

/***/ }),

/***/ 29021:
/***/ ((module) => {

"use strict";
module.exports = require("fs");

/***/ }),

/***/ 29294:
/***/ ((module) => {

"use strict";
module.exports = require("next/dist/server/app-render/work-async-storage.external.js");

/***/ }),

/***/ 33873:
/***/ ((module) => {

"use strict";
module.exports = require("path");

/***/ }),

/***/ 42057:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 81053));


/***/ }),

/***/ 48813:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "._1s1d76t0 {\n  display: inline-flex;\n  align-items: center;\n  gap: 4px;\n  padding: 4px 10px;\n  border-radius: 12px;\n  font-size: 12px;\n  font-weight: 600;\n  text-decoration: none;\n  transition: all 0.2s ease;\n  cursor: pointer;\n  border: 1px solid transparent;\n}\n._1s1d76t0:hover {\n  transform: translateY(-1px);\n  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);\n}\n._1s1d76t0:active {\n  transform: translateY(0);\n}\n._1s1d76t0:focus {\n  outline: 2px solid var(--_19ez2rzm);\n  outline-offset: 2px;\n}\n._1s1d76t1 {\n  padding: 4px 8px;\n  font-size: 11px;\n}\n._1s1d76t2 {\n  background: var(--_19ez2rzm);\n  color: var(--_19ez2rzq);\n  border-color: var(--_19ez2rzp);\n}\n._1s1d76t2:hover {\n  background: var(--_19ez2rzp);\n  box-shadow: 0 2px 6px rgba(130, 80, 223, 0.3);\n}\n._1s1d76t3 {\n  background: var(--_19ez2rzc);\n  color: var(--_19ez2rz0);\n  border-color: var(--_19ez2rze);\n}\n._1s1d76t3:hover {\n  background: var(--_19ez2rz8);\n  border-color: var(--_19ez2rzi);\n  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.1);\n}\n._1s1d76t4 {\n  background: var(--_19ez2rz10);\n  color: var(--_19ez2rzq);\n  border-color: var(--_19ez2rz10);\n}\n._1s1d76t4:hover {\n  opacity: 0.9;\n  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.2);\n}\n._1s1d76t5 {\n  background: var(--_19ez2rzu);\n  color: var(--_19ez2rzq);\n  border-color: var(--_19ez2rzu);\n}\n._1s1d76t5:hover {\n  opacity: 0.9;\n  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.2);\n}\n._1s1d76t6 {\n  background: var(--_19ez2rzx);\n  color: var(--_19ez2rz0);\n  border-color: var(--_19ez2rzx);\n}\n._1s1d76t6:hover {\n  opacity: 0.9;\n  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.2);\n}\n._1s1d76t7 {\n  background: var(--_19ez2rz2);\n  color: var(--_19ez2rzq);\n  border-color: var(--_19ez2rz2);\n}\n._1s1d76t7:hover {\n  opacity: 0.9;\n  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.15);\n}\n._1s1d76t8 {\n  background: var(--_19ez2rz1);\n  color: var(--_19ez2rzq);\n  border-color: var(--_19ez2rz1);\n  opacity: 0.8;\n}\n._1s1d76t8:hover {\n  opacity: 1;\n}\n._1s1d76t9 {\n  background: var(--_19ez2rz10);\n  color: var(--_19ez2rzq);\n  border-color: var(--_19ez2rz10);\n}\n._1s1d76t9:hover {\n  opacity: 0.9;\n  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.2);\n}\n._1s1d76ta {\n  background: var(--_19ez2rzc);\n  color: var(--_19ez2rz1);\n  border-color: var(--_19ez2rze);\n}\n._1s1d76ta:hover {\n  opacity: 0.9;\n  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.1);\n}\n._1s1d76tb {\n  font-size: 10px;\n  font-weight: 700;\n  letter-spacing: 0.03em;\n  text-transform: uppercase;\n  opacity: 0.9;\n  padding-left: 2px;\n  border-left: 1px solid rgba(255, 255, 255, 0.4);\n  margin-left: 2px;\n}\n._1s1d76tc {\n  width: 14px;\n  height: 14px;\n  flex-shrink: 0;\n}\n._1s1d76td {\n  white-space: nowrap;\n  overflow: hidden;\n  text-overflow: ellipsis;\n  max-width: 200px;\n}\n@media screen and (max-width: 768px) {\n  ._1s1d76t0 {\n    font-size: 11px;\n    padding: 3px 8px;\n  }\n  ._1s1d76tc {\n    width: 12px;\n    height: 12px;\n  }\n  ._1s1d76td {\n    max-width: 150px;\n  }\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 51238:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
__webpack_require__.r(__webpack_exports__);
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   "default": () => (/* binding */ FanoutLayout),
/* harmony export */   metadata: () => (/* binding */ metadata)
/* harmony export */ });
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(12129);
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__);

const metadata = {
    title: "Compare Variants - Stapler Squad",
    description: "Compare parallel fan-out variants side by side and pick a winner."
};
function FanoutLayout({ children }) {
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.Fragment, {
        children: children
    });
}


/***/ }),

/***/ 51339:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, ".e4319t0 {\n  display: flex;\n  align-items: center;\n  padding: var(--_19ez2rz2z) var(--_19ez2rz30);\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 55591:
/***/ ((module) => {

"use strict";
module.exports = require("https");

/***/ }),

/***/ 62722:
/***/ (() => {



/***/ }),

/***/ 63033:
/***/ ((module) => {

"use strict";
module.exports = require("next/dist/server/app-render/work-unit-async-storage.external.js");

/***/ }),

/***/ 72354:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
__webpack_require__.r(__webpack_exports__);
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   GlobalError: () => (/* reexport default from dynamic */ next_dist_client_components_error_boundary__WEBPACK_IMPORTED_MODULE_2___default.a),
/* harmony export */   __next_app__: () => (/* binding */ __next_app__),
/* harmony export */   pages: () => (/* binding */ pages),
/* harmony export */   routeModule: () => (/* binding */ routeModule),
/* harmony export */   tree: () => (/* binding */ tree)
/* harmony export */ });
/* harmony import */ var next_dist_server_route_modules_app_page_module_compiled__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(54315);
/* harmony import */ var next_dist_server_route_modules_app_page_module_compiled__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(next_dist_server_route_modules_app_page_module_compiled__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var next_dist_server_route_kind__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(34788);
/* harmony import */ var next_dist_client_components_error_boundary__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(92838);
/* harmony import */ var next_dist_client_components_error_boundary__WEBPACK_IMPORTED_MODULE_2___default = /*#__PURE__*/__webpack_require__.n(next_dist_client_components_error_boundary__WEBPACK_IMPORTED_MODULE_2__);
/* harmony import */ var next_dist_server_app_render_entry_base__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(79793);
/* harmony import */ var next_dist_server_app_render_entry_base__WEBPACK_IMPORTED_MODULE_3___default = /*#__PURE__*/__webpack_require__.n(next_dist_server_app_render_entry_base__WEBPACK_IMPORTED_MODULE_3__);
/* harmony reexport (unknown) */ var __WEBPACK_REEXPORT_OBJECT__ = {};
/* harmony reexport (unknown) */ for(const __WEBPACK_IMPORT_KEY__ in next_dist_server_app_render_entry_base__WEBPACK_IMPORTED_MODULE_3__) if(["default","tree","pages","GlobalError","__next_app__","routeModule"].indexOf(__WEBPACK_IMPORT_KEY__) < 0) __WEBPACK_REEXPORT_OBJECT__[__WEBPACK_IMPORT_KEY__] = () => next_dist_server_app_render_entry_base__WEBPACK_IMPORTED_MODULE_3__[__WEBPACK_IMPORT_KEY__]
/* harmony reexport (unknown) */ __webpack_require__.d(__webpack_exports__, __WEBPACK_REEXPORT_OBJECT__);
const module0 = () => Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 36776));
const module1 = () => Promise.resolve(/* import() eager */).then(__webpack_require__.t.bind(__webpack_require__, 93250, 23));
const module2 = () => Promise.resolve(/* import() eager */).then(__webpack_require__.t.bind(__webpack_require__, 76963, 23));
const module3 = () => Promise.resolve(/* import() eager */).then(__webpack_require__.t.bind(__webpack_require__, 54552, 23));
const module4 = () => Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 51238));
const page5 = () => Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 11645));


// We inject the tree and pages here so that we can use them in the route
// module.
const tree = {
        children: [
        '',
        {
        children: [
        'fanout',
        {
        children: ['__PAGE__', {}, {
          page: [page5, "/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/fanout/page.tsx"],
          
        }]
      },
        {
        'layout': [module4, "/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/fanout/layout.tsx"],
        
      }
      ]
      },
        {
        'layout': [module0, "/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/layout.tsx"],
'not-found': [module1, "next/dist/client/components/not-found-error"],
'forbidden': [module2, "next/dist/client/components/forbidden-error"],
'unauthorized': [module3, "next/dist/client/components/unauthorized-error"],
        
      }
      ]
      }.children;
const pages = ["/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/fanout/page.tsx"];


const __next_app_require__ = __webpack_require__
const __next_app_load_chunk__ = () => Promise.resolve()
const __next_app__ = {
    require: __next_app_require__,
    loadChunk: __next_app_load_chunk__
};

// Create and export the route module that will be consumed.
const routeModule = new next_dist_server_route_modules_app_page_module_compiled__WEBPACK_IMPORTED_MODULE_0__.AppPageRouteModule({
    definition: {
        kind: next_dist_server_route_kind__WEBPACK_IMPORTED_MODULE_1__.RouteKind.APP_PAGE,
        page: "/fanout/page",
        pathname: "/fanout",
        // The following aren't used in production.
        bundlePath: '',
        filename: '',
        appPaths: []
    },
    userland: {
        loaderTree: tree
    }
});

//# sourceMappingURL=app-page.js.map

/***/ }),

/***/ 73136:
/***/ ((module) => {

"use strict";
module.exports = require("node:url");

/***/ }),

/***/ 73566:
/***/ ((module) => {

"use strict";
module.exports = require("worker_threads");

/***/ }),

/***/ 74075:
/***/ ((module) => {

"use strict";
module.exports = require("zlib");

/***/ }),

/***/ 76760:
/***/ ((module) => {

"use strict";
module.exports = require("node:path");

/***/ }),

/***/ 79551:
/***/ ((module) => {

"use strict";
module.exports = require("url");

/***/ }),

/***/ 81053:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  "default": () => (/* binding */ FanoutRoute)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
// EXTERNAL MODULE: ./src/lib/nav/index.ts
var nav = __webpack_require__(7220);
// EXTERNAL MODULE: ./src/lib/analytics/usePageView.ts
var usePageView = __webpack_require__(15116);
// EXTERNAL MODULE: ./src/gen/session/v1/types_pb.ts
var types_pb = __webpack_require__(29238);
// EXTERNAL MODULE: ./src/components/sessions/DiffViewer.tsx + 2 modules
var DiffViewer = __webpack_require__(96339);
// EXTERNAL MODULE: ./src/lib/contexts/SessionVcsContext.tsx
var SessionVcsContext = __webpack_require__(2452);
// EXTERNAL MODULE: ./src/lib/config.ts
var config = __webpack_require__(99706);
// EXTERNAL MODULE: ./src/lib/utils/connectError.ts
var connectError = __webpack_require__(22602);
// EXTERNAL MODULE: ./src/lib/hooks/useFanoutService.ts
var useFanoutService = __webpack_require__(64626);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Ffanout%2Fpage.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xYmRmMWkxMCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogMTJweDsKICBwYWRkaW5nOiAxNnB4OwogIGhlaWdodDogMTAwJTsKICBtaW4taGVpZ2h0OiAwOwp9Ci5fMWJkZjFpMTEgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IDEycHg7CiAganVzdGlmeS1jb250ZW50OiBzcGFjZS1iZXR3ZWVuOwp9Ci5fMWJkZjFpMTIgewogIGRpc3BsYXk6IGdyaWQ7CiAgZ3JpZC1hdXRvLWZsb3c6IGNvbHVtbjsKICBncmlkLWF1dG8tY29sdW1uczogbWlubWF4KDMyMHB4LCAxZnIpOwogIGdhcDogMTJweDsKICBmbGV4OiAxOwogIG1pbi1oZWlnaHQ6IDA7CiAgb3ZlcmZsb3cteDogYXV0bzsKfQouXzFiZGYxaTEzIHsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXgtZGlyZWN0aW9uOiBjb2x1bW47CiAgZ2FwOiA4cHg7CiAgbWluLWhlaWdodDogMDsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGJvcmRlci1yYWRpdXM6IDhweDsKICBwYWRkaW5nOiA4cHg7Cn0KLl8xYmRmMWkxNCB7CiAgZGlzcGxheTogZmxleDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGp1c3RpZnktY29udGVudDogc3BhY2UtYmV0d2VlbjsKICBnYXA6IDhweDsKfQouXzFiZGYxaTE1IHsKICBmbGV4OiAxOwogIG1pbi1oZWlnaHQ6IDA7CiAgb3ZlcmZsb3c6IGF1dG87Cn0KLl8xYmRmMWkxNiB7CiAgZm9udC1zaXplOiAxMnB4OwogIHBhZGRpbmc6IDJweCA4cHg7CiAgYm9yZGVyLXJhZGl1czogOTk5cHg7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnplKTsKfQouXzFiZGYxaTE3IHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxMCk7Cn0%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Ffanout_2Fpage_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xYmRmMWkxMCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogMTJweDsKICBwYWRkaW5nOiAxNnB4OwogIGhlaWdodDogMTAwJTsKICBtaW4taGVpZ2h0OiAwOwp9Ci5fMWJkZjFpMTEgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IDEycHg7CiAganVzdGlmeS1jb250ZW50OiBzcGFjZS1iZXR3ZWVuOwp9Ci5fMWJkZjFpMTIgewogIGRpc3BsYXk6IGdyaWQ7CiAgZ3JpZC1hdXRvLWZsb3c6IGNvbHVtbjsKICBncmlkLWF1dG8tY29sdW1uczogbWlubWF4KDMyMHB4LCAxZnIpOwogIGdhcDogMTJweDsKICBmbGV4OiAxOwogIG1pbi1oZWlnaHQ6IDA7CiAgb3ZlcmZsb3cteDogYXV0bzsKfQouXzFiZGYxaTEzIHsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXgtZGlyZWN0aW9uOiBjb2x1bW47CiAgZ2FwOiA4cHg7CiAgbWluLWhlaWdodDogMDsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGJvcmRlci1yYWRpdXM6IDhweDsKICBwYWRkaW5nOiA4cHg7Cn0KLl8xYmRmMWkxNCB7CiAgZGlzcGxheTogZmxleDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGp1c3RpZnktY29udGVudDogc3BhY2UtYmV0d2VlbjsKICBnYXA6IDhweDsKfQouXzFiZGYxaTE1IHsKICBmbGV4OiAxOwogIG1pbi1oZWlnaHQ6IDA7CiAgb3ZlcmZsb3c6IGF1dG87Cn0KLl8xYmRmMWkxNiB7CiAgZm9udC1zaXplOiAxMnB4OwogIHBhZGRpbmc6IDJweCA4cHg7CiAgYm9yZGVyLXJhZGl1czogOTk5cHg7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnplKTsKfQouXzFiZGYxaTE3IHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxMCk7Cn0_22_7D = __webpack_require__(22053);
;// ./src/app/fanout/page.css.ts


var badge = '_1bdf1i16';
var column = '_1bdf1i13';
var columnHeader = '_1bdf1i14';
var diffPane = '_1bdf1i15';
var errorText = '_1bdf1i17';
var grid = '_1bdf1i12';
var header = '_1bdf1i11';
var page = '_1bdf1i10';
;// ./src/app/fanout/page.tsx
/* __next_internal_client_entry_do_not_use__ default auto */ 
// +feature: parallel-fanout










const POLL_MS = 5000;
// Mirrors server fanoutWinnerEligibility (session_service_fanout.go); the server is authoritative
// and also rejects an Active session that is still producing output.
const MAYBE_PICKABLE = [
    types_pb/* SessionStatus */.Ph.ACTIVE,
    types_pb/* SessionStatus */.Ph.PAUSED,
    types_pb/* SessionStatus */.Ph.STOPPED,
    types_pb/* SessionStatus */.Ph.HIBERNATED
];
function statusLabel(s) {
    if (s.archivedAt) return "Archived";
    return types_pb/* SessionStatus */.Ph[s.status]?.toString().toLowerCase() ?? "unknown";
}
function FanoutComparison({ groupId }) {
    (0,usePageView/* usePageView */.O)();
    const [members, setMembers] = (0,react.useState)([]);
    const [error, setError] = (0,react.useState)(null);
    const [confirming, setConfirming] = (0,react.useState)(null);
    const [picked, setPicked] = (0,react.useState)(null);
    const [pending, setPending] = (0,react.useState)(false);
    const requestSeq = (0,react.useRef)(0);
    // Only the latest request may update state, so a slow poll issued before a pick can't
    // overwrite the post-pick refresh with stale members.
    const refresh = (0,react.useCallback)(async ()=>{
        const seq = ++requestSeq.current;
        try {
            const next = await (0,useFanoutService/* listFanoutMembers */.tI)(groupId);
            if (seq !== requestSeq.current) return;
            setMembers(next);
            setError(null);
        } catch (err) {
            if (seq !== requestSeq.current) return;
            setError((0,connectError/* getErrorMessage */.u)(err, "Fan-out request failed"));
        }
    }, [
        groupId
    ]);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (!groupId) return;
        void refresh();
        const t = setInterval(()=>void refresh(), POLL_MS);
        return ()=>{
            clearInterval(t);
            requestSeq.current++; // invalidate in-flight requests after unmount/groupId change
        };
    }, [
        groupId,
        refresh
    ]);
    const pick = async (sessionId)=>{
        if (pending) return;
        setPending(true);
        setError(null);
        try {
            const res = await (0,useFanoutService/* pickFanoutWinner */.cf)(groupId, sessionId);
            if (res.failedArchiveSessionIds.length > 0) {
                // Leave picking enabled: repeating the same pick is idempotent and retries the archive.
                await refresh();
                setError(`Winner merged, but ${res.failedArchiveSessionIds.length} other variant(s) could not be archived. Pick the same winner again to retry.`);
            } else {
                setPicked(sessionId);
                await refresh();
            }
        } catch (err) {
            // Includes the structured merge-conflict error (Aborted) — surfaced, base branch unchanged.
            setError((0,connectError/* getErrorMessage */.u)(err, "Fan-out request failed"));
        } finally{
            setConfirming(null);
            setPending(false);
        }
    };
    if (!groupId) return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
        className: page,
        children: "Missing groupId."
    });
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: page,
        "data-testid": "fanout-comparison",
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: header,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("h1", {
                        children: [
                            "Compare variants (",
                            members.length,
                            ")"
                        ]
                    }),
                    picked && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                        "data-testid": "fanout-picked",
                        children: "Winner merged; other variants archived."
                    })
                ]
            }),
            error && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                role: "alert",
                className: errorText,
                "data-testid": "fanout-error",
                children: error
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                className: grid,
                children: members.map((s)=>{
                    const archived = Boolean(s.archivedAt);
                    const canPick = !picked && !archived && MAYBE_PICKABLE.includes(s.status);
                    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("section", {
                        className: column,
                        "data-testid": `fanout-column-${s.id}`,
                        "aria-label": s.title,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                className: columnHeader,
                                children: [
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("strong", {
                                        children: s.title
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                        className: badge,
                                        "data-testid": `fanout-status-${s.id}`,
                                        children: statusLabel(s)
                                    })
                                ]
                            }),
                            confirming === s.id ? /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                "data-testid": `fanout-confirm-${s.id}`,
                                disabled: pending,
                                onClick: ()=>void pick(s.id),
                                children: "Confirm: merge this variant and archive the others"
                            }) : /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                "data-testid": `fanout-pick-${s.id}`,
                                disabled: !canPick || pending,
                                onClick: ()=>setConfirming(s.id),
                                children: "Pick as winner"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                                className: diffPane,
                                children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)(SessionVcsContext/* SessionVcsProvider */.m, {
                                    sessionId: s.id,
                                    baseUrl: (0,config/* getApiBaseUrl */.h)(),
                                    children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)(DiffViewer/* DiffViewer */.M, {
                                        session: s
                                    })
                                })
                            })
                        ]
                    }, s.id);
                })
            })
        ]
    });
}
function FanoutRouteInner() {
    const groupId = (0,nav/* useSearchParams */.ok)().get("groupId") ?? "";
    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)(FanoutComparison, {
        groupId: groupId
    });
}
function FanoutRoute() {
    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)(react.Suspense, {
        children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)(FanoutRouteInner, {})
    });
}


/***/ }),

/***/ 81630:
/***/ ((module) => {

"use strict";
module.exports = require("http");

/***/ }),

/***/ 83997:
/***/ ((module) => {

"use strict";
module.exports = require("tty");

/***/ }),

/***/ 94266:
/***/ (() => {



/***/ }),

/***/ 96339:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  M: () => (/* binding */ DiffViewer)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ./src/lib/contexts/SessionVcsContext.tsx
var SessionVcsContext = __webpack_require__(2452);
// EXTERNAL MODULE: ./src/components/shared/DiffRenderer.tsx + 1 modules
var DiffRenderer = __webpack_require__(13259);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/timestamp.js
var timestamp = __webpack_require__(42865);
// EXTERNAL MODULE: ./src/lib/utils/datetime.ts
var datetime = __webpack_require__(43795);
// EXTERNAL MODULE: ./src/components/shared/GitHubBadge.css.ts
var GitHubBadge_css = __webpack_require__(59634);
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
 */ function CIStatusBadge({ checkConclusion, prUrl, prNumber, lastChecked }) {
    if (!prNumber) {
        return null;
    }
    const info = statusInfo(checkConclusion);
    const href = prUrl ? `${prUrl}/checks` : undefined;
    const tooltipParts = [
        `CI status: ${info.label}`
    ];
    if (lastChecked) {
        tooltipParts.push(`checked ${(0,datetime/* formatRelativeTime */.fw)((0,timestamp/* timestampDate */.FP)(lastChecked).getTime())}`);
    }
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("a", {
        href: href,
        target: "_blank",
        rel: "noopener noreferrer",
        className: `${GitHubBadge_css/* badge */.qS} ${GitHubBadge_css/* prBadge */.Eg} ${info.variant}`,
        role: "status",
        "aria-label": `CI status: ${info.label}`,
        title: tooltipParts.join(" · "),
        "data-testid": "ci-status-badge",
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                className: `${GitHubBadge_css/* icon */.Kk}`,
                "aria-hidden": "true",
                children: info.icon
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                className: `${GitHubBadge_css/* text */.Qq}`,
                children: info.label
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FDiffViewer.css.ts.vanilla.css%22%2C%22source%22%3A%22LmU0MzE5dDAgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejJ6KSB2YXIoLS1fMTllejJyejMwKTsKfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FDiffViewer_css_ts_vanilla_css_22_2C_22source_22_3A_22LmU0MzE5dDAgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejJ6KSB2YXIoLS1fMTllejJyejMwKTsKfQ_22_7D = __webpack_require__(51339);
;// ./src/components/sessions/DiffViewer.css.ts


var diffHeader = 'e4319t0';
;// ./src/components/sessions/DiffViewer.tsx
/* __next_internal_client_entry_do_not_use__ DiffViewer auto */ 




/** Session-aware diff viewer — reads from SessionVcsContext. */ function DiffViewer({ session }) {
    const { diff: rawDiff, diffLoading: loading, refreshDiff } = (0,SessionVcsContext/* useSessionVcsContext */.l)();
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
        children: [
            session?.githubPrNumber ? /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                className: diffHeader,
                children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)(CIStatusBadge, {
                    checkConclusion: session.githubCheckConclusion,
                    prUrl: session.githubPrUrl,
                    prNumber: session.githubPrNumber,
                    lastChecked: session.lastPrStatusCheck
                })
            }) : null,
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(DiffRenderer/* DiffRenderer */.F, {
                content: rawDiff?.content ?? "",
                added: rawDiff?.added ?? 0,
                removed: rawDiff?.removed ?? 0,
                loading: loading,
                onRefresh: refreshDiff
            })
        ]
    });
}


/***/ })

};
;

// load runtime
var __webpack_require__ = require("../../webpack-runtime.js");
__webpack_require__.C(exports);
var __webpack_exec__ = (moduleId) => (__webpack_require__(__webpack_require__.s = moduleId))
var __webpack_exports__ = __webpack_require__.X(0, [4655,4164,8130], () => (__webpack_exec__(72354)));
module.exports = __webpack_exports__;

})();