"use strict";
exports.id = 6656;
exports.ids = [6656];
exports.modules = {

/***/ 26656:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  StageCostChart: () => (/* binding */ StageCostChart)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/component/ResponsiveContainer.js + 1 modules
var ResponsiveContainer = __webpack_require__(51450);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/chart/BarChart.js
var BarChart = __webpack_require__(45951);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/cartesian/CartesianGrid.js
var CartesianGrid = __webpack_require__(7890);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/cartesian/XAxis.js
var XAxis = __webpack_require__(21529);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/cartesian/YAxis.js
var YAxis = __webpack_require__(16990);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/component/Tooltip.js + 11 modules
var Tooltip = __webpack_require__(9205);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/cartesian/Bar.js + 10 modules
var Bar = __webpack_require__(41258);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/component/Cell.js
var Cell = __webpack_require__(68213);
// EXTERNAL MODULE: ./src/app/insights/ModelBreakdownChart.css.ts
var ModelBreakdownChart_css = __webpack_require__(31135);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Finsights%2FStageCostChart.css.ts.vanilla.css%22%2C%22source%22%3A%22LmU3cjVkaTAgewogIGN1cnNvcjogcG9pbnRlcjsKICBib3JkZXI6IG5vbmU7CiAgYmFja2dyb3VuZDogdHJhbnNwYXJlbnQ7CiAgcGFkZGluZzogMDsKICBmb250OiBpbmhlcml0Owp9Ci5lN3I1ZGkwOmZvY3VzLXZpc2libGUgewogIG91dGxpbmU6IDJweCBzb2xpZCB2YXIoLS1fMTllejJyem0pOwogIG91dGxpbmUtb2Zmc2V0OiAycHg7CiAgYm9yZGVyLXJhZGl1czogdmFyKC0tXzE5ZXoycnozNik7Cn0KLmU3cjVkaTEgewogIGJveC1zaGFkb3c6IDAgMCAwIDJweCB2YXIoLS1fMTllejJyem0pOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzYpOwp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Finsights_2FStageCostChart_css_ts_vanilla_css_22_2C_22source_22_3A_22LmU3cjVkaTAgewogIGN1cnNvcjogcG9pbnRlcjsKICBib3JkZXI6IG5vbmU7CiAgYmFja2dyb3VuZDogdHJhbnNwYXJlbnQ7CiAgcGFkZGluZzogMDsKICBmb250OiBpbmhlcml0Owp9Ci5lN3I1ZGkwOmZvY3VzLXZpc2libGUgewogIG91dGxpbmU6IDJweCBzb2xpZCB2YXIoLS1fMTllejJyem0pOwogIG91dGxpbmUtb2Zmc2V0OiAycHg7CiAgYm9yZGVyLXJhZGl1czogdmFyKC0tXzE5ZXoycnozNik7Cn0KLmU3cjVkaTEgewogIGJveC1zaGFkb3c6IDAgMCAwIDJweCB2YXIoLS1fMTllejJyem0pOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzYpOwp9_22_7D = __webpack_require__(40892);
;// ./src/app/insights/StageCostChart.css.ts

var legendButton = 'e7r5di0';
var legendButtonActive = 'e7r5di1';
// EXTERNAL MODULE: ./src/app/insights/insightsFormatters.ts
var insightsFormatters = __webpack_require__(41010);
;// ./src/app/insights/StageCostChart.tsx
// +feature: insights-dashboard
/* __next_internal_client_entry_do_not_use__ StageCostChart auto */ 





// Same stable palette ModelBreakdownChart uses — per ux.md's Accessibility
// section, reusing an already-shipped, presumably-audited palette introduces
// no new color decisions.
const PALETTE = [
    "#6366f1",
    "#10b981",
    "#f59e0b",
    "#ef4444",
    "#3b82f6",
    "#ec4899",
    "#8b5cf6",
    "#14b8a6"
];
function toDataPoints(roles) {
    return [
        ...roles
    ].sort((a, b)=>b.estimatedCostUsd - a.estimatedCostUsd).map((r, i)=>({
            role: r.sessionRole,
            cost: r.estimatedCostUsd,
            color: PALETTE[i % PALETTE.length],
            unpricedSessionCount: r.unpricedSessionCount
        }));
}
/** Builds the role="img" aria-label from the same sorted data the bars render, so it never drifts out of sync with what's visually shown (AC13). */ function buildChartAriaLabel(data) {
    const parts = data.map((d)=>{
        const unpriced = d.unpricedSessionCount > 0 ? ` (${d.unpricedSessionCount} unpriced)` : "";
        return `${(0,insightsFormatters/* roleDisplayLabel */.jd)(d.role)} ${(0,insightsFormatters/* fmtCost */.F0)(d.cost)}${unpriced}`;
    });
    return `Cost by stage: ${parts.join(", ")}`;
}
function StageCostChart({ roles, activeRole, onRoleClick }) {
    const data = (0,react.useMemo)(()=>toDataPoints(roles), [
        roles
    ]);
    if (data.length === 0) {
        return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
            className: ModelBreakdownChart_css/* chartCard */.VC,
            "data-testid": "stage-cost-chart",
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                    className: ModelBreakdownChart_css/* chartTitle */.vc,
                    children: "Cost by Stage"
                }),
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                    className: ModelBreakdownChart_css/* emptyChart */.aX,
                    children: "No data"
                })
            ]
        });
    }
    const ariaLabel = buildChartAriaLabel(data);
    const handleRoleActivate = (role)=>onRoleClick?.(role);
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: ModelBreakdownChart_css/* chartCard */.VC,
        "data-testid": "stage-cost-chart",
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                className: ModelBreakdownChart_css/* chartTitle */.vc,
                children: "Cost by Stage"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                className: ModelBreakdownChart_css/* chartWrap */.jm,
                role: "img",
                "aria-label": ariaLabel,
                children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)(ResponsiveContainer/* ResponsiveContainer */.u, {
                    width: "100%",
                    height: "100%",
                    children: /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(BarChart/* BarChart */.E, {
                        data: data,
                        margin: {
                            top: 4,
                            right: 8,
                            bottom: 4,
                            left: 8
                        },
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(CartesianGrid/* CartesianGrid */.d, {
                                strokeDasharray: "3 3",
                                stroke: "rgba(128,128,128,0.15)",
                                vertical: false
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(XAxis/* XAxis */.W, {
                                dataKey: "role",
                                tickFormatter: insightsFormatters/* roleDisplayLabel */.jd,
                                tick: {
                                    fontSize: 11
                                },
                                tickLine: false,
                                axisLine: false
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(YAxis/* YAxis */.h, {
                                tickFormatter: insightsFormatters/* fmtCost */.F0,
                                tick: {
                                    fontSize: 11
                                },
                                tickLine: false,
                                axisLine: false,
                                width: 56
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(Tooltip/* Tooltip */.m, {
                                formatter: (v)=>[
                                        (0,insightsFormatters/* fmtCost */.F0)(Number(v)),
                                        "Cost"
                                    ],
                                contentStyle: {
                                    fontSize: "12px"
                                }
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(Bar/* Bar */.yP, {
                                dataKey: "cost",
                                radius: [
                                    4,
                                    4,
                                    0,
                                    0
                                ],
                                onClick: (entry)=>{
                                    const point = entry?.payload;
                                    if (point?.role) handleRoleActivate(point.role);
                                },
                                cursor: onRoleClick ? "pointer" : undefined,
                                children: data.map((entry, index)=>/*#__PURE__*/ (0,react_jsx_runtime.jsx)(Cell/* Cell */.f, {
                                        fill: entry.color
                                    }, `cell-${index}`))
                            })
                        ]
                    })
                })
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                className: ModelBreakdownChart_css/* legendRow */.eI,
                children: data.map((d)=>{
                    const isActive = activeRole === d.role;
                    const displayRole = (0,insightsFormatters/* roleDisplayLabel */.jd)(d.role);
                    const label = `${displayRole}${d.unpricedSessionCount > 0 ? `, ${d.unpricedSessionCount} unpriced session${d.unpricedSessionCount === 1 ? "" : "s"}` : ""}`;
                    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("button", {
                        type: "button",
                        role: "button",
                        tabIndex: 0,
                        className: `${ModelBreakdownChart_css/* legendItem */.fI} ${legendButton} ${isActive ? legendButtonActive : ""}`,
                        onClick: ()=>handleRoleActivate(d.role),
                        onKeyDown: (e)=>{
                            if (e.key === "Enter" || e.key === " ") {
                                e.preventDefault();
                                handleRoleActivate(d.role);
                            }
                        },
                        "aria-pressed": onRoleClick ? isActive : undefined,
                        "aria-label": label,
                        "data-testid": `stage-cost-legend-${displayRole}`,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                                className: ModelBreakdownChart_css/* legendDot */.dD,
                                style: {
                                    background: d.color
                                }
                            }),
                            displayRole,
                            d.unpricedSessionCount > 0 && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                                className: ModelBreakdownChart_css/* unpricedLabel */.hu,
                                children: [
                                    " (",
                                    d.unpricedSessionCount,
                                    " unpriced)"
                                ]
                            })
                        ]
                    }, d.role);
                })
            })
        ]
    });
}


