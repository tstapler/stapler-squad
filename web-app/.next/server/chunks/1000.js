"use strict";
exports.id = 1000;
exports.ids = [1000];
exports.modules = {

/***/ 1000:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  DailySpendChart: () => (/* binding */ DailySpendChart)
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
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/chart/LineChart.js




var allowedTooltipTypes = [
    'axis'
];
/**
 * @consumes ResponsiveContainerContext
 * @provides CartesianViewBoxContext
 * @provides CartesianChartContext
 */ var LineChart = /*#__PURE__*/ (0,react.forwardRef)((props, ref)=>{
    return /*#__PURE__*/ react.createElement(CartesianChart/* CartesianChart */.P, {
        chartName: "LineChart",
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
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/container/Layer.js
var Layer = __webpack_require__(44084);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/component/LabelList.js
var LabelList = __webpack_require__(65379);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/component/Dots.js
var Dots = __webpack_require__(58497);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/DataUtils.js
var DataUtils = __webpack_require__(36223);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/ReactUtils.js
var ReactUtils = __webpack_require__(75976);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/ChartUtils.js + 6 modules
var ChartUtils = __webpack_require__(26533);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/component/ActivePoints.js
var ActivePoints = __webpack_require__(25928);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/SetTooltipEntrySettings.js
var SetTooltipEntrySettings = __webpack_require__(25019);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/context/ErrorBarContext.js
var ErrorBarContext = __webpack_require__(23991);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/cartesian/GraphicalItemClipPath.js
var GraphicalItemClipPath = __webpack_require__(71499);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/context/chartLayoutContext.js
var chartLayoutContext = __webpack_require__(87644);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/context/PanoramaContext.js
var PanoramaContext = __webpack_require__(73327);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/reselect@5.1.1/node_modules/reselect/dist/reselect.mjs
var reselect = __webpack_require__(48559);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/selectors/dataSelectors.js
var dataSelectors = __webpack_require__(92396);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/selectors/axisSelectors.js + 8 modules
var axisSelectors = __webpack_require__(66882);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/selectors/lineSelectors.js






var selectXAxisWithScale = (state, xAxisId, _yAxisId, isPanorama)=>(0,axisSelectors/* selectAxisWithScale */.Gx)(state, 'xAxis', xAxisId, isPanorama);
var selectXAxisTicks = (state, xAxisId, _yAxisId, isPanorama)=>(0,axisSelectors/* selectTicksOfGraphicalItem */.CR)(state, 'xAxis', xAxisId, isPanorama);
var selectYAxisWithScale = (state, _xAxisId, yAxisId, isPanorama)=>(0,axisSelectors/* selectAxisWithScale */.Gx)(state, 'yAxis', yAxisId, isPanorama);
var selectYAxisTicks = (state, _xAxisId, yAxisId, isPanorama)=>(0,axisSelectors/* selectTicksOfGraphicalItem */.CR)(state, 'yAxis', yAxisId, isPanorama);
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
var pickLineId = (_state, _xAxisId, _yAxisId, _isPanorama, id)=>id;
function isLineSettings(item) {
    return item.type === 'line';
}
/*
 * There is a race condition problem because we read some data from props and some from the state.
 * The state is updated through a dispatch and is one render behind,
 * and so we have this weird one tick render where the displayedData in one selector have the old dataKey
 * but the new dataKey in another selector.
 *
 * So here instead of reading the dataKey from the props, we always read it from the state.
 */ var selectSynchronisedLineSettings = (0,reselect/* createSelector */.Mz)([
    axisSelectors/* selectUnfilteredCartesianItems */.ld,
    pickLineId
], (graphicalItems, id)=>graphicalItems.filter(isLineSettings).find((x)=>x.id === id));
var selectLinePoints = (0,reselect/* createSelector */.Mz)([
    chartLayoutContext/* selectChartLayout */.fz,
    selectXAxisWithScale,
    selectYAxisWithScale,
    selectXAxisTicks,
    selectYAxisTicks,
    selectSynchronisedLineSettings,
    selectBandSize,
    dataSelectors/* selectChartDataWithIndexesIfNotInPanoramaPosition4 */.k$
], (layout, xAxis, yAxis, xAxisTicks, yAxisTicks, lineSettings, bandSize, _ref)=>{
    var { chartData, dataStartIndex, dataEndIndex } = _ref;
    if (lineSettings == null || xAxis == null || yAxis == null || xAxisTicks == null || yAxisTicks == null || xAxisTicks.length === 0 || yAxisTicks.length === 0 || bandSize == null || layout !== 'horizontal' && layout !== 'vertical') {
        return undefined;
    }
    var { dataKey, data } = lineSettings;
    var displayedData;
    if (data != null && data.length > 0) {
        displayedData = data;
    } else {
        displayedData = chartData === null || chartData === void 0 ? void 0 : chartData.slice(dataStartIndex, dataEndIndex + 1);
    }
    if (displayedData == null) {
        return undefined;
    }
    return computeLinePoints({
        layout,
        xAxis,
        yAxis,
        xAxisTicks,
        yAxisTicks,
        dataKey,
        bandSize,
        displayedData
    });
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/hooks.js
var hooks = __webpack_require__(53151);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/SetLegendPayload.js
var SetLegendPayload = __webpack_require__(49190);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/useAnimationId.js
var useAnimationId = __webpack_require__(87810);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/resolveDefaultProps.js
var resolveDefaultProps = __webpack_require__(25055);
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
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/svgPropertiesAndEvents.js
var svgPropertiesAndEvents = __webpack_require__(30386);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/getRadiusAndStrokeWidthFromDot.js
var getRadiusAndStrokeWidthFromDot = __webpack_require__(86104);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/ActiveShapeUtils.js + 17 modules
var ActiveShapeUtils = __webpack_require__(5800);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/zIndex/ZIndexLayer.js
var ZIndexLayer = __webpack_require__(80731);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/zIndex/DefaultZIndexes.js
var DefaultZIndexes = __webpack_require__(71115);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/propsAreEqual.js
var propsAreEqual = __webpack_require__(7050);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/cartesian/Line.js
var _excluded = [
    "id"
], _excluded2 = [
    "type",
    "layout",
    "connectNulls",
    "needClip",
    "shape"
], _excluded3 = [
    "activeDot",
    "animateNewValues",
    "animationBegin",
    "animationDuration",
    "animationEasing",
    "connectNulls",
    "dot",
    "hide",
    "isAnimationActive",
    "label",
    "legendType",
    "xAxisId",
    "yAxisId",
    "id"
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































/**
 * Internal props, combination of external props + defaultProps + private Recharts state
 */ /**
 * External props, intended for end users to fill in
 */ /**
 * Because of naming conflict, we are forced to ignore certain (valid) SVG attributes.
 */ var computeLegendPayloadFromAreaData = (props)=>{
    var { dataKey, name, stroke, legendType, hide } = props;
    return [
        {
            inactive: hide,
            dataKey,
            type: legendType,
            color: stroke,
            value: (0,ChartUtils/* getTooltipNameProp */.uM)(name, dataKey),
            payload: props
        }
    ];
};
var SetLineTooltipEntrySettings = /*#__PURE__*/ react.memo((_ref)=>{
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
            color: stroke,
            unit,
            graphicalItemId: id
        }
    };
    return /*#__PURE__*/ react.createElement(SetTooltipEntrySettings/* SetTooltipEntrySettings */.r, {
        tooltipEntrySettings: tooltipEntrySettings
    });
});
/**
 * Generates a simple stroke-dasharray string for animating a line draw effect.
 *
 * Uses `totalLength` as the gap (instead of `totalLength - length`) to prevent a floating-point
 * precision artifact: when fractional dash and gap values are serialized to a string attribute
 * and re-parsed by the SVG renderer, their sum can differ from the actual path length by a ULP,
 * causing the dasharray pattern to repeat and render a phantom dot at the path endpoint
 * with round or square strokeLinecap.
 *
 * @param totalLength The total length of the SVG path
 * @param length The currently visible portion of the path
 * @returns A stroke-dasharray string like "50px 200px"
 */ var generateSimpleStrokeDasharray = (totalLength, length)=>{
    return "".concat(length, "px ").concat(totalLength, "px");
};
/**
 * Repeats a dash pattern array a given number of times.
 *
 * If the input array has an odd length, a trailing `0` is appended to make it even
 * before repeating, because SVG stroke-dasharray patterns must have an even number
 * of values to cycle correctly between dash and gap segments.
 *
 * @param lines Array of dash/gap lengths to repeat
 * @param count Number of times to repeat the pattern
 * @returns A new array with the pattern repeated `count` times
 */ function repeat(lines, count) {
    var linesUnit = lines.length % 2 !== 0 ? [
        ...lines,
        0
    ] : lines;
    var result = [];
    for(var i = 0; i < count; ++i){
        result.push(...linesUnit);
    }
    return result;
}
/**
 * Computes a stroke-dasharray string for animating a custom-dashed line draw effect.
 *
 * Given a user-specified dash pattern (e.g. `"7,3"`), this function builds a dasharray
 * that reveals exactly `length` pixels of that pattern, followed by a gap of `totalLength`
 * to hide the remainder of the path.
 *
 * Like {@link generateSimpleStrokeDasharray}, the trailing gap uses `totalLength` rather than
 * `totalLength - length` to avoid floating-point precision artifacts with round/square strokeLinecap.
 *
 * @param length The currently visible portion of the path
 * @param totalLength The total length of the SVG path
 * @param lines The user-specified dash pattern as an array of numbers (e.g. [7, 3])
 * @returns A stroke-dasharray string incorporating the custom dash pattern
 */ var getStrokeDasharray = (length, totalLength, lines)=>{
    var lineLength = lines.reduce((pre, next)=>pre + next, 0);
    // if lineLength is 0 return the default when no strokeDasharray is provided
    if (!lineLength) {
        return generateSimpleStrokeDasharray(totalLength, length);
    }
    var count = Math.floor(length / lineLength);
    var remainLength = length % lineLength;
    var remainLines = [];
    for(var i = 0, sum = 0; i < lines.length; sum += (_lines$i = lines[i]) !== null && _lines$i !== void 0 ? _lines$i : 0, ++i){
        var _lines$i;
        var lineValue = lines[i];
        if (lineValue != null && sum + lineValue > remainLength) {
            remainLines = [
                ...lines.slice(0, i),
                remainLength - sum
            ];
            break;
        }
    }
    var emptyLines = remainLines.length % 2 === 0 ? [
        0,
        totalLength
    ] : [
        totalLength
    ];
    return [
        ...repeat(lines, count),
        ...remainLines,
        ...emptyLines
    ].map((line)=>"".concat(line, "px")).join(', ');
};
function LineDotsWrapper(_ref2) {
    var { clipPathId, points, props } = _ref2;
    var { dot, dataKey, needClip } = props;
    /*
   * Exclude ID from the props passed to the Dots component
   * because then the ID would be applied to multiple dots, and it would no longer be unique.
   */ var { id } = props, propsWithoutId = _objectWithoutProperties(props, _excluded);
    var lineProps = (0,svgPropertiesNoEvents/* svgPropertiesNoEvents */.uZ)(propsWithoutId);
    return /*#__PURE__*/ react.createElement(Dots/* Dots */.n, {
        points: points,
        dot: dot,
        className: "recharts-line-dots",
        dotClassName: "recharts-line-dot",
        dataKey: dataKey,
        baseProps: lineProps,
        needClip: needClip,
        clipPathId: clipPathId
    });
}
function LineLabelListProvider(_ref3) {
    var { showLabels, children, points } = _ref3;
    var labelListEntries = (0,react.useMemo)(()=>{
        return points === null || points === void 0 ? void 0 : points.map((point)=>{
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
                viewBox,
                /*
         * Line is not passing parentViewBox to the LabelList so the labels can escape - looks like a bug, should we pass parentViewBox?
         * Or should this just be the root chart viewBox?
         */ parentViewBox: undefined,
                fill: undefined
            });
        });
    }, [
        points
    ]);
    return /*#__PURE__*/ react.createElement(LabelList/* CartesianLabelListContextProvider */.h8, {
        value: showLabels ? labelListEntries : undefined
    }, children);
}
function StaticCurve(_ref4) {
    var { clipPathId, pathRef, points, strokeDasharray, props } = _ref4;
    var { type, layout, connectNulls, needClip, shape } = props, others = _objectWithoutProperties(props, _excluded2);
    var curveProps = _objectSpread(_objectSpread({}, (0,svgPropertiesAndEvents/* svgPropertiesAndEvents */.a)(others)), {}, {
        fill: 'none',
        className: 'recharts-line-curve',
        clipPath: needClip ? "url(#clipPath-".concat(clipPathId, ")") : undefined,
        points,
        type,
        layout,
        connectNulls,
        strokeDasharray: strokeDasharray !== null && strokeDasharray !== void 0 ? strokeDasharray : props.strokeDasharray
    });
    return /*#__PURE__*/ react.createElement(react.Fragment, null, (points === null || points === void 0 ? void 0 : points.length) > 1 && /*#__PURE__*/ react.createElement(ActiveShapeUtils/* Shape */.y, _extends({
        shapeType: "curve",
        option: shape
    }, curveProps, {
        pathRef: pathRef
    })), /*#__PURE__*/ react.createElement(LineDotsWrapper, {
        points: points,
        clipPathId: clipPathId,
        props: props
    }));
}
function getTotalLength(mainCurve) {
    try {
        return mainCurve && mainCurve.getTotalLength && mainCurve.getTotalLength() || 0;
    } catch (_unused) {
        return 0;
    }
}
function CurveWithAnimation(_ref5) {
    var { clipPathId, props, pathRef, previousPointsRef, longestAnimatedLengthRef } = _ref5;
    var { points, strokeDasharray, isAnimationActive, animationBegin, animationDuration, animationEasing, animateNewValues, width, height, onAnimationEnd, onAnimationStart } = props;
    var prevPoints = previousPointsRef.current;
    var animationId = (0,useAnimationId/* useAnimationId */.n)(points, 'recharts-line-');
    var animationIdRef = (0,react.useRef)(animationId);
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
    var totalLength = getTotalLength(pathRef.current);
    /*
   * Here we want to detect if the length animation has been interrupted.
   * For that we keep a reference to the furthest length that has been animated.
   *
   * And then, to keep things smooth, we add to it the current length that is being animated right now.
   *
   * If we did Math.max then it makes the length animation "pause" but we want to keep it smooth
   * so in case we have some "leftover" length from the previous animation we add it to the current length.
   *
   * This is not perfect because the animation changes speed due to easing. The default easing is 'ease' which is not linear
   * and makes it stand out. But it's good enough I suppose.
   * If we want to fix it then we need to keep track of multiple animations and their easing and timings.
   *
   * If you want to see this in action, try to change the dataKey of the line chart while the initial animation is running.
   * The Line begins with zero length and slowly grows to the full length. While this growth is in progress,
   * change the dataKey and the Line will continue growing from where it has grown so far.
   *
   * This is for the case when new animation triggers. When that happens we get new points, everything re-renders,
   * and we get fresh new state in this component and use the ref stored above.
   *
   * In case when we get render without new animation - for example when opacity changes, or color changes,
   * then the animationId remains the same, and we do not update the starting point.
   * See https://github.com/recharts/recharts/issues/6044
   */ var startingPointRef = (0,react.useRef)(0);
    if (animationIdRef.current !== animationId) {
        startingPointRef.current = longestAnimatedLengthRef.current;
        animationIdRef.current = animationId;
    }
    var startingPoint = startingPointRef.current;
    return /*#__PURE__*/ react.createElement(LineLabelListProvider, {
        points: points,
        showLabels: showLabels
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
        var lengthInterpolated = (0,DataUtils/* interpolate */.GW)(startingPoint, totalLength + startingPoint, t);
        var curLength = Math.min(lengthInterpolated, totalLength);
        var currentStrokeDasharray;
        if (isAnimationActive) {
            if (strokeDasharray) {
                var lines = "".concat(strokeDasharray).split(/[,\s]+/gim).map((num)=>parseFloat(num));
                currentStrokeDasharray = getStrokeDasharray(curLength, totalLength, lines);
            } else {
                currentStrokeDasharray = generateSimpleStrokeDasharray(totalLength, curLength);
            }
        } else {
            currentStrokeDasharray = strokeDasharray == null ? undefined : String(strokeDasharray);
        }
        /*
     * Here it is important to wait a little bit with updating the previousPointsRef
     * before the animation has a time to initialize.
     * If we set the previous pointsRef immediately, we set it before the Legend height it calculated
     * and before pathRef is set.
     * If that happens, the Line will re-render again after Legend had reported its height
     * which will start a new animation with the previous points as the starting point
     * which gives the effect of the Line animating slightly upwards (where the animation distance equals the Legend height).
     * Waiting for t > 0 is indirect but good enough to ensure that the Legend height is calculated and animation works properly.
     *
     * Total length similarly is calculated from the pathRef. We should not update the previousPointsRef
     * before the pathRef is set, otherwise we will have a wrong total length.
     */ if (t > 0 && totalLength > 0) {
            // eslint-disable-next-line no-param-reassign
            previousPointsRef.current = points;
            /*
       * totalLength is set from a ref and is not updated in the first tick of the animation.
       * It defaults to zero which is exactly what we want here because we want to grow from zero,
       * however the same happens when the data change.
       *
       * In that case we want to remember the previous length and continue from there, and only animate the shape.
       *
       * Therefore the totalLength > 0 check.
       *
       * The Animate is about to fire handleAnimationStart which will update the state
       * and cause a re-render and read a new proper totalLength which will be used in the next tick
       * and update the longestAnimatedLengthRef.
       *
       * Why Math.max? Sometimes the curve goes through a smaller length than previously recorded.
       * If we just set it to curLength, then the next animation would start from a smaller length
       * which looks weird. So we keep the longest length ever reached and then animate from there.
       */ // eslint-disable-next-line no-param-reassign
            longestAnimatedLengthRef.current = Math.max(longestAnimatedLengthRef.current, curLength);
        }
        if (prevPoints) {
            var prevPointsDiffFactor = prevPoints.length / points.length;
            var stepData = t === 1 ? points : points.map((entry, index)=>{
                var prevPointIndex = Math.floor(index * prevPointsDiffFactor);
                if (prevPoints[prevPointIndex]) {
                    var prev = prevPoints[prevPointIndex];
                    return _objectSpread(_objectSpread({}, entry), {}, {
                        x: (0,DataUtils/* interpolate */.GW)(prev.x, entry.x, t),
                        y: (0,DataUtils/* interpolate */.GW)(prev.y, entry.y, t)
                    });
                }
                // magic number of faking previous x and y location
                if (animateNewValues) {
                    return _objectSpread(_objectSpread({}, entry), {}, {
                        x: (0,DataUtils/* interpolate */.GW)(width * 2, entry.x, t),
                        y: (0,DataUtils/* interpolate */.GW)(height / 2, entry.y, t)
                    });
                }
                return _objectSpread(_objectSpread({}, entry), {}, {
                    x: entry.x,
                    y: entry.y
                });
            });
            // eslint-disable-next-line no-param-reassign
            previousPointsRef.current = stepData;
            return /*#__PURE__*/ react.createElement(StaticCurve, {
                props: props,
                points: stepData,
                clipPathId: clipPathId,
                pathRef: pathRef,
                strokeDasharray: currentStrokeDasharray
            });
        }
        return /*#__PURE__*/ react.createElement(StaticCurve, {
            props: props,
            points: points,
            clipPathId: clipPathId,
            pathRef: pathRef,
            strokeDasharray: currentStrokeDasharray
        });
    }), /*#__PURE__*/ react.createElement(LabelList/* LabelListFromLabelProp */.qY, {
        label: props.label
    }));
}
function RenderCurve(_ref6) {
    var { clipPathId, props } = _ref6;
    var previousPointsRef = (0,react.useRef)(null);
    var longestAnimatedLengthRef = (0,react.useRef)(0);
    var pathRef = (0,react.useRef)(null);
    return /*#__PURE__*/ react.createElement(CurveWithAnimation, {
        props: props,
        clipPathId: clipPathId,
        previousPointsRef: previousPointsRef,
        longestAnimatedLengthRef: longestAnimatedLengthRef,
        pathRef: pathRef
    });
}
var errorBarDataPointFormatter = (dataPoint, dataKey)=>{
    var _dataPoint$x, _dataPoint$y;
    return {
        x: (_dataPoint$x = dataPoint.x) !== null && _dataPoint$x !== void 0 ? _dataPoint$x : undefined,
        y: (_dataPoint$y = dataPoint.y) !== null && _dataPoint$y !== void 0 ? _dataPoint$y : undefined,
        value: dataPoint.value,
        // getValueByDataKey does not validate the output type
        errorVal: (0,ChartUtils/* getValueByDataKey */.kr)(dataPoint.payload, dataKey)
    };
};
// eslint-disable-next-line react/prefer-stateless-function
class LineWithState extends react.Component {
    render() {
        var { hide, dot, points, className, xAxisId, yAxisId, top, left, width, height, id, needClip, zIndex } = this.props;
        if (hide) {
            return null;
        }
        var layerClass = (0,clsx/* clsx */.$)('recharts-line', className);
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
        }))), /*#__PURE__*/ react.createElement(ErrorBarContext/* SetErrorBarContext */.zk, {
            xAxisId: xAxisId,
            yAxisId: yAxisId,
            data: points,
            dataPointFormatter: errorBarDataPointFormatter,
            errorBarOffset: 0
        }, /*#__PURE__*/ react.createElement(RenderCurve, {
            props: this.props,
            clipPathId: clipPathId
        }))), /*#__PURE__*/ react.createElement(ActivePoints/* ActivePoints */.W, {
            activeDot: this.props.activeDot,
            points: points,
            mainColor: this.props.stroke,
            itemDataKey: this.props.dataKey,
            clipPath: activePointsClipPath
        }));
    }
}
var defaultLineProps = {
    activeDot: true,
    animateNewValues: true,
    animationBegin: 0,
    animationDuration: 1500,
    animationEasing: 'ease',
    connectNulls: false,
    dot: true,
    fill: '#fff',
    hide: false,
    isAnimationActive: 'auto',
    label: false,
    legendType: 'line',
    stroke: '#3182bd',
    strokeWidth: 1,
    xAxisId: 0,
    yAxisId: 0,
    zIndex: DefaultZIndexes/* DefaultZIndexes */.I.line,
    type: 'linear'
};
function LineImpl(props) {
    var _resolveDefaultProps = (0,resolveDefaultProps/* resolveDefaultProps */.e)(props, defaultLineProps), { activeDot, animateNewValues, animationBegin, animationDuration, animationEasing, connectNulls, dot, hide, isAnimationActive, label, legendType, xAxisId, yAxisId, id } = _resolveDefaultProps, everythingElse = _objectWithoutProperties(_resolveDefaultProps, _excluded3);
    var { needClip } = (0,GraphicalItemClipPath/* useNeedsClip */.l)(xAxisId, yAxisId);
    var plotArea = (0,es6_hooks/* usePlotArea */.oM)();
    var layout = (0,chartLayoutContext/* useChartLayout */.WX)();
    var isPanorama = (0,PanoramaContext/* useIsPanorama */.r)();
    var points = (0,hooks/* useAppSelector */.G)((state)=>selectLinePoints(state, xAxisId, yAxisId, isPanorama, id));
    if (layout !== 'horizontal' && layout !== 'vertical' || points == null || plotArea == null) {
        // Cannot render Line in an unsupported layout
        return null;
    }
    var { height, width, x: left, y: top } = plotArea;
    return /*#__PURE__*/ react.createElement(LineWithState, _extends({}, everythingElse, {
        id: id,
        connectNulls: connectNulls,
        dot: dot,
        activeDot: activeDot,
        animateNewValues: animateNewValues,
        animationBegin: animationBegin,
        animationDuration: animationDuration,
        animationEasing: animationEasing,
        isAnimationActive: isAnimationActive,
        hide: hide,
        label: label,
        legendType: legendType,
        xAxisId: xAxisId,
        yAxisId: yAxisId,
        points: points,
        layout: layout,
        height: height,
        width: width,
        left: left,
        top: top,
        needClip: needClip
    }));
}
function computeLinePoints(_ref7) {
    var { layout, xAxis, yAxis, xAxisTicks, yAxisTicks, dataKey, bandSize, displayedData } = _ref7;
    return displayedData.map((entry, index)=>{
        // getValueByDataKey does not validate the output type
        var value = (0,ChartUtils/* getValueByDataKey */.kr)(entry, dataKey);
        if (layout === 'horizontal') {
            var _x = (0,ChartUtils/* getCateCoordinateOfLine */.nb)({
                axis: xAxis,
                ticks: xAxisTicks,
                bandSize,
                entry,
                index
            });
            var _y = (0,DataUtils/* isNullish */.uy)(value) ? null : yAxis.scale.map(value);
            return {
                x: _x,
                y: _y !== null && _y !== void 0 ? _y : null,
                value,
                payload: entry
            };
        }
        var x = (0,DataUtils/* isNullish */.uy)(value) ? null : xAxis.scale.map(value);
        var y = (0,ChartUtils/* getCateCoordinateOfLine */.nb)({
            axis: yAxis,
            ticks: yAxisTicks,
            bandSize,
            entry,
            index
        });
        if (x == null || y == null) {
            return null;
        }
        return {
            x,
            y,
            value,
            payload: entry
        };
    }).filter(Boolean);
}
function LineFn(outsideProps) {
    var props = (0,resolveDefaultProps/* resolveDefaultProps */.e)(outsideProps, defaultLineProps);
    var isPanorama = (0,PanoramaContext/* useIsPanorama */.r)();
    return /*#__PURE__*/ react.createElement(RegisterGraphicalItemId/* RegisterGraphicalItemId */.x, {
        id: props.id,
        type: "line"
    }, (id)=>/*#__PURE__*/ react.createElement(react.Fragment, null, /*#__PURE__*/ react.createElement(SetLegendPayload/* SetLegendPayload */.A, {
            legendPayload: computeLegendPayloadFromAreaData(props)
        }), /*#__PURE__*/ react.createElement(SetLineTooltipEntrySettings, {
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
            type: "line",
            id: id,
            data: props.data,
            xAxisId: props.xAxisId,
            yAxisId: props.yAxisId,
            zAxisId: 0,
            dataKey: props.dataKey,
            hide: props.hide,
            isPanorama: isPanorama
        }), /*#__PURE__*/ react.createElement(LineImpl, _extends({}, props, {
            id: id
        }))));
}
/**
 * @provides LabelListContext
 * @provides ErrorBarContext
 * @consumes CartesianChartContext
 */ var Line = /*#__PURE__*/ react.memo(LineFn, propsAreEqual/* propsAreEqual */.P);
