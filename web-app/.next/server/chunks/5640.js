"use strict";
exports.id = 5640;
exports.ids = [5640];
exports.modules = {

/***/ 3516:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   c: () => (/* binding */ Dot)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(88782);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var clsx__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(37595);
/* harmony import */ var _util_types__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(4155);
/* harmony import */ var _util_svgPropertiesNoEvents__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(40959);
/* harmony import */ var _util_DataUtils__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(36223);
function _extends() {
    return _extends = Object.assign ? Object.assign.bind() : function(n) {
        for(var e = 1; e < arguments.length; e++){
            var t = arguments[e];
            for(var r in t)({}).hasOwnProperty.call(t, r) && (n[r] = t[r]);
        }
        return n;
    }, _extends.apply(null, arguments);
}





/**
 * Renders a dot in the chart.
 *
 * This component accepts X and Y coordinates in pixels.
 * If you need to position the rectangle based on your chart's data,
 * consider using the {@link ReferenceDot} component instead.
 *
 * @param props
 * @constructor
 */ var Dot = (props)=>{
    var { cx, cy, r, className } = props;
    var layerClass = (0,clsx__WEBPACK_IMPORTED_MODULE_1__/* .clsx */ .$)('recharts-dot', className);
    if ((0,_util_DataUtils__WEBPACK_IMPORTED_MODULE_2__/* .isNumber */ .Et)(cx) && (0,_util_DataUtils__WEBPACK_IMPORTED_MODULE_2__/* .isNumber */ .Et)(cy) && (0,_util_DataUtils__WEBPACK_IMPORTED_MODULE_2__/* .isNumber */ .Et)(r)) {
        return /*#__PURE__*/ react__WEBPACK_IMPORTED_MODULE_0__.createElement("circle", _extends({}, (0,_util_svgPropertiesNoEvents__WEBPACK_IMPORTED_MODULE_3__/* .svgPropertiesNoEvents */ .uZ)(props), (0,_util_types__WEBPACK_IMPORTED_MODULE_4__/* .adaptEventHandlers */ ._U)(props), {
            className: layerClass,
            cx: cx,
            cy: cy,
            r: r
        }));
    }
    return null;
};


/***/ }),

/***/ 25928:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   W: () => (/* binding */ ActivePoints)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(88782);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var _util_types__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(4155);
/* harmony import */ var _shape_Dot__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(3516);
/* harmony import */ var _container_Layer__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(44084);
/* harmony import */ var _state_hooks__WEBPACK_IMPORTED_MODULE_6__ = __webpack_require__(53151);
/* harmony import */ var _state_selectors_tooltipSelectors__WEBPACK_IMPORTED_MODULE_7__ = __webpack_require__(930);
/* harmony import */ var _hooks__WEBPACK_IMPORTED_MODULE_8__ = __webpack_require__(48917);
/* harmony import */ var _util_DataUtils__WEBPACK_IMPORTED_MODULE_9__ = __webpack_require__(36223);
/* harmony import */ var _util_svgPropertiesNoEvents__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(40959);
/* harmony import */ var _zIndex_ZIndexLayer__WEBPACK_IMPORTED_MODULE_10__ = __webpack_require__(80731);
/* harmony import */ var _zIndex_DefaultZIndexes__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(71115);
function ownKeys(e, r) {
    var t = Object.keys(e);
    if (Object.getOwnPropertySymbols) {
        var o = Object.getOwnPropertySymbols(e);
        r && (o = o.filter(function(r) {
            return Object.getOwnPropertyDescriptor(e, r).enumerable;
        })), t.push.apply(t, o);
    }
    return t;
}
function _objectSpread(e) {
    for(var r = 1; r < arguments.length; r++){
        var t = null != arguments[r] ? arguments[r] : {};
        r % 2 ? ownKeys(Object(t), !0).forEach(function(r) {
            _defineProperty(e, r, t[r]);
        }) : Object.getOwnPropertyDescriptors ? Object.defineProperties(e, Object.getOwnPropertyDescriptors(t)) : ownKeys(Object(t)).forEach(function(r) {
            Object.defineProperty(e, r, Object.getOwnPropertyDescriptor(t, r));
        });
    }
    return e;
}
function _defineProperty(e, r, t) {
    return (r = _toPropertyKey(r)) in e ? Object.defineProperty(e, r, {
        value: t,
        enumerable: !0,
        configurable: !0,
        writable: !0
    }) : e[r] = t, e;
}
function _toPropertyKey(t) {
    var i = _toPrimitive(t, "string");
    return "symbol" == typeof i ? i : i + "";
}
function _toPrimitive(t, r) {
    if ("object" != typeof t || !t) return t;
    var e = t[Symbol.toPrimitive];
    if (void 0 !== e) {
        var i = e.call(t, r || "default");
        if ("object" != typeof i) return i;
        throw new TypeError("@@toPrimitive must return a primitive value.");
    }
    return ("string" === r ? String : Number)(t);
}












var ActivePoint = (_ref)=>{
    var { point, childIndex, mainColor, activeDot, dataKey, clipPath } = _ref;
    if (activeDot === false || point.x == null || point.y == null) {
        return null;
    }
    var dotPropsTyped = {
        index: childIndex,
        dataKey,
        cx: point.x,
        cy: point.y,
        r: 4,
        fill: mainColor !== null && mainColor !== void 0 ? mainColor : 'none',
        strokeWidth: 2,
        stroke: '#fff',
        payload: point.payload,
        value: point.value
    };
    // @ts-expect-error svgPropertiesNoEventsFromUnknown(activeDot) is contributing unknown props
    var dotProps = _objectSpread(_objectSpread(_objectSpread({}, dotPropsTyped), (0,_util_svgPropertiesNoEvents__WEBPACK_IMPORTED_MODULE_1__/* .svgPropertiesNoEventsFromUnknown */ .ic)(activeDot)), (0,_util_types__WEBPACK_IMPORTED_MODULE_2__/* .adaptEventHandlers */ ._U)(activeDot));
    var dot;
    if (/*#__PURE__*/ (0,react__WEBPACK_IMPORTED_MODULE_0__.isValidElement)(activeDot)) {
        // @ts-expect-error we're improperly typing events
        dot = /*#__PURE__*/ (0,react__WEBPACK_IMPORTED_MODULE_0__.cloneElement)(activeDot, dotProps);
    } else if (typeof activeDot === 'function') {
        dot = activeDot(dotProps);
    } else {
        dot = /*#__PURE__*/ react__WEBPACK_IMPORTED_MODULE_0__.createElement(_shape_Dot__WEBPACK_IMPORTED_MODULE_3__/* .Dot */ .c, dotProps);
    }
    return /*#__PURE__*/ react__WEBPACK_IMPORTED_MODULE_0__.createElement(_container_Layer__WEBPACK_IMPORTED_MODULE_4__/* .Layer */ .W, {
        className: "recharts-active-dot",
        clipPath: clipPath
    }, dot);
};
function ActivePoints(_ref2) {
    var { points, mainColor, activeDot, itemDataKey, clipPath, zIndex = _zIndex_DefaultZIndexes__WEBPACK_IMPORTED_MODULE_5__/* .DefaultZIndexes */ .I.activeDot } = _ref2;
    var activeTooltipIndex = (0,_state_hooks__WEBPACK_IMPORTED_MODULE_6__/* .useAppSelector */ .G)(_state_selectors_tooltipSelectors__WEBPACK_IMPORTED_MODULE_7__/* .selectActiveTooltipIndex */ .A2);
    var activeDataPoints = (0,_hooks__WEBPACK_IMPORTED_MODULE_8__/* .useActiveTooltipDataPoints */ .EI)();
    if (points == null || activeDataPoints == null) {
        return null;
    }
    var activePoint = points.find((p)=>activeDataPoints.includes(p.payload));
    if ((0,_util_DataUtils__WEBPACK_IMPORTED_MODULE_9__/* .isNullish */ .uy)(activePoint)) {
        return null;
    }
    return /*#__PURE__*/ react__WEBPACK_IMPORTED_MODULE_0__.createElement(_zIndex_ZIndexLayer__WEBPACK_IMPORTED_MODULE_10__/* .ZIndexLayer */ .g, {
        zIndex: zIndex
    }, /*#__PURE__*/ react__WEBPACK_IMPORTED_MODULE_0__.createElement(ActivePoint, {
        point: activePoint,
        childIndex: Number(activeTooltipIndex),
        mainColor: mainColor,
        dataKey: itemDataKey,
        activeDot: activeDot,
        clipPath: clipPath
    }));
}


/***/ }),

