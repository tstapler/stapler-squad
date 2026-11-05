(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[8645],{

/***/ 6839:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  "default": () => (/* binding */ SessionDetailRoute)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/nav/index.ts
var nav = __webpack_require__(41078);
// EXTERNAL MODULE: ./src/lib/analytics/usePageView.ts
var usePageView = __webpack_require__(90814);
// EXTERNAL MODULE: ./src/lib/hooks/useInsightsService.ts
var useInsightsService = __webpack_require__(81240);
// EXTERNAL MODULE: ./src/lib/hooks/useBacklogService.ts
var useBacklogService = __webpack_require__(8570);
// EXTERNAL MODULE: ./src/app/insights/SessionDetailContent.tsx + 2 modules
var SessionDetailContent = __webpack_require__(16522);
// EXTERNAL MODULE: ./src/app/insights/insightsFormatters.ts
var insightsFormatters = __webpack_require__(97900);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Finsights%2Fsession-detail%2Fpage.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xeHdoanMxMCB7CiAgbWF4LXdpZHRoOiA5MDBweDsKICBtYXJnaW46IDAgYXV0bzsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejMxKTsKICBoZWlnaHQ6IDEwMCU7CiAgb3ZlcmZsb3cteTogYXV0bzsKfQouXzF4d2hqczExIHsKICBkaXNwbGF5OiBpbmxpbmUtYmxvY2s7CiAgbWFyZ2luLWJvdHRvbTogdmFyKC0tXzE5ZXoycnozMCk7CiAgdGV4dC1kZWNvcmF0aW9uOiBub25lOwp9Ci5fMXh3aGpzMTIgewogIG91dGxpbmU6IG5vbmU7Cn0%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Finsights_2Fsession_detail_2Fpage_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xeHdoanMxMCB7CiAgbWF4LXdpZHRoOiA5MDBweDsKICBtYXJnaW46IDAgYXV0bzsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejMxKTsKICBoZWlnaHQ6IDEwMCU7CiAgb3ZlcmZsb3cteTogYXV0bzsKfQouXzF4d2hqczExIHsKICBkaXNwbGF5OiBpbmxpbmUtYmxvY2s7CiAgbWFyZ2luLWJvdHRvbTogdmFyKC0tXzE5ZXoycnozMCk7CiAgdGV4dC1kZWNvcmF0aW9uOiBub25lOwp9Ci5fMXh3aGpzMTIgewogIG91dGxpbmU6IG5vbmU7Cn0_22_7D = __webpack_require__(9177);
;// ./src/app/insights/session-detail/page.css.ts

var backLink = '_1xwhjs11';
var heading = '_1xwhjs12';
var main = '_1xwhjs10';
;// ./src/app/insights/session-detail/SessionDetailPageClient.tsx
// +feature: insights-session-detail-route
/* __next_internal_client_entry_do_not_use__ SessionDetailPageClient auto */ 







/**
 * Client component for the deep-linkable /insights/session-detail?sessionId=
 * route (Epic 1.4, Story 1.4.3). Fetches independently of any dashboard
 * state — a cold direct navigation (bookmark, refresh, shared link) must
 * render correctly with no parent InsightsDashboard mounted.
 */ function SessionDetailPageClient(param) {
    let { sessionId } = param;
    const { summary, loading, error } = (0,useInsightsService/* useSessionDetail */.xk)(sessionId);
    const { turns } = (0,useInsightsService/* useSessionTurnTimeline */.Ih)(summary === null || summary === void 0 ? void 0 : summary.conversationId);
    const { index: backlogIndex } = (0,useBacklogService/* useBacklogSessionIndex */.Z3)();
    const headingRef = (0,react.useRef)(null);
    // Route mount moves focus to the heading (Epic 1.4, Story 1.4.3d) — a
    // route gets no free role="dialog" AT signal the way the modal does, so
    // this is the only cue a screen reader gets that content changed.
    (0,react.useEffect)(()=>{
        var _headingRef_current;
        (_headingRef_current = headingRef.current) === null || _headingRef_current === void 0 ? void 0 : _headingRef_current.focus();
    }, []);
    const notFound = !loading && !error && !summary;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("main", {
        id: "main-content",
        className: main,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)(nav/* Link */.N_, {
                href: "/insights",
                className: backLink,
                children: "← Back to dashboard"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("h1", {
                ref: headingRef,
                tabIndex: -1,
                className: heading,
                children: [
                    "Session Details",
                    summary ? ": ".concat((0,insightsFormatters/* shortId */.m_)(summary.sessionId || summary.conversationId)) : ""
                ]
            }),
            loading && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                role: "status",
                children: "Loading session…"
            }),
            !loading && error && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                role: "alert",
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                    children: [
                        "Couldn't load session: ",
                        error
                    ]
                })
            }),
            notFound && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                "data-testid": "session-not-found",
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    children: "Session not found."
                })
            }),
            !loading && !error && summary && /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionDetailContent/* SessionDetailContent */.d, {
                session: summary,
                backlogEntry: summary.sessionId ? backlogIndex.get(summary.sessionId) : undefined,
                turns: turns
            })
        ]
    });
}

;// ./src/app/insights/session-detail/page.tsx
// +feature: insights-session-detail-route
/* __next_internal_client_entry_do_not_use__ default auto */ 




/**
 * Durable standalone route for a session's detail view (Epic 1.4, Story
 * 1.4.3), replacing the original `/insights/session/[sessionId]/` dynamic
 * route. This app builds with `output: "export"` (static export embedded in
 * the Go binary — see next.config.ts), and a dynamic path segment has no
 * pre-renderable params at build time since session IDs aren't known until
 * runtime: a cold navigation straight to `/insights/session/<id>` (bookmark,
 * refresh, shared link — bypassing the already-loaded SPA router) rendered
 * the root dashboard instead of the session page, defeating the point of a
 * "bookmarkable route".
 *
 * Uses a `?sessionId=` query param instead, mirroring the identical fix
 * already established at `/sessions/summary?sessionId=`
 * (src/app/sessions/summary/page.tsx, Epic 3.3): a query param resolves
 * entirely client-side via `useSearchParams()` against the same static
 * HTML/JS bundle on cold load, with no dynamic path segment involved.
 */ function SessionDetailRouteInner() {
    (0,usePageView/* usePageView */.O)();
    const searchParams = (0,nav/* useSearchParams */.ok)();
    var _searchParams_get;
    const sessionId = (_searchParams_get = searchParams.get("sessionId")) !== null && _searchParams_get !== void 0 ? _searchParams_get : "";
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionDetailPageClient, {
        sessionId: sessionId
    });
}
function SessionDetailRoute() {
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(react.Suspense, {
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionDetailRouteInner, {})
    });
}


/***/ }),

/***/ 9177:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 56872:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 6839));


/***/ })

},
/******/ __webpack_require__ => { // webpackRuntimeModules
/******/ var __webpack_exec__ = (moduleId) => (__webpack_require__(__webpack_require__.s = moduleId))
/******/ __webpack_require__.O(0, [1869,852,4202,9353,8165,8890,2805,4891,9691,3516,7358], () => (__webpack_exec__(56872)));
/******/ var __webpack_exports__ = __webpack_require__.O();
/******/ _N_E = __webpack_exports__;
/******/ }
]);
//# sourceMappingURL=page-7ca936867fd37315.js.map