// @ts-expect-error we need to set the displayName for debugging purposes
Line.displayName = 'Line';

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Finsights%2FDailySpendChart.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xbWhja2oxMCB7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycno3KTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzgpOwogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MzEpIHZhcigtLV8xOWV6MnJ6MzEpOwp9Ci5fMW1oY2tqMTEgewogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgZm9udC13ZWlnaHQ6IHZhcigtLV8xOWV6MnJ6M2gpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejApOwogIG1hcmdpbi1ib3R0b206IHZhcigtLV8xOWV6MnJ6MzApOwp9Ci5fMW1oY2tqMTIgewogIHdpZHRoOiAxMDAlOwogIGhlaWdodDogMjIwcHg7Cn0KLl8xbWhja2oxMyB7CiAgZGlzcGxheTogZmxleDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGp1c3RpZnktY29udGVudDogY2VudGVyOwogIGhlaWdodDogMjIwcHg7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKfQouXzFtaGNrajE0IHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnp6KTsKICBmb250LXN0eWxlOiBpdGFsaWM7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Finsights_2FDailySpendChart_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xbWhja2oxMCB7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycno3KTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzgpOwogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MzEpIHZhcigtLV8xOWV6MnJ6MzEpOwp9Ci5fMW1oY2tqMTEgewogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgZm9udC13ZWlnaHQ6IHZhcigtLV8xOWV6MnJ6M2gpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejApOwogIG1hcmdpbi1ib3R0b206IHZhcigtLV8xOWV6MnJ6MzApOwp9Ci5fMW1oY2tqMTIgewogIHdpZHRoOiAxMDAlOwogIGhlaWdodDogMjIwcHg7Cn0KLl8xbWhja2oxMyB7CiAgZGlzcGxheTogZmxleDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGp1c3RpZnktY29udGVudDogY2VudGVyOwogIGhlaWdodDogMjIwcHg7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKfQouXzFtaGNrajE0IHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnp6KTsKICBmb250LXN0eWxlOiBpdGFsaWM7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKfQ_22_7D = __webpack_require__(12155);
;// ./src/app/insights/DailySpendChart.css.ts