/***/ 46649:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "._180u4of0 {\n  background: var(--_19ez2rz7);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: var(--_19ez2rz38);\n  padding: var(--_19ez2rz31) var(--_19ez2rz31);\n}\n._180u4of1 {\n  font-size: var(--_19ez2rz3b);\n  font-weight: var(--_19ez2rz3h);\n  color: var(--_19ez2rz0);\n  margin-bottom: var(--_19ez2rz30);\n}\n._180u4of2 {\n  width: 100%;\n  height: 240px;\n}\n._180u4of3 {\n  display: flex;\n  align-items: center;\n  justify-content: center;\n  height: 240px;\n  color: var(--_19ez2rz2);\n  font-size: var(--_19ez2rz3b);\n}\n._180u4of4 {\n  display: flex;\n  flex-wrap: wrap;\n  gap: var(--_19ez2rz30);\n  margin-top: var(--_19ez2rz2z);\n}\n._180u4of5 {\n  display: flex;\n  align-items: center;\n  gap: var(--_19ez2rz2y);\n  font-size: var(--_19ez2rz3a);\n  color: var(--_19ez2rz1);\n}\n._180u4of6 {\n  width: 10px;\n  height: 10px;\n  border-radius: 2px;\n  flex-shrink: 0;\n}\n._180u4of7 {\n  color: var(--_19ez2rzz);\n  font-style: italic;\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 52236:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   B: () => (/* binding */ selectYAxisIdFromGraphicalItemId),
/* harmony export */   W: () => (/* binding */ selectXAxisIdFromGraphicalItemId)
/* harmony export */ });
/* harmony import */ var _cartesianAxisSlice__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(30704);

function selectXAxisIdFromGraphicalItemId(state, id) {
    var _state$graphicalItems, _state$graphicalItems2;
    return (_state$graphicalItems = (_state$graphicalItems2 = state.graphicalItems.cartesianItems.find((item)=>item.id === id)) === null || _state$graphicalItems2 === void 0 ? void 0 : _state$graphicalItems2.xAxisId) !== null && _state$graphicalItems !== void 0 ? _state$graphicalItems : _cartesianAxisSlice__WEBPACK_IMPORTED_MODULE_0__/* .defaultAxisId */ .W3;
}
function selectYAxisIdFromGraphicalItemId(state, id) {
    var _state$graphicalItems3, _state$graphicalItems4;
    return (_state$graphicalItems3 = (_state$graphicalItems4 = state.graphicalItems.cartesianItems.find((item)=>item.id === id)) === null || _state$graphicalItems4 === void 0 ? void 0 : _state$graphicalItems4.yAxisId) !== null && _state$graphicalItems3 !== void 0 ? _state$graphicalItems3 : _cartesianAxisSlice__WEBPACK_IMPORTED_MODULE_0__/* .defaultAxisId */ .W3;
}


/***/ }),

/***/ 58497:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   n: () => (/* binding */ Dots)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(88782);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var clsx__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(37595);
/* harmony import */ var _shape_Dot__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(3516);
/* harmony import */ var _container_Layer__WEBPACK_IMPORTED_MODULE_7__ = __webpack_require__(44084);
/* harmony import */ var _util_ReactUtils__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(75976);
/* harmony import */ var _util_svgPropertiesAndEvents__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(30386);
/* harmony import */ var _zIndex_ZIndexLayer__WEBPACK_IMPORTED_MODULE_6__ = __webpack_require__(80731);
/* harmony import */ var _zIndex_DefaultZIndexes__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(71115);
var _excluded = [
    "points"
];
function ownKeys(e, r) {
    var t = Object.keys(e);
    if (Object.getOwnPropertySymbols) {
        var o = Object.getOwnPropertySymbols(e);
        r && (o = o.filter(function(r) {
            return Object.getOwnPropertyDescriptor(e, r).enumerable;
        })), t.push.apply(t, o);
    }
    return t;
}
function _objectSpread(e) {
    for(var r = 1; r < arguments.length; r++){
        var t = null != arguments[r] ? arguments[r] : {};
        r % 2 ? ownKeys(Object(t), !0).forEach(function(r) {
            _defineProperty(e, r, t[r]);
        }) : Object.getOwnPropertyDescriptors ? Object.defineProperties(e, Object.getOwnPropertyDescriptors(t)) : ownKeys(Object(t)).forEach(function(r) {
            Object.defineProperty(e, r, Object.getOwnPropertyDescriptor(t, r));
        });
    }
    return e;
}
function _defineProperty(e, r, t) {
    return (r = _toPropertyKey(r)) in e ? Object.defineProperty(e, r, {
        value: t,
        enumerable: !0,
        configurable: !0,
        writable: !0
    }) : e[r] = t, e;
}
function _toPropertyKey(t) {
    var i = _toPrimitive(t, "string");
    return "symbol" == typeof i ? i : i + "";
}
function _toPrimitive(t, r) {
    if ("object" != typeof t || !t) return t;
    var e = t[Symbol.toPrimitive];
    if (void 0 !== e) {
        var i = e.call(t, r || "default");
        if ("object" != typeof i) return i;
        throw new TypeError("@@toPrimitive must return a primitive value.");
    }
    return ("string" === r ? String : Number)(t);
}
function _extends() {
    return _extends = Object.assign ? Object.assign.bind() : function(n) {
        for(var e = 1; e < arguments.length; e++){
            var t = arguments[e];
            for(var r in t)({}).hasOwnProperty.call(t, r) && (n[r] = t[r]);
        }
        return n;
    }, _extends.apply(null, arguments);
}
function _objectWithoutProperties(e, t) {
    if (null == e) return {};
    var o, r, i = _objectWithoutPropertiesLoose(e, t);
    if (Object.getOwnPropertySymbols) {
        var n = Object.getOwnPropertySymbols(e);
        for(r = 0; r < n.length; r++)o = n[r], -1 === t.indexOf(o) && ({}).propertyIsEnumerable.call(e, o) && (i[o] = e[o]);
    }
    return i;
}
function _objectWithoutPropertiesLoose(r, e) {
    if (null == r) return {};
    var t = {};
    for(var n in r)if (({}).hasOwnProperty.call(r, n)) {
        if (-1 !== e.indexOf(n)) continue;
        t[n] = r[n];
    }
    return t;
}









function DotItem(_ref) {
    var { option, dotProps, className } = _ref;
    if (/*#__PURE__*/ (0,react__WEBPACK_IMPORTED_MODULE_0__.isValidElement)(option)) {
        // @ts-expect-error we can't type check element cloning properly
        return /*#__PURE__*/ (0,react__WEBPACK_IMPORTED_MODULE_0__.cloneElement)(option, dotProps);
    }
    if (typeof option === 'function') {
        return option(dotProps);
    }
    var finalClassName = (0,clsx__WEBPACK_IMPORTED_MODULE_1__/* .clsx */ .$)(className, typeof option !== 'boolean' ? option.className : '');
    var _ref2 = dotProps !== null && dotProps !== void 0 ? dotProps : {}, { points } = _ref2, props = _objectWithoutProperties(_ref2, _excluded);
    return /*#__PURE__*/ react__WEBPACK_IMPORTED_MODULE_0__.createElement(_shape_Dot__WEBPACK_IMPORTED_MODULE_2__/* .Dot */ .c, _extends({}, props, {
        className: finalClassName
    }));
}
function shouldRenderDots(points, dot) {
    if (points == null) {
        return false;
    }
    if (dot) {
        return true;
    }
    return points.length === 1;
}
function Dots(_ref3) {
    var { points, dot, className, dotClassName, dataKey, baseProps, needClip, clipPathId, zIndex = _zIndex_DefaultZIndexes__WEBPACK_IMPORTED_MODULE_3__/* .DefaultZIndexes */ .I.scatter } = _ref3;
    if (!shouldRenderDots(points, dot)) {
        return null;
    }
    var clipDot = (0,_util_ReactUtils__WEBPACK_IMPORTED_MODULE_4__/* .isClipDot */ .y$)(dot);
    var customDotProps = (0,_util_svgPropertiesAndEvents__WEBPACK_IMPORTED_MODULE_5__/* .svgPropertiesAndEventsFromUnknown */ .y)(dot);
    var dots = points.map((entry, i)=>{
        var _entry$x, _entry$y;
        var dotProps = _objectSpread(_objectSpread(_objectSpread({
            r: 3
        }, baseProps), customDotProps), {}, {
            index: i,
            cx: (_entry$x = entry.x) !== null && _entry$x !== void 0 ? _entry$x : undefined,
            cy: (_entry$y = entry.y) !== null && _entry$y !== void 0 ? _entry$y : undefined,
            dataKey,
            value: entry.value,
            payload: entry.payload,
            points
        });
        return /*#__PURE__*/ react__WEBPACK_IMPORTED_MODULE_0__.createElement(DotItem, {
            key: "dot-".concat(i),
            option: dot,
            dotProps: dotProps,
            className: dotClassName
        });
    });
    var layerProps = {};
    if (needClip && clipPathId != null) {
        layerProps.clipPath = "url(#clipPath-".concat(clipDot ? '' : 'dots-').concat(clipPathId, ")");
    }
    return /*#__PURE__*/ react__WEBPACK_IMPORTED_MODULE_0__.createElement(_zIndex_ZIndexLayer__WEBPACK_IMPORTED_MODULE_6__/* .ZIndexLayer */ .g, {
        zIndex: zIndex
    }, /*#__PURE__*/ react__WEBPACK_IMPORTED_MODULE_0__.createElement(_container_Layer__WEBPACK_IMPORTED_MODULE_7__/* .Layer */ .W, _extends({
        className: className
    }, layerProps), dots));
}