/***/ }),

/***/ 31135:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   VC: () => (/* binding */ chartCard),
/* harmony export */   aX: () => (/* binding */ emptyChart),
/* harmony export */   dD: () => (/* binding */ legendDot),
/* harmony export */   eA: () => (/* binding */ cacheHitLabel),
/* harmony export */   eI: () => (/* binding */ legendRow),
/* harmony export */   fI: () => (/* binding */ legendItem),
/* harmony export */   hu: () => (/* binding */ unpricedLabel),
/* harmony export */   jm: () => (/* binding */ chartWrap),
/* harmony export */   vc: () => (/* binding */ chartTitle)
/* harmony export */ });
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fapp_2Finsights_2FModelBreakdownChart_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA5VSy27DIBC85yu4VGoORNhpm4R8TEQMtjfBYAGJH1X_vYImrW3iSL3YYmd2Z3ZgdUi6_HqCkqDPBUJHlp0Loy_KU3Rl5hXjQ7ITfWr6zXLvcW24MBQldYuslsAnLDFgYcM4XOx00HobODXjHFQRockyruwXX4vV3WgSjOZaOWyhF9GAYxgf8EZAUbqIUQZGpqU2U4wEqGKmAIWP2jldRe1k7CcNfhrgrqQoIeTFTyhvymlK6nZEXwc6B1tL1lGUS9H6BiahUBicqCxFmVBOGF8_XayDvMOZVk4oN4QmEjMLpX9pzKY1cPf22J3_48awmiL_9aXCHyZaXXR3ozydjlv6sf77v9J57OH5wmz_8icP7WN4sdufjO_h347PH_pu_ZudLQ2oM0VkpLAJCg_99IM1XCcFReCYhGzUv53vT_f7vwG9PhEs8wMAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(36694);

var cacheHitLabel = '_1yfvjih8';
var chartCard = '_1yfvjih0';
var chartTitle = '_1yfvjih1';
var chartWrap = '_1yfvjih2';
var emptyChart = '_1yfvjih3';
var legendDot = '_1yfvjih6';
var legendItem = '_1yfvjih5';
var legendRow = '_1yfvjih4';
var unpricedLabel = '_1yfvjih7';

/***/ }),

/***/ 40892:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, ".e7r5di0 {\n  cursor: pointer;\n  border: none;\n  background: transparent;\n  padding: 0;\n  font: inherit;\n}\n.e7r5di0:focus-visible {\n  outline: 2px solid var(--_19ez2rzm);\n  outline-offset: 2px;\n  border-radius: var(--_19ez2rz36);\n}\n.e7r5di1 {\n  box-shadow: 0 0 0 2px var(--_19ez2rzm);\n  border-radius: var(--_19ez2rz36);\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ })

};
;