var chartCard = '_1mhckj10';
var chartTitle = '_1mhckj11';
var chartWrap = '_1mhckj12';
var emptyChart = '_1mhckj13';
var unpricedFootnote = '_1mhckj14';
// EXTERNAL MODULE: ./src/app/insights/insightsFormatters.ts
var insightsFormatters = __webpack_require__(41010);
;// ./src/app/insights/DailySpendChart.tsx
// +feature: insights-dashboard
/* __next_internal_client_entry_do_not_use__ DailySpendChart auto */ 




function toDataPoints(daily) {
    return daily.map((b)=>{
        const d = b.date ? new Date(Number(b.date.seconds) * 1000) : new Date(0);
        const label = d.toLocaleDateString(undefined, {
            month: "short",
            day: "numeric"
        });
        return {
            date: label,
            cost: b.estimatedCostUsd,
            hasUnpriced: (b.unpricedModels?.length ?? 0) > 0
        };
    });
}
function DailySpendChart({ daily }) {
    const data = (0,react.useMemo)(()=>toDataPoints(daily), [
        daily
    ]);
    if (daily.length === 0) {
        return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
            className: chartCard,
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                    className: chartTitle,
                    children: "Daily Spend"
                }),
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                    className: emptyChart,
                    children: "No data"
                })
            ]
        });
    }
    const unpricedDayCount = data.filter((d)=>d.hasUnpriced).length;
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: chartCard,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                className: chartTitle,
                children: "Daily Spend (USD)"
            }),
            unpricedDayCount > 0 && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                className: unpricedFootnote,
                children: [
                    unpricedDayCount,
                    " day",
                    unpricedDayCount !== 1 ? "s" : "",
                    " include",
                    unpricedDayCount === 1 ? "s" : "",
                    " unpriced model usage"
                ]
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                className: chartWrap,
                children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)(ResponsiveContainer/* ResponsiveContainer */.u, {
                    width: "100%",
                    height: "100%",
                    children: /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(LineChart, {
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
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(Line, {
                                type: "monotone",
                                dataKey: "cost",
                                dot: false,
                                strokeWidth: 2,
                                stroke: "var(--primary)",
                                activeDot: {
                                    r: 4
                                }
                            })
                        ]
                    })
                })
            })
        ]
    });
}


/***/ }),

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

/***/ 12155:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "._1mhckj10 {\n  background: var(--_19ez2rz7);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: var(--_19ez2rz38);\n  padding: var(--_19ez2rz31) var(--_19ez2rz31);\n}\n._1mhckj11 {\n  font-size: var(--_19ez2rz3b);\n  font-weight: var(--_19ez2rz3h);\n  color: var(--_19ez2rz0);\n  margin-bottom: var(--_19ez2rz30);\n}\n._1mhckj12 {\n  width: 100%;\n  height: 220px;\n}\n._1mhckj13 {\n  display: flex;\n  align-items: center;\n  justify-content: center;\n  height: 220px;\n  color: var(--_19ez2rz2);\n  font-size: var(--_19ez2rz3b);\n}\n._1mhckj14 {\n  color: var(--_19ez2rzz);\n  font-style: italic;\n  font-size: var(--_19ez2rz3b);\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


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