/***/ }),

/***/ 75640:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  ModelOverTimeChart: () => (/* binding */ ModelOverTimeChart)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/component/ResponsiveContainer.js + 1 modules
var ResponsiveContainer = __webpack_require__(51450);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/optionsSlice.js
var optionsSlice = __webpack_require__(81295);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/chart/CartesianChart.js + 30 modules
var CartesianChart = __webpack_require__(20141);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/chart/AreaChart.js




var allowedTooltipTypes = [
    'axis'
];
/**
 * @consumes ResponsiveContainerContext
 * @provides CartesianViewBoxContext
 * @provides CartesianChartContext
 */ var AreaChart = /*#__PURE__*/ (0,react.forwardRef)((props, ref)=>{
    return /*#__PURE__*/ react.createElement(CartesianChart/* CartesianChart */.P, {
        chartName: "AreaChart",
        defaultTooltipEventType: "axis",
        validateTooltipEventTypes: allowedTooltipTypes,
        tooltipPayloadSearcher: optionsSlice/* arrayTooltipSearcher */.uN,
        categoricalChartProps: props,
        ref: ref
    });
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/cartesian/CartesianGrid.js
var CartesianGrid = __webpack_require__(7890);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/cartesian/XAxis.js
var XAxis = __webpack_require__(21529);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/cartesian/YAxis.js
var YAxis = __webpack_require__(16990);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/component/Tooltip.js + 11 modules
var Tooltip = __webpack_require__(9205);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/clsx@2.1.1/node_modules/clsx/dist/clsx.mjs
var clsx = __webpack_require__(37595);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/shape/Curve.js + 13 modules
var Curve = __webpack_require__(11572);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/container/Layer.js
var Layer = __webpack_require__(44084);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/component/LabelList.js
var LabelList = __webpack_require__(65379);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/component/Dots.js
var Dots = __webpack_require__(58497);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/DataUtils.js
var DataUtils = __webpack_require__(36223);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/ChartUtils.js + 6 modules
var ChartUtils = __webpack_require__(26533);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/ReactUtils.js
var ReactUtils = __webpack_require__(75976);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/component/ActivePoints.js
var ActivePoints = __webpack_require__(25928);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/SetTooltipEntrySettings.js
var SetTooltipEntrySettings = __webpack_require__(25019);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/cartesian/GraphicalItemClipPath.js
var GraphicalItemClipPath = __webpack_require__(71499);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/reselect@5.1.1/node_modules/reselect/dist/reselect.mjs
var reselect = __webpack_require__(48559);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/selectors/axisSelectors.js + 8 modules
var axisSelectors = __webpack_require__(66882);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/context/chartLayoutContext.js
var chartLayoutContext = __webpack_require__(87644);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/selectors/dataSelectors.js
var dataSelectors = __webpack_require__(92396);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/stacks/getStackSeriesIdentifier.js
var getStackSeriesIdentifier = __webpack_require__(13662);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/selectors/rootPropsSelectors.js
var rootPropsSelectors = __webpack_require__(82676);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/selectors/graphicalItemSelectors.js
var graphicalItemSelectors = __webpack_require__(52236);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/selectors/areaSelectors.js









var selectXAxisWithScale = (state, graphicalItemId, isPanorama)=>(0,axisSelectors/* selectAxisWithScale */.Gx)(state, 'xAxis', (0,graphicalItemSelectors/* selectXAxisIdFromGraphicalItemId */.W)(state, graphicalItemId), isPanorama);
var selectXAxisTicks = (state, graphicalItemId, isPanorama)=>(0,axisSelectors/* selectTicksOfGraphicalItem */.CR)(state, 'xAxis', (0,graphicalItemSelectors/* selectXAxisIdFromGraphicalItemId */.W)(state, graphicalItemId), isPanorama);
var selectYAxisWithScale = (state, graphicalItemId, isPanorama)=>(0,axisSelectors/* selectAxisWithScale */.Gx)(state, 'yAxis', (0,graphicalItemSelectors/* selectYAxisIdFromGraphicalItemId */.B)(state, graphicalItemId), isPanorama);
var selectYAxisTicks = (state, graphicalItemId, isPanorama)=>(0,axisSelectors/* selectTicksOfGraphicalItem */.CR)(state, 'yAxis', (0,graphicalItemSelectors/* selectYAxisIdFromGraphicalItemId */.B)(state, graphicalItemId), isPanorama);
var selectBandSize = (0,reselect/* createSelector */.Mz)([
    chartLayoutContext/* selectChartLayout */.fz,
    selectXAxisWithScale,
    selectYAxisWithScale,
    selectXAxisTicks,
    selectYAxisTicks
], (layout, xAxis, yAxis, xAxisTicks, yAxisTicks)=>{
    if ((0,ChartUtils/* isCategoricalAxis */._L)(layout, 'xAxis')) {
        return (0,ChartUtils/* getBandSizeOfAxis */.Hj)(xAxis, xAxisTicks, false);
    }
    return (0,ChartUtils/* getBandSizeOfAxis */.Hj)(yAxis, yAxisTicks, false);
});
var pickAreaId = (_state, id)=>id;
/*
 * There is a race condition problem because we read some data from props and some from the state.
 * The state is updated through a dispatch and is one render behind,
 * and so we have this weird one tick render where the displayedData in one selector have the old dataKey
 * but the new dataKey in another selector.
 *
 * A proper fix is to either move everything into the state, or read the dataKey always from props
 * - but this is a smaller change.
 */ var selectSynchronisedAreaSettings = (0,reselect/* createSelector */.Mz)([
    axisSelectors/* selectUnfilteredCartesianItems */.ld,
    pickAreaId
], (graphicalItems, id)=>graphicalItems.filter((item)=>item.type === 'area').find((item)=>item.id === id));
var selectNumericalAxisType = (state)=>{
    var layout = (0,chartLayoutContext/* selectChartLayout */.fz)(state);
    var isXAxisCategorical = (0,ChartUtils/* isCategoricalAxis */._L)(layout, 'xAxis');
    return isXAxisCategorical ? 'yAxis' : 'xAxis';
};
var selectNumericalAxisIdFromGraphicalItemId = (state, graphicalItemId)=>{
    var axisType = selectNumericalAxisType(state);
    if (axisType === 'yAxis') {
        return (0,graphicalItemSelectors/* selectYAxisIdFromGraphicalItemId */.B)(state, graphicalItemId);
    }
    return (0,graphicalItemSelectors/* selectXAxisIdFromGraphicalItemId */.W)(state, graphicalItemId);
};
var selectNumericalAxisStackGroups = (state, graphicalItemId, isPanorama)=>(0,axisSelectors/* selectStackGroups */.TC)(state, selectNumericalAxisType(state), selectNumericalAxisIdFromGraphicalItemId(state, graphicalItemId), isPanorama);
var selectGraphicalItemStackedData = (0,reselect/* createSelector */.Mz)([
    selectSynchronisedAreaSettings,
    selectNumericalAxisStackGroups
], (areaSettings, stackGroups)=>{
    var _stackGroups$stackId;
    if (areaSettings == null || stackGroups == null) {
        return undefined;
    }
    var { stackId } = areaSettings;
    var stackSeriesIdentifier = (0,getStackSeriesIdentifier/* getStackSeriesIdentifier */.x)(areaSettings);
    if (stackId == null || stackSeriesIdentifier == null) {
        return undefined;
    }
    var groups = (_stackGroups$stackId = stackGroups[stackId]) === null || _stackGroups$stackId === void 0 ? void 0 : _stackGroups$stackId.stackedData;
    var found = groups === null || groups === void 0 ? void 0 : groups.find((v)=>v.key === stackSeriesIdentifier);
    if (found == null) {
        return undefined;
    }
    return found.map((item)=>[
            item[0],
            item[1]
        ]);
});
var selectArea = (0,reselect/* createSelector */.Mz)([
    chartLayoutContext/* selectChartLayout */.fz,
    selectXAxisWithScale,
    selectYAxisWithScale,
    selectXAxisTicks,
    selectYAxisTicks,
    selectGraphicalItemStackedData,
    dataSelectors/* selectChartDataWithIndexesIfNotInPanoramaPosition3 */.rN,
    selectBandSize,
    selectSynchronisedAreaSettings,
    rootPropsSelectors/* selectChartBaseValue */.vm
], (layout, xAxis, yAxis, xAxisTicks, yAxisTicks, stackedData, _ref, bandSize, areaSettings, chartBaseValue)=>{
    var { chartData, dataStartIndex, dataEndIndex } = _ref;
    if (areaSettings == null || layout !== 'horizontal' && layout !== 'vertical' || xAxis == null || yAxis == null || xAxisTicks == null || yAxisTicks == null || xAxisTicks.length === 0 || yAxisTicks.length === 0 || bandSize == null) {
        return undefined;
    }
    var { data } = areaSettings;
    var displayedData;
    if (data && data.length > 0) {
        displayedData = data;
    } else {
        displayedData = chartData === null || chartData === void 0 ? void 0 : chartData.slice(dataStartIndex, dataEndIndex + 1);
    }
    if (displayedData == null) {
        return undefined;
    }
    return computeArea({
        layout,
        xAxis,
        yAxis,
        xAxisTicks,
        yAxisTicks,
        dataStartIndex,
        areaSettings,
        stackedData,
        displayedData,
        chartBaseValue,
        bandSize
    });
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/context/PanoramaContext.js
var PanoramaContext = __webpack_require__(73327);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/selectors/selectors.js + 1 modules
var selectors = __webpack_require__(42753);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/SetLegendPayload.js
var SetLegendPayload = __webpack_require__(49190);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/hooks.js
var hooks = __webpack_require__(53151);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/useAnimationId.js
var useAnimationId = __webpack_require__(87810);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/resolveDefaultProps.js
var resolveDefaultProps = __webpack_require__(25055);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/isWellBehavedNumber.js
var isWellBehavedNumber = __webpack_require__(38490);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/hooks.js + 2 modules
var es6_hooks = __webpack_require__(48917);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/context/RegisterGraphicalItemId.js + 2 modules
var RegisterGraphicalItemId = __webpack_require__(96228);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/SetGraphicalItem.js
var SetGraphicalItem = __webpack_require__(5021);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/svgPropertiesNoEvents.js
var svgPropertiesNoEvents = __webpack_require__(40959);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/animation/JavascriptAnimate.js + 6 modules
var JavascriptAnimate = __webpack_require__(70082);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/getRadiusAndStrokeWidthFromDot.js
var getRadiusAndStrokeWidthFromDot = __webpack_require__(86104);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/svgPropertiesAndEvents.js
var svgPropertiesAndEvents = __webpack_require__(30386);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/zIndex/ZIndexLayer.js
var ZIndexLayer = __webpack_require__(80731);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/zIndex/DefaultZIndexes.js
var DefaultZIndexes = __webpack_require__(71115);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/propsAreEqual.js
var propsAreEqual = __webpack_require__(7050);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/cartesian/Area.js
var _excluded = [
    "id"
], _excluded2 = [
    "activeDot",
    "animationBegin",
    "animationDuration",
    "animationEasing",
    "connectNulls",
    "dot",
    "fill",
    "fillOpacity",
    "hide",
    "isAnimationActive",
    "legendType",
    "stroke",
    "xAxisId",
    "yAxisId"
];
function _extends() {
    return _extends = Object.assign ? Object.assign.bind() : function(n) {
        for(var e = 1; e < arguments.length; e++){
            var t = arguments[e];
            for(var r in t)({}).hasOwnProperty.call(t, r) && (n[r] = t[r]);
        }
        return n;
    }, _extends.apply(null, arguments);
}
function _objectWithoutProperties(e, t) {
    if (null == e) return {};
    var o, r, i = _objectWithoutPropertiesLoose(e, t);
    if (Object.getOwnPropertySymbols) {
        var n = Object.getOwnPropertySymbols(e);
        for(r = 0; r < n.length; r++)o = n[r], -1 === t.indexOf(o) && ({}).propertyIsEnumerable.call(e, o) && (i[o] = e[o]);
    }
    return i;
}
function _objectWithoutPropertiesLoose(r, e) {
    if (null == r) return {};
    var t = {};
    for(var n in r)if (({}).hasOwnProperty.call(r, n)) {
        if (-1 !== e.indexOf(n)) continue;
        t[n] = r[n];
    }
    return t;
}
function ownKeys(e, r) {
    var t = Object.keys(e);
    if (Object.getOwnPropertySymbols) {
        var o = Object.getOwnPropertySymbols(e);
        r && (o = o.filter(function(r) {
            return Object.getOwnPropertyDescriptor(e, r).enumerable;
        })), t.push.apply(t, o);
    }
    return t;
}
function _objectSpread(e) {
    for(var r = 1; r < arguments.length; r++){
        var t = null != arguments[r] ? arguments[r] : {};
        r % 2 ? ownKeys(Object(t), !0).forEach(function(r) {
            _defineProperty(e, r, t[r]);
        }) : Object.getOwnPropertyDescriptors ? Object.defineProperties(e, Object.getOwnPropertyDescriptors(t)) : ownKeys(Object(t)).forEach(function(r) {
            Object.defineProperty(e, r, Object.getOwnPropertyDescriptor(t, r));
        });
    }
    return e;
}
function _defineProperty(e, r, t) {
    return (r = _toPropertyKey(r)) in e ? Object.defineProperty(e, r, {
        value: t,
        enumerable: !0,
        configurable: !0,
        writable: !0
    }) : e[r] = t, e;
}
function _toPropertyKey(t) {
    var i = _toPrimitive(t, "string");
    return "symbol" == typeof i ? i : i + "";
}
function _toPrimitive(t, r) {
    if ("object" != typeof t || !t) return t;
    var e = t[Symbol.toPrimitive];
    if (void 0 !== e) {
        var i = e.call(t, r || "default");
        if ("object" != typeof i) return i;
        throw new TypeError("@@toPrimitive must return a primitive value.");
    }
    return ("string" === r ? String : Number)(t);
}
































/**
 * @inline
 */ /**
 * Our base value array has payload in it, and we expose it externally too.
 */ /**
 * Internal props, combination of external props + defaultProps + private Recharts state
 */ /**
 * External props, intended for end users to fill in
 */ /**
 * Because of naming conflict, we are forced to ignore certain (valid) SVG attributes.
 */ function getLegendItemColor(stroke, fill) {
    return stroke && stroke !== 'none' ? stroke : fill;
}
var computeLegendPayloadFromAreaData = (props)=>{
    var { dataKey, name, stroke, fill, legendType, hide } = props;
    return [
        {
            inactive: hide,
            dataKey,
            type: legendType,
            color: getLegendItemColor(stroke, fill),
            value: (0,ChartUtils/* getTooltipNameProp */.uM)(name, dataKey),
            payload: props
        }
    ];
};
var SetAreaTooltipEntrySettings = /*#__PURE__*/ react.memo((_ref)=>{
    var { dataKey, data, stroke, strokeWidth, fill, name, hide, unit, tooltipType, id } = _ref;
    var tooltipEntrySettings = {
        dataDefinedOnItem: data,
        getPosition: DataUtils/* noop */.lQ,
        settings: {
            stroke,
            strokeWidth,
            fill,
            dataKey,
            nameKey: undefined,
            name: (0,ChartUtils/* getTooltipNameProp */.uM)(name, dataKey),
            hide,
            type: tooltipType,
            color: getLegendItemColor(stroke, fill),
            unit,
            graphicalItemId: id
        }
    };
    return /*#__PURE__*/ react.createElement(SetTooltipEntrySettings/* SetTooltipEntrySettings */.r, {
        tooltipEntrySettings: tooltipEntrySettings
    });
});
function AreaDotsWrapper(_ref2) {
    var { clipPathId, points, props } = _ref2;
    var { needClip, dot, dataKey } = props;
    var areaProps = (0,svgPropertiesNoEvents/* svgPropertiesNoEvents */.uZ)(props);
    return /*#__PURE__*/ react.createElement(Dots/* Dots */.n, {
        points: points,
        dot: dot,
        className: "recharts-area-dots",
        dotClassName: "recharts-area-dot",
        dataKey: dataKey,
        baseProps: areaProps,
        needClip: needClip,
        clipPathId: clipPathId
    });
}
function AreaLabelListProvider(_ref3) {
    var { showLabels, children, points } = _ref3;
    var labelListEntries = points.map((point)=>{
        var _point$x, _point$y;
        var viewBox = {
            x: (_point$x = point.x) !== null && _point$x !== void 0 ? _point$x : 0,
            y: (_point$y = point.y) !== null && _point$y !== void 0 ? _point$y : 0,
            width: 0,
            lowerWidth: 0,
            upperWidth: 0,
            height: 0
        };
        return _objectSpread(_objectSpread({}, viewBox), {}, {
            value: point.value,
            payload: point.payload,
            parentViewBox: undefined,
            viewBox,
            fill: undefined
        });
    });
    return /*#__PURE__*/ react.createElement(LabelList/* CartesianLabelListContextProvider */.h8, {
        value: showLabels ? labelListEntries : undefined
    }, children);
}
function StaticArea(_ref4) {
    var { points, baseLine, needClip, clipPathId, props } = _ref4;
    var { layout, type, stroke, connectNulls, isRange } = props;
    var { id } = props, propsWithoutId = _objectWithoutProperties(props, _excluded);
    var allOtherProps = (0,svgPropertiesNoEvents/* svgPropertiesNoEvents */.uZ)(propsWithoutId);
    var propsWithEvents = (0,svgPropertiesAndEvents/* svgPropertiesAndEvents */.a)(propsWithoutId);
    return /*#__PURE__*/ react.createElement(react.Fragment, null, (points === null || points === void 0 ? void 0 : points.length) > 1 && /*#__PURE__*/ react.createElement(Layer/* Layer */.W, {
        clipPath: needClip ? "url(#clipPath-".concat(clipPathId, ")") : undefined
    }, /*#__PURE__*/ react.createElement(Curve/* Curve */.Ip, _extends({}, propsWithEvents, {
        id: id,
        points: points,
        connectNulls: connectNulls,
        type: type,
        baseLine: baseLine,
        layout: layout,
        stroke: "none",
        className: "recharts-area-area"
    })), stroke !== 'none' && /*#__PURE__*/ react.createElement(Curve/* Curve */.Ip, _extends({}, allOtherProps, {
        className: "recharts-area-curve",
        layout: layout,
        type: type,
        connectNulls: connectNulls,
        fill: "none",
        points: points
    })), stroke !== 'none' && isRange && Array.isArray(baseLine) && /*#__PURE__*/ react.createElement(Curve/* Curve */.Ip, _extends({}, allOtherProps, {
        className: "recharts-area-curve",
        layout: layout,
        type: type,
        connectNulls: connectNulls,
        fill: "none",
        points: baseLine
    }))), /*#__PURE__*/ react.createElement(AreaDotsWrapper, {
        points: points,
        props: propsWithoutId,
        clipPathId: clipPathId
    }));
}
function VerticalRect(_ref5) {
    var _points$, _points;
    var { alpha, baseLine, points, strokeWidth } = _ref5;
    var startY = (_points$ = points[0]) === null || _points$ === void 0 ? void 0 : _points$.y;
    var endY = (_points = points[points.length - 1]) === null || _points === void 0 ? void 0 : _points.y;
    if (!(0,isWellBehavedNumber/* isWellBehavedNumber */.H)(startY) || !(0,isWellBehavedNumber/* isWellBehavedNumber */.H)(endY)) {
        return null;
    }
    var height = alpha * Math.abs(startY - endY);
    var maxX = Math.max(...points.map((entry)=>entry.x || 0));
    if ((0,DataUtils/* isNumber */.Et)(baseLine)) {
        maxX = Math.max(baseLine, maxX);
    } else if (baseLine && Array.isArray(baseLine) && baseLine.length) {
        maxX = Math.max(...baseLine.map((entry)=>entry.x || 0), maxX);
    }
    if ((0,DataUtils/* isNumber */.Et)(maxX)) {
        return /*#__PURE__*/ react.createElement("rect", {
            x: 0,
            y: startY < endY ? startY : startY - height,
            width: maxX + (strokeWidth ? parseInt("".concat(strokeWidth), 10) : 1),
            height: Math.floor(height)
        });
    }
    return null;
}
function HorizontalRect(_ref6) {
    var _points$2, _points2;
    var { alpha, baseLine, points, strokeWidth } = _ref6;
    var startX = (_points$2 = points[0]) === null || _points$2 === void 0 ? void 0 : _points$2.x;
    var endX = (_points2 = points[points.length - 1]) === null || _points2 === void 0 ? void 0 : _points2.x;
    if (!(0,isWellBehavedNumber/* isWellBehavedNumber */.H)(startX) || !(0,isWellBehavedNumber/* isWellBehavedNumber */.H)(endX)) {
        return null;
    }
    var width = alpha * Math.abs(startX - endX);
    var maxY = Math.max(...points.map((entry)=>entry.y || 0));
    if ((0,DataUtils/* isNumber */.Et)(baseLine)) {
        maxY = Math.max(baseLine, maxY);
    } else if (baseLine && Array.isArray(baseLine) && baseLine.length) {
        maxY = Math.max(...baseLine.map((entry)=>entry.y || 0), maxY);
    }
    if ((0,DataUtils/* isNumber */.Et)(maxY)) {
        return /*#__PURE__*/ react.createElement("rect", {
            x: startX < endX ? startX : startX - width,
            y: 0,
            width: width,
            height: Math.floor(maxY + (strokeWidth ? parseInt("".concat(strokeWidth), 10) : 1))
        });
    }
    return null;
}
function ClipRect(_ref7) {
    var { alpha, layout, points, baseLine, strokeWidth } = _ref7;
    if (layout === 'vertical') {
        return /*#__PURE__*/ react.createElement(VerticalRect, {
            alpha: alpha,
            points: points,
            baseLine: baseLine,
            strokeWidth: strokeWidth
        });
    }
    return /*#__PURE__*/ react.createElement(HorizontalRect, {
        alpha: alpha,
        points: points,
        baseLine: baseLine,
        strokeWidth: strokeWidth
    });
}
function AreaWithAnimation(_ref8) {
    var { needClip, clipPathId, props, previousPointsRef, previousBaselineRef } = _ref8;
    var { points, baseLine, isAnimationActive, animationBegin, animationDuration, animationEasing, onAnimationStart, onAnimationEnd } = props;
    var animationInput = (0,react.useMemo)(()=>({
            points,
            baseLine
        }), [
        points,
        baseLine
    ]);
    var animationId = (0,useAnimationId/* useAnimationId */.n)(animationInput, 'recharts-area-');
    var layout = (0,chartLayoutContext/* useCartesianChartLayout */.kz)();
    var [isAnimating, setIsAnimating] = (0,react.useState)(false);
    var showLabels = !isAnimating;
    var handleAnimationEnd = (0,react.useCallback)(()=>{
        if (typeof onAnimationEnd === 'function') {
            onAnimationEnd();
        }
        setIsAnimating(false);
    }, [
        onAnimationEnd
    ]);
    var handleAnimationStart = (0,react.useCallback)(()=>{
        if (typeof onAnimationStart === 'function') {
            onAnimationStart();
        }
        setIsAnimating(true);
    }, [
        onAnimationStart
    ]);
    if (layout == null) {
        return null;
    }
    var prevPoints = previousPointsRef.current;
    var prevBaseLine = previousBaselineRef.current;
    return /*#__PURE__*/ react.createElement(AreaLabelListProvider, {
        showLabels: showLabels,
        points: points
    }, props.children, /*#__PURE__*/ react.createElement(JavascriptAnimate/* JavascriptAnimate */.J, {
        animationId: animationId,
        begin: animationBegin,
        duration: animationDuration,
        isActive: isAnimationActive,
        easing: animationEasing,
        onAnimationEnd: handleAnimationEnd,
        onAnimationStart: handleAnimationStart,
        key: animationId
    }, (t)=>{
        if (prevPoints) {
            var prevPointsDiffFactor = prevPoints.length / points.length;
            var stepPoints = /*
       * Here it is important that at the very end of the animation, on the last frame,
       * we render the original points without any interpolation.
       * This is needed because the code above is checking for reference equality to decide if the animation should run
       * and if we create a new array instance (even if the numbers were the same)
       * then we would break animations.
       */ t === 1 ? points : points.map((entry, index)=>{
                var prevPointIndex = Math.floor(index * prevPointsDiffFactor);
                if (prevPoints[prevPointIndex]) {
                    var prev = prevPoints[prevPointIndex];
                    return _objectSpread(_objectSpread({}, entry), {}, {
                        x: (0,DataUtils/* interpolate */.GW)(prev.x, entry.x, t),
                        y: (0,DataUtils/* interpolate */.GW)(prev.y, entry.y, t)
                    });
                }
                return entry;
            });
            var stepBaseLine;
            if ((0,DataUtils/* isNumber */.Et)(baseLine)) {
                stepBaseLine = (0,DataUtils/* interpolate */.GW)(prevBaseLine, baseLine, t);
            } else if ((0,DataUtils/* isNullish */.uy)(baseLine) || (0,DataUtils/* isNan */.M8)(baseLine)) {
                stepBaseLine = (0,DataUtils/* interpolate */.GW)(prevBaseLine, 0, t);
            } else {
                stepBaseLine = baseLine.map((entry, index)=>{
                    var prevPointIndex = Math.floor(index * prevPointsDiffFactor);
                    if (Array.isArray(prevBaseLine) && prevBaseLine[prevPointIndex]) {
                        var prev = prevBaseLine[prevPointIndex];
                        return _objectSpread(_objectSpread({}, entry), {}, {
                            x: (0,DataUtils/* interpolate */.GW)(prev.x, entry.x, t),
                            y: (0,DataUtils/* interpolate */.GW)(prev.y, entry.y, t)
                        });
                    }
                    return entry;
                });
            }
            if (t > 0) {
                /*
         * We need to keep the refs in the parent component because we need to remember the last shape of the animation
         * even if AreaWithAnimation is unmounted as that happens when changing props.
         *
         * And we need to update the refs here because here is where the interpolation is computed.
         * Eslint doesn't like changing function arguments, but we need it so here is an eslint-disable.
         */ // eslint-disable-next-line no-param-reassign
                previousPointsRef.current = stepPoints;
                // eslint-disable-next-line no-param-reassign
                previousBaselineRef.current = stepBaseLine;
            }
            return /*#__PURE__*/ react.createElement(StaticArea, {
                points: stepPoints,
                baseLine: stepBaseLine,
                needClip: needClip,
                clipPathId: clipPathId,
                props: props
            });
        }
        if (t > 0) {
            // eslint-disable-next-line no-param-reassign
            previousPointsRef.current = points;
            // eslint-disable-next-line no-param-reassign
            previousBaselineRef.current = baseLine;
        }
        return /*#__PURE__*/ react.createElement(Layer/* Layer */.W, null, isAnimationActive && /*#__PURE__*/ react.createElement("defs", null, /*#__PURE__*/ react.createElement("clipPath", {
            id: "animationClipPath-".concat(clipPathId)
        }, /*#__PURE__*/ react.createElement(ClipRect, {
            alpha: t,
            points: points,
            baseLine: baseLine,
            layout: layout,
            strokeWidth: props.strokeWidth
        }))), /*#__PURE__*/ react.createElement(Layer/* Layer */.W, {
            clipPath: "url(#animationClipPath-".concat(clipPathId, ")")
        }, /*#__PURE__*/ react.createElement(StaticArea, {
            points: points,
            baseLine: baseLine,
            needClip: needClip,
            clipPathId: clipPathId,
            props: props
        })));
    }), /*#__PURE__*/ react.createElement(LabelList/* LabelListFromLabelProp */.qY, {
        label: props.label
    }));
}
/*
 * This components decides if the area should be animated or not.
 * It also holds the state of the animation.
 */ function RenderArea(_ref9) {
    var { needClip, clipPathId, props } = _ref9;
    /*
   * These two must be refs, not state!
   * Because we want to store the most recent shape of the animation in case we have to interrupt the animation;
   * that happens when user initiates another animation before the current one finishes.
   *
   * If this was a useState, then every step in the animation would trigger a re-render.
   * So, useRef it is.
   */ var previousPointsRef = (0,react.useRef)(null);
    var previousBaselineRef = (0,react.useRef)();
    return /*#__PURE__*/ react.createElement(AreaWithAnimation, {
        needClip: needClip,
        clipPathId: clipPathId,
        props: props,
        previousPointsRef: previousPointsRef,
        previousBaselineRef: previousBaselineRef
    });
}
class AreaWithState extends react.PureComponent {
    render() {
        var { hide, dot, points, className, top, left, needClip, xAxisId, yAxisId, width, height, id, baseLine, zIndex } = this.props;
        if (hide) {
            return null;
        }
        var layerClass = (0,clsx/* clsx */.$)('recharts-area', className);
        var clipPathId = id;
        var { r, strokeWidth } = (0,getRadiusAndStrokeWidthFromDot/* getRadiusAndStrokeWidthFromDot */.x)(dot);
        var clipDot = (0,ReactUtils/* isClipDot */.y$)(dot);
        var dotSize = r * 2 + strokeWidth;
        var activePointsClipPath = needClip ? "url(#clipPath-".concat(clipDot ? '' : 'dots-').concat(clipPathId, ")") : undefined;
        return /*#__PURE__*/ react.createElement(ZIndexLayer/* ZIndexLayer */.g, {
            zIndex: zIndex
        }, /*#__PURE__*/ react.createElement(Layer/* Layer */.W, {
            className: layerClass
        }, needClip && /*#__PURE__*/ react.createElement("defs", null, /*#__PURE__*/ react.createElement(GraphicalItemClipPath/* GraphicalItemClipPath */.Q, {
            clipPathId: clipPathId,
            xAxisId: xAxisId,
            yAxisId: yAxisId
        }), !clipDot && /*#__PURE__*/ react.createElement("clipPath", {
            id: "clipPath-dots-".concat(clipPathId)
        }, /*#__PURE__*/ react.createElement("rect", {
            x: left - dotSize / 2,
            y: top - dotSize / 2,
            width: width + dotSize,
            height: height + dotSize
        }))), /*#__PURE__*/ react.createElement(RenderArea, {
            needClip: needClip,
            clipPathId: clipPathId,
            props: this.props
        })), /*#__PURE__*/ react.createElement(ActivePoints/* ActivePoints */.W, {
            points: points,
            mainColor: getLegendItemColor(this.props.stroke, this.props.fill),
            itemDataKey: this.props.dataKey,
            activeDot: this.props.activeDot,
            clipPath: activePointsClipPath
        }), this.props.isRange && Array.isArray(baseLine) && /*#__PURE__*/ react.createElement(ActivePoints/* ActivePoints */.W, {
            points: baseLine,
            mainColor: getLegendItemColor(this.props.stroke, this.props.fill),
            itemDataKey: this.props.dataKey,
            activeDot: this.props.activeDot,
            clipPath: activePointsClipPath
        }));
    }
}
var defaultAreaProps = {
    activeDot: true,
    animationBegin: 0,
    animationDuration: 1500,
    animationEasing: 'ease',
    connectNulls: false,
    dot: false,
    fill: '#3182bd',
    fillOpacity: 0.6,
    hide: false,
    isAnimationActive: 'auto',
    legendType: 'line',
    stroke: '#3182bd',
    strokeWidth: 1,
    type: 'linear',
    label: false,
    xAxisId: 0,
    yAxisId: 0,
    zIndex: DefaultZIndexes/* DefaultZIndexes */.I.area
};
function AreaImpl(props) {
    var _useAppSelector;
    var { activeDot, animationBegin, animationDuration, animationEasing, connectNulls, dot, fill, fillOpacity, hide, isAnimationActive, legendType, stroke, xAxisId, yAxisId } = props, everythingElse = _objectWithoutProperties(props, _excluded2);
    var layout = (0,chartLayoutContext/* useChartLayout */.WX)();
    var chartName = (0,selectors/* useChartName */.fW)();
    var { needClip } = (0,GraphicalItemClipPath/* useNeedsClip */.l)(xAxisId, yAxisId);
    var isPanorama = (0,PanoramaContext/* useIsPanorama */.r)();
    var { points, isRange, baseLine } = (_useAppSelector = (0,hooks/* useAppSelector */.G)((state)=>selectArea(state, props.id, isPanorama))) !== null && _useAppSelector !== void 0 ? _useAppSelector : {};
    var plotArea = (0,es6_hooks/* usePlotArea */.oM)();
    if (layout !== 'horizontal' && layout !== 'vertical' || plotArea == null) {
        // Can't render Area in an unsupported layout
        return null;
    }
    if (chartName !== 'AreaChart' && chartName !== 'ComposedChart') {
        // There is nothing stopping us from rendering Area in other charts, except for historical reasons. Do we want to allow that?
        return null;
    }
    var { height, width, x: left, y: top } = plotArea;
    if (!points || !points.length) {
        return null;
    }
    return /*#__PURE__*/ react.createElement(AreaWithState, _extends({}, everythingElse, {
        activeDot: activeDot,
        animationBegin: animationBegin,
        animationDuration: animationDuration,
        animationEasing: animationEasing,
        baseLine: baseLine,
        connectNulls: connectNulls,
        dot: dot,
        fill: fill,
        fillOpacity: fillOpacity,
        height: height,
        hide: hide,
        layout: layout,
        isAnimationActive: isAnimationActive,
        isRange: isRange,
        legendType: legendType,
        needClip: needClip,
        points: points,
        stroke: stroke,
        width: width,
        left: left,
        top: top,
        xAxisId: xAxisId,
        yAxisId: yAxisId
    }));
}
var getBaseValue = (layout, chartBaseValue, itemBaseValue, xAxis, yAxis)=>{
    // The baseValue can be defined both on the AreaChart, and on the Area.
    // The value for the item takes precedence.
    var baseValue = itemBaseValue !== null && itemBaseValue !== void 0 ? itemBaseValue : chartBaseValue;
    if ((0,DataUtils/* isNumber */.Et)(baseValue)) {
        return baseValue;
    }
    var numericAxis = layout === 'horizontal' ? yAxis : xAxis;
    // @ts-expect-error d3scale .domain() returns unknown, Math.max expects number
    var domain = numericAxis.scale.domain();
    if (numericAxis.type === 'number') {
        var domainMax = Math.max(domain[0], domain[1]);
        var domainMin = Math.min(domain[0], domain[1]);
        if (baseValue === 'dataMin') {
            return domainMin;
        }
        if (baseValue === 'dataMax') {
            return domainMax;
        }
        return domainMax < 0 ? domainMax : Math.max(Math.min(domain[0], domain[1]), 0);
    }
    if (baseValue === 'dataMin') {
        return domain[0];
    }
    if (baseValue === 'dataMax') {
        return domain[1];
    }
    return domain[0];
};
function computeArea(_ref0) {
    var { areaSettings: { connectNulls, baseValue: itemBaseValue, dataKey }, stackedData, layout, chartBaseValue, xAxis, yAxis, displayedData, dataStartIndex, xAxisTicks, yAxisTicks, bandSize } = _ref0;
    var hasStack = stackedData && stackedData.length;
    var baseValue = getBaseValue(layout, chartBaseValue, itemBaseValue, xAxis, yAxis);
    var isHorizontalLayout = layout === 'horizontal';
    var isRange = false;
    var points = displayedData.map((entry, index)=>{
        var _valueAsArray$, _valueAsArray, _xAxis$scale$map;
        var valueAsArray;
        if (hasStack) {
            valueAsArray = stackedData[dataStartIndex + index];
        } else {
            var rawValue = (0,ChartUtils/* getValueByDataKey */.kr)(entry, dataKey);
            if (!Array.isArray(rawValue)) {
                valueAsArray = [
                    baseValue,
                    rawValue
                ];
            } else {
                valueAsArray = rawValue;
                isRange = true;
            }
        }
        var value1 = (_valueAsArray$ = (_valueAsArray = valueAsArray) === null || _valueAsArray === void 0 ? void 0 : _valueAsArray[1]) !== null && _valueAsArray$ !== void 0 ? _valueAsArray$ : null;
        var isBreakPoint = value1 == null || hasStack && !connectNulls && (0,ChartUtils/* getValueByDataKey */.kr)(entry, dataKey) == null;
        if (isHorizontalLayout) {
            var _yAxis$scale$map;
            return {
                x: (0,ChartUtils/* getCateCoordinateOfLine */.nb)({
                    axis: xAxis,
                    ticks: xAxisTicks,
                    bandSize,
                    entry,
                    index
                }),
                y: isBreakPoint ? null : (_yAxis$scale$map = yAxis.scale.map(value1)) !== null && _yAxis$scale$map !== void 0 ? _yAxis$scale$map : null,
                value: valueAsArray,
                payload: entry
            };
        }
        return {
            x: isBreakPoint ? null : (_xAxis$scale$map = xAxis.scale.map(value1)) !== null && _xAxis$scale$map !== void 0 ? _xAxis$scale$map : null,
            y: (0,ChartUtils/* getCateCoordinateOfLine */.nb)({
                axis: yAxis,
                ticks: yAxisTicks,
                bandSize,
                entry,
                index
            }),
            value: valueAsArray,
            payload: entry
        };
    });
    var baseLine;
    if (hasStack || isRange) {
        baseLine = points.map((entry)=>{
            var _xAxis$scale$map2;
            var x = Array.isArray(entry.value) ? entry.value[0] : null;
            if (isHorizontalLayout) {
                var _yAxis$scale$map2;
                return {
                    x: entry.x,
                    y: x != null && entry.y != null ? (_yAxis$scale$map2 = yAxis.scale.map(x)) !== null && _yAxis$scale$map2 !== void 0 ? _yAxis$scale$map2 : null : null,
                    payload: entry.payload
                };
            }
            return {
                x: x != null ? (_xAxis$scale$map2 = xAxis.scale.map(x)) !== null && _xAxis$scale$map2 !== void 0 ? _xAxis$scale$map2 : null : null,
                y: entry.y,
                payload: entry.payload
            };
        });
    } else {
        baseLine = isHorizontalLayout ? yAxis.scale.map(baseValue) : xAxis.scale.map(baseValue);
    }
    return {
        points,
        baseLine: baseLine !== null && baseLine !== void 0 ? baseLine : 0,
        isRange
    };
}
function AreaFn(outsideProps) {
    var props = (0,resolveDefaultProps/* resolveDefaultProps */.e)(outsideProps, defaultAreaProps);
    var isPanorama = (0,PanoramaContext/* useIsPanorama */.r)();
    // Report all props to Redux store first, before calling hooks, to avoid circular dependencies.
    return /*#__PURE__*/ react.createElement(RegisterGraphicalItemId/* RegisterGraphicalItemId */.x, {
        id: props.id,
        type: "area"
    }, (id)=>/*#__PURE__*/ react.createElement(react.Fragment, null, /*#__PURE__*/ react.createElement(SetLegendPayload/* SetLegendPayload */.A, {
            legendPayload: computeLegendPayloadFromAreaData(props)
        }), /*#__PURE__*/ react.createElement(SetAreaTooltipEntrySettings, {
            dataKey: props.dataKey,
            data: props.data,
            stroke: props.stroke,
            strokeWidth: props.strokeWidth,
            fill: props.fill,
            name: props.name,
            hide: props.hide,
            unit: props.unit,
            tooltipType: props.tooltipType,
            id: id
        }), /*#__PURE__*/ react.createElement(SetGraphicalItem/* SetCartesianGraphicalItem */.p, {
            type: "area",
            id: id,
            data: props.data,
            dataKey: props.dataKey,
            xAxisId: props.xAxisId,
            yAxisId: props.yAxisId,
            zAxisId: 0,
            stackId: (0,ChartUtils/* getNormalizedStackId */.$8)(props.stackId),
            hide: props.hide,
            barSize: undefined,
            baseValue: props.baseValue,
            isPanorama: isPanorama,
            connectNulls: props.connectNulls
        }), /*#__PURE__*/ react.createElement(AreaImpl, _extends({}, props, {
            id: id
        }))));
}
/**
 * @provides LabelListContext
 * @consumes CartesianChartContext
 */ var Area = /*#__PURE__*/ react.memo(AreaFn, propsAreEqual/* propsAreEqual */.P);
// @ts-expect-error we need to set the displayName for debugging purposes
Area.displayName = 'Area';

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Finsights%2FModelOverTimeChart.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xODB1NG9mMCB7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycno3KTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzgpOwogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MzEpIHZhcigtLV8xOWV6MnJ6MzEpOwp9Ci5fMTgwdTRvZjEgewogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgZm9udC13ZWlnaHQ6IHZhcigtLV8xOWV6MnJ6M2gpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejApOwogIG1hcmdpbi1ib3R0b206IHZhcigtLV8xOWV6MnJ6MzApOwp9Ci5fMTgwdTRvZjIgewogIHdpZHRoOiAxMDAlOwogIGhlaWdodDogMjQwcHg7Cn0KLl8xODB1NG9mMyB7CiAgZGlzcGxheTogZmxleDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGp1c3RpZnktY29udGVudDogY2VudGVyOwogIGhlaWdodDogMjQwcHg7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKfQouXzE4MHU0b2Y0IHsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXgtd3JhcDogd3JhcDsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MzApOwogIG1hcmdpbi10b3A6IHZhcigtLV8xOWV6MnJ6MnopOwp9Ci5fMTgwdTRvZjUgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MnkpOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYSk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7Cn0KLl8xODB1NG9mNiB7CiAgd2lkdGg6IDEwcHg7CiAgaGVpZ2h0OiAxMHB4OwogIGJvcmRlci1yYWRpdXM6IDJweDsKICBmbGV4LXNocmluazogMDsKfQouXzE4MHU0b2Y3IHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnp6KTsKICBmb250LXN0eWxlOiBpdGFsaWM7Cn0%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Finsights_2FModelOverTimeChart_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xODB1NG9mMCB7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycno3KTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzgpOwogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MzEpIHZhcigtLV8xOWV6MnJ6MzEpOwp9Ci5fMTgwdTRvZjEgewogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgZm9udC13ZWlnaHQ6IHZhcigtLV8xOWV6MnJ6M2gpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejApOwogIG1hcmdpbi1ib3R0b206IHZhcigtLV8xOWV6MnJ6MzApOwp9Ci5fMTgwdTRvZjIgewogIHdpZHRoOiAxMDAlOwogIGhlaWdodDogMjQwcHg7Cn0KLl8xODB1NG9mMyB7CiAgZGlzcGxheTogZmxleDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGp1c3RpZnktY29udGVudDogY2VudGVyOwogIGhlaWdodDogMjQwcHg7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKfQouXzE4MHU0b2Y0IHsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXgtd3JhcDogd3JhcDsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MzApOwogIG1hcmdpbi10b3A6IHZhcigtLV8xOWV6MnJ6MnopOwp9Ci5fMTgwdTRvZjUgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MnkpOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYSk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7Cn0KLl8xODB1NG9mNiB7CiAgd2lkdGg6IDEwcHg7CiAgaGVpZ2h0OiAxMHB4OwogIGJvcmRlci1yYWRpdXM6IDJweDsKICBmbGV4LXNocmluazogMDsKfQouXzE4MHU0b2Y3IHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnp6KTsKICBmb250LXN0eWxlOiBpdGFsaWM7Cn0_22_7D = __webpack_require__(46649);
;// ./src/app/insights/ModelOverTimeChart.css.ts

var chartCard = '_180u4of0';
var chartTitle = '_180u4of1';
var chartWrap = '_180u4of2';
var emptyChart = '_180u4of3';
var legend = '_180u4of4';
var legendItem = '_180u4of5';
var legendSwatch = '_180u4of6';
var unpricedLegendLabel = '_180u4of7';
;// ./src/app/insights/ModelOverTimeChart.tsx
// +feature: insights-dashboard
/* __next_internal_client_entry_do_not_use__ ModelOverTimeChart auto */ 



// Distinct colours per model family — maps first to most-expensive models.
const MODEL_COLORS = {
    "claude-opus-4": "var(--primary)",
    "claude-sonnet-4": "#22c55e",
    "claude-haiku-4": "#f59e0b",
    "claude-opus-3": "#8b5cf6",
    "claude-sonnet-3": "#06b6d4",
    "claude-haiku-3": "#ec4899"
};
const FALLBACK_COLORS = [
    "#64748b",
    "#a78bfa",
    "#fb923c",
    "#34d399"
];
function colorForModel(family, index) {
    return MODEL_COLORS[family] ?? FALLBACK_COLORS[index % FALLBACK_COLORS.length];
}
function collectModels(daily) {
    const seen = new Set();
    for (const bucket of daily){
        const map = bucket.costByModel;
        for (const k of Object.keys(map ?? {}))seen.add(k);
    }
    return Array.from(seen).sort();
}
function collectUnpricedModels(daily) {
    const unpriced = new Set();
    for (const bucket of daily){
        for (const m of bucket.unpricedModels ?? [])unpriced.add(m);
    }
    return unpriced;
}
function toDataPoints(daily, models, mode) {
    return daily.map((b)=>{
        const d = b.date ? new Date(Number(b.date.seconds) * 1000) : new Date(0);
        const label = d.toLocaleDateString(undefined, {
            month: "short",
            day: "numeric"
        });
        const point = {
            date: label
        };
        const costMap = b.costByModel;
        const tokMap = b.tokensByModel;
        for (const m of models){
            if (mode === "cost") {
                point[m] = costMap?.[m] ?? 0;
            } else {
                const v = tokMap?.[m];
                point[m] = v != null ? Number(v) : 0;
            }
        }
        return point;
    });
}
function fmtTick(v, mode) {
    if (mode === "cost") return `$${v.toFixed(2)}`;
    if (v >= 1000000) return `${(v / 1000000).toFixed(1)}M`;
    if (v >= 1000) return `${(v / 1000).toFixed(0)}K`;
    return String(v);
}
function ModelOverTimeChart({ daily, mode = "cost" }) {
    const models = (0,react.useMemo)(()=>collectModels(daily), [
        daily
    ]);
    const data = (0,react.useMemo)(()=>toDataPoints(daily, models, mode), [
        daily,
        models,
        mode
    ]);
    const unpricedModels = (0,react.useMemo)(()=>collectUnpricedModels(daily), [
        daily
    ]);
    if (daily.length === 0 || models.length === 0) {
        return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
            className: chartCard,
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                    className: chartTitle,
                    children: "Model Usage Over Time"
                }),
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                    className: emptyChart,
                    children: "No data"
                })
            ]
        });
    }
    const label = mode === "cost" ? "Spend by Model (USD)" : "Tokens by Model";
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: chartCard,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                className: chartTitle,
                children: label
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                className: chartWrap,
                children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)(ResponsiveContainer/* ResponsiveContainer */.u, {
                    width: "100%",
                    height: "100%",
                    children: /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(AreaChart, {
                        data: data,
                        margin: {
                            top: 4,
                            right: 8,
                            bottom: 4,
                            left: 8
                        },
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("defs", {
                                children: models.map((m, i)=>/*#__PURE__*/ (0,react_jsx_runtime.jsxs)("linearGradient", {
                                        id: `grad-${i}`,
                                        x1: "0",
                                        y1: "0",
                                        x2: "0",
                                        y2: "1",
                                        children: [
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("stop", {
                                                offset: "5%",
                                                stopColor: colorForModel(m, i),
                                                stopOpacity: 0.3
                                            }),
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("stop", {
                                                offset: "95%",
                                                stopColor: colorForModel(m, i),
                                                stopOpacity: 0.0
                                            })
                                        ]
                                    }, m))
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(CartesianGrid/* CartesianGrid */.d, {
                                strokeDasharray: "3 3",
                                stroke: "rgba(128,128,128,0.15)"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(XAxis/* XAxis */.W, {
                                dataKey: "date",
                                tick: {
                                    fontSize: 11
                                },
                                tickLine: false,
                                axisLine: false
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(YAxis/* YAxis */.h, {
                                tickFormatter: (v)=>fmtTick(v, mode),
                                tick: {
                                    fontSize: 11
                                },
                                tickLine: false,
                                axisLine: false,
                                width: 56
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(Tooltip/* Tooltip */.m, {
                                formatter: (v)=>mode === "cost" ? `$${Number(v).toFixed(4)}` : fmtTick(Number(v), "tokens"),
                                contentStyle: {
                                    fontSize: "12px"
                                }
                            }),
                            models.map((m, i)=>/*#__PURE__*/ (0,react_jsx_runtime.jsx)(Area, {
                                    type: "monotone",
                                    dataKey: m,
                                    stackId: "1",
                                    stroke: colorForModel(m, i),
                                    fill: `url(#grad-${i})`,
                                    strokeWidth: 1.5,
                                    dot: false,
                                    activeDot: {
                                        r: 3
                                    }
                                }, m))
                        ]
                    })
                })
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                className: legend,
                children: models.map((m, i)=>/*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: legendItem,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                                className: legendSwatch,
                                style: {
                                    background: colorForModel(m, i)
                                }
                            }),
                            m,
                            unpricedModels.has(m) && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: unpricedLegendLabel,
                                children: " (pricing unavailable)"
                            })
                        ]
                    }, m))
            })
        ]
    });
}


/***/ }),

/***/ 86104:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   x: () => (/* binding */ getRadiusAndStrokeWidthFromDot)
/* harmony export */ });
/* harmony import */ var _svgPropertiesNoEvents__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40959);

function getRadiusAndStrokeWidthFromDot(dot) {
    var props = (0,_svgPropertiesNoEvents__WEBPACK_IMPORTED_MODULE_0__/* .svgPropertiesNoEventsFromUnknown */ .ic)(dot);
    var defaultR = 3;
    var defaultStrokeWidth = 2;
    if (props != null) {
        var { r, strokeWidth } = props;
        var realR = Number(r);
        var realStrokeWidth = Number(strokeWidth);
        if (Number.isNaN(realR) || realR < 0) {
            realR = defaultR;
        }
        if (Number.isNaN(realStrokeWidth) || realStrokeWidth < 0) {
            realStrokeWidth = defaultStrokeWidth;
        }
        return {
            r: realR,
            strokeWidth: realStrokeWidth
        };
    }
    return {
        r: defaultR,
        strokeWidth: defaultStrokeWidth
    };
}


/***/ })

};
;